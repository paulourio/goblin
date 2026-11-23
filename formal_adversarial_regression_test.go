package goblin

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func mustAdvRatFromFloat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func assertVIPRVerifiedIfFormal(tb testing.TB, tr *VIPRTrace, wantMode string) {
	tb.Helper()
	if !FormalOracleEnabled() {
		return
	}
	if !tr.HasCertificate() {
		tb.Fatalf("expected VIPR certificate (mode=%s), got none", wantMode)
	}
	if wantMode != "" && tr.Mode() != wantMode {
		tb.Fatalf("VIPR mode = %q, want %q", tr.Mode(), wantMode)
	}
	raw, err := tr.MarshalVIPRJSON()
	if err != nil {
		tb.Fatalf("MarshalVIPRJSON: %v", err)
	}
	resp, err := FormalCheckVIPRJSONL(tb, raw)
	if err != nil || !resp.OK {
		tb.Fatalf("vipr-check rejected certificate: resp=%+v err=%v", resp, err)
	}
}

func TestFormalAdversarialBug1PresolveTinyCoeffRedundancy(t *testing.T) {
	t.Parallel()

	t.Run("tiny_le_coeff_scales_objective", func(t *testing.T) {
		t.Parallel()
		for _, sense := range []ConstraintSense{LessEqual, Equal} {
			m := NewModel("tiny_coeff_bug1", Maximize)
			x := m.AddBinaryVar("x")
			y := m.AddContVar("y", 0, 1e9)
			m.SetObjective(VarExpr(y))
			m.AddConstraint("tiny", Constraint{Expr: NewExpr(x.T(1e-9)), Sense: sense, RHS: 0})
			m.AddConstraint("link", NewExpr(y.T(1), x.T(-1e9)).Le(0))
			p := m.Compile()

			pre := Presolve(p, defaultPresolveMaxPass)
			if pre.Infeasible {
				t.Fatalf("sense=%v: unexpected presolve infeasibility", sense)
			}
			// Presolve may drop tiny-coeff rows only after tightening x to 0; if x remains
			// free (ub > 0), the tiny-coeff row must not be dropped.
			if pre.Reduced != nil && pre.Reduced.varUB[0] > 0 && pre.RemovedRows != 0 {
				t.Fatalf("sense=%v: Presolve dropped non-redundant tiny-coeff row while ub[x]=%g (RemovedRows=%d)",
					sense, pre.Reduced.varUB[0], pre.RemovedRows)
			}

			res, err := SolveCompiled(t.Context(), p, WithPresolve(true))
			if err != nil || res.Status != StatusOptimal {
				t.Fatalf("sense=%v: solve failed: status=%v err=%v", sense, res.Status, err)
			}
			if math.Abs(res.Objective) > 1e-9 || res.Values[0] != 0 || math.Abs(res.Values[1]) > 1e-9 {
				t.Fatalf("sense=%v: got obj=%g vals=%v, want obj=0 vals=[0 0]", sense, res.Objective, res.Values)
			}

			tr := NewVIPRTrace()
			resVIPR, err := SolveCompiled(t.Context(), p, WithPresolve(true), WithVIPRTrace(tr))
			if err != nil || resVIPR.Status != StatusOptimal || math.Abs(resVIPR.Objective) > 1e-9 {
				t.Fatalf("sense=%v: VIPR solve got status=%v obj=%g err=%v", sense, resVIPR.Status, resVIPR.Objective, err)
			}
			assertVIPRVerifiedIfFormal(t, tr, "optimal")

			if FormalOracleEnabled() {
				items, _ := ExportPresolveRulesItems(p)
				for idx, raw := range items {
					resp, callErr := FormalCheckRulesJSONL(t, raw)
					if callErr != nil || !resp.OK {
						t.Fatalf("sense=%v rules-check item %d failed: resp=%+v err=%v", sense, idx, resp, callErr)
					}
				}
			}
		}
	})

	t.Run("tiny_ge_and_pure_int", func(t *testing.T) {
		t.Parallel()
		mGE := NewModel("tiny_ge_bug1", Maximize)
		x := mGE.AddBinaryVar("x")
		y := mGE.AddContVar("y", 0, 1e9)
		mGE.SetObjective(VarExpr(y))
		mGE.AddConstraint("tiny_ge", NewExpr(x.T(-1e-9)).Ge(0))
		mGE.AddConstraint("link", NewExpr(y.T(1), x.T(-1e9)).Le(0))
		pGE := mGE.Compile()
		resGE, err := SolveCompiled(t.Context(), pGE, WithPresolve(true))
		if err != nil || resGE.Status != StatusOptimal || math.Abs(resGE.Objective) > 1e-9 {
			t.Fatalf("tiny GE: status=%v obj=%g vals=%v err=%v, want Optimal 0", resGE.Status, resGE.Objective, resGE.Values, err)
		}

		mInt := NewModel("tiny_pure_int_bug1", Maximize)
		ix := mInt.AddIntVar("x", 0, 5)
		iy := mInt.AddIntVar("y", 0, 5)
		mInt.SetObjective(NewExpr(ix.T(1), iy.T(1)))
		mInt.AddConstraint("tiny_sum", NewExpr(ix.T(1e-10), iy.T(1e-10)).Le(0))
		pInt := mInt.Compile()
		resInt, err := SolveCompiled(t.Context(), pInt, WithPresolve(true))
		if err != nil || resInt.Status != StatusOptimal || math.Abs(resInt.Objective) > 1e-9 {
			t.Fatalf("tiny pure int: status=%v obj=%g vals=%v err=%v, want Optimal 0", resInt.Status, resInt.Objective, resInt.Values, err)
		}
	})

	t.Run("subnormal_wide_activity_margin", func(t *testing.T) {
		t.Parallel()
		tiny := math.Ldexp(1, -1050)
		m := NewModel("subnormal_row_bug1", Maximize)
		x := m.AddBinaryVar("x")
		y := m.AddContVar("y", 0, 1e6)
		m.SetObjective(VarExpr(y))
		m.AddConstraint("sub", NewExpr(x.T(tiny)).Le(0))
		m.AddConstraint("link", NewExpr(y.T(1), x.T(-1e6)).Le(0))
		p := m.Compile()
		pre := Presolve(p, defaultPresolveMaxPass)
		if pre.Infeasible || pre.RemovedRows != 0 {
			t.Fatalf("subnormal row presolve: infeas=%v removed=%d, want false/0", pre.Infeasible, pre.RemovedRows)
		}
	})
}

func TestFormalAdversarialBug2DynamicOrbitRounding(t *testing.T) {
	t.Parallel()

	bigM := math.Ldexp(1, 53)
	m := NewModel("dynamic_orbit_2p53_bug2", Minimize)
	f := m.AddBinaryVar("f")
	v := m.AddContVar("v", 0, 1)
	y := make([]Var, 3)
	x := make([]Var, 3)
	for g := range 3 {
		y[g] = m.AddBinaryVar(fmt.Sprintf("y%d", g))
		x[g] = m.AddIntVar(fmt.Sprintf("x%d", g), 0, bigM)
	}
	m.SetObjective(NewExpr(f.T(10), v.T(1)))
	m.AddConstraint("force_f", NewExpr(f.T(2)).Ge(1))
	m.AddConstraint("part_y", NewExpr(y[0].T(1), y[1].T(1), y[2].T(1)).Eq(1))
	m.AddConstraint("cap0", NewExpr(x[0].T(1), f.T(0.25)).Le(bigM))
	m.AddConstraint("cap1", NewExpr(x[1].T(1), f.T(0.25)).Le(bigM))
	m.AddConstraint("cap2", NewExpr(x[2].T(1)).Le(bigM))
	for g := range 3 {
		m.AddConstraint(fmt.Sprintf("link_x%d", g), NewExpr(x[g].T(1), y[g].T(-bigM)).Ge(0))
		m.AddConstraint(fmt.Sprintf("link_v%d", g), NewExpr(y[g].T(1), v.T(-1)).Le(0))
	}
	p := m.Compile()

	// Verify detectDynamicNodeOrbit does not merge asymmetric group 2 with groups 0 and 1 when f=1.
	curLB := append([]float64(nil), p.varLB...)
	curUB := append([]float64(nil), p.varUB...)
	curLB[0], curUB[0] = 1, 1 // fix f = 1
	var sc *orbitTrialScratch
	orb := detectDynamicNodeOrbit(p, curLB, curUB, 2, time.Time{}, nil, &sc, nil) // bVar = y0 (col 2)
	for _, col := range orb {
		if col == 6 { // y2 is col 6 (f=0, v=1, y0=2, x0=3, y1=4, x1=5, y2=6, x2=7)
			t.Fatalf("detectDynamicNodeOrbit falsely placed asymmetric y2 (col 6) in y0 orbit %v", orb)
		}
	}

	configs := []struct {
		name string
		opts []Option
	}{
		{"default", nil},
		{"no_presolve", []Option{WithPresolve(false)}},
		{"no_cuts", []Option{WithCuts(false)}},
		{"no_heur", []Option{WithHeuristics(false)}},
		{"workers2", []Option{WithWorkers(2)}},
	}
	for _, tc := range configs {
		res, err := SolveCompiled(t.Context(), p, tc.opts...)
		if err != nil {
			t.Fatalf("%s: err=%v", tc.name, err)
		}
		if res.Status == StatusInfeasible {
			t.Fatalf("%s: returned false StatusInfeasible on feasible dynamic-orbit MIP", tc.name)
		}
		if res.Status == StatusOptimal {
			if math.Abs(res.Objective-11.0) > 1e-6 || res.Values[6] < 0.5 {
				t.Fatalf("%s: obj=%g vals=%v, want obj=11 with y2=1", tc.name, res.Objective, res.Values)
			}
		}
	}

	// Well-conditioned variant where y_g=0 forces zero activity on cap0/cap1 while
	// still colliding at 2^53 - 0.25 == 2^53 in float64 residual rounding.
	mOpt := NewModel("dynamic_orbit_2p53_opt_bug2", Minimize)
	f2 := mOpt.AddBinaryVar("f")
	v2 := mOpt.AddContVar("v", 0, 1)
	y2 := make([]Var, 3)
	for g := range 3 {
		y2[g] = mOpt.AddBinaryVar(fmt.Sprintf("y%d", g))
	}
	mOpt.SetObjective(NewExpr(f2.T(10), v2.T(1)))
	mOpt.AddConstraint("force_f", NewExpr(f2.T(2)).Ge(1))
	mOpt.AddConstraint("part_y", NewExpr(y2[0].T(1), y2[1].T(1), y2[2].T(1)).Eq(1))
	mOpt.AddConstraint("cap0", NewExpr(y2[0].T(bigM), f2.T(0.25)).Le(bigM))
	mOpt.AddConstraint("cap1", NewExpr(y2[1].T(bigM), f2.T(0.25)).Le(bigM))
	mOpt.AddConstraint("cap2", NewExpr(y2[2].T(bigM)).Le(bigM))
	for g := range 3 {
		mOpt.AddConstraint(fmt.Sprintf("link_v%d", g), NewExpr(y2[g].T(1), v2.T(-1)).Le(0))
	}
	pOpt := mOpt.Compile()

	lb2 := append([]float64(nil), pOpt.varLB...)
	ub2 := append([]float64(nil), pOpt.varUB...)
	lb2[0], ub2[0] = 1, 1
	var sc2 *orbitTrialScratch
	orb2 := detectDynamicNodeOrbit(pOpt, lb2, ub2, 2, time.Time{}, nil, &sc2, nil)
	for _, col := range orb2 {
		if col == 4 { // y2 is col 4
			t.Fatalf("detectDynamicNodeOrbit(pOpt) falsely merged y2 (col 4) into y0 orbit %v", orb2)
		}
	}
	for _, tc := range configs {
		res, err := SolveCompiled(t.Context(), pOpt, tc.opts...)
		if err != nil {
			t.Fatalf("pOpt %s: err=%v", tc.name, err)
		}
		if res.Status == StatusInfeasible {
			t.Fatalf("pOpt %s: false StatusInfeasible", tc.name)
		}
	}

	// Exact rational control with fCoeff=0.75 (2^53 - 0.75 = 2^53 - 1) verifies full VIPR optimality.
	mCtrl := NewModel("dynamic_orbit_2p53_ctrl_bug2", Minimize)
	fc := mCtrl.AddBinaryVar("f")
	vc := mCtrl.AddContVar("v", 0, 1)
	yc := make([]Var, 3)
	xc := make([]Var, 3)
	for g := range 3 {
		yc[g] = mCtrl.AddBinaryVar(fmt.Sprintf("y%d", g))
		xc[g] = mCtrl.AddIntVar(fmt.Sprintf("x%d", g), 0, bigM)
	}
	mCtrl.SetObjective(NewExpr(fc.T(10), vc.T(1)))
	mCtrl.AddConstraint("force_f", NewExpr(fc.T(2)).Ge(1))
	mCtrl.AddConstraint("part_y", NewExpr(yc[0].T(1), yc[1].T(1), yc[2].T(1)).Eq(1))
	mCtrl.AddConstraint("cap0", NewExpr(xc[0].T(1), fc.T(0.75)).Le(bigM))
	mCtrl.AddConstraint("cap1", NewExpr(xc[1].T(1), fc.T(0.75)).Le(bigM))
	mCtrl.AddConstraint("cap2", NewExpr(xc[2].T(1)).Le(bigM))
	for g := range 3 {
		mCtrl.AddConstraint(fmt.Sprintf("link_x%d", g), NewExpr(xc[g].T(1), yc[g].T(-bigM)).Ge(0))
		mCtrl.AddConstraint(fmt.Sprintf("link_v%d", g), NewExpr(yc[g].T(1), vc.T(-1)).Le(0))
	}
	pCtrl := mCtrl.Compile()
	tr := NewVIPRTrace()
	resVIPR, err := SolveCompiled(t.Context(), pCtrl, WithPresolve(false), WithVIPRTrace(tr))
	if err != nil || resVIPR.Status != StatusOptimal || math.Abs(resVIPR.Objective-11.0) > 1e-6 {
		t.Fatalf("VIPR ctrl solve: status=%v obj=%g err=%v", resVIPR.Status, resVIPR.Objective, err)
	}
	assertVIPRVerifiedIfFormal(t, tr, "optimal")
}

func TestFormalAdversarialBug3ObjConstantGapAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("rel_gap_excludes_obj_constant_min_and_max", func(t *testing.T) {
		t.Parallel()
		for _, sense := range []Sense{Minimize, Maximize} {
			m := NewModel("obj_const_1e12_bug3a", sense)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddBinaryVar("y")
			sign := 1.0
			if sense == Maximize {
				sign = -1.0
			}
			m.SetObjective(NewExpr(x.T(sign*4), y.T(sign*10)).AddConstant(1e12))
			m.AddConstraint("cover", NewExpr(x.T(3), y.T(10)).Ge(5))
			p := m.Compile()

			for _, workers := range []int{1, 2} {
				res, err := SolveCompiled(t.Context(), p, WithWorkers(workers))
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("sense=%v workers=%d: status=%v err=%v", sense, workers, res.Status, err)
				}
				if math.Abs(res.Values[0]-2.0) > 1e-6 || math.Abs(res.Values[1]) > 1e-6 {
					t.Fatalf("sense=%v workers=%d: vals=%v obj=%.12g, want [2 0] (linear obj 8)", sense, workers, res.Values, res.Objective)
				}
			}

			tr := NewVIPRTrace()
			resVIPR, err := SolveCompiled(t.Context(), p, WithVIPRTrace(tr))
			if err != nil || resVIPR.Status != StatusOptimal || math.Abs(resVIPR.Values[0]-2.0) > 1e-6 {
				t.Fatalf("sense=%v VIPR: status=%v vals=%v err=%v", sense, resVIPR.Status, resVIPR.Values, err)
			}
			assertVIPRVerifiedIfFormal(t, tr, "optimal")
		}
	})

	t.Run("zero_tol_catastrophic_cancellation_1e17", func(t *testing.T) {
		t.Parallel()
		m := NewModel("obj_const_1e17_bug3b", Minimize)
		x := m.AddIntVar("x", 0, math.Inf(1))
		y := m.AddBinaryVar("y")
		m.SetObjective(NewExpr(x.T(3), y.T(7)).AddConstant(1e17))
		m.AddConstraint("cover", NewExpr(x.T(3), y.T(10)).Ge(5))
		p := m.Compile()

		for _, workers := range []int{1, 2} {
			res, err := SolveCompiled(t.Context(), p, WithRelGapTol(0), WithAbsGapTol(0), WithWorkers(workers))
			if err != nil || res.Status != StatusOptimal {
				t.Fatalf("workers=%d: status=%v err=%v", workers, res.Status, err)
			}
			if math.Abs(res.Values[0]-2.0) > 1e-6 || math.Abs(res.Values[1]) > 1e-6 {
				t.Fatalf("workers=%d: vals=%v, want exact optimum [2 0] (linear obj 6, not [0 1] with 7)", workers, res.Values)
			}
		}

		tr := NewVIPRTrace()
		resVIPR, err := SolveCompiled(t.Context(), p, WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if err != nil || resVIPR.Status != StatusOptimal || math.Abs(resVIPR.Values[0]-2.0) > 1e-6 {
			t.Fatalf("VIPR solve: status=%v vals=%v err=%v", resVIPR.Status, resVIPR.Values, err)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})
}

func TestFormalAdversarialBug4CMIRUnderflowAndVUBRounding(t *testing.T) {
	t.Parallel()

	t.Run("cmir_neg_zero_underflow", func(t *testing.T) {
		t.Parallel()
		m := NewModel("cmir_underflow_bug4", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, math.Inf(1))
		m.SetObjective(NewExpr(x0.T(1), x1.T(-1e-20)))
		m.AddConstraint("r0", NewExpr(x0.T(1e30), x1.T(-1e-300)).Le(0.5e30))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", lpRes.Status)
		}
		var ws cutGenWorkspace
		cuts := separateRowMIRCutsWithWS(p, lpRes.Primal, &ws)
		// Any valid cut must remain satisfied by integer-feasible (x0=1, x1=10^330).
		x1Huge := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(330), nil))
		for ci, c := range cuts {
			lhs := new(big.Rat)
			for k, col := range c.ColIdx {
				vr := mustAdvRatFromFloat(t, c.Val[k])
				switch col {
				case 0:
					lhs.Add(lhs, vr)
				case 1:
					lhs.Add(lhs, new(big.Rat).Mul(vr, x1Huge))
				}
			}
			rhs := mustAdvRatFromFloat(t, c.RHS)
			if lhs.Cmp(rhs) > 0 {
				t.Fatalf("cMIR cut %d (%+v) cuts off integer-feasible (1, 10^330): lhs=%v > rhs=%v", ci, c, lhs, rhs)
			}
		}

		if FormalOracleEnabled() && len(cuts) > 0 {
			x1HugeF, _ := x1Huge.Float64()
			cutItems, err := ExportSeparatedCutsItems(p, [][]float64{{0, 0}, {1, x1HugeF}})
			if err != nil {
				t.Fatalf("ExportSeparatedCutsItems: %v", err)
			}
			for _, item := range cutItems {
				resp, callErr := FormalCheckCutsJSONL(t, item)
				if callErr != nil || !resp.OK {
					t.Fatalf("cuts-check rejected cut certificate: resp=%+v err=%v", resp, callErr)
				}
			}
		}
	})

	t.Run("cmir_negative_alpha_near_zero_high_f0", func(t *testing.T) {
		t.Parallel()
		// x0 - 2^-55 * x1 <= 0.98 has exact MIR(-2^-55) = -50 * 2^-55 < -1.38e-15 when f0 = 0.98.
		a1 := -math.Ldexp(1, -55)
		m := NewModel("cmir_neg_small_alpha_high_f0", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, math.Inf(1))
		m.SetObjective(NewExpr(x0.T(1), x1.T(-1e-18)))
		m.AddConstraint("r0", NewExpr(x0.T(1), x1.T(a1)).Le(0.98))
		p := m.Compile()

		var ws cutGenWorkspace
		cuts := separateRowMIRCutsWithWS(p, []float64{0.98, 0}, &ws)
		// Integer feasible point: x0 = 1, x1 = ceil(0.02 / 2^-55) = 720575940379280.
		x1Feas := new(big.Rat).SetInt64(720575940379280)
		// Verify exact feasibility of (1, x1Feas): 1 + a1*x1Feas <= 0.98.
		lhsOrig := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).Mul(mustAdvRatFromFloat(t, a1), x1Feas))
		if lhsOrig.Cmp(mustAdvRatFromFloat(t, 0.98)) > 0 {
			t.Fatalf("test witness not feasible: %v > 0.98", lhsOrig)
		}
		for ci, c := range cuts {
			lhs := new(big.Rat)
			for k, col := range c.ColIdx {
				vr := mustAdvRatFromFloat(t, c.Val[k])
				switch col {
				case 0:
					lhs.Add(lhs, vr)
				case 1:
					lhs.Add(lhs, new(big.Rat).Mul(vr, x1Feas))
				}
			}
			rhs := mustAdvRatFromFloat(t, c.RHS)
			if lhs.Cmp(rhs) > 0 {
				t.Fatalf("cMIR cut %d (%+v) cuts off integer-feasible (1, %v): lhs=%v > rhs=%v", ci, c, x1Feas, lhs, rhs)
			}
		}
		if FormalOracleEnabled() && len(cuts) > 0 {
			x1FeasF, _ := x1Feas.Float64()
			cutItems, err := ExportSeparatedCutsItems(p, [][]float64{{0, 0}, {1, x1FeasF}})
			if err != nil {
				t.Fatalf("ExportSeparatedCutsItems: %v", err)
			}
			for _, item := range cutItems {
				resp, callErr := FormalCheckCutsJSONL(t, item)
				if callErr != nil || !resp.OK {
					t.Fatalf("cuts-check rejected cut certificate: resp=%+v err=%v", resp, callErr)
				}
			}
		}
	})

	t.Run("cmir_inf_overflow", func(t *testing.T) {
		t.Parallel()
		// 1. Integer alpha = num / delta overflows to +Inf; without an explicit Inf guard,
		// nextDown(+Inf) returns finite +MaxFloat64 and emits a bogus cut coefficient.
		mInt := NewModel("cmir_int_pos_inf_overflow", Maximize)
		x0 := mInt.AddBinaryVar("x0")
		x1 := mInt.AddBinaryVar("x1")
		mInt.SetObjective(NewExpr(x0.T(1), x1.T(1)))
		mInt.AddConstraint("r0", NewExpr(x0.T(1e-8), x1.T(1e305)).Le(0.5e-8))
		pInt := mInt.Compile()

		var ws cutGenWorkspace
		cutsInt := separateRowMIRCutsWithWS(pInt, []float64{0.5, 0}, &ws)
		if len(cutsInt) != 0 {
			t.Fatalf("integer alpha=+Inf overflow emitted bogus cuts: %+v", cutsInt)
		}

		// 2. Continuous g = nextUp((-a) / contDenLo) overflows to +Inf; must discard the cut
		// rather than emitting an infinite coefficient.
		mCont := NewModel("cmir_cont_inf_overflow", Maximize)
		y0 := mCont.AddBinaryVar("y0")
		c1 := mCont.AddContVar("c1", 0, 1)
		mCont.SetObjective(NewExpr(y0.T(1), c1.T(-1)))
		mCont.AddConstraint("r0", NewExpr(y0.T(1e-8), c1.T(-1e305)).Le(0.5e-8))
		pCont := mCont.Compile()

		cutsCont := separateRowMIRCutsWithWS(pCont, []float64{0.5, 0}, &ws)
		if len(cutsCont) != 0 {
			t.Fatalf("continuous g=+Inf overflow emitted bogus cuts: %+v", cutsCont)
		}
	})

	t.Run("vub_effub_outward_rounding", func(t *testing.T) {
		t.Parallel()
		m := NewModel("vub_non_dyadic_effub", Maximize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		y := m.AddBinaryVar("y")
		m.SetObjective(NewExpr(x0.T(1), x1.T(1)))
		m.AddConstraint("vub", NewExpr(x0.T(3), x1.T(7), y.T(-1)).Le(0))
		p := m.Compile()

		cache := buildCutStaticCache(p)
		if len(cache.disaggVUBs) != 2 {
			t.Fatalf("expected 2 disaggregated VUB terms, got %d", len(cache.disaggVUBs))
		}
		wantMins := map[int]*big.Rat{
			0: big.NewRat(1, 3),
			1: big.NewRat(1, 7),
		}
		for _, term := range cache.disaggVUBs {
			got := mustAdvRatFromFloat(t, term.effUB)
			want := wantMins[term.col]
			if got.Cmp(want) < 0 {
				t.Fatalf("col %d: effUB=%v (%.17g) < exact cap/ak=%v", term.col, got, term.effUB, want)
			}
			if FormalOracleEnabled() {
				type vubSol struct {
					X []ratPayload `json:"x"`
					Y ratPayload   `json:"y"`
				}
				uPtr := bigRatToPayload(big.NewRat(10, 1))
				req := map[string]any{
					"subcommand": "vub-check",
					"op":         "vub-check",
					"a":          []ratPayload{bigRatToPayload(big.NewRat(3, 1)), bigRatToPayload(big.NewRat(7, 1))},
					"u":          []*ratPayload{&uPtr, &uPtr},
					"cap":        bigRatToPayload(big.NewRat(1, 1)),
					"rhsRow":     bigRatToPayload(big.NewRat(0, 1)),
					"targetIdx":  term.col,
					"effUB":      bigRatToPayload(got),
					"cutRHS":     bigRatToPayload(big.NewRat(0, 1)),
					"solutions": []vubSol{
						{X: []ratPayload{bigRatToPayload(big.NewRat(0, 1)), bigRatToPayload(big.NewRat(0, 1))}, Y: bigRatToPayload(big.NewRat(0, 1))},
						{X: []ratPayload{bigRatToPayload(big.NewRat(1, 3)), bigRatToPayload(big.NewRat(0, 1))}, Y: bigRatToPayload(big.NewRat(1, 1))},
						{X: []ratPayload{bigRatToPayload(big.NewRat(0, 1)), bigRatToPayload(big.NewRat(1, 7))}, Y: bigRatToPayload(big.NewRat(1, 1))},
					},
				}
				raw, err := json.Marshal(req)
				if err != nil {
					t.Fatalf("marshal vub-check: %v", err)
				}
				resp, callErr := FormalCheckCutsJSONL(t, raw)
				if callErr != nil || !resp.OK {
					t.Fatalf("cuts-check vub-check rejected col %d: resp=%+v err=%v", term.col, resp, callErr)
				}
			}
		}
	})
}

func TestFormalAdversarialBug6PostsolveLPSlackSingleton(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sense    Sense
		c0       float64
		slackRHS float64
	}{
		{"max_slack_9e_6", Maximize, 1e18, 9e-6},
		{"max_slack_9e_8", Maximize, 1e18, 9e-8},
		{"min_slack_9e_6", Minimize, -1e18, 9e-6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := NewModel(tc.name, tc.sense)
			x := m.AddContVar("x", 0, 1e6)
			m.SetObjective(NewExpr(x.T(tc.c0)))
			m.AddConstraint("slack_singleton", NewExpr(x.T(1e-8)).Le(tc.slackRHS))
			m.AddConstraint("tight_singleton", NewExpr(x.T(3e9)).Le(15e9))
			p := m.Compile()

			resNoPre, err := SolveCompiled(t.Context(), p, WithPresolve(false))
			if err != nil || resNoPre.Status != StatusOptimal {
				t.Fatalf("no-presolve failed: status=%v err=%v", resNoPre.Status, err)
			}
			resPre, err := SolveCompiled(t.Context(), p, WithPresolve(true))
			if err != nil || resPre.Status != StatusOptimal {
				t.Fatalf("presolve failed: status=%v err=%v", resPre.Status, err)
			}
			if len(resPre.Duals) != 2 {
				t.Fatalf("expected 2 duals, got %v", resPre.Duals)
			}
			if resPre.Duals[0] != 0 {
				t.Fatalf("slack singleton absorbed non-zero dual %g (want 0; no-presolve=%v)", resPre.Duals[0], resNoPre.Duals)
			}
			if math.Abs(resPre.Duals[1]-resNoPre.Duals[1]) > 1e-6*max(1.0, math.Abs(resNoPre.Duals[1])) {
				t.Fatalf("tight singleton dual=%g, want ~%g", resPre.Duals[1], resNoPre.Duals[1])
			}

			if FormalOracleEnabled() {
				cert, okBase := exportCompiledProblemToLPPayload(p, "safe-bound")
				if !okBase {
					t.Fatal("exportCompiledProblemToLPPayload failed")
				}
				senseSign := 1.0
				if p.sense == Maximize {
					senseSign = -1.0
				}
				yBits := make([]uint64, len(resPre.Duals))
				for i, d := range resPre.Duals {
					yi := senseSign * d
					if yi == 0 {
						yi = 0
					}
					yBits[i] = math.Float64bits(yi)
				}
				cert.YBits = yBits
				raw, err := json.Marshal(cert)
				if err != nil {
					t.Fatalf("marshal lp-check: %v", err)
				}
				resp, callErr := FormalCheckLPJSONL(t, raw)
				if callErr != nil || !resp.OK || resp.Bound == nil {
					t.Fatalf("lp-check safe-bound rejected postsolveLP duals: resp=%+v err=%v", resp, callErr)
				}
				boundRat, bErr := resp.Bound.BigRat()
				if bErr != nil {
					t.Fatalf("parse bound: %v", bErr)
				}
				// Verify exact dual bound is within tiny relative tolerance of optimal objective.
				optMin := new(big.Rat).Mul(mustAdvRatFromFloat(t, p.objMin[0]), big.NewRat(5, 1))
				gap := new(big.Rat).Sub(optMin, boundRat)
				gapF, _ := gap.Float64()
				if gapF < -1e-6 || gapF > 1e-6*math.Abs(p.objMin[0])*5 {
					t.Fatalf("lp-check safe-bound gap too large: optMin=%v bound=%v gap=%g", optMin, boundRat, gapF)
				}
			}
		})
	}
}

func TestFormalAdversarialBug7Dot2OutwardActivityMargin(t *testing.T) {
	t.Parallel()

	// Mixed-scale binary strengthening where 0 < eb < 0.5*ulp(val) previously caused
	// deficitLo = val - eb to round back to val, under-estimating surplusHi and cutting
	// off feasible solutions with b = 0.
	m := NewModel("dot2_outward_bin_strengthen_bug7", Maximize)
	b := m.AddBinaryVar("b")
	x := m.AddContVar("x", 0, 1)
	m.SetObjective(NewExpr(b.T(1), x.T(1)))
	m.AddConstraint("mixed", NewExpr(b.T(2e11), x.T(1e-12)).Le(1e11))
	p := m.Compile()

	var s rowActivitySummary
	st, en := p.rowPtr[0], p.rowPtr[1]
	evalRowActivitySummaryInto(&s, p.colIdx[st:en], p.rowVal[st:en], p.varLB, p.varUB, p.rowRHS[0], false, false, true)
	// Exact deficit = 1e11 - (2e11 + 1e-12) = -1e11 - 1e-12; deficitLo must be <= exact deficit.
	exactDef := new(big.Rat).Sub(
		mustAdvRatFromFloat(t, 1e11),
		new(big.Rat).Add(mustAdvRatFromFloat(t, 2e11), mustAdvRatFromFloat(t, 1e-12)),
	)
	if mustAdvRatFromFloat(t, s.deficitLo).Cmp(exactDef) > 0 {
		t.Fatalf("deficitLo=%.17g > exact deficit %v (outward rounding lost)", s.deficitLo, exactDef)
	}

	pre := Presolve(p, defaultPresolveMaxPass)
	if pre.Infeasible {
		t.Fatal("unexpected presolve infeasibility")
	}
	if pre.Reduced != nil {
		// Feasible point (b=0, x=1) must remain feasible in the presolved problem.
		bV, rV, iV := pre.Reduced.MaxViolation([]float64{0, 1}, true)
		if bV > 0 || rV > 0 || iV > 0 {
			t.Fatalf("presolved row cut off exact feasible point (0, 1): viol=(%g,%g,%g)", bV, rV, iV)
		}
	}

	if FormalOracleEnabled() {
		items, _ := ExportPresolveRulesItems(p)
		for idx, raw := range items {
			resp, err := FormalCheckRulesJSONL(t, raw)
			if err != nil || !resp.OK {
				t.Fatalf("rules-check item %d failed: resp=%+v err=%v", idx, resp, err)
			}
		}
	}
}

func TestFormalAdversarialBug8FalseUnboundedRayOnBoundedLPAndMIP(t *testing.T) {
	t.Parallel()

	buildModel := func(mip bool, r2XCoeff float64) *CompiledProblem {
		name := "bug8_lp"
		if mip {
			name = "bug8_mip"
		}
		m := NewModel(name, Minimize)
		var x, y Var
		if mip {
			x = m.AddIntVar("x", 0, math.Inf(1))
			y = m.AddIntVar("y", 0, math.Inf(1))
		} else {
			x = m.AddContVar("x", 0, math.Inf(1))
			y = m.AddContVar("y", 0, math.Inf(1))
		}
		w := m.AddContVar("w", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("r0", NewExpr(y.T(1), x.T(-1)).Eq(0))
		m.AddConstraint("r1", NewExpr(w.T(1), y.T(-1), x.T(-math.Ldexp(1, -54))).Eq(0))
		m.AddConstraint("r2", NewExpr(w.T(math.Ldexp(1, 24)), y.T(-math.Ldexp(1, 24)), x.T(r2XCoeff)).Le(1))
		return m.Compile()
	}

	// Ground-truth check over Q: substituting y = x and w = (1 + 2^-54)*x into r2 yields
	// (2^-30 - 2^-31)*x = +2^-31*x <= 1, bounding 0 <= x <= 2^31.
	netCoeff := new(big.Rat).Sub(
		new(big.Rat).Mul(mustAdvRatFromFloat(t, math.Ldexp(1, 24)), mustAdvRatFromFloat(t, math.Ldexp(1, -54))),
		mustAdvRatFromFloat(t, math.Ldexp(1, -31)),
	)
	if netCoeff.Cmp(mustAdvRatFromFloat(t, math.Ldexp(1, -31))) != 0 {
		t.Fatalf("unexpected exact net coefficient %v", netCoeff)
	}

	for _, mip := range []bool{false, true} {
		p := buildModel(mip, -math.Ldexp(1, -31))
		if _, ok := ExtractUnboundedCertificate(t.Context(), p, nil); ok {
			t.Fatalf("mip=%v: ExtractUnboundedCertificate falsely certified bounded model", mip)
		}
		for _, preOn := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), p, WithPresolve(preOn))
			if err != nil {
				t.Fatalf("mip=%v pre=%v err: %v", mip, preOn, err)
			}
			if res.Status == StatusUnbounded || res.Status == StatusInfeasible {
				t.Fatalf("mip=%v pre=%v: bounded feasible model returned invalid status=%v", mip, preOn, res.Status)
			}
		}
	}

	// Positive control: changing r2's x coefficient to -2^-29 makes net coeff 2^-30 - 2^-29 = -2^-30 < 0,
	// so (1, 1, 1 + 2^-54) is a genuine exact unbounded ray over Q for both LP and MILP.
	for _, mip := range []bool{false, true} {
		pUnb := buildModel(mip, -math.Ldexp(1, -29))
		for _, preOn := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), pUnb, WithPresolve(preOn))
			if err != nil || res.Status != StatusUnbounded {
				t.Fatalf("unbounded control mip=%v pre=%v: status=%v err=%v, want StatusUnbounded",
					mip, preOn, res.Status, err)
			}
		}
		cert, ok := ExtractUnboundedCertificate(t.Context(), pUnb, nil)
		if !ok || !VerifyUnboundedCertificate(pUnb, cert) {
			t.Fatalf("unbounded control mip=%v: failed exact certificate verification", mip)
		}
	}
}

func TestFormalAdversarialBug9DelayedObjConstantCancellation(t *testing.T) {
	t.Parallel()

	bigC := math.Ldexp(1, 56)

	t.Run("k5_clique_negative_cost_cancellation", func(t *testing.T) {
		t.Parallel()
		m := NewModel("bug9_k5_delayed_cancel", Minimize)
		var z [5]Var
		for i := range 5 {
			z[i] = m.AddBinaryVar("z_" + strconv.Itoa(i))
		}
		x := m.AddIntVar("x", 0, 10)
		y := m.AddBinaryVar("y")
		m.SetObjective(Expr{
			Terms: []Term{
				z[0].T(-bigC), z[1].T(-bigC), z[2].T(-bigC), z[3].T(-bigC), z[4].T(-bigC),
				x.T(3), y.T(7),
			},
			Constant: bigC,
		})
		for i := range 5 {
			for j := i + 1; j < 5; j++ {
				m.AddConstraint("clq_"+strconv.Itoa(i)+"_"+strconv.Itoa(j), NewExpr(z[i].T(1), z[j].T(1)).Le(1))
			}
		}
		m.AddConstraint("z_ge1", NewExpr(z[0].T(1), z[1].T(1), z[2].T(1), z[3].T(1), z[4].T(1)).Ge(1))
		m.AddConstraint("xy_cov", NewExpr(x.T(3), y.T(10)).Ge(5))
		p := m.Compile()

		for _, preOn := range []bool{false, true} {
			for _, cutsOn := range []bool{false, true} {
				for _, w := range []int{1, 2, 4} {
					res, err := SolveCompiled(t.Context(), p,
						WithPresolve(preOn), WithCuts(cutsOn), WithHeuristics(true), WithWorkers(w),
						WithRelGapTol(0), WithAbsGapTol(0),
					)
					if err != nil || res.Status != StatusOptimal {
						t.Fatalf("pre=%v cuts=%v w=%d: status=%v err=%v", preOn, cutsOn, w, res.Status, err)
					}
					if math.Abs(res.Objective-6.0) > 1e-6 || res.BestBound > res.Objective+1e-9 || res.BestBound > 6.0+1e-6 {
						t.Fatalf("pre=%v cuts=%v w=%d: obj=%.12g bound=%.12g, want 6 with bound<=obj",
							preOn, cutsOn, w, res.Objective, res.BestBound)
					}
				}
			}
		}

		tr := NewVIPRTrace()
		resV, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-6.0) > 1e-6 {
			t.Fatalf("VIPR k5: status=%v obj=%.12g err=%v", resV.Status, resV.Objective, errV)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})

	t.Run("odd_triangle_delayed_positive_cost_cancellation", func(t *testing.T) {
		t.Parallel()
		// Root LP achieves b0=b1=b2=0.5 so w=0 (rootLinMin=+5), while every integer point has
		// sum(b)<=1 forcing w=1 (c^T x = +2^56 + 6), cancelling ObjConstant = -2^56 only after branching.
		m := NewModel("bug9_delayed_pos_cancel", Minimize)
		b0 := m.AddBinaryVar("b0")
		b1 := m.AddBinaryVar("b1")
		b2 := m.AddBinaryVar("b2")
		w := m.AddBinaryVar("w")
		x := m.AddIntVar("x", 0, 10)
		y := m.AddBinaryVar("y")
		m.SetObjective(NewExpr(w.T(bigC), x.T(3), y.T(7)).AddConstant(-bigC))
		m.AddConstraint("e01", NewExpr(b0.T(1), b1.T(1)).Le(1))
		m.AddConstraint("e12", NewExpr(b1.T(1), b2.T(1)).Le(1))
		m.AddConstraint("e02", NewExpr(b0.T(1), b2.T(1)).Le(1))
		m.AddConstraint("force_w", NewExpr(w.T(1), b0.T(1), b1.T(1), b2.T(1)).Ge(1.5))
		m.AddConstraint("xy_cov", NewExpr(x.T(3), y.T(10)).Ge(5))
		p := m.Compile()

		for _, cutsOn := range []bool{false, true} {
			for _, heurOn := range []bool{false, true} {
				res, err := SolveCompiled(t.Context(), p,
					WithPresolve(false), WithCuts(cutsOn), WithHeuristics(heurOn),
					WithRelGapTol(0), WithAbsGapTol(0),
				)
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("cuts=%v heur=%v: status=%v err=%v", cutsOn, heurOn, res.Status, err)
				}
				if math.Abs(res.Objective-6.0) > 1e-6 || res.BestBound > res.Objective+1e-9 || res.BestBound > 6.0+1e-6 {
					t.Fatalf("cuts=%v heur=%v: obj=%.12g bound=%.12g, want 6 with bound<=obj",
						cutsOn, heurOn, res.Objective, res.BestBound)
				}
			}
		}

		tr := NewVIPRTrace()
		resV, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-6.0) > 1e-6 {
			t.Fatalf("VIPR delayed pos: status=%v obj=%.12g err=%v", resV.Status, resV.Objective, errV)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})

	t.Run("maximize_non_cancelling_huge_constant_control", func(t *testing.T) {
		t.Parallel()
		for _, xUB := range []float64{1e17, math.Inf(1)} {
			m := NewModel("bug9_max_non_cancel", Maximize)
			x := m.AddIntVar("x", 0, xUB)
			y := m.AddBinaryVar("y")
			m.SetObjective(NewExpr(x.T(-3), y.T(-7)).AddConstant(1e17))
			m.AddConstraint("cover", NewExpr(x.T(3), y.T(10)).Ge(5))
			p := m.Compile()

			res, err := SolveCompiled(t.Context(), p, WithRelGapTol(0), WithAbsGapTol(0))
			if err != nil || res.Status != StatusOptimal {
				t.Fatalf("xUB=%g: status=%v err=%v", xUB, res.Status, err)
			}
			if math.Abs(res.Values[0]-2.0) > 1e-6 || math.Abs(res.Values[1]) > 1e-6 {
				t.Fatalf("xUB=%g: vals=%v, want [2 0]", xUB, res.Values)
			}
			if res.BestBound < res.Objective-1e-9 {
				t.Fatalf("xUB=%g: inverted Maximize BestBound=%g < Objective=%g", xUB, res.BestBound, res.Objective)
			}
		}
	})
}

func TestFormalAdversarialBug10StrongBranchProbeCutOffShift(t *testing.T) {
	t.Parallel()

	bigC := math.Ldexp(1, 56)
	m := NewModel("bug10_sb_probe_shift", Minimize)
	z1 := m.AddBinaryVar("z1")
	z2 := m.AddBinaryVar("z2")
	x := m.AddIntVar("x", 0, 10)
	y := m.AddBinaryVar("y")
	u1 := m.AddBinaryVar("u1")
	u2 := m.AddBinaryVar("u2")
	u3 := m.AddBinaryVar("u3")
	m.SetObjective(NewExpr(
		z1.T(bigC), z2.T(bigC),
		x.T(4), y.T(15),
		u1.T(-1), u2.T(-1), u3.T(-1),
	).AddConstant(-bigC))
	m.AddConstraint("z_part", NewExpr(z1.T(1), z2.T(1)).Eq(1))
	m.AddConstraint("cover", NewExpr(x.T(4), y.T(16)).Ge(11))
	m.AddConstraint("u12", NewExpr(u1.T(1), u2.T(1)).Le(1))
	m.AddConstraint("u23", NewExpr(u2.T(1), u3.T(1)).Le(1))
	m.AddConstraint("u13", NewExpr(u1.T(1), u3.T(1)).Le(1))
	p := m.Compile()

	// Verify shifted SolveBoundOnly returns compensated bound ~9.5 on y <= 0 (never +16.0).
	lp := NewLPSolver(p, nil)
	defer lp.Close()
	if st := lp.Solve().Status; st != StatusOptimal {
		t.Fatalf("root LP status=%v, want StatusOptimal", st)
	}
	var cp lpCheckpoint
	lp.captureCheckpoint(&cp)
	lp.SetColBound(3, 0, 0) // y <= 0
	stP, bndP, _ := lp.solveBoundOnlyWithShift(50, p.objConstMin())
	if stP != StatusOptimal || math.IsNaN(bndP) || bndP > 9.5+1e-6 || bndP < 9.5-1e-6 {
		t.Fatalf("shifted probe y<=0: status=%v bound=%.12g, want Optimal ~9.5", stP, bndP)
	}
	lp.restoreCheckpoint(&cp)

	for _, preOn := range []bool{false, true} {
		for _, cutsOn := range []bool{false, true} {
			for _, w := range []int{1, 2, 4} {
				res, err := SolveCompiled(t.Context(), p,
					WithPresolve(preOn), WithCuts(cutsOn), WithHeuristics(true), WithWorkers(w),
					WithRelGapTol(0), WithAbsGapTol(0),
				)
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("pre=%v cuts=%v w=%d: status=%v err=%v", preOn, cutsOn, w, res.Status, err)
				}
				if math.Abs(res.Objective-11.0) > 1e-6 || res.BestBound > res.Objective+1e-9 || res.BestBound > 11.0+1e-6 {
					t.Fatalf("pre=%v cuts=%v w=%d: obj=%.12g bound=%.12g, want 11 with bound<=obj",
						preOn, cutsOn, w, res.Objective, res.BestBound)
				}
			}
		}
	}

	tr := NewVIPRTrace()
	resV, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(false), WithHeuristics(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
	if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-11.0) > 1e-6 {
		t.Fatalf("VIPR sb probe: status=%v obj=%.12g err=%v", resV.Status, resV.Objective, errV)
	}
	assertVIPRVerifiedIfFormal(t, tr, "optimal")
}

func TestFormalAdversarialBug11HeurSolverUnscaleInit(t *testing.T) {
	t.Parallel()

	m := NewModel("bug11_unscale_heur_init", Maximize)
	c0 := m.AddContVar("c0", 0, math.Inf(1))
	c1 := m.AddContVar("c1", 0, 10)
	b0 := m.AddBinaryVar("b0")
	m.SetObjective(NewExpr(c0.T(-1e-300), c1.T(1), b0.T(1)))
	m.AddConstraint("scale_row", NewExpr(c0.T(math.Ldexp(1, 30)), c1.T(1)).Ge(5))
	m.AddConstraint("couple_row", NewExpr(c0.T(1), c1.T(1)).Le(8))
	m.AddConstraint("cap_huge", NewExpr(c0.T(1), b0.T(1e306)).Le(1e306))
	p := m.Compile()

	lp := NewLPSolver(p, nil)
	defer lp.Close()
	if !lp.scaled {
		t.Fatal("expected initial LP solver to be Ruiz-scaled")
	}
	if st := lp.Solve().Status; st != StatusOptimal {
		t.Fatalf("root LP status=%v, want StatusOptimal", st)
	}
	ubNode := slices.Clone(p.varUB)
	ubNode[0] = 1e306
	lp.SetStructuralBounds(p.varLB, ubNode)
	if lp.scaled {
		t.Fatal("expected SetStructuralBounds(1e306) to unscale lp in place")
	}
	nodeRes := lp.Solve()
	if nodeRes.Status != StatusOptimal {
		t.Fatalf("node LP status=%v, want StatusOptimal", nodeRes.Status)
	}

	hs := newHeurSolverFrom(p, lp, time.Time{}, nil, false)
	defer hs.Close()
	if hs.scaled != lp.scaled {
		t.Fatalf("newHeurSolverFrom scaled=%v != lp.scaled=%v", hs.scaled, lp.scaled)
	}
	seen := make([]bool, p.numVars+p.numConstraints)
	for i := range p.numConstraints {
		bv := hs.ws.basis[i]
		if bv < 0 || bv >= len(seen) || seen[bv] {
			t.Fatalf("invalid hs.ws.basis=%v after unscale copy", hs.ws.basis[:p.numConstraints])
		}
		seen[bv] = true
	}
	hsRes := hs.Solve()
	if hsRes.Status != StatusOptimal || math.Abs(hsRes.Objective-nodeRes.Objective) > 1e-6 {
		t.Fatalf("hs.Solve() status=%v obj=%g, want Optimal %g", hsRes.Status, hsRes.Objective, nodeRes.Objective)
	}

	// Direct CopyRootBasisFrom onto a freshly scaled solver must also unscale cleanly.
	dst := NewLPSolver(p, nil)
	defer dst.Close()
	dst.CopyRootBasisFrom(lp)
	if dst.scaled || !dst.dualValid {
		t.Fatalf("CopyRootBasisFrom failed to sync unscaled state: scaled=%v dualValid=%v", dst.scaled, dst.dualValid)
	}
	dstRes := dst.Solve()
	if dstRes.Status != StatusOptimal || math.Abs(dstRes.Objective-nodeRes.Objective) > 1e-6 {
		t.Fatalf("dst.Solve() status=%v obj=%g, want Optimal %g", dstRes.Status, dstRes.Objective, nodeRes.Objective)
	}
}

func TestFormalAdversarialBug12MeyerSubTolCliffAndPresolveGCD(t *testing.T) {
	t.Parallel()

	unboundedInfeasCases := []struct {
		name string
		c0   float64
		c1   float64
		rhs  float64
	}{
		{"gcd2_5e_7", 2, -2, 5e-7},
		{"gcd2_1e_8", 2, -2, 1e-8},
		{"gcd1_5e_7", 1, -1, 5e-7},
		{"rat_half_5e_7", 0.5, -0.5, 5e-7},
	}

	for _, tc := range unboundedInfeasCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := NewModel("bug12_"+tc.name, Minimize)
			x := m.AddIntVar("x", 0, math.Inf(1))
			y := m.AddIntVar("y", 0, math.Inf(1))
			m.SetObjective(NewExpr(x.T(-1)))
			m.AddConstraint("eq", NewExpr(x.T(tc.c0), y.T(tc.c1)).Eq(tc.rhs))
			p := m.Compile()

			// Presolve must not silently round the non-integer equality RHS to 0.
			pre := Presolve(p, defaultPresolveMaxPass)
			if !pre.Infeasible && pre.Reduced != nil && pre.Reduced.numConstraints > 0 && pre.Reduced.rowRHS[0] == 0 {
				t.Fatalf("%s: Presolve silently rounded non-integer RHS %g to 0", tc.name, tc.rhs)
			}

			for _, preOn := range []bool{false, true} {
				for _, w := range []int{1, 2, 4} {
					res, err := SolveCompiled(t.Context(), p, WithPresolve(preOn), WithWorkers(w))
					if err != nil || res.Status != StatusInfeasible {
						t.Fatalf("%s pre=%v w=%d: status=%v err=%v, want StatusInfeasible",
							tc.name, preOn, w, res.Status, err)
					}
				}
				tr := NewVIPRTrace()
				resV, errV := SolveCompiled(t.Context(), p, WithPresolve(preOn), WithVIPRTrace(tr))
				if errV != nil || resV.Status != StatusInfeasible {
					t.Fatalf("%s VIPR pre=%v: status=%v err=%v, want StatusInfeasible", tc.name, preOn, resV.Status, errV)
				}
				assertVIPRVerifiedIfFormal(t, tr, "infeasible")
			}
		})
	}

	// Finite MILP tolerance contract must remain intact (sub-presolveIntTol noise accepted
	// under both presolve modes, and sub-incumbentTol noise accepted without presolve).
	t.Run("finite_milp_tolerance_contract_preserved", func(t *testing.T) {
		t.Parallel()
		mSmall := NewModel("bug12_finite_tol_1e_8", Minimize)
		xs := mSmall.AddIntVar("x", 0, 10)
		ys := mSmall.AddIntVar("y", 0, 10)
		mSmall.SetObjective(NewExpr(xs.T(-1)))
		mSmall.AddConstraint("eq", NewExpr(xs.T(2), ys.T(-2)).Eq(1e-8))
		pSmall := mSmall.Compile()
		for _, preOn := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), pSmall, WithPresolve(preOn))
			if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-(-10.0)) > 1e-6 {
				t.Fatalf("finite 1e-8 control pre=%v: status=%v obj=%g err=%v, want Optimal -10",
					preOn, res.Status, res.Objective, err)
			}
		}

		mMid := NewModel("bug12_finite_tol_5e_7", Minimize)
		xm := mMid.AddIntVar("x", 0, 10)
		ym := mMid.AddIntVar("y", 0, 10)
		mMid.SetObjective(NewExpr(xm.T(-1)))
		mMid.AddConstraint("eq", NewExpr(xm.T(2), ym.T(-2)).Eq(5e-7))
		pMid := mMid.Compile()
		resNoPre, err := SolveCompiled(t.Context(), pMid, WithPresolve(false))
		if err != nil || resNoPre.Status != StatusOptimal || math.Abs(resNoPre.Objective-(-10.0)) > 1e-6 {
			t.Fatalf("finite 5e-7 pre=false: status=%v obj=%g err=%v, want Optimal -10",
				resNoPre.Status, resNoPre.Objective, err)
		}
		// With presolve enabled, Savelsbergh tightening proves exact-Q infeasibility with a VIPR proof.
		tr := NewVIPRTrace()
		resPre, errPre := SolveCompiled(t.Context(), pMid, WithPresolve(true), WithVIPRTrace(tr))
		if errPre != nil || resPre.Status != StatusInfeasible {
			t.Fatalf("finite 5e-7 pre=true: status=%v err=%v, want StatusInfeasible", resPre.Status, errPre)
		}
		assertVIPRVerifiedIfFormal(t, tr, "infeasible")
	})

	// Opposing inequalities 2x - 2y <= 5e-7 and 2x - 2y >= 5e-7 must never certify StatusUnbounded.
	t.Run("two_ineq_sub_tol_never_unbounded", func(t *testing.T) {
		t.Parallel()
		m := NewModel("bug12_two_ineq", Minimize)
		x := m.AddIntVar("x", 0, math.Inf(1))
		y := m.AddIntVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("le", NewExpr(x.T(2), y.T(-2)).Le(5e-7))
		m.AddConstraint("ge", NewExpr(x.T(2), y.T(-2)).Ge(5e-7))
		p := m.Compile()

		for _, preOn := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), p, WithPresolve(preOn))
			if err != nil {
				t.Fatalf("two_ineq pre=%v err: %v", preOn, err)
			}
			if res.Status == StatusUnbounded {
				t.Fatalf("two_ineq pre=%v: exact-Q infeasible MILP falsely certified StatusUnbounded", preOn)
			}
		}
	})
}

func TestFormalAdversarialBug13HeuristicIncumbentTol(t *testing.T) {
	t.Parallel()

	t.Run("seed21_multi_row_cont_completion_and_exact_opt", func(t *testing.T) {
		t.Parallel()
		m := NewModel("bug13_seed21", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, 5)
		x2 := m.AddContVar("x2", 0, 5)
		x3 := m.AddBinaryVar("x3")
		m.SetObjective(NewExpr(x0.T(3), x1.T(3), x2.T(3), x3.T(4)))
		m.AddConstraint("r0", NewExpr(x0.T(-1e-6), x1.T(1e12), x2.T(-1e-12), x3.T(1e6)).Le(0))
		m.AddConstraint("r1", NewExpr(x0.T(1e-10), x1.T(-2e11), x2.T(-1e-6), x3.T(2e11)).Ge(-1e-9))
		p := m.Compile()

		// Exact optimum: x0=1, x1=0, x2=11/10000, x3=0 => objective 30033/10000.
		wantObjRat := big.NewRat(30033, 10000)
		wantObj, _ := wantObjRat.Float64()

		// Propagated continuous bounds pad x2 outward to 1.0011 (rV = 1e-6).
		// trySingleRowContinuousCompletion must not accept this infeasible corner.
		hlb := []float64{1, 0, 0, 0}
		hub := []float64{1, 0, 1.0011, 0}
		if sol, _, ok := trySingleRowContinuousCompletion(p, hlb, hub); ok {
			t.Fatalf("trySingleRowContinuousCompletion accepted padded infeasible corner: sol=%v", sol)
		}

		for _, preOn := range []bool{false, true} {
			for _, heurOn := range []bool{false, true} {
				for _, w := range []int{1, 2, 4} {
					res, err := SolveCompiled(t.Context(), p,
						WithPureGo(), WithPresolve(preOn), WithCuts(true),
						WithHeuristics(heurOn), WithWorkers(w),
						WithRelGapTol(0), WithAbsGapTol(0),
					)
					if err != nil || res.Status != StatusOptimal {
						t.Fatalf("pre=%v heur=%v w=%d: status=%v err=%v", preOn, heurOn, w, res.Status, err)
					}
					if math.Abs(res.Objective-wantObj) > 1e-8 {
						t.Fatalf("pre=%v heur=%v w=%d: obj=%.12g, want %.12g (%s)",
							preOn, heurOn, w, res.Objective, wantObj, wantObjRat.RatString())
					}
					if res.BestBound < res.Objective-1e-9 {
						t.Fatalf("pre=%v heur=%v w=%d: inverted bound %.12g < obj %.12g",
							preOn, heurOn, w, res.BestBound, res.Objective)
					}
					bV, rV, iV := p.MaxViolation(res.Values, true)
					if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
						t.Fatalf("pre=%v heur=%v w=%d: residual (%g,%g,%g) exceeds 1e-9",
							preOn, heurOn, w, bV, rV, iV)
					}
				}
			}
		}

		tr := NewVIPRTrace()
		resV, errV := SolveCompiled(t.Context(), p,
			WithPureGo(), WithPresolve(false), WithCuts(true), WithHeuristics(true),
			WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr),
		)
		if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-wantObj) > 1e-8 {
			t.Fatalf("VIPR seed21: status=%v obj=%.12g err=%v", resV.Status, resV.Objective, errV)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})

	t.Run("seed22_tiny_norm_row_scaled_incumbent_and_weak_duality", func(t *testing.T) {
		t.Parallel()
		m := NewModel("bug13_seed22", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, 5)
		x2 := m.AddContVar("x2", 0, 1e9)
		x3 := m.AddBinaryVar("x3")
		x4 := m.AddIntVar("x4", 0, 5)
		m.SetObjective(NewExpr(x0.T(4), x1.T(5), x2.T(1), x3.T(5), x4.T(2)))
		m.AddConstraint("r0", NewExpr(x0.T(1e-10), x1.T(-1e-9), x2.T(1e-9), x3.T(2e11), x4.T(1e-9)).Le(-1e-12))
		m.AddConstraint("r1", NewExpr(x0.T(1e-12), x1.T(1e12), x2.T(1), x3.T(2e11), x4.T(-1e-6)).Ge(-1e-9))
		p := m.Compile()

		pre := Presolve(p, defaultPresolveMaxPass)
		if pre.Infeasible || pre.Reduced == nil {
			t.Fatalf("expected feasible presolved model, got infeas=%v reduced=%v", pre.Infeasible, pre.Reduced)
		}
		zeros := make([]float64, pre.Reduced.numVars)
		if incumbentFeasibleScaled(pre.Reduced, zeros, primalTol) {
			t.Fatal("incumbentFeasibleScaled accepted zero vector on tiny-norm violated row")
		}

		wantObjRat := big.NewRat(5, 1)
		wantObj, _ := wantObjRat.Float64()

		for _, heurOn := range []bool{false, true} {
			for _, w := range []int{1, 2, 4} {
				res, err := SolveCompiled(t.Context(), p,
					WithPureGo(), WithPresolve(true), WithCuts(true),
					WithHeuristics(heurOn), WithWorkers(w),
					WithRelGapTol(0), WithAbsGapTol(0),
				)
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("heur=%v w=%d: status=%v err=%v", heurOn, w, res.Status, err)
				}
				if math.Abs(res.Objective-wantObj) > 1e-8 {
					t.Fatalf("heur=%v w=%d: obj=%.12g, want %.12g", heurOn, w, res.Objective, wantObj)
				}
				if res.BestBound < res.RootLPBound-1e-9 || res.BestBound > res.Objective+1e-9 {
					t.Fatalf("heur=%v w=%d: weak duality violated (rootLP=%.12g bestBound=%.12g obj=%.12g)",
						heurOn, w, res.RootLPBound, res.BestBound, res.Objective)
				}
				bV, rV, iV := p.MaxViolation(res.Values, true)
				if bV > 0 || rV > 0 || iV > 0 {
					t.Fatalf("heur=%v w=%d: non-zero residual (%g,%g,%g)", heurOn, w, bV, rV, iV)
				}
			}
		}

		tr := NewVIPRTrace()
		resV, errV := SolveCompiled(t.Context(), pre.Reduced,
			WithPureGo(), WithPresolve(false), WithCuts(true), WithHeuristics(true),
			WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr),
		)
		if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-wantObj) > 1e-8 {
			t.Fatalf("VIPR reduced seed22: status=%v obj=%.12g err=%v", resV.Status, resV.Objective, errV)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})

	t.Run("polish_rejects_lp_infeasible_and_resolves_fixed_int_leaf", func(t *testing.T) {
		t.Parallel()

		// Tiny-norm infeasible row: raw rV=1e-12 <= 1e-6, but fixed-integer LP is infeasible.
		mInf := NewModel("bug13_polish_infeas", Minimize)
		b0 := mInf.AddBinaryVar("b0")
		c0 := mInf.AddContVar("c0", 0, 1)
		mInf.SetObjective(NewExpr(b0.T(1), c0.T(1)))
		mInf.AddConstraint("tiny_infeas", NewExpr(b0.T(1e-9), c0.T(1e-9)).Le(-1e-12))
		pInf := mInf.Compile()

		hs := NewLPSolver(pInf, nil)
		defer hs.Close()
		var fixLB, fixUB []float64
		if cand, ok := polishIncumbentWith(pInf, time.Time{}, nil, []float64{0, 0}, nil, &fixLB, &fixUB, func() *LPSolver { return hs }); ok {
			t.Fatalf("polishIncumbentWith accepted LP-infeasible candidate: %v", cand)
		}

		// Cut-relaxed fixed-integer leaf: must close to exact 64 even with heuristics off.
		mLeaf := NewModel("bug13_fixed_leaf", Minimize)
		x := mLeaf.AddContVar("x", 0, 4)
		y := mLeaf.AddContVar("y", -1, 1)
		z := mLeaf.AddIntVar("z", 0, 100)
		mLeaf.SetObjective(NewExpr(x.T(1), y.T(4), z.T(9)))
		mLeaf.AddConstraint("c1", NewExpr(x.T(1), y.T(1)).Le(5))
		mLeaf.AddConstraint("c2", NewExpr(x.T(1), z.T(1)).Ge(10))
		mLeaf.AddConstraint("c3", NewExpr(y.T(-1), z.T(1)).Eq(7.5))
		pLeaf := mLeaf.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(t.Context(), pLeaf,
			WithPureGo(), WithPresolve(false), WithCuts(true), WithHeuristics(false),
			WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr),
		)
		if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-64.0) > 1e-9 || math.Abs(res.BestBound-64.0) > 1e-9 {
			t.Fatalf("fixed-int leaf VIPR: status=%v obj=%.12g bound=%.12g err=%v",
				res.Status, res.Objective, res.BestBound, err)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")
	})

	t.Run("finite_milp_1e_6_contract_preserved", func(t *testing.T) {
		t.Parallel()
		m := NewModel("bug13_finite_1e6_control", Minimize)
		x := m.AddIntVar("x", 0, 10)
		y := m.AddIntVar("y", 0, 10)
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("eq", NewExpr(x.T(2), y.T(-2)).Eq(5e-7))
		p := m.Compile()

		if !incumbentFeasibleScaled(p, []float64{10, 10}, incumbentTol) {
			t.Fatal("incumbentFeasibleScaled rejected standard-norm finite MILP 1e-6 contract point")
		}
		res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(false))
		if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-(-10.0)) > 1e-6 {
			t.Fatalf("finite 1e-6 control: status=%v obj=%.12g err=%v, want Optimal -10",
				res.Status, res.Objective, err)
		}
	})
}

func TestFormalAdversarialBug14SubnormalRuizAndFixedSupportTol(t *testing.T) {
	t.Parallel()

	buildSeed25 := func() *CompiledProblem {
		m := NewModel("bug14_seed25", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, 5)
		x2 := m.AddContVar("x2", 0, 1)
		x3 := m.AddBinaryVar("x3")
		m.SetObjective(NewExpr(x0.T(5), x1.T(4), x2.T(5), x3.T(4)))
		m.AddConstraint("r0", NewExpr(x0.T(-1e-12), x1.T(2e11), x2.T(-1e-9), x3.T(1)).Le(1e-9))
		m.AddConstraint("r1", NewExpr(x0.T(-1e-9), x1.T(1), x2.T(-math.Ldexp(1, -1000)), x3.T(-1e-6)).Ge(-1e-12))
		return m.Compile()
	}

	t.Run("seed25_fresh_lp_and_full_solve_matrix", func(t *testing.T) {
		t.Parallel()
		p := buildSeed25()

		// Fresh crash-basis LP on fixed-zero support [1,0,*,0] must detect exact infeasibility.
		lpFresh := NewLPSolver(p, nil)
		defer lpFresh.Close()
		lpFresh.SetStructuralBounds([]float64{1, 0, 0, 0}, []float64{1, 0, 1, 0})
		if resF := lpFresh.Solve(); resF.Status != StatusInfeasible {
			t.Fatalf("fresh LP on [1,0,*,0]: status=%v obj=%.12g primal=%v, want StatusInfeasible",
				resF.Status, resF.Objective, resF.Primal)
		}

		// Domain-scaled root dual amplification must reject [1,0,1,0] while accepting exact [0,0,1,0].
		lpRoot := NewLPSolver(p, nil)
		defer lpRoot.Close()
		rootRes := lpRoot.Solve()
		if rootRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v, want StatusOptimal", rootRes.Status)
		}
		dualAmp := computeRootDualAmp(p.numConstraints, rootRes.Dual, maxAbsFloat64(p.objMin))
		if incumbentFeasibleWithDual(p, []float64{1, 0, 1, 0}, dualAmp, incumbentTol) {
			t.Fatal("incumbentFeasibleWithDual accepted dual-amplified infeasible point [1,0,1,0]")
		}
		if !incumbentFeasibleWithDual(p, []float64{0, 0, 1, 0}, dualAmp, incumbentTol) {
			t.Fatal("incumbentFeasibleWithDual rejected exact optimum [0,0,1,0]")
		}

		wantObjRat := big.NewRat(5, 1)
		wantObj, _ := wantObjRat.Float64()

		for _, preOn := range []bool{false, true} {
			for _, cutsOn := range []bool{false, true} {
				for _, heurOn := range []bool{false, true} {
					for _, w := range []int{1, 2, 4} {
						res, err := SolveCompiled(t.Context(), p,
							WithPureGo(), WithPresolve(preOn), WithCuts(cutsOn),
							WithHeuristics(heurOn), WithWorkers(w),
							WithRelGapTol(0), WithAbsGapTol(0),
						)
						if err != nil || res.Status != StatusOptimal {
							t.Fatalf("pre=%v cuts=%v heur=%v w=%d: status=%v err=%v",
								preOn, cutsOn, heurOn, w, res.Status, err)
						}
						if math.Abs(res.Objective-wantObj) > 1e-8 {
							t.Fatalf("pre=%v cuts=%v heur=%v w=%d: obj=%.12g, want %.12g",
								preOn, cutsOn, heurOn, w, res.Objective, wantObj)
						}
						if res.BestBound > res.RootLPBound+1e-9 || res.BestBound < res.Objective-1e-9 {
							t.Fatalf("pre=%v cuts=%v heur=%v w=%d: invalid bounds (obj=%.12g bound=%.12g rootLP=%.12g)",
								preOn, cutsOn, heurOn, w, res.Objective, res.BestBound, res.RootLPBound)
						}
						bV, rV, iV := p.MaxViolation(res.Values, true)
						if bV > 0 || rV > 0 || iV > 0 {
							t.Fatalf("pre=%v cuts=%v heur=%v w=%d: non-zero residual (%g,%g,%g)",
								preOn, cutsOn, heurOn, w, bV, rV, iV)
						}
					}
				}
			}
		}

		tr := NewVIPRTrace()
		resV, errV := SolveCompiled(t.Context(), p,
			WithPureGo(), WithPresolve(false), WithCuts(true), WithHeuristics(true),
			WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr),
		)
		if errV != nil || resV.Status != StatusOptimal || math.Abs(resV.Objective-wantObj) > 1e-8 {
			t.Fatalf("VIPR seed25: status=%v obj=%.12g bound=%.12g err=%v",
				resV.Status, resV.Objective, resV.BestBound, errV)
		}
		assertVIPRVerifiedIfFormal(t, tr, "")
		if tr.HasCertificate() && tr.TargetLB() != nil {
			lbF, _ := tr.TargetLB().Float64()
			if lbF < -wantObj-1e-6 || lbF > -wantObj+1e-9 {
				t.Fatalf("VIPR seed25 TargetLB=%s (%.12g), want ~%.12g", tr.TargetLB().RatString(), lbF, -wantObj)
			}
		}
	})

	t.Run("continuous_rounding_under_large_dual_and_finite_1e6_control", func(t *testing.T) {
		t.Parallel()

		// Valid continuous coordinate 1/3 under a 1e12 dual must remain accepted via the 16-ulp floor.
		mCont := NewModel("bug14_cont_ulp_floor", Minimize)
		xc := mCont.AddContVar("x", 0, 1)
		mCont.SetObjective(NewExpr(xc.T(1)))
		mCont.AddConstraint("eq", NewExpr(xc.T(3)).Eq(1))
		pCont := mCont.Compile()
		if !incumbentFeasibleWithDual(pCont, []float64{1.0 / 3.0}, []float64{1e12}, incumbentTol) {
			t.Fatal("incumbentFeasibleWithDual rejected float64 1/3 coordinate under huge dual")
		}

		// Standard-norm finite MILP 1e-6 contract must remain intact.
		mFin := NewModel("bug14_finite_1e6", Minimize)
		x := mFin.AddIntVar("x", 0, 10)
		y := mFin.AddIntVar("y", 0, 10)
		mFin.SetObjective(NewExpr(x.T(-1)))
		mFin.AddConstraint("eq", NewExpr(x.T(2), y.T(-2)).Eq(5e-7))
		pFin := mFin.Compile()
		res, err := SolveCompiled(t.Context(), pFin, WithPureGo(), WithPresolve(false))
		if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-(-10.0)) > 1e-6 {
			t.Fatalf("finite 1e-6 control: status=%v obj=%.12g err=%v, want Optimal -10",
				res.Status, res.Objective, err)
		}
	})

	t.Run("subnormal_active_support_infeasible_never_ghost_incumbent", func(t *testing.T) {
		t.Parallel()
		// Exact-Q infeasible MIP from live fuzz seed {70, 62, 0}:
		// r0 forces x1 >= 1, r1 forces x1 <= 0; [1, 0] has r0 residual 1e-12 on active coeff 2^-1000.
		m := NewModel("bug14_ghost_infeas", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddIntVar("x1", 0, 5)
		m.SetObjective(NewExpr(x0.T(4), x1.T(3)))
		m.AddConstraint("r0", NewExpr(x0.T(math.Ldexp(1, -1000)), x1.T(-2e11)).Le(-1e-12))
		m.AddConstraint("r1", NewExpr(x0.T(1e-12), x1.T(-1e-9)).Ge(0))
		p := m.Compile()

		if incumbentFeasibleScaled(p, []float64{1, 0}, incumbentTol) {
			t.Fatal("incumbentFeasibleScaled accepted ghost candidate [1, 0] with 1e-12 residual on 2^-1000 active coeff")
		}
		for _, preOn := range []bool{false, true} {
			for _, heurOn := range []bool{false, true} {
				tr := NewVIPRTrace()
				res, err := SolveCompiled(t.Context(), p,
					WithPureGo(), WithPresolve(preOn), WithCuts(true), WithHeuristics(heurOn),
					WithVIPRTrace(tr),
				)
				if err != nil || res.Status != StatusInfeasible {
					t.Fatalf("pre=%v heur=%v: status=%v obj=%g err=%v, want StatusInfeasible",
						preOn, heurOn, res.Status, res.Objective, err)
				}
				assertVIPRVerifiedIfFormal(t, tr, "infeasible")
			}
		}
	})
}

func TestFastCertifyUnboundedRayFastPathAndNegativeControls(t *testing.T) {
	t.Run("unambiguous_unbounded_lp_fast_path", func(t *testing.T) {
		const nVars, nRows = 60, 40
		m := NewModel("fast_unb_lp", Minimize)
		x := make([]Var, nVars)
		objTerms := make([]Term, nVars)
		sumTerms := make([]Term, nVars)
		for j := range nVars {
			x[j] = m.AddContVar(fmt.Sprintf("x%d", j), 0, math.Inf(1))
			objTerms[j] = x[j].T(-1.0 - 0.01*float64(j%7))
			sumTerms[j] = x[j].T(1.0)
		}
		m.SetObjective(NewExpr(objTerms...))
		m.AddConstraint("min_sum", NewExpr(sumTerms...).Ge(float64(nVars)))
		for i := range nRows - 1 {
			a := i % (nVars - 1)
			b := (i + 1) % nVars
			c := (i + 7) % nVars
			m.AddConstraint(fmt.Sprintf("chain_%d", i), NewExpr(x[a].T(1), x[b].T(-1), x[c].T(-0.5)).Le(2.0))
		}
		p := m.Compile()
		ws := AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer ReleaseWorkspace(ws)

		// Warm up workspace buffers once, then assert low-allocation fast certification (no *big.Rat snapshot).
		lp := NewLPSolver(p, ws)
		if res := lp.Solve(); res.Status != StatusUnbounded {
			t.Fatalf("warmup status = %v, want StatusUnbounded", res.Status)
		}
		allocs := testing.AllocsPerRun(20, func() {
			lp2 := NewLPSolver(p, ws)
			if res := lp2.Solve(); res.Status != StatusUnbounded {
				t.Fatalf("status = %v, want StatusUnbounded", res.Status)
			}
		})
		if allocs > 10 {
			t.Fatalf("fastCertifyUnboundedRay regressed into *big.Rat fallback: allocs/run = %.1f (want <= 10)", allocs)
		}
	})

	t.Run("negative_control_near_cancellation_rejects_fast_path", func(t *testing.T) {
		m := NewModel("fast_unb_neg", Minimize)
		x0 := m.AddContVar("x0", 0, math.Inf(1))
		x1 := m.AddContVar("x1", 0, math.Inf(1))
		m.SetObjective(NewExpr(x0.T(-1), x1.T(-1)))
		// (1 + 2^-52)*x0 - x1 <= 1 is bounded in direction (1, 1) since (1+2^-52)*1 - 1 = +2^-52 > 0.
		m.AddConstraint("tight", NewExpr(x0.T(math.Nextafter(1.0, 2.0)), x1.T(-1)).Le(1))
		p := m.Compile()
		ws := AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer ReleaseWorkspace(ws)
		lp := NewLPSolver(p, ws)
		// Construct a synthetic basis where x1 is basic in row 0 with fCol = [-1] so ray = [1, 1].
		lp.ws.basis[0] = 1
		lp.ws.status[0] = statusAtLower
		lp.ws.status[1] = statusBasic
		lp.ws.status[2] = statusAtLower
		fCol := []float64{-1.0}
		if lp.fastCertifyUnboundedRay(0, 1.0, fCol) {
			t.Fatal("fastCertifyUnboundedRay falsely certified blocked direction (1, 1) with +2^-52 residual")
		}
	})
}

func TestFormalAdversarialExactIntGcdDivisionNoRelaxation(t *testing.T) {
	t.Parallel()

	t.Run("exact_int_coeffs_sub_inttol_rhs_divides_exact_floor_ceil", func(t *testing.T) {
		t.Parallel()
		// 2*x + 2*y <= 3.99999995 has exact integer coefficients (maxRoundErr == 0),
		// so GCD division by 2 must tighten to x + y <= 1 (not x + y <= 2).
		mLE := NewModel("gcd_exact_le_sub_inttol", Maximize)
		x := mLE.AddBinaryVar("x")
		y := mLE.AddBinaryVar("y")
		mLE.SetObjective(NewExpr(x.T(1), y.T(1)))
		mLE.AddConstraint("le", NewExpr(x.T(2), y.T(2)).Le(3.99999995))
		pLE := mLE.Compile()

		itemsLE, preLE := ExportPresolveRulesItems(pLE)
		if preLE.Infeasible || preLE.Reduced == nil || preLE.Reduced.numConstraints != 1 {
			t.Fatalf("unexpected Presolve result on pLE: %+v", preLE)
		}
		if preLE.Reduced.rowRHS[0] != 1.0 {
			t.Fatalf("GCD division relaxed exact integer row to RHS=%g, want 1.0", preLE.Reduced.rowRHS[0])
		}
		foundRawRHS := false
		for _, raw := range itemsLE {
			if strings.Contains(string(raw), `"gcd-div-check"`) {
				var dec exportGcdDivCheckItem
				if err := json.Unmarshal(raw, &dec); err != nil {
					t.Fatalf("unmarshal gcd-div-check: %v", err)
				}
				wantRHS, _ := float64ToPayload(3.99999995)
				if dec.RHS != wantRHS || dec.DivRHS == nil || *dec.DivRHS != 1 {
					t.Fatalf("exported gcd-div-check rewrote RHS or DivRHS: got rhs=%+v div_rhs=%v, want %+v and 1",
						dec.RHS, dec.DivRHS, wantRHS)
				}
				foundRawRHS = true
			}
			if FormalOracleEnabled() {
				resp, err := FormalCheckRulesJSONL(t, raw)
				if err != nil || !resp.OK {
					t.Fatalf("rules-check rejected live item %s: resp=%+v err=%v", raw, resp, err)
				}
			}
		}
		if !foundRawRHS {
			t.Fatal("expected live gcd-div-check item from ExportPresolveRulesItems")
		}

		for _, preOn := range []bool{true, false} {
			res, err := SolveCompiled(t.Context(), pLE, WithPresolve(preOn))
			if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-1.0) > 1e-9 {
				t.Fatalf("pLE pre=%v: status=%v obj=%.12g err=%v, want Optimal 1",
					preOn, res.Status, res.Objective, err)
			}
		}
		resCert, errCert := SolveCompiled(t.Context(), pLE, WithCertified(true))
		if errCert != nil || resCert.Status != StatusOptimal || resCert.Objective != 1.0 {
			t.Fatalf("pLE certified: status=%v obj=%.12g err=%v, want Optimal 1",
				resCert.Status, resCert.Objective, errCert)
		}

		// Companion GreaterEqual row: 2*x + 2*y >= 2.00000005 on {0, 1, 2} must divide to x + y >= 2 (obj = 2).
		mGE := NewModel("gcd_exact_ge_sub_inttol", Minimize)
		gx := mGE.AddIntVar("x", 0, 2)
		gy := mGE.AddIntVar("y", 0, 2)
		mGE.SetObjective(NewExpr(gx.T(1), gy.T(1)))
		mGE.AddConstraint("ge", NewExpr(gx.T(2), gy.T(2)).Ge(2.00000005))
		pGE := mGE.Compile()

		itemsGE, preGE := ExportPresolveRulesItems(pGE)
		if preGE.Infeasible || preGE.Reduced == nil || preGE.Reduced.numConstraints != 1 || preGE.Reduced.rowRHS[0] != 2.0 {
			t.Fatalf("expected GreaterEqual GCD division RHS=2.0, got %+v", preGE)
		}
		if FormalOracleEnabled() {
			for _, raw := range itemsGE {
				resp, err := FormalCheckRulesJSONL(t, raw)
				if err != nil || !resp.OK {
					t.Fatalf("rules-check rejected GE live item %s: resp=%+v err=%v", raw, resp, err)
				}
			}
		}
	})

	t.Run("rounded_near_int_coeffs_compensate_domain_error_soundly", func(t *testing.T) {
		t.Parallel()
		// (2 - 1e-10)*x + 2*y <= 3.99999999985 on x, y in {0, 1, 2} is satisfied over Q at (x=2, y=0)
		// since 2*(2 - 1e-10) = 3.9999999998 <= 3.99999999985. Coefficient rounding compensation
		// (maxRoundErr > 0) must keep x + y <= 2 while tightening to x + y <= 1 when RHS = 3.99999995.
		mFeas := NewModel("gcd_rounded_feas", Maximize)
		fx := mFeas.AddIntVar("x", 0, 2)
		fy := mFeas.AddIntVar("y", 0, 2)
		mFeas.SetObjective(NewExpr(fx.T(1), fy.T(1)))
		mFeas.AddConstraint("le", NewExpr(fx.T(2.0-1e-10), fy.T(2.0)).Le(3.99999999985))
		pFeas := mFeas.Compile()

		itemsFeas, preFeas := ExportPresolveRulesItems(pFeas)
		if preFeas.Infeasible || preFeas.Reduced == nil || preFeas.Reduced.numConstraints != 1 || preFeas.Reduced.rowRHS[0] != 2.0 {
			t.Fatalf("rounded feasible GCD row failed to preserve (2,0): pre=%+v", preFeas)
		}
		if FormalOracleEnabled() {
			for _, raw := range itemsFeas {
				resp, err := FormalCheckRulesJSONL(t, raw)
				if err != nil || !resp.OK {
					t.Fatalf("rules-check rejected rounded item %s: resp=%+v err=%v", raw, resp, err)
				}
			}
		}
		resF, errF := SolveCompiled(t.Context(), pFeas, WithPresolve(true))
		if errF != nil || resF.Status != StatusOptimal || math.Abs(resF.Objective-2.0) > 1e-9 {
			t.Fatalf("rounded feasible solve: status=%v obj=%g err=%v, want Optimal 2", resF.Status, resF.Objective, errF)
		}

		mTight := NewModel("gcd_rounded_tight", Maximize)
		tx := mTight.AddIntVar("x", 0, 2)
		ty := mTight.AddIntVar("y", 0, 2)
		mTight.SetObjective(NewExpr(tx.T(1), ty.T(1)))
		mTight.AddConstraint("le", NewExpr(tx.T(2.0-1e-10), ty.T(2.0)).Le(3.99999995))
		pTight := mTight.Compile()

		_, preTight := ExportPresolveRulesItems(pTight)
		if preTight.Infeasible || preTight.Reduced == nil || preTight.Reduced.numConstraints != 1 || preTight.Reduced.rowRHS[0] != 1.0 {
			t.Fatalf("rounded tight GCD row over-relaxed: pre=%+v", preTight)
		}
	})
}
