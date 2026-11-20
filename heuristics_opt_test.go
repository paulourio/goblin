package goblin

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// propagateRowUnguarded mirrors propagateRow without the float64 slack guard so tests
// can verify exact bound-tightening equivalence across adversarial rows.
func propagateRowUnguarded(p *CompiledProblem, i int, lb, ub []float64) (feasible, changed bool) {
	s, e := p.rowPtr[i], p.rowPtr[i+1]
	cols := p.colIdx[s:e]
	vals := p.rowVal[s:e]
	rhs := p.rowRHS[i]
	sense := p.rowSense[i]
	hasUB := sense == LessEqual || sense == Equal
	hasLB := sense == GreaterEqual || sense == Equal

	sum, earlyBail := evalRowActivitySummary(cols, vals, lb, ub, rhs, true, hasUB, hasLB)
	if earlyBail || sum.extreme {
		return true, false
	}

	rowScale := max(1.0, max(sum.l1Min, sum.l1Max))
	rowFeasTol := max(intFeasTol, presolveRelScaleTol*rowScale)

	if hasUB && sum.minInfCount == 0 && sum.slackHi < -rowFeasTol {
		return false, false
	}
	if hasLB && sum.maxInfCount == 0 && sum.deficitLo > rowFeasTol {
		return false, false
	}

	upperChanged := false
	if hasUB && sum.minInfCount == 0 {
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				newUB := qVal + qEB
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / a
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
						upperChanged = true
					}
				}
			} else if a < -heurCoeffZeroTol {
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				absA := -a
				newLB := qVal - qEB
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / absA
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
						upperChanged = true
					}
				}
			}
		}
	}

	if hasLB && upperChanged {
		sum, _ = evalRowActivitySummary(cols, vals, lb, ub, rhs, false, false, true)
		if sum.extreme {
			return true, changed
		}
		rowScale = max(1.0, sum.l1Max)
		rowFeasTol = max(intFeasTol, presolveRelScaleTol*rowScale)
		if sum.maxInfCount == 0 && sum.deficitLo > rowFeasTol {
			return false, false
		}
	}

	if hasLB && sum.maxInfCount == 0 {
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				newLB := qVal - qEB
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / a
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
					}
				}
			} else if a < -heurCoeffZeroTol {
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				absA := -a
				newUB := qVal + qEB
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / absA
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
					}
				}
			}
		}
	}
	return true, changed
}

func TestCanSkipBoundTighteningEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		slackLo     float64
		absA        float64
		l, u        float64
		targetBound float64
		isInt       bool
		want        bool
	}{
		{"negative_slack", -1e-12, 1, 0, 1, 1, true, false},
		{"nan_slack", math.NaN(), 1, 0, 1, 1, true, false},
		{"inf_target", 10, 1, 0, math.Inf(1), math.Inf(1), true, false},
		{"non_int_target_int_var", 10, 1, 0, 3.7, 3.7, true, false},
		{"non_int_target_cont_var", 10, 1, 0, 3.7, 3.7, false, true},
		{"inverted_bounds", 10, 1, 2, 1, 1, true, false},
		{"overflow_span", 10, 1, -1e308, 1e308, 1e308, false, false},
		{"overflow_prod", 1e308, 1e200, 0, 1e200, 1e200, false, false},
		{"tight_unit_binary", 1.0 - 1e-15, 1, 0, 1, 1, true, true},
		{"near_tight_unit_binary", 1.0 - 1e-7, 1, 0, 1, 1, true, false},
		{"strictly_tightenable", 0.5, 1, 0, 1, 1, true, false},
		{"ample_slack_integer", 5.0, 2, 0, 2, 2, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canSkipBoundTightening(tc.slackLo, tc.absA, tc.l, tc.u, tc.targetBound, tc.isInt)
			if got != tc.want {
				t.Fatalf("canSkipBoundTightening(%v, %v, [%v,%v], %v, int=%v) = %v, want %v",
					tc.slackLo, tc.absA, tc.l, tc.u, tc.targetBound, tc.isInt, got, tc.want)
			}
		})
	}
}

func TestPropagateRowSlackGuardEquivalence(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(42, 101))
	senses := []ConstraintSense{LessEqual, GreaterEqual, Equal}
	cats := []VarCategory{Binary, Integer, Continuous}

	for trial := range 200 {
		m := NewModel(fmt.Sprintf("prop_%d", trial), Minimize)
		nVars := 4 + rng.IntN(8)
		vars := make([]Var, nVars)
		lb0 := make([]float64, nVars)
		ub0 := make([]float64, nVars)
		for j := range nVars {
			cat := cats[rng.IntN(len(cats))]
			var l, u float64
			switch cat {
			case Binary:
				l, u = 0, 1
			case Integer:
				if rng.IntN(6) == 0 {
					l, u = -0.3, 3.7
				} else {
					l = float64(rng.IntN(5) - 2)
					u = l + float64(1+rng.IntN(6))
				}
			default:
				l = float64(rng.IntN(7)-3) * 0.5
				u = l + float64(1+rng.IntN(10))*0.5
				if rng.IntN(8) == 0 {
					u = math.Inf(1)
				}
			}
			vars[j] = m.AddVar(fmt.Sprintf("x%d", j), l, u, cat)
			lb0[j], ub0[j] = l, u
		}
		expr := NewExpr()
		for j := range nVars {
			coeff := float64(rng.IntN(11)-5) * 0.5
			if coeff == 0 {
				coeff = 1.0
			}
			expr = expr.AddTerm(vars[j], coeff)
		}
		rhs := float64(rng.IntN(15)-5) * 0.5
		sense := senses[rng.IntN(len(senses))]
		m.AddConstraint("r0", Constraint{Expr: expr, Sense: sense, RHS: rhs})
		p := m.Compile()

		lbGot, ubGot := slices.Clone(lb0), slices.Clone(ub0)
		lbRef, ubRef := slices.Clone(lb0), slices.Clone(ub0)
		feasGot, chgGot := propagateRow(p, 0, lbGot, ubGot, nil)
		feasRef, chgRef := propagateRowUnguarded(p, 0, lbRef, ubRef)
		if feasGot != feasRef || chgGot != chgRef || !slices.Equal(lbGot, lbRef) || !slices.Equal(ubGot, ubRef) {
			t.Fatalf("trial %d mismatch: got (feas=%v chg=%v lb=%v ub=%v), want (feas=%v chg=%v lb=%v ub=%v)",
				trial, feasGot, chgGot, lbGot, ubGot, feasRef, chgRef, lbRef, ubRef)
		}
	}

	// Verify 1e17 cancellation row never bypasses exact wide-accumulator tightening.
	mCancel := NewModel("prop_cancel_1e17", Maximize)
	x0 := mCancel.AddContVar("x0", 1, 1)
	x1 := mCancel.AddContVar("x1", 1, 1)
	z := mCancel.AddIntVar("z", 0, 10)
	mCancel.SetObjective(VarExpr(z))
	mCancel.AddConstraint("c", NewExpr(x0.T(1e17), x1.T(-1e17), z.T(7)).Le(0))
	pCancel := mCancel.Compile()
	lbC := slices.Clone(pCancel.varLB)
	ubC := slices.Clone(pCancel.varUB)
	feasC, chgC := propagateRow(pCancel, 0, lbC, ubC, nil)
	if !feasC || !chgC || ubC[2] != 0 {
		t.Fatalf("1e17 cancellation propagation failed: feas=%v chg=%v ub[z]=%v, want true/true/0", feasC, chgC, ubC[2])
	}
}

func TestSolveFixedIntContinuousLPBatchFixing(t *testing.T) {
	t.Parallel()

	// Build a lot-sizing style problem whose free integer production variables are
	// integral at the initial LP optimum so all redundant re-solves are skipped.
	const periods = 40
	m := NewModel("batch_fix_all_integral", Minimize)
	y := make([]Var, periods)
	q := make([]Var, periods)
	inv := make([]Var, periods)
	obj := NewExpr()
	for tIdx := range periods {
		y[tIdx] = m.AddBinaryVar(fmt.Sprintf("y_%d", tIdx))
		q[tIdx] = m.AddIntVar(fmt.Sprintf("q_%d", tIdx), 0, 20)
		inv[tIdx] = m.AddContVar(fmt.Sprintf("inv_%d", tIdx), 0, 50)
		obj = obj.AddTerm(y[tIdx], 10).AddTerm(q[tIdx], 2+float64(tIdx%3)).AddTerm(inv[tIdx], 0.5)
	}
	m.SetObjective(obj)
	for tIdx := range periods {
		m.AddConstraint(fmt.Sprintf("cap_%d", tIdx), NewExpr(q[tIdx].T(1), y[tIdx].T(-20)).Le(0))
		bal := NewExpr(q[tIdx].T(1), inv[tIdx].T(-1))
		if tIdx > 0 {
			bal = bal.AddTerm(inv[tIdx-1], 1)
		}
		m.AddConstraint(fmt.Sprintf("bal_%d", tIdx), bal.Eq(float64(3+tIdx%4)))
	}
	p := m.Compile()
	hlb := slices.Clone(p.varLB)
	hub := slices.Clone(p.varUB)
	for tIdx := range periods {
		col := y[tIdx].Index()
		hlb[col] = 1
		hub[col] = 1
	}
	aux := NewLPSolver(p, nil)
	defer aux.Close()

	sol, objMin, ok := solveFixedIntContinuousLP(p, hlb, hub, aux, findContLinkedIntCols(p))
	if !ok {
		t.Fatalf("solveFixedIntContinuousLP returned ok=false on feasible lot-sizing subproblem")
	}
	bV, rV, iV := p.MaxViolation(sol, true)
	if bV > intFeasTol || rV > intFeasTol || iV > intFeasTol {
		t.Fatalf("batch-fixed solution violates feasibility: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
	}
	if math.IsNaN(objMin) || math.IsInf(objMin, 0) {
		t.Fatalf("invalid objective %v", objMin)
	}

	// Build a mixed case with both already-integral and fractional free integers where
	// the lower branch (lo) beats the upper branch (hi).
	mFrac := NewModel("batch_fix_mixed_frac", Minimize)
	bFix := mFrac.AddBinaryVar("b")
	iInt := mFrac.AddIntVar("i_int", 0, 10)
	iFrac := mFrac.AddIntVar("i_frac", 0, 10)
	cVar := mFrac.AddContVar("c", 0, 100)
	mFrac.SetObjective(NewExpr(bFix.T(1), iInt.T(2), iFrac.T(3), cVar.T(1)))
	mFrac.AddConstraint("c1", NewExpr(iInt.T(1), cVar.T(1)).Ge(5))
	mFrac.AddConstraint("c2", NewExpr(iFrac.T(2), cVar.T(1)).Ge(3))
	mFrac.AddConstraint("c3", NewExpr(iFrac.T(2), cVar.T(-1)).Le(3))
	mFrac.AddConstraint("c4", NewExpr(bFix.T(1), cVar.T(0.1)).Ge(1))
	pFrac := mFrac.Compile()
	flb := slices.Clone(pFrac.varLB)
	fub := slices.Clone(pFrac.varUB)
	flb[0], fub[0] = 1, 1
	auxFrac := NewLPSolver(pFrac, nil)
	defer auxFrac.Close()
	solF, _, okF := solveFixedIntContinuousLP(pFrac, flb, fub, auxFrac, findContLinkedIntCols(pFrac))
	if !okF {
		t.Fatalf("solveFixedIntContinuousLP failed on mixed integral+fractional subproblem")
	}
	bV, rV, iV = pFrac.MaxViolation(solF, true)
	if bV > intFeasTol || rV > intFeasTol || iV > intFeasTol {
		t.Fatalf("mixed solution infeasible: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
	}
}

func TestSubMIPNoIncumbentCapAndBufferReuse(t *testing.T) {
	t.Parallel()

	// Build a multi-dimensional 0-1 knapsack with a strict LP-IP gap and non-trivial
	// branch-and-bound tree so sub-MIP mode hits subMIPNoIncumbentNodeCap cleanly.
	m := NewModel("submip_no_incumbent", Minimize)
	const n = 24
	vars := make([]Var, n)
	obj := NewExpr()
	cap0 := NewExpr()
	cap1 := NewExpr()
	cap2 := NewExpr()
	for j := range n {
		vars[j] = m.AddBinaryVar(fmt.Sprintf("x_%d", j))
		obj = obj.AddTerm(vars[j], -float64(10+(j*3)%7))
		cap0 = cap0.AddTerm(vars[j], float64(7+(j*5)%11))
		cap1 = cap1.AddTerm(vars[j], float64(6+(j*7)%13))
		cap2 = cap2.AddTerm(vars[j], float64(5+(j*11)%9))
	}
	m.SetObjective(obj)
	m.AddConstraint("cap0", cap0.Le(85))
	m.AddConstraint("cap1", cap1.Le(83))
	m.AddConstraint("cap2", cap2.Le(71))
	p := m.Compile()

	// Solve once to obtain the true integer optimum, then invoke solveBranchAndCutWithDone
	// in sub-MIP mode with a cutoff strictly between the LP relaxation and the integer
	// optimum so the root cannot prune and no improving incumbent exists.
	optRef, err := SolveCompiled(t.Context(), p)
	if err != nil || optRef.Status != StatusOptimal {
		t.Fatalf("reference solve failed: status=%v err=%v", optRef.Status, err)
	}
	if optRef.RootLPBound >= optRef.Objective-1e-3 {
		t.Fatalf("expected non-zero root LP gap, got rootLP=%.6f opt=%.6f", optRef.RootLPBound, optRef.Objective)
	}

	opts := DefaultOptions()
	opts.isSubMIP = true
	opts.hasCutoff = true
	opts.cutoffMin = 0.5 * (optRef.RootLPBound + optRef.Objective)
	opts.MaxNodes = heurSubMIPMaxNodes
	opts.EnablePresolve = false
	opts.EnableCuts = false
	opts.EnableHeuristics = false
	res := solveBranchAndCutWithDone(p, opts, time.Now(), time.Time{}, nil)
	if res.Status != StatusNodeLimit {
		t.Fatalf("sub-MIP with unattainable cutoff status = %v, want StatusNodeLimit", res.Status)
	}
	if res.Nodes != subMIPNoIncumbentNodeCap {
		t.Fatalf("sub-MIP explored %d nodes without an incumbent, want %d", res.Nodes, subMIPNoIncumbentNodeCap)
	}

	// Verify selectBranchVarWithBuf matches selectBranchVar and reuses its slice buffer.
	lp := NewLPSolver(p, nil)
	defer lp.Close()
	lpRes := lp.Solve()
	if lpRes.Status != StatusOptimal {
		t.Fatalf("root LP status = %v, want Optimal", lpRes.Status)
	}
	curLB := slices.Clone(p.varLB)
	curUB := slices.Clone(p.varUB)
	pCosts := make([]pseudoCost, p.numVars)
	for _, col := range p.intCols {
		pCosts[col] = pseudoCost{downSum: 1, upSum: 1, downCount: 2, upCount: 2}
	}
	var buf []branchCand
	for _, depth := range []int{0, 1, strongBranchMaxDepth + 1} {
		itersA, itersB := 0, 0
		colA, valA := selectBranchVar(p, lp, lpRes, curLB, curUB, pCosts, depth, &itersA, func() int { return 1000 })
		colB, valB := selectBranchVarWithBuf(p, lp, lpRes, curLB, curUB, pCosts, depth, &itersB, func() int { return 1000 }, &buf)
		if colA != colB || valA != valB || itersA != itersB {
			t.Fatalf("depth %d mismatch: got (%d,%v,%d), want (%d,%v,%d)", depth, colB, valB, itersB, colA, valA, itersA)
		}
	}
	warmCap := cap(buf)
	for range 10 {
		iters := 0
		selectBranchVarWithBuf(p, lp, lpRes, curLB, curUB, pCosts, 0, &iters, func() int { return 1000 }, &buf)
	}
	if cap(buf) != warmCap {
		t.Fatalf("sbCands buffer reallocated: cap grew from %d to %d", warmCap, cap(buf))
	}

	// Verify WithCuts(false) still solves cleanly via pre-cut heuristics and tree search.
	resNoCut, err := SolveCompiled(t.Context(), p, WithCuts(false))
	if err != nil || resNoCut.Status != StatusOptimal {
		t.Fatalf("SolveCompiled(WithCuts(false)) = (%v, %v), want Optimal", resNoCut.Status, err)
	}
	resCut, err := SolveCompiled(t.Context(), p, WithCuts(true))
	if err != nil || resCut.Status != StatusOptimal {
		t.Fatalf("SolveCompiled(WithCuts(true)) = (%v, %v), want Optimal", resCut.Status, err)
	}
	if math.Abs(resNoCut.Objective-resCut.Objective) > 1e-6 {
		t.Fatalf("cuts vs no-cuts objective mismatch: %.6f vs %.6f", resCut.Objective, resNoCut.Objective)
	}
}

func TestTryFractionalDivingBranch1Reuse(t *testing.T) {
	t.Parallel()

	// Construct an equality-supported model whose root LP forces x0 = 0.5 (obj = 0),
	// where fixing x0 = 1 (branch 1) raises the LP objective to 1 and fixing x0 = 0
	// (branch 2) raises it to 5, exercising the branch-1 reuse path without a third solve.
	m := NewModel("dive_branch1_wins", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddBinaryVar("x1")
	x2 := m.AddBinaryVar("x2")
	x3 := m.AddBinaryVar("x3")
	y := m.AddContVar("y", 0, 10)
	z := m.AddContVar("z", 0, 10)
	m.SetObjective(NewExpr(y.T(1), z.T(5)))
	m.AddConstraint("eq0", NewExpr(x0.T(1), x1.T(1), x2.T(1)).Eq(2))
	m.AddConstraint("eq1", NewExpr(x0.T(1), x3.T(1)).Eq(1))
	m.AddConstraint("up_pen", NewExpr(x0.T(2), y.T(-1)).Le(1))
	m.AddConstraint("dn_pen", NewExpr(x0.T(2), z.T(1)).Ge(1))
	p := m.Compile()

	lp := NewLPSolver(p, nil)
	defer lp.Close()
	rootRes := lp.Solve()
	if rootRes.Status != StatusOptimal {
		t.Fatalf("root LP status = %v, want Optimal", rootRes.Status)
	}
	if math.Abs(rootRes.Primal[x0.Index()]-0.5) > 1e-6 {
		t.Fatalf("root LP x0 = %v, want 0.5", rootRes.Primal[x0.Index()])
	}
	diveLP := NewLPSolver(p, nil)
	defer diveLP.Close()
	diveLP.CopyRootBasisFrom(lp)

	sol, objMin, ok := tryFractionalDiving(p, rootRes.Primal, p.varLB, p.varUB, diveLP, math.Inf(1), true, findContLinkedIntCols(p))
	if !ok {
		t.Fatalf("tryFractionalDiving returned ok=false on feasible equality-supported model")
	}
	bV, rV, iV := p.MaxViolation(sol, true)
	if bV > incumbentTol || rV > incumbentTol || iV > incumbentTol {
		t.Fatalf("diving solution infeasible: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
	}
	optRes, err := SolveCompiled(t.Context(), p, WithHeuristics(false))
	if err != nil || optRes.Status != StatusOptimal {
		t.Fatalf("reference solve failed: status=%v err=%v", optRes.Status, err)
	}
	if math.Abs(objMin-optRes.Objective) > 1e-6 {
		t.Fatalf("diving obj %.6f != optimal %.6f", objMin, optRes.Objective)
	}
}
