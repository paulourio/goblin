package goblin

import (
	"math"
	"math/rand/v2"
	"testing"
)

func denseMatVec(mat [][]float64, x []float64) []float64 {
	m := len(mat)
	out := make([]float64, m)
	for r := range m {
		sum := 0.0
		row := mat[r]
		for c := range m {
			sum += row[c] * x[c]
		}
		out[r] = sum
	}
	return out
}

func denseMatTVec(mat [][]float64, y []float64) []float64 {
	m := len(mat)
	out := make([]float64, m)
	for c := range m {
		sum := 0.0
		for r := range m {
			sum += mat[r][c] * y[r]
		}
		out[c] = sum
	}
	return out
}

func maxAbsDiff(a, b []float64) float64 {
	best := 0.0
	for i := range a {
		if d := math.Abs(a[i] - b[i]); d > best {
			best = d
		}
	}
	return best
}

func maxAbsNorm(v []float64) float64 {
	best := 0.0
	for _, x := range v {
		if ax := math.Abs(x); ax > best {
			best = ax
		}
	}
	return best
}

func assertWorkClean(t *testing.T, lu *LUFactorization, context string) {
	t.Helper()
	for i := range lu.m {
		if lu.work[i] != 0 {
			t.Fatalf("%s: dirty lu.work[%d] = %.3e", context, i, lu.work[i])
		}
	}
}

func assertSparseSupportUnique(t *testing.T, vec []float64, idxs []int, dropTol float64, context string) {
	t.Helper()
	seen := make([]bool, len(vec))
	for _, idx := range idxs {
		if idx < 0 || idx >= len(vec) {
			t.Fatalf("%s: out-of-range index %d (m=%d)", context, idx, len(vec))
		}
		if seen[idx] {
			t.Fatalf("%s: duplicate output index %d", context, idx)
		}
		seen[idx] = true
		if math.Abs(vec[idx]) <= dropTol {
			t.Fatalf("%s: returned index %d has |val|=%.3e <= %.3e", context, idx, math.Abs(vec[idx]), dropTol)
		}
	}
	for i, v := range vec {
		if math.Abs(v) > dropTol && !seen[i] {
			t.Fatalf("%s: nonzero vec[%d]=%.3e missing from support list", context, i, v)
		}
	}
}

func factorizeDenseMatrix(t *testing.T, lu *LUFactorization, mat [][]float64) {
	t.Helper()
	m := len(mat)
	err := lu.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		for r := range m {
			if v := mat[r][col]; v != 0 {
				rows = append(rows, r)
				vals = append(vals, v)
			}
		}
		return rows, vals
	})
	if err != nil {
		t.Fatalf("FactorizeSparse failed: %v", err)
	}
}

func verifyAllSolvesAgainstBasis(t *testing.T, lu *LUFactorization, mat [][]float64, rng *rand.Rand, step int) {
	t.Helper()
	m := len(mat)
	tol := 1e-11

	// Sparse RHS FTRAN checks across FTRAN, FTRANExact, FTRANSparse, FTRANSparseHint, and FTRANSparseFrom.
	bOrig := make([]float64, m)
	inIdx := make([]int, 0, 4)
	seen := make([]bool, m)
	nSeed := min(3, m)
	for range nSeed {
		idx := rng.IntN(m)
		v := (rng.Float64() - 0.5) * 4.0
		if math.Abs(v) < 0.25 {
			v = 1.25
		}
		bOrig[idx] = v
		if !seen[idx] {
			seen[idx] = true
			inIdx = append(inIdx, idx)
		}
	}

	xExact := append([]float64(nil), bOrig...)
	lu.FTRANExact(xExact)
	assertWorkClean(t, lu, "FTRANExact")
	resExact := maxAbsDiff(denseMatVec(mat, xExact), bOrig)
	scaleX := max(1.0, maxAbsNorm(xExact))
	if resExact > tol*scaleX {
		t.Fatalf("step %d: FTRANExact residual %.3e exceeds %.3e", step, resExact, tol*scaleX)
	}

	xDense := append([]float64(nil), bOrig...)
	lu.FTRAN(xDense)
	assertWorkClean(t, lu, "FTRAN")
	if d := maxAbsDiff(xDense, xExact); d > tol*scaleX {
		t.Fatalf("step %d: FTRAN vs FTRANExact diff %.3e", step, d)
	}

	xSparse := append([]float64(nil), bOrig...)
	sIdx := lu.FTRANSparse(xSparse, nil)
	assertWorkClean(t, lu, "FTRANSparse")
	assertSparseSupportUnique(t, xSparse, sIdx, luSolveDropTol, "FTRANSparse")
	if d := maxAbsDiff(xSparse, xExact); d > tol*scaleX {
		t.Fatalf("step %d: FTRANSparse vs FTRANExact diff %.3e", step, d)
	}

	xHint := append([]float64(nil), bOrig...)
	hIdx := lu.FTRANSparseHint(xHint, inIdx, nil)
	assertWorkClean(t, lu, "FTRANSparseHint")
	assertSparseSupportUnique(t, xHint, hIdx, luSolveDropTol, "FTRANSparseHint")
	if d := maxAbsDiff(xHint, xExact); d > tol*scaleX {
		t.Fatalf("step %d: FTRANSparseHint vs FTRANExact diff %.3e", step, d)
	}

	srcCopy := append([]float64(nil), bOrig...)
	xFrom := make([]float64, m)
	fromIdx := lu.FTRANSparseFrom(xFrom, srcCopy, inIdx, nil)
	assertWorkClean(t, lu, "FTRANSparseFrom")
	assertSparseSupportUnique(t, xFrom, fromIdx, luSolveDropTol, "FTRANSparseFrom")
	if d := maxAbsDiff(srcCopy, bOrig); d != 0 {
		t.Fatalf("step %d: FTRANSparseFrom mutated src (diff %.3e)", step, d)
	}
	if d := maxAbsDiff(xFrom, xExact); d > tol*scaleX {
		t.Fatalf("step %d: FTRANSparseFrom vs FTRANExact diff %.3e", step, d)
	}

	// Sparse unit-vector BTRAN checks across BTRAN, BTRANExact, BTRANSparse, and BTRANSparseHint.
	cOrig := make([]float64, m)
	piv := rng.IntN(m)
	cOrig[piv] = 1.0
	yExact := append([]float64(nil), cOrig...)
	lu.BTRANExact(yExact)
	assertWorkClean(t, lu, "BTRANExact")
	resB := maxAbsDiff(denseMatTVec(mat, yExact), cOrig)
	scaleY := max(1.0, maxAbsNorm(yExact))
	if resB > tol*scaleY {
		t.Fatalf("step %d: BTRANExact residual %.3e exceeds %.3e", step, resB, tol*scaleY)
	}

	yDense := append([]float64(nil), cOrig...)
	lu.BTRAN(yDense)
	assertWorkClean(t, lu, "BTRAN")
	if d := maxAbsDiff(yDense, yExact); d > tol*scaleY {
		t.Fatalf("step %d: BTRAN vs BTRANExact diff %.3e", step, d)
	}

	ySparse := append([]float64(nil), cOrig...)
	bSparseIdx := lu.BTRANSparse(ySparse, nil)
	assertWorkClean(t, lu, "BTRANSparse")
	assertSparseSupportUnique(t, ySparse, bSparseIdx, luDropTol, "BTRANSparse")
	if d := maxAbsDiff(ySparse, yExact); d > tol*scaleY {
		t.Fatalf("step %d: BTRANSparse vs BTRANExact diff %.3e", step, d)
	}

	yHint := append([]float64(nil), cOrig...)
	bHintIdx := lu.BTRANSparseHint(yHint, []int{piv}, nil)
	assertWorkClean(t, lu, "BTRANSparseHint")
	assertSparseSupportUnique(t, yHint, bHintIdx, luDropTol, "BTRANSparseHint")
	if d := maxAbsDiff(yHint, yExact); d > tol*scaleY {
		t.Fatalf("step %d: BTRANSparseHint vs BTRANExact diff %.3e", step, d)
	}
}

func buildFullNucleusMatrix(m int, rng *rand.Rand) [][]float64 {
	mat := make([][]float64, m)
	for r := range m {
		mat[r] = make([]float64, m)
	}
	for c := range m {
		mat[c][c] = 5.0 + rng.Float64()*2.0
		mat[(c+1)%m][c] = (rng.Float64() - 0.5) * 1.4
		mat[(c+5)%m][c] = (rng.Float64() - 0.5) * 1.4
		for range 2 {
			r := rng.IntN(m)
			if r != c {
				mat[r][c] = (rng.Float64() - 0.5) * 1.1
			}
		}
	}
	return mat
}

func buildSingletonHeavyMatrix(m int, rng *rand.Rand) [][]float64 {
	mat := make([][]float64, m)
	for r := range m {
		mat[r] = make([]float64, m)
	}
	nSing := (m * 7) / 10
	for c := range nSing {
		mat[c][c] = 1.0
	}
	for c := nSing; c < m; c++ {
		mat[c][c] = 4.5 + rng.Float64()
		for k := range 3 {
			r := nSing + (c-nSing+k+1)%(m-nSing)
			mat[r][c] = (rng.Float64() - 0.5) * 1.2
		}
		for range 2 {
			r := rng.IntN(nSing)
			mat[r][c] = (rng.Float64() - 0.5) * 1.5
		}
	}
	return mat
}

func buildHypersparseBlockMatrix(m int, rng *rand.Rand) [][]float64 {
	mat := make([][]float64, m)
	for r := range m {
		mat[r] = make([]float64, m)
	}
	const block = 4
	for base := 0; base < m; base += block {
		end := min(base+block, m)
		for c := base; c < end; c++ {
			mat[c][c] = 4.0 + rng.Float64()
			if c+1 < end {
				mat[c+1][c] = 0.5 + rng.Float64()*0.3
			}
			if c > base {
				mat[c-1][c] = -0.4 - rng.Float64()*0.2
			}
		}
	}
	return mat
}

// TestLUPFIRangeSkippedBTRANAndLongUpdateChain verifies exact forward and transpose
// residuals across up to 56 consecutive pointer-free PFI updates, including repeated pivots,
// disjoint-range BTRAN skipping, singleton-heavy bases, pure diagonal bases, and
// hypersparse block bases.
func TestLUPFIRangeSkippedBTRANAndLongUpdateChain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		m         int
		updates   int
		repeatHot bool
		build     func(m int, rng *rand.Rand) [][]float64
	}{
		{
			name:      "full_nucleus_56_updates_repeated_pivots",
			m:         48,
			updates:   56,
			repeatHot: true,
			build:     buildFullNucleusMatrix,
		},
		{
			name:      "singleton_heavy_56_updates",
			m:         80,
			updates:   56,
			repeatHot: true,
			build:     buildSingletonHeavyMatrix,
		},
		{
			name:      "pure_diagonal_56_updates",
			m:         64,
			updates:   56,
			repeatHot: true,
			build: func(m int, _ *rand.Rand) [][]float64 {
				mat := make([][]float64, m)
				for i := range m {
					mat[i] = make([]float64, m)
					mat[i][i] = 2.0 + float64(i%5)*0.25
				}
				return mat
			},
		},
		{
			name:      "hypersparse_disjoint_range_btran_56_updates",
			m:         192,
			updates:   56,
			repeatHot: false,
			build:     buildHypersparseBlockMatrix,
		},
	}

	for idx, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewPCG(101+uint64(idx), 709))
			mat := tc.build(tc.m, rng)
			lu := &LUFactorization{}
			factorizeDenseMatrix(t, lu, mat)

			if lu.NeedsRefactor() {
				t.Fatalf("fresh factorization reported NeedsRefactor() == true")
			}
			verifyAllSolvesAgainstBasis(t, lu, mat, rng, 0)

			m := tc.m
			colVec := make([]float64, m)
			colIdx := make([]int, 0, 8)
			hotCols := []int{0, 1, m / 3, m - 1}

			for step := 1; step <= tc.updates; step++ {
				var pivotRow int
				if tc.repeatHot && step <= 24 {
					pivotRow = hotCols[step%len(hotCols)]
				} else if tc.name == "hypersparse_disjoint_range_btran_56_updates" {
					// Place updates across strictly disjoint index blocks so BTRAN range skipping fires.
					pivotRow = ((step - 1) * 4) % m
				} else {
					pivotRow = rng.IntN(m)
				}

				clear(colVec)
				colIdx = colIdx[:0]
				if tc.name == "hypersparse_disjoint_range_btran_56_updates" {
					base := (pivotRow / 4) * 4
					end := min(base+4, m)
					for r := base; r < end; r++ {
						if r == pivotRow {
							colVec[r] = 3.5 + rng.Float64()
						} else {
							colVec[r] = (rng.Float64() - 0.5) * 0.6
						}
						colIdx = append(colIdx, r)
					}
				} else {
					colVec[pivotRow] = 3.5 + rng.Float64()*2.0
					colIdx = append(colIdx, pivotRow)
					for range 3 {
						r := rng.IntN(m)
						if colVec[r] == 0 {
							colVec[r] = (rng.Float64() - 0.5) * 1.2
							colIdx = append(colIdx, r)
						}
					}
				}
				aOrig := append([]float64(nil), colVec...)

				var pushErr error
				if step%2 == 0 {
					fIdx := lu.FTRANSparseHint(colVec, colIdx, nil)
					pushErr = lu.PushEtaSparse(pivotRow, colVec, fIdx)
				} else {
					lu.FTRANExact(colVec)
					pushErr = lu.PushEta(pivotRow, colVec)
				}
				assertWorkClean(t, lu, "PushEta")
				if pushErr != nil {
					t.Fatalf("step %d: PushEta failed: %v", step, pushErr)
				}
				for r := range m {
					mat[r][pivotRow] = aOrig[r]
				}

				// Verify bounding box metadata on newly pushed eta.
				eta := lu.etas[len(lu.etas)-1]
				if eta.start < eta.end {
					for p := eta.start; p < eta.end; p++ {
						if lu.etaIdx[p] < eta.minIdx || lu.etaIdx[p] > eta.maxIdx {
							t.Fatalf("step %d: etaIdx[%d]=%d outside [%d, %d]", step, p, lu.etaIdx[p], eta.minIdx, eta.maxIdx)
						}
					}
				}

				if step <= 8 || step%8 == 0 || step == tc.updates {
					verifyAllSolvesAgainstBasis(t, lu, mat, rng, step)
				}
			}
			if !lu.NeedsRefactor() {
				t.Fatalf("expected NeedsRefactor() == true after %d updates (maxEtaUpdates=%d)", len(lu.etas), maxEtaUpdates)
			}
		})
	}
}

// TestLUFTRANSparseFromAndAliasedHints checks out-of-place FTRANSparseFrom (preserving src,
// handling duplicate srcIdx, dense fallback, and nil hints) and aliased inIdx/outIdx across
// FTRANSparseHint, FTRANSparseFrom, and BTRANSparseHint in both hypersparse and dense modes.
func TestLUFTRANSparseFromAndAliasedHints(t *testing.T) {
	t.Parallel()
	const m = 96
	rng := rand.New(rand.NewPCG(77, 99))
	mat := buildFullNucleusMatrix(m, rng)
	lu := &LUFactorization{}
	factorizeDenseMatrix(t, lu, mat)

	// Push 12 PFI updates so etas are active.
	for step := range 12 {
		piv := (step * 7) % m
		a := make([]float64, m)
		a[piv] = 4.0 + float64(step)*0.3
		a[(piv+2)%m] = -0.75
		aOrig := append([]float64(nil), a...)
		fIdx := lu.FTRANSparseHint(a, []int{piv, (piv + 2) % m}, nil)
		// Deliberately duplicate an index in fColIdx to test deduplication in PushEtaSparse.
		if len(fIdx) > 0 {
			fIdx = append(fIdx, fIdx[0])
		}
		if err := lu.PushEtaSparse(piv, a, fIdx); err != nil {
			t.Fatalf("step %d: PushEtaSparse failed: %v", step, err)
		}
		for r := range m {
			mat[r][piv] = aOrig[r]
		}
	}

	for _, ema := range []float64{0.01, 0.50} {
		lu.ftranEMA = ema
		lu.btranEMA = ema

		// Sparse src with duplicate srcIdx and aliased srcIdx/dstIdx.
		src := make([]float64, m)
		src[0] = 1.5
		src[13] = -2.25
		src[47] = 0.75
		srcOrig := append([]float64(nil), src...)

		xRef := append([]float64(nil), src...)
		lu.FTRANExact(xRef)

		dst := make([]float64, m)
		aliasIdx := make([]int, 0, m)
		aliasIdx = append(aliasIdx, 0, 13, 0, 47, 13)
		aliasIdx = lu.FTRANSparseFrom(dst, src, aliasIdx, aliasIdx)
		assertWorkClean(t, lu, "FTRANSparseFrom")
		assertSparseSupportUnique(t, dst, aliasIdx, luSolveDropTol, "FTRANSparseFrom")
		if maxAbsDiff(src, srcOrig) != 0 {
			t.Fatalf("ema=%.2f: FTRANSparseFrom mutated src", ema)
		}
		if d := maxAbsDiff(dst, xRef); d > 1e-11 {
			t.Fatalf("ema=%.2f: FTRANSparseFrom vs FTRANExact diff %.3e", ema, d)
		}

		// Nil srcIdx (dense scan fallback of FTRANSparseFrom).
		clear(dst)
		nilIdx := lu.FTRANSparseFrom(dst, src, nil, nil)
		assertSparseSupportUnique(t, dst, nilIdx, luSolveDropTol, "FTRANSparseFrom-nil")
		if maxAbsDiff(src, srcOrig) != 0 {
			t.Fatalf("ema=%.2f: FTRANSparseFrom(nil) mutated src", ema)
		}
		if d := maxAbsDiff(dst, xRef); d > 1e-11 {
			t.Fatalf("ema=%.2f: FTRANSparseFrom(nil) vs FTRANExact diff %.3e", ema, d)
		}

		// Aliased FTRANSparseHint.
		x := append([]float64(nil), src...)
		aliasIdx = append(aliasIdx[:0], 0, 13, 47)
		aliasIdx = lu.FTRANSparseHint(x, aliasIdx, aliasIdx)
		assertSparseSupportUnique(t, x, aliasIdx, luSolveDropTol, "alias-FTRANSparseHint")
		if d := maxAbsDiff(x, xRef); d > 1e-11 {
			t.Fatalf("ema=%.2f: aliased FTRANSparseHint diff %.3e", ema, d)
		}

		// Aliased BTRANSparseHint.
		y := make([]float64, m)
		y[7] = 1.0
		y[29] = -0.5
		yRef := append([]float64(nil), y...)
		lu.BTRANExact(yRef)
		aliasIdx = append(aliasIdx[:0], 7, 29, 7)
		aliasIdx = lu.BTRANSparseHint(y, aliasIdx, aliasIdx)
		assertSparseSupportUnique(t, y, aliasIdx, luDropTol, "alias-BTRANSparseHint")
		if d := maxAbsDiff(y, yRef); d > 1e-11 {
			t.Fatalf("ema=%.2f: aliased BTRANSparseHint diff %.3e", ema, d)
		}
	}
}

// TestLUGilbertPeierlsHypersparseAndDenseFallbackEquivalence verifies identical numerical
// results across hypersparse Gilbert-Peierls reach DFS, mid-DFS abort fallback, and
// EMA-gated dense sweeps for both FTRAN and BTRAN with and without PFI etas.
func TestLUGilbertPeierlsHypersparseAndDenseFallbackEquivalence(t *testing.T) {
	t.Parallel()

	const mHyper = 160
	rng := rand.New(rand.NewPCG(313, 521))
	matH := buildHypersparseBlockMatrix(mHyper, rng)
	luH := &LUFactorization{}
	factorizeDenseMatrix(t, luH, matH)

	checkModes := func(label string, lu *LUFactorization, mat [][]float64, seedRow int) {
		t.Helper()
		m := len(mat)

		lu.ftranEMA = 0.01
		fHyper := make([]float64, m)
		fHyper[seedRow] = 2.5
		fHyperIdx := lu.FTRANSparseHint(fHyper, []int{seedRow}, nil)
		assertWorkClean(t, lu, label+"/FTRAN-hyper")
		assertSparseSupportUnique(t, fHyper, fHyperIdx, luSolveDropTol, label+"/FTRAN-hyper")

		lu.ftranEMA = 0.50
		fDense := make([]float64, m)
		fDense[seedRow] = 2.5
		fDenseIdx := lu.FTRANSparseHint(fDense, []int{seedRow}, nil)
		assertWorkClean(t, lu, label+"/FTRAN-dense")
		assertSparseSupportUnique(t, fDense, fDenseIdx, luSolveDropTol, label+"/FTRAN-dense")

		fExact := make([]float64, m)
		fExact[seedRow] = 2.5
		lu.FTRANExact(fExact)
		assertWorkClean(t, lu, label+"/FTRAN-exact")

		if d := maxAbsDiff(fHyper, fDense); d > 1e-12 {
			t.Fatalf("%s: FTRAN hyper vs dense diff %.3e", label, d)
		}
		if d := maxAbsDiff(fHyper, fExact); d > 1e-11 {
			t.Fatalf("%s: FTRAN hyper vs exact diff %.3e", label, d)
		}

		lu.btranEMA = 0.01
		bHyper := make([]float64, m)
		bHyper[seedRow] = 1.0
		bHyperIdx := lu.BTRANSparseHint(bHyper, []int{seedRow}, nil)
		assertWorkClean(t, lu, label+"/BTRAN-hyper")
		assertSparseSupportUnique(t, bHyper, bHyperIdx, luDropTol, label+"/BTRAN-hyper")

		lu.btranEMA = 0.50
		bDense := make([]float64, m)
		bDense[seedRow] = 1.0
		bDenseIdx := lu.BTRANSparseHint(bDense, []int{seedRow}, nil)
		assertWorkClean(t, lu, label+"/BTRAN-dense")
		assertSparseSupportUnique(t, bDense, bDenseIdx, luDropTol, label+"/BTRAN-dense")

		bExact := make([]float64, m)
		bExact[seedRow] = 1.0
		lu.BTRANExact(bExact)
		assertWorkClean(t, lu, label+"/BTRAN-exact")

		if d := maxAbsDiff(bHyper, bDense); d > 1e-12 {
			t.Fatalf("%s: BTRAN hyper vs dense diff %.3e", label, d)
		}
		if d := maxAbsDiff(bHyper, bExact); d > 1e-11 {
			t.Fatalf("%s: BTRAN hyper vs exact diff %.3e", label, d)
		}
	}

	checkModes("hyper_no_eta", luH, matH, 12)

	for step := range 20 {
		piv := (step * 4) % mHyper
		if step >= 12 {
			piv = (step - 12) * 4
		}
		base := (piv / 4) * 4
		col := make([]float64, mHyper)
		colIdx := make([]int, 0, 4)
		for r := base; r < base+4; r++ {
			if r == piv {
				col[r] = 4.2 + float64(step)*0.1
			} else {
				col[r] = 0.35 * math.Pow(-1, float64(r+step))
			}
			colIdx = append(colIdx, r)
		}
		aOrig := append([]float64(nil), col...)
		luH.ftranEMA = 0.01
		fIdx := luH.FTRANSparseHint(col, colIdx, nil)
		if err := luH.PushEtaSparse(piv, col, fIdx); err != nil {
			t.Fatalf("hyper step %d PushEtaSparse failed: %v", step, err)
		}
		for r := range mHyper {
			matH[r][piv] = aOrig[r]
		}
	}
	checkModes("hyper_with_20_pfi", luH, matH, 8)
	checkModes("hyper_with_20_pfi_unpivoted", luH, matH, 100)

	// Strongly coupled dense-reach matrix (m = 128) triggering mid-DFS abort when EMA < 0.08.
	const mDense = 128
	matD := buildFullNucleusMatrix(mDense, rng)
	luD := &LUFactorization{}
	factorizeDenseMatrix(t, luD, matD)
	checkModes("dense_abort_no_eta", luD, matD, 5)

	for step := range 10 {
		piv := (step * 11) % mDense
		col := make([]float64, mDense)
		col[piv] = 5.0
		col[(piv+3)%mDense] = -0.8
		aOrig := append([]float64(nil), col...)
		fIdx := luD.FTRANSparseHint(col, []int{piv, (piv + 3) % mDense}, nil)
		if err := luD.PushEtaSparse(piv, col, fIdx); err != nil {
			t.Fatalf("dense step %d PushEtaSparse failed: %v", step, err)
		}
		for r := range mDense {
			matD[r][piv] = aOrig[r]
		}
	}
	checkModes("dense_abort_with_10_pfi", luD, matD, 17)

	// Verify EMA naturally rises above hyperEMACutoff on repeated dense solves.
	luD.ftranEMA = hyperInitEMA
	luD.btranEMA = hyperInitEMA
	tmp := make([]float64, mDense)
	for i := range 10 {
		clear(tmp)
		tmp[i] = 1.0
		_ = luD.FTRANSparseHint(tmp, []int{i}, nil)
		clear(tmp)
		tmp[i] = 1.0
		_ = luD.BTRANSparseHint(tmp, []int{i}, nil)
	}
	if luD.ftranEMA < hyperEMACutoff {
		t.Fatalf("expected dense ftranEMA >= %.2f after 10 dense solves, got %.4f", hyperEMACutoff, luD.ftranEMA)
	}
	if luD.btranEMA < hyperEMACutoff {
		t.Fatalf("expected dense btranEMA >= %.2f after 10 dense solves, got %.4f", hyperEMACutoff, luD.btranEMA)
	}
}

// TestLUNucColMaxRowSingletonAndDenseBalanceRow models a MAD-style basis containing
// dense group-balance rows with large coefficients alongside unit-scale sparse columns
// that generate nucleus row singletons, verifying nucColMax O(1) acceptance, exact
// column-max fallback, arena compaction, and sub-1e-11 residuals.
func TestLUNucColMaxRowSingletonAndDenseBalanceRow(t *testing.T) {
	t.Parallel()
	const m = 128
	const nDense = 4
	rng := rand.New(rand.NewPCG(6060, 7070))

	mat := make([][]float64, m)
	for r := range m {
		mat[r] = make([]float64, m)
	}

	// Dense group-balance rows 0..3 have large entries on columns 0..3 and +/-1 across remaining columns.
	for r := range nDense {
		mat[r][r] = 1000.0 + float64(r)*100.0
		mat[r][(r+1)%nDense] = 50.0
		for c := nDense; c < m; c++ {
			if (c+r)%2 == 0 {
				mat[r][c] = 1.0
			} else {
				mat[r][c] = -1.0
			}
		}
	}
	// Remaining rows form a coupled sparse block with degree >= 2 initially that cascades
	// into nucleus row singletons as elimination proceeds.
	for c := nDense; c < m; c++ {
		mat[c][c] = 1.0
		next := nDense + (c-nDense+1)%(m-nDense)
		mat[next][c] = 0.5
		if c%5 == 0 {
			mat[nDense+(c-nDense+7)%(m-nDense)][c] = -0.25
		}
	}
	// Couple columns 0..3 into lower rows so no Stage 1/2 peeling removes the dense block.
	for c := range nDense {
		mat[nDense+c][c] = 2.0
		mat[nDense+c+4][c] = -1.5
	}

	lu := &LUFactorization{}
	factorizeDenseMatrix(t, lu, mat)
	if lu.nucStart == m {
		t.Fatalf("expected non-empty Stage 3 nucleus, got nucStart=%d", lu.nucStart)
	}
	verifyAllSolvesAgainstBasis(t, lu, mat, rng, 0)

	// Also verify heavy fill-in dense nucleus compaction (96x96, 10 nnz/col).
	const mFill = 96
	matFill := make([][]float64, mFill)
	for r := range mFill {
		matFill[r] = make([]float64, mFill)
	}
	for c := range mFill {
		matFill[c][c] = 10.0 + rng.Float64()*2.0
		for k := 1; k <= 9; k++ {
			r := (c + k*7 + rng.IntN(3)) % mFill
			if r != c {
				matFill[r][c] = (rng.Float64() - 0.5) * 1.6
			}
		}
	}
	luFill := &LUFactorization{}
	factorizeDenseMatrix(t, luFill, matFill)
	verifyAllSolvesAgainstBasis(t, luFill, matFill, rng, 0)
	factorizeDenseMatrix(t, luFill, matFill)
	verifyAllSolvesAgainstBasis(t, luFill, matFill, rng, 1)
}

// TestLUPFIPivotGuardsAndExtremeExponents verifies tiny/NaN/Inf pivot rejection preserves
// the existing factorization atomically and that FTRANExact/BTRANExact preserve extreme
// 1e-200 right-hand sides across PFI updates.
func TestLUPFIPivotGuardsAndExtremeExponents(t *testing.T) {
	t.Parallel()
	const m = 16
	rng := rand.New(rand.NewPCG(909, 123))
	mat := buildFullNucleusMatrix(m, rng)
	lu := &LUFactorization{}
	factorizeDenseMatrix(t, lu, mat)

	for step := range 3 {
		col := make([]float64, m)
		col[step] = 4.0
		col[(step+1)%m] = 0.5
		aOrig := append([]float64(nil), col...)
		fIdx := lu.FTRANSparseHint(col, []int{step, (step + 1) % m}, nil)
		if err := lu.PushEtaSparse(step, col, fIdx); err != nil {
			t.Fatalf("setup update %d failed: %v", step, err)
		}
		for r := range m {
			mat[r][step] = aOrig[r]
		}
	}
	etasBefore := len(lu.etas)

	badTiny := make([]float64, m)
	badTiny[0] = 1e-13
	badTiny[1] = 1.0
	if err := lu.PushEtaSparse(0, badTiny, []int{0, 1}); err == nil {
		t.Fatal("expected error for tiny pivot 1e-13")
	}
	badNaN := make([]float64, m)
	badNaN[0] = math.NaN()
	if err := lu.PushEta(0, badNaN); err == nil {
		t.Fatal("expected error for NaN pivot")
	}
	badInf := make([]float64, m)
	badInf[0] = math.Inf(1)
	if err := lu.PushEta(0, badInf); err == nil {
		t.Fatal("expected error for Inf pivot")
	}
	if len(lu.etas) != etasBefore {
		t.Fatalf("rejected updates mutated eta count: got %d, want %d", len(lu.etas), etasBefore)
	}
	verifyAllSolvesAgainstBasis(t, lu, mat, rng, 99)

	// Extreme-exponent exact solves (1e-200 RHS) preserved across PFI updates.
	luSub := &LUFactorization{}
	matSub := [][]float64{
		{2.0, 1.0, 0.0},
		{0.0, 2.0, 1.0},
		{0.0, 0.0, 2.0},
	}
	factorizeDenseMatrix(t, luSub, matSub)
	aSub := []float64{2.0, 0.5, 0.25}
	fSub := append([]float64(nil), aSub...)
	luSub.FTRANExact(fSub)
	if err := luSub.PushEta(0, fSub); err != nil {
		t.Fatalf("subnormal setup PushEta failed: %v", err)
	}
	for r := range 3 {
		matSub[r][0] = aSub[r]
	}

	wantX := []float64{1e-200, -2e-200, 3e-200}
	rhsSub := denseMatVec(matSub, wantX)
	gotX := append([]float64(nil), rhsSub...)
	luSub.FTRANExact(gotX)
	for i := range 3 {
		if math.Abs(gotX[i]-wantX[i]) > 1e-212 {
			t.Fatalf("FTRANExact 1e-200[%d] = %.15e, want %.15e", i, gotX[i], wantX[i])
		}
	}

	wantY := []float64{-1e-200, 3e-200, -2e-200}
	cSub := denseMatTVec(matSub, wantY)
	gotY := append([]float64(nil), cSub...)
	luSub.BTRANExact(gotY)
	for i := range 3 {
		if math.Abs(gotY[i]-wantY[i]) > 1e-212 {
			t.Fatalf("BTRANExact 1e-200[%d] = %.15e, want %.15e", i, gotY[i], wantY[i])
		}
	}
}

// TestLUSteadyStateZeroAllocations verifies zero heap allocations during steady-state
// FactorizeSparse, FTRANSparseHint (hypersparse and dense), FTRANSparseFrom,
// BTRANSparseHint (hypersparse and dense), and PushEtaSparse after warmup.
func TestLUSteadyStateZeroAllocations(t *testing.T) {
	const m = 128
	rng := rand.New(rand.NewPCG(4242, 8484))
	mat := buildSingletonHeavyMatrix(m, rng)

	colPtr := make([]int, m+1)
	var rowIdx []int
	var colVal []float64
	for c := range m {
		colPtr[c] = len(rowIdx)
		for r := range m {
			if v := mat[r][c]; v != 0 {
				rowIdx = append(rowIdx, r)
				colVal = append(colVal, v)
			}
		}
	}
	colPtr[m] = len(rowIdx)

	appendCol := func(col int, rows []int, vals []float64) ([]int, []float64) {
		for p := colPtr[col]; p < colPtr[col+1]; p++ {
			rows = append(rows, rowIdx[p])
			vals = append(vals, colVal[p])
		}
		return rows, vals
	}

	lu := newLUFactorization(m)
	for range 3 {
		if err := lu.FactorizeSparse(m, appendCol); err != nil {
			t.Fatalf("warmup FactorizeSparse failed: %v", err)
		}
	}
	if lu.nucStart == m {
		t.Fatalf("expected non-empty Stage 3 nucleus for allocation test, got nucStart=%d", lu.nucStart)
	}

	xBuf := make([]float64, m)
	srcBuf := make([]float64, m)
	dstBuf := make([]float64, m)
	bBuf := make([]float64, m)
	idxBuf := make([]int, 0, m)
	inHint := []int{0, 1, m - 1}
	unitHint := []int{m - 2}

	pivHints := make([][2]int, 16)
	for step := range 16 {
		piv := (step * 5) % m
		pivHints[step] = [2]int{piv, (piv + 1) % m}
		clear(xBuf)
		xBuf[piv] = 4.0
		xBuf[(piv+1)%m] = 0.5
		idxBuf = lu.FTRANSparseHint(xBuf, pivHints[step][:], idxBuf)
		if err := lu.PushEtaSparse(piv, xBuf, idxBuf); err != nil {
			t.Fatalf("warmup PushEtaSparse %d failed: %v", step, err)
		}
	}
	if err := lu.FactorizeSparse(m, appendCol); err != nil {
		t.Fatalf("post-warmup FactorizeSparse failed: %v", err)
	}

	factorAllocs := testing.AllocsPerRun(50, func() {
		if err := lu.FactorizeSparse(m, appendCol); err != nil {
			t.Fatalf("FactorizeSparse failed: %v", err)
		}
	})
	t.Logf("steady-state FactorizeSparse allocs/op: %.1f (m=%d, nNuc=%d)", factorAllocs, m, m-lu.nucStart)
	if factorAllocs != 0 {
		t.Fatalf("steady-state FactorizeSparse allocated %.1f times/op, want 0", factorAllocs)
	}

	// Push 8 PFI updates so steady-state solves exercise active etas.
	for step := range 8 {
		piv := pivHints[step][0]
		clear(xBuf)
		xBuf[piv] = 4.0
		xBuf[pivHints[step][1]] = 0.5
		idxBuf = lu.FTRANSparseHint(xBuf, pivHints[step][:], idxBuf)
		if err := lu.PushEtaSparse(piv, xBuf, idxBuf); err != nil {
			t.Fatalf("setup PushEtaSparse %d failed: %v", step, err)
		}
	}

	lu.ftranEMA = 0.01
	ftranHyperAllocs := testing.AllocsPerRun(100, func() {
		xBuf[0] = 1.0
		xBuf[1] = -0.5
		xBuf[m-1] = 0.75
		idxBuf = lu.FTRANSparseHint(xBuf, inHint, idxBuf)
		for _, i := range idxBuf {
			xBuf[i] = 0
		}
	})
	t.Logf("steady-state FTRANSparseHint (hyper) allocs/op: %.1f", ftranHyperAllocs)
	if ftranHyperAllocs != 0 {
		t.Fatalf("steady-state FTRANSparseHint (hyper) allocated %.1f times/op, want 0", ftranHyperAllocs)
	}

	lu.ftranEMA = 0.50
	ftranDenseAllocs := testing.AllocsPerRun(100, func() {
		xBuf[0] = 1.0
		xBuf[1] = -0.5
		xBuf[m-1] = 0.75
		idxBuf = lu.FTRANSparseHint(xBuf, inHint, idxBuf)
		for _, i := range idxBuf {
			xBuf[i] = 0
		}
	})
	t.Logf("steady-state FTRANSparseHint (dense) allocs/op: %.1f", ftranDenseAllocs)
	if ftranDenseAllocs != 0 {
		t.Fatalf("steady-state FTRANSparseHint (dense) allocated %.1f times/op, want 0", ftranDenseAllocs)
	}

	srcBuf[0] = 1.0
	srcBuf[1] = -0.5
	srcBuf[m-1] = 0.75
	ftranFromAllocs := testing.AllocsPerRun(100, func() {
		idxBuf = lu.FTRANSparseFrom(dstBuf, srcBuf, inHint, idxBuf)
		for _, i := range idxBuf {
			dstBuf[i] = 0
		}
	})
	t.Logf("steady-state FTRANSparseFrom allocs/op: %.1f", ftranFromAllocs)
	if ftranFromAllocs != 0 {
		t.Fatalf("steady-state FTRANSparseFrom allocated %.1f times/op, want 0", ftranFromAllocs)
	}

	lu.btranEMA = 0.01
	btranHyperAllocs := testing.AllocsPerRun(100, func() {
		bBuf[m-2] = 1.0
		idxBuf = lu.BTRANSparseHint(bBuf, unitHint, idxBuf)
		for _, i := range idxBuf {
			bBuf[i] = 0
		}
	})
	t.Logf("steady-state BTRANSparseHint (hyper) allocs/op: %.1f", btranHyperAllocs)
	if btranHyperAllocs != 0 {
		t.Fatalf("steady-state BTRANSparseHint (hyper) allocated %.1f times/op, want 0", btranHyperAllocs)
	}

	lu.btranEMA = 0.50
	btranDenseAllocs := testing.AllocsPerRun(100, func() {
		bBuf[m-2] = 1.0
		idxBuf = lu.BTRANSparseHint(bBuf, unitHint, idxBuf)
		for _, i := range idxBuf {
			bBuf[i] = 0
		}
	})
	t.Logf("steady-state BTRANSparseHint (dense) allocs/op: %.1f", btranDenseAllocs)
	if btranDenseAllocs != 0 {
		t.Fatalf("steady-state BTRANSparseHint (dense) allocated %.1f times/op, want 0", btranDenseAllocs)
	}

	pushCycleAllocs := testing.AllocsPerRun(25, func() {
		if err := lu.FactorizeSparse(m, appendCol); err != nil {
			t.Fatalf("FactorizeSparse failed: %v", err)
		}
		for step := range 16 {
			piv := pivHints[step][0]
			other := pivHints[step][1]
			xBuf[piv] = 4.0
			xBuf[other] = 0.5
			idxBuf = lu.FTRANSparseHint(xBuf, pivHints[step][:], idxBuf)
			if err := lu.PushEtaSparse(piv, xBuf, idxBuf); err != nil {
				t.Fatalf("PushEtaSparse %d failed: %v", step, err)
			}
			for _, i := range idxBuf {
				xBuf[i] = 0
			}
		}
	})
	t.Logf("steady-state Factorize + 16x(FTRANSparseHint + PushEtaSparse) allocs/op: %.1f", pushCycleAllocs)
	if pushCycleAllocs != 0 {
		t.Fatalf("steady-state PFI update cycle allocated %.1f times/op, want 0", pushCycleAllocs)
	}
}
