package goblin

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
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

// TestRuizScaledCutsCrashRecoveryAndInternalGuards exercises Ruiz-scaled dynamic cuts
// (pow2EquilibrateExp, rowScaleOf, unscaledBasicVal, unscaledTabEntry, snapshot/restore,
// PurgeSlackCuts), pre-equilibration overflow protection, crash basis recovery helpers,
// MIR/finalizeCut non-finite guards, and branch selection invariants.
func TestRuizScaledCutsCrashRecoveryAndInternalGuards(t *testing.T) {
	// 1. Ruiz-scaled LP with dynamic cut insertion, snapshot/restore, and slack cut purging.
	m := NewModel("ruiz_cuts", Minimize)
	x := m.AddContVar("x", 0, 100)
	y := m.AddContVar("y", 0, 100)
	m.SetObjective(NewExpr(x.T(1), y.T(2)))
	m.AddConstraint("r0", NewExpr(x.T(1e-8), y.T(1e-8)).Ge(3e-8))
	m.AddConstraint("r1", NewExpr(x.T(2e4), y.T(1e4)).Ge(4e4))
	p := m.Compile()
	s := NewLPSolver(p, nil)
	defer s.Close()
	if !s.scaled {
		t.Fatal("expected Ruiz scaling to activate on wide-scale LP")
	}
	// PurgeSlackCuts before Solve() must be a safe no-op.
	if purged := s.PurgeSlackCuts(1e-4); purged != 0 {
		t.Fatalf("PurgeSlackCuts before Solve = %d, want 0", purged)
	}
	var nilSolver *LPSolver
	if purged := nilSolver.PurgeSlackCuts(1e-4); purged != 0 {
		t.Fatalf("nil PurgeSlackCuts = %d, want 0", purged)
	}

	r0 := s.Solve()
	if r0.Status != StatusOptimal || math.Abs(r0.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("Ruiz base solve: status=%v obj=%v, want Optimal 3", r0.Status, r0.ObjectiveMin)
	}
	if rs := s.rowScaleOf(0); rs <= 0 || math.IsNaN(rs) || math.IsInf(rs, 0) {
		t.Fatalf("invalid rowScaleOf(0) = %v", rs)
	}
	_ = s.unscaledBasicVal(0)
	_ = s.unscaledTabEntry(s.ws.basis[0], 0)

	snap := s.snapshot()
	// Active cut: x <= 2 -> optimum shifts to x=2, y=1, obj=4.
	s.AddCut([]int{0}, []float64{1.0}, 2.0)
	// Inactive slack cut: x + y <= 100.
	s.AddCut([]int{0, 1}, []float64{1.0, 1.0}, 100.0)
	if len(s.extraRows) != 2 {
		t.Fatalf("expected 2 extraRows, got %d", len(s.extraRows))
	}
	r1 := s.Solve()
	if r1.Status != StatusOptimal || math.Abs(r1.ObjectiveMin-4.0) > 1e-6 {
		t.Fatalf("Ruiz cut solve: status=%v obj=%v, want Optimal 4", r1.Status, r1.ObjectiveMin)
	}
	if rsCut := s.rowScaleOf(p.numConstraints); rsCut <= 0 {
		t.Fatalf("invalid extra rowScaleOf = %v", rsCut)
	}
	if purged := s.PurgeSlackCuts(1e-4); purged < 1 {
		t.Fatalf("expected inactive cut purge >= 1, got %d", purged)
	}
	s.restore(snap)
	if len(s.extraRows) != 0 {
		t.Fatalf("restore left %d extraRows", len(s.extraRows))
	}
	r2 := s.Solve()
	if r2.Status != StatusOptimal || math.Abs(r2.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("after restore: status=%v obj=%v, want Optimal 3", r2.Status, r2.ObjectiveMin)
	}

	// Ruiz AddCut with huge coefficient that would overflow naive vals[k] * colScale[c]
	// must use Ldexp safely without producing NaN/Inf.
	mBig := NewModel("ruiz_cut_ldexp", Minimize)
	bx := mBig.AddContVar("bx", 0, 10)
	by := mBig.AddContVar("by", 0, 10)
	mBig.SetObjective(NewExpr(bx.T(1), by.T(1)))
	mBig.AddConstraint("tiny", NewExpr(bx.T(1e-8), by.T(1e-8)).Ge(1e-8))
	mBig.AddConstraint("big", NewExpr(bx.T(1e4), by.T(1e-8)).Ge(1e4))
	sBig := NewLPSolver(mBig.Compile(), nil)
	defer sBig.Close()
	if rB0 := sBig.Solve(); rB0.Status != StatusOptimal {
		t.Fatalf("ruiz_cut_ldexp base: %v", rB0.Status)
	}
	sBig.AddCut([]int{1}, []float64{1e305}, 1e306)
	if rB1 := sBig.Solve(); rB1.Status != StatusOptimal || math.IsNaN(rB1.ObjectiveMin) {
		t.Fatalf("ruiz_cut_ldexp after huge cut: status=%v obj=%v", rB1.Status, rB1.ObjectiveMin)
	}

	// CopyRootBasisFrom rejects mismatched problem dimensions safely.
	mOther := NewModel("other_dim", Minimize)
	ox := mOther.AddContVar("ox", 0, 5)
	mOther.SetObjective(VarExpr(ox))
	mOther.AddConstraint("c0", VarExpr(ox).Ge(1))
	sOther := NewLPSolver(mOther.Compile(), nil)
	defer sOther.Close()
	sOther.CopyRootBasisFrom(s)
	if sOther.dualValid {
		t.Fatal("CopyRootBasisFrom copied root basis across mismatched dimensions")
	}

	// 2. Crash basis recovery helpers (reinitCrashKeepingBounds and reinitSmallestMagnitudeCrash).
	s.reinitCrashKeepingBounds()
	if rCrash := s.Solve(); rCrash.Status != StatusOptimal || math.Abs(rCrash.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("after reinitCrashKeepingBounds: status=%v obj=%v", rCrash.Status, rCrash.ObjectiveMin)
	}
	s.reinitSmallestMagnitudeCrash()
	if rSmall := s.Solve(); rSmall.Status != StatusOptimal || math.Abs(rSmall.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("after reinitSmallestMagnitudeCrash: status=%v obj=%v", rSmall.Status, rSmall.ObjectiveMin)
	}

	// 3. budgetBound helper.
	eng := &babEngine{defaultLPIter: 1000}
	s.MaxIter = 500
	if !eng.budgetBound(s) {
		t.Fatal("expected budgetBound == true when MaxIter < defaultLPIter")
	}
	s.MaxIter = 1000
	if eng.budgetBound(s) {
		t.Fatal("expected budgetBound == false when MaxIter == defaultLPIter")
	}

	// 4. M9: separateRowMIRCuts overflow and finalizeCut non-finite guards.
	mMIR := NewModel("mir_overflow", Minimize)
	mx := mMIR.AddIntVar("mx", 0, 10)
	my := mMIR.AddContVar("my", 0, 10)
	mMIR.SetObjective(NewExpr(mx.T(1), my.T(1)))
	mMIR.AddConstraint("r0", NewExpr(mx.T(1e-200), my.T(1)).Le(-1e200))
	pMIR := mMIR.Compile()
	for _, c := range separateRowMIRCuts(pMIR, []float64{0.5, 0.0}) {
		if math.IsNaN(c.RHS) || math.IsInf(c.RHS, 0) || math.IsNaN(c.Efficacy) || math.IsInf(c.Efficacy, 0) {
			t.Fatalf("separateRowMIRCuts emitted non-finite cut: %+v", c)
		}
	}
	if _, ok := finalizeCut(pMIR, []float64{1.0, 0.0}, math.NaN(), []float64{0.5, 0.0}); ok {
		t.Fatal("finalizeCut accepted NaN rhs")
	}
	if _, ok := finalizeCut(pMIR, []float64{1.0, 0.0}, math.Inf(-1), []float64{0.5, 0.0}); ok {
		t.Fatal("finalizeCut accepted -Inf rhs")
	}
	if cLarge, ok := finalizeCut(pMIR, []float64{1e200, 1e200}, 0.0, []float64{1.0, 1.0}); !ok || math.IsNaN(cLarge.RHS) || math.IsInf(cLarge.RHS, 0) {
		t.Fatalf("finalizeCut failed scaled norm on [1e200, 1e200]: ok=%v cut=%+v", ok, cLarge)
	}
	if _, ok := finalizeCut(pMIR, []float64{1.5e308, 1.5e308}, 0.0, []float64{1.0, 1.0}); ok {
		t.Fatal("finalizeCut accepted norm-overflowing [1.5e308, 1.5e308] cut")
	}

	// 5. M6: selectBranchVar equal-score sorting, out-of-bounds v clamping, and createChildNodes strict tightening.
	mBr := NewModel("branch_select", Minimize)
	bVars := make([]Var, 20)
	for j := range bVars {
		bVars[j] = mBr.AddIntVar(fmt.Sprintf("x%d", j), 0, 1)
	}
	mBr.SetObjective(VarExpr(bVars[0]))
	pBr := mBr.Compile()
	lpBr := NewLPSolver(pBr, nil)
	defer lpBr.Close()
	primal := make([]float64, 20)
	curLB := make([]float64, 20)
	curUB := make([]float64, 20)
	for j := range 20 {
		primal[j] = 0.5
		curLB[j] = 0
		curUB[j] = 1
	}
	// Fix col 0 to [1, 1] and give col 1 an out-of-bounds value -1e-5; equal scores on remaining cols.
	curLB[0] = 1
	curUB[0] = 1
	primal[0] = 0.5
	primal[1] = -1e-5
	pCosts := make([]pseudoCost, 20)
	iters := 0
	brCol, brVal := selectBranchVar(pBr, lpBr, LPResult{Status: StatusOptimal, Primal: primal}, curLB, curUB, pCosts, 1, &iters, func() int { return 100 })
	if brCol <= 0 || brVal <= curLB[brCol] || brVal >= curUB[brCol] {
		t.Fatalf("selectBranchVar returned invalid (%d, %v) for bounds [%v, %v]", brCol, brVal, curLB[max(0, brCol)], curUB[max(0, brCol)])
	}
	// Near-integral selector must clamp out-of-bounds value strictly inside (curLB, curUB).
	nearPrimal := make([]float64, 20)
	nearPrimal[1] = 1.0 + 5e-6
	nearCol, nearVal := selectNearIntegralBranchVar(pBr, nearPrimal, curLB, curUB)
	if nearCol != 1 || nearVal <= 0.0 || nearVal >= 1.0 {
		t.Fatalf("selectNearIntegralBranchVar = (%d, %v), want col=1 strictly in (0, 1)", nearCol, nearVal)
	}
	// createChildNodes must strictly tighten both children even when brVal is at a bound.
	cnt := 0
	downN, upN := createChildNodes(0, 1, nil, 1, 1.0, 0.0, 1.0, pseudoCost{}, &cnt)
	if downN.delta.ub >= 1.0 || upN.delta.lb <= 0.0 {
		t.Fatalf("createChildNodes failed strict tightening: down.ub=%v up.lb=%v", downN.delta.ub, upN.delta.lb)
	}

	// 6. MAJ-3: Ruiz-scaled SetStructuralBounds and AddCut with 1e305 bounds/RHS unscale cleanly
	// instead of overflowing colExp/rowExp into StatusUndefined or rejecting finite cuts.
	mUnscale := NewModel("ruiz_unscale_1e305", Minimize)
	ux := mUnscale.AddContVar("ux", 0, 10)
	uy := mUnscale.AddContVar("uy", 0, 10)
	mUnscale.SetObjective(NewExpr(ux.T(1), uy.T(2)))
	mUnscale.AddConstraint("r0", NewExpr(ux.T(1e-8), uy.T(1e-8)).Ge(2e-8))
	mUnscale.AddConstraint("r1", NewExpr(ux.T(1e4), uy.T(1e-8)).Ge(1e4))
	sUnscale := NewLPSolver(mUnscale.Compile(), nil)
	defer sUnscale.Close()
	if !sUnscale.scaled {
		t.Fatal("expected initial Ruiz scaling on ruiz_unscale_1e305")
	}
	if rInit := sUnscale.Solve(); rInit.Status != StatusOptimal || math.Abs(rInit.ObjectiveMin-2.0) > 1e-5 {
		t.Fatalf("ruiz_unscale_1e305 initial solve: status=%v obj=%v, want Optimal 2", rInit.Status, rInit.ObjectiveMin)
	}
	snapScaled := sUnscale.snapshot()
	sUnscale.AddCut([]int{0}, []float64{1.0}, 1e305)
	if rCut := sUnscale.Solve(); rCut.Status != StatusOptimal || math.Abs(rCut.ObjectiveMin-2.0) > 1e-5 {
		t.Fatalf("ruiz_unscale_1e305 after 1e305 RHS cut: status=%v obj=%v, want Optimal 2", rCut.Status, rCut.ObjectiveMin)
	}
	sUnscale.SetStructuralBounds([]float64{0, 0}, []float64{10, 1e305})
	if rBnd := sUnscale.Solve(); rBnd.Status != StatusOptimal || math.Abs(rBnd.ObjectiveMin-2.0) > 1e-5 {
		t.Fatalf("ruiz_unscale_1e305 after 1e305 UB: status=%v obj=%v, want Optimal 2", rBnd.Status, rBnd.ObjectiveMin)
	}
	sUnscale.restore(snapScaled)
	if !sUnscale.scaled && (math.Abs(sUnscale.ws.x[0]-1.0) > 1e-4 || math.Abs(sUnscale.ws.x[1]-1.0) > 1e-4) {
		t.Fatalf("ruiz_unscale_1e305 restore left scaled xb in unscaled ws.x: %v, want [1 1]", sUnscale.ws.x[:2])
	}
	if rRest := sUnscale.Solve(); rRest.Status != StatusOptimal || math.Abs(rRest.ObjectiveMin-2.0) > 1e-5 {
		t.Fatalf("ruiz_unscale_1e305 after restore across unscale: status=%v obj=%v, want Optimal 2", rRest.Status, rRest.ObjectiveMin)
	}

	// 7. finalizeCut power-of-two scaling preserves exact zero-slack witness feasibility across integer ratios.
	mNorm := NewModel("finalize_cut_pow2", Minimize)
	_ = mNorm.AddBinaryVar("x0")
	_ = mNorm.AddContVar("x1", 0, 1e18)
	_ = mNorm.AddContVar("x2", -1e18, 0)
	pNorm := mNorm.Compile()
	scale52 := math.Ldexp(1.0, 52)
	for a := 1; a <= 15; a++ {
		for b := a + 1; b <= 15; b++ {
			cut, ok := finalizeCut(pNorm, []float64{0, float64(a), float64(b)}, 0.0, []float64{0, 1.0, 0})
			if !ok {
				continue
			}
			pt := []float64{1.0, float64(b) * scale52, -float64(a) * scale52}
			act := 0.0
			for k, c := range cut.ColIdx {
				act += cut.Val[k] * pt[c]
			}
			if act > cut.RHS+1e-7 {
				t.Fatalf("finalizeCut(%d,%d) violated exact witness: act=%.9f > rhs=%.9f", a, b, act, cut.RHS)
			}
		}
	}

	// 8. tightenRowLeVerifyBounds rounds (numVal + numErr) / at outward across 1.0 power-of-two boundary.
	lbBnd := []float64{1.0 - 3.0*math.Ldexp(1.0, -53), 0, 0}
	ubBnd := []float64{math.Inf(1), 1, 1}
	at := 1.0 + math.Ldexp(1.0, -51)
	if tightenRowLeVerifyBounds(
		[]int{0, 1, 2},
		[]float64{at, -1.0, -math.Ldexp(1.0, -54)},
		0.0,
		1.0,
		lbBnd,
		ubBnd,
	) {
		t.Fatal("tightenRowLeVerifyBounds falsely declared infeasibility across 1.0 ULP boundary")
	}

	// 9. separateRowMIRCuts downward quotient rounding keeps every emitted cut valid at exact integer witnesses.
	mirDelta := 5.0 + math.Ldexp(1.0, -50)
	mirA1 := 15.0 + math.Ldexp(1.0, -49)
	mirA2 := -(10.0 + math.Ldexp(1.0, -49))
	mirU1 := math.Ldexp(1.0, 49)
	mirU2 := 3.0 * math.Ldexp(1.0, 48)
	mMIRCuts := NewModel("mir_direct_soundness", Maximize)
	mx0 := mMIRCuts.AddBinaryVar("x0")
	mx1 := mMIRCuts.AddIntVar("x1", 0, mirU1)
	mx2 := mMIRCuts.AddIntVar("x2", 0, mirU2)
	mMIRCuts.AddConstraint("r", NewExpr(mx0.T(mirDelta), mx1.T(mirA1), mx2.T(mirA2)).Le(4.9))
	pMIRCuts := mMIRCuts.Compile()
	mirWit := []float64{1.0, mirU1, mirU2}
	for _, cut := range separateRowMIRCuts(pMIRCuts, []float64{4.9 / mirDelta, 0, 0}) {
		act := 0.0
		for k, c := range cut.ColIdx {
			act += cut.Val[k] * mirWit[c]
		}
		if act > cut.RHS+1e-7 {
			t.Fatalf("separateRowMIRCuts emitted cut violating exact witness: act=%.9f > rhs=%.9f", act, cut.RHS)
		}
	}
}

func assertExtraColumnParity(t *testing.T, label string, s *LPSolver) {
	t.Helper()
	p := s.mat
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()

	// Seed deterministic non-zero duals to test colDualNoiseTol against row-wise scan.
	savedY := make([]float64, m)
	copy(savedY, s.ws.y[:m])
	for i := range m {
		s.ws.y[i] = float64(i+1) * 0.375
		if i%2 == 1 {
			s.ws.y[i] = -s.ws.y[i]
		}
	}
	defer copy(s.ws.y[:m], savedY)

	ref := make([]float64, m)
	gotFill := make([]float64, m)
	gotSparse := make([]float64, m)
	gotScaled := make([]float64, m)
	const scale = -2.25

	for col := range n {
		clear(ref)
		for k := p.colPtr[col]; k < p.colPtr[col+1]; k++ {
			ref[p.rowIdx[k]] = p.colVal[k]
		}
		for e, dr := range s.extraRows {
			for k, c := range dr.ColIdx {
				if c == col && dr.Val[k] != 0 {
					ref[baseM+e] = dr.Val[k]
				}
			}
		}

		clear(gotFill)
		s.fillColumn(col, gotFill)
		for i := range m {
			if math.Float64bits(gotFill[i]) != math.Float64bits(ref[i]) {
				t.Fatalf("%s col %d fillColumn[%d] = %.17g, want %.17g", label, col, i, gotFill[i], ref[i])
			}
		}

		clear(gotSparse)
		spIdx := s.fillColumnSparse(col, gotSparse, nil)
		for i := range m {
			if math.Float64bits(gotSparse[i]) != math.Float64bits(ref[i]) {
				t.Fatalf("%s col %d fillColumnSparse[%d] = %.17g, want %.17g", label, col, i, gotSparse[i], ref[i])
			}
		}
		var wantRows []int
		var wantVals []float64
		for i, v := range ref {
			if v != 0 {
				wantRows = append(wantRows, i)
				wantVals = append(wantVals, v)
			}
		}
		if len(spIdx) != len(wantRows) {
			t.Fatalf("%s col %d fillColumnSparse idx len = %d, want %d", label, col, len(spIdx), len(wantRows))
		}
		for k, r := range spIdx {
			if r != wantRows[k] {
				t.Fatalf("%s col %d fillColumnSparse idx[%d] = %d, want %d", label, col, k, r, wantRows[k])
			}
		}

		gotRows, gotVals := s.appendBasisColumn(col, nil, nil)
		if len(gotRows) != len(wantRows) || len(gotVals) != len(wantVals) {
			t.Fatalf("%s col %d appendBasisColumn len = (%d,%d), want (%d,%d)", label, col, len(gotRows), len(gotVals), len(wantRows), len(wantVals))
		}
		for k := range wantRows {
			if gotRows[k] != wantRows[k] || math.Float64bits(gotVals[k]) != math.Float64bits(wantVals[k]) {
				t.Fatalf("%s col %d appendBasisColumn[%d] = (%d,%.17g), want (%d,%.17g)", label, col, k, gotRows[k], gotVals[k], wantRows[k], wantVals[k])
			}
		}

		clear(gotScaled)
		scIdx := s.addScaledColumn(col, scale, gotScaled, nil)
		touched := make([]bool, m)
		for _, r := range scIdx {
			touched[r] = true
		}
		for i, rv := range ref {
			want := 0.0
			if rv != 0 {
				want = scale * rv
				if !touched[i] {
					t.Fatalf("%s col %d addScaledColumn missing touched row %d", label, col, i)
				}
			}
			if math.Float64bits(gotScaled[i]) != math.Float64bits(want) {
				t.Fatalf("%s col %d addScaledColumn[%d] = %.17g, want %.17g", label, col, i, gotScaled[i], want)
			}
		}

		refL1 := 0.0
		for q := p.colPtr[col]; q < p.colPtr[col+1]; q++ {
			r := p.rowIdx[q]
			if uint(r) < uint(m) {
				refL1 += math.Abs(s.ws.y[r] * p.colVal[q])
			}
		}
		for e, dr := range s.extraRows {
			yr := s.ws.y[baseM+e]
			if yr == 0 {
				continue
			}
			for k, c := range dr.ColIdx {
				if c == col {
					refL1 += math.Abs(yr * dr.Val[k])
					break
				}
			}
		}
		wantNoise := 128.0 * unitRoundoff64 * refL1
		gotNoise := s.colDualNoiseTol(col, s.ws.cost)
		if math.Float64bits(gotNoise) != math.Float64bits(wantNoise) {
			t.Fatalf("%s col %d colDualNoiseTol = %.17g, want %.17g", label, col, gotNoise, wantNoise)
		}
	}
}

// TestExtraRowCSCSyncAndColumnParity verifies that the lazy CSC index over dynamic cut rows
// matches direct row-wise scans bit-for-bit across AddCut, PurgeSlackCuts, snapshot restore,
// unscaleToOriginal, InitCrashBasis, and shared workspace tolerance reuse.
func TestExtraRowCSCSyncAndColumnParity(t *testing.T) {
	// Exercise Ruiz-scaled LP transitions across AddCut, PurgeSlackCuts, snapshot restore, and unscaleToOriginal.
	m := NewModel("extra_csc_ruiz", Minimize)
	x0 := m.AddContVar("x0", 0, 100)
	x1 := m.AddContVar("x1", 0, 100)
	x2 := m.AddContVar("x2", 0, 100)
	m.SetObjective(NewExpr(x0.T(1), x1.T(2), x2.T(3)))
	m.AddConstraint("r0", NewExpr(x0.T(1e-8), x1.T(1e-8), x2.T(1e-8)).Ge(3e-8))
	m.AddConstraint("r1", NewExpr(x0.T(2e4), x1.T(1e4)).Ge(4e4))
	p := m.Compile()
	s := NewLPSolver(p, nil)
	defer s.Close()
	if !s.scaled {
		t.Fatal("expected Ruiz scaling on wide-scale test LP")
	}

	if res := s.Solve(); res.Status != StatusOptimal {
		t.Fatalf("base solve failed: %v", res.Status)
	}
	assertExtraColumnParity(t, "base_zero_cuts", s)

	snap0 := s.snapshot()
	// Add an active cut, a loose slack cut, and a non-canonical unsorted duplicate cut.
	s.AddCut([]int{0}, []float64{1.0}, 2.0)
	assertExtraColumnParity(t, "after_first_addcut", s)
	snap1 := s.snapshot()

	s.AddCut([]int{2, 0, 2}, []float64{0.5, 1.0, 0.5}, 100.0)
	s.AddCut([]int{1, 2}, []float64{1.5, -0.25}, 150.0)
	assertExtraColumnParity(t, "after_three_addcuts", s)

	resCut := s.Solve()
	if resCut.Status != StatusOptimal || math.Abs(resCut.ObjectiveMin-4.0) > 1e-6 {
		t.Fatalf("cut solve status=%v obj=%v, want Optimal 4", resCut.Status, resCut.ObjectiveMin)
	}
	for r := range s.TotalRows() {
		if !s.computeExactTableauRow(r) {
			t.Fatalf("computeExactTableauRow(%d) failed after cuts", r)
		}
	}

	purged := s.PurgeSlackCuts(1e-4)
	if purged < 1 {
		t.Fatalf("expected PurgeSlackCuts to drop inactive cuts, dropped %d", purged)
	}
	assertExtraColumnParity(t, "after_purge_slack_cuts", s)
	if resPurge := s.Solve(); resPurge.Status != StatusOptimal || math.Abs(resPurge.ObjectiveMin-4.0) > 1e-6 {
		t.Fatalf("post-purge solve status=%v obj=%v, want Optimal 4", resPurge.Status, resPurge.ObjectiveMin)
	}

	// Restore to snap1, append a second cut, and trigger unscaleToOriginal.
	s.restore(snap1)
	assertExtraColumnParity(t, "after_restore_snap1", s)
	s.AddCut([]int{0, 1}, []float64{1.0, 2.0}, 50.0)
	assertExtraColumnParity(t, "before_unscale_two_cuts", s)
	s.unscaleToOriginal()
	if s.scaled {
		t.Fatal("expected solver to be unscaled after unscaleToOriginal")
	}
	assertExtraColumnParity(t, "after_unscale_two_cuts", s)
	if resUnscaled := s.Solve(); resUnscaled.Status != StatusOptimal || math.Abs(resUnscaled.ObjectiveMin-4.0) > 1e-6 {
		t.Fatalf("post-unscale solve status=%v obj=%v, want Optimal 4", resUnscaled.Status, resUnscaled.ObjectiveMin)
	}

	s.InitCrashBasis()
	assertExtraColumnParity(t, "after_init_crash_with_cuts", s)

	s.restore(snap0)
	if len(s.extraRows) != 0 {
		t.Fatalf("expected 0 extraRows after restoring snap0, got %d", len(s.extraRows))
	}
	assertExtraColumnParity(t, "after_restore_snap0", s)

	// Verify pooled Workspace varTol invalidation when two solvers with different column scales share one Workspace.
	mSmall := NewModel("small_norm", Minimize)
	a0 := mSmall.AddContVar("a0", 0, 10)
	a1 := mSmall.AddContVar("a1", 0, 10)
	mSmall.SetObjective(NewExpr(a0.T(1), a1.T(1)))
	mSmall.AddConstraint("c0", NewExpr(a0.T(1), a1.T(1)).Ge(2))

	mBig := NewModel("big_norm", Minimize)
	b0 := mBig.AddContVar("b0", 0, 10)
	b1 := mBig.AddContVar("b1", 0, 10)
	mBig.SetObjective(NewExpr(b0.T(1), b1.T(1)))
	mBig.AddConstraint("c0", NewExpr(b0.T(1000), b1.T(1)).Ge(2))

	ws := AcquireWorkspace(2, 1)
	defer ReleaseWorkspace(ws)

	s1 := NewLPSolver(mSmall.Compile(), ws)
	tol1 := append([]float64(nil), s1.syncVarTol(ws.totVar)...)
	s2 := NewLPSolver(mBig.Compile(), ws)
	tol2 := s2.syncVarTol(ws.totVar)
	if math.Float64bits(tol1[0]) == math.Float64bits(tol2[0]) {
		t.Fatalf("shared workspace leaked stale varTol[0]: got %.17g for both models", tol2[0])
	}
	wantTol2 := s2.feasTol(0)
	if math.Float64bits(tol2[0]) != math.Float64bits(wantTol2) {
		t.Fatalf("shared workspace varTol[0] = %.17g, want %.17g", tol2[0], wantTol2)
	}
}

func loadPublicPuLPJSONForTest(t *testing.T, id string) *Model {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "public", id+".json"))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	var doc struct {
		Parameters struct {
			Name  string `json:"name"`
			Sense int    `json:"sense"`
		} `json:"parameters"`
		Objective struct {
			Coefficients []struct {
				Name  string  `json:"name"`
				Value float64 `json:"value"`
			} `json:"coefficients"`
			Constant float64 `json:"constant,omitzero"`
		} `json:"objective"`
		Variables []struct {
			Name     string   `json:"name"`
			Cat      string   `json:"cat"`
			LowBound *float64 `json:"lowBound"`
			UpBound  *float64 `json:"upBound"`
		} `json:"variables"`
		Constraints []struct {
			Name         string `json:"name"`
			Sense        int    `json:"sense"`
			Coefficients []struct {
				Name  string  `json:"name"`
				Value float64 `json:"value"`
			} `json:"coefficients"`
			Constant float64 `json:"constant"`
		} `json:"constraints"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", id, err)
	}
	sense := Minimize
	if doc.Parameters.Sense == -1 {
		sense = Maximize
	}
	m := NewModel(doc.Parameters.Name, sense)
	vars := make(map[string]Var, len(doc.Variables))
	for _, v := range doc.Variables {
		lb := math.Inf(-1)
		if v.LowBound != nil {
			lb = *v.LowBound
		}
		ub := math.Inf(1)
		if v.UpBound != nil {
			ub = *v.UpBound
		}
		if v.Cat == "Integer" {
			vars[v.Name] = m.AddIntVar(v.Name, lb, ub)
		} else {
			vars[v.Name] = m.AddContVar(v.Name, lb, ub)
		}
	}
	obj := NewExpr().AddConstant(doc.Objective.Constant)
	for _, c := range doc.Objective.Coefficients {
		obj = obj.AddTerm(vars[c.Name], c.Value)
	}
	m.SetObjective(obj)
	for _, con := range doc.Constraints {
		e := NewExpr()
		for _, c := range con.Coefficients {
			e = e.AddTerm(vars[c.Name], c.Value)
		}
		rhs := -con.Constant
		switch con.Sense {
		case -1:
			m.AddConstraint(con.Name, Le(e, rhs))
		case 0:
			m.AddConstraint(con.Name, Eq(e, rhs))
		case 1:
			m.AddConstraint(con.Name, Ge(e, rhs))
		default:
			t.Fatalf("%s constraint %s invalid sense %d", id, con.Name, con.Sense)
		}
	}
	return m
}

// TestDualPhase1NetlibNoPrimalFallback verifies that Dual Phase I via temporary bound shifting
// solves the Netlib instances with negative-cost unbounded columns directly inside sparse dual
// simplex without falling back to two-phase primal simplex, while matching exact objectives and KKT checks.
func TestDualPhase1NetlibNoPrimalFallback(t *testing.T) {
	rawManifest, err := os.ReadFile(filepath.Join("testdata", "public", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest []struct {
		ID                 string  `json:"id"`
		IsMIP              bool    `json:"is_mip"`
		ReferenceObjective float64 `json:"reference_objective"`
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	mustUseDualPhase1 := map[string]bool{
		"netlib_e226":     true,
		"netlib_lotfi":    true,
		"netlib_israel":   true,
		"netlib_share2b":  true,
		"netlib_stocfor1": true,
	}

	for _, inst := range manifest {
		if inst.IsMIP {
			continue
		}
		t.Run(inst.ID, func(t *testing.T) {
			m := loadPublicPuLPJSONForTest(t, inst.ID)
			p := m.Compile()
			s := NewLPSolver(p, nil)
			defer s.Close()

			res := s.Solve()
			t.Logf("%s: status=%v obj=%.10g ref=%.10g iters=%d dualIters=%d primalIters=%d dualPhase1=%v primalFallback=%v scaled=%v",
				inst.ID, res.Status, res.Objective, inst.ReferenceObjective, res.Iterations,
				s.lastDualIters, s.lastPrimalIters, s.lastDualPhase1Used, s.lastPrimalFallback, s.scaled)

			if res.Status != StatusOptimal {
				t.Fatalf("%s: status=%v, want Optimal", inst.ID, res.Status)
			}
			rel := math.Abs(res.Objective-inst.ReferenceObjective) / max(1.0, math.Abs(inst.ReferenceObjective))
			if rel > 1e-5 {
				t.Fatalf("%s: objective=%.10g, want %.10g (rel=%.2e)", inst.ID, res.Objective, inst.ReferenceObjective, rel)
			}
			bV, rV, _ := p.MaxViolation(res.Primal, false)
			if bV > 1e-5 || rV > 1e-5 {
				t.Fatalf("%s: primal violation bound=%.2e row=%.2e", inst.ID, bV, rV)
			}
			if !s.primalResidualsWithinTol(res.Primal) || !s.dualResidualsWithinTol(res.Primal, res.ObjectiveMin) {
				t.Fatalf("%s: KKT residual verification failed", inst.ID)
			}
			if mustUseDualPhase1[inst.ID] {
				if !s.lastDualPhase1Used {
					t.Fatalf("%s: expected Dual Phase I bound shifting to activate", inst.ID)
				}
				if s.lastPrimalFallback {
					t.Fatalf("%s: unexpectedly fell back to two-phase primal simplex (dualIters=%d primalIters=%d)",
						inst.ID, s.lastDualIters, s.lastPrimalIters)
				}
			}
		})
	}
}

// TestDualPhase1EdgeCases exercises soundness of Dual Phase I bound shifting across unbounded LPs,
// genuinely infeasible LPs, feasible LPs whose shifted subproblem is infeasible, free variables,
// huge optimal solutions exceeding the temporary shift bound, and Ruiz-scaled LPs.
func TestDualPhase1EdgeCases(t *testing.T) {
	t.Run("unbounded_negative_cost_pos_inf", func(t *testing.T) {
		m := NewModel("unb_neg", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("c0", NewExpr(x.T(1), y.T(-1)).Le(1))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		res := s.Solve()
		if res.Status != StatusUnbounded {
			t.Fatalf("status=%v, want Unbounded", res.Status)
		}
	})

	t.Run("unbounded_positive_cost_neg_inf", func(t *testing.T) {
		m := NewModel("unb_pos", Minimize)
		x := m.AddContVar("x", math.Inf(-1), 0)
		y := m.AddContVar("y", math.Inf(-1), 0)
		m.SetObjective(NewExpr(x.T(2)))
		m.AddConstraint("c0", NewExpr(x.T(1), y.T(-1)).Ge(-1))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		res := s.Solve()
		if res.Status != StatusUnbounded {
			t.Fatalf("status=%v, want Unbounded", res.Status)
		}
	})

	t.Run("infeasible_original_and_shifted", func(t *testing.T) {
		m := NewModel("inf_both", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-3), y.T(1)))
		m.AddConstraint("c0", NewExpr(x.T(1), y.T(1)).Le(2))
		m.AddConstraint("c1", NewExpr(x.T(1), y.T(1)).Ge(5))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		res := s.Solve()
		if res.Status != StatusInfeasible {
			t.Fatalf("status=%v, want Infeasible", res.Status)
		}
		if !s.lastDualPhase1Used || s.lastPrimalFallback {
			t.Fatalf("expected direct Dual Phase I Farkas certification: dualPhase1=%v primalFallback=%v",
				s.lastDualPhase1Used, s.lastPrimalFallback)
		}
	})

	t.Run("feasible_original_but_shifted_infeasible", func(t *testing.T) {
		// Row 1e-8*x >= 1 forces x >= 1e8 while workScale=1 sets tempUB=1e6, making the shifted LP infeasible.
		m := NewModel("shifted_inf_orig_feas", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("lo", NewExpr(x.T(1e-8)).Ge(1))
		m.AddConstraint("hi", NewExpr(x.T(1e-8)).Le(2))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		res := s.Solve()
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want Optimal (must not report false Infeasible from temporary shift bound)", res.Status)
		}
		if math.Abs(res.Primal[0]-2e8) > 1e2 || math.Abs(res.Objective-(-2e8)) > 1e2 {
			t.Fatalf("x=%g obj=%g, want x=2e8 obj=-2e8", res.Primal[0], res.Objective)
		}
	})

	t.Run("huge_optimal_solution_pinned_temp_bound", func(t *testing.T) {
		// Optimal x*=1e8 exceeds shift=1e6, pinning x at the temporary bound before recovery.
		m := NewModel("huge_opt", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, 10)
		m.SetObjective(NewExpr(x.T(-1), y.T(2)))
		m.AddConstraint("cap", NewExpr(x.T(1e-8), y.T(1)).Le(1))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		res := s.Solve()
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want Optimal", res.Status)
		}
		if math.Abs(res.Primal[0]-1e8) > 1e2 || math.Abs(res.Objective-(-1e8)) > 1e2 {
			t.Fatalf("x=%g obj=%g, want x=1e8 obj=-1e8", res.Primal[0], res.Objective)
		}
	})

	t.Run("free_variable_bounded_unbounded_infeasible", func(t *testing.T) {
		mOpt := NewModel("free_opt", Minimize)
		f := mOpt.AddContVar("f", math.Inf(-1), math.Inf(1))
		x := mOpt.AddContVar("x", 0, 10)
		mOpt.SetObjective(NewExpr(f.T(2), x.T(1)))
		mOpt.AddConstraint("c0", NewExpr(f.T(1), x.T(1)).Ge(3))
		mOpt.AddConstraint("c1", NewExpr(f.T(1), x.T(-1)).Ge(-5))
		sOpt := NewLPSolver(mOpt.Compile(), nil)
		defer sOpt.Close()
		rOpt := sOpt.Solve()
		// Minimize 2f + x with f >= 3 - x and f >= x - 5, x in [0, 10]: minimum at f=-1, x=4, obj=2.
		if rOpt.Status != StatusOptimal || math.Abs(rOpt.Objective-2.0) > 1e-6 {
			t.Fatalf("free_opt: status=%v obj=%g, want Optimal 2", rOpt.Status, rOpt.Objective)
		}
		if !sOpt.lastDualPhase1Used || sOpt.lastPrimalFallback {
			t.Fatalf("free_opt: expected Dual Phase I solve without primal fallback (dualPhase1=%v primalFallback=%v)",
				sOpt.lastDualPhase1Used, sOpt.lastPrimalFallback)
		}

		mUnb := NewModel("free_unb", Minimize)
		fu := mUnb.AddContVar("f", math.Inf(-1), math.Inf(1))
		mUnb.SetObjective(NewExpr(fu.T(1)))
		mUnb.AddConstraint("c0", NewExpr(fu.T(1)).Le(5))
		sUnb := NewLPSolver(mUnb.Compile(), nil)
		defer sUnb.Close()
		if rUnb := sUnb.Solve(); rUnb.Status != StatusUnbounded {
			t.Fatalf("free_unb: status=%v, want Unbounded", rUnb.Status)
		}

		mInf := NewModel("free_inf", Minimize)
		fi := mInf.AddContVar("f", math.Inf(-1), math.Inf(1))
		xi := mInf.AddContVar("x", 0, 1)
		mInf.SetObjective(NewExpr(fi.T(1)))
		mInf.AddConstraint("c0", NewExpr(fi.T(1), xi.T(1)).Le(1))
		mInf.AddConstraint("c1", NewExpr(fi.T(1), xi.T(1)).Ge(5))
		sInf := NewLPSolver(mInf.Compile(), nil)
		defer sInf.Close()
		if rInf := sInf.Solve(); rInf.Status != StatusInfeasible {
			t.Fatalf("free_inf: status=%v, want Infeasible", rInf.Status)
		}
	})

	t.Run("ruiz_scaled_dual_phase1", func(t *testing.T) {
		m := NewModel("ruiz_dp1", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-2), y.T(-1)))
		m.AddConstraint("r0", NewExpr(x.T(1e-8), y.T(1e-8)).Le(5e-8))
		m.AddConstraint("r1", NewExpr(x.T(2e4), y.T(1e4)).Le(8e4))
		s := NewLPSolver(m.Compile(), nil)
		defer s.Close()
		if !s.scaled {
			t.Fatal("expected Ruiz scaling to activate")
		}
		res := s.Solve()
		// Maximize 2x + y s.t. x + y <= 5, 2x + y <= 8 -> optimal 2x + y = 8 (min = -8).
		if res.Status != StatusOptimal || math.Abs(res.Objective-(-8.0)) > 1e-5 {
			t.Fatalf("ruiz_dp1: status=%v obj=%g, want Optimal -8", res.Status, res.Objective)
		}
		if !s.lastDualPhase1Used || s.lastPrimalFallback {
			t.Fatalf("ruiz_dp1: expected Dual Phase I without primal fallback (dualPhase1=%v primalFallback=%v)",
				s.lastDualPhase1Used, s.lastPrimalFallback)
		}
	})
}

func assertSolverBoundStateEqual(t *testing.T, label string, got, want *LPSolver) {
	t.Helper()
	if got.scaled != want.scaled || got.invalidBounds != want.invalidBounds ||
		got.luValid != want.luValid || got.dualValid != want.dualValid {
		t.Fatalf("%s: flags mismatch got=(scaled=%v inv=%v lu=%v dual=%v) want=(scaled=%v inv=%v lu=%v dual=%v)",
			label, got.scaled, got.invalidBounds, got.luValid, got.dualValid,
			want.scaled, want.invalidBounds, want.luValid, want.dualValid)
	}
	n := want.prob.numVars
	m := want.TotalRows()
	for j := range n {
		if math.Float64bits(got.ws.lb[j]) != math.Float64bits(want.ws.lb[j]) ||
			math.Float64bits(got.ws.ub[j]) != math.Float64bits(want.ws.ub[j]) ||
			got.ws.status[j] != want.ws.status[j] ||
			math.Float64bits(got.ws.x[j]) != math.Float64bits(want.ws.x[j]) {
			t.Fatalf("%s col %d: got=(lb=%g ub=%g st=%v x=%g) want=(lb=%g ub=%g st=%v x=%g)",
				label, j, got.ws.lb[j], got.ws.ub[j], got.ws.status[j], got.ws.x[j],
				want.ws.lb[j], want.ws.ub[j], want.ws.status[j], want.ws.x[j])
		}
	}
	if got.scaled {
		for j := range n {
			if math.Float64bits(got.origLB[j]) != math.Float64bits(want.origLB[j]) ||
				math.Float64bits(got.origUB[j]) != math.Float64bits(want.origUB[j]) {
				t.Fatalf("%s orig bounds col %d mismatch: got=[%g,%g] want=[%g,%g]",
					label, j, got.origLB[j], got.origUB[j], want.origLB[j], want.origUB[j])
			}
		}
	}
	if got.basicBoundsValid && want.basicBoundsValid {
		for r := range m {
			if math.Float64bits(got.ws.basicLB[r]) != math.Float64bits(want.ws.basicLB[r]) ||
				math.Float64bits(got.ws.basicUB[r]) != math.Float64bits(want.ws.basicUB[r]) {
				t.Fatalf("%s basic bounds row %d mismatch: got=[%g,%g] want=[%g,%g]",
					label, r, got.ws.basicLB[r], got.ws.basicUB[r], want.ws.basicLB[r], want.ws.basicUB[r])
			}
		}
	}
}

// TestSetColBoundEquivalence verifies bit-for-bit state parity between SetColBound and
// SetStructuralBounds across basic, lower, upper, fixed, free, invalid, and Ruiz-scaled transitions.
func TestSetColBoundEquivalence(t *testing.T) {
	buildPair := func(scaled bool) (*LPSolver, *LPSolver, []float64, []float64) {
		m := NewModel("set_col_bound", Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		x2 := m.AddContVar("x2", 0, 10)
		x3 := m.AddContVar("x3", 0, 10)
		m.SetObjective(NewExpr(x0.T(1), x1.T(2), x2.T(-3), x3.T(4)))
		if scaled {
			m.AddConstraint("r0", NewExpr(x0.T(1e-8), x1.T(1e-8), x2.T(1e-8), x3.T(1e-8)).Ge(3e-8))
			m.AddConstraint("r1", NewExpr(x0.T(2e4), x1.T(1e4), x2.T(1e4)).Le(12e4))
		} else {
			m.AddConstraint("r0", NewExpr(x0.T(1), x1.T(1), x2.T(1), x3.T(1)).Ge(3))
			m.AddConstraint("r1", NewExpr(x0.T(2), x1.T(1), x2.T(1)).Le(12))
		}
		p := m.Compile()
		sA := NewLPSolver(p, nil)
		sB := NewLPSolver(p, nil)
		if scaled && (!sA.scaled || !sB.scaled) {
			t.Fatal("expected Ruiz scaling on scaled test pair")
		}
		rA := sA.Solve()
		rB := sB.Solve()
		if rA.Status != StatusOptimal || rB.Status != StatusOptimal {
			t.Fatalf("initial solves failed: %v / %v", rA.Status, rB.Status)
		}
		lb := slices.Clone(p.varLB)
		ub := slices.Clone(p.varUB)
		return sA, sB, lb, ub
	}

	for _, scaled := range []bool{false, true} {
		name := "unscaled"
		if scaled {
			name = "scaled"
		}
		t.Run(name, func(t *testing.T) {
			sCol, sBulk, lb, ub := buildPair(scaled)
			defer sCol.Close()
			defer sBulk.Close()

			// Exercise every column across basic, at-lower, at-upper, fixed, one-sided, free, and invalid bounds.
			transitions := [][2]float64{
				{2, 8},
				{5, 5},                      // fixed
				{1, 9},                      // boxed from fixed (tests dj sign branch)
				{0, math.Inf(1)},            // lower only
				{math.Inf(-1), 6},           // upper only
				{math.Inf(-1), math.Inf(1)}, // free / superbasic
				{0, 10},                     // boxed from free
				{math.NaN(), 10},            // invalid NaN
				{0, 10},                     // recover from invalid
				{math.Inf(1), 10},           // invalid +Inf lb
				{0, 10},                     // recover
				{0, math.Inf(-1)},           // invalid -Inf ub
				{0, 10},                     // recover
			}
			for col := range sCol.prob.numVars {
				for step, tr := range transitions {
					origL, origU := lb[col], ub[col]
					lb[col], ub[col] = tr[0], tr[1]
					sCol.SetColBound(col, tr[0], tr[1])
					sBulk.SetStructuralBounds(lb, ub)
					label := fmt.Sprintf("%s_col%d_step%d", name, col, step)
					assertSolverBoundStateEqual(t, label, sCol, sBulk)
					if math.IsNaN(tr[0]) || math.IsNaN(tr[1]) || math.IsInf(tr[0], 1) || math.IsInf(tr[1], -1) {
						lb[col], ub[col] = origL, origU
					}
				}
			}

			// Verify extreme bound triggers identical unscaleToOriginal on scaled solver.
			if scaled {
				// Choose an extreme finite bound that overflows or underflows colExp[col].
				unscaleCol := 0
				extremeUB := math.MaxFloat64 / 2
				for j, ce := range sCol.colExp {
					if ce < 0 {
						unscaleCol = j
						extremeUB = math.MaxFloat64 / 2
						break
					}
					if ce > 0 {
						unscaleCol = j
						extremeUB = math.SmallestNonzeroFloat64
					}
				}
				lb[unscaleCol], ub[unscaleCol] = 0, extremeUB
				sCol.SetColBound(unscaleCol, 0, extremeUB)
				sBulk.SetStructuralBounds(lb, ub)
				assertSolverBoundStateEqual(t, "scaled_extreme_unscale", sCol, sBulk)
				if sCol.scaled || sBulk.scaled {
					t.Fatalf("expected unscaleToOriginal on extreme bound col=%d ub=%g colExp=%d",
						unscaleCol, extremeUB, sCol.colExp[unscaleCol])
				}
			}
		})
	}
}

// TestSolveBoundOnlyAndSkipExactFarkas verifies SolveBoundOnly dual-bound validity across optimal,
// iteration-limited, and infeasible probes, state restoration, and zero allocations on warm probes.
func TestSolveBoundOnlyAndSkipExactFarkas(t *testing.T) {
	mdl := randomBoxedLP(7, 20, 40)
	p := mdl.Compile()
	s := NewLPSolver(p, nil)
	defer s.Close()

	base := s.Solve()
	if base.Status != StatusOptimal {
		t.Fatalf("base solve: %v", base.Status)
	}

	// Pick a basic structural variable strictly inside (0, 10) to tighten into a multi-pivot probe.
	probeCol := -1
	for j := range p.numVars {
		if s.ws.status[j] == statusBasic && base.Primal[j] > 1.0 && base.Primal[j] < 9.0 {
			probeCol = j
			break
		}
	}
	if probeCol == -1 {
		t.Fatal("could not find an interior basic variable for branching probe")
	}

	var cp lpCheckpoint
	s.captureCheckpoint(&cp)

	// Compute true optimal objective after tightening probeCol >= ceil(val) + 1.
	tightLB := math.Min(9.5, math.Floor(base.Primal[probeCol])+1.5)
	s.SetColBound(probeCol, tightLB, 10)
	refRes := s.Solve()
	if refRes.Status != StatusOptimal {
		t.Fatalf("reference tightened solve: %v", refRes.Status)
	}

	// 1. Optimal SolveBoundOnly matches reference ObjectiveMin and restores solver flags.
	s.restoreCheckpoint(&cp)
	origMaxIter := s.MaxIter
	s.SetColBound(probeCol, tightLB, 10)
	stOpt, boundOpt, itOpt := s.SolveBoundOnly(500)
	if stOpt != StatusOptimal {
		t.Fatalf("SolveBoundOnly(500) status=%v, want Optimal", stOpt)
	}
	if math.Abs(boundOpt-refRes.ObjectiveMin) > 1e-6*max(1.0, math.Abs(refRes.ObjectiveMin)) {
		t.Fatalf("SolveBoundOnly bound=%.10g, want %.10g", boundOpt, refRes.ObjectiveMin)
	}
	if s.MaxIter != origMaxIter || s.SkipExactFarkas {
		t.Fatalf("SolveBoundOnly mutated solver settings: MaxIter=%d SkipExactFarkas=%v", s.MaxIter, s.SkipExactFarkas)
	}

	// 2. Iteration-limited probes return valid dual lower bounds <= refRes.ObjectiveMin.
	if itOpt >= 2 {
		prevBound := base.ObjectiveMin - 1e-6
		for capIter := 1; capIter < min(itOpt, 5); capIter++ {
			s.restoreCheckpoint(&cp)
			s.SetColBound(probeCol, tightLB, 10)
			stLim, boundLim, itLim := s.SolveBoundOnly(capIter)
			if stLim != StatusIterationLimit && stLim != StatusOptimal {
				t.Fatalf("cap=%d: unexpected status=%v", capIter, stLim)
			}
			if itLim > capIter {
				t.Fatalf("cap=%d: iters=%d exceeded cap", capIter, itLim)
			}
			if math.IsNaN(boundLim) {
				t.Fatalf("cap=%d: expected valid finite dual bound on dual-feasible iteration limit", capIter)
			}
			if boundLim > refRes.ObjectiveMin+1e-6*max(1.0, math.Abs(refRes.ObjectiveMin)) {
				t.Fatalf("cap=%d: invalid dual lower bound %.10g > true opt %.10g", capIter, boundLim, refRes.ObjectiveMin)
			}
			if boundLim < prevBound-1e-6 {
				t.Fatalf("cap=%d: dual bound regressed from %.10g to %.10g", capIter, prevBound, boundLim)
			}
			prevBound = boundLim
		}
	}

	// 3. Infeasible probe exercises SkipExactFarkas and returns StatusInfeasible with NaN bound.
	s.restoreCheckpoint(&cp)
	for j := range p.numVars {
		s.SetColBound(j, 0, 0)
	}
	stInf, boundInf, _ := s.SolveBoundOnly(50)
	if stInf != StatusInfeasible || !math.IsNaN(boundInf) {
		t.Fatalf("infeasible probe: status=%v bound=%v, want Infeasible + NaN", stInf, boundInf)
	}

	// 4. Verify zero heap allocations for warm-started SolveBoundOnly probe (no result-slice allocs).
	s.restoreCheckpoint(&cp)
	s.SetColBound(probeCol, tightLB, 10)
	_, _, _ = s.SolveBoundOnly(20) // warm up workspace buffers once
	allocs := testing.AllocsPerRun(50, func() {
		s.restoreCheckpoint(&cp)
		s.SetColBound(probeCol, tightLB, 10)
		st, bnd, _ := s.SolveBoundOnly(20)
		if (st != StatusOptimal && st != StatusIterationLimit) || math.IsNaN(bnd) {
			t.Fatalf("unexpected probe result in alloc loop: status=%v bound=%v", st, bnd)
		}
	})
	if allocs > 0 {
		t.Fatalf("warm SolveBoundOnly + checkpoint cycle allocated %.1f allocs/run, want 0", allocs)
	}
}

// TestCheckpointCaptureRestoreZeroAlloc verifies exact basis preservation across
// captureCheckpoint and restoreCheckpoint, including cut rollback, Ruiz unscaling, and 0 allocs on reuse.
func TestCheckpointCaptureRestoreZeroAlloc(t *testing.T) {
	mdl := randomBoxedLP(13, 15, 25)
	p := mdl.Compile()
	s := NewLPSolver(p, nil)
	defer s.Close()

	base := s.Solve()
	if base.Status != StatusOptimal {
		t.Fatalf("base solve: %v", base.Status)
	}

	var cp lpCheckpoint
	s.captureCheckpoint(&cp)

	// Mutate bounds and solve to a different basis, then restore checkpoint.
	s.SetColBound(0, 8, 10)
	s.SetColBound(1, 0, 1)
	if alt := s.Solve(); alt.Status != StatusOptimal && alt.Status != StatusInfeasible {
		t.Fatalf("mutated solve: %v", alt.Status)
	}
	s.restoreCheckpoint(&cp)
	restored := s.Solve()
	if restored.Status != StatusOptimal || math.Abs(restored.ObjectiveMin-base.ObjectiveMin) > 1e-8 {
		t.Fatalf("restored solve: status=%v obj=%.10g, want %.10g", restored.Status, restored.ObjectiveMin, base.ObjectiveMin)
	}
	if restored.Iterations != 0 {
		t.Fatalf("restored optimal checkpoint required %d iterations, want 0", restored.Iterations)
	}

	// Verify 0 allocations when reusing cp across repeated capture/mutate/restore cycles.
	allocs := testing.AllocsPerRun(100, func() {
		s.captureCheckpoint(&cp)
		s.SetColBound(0, 5, 10)
		s.restoreCheckpoint(&cp)
	})
	if allocs > 0 {
		t.Fatalf("captureCheckpoint/SetColBound/restoreCheckpoint allocated %.1f allocs/run, want 0", allocs)
	}

	// Verify checkpoint rollback across added dynamic cuts and Ruiz unscaleToOriginal.
	mRuiz := NewModel("cp_ruiz", Minimize)
	rx := mRuiz.AddContVar("rx", 0, 10)
	ry := mRuiz.AddContVar("ry", 0, 10)
	mRuiz.SetObjective(NewExpr(rx.T(1), ry.T(2)))
	mRuiz.AddConstraint("r0", NewExpr(rx.T(1e-8), ry.T(1e-8)).Ge(3e-8))
	mRuiz.AddConstraint("r1", NewExpr(rx.T(2e4), ry.T(1e4)).Ge(4e4))
	sRuiz := NewLPSolver(mRuiz.Compile(), nil)
	defer sRuiz.Close()
	rBase := sRuiz.Solve()
	if rBase.Status != StatusOptimal || math.Abs(rBase.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("ruiz base: status=%v obj=%g", rBase.Status, rBase.ObjectiveMin)
	}
	var cpRuiz lpCheckpoint
	sRuiz.captureCheckpoint(&cpRuiz)
	sRuiz.AddCut([]int{0}, []float64{1.0}, 2.0)
	unscaleCol := 0
	extremeUB := math.MaxFloat64 / 2
	for j, ce := range sRuiz.colExp {
		if ce < 0 {
			unscaleCol = j
			extremeUB = math.MaxFloat64 / 2
			break
		}
		if ce > 0 {
			unscaleCol = j
			extremeUB = math.SmallestNonzeroFloat64
		}
	}
	sRuiz.SetColBound(unscaleCol, 0, extremeUB) // triggers unscaleToOriginal
	if sRuiz.scaled {
		t.Fatal("expected unscaleToOriginal after extreme bound")
	}
	sRuiz.restoreCheckpoint(&cpRuiz)
	if len(sRuiz.extraRows) != 0 {
		t.Fatalf("restoreCheckpoint left %d extraRows", len(sRuiz.extraRows))
	}
	rAfter := sRuiz.Solve()
	if rAfter.Status != StatusOptimal || math.Abs(rAfter.ObjectiveMin-3.0) > 1e-6 {
		t.Fatalf("ruiz post-restore: status=%v obj=%g, want Optimal 3", rAfter.Status, rAfter.ObjectiveMin)
	}
}

// TestTwoSumTwoProdFMASafety verifies that error-free transformations (twoProd + twoSum)
// inside dot2Acc and wideDotAcc remain exact when compiled with FMA contraction enabled
// (e.g. GOAMD64=v3, GOAMD64=v4, or arm64).
func TestTwoSumTwoProdFMASafety(t *testing.T) {
	a := 1.0 + math.Ldexp(1.0, -27)
	b := 1.0 + math.Ldexp(1.0, -27)
	p, pe := twoProd(a, b)
	// Exact value of (1 + 2^-27)^2 = 1 + 2^-26 + 2^-54; p rounds to 1 + 2^-26, pe must equal 2^-54.
	wantPE := math.Ldexp(1.0, -54)
	if pe != wantPE {
		t.Fatalf("twoProd(%g, %g) err=%g, want %g (p=%g)", a, b, pe, wantPE, p)
	}

	// Accumulate a*b and -(1 + 2^-26) in dot2Acc: exact sum is 2^-54.
	// If the compiler contracts twoSum(acc.s, p) into FMA(a, b, acc.s), pe is double-counted
	// or cancelled incorrectly and the result deviates from 2^-54.
	var d2 dot2Acc
	d2.addVal(-(1.0 + math.Ldexp(1.0, -26)))
	d2.addProd(a, b)
	val, _, ok := d2.result()
	if !ok || val != wantPE {
		t.Fatalf("dot2Acc exact cancellation = %g (ok=%v), want %g", val, ok, wantPE)
	}

	var wd wideDotAcc
	wd.addVal(-(1.0 + math.Ldexp(1.0, -26)))
	wd.addProd(a, b)
	wval, _, wok := wd.finishDD()
	if !wok || wval != wantPE {
		t.Fatalf("wideDotAcc exact cancellation = %g (ok=%v), want %g", wval, wok, wantPE)
	}
}

// TestCrashEqualityBasisInvariants verifies Pass 1 zero-cost crash, Pass 2 positive-coefficient
// GUB crash, exact analytical steepest-edge initialization, dense-coupling skipping, and zero
// steady-state allocations on repeated InitCrashBasis calls.
func TestCrashEqualityBasisInvariants(t *testing.T) {
	// 1. Mix of Pass 1 zero-cost equalities and Pass 2 costed positive-coefficient equalities
	//    coupled to inequality rows (total >= 8 rows so crashEqualityBasis activates).
	m := NewModel("crash_invariants", Minimize)
	nGroups := 4
	x := make([]Var, 2*nGroups)
	for j := range x {
		x[j] = m.AddContVar(fmt.Sprintf("x%d", j), 0, 1)
	}
	z := m.AddContVar("z", 0, math.Inf(1))
	// First 2 pairs have zero cost (Pass 1); next 2 pairs have positive costs (Pass 2).
	m.SetObjective(NewExpr(
		z.T(1.0),
		x[4].T(5.0), x[5].T(2.0),
		x[6].T(6.0), x[7].T(3.0),
	))
	for i := range nGroups {
		m.AddConstraint(fmt.Sprintf("eq_%d", i), NewExpr(x[2*i].T(1.0), x[2*i+1].T(1.0)).Eq(1.0))
	}
	// 4 inequality coupling rows (total = 8 rows) with non-unit weights to verify DSE = ||B^-1 e_r||^2.
	m.AddConstraint("c0", NewExpr(z.T(1.0), x[0].T(-2.0), x[2].T(-3.0)).Ge(0))
	m.AddConstraint("c1", NewExpr(z.T(1.0), x[1].T(-4.0), x[3].T(-5.0)).Ge(0))
	m.AddConstraint("c2", NewExpr(z.T(1.0), x[4].T(-1.5), x[5].T(-2.5)).Ge(0))
	m.AddConstraint("c3", NewExpr(z.T(1.0), x[6].T(-0.5), x[7].T(-3.5)).Ge(0))

	p := m.Compile()
	s := NewLPSolver(p, nil)
	defer s.Close()

	// Verify all 4 equality rows crashed to structural variables (Pass 1: x0, x2; Pass 2: x5, x7).
	wantBasic := []int{0, 2, 5, 7}
	for r, wantCol := range wantBasic {
		if got := s.ws.basis[r]; got != wantCol {
			t.Fatalf("row %d crashed basis = %d, want structural col %d", r, got, wantCol)
		}
		if s.ws.status[p.numVars+r] != statusFixed {
			t.Fatalf("row %d artificial slack status = %v, want statusFixed", r, s.ws.status[p.numVars+r])
		}
	}
	// Verify exact analytical DSE weights on inequality coupling rows c0..c3.
	// c0 touches x0 (-2) and x2 (-3) -> 1 + 4 + 9 = 14.
	// c1 untouched by basic cols -> 1.
	// c2 touches x5 (-2.5) -> 1 + 6.25 = 7.25.
	// c3 touches x7 (-3.5) -> 1 + 12.25 = 13.25.
	wantDSE := []float64{14.0, 1.0, 7.25, 13.25}
	for k, want := range wantDSE {
		if got := s.ws.dse[nGroups+k]; math.Abs(got-want) > 1e-12 {
			t.Fatalf("ineq row %d DSE = %.12g, want %.12g", nGroups+k, got, want)
		}
	}

	// Verify optimal solve matches exact continuous LP optimum:
	// x0+x1=1, x2+x3=1 achieve min max(2x0+3x2, 4x1+5x3) = 25/8 = 3.125;
	// x5=1 (cost 2, c2=2.5) and x7=1 (cost 3, c3=3.5) force z = 3.5 (moving x7->x6 costs 3/3 = 1 per unit z),
	// giving total objective = 2 + 3 + 3.5 = 8.5.
	res := s.Solve()
	if res.Status != StatusOptimal || math.Abs(res.ObjectiveMin-8.5) > 1e-7 {
		t.Fatalf("crashed LP solve: status=%v obj=%.12g, want Optimal 8.5", res.Status, res.ObjectiveMin)
	}

	// Repeated InitCrashBasis must perform zero heap allocations on a warmed workspace.
	s.InitCrashBasis()
	allocs := testing.AllocsPerRun(50, func() {
		s.InitCrashBasis()
	})
	if allocs != 0 {
		t.Fatalf("InitCrashBasis steady-state allocs = %.1f, want 0", allocs)
	}

	// 2. Columns touching a dense coupling row (> 128 nonzeros) must be skipped by crash.
	mDense := NewModel("crash_dense_skip", Minimize)
	nItems := 140
	xd := make([]Var, 2*nItems)
	denseTerms := make([]Term, 0, nItems+1)
	zd := mDense.AddContVar("z", 0, math.Inf(1))
	denseTerms = append(denseTerms, zd.T(1.0))
	for i := range nItems {
		xd[2*i] = mDense.AddContVar(fmt.Sprintf("a%d", i), 0, 1)
		xd[2*i+1] = mDense.AddContVar(fmt.Sprintf("b%d", i), 0, 1)
		denseTerms = append(denseTerms, xd[2*i].T(-1.0))
	}
	mDense.SetObjective(VarExpr(zd))
	mDense.AddConstraint("dense_coupling", Ge(Expr{Terms: denseTerms}, 0))
	for i := range nItems {
		mDense.AddConstraint(fmt.Sprintf("eq_%d", i), NewExpr(xd[2*i].T(1.0), xd[2*i+1].T(1.0)).Eq(1.0))
	}
	pDense := mDense.Compile()
	sDense := NewLPSolver(pDense, nil)
	defer sDense.Close()
	for i := range nItems {
		// zd is col 0; xd[2*i] (col 1+2*i) touches dense_coupling (len 141 > 128),
		// so crash must select xd[2*i+1] (col 2+2*i, degree 1).
		if got := sDense.ws.basis[1+i]; got != 2+2*i {
			t.Fatalf("dense-skip eq row %d crashed to col %d, want sparse col %d", i, got, 2+2*i)
		}
	}
}
