"""Shared workspace bootstrap, CBC library path, and provenance helpers for bench scripts.

All scripts in `bench/` set `sys.dont_write_bytecode = True` before importing this module
so execution never leaves `__pycache__` directories in the working tree.
"""

from __future__ import annotations

import glob
import os
import platform
import subprocess
import sys
from datetime import UTC, datetime
from typing import Any

GOBLIN_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
REPO_ROOT = os.path.abspath(os.path.join(GOBLIN_ROOT, ".."))
CBC_DIST_DIR = os.path.join(GOBLIN_ROOT, "third_party", "Cbc", "dist")


def bootstrap_venv(*, include_pulp: bool = False) -> None:
    """Add the sibling PuLP virtualenv site-packages (and optional vendored PuLP) to sys.path."""
    pattern = os.path.join(
        REPO_ROOT, "pulp", ".venv", "lib", "python*", "site-packages"
    )
    for cand_site in sorted(glob.glob(pattern)):
        if cand_site not in sys.path:
            sys.path.insert(0, cand_site)
    if include_pulp:
        vendored_pulp = os.path.join(REPO_ROOT, "pulp")
        if (
            os.path.isdir(os.path.join(vendored_pulp, "pulp"))
            and vendored_pulp not in sys.path
        ):
            sys.path.insert(0, vendored_pulp)


def prepend_cbc_lib_dir() -> None:
    """Prepend the vendored CBC shared library directory to LD_LIBRARY_PATH when present."""
    cbc_lib_dir = os.path.join(CBC_DIST_DIR, "lib")
    if os.path.isdir(cbc_lib_dir):
        ld = os.environ.get("LD_LIBRARY_PATH", "")
        if cbc_lib_dir not in ld.split(":"):
            os.environ["LD_LIBRARY_PATH"] = f"{cbc_lib_dir}:{ld}" if ld else cbc_lib_dir


def cpu_affinity() -> str:
    """Return the CPUs allowed for the current process compressed into range notation."""
    if not hasattr(os, "sched_getaffinity"):
        return "unknown"
    cpus = sorted(os.sched_getaffinity(0))
    ranges: list[str] = []
    start = prev = cpus[0]
    for cpu in [*cpus[1:], -1]:
        if cpu != prev + 1:
            ranges.append(str(start) if start == prev else f"{start}-{prev}")
            start = cpu
        prev = cpu
    return ",".join(ranges)


def repo_provenance() -> dict[str, Any]:
    """Collect git revision, dirty status, source fingerprint, kernel release, and UTC timestamp."""
    git_commit = "unknown"
    git_dirty = True
    try:
        git_commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=GOBLIN_ROOT, text=True, timeout=5
        ).strip()
        status = subprocess.check_output(
            ["git", "status", "--porcelain", "--", ".", ":(exclude)bench/results"],
            cwd=GOBLIN_ROOT,
            text=True,
            timeout=5,
        ).strip()
        git_dirty = len(status) > 0
    except (OSError, subprocess.SubprocessError):
        pass

    fingerprint = subprocess.check_output(
        ["go", "run", "./cmd/goblin-bench", "-print-fingerprint"],
        cwd=GOBLIN_ROOT,
        text=True,
        timeout=30,
    ).strip()

    return {
        "git_commit": git_commit,
        "git_dirty": git_dirty,
        "source_fingerprint": fingerprint,
        "uname_release": platform.uname().release,
        "measured_at": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
