#!/usr/bin/env python3
"""
Python SciPy Warm-State Linear Sum Assignment Benchmark & Multi-Seed Golden Reference Harness.

Measures in warm state across K randomized trial instances per family:
1. Pure In-Memory Solve Time (t_solve_ms):
   - Direct call to scipy.optimize.linear_sum_assignment(cost_matrix, maximize=...) on a
     pre-allocated C-contiguous float64 numpy.ndarray.
2. Matrix Preparation + Solve Pipeline (t_prep_ms, t_total_ms).
3. Exact Reference Metrics and Provenance:
   - Canonical (trial 0) optimal objective, per-trial optimal objectives across all K trials,
     bit-level SHA-256 verification of Trial 0 cost matrices against manifest.json, and
     provenance (scipy_version, numpy_version, cpu_affinity).
"""

from __future__ import annotations

import argparse
import glob
import hashlib
import json
import os
import statistics
import sys
import time
from typing import Any

# Support running directly from the workspace with the sibling PuLP venv.
_GOBLIN_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
_REPO_ROOT = os.path.abspath(os.path.join(_GOBLIN_ROOT, ".."))
for _cand_site in sorted(
    glob.glob(
        os.path.join(_REPO_ROOT, "pulp", ".venv", "lib", "python*", "site-packages")
    )
):
    if _cand_site not in sys.path:
        sys.path.insert(0, _cand_site)

import numpy as np
import scipy
from scipy.optimize import linear_sum_assignment


def _cpu_affinity() -> str:
    """Return the CPUs this process may run on, compressed into ranges."""
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


def _splitmix64_vec(seed: int, count: int, start_idx: int = 0) -> Any:
    """Vectorized 64-bit SplitMix64 matching bench/assign_corpus.go splitMix64At."""
    with np.errstate(over="ignore"):
        idx = np.arange(start_idx + 1, start_idx + count + 1, dtype=np.uint64)
        z = np.uint64(seed) + idx * np.uint64(0x9E3779B97F4A7C15)
        z = (z ^ (z >> np.uint64(30))) * np.uint64(0xBF58476D1CE4E5B9)
        z = (z ^ (z >> np.uint64(27))) * np.uint64(0x94D049BB133111EB)
        z = z ^ (z >> np.uint64(31))
    return z


def generate_assign_matrix(case: dict[str, Any], trial: int) -> Any:
    """Construct the exact 2D float64 matrix matching GenerateAssignMatrix in Go."""
    rows = int(case["rows"])
    cols = int(case["cols"])
    total = rows * cols
    family = str(case["family"])
    tier = str(case["tier"])
    trial_seeds = case.get("trial_seeds", [])
    if 0 <= trial < len(trial_seeds):
        seed = int(trial_seeds[trial])
    else:
        seed = int(case["base_seed"]) + trial * 100003

    if family == "canonical":
        base = np.array(
            [
                90.0,
                75.0,
                75.0,
                80.0,
                35.0,
                85.0,
                55.0,
                65.0,
                125.0,
                95.0,
                90.0,
                105.0,
                45.0,
                110.0,
                95.0,
                115.0,
            ],
            dtype=np.float64,
        )
        idx = np.arange(total) % 16
        mat = base[idx].copy()
        if trial > 0:
            u = _splitmix64_vec(seed, total)
            delta = (u % np.uint64(21)).astype(np.int64) - 10
            mat = mat + delta.astype(np.float64)
        return mat.reshape((rows, cols))

    if family in ("uniform", "wide", "tall", "maximize"):
        u = _splitmix64_vec(seed, total)
        vals = (u % np.uint64(10000000)).astype(np.float64) * (1.0 / 1024.0)
        return vals.reshape((rows, cols))

    if family == "integer_ties":
        mod = 16
        if tier == "tier1_small":
            mod = 8
        elif tier == "tier3_large":
            mod = 32
        u = _splitmix64_vec(seed, total)
        vals = (u % np.uint64(mod)).astype(np.float64)
        return vals.reshape((rows, cols))

    if family == "geometric":
        coord_scale = 1.0 / 128.0
        u_coords = _splitmix64_vec(seed, 2 * rows + 2 * cols)
        coords = (u_coords % np.uint64(100000)).astype(np.float64) * coord_scale
        px = coords[0:rows]
        py = coords[rows : 2 * rows]
        qx = coords[2 * rows : 2 * rows + cols]
        qy = coords[2 * rows + cols : 2 * rows + 2 * cols]
        dx = px[:, None] - qx[None, :]
        dy = py[:, None] - qy[None, :]
        return np.ascontiguousarray(dx * dx + dy * dy, dtype=np.float64)

    if family == "rank1":
        noise_scale = 1.0 / 64.0
        u_ab = _splitmix64_vec(seed, rows + cols)
        a = (np.uint64(100) + (u_ab[0:rows] % np.uint64(900))).astype(np.float64)
        b = (np.uint64(100) + (u_ab[rows : rows + cols] % np.uint64(900))).astype(
            np.float64
        )
        u_noise = _splitmix64_vec(seed, total, start_idx=rows + cols)
        noise = (u_noise % np.uint64(64)).astype(np.float64).reshape(
            (rows, cols)
        ) * noise_scale
        return np.ascontiguousarray(a[:, None] * b[None, :] + noise, dtype=np.float64)

    if family == "sparse":
        u = _splitmix64_vec(seed, total).reshape((rows, cols))
        vals = (np.uint64(10) + (u % np.uint64(500))).astype(np.float64)
        r_idx = np.arange(rows)[:, None]
        c_idx = np.arange(cols)[None, :]
        band = (c_idx - r_idx + cols) % cols
        mask = (band > 2) & ((u % np.uint64(10)) < np.uint64(6))
        vals[mask] = np.inf
        return np.ascontiguousarray(vals, dtype=np.float64)

    raise ValueError(f"unknown assign corpus family: {family}")


def _matrix_sha256(mat: Any) -> str:
    arr = np.ascontiguousarray(mat, dtype="<f8")
    return hashlib.sha256(arr.tobytes()).hexdigest()[:16]


def _warm_scipy() -> None:
    """Warm up NumPy and SciPy rectangular_lsap C++ extension before timing."""
    for n in (16, 64, 128):
        mat = np.arange(n * n, dtype=np.float64).reshape((n, n)) % 29.0
        linear_sum_assignment(mat, maximize=False)
        linear_sum_assignment(mat, maximize=True)


def benchmark_case(case: dict[str, Any], default_trials: int) -> dict[str, Any]:
    cid = str(case["id"])
    tier = str(case["tier"])
    rows = int(case["rows"])
    cols = int(case["cols"])
    sense = int(case["sense"])
    maximize = sense == -1
    trials = int(case.get("trials", default_trials))
    if tier == "tier3_large":
        trials = min(trials, 5)
        warmup = 1
    else:
        warmup = 2

    # Pre-verify Trial 0 SHA-256 bit-level parity with Go manifest.json
    mat0 = generate_assign_matrix(case, 0)
    got_sha = _matrix_sha256(mat0)
    want_sha = str(case.get("trial0_sha256", ""))
    if want_sha and got_sha != want_sha:
        raise RuntimeError(
            f"{cid}: trial 0 SHA-256 mismatch: python={got_sha} vs manifest={want_sha}"
        )

    t_prep_list: list[float] = []
    t_solve_list: list[float] = []
    t_total_list: list[float] = []
    trial_objs: list[float] = []

    for run_idx in range(warmup + trials):
        trial_idx = 0 if run_idx < warmup else (run_idx - warmup)

        t0 = time.perf_counter()
        mat = generate_assign_matrix(case, trial_idx)
        t1 = time.perf_counter()
        row_ind, col_ind = linear_sum_assignment(mat, maximize=maximize)
        obj = float(mat[row_ind, col_ind].sum())
        t2 = time.perf_counter()

        if run_idx >= warmup:
            t_prep_list.append((t1 - t0) * 1000.0)
            t_solve_list.append((t2 - t1) * 1000.0)
            t_total_list.append((t2 - t0) * 1000.0)
            trial_objs.append(obj)

    solve_mean = statistics.mean(t_solve_list)
    solve_sd = statistics.stdev(t_solve_list) if len(t_solve_list) >= 2 else 0.0
    total_mean = statistics.mean(t_total_list)
    total_sd = statistics.stdev(t_total_list) if len(t_total_list) >= 2 else 0.0

    return {
        "id": cid,
        "tier": tier,
        "rows": rows,
        "cols": cols,
        "sense": sense,
        "family": str(case["family"]),
        "trials": trials,
        "scipy_version": str(scipy.__version__),
        "numpy_version": str(np.__version__),
        "cpu_affinity": _cpu_affinity(),
        "objective": trial_objs[0],
        "trial_objectives": trial_objs,
        "t_prep_median_ms": statistics.median(t_prep_list),
        "t_solve_median_ms": statistics.median(t_solve_list),
        "t_total_median_ms": statistics.median(t_total_list),
        "t_solve_mean_ms": solve_mean,
        "t_solve_sd_ms": solve_sd,
        "t_total_mean_ms": total_mean,
        "t_total_sd_ms": total_sd,
        "t_solve_trials_ms": t_solve_list,
        "t_total_trials_ms": t_total_list,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument(
        "--corpus-dir",
        type=str,
        default="testdata/assign_corpus",
        help="Directory containing manifest.json",
    )
    parser.add_argument(
        "--out-json",
        type=str,
        default="bench/results/scipy_assign_baseline.json",
        help="Output JSON path",
    )
    parser.add_argument(
        "--trials",
        type=int,
        default=11,
        help="Default measured trials for Tiers 1-2",
    )
    args = parser.parse_args()

    manifest_path = os.path.join(args.corpus_dir, "manifest.json")
    if not os.path.isfile(manifest_path):
        sys.exit(
            f"manifest not found at {manifest_path}; run `go run ./cmd/goblin-bench -generate-corpus` first"
        )

    with open(manifest_path, "r", encoding="utf-8") as fh:
        manifest = json.load(fh)

    cases: list[dict[str, Any]] = manifest.get("cases", [])
    _warm_scipy()

    print(
        f"=== SCIPY ({scipy.__version__}) WARM-STATE LSAP BASELINE (CPU={_cpu_affinity()}) ==="
    )
    print(
        f"{'Problem ID':<20} | {'Shape':>11} | {'Prep (ms)':>10} | {'Solve (ms)':>10} | {'Total (ms)':>10} | {'Canonical Obj':>15}"
    )
    print("-" * 90)

    out_map: dict[str, dict[str, Any]] = {}
    for case in cases:
        res = benchmark_case(case, args.trials)
        out_map[res["id"]] = res
        shape_str = f"{res['rows']}x{res['cols']}"
        print(
            f"{res['id']:<20} | {shape_str:>11} | {res['t_prep_median_ms']:>8.3f}ms | "
            f"{res['t_solve_median_ms']:>8.3f}ms | {res['t_total_median_ms']:>8.3f}ms | {res['objective']:>15.3f}"
        )

    os.makedirs(os.path.dirname(os.path.abspath(args.out_json)), exist_ok=True)
    with open(args.out_json, "w", encoding="utf-8") as out_fh:
        json.dump(out_map, out_fh, indent=2)
        out_fh.write("\n")
    print(f"\nSaved SciPy baseline telemetry to {args.out_json}")


if __name__ == "__main__":
    main()
