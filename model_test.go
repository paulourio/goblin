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
	if p.NumVars() != 4 || p.NumConstraints() != 3 {
		t.Fatalf("unexpected dimensions: vars=%d, constraints=%d", p.NumVars(), p.NumConstraints())
	}
	if p.NumIntVars() != 2 || p.IntCol(0) != 2 || p.IntCol(1) != 3 {
		t.Fatalf("unexpected integer columns: n=%d [%d, %d]", p.NumIntVars(), p.IntCol(0), p.IntCol(1))
	}
	if !p.IsBinaryVar(3) || p.IsBinaryVar(2) {
		t.Fatalf("expected only variable 3 (w) to have bit set in binary mask")
	}
	if p.RowStride() != 8 || p.DenseRowLen() != 3*8 {
		t.Fatalf("expected RowStride=8 and DenseRowLen=24, got stride=%d len=%d", p.RowStride(), p.DenseRowLen())
	}

	// Verify CSR and CSC consistency
	if p.NNZ() != 7 {
		t.Fatalf("expected 7 non-zeros, got %d", p.NNZ())
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

func TestExprSumAndCompiledEvalStickyRounding(t *testing.T) {
	t.Parallel()

	sumCases := []struct {
		name string
		vals []float64
		want float64
	}{
		{"half_plus_tiny_rounds_up", []float64{1, 0x1p-53, 0x1p-110}, 1 + 0x1p-52},
		{"half_minus_tiny_rounds_even", []float64{1, 0x1p-53, -0x1p-110}, 1},
		{"odd_half_minus_tiny_stays_odd", []float64{1 + 0x1p-52, 0x1p-53, -0x1p-110}, 1 + 0x1p-52},
		{"half_plus_min_subnormal_rounds_up", []float64{1, 0x1p-53, 0x1p-1074}, 1 + 0x1p-52},
		{"huge_cancellation_exact", []float64{1e308, 1e308, -1e308}, 1e308},
	}
	for _, tc := range sumCases {
		exprs := make([]gb.Expr, len(tc.vals))
		for i, v := range tc.vals {
			exprs[i] = gb.ConstExpr(v)
		}
		if got := gb.Sum(exprs...).Constant; got != tc.want {
			t.Fatalf("%s: Sum = %.17g (0x%x), want %.17g (0x%x)",
				tc.name, got, math.Float64bits(got), tc.want, math.Float64bits(tc.want))
		}
	}

	if got := gb.Sum(gb.ConstExpr(math.Inf(1)), gb.ConstExpr(math.Inf(-1))).Constant; !math.IsNaN(got) {
		t.Fatalf("Sum(+Inf, -Inf) = %v, want NaN", got)
	}
	if got := gb.Sum(gb.ConstExpr(1), gb.ConstExpr(math.Inf(1))).Constant; !math.IsInf(got, 1) {
		t.Fatalf("Sum(1, +Inf) = %v, want +Inf", got)
	}

	m := gb.NewModel("sticky_eval", gb.Minimize)
	x0 := m.AddContVar("x0", -10, 10)
	x1 := m.AddContVar("x1", -10, 10)
	x2 := m.AddContVar("x2", -10, 10)
	m.SetObjective(gb.NewExpr(x0.T(1), x1.T(0x1p-53), x2.T(0x1p-110)))
	m.AddConstraint("c_up", gb.Le(gb.NewExpr(x0.T(1), x1.T(0x1p-53), x2.T(0x1p-1074)), 1+0x1p-52))
	m.AddConstraint("c_down", gb.Ge(gb.NewExpr(x0.T(1), x1.T(0x1p-53), x2.T(-0x1p-110)), 1))
	p := m.Compile()

	pt := []float64{1, 1, 1}
	wantObj := 1 + 0x1p-52
	if gotObj := p.EvaluateObjective(pt); gotObj != wantObj {
		t.Fatalf("EvaluateObjective tie-break = %.17g (0x%x), want %.17g (0x%x)",
			gotObj, math.Float64bits(gotObj), wantObj, math.Float64bits(wantObj))
	}
	bv, rv, _ := p.MaxViolation(pt, false)
	if bv != 0 || rv != 0 {
		t.Fatalf("MaxViolation sticky tie-break = (bound=%g, row=%g), want (0, 0)", bv, rv)
	}
	if _, rvUp, _ := p.MaxViolation([]float64{1 + 0x1p-52, 1, 1}, false); rvUp != 0x1p-53 {
		t.Fatalf("MaxViolation shifted odd point = %.17g, want 0x1p-53", rvUp)
	}
	if _, rvDown, _ := p.MaxViolation([]float64{1 - 0x1p-53, 1, 1}, false); rvDown != 0x1p-110 {
		t.Fatalf("MaxViolation sub-halfway point = %.17g, want 0x1p-110", rvDown)
	}
}

func TestModelNilReceiverAndEnumString(t *testing.T) {
	t.Parallel()

	var m *gb.Model
	if got := m.Name(); got != "" {
		t.Fatalf("nil.Name() = %q, want empty", got)
	}
	if got := m.Sense(); got != gb.Minimize {
		t.Fatalf("nil.Sense() = %v, want Minimize", got)
	}
	if got := m.NumVars(); got != 0 {
		t.Fatalf("nil.NumVars() = %d, want 0", got)
	}
	if got := m.NumConstraints(); got != 0 {
		t.Fatalf("nil.NumConstraints() = %d, want 0", got)
	}
	if _, ok := m.VarByName("x"); ok {
		t.Fatalf("nil.VarByName() ok = true, want false")
	}
	if got := m.VarName(gb.Var(1)); got != "" {
		t.Fatalf("nil.VarName() = %q, want empty", got)
	}
	if lb, ub := m.VarBounds(gb.Var(1)); lb != 0 || ub != 0 {
		t.Fatalf("nil.VarBounds() = (%v, %v), want (0, 0)", lb, ub)
	}
	if got := m.VarCategoryOf(gb.Var(1)); got != gb.Continuous {
		t.Fatalf("nil.VarCategoryOf() = %v, want Continuous", got)
	}
	if m.IsMIP() {
		t.Fatalf("nil.IsMIP() = true, want false")
	}

	if got := gb.Continuous.String(); got != "Continuous" {
		t.Fatalf("Continuous.String() = %q", got)
	}
	if got := gb.VarCategory(7).String(); got != "VarCategory(7)" {
		t.Fatalf("VarCategory(7).String() = %q, want VarCategory(7)", got)
	}
	if got := gb.Equal.String(); got != "=" {
		t.Fatalf("Equal.String() = %q", got)
	}
	if got := gb.ConstraintSense(7).String(); got != "ConstraintSense(7)" {
		t.Fatalf("ConstraintSense(7).String() = %q, want ConstraintSense(7)", got)
	}
}
