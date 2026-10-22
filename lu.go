package gopt

import (
	"fmt"
	"math"

	"gopt/internal/simd"
)

const (
	luPivotTol    = 1e-11
	maxEtaUpdates = 56
)

type etaVector struct {
	pivotRow int
	pivotInv float64
	idx      []int
	val      []float64
}

// LUFactorization maintains P * B * Q = L * U with triangular singleton pre-ordering,
// partial pivoting on the active nucleus, and column-spike product-form-of-the-inverse (PFI) eta updates.
type LUFactorization struct {
	m          int
	numColSing int
	nucStart   int
	nucStride  int
	lu         []float64
	uDiag      []float64
	perm       []int
	invPerm    []int
	colPerm    []int
	invColPerm []int
	lFirst     []int
	lLast      []int
	uFirst     []int
	uLast      []int
	bColPtr    []int
	bRowIdx    []int
	bVal       []float64
	bRowPtr    []int
	bColIdx    []int
	bRowVal    []float64
	rowDeg     []int
	colDeg     []int
	rowDone    []bool
	colDone    []bool
	workInt    []int
	etas       []etaVector
	etaPool    []float64
	etaIdx     []int
	work       []float64
}

func newLUFactorization(m int) *LUFactorization {
	f := &LUFactorization{}
	f.Reset(m)
	return f
}

func (f *LUFactorization) Reset(m int) {
	f.m = m
	f.numColSing = 0
	f.nucStart = 0
	f.nucStride = 0
	if cap(f.perm) < m {
		f.uDiag = make([]float64, m)
		f.perm = make([]int, m)
		f.invPerm = make([]int, m)
		f.colPerm = make([]int, m)
		f.invColPerm = make([]int, m)
		f.lFirst = make([]int, m)
		f.lLast = make([]int, m)
		f.uFirst = make([]int, m)
		f.uLast = make([]int, m)
		f.bColPtr = make([]int, m+1)
		f.bRowPtr = make([]int, m+1)
		f.rowDeg = make([]int, m)
		f.colDeg = make([]int, m)
		f.rowDone = make([]bool, m)
		f.colDone = make([]bool, m)
		f.workInt = make([]int, m)
		f.work = make([]float64, m)
	} else {
		f.uDiag = f.uDiag[:m]
		f.perm = f.perm[:m]
		f.invPerm = f.invPerm[:m]
		f.colPerm = f.colPerm[:m]
		f.invColPerm = f.invColPerm[:m]
		f.lFirst = f.lFirst[:m]
		f.lLast = f.lLast[:m]
		f.uFirst = f.uFirst[:m]
		f.uLast = f.uLast[:m]
		f.bColPtr = f.bColPtr[:m+1]
		f.bRowPtr = f.bRowPtr[:m+1]
		f.rowDeg = f.rowDeg[:m]
		f.colDeg = f.colDeg[:m]
		f.rowDone = f.rowDone[:m]
		f.colDone = f.colDone[:m]
		f.workInt = f.workInt[:m]
		f.work = f.work[:m]
	}
	f.etas = f.etas[:0]
	f.etaPool = f.etaPool[:0]
	f.etaIdx = f.etaIdx[:0]
}

func (f *LUFactorization) NeedsRefactor() bool {
	return len(f.etas) >= maxEtaUpdates
}

func (f *LUFactorization) Factorize(m int, fillCol func(col int, dst []float64)) error {
	colBuf := make([]float64, m)
	return f.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		clear(colBuf)
		fillCol(col, colBuf)
		for r, v := range colBuf {
			if math.Abs(v) > 1e-16 {
				rows = append(rows, r)
				vals = append(vals, v)
			}
		}
		return rows, vals
	})
}

func (f *LUFactorization) FactorizeSparse(m int, appendCol func(col int, rows []int, vals []float64) ([]int, []float64)) error {
	f.Reset(m)
	if m == 0 {
		return nil
	}

	f.bRowIdx = f.bRowIdx[:0]
	f.bVal = f.bVal[:0]
	f.bColPtr[0] = 0
	for col := range m {
		f.bRowIdx, f.bVal = appendCol(col, f.bRowIdx, f.bVal)
		f.bColPtr[col+1] = len(f.bRowIdx)
	}

	clear(f.rowDeg)
	clear(f.colDeg)
	clear(f.rowDone)
	clear(f.colDone)
	for col := range m {
		deg := 0
		for p := f.bColPtr[col]; p < f.bColPtr[col+1]; p++ {
			if math.Abs(f.bVal[p]) > 1e-16 {
				f.rowDeg[f.bRowIdx[p]]++
				deg++
			}
		}
		f.colDeg[col] = deg
	}

	f.bRowPtr[0] = 0
	for r := range m {
		f.bRowPtr[r+1] = f.bRowPtr[r] + f.rowDeg[r]
		f.workInt[r] = f.bRowPtr[r]
	}
	nnz := f.bRowPtr[m]
	if cap(f.bColIdx) < nnz {
		f.bColIdx = make([]int, nnz)
		f.bRowVal = make([]float64, nnz)
	} else {
		f.bColIdx = f.bColIdx[:nnz]
		f.bRowVal = f.bRowVal[:nnz]
	}
	for col := range m {
		for p := f.bColPtr[col]; p < f.bColPtr[col+1]; p++ {
			v := f.bVal[p]
			if math.Abs(v) > 1e-16 {
				r := f.bRowIdx[p]
				pos := f.workInt[r]
				f.bColIdx[pos] = col
				f.bRowVal[pos] = v
				f.workInt[r] = pos + 1
			}
		}
	}

	// Stage 1: Column singletons (upper-triangular prefix 0 .. numColSing-1).
	q := f.workInt[:0]
	for col := range m {
		if f.colDeg[col] == 1 {
			q = append(q, col)
		}
	}
	k := 0
	for head := 0; head < len(q); head++ {
		col := q[head]
		if f.colDone[col] || f.colDeg[col] != 1 {
			continue
		}
		pivRow := -1
		pivVal := 0.0
		for p := f.bColPtr[col]; p < f.bColPtr[col+1]; p++ {
			r := f.bRowIdx[p]
			if !f.rowDone[r] && math.Abs(f.bVal[p]) > 1e-16 {
				pivRow = r
				pivVal = f.bVal[p]
				break
			}
		}
		if pivRow == -1 || math.Abs(pivVal) < 1e-7 {
			continue
		}
		f.perm[k] = pivRow
		f.colPerm[k] = col
		f.uDiag[k] = pivVal
		f.rowDone[pivRow] = true
		f.colDone[col] = true
		k++
		for p := f.bRowPtr[pivRow]; p < f.bRowPtr[pivRow+1]; p++ {
			c := f.bColIdx[p]
			if !f.colDone[c] {
				f.colDeg[c]--
				if f.colDeg[c] == 1 {
					q = append(q, c)
				}
			}
		}
	}
	numColSing := k
	f.numColSing = numColSing

	// Stage 2: Row singletons (lower-triangular block numColSing .. nucStart-1).
	q = q[:0]
	for r := range m {
		if !f.rowDone[r] && f.rowDeg[r] == 1 {
			q = append(q, r)
		}
	}
	for head := 0; head < len(q); head++ {
		r := q[head]
		if f.rowDone[r] || f.rowDeg[r] != 1 {
			continue
		}
		pivCol := -1
		pivVal := 0.0
		for p := f.bRowPtr[r]; p < f.bRowPtr[r+1]; p++ {
			c := f.bColIdx[p]
			if !f.colDone[c] {
				pivCol = c
				pivVal = f.bRowVal[p]
				break
			}
		}
		if pivCol == -1 || math.Abs(pivVal) < 1e-7 {
			continue
		}
		f.perm[k] = r
		f.colPerm[k] = pivCol
		f.uDiag[k] = pivVal
		f.rowDone[r] = true
		f.colDone[pivCol] = true
		k++
		for p := f.bColPtr[pivCol]; p < f.bColPtr[pivCol+1]; p++ {
			r2 := f.bRowIdx[p]
			if !f.rowDone[r2] && math.Abs(f.bVal[p]) > 1e-16 {
				f.rowDeg[r2]--
				if f.rowDeg[r2] == 1 {
					q = append(q, r2)
				}
			}
		}
	}
	nucStart := k
	f.nucStart = nucStart

	// Stage 3: Active nucleus (nucStart .. m-1).
	kr := nucStart
	for r := range m {
		if !f.rowDone[r] {
			f.perm[kr] = r
			kr++
		}
	}
	kc := nucStart
	for c := range m {
		if !f.colDone[c] {
			f.colPerm[kc] = c
			kc++
		}
	}
	for i, r := range f.perm {
		f.invPerm[r] = i
	}
	for j, c := range f.colPerm {
		f.invColPerm[c] = j
	}

	nNuc := m - nucStart
	if nNuc == 0 {
		return nil
	}

	nucStride := (nNuc + 7) &^ 7
	f.nucStride = nucStride
	reqLU := nNuc * nucStride
	if cap(f.lu) < reqLU {
		f.lu = make([]float64, reqLU)
	} else {
		f.lu = f.lu[:reqLU]
		clear(f.lu)
	}
	lu := f.lu

	for nr := range nNuc {
		f.lFirst[nr] = -1
		f.lLast[nr] = -1
		f.uFirst[nr] = -1
		f.uLast[nr] = -1
	}

	// Scatter sparse nonzeros of the nucleus block into the compact nNuc x nucStride buffer.
	for nc := range nNuc {
		c := f.colPerm[nucStart+nc]
		for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
			v := f.bVal[p]
			if math.Abs(v) <= 1e-16 {
				continue
			}
			row := f.invPerm[f.bRowIdx[p]]
			if row >= nucStart {
				lu[(row-nucStart)*nucStride+nc] = v
			}
		}
	}

	// Dense Gaussian elimination with partial pivoting strictly on the nNuc x nNuc nucleus.
	for nk := range nNuc {
		bestRow := nk
		bestVal := math.Abs(lu[nk*nucStride+nk])
		for nr := nk + 1; nr < nNuc; nr++ {
			v := math.Abs(lu[nr*nucStride+nk])
			if v > bestVal {
				bestVal = v
				bestRow = nr
			}
		}
		if bestVal < luPivotTol {
			return fmt.Errorf("singular basis at step %d (max pivot %.3e)", nucStart+nk, bestVal)
		}

		if bestRow != nk {
			f.perm[nucStart+nk], f.perm[nucStart+bestRow] = f.perm[nucStart+bestRow], f.perm[nucStart+nk]
			f.lFirst[nk], f.lFirst[bestRow] = f.lFirst[bestRow], f.lFirst[nk]
			f.lLast[nk], f.lLast[bestRow] = f.lLast[bestRow], f.lLast[nk]
			rowK := lu[nk*nucStride : nk*nucStride+nNuc]
			rowBest := lu[bestRow*nucStride : bestRow*nucStride+nNuc]
			for nc := range nNuc {
				rowK[nc], rowBest[nc] = rowBest[nc], rowK[nc]
			}
		}

		pivVal := lu[nk*nucStride+nk]
		invPiv := 1.0 / pivVal

		firstCol := -1
		lastCol := -1
		for nc := nk + 1; nc < nNuc; nc++ {
			if math.Abs(lu[nk*nucStride+nc]) > 1e-16 {
				if firstCol == -1 {
					firstCol = nc
				}
				lastCol = nc
			}
		}
		f.uFirst[nk] = firstCol
		f.uLast[nk] = lastCol

		for nr := nk + 1; nr < nNuc; nr++ {
			aik := lu[nr*nucStride+nk]
			if math.Abs(aik) <= 1e-16 {
				lu[nr*nucStride+nk] = 0
				continue
			}
			mult := aik * invPiv
			lu[nr*nucStride+nk] = mult
			if f.lFirst[nr] == -1 {
				f.lFirst[nr] = nk
			}
			f.lLast[nr] = nk
			if firstCol != -1 {
				pivSlice := lu[nk*nucStride+firstCol : nk*nucStride+lastCol+1]
				targetSlice := lu[nr*nucStride+firstCol : nr*nucStride+lastCol+1]
				simd.Axpy64(targetSlice, pivSlice, -mult)
			}
		}
	}

	for nr := range nNuc {
		f.invPerm[f.perm[nucStart+nr]] = nucStart + nr
	}
	return nil
}

func (f *LUFactorization) FTRAN(rhs []float64) {
	m := f.m
	if m == 0 {
		return
	}
	w := f.work
	for i := range m {
		w[i] = rhs[f.perm[i]]
	}

	numColSing := f.numColSing
	nucStart := f.nucStart

	// Stage 2: Row singletons (lower-triangular block numColSing .. nucStart-1).
	for k := numColSing; k < nucStart; k++ {
		wk := w[k]
		if math.Abs(wk) <= 1e-16 {
			w[k] = 0
			continue
		}
		wk /= f.uDiag[k]
		w[k] = wk
		c := f.colPerm[k]
		for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
			i := f.invPerm[f.bRowIdx[p]]
			if i > k {
				w[i] -= f.bVal[p] * wk
			}
		}
	}

	// Stage 3: Active nucleus (nucStart .. m-1).
	nNuc := m - nucStart
	if nNuc > 0 {
		nucStride := f.nucStride
		lu := f.lu
		wNuc := w[nucStart:m]
		for nr := 1; nr < nNuc; nr++ {
			lf := f.lFirst[nr]
			if lf != -1 {
				end := f.lLast[nr] + 1
				row := lu[nr*nucStride+lf : nr*nucStride+end]
				wNuc[nr] -= simd.Dot64(row, wNuc[lf:end])
			}
		}
		for nr := nNuc - 1; nr >= 0; nr-- {
			sum := wNuc[nr]
			uf := f.uFirst[nr]
			if uf != -1 {
				end := f.uLast[nr] + 1
				row := lu[nr*nucStride+uf : nr*nucStride+end]
				sum -= simd.Dot64(row, wNuc[uf:end])
			}
			if math.Abs(sum) > 1e-16 {
				wNuc[nr] = sum / lu[nr*nucStride+nr]
			} else {
				wNuc[nr] = 0
			}
		}
	}

	// Stage 1: Column singletons (upper-triangular prefix 0 .. numColSing-1).
	if numColSing > 0 {
		for j := m - 1; j >= 0; j-- {
			wj := w[j]
			if math.Abs(wj) <= 1e-16 {
				w[j] = 0
				continue
			}
			if j < numColSing {
				wj /= f.uDiag[j]
				w[j] = wj
			}
			c := f.colPerm[j]
			limit := min(j, numColSing)
			for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
				i := f.invPerm[f.bRowIdx[p]]
				if i < limit {
					w[i] -= f.bVal[p] * wj
				}
			}
		}
	}

	for k := range m {
		rhs[f.colPerm[k]] = w[k]
	}
	for _, eta := range f.etas {
		pr := eta.pivotRow
		xp := rhs[pr] * eta.pivotInv
		rhs[pr] = xp
		if xp != 0 {
			for k, idx := range eta.idx {
				rhs[idx] -= eta.val[k] * xp
			}
		}
	}
}

func (f *LUFactorization) BTRAN(rhs []float64) {
	m := f.m
	if m == 0 {
		return
	}
	w := f.work

	for e := len(f.etas) - 1; e >= 0; e-- {
		eta := f.etas[e]
		pr := eta.pivotRow
		sum := rhs[pr]
		for k, idx := range eta.idx {
			sum -= eta.val[k] * rhs[idx]
		}
		rhs[pr] = sum * eta.pivotInv
	}
	for k := range m {
		w[k] = rhs[f.colPerm[k]]
	}

	numColSing := f.numColSing
	nucStart := f.nucStart

	// Stage 1: Column singletons (0 .. numColSing-1).
	for i := range numColSing {
		wi := w[i]
		if math.Abs(wi) <= 1e-16 {
			w[i] = 0
			continue
		}
		wi /= f.uDiag[i]
		w[i] = wi
		r := f.perm[i]
		for p := f.bRowPtr[r]; p < f.bRowPtr[r+1]; p++ {
			j := f.invColPerm[f.bColIdx[p]]
			if j > i {
				w[j] -= f.bRowVal[p] * wi
			}
		}
	}

	// Stage 3: Active nucleus (nucStart .. m-1).
	nNuc := m - nucStart
	if nNuc > 0 {
		nucStride := f.nucStride
		lu := f.lu
		wNuc := w[nucStart:m]
		for nk := range nNuc {
			wk := wNuc[nk]
			if math.Abs(wk) <= 1e-16 {
				wNuc[nk] = 0
				continue
			}
			wk /= lu[nk*nucStride+nk]
			wNuc[nk] = wk
			uf := f.uFirst[nk]
			if uf != -1 {
				end := f.uLast[nk] + 1
				simd.Axpy64(wNuc[uf:end], lu[nk*nucStride+uf:nk*nucStride+end], -wk)
			}
		}
		for nk := nNuc - 1; nk >= 1; nk-- {
			wk := wNuc[nk]
			if math.Abs(wk) <= 1e-16 {
				wNuc[nk] = 0
				continue
			}
			lf := f.lFirst[nk]
			if lf != -1 {
				end := f.lLast[nk] + 1
				simd.Axpy64(wNuc[lf:end], lu[nk*nucStride+lf:nk*nucStride+end], -wk)
			}
		}
	}

	// Stage 2: Row singletons (nucStart-1 down to numColSing).
	if nucStart > numColSing {
		for i := m - 1; i >= numColSing; i-- {
			wi := w[i]
			if math.Abs(wi) <= 1e-16 {
				w[i] = 0
				continue
			}
			if i < nucStart {
				wi /= f.uDiag[i]
				w[i] = wi
			}
			r := f.perm[i]
			limit := min(i, nucStart)
			for p := f.bRowPtr[r]; p < f.bRowPtr[r+1]; p++ {
				k := f.invColPerm[f.bColIdx[p]]
				if k >= numColSing && k < limit {
					w[k] -= f.bRowVal[p] * wi
				}
			}
		}
	}

	for i := range m {
		rhs[f.perm[i]] = w[i]
	}
}

func (f *LUFactorization) PushEta(pivotRow int, fCol []float64) error {
	piv := fCol[pivotRow]
	if math.Abs(piv) < luPivotTol {
		return fmt.Errorf("tiny eta pivot %.3e at row %d", piv, pivotRow)
	}
	startVal := len(f.etaPool)
	startIdx := len(f.etaIdx)
	for i, v := range fCol {
		if i != pivotRow && math.Abs(v) > 1e-15 {
			f.etaIdx = append(f.etaIdx, i)
			f.etaPool = append(f.etaPool, v)
		}
	}
	f.etas = append(f.etas, etaVector{
		pivotRow: pivotRow,
		pivotInv: 1.0 / piv,
		idx:      f.etaIdx[startIdx:],
		val:      f.etaPool[startVal:],
	})
	return nil
}
