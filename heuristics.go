package goblin

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	intFeasTol            = 1e-6
	heurCoeffZeroTol      = 1e-12
	heurIntBoundTol       = 1e-7
	minPropSteps          = 32
	propStepsPerRow       = 3
	rowQueueCapMult       = 2
	maxDiveStepsCap       = 18
	maxEqSupportRowNNZ    = 32
	nodePropPasses        = 2
	subMIPPresolvePasses  = 4
	subMIPTimeLimitMillis = 1500
)

func propagateRow(p *CompiledProblem, i int, lb, ub []float64, pushCol func(col int)) (feasible, changed bool) {
	s, e := p.RowPtr[i], p.RowPtr[i+1]
	rhs := p.RowRHS[i]
	sense := p.RowSense[i]
	hasUB := sense == LessEqual || sense == Equal
	hasLB := sense == GreaterEqual || sense == Equal

	minAct := 0.0
	maxAct := 0.0
	minInf := 0
	maxInf := 0

	for k := s; k < e; k++ {
		col := p.ColIdx[k]
		a := p.RowVal[k]
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

	if hasUB && minInf == 0 && minAct > rhs+1e-6 {
		return false, false
	}
	if hasLB && maxInf == 0 && maxAct < rhs-1e-6 {
		return false, false
	}

	if hasUB && minInf == 0 {
		slack := rhs - minAct
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			a := p.RowVal[k]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				newUB := l + slack/a
				if p.VarCat[col] != Continuous {
					newUB = math.Floor(newUB + heurIntBoundTol)
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
				if p.VarCat[col] != Continuous {
					newLB = math.Ceil(newLB - heurIntBoundTol)
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
			col := p.ColIdx[k]
			a := p.RowVal[k]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			if a > heurCoeffZeroTol {
				newLB := u - surplus/a
				if p.VarCat[col] != Continuous {
					newLB = math.Ceil(newLB - heurIntBoundTol)
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
				if p.VarCat[col] != Continuous {
					newUB = math.Floor(newUB + heurIntBoundTol)
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
	m := p.NumConstraints
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
		for k := p.ColPtr[col]; k < p.ColPtr[col+1]; k++ {
			r := p.RowIdx[k]
			if !inQueue[r] {
				inQueue[r] = true
				q = append(q, r)
			}
		}
	}
	pushCol(startCol)

	head := 0
	maxSteps := max(minPropSteps, propStepsPerRow*p.NumConstraints)
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

func findContLinkedIntCols(p *CompiledProblem) []bool {
	if len(p.IntCols) == 0 || len(p.IntCols) == p.NumVars {
		return nil
	}
	rowHasCont := make([]bool, p.NumConstraints)
	for i := range p.NumConstraints {
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		for k := s; k < e; k++ {
			if p.VarCat[p.ColIdx[k]] == Continuous {
				rowHasCont[i] = true
				break
			}
		}
	}
	var isContLinked []bool
	for _, col := range p.IntCols {
		if p.VarCat[col] != Integer {
			continue
		}
		s, e := p.ColPtr[col], p.ColPtr[col+1]
		if s == e {
			continue
		}
		allContRows := true
		for k := s; k < e; k++ {
			if !rowHasCont[p.RowIdx[k]] {
				allContRows = false
				break
			}
		}
		if allContRows {
			if isContLinked == nil {
				isContLinked = make([]bool, p.NumVars)
			}
			isContLinked[col] = true
		}
	}
	return isContLinked
}

func tryPropagatedRounding(
	p *CompiledProblem,
	lpPrimal []float64,
	curLB, curUB []float64,
	auxSolver *LPSolver,
) (sol []float64, objMin float64, ok bool) {
	n := p.NumVars
	hasCont := len(p.IntCols) < n
	isContLinked := findContLinkedIntCols(p)

	type cand struct {
		col  int
		frac float64
	}
	cands := make([]cand, 0, len(p.IntCols))
	for _, col := range p.IntCols {
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
	if hasCont && len(fracCands) > 32 {
		return nil, 0, false
	}

	hlb := make([]float64, n)
	hub := make([]float64, n)
	candX := make([]float64, n)
	rowQueue := make([]int, 0, p.NumConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.NumConstraints)

	var bestSol []float64
	bestObj := math.Inf(1)

	maxStrat := 2
	if len(fracCands) == 0 {
		maxStrat = 1
	}

	for strategy := range maxStrat {
		copy(hlb, baseHLB)
		copy(hub, baseHUB)
		feasible := true

		for _, c := range fracCands {
			col := c.col
			if hlb[col] == hub[col] {
				continue
			}
			v := min(max(lpPrimal[col], hlb[col]), hub[col])
			var target float64
			if strategy == 0 || math.Abs(v-math.Round(v)) <= 0.15 {
				target = math.Round(v)
			} else {
				if p.ObjMin[col] > heurCoeffZeroTol {
					target = math.Floor(v)
				} else if p.ObjMin[col] < -heurCoeffZeroTol {
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
			if bV <= 1e-6 && rV <= 1e-6 && iV <= 1e-6 {
				obj := 0.0
				for j := range n {
					obj += p.ObjMin[j] * candX[j]
				}
				if obj < bestObj {
					bestObj = obj
					bestSol = slices.Clone(candX)
				}
			}
		} else if auxSolver != nil {
			if sSol, sObj, sOk := solveFixedIntContinuousLP(p, hlb, hub, auxSolver); sOk && sObj < bestObj {
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

func solveFixedIntContinuousLP(
	p *CompiledProblem,
	hlb, hub []float64,
	auxSolver *LPSolver,
) (sol []float64, objMin float64, ok bool) {
	n := p.NumVars
	nCont := n - len(p.IntCols)
	hasUnfixedInt := false
	for _, col := range p.IntCols {
		if hlb[col] < hub[col] {
			hasUnfixedInt = true
			break
		}
	}
	if !hasUnfixedInt && auxSolver != nil {
		auxSolver.SetStructuralBounds(hlb, hub)
		res := auxSolver.Solve()
		if res.Status == StatusOptimal {
			bV, rV, iV := p.MaxViolation(res.Primal, true)
			if bV <= 1e-6 && rV <= 1e-6 && iV <= 1e-6 {
				return slices.Clone(res.Primal), res.ObjectiveMin, true
			}
		}
		if res.Status == StatusInfeasible {
			return nil, 0, false
		}
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
			fixedObj += p.ObjMin[j] * hlb[j]
			continue
		}
		isFree[j] = true
		subIdx := len(freeCols)
		v := subModel.AddVar(fmt.Sprintf("c_%d", j), hlb[j], hub[j], Continuous)
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.VarCat[j] != Continuous {
			subIntIdxs = append(subIntIdxs, subIdx)
		}
		if p.ObjMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.ObjMin[j]))
		}
	}
	subModel.SetObjective(Expr{Terms: objTerms, Constant: fixedObj})

	for i := range p.NumConstraints {
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		fixedAct := 0.0
		var terms []Term
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			val := p.RowVal[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], val))
			} else {
				fixedAct += val * hlb[col]
			}
		}
		sense := p.RowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if fixedAct > p.RowRHS[i]+1e-6 {
					return nil, 0, false
				}
			case GreaterEqual:
				if fixedAct < p.RowRHS[i]-1e-6 {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(fixedAct-p.RowRHS[i]) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("r_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   p.RowRHS[i] - fixedAct,
		})
	}

	subP := subModel.Compile()
	ws := AcquireWorkspace(subP.NumVars, subP.NumConstraints)
	defer ReleaseWorkspace(ws)
	lp := NewLPSolver(subP, ws)
	if auxSolver != nil {
		lp.Deadline = auxSolver.Deadline
		lp.DisableSIMD = auxSolver.DisableSIMD
	}
	res := lp.Solve()
	if res.Status != StatusOptimal || len(res.Primal) != subP.NumVars {
		return nil, 0, false
	}

	if len(subIntIdxs) > 0 {
		subLB := slices.Clone(subP.VarLB)
		subUB := slices.Clone(subP.VarUB)
		for _, idx := range subIntIdxs {
			v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
			rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
			if math.Abs(v-rv) <= intFeasTol {
				subLB[idx] = rv
				subUB[idx] = rv
				lp.SetStructuralBounds(subLB, subUB)
				res = lp.Solve()
				if res.Status != StatusOptimal || len(res.Primal) != subP.NumVars {
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
			okLo := resLo.Status == StatusOptimal && len(resLo.Primal) == subP.NumVars

			if hi > lo {
				subLB[idx] = hi
				subUB[idx] = hi
				lp.SetStructuralBounds(subLB, subUB)
				resHi := lp.Solve()
				okHi := resHi.Status == StatusOptimal && len(resHi.Primal) == subP.NumVars
				if okHi && (!okLo || resHi.ObjectiveMin < resLo.ObjectiveMin-1e-9) {
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
			if res.Status != StatusOptimal || len(res.Primal) != subP.NumVars {
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
	if bV <= 1e-6 && rV <= 1e-6 && iV <= 1e-6 {
		obj := 0.0
		for j := range n {
			obj += p.ObjMin[j] * cand[j]
		}
		return cand, obj, true
	}
	return nil, 0, false
}

func tryFractionalDiving(
	p *CompiledProblem,
	rootPrimal []float64,
	baseLB, baseUB []float64,
	diveSolver *LPSolver,
	bestObjMin float64,
	onlyEqSupport bool,
) (sol []float64, objMin float64, ok bool) {
	n := p.NumVars
	isRootFrac := make([]bool, n)
	for _, col := range p.IntCols {
		v := rootPrimal[col]
		f := v - math.Floor(v)
		if f > intFeasTol && f < 1.0-intFeasTol {
			isRootFrac[col] = true
		}
	}
	eqRowCount := make([]int, n)
	inRootSupport := make([]bool, n)
	hasEqSupport := false
	for i := range p.NumConstraints {
		if p.RowSense[i] != Equal {
			continue
		}
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		if e-s < 2 || e-s > 32 {
			continue
		}
		hasFrac := false
		allInt := true
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			if p.VarCat[col] == Continuous {
				allInt = false
				break
			}
			if isRootFrac[col] {
				hasFrac = true
			}
		}
		if allInt {
			for k := s; k < e; k++ {
				col := p.ColIdx[k]
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
	rowQueue := make([]int, 0, p.NumConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.NumConstraints)
	isContLinked := findContLinkedIntCols(p)

	curObjMin := 0.0
	for j := range n {
		curObjMin += p.ObjMin[j] * curX[j]
	}
	savedDLB := make([]float64, n)
	savedDUB := make([]float64, n)
	branch1DLB := make([]float64, n)
	branch1DUB := make([]float64, n)

	maxDiveSteps := min(maxDiveStepsCap, len(p.IntCols))
	for range maxDiveSteps {
		if diveSolver != nil && diveSolver.timeExpired() {
			return nil, 0, false
		}
		bestCol := -1
		bestScore := -1.0
		bestVal := 0.0
		for _, col := range p.IntCols {
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
			for _, col := range p.IntCols {
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
			for _, col := range p.IntCols {
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
				for _, col := range p.IntCols {
					if isContLinked[col] {
						continue
					}
					iv := min(max(math.Round(curX[col]), baseLB[col]), baseUB[col])
					fixLB[col] = iv
					fixUB[col] = iv
				}
				if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, nil); okLP && lpObj < bestObjMin {
					return lpSol, lpObj, true
				}
			}
			for _, col := range p.IntCols {
				curX[col] = math.Round(curX[col])
			}
			bV, rV, iV := p.MaxViolation(curX, true)
			if bV <= 1e-5 && rV <= 1e-5 && iV <= 1e-5 {
				obj := 0.0
				for j := range n {
					obj += p.ObjMin[j] * curX[j]
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
			ok1 = res1.Status == StatusOptimal && res1.ObjectiveMin < bestObjMin-1e-7
		}

		if ok1 && res1.ObjectiveMin <= curObjMin+1e-5 {
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
			ok2 = res2.Status == StatusOptimal && res2.ObjectiveMin < bestObjMin-1e-7
		}

		if ok2 && (!ok1 || res2.ObjectiveMin <= res1.ObjectiveMin+1e-6) {
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
) (sol []float64, objMin float64, ok bool) {
	if p.Name == "__submip__" || len(p.IntCols) == 0 {
		return nil, 0, false
	}
	if !deadline.IsZero() && time.Now().After(deadline) {
		return nil, 0, false
	}

	n := p.NumVars
	hlb := slices.Clone(curLB)
	hub := slices.Clone(curUB)

	isFrac := make([]bool, n)
	baseFrac := 0
	for _, col := range p.IntCols {
		v := lpPrimal[col]
		rv := math.Round(v)
		if math.Abs(v-rv) > intFeasTol {
			isFrac[col] = true
			baseFrac++
		}
	}
	if baseFrac == 0 || baseFrac > 120 {
		return nil, 0, false
	}

	var status []varBasisStatus
	if lpSolver != nil && lpSolver.WS != nil && len(lpSolver.WS.Status) >= n {
		status = lpSolver.WS.Status
	}

	for i := range p.NumConstraints {
		if p.RowSense[i] != Equal {
			continue
		}
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		if e-s > maxEqSupportRowNNZ {
			continue
		}
		rowHasFrac := false
		basicInRow := 0
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			if isFrac[col] {
				rowHasFrac = true
			}
			if status != nil && status[col] == statusBasic {
				basicInRow++
			}
		}
		if rowHasFrac || basicInRow >= 2 {
			for k := s; k < e; k++ {
				col := p.ColIdx[k]
				if p.VarCat[col] != Continuous {
					isFrac[col] = true
				}
			}
		}
	}

	isContLinked := findContLinkedIntCols(p)
	for _, col := range p.IntCols {
		if isContLinked != nil && isContLinked[col] {
			isFrac[col] = true
		}
	}

	nFrac := 0
	for _, col := range p.IntCols {
		if isFrac[col] {
			nFrac++
		} else {
			rv := math.Round(lpPrimal[col])
			iv := min(max(rv, hlb[col]), hub[col])
			hlb[col] = iv
			hub[col] = iv
		}
	}

	if nFrac == 0 || nFrac > 240 || nFrac >= len(p.IntCols) {
		return nil, 0, false
	}

	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, false
	}

	subModel := NewModel("__submip__", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nFrac+(n-len(p.IntCols)))

	var objTerms []Term
	fixedObjMin := 0.0
	for j := range n {
		if hlb[j] == hub[j] {
			fixedObjMin += p.ObjMin[j] * hlb[j]
			continue
		}
		isFree[j] = true
		v := subModel.AddVar(fmt.Sprintf("v_%d", j), hlb[j], hub[j], p.VarCat[j])
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.ObjMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.ObjMin[j]))
		}
	}

	if len(freeCols) == 0 {
		return nil, 0, false
	}

	subModel.SetObjective(Expr{Terms: objTerms, Constant: fixedObjMin})

	for i := range p.NumConstraints {
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		fixedAct := 0.0
		var terms []Term
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			val := p.RowVal[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], val))
			} else {
				fixedAct += val * hlb[col]
			}
		}
		rhs := p.RowRHS[i] - fixedAct
		sense := p.RowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if fixedAct > p.RowRHS[i]+1e-6 {
					return nil, 0, false
				}
			case GreaterEqual:
				if fixedAct < p.RowRHS[i]-1e-6 {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(fixedAct-p.RowRHS[i]) > intFeasTol {
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
	if pres := presolveWithDeadline(subP, subMIPPresolvePasses, deadline); pres.TimedOut || pres.Infeasible {
		return nil, 0, false
	} else if pres.Reduced != nil {
		subP = pres.Reduced
	}

	subOpts := DefaultOptions()
	subOpts.MaxNodes = 800
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
	subOpts.MaxCutPasses = 3
	subOpts.EnableHeuristics = true
	if lpSolver != nil {
		subOpts.DisableSIMD = lpSolver.DisableSIMD
	}

	subRes := solveBranchAndCut(subP, subOpts, time.Now())
	if len(subRes.Values) != subP.NumVars {
		return nil, 0, false
	}

	cand := slices.Clone(hlb)
	for idx, origCol := range freeCols {
		cand[origCol] = subRes.Values[idx]
	}
	for _, col := range p.IntCols {
		cand[col] = math.Round(cand[col])
	}

	bV, rV, iV := p.MaxViolation(cand, true)
	if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
		fixLB := slices.Clone(curLB)
		fixUB := slices.Clone(curUB)
		for _, col := range p.IntCols {
			fixLB[col] = cand[col]
			fixUB[col] = cand[col]
		}
		if lpSol, lpObj, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, nil); okLP && lpObj < bestObjMin-1e-7 {
			return lpSol, lpObj, true
		}
		return nil, 0, false
	}

	obj := 0.0
	for j := range n {
		obj += p.ObjMin[j] * cand[j]
	}
	if obj >= bestObjMin-1e-7 {
		return nil, 0, false
	}
	return cand, obj, true
}
