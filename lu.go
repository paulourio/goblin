package goblin

import (
	"fmt"
	"math"
)

const (
	luPivotTol     = 1e-11
	luMarkowitzTol = 0.1
	maxEtaUpdates  = 56
	minEtaIdxCap   = 16384
	etaIdxRowMult  = 24
	luDropTol      = 1e-13
	luSolveDropTol = 1e-13
)

type etaVector struct {
	pivotRow int
	pivotInv float64
	idx      []int
	val      []float64
}

// LUFactorization maintains P * B * Q = L * U with triangular singleton pre-ordering,
// sparse Markowitz threshold pivoting on the active nucleus, pre-permuted CSR/CSC
// triangular factors, and product-form-of-the-inverse (PFI) eta updates.
type LUFactorization struct {
	m          int
	numColSing int
	nucStart   int
	valid      bool

	uDiag      []float64
	uInvDiag   []float64
	perm       []int
	invPerm    []int
	colPerm    []int
	invColPerm []int

	// Unified unit-lower-triangular factor L in pivot coordinates [0..m-1] (CSC + CSR)
	lColPtr []int
	lRowIdx []int
	lColVal []float64
	lRowPtr []int
	lColIdx []int
	lRowVal []float64

	// Unified upper-triangular factor U (off-diagonal) in pivot coordinates [0..m-1] (CSC + CSR)
	uColPtr []int
	uRowIdx []int
	uColVal []float64
	uRowPtr []int
	uColIdx []int
	uRowVal []float64

	// Input basis in CSC and CSR
	bColPtr []int
	bRowIdx []int
	bVal    []float64
	bRowPtr []int
	bColIdx []int
	bRowVal []float64

	rowDeg  []int
	colDeg  []int
	rowDone []bool
	colDone []bool
	nzMark  []bool
	workInt []int
	work    []float64

	// Reusable buffers for sparse Markowitz nucleus elimination
	nucRows    []int
	nucCols    []int
	nucPivRow  []int
	nucPivCol  []int
	nucRowDone []bool
	nucColDone []bool
	nucRCols   [][]int
	nucRVals   [][]float64
	nucCRows   [][]int
	nucCVals   [][]float64
	nucWorkPos []int
	rowSingQ   []int
	colSingQ   []int
	remCols    []int

	etas    []etaVector
	etaPool []float64
	etaIdx  []int
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
	f.valid = false
	if cap(f.perm) < m {
		f.uDiag = make([]float64, m)
		f.uInvDiag = make([]float64, m)
		f.perm = make([]int, m)
		f.invPerm = make([]int, m)
		f.colPerm = make([]int, m)
		f.invColPerm = make([]int, m)
		f.lColPtr = make([]int, m+1)
		f.lRowPtr = make([]int, m+1)
		f.uColPtr = make([]int, m+1)
		f.uRowPtr = make([]int, m+1)
		f.bColPtr = make([]int, m+1)
		f.bRowPtr = make([]int, m+1)
		f.rowDeg = make([]int, m)
		f.colDeg = make([]int, m)
		f.rowDone = make([]bool, m)
		f.colDone = make([]bool, m)
		f.nzMark = make([]bool, m)
		f.workInt = make([]int, m)
		f.work = make([]float64, m)
	} else {
		f.uDiag = f.uDiag[:m]
		f.uInvDiag = f.uInvDiag[:m]
		f.perm = f.perm[:m]
		f.invPerm = f.invPerm[:m]
		f.colPerm = f.colPerm[:m]
		f.invColPerm = f.invColPerm[:m]
		f.lColPtr = f.lColPtr[:m+1]
		f.lRowPtr = f.lRowPtr[:m+1]
		f.uColPtr = f.uColPtr[:m+1]
		f.uRowPtr = f.uRowPtr[:m+1]
		f.bColPtr = f.bColPtr[:m+1]
		f.bRowPtr = f.bRowPtr[:m+1]
		f.rowDeg = f.rowDeg[:m]
		f.colDeg = f.colDeg[:m]
		f.rowDone = f.rowDone[:m]
		f.colDone = f.colDone[:m]
		f.nzMark = f.nzMark[:m]
		clear(f.nzMark)
		f.workInt = f.workInt[:m]
		f.work = f.work[:m]
		clear(f.work)
	}
	f.etas = f.etas[:0]
	f.etaPool = f.etaPool[:0]
	f.etaIdx = f.etaIdx[:0]
}

func (f *LUFactorization) NeedsRefactor() bool {
	return len(f.etas) >= maxEtaUpdates || len(f.etaIdx) >= max(minEtaIdxCap, etaIdxRowMult*f.m)
}

func (f *LUFactorization) Factorize(m int, fillCol func(col int, dst []float64)) error {
	colBuf := make([]float64, m)
	return f.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		clear(colBuf)
		fillCol(col, colBuf)
		for r, v := range colBuf {
			if math.Abs(v) > luDropTol {
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
		f.valid = true
		return nil
	}

	bColPtr := f.bColPtr[:m+1]
	bRowPtr := f.bRowPtr[:m+1]
	rowDeg := f.rowDeg[:m]
	colDeg := f.colDeg[:m]
	rowDone := f.rowDone[:m]
	colDone := f.colDone[:m]
	workInt := f.workInt[:m]

	f.bRowIdx = f.bRowIdx[:0]
	f.bVal = f.bVal[:0]
	bColPtr[0] = 0
	for col := range m {
		f.bRowIdx, f.bVal = appendCol(col, f.bRowIdx, f.bVal)
		bColPtr[col+1] = len(f.bRowIdx)
	}

	clear(rowDeg)
	clear(colDeg)
	clear(rowDone)
	clear(colDone)
	bRowIdx := f.bRowIdx
	bVal := f.bVal
	for col := range m {
		deg := 0
		for p := bColPtr[col]; p < bColPtr[col+1]; p++ {
			if math.Abs(bVal[p]) > luDropTol {
				rowDeg[bRowIdx[p]]++
				deg++
			}
		}
		colDeg[col] = deg
	}

	bRowPtr[0] = 0
	for r := range m {
		bRowPtr[r+1] = bRowPtr[r] + rowDeg[r]
		workInt[r] = bRowPtr[r]
	}
	nnz := bRowPtr[m]
	if cap(f.bColIdx) < nnz {
		f.bColIdx = make([]int, nnz)
		f.bRowVal = make([]float64, nnz)
	} else {
		f.bColIdx = f.bColIdx[:nnz]
		f.bRowVal = f.bRowVal[:nnz]
	}
	bColIdx := f.bColIdx
	bRowVal := f.bRowVal
	for col := range m {
		for p := bColPtr[col]; p < bColPtr[col+1]; p++ {
			v := bVal[p]
			if math.Abs(v) > luDropTol {
				r := bRowIdx[p]
				pos := workInt[r]
				bColIdx[pos] = col
				bRowVal[pos] = v
				workInt[r] = pos + 1
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
			if !f.rowDone[r] && math.Abs(f.bVal[p]) > luDropTol {
				pivRow = r
				pivVal = f.bVal[p]
				break
			}
		}
		if pivRow == -1 || math.Abs(pivVal) < luPivotTol {
			continue
		}
		f.perm[k] = pivRow
		f.colPerm[k] = col
		f.uDiag[k] = pivVal
		f.uInvDiag[k] = 1.0 / pivVal
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
		if pivCol == -1 || math.Abs(pivVal) < luPivotTol {
			continue
		}
		colMax := 0.0
		for p := f.bColPtr[pivCol]; p < f.bColPtr[pivCol+1]; p++ {
			r2 := f.bRowIdx[p]
			if !f.rowDone[r2] {
				av := math.Abs(f.bVal[p])
				if av > colMax {
					colMax = av
				}
			}
		}
		if math.Abs(pivVal) < luMarkowitzTol*colMax {
			continue
		}
		f.perm[k] = r
		f.colPerm[k] = pivCol
		f.uDiag[k] = pivVal
		f.uInvDiag[k] = 1.0 / pivVal
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
	nNuc := m - nucStart

	// Stage 3: Sparse Markowitz LU on the nNuc x nNuc active nucleus.
	if nNuc > 0 {
		if err := f.factorizeNucleusSparse(m, nucStart, nNuc); err != nil {
			return err
		}
	}

	for i, r := range f.perm {
		f.invPerm[r] = i
	}
	for j, c := range f.colPerm {
		f.invColPerm[c] = j
	}

	// Build unified L (CSC + CSR) and U (CSR + CSC) in pivot coordinates [0..m-1].
	f.assembleFactors(m, numColSing, nucStart, nNuc)
	f.valid = true
	return nil
}

func (f *LUFactorization) ensureNucleusBuffers(nNuc int) {
	if cap(f.nucRows) < nNuc {
		f.nucRows = make([]int, nNuc)
		f.nucCols = make([]int, nNuc)
		f.nucPivRow = make([]int, nNuc)
		f.nucPivCol = make([]int, nNuc)
		f.nucRowDone = make([]bool, nNuc)
		f.nucColDone = make([]bool, nNuc)
		f.nucRCols = make([][]int, nNuc)
		f.nucRVals = make([][]float64, nNuc)
		f.nucCRows = make([][]int, nNuc)
		f.nucCVals = make([][]float64, nNuc)
		f.nucWorkPos = make([]int, nNuc)
	} else {
		f.nucRows = f.nucRows[:nNuc]
		f.nucCols = f.nucCols[:nNuc]
		f.nucPivRow = f.nucPivRow[:nNuc]
		f.nucPivCol = f.nucPivCol[:nNuc]
		f.nucRowDone = f.nucRowDone[:nNuc]
		f.nucColDone = f.nucColDone[:nNuc]
		clear(f.nucRowDone)
		clear(f.nucColDone)
		f.nucRCols = f.nucRCols[:nNuc]
		f.nucRVals = f.nucRVals[:nNuc]
		f.nucCRows = f.nucCRows[:nNuc]
		f.nucCVals = f.nucCVals[:nNuc]
		f.nucWorkPos = f.nucWorkPos[:nNuc]
	}
	for i := range nNuc {
		f.nucWorkPos[i] = -1
		f.nucRCols[i] = f.nucRCols[i][:0]
		f.nucRVals[i] = f.nucRVals[i][:0]
		f.nucCRows[i] = f.nucCRows[i][:0]
		f.nucCVals[i] = f.nucCVals[i][:0]
	}
}

func (f *LUFactorization) factorizeNucleusSparse(m, nucStart, nNuc int) error {
	f.ensureNucleusBuffers(nNuc)

	nr := 0
	for r := range m {
		if !f.rowDone[r] {
			f.nucRows[nr] = r
			f.invPerm[r] = nr
			nr++
		}
	}
	nc := 0
	for c := range m {
		if !f.colDone[c] {
			f.nucCols[nc] = c
			f.invColPerm[c] = nc
			nc++
		}
	}

	rCols := f.nucRCols
	rVals := f.nucRVals
	cRows := f.nucCRows
	cVals := f.nucCVals

	for ncIdx := range nNuc {
		c := f.nucCols[ncIdx]
		for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
			v := f.bVal[p]
			if math.Abs(v) <= luDropTol {
				continue
			}
			r := f.bRowIdx[p]
			if !f.rowDone[r] {
				nrIdx := f.invPerm[r]
				rCols[nrIdx] = append(rCols[nrIdx], ncIdx)
				rVals[nrIdx] = append(rVals[nrIdx], v)
				cRows[ncIdx] = append(cRows[ncIdx], nrIdx)
				cVals[ncIdx] = append(cVals[ncIdx], v)
			}
		}
	}

	colSingQ := f.colSingQ[:0]
	rowSingQ := f.rowSingQ[:0]
	remCols := f.remCols[:0]
	for i := range nNuc {
		remCols = append(remCols, i)
		if len(cRows[i]) == 1 {
			colSingQ = append(colSingQ, i)
		}
		if len(rCols[i]) == 1 {
			rowSingQ = append(rowSingQ, i)
		}
	}
	colSingHead := 0
	rowSingHead := 0
	workPos := f.nucWorkPos

	for nk := range nNuc {
		pivNR := -1
		pivNC := -1
		pivVal := 0.0

		// 1. Check column singletons inside the nucleus (Markowitz cost 0).
		for colSingHead < len(colSingQ) {
			candC := colSingQ[colSingHead]
			colSingHead++
			if f.nucColDone[candC] || len(cRows[candC]) != 1 {
				continue
			}
			candR := cRows[candC][0]
			v := cVals[candC][0]
			if !f.nucRowDone[candR] && math.Abs(v) >= luPivotTol {
				pivNR = candR
				pivNC = candC
				pivVal = v
				break
			}
		}

		// 2. Check row singletons inside the nucleus (Markowitz cost 0, subject to column threshold).
		if pivNR == -1 {
			for rowSingHead < len(rowSingQ) {
				candR := rowSingQ[rowSingHead]
				rowSingHead++
				if f.nucRowDone[candR] || len(rCols[candR]) != 1 {
					continue
				}
				candC := rCols[candR][0]
				v := rVals[candR][0]
				if f.nucColDone[candC] || math.Abs(v) < luPivotTol {
					continue
				}
				colMax := 0.0
				for _, cv := range cVals[candC] {
					av := math.Abs(cv)
					if av > colMax {
						colMax = av
					}
				}
				if math.Abs(v) >= luMarkowitzTol*colMax {
					pivNR = candR
					pivNC = candC
					pivVal = v
					break
				}
			}
		}

		// 3. Markowitz threshold search across remaining active columns.
		if pivNR == -1 {
			bestMerit := int(1<<30 - 1)
			bestAbs := 0.0
			checkedCols := 0

			w := 0
			for _, candC := range remCols {
				if f.nucColDone[candC] {
					continue
				}
				remCols[w] = candC
				w++
			}
			remCols = remCols[:w]

			for _, candC := range remCols {
				cDeg := len(cRows[candC])
				if cDeg == 0 {
					return fmt.Errorf("singular basis at nucleus step %d (empty column)", nk)
				}
				if cDeg-1 > bestMerit && pivNR != -1 {
					continue
				}
				colMax := 0.0
				vals := cVals[candC]
				rows := cRows[candC]
				for _, v := range vals {
					av := math.Abs(v)
					if av > colMax {
						colMax = av
					}
				}
				if colMax < luPivotTol {
					continue
				}
				thresh := max(luPivotTol, luMarkowitzTol*colMax)
				for idx, candR := range rows {
					v := vals[idx]
					av := math.Abs(v)
					if av < thresh {
						continue
					}
					rDeg := len(rCols[candR])
					merit := (rDeg - 1) * (cDeg - 1)
					if merit < bestMerit || (merit == bestMerit && av > bestAbs) {
						bestMerit = merit
						bestAbs = av
						pivNR = candR
						pivNC = candC
						pivVal = v
					}
				}
				checkedCols++
				if pivNR != -1 && (bestMerit <= 1 || (checkedCols >= 6 && bestMerit <= 4) || checkedCols >= 16) {
					break
				}
			}

			// Fallback: maximum absolute entry across all active columns.
			if pivNR == -1 {
				for _, candC := range remCols {
					vals := cVals[candC]
					rows := cRows[candC]
					for idx, candR := range rows {
						av := math.Abs(vals[idx])
						if av > bestAbs {
							bestAbs = av
							pivNR = candR
							pivNC = candC
							pivVal = vals[idx]
						}
					}
				}
				if pivNR == -1 || math.Abs(pivVal) < luPivotTol {
					return fmt.Errorf("singular basis at step %d (max pivot %.3e)", nucStart+nk, bestAbs)
				}
			}
		}

		gk := nucStart + nk
		f.perm[gk] = f.nucRows[pivNR]
		f.colPerm[gk] = f.nucCols[pivNC]
		f.uDiag[gk] = pivVal
		f.uInvDiag[gk] = 1.0 / pivVal
		f.nucPivRow[nk] = pivNR
		f.nucPivCol[nk] = pivNC
		f.nucRowDone[pivNR] = true
		f.nucColDone[pivNC] = true

		// Remove pivot entry from pivot row and pivot col.
		removeEntry(rCols, rVals, pivNR, pivNC)
		removeEntry(cRows, cVals, pivNC, pivNR)

		pivRCols := rCols[pivNR]
		pivRVals := rVals[pivNR]
		for _, c := range pivRCols {
			removeEntry(cRows, cVals, c, pivNR)
			if len(cRows[c]) == 1 {
				colSingQ = append(colSingQ, c)
			}
		}

		pivCRows := cRows[pivNC]
		pivCVals := cVals[pivNC]
		invPiv := 1.0 / pivVal
		lWrite := 0

		for idx, r := range pivCRows {
			aik := pivCVals[idx]
			removeEntry(rCols, rVals, r, pivNC)
			if math.Abs(aik) <= luDropTol {
				if len(rCols[r]) == 1 {
					rowSingQ = append(rowSingQ, r)
				}
				continue
			}
			mult := aik * invPiv
			pivCRows[lWrite] = r
			pivCVals[lWrite] = mult
			lWrite++

			if len(pivRCols) == 0 {
				if len(rCols[r]) == 1 {
					rowSingQ = append(rowSingQ, r)
				}
				continue
			}

			rc := rCols[r]
			rv := rVals[r]
			for p, c := range rc {
				workPos[c] = p
			}
			for p, c := range pivRCols {
				delta := -mult * pivRVals[p]
				pos := workPos[c]
				if pos >= 0 {
					newV := rv[pos] + delta
					rv[pos] = newV
					updateEntry(cRows[c], cVals[c], r, newV)
				} else if math.Abs(delta) > luDropTol {
					workPos[c] = len(rc)
					rc = append(rc, c)
					rv = append(rv, delta)
					cRows[c] = append(cRows[c], r)
					cVals[c] = append(cVals[c], delta)
				}
			}

			w := 0
			for p, c := range rc {
				workPos[c] = -1
				v := rv[p]
				if math.Abs(v) > luDropTol {
					rc[w] = c
					rv[w] = v
					w++
				} else {
					removeEntry(cRows, cVals, c, r)
					if len(cRows[c]) == 1 {
						colSingQ = append(colSingQ, c)
					}
				}
			}
			rCols[r] = rc[:w]
			rVals[r] = rv[:w]
			if w == 1 {
				rowSingQ = append(rowSingQ, r)
			}
		}
		cRows[pivNC] = pivCRows[:lWrite]
		cVals[pivNC] = pivCVals[:lWrite]
	}

	f.colSingQ = colSingQ[:0]
	f.rowSingQ = rowSingQ[:0]
	f.remCols = remCols[:0]
	return nil
}

func removeEntry(idxLists [][]int, valLists [][]float64, listID, target int) {
	idxs := idxLists[listID]
	vals := valLists[listID]
	for i, id := range idxs {
		if id == target {
			last := len(idxs) - 1
			idxs[i] = idxs[last]
			vals[i] = vals[last]
			idxLists[listID] = idxs[:last]
			valLists[listID] = vals[:last]
			return
		}
	}
}

func updateEntry(idxs []int, vals []float64, target int, newVal float64) {
	for i, id := range idxs {
		if id == target {
			vals[i] = newVal
			return
		}
	}
}

func (f *LUFactorization) assembleFactors(m, numColSing, nucStart, nNuc int) {
	// 1. Assemble L in CSC (columns 0..m-1)
	lColPtr := f.lColPtr[:m+1]
	lRowPtr := f.lRowPtr[:m+1]
	uRowPtr := f.uRowPtr[:m+1]
	uColPtr := f.uColPtr[:m+1]
	bColPtr := f.bColPtr[:m+1]
	bRowPtr := f.bRowPtr[:m+1]
	rowDeg := f.rowDeg[:m]
	colDeg := f.colDeg[:m]
	workInt := f.workInt[:m]
	colPerm := f.colPerm[:m]
	perm := f.perm[:m]
	uInvDiag := f.uInvDiag[:m]

	f.lRowIdx = f.lRowIdx[:0]
	f.lColVal = f.lColVal[:0]
	clear(rowDeg)

	for k := range numColSing {
		lColPtr[k] = 0
	}
	bVal := f.bVal
	bRowIdx := f.bRowIdx
	invPerm := f.invPerm
	for k := numColSing; k < nucStart; k++ {
		lColPtr[k] = len(f.lRowIdx)
		c := colPerm[k]
		invD := uInvDiag[k]
		for p := bColPtr[c]; p < bColPtr[c+1]; p++ {
			v := bVal[p]
			if math.Abs(v) <= luDropTol {
				continue
			}
			i := invPerm[bRowIdx[p]]
			if i > k {
				mult := v * invD
				f.lRowIdx = append(f.lRowIdx, i)
				f.lColVal = append(f.lColVal, mult)
				rowDeg[i]++
			}
		}
	}
	for nk := range nNuc {
		k := nucStart + nk
		lColPtr[k] = len(f.lRowIdx)
		pivNC := f.nucPivCol[nk]
		rows := f.nucCRows[pivNC]
		vals := f.nucCVals[pivNC]
		for p, nr := range rows {
			i := invPerm[f.nucRows[nr]]
			f.lRowIdx = append(f.lRowIdx, i)
			f.lColVal = append(f.lColVal, vals[p])
			rowDeg[i]++
		}
	}
	lColPtr[m] = len(f.lRowIdx)

	// Transpose L CSC -> L CSR
	lNNZ := len(f.lRowIdx)
	lRowPtr[0] = 0
	for i := range m {
		lRowPtr[i+1] = lRowPtr[i] + rowDeg[i]
		workInt[i] = lRowPtr[i]
	}
	if cap(f.lColIdx) < lNNZ {
		f.lColIdx = make([]int, lNNZ)
		f.lRowVal = make([]float64, lNNZ)
	} else {
		f.lColIdx = f.lColIdx[:lNNZ]
		f.lRowVal = f.lRowVal[:lNNZ]
	}
	lRowIdx := f.lRowIdx
	lColVal := f.lColVal
	lColIdx := f.lColIdx
	lRowVal := f.lRowVal
	for k := numColSing; k < m; k++ {
		for p := lColPtr[k]; p < lColPtr[k+1]; p++ {
			i := lRowIdx[p]
			pos := workInt[i]
			lColIdx[pos] = k
			lRowVal[pos] = lColVal[p]
			workInt[i] = pos + 1
		}
	}

	// 2. Assemble U in CSR (rows 0..m-1)
	f.uColIdx = f.uColIdx[:0]
	f.uRowVal = f.uRowVal[:0]
	clear(colDeg)

	bColIdx := f.bColIdx
	bRowVal := f.bRowVal
	invColPerm := f.invColPerm
	for i := range numColSing {
		uRowPtr[i] = len(f.uColIdx)
		r := perm[i]
		for p := bRowPtr[r]; p < bRowPtr[r+1]; p++ {
			j := invColPerm[bColIdx[p]]
			if j > i {
				v := bRowVal[p]
				f.uColIdx = append(f.uColIdx, j)
				f.uRowVal = append(f.uRowVal, v)
				colDeg[j]++
			}
		}
	}
	for i := numColSing; i < nucStart; i++ {
		uRowPtr[i] = len(f.uColIdx)
	}
	for nk := range nNuc {
		i := nucStart + nk
		uRowPtr[i] = len(f.uColIdx)
		pivNR := f.nucPivRow[nk]
		cols := f.nucRCols[pivNR]
		vals := f.nucRVals[pivNR]
		for p, nc := range cols {
			j := invColPerm[f.nucCols[nc]]
			f.uColIdx = append(f.uColIdx, j)
			f.uRowVal = append(f.uRowVal, vals[p])
			colDeg[j]++
		}
	}
	uRowPtr[m] = len(f.uColIdx)

	// Transpose U CSR -> U CSC
	uNNZ := len(f.uColIdx)
	uColPtr[0] = 0
	for j := range m {
		uColPtr[j+1] = uColPtr[j] + colDeg[j]
		workInt[j] = uColPtr[j]
	}
	if cap(f.uRowIdx) < uNNZ {
		f.uRowIdx = make([]int, uNNZ)
		f.uColVal = make([]float64, uNNZ)
	} else {
		f.uRowIdx = f.uRowIdx[:uNNZ]
		f.uColVal = f.uColVal[:uNNZ]
	}
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal
	for i := range m {
		for p := uRowPtr[i]; p < uRowPtr[i+1]; p++ {
			j := uColIdx[p]
			pos := workInt[j]
			uRowIdx[pos] = i
			uColVal[pos] = uRowVal[p]
			workInt[j] = pos + 1
		}
	}
}

func (f *LUFactorization) FTRAN(rhs []float64) {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.uInvDiag) < m || len(f.lColPtr) <= m || len(f.uColPtr) <= m {
		return
	}
	rhs = rhs[:m]
	w := f.work[:m]
	perm := f.perm[:m]
	firstNZ := m
	lastNZ := -1
	for i, p := range perm {
		if uint(p) >= uint(m) {
			continue
		}
		v := rhs[p]
		if v != 0 {
			rhs[p] = 0
			w[i] = v
			if lastNZ < 0 {
				firstNZ = i
			}
			lastNZ = i
		}
	}

	if lastNZ >= 0 {
		lColPtr := f.lColPtr[:m+1]
		lRowIdx := f.lRowIdx
		lColVal := f.lColVal
		uInvDiag := f.uInvDiag[:m]
		uColPtr := f.uColPtr[:m+1]
		uRowIdx := f.uRowIdx
		uColVal := f.uColVal

		// Solve unit lower-triangular L * y = P * rhs
		startK := max(f.numColSing, firstNZ)
		for k := startK; k <= lastNZ && uint(k) < uint(m); k++ {
			wk := w[k]
			if wk == 0 {
				continue
			}
			if wk <= luDropTol && wk >= -luDropTol {
				w[k] = 0
				continue
			}
			start, end := lColPtr[k], lColPtr[k+1]
			rows := lRowIdx[start:end]
			vals := lColVal[start:end]
			if len(rows) > 0 {
				vals = vals[:len(rows)]
				for p, r := range rows {
					if uint(r) < uint(m) {
						w[r] -= vals[p] * wk
						if r > lastNZ {
							lastNZ = r
						}
					}
				}
			}
		}

		// Solve upper-triangular U * x = y.
		minNZ := firstNZ
		for j := lastNZ; j >= minNZ; j-- {
			if uint(j) >= uint(m) {
				continue
			}
			wj := w[j]
			if wj == 0 {
				continue
			}
			if wj <= luDropTol && wj >= -luDropTol {
				w[j] = 0
				continue
			}
			wj *= uInvDiag[j]
			w[j] = wj
			start, end := uColPtr[j], uColPtr[j+1]
			rows := uRowIdx[start:end]
			vals := uColVal[start:end]
			if len(rows) > 0 {
				vals = vals[:len(rows)]
				for p, r := range rows {
					if uint(r) < uint(m) {
						w[r] -= vals[p] * wj
						if r < minNZ {
							minNZ = r
						}
					}
				}
			}
		}

		colPerm := f.colPerm[:lastNZ+1]
		wOut := w[:lastNZ+1]
		for k := minNZ; k <= lastNZ; k++ {
			if wk := wOut[k]; wk != 0 {
				wOut[k] = 0
				c := colPerm[k]
				if uint(c) < uint(m) {
					rhs[c] = wk
				}
			}
		}
	}

	etas := f.etas
	for e := range etas {
		eta := &etas[e]
		pr := eta.pivotRow
		if uint(pr) >= uint(m) {
			continue
		}
		xp := rhs[pr]
		if xp == 0 {
			continue
		}
		if xp <= luDropTol && xp >= -luDropTol {
			rhs[pr] = 0
			continue
		}
		xp *= eta.pivotInv
		rhs[pr] = xp
		idxs := eta.idx
		vals := eta.val[:len(idxs)]
		for k, idx := range idxs {
			if uint(idx) < uint(m) {
				rhs[idx] -= vals[k] * xp
			}
		}
	}
}

// FTRANSparse solves B * x = rhs in place and appends the indices i with |x[i]| > luSolveDropTol to nzIdx.
func (f *LUFactorization) FTRANSparse(rhs []float64, nzIdx []int) []int {
	return f.FTRANSparseHint(rhs, nil, nzIdx)
}

// FTRANSparseHint solves B * x = rhs in place using optional candidate input nonzero
// indices inIdx, returning output indices i with |x[i]| > luSolveDropTol in nzIdx.
func (f *LUFactorization) FTRANSparseHint(rhs []float64, inIdx, nzIdx []int) []int {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.invPerm) < m || len(f.colPerm) < m || len(f.nzMark) < m || len(f.uInvDiag) < m || len(f.lColPtr) <= m || len(f.uColPtr) <= m {
		return nzIdx[:0]
	}
	rhs = rhs[:m]
	w := f.work[:m]
	firstNZ := m
	lastNZ := -1
	if inIdx != nil && len(inIdx) < (m>>2) {
		invPerm := f.invPerm[:m]
		for _, p := range inIdx {
			if uint(p) >= uint(m) {
				continue
			}
			v := rhs[p]
			if v != 0 {
				rhs[p] = 0
				i := invPerm[p]
				w[i] = v
				if i < firstNZ {
					firstNZ = i
				}
				if i > lastNZ {
					lastNZ = i
				}
			}
		}
	} else {
		perm := f.perm[:m]
		for i, p := range perm {
			if uint(p) >= uint(m) {
				continue
			}
			v := rhs[p]
			if v != 0 {
				rhs[p] = 0
				w[i] = v
				if lastNZ < 0 {
					firstNZ = i
				}
				lastNZ = i
			}
		}
	}

	nzIdx = nzIdx[:0]
	nzMark := f.nzMark[:m]
	if lastNZ >= 0 {
		lColPtr := f.lColPtr[:m+1]
		lRowIdx := f.lRowIdx
		lColVal := f.lColVal
		uInvDiag := f.uInvDiag[:m]
		uColPtr := f.uColPtr[:m+1]
		uRowIdx := f.uRowIdx
		uColVal := f.uColVal

		startK := max(f.numColSing, firstNZ)
		for k := startK; k <= lastNZ && uint(k) < uint(m); k++ {
			wk := w[k]
			if wk == 0 {
				continue
			}
			if wk <= luDropTol && wk >= -luDropTol {
				w[k] = 0
				continue
			}
			start, end := lColPtr[k], lColPtr[k+1]
			rows := lRowIdx[start:end]
			vals := lColVal[start:end]
			if len(rows) > 0 {
				vals = vals[:len(rows)]
				for p, r := range rows {
					if uint(r) < uint(m) {
						w[r] -= vals[p] * wk
						if r > lastNZ {
							lastNZ = r
						}
					}
				}
			}
		}

		minNZ := firstNZ
		for j := lastNZ; j >= minNZ; j-- {
			if uint(j) >= uint(m) {
				continue
			}
			wj := w[j]
			if wj == 0 {
				continue
			}
			if wj <= luDropTol && wj >= -luDropTol {
				w[j] = 0
				continue
			}
			wj *= uInvDiag[j]
			w[j] = wj
			start, end := uColPtr[j], uColPtr[j+1]
			rows := uRowIdx[start:end]
			vals := uColVal[start:end]
			if len(rows) > 0 {
				vals = vals[:len(rows)]
				for p, r := range rows {
					if uint(r) < uint(m) {
						w[r] -= vals[p] * wj
						if r < minNZ {
							minNZ = r
						}
					}
				}
			}
		}

		colPerm := f.colPerm[:lastNZ+1]
		wOut := w[:lastNZ+1]
		for k := minNZ; k <= lastNZ; k++ {
			if wk := wOut[k]; wk != 0 {
				wOut[k] = 0
				c := colPerm[k]
				if uint(c) < uint(m) {
					rhs[c] = wk
					nzMark[c] = true
					nzIdx = append(nzIdx, c)
				}
			}
		}
	}

	etas := f.etas
	for e := range etas {
		eta := &etas[e]
		pr := eta.pivotRow
		if uint(pr) >= uint(m) {
			continue
		}
		xp := rhs[pr]
		if xp == 0 {
			continue
		}
		if xp <= luDropTol && xp >= -luDropTol {
			rhs[pr] = 0
			continue
		}
		xp *= eta.pivotInv
		rhs[pr] = xp
		idxs := eta.idx
		vals := eta.val[:len(idxs)]
		for k, idx := range idxs {
			if uint(idx) < uint(m) {
				rhs[idx] -= vals[k] * xp
				if !nzMark[idx] {
					nzMark[idx] = true
					nzIdx = append(nzIdx, idx)
				}
			}
		}
	}

	out := nzIdx[:0]
	for _, i := range nzIdx {
		nzMark[i] = false
		v := rhs[i]
		if v > luSolveDropTol || v < -luSolveDropTol {
			out = append(out, i)
		} else {
			rhs[i] = 0
		}
	}
	return out
}

func (f *LUFactorization) BTRAN(rhs []float64) {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.uInvDiag) < m || len(f.lRowPtr) <= m || len(f.uRowPtr) <= m {
		return
	}
	rhs = rhs[:m]
	w := f.work[:m]

	etas := f.etas
	for e := len(etas) - 1; e >= 0; e-- {
		eta := &etas[e]
		pr := eta.pivotRow
		if uint(pr) >= uint(m) {
			continue
		}
		sum := rhs[pr]
		idxs := eta.idx
		vals := eta.val[:len(idxs)]
		for k, idx := range idxs {
			if uint(idx) < uint(m) {
				sum -= vals[k] * rhs[idx]
			}
		}
		rhs[pr] = sum * eta.pivotInv
	}
	colPerm := f.colPerm[:m]
	firstNZ := m
	lastNZ := -1
	for k, c := range colPerm {
		if uint(c) >= uint(m) {
			continue
		}
		v := rhs[c]
		if v != 0 {
			rhs[c] = 0
			w[k] = v
			if lastNZ < 0 {
				firstNZ = k
			}
			lastNZ = k
		}
	}
	if lastNZ < 0 {
		return
	}

	uInvDiag := f.uInvDiag[:m]
	uRowPtr := f.uRowPtr[:m+1]
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal
	lRowPtr := f.lRowPtr[:m+1]
	lColIdx := f.lColIdx
	lRowVal := f.lRowVal

	// Solve U^T * z = Q^T * rhs
	for i := firstNZ; i <= lastNZ && uint(i) < uint(m); i++ {
		wi := w[i]
		if wi == 0 {
			continue
		}
		if wi <= luDropTol && wi >= -luDropTol {
			w[i] = 0
			continue
		}
		wi *= uInvDiag[i]
		w[i] = wi
		start, end := uRowPtr[i], uRowPtr[i+1]
		cols := uColIdx[start:end]
		vals := uRowVal[start:end]
		if len(cols) > 0 {
			vals = vals[:len(cols)]
			for p, c := range cols {
				if uint(c) < uint(m) {
					w[c] -= vals[p] * wi
					if c > lastNZ {
						lastNZ = c
					}
				}
			}
		}
	}

	// Solve unit upper-triangular L^T * y = z
	minNZ := firstNZ
	stopI := max(f.numColSing, minNZ)
	for i := lastNZ; i >= stopI; i-- {
		if uint(i) >= uint(m) {
			continue
		}
		wi := w[i]
		if wi == 0 {
			continue
		}
		if wi <= luDropTol && wi >= -luDropTol {
			w[i] = 0
			continue
		}
		start, end := lRowPtr[i], lRowPtr[i+1]
		cols := lColIdx[start:end]
		vals := lRowVal[start:end]
		if len(cols) > 0 {
			vals = vals[:len(cols)]
			for p, c := range cols {
				if uint(c) < uint(m) {
					w[c] -= vals[p] * wi
					if c < minNZ {
						minNZ = c
						stopI = max(f.numColSing, minNZ)
					}
				}
			}
		}
	}

	perm := f.perm[:lastNZ+1]
	wOut := w[:lastNZ+1]
	for i := minNZ; i <= lastNZ; i++ {
		if wi := wOut[i]; wi != 0 {
			wOut[i] = 0
			p := perm[i]
			if uint(p) < uint(m) {
				rhs[p] = wi
			}
		}
	}
}

// BTRANSparse solves B^T * y = rhs in place and appends the indices i with |y[i]| > luDropTol to nzIdx.
func (f *LUFactorization) BTRANSparse(rhs []float64, nzIdx []int) []int {
	return f.BTRANSparseHint(rhs, nil, nzIdx)
}

// BTRANSparseHint solves B^T * y = rhs in place using optional candidate input nonzero
// indices inIdx, returning output indices i with |y[i]| > luDropTol in nzIdx.
func (f *LUFactorization) BTRANSparseHint(rhs []float64, inIdx, nzIdx []int) []int {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.invColPerm) < m || len(f.nzMark) < m || len(f.uInvDiag) < m || len(f.lRowPtr) <= m || len(f.uRowPtr) <= m {
		return nzIdx[:0]
	}
	rhs = rhs[:m]
	w := f.work[:m]
	etas := f.etas
	firstNZ := m
	lastNZ := -1

	if inIdx != nil && len(inIdx) < (m>>2) {
		nzMark := f.nzMark[:m]
		workIdx := nzIdx[:0]
		for _, p := range inIdx {
			if uint(p) < uint(m) && !nzMark[p] {
				nzMark[p] = true
				workIdx = append(workIdx, p)
			}
		}
		for e := len(etas) - 1; e >= 0; e-- {
			eta := &etas[e]
			pr := eta.pivotRow
			if uint(pr) >= uint(m) {
				continue
			}
			sum := rhs[pr]
			idxs := eta.idx
			vals := eta.val[:len(idxs)]
			for k, idx := range idxs {
				if uint(idx) < uint(m) {
					sum -= vals[k] * rhs[idx]
				}
			}
			rhs[pr] = sum * eta.pivotInv
			if sum != 0 && !nzMark[pr] {
				nzMark[pr] = true
				workIdx = append(workIdx, pr)
			}
		}
		invColPerm := f.invColPerm[:m]
		for _, c := range workIdx {
			nzMark[c] = false
			v := rhs[c]
			if v != 0 {
				rhs[c] = 0
				k := invColPerm[c]
				w[k] = v
				if k < firstNZ {
					firstNZ = k
				}
				if k > lastNZ {
					lastNZ = k
				}
			}
		}
		nzIdx = workIdx[:0]
	} else {
		for e := len(etas) - 1; e >= 0; e-- {
			eta := &etas[e]
			pr := eta.pivotRow
			if uint(pr) >= uint(m) {
				continue
			}
			sum := rhs[pr]
			idxs := eta.idx
			vals := eta.val[:len(idxs)]
			for k, idx := range idxs {
				if uint(idx) < uint(m) {
					sum -= vals[k] * rhs[idx]
				}
			}
			rhs[pr] = sum * eta.pivotInv
		}
		colPerm := f.colPerm[:m]
		for k, c := range colPerm {
			if uint(c) >= uint(m) {
				continue
			}
			v := rhs[c]
			if v != 0 {
				rhs[c] = 0
				w[k] = v
				if lastNZ < 0 {
					firstNZ = k
				}
				lastNZ = k
			}
		}
		nzIdx = nzIdx[:0]
	}
	if lastNZ < 0 {
		return nzIdx
	}

	uInvDiag := f.uInvDiag[:m]
	uRowPtr := f.uRowPtr[:m+1]
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal
	lRowPtr := f.lRowPtr[:m+1]
	lColIdx := f.lColIdx
	lRowVal := f.lRowVal

	for i := firstNZ; i <= lastNZ && uint(i) < uint(m); i++ {
		wi := w[i]
		if wi == 0 {
			continue
		}
		if wi <= luDropTol && wi >= -luDropTol {
			w[i] = 0
			continue
		}
		wi *= uInvDiag[i]
		w[i] = wi
		start, end := uRowPtr[i], uRowPtr[i+1]
		cols := uColIdx[start:end]
		vals := uRowVal[start:end]
		if len(cols) > 0 {
			vals = vals[:len(cols)]
			for p, c := range cols {
				if uint(c) < uint(m) {
					w[c] -= vals[p] * wi
					if c > lastNZ {
						lastNZ = c
					}
				}
			}
		}
	}

	minNZ := firstNZ
	stopI := max(f.numColSing, minNZ)
	for i := lastNZ; i >= stopI; i-- {
		if uint(i) >= uint(m) {
			continue
		}
		wi := w[i]
		if wi == 0 {
			continue
		}
		if wi <= luDropTol && wi >= -luDropTol {
			w[i] = 0
			continue
		}
		start, end := lRowPtr[i], lRowPtr[i+1]
		cols := lColIdx[start:end]
		vals := lRowVal[start:end]
		if len(cols) > 0 {
			vals = vals[:len(cols)]
			for p, c := range cols {
				if uint(c) < uint(m) {
					w[c] -= vals[p] * wi
					if c < minNZ {
						minNZ = c
						stopI = max(f.numColSing, minNZ)
					}
				}
			}
		}
	}

	perm := f.perm[:lastNZ+1]
	wOut := w[:lastNZ+1]
	for i := minNZ; i <= lastNZ; i++ {
		if wi := wOut[i]; wi != 0 {
			wOut[i] = 0
			p := perm[i]
			if uint(p) < uint(m) && (wi > luDropTol || wi < -luDropTol) {
				rhs[p] = wi
				nzIdx = append(nzIdx, p)
			}
		}
	}
	return nzIdx
}

func (f *LUFactorization) PushEta(pivotRow int, fCol []float64) error {
	piv := fCol[pivotRow]
	if math.Abs(piv) < luPivotTol {
		return fmt.Errorf("tiny eta pivot %.3e at row %d", piv, pivotRow)
	}
	startVal := len(f.etaPool)
	startIdx := len(f.etaIdx)
	for i, v := range fCol {
		if i != pivotRow && math.Abs(v) > luSolveDropTol {
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

func (f *LUFactorization) PushEtaSparse(pivotRow int, fCol []float64, fColIdx []int) error {
	piv := fCol[pivotRow]
	if math.Abs(piv) < luPivotTol {
		return fmt.Errorf("tiny eta pivot %.3e at row %d", piv, pivotRow)
	}
	startVal := len(f.etaPool)
	startIdx := len(f.etaIdx)
	for _, i := range fColIdx {
		if i != pivotRow {
			v := fCol[i]
			if math.Abs(v) > luSolveDropTol {
				f.etaIdx = append(f.etaIdx, i)
				f.etaPool = append(f.etaPool, v)
			}
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
