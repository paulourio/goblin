#!/usr/bin/env python3
"""Automated GOARCH=arm64 ELF extractor and Lean 4 verifier for Track 5.

Compiles CGO_ENABLED=0 Linux arm64 Go test binaries, disassembles target
symbols via `go tool objdump`, extracts 32-bit little-endian AArch64
instruction words and external relocation targets, emits 16-word chunked
Lean 4 definitions, and verifies embedded instruction tables across
Track5A.lean, Track5B.lean, and Track5D.lean.
"""

from __future__ import annotations

import argparse
import json
import re
import struct
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

# Disable bytecode emission before importing local modules (pre-commit forbids __pycache__).
sys.dont_write_bytecode = True

from _elf_common import (
    build_go_test_elf,
    format_lean_chunks,
    format_lean_match_map,
    parse_elf64_sections,
    parse_lean_match_body,
    read_module_path,
    resolve_repo_root,
    sign_extend,
    strip_module_prefixes,
)

RE_OBJDUMP_TEXT = re.compile(r"^TEXT\s+(\S+)\(SB\)")
RE_OBJDUMP_INSN = re.compile(r"^\s*\S+\s+0x([0-9a-fA-F]+)\s+([0-9a-fA-F]{8})\s+(.*)$")
RE_OBJDUMP_CALL_TARGET = re.compile(r"\b(?:CALL|JMP)\s+([^\s+(]+)(?:\+\d+)?\(SB\)")

RE_LEAN_ARRAY_WORDS = re.compile(
    r"def\s+(\w+)Words\s*:\s*Array\s+UInt32\s*:=\s*#\[(.*?)\]",
    re.DOTALL,
)
RE_LEAN_CHUNK_WORDS = re.compile(
    r"def\s+(\w+)Chunks\s*:\s*List\s*\(List\s+UInt32\)\s*:=\s*\[(.*?)\]\s*\n"
    r"def\s+\1Words",
    re.DOTALL,
)
RE_HEX_WORD = re.compile(r"0x[0-9a-fA-F]+")

# Fail-closed allow-list matching Track5B.allowedZeroGCSyms.
ALLOWED_ZEROGC_SYMS: frozenset[str] = frozenset(
    {
        "runtime.panicBounds",
        "runtime.panicdivide",
        "runtime.morestack_noctxt.abi0",
        "runtime.memclrNoHeapPointers",
        "runtime.selectnbrecv",
        "math.Float64bits",
        "math.IsInf",
        "math.Nextafter",
        "internal/simd.InitReducedRow64",
        "internal/simd.ScanNearTightMatrix64Scalar",
        "internal/simd.ScanNearTightRow64Scalar",
        "assign.twoSumAssign",
        "assign.exactReducedCost",
        "assign.refillQueueIndirectScalar",
        "assign.relaxRowIndirectScalar",
        "assign.scanCancelRowsFallback",
        "assign.buildCancelEdgesFallback",
        "assign.projectCancelViolFallback",
    }
)

# Internal fallback callees invoked from Track 5D hasPotentialCancellation (A4).
FALLBACK_CALLEES: tuple[str, ...] = (
    "assign.scanCancelRowsFallback",
    "assign.buildCancelEdgesFallback",
    "assign.projectCancelViolFallback",
    "internal/simd.ScanNearTightMatrix64Scalar",
    "internal/simd.ScanNearTightRow64Scalar",
)

# Expected 64-bit .rodata constants for every ADRP+LDR pair across verified routines (A15).
EXPECTED_RODATA_CONSTANTS: dict[str, dict[int, int]] = {
    "hasInfeasibleIntegerEqRow": {
        25: 0x3E7AD7F29ABCAF48,  # 1e-7
        43: 0x3E7AD7F29ABCAF48,  # 1e-7
        70: 0x41CDCD6500000000,  # 1e9
        79: 0x41CDCD6500000000,  # 1e9
        82: 0x41CDCD6500000000,  # 1e9
        92: 0x43D0000000000000,  # 2^62
        96: 0x3EB0C6F7A0B5ED8D,  # 1e-6
        112: 0x41CDCD6500000000,  # 1e9
        115: 0x41CDCD6500000000,  # 1e9
        118: 0x41CDCD6500000000,  # 1e9
        120: 0x3E7AD7F29ABCAF48,  # 1e-7
    },
    "augmentRowScalar": {
        62: 0x7FF0000000000000,  # +Inf
    },
    "initReducedRow64": {
        19: 0x7FF0000000000000,  # +Inf
        21: 0x7FF0000000000000,  # +Inf
        23: 0x7FF0000000000000,  # +Inf
        25: 0x7FF0000000000000,  # +Inf
        29: 0x7FF0000000000000,  # +Inf
    },
    "refillQueueIndirectScalar": {
        134: 0x7FEFFFFFFFFFFFFF,  # +MaxFloat64
    },
    "clampFinite": {
        1: 0x7FEFFFFFFFFFFFFF,  # +MaxFloat64
        5: 0xFFEFFFFFFFFFFFFF,  # -MaxFloat64
    },
    "exactReducedCost": {
        33: 0x7FEFFFFFFFFFFFFF,  # +MaxFloat64
    },
    "hasPotentialCancellation": {
        32: 0x7FEFFFFFFFFFFFFF,
        36: 0x7F70000000000000,
        131: 0x3CF0800000000000,
        168: 0x7FEFFFFFFFFFFFFF,
        172: 0x7F70000000000000,
        213: 0x4130000000000000,
        219: 0x3CE0000000000000,
        253: 0x3FF0000000000010,
        302: 0x4050000000000000,
        305: 0x0000000000000001,  # SmallestNonzeroFloat64
        309: 0x7FEFFFFFFFFFFFFF,
        337: 0x3CF0000000000000,
        340: 0x0000000000000008,  # 8 * SmallestNonzeroFloat64
        342: 0x3FF0000000000020,
        348: 0x7FB0000000000000,
        510: 0x3CF0000000000000,
        515: 0x0000000000000001,  # SmallestNonzeroFloat64
        519: 0x7FEFFFFFFFFFFFFF,
        531: 0x0000000000000004,  # 4 * SmallestNonzeroFloat64
        533: 0x3FF0000000000010,
        774: 0x7FEFFFFFFFFFFFFF,
        778: 0x7FEFFFFFFFFFFFFF,
        781: 0xFFEFFFFFFFFFFFFF,
        785: 0xFFEFFFFFFFFFFFFF,
        790: 0x7FEFFFFFFFFFFFFF,
        846: 0xFFF0000000000000,  # -Inf
        849: 0x7FEFFFFFFFFFFFFF,
        853: 0x7FEFFFFFFFFFFFFF,
        856: 0xFFEFFFFFFFFFFFFF,
        860: 0xFFEFFFFFFFFFFFFF,
        1066: 0x3C30000000000000,
        1071: 0x3FF0000000000010,
        1075: 0x3FF0000000000010,
        1077: 0x3C30000000000000,
        1146: 0x3CE0000000000000,
        1318: 0x7FF0000000000000,  # +Inf
    },
}

# Expected static symbol names for every ADRP+ADD pair across verified routines (A15).
EXPECTED_DATA_SYMBOLS: dict[str, dict[int, str]] = {
    "augmentRowScalar": {
        75: "assign.ErrInfeasible",
        222: "assign.errContextCanceled",
    },
    "refillQueueIndirectScalar": {
        29: "assign.ErrInfeasible",
        171: "assign.ErrInfeasible",
    },
}


@dataclass(frozen=True)
class RoutineBinary:
    """Extracted AArch64 instruction words, external branch targets, and addresses."""

    symbol: str
    words: tuple[int, ...]
    targets: dict[int, str]
    addrs: tuple[int, ...] = ()


@dataclass(frozen=True)
class VerifySpec:
    """Mapping between a Lean definition prefix and its Go ELF symbol."""

    track_file: str
    lean_prefix: str
    go_pkg: str
    go_symbol: str
    noinline: bool


VERIFY_SPECS: tuple[VerifySpec, ...] = (
    VerifySpec("Track5A.lean", "splitmix64", ".", "presolveSplitmix64", True),
    VerifySpec("Track5A.lean", "canonicalFloatBits", ".", "canonicalFloatBits", True),
    VerifySpec("Track5A.lean", "divCeilInt64", ".", "divCeilInt64", True),
    VerifySpec("Track5A.lean", "gcdInt64", ".", "gcdInt64", True),
    VerifySpec(
        "Track5B.lean",
        "hasInfeasibleIntegerEqRow",
        ".",
        "hasInfeasibleIntegerEqRow",
        False,
    ),
    VerifySpec(
        "Track5B.lean", "augmentRowScalar", ".", "assign.augmentRowScalar", False
    ),
    VerifySpec(
        "Track5B.lean",
        "initReducedRow64",
        ".",
        "internal/simd.InitReducedRow64",
        False,
    ),
    VerifySpec(
        "Track5B.lean",
        "refillQueueIndirectScalar",
        ".",
        "assign.refillQueueIndirectScalar",
        False,
    ),
    VerifySpec(
        "Track5B.lean",
        "relaxRowIndirectScalar",
        ".",
        "assign.relaxRowIndirectScalar",
        False,
    ),
    VerifySpec("Track5D.lean", "twoSumAssign", "./assign", "assign.twoSumAssign", True),
    VerifySpec("Track5D.lean", "clampFinite", "./assign", "assign.clampFinite", True),
    VerifySpec(
        "Track5D.lean",
        "exactReducedCostNoinline",
        "./assign",
        "assign.exactReducedCost",
        True,
    ),
    VerifySpec(
        "Track5D.lean",
        "exactReducedCost",
        "./assign",
        "assign.exactReducedCost",
        False,
    ),
    VerifySpec(
        "Track5D.lean",
        "hasPotentialCancellation",
        "./assign",
        "assign.hasPotentialCancellation",
        False,
    ),
)


def is_supported_arm64_word(word: int) -> bool:
    """Return True when `word` decodes cleanly under Track5A/Track5D ASTs."""
    is64 = ((word >> 31) & 1) == 1
    if word == 0xD503201F or (word & 0xFFFFFC1F) == 0xD65F0000:
        return True
    if (word & 0x7C000000) == 0x14000000 or (word & 0xFF000010) == 0x54000000:
        return True
    if (word & 0x7E000000) in (0x34000000, 0x36000000):
        return True
    if (word & 0x9F000000) == 0x90000000:
        return True
    if (word & 0x1F800000) == 0x12800000:
        return ((word >> 29) & 0x3) in (0, 2, 3)
    if (word & 0x1F800000) in (0x11000000, 0x12000000):
        return True
    if (word & 0x1F800000) == 0x13000000:
        return ((word >> 29) & 0x3) in (0, 1, 2)
    if (word & 0x1F000000) == 0x0A000000 or (word & 0x1F200000) == 0x0B000000:
        return True
    if (word & 0x1FE00000) == 0x0B200000:
        return True
    if (word & 0x1FE00800) == 0x1A800000:
        return ((word >> 30) & 1) == 0 and ((word >> 29) & 1) == 0
    if (word & 0x7FE00000) == 0x1AC00000:
        return ((word >> 10) & 0x3F) in (2, 3)
    if (word & 0x7FE00000) == 0x1B000000:
        return True
    if (word & 0x3A000000) == 0x28000000:
        opc = (word >> 30) & 0x3
        vec = (word >> 26) & 1
        idx_mode = (word >> 23) & 0x3
        return idx_mode in (1, 2, 3) and (
            (vec == 1 and opc == 1) or (vec == 0 and opc == 2)
        )
    if (word & 0x3B000000) == 0x39000000:
        size = (word >> 30) & 0x3
        vec = (word >> 26) & 1
        opc = (word >> 22) & 0x3
        return opc in (0, 1) and (
            (vec == 1 and size == 3) or (vec == 0 and size in (0, 3))
        )
    if (word & 0x3B200000) == 0x38000000 and ((word >> 10) & 0x3) in (0, 1, 3):
        size = (word >> 30) & 0x3
        vec = (word >> 26) & 1
        opc = (word >> 22) & 0x3
        return vec == 0 and size == 3 and opc in (0, 1)
    if (word & 0x3B200C00) == 0x38200800:
        size = (word >> 30) & 0x3
        vec = (word >> 26) & 1
        opc = (word >> 22) & 0x3
        if vec == 1 and size == 3 and opc in (0, 1):
            return True
        if vec == 0 and size == 3 and opc in (0, 1):
            return True
        return vec == 0 and size == 0 and opc in (0, 1, 2)
    if (word & 0xFFE0FC17) == 0x1E602000 or (word & 0xFFE07C00) == 0x1E604000:
        return True
    if (word & 0xFFE00C00) == 0x1E600800:
        return True
    if (word & 0x7FE0FC00) == 0x1E600000:
        rmode = (word >> 19) & 0x3
        opcode = (word >> 16) & 0x7
        if is64 and rmode == 3 and opcode == 0:
            return True
        return is64 and rmode == 0 and opcode in (2, 6, 7)
    # Track5D scalar binary64 extensions: FMADDD/FMSUBD/FNMADDD/FNMSUBD and FMOVD $imm8.
    if (word & 0xFFC00000) == 0x1F400000:
        return True
    return (word & 0xFFE01FE0) == 0x1E601000


def normalize_arm64_routine(
    routine: RoutineBinary,
) -> list[tuple[int | str | bool, ...]]:
    """Canonicalize PC-relative ADRP and external branch relocations.

    Mirrors `GoblinFormal.Track5A.normalizeArm64Prog` and
    `GoblinFormal.Track5D.normalizeArm64ProgD`: intra-function instructions
    must match bit-for-bit, while external `B`/`BL` relocations and `ADRP`
    page/offset pairs normalize to symbol and register forms.
    """
    words = routine.words
    n_words = len(words)
    out: list[tuple[int | str | bool, ...]] = []
    prev_adrp_rd: int | None = None

    for idx, word in enumerate(words):
        if not is_supported_arm64_word(word):
            msg = (
                f"{routine.symbol}[{idx}]: unsupported ARM64 instruction "
                f"word 0x{word:08x}"
            )
            raise ValueError(msg)

        rd = word & 0x1F
        rn = (word >> 5) & 0x1F
        is64 = ((word >> 31) & 1) == 1
        cur_adrp_rd: int | None = None

        if (word & 0x7C000000) == 0x14000000:
            imm26 = sign_extend(word & 0x03FFFFFF, 26)
            tgt = idx + imm26
            op_tag = "bl" if is64 else "b"
            if 0 <= tgt < n_words:
                out.append(("branch_rel", op_tag, imm26))
            else:
                sym = routine.targets.get(idx)
                if sym is None:
                    msg = f"{routine.symbol}[{idx}]: missing external {op_tag} target"
                    raise ValueError(msg)
                out.append((f"{op_tag}_ext", sym))
        elif (word & 0x9F000000) == 0x90000000:
            cur_adrp_rd = rd
            out.append(("adrp_norm", rd))
        elif (word & 0x3B000000) == 0x39000000:
            size = (word >> 30) & 0x3
            vec = (word >> 26) & 1
            opc = (word >> 22) & 0x3
            if size == 3 and opc == 1 and prev_adrp_rd == rn:
                out.append(("adrp_ldr64_norm", vec == 1, rd, rn))
            else:
                out.append(("word", word))
        elif (word & 0x1F800000) == 0x11000000:
            is_sub = ((word >> 30) & 1) == 1
            set_flags = ((word >> 29) & 1) == 1
            if is64 and not is_sub and not set_flags and prev_adrp_rd == rn:
                out.append(("adrp_add64_norm", rd, rn))
            else:
                out.append(("word", word))
        else:
            out.append(("word", word))

        prev_adrp_rd = cur_adrp_rd

    return out


def build_arm64_test_elf(
    repo_root: Path, out_elf: Path, go_pkg: str, noinline: bool
) -> None:
    """Compile a static Linux arm64 test ELF binary for `go_pkg` with `-trimpath`."""
    build_go_test_elf(
        repo_root,
        out_elf,
        go_pkg,
        {"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "arm64"},
        ("-gcflags=-l",) if noinline else (),
    )


def read_elf64_u64(
    data: bytes, sections: dict[str, tuple[int, int, int]], vaddr: int
) -> tuple[str, int]:
    """Read an 8-byte little-endian word at virtual address `vaddr` from the ELF."""
    for name, (sh_addr, sh_offset, sh_size) in sections.items():
        if sh_addr != 0 and sh_addr <= vaddr and vaddr + 8 <= sh_addr + sh_size:
            off = sh_offset + (vaddr - sh_addr)
            return name, struct.unpack_from("<Q", data, off)[0]
    msg = f"virtual address 0x{vaddr:x} not mapped in any ELF section"
    raise ValueError(msg)


def parse_elf_nm_symbols(
    repo_root: Path, elf_path: Path, module_path: str
) -> dict[int, list[str]]:
    """Map ELF virtual addresses to module-stripped symbol names via `go tool nm`."""
    proc = subprocess.run(
        ["go", "tool", "nm", str(elf_path)],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        msg = f"go tool nm failed on {elf_path}:\n{proc.stderr}"
        raise RuntimeError(msg)
    by_addr: dict[int, list[str]] = {}
    for line in proc.stdout.splitlines():
        parts = line.strip().split(None, 2)
        if len(parts) != 3:
            continue
        try:
            addr = int(parts[0], 16)
        except ValueError:
            continue
        by_addr.setdefault(addr, []).append(
            strip_module_prefixes(parts[2], module_path)
        )
    return by_addr


def verify_adrp_targets(
    lean_prefix: str,
    routine: RoutineBinary,
    elf_bytes: bytes,
    sections: dict[str, tuple[int, int, int]],
    nm_by_addr: dict[int, list[str]],
) -> tuple[int, int]:
    """Verify every ADRP+LDR .rodata constant and ADRP+ADD symbol target (A15)."""
    if len(routine.addrs) != len(routine.words):
        msg = f"{routine.symbol}: missing instruction addresses for ADRP verification"
        raise ValueError(msg)

    actual_rodata: dict[int, int] = {}
    actual_data_syms: dict[int, list[str]] = {}

    for idx in range(1, len(routine.words)):
        w_prev = routine.words[idx - 1]
        if (w_prev & 0x9F000000) != 0x90000000:
            continue
        rd_prev = w_prev & 0x1F
        immlo = (w_prev >> 29) & 0x3
        immhi = (w_prev >> 5) & 0x7FFFF
        imm_pages = sign_extend((immhi << 2) | immlo, 21)
        page = (routine.addrs[idx - 1] & ~0xFFF) + (imm_pages << 12)

        w_cur = routine.words[idx]
        rn_cur = (w_cur >> 5) & 0x1F
        if rn_cur != rd_prev:
            continue

        if (w_cur & 0x3B000000) == 0x39000000:
            size = (w_cur >> 30) & 0x3
            opc = (w_cur >> 22) & 0x3
            if size == 3 and opc == 1:
                imm12 = ((w_cur >> 10) & 0xFFF) << 3
                vaddr = page + imm12
                sec_name, val = read_elf64_u64(elf_bytes, sections, vaddr)
                if sec_name != ".rodata":
                    msg = (
                        f"{lean_prefix}[{idx}]: ADRP+LDR resolves to section "
                        f"'{sec_name}' (expected '.rodata')"
                    )
                    raise ValueError(msg)
                # Cross-check Go's $f64.<hex> symbol name whenever present at vaddr.
                for sym_name in nm_by_addr.get(vaddr, ()):
                    if sym_name.startswith("$f64."):
                        sym_hex = int(sym_name[5:], 16)
                        if sym_hex != val:
                            msg = (
                                f"{lean_prefix}[{idx}]: .rodata value 0x{val:016x} "
                                f"disagrees with symbol {sym_name}"
                            )
                            raise ValueError(msg)
                actual_rodata[idx] = val
        elif (w_cur & 0x1F800000) == 0x11000000:
            is64 = ((w_cur >> 31) & 1) == 1
            is_sub = ((w_cur >> 30) & 1) == 1
            set_flags = ((w_cur >> 29) & 1) == 1
            if is64 and not is_sub and not set_flags:
                shift = (w_cur >> 22) & 0x3
                imm12 = ((w_cur >> 10) & 0xFFF) << (12 if shift == 1 else 0)
                vaddr = page + imm12
                actual_data_syms[idx] = nm_by_addr.get(vaddr, [])

    expected_rodata = EXPECTED_RODATA_CONSTANTS.get(lean_prefix, {})
    if actual_rodata != expected_rodata:
        msg = (
            f"{lean_prefix}: .rodata constant mismatch on ADRP+LDR pairs:\n"
            f"  expected={ {k: hex(v) for k, v in sorted(expected_rodata.items())} }\n"
            f"  actual  ={ {k: hex(v) for k, v in sorted(actual_rodata.items())} }"
        )
        raise ValueError(msg)

    expected_syms = EXPECTED_DATA_SYMBOLS.get(lean_prefix, {})
    if set(actual_data_syms) != set(expected_syms):
        msg = (
            f"{lean_prefix}: ADRP+ADD symbol indices mismatch: "
            f"expected={sorted(expected_syms)}, actual={sorted(actual_data_syms)}"
        )
        raise ValueError(msg)
    for idx, want_sym in expected_syms.items():
        got_syms = actual_data_syms[idx]
        if want_sym not in got_syms:
            msg = (
                f"{lean_prefix}[{idx}]: ADRP+ADD resolved to {got_syms}, "
                f"expected '{want_sym}'"
            )
            raise ValueError(msg)

    return len(actual_rodata), len(actual_data_syms)


def verify_fallback_callees(repo_root: Path, elf_path: Path, module_path: str) -> None:
    """Verify internal fallback callees have zero allocating runtime calls (A4)."""
    sym_regex = "|".join(
        re.escape(s.rsplit(".", maxsplit=1)[-1]) for s in FALLBACK_CALLEES
    )
    routines = extract_routines_objdump(repo_root, elf_path, sym_regex, module_path)
    for sym in FALLBACK_CALLEES:
        if sym not in routines:
            msg = f"fallback callee '{sym}' missing from arm64 ./assign ELF"
            raise KeyError(msg)
        routine = routines[sym]
        for idx, word in enumerate(routine.words):
            if not is_supported_arm64_word(word):
                msg = (
                    f"fallback callee {sym}[{idx}]: unsupported ARM64 word 0x{word:08x}"
                )
                raise ValueError(msg)
            if (word & 0xFFFFFC1F) == 0xD65F0000 and ((word >> 5) & 0x1F) != 30:
                msg = (
                    f"fallback callee {sym}[{idx}]: non-X30 RET instruction "
                    f"0x{word:08x}"
                )
                raise ValueError(msg)
        for idx, tgt in sorted(routine.targets.items()):
            if tgt not in ALLOWED_ZEROGC_SYMS:
                msg = (
                    f"fallback callee {sym}[{idx}]: forbidden/non-allowlisted "
                    f"external call target '{tgt}'"
                )
                raise ValueError(msg)
        print(
            f"VERIFIED fallback callee {sym} "
            f"({len(routine.words)} insns, {len(routine.targets)} ext relocs, ZeroGC allow-list clean)"
        )


def extract_routines_objdump(
    repo_root: Path,
    elf_path: Path,
    sym_pattern: str,
    module_path: str,
) -> dict[str, RoutineBinary]:
    """Disassemble matching symbols via `go tool objdump` into RoutineBinary records."""
    proc = subprocess.run(
        ["go", "tool", "objdump", "-s", sym_pattern, str(elf_path)],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        msg = f"go tool objdump failed on {elf_path}:\n{proc.stderr}"
        raise RuntimeError(msg)

    raw_blocks: dict[str, list[tuple[int, int, str]]] = {}
    cur_sym: str | None = None

    for line in proc.stdout.splitlines():
        m_text = RE_OBJDUMP_TEXT.match(line)
        if m_text is not None:
            raw_name = m_text.group(1)
            short_name = strip_module_prefixes(raw_name, module_path)
            cur_sym = short_name
            raw_blocks[cur_sym] = []
            continue
        if cur_sym is None:
            continue
        m_insn = RE_OBJDUMP_INSN.match(line)
        if m_insn is not None:
            addr = int(m_insn.group(1), 16)
            word = int(m_insn.group(2), 16)
            disasm = m_insn.group(3).strip()
            raw_blocks[cur_sym].append((addr, word, disasm))

    extracted: dict[str, RoutineBinary] = {}
    for sym_name, rows in raw_blocks.items():
        trimmed = list(rows)
        while trimmed and trimmed[-1][1] == 0:
            trimmed.pop()
        if not trimmed:
            continue
        addrs = tuple(a for a, _, _ in trimmed)
        words = tuple(w for _, w, _ in trimmed)
        n_words = len(words)
        targets: dict[int, str] = {}

        for idx, (_addr, word, disasm) in enumerate(trimmed):
            top = word & 0xFC000000
            if top not in (0x14000000, 0x94000000):
                continue
            imm26 = sign_extend(word & 0x03FFFFFF, 26)
            tgt_idx = idx + imm26
            if 0 <= tgt_idx < n_words:
                continue
            m_call = RE_OBJDUMP_CALL_TARGET.search(disasm)
            if m_call is None:
                msg = (
                    f"{sym_name}[{idx}]: unresolved external branch "
                    f"in objdump line '{disasm}'"
                )
                raise ValueError(msg)
            targets[idx] = strip_module_prefixes(m_call.group(1), module_path)

        extracted[sym_name] = RoutineBinary(
            symbol=sym_name,
            words=words,
            targets=targets,
            addrs=addrs,
        )

    return extracted


def default_lean_prefix(symbol: str) -> str:
    """Derive a camelCase Lean identifier prefix from a short Go symbol."""
    base = symbol.rsplit(".", maxsplit=1)[-1].rsplit("/", maxsplit=1)[-1]
    if not base:
        return "routine"
    return base[0].lower() + base[1:]


def format_lean_routine(
    routine: RoutineBinary,
    lean_prefix: str,
    chunk_size: int = 16,
    use_chunks: bool = True,
) -> str:
    """Render Lean 4 chunked word array and relocation target definitions."""
    words = routine.words
    lines: list[str] = []

    if use_chunks:
        lines.append(
            f"/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words "
            f"({len(words)} insns) for `{routine.symbol}` in {chunk_size}-word chunks. -/"
        )
        lines.append(f"def {lean_prefix}Chunks : List (List UInt32) := [")
        lines.extend(format_lean_chunks(words, chunk_size, 8))
        lines.append("]")
        lines.append(
            f"def {lean_prefix}Words : Array UInt32 := ⟨{lean_prefix}Chunks.flatten⟩"
        )
    else:
        lines.append(
            f"/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words "
            f"({len(words)} insns) for `{routine.symbol}`. -/"
        )
        lines.append(f"def {lean_prefix}Words : Array UInt32 := #[")
        for i in range(0, len(words), 4):
            row = words[i : i + 4]
            comma = "," if i + 4 < len(words) else ""
            hex_list = ", ".join(f"0x{w:08x}" for w in row)
            lines.append(f"  {hex_list}{comma}")
        lines.append("]")

    lines.extend(
        format_lean_match_map(
            f"{lean_prefix}Targets",
            "idx",
            "String",
            routine.targets,
            "{}",
            '"{}"',
        )
    )

    return "\n".join(lines)


def parse_lean_routines(lean_path: Path) -> dict[str, RoutineBinary]:
    """Parse embedded instruction word lists and target maps from a Lean file."""
    content = lean_path.read_text(encoding="utf-8")
    parsed_words: dict[str, tuple[int, ...]] = {}

    for match in RE_LEAN_ARRAY_WORDS.finditer(content):
        prefix = match.group(1)
        words = tuple(int(tok, 16) for tok in RE_HEX_WORD.findall(match.group(2)))
        parsed_words[prefix] = words

    for match in RE_LEAN_CHUNK_WORDS.finditer(content):
        prefix = match.group(1)
        words = tuple(int(tok, 16) for tok in RE_HEX_WORD.findall(match.group(2)))
        parsed_words[prefix] = words

    routines: dict[str, RoutineBinary] = {}
    for prefix, words in parsed_words.items():
        targets: dict[int, str] = {}
        body = parse_lean_match_body(content, f"{prefix}Targets", "String")
        if body is not None:
            for arm in re.finditer(r"\|\s*([0-9|\s]+)=>\s*some\s+\"([^\"]+)\"", body):
                sym = arm.group(2)
                for idx_tok in re.findall(r"\d+", arm.group(1)):
                    targets[int(idx_tok)] = sym
        else:
            if_pat = re.compile(
                rf"def\s+{re.escape(prefix)}Targets\s*\([^)]*\)\s*:\s*Option\s+String\s*:="
                r"(.*?)(?:\n\n|\nprivate\s|\ndef\s|\ntheorem\s|\n/--)",
                re.DOTALL,
            )
            m_if = if_pat.search(content)
            if m_if is not None:
                for arm in re.finditer(
                    r"==\s*(\d+)\s*then\s+some\s+\"([^\"]+)\"",
                    m_if.group(1),
                ):
                    targets[int(arm.group(1))] = arm.group(2)

        routines[prefix] = RoutineBinary(
            symbol=prefix,
            words=words,
            targets=targets,
        )

    return routines


def verify_all_tracks(repo_root: Path) -> None:
    """Verify Track5A.lean, Track5B.lean, and Track5D.lean against live ELF builds."""
    module_path = read_module_path(repo_root)
    formal_dir = repo_root / "formal" / "GoblinFormal"

    lean_tables: dict[str, dict[str, RoutineBinary]] = {}
    for track_name in ("Track5A.lean", "Track5B.lean", "Track5D.lean"):
        tpath = formal_dir / track_name
        if not tpath.is_file():
            msg = f"missing required Lean formalization file: {tpath}"
            raise FileNotFoundError(msg)
        lean_tables[track_name] = parse_lean_routines(tpath)

    build_groups: dict[tuple[str, bool], list[VerifySpec]] = {}
    for spec in VERIFY_SPECS:
        build_groups.setdefault((spec.go_pkg, spec.noinline), []).append(spec)

    live_extracted: dict[tuple[str, bool, str], RoutineBinary] = {}
    adrp_counts: dict[str, tuple[int, int]] = {}
    with tempfile.TemporaryDirectory(prefix="track5_elf_") as tmp_dir:
        tmp_path = Path(tmp_dir)
        for idx, ((go_pkg, noinline), specs) in enumerate(build_groups.items()):
            elf_path = tmp_path / f"bin_{idx}.elf"
            build_arm64_test_elf(repo_root, elf_path, go_pkg, noinline)
            short_tails = sorted(
                {s.go_symbol.rsplit(".", maxsplit=1)[-1] for s in specs}
            )
            sym_regex = "|".join(re.escape(tok) for tok in short_tails)
            routines = extract_routines_objdump(
                repo_root, elf_path, sym_regex, module_path
            )
            elf_bytes = elf_path.read_bytes()
            sections = parse_elf64_sections(elf_bytes)
            nm_by_addr = parse_elf_nm_symbols(repo_root, elf_path, module_path)
            for spec in specs:
                if spec.go_symbol not in routines:
                    msg = (
                        f"symbol '{spec.go_symbol}' missing from arm64 ELF "
                        f"for package '{go_pkg}' (noinline={noinline})"
                    )
                    raise KeyError(msg)
                live_routine = routines[spec.go_symbol]
                live_extracted[(go_pkg, noinline, spec.go_symbol)] = live_routine
                adrp_counts[spec.lean_prefix] = verify_adrp_targets(
                    spec.lean_prefix, live_routine, elf_bytes, sections, nm_by_addr
                )
            if go_pkg == "./assign" and not noinline:
                verify_fallback_callees(repo_root, elf_path, module_path)

    verified_count = 0
    for spec in VERIFY_SPECS:
        file_routines = lean_tables[spec.track_file]
        if spec.lean_prefix not in file_routines:
            msg = (
                f"{spec.track_file}: missing definition "
                f"'{spec.lean_prefix}Words' / '{spec.lean_prefix}Chunks'"
            )
            raise KeyError(msg)

        expected = file_routines[spec.lean_prefix]
        live = live_extracted[(spec.go_pkg, spec.noinline, spec.go_symbol)]

        if len(expected.words) != len(live.words):
            msg = (
                f"{spec.track_file}::{spec.lean_prefix} ({spec.go_symbol}): "
                f"word count mismatch: Lean={len(expected.words)}, ELF={len(live.words)}"
            )
            raise ValueError(msg)

        if expected.targets != live.targets:
            msg = (
                f"{spec.track_file}::{spec.lean_prefix} ({spec.go_symbol}): "
                f"relocation target mismatch:\n"
                f"  Lean={expected.targets}\n"
                f"  ELF ={live.targets}"
            )
            raise ValueError(msg)

        exp_norm = normalize_arm64_routine(expected)
        live_norm = normalize_arm64_routine(live)
        if exp_norm != live_norm:
            for i, (lhs, rhs) in enumerate(zip(exp_norm, live_norm, strict=True)):
                if lhs != rhs:
                    msg = (
                        f"{spec.track_file}::{spec.lean_prefix} ({spec.go_symbol}) "
                        f"mismatch at pcIdx={i}: Lean={lhs}, ELF={rhs}"
                    )
                    raise ValueError(msg)

        n_rodata, n_data = adrp_counts[spec.lean_prefix]
        verified_count += 1
        print(
            f"VERIFIED {spec.track_file}::{spec.lean_prefix} "
            f"<- {spec.go_symbol} ({len(live.words)} insns, "
            f"{len(live.targets)} ext relocs, {n_rodata} rodata consts, {n_data} data syms)"
        )

    print(f"ALL {verified_count} Track 5A/5B/5D arm64 ELF routines verified cleanly.")


def build_arg_parser() -> argparse.ArgumentParser:
    """Construct the command-line parser."""
    parser = argparse.ArgumentParser(
        description=(
            "Extract GOARCH=arm64 ELF instruction words via go tool objdump "
            "and verify Track5A/5B/5D Lean formalizations."
        )
    )
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=None,
        help="Path to the repository root (defaults to parent of scripts/).",
    )
    parser.add_argument(
        "--verify",
        action="store_true",
        help="Verify extracted words and targets against Track5A, Track5B, and Track5D.",
    )
    parser.add_argument(
        "--symbol",
        type=str,
        default=None,
        help="Short symbol name to extract (for example assign.exactReducedCost).",
    )
    parser.add_argument(
        "--package",
        type=str,
        default="./assign",
        help="Go package to compile with go test -c (default: ./assign).",
    )
    parser.add_argument(
        "--noinline",
        action="store_true",
        help="Compile the target Go package with -gcflags=-l.",
    )
    parser.add_argument(
        "--lean-prefix",
        type=str,
        default=None,
        help="Lean definition prefix when emitting chunks (defaults to symbol tail).",
    )
    parser.add_argument(
        "--flat",
        action="store_true",
        help="Emit a flat #[...] Array UInt32 instead of 16-word chunks.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit extracted symbol words and targets as JSON.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    """Entry point for CLI execution."""
    parser = build_arg_parser()
    args = parser.parse_args(argv)
    repo_root = resolve_repo_root(args.repo_root)

    if args.verify:
        verify_all_tracks(repo_root)
        return 0

    if args.symbol is None:
        parser.print_help()
        return 0

    module_path = read_module_path(repo_root)
    tail_tok = args.symbol.rsplit(".", maxsplit=1)[-1]
    with tempfile.TemporaryDirectory(prefix="track5_extract_") as tmp_dir:
        elf_path = Path(tmp_dir) / "target.elf"
        build_arm64_test_elf(repo_root, elf_path, args.package, args.noinline)
        routines = extract_routines_objdump(
            repo_root, elf_path, re.escape(tail_tok), module_path
        )

    if args.symbol not in routines:
        available = ", ".join(sorted(routines))
        print(
            f"ERROR: symbol '{args.symbol}' not found (matched: {available})",
            file=sys.stderr,
        )
        return 1

    routine = routines[args.symbol]
    normalize_arm64_routine(routine)

    if args.json:
        payload = {
            "symbol": routine.symbol,
            "words": list(routine.words),
            "targets": [
                {"idx": idx, "symbol": sym}
                for idx, sym in sorted(routine.targets.items())
            ],
        }
        print(json.dumps(payload, indent=2))
        return 0

    prefix = (
        args.lean_prefix
        if args.lean_prefix is not None
        else default_lean_prefix(routine.symbol)
    )
    print(
        format_lean_routine(
            routine,
            lean_prefix=prefix,
            chunk_size=16,
            use_chunks=not args.flat,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
