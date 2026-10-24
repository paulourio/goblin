#!/usr/bin/env python3
"""Differential test of goblin/assign against scipy.optimize.linear_sum_assignment.

Generates random rectangular and square assignment instances across 7 families
(uniform continuous, integer ties, binary {0, 1}, Euclidean geometric, rank-1 outer
products, multi-scale rows, and sparse matrices with forbidden +Inf/-Inf edges,
including infeasible instances), solves them with SciPy and with goblin (both scalar
and SIMD via `go run ./cmd/goblin-difftest -assign`), and verifies exact parity on
feasibility status, objective value, and primal validity.
"""

from __future__ import annotations

import argparse
import glob
import json
import math
import os
import random
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

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
from scipy.optimize import linear_sum_assignment

OBJ_RTOL = 1e-9


def generate_instance(rng: random.Random, idx: int) -> dict[str, Any]:
    shape_kind = rng.randint(0, 3)
    if shape_kind == 0:
        rows = cols = rng.randint(1, 40)
    elif shape_kind == 1:
        rows = rng.randint(1, 25)
        cols = rows + rng.randint(1, 30)
    elif shape_kind == 2:
        cols = rng.randint(1, 25)
        rows = cols + rng.randint(1, 30)
    else:
        rows = rng.randint(16, 48)
        cols = rng.choice([rows, 260])

    maximize = rng.choice([False, True])
    sense = -1 if maximize else 1
    fam = idx % 7
    cost: list[float] = []

    if fam == 0:
        cost = [round(rng.uniform(-1000.0, 1000.0), 4) for _ in range(rows * cols)]
    elif fam == 1:
        cost = [float(rng.randint(0, 5)) for _ in range(rows * cols)]
    elif fam == 2:
        cost = [float(rng.randint(0, 1)) for _ in range(rows * cols)]
    elif fam == 3:
        px = [rng.uniform(0, 50) for _ in range(rows)]
        py = [rng.uniform(0, 50) for _ in range(rows)]
        qx = [rng.uniform(0, 50) for _ in range(cols)]
        qy = [rng.uniform(0, 50) for _ in range(cols)]
        for r in range(rows):
            for c in range(cols):
                d2 = (px[r] - qx[c]) ** 2 + (py[r] - qy[c]) ** 2
                cost.append(round(d2, 4))
    elif fam == 4:
        a = [float(rng.randint(1, 40)) for _ in range(rows)]
        b = [float(rng.randint(1, 40)) for _ in range(cols)]
        cost = [a[r] * b[c] for r in range(rows) for c in range(cols)]
    elif fam == 5:
        exps = [1e-3, 1e-1, 1.0, 1e2, 1e4]
        for _ in range(rows):
            s = rng.choice(exps)
            for _ in range(cols):
                cost.append(round(rng.uniform(-5.0, 15.0) * s, 5))
    else:
        forbid = "-Inf" if maximize else "+Inf"
        for _ in range(rows * cols):
            if rng.random() < 0.35:
                cost.append(float("-inf") if forbid == "-Inf" else float("inf"))
            else:
                cost.append(float(rng.randint(-30, 150)))

    return {
        "id": f"assign_{idx}",
        "rows": rows,
        "cols": cols,
        "sense": sense,
        "cost": [
            ("+Inf" if math.isinf(v) and v > 0 else ("-Inf" if math.isinf(v) else v))
            for v in cost
        ],
        "raw_cost": cost,
    }


def solve_scipy(inst: dict[str, Any]) -> tuple[str, float | None]:
    rows, cols = inst["rows"], inst["cols"]
    maximize = inst["sense"] == -1
    mat = np.array(inst["raw_cost"], dtype=np.float64).reshape((rows, cols))
    try:
        r_ind, c_ind = linear_sum_assignment(mat, maximize=maximize)
    except ValueError:
        return "Infeasible", None
    chosen = mat[r_ind, c_ind]
    if not np.all(np.isfinite(chosen)):
        return "Infeasible", None
    return "Optimal", float(chosen.sum())


def verify_goblin_solution(
    inst: dict[str, Any], row_ind: list[int], col_ind: list[int], got_obj: float
) -> tuple[bool, str]:
    rows, cols = inst["rows"], inst["cols"]
    k = min(rows, cols)
    if len(row_ind) != k or len(col_ind) != k:
        return False, f"wrong cardinality ({len(row_ind)}, {len(col_ind)}), want {k}"
    if row_ind != sorted(row_ind) or len(set(row_ind)) != k:
        return False, "row_ind not strictly increasing unique indices"
    if len(set(col_ind)) != k:
        return False, "col_ind contains duplicate columns"
    raw = inst["raw_cost"]
    total = 0.0
    for r, c in zip(row_ind, col_ind):
        if not (0 <= r < rows and 0 <= c < cols):
            return False, f"index out of bounds ({r}, {c})"
        v = raw[r * cols + c]
        if not math.isfinite(v):
            return False, f"assigned forbidden non-finite edge ({r}, {c})"
        total += v
    tol = OBJ_RTOL * max(1.0, abs(total))
    if abs(total - got_obj) > tol:
        return False, f"sum(cost)={total} != reported obj={got_obj}"
    return True, "ok"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--instances", type=int, default=400)
    parser.add_argument("--seed", type=int, default=1)
    args = parser.parse_args()

    rng = random.Random(args.seed)
    refs: dict[str, tuple[dict[str, Any], str, float | None]] = {}

    with tempfile.TemporaryDirectory(prefix="goblin_assign_difftest_") as tmp:
        inst_path = Path(tmp) / "assign_instances.jsonl"
        with inst_path.open("w", encoding="utf-8") as fh:
            for k in range(args.instances):
                inst = generate_instance(rng, k)
                status, obj = solve_scipy(inst)
                refs[inst["id"]] = (inst, status, obj)
                payload = {
                    "id": inst["id"],
                    "rows": inst["rows"],
                    "cols": inst["cols"],
                    "sense": inst["sense"],
                    "cost": inst["cost"],
                }
                fh.write(json.dumps(payload) + "\n")

        failures: list[str] = []
        for mode_name, extra_flags in [("simd", []), ("scalar", ["-no-simd"])]:
            cmd = [
                "go",
                "run",
                "./cmd/goblin-difftest",
                "-assign",
                "-in",
                str(inst_path),
                *extra_flags,
            ]
            proc = subprocess.run(
                cmd, cwd=_GOBLIN_ROOT, capture_output=True, text=True, check=True
            )
            for line in proc.stdout.splitlines():
                if not line.strip():
                    continue
                got = json.loads(line)
                inst, ref_status, ref_obj = refs[got["id"]]
                got_status = got["status"]
                if got_status != ref_status:
                    failures.append(
                        f"[{mode_name}] {got['id']}: status goblin={got_status} vs scipy={ref_status}"
                    )
                    continue
                if got_status == "Optimal":
                    assert ref_obj is not None
                    ok, msg = verify_goblin_solution(
                        inst, got["row_ind"], got["col_ind"], got["obj"]
                    )
                    if not ok:
                        failures.append(f"[{mode_name}] {got['id']}: {msg}")
                        continue
                    tol = 1e-7 * max(1.0, abs(ref_obj))
                    if abs(got["obj"] - ref_obj) > tol:
                        failures.append(
                            f"[{mode_name}] {got['id']}: obj goblin={got['obj']:.10g} vs scipy={ref_obj:.10g}"
                        )

        print(
            f"Differential test complete: {args.instances} instances x 2 modes (scalar, simd), {len(failures)} failures."
        )
        if failures:
            for f in failures[:20]:
                print("  FAIL:", f, file=sys.stderr)
            sys.exit(1)


if __name__ == "__main__":
    main()
