//go:build !cbc

package goblin_test

import (
	"context"
	"testing"

	gb "github.com/paulourio/goblin"
)

func TestCBCDisabledByDefaultWithoutBuildTag(t *testing.T) {
	if gb.CBCAvailable() {
		t.Fatal("CBCAvailable() = true without -tags cbc, want false")
	}

	m := gb.NewModel("default_nocbc", gb.Minimize)
	x := m.AddIntVar("x", 0, 5)
	m.SetObjective(gb.NewExpr(gb.T(x, 1)))
	m.AddConstraint("lb", gb.Ge(gb.NewExpr(gb.T(x, 1)), 2))

	res, err := m.Solve(context.Background(), gb.WithCBC())
	if err != nil {
		t.Fatalf("Solve with WithCBC() without -tags cbc returned error: %v", err)
	}
	if res.Backend != gb.BackendCBC {
		t.Fatalf("res.Backend = %v, want BackendCBC", res.Backend)
	}
	if res.Status != gb.StatusUndefined {
		t.Fatalf("res.Status = %v, want StatusUndefined when built without -tags cbc", res.Status)
	}
}
