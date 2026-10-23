package goblin

import (
	"math"
	"sort"
)

// DenseThreshold is the maximum number of variables for which CompiledProblem
// also prebuilds a contiguous SIMD-aligned dense row matrix.
const DenseThreshold = 256

const (
	bitsPerWord       = 64
	wordBitMask       = bitsPerWord - 1
	simdRowAlign      = 8
	simdRowAlignMask  = simdRowAlign - 1
	defaultTouchedCap = 64
	compileZeroTol    = 1e-15
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
	Name       string
	Sense      Sense
	InvalidErr error
	modelID    uint32

	NumVars        int
	NumConstraints int
	IsMIP          bool

	// Variable metadata (length = NumVars)
	VarNames []string
	VarLB    []float64
	VarUB    []float64
	VarCat   []VarCategory

	// Bitmasks for fast integer/binary checks (each uint64 covers 64 variables).
	BinaryMask  []uint64
	IntegerMask []uint64
	IntCols     []int   // Indices of all Binary or Integer variables
	IntCols32   []int32 // 32-bit indices for direct C++ CBC setInteger calls

	// Normalized objective: the internal solver always minimizes ObjMin[j]*x[j].
	// Original objective value = Sense * (sum_j ObjMin[j]*x[j]) + ObjConstant.
	ObjName     string
	ObjCoeffs   []float64 // Original objective coefficients as specified
	ObjMin      []float64 // Objective coefficients normalized for minimizing
	ObjConstant float64

	// Constraint metadata (length = NumConstraints)
	RowNames []string
	RowSense []ConstraintSense
	RowRHS   []float64
	RowLB    []float64 // Equivalent ranged row lower bound: RowLB[i] <= a_i^T x <= RowUB[i]
	RowUB    []float64 // Equivalent ranged row upper bound

	// CSR (Compressed Sparse Row) representation of constraint matrix A
	RowPtr []int
	ColIdx []int
	RowVal []float64

	// CSC (Compressed Sparse Column) representation of constraint matrix A
	ColPtr []int
	RowIdx []int
	ColVal []float64

	// Pre-clamped and 32-bit-indexed CSC arrays for zero-copy C++ OsiClpSolverInterface::loadProblem
	ColPtr32 []int32
	RowIdx32 []int32
	ColLBClp []float64
	ColUBClp []float64
	RowLBClp []float64
	RowUBClp []float64

	// Contiguous SIMD-padded dense rows when NumVars <= DenseThreshold.
	// RowStride is NumVars rounded up to a multiple of 8 float64s (64 bytes).
	RowStride int
	DenseRows []float64
}

// CompileChecked validates the Model and returns a CompiledProblem or the first check error.
func (m *Model) CompileChecked() (*CompiledProblem, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m.Compile(), nil
}

// Compile transforms a symbolic Model into a flat, canonical CompiledProblem.
// Duplicate variable terms within the objective or within a single constraint
// are summed and zero coefficients are dropped.
func (m *Model) Compile() *CompiledProblem {
	n := len(m.vars)
	mc := len(m.constraints)

	numWords := (n + wordBitMask) / bitsPerWord
	p := &CompiledProblem{
		Name:           m.name,
		Sense:          m.sense,
		InvalidErr:     m.Validate(),
		modelID:        m.id,
		NumVars:        n,
		NumConstraints: mc,
		VarNames:       make([]string, n),
		VarLB:          make([]float64, n),
		VarUB:          make([]float64, n),
		VarCat:         make([]VarCategory, n),
		BinaryMask:     make([]uint64, numWords),
		IntegerMask:    make([]uint64, numWords),
		ObjName:        m.objName,
		ObjCoeffs:      make([]float64, n),
		ObjMin:         make([]float64, n),
		ObjConstant:    m.objConst,
		RowNames:       make([]string, mc),
		RowSense:       make([]ConstraintSense, mc),
		RowRHS:         make([]float64, mc),
		RowLB:          make([]float64, mc),
		RowUB:          make([]float64, mc),
		RowPtr:         make([]int, mc+1),
		ColLBClp:       make([]float64, n),
		ColUBClp:       make([]float64, n),
		RowLBClp:       make([]float64, mc),
		RowUBClp:       make([]float64, mc),
	}

	for j, v := range m.vars {
		p.VarNames[j] = v.name
		p.VarLB[j] = v.lb
		p.VarUB[j] = v.ub
		p.VarCat[j] = v.cat
		p.ColLBClp[j] = clampCBCBound(v.lb)
		p.ColUBClp[j] = clampCBCBound(v.ub)
		if v.cat != Continuous {
			p.IsMIP = true
			p.IntegerMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			p.IntCols = append(p.IntCols, j)
			p.IntCols32 = append(p.IntCols32, int32(j))
			if v.cat == Binary {
				p.BinaryMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			}
		}
	}

	// Accumulate objective terms
	for _, t := range m.objTerms {
		if col, ok := m.validVar(t.Var); ok && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) {
			p.ObjCoeffs[col] += t.Coeff
		}
	}
	senseSign := 1.0
	if m.sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		if p.ObjCoeffs[j] != 0 {
			p.ObjMin[j] = senseSign * p.ObjCoeffs[j]
		}
	}

	// If n <= DenseThreshold, allocate SIMD-aligned dense rows (multiple of 8 float64s)
	useDense := n > 0 && n <= DenseThreshold
	if useDense {
		p.RowStride = (n + simdRowAlignMask) &^ simdRowAlignMask
		p.DenseRows = make([]float64, mc*p.RowStride)
	}

	// Scratch buffers for deduplicating terms in each constraint row
	rowAcc := make([]float64, n)
	touched := make([]int, 0, min(n, defaultTouchedCap))
	inTouched := make([]bool, n)

	totalNNZ := 0
	for i := range mc {
		totalNNZ += len(m.constraints[i].terms)
	}
	p.ColIdx = make([]int, 0, totalNNZ)
	p.RowVal = make([]float64, 0, totalNNZ)

	for i, c := range m.constraints {
		p.RowNames[i] = c.name
		p.RowSense[i] = c.sense
		p.RowRHS[i] = c.rhs

		switch c.sense {
		case LessEqual:
			p.RowLB[i] = math.Inf(-1)
			p.RowUB[i] = c.rhs
		case GreaterEqual:
			p.RowLB[i] = c.rhs
			p.RowUB[i] = math.Inf(1)
		case Equal:
			p.RowLB[i] = c.rhs
			p.RowUB[i] = c.rhs
		}
		p.RowLBClp[i] = clampCBCBound(p.RowLB[i])
		p.RowUBClp[i] = clampCBCBound(p.RowUB[i])

		touched = touched[:0]
		for _, t := range c.terms {
			col, ok := m.validVar(t.Var)
			if !ok || t.Coeff == 0 || math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
				continue
			}
			if !inTouched[col] {
				inTouched[col] = true
				touched = append(touched, col)
			}
			rowAcc[col] += t.Coeff
		}
		sort.Ints(touched)

		p.RowPtr[i] = len(p.ColIdx)
		var denseRow []float64
		if useDense {
			denseRow = p.DenseRows[i*p.RowStride : (i+1)*p.RowStride]
		}
		for _, col := range touched {
			val := rowAcc[col]
			rowAcc[col] = 0
			inTouched[col] = false
			if math.Abs(val) <= compileZeroTol {
				continue
			}
			p.ColIdx = append(p.ColIdx, col)
			p.RowVal = append(p.RowVal, val)
			if useDense {
				denseRow[col] = val
			}
		}
	}
	p.RowPtr[mc] = len(p.ColIdx)

	// Build CSC (Compressed Sparse Column) transpose from CSR
	nnz := len(p.ColIdx)
	p.ColPtr = make([]int, n+1)
	p.ColPtr32 = make([]int32, n+1)
	p.RowIdx = make([]int, nnz)
	p.RowIdx32 = make([]int32, nnz)
	p.ColVal = make([]float64, nnz)

	for _, col := range p.ColIdx {
		p.ColPtr[col+1]++
	}
	for j := range n {
		p.ColPtr[j+1] += p.ColPtr[j]
		p.ColPtr32[j+1] = int32(p.ColPtr[j+1])
	}
	writePos := make([]int, n)
	copy(writePos, p.ColPtr[:n])
	for i := range mc {
		for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
			col := p.ColIdx[k]
			pos := writePos[col]
			p.RowIdx[pos] = i
			p.RowIdx32[pos] = int32(i)
			p.ColVal[pos] = p.RowVal[k]
			writePos[col]++
		}
	}

	return p
}

// ObjectiveFromMin converts an internal minimized objective value back to the
// user's original sense and adds ObjConstant.
func (p *CompiledProblem) ObjectiveFromMin(minObj float64) float64 {
	if p.Sense == Maximize {
		return -minObj + p.ObjConstant
	}
	return minObj + p.ObjConstant
}

// EvaluateObjective computes the original objective value for primal vector x.
func (p *CompiledProblem) EvaluateObjective(x []float64) float64 {
	sum := p.ObjConstant
	for j := 0; j < p.NumVars && j < len(x); j++ {
		sum += p.ObjCoeffs[j] * x[j]
	}
	return sum
}

// MaxViolation computes the largest bound, row, and integer residual of x.
func (p *CompiledProblem) MaxViolation(x []float64, checkIntegrality bool) (boundViol, rowViol, intViol float64) {
	if len(x) < p.NumVars {
		return math.Inf(1), math.Inf(1), math.Inf(1)
	}
	for j := range p.NumVars {
		v := x[j]
		if v < p.VarLB[j] {
			boundViol = max(boundViol, p.VarLB[j]-v)
		}
		if v > p.VarUB[j] {
			boundViol = max(boundViol, v-p.VarUB[j])
		}
		if checkIntegrality && p.VarCat[j] != Continuous {
			diff := math.Abs(v - math.Round(v))
			intViol = max(intViol, diff)
		}
	}
	for i := range p.NumConstraints {
		act := 0.0
		for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
			act += p.RowVal[k] * x[p.ColIdx[k]]
		}
		if act < p.RowLB[i] {
			rowViol = max(rowViol, p.RowLB[i]-act)
		}
		if act > p.RowUB[i] {
			rowViol = max(rowViol, act-p.RowUB[i])
		}
	}
	return boundViol, rowViol, intViol
}
