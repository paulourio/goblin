package goblin_test

import (
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
)

func assertSolveFeasibleParity(t *testing.T, m *gb.Model, wantObj float64, tol float64) {
	t.Helper()
	p := m.Compile()
	if err := p.Err(); err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	pre := gb.Presolve(p, 0)
	if pre.Infeasible {
		t.Fatalf("Presolve() unexpectedly returned Infeasible=true")
	}
	viprPre := gb.PresolveForVIPR(p, 0)
	if viprPre.Infeasible {
		t.Fatalf("PresolveForVIPR() unexpectedly returned Infeasible=true")
	}

	resNoPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(false))
	if err != nil {
		t.Fatalf("SolveCompiled(WithPresolve(false)) error = %v", err)
	}
	resPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(true))
	if err != nil {
		t.Fatalf("SolveCompiled(WithPresolve(true)) error = %v", err)
	}

	if resNoPre.Status != gb.StatusOptimal || resPre.Status != gb.StatusOptimal {
		t.Fatalf("status mismatch: noPre=%v pre=%v, want both Optimal", resNoPre.Status, resPre.Status)
	}
	if math.Abs(resNoPre.Objective-wantObj) > tol || math.Abs(resPre.Objective-wantObj) > tol {
		t.Fatalf("objective mismatch: noPre=%.12g pre=%.12g want=%.12g (tol=%g)",
			resNoPre.Objective, resPre.Objective, wantObj, tol)
	}
	for label, res := range map[string]gb.Result{"noPre": resNoPre, "pre": resPre} {
		bViol, rViol, iViol := p.MaxViolation(res.Values, p.IsMIP())
		if bViol > 1e-7 || rViol > 1e-7 || iViol > 1e-7 {
			t.Fatalf("%s violation too large: bound=%g row=%g int=%g (vals=%v)",
				label, bViol, rViol, iViol, res.Values)
		}
	}
}

func assertSolveBothInfeasible(t *testing.T, m *gb.Model) {
	t.Helper()
	p := m.Compile()
	if err := p.Err(); err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	resNoPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(false))
	if err != nil {
		t.Fatalf("SolveCompiled(WithPresolve(false)) error = %v", err)
	}
	resPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(true))
	if err != nil {
		t.Fatalf("SolveCompiled(WithPresolve(true)) error = %v", err)
	}
	if resNoPre.Status != gb.StatusInfeasible {
		t.Fatalf("WithPresolve(false) status=%v obj=%g vals=%v, want Infeasible",
			resNoPre.Status, resNoPre.Objective, resNoPre.Values)
	}
	if resPre.Status != gb.StatusInfeasible {
		t.Fatalf("WithPresolve(true) status=%v obj=%g vals=%v, want Infeasible",
			resPre.Status, resPre.Objective, resPre.Values)
	}
}

func TestPresolveBug57ExactTinyRowBinaryInfeasible(t *testing.T) {
	t.Parallel()

	scale := math.Ldexp(1, -25) // Exact 2^-25 = 1/33554432.

	t.Run("fuzz_bug57_exact_reproducer", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("bug57_repro", gb.Minimize)
		v0 := m.AddBinaryVar("v0")
		v1 := m.AddBinaryVar("v1")
		m.SetObjective(gb.NewExpr(v1.T(-0.0004730224609375)))
		m.AddConstraint("r0", gb.NewExpr(v0.T(scale)).Le(-4*scale))

		p := m.Compile()
		if pre := gb.Presolve(p, 0); !pre.Infeasible {
			t.Fatalf("Presolve() Infeasible=false, want true")
		}
		if vpre := gb.PresolveForVIPR(p, 0); !vpre.Infeasible {
			t.Fatalf("PresolveForVIPR() Infeasible=false, want true")
		}
		assertSolveBothInfeasible(t, m)
	})

	t.Run("ge_and_eq_tiny_variants", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			build  func(m *gb.Model, v0, v1 gb.Var)
			strict bool
		}{
			{
				name:   "neg_ge_infeasible",
				strict: true,
				build: func(m *gb.Model, v0, _ gb.Var) {
					m.AddConstraint("r0", gb.NewExpr(v0.T(-scale)).Ge(4*scale))
				},
			},
			{
				name:   "pos_ge_above_ub_infeasible",
				strict: true,
				build: func(m *gb.Model, v0, _ gb.Var) {
					m.AddConstraint("r0", gb.NewExpr(v0.T(scale)).Ge(2*scale))
				},
			},
			{
				name:   "eq_negative_rhs_infeasible",
				strict: true,
				build: func(m *gb.Model, v0, _ gb.Var) {
					m.AddConstraint("r0", gb.NewExpr(v0.T(scale)).Eq(-4*scale))
				},
			},
			{
				name:   "eq_fractional_binary_sum_infeasible",
				strict: true,
				build: func(m *gb.Model, v0, v1 gb.Var) {
					m.AddConstraint("r0", gb.NewExpr(v0.T(scale), v1.T(scale)).Eq(0.5*scale))
				},
			},
			{
				name:   "sub_2pow38_scale_infeasible",
				strict: true,
				build: func(m *gb.Model, v0, _ gb.Var) {
					s38 := math.Ldexp(1, -38)
					m.AddConstraint("r0", gb.NewExpr(v0.T(s38)).Le(-2*s38))
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				m := gb.NewModel(tc.name, gb.Minimize)
				v0 := m.AddBinaryVar("v0")
				v1 := m.AddBinaryVar("v1")
				m.SetObjective(gb.NewExpr(v0.T(1), v1.T(-1)))
				tc.build(m, v0, v1)
				if tc.strict {
					p := m.Compile()
					if !gb.Presolve(p, 0).Infeasible {
						t.Fatalf("Presolve() returned Infeasible=false, want true")
					}
					if !gb.PresolveForVIPR(p, 0).Infeasible {
						t.Fatalf("PresolveForVIPR() returned Infeasible=false, want true")
					}
				}
				assertSolveBothInfeasible(t, m)
			})
		}
	})
}

func TestPresolveTinyRowEmptyAndKeptTermsZero(t *testing.T) {
	t.Parallel()

	scale := math.Ldexp(1, -25)

	t.Run("zero_vars_tiny_infeasible_rows", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name  string
			sense gb.ConstraintSense
			rhs   float64
		}{
			{"le_neg", gb.LessEqual, -1e-9},
			{"ge_pos", gb.GreaterEqual, 1e-9},
			{"eq_neg", gb.Equal, -1e-9},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				m := gb.NewModel("empty_"+tc.name, gb.Minimize)
				m.AddConstraint("empty", gb.Constraint{Sense: tc.sense, RHS: tc.rhs})
				p := m.Compile()
				if !gb.Presolve(p, 0).Infeasible {
					t.Fatalf("Presolve() Infeasible=false for empty %v row rhs=%g", tc.sense, tc.rhs)
				}
				assertSolveBothInfeasible(t, m)
			})
		}
	})

	t.Run("prefixed_zero_and_cross_row_fixed_tiny_le", func(t *testing.T) {
		t.Parallel()
		// All terms pre-fixed at [0, 0] yield keptTerms == 0 in buildCompiledDirect.
		mPre := gb.NewModel("prefixed_zero_tiny", gb.Minimize)
		x0 := mPre.AddContVar("x0", 0, 0)
		mPre.SetObjective(gb.VarExpr(x0))
		mPre.AddConstraint("r0", gb.NewExpr(x0.T(scale)).Le(-4*scale))
		if !gb.Presolve(mPre.Compile(), 0).Infeasible {
			t.Fatalf("Presolve() Infeasible=false on pre-fixed tiny row")
		}
		assertSolveBothInfeasible(t, mPre)

		// Another constraint fixes v0 and v1 to 0, leaving r0 with keptTerms == 0.
		mCross := gb.NewModel("cross_row_fixed_tiny", gb.Minimize)
		v0 := mCross.AddBinaryVar("v0")
		v1 := mCross.AddBinaryVar("v1")
		mCross.SetObjective(gb.NewExpr(v0.T(1), v1.T(1)))
		mCross.AddConstraint("fix0", gb.NewExpr(v0.T(1)).Le(0))
		mCross.AddConstraint("fix1", gb.NewExpr(v1.T(1)).Le(0))
		mCross.AddConstraint("r0", gb.NewExpr(v0.T(scale), v1.T(scale)).Le(-4*scale))
		if !gb.Presolve(mCross.Compile(), 0).Infeasible {
			t.Fatalf("Presolve() Infeasible=false on cross-row fixed tiny row")
		}
		assertSolveBothInfeasible(t, mCross)
	})

	t.Run("mixed_fixed_unit_and_active_tiny_term", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("mixed_zero_and_tiny", gb.Minimize)
		vZero := m.AddContVar("vZero", 0, 0)
		v0 := m.AddBinaryVar("v0")
		v1 := m.AddBinaryVar("v1")
		m.SetObjective(gb.NewExpr(v1.T(-1)))
		m.AddConstraint("r0", gb.NewExpr(vZero.T(1.0), v0.T(scale)).Le(-4*scale))
		if !gb.Presolve(m.Compile(), 0).Infeasible {
			t.Fatalf("Presolve() Infeasible=false on mixed [0,0]+tiny row")
		}
		assertSolveBothInfeasible(t, m)
	})
}

func TestPresolveTinyRowContinuousAndIntegerTightening(t *testing.T) {
	t.Parallel()

	scale25 := math.Ldexp(1, -25)
	scale38 := math.Ldexp(1, -38)

	t.Run("integer_bTol_tight_floor_on_tiny_rows", func(t *testing.T) {
		t.Parallel()
		for _, a := range []float64{scale25, scale38} {
			m := gb.NewModel("int_tight", gb.Maximize)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddIntVar("y", 0, 10)
			m.SetObjective(gb.NewExpr(x.T(1), y.T(1)))
			m.AddConstraint("cap", gb.NewExpr(x.T(a)).Le(1.75*a))
			m.AddConstraint("link", gb.NewExpr(y.T(a), x.T(-a)).Le(0))
			assertSolveFeasibleParity(t, m, 2.0, 1e-7)
		}
	})

	t.Run("continuous_minInfCount_one_tightens_unbounded_var", func(t *testing.T) {
		t.Parallel()
		// z >= 0 has infinite upper bound, so -scale25*z gives minInfCount == 1 in r0.
		// r0 tightens z >= 4 while r1 restricts z <= 2, which proves the model infeasible across passes.
		m := gb.NewModel("cont_min_inf_1_tiny", gb.Minimize)
		v0 := m.AddBinaryVar("v0")
		z := m.AddContVar("z", 0, math.Inf(1))
		m.SetObjective(gb.NewExpr(z.T(1)))
		m.AddConstraint("r0", gb.NewExpr(v0.T(scale25), z.T(-scale25)).Le(-4*scale25))
		m.AddConstraint("r1", gb.NewExpr(z.T(scale25)).Le(2*scale25))
		if !gb.Presolve(m.Compile(), 0).Infeasible {
			t.Fatalf("Presolve() missed continuous minInfCount==1 tiny-row tightening conflict")
		}
		assertSolveBothInfeasible(t, m)
	})

	t.Run("continuous_tiny_bounds_tightening_parity", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("cont_tiny_tighten", gb.Maximize)
		x := m.AddContVar("x", 0, 10)
		b := m.AddBinaryVar("b")
		m.SetObjective(gb.NewExpr(x.T(1), b.T(0.1)))
		m.AddConstraint("upper", gb.NewExpr(x.T(scale38), b.T(scale38)).Le(2.5*scale38))
		assertSolveFeasibleParity(t, m, 2.5, 1e-6)
	})
}

func TestPresolveTinyRowBinStrengtheningAndLPPresolve(t *testing.T) {
	t.Parallel()

	scale25 := math.Ldexp(1, -25)
	scale34 := math.Ldexp(1, -34)

	t.Run("binary_coeff_strengthening_tiny_row", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("bin_strength_tiny", gb.Maximize)
		v0 := m.AddBinaryVar("v0")
		v1 := m.AddBinaryVar("v1")
		m.SetObjective(gb.NewExpr(v0.T(1), v1.T(2)))
		m.AddConstraint("knap", gb.NewExpr(v0.T(scale25), v1.T(4*scale25)).Le(4*scale25))
		assertSolveFeasibleParity(t, m, 2.0, 1e-7)
	})

	t.Run("lp_dual_fix_empty_tiny_row_infeasible", func(t *testing.T) {
		t.Parallel()
		// Continuous dual-dominated x0, x1 get dual-fixed to 0 in presolveLP.
		// Pass 2 must detect 0 <= -4*2^-25 as infeasible at the tiny coefficient scale.
		m := gb.NewModel("lp_dual_fix_tiny_infeas", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 1)
		x1 := m.AddContVar("x1", 0, 1)
		m.SetObjective(gb.NewExpr(x0.T(1), x1.T(1)))
		m.AddConstraint("r0", gb.NewExpr(x0.T(scale25), x1.T(scale25)).Le(-4*scale25))
		assertSolveBothInfeasible(t, m)
	})

	t.Run("lp_sub_singleton_2pow34_infeasible", func(t *testing.T) {
		t.Parallel()
		// |2^-34| < lpSingletonMinCoef (1e-9), so LP presolve skips singleton handling
		// and relies on the row activity check to catch the infeasible tiny constraint.
		m := gb.NewModel("lp_sub_singleton_tiny", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 1)
		m.SetObjective(gb.NewExpr(x0.T(-1)))
		m.AddConstraint("r0", gb.NewExpr(x0.T(scale34)).Le(-4*scale34))
		assertSolveBothInfeasible(t, m)
	})

	t.Run("lp_singleton_tiny_negative_sub_tol_parity", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("lp_singleton_subtol_parity", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 1)
		m.SetObjective(gb.NewExpr(x0.T(1)))
		m.AddConstraint("r0", gb.NewExpr(x0.T(scale25)).Le(-1e-8*scale25))
		p := m.Compile()
		resNoPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(false))
		if err != nil || resNoPre.Status != gb.StatusOptimal {
			t.Fatalf("noPre status=%v err=%v, want Optimal", resNoPre.Status, err)
		}
		resPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(true))
		if err != nil || resPre.Status != gb.StatusOptimal {
			t.Fatalf("pre status=%v err=%v, want Optimal", resPre.Status, err)
		}
		if math.Abs(resPre.Objective) > 1e-7 || math.Abs(resNoPre.Objective) > 1e-7 {
			t.Fatalf("unexpected obj: noPre=%g pre=%g", resNoPre.Objective, resPre.Objective)
		}
		if !gb.PresolveForVIPR(p, 0).Infeasible {
			t.Fatalf("PresolveForVIPR() expected exact-Q infeasibility on x0 <= -1e-8, x0 >= 0")
		}
	})

	t.Run("lp_nonzero_fixed_sub_ulp_empty_row_parity", func(t *testing.T) {
		t.Parallel()
		// Substituting fixed x0=2.5 leaves +1/2^71 (+4.235e-22) residual within scaled emptyTol.
		// Presolve and buildCompiledDirectWithRowMap must both treat it as feasible.
		m := gb.NewModel("lp_fixed_sub_ulp_parity", gb.Minimize)
		x0 := m.AddContVar("x0", 2.5, 2.5)
		m.SetObjective(gb.NewExpr(x0.T(-0.04)))
		m.AddConstraint("r0", gb.NewExpr(x0.T(-5e-6)).Ge(-1.25e-5))
		for range 4 {
			m.AddConstraint("", gb.Constraint{Sense: gb.LessEqual, RHS: 0})
		}
		p := m.Compile()
		resNoPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(false))
		if err != nil || resNoPre.Status != gb.StatusOptimal {
			t.Fatalf("noPre status=%v err=%v, want Optimal", resNoPre.Status, err)
		}
		resPre, err := gb.SolveCompiled(t.Context(), p, gb.WithPresolve(true))
		if err != nil || resPre.Status != gb.StatusOptimal {
			t.Fatalf("pre status=%v obj=%g err=%v, want Optimal", resPre.Status, resPre.Objective, err)
		}
		if math.Abs(resPre.Objective-resNoPre.Objective) > 1e-9 {
			t.Fatalf("obj mismatch: noPre=%g pre=%g", resNoPre.Objective, resPre.Objective)
		}
	})

	t.Run("lp_singleton_and_nonzero_fixed_tiny_infeasible", func(t *testing.T) {
		t.Parallel()
		// Both paths reject a tiny infeasible singleton (-4 * 2^-25).
		mClear := gb.NewModel("lp_sng_tiny_clear_infeas", gb.Minimize)
		cx0 := mClear.AddContVar("cx0", 0, 1)
		mClear.SetObjective(gb.NewExpr(cx0.T(1)))
		mClear.AddConstraint("r0", gb.NewExpr(cx0.T(scale25)).Le(-4*scale25))
		assertSolveBothInfeasible(t, mClear)

		// LP presolve rejects -2e-7 * 2^-25 because it exceeds colFeasTol=1e-7.
		mEdge := gb.NewModel("lp_sng_tiny_edge_infeas", gb.Minimize)
		ex0 := mEdge.AddContVar("ex0", 0, 1)
		mEdge.SetObjective(gb.NewExpr(ex0.T(1)))
		mEdge.AddConstraint("r0", gb.NewExpr(ex0.T(scale25)).Le(-2e-7*scale25))
		pEdge := mEdge.Compile()
		resEdge, err := gb.SolveCompiled(t.Context(), pEdge, gb.WithPresolve(true))
		if err != nil || resEdge.Status != gb.StatusInfeasible {
			t.Fatalf("edge singleton got status=%v obj=%g err=%v, want Infeasible", resEdge.Status, resEdge.Objective, err)
		}
		if !gb.PresolveForVIPR(pEdge, 0).Infeasible {
			t.Fatalf("PresolveForVIPR() expected Infeasible on edge singleton")
		}

		// Substituting fixed x0=1 shifts 2^-25*x0 + 2^-25*x1 <= -3*2^-25 to -4*2^-25.
		mSub := gb.NewModel("lp_nz_subst_tiny_infeas", gb.Minimize)
		x0 := mSub.AddContVar("x0", 1, 1)
		x1 := mSub.AddContVar("x1", 0, 1)
		mSub.SetObjective(gb.NewExpr(x0.T(1), x1.T(1)))
		mSub.AddConstraint("r0", gb.NewExpr(x0.T(scale25), x1.T(scale25)).Le(-3*scale25))
		assertSolveBothInfeasible(t, mSub)

		// Tiny LP equal rows stay blocked from dual fixing.
		mEq := gb.NewModel("lp_eq_tiny_infeas", gb.Minimize)
		e0 := mEq.AddContVar("e0", 0, 1)
		e1 := mEq.AddContVar("e1", 0, 1)
		mEq.SetObjective(gb.NewExpr(e0.T(1), e1.T(1)))
		mEq.AddConstraint("r0", gb.NewExpr(e0.T(scale25), e1.T(scale25)).Eq(-4*scale25))
		assertSolveBothInfeasible(t, mEq)
	})
}

func TestPresolveTinyRowPositiveFeasibleControls(t *testing.T) {
	t.Parallel()

	scale25 := math.Ldexp(1, -25)
	scale38 := math.Ldexp(1, -38)

	t.Run("zero_vars_feasible_empty_row", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("empty_ok", gb.Minimize)
		m.AddConstraint("ok", gb.Constraint{Sense: gb.Equal, RHS: 0})
		assertSolveFeasibleParity(t, m, 0.0, 1e-9)
	})

	t.Run("binary_tiny_row_feasible_optimum", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("bin_tiny_ok", gb.Minimize)
		v0 := m.AddBinaryVar("v0")
		v1 := m.AddBinaryVar("v1")
		m.SetObjective(gb.NewExpr(v0.T(3), v1.T(-0.5)))
		m.AddConstraint("r0", gb.NewExpr(v0.T(scale25), v1.T(scale25)).Ge(scale25))
		assertSolveFeasibleParity(t, m, -0.5, 1e-7)
	})

	t.Run("lp_tiny_row_feasible_equality_and_ineq", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("lp_tiny_ok", gb.Maximize)
		x0 := m.AddContVar("x0", 0, 2)
		x1 := m.AddContVar("x1", 0, 2)
		m.SetObjective(gb.NewExpr(x0.T(2), x1.T(5)))
		m.AddConstraint("eq", gb.NewExpr(x0.T(scale38), x1.T(2*scale38)).Eq(3*scale38))
		m.AddConstraint("cap", gb.NewExpr(x1.T(scale25)).Le(1.5*scale25))
		assertSolveFeasibleParity(t, m, 7.5, 1e-6)
	})
}
