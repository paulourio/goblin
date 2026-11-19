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
	objName     string
	objCoeffs   []float64 // Original objective coefficients as specified
	objMin      []float64 // Objective coefficients normalized for minimizing
	objConstant float64

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

	// Accumulate objective terms
	for _, t := range m.objTerms {
		if col, ok := m.validVar(t.Var); ok && !math.IsNaN(t.Coeff) && !math.IsInf(t.Coeff, 0) {
			p.objCoeffs[col] += t.Coeff
		}
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
	inTouched := make([]bool, n)

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

		p.rowPtr[i] = len(p.colIdx)
		var denseRow []float64
		if useDense {
			denseRow = p.denseRows[i*p.rowStride : (i+1)*p.rowStride]
		}
		for _, col := range touched {
			val := rowAcc[col]
			rowAcc[col] = 0
			inTouched[col] = false
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
	if p.sense == Maximize {
		return -minObj + p.objConstant
	}
	return minObj + p.objConstant
}

// EvaluateObjective computes the original objective value for primal vector x.
func (p *CompiledProblem) EvaluateObjective(x []float64) float64 {
	if p == nil {
		return 0
	}
	sum := p.objConstant
	for j := 0; j < p.numVars && j < len(x); j++ {
		sum += p.objCoeffs[j] * x[j]
	}
	return sum
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
		act := 0.0
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			act += p.rowVal[k] * x[p.colIdx[k]]
		}
		if math.IsNaN(act) {
			rowViol = math.Inf(1)
			continue
		}
		if act < p.rowLB[i] {
			rowViol = max(rowViol, p.rowLB[i]-act)
		}
		if act > p.rowUB[i] {
			rowViol = max(rowViol, act-p.rowUB[i])
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
