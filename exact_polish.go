package goblin

import (
	"context"
	"math"
	"math/big"
	"slices"
	"time"
)

const (
	defaultExactPolishMaxNucleus = 512
	defaultExactPolishMaxBits    = 4096
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

func exactMinObjVecRat(p *CompiledProblem) ([]*big.Rat, bool) {
	if p == nil {
		return nil, false
	}
	n := p.numVars
	out := make([]*big.Rat, n)
	if eo := p.exactOrig; eo != nil && eo.objHasLoss {
		for j := range n {
			out[j] = new(big.Rat)
		}
		for _, t := range eo.objTerms {
			if t.col < 0 || t.col >= n {
				return nil, false
			}
			rv, ok := exactRatFromFloat(t.coeff)
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
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && eo.rows[i].rhsInexact {
		er := eo.rows[i]
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
	return exactRatFromFloat(p.rowRHS[i])
}

func exactCanonicalRowTermsRat(p *CompiledProblem, i int) ([]int, []*big.Rat, bool) {
	if p == nil || i < 0 || i >= p.numConstraints {
		return nil, nil, false
	}
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && eo.rows[i].hasDupLoss {
		er := eo.rows[i]
		acc := make(map[int]*big.Rat, er.termEnd-er.termStart)
		cols := make([]int, 0, er.termEnd-er.termStart)
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= p.numVars {
				return nil, nil, false
			}
			rv, ok := exactRatFromFloat(t.coeff)
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
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && eo.rows[i].hasDupLoss {
		er := eo.rows[i]
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= len(x) {
				return nil, false
			}
			if t.coeff == 0 || x[t.col].Sign() == 0 {
				continue
			}
			av, ok := exactRatFromFloat(t.coeff)
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
	if eo := p.exactOrig; eo != nil && i < len(eo.rows) && eo.rows[i].hasDupLoss {
		er := eo.rows[i]
		for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
			if t.col < 0 || t.col >= len(x0) || t.col >= len(d) {
				return nil, nil, false
			}
			if t.coeff == 0 {
				continue
			}
			av, okA := exactRatFromFloat(t.coeff)
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

	objConstM, ok := exactRatFromFloat(p.objConstMinVal)
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
			lr, okL := exactRatFromFloat(loF)
			if !okL {
				return nil, false
			}
			snap.lb[j] = lr
		}
		if !math.IsInf(hiF, 1) {
			ur, okU := exactRatFromFloat(hiF)
			if !okU {
				return nil, false
			}
			snap.ub[j] = ur
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
	objConst, ok := exactRatFromFloat(p.objConstMinVal)
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
	if p.exactOrig != nil && p.exactOrig.objHasLoss {
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
		lo, hi := p.varLB[j], p.varUB[j]
		if math.IsNaN(lo) || math.IsNaN(hi) || lo > hi {
			return false
		}
		if !math.IsInf(lo, -1) {
			loRat, ok := exactRatFromFloat(lo)
			if !ok || x0j.Cmp(loRat) < 0 {
				return false
			}
			if dj.Sign() < 0 {
				return false
			}
		}
		if !math.IsInf(hi, 1) {
			hiRat, ok := exactRatFromFloat(hi)
			if !ok || x0j.Cmp(hiRat) > 0 {
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
	if stop.canceled() || p == nil || p.invalidErr != nil || p.numVars == 0 {
		return UnboundedCertificate{}, false
	}
	isMIP := p.isMIP && len(p.intCols) > 0
	if len(hint) != p.numVars {
		hint = nil
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
		if x0LP == nil {
			if hint != nil {
				if xH, okH := exactPrimalIfFeasible(p, hint, false); okH {
					x0LP = xH
				}
			}
		}
		if x0LP == nil {
			xFeas, okF := solveExactLPFeasibleAnchor(stop, p)
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
	if hint != nil {
		if xCand, okC := refineExactMIPAnchor(stop, p, hint); okC {
			x0MIP = xCand
		}
	}
	if x0MIP == nil && x0LP != nil && isRatSliceIntegralOnCols(p, x0LP) {
		x0MIP = x0LP
	}
	if x0MIP == nil {
		xCand, okC := findExactMIPFeasibleAnchor(stop, p)
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
		lo, hi := p.varLB[j], p.varUB[j]
		if math.IsNaN(lo) || math.IsNaN(hi) || lo > hi {
			return UnboundedCertificate{}, false
		}
		var xj *big.Rat
		if isMIP && p.varCat[j] != Continuous {
			cand, ok := pickIntegerInBounds(lo, hi)
			if !ok {
				return UnboundedCertificate{}, false
			}
			xj = cand
		} else {
			cand, ok := pickRationalInBounds(lo, hi)
			if !ok {
				return UnboundedCertificate{}, false
			}
			xj = cand
		}
		x0[j] = xj

		if rayCol < 0 {
			if objMinVec[j].Sign() < 0 && math.IsInf(hi, 1) {
				rayCol = j
				rayDir = 1
			} else if objMinVec[j].Sign() > 0 && math.IsInf(lo, -1) {
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

func pickRationalInBounds(lo, hi float64) (*big.Rat, bool) {
	hasLo := !math.IsInf(lo, -1)
	hasHi := !math.IsInf(hi, 1)
	if (!hasLo || lo <= 0) && (!hasHi || 0 <= hi) {
		return new(big.Rat), true
	}
	if hasLo {
		return exactRatFromFloat(lo)
	}
	return exactRatFromFloat(hi)
}

func pickIntegerInBounds(lo, hi float64) (*big.Rat, bool) {
	hasLo := !math.IsInf(lo, -1)
	hasHi := !math.IsInf(hi, 1)
	if (!hasLo || lo <= 0) && (!hasHi || 0 <= hi) {
		return new(big.Rat), true
	}
	if hasLo {
		cl := math.Ceil(lo)
		if hasHi && cl > hi {
			return nil, false
		}
		r, ok := exactRatFromFloat(cl)
		if !ok || !r.IsInt() {
			return nil, false
		}
		return r, true
	}
	fl := math.Floor(hi)
	r, ok := exactRatFromFloat(fl)
	if !ok || !r.IsInt() {
		return nil, false
	}
	return r, true
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
	if p.exactOrig != nil && p.exactOrig.objHasLoss {
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
		if dj.Sign() > 0 && !math.IsInf(p.varUB[j], 1) {
			return nil, false
		}
		if dj.Sign() < 0 && !math.IsInf(p.varLB[j], -1) {
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
	hp.lazy = nil
	hp.exactOrig = p.exactOrig.cloneForContRelax(p.sense == Maximize)
	if hp.exactOrig != nil {
		for i := range hp.exactOrig.rows {
			hp.exactOrig.rows[i].rhs = 0
			hp.exactOrig.rows[i].exprConst = 0
			hp.exactOrig.rows[i].rhsInexact = false
		}
	}
	hp.objConstant = 0
	hp.objConstMinVal = 0
	hp.objCoeffs = slices.Clone(p.objMin)
	hp.objMin = slices.Clone(p.objMin)
	hp.rowRHS = make([]float64, m)
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
	if res.Status != StatusOptimal {
		return nil, false
	}
	snap, ok := buildExactSolverSnapshot(stop, lp)
	if !ok {
		return nil, false
	}
	return snap.extractExactPrimal(stop)
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
		lo, hi := p.varLB[j], p.varUB[j]
		if math.IsNaN(lo) || math.IsNaN(hi) || lo > hi {
			return false
		}
		if !math.IsInf(lo, -1) {
			lr, ok := exactRatFromFloat(lo)
			if !ok || xj.Cmp(lr) < 0 {
				return false
			}
		}
		if !math.IsInf(hi, 1) {
			ur, ok := exactRatFromFloat(hi)
			if !ok || xj.Cmp(ur) > 0 {
				return false
			}
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
		for i := range fp.exactOrig.rows {
			if i < len(p.exactOrig.rows) && p.exactOrig.rows[i].intTightened {
				fp.exactOrig.rows[i].rhsInexact = false
			}
		}
	}
	fp.varLB = slices.Clone(p.varLB)
	fp.varUB = slices.Clone(p.varUB)
	for j := range p.numVars {
		if p.varCat[j] != Continuous {
			iv := math.Round(cand[j])
			if math.IsNaN(iv) || math.IsInf(iv, 0) || iv < p.varLB[j] || iv > p.varUB[j] {
				return nil, false
			}
			rInt, okI := exactRatFromFloat(iv)
			if !okI || !rInt.IsInt() {
				return nil, false
			}
			fp.varLB[j] = iv
			fp.varUB[j] = iv
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
		return nil, false
	}
	return xExact, true
}

func findExactMIPFeasibleAnchor(stop exactStop, p *CompiledProblem) ([]*big.Rat, bool) {
	if stop.canceled() {
		return nil, false
	}
	feasProb := p.zeroObjCopy()
	feasProb.sense = Minimize

	opt := DefaultOptions()
	opt.EnablePresolve = false
	opt.Workers = 1

	feasRes := solveBranchAndCutWithDone(feasProb, opt, time.Now(), stop.deadline, stop.done)
	if stop.canceled() || feasRes.Status != StatusOptimal || len(feasRes.Values) != p.numVars {
		return nil, false
	}
	return refineExactMIPAnchor(stop, p, feasRes.Values)
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
	if _, ok := exactRatFromFloat(p.objConstMin()); !ok {
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
		if !math.IsInf(p.varLB[j], -1) {
			if _, ok := exactRatFromFloat(p.varLB[j]); !ok {
				return false
			}
		}
		if !math.IsInf(p.varUB[j], 1) {
			if _, ok := exactRatFromFloat(p.varUB[j]); !ok {
				return false
			}
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
