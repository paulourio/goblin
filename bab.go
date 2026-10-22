package gopt

import (
	"container/heap"
	"math"
	"slices"
	"time"
)

type boundDelta struct {
	col int
	lb  float64
	ub  float64
}

type searchNode struct {
	lowerBound float64
	estimate   float64
	depth      int
	id         int
	deltas     []boundDelta
}

type nodeQueue []*searchNode

func (pq nodeQueue) Len() int { return len(pq) }
func (pq nodeQueue) Less(i, j int) bool {
	if math.Abs(pq[i].lowerBound-pq[j].lowerBound) > 1e-7 {
		return pq[i].lowerBound < pq[j].lowerBound
	}
	if math.Abs(pq[i].estimate-pq[j].estimate) > 1e-7 {
		return pq[i].estimate < pq[j].estimate
	}
	return pq[i].depth > pq[j].depth
}
func (pq nodeQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }
func (pq *nodeQueue) Push(x any)   { *pq = append(*pq, x.(*searchNode)) }
func (pq *nodeQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*pq = old[:n-1]
	return item
}

type pseudoCost struct {
	downSum   float64
	upSum     float64
	downCount int
	upCount   int
}

func solveBranchAndCut(p *CompiledProblem, opts Options, startTime time.Time) Result {
	n := p.NumVars
	var deadline time.Time
	if opts.TimeLimit > 0 {
		deadline = startTime.Add(opts.TimeLimit)
	}
	timeExpired := func() bool {
		return !deadline.IsZero() && time.Now().After(deadline)
	}

	ws := AcquireWorkspace(n, p.NumConstraints)
	defer ReleaseWorkspace(ws)

	lpSolver := NewLPSolver(p, ws)
	lpSolver.Deadline = deadline
	rootRes := lpSolver.Solve()
	totalLPIters := rootRes.Iterations

	if rootRes.Status != StatusOptimal {
		return Result{
			modelID:    p.modelID,
			Status:     rootRes.Status,
			Iterations: totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}

	rootLPMin := rootRes.ObjectiveMin
	rootLPUser := p.ObjectiveFromMin(rootLPMin)

	if !p.IsMIP || len(p.IntCols) == 0 {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    rootRes.Objective,
			BestBound:    rootRes.Objective,
			RootLPBound:  rootLPUser,
			RootCutBound: rootLPUser,
			Values:       rootRes.Primal,
			Duals:        rootRes.Dual,
			ReducedCosts: rootRes.ReducedCosts,
			Nodes:        1,
			Iterations:   totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	if isIntegerFeasible(p, rootRes.Primal) {
		cand := slices.Clone(rootRes.Primal)
		snapToIntegers(p, cand)
		bV, rV, iV := p.MaxViolation(cand, true)
		if bV <= 1e-5 && rV <= 1e-5 && iV <= 1e-5 {
			obj := p.EvaluateObjective(cand)
			return Result{
				modelID:      p.modelID,
				Status:       StatusOptimal,
				Objective:    obj,
				BestBound:    obj,
				RootLPBound:  rootLPUser,
				RootCutBound: rootLPUser,
				Values:       cand,
				Nodes:        1,
				Iterations:   totalLPIters,
				SolveTime:    time.Since(startTime),
			}
		}
	}

	bestObjMin := math.Inf(1)
	var bestPrimal []float64

	var heurWS *Workspace
	var heurSolver *LPSolver
	if opts.EnableHeuristics {
		heurWS = AcquireWorkspace(n, p.NumConstraints)
		defer ReleaseWorkspace(heurWS)
		heurSolver = NewLPSolver(p, heurWS)
		heurSolver.Deadline = deadline
		heurSolver.CopyRootBasisFrom(lpSolver)

		if !timeExpired() {
			if sol, objMin, ok := tryPropagatedRounding(p, rootRes.Primal, p.VarLB, p.VarUB, heurSolver); ok {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
		if !timeExpired() && !isGapClosed(rootLPMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			heurSolver.CopyRootBasisFrom(lpSolver)
			if sol, objMin, ok := tryFractionalDiving(p, rootRes.Primal, p.VarLB, p.VarUB, heurSolver, bestObjMin, true); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
		if !timeExpired() && !isGapClosed(rootLPMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryRootSubMIP(p, lpSolver, rootRes.Primal, p.VarLB, p.VarUB, bestObjMin, deadline); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
	}

	if bestPrimal != nil && isGapClosed(rootLPMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		obj := p.EvaluateObjective(bestPrimal)
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    obj,
			BestBound:    obj,
			RootLPBound:  rootLPUser,
			RootCutBound: rootLPUser,
			Values:       bestPrimal,
			Nodes:        1,
			Iterations:   totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	curLPRes := rootRes
	cutsAdded := 0
	if opts.EnableCuts && opts.MaxCutPasses > 0 {
		stagnantRounds := 0
		maxCutsPerRound := min(60, max(15, p.NumConstraints/3))
		for pass := 0; pass < opts.MaxCutPasses; pass++ {
			if timeExpired() {
				break
			}
			if curLPRes.ObjectiveMin >= bestObjMin-opts.AbsGapTol || isGapClosed(curLPRes.ObjectiveMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
				break
			}
			newCuts := generateCutRound(lpSolver, curLPRes.Primal, maxCutsPerRound)
			if len(newCuts) == 0 || timeExpired() {
				break
			}
			prevObj := curLPRes.ObjectiveMin
			for _, c := range newCuts {
				lpSolver.AddCut(c.ColIdx, c.Val, c.RHS)
				cutsAdded++
			}
			nextRes := lpSolver.Solve()
			totalLPIters += nextRes.Iterations
			if nextRes.Status == StatusInfeasible {
				if bestPrimal != nil {
					break
				}
				return Result{
					modelID:     p.modelID,
					Status:      StatusInfeasible,
					RootLPBound: rootLPUser,
					Nodes:       1,
					Iterations:  totalLPIters,
					CutsAdded:   cutsAdded,
					SolveTime:   time.Since(startTime),
				}
			}
			if nextRes.Status != StatusOptimal {
				break
			}
			curLPRes = nextRes

			if isIntegerFeasible(p, curLPRes.Primal) {
				cand := slices.Clone(curLPRes.Primal)
				snapToIntegers(p, cand)
				bV, rV, iV := p.MaxViolation(cand, true)
				if bV <= 1e-5 && rV <= 1e-5 && iV <= 1e-5 {
					objMin := 0.0
					for j := range n {
						objMin += p.ObjMin[j] * cand[j]
					}
					if objMin < bestObjMin {
						bestObjMin = objMin
						bestPrimal = cand
					}
					break
				}
			}

			improvement := (curLPRes.ObjectiveMin - prevObj) / max(1.0, math.Abs(prevObj))
			if improvement < 1e-3 {
				stagnantRounds++
				if stagnantRounds >= 2 {
					break
				}
			} else {
				stagnantRounds = 0
			}
		}
	}

	rootCutMin := curLPRes.ObjectiveMin
	rootCutUser := p.ObjectiveFromMin(rootCutMin)

	postCutSubMIPFound := false
	if opts.EnableHeuristics && !timeExpired() && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		if sol, objMin, ok := tryRootSubMIP(p, lpSolver, curLPRes.Primal, p.VarLB, p.VarUB, bestObjMin, deadline); ok && objMin < bestObjMin {
			bestObjMin = objMin
			bestPrimal = sol
			postCutSubMIPFound = true
		}
		if !timeExpired() && !postCutSubMIPFound && cutsAdded > 0 && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryPropagatedRounding(p, curLPRes.Primal, p.VarLB, p.VarUB, heurSolver); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
		if !timeExpired() && !postCutSubMIPFound && cutsAdded > 0 && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryFractionalDiving(p, curLPRes.Primal, p.VarLB, p.VarUB, heurSolver, bestObjMin, false); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
	}

	if bestPrimal != nil && isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		obj := p.EvaluateObjective(bestPrimal)
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    obj,
			BestBound:    obj,
			RootLPBound:  rootLPUser,
			RootCutBound: rootCutUser,
			Values:       bestPrimal,
			Nodes:        1,
			Iterations:   totalLPIters,
			CutsAdded:    cutsAdded,
			SolveTime:    time.Since(startTime),
		}
	}

	pCosts := make([]pseudoCost, n)
	for _, j := range p.IntCols {
		objMag := max(1e-2, math.Abs(p.ObjMin[j])*0.1)
		pCosts[j] = pseudoCost{downSum: objMag, upSum: objMag}
	}

	pq := make(nodeQueue, 0, 256)
	heap.Init(&pq)

	curLB := slices.Clone(p.VarLB)
	curUB := slices.Clone(p.VarUB)

	nodesExplored := 1
	var plungeNode *searchNode
	hitNodeLimit := false
	hitTimeLimit := false
	hitLPLimitStatus := StatusNotSolved
	unsolvedMinBound := math.Inf(1)

	if timeExpired() {
		hitTimeLimit = true
		unsolvedMinBound = rootCutMin
	} else if opts.MaxNodes > 0 && nodesExplored >= opts.MaxNodes {
		hitNodeLimit = true
		unsolvedMinBound = rootCutMin
	} else {
		brVar, brVal := selectBranchVar(p, lpSolver, curLPRes, curLB, curUB, pCosts, 0, &totalLPIters)
		if brVar == -1 {
			cand := slices.Clone(curLPRes.Primal)
			snapToIntegers(p, cand)
			bV, rV, iV := p.MaxViolation(cand, true)
			if bV <= 1e-5 && rV <= 1e-5 && iV <= 1e-5 {
				obj := p.EvaluateObjective(cand)
				return Result{
					modelID:      p.modelID,
					Status:       StatusOptimal,
					Objective:    obj,
					BestBound:    obj,
					RootLPBound:  rootLPUser,
					RootCutBound: rootCutUser,
					Values:       cand,
					Nodes:        1,
					Iterations:   totalLPIters,
					CutsAdded:    cutsAdded,
					SolveTime:    time.Since(startTime),
				}
			}
			unsolvedMinBound = rootCutMin
			hitLPLimitStatus = StatusNumericalError
		} else {
			nodeCounter := 1
			downNode, upNode := createChildNodes(rootCutMin, 1, nil, brVar, brVal, curLB[brVar], curUB[brVar], pCosts[brVar], &nodeCounter)
			heap.Push(&pq, downNode)
			heap.Push(&pq, upNode)

			for pq.Len() > 0 || plungeNode != nil {
				if opts.MaxNodes > 0 && nodesExplored >= opts.MaxNodes {
					hitNodeLimit = true
					break
				}
				if timeExpired() {
					hitTimeLimit = true
					break
				}

				var node *searchNode
				if plungeNode != nil && (nodesExplored%8 != 0) {
					node = plungeNode
					plungeNode = nil
				} else {
					if plungeNode != nil {
						heap.Push(&pq, plungeNode)
						plungeNode = nil
					}
					node = heap.Pop(&pq).(*searchNode)
				}

				if node.lowerBound >= bestObjMin-opts.AbsGapTol || isGapClosed(node.lowerBound, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
					continue
				}

				nodesExplored++

				copy(curLB, p.VarLB)
				copy(curUB, p.VarUB)
				for _, d := range node.deltas {
					curLB[d.col] = max(curLB[d.col], d.lb)
					curUB[d.col] = min(curUB[d.col], d.ub)
				}

				if opts.EnablePresolve {
					if !propagateBounds(p, curLB, curUB, 2) {
						continue
					}
				}

				lpSolver.SetStructuralBounds(curLB, curUB)
				nodeRes := lpSolver.Solve()
				totalLPIters += nodeRes.Iterations

				if nodeRes.Status != StatusOptimal {
					if nodeRes.Status == StatusInfeasible {
						continue
					}
					if nodeRes.Status == StatusTimeLimit {
						heap.Push(&pq, node)
						hitTimeLimit = true
						break
					}
					// Retry from a fresh crash basis before giving up on this node.
					lpSolver.InitCrashBasis()
					lpSolver.SetStructuralBounds(curLB, curUB)
					nodeRes = lpSolver.Solve()
					totalLPIters += nodeRes.Iterations
					if nodeRes.Status == StatusInfeasible {
						continue
					}
					if nodeRes.Status == StatusTimeLimit {
						heap.Push(&pq, node)
						hitTimeLimit = true
						break
					}
					if nodeRes.Status != StatusOptimal {
						unsolvedMinBound = min(unsolvedMinBound, node.lowerBound)
						if hitLPLimitStatus == StatusNotSolved {
							hitLPLimitStatus = nodeRes.Status
						}
						continue
					}
				}
				if nodeRes.ObjectiveMin >= bestObjMin-opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
					continue
				}

				if len(node.deltas) > 0 {
					lastDelta := node.deltas[len(node.deltas)-1]
					gain := max(0.0, nodeRes.ObjectiveMin-node.lowerBound)
					if lastDelta.ub < p.VarUB[lastDelta.col] {
						pCosts[lastDelta.col].downSum += gain
						pCosts[lastDelta.col].downCount++
					} else {
						pCosts[lastDelta.col].upSum += gain
						pCosts[lastDelta.col].upCount++
					}
				}

				if isIntegerFeasible(p, nodeRes.Primal) {
					snapToIntegers(p, nodeRes.Primal)
					bV, rV, iV := p.MaxViolation(nodeRes.Primal, true)
					if bV <= 1e-5 && rV <= 1e-5 && iV <= 1e-5 {
						objMin := 0.0
						for j := range n {
							objMin += p.ObjMin[j] * nodeRes.Primal[j]
						}
						if objMin < bestObjMin {
							bestObjMin = objMin
							bestPrimal = slices.Clone(nodeRes.Primal)
						}
						continue
					}
				}

				if opts.EnableHeuristics && !timeExpired() && ((!postCutSubMIPFound && node.depth <= 5) || nodesExplored%4 == 0) {
					if sol, objMin, ok := tryPropagatedRounding(p, nodeRes.Primal, curLB, curUB, heurSolver); ok && objMin < bestObjMin {
						bestObjMin = objMin
						bestPrimal = sol
						if nodeRes.ObjectiveMin >= bestObjMin-opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
							continue
						}
					}
				}

				bVar, bVal := selectBranchVar(p, lpSolver, nodeRes, curLB, curUB, pCosts, node.depth, &totalLPIters)
				if bVar == -1 {
					continue
				}

				cDown, cUp := createChildNodes(
					nodeRes.ObjectiveMin,
					node.depth+1,
					node.deltas,
					bVar,
					bVal,
					curLB[bVar],
					curUB[bVar],
					pCosts[bVar],
					&nodeCounter,
				)

				frac := bVal - math.Floor(bVal)
				if frac >= 0.5 {
					plungeNode = cUp
					heap.Push(&pq, cDown)
				} else {
					plungeNode = cDown
					heap.Push(&pq, cUp)
				}
			}
		}
	}

	bestBoundMin := min(bestObjMin, unsolvedMinBound)
	for _, nd := range pq {
		if nd.lowerBound < bestBoundMin {
			bestBoundMin = nd.lowerBound
		}
	}
	if plungeNode != nil && plungeNode.lowerBound < bestBoundMin {
		bestBoundMin = plungeNode.lowerBound
	}

	if bestPrimal == nil {
		status := StatusInfeasible
		if hitTimeLimit || timeExpired() {
			status = StatusTimeLimit
		} else if hitNodeLimit || pq.Len() > 0 || plungeNode != nil {
			status = StatusNodeLimit
		} else if hitLPLimitStatus != StatusNotSolved {
			status = hitLPLimitStatus
		}
		bestBoundUser := 0.0
		if status != StatusInfeasible {
			if math.IsInf(bestBoundMin, 1) {
				bestBoundMin = rootCutMin
			}
			bestBoundUser = p.ObjectiveFromMin(bestBoundMin)
		}
		return Result{
			modelID:      p.modelID,
			Status:       status,
			BestBound:    bestBoundUser,
			RootLPBound:  rootLPUser,
			RootCutBound: rootCutUser,
			Nodes:        nodesExplored,
			Iterations:   totalLPIters,
			CutsAdded:    cutsAdded,
			SolveTime:    time.Since(startTime),
		}
	}

	status := StatusOptimal
	if !isGapClosed(bestBoundMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		if hitTimeLimit || timeExpired() {
			status = StatusTimeLimit
		} else if hitNodeLimit || pq.Len() > 0 || plungeNode != nil {
			status = StatusNodeLimit
		} else if hitLPLimitStatus != StatusNotSolved {
			status = hitLPLimitStatus
		}
	} else {
		bestBoundMin = bestObjMin
	}

	finalObj := p.EvaluateObjective(bestPrimal)
	return Result{
		modelID:      p.modelID,
		Status:       status,
		Objective:    finalObj,
		BestBound:    p.ObjectiveFromMin(bestBoundMin),
		RootLPBound:  rootLPUser,
		RootCutBound: rootCutUser,
		Values:       bestPrimal,
		Nodes:        nodesExplored,
		Iterations:   totalLPIters,
		CutsAdded:    cutsAdded,
		SolveTime:    time.Since(startTime),
	}
}

func createChildNodes(
	parentLB float64,
	depth int,
	parentDeltas []boundDelta,
	brVar int,
	brVal float64,
	curLB, curUB float64,
	pc pseudoCost,
	counter *int,
) (downNode, upNode *searchNode) {
	floorVal := math.Floor(brVal)
	ceilVal := math.Ceil(brVal)
	fDown := brVal - floorVal
	fUp := ceilVal - brVal

	avgDown := pc.downSum / float64(max(1, pc.downCount))
	avgUp := pc.upSum / float64(max(1, pc.upCount))

	downDeltas := make([]boundDelta, len(parentDeltas)+1)
	copy(downDeltas, parentDeltas)
	downDeltas[len(parentDeltas)] = boundDelta{col: brVar, lb: curLB, ub: floorVal}

	upDeltas := make([]boundDelta, len(parentDeltas)+1)
	copy(upDeltas, parentDeltas)
	upDeltas[len(parentDeltas)] = boundDelta{col: brVar, lb: ceilVal, ub: curUB}

	*counter++
	downID := *counter
	*counter++
	upID := *counter

	downNode = &searchNode{
		lowerBound: parentLB,
		estimate:   parentLB + fDown*avgDown,
		depth:      depth,
		id:         downID,
		deltas:     downDeltas,
	}
	upNode = &searchNode{
		lowerBound: parentLB,
		estimate:   parentLB + fUp*avgUp,
		depth:      depth,
		id:         upID,
		deltas:     upDeltas,
	}
	return downNode, upNode
}

func selectBranchVar(
	p *CompiledProblem,
	lpSolver *LPSolver,
	lpRes LPResult,
	curLB, curUB []float64,
	pCosts []pseudoCost,
	depth int,
	totalIters *int,
) (bestCol int, bestVal float64) {
	bestCol = -1
	bestScore := -1.0

	type cand struct {
		col   int
		val   float64
		score float64
	}
	var cands []cand

	for _, col := range p.IntCols {
		v := lpRes.Primal[col]
		f := v - math.Floor(v)
		if f <= intFeasTol || f >= 1.0-intFeasTol {
			continue
		}
		pc := pCosts[col]
		dGain := f * (pc.downSum / float64(max(1, pc.downCount)))
		uGain := (1.0 - f) * (pc.upSum / float64(max(1, pc.upCount)))
		score := max(dGain, 1e-4) * max(uGain, 1e-4)
		score += 1e-6 * (0.5 - math.Abs(f-0.5)) * (1.0 + math.Abs(p.ObjMin[col]))
		cands = append(cands, cand{col: col, val: v, score: score})
	}

	if len(cands) == 0 {
		return -1, 0
	}

	if depth <= 2 && len(cands) > 1 {
		slices.SortFunc(cands, func(a, b cand) int {
			if a.score > b.score {
				return -1
			}
			return 1
		})

		sbLimit := min(5, len(cands))
		savedMaxIter := lpSolver.MaxIter
		lpSolver.MaxIter = 25

		for idx := 0; idx < sbLimit; idx++ {
			if lpSolver.timeExpired() {
				break
			}
			c := &cands[idx]
			col := c.col
			if pCosts[col].downCount >= 2 && pCosts[col].upCount >= 2 {
				continue
			}
			v := c.val
			fl := math.Floor(v)
			cl := math.Ceil(v)
			origL, origU := curLB[col], curUB[col]

			curUB[col] = fl
			lpSolver.SetStructuralBounds(curLB, curUB)
			resDown := lpSolver.Solve()
			*totalIters += resDown.Iterations
			curUB[col] = origU

			curLB[col] = cl
			lpSolver.SetStructuralBounds(curLB, curUB)
			resUp := lpSolver.Solve()
			*totalIters += resUp.Iterations
			curLB[col] = origL

			dGain := 1e4
			if resDown.Status == StatusOptimal || resDown.Status == StatusIterationLimit {
				dGain = max(1e-4, resDown.ObjectiveMin-lpRes.ObjectiveMin)
				pCosts[col].downSum += dGain / (v - fl)
				pCosts[col].downCount++
			}
			uGain := 1e4
			if resUp.Status == StatusOptimal || resUp.Status == StatusIterationLimit {
				uGain = max(1e-4, resUp.ObjectiveMin-lpRes.ObjectiveMin)
				pCosts[col].upSum += uGain / (cl - v)
				pCosts[col].upCount++
			}
			c.score = max(dGain, 1e-4) * max(uGain, 1e-4)
		}

		lpSolver.MaxIter = savedMaxIter
		lpSolver.SetStructuralBounds(curLB, curUB)
	}

	for _, c := range cands {
		if c.score > bestScore {
			bestScore = c.score
			bestCol = c.col
			bestVal = c.val
		}
	}
	return bestCol, bestVal
}

func isIntegerFeasible(p *CompiledProblem, x []float64) bool {
	for _, col := range p.IntCols {
		v := x[col]
		if math.Abs(v-math.Round(v)) > intFeasTol {
			return false
		}
	}
	return true
}

func snapToIntegers(p *CompiledProblem, x []float64) {
	for _, col := range p.IntCols {
		v := x[col]
		if math.Abs(v-math.Round(v)) <= 1e-4 {
			x[col] = math.Round(v)
		}
	}
}

func isGapClosed(lowerBound, upperBound, relTol, absTol float64) bool {
	if math.IsInf(upperBound, 1) || math.IsInf(lowerBound, -1) {
		return false
	}
	diff := upperBound - lowerBound
	if diff <= absTol {
		return true
	}
	denom := max(1.0, math.Abs(upperBound))
	return (diff / denom) <= relTol
}
