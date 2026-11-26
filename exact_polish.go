package goblin

import (
	"context"
	"math"
	"math/big"
	"slices"
	"strconv"
	"time"
)

const (
	defaultExactPolishMaxNucleus  = 512
	defaultExactPolishMaxBits     = 4096
	exactNumericalFallbackMaxRows = 64
	exactNumericalFallbackMaxVars = 128
)

// ExactLPSolution holds an exact rational basic primal-dual solution over Q.
type ExactLPSolution struct {
	Primal         []*big.Rat
	DualMin        []*big.Rat
	DualUser       []*big.Rat
	ReducedCostMin []*big.Rat
	ReducedCost    []*big.Rat
	ObjectiveMin   *big.Rat
	ObjectiveUser  *big.Rat
}

// UnboundedCertificate holds an exact rational feasible anchor X0 and recession
// ray Ray certifying LP or MILP unboundedness over Q. When IsMIP is true, X0 and
// Ray are integral on all integer-constrained columns of the problem.
type UnboundedCertificate struct {
	IsMIP    bool
	X0       []*big.Rat
	Ray      []*big.Rat
	ObjSlope *big.Rat
}

type exactSparseTerm struct {
	col int
	val *big.Rat
}

type exactBackStep struct {
	eq  int
	col int
}

type exactSolverSnapshot struct {
	p          *CompiledProblem
	n          int
	m          int
	objConstM  *big.Rat
	objMin     []*big.Rat
	lb         []*big.Rat
	ub         []*big.Rat
	rowSense   []ConstraintSense
	rowRHS     []*big.Rat
	colRows    [][]int
	colVals    [][]*big.Rat
	basis      []int
	status     []varBasisStatus
	nonBasicAt []*big.Rat
}

type exactStop struct {
	deadline        time.Time
	done            <-chan struct{}
	alreadyCanceled bool
	maxLPIter       int
	maxMIPNodes     int
}

func exactStopFromCtx(ctx context.Context) exactStop {
	if ctx == nil {
		return exactStop{}
	}
	if ctx.Err() != nil {
		return exactStop{alreadyCanceled: true}
	}
	dl, _ := ctx.Deadline()
	return exactStop{deadline: dl, done: ctx.Done()}
}

func (st exactStop) canceled() bool {
	return st.alreadyCanceled || stopRequested(st.deadline, st.done)
}

func (st exactStop) applyToLPSolver(lp *LPSolver) {
	if lp == nil {
		return
	}
	lp.Done = st.done
	lp.Deadline = st.deadline
	if st.maxLPIter > 0 {
		lp.MaxIter = st.maxLPIter
	}
}

func exactRatFromFloat(f float64) (*big.Rat, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, false
	}
	r := new(big.Rat).SetFloat64(f)
	if r == nil {
		return nil, false
	}
	return r, true
}

func exactRatFitsBits(r *big.Rat) bool {
	if r == nil {
		return false
	}
	return r.Num().BitLen() <= defaultExactPolishMaxBits &&
		r.Denom().BitLen() <= defaultExactPolishMaxBits
}

func exactRatToFloat(r *big.Rat) float64 {
	if r == nil || r.Sign() == 0 {
		return 0
	}
	v, _ := r.Float64()
	if v == 0 {
		return 0
	}
	return v
}

// exactRatToFloatFloor returns the greatest IEEE-754 binary64 value <= r
// (or math.Inf(-1) when r < -math.MaxFloat64).
func exactRatToFloatFloor(r *big.Rat) float64 {
	if r == nil || r.Sign() == 0 {
		return 0
	}
	f, exact := r.Float64()
	if math.IsInf(f, 1) {
		return math.MaxFloat64
	}
	if math.IsInf(f, -1) {
		return math.Inf(-1)
	}
	if !exact {
		fr := new(big.Rat).SetFloat64(f)
		if fr == nil {
			return math.Inf(-1)
		}
		if fr.Cmp(r) > 0 {
			f = math.Nextafter(f, math.Inf(-1))
		}
	}
	if f == 0 {
		return 0
	}
	return f
}

// exactRatToFloatCeil returns the least IEEE-754 binary64 value >= r
// (or math.Inf(1) when r > math.MaxFloat64).
func exactRatToFloatCeil(r *big.Rat) float64 {
	if r == nil || r.Sign() == 0 {
		return 0
	}
	f, exact := r.Float64()
	if math.IsInf(f, -1) {
		return -math.MaxFloat64
	}
	if math.IsInf(f, 1) {
		return math.Inf(1)
	}
	if !exact {
		fr := new(big.Rat).SetFloat64(f)
		if fr == nil {
			return math.Inf(1)
		}
		if fr.Cmp(r) < 0 {
			f = math.Nextafter(f, math.Inf(1))
		}
	}
	if f == 0 {
		return 0
	}
	return f
}

// exactRatToOutwardBound converts an exact rational dual bound (in user objective
// units) into the outward-rounded float64 global dual bound: floor for Minimize
// (greatest float64 <= r) and ceil for Maximize (least float64 >= r).
func exactRatToOutwardBound(sense Sense, r *big.Rat) float64 {
	if sense == Maximize {
		return exactRatToFloatCeil(r)
	}
	return exactRatToFloatFloor(r)
}

func cloneRatSlice(src []*big.Rat) []*big.Rat {
	out := make([]*big.Rat, len(src))
	for i, r := range src {
		if r != nil {
			out[i] = new(big.Rat).Set(r)
		} else {
			out[i] = new(big.Rat)
		}
	}
	return out
}

func exactObjConstMinRat(p *CompiledProblem) (*big.Rat, bool) {
	if p == nil {
		return nil, false
	}
	if eo := p.exactOrig; eo != nil && eo.objConstRat != nil {
		if p.sense == Maximize {
			return new(big.Rat).Neg(eo.objConstRat), true
		}
		return new(big.Rat).Set(eo.objConstRat), true
	}
	return exactRatFromFloat(p.objConstMin())
}

func exactVarLBRat(p *CompiledProblem, j int) (hasLB bool, lb *big.Rat, ok bool) {
	if p == nil || j < 0 || j >= p.numVars || j >= len(p.varLB) {
		return false, nil, false
	}
	if eo := p.exactOrig; eo != nil && j < len(eo.varLBRat) && eo.varLBRat[j] != nil {
		return true, new(big.Rat).Set(eo.varLBRat[j]), true
	}
	lo := p.varLB[j]
	if math.IsNaN(lo) || math.IsInf(lo, 1) {
		return false, nil, false
	}
	if math.IsInf(lo, -1) {
		return false, nil, true
	}
	r, okR := exactRatFromFloat(lo)
	return true, r, okR
}

func exactVarUBRat(p *CompiledProblem, j int) (hasUB bool, ub *big.Rat, ok bool) {
	if p == nil || j < 0 || j >= p.numVars || j >= len(p.varUB) {
		return false, nil, false
	}
	if eo := p.exactOrig; eo != nil && j < len(eo.varUBRat) && eo.varUBRat[j] != nil {
		return true, new(big.Rat).Set(eo.varUBRat[j]), true
	}
	hi := p.varUB[j]
	if math.IsNaN(hi) || math.IsInf(hi, -1) {
		return false, nil, false
	}
	if math.IsInf(hi, 1) {
		return false, nil, true
	}
	r, okR := exactRatFromFloat(hi)
	return true, r, okR
}

func exactModelTermRat(t exactModelTerm) (*big.Rat, bool) {
	if t.coeffRat != nil {
		return t.coeffRat, true
	}
	return exactRatFromFloat(t.coeff)
}

func exactMinObjVecRat(p *CompiledProblem) ([]*big.Rat, bool) {
	if p == nil {
		return nil, false
	}
	n := p.numVars
	out := make([]*big.Rat, n)
	if eo := p.exactOrig; eo != nil && (eo.objHasLoss || eo.objHasRat) {
		for j := range n {
			out[j] = new(big.Rat)
		}
		for _, t := range eo.objTerms {
			if t.col < 0 || t.col >= n {
				return nil, false
			}
			rv, ok := exactModelTermRat(t)
			if !ok {
				return nil, false
			}
			out[t.col].Add(out[t.col], rv)
		}
		if p.sense == Maximize {
			for j := range n {
				if out[j].Sign() != 0 {
					out[j].Neg(out[j])
				}
			}
		}
		return out, true
	}
	for j := range n {
		cj, ok := exactRatFromFloat(p.objMin[j])
		if !ok {
			return nil, false
		}
		out[j] = cj
	}
	return out, true
}

func exactRowRHSRat(p *CompiledProblem, i int) (*big.Rat, bool) {
	if p == nil || i < 0 || i >= p.numConstraints {
		return nil, false
	}
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) {
		er := eo.rows[i]
		if er.rhsRat != nil || er.exprConstRat != nil {
			rhsR := er.rhsRat
			if rhsR == nil {
				var okR bool
				rhsR, okR = exactRatFromFloat(er.rhs)
				if !okR {
					return nil, false
				}
			}
			ecR := er.exprConstRat
			if ecR == nil {
				var okE bool
				ecR, okE = exactRatFromFloat(er.exprConst)
				if !okE {
					return nil, false
				}
			}
			return new(big.Rat).Sub(rhsR, ecR), true
		}
		if er.rhsInexact {
			r, okR := exactRatFromFloat(er.rhs)
			if !okR {
				return nil, false
			}
			ec, okE := exactRatFromFloat(er.exprConst)
			if !okE {
				return nil, false
			}
			return new(big.Rat).Sub(r, ec), true
		}
	}
	return exactRatFromFloat(p.rowRHS[i])
}

func exactCanonicalRowTermsRat(p *CompiledProblem, i int) ([]int, []*big.Rat, bool) {
	if p == nil || i < 0 || i >= p.numConstraints {
		return nil, nil, false
	}
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasDupLoss || eo.rows[i].hasRatTerms) {
		er := eo.rows[i]
		acc := make(map[int]*big.Rat, er.termEnd-er.termStart)
		cols := make([]int, 0, er.termEnd-er.termStart)
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= p.numVars {
				return nil, nil, false
			}
			rv, ok := exactModelTermRat(t)
			if !ok {
				return nil, nil, false
			}
			cur := acc[t.col]
			if cur == nil {
				cur = new(big.Rat)
				acc[t.col] = cur
				cols = append(cols, t.col)
			}
			cur.Add(cur, rv)
		}
		slices.Sort(cols)
		outCols := make([]int, 0, len(cols))
		outVals := make([]*big.Rat, 0, len(cols))
		for _, col := range cols {
			if v := acc[col]; v != nil && v.Sign() != 0 {
				outCols = append(outCols, col)
				outVals = append(outVals, v)
			}
		}
		return outCols, outVals, true
	}
	st, en := p.rowPtr[i], p.rowPtr[i+1]
	cols := make([]int, 0, en-st)
	vals := make([]*big.Rat, 0, en-st)
	for k := st; k < en; k++ {
		fv := p.rowVal[k]
		if fv == 0 {
			continue
		}
		rv, ok := exactRatFromFloat(fv)
		if !ok {
			return nil, nil, false
		}
		cols = append(cols, p.colIdx[k])
		vals = append(vals, rv)
	}
	return cols, vals, true
}

func exactEvalRowActivityRat(p *CompiledProblem, i int, x []*big.Rat) (*big.Rat, bool) {
	act := new(big.Rat)
	var prod big.Rat
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasDupLoss || eo.rows[i].hasRatTerms) {
		er := eo.rows[i]
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= len(x) {
				return nil, false
			}
			if x[t.col].Sign() == 0 {
				continue
			}
			if t.coeffRat == nil && t.coeff == 0 {
				continue
			}
			if t.coeffRat != nil && t.coeffRat.Sign() == 0 {
				continue
			}
			av, ok := exactModelTermRat(t)
			if !ok {
				return nil, false
			}
			prod.Mul(av, x[t.col])
			act.Add(act, &prod)
		}
		return act, true
	}
	for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
		col := p.colIdx[k]
		if col < 0 || col >= len(x) {
			return nil, false
		}
		fv := p.rowVal[k]
		if fv == 0 || x[col].Sign() == 0 {
			continue
		}
		av, ok := exactRatFromFloat(fv)
		if !ok {
			return nil, false
		}
		prod.Mul(av, x[col])
		act.Add(act, &prod)
	}
	return act, true
}

func exactEvalRowDualActivityRat(p *CompiledProblem, i int, x0, d []*big.Rat) (actX, actD *big.Rat, ok bool) {
	actX = new(big.Rat)
	actD = new(big.Rat)
	var prod big.Rat
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasDupLoss || eo.rows[i].hasRatTerms) {
		er := eo.rows[i]
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= len(x0) || t.col >= len(d) {
				return nil, nil, false
			}
			if t.coeffRat == nil && t.coeff == 0 {
				continue
			}
			if t.coeffRat != nil && t.coeffRat.Sign() == 0 {
				continue
			}
			av, okA := exactModelTermRat(t)
			if !okA {
				return nil, nil, false
			}
			if x0[t.col].Sign() != 0 {
				prod.Mul(av, x0[t.col])
				actX.Add(actX, &prod)
			}
			if d[t.col].Sign() != 0 {
				prod.Mul(av, d[t.col])
				actD.Add(actD, &prod)
			}
		}
		return actX, actD, true
	}
	for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
		col := p.colIdx[k]
		if col < 0 || col >= len(x0) || col >= len(d) {
			return nil, nil, false
		}
		fv := p.rowVal[k]
		if fv == 0 {
			continue
		}
		av, okA := exactRatFromFloat(fv)
		if !okA {
			return nil, nil, false
		}
		if x0[col].Sign() != 0 {
			prod.Mul(av, x0[col])
			actX.Add(actX, &prod)
		}
		if d[col].Sign() != 0 {
			prod.Mul(av, d[col])
			actD.Add(actD, &prod)
		}
	}
	return actX, actD, true
}

func exactColMatrixRat(p *CompiledProblem) ([][]int, [][]*big.Rat, bool) {
	n := p.numVars
	m := p.numConstraints
	colRows := make([][]int, n)
	colVals := make([][]*big.Rat, n)
	for i := range m {
		cols, vals, ok := exactCanonicalRowTermsRat(p, i)
		if !ok {
			return nil, nil, false
		}
		for k, col := range cols {
			colRows[col] = append(colRows[col], i)
			colVals[col] = append(colVals[col], vals[k])
		}
	}
	return colRows, colVals, true
}

func buildExactSolverSnapshot(stop exactStop, s *LPSolver) (*exactSolverSnapshot, bool) {
	if stop.canceled() || s == nil || s.prob == nil || s.ws == nil {
		return nil, false
	}
	if s.invalidBounds || s.invalidCut || s.infeasibleCut {
		return nil, false
	}
	p := s.prob
	ws := s.ws
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()
	tot := n + m
	if len(ws.status) < tot || len(ws.lb) < n || len(ws.ub) < n {
		return nil, false
	}
	if m > 0 && len(ws.basis) < m {
		return nil, false
	}

	var objConstM *big.Rat
	var ok bool
	if p.exactOrig != nil && p.exactOrig.objConstRat != nil {
		objConstM, ok = exactObjConstMinRat(p)
	} else {
		objConstM, ok = exactRatFromFloat(p.objConstMinVal)
	}
	if !ok {
		return nil, false
	}

	snap := &exactSolverSnapshot{
		p:          p,
		n:          n,
		m:          m,
		objConstM:  objConstM,
		objMin:     make([]*big.Rat, n),
		lb:         make([]*big.Rat, n),
		ub:         make([]*big.Rat, n),
		rowSense:   make([]ConstraintSense, m),
		rowRHS:     make([]*big.Rat, m),
		colRows:    make([][]int, n),
		colVals:    make([][]*big.Rat, n),
		status:     slices.Clone(ws.status[:tot]),
		nonBasicAt: make([]*big.Rat, n),
	}
	if m > 0 {
		snap.basis = slices.Clone(ws.basis[:m])
	}

	objMinVec, okObjVec := exactMinObjVecRat(p)
	if !okObjVec {
		return nil, false
	}
	var exactCRows [][]int
	var exactCVals [][]*big.Rat
	if p.exactOrig != nil {
		var okMat bool
		exactCRows, exactCVals, okMat = exactColMatrixRat(p)
		if !okMat {
			return nil, false
		}
	}

	for j := range n {
		if j&255 == 0 && stop.canceled() {
			return nil, false
		}
		snap.objMin[j] = objMinVec[j]

		var loF, hiF float64
		if s.scaled && len(s.origLB) >= n && len(s.origUB) >= n {
			loF, hiF = s.origLB[j], s.origUB[j]
		} else {
			loF, hiF = ws.lb[j], ws.ub[j]
		}
		if math.IsNaN(loF) || math.IsNaN(hiF) || math.IsInf(loF, 1) || math.IsInf(hiF, -1) || loF > hiF {
			return nil, false
		}
		if !math.IsInf(loF, -1) {
			if eo := p.exactOrig; eo != nil && j < len(eo.varLBRat) && eo.varLBRat[j] != nil && j < len(p.varLB) && loF == p.varLB[j] {
				snap.lb[j] = new(big.Rat).Set(eo.varLBRat[j])
			} else {
				lr, okL := exactRatFromFloat(loF)
				if !okL {
					return nil, false
				}
				snap.lb[j] = lr
			}
		}
		if !math.IsInf(hiF, 1) {
			if eo := p.exactOrig; eo != nil && j < len(eo.varUBRat) && eo.varUBRat[j] != nil && j < len(p.varUB) && hiF == p.varUB[j] {
				snap.ub[j] = new(big.Rat).Set(eo.varUBRat[j])
			} else {
				ur, okU := exactRatFromFloat(hiF)
				if !okU {
					return nil, false
				}
				snap.ub[j] = ur
			}
		}

		if exactCRows != nil {
			if len(exactCRows[j]) > 0 {
				snap.colRows[j] = exactCRows[j]
				snap.colVals[j] = exactCVals[j]
			}
		} else {
			st, en := p.colPtr[j], p.colPtr[j+1]
			if en > st {
				rSlice := make([]int, 0, en-st)
				vSlice := make([]*big.Rat, 0, en-st)
				for idx := st; idx < en; idx++ {
					fv := p.colVal[idx]
					if fv == 0 {
						continue
					}
					rv, okV := exactRatFromFloat(fv)
					if !okV {
						return nil, false
					}
					rSlice = append(rSlice, p.rowIdx[idx])
					vSlice = append(vSlice, rv)
				}
				snap.colRows[j] = rSlice
				snap.colVals[j] = vSlice
			}
		}

		if snap.status[j] != statusBasic {
			snap.nonBasicAt[j] = exactNonBasicStructuralVal(snap.status[j], snap.lb[j], snap.ub[j])
		}
	}

	for i := range baseM {
		if i&255 == 0 && stop.canceled() {
			return nil, false
		}
		snap.rowSense[i] = p.rowSense[i]
		rhs, okR := exactRowRHSRat(p, i)
		if !okR {
			return nil, false
		}
		snap.rowRHS[i] = rhs
	}

	for e, dr := range s.extraRows {
		if stop.canceled() {
			return nil, false
		}
		row := baseM + e
		snap.rowSense[row] = LessEqual
		rhsF := dr.RHS
		if s.scaled {
			rhsF = dr.OrigRHS
		}
		rhs, okR := exactRatFromFloat(rhsF)
		if !okR {
			return nil, false
		}
		snap.rowRHS[row] = rhs

		vals := dr.Val
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
		}
		if len(vals) < len(dr.ColIdx) {
			return nil, false
		}
		for k, col := range dr.ColIdx {
			if col < 0 || col >= n {
				return nil, false
			}
			fv := vals[k]
			if fv == 0 {
				continue
			}
			rv, okV := exactRatFromFloat(fv)
			if !okV {
				return nil, false
			}
			snap.appendColEntry(col, row, rv)
		}
	}

	if m > 0 {
		seen := make([]bool, tot)
		for q := range m {
			bVar := snap.basis[q]
			if bVar < 0 || bVar >= tot || seen[bVar] {
				return nil, false
			}
			seen[bVar] = true
		}
	}

	return snap, true
}

func (snap *exactSolverSnapshot) appendColEntry(col, row int, rv *big.Rat) {
	rows := snap.colRows[col]
	for k, existingRow := range rows {
		if existingRow == row {
			snap.colVals[col][k].Add(snap.colVals[col][k], rv)
			return
		}
	}
	snap.colRows[col] = append(rows, row)
	snap.colVals[col] = append(snap.colVals[col], rv)
}

func exactNonBasicStructuralVal(st varBasisStatus, lb, ub *big.Rat) *big.Rat {
	switch st {
	case statusAtLower, statusFixed:
		if lb != nil {
			return new(big.Rat).Set(lb)
		}
		if ub != nil {
			return new(big.Rat).Set(ub)
		}
		return new(big.Rat)
	case statusAtUpper:
		if ub != nil {
			return new(big.Rat).Set(ub)
		}
		if lb != nil {
			return new(big.Rat).Set(lb)
		}
		return new(big.Rat)
	default:
		v := new(big.Rat)
		if lb != nil && v.Cmp(lb) < 0 {
			v.Set(lb)
		}
		if ub != nil && v.Cmp(ub) > 0 {
			v.Set(ub)
		}
		return v
	}
}

func coalesceExactTerms(terms []exactSparseTerm) []exactSparseTerm {
	if len(terms) <= 1 {
		if len(terms) == 1 && terms[0].val.Sign() == 0 {
			return nil
		}
		return terms
	}
	slices.SortFunc(terms, func(a, b exactSparseTerm) int {
		return a.col - b.col
	})
	out := make([]exactSparseTerm, 0, len(terms))
	for _, t := range terms {
		if t.val.Sign() == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1].col == t.col {
			out[len(out)-1].val.Add(out[len(out)-1].val, t.val)
			if out[len(out)-1].val.Sign() == 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, exactSparseTerm{col: t.col, val: new(big.Rat).Set(t.val)})
	}
	return out
}

func solveExactDenseRat(stop exactStop, mat [][]*big.Rat, rhs []*big.Rat) ([]*big.Rat, bool) {
	n := len(rhs)
	if n == 0 {
		return nil, true
	}
	var factor, prod, bestAbs, candAbs big.Rat
	for col := range n {
		if stop.canceled() {
			return nil, false
		}
		piv := -1
		for row := col; row < n; row++ {
			v := mat[row][col]
			if v.Sign() == 0 {
				continue
			}
			candAbs.Abs(v)
			if piv < 0 || candAbs.Cmp(&bestAbs) > 0 {
				bestAbs.Set(&candAbs)
				piv = row
			}
		}
		if piv < 0 {
			return nil, false
		}
		if piv != col {
			mat[col], mat[piv] = mat[piv], mat[col]
			rhs[col], rhs[piv] = rhs[piv], rhs[col]
		}
		pivVal := mat[col][col]
		for row := col + 1; row < n; row++ {
			entry := mat[row][col]
			if entry.Sign() == 0 {
				continue
			}
			factor.Quo(entry, pivVal)
			if !exactRatFitsBits(&factor) {
				return nil, false
			}
			entry.SetInt64(0)
			for k := col + 1; k < n; k++ {
				if mat[col][k].Sign() == 0 {
					continue
				}
				prod.Mul(&factor, mat[col][k])
				mat[row][k].Sub(mat[row][k], &prod)
				if !exactRatFitsBits(mat[row][k]) {
					return nil, false
				}
			}
			if rhs[col].Sign() != 0 {
				prod.Mul(&factor, rhs[col])
				rhs[row].Sub(rhs[row], &prod)
				if !exactRatFitsBits(rhs[row]) {
					return nil, false
				}
			}
		}
	}

	sol := make([]*big.Rat, n)
	var sum big.Rat
	for i := n - 1; i >= 0; i-- {
		if i&63 == 0 && stop.canceled() {
			return nil, false
		}
		sum.Set(rhs[i])
		for j := i + 1; j < n; j++ {
			if mat[i][j].Sign() != 0 && sol[j].Sign() != 0 {
				prod.Mul(mat[i][j], sol[j])
				sum.Sub(&sum, &prod)
			}
		}
		sol[i] = new(big.Rat).Quo(&sum, mat[i][i])
		if !exactRatFitsBits(sol[i]) {
			return nil, false
		}
	}
	return sol, true
}

// solveExactPeelSystem solves a square m x m sparse linear system over Q by peeling
// singleton equations and singleton unknowns before factoring any dense nucleus.
func solveExactPeelSystem(stop exactStop, m int, rawTerms [][]exactSparseTerm, eqRHS []*big.Rat) ([]*big.Rat, bool) {
	if stop.canceled() {
		return nil, false
	}
	if m == 0 {
		return []*big.Rat{}, true
	}
	if len(rawTerms) < m || len(eqRHS) < m {
		return nil, false
	}

	eqTerms := make([][]exactSparseTerm, m)
	colEqs := make([][]int, m)
	eqActive := make([]bool, m)
	eqDeg := make([]int, m)
	varDeg := make([]int, m)
	varKnown := make([]bool, m)
	varDeferred := make([]bool, m)
	w := make([]*big.Rat, m)
	for i := range m {
		w[i] = new(big.Rat)
	}

	for q := range m {
		ents := coalesceExactTerms(rawTerms[q])
		if len(ents) == 0 {
			return nil, false
		}
		eqTerms[q] = ents
		eqActive[q] = true
		eqDeg[q] = len(ents)
		for _, ent := range ents {
			if ent.col < 0 || ent.col >= m {
				return nil, false
			}
			colEqs[ent.col] = append(colEqs[ent.col], q)
			varDeg[ent.col]++
		}
	}

	var rowQ, colQ []int
	var backStack []exactBackStep
	for q := range m {
		if varDeg[q] == 0 {
			return nil, false
		}
		if eqDeg[q] == 1 {
			rowQ = append(rowQ, q)
		}
		if varDeg[q] == 1 {
			colQ = append(colQ, q)
		}
	}

	var tmp, acc big.Rat
	steps := 0
	for len(rowQ) > 0 || len(colQ) > 0 {
		for len(rowQ) > 0 {
			steps++
			if steps&255 == 0 && stop.canceled() {
				return nil, false
			}
			q := rowQ[len(rowQ)-1]
			rowQ = rowQ[:len(rowQ)-1]
			if !eqActive[q] || eqDeg[q] != 1 {
				continue
			}
			targetCol := -1
			var targetVal *big.Rat
			acc.Set(eqRHS[q])
			for _, ent := range eqTerms[q] {
				switch {
				case varKnown[ent.col]:
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						acc.Sub(&acc, &tmp)
					}
				case !varDeferred[ent.col]:
					targetCol = ent.col
					targetVal = ent.val
				}
			}
			if targetCol < 0 || targetVal.Sign() == 0 {
				return nil, false
			}
			w[targetCol].Quo(&acc, targetVal)
			if !exactRatFitsBits(w[targetCol]) {
				return nil, false
			}
			varKnown[targetCol] = true
			eqActive[q] = false
			for _, q2 := range colEqs[targetCol] {
				if eqActive[q2] {
					eqDeg[q2]--
					if eqDeg[q2] == 1 {
						rowQ = append(rowQ, q2)
					}
				}
			}
		}

		for len(colQ) > 0 && len(rowQ) == 0 {
			steps++
			if steps&255 == 0 && stop.canceled() {
				return nil, false
			}
			c := colQ[len(colQ)-1]
			colQ = colQ[:len(colQ)-1]
			if varKnown[c] || varDeferred[c] || varDeg[c] != 1 {
				continue
			}
			actEq := -1
			for _, q := range colEqs[c] {
				if eqActive[q] {
					actEq = q
					break
				}
			}
			if actEq < 0 {
				return nil, false
			}
			backStack = append(backStack, exactBackStep{eq: actEq, col: c})
			varDeferred[c] = true
			eqActive[actEq] = false
			for _, ent := range eqTerms[actEq] {
				if !varKnown[ent.col] && !varDeferred[ent.col] {
					varDeg[ent.col]--
					if varDeg[ent.col] == 1 {
						colQ = append(colQ, ent.col)
					}
				}
			}
		}
	}

	var nucEqs, nucVars []int
	nucVarPos := make([]int, m)
	for i := range m {
		nucVarPos[i] = -1
		if eqActive[i] {
			nucEqs = append(nucEqs, i)
		}
		if !varKnown[i] && !varDeferred[i] {
			nucVarPos[i] = len(nucVars)
			nucVars = append(nucVars, i)
		}
	}
	nNuc := len(nucVars)
	if len(nucEqs) != nNuc || nNuc > defaultExactPolishMaxNucleus {
		return nil, false
	}

	if nNuc > 0 {
		mat := make([][]*big.Rat, nNuc)
		rhsNuc := make([]*big.Rat, nNuc)
		for rIdx, q := range nucEqs {
			row := make([]*big.Rat, nNuc)
			for k := range nNuc {
				row[k] = new(big.Rat)
			}
			rhsR := new(big.Rat).Set(eqRHS[q])
			for _, ent := range eqTerms[q] {
				if varKnown[ent.col] {
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						rhsR.Sub(rhsR, &tmp)
					}
				} else if pos := nucVarPos[ent.col]; pos >= 0 {
					row[pos].Set(ent.val)
				} else {
					return nil, false
				}
			}
			mat[rIdx] = row
			rhsNuc[rIdx] = rhsR
		}
		solNuc, ok := solveExactDenseRat(stop, mat, rhsNuc)
		if !ok {
			return nil, false
		}
		for k, origCol := range nucVars {
			w[origCol].Set(solNuc[k])
			varKnown[origCol] = true
		}
	}

	for _, bs := range slices.Backward(backStack) {
		if stop.canceled() {
			return nil, false
		}
		acc.Set(eqRHS[bs.eq])
		var pivVal *big.Rat
		for _, ent := range eqTerms[bs.eq] {
			if ent.col == bs.col {
				pivVal = ent.val
			} else if varKnown[ent.col] {
				if w[ent.col].Sign() != 0 {
					tmp.Mul(ent.val, w[ent.col])
					acc.Sub(&acc, &tmp)
				}
			} else {
				return nil, false
			}
		}
		if pivVal == nil || pivVal.Sign() == 0 {
			return nil, false
		}
		w[bs.col].Quo(&acc, pivVal)
		if !exactRatFitsBits(w[bs.col]) {
			return nil, false
		}
		varKnown[bs.col] = true
	}

	return w, true
}

// solveBasisTranspose solves B^T yMin = rhsB over Q for the basis stored in snap.
func (snap *exactSolverSnapshot) solveBasisTranspose(stop exactStop, rhsB []*big.Rat) ([]*big.Rat, bool) {
	if snap.m == 0 {
		return []*big.Rat{}, true
	}
	one := big.NewRat(1, 1)
	eqTerms := make([][]exactSparseTerm, snap.m)
	for q, bVar := range snap.basis {
		if bVar < snap.n {
			rows := snap.colRows[bVar]
			vals := snap.colVals[bVar]
			ents := make([]exactSparseTerm, 0, len(rows))
			for k, ri := range rows {
				if vals[k].Sign() != 0 {
					ents = append(ents, exactSparseTerm{col: ri, val: vals[k]})
				}
			}
			if len(ents) == 0 {
				return nil, false
			}
			eqTerms[q] = ents
		} else {
			si := bVar - snap.n
			if si < 0 || si >= snap.m {
				return nil, false
			}
			eqTerms[q] = []exactSparseTerm{{col: si, val: one}}
		}
	}
	return solveExactPeelSystem(stop, snap.m, eqTerms, rhsB)
}

// solveBasisForward solves B f = rhsRow over Q for the basis stored in snap.
func (snap *exactSolverSnapshot) solveBasisForward(stop exactStop, rhsRow []*big.Rat) ([]*big.Rat, bool) {
	if snap.m == 0 {
		return []*big.Rat{}, true
	}
	one := big.NewRat(1, 1)
	eqTerms := make([][]exactSparseTerm, snap.m)
	for q, bVar := range snap.basis {
		if bVar < snap.n {
			rows := snap.colRows[bVar]
			vals := snap.colVals[bVar]
			for k, ri := range rows {
				if vals[k].Sign() != 0 {
					eqTerms[ri] = append(eqTerms[ri], exactSparseTerm{col: q, val: vals[k]})
				}
			}
		} else {
			si := bVar - snap.n
			if si < 0 || si >= snap.m {
				return nil, false
			}
			eqTerms[si] = append(eqTerms[si], exactSparseTerm{col: q, val: one})
		}
	}
	return solveExactPeelSystem(stop, snap.m, eqTerms, rhsRow)
}

func (snap *exactSolverSnapshot) computeReducedCostsMin(yMin []*big.Rat) []*big.Rat {
	rcMin := make([]*big.Rat, snap.n)
	var dot, prod big.Rat
	for j := range snap.n {
		dot.SetInt64(0)
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for k, ri := range rows {
			if yMin[ri].Sign() != 0 && vals[k].Sign() != 0 {
				prod.Mul(yMin[ri], vals[k])
				dot.Add(&dot, &prod)
			}
		}
		rcMin[j] = new(big.Rat).Sub(snap.objMin[j], &dot)
	}
	return rcMin
}

func (snap *exactSolverSnapshot) verifyDualConeBounds(yMin, rcMin []*big.Rat) bool {
	if len(yMin) < snap.m || len(rcMin) < snap.n {
		return false
	}
	for i := range snap.m {
		yi := yMin[i]
		switch snap.rowSense[i] {
		case LessEqual:
			if yi.Sign() > 0 {
				return false
			}
		case GreaterEqual:
			if yi.Sign() < 0 {
				return false
			}
		}
	}
	for j := range snap.n {
		rcj := rcMin[j]
		if rcj.Sign() > 0 && snap.lb[j] == nil {
			return false
		}
		if rcj.Sign() < 0 && snap.ub[j] == nil {
			return false
		}
	}
	return true
}

func (snap *exactSolverSnapshot) userDualsAndRC(yMin, rcMin []*big.Rat) (dualUser, rcUser []*big.Rat) {
	if len(yMin) < snap.m || len(rcMin) < snap.n {
		return nil, nil
	}
	dualUser = make([]*big.Rat, snap.m)
	rcUser = make([]*big.Rat, snap.n)
	neg := snap.p.sense == Maximize
	for i := range snap.m {
		yi := yMin[i]
		if neg {
			dualUser[i] = new(big.Rat).Neg(yi)
		} else {
			dualUser[i] = new(big.Rat).Set(yi)
		}
	}
	for j := range snap.n {
		rcj := rcMin[j]
		if neg {
			rcUser[j] = new(big.Rat).Neg(rcj)
		} else {
			rcUser[j] = new(big.Rat).Set(rcj)
		}
	}
	return dualUser, rcUser
}

// ExactBasisDuals solves B^T y_min = c_{min,B} in exact rational arithmetic for the
// active basis of s and verifies canonical dual-cone signs and zero reduced-cost
// residuals on unbounded variable sides.
//
// Canonical dualMin follows Minimize sign rules (LessEqual <= 0, GreaterEqual >= 0,
// Equal unconstrained); dualUser negates dualMin when s solves a Maximize model.
func ExactBasisDuals(ctx context.Context, s *LPSolver) (dualMin, dualUser []*big.Rat, ok bool) {
	stop := exactStopFromCtx(ctx)
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return nil, nil, false
	}
	yMin, _, dualUser, _, ok := snap.extractExactDuals(stop)
	if !ok {
		return nil, nil, false
	}
	return yMin, dualUser, true
}

func (snap *exactSolverSnapshot) basicObjMinRHS() []*big.Rat {
	rhsB := make([]*big.Rat, snap.m)
	for q, bVar := range snap.basis {
		if bVar < snap.n {
			rhsB[q] = new(big.Rat).Set(snap.objMin[bVar])
		} else {
			rhsB[q] = new(big.Rat)
		}
	}
	return rhsB
}

func (snap *exactSolverSnapshot) extractExactDuals(stop exactStop) (yMin, rcMin, dualUser, rcUser []*big.Rat, ok bool) {
	if snap.m == 0 {
		yMin = []*big.Rat{}
		rcMin = cloneRatSlice(snap.objMin)
		if !snap.verifyDualConeBounds(yMin, rcMin) {
			return nil, nil, nil, nil, false
		}
		dualUser, rcUser = snap.userDualsAndRC(yMin, rcMin)
		return yMin, rcMin, dualUser, rcUser, true
	}

	yMin, ok = snap.solveBasisTranspose(stop, snap.basicObjMinRHS())
	if !ok {
		return nil, nil, nil, nil, false
	}
	rcMin = snap.computeReducedCostsMin(yMin)
	if !snap.verifyDualConeBounds(yMin, rcMin) {
		return nil, nil, nil, nil, false
	}
	dualUser, rcUser = snap.userDualsAndRC(yMin, rcMin)
	return yMin, rcMin, dualUser, rcUser, true
}

// ExactBasisSolution extracts an exact rational basic primal-dual pair from s
// and verifies primal bounds, row constraints, dual signs, and zero duality gap over Q.
func ExactBasisSolution(ctx context.Context, s *LPSolver) (ExactLPSolution, bool) {
	return exactBasisSolutionStop(exactStopFromCtx(ctx), s)
}

func (snap *exactSolverSnapshot) alignDualNonBasicStatuses(varLB, varUB, yMin, rcMin []*big.Rat) bool {
	dualFeas := true
	zeroRat := new(big.Rat)
	var rcVal big.Rat
	tot := snap.n + snap.m
	for v := range tot {
		if snap.status[v] == statusBasic {
			continue
		}
		lbv, ubv := varLB[v], varUB[v]
		if lbv != nil && ubv != nil && lbv.Cmp(ubv) == 0 {
			snap.status[v] = statusFixed
			if v < snap.n {
				snap.nonBasicAt[v] = exactNonBasicStructuralVal(snap.status[v], lbv, ubv)
			}
			continue
		}
		if v < snap.n {
			rcVal.Set(rcMin[v])
		} else {
			rcVal.Neg(yMin[v-snap.n])
		}
		switch sgn := rcVal.Sign(); {
		case sgn > 0:
			if lbv == nil {
				dualFeas = false
			} else {
				snap.status[v] = statusAtLower
			}
		case sgn < 0:
			if ubv == nil {
				dualFeas = false
			} else {
				snap.status[v] = statusAtUpper
			}
		default:
			switch snap.status[v] {
			case statusFixed:
				if lbv != nil {
					snap.status[v] = statusAtLower
				} else if ubv != nil {
					snap.status[v] = statusAtUpper
				} else {
					snap.status[v] = statusSuperBasic
				}
			case statusAtLower:
				if lbv == nil {
					if ubv != nil {
						snap.status[v] = statusAtUpper
					} else {
						snap.status[v] = statusSuperBasic
					}
				}
			case statusAtUpper:
				if ubv == nil {
					if lbv != nil {
						snap.status[v] = statusAtLower
					} else {
						snap.status[v] = statusSuperBasic
					}
				}
			case statusSuperBasic:
				if !exactSatisfiesBounds(zeroRat, lbv, ubv) {
					if lbv != nil {
						snap.status[v] = statusAtLower
					} else if ubv != nil {
						snap.status[v] = statusAtUpper
					}
				}
			}
		}
		if v < snap.n {
			snap.nonBasicAt[v] = exactNonBasicStructuralVal(snap.status[v], lbv, ubv)
		}
	}
	return dualFeas
}

func (snap *exactSolverSnapshot) solveExactDualSimplex(stop exactStop, maxPivots int) (ExactLPSolution, bool) {
	sol, st, _, _, _, _, ok := snap.solveExactDualSimplexOutcome(stop, maxPivots)
	return sol, ok && st == StatusOptimal
}

func (snap *exactSolverSnapshot) solveExactDualSimplexOutcome(
	stop exactStop,
	maxPivots int,
) (sol ExactLPSolution, st Status, pivots int, infeasRow int, infeasDir float64, farkasY []*big.Rat, ok bool) {
	if snap.m == 0 {
		yMin, rcMin, dualUser, rcUser, okD := snap.extractExactDuals(stop)
		if !okD {
			return ExactLPSolution{}, StatusNotSolved, 0, -1, 0, nil, false
		}
		solOpt, okSol := snap.extractExactSolutionFromDuals(stop, yMin, rcMin, dualUser, rcUser)
		if !okSol {
			return ExactLPSolution{}, StatusNotSolved, 0, -1, 0, nil, false
		}
		return solOpt, StatusOptimal, 0, -1, 0, nil, true
	}
	n := snap.n
	m := snap.m
	tot := n + m
	varLB := make([]*big.Rat, tot)
	varUB := make([]*big.Rat, tot)
	copy(varLB[:n], snap.lb)
	copy(varUB[:n], snap.ub)
	zeroRat := new(big.Rat)
	for i := range m {
		switch snap.rowSense[i] {
		case LessEqual:
			varLB[n+i] = zeroRat
		case GreaterEqual:
			varUB[n+i] = zeroRat
		case Equal:
			varLB[n+i] = zeroRat
			varUB[n+i] = zeroRat
		}
	}

	eQ := make([]*big.Rat, m)
	for i := range m {
		eQ[i] = new(big.Rat)
	}
	var prod, alphaK, rcK, candRatio, bestRatio big.Rat
	for pivotIdx := range maxPivots {
		if stop.canceled() {
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}
		yMin, okY := snap.solveBasisTranspose(stop, snap.basicObjMinRHS())
		if !okY || len(yMin) != m {
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}
		rcMin := snap.computeReducedCostsMin(yMin)
		for i := range m {
			if !exactRatFitsBits(yMin[i]) {
				return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
			}
		}
		for j := range n {
			if !exactRatFitsBits(rcMin[j]) {
				return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
			}
		}
		if !snap.alignDualNonBasicStatuses(varLB, varUB, yMin, rcMin) {
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}

		rhsAdj := cloneRatSlice(snap.rowRHS)
		for j := range n {
			if snap.status[j] == statusBasic {
				continue
			}
			xj := snap.nonBasicAt[j]
			if xj == nil || xj.Sign() == 0 {
				continue
			}
			rows := snap.colRows[j]
			vals := snap.colVals[j]
			for k, ri := range rows {
				prod.Mul(vals[k], xj)
				rhsAdj[ri].Sub(rhsAdj[ri], &prod)
			}
		}
		xB, okX := snap.solveBasisForward(stop, rhsAdj)
		if !okX || len(xB) != m {
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}
		for q := range m {
			if !exactRatFitsBits(xB[q]) {
				return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
			}
		}

		leaveQ := -1
		leaveVar := tot
		leaveAtLower := false
		for q := range m {
			v := snap.basis[q]
			if varLB[v] != nil && xB[q].Cmp(varLB[v]) < 0 {
				if v < leaveVar {
					leaveQ = q
					leaveVar = v
					leaveAtLower = true
				}
			} else if varUB[v] != nil && xB[q].Cmp(varUB[v]) > 0 {
				if v < leaveVar {
					leaveQ = q
					leaveVar = v
					leaveAtLower = false
				}
			}
		}
		if leaveQ < 0 {
			if !snap.verifyDualConeBounds(yMin, rcMin) {
				return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
			}
			dualUser, rcUser := snap.userDualsAndRC(yMin, rcMin)
			solOpt, okSol := snap.extractExactSolutionFromDuals(stop, yMin, rcMin, dualUser, rcUser)
			if !okSol {
				return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
			}
			return solOpt, StatusOptimal, pivotIdx, -1, 0, nil, true
		}

		eQ[leaveQ].SetInt64(1)
		rho, okRho := snap.solveBasisTranspose(stop, eQ)
		eQ[leaveQ].SetInt64(0)
		if !okRho || len(rho) != m {
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}

		enterK := -1
		for k := range tot {
			st := snap.status[k]
			if st == statusBasic || st == statusFixed {
				continue
			}
			if k < n {
				alphaK.SetInt64(0)
				rows := snap.colRows[k]
				vals := snap.colVals[k]
				for idx, ri := range rows {
					if rho[ri].Sign() != 0 {
						prod.Mul(vals[idx], rho[ri])
						alphaK.Add(&alphaK, &prod)
					}
				}
			} else {
				alphaK.Set(rho[k-n])
			}
			aSgn := alphaK.Sign()
			if aSgn == 0 {
				continue
			}
			if k < n {
				rcK.Set(rcMin[k])
			} else {
				rcK.Neg(yMin[k-n])
			}
			blocks := false
			switch {
			case st == statusSuperBasic:
				blocks = true
				candRatio.SetInt64(0)
			case leaveAtLower && ((st == statusAtLower && aSgn < 0) || (st == statusAtUpper && aSgn > 0)):
				blocks = true
				candRatio.Quo(&rcK, &alphaK)
				candRatio.Neg(&candRatio)
			case !leaveAtLower && ((st == statusAtLower && aSgn > 0) || (st == statusAtUpper && aSgn < 0)):
				blocks = true
				candRatio.Quo(&rcK, &alphaK)
			}
			if !blocks || candRatio.Sign() < 0 {
				continue
			}
			if enterK < 0 || candRatio.Cmp(&bestRatio) < 0 {
				enterK = k
				bestRatio.Set(&candRatio)
			}
		}
		if enterK < 0 {
			candPos := make([]*big.Rat, m)
			candNeg := make([]*big.Rat, m)
			for i := range m {
				candPos[i] = new(big.Rat).Set(rho[i])
				candNeg[i] = new(big.Rat).Neg(rho[i])
			}
			first, second := candPos, candNeg
			dir := -1.0
			if leaveAtLower {
				first, second = candNeg, candPos
				dir = 1.0
			}
			if w, okF := verifyExactSnapshotFarkas(snap, first); okF {
				return ExactLPSolution{}, StatusInfeasible, pivotIdx, leaveQ, dir, w, true
			}
			if w, okF := verifyExactSnapshotFarkas(snap, second); okF {
				return ExactLPSolution{}, StatusInfeasible, pivotIdx, leaveQ, -dir, w, true
			}
			return ExactLPSolution{}, StatusNotSolved, pivotIdx, -1, 0, nil, false
		}

		snap.basis[leaveQ] = enterK
		snap.status[enterK] = statusBasic
		if enterK < n {
			snap.nonBasicAt[enterK] = nil
		}
		if varLB[leaveVar] != nil && varUB[leaveVar] != nil && varLB[leaveVar].Cmp(varUB[leaveVar]) == 0 {
			snap.status[leaveVar] = statusFixed
		} else if leaveAtLower {
			snap.status[leaveVar] = statusAtLower
		} else {
			snap.status[leaveVar] = statusAtUpper
		}
		if leaveVar < n {
			snap.nonBasicAt[leaveVar] = exactNonBasicStructuralVal(snap.status[leaveVar], varLB[leaveVar], varUB[leaveVar])
		}
	}
	return ExactLPSolution{}, StatusNotSolved, maxPivots, -1, 0, nil, false
}

func verifyExactSnapshotFarkas(snap *exactSolverSnapshot, cand []*big.Rat) ([]*big.Rat, bool) {
	if snap == nil || len(cand) < snap.m {
		return nil, false
	}
	w := make([]*big.Rat, snap.m)
	anyNonZero := false
	wDotB := new(big.Rat)
	var tmp big.Rat
	for i := range snap.m {
		wi := new(big.Rat).Set(cand[i])
		switch snap.rowSense[i] {
		case LessEqual:
			if wi.Sign() > 0 {
				wi.SetInt64(0)
			}
		case GreaterEqual:
			if wi.Sign() < 0 {
				wi.SetInt64(0)
			}
		}
		w[i] = wi
		if wi.Sign() != 0 {
			anyNonZero = true
			if snap.rowRHS[i] != nil && snap.rowRHS[i].Sign() != 0 {
				tmp.Mul(wi, snap.rowRHS[i])
				wDotB.Add(wDotB, &tmp)
			}
		}
	}
	if !anyNonZero {
		return nil, false
	}

	maxLHS := new(big.Rat)
	var aj, b1, b2 big.Rat
	for j := range snap.n {
		lbj, ubj := snap.lb[j], snap.ub[j]
		if lbj != nil && ubj != nil && lbj.Cmp(ubj) > 0 {
			return w, true
		}
		aj.SetInt64(0)
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for k, ri := range rows {
			if uint(ri) < uint(snap.m) && w[ri].Sign() != 0 {
				tmp.Mul(vals[k], w[ri])
				aj.Add(&aj, &tmp)
			}
		}
		sgn := aj.Sign()
		if sgn == 0 {
			continue
		}
		hasL := lbj != nil
		hasU := ubj != nil
		if (!hasU && sgn > 0) || (!hasL && sgn < 0) {
			return nil, false
		}
		switch {
		case hasL && hasU:
			b1.Mul(lbj, &aj)
			b2.Mul(ubj, &aj)
			if b2.Cmp(&b1) > 0 {
				maxLHS.Add(maxLHS, &b2)
			} else {
				maxLHS.Add(maxLHS, &b1)
			}
		case hasL:
			b1.Mul(lbj, &aj)
			maxLHS.Add(maxLHS, &b1)
		case hasU:
			b2.Mul(ubj, &aj)
			maxLHS.Add(maxLHS, &b2)
		}
	}
	if maxLHS.Cmp(wDotB) < 0 {
		return w, true
	}
	return nil, false
}

func (snap *exactSolverSnapshot) resetToAllSlackBasis() {
	n := snap.n
	m := snap.m
	tot := n + m
	if len(snap.basis) < m {
		snap.basis = make([]int, m)
	}
	snap.basis = snap.basis[:m]
	if len(snap.status) < tot {
		snap.status = make([]varBasisStatus, tot)
	}
	snap.status = snap.status[:tot]
	if len(snap.nonBasicAt) < n {
		snap.nonBasicAt = make([]*big.Rat, n)
	}
	zeroRat := new(big.Rat)
	for j := range n {
		lb, ub := snap.lb[j], snap.ub[j]
		c := snap.objMin[j]
		switch {
		case lb != nil && ub != nil && lb.Cmp(ub) == 0:
			snap.status[j] = statusFixed
			snap.nonBasicAt[j] = new(big.Rat).Set(lb)
		case c != nil && c.Sign() < 0 && ub != nil:
			snap.status[j] = statusAtUpper
			snap.nonBasicAt[j] = new(big.Rat).Set(ub)
		case lb != nil && (c == nil || c.Sign() >= 0 || ub == nil):
			snap.status[j] = statusAtLower
			snap.nonBasicAt[j] = new(big.Rat).Set(lb)
		case ub != nil:
			snap.status[j] = statusAtUpper
			snap.nonBasicAt[j] = new(big.Rat).Set(ub)
		default:
			snap.status[j] = statusSuperBasic
			snap.nonBasicAt[j] = new(big.Rat).Set(zeroRat)
		}
	}
	for i := range m {
		slack := n + i
		snap.basis[i] = slack
		snap.status[slack] = statusBasic
	}
}

func ratSliceToFiniteFloats(src []*big.Rat, count int) ([]float64, bool) {
	if len(src) < count {
		return nil, false
	}
	out := make([]float64, count)
	for i := range count {
		v := exactRatToFloat(src[i])
		if isNonFinite(v) {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func (s *LPSolver) syncBasisFromSnap(snap *exactSolverSnapshot, primal []float64) {
	ws := s.ws
	n := s.prob.numVars
	m := s.TotalRows()
	tot := n + m
	copy(ws.basis[:m], snap.basis[:m])
	copy(ws.status[:tot], snap.status[:tot])
	for k := range tot {
		ws.basicRow[k] = -1
	}
	for q, bVar := range ws.basis[:m] {
		if uint(bVar) < uint(tot) {
			ws.basicRow[bVar] = q
		}
	}
	s.luValid = false
	ws.lu.valid = false
	s.dualValid = false
	s.basicBoundsValid = false
	s.varTolValid = false

	if len(primal) < n || len(snap.rowRHS) < m {
		return
	}
	primalRat := make([]*big.Rat, n)
	for j := range n {
		rj, okR := exactRatFromFloat(primal[j])
		if !okR {
			return
		}
		primalRat[j] = rj
		xv := primal[j]
		if s.scaled {
			xv *= s.invColScale[j]
			if isNonFinite(xv) {
				return
			}
		}
		ws.x[j] = xv
	}
	rowAct := make([]*big.Rat, m)
	for i := range m {
		rowAct[i] = new(big.Rat)
	}
	var prod big.Rat
	for j := range n {
		xj := primalRat[j]
		if xj.Sign() == 0 {
			continue
		}
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for idx, ri := range rows {
			if uint(ri) < uint(m) {
				prod.Mul(vals[idx], xj)
				rowAct[ri].Add(rowAct[ri], &prod)
			}
		}
	}
	var slackRat big.Rat
	for i := range m {
		slackRat.Sub(snap.rowRHS[i], rowAct[i])
		sv := exactRatToFloat(&slackRat)
		if s.scaled {
			sv *= s.rowScaleOf(i)
		}
		if !isNonFinite(sv) {
			ws.x[n+i] = sv
		}
	}
	for q := range m {
		if bVar := ws.basis[q]; uint(bVar) < uint(tot) {
			ws.xb[q] = ws.x[bVar]
		}
	}
}

func (s *LPSolver) tryExactNumericalFallback(totalIter *int) (LPResult, bool) {
	if s == nil || s.prob == nil || s.ws == nil || s.fastHeuristicPrimal || s.invalidBounds || s.invalidCut || s.infeasibleCut {
		return LPResult{}, false
	}
	p := s.prob
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()
	if m > exactNumericalFallbackMaxRows || n > exactNumericalFallbackMaxVars {
		return LPResult{}, false
	}
	rem := s.MaxIter - *totalIter
	if rem <= 0 {
		return LPResult{}, false
	}
	stop := exactStop{deadline: s.Deadline, done: s.Done}
	if stop.canceled() {
		return LPResult{}, false
	}
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return LPResult{}, false
	}
	maxPivots := min(rem, min(128, max(32, m+16)))
	sol, st, pivots, infeasRow, infeasDir, farkasY, ok := snap.solveExactDualSimplexOutcome(stop, maxPivots)
	*totalIter += pivots
	s.lastDualIters += pivots
	if !ok {
		rem = s.MaxIter - *totalIter
		if rem <= 0 || stop.canceled() {
			return LPResult{}, false
		}
		snap.resetToAllSlackBasis()
		sol, st, pivots, infeasRow, infeasDir, farkasY, ok = snap.solveExactDualSimplexOutcome(stop, min(rem, min(128, max(32, m+16))))
		*totalIter += pivots
		s.lastDualIters += pivots
		if !ok {
			return LPResult{}, false
		}
	}

	switch st {
	case StatusOptimal:
		primal, okP := ratSliceToFiniteFloats(sol.Primal, n)
		duals, okD := ratSliceToFiniteFloats(sol.DualUser, baseM)
		rc, okR := ratSliceToFiniteFloats(sol.ReducedCost, n)
		if !okP || !okD || !okR {
			return LPResult{}, false
		}
		objMin := evalLinearSum(0, p.objMin, primal, n)
		userObj := p.EvaluateObjective(primal)
		if isNonFinite(objMin) || isNonFinite(userObj) || !s.primalResidualsWithinTol(primal) {
			return LPResult{}, false
		}
		s.syncBasisFromSnap(snap, primal)
		return LPResult{
			Status:       StatusOptimal,
			ObjectiveMin: objMin,
			Objective:    userObj,
			Primal:       primal,
			Dual:         duals,
			ReducedCosts: rc,
			Iterations:   *totalIter,
		}, true

	case StatusInfeasible:
		s.syncBasisFromSnap(snap, nil)
		s.lastPrimalFallback = false
		s.lastInfeasRow = infeasRow
		s.lastInfeasDir = infeasDir
		var duals []float64
		if baseM > 0 {
			if candDuals, okF := ratSliceToFiniteFloats(farkasY, baseM); okF {
				for _, v := range candDuals {
					if v != 0 {
						duals = candDuals
						break
					}
				}
			}
		}
		return LPResult{
			Status:     StatusInfeasible,
			Dual:       duals,
			Iterations: *totalIter,
		}, true
	}
	return LPResult{}, false
}

// exactPolishBasisSolutionStop tries the current basis first and then runs bounded
// exact-rational dual simplex pivots with Bland's rule to clear sub-tolerance slack violations.
func exactPolishBasisSolutionStop(stop exactStop, s *LPSolver) (ExactLPSolution, bool) {
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return ExactLPSolution{}, false
	}
	if yMin, rcMin, dualUser, rcUser, okD := snap.extractExactDuals(stop); okD {
		if sol, okSol := snap.extractExactSolutionFromDuals(stop, yMin, rcMin, dualUser, rcUser); okSol {
			return sol, true
		}
	}
	return snap.solveExactDualSimplex(stop, min(128, max(32, snap.m+16)))
}

func (snap *exactSolverSnapshot) extractExactSolutionFromDuals(
	stop exactStop,
	yMin, rcMin, dualUser, rcUser []*big.Rat,
) (ExactLPSolution, bool) {
	primal, ok := snap.extractExactPrimal(stop)
	if !ok {
		return ExactLPSolution{}, false
	}

	objMin := new(big.Rat).Set(snap.objConstM)
	var prod big.Rat
	for j := range snap.n {
		if snap.objMin[j].Sign() != 0 && primal[j].Sign() != 0 {
			prod.Mul(snap.objMin[j], primal[j])
			objMin.Add(objMin, &prod)
		}
	}

	dualBound, ok := snap.evalSafeDualBoundMin(yMin, rcMin)
	if !ok || dualBound.Cmp(objMin) != 0 {
		return ExactLPSolution{}, false
	}

	objUser := new(big.Rat).Set(objMin)
	if snap.p.sense == Maximize {
		// objMin = -userLin + objConstMinVal where objConstMinVal = -objConstant;
		// hence userObj = -objMin.
		objUser.Neg(objMin)
	}

	return ExactLPSolution{
		Primal:         primal,
		DualMin:        yMin,
		DualUser:       dualUser,
		ReducedCostMin: rcMin,
		ReducedCost:    rcUser,
		ObjectiveMin:   objMin,
		ObjectiveUser:  objUser,
	}, true
}

func exactBasisSolutionStop(stop exactStop, s *LPSolver) (ExactLPSolution, bool) {
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return ExactLPSolution{}, false
	}
	yMin, rcMin, dualUser, rcUser, ok := snap.extractExactDuals(stop)
	if !ok {
		return ExactLPSolution{}, false
	}
	return snap.extractExactSolutionFromDuals(stop, yMin, rcMin, dualUser, rcUser)
}

func (snap *exactSolverSnapshot) extractExactPrimal(stop exactStop) ([]*big.Rat, bool) {
	primal := make([]*big.Rat, snap.n)
	if snap.m == 0 {
		for j := range snap.n {
			cSign := snap.objMin[j].Sign()
			switch {
			case cSign > 0:
				if snap.lb[j] == nil {
					return nil, false
				}
				primal[j] = new(big.Rat).Set(snap.lb[j])
			case cSign < 0:
				if snap.ub[j] == nil {
					return nil, false
				}
				primal[j] = new(big.Rat).Set(snap.ub[j])
			default:
				primal[j] = exactNonBasicStructuralVal(snap.status[j], snap.lb[j], snap.ub[j])
			}
			if !exactSatisfiesBounds(primal[j], snap.lb[j], snap.ub[j]) {
				return nil, false
			}
		}
		return primal, true
	}

	rhsAdj := cloneRatSlice(snap.rowRHS)
	var prod big.Rat
	for j := range snap.n {
		if snap.status[j] == statusBasic {
			continue
		}
		xj := snap.nonBasicAt[j]
		primal[j] = new(big.Rat).Set(xj)
		if xj.Sign() == 0 {
			continue
		}
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for k, ri := range rows {
			prod.Mul(vals[k], xj)
			rhsAdj[ri].Sub(rhsAdj[ri], &prod)
		}
	}

	xb, ok := snap.solveBasisForward(stop, rhsAdj)
	if !ok {
		return nil, false
	}
	for q, bVar := range snap.basis {
		val := xb[q]
		if bVar < snap.n {
			primal[bVar] = val
		} else {
			si := bVar - snap.n
			switch snap.rowSense[si] {
			case LessEqual:
				if val.Sign() < 0 {
					return nil, false
				}
			case GreaterEqual:
				if val.Sign() > 0 {
					return nil, false
				}
			case Equal:
				if val.Sign() != 0 {
					return nil, false
				}
			}
		}
	}
	for j := range snap.n {
		if primal[j] == nil || !exactSatisfiesBounds(primal[j], snap.lb[j], snap.ub[j]) {
			return nil, false
		}
	}
	if !snap.verifyExactRows(primal) {
		return nil, false
	}
	return primal, true
}

func exactSatisfiesBounds(x, lb, ub *big.Rat) bool {
	if lb != nil && x.Cmp(lb) < 0 {
		return false
	}
	if ub != nil && x.Cmp(ub) > 0 {
		return false
	}
	return true
}

func (snap *exactSolverSnapshot) verifyExactRows(x []*big.Rat) bool {
	act := make([]*big.Rat, snap.m)
	for i := range snap.m {
		act[i] = new(big.Rat)
	}
	var prod big.Rat
	for j := range snap.n {
		if x[j].Sign() == 0 {
			continue
		}
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for k, ri := range rows {
			prod.Mul(vals[k], x[j])
			act[ri].Add(act[ri], &prod)
		}
	}
	for i := range snap.m {
		cmp := act[i].Cmp(snap.rowRHS[i])
		switch snap.rowSense[i] {
		case LessEqual:
			if cmp > 0 {
				return false
			}
		case GreaterEqual:
			if cmp < 0 {
				return false
			}
		case Equal:
			if cmp != 0 {
				return false
			}
		}
	}
	return true
}

func (snap *exactSolverSnapshot) evalSafeDualBoundMin(yMin, rcMin []*big.Rat) (*big.Rat, bool) {
	bnd := new(big.Rat).Set(snap.objConstM)
	var prod big.Rat
	for i := range snap.m {
		yi := new(big.Rat).Set(yMin[i])
		if snap.rowSense[i] == LessEqual && yi.Sign() > 0 {
			yi.SetInt64(0)
		}
		if snap.rowSense[i] == GreaterEqual && yi.Sign() < 0 {
			yi.SetInt64(0)
		}
		if yi.Sign() != 0 && snap.rowRHS[i].Sign() != 0 {
			prod.Mul(yi, snap.rowRHS[i])
			bnd.Add(bnd, &prod)
		}
	}
	for j := range snap.n {
		s := rcMin[j].Sign()
		if s > 0 {
			if snap.lb[j] == nil {
				return nil, false
			}
			prod.Mul(rcMin[j], snap.lb[j])
			bnd.Add(bnd, &prod)
		} else if s < 0 {
			if snap.ub[j] == nil {
				return nil, false
			}
			prod.Mul(rcMin[j], snap.ub[j])
			bnd.Add(bnd, &prod)
		}
	}
	return bnd, true
}

// ExactSafeDualBound evaluates the exact rational Neumaier-Shcherbina safe dual
// lower bound on p's minimized objective from canonical Minimize duals dualMin.
func ExactSafeDualBound(p *CompiledProblem, dualMin []*big.Rat) (*big.Rat, bool) {
	if p == nil || p.invalidErr != nil || len(dualMin) < p.numConstraints {
		return nil, false
	}
	n := p.numVars
	m := p.numConstraints
	var objConst *big.Rat
	var ok bool
	if p.exactOrig != nil && p.exactOrig.objConstRat != nil {
		objConst, ok = exactObjConstMinRat(p)
	} else {
		objConst, ok = exactRatFromFloat(p.objConstMinVal)
	}
	if !ok {
		return nil, false
	}
	yProj := make([]*big.Rat, m)
	bnd := new(big.Rat).Set(objConst)
	var prod big.Rat
	for i := range m {
		yi := new(big.Rat)
		if dualMin[i] != nil {
			yi.Set(dualMin[i])
		}
		if p.rowSense[i] == LessEqual && yi.Sign() > 0 {
			yi.SetInt64(0)
		}
		if p.rowSense[i] == GreaterEqual && yi.Sign() < 0 {
			yi.SetInt64(0)
		}
		yProj[i] = yi
		if yi.Sign() != 0 {
			rhs, okR := exactRowRHSRat(p, i)
			if !okR {
				return nil, false
			}
			prod.Mul(yi, rhs)
			bnd.Add(bnd, &prod)
		}
	}

	var dot, rc big.Rat
	if p.exactOrig != nil {
		objMinVec, okObj := exactMinObjVecRat(p)
		if !okObj {
			return nil, false
		}
		colDots := make([]*big.Rat, n)
		for j := range n {
			colDots[j] = new(big.Rat)
		}
		for i := range m {
			yi := yProj[i]
			if yi.Sign() == 0 {
				continue
			}
			cols, vals, okR := exactCanonicalRowTermsRat(p, i)
			if !okR {
				return nil, false
			}
			for k, col := range cols {
				prod.Mul(yi, vals[k])
				colDots[col].Add(colDots[col], &prod)
			}
		}
		for j := range n {
			rc.Sub(objMinVec[j], colDots[j])
			if rc.Sign() > 0 {
				hasLB, lb, okL := exactVarLBRat(p, j)
				if !hasLB || !okL {
					return nil, false
				}
				prod.Mul(&rc, lb)
				bnd.Add(bnd, &prod)
			} else if rc.Sign() < 0 {
				hasUB, ub, okU := exactVarUBRat(p, j)
				if !hasUB || !okU {
					return nil, false
				}
				prod.Mul(&rc, ub)
				bnd.Add(bnd, &prod)
			}
		}
		return bnd, true
	}

	for j := range n {
		cj, okC := exactRatFromFloat(p.objMin[j])
		if !okC {
			return nil, false
		}
		dot.SetInt64(0)
		for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
			ri := p.rowIdx[idx]
			if yProj[ri].Sign() == 0 || p.colVal[idx] == 0 {
				continue
			}
			av, okA := exactRatFromFloat(p.colVal[idx])
			if !okA {
				return nil, false
			}
			prod.Mul(yProj[ri], av)
			dot.Add(&dot, &prod)
		}
		rc.Sub(cj, &dot)
		if rc.Sign() > 0 {
			if math.IsInf(p.varLB[j], -1) {
				return nil, false
			}
			lb, okL := exactRatFromFloat(p.varLB[j])
			if !okL {
				return nil, false
			}
			prod.Mul(&rc, lb)
			bnd.Add(bnd, &prod)
		} else if rc.Sign() < 0 {
			if math.IsInf(p.varUB[j], 1) {
				return nil, false
			}
			ub, okU := exactRatFromFloat(p.varUB[j])
			if !okU {
				return nil, false
			}
			prod.Mul(&rc, ub)
			bnd.Add(bnd, &prod)
		}
	}
	return bnd, true
}

// PolishExactBasisDuals refines res in place using the exact rational basis of s
// when s is at StatusOptimal and within exact factorization limits.
func PolishExactBasisDuals(ctx context.Context, s *LPSolver, res *LPResult) bool {
	return polishExactBasisDuals(exactStopFromCtx(ctx), s, res)
}

func copyRatSliceToFloat(dst *[]float64, src []*big.Rat) {
	if len(*dst) != len(src) {
		*dst = make([]float64, len(src))
	}
	out := *dst
	for i, r := range src {
		out[i] = exactRatToFloat(r)
	}
}

func polishExactBasisDuals(stop exactStop, s *LPSolver, res *LPResult) bool {
	if res == nil || res.Status != StatusOptimal {
		return false
	}
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return false
	}
	yMin, rcMin, dualUser, rcUser, ok := snap.extractExactDuals(stop)
	if !ok {
		return false
	}
	if sol, okSol := snap.extractExactSolutionFromDuals(stop, yMin, rcMin, dualUser, rcUser); okSol {
		copyRatSliceToFloat(&res.Primal, sol.Primal)
		copyRatSliceToFloat(&res.Dual, sol.DualUser)
		copyRatSliceToFloat(&res.ReducedCosts, sol.ReducedCost)
		res.ObjectiveMin = exactRatToFloat(sol.ObjectiveMin)
		res.Objective = exactRatToFloat(sol.ObjectiveUser)
		return true
	}
	copyRatSliceToFloat(&res.Dual, dualUser)
	copyRatSliceToFloat(&res.ReducedCosts, rcUser)
	return true
}

// VerifyUnboundedCertificate checks in exact rational arithmetic that cert is a
// valid LP unboundedness certificate (or Meyer MILP unboundedness certificate when
// p has integer-constrained columns).
func VerifyUnboundedCertificate(p *CompiledProblem, cert UnboundedCertificate) bool {
	if p == nil || p.invalidErr != nil {
		return false
	}
	n := p.numVars
	m := p.numConstraints
	if len(cert.X0) != n || len(cert.Ray) != n {
		return false
	}
	needMIP := p.isMIP && len(p.intCols) > 0
	if needMIP && !cert.IsMIP {
		return false
	}

	var objMinVec []*big.Rat
	if p.exactOrig != nil && (p.exactOrig.objHasLoss || p.exactOrig.objHasRat) {
		var okO bool
		objMinVec, okO = exactMinObjVecRat(p)
		if !okO {
			return false
		}
	}
	var slope, prod big.Rat
	for j := range n {
		x0j := cert.X0[j]
		dj := cert.Ray[j]
		if x0j == nil || dj == nil {
			return false
		}
		hasLB, loRat, okL := exactVarLBRat(p, j)
		hasUB, hiRat, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return false
		}
		if hasLB && hasUB && loRat.Cmp(hiRat) > 0 {
			return false
		}
		if hasLB {
			if x0j.Cmp(loRat) < 0 {
				return false
			}
			if dj.Sign() < 0 {
				return false
			}
		}
		if hasUB {
			if x0j.Cmp(hiRat) > 0 {
				return false
			}
			if dj.Sign() > 0 {
				return false
			}
		}
		if (needMIP || cert.IsMIP) && p.varCat[j] != Continuous {
			if !x0j.IsInt() || !dj.IsInt() {
				return false
			}
		}
		if dj.Sign() != 0 {
			if objMinVec != nil {
				if objMinVec[j].Sign() != 0 {
					prod.Mul(objMinVec[j], dj)
					slope.Add(&slope, &prod)
				}
			} else if p.objMin[j] != 0 {
				cj, ok := exactRatFromFloat(p.objMin[j])
				if !ok {
					return false
				}
				prod.Mul(cj, dj)
				slope.Add(&slope, &prod)
			}
		}
	}
	if slope.Sign() >= 0 {
		return false
	}
	if cert.ObjSlope != nil && cert.ObjSlope.Cmp(&slope) != 0 {
		return false
	}

	for i := range m {
		rhs, ok := exactRowRHSRat(p, i)
		if !ok {
			return false
		}
		actX, actD, okAct := exactEvalRowDualActivityRat(p, i, cert.X0, cert.Ray)
		if !okAct {
			return false
		}
		switch p.rowSense[i] {
		case LessEqual:
			if actX.Cmp(rhs) > 0 || actD.Sign() > 0 {
				return false
			}
		case GreaterEqual:
			if actX.Cmp(rhs) < 0 || actD.Sign() < 0 {
				return false
			}
		case Equal:
			if actX.Cmp(rhs) != 0 || actD.Sign() != 0 {
				return false
			}
		}
	}
	return true
}

// ExtractUnboundedCertificate constructs an exact rational LP or MILP unboundedness
// certificate (x0, d) for p. Callers may pass an active LPSolver s (or nil to solve
// the relaxation internally) and an optional primal/incumbent hint slice.
//
// For MILP instances, rational recession-ray components on integer-constrained columns
// are scaled by the least common multiple of their denominators so d_I lies in Z^|I|,
// and an exact integer-feasible anchor x0 is verified over Q.
func ExtractUnboundedCertificate(ctx context.Context, p *CompiledProblem, s *LPSolver, primalHint ...[]float64) (UnboundedCertificate, bool) {
	var hint []float64
	if len(primalHint) > 0 && p != nil && len(primalHint[0]) == p.numVars {
		hint = primalHint[0]
	}
	return extractUnboundedCertificateStop(exactStopFromCtx(ctx), p, s, hint)
}

func extractUnboundedCertificateStop(stop exactStop, p *CompiledProblem, s *LPSolver, hint []float64) (UnboundedCertificate, bool) {
	return extractUnboundedCertificateCore(stop, p, s, hint, nil)
}

func extractUnboundedCertificateWithRatAnchorStop(
	stop exactStop,
	p *CompiledProblem,
	s *LPSolver,
	ratAnchor []*big.Rat,
) (UnboundedCertificate, bool) {
	return extractUnboundedCertificateCore(stop, p, s, nil, ratAnchor)
}

func extractUnboundedCertificateCore(
	stop exactStop,
	p *CompiledProblem,
	s *LPSolver,
	hint []float64,
	ratAnchor []*big.Rat,
) (UnboundedCertificate, bool) {
	if stop.canceled() || p == nil || p.invalidErr != nil || p.numVars == 0 {
		return UnboundedCertificate{}, false
	}
	isMIP := p.isMIP && len(p.intCols) > 0
	if len(hint) != p.numVars {
		hint = nil
	}
	if len(ratAnchor) != p.numVars {
		ratAnchor = nil
	}

	if p.numConstraints == 0 {
		return extractFreeUnboundedCert(p, isMIP)
	}

	var x0LP, rayLP []*big.Rat
	var slopeLP *big.Rat
	if s != nil && s.prob == p && len(s.extraRows) == 0 && solverMatchesProblemBounds(s, p) {
		x0LP, rayLP, slopeLP = extractExactLPRayFromSolver(stop, p, s)
	}

	if rayLP == nil {
		if stop.canceled() {
			return UnboundedCertificate{}, false
		}
		lpProb := shallowContRelaxProblem(p)
		lp := NewLPSolver(lpProb, nil)
		stop.applyToLPSolver(lp)
		lpRes := lp.Solve()
		if lpRes.Status == StatusUnbounded {
			xCand, rCand, sCand := extractExactLPRayFromSolver(stop, lpProb, lp)
			if x0LP == nil {
				x0LP = xCand
			}
			rayLP, slopeLP = rCand, sCand
		}
		lp.Close()
	}

	if rayLP == nil {
		var okRay bool
		rayLP, slopeLP, okRay = solveHomogeneousRecessionRayLP(stop, p)
		if !okRay {
			return UnboundedCertificate{}, false
		}
	}

	if !isMIP {
		if ratAnchor != nil && verifyExactPrimalRat(p, ratAnchor, false) {
			x0LP = ratAnchor
		}
		if x0LP == nil && hint != nil {
			if xH, okH := exactPrimalIfFeasible(p, hint, false); okH {
				x0LP = xH
			}
		}
		if x0LP == nil {
			xFeas, okF := solveExactLPFeasibleAnchor(stop, p)
			if !okF {
				xFeas, okF = stepExactRayAnchor(p, hint, rayLP, false)
				if !okF && hint != nil {
					xFeas, okF = stepExactRayAnchor(p, nil, rayLP, false)
				}
			}
			if !okF {
				return UnboundedCertificate{}, false
			}
			x0LP = xFeas
		}
		cert := UnboundedCertificate{
			IsMIP:    false,
			X0:       x0LP,
			Ray:      rayLP,
			ObjSlope: slopeLP,
		}
		if !VerifyUnboundedCertificate(p, cert) {
			return UnboundedCertificate{}, false
		}
		return cert, true
	}

	rayMIP, slopeMIP, okScale := scaleRayForIntegerCols(p, rayLP, slopeLP)
	if !okScale {
		return UnboundedCertificate{}, false
	}

	var x0MIP []*big.Rat
	if ratAnchor != nil && verifyExactPrimalRat(p, ratAnchor, true) {
		x0MIP = ratAnchor
	}
	if x0MIP == nil && hint != nil {
		if xCand, okC := refineExactMIPAnchor(stop, p, hint); okC {
			x0MIP = xCand
		}
	}
	if x0MIP == nil && x0LP != nil && isRatSliceIntegralOnCols(p, x0LP) {
		x0MIP = x0LP
	}
	if x0MIP == nil {
		xCand, okC := findExactMIPFeasibleAnchorWithRay(stop, p, rayMIP)
		if !okC {
			return UnboundedCertificate{}, false
		}
		x0MIP = xCand
	}

	cert := UnboundedCertificate{
		IsMIP:    true,
		X0:       x0MIP,
		Ray:      rayMIP,
		ObjSlope: slopeMIP,
	}
	if !VerifyUnboundedCertificate(p, cert) {
		return UnboundedCertificate{}, false
	}
	return cert, true
}

func solverMatchesProblemBounds(s *LPSolver, p *CompiledProblem) bool {
	if s == nil || s.ws == nil {
		return false
	}
	n := p.numVars
	var lb, ub []float64
	if s.scaled {
		lb, ub = s.origLB, s.origUB
	} else {
		lb, ub = s.ws.lb, s.ws.ub
	}
	if len(lb) < n || len(ub) < n {
		return false
	}
	for j := range n {
		if lb[j] != p.varLB[j] || ub[j] != p.varUB[j] {
			return false
		}
	}
	return true
}

func shallowContRelaxProblem(p *CompiledProblem) *CompiledProblem {
	cp := *p
	cp.isMIP = false
	cp.intCols = nil
	cp.binaryMask = nil
	cp.lazy = nil
	cp.exactOrig = p.exactOrig.cloneForContRelax(false)
	varCat := make([]VarCategory, p.numVars)
	for j := range p.numVars {
		varCat[j] = Continuous
	}
	cp.varCat = varCat
	return &cp
}

func extractFreeUnboundedCert(p *CompiledProblem, isMIP bool) (UnboundedCertificate, bool) {
	n := p.numVars
	objMinVec, okObj := exactMinObjVecRat(p)
	if !okObj {
		return UnboundedCertificate{}, false
	}
	x0 := make([]*big.Rat, n)
	ray := make([]*big.Rat, n)
	rayCol := -1
	var rayDir int64

	for j := range n {
		ray[j] = new(big.Rat)
		hasLB, loRat, okL := exactVarLBRat(p, j)
		hasUB, hiRat, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return UnboundedCertificate{}, false
		}
		if hasLB && hasUB && loRat.Cmp(hiRat) > 0 {
			return UnboundedCertificate{}, false
		}
		var xj *big.Rat
		if isMIP && p.varCat[j] != Continuous {
			cand, ok := pickIntegerInExactBounds(hasLB, loRat, hasUB, hiRat)
			if !ok {
				return UnboundedCertificate{}, false
			}
			xj = cand
		} else {
			xj = pickRationalInExactBounds(hasLB, loRat, hasUB, hiRat)
		}
		x0[j] = xj

		if rayCol < 0 {
			if objMinVec[j].Sign() < 0 && !hasUB {
				rayCol = j
				rayDir = 1
			} else if objMinVec[j].Sign() > 0 && !hasLB {
				rayCol = j
				rayDir = -1
			}
		}
	}
	if rayCol < 0 {
		return UnboundedCertificate{}, false
	}
	ray[rayCol].SetInt64(rayDir)
	slope := new(big.Rat).Mul(objMinVec[rayCol], ray[rayCol])
	cert := UnboundedCertificate{
		IsMIP:    isMIP,
		X0:       x0,
		Ray:      ray,
		ObjSlope: slope,
	}
	if !VerifyUnboundedCertificate(p, cert) {
		return UnboundedCertificate{}, false
	}
	return cert, true
}

func pickRationalInExactBounds(hasLo bool, loRat *big.Rat, hasHi bool, hiRat *big.Rat) *big.Rat {
	if (!hasLo || loRat.Sign() <= 0) && (!hasHi || hiRat.Sign() >= 0) {
		return new(big.Rat)
	}
	if hasLo {
		return new(big.Rat).Set(loRat)
	}
	return new(big.Rat).Set(hiRat)
}

func pickIntegerInExactBounds(hasLo bool, loRat *big.Rat, hasHi bool, hiRat *big.Rat) (*big.Rat, bool) {
	if (!hasLo || loRat.Sign() <= 0) && (!hasHi || hiRat.Sign() >= 0) {
		return new(big.Rat), true
	}
	if hasLo {
		cl := new(big.Rat).SetInt(ceilExactRat(loRat))
		if hasHi && cl.Cmp(hiRat) > 0 {
			return nil, false
		}
		return cl, true
	}
	return new(big.Rat).SetInt(floorExactRat(hiRat)), true
}

func extractExactLPRayFromSolver(stop exactStop, p *CompiledProblem, s *LPSolver) (x0, ray []*big.Rat, slope *big.Rat) {
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return nil, nil, nil
	}
	if xCand, okP := snap.extractExactPrimal(stop); okP {
		x0 = xCand
	}

	// Compute exact basis multipliers yMin so candidate nonbasic edges with negative
	// directional derivative are prioritized first.
	yMin, okY := snap.solveBasisTranspose(stop, snap.basicObjMinRHS())
	if !okY {
		return x0, nil, nil
	}
	rcMin := snap.computeReducedCostsMin(yMin)

	type edgeCand struct {
		enterVar int
		dir      int64
	}
	var cands []edgeCand
	for j := range snap.n {
		if snap.status[j] == statusBasic {
			continue
		}
		if snap.ub[j] == nil && rcMin[j].Sign() < 0 {
			cands = append(cands, edgeCand{enterVar: j, dir: 1})
		}
		if snap.lb[j] == nil && rcMin[j].Sign() > 0 {
			cands = append(cands, edgeCand{enterVar: j, dir: -1})
		}
	}
	for i := range snap.m {
		slackVar := snap.n + i
		if snap.status[slackVar] == statusBasic {
			continue
		}
		// Slack variable s_i has cost 0 and column +e_i, so its reduced cost is -yMin[i].
		switch snap.rowSense[i] {
		case LessEqual:
			if yMin[i].Sign() > 0 {
				cands = append(cands, edgeCand{enterVar: slackVar, dir: 1})
			}
		case GreaterEqual:
			if yMin[i].Sign() < 0 {
				cands = append(cands, edgeCand{enterVar: slackVar, dir: -1})
			}
		}
	}

	rhsCol := make([]*big.Rat, snap.m)
	for i := range snap.m {
		rhsCol[i] = new(big.Rat)
	}

	for _, cand := range cands {
		if stop.canceled() {
			return x0, nil, nil
		}
		for i := range snap.m {
			rhsCol[i].SetInt64(0)
		}
		if cand.enterVar < snap.n {
			rows := snap.colRows[cand.enterVar]
			vals := snap.colVals[cand.enterVar]
			for k, ri := range rows {
				rhsCol[ri].Set(vals[k])
			}
		} else {
			rhsCol[cand.enterVar-snap.n].SetInt64(1)
		}

		fVec, okF := snap.solveBasisForward(stop, rhsCol)
		if !okF {
			continue
		}
		d := make([]*big.Rat, snap.n)
		for j := range snap.n {
			d[j] = new(big.Rat)
		}
		if cand.enterVar < snap.n {
			d[cand.enterVar].SetInt64(cand.dir)
		}
		for q, bVar := range snap.basis {
			if bVar < snap.n && fVec[q].Sign() != 0 {
				if cand.dir > 0 {
					d[bVar].Neg(fVec[q])
				} else {
					d[bVar].Set(fVec[q])
				}
			}
		}
		if sl, okR := verifyExactRecessionRayOnly(p, d); okR {
			return x0, d, sl
		}
	}

	return x0, nil, nil
}

// verifyExactUnboundedEdge verifies over Q whether moving enterVar in direction
// enterDir from the active basis of s produces an unblocked, strictly improving
// recession ray. When exactKnown is true, verified is authoritative over Q; when
// exactKnown is false, exact basis factorization exceeded caps or was canceled.
func verifyExactUnboundedEdge(s *LPSolver, enterVar int, enterDir float64) (verified, exactKnown bool) {
	if s == nil || s.prob == nil || s.ws == nil || enterDir == 0 || math.IsNaN(enterDir) || math.IsInf(enterDir, 0) {
		return false, false
	}
	stop := exactStop{deadline: s.Deadline, done: s.Done}
	snap, ok := buildExactSolverSnapshot(stop, s)
	if !ok {
		return false, false
	}
	tot := snap.n + snap.m
	if enterVar < 0 || enterVar >= tot || snap.status[enterVar] == statusBasic {
		return false, false
	}
	dirSign := int64(1)
	if enterDir < 0 {
		dirSign = -1
	}

	if enterVar < snap.n {
		if dirSign > 0 && snap.ub[enterVar] != nil {
			return false, true
		}
		if dirSign < 0 && snap.lb[enterVar] != nil {
			return false, true
		}
	} else {
		si := enterVar - snap.n
		switch snap.rowSense[si] {
		case LessEqual:
			if dirSign < 0 {
				return false, true
			}
		case GreaterEqual:
			if dirSign > 0 {
				return false, true
			}
		case Equal:
			return false, true
		}
	}

	rhsCol := make([]*big.Rat, snap.m)
	for i := range snap.m {
		rhsCol[i] = new(big.Rat)
	}
	if enterVar < snap.n {
		rows := snap.colRows[enterVar]
		vals := snap.colVals[enterVar]
		for k, ri := range rows {
			rhsCol[ri].Add(rhsCol[ri], vals[k])
		}
	} else {
		rhsCol[enterVar-snap.n].SetInt64(1)
	}

	fVec, okF := snap.solveBasisForward(stop, rhsCol)
	if !okF {
		return false, false
	}

	d := make([]*big.Rat, snap.n)
	for j := range snap.n {
		d[j] = new(big.Rat)
	}
	if enterVar < snap.n {
		d[enterVar].SetInt64(dirSign)
	}
	var delta big.Rat
	for q, bVar := range snap.basis {
		if fVec[q].Sign() == 0 {
			continue
		}
		if dirSign > 0 {
			delta.Neg(fVec[q])
		} else {
			delta.Set(fVec[q])
		}
		if bVar < snap.n {
			if delta.Sign() > 0 && snap.ub[bVar] != nil {
				return false, true
			}
			if delta.Sign() < 0 && snap.lb[bVar] != nil {
				return false, true
			}
			d[bVar].Set(&delta)
		} else {
			si := bVar - snap.n
			switch snap.rowSense[si] {
			case LessEqual:
				if delta.Sign() < 0 {
					return false, true
				}
			case GreaterEqual:
				if delta.Sign() > 0 {
					return false, true
				}
			case Equal:
				if delta.Sign() != 0 {
					return false, true
				}
			}
		}
	}

	var slope, prod big.Rat
	for j := range snap.n {
		if d[j].Sign() != 0 && snap.objMin[j].Sign() != 0 {
			prod.Mul(snap.objMin[j], d[j])
			slope.Add(&slope, &prod)
		}
	}
	if slope.Sign() >= 0 {
		return false, true
	}

	act := make([]*big.Rat, snap.m)
	for i := range snap.m {
		act[i] = new(big.Rat)
	}
	for j := range snap.n {
		if d[j].Sign() == 0 {
			continue
		}
		rows := snap.colRows[j]
		vals := snap.colVals[j]
		for k, ri := range rows {
			prod.Mul(vals[k], d[j])
			act[ri].Add(act[ri], &prod)
		}
	}
	for i := range snap.m {
		switch snap.rowSense[i] {
		case LessEqual:
			if act[i].Sign() > 0 {
				return false, true
			}
		case GreaterEqual:
			if act[i].Sign() < 0 {
				return false, true
			}
		case Equal:
			if act[i].Sign() != 0 {
				return false, true
			}
		}
	}
	return true, true
}

func verifyExactRecessionRayOnly(p *CompiledProblem, d []*big.Rat) (*big.Rat, bool) {
	n := p.numVars
	if len(d) != n {
		return nil, false
	}
	var objMinVec []*big.Rat
	if p.exactOrig != nil && (p.exactOrig.objHasLoss || p.exactOrig.objHasRat) {
		var okO bool
		objMinVec, okO = exactMinObjVecRat(p)
		if !okO {
			return nil, false
		}
	}
	slope := new(big.Rat)
	var prod big.Rat
	for j := range n {
		dj := d[j]
		if dj == nil {
			return nil, false
		}
		hasLB, _, okL := exactVarLBRat(p, j)
		hasUB, _, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return nil, false
		}
		if dj.Sign() > 0 && hasUB {
			return nil, false
		}
		if dj.Sign() < 0 && hasLB {
			return nil, false
		}
		if dj.Sign() != 0 {
			if objMinVec != nil {
				if objMinVec[j].Sign() != 0 {
					prod.Mul(objMinVec[j], dj)
					slope.Add(slope, &prod)
				}
			} else if p.objMin[j] != 0 {
				cj, ok := exactRatFromFloat(p.objMin[j])
				if !ok {
					return nil, false
				}
				prod.Mul(cj, dj)
				slope.Add(slope, &prod)
			}
		}
	}
	if slope.Sign() >= 0 {
		return nil, false
	}
	for i := range p.numConstraints {
		actD, okAct := exactEvalRowActivityRat(p, i, d)
		if !okAct {
			return nil, false
		}
		switch p.rowSense[i] {
		case LessEqual:
			if actD.Sign() > 0 {
				return nil, false
			}
		case GreaterEqual:
			if actD.Sign() < 0 {
				return nil, false
			}
		case Equal:
			if actD.Sign() != 0 {
				return nil, false
			}
		}
	}
	return slope, true
}

// solveHomogeneousRecessionRayLP solves the bounded homogeneous recession cone LP
// min { c_min^T d : A d diamond 0, d_j in [lo_j, hi_j] subset [-1, 1] } to recover
// an exact extreme recession ray even when the primal LP vertex is degenerate.
func solveHomogeneousRecessionRayLP(stop exactStop, p *CompiledProblem) ([]*big.Rat, *big.Rat, bool) {
	if stop.canceled() {
		return nil, nil, false
	}
	n := p.numVars
	m := p.numConstraints
	hp := *p
	hp.sense = Minimize
	hp.isMIP = false
	hp.intCols = nil
	hp.binaryMask = nil
	hp.lazy = nil
	hp.exactOrig = p.exactOrig.cloneForContRelax(p.sense == Maximize)
	if hp.exactOrig != nil {
		hp.exactOrig.objConstRat = nil
		hp.exactOrig.varLBRat = nil
		hp.exactOrig.varUBRat = nil
		for i := range hp.exactOrig.rows {
			hp.exactOrig.rows[i].rhs = 0
			hp.exactOrig.rows[i].exprConst = 0
			hp.exactOrig.rows[i].rhsRat = nil
			hp.exactOrig.rows[i].exprConstRat = nil
			hp.exactOrig.rows[i].rhsInexact = false
		}
	}
	hp.objConstant = 0
	hp.objConstMinVal = 0
	hp.objCoeffs = slices.Clone(p.objMin)
	hp.objMin = slices.Clone(p.objMin)
	hp.rowRHS = make([]float64, m)
	hp.rowLB = make([]float64, m)
	hp.rowUB = make([]float64, m)
	for i := range m {
		switch p.rowSense[i] {
		case LessEqual:
			hp.rowLB[i] = math.Inf(-1)
		case GreaterEqual:
			hp.rowUB[i] = math.Inf(1)
		}
	}
	hp.varLB = make([]float64, n)
	hp.varUB = make([]float64, n)
	hp.varCat = make([]VarCategory, n)
	for j := range n {
		hp.varCat[j] = Continuous
		if math.IsInf(p.varLB[j], -1) {
			hp.varLB[j] = -1
		} else {
			hp.varLB[j] = 0
		}
		if math.IsInf(p.varUB[j], 1) {
			hp.varUB[j] = 1
		} else {
			hp.varUB[j] = 0
		}
	}

	lp := NewLPSolver(&hp, nil)
	defer lp.Close()
	stop.applyToLPSolver(lp)
	res := lp.Solve()
	if res.Status != StatusOptimal {
		return nil, nil, false
	}
	snap, ok := buildExactSolverSnapshot(stop, lp)
	if !ok {
		return nil, nil, false
	}
	d, ok := snap.extractExactPrimal(stop)
	if !ok {
		if sol, okSol := exactPolishBasisSolutionStop(stop, lp); okSol {
			d = sol.Primal
			ok = true
		}
	}
	if !ok {
		return nil, nil, false
	}
	slope, ok := verifyExactRecessionRayOnly(p, d)
	if !ok {
		return nil, nil, false
	}
	return d, slope, true
}

func solveExactLPFeasibleAnchor(stop exactStop, p *CompiledProblem) ([]*big.Rat, bool) {
	if stop.canceled() {
		return nil, false
	}
	fp := shallowContRelaxProblem(p.zeroObjCopy())
	fp.sense = Minimize

	lp := NewLPSolver(fp, nil)
	defer lp.Close()
	stop.applyToLPSolver(lp)
	res := lp.Solve()

	maxPivots := min(defaultExactPolishMaxNucleus, max(32, p.numConstraints+16))
	if stop.maxLPIter > 0 {
		maxPivots = min(maxPivots, stop.maxLPIter)
	}
	if res.Status == StatusOptimal {
		snap, ok := buildExactSolverSnapshot(stop, lp)
		if !ok {
			return nil, false
		}
		if xExact, okP := snap.extractExactPrimal(stop); okP {
			return xExact, true
		}
		if maxPivots <= 0 {
			return nil, false
		}
		if sol, okSol := snap.solveExactDualSimplex(stop, maxPivots); okSol {
			return sol.Primal, true
		}
		if !stop.canceled() && snap.m <= exactNumericalFallbackMaxRows && snap.n <= exactNumericalFallbackMaxVars {
			snap.resetToAllSlackBasis()
			if sol, okSol := snap.solveExactDualSimplex(stop, maxPivots); okSol {
				return sol.Primal, true
			}
		}
		return nil, false
	}

	if res.Status == StatusTimeLimit || res.Status == StatusIterationLimit ||
		p.numConstraints > exactNumericalFallbackMaxRows || p.numVars > exactNumericalFallbackMaxVars || maxPivots <= 0 {
		return nil, false
	}
	snap, ok := buildExactSolverSnapshot(stop, lp)
	if !ok {
		return nil, false
	}
	snap.resetToAllSlackBasis()
	if sol, okSol := snap.solveExactDualSimplex(stop, maxPivots); okSol {
		return sol.Primal, true
	}
	return nil, false
}

func stepExactRayAnchor(p *CompiledProblem, cand []float64, ray []*big.Rat, requireInt bool) ([]*big.Rat, bool) {
	if p == nil || p.invalidErr != nil || len(ray) != p.numVars {
		return nil, false
	}
	n := p.numVars
	if len(p.varLB) < n || len(p.varUB) < n || len(p.varCat) < n {
		return nil, false
	}
	ray = ray[:n]
	for _, rj := range ray {
		if rj == nil {
			return nil, false
		}
	}
	x0 := make([]*big.Rat, n)
	for j := range n {
		hasLB, loRat, okL := exactVarLBRat(p, j)
		hasUB, hiRat, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return nil, false
		}
		if hasLB && hasUB && loRat.Cmp(hiRat) > 0 {
			return nil, false
		}
		lo, hi := p.varLB[j], p.varUB[j]
		isIntCol := requireInt && p.varCat[j] != Continuous
		var xj *big.Rat
		if len(cand) == n && !math.IsNaN(cand[j]) && !math.IsInf(cand[j], 0) {
			v := min(max(cand[j], lo), hi)
			if isIntCol {
				v = math.Round(v)
				if v < lo {
					v = math.Ceil(lo)
				}
				if v > hi {
					v = math.Floor(hi)
				}
			}
			if r, okR := exactRatFromFloat(v); okR && (!isIntCol || r.IsInt()) && (!hasLB || r.Cmp(loRat) >= 0) && (!hasUB || r.Cmp(hiRat) <= 0) {
				xj = r
			}
		}
		if xj == nil {
			if isIntCol {
				var okB bool
				xj, okB = pickIntegerInExactBounds(hasLB, loRat, hasUB, hiRat)
				if !okB {
					return nil, false
				}
			} else {
				xj = pickRationalInExactBounds(hasLB, loRat, hasUB, hiRat)
			}
		}
		x0[j] = xj
	}

	tMin := new(big.Rat)
	var diff, negAct, need big.Rat
	for i := range p.numConstraints {
		rhs, okR := exactRowRHSRat(p, i)
		actX, actD, okA := exactEvalRowDualActivityRat(p, i, x0, ray)
		if !okR || !okA {
			return nil, false
		}
		switch p.rowSense[i] {
		case LessEqual:
			if actX.Cmp(rhs) <= 0 {
				continue
			}
			if actD.Sign() >= 0 {
				return nil, false
			}
			diff.Sub(actX, rhs)
			negAct.Neg(actD)
			need.Quo(&diff, &negAct)
		case GreaterEqual:
			if actX.Cmp(rhs) >= 0 {
				continue
			}
			if actD.Sign() <= 0 {
				return nil, false
			}
			diff.Sub(rhs, actX)
			need.Quo(&diff, actD)
		case Equal:
			if actX.Cmp(rhs) != 0 {
				return nil, false
			}
			continue
		default:
			return nil, false
		}
		if need.Cmp(tMin) > 0 {
			tMin.Set(&need)
		}
	}

	if tMin.Sign() > 0 {
		if requireInt {
			needIntStep := false
			for j, rj := range ray {
				if p.varCat[j] != Continuous && rj.Sign() != 0 {
					needIntStep = true
					break
				}
			}
			if needIntStep && !tMin.IsInt() {
				var q, rem big.Int
				q.QuoRem(tMin.Num(), tMin.Denom(), &rem)
				if rem.Sign() > 0 {
					q.Add(&q, big.NewInt(1))
				}
				tMin.SetInt(&q)
			}
		}
		var step big.Rat
		for j, rj := range ray {
			if rj.Sign() != 0 {
				step.Mul(tMin, rj)
				x0[j].Add(x0[j], &step)
				if !exactRatFitsBits(x0[j]) {
					return nil, false
				}
			}
		}
	}

	if !verifyExactPrimalRat(p, x0, requireInt) {
		return nil, false
	}
	return x0, true
}

func scaleRayForIntegerCols(p *CompiledProblem, rayLP []*big.Rat, slopeLP *big.Rat) ([]*big.Rat, *big.Rat, bool) {
	one := big.NewInt(1)
	lcm := big.NewInt(1)
	var g big.Int
	for j := range p.numVars {
		if p.varCat[j] == Continuous {
			continue
		}
		den := rayLP[j].Denom()
		if den.Cmp(one) <= 0 {
			continue
		}
		g.GCD(nil, nil, lcm, den)
		lcm.Quo(lcm, &g)
		lcm.Mul(lcm, den)
		if lcm.BitLen() > defaultExactPolishMaxBits {
			return nil, nil, false
		}
	}
	if lcm.Cmp(one) == 0 {
		return cloneRatSlice(rayLP), new(big.Rat).Set(slopeLP), true
	}
	scale := new(big.Rat).SetInt(lcm)
	scaled := make([]*big.Rat, len(rayLP))
	for j, r := range rayLP {
		scaled[j] = new(big.Rat).Mul(r, scale)
		if !exactRatFitsBits(scaled[j]) {
			return nil, nil, false
		}
	}
	scaledSlope := new(big.Rat).Mul(slopeLP, scale)
	return scaled, scaledSlope, true
}

func isRatSliceIntegralOnCols(p *CompiledProblem, x []*big.Rat) bool {
	for j := range p.numVars {
		if p.varCat[j] != Continuous && !x[j].IsInt() {
			return false
		}
	}
	return true
}

func verifyExactPrimalRat(p *CompiledProblem, x []*big.Rat, requireInt bool) bool {
	if p == nil || p.invalidErr != nil || len(x) != p.numVars || len(p.varCat) < len(x) || len(p.varLB) < len(x) || len(p.varUB) < len(x) {
		return false
	}
	for j, xj := range x {
		if xj == nil {
			return false
		}
		if requireInt && p.varCat[j] != Continuous && !xj.IsInt() {
			return false
		}
		hasLB, lr, okL := exactVarLBRat(p, j)
		hasUB, ur, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return false
		}
		if hasLB && hasUB && lr.Cmp(ur) > 0 {
			return false
		}
		if hasLB && xj.Cmp(lr) < 0 {
			return false
		}
		if hasUB && xj.Cmp(ur) > 0 {
			return false
		}
	}
	for i := range p.numConstraints {
		rhs, ok := exactRowRHSRat(p, i)
		if !ok {
			return false
		}
		act, okAct := exactEvalRowActivityRat(p, i, x)
		if !okAct {
			return false
		}
		cmp := act.Cmp(rhs)
		switch p.rowSense[i] {
		case LessEqual:
			if cmp > 0 {
				return false
			}
		case GreaterEqual:
			if cmp < 0 {
				return false
			}
		case Equal:
			if cmp != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func floatSliceToExactPrimal(p *CompiledProblem, cand []float64, requireInt, roundInt bool) ([]*big.Rat, bool) {
	if p == nil || p.invalidErr != nil || len(cand) != p.numVars || len(p.varCat) < p.numVars {
		return nil, false
	}
	x := make([]*big.Rat, p.numVars)
	for j := range p.numVars {
		v := cand[j]
		if requireInt && roundInt && p.varCat[j] != Continuous {
			v = math.Round(v)
		}
		r, ok := exactRatFromFloat(v)
		if !ok {
			return nil, false
		}
		if requireInt && p.varCat[j] != Continuous && !r.IsInt() {
			return nil, false
		}
		x[j] = r
	}
	if !verifyExactPrimalRat(p, x, requireInt) {
		return nil, false
	}
	return x, true
}

func exactPrimalIfFeasible(p *CompiledProblem, cand []float64, requireInt bool) ([]*big.Rat, bool) {
	return floatSliceToExactPrimal(p, cand, requireInt, true)
}

func refineExactMIPAnchor(stop exactStop, p *CompiledProblem, cand []float64) ([]*big.Rat, bool) {
	if stop.canceled() || len(cand) != p.numVars {
		return nil, false
	}
	if xDirect, ok := exactPrimalIfFeasible(p, cand, true); ok {
		return xDirect, true
	}
	if len(p.intCols) == p.numVars {
		return nil, false
	}

	// Fix integer columns to rounded integers on a copy of p and solve continuous
	// columns via exact basis factorization so non-dyadic rationals (such as 1/3)
	// satisfy equality rows with zero residual over Q.
	fp := shallowContRelaxProblem(p.zeroObjCopy())
	fp.sense = Minimize
	if fp.exactOrig != nil && p.exactOrig != nil {
		if fp.exactOrig.varLBRat != nil {
			fp.exactOrig.varLBRat = slices.Clone(fp.exactOrig.varLBRat)
		}
		if fp.exactOrig.varUBRat != nil {
			fp.exactOrig.varUBRat = slices.Clone(fp.exactOrig.varUBRat)
		}
		if p.exactOrig.hasIntTightened {
			fp.rowRHS = slices.Clone(p.rowRHS)
			fp.rowLB = slices.Clone(p.rowLB)
			fp.rowUB = slices.Clone(p.rowUB)
		}
		for i := range fp.exactOrig.rows {
			if i < len(p.exactOrig.rows) && p.exactOrig.rows[i].intTightened {
				tight := p.exactOrig.rows[i].intTightRHS
				fp.exactOrig.rows[i].rhsInexact = false
				if p.exactOrig.rows[i].intTightRat != nil {
					fp.exactOrig.rows[i].rhsRat = p.exactOrig.rows[i].intTightRat
					fp.exactOrig.rows[i].exprConstRat = nil
				}
				fp.rowRHS[i] = tight
				switch fp.rowSense[i] {
				case LessEqual:
					fp.rowLB[i] = math.Inf(-1)
					fp.rowUB[i] = tight
				case GreaterEqual:
					fp.rowLB[i] = tight
					fp.rowUB[i] = math.Inf(1)
				case Equal:
					fp.rowLB[i] = tight
					fp.rowUB[i] = tight
				}
			}
		}
	}
	fp.varLB = slices.Clone(p.varLB)
	fp.varUB = slices.Clone(p.varUB)
	for j := range p.numVars {
		if p.varCat[j] != Continuous {
			iv := math.Round(cand[j])
			if math.IsNaN(iv) || math.IsInf(iv, 0) {
				return nil, false
			}
			rInt, okI := exactRatFromFloat(iv)
			if !okI || !rInt.IsInt() {
				return nil, false
			}
			hasLB, loRat, okL := exactVarLBRat(p, j)
			hasUB, hiRat, okU := exactVarUBRat(p, j)
			if !okL || !okU {
				return nil, false
			}
			if (hasLB && rInt.Cmp(loRat) < 0) || (hasUB && rInt.Cmp(hiRat) > 0) {
				return nil, false
			}
			fp.varLB[j] = iv
			fp.varUB[j] = iv
			if fp.exactOrig != nil {
				if j < len(fp.exactOrig.varLBRat) {
					fp.exactOrig.varLBRat[j] = rInt
				}
				if j < len(fp.exactOrig.varUBRat) {
					fp.exactOrig.varUBRat[j] = rInt
				}
			}
		}
	}

	lp := NewLPSolver(fp, nil)
	defer lp.Close()
	stop.applyToLPSolver(lp)
	res := lp.Solve()
	if res.Status != StatusOptimal {
		return nil, false
	}
	snap, ok := buildExactSolverSnapshot(stop, lp)
	if !ok {
		return nil, false
	}
	xExact, ok := snap.extractExactPrimal(stop)
	if !ok || !isRatSliceIntegralOnCols(p, xExact) || !verifyExactPrimalRat(p, xExact, true) {
		if sol, okSol := exactPolishBasisSolutionStop(stop, lp); okSol {
			xExact = sol.Primal
			ok = true
		}
	}
	if !ok || !isRatSliceIntegralOnCols(p, xExact) || !verifyExactPrimalRat(p, xExact, true) {
		return nil, false
	}
	return xExact, true
}

func semiBoundedFeasTermCoeff(lo, hi float64) float64 {
	var sign, bnd float64
	switch {
	case !math.IsInf(lo, -1) && math.IsInf(hi, 1):
		sign = 1.0
		bnd = math.Abs(lo)
	case math.IsInf(lo, -1) && !math.IsInf(hi, 1):
		sign = -1.0
		bnd = math.Abs(hi)
	default:
		return 0
	}
	if bnd <= 1e6 {
		return sign
	}
	_, exp := math.Frexp(bnd)
	return math.Ldexp(sign, -exp)
}

// semiBoundedFeasObjCopy clones p with a Minimize objective that penalizes
// semi-bounded variables moving toward their infinite bound while preserving
// row and variable dimensions for VIPR feasibility subtraces.
func (p *CompiledProblem) semiBoundedFeasObjCopy() *CompiledProblem {
	cp := *p
	cp.lazy = nil
	cp.sense = Minimize
	cp.objCoeffs = make([]float64, p.numVars)
	for j := range p.numVars {
		cp.objCoeffs[j] = semiBoundedFeasTermCoeff(p.varLB[j], p.varUB[j])
	}
	cp.objMin = slices.Clone(cp.objCoeffs)
	cp.objConstant = 0
	cp.objConstMinVal = 0
	cp.exactOrig = p.exactOrig.cloneZeroObj()
	return &cp
}

// boundedFeasAnchorProblem constructs a bounded-below feasibility MIP over the
// exact-tightened problem, adding L1 auxiliary bounds t_k >= |x_j| for any
// doubly-unbounded integer variables so branch-and-bound never dives along zero-cost rays.
func boundedFeasAnchorProblem(p *CompiledProblem) (*CompiledProblem, bool) {
	if p == nil || p.invalidErr != nil {
		return nil, false
	}
	tp := p.exactTightenedProblem()
	n := tp.numVars
	var freeCols []int
	for j := range n {
		lo, hi := tp.varLB[j], tp.varUB[j]
		if math.IsNaN(lo) || math.IsNaN(hi) || lo > hi {
			return nil, false
		}
		if tp.varCat[j] != Continuous && math.IsInf(lo, -1) && math.IsInf(hi, 1) {
			freeCols = append(freeCols, j)
		}
	}
	cp := tp.semiBoundedFeasObjCopy()
	if len(freeCols) == 0 {
		return cp, true
	}

	kFree := len(freeCols)
	newN := n + kFree
	m := tp.numConstraints
	newM := m + 2*kFree

	cp.varNames = append(slices.Clone(tp.varNames), make([]string, kFree)...)
	cp.varLB = append(slices.Clone(tp.varLB), make([]float64, kFree)...)
	cp.varUB = append(slices.Clone(tp.varUB), make([]float64, kFree)...)
	cp.varCat = append(slices.Clone(tp.varCat), make([]VarCategory, kFree)...)
	cp.objCoeffs = append(cp.objCoeffs, make([]float64, kFree)...)
	cp.objMin = append(cp.objMin, make([]float64, kFree)...)
	needWords := (newN + bitsPerWord - 1) / bitsPerWord
	mask := make([]uint64, needWords)
	copy(mask, tp.binaryMask)
	cp.binaryMask = mask

	cp.rowNames = append(slices.Clone(tp.rowNames), make([]string, 2*kFree)...)
	cp.rowSense = append(slices.Clone(tp.rowSense), make([]ConstraintSense, 2*kFree)...)
	cp.rowRHS = append(slices.Clone(tp.rowRHS), make([]float64, 2*kFree)...)
	cp.rowLB = append(slices.Clone(tp.rowLB), make([]float64, 2*kFree)...)
	cp.rowUB = append(slices.Clone(tp.rowUB), make([]float64, 2*kFree)...)

	cp.rowPtr = make([]int, newM+1)
	copy(cp.rowPtr, tp.rowPtr[:m+1])
	baseNNZ := len(tp.colIdx)
	cp.colIdx = make([]int, baseNNZ, baseNNZ+4*kFree)
	copy(cp.colIdx, tp.colIdx)
	cp.rowVal = make([]float64, baseNNZ, baseNNZ+4*kFree)
	copy(cp.rowVal, tp.rowVal)

	for k, origCol := range freeCols {
		auxCol := n + k
		idxStr := strconv.Itoa(k)
		cp.varNames[auxCol] = "_feas_abs_" + idxStr
		cp.varLB[auxCol] = 0
		cp.varUB[auxCol] = math.Inf(1)
		cp.varCat[auxCol] = Continuous
		cp.objCoeffs[auxCol] = 1.0
		cp.objMin[auxCol] = 1.0

		posRow := m + 2*k
		cp.rowNames[posRow] = "_feas_pos_" + idxStr
		cp.rowSense[posRow] = LessEqual
		cp.rowLB[posRow] = math.Inf(-1)
		cp.colIdx = append(cp.colIdx, origCol, auxCol)
		cp.rowVal = append(cp.rowVal, 1.0, -1.0)
		cp.rowPtr[posRow+1] = len(cp.colIdx)

		negRow := posRow + 1
		cp.rowNames[negRow] = "_feas_neg_" + idxStr
		cp.rowSense[negRow] = LessEqual
		cp.rowLB[negRow] = math.Inf(-1)
		cp.colIdx = append(cp.colIdx, origCol, auxCol)
		cp.rowVal = append(cp.rowVal, -1.0, -1.0)
		cp.rowPtr[negRow+1] = len(cp.colIdx)
	}

	if cp.exactOrig != nil && len(cp.exactOrig.rows) > 0 {
		cp.exactOrig.rows = append(slices.Clone(cp.exactOrig.rows), make([]exactModelRow, 2*kFree)...)
	}
	cp.numVars = newN
	cp.numConstraints = newM
	cp.denseRows = nil
	if newN > 0 && newM > 0 {
		cp.rowStride = sparseNoDenseStride
	} else {
		cp.rowStride = 0
	}
	buildCSCFromCSR(cp)
	return cp, true
}

func findExactMIPFeasibleAnchor(stop exactStop, p *CompiledProblem) ([]*big.Rat, bool) {
	return findExactMIPFeasibleAnchorWithRay(stop, p, nil)
}

func findExactMIPFeasibleAnchorWithRay(stop exactStop, p *CompiledProblem, rayMIP []*big.Rat) ([]*big.Rat, bool) {
	if stop.canceled() || hasExactGCDInfeasibleEqRow(p) {
		return nil, false
	}
	feasProb, okProb := boundedFeasAnchorProblem(p)
	if !okProb {
		return nil, false
	}

	opt := DefaultOptions()
	opt.EnablePresolve = false
	opt.Workers = 1
	opt.AbsGapTol = math.Inf(1)
	if stop.maxMIPNodes > 0 {
		opt.MaxNodes = stop.maxMIPNodes
	}
	if stop.maxLPIter > 0 {
		opt.MaxIter = stop.maxLPIter
	}

	feasRes := solveBranchAndCutWithDone(feasProb, opt, time.Now(), stop.deadline, stop.done)
	totNodes := feasRes.Nodes
	totIters := feasRes.Iterations
	if !stop.canceled() && feasRes.Status == StatusNumericalError && feasProb.numVars > p.numVars {
		fbOpt := opt
		canFallback := true
		if stop.maxLPIter > 0 {
			fbOpt.MaxIter -= totIters
			canFallback = fbOpt.MaxIter > 0
		}
		if canFallback && stop.maxMIPNodes > 0 {
			fbOpt.MaxNodes -= totNodes
			canFallback = fbOpt.MaxNodes > 0
		}
		if canFallback {
			feasRes = solveBranchAndCutWithDone(p.semiBoundedFeasObjCopy(), fbOpt, time.Now(), stop.deadline, stop.done)
			totNodes += feasRes.Nodes
			totIters += feasRes.Iterations
		}
	}
	if stop.canceled() || feasRes.Status != StatusOptimal || len(feasRes.Values) < p.numVars {
		return nil, false
	}
	cand := feasRes.Values[:p.numVars]
	if xExact, okC := refineExactMIPAnchor(stop, p, cand); okC {
		return xExact, true
	}
	if len(rayMIP) == p.numVars {
		if xRay, okR := stepExactRayAnchor(p, cand, rayMIP, true); okR {
			return xRay, true
		}
	}
	if p.numConstraints <= defaultExactPolishMaxNucleus {
		remIter := 0
		if stop.maxLPIter > 0 {
			remIter = stop.maxLPIter - totIters
			if remIter <= 0 {
				return nil, false
			}
		}
		remNodes := 256
		if stop.maxMIPNodes > 0 {
			remNodes = min(256, stop.maxMIPNodes-totNodes)
			if remNodes <= 0 {
				return nil, false
			}
		}
		if xExact, stExact, _, _ := solveExactMIPFeasibilityStop(stop, p, remNodes, remIter); stExact == StatusOptimal {
			return xExact, true
		}
	}
	return nil, false
}

type exactFeasNode struct {
	parent  *exactFeasNode
	col     int
	isUpper bool
	bound   *big.Rat
	lbObjF  float64
	depth   int
	fracCnt int
	seq     int
}

func exactFeasNodeLess(a, b *exactFeasNode) bool {
	if a.lbObjF != b.lbObjF {
		return a.lbObjF < b.lbObjF
	}
	if a.fracCnt != b.fracCnt {
		return a.fracCnt < b.fracCnt
	}
	if a.depth != b.depth {
		return a.depth < b.depth
	}
	return a.seq < b.seq
}

func exactFeasHeapPush(h []*exactFeasNode, node *exactFeasNode) []*exactFeasNode {
	h = append(h, node)
	i := len(h) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !exactFeasNodeLess(h[i], h[p]) {
			break
		}
		h[i], h[p] = h[p], h[i]
		i = p
	}
	return h
}

func exactFeasHeapPop(h []*exactFeasNode) (*exactFeasNode, []*exactFeasNode) {
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h[last] = nil
	h = h[:last]
	n := len(h)
	i := 0
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		best := left
		if right := left + 1; right < n && exactFeasNodeLess(h[right], h[left]) {
			best = right
		}
		if !exactFeasNodeLess(h[best], h[i]) {
			break
		}
		h[i], h[best] = h[best], h[i]
		i = best
	}
	return top, h
}

func materializeExactFeasBounds(
	rootLB, rootUB []*big.Rat,
	node *exactFeasNode,
	curLB, curUB []*big.Rat,
	floatLB, floatUB []float64,
) bool {
	copy(curLB, rootLB)
	copy(curUB, rootUB)
	for cur := node; cur != nil && cur.col >= 0; cur = cur.parent {
		c := cur.col
		if cur.isUpper {
			if curUB[c] == nil || cur.bound.Cmp(curUB[c]) < 0 {
				curUB[c] = cur.bound
			}
		} else if curLB[c] == nil || cur.bound.Cmp(curLB[c]) > 0 {
			curLB[c] = cur.bound
		}
		if curLB[c] != nil && curUB[c] != nil && curLB[c].Cmp(curUB[c]) > 0 {
			return false
		}
	}
	for j := range curLB {
		if curLB[j] == nil {
			floatLB[j] = math.Inf(-1)
		} else {
			floatLB[j] = exactRatToFloatFloor(curLB[j])
		}
		if curUB[j] == nil {
			floatUB[j] = math.Inf(1)
		} else {
			floatUB[j] = exactRatToFloatCeil(curUB[j])
		}
	}
	return true
}

func floorExactRat(r *big.Rat) *big.Int {
	fl := new(big.Int)
	rem := new(big.Int)
	fl.QuoRem(r.Num(), r.Denom(), rem)
	if rem.Sign() < 0 {
		fl.Sub(fl, big.NewInt(1))
	}
	return fl
}

func ceilExactRat(r *big.Rat) *big.Int {
	cl := new(big.Int)
	rem := new(big.Int)
	cl.QuoRem(r.Num(), r.Denom(), rem)
	if rem.Sign() > 0 {
		cl.Add(cl, big.NewInt(1))
	}
	return cl
}

func pickExactFractionalIntCol(intCols []int, x []*big.Rat) (brCol int, flInt *big.Int, fracCnt int) {
	brCol = -1
	bestScore := math.Inf(1)
	var diff big.Rat
	for _, col := range intCols {
		if uint(col) >= uint(len(x)) || x[col] == nil || x[col].IsInt() {
			continue
		}
		fracCnt++
		fl := floorExactRat(x[col])
		diff.SetInt(fl)
		diff.Sub(x[col], &diff)
		fracF, _ := diff.Float64()
		score := math.Abs(fracF - 0.5)
		if brCol < 0 || score < bestScore {
			brCol = col
			flInt = fl
			bestScore = score
		}
	}
	return brCol, flInt, fracCnt
}

func solveExactFeasNodeLP(
	stop exactStop,
	lp *LPSolver,
	workProb *CompiledProblem,
	defaultLPIter, maxIter int,
	usedIters *int,
	curLB, curUB []*big.Rat,
	floatLB, floatUB []float64,
) (ExactLPSolution, Status) {
	var lpRes LPResult
	budgetHit := false
	for attempt := range 2 {
		if attempt == 1 {
			lp.InitCrashBasis()
		}
		if maxIter > 0 {
			rem := maxIter - *usedIters
			if rem <= 0 {
				return ExactLPSolution{}, StatusIterationLimit
			}
			lp.MaxIter = min(defaultLPIter, rem)
		}
		lp.SetStructuralBounds(floatLB, floatUB)
		lpRes = lp.Solve()
		*usedIters += lpRes.Iterations
		if stop.canceled() || lpRes.Status == StatusTimeLimit {
			return ExactLPSolution{}, StatusTimeLimit
		}
		budgetHit = maxIter > 0 && lp.MaxIter < defaultLPIter
		if lpRes.Status == StatusOptimal || lpRes.Status == StatusInfeasible || (lpRes.Status == StatusIterationLimit && budgetHit) {
			break
		}
	}
	if lpRes.Status == StatusIterationLimit && budgetHit {
		return ExactLPSolution{}, StatusIterationLimit
	}

	snap, okS := buildExactSolverSnapshot(stop, lp)
	if !okS {
		lp.InitCrashBasis()
		lp.SetStructuralBounds(floatLB, floatUB)
		snap, okS = buildExactSolverSnapshot(stop, lp)
		if !okS {
			if stop.canceled() {
				return ExactLPSolution{}, StatusTimeLimit
			}
			return ExactLPSolution{}, StatusNumericalError
		}
	}
	copy(snap.lb, curLB)
	copy(snap.ub, curUB)

	maxPivots := min(defaultExactPolishMaxNucleus, max(64, 4*workProb.numConstraints+16))
	sol, stExact, _, _, _, _, okExact := snap.solveExactDualSimplexOutcome(stop, maxPivots)
	if !okExact || (stExact != StatusOptimal && stExact != StatusInfeasible) {
		snap.resetToAllSlackBasis()
		sol, stExact, _, _, _, _, okExact = snap.solveExactDualSimplexOutcome(stop, maxPivots)
	}
	if stop.canceled() {
		return ExactLPSolution{}, StatusTimeLimit
	}
	if !okExact || (stExact != StatusOptimal && stExact != StatusInfeasible) {
		return ExactLPSolution{}, StatusNumericalError
	}
	return sol, stExact
}

// solveExactMIPFeasibilityStop runs bounded rational branch-and-bound over Q
// to decide integer feasibility on small problems with sub-ULP tolerances.
func solveExactMIPFeasibilityStop(
	stop exactStop,
	p *CompiledProblem,
	maxNodes, maxIter int,
) (anchor []*big.Rat, st Status, usedNodes, usedIters int) {
	if stop.canceled() {
		return nil, StatusTimeLimit, 0, 0
	}
	if hasExactGCDInfeasibleEqRow(p) {
		return nil, StatusInfeasible, 0, 0
	}
	workProb, okProb := boundedFeasAnchorProblem(p)
	if !okProb {
		return nil, StatusNumericalError, 0, 0
	}
	wn := workProb.numVars
	rootLB := make([]*big.Rat, wn)
	rootUB := make([]*big.Rat, wn)
	for j := range wn {
		hasLB, lbR, okL := exactVarLBRat(workProb, j)
		hasUB, ubR, okU := exactVarUBRat(workProb, j)
		if !okL || !okU {
			return nil, StatusNumericalError, 0, 0
		}
		if hasLB {
			rootLB[j] = lbR
		}
		if hasUB {
			rootUB[j] = ubR
		}
		if rootLB[j] != nil && rootUB[j] != nil && rootLB[j].Cmp(rootUB[j]) > 0 {
			return nil, StatusInfeasible, 0, 0
		}
	}

	lp := NewLPSolver(workProb, nil)
	defer lp.Close()
	defaultLPIter := lp.MaxIter
	stop.applyToLPSolver(lp)

	curLB := make([]*big.Rat, wn)
	curUB := make([]*big.Rat, wn)
	floatLB := make([]float64, wn)
	floatUB := make([]float64, wn)

	seq := 0
	pq := []*exactFeasNode{{col: -1, seq: seq}}
	one := big.NewInt(1)
	for len(pq) > 0 {
		if stop.canceled() {
			return nil, StatusTimeLimit, usedNodes, usedIters
		}
		if maxNodes > 0 && usedNodes >= maxNodes {
			return nil, StatusNodeLimit, usedNodes, usedIters
		}
		if maxIter > 0 && usedNodes > 0 && usedIters >= maxIter {
			return nil, StatusIterationLimit, usedNodes, usedIters
		}
		var node *exactFeasNode
		node, pq = exactFeasHeapPop(pq)
		if !materializeExactFeasBounds(rootLB, rootUB, node, curLB, curUB, floatLB, floatUB) {
			continue
		}
		usedNodes++
		sol, nodeSt := solveExactFeasNodeLP(stop, lp, workProb, defaultLPIter, maxIter, &usedIters, curLB, curUB, floatLB, floatUB)
		switch nodeSt {
		case StatusInfeasible:
			continue
		case StatusOptimal:
		default:
			return nil, nodeSt, usedNodes, usedIters
		}
		if len(sol.Primal) < p.numVars {
			return nil, StatusNumericalError, usedNodes, usedIters
		}
		brCol, flInt, fracCnt := pickExactFractionalIntCol(workProb.intCols, sol.Primal)
		if brCol < 0 {
			cand := sol.Primal[:p.numVars]
			if verifyExactPrimalRat(p, cand, true) {
				return cand, StatusOptimal, usedNodes, usedIters
			}
			return nil, StatusNumericalError, usedNodes, usedIters
		}
		objF := 0.0
		if sol.ObjectiveMin != nil {
			objF, _ = sol.ObjectiveMin.Float64()
		}
		flRat := new(big.Rat).SetInt(flInt)
		clInt := new(big.Int).Add(flInt, one)
		clRat := new(big.Rat).SetInt(clInt)
		if curLB[brCol] == nil || flRat.Cmp(curLB[brCol]) >= 0 {
			seq++
			pq = exactFeasHeapPush(pq, &exactFeasNode{
				parent:  node,
				col:     brCol,
				isUpper: true,
				bound:   flRat,
				lbObjF:  objF,
				depth:   node.depth + 1,
				fracCnt: fracCnt,
				seq:     seq,
			})
		}
		if curUB[brCol] == nil || clRat.Cmp(curUB[brCol]) <= 0 {
			seq++
			pq = exactFeasHeapPush(pq, &exactFeasNode{
				parent:  node,
				col:     brCol,
				isUpper: false,
				bound:   clRat,
				lbObjF:  objF,
				depth:   node.depth + 1,
				fracCnt: fracCnt,
				seq:     seq,
			})
		}
	}
	return nil, StatusInfeasible, usedNodes, usedIters
}

// exactMIPFeasibleRat reports whether x satisfies every variable bound, integrality
// requirement, and linear constraint of p in exact rational arithmetic.
func exactMIPFeasibleRat(p *CompiledProblem, x []float64) bool {
	if p == nil || len(x) < p.numVars {
		return false
	}
	_, ok := floatSliceToExactPrimal(p, x[:p.numVars], true, false)
	return ok
}

type exactGCDScratch struct {
	one       big.Int
	g         big.Int
	lcmDen    big.Int
	tmpGCD    big.Int
	scaledNum big.Int
}

func (sc *exactGCDScratch) rowScale(vals []*big.Rat, maxDenomBits int) (*big.Rat, bool) {
	sc.one.SetInt64(1)
	sc.lcmDen.SetInt64(1)
	for _, vr := range vals {
		if vr.Sign() == 0 {
			continue
		}
		den := vr.Denom()
		if den.Cmp(&sc.one) > 0 {
			sc.tmpGCD.GCD(nil, nil, &sc.lcmDen, den)
			sc.lcmDen.Quo(&sc.lcmDen, &sc.tmpGCD)
			sc.lcmDen.Mul(&sc.lcmDen, den)
			if sc.lcmDen.BitLen() > maxDenomBits {
				return nil, false
			}
		}
	}
	sc.g.SetInt64(0)
	for _, vr := range vals {
		if vr.Sign() == 0 {
			continue
		}
		sc.scaledNum.Quo(&sc.lcmDen, vr.Denom())
		sc.scaledNum.Mul(&sc.scaledNum, vr.Num())
		sc.scaledNum.Abs(&sc.scaledNum)
		if sc.scaledNum.Sign() > 0 {
			if sc.g.Sign() == 0 {
				sc.g.Set(&sc.scaledNum)
			} else {
				sc.g.GCD(nil, nil, &sc.g, &sc.scaledNum)
			}
		}
	}
	if sc.g.Sign() <= 0 {
		return nil, false
	}
	return new(big.Rat).SetFrac(new(big.Int).Set(&sc.lcmDen), new(big.Int).Set(&sc.g)), true
}

func hasExactGCDInfeasibleEqRow(p *CompiledProblem) bool {
	const maxRatDenomBits = 2048
	if p == nil || !p.isMIP {
		return false
	}
	if p.exactOrig != nil && p.exactOrig.intEqInfeas {
		return true
	}
	if _, ok := exactObjConstMinRat(p); !ok {
		return false
	}
	if p.exactOrig != nil {
		if _, ok := exactMinObjVecRat(p); !ok {
			return false
		}
	} else {
		for j := range p.numVars {
			if _, ok := exactRatFromFloat(p.objMin[j]); !ok {
				return false
			}
		}
	}
	for j := range p.numVars {
		if _, _, okL := exactVarLBRat(p, j); !okL {
			return false
		}
		if _, _, okU := exactVarUBRat(p, j); !okU {
			return false
		}
	}
	var gcdSc exactGCDScratch
	for i := range p.numConstraints {
		rhsRat, ok := exactRowRHSRat(p, i)
		if !ok {
			return false
		}
		cols, vals, okC := exactCanonicalRowTermsRat(p, i)
		if !okC {
			return false
		}
		if p.rowSense[i] != Equal {
			continue
		}
		if len(cols) == 0 {
			if rhsRat.Sign() != 0 {
				return true
			}
			continue
		}
		allInt := true
		for _, col := range cols {
			if p.varCat[col] == Continuous {
				allInt = false
				break
			}
		}
		if !allInt {
			continue
		}
		scaleMult, okScale := gcdSc.rowScale(vals, maxRatDenomBits)
		if !okScale {
			continue
		}
		rhsScaled := new(big.Rat).Mul(rhsRat, scaleMult)
		if !rhsScaled.IsInt() {
			return true
		}
	}
	return false
}
