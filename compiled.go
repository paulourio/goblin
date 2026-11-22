package goblin

import (
	"fmt"
	"math"
	"slices"
	"sync"
)

// DenseThreshold is the maximum number of variables for which CompiledProblem
// also prebuilds a contiguous SIMD-aligned dense row matrix.
const DenseThreshold = 256

const (
	bitsPerWord         = 64
	wordBitMask         = bitsPerWord - 1
	simdRowAlign        = 8
	simdRowAlignMask    = simdRowAlign - 1
	defaultTouchedCap   = 64
	maxDenseRowFloats   = 1 << 20
	minDenseRowCheckMC  = 32
	denseSparsityMult   = 5
	sparseNoDenseStride = -1
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

// cbcMatrixData holds lazily constructed 32-bit CSC indices, clamped bounds,
// and cached matrix validation for zero-copy C++ OsiClpSolverInterface::loadProblem calls.
type cbcMatrixData struct {
	colPtr32  []int32
	rowIdx32  []int32
	colLBClp  []float64
	colUBClp  []float64
	rowLBClp  []float64
	rowUBClp  []float64
	intCols32 []int32
	valid     bool
}

// compiledLazyCache holds concurrency-safe lazy state shared by a CompiledProblem.
// Storing it behind a pointer keeps CompiledProblem free of embedded locks so
// shallow copies (such as zero-objective feasibility probes) never copy sync.Once.
// scaledLPData stores immutable Ruiz equilibration and column-tolerance metadata
// computed once per CompiledProblem.
type scaledLPData struct {
	mat         *CompiledProblem
	rowScale    []float64
	colScale    []float64
	invColScale []float64
	colTol      []float64
	slackTol    []float64
	colExp      []int
	colHasEq    []bool
	objScale    float64
	scaled      bool
}

type compiledLazyCache struct {
	cbc        *cbcMatrixData
	scaled     *scaledLPData
	rowMaxAbs  []float64 // Precomputed max_k |A[i,k]| per row for O(1) incumbent tolerance scaling
	cbcOnce    sync.Once
	scaledOnce sync.Once
}

type compiledProblemAlloc struct {
	lazy compiledLazyCache
	p    CompiledProblem
}

func newCompiledProblem() *CompiledProblem {
	alloc := new(compiledProblemAlloc)
	alloc.p.lazy = &alloc.lazy
	return &alloc.p
}

// CompiledProblem is an immutable, flat matrix layout of an LP or MILP model
// built for cache locality, SIMD row sweeps, pooled solver workspaces,
// and zero-copy C++ COIN-OR CBC / OsiClpSolverInterface calls.
type CompiledProblem struct {
	lazy       *compiledLazyCache
	invalidErr error
	name       string
	objName    string

	// Variable metadata (length = NumVars)
	varNames []string
	varLB    []float64
	varUB    []float64
	varCat   []VarCategory

	// Binary bitmask (each uint64 covers 64 variables) and integer column indices.
	binaryMask []uint64
	intCols    []int

	// Normalized objective: the internal solver always minimizes ObjMin[j]*x[j].
	// Original objective value = Sense * (sum_j ObjMin[j]*x[j]) + ObjConstant.
	objCoeffs []float64 // Original objective coefficients as specified
	objMin    []float64 // Objective coefficients normalized for minimizing

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

	// Contiguous SIMD-padded dense rows when NumVars <= DenseThreshold and density thresholds hold.
	denseRows []float64

	// Scalar metadata packed at the tail to minimize GC pointer scan span.
	modelID        uint64
	objConstant    float64
	objConstMinVal float64
	numVars        int
	numConstraints int
	rowStride      int
	sense          Sense
	isMIP          bool
}

func denseRowStride(n, mc, nnz int) int {
	if n <= 0 || n > DenseThreshold || mc <= 0 {
		return 0
	}
	stride := (n + simdRowAlignMask) &^ simdRowAlignMask
	if mc > maxDenseRowFloats/stride {
		return 0
	}
	if mc > minDenseRowCheckMC && nnz*denseSparsityMult < mc*n {
		return 0
	}
	return stride
}

func buildCSCFromCSR(p *CompiledProblem) {
	n := p.numVars
	mc := p.numConstraints
	nnz := len(p.colIdx)
	p.colPtr = make([]int, n+1)
	p.rowIdx = make([]int, nnz)
	p.colVal = make([]float64, nnz)
	var rowMaxAbs []float64
	if p.lazy != nil {
		rowMaxAbs = make([]float64, mc)
		p.lazy.rowMaxAbs = rowMaxAbs
	}
	if nnz == 0 {
		return
	}
	for _, col := range p.colIdx {
		p.colPtr[col+1]++
	}
	for j := range n {
		p.colPtr[j+1] += p.colPtr[j]
	}
	for i := range mc {
		maxA := 0.0
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			pos := p.colPtr[col]
			val := p.rowVal[k]
			p.rowIdx[pos] = i
			p.colVal[pos] = val
			p.colPtr[col] = pos + 1
			if av := math.Abs(val); av > maxA {
				maxA = av
			}
		}
		if rowMaxAbs != nil {
			rowMaxAbs[i] = maxA
		}
	}
	copy(p.colPtr[1:], p.colPtr[:n])
	p.colPtr[0] = 0
}

func (p *CompiledProblem) ensureCBCData() *cbcMatrixData {
	if p == nil {
		return nil
	}
	if p.lazy == nil {
		return buildCBCMatrixData(p)
	}
	p.lazy.cbcOnce.Do(func() {
		p.lazy.cbc = buildCBCMatrixData(p)
	})
	return p.lazy.cbc
}

func buildCBCMatrixData(p *CompiledProblem) *cbcMatrixData {
	n := p.numVars
	m := p.numConstraints
	if n < 0 || m < 0 || n > math.MaxInt32 || m > math.MaxInt32 {
		return &cbcMatrixData{}
	}
	if len(p.varLB) != n || len(p.varUB) != n || len(p.varCat) != n {
		return &cbcMatrixData{}
	}
	if len(p.rowLB) != m || len(p.rowUB) != m {
		return &cbcMatrixData{}
	}
	if len(p.colPtr) != n+1 || p.colPtr[0] != 0 {
		return &cbcMatrixData{}
	}
	for j := range n {
		if p.colPtr[j] > p.colPtr[j+1] {
			return &cbcMatrixData{}
		}
	}
	nnz := p.colPtr[n]
	if nnz < 0 || nnz > math.MaxInt32 || len(p.rowIdx) != nnz || len(p.colVal) != nnz {
		return &cbcMatrixData{}
	}
	if len(p.intCols) > n {
		return &cbcMatrixData{}
	}

	colPtr32 := make([]int32, n+1)
	for j := range n + 1 {
		colPtr32[j] = int32(p.colPtr[j])
	}
	rowIdx32 := make([]int32, nnz)
	for k := range nnz {
		r := p.rowIdx[k]
		if r < 0 || r >= m {
			return &cbcMatrixData{}
		}
		rowIdx32[k] = int32(r)
	}
	colLBClp := make([]float64, n)
	colUBClp := make([]float64, n)
	for j := range n {
		colLBClp[j] = clampCBCBound(p.varLB[j])
		colUBClp[j] = clampCBCBound(p.varUB[j])
	}
	rowLBClp := make([]float64, m)
	rowUBClp := make([]float64, m)
	for i := range m {
		rowLBClp[i] = clampCBCBound(p.rowLB[i])
		rowUBClp[i] = clampCBCBound(p.rowUB[i])
	}
	var intCols32 []int32
	if len(p.intCols) > 0 {
		intCols32 = make([]int32, len(p.intCols))
		for k, col := range p.intCols {
			if col < 0 || col >= n {
				return &cbcMatrixData{}
			}
			intCols32[k] = int32(col)
		}
	}
	return &cbcMatrixData{
		colPtr32:  colPtr32,
		rowIdx32:  rowIdx32,
		colLBClp:  colLBClp,
		colUBClp:  colUBClp,
		rowLBClp:  rowLBClp,
		rowUBClp:  rowUBClp,
		intCols32: intCols32,
		valid:     true,
	}
}

// CompileChecked validates the Model and returns a CompiledProblem or the first check error.
func (m *Model) CompileChecked() (*CompiledProblem, error) {
	if m == nil {
		return nil, ErrNilModel
	}
	return m.compile(true)
}

// Compile transforms a symbolic Model into a flat, canonical CompiledProblem.
// Duplicate variable terms within the objective or within a single constraint
// are summed and zero coefficients are dropped.
func (m *Model) Compile() *CompiledProblem {
	if m == nil {
		return nil
	}
	p, _ := m.compile(false)
	return p
}

func (m *Model) compile(abortOnErr bool) (*CompiledProblem, error) {
	firstErr := m.err
	if abortOnErr && firstErr != nil {
		return nil, firstErr
	}

	n := len(m.vars)
	mc := len(m.constraints)
	numInt := 0
	for j := range n {
		v := &m.vars[j]
		if firstErr == nil {
			if !v.cat.valid() {
				firstErr = fmt.Errorf("variable %q has invalid category %d", v.name, uint8(v.cat))
			} else if math.IsNaN(v.lb) || math.IsNaN(v.ub) || math.IsInf(v.lb, 1) || math.IsInf(v.ub, -1) {
				firstErr = fmt.Errorf("variable %q has invalid bounds [%v, %v]", v.name, v.lb, v.ub)
			}
		}
		if v.cat != Continuous {
			numInt++
		}
	}
	if firstErr == nil && (math.IsNaN(m.objConst) || math.IsInf(m.objConst, 0)) {
		firstErr = fmt.Errorf("objective constant is non-finite (%v)", m.objConst)
	}
	if abortOnErr && firstErr != nil {
		return nil, firstErr
	}

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
	p := newCompiledProblem()
	p.name = m.Name()
	p.sense = sense
	p.modelID = modelID
	p.numVars = n
	p.numConstraints = mc
	p.varNames = make([]string, n)
	p.varLB = make([]float64, n)
	p.varUB = make([]float64, n)
	p.varCat = make([]VarCategory, n)
	p.binaryMask = make([]uint64, numWords)
	if numInt > 0 {
		p.isMIP = true
		p.intCols = make([]int, 0, numInt)
	}
	p.objName = m.objName
	p.objCoeffs = make([]float64, n)
	p.objConstant = m.objConst
	p.objConstMinVal = constMinVal
	p.rowNames = make([]string, mc)
	p.rowSense = make([]ConstraintSense, mc)
	p.rowRHS = make([]float64, mc)
	p.rowLB = make([]float64, mc)
	p.rowUB = make([]float64, mc)
	p.rowPtr = make([]int, mc+1)

	for j := range n {
		v := &m.vars[j]
		cat := v.cat
		if cat == Integer && v.lb >= 0 && v.ub <= 1 && v.lb <= v.ub {
			cat = Binary
		}
		p.varNames[j] = v.name
		p.varLB[j] = v.lb
		p.varUB[j] = v.ub
		p.varCat[j] = cat
		if cat != Continuous {
			p.intCols = append(p.intCols, j)
			if cat == Binary {
				p.binaryMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			}
		}
	}

	// Accumulate and validate objective terms in a single pass, routing duplicate columns through wideDotAcc.
	inTouched := make([]uint8, n)
	touched := make([]int, 0, min(n, defaultTouchedCap))
	objHasDup := false
	for _, t := range m.objTerms {
		col, ok := m.validVar(t.Var)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("objective references invalid or foreign Var handle %v", t.Var)
				if abortOnErr {
					return nil, firstErr
				}
			}
			continue
		}
		if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
			if firstErr == nil {
				firstErr = fmt.Errorf("objective has non-finite coefficient %v", t.Coeff)
				if abortOnErr {
					return nil, firstErr
				}
			}
			continue
		}
		if t.Coeff == 0 {
			continue
		}
		if inTouched[col] == 0 {
			inTouched[col] = 1
			touched = append(touched, col)
			p.objCoeffs[col] = t.Coeff
		} else {
			inTouched[col] = 2
			objHasDup = true
		}
	}
	if objHasDup {
		wideMap := make(map[int]*wideDotAcc)
		for _, col := range touched {
			if inTouched[col] > 1 {
				wideMap[col] = &wideDotAcc{}
			}
		}
		for _, t := range m.objTerms {
			if col, ok := m.validVar(t.Var); ok && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) && t.Coeff != 0 {
				if w, exists := wideMap[col]; exists {
					w.addVal(t.Coeff)
				}
			}
		}
		for _, col := range touched {
			if w, exists := wideMap[col]; exists {
				val := w.finish(math.NaN())
				if math.IsNaN(val) || math.IsInf(val, 0) {
					if firstErr == nil {
						firstErr = fmt.Errorf("objective has non-finite accumulated coefficient for variable %q", m.vars[col].name)
						if abortOnErr {
							return nil, firstErr
						}
					}
				}
				if val == 0 {
					val = 0
				}
				p.objCoeffs[col] = val
			}
		}
	}
	for _, col := range touched {
		inTouched[col] = 0
	}

	if sense == Maximize {
		p.objMin = make([]float64, n)
		for j := range n {
			if c := p.objCoeffs[j]; c != 0 {
				p.objMin[j] = -c
			}
		}
	} else {
		p.objMin = p.objCoeffs
	}

	totalNNZ := len(m.conTerms)

	// Allocate SIMD-aligned dense rows only when dimensions and matrix density justify it.
	stride := denseRowStride(n, mc, totalNNZ)
	useDense := stride > 0
	if useDense {
		p.rowStride = stride
		p.denseRows = make([]float64, mc*stride)
	} else if n > 0 && mc > 0 {
		// Negative sentinel prevents sparse-only matrices from matching len(denseRows)==baseM*rowStride.
		p.rowStride = sparseNoDenseStride
	}

	// Scratch buffers for deduplicating terms in each constraint row.
	rowAcc := make([]float64, n)
	p.colIdx = make([]int, 0, totalNNZ)
	p.rowVal = make([]float64, 0, totalNNZ)

	for i := range mc {
		c := &m.constraints[i]
		if firstErr == nil {
			if !c.sense.valid() {
				firstErr = fmt.Errorf("constraint %q has invalid sense %d", c.name, int8(c.sense))
				if abortOnErr {
					return nil, firstErr
				}
			} else if math.IsNaN(c.rhs) || math.IsInf(c.rhs, 0) {
				firstErr = fmt.Errorf("constraint %q has non-finite RHS %v", c.name, c.rhs)
				if abortOnErr {
					return nil, firstErr
				}
			}
		}
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

		touched = touched[:0]
		rowHasDup := false
		sorted := true
		prevCol := -1
		cTerms := m.conTerms[c.termStart:c.termEnd]
		for _, t := range cTerms {
			col, ok := m.validVar(t.Var)
			if !ok {
				if firstErr == nil {
					firstErr = fmt.Errorf("constraint %q references invalid or foreign Var handle %v", c.name, t.Var)
					if abortOnErr {
						return nil, firstErr
					}
				}
				continue
			}
			if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
				if firstErr == nil {
					firstErr = fmt.Errorf("constraint %q has non-finite coefficient %v", c.name, t.Coeff)
					if abortOnErr {
						return nil, firstErr
					}
				}
				continue
			}
			if t.Coeff == 0 {
				continue
			}
			if inTouched[col] == 0 {
				inTouched[col] = 1
				if col < prevCol {
					sorted = false
				}
				prevCol = col
				touched = append(touched, col)
				rowAcc[col] = t.Coeff
			} else {
				inTouched[col] = 2
				rowHasDup = true
			}
		}
		if rowHasDup {
			wideMap := make(map[int]*wideDotAcc)
			for _, col := range touched {
				if inTouched[col] > 1 {
					wideMap[col] = &wideDotAcc{}
				}
			}
			for _, t := range cTerms {
				if col, ok := m.validVar(t.Var); ok && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) && t.Coeff != 0 {
					if w, exists := wideMap[col]; exists {
						w.addVal(t.Coeff)
					}
				}
			}
			for _, col := range touched {
				if w, exists := wideMap[col]; exists {
					val := w.finish(math.NaN())
					if math.IsNaN(val) || math.IsInf(val, 0) {
						if firstErr == nil {
							firstErr = fmt.Errorf("constraint %q has non-finite accumulated coefficient for variable %q", c.name, m.vars[col].name)
							if abortOnErr {
								return nil, firstErr
							}
						}
					}
					rowAcc[col] = val
				}
			}
		}
		if !sorted {
			slices.Sort(touched)
		}

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
	p.invalidErr = firstErr

	buildCSCFromCSR(p)

	return p, firstErr
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

func growExpansion(e []float64, b float64) []float64 {
	if b == 0 || math.IsNaN(b) || math.IsInf(b, 0) {
		return e
	}
	q := b
	w := 0
	for _, en := range e {
		var h float64
		q, h = twoSum(q, en)
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return e[:w]
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
	return e
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

func collapseExpansionBuf(e *[expansionCap]float64, n int) int {
	if n <= 1 {
		return n
	}
	return len(compressExpansion(e[:n]))
}

func growExpansionBuf(e *[expansionCap]float64, n int, b float64) (int, bool) {
	if b == 0 {
		return n, true
	}
	if math.IsNaN(b) || math.IsInf(b, 0) {
		return n, false
	}
	if n >= expansionCap {
		n = collapseExpansionBuf(e, n)
		if n >= expansionCap {
			top, rem := expansionTopRem(e[:n])
			n = 0
			if rem != 0 {
				e[0] = rem
				n = 1
			}
			if top != 0 {
				e[n] = top
				n++
			}
		}
	}
	q := b
	w := 0
	for i := range n {
		var h float64
		q, h = twoSum(q, e[i])
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return w, false
		}
		if h != 0 {
			e[w] = h
			w++
		}
	}
	if q != 0 {
		e[w] = q
		w++
	}
	return w, true
}

// wideDotAcc accumulates values and products across three power-of-two Shewchuk expansions
// (2^1056, 2^0, and 2^-1056) using pointer-free fixed buffers so stack instances never escape.
type wideDotAcc struct {
	hiBuf  [expansionCap]float64
	midBuf [expansionCap]float64
	loBuf  [expansionCap]float64
	hiLen  int
	midLen int
	loLen  int
	nonFin bool
}

func (w *wideDotAcc) isEmpty() bool {
	return w.hiLen == 0 && w.midLen == 0 && w.loLen == 0
}

func (w *wideDotAcc) addVal(v float64) {
	if v == 0 || w.nonFin {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.nonFin = true
		return
	}
	_, e := math.Frexp(math.Abs(v))
	var ok bool
	switch {
	case e > wideExpCutoff:
		w.hiLen, ok = growExpansionBuf(&w.hiBuf, w.hiLen, math.Ldexp(v, -wideHiShift))
	case e < -wideExpCutoff:
		w.loLen, ok = growExpansionBuf(&w.loBuf, w.loLen, math.Ldexp(v, wideHiShift))
	default:
		w.midLen, ok = growExpansionBuf(&w.midBuf, w.midLen, v)
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
	_, ea := math.Frexp(math.Abs(a))
	_, eb := math.Frexp(math.Abs(b))
	sumExp := ea + eb
	var ok1, ok2 bool
	switch {
	case sumExp > wideExpCutoff:
		p, pe := twoProd(math.Ldexp(a, -wideHalfShift), math.Ldexp(b, -wideHalfShift))
		w.hiLen, ok1 = growExpansionBuf(&w.hiBuf, w.hiLen, pe)
		w.hiLen, ok2 = growExpansionBuf(&w.hiBuf, w.hiLen, p)
	case sumExp < -wideExpCutoff:
		p, pe := twoProd(math.Ldexp(a, wideHalfShift), math.Ldexp(b, wideHalfShift))
		w.loLen, ok1 = growExpansionBuf(&w.loBuf, w.loLen, pe)
		w.loLen, ok2 = growExpansionBuf(&w.loBuf, w.loLen, p)
	default:
		p, pe := twoProd(a, b)
		w.midLen, ok1 = growExpansionBuf(&w.midBuf, w.midLen, pe)
		w.midLen, ok2 = growExpansionBuf(&w.midBuf, w.midLen, p)
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
	if w.isEmpty() {
		return 0, 0, true
	}
	if w.hiLen == 0 && w.loLen == 0 {
		w.midLen = collapseExpansionBuf(&w.midBuf, w.midLen)
		s, e := expansionTopRem(w.midBuf[:w.midLen])
		return s, e, true
	}
	return w.finishDDMultiBucket()
}

func (w *wideDotAcc) finishDDMultiBucket() (top, rem float64, ok bool) {
	w.hiLen = collapseExpansionBuf(&w.hiBuf, w.hiLen)
	w.midLen = collapseExpansionBuf(&w.midBuf, w.midLen)
	w.loLen = collapseExpansionBuf(&w.loBuf, w.loLen)
	hi := w.hiBuf[:w.hiLen]
	mid := w.midBuf[:w.midLen]
	lo := w.loBuf[:w.loLen]

	var midBuf [3 * expansionCap]float64
	midMerged := midBuf[:0]
	if len(hi) == 0 {
		midMerged = append(midMerged, mid...)
	} else {
		hiTop := hi[len(hi)-1]
		_, eHi := math.Frexp(math.Abs(hiTop))
		if eHi+wideHiShift > 1026 {
			return math.Copysign(math.Inf(1), hiTop), 0, false
		}
		var halfBuf [2 * expansionCap]float64
		half := halfBuf[:0]
		for _, h := range hi {
			half = growExpansion(half, math.Ldexp(h, wideHalfShift))
		}
		for _, m := range mid {
			if math.Abs(m) >= 0x1p-400 {
				half = growExpansion(half, math.Ldexp(m, -wideHalfShift))
			}
		}
		half = compressExpansion(half)
		if len(half) > 0 {
			halfTop := half[len(half)-1]
			_, eHalf := math.Frexp(math.Abs(halfTop))
			if eHalf+wideHalfShift > 1024 {
				var sc4Buf [3 * expansionCap]float64
				sc4 := sc4Buf[:0]
				for _, m := range mid {
					if math.Abs(m) < 0x1p-400 && math.Abs(m) >= 0x1p-1018 {
						sc4 = growExpansion(sc4, math.Ldexp(m, -4))
					}
				}
				for _, c := range half {
					sc4 = growExpansion(sc4, math.Ldexp(c, wideHalfShift-4))
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
		for _, m := range mid {
			if math.Abs(m) < 0x1p-400 {
				midMerged = growExpansion(midMerged, m)
			}
		}
		for _, c := range half {
			midMerged = growExpansion(midMerged, math.Ldexp(c, wideHalfShift))
		}
		midMerged = compressExpansion(midMerged)
	}

	if len(lo) == 0 {
		s, e := expansionTopRem(midMerged)
		return s, e, true
	}

	var loSmallBuf [expansionCap]float64
	loSmall := loSmallBuf[:0]
	addedLoNorm := false
	for _, l := range lo {
		if math.Abs(l) >= 0x1p34 {
			midMerged = growExpansion(midMerged, math.Ldexp(math.Ldexp(l, -wideHalfShift), -wideHalfShift))
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
				up = growExpansion(up, math.Ldexp(m, wideHalfShift))
			} else {
				topKeep = append(topKeep, m)
			}
		}
		for _, l := range loSmall {
			if math.Abs(l) >= 0x1p-490 {
				up = growExpansion(up, math.Ldexp(l, -wideHalfShift))
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
				rebuilt = growExpansion(rebuilt, math.Ldexp(u, -wideHalfShift))
			}
		}
		for _, m := range topKeep {
			rebuilt = growExpansion(rebuilt, m)
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

func evalSparseRowDirTol(cols []int, vals, x, lb, ub []float64, minTol, relFactor float64) (rTolLower, rTolUpper float64, ok bool) {
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
			return 0, 0, false
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
			return 0, 0, false
		}
		rTolLower = satAddNonNeg(rTolLower, incRow)
		rTolUpper = satAddNonNeg(rTolUpper, decRow)
	}
	if minTol > 0 {
		rTolLower = max(minTol, rTolLower)
		rTolUpper = max(minTol, rTolUpper)
	}
	return rTolLower, rTolUpper, true
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
	if p == nil || len(p.denseRows) == 0 {
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
