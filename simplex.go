package goblin

import (
	"cmp"
	"math"
	"math/big"
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
	verifyFeasTol      = 1e-5
	minBFRTFlipPivot   = 1e-5
	primalRatioTieTol  = 1e-12
	primalSnapTol      = 1e-6
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
	unitRoundoff64     = 0x1p-53
	twoProdExactMin    = 0x1p-969
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
	extraColPtr []int
	extraRowIdx []int
	extraColVal []float64
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
	// colHasEq reports whether structural column j has a nonzero in any equation row.
	colHasEq      []bool
	rowScale      []float64
	colScale      []float64
	invColScale   []float64
	colExp        []int
	origLB        []float64
	origUB        []float64
	objScale      float64
	effDualTol    float64
	varTolValid   bool
	extraCSCDirty bool
	invalidBounds bool
	invalidCut    bool
	infeasibleCut bool
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
	ws := s.ws
	if s.varTolValid && len(ws.varTol) == tot {
		return ws.varTol
	}
	if cap(ws.varTol) < tot {
		ws.varTol = make([]float64, tot)
	} else {
		ws.varTol = ws.varTol[:tot]
	}
	n := copy(ws.varTol, s.colTol)
	if s.scaled {
		n += copy(ws.varTol[n:], s.slackTol)
	}
	for k := n; k < tot; k++ {
		ws.varTol[k] = primalTol
	}
	s.varTolValid = true
	return ws.varTol
}

// ensureExtraCSC refreshes the cached column-compressed index over active dynamic cut rows when extraRows has changed.
func (s *LPSolver) ensureExtraCSC() {
	if s.extraCSCDirty {
		s.rebuildExtraCSC()
	}
}

// rebuildExtraCSC constructs the column-compressed index over extraRows using a two-pass counting sort.
func (s *LPSolver) rebuildExtraCSC() {
	n := s.mat.numVars
	if len(s.extraRows) == 0 {
		s.extraRowIdx = s.extraRowIdx[:0]
		s.extraColVal = s.extraColVal[:0]
		if len(s.extraColPtr) > 0 {
			clear(s.extraColPtr)
		}
		s.extraCSCDirty = false
		return
	}
	if cap(s.extraColPtr) < n+1 {
		s.extraColPtr = make([]int, n+1)
	} else {
		s.extraColPtr = s.extraColPtr[:n+1]
		clear(s.extraColPtr)
	}
	ptr := s.extraColPtr
	total := 0
	for _, dr := range s.extraRows {
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(n) && vals[k] != 0 {
				ptr[c+1]++
				total++
			}
		}
	}
	for j := 1; j <= n; j++ {
		ptr[j] += ptr[j-1]
	}
	if total == 0 {
		s.extraRowIdx = s.extraRowIdx[:0]
		s.extraColVal = s.extraColVal[:0]
		s.extraCSCDirty = false
		return
	}
	if cap(s.extraRowIdx) < total {
		s.extraRowIdx = make([]int, total)
	} else {
		s.extraRowIdx = s.extraRowIdx[:total]
	}
	if cap(s.extraColVal) < total {
		s.extraColVal = make([]float64, total)
	} else {
		s.extraColVal = s.extraColVal[:total]
	}
	rows := s.extraRowIdx
	outVals := s.extraColVal
	baseM := s.mat.numConstraints
	for e, dr := range s.extraRows {
		row := baseM + e
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(n) {
				if v := vals[k]; v != 0 {
					idx := ptr[c]
					rows[idx] = row
					outVals[idx] = v
					ptr[c] = idx + 1
				}
			}
		}
	}
	copy(ptr[1:n+1], ptr[:n])
	ptr[0] = 0
	s.extraCSCDirty = false
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

func (s *LPSolver) unscaledTabEntryWithErr(bVar, k int) (alpha, errBound float64) {
	alpha = s.ws.tabRow[k]
	if k < len(s.ws.tabRowErr) {
		errBound = s.ws.tabRowErr[k]
	}
	if !s.scaled {
		return alpha, errBound
	}
	cb := s.colScale[bVar]
	var scale float64
	if k < len(s.colScale) {
		scale = cb * s.invColScale[k]
	} else {
		scale = cb * s.rowScaleOf(k-len(s.colScale))
	}
	alpha *= scale
	if errBound > 0 {
		errBound = math.Nextafter(errBound*scale, math.Inf(1))
	}
	return alpha, errBound
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

func buildScaledLP(p *CompiledProblem) (mat *CompiledProblem, rowScale, colScale, invColScale, colTol, slackTol []float64, colExp []int, colHasEq []bool, objScale float64, scaled bool) {
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
		return p, nil, nil, nil, colTol, nil, nil, colHasEq, 1.0, false
	}

	origColTol := slices.Clone(colTol)
	unscaledFallback := func() (*CompiledProblem, []float64, []float64, []float64, []float64, []float64, []int, []bool, float64, bool) {
		return p, nil, nil, nil, origColTol, nil, nil, colHasEq, 1.0, false
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
	colExp = make([]int, n)
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
				if rmax > 0 && !math.IsInf(rmax, 1) {
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
				if cmax > 0 && !math.IsInf(cmax, 1) {
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
		if sc := math.Ldexp(math.Abs(p.objMin[j]), colExp[j]); sc > maxScaledObj {
			maxScaledObj = sc
		}
	}
	if math.IsInf(maxScaledObj, 0) || math.IsNaN(maxScaledObj) {
		return unscaledFallback()
	}

	objExp := 0
	objScale = 1.0
	if maxScaledObj > 0 && (maxScaledObj < 1e-3 || maxScaledObj > 1e3) {
		_, oe := math.Frexp(maxScaledObj)
		objExp = min(max(oe-1, -ruizScaleMaxExp), ruizObjMaxExp)
		objScale = math.Ldexp(1.0, objExp)
	}

	scaleFiniteOK := func(orig, scaledVal float64) bool {
		if math.IsInf(orig, 0) {
			return math.IsInf(scaledVal, 0) && math.Signbit(orig) == math.Signbit(scaledVal)
		}
		if math.IsNaN(scaledVal) || math.IsInf(scaledVal, 0) {
			return false
		}
		return orig == 0 || scaledVal != 0
	}

	cp := *p
	cp.varLB = make([]float64, n)
	cp.varUB = make([]float64, n)
	cp.objMin = make([]float64, n)
	for j := range n {
		slb := math.Ldexp(p.varLB[j], -colExp[j])
		sub := math.Ldexp(p.varUB[j], -colExp[j])
		if !scaleFiniteOK(p.varLB[j], slb) || !scaleFiniteOK(p.varUB[j], sub) {
			return unscaledFallback()
		}
		cp.varLB[j] = slb
		cp.varUB[j] = sub
		if p.objMin[j] != 0 {
			so := math.Ldexp(p.objMin[j], colExp[j]-objExp)
			if !scaleFiniteOK(p.objMin[j], so) {
				return unscaledFallback()
			}
			cp.objMin[j] = so
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
		re := rowExp[i]
		srhs := math.Ldexp(p.rowRHS[i], re)
		srlb := math.Ldexp(p.rowLB[i], re)
		srub := math.Ldexp(p.rowUB[i], re)
		if !scaleFiniteOK(p.rowRHS[i], srhs) || !scaleFiniteOK(p.rowLB[i], srlb) || !scaleFiniteOK(p.rowUB[i], srub) {
			return unscaledFallback()
		}
		cp.rowRHS[i] = srhs
		cp.rowLB[i] = srlb
		cp.rowUB[i] = srub
		var denseRow []float64
		if len(cp.denseRows) > 0 {
			denseRow = cp.denseRows[i*cp.rowStride : (i+1)*cp.rowStride]
		}
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			sv := math.Ldexp(p.rowVal[k], re+colExp[col])
			if !scaleFiniteOK(p.rowVal[k], sv) {
				return unscaledFallback()
			}
			cp.rowVal[k] = sv
			if denseRow != nil {
				denseRow[col] = sv
			}
		}
	}
	cp.colVal = make([]float64, len(p.colVal))
	for j := range n {
		ce := colExp[j]
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			sv := math.Ldexp(p.colVal[k], rowExp[p.rowIdx[k]]+ce)
			if !scaleFiniteOK(p.colVal[k], sv) {
				return unscaledFallback()
			}
			cp.colVal[k] = sv
		}
	}
	return &cp, rowScale, colScale, invColScale, colTol, slackTol, colExp, colHasEq, objScale, true
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
	mat, rowScale, colScale, invColScale, colTol, slackTol, colExp, colHasEq, objScale, scaled := buildScaledLP(p)
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
		colExp:      colExp,
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

// Close releases any internally pooled Workspace owned by s and detaches the workspace reference.
func (s *LPSolver) Close() {
	if s == nil {
		return
	}
	if s.ownsWS && s.ws != nil {
		ReleaseWorkspace(s.ws)
		s.ownsWS = false
	}
	s.ws = nil
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
	s.invalidBounds = false
	s.varTolValid = false
	s.extraCSCDirty = len(s.extraRows) > 0
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
	if s.prob.numVars != src.prob.numVars || s.prob.numConstraints != src.prob.numConstraints {
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
	s.invalidBounds = src.invalidBounds
	s.invalidCut = src.invalidCut
	s.infeasibleCut = src.infeasibleCut
	s.varTolValid = false
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

func scaleBoundFiniteOK(orig, scaledVal float64) bool {
	if math.IsInf(orig, 0) {
		return math.IsInf(scaledVal, 0) && math.Signbit(orig) == math.Signbit(scaledVal)
	}
	if math.IsNaN(scaledVal) || math.IsInf(scaledVal, 0) {
		return false
	}
	return orig == 0 || scaledVal != 0
}

// unscaleToOriginal converts a Ruiz-scaled LPSolver back to original problem units in place,
// preserving the current basis and dynamic cut rows when extreme bounds or cuts exceed scaled float64 range.
func (s *LPSolver) unscaleToOriginal() {
	if !s.scaled || s.prob == nil || s.ws == nil {
		return
	}
	p := s.prob
	n := p.numVars
	m := s.TotalRows()
	ws := s.ws

	for j := range n {
		cn := 0.0
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			cn = max(cn, math.Abs(p.colVal[k]))
		}
		s.colTol[j] = max(primalTol/max(1.0, cn), minScaledPrimalTol)
	}
	for idx := range s.extraRows {
		dr := &s.extraRows[idx]
		if dr.OrigVal != nil {
			copy(dr.Val, dr.OrigVal)
			dr.OrigVal = nil
		}
		dr.RHS = dr.OrigRHS
		dr.rowScale = 1.0
	}
	if len(s.extraRows) > 0 {
		s.extraCSCDirty = true
	}
	for j := range n {
		ws.cost[j] = p.objMin[j]
		ws.lb[j] = s.origLB[j]
		ws.ub[j] = s.origUB[j]
		if ws.status[j] != statusBasic {
			ws.x[j] = nonBasicValue(ws.status[j], ws.lb[j], ws.ub[j])
		}
	}
	for i := range m {
		slackVar := n + i
		ws.cost[slackVar] = 0
		if ws.status[slackVar] != statusBasic {
			ws.x[slackVar] = nonBasicValue(ws.status[slackVar], ws.lb[slackVar], ws.ub[slackVar])
		}
		ws.dse[i] = 1.0
	}
	s.mat = p
	s.scaled = false
	s.slackTol = nil
	s.varTolValid = false
	s.rowScale = nil
	s.colScale = nil
	s.invColScale = nil
	s.colExp = nil
	s.origLB = nil
	s.origUB = nil
	s.objScale = 1.0
	s.effDualTol = dualTol
	s.luValid = false
	s.dualValid = false
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
	needUnscale := false
	for j := range n {
		lbj, ubj := lb[j], ub[j]
		if math.IsNaN(lbj) || math.IsNaN(ubj) || math.IsInf(lbj, 1) || math.IsInf(ubj, -1) {
			s.invalidBounds = true
			return
		}
		if s.scaled && !needUnscale {
			slb := math.Ldexp(lbj, -s.colExp[j])
			sub := math.Ldexp(ubj, -s.colExp[j])
			if !scaleBoundFiniteOK(lbj, slb) || !scaleBoundFiniteOK(ubj, sub) {
				needUnscale = true
			}
		}
	}
	if needUnscale {
		s.unscaleToOriginal()
	}
	s.invalidBounds = false
	if s.scaled {
		copy(s.origLB, lb[:n])
		copy(s.origUB, ub[:n])
	}
	for j := range n {
		lbj, ubj := lb[j], ub[j]
		if s.scaled {
			lbj = math.Ldexp(lbj, -s.colExp[j])
			ubj = math.Ldexp(ubj, -s.colExp[j])
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

type cutBuildStatus uint8

const (
	cutBuildOK cutBuildStatus = iota
	cutBuildSkip
	cutBuildInvalid
	cutBuildInfeasible
)

func (s *LPSolver) buildCutRow(colIdx []int, val []float64, rhs float64) (dynamicRow, cutBuildStatus) {
	if math.IsNaN(rhs) {
		return dynamicRow{}, cutBuildInvalid
	}
	p := s.prob
	n := p.numVars
	kLim := min(len(colIdx), len(val))

	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c < 0 || c >= n {
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return dynamicRow{}, cutBuildInvalid
		}
	}
	if math.IsInf(rhs, 1) {
		return dynamicRow{}, cutBuildSkip
	}
	if math.IsInf(rhs, -1) {
		return dynamicRow{}, cutBuildInfeasible
	}

	// Check whether colIdx is already strictly ascending with valid indices and nonzero values.
	canonical := true
	prev := -1
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c <= prev || c >= n || v == 0 {
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
			if c < 0 || c >= n || v == 0 {
				continue
			}
			pairs = append(pairs, termPair{col: c, val: v})
		}
		slices.SortFunc(pairs, func(a, b termPair) int { return cmp.Compare(a.col, b.col) })
		cols = make([]int, 0, len(pairs))
		vals = make([]float64, 0, len(pairs))
		for i := 0; i < len(pairs); {
			start := i
			c := pairs[i].col
			sum := pairs[i].val
			for i++; i < len(pairs) && pairs[i].col == c; i++ {
				sum += pairs[i].val
			}
			if i-start > 1 {
				var w wideDotAcc
				for k := start; k < i; k++ {
					w.addVal(pairs[k].val)
				}
				sum = w.finish(math.NaN())
				if math.IsNaN(sum) || math.IsInf(sum, 0) {
					return dynamicRow{}, cutBuildInvalid
				}
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
		maxAbs := 0.0
		maxScaledExp := math.MinInt
		overflowedPre := false
		rExpLo := -2048
		rExpHi := 2048
		for k, c := range cols {
			sv := math.Ldexp(vals[k], s.colExp[c])
			_, ve := math.Frexp(math.Abs(vals[k]))
			se := (ve - 1) + s.colExp[c]
			if se > maxScaledExp {
				maxScaledExp = se
			}
			rExpLo = max(rExpLo, -1021-se)
			rExpHi = min(rExpHi, 1023-se)
			if math.IsInf(sv, 0) || math.IsNaN(sv) {
				overflowedPre = true
			} else if av := math.Abs(sv); av > maxAbs {
				maxAbs = av
			}
		}
		if rhs != 0 {
			_, re := math.Frexp(math.Abs(rhs))
			rhsE := re - 1
			rExpLo = max(rExpLo, -1021-rhsE)
			rExpHi = min(rExpHi, 1023-rhsE)
		}
		if rExpLo > rExpHi {
			s.unscaleToOriginal()
		} else {
			rExp := 0
			if overflowedPre {
				rExp = min(max(-maxScaledExp, -ruizScaleMaxExp), ruizScaleMaxExp)
			} else if maxAbs > 0 {
				rExp = pow2EquilibrateExp(maxAbs, ruizScaleMaxExp)
			}
			rExp = min(max(rExp, rExpLo), rExpHi)
			rs := math.Ldexp(1.0, rExp)
			scaledRHS := math.Ldexp(rhs, rExp)
			scaleOK := rs > 0 && !math.IsInf(rs, 0) && !math.IsNaN(rs) && scaleBoundFiniteOK(rhs, scaledRHS)
			if scaleOK {
				scaledVals := make([]float64, len(vals))
				for k, c := range cols {
					sv := math.Ldexp(vals[k], rExp+s.colExp[c])
					if !scaleBoundFiniteOK(vals[k], sv) {
						scaleOK = false
						break
					}
					scaledVals[k] = sv
				}
				if scaleOK {
					origVal = vals
					vals = scaledVals
					rhs = scaledRHS
					rowScale = rs
				}
			}
			if !scaleOK {
				s.unscaleToOriginal()
			}
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
	return dr, cutBuildOK
}

func (s *LPSolver) newCutRow(colIdx []int, val []float64, rhs float64) dynamicRow {
	dr, _ := s.buildCutRow(colIdx, val, rhs)
	return dr
}

// AddCut appends the cutting plane sum_k val[k]*x[colIdx[k]] <= rhs to the continuous LP.
// The new row's slack enters the basis, so the current basis stays primal-degenerate-feasible
// for all existing rows and the next Solve warm-starts with the dual simplex.
func (s *LPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	if s == nil || s.prob == nil || s.ws == nil {
		return
	}
	dr, st := s.buildCutRow(colIdx, val, rhs)
	switch st {
	case cutBuildOK:
		s.addCutRows([]dynamicRow{dr})
	case cutBuildSkip:
		return
	case cutBuildInfeasible:
		s.infeasibleCut = true
	case cutBuildInvalid:
		s.invalidCut = true
	}
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
	if !s.scaled {
		for idx := range rows {
			dr := &rows[idx]
			if dr.OrigVal != nil {
				copy(dr.Val, dr.OrigVal)
				dr.OrigVal = nil
				dr.RHS = dr.OrigRHS
				dr.rowScale = 1.0
			}
		}
	}
	s.extraRows = append(s.extraRows, rows...)
	s.extraCSCDirty = true
	s.varTolValid = false
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
	numExtra      int
	basis         []int
	status        []varBasisStatus
	lb, ub        []float64
	origLB        []float64
	origUB        []float64
	xb            []float64
	origXB        []float64
	dse           []float64
	invalidBounds bool
	invalidCut    bool
	infeasibleCut bool
}

func (s *LPSolver) snapshot() lpSnapshot {
	ws := s.ws
	n := s.prob.numVars
	m := s.TotalRows()
	snap := lpSnapshot{
		numExtra:      len(s.extraRows),
		basis:         slices.Clone(ws.basis[:m]),
		status:        slices.Clone(ws.status[:n+m]),
		lb:            slices.Clone(ws.lb[:n]),
		ub:            slices.Clone(ws.ub[:n]),
		xb:            slices.Clone(ws.xb[:m]),
		dse:           slices.Clone(ws.dse[:m]),
		invalidBounds: s.invalidBounds,
		invalidCut:    s.invalidCut,
		infeasibleCut: s.infeasibleCut,
	}
	if s.scaled {
		snap.origLB = slices.Clone(s.origLB)
		snap.origUB = slices.Clone(s.origUB)
		snap.origXB = make([]float64, m)
		for i := range m {
			snap.origXB[i] = s.unscaledBasicVal(i)
		}
	}
	return snap
}

// restore rolls the solver back to snap, discarding any cut rows added after it was taken.
func (s *LPSolver) restore(snap lpSnapshot) {
	mp := s.mat
	n := mp.numVars
	baseM := mp.numConstraints
	s.extraRows = s.extraRows[:snap.numExtra]
	s.extraCSCDirty = true
	s.invalidBounds = snap.invalidBounds
	s.invalidCut = snap.invalidCut
	s.infeasibleCut = snap.infeasibleCut
	s.varTolValid = false
	restLB, restUB := snap.lb, snap.ub
	unscaledSinceSnap := false
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
	} else if len(snap.origLB) == n {
		restLB, restUB = snap.origLB, snap.origUB
		unscaledSinceSnap = true
	}
	m := s.TotalRows()
	ws := s.ws
	ws.ensureSize(n, m)
	for j := range n {
		ws.lb[j] = restLB[j]
		ws.ub[j] = restUB[j]
		ws.cost[j] = mp.objMin[j]
		ws.basicRow[j] = -1
	}
	for i := range m {
		slack := n + i
		ws.cost[slack] = 0
		ws.lb[slack], ws.ub[slack] = s.slackBounds(i)
		ws.basicRow[slack] = -1
	}
	restXB := snap.xb
	if unscaledSinceSnap && len(snap.origXB) >= m {
		restXB = snap.origXB
	}
	copy(ws.status[:n+m], snap.status)
	copy(ws.xb[:m], restXB[:m])
	for i := range m {
		bVar := snap.basis[i]
		ws.basis[i] = bVar
		ws.basicRow[bVar] = i
		if unscaledSinceSnap {
			ws.dse[i] = 1.0
		} else {
			ws.dse[i] = snap.dse[i]
		}
		ws.x[bVar] = restXB[i]
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
	if s == nil || s.prob == nil || s.ws == nil || !s.luValid || len(s.extraRows) == 0 {
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
	s.extraCSCDirty = true
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
	if len(s.extraRows) > 0 {
		s.ensureExtraCSC()
		for k := s.extraColPtr[colVar]; k < s.extraColPtr[colVar+1]; k++ {
			dst[s.extraRowIdx[k]] = s.extraColVal[k]
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
	if len(s.extraRows) > 0 {
		s.ensureExtraCSC()
		for k := s.extraColPtr[colVar]; k < s.extraColPtr[colVar+1]; k++ {
			r := s.extraRowIdx[k]
			dst[r] = s.extraColVal[k]
			idx = append(idx, r)
		}
	}
	return idx
}

func (s *LPSolver) addScaledColumn(colVar int, scale float64, dst []float64, idx []int) []int {
	if scale == 0 {
		return idx
	}
	p := s.mat
	n := p.numVars
	if colVar >= n {
		r := colVar - n
		if dst[r] == 0 {
			idx = append(idx, r)
		}
		dst[r] += scale
		return idx
	}
	for k := p.colPtr[colVar]; k < p.colPtr[colVar+1]; k++ {
		r := p.rowIdx[k]
		if dst[r] == 0 {
			idx = append(idx, r)
		}
		dst[r] += scale * p.colVal[k]
	}
	if len(s.extraRows) > 0 {
		s.ensureExtraCSC()
		for k := s.extraColPtr[colVar]; k < s.extraColPtr[colVar+1]; k++ {
			r := s.extraRowIdx[k]
			if dst[r] == 0 {
				idx = append(idx, r)
			}
			dst[r] += scale * s.extraColVal[k]
		}
	}
	return idx
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
	if len(s.extraRows) > 0 {
		s.ensureExtraCSC()
		es, ee := s.extraColPtr[colVar], s.extraColPtr[colVar+1]
		rows = append(rows, s.extraRowIdx[es:ee]...)
		vals = append(vals, s.extraColVal[es:ee]...)
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
	if s == nil || s.prob == nil || s.prob.invalidErr != nil || s.ws == nil || s.invalidBounds || s.invalidCut {
		return LPResult{Status: StatusUndefined}
	}
	if s.infeasibleCut {
		return LPResult{Status: StatusInfeasible}
	}
	p := s.prob
	if s.timeExpired() {
		return LPResult{Status: StatusTimeLimit}
	}
	n := p.numVars
	m := s.TotalRows()
	ws := s.ws

	for j := range n {
		lbj := s.unscaledBound(j, false)
		ubj := s.unscaledBound(j, true)
		if math.IsNaN(lbj) || math.IsNaN(ubj) || math.IsInf(lbj, 1) || math.IsInf(ubj, -1) {
			return LPResult{Status: StatusUndefined}
		}
		if lbj > ubj+primalTol {
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
	dualFresh := false
	if !s.luValid || ws.lu.m != m || ws.lu.NeedsRefactor() {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err := s.factorizeBasis(); err != nil {
				return LPResult{Status: StatusNumericalError}
			}
		}
		s.computeDualsAndReducedCosts(ws.workCost)
		s.dualValid = true
		dualFresh = true
	} else if !s.dualValid {
		s.computeDualsAndReducedCosts(ws.workCost)
		s.dualValid = true
		dualFresh = true
	}

	dualFeasible := s.stabilizeNonBasicBounds()
	s.computePrimalValues()
	primalFresh := true

	totalIter := 0

	if dualFeasible {
		status, iters, pFresh, dFresh := s.runDualSimplexLoop(ws.workCost, primalFresh, dualFresh)
		totalIter += iters
		switch status {
		case StatusOptimal:
			s.dualValid = true
			if res, ok := s.finalizeOptimalResult(totalIter, pFresh, dFresh); ok {
				return res
			}
			// Primal residual failed after dual termination; fall through to primal recovery.
		case StatusInfeasible, StatusIterationLimit, StatusTimeLimit:
			s.dualValid = false
			return LPResult{Status: status, Iterations: totalIter}
		}
	}

	s.dualValid = false
	remIter := s.MaxIter - totalIter
	if remIter <= 0 {
		return LPResult{Status: StatusIterationLimit, Iterations: totalIter}
	}
	status, iters, pFresh, dFresh := s.solveTwoPhasePrimalWithLimit(remIter)
	totalIter += iters
	if status != StatusOptimal {
		return LPResult{Status: status, Iterations: totalIter}
	}
	s.dualValid = true
	if res, ok := s.finalizeOptimalResult(totalIter, pFresh, dFresh); ok {
		return res
	}
	return LPResult{Status: StatusNumericalError, Iterations: totalIter}
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
	}
	objMin := evalLinearSum(0, p.objMin, primal, n)
	userObj := p.EvaluateObjective(primal)
	if math.IsNaN(objMin) || math.IsInf(objMin, 0) || math.IsNaN(userObj) || math.IsInf(userObj, 0) {
		return LPResult{Status: StatusNumericalError}
	}
	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    userObj,
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
	ws.lu.BTRANExact(ws.y)

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
		if yr == 0 {
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

func (s *LPSolver) colDualNoiseTol(k int, cost []float64) float64 {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	y := ws.y[:m]
	if k >= n {
		r := k - n
		if uint(r) < uint(m) {
			ck := cost[k]
			if ck == 0 || y[r] == 0 {
				return 0
			}
			return 2.0 * unitRoundoff64 * (math.Abs(ck) + math.Abs(y[r]))
		}
		return 0
	}
	l1 := 0.0
	for q := p.colPtr[k]; q < p.colPtr[k+1]; q++ {
		r := p.rowIdx[q]
		if uint(r) < uint(m) {
			l1 += math.Abs(y[r] * p.colVal[q])
		}
	}
	if len(s.extraRows) > 0 {
		s.ensureExtraCSC()
		for q := s.extraColPtr[k]; q < s.extraColPtr[k+1]; q++ {
			r := s.extraRowIdx[q]
			if uint(r) < uint(m) {
				l1 += math.Abs(y[r] * s.extraColVal[q])
			}
		}
	}
	return 128.0 * unitRoundoff64 * l1
}

func (s *LPSolver) hasInfiniteStructuralBound() bool {
	n := s.mat.numVars
	status := s.ws.status[:n]
	lb := s.ws.lb[:n]
	ub := s.ws.ub[:n]
	for j := range n {
		if status[j] != statusFixed && (math.IsInf(lb[j], -1) || math.IsInf(ub[j], 1)) {
			return true
		}
	}
	return false
}

// slackCanMoveUnbounded reports whether row r's slack variable s_r = b_r - a_r^T x
// can move without bound toward +Inf (toPosInf == true) or -Inf (toPosInf == false)
// under the current non-fixed structural variable bounds.
func (s *LPSolver) slackCanMoveUnbounded(r int, toPosInf bool) bool {
	p := s.mat
	ws := s.ws
	n := p.numVars
	baseM := p.numConstraints
	status := ws.status[:n]
	lb := ws.lb[:n]
	ub := ws.ub[:n]
	checkTerm := func(c int, a float64) bool {
		if a == 0 || status[c] == statusFixed {
			return false
		}
		if toPosInf {
			return (a > 0 && math.IsInf(lb[c], -1)) || (a < 0 && math.IsInf(ub[c], 1))
		}
		return (a > 0 && math.IsInf(ub[c], 1)) || (a < 0 && math.IsInf(lb[c], -1))
	}
	if r < baseM {
		st, en := p.rowPtr[r], p.rowPtr[r+1]
		cols := p.colIdx[st:en]
		vals := p.rowVal[st:en]
		for k, c := range cols {
			if checkTerm(c, vals[k]) {
				return true
			}
		}
		return false
	}
	e := r - baseM
	if uint(e) >= uint(len(s.extraRows)) {
		return false
	}
	dr := s.extraRows[e]
	for k, c := range dr.ColIdx {
		if checkTerm(c, dr.Val[k]) {
			return true
		}
	}
	return false
}

func (s *LPSolver) stabilizeNonBasicBounds() bool {
	ws := s.ws
	n := s.mat.numVars
	tot := ws.totVar
	status := ws.status[:tot]
	lbSlice := ws.lb[:tot]
	ubSlice := ws.ub[:tot]
	djSlice := ws.dj[:tot]
	xSlice := ws.x[:tot]
	costSlice := ws.workCost[:tot]
	dTol := s.effDualTol
	dualFeas := true
	hasInfStruct := s.hasInfiniteStructuralBound()
	needRefinedCheck := false

	for k, st := range status {
		if st == statusBasic {
			continue
		}
		lb, ub, dj := lbSlice[k], ubSlice[k], djSlice[k]
		if math.IsNaN(dj) {
			dualFeas = false
			continue
		}
		if st == statusFixed {
			xSlice[k] = lb
			continue
		}
		hasLB := !math.IsInf(lb, -1)
		hasUB := !math.IsInf(ub, 1)

		if hasLB && hasUB {
			span := ub - lb
			if dj < -dTol || (dj < -1e-14 && (-dj)*span > primalTol && -dj > s.colDualNoiseTol(k, costSlice)) {
				status[k] = statusAtUpper
				xSlice[k] = ub
			} else if dj > dTol || (dj > 1e-14 && dj*span > primalTol && dj > s.colDualNoiseTol(k, costSlice)) {
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
		switch st {
		case statusAtLower:
			if dj < -dTol {
				dualFeas = false
			} else if hasInfStruct && (k < n || s.slackCanMoveUnbounded(k-n, true)) {
				if dj < 0 {
					needRefinedCheck = true
				} else if cNoise := s.colDualNoiseTol(k, costSlice); cNoise > 0 && dj <= cNoise {
					needRefinedCheck = true
				}
			}
		case statusAtUpper:
			if dj > dTol {
				dualFeas = false
			} else if hasInfStruct && (k < n || s.slackCanMoveUnbounded(k-n, false)) {
				if dj > 0 {
					needRefinedCheck = true
				} else if cNoise := s.colDualNoiseTol(k, costSlice); cNoise > 0 && -dj <= cNoise {
					needRefinedCheck = true
				}
			}
		case statusSuperBasic:
			if dj > dTol || dj < -dTol {
				dualFeas = false
			} else if hasInfStruct {
				if dj != 0 || s.colDualNoiseTol(k, costSlice) > 0 {
					needRefinedCheck = true
				}
			}
		}
	}
	if dualFeas && needRefinedCheck && s.hasImprovingUnboundedColumn(costSlice) {
		return false
	}
	return dualFeas
}

func (s *LPSolver) isDualFeasible() bool {
	ws := s.ws
	n := s.mat.numVars
	tot := ws.totVar
	status := ws.status[:tot]
	djSlice := ws.dj[:tot]
	lbSlice := ws.lb[:tot]
	ubSlice := ws.ub[:tot]
	costSlice := ws.cost[:tot]
	vTol := s.effDualTol
	hasInfStruct := s.hasInfiniteStructuralBound()
	needRefinedCheck := false
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
			if math.IsInf(ubSlice[k], 1) {
				if hasInfStruct && (k < n || s.slackCanMoveUnbounded(k-n, true)) {
					if dj < 0 {
						needRefinedCheck = true
					} else if cNoise := s.colDualNoiseTol(k, costSlice); cNoise > 0 && dj <= cNoise {
						needRefinedCheck = true
					}
				}
			} else if dj < -1e-14 && (-dj)*(ubSlice[k]-lbSlice[k]) > primalTol && -dj > s.colDualNoiseTol(k, costSlice) {
				return false
			}
		case statusAtUpper:
			if dj > vTol {
				return false
			}
			if math.IsInf(lbSlice[k], -1) {
				if hasInfStruct && (k < n || s.slackCanMoveUnbounded(k-n, false)) {
					if dj > 0 {
						needRefinedCheck = true
					} else if cNoise := s.colDualNoiseTol(k, costSlice); cNoise > 0 && -dj <= cNoise {
						needRefinedCheck = true
					}
				}
			} else if dj > 1e-14 && dj*(ubSlice[k]-lbSlice[k]) > primalTol && dj > s.colDualNoiseTol(k, costSlice) {
				return false
			}
		case statusSuperBasic:
			if dj > vTol || dj < -vTol {
				return false
			}
			if hasInfStruct && (dj != 0 || s.colDualNoiseTol(k, costSlice) > 0) {
				needRefinedCheck = true
			}
		}
	}
	if needRefinedCheck && s.hasImprovingUnboundedColumn(costSlice) {
		return false
	}
	return true
}

func (s *LPSolver) recomputeWorkingRowResidual(row int, includeBasic bool, xbCand []float64, origVal float64) float64 {
	p := s.mat
	ws := s.ws
	n := p.numVars
	baseM := p.numConstraints
	tot := ws.totVar
	status := ws.status[:tot]
	xSlice := ws.x[:tot]

	rhs := s.rowRHS(row)
	slackVar := n + row
	var slackTerm float64
	hasSlackTerm := false
	if status[slackVar] == statusBasic {
		if includeBasic {
			bSlot := ws.basicRow[slackVar]
			if bSlot >= 0 && bSlot < len(xbCand) {
				slackTerm = -xbCand[bSlot]
				hasSlackTerm = true
			}
		}
	} else {
		slackTerm = -xSlice[slackVar]
		hasSlackTerm = true
	}

	var cols []int
	var vals []float64
	if row < baseM {
		st, en := p.rowPtr[row], p.rowPtr[row+1]
		cols = p.colIdx[st:en]
		vals = p.rowVal[st:en]
	} else {
		dr := s.extraRows[row-baseM]
		cols = dr.ColIdx
		vals = dr.Val[:len(cols)]
	}

	// Evaluate in double-double first and escalate to wideDotAcc only on overflow, underflow, or deep cancellation.
	var acc dot2Acc
	acc.addVal(rhs)
	if hasSlackTerm {
		acc.addVal(slackTerm)
	}
	for k, col := range cols {
		if uint(col) >= uint(n) {
			continue
		}
		if status[col] == statusBasic {
			if !includeBasic {
				continue
			}
			bSlot := ws.basicRow[col]
			if bSlot >= 0 && bSlot < len(xbCand) {
				acc.addProd(-vals[k], xbCand[bSlot])
			}
		} else {
			acc.addProd(-vals[k], xSlice[col])
		}
	}
	if val, eb, ok := acc.result(); ok && eb <= 0x1p-48*math.Abs(val) {
		return val
	}

	var w wideDotAcc
	w.addVal(rhs)
	if hasSlackTerm {
		w.addVal(slackTerm)
	}
	for k, col := range cols {
		if uint(col) >= uint(n) {
			continue
		}
		if status[col] == statusBasic {
			if !includeBasic {
				continue
			}
			bSlot := ws.basicRow[col]
			if bSlot >= 0 && bSlot < len(xbCand) {
				w.addProd(-vals[k], xbCand[bSlot])
			}
		} else {
			w.addProd(-vals[k], xSlice[col])
		}
	}
	if w.nonFin {
		return origVal
	}
	res := w.finish(origVal)
	if math.IsNaN(res) || math.IsInf(res, 0) {
		return origVal
	}
	return res
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

	rowMax := ws.ensureRowMaxTerm(m)
	for i := range m {
		xb := s.rowRHS(i)
		rm := math.Abs(xb)
		slackVar := n + i
		if status[slackVar] != statusBasic {
			xs := xSlice[slackVar]
			if axs := math.Abs(xs); !(axs <= rm) {
				rm = axs
			}
			xb -= xs
		}
		rowMax[i] = rm
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
				prod := vals[k] * xj
				if ap := math.Abs(prod); !(ap <= rowMax[r]) {
					rowMax[r] = ap
				}
				xbSlice[r] -= prod
			}
		}
	}
	for r, dr := range s.extraRows {
		sum := 0.0
		row := baseM + r
		rm := rowMax[row]
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(tot) && status[c] != statusBasic {
				prod := vals[k] * xSlice[c]
				if ap := math.Abs(prod); !(ap <= rm) {
					rm = ap
				}
				sum += prod
			}
		}
		rowMax[row] = rm
		xbSlice[row] -= sum
	}
	for i := range m {
		rm := rowMax[i]
		if rm == 0 {
			continue
		}
		if !(math.Abs(xbSlice[i]) >= rm*0x1p-30) || math.IsInf(xbSlice[i], 0) || math.IsInf(rm, 0) {
			xbSlice[i] = s.recomputeWorkingRowResidual(i, false, nil, xbSlice[i])
		}
	}

	ws.lu.FTRANExact(ws.xb)
	for i, bVar := range basis {
		if uint(bVar) < uint(tot) {
			xSlice[bVar] = xbSlice[i]
		}
	}
}

func (s *LPSolver) evalExactTableauFromRho() bool {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	baseM := p.numConstraints
	tot := ws.totVar
	rho := ws.rho[:m]
	tabRow := ws.tabRow[:tot]
	tabErr := ws.tabRowErr[:tot]

	colAcc := ws.ensureDotAcc(n)
	for _, k := range ws.tabRowIdx {
		if uint(k) < uint(tot) {
			tabRow[k] = 0
			tabErr[k] = 0
		}
	}
	tabIdx := ws.tabRowIdx[:0]
	for i := range m {
		yi := rho[i]
		if yi == 0 {
			continue
		}
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			clear(tabRow)
			clear(tabErr)
			ws.tabRowIdx = tabIdx[:0]
			return false
		}
		if i < baseM {
			for q := p.rowPtr[i]; q < p.rowPtr[i+1]; q++ {
				colAcc[p.colIdx[q]].addProd(yi, p.rowVal[q])
			}
		} else {
			dr := s.extraRows[i-baseM]
			for k, col := range dr.ColIdx {
				colAcc[col].addProd(yi, dr.Val[k])
			}
		}
		slack := n + i
		tabRow[slack] = yi
		tabErr[slack] = 0
		tabIdx = append(tabIdx, slack)
	}
	for j := range n {
		v, eb, ok := colAcc[j].result()
		if !ok {
			clear(tabRow)
			clear(tabErr)
			ws.tabRowIdx = tabIdx[:0]
			return false
		}
		tabRow[j] = v
		tabErr[j] = eb
	}
	ws.tabRowIdx = tabIdx
	return true
}

// computeExactTableauRow evaluates row r of B^{-1} [A I] using BTRANExact without intermediate
// drop tolerances and verifies that the basis identity residual satisfies ||rho^T B - e_r|| <= 1e-11
// before GMI cut generation consumes the row.
func (s *LPSolver) computeExactTableauRow(r int) bool {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	if uint(r) >= uint(m) || !ws.lu.valid {
		return false
	}
	tot := ws.totVar
	rho := ws.rho[:m]
	tabRow := ws.tabRow[:tot]
	tabErr := ws.tabRowErr[:tot]
	tabMark := ws.tabRowMark[:tot]
	basis := ws.basis[:m]
	numChunks := (n + 63) >> 6
	chunkMark := ws.tabRowChunkMark[:numChunks]

	clear(rho)
	ws.rhoIdx = ws.rhoIdx[:0]
	ws.rhoSparse = false
	for _, k := range ws.tabRowIdx {
		if uint(k) < uint(tot) {
			tabMark[k] = false
		}
	}
	ws.tabRowIdx = ws.tabRowIdx[:0]
	clear(tabRow)
	clear(tabErr)
	clear(chunkMark)
	ws.tabRowChunks = ws.tabRowChunks[:0]
	ws.tabRowSparse = false

	rho[r] = 1.0
	ws.lu.BTRANExact(ws.rho)

	if !s.evalExactTableauFromRho() {
		return false
	}

	needRefine := false
	for q, bCol := range basis {
		want := 0.0
		if q == r {
			want = 1.0
		}
		if math.Abs(tabRow[bCol]-want) > 1e-13 {
			needRefine = true
			break
		}
	}
	if needRefine {
		corr := ensureFloat64Slice(&ws.refineBuf1, m, false)
		for q, bCol := range basis {
			want := 0.0
			if q == r {
				want = 1.0
			}
			corr[q] = want - tabRow[bCol]
		}
		ws.lu.BTRANExact(corr)
		for i := range m {
			rho[i] += corr[i]
		}
		if !s.evalExactTableauFromRho() {
			return false
		}
	}

	for q, bCol := range basis {
		want := 0.0
		if q == r {
			want = 1.0
		}
		err := math.Abs(tabRow[bCol]-want) + tabErr[bCol]
		if math.IsNaN(err) || err > 1e-11 || err*max(1.0, math.Abs(ws.xb[q])) > 1e-10 {
			clear(tabRow)
			clear(tabErr)
			ws.tabRowIdx = ws.tabRowIdx[:0]
			return false
		}
		tabRow[bCol] = want
		tabErr[bCol] = 0
	}

	chunks := ws.tabRowChunks[:0]
	for ch := range numChunks {
		chunkMark[ch] = true
		chunks = append(chunks, ch)
	}
	ws.tabRowChunks = chunks
	return true
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
	totalNNZ := 0
	for _, i := range ws.rhoIdx {
		if uint(i) >= uint(m) {
			continue
		}
		if i < baseM {
			totalNNZ += p.rowPtr[i+1] - p.rowPtr[i]
		} else {
			totalNNZ += len(s.extraRows[i-baseM].ColIdx)
		}
	}

	if n > 0 && n <= DenseThreshold && len(p.denseRows) == baseM*p.rowStride && totalNNZ >= min(64, n>>1) {
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
			sparseRowCap = max(8, n>>4)
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
				cols := dr.ColIdx
				vals := dr.Val[:len(cols)]
				for k, col := range cols {
					if uint(col) < uint(n) {
						tabStruct[col] += yi * vals[k]
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
	tauIdx := ws.tauIdx[:0]
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
		tauIdx = s.addScaledColumn(fk, deltaX, ws.tau, tauIdx)
	}
	ws.tauIdx = ws.lu.FTRANSparseHint(ws.tau, tauIdx, tauIdx)
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
		rho := ws.rho[:m]
		if ws.rhoSparse {
			for _, i := range ws.rhoIdx {
				tau[i] = rho[i]
			}
			ws.tauIdx = ws.lu.FTRANSparseHint(tau, ws.rhoIdx, ws.tauIdx)
		} else {
			copy(tau, rho)
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
	newLeaveWeight := leaveWeight * invPiv2
	if !(newLeaveWeight >= minDSEWeight && newLeaveWeight <= maxDSEWeight) {
		newLeaveWeight = min(max(newLeaveWeight, minDSEWeight), maxDSEWeight)
		if math.IsNaN(newLeaveWeight) {
			newLeaveWeight = 1.0
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
		tabStruct := tabRow[:n]
		statusN := status[:n]
		djN := dj[:n]
		for _, ch := range ws.tabRowChunks {
			start := ch << tabRowChunkBits
			end := min(start+tabRowChunkSize, n)
			if start >= end {
				continue
			}
			size := end - start
			tabChunk := tabStruct[start : start+size]
			stChunk := statusN[start : start+size]
			djChunk := djN[start : start+size]
			for i, tab := range tabChunk {
				if tab == 0 {
					continue
				}
				alphaBar := -leaveDir * tab
				st := stChunk[i]
				if tab > pivotTol || tab < -pivotTol {
					absA, djSign, super, ok := dualBlockDir(alphaBar, st)
					if !ok {
						continue
					}
					k := start + i
					if super {
						thetaMax, minRatio = 0, 0
						cands = append(cands, bfrtCand{col: k, ratio: 0, absA: absA})
						continue
					}
					d := djSign * djChunk[i]
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
						invA := 1.0 / absA
						r0 := d * invA
						if hr := (d + effHarris) * invA; hr < thetaMax {
							thetaMax = hr
						}
						cands = append(cands, bfrtCand{col: k, ratio: r0, absA: absA})
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
			invA := 1.0 / absA
			r0 := d * invA
			if ratio := (d + effHarris) * invA; ratio < thetaMax {
				thetaMax = ratio
			}
			cands = append(cands, bfrtCand{col: k, ratio: r0, absA: absA})
		}
	}
	ws.bfrtCands = cands
	return thetaMax, minRatio, subTol, false
}

func (s *LPSolver) runDualSimplexLoop(cost []float64, primalFresh, dualFresh bool) (Status, int, bool, bool) {
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
			return StatusTimeLimit, iter, false, false
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter, false, false
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			primalFresh = true
			dualFresh = true
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
				v2 := viol * viol
				if v2 > bestScore*w {
					score := v2 / w
					if leaveRow == -1 || score > bestScore {
						bestScore = score
						leaveRow = r
						leaveDir = dir
						primalInfeas = viol
					}
				}
			}
		}

		if leaveRow == -1 {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, iter, false, false
				}
				s.computeDualsAndReducedCosts(cost)
				s.computePrimalValues()
				primalFresh = true
				dualFresh = true
				continue
			}
			if !s.isDualFeasible() {
				return StatusNumericalError, iter, false, false
			}
			return StatusOptimal, iter, primalFresh, dualFresh
		}

		s.ComputeTableauRow(leaveRow)

		thetaMax, minRatio, hadSubTolBlocking, hasNaN := s.dualRatioPass1(leaveDir, bland)
		if hasNaN {
			return StatusNumericalError, iter, false, false
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
					primalFresh = true
					dualFresh = true
					continue
				}
			}
			if hadSubTolBlocking || !s.verifyDualInfeasibleRow(leaveRow, leaveDir) {
				return StatusNumericalError, iter, false, false
			}
			return StatusInfeasible, iter, false, false
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
				return StatusNumericalError, iter, false, false
			}
			if !s.verifyDualInfeasibleRow(leaveRow, leaveDir) {
				if len(ws.lu.etas) > 0 && s.factorizeBasis() == nil && s.verifyDualInfeasibleRow(leaveRow, leaveDir) {
					return StatusInfeasible, iter, false, false
				}
				return StatusNumericalError, iter, false, false
			}
			return StatusInfeasible, iter, false, false
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
				return StatusNumericalError, iter, false, false
			}
			lastRefLeave, lastRefEnter = leaveRow, enterVar
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter, false, false
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			primalFresh = true
			dualFresh = true
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
				return StatusNumericalError, iter, false, false
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			primalFresh = true
			dualFresh = true
		} else {
			primalFresh = false
			dualFresh = false
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

	return StatusIterationLimit, s.MaxIter, false, false
}

func (s *LPSolver) solveTwoPhasePrimalWithLimit(maxIter int) (Status, int, bool, bool) {
	ws := s.ws
	m := s.TotalRows()
	totalIter := 0
	primalFresh := false

	if !s.luValid || !ws.lu.valid {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, 0, false, false
			}
		}
		s.computePrimalValues()
		primalFresh = true
	}

	blandTrigger := max(blandMinTrigger, blandRowMult*m)
	isBland := func(streak int) bool {
		return streak >= blandTrigger && ((streak-blandTrigger)&blandBurstMask) < blandBurstWindow
	}
	degenStreak := 0
	retriedSmallCrash := false
	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter, false, false
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter, false, false
			}
			s.computePrimalValues()
			primalFresh = true
		}

		clear(ws.workCost)
		anyInfeasible := false
		for i := range m {
			bVar := ws.basis[i]
			val := ws.xb[i]
			tol := s.feasTol(bVar)
			if val < ws.lb[bVar]-tol {
				ws.workCost[bVar] = -1.0
				anyInfeasible = true
			} else if val > ws.ub[bVar]+tol {
				ws.workCost[bVar] = 1.0
				anyInfeasible = true
			}
		}

		if !anyInfeasible {
			if len(ws.lu.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter, false, false
				}
				s.computePrimalValues()
				primalFresh = true
				continue
			}
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
					return StatusNumericalError, totalIter, false, false
				}
				s.computePrimalValues()
				primalFresh = true
				continue
			}
			if s.verifyPhase1Infeasible() {
				return StatusInfeasible, totalIter, false, false
			}
			if !retriedSmallCrash {
				retriedSmallCrash = true
				s.reinitSmallestMagnitudeCrash()
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter, false, false
				}
				s.computePrimalValues()
				primalFresh = true
				degenStreak = 0
				continue
			}
			return StatusNumericalError, totalIter, false, false
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter, false, false
		}
		primalFresh = false
	}

	degenStreak = 0
	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter, false, false
		}
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter, false, false
			}
			s.computePrimalValues()
			primalFresh = true
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
					return StatusNumericalError, totalIter, false, false
				}
				s.computePrimalValues()
				primalFresh = true
				continue
			}
			for i := range m {
				bVar := ws.basis[i]
				val := ws.xb[i]
				vTol := max(verifyFeasTol*min(1.0, s.feasTol(bVar)/primalTol), s.feasTol(bVar))
				if math.IsNaN(val) || val < ws.lb[bVar]-vTol || val > ws.ub[bVar]+vTol {
					return StatusNumericalError, totalIter, false, false
				}
			}
			return StatusOptimal, totalIter, primalFresh, true
		}
		if stepStatus == StatusUnbounded && len(ws.lu.etas) > 0 {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter, false, false
			}
			s.computePrimalValues()
			primalFresh = true
			continue
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter, false, false
		}
		primalFresh = false
	}

	return StatusIterationLimit, totalIter, false, false
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
	if enterVar == -1 && !phase1 {
		costSlice := ws.cost[:tot]
		bestGain := 10.0 * primalTol
		for k := range tot {
			st := ws.status[k]
			if st == statusBasic || st == statusFixed {
				continue
			}
			dj := ws.dj[k]
			switch st {
			case statusAtLower:
				if !(dj < -1e-15) || math.IsInf(ws.ub[k], 1) {
					continue
				}
				span := ws.ub[k] - ws.lb[k]
				if span > 0 {
					if gain := (-dj) * span; gain > bestGain && -dj > s.colDualNoiseTol(k, costSlice) {
						if bland {
							enterVar = k
							enterDir = 1.0
							goto runPrimalFTRAN
						}
						bestGain = gain
						enterVar = k
						enterDir = 1.0
					}
				}
			case statusAtUpper:
				if !(dj > 1e-15) || math.IsInf(ws.lb[k], -1) {
					continue
				}
				span := ws.ub[k] - ws.lb[k]
				if span > 0 {
					if gain := dj * span; gain > bestGain && dj > s.colDualNoiseTol(k, costSlice) {
						if bland {
							enterVar = k
							enterDir = -1.0
							goto runPrimalFTRAN
						}
						bestGain = gain
						enterVar = k
						enterDir = -1.0
					}
				}
			}
		}
	}
	if enterVar == -1 && !phase1 {
		enterVar, enterDir = s.priceCompensatedPhase2Fallback(bland)
	}
	if enterVar == -1 {
		return StatusOptimal, 0
	}

runPrimalFTRAN:
	clear(ws.fCol)
	ws.fColSparse = false
	s.fillColumn(enterVar, ws.fCol)
	ws.lu.FTRANExact(ws.fCol)

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

	if leaveRow == -1 {
		if math.IsInf(maxTheta, 1) {
			if phase1 || subTolBlocking {
				// An improving phase-1 ray or a phase-2 ray with sub-tolerance blocking pivots
				// is not a valid certificate of infeasibility or unboundedness.
				return StatusNumericalError, 0
			}
			if len(ws.lu.etas) == 0 && !s.verifyUnboundedRay(enterVar, enterDir, ws.fCol) {
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

func evalColDualDot(c0 float64, rows []int, vals, y0, dy1, dy2Abs []float64) (dj, eb, dy2Scaled, yDotScaled float64, ok bool) {
	const shift = 32
	var acc dot2Acc
	acc.addVal(c0)
	dy2Raw := 0.0
	yDotRaw := 0.0
	for idx, r := range rows {
		if uint(r) >= uint(len(y0)) {
			continue
		}
		a := vals[idx]
		acc.addProd(-y0[r], a)
		if dy1 != nil && dy1[r] != 0 {
			acc.addProd(-dy1[r], a)
		}
		aa := math.Abs(a)
		if dy2Abs != nil {
			dy2Raw += dy2Abs[r] * aa
		}
		yrAbs := math.Abs(y0[r])
		if dy1 != nil {
			yrAbs += math.Abs(dy1[r])
		}
		yDotRaw += yrAbs * aa
	}
	v, errB, accOK := acc.result()
	if accOK && !math.IsNaN(dy2Raw) && !math.IsInf(dy2Raw, 0) && !math.IsNaN(yDotRaw) && !math.IsInf(yDotRaw, 0) {
		return v, errB, math.Ldexp(dy2Raw, -shift), math.Ldexp(yDotRaw, -shift), true
	}
	if math.IsNaN(c0) || math.IsInf(c0, 0) {
		return 0, 0, 0, 0, false
	}
	maxExp := 0
	if c0 != 0 {
		_, ce := math.Frexp(math.Abs(c0))
		if ce > maxExp {
			maxExp = ce
		}
	}
	var w wideDotAcc
	w.addVal(c0)
	hiL1 := 0.0
	midL1 := 0.0
	if c0 != 0 {
		_, ce := math.Frexp(math.Abs(c0))
		if ce > wideExpCutoff {
			hiL1 += math.Ldexp(math.Abs(c0), -wideHiShift)
		} else {
			midL1 += math.Abs(c0)
		}
	}
	for idx, r := range rows {
		if uint(r) >= uint(len(y0)) {
			continue
		}
		a := vals[idx]
		y0r := y0[r]
		if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(y0r) || math.IsInf(y0r, 0) {
			return 0, 0, 0, 0, false
		}
		w.addProd(-y0r, a)
		aa := math.Abs(a)
		if aa != 0 && y0r != 0 {
			_, ea := math.Frexp(aa)
			_, ey := math.Frexp(math.Abs(y0r))
			if ea+ey > maxExp {
				maxExp = ea + ey
			}
			if ea+ey > wideExpCutoff {
				hiL1 += math.Ldexp(math.Abs(y0r), -wideHalfShift) * math.Ldexp(aa, -wideHalfShift)
			} else {
				midL1 += math.Abs(y0r) * aa
			}
		}
		if dy1 != nil && dy1[r] != 0 {
			if math.IsNaN(dy1[r]) || math.IsInf(dy1[r], 0) {
				return 0, 0, 0, 0, false
			}
			w.addProd(-dy1[r], a)
			if aa != 0 {
				_, ea := math.Frexp(aa)
				_, ed := math.Frexp(math.Abs(dy1[r]))
				if ea+ed > maxExp {
					maxExp = ea + ed
				}
				if ea+ed > wideExpCutoff {
					hiL1 += math.Ldexp(math.Abs(dy1[r]), -wideHalfShift) * math.Ldexp(aa, -wideHalfShift)
				} else {
					midL1 += math.Abs(dy1[r]) * aa
				}
			}
		}
		if dy2Abs != nil && dy2Abs[r] != 0 && aa != 0 {
			if math.IsNaN(dy2Abs[r]) || math.IsInf(dy2Abs[r], 0) {
				return 0, 0, 0, 0, false
			}
			_, ea := math.Frexp(aa)
			_, ed2 := math.Frexp(dy2Abs[r])
			if ea+ed2 > maxExp {
				maxExp = ea + ed2
			}
		}
	}
	if w.nonFin {
		return 0, 0, 0, 0, false
	}
	dj = w.finish(math.NaN())
	if math.IsNaN(dj) || math.IsInf(dj, 0) {
		return 0, 0, 0, 0, false
	}
	dynShift := max(shift, maxExp-wideExpCutoff)
	s1 := dynShift / 2
	s2 := dynShift - s1
	dy2Dyn := 0.0
	yDotDyn := 0.0
	for idx, r := range rows {
		if uint(r) >= uint(len(y0)) {
			continue
		}
		aas := math.Ldexp(math.Abs(vals[idx]), -s1)
		if aas == 0 {
			continue
		}
		yrS := math.Ldexp(math.Abs(y0[r]), -s2)
		if dy1 != nil && dy1[r] != 0 {
			yrS += math.Ldexp(math.Abs(dy1[r]), -s2)
		}
		yDotDyn += yrS * aas
		if dy2Abs != nil && dy2Abs[r] != 0 {
			dy2Dyn += math.Ldexp(dy2Abs[r], -s2) * aas
		}
	}
	if math.IsNaN(dy2Dyn) || math.IsInf(dy2Dyn, 0) || math.IsNaN(yDotDyn) || math.IsInf(yDotDyn, 0) {
		return 0, 0, 0, 0, false
	}
	const maxScCap = 0x1p1020
	dy2Scaled = min(maxScCap, math.Ldexp(dy2Dyn, dynShift-shift))
	yDotScaled = min(maxScCap, math.Ldexp(yDotDyn, dynShift-shift))
	ebFactor := float64(len(rows)+2) * 0x1p-103
	ebHi := math.Ldexp(ebFactor*hiL1, wideHiShift)
	ebMid := ebFactor * midL1
	if math.IsInf(ebHi, 1) || math.IsInf(ebHi+ebMid, 1) {
		eb = maxScCap
	} else {
		eb = ebHi + ebMid
	}
	return dj, eb, dy2Scaled, yDotScaled, true
}

func combineDualNoiseTol(eb, dy2Scaled, yDotScaled, maxBasisDualRel, mult float64) float64 {
	tail := math.Ldexp(mult*(dy2Scaled+maxBasisDualRel*yDotScaled), 32)
	if math.IsInf(tail, 1) {
		return math.MaxFloat64
	}
	res := eb + tail
	if math.IsInf(res, 1) {
		return math.MaxFloat64
	}
	return res
}

func (s *LPSolver) computeWorkingDualRefinement(cost []float64) (y0, dy1, dy2Abs []float64, ok bool) {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	baseM := p.numConstraints
	tot := ws.totVar
	if m == 0 || len(ws.y) < m || len(cost) < tot {
		return nil, nil, nil, false
	}
	y0 = slices.Clone(ws.y[:m])
	for _, yi := range y0 {
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			return nil, nil, nil, false
		}
	}
	dy1 = make([]float64, m)
	dy2Abs = make([]float64, m)
	if !s.luValid || !ws.lu.valid {
		return y0, dy1, dy2Abs, true
	}

	basis := ws.basis[:m]
	var scratchRows []int
	var scratchVals []float64
	evalBasicResiduals := func(useDY1 bool, dst []float64) (anyNonZero, anyErr, valid bool) {
		var curDY1 []float64
		if useDY1 {
			curDY1 = dy1
		}
		for q, bCol := range basis {
			if bCol >= n {
				r := bCol - n
				if uint(r) >= uint(m) {
					return false, false, false
				}
				scratchRows = append(scratchRows[:0], r)
				scratchVals = append(scratchVals[:0], 1.0)
			} else {
				scratchRows, scratchVals = s.appendBasisColumn(bCol, scratchRows[:0], scratchVals[:0])
			}
			_ = baseM
			v, eb, _, _, dotOK := evalColDualDot(cost[bCol], scratchRows, scratchVals, y0, curDY1, nil)
			if !dotOK {
				return false, false, false
			}
			dst[q] = v
			if v != 0 {
				anyNonZero = true
			}
			if eb != 0 {
				anyErr = true
			}
		}
		return anyNonZero, anyErr, true
	}

	anyR1, anyEB1, okR1 := evalBasicResiduals(false, dy1)
	if !okR1 {
		clear(dy1)
		return y0, dy1, dy2Abs, true
	}
	if !anyR1 && !anyEB1 {
		return y0, dy1, dy2Abs, true
	}
	if anyR1 {
		ws.lu.BTRANExact(dy1)
		validDY1 := true
		for _, v := range dy1 {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				validDY1 = false
				break
			}
		}
		if !validDY1 {
			clear(dy1)
			return y0, dy1, dy2Abs, true
		}
		dy2 := make([]float64, m)
		anyR2, _, okR2 := evalBasicResiduals(true, dy2)
		if okR2 && anyR2 {
			ws.lu.BTRANExact(dy2)
			for i, v := range dy2 {
				if !math.IsNaN(v) && !math.IsInf(v, 0) {
					dy2Abs[i] = math.Abs(v)
				}
			}
		}
	}
	parent := make([]int, m)
	compSize := make([]int, m)
	for i := range m {
		parent[i] = i
		compSize[i] = 1
	}
	findRoot := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	unionRows := func(a, b int) {
		ra, rb := findRoot(a), findRoot(b)
		if ra == rb {
			return
		}
		if compSize[ra] < compSize[rb] {
			ra, rb = rb, ra
		}
		parent[rb] = ra
		compSize[ra] += compSize[rb]
	}
	for _, bCol := range basis {
		if bCol >= n {
			continue
		}
		scratchRows, scratchVals = s.appendBasisColumn(bCol, scratchRows[:0], scratchVals[:0])
		firstRow := -1
		for idx, r := range scratchRows {
			if uint(r) >= uint(m) || scratchVals[idx] == 0 {
				continue
			}
			if firstRow == -1 {
				firstRow = r
			} else {
				unionRows(firstRow, r)
			}
		}
	}
	compMaxDual := make([]float64, m)
	for i := range m {
		ay := math.Abs(y0[i]) + math.Abs(dy1[i])
		root := findRoot(i)
		if ay > compMaxDual[root] {
			compMaxDual[root] = ay
		}
	}
	for i := range m {
		root := findRoot(i)
		rowScaleFloor := float64(compSize[root]+2) * 0x1p-102
		if ddFloor := rowScaleFloor * compMaxDual[root]; dy2Abs[i] < ddFloor {
			dy2Abs[i] = ddFloor
		}
	}
	return y0, dy1, dy2Abs, true
}

func (s *LPSolver) buildExtraColBuckets(orig bool) (colOff, rows []int, vals []float64) {
	if len(s.extraRows) == 0 {
		return nil, nil, nil
	}
	if !orig || !s.scaled {
		s.ensureExtraCSC()
		if len(s.extraColVal) == 0 {
			return nil, nil, nil
		}
		return s.extraColPtr, s.extraRowIdx, s.extraColVal
	}
	n := s.prob.numVars
	baseM := s.prob.numConstraints
	colOff = make([]int, n+1)
	total := 0
	for _, dr := range s.extraRows {
		for _, c := range dr.ColIdx {
			if uint(c) < uint(n) {
				colOff[c+1]++
				total++
			}
		}
	}
	if total == 0 {
		return nil, nil, nil
	}
	for j := 1; j <= n; j++ {
		colOff[j] += colOff[j-1]
	}
	rows = make([]int, total)
	vals = make([]float64, total)
	pos := slices.Clone(colOff[:n])
	for e, dr := range s.extraRows {
		rv := dr.Val
		if orig && s.scaled && dr.OrigVal != nil {
			rv = dr.OrigVal
		}
		rowIdx := baseM + e
		for k, c := range dr.ColIdx {
			if uint(c) < uint(n) {
				idx := pos[c]
				rows[idx] = rowIdx
				vals[idx] = rv[k]
				pos[c] = idx + 1
			}
		}
	}
	return colOff, rows, vals
}

func (s *LPSolver) computeRefinedWorkingPricing(cost []float64) (djC, djE, dy2Sc, yDotSc []float64, maxBasisDualRel float64, ok bool) {
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	y0, dy1, dy2Abs, refOK := s.computeWorkingDualRefinement(cost)
	if !refOK {
		return nil, nil, nil, nil, 0, false
	}
	djC = make([]float64, tot)
	djE = make([]float64, tot)
	dy2Sc = make([]float64, tot)
	yDotSc = make([]float64, tot)

	exOff, exRows, exVals := s.buildExtraColBuckets(false)
	var colRows []int
	var colVals []float64
	for j := range n {
		st, en := p.colPtr[j], p.colPtr[j+1]
		colRows = append(colRows[:0], p.rowIdx[st:en]...)
		colVals = append(colVals[:0], p.colVal[st:en]...)
		if exOff != nil {
			es, ee := exOff[j], exOff[j+1]
			colRows = append(colRows, exRows[es:ee]...)
			colVals = append(colVals, exVals[es:ee]...)
		}
		v, eb, d2s, yds, dotOK := evalColDualDot(cost[j], colRows, colVals, y0, dy1, dy2Abs)
		if !dotOK {
			return nil, nil, nil, nil, 0, false
		}
		djC[j] = v
		djE[j] = eb
		dy2Sc[j] = d2s
		yDotSc[j] = yds
	}
	for r := range m {
		k := n + r
		colRows = append(colRows[:0], r)
		colVals = append(colVals[:0], 1.0)
		v, eb, d2s, yds, dotOK := evalColDualDot(cost[k], colRows, colVals, y0, dy1, dy2Abs)
		if !dotOK {
			return nil, nil, nil, nil, 0, false
		}
		djC[k] = v
		djE[k] = eb
		dy2Sc[k] = d2s
		yDotSc[k] = yds
	}
	for _, bCol := range ws.basis[:m] {
		if uint(bCol) >= uint(tot) {
			continue
		}
		denSc := yDotSc[bCol] + math.Ldexp(math.Abs(cost[bCol]), -32)
		if denSc > 0 {
			numSc := math.Ldexp(math.Abs(djC[bCol])+djE[bCol], -32)
			if rel := numSc / denSc; rel > maxBasisDualRel {
				maxBasisDualRel = rel
			}
		}
	}
	return djC, djE, dy2Sc, yDotSc, maxBasisDualRel, true
}

func (s *LPSolver) hasImprovingUnboundedColumn(cost []float64) bool {
	if !s.hasInfiniteStructuralBound() {
		return false
	}
	ws := s.ws
	n := s.mat.numVars
	tot := ws.totVar
	djC, djE, dy2Sc, yDotSc, maxBasisDualRel, ok := s.computeRefinedWorkingPricing(cost)
	if !ok {
		return true
	}
	status := ws.status[:tot]
	for k := range tot {
		st := status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := djC[k]
		noiseTol := combineDualNoiseTol(djE[k], dy2Sc[k], yDotSc[k], maxBasisDualRel, 8.0)
		switch st {
		case statusAtLower:
			if math.IsInf(ws.ub[k], 1) && -dj > noiseTol && (k < n || s.slackCanMoveUnbounded(k-n, true)) {
				return true
			}
		case statusAtUpper:
			if math.IsInf(ws.lb[k], -1) && dj > noiseTol && (k < n || s.slackCanMoveUnbounded(k-n, false)) {
				return true
			}
		case statusSuperBasic:
			if math.Abs(dj) > noiseTol {
				return true
			}
		}
	}
	return false
}

func (s *LPSolver) priceCompensatedPhase2Fallback(bland bool) (int, float64) {
	ws := s.ws
	n := s.mat.numVars
	m := s.TotalRows()
	if n == 0 || m == 0 {
		return -1, 0
	}
	tot := ws.totVar
	cost := ws.cost[:tot]
	status := ws.status[:tot]

	djC, djE, dy2Sc, yDotSc, maxBasisDualRel, ok := s.computeRefinedWorkingPricing(cost)
	if !ok {
		return -1, 0
	}

	enterVar := -1
	enterDir := 0.0
	bestGain := 10.0 * primalTol
	bestInfAbs := 0.0
	hasInfCand := false

	for k := range tot {
		st := status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := djC[k]
		noiseTol := combineDualNoiseTol(djE[k], dy2Sc[k], yDotSc[k], maxBasisDualRel, 8.0)
		switch st {
		case statusAtLower:
			if math.IsInf(ws.ub[k], 1) {
				if -dj > noiseTol {
					if bland {
						return k, 1.0
					}
					if !hasInfCand || -dj > bestInfAbs {
						hasInfCand = true
						bestInfAbs = -dj
						enterVar = k
						enterDir = 1.0
					}
				}
				continue
			}
			if k < n {
				if span := ws.ub[k] - ws.lb[k]; span > 0 && -dj > noiseTol {
					if gain := (-dj - noiseTol) * span; gain > bestGain {
						if bland {
							return k, 1.0
						}
						if !hasInfCand {
							bestGain = gain
							enterVar = k
							enterDir = 1.0
						}
					}
				}
			}
		case statusAtUpper:
			if math.IsInf(ws.lb[k], -1) {
				if dj > noiseTol {
					if bland {
						return k, -1.0
					}
					if !hasInfCand || dj > bestInfAbs {
						hasInfCand = true
						bestInfAbs = dj
						enterVar = k
						enterDir = -1.0
					}
				}
				continue
			}
			if k < n {
				if span := ws.ub[k] - ws.lb[k]; span > 0 && dj > noiseTol {
					if gain := (dj - noiseTol) * span; gain > bestGain {
						if bland {
							return k, -1.0
						}
						if !hasInfCand {
							bestGain = gain
							enterVar = k
							enterDir = -1.0
						}
					}
				}
			}
		case statusSuperBasic:
			if math.Abs(dj) > noiseTol {
				dir := -1.0
				if dj < 0 {
					dir = 1.0
				}
				if bland {
					return k, dir
				}
				if !hasInfCand || math.Abs(dj) > bestInfAbs {
					hasInfCand = true
					bestInfAbs = math.Abs(dj)
					enterVar = k
					enterDir = dir
				}
			}
		}
	}
	return enterVar, enterDir
}

func (s *LPSolver) verifyUnboundedRay(enterVar int, enterDir float64, fCol []float64) bool {
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	if n == 0 || len(fCol) < m {
		return false
	}
	ray := make([]float64, n)
	colSc := func(j int) float64 {
		if s.scaled && j < len(s.colScale) {
			return s.colScale[j]
		}
		return 1.0
	}
	if enterVar >= 0 && enterVar < n {
		ray[enterVar] = enterDir * colSc(enterVar)
	}
	for i := range m {
		bVar := ws.basis[i]
		if bVar >= 0 && bVar < n {
			fi := fCol[i]
			if math.IsNaN(fi) || math.IsInf(fi, 0) {
				return false
			}
			ray[bVar] = -enterDir * fi * colSc(bVar)
		}
	}
	anyNonZero := false
	for j, rj := range ray {
		if math.IsNaN(rj) || math.IsInf(rj, 0) {
			return false
		}
		if rj == 0 {
			continue
		}
		anyNonZero = true
		if rj > 0 && !math.IsInf(s.unscaledBound(j, true), 1) {
			return false
		}
		if rj < 0 && !math.IsInf(s.unscaledBound(j, false), -1) {
			return false
		}
	}
	if !anyNonZero {
		return false
	}
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		act, rTol, ok := evalSparseRowResidualAndTol(p.colIdx[st:en], p.rowVal[st:en], ray, nil, nil, 0, 0, 1e-9)
		if !ok {
			return false
		}
		switch p.rowSense[i] {
		case LessEqual:
			if act > rTol {
				return false
			}
		case GreaterEqual:
			if act < -rTol {
				return false
			}
		case Equal:
			if math.Abs(act) > rTol {
				return false
			}
		}
	}
	for _, dr := range s.extraRows {
		vals := dr.Val
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
		}
		act, rTol, ok := evalSparseRowResidualAndTol(dr.ColIdx, vals, ray, nil, nil, 0, 0, 1e-9)
		if !ok || act > rTol {
			return false
		}
	}
	var objAcc dot2Acc
	for j, rj := range ray {
		if rj != 0 && p.objMin[j] != 0 {
			objAcc.addProd(p.objMin[j], rj)
		}
	}
	val, eb, ok := objAcc.result()
	if !ok {
		const shift = 32
		var objAccS dot2Acc
		for j, rj := range ray {
			if rj != 0 && p.objMin[j] != 0 {
				objAccS.addProd(math.Ldexp(p.objMin[j], -shift/2), math.Ldexp(rj, -shift/2))
			}
		}
		vs, ebs, okS := objAccS.result()
		if !okS {
			return false
		}
		return vs+ebs < 0
	}
	return val+eb < 0
}

func (s *LPSolver) refineOptimalBasis(primalFresh, dualFresh bool) {
	if !s.luValid || s.ws == nil || !s.ws.lu.valid {
		return
	}
	p := s.mat
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	tot := ws.totVar
	status := ws.status[:tot]
	xSlice := ws.x[:tot]
	xbSlice := ws.xb[:m]
	basis := ws.basis[:m]
	baseM := p.numConstraints

	if !primalFresh {
		s.computePrimalValues()
	}

	basicPos := ws.basicRow[:tot]

	evalPrimalRes := func(xbCand []float64, dst []float64) float64 {
		rowMax := ws.ensureRowMaxTerm(m)
		for i := range m {
			r := s.rowRHS(i)
			rm := math.Abs(r)
			slackVar := n + i
			if q := basicPos[slackVar]; q >= 0 {
				if aq := math.Abs(xbCand[q]); !(aq <= rm) {
					rm = aq
				}
				r -= xbCand[q]
			} else {
				if as := math.Abs(xSlice[slackVar]); !(as <= rm) {
					rm = as
				}
				r -= xSlice[slackVar]
			}
			rowMax[i] = rm
			dst[i] = r
		}
		for j := range n {
			xj := xSlice[j]
			if q := basicPos[j]; q >= 0 {
				xj = xbCand[q]
			}
			if xj == 0 {
				continue
			}
			for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
				row := p.rowIdx[idx]
				prod := p.colVal[idx] * xj
				if ap := math.Abs(prod); !(ap <= rowMax[row]) {
					rowMax[row] = ap
				}
				dst[row] -= prod
			}
		}
		for e, dr := range s.extraRows {
			sum := 0.0
			row := baseM + e
			rm := rowMax[row]
			for k, c := range dr.ColIdx {
				xc := xSlice[c]
				if q := basicPos[c]; q >= 0 {
					xc = xbCand[q]
				}
				if xc != 0 {
					prod := dr.Val[k] * xc
					if ap := math.Abs(prod); !(ap <= rm) {
						rm = ap
					}
					sum += prod
				}
			}
			rowMax[row] = rm
			dst[row] -= sum
		}
		maxR := 0.0
		for i, v := range dst {
			rm := rowMax[i]
			if rm != 0 && (!(math.Abs(v) >= rm*0x1p-30) || math.IsInf(v, 0) || math.IsInf(rm, 0)) {
				v = s.recomputeWorkingRowResidual(i, true, xbCand, v)
				dst[i] = v
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return math.Inf(1)
				}
			}
			if av := math.Abs(v); av > maxR {
				maxR = av
			}
		}
		return maxR
	}

	resP := ensureFloat64Slice(&ws.refineBuf1, m, false)
	oldP := evalPrimalRes(xbSlice, resP)
	if oldP > 0 && !math.IsInf(oldP, 1) && (m <= 64 || oldP > 1e-12) {
		ws.lu.FTRANExact(resP)
		candXB := ensureFloat64Slice(&ws.refineBuf2, m, false)
		okP := true
		for i := range m {
			v := xbSlice[i] + resP[i]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				okP = false
				break
			}
			candXB[i] = v
		}
		if okP {
			if evalPrimalRes(candXB, resP) < oldP {
				copy(xbSlice, candXB)
				for i, bVar := range basis {
					xSlice[bVar] = candXB[i]
				}
			}
		}
	}

	if !dualFresh {
		s.computeDualsAndReducedCosts(ws.cost)
	}
	y := ws.y[:m]
	cost := ws.cost[:tot]
	dualAcc := ws.ensureDotAcc(m)
	var scratchRows []int
	var scratchVals []float64
	evalDualRes := func(yCand, dst []float64) float64 {
		clear(dualAcc)
		for q, bCol := range basis {
			dualAcc[q].addVal(cost[bCol])
			if bCol >= n {
				dualAcc[q].addVal(-yCand[bCol-n])
			} else {
				for idx := p.colPtr[bCol]; idx < p.colPtr[bCol+1]; idx++ {
					dualAcc[q].addProd(-yCand[p.rowIdx[idx]], p.colVal[idx])
				}
			}
		}
		for e, dr := range s.extraRows {
			yr := yCand[baseM+e]
			if yr == 0 {
				continue
			}
			for k, c := range dr.ColIdx {
				if q := basicPos[c]; q >= 0 {
					dualAcc[q].addProd(-yr, dr.Val[k])
				}
			}
		}
		maxR := 0.0
		for q, bCol := range basis {
			r, _, ok := dualAcc[q].result()
			if !ok {
				if bCol >= n {
					scratchRows = append(scratchRows[:0], bCol-n)
					scratchVals = append(scratchVals[:0], 1.0)
				} else {
					scratchRows, scratchVals = s.appendBasisColumn(bCol, scratchRows[:0], scratchVals[:0])
				}
				r, _, _, _, ok = evalColDualDot(cost[bCol], scratchRows, scratchVals, yCand, nil, nil)
				if !ok {
					return math.Inf(1)
				}
			}
			dst[q] = r
			if ar := math.Abs(r); ar > maxR {
				maxR = ar
			}
		}
		return maxR
	}

	resD := ensureFloat64Slice(&ws.refineBuf1, m, false)
	oldD := evalDualRes(y, resD)
	if oldD > 0 && !math.IsInf(oldD, 1) && (m <= 64 || oldD > 1e-12 || s.hasInfiniteStructuralBound()) {
		ws.lu.BTRANExact(resD)
		candY := ensureFloat64Slice(&ws.refineBuf2, m, false)
		okD := true
		for i := range m {
			v := y[i] + resD[i]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				okD = false
				break
			}
			candY[i] = v
		}
		if okD {
			if evalDualRes(candY, resD) < oldD {
				copy(y, candY)
				dj := ws.dj[:tot]
				for j := range n {
					if status[j] == statusBasic {
						dj[j] = 0
						continue
					}
					dot := 0.0
					for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
						dot += y[p.rowIdx[idx]] * p.colVal[idx]
					}
					dj[j] = cost[j] - dot
				}
				for e, dr := range s.extraRows {
					yr := y[baseM+e]
					if yr == 0 {
						continue
					}
					for k, c := range dr.ColIdx {
						if status[c] != statusBasic {
							dj[c] -= yr * dr.Val[k]
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
		}
	}
}

func (s *LPSolver) buildRefinedOptimalResult(totalIter int, primalFresh, dualFresh bool) LPResult {
	s.refineOptimalBasis(primalFresh, dualFresh)
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := p.numConstraints

	primal := make([]float64, n)
	dj := make([]float64, n)
	duals := make([]float64, m)

	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	if s.scaled {
		objS := s.objScale
		for j := range n {
			val := ws.x[j] * s.colScale[j]
			lbj, ubj := s.origLB[j], s.origUB[j]
			snapTol := max(primalSnapTol, 1e-8*max(1.0, math.Abs(val)))
			if val < lbj && lbj-val <= snapTol {
				val = lbj
			} else if val > ubj && val-ubj <= snapTol {
				val = ubj
			}
			primal[j] = val
			dj[j] = senseSign * objS * s.invColScale[j] * ws.dj[j]
		}
		for i := range m {
			duals[i] = senseSign * objS * s.rowScale[i] * ws.y[i]
		}
	} else {
		for j := range n {
			val := ws.x[j]
			snapTol := max(primalSnapTol, 1e-8*max(1.0, math.Abs(val)))
			if val < ws.lb[j] && ws.lb[j]-val <= snapTol {
				val = ws.lb[j]
			} else if val > ws.ub[j] && val-ws.ub[j] <= snapTol {
				val = ws.ub[j]
			}
			primal[j] = val
			dj[j] = senseSign * ws.dj[j]
		}
		for i := range m {
			duals[i] = senseSign * ws.y[i]
		}
	}
	objMin := evalLinearSum(0, p.objMin, primal, n)

	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    p.EvaluateObjective(primal),
		Primal:       primal,
		Dual:         duals,
		ReducedCosts: dj,
		Iterations:   totalIter,
	}
}

func (s *LPSolver) finalizeOptimalResult(totalIter int, primalFresh, dualFresh bool) (LPResult, bool) {
	res := s.buildRefinedOptimalResult(totalIter, primalFresh, dualFresh)
	if !math.IsNaN(res.Objective) && !math.IsInf(res.Objective, 0) &&
		s.primalResidualsWithinTol(res.Primal) && s.dualResidualsWithinTol(res.Primal, res.ObjectiveMin) {
		return res, true
	}
	if err := s.factorizeBasis(); err != nil {
		return LPResult{}, false
	}
	res = s.buildRefinedOptimalResult(totalIter, false, false)
	if !math.IsNaN(res.Objective) && !math.IsInf(res.Objective, 0) &&
		s.primalResidualsWithinTol(res.Primal) && s.dualResidualsWithinTol(res.Primal, res.ObjectiveMin) {
		return res, true
	}
	return LPResult{}, false
}

func (s *LPSolver) boundedRowSpan(cols []int, vals []float64) (float64, bool) {
	span := 0.0
	for k, c := range cols {
		a := math.Abs(vals[k])
		if a == 0 {
			continue
		}
		lbc := s.unscaledBound(c, false)
		ubc := s.unscaledBound(c, true)
		if math.IsInf(lbc, -1) || math.IsInf(ubc, 1) {
			return 0, false
		}
		w := ubc - lbc
		if math.IsInf(w, 1) {
			p := math.Ldexp(a, 2) * (math.Ldexp(ubc, -2) - math.Ldexp(lbc, -2))
			if math.IsNaN(p) || math.IsInf(p, 0) {
				return 0, false
			}
			span += p
		} else if w > 0 {
			span += a * w
		}
		if math.IsNaN(span) || math.IsInf(span, 0) {
			return 0, false
		}
	}
	return span, true
}

func (s *LPSolver) dualResidualsWithinTol(primal []float64, objMin float64) bool {
	p := s.prob
	ws := s.ws
	n := p.numVars
	m := s.TotalRows()
	if len(primal) < n || len(ws.y) < m || math.IsNaN(objMin) || math.IsInf(objMin, 0) {
		return false
	}
	hasInfStruct := s.hasInfiniteStructuralBound()
	if !hasInfStruct {
		if s.dualResidualsEval(primal, objMin, false, ws.y[:m], nil, nil) {
			return true
		}
	}
	yWork, dy1Work, dy2AbsWork, refOK := s.computeWorkingDualRefinement(ws.cost[:ws.totVar])
	if !refOK {
		return false
	}
	return s.dualResidualsEval(primal, objMin, hasInfStruct, yWork, dy1Work, dy2AbsWork)
}

func (s *LPSolver) dualResidualsEval(
	primal []float64,
	objMin float64,
	hasInfStruct bool,
	yWork, dy1Work, dy2AbsWork []float64,
) bool {
	p := s.prob
	ws := s.ws
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()

	yMin := ensureFloat64Slice(&ws.refineBuf1, m, false)
	var dy1Min, dy2MinAbs []float64
	if dy1Work != nil {
		dy1Min = ensureFloat64Slice(&ws.refineBuf2, m, false)
		dy2MinAbs = ensureFloat64Slice(&ws.refineBuf3, m, false)
	}
	maxObj := 1.0
	for j := range n {
		if av := math.Abs(p.objMin[j]); av > maxObj {
			maxObj = av
		}
	}
	maxDual := maxObj
	for i := range m {
		mult := 1.0
		if s.scaled {
			mult = s.objScale * s.rowScaleOf(i)
		}
		yi := yWork[i] * mult
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			return false
		}
		yMin[i] = yi
		if dy1Work != nil {
			d1 := dy1Work[i] * mult
			d2 := dy2AbsWork[i] * mult
			if math.IsNaN(d1) || math.IsInf(d1, 0) || math.IsNaN(d2) || math.IsInf(d2, 0) {
				return false
			}
			dy1Min[i] = d1
			dy2MinAbs[i] = d2
		}
		if ay := math.Abs(yi); ay > maxDual {
			maxDual = ay
		}
	}

	djOrig := ensureFloat64Slice(&ws.evalDJOrig, n, false)
	djErr := ensureFloat64Slice(&ws.evalDJErr, n, false)
	dy2Sc := ensureFloat64Slice(&ws.evalDY2Sc, n, false)
	yDotSc := ensureFloat64Slice(&ws.evalYDotSc, n, dy1Work == nil)
	if dy1Work == nil {
		colAcc := ws.ensureDotAcc(n)
		yL1 := yDotSc
		for j := range n {
			colAcc[j].addVal(p.objMin[j])
			for q := p.colPtr[j]; q < p.colPtr[j+1]; q++ {
				yr := yMin[p.rowIdx[q]]
				av := p.colVal[q]
				colAcc[j].addProd(-yr, av)
				yL1[j] += math.Abs(yr * av)
			}
		}
		for e, dr := range s.extraRows {
			yr := yMin[baseM+e]
			if yr == 0 {
				continue
			}
			vals := dr.Val
			if s.scaled && dr.OrigVal != nil {
				vals = dr.OrigVal
			}
			for k, c := range dr.ColIdx {
				colAcc[c].addProd(-yr, vals[k])
				yL1[c] += math.Abs(yr * vals[k])
			}
		}
		var exOff, exRows []int
		var exVals []float64
		builtEx := false
		var colRows []int
		var colVals []float64
		for j := range n {
			v, eb, ok := colAcc[j].result()
			if ok && !math.IsNaN(yL1[j]) && !math.IsInf(yL1[j], 0) {
				djOrig[j] = v
				djErr[j] = eb
				dy2Sc[j] = 0
				yDotSc[j] = math.Ldexp(yL1[j], -32)
				continue
			}
			if !builtEx {
				exOff, exRows, exVals = s.buildExtraColBuckets(true)
				builtEx = true
			}
			st, en := p.colPtr[j], p.colPtr[j+1]
			colRows = append(colRows[:0], p.rowIdx[st:en]...)
			colVals = append(colVals[:0], p.colVal[st:en]...)
			if exOff != nil {
				es, ee := exOff[j], exOff[j+1]
				colRows = append(colRows, exRows[es:ee]...)
				colVals = append(colVals, exVals[es:ee]...)
			}
			var d2s, yds float64
			v, eb, d2s, yds, ok = evalColDualDot(p.objMin[j], colRows, colVals, yMin, nil, nil)
			if !ok {
				return false
			}
			djOrig[j] = v
			djErr[j] = eb
			dy2Sc[j] = d2s
			yDotSc[j] = yds
		}
	} else {
		exOff, exRows, exVals := s.buildExtraColBuckets(true)
		var colRows []int
		var colVals []float64
		for j := range n {
			st, en := p.colPtr[j], p.colPtr[j+1]
			colRows = append(colRows[:0], p.rowIdx[st:en]...)
			colVals = append(colVals[:0], p.colVal[st:en]...)
			if exOff != nil {
				es, ee := exOff[j], exOff[j+1]
				colRows = append(colRows, exRows[es:ee]...)
				colVals = append(colVals, exVals[es:ee]...)
			}
			v, eb, d2s, yds, ok := evalColDualDot(p.objMin[j], colRows, colVals, yMin, dy1Min, dy2MinAbs)
			if !ok {
				return false
			}
			djOrig[j] = v
			djErr[j] = eb
			dy2Sc[j] = d2s
			yDotSc[j] = yds
		}
	}

	rowDualAt := func(r int) (yiSum, yiRem, d2Abs float64) {
		if dy1Min == nil {
			return yMin[r], 0, 0
		}
		sVal, rVal := twoSum(yMin[r], dy1Min[r])
		return sVal, rVal, dy2MinAbs[r]
	}

	maxBasisDualRel := 0.0
	for _, bCol := range ws.basis[:m] {
		if bCol < n {
			denSc := yDotSc[bCol] + math.Ldexp(math.Abs(p.objMin[bCol]), -32)
			if denSc > 0 {
				numSc := math.Ldexp(math.Abs(djOrig[bCol])+djErr[bCol], -32)
				if rel := numSc / denSc; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		} else if r := bCol - n; uint(r) < uint(m) {
			slackY, slackEB, _ := rowDualAt(r)
			d1Abs := 0.0
			if dy1Min != nil {
				d1Abs = math.Abs(dy1Min[r])
			}
			denSc := math.Ldexp(math.Abs(yMin[r]), -32) + math.Ldexp(d1Abs, -32)
			if denSc > 0 {
				numSc := math.Ldexp(math.Abs(slackY)+math.Abs(slackEB), -32)
				if rel := numSc / denSc; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		}
	}

	signTol := max(verifyFeasTol, 1e-7*maxDual)
	slackNoise := max(1e-13, 1e-12*maxDual)
	compGap := 0.0
	status := ws.status[:ws.totVar]

	unscaledLB, unscaledUB := ws.lb[:n], ws.ub[:n]
	if s.scaled {
		unscaledLB, unscaledUB = s.origLB, s.origUB
	}

	for i := range baseM {
		yiSum, yiRem, d2Abs := rowDualAt(i)
		if yiSum == 0 && yiRem == 0 {
			continue
		}
		slackBasic := status[n+i] == statusBasic
		strictSign := !slackBasic && hasInfStruct && s.slackCanMoveUnbounded(i, p.rowSense[i] == LessEqual)
		rowSignTol := signTol
		if strictSign {
			d1Abs := 0.0
			if dy1Min != nil {
				d1Abs = math.Abs(dy1Min[i])
			}
			rowSignTol = combineDualNoiseTol(
				math.Abs(yiRem),
				math.Ldexp(d2Abs, -32),
				math.Ldexp(math.Abs(yMin[i]), -32)+math.Ldexp(d1Abs, -32),
				maxBasisDualRel,
				16.0,
			)
		} else if slackBasic && math.Abs(yiSum) > signTol {
			return false
		}
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		res, rTol, ok := evalSparseRowResidualAndTol(p.colIdx[st:en], p.rowVal[st:en], primal, unscaledLB, unscaledUB, p.rowRHS[i], verifyFeasTol, 1e-8)
		if !ok {
			return false
		}
		switch p.rowSense[i] {
		case LessEqual:
			if yiSum > rowSignTol {
				return false
			}
			if yiSum > slackNoise && !strictSign {
				if span, spOK := s.boundedRowSpan(p.colIdx[st:en], p.rowVal[st:en]); spOK {
					compGap += (yiSum - slackNoise) * span
				} else {
					return false
				}
			}
			if yiSum < -slackNoise && res < 0 {
				diff := -res
				if diff > rTol {
					compGap += (-yiSum - slackNoise) * (diff - rTol)
				}
			}
		case GreaterEqual:
			if yiSum < -rowSignTol {
				return false
			}
			if yiSum < -slackNoise && !strictSign {
				if span, spOK := s.boundedRowSpan(p.colIdx[st:en], p.rowVal[st:en]); spOK {
					compGap += (-yiSum - slackNoise) * span
				} else {
					return false
				}
			}
			if yiSum > slackNoise && res > 0 {
				if res > rTol {
					compGap += (yiSum - slackNoise) * (res - rTol)
				}
			}
		}
	}
	for e, dr := range s.extraRows {
		idx := baseM + e
		yiSum, yiRem, d2Abs := rowDualAt(idx)
		if yiSum == 0 && yiRem == 0 {
			continue
		}
		slackBasic := status[n+idx] == statusBasic
		strictSign := !slackBasic && hasInfStruct && s.slackCanMoveUnbounded(idx, true)
		rowSignTol := signTol
		if strictSign {
			d1Abs := 0.0
			if dy1Min != nil {
				d1Abs = math.Abs(dy1Min[idx])
			}
			rowSignTol = combineDualNoiseTol(
				math.Abs(yiRem),
				math.Ldexp(d2Abs, -32),
				math.Ldexp(math.Abs(yMin[idx]), -32)+math.Ldexp(d1Abs, -32),
				maxBasisDualRel,
				16.0,
			)
		} else if slackBasic && math.Abs(yiSum) > signTol {
			return false
		}
		if yiSum > rowSignTol {
			return false
		}
		vals := dr.Val
		rhs := dr.RHS
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
			rhs = dr.OrigRHS
		}
		if yiSum > slackNoise && !strictSign {
			if span, spOK := s.boundedRowSpan(dr.ColIdx, vals); spOK {
				compGap += (yiSum - slackNoise) * span
			} else {
				return false
			}
		}
		if yiSum < -slackNoise {
			res, rTol, ok := evalSparseRowResidualAndTol(dr.ColIdx, vals, primal, unscaledLB, unscaledUB, rhs, verifyFeasTol, 1e-8)
			if !ok {
				return false
			}
			if res < 0 {
				diff := -res
				if diff > rTol {
					compGap += (-yiSum - slackNoise) * (diff - rTol)
				}
			}
		}
	}

	for j := range n {
		dj := djOrig[j]
		if math.IsNaN(dj) || math.IsInf(dj, 0) {
			return false
		}
		lbj := s.unscaledBound(j, false)
		ubj := s.unscaledBound(j, true)
		if lbj == ubj {
			continue
		}
		colL1Sc := yDotSc[j] + math.Ldexp(math.Abs(p.objMin[j]), -32)
		dTolRel := math.Ldexp(1e-7*colL1Sc, 32)
		if math.IsInf(dTolRel, 1) {
			dTolRel = math.MaxFloat64
		}
		dTol := max(verifyFeasTol, max(1e-7, dTolRel))
		if status[j] == statusBasic {
			if math.Abs(dj) > dTol {
				return false
			}
		} else {
			infNoise := combineDualNoiseTol(djErr[j], dy2Sc[j], yDotSc[j], maxBasisDualRel, 16.0)
			if math.IsInf(ubj, 1) && dj < -infNoise {
				return false
			}
			if math.IsInf(lbj, -1) && dj > infNoise {
				return false
			}
			bTol := max(verifyFeasTol, 1e-8*max(1.0, math.Abs(primal[j])))
			if !math.IsInf(ubj, 1) && dj < -dTol && primal[j] < ubj && ubj-primal[j] > bTol {
				return false
			}
			if !math.IsInf(lbj, -1) && dj > dTol && primal[j] > lbj && primal[j]-lbj > bTol {
				return false
			}
		}
		boxNoise := combineDualNoiseTol(djErr[j], dy2Sc[j], yDotSc[j], maxBasisDualRel, 4.0)
		if !math.IsInf(ubj, 1) && dj < -boxNoise && primal[j] < ubj {
			rem := ubj - primal[j]
			if math.IsInf(rem, 1) {
				compGap += math.Ldexp(-dj-boxNoise, 2) * (math.Ldexp(ubj, -2) - math.Ldexp(primal[j], -2))
			} else if rem > 0 {
				compGap += (-dj - boxNoise) * rem
			}
		}
		if !math.IsInf(lbj, -1) && dj > boxNoise && primal[j] > lbj {
			rem := primal[j] - lbj
			if math.IsInf(rem, 1) {
				compGap += math.Ldexp(dj-boxNoise, 2) * (math.Ldexp(primal[j], -2) - math.Ldexp(lbj, -2))
			} else if rem > 0 {
				compGap += (dj - boxNoise) * rem
			}
		}
	}

	if math.IsNaN(compGap) || math.IsInf(compGap, 0) {
		return false
	}
	return compGap <= max(1e-4, 1e-5*max(1.0, math.Abs(objMin)))
}

func (s *LPSolver) primalResidualsWithinTol(primal []float64) bool {
	p := s.prob
	n := p.numVars
	if len(primal) < n {
		return false
	}
	for j := range n {
		v := primal[j]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		lbj := s.unscaledBound(j, false)
		ubj := s.unscaledBound(j, true)
		bTol := max(verifyFeasTol, 1e-8*max(1.0, math.Abs(v)))
		if (!math.IsInf(lbj, -1) && v < lbj && lbj-v > bTol) ||
			(!math.IsInf(ubj, 1) && v > ubj && v-ubj > bTol) {
			return false
		}
	}
	unscaledLB, unscaledUB := s.ws.lb[:n], s.ws.ub[:n]
	if s.scaled {
		unscaledLB, unscaledUB = s.origLB, s.origUB
	}
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		res, rTolLo, rTolHi, ok := evalSparseRowResidualAndDirTol(p.colIdx[st:en], p.rowVal[st:en], primal, unscaledLB, unscaledUB, p.rowRHS[i], verifyFeasTol, 1e-8)
		if !ok {
			return false
		}
		if (!math.IsInf(p.rowLB[i], -1) && res < -rTolLo) ||
			(!math.IsInf(p.rowUB[i], 1) && res > rTolHi) {
			return false
		}
	}
	for _, dr := range s.extraRows {
		vals := dr.Val
		rhs := dr.RHS
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
			rhs = dr.OrigRHS
		}
		res, _, rTolHi, ok := evalSparseRowResidualAndDirTol(dr.ColIdx, vals, primal, unscaledLB, unscaledUB, rhs, verifyFeasTol, 1e-8)
		if !ok {
			return false
		}
		if res > rTolHi {
			return false
		}
	}
	return true
}

func twoSum(a, b float64) (s, err float64) {
	s = a + b
	bb := s - a
	err = (a - (s - bb)) + (b - bb)
	return s, err
}

func twoProd(a, b float64) (p, err float64) {
	p = a * b
	err = math.FMA(a, b, -p)
	return p, err
}

type dot2Acc struct {
	s        float64
	c        float64
	errL1    float64
	absL1    float64
	count    int
	lowCount int
	hasPos   bool
	hasNeg   bool
	nonFin   bool
}

func (acc *dot2Acc) addVal(v float64) {
	if v == 0 {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		acc.nonFin = true
		return
	}
	if v > 0 {
		acc.hasPos = true
	} else {
		acc.hasNeg = true
	}
	ns, se := twoSum(acc.s, v)
	if math.IsNaN(ns) || math.IsInf(ns, 0) {
		acc.nonFin = true
		return
	}
	acc.s = ns
	acc.c += se
	acc.errL1 += math.Abs(se)
	acc.absL1 += math.Abs(v)
	acc.count++
}

func (acc *dot2Acc) addProd(a, b float64) {
	if a == 0 || b == 0 {
		return
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		acc.nonFin = true
		return
	}
	if (a > 0) == (b > 0) {
		acc.hasPos = true
	} else {
		acc.hasNeg = true
	}
	p, pe := twoProd(a, b)
	if math.IsNaN(p) || math.IsInf(p, 0) || math.IsNaN(pe) || math.IsInf(pe, 0) {
		acc.nonFin = true
		return
	}
	if math.Abs(p) < twoProdExactMin {
		acc.lowCount++
	}
	ns, se := twoSum(acc.s, p)
	if math.IsNaN(ns) || math.IsInf(ns, 0) {
		acc.nonFin = true
		return
	}
	acc.s = ns
	q := pe + se
	acc.c += q
	acc.errL1 += math.Abs(pe) + math.Abs(se)
	acc.absL1 += math.Abs(p)
	acc.count++
}

func (acc *dot2Acc) result() (val, errBound float64, ok bool) {
	if acc.nonFin || math.IsNaN(acc.c) || math.IsInf(acc.c, 0) || math.IsNaN(acc.errL1) || math.IsInf(acc.errL1, 0) {
		return 0, 0, false
	}
	if acc.count == 0 {
		return 0, 0, true
	}
	val, finalRem := twoSum(acc.s, acc.c)
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, 0, false
	}
	ku := float64(acc.count+2) * unitRoundoff64
	if ku >= 0.5 {
		return 0, 0, false
	}
	gamma := ku / (1.0 - ku)
	errBound = 2.0*(math.Abs(finalRem)+gamma*acc.errL1) + float64(acc.lowCount)*math.SmallestNonzeroFloat64
	if math.IsNaN(errBound) || math.IsInf(errBound, 0) {
		return 0, 0, false
	}
	return val, errBound, true
}

func safeTermProd(c, x float64) float64 {
	if c == 0 {
		return 0
	}
	return c * x
}

func tightenRowLeVerifyBounds(cols []int, vals []float64, rhs, sign float64, lb, ub []float64) (infeasible bool) {
	d := len(cols)
	if d == 0 {
		return sign*rhs < 0
	}
	missingCnt := 0
	missingIdx := -1
	anyTargetInf := false
	for k := range d {
		c := cols[k]
		ak := sign * vals[k]
		if ak == 0 {
			continue
		}
		if (ak > 0 && math.IsInf(ub[c], 1)) || (ak < 0 && math.IsInf(lb[c], -1)) {
			anyTargetInf = true
		}
		if (ak > 0 && math.IsInf(lb[c], -1)) || (ak < 0 && math.IsInf(ub[c], 1)) {
			missingCnt++
			missingIdx = k
			if missingCnt >= 2 {
				return false
			}
		}
	}
	if !anyTargetInf {
		return false
	}

	applyTarget := func(t int) bool {
		c := cols[t]
		at := sign * vals[t]
		if at == 0 {
			return false
		}
		if at > 0 && !math.IsInf(ub[c], 1) {
			return false
		}
		if at < 0 && !math.IsInf(lb[c], -1) {
			return false
		}
		var acc dot2Acc
		acc.addVal(sign * rhs)
		for k := range d {
			if k == t {
				continue
			}
			ak := sign * vals[k]
			if ak == 0 {
				continue
			}
			ext := lb[cols[k]]
			if ak < 0 {
				ext = ub[cols[k]]
			}
			acc.addProd(-ak, ext)
		}
		numVal, numErr, ok := acc.result()
		if !ok {
			return false
		}
		hi := numVal + numErr
		if numErr > 0 && !math.IsNaN(hi) && !math.IsInf(hi, 0) {
			hi = math.Nextafter(hi, math.Inf(1))
		}
		if math.IsNaN(hi) || math.IsInf(hi, 0) {
			return false
		}
		q := hi / at
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return false
		}
		if at > 0 {
			var bnd float64
			switch {
			case q == 0 && hi <= 0:
				bnd = 0
			case q == 0 && hi > 0:
				bnd = math.SmallestNonzeroFloat64
			default:
				bnd = math.Nextafter(q, math.Inf(1))
			}
			if !math.IsInf(bnd, 1) && bnd < ub[c] {
				ub[c] = bnd
				if lb[c] > ub[c] {
					return true
				}
			}
		} else {
			var bnd float64
			switch {
			case q == 0 && hi <= 0:
				bnd = 0
			case q == 0 && hi > 0:
				bnd = -math.SmallestNonzeroFloat64
			default:
				bnd = math.Nextafter(q, math.Inf(-1))
			}
			if !math.IsInf(bnd, -1) && bnd > lb[c] {
				lb[c] = bnd
				if lb[c] > ub[c] {
					return true
				}
			}
		}
		return false
	}

	if missingCnt == 1 {
		return applyTarget(missingIdx)
	}
	for t := range d {
		if applyTarget(t) {
			return true
		}
	}
	return false
}

func (s *LPSolver) computeVerifyBounds() (effLB, effUB []float64, provenInfeasible, invalid bool) {
	p := s.prob
	n := p.numVars
	effLB = make([]float64, n)
	effUB = make([]float64, n)
	hasInf := false
	for j := range n {
		lbj := s.unscaledBound(j, false)
		ubj := s.unscaledBound(j, true)
		if math.IsNaN(lbj) || math.IsNaN(ubj) {
			return nil, nil, false, true
		}
		if lbj > ubj {
			return nil, nil, true, false
		}
		if math.IsInf(lbj, -1) || math.IsInf(ubj, 1) {
			hasInf = true
		}
		effLB[j] = lbj
		effUB[j] = ubj
	}
	if !hasInf {
		return effLB, effUB, false, false
	}
	for range 2 {
		for i := range p.numConstraints {
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			cols := p.colIdx[st:en]
			vals := p.rowVal[st:en]
			rhs := p.rowRHS[i]
			switch p.rowSense[i] {
			case LessEqual:
				if tightenRowLeVerifyBounds(cols, vals, rhs, 1.0, effLB, effUB) {
					return effLB, effUB, true, false
				}
			case GreaterEqual:
				if tightenRowLeVerifyBounds(cols, vals, rhs, -1.0, effLB, effUB) {
					return effLB, effUB, true, false
				}
			case Equal:
				if tightenRowLeVerifyBounds(cols, vals, rhs, 1.0, effLB, effUB) ||
					tightenRowLeVerifyBounds(cols, vals, rhs, -1.0, effLB, effUB) {
					return effLB, effUB, true, false
				}
			}
		}
		for _, dr := range s.extraRows {
			vals := dr.Val
			rhs := dr.RHS
			if s.scaled && dr.OrigVal != nil {
				vals = dr.OrigVal
				rhs = dr.OrigRHS
			}
			if tightenRowLeVerifyBounds(dr.ColIdx, vals, rhs, 1.0, effLB, effUB) {
				return effLB, effUB, true, false
			}
		}
	}
	return effLB, effUB, false, false
}

func (s *LPSolver) exactColDotSign(w []float64, col int) int {
	p := s.prob
	baseM := p.numConstraints
	sum := new(big.Rat)
	term := new(big.Rat)
	rv := new(big.Rat)
	for q := p.colPtr[col]; q < p.colPtr[col+1]; q++ {
		r := p.rowIdx[q]
		if uint(r) < uint(baseM) && w[r] != 0 && p.colVal[q] != 0 {
			term.SetFloat64(w[r])
			rv.SetFloat64(p.colVal[q])
			term.Mul(term, rv)
			sum.Add(sum, term)
		}
	}
	for e, dr := range s.extraRows {
		wr := w[baseM+e]
		if wr == 0 {
			continue
		}
		vals := dr.Val
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
		}
		for k, c := range dr.ColIdx {
			if c == col && vals[k] != 0 {
				term.SetFloat64(wr)
				rv.SetFloat64(vals[k])
				term.Mul(term, rv)
				sum.Add(sum, term)
			}
		}
	}
	return sum.Sign()
}

// verifyUnscaledFarkas evaluates a basis-free Farkas infeasibility certificate directly in
// original unscaled problem space across all structural columns (basic, nonbasic, and fixed)
// and slacks: max_{x in [lb, ub], s in [slb, sub]} w^T (A x + s) < w^T b.
func (s *LPSolver) verifyUnscaledFarkas(yScaled []float64, sign float64) bool {
	p := s.prob
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()
	if len(yScaled) < m || m == 0 {
		return false
	}

	effLB, effUB, provenInf, invalid := s.computeVerifyBounds()
	if invalid {
		return false
	}
	if provenInf {
		return true
	}

	w := make([]float64, m)
	wNorm := 0.0
	for i := range m {
		wi := sign * yScaled[i] * s.rowScaleOf(i)
		if math.IsNaN(wi) || math.IsInf(wi, 0) {
			return false
		}
		// Project into the exact Farkas dual cone before forming w^T A and w^T b
		// so ~1e-16 BTRAN roundoff on inactive inequality rows never invalidates a certificate.
		if i < baseM {
			switch p.rowSense[i] {
			case LessEqual:
				if wi > 0 {
					wi = 0
				}
			case GreaterEqual:
				if wi < 0 {
					wi = 0
				}
			}
		} else if wi > 0 {
			wi = 0
		}
		w[i] = wi
		if av := math.Abs(wi); av > wNorm {
			wNorm = av
		}
	}
	if wNorm == 0 {
		return false
	}
	_, wExp := math.Frexp(wNorm)
	var bAcc dot2Acc
	for i := range m {
		wi := math.Ldexp(w[i], -wExp)
		w[i] = wi
		bAcc.addProd(wi, s.unscaledRowRHS(i))
	}
	wDotB, wDotBErr, okB := bAcc.result()
	if !okB {
		return false
	}

	colAcc := s.ws.ensureDotAcc(n)
	for j := range n {
		acc := &colAcc[j]
		for q := p.colPtr[j]; q < p.colPtr[j+1]; q++ {
			r := p.rowIdx[q]
			if uint(r) < uint(baseM) {
				acc.addProd(w[r], p.colVal[q])
			}
		}
	}
	for e, dr := range s.extraRows {
		wr := w[baseM+e]
		if wr == 0 {
			continue
		}
		vals := dr.Val
		if s.scaled && dr.OrigVal != nil {
			vals = dr.OrigVal
		}
		for k, c := range dr.ColIdx {
			if uint(c) < uint(n) {
				colAcc[c].addProd(wr, vals[k])
			}
		}
	}

	var lhsAcc dot2Acc
	extraBoundErr := 0.0
	for j := range n {
		lbj := effLB[j]
		ubj := effUB[j]
		if lbj > ubj {
			return true
		}
		acc := &colAcc[j]
		a, errBound, okA := acc.result()
		if !okA {
			return false
		}
		aLo := a - errBound
		aHi := a + errBound
		if math.IsNaN(aLo) || math.IsInf(aLo, 0) || math.IsNaN(aHi) || math.IsInf(aHi, 0) {
			return false
		}
		hasL := !math.IsInf(lbj, -1)
		hasU := !math.IsInf(ubj, 1)
		if !hasU || !hasL {
			if math.Abs(a) <= errBound {
				sgn := s.exactColDotSign(w, j)
				if (!hasU && sgn > 0) || (!hasL && sgn < 0) {
					return false
				}
				if sgn == 0 {
					aLo, aHi = 0, 0
				} else if sgn < 0 {
					aHi = min(aHi, 0)
				} else {
					aLo = max(aLo, 0)
				}
			} else if (!hasU && aHi > 0) || (!hasL && aLo < 0) {
				return false
			}
		}
		var cMax float64
		switch {
		case hasL && hasU:
			cMax = max(max(safeTermProd(aLo, lbj), safeTermProd(aLo, ubj)), max(safeTermProd(aHi, lbj), safeTermProd(aHi, ubj)))
		case hasL:
			cMax = max(safeTermProd(aLo, lbj), safeTermProd(aHi, lbj))
		case hasU:
			cMax = max(safeTermProd(aLo, ubj), safeTermProd(aHi, ubj))
		default:
			cMax = 0
		}
		if math.IsNaN(cMax) || math.IsInf(cMax, 0) {
			return false
		}
		extraBoundErr += 2.0 * unitRoundoff64 * math.Abs(cMax)
		lhsAcc.addVal(cMax)
	}

	for i := range m {
		wi := w[i]
		if wi == 0 {
			continue
		}
		var slb, sub float64
		if i < baseM {
			switch p.rowSense[i] {
			case LessEqual:
				slb, sub = 0, math.Inf(1)
			case GreaterEqual:
				slb, sub = math.Inf(-1), 0
			case Equal:
				slb, sub = 0, 0
			}
		} else {
			slb, sub = 0, math.Inf(1)
		}
		if wi > 0 && math.IsInf(sub, 1) {
			return false
		}
		if wi < 0 && math.IsInf(slb, -1) {
			return false
		}
		sMax := max(safeTermProd(wi, slb), safeTermProd(wi, sub))
		if math.IsNaN(sMax) || math.IsInf(sMax, 0) {
			return false
		}
		extraBoundErr += 2.0 * unitRoundoff64 * math.Abs(sMax)
		lhsAcc.addVal(sMax)
	}

	maxLHS, lhsErr, okLHS := lhsAcc.result()
	if !okLHS || math.IsNaN(extraBoundErr) || math.IsInf(extraBoundErr, 0) {
		return false
	}
	tol := max(1e-9, 1e-10*(bAcc.absL1+lhsAcc.absL1))
	return maxLHS+lhsErr+extraBoundErr < wDotB-wDotBErr-tol
}

// verifyExactRationalFarkas solves the unscaled transpose basis system B_orig^T w = d in exact
// rational arithmetic (peeling slack and singleton basis equations before factoring any remaining
// nucleus of size <= 64) when floating-point BTRAN rounding prevents certifying an infeasible LP
// containing free or unbounded columns with non-dyadic multipliers.
func (s *LPSolver) verifyExactRationalFarkas(unscaledRHS []float64) bool {
	const maxExactRatRows = 64
	p := s.prob
	n := p.numVars
	baseM := p.numConstraints
	m := s.TotalRows()
	if m == 0 || len(unscaledRHS) < m {
		return false
	}
	effLB, effUB, provenInf, invalid := s.computeVerifyBounds()
	if invalid {
		return false
	}
	if provenInf {
		return true
	}

	type ratEntry struct {
		col int
		val *big.Rat
	}
	eqEntries := make([][]ratEntry, m)
	eqRHS := make([]*big.Rat, m)
	colEqs := make([][]int, m)
	eqActive := make([]bool, m)
	eqDeg := make([]int, m)
	varDeg := make([]int, m)

	for q := range m {
		bVar := s.ws.basis[q]
		r := new(big.Rat)
		if unscaledRHS[q] != 0 {
			rhsVal := unscaledRHS[q]
			if s.scaled {
				if bVar < n {
					rhsVal *= s.invColScale[bVar]
				} else {
					rhsVal *= s.rowScaleOf(bVar - n)
				}
			}
			if math.IsNaN(rhsVal) || math.IsInf(rhsVal, 0) {
				return false
			}
			r.SetFloat64(rhsVal)
		}
		eqRHS[q] = r
		if bVar < n {
			var ents []ratEntry
			for idx := p.colPtr[bVar]; idx < p.colPtr[bVar+1]; idx++ {
				ri := p.rowIdx[idx]
				if uint(ri) < uint(baseM) && p.colVal[idx] != 0 {
					rv := new(big.Rat).SetFloat64(p.colVal[idx])
					ents = append(ents, ratEntry{col: ri, val: rv})
				}
			}
			for e, dr := range s.extraRows {
				vals := dr.Val
				if s.scaled && dr.OrigVal != nil {
					vals = dr.OrigVal
				}
				for k, c := range dr.ColIdx {
					if c == bVar && vals[k] != 0 {
						rv := new(big.Rat).SetFloat64(vals[k])
						ents = append(ents, ratEntry{col: baseM + e, val: rv})
					}
				}
			}
			eqEntries[q] = ents
		} else {
			si := bVar - n
			if uint(si) >= uint(m) {
				return false
			}
			eqEntries[q] = []ratEntry{{col: si, val: big.NewRat(1, 1)}}
		}
		if len(eqEntries[q]) == 0 {
			return false
		}
		eqActive[q] = true
		eqDeg[q] = len(eqEntries[q])
		for _, ent := range eqEntries[q] {
			colEqs[ent.col] = append(colEqs[ent.col], q)
			varDeg[ent.col]++
		}
	}

	w := make([]*big.Rat, m)
	for i := range m {
		w[i] = new(big.Rat)
	}
	varKnown := make([]bool, m)
	varDeferred := make([]bool, m)
	type backStep struct {
		eq  int
		col int
	}
	var backStack []backStep
	var rowQ, colQ []int
	for q := range m {
		if eqDeg[q] == 1 {
			rowQ = append(rowQ, q)
		}
	}
	for i := range m {
		if varDeg[i] == 0 {
			return false
		}
		if varDeg[i] == 1 {
			colQ = append(colQ, i)
		}
	}

	tmp := new(big.Rat)
	accR := new(big.Rat)
	for len(rowQ) > 0 || len(colQ) > 0 {
		for len(rowQ) > 0 {
			q := rowQ[len(rowQ)-1]
			rowQ = rowQ[:len(rowQ)-1]
			if !eqActive[q] || eqDeg[q] != 1 {
				continue
			}
			targetCol := -1
			var targetVal *big.Rat
			accR.Set(eqRHS[q])
			for _, ent := range eqEntries[q] {
				if varKnown[ent.col] {
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						accR.Sub(accR, tmp)
					}
				} else if !varDeferred[ent.col] {
					targetCol = ent.col
					targetVal = ent.val
				}
			}
			if targetCol < 0 || targetVal.Sign() == 0 {
				return false
			}
			w[targetCol].Quo(accR, targetVal)
			varKnown[targetCol] = true
			eqActive[q] = false
			for _, q2 := range colEqs[targetCol] {
				if !eqActive[q2] {
					continue
				}
				eqDeg[q2]--
				if eqDeg[q2] == 1 {
					rowQ = append(rowQ, q2)
				}
			}
		}
		for len(colQ) > 0 && len(rowQ) == 0 {
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
				return false
			}
			backStack = append(backStack, backStep{eq: actEq, col: c})
			varDeferred[c] = true
			eqActive[actEq] = false
			for _, ent := range eqEntries[actEq] {
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
	if len(nucEqs) != nNuc || nNuc > maxExactRatRows {
		return false
	}

	if nNuc > 0 {
		mat := make([][]*big.Rat, nNuc)
		for rowIdx, q := range nucEqs {
			row := make([]*big.Rat, nNuc+1)
			for k := range nNuc {
				row[k] = new(big.Rat)
			}
			rhsR := new(big.Rat).Set(eqRHS[q])
			for _, ent := range eqEntries[q] {
				if varKnown[ent.col] {
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						rhsR.Sub(rhsR, tmp)
					}
				} else if pos := nucVarPos[ent.col]; pos >= 0 {
					row[pos].Set(ent.val)
				} else {
					return false
				}
			}
			row[nNuc] = rhsR
			mat[rowIdx] = row
		}

		factor := new(big.Rat)
		for col := range nNuc {
			piv := -1
			for r := col; r < nNuc; r++ {
				if mat[r][col].Sign() != 0 {
					piv = r
					break
				}
			}
			if piv == -1 {
				return false
			}
			if piv != col {
				mat[col], mat[piv] = mat[piv], mat[col]
			}
			invPiv := new(big.Rat).Inv(mat[col][col])
			for c := col; c <= nNuc; c++ {
				if mat[col][c].Sign() != 0 {
					mat[col][c].Mul(mat[col][c], invPiv)
				}
			}
			for r := range nNuc {
				if r == col || mat[r][col].Sign() == 0 {
					continue
				}
				factor.Set(mat[r][col])
				mat[r][col].SetInt64(0)
				for c := col + 1; c <= nNuc; c++ {
					if mat[col][c].Sign() != 0 {
						tmp.Mul(factor, mat[col][c])
						mat[r][c].Sub(mat[r][c], tmp)
					}
				}
			}
		}
		for k, origCol := range nucVars {
			w[origCol].Set(mat[k][nNuc])
			varKnown[origCol] = true
		}
	}

	for _, bs := range slices.Backward(backStack) {
		accR.Set(eqRHS[bs.eq])
		var pivVal *big.Rat
		for _, ent := range eqEntries[bs.eq] {
			if ent.col == bs.col {
				pivVal = ent.val
			} else if varKnown[ent.col] {
				if w[ent.col].Sign() != 0 {
					tmp.Mul(ent.val, w[ent.col])
					accR.Sub(accR, tmp)
				}
			} else {
				return false
			}
		}
		if pivVal == nil || pivVal.Sign() == 0 {
			return false
		}
		w[bs.col].Quo(accR, pivVal)
		varKnown[bs.col] = true
	}

	for q := range m {
		if eqActive[q] && eqDeg[q] == 0 {
			accR.Set(eqRHS[q])
			for _, ent := range eqEntries[q] {
				if w[ent.col].Sign() != 0 {
					tmp.Mul(ent.val, w[ent.col])
					accR.Sub(accR, tmp)
				}
			}
			if accR.Sign() != 0 {
				return false
			}
		}
	}

	anyNonZero := false
	wDotB := new(big.Rat)
	for i := range m {
		wi := w[i]
		if i < baseM {
			switch p.rowSense[i] {
			case LessEqual:
				if wi.Sign() > 0 {
					wi.SetInt64(0)
				}
			case GreaterEqual:
				if wi.Sign() < 0 {
					wi.SetInt64(0)
				}
			}
		} else if wi.Sign() > 0 {
			wi.SetInt64(0)
		}
		if wi.Sign() != 0 {
			anyNonZero = true
			rhs := s.unscaledRowRHS(i)
			if rhs != 0 {
				tmp.SetFloat64(rhs)
				tmp.Mul(tmp, wi)
				wDotB.Add(wDotB, tmp)
			}
		}
	}
	if !anyNonZero {
		return false
	}

	maxLHS := new(big.Rat)
	aj := new(big.Rat)
	b1 := new(big.Rat)
	b2 := new(big.Rat)
	for j := range n {
		lbj, ubj := effLB[j], effUB[j]
		if lbj > ubj {
			return true
		}
		aj.SetInt64(0)
		for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
			ri := p.rowIdx[idx]
			if uint(ri) < uint(baseM) && w[ri].Sign() != 0 && p.colVal[idx] != 0 {
				tmp.SetFloat64(p.colVal[idx])
				tmp.Mul(tmp, w[ri])
				aj.Add(aj, tmp)
			}
		}
		for e, dr := range s.extraRows {
			if w[baseM+e].Sign() == 0 {
				continue
			}
			vals := dr.Val
			if s.scaled && dr.OrigVal != nil {
				vals = dr.OrigVal
			}
			for k, c := range dr.ColIdx {
				if c == j && vals[k] != 0 {
					tmp.SetFloat64(vals[k])
					tmp.Mul(tmp, w[baseM+e])
					aj.Add(aj, tmp)
				}
			}
		}
		sgn := aj.Sign()
		if sgn == 0 {
			continue
		}
		hasL := !math.IsInf(lbj, -1)
		hasU := !math.IsInf(ubj, 1)
		if (!hasU && sgn > 0) || (!hasL && sgn < 0) {
			return false
		}
		switch {
		case hasL && hasU:
			b1.SetFloat64(lbj)
			b1.Mul(b1, aj)
			b2.SetFloat64(ubj)
			b2.Mul(b2, aj)
			if b2.Cmp(b1) > 0 {
				maxLHS.Add(maxLHS, b2)
			} else {
				maxLHS.Add(maxLHS, b1)
			}
		case hasL:
			b1.SetFloat64(lbj)
			b1.Mul(b1, aj)
			maxLHS.Add(maxLHS, b1)
		case hasU:
			b2.SetFloat64(ubj)
			b2.Mul(b2, aj)
			maxLHS.Add(maxLHS, b2)
		}
	}
	return maxLHS.Cmp(wDotB) < 0
}

func (s *LPSolver) addUnboundedBasisTilt(rhs []float64, skipRow int, sign float64) bool {
	ws := s.ws
	n := s.prob.numVars
	m := s.TotalRows()
	basis := ws.basis[:m]
	tilted := false
	const tiltMag = 1e-11
	for q, bVar := range basis {
		if q == skipRow || bVar >= n {
			continue
		}
		hasL := !math.IsInf(ws.lb[bVar], -1)
		hasU := !math.IsInf(ws.ub[bVar], 1)
		if hasL && !hasU {
			rhs[q] -= sign * tiltMag
			tilted = true
		} else if !hasL && hasU {
			rhs[q] += sign * tiltMag
			tilted = true
		}
	}
	return tilted
}

// verifyDualInfeasibleRow confirms a dual-simplex Farkas infeasibility certificate in
// original unscaled problem space across all columns using an un-dropped BTRAN solve.
func (s *LPSolver) verifyDualInfeasibleRow(leaveRow int, leaveDir float64) bool {
	ws := s.ws
	m := s.TotalRows()
	if uint(leaveRow) >= uint(m) {
		return false
	}

	sign := 1.0
	if leaveDir < 0 {
		sign = -1.0
	}
	rho := make([]float64, m)
	rho[leaveRow] = 1.0
	ws.lu.BTRANExact(rho)
	if s.verifyUnscaledFarkas(rho, sign) {
		return true
	}

	clear(rho)
	rho[leaveRow] = 1.0
	if s.addUnboundedBasisTilt(rho, leaveRow, sign) {
		ws.lu.BTRANExact(rho)
		if s.verifyUnscaledFarkas(rho, sign) {
			return true
		}
	}

	clear(rho)
	rho[leaveRow] = sign
	return s.verifyExactRationalFarkas(rho)
}

// verifyPhase1Infeasible confirms a Phase I infeasibility certificate in original
// unscaled problem space across all columns using an un-dropped BTRAN solve.
func (s *LPSolver) verifyPhase1Infeasible() bool {
	ws := s.ws
	m := s.TotalRows()
	y := make([]float64, m)
	for i := range m {
		y[i] = ws.workCost[ws.basis[i]]
	}
	ws.lu.BTRANExact(y)
	if s.verifyUnscaledFarkas(y, 1.0) {
		return true
	}

	for i := range m {
		y[i] = ws.workCost[ws.basis[i]]
	}
	if s.addUnboundedBasisTilt(y, -1, 1.0) {
		ws.lu.BTRANExact(y)
		if s.verifyUnscaledFarkas(y, 1.0) {
			return true
		}
	}

	for i := range m {
		y[i] = ws.workCost[ws.basis[i]]
	}
	return s.verifyExactRationalFarkas(y)
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
