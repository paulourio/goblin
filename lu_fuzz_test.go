package goblin

import (
	"fmt"
	"math"
	"math/big"
	"testing"
)

type luByteCursor struct {
	data []byte
	pos  int
}

func (c *luByteCursor) u8() uint8 {
	if c.pos >= len(c.data) {
		return 0
	}
	b := c.data[c.pos]
	c.pos++
	return b
}

func (c *luByteCursor) pick(n int) int {
	if n <= 1 {
		return 0
	}
	return int(c.u8()) % n
}

func (c *luByteCursor) boolBit() bool {
	return (c.u8() & 1) == 1
}

func assertLUWorkClean(tb testing.TB, lu *LUFactorization, label string) {
	tb.Helper()
	for i := range lu.m {
		if lu.work[i] != 0 {
			tb.Fatalf("%s: dirty lu.work[%d]=%.3e", label, i, lu.work[i])
		}
		if lu.spikeWork[i] != 0 {
			tb.Fatalf("%s: dirty lu.spikeWork[%d]=%.3e", label, i, lu.spikeWork[i])
		}
	}
}

func assertLUSupportExact(tb testing.TB, vec []float64, idxs []int, dropTol float64, label string) {
	tb.Helper()
	seen := make([]bool, len(vec))
	for _, idx := range idxs {
		if idx < 0 || idx >= len(vec) {
			tb.Fatalf("%s: out-of-range index %d (m=%d)", label, idx, len(vec))
		}
		if seen[idx] {
			tb.Fatalf("%s: duplicate support index %d", label, idx)
		}
		seen[idx] = true
		if math.Abs(vec[idx]) <= dropTol {
			tb.Fatalf("%s: returned index %d has |val|=%.3e <= %.3e", label, idx, math.Abs(vec[idx]), dropTol)
		}
	}
	for i, v := range vec {
		if math.Abs(v) > dropTol && !seen[i] {
			tb.Fatalf("%s: nonzero entry [%d]=%.3e missing from support indices", label, i, v)
		}
	}
}

// exactRatSolvePair solves B * x = rhsF and B^T * y = rhsB over Q using sparse-skipping
// partial-pivoted Gaussian elimination on the dense float64 matrix mat.
func exactRatSolvePair(mat [][]float64, rhsF, rhsB []float64) (xFloat, yFloat []float64, minPivot, matInf float64, ok bool) {
	m := len(mat)
	if m == 0 || len(rhsF) != m || len(rhsB) != m {
		return nil, nil, 0, 0, false
	}
	lu := make([][]*big.Rat, m)
	b := make([]*big.Rat, m)
	c := make([]*big.Rat, m)
	rowPerm := make([]int, m)
	for i := range m {
		rowPerm[i] = i
		lu[i] = make([]*big.Rat, m)
		rowSum := 0.0
		for j := range m {
			v := mat[i][j]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, nil, 0, 0, false
			}
			rowSum += math.Abs(v)
			r := new(big.Rat).SetFloat64(v)
			if r == nil {
				return nil, nil, 0, 0, false
			}
			lu[i][j] = r
		}
		if rowSum > matInf {
			matInf = rowSum
		}
		br := new(big.Rat).SetFloat64(rhsF[i])
		cr := new(big.Rat).SetFloat64(rhsB[i])
		if br == nil || cr == nil {
			return nil, nil, 0, 0, false
		}
		b[i] = br
		c[i] = cr
	}

	minPivot = math.Inf(1)
	tmp := new(big.Rat)
	for k := range m {
		pivRow := -1
		bestAbs := -1.0
		for r := k; r < m; r++ {
			if lu[r][k].Sign() == 0 {
				continue
			}
			fv, _ := lu[r][k].Float64()
			av := math.Abs(fv)
			if pivRow == -1 || av > bestAbs {
				pivRow = r
				bestAbs = av
			}
		}
		if pivRow == -1 || bestAbs == 0 {
			return nil, nil, 0, matInf, false
		}
		if bestAbs < minPivot {
			minPivot = bestAbs
		}
		if pivRow != k {
			lu[k], lu[pivRow] = lu[pivRow], lu[k]
			rowPerm[k], rowPerm[pivRow] = rowPerm[pivRow], rowPerm[k]
		}
		piv := lu[k][k]
		rowK := lu[k]
		for r := k + 1; r < m; r++ {
			rowR := lu[r]
			if rowR[k].Sign() == 0 {
				continue
			}
			mult := new(big.Rat).Quo(rowR[k], piv)
			rowR[k] = mult
			for col := k + 1; col < m; col++ {
				if rowK[col].Sign() == 0 {
					continue
				}
				tmp.Mul(mult, rowK[col])
				rowR[col].Sub(rowR[col], tmp)
			}
		}
	}

	// Forward solve: B * x = rhsF via L * z = P * rhsF, U * x = z.
	xRat := make([]*big.Rat, m)
	for i := range m {
		xRat[i] = new(big.Rat).Set(b[rowPerm[i]])
	}
	for i := range m {
		xi := xRat[i]
		for k := range i {
			if lu[i][k].Sign() == 0 || xRat[k].Sign() == 0 {
				continue
			}
			tmp.Mul(lu[i][k], xRat[k])
			xi.Sub(xi, tmp)
		}
	}
	for i := m - 1; i >= 0; i-- {
		xi := xRat[i]
		rowI := lu[i]
		for col := i + 1; col < m; col++ {
			if rowI[col].Sign() == 0 || xRat[col].Sign() == 0 {
				continue
			}
			tmp.Mul(rowI[col], xRat[col])
			xi.Sub(xi, tmp)
		}
		xi.Quo(xi, rowI[i])
	}

	// Transpose solve: B^T * y = rhsB via U^T * w = rhsB, L^T * u = w, y = P^T * u.
	wRat := make([]*big.Rat, m)
	for i := range m {
		wRat[i] = new(big.Rat).Set(c[i])
	}
	for i := range m {
		wi := wRat[i]
		for k := range i {
			if lu[k][i].Sign() == 0 || wRat[k].Sign() == 0 {
				continue
			}
			tmp.Mul(lu[k][i], wRat[k])
			wi.Sub(wi, tmp)
		}
		wi.Quo(wi, lu[i][i])
	}
	for i := m - 1; i >= 0; i-- {
		wi := wRat[i]
		for r := i + 1; r < m; r++ {
			if lu[r][i].Sign() == 0 || wRat[r].Sign() == 0 {
				continue
			}
			tmp.Mul(lu[r][i], wRat[r])
			wi.Sub(wi, tmp)
		}
	}
	yRat := make([]*big.Rat, m)
	for i := range m {
		yRat[rowPerm[i]] = wRat[i]
	}

	xFloat = make([]float64, m)
	yFloat = make([]float64, m)
	for i := range m {
		xf, _ := xRat[i].Float64()
		yf, _ := yRat[i].Float64()
		if math.IsNaN(xf) || math.IsInf(xf, 0) || math.IsNaN(yf) || math.IsInf(yf, 0) {
			return nil, nil, 0, matInf, false
		}
		xFloat[i] = xf
		yFloat[i] = yf
	}
	return xFloat, yFloat, minPivot, matInf, true
}

func buildFuzzSparseBasis(c *luByteCursor, m, mode int) [][]float64 {
	mat := make([][]float64, m)
	for r := range m {
		mat[r] = make([]float64, m)
	}
	offVals := []float64{-1.0, -0.5, -0.25, 0.25, 0.5, 1.0, -0.125, 0.125}

	switch mode {
	case 0:
		// Diagonally dominant sparse nucleus (Stage 3 Markowitz) with decoded swaps.
		for col := range m {
			diag := 4.0 + float64(c.u8()&7)*0.5
			if c.boolBit() {
				diag = -diag
			}
			mat[col][col] = diag
			nOff := 2 + c.pick(3)
			for range nOff {
				r := c.pick(m)
				if r != col {
					mat[r][col] = offVals[c.pick(len(offVals))]
				}
			}
		}
	case 1:
		// Stage 1 and Stage 2 singleton prefix followed by coupled nucleus.
		nSing := max(1, (m*(3+c.pick(5)))/10)
		if nSing >= m {
			nSing = m - 2
		}
		for col := range nSing {
			diag := 1.0
			if c.boolBit() {
				diag = float64(1+c.pick(4)) * 0.5
			}
			mat[col][col] = diag
		}
		for col := nSing; col < m; col++ {
			mat[col][col] = 5.0 + float64(c.u8()&3)
			nucSpan := m - nSing
			for k := range 2 {
				r := nSing + (col-nSing+k+1)%nucSpan
				if r != col {
					mat[r][col] = offVals[c.pick(len(offVals))]
				}
			}
			if c.boolBit() {
				r := c.pick(nSing)
				mat[r][col] = offVals[c.pick(len(offVals))]
			}
		}
	case 2:
		// Block-hypersparse 4x4 diagonal blocks with sparse couplings across hyperMinM=64.
		const blk = 4
		for base := 0; base < m; base += blk {
			end := min(base+blk, m)
			for col := base; col < end; col++ {
				mat[col][col] = 4.0 + float64(c.u8()&3)*0.5
				if col+1 < end {
					mat[col+1][col] = offVals[c.pick(len(offVals))]
				}
				if col > base {
					mat[col-1][col] = offVals[c.pick(len(offVals))]
				}
			}
		}
		if c.boolBit() && m >= 8 {
			links := 1 + c.pick(3)
			for range links {
				r := c.pick(m)
				col := c.pick(m)
				if r != col {
					mat[r][col] = 0.25
				}
			}
		}
	case 3:
		// Dense coupling rows over sparse lower triangular/nucleus columns.
		nDense := min(3, m/4)
		for r := range nDense {
			mat[r][r] = 64.0 + float64(r*16)
			for col := nDense; col < m; col++ {
				if (col+r)%3 == 0 {
					if c.boolBit() {
						mat[r][col] = 0.5
					} else {
						mat[r][col] = -0.5
					}
				}
			}
		}
		for col := nDense; col < m; col++ {
			mat[col][col] = 4.0 + float64(c.u8()&3)*0.5
			next := nDense + (col-nDense+1)%(m-nDense)
			if next != col {
				mat[next][col] = 0.25
			}
		}
	default:
		// Mixed-scale near-boundary basis testing luPivotTol=1e-11 and small-diag U entries.
		for col := range m {
			var diag float64
			switch c.pick(6) {
			case 0:
				diag = 1e-6
			case 1:
				diag = 1e-9
			case 2:
				diag = 2e-11
			default:
				diag = 2.0 + float64(c.u8()&3)*0.5
			}
			if c.boolBit() {
				diag = -diag
			}
			mat[col][col] = diag
			for range 2 {
				r := c.pick(m)
				if r != col {
					mat[r][col] = offVals[c.pick(len(offVals))] * 0.125
				}
			}
		}
	}

	// Optional row permutation while preserving nonsingularity.
	if c.boolBit() {
		swaps := 1 + c.pick(min(m, 8))
		for range swaps {
			r1 := c.pick(m)
			r2 := c.pick(m)
			if r1 != r2 {
				mat[r1], mat[r2] = mat[r2], mat[r1]
			}
		}
	}
	return mat
}

func luVecMaxAbs(v []float64) float64 {
	best := 0.0
	for _, x := range v {
		if ax := math.Abs(x); ax > best {
			best = ax
		}
	}
	return best
}

func luVecMaxDiff(a, b []float64) float64 {
	best := 0.0
	for i := range a {
		if d := math.Abs(a[i] - b[i]); d > best {
			best = d
		}
	}
	return best
}

func verifyLUSolvesVsExact(tb testing.TB, lu *LUFactorization, mat [][]float64, c *luByteCursor, step int) {
	tb.Helper()
	m := len(mat)
	bOrig := make([]float64, m)
	inF := make([]int, 0, 4)
	seenF := make([]bool, m)
	nSeed := 1 + c.pick(min(3, m))
	for range nSeed {
		r := c.pick(m)
		v := []float64{1.0, -1.5, 2.25, -0.5, 0.75}[c.pick(5)]
		bOrig[r] = v
		if !seenF[r] {
			seenF[r] = true
			inF = append(inF, r)
		}
	}

	cOrig := make([]float64, m)
	inB := make([]int, 0, 2)
	bIdx0 := c.pick(m)
	cOrig[bIdx0] = 1.0
	inB = append(inB, bIdx0)
	if c.boolBit() && m > 1 {
		bIdx1 := (bIdx0 + 1) % m
		cOrig[bIdx1] = -0.5
		inB = append(inB, bIdx1)
	}

	xExactRef, yExactRef, minPiv, matInf, okRat := exactRatSolvePair(mat, bOrig, cOrig)
	bInf := luVecMaxAbs(bOrig)
	cInf := luVecMaxAbs(cOrig)
	xRefNorm := 0.0
	yRefNorm := 0.0
	if okRat {
		xRefNorm = luVecMaxAbs(xExactRef)
		yRefNorm = luVecMaxAbs(yExactRef)
	}
	wellConditioned := okRat && !lu.NeedsRefactor() &&
		lu.UpdatePivotsWellConditioned(1e4) &&
		minPiv >= 1e-5 && matInf <= 1e5 &&
		xRefNorm <= 1e5*max(1.0, bInf) &&
		yRefNorm <= 1e5*max(1.0, cInf)

	// Always run dense and sparse solves across EMA settings to exercise all paths.
	emaModes := []float64{0.01}
	if m >= hyperMinM {
		emaModes = []float64{0.01, 0.50}
	}

	fEx := append([]float64(nil), bOrig...)
	lu.FTRANExact(fEx)
	assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/FTRANExact", step))

	fDen := append([]float64(nil), bOrig...)
	lu.FTRAN(fDen)
	assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/FTRAN", step))

	bEx := append([]float64(nil), cOrig...)
	lu.BTRANExact(bEx)
	assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/BTRANExact", step))

	bDen := append([]float64(nil), cOrig...)
	lu.BTRAN(bDen)
	assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/BTRAN", step))

	for _, ema := range emaModes {
		lu.ftranEMA = ema
		fSp := append([]float64(nil), bOrig...)
		fIdx := lu.FTRANSparseHint(fSp, inF, nil)
		assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/FTRANSparseHint/ema=%.2f", step, ema))
		assertLUSupportExact(tb, fSp, fIdx, luSolveDropTol, fmt.Sprintf("step=%d/FTRANSparseHint/ema=%.2f", step, ema))

		lu.btranEMA = ema
		bSp := append([]float64(nil), cOrig...)
		bIdx := lu.BTRANSparseHint(bSp, inB, nil)
		assertLUWorkClean(tb, lu, fmt.Sprintf("step=%d/BTRANSparseHint/ema=%.2f", step, ema))
		assertLUSupportExact(tb, bSp, bIdx, luDropTol, fmt.Sprintf("step=%d/BTRANSparseHint/ema=%.2f", step, ema))

		if wellConditioned {
			tolF := min(1e-5, max(1e-9, 1e-10*max(1.0, 1.0/minPiv)*max(1.0, xRefNorm)))
			tolB := min(1e-5, max(1e-9, 1e-10*max(1.0, 1.0/minPiv)*max(1.0, yRefNorm)))
			if d := luVecMaxDiff(fSp, xExactRef); d > tolF {
				tb.Fatalf("step=%d ema=%.2f: FTRANSparseHint vs big.Rat diff=%.3e > %.3e", step, ema, d, tolF)
			}
			if d := luVecMaxDiff(bSp, yExactRef); d > tolB {
				tb.Fatalf("step=%d ema=%.2f: BTRANSparseHint vs big.Rat diff=%.3e > %.3e", step, ema, d, tolB)
			}
		}
	}

	if !wellConditioned {
		return
	}

	tolF := min(1e-5, max(1e-9, 1e-10*max(1.0, 1.0/minPiv)*max(1.0, xRefNorm)))
	tolB := min(1e-5, max(1e-9, 1e-10*max(1.0, 1.0/minPiv)*max(1.0, yRefNorm)))
	if d := luVecMaxDiff(fEx, xExactRef); d > tolF {
		tb.Fatalf("step=%d: FTRANExact vs big.Rat diff=%.3e > %.3e", step, d, tolF)
	}
	if d := luVecMaxDiff(fDen, xExactRef); d > tolF {
		tb.Fatalf("step=%d: FTRAN vs big.Rat diff=%.3e > %.3e", step, d, tolF)
	}
	if d := luVecMaxDiff(bEx, yExactRef); d > tolB {
		tb.Fatalf("step=%d: BTRANExact vs big.Rat diff=%.3e > %.3e", step, d, tolB)
	}
	if d := luVecMaxDiff(bDen, yExactRef); d > tolB {
		tb.Fatalf("step=%d: BTRAN vs big.Rat diff=%.3e > %.3e", step, d, tolB)
	}

	// Verify FTRANExact and BTRANExact preserve sub-luDropTol scaled RHS entries.
	const tinyScale = 1e-18
	fTiny := make([]float64, m)
	bTiny := make([]float64, m)
	for i := range m {
		fTiny[i] = bOrig[i] * tinyScale
		bTiny[i] = cOrig[i] * tinyScale
	}
	lu.FTRANExact(fTiny)
	lu.BTRANExact(bTiny)
	for i := range m {
		wantF := xExactRef[i] * tinyScale
		if d := math.Abs(fTiny[i] - wantF); d > tolF*tinyScale {
			tb.Fatalf("step=%d: FTRANExact tiny[%d]=%.3e want %.3e (diff=%.3e)", step, i, fTiny[i], wantF, d)
		}
		wantB := yExactRef[i] * tinyScale
		if d := math.Abs(bTiny[i] - wantB); d > tolB*tinyScale {
			tb.Fatalf("step=%d: BTRANExact tiny[%d]=%.3e want %.3e (diff=%.3e)", step, i, bTiny[i], wantB, d)
		}
	}
}

func driveDirectPushFTFromSpikeEdge(tb testing.TB, lu *LUFactorization, mat [][]float64, pivotRow int, edgeKind uint8) {
	tb.Helper()
	m := lu.m
	if m <= 0 || !lu.valid || pivotRow < 0 || pivotRow >= m {
		return
	}
	etasBefore := len(lu.etas)
	updatesBefore := lu.numUpdates
	p := int(lu.invColPerm[pivotRow])

	switch edgeKind % 3 {
	case 0:
		// Tiny spike pivot straddling luPivotTol=1e-11.
		lu.spikeValid = false
		lu.spikeIdx = append(lu.spikeIdx[:0], asInt32(p))
		lu.spikeWork[p] = 5e-12
		if err := lu.pushFTFromSpike(pivotRow); err == nil {
			tb.Fatalf("expected pushFTFromSpike to reject tiny pivot 5e-12 at p=%d", p)
		}
	case 1:
		// Cancellation spike straddling ftCancelTol=1e-8 when row p of U has off-diagonal entries.
		if lu.uRowLen[p] > 0 {
			st := lu.uRowStart[p]
			end := st + lu.uRowLen[p]
			c0 := lu.uColIdx[st]
			v0 := lu.uRowVal[st]
			if end > st && c0 >= 0 && int(c0) < m && v0 != 0 && math.Abs(lu.uDiag[c0]) >= luPivotTol {
				// Choose spike[c0]=lu.uDiag[c0] and spike[p]=v0 so uNew=vp-dotRV cancels when c0 is the sole reached col.
				// Even if deeper rows are reached, verify cleanup and atomicity on rejection or exactness on acceptance.
				lu.spikeValid = false
				lu.spikeIdx = append(lu.spikeIdx[:0], c0, asInt32(p))
				lu.spikeWork[c0] = lu.uDiag[c0]
				lu.spikeWork[p] = v0
				if err := lu.pushFTFromSpike(pivotRow); err == nil {
					// If accepted because deeper U dependencies broke exact cancellation, refactor before continuing.
					assertLUWorkClean(tb, lu, "direct_cancel_accept")
					_ = lu.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
						for r := range m {
							if v := mat[r][col]; v != 0 {
								rows = append(rows, r)
								vals = append(vals, v)
							}
						}
						return rows, vals
					})
					return
				}
			}
		}
	default:
		// Huge row multiplier exceeding ftMaxRowMult=1.5e6 when uRowLen[p] > 0.
		if lu.uRowLen[p] > 0 {
			st := lu.uRowStart[p]
			c0 := lu.uColIdx[st]
			if c0 >= 0 && int(c0) < m {
				savedDiag := lu.uDiag[c0]
				savedInv := lu.uInvDiag[c0]
				lu.uDiag[c0] = 1e-8
				lu.uInvDiag[c0] = 1e8
				lu.spikeValid = false
				lu.spikeIdx = append(lu.spikeIdx[:0], asInt32(p), c0)
				lu.spikeWork[p] = 1.0
				lu.spikeWork[c0] = 0.0
				err := lu.pushFTFromSpike(pivotRow)
				lu.uDiag[c0] = savedDiag
				lu.uInvDiag[c0] = savedInv
				if err == nil {
					tb.Fatalf("expected pushFTFromSpike to reject multiplier > ftMaxRowMult at row p=%d", p)
				}
			}
		}
	}

	assertLUWorkClean(tb, lu, "direct_pushFTFromSpike_reject")
	if len(lu.etas) != etasBefore || lu.numUpdates != updatesBefore {
		tb.Fatalf("rejected pushFTFromSpike mutated state: etas %d->%d updates %d->%d",
			etasBefore, len(lu.etas), updatesBefore, lu.numUpdates)
	}
}

func FuzzLUForrestTomlinVsExact(f *testing.F) {
	// Seeds span m in [4..80] across hyperMinM=64, 1..65 updates across maxEtaUpdates=56,
	// arena compaction, and near-singular / cancellation edges.
	f.Add([]byte{0, 0, 2, 10, 0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{1, 12, 1, 56, 3, 2, 1, 0, 5, 4, 3, 2, 1})
	f.Add([]byte{2, 59, 0, 64, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	f.Add([]byte{2, 60, 1, 64, 2, 4, 6, 8, 10, 12, 14, 16})
	f.Add([]byte{2, 61, 2, 64, 3, 1, 4, 1, 5, 9, 2, 6})
	f.Add([]byte{3, 76, 3, 60, 7, 11, 13, 17, 19, 23, 29, 31})
	f.Add([]byte{4, 20, 4, 30, 0, 2, 4, 6, 8, 10, 12, 14})
	f.Add([]byte{5, 68, 5, 64, 9, 8, 7, 6, 5, 4, 3, 2, 1})

	f.Fuzz(func(t *testing.T, data []byte) {
		c := &luByteCursor{data: data}
		mode := c.pick(5)
		m := 4 + int(c.u8())%77 // [4..80], crossing hyperMinM=64
		if len(data) > 1 && data[1] >= 59 && data[1] <= 63 {
			m = int(data[1]) + 4 // preserve exact boundary hits around 63, 64, 65, 66, 67
		}
		emaInitMode := c.pick(3)
		nUpdates := 1 + int(c.u8())%65 // [1..65], crossing maxEtaUpdates=56

		mat := buildFuzzSparseBasis(c, m, mode)
		lu := newLUFactorization(m)
		switch emaInitMode {
		case 0:
			lu.ftranEMA = 0.01
			lu.btranEMA = 0.01
		case 1:
			lu.ftranEMA = 0.50
			lu.btranEMA = 0.50
		}

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
			// Only near-singular or random-coupling draws may fail factorization.
			if lu.valid {
				t.Fatalf("FactorizeSparse returned err=%v yet left lu.valid=true", err)
			}
			return
		}
		if lu.NeedsRefactor() {
			t.Fatalf("fresh FactorizeSparse reported NeedsRefactor()=true")
		}
		assertLUWorkClean(t, lu, "after_factorize")
		verifyLUSolvesVsExact(t, lu, mat, c, 0)

		colVec := make([]float64, m)
		aOrig := make([]float64, m)
		inIdx := make([]int, 0, 12)
		compacted := false

		for step := 1; step <= nUpdates; step++ {
			opByte := c.u8()
			var pivotRow int
			if opByte&1 == 0 {
				pivotRow = c.pick(min(m, 4)) // repeat hot pivots to trigger U row/col compaction
			} else {
				pivotRow = c.pick(m)
			}

			clear(colVec)
			inIdx = inIdx[:0]

			edgeMode := (opByte >> 1) & 7
			switch edgeMode {
			case 0:
				// Compaction driver: 6..12 off-diagonal entries exceeding initial headroom.
				colVec[pivotRow] = 4.0 + float64(c.pick(5))*0.5
				inIdx = append(inIdx, pivotRow)
				extra := min(m-1, 6+c.pick(7))
				for k := 1; k <= extra; k++ {
					r := (pivotRow + k*3 + c.pick(3)) % m
					if colVec[r] == 0 {
						colVec[r] = []float64{0.25, -0.25, 0.5, -0.5}[c.pick(4)]
						inIdx = append(inIdx, r)
					}
				}
			case 1:
				// Near-singular pivot straddling luPivotTol=1e-11.
				pivCand := []float64{5e-12, 1e-11, 2e-11, 1e-9, -5e-12}
				colVec[pivotRow] = pivCand[c.pick(len(pivCand))]
				inIdx = append(inIdx, pivotRow)
				if m > 1 {
					r2 := (pivotRow + 1) % m
					colVec[r2] = 0.5
					inIdx = append(inIdx, r2)
				}
			default:
				// Sparse incoming basis column with 1..4 off-diagonal terms.
				piv := 3.5 + float64(c.pick(6))*0.5
				if c.boolBit() {
					piv = -piv
				}
				colVec[pivotRow] = piv
				inIdx = append(inIdx, pivotRow)
				nOff := 1 + c.pick(min(4, max(1, m-1)))
				for range nOff {
					r := c.pick(m)
					if colVec[r] == 0 {
						colVec[r] = []float64{-0.5, -0.25, 0.25, 0.5}[c.pick(4)]
						inIdx = append(inIdx, r)
					}
				}
			}
			copy(aOrig, colVec)

			etasBefore := len(lu.etas)
			updatesBefore := lu.numUpdates
			uColFreeBefore := lu.uColFree
			uRowFreeBefore := lu.uRowFree

			var pushErr error
			switch (opByte >> 4) % 3 {
			case 0:
				_ = lu.FTRANSparseHintSpike(colVec, inIdx, nil)
				pushErr = lu.PushFTSpike(pivotRow)
			case 1:
				_ = lu.FTRANSparseHintSpike(colVec, inIdx, nil)
				sw := lu.spikeWork[:m]
				for idxPos, r := range lu.spikeIdx {
					if r >= 0 && int(r) < m {
						sw[r] = lu.spikeVal[idxPos]
					}
				}
				lu.spikeValid = false
				pushErr = lu.pushFTFromSpike(pivotRow)
			default:
				fIdx := lu.FTRANSparseHint(colVec, inIdx, nil)
				pushErr = lu.PushEtaSparse(pivotRow, colVec, fIdx)
			}
			assertLUWorkClean(t, lu, fmt.Sprintf("step=%d/after_push", step))

			if pushErr != nil {
				if len(lu.etas) != etasBefore || lu.numUpdates != updatesBefore {
					t.Fatalf("step=%d: rejected update mutated state: etas %d->%d updates %d->%d",
						step, etasBefore, len(lu.etas), updatesBefore, lu.numUpdates)
				}
				verifyLUSolvesVsExact(t, lu, mat, c, step)
				continue
			}

			for r := range m {
				mat[r][pivotRow] = aOrig[r]
			}
			if lu.uColFree < uColFreeBefore || lu.uRowFree < uRowFreeBefore {
				compacted = true
			}
			if lu.numUpdates >= maxEtaUpdates && !lu.NeedsRefactor() {
				t.Fatalf("step=%d: numUpdates=%d >= maxEtaUpdates=%d yet NeedsRefactor()=false",
					step, lu.numUpdates, maxEtaUpdates)
			}
			if step == 1 || step == 2 || step == 16 || step == 32 || step == 55 || step == 56 || step == nUpdates || compacted {
				verifyLUSolvesVsExact(t, lu, mat, c, step)
				compacted = false
			}
		}

		if !lu.NeedsRefactor() {
			driveDirectPushFTFromSpikeEdge(t, lu, mat, c.pick(m), c.u8())
			verifyLUSolvesVsExact(t, lu, mat, c, nUpdates+1)
		}
	})
}
