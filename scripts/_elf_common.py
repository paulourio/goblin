"""Shared ELF64, Go binary-build, and Lean table utilities for Track 5 extractors.

Both `extract_elf_track5.py` and `extract_amd64_track5e.py` import this module
after setting `sys.dont_write_bytecode = True` so verification runs never leave
`__pycache__` artifacts in the repository working tree.
"""

from __future__ import annotations

import os
import re
import struct
import subprocess
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import TypeVar

_V = TypeVar("_V")


def resolve_repo_root(explicit_root: Path | None) -> Path:
    """Return the repository root containing go.mod."""
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
    """Read the Go module path from go.mod."""
    for line in (repo_root / "go.mod").read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped.startswith("module "):
            return stripped.split()[1].strip()
    msg = "module directive missing in go.mod"
    raise ValueError(msg)


def strip_module_prefixes(name: str, module_path: str) -> str:
    """Remove Go module slash and dot prefixes from linker symbol names."""
    return name.replace(f"{module_path}/", "").replace(f"{module_path}.", "")


def cstr(buf: bytes, offset: int) -> str:
    """Read a NUL-terminated UTF-8 string."""
    end = buf.find(b"\x00", offset)
    if end < 0:
        msg = f"unterminated NUL string at offset {offset}"
        raise ValueError(msg)
    return buf[offset:end].decode("utf-8", errors="replace")


def sign_extend(value: int, bits: int) -> int:
    """Sign-extend an unsigned bitfield of width `bits` to a Python int."""
    mask = (1 << bits) - 1
    masked = value & mask
    sign_bit = 1 << (bits - 1)
    return masked - (1 << bits) if (masked & sign_bit) else masked


def build_go_test_elf(
    repo_root: Path,
    out_elf: Path,
    go_pkg: str,
    env_overrides: Mapping[str, str],
    extra_flags: Sequence[str] = (),
) -> None:
    """Compile a deterministic Go test ELF binary for `go_pkg` with `-trimpath`."""
    env = os.environ.copy()
    env.update(env_overrides)
    cmd = ["go", "test", "-c", "-trimpath", *extra_flags, "-o", str(out_elf), go_pkg]
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


def parse_elf64_sections(data: bytes) -> dict[str, tuple[int, int, int]]:
    """Parse little-endian ELF64 section headers into name -> (vaddr, offset, size)."""
    if len(data) < 64 or data[:4] != b"\x7fELF" or data[4] != 2 or data[5] != 1:
        msg = "expected little-endian ELF64 binary"
        raise ValueError(msg)
    e_shoff = struct.unpack_from("<Q", data, 40)[0]
    e_shentsize, e_shnum, e_shstrndx = struct.unpack_from("<HHH", data, 58)
    raw_secs: list[tuple[int, int, int, int]] = []
    for i in range(e_shnum):
        off = e_shoff + i * e_shentsize
        sh_name, _sh_type, _sh_flags, sh_addr, sh_offset, sh_size = struct.unpack_from(
            "<IIQQQQ", data, off
        )
        raw_secs.append((sh_name, sh_addr, sh_offset, sh_size))
    _, _, str_off, str_size = raw_secs[e_shstrndx]
    shstr = data[str_off : str_off + str_size]
    return {
        cstr(shstr, sh_name): (sh_addr, sh_offset, sh_size)
        for sh_name, sh_addr, sh_offset, sh_size in raw_secs
    }


def format_lean_chunks(
    items: Sequence[int], chunk_size: int, hex_width: int
) -> list[str]:
    """Format an integer sequence as indented Lean chunk list lines."""
    chunks = [items[i : i + chunk_size] for i in range(0, len(items), chunk_size)]
    lines: list[str] = []
    for idx, chunk in enumerate(chunks):
        comma = "," if idx + 1 < len(chunks) else ""
        hex_list = ", ".join(f"0x{item:0{hex_width}x}" for item in chunk)
        lines.append(f"  [{hex_list}]{comma}")
    return lines


def format_lean_match_map(
    def_name: str,
    arg_name: str,
    ret_type: str,
    entries: Mapping[int, _V],
    key_fmt: str,
    val_fmt: str,
) -> list[str]:
    """Format an integer-keyed Lean `Option` lookup function via `match`."""
    if not entries:
        return [f"def {def_name} (_ : Nat) : Option {ret_type} := none"]
    grouped: dict[_V, list[int]] = {}
    for key in sorted(entries):
        grouped.setdefault(entries[key], []).append(key)
    lines = [
        f"def {def_name} ({arg_name} : Nat) : Option {ret_type} :=",
        f"  match {arg_name} with",
    ]
    for val, keys in grouped.items():
        pat = " | ".join(key_fmt.format(k) for k in keys)
        lines.append(f"  | {pat} => some {val_fmt.format(val)}")
    lines.append("  | _ => none")
    return lines


def parse_lean_match_body(content: str, def_name: str, ret_type_re: str) -> str | None:
    """Return the match body of `def <def_name> (...) : Option <ret_type> := match ...`."""
    pat = re.compile(
        rf"def\s+{re.escape(def_name)}\s*\([^)]*\)\s*:\s*Option\s+{ret_type_re}\s*:=\s*"
        r"match\s+\w+\s+with(.*?)\|\s*_\s*=>\s*none",
        re.DOTALL,
    )
    match = pat.search(content)
    return match.group(1) if match is not None else None
