package gopt

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

func randomBoxedLP(seed uint64, m, n int) *Model {
	r := rand.New(rand.NewPCG(seed, 11))
	mdl := NewModel(fmt.Sprintf("lp_%d", seed), Minimize)
	xs := make([]Var, n)
	obj := NewExpr()
	for j := range xs {
		xs[j] = mdl.AddContVar(fmt.Sprintf("x%d", j), 0, 10)
		obj = obj.AddTerm(xs[j], 1+r.Float64()*9)
	}
	mdl.SetObjective(obj)
	for i := range m {
		e := NewExpr()
		for j := range xs {
			if r.Float64() < 0.3 {
				e = e.AddTerm(xs[j], 0.5+r.Float64()*4)
			}
		}
		if len(e.Terms) > 0 {
			mdl.AddConstraint(fmt.Sprintf("r%d", i), Ge(e, 5+r.Float64()*20))
		}
	}
	return mdl
}

// TestAddCutRowsMatchesCompiledRows verifies that a batch of rows appended to a warm-started
// LPSolver (with preserved steepest-edge weights) yields the same optimum as compiling the rows
// into the model, and that restoring a snapshot recovers the original optimum.
func TestAddCutRowsMatchesCompiledRows(t *testing.T) {
	for seed := range uint64(8) {
		mdl := randomBoxedLP(seed, 12, 20)
		p := mdl.Compile()
		s := NewLPSolver(p, nil)
		base := s.Solve()
		if base.Status != StatusOptimal {
			t.Fatalf("seed %d: base LP %v", seed, base.Status)
		}

		r := rand.New(rand.NewPCG(seed, 99))
		type row struct {
			cols []int
			vals []float64
			rhs  float64
		}
		var cuts []row
		for range 4 {
			var c row
			act := 0.0
			for j := range p.NumVars {
				if r.Float64() < 0.4 {
					v := 0.5 + r.Float64()
					c.cols = append(c.cols, j)
					c.vals = append(c.vals, v)
					act += v * base.Primal[j]
				}
			}
			c.rhs = act * 0.9 // cut off the current optimum
			cuts = append(cuts, c)
		}

		snap := s.snapshot()
		rows := make([]dynamicRow, len(cuts))
		for k, c := range cuts {
			rows[k] = s.newCutRow(c.cols, c.vals, c.rhs)
		}
		s.addCutRows(rows)
		withCuts := s.Solve()

		ref := randomBoxedLP(seed, 12, 20)
		for k, c := range cuts {
			e := NewExpr()
			for i, col := range c.cols {
				e = e.AddTerm(makeVar(ref.id, col), c.vals[i])
			}
			ref.AddConstraint(fmt.Sprintf("cut%d", k), Le(e, c.rhs))
		}
		refRes := NewLPSolver(ref.Compile(), nil).Solve()
		if withCuts.Status != refRes.Status {
			t.Fatalf("seed %d: warm cut status %v, compiled %v", seed, withCuts.Status, refRes.Status)
		}
		if refRes.Status == StatusOptimal && math.Abs(withCuts.ObjectiveMin-refRes.ObjectiveMin) > 1e-7*max(1, math.Abs(refRes.ObjectiveMin)) {
			t.Fatalf("seed %d: warm cut objective %.9f, compiled %.9f", seed, withCuts.ObjectiveMin, refRes.ObjectiveMin)
		}

		s.restore(snap)
		if len(s.ExtraRows) != 0 {
			t.Fatalf("seed %d: restore left %d extra rows", seed, len(s.ExtraRows))
		}
		again := s.Solve()
		if again.Status != StatusOptimal || math.Abs(again.ObjectiveMin-base.ObjectiveMin) > 1e-7*max(1, math.Abs(base.ObjectiveMin)) {
			t.Fatalf("seed %d: after restore %v %.9f, want %.9f", seed, again.Status, again.ObjectiveMin, base.ObjectiveMin)
		}
		if again.Iterations != 0 {
			t.Fatalf("seed %d: restored optimal basis needed %d iterations", seed, again.Iterations)
		}
	}
}

// TestFinalizeCutCompensatesDroppedCoefficients: dropping a tiny coefficient must relax the
// right-hand side by the largest possible value of the dropped term, and cuts whose dropped
// term is unbounded must be rejected.
func TestFinalizeCutCompensatesDroppedCoefficients(t *testing.T) {
	m := NewModel("fc", Minimize)
	m.AddContVar("a", 0, 5)
	m.AddContVar("b", -1e6, 1e6)
	m.AddContVar("c", 0, math.Inf(1))
	p := m.Compile()
	lp := []float64{5, 0, 0}

	// 1*a - 1e-12*b <= 1 : dropping -1e-12*b requires rhs += 1e-12*1e6.
	cut, ok := finalizeCut(p, []float64{1, -1e-12, 0}, 1, lp)
	if !ok {
		t.Fatal("cut rejected")
	}
	if len(cut.ColIdx) != 1 || cut.ColIdx[0] != 0 {
		t.Fatalf("unexpected support %v", cut.ColIdx)
	}
	want := (1 + 1e-12*1e6) + rhsSafetyMargin // norm of the kept part is 1
	if math.Abs(cut.RHS-want) > 1e-15 {
		t.Fatalf("rhs=%.17g, want %.17g", cut.RHS, want)
	}

	// A dropped term on an unbounded variable cannot be compensated.
	if _, ok := finalizeCut(p, []float64{1, 0, -1e-12}, 1, lp); ok {
		t.Fatal("cut with an uncompensable dropped term was accepted")
	}
}
