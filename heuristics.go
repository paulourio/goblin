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
	heurSubMIPMaxNodes       = 250
	heurSubMIPCutPasses      = 3
	nodePropPasses           = 2
	subMIPPresolvePasses     = 4
	subMIPTimeLimitMillis    = 1500
	propSlackGuardRelMargin  = 0x1p-30
	propSlackGuardTolScale   = 0.25
	ownerUnassigned          = -2
	ownerPureEpigraph        = -1
)

// canSkipBoundTightening reports whether slackLo, a certified lower bound on the row
// slack, exceeds absA*(u-l) by enough margin that residualQuotient cannot tighten
// targetBound beyond heurIntBoundTol.
func canSkipBoundTightening(slackLo, absA, l, u, targetBound float64, isInt bool) bool {
	if slackLo < 0 || math.IsInf(targetBound, 0) {
		return false
	}
	if isInt && isFractionalFloat64(targetBound) {
		return false
	}
	span := u - l
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return false
	}
	prod := absA * span
	if math.IsNaN(prod) || math.IsInf(prod, 0) {
		return false
	}
	return slackLo > prod+propSlackGuardRelMargin*prod-propSlackGuardTolScale*absA*heurIntBoundTol
}

// propagateRow tightens variable bounds on row i from residual row activities and reports
// whether the row remains feasible and whether any bound shrunk by more than heurIntBoundTol.
func propagateRow(p *CompiledProblem, i int, lb, ub []float64, pushCol func(col int)) (feasible, changed bool) {
	s, e := p.rowPtr[i], p.rowPtr[i+1]
	cols := p.colIdx[s:e]
	vals := p.rowVal[s:e]
	rhs := p.rowRHS[i]
	sense := p.rowSense[i]
	hasUB := sense == LessEqual || sense == Equal
	hasLB := sense == GreaterEqual || sense == Equal

	var sum rowActivitySummary
	earlyBail := evalRowActivitySummaryInto(&sum, cols, vals, lb, ub, rhs, true, hasUB, hasLB)
	if earlyBail || sum.extreme {
		return true, false
	}

	rowScale := max(1.0, max(sum.l1Min, sum.l1Max))
	rowFeasTol := max(intFeasTol, presolveRelScaleTol*rowScale)

	if hasUB && sum.minInfCount == 0 && sum.slackHi < -rowFeasTol {
		return false, false
	}
	if hasLB && sum.maxInfCount == 0 && sum.deficitLo > rowFeasTol {
		return false, false
	}

	upperChanged := false
	if hasUB && sum.minInfCount == 0 {
		ubSlackLo := sum.slackLo
		canGuardUB := !sum.useWideSlack
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			isInt := p.varCat[col] != Continuous
			if a > heurCoeffZeroTol {
				if canGuardUB && canSkipBoundTightening(ubSlackLo, a, l, u, u, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				newUB := qVal + qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / a
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
						upperChanged = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			} else if a < -heurCoeffZeroTol {
				absA := -a
				if canGuardUB && canSkipBoundTightening(ubSlackLo, absA, l, u, l, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				newLB := qVal - qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / absA
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
						upperChanged = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			}
		}
	}

	if hasLB && upperChanged {
		evalRowActivitySummaryInto(&sum, cols, vals, lb, ub, rhs, false, false, true)
		if sum.extreme {
			return true, changed
		}
		rowScale = max(1.0, sum.l1Max)
		rowFeasTol = max(intFeasTol, presolveRelScaleTol*rowScale)
		if sum.maxInfCount == 0 && sum.deficitLo > rowFeasTol {
			return false, false
		}
	}

	if hasLB && sum.maxInfCount == 0 {
		lbSlackLo := -sum.deficitHi
		canGuardLB := !sum.useWideDeficit
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			isInt := p.varCat[col] != Continuous
			if a > heurCoeffZeroTol {
				if canGuardLB && canSkipBoundTightening(lbSlackLo, a, l, u, l, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				newLB := qVal - qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / a
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			} else if a < -heurCoeffZeroTol {
				absA := -a
				if canGuardLB && canSkipBoundTightening(lbSlackLo, absA, l, u, u, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				newUB := qVal + qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / absA
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			}
		}
	}
	return true, changed
}

func evalFixedRowResidual(cols []int, vals, x []float64, isFree []bool, rhs float64) (remRHS float64, ok bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return 0, false
	}
	var acc dot2Acc
	acc.addVal(rhs)
	for k, col := range cols {
		if isFree != nil && isFree[col] {
			continue
		}
		acc.addProd(-vals[k], x[col])
	}
	if val, eb, accOK := acc.result(); accOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		return val, true
	}
	var w wideDotAcc
	w.addVal(rhs)
	for k, col := range cols {
		if isFree != nil && isFree[col] {
			continue
		}
		w.addProd(-vals[k], x[col])
	}
	val := w.finish(math.NaN())
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, false
	}
	return val, true
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
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	n := p.numVars
	hasCont := len(p.intCols) < n

	type cand struct {
		col  int
		frac float64
	}
	baseHLB := slices.Clone(curLB)
	baseHUB := slices.Clone(curUB)
	fracCands := make([]cand, 0, min(len(p.intCols), 32))
	fixedAny := false
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		v := lpPrimal[col]
		rv := math.Round(v)
		f := math.Abs(v - rv)
		if f <= intFeasTol {
			iv := min(max(rv, baseHLB[col]), baseHUB[col])
			baseHLB[col] = iv
			baseHUB[col] = iv
			fixedAny = true
			continue
		}
		fracCands = append(fracCands, cand{col: col, frac: f})
	}
	if len(fracCands) > 1 {
		slices.SortFunc(fracCands, func(a, b cand) int {
			return cmp.Compare(a.frac, b.frac)
		})
	}
	if fixedAny {
		if !propagateBounds(p, baseHLB, baseHUB, 1) {
			return nil, 0, false
		}
	}
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

	snapHLB := make([]float64, n)
	snapHUB := make([]float64, n)
	var prevHLB, prevHUB []float64
	hasPrevFixed := false
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

			copy(snapHLB, hlb)
			copy(snapHUB, hub)
			savedLB := hlb[col]
			savedUB := hub[col]
			hlb[col] = target
			hub[col] = target

			if !propagateFromCol(p, hlb, hub, col, &rowQueue, inQueue) {
				copy(hlb, snapHLB)
				copy(hub, snapHUB)
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
		if hasPrevFixed && slices.Equal(hlb, prevHLB) && slices.Equal(hub, prevHUB) {
			continue
		}
		if strategy == 0 && maxStrat > 1 {
			prevHLB = slices.Clone(hlb)
			prevHUB = slices.Clone(hub)
			hasPrevFixed = true
		}

		if !hasCont {
			for j := range n {
				candX[j] = hlb[j]
			}
			bV, rV, iV := p.MaxViolation(candX, true)
			if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
				obj := minObjective(p, candX)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestObj {
					bestObj = obj
					bestSol = slices.Clone(candX)
				}
			}
		} else if auxSolver != nil {
			if sSol, sObj, sOk := solveFixedIntContinuousLP(p, hlb, hub, lpFallbackSolver, isContLinked); sOk && !math.IsNaN(sObj) && !math.IsInf(sObj, 0) && sObj < bestObj {
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
	epiIdx int
	c      float64
	alpha  float64
	beta   float64
}

// solveEpigraphContinuousLP solves the fixed-integer continuous (plus ContLinked integer)
// subproblem in closed form when every active constraint row is an epigraph lower bound
// z >= c + alpha * m + beta * n coupling an epigraph variable z (ObjMin[z] > 0, UB = +Inf)
// with at most one non-epigraph continuous variable m and one ContLinked integer variable n.
func solveEpigraphContinuousLP(p *CompiledProblem, hlb, hub []float64, isContLinked []bool) ([]float64, float64, bool) {
	n := p.numVars
	m := p.numConstraints

	// Check if all free variables are either Continuous or ContLinked integers.
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

	isFreeEpi := make([]bool, n)
	for j := range n {
		if hlb[j] < hub[j] {
			isFreeEpi[j] = true
		}
	}

	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		epiCol := -1
		contCol := -1
		intCol := -1
		freeCount := 0
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if !isFreeEpi[col] {
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
		remRHS, remOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFreeEpi, p.rowRHS[i])
		if !remOK {
			return nil, 0, false
		}
		sense := p.rowSense[i]
		if freeCount == 0 {
			switch sense {
			case LessEqual:
				if remRHS < -intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if remRHS > intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(remRHS) > intFeasTol {
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
		cTerm := remRHS / aEpi
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
		epiIdx := slices.Index(comps[cIdx].epiCols, epiCol)
		if epiIdx < 0 {
			epiIdx = len(comps[cIdx].epiCols)
			comps[cIdx].epiCols = append(comps[cIdx].epiCols, epiCol)
		}
		comps[cIdx].rows = append(comps[cIdx].rows, epigraphRowTerm{
			epiCol: epiCol,
			epiIdx: epiIdx,
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
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
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

// evalEpigraphCompObjWide isolates wideDotAcc stack allocation off solveEpigraphComponent.
//
//go:noinline
func evalEpigraphCompObjWide(objM, mv, objN, nv float64, nCol int, epiObj, epiMax []float64) float64 {
	var compObj wideDotAcc
	compObj.addProd(objM, mv)
	if nCol != -1 {
		compObj.addProd(objN, nv)
	}
	n := min(len(epiObj), len(epiMax))
	for ei := range n {
		compObj.addProd(epiObj[ei], epiMax[ei])
	}
	return compObj.finish(math.NaN())
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
		addNCand(0.0)
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

	if len(mCands) == 0 || len(nCands) == 0 {
		return
	}
	numEpi := len(comp.epiCols)
	objM := p.objMin[mCol]
	objN := 0.0
	if nCol != -1 {
		objN = p.objMin[nCol]
	}

	// Partition rows into beta==0 (m-only) and beta!=0 (m+n coupled) so m-only
	// epigraph maxima and objective terms are evaluated once per m candidate.
	var mOnlyBuf [32]epigraphRowTerm
	var mnBuf [8]epigraphRowTerm
	mOnlyRows := mOnlyBuf[:0]
	mnRows := mnBuf[:0]
	var epiHasBetaBuf [32]bool
	var epiHasBeta []bool
	if numEpi <= len(epiHasBetaBuf) {
		epiHasBeta = epiHasBetaBuf[:numEpi]
	} else {
		epiHasBeta = make([]bool, numEpi)
	}
	for _, r := range rows {
		if r.beta == 0 {
			mOnlyRows = append(mOnlyRows, r)
		} else {
			mnRows = append(mnRows, r)
			epiHasBeta[r.epiIdx] = true
		}
	}
	var mnEpiBuf [8]int
	mnEpiIdxs := mnEpiBuf[:0]
	for ei := range numEpi {
		if epiHasBeta[ei] {
			mnEpiIdxs = append(mnEpiIdxs, ei)
		}
	}

	var baseEpiBuf, epiMaxBuf, bestEpiBuf, epiObjBuf, epiLBBuf [32]float64
	var baseEpi, epiMax, bestEpiVals, epiObj, epiLB []float64
	if numEpi <= len(baseEpiBuf) {
		baseEpi = baseEpiBuf[:numEpi]
		epiMax = epiMaxBuf[:numEpi]
		bestEpiVals = bestEpiBuf[:numEpi]
		epiObj = epiObjBuf[:numEpi]
		epiLB = epiLBBuf[:numEpi]
	} else {
		baseEpi = make([]float64, numEpi)
		epiMax = make([]float64, numEpi)
		bestEpiVals = make([]float64, numEpi)
		epiObj = make([]float64, numEpi)
		epiLB = make([]float64, numEpi)
	}
	for ei, ec := range comp.epiCols {
		epiObj[ei] = p.objMin[ec]
		epiLB[ei] = hlb[ec]
	}

	if len(mnRows) == 0 && len(nCands) > 1 {
		bestNV := nCands[0]
		bestNC := objN * bestNV
		for _, nv := range nCands[1:] {
			if c := objN * nv; c < bestNC {
				bestNC = c
				bestNV = nv
			}
		}
		nCands = []float64{bestNV}
	}

	bestCompObj := math.Inf(1)
	bestM := mCands[0]
	bestN := nCands[0]
	screenFactor := float64(numEpi+4) * 1e-13

	for _, mv := range mCands {
		copy(baseEpi, epiLB)
		for _, r := range mOnlyRows {
			if val := r.c + r.alpha*mv; val > baseEpi[r.epiIdx] {
				baseEpi[r.epiIdx] = val
			}
		}
		baseSum := objM * mv
		baseAbs := math.Abs(baseSum)
		for ei := range numEpi {
			if !epiHasBeta[ei] {
				term := epiObj[ei] * baseEpi[ei]
				baseSum += term
				baseAbs += math.Abs(term)
			}
		}

		for _, nv := range nCands {
			for _, ei := range mnEpiIdxs {
				epiMax[ei] = baseEpi[ei]
			}
			for _, r := range mnRows {
				if val := r.c + r.alpha*mv + r.beta*nv; val > epiMax[r.epiIdx] {
					epiMax[r.epiIdx] = val
				}
			}
			nTerm := objN * nv
			candObj := baseSum + nTerm
			candAbs := baseAbs + math.Abs(nTerm)
			for _, ei := range mnEpiIdxs {
				term := epiObj[ei] * epiMax[ei]
				candObj += term
				candAbs += math.Abs(term)
			}
			errEst := screenFactor * max(1.0, candAbs)
			if !isNonFinite(candObj) && !isNonFinite(candAbs) && candObj-errEst >= bestCompObj {
				continue
			}

			copy(epiMax, baseEpi)
			for _, r := range mnRows {
				if val := r.c + r.alpha*mv + r.beta*nv; val > epiMax[r.epiIdx] {
					epiMax[r.epiIdx] = val
				}
			}
			var acc dot2Acc
			acc.addProd(objM, mv)
			if nCol != -1 {
				acc.addProd(objN, nv)
			}
			for ei := range numEpi {
				acc.addProd(epiObj[ei], epiMax[ei])
			}
			obj, eb, ok := acc.result()
			if !ok || eb > 0x1p-48*math.Abs(obj) {
				obj = evalEpigraphCompObjWide(objM, mv, objN, nv, nCol, epiObj, epiMax[:numEpi])
			}
			if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestCompObj {
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
			rhs := p.rowRHS[i]
			delta, deltaOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], cand, nil, rhs)
			if !deltaOK {
				return nil, 0, false
			}
			sense := p.rowSense[i]
			if (sense == LessEqual && delta >= -heurIntBoundTol) ||
				(sense == GreaterEqual && delta <= heurIntBoundTol) ||
				(sense == Equal && math.Abs(delta) <= heurIntBoundTol) {
				continue
			}
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
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
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
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, false
	}
	if eSol, eObj, eOk := solveEpigraphContinuousLP(p, hlb, hub, isContLinked); eOk {
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
				obj := minObjective(p, res.Primal)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) {
					return slices.Clone(res.Primal), obj, true
				}
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

	for j := range n {
		if hlb[j] == hub[j] {
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
	subModel.SetObjective(Expr{Terms: objTerms})

	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], p.rowVal[k]))
			}
		}
		remRHS, remOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFree, p.rowRHS[i])
		if !remOK {
			return nil, 0, false
		}
		sense := p.rowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if remRHS < -intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if remRHS > intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(remRHS) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("r_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   remRHS,
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

	var subLB, subUB []float64
	skippedIntegralFix := false
	if len(subIntIdxs) > 0 {
		subLB = slices.Clone(subP.varLB)
		subUB = slices.Clone(subP.varUB)
		var fracSubIdxs []int
		for _, idx := range subIntIdxs {
			v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
			rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
			if math.Abs(v-rv) <= intFeasTol {
				subLB[idx] = rv
				subUB[idx] = rv
				skippedIntegralFix = true
			} else {
				fracSubIdxs = append(fracSubIdxs, idx)
			}
		}
		if len(fracSubIdxs) > 0 {
			// Apply all batched integral fixings once before rounding fractional variables.
			lp.SetStructuralBounds(subLB, subUB)
			skippedIntegralFix = false
			resLoPrimal := make([]float64, subP.numVars)
			for _, idx := range fracSubIdxs {
				v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
				rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
				if math.Abs(v-rv) <= intFeasTol {
					subLB[idx] = rv
					subUB[idx] = rv
					skippedIntegralFix = true
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
					if okLo {
						copy(resLoPrimal, resLo.Primal)
					}
					subLB[idx] = hi
					subUB[idx] = hi
					lp.SetStructuralBounds(subLB, subUB)
					resHi := lp.Solve()
					okHi := resHi.Status == StatusOptimal && len(resHi.Primal) == subP.numVars
					if okHi && (!okLo || lpMinWithConst(subP, resHi) < lpMinWithConst(subP, resLo)-heurDedupTol) {
						res = resHi
						skippedIntegralFix = false
						continue
					}
				}
				if !okLo {
					return nil, 0, false
				}
				subLB[idx] = lo
				subUB[idx] = lo
				if hi > lo {
					resLo.Primal = resLoPrimal
				}
				res = resLo
				skippedIntegralFix = false
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
	if (bV > intFeasTol || rV > intFeasTol || iV > intFeasTol) && skippedIntegralFix {
		// Re-solve once with all batched integer fixings applied when rounding small
		// integral residuals violated a high-coefficient constraint.
		lp.SetStructuralBounds(subLB, subUB)
		res = lp.Solve()
		if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
			return nil, 0, false
		}
		copy(cand, hlb)
		for idx, col := range freeCols {
			cand[col] = res.Primal[idx]
		}
		for _, idx := range subIntIdxs {
			col := freeCols[idx]
			cand[col] = math.Round(cand[col])
		}
		bV, rV, iV = p.MaxViolation(cand, true)
	}
	if bV <= intFeasTol && rV <= intFeasTol && iV <= intFeasTol {
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
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
	isContLinked []bool,
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

	curObjMin := minObjective(p, curX)
	savedDLB := make([]float64, n)
	savedDUB := make([]float64, n)
	branch1DLB := make([]float64, n)
	branch1DUB := make([]float64, n)
	branch1X := make([]float64, n)

	if diveSolver != nil {
		prevFast := diveSolver.fastHeuristicPrimal
		diveSolver.fastHeuristicPrimal = true
		defer func() {
			diveSolver.fastHeuristicPrimal = prevFast
		}()
	}

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
				if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, diveSolver, isContLinked); okLP && lpObj < bestObjMin {
					return lpSol, lpObj, true
				}
			}
			for _, col := range p.intCols {
				curX[col] = math.Round(curX[col])
			}
			bV, rV, iV := p.MaxViolation(curX, true)
			if bV <= incumbentTol && rV <= incumbentTol && iV <= incumbentTol {
				obj := minObjective(p, curX)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestObjMin {
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
		var m1 float64
		ok1 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok1 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res1 = diveSolver.Solve()
			m1 = lpMinWithConst(p, res1)
			ok1 = res1.Status == StatusOptimal && !math.IsNaN(m1) && !math.IsInf(m1, 0) && m1 < bestObjMin-heurIntBoundTol
		}

		if ok1 && m1 <= curObjMin+incumbentTol {
			copy(curX, res1.Primal)
			curObjMin = m1
			continue
		}

		if alt == target {
			if !ok1 {
				return nil, 0, false
			}
			copy(curX, res1.Primal)
			curObjMin = m1
			continue
		}

		copy(branch1DLB, dlb)
		copy(branch1DUB, dub)
		if ok1 {
			copy(branch1X, res1.Primal)
		}
		copy(dlb, savedDLB)
		copy(dub, savedDUB)
		dlb[bestCol] = alt
		dub[bestCol] = alt
		var res2 LPResult
		var m2 float64
		ok2 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok2 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res2 = diveSolver.Solve()
			m2 = lpMinWithConst(p, res2)
			ok2 = res2.Status == StatusOptimal && !math.IsNaN(m2) && !math.IsInf(m2, 0) && m2 < bestObjMin-heurIntBoundTol
		}

		if ok2 && (!ok1 || m2 <= m1+intFeasTol) {
			copy(curX, res2.Primal)
			curObjMin = m2
		} else if ok1 {
			copy(dlb, branch1DLB)
			copy(dub, branch1DUB)
			copy(curX, branch1X)
			curObjMin = m1
		} else {
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// tryRootSubMIP fixes integer variables that are already integral at the root LP
// relaxation and solves the compact reduced sub-MIP over the remaining fractional
// integer variables and continuous variables, returning any explored sub-MIP nodes
// and simplex pivots so the parent search accounts for shared budgets.
func tryRootSubMIP(
	p *CompiledProblem,
	lpSolver *LPSolver,
	rootStatus []varBasisStatus,
	lpPrimal []float64,
	curLB, curUB []float64,
	bestObjMin float64,
	deadline time.Time,
	isSubMIP bool,
	maxNodesCap int,
	maxIterCap int,
	isContLinked []bool,
	enableCuts bool,
) (sol []float64, objMin float64, nodesUsed, itersUsed int, ok bool) {
	if isSubMIP || len(p.intCols) == 0 || maxNodesCap <= 0 || maxIterCap <= 0 {
		return nil, 0, 0, 0, false
	}
	var done <-chan struct{}
	if lpSolver != nil {
		done = lpSolver.Done
	}
	if stopRequested(deadline, done) {
		return nil, 0, 0, 0, false
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
		return nil, 0, 0, 0, false
	}

	var status []varBasisStatus
	if len(rootStatus) >= n {
		status = rootStatus
	} else if lpSolver != nil && lpSolver.ws != nil && len(lpSolver.ws.status) >= n {
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

	if !hasEqRows && isContLinked == nil {
		return nil, 0, 0, 0, false
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

	minFixed := max(2, len(p.intCols)/4)
	if !enableCuts {
		minFixed = max(minFixed, len(p.intCols)/2)
	}
	if nFrac == 0 || nFrac > heurSubMIPFreeFracCap || len(p.intCols)-nFrac < minFixed {
		return nil, 0, 0, 0, false
	}

	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, 0, 0, false
	}

	subModel := NewModel("submip", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nFrac+(n-len(p.intCols)))

	var objTerms []Term
	var fixedObjAcc dot2Acc
	fixedObjAcc.addVal(p.objConstMin())
	for j := range n {
		if hlb[j] == hub[j] {
			fixedObjAcc.addProd(p.objMin[j], hlb[j])
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
		return nil, 0, 0, 0, false
	}

	var fixedObjMin float64
	if val, eb, fOK := fixedObjAcc.result(); fOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		fixedObjMin = val
	} else {
		var w wideDotAcc
		w.addVal(p.objConstMin())
		for j := range n {
			if !isFree[j] {
				w.addProd(p.objMin[j], hlb[j])
			}
		}
		fixedObjMin = w.finish(math.NaN())
	}
	if math.IsNaN(fixedObjMin) || math.IsInf(fixedObjMin, 0) {
		return nil, 0, 0, 0, false
	}

	subModel.SetObjective(Expr{Terms: objTerms, Constant: fixedObjMin})

	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], p.rowVal[k]))
			}
		}
		rhs, rhsOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFree, p.rowRHS[i])
		if !rhsOK {
			return nil, 0, 0, 0, false
		}
		sense := p.rowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if rhs < -intFeasTol {
					return nil, 0, 0, 0, false
				}
			case GreaterEqual:
				if rhs > intFeasTol {
					return nil, 0, 0, 0, false
				}
			case Equal:
				if math.Abs(rhs) > intFeasTol {
					return nil, 0, 0, 0, false
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
		return nil, 0, 0, 0, false
	} else if pres.Reduced != nil && pres.Reduced.invalidErr == nil {
		subP = pres.Reduced
	}

	subOpts := DefaultOptions()
	subOpts.isSubMIP = true
	subOpts.MaxNodes = min(heurSubMIPMaxNodes, maxNodesCap)
	if maxIterCap < math.MaxInt {
		subOpts.MaxIter = maxIterCap
	}
	subOpts.TimeLimit = subMIPTimeLimitMillis * time.Millisecond
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return nil, 0, 0, 0, false
		}
		subOpts.TimeLimit = min(subOpts.TimeLimit, rem)
	}
	subOpts.EnablePresolve = true
	subOpts.EnableCuts = enableCuts
	if enableCuts {
		subOpts.MaxCutPasses = heurSubMIPCutPasses
	} else {
		subOpts.MaxCutPasses = 0
	}
	subOpts.EnableHeuristics = true
	subOpts.Workers = 1
	if lpSolver != nil {
		subOpts.DisableSIMD = lpSolver.DisableSIMD
	}
	if !math.IsInf(bestObjMin, 1) {
		subOpts.hasCutoff = true
		subOpts.cutoffMin = bestObjMin
	}

	subRes := solveBranchAndCutWithDone(subP, subOpts, time.Now(), deadline, done)
	usedN, usedIt := subRes.Nodes, subRes.Iterations
	if len(subRes.Values) != subP.numVars {
		return nil, 0, usedN, usedIt, false
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
		if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, lpSolver, isContLinked); okLP && lpObj < bestObjMin-heurIntBoundTol {
			return lpSol, lpObj, usedN, usedIt, true
		}
		return nil, 0, usedN, usedIt, false
	}

	obj := minObjective(p, cand)
	if math.IsNaN(obj) || math.IsInf(obj, 0) || obj >= bestObjMin-heurIntBoundTol {
		return nil, 0, usedN, usedIt, false
	}
	return cand, obj, usedN, usedIt, true
}
