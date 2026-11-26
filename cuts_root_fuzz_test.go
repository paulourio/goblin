package goblin_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

func runRelaxCutDynamismDirectRegime(tb testing.TB, cur *certFuzzCursor) {
	tb.Helper()
	n := cur.intRange(3, 6)
	m := gb.NewModel("relax_dyn_direct", gb.Minimize)
	xs := make([]gb.Var, n)
	lbs := make([]float64, n)
	ubs := make([]float64, n)
	for j := range n {
		var lb, ub float64
		switch cur.u8() % 5 {
		case 0:
			lb, ub = 0, 1
			xs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
		case 1:
			lb, ub = 0, float64(cur.intRange(2, 8))
			xs[j] = m.AddContVar("c"+strconv.Itoa(j), lb, ub)
		case 2:
			lb, ub = -float64(cur.intRange(1, 5)), float64(cur.intRange(1, 5))
			xs[j] = m.AddIntVar("i"+strconv.Itoa(j), lb, ub)
		case 3:
			// Wide upper bound exercising maxCutDynamismRelaxWeak cap rejection.
			lb, ub = 0, 1e6
			xs[j] = m.AddContVar("w"+strconv.Itoa(j), lb, ub)
		default:
			// Semi-infinite bound exercising relaxCutRHSForTerm rejection.
			lb, ub = 0, math.Inf(1)
			xs[j] = m.AddContVar("u"+strconv.Itoa(j), lb, ub)
		}
		lbs[j], ubs[j] = lb, ub
	}
	m.SetObjective(gb.NewExpr(xs[0].T(1)))
	m.AddConstraint("dummy", gb.Ge(gb.NewExpr(xs[0].T(1), xs[1].T(1)), 0.5))
	p := m.Compile()

	// Build cut coefficients crossing maxCutDynamism (1e5), maxCutDynamismRelaxTerm (1e-6),
	// maxCutDynamismRelaxWeak (1e-4), zero entries, and cutDropTol (1e-11).
	coeffChoices := []float64{
		0, 1e-12, -1e-12, 5e-10, -5e-10, 2e-9, -2e-9,
		5e-8, -5e-8, 9e-7, -9e-7, 5e-6, -5e-6,
		1e-4, -1e-4, 0.5, -0.5, 1.0, -1.0, 2.5, -3.0, 10.0,
	}
	idx := make([]int, 0, n)
	val := make([]float64, 0, n)
	for j := range n {
		c := cur.pickFloat(coeffChoices)
		if j == 0 && math.Abs(c) < 0.5 {
			c = 1.0
		}
		idx = append(idx, j)
		val = append(val, c)
	}
	rhs := cur.pickFloat([]float64{-2.0, -0.5, 0.0, 0.5, 1.5, 5.0})

	// Build a sample of box-feasible points (using finite corners or 0/10 on unbounded vars)
	// and an LP point violating the cut so finalizeSparseCut also runs.
	var feasPts [][]float64
	for pt := range 8 {
		x := make([]float64, n)
		for j := range n {
			hi := ubs[j]
			if math.IsInf(hi, 1) {
				hi = 10.0
			}
			lo := lbs[j]
			if math.IsInf(lo, -1) {
				lo = -10.0
			}
			if (pt>>uint(j%3))&1 == 1 {
				x[j] = hi
			} else {
				x[j] = lo
			}
		}
		feasPts = append(feasPts, x)
	}
	lpPrimal := make([]float64, n)
	for j := range n {
		if math.IsInf(ubs[j], 1) {
			lpPrimal[j] = 1.0
		} else {
			lpPrimal[j] = 0.5*(lbs[j]+ubs[j]) + 0.4*(ubs[j]-lbs[j])
		}
	}

	if _, _, err := gb.ExerciseRelaxCutDynamismForTest(p, idx, val, rhs, lpPrimal, feasPts); err != nil {
		tb.Fatalf("ExerciseRelaxCutDynamismForTest failed: %v", err)
	}

	// Also exercise duplicate-column twoSum merging inside finalizeSparseCut (cuts.go:2784-2794).
	if cur.u8()%2 == 0 {
		dupIdx := []int{0, 0, 1}
		dupVal := []float64{1.0, 1e-16 + cur.pickFloat([]float64{0, 0.25, -0.25}), cur.pickFloat([]float64{0.5, 1.0, -0.5})}
		if _, _, err := gb.ExerciseRelaxCutDynamismForTest(p, dupIdx, dupVal, rhs, lpPrimal, feasPts); err != nil {
			tb.Fatalf("ExerciseRelaxCutDynamismForTest duplicate-col failed: %v", err)
		}
	}
}

func buildBinpackSpeculativeCutMIP(cur *certFuzzCursor, compact bool) *gb.CompiledProblem {
	// Pure-integer bin-packing / multi-knapsack MIP with fractional capacities/weights and
	// structural row count > extra cut count to trigger speculative root-cut lookahead + rollback.
	numBins := 4
	numItems := 7
	if compact {
		numBins = 3
		numItems = 5
	}
	m := gb.NewModel("spec_binpack", gb.Minimize)
	y := make([]gb.Var, numBins)
	x := make([][]gb.Var, numItems)
	objTerms := make([]gb.Term, 0, numBins+numItems*numBins)
	binCosts := []float64{10.1, 10.2, 10.319, 10.619}
	for b := range numBins {
		y[b] = m.AddBinaryVar("y" + strconv.Itoa(b))
		objTerms = append(objTerms, gb.T(y[b], binCosts[b%len(binCosts)]))
	}
	for i := range numItems {
		x[i] = make([]gb.Var, numBins)
		assignTerms := make([]gb.Term, numBins)
		for b := range numBins {
			x[i][b] = m.AddBinaryVar("x" + strconv.Itoa(i) + "_" + strconv.Itoa(b))
			assignTerms[b] = gb.T(x[i][b], 1)
			if cur.u8()%5 == 0 {
				objTerms = append(objTerms, gb.T(x[i][b], 0.01*float64(b+1)))
			}
		}
		m.AddConstraint("assign_"+strconv.Itoa(i), gb.Eq(gb.NewExpr(assignTerms...), 1))
	}

	weights := make([]float64, numItems)
	wPool := []float64{0.23, 0.31, 0.37, 0.41, 0.44, 0.53, 0.61, 0.67}
	for i := range numItems {
		weights[i] = cur.pickFloat(wPool)
		if cur.u8()%3 == 0 {
			// Inject subtle scaling perturbation that induces tiny tableau cut coefficients.
			weights[i] += cur.pickFloat([]float64{1e-7, -1e-7, 1e-6, 0})
		}
	}
	capVal := cur.pickFloat([]float64{1.0, 1.05, 1.15, 1.25})
	for b := range numBins {
		terms := make([]gb.Term, 0, numItems+1)
		for i := range numItems {
			terms = append(terms, gb.T(x[i][b], weights[i]))
			if cur.u8()%2 == 0 {
				m.AddConstraint("vub_"+strconv.Itoa(i)+"_"+strconv.Itoa(b),
					gb.Le(gb.NewExpr(gb.T(x[i][b], 1), gb.T(y[b], -1)), 0))
			}
		}
		terms = append(terms, gb.T(y[b], -capVal))
		m.AddConstraint("cap_"+strconv.Itoa(b), gb.Le(gb.NewExpr(terms...), 0))
	}
	// Symmetry-breaking chain y[b] >= y[b+1] keeps search fast while stressing GMI rounds.
	for b := range numBins - 1 {
		m.AddConstraint("sym_"+strconv.Itoa(b), gb.Ge(gb.NewExpr(gb.T(y[b], 1), gb.T(y[b+1], -1)), 0))
	}
	m.SetObjective(gb.NewExpr(objTerms...))
	return m.Compile()
}

func buildMixedScaleGMIMIP(cur *certFuzzCursor) *gb.CompiledProblem {
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("mixed_gmi_cuts", sense)
	n := cur.intRange(4, 7)
	xs := make([]gb.Var, n)
	for j := range n {
		switch cur.u8() % 5 {
		case 0:
			xs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
		case 1:
			xs[j] = m.AddIntVar("i"+strconv.Itoa(j), 0, float64(cur.intRange(2, 6)))
		case 2:
			// Fixed integer variable exercising cMIR fixed-bound observer path.
			fv := float64(cur.intRange(1, 3))
			xs[j] = m.AddIntVar("f"+strconv.Itoa(j), fv, fv)
		case 3:
			if j == n-1 {
				xs[j] = m.AddContVar("c"+strconv.Itoa(j), 0, float64(cur.intRange(2, 10)))
			} else {
				xs[j] = m.AddIntVar("i"+strconv.Itoa(j), 0, float64(cur.intRange(1, 5)))
			}
		default:
			if j == 0 && cur.u8()%3 == 0 {
				// Doubly-unbounded integer variable exercising cMIR exact-multiple branch.
				xs[j] = m.AddIntVar("free"+strconv.Itoa(j), math.Inf(-1), math.Inf(1))
			} else {
				xs[j] = m.AddIntVar("i"+strconv.Itoa(j), 0, float64(cur.intRange(1, 5)))
			}
		}
	}
	mRows := cur.intRange(3, 6)
	for i := range mRows {
		terms := make([]gb.Term, 0, n)
		for j := range n {
			c := cur.pickFloat([]float64{-3, -2, -1, 0, 1, 2, 3, 5, 0.5, 1.5, 1e-6, -1e-6})
			if c != 0 {
				terms = append(terms, gb.T(xs[j], c))
			}
		}
		if len(terms) == 0 {
			terms = append(terms, gb.T(xs[i%n], 2))
		}
		rhs := cur.pickFloat([]float64{-1.5, 0.5, 1.5, 2.5, 4.5, 7.5, 9.0})
		switch cur.u8() % 3 {
		case 0:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), rhs))
		case 1:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(gb.NewExpr(terms...), rhs))
		default:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(gb.NewExpr(terms...), rhs))
		}
	}
	objTerms := make([]gb.Term, 0, n)
	for j := range n {
		c := float64(cur.intRange(-7, 7))
		if c != 0 {
			objTerms = append(objTerms, gb.T(xs[j], c))
		}
	}
	m.SetObjective(gb.NewExpr(objTerms...))
	return m.Compile()
}

func assertMIPResultsSoundParity(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	nameA string,
	resA gb.Result,
	nameB string,
	resB gb.Result,
) {
	tb.Helper()
	if (resA.Status != gb.StatusOptimal && resA.Status != gb.StatusInfeasible) ||
		(resB.Status != gb.StatusOptimal && resB.Status != gb.StatusInfeasible) {
		return
	}
	exactA := resA.Status == gb.StatusOptimal && len(resA.Values) == p.NumVars() && isExactRatMIPFeasible(p, resA.Values)
	exactB := resB.Status == gb.StatusOptimal && len(resB.Values) == p.NumVars() && isExactRatMIPFeasible(p, resB.Values)
	if resA.Status != resB.Status {
		// Reject any case where one solver proves Infeasible while the other exhibits an exact-Q feasible point.
		if (resA.Status == gb.StatusInfeasible && exactB) || (resB.Status == gb.StatusInfeasible && exactA) {
			tb.Fatalf("[%s] sound status contradiction: %s=%s (exact=%v) vs %s=%s (exact=%v)",
				label, nameA, resA.Status, exactA, nameB, resB.Status, exactB)
		}
		return
	}
	if resA.Status != gb.StatusOptimal {
		return
	}
	scale := max(1.0, math.Abs(resA.Objective), math.Abs(resB.Objective))
	tol := 1e-4 * scale
	if p.Sense() == gb.Minimize {
		if exactB && resA.Objective > resB.Objective+tol {
			tb.Fatalf("[%s] %s missed exact-feasible better Min solution from %s: %.10g > %.10g",
				label, nameA, nameB, resA.Objective, resB.Objective)
		}
		if exactA && resB.Objective > resA.Objective+tol {
			tb.Fatalf("[%s] %s missed exact-feasible better Min solution from %s: %.10g > %.10g",
				label, nameB, nameA, resB.Objective, resA.Objective)
		}
	} else {
		if exactB && resA.Objective < resB.Objective-tol {
			tb.Fatalf("[%s] %s missed exact-feasible better Max solution from %s: %.10g < %.10g",
				label, nameA, nameB, resA.Objective, resB.Objective)
		}
		if exactA && resB.Objective < resA.Objective-tol {
			tb.Fatalf("[%s] %s missed exact-feasible better Max solution from %s: %.10g < %.10g",
				label, nameB, nameA, resB.Objective, resA.Objective)
		}
	}
}

func checkRootCutsAndTrack2Parity(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	presolve bool,
	maxCutPasses int,
	runCertifiedOracle bool,
) {
	tb.Helper()
	// 1. Uncertified pure-Go solve with root cuts and speculative lookahead enabled.
	resCuts, errCuts := gb.SolveCompiled(tb.Context(), p,
		gb.WithPureGo(),
		gb.WithPresolve(presolve),
		gb.WithCuts(true),
		gb.WithMaxCutPasses(maxCutPasses),
		gb.WithHeuristics(false),
		gb.WithRelGapTol(0),
		gb.WithAbsGapTol(0),
		gb.WithMaxNodes(1200),
		gb.WithTimeLimit(1500*time.Millisecond),
	)
	if errCuts != nil {
		tb.Fatalf("[%s] uncertified cuts solve error: %v", label, errCuts)
	}
	assertAllowedStatus(tb, label+"/uncert_cuts", p, true, resCuts)

	// 2. Compare against cuts-disabled baseline on the same instance.
	resNoCuts, errNoCuts := gb.SolveCompiled(tb.Context(), p,
		gb.WithPureGo(),
		gb.WithPresolve(false),
		gb.WithCuts(false),
		gb.WithRelGapTol(0),
		gb.WithAbsGapTol(0),
		gb.WithMaxNodes(1200),
		gb.WithTimeLimit(1500*time.Millisecond),
	)
	if errNoCuts != nil {
		tb.Fatalf("[%s] no-cuts solve error: %v", label, errNoCuts)
	}
	assertMIPResultsSoundParity(tb, label+"/cuts_vs_nocuts", p, "cuts", resCuts, "noCuts", resNoCuts)

	// 3. Optional CBC parity (active under -tags=cbc).
	compareWithOptionalCBC(tb, label+"/cbc", p, resCuts)

	// 4. Certified exact-rational oracle parity (WithCertified + VIPR) on compact models.
	var sols [][]float64
	if resCuts.Status == gb.StatusOptimal && len(resCuts.Values) == p.NumVars() && isExactRatMIPFeasible(p, resCuts.Values) {
		sols = append(sols, resCuts.Values)
	}
	if runCertifiedOracle {
		tr := gb.NewVIPRTrace()
		var wit gb.CertifiedWitness
		resCert, errCert := gb.SolveCompiled(tb.Context(), p,
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithCertifiedWitness(&wit),
			gb.WithVIPRTrace(tr),
			gb.WithPresolve(presolve),
			gb.WithCuts(true),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
			gb.WithMaxNodes(600),
			gb.WithTimeLimit(1500*time.Millisecond),
		)
		if errCert != nil {
			assertFailClosedResultAndWitness(tb, label+"/cert_fail_closed", resCert, errCert, wit)
		} else {
			assertAllowedStatus(tb, label+"/cert", p, true, resCert)
			verifyWitnessRationalInvariants(tb, label+"/cert", p, resCert, wit, 0, 0, true)
			if resCert.Status == gb.StatusOptimal && len(resCert.Values) == p.NumVars() {
				sols = append(sols, resCert.Values)
			}
			assertMIPResultsSoundParity(tb, label+"/cuts_vs_cert", p, "cuts", resCuts, "cert", resCert)
			if formalFuzzEnabled() && tr.HasCertificate() {
				if errF := checkFormalVIPRResult(tb, label+"/cert_vipr", p, resCert, tr, 0, 0); errF != nil {
					tb.Fatal(errF)
				}
			}
		}
	}

	// 5. Lean Track 2 cuts-check algebraic verification of live separated cuts (including rescued GMI cuts).
	if formalFuzzEnabled() {
		items, errItems := gb.ExportSeparatedCutsItems(p, sols)
		if errItems != nil {
			tb.Fatalf("[%s] ExportSeparatedCutsItems failed: %v", label, errItems)
		}
		for _, item := range items {
			mustFormalCutsCheck(tb, item)
		}
	}
}

func runStepAColdBlockRegime(tb testing.TB, cur *certFuzzCursor) {
	tb.Helper()
	switch cur.u8() % 3 {
	case 0:
		// Cold block 1: bab.go (*parWorker).tryInTreeCuts (numVars >= 120, parallel workers, depth >= 1).
		const n = 124
		m := gb.NewModel("intree_cuts_124", gb.Maximize)
		xs := make([]gb.Var, n)
		objTerms := make([]gb.Term, n)
		for j := range n {
			xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
			objTerms[j] = gb.T(xs[j], float64(5+int(cur.u8()%13)))
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		// 12 coupled multi-knapsack rows creating fractional root LP and shallow B&B tree cuts.
		for i := range 12 {
			terms := make([]gb.Term, 0, 24)
			wSum := 0.0
			for k := range 22 {
				col := (i*10 + k) % n
				w := float64(2+int(cur.u8()%7)) + 0.1*float64(k%3)
				terms = append(terms, gb.T(xs[col], w))
				wSum += w
			}
			m.AddConstraint("mk"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), math.Floor(0.45*wSum)+0.5))
		}
		p := m.Compile()
		res, err := gb.SolveCompiled(tb.Context(), p,
			gb.WithPureGo(),
			gb.WithWorkers(2),
			gb.WithPresolve(false),
			gb.WithCuts(true),
			gb.WithMaxCutPasses(1),
			gb.WithHeuristics(false),
			gb.WithMaxNodes(60),
			gb.WithTimeLimit(300*time.Millisecond),
		)
		if err != nil {
			tb.Fatalf("[intree_cuts_124] SolveCompiled error: %v", err)
		}
		assertAllowedStatus(tb, "intree_cuts_124", p, true, res)

	case 1:
		// Cold block 2: exact_polish.go unbounded MILP recession ray + solveExactFeasNodeLP and bab.go solveUnboundedMIPFeasibility.
		sense := gb.Minimize
		if cur.u8()%2 == 1 {
			sense = gb.Maximize
		}
		m := gb.NewModel("unb_mip_cold", sense)
		x0 := m.AddIntVar("x0", 0, math.Inf(1))
		x1 := m.AddIntVar("x1", 0, math.Inf(1))
		x2 := m.AddIntVar("x2", 0, 10)
		x3 := m.AddContVar("x3", 0, math.Inf(1))
		cDir := -1.0
		if sense == gb.Maximize {
			cDir = 1.0
		}
		m.SetObjective(gb.NewExpr(
			x0.T(cDir*float64(cur.intRange(2, 5))),
			x1.T(cDir*float64(cur.intRange(1, 3))),
			x2.T(float64(cur.intRange(-2, 2))),
			x3.T(cDir*0.5),
		))
		a0 := float64(cur.intRange(2, 4))
		a1 := float64(cur.intRange(2, 4))
		rhs0 := float64(cur.intRange(1, 5))
		switch cur.u8() % 3 {
		case 0:
			m.AddConstraint("dio", gb.Eq(gb.NewExpr(x0.T(a0), x1.T(-a1), x2.T(1)), rhs0))
		case 1:
			m.AddConstraint("ge_anchor", gb.Ge(gb.NewExpr(x0.T(a0), x1.T(a1), x3.T(1)), rhs0+2))
		default:
			// Opposing sub-tolerance integer inequality band triggering solveExactMIPFeasibilityStop / solveExactFeasNodeLP.
			eps := cur.pickFloat([]float64{1e-8, 1e-9, 1e-10})
			m.AddConstraint("subtol_lo", gb.Ge(gb.NewExpr(x0.T(2), x1.T(-2)), eps))
			m.AddConstraint("subtol_hi", gb.Le(gb.NewExpr(x0.T(2), x1.T(-2)), eps))
		}
		m.AddConstraint("link", gb.Le(gb.NewExpr(x2.T(2), x3.T(-1)), float64(cur.intRange(2, 8))))
		p := m.Compile()

		resUncert, errUncert := gb.SolveCompiled(tb.Context(), p,
			gb.WithPureGo(),
			gb.WithPresolve(false),
			gb.WithMaxNodes(120),
			gb.WithTimeLimit(500*time.Millisecond),
		)
		if errUncert != nil {
			tb.Fatalf("[unb_mip_cold/uncert] SolveCompiled error: %v", errUncert)
		}
		assertAllowedStatus(tb, "unb_mip_cold/uncert", p, true, resUncert)

		tr := gb.NewVIPRTrace()
		var wit gb.CertifiedWitness
		resCert, errCert := gb.SolveCompiled(tb.Context(), p,
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithCertifiedWitness(&wit),
			gb.WithVIPRTrace(tr),
			gb.WithPresolve(cur.u8()%2 == 0),
			gb.WithMaxNodes(200),
			gb.WithTimeLimit(time.Second),
		)
		if errCert != nil {
			assertFailClosedResultAndWitness(tb, "unb_mip_cold/fail_closed", resCert, errCert, wit)
		} else {
			assertAllowedStatus(tb, "unb_mip_cold", p, true, resCert)
			verifyWitnessRationalInvariants(tb, "unb_mip_cold", p, resCert, wit, 0, 0, true)
			if formalFuzzEnabled() {
				if errF := checkFormalVIPRResult(tb, "unb_mip_cold", p, resCert, tr, 0, 0); errF != nil {
					tb.Fatal(errF)
				}
			}
		}

	default:
		// Cold block 3: presolve.go handleExtremeRowTightening and evalRowActivityWide (a*ub > MaxFloat64 and wide-exponent rows).
		m := gb.NewModel("extreme_presolve_cold", gb.Minimize)
		bigUB := cur.pickFloat([]float64{1e150, 1e160, 1e180})
		x0 := m.AddIntVar("x0", 0, bigUB)
		x1 := m.AddIntVar("x1", 0, 1e6)
		x2 := m.AddBinaryVar("x2")
		x3 := m.AddContVar("x3", 0, bigUB)
		bigA := cur.pickFloat([]float64{1e160, 1e180, 1e200})
		tinyA := cur.pickFloat([]float64{1e-100, 1e-120, 1e-150})
		rhsHi := cur.pickFloat([]float64{1e200, 1e250, -1e100})
		m.AddConstraint("ext_le", gb.Le(gb.NewExpr(x0.T(bigA), x1.T(-tinyA), x2.T(1), x3.T(tinyA)), rhsHi))
		m.AddConstraint("ext_ge", gb.Ge(gb.NewExpr(x0.T(bigA), x1.T(1), x2.T(bigA)), cur.pickFloat([]float64{1e150, 1e180, -1e200})))
		// Non-dyadic cancellation (0.3*1e25 + 0.7*1e25 - 1e25 = 0) produces large dot2Acc errL1, triggering evalRowActivityWide.
		x4 := m.AddIntVar("x4", 1, 2)
		x5 := m.AddIntVar("x5", 1, 2)
		x6 := m.AddIntVar("x6", 1, 2)
		m.AddConstraint("wide_cancel", gb.Le(gb.NewExpr(x4.T(0.3e25), x5.T(0.7e25), x6.T(-1e25), x2.T(1)), -1e25))
		m.AddConstraint("wide_cancel_ge", gb.Ge(gb.NewExpr(x4.T(0.3e25), x5.T(0.7e25), x6.T(-1e25), x2.T(1)), 1e25))
		m.SetObjective(gb.NewExpr(x0.T(1), x1.T(2), x2.T(-5), x3.T(1e-3)))
		p := m.Compile()
		pre := gb.Presolve(p, 4)
		if pre.Reduced != nil && !pre.Infeasible {
			res, err := gb.SolveCompiled(tb.Context(), p,
				gb.WithPureGo(),
				gb.WithPresolve(true),
				gb.WithCuts(true),
				gb.WithMaxNodes(100),
				gb.WithTimeLimit(500*time.Millisecond),
			)
			if err != nil {
				tb.Fatalf("[extreme_presolve_cold] SolveCompiled error: %v", err)
			}
			assertAllowedStatus(tb, "extreme_presolve_cold", p, true, res)
		}
		if formalFuzzEnabled() {
			_, _ = verifyFormalPresolveRules(tb, p)
		}
	}
}

// FuzzRelaxCutDynamismAndRootCuts exercises GMI relaxCutDynamismTerms (term-ratio
// and cumulative RHS-weakening caps), pure-integer speculative root-cut lookahead
// and rollback, top uncovered cold blocks from corpus audit (parallel in-tree cuts,
// unbounded MILP recession-ray certification, extreme-scale presolve tightening,
// and duplicate-column / free-integer cut paths), and live cut separation against
// WithCertified(true), optional CBC, and Lean Track 2 cuts-check verification.
func FuzzRelaxCutDynamismAndRootCuts(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		{1, 0, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25, 27, 29},
		{2, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30},
		{3, 1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 12, 7},
		{3, 0, 2, 0, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26},
		{3, 1, 1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
		{3, 0, 2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
		{3, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 2, 0, 1},
		{3, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := newCertFuzzCursor(data)
		regime := cur.u8() % 4
		presolve := cur.u8()%2 == 0
		maxCutPasses := cur.intRange(8, 20)

		// Always exercise direct relaxCutDynamismTerms threshold boundaries.
		runRelaxCutDynamismDirectRegime(t, cur)

		switch regime {
		case 0:
			// Pure-integer bin-packing MIP targeting speculative root-cut lookahead and rollback.
			p := buildBinpackSpeculativeCutMIP(cur, false)
			checkRootCutsAndTrack2Parity(t, "binpack_spec", p, presolve, maxCutPasses, false)
		case 1:
			// Compact pure-integer bin-packing MIP with full WithCertified + VIPR + cuts-check.
			p := buildBinpackSpeculativeCutMIP(cur, true)
			checkRootCutsAndTrack2Parity(t, "binpack_cert", p, presolve, maxCutPasses, true)
		case 2:
			// Mixed-scale general MILP exercising GMI relaxation, cover/MIR/VUB cuts, and cuts-check.
			p := buildMixedScaleGMIMIP(cur)
			checkRootCutsAndTrack2Parity(t, "mixed_gmi", p, presolve, maxCutPasses, true)
		default:
			// Top uncovered cold blocks from Step A corpus audit.
			runStepAColdBlockRegime(t, cur)
		}
	})
}
