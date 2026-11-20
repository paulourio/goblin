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
