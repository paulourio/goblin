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
import os
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

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


@dataclass(frozen=True)
class RoutineBinary:
    """Extracted AArch64 instruction words and external branch targets."""

    symbol: str
    words: tuple[int, ...]
    targets: dict[int, str]


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


def resolve_repo_root(explicit_root: Path | None) -> Path:
    """Locate the repository root containing go.mod and formal/lakefile.toml."""
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


def sign_extend(value: int, bits: int) -> int:
    """Sign-extend an unsigned bitfield of width `bits` to a Python int."""
    mask = (1 << bits) - 1
    masked = value & mask
    sign_bit = 1 << (bits - 1)
    return masked - (1 << bits) if (masked & sign_bit) else masked


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
    if (word & 0x5FE00000) == 0x1AC00000:
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
    # Track5D scalar binary64 extensions: FMADDD/FMSUBD and FMOVD $imm8.
    if (word & 0xFFE00000) == 0x1F400000:
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
    """Compile a static Linux arm64 test ELF binary for `go_pkg`."""
    env = os.environ.copy()
    env.update({"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "arm64"})
    cmd = ["go", "test", "-c"]
    if noinline:
        cmd.append("-gcflags=-l")
    cmd.extend(["-o", str(out_elf), go_pkg])
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

    raw_blocks: dict[str, list[tuple[int, str]]] = {}
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
            word = int(m_insn.group(2), 16)
            disasm = m_insn.group(3).strip()
            raw_blocks[cur_sym].append((word, disasm))

    extracted: dict[str, RoutineBinary] = {}
    for sym_name, rows in raw_blocks.items():
        trimmed = list(rows)
        while trimmed and trimmed[-1][0] == 0:
            trimmed.pop()
        if not trimmed:
            continue
        words = tuple(w for w, _ in trimmed)
        n_words = len(words)
        targets: dict[int, str] = {}

        for idx, (word, disasm) in enumerate(trimmed):
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
        chunks = [words[i : i + chunk_size] for i in range(0, len(words), chunk_size)]
        for c_idx, chunk in enumerate(chunks):
            comma = "," if c_idx + 1 < len(chunks) else ""
            hex_list = ", ".join(f"0x{w:08x}" for w in chunk)
            lines.append(f"  [{hex_list}]{comma}")
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

    if not routine.targets:
        lines.append(f"def {lean_prefix}Targets (_ : Nat) : Option String := none")
    else:
        grouped: dict[str, list[int]] = {}
        for idx in sorted(routine.targets):
            sym = routine.targets[idx]
            grouped.setdefault(sym, []).append(idx)
        lines.append(f"def {lean_prefix}Targets (idx : Nat) : Option String :=")
        lines.append("  match idx with")
        for sym, indices in grouped.items():
            pat = " | ".join(str(i) for i in indices)
            lines.append(f'  | {pat} => some "{sym}"')
        lines.append("  | _ => none")

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
        match_pat = re.compile(
            rf"def\s+{re.escape(prefix)}Targets\s*\([^)]*\)\s*:\s*Option\s+String\s*:=\s*"
            r"match\s+\w+\s+with(.*?)\|\s*_\s*=>\s*none",
            re.DOTALL,
        )
        m_match = match_pat.search(content)
        if m_match is not None:
            body = m_match.group(1)
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
            for spec in specs:
                if spec.go_symbol not in routines:
                    msg = (
                        f"symbol '{spec.go_symbol}' missing from arm64 ELF "
                        f"for package '{go_pkg}' (noinline={noinline})"
                    )
                    raise KeyError(msg)
                live_extracted[(go_pkg, noinline, spec.go_symbol)] = routines[
                    spec.go_symbol
                ]

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

        verified_count += 1
        print(
            f"VERIFIED {spec.track_file}::{spec.lean_prefix} "
            f"<- {spec.go_symbol} ({len(live.words)} insns, "
            f"{len(live.targets)} ext relocs)"
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
