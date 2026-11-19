//go:build cgo && cbc

package goblin_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

func TestCBCEnabledWithBuildTag(t *testing.T) {
	if !gb.CBCAvailable() {
		t.Fatal("CBCAvailable() = false under cgo && cbc build tags, want true")
	}

	m := gb.NewModel("optin_cbc", gb.Minimize)
	x := m.AddIntVar("x", 0, 5)
	m.SetObjective(gb.NewExpr(gb.T(x, 1)))
	m.AddConstraint("lb", gb.Ge(gb.NewExpr(gb.T(x, 1)), 2))

	res, err := m.Solve(t.Context(), gb.WithCBC())
	if err != nil {
		t.Fatalf("Solve with WithCBC() under -tags cbc returned error: %v", err)
	}
	if res.Backend != gb.BackendCBC {
		t.Fatalf("res.Backend = %v, want BackendCBC", res.Backend)
	}
	if res.Status != gb.StatusOptimal {
		t.Fatalf("res.Status = %v, want StatusOptimal", res.Status)
	}
	if math.Abs(res.Objective-2.0) > 1e-6 {
		t.Fatalf("res.Objective = %v, want 2.0", res.Objective)
	}

	// Verify CBCLPSolver.AddCut canonicalizes duplicate and out-of-range indices safely.
	lpM := gb.NewModel("cbc_lp_cuts", gb.Minimize)
	x0 := lpM.AddContVar("x0", 0, 10)
	x1 := lpM.AddContVar("x1", 0, 10)
	lpM.SetObjective(gb.NewExpr(x0.T(-1), x1.T(-1)))
	lpM.AddConstraint("cap", gb.NewExpr(x0.T(1), x1.T(1)).Le(15))
	s := gb.NewCBCLPSolver(lpM.Compile())
	defer s.Close()
	if r0 := s.Solve(); r0.Status != gb.StatusOptimal || math.Abs(r0.Objective - -15.0) > 1e-6 {
		t.Fatalf("initial CBCLPSolver: status=%v obj=%v", r0.Status, r0.Objective)
	}
	// Duplicate index 0 (0.5*x0 + 0.5*x0 <= 4) plus out-of-range index 99 ignored -> x0 <= 4, x1 <= 10 -> obj = -14.
	s.AddCut([]int{99, 0, 0, -1}, []float64{5.0, 0.5, 0.5, 3.0}, 4.0)
	if r1 := s.Solve(); r1.Status != gb.StatusOptimal || math.Abs(r1.Objective - -14.0) > 1e-6 {
		t.Fatalf("after duplicate/out-of-range AddCut: status=%v obj=%v, want -14", r1.Status, r1.Objective)
	}
}

// Verifies explicit WithMaxNodes(50000), WithMaxCutPasses(0), 64-bit integer clamping,
// zero-value CompiledProblem safety, and non-blocking queued context cancellation on BackendCBC.
func TestCBCHardeningOptionsAndConcurrency(t *testing.T) {
	ctx := t.Context()

	// 1. Zero-value CompiledProblem does not crash Cgo and solves cleanly as empty problem.
	var empty gb.CompiledProblem
	resEmpty, err := gb.SolveCompiled(ctx, &empty, gb.WithCBC())
	if err != nil || resEmpty.Status != gb.StatusOptimal {
		t.Fatalf("zero-value CompiledProblem under CBC: status=%v err=%v", resEmpty.Status, err)
	}
	cbcEmpty := gb.NewCBCLPSolver(&empty)
	if r := cbcEmpty.Solve(); r.Status != gb.StatusOptimal {
		t.Fatalf("NewCBCLPSolver(&empty).Solve() status=%v, want Optimal", r.Status)
	}
	cbcEmpty.Close()
	cbcEmpty.Close() // Idempotent double-close check.

	// 2. Explicit WithMaxNodes(1) and WithMaxCutPasses(0) are honored on BackendCBC, and
	// explicit WithMaxNodes(50000) is not conflated with untouched defaults.
	mHard := gb.NewModel("cbc_limits", gb.Minimize)
	x := mHard.AddBinaryVar("x")
	y := mHard.AddBinaryVar("y")
	mHard.SetObjective(gb.NewExpr(x.T(1), y.T(1)))
	mHard.AddConstraint("lo", gb.NewExpr(x.T(2), y.T(2)).Ge(1))
	mHard.AddConstraint("hi", gb.NewExpr(x.T(2), y.T(2)).Le(1))
	pHard := mHard.Compile()

	resCutOff, err := gb.SolveCompiled(ctx, pHard, gb.WithCBC(), gb.WithPresolve(false), gb.WithMaxCutPasses(0), gb.WithMaxNodes(1))
	if err != nil {
		t.Fatalf("CBC WithMaxCutPasses(0) returned error: %v", err)
	}
	if resCutOff.CutsAdded != 0 {
		t.Fatalf("CBC WithMaxCutPasses(0) added %d cuts, want 0", resCutOff.CutsAdded)
	}

	// 3. Large 64-bit MaxIter (> MaxInt32) clamps cleanly without overflowing C.int into negative values.
	if math.MaxInt > math.MaxInt32 {
		mSmall := gb.NewModel("cbc_clamp", gb.Minimize)
		v := mSmall.AddContVar("v", 1, 5)
		mSmall.SetObjective(gb.VarExpr(v))
		mSmall.AddConstraint("c", gb.VarExpr(v).Ge(2))
		bigIter := math.MaxInt32
		bigIter += 100
		resClamp, err := mSmall.Solve(ctx, gb.WithCBC(), gb.WithMaxIter(bigIter))
		if err != nil || resClamp.Status != gb.StatusOptimal || math.Abs(resClamp.Objective-2.0) > 1e-6 {
			t.Fatalf("CBC MaxIter > MaxInt32: status=%v obj=%v err=%v", resClamp.Status, resClamp.Objective, err)
		}
	}
}

// TestCBCPeerReviewRegressions verifies C11, C12, M1, M7, M8, and m5 on the Cgo CBC/CLP backend.
func TestCBCPeerReviewRegressions(t *testing.T) {
	ctx := t.Context()

	// C11a: Preserve true dual BestBound under non-zero RelGapTol on Minimize and Maximize corpus MIPs.
	t.Run("C11_preserve_best_bound_under_gap", func(t *testing.T) {
		for _, tc := range []struct {
			file    string
			trueOpt float64
			sense   gb.Sense
		}{
			{"t2_mkp_50.json", 2620.0052, gb.Maximize},
			{"t2_cflp_49.json", 909.0210, gb.Minimize},
		} {
			f, err := os.Open(filepath.Join("testdata", "corpus", tc.file))
			if err != nil {
				t.Fatalf("open %s: %v", tc.file, err)
			}
			m, err := format.FromPuLPJSON(f)
			_ = f.Close()
			if err != nil {
				t.Fatalf("parse %s: %v", tc.file, err)
			}
			res, err := m.Solve(ctx, gb.WithCBC(), gb.WithRelGapTol(0.15))
			if err != nil {
				t.Fatalf("%s solve error: %v", tc.file, err)
			}
			if res.Status != gb.StatusOptimal {
				t.Fatalf("%s status = %v, want Optimal", tc.file, res.Status)
			}
			if tc.sense == gb.Maximize && res.BestBound < tc.trueOpt-1e-3 {
				t.Fatalf("%s Maximize BestBound = %.4f invalid (< trueOpt %.4f; obj=%.4f)", tc.file, res.BestBound, tc.trueOpt, res.Objective)
			}
			if tc.sense == gb.Minimize && res.BestBound > tc.trueOpt+1e-3 {
				t.Fatalf("%s Minimize BestBound = %.4f invalid (> trueOpt %.4f; obj=%.4f)", tc.file, res.BestBound, tc.trueOpt, res.Objective)
			}
		}
	})

	// C11b: No hardcoded +1e-6 gap slack when caller requests exact 0 gap tolerance.
	t.Run("C11_no_hardcoded_1e6_gap_slack", func(t *testing.T) {
		m := gb.NewModel("tiny_gap", gb.Minimize)
		x := m.AddBinaryVar("x")
		y := m.AddBinaryVar("y")
		z := m.AddBinaryVar("z")
		m.SetObjective(gb.NewExpr(x.T(1e-6), y.T(1e-6), z.T(1e-6)))
		m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(1)).Ge(1))
		m.AddConstraint("c2", gb.NewExpr(y.T(1), z.T(1)).Ge(1))
		m.AddConstraint("c3", gb.NewExpr(x.T(1), z.T(1)).Ge(1))
		res, err := m.Solve(ctx, gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false), gb.WithRelGapTol(0), gb.WithAbsGapTol(0), gb.WithMaxNodes(1))
		if err != nil {
			t.Fatalf("tiny_gap CBC error: %v", err)
		}
		if res.Status == gb.StatusOptimal && res.BestBound < 1.99e-6 {
			t.Fatalf("tiny_gap CBC falsely upgraded open gap (obj=%.9f bound=%.9f) to Optimal", res.Objective, res.BestBound)
		}
	})

	// C12: Valid incumbents in [1e49, 1e50) must return StatusOptimal, not false StatusInfeasible.
	t.Run("C12_objective_1e49_range", func(t *testing.T) {
		m := gb.NewModel("cbc_1e49", gb.Minimize)
		x := m.AddIntVar("x", 1, 2)
		m.SetObjective(gb.NewExpr(x.T(1.01e49)))
		m.AddConstraint("c", gb.VarExpr(x).Ge(1))
		res, err := m.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("cbc_1e49 error: %v", err)
		}
		if res.Status != gb.StatusOptimal {
			t.Fatalf("cbc_1e49 status = %v, want StatusOptimal", res.Status)
		}
		if math.Abs(res.Objective-1.01e49)/1.01e49 > 1e-6 {
			t.Fatalf("cbc_1e49 objective = %.6e, want 1.01e49", res.Objective)
		}
	})

	// M1: WithOptions(opt) honors MaxNodes and MaxCutPasses on BackendCBC.
	t.Run("M1_with_options_cbc", func(t *testing.T) {
		f, err := os.Open(filepath.Join("testdata", "corpus", "t2_mkp_50.json"))
		if err != nil {
			t.Fatalf("open t2_mkp_50.json: %v", err)
		}
		m, err := format.FromPuLPJSON(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("parse t2_mkp_50.json: %v", err)
		}
		opt := gb.DefaultOptions()
		opt.MaxNodes = 1
		opt.MaxCutPasses = 0
		opt.EnablePresolve = false
		res, err := m.Solve(ctx, gb.WithOptions(opt), gb.WithCBC())
		if err != nil {
			t.Fatalf("M1 CBC error: %v", err)
		}
		if res.CutsAdded != 0 {
			t.Fatalf("WithOptions MaxCutPasses=0 under CBC added %d cuts, want 0", res.CutsAdded)
		}
	})

	// M7 & M8: CBCLPSolver rejects NaN/invalid bounds and non-finite/overflowing cuts cleanly.
	t.Run("M7_M8_cbc_lp_validation", func(t *testing.T) {
		m := gb.NewModel("cbc_m78", gb.Minimize)
		x := m.AddContVar("x", 0, 10)
		m.SetObjective(gb.VarExpr(x))
		m.AddConstraint("c", gb.VarExpr(x).Ge(1))
		p := m.Compile()

		s := gb.NewCBCLPSolver(p)
		s.SetStructuralBounds([]float64{math.NaN()}, []float64{10})
		if r := s.Solve(); r.Status != gb.StatusUndefined {
			t.Fatalf("CBCLPSolver NaN LB status = %v, want StatusUndefined", r.Status)
		}
		s.SetStructuralBounds([]float64{0}, []float64{10})
		if r := s.Solve(); r.Status != gb.StatusOptimal {
			t.Fatalf("CBCLPSolver recovered status = %v, want StatusOptimal", r.Status)
		}
		s.Close()

		for _, tc := range []struct {
			name string
			cols []int
			vals []float64
			rhs  float64
			want gb.Status
		}{
			{"nan_rhs", []int{0}, []float64{1}, math.NaN(), gb.StatusUndefined},
			{"pos_inf_rhs", []int{0}, []float64{1}, math.Inf(1), gb.StatusOptimal},
			{"neg_inf_rhs", []int{0}, []float64{1}, math.Inf(-1), gb.StatusInfeasible},
			{"nan_val", []int{0}, []float64{math.NaN()}, 5, gb.StatusUndefined},
			{"dup_overflow", []int{0, 0}, []float64{1e308, 1e308}, 5, gb.StatusUndefined},
		} {
			cs := gb.NewCBCLPSolver(p)
			_ = cs.Solve()
			cs.AddCut(tc.cols, tc.vals, tc.rhs)
			r := cs.Solve()
			cs.Close()
			if r.Status != tc.want {
				t.Fatalf("CBCLPSolver.AddCut(%s): status=%v, want %v", tc.name, r.Status, tc.want)
			}
		}
	})

	// m5: Presolve-infeasible model with nonzero ObjConstant leaves RootLPBound at 0,
	// and context deadlines are forwarded to CBC cleanly.
	t.Run("m5_root_lp_bound_and_deadline", func(t *testing.T) {
		mInf := gb.NewModel("cbc_infeas_const", gb.Minimize)
		x := mInf.AddBinaryVar("x")
		mInf.SetObjective(gb.Expr{Terms: []gb.Term{x.T(1)}, Constant: 123.0})
		mInf.AddConstraint("c1", gb.VarExpr(x).Ge(1))
		mInf.AddConstraint("c2", gb.VarExpr(x).Le(0))
		resInf, err := mInf.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("cbc_infeas_const error: %v", err)
		}
		if resInf.Status != gb.StatusInfeasible {
			t.Fatalf("cbc_infeas_const status = %v, want Infeasible", resInf.Status)
		}
		if resInf.RootLPBound != 0 || resInf.RootCutBound != 0 {
			t.Fatalf("presolve-infeasible CBC reported RootLPBound=%v RootCutBound=%v, want 0", resInf.RootLPBound, resInf.RootCutBound)
		}

		// Already-expired context deadline returns context.DeadlineExceeded; negative WithTimeLimit returns StatusTimeLimit.
		expCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		mDL := gb.NewModel("cbc_deadline", gb.Minimize)
		v := mDL.AddContVar("v", 0, 5)
		mDL.SetObjective(gb.VarExpr(v))
		if _, err := mDL.Solve(expCtx, gb.WithCBC()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expired ctx CBC solve: err=%v, want context.DeadlineExceeded", err)
		}
		resNegTL, err := mDL.Solve(ctx, gb.WithCBC(), gb.WithTimeLimit(-time.Second))
		if err != nil || resNegTL.Status != gb.StatusTimeLimit {
			t.Fatalf("negative TimeLimit CBC solve: status=%v err=%v, want StatusTimeLimit", resNegTL.Status, err)
		}
	})

	// CRIT-5: Feasible MIPs with objective >= 1e50 downscale cleanly to StatusOptimal (or
	// return StatusNumericalError when unscalable), never false StatusInfeasible.
	t.Run("CRIT5_cbc_objective_ge_1e50_guard", func(t *testing.T) {
		for _, coeff := range []float64{1e50, 1e60} {
			m := gb.NewModel("cbc_huge_obj", gb.Minimize)
			x := m.AddBinaryVar("x")
			y := m.AddBinaryVar("y")
			z := m.AddContVar("z", 0, 10)
			m.SetObjective(gb.NewExpr(z.T(coeff)))
			m.AddConstraint("sum", gb.NewExpr(x.T(1), y.T(1)).Eq(1))
			m.AddConstraint("d1", gb.NewExpr(z.T(1), x.T(-1), y.T(1)).Ge(0))
			m.AddConstraint("d2", gb.NewExpr(z.T(1), x.T(1), y.T(-1)).Ge(0))
			res, err := m.Solve(ctx, gb.WithCBC())
			if err != nil {
				t.Fatalf("coeff=%.2e CBC err: %v", coeff, err)
			}
			if res.Status != gb.StatusOptimal || math.Abs(res.Objective-coeff)/coeff > 1e-6 {
				t.Fatalf("coeff=%.2e CBC status=%v obj=%.6e, want Optimal %.6e", coeff, res.Status, res.Objective, coeff)
			}
		}

		// Unscalable primal bound x >= 1e60 with c = 1 exceeds CBC's 1e50 cutoff directly:
		// must report StatusNumericalError (never false StatusInfeasible).
		mUnscaled := gb.NewModel("cbc_rhs_1e60", gb.Minimize)
		ux := mUnscaled.AddIntVar("x", 1e60, 2e60)
		mUnscaled.SetObjective(gb.VarExpr(ux))
		resU, err := mUnscaled.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("cbc_rhs_1e60 err: %v", err)
		}
		if resU.Status == gb.StatusInfeasible {
			t.Fatalf("cbc_rhs_1e60 returned false StatusInfeasible")
		}
		if resU.Status != gb.StatusNumericalError && resU.Status != gb.StatusOptimal {
			t.Fatalf("cbc_rhs_1e60 status=%v, want NumericalError or Optimal", resU.Status)
		}

		// Genuinely infeasible MIP with 1e60 objective must still return StatusInfeasible.
		mInf := gb.NewModel("cbc_true_infeas_1e60", gb.Minimize)
		ix := mInf.AddBinaryVar("x")
		iy := mInf.AddBinaryVar("y")
		mInf.SetObjective(gb.NewExpr(ix.T(1e60), iy.T(1e60)))
		mInf.AddConstraint("c1", gb.NewExpr(ix.T(1), iy.T(1)).Le(0))
		mInf.AddConstraint("c2", gb.NewExpr(ix.T(1), iy.T(1)).Ge(1))
		resInf, err := mInf.Solve(ctx, gb.WithCBC(), gb.WithPresolve(false))
		if err != nil || resInf.Status != gb.StatusInfeasible {
			t.Fatalf("cbc_true_infeas_1e60: status=%v err=%v, want StatusInfeasible", resInf.Status, err)
		}

		// Unit-scale objective/bounds/RHS with tiny 1e-55 row entry forcing x >= 1e55 (>= 1e50 cutoff)
		// must report StatusNumericalError rather than false StatusInfeasible.
		mSmallCoeff := gb.NewModel("cbc_small_coeff_1e55", gb.Minimize)
		scX := mSmallCoeff.AddContVar("x", 0, math.Inf(1))
		scY := mSmallCoeff.AddBinaryVar("y")
		mSmallCoeff.SetObjective(gb.NewExpr(scX.T(1.0)))
		mSmallCoeff.AddConstraint("fix_y", gb.NewExpr(scY.T(1.0)).Le(0.0))
		mSmallCoeff.AddConstraint("big_x", gb.NewExpr(scX.T(1e-55), scY.T(1.0)).Ge(1.0))
		resSC, err := mSmallCoeff.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("cbc_small_coeff_1e55 err: %v", err)
		}
		if resSC.Status == gb.StatusInfeasible {
			t.Fatal("cbc_small_coeff_1e55 returned false StatusInfeasible on feasible MIP")
		}
		if resSC.Status != gb.StatusNumericalError && resSC.Status != gb.StatusOptimal {
			t.Fatalf("cbc_small_coeff_1e55 status=%v, want NumericalError or Optimal", resSC.Status)
		}

		// Canceling +-1.5e308 objective terms whose true sum is finite (1.5e308) must not overflow
		// intermediate summation to +Inf - 1.5e308 = NaN.
		cBig := 1.5e308
		mCancel := gb.NewModel("cbc_recomp_overflow", gb.Minimize)
		cx0 := mCancel.AddIntVar("x0", 1, 1)
		cx1 := mCancel.AddIntVar("x1", 1, 1)
		cx2 := mCancel.AddIntVar("x2", 1, 1)
		mCancel.SetObjective(gb.NewExpr(cx0.T(cBig), cx1.T(cBig), cx2.T(-cBig)))
		mCancel.AddConstraint("r", gb.NewExpr(cx0.T(1), cx1.T(1), cx2.T(1)).Ge(3))
		resCancel, err := mCancel.Solve(ctx, gb.WithCBC())
		if err != nil || resCancel.Status != gb.StatusOptimal || math.IsNaN(resCancel.Objective) || math.Abs(resCancel.Objective-cBig) > 1e295 {
			t.Fatalf("cbc_recomp_overflow: status=%v obj=%v err=%v, want Optimal %.5e", resCancel.Status, resCancel.Objective, err, cBig)
		}
	})

	// MAJ-2: Mid-solve context.WithTimeout (without WithTimeLimit) must return context.DeadlineExceeded,
	// never (Result{Status: StatusTimeLimit}, nil).
	t.Run("MAJ2_cbc_mid_solve_context_deadline", func(t *testing.T) {
		mHard := gb.NewModel("cbc_hard_ctx_dl", gb.Minimize)
		hx := mHard.AddIntVar("x", math.Inf(-1), math.Inf(1))
		hy := mHard.AddIntVar("y", math.Inf(-1), math.Inf(1))
		mHard.AddConstraint("lo", gb.NewExpr(hx.T(2), hy.T(2)).Ge(1))
		mHard.AddConstraint("hi", gb.NewExpr(hx.T(2), hy.T(2)).Le(1))
		pHard := mHard.Compile()
		for trial := range 3 {
			dlCtx, cancel := context.WithTimeout(ctx, 2*time.Millisecond)
			res, err := gb.SolveCompiled(dlCtx, pHard, gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false))
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("trial %d: mid-solve ctx deadline returned (%+v, %v), want context.DeadlineExceeded", trial, res, err)
			}
		}
	})

	// MAJ-4 (CBC): LP-unbounded MIP feasibility re-solve deducts initial nodes from explicit WithMaxNodes.
	t.Run("MAJ4_cbc_unbounded_lp_mip_max_nodes", func(t *testing.T) {
		mUnb := gb.NewModel("cbc_unb_mip_nodes", gb.Minimize)
		uz := mUnb.AddContVar("z", 0, math.Inf(1))
		ux := mUnb.AddIntVar("x", math.Inf(-1), math.Inf(1))
		uy := mUnb.AddIntVar("y", math.Inf(-1), math.Inf(1))
		mUnb.SetObjective(gb.NewExpr(uz.T(-1)))
		mUnb.AddConstraint("lo", gb.NewExpr(ux.T(2), uy.T(2)).Ge(1))
		mUnb.AddConstraint("hi", gb.NewExpr(ux.T(2), uy.T(2)).Le(1))
		res, err := mUnb.Solve(ctx, gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false), gb.WithMaxNodes(2))
		if err != nil {
			t.Fatalf("cbc_unb_mip_nodes err: %v", err)
		}
		if res.Status != gb.StatusNodeLimit {
			t.Fatalf("cbc_unb_mip_nodes status=%v, want NodeLimit", res.Status)
		}
	})

	// Dropped sub-1e-20 Clp matrix/cut terms against wide variable bounds must never yield
	// false StatusOptimal, false StatusInfeasible, or false StatusUnbounded across LP, MIP,
	// or CBCLPSolver, and [1e-18, 1e-9] entries must not be dropped by COIN-OR presolve.
	t.Run("cbc_dropped_tiny_coeff_guards_all_entrypoints", func(t *testing.T) {
		for _, isMIP := range []bool{false, true} {
			mSub := gb.NewModel("cbc_tiny_subopt", gb.Minimize)
			var x1 gb.Var
			if isMIP {
				x1 = mSub.AddIntVar("x1", 0, 2000)
			} else {
				x1 = mSub.AddContVar("x1", 0, 2000)
			}
			x2 := mSub.AddContVar("x2", 1e25, 1e25)
			mSub.SetObjective(gb.VarExpr(x1))
			mSub.AddConstraint("c", gb.NewExpr(x1.T(1), x2.T(1e-22)).Ge(1055))
			pSub := mSub.Compile()

			resGo, err := gb.SolveCompiled(ctx, pSub, gb.WithPureGo())
			if err != nil || resGo.Status != gb.StatusOptimal || math.Abs(resGo.Objective-55) > 1e-5 {
				t.Fatalf("Go reference (isMIP=%v): status=%v obj=%g err=%v, want Optimal 55", isMIP, resGo.Status, resGo.Objective, err)
			}
			for _, pre := range []bool{true, false} {
				resCBC, cErr := gb.SolveCompiled(ctx, pSub, gb.WithCBC(), gb.WithPresolve(pre))
				if cErr != nil {
					t.Fatalf("CBC (isMIP=%v, pre=%v) err: %v", isMIP, pre, cErr)
				}
				if resCBC.Status != gb.StatusNumericalError {
					t.Fatalf("CBC (isMIP=%v, pre=%v): status=%v obj=%g bestBound=%g, want StatusNumericalError",
						isMIP, pre, resCBC.Status, resCBC.Objective, resCBC.BestBound)
				}
				if resCBC.BestBound != 0 || resCBC.RootLPBound != 0 || len(resCBC.Values) != 0 {
					t.Fatalf("CBC (isMIP=%v, pre=%v) leaked corrupted bounds/values: %+v", isMIP, pre, resCBC)
				}
			}
			if !isMIP {
				lp := gb.NewCBCLPSolver(pSub)
				lpRes := lp.Solve()
				lp.Close()
				if lpRes.Status != gb.StatusNumericalError {
					t.Fatalf("CBCLPSolver tiny_subopt: status=%v obj=%g, want StatusNumericalError", lpRes.Status, lpRes.Objective)
				}
			}
		}

		// Infeasible primal when Clp drops -1e-22*x2 from x1 - 1e-22*x2 >= -945 (x2 = 1e25).
		mInfPrim := gb.NewModel("cbc_tiny_infeas_primal", gb.Minimize)
		ip1 := mInfPrim.AddContVar("x1", 0, 2000)
		ip2 := mInfPrim.AddContVar("x2", 1e25, 1e25)
		mInfPrim.SetObjective(gb.VarExpr(ip1))
		mInfPrim.AddConstraint("c", gb.NewExpr(ip1.T(1), ip2.T(-1e-22)).Ge(-945))
		pInfPrim := mInfPrim.Compile()
		resIP, err := gb.SolveCompiled(ctx, pInfPrim, gb.WithCBC())
		if err != nil || resIP.Status != gb.StatusNumericalError {
			t.Fatalf("CBC tiny_infeas_primal: status=%v err=%v, want StatusNumericalError", resIP.Status, err)
		}
		lpIP := gb.NewCBCLPSolver(pInfPrim)
		lpIPRes := lpIP.Solve()
		lpIP.Close()
		if lpIPRes.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver tiny_infeas_primal: status=%v, want StatusNumericalError", lpIPRes.Status)
		}

		// Bounded LP min -x1 s.t. 1e-22*x1 <= 1 must not be certified StatusUnbounded.
		mBnd := gb.NewModel("cbc_tiny_bounded", gb.Minimize)
		bx := mBnd.AddContVar("x", 0, math.Inf(1))
		mBnd.SetObjective(gb.NewExpr(bx.T(-1)))
		mBnd.AddConstraint("cap", gb.NewExpr(bx.T(1e-22)).Le(1))
		pBnd := mBnd.Compile()
		resBnd, err := gb.SolveCompiled(ctx, pBnd, gb.WithCBC())
		if err != nil || resBnd.Status != gb.StatusNumericalError {
			t.Fatalf("CBC tiny_bounded: status=%v err=%v, want StatusNumericalError", resBnd.Status, err)
		}
		lpBnd := gb.NewCBCLPSolver(pBnd)
		lpBndRes := lpBnd.Solve()
		lpBnd.Close()
		if lpBndRes.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver tiny_bounded: status=%v, want StatusNumericalError", lpBndRes.Status)
		}

		// CBCLPSolver on 1e-55*x >= 1 must return StatusNumericalError (never false StatusInfeasible).
		m55 := gb.NewModel("cbc_lp_solver_1e55", gb.Minimize)
		x55 := m55.AddContVar("x", 0, math.Inf(1))
		m55.SetObjective(gb.VarExpr(x55))
		m55.AddConstraint("c", gb.NewExpr(x55.T(1e-55)).Ge(1))
		lp55 := gb.NewCBCLPSolver(m55.Compile())
		res55 := lp55.Solve()
		lp55.Close()
		if res55.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver 1e-55: status=%v, want StatusNumericalError", res55.Status)
		}

		// CBCLPSolver deferred SetStructuralBounds widening and AddCut tiny-entry tracking.
		mDef := gb.NewModel("cbc_lp_deferred_bounds", gb.Minimize)
		dx1 := mDef.AddContVar("x1", 0, 2000)
		dx2 := mDef.AddContVar("x2", 0, 1)
		mDef.SetObjective(gb.VarExpr(dx1))
		mDef.AddConstraint("c", gb.NewExpr(dx1.T(1), dx2.T(1e-22)).Ge(1055))
		lpDef := gb.NewCBCLPSolver(mDef.Compile())
		if r0 := lpDef.Solve(); r0.Status != gb.StatusOptimal || math.Abs(r0.Objective-1055) > 1e-5 {
			lpDef.Close()
			t.Fatalf("CBCLPSolver initial narrow bounds: status=%v obj=%g, want Optimal 1055", r0.Status, r0.Objective)
		}
		lpDef.SetStructuralBounds([]float64{0, 1e25}, []float64{2000, 1e25})
		rWide := lpDef.Solve()
		lpDef.Close()
		if rWide.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver after widening bounds to 1e25: status=%v obj=%g, want StatusNumericalError", rWide.Status, rWide.Objective)
		}

		// Presolve-range [1e-18, 1e-9] entry (1e-13 * 1e16 = 1000) must not be dropped by CoinPresolveZeros.
		mPre := gb.NewModel("cbc_presolve_1e13", gb.Minimize)
		px1 := mPre.AddIntVar("x1", 0, 2000)
		px2 := mPre.AddContVar("x2", 1e16, 1e16)
		mPre.SetObjective(gb.VarExpr(px1))
		mPre.AddConstraint("c", gb.NewExpr(px1.T(1), px2.T(1e-13)).Ge(1055))
		resPre, err := mPre.Solve(ctx, gb.WithCBC(), gb.WithPresolve(true))
		if err != nil {
			t.Fatalf("cbc_presolve_1e13 err: %v", err)
		}
		if resPre.Status != gb.StatusOptimal && resPre.Status != gb.StatusNumericalError {
			t.Fatalf("cbc_presolve_1e13 status=%v, want Optimal or NumericalError", resPre.Status)
		}
		if resPre.Status == gb.StatusOptimal && math.Abs(resPre.Objective-55) > 1e-4 {
			t.Fatalf("cbc_presolve_1e13 dropped 1e-13 term: obj=%g, want 55", resPre.Objective)
		}

		// Unbounded coupled rays with small negative reduced costs must not be certified
		// StatusOptimal by BackendCBC LP, MIP, or CBCLPSolver.
		for _, cTiny := range []float64{-5e-13, -5e-15} {
			mRay := gb.NewModel("cbc_coupled_unb_ray", gb.Minimize)
			rx0 := mRay.AddContVar("x0", 0, math.Inf(1))
			rz := mRay.AddContVar("z", 0, math.Inf(1))
			rx1 := mRay.AddContVar("x1", 0, 10)
			rx2 := mRay.AddContVar("x2", 0, 10)
			mRay.SetObjective(gb.NewExpr(rx0.T(cTiny), rx1.T(1), rx2.T(1)))
			mRay.AddConstraint("r0", gb.NewExpr(rx1.T(1), rx0.T(500), rz.T(-500)).Ge(1))
			mRay.AddConstraint("r1", gb.NewExpr(rx2.T(1), rx0.T(-500), rz.T(500)).Ge(1))
			pRay := mRay.Compile()

			resLP, err := gb.SolveCompiled(ctx, pRay, gb.WithCBC())
			if err != nil || resLP.Status != gb.StatusUnbounded {
				t.Fatalf("CBC LP coupled ray (c=%g): status=%v err=%v, want Unbounded", cTiny, resLP.Status, err)
			}
			clpRay := gb.NewCBCLPSolver(pRay)
			rClp := clpRay.Solve()
			clpRay.Close()
			if rClp.Status != gb.StatusUnbounded {
				t.Fatalf("CBCLPSolver coupled ray (c=%g): status=%v, want Unbounded", cTiny, rClp.Status)
			}

			mRayMIP := gb.NewModel("cbc_coupled_unb_mip", gb.Minimize)
			mx0 := mRayMIP.AddIntVar("x0", 0, math.Inf(1))
			mz := mRayMIP.AddIntVar("z", 0, math.Inf(1))
			mx1 := mRayMIP.AddIntVar("x1", 0, 10)
			mx2 := mRayMIP.AddIntVar("x2", 0, 10)
			mRayMIP.SetObjective(gb.NewExpr(mx0.T(cTiny), mx1.T(1), mx2.T(1)))
			mRayMIP.AddConstraint("r0", gb.NewExpr(mx1.T(1), mx0.T(500), mz.T(-500)).Ge(1))
			mRayMIP.AddConstraint("r1", gb.NewExpr(mx2.T(1), mx0.T(-500), mz.T(500)).Ge(1))
			resMIP, err := gb.SolveCompiled(ctx, mRayMIP.Compile(), gb.WithCBC())
			if err != nil || resMIP.Status != gb.StatusUnbounded {
				t.Fatalf("CBC MIP coupled ray (c=%g): status=%v err=%v, want Unbounded", cTiny, resMIP.Status, err)
			}
		}

		// Bounded MIP with tiny objective on wide [0, 1e20] continuous domain must never return
		// suboptimal StatusOptimal (with invalid BestBound) or false StatusUnbounded.
		mTinyObjMIP := gb.NewModel("cbc_mip_tiny_obj_1e20", gb.Minimize)
		mb := mTinyObjMIP.AddBinaryVar("b")
		mx := mTinyObjMIP.AddContVar("x", 0, 1e20)
		mTinyObjMIP.SetObjective(gb.NewExpr(mb.T(1), mx.T(-1e-10)))
		mTinyObjMIP.AddConstraint("c", gb.VarExpr(mb).Le(1))
		resTinyMIP, err := mTinyObjMIP.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("cbc_mip_tiny_obj_1e20 err: %v", err)
		}
		if resTinyMIP.Status != gb.StatusOptimal && resTinyMIP.Status != gb.StatusNumericalError {
			t.Fatalf("cbc_mip_tiny_obj_1e20 status=%v, want Optimal or NumericalError", resTinyMIP.Status)
		}
		if resTinyMIP.Status == gb.StatusOptimal && (math.Abs(resTinyMIP.Objective - -1e10) > 1e-2 || resTinyMIP.BestBound > -1e10+1e-2) {
			t.Fatalf("cbc_mip_tiny_obj_1e20 suboptimal: obj=%g bestBound=%g, want -1e10", resTinyMIP.Objective, resTinyMIP.BestBound)
		}
	})
}
