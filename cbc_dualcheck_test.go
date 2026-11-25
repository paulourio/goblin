//go:build cbc && cgo

package goblin_test

import (
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
)

func buildUnboundedNegInfMaxModel() *gb.Model {
	m := gb.NewModel("cbc_dualcheck_neginf_max", gb.Maximize)
	a := m.AddContVar("a", -2, 4)
	v8 := m.AddContVar("v8", -1, 1)
	l9 := m.AddContVar("l9", 0, 2)
	c3 := m.AddContVar("c3", math.Inf(-1), 0)
	c4 := m.AddContVar("c4", 0, 5)
	c5 := m.AddContVar("c5", math.Inf(-1), 1)
	c6 := m.AddContVar("c6", math.Inf(-1), 1)

	m.AddConstraint("r_0", gb.NewExpr().Ge(-5))
	m.AddConstraint("r_1", gb.NewExpr(c3.T(-2), c6.T(-2)).Ge(-3))
	m.AddConstraint("r_2", gb.NewExpr(v8.T(-0.5), c3.T(-0.5), c4.T(-0.5), c5.T(-0.5)).Ge(2))
	m.AddConstraint("r_3", gb.NewExpr(a.T(1.5), v8.T(1), l9.T(1.5)).Le(0.5))
	m.AddConstraint("r_4", gb.NewExpr(l9.T(1.5), c4.T(2), c5.T(-1)).Ge(0.5))

	m.SetObjective(gb.NewExpr(
		a.T(-0.5),
		v8.T(-2.5),
		l9.T(-3),
		c3.T(2),
		c4.T(3),
		c5.T(2),
		c6.T(-0.5),
	).AddConstant(-3.5))
	return m
}

func buildUnboundedPosInfMinModel() *gb.Model {
	// Sign-flipped mirror where half-infinite [lb, +Inf) variables drive the objective to -Inf.
	m := gb.NewModel("cbc_dualcheck_posinf_min", gb.Minimize)
	a := m.AddContVar("a", -4, 2)
	v8 := m.AddContVar("v8", -1, 1)
	l9 := m.AddContVar("l9", -2, 0)
	c3 := m.AddContVar("c3", 0, math.Inf(1))
	c4 := m.AddContVar("c4", -5, 0)
	c5 := m.AddContVar("c5", -1, math.Inf(1))
	c6 := m.AddContVar("c6", -1, math.Inf(1))

	m.AddConstraint("r_0", gb.NewExpr().Ge(-5))
	m.AddConstraint("r_1", gb.NewExpr(c3.T(2), c6.T(2)).Ge(-3))
	m.AddConstraint("r_2", gb.NewExpr(v8.T(0.5), c3.T(0.5), c4.T(0.5), c5.T(0.5)).Ge(2))
	m.AddConstraint("r_3", gb.NewExpr(a.T(-1.5), v8.T(-1), l9.T(-1.5)).Le(0.5))
	m.AddConstraint("r_4", gb.NewExpr(l9.T(-1.5), c4.T(-2), c5.T(1)).Ge(0.5))

	m.SetObjective(gb.NewExpr(
		a.T(-0.5),
		v8.T(-2.5),
		l9.T(-3),
		c3.T(2),
		c4.T(3),
		c5.T(2),
		c6.T(-0.5),
	).AddConstant(3.5))
	return m
}

func buildUnboundedFreeMinModel() *gb.Model {
	// Free variable (-Inf, +Inf) coupled with a slack-active row to test both dual checks together.
	m := gb.NewModel("cbc_dualcheck_free_min", gb.Minimize)
	x0 := m.AddContVar("x0", math.Inf(-1), math.Inf(1))
	x1 := m.AddContVar("x1", math.Inf(-1), 0)
	x2 := m.AddContVar("x2", -1, 1)
	x3 := m.AddContVar("x3", 0, 5)
	x4 := m.AddContVar("x4", math.Inf(-1), 1)

	m.AddConstraint("r_free", gb.NewExpr(x1.T(-2), x0.T(-2)).Ge(-3))
	m.AddConstraint("r_slack", gb.NewExpr(x2.T(-0.5), x1.T(-0.5), x3.T(-0.5), x4.T(-0.5)).Ge(2))
	m.AddConstraint("r_bnd", gb.NewExpr(x3.T(2), x4.T(-1)).Ge(0.5))

	m.SetObjective(gb.NewExpr(
		x2.T(2.5),
		x1.T(-2),
		x3.T(-3),
		x4.T(-2),
		x0.T(0.5),
	))
	return m
}

func assertAllBackendsUnbounded(t *testing.T, name string, p *gb.CompiledProblem) {
	t.Helper()
	ctx := t.Context()

	resGo, err := gb.SolveCompiled(ctx, p, gb.WithPureGo())
	if err != nil || resGo.Status != gb.StatusUnbounded {
		t.Fatalf("%s PureGo: status=%v err=%v, want Unbounded", name, resGo.Status, err)
	}

	for _, tc := range []struct {
		label string
		opts  []gb.Option
	}{
		{"cbc_default", []gb.Option{gb.WithCBC()}},
		{"cbc_nopresolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false)}},
	} {
		resCBC, err := gb.SolveCompiled(ctx, p, tc.opts...)
		if err != nil {
			t.Fatalf("%s %s err: %v", name, tc.label, err)
		}
		if resCBC.Backend != gb.BackendCBC {
			t.Fatalf("%s %s backend=%v, want BackendCBC", name, tc.label, resCBC.Backend)
		}
		if resCBC.Status != gb.StatusUnbounded {
			t.Fatalf("%s %s: status=%v obj=%g fellBack=%v, want Unbounded",
				name, tc.label, resCBC.Status, resCBC.Objective, resCBC.FellBackToGo)
		}
	}

	clp := gb.NewCBCLPSolver(p)
	defer clp.Close()
	rClp := clp.Solve()
	if rClp.Status != gb.StatusUnbounded {
		t.Fatalf("%s CBCLPSolver: status=%v obj=%g, want Unbounded", name, rClp.Status, rClp.Objective)
	}
}

func TestCBCLPDualCheckInfiniteBoundUnbounded(t *testing.T) {
	t.Parallel()

	t.Run("neginf_maximize_repro", func(t *testing.T) {
		t.Parallel()
		p := buildUnboundedNegInfMaxModel().Compile()
		assertAllBackendsUnbounded(t, "neginf_max", p)
	})

	t.Run("posinf_minimize_mirror", func(t *testing.T) {
		t.Parallel()
		p := buildUnboundedPosInfMinModel().Compile()
		assertAllBackendsUnbounded(t, "posinf_min", p)
	})

	t.Run("free_var_minimize", func(t *testing.T) {
		t.Parallel()
		p := buildUnboundedFreeMinModel().Compile()
		assertAllBackendsUnbounded(t, "free_min", p)
	})

	t.Run("coupled_neginf_posinf_unbounded", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("coupled_inf", gb.Minimize)
		x := m.AddContVar("x", math.Inf(-1), 2)
		y := m.AddContVar("y", -2, math.Inf(1))
		z := m.AddContVar("z", math.Inf(-1), math.Inf(1))
		m.AddConstraint("eq", gb.NewExpr(x.T(1), y.T(1), z.T(-1)).Eq(0))
		m.AddConstraint("cap", gb.NewExpr(y.T(1), z.T(-1)).Le(1))
		m.SetObjective(gb.NewExpr(x.T(1), y.T(-0.5), z.T(0.25)))
		assertAllBackendsUnbounded(t, "coupled_inf", m.Compile())
	})
}

func TestCBCLPDualCheckInfiniteBoundPositiveAndWarmStart(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("bounded_mixed_infinite_bounds_optimal", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("bounded_mixed_inf", gb.Maximize)
		xNeg := m.AddContVar("x_neg", math.Inf(-1), 4)
		xPos := m.AddContVar("x_pos", -2, math.Inf(1))
		xFree := m.AddContVar("x_free", math.Inf(-1), math.Inf(1))
		m.AddConstraint("c1", gb.NewExpr(xNeg.T(-1), xPos.T(1)).Le(6))
		m.AddConstraint("c2", gb.NewExpr(xNeg.T(1), xFree.T(2)).Ge(2))
		m.AddConstraint("c3", gb.NewExpr(xPos.T(1), xFree.T(-1)).Le(5))
		m.SetObjective(gb.NewExpr(xNeg.T(2), xPos.T(-1), xFree.T(-1)).AddConstant(1.5))
		p := m.Compile()

		resGo, err := gb.SolveCompiled(ctx, p, gb.WithPureGo())
		if err != nil || resGo.Status != gb.StatusOptimal {
			t.Fatalf("PureGo: status=%v err=%v, want Optimal", resGo.Status, err)
		}
		resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resCBC.Status != gb.StatusOptimal {
			t.Fatalf("CBC: status=%v err=%v, want Optimal", resCBC.Status, err)
		}
		if resCBC.FellBackToGo {
			t.Fatalf("CBC unexpected FellBackToGo=true on normal-scaled bounded infinite-bound LP")
		}
		if math.Abs(resCBC.Objective-resGo.Objective) > 1e-6 {
			t.Fatalf("CBC obj=%g, want %g", resCBC.Objective, resGo.Objective)
		}

		clp := gb.NewCBCLPSolver(p)
		defer clp.Close()
		rClp := clp.Solve()
		if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective-resGo.Objective) > 1e-6 {
			t.Fatalf("CBCLPSolver: status=%v obj=%g, want Optimal %g", rClp.Status, rClp.Objective, resGo.Objective)
		}
	})

	t.Run("warm_start_bound_narrowing_and_cut_bounding", func(t *testing.T) {
		t.Parallel()
		p := buildUnboundedNegInfMaxModel().Compile()
		clp := gb.NewCBCLPSolver(p)
		defer clp.Close()

		if r0 := clp.Solve(); r0.Status != gb.StatusUnbounded {
			t.Fatalf("initial Solve status=%v, want Unbounded", r0.Status)
		}

		// Bound c6 below at -10 to make the LP finite and optimal.
		lb := []float64{-2, -1, 0, math.Inf(-1), 0, math.Inf(-1), -10}
		ub := []float64{4, 1, 2, 0, 5, 1, 1}
		clp.SetStructuralBounds(lb, ub)
		r1 := clp.Solve()

		lpRef := gb.NewLPSolver(p, nil)
		defer lpRef.Close()
		lpRef.SetStructuralBounds(lb, ub)
		refRes := lpRef.Solve()
		if refRes.Status != gb.StatusOptimal {
			t.Fatalf("Go LPSolver reference status=%v, want Optimal", refRes.Status)
		}
		if r1.Status != gb.StatusOptimal || math.Abs(r1.Objective-refRes.Objective) > 1e-6 {
			t.Fatalf("CBCLPSolver after SetStructuralBounds: status=%v obj=%g, want Optimal %g",
				r1.Status, r1.Objective, refRes.Objective)
		}

		// Restore unbounded bounds and bound via AddCut (-c6 <= 5 => c6 >= -5).
		origLB := []float64{-2, -1, 0, math.Inf(-1), 0, math.Inf(-1), math.Inf(-1)}
		clp.SetStructuralBounds(origLB, ub)
		if r2 := clp.Solve(); r2.Status != gb.StatusUnbounded {
			t.Fatalf("after restoring -Inf bound: status=%v, want Unbounded", r2.Status)
		}
		clp.AddCut([]int{6}, []float64{-1}, 5)
		r3 := clp.Solve()

		lpRef.SetStructuralBounds(origLB, ub)
		lpRef.AddCut([]int{6}, []float64{-1}, 5)
		refCut := lpRef.Solve()
		if r3.Status != gb.StatusOptimal || math.Abs(r3.Objective-refCut.Objective) > 1e-6 {
			t.Fatalf("CBCLPSolver after AddCut bounding c6: status=%v obj=%g, want Optimal %g",
				r3.Status, r3.Objective, refCut.Objective)
		}
	})
}

func assertObjAmplifiedTinyMatch(t *testing.T, label string, objScale, tinyCoeff float64, useGe bool) {
	t.Helper()
	ctx := t.Context()

	m := gb.NewModel(label, gb.Maximize)
	vars := make([]gb.Var, 5)
	objTerms := make([]gb.Term, 5)
	for i := range vars {
		name := string(rune('a' + i))
		vars[i] = m.AddIntVar(name, 0, 10)
		objTerms[i] = vars[i].T(objScale)
	}
	if useGe {
		m.AddConstraint("c0", gb.NewExpr(vars[0].T(-tinyCoeff), vars[1].T(-1)).Ge(0))
	} else {
		m.AddConstraint("c0", gb.NewExpr(vars[0].T(tinyCoeff), vars[1].T(1)).Le(0))
	}
	m.SetObjective(gb.NewExpr(objTerms...))
	p := m.Compile()

	wantObj := 30.0 * objScale
	resGo, err := gb.SolveCompiled(ctx, p, gb.WithPureGo())
	if err != nil || resGo.Status != gb.StatusOptimal {
		t.Fatalf("%s PureGo: status=%v err=%v, want Optimal", label, resGo.Status, err)
	}
	if math.Abs(resGo.Objective-wantObj) > 1e-6*wantObj {
		t.Fatalf("%s PureGo obj=%g, want %g", label, resGo.Objective, wantObj)
	}

	for _, tc := range []struct {
		name string
		opts []gb.Option
	}{
		{"cbc_default", []gb.Option{gb.WithCBC()}},
		{"cbc_nopresolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false)}},
	} {
		resCBC, err := gb.SolveCompiled(ctx, p, tc.opts...)
		if err != nil || resCBC.Status != gb.StatusOptimal {
			t.Fatalf("%s %s: status=%v err=%v, want Optimal", label, tc.name, resCBC.Status, err)
		}
		if math.Abs(resCBC.Objective-wantObj) > 1e-6*wantObj {
			t.Fatalf("%s %s obj=%g (vals=%v), want %g", label, tc.name, resCBC.Objective, resCBC.Values, wantObj)
		}
		if bV, rV, iV := p.MaxViolation(resCBC.Values, true); bV > 1e-9 || rV > 1e-24 || iV > 1e-9 {
			t.Fatalf("%s %s violation=(%g,%g,%g) vals=%v", label, tc.name, bV, rV, iV, resCBC.Values)
		}
	}
}

func TestCBCObjAmplifiedTinyMatrixCoeff(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("mip_1e35_1e20_repro", func(t *testing.T) {
		t.Parallel()
		assertObjAmplifiedTinyMatch(t, "mip_1e35_1e20", 1e35, 1e-20, false)
	})

	t.Run("mip_1e35_1e20_ge_mirror", func(t *testing.T) {
		t.Parallel()
		assertObjAmplifiedTinyMatch(t, "mip_1e35_1e20_ge", 1e35, 1e-20, true)
	})

	t.Run("mip_1_1e20", func(t *testing.T) {
		t.Parallel()
		assertObjAmplifiedTinyMatch(t, "mip_1_1e20", 1.0, 1e-20, false)
	})

	t.Run("mip_1e35_1e10", func(t *testing.T) {
		t.Parallel()
		assertObjAmplifiedTinyMatch(t, "mip_1e35_1e10", 1e35, 1e-10, false)
	})

	t.Run("lp_single_col_1e35_1e20", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("lp_single_tiny", gb.Maximize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		m.AddConstraint("c0", gb.NewExpr(x0.T(1e-20)).Le(0))
		m.SetObjective(gb.NewExpr(x0.T(1e35), x1.T(1e35)))
		p := m.Compile()

		wantObj := 10.0 * 1e35
		resGo, err := gb.SolveCompiled(ctx, p, gb.WithPureGo())
		if err != nil || resGo.Status != gb.StatusOptimal || math.Abs(resGo.Objective-wantObj) > 1e-6*wantObj {
			t.Fatalf("PureGo: status=%v obj=%g err=%v, want Optimal %g", resGo.Status, resGo.Objective, err, wantObj)
		}
		resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resCBC.Status != gb.StatusOptimal || math.Abs(resCBC.Objective-wantObj) > 1e-6*wantObj {
			t.Fatalf("CBC: status=%v obj=%g vals=%v err=%v, want Optimal %g",
				resCBC.Status, resCBC.Objective, resCBC.Values, err, wantObj)
		}

		clp := gb.NewCBCLPSolver(p)
		defer clp.Close()
		rClp := clp.Solve()
		if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective-wantObj) > 1e-6*wantObj {
			t.Fatalf("CBCLPSolver: status=%v obj=%g primal=%v, want Optimal %g",
				rClp.Status, rClp.Objective, rClp.Primal, wantObj)
		}
	})

	t.Run("zero_obj_tiny_coeff_remains_optimal", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("zero_obj_tiny_mip", gb.Minimize)
		x1 := m.AddIntVar("x1", 0, 2000)
		x2 := m.AddIntVar("x2", 0, 1)
		m.SetObjective(gb.VarExpr(x1))
		m.AddConstraint("c", gb.NewExpr(x1.T(1), x2.T(1e-22)).Ge(1055))
		p := m.Compile()

		res, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || res.Status != gb.StatusOptimal || math.Abs(res.Objective-1055) > 1e-5 {
			t.Fatalf("CBC zero-obj tiny coeff MIP: status=%v obj=%g err=%v, want Optimal 1055",
				res.Status, res.Objective, err)
		}
	})

	t.Run("lp_wide_bound_tiny_coeff_obj_1e35", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			c0   float64
		}{
			{"neg_tiny_c0", -1e-5},
			{"zero_c0", 0},
		} {
			m := gb.NewModel("lp_wide_tiny_"+tc.name, gb.Maximize)
			x0 := m.AddContVar("x0", -1e9, 1e9)
			x1 := m.AddContVar("x1", -1e9, 1e9)
			m.AddConstraint("c0", gb.NewExpr(x0.T(1e-20), x1.T(1)).Le(0))
			m.SetObjective(gb.NewExpr(x0.T(tc.c0), x1.T(1e35)))
			p := m.Compile()

			resGo, err := gb.SolveCompiled(ctx, p, gb.WithPureGo())
			if err != nil || resGo.Status != gb.StatusOptimal || math.Abs(resGo.Objective-1e24) > 1e18 {
				t.Fatalf("%s PureGo: status=%v obj=%g err=%v, want Optimal ~1e24",
					tc.name, resGo.Status, resGo.Objective, err)
			}

			for _, pre := range []bool{true, false} {
				resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC(), gb.WithPresolve(pre))
				if err != nil {
					t.Fatalf("%s CBC(pre=%v) err: %v", tc.name, pre, err)
				}
				if resCBC.Status != gb.StatusNumericalError &&
					(resCBC.Status != gb.StatusOptimal || math.Abs(resCBC.Objective-resGo.Objective) > 1e-6*math.Abs(resGo.Objective)) {
					t.Fatalf("%s CBC(pre=%v): status=%v obj=%g vals=%v, want NumericalError or Optimal %g",
						tc.name, pre, resCBC.Status, resCBC.Objective, resCBC.Values, resGo.Objective)
				}
			}

			clp := gb.NewCBCLPSolver(p)
			rClp := clp.Solve()
			clp.Close()
			if rClp.Status != gb.StatusNumericalError &&
				(rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective-resGo.Objective) > 1e-6*math.Abs(resGo.Objective)) {
				t.Fatalf("%s CBCLPSolver: status=%v obj=%g primal=%v, want NumericalError or Optimal %g",
					tc.name, rClp.Status, rClp.Objective, rClp.Primal, resGo.Objective)
			}
		}
	})
}
