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
	ftCancelTol    = 1e-8
	ftMaxRowMult   = 1.5e6
	luDropTol      = 1e-13
	luCancelRelTol = 1e-15
	luSolveDropTol = 1e-13
	maxMeritInit   = 1<<30 - 1
	hyperMinM      = 64
	hyperEMACutoff = 0.08
	hyperEMAAlpha  = 0.15
	hyperInitEMA   = 0.05
	maxLUDim       = math.MaxInt32 / 2
)

// asInt32 converts a bounds-checked basis index or pool offset to int32.
func asInt32(v int) int32 {
	return int32(v) //nolint:gosec // G115: basis dimensions and pool offsets are bounded by maxLUDim.
}

func ensureSlice[T any](s []T, n int) []T {
	if cap(s) < n {
		newCap := max(n, max(cap(s)*2, 16))
		return make([]T, n, newCap)
	}
	return s[:n]
}

func growSlice[T any](s []T, need int) []T {
	if cap(s) >= need {
		return s
	}
	newCap := max(need, max(cap(s)*2, 64))
	ns := make([]T, len(s), newCap)
	copy(ns, s)
	return ns
}

// etaVector stores a pointer-free 20-byte Forrest-Tomlin row-eta update R = I - e_p * r^T
// indexing into flat index and value pools, along with the index bounding box [minIdx, maxIdx]
// for O(1) disjoint-range skipping in FTRAN.
type etaVector struct {
	pivotRow int32
	start    int32
	end      int32
	minIdx   int32
	maxIdx   int32
}

// LUFactorization maintains R_t * ... * R_1 * L^{-1} * P * B * Q = U_t with triangular
// singleton pre-ordering, flat single-value Markowitz nucleus elimination, pre-permuted
// int32 CSR/CSC triangular factors, in-place mutated-U Forrest-Tomlin updates, and
// EMA-gated Gilbert-Peierls hypersparse triangular solves.
type LUFactorization struct {
	m                 int
	numColSing        int
	nucStart          int
	valid             bool
	hasL              bool
	hasU              bool
	emaInit           bool
	uPermuted         bool
	spikeValid        bool
	numUpdates        int
	maxUpdatePivotInv float64

	uDiag      []float64
	uInvDiag   []float64
	perm       []int32
	invPerm    []int32
	colPerm    []int32
	invColPerm []int32

	// Unified unit-lower-triangular factor L in pivot coordinates [0..m-1] (CSC + CSR).
	lColPtr []int32
	lRowIdx []int32
	lColVal []float64
	lRowPtr []int32
	lColIdx []int32
	lRowVal []float64

	// In-place mutable upper-triangular factor U (off-diagonal) in pivot coordinates [0..m-1] (CSC + CSR).
	uColStart []int32
	uColLen   []int32
	uColCap   []int32
	uRowIdx   []int32
	uColVal   []float64
	uColFree  int32
	uRowStart []int32
	uRowLen   []int32
	uRowCap   []int32
	uColIdx   []int32
	uRowVal   []float64
	uRowFree  int32
	uLiveNNZ  int
	uMaxNNZ   int
	uMinMoved int
	uMaxMoved int
	uOrder    []int32
	uPos      []int32
	uTmpIdx   []int32
	uTmpVal   []float64

	// Input basis staging buffers (CSC + CSR over non-pre-peeled columns).
	colRowsBuf []int
	colValsBuf []float64
	bColPtr    []int32
	bRowIdx    []int32
	bVal       []float64
	bRowPtr    []int32
	bColIdx    []int32
	bRowVal    []float64

	rowDeg  []int32
	colDeg  []int32
	rowDone []bool
	colDone []bool
	workInt []int32
	work    []float64

	// Flat single-value arena and intrusive Markowitz degree buckets for Stage 3 nucleus.
	nucRows        []int32
	nucCols        []int32
	nucRowDone     []bool
	nucColDone     []bool
	nucRowStart    []int32
	nucRowLen      []int32
	nucRowCap      []int32
	nucRowMax      []float64
	nucColMax      []float64
	nucColMaxDirty []bool
	nucRowCol      []int32
	nucRowVal      []float64
	nucRowFree     int32
	nucColStart    []int32
	nucColLen      []int32
	nucColCap      []int32
	nucColRow      []int32
	nucColFree     int32
	nucWorkPos     []int32
	nucPivRows     []int32
	rowHead        []int32
	rowPrev        []int32
	rowNext        []int32
	rowBucketDeg   []int32
	colHead        []int32
	colPrev        []int32
	colNext        []int32
	colBucketDeg   []int32
	minRowDeg      int32
	minColDeg      int32

	// Completed nucleus L columns and U rows in flat append-only arenas.
	nucLPtr []int32
	nucLRow []int32
	nucLVal []float64
	nucUPtr []int32
	nucUCol []int32
	nucUVal []float64

	// Pointer-free Forrest-Tomlin row-eta file, flat backing pools, and spike/FT scratch.
	etas      []etaVector
	etaPool   []float64
	etaIdx    []int32
	ftWork    []float64
	spikeWork []float64
	spikeIdx  []int32
	spikeVal  []float64

	// Iterative Gilbert-Peierls reach DFS state and output density EMAs.
	dfsMark  []uint32
	dfsGen   uint32
	dfsStack []int32
	dfsCur   []int32
	topOrder []int32
	dfsSeeds []int32
	ftranEMA float64
	btranEMA float64
}

func newLUFactorization(m int) *LUFactorization {
	f := &LUFactorization{}
	f.Reset(m)
	return f
}

// Reset resizes internal permutation, factor, and scratch slices for an m-by-m basis
// and clears all Forrest-Tomlin updates.
func (f *LUFactorization) Reset(m int) {
	if m < 0 {
		m = 0
	}
	if m > maxLUDim {
		panic(fmt.Sprintf("lu basis dimension %d exceeds limit %d", m, maxLUDim))
	}
	prevM := f.m
	f.m = m
	f.numColSing = 0
	f.nucStart = 0
	f.valid = false
	f.hasL = false
	f.hasU = false
	f.uPermuted = false
	f.spikeValid = false
	f.numUpdates = 0
	f.maxUpdatePivotInv = 0
	f.uColFree = 0
	f.uRowFree = 0
	f.uLiveNNZ = 0
	f.uMaxNNZ = 0
	f.uMinMoved = m
	f.uMaxMoved = -1

	if cap(f.perm) < m {
		newCap := max(m, max(cap(f.perm)*2, 16))
		f.uDiag = make([]float64, m, newCap)
		f.uInvDiag = make([]float64, m, newCap)
		f.perm = make([]int32, m, newCap)
		f.invPerm = make([]int32, m, newCap)
		f.colPerm = make([]int32, m, newCap)
		f.invColPerm = make([]int32, m, newCap)
		f.lColPtr = make([]int32, m+1, newCap+1)
		f.lRowPtr = make([]int32, m+1, newCap+1)
		f.uColStart = make([]int32, m, newCap)
		f.uColLen = make([]int32, m, newCap)
		f.uColCap = make([]int32, m, newCap)
		f.uRowStart = make([]int32, m, newCap)
		f.uRowLen = make([]int32, m, newCap)
		f.uRowCap = make([]int32, m, newCap)
		f.uOrder = make([]int32, m, newCap)
		f.uPos = make([]int32, m, newCap)
		f.bColPtr = make([]int32, m+1, newCap+1)
		f.bRowPtr = make([]int32, m+1, newCap+1)
		f.rowDeg = make([]int32, m, newCap)
		f.colDeg = make([]int32, m, newCap)
		f.rowDone = make([]bool, m, newCap)
		f.colDone = make([]bool, m, newCap)
		f.workInt = make([]int32, m, newCap)
		f.work = make([]float64, m, newCap)
		f.ftWork = make([]float64, m, newCap)
		f.spikeWork = make([]float64, m, newCap)
		f.spikeIdx = make([]int32, 0, newCap)
		f.spikeVal = make([]float64, 0, newCap)
		f.dfsMark = make([]uint32, m, newCap)
		f.dfsGen = 0
		f.dfsStack = make([]int32, m, newCap)
		f.dfsCur = make([]int32, m, newCap)
		f.topOrder = make([]int32, 0, newCap)
		f.dfsSeeds = make([]int32, 0, newCap)
	} else {
		f.uDiag = f.uDiag[:m]
		f.uInvDiag = f.uInvDiag[:m]
		f.perm = f.perm[:m]
		f.invPerm = f.invPerm[:m]
		f.colPerm = f.colPerm[:m]
		f.invColPerm = f.invColPerm[:m]
		f.lColPtr = f.lColPtr[:m+1]
		f.lRowPtr = f.lRowPtr[:m+1]
		f.uColStart = f.uColStart[:m]
		f.uColLen = f.uColLen[:m]
		f.uColCap = f.uColCap[:m]
		f.uRowStart = f.uRowStart[:m]
		f.uRowLen = f.uRowLen[:m]
		f.uRowCap = f.uRowCap[:m]
		f.uOrder = f.uOrder[:m]
		f.uPos = f.uPos[:m]
		f.bColPtr = f.bColPtr[:m+1]
		f.bRowPtr = f.bRowPtr[:m+1]
		f.rowDeg = f.rowDeg[:m]
		f.colDeg = f.colDeg[:m]
		f.rowDone = f.rowDone[:m]
		f.colDone = f.colDone[:m]
		f.workInt = f.workInt[:m]
		f.work = f.work[:m]
		f.ftWork = f.ftWork[:m]
		f.spikeWork = f.spikeWork[:m]
		f.spikeIdx = f.spikeIdx[:0]
		f.spikeVal = f.spikeVal[:0]
		f.dfsMark = f.dfsMark[:m]
		if m > prevM {
			clear(f.work[prevM:m])
			clear(f.ftWork[prevM:m])
			clear(f.spikeWork[prevM:m])
			clear(f.dfsMark[prevM:m])
		}
		f.dfsStack = f.dfsStack[:m]
		f.dfsCur = f.dfsCur[:m]
		f.topOrder = f.topOrder[:0]
		f.dfsSeeds = f.dfsSeeds[:0]
	}
	if !f.emaInit {
		f.ftranEMA = hyperInitEMA
		f.btranEMA = hyperInitEMA
		f.emaInit = true
	}
	f.etas = f.etas[:0]
	f.etaPool = f.etaPool[:0]
	f.etaIdx = f.etaIdx[:0]
}

// NeedsRefactor reports whether the factorization is invalid or the accumulated
// Forrest-Tomlin updates have reached their refactorization cap.
func (f *LUFactorization) NeedsRefactor() bool {
	return !f.valid || f.numUpdates >= maxEtaUpdates || len(f.etaIdx) >= max(minEtaIdxCap, etaIdxRowMult*f.m) || (f.uMaxNNZ > 0 && f.uLiveNNZ > f.uMaxNNZ)
}

// UpdatePivotsWellConditioned reports whether all accumulated Forrest-Tomlin update
// pivots have finite inverse magnitude at most maxInv.
func (f *LUFactorization) UpdatePivotsWellConditioned(maxInv float64) bool {
	return !math.IsNaN(f.maxUpdatePivotInv) && !math.IsInf(f.maxUpdatePivotInv, 0) && f.maxUpdatePivotInv <= maxInv
}

// Factorize computes the sparse LU factorization of an m-by-m basis populated via dense column callback fillCol.
func (f *LUFactorization) Factorize(m int, fillCol func(col int, dst []float64)) error {
	colBuf := make([]float64, m)
	return f.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		clear(colBuf)
		fillCol(col, colBuf)
		for r, v := range colBuf {
			if v != 0 {
				rows = append(rows, r)
				vals = append(vals, v)
			}
		}
		return rows, vals
	})
}

// FactorizeSparse computes P * B * Q = L * U over sparse basis columns supplied by appendCol,
// pre-peeling initial column singletons and row singletons before eliminating the Markowitz nucleus.
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
	perm := f.perm[:m]
	colPerm := f.colPerm[:m]
	uDiag := f.uDiag[:m]
	uInvDiag := f.uInvDiag[:m]

	clear(rowDeg)
	clear(colDeg)
	clear(rowDone)
	clear(colDone)

	bRowIdx := f.bRowIdx[:0]
	bVal := f.bVal[:0]
	colRows := f.colRowsBuf
	colVals := f.colValsBuf
	bColPtr[0] = 0
	k := 0

	for col := range m {
		colRows, colVals = appendCol(col, colRows[:0], colVals[:0])
		nzCount := 0
		singleRow := -1
		singleVal := 0.0
		for i, r := range colRows {
			v := colVals[i]
			if v != 0 {
				nzCount++
				singleRow = r
				singleVal = v
			}
		}
		if nzCount == 1 && uint(singleRow) < uint(m) && !rowDone[singleRow] && math.Abs(singleVal) >= luPivotTol {
			perm[k] = asInt32(singleRow)
			colPerm[k] = asInt32(col)
			uDiag[k] = singleVal
			uInvDiag[k] = 1.0 / singleVal
			rowDone[singleRow] = true
			colDone[col] = true
			k++
			bColPtr[col+1] = asInt32(len(bRowIdx))
			continue
		}
		if cap(bRowIdx) < len(bRowIdx)+nzCount {
			bRowIdx = growSlice(bRowIdx, len(bRowIdx)+nzCount)
			bVal = growSlice(bVal, len(bVal)+nzCount)
		}
		for i, r := range colRows {
			v := colVals[i]
			if v != 0 {
				bRowIdx = append(bRowIdx, asInt32(r))
				bVal = append(bVal, v)
				rowDeg[r]++
			}
		}
		bColPtr[col+1] = asInt32(len(bRowIdx))
	}
	f.colRowsBuf = colRows[:0]
	f.colValsBuf = colVals[:0]
	f.bRowIdx = bRowIdx
	f.bVal = bVal

	// Fast path when all m columns were pre-peeled as unit or diagonal column singletons.
	if k == m {
		f.numColSing = m
		f.nucStart = m
		invPerm := f.invPerm[:m]
		invColPerm := f.invColPerm[:m]
		for i := range m {
			invPerm[perm[i]] = asInt32(i)
			invColPerm[colPerm[i]] = asInt32(i)
		}
		clear(f.lColPtr[:m+1])
		clear(f.lRowPtr[:m+1])
		clear(f.uColStart[:m])
		clear(f.uColLen[:m])
		clear(f.uColCap[:m])
		clear(f.uRowStart[:m])
		clear(f.uRowLen[:m])
		clear(f.uRowCap[:m])
		f.lRowIdx = f.lRowIdx[:0]
		f.lColVal = f.lColVal[:0]
		f.lColIdx = f.lColIdx[:0]
		f.lRowVal = f.lRowVal[:0]
		uArenaNeed := max(m*4, 256)
		if cap(f.uColIdx) < uArenaNeed {
			f.uColIdx = make([]int32, uArenaNeed)
			f.uRowVal = make([]float64, uArenaNeed)
		} else {
			f.uColIdx = f.uColIdx[:cap(f.uColIdx)]
			f.uRowVal = f.uRowVal[:cap(f.uRowVal)]
		}
		if cap(f.uRowIdx) < uArenaNeed {
			f.uRowIdx = make([]int32, uArenaNeed)
			f.uColVal = make([]float64, uArenaNeed)
		} else {
			f.uRowIdx = f.uRowIdx[:cap(f.uRowIdx)]
			f.uColVal = f.uColVal[:cap(f.uColVal)]
		}
		f.uColFree = 0
		f.uRowFree = 0
		f.uLiveNNZ = 0
		f.uMaxNNZ = max(m*8, 4096)
		f.uMinMoved = m
		f.uMaxMoved = -1
		f.uPermuted = false
		f.spikeValid = false
		f.numUpdates = 0
		f.maxUpdatePivotInv = 0
		f.hasL = false
		f.hasU = false
		f.valid = true
		return nil
	}

	// Build B CSR over non-pre-peeled columns and initialize Stage 1 column singleton queue.
	bRowPtr[0] = 0
	for r := range m {
		bRowPtr[r+1] = bRowPtr[r] + rowDeg[r]
		workInt[r] = bRowPtr[r]
	}
	nnz := int(bRowPtr[m])
	f.bColIdx = ensureSlice(f.bColIdx, nnz)
	f.bRowVal = ensureSlice(f.bRowVal, nnz)
	bColIdx := f.bColIdx
	bRowVal := f.bRowVal
	q := f.dfsSeeds[:0]
	for col := range m {
		if colDone[col] {
			continue
		}
		deg := int32(0)
		start, end := bColPtr[col], bColPtr[col+1]
		for p := start; p < end; p++ {
			r := bRowIdx[p]
			pos := workInt[r]
			bColIdx[pos] = asInt32(col)
			bRowVal[pos] = bVal[p]
			workInt[r] = pos + 1
			if !rowDone[r] {
				deg++
			}
		}
		colDeg[col] = deg
		if deg == 1 {
			q = append(q, asInt32(col))
		}
	}

	// Stage 1: Remaining column singletons (upper-triangular prefix 0 .. numColSing-1).
	for head := 0; head < len(q); head++ {
		col := q[head]
		if colDone[col] || colDeg[col] != 1 {
			continue
		}
		pivRow := int32(-1)
		pivVal := 0.0
		for p := bColPtr[col]; p < bColPtr[col+1]; p++ {
			r := bRowIdx[p]
			if !rowDone[r] {
				pivRow = r
				pivVal = bVal[p]
				break
			}
		}
		if pivRow < 0 || math.Abs(pivVal) < luPivotTol {
			continue
		}
		perm[k] = pivRow
		colPerm[k] = col
		uDiag[k] = pivVal
		uInvDiag[k] = 1.0 / pivVal
		rowDone[pivRow] = true
		colDone[col] = true
		k++
		for p := bRowPtr[pivRow]; p < bRowPtr[pivRow+1]; p++ {
			c := bColIdx[p]
			if !colDone[c] {
				colDeg[c]--
				if colDeg[c] == 1 {
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
		if !rowDone[r] && rowDeg[r] == 1 {
			q = append(q, asInt32(r))
		}
	}
	for head := 0; head < len(q); head++ {
		r := q[head]
		if rowDone[r] || rowDeg[r] != 1 {
			continue
		}
		pivCol := int32(-1)
		pivVal := 0.0
		for p := bRowPtr[r]; p < bRowPtr[r+1]; p++ {
			c := bColIdx[p]
			if !colDone[c] {
				pivCol = c
				pivVal = bRowVal[p]
				break
			}
		}
		if pivCol < 0 || math.Abs(pivVal) < luPivotTol {
			continue
		}
		colMax := 0.0
		for p := bColPtr[pivCol]; p < bColPtr[pivCol+1]; p++ {
			r2 := bRowIdx[p]
			if !rowDone[r2] {
				av := math.Abs(bVal[p])
				if av > colMax {
					colMax = av
				}
			}
		}
		if math.Abs(pivVal) < luMarkowitzTol*colMax {
			continue
		}
		perm[k] = r
		colPerm[k] = pivCol
		uDiag[k] = pivVal
		uInvDiag[k] = 1.0 / pivVal
		rowDone[r] = true
		colDone[pivCol] = true
		k++
		for p := bColPtr[pivCol]; p < bColPtr[pivCol+1]; p++ {
			r2 := bRowIdx[p]
			if !rowDone[r2] {
				rowDeg[r2]--
				if rowDeg[r2] == 1 {
					q = append(q, r2)
				}
			}
		}
	}
	f.dfsSeeds = q[:0]
	nucStart := k
	f.nucStart = nucStart
	nNuc := m - nucStart

	// Stage 3: Flat single-value sparse Markowitz LU on the nNuc x nNuc active nucleus.
	if nNuc > 0 {
		if err := f.factorizeNucleusSparse(m, nucStart, nNuc); err != nil {
			return err
		}
	}

	invPerm := f.invPerm[:m]
	invColPerm := f.invColPerm[:m]
	for i := range m {
		invPerm[perm[i]] = asInt32(i)
		invColPerm[colPerm[i]] = asInt32(i)
	}

	f.assembleFactors(m, numColSing, nucStart, nNuc)
	f.valid = true
	return nil
}

func (f *LUFactorization) ensureNucleusBuffers(nNuc int) {
	f.nucRows = ensureSlice(f.nucRows, nNuc)
	f.nucCols = ensureSlice(f.nucCols, nNuc)
	f.nucRowDone = ensureSlice(f.nucRowDone, nNuc)
	f.nucColDone = ensureSlice(f.nucColDone, nNuc)
	clear(f.nucRowDone)
	clear(f.nucColDone)
	f.nucRowStart = ensureSlice(f.nucRowStart, nNuc)
	f.nucRowLen = ensureSlice(f.nucRowLen, nNuc)
	f.nucRowCap = ensureSlice(f.nucRowCap, nNuc)
	f.nucRowMax = ensureSlice(f.nucRowMax, nNuc)
	f.nucColMax = ensureSlice(f.nucColMax, nNuc)
	f.nucColMaxDirty = ensureSlice(f.nucColMaxDirty, nNuc)
	clear(f.nucRowMax)
	clear(f.nucColMax)
	clear(f.nucColMaxDirty)
	f.nucColStart = ensureSlice(f.nucColStart, nNuc)
	f.nucColLen = ensureSlice(f.nucColLen, nNuc)
	f.nucColCap = ensureSlice(f.nucColCap, nNuc)
	f.nucWorkPos = ensureSlice(f.nucWorkPos, nNuc)
	f.rowHead = ensureSlice(f.rowHead, nNuc+1)
	f.rowPrev = ensureSlice(f.rowPrev, nNuc)
	f.rowNext = ensureSlice(f.rowNext, nNuc)
	f.rowBucketDeg = ensureSlice(f.rowBucketDeg, nNuc)
	f.colHead = ensureSlice(f.colHead, nNuc+1)
	f.colPrev = ensureSlice(f.colPrev, nNuc)
	f.colNext = ensureSlice(f.colNext, nNuc)
	f.colBucketDeg = ensureSlice(f.colBucketDeg, nNuc)
	f.nucLPtr = ensureSlice(f.nucLPtr, nNuc+1)
	f.nucUPtr = ensureSlice(f.nucUPtr, nNuc+1)
	f.nucLRow = f.nucLRow[:0]
	f.nucLVal = f.nucLVal[:0]
	f.nucUCol = f.nucUCol[:0]
	f.nucUVal = f.nucUVal[:0]

	for i := range nNuc + 1 {
		f.rowHead[i] = -1
		f.colHead[i] = -1
	}
	for i := range nNuc {
		f.nucWorkPos[i] = -1
		f.rowBucketDeg[i] = -1
		f.colBucketDeg[i] = -1
		f.rowPrev[i] = -1
		f.rowNext[i] = -1
		f.colPrev[i] = -1
		f.colNext[i] = -1
	}
	f.minRowDeg = asInt32(nNuc + 1)
	f.minColDeg = asInt32(nNuc + 1)
}

func (f *LUFactorization) removeRowBucket(r int32) {
	d := f.rowBucketDeg[r]
	if d < 0 {
		return
	}
	prev := f.rowPrev[r]
	next := f.rowNext[r]
	if prev >= 0 {
		f.rowNext[prev] = next
	} else {
		f.rowHead[d] = next
	}
	if next >= 0 {
		f.rowPrev[next] = prev
	}
	f.rowPrev[r] = -1
	f.rowNext[r] = -1
	f.rowBucketDeg[r] = -1
}

func (f *LUFactorization) insertRowBucket(r, d int32) {
	head := f.rowHead[d]
	f.rowPrev[r] = -1
	f.rowNext[r] = head
	if head >= 0 {
		f.rowPrev[head] = r
	}
	f.rowHead[d] = r
	f.rowBucketDeg[r] = d
	if d < f.minRowDeg {
		f.minRowDeg = d
	}
}

func (f *LUFactorization) moveRowBucket(r, newDeg int32) {
	if f.rowBucketDeg[r] == newDeg {
		return
	}
	f.removeRowBucket(r)
	f.insertRowBucket(r, newDeg)
}

func (f *LUFactorization) removeColBucket(c int32) {
	d := f.colBucketDeg[c]
	if d < 0 {
		return
	}
	prev := f.colPrev[c]
	next := f.colNext[c]
	if prev >= 0 {
		f.colNext[prev] = next
	} else {
		f.colHead[d] = next
	}
	if next >= 0 {
		f.colPrev[next] = prev
	}
	f.colPrev[c] = -1
	f.colNext[c] = -1
	f.colBucketDeg[c] = -1
}

func (f *LUFactorization) insertColBucket(c, d int32) {
	head := f.colHead[d]
	f.colPrev[c] = -1
	f.colNext[c] = head
	if head >= 0 {
		f.colPrev[head] = c
	}
	f.colHead[d] = c
	f.colBucketDeg[c] = d
	if d < f.minColDeg {
		f.minColDeg = d
	}
}

func (f *LUFactorization) moveColBucket(c, newDeg int32) {
	if f.colBucketDeg[c] == newDeg {
		return
	}
	f.removeColBucket(c)
	f.insertColBucket(c, newDeg)
}

func (f *LUFactorization) compactNucRows(nNuc int, extraNeed int32) {
	total := int32(0)
	totalLive := int32(0)
	for r := range nNuc {
		if f.nucRowDone[r] {
			continue
		}
		l := f.nucRowLen[r]
		totalLive += l
		total += l + max(int32(4), l>>1)
	}
	if int(total+extraNeed) <= len(f.nucRowCol) {
		if int(f.nucRowFree+totalLive) <= len(f.nucRowCol) {
			tmpPos := f.nucRowFree
			cols := f.nucRowCol
			vals := f.nucRowVal
			for r := range nNuc {
				if f.nucRowDone[r] {
					continue
				}
				l := f.nucRowLen[r]
				s := f.nucRowStart[r]
				copy(cols[tmpPos:tmpPos+l], cols[s:s+l])
				copy(vals[tmpPos:tmpPos+l], vals[s:s+l])
				f.nucRowStart[r] = tmpPos
				tmpPos += l
			}
			pos := int32(0)
			for r := range nNuc {
				if f.nucRowDone[r] {
					f.nucRowStart[r] = 0
					f.nucRowLen[r] = 0
					f.nucRowCap[r] = 0
					continue
				}
				l := f.nucRowLen[r]
				s := f.nucRowStart[r]
				c := l + max(int32(4), l>>1)
				copy(cols[pos:pos+l], cols[s:s+l])
				copy(vals[pos:pos+l], vals[s:s+l])
				f.nucRowStart[r] = pos
				f.nucRowCap[r] = c
				pos += c
			}
			f.nucRowFree = pos
			return
		}
		if cap(f.lColIdx) >= int(totalLive) && cap(f.lRowVal) >= int(totalLive) {
			tmpCols := f.lColIdx[:totalLive]
			tmpVals := f.lRowVal[:totalLive]
			cols := f.nucRowCol
			vals := f.nucRowVal
			tmpPos := int32(0)
			for r := range nNuc {
				if f.nucRowDone[r] {
					continue
				}
				l := f.nucRowLen[r]
				s := f.nucRowStart[r]
				copy(tmpCols[tmpPos:tmpPos+l], cols[s:s+l])
				copy(tmpVals[tmpPos:tmpPos+l], vals[s:s+l])
				f.nucRowStart[r] = tmpPos
				tmpPos += l
			}
			pos := int32(0)
			for r := range nNuc {
				if f.nucRowDone[r] {
					f.nucRowStart[r] = 0
					f.nucRowLen[r] = 0
					f.nucRowCap[r] = 0
					continue
				}
				l := f.nucRowLen[r]
				s := f.nucRowStart[r]
				c := l + max(int32(4), l>>1)
				copy(cols[pos:pos+l], tmpCols[s:s+l])
				copy(vals[pos:pos+l], tmpVals[s:s+l])
				f.nucRowStart[r] = pos
				f.nucRowCap[r] = c
				pos += c
			}
			f.nucRowFree = pos
			return
		}
	}

	needCap := int(total + extraNeed + max(int32(64), total>>1))
	newCap := max(needCap, len(f.nucRowCol)*2)
	newCols := make([]int32, newCap)
	newVals := make([]float64, newCap)
	pos := int32(0)
	oldCols := f.nucRowCol
	oldVals := f.nucRowVal
	for r := range nNuc {
		if f.nucRowDone[r] {
			f.nucRowStart[r] = 0
			f.nucRowLen[r] = 0
			f.nucRowCap[r] = 0
			continue
		}
		l := f.nucRowLen[r]
		s := f.nucRowStart[r]
		c := l + max(int32(4), l>>1)
		copy(newCols[pos:pos+l], oldCols[s:s+l])
		copy(newVals[pos:pos+l], oldVals[s:s+l])
		f.nucRowStart[r] = pos
		f.nucRowCap[r] = c
		pos += c
	}
	f.nucRowCol = newCols
	f.nucRowVal = newVals
	f.nucRowFree = pos
}

func (f *LUFactorization) growNucRow(nNuc int, r, minNeed int32) {
	newSlotCap := max(minNeed+4, minNeed*2)
	if int(f.nucRowFree+newSlotCap) > len(f.nucRowCol) {
		f.compactNucRows(nNuc, newSlotCap)
	}
	oldStart := f.nucRowStart[r]
	oldLen := f.nucRowLen[r]
	newStart := f.nucRowFree
	copy(f.nucRowCol[newStart:newStart+oldLen], f.nucRowCol[oldStart:oldStart+oldLen])
	copy(f.nucRowVal[newStart:newStart+oldLen], f.nucRowVal[oldStart:oldStart+oldLen])
	f.nucRowStart[r] = newStart
	f.nucRowCap[r] = newSlotCap
	f.nucRowFree = newStart + newSlotCap
}

func (f *LUFactorization) compactNucCols(nNuc int, extraNeed int32) {
	total := int32(0)
	totalLive := int32(0)
	for c := range nNuc {
		if f.nucColDone[c] {
			continue
		}
		l := f.nucColLen[c]
		totalLive += l
		total += l + max(int32(4), l>>1)
	}
	if int(total+extraNeed) <= len(f.nucColRow) {
		if int(f.nucColFree+totalLive) <= len(f.nucColRow) {
			tmpPos := f.nucColFree
			rows := f.nucColRow
			for c := range nNuc {
				if f.nucColDone[c] {
					continue
				}
				l := f.nucColLen[c]
				s := f.nucColStart[c]
				copy(rows[tmpPos:tmpPos+l], rows[s:s+l])
				f.nucColStart[c] = tmpPos
				tmpPos += l
			}
			pos := int32(0)
			for c := range nNuc {
				if f.nucColDone[c] {
					f.nucColStart[c] = 0
					f.nucColLen[c] = 0
					f.nucColCap[c] = 0
					continue
				}
				l := f.nucColLen[c]
				s := f.nucColStart[c]
				capC := l + max(int32(4), l>>1)
				copy(rows[pos:pos+l], rows[s:s+l])
				f.nucColStart[c] = pos
				f.nucColCap[c] = capC
				pos += capC
			}
			f.nucColFree = pos
			return
		}
		if cap(f.uRowIdx) >= int(totalLive) {
			tmpRows := f.uRowIdx[:totalLive]
			rows := f.nucColRow
			tmpPos := int32(0)
			for c := range nNuc {
				if f.nucColDone[c] {
					continue
				}
				l := f.nucColLen[c]
				s := f.nucColStart[c]
				copy(tmpRows[tmpPos:tmpPos+l], rows[s:s+l])
				f.nucColStart[c] = tmpPos
				tmpPos += l
			}
			pos := int32(0)
			for c := range nNuc {
				if f.nucColDone[c] {
					f.nucColStart[c] = 0
					f.nucColLen[c] = 0
					f.nucColCap[c] = 0
					continue
				}
				l := f.nucColLen[c]
				s := f.nucColStart[c]
				capC := l + max(int32(4), l>>1)
				copy(rows[pos:pos+l], tmpRows[s:s+l])
				f.nucColStart[c] = pos
				f.nucColCap[c] = capC
				pos += capC
			}
			f.nucColFree = pos
			return
		}
	}

	needCap := int(total + extraNeed + max(int32(64), total>>1))
	newCap := max(needCap, len(f.nucColRow)*2)
	newRows := make([]int32, newCap)
	oldRows := f.nucColRow
	pos := int32(0)
	for c := range nNuc {
		if f.nucColDone[c] {
			f.nucColStart[c] = 0
			f.nucColLen[c] = 0
			f.nucColCap[c] = 0
			continue
		}
		l := f.nucColLen[c]
		s := f.nucColStart[c]
		capC := l + max(int32(4), l>>1)
		copy(newRows[pos:pos+l], oldRows[s:s+l])
		f.nucColStart[c] = pos
		f.nucColCap[c] = capC
		pos += capC
	}
	f.nucColRow = newRows
	f.nucColFree = pos
}

func (f *LUFactorization) appendNucCol(nNuc int, c, r int32) {
	l := f.nucColLen[c]
	if l >= f.nucColCap[c] {
		newSlotCap := max(l+4, l*2)
		if int(f.nucColFree+newSlotCap) > len(f.nucColRow) {
			f.compactNucCols(nNuc, newSlotCap)
		}
		oldStart := f.nucColStart[c]
		newStart := f.nucColFree
		copy(f.nucColRow[newStart:newStart+l], f.nucColRow[oldStart:oldStart+l])
		f.nucColStart[c] = newStart
		f.nucColCap[c] = newSlotCap
		f.nucColFree = newStart + newSlotCap
	}
	f.nucColRow[f.nucColStart[c]+l] = r
	f.nucColLen[c] = l + 1
}

func (f *LUFactorization) removeNucColEntry(c, r int32) {
	s := f.nucColStart[c]
	l := f.nucColLen[c]
	rows := f.nucColRow
	end := s + l
	for p := s; p < end; p++ {
		if rows[p] == r {
			last := end - 1
			rows[p] = rows[last]
			f.nucColLen[c] = l - 1
			return
		}
	}
}

func (f *LUFactorization) popNucRowEntry(r, c int32) (float64, bool) {
	s := f.nucRowStart[r]
	l := f.nucRowLen[r]
	cols := f.nucRowCol
	vals := f.nucRowVal
	end := s + l
	for p := s; p < end; p++ {
		if cols[p] == c {
			v := vals[p]
			last := end - 1
			cols[p] = cols[last]
			vals[p] = vals[last]
			f.nucRowLen[r] = l - 1
			return v, true
		}
	}
	return 0, false
}

func (f *LUFactorization) refreshNucColMax(c int32) float64 {
	cStart := f.nucColStart[c]
	cEnd := cStart + f.nucColLen[c]
	oldMax := f.nucColMax[c]
	colMax := 0.0
	for p := cStart; p < cEnd; p++ {
		r2 := f.nucColRow[p]
		if rm := f.nucRowMax[r2]; rm >= 0 && rm <= colMax {
			continue
		}
		cv := f.findRowVal(r2, c)
		if ac := math.Abs(cv); ac > colMax {
			colMax = ac
			if colMax >= oldMax {
				break
			}
		}
	}
	f.nucColMax[c] = colMax
	f.nucColMaxDirty[c] = false
	return colMax
}

func (f *LUFactorization) findRowVal(r, c int32) float64 {
	s := f.nucRowStart[r]
	end := s + f.nucRowLen[r]
	cols := f.nucRowCol[s:end]
	vals := f.nucRowVal[s:end]
	for i, col := range cols {
		if col == c {
			return vals[i]
		}
	}
	return 0
}

func (f *LUFactorization) factorizeNucleusSparse(m, nucStart, nNuc int) error {
	f.ensureNucleusBuffers(nNuc)

	nr := int32(0)
	for r := range m {
		if !f.rowDone[r] {
			f.nucRows[nr] = asInt32(r)
			f.invPerm[r] = nr
			nr++
		}
	}
	nc := int32(0)
	for c := range m {
		if !f.colDone[c] {
			f.nucCols[nc] = asInt32(c)
			f.invColPerm[c] = nc
			nc++
		}
	}

	// Pass 1: count initial nucleus row and column degrees to allocate elbow-room arenas once.
	clear(f.nucRowLen)
	clear(f.nucColLen)
	initNNZ := int32(0)
	for ncIdx := range nNuc {
		c := f.nucCols[ncIdx]
		cDeg := int32(0)
		for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
			r := f.bRowIdx[p]
			if !f.rowDone[r] {
				nrIdx := f.invPerm[r]
				f.nucRowLen[nrIdx]++
				cDeg++
			}
		}
		f.nucColLen[ncIdx] = cDeg
		initNNZ += cDeg
	}

	rowPos := int32(0)
	colPos := int32(0)
	for i := range nNuc {
		rDeg := f.nucRowLen[i]
		rCap := rDeg + max(int32(4), rDeg)
		f.nucRowStart[i] = rowPos
		f.nucRowCap[i] = rCap
		f.nucRowLen[i] = 0
		rowPos += rCap

		cDeg := f.nucColLen[i]
		cCap := cDeg + max(int32(4), cDeg)
		f.nucColStart[i] = colPos
		f.nucColCap[i] = cCap
		f.nucColLen[i] = 0
		colPos += cCap
	}
	f.nucRowFree = rowPos
	f.nucColFree = colPos

	rowArenaNeed := int(rowPos) + max(256, int(initNNZ)*2)
	colArenaNeed := int(colPos) + max(256, int(initNNZ)*2)
	if cap(f.nucRowCol) < rowArenaNeed || cap(f.nucRowVal) < rowArenaNeed {
		newCap := max(rowArenaNeed, max(cap(f.nucRowCol)*2, 256))
		f.nucRowCol = make([]int32, newCap)
		f.nucRowVal = make([]float64, newCap)
	} else {
		f.nucRowCol = f.nucRowCol[:cap(f.nucRowCol)]
		f.nucRowVal = f.nucRowVal[:cap(f.nucRowVal)]
	}
	if cap(f.nucColRow) < colArenaNeed {
		newCap := max(colArenaNeed, max(cap(f.nucColRow)*2, 256))
		f.nucColRow = make([]int32, newCap)
	} else {
		f.nucColRow = f.nucColRow[:cap(f.nucColRow)]
	}

	// Pass 2: populate flat row arena (with values), flat column pattern arena,
	// and initial row/column magnitude maxima in one pass.
	for ncIdx := range nNuc {
		c := f.nucCols[ncIdx]
		cWrite := f.nucColStart[ncIdx]
		cMax := 0.0
		for p := f.bColPtr[c]; p < f.bColPtr[c+1]; p++ {
			r := f.bRowIdx[p]
			if !f.rowDone[r] {
				v := f.bVal[p]
				av := math.Abs(v)
				nrIdx := f.invPerm[r]
				rPos := f.nucRowStart[nrIdx] + f.nucRowLen[nrIdx]
				f.nucRowCol[rPos] = asInt32(ncIdx)
				f.nucRowVal[rPos] = v
				f.nucRowLen[nrIdx]++
				if av > f.nucRowMax[nrIdx] {
					f.nucRowMax[nrIdx] = av
				}
				if av > cMax {
					cMax = av
				}
				f.nucColRow[cWrite] = nrIdx
				cWrite++
			}
		}
		f.nucColLen[ncIdx] = cWrite - f.nucColStart[ncIdx]
		f.nucColMax[ncIdx] = cMax
	}

	for i := range nNuc {
		f.insertRowBucket(asInt32(i), f.nucRowLen[i])
		f.insertColBucket(asInt32(i), f.nucColLen[i])
	}

	workPos := f.nucWorkPos
	nNuc32 := asInt32(nNuc)

	for nk := range nNuc {
		pivNR := int32(-1)
		pivNC := int32(-1)
		pivVal := 0.0

		// 0. Reject empty active rows or columns immediately.
		if f.colHead[0] >= 0 || f.rowHead[0] >= 0 {
			return fmt.Errorf("singular basis at nucleus step %d (empty row or column)", nk)
		}

		// 1. Column singletons inside the nucleus (Markowitz cost 0, zero multipliers).
		for candC := f.colHead[1]; candC >= 0; candC = f.colNext[candC] {
			candR := f.nucColRow[f.nucColStart[candC]]
			v := f.findRowVal(candR, candC)
			if math.Abs(v) >= luPivotTol {
				pivNR = candR
				pivNC = candC
				pivVal = v
				break
			}
		}

		// 2. Row singletons inside the nucleus (O(1) nucColMax upper bound, rare exact fallback).
		if pivNR < 0 {
			for candR := f.rowHead[1]; candR >= 0; candR = f.rowNext[candR] {
				s := f.nucRowStart[candR]
				candC := f.nucRowCol[s]
				v := f.nucRowVal[s]
				av := math.Abs(v)
				if av < luPivotTol {
					continue
				}
				if av >= luMarkowitzTol*f.nucColMax[candC] {
					pivNR = candR
					pivNC = candC
					pivVal = v
					break
				}
				if !f.nucColMaxDirty[candC] {
					continue
				}
				// Recompute exact column max only when the monotonic upper bound is dirty,
				// skipping rows whose cached rowMax cannot improve colMax and stopping early
				// once the monotonic upper bound oldMax is attained.
				cStart := f.nucColStart[candC]
				cEnd := cStart + f.nucColLen[candC]
				oldMax := f.nucColMax[candC]
				colMax := av
				for p := cStart; p < cEnd; p++ {
					r2 := f.nucColRow[p]
					if r2 == candR {
						continue
					}
					if rm := f.nucRowMax[r2]; rm >= 0 && rm <= colMax {
						continue
					}
					cv := f.findRowVal(r2, candC)
					if ac := math.Abs(cv); ac > colMax {
						colMax = ac
						if colMax >= oldMax {
							break
						}
					}
				}
				f.nucColMax[candC] = colMax
				f.nucColMaxDirty[candC] = false
				if av >= luMarkowitzTol*colMax {
					pivNR = candR
					pivNC = candC
					pivVal = v
					break
				}
			}
		}

		// 3. Ascending-degree Zlatev/Suhl-Suhl Markowitz threshold search.
		if pivNR < 0 {
			if f.minRowDeg < 2 {
				f.minRowDeg = 2
			}
			for f.minRowDeg <= nNuc32 && f.rowHead[f.minRowDeg] < 0 {
				f.minRowDeg++
			}
			if f.minColDeg < 2 {
				f.minColDeg = 2
			}
			for f.minColDeg <= nNuc32 && f.colHead[f.minColDeg] < 0 {
				f.minColDeg++
			}

			bestMerit := int32(maxMeritInit)
			bestAbs := 0.0
			searched := 0

			// Seed bestMerit from sparsest rows first when minRowDeg is small so column search
			// immediately prunes dense rows via merit >= bestMerit without scanning them.
			if f.minColDeg < f.minRowDeg && f.minRowDeg <= 8 && f.rowHead[f.minRowDeg] >= 0 {
				dRow := f.minRowDeg
				probed := 0
				for r := f.rowHead[dRow]; r >= 0 && probed < 2; r = f.rowNext[r] {
					s := f.nucRowStart[r]
					end := s + dRow
					for p := s; p < end; p++ {
						v := f.nucRowVal[p]
						av := math.Abs(v)
						if av < luPivotTol {
							continue
						}
						c := f.nucRowCol[p]
						merit := (dRow - 1) * (f.nucColLen[c] - 1)
						if pivNR >= 0 && (merit > bestMerit || (merit == bestMerit && av <= bestAbs)) {
							continue
						}
						if av < luMarkowitzTol*f.nucColMax[c] {
							if !f.nucColMaxDirty[c] || av < luMarkowitzTol*f.refreshNucColMax(c) {
								continue
							}
						}
						bestMerit = merit
						bestAbs = av
						pivNR = r
						pivNC = c
						pivVal = v
					}
					probed++
				}
			}

			startDeg := min(f.minRowDeg, f.minColDeg)
			for d := startDeg; d <= nNuc32; d++ {
				if pivNR >= 0 && (d-1)*(min(f.minRowDeg, f.minColDeg)-1) >= bestMerit {
					break
				}

				// Inspect rows of degree d (values are contiguous in L1 cache, exact column-threshold guarded).
				if f.rowHead[d] >= 0 && (pivNR < 0 || (d-1)*(f.minColDeg-1) < bestMerit) {
					for r := f.rowHead[d]; r >= 0; r = f.rowNext[r] {
						s := f.nucRowStart[r]
						end := s + d
						for p := s; p < end; p++ {
							v := f.nucRowVal[p]
							av := math.Abs(v)
							if av < luPivotTol {
								continue
							}
							c := f.nucRowCol[p]
							merit := (d - 1) * (f.nucColLen[c] - 1)
							if pivNR >= 0 && (merit > bestMerit || (merit == bestMerit && av <= bestAbs)) {
								continue
							}
							if av < luMarkowitzTol*f.nucColMax[c] {
								if !f.nucColMaxDirty[c] || av < luMarkowitzTol*f.refreshNucColMax(c) {
									continue
								}
							}
							bestMerit = merit
							bestAbs = av
							pivNR = r
							pivNC = c
							pivVal = v
						}
						searched++
						if pivNR >= 0 && (bestMerit <= (d-1)*(d-1) || searched >= 4) {
							break
						}
					}
					if pivNR >= 0 && searched >= 4 {
						break
					}
				}

				// Inspect columns of degree d only when strictly sparser than minRowDeg.
				if f.colHead[d] >= 0 && d < f.minRowDeg && (pivNR < 0 || (d-1)*(f.minRowDeg-1) < bestMerit) {
					for c := f.colHead[d]; c >= 0; c = f.colNext[c] {
						colThresh := max(luPivotTol, luMarkowitzTol*f.nucColMax[c])
						cStart := f.nucColStart[c]
						cEnd := cStart + d
						// Probe the sparsest row in column c first so a valid pivot tightens
						// bestMerit before calling findRowVal on denser rows in the same column.
						minP := cStart
						minRLen := f.nucRowLen[f.nucColRow[cStart]]
						for p := cStart + 1; p < cEnd; p++ {
							if rl := f.nucRowLen[f.nucColRow[p]]; rl < minRLen {
								minRLen = rl
								minP = p
							}
						}
						if pivNR >= 0 && (minRLen-1)*(d-1) >= bestMerit {
							searched++
							if searched >= 4 {
								break
							}
							continue
						}
						probeColEntry := func(r int32) {
							merit := (f.nucRowLen[r] - 1) * (d - 1)
							if pivNR >= 0 && merit >= bestMerit {
								return
							}
							v := f.findRowVal(r, c)
							av := math.Abs(v)
							if av < luPivotTol {
								return
							}
							if av < colThresh {
								if !f.nucColMaxDirty[c] {
									return
								}
								colThresh = max(luPivotTol, luMarkowitzTol*f.refreshNucColMax(c))
								if av < colThresh {
									return
								}
							}
							if merit < bestMerit || (merit == bestMerit && av > bestAbs) {
								bestMerit = merit
								bestAbs = av
								pivNR = r
								pivNC = c
								pivVal = v
							}
						}
						probeColEntry(f.nucColRow[minP])
						for p := cStart; p < cEnd; p++ {
							if p == minP {
								continue
							}
							probeColEntry(f.nucColRow[p])
						}
						searched++
						if pivNR >= 0 && searched >= 4 {
							break
						}
					}
					if pivNR >= 0 && searched >= 4 {
						break
					}
				}
			}

			// Fallback: global maximum absolute entry across all active rows.
			if pivNR < 0 {
				bestAbs = 0
				for r := range nNuc {
					if f.nucRowDone[r] {
						continue
					}
					s := f.nucRowStart[r]
					end := s + f.nucRowLen[r]
					for p := s; p < end; p++ {
						v := f.nucRowVal[p]
						av := math.Abs(v)
						if av > bestAbs {
							bestAbs = av
							pivNR = asInt32(r)
							pivNC = f.nucRowCol[p]
							pivVal = v
						}
					}
				}
				if pivNR < 0 || math.Abs(pivVal) < luPivotTol {
					return fmt.Errorf("singular basis at step %d (max pivot %.3e)", nucStart+nk, bestAbs)
				}
			}
		}

		gk := nucStart + nk
		f.perm[gk] = f.nucRows[pivNR]
		f.colPerm[gk] = f.nucCols[pivNC]
		f.uDiag[gk] = pivVal
		f.uInvDiag[gk] = 1.0 / pivVal
		f.removeRowBucket(pivNR)
		f.removeColBucket(pivNC)
		f.nucRowDone[pivNR] = true
		f.nucColDone[pivNC] = true

		// Extract off-diagonal pivot row into flat nucUCol/nucUVal and unlink pivNR from its columns.
		uStart := asInt32(len(f.nucUCol))
		f.nucUPtr[nk] = uStart
		pRowStart := f.nucRowStart[pivNR]
		pRowEnd := pRowStart + f.nucRowLen[pivNR]
		if cap(f.nucUCol) < int(uStart)+int(f.nucRowLen[pivNR]) {
			f.nucUCol = growSlice(f.nucUCol, int(uStart)+int(f.nucRowLen[pivNR]))
			f.nucUVal = growSlice(f.nucUVal, int(uStart)+int(f.nucRowLen[pivNR]))
		}
		for p := pRowStart; p < pRowEnd; p++ {
			c := f.nucRowCol[p]
			if c == pivNC {
				continue
			}
			val := f.nucRowVal[p]
			f.nucUCol = append(f.nucUCol, c)
			f.nucUVal = append(f.nucUVal, val)
			if math.Abs(val) >= f.nucColMax[c] {
				f.nucColMaxDirty[c] = true
			}
			f.removeNucColEntry(c, pivNR)
			f.moveColBucket(c, f.nucColLen[c])
		}
		uEnd := asInt32(len(f.nucUCol))
		f.nucUPtr[nk+1] = uEnd
		f.nucRowLen[pivNR] = 0

		// Snapshot pivot column rows (excluding pivNR) so arena compaction never aliases active loops.
		pivRows := f.nucPivRows[:0]
		pColStart := f.nucColStart[pivNC]
		pColEnd := pColStart + f.nucColLen[pivNC]
		for p := pColStart; p < pColEnd; p++ {
			r := f.nucColRow[p]
			if r != pivNR {
				pivRows = append(pivRows, r)
			}
		}
		f.nucPivRows = pivRows
		f.nucColLen[pivNC] = 0

		f.nucLPtr[nk] = asInt32(len(f.nucLRow))
		invPiv := 1.0 / pivVal
		pivUCount := uEnd - uStart

		for _, r := range pivRows {
			aik, found := f.popNucRowEntry(r, pivNC)
			if !found || aik == 0 {
				f.moveRowBucket(r, f.nucRowLen[r])
				continue
			}
			mult := aik * invPiv
			f.nucLRow = append(f.nucLRow, r)
			f.nucLVal = append(f.nucLVal, mult)

			if pivUCount == 0 {
				f.moveRowBucket(r, f.nucRowLen[r])
				continue
			}

			rStart := f.nucRowStart[r]
			rLen := f.nucRowLen[r]
			rCap := f.nucRowCap[r]
			rowCols := f.nucRowCol
			rowVals := f.nucRowVal
			for idx := range rLen {
				workPos[rowCols[rStart+idx]] = idx
			}

			for p := uStart; p < uEnd; p++ {
				c := f.nucUCol[p]
				delta := -mult * f.nucUVal[p]
				pos := workPos[c]
				if pos >= 0 {
					oldV := rowVals[rStart+pos]
					newV := oldV + delta
					if math.Abs(newV) <= luCancelRelTol*max(math.Abs(oldV), math.Abs(delta)) {
						newV = 0
					}
					rowVals[rStart+pos] = newV
					av := math.Abs(newV)
					if av > f.nucColMax[c] {
						f.nucColMax[c] = av
					} else if math.Abs(oldV) >= f.nucColMax[c] && av < math.Abs(oldV) {
						f.nucColMaxDirty[c] = true
					}
				} else if delta != 0 {
					if rLen >= rCap {
						f.nucRowLen[r] = rLen
						f.growNucRow(nNuc, r, rLen+(uEnd-p))
						rStart = f.nucRowStart[r]
						rCap = f.nucRowCap[r]
						rowCols = f.nucRowCol
						rowVals = f.nucRowVal
					}
					rowCols[rStart+rLen] = c
					rowVals[rStart+rLen] = delta
					workPos[c] = rLen
					rLen++
					if av := math.Abs(delta); av > f.nucColMax[c] {
						f.nucColMax[c] = av
					}
					f.appendNucCol(nNuc, c, r)
					f.moveColBucket(c, f.nucColLen[c])
				}
			}

			w := int32(0)
			rMax := 0.0
			for idx := range rLen {
				c := rowCols[rStart+idx]
				workPos[c] = -1
				v := rowVals[rStart+idx]
				if v != 0 {
					rowCols[rStart+w] = c
					rowVals[rStart+w] = v
					if av := math.Abs(v); av > rMax {
						rMax = av
					}
					w++
				} else {
					f.removeNucColEntry(c, r)
					f.moveColBucket(c, f.nucColLen[c])
				}
			}
			f.nucRowLen[r] = w
			f.nucRowMax[r] = rMax
			f.moveRowBucket(r, w)
		}
		f.nucLPtr[nk+1] = asInt32(len(f.nucLRow))
	}

	return nil
}

func (f *LUFactorization) assembleFactors(m, numColSing, nucStart, nNuc int) {
	lColPtr := f.lColPtr[:m+1]
	lRowPtr := f.lRowPtr[:m+1]
	uRowStart := f.uRowStart[:m]
	uRowLen := f.uRowLen[:m]
	uRowCap := f.uRowCap[:m]
	uColStart := f.uColStart[:m]
	uColLen := f.uColLen[:m]
	uColCap := f.uColCap[:m]
	bColPtr := f.bColPtr[:m+1]
	bRowPtr := f.bRowPtr[:m+1]
	rowDeg := f.rowDeg[:m]
	colDeg := f.colDeg[:m]
	workInt := f.workInt[:m]
	colPerm := f.colPerm[:m]
	perm := f.perm[:m]
	uInvDiag := f.uInvDiag[:m]
	invPerm := f.invPerm[:m]
	invColPerm := f.invColPerm[:m]

	// 1. Assemble L in CSC (columns 0..m-1) with pre-sized direct writes.
	lUpper := len(f.nucLRow)
	for k := numColSing; k < nucStart; k++ {
		c := colPerm[k]
		lUpper += int(bColPtr[c+1] - bColPtr[c])
	}
	f.lRowIdx = ensureSlice(f.lRowIdx, lUpper)
	f.lColVal = ensureSlice(f.lColVal, lUpper)
	lRowIdx := f.lRowIdx
	lColVal := f.lColVal
	clear(rowDeg)

	lPos := int32(0)
	for k := range numColSing {
		lColPtr[k] = 0
	}
	bVal := f.bVal
	bRowIdx := f.bRowIdx
	for k := numColSing; k < nucStart; k++ {
		lColPtr[k] = lPos
		c := colPerm[k]
		invD := uInvDiag[k]
		k32 := asInt32(k)
		for p := bColPtr[c]; p < bColPtr[c+1]; p++ {
			i := invPerm[bRowIdx[p]]
			if i > k32 {
				lRowIdx[lPos] = i
				lColVal[lPos] = bVal[p] * invD
				lPos++
				rowDeg[i]++
			}
		}
	}
	nucLPtr := f.nucLPtr
	nucLRow := f.nucLRow
	nucLVal := f.nucLVal
	nucRows := f.nucRows
	for nk := range nNuc {
		k := nucStart + nk
		lColPtr[k] = lPos
		for p := nucLPtr[nk]; p < nucLPtr[nk+1]; p++ {
			i := invPerm[nucRows[nucLRow[p]]]
			lRowIdx[lPos] = i
			lColVal[lPos] = nucLVal[p]
			lPos++
			rowDeg[i]++
		}
	}
	lColPtr[m] = lPos
	lNNZ := int(lPos)
	f.lRowIdx = lRowIdx[:lNNZ]
	f.lColVal = lColVal[:lNNZ]

	if lNNZ == 0 {
		f.hasL = false
		clear(lRowPtr)
		f.lColIdx = f.lColIdx[:0]
		f.lRowVal = f.lRowVal[:0]
	} else {
		f.hasL = true
		lRowPtr[0] = 0
		for i := range m {
			lRowPtr[i+1] = lRowPtr[i] + rowDeg[i]
			workInt[i] = lRowPtr[i]
		}
		f.lColIdx = ensureSlice(f.lColIdx, lNNZ)
		f.lRowVal = ensureSlice(f.lRowVal, lNNZ)
		lColIdx := f.lColIdx
		lRowVal := f.lRowVal
		for k := numColSing; k < m; k++ {
			k32 := asInt32(k)
			for p := lColPtr[k]; p < lColPtr[k+1]; p++ {
				i := lRowIdx[p]
				pos := workInt[i]
				lColIdx[pos] = k32
				lRowVal[pos] = lColVal[p]
				workInt[i] = pos + 1
			}
		}
	}

	// 2. Assemble U in mutable CSR/CSC arenas (rows/cols 0..m-1) with per-row headroom.
	const rowHeadroom = int32(2)
	uUpper := len(f.nucUCol)
	for i := range numColSing {
		r := perm[i]
		uUpper += int(bRowPtr[r+1] - bRowPtr[r])
	}
	uArenaNeed := max(uUpper*2+m*6, 256)
	if cap(f.uColIdx) < uArenaNeed {
		f.uColIdx = make([]int32, uArenaNeed)
		f.uRowVal = make([]float64, uArenaNeed)
	} else {
		f.uColIdx = f.uColIdx[:cap(f.uColIdx)]
		f.uRowVal = f.uRowVal[:cap(f.uRowVal)]
	}
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal
	clear(colDeg)

	uPos := int32(0)
	uNNZ := 0
	bColIdx := f.bColIdx
	bRowVal := f.bRowVal
	for i := range numColSing {
		uRowStart[i] = uPos
		r := perm[i]
		i32 := asInt32(i)
		for p := bRowPtr[r]; p < bRowPtr[r+1]; p++ {
			j := invColPerm[bColIdx[p]]
			if j > i32 {
				uColIdx[uPos] = j
				uRowVal[uPos] = bRowVal[p]
				uPos++
				colDeg[j]++
			}
		}
		deg := uPos - uRowStart[i]
		uRowLen[i] = deg
		uRowCap[i] = deg + rowHeadroom
		uPos += rowHeadroom
		uNNZ += int(deg)
	}
	for i := numColSing; i < nucStart; i++ {
		uRowStart[i] = uPos
		uRowLen[i] = 0
		uRowCap[i] = rowHeadroom
		uPos += rowHeadroom
	}
	nucUPtr := f.nucUPtr
	nucUCol := f.nucUCol
	nucUVal := f.nucUVal
	nucCols := f.nucCols
	for nk := range nNuc {
		i := nucStart + nk
		uRowStart[i] = uPos
		for p := nucUPtr[nk]; p < nucUPtr[nk+1]; p++ {
			j := invColPerm[nucCols[nucUCol[p]]]
			uColIdx[uPos] = j
			uRowVal[uPos] = nucUVal[p]
			uPos++
			colDeg[j]++
		}
		deg := uPos - uRowStart[i]
		uRowLen[i] = deg
		uRowCap[i] = deg + rowHeadroom
		uPos += rowHeadroom
		uNNZ += int(deg)
	}
	f.uRowFree = uPos
	f.uLiveNNZ = uNNZ
	f.uMaxNNZ = max(uNNZ*4+m*8, 4096)
	f.uMinMoved = m
	f.uMaxMoved = -1
	f.uPermuted = false
	f.spikeValid = false
	f.numUpdates = 0
	f.maxUpdatePivotInv = 0

	if cap(f.uRowIdx) < uArenaNeed {
		f.uRowIdx = make([]int32, uArenaNeed)
		f.uColVal = make([]float64, uArenaNeed)
	} else {
		f.uRowIdx = f.uRowIdx[:cap(f.uRowIdx)]
		f.uColVal = f.uColVal[:cap(f.uColVal)]
	}
	if uNNZ == 0 {
		f.hasU = false
		clear(uColStart)
		clear(uColLen)
		clear(uColCap)
		f.uColFree = 0
	} else {
		f.hasU = true
		cPos := int32(0)
		for j := range m {
			uColStart[j] = cPos
			deg := colDeg[j]
			uColLen[j] = deg
			uColCap[j] = deg
			workInt[j] = cPos
			cPos += deg
		}
		f.uColFree = cPos
		uRowIdx := f.uRowIdx
		uColVal := f.uColVal
		for i := range m {
			i32 := asInt32(i)
			start := uRowStart[i]
			end := start + uRowLen[i]
			for p := start; p < end; p++ {
				j := uColIdx[p]
				pos := workInt[j]
				uRowIdx[pos] = i32
				uColVal[pos] = uRowVal[p]
				workInt[j] = pos + 1
			}
		}
	}
}

func (f *LUFactorization) nextDFSGen() uint32 {
	f.dfsGen++
	if f.dfsGen == 0 {
		clear(f.dfsMark[:f.m])
		f.dfsGen = 1
	}
	return f.dfsGen
}

// reachDAG performs an iterative post-order DFS from seeds over a DAG defined by (ptr, adj).
// Iterating the returned slice in reverse yields exact topological solve order.
// Returns (nil, false) immediately if the reach exceeds limit.
func (f *LUFactorization) reachDAG(seeds []int32, ptr, adj []int32, limit int) ([]int32, bool) {
	m := f.m
	gen := f.nextDFSGen()
	mark := f.dfsMark[:m]
	stack := f.dfsStack[:m]
	cur := f.dfsCur[:m]
	top := f.topOrder[:m]
	topLen := 0
	visited := 0

	for _, s := range seeds {
		if s < 0 || int(s) >= len(mark) || mark[s] == gen {
			continue
		}
		mark[s] = gen
		visited++
		if visited > limit {
			return nil, false
		}
		sp := 1
		stack[0] = s
		cur[0] = ptr[s]
		for sp > 0 {
			u := stack[sp-1]
			p := cur[sp-1]
			end := ptr[u+1]
			advanced := false
			for p < end {
				v := adj[p]
				p++
				if v >= 0 && int(v) < len(mark) && mark[v] != gen {
					mark[v] = gen
					visited++
					if visited > limit {
						return nil, false
					}
					cur[sp-1] = p
					stack[sp] = v
					cur[sp] = ptr[v]
					sp++
					advanced = true
					break
				}
			}
			if !advanced {
				top[topLen] = u
				topLen++
				sp--
			}
		}
	}
	f.topOrder = top[:topLen]
	return f.topOrder, true
}

// reachDAGRanges performs an iterative post-order DFS from seeds over a mutable DAG
// defined by per-node (starts, lens) ranges into adj.
func (f *LUFactorization) reachDAGRanges(seeds []int32, starts, lens, adj []int32, limit int) ([]int32, bool) {
	m := f.m
	gen := f.nextDFSGen()
	mark := f.dfsMark[:m]
	stack := f.dfsStack[:m]
	cur := f.dfsCur[:m]
	top := f.topOrder[:m]
	topLen := 0
	visited := 0

	for _, s := range seeds {
		if s < 0 || int(s) >= len(mark) || mark[s] == gen {
			continue
		}
		mark[s] = gen
		visited++
		if visited > limit {
			return nil, false
		}
		sp := 1
		stack[0] = s
		cur[0] = starts[s]
		for sp > 0 {
			u := stack[sp-1]
			p := cur[sp-1]
			end := starts[u] + lens[u]
			advanced := false
			for p < end {
				v := adj[p]
				p++
				if v >= 0 && int(v) < len(mark) && mark[v] != gen {
					mark[v] = gen
					visited++
					if visited > limit {
						return nil, false
					}
					cur[sp-1] = p
					stack[sp] = v
					cur[sp] = starts[v]
					sp++
					advanced = true
					break
				}
			}
			if !advanced {
				top[topLen] = u
				topLen++
				sp--
			}
		}
	}
	f.topOrder = top[:topLen]
	return f.topOrder, true
}

func (f *LUFactorization) solveLDense(w []float64, firstNZ, lastNZ int, dropTol float64) int {
	if !f.hasL {
		return lastNZ
	}
	m := len(w)
	lColPtr := f.lColPtr[:m+1]
	lRowIdx := f.lRowIdx
	lColVal := f.lColVal[:len(lRowIdx)]
	startK := max(f.numColSing, firstNZ)
	for k := startK; k <= lastNZ && uint(k) < uint(len(w)); k++ {
		wk := w[k]
		if wk == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wk) <= dropTol {
			w[k] = 0
			continue
		}
		start, end := lColPtr[k], lColPtr[k+1]
		if start < 0 || int(end) > len(lRowIdx) {
			continue
		}
		for p := start; p < end; p++ {
			r := lRowIdx[p]
			if r >= 0 && int(r) < len(w) {
				w[r] -= lColVal[p] * wk
				if int(r) > lastNZ {
					lastNZ = int(r)
				}
			}
		}
	}
	return lastNZ
}

func (f *LUFactorization) solveUDense(w []float64, firstNZ, lastNZ int, dropTol float64) (int, int) {
	m := len(w)
	uInvDiag := f.uInvDiag[:len(w)]
	if !f.hasU {
		for j := firstNZ; j <= lastNZ; j++ {
			wj := w[j]
			if wj == 0 {
				continue
			}
			if dropTol > 0 && math.Abs(wj) <= dropTol {
				w[j] = 0
				continue
			}
			w[j] = wj * uInvDiag[j]
		}
		return firstNZ, lastNZ
	}
	uColStart := f.uColStart[:m]
	uColLen := f.uColLen[:m]
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal[:len(uRowIdx)]
	if !f.uPermuted {
		minNZ := firstNZ
		for j := lastNZ; j >= minNZ; j-- {
			wj := w[j]
			if wj == 0 {
				continue
			}
			if dropTol > 0 && math.Abs(wj) <= dropTol {
				w[j] = 0
				continue
			}
			wj *= uInvDiag[j]
			w[j] = wj
			start := uColStart[j]
			end := start + uColLen[j]
			for p := start; p < end; p++ {
				r := uRowIdx[p]
				if r >= 0 && int(r) < len(w) {
					w[r] -= uColVal[p] * wj
					if int(r) < minNZ {
						minNZ = int(r)
					}
				}
			}
		}
		return minNZ, lastNZ
	}
	uOrder := f.uOrder[:m]
	uMinMoved := f.uMinMoved
	uMaxMoved := f.uMaxMoved
	minNZ := firstNZ
	outMin := m
	outMax := -1
	if lastNZ >= uMinMoved {
		posStart := max(lastNZ, uMaxMoved)
		if posStart >= m {
			posStart = m - 1
		}
		for pos := posStart; pos >= uMinMoved; pos-- {
			j := uOrder[pos]
			wj := w[j]
			if wj == 0 {
				continue
			}
			if dropTol > 0 && math.Abs(wj) <= dropTol {
				w[j] = 0
				continue
			}
			wj *= uInvDiag[j]
			w[j] = wj
			if int(j) < outMin {
				outMin = int(j)
			}
			if int(j) > outMax {
				outMax = int(j)
			}
			start := uColStart[j]
			end := start + uColLen[j]
			for p := start; p < end; p++ {
				r := uRowIdx[p]
				if r >= 0 && int(r) < len(w) {
					w[r] -= uColVal[p] * wj
					if int(r) < minNZ {
						minNZ = int(r)
					}
				}
			}
		}
	}
	jStart := min(lastNZ, uMinMoved-1)
	for j := jStart; j >= minNZ; j-- {
		wj := w[j]
		if wj == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wj) <= dropTol {
			w[j] = 0
			continue
		}
		wj *= uInvDiag[j]
		w[j] = wj
		if j < outMin {
			outMin = j
		}
		if j > outMax {
			outMax = j
		}
		start := uColStart[j]
		end := start + uColLen[j]
		for p := start; p < end; p++ {
			r := uRowIdx[p]
			if r >= 0 && int(r) < len(w) {
				w[r] -= uColVal[p] * wj
				if int(r) < minNZ {
					minNZ = int(r)
				}
			}
		}
	}
	return outMin, outMax
}

func (f *LUFactorization) solveUTDense(w []float64, firstNZ, lastNZ int, dropTol float64) (int, int) {
	m := len(w)
	uInvDiag := f.uInvDiag[:len(w)]
	if !f.hasU {
		for i := firstNZ; i <= lastNZ; i++ {
			wi := w[i]
			if wi == 0 {
				continue
			}
			if dropTol > 0 && math.Abs(wi) <= dropTol {
				w[i] = 0
				continue
			}
			w[i] = wi * uInvDiag[i]
		}
		return firstNZ, lastNZ
	}
	uRowStart := f.uRowStart[:m]
	uRowLen := f.uRowLen[:m]
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal[:len(uColIdx)]
	if !f.uPermuted {
		for i := firstNZ; i <= lastNZ && uint(i) < uint(len(w)); i++ {
			wi := w[i]
			if wi == 0 {
				continue
			}
			if dropTol > 0 && math.Abs(wi) <= dropTol {
				w[i] = 0
				continue
			}
			wi *= uInvDiag[i]
			w[i] = wi
			start := uRowStart[i]
			end := start + uRowLen[i]
			for p := start; p < end; p++ {
				c := uColIdx[p]
				if c >= 0 && int(c) < len(w) {
					w[c] -= uRowVal[p] * wi
					if int(c) > lastNZ {
						lastNZ = int(c)
					}
				}
			}
		}
		return firstNZ, lastNZ
	}
	uOrder := f.uOrder[:m]
	uMinMoved := f.uMinMoved
	uMaxMoved := f.uMaxMoved
	outMin := m
	outMax := -1
	for i := firstNZ; i <= lastNZ && i < uMinMoved; i++ {
		wi := w[i]
		if wi == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wi) <= dropTol {
			w[i] = 0
			continue
		}
		wi *= uInvDiag[i]
		w[i] = wi
		if i < outMin {
			outMin = i
		}
		if i > outMax {
			outMax = i
		}
		start := uRowStart[i]
		end := start + uRowLen[i]
		for p := start; p < end; p++ {
			c := uColIdx[p]
			if c >= 0 && int(c) < len(w) {
				w[c] -= uRowVal[p] * wi
				if int(c) > lastNZ {
					lastNZ = int(c)
				}
			}
		}
	}
	if lastNZ < uMinMoved {
		return outMin, outMax
	}
	posEnd := max(lastNZ, uMaxMoved)
	if posEnd >= m {
		posEnd = m - 1
	}
	for pos := uMinMoved; pos <= posEnd; pos++ {
		i := uOrder[pos]
		wi := w[i]
		if wi == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wi) <= dropTol {
			w[i] = 0
			continue
		}
		wi *= uInvDiag[i]
		w[i] = wi
		if int(i) < outMin {
			outMin = int(i)
		}
		if int(i) > outMax {
			outMax = int(i)
		}
		start := uRowStart[i]
		end := start + uRowLen[i]
		for p := start; p < end; p++ {
			c := uColIdx[p]
			if c >= 0 && int(c) < len(w) {
				w[c] -= uRowVal[p] * wi
				if int(c) > posEnd {
					posEnd = int(c)
					if posEnd >= m {
						posEnd = m - 1
					}
				}
			}
		}
	}
	return outMin, outMax
}

func (f *LUFactorization) solveLTDense(w []float64, firstNZ, lastNZ int, dropTol float64) int {
	minNZ := firstNZ
	if !f.hasL {
		return minNZ
	}
	m := len(w)
	lRowPtr := f.lRowPtr[:m+1]
	lColIdx := f.lColIdx
	lRowVal := f.lRowVal[:len(lColIdx)]
	stopI := max(f.numColSing, minNZ)
	for i := lastNZ; i >= stopI; i-- {
		wi := w[i]
		if wi == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wi) <= dropTol {
			w[i] = 0
			continue
		}
		start, end := lRowPtr[i], lRowPtr[i+1]
		if start < 0 || int(end) > len(lColIdx) {
			continue
		}
		for p := start; p < end; p++ {
			c := lColIdx[p]
			if c >= 0 && int(c) < len(w) {
				w[c] -= lRowVal[p] * wi
				if int(c) < minNZ {
					minNZ = int(c)
					stopI = max(f.numColSing, minNZ)
				}
			}
		}
	}
	return minNZ
}

func (f *LUFactorization) applyFTEtasFTRANDense(w []float64, firstNZ, lastNZ int, dropTol float64) (int, int) {
	etas := f.etas
	if len(etas) == 0 {
		return firstNZ, lastNZ
	}
	etaIdx := f.etaIdx
	etaPool := f.etaPool[:len(etaIdx)]
	for e := range etas {
		eta := &etas[e]
		if eta.start >= eta.end || eta.maxIdx < asInt32(firstNZ) || eta.minIdx > asInt32(lastNZ) {
			continue
		}
		sum := 0.0
		for p := eta.start; p < eta.end; p++ {
			idx := etaIdx[p]
			if idx >= 0 && int(idx) < len(w) {
				sum += etaPool[p] * w[idx]
			}
		}
		if sum != 0 {
			pr := int(eta.pivotRow)
			if uint(pr) < uint(len(w)) {
				v := w[pr] - sum
				if dropTol > 0 && math.Abs(v) <= dropTol {
					w[pr] = 0
				} else {
					w[pr] = v
					if pr < firstNZ {
						firstNZ = pr
					}
					if pr > lastNZ {
						lastNZ = pr
					}
				}
			}
		}
	}
	return firstNZ, lastNZ
}

// FTRAN solves the forward basis system B * x = rhs in place, applying L, Forrest-Tomlin
// row-etas, and U in order.
func (f *LUFactorization) FTRAN(rhs []float64) {
	f.ftranDense(rhs, luDropTol)
}

// FTRANExact solves the forward basis system B * x = rhs in place without intermediate
// luDropTol truncation so sub-tolerance right-hand-side entries are preserved across U and etas.
func (f *LUFactorization) FTRANExact(rhs []float64) {
	f.ftranDense(rhs, 0)
}

func (f *LUFactorization) ftranDense(rhs []float64, dropTol float64) {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.uInvDiag) < m {
		return
	}
	rhs = rhs[:m]
	w := f.work[:m]
	perm := f.perm[:m]
	firstNZ := m
	lastNZ := -1
	for i, p := range perm {
		if p < 0 || int(p) >= len(rhs) {
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
	if lastNZ < 0 {
		return
	}

	lastNZ = f.solveLDense(w, firstNZ, lastNZ, dropTol)
	if len(f.etas) > 0 {
		firstNZ, lastNZ = f.applyFTEtasFTRANDense(w, firstNZ, lastNZ, dropTol)
	}
	minNZ, maxNZ := f.solveUDense(w, firstNZ, lastNZ, dropTol)

	colPerm := f.colPerm[:m]
	for k := minNZ; k <= maxNZ; k++ {
		if wk := w[k]; wk != 0 {
			w[k] = 0
			c := colPerm[k]
			if c >= 0 && int(c) < len(rhs) {
				rhs[c] = wk
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
	return f.ftranSparseCore(rhs, rhs, inIdx, nzIdx, true, false)
}

// FTRANSparseHintSpike solves B * x = rhs in place and captures the exact pre-U spike
// vector R_t * ... * R_1 * L^{-1} * P * rhs for a subsequent PushFTSpike update.
func (f *LUFactorization) FTRANSparseHintSpike(rhs []float64, inIdx, nzIdx []int) []int {
	return f.ftranSparseCore(rhs, rhs, inIdx, nzIdx, true, true)
}

// FTRANSparseFrom solves dst = B^{-1} * src out of place, assuming dst[:m] is zeroed,
// without mutating src.
func (f *LUFactorization) FTRANSparseFrom(dst, src []float64, srcIdx, dstIdx []int) []int {
	return f.ftranSparseCore(dst, src, srcIdx, dstIdx, false, false)
}

func (f *LUFactorization) ftranSparseCore(dst, src []float64, inIdx, outIdx []int, clearSrc, recordSpike bool) []int {
	m := f.m
	if !f.valid || m <= 0 || len(dst) < m || len(src) < m || len(f.work) < m || len(f.perm) < m || len(f.invPerm) < m || len(f.colPerm) < m || len(f.uInvDiag) < m {
		if recordSpike {
			f.spikeValid = false
		}
		return outIdx[:0]
	}

	dst = dst[:m]
	src = src[:m]
	w := f.work[:m]
	seeds := f.dfsSeeds[:0]
	firstNZ := m
	lastNZ := -1
	sparseInput := inIdx != nil && len(inIdx)*2 <= m

	if sparseInput {
		invPerm := f.invPerm[:m]
		for _, p := range inIdx {
			if uint(p) >= uint(len(src)) {
				continue
			}
			v := src[p]
			if v != 0 {
				if clearSrc {
					src[p] = 0
				}
				i := invPerm[p]
				if w[i] == 0 {
					seeds = append(seeds, i)
				}
				w[i] = v
				if int(i) < firstNZ {
					firstNZ = int(i)
				}
				if int(i) > lastNZ {
					lastNZ = int(i)
				}
			}
		}
	} else {
		perm := f.perm[:m]
		for i, p := range perm {
			if p < 0 || int(p) >= len(src) {
				continue
			}
			v := src[p]
			if v != 0 {
				if clearSrc {
					src[p] = 0
				}
				w[i] = v
				if lastNZ < 0 {
					firstNZ = i
				}
				lastNZ = i
			}
		}
	}

	outIdx = outIdx[:0]
	if lastNZ < 0 {
		if recordSpike {
			f.spikeIdx = f.spikeIdx[:0]
			f.spikeVal = f.spikeVal[:0]
			f.spikeValid = true
		}
		f.ftranEMA = (1 - hyperEMAAlpha) * f.ftranEMA
		return outIdx
	}

	useHyper := sparseInput && m >= hyperMinM && len(seeds) > 0 && len(seeds) <= m/16 && f.ftranEMA < hyperEMACutoff

	if cap(outIdx) < m {
		outIdx = make([]int, 0, m)
	}
	outBuf := outIdx[:m]
	outW := 0
	tol := luSolveDropTol
	colPerm := f.colPerm[:m]

	if useHyper {
		uOrder, ok := f.ftranHypersparseLU(w, seeds, recordSpike)
		if ok {
			for _, j := range uOrder {
				if wj := w[j]; wj != 0 {
					w[j] = 0
					if math.Abs(wj) > tol {
						c := int(colPerm[j])
						dst[c] = wj
						outBuf[outW] = c
						outW++
					}
				}
			}
			f.ftranEMA = (1-hyperEMAAlpha)*f.ftranEMA + hyperEMAAlpha*(float64(outW)/float64(m))
			return outBuf[:outW]
		}
	}

	lastNZ = f.solveLDense(w, firstNZ, lastNZ, luDropTol)
	if len(f.etas) > 0 {
		firstNZ, lastNZ = f.applyFTEtasFTRANDense(w, firstNZ, lastNZ, luDropTol)
	}
	if recordSpike {
		sIdx := f.spikeIdx[:0]
		sVal := f.spikeVal[:0]
		for k := firstNZ; k <= lastNZ; k++ {
			if wk := w[k]; wk != 0 {
				sIdx = append(sIdx, asInt32(k))
				sVal = append(sVal, wk)
			}
		}
		f.spikeIdx = sIdx
		f.spikeVal = sVal
		f.spikeValid = true
	}
	minNZ, maxNZ := f.solveUDense(w, firstNZ, lastNZ, luDropTol)
	for k := minNZ; k <= maxNZ; k++ {
		if wk := w[k]; wk != 0 {
			w[k] = 0
			if math.Abs(wk) > tol {
				c := int(colPerm[k])
				if uint(c) < uint(len(dst)) {
					dst[c] = wk
					outBuf[outW] = c
					outW++
				}
			}
		}
	}
	f.ftranEMA = (1-hyperEMAAlpha)*f.ftranEMA + hyperEMAAlpha*(float64(outW)/float64(m))
	return outBuf[:outW]
}

// ftranHypersparseLU solves L * R^{-1} * U * z = w over the DAG reach of seeds.
// Returns (nil, false) on L-reach abort without modifying w; on U-reach abort it completes U
// via dense sweep and returns the compacted active index list.
func (f *LUFactorization) ftranHypersparseLU(w []float64, seeds []int32, recordSpike bool) ([]int32, bool) {
	m := f.m
	limit := m / 10
	dropTol := luDropTol

	lOrder := seeds
	if f.hasL {
		var ok bool
		lOrder, ok = f.reachDAG(seeds, f.lColPtr, f.lRowIdx, limit)
		if !ok {
			return nil, false
		}
		lColPtr := f.lColPtr[:m+1]
		lRowIdx := f.lRowIdx
		lColVal := f.lColVal[:len(lRowIdx)]
		for t := len(lOrder) - 1; t >= 0; t-- {
			k := lOrder[t]
			wk := w[k]
			if wk == 0 {
				continue
			}
			if math.Abs(wk) <= dropTol {
				w[k] = 0
				continue
			}
			start, end := lColPtr[k], lColPtr[k+1]
			if start < 0 || int(end) > len(lRowIdx) {
				continue
			}
			for p := start; p < end; p++ {
				r := lRowIdx[p]
				if r >= 0 && int(r) < len(w) {
					w[r] -= lColVal[p] * wk
				}
			}
		}
	}

	uSeeds := f.dfsSeeds[:0]
	if f.hasL {
		for _, i := range lOrder {
			if w[i] != 0 {
				uSeeds = append(uSeeds, i)
			}
		}
	} else {
		wPos := 0
		for _, i := range lOrder {
			if w[i] != 0 {
				lOrder[wPos] = i
				wPos++
			}
		}
		uSeeds = lOrder[:wPos]
	}

	if len(f.etas) > 0 && len(uSeeds) > 0 {
		gen := f.nextDFSGen()
		mark := f.dfsMark[:m]
		wMin := int32(math.MaxInt32)
		wMax := int32(-1)
		for _, i := range uSeeds {
			mark[i] = gen
			if i < wMin {
				wMin = i
			}
			if i > wMax {
				wMax = i
			}
		}
		etas := f.etas
		etaIdx := f.etaIdx
		etaPool := f.etaPool[:len(etaIdx)]
		for e := range etas {
			eta := &etas[e]
			if eta.start >= eta.end || eta.maxIdx < wMin || eta.minIdx > wMax {
				continue
			}
			sum := 0.0
			for p := eta.start; p < eta.end; p++ {
				idx := etaIdx[p]
				if idx >= 0 && int(idx) < len(w) {
					sum += etaPool[p] * w[idx]
				}
			}
			if sum != 0 {
				pr := eta.pivotRow
				if pr >= 0 && int(pr) < len(w) {
					v := w[pr] - sum
					if math.Abs(v) <= dropTol {
						w[pr] = 0
					} else {
						w[pr] = v
						if mark[pr] != gen {
							mark[pr] = gen
							uSeeds = append(uSeeds, pr)
						}
						if pr < wMin {
							wMin = pr
						}
						if pr > wMax {
							wMax = pr
						}
					}
				}
			}
		}
	}

	if recordSpike {
		sIdx := f.spikeIdx[:0]
		sVal := f.spikeVal[:0]
		for _, i := range uSeeds {
			if wi := w[i]; wi != 0 {
				sIdx = append(sIdx, i)
				sVal = append(sVal, wi)
			}
		}
		f.spikeIdx = sIdx
		f.spikeVal = sVal
		f.spikeValid = true
	}

	if len(uSeeds) == 0 {
		return nil, true
	}

	uInvDiag := f.uInvDiag[:m]
	if !f.hasU {
		for _, i := range uSeeds {
			wi := w[i]
			if wi == 0 {
				continue
			}
			if math.Abs(wi) <= dropTol {
				w[i] = 0
				continue
			}
			w[i] = wi * uInvDiag[i]
		}
		return uSeeds, true
	}

	uOrder, ok := f.reachDAGRanges(uSeeds, f.uColStart, f.uColLen, f.uRowIdx, limit)
	if !ok {
		firstNZ := m
		lastNZ := -1
		for _, i := range uSeeds {
			if int(i) < firstNZ {
				firstNZ = int(i)
			}
			if int(i) > lastNZ {
				lastNZ = int(i)
			}
		}
		minNZ, maxNZ := f.solveUDense(w, firstNZ, lastNZ, dropTol)
		top := f.topOrder[:0]
		for k := minNZ; k <= maxNZ; k++ {
			if w[k] != 0 {
				top = append(top, asInt32(k))
			}
		}
		f.topOrder = top
		return top, true
	}

	uColStart := f.uColStart[:m]
	uColLen := f.uColLen[:m]
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal[:len(uRowIdx)]
	for t := len(uOrder) - 1; t >= 0; t-- {
		j := uOrder[t]
		wj := w[j]
		if wj == 0 {
			continue
		}
		if math.Abs(wj) <= dropTol {
			w[j] = 0
			continue
		}
		wj *= uInvDiag[j]
		w[j] = wj
		start := uColStart[j]
		end := start + uColLen[j]
		for p := start; p < end; p++ {
			r := uRowIdx[p]
			if r >= 0 && int(r) < len(w) {
				w[r] -= uColVal[p] * wj
			}
		}
	}
	return uOrder, true
}

// BTRAN solves the transposed basis system B^T * y = rhs in place, applying U^T,
// Forrest-Tomlin row-etas in reverse, and L^T.
func (f *LUFactorization) BTRAN(rhs []float64) {
	f.btranDense(rhs, luDropTol)
}

// BTRANExact solves B^T * y = rhs in place without applying intermediate luDropTol truncation,
// used to verify Farkas infeasibility certificates when sub-tolerance entries could otherwise vanish.
func (f *LUFactorization) BTRANExact(rhs []float64) {
	f.btranDense(rhs, 0)
}

// applyFTEtasBTRANDense applies Forrest-Tomlin row-etas R_e^T = I - r_e * e_{p_e}^T in reverse
// order to w, skipping each eta in O(1) when w[p_e] == 0.
func (f *LUFactorization) applyFTEtasBTRANDense(w []float64, firstNZ, lastNZ int, dropTol float64) (int, int) {
	etas := f.etas
	if len(etas) == 0 {
		return firstNZ, lastNZ
	}
	etaIdx := f.etaIdx
	etaPool := f.etaPool[:len(etaIdx)]
	for e := len(etas) - 1; e >= 0; e-- {
		eta := &etas[e]
		pr := eta.pivotRow
		if pr < 0 || int(pr) >= len(w) {
			continue
		}
		wp := w[pr]
		if wp == 0 {
			continue
		}
		if dropTol > 0 && math.Abs(wp) <= dropTol {
			w[pr] = 0
			continue
		}
		for p := eta.start; p < eta.end; p++ {
			idx := etaIdx[p]
			if idx >= 0 && int(idx) < len(w) {
				w[idx] -= etaPool[p] * wp
			}
		}
		if eta.start < eta.end {
			if int(eta.minIdx) < firstNZ {
				firstNZ = int(eta.minIdx)
			}
			if int(eta.maxIdx) > lastNZ {
				lastNZ = int(eta.maxIdx)
			}
		}
	}
	return firstNZ, lastNZ
}

func (f *LUFactorization) btranDense(rhs []float64, dropTol float64) {
	m := f.m
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.uInvDiag) < m {
		return
	}
	rhs = rhs[:m]
	w := f.work[:m]
	colPerm := f.colPerm[:m]
	firstNZ := m
	lastNZ := -1
	for k, c := range colPerm {
		if c < 0 || int(c) >= len(rhs) {
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

	firstNZ, lastNZ = f.solveUTDense(w, firstNZ, lastNZ, dropTol)
	if len(f.etas) > 0 && lastNZ >= firstNZ {
		firstNZ, lastNZ = f.applyFTEtasBTRANDense(w, firstNZ, lastNZ, dropTol)
	}
	if lastNZ < firstNZ {
		return
	}
	minNZ := f.solveLTDense(w, firstNZ, lastNZ, dropTol)

	perm := f.perm[:m]
	for i := minNZ; i <= lastNZ; i++ {
		if wi := w[i]; wi != 0 {
			w[i] = 0
			p := perm[i]
			if p >= 0 && int(p) < len(rhs) {
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
	if !f.valid || m <= 0 || len(rhs) < m || len(f.work) < m || len(f.perm) < m || len(f.colPerm) < m || len(f.invColPerm) < m || len(f.uInvDiag) < m {
		return nzIdx[:0]
	}
	rhs = rhs[:m]
	w := f.work[:m]
	sparseInput := inIdx != nil && len(inIdx)*2 <= m

	uSeeds := f.dfsSeeds[:0]
	wFirstNZ := m
	wLastNZ := -1

	if sparseInput {
		invColPerm := f.invColPerm[:m]
		for _, c := range inIdx {
			if uint(c) >= uint(len(rhs)) {
				continue
			}
			v := rhs[c]
			if v != 0 {
				rhs[c] = 0
				k := invColPerm[c]
				if w[k] == 0 {
					uSeeds = append(uSeeds, k)
				}
				w[k] = v
				if int(k) < wFirstNZ {
					wFirstNZ = int(k)
				}
				if int(k) > wLastNZ {
					wLastNZ = int(k)
				}
			}
		}
	} else {
		colPerm := f.colPerm[:m]
		for k, c := range colPerm {
			if c < 0 || int(c) >= len(rhs) {
				continue
			}
			v := rhs[c]
			if v != 0 {
				rhs[c] = 0
				w[k] = v
				if wLastNZ < 0 {
					wFirstNZ = k
				}
				wLastNZ = k
			}
		}
	}

	nzIdx = nzIdx[:0]
	if wLastNZ < 0 {
		f.btranEMA = (1 - hyperEMAAlpha) * f.btranEMA
		return nzIdx
	}

	if cap(nzIdx) < m {
		nzIdx = make([]int, 0, m)
	}
	outBuf := nzIdx[:m]

	useHyper := sparseInput && m >= hyperMinM && len(uSeeds) > 0 && len(uSeeds) <= m/16 && f.btranEMA < hyperEMACutoff
	if useHyper {
		outW, ok := f.btranHypersparse(rhs, w, uSeeds, outBuf)
		if ok {
			f.btranEMA = (1-hyperEMAAlpha)*f.btranEMA + hyperEMAAlpha*(float64(outW)/float64(m))
			return outBuf[:outW]
		}
	}

	wFirstNZ, wLastNZ = f.solveUTDense(w, wFirstNZ, wLastNZ, luDropTol)
	if len(f.etas) > 0 && wLastNZ >= wFirstNZ {
		wFirstNZ, wLastNZ = f.applyFTEtasBTRANDense(w, wFirstNZ, wLastNZ, luDropTol)
	}
	if wLastNZ < wFirstNZ {
		f.btranEMA = (1 - hyperEMAAlpha) * f.btranEMA
		return outBuf[:0]
	}
	minNZ := f.solveLTDense(w, wFirstNZ, wLastNZ, luDropTol)

	perm := f.perm[:m]
	tol := luDropTol
	outW := 0
	for i := minNZ; i <= wLastNZ; i++ {
		if wi := w[i]; wi != 0 {
			w[i] = 0
			if math.Abs(wi) > tol {
				p := int(perm[i])
				if uint(p) < uint(len(rhs)) {
					rhs[p] = wi
					outBuf[outW] = p
					outW++
				}
			}
		}
	}
	f.btranEMA = (1-hyperEMAAlpha)*f.btranEMA + hyperEMAAlpha*(float64(outW)/float64(m))
	return outBuf[:outW]
}

func (f *LUFactorization) btranHypersparse(rhs, w []float64, seeds []int32, outBuf []int) (int, bool) {
	m := f.m
	limit := m / 10
	uInvDiag := f.uInvDiag[:m]
	perm := f.perm[:m]
	tol := luDropTol

	uOrder := seeds
	if !f.hasU {
		for _, i := range seeds {
			wi := w[i]
			if wi == 0 {
				continue
			}
			if math.Abs(wi) <= tol {
				w[i] = 0
				continue
			}
			w[i] = wi * uInvDiag[i]
		}
	} else {
		var ok bool
		uOrder, ok = f.reachDAGRanges(seeds, f.uRowStart, f.uRowLen, f.uColIdx, limit)
		if !ok {
			return 0, false
		}
		uRowStart := f.uRowStart[:m]
		uRowLen := f.uRowLen[:m]
		uColIdx := f.uColIdx
		uRowVal := f.uRowVal[:len(uColIdx)]
		for t := len(uOrder) - 1; t >= 0; t-- {
			i := uOrder[t]
			wi := w[i]
			if wi == 0 {
				continue
			}
			if math.Abs(wi) <= tol {
				w[i] = 0
				continue
			}
			wi *= uInvDiag[i]
			w[i] = wi
			start := uRowStart[i]
			end := start + uRowLen[i]
			for p := start; p < end; p++ {
				c := uColIdx[p]
				if c >= 0 && int(c) < len(w) {
					w[c] -= uRowVal[p] * wi
				}
			}
		}
	}

	var lSeeds []int32
	if f.hasU {
		lSeeds = f.dfsSeeds[:0]
		for _, i := range uOrder {
			if w[i] != 0 {
				lSeeds = append(lSeeds, i)
			}
		}
	} else {
		wPos := 0
		for _, i := range uOrder {
			if w[i] != 0 {
				uOrder[wPos] = i
				wPos++
			}
		}
		lSeeds = uOrder[:wPos]
	}

	if len(f.etas) > 0 && len(lSeeds) > 0 {
		gen := f.nextDFSGen()
		mark := f.dfsMark[:m]
		for _, i := range lSeeds {
			mark[i] = gen
		}
		etas := f.etas
		etaIdx := f.etaIdx
		etaPool := f.etaPool[:len(etaIdx)]
		for e := len(etas) - 1; e >= 0; e-- {
			eta := &etas[e]
			pr := eta.pivotRow
			if pr < 0 || int(pr) >= len(w) {
				continue
			}
			wp := w[pr]
			if wp == 0 {
				continue
			}
			if math.Abs(wp) <= tol {
				w[pr] = 0
				continue
			}
			for p := eta.start; p < eta.end; p++ {
				j := etaIdx[p]
				if j >= 0 && int(j) < len(w) {
					w[j] -= etaPool[p] * wp
					if mark[j] != gen {
						mark[j] = gen
						lSeeds = append(lSeeds, j)
					}
				}
			}
		}
	}

	if len(lSeeds) == 0 {
		return 0, true
	}

	if !f.hasL {
		outW := 0
		for _, i := range lSeeds {
			if wi := w[i]; wi != 0 {
				w[i] = 0
				if math.Abs(wi) > tol {
					p := int(perm[i])
					rhs[p] = wi
					outBuf[outW] = p
					outW++
				}
			}
		}
		return outW, true
	}

	lOrder, ok := f.reachDAG(lSeeds, f.lRowPtr, f.lColIdx, limit)
	if !ok {
		firstNZ := m
		lastNZ := -1
		for _, i := range lSeeds {
			if int(i) < firstNZ {
				firstNZ = int(i)
			}
			if int(i) > lastNZ {
				lastNZ = int(i)
			}
		}
		minNZ := f.solveLTDense(w, firstNZ, lastNZ, tol)
		outW := 0
		for i := minNZ; i <= lastNZ; i++ {
			if wi := w[i]; wi != 0 {
				w[i] = 0
				if math.Abs(wi) > tol {
					p := int(perm[i])
					rhs[p] = wi
					outBuf[outW] = p
					outW++
				}
			}
		}
		return outW, true
	}

	lRowPtr := f.lRowPtr[:m+1]
	lColIdx := f.lColIdx
	lRowVal := f.lRowVal[:len(lColIdx)]
	for t := len(lOrder) - 1; t >= 0; t-- {
		i := lOrder[t]
		wi := w[i]
		if wi == 0 {
			continue
		}
		if math.Abs(wi) <= tol {
			w[i] = 0
			continue
		}
		start, end := lRowPtr[i], lRowPtr[i+1]
		if start < 0 || int(end) > len(lColIdx) {
			continue
		}
		for p := start; p < end; p++ {
			c := lColIdx[p]
			if c >= 0 && int(c) < len(w) {
				w[c] -= lRowVal[p] * wi
			}
		}
	}
	outW := 0
	for _, i := range lOrder {
		if wi := w[i]; wi != 0 {
			w[i] = 0
			if math.Abs(wi) > tol {
				p := int(perm[i])
				rhs[p] = wi
				outBuf[outW] = p
				outW++
			}
		}
	}
	return outW, true
}

func compactUArena(m int, starts, lens, caps []int32, idx *[]int32, val *[]float64, tmpIdx *[]int32, tmpVal *[]float64, extraNeed int32) int32 {
	total := int32(0)
	for k := range m {
		total += lens[k]
	}
	need := int(total + extraNeed)
	newCap := max(need*2, max(cap(*idx), 256))
	var newIdx []int32
	var newVal []float64
	if cap(*tmpIdx) >= newCap && cap(*tmpVal) >= newCap {
		newIdx = (*tmpIdx)[:cap(*tmpIdx)]
		newVal = (*tmpVal)[:cap(*tmpVal)]
	} else {
		newIdx = make([]int32, newCap)
		newVal = make([]float64, newCap)
	}
	pos := int32(0)
	oldIdx := *idx
	oldVal := *val
	for k := range m {
		l := lens[k]
		if l > 0 {
			st := starts[k]
			copy(newIdx[pos:pos+l], oldIdx[st:st+l])
			copy(newVal[pos:pos+l], oldVal[st:st+l])
		}
		starts[k] = pos
		caps[k] = l
		pos += l
	}
	*tmpIdx = oldIdx
	*tmpVal = oldVal
	*idx = newIdx
	*val = newVal
	return pos
}

func (f *LUFactorization) compactUCols(extraNeed int32) {
	f.uColFree = compactUArena(f.m, f.uColStart[:f.m], f.uColLen[:f.m], f.uColCap[:f.m], &f.uRowIdx, &f.uColVal, &f.uTmpIdx, &f.uTmpVal, extraNeed)
}

func (f *LUFactorization) compactURows(extraNeed int32) {
	f.uRowFree = compactUArena(f.m, f.uRowStart[:f.m], f.uRowLen[:f.m], f.uRowCap[:f.m], &f.uColIdx, &f.uRowVal, &f.uTmpIdx, &f.uTmpVal, extraNeed)
}

func (f *LUFactorization) removeURowEntry(r, c int32) {
	st := f.uRowStart[r]
	l := f.uRowLen[r]
	end := st + l
	uColIdx := f.uColIdx
	uRowVal := f.uRowVal
	for p := st; p < end; p++ {
		if uColIdx[p] == c {
			last := end - 1
			uColIdx[p] = uColIdx[last]
			uRowVal[p] = uRowVal[last]
			f.uRowLen[r] = l - 1
			return
		}
	}
}

func (f *LUFactorization) removeUColEntry(c, r int32) {
	st := f.uColStart[c]
	l := f.uColLen[c]
	end := st + l
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal
	for p := st; p < end; p++ {
		if uRowIdx[p] == r {
			last := end - 1
			uRowIdx[p] = uRowIdx[last]
			uColVal[p] = uColVal[last]
			f.uColLen[c] = l - 1
			return
		}
	}
}

func (f *LUFactorization) appendURowEntry(r, c int32, val float64) {
	l := f.uRowLen[r]
	if l >= f.uRowCap[r] {
		newCap := max(l*2+2, 4)
		if int(f.uRowFree+newCap) > len(f.uColIdx) {
			f.compactURows(newCap)
		}
		newStart := f.uRowFree
		if l > 0 {
			oldStart := f.uRowStart[r]
			copy(f.uColIdx[newStart:newStart+l], f.uColIdx[oldStart:oldStart+l])
			copy(f.uRowVal[newStart:newStart+l], f.uRowVal[oldStart:oldStart+l])
		}
		f.uRowStart[r] = newStart
		f.uRowCap[r] = newCap
		f.uRowFree = newStart + newCap
	}
	pos := f.uRowStart[r] + l
	f.uColIdx[pos] = c
	f.uRowVal[pos] = val
	f.uRowLen[r] = l + 1
}

func (f *LUFactorization) allocUColSlot(c, need int32) int32 {
	if need <= f.uColCap[c] {
		return f.uColStart[c]
	}
	newCap := max(need+4, need*2)
	if int(f.uColFree+newCap) > len(f.uRowIdx) {
		f.compactUCols(newCap)
	}
	st := f.uColFree
	f.uColStart[c] = st
	f.uColCap[c] = newCap
	f.uColFree = st + newCap
	return st
}

// PushFTSpike performs an in-place mutated-U Forrest-Tomlin update at pivotRow using the
// exact pre-U spike vector captured by FTRANSparseHintSpike.
func (f *LUFactorization) PushFTSpike(pivotRow int) error {
	m := f.m
	if !f.valid || !f.spikeValid || m <= 0 || uint(pivotRow) >= uint(m) {
		f.spikeValid = false
		return fmt.Errorf("invalid LU or spike state at pivot row %d for basis size %d", pivotRow, m)
	}
	f.spikeValid = false
	sw := f.spikeWork[:m]
	sIdx := f.spikeIdx
	sVal := f.spikeVal
	for k, r := range sIdx {
		sw[r] = sVal[k]
	}
	return f.pushFTFromSpike(pivotRow)
}

// PushEta performs an in-place mutated-U Forrest-Tomlin update at pivotRow from dense
// transformed column fCol = B^{-1} * a.
func (f *LUFactorization) PushEta(pivotRow int, fCol []float64) error {
	m := f.m
	if !f.valid || m <= 0 || uint(pivotRow) >= uint(m) || len(fCol) < m {
		return fmt.Errorf("invalid LU state or pivot row %d for basis size %d", pivotRow, m)
	}
	piv := fCol[pivotRow]
	if math.IsNaN(piv) || math.IsInf(piv, 0) || math.Abs(piv) < luPivotTol {
		return fmt.Errorf("tiny eta pivot %.3e at row %d", piv, pivotRow)
	}
	f.spikeValid = false
	sw := f.spikeWork[:m]
	sIdx := f.spikeIdx[:0]
	gen := f.nextDFSGen()
	mark := f.dfsMark[:m]
	invColPerm := f.invColPerm[:m]
	uDiag := f.uDiag[:m]
	uColStart := f.uColStart[:m]
	uColLen := f.uColLen[:m]
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal
	tol := luSolveDropTol
	for i, fi := range fCol[:m] {
		if math.Abs(fi) <= tol {
			continue
		}
		k := invColPerm[i]
		if mark[k] != gen {
			mark[k] = gen
			sIdx = append(sIdx, k)
		}
		sw[k] += uDiag[k] * fi
		if f.hasU {
			st := uColStart[k]
			end := st + uColLen[k]
			for q := st; q < end; q++ {
				r := uRowIdx[q]
				if mark[r] != gen {
					mark[r] = gen
					sIdx = append(sIdx, r)
				}
				sw[r] += uColVal[q] * fi
			}
		}
	}
	f.spikeIdx = sIdx
	return f.pushFTFromSpike(pivotRow)
}

// PushEtaSparse performs an in-place mutated-U Forrest-Tomlin update at pivotRow from
// sparse transformed column (fCol, fColIdx) = B^{-1} * a.
func (f *LUFactorization) PushEtaSparse(pivotRow int, fCol []float64, fColIdx []int) error {
	m := f.m
	if !f.valid || m <= 0 || uint(pivotRow) >= uint(m) || len(fCol) < m {
		return fmt.Errorf("invalid LU state or pivot row %d for basis size %d", pivotRow, m)
	}
	piv := fCol[pivotRow]
	if math.IsNaN(piv) || math.IsInf(piv, 0) || math.Abs(piv) < luPivotTol {
		return fmt.Errorf("tiny eta pivot %.3e at row %d", piv, pivotRow)
	}
	f.spikeValid = false
	sw := f.spikeWork[:m]
	sIdx := f.spikeIdx[:0]
	// Deduplicate fColIdx using a separate mark pass via workInt? Use dfsMark with 2 gens:
	// first pass deduplicates fColIdx into f.dfsSeeds, second pass scatters U * (Q^T f) into sw.
	inGen := f.nextDFSGen()
	mark := f.dfsMark[:m]
	uniq := f.dfsSeeds[:0]
	for _, i := range fColIdx {
		if uint(i) < uint(m) && mark[i] != inGen {
			mark[i] = inGen
			uniq = append(uniq, asInt32(i))
		}
	}
	outGen := f.nextDFSGen()
	invColPerm := f.invColPerm[:m]
	uDiag := f.uDiag[:m]
	uColStart := f.uColStart[:m]
	uColLen := f.uColLen[:m]
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal
	tol := luSolveDropTol
	for _, i := range uniq {
		fi := fCol[i]
		if math.Abs(fi) <= tol {
			continue
		}
		k := invColPerm[i]
		if mark[k] != outGen {
			mark[k] = outGen
			sIdx = append(sIdx, k)
		}
		sw[k] += uDiag[k] * fi
		if f.hasU {
			st := uColStart[k]
			end := st + uColLen[k]
			for q := st; q < end; q++ {
				r := uRowIdx[q]
				if mark[r] != outGen {
					mark[r] = outGen
					sIdx = append(sIdx, r)
				}
				sw[r] += uColVal[q] * fi
			}
		}
	}
	f.spikeIdx = sIdx
	return f.pushFTFromSpike(pivotRow)
}

func (f *LUFactorization) pushFTFromSpike(pivotRow int) error {
	m := f.m
	sw := f.spikeWork[:m]
	sIdx := f.spikeIdx
	defer func() {
		for _, r := range sIdx {
			sw[r] = 0
		}
	}()

	p := f.invColPerm[pivotRow]
	posP := p
	if f.uPermuted {
		posP = f.uPos[p]
	}

	tol := luSolveDropTol
	maxPos := int32(-1)
	offDiagCount := int32(0)
	for _, r := range sIdx {
		v := sw[r]
		av := math.Abs(v)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("non-finite spike entry at row %d", r)
		}
		if av <= tol {
			sw[r] = 0
			continue
		}
		rp := r
		if f.uPermuted {
			rp = f.uPos[r]
		}
		if rp > maxPos {
			maxPos = rp
		}
		if r != p {
			offDiagCount++
		}
	}

	vp := sw[p]
	if maxPos <= posP {
		// Fast path: spike is already upper-triangular in current uOrder; no row-eta or permutation needed.
		if math.Abs(vp) < luPivotTol {
			return fmt.Errorf("tiny FT diagonal %.3e at pivot %d", vp, p)
		}
		f.replaceUColumn(p, sIdx, sw, offDiagCount)
		f.uDiag[p] = vp
		invPiv := 1.0 / vp
		f.uInvDiag[p] = invPiv
		if ai := math.Abs(invPiv); ai > f.maxUpdatePivotInv {
			f.maxUpdatePivotInv = ai
		}
		f.numUpdates++
		return nil
	}

	// General Forrest-Tomlin update: eliminate old off-diagonal row p of U into a row-eta R = I - e_p * r^T,
	// replace column p with the spike, and move p to the end of uOrder (m-1).
	if !f.uPermuted {
		uOrder := f.uOrder[:m]
		uPos := f.uPos[:m]
		for i := range m {
			i32 := asInt32(i)
			uOrder[i] = i32
			uPos[i] = i32
		}
	}

	origEtaIdxLen := len(f.etaIdx)
	dotRV := 0.0
	maxRVTerm := 0.0
	maxAbsR := 0.0
	etaStart := asInt32(origEtaIdxLen)
	etaMin := int32(math.MaxInt32)
	etaMax := int32(-1)

	if f.uRowLen[p] > 0 {
		ftWork := f.ftWork[:m]
		seeds := f.dfsSeeds[:0]
		hSt := f.uRowStart[p]
		hEnd := hSt + f.uRowLen[p]
		uColIdx := f.uColIdx
		uRowVal := f.uRowVal
		for q := hSt; q < hEnd; q++ {
			c := uColIdx[q]
			v := uRowVal[q]
			if v != 0 {
				ftWork[c] = v
				seeds = append(seeds, c)
			}
		}

		if len(seeds) > 0 {
			uInvDiag := f.uInvDiag[:m]
			uRowStart := f.uRowStart[:m]
			uRowLen := f.uRowLen[:m]
			order, ok := f.reachDAGRanges(seeds, uRowStart, uRowLen, uColIdx, m/4)
			if ok {
				for t := len(order) - 1; t >= 0; t-- {
					i := order[t]
					wi := ftWork[i]
					if wi == 0 {
						continue
					}
					if math.Abs(wi) <= tol {
						ftWork[i] = 0
						continue
					}
					wi *= uInvDiag[i]
					ftWork[i] = wi
					st := uRowStart[i]
					end := st + uRowLen[i]
					for q := st; q < end; q++ {
						c := uColIdx[q]
						ftWork[c] -= uRowVal[q] * wi
					}
				}
				etaIdx := f.etaIdx
				etaPool := f.etaPool
				for _, i := range order {
					ri := ftWork[i]
					ftWork[i] = 0
					ari := math.Abs(ri)
					if ari <= tol {
						continue
					}
					if ari > maxAbsR {
						maxAbsR = ari
					}
					if len(etaIdx) == cap(etaIdx) {
						etaIdx = growSlice(etaIdx, len(etaIdx)+16)
						etaPool = growSlice(etaPool, len(etaPool)+16)
					}
					etaIdx = append(etaIdx, i)
					etaPool = append(etaPool, ri)
					if i < etaMin {
						etaMin = i
					}
					if i > etaMax {
						etaMax = i
					}
					if vi := sw[i]; vi != 0 {
						term := ri * vi
						dotRV += term
						if at := math.Abs(term); at > maxRVTerm {
							maxRVTerm = at
						}
					}
				}
				f.etaIdx = etaIdx
				f.etaPool = etaPool
			} else {
				uOrder := f.uOrder[:m]
				etaIdx := f.etaIdx
				etaPool := f.etaPool
				for pos := int(posP) + 1; pos < m; pos++ {
					i := uOrder[pos]
					wi := ftWork[i]
					ftWork[i] = 0
					if wi == 0 {
						continue
					}
					if math.Abs(wi) <= tol {
						continue
					}
					wi *= uInvDiag[i]
					st := uRowStart[i]
					end := st + uRowLen[i]
					for q := st; q < end; q++ {
						c := uColIdx[q]
						ftWork[c] -= uRowVal[q] * wi
					}
					ari := math.Abs(wi)
					if ari <= tol {
						continue
					}
					if ari > maxAbsR {
						maxAbsR = ari
					}
					if len(etaIdx) == cap(etaIdx) {
						etaIdx = growSlice(etaIdx, len(etaIdx)+16)
						etaPool = growSlice(etaPool, len(etaPool)+16)
					}
					etaIdx = append(etaIdx, i)
					etaPool = append(etaPool, wi)
					if i < etaMin {
						etaMin = i
					}
					if i > etaMax {
						etaMax = i
					}
					if vi := sw[i]; vi != 0 {
						term := wi * vi
						dotRV += term
						if at := math.Abs(term); at > maxRVTerm {
							maxRVTerm = at
						}
					}
				}
				f.etaIdx = etaIdx
				f.etaPool = etaPool
			}
		}
	}

	uNew := vp - dotRV
	scale := max(math.Abs(vp), maxRVTerm)
	if math.IsNaN(uNew) || math.IsInf(uNew, 0) || math.Abs(uNew) < luPivotTol ||
		maxAbsR > ftMaxRowMult || (maxRVTerm > 0 && scale > 0 && math.Abs(uNew) < ftCancelTol*scale) {
		f.etaIdx = f.etaIdx[:origEtaIdxLen]
		f.etaPool = f.etaPool[:origEtaIdxLen]
		return fmt.Errorf("unstable FT update at pivot %d (uNew=%.3e, maxR=%.3e)", p, uNew, maxAbsR)
	}

	// Commit in-place mutation of U:
	// 1. Delete old off-diagonal entries of row p from their columns in U CSC and clear row p.
	if f.uRowLen[p] > 0 {
		hSt := f.uRowStart[p]
		hEnd := hSt + f.uRowLen[p]
		for q := hSt; q < hEnd; q++ {
			f.removeUColEntry(f.uColIdx[q], p)
		}
		f.uLiveNNZ -= int(f.uRowLen[p])
		f.uRowLen[p] = 0
	}

	// 2. Replace column p with the off-diagonal entries of spike v.
	f.replaceUColumn(p, sIdx, sw, offDiagCount)
	f.uDiag[p] = uNew
	invPiv := 1.0 / uNew
	f.uInvDiag[p] = invPiv
	if ai := math.Abs(invPiv); ai > f.maxUpdatePivotInv {
		f.maxUpdatePivotInv = ai
	}

	// 3. Move p from posP to dstPos (= maxPos) in uOrder and update uPos.
	uOrder := f.uOrder[:m]
	uPos := f.uPos[:m]
	dstPos := int(maxPos)
	if int(posP) < dstPos {
		copy(uOrder[posP:dstPos], uOrder[int(posP)+1:dstPos+1])
		uOrder[dstPos] = p
		for pos := int(posP); pos <= dstPos; pos++ {
			uPos[uOrder[pos]] = asInt32(pos)
		}
		if int(posP) < f.uMinMoved {
			f.uMinMoved = int(posP)
		}
		if dstPos > f.uMaxMoved {
			f.uMaxMoved = dstPos
		}
		f.uPermuted = true
	}

	// 4. Append row-eta if non-empty.
	etaEnd := asInt32(len(f.etaIdx))
	if etaEnd > etaStart {
		if len(f.etas) == cap(f.etas) {
			f.etas = growSlice(f.etas, len(f.etas)+16)
		}
		f.etas = append(f.etas, etaVector{
			pivotRow: p,
			start:    etaStart,
			end:      etaEnd,
			minIdx:   etaMin,
			maxIdx:   etaMax,
		})
	}
	f.numUpdates++
	return nil
}

func (f *LUFactorization) replaceUColumn(p int32, sIdx []int32, sw []float64, offDiagCount int32) {
	// Remove old column p entries from their rows in U CSR.
	oldSt := f.uColStart[p]
	oldLen := f.uColLen[p]
	if oldLen > 0 {
		uRowIdx := f.uRowIdx
		for q := oldSt; q < oldSt+oldLen; q++ {
			f.removeURowEntry(uRowIdx[q], p)
		}
		f.uLiveNNZ -= int(oldLen)
		f.uColLen[p] = 0
	}
	if offDiagCount == 0 {
		return
	}
	f.hasU = true
	dstSt := f.allocUColSlot(p, offDiagCount)
	uRowIdx := f.uRowIdx
	uColVal := f.uColVal
	pos := dstSt
	for _, r := range sIdx {
		if r == p {
			continue
		}
		v := sw[r]
		if v == 0 {
			continue
		}
		uRowIdx[pos] = r
		uColVal[pos] = v
		pos++
		f.appendURowEntry(r, p, v)
	}
	written := pos - dstSt
	f.uColLen[p] = written
	f.uLiveNNZ += int(written)
}
