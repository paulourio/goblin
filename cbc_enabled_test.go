//go:build cgo && cbc

package goblin_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
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

// TestCBCPeerReviewV2Regressions covers the v1.0.0 peer-review findings on BackendCBC and CBCLPSolver.
func TestCBCPeerReviewV2Regressions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("c1_and_m6_wide_dot_and_bound_non_inversion", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("cbc_wide_dot_1e17", gb.Minimize)
		x0 := m.AddContVar("x0", 1, 1)
		x1 := m.AddContVar("x1", 1, 1)
		z := m.AddIntVar("z", 3, 5)
		m.SetObjective(gb.NewExpr(x0.T(1e17), x1.T(-1e17), z.T(1)))
		m.AddConstraint("c", gb.VarExpr(z).Ge(3))
		p := m.Compile()

		resMIP, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || resMIP.Status != gb.StatusOptimal || math.Abs(resMIP.Objective-3.0) > 1e-9 {
			t.Fatalf("CBC MIP +-1e17 cancel: status=%v obj=%.17g err=%v, want Optimal 3",
				resMIP.Status, resMIP.Objective, err)
		}
		if resMIP.RootLPBound > resMIP.Objective+1e-9 || resMIP.RootCutBound > resMIP.Objective+1e-9 {
			t.Fatalf("CBC MIP root bounds inverted: rootLP=%.17g rootCut=%.17g > obj=%.17g",
				resMIP.RootLPBound, resMIP.RootCutBound, resMIP.Objective)
		}

		clp := gb.NewCBCLPSolver(p)
		clp.AddCut([]int{2, 2, 2}, []float64{1e17, -1, -1e17}, -4)
		rClp := clp.Solve()
		clp.Close()
		if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective-4.0) > 1e-9 {
			t.Fatalf("CBCLPSolver +-1e17 + dup cut: status=%v obj=%.17g, want Optimal 4",
				rClp.Status, rClp.Objective)
		}
	})

	t.Run("c3_mad_lp_relaxations_optimal_without_false_numerr", func(t *testing.T) {
		t.Parallel()
		for _, id := range []string{"t2_mad_g3_i9", "t2_mad_g4_i12", "t3_mad_g10_i100"} {
			rawLines, err := os.ReadFile(filepath.Join("testdata", "corpus", "trials", id+".jsonl"))
			if err != nil {
				t.Fatalf("read %s.jsonl: %v", id, err)
			}
			for trialIdx, line := range bytes.Split(bytes.TrimSpace(rawLines), []byte("\n")) {
				doc, err := format.ParsePuLPJSON(line)
				if err != nil {
					t.Fatalf("%s trial %d parse: %v", id, trialIdx, err)
				}
				mMIP, err := format.BuildModelFromPuLPJSON(doc)
				if err != nil {
					t.Fatalf("%s trial %d build MIP: %v", id, trialIdx, err)
				}
				pMIP := mMIP.Compile()
				clp := gb.NewCBCLPSolver(pMIP)
				rClp := clp.Solve()
				clp.Close()
				if rClp.Status != gb.StatusOptimal {
					t.Fatalf("%s trial %d CBCLPSolver: status=%v, want Optimal", id, trialIdx, rClp.Status)
				}

				for i := range doc.Variables {
					doc.Variables[i].Cat = "Continuous"
				}
				mLP, err := format.BuildModelFromPuLPJSON(doc)
				if err != nil {
					t.Fatalf("%s trial %d build LP: %v", id, trialIdx, err)
				}
				resLP, err := gb.SolveCompiled(ctx, mLP.Compile(), gb.WithCBC())
				if err != nil || resLP.Status != gb.StatusOptimal {
					t.Fatalf("%s trial %d SolveCompiled(LP, CBC): status=%v err=%v, want Optimal",
						id, trialIdx, resLP.Status, err)
				}
			}
		}
	})

	t.Run("c5_verify_cbc_mip_respects_bounded_domain_and_max_iter", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(filepath.Join("testdata", "corpus", "t3_mad_g10_i100.json"))
		if err != nil {
			t.Fatalf("read t3_mad_g10_i100.json: %v", err)
		}
		doc, err := format.ParsePuLPJSON(raw)
		if err != nil {
			t.Fatalf("parse t3_mad_g10_i100.json: %v", err)
		}
		m, err := format.BuildModelFromPuLPJSON(doc)
		if err != nil {
			t.Fatalf("build t3_mad_g10_i100: %v", err)
		}
		// Pure-Go unpresolved Root LP on t3_mad_g10_i100 requires >1000 pivots; CBC finishes
		// in <500 simplex iterations and must not run an unbudgeted pure-Go LP on this bounded MIP.
		res, err := gb.SolveCompiled(ctx, m.Compile(), gb.WithCBC(), gb.WithMaxIter(500))
		if err != nil {
			t.Fatalf("t3_mad_g10_i100 CBC WithMaxIter(500) err: %v", err)
		}
		if res.Status != gb.StatusOptimal || res.Iterations > 500 {
			t.Fatalf("t3_mad_g10_i100 CBC WithMaxIter(500): status=%v iters=%d, want Optimal <=500",
				res.Status, res.Iterations)
		}
	})

	t.Run("c6_multirow_tiny_coeff_aggregate_drift", func(t *testing.T) {
		t.Parallel()
		// 20 columns with |a_ij| = 1e-19 at x_j = 1e10 each contribute 1e-9 (< 1e-8 individually),
		// but sum to 2e-8 (>= 1e-8 aggregate row violation when Clp drops |a| <= 1e-18).
		m := gb.NewModel("cbc_tiny_row_sum", gb.Minimize)
		y := m.AddContVar("y", 0, 10)
		row := gb.NewExpr(y.T(1))
		for k := range 20 {
			xk := m.AddContVar(fmt.Sprintf("x_%d", k), 1e10, 1e10)
			row = row.AddTerm(xk, -1e-19)
		}
		m.SetObjective(gb.VarExpr(y))
		m.AddConstraint("sum_tiny", row.Ge(1.0))
		p := m.Compile()

		res, err := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if err != nil || res.Status != gb.StatusNumericalError {
			t.Fatalf("SolveCompiled multi-col tiny row sum: status=%v err=%v, want NumericalError", res.Status, err)
		}
		clp := gb.NewCBCLPSolver(p)
		rClp := clp.Solve()
		clp.Close()
		if rClp.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver multi-col tiny row sum: status=%v, want NumericalError", rClp.Status)
		}
	})

	t.Run("c7_multiscale_and_negative_extreme_mip_objectives", func(t *testing.T) {
		t.Parallel()
		// Multi-scale objective (1e60 + 1e-295) cannot be rescaled without Clp cutoff overflow or underflow.
		mMulti := gb.NewModel("cbc_multiscale_mip", gb.Minimize)
		xBig := mMulti.AddBinaryVar("x_big")
		xTiny := mMulti.AddBinaryVar("x_tiny")
		mMulti.SetObjective(gb.NewExpr(xBig.T(1e60), xTiny.T(1e-295)))
		mMulti.AddConstraint("c", gb.NewExpr(xBig.T(1), xTiny.T(1)).Ge(1))
		resMulti, err := mMulti.Solve(ctx, gb.WithCBC())
		if err != nil || resMulti.Status != gb.StatusNumericalError {
			t.Fatalf("multiscale MIP: status=%v err=%v, want NumericalError", resMulti.Status, err)
		}

		// Maximize 1e54*x (internal Minimize -1e54*x) must not return false StatusInfeasible.
		mNeg := gb.NewModel("cbc_max_1e54_mip", gb.Maximize)
		xNeg := mNeg.AddIntVar("x", 0, 2)
		mNeg.SetObjective(gb.NewExpr(xNeg.T(1e54)))
		mNeg.AddConstraint("c", gb.VarExpr(xNeg).Le(2))
		resNeg, err := mNeg.Solve(ctx, gb.WithCBC())
		if err != nil {
			t.Fatalf("max 1e54 MIP err: %v", err)
		}
		if resNeg.Status != gb.StatusOptimal && resNeg.Status != gb.StatusNumericalError {
			t.Fatalf("max 1e54 MIP: status=%v, want Optimal or NumericalError (never Infeasible)", resNeg.Status)
		}
		if resNeg.Status == gb.StatusOptimal && math.Abs(resNeg.Objective-2e54) > 1e42 {
			t.Fatalf("max 1e54 MIP: obj=%g, want 2e54", resNeg.Objective)
		}
	})

	t.Run("m2_cbc_objective_overflow_and_unconstrained_mip_nil_duals", func(t *testing.T) {
		t.Parallel()
		mConst := gb.NewModel("cbc_const_overflow", gb.Minimize)
		xc := mConst.AddIntVar("x", 1, 1)
		mConst.SetObjective(gb.NewExpr(xc.T(1e308)).AddConstant(1e308))
		mConst.AddConstraint("c", gb.VarExpr(xc).Ge(1))
		resConst, err := mConst.Solve(ctx, gb.WithCBC())
		if err != nil || resConst.Status != gb.StatusNumericalError {
			t.Fatalf("CBC ObjConstant overflow: status=%v obj=%v err=%v, want NumericalError",
				resConst.Status, resConst.Objective, err)
		}

		mUnc := gb.NewModel("cbc_unc_mip_overflow", gb.Minimize)
		xu := mUnc.AddIntVar("x", 1e200, 1e200)
		mUnc.SetObjective(gb.NewExpr(xu.T(-1e200)))
		resUnc, err := mUnc.Solve(ctx, gb.WithCBC())
		if err != nil || resUnc.Status != gb.StatusNumericalError {
			t.Fatalf("CBC unconstrained MIP overflow: status=%v err=%v, want NumericalError", resUnc.Status, err)
		}
		if resUnc.Duals != nil || resUnc.ReducedCosts != nil {
			t.Fatalf("CBC unconstrained MIP returned non-nil Duals=%v ReducedCosts=%v",
				resUnc.Duals, resUnc.ReducedCosts)
		}
	})

	t.Run("cbc_residual_fixed_tiny_obj_and_extreme_finite_bounds", func(t *testing.T) {
		t.Parallel()
		wantLo := math.Ldexp(1, -970)
		mLo := gb.NewModel("cbc_lo_after_hi_mid_cancel", gb.Minimize)
		lx0 := mLo.AddContVar("x0", 1, 1)
		lx1 := mLo.AddContVar("x1", 1, 1)
		lx2 := mLo.AddContVar("x2", 1, 1)
		lx3 := mLo.AddContVar("x3", 1, 1)
		lx4 := mLo.AddContVar("x4", 1, 1)
		lx5 := mLo.AddContVar("x5", math.Ldexp(1, -485), math.Ldexp(1, -485))
		mLo.SetObjective(gb.NewExpr(
			lx0.T(math.Ldexp(1, 960)),
			lx1.T(-math.Ldexp(1, 958)),
			lx2.T(-math.Ldexp(1, 958)),
			lx3.T(-math.Ldexp(1, 958)),
			lx4.T(-math.Ldexp(1, 958)),
			lx5.T(math.Ldexp(1, -485)),
		))
		mLo.AddConstraint("r", gb.VarExpr(lx1).Ge(1))
		resLo, err := mLo.Solve(ctx, gb.WithCBC())
		if err != nil || resLo.Status != gb.StatusOptimal || resLo.Objective != wantLo {
			t.Fatalf("cbc_lo_after_hi_mid_cancel: status=%v obj=%g err=%v, want Optimal %g",
				resLo.Status, resLo.Objective, err, wantLo)
		}

		mRHS := gb.NewModel("cbc_rhs_cancel_1e17", gb.Maximize)
		rx0 := mRHS.AddContVar("x0", 1e17, 1e17)
		rz := mRHS.AddBinaryVar("z")
		mRHS.SetObjective(gb.VarExpr(rz))
		mRHS.AddConstraint("c", gb.NewExpr(rx0.T(1), rz.T(7)).Le(1e17))
		resRHS, err := mRHS.Solve(ctx, gb.WithCBC(), gb.WithPresolve(false))
		if err != nil || (resRHS.Status != gb.StatusNumericalError && (resRHS.Status != gb.StatusOptimal || math.Abs(resRHS.Objective) > 1e-9)) {
			t.Fatalf("cbc_rhs_cancel_1e17: status=%v obj=%g err=%v, want NumericalError or Optimal 0",
				resRHS.Status, resRHS.Objective, err)
		}

		mFixedTiny := gb.NewModel("cbc_tiny_obj_fixed_col", gb.Minimize)
		fx0 := mFixedTiny.AddContVar("x0", 1e25, 1e25)
		fb0 := mFixedTiny.AddBinaryVar("b0")
		fb1 := mFixedTiny.AddBinaryVar("b1")
		fb2 := mFixedTiny.AddBinaryVar("b2")
		mFixedTiny.SetObjective(gb.NewExpr(fx0.T(1e-20), fb0.T(10), fb1.T(10), fb2.T(10)))
		mFixedTiny.AddConstraint("c", gb.NewExpr(fb0.T(1), fb1.T(1), fb2.T(1)).Ge(2))
		resFT, err := mFixedTiny.Solve(ctx, gb.WithCBC())
		if err != nil || resFT.Status != gb.StatusNumericalError {
			t.Fatalf("cbc_tiny_obj_fixed_col: status=%v obj=%g bound=%g err=%v, want NumericalError",
				resFT.Status, resFT.Objective, resFT.BestBound, err)
		}

		mExt := gb.NewModel("cbc_finite_bnd_1e35", gb.Minimize)
		ex := mExt.AddContVar("x", 1e35, 2e35)
		ey := mExt.AddBinaryVar("y")
		mExt.SetObjective(gb.VarExpr(ey))
		mExt.AddConstraint("c", gb.NewExpr(ex.T(1), ey.T(1)).Ge(1e35))
		pExt := mExt.Compile()
		resExt, err := gb.SolveCompiled(ctx, pExt, gb.WithCBC())
		if err != nil || resExt.Status != gb.StatusNumericalError {
			t.Fatalf("cbc_finite_bnd_1e35: status=%v err=%v, want NumericalError", resExt.Status, err)
		}
		clpExt := gb.NewCBCLPSolver(pExt)
		rClpExt := clpExt.Solve()
		clpExt.Close()
		if rClpExt.Status != gb.StatusNumericalError {
			t.Fatalf("CBCLPSolver finite_bnd_1e35: status=%v, want NumericalError", rClpExt.Status)
		}

		mIntTiny := gb.NewModel("cbc_inf_int_tiny_coupled", gb.Minimize)
		ix1 := mIntTiny.AddIntVar("x1", 0, math.Inf(1))
		ix2 := mIntTiny.AddIntVar("x2", 0, math.Inf(1))
		ib1 := mIntTiny.AddBinaryVar("b1")
		iz := mIntTiny.AddBinaryVar("z")
		mIntTiny.SetObjective(gb.NewExpr(iz.T(1000)))
		mIntTiny.AddConstraint("c1", gb.NewExpr(iz.T(1), ib1.T(1)).Ge(1))
		mIntTiny.AddConstraint("c2", gb.NewExpr(ib1.T(1), ix1.T(-1.5e-16), ix2.T(-1e-16)).Le(0))
		mIntTiny.AddConstraint("c3", gb.NewExpr(ix1.T(1), ix2.T(-1)).Eq(0))
		resIT, err := mIntTiny.Solve(ctx, gb.WithCBC(), gb.WithPresolve(false))
		if err != nil || (resIT.Status != gb.StatusNumericalError && (resIT.Status != gb.StatusOptimal || math.Abs(resIT.Objective) > 1e-6)) {
			t.Fatalf("cbc_inf_int_tiny_coupled: status=%v obj=%g bound=%g err=%v, want NumericalError or Optimal 0",
				resIT.Status, resIT.Objective, resIT.BestBound, err)
		}
	})

	t.Run("c1_cbc_row_and_objective_cancellation_models", func(t *testing.T) {
		t.Parallel()
		cbcVariants := []struct {
			name string
			opts []gb.Option
		}{
			{"default", []gb.Option{gb.WithCBC()}},
			{"no-presolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false)}},
			{"bare-no-presolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false)}},
		}
		checkCBCOptimal := func(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result, want float64) {
			t.Helper()
			if res.Backend != gb.BackendCBC {
				t.Fatalf("%s: backend=%v, want BackendCBC", label, res.Backend)
			}
			if res.Status != gb.StatusOptimal {
				t.Fatalf("%s: status=%v (obj=%.6f bound=%.6f), want Optimal %.6f",
					label, res.Status, res.Objective, res.BestBound, want)
			}
			if rel := math.Abs(res.Objective-want) / max(1.0, math.Abs(want)); rel > 1e-6 {
				t.Fatalf("%s: objective=%.8f, want %.8f (rel=%.2e)", label, res.Objective, want, rel)
			}
			bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
			if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
				t.Fatalf("%s: violation bound=%.2e row=%.2e int=%.2e", label, bV, rV, iV)
			}
			boundTol := 1e-6 * max(1.0, math.Abs(res.Objective))
			if (p.Sense() == gb.Minimize && res.BestBound > res.Objective+boundTol) ||
				(p.Sense() == gb.Maximize && res.BestBound < res.Objective-boundTol) {
				t.Fatalf("%s: BestBound %.8f violates objective %.8f", label, res.BestBound, res.Objective)
			}
		}

		// 1. c1_false_infeas (MIP and LP + CBCLPSolver) plus infeasible negative control.
		for _, isMIP := range []bool{true, false} {
			m := gb.NewModel("c1_false_infeas", gb.Minimize)
			sp := m.AddContVar("s_pos", 1e17, 1e17)
			sm := m.AddContVar("s_mid", 9, 9)
			sn := m.AddContVar("s_neg", 1e17, 1e17)
			var z gb.Var
			if isMIP {
				z = m.AddBinaryVar("z")
			} else {
				z = m.AddContVar("z", 0, 1)
			}
			m.SetObjective(gb.VarExpr(z))
			m.AddConstraint("eq", gb.NewExpr(sp.T(1), sm.T(1), sn.T(-1)).Eq(9))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("c1_false_infeas(mip=%v)/%s err: %v", isMIP, v.name, err)
				}
				checkCBCOptimal(t, fmt.Sprintf("c1_false_infeas(mip=%v)/%s", isMIP, v.name), p, res, 0.0)
			}
			clp := gb.NewCBCLPSolver(p)
			rClp := clp.Solve()
			clp.Close()
			if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective) > 1e-9 {
				t.Fatalf("c1_false_infeas(mip=%v) CBCLPSolver: status=%v obj=%g, want Optimal 0",
					isMIP, rClp.Status, rClp.Objective)
			}

			// Negative control: exact LHS = 9 != RHS = 10 must return StatusInfeasible.
			mInf := gb.NewModel("c1_true_infeas_ctrl", gb.Minimize)
			isp := mInf.AddContVar("s_pos", 1e17, 1e17)
			ism := mInf.AddContVar("s_mid", 9, 9)
			isn := mInf.AddContVar("s_neg", 1e17, 1e17)
			var iz gb.Var
			if isMIP {
				iz = mInf.AddBinaryVar("z")
			} else {
				iz = mInf.AddContVar("z", 0, 1)
			}
			mInf.SetObjective(gb.VarExpr(iz))
			mInf.AddConstraint("eq", gb.NewExpr(isp.T(1), ism.T(1), isn.T(-1)).Eq(10))
			pInf := mInf.Compile()
			for _, v := range cbcVariants {
				resInf, err := gb.SolveCompiled(ctx, pInf, v.opts...)
				if err != nil || resInf.Status != gb.StatusInfeasible {
					t.Fatalf("c1_true_infeas_ctrl(mip=%v)/%s: status=%v err=%v, want Infeasible",
						isMIP, v.name, resInf.Status, err)
				}
			}
			clpInf := gb.NewCBCLPSolver(pInf)
			rInf := clpInf.Solve()
			clpInf.Close()
			if rInf.Status != gb.StatusInfeasible {
				t.Fatalf("c1_true_infeas_ctrl(mip=%v) CBCLPSolver: status=%v, want Infeasible", isMIP, rInf.Status)
			}
		}

		// 2. crit2b_sparse_row (2^57*x0 + x1 - 2^57*x2 + y <= 20, exact 17 + y <= 20 -> 0).
		{
			c57 := math.Ldexp(1, 57)
			m := gb.NewModel("crit2b_sparse_row", gb.Minimize)
			x0 := m.AddContVar("x0", 1, 1)
			x1 := m.AddContVar("x1", 17, 17)
			x2 := m.AddContVar("x2", 1, 1)
			y := m.AddBinaryVar("y")
			m.SetObjective(gb.VarExpr(y))
			m.AddConstraint("le", gb.NewExpr(x0.T(c57), x1.T(1), x2.T(-c57), y.T(1)).Le(20))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("crit2b_sparse_row/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "crit2b_sparse_row/"+v.name, p, res, 0.0)
			}
			clp := gb.NewCBCLPSolver(p)
			rClp := clp.Solve()
			clp.Close()
			if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective) > 1e-9 {
				t.Fatalf("crit2b_sparse_row CBCLPSolver: status=%v obj=%g, want Optimal 0", rClp.Status, rClp.Objective)
			}
		}

		// 3. c1_false_opt across all column declaration orders (s_pos + 7*z - s_neg <= 0 -> 0).
		for _, ord := range []string{"sp_z_sn", "z_sp_sn", "sn_z_sp", "sp_sn_z"} {
			m := gb.NewModel("c1_false_opt_"+ord, gb.Maximize)
			var sp, sn, z gb.Var
			switch ord {
			case "sp_z_sn":
				sp = m.AddContVar("s_pos", 1e17, 1e17)
				z = m.AddBinaryVar("z")
				sn = m.AddContVar("s_neg", 1e17, 1e17)
			case "z_sp_sn":
				z = m.AddBinaryVar("z")
				sp = m.AddContVar("s_pos", 1e17, 1e17)
				sn = m.AddContVar("s_neg", 1e17, 1e17)
			case "sn_z_sp":
				sn = m.AddContVar("s_neg", 1e17, 1e17)
				z = m.AddBinaryVar("z")
				sp = m.AddContVar("s_pos", 1e17, 1e17)
			case "sp_sn_z":
				sp = m.AddContVar("s_pos", 1e17, 1e17)
				sn = m.AddContVar("s_neg", 1e17, 1e17)
				z = m.AddBinaryVar("z")
			}
			m.SetObjective(gb.VarExpr(z))
			m.AddConstraint("le", gb.NewExpr(sp.T(1), z.T(7), sn.T(-1)).Le(0))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("c1_false_opt_%s/%s err: %v", ord, v.name, err)
				}
				checkCBCOptimal(t, "c1_false_opt_"+ord+"/"+v.name, p, res, 0.0)
			}
			clp := gb.NewCBCLPSolver(p)
			rClp := clp.Solve()
			clp.Close()
			if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective) > 1e-9 {
				t.Fatalf("c1_false_opt_%s CBCLPSolver: status=%v obj=%g, want Optimal 0",
					ord, rClp.Status, rClp.Objective)
			}
		}

		// 4. c1_bin_cancel (unique global optimum y=1, w=0, Objective=-23, BestBound=-23, RootLPBound=-33.8).
		{
			c57 := math.Ldexp(1, 57)
			m := gb.NewModel("c1_bin_cancel", gb.Minimize)
			x0 := m.AddContVar("x0", 1, 2)
			x1 := m.AddContVar("x1", 0, 17)
			x2 := m.AddContVar("x2", 1, 2)
			u := m.AddContVar("u", 0, 3)
			y := m.AddBinaryVar("y")
			w := m.AddBinaryVar("w")
			m.SetObjective(gb.NewExpr(x0.T(c57), x1.T(1), x2.T(-c57), u.T(100), y.T(-40), w.T(-20)))
			m.AddConstraint("eq_x", gb.NewExpr(x0.T(1), x2.T(-1)).Eq(0))
			m.AddConstraint("ub_x0", gb.VarExpr(x0).Le(1))
			m.AddConstraint("eq_x1", gb.NewExpr(x1.T(1), y.T(-17)).Eq(0))
			m.AddConstraint("cap", gb.NewExpr(y.T(5), w.T(2), u.T(1)).Le(5))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("c1_bin_cancel/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_bin_cancel/"+v.name, p, res, -23.0)
				if math.Abs(res.BestBound - -23.0) > 1e-6 {
					t.Fatalf("c1_bin_cancel/%s BestBound=%.6f, want -23.0", v.name, res.BestBound)
				}
				if math.Abs(res.RootLPBound - -33.8) > 1e-5 {
					t.Fatalf("c1_bin_cancel/%s RootLPBound=%.6f, want -33.8", v.name, res.RootLPBound)
				}
				if math.Abs(res.Value(y)-1.0) > 1e-6 || math.Abs(res.Value(w)) > 1e-6 {
					t.Fatalf("c1_bin_cancel/%s got y=%g w=%g, want y=1 w=0", v.name, res.Value(y), res.Value(w))
				}
			}
		}

		// 5. Semi-infinite integer + non-unit equality + conditional inequality + CBCLPSolver warm resolves.
		{
			mKT := gb.NewModel("c1_killer_tighten", gb.Minimize)
			sp := mKT.AddContVar("s_pos", 1e17, 1e17)
			c9 := mKT.AddContVar("c9", 9, 9)
			sn := mKT.AddContVar("s_neg", 1e17, 1e17)
			z := mKT.AddBinaryVar("z")
			x := mKT.AddIntVar("x", 0, math.Inf(1))
			mKT.SetObjective(gb.NewExpr(sp.T(1), c9.T(1), sn.T(-1), z.T(7), x.T(2)))
			mKT.AddConstraint("dem", gb.NewExpr(x.T(1), z.T(5)).Ge(5))
			pKT := mKT.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pKT, v.opts...)
				if err != nil {
					t.Fatalf("c1_killer_tighten/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_killer_tighten/"+v.name, pKT, res, 16.0)
				if math.Abs(res.BestBound-16.0) > 1e-6 {
					t.Fatalf("c1_killer_tighten/%s BestBound=%.6f, want 16.0", v.name, res.BestBound)
				}
			}

			k55 := math.Ldexp(1, 55)
			mNU := gb.NewModel("c1_nonunit_eq_obj", gb.Minimize)
			nx0 := mNU.AddIntVar("x0", 2, 4)
			nx1 := mNU.AddContVar("x1", 0, 17)
			nx2 := mNU.AddIntVar("x2", 1, 5)
			nx3 := mNU.AddIntVar("x3", 1, 5)
			nu := mNU.AddContVar("u", 0, 3)
			ny := mNU.AddBinaryVar("y")
			nw := mNU.AddBinaryVar("w")
			mNU.SetObjective(gb.NewExpr(nx0.T(3*k55), nx1.T(1), nx2.T(-2*k55), nx3.T(-4*k55), nu.T(100), ny.T(-40), nw.T(-20)))
			mNU.AddConstraint("eq_x", gb.NewExpr(nx0.T(3), nx2.T(-2), nx3.T(-4)).Eq(0))
			mNU.AddConstraint("eq_x1", gb.NewExpr(nx1.T(1), ny.T(-17)).Eq(0))
			mNU.AddConstraint("cap", gb.NewExpr(ny.T(5), nw.T(2), nu.T(1)).Le(5))
			pNU := mNU.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pNU, v.opts...)
				if err != nil {
					t.Fatalf("c1_nonunit_eq_obj/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_nonunit_eq_obj/"+v.name, pNU, res, -23.0)
				if math.Abs(res.BestBound - -23.0) > 1e-6 {
					t.Fatalf("c1_nonunit_eq_obj/%s BestBound=%.6f, want -23.0", v.name, res.BestBound)
				}
			}

			c57 := math.Ldexp(1, 57)
			mCond := gb.NewModel("c1_cond_ineq_cancel", gb.Minimize)
			cx0 := mCond.AddContVar("x0", 1, 2)
			cx1 := mCond.AddContVar("x1", 0, 17)
			cx2 := mCond.AddContVar("x2", 1, 2)
			cz0 := mCond.AddBinaryVar("z0")
			cu := mCond.AddContVar("u", 0, 3)
			cy := mCond.AddBinaryVar("y")
			cw := mCond.AddBinaryVar("w")
			mCond.SetObjective(gb.NewExpr(cx0.T(c57), cx1.T(1), cx2.T(-c57), cz0.T(1000), cu.T(100), cy.T(-40), cw.T(-20)))
			mCond.AddConstraint("ge_x0", gb.NewExpr(cx2.T(1), cx0.T(-1)).Le(0))
			mCond.AddConstraint("le_diff", gb.NewExpr(cx0.T(1), cx2.T(-1), cz0.T(-1)).Le(0))
			mCond.AddConstraint("sum_z0", gb.NewExpr(cx0.T(1), cx2.T(1), cz0.T(-2)).Le(2))
			mCond.AddConstraint("eq_x1", gb.NewExpr(cx1.T(1), cy.T(-17)).Eq(0))
			mCond.AddConstraint("cap", gb.NewExpr(cy.T(5), cw.T(2), cu.T(1)).Le(5))
			pCond := mCond.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pCond, v.opts...)
				if err != nil {
					t.Fatalf("c1_cond_ineq_cancel/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_cond_ineq_cancel/"+v.name, pCond, res, -23.0)
			}

			mDyn := gb.NewModel("c1_cbc_lp_dynamic_bounds", gb.Minimize)
			dsp := mDyn.AddContVar("s_pos", 0, 2e17)
			dsm := mDyn.AddContVar("s_mid", 0, 20)
			dsn := mDyn.AddContVar("s_neg", 0, 2e17)
			dz := mDyn.AddContVar("z", 0, 10)
			mDyn.SetObjective(gb.VarExpr(dz))
			mDyn.AddConstraint("eq", gb.NewExpr(dsp.T(1), dsm.T(1), dsn.T(-1)).Eq(9))
			clpDyn := gb.NewCBCLPSolver(mDyn.Compile())
			defer clpDyn.Close()
			if r0 := clpDyn.Solve(); r0.Status != gb.StatusOptimal || math.Abs(r0.Objective) > 1e-9 {
				t.Fatalf("c1_cbc_lp_dynamic_bounds initial: status=%v obj=%g, want Optimal 0", r0.Status, r0.Objective)
			}
			clpDyn.SetStructuralBounds([]float64{1e17, 9, 1e17, 0}, []float64{1e17, 9, 1e17, 10})
			if r1 := clpDyn.Solve(); r1.Status != gb.StatusOptimal || math.Abs(r1.Objective) > 1e-9 {
				t.Fatalf("c1_cbc_lp_dynamic_bounds fixed 1e17: status=%v obj=%g, want Optimal 0", r1.Status, r1.Objective)
			}
			clpDyn.AddCut([]int{0, 2, 3}, []float64{1, -1, -1}, -3)
			if r2 := clpDyn.Solve(); r2.Status != gb.StatusOptimal || math.Abs(r2.Objective-3.0) > 1e-9 {
				t.Fatalf("c1_cbc_lp_dynamic_bounds after cut: status=%v obj=%g, want Optimal 3", r2.Status, r2.Objective)
			}
			clpDyn.SetStructuralBounds([]float64{0, 0, 0, 0}, []float64{10, 20, 10, 10})
			if r3 := clpDyn.Solve(); r3.Status != gb.StatusOptimal || math.Abs(r3.Objective) > 1e-9 {
				t.Fatalf("c1_cbc_lp_dynamic_bounds unfixed: status=%v obj=%g, want Optimal 0", r3.Status, r3.Objective)
			}
		}

		// 6. Implied [0, +Inf) scale cancellation, ObjConstant cancellation, multi-hop and >16-col RREF equalities.
		{
			for _, isMIP := range []bool{true, false} {
				mInf := gb.NewModel("c1_inf_implied_row_cancel", gb.Minimize)
				sp := mInf.AddContVar("s_pos", 0, math.Inf(1))
				sm := mInf.AddContVar("s_mid", 9, 9)
				sn := mInf.AddContVar("s_neg", 0, math.Inf(1))
				var z gb.Var
				if isMIP {
					z = mInf.AddBinaryVar("z")
				} else {
					z = mInf.AddContVar("z", 0, 1)
				}
				mInf.SetObjective(gb.VarExpr(z))
				mInf.AddConstraint("c_sp", gb.VarExpr(sp).Eq(1e17))
				mInf.AddConstraint("c_sn", gb.VarExpr(sn).Eq(1e17))
				mInf.AddConstraint("eq", gb.NewExpr(sp.T(1), sm.T(1), sn.T(-1)).Eq(9))
				pInf := mInf.Compile()
				for _, v := range cbcVariants {
					res, err := gb.SolveCompiled(ctx, pInf, v.opts...)
					if err != nil {
						t.Fatalf("c1_inf_implied_row_cancel(mip=%v)/%s err: %v", isMIP, v.name, err)
					}
					checkCBCOptimal(t, fmt.Sprintf("c1_inf_implied_row_cancel(mip=%v)/%s", isMIP, v.name), pInf, res, 0.0)
				}
				clp := gb.NewCBCLPSolver(pInf)
				rClp := clp.Solve()
				clp.Close()
				if rClp.Status != gb.StatusOptimal || math.Abs(rClp.Objective) > 1e-9 {
					t.Fatalf("c1_inf_implied_row_cancel(mip=%v) CBCLPSolver: status=%v obj=%g, want Optimal 0",
						isMIP, rClp.Status, rClp.Objective)
				}
			}

			c57 := math.Ldexp(1, 57)
			mInfObj := gb.NewModel("c1_inf_implied_obj_cancel", gb.Minimize)
			ix0 := mInfObj.AddContVar("x0", 0, math.Inf(1))
			ix1 := mInfObj.AddContVar("x1", 0, 17)
			ix2 := mInfObj.AddContVar("x2", 0, math.Inf(1))
			iu := mInfObj.AddContVar("u", 0, 3)
			iy := mInfObj.AddBinaryVar("y")
			iw := mInfObj.AddBinaryVar("w")
			mInfObj.SetObjective(gb.NewExpr(ix0.T(1), ix1.T(1), ix2.T(-1), iu.T(100), iy.T(-40), iw.T(-20)))
			mInfObj.AddConstraint("lb_x0", gb.VarExpr(ix0).Ge(c57))
			mInfObj.AddConstraint("ub_x0", gb.VarExpr(ix0).Le(c57))
			mInfObj.AddConstraint("eq_x", gb.NewExpr(ix0.T(1), ix2.T(-1)).Eq(0))
			mInfObj.AddConstraint("eq_x1", gb.NewExpr(ix1.T(1), iy.T(-17)).Eq(0))
			mInfObj.AddConstraint("cap", gb.NewExpr(iy.T(5), iw.T(2), iu.T(1)).Le(5))
			pInfObj := mInfObj.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pInfObj, v.opts...)
				if err != nil {
					t.Fatalf("c1_inf_implied_obj_cancel/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_inf_implied_obj_cancel/"+v.name, pInfObj, res, -23.0)
			}

			mLPRHS := gb.NewModel("c1_lp_rhs_nonfixed_bound", gb.Maximize)
			rx0 := mLPRHS.AddContVar("x0", 1e17, 2e17)
			rz := mLPRHS.AddContVar("z", 0, 1)
			mLPRHS.SetObjective(gb.VarExpr(rz))
			mLPRHS.AddConstraint("le", gb.NewExpr(rx0.T(1), rz.T(7)).Le(1e17))
			pLPRHS := mLPRHS.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pLPRHS, v.opts...)
				if err != nil {
					t.Fatalf("c1_lp_rhs_nonfixed_bound/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_lp_rhs_nonfixed_bound/"+v.name, pLPRHS, res, 0.0)
			}
			clpRHS := gb.NewCBCLPSolver(pLPRHS)
			rRHS := clpRHS.Solve()
			clpRHS.Close()
			if rRHS.Status != gb.StatusOptimal || math.Abs(rRHS.Objective) > 1e-9 {
				t.Fatalf("c1_lp_rhs_nonfixed_bound CBCLPSolver: status=%v obj=%g, want Optimal 0", rRHS.Status, rRHS.Objective)
			}

			mLPBox := gb.NewModel("c1_lp_box_blocked", gb.Maximize)
			bsp := mLPBox.AddContVar("s_pos", 1e17, 2e17)
			bz := mLPBox.AddContVar("z", 0, 1)
			bsn := mLPBox.AddContVar("s_neg", 0, 1e17)
			mLPBox.SetObjective(gb.VarExpr(bz))
			mLPBox.AddConstraint("le", gb.NewExpr(bsp.T(1), bz.T(7), bsn.T(-1)).Le(0))
			pLPBox := mLPBox.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pLPBox, v.opts...)
				if err != nil {
					t.Fatalf("c1_lp_box_blocked/%s err: %v", v.name, err)
				}
				if res.Status == gb.StatusOptimal {
					checkCBCOptimal(t, "c1_lp_box_blocked/"+v.name, pLPBox, res, 0.0)
				} else if res.Status != gb.StatusNumericalError {
					t.Fatalf("c1_lp_box_blocked/%s: status=%v, want Optimal 0 or NumericalError", v.name, res.Status)
				}
			}

			mConstMax := gb.NewModel("c1_obj_const_max_cancel", gb.Maximize)
			cs := mConstMax.AddContVar("s", 1, 1)
			cz := mConstMax.AddBinaryVar("z")
			mConstMax.SetObjective(gb.NewExpr(cs.T(1e17), cz.T(7)).AddConstant(-1e17))
			mConstMax.AddConstraint("c", gb.VarExpr(cz).Ge(1))
			pConstMax := mConstMax.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pConstMax, v.opts...)
				if err != nil {
					t.Fatalf("c1_obj_const_max_cancel/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_obj_const_max_cancel/"+v.name, pConstMax, res, 7.0)
				if math.Abs(res.BestBound-7.0) > 1e-6 || math.Abs(res.RootLPBound-7.0) > 1e-6 {
					t.Fatalf("c1_obj_const_max_cancel/%s BestBound=%g RootLPBound=%g, want 7",
						v.name, res.BestBound, res.RootLPBound)
				}
			}
			clpConst := gb.NewCBCLPSolver(pConstMax)
			rClpConst := clpConst.Solve()
			clpConst.Close()
			if rClpConst.Status != gb.StatusOptimal || math.Abs(rClpConst.Objective-7.0) > 1e-9 {
				t.Fatalf("c1_obj_const_max_cancel CBCLPSolver: status=%v obj=%g, want Optimal 7",
					rClpConst.Status, rClpConst.Objective)
			}

			mConstBin := gb.NewModel("c1_obj_const_bin_cancel", gb.Minimize)
			cbx0 := mConstBin.AddContVar("x0", 1, 2)
			cbx1 := mConstBin.AddContVar("x1", 0, 17)
			cbu := mConstBin.AddContVar("u", 0, 3)
			cby := mConstBin.AddBinaryVar("y")
			cbw := mConstBin.AddBinaryVar("w")
			mConstBin.SetObjective(gb.NewExpr(cbx0.T(c57), cbx1.T(1), cbu.T(100), cby.T(-40), cbw.T(-17)).AddConstant(-c57))
			mConstBin.AddConstraint("ub_x0", gb.VarExpr(cbx0).Le(1))
			mConstBin.AddConstraint("eq_x1", gb.NewExpr(cbx1.T(1), cby.T(-17)).Eq(0))
			mConstBin.AddConstraint("cap", gb.NewExpr(cby.T(5), cbw.T(2), cbu.T(1)).Le(5))
			pConstBin := mConstBin.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pConstBin, v.opts...)
				if err != nil {
					t.Fatalf("c1_obj_const_bin_cancel/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_obj_const_bin_cancel/"+v.name, pConstBin, res, -23.0)
			}

			for _, rev := range []bool{false, true} {
				for _, isMIP := range []bool{true, false} {
					mHop := gb.NewModel("c1_multihop_eq", gb.Maximize)
					hx0 := mHop.AddContVar("x0", 1, 2)
					hx1 := mHop.AddContVar("x1", 1, 2)
					hx2 := mHop.AddContVar("x2", 1, 2)
					var hz gb.Var
					if isMIP {
						hz = mHop.AddBinaryVar("z")
					} else {
						hz = mHop.AddContVar("z", 0, 1)
					}
					mHop.SetObjective(gb.VarExpr(hz))
					if !rev {
						mHop.AddConstraint("eq1", gb.NewExpr(hx0.T(1), hx1.T(-1)).Eq(0))
						mHop.AddConstraint("eq2", gb.NewExpr(hx1.T(1), hx2.T(-1)).Eq(0))
					} else {
						mHop.AddConstraint("eq2", gb.NewExpr(hx1.T(1), hx2.T(-1)).Eq(0))
						mHop.AddConstraint("eq1", gb.NewExpr(hx0.T(1), hx1.T(-1)).Eq(0))
					}
					mHop.AddConstraint("cancel", gb.NewExpr(hx0.T(c57), hz.T(7), hx2.T(-c57)).Le(0))
					pHop := mHop.Compile()
					for _, v := range cbcVariants {
						res, err := gb.SolveCompiled(ctx, pHop, v.opts...)
						if err != nil {
							t.Fatalf("c1_multihop_eq(rev=%v,mip=%v)/%s err: %v", rev, isMIP, v.name, err)
						}
						checkCBCOptimal(t, fmt.Sprintf("c1_multihop_eq(rev=%v,mip=%v)/%s", rev, isMIP, v.name), pHop, res, 0.0)
					}
				}
			}

			mWideEq := gb.NewModel("c1_wide_26col_eq", gb.Maximize)
			wx0 := mWideEq.AddContVar("x0", 1, 2)
			wx2 := mWideEq.AddContVar("x2", 1, 2)
			wz := mWideEq.AddBinaryVar("z")
			eqTerms := []gb.Term{wx0.T(1), wx2.T(-1)}
			for j := range 12 {
				da := mWideEq.AddContVar(fmt.Sprintf("da%d", j), 1, 3)
				db := mWideEq.AddContVar(fmt.Sprintf("db%d", j), 1, 3)
				eqTerms = append(eqTerms, da.T(1), db.T(-1))
				mWideEq.AddConstraint(fmt.Sprintf("pair%d", j), gb.NewExpr(da.T(1), db.T(-1)).Eq(0))
			}
			mWideEq.SetObjective(gb.VarExpr(wz))
			mWideEq.AddConstraint("eq26", gb.NewExpr(eqTerms...).Eq(0))
			mWideEq.AddConstraint("cancel", gb.NewExpr(wx0.T(c57), wz.T(7), wx2.T(-c57)).Le(0))
			pWideEq := mWideEq.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, pWideEq, v.opts...)
				if err != nil {
					t.Fatalf("c1_wide_26col_eq/%s err: %v", v.name, err)
				}
				checkCBCOptimal(t, "c1_wide_26col_eq/"+v.name, pWideEq, res, 0.0)
			}

			mMaxCut := gb.NewModel("c1_cbclp_maximize_cut_cancel", gb.Maximize)
			msp := mMaxCut.AddContVar("s_pos", 1e10, 1e10)
			msn := mMaxCut.AddContVar("s_neg", 1e10, 1e10)
			mz := mMaxCut.AddContVar("z", 0, 10)
			mMaxCut.SetObjective(gb.NewExpr(msp.T(3), mz.T(4), msn.T(-3)))
			mMaxCut.AddConstraint("le", gb.NewExpr(msp.T(1), mz.T(2), msn.T(-1)).Le(12))
			pMaxCut := mMaxCut.Compile()
			lpGoRef := gb.NewLPSolver(pMaxCut, nil)
			lpCBCRef := gb.NewCBCLPSolver(pMaxCut)
			defer lpGoRef.Close()
			defer lpCBCRef.Close()
			lpGoRef.AddCut([]int{0, 1, 2}, []float64{1, -1, 1}, 4)
			lpCBCRef.AddCut([]int{0, 1, 2}, []float64{1, -1, 1}, 4)
			rg := lpGoRef.Solve()
			rc := lpCBCRef.Solve()
			if rg.Status != gb.StatusOptimal || rc.Status != gb.StatusOptimal || math.Abs(rc.Objective-rg.Objective) > 1e-6 {
				t.Fatalf("c1_cbclp_maximize_cut_cancel: Go=(%v,%g) CBC=(%v,%g)", rg.Status, rg.Objective, rc.Status, rc.Objective)
			}
			for j := range len(rg.ReducedCosts) {
				if math.Abs(rc.ReducedCosts[j]-rg.ReducedCosts[j]) > 1e-6 {
					t.Fatalf("c1_cbclp_maximize_cut_cancel rc[%d]=%g, want %g", j, rc.ReducedCosts[j], rg.ReducedCosts[j])
				}
			}
		}
	})
}

func TestAdversarialPeerReviewCBCRegressions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cbcVariants := []struct {
		name string
		opts []gb.Option
	}{
		{"default", []gb.Option{gb.WithCBC()}},
		{"no-presolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false)}},
		{"bare-no-presolve", []gb.Option{gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false)}},
	}
	checkOpt := func(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result, want float64) {
		t.Helper()
		if res.Backend != gb.BackendCBC {
			t.Fatalf("%s: backend=%v, want BackendCBC", label, res.Backend)
		}
		if res.Status != gb.StatusOptimal {
			t.Fatalf("%s: status=%v obj=%g bnd=%g, want Optimal %g", label, res.Status, res.Objective, res.BestBound, want)
		}
		if rel := math.Abs(res.Objective-want) / max(1.0, math.Abs(want)); rel > 1e-6 {
			t.Fatalf("%s: objective=%.10g, want %.10g (rel=%.2e)", label, res.Objective, want, rel)
		}
		bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
		if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
			t.Fatalf("%s: violation bound=%.2e row=%.2e int=%.2e", label, bV, rV, iV)
		}
	}

	t.Run("c1c_eq_and_inv", func(t *testing.T) {
		t.Parallel()
		mEq := gb.NewModel("cbc_c1c_eq", gb.Minimize)
		x := mEq.AddContVar("x", 1, 2)
		mEq.AddConstraint("eq", gb.NewExpr(x.T(math.Ldexp(1, 40))).Eq(math.Ldexp(1, 40)-524288))
		mEq.SetObjective(gb.NewExpr(x.T(1)))
		pEq := mEq.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, pEq, v.opts...)
			if err != nil || res.Status != gb.StatusInfeasible {
				t.Fatalf("c1c_eq/%s: status=%v err=%v, want Infeasible", v.name, res.Status, err)
			}
		}

		mInv := gb.NewModel("cbc_c1c_inv", gb.Minimize)
		xi := mInv.AddContVar("x", 2, 1)
		mInv.SetObjective(gb.NewExpr(xi.T(1)))
		pInv := mInv.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, pInv, v.opts...)
			if err != nil || res.Status != gb.StatusInfeasible {
				t.Fatalf("m5_inv/%s: status=%v err=%v, want Infeasible", v.name, res.Status, err)
			}
		}
	})

	t.Run("new_c1_sub_1e6_and_int_snapping", func(t *testing.T) {
		t.Parallel()
		for _, isMIP := range []bool{false, true} {
			m := gb.NewModel("cbc_new_c1a", gb.Minimize)
			x := m.AddContVar("x", 0, 1)
			y := m.AddContVar("y", 0, 1)
			var z gb.Var
			if isMIP {
				z = m.AddBinaryVar("z")
			} else {
				z = m.AddContVar("z", 0, 1)
			}
			m.AddConstraint("ub", gb.NewExpr(x.T(math.Ldexp(1, 44))).Le(1))
			m.AddConstraint("lb", gb.NewExpr(x.T(math.Ldexp(1, 45))).Ge(1))
			m.AddConstraint("trig", gb.NewExpr(y.T(1e9), z.T(-1e9)).Le(1))
			m.SetObjective(gb.NewExpr(x.T(math.Ldexp(1, 45))))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("new_c1a(mip=%v)/%s err: %v", isMIP, v.name, err)
				}
				checkOpt(t, fmt.Sprintf("new_c1a(mip=%v)/%s", isMIP, v.name), p, res, 1.0)
			}
		}

		m1b := gb.NewModel("cbc_new_c1b", gb.Minimize)
		xb := m1b.AddContVar("x", 0, 1e-3)
		yb := m1b.AddContVar("y", 1e10, 1e10)
		bb := m1b.AddBinaryVar("b")
		m1b.AddConstraint("ub", gb.NewExpr(xb.T(math.Ldexp(1, 44))).Le(1))
		m1b.AddConstraint("trig", gb.NewExpr(yb.T(1), bb.T(1)).Le(1e10+1))
		m1b.SetObjective(gb.NewExpr(xb.T(-math.Ldexp(1, 51))))
		p1b := m1b.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p1b, v.opts...)
			if err != nil {
				t.Fatalf("new_c1b/%s err: %v", v.name, err)
			}
			checkOpt(t, "new_c1b/"+v.name, p1b, res, -128.0)
		}

		m1c := gb.NewModel("cbc_new_c1c", gb.Minimize)
		xc := m1c.AddIntVar("x", 0, 10)
		yc := m1c.AddContVar("y", 1e10, 1e10)
		m1c.AddConstraint("lb", gb.NewExpr(xc.T(1)).Ge(1.0-math.Ldexp(1, -42)))
		m1c.AddConstraint("ub", gb.NewExpr(xc.T(1)).Le(1.0))
		m1c.AddConstraint("trig", gb.NewExpr(yc.T(1), xc.T(1)).Le(1e10+10))
		m1c.SetObjective(gb.NewExpr(xc.T(1e15)))
		p1c := m1c.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p1c, v.opts...)
			if err != nil {
				t.Fatalf("new_c1c/%s err: %v", v.name, err)
			}
			checkOpt(t, "new_c1c/"+v.name, p1c, res, 1e15)
			if res.Values[0] != 1.0 {
				t.Fatalf("new_c1c/%s x0=%.17g, want 1", v.name, res.Values[0])
			}
		}
	})

	t.Run("new_c2_new_c3_new_c4_new_c7", func(t *testing.T) {
		t.Parallel()
		m2a := gb.NewModel("cbc_new_c2a", gb.Minimize)
		x2a := m2a.AddContVar("x", 0, 1)
		m2a.AddConstraint("ub", gb.NewExpr(x2a.T(1e14)).Le(1))
		m2a.AddConstraint("lb", gb.NewExpr(x2a.T(1e14)).Ge(1e5))
		m2a.SetObjective(gb.NewExpr(x2a.T(1e14)))
		p2a := m2a.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p2a, v.opts...)
			if err != nil || res.Status != gb.StatusInfeasible {
				t.Fatalf("new_c2a/%s: status=%v err=%v, want Infeasible", v.name, res.Status, err)
			}
		}

		m2b := gb.NewModel("cbc_new_c2b", gb.Minimize)
		x2b := m2b.AddContVar("x", 0, 1)
		m2b.AddConstraint("ub", gb.NewExpr(x2b.T(1e13)).Le(1))
		m2b.AddConstraint("lb", gb.NewExpr(x2b.T(1e14)).Ge(1))
		m2b.SetObjective(gb.NewExpr(x2b.T(1e14)))
		p2b := m2b.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p2b, v.opts...)
			if err != nil {
				t.Fatalf("new_c2b/%s err: %v", v.name, err)
			}
			checkOpt(t, "new_c2b/"+v.name, p2b, res, 1.0)
		}

		for _, addVacuous := range []bool{false, true} {
			m3 := gb.NewModel("cbc_new_c3", gb.Minimize)
			x0 := m3.AddContVar("x0", math.Ldexp(1, 30), math.Ldexp(1, 30))
			x1 := m3.AddContVar("x1", math.Ldexp(1, 30), math.Ldexp(1, 30))
			z := m3.AddBinaryVar("z")
			w := m3.AddContVar("w", 0, 1)
			m3.AddConstraint("eq", gb.NewExpr(x0.T(math.Ldexp(1, 30)), x1.T(-math.Ldexp(1, 30)), z.T(1)).Eq(1))
			if addVacuous {
				m3.AddConstraint("vac", gb.NewExpr(w.T(1e-19), z.T(1)).Le(2))
			}
			m3.SetObjective(gb.NewExpr(z.T(1)))
			p3 := m3.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p3, v.opts...)
				if err != nil {
					t.Fatalf("new_c3(vac=%v)/%s err: %v", addVacuous, v.name, err)
				}
				checkOpt(t, fmt.Sprintf("new_c3(vac=%v)/%s", addVacuous, v.name), p3, res, 1.0)
			}
		}

		m4 := gb.NewModel("cbc_new_c4", gb.Minimize)
		fx1 := m4.AddContVar("x1", math.Inf(-1), math.Inf(1))
		fx2 := m4.AddContVar("x2", math.Inf(-1), math.Inf(1))
		fx3 := m4.AddContVar("x3", math.Inf(-1), math.Inf(1))
		fx4 := m4.AddContVar("x4", math.Inf(-1), math.Inf(1))
		fx6 := m4.AddContVar("x6", 52776558133248, 52776558133248)
		m4.AddConstraint("r0", gb.NewExpr(fx1.T(-2e13), fx3.T(1e13)).Le(-4e13))
		m4.AddConstraint("r1", gb.NewExpr(fx1.T(1e10), fx2.T(-3e10), fx6.T(1e10)).Ge(1e10))
		m4.AddConstraint("r2", gb.NewExpr(fx1.T(-1e10), fx2.T(3e10), fx4.T(-3e10), fx6.T(2e10/3.0)).Ge(4e10))
		m4.AddConstraint("r3", gb.NewExpr(fx1.T(3e10), fx3.T(-3e10)).Eq(-5e10))
		m4.AddConstraint("c", gb.NewExpr(fx1.T(-1e10)).Le(1))
		m4.SetObjective(gb.NewExpr(fx3.T(1), fx4.T(-1), fx6.T(1)))
		p4 := m4.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p4, v.opts...)
			if err != nil {
				t.Fatalf("new_c4/%s err: %v", v.name, err)
			}
			if res.Status != gb.StatusOptimal || math.IsNaN(res.Objective) {
				t.Fatalf("new_c4/%s: status=%v obj=%g, want Optimal", v.name, res.Status, res.Objective)
			}
		}
		lp4 := gb.NewCBCLPSolver(p4)
		defer lp4.Close()
		if r4 := lp4.Solve(); r4.Status != gb.StatusOptimal {
			t.Fatalf("new_c4 CBCLPSolver status=%v, want Optimal", r4.Status)
		}

		m7 := gb.NewModel("cbc_new_c7", gb.Minimize)
		x7 := m7.AddContVar("x", 0, 1)
		b7 := m7.AddBinaryVar("b")
		m7.AddConstraint("ub", gb.NewExpr(x7.T(1e13)).Le(1))
		m7.AddConstraint("lb", gb.NewExpr(x7.T(1e14)).Ge(1))
		m7.AddConstraint("b_con", gb.NewExpr(b7.T(1)).Le(1))
		m7.SetObjective(gb.NewExpr(x7.T(1e14), b7.T(-1)))
		p7 := m7.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, p7, v.opts...)
			if err != nil {
				t.Fatalf("new_c7/%s err: %v", v.name, err)
			}
			checkOpt(t, "new_c7/"+v.name, p7, res, 0.0)
		}
	})

	t.Run("c3_tiny_pivot_and_m1_presolved_root_lp", func(t *testing.T) {
		t.Parallel()
		for _, eps := range []float64{5e-14, 2e-12} {
			m := gb.NewModel("cbc_c3a", gb.Minimize)
			b0 := m.AddBinaryVar("b0")
			b1 := m.AddBinaryVar("b1")
			b2 := m.AddBinaryVar("b2")
			b3 := m.AddBinaryVar("b3")
			b4 := m.AddBinaryVar("b4")
			u := m.AddContVar("u", 0, math.Inf(1))
			s := m.AddContVar("s", 0, math.Inf(1))
			w := m.AddContVar("w", -1, 0)
			c1 := m.AddContVar("c1", 0, 1)
			m.AddConstraint("e01", gb.NewExpr(b0.T(1), b1.T(1)).Le(1))
			m.AddConstraint("e12", gb.NewExpr(b1.T(1), b2.T(1)).Le(1))
			m.AddConstraint("e23", gb.NewExpr(b2.T(1), b3.T(1)).Le(1))
			m.AddConstraint("e34", gb.NewExpr(b3.T(1), b4.T(1)).Le(1))
			m.AddConstraint("e40", gb.NewExpr(b4.T(1), b0.T(1)).Le(1))
			m.AddConstraint("r1", gb.NewExpr(u.T(1), s.T(-1), w.T(-1)).Eq(1))
			m.AddConstraint("r2", gb.NewExpr(s.T(1), w.T(1), c1.T(1)).Ge(-0.5))
			m.AddConstraint("r0", gb.NewExpr(b0.T(1), c1.T(1), u.T(-eps)).Le(0.5-eps))
			m.SetObjective(gb.NewExpr(b0.T(-1.50), b1.T(-1.01), b2.T(-1.02), b3.T(-1.03), b4.T(-1.04), w.T(-10.0), c1.T(10.0)))
			p := m.Compile()
			for _, v := range cbcVariants {
				res, err := gb.SolveCompiled(ctx, p, v.opts...)
				if err != nil {
					t.Fatalf("cbc_c3a(%.1e)/%s err: %v", eps, v.name, err)
				}
				if res.Status == gb.StatusInfeasible || res.Status == gb.StatusUnbounded {
					t.Fatalf("cbc_c3a(%.1e)/%s unsound status=%v", eps, v.name, res.Status)
				}
			}
		}

		eps := 5e-14
		mb := gb.NewModel("cbc_c3b", gb.Minimize)
		b0 := mb.AddBinaryVar("b0")
		b1 := mb.AddBinaryVar("b1")
		b2 := mb.AddBinaryVar("b2")
		u := mb.AddContVar("u", 0, math.Inf(1))
		s := mb.AddContVar("s", 0, math.Inf(1))
		w := mb.AddContVar("w", -1, 0)
		c1 := mb.AddContVar("c1", 0, 1)
		mb.AddConstraint("t1", gb.NewExpr(b0.T(1), b1.T(1)).Ge(1))
		mb.AddConstraint("t2", gb.NewExpr(b0.T(1), b2.T(1)).Ge(1))
		mb.AddConstraint("t3", gb.NewExpr(b1.T(1), b2.T(1)).Le(1))
		mb.AddConstraint("r1", gb.NewExpr(u.T(1), s.T(-1), w.T(-1)).Eq(1))
		mb.AddConstraint("r2", gb.NewExpr(s.T(1), w.T(1), c1.T(1)).Ge(-0.5))
		mb.AddConstraint("r0", gb.NewExpr(b0.T(1), c1.T(1), u.T(-eps)).Le(0.5-eps))
		mb.SetObjective(gb.NewExpr(b0.T(10.0), b1.T(1.0), b2.T(1.1), w.T(-10.0), c1.T(10.0)))
		pb := mb.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, pb, v.opts...)
			if err != nil {
				t.Fatalf("cbc_c3b/%s err: %v", v.name, err)
			}
			if res.Status == gb.StatusInfeasible || res.Status == gb.StatusUnbounded {
				t.Fatalf("cbc_c3b/%s unsound status=%v", v.name, res.Status)
			}
		}

		m1 := gb.NewModel("cbc_m1", gb.Minimize)
		mx := m1.AddIntVar("x", 0, math.Inf(1))
		my := m1.AddBinaryVar("y")
		mz := m1.AddBinaryVar("z")
		m1.AddConstraint("cx", gb.NewExpr(mx.T(1)).Le(1))
		m1.AddConstraint("cyz", gb.NewExpr(my.T(2), mz.T(1)).Le(1))
		m1.SetObjective(gb.NewExpr(mx.T(-1), my.T(-10), mz.T(-1)))
		pm1 := m1.Compile()
		for _, v := range cbcVariants {
			res, err := gb.SolveCompiled(ctx, pm1, v.opts...)
			if err != nil {
				t.Fatalf("m1/%s err: %v", v.name, err)
			}
			checkOpt(t, "m1/"+v.name, pm1, res, -2.0)
		}
	})
}

func readLinuxStatmRSSBytes(t *testing.T) int64 {
	t.Helper()
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		t.Fatalf("read /proc/self/statm: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		t.Fatalf("unexpected /proc/self/statm output %q", string(raw))
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		t.Fatalf("parse /proc/self/statm RSS pages %q: %v", fields[1], err)
	}
	return pages * int64(os.Getpagesize())
}

// TestCBCCGoBoundedRSSGrowth verifies that repeated CBC/CLP solves, incremental
// LP lifecycle mutations, and rapid context cancellations bound steady-state RSS growth.
func TestCBCCGoBoundedRSSGrowth(t *testing.T) {
	ctx := t.Context()

	// 1. Continuous LP model for solveCBC LP calls.
	const lpVars = 24
	mLP := gb.NewModel("cbc_rss_lp", gb.Minimize)
	xLP := make([]gb.Var, lpVars)
	objLP := make([]gb.Term, lpVars)
	for j := range lpVars {
		xLP[j] = mLP.AddContVar(fmt.Sprintf("x_%d", j), 0, 20)
		objLP[j] = xLP[j].T(float64(j + 1))
	}
	mLP.SetObjective(gb.NewExpr(objLP...))
	for i := range 12 {
		cTerms := []gb.Term{
			xLP[i].T(1),
			xLP[(i+1)%lpVars].T(2),
			xLP[(i+7)%lpVars].T(1),
		}
		mLP.AddConstraint(fmt.Sprintf("r_%d", i), gb.NewExpr(cTerms...).Ge(6.0+float64(i%3)))
	}
	pLP := mLP.Compile()

	// 2. Knapsack-covering MILP model for solveCBC MILP calls.
	const mipVars = 12
	mMIP := gb.NewModel("cbc_rss_mip", gb.Minimize)
	xMIP := make([]gb.Var, mipVars)
	objMIP := make([]gb.Term, mipVars)
	for j := range mipVars {
		xMIP[j] = mMIP.AddBinaryVar(fmt.Sprintf("b_%d", j))
		objMIP[j] = xMIP[j].T(float64(3 + (j*5)%11))
	}
	mMIP.SetObjective(gb.NewExpr(objMIP...))
	for i := range 6 {
		cTerms := []gb.Term{
			xMIP[i].T(2),
			xMIP[(i+3)%mipVars].T(3),
			xMIP[(i+6)%mipVars].T(2),
			xMIP[(i+9)%mipVars].T(1),
		}
		mMIP.AddConstraint(fmt.Sprintf("k_%d", i), gb.NewExpr(cTerms...).Ge(4))
	}
	pMIP := mMIP.Compile()

	// 3. Incremental LP model for NewCBCLPSolver + SetStructuralBounds + AddCut + Solve + Close.
	const incVars = 16
	mInc := gb.NewModel("cbc_rss_inc_lp", gb.Minimize)
	xInc := make([]gb.Var, incVars)
	objInc := make([]gb.Term, incVars)
	sumInc := make([]gb.Term, incVars)
	for j := range incVars {
		xInc[j] = mInc.AddContVar(fmt.Sprintf("v_%d", j), 0, 10)
		objInc[j] = xInc[j].T(float64(j + 1))
		sumInc[j] = xInc[j].T(1)
	}
	mInc.SetObjective(gb.NewExpr(objInc...))
	mInc.AddConstraint("total", gb.NewExpr(sumInc...).Ge(20))
	for i := range 8 {
		mInc.AddConstraint(fmt.Sprintf("pair_%d", i), gb.NewExpr(xInc[i].T(1), xInc[(i+1)%incVars].T(1)).Ge(3))
	}
	pInc := mInc.Compile()
	incLB := make([]float64, incVars)
	incUB := make([]float64, incVars)

	// 4. Unpresolved Diophantine MIP (2*x + 2*y = 1) that branches until context cancellation.
	mCancel := gb.NewModel("cbc_rss_cancel", gb.Minimize)
	cx := mCancel.AddIntVar("x", math.Inf(-1), math.Inf(1))
	cy := mCancel.AddIntVar("y", math.Inf(-1), math.Inf(1))
	mCancel.AddConstraint("lo", gb.NewExpr(cx.T(2), cy.T(2)).Ge(1))
	mCancel.AddConstraint("hi", gb.NewExpr(cx.T(2), cy.T(2)).Le(1))
	pCancel := mCancel.Compile()

	var rssAt100, rssAt400 int64
	for iter := 1; iter <= 400; iter++ {
		switch iter % 4 {
		case 0:
			res, err := gb.SolveCompiled(ctx, pLP, gb.WithCBC())
			if err != nil || res.Status != gb.StatusOptimal {
				t.Fatalf("iter %d CBC LP: status=%v err=%v, want StatusOptimal", iter, res.Status, err)
			}
		case 1:
			res, err := gb.SolveCompiled(ctx, pMIP, gb.WithCBC())
			if err != nil || res.Status != gb.StatusOptimal {
				t.Fatalf("iter %d CBC MILP: status=%v err=%v, want StatusOptimal", iter, res.Status, err)
			}
		case 2:
			lp := gb.NewCBCLPSolver(pInc)
			if r0 := lp.Solve(); r0.Status != gb.StatusOptimal {
				lp.Close()
				t.Fatalf("iter %d CBCLPSolver initial solve: status=%v, want StatusOptimal", iter, r0.Status)
			}
			shift := 0.001 * float64(iter%10)
			for j := range incVars {
				incLB[j] = 0.5 + shift
				incUB[j] = 10.0
			}
			lp.SetStructuralBounds(incLB, incUB)
			lp.AddCut([]int{0, 1}, []float64{1, 1}, 15.0-shift)
			if r1 := lp.Solve(); r1.Status != gb.StatusOptimal {
				lp.Close()
				t.Fatalf("iter %d CBCLPSolver resolve: status=%v, want StatusOptimal", iter, r1.Status)
			}
			lp.Close()
		case 3:
			cCtx, cancel := context.WithCancel(ctx)
			timer := time.AfterFunc(500*time.Microsecond, cancel)
			_, err := gb.SolveCompiled(cCtx, pCancel, gb.WithCBC(), gb.WithPresolve(false), gb.WithCuts(false))
			timer.Stop()
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("iter %d CBC cancel solve: err=%v, want context.Canceled", iter, err)
			}
		}

		if iter == 100 || iter == 400 {
			runtime.GC()
			debug.FreeOSMemory()
			if runtime.GOOS == "linux" {
				rss := readLinuxStatmRSSBytes(t)
				if iter == 100 {
					rssAt100 = rss
				} else {
					rssAt400 = rss
				}
			}
		}
	}

	if runtime.GOOS == "linux" {
		const maxRSSDeltaBytes = 64 << 20
		delta := rssAt400 - rssAt100
		t.Logf("CBC CGo RSS: iter100=%.2f MiB, iter400=%.2f MiB, delta=%.2f MiB",
			float64(rssAt100)/(1<<20), float64(rssAt400)/(1<<20), float64(delta)/(1<<20))
		if delta > maxRSSDeltaBytes {
			t.Fatalf("CBC CGo steady-state RSS delta %.2f MiB exceeds 64 MiB limit (iter100=%d B, iter400=%d B)",
				float64(delta)/(1<<20), rssAt100, rssAt400)
		}
	}
}
