package goblin

import (
	"math"
	"sort"
)

// DenseThreshold is the maximum number of variables for which CompiledProblem
// also prebuilds a contiguous SIMD-aligned dense row matrix.
const DenseThreshold = 256

const (
	bitsPerWord        = 64
	wordBitMask        = bitsPerWord - 1
	simdRowAlign       = 8
	simdRowAlignMask   = simdRowAlign - 1
	defaultTouchedCap  = 64
	maxDenseRowFloats  = 1 << 20
	minDenseRowCheckMC = 256
	denseSparsityMult  = 16
)

// cbcInfinity matches COIN_DBL_MAX (DBL_MAX) used by COIN-OR Clp and Cbc for unbounded sides.
const cbcInfinity = math.MaxFloat64

func clampCBCBound(v float64) float64 {
	if math.IsInf(v, -1) || v < -cbcInfinity {
		return -cbcInfinity
	}
	if math.IsInf(v, 1) || v > cbcInfinity {
		return cbcInfinity
	}
	return v
}

// CompiledProblem is an immutable, flat matrix layout of an LP or MILP model
// built for cache locality, SIMD row sweeps, pooled solver workspaces,
// and zero-copy C++ COIN-OR CBC / OsiClpSolverInterface calls.
type CompiledProblem struct {
	name       string
	sense      Sense
	invalidErr error
	modelID    uint64

	numVars        int
	numConstraints int
	isMIP          bool

	// Variable metadata (length = NumVars)
	varNames []string
	varLB    []float64
	varUB    []float64
	varCat   []VarCategory

	// Bitmasks for fast integer/binary checks (each uint64 covers 64 variables).
	binaryMask  []uint64
	integerMask []uint64
	intCols     []int   // Indices of all Binary or Integer variables
	intCols32   []int32 // 32-bit indices for direct C++ CBC setInteger calls

	// Normalized objective: the internal solver always minimizes ObjMin[j]*x[j].
	// Original objective value = Sense * (sum_j ObjMin[j]*x[j]) + ObjConstant.
	objName        string
	objCoeffs      []float64 // Original objective coefficients as specified
	objMin         []float64 // Objective coefficients normalized for minimizing
	objConstant    float64
	objConstMinVal float64

	// Constraint metadata (length = NumConstraints)
	rowNames []string
	rowSense []ConstraintSense
	rowRHS   []float64
	rowLB    []float64 // Equivalent ranged row lower bound: RowLB[i] <= a_i^T x <= RowUB[i]
	rowUB    []float64 // Equivalent ranged row upper bound

	// CSR (Compressed Sparse Row) representation of constraint matrix A
	rowPtr []int
	colIdx []int
	rowVal []float64

	// CSC (Compressed Sparse Column) representation of constraint matrix A
	colPtr []int
	rowIdx []int
	colVal []float64

	// Pre-clamped and 32-bit-indexed CSC arrays for zero-copy C++ OsiClpSolverInterface::loadProblem
	colPtr32 []int32
	rowIdx32 []int32
	colLBClp []float64
	colUBClp []float64
	rowLBClp []float64
	rowUBClp []float64

	// Contiguous SIMD-padded dense rows when NumVars <= DenseThreshold.
	// RowStride is NumVars rounded up to a multiple of 8 float64s (64 bytes).
	rowStride int
	denseRows []float64
}

// CompileChecked validates the Model and returns a CompiledProblem or the first check error.
func (m *Model) CompileChecked() (*CompiledProblem, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m.compile(nil), nil
}

// Compile transforms a symbolic Model into a flat, canonical CompiledProblem.
// Duplicate variable terms within the objective or within a single constraint
// are summed and zero coefficients are dropped.
func (m *Model) Compile() *CompiledProblem {
	if m == nil {
		return nil
	}
	return m.compile(m.Validate())
}

func (m *Model) compile(validatedErr error) *CompiledProblem {
	n := len(m.vars)
	mc := len(m.constraints)
	modelID := m.id
	if modelID == 0 {
		modelID = nextModelID.Add(1)
	}
	sense := m.Sense()

	constMinVal := m.objConst
	if sense == Maximize && constMinVal != 0 {
		constMinVal = -constMinVal
	}

	numWords := (n + wordBitMask) / bitsPerWord
	p := &CompiledProblem{
		name:           m.Name(),
		sense:          sense,
		invalidErr:     validatedErr,
		modelID:        modelID,
		numVars:        n,
		numConstraints: mc,
		varNames:       make([]string, n),
		varLB:          make([]float64, n),
		varUB:          make([]float64, n),
		varCat:         make([]VarCategory, n),
		binaryMask:     make([]uint64, numWords),
		integerMask:    make([]uint64, numWords),
		objName:        m.objName,
		objCoeffs:      make([]float64, n),
		objMin:         make([]float64, n),
		objConstant:    m.objConst,
		objConstMinVal: constMinVal,
		rowNames:       make([]string, mc),
		rowSense:       make([]ConstraintSense, mc),
		rowRHS:         make([]float64, mc),
		rowLB:          make([]float64, mc),
		rowUB:          make([]float64, mc),
		rowPtr:         make([]int, mc+1),
		colLBClp:       make([]float64, n),
		colUBClp:       make([]float64, n),
		rowLBClp:       make([]float64, mc),
		rowUBClp:       make([]float64, mc),
	}

	for j, v := range m.vars {
		cat := v.cat
		if cat == Integer && v.lb >= 0 && v.ub <= 1 && v.lb <= v.ub {
			cat = Binary
		}
		p.varNames[j] = v.name
		p.varLB[j] = v.lb
		p.varUB[j] = v.ub
		p.varCat[j] = cat
		p.colLBClp[j] = clampCBCBound(v.lb)
		p.colUBClp[j] = clampCBCBound(v.ub)
		if cat != Continuous {
			p.isMIP = true
			p.integerMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			p.intCols = append(p.intCols, j)
			if j <= math.MaxInt32 {
				p.intCols32 = append(p.intCols32, int32(j))
			}
			if cat == Binary {
				p.binaryMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			}
		}
	}

	// Accumulate objective terms, routing any duplicate variable columns through wideDotAcc.
	objCounts := make([]uint8, n)
	var objDupCols []int
	for _, t := range m.objTerms {
		if col, ok := m.validVar(t.Var); ok && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) && t.Coeff != 0 {
			switch objCounts[col] {
			case 0:
				objCounts[col] = 1
				p.objCoeffs[col] = t.Coeff
			case 1:
				objCounts[col] = 2
				objDupCols = append(objDupCols, col)
			}
		}
	}
	for _, col := range objDupCols {
		var w wideDotAcc
		for _, t := range m.objTerms {
			if c, ok := m.validVar(t.Var); ok && c == col && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) {
				w.addVal(t.Coeff)
			}
		}
		p.objCoeffs[col] = w.finish(math.NaN())
	}
	senseSign := 1.0
	if m.sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		if p.objCoeffs[j] != 0 {
			p.objMin[j] = senseSign * p.objCoeffs[j]
		}
	}

	totalNNZ := 0
	for i := range mc {
		totalNNZ += len(m.constraints[i].terms)
	}

	// If n <= DenseThreshold and total dense footprint stays bounded, allocate SIMD-aligned dense rows.
	stride := (n + simdRowAlignMask) &^ simdRowAlignMask
	useDense := n > 0 && n <= DenseThreshold && mc*stride <= maxDenseRowFloats &&
		(mc <= minDenseRowCheckMC || totalNNZ*denseSparsityMult >= mc*n)
	if useDense {
		p.rowStride = stride
		p.denseRows = make([]float64, mc*p.rowStride)
	}

	// Scratch buffers for deduplicating terms in each constraint row
	rowAcc := make([]float64, n)
	touched := make([]int, 0, min(n, defaultTouchedCap))
	inTouched := make([]uint8, n)

	p.colIdx = make([]int, 0, totalNNZ)
	p.rowVal = make([]float64, 0, totalNNZ)

	for i, c := range m.constraints {
		p.rowNames[i] = c.name
		p.rowSense[i] = c.sense
		p.rowRHS[i] = c.rhs

		switch c.sense {
		case LessEqual:
			p.rowLB[i] = math.Inf(-1)
			p.rowUB[i] = c.rhs
		case GreaterEqual:
			p.rowLB[i] = c.rhs
			p.rowUB[i] = math.Inf(1)
		case Equal:
			p.rowLB[i] = c.rhs
			p.rowUB[i] = c.rhs
		}
		p.rowLBClp[i] = clampCBCBound(p.rowLB[i])
		p.rowUBClp[i] = clampCBCBound(p.rowUB[i])

		touched = touched[:0]
		rowHasDup := false
		for _, t := range c.terms {
			col, ok := m.validVar(t.Var)
			if !ok || t.Coeff == 0 || math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
				continue
			}
			if inTouched[col] == 0 {
				inTouched[col] = 1
				touched = append(touched, col)
				rowAcc[col] = t.Coeff
			} else {
				inTouched[col] = 2
				rowHasDup = true
			}
		}
		if rowHasDup {
			for _, col := range touched {
				if inTouched[col] > 1 {
					var w wideDotAcc
					for _, t := range c.terms {
						if tc, ok := m.validVar(t.Var); ok && tc == col && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) {
							w.addVal(t.Coeff)
						}
					}
					rowAcc[col] = w.finish(math.NaN())
				}
			}
		}
		sort.Ints(touched)

		p.rowPtr[i] = len(p.colIdx)
		var denseRow []float64
		if useDense {
			denseRow = p.denseRows[i*p.rowStride : (i+1)*p.rowStride]
		}
		for _, col := range touched {
			val := rowAcc[col]
			rowAcc[col] = 0
			inTouched[col] = 0
			if val == 0 {
				continue
			}
			p.colIdx = append(p.colIdx, col)
			p.rowVal = append(p.rowVal, val)
			if useDense {
				denseRow[col] = val
			}
		}
	}
	p.rowPtr[mc] = len(p.colIdx)

	// Build CSC (Compressed Sparse Column) transpose from CSR
	nnz := len(p.colIdx)
	fit32 := n <= math.MaxInt32 && mc <= math.MaxInt32 && nnz <= math.MaxInt32
	p.colPtr = make([]int, n+1)
	p.rowIdx = make([]int, nnz)
	p.colVal = make([]float64, nnz)
	if fit32 {
		p.colPtr32 = make([]int32, n+1)
		p.rowIdx32 = make([]int32, nnz)
	}

	for _, col := range p.colIdx {
		p.colPtr[col+1]++
	}
	for j := range n {
		p.colPtr[j+1] += p.colPtr[j]
		if fit32 {
			p.colPtr32[j+1] = int32(p.colPtr[j+1])
		}
	}
	writePos := make([]int, n)
	copy(writePos, p.colPtr[:n])
	for i := range mc {
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			pos := writePos[col]
			p.rowIdx[pos] = i
			if fit32 {
				p.rowIdx32[pos] = int32(i)
			}
			p.colVal[pos] = p.rowVal[k]
			writePos[col]++
		}
	}

	return p
}

// ObjectiveFromMin converts an internal minimized objective value back to the
// user's original sense and adds ObjConstant.
func (p *CompiledProblem) ObjectiveFromMin(minObj float64) float64 {
	if p == nil {
		return 0
	}
	v := minObj
	if p.sense == Maximize {
		v = -minObj
	}
	res := v + p.objConstant
	if math.IsInf(res, 0) && !math.IsInf(v, 0) && !math.IsInf(p.objConstant, 0) && (v > 0) != (p.objConstant > 0) {
		return math.Ldexp(math.Ldexp(v, -2)+math.Ldexp(p.objConstant, -2), 2)
	}
	return res
}

func (p *CompiledProblem) objConstMin() float64 {
	if p == nil || p.objConstant == 0 {
		return 0
	}
	if p.sense == Maximize {
		return -p.objConstant
	}
	return p.objConstant
}

func (p *CompiledProblem) userObjFromShiftedMin(shiftedMin float64) float64 {
	if p == nil {
		return 0
	}
	if p.sense == Maximize && shiftedMin != 0 {
		return -shiftedMin
	}
	return shiftedMin
}

func satAddNonNeg(a, b float64) float64 {
	s := a + b
	if math.IsInf(s, 1) {
		return math.MaxFloat64
	}
	return s
}

const (
	wideHiShift   = 1056
	wideHalfShift = 528
	wideExpCutoff = 960
	expansionCap  = 64
)

func growExpansion(e []float64, b float64) ([]float64, bool) {
	if b == 0 {
		return e, true
	}
	if math.IsNaN(b) || math.IsInf(b, 0) {
		return e, false
	}
	q := b
	w := 0
	for _, en := range e {
		var h float64
		q, h = twoSum(q, en)
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return e[:w], false
		}
		if h != 0 {
			e[w] = h
			w++
		}
	}
	e = e[:w]
	if q != 0 {
		e = append(e, q)
	}
	return e, true
}

func compressExpansion(e []float64) []float64 {
	n := len(e)
	if n == 0 {
		return e
	}
	q := e[n-1]
	bottom := n - 1
	for i := n - 2; i >= 0; i-- {
		newQ, err := twoSum(q, e[i])
		if err != 0 {
			e[bottom] = newQ
			bottom--
			q = err
		} else {
			q = newQ
		}
	}
	e[bottom] = q
	w := 0
	q = e[bottom]
	for i := bottom + 1; i < n; i++ {
		newQ, err := twoSum(e[i], q)
		if err != 0 {
			e[w] = err
			w++
		}
		q = newQ
	}
	if q != 0 {
		e[w] = q
		return e[:w+1]
	}
	return e[:w]
}

func expansionTopRem(comp []float64) (top, rem float64) {
	if len(comp) == 0 {
		return 0, 0
	}
	if len(comp) == 1 {
		return comp[0], 0
	}
	for i := range len(comp) - 1 {
		rem += comp[i]
	}
	return twoSum(comp[len(comp)-1], rem)
}

func roundToOddSticky(top, rem float64) float64 {
	if rem == 0 || math.Float64bits(top)&1 != 0 {
		return top
	}
	if (top > 0) == (rem > 0) {
		return math.Nextafter(top, math.Copysign(math.Inf(1), top))
	}
	return math.Nextafter(top, 0)
}

// wideDotAcc accumulates values and products across three power-of-two Shewchuk expansions
// (2^1056, 2^0, and 2^-1056) so finite float64 products never overflow, underflow, or cancel inaccurately.
type wideDotAcc struct {
	hi     []float64
	mid    []float64
	lo     []float64
	hiBuf  [expansionCap]float64
	midBuf [expansionCap]float64
	loBuf  [expansionCap]float64
	nonFin bool
}

func (w *wideDotAcc) ensureInit() {
	if w.hi == nil {
		w.hi = w.hiBuf[:0]
		w.mid = w.midBuf[:0]
		w.lo = w.loBuf[:0]
	}
}

func (w *wideDotAcc) addVal(v float64) {
	if v == 0 || w.nonFin {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.nonFin = true
		return
	}
	w.ensureInit()
	_, e := math.Frexp(math.Abs(v))
	var ok bool
	switch {
	case e > wideExpCutoff:
		w.hi, ok = growExpansion(w.hi, math.Ldexp(v, -wideHiShift))
	case e < -wideExpCutoff:
		w.lo, ok = growExpansion(w.lo, math.Ldexp(v, wideHiShift))
	default:
		w.mid, ok = growExpansion(w.mid, v)
	}
	if !ok {
		w.nonFin = true
	}
}

func (w *wideDotAcc) addProd(a, b float64) {
	if a == 0 || b == 0 || w.nonFin {
		return
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		w.nonFin = true
		return
	}
	w.ensureInit()
	_, ea := math.Frexp(math.Abs(a))
	_, eb := math.Frexp(math.Abs(b))
	sumExp := ea + eb
	var ok1, ok2 bool
	switch {
	case sumExp > wideExpCutoff:
		p, pe := twoProd(math.Ldexp(a, -wideHalfShift), math.Ldexp(b, -wideHalfShift))
		w.hi, ok1 = growExpansion(w.hi, pe)
		w.hi, ok2 = growExpansion(w.hi, p)
	case sumExp < -wideExpCutoff:
		p, pe := twoProd(math.Ldexp(a, wideHalfShift), math.Ldexp(b, wideHalfShift))
		w.lo, ok1 = growExpansion(w.lo, pe)
		w.lo, ok2 = growExpansion(w.lo, p)
	default:
		p, pe := twoProd(a, b)
		w.mid, ok1 = growExpansion(w.mid, pe)
		w.mid, ok2 = growExpansion(w.mid, p)
	}
	if !ok1 || !ok2 {
		w.nonFin = true
	}
}

func (w *wideDotAcc) finish(fallback float64) float64 {
	top, rem, ok := w.finishDD()
	if !ok {
		if w.nonFin {
			return fallback
		}
		return top
	}
	comb := top + rem
	if math.IsNaN(comb) {
		return top
	}
	return comb
}

func (w *wideDotAcc) finishDD() (top, rem float64, ok bool) {
	if w.nonFin {
		return 0, 0, false
	}
	if w.hi == nil {
		return 0, 0, true
	}
	w.hi = compressExpansion(w.hi)
	w.mid = compressExpansion(w.mid)
	w.lo = compressExpansion(w.lo)

	var midBuf [3 * expansionCap]float64
	midMerged := midBuf[:0]
	if len(w.hi) == 0 {
		midMerged = append(midMerged, w.mid...)
	} else {
		hiTop := w.hi[len(w.hi)-1]
		_, eHi := math.Frexp(math.Abs(hiTop))
		if eHi+wideHiShift > 1026 {
			return math.Copysign(math.Inf(1), hiTop), 0, false
		}
		var halfBuf [2 * expansionCap]float64
		half := halfBuf[:0]
		for _, h := range w.hi {
			half, _ = growExpansion(half, math.Ldexp(h, wideHalfShift))
		}
		for _, m := range w.mid {
			if math.Abs(m) >= 0x1p-400 {
				half, _ = growExpansion(half, math.Ldexp(m, -wideHalfShift))
			}
		}
		half = compressExpansion(half)
		if len(half) > 0 {
			halfTop := half[len(half)-1]
			_, eHalf := math.Frexp(math.Abs(halfTop))
			if eHalf+wideHalfShift > 1024 {
				var sc4Buf [3 * expansionCap]float64
				sc4 := sc4Buf[:0]
				for _, m := range w.mid {
					if math.Abs(m) < 0x1p-400 && math.Abs(m) >= 0x1p-1018 {
						sc4, _ = growExpansion(sc4, math.Ldexp(m, -4))
					}
				}
				for _, c := range half {
					sc4, _ = growExpansion(sc4, math.Ldexp(c, wideHalfShift-4))
				}
				sc4 = compressExpansion(sc4)
				t4, r4 := expansionTopRem(sc4)
				res := math.Ldexp(t4, 4)
				if math.IsInf(res, 0) {
					if math.Abs(t4) == 0x1p1020 && (t4 > 0) != (r4 > 0) && math.Abs(r4) > 0x1p966 {
						return math.Copysign(math.MaxFloat64, t4), 0, true
					}
					return res, 0, false
				}
				remRes := math.Ldexp(r4, 4)
				if math.IsNaN(remRes) || math.IsInf(remRes, 0) {
					return res, 0, true
				}
				s, e := twoSum(res, remRes)
				return s, e, true
			}
		}
		for _, m := range w.mid {
			if math.Abs(m) < 0x1p-400 {
				midMerged, _ = growExpansion(midMerged, m)
			}
		}
		for _, c := range half {
			midMerged, _ = growExpansion(midMerged, math.Ldexp(c, wideHalfShift))
		}
		midMerged = compressExpansion(midMerged)
	}

	if len(w.lo) == 0 {
		s, e := expansionTopRem(midMerged)
		return s, e, true
	}

	var loSmallBuf [expansionCap]float64
	loSmall := loSmallBuf[:0]
	addedLoNorm := false
	for _, l := range w.lo {
		if math.Abs(l) >= 0x1p34 {
			midMerged, _ = growExpansion(midMerged, math.Ldexp(math.Ldexp(l, -wideHalfShift), -wideHalfShift))
			addedLoNorm = true
		} else {
			loSmall = append(loSmall, l)
		}
	}
	if addedLoNorm {
		midMerged = compressExpansion(midMerged)
	}
	if len(loSmall) == 0 {
		s, e := expansionTopRem(midMerged)
		return s, e, true
	}

	if len(midMerged) == 0 {
		loTop, loRem := expansionTopRem(loSmall)
		if loTop == 0 {
			return 0, 0, true
		}
		adj := roundToOddSticky(loTop, loRem)
		return math.Ldexp(math.Ldexp(adj, -wideHalfShift), -wideHalfShift), 0, true
	}

	if math.Abs(midMerged[0]) < 0x1p-960 {
		var upBuf [2 * expansionCap]float64
		up := upBuf[:0]
		var topKeepBuf [expansionCap]float64
		topKeep := topKeepBuf[:0]
		for _, m := range midMerged {
			if math.Abs(m) < 0x1p400 {
				up, _ = growExpansion(up, math.Ldexp(m, wideHalfShift))
			} else {
				topKeep = append(topKeep, m)
			}
		}
		for _, l := range loSmall {
			if math.Abs(l) >= 0x1p-490 {
				up, _ = growExpansion(up, math.Ldexp(l, -wideHalfShift))
			}
		}
		up = compressExpansion(up)
		if len(topKeep) == 0 {
			uTop, uRem := expansionTopRem(up)
			if uTop == 0 {
				return 0, 0, true
			}
			if math.Abs(uTop) >= 0x1p-493 {
				s0 := math.Ldexp(uTop, -wideHalfShift)
				e0 := math.Ldexp(uRem, -wideHalfShift)
				s, e := twoSum(s0, e0)
				return s, e, true
			}
			adj := roundToOddSticky(uTop, uRem)
			return math.Ldexp(adj, -wideHalfShift), 0, true
		}
		rebuilt := midBuf[:0]
		for _, u := range up {
			if math.Abs(u) >= 0x1p-494 {
				rebuilt, _ = growExpansion(rebuilt, math.Ldexp(u, -wideHalfShift))
			}
		}
		for _, m := range topKeep {
			rebuilt, _ = growExpansion(rebuilt, m)
		}
		rebuilt = compressExpansion(rebuilt)
		s, e := expansionTopRem(rebuilt)
		return s, e, true
	}

	loTop, loRem := expansionTopRem(loSmall)
	loUnscaled := math.Ldexp(math.Ldexp(roundToOddSticky(loTop, loRem), -wideHalfShift), -wideHalfShift)
	r := 0.0
	for i := range len(midMerged) - 1 {
		r += midMerged[i]
	}
	if r == 0 {
		r = loUnscaled
	} else {
		r += loUnscaled
	}
	s, e := twoSum(midMerged[len(midMerged)-1], r)
	return s, e, true
}

func divDDByFloat(numTop, numRem, a, numEB float64) (val, eb float64, ok bool) {
	if a == 0 || math.IsNaN(numTop) || math.IsNaN(numRem) || math.IsInf(numTop, 0) || math.IsInf(numRem, 0) {
		return 0, 0, false
	}
	q1 := numTop / a
	if math.IsNaN(q1) || math.IsInf(q1, 0) {
		return 0, 0, false
	}
	absA := math.Abs(a)
	p, pe := twoProd(q1, a)
	if math.IsNaN(p) || math.IsInf(p, 0) || math.IsNaN(pe) || math.IsInf(pe, 0) {
		q2 := numRem / a
		val = q1 + q2
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, 0, false
		}
		eb = numEB/absA + 0x1p-50*(math.Abs(q1)+math.Abs(q2))
		return val, eb, true
	}
	rem := (numTop - p) - pe + numRem
	q2 := rem / a
	val = q1 + q2
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, 0, false
	}
	eb = numEB/absA + 0x1p-51*(math.Abs(q1)+math.Abs(q2))
	return val, eb, true
}

func evalLinearSum(constant float64, coeffs, x []float64, n int) float64 {
	k := min(n, min(len(coeffs), len(x)))
	var acc dot2Acc
	acc.addVal(constant)
	for j := range k {
		c := coeffs[j]
		xj := x[j]
		if c == 0 && math.Abs(xj) <= math.MaxFloat64 {
			continue
		}
		if xj == 0 && math.Abs(c) <= math.MaxFloat64 {
			continue
		}
		acc.addProd(c, xj)
	}
	if val, eb, ok := acc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		return val
	}
	if math.IsNaN(constant) {
		return math.NaN()
	}
	hasPosInf := math.IsInf(constant, 1)
	hasNegInf := math.IsInf(constant, -1)
	for j := range k {
		c := coeffs[j]
		xj := x[j]
		if math.IsNaN(c) || math.IsNaN(xj) {
			return math.NaN()
		}
		if c == 0 || xj == 0 {
			if (c == 0 && math.IsInf(xj, 0)) || (xj == 0 && math.IsInf(c, 0)) {
				return math.NaN()
			}
			continue
		}
		if math.IsInf(c, 0) || math.IsInf(xj, 0) {
			if (c > 0) == (xj > 0) {
				hasPosInf = true
			} else {
				hasNegInf = true
			}
		}
	}
	if hasPosInf && hasNegInf {
		return math.NaN()
	}
	if hasPosInf {
		return math.Inf(1)
	}
	if hasNegInf {
		return math.Inf(-1)
	}
	var w wideDotAcc
	w.addVal(constant)
	for j := range k {
		w.addProd(coeffs[j], x[j])
	}
	return w.finish(math.NaN())
}

func evalSparseRowSum(cols []int, vals, x []float64) float64 {
	kLim := min(len(cols), len(vals))
	var acc dot2Acc
	for k := range kLim {
		c := vals[k]
		xj := x[cols[k]]
		if c == 0 && math.Abs(xj) <= math.MaxFloat64 {
			continue
		}
		if xj == 0 && math.Abs(c) <= math.MaxFloat64 {
			continue
		}
		acc.addProd(c, xj)
	}
	if val, eb, ok := acc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		return val
	}
	hasPosInf := false
	hasNegInf := false
	for k := range kLim {
		c := vals[k]
		xj := x[cols[k]]
		if math.IsNaN(c) || math.IsNaN(xj) {
			return math.NaN()
		}
		if c == 0 || xj == 0 {
			if (c == 0 && math.IsInf(xj, 0)) || (xj == 0 && math.IsInf(c, 0)) {
				return math.NaN()
			}
			continue
		}
		if math.IsInf(c, 0) || math.IsInf(xj, 0) {
			if (c > 0) == (xj > 0) {
				hasPosInf = true
			} else {
				hasNegInf = true
			}
		}
	}
	if hasPosInf && hasNegInf {
		return math.NaN()
	}
	if hasPosInf {
		return math.Inf(1)
	}
	if hasNegInf {
		return math.Inf(-1)
	}
	var w wideDotAcc
	for k := range kLim {
		w.addProd(vals[k], x[cols[k]])
	}
	return w.finish(math.NaN())
}

func evalSparseRowResidual(cols []int, vals, x []float64, rhs float64) float64 {
	if math.IsNaN(rhs) {
		return math.NaN()
	}
	kLim := min(len(cols), len(vals))
	var acc dot2Acc
	acc.addVal(-rhs)
	for k := range kLim {
		c := vals[k]
		xj := x[cols[k]]
		if c == 0 && math.Abs(xj) <= math.MaxFloat64 {
			continue
		}
		if xj == 0 && math.Abs(c) <= math.MaxFloat64 {
			continue
		}
		acc.addProd(c, xj)
	}
	if val, eb, ok := acc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		return val
	}
	hasPosInf := math.IsInf(rhs, -1)
	hasNegInf := math.IsInf(rhs, 1)
	for k := range kLim {
		c := vals[k]
		xj := x[cols[k]]
		if math.IsNaN(c) || math.IsNaN(xj) {
			return math.NaN()
		}
		if c == 0 || xj == 0 {
			if (c == 0 && math.IsInf(xj, 0)) || (xj == 0 && math.IsInf(c, 0)) {
				return math.NaN()
			}
			continue
		}
		if math.IsInf(c, 0) || math.IsInf(xj, 0) {
			if (c > 0) == (xj > 0) {
				hasPosInf = true
			} else {
				hasNegInf = true
			}
		}
	}
	if hasPosInf && hasNegInf {
		return math.NaN()
	}
	if hasPosInf {
		return math.Inf(1)
	}
	if hasNegInf {
		return math.Inf(-1)
	}
	var w wideDotAcc
	w.addVal(-rhs)
	for k := range kLim {
		w.addProd(vals[k], x[cols[k]])
	}
	return w.finish(math.NaN())
}

func evalSparseRowResidualAndTol(cols []int, vals, x, lb, ub []float64, rhs, minTol, relFactor float64) (res, rTol float64, ok bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return 0, 0, false
	}
	kLim := min(len(cols), len(vals))
	hasBounds := len(lb) > 0 && len(ub) > 0
	var acc dot2Acc
	acc.addVal(-rhs)
	freeL1 := 0.0
	for k := range kLim {
		col := cols[k]
		c := vals[k]
		xj := x[col]
		if c == 0 && math.Abs(xj) <= math.MaxFloat64 {
			continue
		}
		if xj == 0 && math.Abs(c) <= math.MaxFloat64 {
			continue
		}
		acc.addProd(c, xj)
		if !hasBounds || col >= len(lb) || col >= len(ub) || lb[col] < ub[col] {
			freeL1 += math.Abs(c * xj)
		}
	}
	if val, eb, accOK := acc.result(); accOK && freeL1 <= math.MaxFloat64 {
		if eb > 0x1p-20*max(1.0, math.Abs(val)) {
			val = evalSparseRowResidual(cols, vals, x, rhs)
			if math.IsNaN(val) || math.IsInf(val, 0) {
				return 0, 0, false
			}
		}
		if minTol > 0 {
			rTol = max(minTol, relFactor*max(1.0, freeL1))
		} else {
			rTol = relFactor * freeL1
		}
		return val, rTol, true
	}
	res = evalSparseRowResidual(cols, vals, x, rhs)
	if math.IsNaN(res) || math.IsInf(res, 0) {
		return 0, 0, false
	}
	maxExp := 0
	for k := range kLim {
		col := cols[k]
		if hasBounds && col < len(lb) && col < len(ub) && lb[col] >= ub[col] {
			continue
		}
		ac := math.Abs(vals[k])
		ax := math.Abs(x[col])
		if ac == 0 || ax == 0 {
			continue
		}
		_, ea := math.Frexp(ac)
		_, ex := math.Frexp(ax)
		if ea+ex > maxExp {
			maxExp = ea + ex
		}
	}
	shift := max(32, maxExp-wideExpCutoff)
	s1 := shift / 2
	s2 := shift - s1
	l1Scaled := 0.0
	for k := range kLim {
		col := cols[k]
		if hasBounds && col < len(lb) && col < len(ub) && lb[col] >= ub[col] {
			continue
		}
		ac := math.Abs(vals[k])
		ax := math.Abs(x[col])
		if ac == 0 || ax == 0 {
			continue
		}
		pScaled := math.Ldexp(ac, -s1) * math.Ldexp(ax, -s2)
		if math.IsNaN(pScaled) || math.IsInf(pScaled, 0) {
			return 0, 0, false
		}
		l1Scaled += pScaled
	}
	if math.IsNaN(l1Scaled) || math.IsInf(l1Scaled, 0) {
		return 0, 0, false
	}
	relVal := math.Ldexp(relFactor*l1Scaled, shift)
	if math.IsInf(relVal, 1) {
		relVal = math.MaxFloat64
	}
	if minTol > 0 {
		rTol = max(minTol, max(relFactor, relVal))
	} else {
		rTol = relVal
	}
	return res, rTol, true
}

func evalSparseRowResidualAndDirTol(cols []int, vals, x, lb, ub []float64, rhs, minTol, relFactor float64) (res, rTolLower, rTolUpper float64, ok bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return 0, 0, 0, false
	}
	res = evalSparseRowResidual(cols, vals, x, rhs)
	if math.IsNaN(res) || math.IsInf(res, 0) {
		return 0, 0, 0, false
	}
	kLim := min(len(cols), len(vals))
	hasBounds := len(lb) > 0 && len(ub) > 0
	for k := range kLim {
		col := cols[k]
		c := vals[k]
		if c == 0 || (hasBounds && col < len(lb) && col < len(ub) && lb[col] >= ub[col]) {
			continue
		}
		xj := x[col]
		if math.IsNaN(xj) || math.IsInf(xj, 0) {
			return 0, 0, 0, false
		}
		step := relFactor * max(1.0, math.Abs(xj))
		availInc := step
		availDec := step
		if hasBounds && col < len(lb) && col < len(ub) {
			if !math.IsInf(ub[col], 1) {
				availInc = min(step, max(0.0, ub[col]-xj))
			}
			if !math.IsInf(lb[col], -1) {
				availDec = min(step, max(0.0, xj-lb[col]))
			}
		}
		ac := math.Abs(c)
		var incRow, decRow float64
		if c > 0 {
			incRow = ac * availInc
			decRow = ac * availDec
		} else {
			incRow = ac * availDec
			decRow = ac * availInc
		}
		if math.IsNaN(incRow) || math.IsNaN(decRow) {
			return 0, 0, 0, false
		}
		rTolLower = satAddNonNeg(rTolLower, incRow)
		rTolUpper = satAddNonNeg(rTolUpper, decRow)
	}
	if minTol > 0 {
		rTolLower = max(minTol, rTolLower)
		rTolUpper = max(minTol, rTolUpper)
	}
	return res, rTolLower, rTolUpper, true
}

// EvaluateObjective computes the original objective value for primal vector x.
func (p *CompiledProblem) EvaluateObjective(x []float64) float64 {
	if p == nil {
		return 0
	}
	return evalLinearSum(p.objConstant, p.objCoeffs, x, p.numVars)
}

// MaxViolation computes the largest bound, row, and integer residual of x.
func (p *CompiledProblem) MaxViolation(x []float64, checkIntegrality bool) (boundViol, rowViol, intViol float64) {
	if p == nil || len(x) < p.numVars {
		return math.Inf(1), math.Inf(1), math.Inf(1)
	}
	for j := range p.numVars {
		v := x[j]
		if math.IsNaN(v) {
			boundViol = math.Inf(1)
			if checkIntegrality && p.varCat[j] != Continuous {
				intViol = math.Inf(1)
			}
			continue
		}
		if v < p.varLB[j] {
			boundViol = max(boundViol, p.varLB[j]-v)
		}
		if v > p.varUB[j] {
			boundViol = max(boundViol, v-p.varUB[j])
		}
		if checkIntegrality && p.varCat[j] != Continuous {
			diff := math.Abs(v - math.Round(v))
			if math.IsNaN(diff) {
				intViol = math.Inf(1)
			} else {
				intViol = max(intViol, diff)
			}
		}
	}
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		cols := p.colIdx[s:e]
		vals := p.rowVal[s:e]
		lb, ub := p.rowLB[i], p.rowUB[i]
		hasLB := !math.IsInf(lb, -1)
		hasUB := !math.IsInf(ub, 1)
		switch {
		case hasLB && hasUB && lb == ub:
			res := evalSparseRowResidual(cols, vals, x, lb)
			if math.IsNaN(res) {
				rowViol = math.Inf(1)
			} else {
				rowViol = max(rowViol, math.Abs(res))
			}
		case !hasLB && !hasUB:
			if math.IsNaN(evalSparseRowSum(cols, vals, x)) {
				rowViol = math.Inf(1)
			}
		default:
			if hasLB {
				res := evalSparseRowResidual(cols, vals, x, lb)
				if math.IsNaN(res) {
					rowViol = math.Inf(1)
				} else if res < 0 {
					rowViol = max(rowViol, -res)
				}
			}
			if hasUB {
				res := evalSparseRowResidual(cols, vals, x, ub)
				if math.IsNaN(res) {
					rowViol = math.Inf(1)
				} else if res > 0 {
					rowViol = max(rowViol, res)
				}
			}
		}
	}
	return boundViol, rowViol, intViol
}

// Name returns the compiled problem name ("NoName" when unnamed).
func (p *CompiledProblem) Name() string {
	if p == nil {
		return ""
	}
	if p.name == "" {
		return "NoName"
	}
	return p.name
}

// Sense returns the compiled optimization direction (Minimize when unset).
func (p *CompiledProblem) Sense() Sense {
	if p == nil || p.sense == 0 {
		return Minimize
	}
	return p.sense
}

// Err returns the validation error recorded when the model was compiled, if any.
func (p *CompiledProblem) Err() error {
	if p == nil {
		return ErrNilProblem
	}
	return p.invalidErr
}

// ObjName returns the objective name, or an empty string when unnamed.
func (p *CompiledProblem) ObjName() string {
	if p == nil {
		return ""
	}
	return p.objName
}

// ObjConstant returns the additive constant term in the objective.
func (p *CompiledProblem) ObjConstant() float64 {
	if p == nil {
		return 0
	}
	return p.objConstant
}

// NumVars returns the number of structural decision variables.
func (p *CompiledProblem) NumVars() int {
	if p == nil {
		return 0
	}
	return p.numVars
}

// NumConstraints returns the number of linear constraints.
func (p *CompiledProblem) NumConstraints() int {
	if p == nil {
		return 0
	}
	return p.numConstraints
}

// NNZ returns the number of structural nonzero coefficients in the constraint matrix.
func (p *CompiledProblem) NNZ() int {
	if p == nil {
		return 0
	}
	return len(p.colIdx)
}

// IsMIP reports whether the problem has at least one integer or binary variable.
func (p *CompiledProblem) IsMIP() bool {
	return p != nil && p.isMIP
}

// NumIntVars returns the number of integer and binary variables.
func (p *CompiledProblem) NumIntVars() int {
	if p == nil {
		return 0
	}
	return len(p.intCols)
}

// IntCol returns the variable index of the k-th integer or binary variable.
func (p *CompiledProblem) IntCol(k int) int {
	if p == nil || uint(k) >= uint(len(p.intCols)) {
		return -1
	}
	return p.intCols[k]
}

// IsBinaryVar reports whether variable j is marked in the compiled binary bitmask.
func (p *CompiledProblem) IsBinaryVar(j int) bool {
	if p == nil || uint(j) >= uint(p.numVars) {
		return false
	}
	return (p.binaryMask[j/bitsPerWord] & (1 << (uint(j) % bitsPerWord))) != 0
}

// VarName returns the name of variable j.
func (p *CompiledProblem) VarName(j int) string {
	if p == nil || uint(j) >= uint(len(p.varNames)) {
		return ""
	}
	return p.varNames[j]
}

// VarBounds returns the lower and upper bounds of variable j.
func (p *CompiledProblem) VarBounds(j int) (lb, ub float64) {
	if p == nil || uint(j) >= uint(p.numVars) {
		return 0, 0
	}
	return p.varLB[j], p.varUB[j]
}

// VarCategory returns the domain category of variable j.
func (p *CompiledProblem) VarCategory(j int) VarCategory {
	if p == nil || uint(j) >= uint(len(p.varCat)) {
		return Continuous
	}
	return p.varCat[j]
}

// ObjCoeff returns the user-sense objective coefficient of variable j.
func (p *CompiledProblem) ObjCoeff(j int) float64 {
	if p == nil || uint(j) >= uint(len(p.objCoeffs)) {
		return 0
	}
	return p.objCoeffs[j]
}

// ObjMinCoeff returns the minimization-normalized objective coefficient of variable j.
func (p *CompiledProblem) ObjMinCoeff(j int) float64 {
	if p == nil || uint(j) >= uint(len(p.objMin)) {
		return 0
	}
	return p.objMin[j]
}

// RowName returns the name of constraint row i.
func (p *CompiledProblem) RowName(i int) string {
	if p == nil || uint(i) >= uint(len(p.rowNames)) {
		return ""
	}
	return p.rowNames[i]
}

// RowSense returns the relational sense of constraint row i.
func (p *CompiledProblem) RowSense(i int) ConstraintSense {
	if p == nil || uint(i) >= uint(len(p.rowSense)) {
		return Equal
	}
	return p.rowSense[i]
}

// RowRHS returns the right-hand side of constraint row i.
func (p *CompiledProblem) RowRHS(i int) float64 {
	if p == nil || uint(i) >= uint(len(p.rowRHS)) {
		return 0
	}
	return p.rowRHS[i]
}

// RowBounds returns the equivalent ranged bounds [lb, ub] of constraint row i.
func (p *CompiledProblem) RowBounds(i int) (lb, ub float64) {
	if p == nil || uint(i) >= uint(p.numConstraints) {
		return 0, 0
	}
	return p.rowLB[i], p.rowUB[i]
}

// RowLen returns the number of nonzero terms in constraint row i.
func (p *CompiledProblem) RowLen(i int) int {
	if p == nil || uint(i) >= uint(p.numConstraints) {
		return 0
	}
	return p.rowPtr[i+1] - p.rowPtr[i]
}

// ForEachRowTerm invokes fn(col, val) for each nonzero entry in constraint row i in ascending column order.
func (p *CompiledProblem) ForEachRowTerm(i int, fn func(col int, val float64)) {
	if p == nil || fn == nil || uint(i) >= uint(p.numConstraints) {
		return
	}
	for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
		fn(p.colIdx[k], p.rowVal[k])
	}
}

// ForEachColTerm invokes fn(row, val) for each nonzero entry in column j in ascending row order.
func (p *CompiledProblem) ForEachColTerm(j int, fn func(row int, val float64)) {
	if p == nil || fn == nil || uint(j) >= uint(p.numVars) {
		return
	}
	for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
		fn(p.rowIdx[k], p.colVal[k])
	}
}

// RowStride returns the SIMD row stride in float64 elements when dense rows are prebuilt, or 0 otherwise.
func (p *CompiledProblem) RowStride() int {
	if p == nil {
		return 0
	}
	return p.rowStride
}

// DenseRowLen returns the total length of the prebuilt dense row buffer, or 0 when sparse-only.
func (p *CompiledProblem) DenseRowLen() int {
	if p == nil {
		return 0
	}
	return len(p.denseRows)
}
