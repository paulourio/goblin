package goblin

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/paulourio/goblin/internal/simd"
)

const (
	primalTol = 1e-7
	dualTol   = 1e-7
	pivotTol  = 1e-8
	harrisTol = 1e-9
	// minScaledPrimalTol floors the column-normalized primal tolerance on unscaled models.
	minScaledPrimalTol = 1e-12
	minRuizPrimalTol   = 1e-15
	defaultMinMaxIter  = 2000
	defaultIterMult    = 20
	minStaysFeasPivot  = 1e-3
	pivotTieRelTol     = 1e-11
	minDualPivotElem   = 1e-10
	pivAgreeRelTol     = 1e-4
	minDSEWeight       = 1e-4
	maxDSEWeight       = 1e12
	minDSEColEntry     = 1e-14
	phase1RelaxTol     = 1e-5
	verifyFeasTol      = 1e-5
	minBFRTFlipPivot   = 1e-5
	primalRatioTieTol  = 1e-12
	primalSnapTol      = 1e-6
	maxDenseCutVars    = 2048
	maxBFRTFlips       = 64
	scalePivotVarMin   = 100
	ruizScalePasses    = 4
	ruizScaleMaxExp    = 26
	ruizObjMaxExp      = 20
	scaleMinMatNorm    = 1e-4
	scaleMaxMatNorm    = 1e6
	scaleMinObjNorm    = 1e-5
	scaleMaxObjNorm    = 1e6
	blandMinTrigger    = 512
	blandRowMult       = 4
	blandBurstMask     = 63
	blandBurstWindow   = 16
	degenStepTol       = 1e-11
)

// LPResult holds the result of a continuous LP solve.
type LPResult struct {
	Status       Status
	ObjectiveMin float64
	Objective    float64
	Primal       []float64
	Dual         []float64
	ReducedCosts []float64
	Iterations   int
}

type dynamicRow struct {
	ColIdx   []int
	Val      []float64
	OrigVal  []float64
	Dense    []float64
	RHS      float64
	OrigRHS  float64
	rowScale float64
}

// LPSolver encapsulates a Revised Dual & Primal Simplex state that can be solved
// from scratch and repeatedly warm-started with modified bounds or added cutting planes.
type LPSolver struct {
	prob        *CompiledProblem
	mat         *CompiledProblem
	ws          *Workspace
	extraRows   []dynamicRow
	MaxIter     int
	Deadline    time.Time
	Done        <-chan struct{}
	DisableSIMD bool
	ownsWS      bool
	scaled      bool
	luValid     bool
	dualValid   bool
	// colTol holds the primal bound tolerance of each structural column (see feasTol).
	colTol []float64
	// slackTol holds the scaled primal tolerance of each slack variable when scaled is true.
	slackTol []float64
	// varTol caches feasTol(k) across all tot = n + m variables for branch-free dual pricing.
	varTol []float64
	// colHasEq reports whether structural column j has a nonzero in any equation row.
	colHasEq    []bool
	rowScale    []float64
	colScale    []float64
	invColScale []float64
	origLB      []float64
	origUB      []float64
	objScale    float64
	effDualTol  float64
}

func (s *LPSolver) axpy64(dst, src []float64, a float64) {
	if s.DisableSIMD {
		simd.Axpy64Scalar(dst, src, a)
	} else {
		simd.Axpy64(dst, src, a)
	}
}

// feasTol returns the primal bound tolerance of variable k. A value error e in a
// structural x_k perturbs some row activity by up to e*max_i|a_ik|, so structural columns are
// measured in column-normalized units: primalTol / max(1, max_i|a_ik|), floored at
// minScaledPrimalTol. Without this, a basic big-M indicator resting 5e-8 outside its bound
// (inside an absolute 1e-7 tolerance) shifts its row by 0.05 once the value is reported.
// Slack columns are unit columns and use primalTol.
func (s *LPSolver) feasTol(k int) float64 {
	if k < len(s.colTol) {
		return s.colTol[k]
	}
	if s.scaled {
		r := k - len(s.colTol)
		if uint(r) < uint(len(s.slackTol)) {
			return s.slackTol[r]
		}
	}
	return primalTol
}

func (s *LPSolver) syncVarTol(tot int) []float64 {
	if len(s.varTol) == tot {
		return s.varTol
	}
	if cap(s.varTol) < tot {
		s.varTol = make([]float64, tot)
	} else {
		s.varTol = s.varTol[:tot]
	}
	n := copy(s.varTol, s.colTol)
	if s.scaled {
		n += copy(s.varTol[n:], s.slackTol)
	}
	for k := n; k < tot; k++ {
		s.varTol[k] = primalTol
	}
	return s.varTol
}

func (s *LPSolver) rowScaleOf(r int) float64 {
	if !s.scaled {
		return 1.0
	}
	if r < len(s.rowScale) {
		return s.rowScale[r]
	}
	e := r - len(s.rowScale)
	if uint(e) < uint(len(s.extraRows)) && s.extraRows[e].rowScale > 0 {
		return s.extraRows[e].rowScale
	}
	return 1.0
}

func (s *LPSolver) unscaledBasicVal(r int) float64 {
	v := s.ws.xb[r]
	if !s.scaled || v == 0 {
		return v
	}
	bVar := s.ws.basis[r]
	if bVar < len(s.colScale) {
		return v * s.colScale[bVar]
	}
	return v / s.rowScaleOf(bVar-len(s.colScale))
}

func (s *LPSolver) unscaledTabEntry(bVar, k int) float64 {
	alpha := s.ws.tabRow[k]
	if !s.scaled || alpha == 0 {
		return alpha
	}
	cb := s.colScale[bVar]
	if k < len(s.colScale) {
		return alpha * cb * s.invColScale[k]
	}
	return alpha * cb * s.rowScaleOf(k-len(s.colScale))
}

func (s *LPSolver) unscaledBound(k int, upper bool) float64 {
	if !s.scaled {
		if upper {
			return s.ws.ub[k]
		}
		return s.ws.lb[k]
	}
	if k < len(s.origLB) {
		if upper {
			return s.origUB[k]
		}
		return s.origLB[k]
	}
	if upper {
		return s.ws.ub[k] / s.rowScaleOf(k-len(s.origLB))
	}
	return s.ws.lb[k] / s.rowScaleOf(k-len(s.origLB))
}

func (s *LPSolver) unscaledRowRHS(rowIdx int) float64 {
	if rowIdx < s.prob.numConstraints {
		return s.prob.rowRHS[rowIdx]
	}
	dr := s.extraRows[rowIdx-s.prob.numConstraints]
	if s.scaled {
		return dr.OrigRHS
	}
	return dr.RHS
}

func pow2EquilibrateExp(v float64, maxExp int) int {
	if !(v > 0) || math.IsInf(v, 0) {
		return 0
	}
	_, e := math.Frexp(v)
	return min(max(-(e-1), -maxExp), maxExp)
}

func buildScaledLP(p *CompiledProblem) (mat *CompiledProblem, rowScale, colScale, invColScale, colTol, slackTol []float64, colHasEq []bool, objScale float64, scaled bool) {
	n := p.numVars
	m := p.numConstraints
	colTol = make([]float64, n)
	colHasEq = make([]bool, n)

	minColNorm := math.Inf(1)
	maxColNorm := 0.0
	minAbsElem := math.Inf(1)
	for j := range n {
		cn := 0.0
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			av := math.Abs(p.colVal[k])
			cn = max(cn, av)
			if av > 0 && av < minAbsElem {
				minAbsElem = av
			}
			if p.rowSense[p.rowIdx[k]] == Equal {
				colHasEq[j] = true
			}
		}
		if cn > 0 {
			minColNorm = min(minColNorm, cn)
			maxColNorm = max(maxColNorm, cn)
		}
		colTol[j] = max(primalTol/max(1.0, cn), minScaledPrimalTol)
	}

	minRowNorm := math.Inf(1)
	maxRowNorm := 0.0
	for i := range m {
		rn := 0.0
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			rn = max(rn, math.Abs(p.rowVal[k]))
		}
		if rn > 0 {
			minRowNorm = min(minRowNorm, rn)
			maxRowNorm = max(maxRowNorm, rn)
		}
	}

	maxObj := 0.0
	minPosObj := math.Inf(1)
	for j := range n {
		c := math.Abs(p.objMin[j])
		if c > maxObj {
			maxObj = c
		}
		if c > 0 && c < minPosObj {
			minPosObj = c
		}
	}

	needMat := m > 0 && n > 0 && maxRowNorm > 0 &&
		(minRowNorm < scaleMinMatNorm || maxRowNorm > scaleMaxMatNorm ||
			minColNorm < scaleMinMatNorm || maxColNorm > scaleMaxMatNorm ||
			minAbsElem < scaleMinMatNorm || maxColNorm > scaleMaxMatNorm*minAbsElem)
	needObj := m > 0 && maxObj > 0 &&
		(maxObj < scaleMinObjNorm || maxObj > scaleMaxObjNorm ||
			minPosObj < scaleMinObjNorm || maxObj > scaleMaxObjNorm*minPosObj)
	if !needMat && !needObj {
		return p, nil, nil, nil, colTol, nil, colHasEq, 1.0, false
	}

	origColNorm := make([]float64, n)
	for j := range n {
		cn := 0.0
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			cn = max(cn, math.Abs(p.colVal[k]))
		}
		origColNorm[j] = max(1.0, cn)
	}

	rowExp := make([]int, m)
	colExp := make([]int, n)
	if needMat {
		for pass := range ruizScalePasses {
			finalPass := pass == ruizScalePasses-1
			for i := range m {
				rmax := 0.0
				for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
					v := math.Ldexp(math.Abs(p.rowVal[k]), rowExp[i]+colExp[p.colIdx[k]])
					if v > rmax {
						rmax = v
					}
				}
				if rmax > 0 {
					_, e := math.Frexp(rmax)
					delta := -(e - 1) / 2
					if finalPass && (rmax < 0.25 || rmax > 4.0) {
						delta = -(e - 1)
					}
					rowExp[i] = min(max(rowExp[i]+delta, -ruizScaleMaxExp), ruizScaleMaxExp)
				}
			}
			for j := range n {
				cmax := 0.0
				for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
					v := math.Ldexp(math.Abs(p.colVal[k]), rowExp[p.rowIdx[k]]+colExp[j])
					if v > cmax {
						cmax = v
					}
				}
				if cmax > 0 {
					_, e := math.Frexp(cmax)
					delta := -(e - 1) / 2
					if finalPass && (cmax < 0.25 || cmax > 4.0) {
						delta = -(e - 1)
					}
					colExp[j] = min(max(colExp[j]+delta, -ruizScaleMaxExp), ruizScaleMaxExp)
				}
			}
		}
	}

	rowScale = make([]float64, m)
	slackTol = make([]float64, m)
	for i := range m {
		rs := math.Ldexp(1.0, rowExp[i])
		rowScale[i] = rs
		slackTol[i] = max(primalTol*min(1.0, rs), minRuizPrimalTol)
	}
	colScale = make([]float64, n)
	invColScale = make([]float64, n)
	maxScaledObj := 0.0
	for j := range n {
		cs := math.Ldexp(1.0, colExp[j])
		colScale[j] = cs
		invColScale[j] = math.Ldexp(1.0, -colExp[j])
		colTol[j] = max(primalTol/(cs*origColNorm[j]), minRuizPrimalTol)
		if sc := math.Abs(p.objMin[j]) * cs; sc > maxScaledObj {
			maxScaledObj = sc
		}
	}

	objScale = 1.0
	if maxScaledObj > 0 && (maxScaledObj < 1e-3 || maxScaledObj > 1e3) {
		_, oe := math.Frexp(maxScaledObj)
		objExp := min(max(oe-1, -ruizScaleMaxExp), ruizObjMaxExp)
		objScale = math.Ldexp(1.0, objExp)
	}
	invObjScale := 1.0 / objScale

	cp := *p
	cp.varLB = make([]float64, n)
	cp.varUB = make([]float64, n)
	cp.objMin = make([]float64, n)
	for j := range n {
		cp.varLB[j] = p.varLB[j] * invColScale[j]
		cp.varUB[j] = p.varUB[j] * invColScale[j]
		if p.objMin[j] != 0 {
			cp.objMin[j] = p.objMin[j] * colScale[j] * invObjScale
		}
	}
	cp.rowRHS = make([]float64, m)
	cp.rowLB = make([]float64, m)
	cp.rowUB = make([]float64, m)
	cp.rowVal = make([]float64, len(p.rowVal))
	if len(p.denseRows) > 0 {
		cp.denseRows = make([]float64, len(p.denseRows))
	}
	for i := range m {
		rs := rowScale[i]
		cp.rowRHS[i] = p.rowRHS[i] * rs
		cp.rowLB[i] = p.rowLB[i] * rs
		cp.rowUB[i] = p.rowUB[i] * rs
		var denseRow []float64
		if len(cp.denseRows) > 0 {
			denseRow = cp.denseRows[i*cp.rowStride : (i+1)*cp.rowStride]
		}
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			sv := p.rowVal[k] * rs * colScale[col]
			cp.rowVal[k] = sv
			if denseRow != nil {
				denseRow[col] = sv
			}
		}
	}
	cp.colVal = make([]float64, len(p.colVal))
	for j := range n {
		cs := colScale[j]
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			cp.colVal[k] = p.colVal[k] * rowScale[p.rowIdx[k]] * cs
		}
	}
	return &cp, rowScale, colScale, invColScale, colTol, slackTol, colHasEq, objScale, true
}

func (s *LPSolver) timeExpired() bool {
	return stopRequested(s.Deadline, s.Done)
}

// NewLPSolver creates an LPSolver initialized for the given CompiledProblem.
func NewLPSolver(p *CompiledProblem, ws *Workspace) *LPSolver {
	if p == nil {
		return &LPSolver{}
	}
	ownsWS := false
	if ws == nil {
		ws = AcquireWorkspace(p.numVars, p.numConstraints)
		ownsWS = true
	} else {
		ws.ensureSize(p.numVars, p.numConstraints)
	}
	mat, rowScale, colScale, invColScale, colTol, slackTol, colHasEq, objScale, scaled := buildScaledLP(p)
	effDual := dualTol
	if scaled {
		effDual = max(dualTol/max(1.0, objScale), 1e-13)
	}
	s := &LPSolver{
		prob:        p,
		mat:         mat,
		ws:          ws,
		MaxIter:     max(defaultMinMaxIter, defaultIterMult*(p.numVars+p.numConstraints)),
		ownsWS:      ownsWS,
		scaled:      scaled,
		colTol:      colTol,
		slackTol:    slackTol,
		colHasEq:    colHasEq,
		rowScale:    rowScale,
		colScale:    colScale,
		invColScale: invColScale,
		objScale:    objScale,
		effDualTol:  effDual,
	}
	if scaled {
		s.origLB = slices.Clone(p.varLB)
		s.origUB = slices.Clone(p.varUB)
	}
	s.InitCrashBasis()
	return s
}

// Close releases any internally pooled Workspace owned by s.
func (s *LPSolver) Close() {
	if s != nil && s.ownsWS && s.ws != nil {
		ReleaseWorkspace(s.ws)
		s.ws = nil
		s.ownsWS = false
	}
}

// TotalRows returns the total number of rows (structural constraints plus active dynamic cuts).
func (s *LPSolver) TotalRows() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numConstraints + len(s.extraRows)
}

// NumVars returns the number of structural columns in the LP.
func (s *LPSolver) NumVars() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numVars
}

// NumCuts returns the number of active cutting-plane rows appended to the LP.
func (s *LPSolver) NumCuts() int {
	if s == nil {
		return 0
	}
	return len(s.extraRows)
}

// InitCrashBasis resets the solver to an all-slack basis with nonbasic structural variables placed at cost-favorable bounds.
func (s *LPSolver) InitCrashBasis() {
	if s == nil || s.prob == nil || s.ws == nil {
		return
	}
	p := s.prob
	mp := s.mat
	n := p.numVars
	m := s.TotalRows()
	ws := s.ws
	ws.ensureSize(n, m)
	if s.scaled {
		copy(s.origLB, p.varLB)
		copy(s.origUB, p.varUB)
	}

	for j := range n {
		ws.lb[j] = mp.varLB[j]
		ws.ub[j] = mp.varUB[j]
		ws.cost[j] = mp.objMin[j]
		ws.basicRow[j] = -1
		ws.status[j] = initialNonBasicStatus(ws.lb[j], ws.ub[j], ws.cost[j])
		ws.x[j] = nonBasicValue(ws.status[j], ws.lb[j], ws.ub[j])
	}

	for i := range m {
		slackVar := n + i
		ws.cost[slackVar] = 0
		if i < p.numConstraints {
			switch p.rowSense[i] {
			case LessEqual:
				ws.lb[slackVar] = 0
				ws.ub[slackVar] = math.Inf(1)
			case GreaterEqual:
				ws.lb[slackVar] = math.Inf(-1)
				ws.ub[slackVar] = 0
			case Equal:
				ws.lb[slackVar] = 0
				ws.ub[slackVar] = 0
			}
		} else {
			ws.lb[slackVar] = 0
			ws.ub[slackVar] = math.Inf(1)
		}
		ws.basis[i] = slackVar
		ws.basicRow[slackVar] = i
		ws.status[slackVar] = statusBasic
		ws.dse[i] = 1.0
	}
	s.luValid = false
	s.dualValid = false
}

// CopyRootBasisFrom copies the root basis, bounds, duals, reduced costs, and steepest-edge weights from src when neither solver holds extra cut rows.
func (s *LPSolver) CopyRootBasisFrom(src *LPSolver) {
	if s == nil || src == nil || s.prob == nil || src.prob == nil || s.ws == nil || src.ws == nil {
		return
	}
	if len(src.extraRows) > 0 || len(s.extraRows) > 0 || s.scaled != src.scaled {
		return
	}
	n := s.prob.numVars
	m := s.prob.numConstraints
	ws := s.ws
	sws := src.ws
	ws.ensureSize(n, m)
	copy(ws.cost[:n+m], sws.cost[:n+m])
	copy(ws.basis[:m], sws.basis[:m])
	copy(ws.basicRow[:n+m], sws.basicRow[:n+m])
	copy(ws.status[:n+m], sws.status[:n+m])
	copy(ws.x[:n+m], sws.x[:n+m])
	copy(ws.lb[:n+m], sws.lb[:n+m])
	copy(ws.ub[:n+m], sws.ub[:n+m])
	copy(ws.dse[:m], sws.dse[:m])
	copy(ws.y[:m], sws.y[:m])
	copy(ws.dj[:n+m], sws.dj[:n+m])
	if s.scaled {
		copy(s.origLB, src.origLB)
		copy(s.origUB, src.origUB)
	}
	s.luValid = false
	s.dualValid = true
}

func initialNonBasicStatus(lb, ub, cost float64) varBasisStatus {
	if lb == ub {
		return statusFixed
	}
	hasLB := !math.IsInf(lb, -1)
	hasUB := !math.IsInf(ub, 1)
	if hasLB && hasUB {
		if cost < 0 {
			return statusAtUpper
		}
		return statusAtLower
	}
	if hasLB {
		return statusAtLower
	}
	if hasUB {
		return statusAtUpper
	}
	return statusSuperBasic
}

func nonBasicValue(st varBasisStatus, lb, ub float64) float64 {
	switch st {
	case statusAtLower, statusFixed:
		return lb
	case statusAtUpper:
		return ub
	default:
		return 0
	}
}

// SetStructuralBounds updates the working bounds of structural columns in place while preserving the current basis for warm-started re-solves.
func (s *LPSolver) SetStructuralBounds(lb, ub []float64) {
	if s == nil || s.prob == nil || s.ws == nil {
		return
	}
	ws := s.ws
	n := s.prob.numVars
	if len(lb) < n || len(ub) < n {
		return
	}
	if s.scaled {
		copy(s.origLB, lb[:n])
		copy(s.origUB, ub[:n])
	}
	for j := range n {
		lbj, ubj := lb[j], ub[j]
		if s.scaled {
			inv := s.invColScale[j]
			lbj *= inv
			ubj *= inv
		}
		ws.lb[j] = lbj
		ws.ub[j] = ubj
		st := ws.status[j]
		if st == statusBasic {
			continue
		}
		hasLB := !math.IsInf(lbj, -1)
		hasUB := !math.IsInf(ubj, 1)
		if lbj == ubj {
			ws.status[j] = statusFixed
			ws.x[j] = lbj
		} else if hasLB && hasUB {
			if st == statusAtUpper || (st == statusFixed && ws.dj[j] < 0) {
				ws.status[j] = statusAtUpper
				ws.x[j] = ubj
			} else {
				ws.status[j] = statusAtLower
				ws.x[j] = lbj
			}
		} else if hasLB {
			ws.status[j] = statusAtLower
			ws.x[j] = lbj
		} else if hasUB {
			ws.status[j] = statusAtUpper
			ws.x[j] = ubj
		} else {
			ws.status[j] = statusSuperBasic
			ws.x[j] = 0
		}
	}
}

func (s *LPSolver) newCutRow(colIdx []int, val []float64, rhs float64) dynamicRow {
	p := s.prob
	n := p.numVars
	kLim := min(len(colIdx), len(val))

	// Check whether colIdx is already strictly ascending with valid indices and above-threshold values.
	canonical := true
	prev := -1
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c <= prev || c >= n || v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			canonical = false
			break
		}
		prev = c
	}

	var cols []int
	var vals []float64
	if canonical {
		cols = slices.Clone(colIdx[:kLim])
		vals = slices.Clone(val[:kLim])
	} else {
		type termPair struct {
			col int
			val float64
		}
		pairs := make([]termPair, 0, kLim)
		for k := range kLim {
			c := colIdx[k]
			v := val[k]
			if c < 0 || c >= n || v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			pairs = append(pairs, termPair{col: c, val: v})
		}
		slices.SortFunc(pairs, func(a, b termPair) int { return cmp.Compare(a.col, b.col) })
		cols = make([]int, 0, len(pairs))
		vals = make([]float64, 0, len(pairs))
		for i := 0; i < len(pairs); {
			c := pairs[i].col
			sum := pairs[i].val
			for i++; i < len(pairs) && pairs[i].col == c; i++ {
				sum += pairs[i].val
			}
			if sum == 0 {
				continue
			}
			cols = append(cols, c)
			vals = append(vals, sum)
		}
	}

	origRHS := rhs
	var origVal []float64
	rowScale := 1.0
	if s.scaled {
		origVal = slices.Clone(vals)
		maxAbs := 0.0
		for k, c := range cols {
			sv := vals[k] * s.colScale[c]
			vals[k] = sv
			if av := math.Abs(sv); av > maxAbs {
				maxAbs = av
			}
		}
		if maxAbs > 0 {
			rExp := pow2EquilibrateExp(maxAbs, ruizScaleMaxExp)
			rowScale = math.Ldexp(1.0, rExp)
			for k := range vals {
				vals[k] *= rowScale
			}
			rhs *= rowScale
		}
	}

	dr := dynamicRow{
		ColIdx:   cols,
		Val:      vals,
		OrigVal:  origVal,
		RHS:      rhs,
		OrigRHS:  origRHS,
		rowScale: rowScale,
	}
	if n > 0 && n <= maxDenseCutVars {
		dr.Dense = make([]float64, max(n, p.rowStride))
		for k, col := range cols {
			dr.Dense[col] = vals[k]
		}
	}
	return dr
}

// AddCut appends the cutting plane sum_k val[k]*x[colIdx[k]] <= rhs to the continuous LP.
// The new row's slack enters the basis, so the current basis stays primal-degenerate-feasible
// for all existing rows and the next Solve warm-starts with the dual simplex.
func (s *LPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	if s == nil || s.prob == nil || s.ws == nil {
		return
	}
	s.addCutRows([]dynamicRow{s.newCutRow(colIdx, val, rhs)})
}

// addCutRows appends a batch of cut rows with a single workspace resize. Dual steepest-edge
// weights of existing rows are preserved: appending rows whose slacks are basic leaves the
// existing rows of B^{-1} unchanged, so their weights ||e_r^T B^{-1}||^2 remain exact.
func (s *LPSolver) addCutRows(rows []dynamicRow) {
	if len(rows) == 0 {
		return
	}
	mp := s.mat
	n := mp.numVars
	s.extraRows = append(s.extraRows, rows...)
	s.varTol = s.varTol[:0]
	if s.scaled {
		for _, dr := range rows {
			rs := dr.rowScale
			if rs <= 0 {
				rs = 1.0
			}
			s.slackTol = append(s.slackTol, max(primalTol*min(1.0, rs), minRuizPrimalTol))
		}
	}

	ws := s.ws
	oldM := ws.m
	newM := oldM + len(rows)

	savedBasis := slices.Clone(ws.basis[:oldM])
	savedStatus := slices.Clone(ws.status[:n+oldM])
	savedLB := slices.Clone(ws.lb[:n])
	savedUB := slices.Clone(ws.ub[:n])
	savedDSE := slices.Clone(ws.dse[:oldM])

	ws.ensureSize(n, newM)
	for j := range n {
		ws.lb[j] = savedLB[j]
		ws.ub[j] = savedUB[j]
		ws.cost[j] = mp.objMin[j]
		ws.basicRow[j] = -1
		ws.status[j] = savedStatus[j]
	}
	for i := range oldM {
		slackOld := n + i
		ws.cost[slackOld] = 0
		ws.lb[slackOld], ws.ub[slackOld] = s.slackBounds(i)
		ws.basicRow[slackOld] = -1
		ws.status[slackOld] = savedStatus[slackOld]
	}

	for i := range oldM {
		bVar := savedBasis[i]
		ws.basis[i] = bVar
		ws.basicRow[bVar] = i
		ws.status[bVar] = statusBasic
		ws.dse[i] = savedDSE[i]
	}
	for i := oldM; i < newM; i++ {
		newSlack := n + i
		ws.cost[newSlack] = 0
		ws.lb[newSlack] = 0
		ws.ub[newSlack] = math.Inf(1)
		ws.status[newSlack] = statusBasic
		ws.basis[i] = newSlack
		ws.basicRow[newSlack] = i
		ws.dse[i] = 1.0
	}

	for k := range n + newM {
		if ws.status[k] != statusBasic {
			ws.x[k] = nonBasicValue(ws.status[k], ws.lb[k], ws.ub[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

// lpSnapshot captures the basis and bound state of an LPSolver so that a batch of
// cutting planes can be rolled back if the re-solve fails.
type lpSnapshot struct {
	numExtra int
	basis    []int
	status   []varBasisStatus
	lb, ub   []float64
	origLB   []float64
	origUB   []float64
	xb       []float64
	dse      []float64
}

func (s *LPSolver) snapshot() lpSnapshot {
	ws := s.ws
	n := s.prob.numVars
	m := s.TotalRows()
	snap := lpSnapshot{
		numExtra: len(s.extraRows),
		basis:    slices.Clone(ws.basis[:m]),
		status:   slices.Clone(ws.status[:n+m]),
		lb:       slices.Clone(ws.lb[:n]),
		ub:       slices.Clone(ws.ub[:n]),
		xb:       slices.Clone(ws.xb[:m]),
		dse:      slices.Clone(ws.dse[:m]),
	}
	if s.scaled {
		snap.origLB = slices.Clone(s.origLB)
		snap.origUB = slices.Clone(s.origUB)
	}
	return snap
}

// restore rolls the solver back to snap, discarding any cut rows added after it was taken.
func (s *LPSolver) restore(snap lpSnapshot) {
	mp := s.mat
	n := mp.numVars
	baseM := mp.numConstraints
	s.extraRows = s.extraRows[:snap.numExtra]
	s.varTol = s.varTol[:0]
	if s.scaled {
		if len(snap.origLB) == n {
			copy(s.origLB, snap.origLB)
			copy(s.origUB, snap.origUB)
		}
		s.slackTol = s.slackTol[:baseM]
		for _, dr := range s.extraRows {
			rs := dr.rowScale
			if rs <= 0 {
				rs = 1.0
			}
			s.slackTol = append(s.slackTol, max(primalTol*min(1.0, rs), minRuizPrimalTol))
		}
	}
	m := s.TotalRows()
	ws := s.ws
	ws.ensureSize(n, m)
	for j := range n {
		ws.lb[j] = snap.lb[j]
		ws.ub[j] = snap.ub[j]
		ws.cost[j] = mp.objMin[j]
		ws.basicRow[j] = -1
	}
	for i := range m {
		slack := n + i
		ws.cost[slack] = 0
		ws.lb[slack], ws.ub[slack] = s.slackBounds(i)
		ws.basicRow[slack] = -1
	}
	copy(ws.status[:n+m], snap.status)
	copy(ws.xb[:m], snap.xb)
	for i := range m {
		bVar := snap.basis[i]
		ws.basis[i] = bVar
		ws.basicRow[bVar] = i
		ws.dse[i] = snap.dse[i]
		ws.x[bVar] = snap.xb[i]
	}
	for k := range n + m {
		if ws.status[k] != statusBasic {
			ws.x[k] = nonBasicValue(ws.status[k], ws.lb[k], ws.ub[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

// PurgeSlackCuts drops dynamic cut rows whose slack variable is basic with primal value
// above slackTol, compacting ExtraRows and basis arrays in place while preserving primal
// feasibility, dual optimality, and steepest-edge weights.
func (s *LPSolver) PurgeSlackCuts(slackTol float64) int {
	if len(s.extraRows) == 0 {
		return 0
	}
	p := s.prob
	n := p.numVars
	baseM := p.numConstraints
	oldExtra := len(s.extraRows)
	oldM := baseM + oldExtra
	ws := s.ws

	dropExtra := make([]bool, oldExtra)
	dropped := 0
	for e := range oldExtra {
		slackVar := n + baseM + e
		if ws.status[slackVar] == statusBasic {
			bSlot := ws.basicRow[slackVar]
			thresh := slackTol * s.rowScaleOf(baseM+e)
			if bSlot >= 0 && bSlot < oldM && ws.basis[bSlot] == slackVar && ws.xb[bSlot] > thresh {
				dropExtra[e] = true
				dropped++
			}
		}
	}
	if dropped == 0 {
		return 0
	}

	rowMap := make([]int, oldM)
	for i := range baseM {
		rowMap[i] = i
	}
	newExtra := 0
	for e := range oldExtra {
		if dropExtra[e] {
			rowMap[baseM+e] = -1
			continue
		}
		if newExtra != e {
			s.extraRows[newExtra] = s.extraRows[e]
		}
		rowMap[baseM+e] = baseM + newExtra
		newExtra++
	}
	clear(s.extraRows[newExtra:])
	s.extraRows = s.extraRows[:newExtra]
	newM := baseM + newExtra

	newStatus := make([]varBasisStatus, n+newM)
	copy(newStatus[:n+baseM], ws.status[:n+baseM])
	for e := range oldExtra {
		if mappedRow := rowMap[baseM+e]; mappedRow >= 0 {
			newStatus[n+mappedRow] = ws.status[n+baseM+e]
		}
	}

	newBasis := make([]int, 0, newM)
	newXB := make([]float64, 0, newM)
	newDSE := make([]float64, 0, newM)
	for slot := range oldM {
		bVar := ws.basis[slot]
		if bVar >= n {
			mappedRow := rowMap[bVar-n]
			if mappedRow < 0 {
				continue
			}
			bVar = n + mappedRow
		}
		newBasis = append(newBasis, bVar)
		newXB = append(newXB, ws.xb[slot])
		newDSE = append(newDSE, ws.dse[slot])
	}

	s.restore(lpSnapshot{
		numExtra: newExtra,
		basis:    newBasis,
		status:   newStatus,
		lb:       slices.Clone(ws.lb[:n]),
		ub:       slices.Clone(ws.ub[:n]),
		xb:       newXB,
		dse:      newDSE,
	})
	return dropped
}

// slackBounds returns the bounds of the slack of row i (a_i x + s_i = rhs_i).
func (s *LPSolver) slackBounds(i int) (lb, ub float64) {
	if i >= s.prob.numConstraints {
		return 0, math.Inf(1)
	}
	switch s.prob.rowSense[i] {
	case GreaterEqual:
		return math.Inf(-1), 0
	case Equal:
		return 0, 0
	default:
		return 0, math.Inf(1)
	}
}

func (s *LPSolver) fillColumn(colVar int, dst []float64) {
	p := s.mat
	n := p.numVars
	if colVar >= n {
		row := colVar - n
		dst[row] = 1.0
		return
	}
	for k := p.colPtr[colVar]; k < p.colPtr[colVar+1]; k++ {
		dst[p.rowIdx[k]] = p.colVal[k]
	}
	baseM := p.numConstraints
	for r, dr := range s.extraRows {
		if dr.Dense != nil {
			dst[baseM+r] = dr.Dense[colVar]
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					dst[baseM+r] = dr.Val[k]
					break
				}
			}
		}
	}
}

func (s *LPSolver) fillColumnSparse(colVar int, dst []float64, idx []int) []int {
	p := s.mat
	n := p.numVars
	idx = idx[:0]
	if colVar >= n {
		row := colVar - n
		dst[row] = 1.0
		return append(idx, row)
	}
	for k := p.colPtr[colVar]; k < p.colPtr[colVar+1]; k++ {
		r := p.rowIdx[k]
		dst[r] = p.colVal[k]
		idx = append(idx, r)
	}
	baseM := p.numConstraints
	for r, dr := range s.extraRows {
		var v float64
		if dr.Dense != nil {
			v = dr.Dense[colVar]
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					v = dr.Val[k]
					break
				}
			}
		}
		if v != 0 {
			dst[baseM+r] = v
			idx = append(idx, baseM+r)
		}
	}
	return idx
}

func (s *LPSolver) addScaledColumn(colVar int, scale float64, dst []float64) {
	if scale == 0 {
		return
	}
	p := s.mat
	n := p.numVars
	if colVar >= n {
		dst[colVar-n] += scale
		return
	}
	for k := p.colPtr[colVar]; k < p.colPtr[colVar+1]; k++ {
		dst[p.rowIdx[k]] += scale * p.colVal[k]
	}
	baseM := p.numConstraints
	for r, dr := range s.extraRows {
		if dr.Dense != nil {
			dst[baseM+r] += scale * dr.Dense[colVar]
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					dst[baseM+r] += scale * dr.Val[k]
					break
				}
			}
		}
	}
}

func (s *LPSolver) appendBasisColumn(colVar int, rows []int, vals []float64) ([]int, []float64) {
	p := s.mat
	n := p.numVars
	if colVar >= n {
		rows = append(rows, colVar-n)
		vals = append(vals, 1.0)
		return rows, vals
	}
	start, end := p.colPtr[colVar], p.colPtr[colVar+1]
	rows = append(rows, p.rowIdx[start:end]...)
	vals = append(vals, p.colVal[start:end]...)
	baseM := p.numConstraints
	for r, dr := range s.extraRows {
		if dr.Dense != nil {
			if v := dr.Dense[colVar]; v != 0 {
				rows = append(rows, baseM+r)
				vals = append(vals, v)
			}
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					if v := dr.Val[k]; v != 0 {
						rows = append(rows, baseM+r)
						vals = append(vals, v)
					}
					break
				}
			}
		}
	}
	return rows, vals
}

func (s *LPSolver) rowRHS(i int) float64 {
	if i < s.mat.numConstraints {
		return s.mat.rowRHS[i]
	}
	return s.extraRows[i-s.mat.numConstraints].RHS
}

// Solve optimizes the current continuous LP relaxation via revised dual simplex with a two-phase primal simplex fallback.
func (s *LPSolver) Solve() LPResult {
	if s == nil || s.prob == nil || s.prob.invalidErr != nil {
		return LPResult{Status: StatusUndefined}
	}
	p := s.prob
	if s.timeExpired() {
		return LPResult{Status: StatusTimeLimit}
	}
	n := p.numVars
	m := s.TotalRows()
	ws := s.ws

	for j := range n {
		if s.unscaledBound(j, false) > s.unscaledBound(j, true)+primalTol {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		for i := range p.numConstraints {
			if 0 < p.rowLB[i]-primalTol || 0 > p.rowUB[i]+primalTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		for _, dr := range s.extraRows {
			rhs := dr.RHS
			if s.scaled {
				rhs = dr.OrigRHS
			}
			if 0 > rhs+primalTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		return LPResult{
			Status:       StatusOptimal,
			ObjectiveMin: 0,
			Objective:    p.objConstant,
			Primal:       []float64{},
			Dual:         make([]float64, p.numConstraints),
			ReducedCosts: []float64{},
		}
	}
	if m == 0 {
		return s.solveUnconstrained()
	}

	copy(ws.workCost, ws.cost)
	if !s.luValid || ws.lu.m != m || ws.lu.NeedsRefactor() {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err := s.factorizeBasis(); err != nil {
				return LPResult{Status: StatusNumericalError}
			}
		}
		s.computeDualsAndReducedCosts(ws.workCost)
		s.dualValid = true
	} else if !s.dualValid {
		s.computeDualsAndReducedCosts(ws.workCost)
		s.dualValid = true
	}

	dualFeasible := s.stabilizeNonBasicBounds()
	s.computePrimalValues()

	totalIter := 0

	if dualFeasible {
		status, iters := s.runDualSimplexLoop(ws.workCost)
		totalIter += iters
		if status == StatusOptimal {
			s.dualValid = true
			return s.buildResult(StatusOptimal, totalIter)
		}
		if status == StatusInfeasible || status == StatusIterationLimit || status == StatusTimeLimit {
			s.dualValid = false
			return LPResult{Status: status, Iterations: totalIter}
		}
	}

	s.dualValid = false
	remIter := s.MaxIter - totalIter
	if remIter <= 0 {
		return LPResult{Status: StatusIterationLimit, Iterations: totalIter}
	}
	status, iters := s.solveTwoPhasePrimalWithLimit(remIter)
	totalIter += iters
	if status != StatusOptimal {
		return LPResult{Status: status, Iterations: totalIter}
	}
	s.dualValid = true
	return s.buildResult(StatusOptimal, totalIter)
}

func (s *LPSolver) reinitCrashKeepingBounds() {
	n := s.prob.numVars
	var savedLB, savedUB []float64
	if s.scaled {
		savedLB = slices.Clone(s.origLB)
		savedUB = slices.Clone(s.origUB)
	} else {
		savedLB = slices.Clone(s.ws.lb[:n])
		savedUB = slices.Clone(s.ws.ub[:n])
	}
	s.InitCrashBasis()
	s.SetStructuralBounds(savedLB, savedUB)
}

func (s *LPSolver) solveUnconstrained() LPResult {
	p := s.prob
	n := p.numVars
	ws := s.ws
	primal := make([]float64, n)
	dj := make([]float64, n)
	objMin := 0.0
	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		c := ws.cost[j]
		dj[j] = senseSign * c
		if c > 0 {
			if math.IsInf(ws.lb[j], -1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.lb[j]
		} else if c < 0 {
			if math.IsInf(ws.ub[j], 1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.ub[j]
		} else {
			if !math.IsInf(ws.lb[j], -1) && !math.IsInf(ws.ub[j], 1) {
				if ws.lb[j] <= 0 && 0 <= ws.ub[j] {
					primal[j] = 0
				} else {
					primal[j] = ws.lb[j]
				}
			} else if !math.IsInf(ws.lb[j], -1) {
				primal[j] = max(0.0, ws.lb[j])
			} else if !math.IsInf(ws.ub[j], 1) {
				primal[j] = min(0.0, ws.ub[j])
			}
		}
		if c != 0 {
			objMin += c * primal[j]
		}
	}
	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		ReducedCosts: dj,
	}
}

func (s *LPSolver) factorizeBasis() error {
	ws := s.ws
	m := s.TotalRows()
	err := ws.lu.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		return s.appendBasisColumn(ws.basis[col], rows, vals)
	})
	s.luValid = err == nil
	return err
}

func (s *LPSolver) computeDualsAndReducedCosts(cost []float64) {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar

	cost = cost[:tot]
	basis := ws.basis[:m]
	y := ws.y[:m]
	status := ws.status[:tot]
	dj := ws.dj[:tot]

	for i, bVar := range basis {
		y[i] = cost[bVar]
	}
	ws.lu.BTRAN(ws.y)

	baseM := p.numConstraints
	colPtr := p.colPtr[:n+1]
	rowIdx := p.rowIdx
	colVal := p.colVal[:len(rowIdx)]
	for j := range n {
		if status[j] == statusBasic {
			dj[j] = 0
			continue
		}
		dot := 0.0
		start, end := colPtr[j], colPtr[j+1]
		rows := rowIdx[start:end]
		vals := colVal[start:end]
		for k, r := range rows {
			if uint(r) < uint(m) {
				dot += y[r] * vals[k]
			}
		}
		dj[j] = cost[j] - dot
	}
	for r, dr := range s.extraRows {
		yr := y[baseM+r]
		if yr <= luDropTol && yr >= -luDropTol {
			continue
		}
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(tot) && status[c] != statusBasic {
				dj[c] -= yr * vals[k]
			}
		}
	}

	for i, yi := range y {
		slackVar := n + i
		if status[slackVar] == statusBasic {
			dj[slackVar] = 0
		} else {
			dj[slackVar] = cost[slackVar] - yi
		}
	}
}

func (s *LPSolver) stabilizeNonBasicBounds() bool {
	ws := s.ws
	tot := ws.totVar
	status := ws.status[:tot]
	lbSlice := ws.lb[:tot]
	ubSlice := ws.ub[:tot]
	djSlice := ws.dj[:tot]
	xSlice := ws.x[:tot]
	dTol := s.effDualTol
	dualFeas := true

	for k, st := range status {
		if st == statusBasic {
			continue
		}
		lb, ub, dj := lbSlice[k], ubSlice[k], djSlice[k]
		if st == statusFixed {
			xSlice[k] = lb
			continue
		}
		hasLB := !math.IsInf(lb, -1)
		hasUB := !math.IsInf(ub, 1)

		if hasLB && hasUB {
			if dj < -dTol {
				status[k] = statusAtUpper
				xSlice[k] = ub
			} else if dj > dTol {
				status[k] = statusAtLower
				xSlice[k] = lb
			} else {
				if st != statusAtUpper {
					status[k] = statusAtLower
					xSlice[k] = lb
				} else {
					xSlice[k] = ub
				}
			}
			continue
		}
		xSlice[k] = nonBasicValue(st, lb, ub)
		if st == statusAtLower && dj < -dTol {
			dualFeas = false
		} else if st == statusAtUpper && dj > dTol {
			dualFeas = false
		} else if st == statusSuperBasic && (dj > dTol || dj < -dTol) {
			dualFeas = false
		}
	}
	return dualFeas
}

func (s *LPSolver) isDualFeasible() bool {
	ws := s.ws
	status := ws.status[:ws.totVar]
	djSlice := ws.dj[:len(status)]
	lbSlice := ws.lb[:len(status)]
	ubSlice := ws.ub[:len(status)]
	vTol := s.effDualTol
	for k, st := range status {
		dj := djSlice[k]
		if math.IsNaN(dj) {
			return false
		}
		switch st {
		case statusAtLower:
			if dj < -vTol {
				return false
			}
			if dj < 0 && !math.IsInf(ubSlice[k], 1) && (-dj)*(ubSlice[k]-lbSlice[k]) > primalTol {
				return false
			}
		case statusAtUpper:
			if dj > vTol {
				return false
			}
			if dj > 0 && !math.IsInf(lbSlice[k], -1) && dj*(ubSlice[k]-lbSlice[k]) > primalTol {
				return false
			}
		case statusSuperBasic:
			if dj > vTol || dj < -vTol {
				return false
			}
		}
	}
	return true
}

func (s *LPSolver) computePrimalValues() {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	status := ws.status[:tot]
	xSlice := ws.x[:tot]
	xbSlice := ws.xb[:m]
	basis := ws.basis[:m]

	for i := range m {
		xb := s.rowRHS(i)
		slackVar := n + i
		if status[slackVar] != statusBasic {
			xb -= xSlice[slackVar]
		}
		xbSlice[i] = xb
	}

	baseM := p.numConstraints
	colPtr := p.colPtr[:n+1]
	rowIdx := p.rowIdx
	colVal := p.colVal[:len(rowIdx)]
	for j := range n {
		if status[j] == statusBasic {
			continue
		}
		xj := xSlice[j]
		if xj == 0 {
			continue
		}
		start, end := colPtr[j], colPtr[j+1]
		rows := rowIdx[start:end]
		vals := colVal[start:end]
		for k, r := range rows {
			if uint(r) < uint(m) {
				xbSlice[r] -= vals[k] * xj
			}
		}
	}
	for r, dr := range s.extraRows {
		sum := 0.0
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(tot) && status[c] != statusBasic {
				sum += vals[k] * xSlice[c]
			}
		}
		xbSlice[baseM+r] -= sum
	}

	ws.lu.FTRAN(ws.xb)
	for i, bVar := range basis {
		if uint(bVar) < uint(tot) {
			xSlice[bVar] = xbSlice[i]
		}
	}
}

// ComputeTableauRow evaluates row r of B^{-1} [A I] into WS.Rho and WS.TabRow, tracking either sparse nonbasic support or 64-column chunks.
func (s *LPSolver) ComputeTableauRow(r int) {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	rho := ws.rho[:m]
	tabRow := ws.tabRow[:tot]
	tabMark := ws.tabRowMark[:tot]
	status := ws.status[:tot]
	basis := ws.basis[:m]
	numChunks := (n + 63) >> 6
	chunkMark := ws.tabRowChunkMark[:numChunks]

	if ws.rhoSparse {
		for _, i := range ws.rhoIdx {
			if uint(i) < uint(m) {
				rho[i] = 0
			}
		}
		ws.rhoIdx = ws.rhoIdx[:0]
	} else {
		clear(rho)
	}
	rho[r] = 1.0
	var unitHint [1]int
	unitHint[0] = r
	ws.rhoIdx = ws.lu.BTRANSparseHint(ws.rho, unitHint[:], ws.rhoIdx)
	ws.rhoSparse = true

	if ws.tabRowSparse {
		for _, k := range ws.tabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
				tabMark[k] = false
			}
		}
		ws.tabRowIdx = ws.tabRowIdx[:0]
	} else {
		for _, ch := range ws.tabRowChunks {
			if uint(ch) < uint(numChunks) {
				chunkMark[ch] = false
				start := ch << 6
				end := min(start+64, n)
				clear(tabRow[start:end])
			}
		}
		ws.tabRowChunks = ws.tabRowChunks[:0]
		for _, k := range ws.tabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
			}
		}
		ws.tabRowIdx = ws.tabRowIdx[:0]
	}
	baseM := p.numConstraints

	if n > 0 && n <= DenseThreshold && len(p.denseRows) == baseM*p.rowStride {
		ws.tabRowSparse = false
		chunks := ws.tabRowChunks[:0]
		for ch := range numChunks {
			chunkMark[ch] = true
			chunks = append(chunks, ch)
		}
		ws.tabRowChunks = chunks
		stride := p.rowStride
		tabStruct := tabRow[:n]
		sparseRowCap := n >> 2
		if simd.UsingHardwareSIMDWith(s.DisableSIMD) && n >= 32 {
			sparseRowCap = min(8, sparseRowCap)
		}
		tabIdx := ws.tabRowIdx
		for _, i := range ws.rhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.rowPtr[i], p.rowPtr[i+1]
				if end-start <= sparseRowCap {
					cols := p.colIdx[start:end]
					vals := p.rowVal[start:end]
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
						}
					}
				} else {
					s.axpy64(tabStruct, p.denseRows[i*stride:][:n], yi)
				}
			} else {
				dr := s.extraRows[i-baseM]
				if len(dr.ColIdx) <= sparseRowCap {
					cols := dr.ColIdx
					vals := dr.Val[:len(cols)]
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
						}
					}
				} else if dr.Dense != nil {
					s.axpy64(tabStruct, dr.Dense[:n], yi)
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabRow[slack] = yi
				tabIdx = append(tabIdx, slack)
			}
		}
		for _, bVar := range basis {
			if uint(bVar) < uint(n) {
				tabStruct[bVar] = 0
			}
		}
		br := basis[r]
		tabRow[br] = 1.0
		if br >= n {
			tabIdx = append(tabIdx, br)
		}
		ws.tabRowIdx = tabIdx
		return
	}

	totalNNZ := 0
	for _, i := range ws.rhoIdx {
		if i < baseM {
			totalNNZ += p.rowPtr[i+1] - p.rowPtr[i]
		} else {
			totalNNZ += len(s.extraRows[i-baseM].ColIdx)
		}
	}

	if totalNNZ >= 64 {
		ws.tabRowSparse = false
		tabStruct := tabRow[:n]
		chunks := ws.tabRowChunks[:0]
		tabIdx := ws.tabRowIdx
		for _, i := range ws.rhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			var cols []int
			var vals []float64
			if i < baseM {
				start, end := p.rowPtr[i], p.rowPtr[i+1]
				cols = p.colIdx[start:end]
				vals = p.rowVal[start:end]
			} else {
				dr := s.extraRows[i-baseM]
				cols = dr.ColIdx
				vals = dr.Val[:len(cols)]
			}
			cnt := len(cols)
			if cnt >= 32 {
				c0 := cols[0]
				if cols[cnt-1]-c0 == cnt-1 {
					s.axpy64(tabStruct[c0:][:cnt], vals[:cnt], yi)
					for ch := c0 >> 6; ch <= (c0+cnt-1)>>6; ch++ {
						if uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
				} else if cols[31]-c0 == 31 {
					lo, hi := 32, cnt-1
					for lo < hi {
						mid := int(uint(lo+hi+1) >> 1)
						if cols[mid-1]-c0 == mid-1 {
							lo = mid
						} else {
							hi = mid - 1
						}
					}
					s.axpy64(tabStruct[c0:][:lo], vals[:lo], yi)
					for ch := c0 >> 6; ch <= (c0+lo-1)>>6; ch++ {
						if uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
					for k := lo; k < cnt; k++ {
						col := cols[k]
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
							if ch := col >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
								chunkMark[ch] = true
								chunks = append(chunks, ch)
							}
						}
					}
				} else {
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
							if ch := col >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
								chunkMark[ch] = true
								chunks = append(chunks, ch)
							}
						}
					}
				}
			} else {
				for k, col := range cols {
					if uint(col) < uint(n) {
						tabStruct[col] += yi * vals[k]
						if ch := col >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabRow[slack] = yi
				tabIdx = append(tabIdx, slack)
			}
		}
		for _, bVar := range basis {
			if uint(bVar) < uint(n) && chunkMark[bVar>>6] {
				tabStruct[bVar] = 0
			}
		}
		br := basis[r]
		tabRow[br] = 1.0
		if br < n {
			if ch := br >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
				chunkMark[ch] = true
				chunks = append(chunks, ch)
			}
		} else {
			tabIdx = append(tabIdx, br)
		}
		ws.tabRowChunks = chunks
		ws.tabRowIdx = tabIdx
	} else {
		ws.tabRowSparse = true
		tabIdx := ws.tabRowIdx
		for _, i := range ws.rhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.rowPtr[i], p.rowPtr[i+1]
				cols := p.colIdx[start:end]
				vals := p.rowVal[start:end]
				for k, col := range cols {
					if uint(col) >= uint(tot) || status[col] == statusBasic {
						continue
					}
					if !tabMark[col] {
						tabMark[col] = true
						tabIdx = append(tabIdx, col)
					}
					tabRow[col] += yi * vals[k]
				}
			} else {
				dr := s.extraRows[i-baseM]
				cols := dr.ColIdx
				vals := dr.Val[:len(cols)]
				for k, col := range cols {
					if uint(col) >= uint(tot) || status[col] == statusBasic {
						continue
					}
					if !tabMark[col] {
						tabMark[col] = true
						tabIdx = append(tabIdx, col)
					}
					tabRow[col] += yi * vals[k]
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabMark[slack] = true
				tabIdx = append(tabIdx, slack)
				tabRow[slack] = yi
			}
		}
		br := basis[r]
		if uint(br) < uint(tot) {
			if !tabMark[br] {
				tabMark[br] = true
				tabIdx = append(tabIdx, br)
			}
			tabRow[br] = 1.0
		}
		ws.tabRowIdx = tabIdx
	}
}

func bfrtCandLess(a, b bfrtCand) bool {
	if a.ratio != b.ratio {
		return a.ratio < b.ratio
	}
	if a.absA != b.absA {
		return a.absA > b.absA
	}
	return a.col < b.col
}

func sortBFRTCands(a []bfrtCand) {
	for len(a) > 16 {
		mid := len(a) >> 1
		last := len(a) - 1
		if bfrtCandLess(a[mid], a[0]) {
			a[0], a[mid] = a[mid], a[0]
		}
		if bfrtCandLess(a[last], a[mid]) {
			a[mid], a[last] = a[last], a[mid]
			if bfrtCandLess(a[mid], a[0]) {
				a[0], a[mid] = a[mid], a[0]
			}
		}
		pivot := a[mid]
		i, j := 0, last
		for i <= j {
			for bfrtCandLess(a[i], pivot) {
				i++
			}
			for bfrtCandLess(pivot, a[j]) {
				j--
			}
			if i <= j {
				a[i], a[j] = a[j], a[i]
				i++
				j--
			}
		}
		if j < len(a)-i {
			if j > 0 {
				sortBFRTCands(a[:j+1])
			}
			a = a[i:]
		} else {
			if i < len(a)-1 {
				sortBFRTCands(a[i:])
			}
			a = a[:j+1]
		}
	}
	for i := 1; i < len(a); i++ {
		cur := a[i]
		j := i - 1
		for j >= 0 && bfrtCandLess(cur, a[j]) {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = cur
	}
}

// evalDualRatioCand classifies tableau entry tab on a leaving row; ok reports whether column k blocks dual movement.
func evalDualRatioCand(tab, leaveDir float64, st varBasisStatus, djk float64) (absA, d float64, superBasic, ok bool) {
	alphaBar := -leaveDir * tab
	switch st {
	case statusAtLower:
		if alphaBar > pivotTol {
			return alphaBar, djk, false, true
		}
	case statusAtUpper:
		if -alphaBar > pivotTol {
			return -alphaBar, -djk, false, true
		}
	case statusSuperBasic:
		if alphaBar < 0 {
			alphaBar = -alphaBar
		}
		return alphaBar, 0, true, true
	}
	return 0, 0, false, false
}

// selectDualBFRT executes the Bound-Flipping Ratio Test over boxed nonbasic columns outside equality rows.
func (s *LPSolver) selectDualBFRT(primalInfeas, leaveDir float64) int {
	ws := s.ws
	cands := ws.bfrtCands
	if len(cands) == 0 {
		return -1
	}
	minIdx := 0
	for i := 1; i < len(cands); i++ {
		if bfrtCandLess(cands[i], cands[minIdx]) {
			minIdx = i
		}
	}
	c0 := cands[minIdx]
	if c0.ratio <= primalTol || c0.absA < minStaysFeasPivot {
		return -1
	}
	k0 := c0.col
	if (k0 < len(s.colHasEq) && s.colHasEq[k0]) || ws.status[k0] == statusSuperBasic || math.IsInf(ws.lb[k0], -1) || math.IsInf(ws.ub[k0], 1) {
		return -1
	}
	maxFlip0 := c0.absA * (ws.ub[k0] - ws.lb[k0])
	if maxFlip0 <= 0 || primalInfeas-maxFlip0 <= primalTol {
		return -1
	}

	n := s.prob.numVars
	tot := ws.totVar
	tabRow := ws.tabRow[:tot]
	status := ws.status[:tot]
	dj := ws.dj[:tot]
	cands = cands[:0]
	if !ws.tabRowSparse {
		for _, ch := range ws.tabRowChunks {
			start := ch << tabRowChunkBits
			end := min(start+tabRowChunkSize, n)
			for k := start; k < end; k++ {
				if tab := tabRow[k]; tab > pivotTol || tab < -pivotTol {
					if absA, d, super, ok := evalDualRatioCand(tab, leaveDir, status[k], dj[k]); ok {
						ratio := 0.0
						if !super && d > 0 {
							ratio = d / absA
						}
						cands = append(cands, bfrtCand{col: k, ratio: ratio, absA: absA})
					}
				}
			}
		}
	}
	for _, k := range ws.tabRowIdx {
		if uint(k) < uint(tot) {
			if tab := tabRow[k]; tab > pivotTol || tab < -pivotTol {
				if absA, d, super, ok := evalDualRatioCand(tab, leaveDir, status[k], dj[k]); ok {
					ratio := 0.0
					if !super && d > 0 {
						ratio = d / absA
					}
					cands = append(cands, bfrtCand{col: k, ratio: ratio, absA: absA})
				}
			}
		}
	}
	ws.bfrtCands = cands
	if len(cands) <= 1 {
		return -1
	}

	lbSlice := ws.lb[:tot]
	ubSlice := ws.ub[:tot]
	colHasEq := s.colHasEq
	for i := range cands {
		k := cands[i].col
		absA := cands[i].absA
		maxFlip := math.Inf(1)
		inEq := uint(k) < uint(len(colHasEq)) && colHasEq[k]
		if !inEq && absA >= minBFRTFlipPivot && uint(k) < uint(tot) && status[k] != statusSuperBasic {
			lbk, ubk := lbSlice[k], ubSlice[k]
			if !math.IsInf(lbk, -1) && !math.IsInf(ubk, 1) {
				maxFlip = absA * (ubk - lbk)
			}
		}
		cands[i].maxFlip = maxFlip
	}

	sortBFRTCands(cands)

	remInfeas := primalInfeas
	stopIdx := 0
	for stopIdx < len(cands)-1 && len(ws.flipCols) < maxBFRTFlips {
		c := cands[stopIdx]
		if math.IsInf(c.maxFlip, 1) || c.maxFlip <= 0 || c.absA < minStaysFeasPivot || remInfeas-c.maxFlip <= primalTol {
			break
		}
		ws.flipCols = append(ws.flipCols, c.col)
		remInfeas -= c.maxFlip
		stopIdx++
	}
	if len(ws.flipCols) == 0 {
		return -1
	}

	thetaLim := math.Inf(1)
	for i := stopIdx; i < len(cands); i++ {
		hr := cands[i].ratio + harrisTol/cands[i].absA
		if hr < thetaLim {
			thetaLim = hr
		}
	}

	enterVar := -1
	enterRatio := 0.0
	bestAbs := 0.0
	bestStaysFeas := false
	for i := stopIdx; i < len(cands); i++ {
		c := cands[i]
		if c.ratio > thetaLim+minScaledPrimalTol {
			break
		}
		staysFeas := c.absA >= minStaysFeasPivot && remInfeas <= c.maxFlip+primalTol
		if staysFeas && !bestStaysFeas {
			bestStaysFeas = true
			bestAbs = c.absA
			enterVar = c.col
			enterRatio = c.ratio
		} else if staysFeas == bestStaysFeas && (c.absA > bestAbs || (c.absA == bestAbs && c.col < enterVar)) {
			bestAbs = c.absA
			enterVar = c.col
			enterRatio = c.ratio
		}
	}
	if enterVar == -1 || bestAbs < minStaysFeasPivot {
		ws.flipCols = ws.flipCols[:0]
		return -1
	}
	// Only flip variables whose breakpoint is strictly before enterRatio so every
	// flipped variable moves strictly into the dual-feasible interior.
	keep := 0
	for i := 0; i < len(ws.flipCols); i++ {
		if cands[i].ratio < enterRatio-pivotTieRelTol {
			ws.flipCols[keep] = ws.flipCols[i]
			keep++
		}
	}
	ws.flipCols = ws.flipCols[:keep]
	if keep == 0 {
		return -1
	}
	return enterVar
}

func (s *LPSolver) recomputeRowBasicDeg() {
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	rowDeg := ws.rowBasicDeg[:m]
	clear(rowDeg)
	basis := ws.basis[:m]
	colPtr := p.colPtr[:n+1]
	rowIdx := p.rowIdx
	for _, bVar := range basis {
		if uint(bVar) < uint(n) {
			for _, r := range rowIdx[colPtr[bVar]:colPtr[bVar+1]] {
				rowDeg[r]++
			}
		}
	}
}

func (s *LPSolver) colBasisLoad(k int, dynamic bool) int {
	p := s.prob
	if uint(k) >= uint(p.numVars) {
		return 0
	}
	start, end := p.colPtr[k], p.colPtr[k+1]
	if !dynamic {
		return end - start
	}
	rowDeg := s.ws.rowBasicDeg
	load := 0
	for _, r := range p.rowIdx[start:end] {
		load += rowDeg[r]
	}
	return load
}

// applyDualBoundFlips flips all columns selected by selectDualBFRT to their opposite bounds and updates basic primals via a batched sparse FTRAN.
func (s *LPSolver) applyDualBoundFlips(leaveRow int, leaveDir float64) float64 {
	ws := s.ws
	m := s.TotalRows()
	tot := ws.totVar
	lb := ws.lb[:tot]
	ub := ws.ub[:tot]
	status := ws.status[:tot]
	xb := ws.xb[:m]
	basis := ws.basis[:m]

	if ws.tauSparse {
		for _, i := range ws.tauIdx {
			if uint(i) < uint(m) {
				ws.tau[i] = 0
			}
		}
		ws.tauIdx = ws.tauIdx[:0]
	} else {
		clear(ws.tau)
	}
	for _, fk := range ws.flipCols {
		lbf, ubf := lb[fk], ub[fk]
		var deltaX float64
		if status[fk] == statusAtLower {
			status[fk] = statusAtUpper
			ws.x[fk] = ubf
			deltaX = ubf - lbf
		} else {
			status[fk] = statusAtLower
			ws.x[fk] = lbf
			deltaX = lbf - ubf
		}
		s.addScaledColumn(fk, deltaX, ws.tau)
	}
	ws.tauIdx = ws.lu.FTRANSparse(ws.tau, ws.tauIdx)
	for _, i := range ws.tauIdx {
		xb[i] -= ws.tau[i]
		ws.tau[i] = 0
		ws.x[basis[i]] = xb[i]
	}
	ws.tauIdx = ws.tauIdx[:0]
	ws.tauSparse = true
	bVar := basis[leaveRow]
	if leaveDir > 0 {
		return lb[bVar] - xb[leaveRow]
	}
	return xb[leaveRow] - ub[bVar]
}

// updateDualSteepestEdge updates row steepest-edge weights via the Forrest-Goldfarb recurrence over FColIdx.
func (s *LPSolver) updateDualSteepestEdge(leaveRow int, pivElem float64) {
	ws := s.ws
	m := s.TotalRows()
	dse := ws.dse[:m]
	invPiv := 1.0 / pivElem
	invPiv2 := invPiv * invPiv
	leaveWeight := dse[leaveRow]
	newLeaveWeight := leaveWeight * invPiv2
	if !(newLeaveWeight >= minDSEWeight && newLeaveWeight <= maxDSEWeight) {
		newLeaveWeight = min(max(newLeaveWeight, minDSEWeight), maxDSEWeight)
		if math.IsNaN(newLeaveWeight) {
			newLeaveWeight = 1.0
		}
	}
	fCol := ws.fCol[:m]
	if len(ws.fColIdx) > 1 {
		tau := ws.tau[:m]
		if ws.tauSparse {
			for _, i := range ws.tauIdx {
				if uint(i) < uint(m) {
					tau[i] = 0
				}
			}
			ws.tauIdx = ws.tauIdx[:0]
		} else {
			clear(tau)
		}
		if ws.rhoSparse {
			rho := ws.rho[:m]
			for _, i := range ws.rhoIdx {
				tau[i] = rho[i]
			}
			ws.tauIdx = ws.lu.FTRANSparseHint(tau, ws.rhoIdx, ws.tauIdx)
		} else {
			copy(tau, ws.rho[:m])
			ws.tauIdx = ws.lu.FTRANSparse(ws.tau, ws.tauIdx)
		}
		ws.tauSparse = true
		for _, i := range ws.fColIdx {
			if i != leaveRow && uint(i) < uint(m) {
				fi := fCol[i]
				if fi > minDSEColEntry || fi < -minDSEColEntry {
					ratio := fi * invPiv
					w := dse[i] - 2.0*ratio*tau[i] + ratio*ratio*leaveWeight
					if !(w >= minDSEWeight && w <= maxDSEWeight) {
						w = min(max(w, minDSEWeight), maxDSEWeight)
						if math.IsNaN(w) {
							w = 1.0
						}
					}
					dse[i] = w
				}
			}
		}
	}
	dse[leaveRow] = newLeaveWeight
}

func dualBlockDir(alphaBar float64, st varBasisStatus) (absA, djSign float64, super, ok bool) {
	switch st {
	case statusAtLower:
		if alphaBar > pivotTol {
			return alphaBar, 1.0, false, true
		}
	case statusAtUpper:
		if alphaBar < -pivotTol {
			return -alphaBar, -1.0, false, true
		}
	case statusSuperBasic:
		return math.Abs(alphaBar), 0, true, true
	}
	return 0, 0, false, false
}

func dualSubTolCheck(tab, alphaBar float64, st varBasisStatus) (blocks, isNaN bool) {
	if math.IsNaN(tab) {
		return false, true
	}
	return (st == statusAtLower && alphaBar > 0) || (st == statusAtUpper && alphaBar < 0) || st == statusSuperBasic, false
}

// dualRatioPass1 runs Pass 1 of the Harris dual ratio test in registers without loading dj[k] on non-blocking columns.
func (s *LPSolver) dualRatioPass1(leaveDir float64, bland bool) (thetaMax, minRatio float64, subTol, hasNaN bool) {
	ws := s.ws
	n := s.prob.numVars
	tot := ws.totVar
	thetaMax = math.Inf(1)
	minRatio = math.Inf(1)
	cands := ws.bfrtCands[:0]
	tabRow := ws.tabRow[:tot]
	status := ws.status[:tot]
	dj := ws.dj[:tot]
	effHarris := min(harrisTol, 0.01*s.effDualTol)

	if !ws.tabRowSparse {
		for _, ch := range ws.tabRowChunks {
			start := ch << tabRowChunkBits
			end := min(start+tabRowChunkSize, n)
			for k := start; k < end; k++ {
				tab := tabRow[k]
				if tab == 0 {
					continue
				}
				alphaBar := -leaveDir * tab
				st := status[k]
				if tab > pivotTol || tab < -pivotTol {
					absA, djSign, super, ok := dualBlockDir(alphaBar, st)
					if !ok {
						continue
					}
					if super {
						thetaMax, minRatio = 0, 0
						cands = append(cands, bfrtCand{col: k, ratio: 0, absA: absA})
						continue
					}
					d := djSign * dj[k]
					if math.IsNaN(d) {
						ws.bfrtCands = cands
						return 0, 0, false, true
					}
					d = max(0.0, d)
					if bland {
						minRatio = min(minRatio, d/absA)
					}
					if d <= (thetaMax+2*minScaledPrimalTol)*absA {
						thetaMax = min(thetaMax, (d+effHarris)/absA)
						cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
					}
				} else {
					blk, nan := dualSubTolCheck(tab, alphaBar, st)
					if nan {
						ws.bfrtCands = cands
						return 0, 0, false, true
					}
					subTol = subTol || blk
				}
			}
		}
	}
	for _, k := range ws.tabRowIdx {
		if uint(k) >= uint(tot) {
			continue
		}
		tab := tabRow[k]
		if tab == 0 {
			continue
		}
		alphaBar := -leaveDir * tab
		st := status[k]
		if !(tab > pivotTol || tab < -pivotTol) {
			blk, nan := dualSubTolCheck(tab, alphaBar, st)
			if nan {
				ws.bfrtCands = cands
				return 0, 0, false, true
			}
			if blk {
				subTol = true
			}
			continue
		}
		absA, djSign, super, ok := dualBlockDir(alphaBar, st)
		if !ok {
			continue
		}
		if super {
			thetaMax, minRatio = 0, 0
			cands = append(cands, bfrtCand{col: k, ratio: 0, absA: absA})
			continue
		}
		d := djSign * dj[k]
		if math.IsNaN(d) {
			ws.bfrtCands = cands
			return 0, 0, false, true
		}
		if d < 0 {
			d = 0
		}
		if bland {
			if r0 := d / absA; r0 < minRatio {
				minRatio = r0
			}
		}
		if d <= (thetaMax+2*minScaledPrimalTol)*absA {
			r0 := d / absA
			if ratio := (d + effHarris) / absA; ratio < thetaMax {
				thetaMax = ratio
			}
			cands = append(cands, bfrtCand{col: k, ratio: r0, absA: absA})
		}
	}
	ws.bfrtCands = cands
	return thetaMax, minRatio, subTol, false
}

func (s *LPSolver) runDualSimplexLoop(cost []float64) (Status, int) {
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	varTol := s.syncVarTol(tot)
	s.recomputeRowBasicDeg()
	degenStreak := 0
	blandTrigger := max(blandMinTrigger, blandRowMult*m)
	lastRefLeave := -1
	lastRefEnter := -1

	for iter := range s.MaxIter {
		if (iter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, iter
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		}

		bland := degenStreak >= blandTrigger && ((degenStreak-blandTrigger)&blandBurstMask) < blandBurstWindow
		leaveRow := -1
		bestScore := -1.0
		bestLeaveVar := tot
		leaveDir := 0.0
		primalInfeas := 0.0

		basis := ws.basis[:m]
		xb := ws.xb[:m]
		dse := ws.dse[:m]
		lb := ws.lb[:tot]
		ub := ws.ub[:tot]
		for r, bVar := range basis {
			val := xb[r]
			lbr, ubr := lb[bVar], ub[bVar]
			tol := varTol[bVar]
			var viol, dir float64
			if val < lbr-tol {
				viol = lbr - val
				dir = 1.0
			} else if val > ubr+tol {
				viol = val - ubr
				dir = -1.0
			}
			if viol > 0 {
				if bland {
					if leaveRow == -1 || bVar < bestLeaveVar {
						bestLeaveVar = bVar
						leaveRow = r
						leaveDir = dir
						primalInfeas = viol
					}
					continue
				}
				w := dse[r]
				if !(w >= minDSEWeight && w <= maxDSEWeight) {
					w = 1.0
					dse[r] = 1.0
				}
				score := (viol * viol) / w
				if leaveRow == -1 || score > bestScore {
					bestScore = score
					leaveRow = r
					leaveDir = dir
					primalInfeas = viol
				}
			}
		}

		if leaveRow == -1 {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, iter
				}
				s.computeDualsAndReducedCosts(cost)
				s.computePrimalValues()
				continue
			}
			if !s.isDualFeasible() {
				return StatusNumericalError, iter
			}
			return StatusOptimal, iter
		}

		s.ComputeTableauRow(leaveRow)

		thetaMax, minRatio, hadSubTolBlocking, hasNaN := s.dualRatioPass1(leaveDir, bland)
		if hasNaN {
			return StatusNumericalError, iter
		}
		cands := ws.bfrtCands
		tabRow := ws.tabRow[:tot]
		status := ws.status[:tot]
		dj := ws.dj[:tot]

		if math.IsInf(thetaMax, 1) {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err == nil {
					s.computeDualsAndReducedCosts(cost)
					s.computePrimalValues()
					continue
				}
			}
			if hadSubTolBlocking || !s.verifyDualInfeasibleRow(leaveRow, leaveDir, primalInfeas) {
				return StatusNumericalError, iter
			}
			return StatusInfeasible, iter
		}

		// Pass 2 of Harris ratio test: pick the largest-pivot candidate within thetaMax, preferring stays-feasible columns.
		enterVar := -1
		bestPivAbs := 0.0
		bestLoad := 0
		bestStaysFeas := false
		dynLoad := thetaMax > harrisTol && iter < 3*m
		thetaLim := thetaMax + minScaledPrimalTol
		if bland {
			blandLim := minRatio + primalRatioTieTol
			for _, c := range cands {
				if c.ratio <= blandLim && (enterVar == -1 || c.col < enterVar) {
					enterVar = c.col
				}
			}
		} else {
			scalePiv := n > scalePivotVarMin
			for _, c := range cands {
				if c.ratio <= thetaLim {
					k := c.col
					absA := c.absA
					staysFeas := absA >= minStaysFeasPivot && primalInfeas <= absA*(ub[k]-lb[k])+primalTol
					pivScore := absA
					if scalePiv {
						pivScore = absA * varTol[k]
					}
					if enterVar == -1 || (staysFeas && !bestStaysFeas) {
						bestStaysFeas = staysFeas
						bestPivAbs = pivScore
						bestLoad = s.colBasisLoad(k, dynLoad)
						enterVar = k
					} else if staysFeas == bestStaysFeas {
						if pivScore > bestPivAbs*(1.0+pivotTieRelTol) {
							bestPivAbs = pivScore
							bestLoad = s.colBasisLoad(k, dynLoad)
							enterVar = k
						} else if pivScore >= bestPivAbs*(1.0-pivotTieRelTol) {
							load := s.colBasisLoad(k, dynLoad)
							if load < bestLoad || (load == bestLoad && k < enterVar) {
								bestPivAbs = pivScore
								bestLoad = load
								enterVar = k
							}
						}
					}
				}
			}
		}

		if enterVar == -1 {
			if hadSubTolBlocking || !s.verifyDualInfeasibleRow(leaveRow, leaveDir, primalInfeas) {
				return StatusNumericalError, iter
			}
			return StatusInfeasible, iter
		}

		ws.flipCols = ws.flipCols[:0]
		if !bland && !bestStaysFeas && thetaMax > harrisTol && !math.IsInf(lb[enterVar], -1) && !math.IsInf(ub[enterVar], 1) {
			if bfrtEnter := s.selectDualBFRT(primalInfeas, leaveDir); bfrtEnter != -1 {
				enterVar = bfrtEnter
			}
		}

		if ws.fColSparse {
			for _, i := range ws.fColIdx {
				ws.fCol[i] = 0
			}
			ws.fColIdx = ws.fColIdx[:0]
		} else {
			clear(ws.fCol)
		}
		ws.fColIdx = s.fillColumnSparse(enterVar, ws.fCol, ws.fColIdx)
		ws.fColIdx = ws.lu.FTRANSparseHint(ws.fCol, ws.fColIdx, ws.fColIdx)
		ws.fColSparse = true

		pivElem := ws.fCol[leaveRow]
		tabElem := tabRow[enterVar]
		if (pivElem < minDualPivotElem && pivElem > -minDualPivotElem) ||
			pivElem*tabElem <= 0 ||
			math.Abs(pivElem-tabElem) > pivAgreeRelTol*max(1.0, math.Abs(pivElem)) {
			if leaveRow == lastRefLeave && enterVar == lastRefEnter {
				return StatusNumericalError, iter
			}
			lastRefLeave, lastRefEnter = leaveRow, enterVar
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			continue
		}
		lastRefLeave, lastRefEnter = -1, -1

		if len(ws.flipCols) > 0 {
			primalInfeas = s.applyDualBoundFlips(leaveRow, leaveDir)
		}
		if math.Abs(dj[enterVar]/tabElem) > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}

		s.updateDualSteepestEdge(leaveRow, pivElem)
		fCol := ws.fCol[:m]

		leaveVar := basis[leaveRow]
		var newLeaveVal float64
		if leaveDir > 0 {
			status[leaveVar] = statusAtLower
			newLeaveVal = lb[leaveVar]
		} else {
			status[leaveVar] = statusAtUpper
			newLeaveVal = ub[leaveVar]
		}
		if lb[leaveVar] == ub[leaveVar] {
			status[leaveVar] = statusFixed
		}

		primalStep := -leaveDir * primalInfeas / pivElem
		oldEnterVal := ws.x[enterVar]
		newEnterVal := oldEnterVal + primalStep

		xSlice := ws.x[:tot]
		for _, i := range ws.fColIdx {
			if i != leaveRow && uint(i) < uint(m) {
				xb[i] -= fCol[i] * primalStep
				xSlice[basis[i]] = xb[i]
			}
		}
		xb[leaveRow] = newEnterVal
		xSlice[leaveVar] = newLeaveVal
		xSlice[enterVar] = newEnterVal

		rowDeg := ws.rowBasicDeg[:m]
		if leaveVar < n {
			for _, r := range p.rowIdx[p.colPtr[leaveVar]:p.colPtr[leaveVar+1]] {
				rowDeg[r]--
			}
		}
		if enterVar < n {
			for _, r := range p.rowIdx[p.colPtr[enterVar]:p.colPtr[enterVar+1]] {
				rowDeg[r]++
			}
		}
		basis[leaveRow] = enterVar
		ws.basicRow[leaveVar] = -1
		ws.basicRow[enterVar] = leaveRow
		status[enterVar] = statusBasic

		if err := ws.lu.PushEtaSparse(leaveRow, ws.fCol, ws.fColIdx); err != nil {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		} else {
			dualStep := dj[enterVar] / tabRow[enterVar]
			if dualStep != 0 {
				if ws.rhoSparse {
					y := ws.y[:m]
					rho := ws.rho[:m]
					for _, i := range ws.rhoIdx {
						y[i] += dualStep * rho[i]
					}
				} else {
					s.axpy64(ws.y[:m], ws.rho[:m], dualStep)
				}
				if ws.tabRowSparse {
					for _, k := range ws.tabRowIdx {
						dj[k] -= dualStep * tabRow[k]
					}
				} else {
					for _, ch := range ws.tabRowChunks {
						start := ch << 6
						end := min(start+64, n)
						s.axpy64(dj[start:end], tabRow[start:end], -dualStep)
					}
					for _, k := range ws.tabRowIdx {
						if status[k] != statusBasic {
							dj[k] -= dualStep * tabRow[k]
						}
					}
				}
			}
			dj[enterVar] = 0
		}
	}

	return StatusIterationLimit, s.MaxIter
}

func (s *LPSolver) solveTwoPhasePrimalWithLimit(maxIter int) (Status, int) {
	ws := s.ws
	m := s.TotalRows()
	totalIter := 0
	phase1Feasible := false

	if !s.luValid || !ws.lu.valid {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, 0
			}
		}
		s.computePrimalValues()
	}

	blandTrigger := max(blandMinTrigger, blandRowMult*m)
	isBland := func(streak int) bool {
		return streak >= blandTrigger && ((streak-blandTrigger)&blandBurstMask) < blandBurstWindow
	}
	degenStreak := 0
	retriedSmallCrash := false
	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}

		clear(ws.workCost)
		anyInfeasible := false
		totalInfeas := 0.0
		for i := range m {
			bVar := ws.basis[i]
			val := ws.xb[i]
			tol := s.feasTol(bVar)
			if val < ws.lb[bVar]-tol {
				ws.workCost[bVar] = -1.0
				anyInfeasible = true
				totalInfeas += ws.lb[bVar] - val
			} else if val > ws.ub[bVar]+tol {
				ws.workCost[bVar] = 1.0
				anyInfeasible = true
				totalInfeas += val - ws.ub[bVar]
			}
		}

		if !anyInfeasible {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			phase1Feasible = true
			break
		}

		s.computeDualsAndReducedCosts(ws.workCost)
		stepStatus, stepLen := s.primalSimplexStep(true, isBland(degenStreak))
		totalIter++
		if stepLen > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}
		if stepStatus == StatusOptimal {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			if s.verifyPhase1Infeasible(totalInfeas) {
				return StatusInfeasible, totalIter
			}
			if !retriedSmallCrash {
				retriedSmallCrash = true
				s.reinitSmallestMagnitudeCrash()
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				degenStreak = 0
				continue
			}
			return StatusNumericalError, totalIter
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter
		}
	}

	if !phase1Feasible {
		maxViol := 0.0
		for i := range m {
			bVar := ws.basis[i]
			val := ws.xb[i]
			rTol := max(phase1RelaxTol, s.feasTol(bVar))
			if val < ws.lb[bVar]-rTol {
				maxViol = max(maxViol, ws.lb[bVar]-val)
			} else if val > ws.ub[bVar]+rTol {
				maxViol = max(maxViol, val-ws.ub[bVar])
			}
		}
		if maxViol > 0 {
			return StatusIterationLimit, totalIter
		}
	}

	degenStreak = 0
	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}
		s.computeDualsAndReducedCosts(ws.cost)
		stepStatus, stepLen := s.primalSimplexStep(false, isBland(degenStreak))
		totalIter++
		if stepLen > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}
		if stepStatus == StatusOptimal {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			for i := range m {
				bVar := ws.basis[i]
				val := ws.xb[i]
				vTol := max(verifyFeasTol*min(1.0, s.feasTol(bVar)/primalTol), s.feasTol(bVar))
				if math.IsNaN(val) || val < ws.lb[bVar]-vTol || val > ws.ub[bVar]+vTol {
					return StatusNumericalError, totalIter
				}
			}
			return StatusOptimal, totalIter
		}
		if stepStatus == StatusUnbounded && len(ws.lu.etas) > 0 {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
			continue
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter
		}
	}

	return StatusIterationLimit, totalIter
}

func (s *LPSolver) primalSimplexStep(phase1, bland bool) (Status, float64) {
	ws := s.ws
	m := s.TotalRows()
	tot := ws.totVar

	enterVar := -1
	enterDir := 0.0
	dTol := dualTol
	if !phase1 {
		dTol = s.effDualTol
	}
	bestViolation := dTol

	for k := range tot {
		st := ws.status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := ws.dj[k]
		if math.IsNaN(dj) {
			return StatusNumericalError, 0
		}
		switch st {
		case statusAtLower:
			if -dj > bestViolation {
				enterVar = k
				enterDir = 1.0
				if bland {
					goto foundEnter
				}
				bestViolation = -dj
			}
		case statusAtUpper:
			if dj > bestViolation {
				enterVar = k
				enterDir = -1.0
				if bland {
					goto foundEnter
				}
				bestViolation = dj
			}
		case statusSuperBasic:
			if math.Abs(dj) > bestViolation {
				enterVar = k
				if dj < 0 {
					enterDir = 1.0
				} else {
					enterDir = -1.0
				}
				if bland {
					goto foundEnter
				}
				bestViolation = math.Abs(dj)
			}
		}
	}

foundEnter:
	boundWeightedFallback := false
	if enterVar == -1 && !phase1 && !bland {
		bestGain := 10.0 * primalTol
		for k := range tot {
			st := ws.status[k]
			if st != statusAtLower && st != statusAtUpper {
				continue
			}
			span := ws.ub[k] - ws.lb[k]
			if !(span > 0) || math.IsInf(span, 1) {
				continue
			}
			dj := ws.dj[k]
			if st == statusAtLower && dj < -1e-15 {
				if gain := (-dj) * span; gain > bestGain {
					bestGain = gain
					enterVar = k
					enterDir = 1.0
					boundWeightedFallback = true
				}
			} else if st == statusAtUpper && dj > 1e-15 {
				if gain := dj * span; gain > bestGain {
					bestGain = gain
					enterVar = k
					enterDir = -1.0
					boundWeightedFallback = true
				}
			}
		}
	}
	if enterVar == -1 {
		return StatusOptimal, 0
	}

	clear(ws.fCol)
	ws.fColSparse = false
	s.fillColumn(enterVar, ws.fCol)
	ws.lu.FTRAN(ws.fCol)

	var maxTheta float64
	if enterDir > 0 {
		maxTheta = ws.ub[enterVar] - ws.x[enterVar]
	} else {
		maxTheta = ws.x[enterVar] - ws.lb[enterVar]
	}
	leaveRow := -1
	leaveToUpper := false
	bestTheta := maxTheta
	bestPivot := 0.0
	bestLeaveVar := tot
	subTolBlocking := false

	betterLeave := func(theta, absD float64, bVar int) bool {
		if theta < bestTheta-primalRatioTieTol {
			return true
		}
		if math.Abs(theta-bestTheta) <= primalRatioTieTol {
			if bland {
				return leaveRow == -1 || bVar < bestLeaveVar
			}
			return absD > bestPivot
		}
		return false
	}

	for i := range m {
		bVar := ws.basis[i]
		val := ws.xb[i]
		lb, ub := ws.lb[bVar], ws.ub[bVar]
		d := enterDir * ws.fCol[i]
		if math.IsNaN(d) {
			return StatusNumericalError, 0
		}

		if phase1 {
			tol := s.feasTol(bVar)
			if val < lb-tol {
				if d < -pivotTol && !math.IsInf(ub, 1) {
					theta := (ub - val) / (-d)
					if betterLeave(theta, -d, bVar) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = true
						bestPivot = -d
						bestLeaveVar = bVar
					}
				} else if d < -pivotTol {
					theta := (lb - val) / (-d)
					if betterLeave(theta, -d, bVar) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = false
						bestPivot = -d
						bestLeaveVar = bVar
					}
				}
				continue
			} else if val > ub+tol {
				if d > pivotTol && !math.IsInf(lb, -1) {
					theta := (val - lb) / d
					if betterLeave(theta, d, bVar) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = false
						bestPivot = d
						bestLeaveVar = bVar
					}
				} else if d > pivotTol {
					theta := (val - ub) / d
					if betterLeave(theta, d, bVar) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = true
						bestPivot = d
						bestLeaveVar = bVar
					}
				}
				continue
			}
		}

		if d > pivotTol && !math.IsInf(lb, -1) {
			theta := max(0.0, val-lb) / d
			if betterLeave(theta, d, bVar) {
				bestTheta = theta
				leaveRow = i
				leaveToUpper = false
				bestPivot = d
				bestLeaveVar = bVar
			}
		} else if d < -pivotTol && !math.IsInf(ub, 1) {
			theta := max(0.0, ub-val) / (-d)
			if betterLeave(theta, -d, bVar) {
				bestTheta = theta
				leaveRow = i
				leaveToUpper = true
				bestPivot = -d
				bestLeaveVar = bVar
			}
		} else if !phase1 {
			if (d > 0 && !math.IsInf(lb, -1)) || (d < 0 && !math.IsInf(ub, 1)) {
				subTolBlocking = true
			}
		}
	}

	if boundWeightedFallback && !(bestTheta > degenStepTol) {
		return StatusOptimal, 0
	}

	if leaveRow == -1 {
		if math.IsInf(maxTheta, 1) {
			if phase1 || subTolBlocking {
				// An improving phase-1 ray or a phase-2 ray with sub-tolerance blocking pivots
				// is not a valid certificate of infeasibility or unboundedness.
				return StatusNumericalError, 0
			}
			return StatusUnbounded, 0
		}
		step := enterDir * maxTheta
		for i := range m {
			ws.xb[i] -= ws.fCol[i] * step
			ws.x[ws.basis[i]] = ws.xb[i]
		}
		if enterDir > 0 {
			ws.status[enterVar] = statusAtUpper
			ws.x[enterVar] = ws.ub[enterVar]
		} else {
			ws.status[enterVar] = statusAtLower
			ws.x[enterVar] = ws.lb[enterVar]
		}
		return StatusNotSolved, maxTheta
	}

	step := enterDir * bestTheta
	newEnterVal := ws.x[enterVar] + step
	leaveVar := ws.basis[leaveRow]

	for i := range m {
		if i == leaveRow {
			ws.xb[i] = newEnterVal
		} else {
			ws.xb[i] -= ws.fCol[i] * step
		}
		ws.x[ws.basis[i]] = ws.xb[i]
	}

	if ws.lb[leaveVar] == ws.ub[leaveVar] {
		ws.status[leaveVar] = statusFixed
		ws.x[leaveVar] = ws.lb[leaveVar]
	} else if leaveToUpper {
		ws.status[leaveVar] = statusAtUpper
		ws.x[leaveVar] = ws.ub[leaveVar]
	} else {
		ws.status[leaveVar] = statusAtLower
		ws.x[leaveVar] = ws.lb[leaveVar]
	}

	ws.x[enterVar] = newEnterVal
	ws.basis[leaveRow] = enterVar
	ws.basicRow[leaveVar] = -1
	ws.basicRow[enterVar] = leaveRow
	ws.status[enterVar] = statusBasic

	if err := ws.lu.PushEta(leaveRow, ws.fCol); err != nil {
		if err := s.factorizeBasis(); err != nil {
			return StatusNumericalError, bestTheta
		}
		s.computePrimalValues()
	}

	return StatusNotSolved, bestTheta
}

func (s *LPSolver) buildResult(status Status, iters int) LPResult {
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := p.numConstraints

	s.computePrimalValues()
	s.computeDualsAndReducedCosts(ws.cost)

	primal := make([]float64, n)
	dj := make([]float64, n)
	duals := make([]float64, m)

	objMin := 0.0
	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	if s.scaled {
		objS := s.objScale
		for j := range n {
			val := ws.x[j] * s.colScale[j]
			lbj, ubj := s.origLB[j], s.origUB[j]
			if val < lbj && lbj-val <= primalSnapTol {
				val = lbj
			} else if val > ubj && val-ubj <= primalSnapTol {
				val = ubj
			}
			primal[j] = val
			objMin += p.objMin[j] * val
			dj[j] = senseSign * objS * s.invColScale[j] * ws.dj[j]
		}
		for i := range m {
			duals[i] = senseSign * objS * s.rowScale[i] * ws.y[i]
		}
	} else {
		for j := range n {
			val := ws.x[j]
			if val < ws.lb[j] && ws.lb[j]-val <= primalSnapTol {
				val = ws.lb[j]
			} else if val > ws.ub[j] && val-ws.ub[j] <= primalSnapTol {
				val = ws.ub[j]
			}
			primal[j] = val
			objMin += ws.cost[j] * val
			dj[j] = senseSign * ws.dj[j]
		}
		for i := range m {
			duals[i] = senseSign * ws.y[i]
		}
	}

	return LPResult{
		Status:       status,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		Dual:         duals,
		ReducedCosts: dj,
		Iterations:   iters,
	}
}

// verifyDualInfeasibleRow confirms a dual-simplex Farkas infeasibility certificate using an
// un-dropped BTRAN solve so entries below luDropTol or pivotTol cannot produce false infeasibility.
func (s *LPSolver) verifyDualInfeasibleRow(leaveRow int, leaveDir, primalInfeas float64) bool {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	if uint(leaveRow) >= uint(m) {
		return false
	}

	rho := make([]float64, m)
	rho[leaveRow] = 1.0
	ws.lu.BTRANExact(rho)
	for _, v := range rho {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}

	tabExact := make([]float64, tot)
	status := ws.status[:tot]
	baseM := p.numConstraints
	for j := range n {
		if status[j] == statusBasic || status[j] == statusFixed {
			continue
		}
		sum := 0.0
		for q := p.colPtr[j]; q < p.colPtr[j+1]; q++ {
			r := p.rowIdx[q]
			if uint(r) < uint(m) {
				sum += rho[r] * p.colVal[q]
			}
		}
		tabExact[j] = sum
	}
	for e, dr := range s.extraRows {
		yr := rho[baseM+e]
		if yr == 0 {
			continue
		}
		for k, c := range dr.ColIdx {
			if uint(c) < uint(n) && status[c] != statusBasic && status[c] != statusFixed {
				tabExact[c] += yr * dr.Val[k]
			}
		}
	}
	for i := range m {
		slack := n + i
		if status[slack] != statusBasic && status[slack] != statusFixed {
			tabExact[slack] = rho[i]
		}
	}

	maxRelief := 0.0
	for k := range tot {
		st := status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		tab := tabExact[k]
		if math.IsNaN(tab) {
			return false
		}
		if tab == 0 {
			continue
		}
		alphaBar := -leaveDir * tab
		switch st {
		case statusAtLower:
			if alphaBar > 0 {
				span := ws.ub[k] - ws.x[k]
				if math.IsInf(span, 1) {
					return false
				}
				if span > 0 {
					maxRelief += alphaBar * span
				}
			}
		case statusAtUpper:
			if alphaBar < 0 {
				span := ws.x[k] - ws.lb[k]
				if math.IsInf(span, 1) {
					return false
				}
				if span > 0 {
					maxRelief += (-alphaBar) * span
				}
			}
		case statusSuperBasic:
			return false
		}
	}

	tol := s.feasTol(ws.basis[leaveRow])
	return maxRelief < max(0.0, primalInfeas-tol)*0.5
}

// verifyPhase1Infeasible confirms that a Phase I local minimum is a valid Farkas certificate
// by computing exact un-dropped multipliers and bounding total bound-weighted Phase I relief.
func (s *LPSolver) verifyPhase1Infeasible(totalInfeas float64) bool {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar

	y := make([]float64, m)
	for i := range m {
		y[i] = ws.workCost[ws.basis[i]]
	}
	ws.lu.BTRANExact(y)
	for _, v := range y {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}

	status := ws.status[:tot]
	djExact := make([]float64, tot)
	baseM := p.numConstraints
	for j := range n {
		if status[j] == statusBasic || status[j] == statusFixed {
			continue
		}
		dot := 0.0
		for q := p.colPtr[j]; q < p.colPtr[j+1]; q++ {
			r := p.rowIdx[q]
			if uint(r) < uint(m) {
				dot += y[r] * p.colVal[q]
			}
		}
		djExact[j] = ws.workCost[j] - dot
	}
	for e, dr := range s.extraRows {
		yr := y[baseM+e]
		if yr == 0 {
			continue
		}
		for k, c := range dr.ColIdx {
			if uint(c) < uint(n) && status[c] != statusBasic && status[c] != statusFixed {
				djExact[c] -= yr * dr.Val[k]
			}
		}
	}
	for i := range m {
		slack := n + i
		if status[slack] != statusBasic && status[slack] != statusFixed {
			djExact[slack] = ws.workCost[slack] - y[i]
		}
	}

	maxRelief := 0.0
	for k := range tot {
		st := status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := djExact[k]
		if math.IsNaN(dj) {
			return false
		}
		switch st {
		case statusAtLower:
			if dj < -1e-16 {
				span := ws.ub[k] - ws.x[k]
				if math.IsInf(span, 1) {
					return false
				}
				if span > 0 {
					maxRelief += (-dj) * span
				}
			}
		case statusAtUpper:
			if dj > 1e-16 {
				span := ws.x[k] - ws.lb[k]
				if math.IsInf(span, 1) {
					return false
				}
				if span > 0 {
					maxRelief += dj * span
				}
			}
		case statusSuperBasic:
			if dj > 1e-16 || dj < -1e-16 {
				return false
			}
		}
	}

	return maxRelief < 0.5*totalInfeas
}

// reinitSmallestMagnitudeCrash resets the basis to all slacks while placing boxed nonbasic
// structurals at their smallest-magnitude bound so Phase I avoids extreme upper-bound cancellation.
func (s *LPSolver) reinitSmallestMagnitudeCrash() {
	n := s.prob.numVars
	m := s.TotalRows()
	ws := s.ws
	savedLB := slices.Clone(ws.lb[:n])
	savedUB := slices.Clone(ws.ub[:n])
	var savedOrigLB, savedOrigUB []float64
	if s.scaled {
		savedOrigLB = slices.Clone(s.origLB)
		savedOrigUB = slices.Clone(s.origUB)
	}
	s.InitCrashBasis()
	copy(ws.lb[:n], savedLB)
	copy(ws.ub[:n], savedUB)
	if s.scaled {
		copy(s.origLB, savedOrigLB)
		copy(s.origUB, savedOrigUB)
	}
	for j := range n {
		lb, ub := ws.lb[j], ws.ub[j]
		ws.basicRow[j] = -1
		switch {
		case lb == ub:
			ws.status[j] = statusFixed
			ws.x[j] = lb
		case !math.IsInf(lb, -1) && !math.IsInf(ub, 1):
			if math.Abs(lb) <= math.Abs(ub) {
				ws.status[j] = statusAtLower
				ws.x[j] = lb
			} else {
				ws.status[j] = statusAtUpper
				ws.x[j] = ub
			}
		case !math.IsInf(lb, -1):
			ws.status[j] = statusAtLower
			ws.x[j] = lb
		case !math.IsInf(ub, 1):
			ws.status[j] = statusAtUpper
			ws.x[j] = ub
		default:
			ws.status[j] = statusSuperBasic
			ws.x[j] = 0
		}
	}
	for i := range m {
		slack := n + i
		ws.basis[i] = slack
		ws.basicRow[slack] = i
		ws.status[slack] = statusBasic
		ws.dse[i] = 1.0
	}
	s.luValid = false
	s.dualValid = false
}
