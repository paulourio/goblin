#!/usr/bin/env python3
"""Differential test of gopt against COIN-OR CBC on randomly generated MILPs.

Each run generates instances from four families (mixed general MILPs, big-M fixed-charge
facility location with tiny demands, equality-constrained integer knapsacks, and badly
scaled MILPs), solves them with CBC through PuLP, solves them with gopt in three option
configurations through ``go run ./cmd/gopt-difftest``, and compares the outcomes.

CBC is not treated as infallible. When gopt and CBC disagree, gopt's solution is checked
for feasibility in PuLP's own model and CBC is re-run with preprocessing disabled; the
disagreement is classified as a CBC defect only if gopt's solution is feasible and at
least as good as every CBC answer. Any other disagreement is a gopt failure and makes the
script exit with status 1.

CBC is located through PuLP's bundled binary (PULP_CBC_CMD), the CBC_BIN environment
variable, or ``cbc`` on PATH.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import random
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

import pulp

FEAS_TOL = 1e-6
OBJ_RTOL = 1e-5
MODES: dict[str, list[str]] = {
    "default": [],
    "bare": ["-no-heuristics", "-no-cuts"],
    "bare-no-presolve": ["-no-heuristics", "-no-cuts", "-no-presolve"],
}


def make_cbc(options: list[str] | None = None) -> Any:
    """Return a single-threaded CBC solver with a 60 s time limit."""
    opts = options or []
    bundled = getattr(pulp, "PULP_CBC_CMD", None)
    if bundled is not None:
        solver = bundled(msg=False, threads=1, timeLimit=60, options=opts)
        if solver.available():
            return solver
    path = os.environ.get("CBC_BIN") or shutil.which("cbc")
    if not path:
        sys.exit("CBC not found: install PuLP with its bundled CBC or set CBC_BIN")
    return pulp.COIN_CMD(path=path, msg=False, threads=1, timeLimit=60, options=opts)


def fam_general(rng: random.Random, k: int) -> pulp.LpProblem:
    """Mixed MILP with mixed signs and senses, negative integer bounds, free continuous vars."""
    n, m = rng.randint(4, 18), rng.randint(2, 10)
    prob = pulp.LpProblem(f"gen{k}", rng.choice([pulp.LpMinimize, pulp.LpMaximize]))
    xs = []
    for j in range(n):
        kind = rng.random()
        if kind < 0.35:
            v = pulp.LpVariable(f"x{j}", 0, 1, cat="Integer")
        elif kind < 0.7:
            lo = rng.choice([-5, -2, 0, 0, 1])
            v = pulp.LpVariable(f"x{j}", lo, lo + rng.randint(1, 9), cat="Integer")
        else:
            v = pulp.LpVariable(
                f"x{j}", rng.choice([None, -3.5, 0, 0]), rng.choice([None, 7.25, 12.0])
            )
        xs.append(v)
    prob += pulp.lpSum(rng.randint(-9, 9) * v for v in xs)
    for i in range(m):
        terms = [
            (rng.randint(-8, 8) + rng.choice([0, 0, 0.5, 0.25]), v)
            for v in xs
            if rng.random() < 0.6
        ]
        terms = [(a, v) for a, v in terms if a != 0]
        if not terms:
            continue
        e = pulp.lpSum(a * v for a, v in terms)
        rhs = rng.randint(-10, 25) + rng.choice([0, 0, 0.5])
        s = rng.random()
        if s < 0.45:
            prob += e <= rhs, f"c{i}"
        elif s < 0.85:
            prob += e >= rhs - 15, f"c{i}"
        else:
            prob += e == rhs, f"c{i}"
    for j, v in enumerate(xs):
        if v.lowBound is None or v.upBound is None:
            prob += v <= 50, f"bnd_up_{j}"
            prob += v >= -50, f"bnd_lo_{j}"
    return prob


def fam_bigm(rng: random.Random, k: int) -> pulp.LpProblem:
    """Fixed-charge facility location with large big-M and small demands."""
    nf, nc = rng.randint(3, 6), rng.randint(3, 8)
    big_m = rng.choice([1e4, 1e5, 1e6])
    prob = pulp.LpProblem(f"bigm{k}", pulp.LpMinimize)
    y = [pulp.LpVariable(f"y{i}", cat="Binary") for i in range(nf)]
    x = [[pulp.LpVariable(f"x{i}_{j}", 0) for j in range(nc)] for i in range(nf)]
    fixed = [rng.randint(5, 40) for _ in range(nf)]
    cost = [[rng.randint(1, 20) for _ in range(nc)] for _ in range(nf)]
    dem = [rng.choice([0.01, 0.05, 0.2, 1.0, 3.0]) for _ in range(nc)]
    prob += pulp.lpSum(fixed[i] * y[i] for i in range(nf)) + pulp.lpSum(
        cost[i][j] * x[i][j] for i in range(nf) for j in range(nc)
    )
    for j in range(nc):
        prob += pulp.lpSum(x[i][j] for i in range(nf)) >= dem[j], f"d{j}"
    for i in range(nf):
        prob += pulp.lpSum(x[i][j] for j in range(nc)) - big_m * y[i] <= 0, f"vub{i}"
    return prob


def fam_eqknap(rng: random.Random, k: int) -> pulp.LpProblem:
    """Equality-constrained general-integer knapsack (often infeasible or tight)."""
    n = rng.randint(4, 9)
    prob = pulp.LpProblem(f"eqk{k}", pulp.LpMinimize)
    x = [
        pulp.LpVariable(f"x{j}", 0, rng.randint(2, 6), cat="Integer") for j in range(n)
    ]
    a = [rng.randint(3, 40) for _ in range(n)]
    prob += pulp.lpSum(rng.randint(1, 15) * v for v in x)
    prob += pulp.lpSum(a[j] * x[j] for j in range(n)) == rng.randint(30, 150), "eq"
    if rng.random() < 0.5:
        b = [rng.randint(-10, 10) for _ in range(n)]
        prob += pulp.lpSum(b[j] * x[j] for j in range(n)) <= rng.randint(0, 20), "side"
    return prob


def fam_scaled(rng: random.Random, k: int) -> pulp.LpProblem:
    """Badly scaled MILP with row coefficients spanning 1e-3 to 1e4."""
    n, m = rng.randint(5, 12), rng.randint(3, 7)
    prob = pulp.LpProblem(f"scl{k}", pulp.LpMaximize)
    x = [
        pulp.LpVariable(
            f"x{j}",
            0,
            rng.choice([1, 10, 100]),
            cat=rng.choice(["Integer", "Continuous"]),
        )
        for j in range(n)
    ]
    prob += pulp.lpSum(rng.uniform(0.1, 10) * v for v in x)
    for i in range(m):
        scale = 10 ** rng.uniform(-3, 4)
        prob += (
            pulp.lpSum(round(rng.uniform(0, 5) * scale, 6) * v for v in x)
            <= round(rng.uniform(5, 50) * scale, 6),
            f"c{i}",
        )
    return prob


FAMILIES = [fam_general, fam_general, fam_bigm, fam_eqknap, fam_scaled]


def solve_cbc(
    doc: dict[str, Any], options: list[str] | None = None
) -> tuple[str, float | None]:
    _, prob = pulp.LpProblem.fromDict(doc)
    prob.solve(make_cbc(options))
    status = pulp.LpStatus[prob.status]
    obj = pulp.value(prob.objective) if prob.status == pulp.LpStatusOptimal else None
    return status, (
        float(obj)
        if obj is not None
        else (0.0 if prob.status == pulp.LpStatusOptimal else None)
    )


def feasible_in_pulp(doc: dict[str, Any], x: dict[str, float]) -> tuple[bool, float]:
    """Evaluate a solution in PuLP's model; return (feasible, objective)."""
    _, prob = pulp.LpProblem.fromDict(doc)
    for name, var in prob.variablesDict().items():
        val = x.get(name, 0.0)
        var.varValue = val
        if var.lowBound is not None and val < var.lowBound - FEAS_TOL:
            return False, math.nan
        if var.upBound is not None and val > var.upBound + FEAS_TOL:
            return False, math.nan
        if var.cat == pulp.LpInteger and abs(val - round(val)) > FEAS_TOL:
            return False, math.nan
    for con in prob.constraints.values():
        slack = con.value()
        if (con.sense == pulp.LpConstraintLE and slack > FEAS_TOL) or (
            con.sense == pulp.LpConstraintGE and slack < -FEAS_TOL
        ):
            return False, math.nan
        if con.sense == pulp.LpConstraintEQ and abs(slack) > FEAS_TOL:
            return False, math.nan
    return True, float(pulp.value(prob.objective) or 0.0)


def better_or_equal(a: float, b: float, sense: int) -> bool:
    tol = OBJ_RTOL * max(1.0, abs(b))
    return a <= b + tol if sense == pulp.LpMinimize else a >= b - tol


def classify(ref: dict[str, Any], got: dict[str, Any]) -> str:
    """Return 'ok', 'cbc-defect', or a description of a gopt failure."""
    cs, gs = ref["status"], got["status"]
    if gs == "Optimal" and cs == "Optimal":
        if abs(got["obj"] - ref["obj"]) <= OBJ_RTOL * max(1.0, abs(ref["obj"])):
            if max(got["bviol"], got["rviol"], got["iviol"]) <= 1e-5:
                return "ok"
            return f"gopt solution violates the model (b={got['bviol']:.1e} r={got['rviol']:.1e})"
    elif gs == cs:
        return "ok"
    if gs != "Optimal":
        return f"status gopt={gs} cbc={cs}"
    feasible, obj = feasible_in_pulp(ref["doc"], got["x"])
    if not feasible:
        return f"gopt Optimal {got['obj']:.6g} is infeasible in PuLP (cbc={cs} {ref['obj']})"
    sense = ref["doc"]["parameters"]["sense"]
    answers = [(cs, ref["obj"])]
    answers.append(solve_cbc(ref["doc"], ["preprocess off", "presolve off"]))
    for status, cbc_obj in answers:
        if (
            status == "Optimal"
            and cbc_obj is not None
            and not better_or_equal(obj, cbc_obj, sense)
        ):
            return f"objective gopt={obj:.6g} cbc={cbc_obj:.6g} (gopt worse)"
    return "cbc-defect"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--instances", type=int, default=400)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument(
        "--keep", type=Path, help="write generated instances (with CBC results) here"
    )
    args = parser.parse_args()

    root = Path(__file__).resolve().parent.parent
    rng = random.Random(args.seed)
    refs: dict[str, dict[str, Any]] = {}
    with tempfile.TemporaryDirectory(prefix="gopt_difftest_") as tmp:
        inst_path = Path(args.keep) if args.keep else Path(tmp) / "instances.jsonl"
        with inst_path.open("w", encoding="utf-8") as fh:
            for k in range(args.instances):
                fam = FAMILIES[k % len(FAMILIES)]
                doc = fam(rng, k).toDict()
                status, obj = solve_cbc(doc)
                rec = {
                    "id": doc["parameters"]["name"],
                    "fam": fam.__name__,
                    "status": status,
                    "obj": obj,
                    "doc": doc,
                }
                refs[rec["id"]] = rec
                fh.write(json.dumps(rec) + "\n")

        failures = 0
        for mode, flags in MODES.items():
            out = subprocess.run(
                ["go", "run", "./cmd/gopt-difftest", "-in", str(inst_path), *flags],
                cwd=root,
                check=True,
                capture_output=True,
                text=True,
            ).stdout
            tally: dict[str, int] = {}
            for line in out.splitlines():
                got = json.loads(line)
                verdict = classify(refs[got["id"]], got)
                key = verdict if verdict in ("ok", "cbc-defect") else "gopt-failure"
                tally[key] = tally.get(key, 0) + 1
                if key == "cbc-defect":
                    print(
                        f"[{mode}] {got['id']}: CBC disagrees but gopt's solution is verified feasible and optimal-or-better"
                    )
                elif key == "gopt-failure":
                    failures += 1
                    print(f"[{mode}] {got['id']}: FAIL {verdict}")
            print(f"[{mode}] {sum(tally.values())} instances: {tally}")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
