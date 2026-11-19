package goblin

import (
	"cmp"
	"container/heap"
	"math"
	"slices"
	"time"
)

// incumbentTol is the largest bound, row, or integer residual allowed when a
// rounded LP point becomes an incumbent.
const (
	incumbentTol            = 1e-6
	nodeQueueCmpTol         = 1e-7
	maxRootCutsCap          = 60
	minRootCutsFloor        = 15
	rootCutsRowDiv          = 3
	minCutRoundImprov       = 1e-3
	maxStagnantCutRound     = 2
	zeroGainAbsTol          = 1e-6
	zeroGainRelTol          = 1e-7
	slackCutPurgeTol        = 0.05
	minPseudoCostInit       = 1e-2
	pseudoCostObjScale      = 0.1
	minPseudoDist           = 1e-3
	initNodeQueueCap        = 256
	minPseudoScoreFloor     = 1e-4
	pseudoTieScale          = 1e-6
	strongBranchMaxDepth    = 2
	strongBranchReliableCnt = 2
	strongBranchCandCap     = 5
	strongBranchInfeas      = 1e4
	strongBranchMaxIter     = 25
	intSnapTol              = 1e-4
	postCutDiveVarMin       = 100
	subMIPStagnantNodeCap   = 48
	plungeQueuePeriod       = 8
	largeModelHeurVarCap    = 200
	heurShallowDepth        = 5
	heurBaseStride          = 4
	heurMedStride           = 16
	heurSlowStride          = 64
	heurMedMissCap          = 4
	heurSlowMissCap         = 12
	tightenObjPasses        = 3
	nodeCompactStepCap      = 128
	rootIterExpandMult      = 4
)

type boundDelta struct {
	col int
	lb  float64
	ub  float64
}

type searchNode struct {
	parent     *searchNode
	base       []boundDelta
	delta      boundDelta
	lowerBound float64
	estimate   float64
	// brDist is the step from the parent's LP value to the child bound.
	brDist   float64
	depth    int
	id       int
	steps    uint16
	hasDelta bool
	// up is true on an upward branch.
	up bool
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

// babEngine holds the orchestration state for a single Branch-and-Cut solve.
type babEngine struct {
	p                  *CompiledProblem
	opts               Options
	startTime          time.Time
	deadline           time.Time
	done               <-chan struct{}
	lpSolver           *LPSolver
	heurSolver         *LPSolver
	defaultLPIter      int
	totalLPIters       int
	rootLPMin          float64
	rootLPUser         float64
	rootCutMin         float64
	rootCutUser        float64
	bestObjMin         float64
	bestPrimal         []float64
	cutsAdded          int
	postCutSubMIPFound bool
	hitTimeLimit       bool
	hitIterLimit       bool
	hitNodeLimit       bool
	hitLPLimitStatus   Status
	unsolvedMinBound   float64
	prunedMinBound     float64
	nodesExplored      int
	plungeNode         *searchNode
	pq                 nodeQueue
	fixScratchLB       []float64
	fixScratchUB       []float64
}

func (b *babEngine) ensureHeurSolver() *LPSolver {
	if b.heurSolver == nil {
		b.heurSolver = NewLPSolver(b.p, nil)
		b.heurSolver.Deadline = b.deadline
		b.heurSolver.Done = b.done
		b.heurSolver.DisableSIMD = b.opts.DisableSIMD
		if b.lpSolver != nil && b.lpSolver.dualValid {
			b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		}
	}
	return b.heurSolver
}

func hasInfeasibleIntegerEqRow(p *CompiledProblem) bool {
	if !p.isMIP {
		return false
	}
	for i := range p.numConstraints {
		if p.rowSense[i] != Equal {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if s == e {
			if math.Abs(p.rowRHS[i]) > primalTol {
				return true
			}
			continue
		}
		allExactInt := true
		var g int64
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if p.varCat[col] == Continuous {
				allExactInt = false
				break
			}
			v := p.rowVal[k]
			rv := math.Round(v)
			if v != rv || math.Abs(rv) > presolveGCDMaxCoeff {
				allExactInt = false
				break
			}
			if iv := int64(math.Abs(rv)); iv > 0 {
				g = gcdInt64(g, iv)
			}
		}
		if allExactInt && g > 0 {
			scaled := p.rowRHS[i] / float64(g)
			if math.Abs(scaled-math.Round(scaled)) > presolveFeasTol {
				return true
			}
		}
	}
	return false
}

func (b *babEngine) timeExpired() bool {
	return stopRequested(b.deadline, b.done)
}

func (b *babEngine) iterLeft() int {
	if b.opts.MaxIter <= 0 {
		return math.MaxInt
	}
	return b.opts.MaxIter - b.totalLPIters
}

func (b *babEngine) nodesLeft() int {
	if b.opts.MaxNodes <= 0 {
		return math.MaxInt
	}
	return b.opts.MaxNodes - b.nodesExplored
}

// capLPIter caps the next solve of s at min(base, remaining MaxIter budget) and reports whether any budget remains.
func (b *babEngine) capLPIter(s *LPSolver, base int) bool {
	rem := b.iterLeft()
	if rem <= 0 {
		return false
	}
	s.MaxIter = min(base, rem)
	return true
}

// budgetBound reports whether an iteration-limited solve of s was cut short by MaxIter rather than by the per-LP safeguard.
func (b *babEngine) budgetBound(s *LPSolver) bool {
	return s.MaxIter < b.defaultLPIter
}

func solveBranchAndCutWithDone(p *CompiledProblem, opts Options, startTime, deadline time.Time, done <-chan struct{}) Result {
	n := p.numVars
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
	if !opts.EnablePresolve && hasInfeasibleIntegerEqRow(p) {
		return Result{
			modelID:   p.modelID,
			Status:    StatusInfeasible,
			SolveTime: time.Since(startTime),
		}
	}

	ws := AcquireWorkspace(n, p.numConstraints)
	defer ReleaseWorkspace(ws)

	lpSolver := NewLPSolver(p, ws)
	lpSolver.Deadline = deadline
	lpSolver.Done = done
	lpSolver.DisableSIMD = opts.DisableSIMD

	b := &babEngine{
		p:                p,
		opts:             opts,
		startTime:        startTime,
		deadline:         deadline,
		done:             done,
		lpSolver:         lpSolver,
		defaultLPIter:    lpSolver.MaxIter,
		bestObjMin:       math.Inf(1),
		hitLPLimitStatus: StatusNotSolved,
		unsolvedMinBound: math.Inf(1),
		prunedMinBound:   math.Inf(1),
		nodesExplored:    1,
	}
	defer func() {
		b.heurSolver.Close()
	}()

	b.capLPIter(lpSolver, b.defaultLPIter)
	rootRes := lpSolver.Solve()
	b.totalLPIters = rootRes.Iterations
	if rootRes.Status == StatusIterationLimit && !b.budgetBound(lpSolver) && !b.timeExpired() {
		if b.capLPIter(lpSolver, rootIterExpandMult*b.defaultLPIter) {
			retryRes := lpSolver.Solve()
			b.totalLPIters += retryRes.Iterations
			rootRes = retryRes
		}
	}

	if rootRes.Status != StatusOptimal {
		if rootRes.Status == StatusUnbounded && p.isMIP && len(p.intCols) > 0 {
			return b.solveUnboundedMIPFeasibility()
		}
		return Result{
			modelID:    p.modelID,
			Status:     rootRes.Status,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}

	if math.IsNaN(rootRes.ObjectiveMin) || math.IsInf(rootRes.ObjectiveMin, 0) {
		return Result{
			modelID:    p.modelID,
			Status:     StatusNumericalError,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}
	b.rootLPMin = rootRes.ObjectiveMin
	b.rootLPUser = p.ObjectiveFromMin(b.rootLPMin)

	if !p.isMIP || len(p.intCols) == 0 {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    rootRes.Objective,
			BestBound:    rootRes.Objective,
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootLPUser,
			Values:       rootRes.Primal,
			Duals:        rootRes.Dual,
			ReducedCosts: rootRes.ReducedCosts,
			Nodes:        1,
			Iterations:   b.totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	if opts.hasCutoff {
		b.bestObjMin = opts.cutoffMin - p.objConstant
	}
	if opts.isSubMIP && b.bestObjMin <= b.rootLPMin+opts.AbsGapTol {
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    math.NaN(),
			BestBound:    b.rootLPUser,
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootLPUser,
			Nodes:        1,
			Iterations:   b.totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	if isIntegerFeasible(p, rootRes.Primal) {
		if cand, ok := b.polishIncumbent(rootRes.Primal); ok {
			if candMin := minObjective(p, cand); candMin < b.bestObjMin {
				b.bestObjMin = candMin
				b.bestPrimal = cand
			}
			if b.bestPrimal != nil && isGapClosed(b.rootLPMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
				obj := p.EvaluateObjective(b.bestPrimal)
				boundMin := min(b.bestObjMin, b.rootLPMin)
				return Result{
					modelID:      p.modelID,
					Status:       StatusOptimal,
					Objective:    obj,
					BestBound:    p.ObjectiveFromMin(boundMin),
					RootLPBound:  b.rootLPUser,
					RootCutBound: b.rootLPUser,
					Values:       b.bestPrimal,
					Nodes:        1,
					Iterations:   b.totalLPIters,
					SolveTime:    time.Since(startTime),
				}
			}
		}
	}

	if opts.EnableHeuristics {
		b.ensureHeurSolver()
		b.runPreCutHeuristics(rootRes.Primal)
	}

	if b.bestPrimal != nil && isGapClosed(b.rootLPMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		boundMin := min(b.bestObjMin, b.rootLPMin)
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(b.bestPrimal),
			BestBound:    p.ObjectiveFromMin(boundMin),
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootLPUser,
			Values:       b.bestPrimal,
			Nodes:        b.nodesExplored,
			Iterations:   b.totalLPIters,
			SolveTime:    time.Since(startTime),
		}
	}

	curLPRes := b.runRootCutLoop(rootRes)
	b.rootCutMin = curLPRes.ObjectiveMin
	b.rootCutUser = p.ObjectiveFromMin(b.rootCutMin)

	b.runPostCutHeuristics(curLPRes.Primal)
	if b.bestPrimal != nil && isGapClosed(b.rootCutMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		boundMin := min(b.bestObjMin, b.rootCutMin)
		return Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(b.bestPrimal),
			BestBound:    p.ObjectiveFromMin(boundMin),
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootCutUser,
			Values:       b.bestPrimal,
			Nodes:        b.nodesExplored,
			Iterations:   b.totalLPIters,
			CutsAdded:    b.cutsAdded,
			SolveTime:    time.Since(startTime),
		}
	}

	if earlyRes, doneEarly := b.exploreTree(curLPRes); doneEarly {
		return earlyRes
	}
	return b.finalizeResult()
}

// solveUnboundedMIPFeasibility resolves an LP-unbounded MILP by checking whether its integer hull is non-empty.
// By Meyer's theorem for rational MILPs, an LP-unbounded MILP is unbounded if and only if an integer-feasible point exists.
func (b *babEngine) solveUnboundedMIPFeasibility() Result {
	p := b.p
	feasProb := *p
	feasProb.objCoeffs = make([]float64, p.numVars)
	feasProb.objMin = make([]float64, p.numVars)
	feasProb.objConstant = 0
	feasOpts := b.opts
	if b.opts.MaxIter > 0 {
		rem := b.opts.MaxIter - b.totalLPIters
		if rem <= 0 {
			return Result{
				modelID:    p.modelID,
				Status:     StatusIterationLimit,
				Nodes:      b.nodesExplored,
				Iterations: b.totalLPIters,
				SolveTime:  time.Since(b.startTime),
			}
		}
		feasOpts.MaxIter = rem
	}
	if b.opts.MaxNodes > 0 {
		remNodes := b.opts.MaxNodes - b.nodesExplored
		if remNodes <= 0 {
			return Result{
				modelID:    p.modelID,
				Status:     StatusNodeLimit,
				Nodes:      b.nodesExplored,
				Iterations: b.totalLPIters,
				SolveTime:  time.Since(b.startTime),
			}
		}
		feasOpts.MaxNodes = remNodes
	}
	feasRes := solveBranchAndCutWithDone(&feasProb, feasOpts, b.startTime, b.deadline, b.done)
	st := feasRes.Status
	if st == StatusOptimal {
		st = StatusUnbounded
	}
	return Result{
		modelID:    p.modelID,
		Status:     st,
		Nodes:      b.nodesExplored + feasRes.Nodes,
		Iterations: b.totalLPIters + feasRes.Iterations,
		CutsAdded:  feasRes.CutsAdded,
		SolveTime:  time.Since(b.startTime),
	}
}

func (b *babEngine) runPreCutHeuristics(rootPrimal []float64) {
	p := b.p
	b.heurSolver.CopyRootBasisFrom(b.lpSolver)
	if !b.timeExpired() {
		if sol, objMin, ok := tryPropagatedRounding(p, rootPrimal, p.varLB, p.varUB, b.heurSolver); ok && objMin < b.bestObjMin {
			b.bestObjMin = objMin
			b.bestPrimal = sol
		}
	}
	if !b.timeExpired() && !isGapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		if sol, objMin, ok := tryFractionalDiving(p, rootPrimal, p.varLB, p.varUB, b.heurSolver, b.bestObjMin, true); ok && objMin < b.bestObjMin {
			b.bestObjMin = objMin
			b.bestPrimal = sol
		}
	}
	if !b.timeExpired() && !isGapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		remNodes := b.nodesLeft()
		remIter := b.iterLeft()
		if remNodes > 0 && remIter > 0 {
			b.heurSolver.CopyRootBasisFrom(b.lpSolver)
			sol, objMin, usedN, usedIt, ok := tryRootSubMIP(p, b.heurSolver, rootPrimal, p.varLB, p.varUB, b.bestObjMin, b.deadline, b.opts.isSubMIP, remNodes, remIter)
			b.nodesExplored += usedN
			b.totalLPIters += usedIt
			if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
				b.hitIterLimit = true
			}
			if ok && objMin < b.bestObjMin {
				b.bestObjMin = objMin
				b.bestPrimal = sol
			}
		}
	}
}

// runRootCutLoop executes up to MaxCutPasses cutting-plane rounds at the root node,
// rolling back non-optimal or zero-gain trailing rounds and purging slack cuts before branching.
func (b *babEngine) runRootCutLoop(rootRes LPResult) LPResult {
	curLPRes := rootRes
	if !b.opts.EnableCuts || b.opts.MaxCutPasses <= 0 {
		return curLPRes
	}
	p := b.p
	lpSolver := b.lpSolver
	stagnantRounds := 0
	maxCutsPerRound := min(maxRootCutsCap, max(minRootCutsFloor, p.numConstraints/rootCutsRowDiv))
	anchorSnap := lpSolver.snapshot()
	anchorLPRes := curLPRes
	anchorCuts := 0

	for range b.opts.MaxCutPasses {
		if b.timeExpired() {
			break
		}
		if curLPRes.ObjectiveMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(curLPRes.ObjectiveMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			break
		}
		newCuts := generateCutRound(lpSolver, curLPRes.Primal, maxCutsPerRound)
		if len(newCuts) == 0 || b.timeExpired() {
			break
		}
		if !b.capLPIter(lpSolver, b.defaultLPIter) {
			b.hitIterLimit = true
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
		b.totalLPIters += nextRes.Iterations
		if nextRes.Status != StatusOptimal {
			// Valid cuts never make a feasible LP infeasible, and a failed re-solve
			// leaves no usable basis. Roll the round back so branch-and-bound starts
			// from the last optimal LP state.
			lpSolver.restore(snap)
			switch {
			case nextRes.Status == StatusTimeLimit:
				b.hitTimeLimit = true
			case nextRes.Status == StatusIterationLimit && b.budgetBound(lpSolver):
				b.hitIterLimit = true
			}
			break
		}
		b.cutsAdded += len(newCuts)
		curLPRes = nextRes

		if isIntegerFeasible(p, curLPRes.Primal) {
			if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
				if objMin := minObjective(p, cand); objMin < b.bestObjMin {
					b.bestObjMin = objMin
					b.bestPrimal = cand
				}
				if b.bestPrimal != nil && isGapClosed(curLPRes.ObjectiveMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
					anchorSnap = lpSolver.snapshot()
					anchorLPRes = curLPRes
					anchorCuts = b.cutsAdded
					break
				}
			}
		}

		zeroGainTol := max(zeroGainAbsTol, zeroGainRelTol*math.Abs(prevObj))
		if curLPRes.ObjectiveMin-prevObj > zeroGainTol {
			anchorSnap = lpSolver.snapshot()
			anchorLPRes = curLPRes
			anchorCuts = b.cutsAdded
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
	if curLPRes.ObjectiveMin-anchorLPRes.ObjectiveMin <= max(zeroGainAbsTol, zeroGainRelTol*math.Abs(anchorLPRes.ObjectiveMin)) && b.cutsAdded > anchorCuts {
		lpSolver.restore(anchorSnap)
		curLPRes = anchorLPRes
		b.cutsAdded = anchorCuts
	}
	if len(lpSolver.extraRows) > 0 {
		b.cutsAdded -= lpSolver.PurgeSlackCuts(slackCutPurgeTol)
	}
	return curLPRes
}

func (b *babEngine) runPostCutHeuristics(cutPrimal []float64) {
	p := b.p
	cutsImproved := b.cutsAdded > 0 && b.rootCutMin-b.rootLPMin > max(zeroGainAbsTol, zeroGainRelTol*math.Abs(b.rootLPMin))
	if !b.opts.EnableHeuristics || !cutsImproved || b.timeExpired() || isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		return
	}
	hs := b.ensureHeurSolver()
	remNodes := b.nodesLeft()
	remIter := b.iterLeft()
	if remNodes > 0 && remIter > 0 {
		sol, objMin, usedN, usedIt, ok := tryRootSubMIP(p, hs, cutPrimal, p.varLB, p.varUB, b.bestObjMin, b.deadline, b.opts.isSubMIP, remNodes, remIter)
		b.nodesExplored += usedN
		b.totalLPIters += usedIt
		if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
			b.hitIterLimit = true
		}
		if ok && objMin < b.bestObjMin {
			b.bestObjMin = objMin
			b.bestPrimal = sol
			b.postCutSubMIPFound = true
		}
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		if sol, objMin, ok := tryPropagatedRounding(p, cutPrimal, p.varLB, p.varUB, b.heurSolver); ok && objMin < b.bestObjMin {
			b.bestObjMin = objMin
			b.bestPrimal = sol
		}
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		if sol, objMin, ok := tryFractionalDiving(p, cutPrimal, p.varLB, p.varUB, b.heurSolver, b.bestObjMin, p.numVars > postCutDiveVarMin); ok && objMin < b.bestObjMin {
			b.bestObjMin = objMin
			b.bestPrimal = sol
		}
	}
}

func nodeHeurStride(n, heurMisses int) int {
	stride := heurBaseStride
	if n > largeModelHeurVarCap || heurMisses >= heurMedMissCap {
		stride = heurMedStride
	}
	if n <= largeModelHeurVarCap && heurMisses >= heurSlowMissCap {
		stride = heurSlowStride
	}
	return stride
}

// exploreTree runs best-bound search interleaved with child plunging, returning early only if
// the root relaxation after cuts rounds directly to a feasible optimum.
func (b *babEngine) exploreTree(curLPRes LPResult) (Result, bool) {
	p := b.p
	n := p.numVars
	pCosts := make([]pseudoCost, n)
	for _, j := range p.intCols {
		objMag := max(minPseudoCostInit, math.Abs(p.objMin[j])*pseudoCostObjScale)
		pCosts[j] = pseudoCost{downSum: objMag, upSum: objMag}
	}

	b.pq = make(nodeQueue, 0, initNodeQueueCap)
	heap.Init(&b.pq)

	baseLB := slices.Clone(p.varLB)
	baseUB := slices.Clone(p.varUB)
	tightenUnboundedIntCols(p, baseLB, baseUB, b.bestObjMin)
	curLB := slices.Clone(baseLB)
	curUB := slices.Clone(baseUB)

	switch {
	case b.hitTimeLimit || b.timeExpired():
		b.hitTimeLimit = true
		b.unsolvedMinBound = b.rootCutMin
		return Result{}, false
	case b.hitIterLimit || b.iterLeft() <= 0:
		b.hitIterLimit = true
		b.unsolvedMinBound = b.rootCutMin
		return Result{}, false
	case b.opts.MaxNodes > 0 && b.nodesExplored >= b.opts.MaxNodes:
		b.hitNodeLimit = true
		b.unsolvedMinBound = b.rootCutMin
		return Result{}, false
	}

	brVar, brVal := selectBranchVar(p, b.lpSolver, curLPRes, curLB, curUB, pCosts, 0, &b.totalLPIters, b.iterLeft)
	if brVar == -1 {
		if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
			if objMin := minObjective(p, cand); objMin < b.bestObjMin {
				b.bestObjMin = objMin
				b.bestPrimal = cand
			}
			if b.bestPrimal != nil && isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
				obj := p.EvaluateObjective(b.bestPrimal)
				boundMin := min(b.bestObjMin, b.rootCutMin)
				return Result{
					modelID:      p.modelID,
					Status:       StatusOptimal,
					Objective:    obj,
					BestBound:    p.ObjectiveFromMin(boundMin),
					RootLPBound:  b.rootLPUser,
					RootCutBound: b.rootCutUser,
					Values:       b.bestPrimal,
					Nodes:        b.nodesExplored,
					Iterations:   b.totalLPIters,
					CutsAdded:    b.cutsAdded,
					SolveTime:    time.Since(b.startTime),
				}, true
			}
		}
		brVar, brVal = selectNearIntegralBranchVar(p, curLPRes.Primal, curLB, curUB)
	}
	if brVar == -1 {
		b.unsolvedMinBound = b.rootCutMin
		b.hitLPLimitStatus = StatusNumericalError
		return Result{}, false
	}

	nodeCounter := 1
	downNode, upNode := createChildNodes(b.rootCutMin, 1, nil, brVar, brVal, curLB[brVar], curUB[brVar], pCosts[brVar], &nodeCounter)
	heap.Push(&b.pq, downNode)
	heap.Push(&b.pq, upNode)

	lastIncumbentNode := 1
	heurMisses := 0
	// Cap per-node pruning relative tolerance at 1.0 so upper - r*max(1,|upper|) remains
	// monotonically non-decreasing when bestObjMin improves; when RelGapTol > 1.0, check the
	// global bound against the full RelGapTol directly.
	pruneRelTol := min(b.opts.RelGapTol, 1.0)
	for b.pq.Len() > 0 || b.plungeNode != nil {
		if b.opts.RelGapTol > 1.0 && b.bestPrimal != nil && isGapClosed(b.currentGlobalBound(), b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			break
		}
		if b.opts.isSubMIP && b.bestPrimal != nil && b.nodesExplored-lastIncumbentNode >= subMIPStagnantNodeCap {
			b.hitNodeLimit = true
			break
		}
		if b.opts.MaxNodes > 0 && b.nodesExplored >= b.opts.MaxNodes {
			b.hitNodeLimit = true
			break
		}
		if b.timeExpired() {
			b.hitTimeLimit = true
			break
		}
		if b.iterLeft() <= 0 {
			b.hitIterLimit = true
			break
		}

		var node *searchNode
		if b.plungeNode != nil && (b.nodesExplored%plungeQueuePeriod != 0) {
			node = b.plungeNode
			b.plungeNode = nil
		} else {
			if b.plungeNode != nil {
				heap.Push(&b.pq, b.plungeNode)
				b.plungeNode = nil
			}
			node = heap.Pop(&b.pq).(*searchNode)
		}

		if node.lowerBound >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(node.lowerBound, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
			b.prunedMinBound = min(b.prunedMinBound, node.lowerBound)
			continue
		}

		b.nodesExplored++

		copy(curLB, baseLB)
		copy(curUB, baseUB)
		applyNodeBounds(node, curLB, curUB)

		if b.opts.EnablePresolve && !propagateBounds(p, curLB, curUB, nodePropPasses) {
			continue
		}

		nodeRes, stop := solveNodeLP(b.lpSolver, curLB, curUB, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters)
		if stop != StatusNotSolved {
			// The node was not evaluated: keep it open so its bound stays in the global bound.
			heap.Push(&b.pq, node)
			if stop == StatusTimeLimit {
				b.hitTimeLimit = true
			} else {
				b.hitIterLimit = true
			}
			break
		}
		if nodeRes.Status == StatusInfeasible {
			continue
		}
		if nodeRes.Status != StatusOptimal {
			b.unsolvedMinBound = min(b.unsolvedMinBound, node.lowerBound)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = nodeRes.Status
			}
			continue
		}
		if nodeRes.ObjectiveMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
			b.prunedMinBound = min(b.prunedMinBound, nodeRes.ObjectiveMin)
			continue
		}

		if node.hasDelta {
			gain := max(0.0, nodeRes.ObjectiveMin-node.lowerBound)
			pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		if isIntegerFeasible(p, nodeRes.Primal) {
			if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
				if objMin := minObjective(p, cand); objMin < b.bestObjMin {
					b.bestObjMin = objMin
					b.bestPrimal = cand
					lastIncumbentNode = b.nodesExplored
					heurMisses = 0
				}
				if nodeRes.ObjectiveMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
					b.prunedMinBound = min(b.prunedMinBound, nodeRes.ObjectiveMin)
					continue
				}
			}
		}

		stride := nodeHeurStride(n, heurMisses)
		runHeur := (!b.postCutSubMIPFound && node.depth <= heurShallowDepth && (n <= largeModelHeurVarCap || node.depth <= 1)) || b.nodesExplored%stride == 0
		if b.opts.EnableHeuristics && !b.timeExpired() && runHeur {
			if sol, objMin, ok := tryPropagatedRounding(p, nodeRes.Primal, curLB, curUB, b.heurSolver); ok && objMin < b.bestObjMin {
				b.bestObjMin = objMin
				b.bestPrimal = sol
				lastIncumbentNode = b.nodesExplored
				heurMisses = 0
				if nodeRes.ObjectiveMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeRes.ObjectiveMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
					b.prunedMinBound = min(b.prunedMinBound, nodeRes.ObjectiveMin)
					continue
				}
			} else {
				heurMisses++
			}
		}

		bVar, bVal := selectBranchVar(p, b.lpSolver, nodeRes, curLB, curUB, pCosts, node.depth, &b.totalLPIters, b.iterLeft)
		if bVar == -1 {
			// The LP point is integral within intFeasTol, yet its rounding violates the
			// rows (typical with big-M constraints). Branch on the integer variable furthest
			// from an integer so the subtree is explored instead of discarded.
			bVar, bVal = selectNearIntegralBranchVar(p, nodeRes.Primal, curLB, curUB)
		}
		if bVar == -1 {
			// Nothing can be branched on: keep the node's bound so the final status
			// never claims an optimal proof over an unexplored region.
			b.unsolvedMinBound = min(b.unsolvedMinBound, nodeRes.ObjectiveMin)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = StatusNumericalError
			}
			continue
		}

		cDown, cUp := createChildNodes(
			nodeRes.ObjectiveMin,
			node.depth+1,
			node,
			bVar,
			bVal,
			curLB[bVar],
			curUB[bVar],
			pCosts[bVar],
			&nodeCounter,
		)

		if bVal-math.Floor(bVal) >= halfFracTarget {
			b.plungeNode = cUp
			heap.Push(&b.pq, cDown)
		} else {
			b.plungeNode = cDown
			heap.Push(&b.pq, cUp)
		}
	}
	return Result{}, false
}

func (b *babEngine) limitStatus() Status {
	switch {
	case b.hitTimeLimit:
		return StatusTimeLimit
	case b.hitIterLimit:
		return StatusIterationLimit
	case b.hitNodeLimit || b.pq.Len() > 0 || b.plungeNode != nil:
		return StatusNodeLimit
	case b.hitLPLimitStatus != StatusNotSolved:
		return b.hitLPLimitStatus
	}
	return StatusOptimal
}

func (b *babEngine) currentGlobalBound() float64 {
	bestBoundMin := min(b.bestObjMin, min(b.unsolvedMinBound, b.prunedMinBound))
	for _, nd := range b.pq {
		if nd.lowerBound < bestBoundMin {
			bestBoundMin = nd.lowerBound
		}
	}
	if b.plungeNode != nil && b.plungeNode.lowerBound < bestBoundMin {
		bestBoundMin = b.plungeNode.lowerBound
	}
	return bestBoundMin
}

func (b *babEngine) finalizeResult() Result {
	p := b.p
	bestBoundMin := b.currentGlobalBound()

	if b.bestPrimal == nil {
		status := b.limitStatus()
		bestBoundUser := 0.0
		if status == StatusOptimal {
			status = StatusInfeasible
		} else {
			if math.IsInf(bestBoundMin, 1) {
				bestBoundMin = b.rootCutMin
			}
			bestBoundUser = p.ObjectiveFromMin(bestBoundMin)
		}
		return Result{
			modelID:      p.modelID,
			Status:       status,
			BestBound:    bestBoundUser,
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootCutUser,
			Nodes:        b.nodesExplored,
			Iterations:   b.totalLPIters,
			CutsAdded:    b.cutsAdded,
			SolveTime:    time.Since(b.startTime),
		}
	}

	status := StatusOptimal
	if !isGapClosed(bestBoundMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		status = b.limitStatus()
		if status == StatusOptimal {
			// Unreachable in exact bookkeeping: an open gap implies an open or unsolved node.
			status = StatusUndefined
		}
	}

	return Result{
		modelID:      p.modelID,
		Status:       status,
		Objective:    p.EvaluateObjective(b.bestPrimal),
		BestBound:    p.ObjectiveFromMin(bestBoundMin),
		RootLPBound:  b.rootLPUser,
		RootCutBound: b.rootCutUser,
		Values:       b.bestPrimal,
		Nodes:        b.nodesExplored,
		Iterations:   b.totalLPIters,
		CutsAdded:    b.cutsAdded,
		SolveTime:    time.Since(b.startTime),
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

func applyNodeBounds(node *searchNode, curLB, curUB []float64) {
	for cur := node; cur != nil; cur = cur.parent {
		if cur.hasDelta {
			d := cur.delta
			curLB[d.col] = max(curLB[d.col], d.lb)
			curUB[d.col] = min(curUB[d.col], d.ub)
		}
		for _, d := range cur.base {
			curLB[d.col] = max(curLB[d.col], d.lb)
			curUB[d.col] = min(curUB[d.col], d.ub)
		}
	}
}

func compactAncestorDeltas(node *searchNode) []boundDelta {
	merged := make(map[int]boundDelta, nodeCompactStepCap)
	for cur := node; cur != nil; cur = cur.parent {
		if cur.hasDelta {
			d := cur.delta
			if ex, ok := merged[d.col]; ok {
				ex.lb = max(ex.lb, d.lb)
				ex.ub = min(ex.ub, d.ub)
				merged[d.col] = ex
			} else {
				merged[d.col] = d
			}
		}
		for _, d := range cur.base {
			if ex, ok := merged[d.col]; ok {
				ex.lb = max(ex.lb, d.lb)
				ex.ub = min(ex.ub, d.ub)
				merged[d.col] = ex
			} else {
				merged[d.col] = d
			}
		}
	}
	out := make([]boundDelta, 0, len(merged))
	for _, d := range merged {
		out = append(out, d)
	}
	return out
}

func createChildNodes(
	parentLB float64,
	depth int,
	parent *searchNode,
	brVar int,
	brVal float64,
	curLB, curUB float64,
	pc pseudoCost,
	counter *int,
) (downNode, upNode *searchNode) {
	floorVal := math.Floor(brVal)
	ceilVal := math.Ceil(brVal)
	if !math.IsInf(curUB, 1) && floorVal >= curUB {
		floorVal = curUB - 1.0
	}
	if !math.IsInf(curLB, -1) && ceilVal <= curLB {
		ceilVal = curLB + 1.0
	}
	if ceilVal <= floorVal {
		ceilVal = floorVal + 1.0
	}
	fDown := min(max(brVal-floorVal, 0.0), 1.0)
	fUp := min(max(ceilVal-brVal, 0.0), 1.0)

	var base []boundDelta
	nextParent := parent
	var steps uint16 = 1
	if parent != nil {
		if parent.steps >= nodeCompactStepCap {
			base = compactAncestorDeltas(parent)
			nextParent = nil
		} else {
			steps = parent.steps + 1
		}
	}

	*counter++
	downID := *counter
	*counter++
	upID := *counter

	downNode = &searchNode{
		parent:     nextParent,
		base:       base,
		delta:      boundDelta{col: brVar, lb: curLB, ub: floorVal},
		lowerBound: parentLB,
		estimate:   parentLB + fDown*pc.avgDown(),
		brDist:     fDown,
		depth:      depth,
		id:         downID,
		steps:      steps,
		hasDelta:   true,
		up:         false,
	}
	upNode = &searchNode{
		parent:     nextParent,
		base:       base,
		delta:      boundDelta{col: brVar, lb: ceilVal, ub: curUB},
		lowerBound: parentLB,
		estimate:   parentLB + fUp*pc.avgUp(),
		brDist:     fUp,
		depth:      depth,
		id:         upID,
		steps:      steps,
		hasDelta:   true,
		up:         true,
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

	for _, col := range p.intCols {
		if curUB[col]-curLB[col] < halfFracTarget {
			continue
		}
		v := min(max(lpRes.Primal[col], curLB[col]), curUB[col])
		f := v - math.Floor(v)
		if f <= intFeasTol || f >= 1.0-intFeasTol {
			continue
		}
		if math.Floor(v) >= curUB[col] || math.Ceil(v) <= curLB[col] {
			continue
		}
		pc := pCosts[col]
		dEst := f * pc.avgDown()
		uEst := (1.0 - f) * pc.avgUp()
		score := max(dEst, minPseudoScoreFloor) * max(uEst, minPseudoScoreFloor)
		score += pseudoTieScale * (halfFracTarget - math.Abs(f-halfFracTarget)) * (1.0 + math.Abs(p.objMin[col]))
		cands = append(cands, cand{col: col, val: v, dEst: dEst, uEst: uEst, score: score})
	}

	if len(cands) == 0 {
		return -1, 0
	}

	if depth <= strongBranchMaxDepth && len(cands) > 1 {
		slices.SortFunc(cands, func(a, b cand) int {
			if c := cmp.Compare(b.score, a.score); c != 0 {
				return c
			}
			return cmp.Compare(a.col, b.col)
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
			if pCosts[col].downCount >= strongBranchReliableCnt && pCosts[col].upCount >= strongBranchReliableCnt {
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
	best, bestDist, bestVal := -1, 0.0, 0.0
	fallbackCol, fallbackVal := -1, 0.0
	for _, col := range p.intCols {
		lb, ub := curLB[col], curUB[col]
		if ub-lb < halfFracTarget {
			continue
		}
		v := min(max(x[col], lb), ub)
		if math.Floor(v) < ub && math.Ceil(v) > lb {
			if d := math.Abs(v - math.Round(v)); d > bestDist {
				best, bestDist, bestVal = col, d, v
			}
		}
		if fallbackCol == -1 && !math.IsNaN(v) {
			fallbackCol = col
			switch {
			case !math.IsInf(lb, -1) && v <= lb:
				fallbackVal = lb + halfFracTarget
			case !math.IsInf(ub, 1) && v >= ub:
				fallbackVal = ub - halfFracTarget
			default:
				fallbackVal = math.Floor(v) + halfFracTarget
			}
		}
	}
	if best != -1 {
		return best, bestVal
	}
	if fallbackCol != -1 {
		return fallbackCol, fallbackVal
	}
	return -1, 0
}

// polishIncumbent snaps near-integral integer components of x, re-optimizes continuous
// variables when needed to eliminate snapping residuals, and verifies all bounds and rows within incumbentTol.
func (b *babEngine) polishIncumbent(x []float64) ([]float64, bool) {
	p := b.p
	cand := slices.Clone(x)
	for _, col := range p.intCols {
		v := cand[col]
		rv := math.Round(v)
		if math.Abs(v-rv) <= intSnapTol {
			cand[col] = rv
		}
	}
	bV, rV, iV := p.MaxViolation(cand, true)
	if iV > incumbentTol {
		return nil, false
	}
	if len(p.intCols) < p.numVars && !b.timeExpired() && (bV > primalTol || rV > primalTol) {
		n := p.numVars
		if len(b.fixScratchLB) < n {
			b.fixScratchLB = make([]float64, n)
			b.fixScratchUB = make([]float64, n)
		}
		fixLB := b.fixScratchLB[:n]
		fixUB := b.fixScratchUB[:n]
		copy(fixLB, p.varLB)
		copy(fixUB, p.varUB)
		validFix := true
		for _, col := range p.intCols {
			iv := cand[col]
			if iv < p.varLB[col]-incumbentTol || iv > p.varUB[col]+incumbentTol {
				validFix = false
				break
			}
			iv = min(max(iv, p.varLB[col]), p.varUB[col])
			fixLB[col] = iv
			fixUB[col] = iv
		}
		if validFix {
			hs := b.ensureHeurSolver()
			hs.SetStructuralBounds(fixLB, fixUB)
			polRes := hs.Solve()
			if polRes.Status == StatusOptimal && len(polRes.Primal) == n {
				copy(cand, polRes.Primal)
				snapToIntegers(p, cand)
				bV, rV, iV = p.MaxViolation(cand, true)
			}
		}
	}
	return cand, bV <= incumbentTol && rV <= incumbentTol && iV <= incumbentTol
}

// minObjective computes sum_j ObjMin[j]*x[j].
func minObjective(p *CompiledProblem, x []float64) float64 {
	return evalLinearSum(0, p.objMin, x, p.numVars)
}

func isIntegerFeasible(p *CompiledProblem, x []float64) bool {
	for _, col := range p.intCols {
		v := x[col]
		if math.Abs(v-math.Round(v)) > intFeasTol {
			return false
		}
	}
	return true
}

func snapToIntegers(p *CompiledProblem, x []float64) {
	for _, col := range p.intCols {
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
	for _, col := range p.intCols {
		if math.IsInf(baseLB[col], -1) || math.IsInf(baseUB[col], 1) {
			hasInfInt = true
			break
		}
	}
	if !hasInfInt {
		return
	}
	n := p.numVars
	minObj := 0.0
	for j := range n {
		c := p.objMin[j]
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
	if slack < -intFeasTol {
		return
	}
	slack = max(0.0, slack)
	tlb := slices.Clone(baseLB)
	tub := slices.Clone(baseUB)
	for j := range n {
		c := p.objMin[j]
		if c > heurCoeffZeroTol {
			u := tlb[j] + slack/c
			if p.varCat[j] != Continuous {
				u = math.Floor(u + heurIntBoundTol)
			}
			if u < tub[j] {
				tub[j] = max(tlb[j], u)
			}
		} else if c < -heurCoeffZeroTol {
			l := tub[j] + slack/c
			if p.varCat[j] != Continuous {
				l = math.Ceil(l - heurIntBoundTol)
			}
			if l > tlb[j] {
				tlb[j] = min(tub[j], l)
			}
		}
	}
	if !propagateBounds(p, tlb, tub, tightenObjPasses) {
		return
	}
	for _, col := range p.intCols {
		if math.IsInf(baseUB[col], 1) && !math.IsInf(tub[col], 1) {
			baseUB[col] = tub[col]
		}
		if math.IsInf(baseLB[col], -1) && !math.IsInf(tlb[col], -1) {
			baseLB[col] = tlb[col]
		}
	}
}
