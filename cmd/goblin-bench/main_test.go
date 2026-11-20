package main

import (
	"math"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

func TestDurationStatsAndBackendParsing(t *testing.T) {
	if got := medianDurationMS(nil); got != 0 {
		t.Fatalf("medianDurationMS(nil) = %v, want 0", got)
	}
	samples := []time.Duration{
		5 * time.Millisecond,
		1 * time.Millisecond,
		3 * time.Millisecond,
	}
	if got := medianDurationMS(samples); math.Abs(got-3.0) > 1e-9 {
		t.Fatalf("medianDurationMS = %v, want 3.0", got)
	}
	mean, sd := meanSDDurationMS(samples)
	if math.Abs(mean-3.0) > 1e-9 || math.Abs(sd-2.0) > 1e-9 {
		t.Fatalf("meanSDDurationMS = (%v, %v), want (3.0, 2.0)", mean, sd)
	}

	if b, err := parseBackend("go"); err != nil || b != gb.BackendGo {
		t.Fatalf("parseBackend(go) = (%v, %v), want BackendGo", b, err)
	}
	if b, err := parseBackend("cbc"); err != nil || b != gb.BackendCBC {
		t.Fatalf("parseBackend(cbc) = (%v, %v), want BackendCBC", b, err)
	}
	if _, err := parseBackend("invalid"); err == nil {
		t.Fatal("expected error from parseBackend(invalid)")
	}
}

func TestMeasureRootLPRelaxationVerifiesStatus(t *testing.T) {
	m := gb.NewModel("bench_root_lp", gb.Minimize)
	x := m.AddIntVar("x", 0, 10)
	y := m.AddIntVar("y", 0, 10)
	m.SetObjective(gb.NewExpr(x.T(2), y.T(3)))
	m.AddConstraint("c", gb.NewExpr(x.T(1), y.T(1)).Ge(2.5))
	p := m.Compile()

	times, lpRes := measureRootLPRelaxation(p, []*gb.CompiledProblem{p, p}, 1, 2, gb.BackendGo, true)
	if len(times) != 2 {
		t.Fatalf("len(times) = %d, want 2", len(times))
	}
	if lpRes.Status != gb.StatusOptimal || math.Abs(lpRes.Objective-5.0) > 1e-9 {
		t.Fatalf("measureRootLPRelaxation = (%v, %v), want Optimal 5.0", lpRes.Status, lpRes.Objective)
	}
}
