package goblin

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
			for j := range p.numVars {
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
		if len(s.extraRows) != 0 {
			t.Fatalf("seed %d: restore left %d extra rows", seed, len(s.extraRows))
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

// TestSparseMarkowitzLUAndHyperSparseSolves verifies that FactorizeSparse with a non-empty
// Markowitz nucleus (Stage 3), FTRANSparse, BTRANSparse, and PushEtaSparse match dense FTRAN
// and BTRAN solves and satisfy B*x = b and B^T*y = c within 1e-11.
func TestSparseMarkowitzLUAndHyperSparseSolves(t *testing.T) {
	const m = 48
	r := rand.New(rand.NewPCG(42, 7))
	colPtr := make([]int, m+1)
	var rowIdx []int
	var colVal []float64

	// Construct a nonsingular basis without column or row singletons so the full m x m
	// matrix enters Stage 3 (sparse Markowitz nucleus).
	for j := range m {
		colPtr[j] = len(rowIdx)
		used := map[int]bool{
			j:           true,
			(j + 1) % m: true,
			(j + 7) % m: true,
		}
		for range 2 {
			used[r.IntN(m)] = true
		}
		rows := make([]int, 0, len(used))
		for i := range used {
			rows = append(rows, i)
		}
		for i := range rows {
			for k := i + 1; k < len(rows); k++ {
				if rows[k] < rows[i] {
					rows[i], rows[k] = rows[k], rows[i]
				}
			}
		}
		for _, i := range rows {
			rowIdx = append(rowIdx, i)
			if i == j {
				colVal = append(colVal, 4.0+r.Float64()*2.0)
			} else {
				colVal = append(colVal, (r.Float64()-0.5)*1.5)
			}
		}
	}
	colPtr[m] = len(rowIdx)

	lu := &LUFactorization{}
	err := lu.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		for k := colPtr[col]; k < colPtr[col+1]; k++ {
			rows = append(rows, rowIdx[k])
			vals = append(vals, colVal[k])
		}
		return rows, vals
	})
	if err != nil {
		t.Fatalf("FactorizeSparse failed: %v", err)
	}
	if lu.nucStart != 0 {
		t.Fatalf("expected full Markowitz nucleus (nucStart=0), got nucStart=%d", lu.nucStart)
	}

	// Verify FTRANSparse, FTRANSparseHint, BTRANSparse, and BTRANSparseHint match dense
	// FTRAN and BTRAN on sparse RHS vectors before and after PushEtaSparse updates.
	for step := range 4 {
		bDense := make([]float64, m)
		bSparse := make([]float64, m)
		bHint := make([]float64, m)
		var bInIdx []int
		seenIn := make([]bool, m)
		for range 3 {
			idx := r.IntN(m)
			val := (r.Float64() - 0.5) * 5.0
			bDense[idx] = val
			bSparse[idx] = val
			bHint[idx] = val
			if !seenIn[idx] {
				seenIn[idx] = true
				bInIdx = append(bInIdx, idx)
			}
		}
		lu.FTRAN(bDense)
		fIdx := lu.FTRANSparse(bSparse, nil)
		fHintIdx := lu.FTRANSparseHint(bHint, bInIdx, nil)
		for i := range m {
			if math.Abs(bDense[i]-bSparse[i]) > 1e-12 {
				t.Fatalf("step %d: FTRANSparse[%d]=%.15g != FTRAN=%.15g", step, i, bSparse[i], bDense[i])
			}
			if math.Abs(bDense[i]-bHint[i]) > 1e-12 {
				t.Fatalf("step %d: FTRANSparseHint[%d]=%.15g != FTRAN=%.15g", step, i, bHint[i], bDense[i])
			}
		}
		inFIdx := make([]bool, m)
		for _, idx := range fIdx {
			inFIdx[idx] = true
		}
		inFHint := make([]bool, m)
		for _, idx := range fHintIdx {
			inFHint[idx] = true
		}
		for i, v := range bSparse {
			if math.Abs(v) > 1e-13 && !inFIdx[i] {
				t.Fatalf("step %d: nonzero FTRANSparse[%d]=%.3e missing from fIdx", step, i, v)
			}
			if math.Abs(bHint[i]) > 1e-13 && !inFHint[i] {
				t.Fatalf("step %d: nonzero FTRANSparseHint[%d]=%.3e missing from fHintIdx", step, i, bHint[i])
			}
		}

		cDense := make([]float64, m)
		cSparse := make([]float64, m)
		cHint := make([]float64, m)
		pivRow := r.IntN(m)
		cDense[pivRow] = 1.0
		cSparse[pivRow] = 1.0
		cHint[pivRow] = 1.0
		lu.BTRAN(cDense)
		rIdx := lu.BTRANSparse(cSparse, nil)
		rHintIdx := lu.BTRANSparseHint(cHint, []int{pivRow}, nil)
		for i := range m {
			if math.Abs(cDense[i]-cSparse[i]) > 1e-12 {
				t.Fatalf("step %d: BTRANSparse[%d]=%.15g != BTRAN=%.15g", step, i, cSparse[i], cDense[i])
			}
			if math.Abs(cDense[i]-cHint[i]) > 1e-12 {
				t.Fatalf("step %d: BTRANSparseHint[%d]=%.15g != BTRAN=%.15g", step, i, cHint[i], cDense[i])
			}
		}
		inRIdx := make([]bool, m)
		for _, idx := range rIdx {
			inRIdx[idx] = true
		}
		inRHint := make([]bool, m)
		for _, idx := range rHintIdx {
			inRHint[idx] = true
		}
		for i, v := range cSparse {
			if math.Abs(v) > 1e-13 && !inRIdx[i] {
				t.Fatalf("step %d: nonzero BTRANSparse[%d]=%.3e missing from rIdx", step, i, v)
			}
			if math.Abs(cHint[i]) > 1e-12 && !inRHint[i] {
				t.Fatalf("step %d: nonzero BTRANSparseHint[%d]=%.3e missing from rHintIdx", step, i, cHint[i])
			}
		}

		// Apply a PushEtaSparse update if pivot is sufficiently large.
		fSparseCopy := make([]float64, m)
		copy(fSparseCopy, bSparse)
		fSparseCopy[pivRow] = 2.5
		fIdx = append(fIdx, pivRow)
		if err := lu.PushEtaSparse(pivRow, fSparseCopy, fIdx); err != nil {
			t.Fatalf("step %d: PushEtaSparse failed: %v", step, err)
		}
	}

	// Verify empty-input and dense-fallback paths of FTRANSparseHint and BTRANSparseHint.
	zeroVec := make([]float64, m)
	if got := lu.FTRANSparseHint(zeroVec, nil, nil); len(got) != 0 {
		t.Fatalf("empty FTRANSparseHint returned %d indices, want 0", len(got))
	}
	if got := lu.BTRANSparseHint(zeroVec, nil, nil); len(got) != 0 {
		t.Fatalf("empty BTRANSparseHint returned %d indices, want 0", len(got))
	}
	denseRef := make([]float64, m)
	denseHint := make([]float64, m)
	denseIn := make([]int, 0, m/2)
	for i := range m / 2 {
		denseRef[i] = float64(i + 1)
		denseHint[i] = float64(i + 1)
		denseIn = append(denseIn, i)
	}
	lu.FTRAN(denseRef)
	lu.FTRANSparseHint(denseHint, denseIn, nil)
	for i := range m {
		if math.Abs(denseRef[i]-denseHint[i]) > 1e-11 {
			t.Fatalf("dense-fallback FTRANSparseHint[%d]=%.15g != FTRAN=%.15g", i, denseHint[i], denseRef[i])
		}
	}
}

func TestLUSingletonDropThresholdAndInvalidGuard(t *testing.T) {
	lu := newLUFactorization(8)
	if !lu.NeedsRefactor() {
		t.Fatal("fresh unfactorized LUFactorization must report NeedsRefactor() == true")
	}

	// Basis with sub-luDropTol (1e-14) entry in column 0 that must not decrement row 1's degree.
	cols := [4]struct {
		rows []int
		vals []float64
	}{
		{rows: []int{0, 1, 3}, vals: []float64{1, 1e-14, 1}},
		{rows: []int{1, 2, 3}, vals: []float64{1, 1, 1}},
		{rows: []int{1, 2}, vals: []float64{1, 2}},
		{rows: []int{2, 3}, vals: []float64{1, 1}},
	}
	if err := lu.FactorizeSparse(4, func(col int, rows []int, vals []float64) ([]int, []float64) {
		rows = append(rows, cols[col].rows...)
		vals = append(vals, cols[col].vals...)
		return rows, vals
	}); err != nil {
		t.Fatalf("FactorizeSparse failed: %v", err)
	}
	if lu.NeedsRefactor() {
		t.Fatal("valid LUFactorization with zero etas must report NeedsRefactor() == false")
	}

	wantX := [4]float64{1, 2, 3, 4}
	var rhs [4]float64
	for c := range 4 {
		for k, r := range cols[c].rows {
			rhs[r] += cols[c].vals[k] * wantX[c]
		}
	}
	sol := rhs[:]
	lu.FTRAN(sol)
	for i, w := range wantX {
		if math.Abs(sol[i]-w) > 1e-11 {
			t.Fatalf("FTRAN[%d] = %.15g, want %.15g", i, sol[i], w)
		}
	}

	lu.Reset(4)
	if !lu.NeedsRefactor() {
		t.Fatal("Reset LUFactorization must report NeedsRefactor() == true")
	}
}
