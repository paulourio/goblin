//go:build cgo && cbc

package goblin_test

import (
	"context"
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

	res, err := m.Solve(context.Background(), gb.WithCBC())
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
}
