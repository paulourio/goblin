//go:build cgo && cbc

package goblin_test

import (
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
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
	mSmall := gb.NewModel("cbc_clamp", gb.Minimize)
	v := mSmall.AddContVar("v", 1, 5)
	mSmall.SetObjective(gb.VarExpr(v))
	mSmall.AddConstraint("c", gb.VarExpr(v).Ge(2))
	resClamp, err := mSmall.Solve(ctx, gb.WithCBC(), gb.WithMaxIter(math.MaxInt32+100))
	if err != nil || resClamp.Status != gb.StatusOptimal || math.Abs(resClamp.Objective-2.0) > 1e-6 {
		t.Fatalf("CBC MaxIter > MaxInt32: status=%v obj=%v err=%v", resClamp.Status, resClamp.Objective, err)
	}
}
