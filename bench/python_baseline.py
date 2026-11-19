#!/usr/bin/env python3
"""
Python PuLP and COIN-OR CBC reference harness.

Each worker times JSON parsing, PuLP model building, MPS writing, and COIN-OR CBC subprocess
execution in warm state across K randomized trial instances per corpus family, while also
recording canonical LP relaxations, per-trial statuses, optimal objectives, and full git and
binary provenance.
"""

from __future__ import annotations

import argparse
import atexit
import glob
import hashlib
import importlib.metadata
import json
import os
import platform
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
from concurrent.futures import ProcessPoolExecutor, as_completed
from datetime import UTC, datetime
from typing import Any

# Support running directly from the workspace with the sibling PuLP virtualenv.
_GOBLIN_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
_REPO_ROOT = os.path.abspath(os.path.join(_GOBLIN_ROOT, ".."))
_CBC_DIST_DIR = os.path.join(_GOBLIN_ROOT, "third_party", "Cbc", "dist")
for _cand_site in sorted(
    glob.glob(
        os.path.join(_REPO_ROOT, "pulp", ".venv", "lib", "python*", "site-packages")
    )
):
    if _cand_site not in sys.path:
        sys.path.insert(0, _cand_site)
_VENDORED_PULP = os.path.join(_REPO_ROOT, "pulp")
if (
    os.path.isdir(os.path.join(_VENDORED_PULP, "pulp"))
    and _VENDORED_PULP not in sys.path
):
    sys.path.insert(0, _VENDORED_PULP)

import pulp

_WORKER_MIP_SOLVER: Any = None
_WORKER_LP_SOLVER: Any = None
_WORKER_TMPDIR: tempfile.TemporaryDirectory[str] | None = None
_WORKER_PULP_VERSION: str = "unknown"
_WORKER_CBC_PATH: str = ""
_WORKER_CBC_VERSION: str = "unknown"
_WORKER_CBC_SHA256: str = ""
_WORKER_SPAWN_FLOOR_MS: float = 0.0
_WORKER_LP_SPAWN_FLOOR_MS: float = 0.0
_WORKER_PROVENANCE: dict[str, Any] = {}


def _detect_pulp_version() -> str:
    ver = getattr(pulp, "__version__", None)
    if ver:
        return str(ver)
    try:
        return importlib.metadata.version("PuLP")
    except importlib.metadata.PackageNotFoundError:
        return "unknown"


def _file_sha256_short(path: str) -> str:
    if not path or not os.path.isfile(path):
        return "unknown"
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        while chunk := fh.read(65536):
            h.update(chunk)
    return h.hexdigest()[:16]


def _detect_cbc_version(cbc_path: str) -> str:
    if not cbc_path or not os.path.isfile(cbc_path):
        return "unknown"
    cbc_lib_dir = os.path.join(_CBC_DIST_DIR, "lib")
    env = dict(os.environ)
    if os.path.isdir(cbc_lib_dir):
        ld = env.get("LD_LIBRARY_PATH", "")
        env["LD_LIBRARY_PATH"] = f"{cbc_lib_dir}:{ld}" if ld else cbc_lib_dir
    try:
        out = subprocess.check_output(
            [cbc_path, "-quit"], text=True, stderr=subprocess.STDOUT, env=env, timeout=5
        )
        lines = [line.strip() for line in out.splitlines() if line.strip()]
        for s in lines:
            if s.startswith("Version:"):
                return s.split("Version:", 1)[1].strip()
        for s in lines:
            if "CBC" in s or "Coin" in s:
                return s.split(" \u2014", 1)[0].strip()
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"[python_baseline] CBC version probe failed: {exc}", file=sys.stderr)
    return "unknown"


def _repo_provenance() -> dict[str, Any]:
    git_commit = "unknown"
    git_dirty = True
    try:
        git_commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=_GOBLIN_ROOT, text=True, timeout=5
        ).strip()
        st = subprocess.check_output(
            ["git", "status", "--porcelain", "--", ".", ":(exclude)bench/results"],
            cwd=_GOBLIN_ROOT,
            text=True,
            timeout=5,
        ).strip()
        git_dirty = len(st) > 0
    except (OSError, subprocess.SubprocessError):
        pass

    fp = subprocess.check_output(
        ["go", "run", "./cmd/goblin-bench", "-print-fingerprint"],
        cwd=_GOBLIN_ROOT,
        text=True,
        timeout=30,
    ).strip()

    return {
        "git_commit": git_commit,
        "git_dirty": git_dirty,
        "source_fingerprint": fp,
        "uname_release": platform.uname().release,
        "measured_at": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }


def _extract_solve_status(solve_ret: Any, prob: Any) -> tuple[int, str]:
    if hasattr(solve_ret, "status"):
        st = solve_ret.status
        code = int(getattr(st, "value", st))
        name = str(getattr(solve_ret, "status_str", getattr(st, "name", str(st))))
    else:
        code = int(solve_ret)
        lp_status_map = getattr(pulp, "LpStatus", {})
        name = str(lp_status_map.get(code, "Unknown"))

    # PuLP 4.x maps CBC's "Optimal (within gap tolerance)" banner at ratioGap=1e-6 to GapLimit (-7).
    if (code == -7 or name == "GapLimit") and any(
        v.varValue is not None for v in prob.variables()
    ):
        return 1, "Optimal"
    return code, name


def _make_cbc_solver(mip: bool = True) -> tuple[Any, str]:
    cbc_lib_dir = os.path.join(_CBC_DIST_DIR, "lib")
    if os.path.isdir(cbc_lib_dir):
        ld = os.environ.get("LD_LIBRARY_PATH", "")
        if cbc_lib_dir not in ld.split(":"):
            os.environ["LD_LIBRARY_PATH"] = f"{cbc_lib_dir}:{ld}" if ld else cbc_lib_dir

    mip_opts = ["ratioGap=1e-6", "allowableGap=1e-6"] if mip else []
    candidates = [
        os.environ.get("CBC_BIN", ""),
        os.path.join(_CBC_DIST_DIR, "bin", "cbc"),
        shutil.which("cbc") or "",
    ]
    for cand in candidates:
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            return (
                pulp.COIN_CMD(
                    path=cand, msg=False, mip=mip, threads=1, options=mip_opts
                ),
                cand,
            )

    pulp_cbc = getattr(pulp, "PULP_CBC_CMD", None)
    if pulp_cbc is not None:
        solver = pulp_cbc(msg=False, mip=mip, threads=1, options=mip_opts)
        if solver.available():
            return solver, getattr(solver, "path", "") or ""

    solver = pulp.COIN_CMD(msg=False, mip=mip, threads=1, options=mip_opts)
    return solver, getattr(solver, "path", "") or ""


def _cleanup_worker() -> None:
    global _WORKER_TMPDIR
    if _WORKER_TMPDIR is not None:
        _WORKER_TMPDIR.cleanup()
        _WORKER_TMPDIR = None


_SPAWN_FLOOR_MIP_MODEL: dict[str, Any] = {
    "parameters": {"name": "spawn_floor_mip", "sense": 1, "status": 0, "sol_status": 0},
    "objective": {"name": "obj", "coefficients": [{"name": "x", "value": 1}]},
    "variables": [
        {
            "name": "x",
            "cat": "Integer",
            "lowBound": 0,
            "upBound": 10,
            "varValue": None,
            "dj": None,
        }
    ],
    "constraints": [
        {
            "name": "c",
            "sense": 1,
            "coefficients": [{"name": "x", "value": 1}],
            "pi": None,
            "constant": -1,
        }
    ],
    "sos1": [],
    "sos2": [],
}

_SPAWN_FLOOR_LP_MODEL: dict[str, Any] = {
    "parameters": {"name": "spawn_floor_lp", "sense": 1, "status": 0, "sol_status": 0},
    "objective": {"name": "obj", "coefficients": [{"name": "x", "value": 1}]},
    "variables": [
        {
            "name": "x",
            "cat": "Continuous",
            "lowBound": 0,
            "upBound": 10,
            "varValue": None,
            "dj": None,
        }
    ],
    "constraints": [
        {
            "name": "c",
            "sense": 1,
            "coefficients": [{"name": "x", "value": 1}],
            "pi": None,
            "constant": -1,
        }
    ],
    "sos1": [],
    "sos2": [],
}


def _cpu_affinity() -> str:
    """Return the CPUs allowed for the current process compressed into ranges."""
    if not hasattr(os, "sched_getaffinity"):
        return "unknown"
    cpus = sorted(os.sched_getaffinity(0))
    ranges: list[str] = []
    start = prev = cpus[0]
    for c in [*cpus[1:], -1]:
        if c != prev + 1:
            ranges.append(str(start) if start == prev else f"{start}-{prev}")
            start = c
        prev = c
    return ",".join(ranges)


def _measure_spawn_floor(
    template: dict[str, Any], solver: Any, repeats: int = 15
) -> float:
    """Measure median wall time in ms of prob.solve() on a 1-variable 1-row model."""
    times: list[float] = []
    raw = json.dumps(template)
    for k in range(repeats + 2):
        _, prob = pulp.LpProblem.fromDict(json.loads(raw))
        t0 = time.perf_counter_ns()
        prob.solve(solver)
        if k >= 2:
            times.append((time.perf_counter_ns() - t0) / 1e6)
    return statistics.median(times)


def _init_worker(warmup_json_path: str, prov: dict[str, Any]) -> None:
    """Warm up Python interpreter, PuLP classes, allocator pages, temp dir, and CBC binary."""
    global _WORKER_MIP_SOLVER, _WORKER_LP_SOLVER, _WORKER_TMPDIR
    global \
        _WORKER_PULP_VERSION, \
        _WORKER_CBC_PATH, \
        _WORKER_CBC_VERSION, \
        _WORKER_CBC_SHA256
    global _WORKER_SPAWN_FLOOR_MS, _WORKER_LP_SPAWN_FLOOR_MS, _WORKER_PROVENANCE
    _WORKER_PROVENANCE = dict(prov)
    _WORKER_MIP_SOLVER, cbc_path = _make_cbc_solver(mip=True)
    _WORKER_LP_SOLVER, _ = _make_cbc_solver(mip=False)
    _WORKER_PULP_VERSION = _detect_pulp_version()
    _WORKER_CBC_PATH = cbc_path
    _WORKER_CBC_VERSION = _detect_cbc_version(cbc_path)
    _WORKER_CBC_SHA256 = _file_sha256_short(cbc_path)
    _WORKER_TMPDIR = tempfile.TemporaryDirectory(prefix="pulp_warm_")
    atexit.register(_cleanup_worker)

    if warmup_json_path and os.path.isfile(warmup_json_path):
        with open(warmup_json_path, "r", encoding="utf-8") as f:
            raw = f.read()
        for _ in range(2):
            data = json.loads(raw)
            _, prob = pulp.LpProblem.fromDict(data)
            mps_path = os.path.join(_WORKER_TMPDIR.name, "warmup.mps")
            prob.writeMPS(mps_path)
            prob.solve(_WORKER_MIP_SOLVER)
            _, prob_lp = pulp.LpProblem.fromDict(json.loads(raw))
            prob_lp.solve(_WORKER_LP_SOLVER)
    _WORKER_SPAWN_FLOOR_MS = _measure_spawn_floor(
        _SPAWN_FLOOR_MIP_MODEL, _WORKER_MIP_SOLVER
    )
    _WORKER_LP_SPAWN_FLOOR_MS = _measure_spawn_floor(
        _SPAWN_FLOOR_LP_MODEL, _WORKER_LP_SOLVER
    )


def _load_trial_lines(json_path: str, n_trials: int) -> list[str]:
    pid = os.path.splitext(os.path.basename(json_path))[0]
    trials_path = os.path.join(os.path.dirname(json_path), "trials", f"{pid}.jsonl")
    if os.path.isfile(trials_path):
        with open(trials_path, "r", encoding="utf-8") as f:
            lines = [line.strip() for line in f if line.strip()]
        if len(lines) >= n_trials:
            return lines[:n_trials]
        if lines:
            n_base = len(lines)
            while len(lines) < n_trials:
                lines.append(lines[len(lines) % n_base])
            return lines
    with open(json_path, "r", encoding="utf-8") as f:
        raw = f.read()
    return [raw] * n_trials


def benchmark_single_json(
    json_path: str, warmup: int = 2, trials: int = 11
) -> dict[str, Any]:
    if _WORKER_MIP_SOLVER is None:
        _init_worker(json_path, _repo_provenance())

    pid = os.path.splitext(os.path.basename(json_path))[0]
    if pid.startswith("t3_"):
        trials = min(trials, 5)
        warmup = max(1, min(warmup, 1))
    else:
        warmup = max(2, warmup)

    trial_lines = _load_trial_lines(json_path, trials)
    canonical_raw = trial_lines[0]

    t_parse_list: list[float] = []
    t_build_list: list[float] = []
    t_format_list: list[float] = []
    t_solve_list: list[float] = []
    t_total_list: list[float] = []
    trial_objectives: list[float | None] = []
    trial_statuses: list[str] = []

    status_code = 0
    status_str = "NotSolved"
    obj_val: float | None = None
    lp_relaxed_obj: float | None = None
    sol_values: dict[str, float] = {}
    n_vars = 0
    n_constrs = 0
    is_mip = False

    assert _WORKER_TMPDIR is not None
    mps_path = os.path.join(_WORKER_TMPDIR.name, f"{pid}_check.mps")

    for run_idx in range(warmup + trials):
        raw_payload = (
            canonical_raw if run_idx < warmup else trial_lines[run_idx - warmup]
        )

        t0 = time.perf_counter_ns()
        data = json.loads(raw_payload)
        t1 = time.perf_counter_ns()

        var_dict, prob = pulp.LpProblem.fromDict(data)
        t2 = time.perf_counter_ns()

        prob.writeMPS(mps_path)
        t3 = time.perf_counter_ns()

        solver = _WORKER_MIP_SOLVER if prob.isMIP() else _WORKER_LP_SOLVER
        t_solve_start = time.perf_counter_ns()
        solve_ret = prob.solve(solver)
        t_solve_end = time.perf_counter_ns()
        cur_code, cur_str = _extract_solve_status(solve_ret, prob)

        if run_idx >= warmup:
            trial_idx = run_idx - warmup
            parse_ms = (t1 - t0) / 1e6
            build_ms = (t2 - t1) / 1e6
            format_ms = (t3 - t2) / 1e6
            solve_ms = (t_solve_end - t_solve_start) / 1e6
            # End-to-end pipeline is parse + build + solve (prob.solve internally calls writeMPS).
            total_ms = parse_ms + build_ms + solve_ms
            t_parse_list.append(parse_ms)
            t_build_list.append(build_ms)
            t_format_list.append(format_ms)
            t_solve_list.append(solve_ms)
            t_total_list.append(total_ms)
            trial_statuses.append(cur_str)

            if cur_code == 1 or cur_str == "Optimal":
                raw_trial_obj = pulp.value(prob.objective)
                trial_objectives.append(
                    float(raw_trial_obj) if raw_trial_obj is not None else 0.0
                )
            else:
                trial_objectives.append(None)

            if trial_idx == 0:
                status_code = cur_code
                status_str = cur_str
                n_vars = len(prob.variables())
                constrs = (
                    prob.constraints()
                    if callable(prob.constraints)
                    else prob.constraints
                )
                n_constrs = len(constrs)
                is_mip = bool(prob.isMIP())
                if status_code == 1 or status_str == "Optimal":
                    raw_obj = pulp.value(prob.objective)
                    obj_val = float(raw_obj) if raw_obj is not None else 0.0
                    for vname, vobj in var_dict.items():
                        if vobj.varValue is not None:
                            sol_values[vname] = float(vobj.varValue)

    # Compute pure LP relaxation objective on canonical trial 0 via CBC (mip=False).
    _, prob_lp = pulp.LpProblem.fromDict(json.loads(canonical_raw))
    lp_ret = prob_lp.solve(_WORKER_LP_SOLVER)
    lp_code, lp_str = _extract_solve_status(lp_ret, prob_lp)
    if lp_code == 1 or lp_str == "Optimal":
        raw_lp_obj = pulp.value(prob_lp.objective)
        lp_relaxed_obj = float(raw_lp_obj) if raw_lp_obj is not None else 0.0

    return {
        "id": pid,
        "num_vars": n_vars,
        "num_constraints": n_constrs,
        "is_mip": is_mip,
        "status_code": status_code,
        "status": status_str,
        "objective": obj_val,
        "trial_objectives": trial_objectives,
        "trial_statuses": trial_statuses,
        "lp_relaxation_objective": lp_relaxed_obj,
        "solution": sol_values,
        "trials": trials,
        "pulp_version": _WORKER_PULP_VERSION,
        "cbc_path": os.path.basename(_WORKER_CBC_PATH),
        "cbc_version": _WORKER_CBC_VERSION,
        "cbc_sha256": _WORKER_CBC_SHA256,
        "cbc_spawn_floor_ms": _WORKER_SPAWN_FLOOR_MS,
        "cbc_lp_spawn_floor_ms": _WORKER_LP_SPAWN_FLOOR_MS,
        "cpu_affinity": _cpu_affinity(),
        "git_commit": _WORKER_PROVENANCE.get("git_commit", ""),
        "git_dirty": bool(_WORKER_PROVENANCE.get("git_dirty", True)),
        "source_fingerprint": _WORKER_PROVENANCE.get("source_fingerprint", ""),
        "uname_release": _WORKER_PROVENANCE.get("uname_release", ""),
        "measured_at": _WORKER_PROVENANCE.get("measured_at", ""),
        "t_solve_trials_ms": t_solve_list,
        "t_total_trials_ms": t_total_list,
        "t_parse_median_ms": statistics.median(t_parse_list),
        "t_build_median_ms": statistics.median(t_build_list),
        "t_format_median_ms": statistics.median(t_format_list),
        "t_solve_median_ms": statistics.median(t_solve_list),
        "t_total_median_ms": statistics.median(t_total_list),
        "t_total_mean_ms": statistics.mean(t_total_list),
        "t_total_sd_ms": statistics.stdev(t_total_list)
        if len(t_total_list) > 1
        else 0.0,
    }


def _bench_worker(task: tuple[str, int, int]) -> dict[str, Any]:
    jf, warmup, trials = task
    return benchmark_single_json(jf, warmup=warmup, trials=trials)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--corpus-dir", required=True)
    parser.add_argument("--out-json", required=True)
    parser.add_argument("--trials", type=int, default=11)
    parser.add_argument("--workers", type=int, default=1)
    parser.add_argument(
        "--require-cbc-version",
        default="CBC devel (git:c1c171ec)",
        help="abort unless every worker ran the specified CBC version (empty string disables check); "
        "set CBC_BIN to select the binary",
    )
    args = parser.parse_args()

    prov = _repo_provenance()
    json_files = sorted(glob.glob(os.path.join(args.corpus_dir, "*.json")))
    warmup_json = json_files[0] if json_files else ""
    for jf in json_files:
        if os.path.basename(jf) == "t1_std_mip.json":
            warmup_json = jf
            break

    tasks = [(jf, 2, args.trials) for jf in json_files]
    results: dict[str, Any] = {}

    with ProcessPoolExecutor(
        max_workers=max(1, args.workers),
        initializer=_init_worker,
        initargs=(warmup_json, prov),
    ) as pool:
        future_map = {pool.submit(_bench_worker, t): t[0] for t in tasks}
        for fut in as_completed(future_map):
            res = fut.result()
            results[res["id"]] = res

    ordered: dict[str, Any] = {}
    for jf in json_files:
        pid = os.path.splitext(os.path.basename(jf))[0]
        res = results[pid]
        ordered[pid] = res
        print(
            f"[Python PuLP+CBC] {res['id']:18s} | vars={res['num_vars']:5d} rows={res['num_constraints']:4d} "
            f"| status={res['status']:10s} | obj={res['objective']!s:14s} "
            f"| build={res['t_build_median_ms']:6.2f}ms format={res['t_format_median_ms']:6.2f}ms "
            f"solve={res['t_solve_median_ms']:7.2f}ms total={res['t_total_median_ms']:7.2f}ms"
        )

    if args.require_cbc_version:
        seen = {str(res["cbc_version"]) for res in ordered.values()}
        if any(not v.startswith(args.require_cbc_version) for v in seen):
            sys.exit(
                f"CBC version mismatch: required {args.require_cbc_version}, measured {sorted(seen)}; "
                "set CBC_BIN to the intended binary or pass --require-cbc-version ''"
            )

    os.makedirs(os.path.dirname(os.path.abspath(args.out_json)), exist_ok=True)
    with open(args.out_json, "w", encoding="utf-8") as f:
        json.dump(ordered, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
