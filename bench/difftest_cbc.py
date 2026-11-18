#!/usr/bin/env python3
"""Differential test of goblin against COIN-OR CBC on randomly generated MILPs.

Each run generates problems from four families: mixed general MILPs, big-M
depot siting, equation-constrained integer knapsacks, and badly scaled MILPs.
The script solves each model with CBC via PuLP and with goblin in three flag
modes via `go run ./cmd/goblin-difftest`, then compares the outcomes.

CBC is not treated as infallible. When goblin and CBC disagree, goblin's primal
vector is checked against all bounds and rows in PuLP's own model, and CBC is
re-run with presolve disabled. A mismatch counts as a CBC defect only when
goblin's vector is feasible and at least as good as every CBC answer. Any other
mismatch is a goblin failure and exits with status 1.

CBC is located through PuLP's bundled binary (PULP_CBC_CMD), the CBC_BIN shell
variable, `cbc` on PATH, or `third_party/Cbc/dist/bin/cbc`.
"""

from __future__ import annotations

import argparse
import glob
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

# Support running directly from the workspace with the sibling PuLP checkout and virtualenv.
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
    cbc_lib_dir = os.path.join(_CBC_DIST_DIR, "lib")
    if os.path.isdir(cbc_lib_dir):
        ld = os.environ.get("LD_LIBRARY_PATH", "")
        if cbc_lib_dir not in ld.split(":"):
            os.environ["LD_LIBRARY_PATH"] = f"{cbc_lib_dir}:{ld}" if ld else cbc_lib_dir

    bundled = getattr(pulp, "PULP_CBC_CMD", None)
    if bundled is not None:
        solver = bundled(
            msg=False, threads=1, timeLimit=60, gapRel=0.0, gapAbs=0.0, options=opts
        )
        if solver.available():
            return solver

    candidates = [
        os.environ.get("CBC_BIN", ""),
        shutil.which("cbc") or "",
        os.path.join(_CBC_DIST_DIR, "bin", "cbc"),
    ]
    for cand in candidates:
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            return pulp.COIN_CMD(
                path=cand,
                msg=False,
                threads=1,
                timeLimit=60,
                gapRel=0.0,
                gapAbs=0.0,
                options=opts,
            )

    sys.exit(
        "CBC not found: install PuLP with bundled CBC, build third_party/Cbc, or set CBC_BIN"
    )


def add_var(
    prob: Any,
    name: str,
    low: float | None = None,
    up: float | None = None,
    cat: str = "Continuous",
) -> Any:
    """Create a decision variable across both PuLP 4.x and legacy PuLP 2.x/3.x."""
    if hasattr(prob, "add_variable"):
        return prob.add_variable(name, low, up, cat=cat)
    return pulp.LpVariable(name, low, up, cat=cat)


def iter_constraints(prob: Any) -> list[Any]:
    """Return the problem constraints list across PuLP 4.x and legacy PuLP dicts."""
    constrs = prob.constraints() if callable(prob.constraints) else prob.constraints
    if isinstance(constrs, dict):
        return list(constrs.values())
    return list(constrs)


def extract_solve_status(prob: Any, solve_ret: Any) -> str:
    """Extract the human-readable status string from PuLP solve results."""
    if hasattr(solve_ret, "status"):
        st = solve_ret.status
        name = str(getattr(solve_ret, "status_str", getattr(st, "name", str(st))))
        return "Optimal" if name == "GapLimit" else name
    code = int(getattr(prob, "status", solve_ret))
    lp_status_map = getattr(pulp, "LpStatus", {})
    return str(lp_status_map.get(code, "Unknown"))


def fam_general(rng: random.Random, k: int) -> Any:
    """Mixed MILP with mixed signs and senses, negative integer bounds, free continuous vars."""
    n, m = rng.randint(4, 18), rng.randint(2, 10)
    prob = pulp.LpProblem(f"gen{k}", rng.choice([pulp.LpMinimize, pulp.LpMaximize]))
    xs = []
    for j in range(n):
        kind = rng.random()
        if kind < 0.35:
            v = add_var(prob, f"x{j}", 0, 1, cat="Integer")
        elif kind < 0.7:
            lo = rng.choice([-5, -2, 0, 0, 1])
            v = add_var(prob, f"x{j}", lo, lo + rng.randint(1, 9), cat="Integer")
        else:
            v = add_var(
                prob,
                f"x{j}",
                rng.choice([None, -3.5, 0, 0]),
                rng.choice([None, 7.25, 12.0]),
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
        if (
            v.lowBound is None
            or not math.isfinite(v.lowBound)
            or v.upBound is None
            or not math.isfinite(v.upBound)
        ):
            prob += v <= 50, f"bnd_up_{j}"
            prob += v >= -50, f"bnd_lo_{j}"
    return prob


def fam_bigm(rng: random.Random, k: int) -> Any:
    """Fixed-charge depot siting with large big-M and small demands."""
    nf, nc = rng.randint(3, 6), rng.randint(3, 8)
    big_m = rng.choice([1e4, 1e5, 1e6])
    prob = pulp.LpProblem(f"bigm{k}", pulp.LpMinimize)
    y = [add_var(prob, f"y{i}", 0, 1, cat="Binary") for i in range(nf)]
    x = [[add_var(prob, f"x{i}_{j}", 0) for j in range(nc)] for i in range(nf)]
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


def fam_eqknap(rng: random.Random, k: int) -> Any:
    """Equation-constrained general-integer knapsack (often infeasible or tight)."""
    n = rng.randint(4, 9)
    prob = pulp.LpProblem(f"eqk{k}", pulp.LpMinimize)
    x = [add_var(prob, f"x{j}", 0, rng.randint(2, 6), cat="Integer") for j in range(n)]
    a = [rng.randint(3, 40) for _ in range(n)]
    prob += pulp.lpSum(rng.randint(1, 15) * v for v in x)
    prob += pulp.lpSum(a[j] * x[j] for j in range(n)) == rng.randint(30, 150), "eq"
    if rng.random() < 0.5:
        b = [rng.randint(-10, 10) for _ in range(n)]
        prob += pulp.lpSum(b[j] * x[j] for j in range(n)) <= rng.randint(0, 20), "side"
    return prob


def fam_scaled(rng: random.Random, k: int) -> Any:
    """Badly scaled MILP with row coefficients spanning 1e-3 to 1e4."""
    n, m = rng.randint(5, 12), rng.randint(3, 7)
    prob = pulp.LpProblem(f"scl{k}", pulp.LpMaximize)
    x = [
        add_var(
            prob,
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
    solve_ret = prob.solve(make_cbc(options))
    status = extract_solve_status(prob, solve_ret)
    if status != "Optimal":
        return status, None
    raw_obj = pulp.value(prob.objective)
    return status, float(raw_obj) if raw_obj is not None else 0.0


def feasible_in_pulp(doc: dict[str, Any], x: dict[str, float]) -> tuple[bool, float]:
    """Evaluate a primal vector in PuLP's model; return (feasible, objective)."""
    _, prob = pulp.LpProblem.fromDict(doc)
    for name, var in prob.variablesDict().items():
        val = x.get(name, 0.0)
        var.varValue = val
        if (
            var.lowBound is not None
            and math.isfinite(var.lowBound)
            and val < var.lowBound - FEAS_TOL
        ):
            return False, math.nan
        if (
            var.upBound is not None
            and math.isfinite(var.upBound)
            and val > var.upBound + FEAS_TOL
        ):
            return False, math.nan
        if (
            var.cat in (pulp.LpInteger, pulp.LpBinary)
            and abs(val - round(val)) > FEAS_TOL
        ):
            return False, math.nan
    for con in iter_constraints(prob):
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
    """Return 'ok', 'cbc-defect', or a summary of a goblin failure."""
    cs, gs = ref["status"], got["status"]
    if gs == "Optimal" and cs == "Optimal":
        if abs(got["obj"] - ref["obj"]) <= OBJ_RTOL * max(1.0, abs(ref["obj"])):
            if max(got["bviol"], got["rviol"], got["iviol"]) <= 1e-5:
                return "ok"
            return f"goblin solution violates the model (b={got['bviol']:.1e} r={got['rviol']:.1e})"
    elif gs == cs:
        return "ok"
    if gs != "Optimal":
        return f"status goblin={gs} cbc={cs}"
    feasible, obj = feasible_in_pulp(ref["doc"], got["x"])
    if not feasible:
        return f"goblin Optimal {got['obj']:.6g} is infeasible in PuLP (cbc={cs} {ref['obj']})"
    sense = ref["doc"]["parameters"]["sense"]
    answers = [(cs, ref["obj"])]
    answers.append(solve_cbc(ref["doc"], ["preprocess off", "presolve off"]))
    for status, cbc_obj in answers:
        if (
            status == "Optimal"
            and cbc_obj is not None
            and not better_or_equal(obj, cbc_obj, sense)
        ):
            return f"objective goblin={obj:.6g} cbc={cbc_obj:.6g} (goblin worse)"
    return "cbc-defect"


def main() -> None:
    parser = argparse.ArgumentParser(description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("--instances", type=int, default=400)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument(
        "--keep", type=Path, help="write generated instances (with CBC results) here"
    )
    args = parser.parse_args()

    root = Path(__file__).resolve().parent.parent
    rng = random.Random(args.seed)
    refs: dict[str, dict[str, Any]] = {}
    with tempfile.TemporaryDirectory(prefix="goblin_difftest_") as tmp:
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
                ["go", "run", "./cmd/goblin-difftest", "-in", str(inst_path), *flags],
                cwd=root,
                check=True,
                capture_output=True,
                text=True,
            ).stdout
            tally: dict[str, int] = {}
            for line in out.splitlines():
                got = json.loads(line)
                verdict = classify(refs[got["id"]], got)
                key = verdict if verdict in ("ok", "cbc-defect") else "goblin-failure"
                tally[key] = tally.get(key, 0) + 1
                if key == "cbc-defect":
                    print(
                        f"[{mode}] {got['id']}: CBC disagrees but goblin's solution is verified feasible and optimal-or-better"
                    )
                elif key == "goblin-failure":
                    failures += 1
                    print(f"[{mode}] {got['id']}: FAIL {verdict}")
            print(f"[{mode}] {sum(tally.values())} instances: {tally}")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
