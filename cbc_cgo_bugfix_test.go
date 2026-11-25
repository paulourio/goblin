//go:build cbc && cgo

package goblin_test

import (
	"context"
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
)

func assertSolutionPrimalValid(t *testing.T, tag string, p *gb.CompiledProblem, vals []float64, tol float64) {
	t.Helper()
	if len(vals) != p.NumVars() {
		t.Fatalf("%s: got %d values, want %d", tag, len(vals), p.NumVars())
	}
	bv, rv, iv := p.MaxViolation(vals, p.IsMIP())
	if bv > tol || rv > tol || iv > tol {
		t.Fatalf("%s: solution violates tol=%g (bound=%g row=%g int=%g) vals=%v",
			tag, tol, bv, rv, iv, vals)
	}
}

func assertNearFloat(t *testing.T, tag string, got, want, absTol, relTol float64) {
	t.Helper()
	diff := math.Abs(got - want)
	limit := max(absTol, relTol*max(math.Abs(got), math.Abs(want)))
	if math.IsNaN(diff) || diff > limit {
		t.Fatalf("%s: got=%.17g want=%.17g diff=%g > limit=%g", tag, got, want, diff, limit)
	}
}

func TestCBCProbingNegativeLBInfiniteUBRegressions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	type milpCase struct {
		name          string
		build         func() *gb.Model
		wantObj       float64
		hasWant       bool
		requireNative bool
		maxAbsTol     float64
	}

	buildBug46 := func(name string, sense gb.Sense, aLB, aUB, v8LB, v8UB, c4LB, c4UB float64, negateIntVars bool) *gb.Model {
		m := gb.NewModel(name, sense)
		// When negateIntVars is true, negate a, v8, and c4 into (-Inf, +1] domains
		// so COIN-OR probing receives -Inf lower bounds while modeling the same polytope.
		var a, v8, c4 gb.Var
		sign := 1.0
		if negateIntVars {
			sign = -1.0
			a = m.AddIntVar("a", -aUB, -aLB)
			v8 = m.AddIntVar("v8", -v8UB, -v8LB)
			c4 = m.AddIntVar("c4", -c4UB, -c4LB)
		} else {
			a = m.AddIntVar("a", aLB, aUB)
			v8 = m.AddIntVar("v8", v8LB, v8UB)
			c4 = m.AddIntVar("c4", c4LB, c4UB)
		}
		l9 := m.AddIntVar("l9", 1, 1)
		c3 := m.AddContVar("c3", -1, 2)
		c5 := m.AddIntVar("c5", -2, -2)

		objSign := 1.0
		if sense == gb.Maximize {
			objSign = -1.0
		}
		m.SetObjective(gb.NewExpr(
			a.T(objSign*4*sign),
			v8.T(objSign*2*sign),
			l9.T(objSign*0.5),
			c3.T(objSign*-2.5),
			c4.T(objSign*3*sign),
			c5.T(objSign*-1.5),
		))
		m.AddConstraint("r_0", gb.NewExpr().Ge(-5))
		m.AddConstraint("r_1", gb.NewExpr(
			a.T(1.5*sign),
			v8.T(-0.5*sign),
			c3.T(-1.5),
			c5.T(2.5),
		).Eq(-3.5))
		m.AddConstraint("r_2", gb.NewExpr(
			a.T(-3*sign),
			l9.T(-0.5),
			c4.T(-0.5*sign),
			c5.T(1),
		).Le(-6))
		return m
	}

	buildShiftedNonNegBug46 := func() *gb.Model {
		m := gb.NewModel("bug46_shifted_nonneg", gb.Minimize)
		ua := m.AddIntVar("ua", 0, math.Inf(1))
		uv8 := m.AddIntVar("uv8", 0, math.Inf(1))
		l9 := m.AddIntVar("l9", 1, 1)
		c3 := m.AddContVar("c3", -1, 2)
		uc4 := m.AddIntVar("uc4", 0, math.Inf(1))
		c5 := m.AddIntVar("c5", -2, -2)
		m.SetObjective(gb.NewExpr(
			ua.T(4), uv8.T(2), l9.T(0.5), c3.T(-2.5), uc4.T(3), c5.T(-1.5),
		).AddConstant(-9))
		m.AddConstraint("r_0", gb.NewExpr().Ge(-5))
		m.AddConstraint("r_1", gb.NewExpr(ua.T(1.5), uv8.T(-0.5), c3.T(-1.5), c5.T(2.5)).Eq(-2.5))
		m.AddConstraint("r_2", gb.NewExpr(ua.T(-3), l9.T(-0.5), uc4.T(-0.5), c5.T(1)).Le(-9.5))
		return m
	}

	buildOpenOnlyC4 := func() *gb.Model {
		m := gb.NewModel("only_c4_neg1_inf", gb.Minimize)
		a := m.AddIntVar("a", 0, math.Inf(1))
		v8 := m.AddIntVar("v8", 0, math.Inf(1))
		c3 := m.AddContVar("c3", -1, 2)
		c4 := m.AddIntVar("c4", -1, math.Inf(1))
		m.SetObjective(gb.NewExpr(a.T(4), v8.T(2), c3.T(-2.5), c4.T(3)).AddConstant(3.5))
		m.AddConstraint("r_1", gb.NewExpr(a.T(1.5), v8.T(-0.5), c3.T(-1.5)).Eq(1.5))
		m.AddConstraint("r_2", gb.NewExpr(a.T(-3), c4.T(-0.5)).Le(-3.5))
		return m
	}

	cases := []milpCase{
		{
			name: "exact_bug46_minimize",
			build: func() *gb.Model {
				return buildBug46("bug46_min", gb.Minimize, -1, math.Inf(1), -1, math.Inf(1), -1, math.Inf(1), false)
			},
			wantObj:       19.0 / 6.0,
			hasWant:       true,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
		{
			name: "exact_bug46_maximize",
			build: func() *gb.Model {
				return buildBug46("bug46_max", gb.Maximize, -1, math.Inf(1), -1, math.Inf(1), -1, math.Inf(1), false)
			},
			wantObj:       -19.0 / 6.0,
			hasWant:       true,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
		{
			name: "neginf_lb_minimize",
			build: func() *gb.Model {
				return buildBug46("bug46_neginf_min", gb.Minimize, -1, math.Inf(1), -1, math.Inf(1), -1, math.Inf(1), true)
			},
			wantObj:       19.0 / 6.0,
			hasWant:       true,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
		{
			name: "neginf_lb_maximize",
			build: func() *gb.Model {
				return buildBug46("bug46_neginf_max", gb.Maximize, -1, math.Inf(1), -1, math.Inf(1), -1, math.Inf(1), true)
			},
			wantObj:       -19.0 / 6.0,
			hasWant:       true,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
		{
			name:          "large_finite_ub_1e15",
			build:         func() *gb.Model { return buildBug46("bug46_1e15_ub", gb.Minimize, -1, 1e15, -1, 1e15, -1, 1e15, false) },
			wantObj:       19.0 / 6.0,
			hasWant:       true,
			requireNative: false,
			maxAbsTol:     1e-6,
		},
		{
			name:          "large_finite_neg_lb_1e15",
			build:         func() *gb.Model { return buildBug46("bug46_1e15_lb", gb.Minimize, -1, 1e15, -1, 1e15, -1, 1e15, true) },
			wantObj:       19.0 / 6.0,
			hasWant:       true,
			requireNative: false,
			maxAbsTol:     1e-6,
		},
		{
			name:          "shifted_nonneg_inf_ub",
			build:         buildShiftedNonNegBug46,
			wantObj:       19.0 / 6.0,
			hasWant:       true,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
		{
			name:          "only_c4_neg1_inf_ub",
			build:         buildOpenOnlyC4,
			hasWant:       false,
			requireNative: true,
			maxAbsTol:     1e-6,
		},
	}

	variants := []struct {
		name string
		opts []gb.Option
	}{
		{"cbc_default", []gb.Option{gb.WithCBC()}},
		{"cbc_nopresolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false)}},
		{"cbc_nocuts", []gb.Option{gb.WithCBC(), gb.WithCuts(false)}},
		{"cbc_noheur", []gb.Option{gb.WithCBC(), gb.WithHeuristics(false)}},
		{"cbc_nopresolve_noheur", []gb.Option{gb.WithCBC(), gb.WithPresolve(false), gb.WithHeuristics(false)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := tc.build().CompileChecked()
			if err != nil {
				t.Fatalf("CompileChecked err=%v", err)
			}

			resGoPre, err := gb.SolveCompiled(ctx, p, gb.WithPresolve(true))
			if err != nil || resGoPre.Status != gb.StatusOptimal {
				t.Fatalf("PureGo(presolve=true) status=%v err=%v", resGoPre.Status, err)
			}
			resGoNoPre, err := gb.SolveCompiled(ctx, p, gb.WithPresolve(false))
			if err != nil || resGoNoPre.Status != gb.StatusOptimal {
				t.Fatalf("PureGo(presolve=false) status=%v err=%v", resGoNoPre.Status, err)
			}
			assertSolutionPrimalValid(t, "PureGo", p, resGoPre.Values, 1e-6)
			assertNearFloat(t, "PureGo presolve parity", resGoPre.Objective, resGoNoPre.Objective, tc.maxAbsTol, 1e-9)
			if tc.hasWant {
				assertNearFloat(t, "PureGo vs wantObj", resGoPre.Objective, tc.wantObj, tc.maxAbsTol, 1e-9)
			}

			for _, v := range variants {
				resCBC, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("%s: err=%v", v.name, err)
				}
				if resCBC.Backend != gb.BackendCBC {
					t.Fatalf("%s: backend=%v, want BackendCBC", v.name, resCBC.Backend)
				}
				if tc.requireNative && resCBC.FellBackToGo {
					t.Fatalf("%s: unexpected FellBackToGo=true; want native CBC solve", v.name)
				}
				if resCBC.Status != gb.StatusOptimal {
					t.Fatalf("%s: status=%v obj=%g bound=%g, want StatusOptimal",
						v.name, resCBC.Status, resCBC.Objective, resCBC.BestBound)
				}
				assertSolutionPrimalValid(t, v.name, p, resCBC.Values, 1e-6)
				assertNearFloat(t, v.name+" objective vs PureGo", resCBC.Objective, resGoPre.Objective, tc.maxAbsTol, 1e-9)
				assertNearFloat(t, v.name+" best bound vs PureGo", resCBC.BestBound, resGoPre.Objective, tc.maxAbsTol, 1e-9)
			}
		})
	}
}

func TestCBCLPObjAmplifiedTinyMatrixCoeff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	buildModel := func(name string, sense gb.Sense, x0Obj, x1Obj float64) *gb.Model {
		m := gb.NewModel(name, sense)
		x0 := m.AddContVar("x0", -1e9, 1e9)
		x1 := m.AddContVar("x1", -1e9, 1e9)
		m.SetObjective(gb.NewExpr(x0.T(x0Obj), x1.T(x1Obj)))
		m.AddConstraint("c", gb.NewExpr(x0.T(1e-20), x1.T(1)).Le(0))
		return m
	}

	lpCases := []struct {
		name   string
		sense  gb.Sense
		x0Obj  float64
		x1Obj  float64
		wantUp float64
	}{
		{
			name:   "bug50_exact_max",
			sense:  gb.Maximize,
			x0Obj:  -1e-5,
			x1Obj:  1e35,
			wantUp: 1e24,
		},
		{
			name:   "bug50_zero_obj_tiny_col",
			sense:  gb.Maximize,
			x0Obj:  0,
			x1Obj:  1e35,
			wantUp: 1e24,
		},
		{
			name:   "bug50_minimize_mirror",
			sense:  gb.Minimize,
			x0Obj:  1e-5,
			x1Obj:  -1e35,
			wantUp: -1e24,
		},
	}

	for _, tc := range lpCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := buildModel(tc.name, tc.sense, tc.x0Obj, tc.x1Obj).CompileChecked()
			if err != nil {
				t.Fatalf("CompileChecked err=%v", err)
			}
			resGo, err := gb.SolveCompiled(ctx, p)
			if err != nil || resGo.Status != gb.StatusOptimal {
				t.Fatalf("PureGo status=%v err=%v", resGo.Status, err)
			}
			assertSolutionPrimalValid(t, "PureGo", p, resGo.Values, 1e-6)
			assertNearFloat(t, "PureGo vs expected magnitude", resGo.Objective, tc.wantUp, 1e12, 1e-9)

			for _, presolve := range []bool{true, false} {
				resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC(), gb.WithPresolve(presolve))
				if err != nil {
					t.Fatalf("CBC(presolve=%v) err=%v", presolve, err)
				}
				if resCBC.Status != gb.StatusOptimal || !resCBC.FellBackToGo {
					t.Fatalf("CBC(presolve=%v) status=%v fellBack=%v obj=%g, want Optimal with FellBackToGo",
						presolve, resCBC.Status, resCBC.FellBackToGo, resCBC.Objective)
				}
				assertSolutionPrimalValid(t, "CBC", p, resCBC.Values, 1e-6)
				assertNearFloat(t, "CBC vs PureGo", resCBC.Objective, resGo.Objective, 1e12, 1e-12)
			}

			clp := gb.NewCBCLPSolver(p)
			defer clp.Close()

			rInit := clp.Solve()
			if rInit.Status != gb.StatusOptimal {
				t.Fatalf("CBCLPSolver initial status=%v obj=%g", rInit.Status, rInit.Objective)
			}
			assertNearFloat(t, "CBCLPSolver initial vs PureGo", rInit.Objective, resGo.Objective, 1e12, 1e-12)

			clp.SetStructuralBounds([]float64{0, -1e9}, []float64{1e9, 1e9})
			rNarrow := clp.Solve()
			if rNarrow.Status != gb.StatusOptimal {
				t.Fatalf("CBCLPSolver narrow status=%v", rNarrow.Status)
			}
			assertNearFloat(t, "CBCLPSolver narrow x0>=0 obj=0", rNarrow.Objective, 0, 1e-2, 1e-9)

			clp.SetStructuralBounds([]float64{-1e9, -1e9}, []float64{1e9, 1e9})
			rWide := clp.Solve()
			if rWide.Status != gb.StatusOptimal {
				t.Fatalf("CBCLPSolver re-widened status=%v", rWide.Status)
			}
			assertNearFloat(t, "CBCLPSolver re-widened vs PureGo", rWide.Objective, resGo.Objective, 1e12, 1e-12)
		})
	}

	t.Run("bug50_addcut_tiny_coeff", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("addcut_tiny_coeff", gb.Maximize)
		x0 := m.AddContVar("x0", -1e9, 1e9)
		x1 := m.AddContVar("x1", -1e9, 1e9)
		m.SetObjective(gb.NewExpr(x0.T(-1e-5), x1.T(1e35)))
		m.AddConstraint("upper_cap", gb.NewExpr(x1.T(1)).Le(1e5))
		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked err=%v", err)
		}
		clp := gb.NewCBCLPSolver(p)
		defer clp.Close()
		clp.AddCut([]int{0, 1}, []float64{1e-20, 1}, 0)
		rCut := clp.Solve()
		if rCut.Status != gb.StatusOptimal {
			t.Fatalf("CBCLPSolver AddCut status=%v", rCut.Status)
		}
		assertNearFloat(t, "CBCLPSolver AddCut objective", rCut.Objective, 1e24, 1e12, 1e-9)
	})

	t.Run("bug50_direct_tiny_col_obj_amplified", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("direct_tiny_col", gb.Maximize)
		x0 := m.AddContVar("x0", 0, 1e10)
		m.SetObjective(gb.NewExpr(x0.T(1)))
		m.AddConstraint("cap", gb.NewExpr(x0.T(1e-19)).Le(2e-19))
		p := m.Compile()
		resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resCBC.Status != gb.StatusOptimal {
			t.Fatalf("CBC status=%v err=%v", resCBC.Status, err)
		}
		assertNearFloat(t, "direct tiny col optimal", resCBC.Objective, 2.0, 1e-6, 1e-9)
	})

	t.Run("bug50_mip_variant", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("bug50_mip", gb.Maximize)
		x0 := m.AddIntVar("x0", -1e9, 1e9)
		x1 := m.AddIntVar("x1", -1e9, 1e9)
		m.SetObjective(gb.NewExpr(x0.T(-1e-5), x1.T(1e15)))
		m.AddConstraint("c", gb.NewExpr(x0.T(1e-20), x1.T(1)).Le(1-5e-12))
		p := m.Compile()
		resGo, err := gb.SolveCompiled(ctx, p)
		if err != nil || resGo.Status != gb.StatusOptimal {
			t.Fatalf("PureGo MIP status=%v err=%v", resGo.Status, err)
		}
		resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resCBC.Status != gb.StatusOptimal {
			t.Fatalf("CBC MIP status=%v err=%v", resCBC.Status, err)
		}
		assertSolutionPrimalValid(t, "CBC MIP", p, resCBC.Values, 1e-6)
		assertNearFloat(t, "CBC MIP vs PureGo", resCBC.Objective, resGo.Objective, 1e-6, 1e-9)
	})

	t.Run("uncoupled_tiny_coeff_stays_native", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("uncoupled_tiny_native", gb.Minimize)
		w := m.AddContVar("w", 0, 1)
		z := m.AddBinaryVar("z")
		m.SetObjective(gb.NewExpr(z.T(1)))
		m.AddConstraint("vac", gb.NewExpr(w.T(1e-19), z.T(1)).Le(2))
		m.AddConstraint("req", gb.NewExpr(z.T(1)).Ge(1))
		p := m.Compile()
		resCBC, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resCBC.Status != gb.StatusOptimal || resCBC.FellBackToGo {
			t.Fatalf("status=%v fellBack=%v obj=%g err=%v, want native Optimal",
				resCBC.Status, resCBC.FellBackToGo, resCBC.Objective, err)
		}
		assertNearFloat(t, "native optimal", resCBC.Objective, 1.0, 1e-9, 1e-9)
	})
}
