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
	incumbentTol             = 1e-6
	nodeQueueCmpTol          = 1e-7
	maxRootCutsCap           = 60
	minRootCutsFloor         = 15
	rootCutsRowDiv           = 3
	minCutRoundImprov        = 1e-3
	maxStagnantCutRound      = 2
	cutDimReturnMinExtraRows = 64
	cutDimReturnImprov       = 1.5e-3
	cutDimReturnMaxRounds    = 2
	zeroGainAbsTol           = 1e-6
	zeroGainRelTol           = 1e-7
	slackCutPurgeTol         = 0.05
	inTreeCutSlackTol        = 0.1
	inTreeCutPurgePeriod     = 64
	inTreeCutPurgeMinCuts    = 32
	inTreeCutMinAge          = 16
	inTreeCutMinAgedPurge    = 8
	minPseudoCostInit        = 1e-2
	pseudoCostObjScale       = 0.1
	minPseudoDist            = 1e-3
	initNodeQueueCap         = 256
	minPseudoScoreFloor      = 1e-4
	pseudoTieScale           = 1e-6
	strongBranchMaxDepth     = 2
	strongBranchReliableCnt  = 2
	strongBranchCandCap      = 5
	strongBranchInfeas       = 1e4
	strongBranchMaxIter      = 25
	maxSBDomainRedRounds     = 4
	intSnapTol               = 1e-4
	postCutDiveVarMin        = 100
	subMIPStagnantNodeCap    = 48
	subMIPNoIncumbentNodeCap = 64
	plungeQueuePeriod        = 8
	largeModelHeurVarCap     = 200
	heurShallowDepth         = 5
	heurBaseStride           = 4
	heurMedStride            = 16
	heurSlowStride           = 64
	heurMedMissCap           = 4
	heurSlowMissCap          = 12
	tightenObjPasses         = 3
	rootPropPasses           = 8
	nodeCompactStepCap       = 128
	rootIterExpandMult       = 4
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

type branchCand struct {
	col        int
	val        float64
	dEst, uEst float64
	score      float64
}

type sbOutcome struct {
	bestCol     int
	bestVal     float64
	pruned      bool
	fixedDeltas []boundDelta
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
	isContLinked       []bool
	isContLinkedReady  bool
	sbCands            []branchCand
	cutWS              cutGenWorkspace
	cutAges            []int
	nodesSincePurge    int
	propQueue          []int
	propInQueue        []bool
	touchedCols        []int
}

func (b *babEngine) contLinkedIntCols() []bool {
	if !b.isContLinkedReady {
		b.isContLinked = findContLinkedIntCols(b.p)
		b.isContLinkedReady = true
	}
	return b.isContLinked
}

func (b *babEngine) ensurePropBuffers() {
	m := b.p.numConstraints
	if len(b.propInQueue) < m {
		b.propInQueue = make([]bool, m)
	}
	if cap(b.propQueue) < min(rowQueueCapMult*m, initNodeQueueCap) {
		b.propQueue = make([]int, 0, min(rowQueueCapMult*m, initNodeQueueCap))
	}
}

func (b *babEngine) cutsEnabled() bool {
	return b.opts.EnableCuts && b.opts.MaxCutPasses > 0
}

func (b *babEngine) ensureHeurSolver() *LPSolver {
	if b.heurSolver == nil {
		b.heurSolver = NewLPSolver(b.p, nil)
		b.heurSolver.Deadline = b.deadline
		b.heurSolver.Done = b.done
		b.heurSolver.DisableSIMD = b.opts.DisableSIMD
		b.heurSolver.SkipExactFarkas = true
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
		if allExactInt && g > 0 && math.Abs(p.rowRHS[i]) <= 0x1p62 {
			loInt := int64(math.Ceil(p.rowRHS[i] - presolveFeasTol))
			hiInt := int64(math.Floor(p.rowRHS[i] + presolveFeasTol))
			if q := divCeilInt64(loInt, g); q*g > hiInt {
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

	rootMin := lpMinWithConst(p, rootRes)
	if math.IsNaN(rootRes.ObjectiveMin) || math.IsInf(rootRes.ObjectiveMin, 0) || math.IsNaN(rootMin) || math.IsInf(rootMin, 0) {
		return Result{
			modelID:    p.modelID,
			Status:     StatusNumericalError,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}
	b.rootLPMin = rootMin
	b.rootLPUser = p.userObjFromShiftedMin(b.rootLPMin)

	if !p.isMIP || len(p.intCols) == 0 {
		return b.optimalOrNumErr(Result{
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
		})
	}

	if opts.hasCutoff {
		b.bestObjMin = opts.cutoffMin
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
			b.acceptIncumbent(cand, minObjective(p, cand))
			if b.bestPrimal != nil && isGapClosed(b.rootLPMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
				obj := p.EvaluateObjective(b.bestPrimal)
				boundMin := min(b.bestObjMin, b.rootLPMin)
				return b.optimalOrNumErr(Result{
					modelID:      p.modelID,
					Status:       StatusOptimal,
					Objective:    obj,
					BestBound:    p.userObjFromShiftedMin(boundMin),
					RootLPBound:  b.rootLPUser,
					RootCutBound: b.rootLPUser,
					Values:       b.bestPrimal,
					Nodes:        1,
					Iterations:   b.totalLPIters,
					SolveTime:    time.Since(startTime),
				})
			}
		}
	}

	if opts.EnableHeuristics {
		b.ensureHeurSolver()
		b.runPreCutHeuristics(rootRes.Primal)
	}

	if b.bestPrimal != nil && isGapClosed(b.rootLPMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		boundMin := min(b.bestObjMin, b.rootLPMin)
		return b.optimalOrNumErr(Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(b.bestPrimal),
			BestBound:    p.userObjFromShiftedMin(boundMin),
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootLPUser,
			Values:       b.bestPrimal,
			Nodes:        b.nodesExplored,
			Iterations:   b.totalLPIters,
			SolveTime:    time.Since(startTime),
		})
	}

	curLPRes := b.runRootCutLoop(rootRes)
	b.rootCutMin = lpMinWithConst(p, curLPRes)
	b.rootCutUser = p.userObjFromShiftedMin(b.rootCutMin)

	b.runPostCutHeuristics(curLPRes.Primal)
	if b.bestPrimal != nil && isGapClosed(b.rootCutMin, b.bestObjMin, opts.RelGapTol, opts.AbsGapTol) {
		boundMin := min(b.bestObjMin, b.rootCutMin)
		return b.optimalOrNumErr(Result{
			modelID:      p.modelID,
			Status:       StatusOptimal,
			Objective:    p.EvaluateObjective(b.bestPrimal),
			BestBound:    p.userObjFromShiftedMin(boundMin),
			RootLPBound:  b.rootLPUser,
			RootCutBound: b.rootCutUser,
			Values:       b.bestPrimal,
			Nodes:        b.nodesExplored,
			Iterations:   b.totalLPIters,
			CutsAdded:    b.cutsAdded,
			SolveTime:    time.Since(startTime),
		})
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
	feasProb.lazy = nil
	feasProb.objCoeffs = make([]float64, p.numVars)
	feasProb.objMin = make([]float64, p.numVars)
	feasProb.objConstant = 0
	feasProb.objConstMinVal = 0
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

func (b *babEngine) runRootSubMIP(primal []float64, hs *LPSolver, isContLinked []bool) bool {
	remNodes := b.nodesLeft()
	remIter := b.iterLeft()
	if remNodes <= 0 || remIter <= 0 {
		return false
	}
	var rootStatus []varBasisStatus
	if b.lpSolver != nil && b.lpSolver.ws != nil && len(b.lpSolver.ws.status) >= b.p.numVars {
		rootStatus = b.lpSolver.ws.status[:b.p.numVars]
	}
	sol, objMin, usedN, usedIt, ok := tryRootSubMIP(b.p, hs, rootStatus, primal, b.p.varLB, b.p.varUB, b.bestObjMin, b.deadline, b.opts.isSubMIP, remNodes, remIter, isContLinked)
	b.nodesExplored += usedN
	b.totalLPIters += usedIt
	if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
		b.hitIterLimit = true
	}
	return ok && b.acceptIncumbent(sol, objMin)
}

func (b *babEngine) runPreCutHeuristics(rootPrimal []float64) {
	p := b.p
	isContLinked := b.contLinkedIntCols()
	b.heurSolver.CopyRootBasisFrom(b.lpSolver)
	if !b.timeExpired() {
		if sol, objMin, ok := tryPropagatedRounding(p, rootPrimal, p.varLB, p.varUB, b.heurSolver, isContLinked); ok {
			b.acceptIncumbent(sol, objMin)
		}
	}
	// Defer fractional diving and root sub-MIP until after root cuts when cutting is enabled.
	if b.cutsEnabled() {
		return
	}
	if !b.timeExpired() && !isGapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		if sol, objMin, ok := tryFractionalDiving(p, rootPrimal, p.varLB, p.varUB, b.heurSolver, b.bestObjMin, true, isContLinked); ok {
			b.acceptIncumbent(sol, objMin)
		}
	}
	if !b.timeExpired() && !isGapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		b.runRootSubMIP(rootPrimal, b.heurSolver, isContLinked)
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
	dimReturnRounds := 0
	maxCutsPerRound := min(maxRootCutsCap, max(minRootCutsFloor, p.numConstraints/rootCutsRowDiv))
	largePoolThresh := max(cutDimReturnMinExtraRows, p.numConstraints)
	var anchorSnap lpSnapshot
	anchorLPRes := curLPRes
	anchorCuts := 0

	for range b.opts.MaxCutPasses {
		if b.timeExpired() {
			break
		}
		curMin := lpMinWithConst(p, curLPRes)
		if curMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(curMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			break
		}
		newCuts := generateCutRound(lpSolver, curLPRes.Primal, maxCutsPerRound, &b.cutWS)
		if len(newCuts) == 0 || b.timeExpired() {
			break
		}
		if !b.capLPIter(lpSolver, b.defaultLPIter) {
			b.hitIterLimit = true
			break
		}
		prevObj := curMin
		// Take one snapshot per round. Share immutable slices when still at anchor.
		curSnap := lpSolver.snapshot()
		if b.cutsAdded == anchorCuts {
			anchorSnap = curSnap
		}
		rows := make([]dynamicRow, len(newCuts))
		for k, c := range newCuts {
			rows[k] = lpSolver.newCutRow(c.ColIdx, c.Val, c.RHS)
		}
		lpSolver.addCutRows(rows)
		nextRes := lpSolver.Solve()
		b.totalLPIters += nextRes.Iterations
		nextMin := lpMinWithConst(p, nextRes)
		if nextRes.Status != StatusOptimal || math.IsNaN(nextMin) || math.IsInf(nextMin, 0) {
			// Valid cuts never make a feasible LP infeasible, and a failed re-solve
			// leaves no usable basis. Roll the round back so branch-and-bound starts
			// from the last optimal LP state.
			lpSolver.restore(curSnap)
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
				b.acceptIncumbent(cand, minObjective(p, cand))
				if b.bestPrimal != nil && isGapClosed(nextMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
					anchorLPRes = curLPRes
					anchorCuts = b.cutsAdded
					break
				}
			}
		}

		zeroGainTol := max(zeroGainAbsTol, zeroGainRelTol*math.Abs(prevObj))
		if nextMin-prevObj > zeroGainTol {
			anchorLPRes = curLPRes
			anchorCuts = b.cutsAdded
		}

		improvement := (nextMin - prevObj) / max(1.0, math.Abs(prevObj))
		if improvement < minCutRoundImprov {
			stagnantRounds++
			if stagnantRounds >= maxStagnantCutRound {
				break
			}
		} else {
			stagnantRounds = 0
		}

		// Stop early on basis bloat. Large cut pools need meaningful bound progress.
		if len(lpSolver.extraRows) >= largePoolThresh {
			relBoundImprov := (nextMin - prevObj) / max(1.0, math.Abs(nextMin))
			if relBoundImprov < cutDimReturnImprov {
				dimReturnRounds++
				if dimReturnRounds >= cutDimReturnMaxRounds {
					break
				}
			} else {
				dimReturnRounds = 0
			}
		}
	}
	curEndMin := lpMinWithConst(p, curLPRes)
	anchorEndMin := lpMinWithConst(p, anchorLPRes)
	if curEndMin-anchorEndMin <= max(zeroGainAbsTol, zeroGainRelTol*math.Abs(anchorEndMin)) && b.cutsAdded > anchorCuts {
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
	if !b.opts.EnableHeuristics || !b.cutsEnabled() || b.timeExpired() || isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		return
	}
	cutsImproved := b.cutsAdded > 0 && b.rootCutMin-b.rootLPMin > max(zeroGainAbsTol, zeroGainRelTol*math.Abs(b.rootLPMin))
	hs := b.ensureHeurSolver()
	isContLinked := b.contLinkedIntCols()
	if !cutsImproved {
		// Cuts did not tighten the root relaxation, so run the deferred pre-cut diving
		// before resorting to a root sub-MIP (preserving root closure on zero-cut models).
		hs.CopyRootBasisFrom(b.lpSolver)
		if sol, objMin, ok := tryFractionalDiving(p, cutPrimal, p.varLB, p.varUB, hs, b.bestObjMin, true, isContLinked); ok {
			b.acceptIncumbent(sol, objMin)
		}
		if !b.timeExpired() && !isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			hs.CopyRootBasisFrom(b.lpSolver)
			b.runRootSubMIP(cutPrimal, hs, isContLinked)
		}
		return
	}
	if b.runRootSubMIP(cutPrimal, hs, isContLinked) {
		b.postCutSubMIPFound = true
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		if sol, objMin, ok := tryPropagatedRounding(p, cutPrimal, p.varLB, p.varUB, hs, isContLinked); ok {
			b.acceptIncumbent(sol, objMin)
		}
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !isGapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
		if sol, objMin, ok := tryFractionalDiving(p, cutPrimal, p.varLB, p.varUB, hs, b.bestObjMin, p.numVars > postCutDiveVarMin, isContLinked); ok {
			b.acceptIncumbent(sol, objMin)
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

// updateCutAgesAndMaybePurge tracks consecutive optimal B&B nodes where each root cut
// remains basic with strictly positive slack, periodically compacting aged-out cuts via
// LPSolver.PurgeSlackCuts when not inside strong branching.
func (b *babEngine) updateCutAgesAndMaybePurge(allowPurge bool) {
	s := b.lpSolver
	numExtra := len(s.extraRows)
	if numExtra == 0 || !s.luValid || s.ws == nil {
		return
	}
	if len(b.cutAges) != numExtra {
		if cap(b.cutAges) >= numExtra {
			b.cutAges = b.cutAges[:numExtra]
			clear(b.cutAges)
		} else {
			b.cutAges = make([]int, numExtra)
		}
	}
	n := b.p.numVars
	baseM := b.p.numConstraints
	m := baseM + numExtra
	ws := s.ws
	agedSlack := 0
	freshSlack := 0
	for e := range numExtra {
		slackVar := n + baseM + e
		isLoose := false
		if ws.status[slackVar] == statusBasic {
			bSlot := ws.basicRow[slackVar]
			if bSlot >= 0 && bSlot < m && ws.basis[bSlot] == slackVar && ws.xb[bSlot] > inTreeCutSlackTol*s.rowScaleOf(baseM+e) {
				isLoose = true
			}
		}
		if isLoose {
			b.cutAges[e]++
			if b.cutAges[e] >= inTreeCutMinAge {
				agedSlack++
			} else {
				freshSlack++
			}
		} else {
			b.cutAges[e] = 0
		}
	}
	b.nodesSincePurge++
	if !allowPurge || numExtra <= inTreeCutPurgeMinCuts || b.nodesSincePurge < inTreeCutPurgePeriod {
		return
	}
	if agedSlack >= max(inTreeCutMinAgedPurge, numExtra/4) && agedSlack >= freshSlack {
		if purged := s.PurgeSlackCuts(inTreeCutSlackTol); purged > 0 {
			b.cutAges = b.cutAges[:len(s.extraRows)]
			clear(b.cutAges)
		}
		b.nodesSincePurge = 0
	}
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

	rootPropConv := false
	if b.opts.EnablePresolve {
		var rootFeas bool
		rootFeas, rootPropConv = propagateBoundsChecked(p, baseLB, baseUB, rootPropPasses)
		if !rootFeas {
			return Result{}, false
		}
		b.ensurePropBuffers()
	}

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

	rootNodeMin := b.rootCutMin
	var brVar int
	var brVal float64
	rootSBRounds := 0
	for {
		sb := selectBranchVarDetailed(
			p, b.lpSolver, curLPRes, curLB, curUB, pCosts, 0,
			&b.totalLPIters, b.iterLeft, &b.sbCands,
			rootSBRounds < maxSBDomainRedRounds,
		)
		if sb.pruned {
			return Result{}, false
		}
		if len(sb.fixedDeltas) == 0 {
			brVar, brVal = sb.bestCol, sb.bestVal
			break
		}
		rootSBRounds++
		b.touchedCols = b.touchedCols[:0]
		for _, d := range sb.fixedDeltas {
			if d.lb > baseLB[d.col] {
				baseLB[d.col] = d.lb
			}
			if d.ub < baseUB[d.col] {
				baseUB[d.col] = d.ub
			}
			curLB[d.col] = baseLB[d.col]
			curUB[d.col] = baseUB[d.col]
			b.touchedCols = append(b.touchedCols, d.col)
		}
		if b.opts.EnablePresolve {
			var propOK bool
			if rootPropConv {
				if len(b.touchedCols) == 1 {
					propOK = propagateFromCol(p, baseLB, baseUB, b.touchedCols[0], &b.propQueue, b.propInQueue)
				} else {
					propOK = propagateFromCols(p, baseLB, baseUB, b.touchedCols, &b.propQueue, b.propInQueue)
				}
			} else {
				propOK = propagateBounds(p, baseLB, baseUB, nodePropPasses)
			}
			if !propOK {
				return Result{}, false
			}
			copy(curLB, baseLB)
			copy(curUB, baseUB)
		}
		reRes, stop := solveNodeLP(b.lpSolver, curLB, curUB, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters)
		if stop != StatusNotSolved {
			if stop == StatusTimeLimit {
				b.hitTimeLimit = true
			} else {
				b.hitIterLimit = true
			}
			b.unsolvedMinBound = rootNodeMin
			return Result{}, false
		}
		if reRes.Status == StatusInfeasible {
			return Result{}, false
		}
		if reRes.Status != StatusOptimal {
			b.unsolvedMinBound = rootNodeMin
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = reRes.Status
			}
			return Result{}, false
		}
		reMin := lpMinWithConst(p, reRes)
		if math.IsNaN(reMin) || math.IsInf(reMin, 0) {
			b.unsolvedMinBound = rootNodeMin
			b.hitLPLimitStatus = StatusNumericalError
			return Result{}, false
		}
		curLPRes = reRes
		rootNodeMin = max(rootNodeMin, reMin)
		if isIntegerFeasible(p, curLPRes.Primal) {
			if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
				b.acceptIncumbent(cand, minObjective(p, cand))
			}
		}
		if b.bestPrimal != nil && isGapClosed(rootNodeMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			boundMin := min(b.bestObjMin, rootNodeMin)
			return b.optimalOrNumErr(Result{
				modelID:      p.modelID,
				Status:       StatusOptimal,
				Objective:    p.EvaluateObjective(b.bestPrimal),
				BestBound:    p.userObjFromShiftedMin(boundMin),
				RootLPBound:  b.rootLPUser,
				RootCutBound: b.rootCutUser,
				Values:       b.bestPrimal,
				Nodes:        b.nodesExplored,
				Iterations:   b.totalLPIters,
				CutsAdded:    b.cutsAdded,
				SolveTime:    time.Since(b.startTime),
			}), true
		}
		if rootNodeMin >= b.bestObjMin-b.opts.AbsGapTol {
			b.prunedMinBound = min(b.prunedMinBound, rootNodeMin)
			return Result{}, false
		}
	}

	if brVar == -1 {
		if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
			b.acceptIncumbent(cand, minObjective(p, cand))
			if b.bestPrimal != nil && isGapClosed(rootNodeMin, b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
				obj := p.EvaluateObjective(b.bestPrimal)
				boundMin := min(b.bestObjMin, rootNodeMin)
				return b.optimalOrNumErr(Result{
					modelID:      p.modelID,
					Status:       StatusOptimal,
					Objective:    obj,
					BestBound:    p.userObjFromShiftedMin(boundMin),
					RootLPBound:  b.rootLPUser,
					RootCutBound: b.rootCutUser,
					Values:       b.bestPrimal,
					Nodes:        b.nodesExplored,
					Iterations:   b.totalLPIters,
					CutsAdded:    b.cutsAdded,
					SolveTime:    time.Since(b.startTime),
				}), true
			}
		}
		brVar, brVal = selectNearIntegralBranchVar(p, curLPRes.Primal, curLB, curUB)
	}
	if brVar == -1 {
		b.unsolvedMinBound = rootNodeMin
		b.hitLPLimitStatus = StatusNumericalError
		return Result{}, false
	}

	nodeCounter := 1
	downNode, upNode := createChildNodes(rootNodeMin, 1, nil, brVar, brVal, curLB[brVar], curUB[brVar], pCosts[brVar], &nodeCounter)
	heap.Push(&b.pq, downNode)
	heap.Push(&b.pq, upNode)

	lastIncumbentNode := 1
	heurMisses := 0
	var isContLinked []bool
	if b.opts.EnableHeuristics {
		isContLinked = b.contLinkedIntCols()
	}
	// Cap per-node pruning relative tolerance at 1.0 so upper - r*max(1,|upper|) remains
	// monotonically non-decreasing when bestObjMin improves; when RelGapTol > 1.0, check the
	// global bound against the full RelGapTol directly.
	pruneRelTol := min(b.opts.RelGapTol, 1.0)
	for b.pq.Len() > 0 || b.plungeNode != nil {
		if b.opts.RelGapTol > 1.0 && b.bestPrimal != nil && isGapClosed(b.currentGlobalBound(), b.bestObjMin, b.opts.RelGapTol, b.opts.AbsGapTol) {
			break
		}
		if b.opts.isSubMIP {
			if b.bestPrimal != nil && b.nodesExplored-lastIncumbentNode >= subMIPStagnantNodeCap {
				b.hitNodeLimit = true
				break
			}
			if b.bestPrimal == nil && b.nodesExplored >= subMIPNoIncumbentNodeCap {
				b.hitNodeLimit = true
				break
			}
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
		fromPlunge := false
		if b.plungeNode != nil && (b.nodesExplored%plungeQueuePeriod != 0) {
			node = b.plungeNode
			b.plungeNode = nil
			fromPlunge = true
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

		if fromPlunge && node.hasDelta {
			// Direct plunge from parent. Apply only the single branching delta.
			d := node.delta
			tightened := false
			if d.lb > curLB[d.col] {
				curLB[d.col] = d.lb
				tightened = true
			}
			if d.ub < curUB[d.col] {
				curUB[d.col] = d.ub
				tightened = true
			}
			if curLB[d.col] > curUB[d.col]+intFeasTol {
				continue
			}
			if b.opts.EnablePresolve && tightened {
				if rootPropConv {
					if !propagateFromCol(p, curLB, curUB, d.col, &b.propQueue, b.propInQueue) {
						continue
					}
				} else if !propagateBounds(p, curLB, curUB, nodePropPasses) {
					continue
				}
			}
		} else {
			copy(curLB, baseLB)
			copy(curUB, baseUB)
			var boundsFeas bool
			b.touchedCols, boundsFeas = applyNodeBoundsTracked(node, curLB, curUB, b.touchedCols[:0])
			if !boundsFeas {
				continue
			}
			if b.opts.EnablePresolve && len(b.touchedCols) > 0 {
				if rootPropConv {
					var propOK bool
					if len(b.touchedCols) == 1 {
						propOK = propagateFromCol(p, curLB, curUB, b.touchedCols[0], &b.propQueue, b.propInQueue)
					} else {
						propOK = propagateFromCols(p, curLB, curUB, b.touchedCols, &b.propQueue, b.propInQueue)
					}
					if !propOK {
						continue
					}
				} else if !propagateBounds(p, curLB, curUB, nodePropPasses) {
					continue
				}
			}
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
		nodeMin := lpMinWithConst(p, nodeRes)
		if math.IsNaN(nodeMin) || math.IsInf(nodeMin, 0) {
			b.unsolvedMinBound = min(b.unsolvedMinBound, node.lowerBound)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = StatusNumericalError
			}
			continue
		}
		if nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
			b.updateCutAgesAndMaybePurge(true)
			b.prunedMinBound = min(b.prunedMinBound, nodeMin)
			continue
		}

		// Age inactive root cuts; only purge when not about to enter strong branching
		// so the live LU factorization remains warm for depth <= strongBranchMaxDepth probes.
		b.updateCutAgesAndMaybePurge(node.depth > strongBranchMaxDepth)

		if node.hasDelta {
			gain := max(0.0, nodeMin-node.lowerBound)
			pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		if isIntegerFeasible(p, nodeRes.Primal) {
			if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
				if b.acceptIncumbent(cand, minObjective(p, cand)) {
					lastIncumbentNode = b.nodesExplored
					heurMisses = 0
				}
				if nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
					b.prunedMinBound = min(b.prunedMinBound, nodeMin)
					continue
				}
			}
		}

		stride := nodeHeurStride(n, heurMisses)
		shallowHeur := !b.postCutSubMIPFound && heurMisses < heurMedMissCap && node.depth <= heurShallowDepth && (n <= largeModelHeurVarCap || node.depth <= 1)
		runHeur := shallowHeur || b.nodesExplored%stride == 0
		if b.opts.EnableHeuristics && !b.timeExpired() && runHeur {
			if sol, objMin, ok := tryPropagatedRounding(p, nodeRes.Primal, curLB, curUB, b.heurSolver, isContLinked); ok && b.acceptIncumbent(sol, objMin) {
				lastIncumbentNode = b.nodesExplored
				heurMisses = 0
				if nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
					b.prunedMinBound = min(b.prunedMinBound, nodeMin)
					continue
				}
			} else {
				heurMisses++
			}
		}

		var bVar int
		var bVal float64
		prunedBySB := false
		stoppedByLimit := false
		sbRounds := 0
		for {
			sb := selectBranchVarDetailed(
				p, b.lpSolver, nodeRes, curLB, curUB, pCosts, node.depth,
				&b.totalLPIters, b.iterLeft, &b.sbCands,
				sbRounds < maxSBDomainRedRounds,
			)
			if sb.pruned {
				prunedBySB = true
				break
			}
			if len(sb.fixedDeltas) == 0 {
				bVar, bVal = sb.bestCol, sb.bestVal
				break
			}
			sbRounds++
			node.base = append(slices.Clip(node.base), sb.fixedDeltas...)
			if b.opts.EnablePresolve {
				b.touchedCols = b.touchedCols[:0]
				for _, d := range sb.fixedDeltas {
					b.touchedCols = append(b.touchedCols, d.col)
				}
				var propOK bool
				if rootPropConv {
					if len(b.touchedCols) == 1 {
						propOK = propagateFromCol(p, curLB, curUB, b.touchedCols[0], &b.propQueue, b.propInQueue)
					} else {
						propOK = propagateFromCols(p, curLB, curUB, b.touchedCols, &b.propQueue, b.propInQueue)
					}
				} else {
					propOK = propagateBounds(p, curLB, curUB, nodePropPasses)
				}
				if !propOK {
					prunedBySB = true
					break
				}
			}
			reRes, stop := solveNodeLP(b.lpSolver, curLB, curUB, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters)
			if stop != StatusNotSolved {
				node.lowerBound = max(node.lowerBound, nodeMin)
				heap.Push(&b.pq, node)
				if stop == StatusTimeLimit {
					b.hitTimeLimit = true
				} else {
					b.hitIterLimit = true
				}
				stoppedByLimit = true
				break
			}
			if reRes.Status == StatusInfeasible {
				prunedBySB = true
				break
			}
			if reRes.Status != StatusOptimal {
				b.unsolvedMinBound = min(b.unsolvedMinBound, nodeMin)
				if b.hitLPLimitStatus == StatusNotSolved {
					b.hitLPLimitStatus = reRes.Status
				}
				prunedBySB = true
				break
			}
			reMin := lpMinWithConst(p, reRes)
			if math.IsNaN(reMin) || math.IsInf(reMin, 0) {
				b.unsolvedMinBound = min(b.unsolvedMinBound, nodeMin)
				if b.hitLPLimitStatus == StatusNotSolved {
					b.hitLPLimitStatus = StatusNumericalError
				}
				prunedBySB = true
				break
			}
			nodeRes = reRes
			nodeMin = max(nodeMin, reMin)
			if nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
				b.prunedMinBound = min(b.prunedMinBound, nodeMin)
				prunedBySB = true
				break
			}
			if isIntegerFeasible(p, nodeRes.Primal) {
				if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
					if b.acceptIncumbent(cand, minObjective(p, cand)) {
						lastIncumbentNode = b.nodesExplored
						heurMisses = 0
					}
					if nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
						b.prunedMinBound = min(b.prunedMinBound, nodeMin)
						prunedBySB = true
						break
					}
				}
			}
		}
		if stoppedByLimit {
			break
		}
		if prunedBySB {
			continue
		}

		if bVar == -1 {
			// The LP point is integral within intFeasTol, yet its rounding violates the
			// rows (typical with big-M constraints). Branch on the integer variable furthest
			// from an integer so the subtree is explored instead of discarded.
			bVar, bVal = selectNearIntegralBranchVar(p, nodeRes.Primal, curLB, curUB)
		}
		if bVar == -1 {
			// Nothing can be branched on: keep the node's bound so the final status
			// never claims an optimal proof over an unexplored region.
			b.unsolvedMinBound = min(b.unsolvedMinBound, nodeMin)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = StatusNumericalError
			}
			continue
		}

		cDown, cUp := createChildNodes(
			nodeMin,
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

// propagateBoundsChecked runs up to maxPasses full sweeps of propagateRow and reports
// whether bounds remain feasible and whether a fixed point was reached before maxPasses.
func propagateBoundsChecked(p *CompiledProblem, lb, ub []float64, maxPasses int) (feasible, converged bool) {
	m := p.numConstraints
	for range maxPasses {
		anyChanged := false
		for i := range m {
			feas, chg := propagateRow(p, i, lb, ub, nil)
			if !feas {
				return false, false
			}
			if chg {
				anyChanged = true
			}
		}
		if !anyChanged {
			return true, true
		}
	}
	return true, false
}

// propagateFromCols propagates tightened bounds starting only from rows incident to startCols,
// using reusable queue and membership scratch buffers.
func propagateFromCols(p *CompiledProblem, lb, ub []float64, startCols []int, queue *[]int, inQueue []bool) bool {
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
	for _, col := range startCols {
		pushCol(col)
	}

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
			bestBoundUser = p.userObjFromShiftedMin(bestBoundMin)
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

	return b.optimalOrNumErr(Result{
		modelID:      p.modelID,
		Status:       status,
		Objective:    p.EvaluateObjective(b.bestPrimal),
		BestBound:    p.userObjFromShiftedMin(bestBoundMin),
		RootLPBound:  b.rootLPUser,
		RootCutBound: b.rootCutUser,
		Values:       b.bestPrimal,
		Nodes:        b.nodesExplored,
		Iterations:   b.totalLPIters,
		CutsAdded:    b.cutsAdded,
		SolveTime:    time.Since(b.startTime),
	})
}

func (b *babEngine) acceptIncumbent(cand []float64, candMin float64) bool {
	if math.IsNaN(candMin) || math.IsInf(candMin, 0) || candMin >= b.bestObjMin {
		return false
	}
	b.bestObjMin = candMin
	b.bestPrimal = cand
	return true
}

func (b *babEngine) optimalOrNumErr(res Result) Result {
	if res.Status == StatusOptimal && (math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) || math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0)) {
		return Result{
			modelID:    res.modelID,
			Status:     StatusNumericalError,
			Nodes:      res.Nodes,
			Iterations: res.Iterations,
			CutsAdded:  res.CutsAdded,
			SolveTime:  res.SolveTime,
		}
	}
	return res
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

func applyNodeBoundsTracked(node *searchNode, curLB, curUB []float64, touched []int) ([]int, bool) {
	for cur := node; cur != nil; cur = cur.parent {
		if cur.hasDelta {
			d := cur.delta
			chg := false
			if d.lb > curLB[d.col] {
				curLB[d.col] = d.lb
				chg = true
			}
			if d.ub < curUB[d.col] {
				curUB[d.col] = d.ub
				chg = true
			}
			if chg {
				if curLB[d.col] > curUB[d.col]+intFeasTol {
					return touched, false
				}
				touched = append(touched, d.col)
			}
		}
		for _, d := range cur.base {
			chg := false
			if d.lb > curLB[d.col] {
				curLB[d.col] = d.lb
				chg = true
			}
			if d.ub < curUB[d.col] {
				curUB[d.col] = d.ub
				chg = true
			}
			if chg {
				if curLB[d.col] > curUB[d.col]+intFeasTol {
					return touched, false
				}
				touched = append(touched, d.col)
			}
		}
	}
	return touched, true
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

// probeBranchBound evaluates one strong-branching probe in place.
// It updates col via SetColBound and runs a zero-allocation SolveBoundOnly solve.
func probeBranchBound(
	lpSolver *LPSolver,
	col int,
	origL, origU, probeLB, probeUB float64,
	maxIter int,
) (Status, float64, int) {
	lpSolver.SetColBound(col, probeLB, probeUB)
	st, bound, iters := lpSolver.SolveBoundOnly(maxIter)
	lpSolver.SetColBound(col, origL, origU)
	return st, bound, iters
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
	return selectBranchVarWithBuf(p, lpSolver, lpRes, curLB, curUB, pCosts, depth, totalIters, iterLeft, nil)
}

func evalBranchCand(p *CompiledProblem, col int, xVal, lb, ub float64, pc pseudoCost) (branchCand, bool) {
	if ub-lb < halfFracTarget {
		return branchCand{}, false
	}
	v := min(max(xVal, lb), ub)
	f := v - math.Floor(v)
	if f <= intFeasTol || f >= 1.0-intFeasTol {
		return branchCand{}, false
	}
	if math.Floor(v) >= ub || math.Ceil(v) <= lb {
		return branchCand{}, false
	}
	dEst := f * pc.avgDown()
	uEst := (1.0 - f) * pc.avgUp()
	score := max(dEst, minPseudoScoreFloor) * max(uEst, minPseudoScoreFloor)
	score += pseudoTieScale * (halfFracTarget - math.Abs(f-halfFracTarget)) * (1.0 + math.Abs(p.objMin[col]))
	return branchCand{col: col, val: v, dEst: dEst, uEst: uEst, score: score}, true
}

func selectBranchVarWithBuf(
	p *CompiledProblem,
	lpSolver *LPSolver,
	lpRes LPResult,
	curLB, curUB []float64,
	pCosts []pseudoCost,
	depth int,
	totalIters *int,
	iterLeft func() int,
	candsBuf *[]branchCand,
) (bestCol int, bestVal float64) {
	out := selectBranchVarDetailed(p, lpSolver, lpRes, curLB, curUB, pCosts, depth, totalIters, iterLeft, candsBuf, false)
	return out.bestCol, out.bestVal
}

func selectBranchVarDetailed(
	p *CompiledProblem,
	lpSolver *LPSolver,
	lpRes LPResult,
	curLB, curUB []float64,
	pCosts []pseudoCost,
	depth int,
	totalIters *int,
	iterLeft func() int,
	candsBuf *[]branchCand,
	allowDomainRed bool,
) sbOutcome {
	bestCol := -1
	bestVal := 0.0
	bestScore := -1.0

	if depth > strongBranchMaxDepth {
		for _, col := range p.intCols {
			if c, ok := evalBranchCand(p, col, lpRes.Primal[col], curLB[col], curUB[col], pCosts[col]); ok && c.score > bestScore {
				bestScore = c.score
				bestCol = c.col
				bestVal = c.val
			}
		}
		return sbOutcome{bestCol: bestCol, bestVal: bestVal}
	}

	var cands []branchCand
	if candsBuf != nil {
		cands = (*candsBuf)[:0]
	}

	for _, col := range p.intCols {
		if c, ok := evalBranchCand(p, col, lpRes.Primal[col], curLB[col], curUB[col], pCosts[col]); ok {
			cands = append(cands, c)
		}
	}
	if candsBuf != nil {
		*candsBuf = cands
	}

	if len(cands) == 0 {
		return sbOutcome{bestCol: -1}
	}

	if len(cands) > 1 {
		slices.SortFunc(cands, func(a, b branchCand) int {
			if c := cmp.Compare(b.score, a.score); c != 0 {
				return c
			}
			return cmp.Compare(a.col, b.col)
		})

		sbLimit := min(strongBranchCandCap, len(cands))
		var sbCP *lpCheckpoint
		if lpSolver.ws != nil {
			sbCP = &lpSolver.ws.sbCP
		} else {
			sbCP = &lpCheckpoint{}
		}
		hasSnap := false

		// childGain scores one strong-branching child from its bound-only probe.
		// Optimal solves update pseudocost statistics; dual-feasible iteration-limited
		// probes contribute their verified dual lower bound to candidate scoring.
		baseObjMin := lpRes.ObjectiveMin
		childGain := func(st Status, bound, est, dist float64, pc *pseudoCost, up bool) float64 {
			switch st {
			case StatusOptimal:
				if math.IsNaN(bound) {
					return est
				}
				g := max(minPseudoScoreFloor, bound-baseObjMin)
				pc.record(up, g, dist)
				return g
			case StatusIterationLimit:
				if !math.IsNaN(bound) {
					return max(minPseudoScoreFloor, bound-baseObjMin)
				}
				return est
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
			if !hasSnap {
				lpSolver.captureCheckpoint(sbCP)
				hasSnap = true
			}
			downSt, downBound, downIters := probeBranchBound(lpSolver, col, origL, origU, origL, fl, sbIter)
			*totalIters += downIters

			var upSt Status
			upBound := math.NaN()
			upProbed := false
			if !lpSolver.timeExpired() {
				sbIter = min(strongBranchMaxIter, iterLeft())
				if sbIter > 0 {
					var upIters int
					upSt, upBound, upIters = probeBranchBound(lpSolver, col, origL, origU, cl, origU, sbIter)
					*totalIters += upIters
					upProbed = true
				}
			}

			if allowDomainRed && cl == fl+1.0 {
				downInfeas := downSt == StatusInfeasible
				upInfeas := upProbed && upSt == StatusInfeasible
				if downInfeas && upInfeas {
					lpSolver.restoreCheckpoint(sbCP)
					return sbOutcome{bestCol: -1, pruned: true}
				}
				if downInfeas {
					curLB[col] = cl
					lpSolver.restoreCheckpoint(sbCP)
					if curLB[col] > curUB[col]+intFeasTol {
						return sbOutcome{bestCol: -1, pruned: true}
					}
					return sbOutcome{
						bestCol:     -1,
						fixedDeltas: []boundDelta{{col: col, lb: cl, ub: origU}},
					}
				}
				if upInfeas {
					curUB[col] = fl
					lpSolver.restoreCheckpoint(sbCP)
					if curLB[col] > curUB[col]+intFeasTol {
						return sbOutcome{bestCol: -1, pruned: true}
					}
					return sbOutcome{
						bestCol:     -1,
						fixedDeltas: []boundDelta{{col: col, lb: origL, ub: fl}},
					}
				}
			}
			if !upProbed {
				break
			}

			dGain := childGain(downSt, downBound, c.dEst, v-fl, &pCosts[col], false)
			uGain := childGain(upSt, upBound, c.uEst, cl-v, &pCosts[col], true)
			c.score = max(dGain, minPseudoScoreFloor) * max(uGain, minPseudoScoreFloor)
		}

		if hasSnap {
			lpSolver.restoreCheckpoint(sbCP)
		}
	}

	for _, c := range cands {
		if c.score > bestScore {
			bestScore = c.score
			bestCol = c.col
			bestVal = c.val
		}
	}
	return sbOutcome{bestCol: bestCol, bestVal: bestVal}
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

// minObjective computes the shifted minimized objective value objConstMin + sum_j ObjMin[j]*x[j].
func minObjective(p *CompiledProblem, x []float64) float64 {
	return evalLinearSum(p.objConstMinVal, p.objMin, x, p.numVars)
}

func lpMinWithConst(p *CompiledProblem, res LPResult) float64 {
	if p == nil || p.objConstant == 0 || len(res.Primal) < p.numVars {
		return res.ObjectiveMin
	}
	return evalLinearSum(p.objConstMin(), p.objMin, res.Primal, p.numVars)
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
	var slackAcc dot2Acc
	slackAcc.addVal(bestObjMin)
	slackAcc.addVal(-p.objConstMin())
	for j := range n {
		c := p.objMin[j]
		if c > 0 {
			if math.IsInf(baseLB[j], -1) {
				return
			}
			slackAcc.addProd(-c, baseLB[j])
		} else if c < 0 {
			if math.IsInf(baseUB[j], 1) {
				return
			}
			slackAcc.addProd(-c, baseUB[j])
		}
	}
	var slack float64
	if val, eb, ok := slackAcc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		if val+eb < -intFeasTol {
			return
		}
		slack = max(0.0, val+eb)
	} else {
		var w wideDotAcc
		w.addVal(bestObjMin)
		w.addVal(-p.objConstMin())
		for j := range n {
			c := p.objMin[j]
			if c > 0 {
				w.addProd(-c, baseLB[j])
			} else if c < 0 {
				w.addProd(-c, baseUB[j])
			}
		}
		val := w.finish(math.NaN())
		if math.IsNaN(val) || math.IsInf(val, 0) || val < -intFeasTol {
			return
		}
		slack = max(0.0, val+0x1p-50*math.Abs(val))
	}
	tlb := slices.Clone(baseLB)
	tub := slices.Clone(baseUB)
	for j := range n {
		c := p.objMin[j]
		if c > heurCoeffZeroTol {
			step := slack / c
			u := tlb[j] + step + 0x1p-51*(math.Abs(tlb[j])+math.Abs(step))
			if p.varCat[j] != Continuous {
				u = math.Floor(u + heurIntBoundTol)
			}
			if u < tub[j] {
				tub[j] = max(tlb[j], u)
			}
		} else if c < -heurCoeffZeroTol {
			step := slack / c
			l := tub[j] + step - 0x1p-51*(math.Abs(tub[j])+math.Abs(step))
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
