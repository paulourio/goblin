package goblin

import (
	"container/heap"
	"math"
	"slices"
	"time"
)

// incumbentTol is the largest bound, row, or integer residual allowed when a
// rounded LP point becomes an incumbent.
const (
	incumbentTol        = 1e-5
	nodeQueueCmpTol     = 1e-7
	maxRootCutsCap      = 60
	minRootCutsFloor    = 15
	rootCutsRowDiv      = 3
	minCutRoundImprov   = 1e-3
	maxStagnantCutRound = 2
	minPseudoCostInit   = 1e-2
	pseudoCostObjScale  = 0.1
	initNodeQueueCap    = 256
	minPseudoScoreFloor = 1e-4
	pseudoTieScale      = 1e-6
	strongBranchCandCap = 5
	strongBranchInfeas  = 1e4
	strongBranchMaxIter = 25
	intSnapTol          = 1e-4
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
	// up is true on an upward branch.
	up bool
	// brDist is the step from the parent's LP value to the child bound.
	brDist float64
}

type nodeQueue []*searchNode

func (pq nodeQueue) Len() int { return len(pq) }
func (pq nodeQueue) Less(i, j int) bool {
	ni, nj := pq[i], pq[j]
	if dl := ni.lowerBound - nj.lowerBound; dl > nodeQueueCmpTol || dl < -nodeQueueCmpTol {
		return dl < 0
	}
	if de := ni.estimate - nj.estimate; de > nodeQueueCmpTol || de < -nodeQueueCmpTol {
		return de < 0
	}
	return ni.depth > nj.depth
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

func (pc pseudoCost) avgDown() float64 { return pc.downSum / float64(max(1, pc.downCount)) }
func (pc pseudoCost) avgUp() float64   { return pc.upSum / float64(max(1, pc.upCount)) }

// record adds an observed objective gain for a branching step of length dist. Steps shorter
// than minPseudoDist (such as fallback branches on near-integral values) are ignored because
// dividing by them would inflate the per-unit estimate by orders of magnitude.
func (pc *pseudoCost) record(up bool, gain, dist float64) {
	const minPseudoDist = 1e-3
	if dist < minPseudoDist {
		return
	}
	if up {
		pc.upSum += gain / dist
		pc.upCount++
	} else {
		pc.downSum += gain / dist
		pc.downCount++
	}
}

func solveBranchAndCutWithDone(p *CompiledProblem, opts Options, startTime, deadline time.Time, done <-chan struct{}) Result {
	n := p.NumVars
	if opts.TimeLimit > 0 {
		optDeadline := startTime.Add(opts.TimeLimit)
		if deadline.IsZero() || optDeadline.Before(deadline) {
			deadline = optDeadline
		}
	}
	if stopRequested(deadline, done) {
		return Result{
			modelID:   p.modelID,
			Status:    StatusTimeLimit,
			SolveTime: time.Since(startTime),
		}
	}
	timeExpired := func() bool {
		return stopRequested(deadline, done)
	}

	ws := AcquireWorkspace(n, p.NumConstraints)
	defer ReleaseWorkspace(ws)

	lpSolver := NewLPSolver(p, ws)
	lpSolver.Deadline = deadline
	lpSolver.Done = done
	lpSolver.DisableSIMD = opts.DisableSIMD
	defaultLPIter := lpSolver.MaxIter

	totalLPIters := 0
	iterLeft := func() int {
		if opts.MaxIter <= 0 {
			return math.MaxInt
		}
		return opts.MaxIter - totalLPIters
	}
	// capLPIter caps the next solve of s at min(base, remaining MaxIter budget) and
	// reports whether any budget remains.
	capLPIter := func(s *LPSolver, base int) bool {
		rem := iterLeft()
		if rem <= 0 {
			return false
		}
		s.MaxIter = min(base, rem)
		return true
	}
	// budgetBound reports whether an iteration-limited solve of s was cut short by MaxIter
	// rather than by the per-LP safeguard.
	budgetBound := func(s *LPSolver) bool { return s.MaxIter < defaultLPIter }

	capLPIter(lpSolver, defaultLPIter)
	rootRes := lpSolver.Solve()
	totalLPIters = rootRes.Iterations

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
		if cand, ok := roundedIncumbent(p, rootRes.Primal); ok {
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
	if opts.hasCutoff {
		bestObjMin = opts.cutoffMin - p.ObjConstant
	}
	var bestPrimal []float64
	isSubMIP := p.Name == "__submip__"
	if isSubMIP && bestObjMin <= rootLPMin+opts.AbsGapTol {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    math.NaN(),
			BestBound:    rootLPUser,
			RootLPBound:  rootLPUser,
			RootCutBound: rootLPUser,
			Nodes:        1,
			Iterations:   totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	var heurWS *Workspace
	var heurSolver *LPSolver
	if opts.EnableHeuristics {
		heurWS = AcquireWorkspace(n, p.NumConstraints)
		defer ReleaseWorkspace(heurWS)
		heurSolver = NewLPSolver(p, heurWS)
		heurSolver.Deadline = deadline
		heurSolver.Done = done
		heurSolver.DisableSIMD = opts.DisableSIMD
		heurSolver.CopyRootBasisFrom(lpSolver)

		if !timeExpired() {
			if sol, objMin, ok := tryPropagatedRounding(p, rootRes.Primal, p.VarLB, p.VarUB, heurSolver); ok && objMin < bestObjMin {
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
		if (!opts.EnableCuts || opts.MaxCutPasses <= 0 || isGapClosed(rootLPMin, bestObjMin, 0.20, opts.AbsGapTol)) && !timeExpired() && !isGapClosed(rootLPMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryRootSubMIP(p, lpSolver, rootRes.Primal, p.VarLB, p.VarUB, bestObjMin, deadline); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
	}

	if bestPrimal != nil && isGapClosed(rootLPMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(bestPrimal),
			BestBound:    rootLPUser,
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
	hitTimeLimit := false
	hitIterLimit := false
	if opts.EnableCuts && opts.MaxCutPasses > 0 {
		stagnantRounds := 0
		maxCutsPerRound := min(maxRootCutsCap, max(minRootCutsFloor, p.NumConstraints/rootCutsRowDiv))
		for range opts.MaxCutPasses {
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
			if !capLPIter(lpSolver, defaultLPIter) {
				hitIterLimit = true
				break
			}
			prevObj := curLPRes.ObjectiveMin
			snap := lpSolver.snapshot()
			rows := make([]dynamicRow, len(newCuts))
			for k, c := range newCuts {
				rows[k] = lpSolver.newCutRow(c.ColIdx, c.Val, c.RHS)
			}
			lpSolver.addCutRows(rows)
			nextRes := lpSolver.Solve()
			totalLPIters += nextRes.Iterations
			if nextRes.Status != StatusOptimal {
				// Valid cuts never make a feasible LP infeasible, and a failed re-solve
				// leaves no usable basis. Roll the round back so branch-and-bound starts
				// from the last optimal LP state.
				lpSolver.restore(snap)
				switch {
				case nextRes.Status == StatusTimeLimit:
					hitTimeLimit = true
				case nextRes.Status == StatusIterationLimit && budgetBound(lpSolver):
					hitIterLimit = true
				}
				break
			}
			cutsAdded += len(newCuts)
			curLPRes = nextRes

			if isIntegerFeasible(p, curLPRes.Primal) {
				if cand, ok := roundedIncumbent(p, curLPRes.Primal); ok {
					if objMin := minObjective(p, cand); objMin < bestObjMin {
						bestObjMin = objMin
						bestPrimal = cand
					}
					break
				}
			}

			improvement := (curLPRes.ObjectiveMin - prevObj) / max(1.0, math.Abs(prevObj))
			if improvement < minCutRoundImprov {
				stagnantRounds++
				if stagnantRounds >= maxStagnantCutRound {
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
	if opts.EnableHeuristics && cutsAdded > 0 && !timeExpired() && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		if sol, objMin, ok := tryRootSubMIP(p, lpSolver, curLPRes.Primal, p.VarLB, p.VarUB, bestObjMin, deadline); ok && objMin < bestObjMin {
			bestObjMin = objMin
			bestPrimal = sol
			postCutSubMIPFound = true
		}
		if !timeExpired() && !postCutSubMIPFound && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryPropagatedRounding(p, curLPRes.Primal, p.VarLB, p.VarUB, heurSolver); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
		if !timeExpired() && !postCutSubMIPFound && !isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
			if sol, objMin, ok := tryFractionalDiving(p, curLPRes.Primal, p.VarLB, p.VarUB, heurSolver, bestObjMin, p.NumVars > 100); ok && objMin < bestObjMin {
				bestObjMin = objMin
				bestPrimal = sol
			}
		}
	}

	if bestPrimal != nil && isGapClosed(rootCutMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(bestPrimal),
			BestBound:    rootCutUser,
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
		objMag := max(minPseudoCostInit, math.Abs(p.ObjMin[j])*pseudoCostObjScale)
		pCosts[j] = pseudoCost{downSum: objMag, upSum: objMag}
	}

	pq := make(nodeQueue, 0, initNodeQueueCap)
	heap.Init(&pq)

	baseLB := slices.Clone(p.VarLB)
	baseUB := slices.Clone(p.VarUB)
	tightenUnboundedIntCols(p, baseLB, baseUB, bestObjMin)
	curLB := slices.Clone(baseLB)
	curUB := slices.Clone(baseUB)

	nodesExplored := 1
	nodeCounter := 1
	var plungeNode *searchNode
	hitNodeLimit := false
	hitLPLimitStatus := StatusNotSolved
	unsolvedMinBound := math.Inf(1)

	switch {
	case hitTimeLimit || timeExpired():
		hitTimeLimit = true
		unsolvedMinBound = rootCutMin
	case hitIterLimit || iterLeft() <= 0:
		hitIterLimit = true
		unsolvedMinBound = rootCutMin
	case opts.MaxNodes > 0 && nodesExplored >= opts.MaxNodes:
		hitNodeLimit = true
		unsolvedMinBound = rootCutMin
	default:
		brVar, brVal := selectBranchVar(p, lpSolver, curLPRes, curLB, curUB, pCosts, 0, &totalLPIters, iterLeft)
		if brVar == -1 {
			if cand, ok := roundedIncumbent(p, curLPRes.Primal); ok {
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
			brVar, brVal = selectNearIntegralBranchVar(p, curLPRes.Primal, curLB, curUB)
		}
		if brVar == -1 {
			unsolvedMinBound = rootCutMin
			hitLPLimitStatus = StatusNumericalError
			break
		}
		downNode, upNode := createChildNodes(rootCutMin, 1, nil, brVar, brVal, curLB[brVar], curUB[brVar], pCosts[brVar], &nodeCounter)
		heap.Push(&pq, downNode)
		heap.Push(&pq, upNode)

		lastIncumbentNode := 1
		heurMisses := 0
		for pq.Len() > 0 || plungeNode != nil {
			if isSubMIP && bestPrimal != nil && nodesExplored-lastIncumbentNode >= 48 {
				hitNodeLimit = true
				break
			}
			if opts.MaxNodes > 0 && nodesExplored >= opts.MaxNodes {
				hitNodeLimit = true
				break
			}
			if timeExpired() {
				hitTimeLimit = true
				break
			}
			if iterLeft() <= 0 {
				hitIterLimit = true
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

			copy(curLB, baseLB)
			copy(curUB, baseUB)
			for _, d := range node.deltas {
				curLB[d.col] = max(curLB[d.col], d.lb)
				curUB[d.col] = min(curUB[d.col], d.ub)
			}

			if opts.EnablePresolve {
				if !propagateBounds(p, curLB, curUB, nodePropPasses) {
					continue
				}
			}

			nodeRes, stop := solveNodeLP(lpSolver, curLB, curUB, defaultLPIter, capLPIter, budgetBound, &totalLPIters)
			if stop != StatusNotSolved {
				// The node was not evaluated: keep it open so its bound stays in the global bound.
				heap.Push(&pq, node)
				if stop == StatusTimeLimit {
					hitTimeLimit = true
				} else {
					hitIterLimit = true
				}
				break
			}
			if nodeRes.Status == StatusInfeasible {
				continue
			}
			if nodeRes.Status != StatusOptimal {
				unsolvedMinBound = min(unsolvedMinBound, node.lowerBound)
				if hitLPLimitStatus == StatusNotSolved {
					hitLPLimitStatus = nodeRes.Status
				}
				continue
			}
			if nodeRes.ObjectiveMin >= bestObjMin-opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
				continue
			}

			if len(node.deltas) > 0 {
				gain := max(0.0, nodeRes.ObjectiveMin-node.lowerBound)
				lastCol := node.deltas[len(node.deltas)-1].col
				pCosts[lastCol].record(node.up, gain, node.brDist)
			}

			if isIntegerFeasible(p, nodeRes.Primal) {
				if cand, ok := roundedIncumbent(p, nodeRes.Primal); ok {
					if objMin := minObjective(p, cand); objMin < bestObjMin {
						bestObjMin = objMin
						bestPrimal = cand
						lastIncumbentNode = nodesExplored
						heurMisses = 0
					}
					continue
				}
			}

			heurStride := 4
			if n > 200 || heurMisses >= 4 {
				heurStride = 16
			}
			if n <= 200 && heurMisses >= 12 {
				heurStride = 64
			}
			if opts.EnableHeuristics && !timeExpired() && ((!postCutSubMIPFound && node.depth <= 5 && (n <= 200 || node.depth <= 1)) || nodesExplored%heurStride == 0) {
				if sol, objMin, ok := tryPropagatedRounding(p, nodeRes.Primal, curLB, curUB, heurSolver); ok && objMin < bestObjMin {
					bestObjMin = objMin
					bestPrimal = sol
					lastIncumbentNode = nodesExplored
					heurMisses = 0
					if nodeRes.ObjectiveMin >= bestObjMin-opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
						continue
					}
				} else {
					heurMisses++
				}
			}

			bVar, bVal := selectBranchVar(p, lpSolver, nodeRes, curLB, curUB, pCosts, node.depth, &totalLPIters, iterLeft)
			if bVar == -1 {
				// The LP point is integral within intFeasTol, yet its rounding violates the
				// rows (typical with big-M constraints). Branch on the integer variable furthest
				// from an integer so the subtree is explored instead of discarded.
				bVar, bVal = selectNearIntegralBranchVar(p, nodeRes.Primal, curLB, curUB)
			}
			if bVar == -1 {
				// Nothing can be branched on: keep the node's bound so the final status
				// never claims an optimal proof over an unexplored region.
				unsolvedMinBound = min(unsolvedMinBound, nodeRes.ObjectiveMin)
				if hitLPLimitStatus == StatusNotSolved {
					hitLPLimitStatus = StatusNumericalError
				}
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
			if frac >= halfFracTarget {
				plungeNode = cUp
				heap.Push(&pq, cDown)
			} else {
				plungeNode = cDown
				heap.Push(&pq, cUp)
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

	limitStatus := func() Status {
		switch {
		case hitTimeLimit:
			return StatusTimeLimit
		case hitIterLimit:
			return StatusIterationLimit
		case hitNodeLimit || pq.Len() > 0 || plungeNode != nil:
			return StatusNodeLimit
		case hitLPLimitStatus != StatusNotSolved:
			return hitLPLimitStatus
		}
		return StatusOptimal
	}

	if bestPrimal == nil {
		status := limitStatus()
		bestBoundUser := 0.0
		if status == StatusOptimal {
			status = StatusInfeasible
		} else {
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
		status = limitStatus()
		if status == StatusOptimal {
			// Unreachable in exact bookkeeping: an open gap implies an open or unsolved node.
			status = StatusUndefined
		}
	}

	return Result{
		modelID:      p.modelID,
		Status:       status,
		Objective:    p.EvaluateObjective(bestPrimal),
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

// solveNodeLP solves the node LP with bounds (lb, ub), retrying once from a fresh
// crash basis when the warm-started solve fails. stop is StatusTimeLimit or
// StatusIterationLimit when the node could not be evaluated because a global resource limit
// was reached, and StatusNotSolved otherwise.
func solveNodeLP(
	s *LPSolver,
	lb, ub []float64,
	defaultLPIter int,
	capLPIter func(*LPSolver, int) bool,
	budgetBound func(*LPSolver) bool,
	totalIters *int,
) (res LPResult, stop Status) {
	for attempt := range 2 {
		if attempt == 1 {
			s.InitCrashBasis()
		}
		if !capLPIter(s, defaultLPIter) {
			return LPResult{Status: StatusIterationLimit}, StatusIterationLimit
		}
		s.SetStructuralBounds(lb, ub)
		res = s.Solve()
		*totalIters += res.Iterations
		switch {
		case res.Status == StatusOptimal || res.Status == StatusInfeasible:
			return res, StatusNotSolved
		case res.Status == StatusTimeLimit:
			return res, StatusTimeLimit
		case res.Status == StatusIterationLimit && budgetBound(s):
			return res, StatusIterationLimit
		}
	}
	return res, StatusNotSolved
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
		estimate:   parentLB + fDown*pc.avgDown(),
		depth:      depth,
		id:         downID,
		deltas:     downDeltas,
		up:         false,
		brDist:     fDown,
	}
	upNode = &searchNode{
		lowerBound: parentLB,
		estimate:   parentLB + fUp*pc.avgUp(),
		depth:      depth,
		id:         upID,
		deltas:     upDeltas,
		up:         true,
		brDist:     fUp,
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
	iterLeft func() int,
) (bestCol int, bestVal float64) {
	bestCol = -1
	bestScore := -1.0

	type cand struct {
		col        int
		val        float64
		dEst, uEst float64
		score      float64
	}
	var cands []cand

	for _, col := range p.IntCols {
		v := lpRes.Primal[col]
		f := v - math.Floor(v)
		if f <= intFeasTol || f >= 1.0-intFeasTol {
			continue
		}
		pc := pCosts[col]
		dEst := f * pc.avgDown()
		uEst := (1.0 - f) * pc.avgUp()
		score := max(dEst, minPseudoScoreFloor) * max(uEst, minPseudoScoreFloor)
		score += pseudoTieScale * (halfFracTarget - math.Abs(f-halfFracTarget)) * (1.0 + math.Abs(p.ObjMin[col]))
		cands = append(cands, cand{col: col, val: v, dEst: dEst, uEst: uEst, score: score})
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

		sbLimit := min(strongBranchCandCap, len(cands))
		savedMaxIter := lpSolver.MaxIter

		// childGain scores one strong-branching child. Only optimal re-solves yield a valid
		// objective (an iteration-limited LPResult carries no objective), so other outcomes
		// fall back to the pseudocost estimate and never feed the pseudocost statistics.
		childGain := func(res LPResult, est, dist float64, pc *pseudoCost, up bool) float64 {
			switch res.Status {
			case StatusOptimal:
				g := max(minPseudoScoreFloor, res.ObjectiveMin-lpRes.ObjectiveMin)
				pc.record(up, g, dist)
				return g
			case StatusInfeasible:
				return strongBranchInfeas
			default:
				return est
			}
		}

		for idx := range sbLimit {
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

			sbIter := min(strongBranchMaxIter, iterLeft())
			if sbIter <= 0 {
				break
			}
			lpSolver.MaxIter = sbIter
			curUB[col] = fl
			lpSolver.SetStructuralBounds(curLB, curUB)
			resDown := lpSolver.Solve()
			*totalIters += resDown.Iterations
			curUB[col] = origU

			sbIter = min(strongBranchMaxIter, iterLeft())
			if sbIter <= 0 {
				break
			}
			lpSolver.MaxIter = sbIter
			curLB[col] = cl
			lpSolver.SetStructuralBounds(curLB, curUB)
			resUp := lpSolver.Solve()
			*totalIters += resUp.Iterations
			curLB[col] = origL

			dGain := childGain(resDown, c.dEst, v-fl, &pCosts[col], false)
			uGain := childGain(resUp, c.uEst, cl-v, &pCosts[col], true)
			c.score = max(dGain, minPseudoScoreFloor) * max(uGain, minPseudoScoreFloor)
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

// selectNearIntegralBranchVar returns the non-fixed integer column whose LP value lies
// furthest from an integer (possibly below intFeasTol), or -1 when every integer column is
// fixed or exactly integral. It is the fallback branching rule for LP points that are
// integral within tolerance but whose rounding is infeasible.
func selectNearIntegralBranchVar(p *CompiledProblem, x, curLB, curUB []float64) (int, float64) {
	best, bestDist := -1, 0.0
	for _, col := range p.IntCols {
		if curUB[col]-curLB[col] < halfFracTarget {
			continue
		}
		v := x[col]
		if d := math.Abs(v - math.Round(v)); d > bestDist {
			best, bestDist = col, d
		}
	}
	if best == -1 {
		return -1, 0
	}
	return best, x[best]
}

// roundedIncumbent snaps the near-integral integer components of x and checks all bounds and rows within incumbentTol.
func roundedIncumbent(p *CompiledProblem, x []float64) ([]float64, bool) {
	cand := slices.Clone(x)
	snapToIntegers(p, cand)
	bV, rV, iV := p.MaxViolation(cand, true)
	return cand, bV <= incumbentTol && rV <= incumbentTol && iV <= incumbentTol
}

// minObjective computes sum_j ObjMin[j]*x[j].
func minObjective(p *CompiledProblem, x []float64) float64 {
	obj := 0.0
	for j := range p.NumVars {
		obj += p.ObjMin[j] * x[j]
	}
	return obj
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
		if math.Abs(v-math.Round(v)) <= intSnapTol {
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

func tightenUnboundedIntCols(p *CompiledProblem, baseLB, baseUB []float64, bestObjMin float64) {
	if math.IsInf(bestObjMin, 1) {
		return
	}
	hasInfInt := false
	for _, col := range p.IntCols {
		if math.IsInf(baseLB[col], -1) || math.IsInf(baseUB[col], 1) {
			hasInfInt = true
			break
		}
	}
	if !hasInfInt {
		return
	}
	n := p.NumVars
	minObj := 0.0
	for j := range n {
		c := p.ObjMin[j]
		if c > 0 {
			if math.IsInf(baseLB[j], -1) {
				return
			}
			minObj += c * baseLB[j]
		} else if c < 0 {
			if math.IsInf(baseUB[j], 1) {
				return
			}
			minObj += c * baseUB[j]
		}
	}
	slack := bestObjMin - minObj
	if slack < -1e-6 {
		return
	}
	slack = max(0.0, slack)
	tlb := slices.Clone(baseLB)
	tub := slices.Clone(baseUB)
	for j := range n {
		c := p.ObjMin[j]
		if c > 1e-12 {
			u := tlb[j] + slack/c
			if p.VarCat[j] != Continuous {
				u = math.Floor(u + heurIntBoundTol)
			}
			if u < tub[j] {
				tub[j] = max(tlb[j], u)
			}
		} else if c < -1e-12 {
			l := tub[j] + slack/c
			if p.VarCat[j] != Continuous {
				l = math.Ceil(l - heurIntBoundTol)
			}
			if l > tlb[j] {
				tlb[j] = min(tub[j], l)
			}
		}
	}
	if !propagateBounds(p, tlb, tub, 3) {
		return
	}
	for _, col := range p.IntCols {
		if math.IsInf(baseUB[col], 1) && !math.IsInf(tub[col], 1) {
			baseUB[col] = tub[col]
		}
		if math.IsInf(baseLB[col], -1) && !math.IsInf(tlb[col], -1) {
			baseLB[col] = tlb[col]
		}
	}
}
