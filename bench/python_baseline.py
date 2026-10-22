#!/usr/bin/env python3
"""
Python PuLP + COIN-OR CBC Warm-State Benchmark & Multi-Seed Golden Reference Harness.

Measures both:
1. End-to-End JSON-to-Solution Pipeline (in warm state across K randomized trial instances):
   - t_parse_ms: JSON parsing (json.loads)
   - t_build_ms: PuLP LpProblem.fromDict construction
   - t_format_ms: PuLP writeMPS serialization
   - t_solve_ms: COIN-OR CBC subprocess execution & .sol parsing
   - t_total_ms: Total end-to-end wall clock time
2. Pure Optimization Reference Metrics and Provenance:
   - LP relaxation root objective, canonical (trial 0) optimal objective and primal solution,
     per-trial optimal objectives across all K randomized trial instances, and solver
     provenance (pulp_version, cbc_path, cbc_version).
"""

from __future__ import annotations

import argparse
import atexit
from concurrent.futures import ProcessPoolExecutor, as_completed
import glob
import importlib.metadata
import json
import os
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
from typing import Any

# Support running directly from the workspace with the sibling PuLP checkout/venv.
_REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
for _cand_site in sorted(
    glob.glob(os.path.join(_REPO_ROOT, "pulp", ".venv", "lib", "python*", "site-packages"))
):
    if _cand_site not in sys.path:
        sys.path.insert(0, _cand_site)
_VENDORED_PULP = os.path.join(_REPO_ROOT, "pulp")
if os.path.isdir(os.path.join(_VENDORED_PULP, "pulp")) and _VENDORED_PULP not in sys.path:
    sys.path.insert(0, _VENDORED_PULP)

import pulp  # noqa: E402

_WORKER_MIP_SOLVER: Any = None
_WORKER_LP_SOLVER: Any = None
_WORKER_TMPDIR: tempfile.TemporaryDirectory[str] | None = None
_WORKER_PULP_VERSION: str = "unknown"
_WORKER_CBC_PATH: str = ""
_WORKER_CBC_VERSION: str = "unknown"


def _detect_pulp_version() -> str:
    ver = getattr(pulp, "__version__", None)
    if ver:
        return str(ver)
    try:
        return importlib.metadata.version("PuLP")
    except Exception:
        return "unknown"


def _detect_cbc_version(cbc_path: str) -> str:
    if not cbc_path or not os.path.isfile(cbc_path):
        return "unknown"
    cbc_lib_dir = os.path.abspath(os.path.join(_REPO_ROOT, "Cbc", "dist", "lib"))
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
                return s
    except Exception:
        pass
    return "unknown"


def _extract_solve_status(solve_ret: Any) -> tuple[int, str]:
    if hasattr(solve_ret, "status"):
        st = solve_ret.status
        code = int(getattr(st, "value", st))
        name = str(getattr(solve_ret, "status_str", getattr(st, "name", str(st))))
        return code, name
    code = int(solve_ret)
    lp_status_map = getattr(pulp, "LpStatus", {})
    return code, str(lp_status_map.get(code, "Unknown"))


def _make_cbc_solver(mip: bool = True) -> tuple[Any, str]:
    cbc_lib_dir = os.path.abspath(os.path.join(_REPO_ROOT, "Cbc", "dist", "lib"))
    if os.path.isdir(cbc_lib_dir):
        ld = os.environ.get("LD_LIBRARY_PATH", "")
        if cbc_lib_dir not in ld.split(":"):
            os.environ["LD_LIBRARY_PATH"] = f"{cbc_lib_dir}:{ld}" if ld else cbc_lib_dir

    pulp_cbc = getattr(pulp, "PULP_CBC_CMD", None)
    if pulp_cbc is not None:
        solver = pulp_cbc(msg=False, mip=mip, threads=1)
        if solver.available():
            return solver, getattr(solver, "path", "") or (shutil.which("cbc") or "")

    candidates = [
        os.environ.get("CBC_BIN", ""),
        shutil.which("cbc") or "",
        "/home/ff/dev/motif/.venv/lib/python3.14/site-packages/pulp/solverdir/cbc/linux/i64/cbc",
        os.path.abspath(os.path.join(_REPO_ROOT, "Cbc", "dist", "bin", "cbc")),
    ]
    for cand in candidates:
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            return pulp.COIN_CMD(path=cand, msg=False, mip=mip, threads=1), cand

    solver = pulp.COIN_CMD(msg=False, mip=mip, threads=1)
    return solver, getattr(solver, "path", "") or ""


def _cleanup_worker() -> None:
    global _WORKER_TMPDIR
    if _WORKER_TMPDIR is not None:
        _WORKER_TMPDIR.cleanup()
        _WORKER_TMPDIR = None


def _init_worker(warmup_json_path: str) -> None:
    """Warm up Python interpreter, PuLP classes, allocator pages, temp dir, and CBC binary."""
    global _WORKER_MIP_SOLVER, _WORKER_LP_SOLVER, _WORKER_TMPDIR
    global _WORKER_PULP_VERSION, _WORKER_CBC_PATH, _WORKER_CBC_VERSION
    _WORKER_MIP_SOLVER, cbc_path = _make_cbc_solver(mip=True)
    _WORKER_LP_SOLVER, _ = _make_cbc_solver(mip=False)
    _WORKER_PULP_VERSION = _detect_pulp_version()
    _WORKER_CBC_PATH = cbc_path
    _WORKER_CBC_VERSION = _detect_cbc_version(cbc_path)
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
    global _WORKER_MIP_SOLVER, _WORKER_LP_SOLVER, _WORKER_TMPDIR
    if _WORKER_MIP_SOLVER is None:
        _init_worker(json_path)

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
        raw_payload = canonical_raw if run_idx < warmup else trial_lines[run_idx - warmup]

        t0 = time.perf_counter()
        data = json.loads(raw_payload)
        t1 = time.perf_counter()

        var_dict, prob = pulp.LpProblem.fromDict(data)
        t2 = time.perf_counter()

        prob.writeMPS(mps_path)
        t3 = time.perf_counter()

        t_solve_start = time.perf_counter()
        solve_ret = prob.solve(_WORKER_MIP_SOLVER)
        t_solve_end = time.perf_counter()
        cur_code, cur_str = _extract_solve_status(solve_ret)

        if run_idx >= warmup:
            trial_idx = run_idx - warmup
            parse_ms = (t1 - t0) * 1000.0
            build_ms = (t2 - t1) * 1000.0
            format_ms = (t3 - t2) * 1000.0
            solve_ms = (t_solve_end - t_solve_start) * 1000.0
            # End-to-end pipeline is parse + build + solve (since prob.solve internally calls writeMPS)
            total_ms = parse_ms + build_ms + solve_ms
            t_parse_list.append(parse_ms)
            t_build_list.append(build_ms)
            t_format_list.append(format_ms)
            t_solve_list.append(solve_ms)
            t_total_list.append(total_ms)

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

    # Also compute pure LP relaxation objective on canonical trial 0 via CBC (mip=False)
    _, prob_lp = pulp.LpProblem.fromDict(json.loads(canonical_raw))
    lp_ret = prob_lp.solve(_WORKER_LP_SOLVER)
    lp_code, lp_str = _extract_solve_status(lp_ret)
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
        "lp_relaxation_objective": lp_relaxed_obj,
        "solution": sol_values,
        "trials": trials,
        "pulp_version": _WORKER_PULP_VERSION,
        "cbc_path": _WORKER_CBC_PATH,
        "cbc_version": _WORKER_CBC_VERSION,
        "t_parse_median_ms": statistics.median(t_parse_list),
        "t_build_median_ms": statistics.median(t_build_list),
        "t_format_median_ms": statistics.median(t_format_list),
        "t_solve_median_ms": statistics.median(t_solve_list),
        "t_total_median_ms": statistics.median(t_total_list),
        "t_total_mean_ms": statistics.mean(t_total_list),
        "t_total_sd_ms": statistics.stdev(t_total_list) if len(t_total_list) > 1 else 0.0,
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
    args = parser.parse_args()

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
        initargs=(warmup_json,),
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
            f"| status={res['status']:10s} | obj={str(res['objective']):14s} "
            f"| build={res['t_build_median_ms']:6.2f}ms format={res['t_format_median_ms']:6.2f}ms "
            f"solve={res['t_solve_median_ms']:7.2f}ms total={res['t_total_median_ms']:7.2f}ms"
        )

    os.makedirs(os.path.dirname(os.path.abspath(args.out_json)), exist_ok=True)
    with open(args.out_json, "w", encoding="utf-8") as f:
        json.dump(ordered, f, indent=2)


if __name__ == "__main__":
    main()
