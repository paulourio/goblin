#!/usr/bin/env python3
"""ELF extractor and Lean 4 verifier for Track 5E amd64 SIMD kernels.

Compiles CGO_ENABLED=0 Linux amd64 GOEXPERIMENT=simd test binaries for
internal/simd, parses ELF64 .text/.rodata/.symtab directly, cross-checks
raw bytes and mnemonics against `go tool objdump`, emits 16-byte chunked
Lean 4 byte definitions, and verifies Track5E.lean against fresh builds.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import struct
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

RE_OBJDUMP_TEXT = re.compile(r"^TEXT\s+(\S+)\(SB\)")
RE_OBJDUMP_INSN = re.compile(r"^\s*\S+\s+0x([0-9a-fA-F]+)\s+([0-9a-fA-F]+)\s+(.*)$")

RE_LEAN_BYTE_CHUNKS = re.compile(
    r"def\s+(\w+)ByteChunks\s*:\s*List\s*\(List\s+UInt8\)\s*:=\s*\[(.*?)\]\s*\n"
    r"def\s+\1Bytes",
    re.DOTALL,
)
RE_HEX_BYTE = re.compile(r"0x[0-9a-fA-F]{1,2}")

VZEROUPPER_BYTES = b"\xc5\xf8\x77"


@dataclass(frozen=True)
class DecodedSpan:
    """Single decoded amd64 instruction span inside a function body."""

    offset: int
    length: int
    kind: str
    rel_disp_offset: int | None = None
    jump_target: int | None = None


@dataclass(frozen=True)
class Amd64RoutineBinary:
    """Extracted amd64 machine bytes, external calls, and rodata constants."""

    symbol: str
    raw_bytes: bytes
    targets: dict[int, str]
    rodata: dict[int, int]
    rodata_symbols: dict[int, str]


@dataclass(frozen=True)
class VerifySpec:
    """Mapping between a Track5E.lean prefix and its internal/simd symbol."""

    lean_prefix: str
    go_pkg: str
    go_symbol: str


VERIFY_SPECS: tuple[VerifySpec, ...] = (
    VerifySpec("allAbsLE64AVX2", "./internal/simd", "internal/simd.allAbsLE64AVX2"),
    VerifySpec("allAbsLE64AVX512", "./internal/simd", "internal/simd.allAbsLE64AVX512"),
    VerifySpec("minNoNaN64AVX2", "./internal/simd", "internal/simd.minNoNaN64AVX2"),
    VerifySpec("minNoNaN64AVX512", "./internal/simd", "internal/simd.minNoNaN64AVX512"),
    VerifySpec(
        "initReducedRow64AVX2", "./internal/simd", "internal/simd.initReducedRow64AVX2"
    ),
    VerifySpec(
        "initReducedRow64AVX512",
        "./internal/simd",
        "internal/simd.initReducedRow64AVX512",
    ),
)


def resolve_repo_root(explicit_root: Path | None) -> Path:
    """Locate the repository root containing go.mod."""
    root = (
        explicit_root.resolve()
        if explicit_root is not None
        else Path(__file__).resolve().parent.parent
    )
    if not (root / "go.mod").is_file():
        msg = f"missing go.mod under repository root {root}"
        raise FileNotFoundError(msg)
    return root


def read_module_path(repo_root: Path) -> str:
    """Read the canonical Go module path from go.mod."""
    for line in (repo_root / "go.mod").read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped.startswith("module "):
            return stripped.split()[1].strip()
    msg = "module directive missing in go.mod"
    raise ValueError(msg)


def strip_module_prefixes(name: str, module_path: str) -> str:
    """Remove Go module slash and dot prefixes from linker symbol names."""
    return name.replace(f"{module_path}/", "").replace(f"{module_path}.", "")


def _cstr(buf: bytes, offset: int) -> str:
    """Extract a NUL-terminated UTF-8 string from an ELF string table."""
    end = buf.index(0, offset)
    return buf[offset:end].decode("utf-8", errors="replace")


def _sign_extend(value: int, bits: int) -> int:
    """Sign-extend an unsigned bitfield to a Python int."""
    mask = (1 << bits) - 1
    masked = value & mask
    sign_bit = 1 << (bits - 1)
    return masked - (1 << bits) if (masked & sign_bit) else masked


def _decode_modrm_len(data: bytes, pos: int) -> tuple[int, bool, int | None]:
    """Return (bytes_consumed, is_rip_rel, disp32_offset) starting at ModR/M."""
    if pos >= len(data):
        msg = f"truncated ModR/M at offset 0x{pos:x}"
        raise ValueError(msg)
    modrm = data[pos]
    mod = (modrm >> 6) & 0x3
    rm = modrm & 0x7
    cur = pos + 1
    if mod == 3:
        return 1, False, None
    if mod == 0 and rm == 5:
        if cur + 4 > len(data):
            msg = f"truncated RIP-relative disp32 at offset 0x{pos:x}"
            raise ValueError(msg)
        return 5, True, cur
    has_sib = rm == 4
    sib_base = rm
    if has_sib:
        if cur >= len(data):
            msg = f"truncated SIB byte at offset 0x{pos:x}"
            raise ValueError(msg)
        sib_base = data[cur] & 0x7
        cur += 1
    if mod == 0 and has_sib and sib_base == 5:
        cur += 4
    elif mod == 1:
        cur += 1
    elif mod == 2:
        cur += 4
    if cur > len(data):
        msg = f"truncated ModR/M displacement at offset 0x{pos:x}"
        raise ValueError(msg)
    return cur - pos, False, None


def decode_amd64_insn_span(data: bytes, offset: int) -> DecodedSpan:
    """Decode one amd64 scalar, SSE, VEX, or EVEX instruction span."""
    n = len(data)
    if offset >= n:
        msg = f"offset 0x{offset:x} out of bounds for length {n}"
        raise ValueError(msg)

    pos = offset
    b0 = data[pos]

    # 2-byte VEX (0xC5).
    if b0 == 0xC5:
        if pos + 3 > n:
            msg = f"truncated 2-byte VEX at 0x{offset:x}"
            raise ValueError(msg)
        op = data[pos + 2]
        if data[pos : pos + 3] == VZEROUPPER_BYTES:
            return DecodedSpan(offset, 3, "vzeroupper")
        if op in (0x10, 0x11, 0x50, 0x59, 0x5C, 0x5D, 0x6F, 0x7F, 0x93, 0xDB):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 3)
            kind = "rip_rodata" if is_rip else "vex2"
            return DecodedSpan(offset, 3 + mlen, kind, disp_off)
        if op == 0xC2:
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 3)
            if pos + 3 + mlen + 1 > n:
                msg = f"truncated VEX2 VCMPPD imm8 at 0x{offset:x}"
                raise ValueError(msg)
            kind = "rip_rodata" if is_rip else "vex2"
            return DecodedSpan(offset, 3 + mlen + 1, kind, disp_off)
        msg = f"unsupported VEX2 opcode 0x{op:02x} at 0x{offset:x}"
        raise ValueError(msg)

    # 3-byte VEX (0xC4).
    if b0 == 0xC4:
        if pos + 4 > n:
            msg = f"truncated 3-byte VEX at 0x{offset:x}"
            raise ValueError(msg)
        vmap = data[pos + 1] & 0x1F
        op = data[pos + 3]
        if vmap == 1 and op in (0x59, 0x5C, 0x5D, 0x6E, 0x6F, 0x7F, 0xDB):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 4)
            return DecodedSpan(
                offset, 4 + mlen, "rip_rodata" if is_rip else "vex3", disp_off
            )
        if vmap == 2 and op in (0x19, 0x59):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 4)
            return DecodedSpan(
                offset, 4 + mlen, "rip_rodata" if is_rip else "vex3", disp_off
            )
        if vmap == 3 and op == 0x4C:
            mlen, _, _ = _decode_modrm_len(data, pos + 4)
            if pos + 4 + mlen + 1 > n:
                msg = f"truncated VPBLENDVB imm8 at 0x{offset:x}"
                raise ValueError(msg)
            return DecodedSpan(offset, 4 + mlen + 1, "vex3")
        msg = f"unsupported VEX3 map={vmap} opcode=0x{op:02x} at 0x{offset:x}"
        raise ValueError(msg)

    # 4-byte EVEX (0x62).
    if b0 == 0x62:
        if pos + 5 > n:
            msg = f"truncated EVEX instruction at 0x{offset:x}"
            raise ValueError(msg)
        emap = data[pos + 1] & 0x07
        op = data[pos + 4]
        if emap == 1 and op in (0x59, 0x5C, 0x5D, 0x6F, 0x7F, 0xDB):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 5)
            return DecodedSpan(
                offset, 5 + mlen, "rip_rodata" if is_rip else "evex", disp_off
            )
        if emap == 1 and op == 0xC2:
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 5)
            if pos + 5 + mlen + 1 > n:
                msg = f"truncated EVEX VCMPPD imm8 at 0x{offset:x}"
                raise ValueError(msg)
            return DecodedSpan(
                offset, 5 + mlen + 1, "rip_rodata" if is_rip else "evex", disp_off
            )
        if emap == 2 and op in (0x19, 0x38, 0x39, 0x59, 0x64):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos + 5)
            return DecodedSpan(
                offset, 5 + mlen, "rip_rodata" if is_rip else "evex", disp_off
            )
        msg = f"unsupported EVEX map={emap} opcode=0x{op:02x} at 0x{offset:x}"
        raise ValueError(msg)

    # Legacy prefixes (66, F2, F3) followed by optional REX (40..4F).
    p66 = False
    pf2 = False
    pf3 = False
    while pos < n and data[pos] in (0x66, 0xF2, 0xF3):
        if data[pos] == 0x66:
            p66 = True
        elif data[pos] == 0xF2:
            pf2 = True
        else:
            pf3 = True
        pos += 1

    rex = 0
    if pos < n and 0x40 <= data[pos] <= 0x4F:
        rex = data[pos]
        pos += 1
    if pos >= n:
        msg = f"truncated opcode after prefixes at 0x{offset:x}"
        raise ValueError(msg)

    op = data[pos]
    pos += 1

    # Single-byte opcodes.
    if op == 0x90:
        return DecodedSpan(offset, pos - offset, "nop")
    if 0x50 <= op <= 0x5F or op == 0xC3:
        return DecodedSpan(offset, pos - offset, "ret" if op == 0xC3 else "gp")
    if op == 0x3C:
        return DecodedSpan(offset, (pos - offset) + 1, "gp")
    if 0x70 <= op <= 0x7F or op == 0xEB:
        if pos >= n:
            msg = f"truncated rel8 branch at 0x{offset:x}"
            raise ValueError(msg)
        rel = _sign_extend(data[pos], 8)
        next_off = pos + 1
        return DecodedSpan(offset, next_off - offset, "jcc", None, next_off + rel)
    if op == 0xE8:
        if pos + 4 > n:
            msg = f"truncated CALL rel32 at 0x{offset:x}"
            raise ValueError(msg)
        return DecodedSpan(offset, (pos - offset) + 4, "call", pos)
    if op == 0xE9:
        if pos + 4 > n:
            msg = f"truncated JMP rel32 at 0x{offset:x}"
            raise ValueError(msg)
        rel = struct.unpack_from("<i", data, pos)[0]
        next_off = pos + 4
        return DecodedSpan(offset, next_off - offset, "jmp", None, next_off + rel)
    if 0xB8 <= op <= 0xBF:
        imm_len = 8 if (rex & 0x08) != 0 else 4
        if pos + imm_len > n:
            msg = f"truncated MOV imm at 0x{offset:x}"
            raise ValueError(msg)
        return DecodedSpan(offset, (pos - offset) + imm_len, "gp")
    if op in (0x31, 0x39, 0x3B, 0x85, 0x89, 0x8B, 0x8D, 0xFF):
        mlen, is_rip, disp_off = _decode_modrm_len(data, pos)
        kind = "rip_rodata" if is_rip else "gp"
        return DecodedSpan(offset, (pos - offset) + mlen, kind, disp_off)
    if op in (0x80, 0x83):
        mlen, _, _ = _decode_modrm_len(data, pos)
        if pos + mlen + 1 > n:
            msg = f"truncated ALU imm8 at 0x{offset:x}"
            raise ValueError(msg)
        return DecodedSpan(offset, (pos - offset) + mlen + 1, "gp")

    # Two-byte 0F escape opcodes.
    if op == 0x0F:
        if pos >= n:
            msg = f"truncated 0F opcode at 0x{offset:x}"
            raise ValueError(msg)
        op2 = data[pos]
        pos += 1
        if 0x80 <= op2 <= 0x8F:
            if pos + 4 > n:
                msg = f"truncated Jcc rel32 at 0x{offset:x}"
                raise ValueError(msg)
            rel = struct.unpack_from("<i", data, pos)[0]
            next_off = pos + 4
            return DecodedSpan(offset, next_off - offset, "jcc", None, next_off + rel)
        if op2 == 0x1F:
            mlen, _, _ = _decode_modrm_len(data, pos)
            return DecodedSpan(offset, (pos - offset) + mlen, "nop")
        if op2 in (0x10, 0x11, 0x2E, 0x57, 0x5D, 0x6E, 0xEB):
            mlen, is_rip, disp_off = _decode_modrm_len(data, pos)
            kind = "rip_rodata" if is_rip else "sse"
            return DecodedSpan(offset, (pos - offset) + mlen, kind, disp_off)
        msg = (
            f"unsupported 0F {op2:02x} (66={p66}, f2={pf2}, f3={pf3}, "
            f"rex=0x{rex:02x}) at 0x{offset:x}"
        )
        raise ValueError(msg)

    msg = (
        f"unsupported opcode 0x{op:02x} (66={p66}, f2={pf2}, f3={pf3}, "
        f"rex=0x{rex:02x}) at 0x{offset:x}"
    )
    raise ValueError(msg)


def decode_amd64_spans(data: bytes) -> list[DecodedSpan]:
    """Decode an entire amd64 function byte buffer into instruction spans."""
    spans: list[DecodedSpan] = []
    off = 0
    while off < len(data):
        span = decode_amd64_insn_span(data, off)
        spans.append(span)
        off += span.length
    valid_offsets = {s.offset for s in spans}
    for s in spans:
        if s.jump_target is not None and s.jump_target not in valid_offsets:
            msg = (
                f"internal jump at 0x{s.offset:x} targets invalid offset "
                f"0x{s.jump_target:x}"
            )
            raise ValueError(msg)
    return spans


def normalize_amd64_routine(routine: Amd64RoutineBinary) -> bytes:
    """Zero out external CALL rel32 and RIP-relative rodata disp32 fields."""
    spans = decode_amd64_spans(routine.raw_bytes)
    if not any(s.kind == "vzeroupper" for s in spans):
        msg = f"{routine.symbol}: missing VZEROUPPER (c5 f8 77) instruction"
        raise ValueError(msg)

    buf = bytearray(routine.raw_bytes)
    for span in spans:
        if span.kind == "call":
            if span.offset not in routine.targets:
                msg = f"{routine.symbol}@0x{span.offset:x}: missing CALL target"
                raise ValueError(msg)
            assert span.rel_disp_offset is not None
            buf[span.rel_disp_offset : span.rel_disp_offset + 4] = b"\x00\x00\x00\x00"
        elif span.kind == "rip_rodata":
            if span.offset not in routine.rodata:
                msg = f"{routine.symbol}@0x{span.offset:x}: missing rodata entry"
                raise ValueError(msg)
            assert span.rel_disp_offset is not None
            buf[span.rel_disp_offset : span.rel_disp_offset + 4] = b"\x00\x00\x00\x00"
    return bytes(buf)


def build_amd64_simd_test_elf(repo_root: Path, out_elf: Path, go_pkg: str) -> None:
    """Compile a deterministic static Linux amd64 GOEXPERIMENT=simd test binary."""
    env = os.environ.copy()
    env.update(
        {
            "CGO_ENABLED": "0",
            "GOOS": "linux",
            "GOARCH": "amd64",
            "GOAMD64": "v1",
            "GOEXPERIMENT": "simd",
        }
    )
    cmd = ["go", "test", "-c", "-trimpath", "-o", str(out_elf), go_pkg]
    proc = subprocess.run(
        cmd,
        cwd=repo_root,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        msg = f"command {' '.join(cmd)} failed:\n{proc.stderr}\n{proc.stdout}"
        raise RuntimeError(msg)


def _parse_elf64_sections(
    elf_bytes: bytes,
) -> dict[str, tuple[int, int, int]]:
    """Return {section_name: (sh_addr, sh_offset, sh_size)} from ELF64."""
    if elf_bytes[:4] != b"\x7fELF" or elf_bytes[4] != 2 or elf_bytes[5] != 1:
        msg = "expected 64-bit little-endian ELF binary"
        raise ValueError(msg)
    e_shoff = struct.unpack_from("<Q", elf_bytes, 40)[0]
    e_shentsize, e_shnum, e_shstrndx = struct.unpack_from("<HHH", elf_bytes, 58)
    raw_secs: list[tuple[int, int, int, int]] = []
    for i in range(e_shnum):
        off = e_shoff + i * e_shentsize
        sh_name, _sh_type, _sh_flags, sh_addr, sh_offset, sh_size = struct.unpack_from(
            "<IIQQQQ", elf_bytes, off
        )
        raw_secs.append((sh_name, sh_addr, sh_offset, sh_size))
    _, _, str_off, str_size = raw_secs[e_shstrndx]
    shstr = elf_bytes[str_off : str_off + str_size]
    return {
        _cstr(shstr, sh_name): (sh_addr, sh_offset, sh_size)
        for sh_name, sh_addr, sh_offset, sh_size in raw_secs
    }


def _parse_elf64_symtab(
    elf_bytes: bytes, sections: dict[str, tuple[int, int, int]]
) -> tuple[dict[str, tuple[int, int]], dict[int, list[str]]]:
    """Return (name -> (vaddr, size), vaddr -> [names]) from ELF64 .symtab."""
    _, sym_off, sym_size = sections[".symtab"]
    _, str_off, str_size = sections[".strtab"]
    strtab = elf_bytes[str_off : str_off + str_size]
    by_name: dict[str, tuple[int, int]] = {}
    by_addr: dict[int, list[str]] = {}
    for off in range(sym_off, sym_off + sym_size, 24):
        st_name, _st_info, _st_other, _st_shndx, st_value, st_size = struct.unpack_from(
            "<IBBHQQ", elf_bytes, off
        )
        if st_name == 0:
            continue
        name = _cstr(strtab, st_name)
        if name:
            by_name[name] = (st_value, st_size)
            by_addr.setdefault(st_value, []).append(name)
    return by_name, by_addr


def _cross_check_go_objdump(
    repo_root: Path,
    elf_path: Path,
    go_symbol: str,
    module_path: str,
    expected_bytes: bytes,
    spans: list[DecodedSpan],
) -> None:
    """Verify ELF .text bytes and instruction boundaries against go tool objdump."""
    tail = go_symbol.rsplit(".", maxsplit=1)[-1]
    proc = subprocess.run(
        [
            "go",
            "tool",
            "objdump",
            "-s",
            rf"internal/simd\.{re.escape(tail)}$",
            str(elf_path),
        ],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        msg = f"go tool objdump failed for {go_symbol}:\n{proc.stderr}"
        raise RuntimeError(msg)

    obj_bytes = bytearray()
    obj_spans: dict[int, tuple[int, str]] = {}
    base_addr: int | None = None
    in_target = False
    for line in proc.stdout.splitlines():
        m_text = RE_OBJDUMP_TEXT.match(line)
        if m_text is not None:
            in_target = strip_module_prefixes(m_text.group(1), module_path) == go_symbol
            continue
        if not in_target:
            continue
        m_insn = RE_OBJDUMP_INSN.match(line)
        if m_insn is not None:
            addr = int(m_insn.group(1), 16)
            raw_hex = bytes.fromhex(m_insn.group(2))
            if base_addr is None:
                base_addr = addr
            obj_spans[addr - base_addr] = (len(raw_hex), m_insn.group(3).strip())
            obj_bytes.extend(raw_hex)

    if bytes(obj_bytes) != expected_bytes:
        msg = (
            f"{go_symbol}: go tool objdump bytes ({len(obj_bytes)}B) != "
            f"ELF .text bytes ({len(expected_bytes)}B)"
        )
        raise ValueError(msg)

    # Verify all decoded instruction offsets and lengths before the first
    # VZEROUPPER against go tool objdump (Go 1.27 x86asm over-reads trailing
    # bytes after C5 F8 77 and desynchronizes until a later byte boundary).
    first_vzu = min(
        (s.offset for s in spans if s.kind == "vzeroupper"), default=len(expected_bytes)
    )
    for s in spans:
        if s.offset < first_vzu:
            if s.offset not in obj_spans:
                msg = (
                    f"{go_symbol}@0x{s.offset:x}: decoded offset absent from go objdump"
                )
                raise ValueError(msg)
            obj_len, _ = obj_spans[s.offset]
            if obj_len != s.length:
                msg = (
                    f"{go_symbol}@0x{s.offset:x}: decoded len {s.length} != "
                    f"go objdump len {obj_len}"
                )
                raise ValueError(msg)
        elif s.offset == first_vzu and s.offset in obj_spans:
            _, asm = obj_spans[s.offset]
            if not asm.startswith("VZEROUPPER"):
                msg = f"{go_symbol}@0x{s.offset:x}: expected VZEROUPPER, got {asm}"
                raise ValueError(msg)

    # When GNU objdump is installed, cross-check 100% of decoded spans including
    # instructions following VZEROUPPER.
    gnu_objdump = shutil.which("objdump")
    if gnu_objdump is not None:
        full_sym = f"{module_path}/{go_symbol}"
        gproc = subprocess.run(
            [gnu_objdump, "-d", "-w", f"--disassemble={full_sym}", str(elf_path)],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
        if gproc.returncode == 0:
            gnu_spans: list[tuple[int, int]] = []
            g_base: int | None = None
            for line in gproc.stdout.splitlines():
                mg = re.match(r"^\s*([0-9a-f]+):\t([0-9a-f ]+?)\t", line)
                if mg is not None:
                    gaddr = int(mg.group(1), 16)
                    if g_base is None:
                        g_base = gaddr
                    glen = len(mg.group(2).split())
                    gnu_spans.append((gaddr - g_base, glen))
            dec_pairs = [(s.offset, s.length) for s in spans]
            if gnu_spans and gnu_spans != dec_pairs:
                msg = (
                    f"{go_symbol}: decoded spans ({len(dec_pairs)}) != "
                    f"GNU objdump spans ({len(gnu_spans)})"
                )
                raise ValueError(msg)


def extract_amd64_routines(
    repo_root: Path,
    elf_path: Path,
    symbols: list[str],
    module_path: str,
) -> dict[str, Amd64RoutineBinary]:
    """Extract target amd64 functions directly from ELF64 .text and .rodata."""
    elf_bytes = elf_path.read_bytes()
    sections = _parse_elf64_sections(elf_bytes)
    by_name, by_addr = _parse_elf64_symtab(elf_bytes, sections)
    text_addr, text_off, text_size = sections[".text"]
    ro_addr, ro_off, ro_size = sections[".rodata"]

    out: dict[str, Amd64RoutineBinary] = {}
    for go_sym in symbols:
        full_sym = f"{module_path}/{go_sym}"
        if full_sym not in by_name:
            msg = f"symbol '{full_sym}' missing from ELF .symtab"
            raise KeyError(msg)
        sym_vaddr, sym_size = by_name[full_sym]
        if sym_vaddr < text_addr or sym_vaddr + sym_size > text_addr + text_size:
            msg = f"symbol '{full_sym}' outside .text section"
            raise ValueError(msg)
        file_off = text_off + (sym_vaddr - text_addr)
        raw = elf_bytes[file_off : file_off + sym_size]
        spans = decode_amd64_spans(raw)
        _cross_check_go_objdump(repo_root, elf_path, go_sym, module_path, raw, spans)

        targets: dict[int, str] = {}
        rodata: dict[int, int] = {}
        rodata_symbols: dict[int, str] = {}
        for span in spans:
            if span.kind == "call":
                assert span.rel_disp_offset is not None
                rel32 = struct.unpack_from("<i", raw, span.rel_disp_offset)[0]
                tgt_vaddr = sym_vaddr + span.offset + span.length + rel32
                names = by_addr.get(tgt_vaddr)
                if not names:
                    msg = (
                        f"{go_sym}@0x{span.offset:x}: unresolved CALL target "
                        f"0x{tgt_vaddr:x}"
                    )
                    raise ValueError(msg)
                targets[span.offset] = strip_module_prefixes(names[0], module_path)
            elif span.kind == "rip_rodata":
                assert span.rel_disp_offset is not None
                disp32 = struct.unpack_from("<i", raw, span.rel_disp_offset)[0]
                tgt_vaddr = sym_vaddr + span.offset + span.length + disp32
                if tgt_vaddr < ro_addr or tgt_vaddr + 8 > ro_addr + ro_size:
                    msg = (
                        f"{go_sym}@0x{span.offset:x}: RIP-relative target "
                        f"0x{tgt_vaddr:x} outside .rodata"
                    )
                    raise ValueError(msg)
                ro_file_off = ro_off + (tgt_vaddr - ro_addr)
                val64 = struct.unpack_from("<Q", elf_bytes, ro_file_off)[0]
                rodata[span.offset] = val64
                ro_names = by_addr.get(tgt_vaddr, [])
                rodata_symbols[span.offset] = (
                    ro_names[0] if ro_names else f"0x{tgt_vaddr:x}"
                )

        routine = Amd64RoutineBinary(
            symbol=go_sym,
            raw_bytes=raw,
            targets=targets,
            rodata=rodata,
            rodata_symbols=rodata_symbols,
        )
        normalize_amd64_routine(routine)
        out[go_sym] = routine
    return out


def format_lean_routine(
    routine: Amd64RoutineBinary, lean_prefix: str, chunk_size: int = 16
) -> str:
    """Render Lean 4 chunked UInt8 array, rodata map, and target map."""
    raw = routine.raw_bytes
    spans = decode_amd64_spans(raw)
    lines: list[str] = [
        (
            f"/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes "
            f"({len(raw)} bytes, {len(spans)} insns) for `{routine.symbol}`. -/"
        ),
        f"def {lean_prefix}ByteChunks : List (List UInt8) := [",
    ]
    chunks = [raw[i : i + chunk_size] for i in range(0, len(raw), chunk_size)]
    for idx, chunk in enumerate(chunks):
        comma = "," if idx + 1 < len(chunks) else ""
        hex_list = ", ".join(f"0x{b:02x}" for b in chunk)
        lines.append(f"  [{hex_list}]{comma}")
    lines.append("]")
    lines.append(
        f"def {lean_prefix}Bytes : Array UInt8 := ⟨{lean_prefix}ByteChunks.flatten⟩"
    )

    if not routine.targets:
        lines.append(f"def {lean_prefix}Targets (_ : Nat) : Option String := none")
    else:
        grouped: dict[str, list[int]] = {}
        for off in sorted(routine.targets):
            grouped.setdefault(routine.targets[off], []).append(off)
        lines.append(f"def {lean_prefix}Targets (off : Nat) : Option String :=")
        lines.append("  match off with")
        for sym, offsets in grouped.items():
            pat = " | ".join(f"0x{o:x}" for o in offsets)
            lines.append(f'  | {pat} => some "{sym}"')
        lines.append("  | _ => none")

    if not routine.rodata:
        lines.append(f"def {lean_prefix}Rodata (_ : Nat) : Option (BitVec 64) := none")
    else:
        grouped_ro: dict[int, list[int]] = {}
        for off in sorted(routine.rodata):
            grouped_ro.setdefault(routine.rodata[off], []).append(off)
        lines.append(f"def {lean_prefix}Rodata (off : Nat) : Option (BitVec 64) :=")
        lines.append("  match off with")
        for val64, offsets in grouped_ro.items():
            pat = " | ".join(f"0x{o:x}" for o in offsets)
            lines.append(f"  | {pat} => some 0x{val64:016x}#64")
        lines.append("  | _ => none")

    return "\n".join(lines)


def parse_lean_track5e(lean_path: Path) -> dict[str, Amd64RoutineBinary]:
    """Parse embedded byte arrays, targets, and rodata maps from Track5E.lean."""
    content = lean_path.read_text(encoding="utf-8")
    parsed_bytes: dict[str, bytes] = {}
    for match in RE_LEAN_BYTE_CHUNKS.finditer(content):
        prefix = match.group(1)
        bs = bytes(int(tok, 16) for tok in RE_HEX_BYTE.findall(match.group(2)))
        parsed_bytes[prefix] = bs

    routines: dict[str, Amd64RoutineBinary] = {}
    for prefix, raw in parsed_bytes.items():
        targets: dict[int, str] = {}
        tgt_pat = re.compile(
            rf"def\s+{re.escape(prefix)}Targets\s*\([^)]*\)\s*:\s*Option\s+String\s*:=\s*"
            r"match\s+\w+\s+with(.*?)\|\s*_\s*=>\s*none",
            re.DOTALL,
        )
        m_tgt = tgt_pat.search(content)
        if m_tgt is not None:
            for arm in re.finditer(
                r"\|\s*([0-9a-fA-Fx|\s]+)=>\s*some\s+\"([^\"]+)\"", m_tgt.group(1)
            ):
                sym = arm.group(2)
                for tok in re.findall(r"0x[0-9a-fA-F]+|\d+", arm.group(1)):
                    targets[int(tok, 0)] = sym

        rodata: dict[int, int] = {}
        ro_pat = re.compile(
            rf"def\s+{re.escape(prefix)}Rodata\s*\([^)]*\)\s*:\s*Option\s*\(BitVec\s+64\)\s*:=\s*"
            r"match\s+\w+\s+with(.*?)\|\s*_\s*=>\s*none",
            re.DOTALL,
        )
        m_ro = ro_pat.search(content)
        if m_ro is not None:
            for arm in re.finditer(
                r"\|\s*([0-9a-fA-Fx|\s]+)=>\s*some\s+(0x[0-9a-fA-F]+)#64",
                m_ro.group(1),
            ):
                val64 = int(arm.group(2), 16)
                for tok in re.findall(r"0x[0-9a-fA-F]+|\d+", arm.group(1)):
                    rodata[int(tok, 0)] = val64

        routines[prefix] = Amd64RoutineBinary(
            symbol=prefix,
            raw_bytes=raw,
            targets=targets,
            rodata=rodata,
            rodata_symbols={},
        )
    return routines


def verify_track5e(repo_root: Path, lean_override: Path | None = None) -> None:
    """Verify embedded Track5E.lean amd64 byte tables against a fresh ELF build."""
    module_path = read_module_path(repo_root)
    lean_path = (
        lean_override.resolve()
        if lean_override is not None
        else repo_root / "formal" / "GoblinFormal" / "Track5E.lean"
    )
    if not lean_path.is_file():
        msg = f"missing required Lean file: {lean_path}"
        raise FileNotFoundError(msg)
    lean_routines = parse_lean_track5e(lean_path)

    with tempfile.TemporaryDirectory(prefix="track5e_amd64_") as tmp_dir:
        elf_path = Path(tmp_dir) / "simd_amd64.elf"
        build_amd64_simd_test_elf(repo_root, elf_path, "./internal/simd")
        live_map = extract_amd64_routines(
            repo_root, elf_path, [s.go_symbol for s in VERIFY_SPECS], module_path
        )

    for spec in VERIFY_SPECS:
        if spec.lean_prefix not in lean_routines:
            msg = f"Track5E.lean missing definition '{spec.lean_prefix}ByteChunks'"
            raise KeyError(msg)
        expected = lean_routines[spec.lean_prefix]
        live = live_map[spec.go_symbol]

        if len(expected.raw_bytes) != len(live.raw_bytes):
            msg = (
                f"Track5E::{spec.lean_prefix} ({spec.go_symbol}): byte count mismatch: "
                f"Lean={len(expected.raw_bytes)}, ELF={len(live.raw_bytes)}"
            )
            raise ValueError(msg)
        if expected.targets != live.targets:
            msg = (
                f"Track5E::{spec.lean_prefix} ({spec.go_symbol}): CALL target mismatch:\n"
                f"  Lean={expected.targets}\n"
                f"  ELF ={live.targets}"
            )
            raise ValueError(msg)
        if expected.rodata != live.rodata:
            msg = (
                f"Track5E::{spec.lean_prefix} ({spec.go_symbol}): rodata mismatch:\n"
                f"  Lean={expected.rodata}\n"
                f"  ELF ={live.rodata}"
            )
            raise ValueError(msg)

        exp_norm = normalize_amd64_routine(expected)
        live_norm = normalize_amd64_routine(live)
        if exp_norm != live_norm:
            for idx, (lhs, rhs) in enumerate(zip(exp_norm, live_norm, strict=True)):
                if lhs != rhs:
                    msg = (
                        f"Track5E::{spec.lean_prefix} ({spec.go_symbol}) mismatch "
                        f"at byte offset 0x{idx:x}: Lean=0x{lhs:02x}, ELF=0x{rhs:02x}"
                    )
                    raise ValueError(msg)

        spans = decode_amd64_spans(live.raw_bytes)
        vzu_count = sum(1 for s in spans if s.kind == "vzeroupper")
        print(
            f"VERIFIED Track5E.lean::{spec.lean_prefix} <- {spec.go_symbol} "
            f"({len(live.raw_bytes)} bytes, {len(spans)} insns, "
            f"{vzu_count} vzeroupper, {len(live.targets)} ext calls, "
            f"{len(live.rodata)} rodata)"
        )

    print(
        f"ALL {len(VERIFY_SPECS)} Track 5E amd64 AVX2/AVX-512 ELF kernels verified cleanly."
    )


def build_arg_parser() -> argparse.ArgumentParser:
    """Build the CLI argument parser."""
    parser = argparse.ArgumentParser(
        description=(
            "Extract GOARCH=amd64 GOEXPERIMENT=simd ELF bytes for internal/simd "
            "kernels and verify Track5E.lean."
        )
    )
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=None,
        help="Path to repository root (defaults to parent of scripts/).",
    )
    parser.add_argument(
        "--lean-file",
        type=Path,
        default=None,
        help="Optional override path for Track5E.lean during --verify.",
    )
    parser.add_argument(
        "--verify",
        action="store_true",
        help="Verify Track5E.lean embedded bytes and relocations against fresh ELF.",
    )
    parser.add_argument(
        "--emit",
        action="store_true",
        help="Emit Lean 4 byte chunk definitions for all Track 5E kernels.",
    )
    parser.add_argument(
        "--symbol",
        type=str,
        default=None,
        help="Filter --emit or --json to a single short symbol name.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit extracted kernel metadata and byte arrays as JSON.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    """CLI entry point."""
    parser = build_arg_parser()
    args = parser.parse_args(argv)
    repo_root = resolve_repo_root(args.repo_root)

    if args.verify:
        verify_track5e(repo_root, args.lean_file)
        return 0

    if not args.emit and not args.json and args.symbol is None:
        parser.print_help()
        return 0

    module_path = read_module_path(repo_root)
    specs = (
        [s for s in VERIFY_SPECS if args.symbol in (s.go_symbol, s.lean_prefix)]
        if args.symbol is not None
        else list(VERIFY_SPECS)
    )
    if not specs:
        print(f"ERROR: unknown symbol '{args.symbol}'", file=sys.stderr)
        return 1

    with tempfile.TemporaryDirectory(prefix="track5e_emit_") as tmp_dir:
        elf_path = Path(tmp_dir) / "simd_amd64.elf"
        build_amd64_simd_test_elf(repo_root, elf_path, "./internal/simd")
        live_map = extract_amd64_routines(
            repo_root, elf_path, [s.go_symbol for s in specs], module_path
        )

    if args.json:
        payload = [
            {
                "lean_prefix": s.lean_prefix,
                "symbol": s.go_symbol,
                "bytes": list(live_map[s.go_symbol].raw_bytes),
                "targets": [
                    {"offset": off, "symbol": sym}
                    for off, sym in sorted(live_map[s.go_symbol].targets.items())
                ],
                "rodata": [
                    {
                        "offset": off,
                        "value": f"0x{val:016x}",
                        "symbol": live_map[s.go_symbol].rodata_symbols.get(off, ""),
                    }
                    for off, val in sorted(live_map[s.go_symbol].rodata.items())
                ],
            }
            for s in specs
        ]
        print(json.dumps(payload, indent=2))
        return 0

    rendered = [
        format_lean_routine(live_map[s.go_symbol], s.lean_prefix) for s in specs
    ]
    print("\n\n".join(rendered))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
