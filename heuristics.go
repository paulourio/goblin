package goblin

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	intFeasTol               = 1e-6
	heurCoeffZeroTol         = 1e-12
	heurIntBoundTol          = 1e-7
	heurDedupTol             = 1e-9
	heurSnapNearFrac         = 0.15
	minPropSteps             = 32
	propStepsPerRow          = 3
	rowQueueCapMult          = 2
	maxDiveStepsCap          = 18
	maxEqSupportRowNNZ       = 32
	heurSubLPSkipFracCap     = 32
	heurCompactLPMaxVars     = 128
	heurNoAuxSkipVars        = 256
	heurSubMIPBaseFracCap    = 120
	heurSubMIPFreeFracCap    = 240
	heurSubMIPSmallModelVars = 200
	heurSubMIPMaxNodes       = 800
	heurSubMIPFallbackNodes  = 120
	heurSubMIPCutPasses      = 3
	nodePropPasses           = 2
	subMIPPresolvePasses     = 4
	subMIPTimeLimitMillis    = 1500
	ownerUnassigned          = -2
	ownerPureEpigraph        = -1
)

// propagateRow tightens variable bounds on row i from residual row activities and reports
// whether the row remains feasible and whether any bound shrunk by more than heurIntBoundTol.
func propagateRow(p *CompiledProblem, i int, lb, ub []float64, pushCol func(col int)) (feasible, changed bool) {
	s, e := p.rowPtr[i], p.rowPtr[i+1]
	rhs := p.rowRHS[i]
	sense := p.rowSense[i]
	hasUB := sense == LessEqual || sense == Equal
	hasLB := sense == GreaterEqual || sense == Equal

	minAct := 0.0
	maxAct := 0.0
	minInf := 0
	maxInf := 0

	for k := s; k < e; k++ {
		col := p.colIdx[k]
		a := p.rowVal[k]
		l, u := lb[col], ub[col]
		if a > 0 {
			if math.IsInf(l, -1) {
				minInf++
			} else {
				minAct += a * l
			}
			if math.IsInf(u, 1) {
				maxInf++
			} else {
				maxAct += a * u
			}
		} else if a < 0 {
			if math.IsInf(u, 1) {
				minInf++
			} else {
				minAct += a * u
			}
			if math.IsInf(l, -1) {
				maxInf++
			} else {
				maxAct += a * l
			}
		}
		if (!hasUB || minInf > 0) && (!hasLB || maxInf > 0) {
			return true, false
		}
	}

	rowScale := max(1.0, max(math.Abs(rhs), max(math.Abs(minAct), math.Abs(maxAct))))
	rowFeasTol := max(intFeasTol, presolveRelScaleTol*rowScale)

	if hasUB && minInf == 0 && minAct > rhs+rowFeasTol {
		return false, false
	}
	if hasLB && maxInf == 0 && maxAct < rhs-rowFeasTol {
		return false, false
	}

	if hasUB && minInf == 0 {
		slack := rhs - minAct
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			a := p.rowVal[k]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				newUB := l + slack/a
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newUB = math.Floor(newUB + bTol)
				}
				if newUB < u-heurIntBoundTol {
					if newUB < l-intFeasTol {
						return false, false
					}
					ub[col] = max(l, newUB)
					changed = true
					if pushCol != nil {
						pushCol(col)
					}
				}
			} else if a < -heurCoeffZeroTol {
				newLB := u + slack/a
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/(-a))
					newLB = math.Ceil(newLB - bTol)
				}
				if newLB > l+heurIntBoundTol {
					if newLB > u+intFeasTol {
						return false, false
					}
					lb[col] = min(u, newLB)
					changed = true
					if pushCol != nil {
						pushCol(col)
					}
				}
			}
		}
	}

	if hasLB && maxInf == 0 {
		surplus := maxAct - rhs
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			a := p.rowVal[k]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				newLB := u - surplus/a
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newLB = math.Ceil(newLB - bTol)
				}
				if newLB > l+heurIntBoundTol {
					if newLB > u+intFeasTol {
						return false, false
					}
					lb[col] = min(u, newLB)
					changed = true
					if pushCol != nil {
						pushCol(col)
					}
				}
			} else if a < -heurCoeffZeroTol {
				newUB := l + surplus/(-a)
				if p.varCat[col] != Continuous {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/(-a))
					newUB = math.Floor(newUB + bTol)
				}
				if newUB < u-heurIntBoundTol {
					if newUB < l-intFeasTol {
						return false, false
					}
					ub[col] = max(l, newUB)
					changed = true
					if pushCol != nil {
						pushCol(col)
					}
				}
			}
		}
	}
	return true, changed
}

func propagateBounds(p *CompiledProblem, lb, ub []float64, maxPasses int) bool {
	m := p.numConstraints
	for range maxPasses {
		anyChanged := false
		for i := range m {
			feas, chg := propagateRow(p, i, lb, ub, nil)
			if !feas {
				return false
			}
			if chg {
				anyChanged = true
			}
		}
		if !anyChanged {
			break
		}
	}
	return true
}

func propagateFromCol(p *CompiledProblem, lb, ub []float64, startCol int, queue *[]int, inQueue []bool) bool {
	q := (*queue)[:0]
	pushCol := func(col int) {
		for k := p.colPtr[col]; k < p.colPtr[col+1]; k++ {
			r := p.rowIdx[k]
			if !inQueue[r] {
				inQueue[r] = true
				q = append(q, r)
			}
		}
	}
	pushCol(startCol)

	head := 0
	maxSteps := max(minPropSteps, propStepsPerRow*p.numConstraints)
	for head < len(q) && head < maxSteps {
		r := q[head]
		head++
		inQueue[r] = false
		feas, _ := propagateRow(p, r, lb, ub, pushCol)
		if !feas {
			for ; head < len(q); head++ {
				inQueue[q[head]] = false
			}
			*queue = q[:0]
			return false
		}
	}
	for ; head < len(q); head++ {
		inQueue[q[head]] = false
	}
	*queue = q[:0]
	return true
}

// findContLinkedIntCols marks general integer columns whose nonzeros appear only in mixed rows
// coupling continuous variables with a single integer variable (such as roundness-penalty multiples).
func findContLinkedIntCols(p *CompiledProblem) []bool {
	if len(p.intCols) == 0 || len(p.intCols) == p.numVars {
		return nil
	}
	rowHasCont := make([]bool, p.numConstraints)
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		nCont, nInt := 0, 0
		for k := s; k < e; k++ {
			if p.varCat[p.colIdx[k]] == Continuous {
				nCont++
			} else {
				nInt++
			}
		}
		if nCont > 0 && nInt == 1 {
			rowHasCont[i] = true
		}
	}
	var isContLinked []bool
	for _, col := range p.intCols {
		if p.varCat[col] != Integer {
			continue
		}
		s, e := p.colPtr[col], p.colPtr[col+1]
		if s == e {
			continue
		}
		allContRows := true
		for k := s; k < e; k++ {
			if !rowHasCont[p.rowIdx[k]] {
				allContRows = false
				break
			}
		}
		if allContRows {
			if isContLinked == nil {
				isContLinked = make([]bool, p.numVars)
			}
			isContLinked[col] = true
		}
	}
	return isContLinked
}

// tryPropagatedRounding rounds fractional integer columns in ascending fractionality order
// with queue-driven bound propagation, testing nearest and objective-directed rounding strategies.
func tryPropagatedRounding(
	p *CompiledProblem,
	lpPrimal []float64,
	curLB, curUB []float64,
	auxSolver *LPSolver,
) (sol []float64, objMin float64, ok bool) {
	n := p.numVars
	hasCont := len(p.intCols) < n
	isContLinked := findContLinkedIntCols(p)

	type cand struct {
		col  int
		frac float64
	}
	cands := make([]cand, 0, len(p.intCols))
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		v := lpPrimal[col]
		f := math.Abs(v - math.Round(v))
		cands = append(cands, cand{col: col, frac: f})
	}
	slices.SortFunc(cands, func(a, b cand) int {
		return cmp.Compare(a.frac, b.frac)
	})

	baseHLB := slices.Clone(curLB)
	baseHUB := slices.Clone(curUB)
	firstFrac := 0
	for firstFrac < len(cands) && cands[firstFrac].frac <= intFeasTol {
		col := cands[firstFrac].col
		iv := min(max(math.Round(lpPrimal[col]), baseHLB[col]), baseHUB[col])
		baseHLB[col] = iv
		baseHUB[col] = iv
		firstFrac++
	}
	if firstFrac > 0 {
		if !propagateBounds(p, baseHLB, baseHUB, 1) {
			return nil, 0, false
		}
	}
	fracCands := cands[firstFrac:]
	lpFallbackSolver := auxSolver
	if hasCont && len(fracCands) > heurSubLPSkipFracCap {
		lpFallbackSolver = nil
	}

	hlb := make([]float64, n)
	hub := make([]float64, n)
	candX := make([]float64, n)
	rowQueue := make([]int, 0, p.numConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.numConstraints)

	var bestSol []float64
	bestObj := math.Inf(1)

	maxStrat := 2
	if len(fracCands) == 0 {
		maxStrat = 1
	}

	for strategy := range maxStrat {
		if auxSolver != nil && auxSolver.timeExpired() {
			break
		}
		copy(hlb, baseHLB)
		copy(hub, baseHUB)
		feasible := true

		for idx, c := range fracCands {
			if (idx&15 == 0) && auxSolver != nil && auxSolver.timeExpired() {
				feasible = false
				break
			}
			col := c.col
			if hlb[col] == hub[col] {
				continue
			}
			v := min(max(lpPrimal[col], hlb[col]), hub[col])
			var target float64
			if strategy == 0 || math.Abs(v-math.Round(v)) <= heurSnapNearFrac {
				target = math.Round(v)
			} else {
				if p.objMin[col] > heurCoeffZeroTol {
					target = math.Floor(v)
				} else if p.objMin[col] < -heurCoeffZeroTol {
					target = math.Ceil(v)
				} else if v >= halfFracTarget {
					target = math.Floor(v)
				} else {
					target = math.Ceil(v)
				}
			}
			target = min(max(target, hlb[col]), hub[col])

			savedLB := hlb[col]
			savedUB := hub[col]
			hlb[col] = target
			hub[col] = target

			if !propagateFromCol(p, hlb, hub, col, &rowQueue, inQueue) {
				alt := math.Ceil(v)
				if target == alt {
					alt = math.Floor(v)
				}
				if alt >= savedLB && alt <= savedUB {
					hlb[col] = alt
					hub[col] = alt
					if !propagateFromCol(p, hlb, hub, col, &rowQueue, inQueue) {
						feasible = false
						break
					}
				} else {
					feasible = false
					break
				}
			}
		}

		if !feasible {
			continue
		}

		if !hasCont {
			for j := range n {
				candX[j] = hlb[j]
			}
			bV, rV, iV := p.MaxViolation(candX, true)
			if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
				obj := 0.0
				for j := range n {
					obj += p.objMin[j] * candX[j]
				}
				if obj < bestObj {
					bestObj = obj
					bestSol = slices.Clone(candX)
				}
			}
		} else if auxSolver != nil {
			if sSol, sObj, sOk := solveFixedIntContinuousLP(p, hlb, hub, lpFallbackSolver); sOk && sObj < bestObj {
				bestObj = sObj
				bestSol = sSol
			}
		}
	}

	if bestSol != nil {
		return bestSol, bestObj, true
	}
	return nil, 0, false
}

type epigraphRowTerm struct {
	epiCol int
	c      float64
	alpha  float64
	beta   float64
}

// solveEpigraphContinuousLP solves the fixed-integer continuous (plus ContLinked integer)
// subproblem in closed form when every active constraint row is an epigraph lower bound
// z >= c + alpha * m + beta * n coupling an epigraph variable z (ObjMin[z] > 0, UB = +Inf)
// with at most one non-epigraph continuous variable m and one ContLinked integer variable n.
func solveEpigraphContinuousLP(p *CompiledProblem, hlb, hub []float64) ([]float64, float64, bool) {
	n := p.numVars
	m := p.numConstraints

	// Check if all free variables are either Continuous or ContLinked integers.
	isContLinked := findContLinkedIntCols(p)
	for _, col := range p.intCols {
		if hlb[col] < hub[col] && (isContLinked == nil || !isContLinked[col]) {
			return nil, 0, false
		}
	}

	// Identify candidate epigraph continuous variables.
	isEpi := make([]bool, n)
	for j := range n {
		if hlb[j] < hub[j] && p.varCat[j] == Continuous && p.objMin[j] > heurCoeffZeroTol &&
			!math.IsInf(hlb[j], -1) && math.IsInf(hub[j], 1) {
			isEpi[j] = true
		}
	}

	// Verify active rows and refine isEpi: in every active row touching z, z must be lower-bounded.
	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		hasFree := false
		for k := s; k < e; k++ {
			if col := p.colIdx[k]; hlb[col] < hub[col] {
				hasFree = true
				break
			}
		}
		if !hasFree {
			continue
		}
		sense := p.rowSense[i]
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if !isEpi[col] {
				continue
			}
			a := p.rowVal[k]
			if !((sense == GreaterEqual && a > heurCoeffZeroTol) || (sense == LessEqual && a < -heurCoeffZeroTol)) {
				isEpi[col] = false
			}
		}
	}

	// Map each non-epigraph free variable into an independent component around contCol.
	// Sentinel values in epiOwner and intOwner: ownerUnassigned (-2) before first row visit,
	// ownerPureEpigraph (-1) for epigraph columns with no partner variables, or contCol >= 0.
	epiOwner := make([]int, n)
	intOwner := make([]int, n)
	contIntPartner := make([]int, n)
	for j := range n {
		epiOwner[j] = ownerUnassigned
		intOwner[j] = ownerUnassigned
		contIntPartner[j] = -1
	}

	compIdxByCont := make([]int, n)
	for j := range n {
		compIdxByCont[j] = -1
	}
	var comps []epigraphComponent
	var pureEpiMaxLB []float64

	getComp := func(contCol int) int {
		idx := compIdxByCont[contCol]
		if idx == -1 {
			idx = len(comps)
			compIdxByCont[contCol] = idx
			comps = append(comps, epigraphComponent{contCol: contCol, intCol: -1})
		}
		return idx
	}

	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		fixedAct := 0.0
		epiCol := -1
		contCol := -1
		intCol := -1
		freeCount := 0
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if hlb[col] == hub[col] {
				fixedAct += val * hlb[col]
				continue
			}
			freeCount++
			switch {
			case isEpi[col] && epiCol == -1:
				epiCol = col
			case p.varCat[col] == Continuous && !isEpi[col] && contCol == -1:
				contCol = col
			case p.varCat[col] != Continuous && intCol == -1:
				intCol = col
			default:
				return nil, 0, false
			}
		}
		sense := p.rowSense[i]
		if freeCount == 0 {
			switch sense {
			case LessEqual:
				if fixedAct > p.rowRHS[i]+intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if fixedAct < p.rowRHS[i]-intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(fixedAct-p.rowRHS[i]) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		if epiCol == -1 {
			return nil, 0, false
		}
		var aEpi, aCont, aInt float64
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			switch col {
			case epiCol:
				aEpi = p.rowVal[k]
			case contCol:
				aCont = p.rowVal[k]
			case intCol:
				aInt = p.rowVal[k]
			}
		}
		cTerm := (p.rowRHS[i] - fixedAct) / aEpi
		alpha := -aCont / aEpi
		beta := -aInt / aEpi

		if contCol == -1 && intCol == -1 {
			if epiOwner[epiCol] != ownerUnassigned && epiOwner[epiCol] != ownerPureEpigraph {
				return nil, 0, false
			}
			epiOwner[epiCol] = ownerPureEpigraph
			if pureEpiMaxLB == nil {
				pureEpiMaxLB = slices.Clone(hlb)
			}
			if cTerm > pureEpiMaxLB[epiCol] {
				pureEpiMaxLB[epiCol] = cTerm
			}
			continue
		}
		if contCol == -1 {
			return nil, 0, false
		}
		if epiOwner[epiCol] == ownerUnassigned {
			epiOwner[epiCol] = contCol
		} else if epiOwner[epiCol] != contCol {
			return nil, 0, false
		}
		cIdx := getComp(contCol)
		if intCol != -1 {
			if intOwner[intCol] == ownerUnassigned {
				intOwner[intCol] = contCol
			} else if intOwner[intCol] != contCol {
				return nil, 0, false
			}
			if comps[cIdx].intCol == -1 {
				comps[cIdx].intCol = intCol
				contIntPartner[contCol] = intCol
			} else if comps[cIdx].intCol != intCol {
				return nil, 0, false
			}
		}
		if !slices.Contains(comps[cIdx].epiCols, epiCol) {
			comps[cIdx].epiCols = append(comps[cIdx].epiCols, epiCol)
		}
		comps[cIdx].rows = append(comps[cIdx].rows, epigraphRowTerm{
			epiCol: epiCol,
			c:      cTerm,
			alpha:  alpha,
			beta:   beta,
		})
	}

	cand := slices.Clone(hlb)
	for j := range n {
		if hlb[j] < hub[j] {
			if pureEpiMaxLB != nil && epiOwner[j] == ownerPureEpigraph {
				cand[j] = pureEpiMaxLB[j]
			} else if p.objMin[j] < 0 {
				if math.IsInf(hub[j], 1) {
					return nil, 0, false
				}
				cand[j] = hub[j]
			} else if math.IsInf(hlb[j], -1) {
				if !math.IsInf(hub[j], 1) && p.objMin[j] == 0 {
					cand[j] = hub[j]
				} else {
					cand[j] = 0
				}
			}
		}
	}

	for idx := range comps {
		solveEpigraphComponent(p, &comps[idx], hlb, hub, cand)
	}

	bV, rV, iV := p.MaxViolation(cand, true)
	if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
		obj := 0.0
		for j := range n {
			obj += p.objMin[j] * cand[j]
		}
		return cand, obj, true
	}
	return nil, 0, false
}

type epigraphComponent struct {
	contCol int
	intCol  int
	epiCols []int
	rows    []epigraphRowTerm
}

// solveEpigraphComponent enumerates piecewise-linear breakpoints (m, n) of one independent epigraph component and writes the optimal assignment into cand.
func solveEpigraphComponent(p *CompiledProblem, comp *epigraphComponent, hlb, hub, cand []float64) {
	mCol := comp.contCol
	nCol := comp.intCol
	mLB, mUB := hlb[mCol], hub[mCol]

	var mCands []float64
	addMCand := func(v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		v = min(max(v, mLB), mUB)
		for _, ex := range mCands {
			if math.Abs(ex-v) <= heurDedupTol {
				return
			}
		}
		mCands = append(mCands, v)
	}
	addMCand(mLB)
	addMCand(mUB)
	addMCand(0.0)

	rows := comp.rows
	for r1 := range rows {
		if math.Abs(rows[r1].beta) <= heurCoeffZeroTol && math.Abs(rows[r1].alpha) > heurCoeffZeroTol {
			addMCand((hlb[rows[r1].epiCol] - rows[r1].c) / rows[r1].alpha)
		}
		for r2 := r1 + 1; r2 < len(rows); r2++ {
			if rows[r1].epiCol == rows[r2].epiCol &&
				math.Abs(rows[r1].beta) <= heurCoeffZeroTol && math.Abs(rows[r2].beta) <= heurCoeffZeroTol {
				da := rows[r1].alpha - rows[r2].alpha
				if math.Abs(da) > heurCoeffZeroTol {
					addMCand((rows[r2].c - rows[r1].c) / da)
				}
			}
		}
	}

	var nCands []float64
	if nCol != -1 {
		nLB, nUB := hlb[nCol], hub[nCol]
		addNCand := func(v float64) {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
			v = min(max(math.Round(v), nLB), nUB)
			if slices.Contains(nCands, v) {
				return
			}
			nCands = append(nCands, v)
		}
		addNCand(nLB)
		addNCand(nUB)
		baseMCands := slices.Clone(mCands)
		for _, mv := range baseMCands {
			for _, r := range rows {
				if math.Abs(r.beta) > heurCoeffZeroTol {
					nt := (hlb[r.epiCol] - r.c - r.alpha*mv) / r.beta
					addNCand(math.Floor(nt))
					addNCand(math.Ceil(nt))
				}
			}
		}
		for _, nv := range nCands {
			for r1 := range rows {
				if math.Abs(rows[r1].beta) > heurCoeffZeroTol && math.Abs(rows[r1].alpha) > heurCoeffZeroTol {
					addMCand((hlb[rows[r1].epiCol] - rows[r1].c - rows[r1].beta*nv) / rows[r1].alpha)
				}
				for r2 := r1 + 1; r2 < len(rows); r2++ {
					if rows[r1].epiCol == rows[r2].epiCol &&
						(math.Abs(rows[r1].beta) > heurCoeffZeroTol || math.Abs(rows[r2].beta) > heurCoeffZeroTol) {
						da := rows[r1].alpha - rows[r2].alpha
						if math.Abs(da) > heurCoeffZeroTol {
							c1 := rows[r1].c + rows[r1].beta*nv
							c2 := rows[r2].c + rows[r2].beta*nv
							addMCand((c2 - c1) / da)
						}
					}
				}
			}
		}
	} else {
		nCands = []float64{0.0}
	}

	bestCompObj := math.Inf(1)
	bestM := mCands[0]
	bestN := nCands[0]
	epiMax := make([]float64, len(comp.epiCols))
	bestEpiVals := make([]float64, len(comp.epiCols))

	for _, mv := range mCands {
		for _, nv := range nCands {
			for ei, ec := range comp.epiCols {
				epiMax[ei] = hlb[ec]
			}
			for _, r := range rows {
				val := r.c + r.alpha*mv + r.beta*nv
				for ei, ec := range comp.epiCols {
					if ec == r.epiCol {
						if val > epiMax[ei] {
							epiMax[ei] = val
						}
						break
					}
				}
			}
			obj := p.objMin[mCol] * mv
			if nCol != -1 {
				obj += p.objMin[nCol] * nv
			}
			for ei, ec := range comp.epiCols {
				obj += p.objMin[ec] * epiMax[ei]
			}
			if obj < bestCompObj {
				bestCompObj = obj
				bestM = mv
				bestN = nv
				copy(bestEpiVals, epiMax)
			}
		}
	}

	cand[mCol] = bestM
	if nCol != -1 {
		cand[nCol] = bestN
	}
	for ei, ec := range comp.epiCols {
		cand[ec] = bestEpiVals[ei]
	}
}

// trySingleRowContinuousCompletion solves separable single-row bounded continuous knapsacks in closed form when every free column appears in at most one row.
func trySingleRowContinuousCompletion(p *CompiledProblem, hlb, hub []float64) ([]float64, float64, bool) {
	n := p.numVars
	cand := slices.Clone(hlb)
	allSingleRow := true
	for j := range n {
		if hlb[j] < hub[j] {
			if p.objMin[j] < 0 {
				cand[j] = hub[j]
			}
			if p.colPtr[j+1]-p.colPtr[j] > 1 {
				allSingleRow = false
			}
		}
	}
	if allSingleRow {
		type rowMove struct {
			col         int
			absA        float64
			dxSign      float64
			maxRoom     float64
			unitPenalty float64
		}
		var moves []rowMove
		for i := range p.numConstraints {
			s, e := p.rowPtr[i], p.rowPtr[i+1]
			act := 0.0
			for k := s; k < e; k++ {
				act += p.rowVal[k] * cand[p.colIdx[k]]
			}
			rhs := p.rowRHS[i]
			sense := p.rowSense[i]
			if (sense == LessEqual && act <= rhs+heurIntBoundTol) ||
				(sense == GreaterEqual && act >= rhs-heurIntBoundTol) ||
				(sense == Equal && math.Abs(act-rhs) <= heurIntBoundTol) {
				continue
			}
			delta := rhs - act // needed change in row activity
			moves = moves[:0]
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				if hlb[col] >= hub[col] {
					continue
				}
				a := p.rowVal[k]
				var room, dxSign float64
				if (delta > 0 && a > 0) || (delta < 0 && a < 0) {
					room = hub[col] - cand[col]
					dxSign = 1.0
				} else {
					room = cand[col] - hlb[col]
					dxSign = -1.0
				}
				if room > heurCoeffZeroTol {
					absA := math.Abs(a)
					pen := max(0.0, p.objMin[col]*dxSign) / absA
					moves = append(moves, rowMove{
						col:         col,
						absA:        absA,
						dxSign:      dxSign,
						maxRoom:     room,
						unitPenalty: pen,
					})
				}
			}
			slices.SortFunc(moves, func(a, b rowMove) int {
				return cmp.Compare(a.unitPenalty, b.unitPenalty)
			})
			rem := math.Abs(delta)
			for _, mv := range moves {
				if rem <= heurDedupTol {
					break
				}
				take := min(mv.maxRoom, rem/mv.absA)
				cand[mv.col] += mv.dxSign * take
				rem -= take * mv.absA
			}
			if rem > intFeasTol {
				return nil, 0, false
			}
		}
	}
	bV, rV, iV := p.MaxViolation(cand, true)
	if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
		obj := 0.0
		for j := range n {
			obj += p.objMin[j] * cand[j]
		}
		return cand, obj, true
	}
	return nil, 0, false
}

// solveFixedIntContinuousLP optimizes the continuous (and continuous-linked integer) variables
// after fixing primary integer decisions, trying closed-form epigraph and single-row solvers before LP fallback.
func solveFixedIntContinuousLP(
	p *CompiledProblem,
	hlb, hub []float64,
	auxSolver *LPSolver,
) (sol []float64, objMin float64, ok bool) {
	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, false
	}
	if eSol, eObj, eOk := solveEpigraphContinuousLP(p, hlb, hub); eOk {
		return eSol, eObj, true
	}
	n := p.numVars
	nCont := n - len(p.intCols)
	hasUnfixedInt := false
	for _, col := range p.intCols {
		if hlb[col] < hub[col] {
			hasUnfixedInt = true
			break
		}
	}
	if !hasUnfixedInt {
		if cSol, cObj, cOk := trySingleRowContinuousCompletion(p, hlb, hub); cOk {
			return cSol, cObj, true
		}
	}
	if !hasUnfixedInt && auxSolver != nil && n <= heurCompactLPMaxVars {
		var savedLB, savedUB []float64
		if auxSolver.scaled {
			savedLB = slices.Clone(auxSolver.origLB)
			savedUB = slices.Clone(auxSolver.origUB)
		} else {
			savedLB = slices.Clone(auxSolver.ws.lb[:n])
			savedUB = slices.Clone(auxSolver.ws.ub[:n])
		}
		auxSolver.SetStructuralBounds(hlb, hub)
		res := auxSolver.Solve()
		auxSolver.SetStructuralBounds(savedLB, savedUB)
		if res.Status == StatusOptimal {
			bV, rV, iV := p.MaxViolation(res.Primal, true)
			if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
				return slices.Clone(res.Primal), res.ObjectiveMin, true
			}
		}
		if res.Status == StatusInfeasible {
			return nil, 0, false
		}
	}

	if auxSolver == nil && n > heurNoAuxSkipVars {
		return nil, 0, false
	}

	subModel := NewModel("__cont_lp__", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nCont)
	var subIntIdxs []int
	var objTerms []Term
	fixedObj := 0.0

	for j := range n {
		if hlb[j] == hub[j] {
			fixedObj += p.objMin[j] * hlb[j]
			continue
		}
		isFree[j] = true
		subIdx := len(freeCols)
		v := subModel.AddVar(fmt.Sprintf("c_%d", j), hlb[j], hub[j], Continuous)
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.varCat[j] != Continuous {
			subIntIdxs = append(subIntIdxs, subIdx)
		}
		if p.objMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.objMin[j]))
		}
	}
	subModel.SetObjective(Expr{Terms: objTerms, Constant: fixedObj})

	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		fixedAct := 0.0
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], val))
			} else {
				fixedAct += val * hlb[col]
			}
		}
		sense := p.rowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if fixedAct > p.rowRHS[i]+intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if fixedAct < p.rowRHS[i]-intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(fixedAct-p.rowRHS[i]) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("r_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   p.rowRHS[i] - fixedAct,
		})
	}

	subP := subModel.Compile()
	ws := AcquireWorkspace(subP.numVars, subP.numConstraints)
	defer ReleaseWorkspace(ws)
	lp := NewLPSolver(subP, ws)
	if auxSolver != nil {
		lp.Deadline = auxSolver.Deadline
		lp.Done = auxSolver.Done
		lp.DisableSIMD = auxSolver.DisableSIMD
	}
	res := lp.Solve()
	if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
		return nil, 0, false
	}

	if len(subIntIdxs) > 0 {
		subLB := slices.Clone(subP.varLB)
		subUB := slices.Clone(subP.varUB)
		for _, idx := range subIntIdxs {
			v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
			rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
			if math.Abs(v-rv) <= intFeasTol {
				subLB[idx] = rv
				subUB[idx] = rv
				lp.SetStructuralBounds(subLB, subUB)
				res = lp.Solve()
				if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
					return nil, 0, false
				}
				continue
			}
			lo := max(subLB[idx], math.Floor(v))
			hi := min(subUB[idx], math.Ceil(v))
			subLB[idx] = lo
			subUB[idx] = lo
			lp.SetStructuralBounds(subLB, subUB)
			resLo := lp.Solve()
			okLo := resLo.Status == StatusOptimal && len(resLo.Primal) == subP.numVars

			if hi > lo {
				subLB[idx] = hi
				subUB[idx] = hi
				lp.SetStructuralBounds(subLB, subUB)
				resHi := lp.Solve()
				okHi := resHi.Status == StatusOptimal && len(resHi.Primal) == subP.numVars
				if okHi && (!okLo || resHi.ObjectiveMin < resLo.ObjectiveMin-heurDedupTol) {
					res = resHi
					continue
				}
			}
			if !okLo {
				return nil, 0, false
			}
			subLB[idx] = lo
			subUB[idx] = lo
			lp.SetStructuralBounds(subLB, subUB)
			res = lp.Solve()
			if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
				return nil, 0, false
			}
		}
	}

	cand := slices.Clone(hlb)
	for idx, col := range freeCols {
		cand[col] = res.Primal[idx]
	}
	for _, idx := range subIntIdxs {
		col := freeCols[idx]
		cand[col] = math.Round(cand[col])
	}
	bV, rV, iV := p.MaxViolation(cand, true)
	if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
		obj := 0.0
		for j := range n {
			obj += p.objMin[j] * cand[j]
		}
		return cand, obj, true
	}
	return nil, 0, false
}

// tryFractionalDiving performs up to maxDiveStepsCap rounds of fixing fractional equality-supported
// integer variables, propagating bounds, and warm-starting the LP relaxation.
func tryFractionalDiving(
	p *CompiledProblem,
	rootPrimal []float64,
	baseLB, baseUB []float64,
	diveSolver *LPSolver,
	bestObjMin float64,
	onlyEqSupport bool,
) (sol []float64, objMin float64, ok bool) {
	n := p.numVars
	isRootFrac := make([]bool, n)
	for _, col := range p.intCols {
		v := rootPrimal[col]
		f := v - math.Floor(v)
		if f > intFeasTol && f < 1.0-intFeasTol {
			isRootFrac[col] = true
		}
	}
	eqRowCount := make([]int, n)
	inRootSupport := make([]bool, n)
	hasEqSupport := false
	for i := range p.numConstraints {
		if p.rowSense[i] != Equal {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if e-s < 2 || e-s > maxEqSupportRowNNZ {
			continue
		}
		hasFrac := false
		allInt := true
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if p.varCat[col] == Continuous {
				allInt = false
				break
			}
			if isRootFrac[col] {
				hasFrac = true
			}
		}
		if allInt {
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				eqRowCount[col]++
				if hasFrac {
					inRootSupport[col] = true
					hasEqSupport = true
				}
			}
		}
	}
	if onlyEqSupport && !hasEqSupport {
		return nil, 0, false
	}

	dlb := slices.Clone(baseLB)
	dub := slices.Clone(baseUB)
	curX := slices.Clone(rootPrimal)
	rowQueue := make([]int, 0, p.numConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.numConstraints)
	isContLinked := findContLinkedIntCols(p)

	curObjMin := 0.0
	for j := range n {
		curObjMin += p.objMin[j] * curX[j]
	}
	savedDLB := make([]float64, n)
	savedDUB := make([]float64, n)
	branch1DLB := make([]float64, n)
	branch1DUB := make([]float64, n)

	maxDiveSteps := min(maxDiveStepsCap, len(p.intCols))
	for range maxDiveSteps {
		if diveSolver != nil && diveSolver.timeExpired() {
			return nil, 0, false
		}
		bestCol := -1
		bestScore := -1.0
		bestVal := 0.0
		for _, col := range p.intCols {
			if (isContLinked != nil && isContLinked[col]) || !inRootSupport[col] || dlb[col] == dub[col] {
				continue
			}
			v := curX[col]
			f := v - math.Floor(v)
			if f > intFeasTol && f < 1.0-intFeasTol {
				score := float64(eqRowCount[col]) + f
				if score > bestScore {
					bestScore = score
					bestCol = col
					bestVal = v
				}
			}
		}
		if bestCol != -1 && inRootSupport[bestCol] {
			bestEq := eqRowCount[bestCol]
			for _, col := range p.intCols {
				if (isContLinked != nil && isContLinked[col]) || dlb[col] == dub[col] || eqRowCount[col] <= bestEq {
					continue
				}
				v := curX[col]
				f := v - math.Floor(v)
				if f <= intFeasTol || f >= 1.0-intFeasTol {
					iv := min(max(math.Round(v), dlb[col]), dub[col])
					dlb[col] = iv
					dub[col] = iv
				}
			}
		}
		if bestCol == -1 {
			bestFrac := -1.0
			for _, col := range p.intCols {
				if (isContLinked != nil && isContLinked[col]) || dlb[col] == dub[col] {
					continue
				}
				v := curX[col]
				f := v - math.Floor(v)
				if f > intFeasTol && f < 1.0-intFeasTol {
					if f > bestFrac {
						bestFrac = f
						bestCol = col
						bestVal = v
					}
				} else {
					iv := math.Round(v)
					iv = min(max(iv, dlb[col]), dub[col])
					dlb[col] = iv
					dub[col] = iv
				}
			}
		}

		if bestCol == -1 {
			if isContLinked != nil {
				fixLB := slices.Clone(baseLB)
				fixUB := slices.Clone(baseUB)
				for _, col := range p.intCols {
					if isContLinked[col] {
						continue
					}
					iv := min(max(math.Round(curX[col]), baseLB[col]), baseUB[col])
					fixLB[col] = iv
					fixUB[col] = iv
				}
				if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, diveSolver); okLP && lpObj < bestObjMin {
					return lpSol, lpObj, true
				}
			}
			for _, col := range p.intCols {
				curX[col] = math.Round(curX[col])
			}
			bV, rV, iV := p.MaxViolation(curX, true)
			if bV <= incumbentTol && rV <= incumbentTol && iV <= incumbentTol {
				obj := 0.0
				for j := range n {
					obj += p.objMin[j] * curX[j]
				}
				if obj < bestObjMin {
					return curX, obj, true
				}
			}
			return nil, 0, false
		}

		copy(savedDLB, dlb)
		copy(savedDUB, dub)
		v := min(max(bestVal, dlb[bestCol]), dub[bestCol])
		target := min(max(math.Ceil(v), dlb[bestCol]), dub[bestCol])
		alt := min(max(math.Floor(v), dlb[bestCol]), dub[bestCol])

		dlb[bestCol] = target
		dub[bestCol] = target
		var res1 LPResult
		ok1 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok1 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res1 = diveSolver.Solve()
			ok1 = res1.Status == StatusOptimal && res1.ObjectiveMin < bestObjMin-heurIntBoundTol
		}

		if ok1 && res1.ObjectiveMin <= curObjMin+incumbentTol {
			curX = res1.Primal
			curObjMin = res1.ObjectiveMin
			continue
		}

		if alt == target {
			if !ok1 {
				return nil, 0, false
			}
			curX = res1.Primal
			curObjMin = res1.ObjectiveMin
			continue
		}

		copy(branch1DLB, dlb)
		copy(branch1DUB, dub)
		copy(dlb, savedDLB)
		copy(dub, savedDUB)
		dlb[bestCol] = alt
		dub[bestCol] = alt
		var res2 LPResult
		ok2 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok2 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res2 = diveSolver.Solve()
			ok2 = res2.Status == StatusOptimal && res2.ObjectiveMin < bestObjMin-heurIntBoundTol
		}

		if ok2 && (!ok1 || res2.ObjectiveMin <= res1.ObjectiveMin+intFeasTol) {
			curX = res2.Primal
			curObjMin = res2.ObjectiveMin
		} else if ok1 {
			copy(dlb, branch1DLB)
			copy(dub, branch1DUB)
			diveSolver.SetStructuralBounds(dlb, dub)
			res1 = diveSolver.Solve()
			if res1.Status != StatusOptimal {
				return nil, 0, false
			}
			curX = res1.Primal
			curObjMin = res1.ObjectiveMin
		} else {
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// tryRootSubMIP fixes integer variables that are already integral at the root LP
// relaxation and solves the compact reduced sub-MIP over the remaining fractional
// integer variables and continuous variables.
func tryRootSubMIP(
	p *CompiledProblem,
	lpSolver *LPSolver,
	lpPrimal []float64,
	curLB, curUB []float64,
	bestObjMin float64,
	deadline time.Time,
	isSubMIP bool,
) (sol []float64, objMin float64, ok bool) {
	if isSubMIP || len(p.intCols) == 0 {
		return nil, 0, false
	}
	var done <-chan struct{}
	if lpSolver != nil {
		done = lpSolver.Done
	}
	if stopRequested(deadline, done) {
		return nil, 0, false
	}

	n := p.numVars
	hlb := slices.Clone(curLB)
	hub := slices.Clone(curUB)

	isFrac := make([]bool, n)
	baseFrac := 0
	for _, col := range p.intCols {
		v := lpPrimal[col]
		rv := math.Round(v)
		if math.Abs(v-rv) > intFeasTol {
			isFrac[col] = true
			baseFrac++
		}
	}
	if baseFrac == 0 || baseFrac > heurSubMIPBaseFracCap {
		return nil, 0, false
	}

	var status []varBasisStatus
	if lpSolver != nil && lpSolver.ws != nil && len(lpSolver.ws.status) >= n {
		status = lpSolver.ws.status
	}

	hasEqRows := false
	for i := range p.numConstraints {
		if p.rowSense[i] != Equal {
			continue
		}
		hasEqRows = true
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if e-s > maxEqSupportRowNNZ {
			continue
		}
		rowHasFrac := false
		basicInRow := 0
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFrac[col] {
				rowHasFrac = true
			}
			if status != nil && status[col] == statusBasic {
				basicInRow++
			}
		}
		if rowHasFrac || (n <= heurSubMIPSmallModelVars && basicInRow >= 2) {
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				if p.varCat[col] != Continuous {
					isFrac[col] = true
				}
			}
		}
	}

	isContLinked := findContLinkedIntCols(p)
	if !hasEqRows && isContLinked == nil {
		return nil, 0, false
	}
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			isFrac[col] = true
		}
	}

	nFrac := 0
	for _, col := range p.intCols {
		if isFrac[col] {
			nFrac++
		} else {
			rv := math.Round(lpPrimal[col])
			iv := min(max(rv, hlb[col]), hub[col])
			hlb[col] = iv
			hub[col] = iv
		}
	}

	if nFrac == 0 || nFrac > heurSubMIPFreeFracCap || nFrac >= len(p.intCols) {
		return nil, 0, false
	}

	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, false
	}

	subModel := NewModel("submip", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nFrac+(n-len(p.intCols)))

	var objTerms []Term
	fixedObjMin := 0.0
	for j := range n {
		if hlb[j] == hub[j] {
			fixedObjMin += p.objMin[j] * hlb[j]
			continue
		}
		isFree[j] = true
		v := subModel.AddVar(fmt.Sprintf("v_%d", j), hlb[j], hub[j], p.varCat[j])
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.objMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.objMin[j]))
		}
	}

	if len(freeCols) == 0 {
		return nil, 0, false
	}

	subModel.SetObjective(Expr{Terms: objTerms, Constant: fixedObjMin})

	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		fixedAct := 0.0
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], val))
			} else {
				fixedAct += val * hlb[col]
			}
		}
		rhs := p.rowRHS[i] - fixedAct
		sense := p.rowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if fixedAct > p.rowRHS[i]+intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if fixedAct < p.rowRHS[i]-intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(fixedAct-p.rowRHS[i]) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("c_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   rhs,
		})
	}

	subP := subModel.Compile()
	if pres := presolveWithDeadline(subP, subMIPPresolvePasses, deadline, done); pres.TimedOut || pres.Infeasible {
		return nil, 0, false
	} else if pres.Reduced != nil {
		subP = pres.Reduced
	}

	subOpts := DefaultOptions()
	subOpts.isSubMIP = true
	subOpts.MaxNodes = heurSubMIPMaxNodes
	if !hasEqRows && isContLinked == nil {
		subOpts.MaxNodes = heurSubMIPFallbackNodes
	}
	subOpts.TimeLimit = subMIPTimeLimitMillis * time.Millisecond
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return nil, 0, false
		}
		subOpts.TimeLimit = min(subOpts.TimeLimit, rem)
	}
	subOpts.EnablePresolve = true
	subOpts.EnableCuts = true
	subOpts.MaxCutPasses = heurSubMIPCutPasses
	subOpts.EnableHeuristics = true
	if lpSolver != nil {
		subOpts.DisableSIMD = lpSolver.DisableSIMD
	}
	if !math.IsInf(bestObjMin, 1) {
		subOpts.hasCutoff = true
		subOpts.cutoffMin = bestObjMin
	}

	subRes := solveBranchAndCutWithDone(subP, subOpts, time.Now(), deadline, done)
	if len(subRes.Values) != subP.numVars {
		return nil, 0, false
	}

	cand := slices.Clone(hlb)
	for idx, origCol := range freeCols {
		cand[origCol] = subRes.Values[idx]
	}
	for _, col := range p.intCols {
		cand[col] = math.Round(cand[col])
	}

	bV, rV, iV := p.MaxViolation(cand, true)
	if bV > incumbentTol || rV > incumbentTol || iV > incumbentTol {
		fixLB := slices.Clone(curLB)
		fixUB := slices.Clone(curUB)
		for _, col := range p.intCols {
			fixLB[col] = cand[col]
			fixUB[col] = cand[col]
		}
		if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, lpSolver); okLP && lpObj < bestObjMin-heurIntBoundTol {
			return lpSol, lpObj, true
		}
		return nil, 0, false
	}

	obj := 0.0
	for j := range n {
		obj += p.objMin[j] * cand[j]
	}
	if obj >= bestObjMin-heurIntBoundTol {
		return nil, 0, false
	}
	return cand, obj, true
}
