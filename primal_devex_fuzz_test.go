package goblin_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

func buildPrimalDevexFuzzLP(cur *certFuzzCursor, regime int) (*gb.CompiledProblem, bool) {
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("primal_devex_fuzz", sense)

	switch regime {
	case 0:
		// General boxed/free/mixed-sense LP with Phase I infeasibilities and bound flips.
		n := cur.intRange(3, 10)
		mRows := cur.intRange(2, 8)
		xs := make([]gb.Var, n)
		for j := range n {
			var lb, ub float64
			switch cur.u8() % 5 {
			case 0:
				lb, ub = 0, float64(cur.intRange(1, 6))
			case 1:
				lb, ub = float64(cur.intRange(-4, 2)), float64(cur.intRange(3, 12))
			case 2:
				lb, ub = 0, math.Inf(1)
			case 3:
				lb, ub = math.Inf(-1), math.Inf(1)
			default:
				v := float64(cur.intRange(-2, 3))
				lb, ub = v, v
			}
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), lb, ub)
		}
		for i := range mRows {
			terms := make([]gb.Term, 0, n)
			for j := range n {
				c := float64(cur.intRange(-5, 5))
				if c != 0 {
					terms = append(terms, gb.T(xs[j], c))
				}
			}
			if len(terms) == 0 {
				terms = append(terms, gb.T(xs[i%n], 1))
			}
			rhs := float64(cur.intRange(-8, 15))
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
			c := float64(cur.intRange(-9, 9))
			if c != 0 {
				objTerms = append(objTerms, gb.T(xs[j], c))
			}
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		return m.Compile(), true

	case 1:
		// Perturbed Klee-Minty or degenerate cycling LP exercising Devex rectifier resets and Bland fallback.
		d := cur.intRange(4, 7)
		xs := make([]gb.Var, d)
		objTerms := make([]gb.Term, 0, d)
		scaleBase := cur.pickFloat([]float64{2.0, 2.5, 3.0})
		for j := range d {
			ub := math.Inf(1)
			if cur.u8()%4 == 0 {
				ub = math.Pow(scaleBase+1, float64(d))
			}
			xs[j] = m.AddContVar("k"+strconv.Itoa(j), 0, ub)
			w := math.Pow(2.0, float64(d-1-j))
			if sense == gb.Minimize {
				w = -w
			}
			objTerms = append(objTerms, gb.T(xs[j], w))
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		rhs := 4.0 + float64(cur.intRange(0, 3))
		for i := range d {
			terms := make([]gb.Term, 0, i+1)
			for k := range i {
				terms = append(terms, gb.T(xs[k], math.Pow(2.0, float64(i+1-k))))
			}
			terms = append(terms, gb.T(xs[i], 1.0))
			if cur.u8()%4 == 0 {
				// Degenerate zero-RHS perturbation.
				m.AddConstraint("km"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), 0))
			} else {
				m.AddConstraint("km"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), rhs))
			}
			rhs *= scaleBase + 1.0
		}
		return m.Compile(), true

	case 2:
		// Chunked-width LP (n in [68, 84]) exercising non-sparse chunked Devex weight and incremental dj updates.
		n := cur.intRange(68, 84)
		mRows := cur.intRange(6, 12)
		xs := make([]gb.Var, n)
		objTerms := make([]gb.Term, 0, n)
		for j := range n {
			ub := float64(cur.intRange(2, 10))
			if j%7 == 0 {
				ub = math.Inf(1)
			}
			xs[j] = m.AddContVar("w"+strconv.Itoa(j), 0, ub)
			objTerms = append(objTerms, gb.T(xs[j], float64(cur.intRange(-7, 7))))
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		for i := range mRows {
			terms := make([]gb.Term, 0, n)
			for j := range n {
				// Keep >= 40% density so ComputeTableauRow triggers chunked mode (!tabRowSparse).
				if (int(cur.u8())+i+j)%5 != 0 {
					c := float64(cur.intRange(-3, 5))
					if c != 0 {
						terms = append(terms, gb.T(xs[j], c))
					}
				}
			}
			if len(terms) == 0 {
				terms = append(terms, gb.T(xs[i%n], 1))
			}
			rhs := float64(cur.intRange(5, 30))
			if cur.u8()%3 == 0 {
				m.AddConstraint("cw"+strconv.Itoa(i), gb.Ge(gb.NewExpr(terms...), rhs))
			} else {
				m.AddConstraint("cw"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), rhs))
			}
		}
		return m.Compile(), false

	default:
		// High-cancellation / wide-span LP targeting compensated Phase II pricing fallback and large-violation scaling.
		mComp := gb.NewModel("primal_comp_fallback", gb.Minimize)
		n := cur.intRange(3, 5)
		xs := make([]gb.Var, n)
		bigC := cur.pickFloat([]float64{1e7, 1e8, 1e9})
		tinyDelta := cur.pickFloat([]float64{-1e-6, -1e-7, -1e-8, 1e-7})
		for j := range n {
			switch j {
			case 0:
				xs[j] = mComp.AddContVar("x0", 0, 1e8)
			case 1:
				if cur.u8()%2 == 0 {
					xs[j] = mComp.AddContVar("x1", 0, math.Inf(1))
				} else {
					xs[j] = mComp.AddContVar("x1", 0, 1e9)
				}
			default:
				xs[j] = mComp.AddContVar("x"+strconv.Itoa(j), 0, float64(cur.intRange(2, 20)))
			}
		}
		mComp.SetObjective(gb.NewExpr(
			xs[0].T(bigC),
			xs[1].T(bigC+tinyDelta),
			xs[2].T(float64(cur.intRange(-5, 5))),
		))
		mComp.AddConstraint("eq_bal", gb.Eq(gb.NewExpr(xs[0].T(1), xs[1].T(1), xs[2].T(0.5)), 10))
		mComp.AddConstraint("cap", gb.Le(gb.NewExpr(xs[1].T(1), xs[2].T(2)), float64(cur.intRange(5, 50))))
		if n > 3 {
			mComp.AddConstraint("ge_p1", gb.Ge(gb.NewExpr(xs[2].T(1), xs[3].T(1)), float64(cur.intRange(1, 6))))
		}
		return mComp.Compile(), false
	}
}

func assertPrimalVsOracleParity(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	resDevex gb.LPResult,
	resOther gb.LPResult,
	otherName string,
) {
	tb.Helper()
	if resDevex.Status == gb.StatusIterationLimit || resDevex.Status == gb.StatusTimeLimit ||
		resOther.Status == gb.StatusIterationLimit || resOther.Status == gb.StatusTimeLimit {
		return
	}
	if resDevex.Status == gb.StatusNumericalError || resOther.Status == gb.StatusNumericalError {
		if isStandardScaledProblem(p) {
			tb.Fatalf("[%s] unexpected NumericalError on standard-scaled LP: devex=%s %s=%s",
				label, resDevex.Status, otherName, resOther.Status)
		}
		return
	}
	if resDevex.Status != resOther.Status {
		tb.Fatalf("[%s] status mismatch: devex=%s vs %s=%s", label, resDevex.Status, otherName, resOther.Status)
	}
	if resDevex.Status == gb.StatusOptimal {
		scale := max(1.0, math.Abs(resDevex.Objective), math.Abs(resOther.Objective))
		if math.Abs(resDevex.Objective-resOther.Objective) > 1e-4*scale {
			tb.Fatalf("[%s] objective mismatch: devex=%.12g vs %s=%.12g",
				label, resDevex.Objective, otherName, resOther.Objective)
		}
	}
}

// FuzzPrimalDevexAndIncrementalDuals exercises forced two-phase primal simplex
// (Harris Devex vs Dantzig and dual/certified oracles), incremental dual/dj updates,
// Phase I sign flips, bound flips, rectifier resets, |tabRow[enterVar]-piv| drift,
// chunked non-sparse updates, workspace reuse, and compensated Phase II fallback.
func FuzzPrimalDevexAndIncrementalDuals(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		{1, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28},
		{2, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25, 27, 29},
		{3, 2, 4, 1, 5, 9, 2, 6, 5, 3, 5, 8, 9, 7, 9, 3},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := newCertFuzzCursor(data)
		regime := int(cur.u8() % 4)
		useScaling := cur.u8()%2 == 0
		rectifierMask := cur.u8()
		p, runCertified := buildPrimalDevexFuzzLP(cur, regime)

		// 1. Step-by-step verification of incremental y/dj updates, Phase I sign flips,
		// bound flips, Devex weight clamps, and rectifier resets.
		if err := gb.VerifyPrimalIncrementalStepsForTest(p, 24, false, useScaling, rectifierMask); err != nil {
			t.Fatalf("incremental primal Devex step invariant failed (regime=%d): %v", regime, err)
		}
		if regime == 0 {
			if err := gb.VerifyPrimalTabRowDriftGuardForTest(p); err != nil {
				t.Fatalf("tabRow drift refactor guard failed: %v", err)
			}
		}

		// 2. Full forced primal solve with Harris Devex (reusing a pooled workspace across solves).
		ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws)

		sDevex, resDevex, diagDevex := gb.RunForcedPrimalForTest(p, ws, false, useScaling, 4000)
		if diagDevex.DevexActiveAfter {
			t.Fatalf("solveTwoPhasePrimalWithLimit left devexActive=true")
		}
		if resDevex.Status == gb.StatusOptimal {
			bV, rV, _ := p.MaxViolation(resDevex.Primal, false)
			if isStandardScaledProblem(p) && (bV > 1e-5 || rV > 1e-5) {
				t.Fatalf("Devex primal violation b=%.2e r=%.2e", bV, rV)
			}
		}
		if formalFuzzEnabled() {
			verifyFormalLPResult(t, "primal_devex", p, sDevex, resDevex)
		}

		// 3. Differential check against Dantzig (disableDevex=true) and default Dual simplex.
		_, resDantzig, diagDantzig := gb.RunForcedPrimalForTest(p, ws, true, useScaling, 4000)
		if diagDantzig.DevexActiveAfter {
			t.Fatalf("Dantzig solve left devexActive=true")
		}
		assertPrimalVsOracleParity(t, "devex_vs_dantzig", p, resDevex, resDantzig, "dantzig")

		// Reuse the same workspace for default dual simplex to verify zero stale Devex bleed.
		sDual := gb.NewLPSolver(p, ws)
		resDual := sDual.Solve()
		assertPrimalVsOracleParity(t, "devex_vs_dual", p, resDevex, resDual, "dual")

		// 4. Certified oracle parity on compact standard LPs.
		if runCertified {
			resCert, errCert := gb.SolveCompiled(t.Context(), p,
				gb.WithPureGo(),
				gb.WithCertified(true),
				gb.WithPresolve(false),
				gb.WithMaxIter(4000),
				gb.WithTimeLimit(time.Second),
			)
			if errCert == nil {
				assertPrimalVsOracleParity(t, "devex_vs_certified", p, resDevex, gb.LPResult{
					Status:    resCert.Status,
					Objective: resCert.Objective,
				}, "certified")
			}
		}
	})
}
