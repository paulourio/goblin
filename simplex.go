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
	Prob        *CompiledProblem
	mat         *CompiledProblem
	WS          *Workspace
	ExtraRows   []dynamicRow
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
	if uint(e) < uint(len(s.ExtraRows)) && s.ExtraRows[e].rowScale > 0 {
		return s.ExtraRows[e].rowScale
	}
	return 1.0
}

func (s *LPSolver) unscaledBasicVal(r int) float64 {
	v := s.WS.XB[r]
	if !s.scaled || v == 0 {
		return v
	}
	bVar := s.WS.Basis[r]
	if bVar < len(s.colScale) {
		return v * s.colScale[bVar]
	}
	return v / s.rowScaleOf(bVar-len(s.colScale))
}

func (s *LPSolver) unscaledTabEntry(bVar, k int) float64 {
	alpha := s.WS.TabRow[k]
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
			return s.WS.UB[k]
		}
		return s.WS.LB[k]
	}
	if k < len(s.origLB) {
		if upper {
			return s.origUB[k]
		}
		return s.origLB[k]
	}
	if upper {
		return s.WS.UB[k] / s.rowScaleOf(k-len(s.origLB))
	}
	return s.WS.LB[k] / s.rowScaleOf(k-len(s.origLB))
}

func (s *LPSolver) unscaledRowRHS(rowIdx int) float64 {
	if rowIdx < s.Prob.NumConstraints {
		return s.Prob.RowRHS[rowIdx]
	}
	dr := s.ExtraRows[rowIdx-s.Prob.NumConstraints]
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
	n := p.NumVars
	m := p.NumConstraints
	colTol = make([]float64, n)
	colHasEq = make([]bool, n)

	minColNorm := math.Inf(1)
	maxColNorm := 0.0
	minAbsElem := math.Inf(1)
	for j := range n {
		cn := 0.0
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			av := math.Abs(p.ColVal[k])
			cn = max(cn, av)
			if av > 0 && av < minAbsElem {
				minAbsElem = av
			}
			if p.RowSense[p.RowIdx[k]] == Equal {
				colHasEq[j] = true
			}
		}
		if cn > 0 {
			minColNorm = min(minColNorm, cn)
			maxColNorm = max(maxColNorm, cn)
		}
		colTol[j] = max(primalTol/max(1.0, cn), minScaledPrimalTol)
	}

	minRowNorm := minAbsElem
	maxRowNorm := maxColNorm
	if minAbsElem < scaleMinMatNorm {
		minRowNorm = math.Inf(1)
		maxRowNorm = 0.0
		for i := range m {
			rn := 0.0
			for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
				rn = max(rn, math.Abs(p.RowVal[k]))
			}
			if rn > 0 {
				minRowNorm = min(minRowNorm, rn)
				maxRowNorm = max(maxRowNorm, rn)
			}
		}
	}

	maxObj := 0.0
	for j := range n {
		if c := math.Abs(p.ObjMin[j]); c > maxObj {
			maxObj = c
		}
	}

	needMat := m > 0 && n > 0 && maxRowNorm > 0 &&
		(minRowNorm < scaleMinMatNorm || maxRowNorm > scaleMaxMatNorm || minColNorm < scaleMinMatNorm || maxColNorm > scaleMaxMatNorm)
	needObj := m > 0 && maxObj > 0 && (maxObj < scaleMinObjNorm || maxObj > scaleMaxObjNorm)
	if !needMat && !needObj {
		return p, nil, nil, nil, colTol, nil, colHasEq, 1.0, false
	}

	origColNorm := make([]float64, n)
	for j := range n {
		cn := 0.0
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			cn = max(cn, math.Abs(p.ColVal[k]))
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
				for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
					v := math.Ldexp(math.Abs(p.RowVal[k]), rowExp[i]+colExp[p.ColIdx[k]])
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
				for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
					v := math.Ldexp(math.Abs(p.ColVal[k]), rowExp[p.RowIdx[k]]+colExp[j])
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
		if sc := math.Abs(p.ObjMin[j]) * cs; sc > maxScaledObj {
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
	cp.VarLB = make([]float64, n)
	cp.VarUB = make([]float64, n)
	cp.ObjMin = make([]float64, n)
	for j := range n {
		cp.VarLB[j] = p.VarLB[j] * invColScale[j]
		cp.VarUB[j] = p.VarUB[j] * invColScale[j]
		if p.ObjMin[j] != 0 {
			cp.ObjMin[j] = p.ObjMin[j] * colScale[j] * invObjScale
		}
	}
	cp.RowRHS = make([]float64, m)
	cp.RowLB = make([]float64, m)
	cp.RowUB = make([]float64, m)
	cp.RowVal = make([]float64, len(p.RowVal))
	if len(p.DenseRows) > 0 {
		cp.DenseRows = make([]float64, len(p.DenseRows))
	}
	for i := range m {
		rs := rowScale[i]
		cp.RowRHS[i] = p.RowRHS[i] * rs
		cp.RowLB[i] = p.RowLB[i] * rs
		cp.RowUB[i] = p.RowUB[i] * rs
		var denseRow []float64
		if len(cp.DenseRows) > 0 {
			denseRow = cp.DenseRows[i*cp.RowStride : (i+1)*cp.RowStride]
		}
		for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
			col := p.ColIdx[k]
			sv := p.RowVal[k] * rs * colScale[col]
			cp.RowVal[k] = sv
			if denseRow != nil {
				denseRow[col] = sv
			}
		}
	}
	cp.ColVal = make([]float64, len(p.ColVal))
	for j := range n {
		cs := colScale[j]
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			cp.ColVal[k] = p.ColVal[k] * rowScale[p.RowIdx[k]] * cs
		}
	}
	return &cp, rowScale, colScale, invColScale, colTol, slackTol, colHasEq, objScale, true
}

func (s *LPSolver) timeExpired() bool {
	return stopRequested(s.Deadline, s.Done)
}

// NewLPSolver creates an LPSolver initialized for the given CompiledProblem.
func NewLPSolver(p *CompiledProblem, ws *Workspace) *LPSolver {
	ownsWS := false
	if ws == nil {
		ws = AcquireWorkspace(p.NumVars, p.NumConstraints)
		ownsWS = true
	} else {
		ws.EnsureSize(p.NumVars, p.NumConstraints)
	}
	mat, rowScale, colScale, invColScale, colTol, slackTol, colHasEq, objScale, scaled := buildScaledLP(p)
	effDual := dualTol
	if scaled {
		effDual = max(dualTol/max(1.0, objScale), 1e-11)
	}
	s := &LPSolver{
		Prob:        p,
		mat:         mat,
		WS:          ws,
		MaxIter:     max(defaultMinMaxIter, defaultIterMult*(p.NumVars+p.NumConstraints)),
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
		s.origLB = slices.Clone(p.VarLB)
		s.origUB = slices.Clone(p.VarUB)
	}
	s.InitCrashBasis()
	return s
}

// Close releases any internally pooled Workspace owned by s.
func (s *LPSolver) Close() {
	if s != nil && s.ownsWS && s.WS != nil {
		ReleaseWorkspace(s.WS)
		s.WS = nil
		s.ownsWS = false
	}
}

// TotalRows returns the total number of rows (structural constraints plus active dynamic cuts).
func (s *LPSolver) TotalRows() int {
	return s.Prob.NumConstraints + len(s.ExtraRows)
}

// InitCrashBasis resets the solver to an all-slack basis with nonbasic structural variables placed at cost-favorable bounds.
func (s *LPSolver) InitCrashBasis() {
	p := s.Prob
	mp := s.mat
	n := p.NumVars
	m := s.TotalRows()
	ws := s.WS
	ws.EnsureSize(n, m)
	if s.scaled {
		copy(s.origLB, p.VarLB)
		copy(s.origUB, p.VarUB)
	}

	for j := range n {
		ws.LB[j] = mp.VarLB[j]
		ws.UB[j] = mp.VarUB[j]
		ws.Cost[j] = mp.ObjMin[j]
		ws.BasicRow[j] = -1
		ws.Status[j] = initialNonBasicStatus(ws.LB[j], ws.UB[j], ws.Cost[j])
		ws.X[j] = nonBasicValue(ws.Status[j], ws.LB[j], ws.UB[j])
	}

	for i := range m {
		slackVar := n + i
		ws.Cost[slackVar] = 0
		if i < p.NumConstraints {
			switch p.RowSense[i] {
			case LessEqual:
				ws.LB[slackVar] = 0
				ws.UB[slackVar] = math.Inf(1)
			case GreaterEqual:
				ws.LB[slackVar] = math.Inf(-1)
				ws.UB[slackVar] = 0
			case Equal:
				ws.LB[slackVar] = 0
				ws.UB[slackVar] = 0
			}
		} else {
			ws.LB[slackVar] = 0
			ws.UB[slackVar] = math.Inf(1)
		}
		ws.Basis[i] = slackVar
		ws.BasicRow[slackVar] = i
		ws.Status[slackVar] = statusBasic
		ws.DSE[i] = 1.0
	}
	s.luValid = false
	s.dualValid = false
}

// CopyRootBasisFrom copies the root basis, bounds, duals, reduced costs, and steepest-edge weights from src when neither solver holds extra cut rows.
func (s *LPSolver) CopyRootBasisFrom(src *LPSolver) {
	if len(src.ExtraRows) > 0 || len(s.ExtraRows) > 0 || s.scaled != src.scaled {
		return
	}
	n := s.Prob.NumVars
	m := s.Prob.NumConstraints
	ws := s.WS
	sws := src.WS
	ws.EnsureSize(n, m)
	copy(ws.Cost[:n+m], sws.Cost[:n+m])
	copy(ws.Basis[:m], sws.Basis[:m])
	copy(ws.BasicRow[:n+m], sws.BasicRow[:n+m])
	copy(ws.Status[:n+m], sws.Status[:n+m])
	copy(ws.X[:n+m], sws.X[:n+m])
	copy(ws.LB[:n+m], sws.LB[:n+m])
	copy(ws.UB[:n+m], sws.UB[:n+m])
	copy(ws.DSE[:m], sws.DSE[:m])
	copy(ws.Y[:m], sws.Y[:m])
	copy(ws.Dj[:n+m], sws.Dj[:n+m])
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
	ws := s.WS
	n := s.Prob.NumVars
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
		ws.LB[j] = lbj
		ws.UB[j] = ubj
		st := ws.Status[j]
		if st == statusBasic {
			continue
		}
		hasLB := !math.IsInf(lbj, -1)
		hasUB := !math.IsInf(ubj, 1)
		if lbj == ubj {
			ws.Status[j] = statusFixed
			ws.X[j] = lbj
		} else if hasLB && hasUB {
			if st == statusAtUpper || (st == statusFixed && ws.Dj[j] < 0) {
				ws.Status[j] = statusAtUpper
				ws.X[j] = ubj
			} else {
				ws.Status[j] = statusAtLower
				ws.X[j] = lbj
			}
		} else if hasLB {
			ws.Status[j] = statusAtLower
			ws.X[j] = lbj
		} else if hasUB {
			ws.Status[j] = statusAtUpper
			ws.X[j] = ubj
		} else {
			ws.Status[j] = statusSuperBasic
			ws.X[j] = 0
		}
	}
}

func (s *LPSolver) newCutRow(colIdx []int, val []float64, rhs float64) dynamicRow {
	p := s.Prob
	n := p.NumVars
	kLim := min(len(colIdx), len(val))

	// Check whether colIdx is already strictly ascending with valid indices and above-threshold values.
	canonical := true
	prev := -1
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c <= prev || c >= n || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) <= compileZeroTol {
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
			if math.Abs(sum) <= compileZeroTol {
				if sum != 0 {
					lbc := s.unscaledBound(c, false)
					ubc := s.unscaledBound(c, true)
					if lo := min(sum*lbc, sum*ubc); !math.IsInf(lo, 0) && !math.IsNaN(lo) {
						rhs -= lo
					}
				}
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
		dr.Dense = make([]float64, max(n, p.RowStride))
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
	n := mp.NumVars
	s.ExtraRows = append(s.ExtraRows, rows...)
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

	ws := s.WS
	oldM := ws.M
	newM := oldM + len(rows)

	savedBasis := slices.Clone(ws.Basis[:oldM])
	savedStatus := slices.Clone(ws.Status[:n+oldM])
	savedLB := slices.Clone(ws.LB[:n])
	savedUB := slices.Clone(ws.UB[:n])
	savedDSE := slices.Clone(ws.DSE[:oldM])

	ws.EnsureSize(n, newM)
	for j := range n {
		ws.LB[j] = savedLB[j]
		ws.UB[j] = savedUB[j]
		ws.Cost[j] = mp.ObjMin[j]
		ws.BasicRow[j] = -1
		ws.Status[j] = savedStatus[j]
	}
	for i := range oldM {
		slackOld := n + i
		ws.Cost[slackOld] = 0
		ws.LB[slackOld], ws.UB[slackOld] = s.slackBounds(i)
		ws.BasicRow[slackOld] = -1
		ws.Status[slackOld] = savedStatus[slackOld]
	}

	for i := range oldM {
		bVar := savedBasis[i]
		ws.Basis[i] = bVar
		ws.BasicRow[bVar] = i
		ws.Status[bVar] = statusBasic
		ws.DSE[i] = savedDSE[i]
	}
	for i := oldM; i < newM; i++ {
		newSlack := n + i
		ws.Cost[newSlack] = 0
		ws.LB[newSlack] = 0
		ws.UB[newSlack] = math.Inf(1)
		ws.Status[newSlack] = statusBasic
		ws.Basis[i] = newSlack
		ws.BasicRow[newSlack] = i
		ws.DSE[i] = 1.0
	}

	for k := range n + newM {
		if ws.Status[k] != statusBasic {
			ws.X[k] = nonBasicValue(ws.Status[k], ws.LB[k], ws.UB[k])
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
	ws := s.WS
	n := s.Prob.NumVars
	m := s.TotalRows()
	snap := lpSnapshot{
		numExtra: len(s.ExtraRows),
		basis:    slices.Clone(ws.Basis[:m]),
		status:   slices.Clone(ws.Status[:n+m]),
		lb:       slices.Clone(ws.LB[:n]),
		ub:       slices.Clone(ws.UB[:n]),
		xb:       slices.Clone(ws.XB[:m]),
		dse:      slices.Clone(ws.DSE[:m]),
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
	n := mp.NumVars
	baseM := mp.NumConstraints
	s.ExtraRows = s.ExtraRows[:snap.numExtra]
	s.varTol = s.varTol[:0]
	if s.scaled {
		if len(snap.origLB) == n {
			copy(s.origLB, snap.origLB)
			copy(s.origUB, snap.origUB)
		}
		s.slackTol = s.slackTol[:baseM]
		for _, dr := range s.ExtraRows {
			rs := dr.rowScale
			if rs <= 0 {
				rs = 1.0
			}
			s.slackTol = append(s.slackTol, max(primalTol*min(1.0, rs), minRuizPrimalTol))
		}
	}
	m := s.TotalRows()
	ws := s.WS
	ws.EnsureSize(n, m)
	for j := range n {
		ws.LB[j] = snap.lb[j]
		ws.UB[j] = snap.ub[j]
		ws.Cost[j] = mp.ObjMin[j]
		ws.BasicRow[j] = -1
	}
	for i := range m {
		slack := n + i
		ws.Cost[slack] = 0
		ws.LB[slack], ws.UB[slack] = s.slackBounds(i)
		ws.BasicRow[slack] = -1
	}
	copy(ws.Status[:n+m], snap.status)
	copy(ws.XB[:m], snap.xb)
	for i := range m {
		bVar := snap.basis[i]
		ws.Basis[i] = bVar
		ws.BasicRow[bVar] = i
		ws.DSE[i] = snap.dse[i]
		ws.X[bVar] = snap.xb[i]
	}
	for k := range n + m {
		if ws.Status[k] != statusBasic {
			ws.X[k] = nonBasicValue(ws.Status[k], ws.LB[k], ws.UB[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

// PurgeSlackCuts drops dynamic cut rows whose slack variable is basic with primal value
// above slackTol, compacting ExtraRows and basis arrays in place while preserving primal
// feasibility, dual optimality, and steepest-edge weights.
func (s *LPSolver) PurgeSlackCuts(slackTol float64) int {
	if len(s.ExtraRows) == 0 {
		return 0
	}
	p := s.Prob
	n := p.NumVars
	baseM := p.NumConstraints
	oldExtra := len(s.ExtraRows)
	oldM := baseM + oldExtra
	ws := s.WS

	dropExtra := make([]bool, oldExtra)
	dropped := 0
	for e := range oldExtra {
		slackVar := n + baseM + e
		if ws.Status[slackVar] == statusBasic {
			bSlot := ws.BasicRow[slackVar]
			thresh := slackTol * s.rowScaleOf(baseM+e)
			if bSlot >= 0 && bSlot < oldM && ws.Basis[bSlot] == slackVar && ws.XB[bSlot] > thresh {
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
			s.ExtraRows[newExtra] = s.ExtraRows[e]
		}
		rowMap[baseM+e] = baseM + newExtra
		newExtra++
	}
	clear(s.ExtraRows[newExtra:])
	s.ExtraRows = s.ExtraRows[:newExtra]
	newM := baseM + newExtra

	newStatus := make([]varBasisStatus, n+newM)
	copy(newStatus[:n+baseM], ws.Status[:n+baseM])
	for e := range oldExtra {
		if mappedRow := rowMap[baseM+e]; mappedRow >= 0 {
			newStatus[n+mappedRow] = ws.Status[n+baseM+e]
		}
	}

	newBasis := make([]int, 0, newM)
	newXB := make([]float64, 0, newM)
	newDSE := make([]float64, 0, newM)
	for slot := range oldM {
		bVar := ws.Basis[slot]
		if bVar >= n {
			mappedRow := rowMap[bVar-n]
			if mappedRow < 0 {
				continue
			}
			bVar = n + mappedRow
		}
		newBasis = append(newBasis, bVar)
		newXB = append(newXB, ws.XB[slot])
		newDSE = append(newDSE, ws.DSE[slot])
	}

	s.restore(lpSnapshot{
		numExtra: newExtra,
		basis:    newBasis,
		status:   newStatus,
		lb:       slices.Clone(ws.LB[:n]),
		ub:       slices.Clone(ws.UB[:n]),
		xb:       newXB,
		dse:      newDSE,
	})
	return dropped
}

// slackBounds returns the bounds of the slack of row i (a_i x + s_i = rhs_i).
func (s *LPSolver) slackBounds(i int) (lb, ub float64) {
	if i >= s.Prob.NumConstraints {
		return 0, math.Inf(1)
	}
	switch s.Prob.RowSense[i] {
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
	n := p.NumVars
	if colVar >= n {
		row := colVar - n
		dst[row] = 1.0
		return
	}
	for k := p.ColPtr[colVar]; k < p.ColPtr[colVar+1]; k++ {
		dst[p.RowIdx[k]] = p.ColVal[k]
	}
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
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
	n := p.NumVars
	idx = idx[:0]
	if colVar >= n {
		row := colVar - n
		dst[row] = 1.0
		return append(idx, row)
	}
	for k := p.ColPtr[colVar]; k < p.ColPtr[colVar+1]; k++ {
		r := p.RowIdx[k]
		dst[r] = p.ColVal[k]
		idx = append(idx, r)
	}
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
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
	n := p.NumVars
	if colVar >= n {
		dst[colVar-n] += scale
		return
	}
	for k := p.ColPtr[colVar]; k < p.ColPtr[colVar+1]; k++ {
		dst[p.RowIdx[k]] += scale * p.ColVal[k]
	}
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
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
	n := p.NumVars
	if colVar >= n {
		rows = append(rows, colVar-n)
		vals = append(vals, 1.0)
		return rows, vals
	}
	start, end := p.ColPtr[colVar], p.ColPtr[colVar+1]
	rows = append(rows, p.RowIdx[start:end]...)
	vals = append(vals, p.ColVal[start:end]...)
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
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
	if i < s.mat.NumConstraints {
		return s.mat.RowRHS[i]
	}
	return s.ExtraRows[i-s.mat.NumConstraints].RHS
}

// Solve optimizes the current continuous LP relaxation via revised dual simplex with a two-phase primal simplex fallback.
func (s *LPSolver) Solve() LPResult {
	p := s.Prob
	if p == nil || p.InvalidErr != nil {
		return LPResult{Status: StatusUndefined}
	}
	if s.timeExpired() {
		return LPResult{Status: StatusTimeLimit}
	}
	n := p.NumVars
	m := s.TotalRows()
	ws := s.WS

	for j := range n {
		if s.unscaledBound(j, false) > s.unscaledBound(j, true)+primalTol {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		for i := range p.NumConstraints {
			if 0 < p.RowLB[i]-primalTol || 0 > p.RowUB[i]+primalTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		for _, dr := range s.ExtraRows {
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
			Objective:    p.ObjConstant,
			Primal:       []float64{},
			Dual:         make([]float64, p.NumConstraints),
			ReducedCosts: []float64{},
		}
	}
	if m == 0 {
		return s.solveUnconstrained()
	}

	copy(ws.WorkCost, ws.Cost)
	if !s.luValid || ws.LU.m != m || ws.LU.NeedsRefactor() {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err := s.factorizeBasis(); err != nil {
				return LPResult{Status: StatusNumericalError}
			}
		}
		s.computeDualsAndReducedCosts(ws.WorkCost)
		s.dualValid = true
	} else if !s.dualValid {
		s.computeDualsAndReducedCosts(ws.WorkCost)
		s.dualValid = true
	}

	dualFeasible := s.stabilizeNonBasicBounds()
	s.computePrimalValues()

	totalIter := 0

	if dualFeasible {
		status, iters := s.runDualSimplexLoop(ws.WorkCost)
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
	n := s.Prob.NumVars
	var savedLB, savedUB []float64
	if s.scaled {
		savedLB = slices.Clone(s.origLB)
		savedUB = slices.Clone(s.origUB)
	} else {
		savedLB = slices.Clone(s.WS.LB[:n])
		savedUB = slices.Clone(s.WS.UB[:n])
	}
	s.InitCrashBasis()
	s.SetStructuralBounds(savedLB, savedUB)
}

func (s *LPSolver) solveUnconstrained() LPResult {
	p := s.Prob
	n := p.NumVars
	ws := s.WS
	primal := make([]float64, n)
	dj := make([]float64, n)
	objMin := 0.0
	senseSign := 1.0
	if p.Sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		c := ws.Cost[j]
		dj[j] = senseSign * c
		if c > 0 {
			if math.IsInf(ws.LB[j], -1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.LB[j]
		} else if c < 0 {
			if math.IsInf(ws.UB[j], 1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.UB[j]
		} else {
			if !math.IsInf(ws.LB[j], -1) && !math.IsInf(ws.UB[j], 1) {
				if ws.LB[j] <= 0 && 0 <= ws.UB[j] {
					primal[j] = 0
				} else {
					primal[j] = ws.LB[j]
				}
			} else if !math.IsInf(ws.LB[j], -1) {
				primal[j] = max(0.0, ws.LB[j])
			} else if !math.IsInf(ws.UB[j], 1) {
				primal[j] = min(0.0, ws.UB[j])
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
	ws := s.WS
	m := s.TotalRows()
	err := ws.LU.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		return s.appendBasisColumn(ws.Basis[col], rows, vals)
	})
	s.luValid = err == nil
	return err
}

func (s *LPSolver) computeDualsAndReducedCosts(cost []float64) {
	p := s.mat
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar

	cost = cost[:tot]
	basis := ws.Basis[:m]
	y := ws.Y[:m]
	status := ws.Status[:tot]
	dj := ws.Dj[:tot]

	for i, bVar := range basis {
		y[i] = cost[bVar]
	}
	ws.LU.BTRAN(ws.Y)

	baseM := p.NumConstraints
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	colVal := p.ColVal[:len(rowIdx)]
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
	for r, dr := range s.ExtraRows {
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
	ws := s.WS
	tot := ws.TotVar
	status := ws.Status[:tot]
	lbSlice := ws.LB[:tot]
	ubSlice := ws.UB[:tot]
	djSlice := ws.Dj[:tot]
	xSlice := ws.X[:tot]
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
	ws := s.WS
	status := ws.Status[:ws.TotVar]
	djSlice := ws.Dj[:len(status)]
	vTol := verifyFeasTol
	if s.scaled {
		vTol = max(verifyFeasTol/max(1.0, s.objScale), 1e-10)
	}
	for k, st := range status {
		dj := djSlice[k]
		if math.IsNaN(dj) || (st == statusAtLower && dj < -vTol) || (st == statusAtUpper && dj > vTol) || (st == statusSuperBasic && (dj > vTol || dj < -vTol)) {
			return false
		}
	}
	return true
}

func (s *LPSolver) computePrimalValues() {
	p := s.mat
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
	status := ws.Status[:tot]
	xSlice := ws.X[:tot]
	xbSlice := ws.XB[:m]
	basis := ws.Basis[:m]

	for i := range m {
		xb := s.rowRHS(i)
		slackVar := n + i
		if status[slackVar] != statusBasic {
			xb -= xSlice[slackVar]
		}
		xbSlice[i] = xb
	}

	baseM := p.NumConstraints
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	colVal := p.ColVal[:len(rowIdx)]
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
	for r, dr := range s.ExtraRows {
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

	ws.LU.FTRAN(ws.XB)
	for i, bVar := range basis {
		if uint(bVar) < uint(tot) {
			xSlice[bVar] = xbSlice[i]
		}
	}
}

// ComputeTableauRow evaluates row r of B^{-1} [A I] into WS.Rho and WS.TabRow, tracking either sparse nonbasic support or 64-column chunks.
func (s *LPSolver) ComputeTableauRow(r int) {
	p := s.mat
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
	rho := ws.Rho[:m]
	tabRow := ws.TabRow[:tot]
	tabMark := ws.TabRowMark[:tot]
	status := ws.Status[:tot]
	basis := ws.Basis[:m]
	numChunks := (n + 63) >> 6
	chunkMark := ws.TabRowChunkMark[:numChunks]

	if ws.RhoSparse {
		for _, i := range ws.RhoIdx {
			if uint(i) < uint(m) {
				rho[i] = 0
			}
		}
		ws.RhoIdx = ws.RhoIdx[:0]
	} else {
		clear(rho)
	}
	rho[r] = 1.0
	var unitHint [1]int
	unitHint[0] = r
	ws.RhoIdx = ws.LU.BTRANSparseHint(ws.Rho, unitHint[:], ws.RhoIdx)
	ws.RhoSparse = true

	if ws.TabRowSparse {
		for _, k := range ws.TabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
				tabMark[k] = false
			}
		}
		ws.TabRowIdx = ws.TabRowIdx[:0]
	} else {
		for _, ch := range ws.TabRowChunks {
			if uint(ch) < uint(numChunks) {
				chunkMark[ch] = false
				start := ch << 6
				end := min(start+64, n)
				clear(tabRow[start:end])
			}
		}
		ws.TabRowChunks = ws.TabRowChunks[:0]
		for _, k := range ws.TabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
			}
		}
		ws.TabRowIdx = ws.TabRowIdx[:0]
	}
	baseM := p.NumConstraints

	if n > 0 && n <= DenseThreshold && len(p.DenseRows) == baseM*p.RowStride {
		ws.TabRowSparse = false
		chunks := ws.TabRowChunks[:0]
		for ch := range numChunks {
			chunkMark[ch] = true
			chunks = append(chunks, ch)
		}
		ws.TabRowChunks = chunks
		stride := p.RowStride
		tabStruct := tabRow[:n]
		sparseRowCap := n >> 2
		if simd.UsingHardwareSIMDWith(s.DisableSIMD) && n >= 32 {
			sparseRowCap = min(8, sparseRowCap)
		}
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				if end-start <= sparseRowCap {
					cols := p.ColIdx[start:end]
					vals := p.RowVal[start:end]
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
						}
					}
				} else {
					s.axpy64(tabStruct, p.DenseRows[i*stride:][:n], yi)
				}
			} else {
				dr := s.ExtraRows[i-baseM]
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
		ws.TabRowIdx = tabIdx
		return
	}

	totalNNZ := 0
	for _, i := range ws.RhoIdx {
		if i < baseM {
			totalNNZ += p.RowPtr[i+1] - p.RowPtr[i]
		} else {
			totalNNZ += len(s.ExtraRows[i-baseM].ColIdx)
		}
	}

	if totalNNZ >= 64 {
		ws.TabRowSparse = false
		tabStruct := tabRow[:n]
		chunks := ws.TabRowChunks[:0]
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			var cols []int
			var vals []float64
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				cols = p.ColIdx[start:end]
				vals = p.RowVal[start:end]
			} else {
				dr := s.ExtraRows[i-baseM]
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
		ws.TabRowChunks = chunks
		ws.TabRowIdx = tabIdx
	} else {
		ws.TabRowSparse = true
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				cols := p.ColIdx[start:end]
				vals := p.RowVal[start:end]
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
				dr := s.ExtraRows[i-baseM]
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
		ws.TabRowIdx = tabIdx
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
	ws := s.WS
	cands := ws.BFRTCands
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
	if (k0 < len(s.colHasEq) && s.colHasEq[k0]) || ws.Status[k0] == statusSuperBasic || math.IsInf(ws.LB[k0], -1) || math.IsInf(ws.UB[k0], 1) {
		return -1
	}
	maxFlip0 := c0.absA * (ws.UB[k0] - ws.LB[k0])
	if maxFlip0 <= 0 || primalInfeas-maxFlip0 <= primalTol {
		return -1
	}

	n := s.Prob.NumVars
	tot := ws.TotVar
	tabRow := ws.TabRow[:tot]
	status := ws.Status[:tot]
	dj := ws.Dj[:tot]
	cands = cands[:0]
	if !ws.TabRowSparse {
		for _, ch := range ws.TabRowChunks {
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
	for _, k := range ws.TabRowIdx {
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
	ws.BFRTCands = cands
	if len(cands) <= 1 {
		return -1
	}

	lbSlice := ws.LB[:tot]
	ubSlice := ws.UB[:tot]
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
	for stopIdx < len(cands)-1 && len(ws.FlipCols) < maxBFRTFlips {
		c := cands[stopIdx]
		if math.IsInf(c.maxFlip, 1) || c.maxFlip <= 0 || c.absA < minStaysFeasPivot || remInfeas-c.maxFlip <= primalTol {
			break
		}
		ws.FlipCols = append(ws.FlipCols, c.col)
		remInfeas -= c.maxFlip
		stopIdx++
	}
	if len(ws.FlipCols) == 0 {
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
		ws.FlipCols = ws.FlipCols[:0]
		return -1
	}
	// Only flip variables whose breakpoint is strictly before enterRatio so every
	// flipped variable moves strictly into the dual-feasible interior.
	keep := 0
	for i := 0; i < len(ws.FlipCols); i++ {
		if cands[i].ratio < enterRatio-pivotTieRelTol {
			ws.FlipCols[keep] = ws.FlipCols[i]
			keep++
		}
	}
	ws.FlipCols = ws.FlipCols[:keep]
	if keep == 0 {
		return -1
	}
	return enterVar
}

func (s *LPSolver) recomputeRowBasicDeg() {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	rowDeg := ws.RowBasicDeg[:m]
	clear(rowDeg)
	basis := ws.Basis[:m]
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	for _, bVar := range basis {
		if uint(bVar) < uint(n) {
			for _, r := range rowIdx[colPtr[bVar]:colPtr[bVar+1]] {
				rowDeg[r]++
			}
		}
	}
}

func (s *LPSolver) colBasisLoad(k int, dynamic bool) int {
	p := s.Prob
	if uint(k) >= uint(p.NumVars) {
		return 0
	}
	start, end := p.ColPtr[k], p.ColPtr[k+1]
	if !dynamic {
		return end - start
	}
	rowDeg := s.WS.RowBasicDeg
	load := 0
	for _, r := range p.RowIdx[start:end] {
		load += rowDeg[r]
	}
	return load
}

// applyDualBoundFlips flips all columns selected by selectDualBFRT to their opposite bounds and updates basic primals via a batched sparse FTRAN.
func (s *LPSolver) applyDualBoundFlips(leaveRow int, leaveDir float64) float64 {
	ws := s.WS
	m := s.TotalRows()
	tot := ws.TotVar
	lb := ws.LB[:tot]
	ub := ws.UB[:tot]
	status := ws.Status[:tot]
	xb := ws.XB[:m]
	basis := ws.Basis[:m]

	if ws.TauSparse {
		for _, i := range ws.TauIdx {
			if uint(i) < uint(m) {
				ws.Tau[i] = 0
			}
		}
		ws.TauIdx = ws.TauIdx[:0]
	} else {
		clear(ws.Tau)
	}
	for _, fk := range ws.FlipCols {
		lbf, ubf := lb[fk], ub[fk]
		var deltaX float64
		if status[fk] == statusAtLower {
			status[fk] = statusAtUpper
			ws.X[fk] = ubf
			deltaX = ubf - lbf
		} else {
			status[fk] = statusAtLower
			ws.X[fk] = lbf
			deltaX = lbf - ubf
		}
		s.addScaledColumn(fk, deltaX, ws.Tau)
	}
	ws.TauIdx = ws.LU.FTRANSparse(ws.Tau, ws.TauIdx)
	for _, i := range ws.TauIdx {
		xb[i] -= ws.Tau[i]
		ws.Tau[i] = 0
		ws.X[basis[i]] = xb[i]
	}
	ws.TauIdx = ws.TauIdx[:0]
	ws.TauSparse = true
	bVar := basis[leaveRow]
	if leaveDir > 0 {
		return lb[bVar] - xb[leaveRow]
	}
	return xb[leaveRow] - ub[bVar]
}

// updateDualSteepestEdge updates row steepest-edge weights via the Forrest-Goldfarb recurrence over FColIdx.
func (s *LPSolver) updateDualSteepestEdge(leaveRow int, pivElem float64) {
	ws := s.WS
	m := s.TotalRows()
	dse := ws.DSE[:m]
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
	fCol := ws.FCol[:m]
	if len(ws.FColIdx) > 1 {
		tau := ws.Tau[:m]
		if ws.TauSparse {
			for _, i := range ws.TauIdx {
				if uint(i) < uint(m) {
					tau[i] = 0
				}
			}
			ws.TauIdx = ws.TauIdx[:0]
		} else {
			clear(tau)
		}
		if ws.RhoSparse {
			rho := ws.Rho[:m]
			for _, i := range ws.RhoIdx {
				tau[i] = rho[i]
			}
			ws.TauIdx = ws.LU.FTRANSparseHint(tau, ws.RhoIdx, ws.TauIdx)
		} else {
			copy(tau, ws.Rho[:m])
			ws.TauIdx = ws.LU.FTRANSparse(ws.Tau, ws.TauIdx)
		}
		ws.TauSparse = true
		for _, i := range ws.FColIdx {
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
	ws := s.WS
	n := s.Prob.NumVars
	tot := ws.TotVar
	thetaMax = math.Inf(1)
	minRatio = math.Inf(1)
	cands := ws.BFRTCands[:0]
	tabRow := ws.TabRow[:tot]
	status := ws.Status[:tot]
	dj := ws.Dj[:tot]

	if !ws.TabRowSparse {
		for _, ch := range ws.TabRowChunks {
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
						ws.BFRTCands = cands
						return 0, 0, false, true
					}
					d = max(0.0, d)
					if bland {
						minRatio = min(minRatio, d/absA)
					}
					if d <= (thetaMax+2*minScaledPrimalTol)*absA {
						thetaMax = min(thetaMax, (d+harrisTol)/absA)
						cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
					}
				} else {
					blk, nan := dualSubTolCheck(tab, alphaBar, st)
					if nan {
						ws.BFRTCands = cands
						return 0, 0, false, true
					}
					subTol = subTol || blk
				}
			}
		}
	}
	for _, k := range ws.TabRowIdx {
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
				ws.BFRTCands = cands
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
			ws.BFRTCands = cands
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
			if ratio := (d + harrisTol) / absA; ratio < thetaMax {
				thetaMax = ratio
			}
			cands = append(cands, bfrtCand{col: k, ratio: r0, absA: absA})
		}
	}
	ws.BFRTCands = cands
	return thetaMax, minRatio, subTol, false
}

func (s *LPSolver) runDualSimplexLoop(cost []float64) (Status, int) {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
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
		if ws.LU.NeedsRefactor() {
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

		basis := ws.Basis[:m]
		xb := ws.XB[:m]
		dse := ws.DSE[:m]
		lb := ws.LB[:tot]
		ub := ws.UB[:tot]
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
			if len(ws.LU.etas) > 0 {
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
		cands := ws.BFRTCands
		tabRow := ws.TabRow[:tot]
		status := ws.Status[:tot]
		dj := ws.Dj[:tot]

		if math.IsInf(thetaMax, 1) {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err == nil {
					s.computeDualsAndReducedCosts(cost)
					s.computePrimalValues()
					continue
				}
			}
			if hadSubTolBlocking {
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
			if hadSubTolBlocking {
				return StatusNumericalError, iter
			}
			return StatusInfeasible, iter
		}

		ws.FlipCols = ws.FlipCols[:0]
		if !bland && !bestStaysFeas && thetaMax > harrisTol && !math.IsInf(lb[enterVar], -1) && !math.IsInf(ub[enterVar], 1) {
			if bfrtEnter := s.selectDualBFRT(primalInfeas, leaveDir); bfrtEnter != -1 {
				enterVar = bfrtEnter
			}
		}

		if ws.FColSparse {
			for _, i := range ws.FColIdx {
				ws.FCol[i] = 0
			}
			ws.FColIdx = ws.FColIdx[:0]
		} else {
			clear(ws.FCol)
		}
		ws.FColIdx = s.fillColumnSparse(enterVar, ws.FCol, ws.FColIdx)
		ws.FColIdx = ws.LU.FTRANSparseHint(ws.FCol, ws.FColIdx, ws.FColIdx)
		ws.FColSparse = true

		pivElem := ws.FCol[leaveRow]
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

		if len(ws.FlipCols) > 0 {
			primalInfeas = s.applyDualBoundFlips(leaveRow, leaveDir)
		}
		if math.Abs(dj[enterVar]/tabElem) > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}

		s.updateDualSteepestEdge(leaveRow, pivElem)
		fCol := ws.FCol[:m]

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
		oldEnterVal := ws.X[enterVar]
		newEnterVal := oldEnterVal + primalStep

		xSlice := ws.X[:tot]
		for _, i := range ws.FColIdx {
			if i != leaveRow && uint(i) < uint(m) {
				xb[i] -= fCol[i] * primalStep
				xSlice[basis[i]] = xb[i]
			}
		}
		xb[leaveRow] = newEnterVal
		xSlice[leaveVar] = newLeaveVal
		xSlice[enterVar] = newEnterVal

		rowDeg := ws.RowBasicDeg[:m]
		if leaveVar < n {
			for _, r := range p.RowIdx[p.ColPtr[leaveVar]:p.ColPtr[leaveVar+1]] {
				rowDeg[r]--
			}
		}
		if enterVar < n {
			for _, r := range p.RowIdx[p.ColPtr[enterVar]:p.ColPtr[enterVar+1]] {
				rowDeg[r]++
			}
		}
		basis[leaveRow] = enterVar
		ws.BasicRow[leaveVar] = -1
		ws.BasicRow[enterVar] = leaveRow
		status[enterVar] = statusBasic

		if err := ws.LU.PushEtaSparse(leaveRow, ws.FCol, ws.FColIdx); err != nil {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		} else {
			dualStep := dj[enterVar] / tabRow[enterVar]
			if dualStep != 0 {
				if ws.RhoSparse {
					y := ws.Y[:m]
					rho := ws.Rho[:m]
					for _, i := range ws.RhoIdx {
						y[i] += dualStep * rho[i]
					}
				} else {
					s.axpy64(ws.Y[:m], ws.Rho[:m], dualStep)
				}
				if ws.TabRowSparse {
					for _, k := range ws.TabRowIdx {
						dj[k] -= dualStep * tabRow[k]
					}
				} else {
					for _, ch := range ws.TabRowChunks {
						start := ch << 6
						end := min(start+64, n)
						s.axpy64(dj[start:end], tabRow[start:end], -dualStep)
					}
					for _, k := range ws.TabRowIdx {
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
	ws := s.WS
	m := s.TotalRows()
	totalIter := 0
	phase1Feasible := false

	if !s.luValid || !ws.LU.valid {
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
	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter
		}
		if ws.LU.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}

		clear(ws.WorkCost)
		anyInfeasible := false
		for i := range m {
			bVar := ws.Basis[i]
			val := ws.XB[i]
			tol := s.feasTol(bVar)
			if val < ws.LB[bVar]-tol {
				ws.WorkCost[bVar] = -1.0
				anyInfeasible = true
			} else if val > ws.UB[bVar]+tol {
				ws.WorkCost[bVar] = 1.0
				anyInfeasible = true
			}
		}

		if !anyInfeasible {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			phase1Feasible = true
			break
		}

		s.computeDualsAndReducedCosts(ws.WorkCost)
		stepStatus, stepLen := s.primalSimplexStep(true, isBland(degenStreak))
		totalIter++
		if stepLen > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}
		if stepStatus == StatusOptimal {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			return StatusInfeasible, totalIter
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter
		}
	}

	if !phase1Feasible {
		maxViol := 0.0
		for i := range m {
			bVar := ws.Basis[i]
			val := ws.XB[i]
			rTol := max(phase1RelaxTol, s.feasTol(bVar))
			if val < ws.LB[bVar]-rTol {
				maxViol = max(maxViol, ws.LB[bVar]-val)
			} else if val > ws.UB[bVar]+rTol {
				maxViol = max(maxViol, val-ws.UB[bVar])
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
		if ws.LU.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}
		s.computeDualsAndReducedCosts(ws.Cost)
		stepStatus, stepLen := s.primalSimplexStep(false, isBland(degenStreak))
		totalIter++
		if stepLen > degenStepTol {
			degenStreak = 0
		} else {
			degenStreak++
		}
		if stepStatus == StatusOptimal {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			for i := range m {
				bVar := ws.Basis[i]
				val := ws.XB[i]
				vTol := max(verifyFeasTol*min(1.0, s.feasTol(bVar)/primalTol), s.feasTol(bVar))
				if math.IsNaN(val) || val < ws.LB[bVar]-vTol || val > ws.UB[bVar]+vTol {
					return StatusNumericalError, totalIter
				}
			}
			return StatusOptimal, totalIter
		}
		if stepStatus == StatusUnbounded && len(ws.LU.etas) > 0 {
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
	ws := s.WS
	m := s.TotalRows()
	tot := ws.TotVar

	enterVar := -1
	enterDir := 0.0
	dTol := dualTol
	if !phase1 {
		dTol = s.effDualTol
	}
	bestViolation := dTol

	for k := range tot {
		st := ws.Status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := ws.Dj[k]
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
	if enterVar == -1 {
		return StatusOptimal, 0
	}

	clear(ws.FCol)
	ws.FColSparse = false
	s.fillColumn(enterVar, ws.FCol)
	ws.LU.FTRAN(ws.FCol)

	var maxTheta float64
	if enterDir > 0 {
		maxTheta = ws.UB[enterVar] - ws.X[enterVar]
	} else {
		maxTheta = ws.X[enterVar] - ws.LB[enterVar]
	}
	leaveRow := -1
	leaveToUpper := false
	bestTheta := maxTheta
	bestPivot := 0.0
	bestLeaveVar := tot

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
		bVar := ws.Basis[i]
		val := ws.XB[i]
		lb, ub := ws.LB[bVar], ws.UB[bVar]
		d := enterDir * ws.FCol[i]
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
		}
	}

	if leaveRow == -1 {
		if math.IsInf(maxTheta, 1) {
			if phase1 {
				// An improving phase-1 ray that no infeasible basic variable blocks
				// cannot occur in exact arithmetic (an infeasible basic would block at its
				// bound); it signals pivots below tolerance, not a certificate of an empty polyhedron.
				return StatusNumericalError, 0
			}
			return StatusUnbounded, 0
		}
		step := enterDir * maxTheta
		for i := range m {
			ws.XB[i] -= ws.FCol[i] * step
			ws.X[ws.Basis[i]] = ws.XB[i]
		}
		if enterDir > 0 {
			ws.Status[enterVar] = statusAtUpper
			ws.X[enterVar] = ws.UB[enterVar]
		} else {
			ws.Status[enterVar] = statusAtLower
			ws.X[enterVar] = ws.LB[enterVar]
		}
		return StatusNotSolved, maxTheta
	}

	step := enterDir * bestTheta
	newEnterVal := ws.X[enterVar] + step
	leaveVar := ws.Basis[leaveRow]

	for i := range m {
		if i == leaveRow {
			ws.XB[i] = newEnterVal
		} else {
			ws.XB[i] -= ws.FCol[i] * step
		}
		ws.X[ws.Basis[i]] = ws.XB[i]
	}

	if ws.LB[leaveVar] == ws.UB[leaveVar] {
		ws.Status[leaveVar] = statusFixed
		ws.X[leaveVar] = ws.LB[leaveVar]
	} else if leaveToUpper {
		ws.Status[leaveVar] = statusAtUpper
		ws.X[leaveVar] = ws.UB[leaveVar]
	} else {
		ws.Status[leaveVar] = statusAtLower
		ws.X[leaveVar] = ws.LB[leaveVar]
	}

	ws.X[enterVar] = newEnterVal
	ws.Basis[leaveRow] = enterVar
	ws.BasicRow[leaveVar] = -1
	ws.BasicRow[enterVar] = leaveRow
	ws.Status[enterVar] = statusBasic

	if err := ws.LU.PushEta(leaveRow, ws.FCol); err != nil {
		if err := s.factorizeBasis(); err != nil {
			return StatusNumericalError, bestTheta
		}
		s.computePrimalValues()
	}

	return StatusNotSolved, bestTheta
}

func (s *LPSolver) buildResult(status Status, iters int) LPResult {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := p.NumConstraints

	s.computePrimalValues()
	s.computeDualsAndReducedCosts(ws.Cost)

	primal := make([]float64, n)
	dj := make([]float64, n)
	duals := make([]float64, m)

	objMin := 0.0
	senseSign := 1.0
	if p.Sense == Maximize {
		senseSign = -1.0
	}
	if s.scaled {
		objS := s.objScale
		for j := range n {
			val := ws.X[j] * s.colScale[j]
			lbj, ubj := s.origLB[j], s.origUB[j]
			if val < lbj && lbj-val <= primalSnapTol {
				val = lbj
			} else if val > ubj && val-ubj <= primalSnapTol {
				val = ubj
			}
			primal[j] = val
			objMin += p.ObjMin[j] * val
			dj[j] = senseSign * objS * s.invColScale[j] * ws.Dj[j]
		}
		for i := range m {
			duals[i] = senseSign * objS * s.rowScale[i] * ws.Y[i]
		}
	} else {
		for j := range n {
			val := ws.X[j]
			if val < ws.LB[j] && ws.LB[j]-val <= primalSnapTol {
				val = ws.LB[j]
			} else if val > ws.UB[j] && val-ws.UB[j] <= primalSnapTol {
				val = ws.UB[j]
			}
			primal[j] = val
			objMin += ws.Cost[j] * val
			dj[j] = senseSign * ws.Dj[j]
		}
		for i := range m {
			duals[i] = senseSign * ws.Y[i]
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
