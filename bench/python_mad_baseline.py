#!/usr/bin/env python3
"""
Python PuLP and COIN-OR CBC baseline harness for the 13-family combinatorial MAD / PMAD suite.

Exports the exact BuildMADModel formulations via `goblin-bench -export-mad-dir` into a temporary
directory, verifies canonical Trial-0 digests against `testdata/mad_corpus/manifest.json`, and
times warm-state JSON parsing, PuLP model construction, and 250 ms-capped COIN-OR CBC subprocess
execution across all 131 trials with full git, binary, and CPU affinity provenance.
"""

from __future__ import annotations

import argparse
import hashlib
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

# Disable bytecode emission before importing local helpers (pre-commit forbids __pycache__).
sys.dont_write_bytecode = True

from _bench_common import (
    CBC_DIST_DIR,
    GOBLIN_ROOT,
    bootstrap_venv,
    cpu_affinity,
    prepend_cbc_lib_dir,
    repo_provenance,
)

bootstrap_venv(include_pulp=True)

import pulp


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
    cbc_lib_dir = os.path.join(CBC_DIST_DIR, "lib")
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
        print(f"[python_mad_baseline] CBC version probe failed: {exc}", file=sys.stderr)
    return "unknown"


def _make_cbc_solver(time_limit_s: float) -> tuple[Any, str]:
    prepend_cbc_lib_dir()
    mip_opts = ["ratioGap=1e-6", "allowableGap=1e-6"]
    candidates = [
        os.environ.get("CBC_BIN", ""),
        os.path.join(CBC_DIST_DIR, "bin", "cbc"),
        shutil.which("cbc") or "",
    ]
    for cand in candidates:
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            return (
                pulp.COIN_CMD(
                    path=cand,
                    msg=False,
                    mip=True,
                    threads=1,
                    timeLimit=time_limit_s,
                    timeMode="elapsed",
                    options=mip_opts,
                ),
                cand,
            )

    pulp_cbc = getattr(pulp, "PULP_CBC_CMD", None)
    if pulp_cbc is not None:
        solver = pulp_cbc(
            msg=False,
            mip=True,
            threads=1,
            timeLimit=time_limit_s,
            timeMode="elapsed",
            options=mip_opts,
        )
        if solver.available():
            return solver, getattr(solver, "path", "") or ""

    solver = pulp.COIN_CMD(
        msg=False,
        mip=True,
        threads=1,
        timeLimit=time_limit_s,
        timeMode="elapsed",
        options=mip_opts,
    )
    return solver, getattr(solver, "path", "") or ""


def _extract_mad_solve_result(
    solve_ret: Any, prob: Any
) -> tuple[str, bool, float | None]:
    if hasattr(solve_ret, "status"):
        st = solve_ret.status
        code = int(getattr(st, "value", st))
        name = str(getattr(solve_ret, "status_str", getattr(st, "name", str(st))))
    else:
        code = int(solve_ret)
        name = "Optimal" if code == 1 else str(code)

    vars_list = prob.variables()
    has_incumbent = len(vars_list) > 0 and all(
        v.varValue is not None for v in vars_list
    )
    obj_val: float | None = None
    if has_incumbent:
        raw_obj = pulp.value(prob.objective)
        if raw_obj is not None:
            obj_val = float(raw_obj)
        else:
            has_incumbent = False

    # PuLP 4.x maps CBC's "Optimal (within gap tolerance)" at ratioGap=1e-6 to GapLimit (-7).
    if (
        code == 1
        or name == "Optimal"
        or ((code == -7 or name == "GapLimit") and has_incumbent)
    ):
        return "Optimal", has_incumbent, obj_val

    if code in (-4, 0, -11) or name in ("TimeLimit", "NotSolved", "Stopped"):
        return "TimeLimit", has_incumbent, obj_val

    return name, has_incumbent, obj_val


def _export_mad_jsonl(out_dir: str, goblin_bench_bin: str) -> dict[str, Any]:
    if goblin_bench_bin:
        cmd = [goblin_bench_bin, "-export-mad-dir", out_dir]
    else:
        cmd = ["go", "run", "./cmd/goblin-bench", "-export-mad-dir", out_dir]
    subprocess.check_call(cmd, cwd=GOBLIN_ROOT, stdout=subprocess.DEVNULL)
    with open(os.path.join(out_dir, "manifest.json"), "r", encoding="utf-8") as f:
        data: dict[str, Any] = json.load(f)
    return data


def _verify_manifest_parity(
    corpus_dir: str, exported_manifest: dict[str, Any]
) -> list[dict[str, Any]]:
    manifest_path = os.path.join(corpus_dir, "manifest.json")
    with open(manifest_path, "r", encoding="utf-8") as f:
        disk_manifest = json.load(f)
    disk_cases: list[dict[str, Any]] = disk_manifest["cases"]
    exp_by_id = {c["id"]: c for c in exported_manifest["cases"]}
    if len(disk_cases) != len(exp_by_id):
        raise RuntimeError(
            f"manifest case count mismatch: disk={len(disk_cases)} exported={len(exp_by_id)}"
        )
    for case in disk_cases:
        cid = case["id"]
        exp = exp_by_id.get(cid)
        if exp is None:
            raise RuntimeError(f"missing exported MAD case {cid}")
        if exp["trial0_sha256"] != case["trial0_sha256"]:
            raise RuntimeError(
                f"{cid}: trial0_sha256 mismatch (disk={case['trial0_sha256']} exported={exp['trial0_sha256']})"
            )
        if int(exp["trials"]) != int(case["trials"]):
            raise RuntimeError(
                f"{cid}: trials mismatch (disk={case['trials']} exported={exp['trials']})"
            )
    return disk_cases


def _load_jsonl_lines(path: str, expected_trials: int) -> list[str]:
    with open(path, "r", encoding="utf-8") as f:
        lines = [line.strip() for line in f if line.strip()]
    if len(lines) != expected_trials:
        raise RuntimeError(
            f"{path}: got {len(lines)} trial lines, expected {expected_trials}"
        )
    return lines


def benchmark_mad_case(
    case: dict[str, Any],
    trial_lines: list[str],
    solver: Any,
    meta: dict[str, Any],
    time_limit_ms: int,
) -> dict[str, Any]:
    cid = str(case["id"])
    tier = str(case["tier"])
    trials = int(case["trials"])
    warmup = 1 if tier == "tier3_large" else 2

    t_parse_list: list[float] = []
    t_build_list: list[float] = []
    t_solve_list: list[float] = []
    t_total_list: list[float] = []
    trial_statuses: list[str] = []
    trial_objectives: list[float | None] = []
    optimal_count = 0
    time_limit_count = 0

    trial0_status = "TimeLimit"
    trial0_has_inc = False
    trial0_obj: float | None = None
    n_vars = 0
    n_constrs = 0

    for run_idx in range(warmup + trials):
        raw_payload = (
            trial_lines[0] if run_idx < warmup else trial_lines[run_idx - warmup]
        )

        t0 = time.perf_counter_ns()
        data = json.loads(raw_payload)
        t1 = time.perf_counter_ns()

        _, prob = pulp.LpProblem.fromDict(data)
        t2 = time.perf_counter_ns()

        solve_ret = prob.solve(solver)
        t3 = time.perf_counter_ns()

        cur_status, has_inc, cur_obj = _extract_mad_solve_result(solve_ret, prob)

        if run_idx >= warmup:
            trial_idx = run_idx - warmup
            parse_ms = (t1 - t0) / 1e6
            build_ms = (t2 - t1) / 1e6
            solve_ms = (t3 - t2) / 1e6
            total_ms = parse_ms + build_ms + solve_ms

            t_parse_list.append(parse_ms)
            t_build_list.append(build_ms)
            t_solve_list.append(solve_ms)
            t_total_list.append(total_ms)
            trial_statuses.append(cur_status)
            trial_objectives.append(cur_obj)

            if cur_status == "Optimal":
                optimal_count += 1
            elif cur_status == "TimeLimit":
                time_limit_count += 1

            if trial_idx == 0:
                trial0_status = cur_status
                trial0_has_inc = has_inc
                trial0_obj = cur_obj
                n_vars = len(prob.variables())
                constrs = (
                    prob.constraints()
                    if callable(prob.constraints)
                    else prob.constraints
                )
                n_constrs = len(constrs)

    return {
        "id": cid,
        "tier": tier,
        "family": str(case["family"]),
        "groups": int(case["groups"]),
        "items": int(case["items"]),
        "periods": int(case["periods"]),
        "trials": trials,
        "time_limit_ms": time_limit_ms,
        "num_vars": n_vars,
        "num_constraints": n_constrs,
        "status": trial0_status,
        "trial_statuses": trial_statuses,
        "has_incumbent": trial0_has_inc,
        "objective": trial0_obj,
        "trial_objectives": trial_objectives,
        "optimal_count": optimal_count,
        "time_limit_count": time_limit_count,
        "pulp_version": meta["pulp_version"],
        "cbc_path": meta["cbc_path"],
        "cbc_version": meta["cbc_version"],
        "cbc_sha256": meta["cbc_sha256"],
        "cpu_affinity": meta["cpu_affinity"],
        "git_commit": meta["git_commit"],
        "git_dirty": meta["git_dirty"],
        "source_fingerprint": meta["source_fingerprint"],
        "uname_release": meta["uname_release"],
        "measured_at": meta["measured_at"],
        "t_parse_median_ms": statistics.median(t_parse_list),
        "t_build_median_ms": statistics.median(t_build_list),
        "t_solve_median_ms": statistics.median(t_solve_list),
        "t_solve_trials_ms": t_solve_list,
        "t_total_median_ms": statistics.median(t_total_list),
        "t_total_mean_ms": statistics.mean(t_total_list),
        "t_total_sd_ms": statistics.stdev(t_total_list)
        if len(t_total_list) > 1
        else 0.0,
        "t_total_trials_ms": t_total_list,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--corpus-dir", default="testdata/mad_corpus")
    parser.add_argument("--out-json", required=True)
    parser.add_argument("--time-limit-ms", type=int, default=250)
    parser.add_argument(
        "--goblin-bench",
        default="",
        help="Optional pre-built goblin-bench binary for -export-mad-dir",
    )
    parser.add_argument(
        "--require-cbc-version",
        default="CBC devel (git:c1c171ec)",
        help="abort unless CBC matches this version prefix (empty string disables check)",
    )
    args = parser.parse_args()

    prov = repo_provenance()
    solver, cbc_path = _make_cbc_solver(args.time_limit_ms / 1000.0)
    cbc_version = _detect_cbc_version(cbc_path)
    if args.require_cbc_version and not cbc_version.startswith(
        args.require_cbc_version
    ):
        sys.exit(
            f"CBC version mismatch: required {args.require_cbc_version}, measured {cbc_version}; "
            "set CBC_BIN to the intended binary or pass --require-cbc-version ''"
        )

    meta: dict[str, Any] = {
        "pulp_version": _detect_pulp_version(),
        "cbc_path": os.path.basename(cbc_path),
        "cbc_version": cbc_version,
        "cbc_sha256": _file_sha256_short(cbc_path),
        "cpu_affinity": cpu_affinity(),
        "git_commit": prov.get("git_commit", ""),
        "git_dirty": bool(prov.get("git_dirty", True)),
        "source_fingerprint": prov.get("source_fingerprint", ""),
        "uname_release": prov.get("uname_release", ""),
        "measured_at": prov.get("measured_at", ""),
    }

    with tempfile.TemporaryDirectory(prefix="goblin_mad_pulp_") as tmp_dir:
        exported_manifest = _export_mad_jsonl(tmp_dir, args.goblin_bench)
        cases = _verify_manifest_parity(args.corpus_dir, exported_manifest)

        # Warm up Python, PuLP fromDict, MPS serialization, and CBC subprocess execution.
        for warm_id in ("pmad_base_g10_i2", "mad_comb_rand_g2_i4"):
            warm_path = os.path.join(tmp_dir, f"{warm_id}.jsonl")
            if os.path.isfile(warm_path):
                with open(warm_path, "r", encoding="utf-8") as f:
                    first_line = f.readline().strip()
                if first_line:
                    for _ in range(2):
                        _, prob = pulp.LpProblem.fromDict(json.loads(first_line))
                        prob.solve(solver)

        ordered: dict[str, Any] = {}
        for case in cases:
            cid = str(case["id"])
            trial_lines = _load_jsonl_lines(
                os.path.join(tmp_dir, f"{cid}.jsonl"), int(case["trials"])
            )
            res = benchmark_mad_case(
                case, trial_lines, solver, meta, args.time_limit_ms
            )
            ordered[cid] = res
            print(
                f"[Python PuLP+CBC MAD] {cid:22s} | vars={res['num_vars']:4d} rows={res['num_constraints']:4d} "
                f"| status={res['status']:9s} ({res['optimal_count']:2d}/{res['trials']:2d} opt) "
                f"| obj={res['objective']!s:12s} | solve={res['t_solve_median_ms']:7.2f}ms "
                f"total={res['t_total_median_ms']:7.2f}ms"
            )

    os.makedirs(os.path.dirname(os.path.abspath(args.out_json)), exist_ok=True)
    with open(args.out_json, "w", encoding="utf-8") as f:
        json.dump(ordered, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
