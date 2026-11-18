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
