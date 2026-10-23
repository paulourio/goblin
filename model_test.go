package goblin_test

import (
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
)

func TestModelAndCompile(t *testing.T) {
	m := gb.NewModel("canonical_mip", gb.Minimize)
	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddIntVar("z", 0, math.Inf(1))
	w := m.AddBinaryVar("w")

	// Objective: x + 4y + 9z + 2w + 5
	m.SetObjective(gb.NewExpr(
		x.T(1),
		y.T(4),
		gb.T(z, 9),
		w.T(2),
	).AddConstant(5))

	// Constraints:
	// c1: x + y <= 5
	// c2: x + z >= 10
	// c3: -y + z + w == 7
	m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(1)).Le(5))
	m.AddConstraint("c2", gb.NewExpr(x.T(1), z.T(1)).Ge(10))
	m.AddConstraint("c3", gb.NewExpr(y.T(-1), z.T(1), w.T(1)).Eq(7))

	if !m.IsMIP() {
		t.Fatalf("expected IsMIP() == true")
	}

	p := m.Compile()
	if p.NumVars != 4 || p.NumConstraints != 3 {
		t.Fatalf("unexpected dimensions: vars=%d, constraints=%d", p.NumVars, p.NumConstraints)
	}
	if len(p.IntCols) != 2 || p.IntCols[0] != 2 || p.IntCols[1] != 3 {
		t.Fatalf("unexpected IntCols: %v", p.IntCols)
	}
	if (p.BinaryMask[0] & (1 << 3)) == 0 {
		t.Fatalf("expected variable 3 (w) to have bit set in BinaryMask")
	}
	if p.RowStride != 8 || len(p.DenseRows) != 3*8 {
		t.Fatalf("expected RowStride=8 and DenseRows len=24, got stride=%d len=%d", p.RowStride, len(p.DenseRows))
	}

	// Verify CSR and CSC consistency
	if len(p.ColIdx) != 7 {
		t.Fatalf("expected 7 non-zeros, got %d", len(p.ColIdx))
	}

	// Evaluate candidate solution x=4, y=-1, z=6, w=0 => obj = 4 - 4 + 54 + 0 + 5 = 59
	sol := []float64{4, -1, 6, 0}
	obj := p.EvaluateObjective(sol)
	if math.Abs(obj-59.0) > 1e-12 {
		t.Fatalf("expected objective 59.0, got %v", obj)
	}
	bViol, rViol, iViol := p.MaxViolation(sol, true)
	if bViol > 1e-12 || rViol > 1e-12 || iViol > 1e-12 {
		t.Fatalf("expected zero violations for feasible integer point, got bound=%v row=%v int=%v", bViol, rViol, iViol)
	}
}
