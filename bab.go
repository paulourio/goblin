package goblin

import (
	"cmp"
	"container/heap"
	"math"
	"slices"
	"sync"
	"sync/atomic"
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
	inTreeCutMinVars         = 120
	inTreeCutMaxDepth        = 4
	inTreeCutShallowDepth    = 2
	inTreeCutStride          = 8
	inTreeCutMaxMisses       = 4
	inTreeCutMaxRounds       = 24
	inTreeCutMinEff          = 0.02
	inTreeCutMinGainAbs      = 1e-5
	inTreeCutMinGainRel      = 1e-6
	inTreeCutMinPoolCap      = 32
	inTreeCutMaxPoolCap      = 128
	inTreeCutMinBatch        = 4
	inTreeCutMaxBatch        = 10
	inTreeCutBatchDiv        = 10
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
	maxParallelWorkers       = 64
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
	brDist      float64
	depth       int
	id          int
	steps       uint16
	hasDelta    bool
	orbitalDown bool
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

type sbConfig struct {
	sym          *nodeSymmetry
	deadline     time.Time
	done         <-chan struct{}
	hasIncumbent bool
	bestObjMin   float64
	pruneRelTol  float64
	absGapTol    float64
	objConstMin  float64
}

type sbOutcome struct {
	bestCol     int
	bestVal     float64
	pruned      bool
	prunedBound float64
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
	inTreeCutMisses    int
	inTreeCutRounds    int
	propQueue          []int
	propInQueue        []bool
	touchedCols        []int
	sym                nodeSymmetry
	orbitBuf           []int
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

func newHeurSolverFrom(p *CompiledProblem, lpSolver *LPSolver, deadline time.Time, done <-chan struct{}, disableSIMD bool) *LPSolver {
	canCopyRoot := lpSolver != nil && lpSolver.dualValid && len(lpSolver.extraRows) == 0
	hs := newLPSolver(p, nil, !canCopyRoot)
	hs.Deadline = deadline
	hs.Done = done
	hs.DisableSIMD = disableSIMD
	hs.SkipExactFarkas = true
	if canCopyRoot {
		hs.CopyRootBasisFrom(lpSolver)
	}
	return hs
}

func (b *babEngine) ensureHeurSolver() *LPSolver {
	if b.heurSolver == nil {
		b.heurSolver = newHeurSolverFrom(b.p, b.lpSolver, b.deadline, b.done, b.opts.DisableSIMD)
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
	feasOpts.Workers = 1
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

func (b *babEngine) runRootSubMIP(primal []float64, hs *LPSolver, isContLinked []bool, enableCuts bool) bool {
	remNodes := b.nodesLeft()
	remIter := b.iterLeft()
	if remNodes <= 0 || remIter <= 0 {
		return false
	}
	var rootStatus []varBasisStatus
	if b.lpSolver != nil && b.lpSolver.ws != nil && len(b.lpSolver.ws.status) >= b.p.numVars {
		rootStatus = b.lpSolver.ws.status[:b.p.numVars]
	}
	sol, objMin, usedN, usedIt, ok := tryRootSubMIP(b.p, hs, rootStatus, primal, b.p.varLB, b.p.varUB, b.bestObjMin, b.deadline, b.opts.isSubMIP, remNodes, remIter, isContLinked, enableCuts)
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
		b.runRootSubMIP(rootPrimal, b.heurSolver, isContLinked, b.cutsEnabled())
	}
}

func addDynamicCutPlanes(lpSolver *LPSolver, cuts []cutPlane) int {
	if len(cuts) == 0 {
		return 0
	}
	rows := make([]dynamicRow, 0, len(cuts))
	for _, c := range cuts {
		if dr, st := lpSolver.buildCutRow(c.ColIdx, c.Val, c.RHS); st == cutBuildOK {
			rows = append(rows, dr)
		}
	}
	if len(rows) > 0 {
		lpSolver.addCutRows(rows)
	}
	return len(rows)
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
		addedBatch := addDynamicCutPlanes(lpSolver, newCuts)
		if addedBatch == 0 {
			break
		}
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
		b.cutsAdded += addedBatch
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
		} else if anchorCuts == 0 {
			break
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
			b.runRootSubMIP(cutPrimal, hs, isContLinked, false)
		}
		return
	}
	if b.runRootSubMIP(cutPrimal, hs, isContLinked, true) {
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
	updateCutAgesState(b.p, b.lpSolver, &b.cutAges, &b.nodesSincePurge, allowPurge)
}

func updateCutAgesState(p *CompiledProblem, s *LPSolver, cutAges *[]int, nodesSincePurge *int, allowPurge bool) {
	numExtra := len(s.extraRows)
	if numExtra == 0 || !s.luValid || s.ws == nil {
		return
	}
	ages := *cutAges
	oldLen := len(ages)
	if oldLen < numExtra {
		if cap(ages) >= numExtra {
			ages = ages[:numExtra]
			clear(ages[oldLen:])
		} else {
			grown := make([]int, numExtra)
			copy(grown, ages)
			ages = grown
		}
	} else if oldLen > numExtra {
		ages = ages[:numExtra]
	}
	n := p.numVars
	baseM := p.numConstraints
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
			ages[e]++
			if ages[e] >= inTreeCutMinAge {
				agedSlack++
			} else {
				freshSlack++
			}
		} else {
			ages[e] = 0
		}
	}
	*cutAges = ages
	*nodesSincePurge++
	if !allowPurge || numExtra <= inTreeCutPurgeMinCuts || *nodesSincePurge < inTreeCutPurgePeriod {
		return
	}
	if agedSlack >= max(inTreeCutMinAgedPurge, numExtra/4) && agedSlack >= freshSlack {
		if purged := s.PurgeSlackCuts(inTreeCutSlackTol); purged > 0 {
			ages = ages[:len(s.extraRows)]
			clear(ages)
			*cutAges = ages
		}
		*nodesSincePurge = 0
	}
}

func (b *babEngine) tryInTreeCuts(
	node *searchNode,
	nodeRes *LPResult,
	nodeMin *float64,
	poolCap, batchCap int,
	pruneRelTol float64,
	lastIncumbentNode, heurMisses *int,
) (pruned, stop bool) {
	if !b.cutsEnabled() || b.opts.isSubMIP || b.p.numVars < inTreeCutMinVars || b.timeExpired() {
		return false, false
	}
	if node.depth < 1 || node.depth > inTreeCutMaxDepth || b.inTreeCutMisses >= inTreeCutMaxMisses || b.inTreeCutRounds >= inTreeCutMaxRounds {
		return false, false
	}
	if len(b.lpSolver.extraRows) >= poolCap || (node.depth > inTreeCutShallowDepth && b.nodesExplored%inTreeCutStride != 0) {
		return false, false
	}
	newCuts := generateInTreeCutRound(b.lpSolver, nodeRes.Primal, batchCap, inTreeCutMinEff, &b.cutWS)
	if len(newCuts) == 0 {
		b.inTreeCutMisses++
		return false, false
	}
	if !b.capLPIter(b.lpSolver, b.defaultLPIter) {
		node.lowerBound = max(node.lowerBound, *nodeMin)
		heap.Push(&b.pq, node)
		b.hitIterLimit = true
		return false, true
	}
	snap := b.lpSolver.snapshot()
	added := addDynamicCutPlanes(b.lpSolver, newCuts)
	if added == 0 {
		b.inTreeCutMisses++
		return false, false
	}
	nextRes := b.lpSolver.Solve()
	b.totalLPIters += nextRes.Iterations
	if nextRes.Status == StatusInfeasible {
		b.cutsAdded += added
		b.inTreeCutRounds++
		b.inTreeCutMisses = 0
		return true, false
	}
	if nextRes.Status == StatusTimeLimit || (nextRes.Status == StatusIterationLimit && b.budgetBound(b.lpSolver)) {
		b.lpSolver.restore(snap)
		node.lowerBound = max(node.lowerBound, *nodeMin)
		heap.Push(&b.pq, node)
		if nextRes.Status == StatusTimeLimit {
			b.hitTimeLimit = true
		} else {
			b.hitIterLimit = true
		}
		return false, true
	}
	nextMin := lpMinWithConst(b.p, nextRes)
	gainTol := max(inTreeCutMinGainAbs, inTreeCutMinGainRel*math.Abs(*nodeMin))
	if nextRes.Status != StatusOptimal || math.IsNaN(nextMin) || math.IsInf(nextMin, 0) || nextMin-*nodeMin <= gainTol {
		b.lpSolver.restore(snap)
		b.inTreeCutMisses++
		return false, false
	}
	b.cutsAdded += added
	b.inTreeCutRounds++
	b.inTreeCutMisses = 0
	*nodeRes = nextRes
	*nodeMin = max(*nodeMin, nextMin)
	if *nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(*nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
		b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
		return true, false
	}
	if isIntegerFeasible(b.p, nodeRes.Primal) {
		if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
			if b.acceptIncumbent(cand, minObjective(b.p, cand)) {
				*lastIncumbentNode = b.nodesExplored
				*heurMisses = 0
			}
			if *nodeMin >= b.bestObjMin-b.opts.AbsGapTol || isGapClosed(*nodeMin, b.bestObjMin, pruneRelTol, b.opts.AbsGapTol) {
				b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
				return true, false
			}
		}
	}
	return false, false
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
	b.sym = initNodeSymmetry(p)

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

	// If tightenUnboundedIntCols or root bound propagation cut off curLPRes.Primal,
	// re-solve the root LP under the tightened bounds so curLPRes stays consistent
	// with curLB/curUB (and cannot leave all integer cols fixed with a stale fractional primal).
	rootBoundsCutLP := false
	if len(curLPRes.Primal) >= n {
		for j := range n {
			if curLPRes.Primal[j] < curLB[j]-primalTol || curLPRes.Primal[j] > curUB[j]+primalTol {
				rootBoundsCutLP = true
				break
			}
		}
	}
	if rootBoundsCutLP {
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
	// Cap per-node pruning relative tolerance at 1.0 so upper - r*max(1,|upper|) remains
	// monotonically non-decreasing when bestObjMin improves; when RelGapTol > 1.0, check the
	// global bound against the full RelGapTol directly.
	pruneRelTol := min(b.opts.RelGapTol, 1.0)
	objConstMin := p.objConstMin()
	var brVar int
	var brVal float64
	rootSBRounds := 0
	for {
		sb := selectBranchVarCore(
			p, b.lpSolver, curLPRes, curLB, curUB, pCosts, 0,
			&b.totalLPIters, b.iterLeft, &b.sbCands,
			rootSBRounds < maxSBDomainRedRounds,
			sbConfig{
				sym:          &b.sym,
				deadline:     b.deadline,
				done:         b.done,
				hasIncumbent: b.bestPrimal != nil,
				bestObjMin:   b.bestObjMin,
				pruneRelTol:  pruneRelTol,
				absGapTol:    b.opts.AbsGapTol,
				objConstMin:  objConstMin,
			},
		)
		b.prunedMinBound = min(b.prunedMinBound, sb.prunedBound)
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
	downNode, upNode := b.createChildNodesWithOrbit(rootNodeMin, 0, nil, brVar, brVal, curLB, curUB, pCosts[brVar], &nodeCounter)
	heap.Push(&b.pq, downNode)
	heap.Push(&b.pq, upNode)

	lastIncumbentNode := 1
	heurMisses := 0
	rootCutsHelped := b.cutsAdded > 0 && b.rootCutMin-b.rootLPMin > max(zeroGainAbsTol, zeroGainRelTol*math.Abs(b.rootLPMin))
	inTreePoolCap := min(inTreeCutMaxPoolCap, max(inTreeCutMinPoolCap, p.numConstraints))
	inTreeBatchCap := min(inTreeCutMaxBatch, max(inTreeCutMinBatch, p.numConstraints/inTreeCutBatchDiv))
	var isContLinked []bool
	if b.opts.EnableHeuristics {
		isContLinked = b.contLinkedIntCols()
	}
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
		if b.opts.Workers > 1 && !b.opts.isSubMIP {
			openCount := b.pq.Len()
			if b.plungeNode != nil {
				openCount++
			}
			workers := min(b.opts.Workers, maxParallelWorkers)
			rampTarget := min(max(2*workers, 8), 32)
			if openCount >= rampTarget {
				return b.exploreTreeParallel(
					workers, pCosts, baseLB, baseUB, rootPropConv, isContLinked,
					rootCutsHelped, inTreePoolCap, inTreeBatchCap,
					pruneRelTol, objConstMin, &nodeCounter, lastIncumbentNode, heurMisses,
				)
			}
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

		if fromPlunge && node.hasDelta && len(node.base) == 0 {
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

		if node.hasDelta && !node.orbitalDown {
			gain := max(0.0, nodeMin-node.lowerBound)
			pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		nodeIntFeas := isIntegerFeasible(p, nodeRes.Primal)
		if nodeIntFeas {
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

		if !nodeIntFeas && rootCutsHelped {
			prunedByCut, stopByLimit := b.tryInTreeCuts(
				node, &nodeRes, &nodeMin, inTreePoolCap, inTreeBatchCap,
				pruneRelTol, &lastIncumbentNode, &heurMisses,
			)
			if stopByLimit {
				break
			}
			if prunedByCut {
				continue
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
			sb := selectBranchVarCore(
				p, b.lpSolver, nodeRes, curLB, curUB, pCosts, node.depth,
				&b.totalLPIters, b.iterLeft, &b.sbCands,
				sbRounds < maxSBDomainRedRounds,
				sbConfig{
					sym:          &b.sym,
					deadline:     b.deadline,
					done:         b.done,
					hasIncumbent: b.bestPrimal != nil,
					bestObjMin:   b.bestObjMin,
					pruneRelTol:  pruneRelTol,
					absGapTol:    b.opts.AbsGapTol,
					objConstMin:  objConstMin,
				},
			)
			b.prunedMinBound = min(b.prunedMinBound, sb.prunedBound)
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

		cDown, cUp := b.createChildNodesWithOrbit(
			nodeMin,
			node.depth,
			node,
			bVar,
			bVal,
			curLB,
			curUB,
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

type nodeOutcomeKind uint8

const (
	nodeOutcomeNone nodeOutcomeKind = iota
	nodeOutcomePruned
	nodeOutcomeUnsolved
)

type parCoordinator struct {
	b              *babEngine
	baseLB         []float64
	baseUB         []float64
	rootPropConv   bool
	isContLinked   []bool
	rootCutsHelped bool
	inTreePoolCap  int
	inTreeBatchCap int
	pruneRelTol    float64
	objConstMin    float64

	mu               sync.Mutex
	cond             *sync.Cond
	pq               nodeQueue
	numWorkers       int
	waitingWorkers   int
	activeBounds     []float64
	bestObjMin       float64
	bestPrimal       []float64
	nodesExplored    int
	unsolvedMinBound float64
	prunedMinBound   float64
	hitTimeLimit     bool
	hitIterLimit     bool
	hitNodeLimit     bool
	hitLPLimitStatus Status

	bestObjBits  atomic.Uint64
	hasIncumbent atomic.Bool
	totalIters   atomic.Int64
	cutsAdded    atomic.Int64
	stopAll      atomic.Bool
}

func (c *parCoordinator) currentGlobalBoundLocked() float64 {
	bestBoundMin := min(c.bestObjMin, min(c.unsolvedMinBound, c.prunedMinBound))
	for _, nd := range c.pq {
		if nd.lowerBound < bestBoundMin {
			bestBoundMin = nd.lowerBound
		}
	}
	for _, ab := range c.activeBounds {
		if ab < bestBoundMin {
			bestBoundMin = ab
		}
	}
	return bestBoundMin
}

func (c *parCoordinator) iterLeft(pendingLocal int) int {
	if c.b.opts.MaxIter <= 0 {
		return math.MaxInt
	}
	return c.b.opts.MaxIter - int(c.totalIters.Load()) - pendingLocal
}

func (c *parCoordinator) capLPIter(s *LPSolver, base, pendingLocal int) bool {
	rem := c.iterLeft(pendingLocal)
	if rem <= 0 {
		return false
	}
	s.MaxIter = min(base, rem)
	return true
}

func (c *parCoordinator) addIters(delta int) {
	if delta > 0 {
		c.totalIters.Add(int64(delta))
	}
}

func (c *parCoordinator) bestObj() (bool, float64) {
	return c.hasIncumbent.Load(), math.Float64frombits(c.bestObjBits.Load())
}

func (c *parCoordinator) acceptIncumbent(cand []float64, candMin float64) bool {
	if math.IsNaN(candMin) || math.IsInf(candMin, 0) {
		return false
	}
	if c.hasIncumbent.Load() && candMin >= math.Float64frombits(c.bestObjBits.Load()) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if candMin >= c.bestObjMin {
		return false
	}
	c.bestObjMin = candMin
	c.bestPrimal = cand
	c.bestObjBits.Store(math.Float64bits(candMin))
	c.hasIncumbent.Store(true)
	if c.b.opts.RelGapTol > 1.0 && isGapClosed(c.currentGlobalBoundLocked(), c.bestObjMin, c.b.opts.RelGapTol, c.b.opts.AbsGapTol) {
		c.stopAll.Store(true)
		c.cond.Broadcast()
	}
	return true
}

func (c *parCoordinator) recordPrevLocked(kind nodeOutcomeKind, bound, minPruned float64, st Status) {
	if minPruned < c.prunedMinBound {
		c.prunedMinBound = minPruned
	}
	switch kind {
	case nodeOutcomePruned:
		if bound < c.prunedMinBound {
			c.prunedMinBound = bound
		}
	case nodeOutcomeUnsolved:
		if bound < c.unsolvedMinBound {
			c.unsolvedMinBound = bound
		}
		if c.hitLPLimitStatus == StatusNotSolved && st != StatusNotSolved {
			c.hitLPLimitStatus = st
		}
	}
}

func (c *parCoordinator) publishBranch(workerID int, pushChild, nextPlunge *searchNode, minPruned float64) {
	c.mu.Lock()
	if minPruned < c.prunedMinBound {
		c.prunedMinBound = minPruned
	}
	heap.Push(&c.pq, pushChild)
	c.activeBounds[workerID] = nextPlunge.lowerBound
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *parCoordinator) abortOnLimit(workerID int, node *searchNode, stop Status, minPruned float64) {
	c.mu.Lock()
	if minPruned < c.prunedMinBound {
		c.prunedMinBound = minPruned
	}
	if node != nil {
		heap.Push(&c.pq, node)
	}
	c.activeBounds[workerID] = math.Inf(1)
	if stop == StatusTimeLimit {
		c.hitTimeLimit = true
	} else {
		c.hitIterLimit = true
	}
	c.stopAll.Store(true)
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *parCoordinator) nextNode(
	workerID int,
	localPlunge *searchNode,
	prevKind nodeOutcomeKind,
	prevBound, prevMinPruned float64,
	prevStatus Status,
	allowPlunge bool,
) (*searchNode, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.recordPrevLocked(prevKind, prevBound, prevMinPruned, prevStatus)
	if localPlunge == nil {
		c.activeBounds[workerID] = math.Inf(1)
	} else {
		c.activeBounds[workerID] = localPlunge.lowerBound
	}

	parkPlunge := func() {
		if localPlunge != nil {
			heap.Push(&c.pq, localPlunge)
			localPlunge = nil
		}
		c.activeBounds[workerID] = math.Inf(1)
	}

	for {
		if c.stopAll.Load() {
			parkPlunge()
			return nil, false
		}
		if c.b.opts.RelGapTol > 1.0 && c.bestPrimal != nil && isGapClosed(c.currentGlobalBoundLocked(), c.bestObjMin, c.b.opts.RelGapTol, c.b.opts.AbsGapTol) {
			parkPlunge()
			c.stopAll.Store(true)
			c.cond.Broadcast()
			return nil, false
		}
		if c.b.opts.MaxNodes > 0 && c.nodesExplored >= c.b.opts.MaxNodes {
			parkPlunge()
			c.hitNodeLimit = true
			c.stopAll.Store(true)
			c.cond.Broadcast()
			return nil, false
		}
		if c.b.timeExpired() {
			parkPlunge()
			c.hitTimeLimit = true
			c.stopAll.Store(true)
			c.cond.Broadcast()
			return nil, false
		}
		if c.iterLeft(0) <= 0 {
			parkPlunge()
			c.hitIterLimit = true
			c.stopAll.Store(true)
			c.cond.Broadcast()
			return nil, false
		}

		var node *searchNode
		fromPlunge := false
		if localPlunge != nil {
			if allowPlunge {
				node = localPlunge
				localPlunge = nil
				fromPlunge = true
			} else {
				heap.Push(&c.pq, localPlunge)
				localPlunge = nil
			}
		}
		if node == nil {
			if c.pq.Len() == 0 {
				c.activeBounds[workerID] = math.Inf(1)
				if c.waitingWorkers+1 == c.numWorkers {
					c.stopAll.Store(true)
					c.cond.Broadcast()
					return nil, false
				}
				c.waitingWorkers++
				c.cond.Wait()
				c.waitingWorkers--
				continue
			}
			node = heap.Pop(&c.pq).(*searchNode)
			fromPlunge = false
		}

		if node.lowerBound >= c.bestObjMin-c.b.opts.AbsGapTol || isGapClosed(node.lowerBound, c.bestObjMin, c.pruneRelTol, c.b.opts.AbsGapTol) {
			if node.lowerBound < c.prunedMinBound {
				c.prunedMinBound = node.lowerBound
			}
			c.activeBounds[workerID] = math.Inf(1)
			continue
		}

		c.nodesExplored++
		c.activeBounds[workerID] = node.lowerBound
		return node, fromPlunge
	}
}

type parWorker struct {
	id               int
	coord            *parCoordinator
	lpSolver         *LPSolver
	ownsLPSolver     bool
	heurSolver       *LPSolver
	pCosts           []pseudoCost
	sym              nodeSymmetry
	curLB            []float64
	curUB            []float64
	propQueue        []int
	propInQueue      []bool
	touchedCols      []int
	fixScratchLB     []float64
	fixScratchUB     []float64
	sbCands          []branchCand
	cutWS            cutGenWorkspace
	cutAges          []int
	nodesSincePurge  int
	inTreeCutMisses  int
	inTreeCutRounds  int
	orbitBuf         []int
	localNodeCounter int
	nodesProcessed   int
	heurMisses       int
	plungeNode       *searchNode
}

func (w *parWorker) close() {
	if w.ownsLPSolver && w.lpSolver != nil {
		w.lpSolver.Close()
		w.lpSolver = nil
	}
	if w.heurSolver != nil {
		w.heurSolver.Close()
		w.heurSolver = nil
	}
}

func (w *parWorker) ensureHeurSolver() *LPSolver {
	if w.heurSolver == nil {
		b := w.coord.b
		w.heurSolver = newHeurSolverFrom(b.p, w.lpSolver, b.deadline, b.done, b.opts.DisableSIMD)
	}
	return w.heurSolver
}

func (w *parWorker) polishIncumbent(x []float64) ([]float64, bool) {
	b := w.coord.b
	return polishIncumbentWith(b.p, b.deadline, b.done, x, &w.fixScratchLB, &w.fixScratchUB, w.ensureHeurSolver)
}

func (w *parWorker) isPruned(bound float64) bool {
	hasInc, bestObjMin := w.coord.bestObj()
	if !hasInc {
		return false
	}
	return bound >= bestObjMin-w.coord.b.opts.AbsGapTol || isGapClosed(bound, bestObjMin, w.coord.pruneRelTol, w.coord.b.opts.AbsGapTol)
}

func (w *parWorker) tryInTreeCuts(node *searchNode, nodeRes *LPResult, nodeMin *float64) (pruned bool, stopStatus Status) {
	c := w.coord
	b := c.b
	p := b.p
	if !b.cutsEnabled() || b.opts.isSubMIP || p.numVars < inTreeCutMinVars || b.timeExpired() {
		return false, StatusNotSolved
	}
	if node.depth < 1 || node.depth > inTreeCutMaxDepth || w.inTreeCutMisses >= inTreeCutMaxMisses || w.inTreeCutRounds >= inTreeCutMaxRounds {
		return false, StatusNotSolved
	}
	if len(w.lpSolver.extraRows) >= c.inTreePoolCap || (node.depth > inTreeCutShallowDepth && w.nodesProcessed%inTreeCutStride != 0) {
		return false, StatusNotSolved
	}
	newCuts := generateInTreeCutRound(w.lpSolver, nodeRes.Primal, c.inTreeBatchCap, inTreeCutMinEff, &w.cutWS)
	if len(newCuts) == 0 {
		w.inTreeCutMisses++
		return false, StatusNotSolved
	}
	if !c.capLPIter(w.lpSolver, b.defaultLPIter, 0) {
		node.lowerBound = max(node.lowerBound, *nodeMin)
		return false, StatusIterationLimit
	}
	snap := w.lpSolver.snapshot()
	added := addDynamicCutPlanes(w.lpSolver, newCuts)
	if added == 0 {
		w.inTreeCutMisses++
		return false, StatusNotSolved
	}
	nextRes := w.lpSolver.Solve()
	c.addIters(nextRes.Iterations)
	if nextRes.Status == StatusInfeasible {
		c.cutsAdded.Add(int64(added))
		w.inTreeCutRounds++
		w.inTreeCutMisses = 0
		return true, StatusNotSolved
	}
	if nextRes.Status == StatusTimeLimit || (nextRes.Status == StatusIterationLimit && b.budgetBound(w.lpSolver)) {
		w.lpSolver.restore(snap)
		node.lowerBound = max(node.lowerBound, *nodeMin)
		return false, nextRes.Status
	}
	nextMin := lpMinWithConst(p, nextRes)
	gainTol := max(inTreeCutMinGainAbs, inTreeCutMinGainRel*math.Abs(*nodeMin))
	if nextRes.Status != StatusOptimal || math.IsNaN(nextMin) || math.IsInf(nextMin, 0) || nextMin-*nodeMin <= gainTol {
		w.lpSolver.restore(snap)
		w.inTreeCutMisses++
		return false, StatusNotSolved
	}
	c.cutsAdded.Add(int64(added))
	w.inTreeCutRounds++
	w.inTreeCutMisses = 0
	*nodeRes = nextRes
	*nodeMin = max(*nodeMin, nextMin)
	if w.isPruned(*nodeMin) {
		return true, StatusNotSolved
	}
	if isIntegerFeasible(p, nodeRes.Primal) {
		if cand, ok := w.polishIncumbent(nodeRes.Primal); ok {
			if c.acceptIncumbent(cand, minObjective(p, cand)) {
				w.heurMisses = 0
			}
			if w.isPruned(*nodeMin) {
				return true, StatusNotSolved
			}
		}
	}
	return false, StatusNotSolved
}

func (w *parWorker) applyNodeBounds(node *searchNode, fromPlunge bool) bool {
	c := w.coord
	p := c.b.p
	enablePresolve := c.b.opts.EnablePresolve
	if fromPlunge && node.hasDelta && len(node.base) == 0 {
		d := node.delta
		tightened := false
		if d.lb > w.curLB[d.col] {
			w.curLB[d.col] = d.lb
			tightened = true
		}
		if d.ub < w.curUB[d.col] {
			w.curUB[d.col] = d.ub
			tightened = true
		}
		if w.curLB[d.col] > w.curUB[d.col]+intFeasTol {
			return false
		}
		if enablePresolve && tightened {
			if c.rootPropConv {
				return propagateFromCol(p, w.curLB, w.curUB, d.col, &w.propQueue, w.propInQueue)
			}
			return propagateBounds(p, w.curLB, w.curUB, nodePropPasses)
		}
		return true
	}
	copy(w.curLB, c.baseLB)
	copy(w.curUB, c.baseUB)
	var boundsFeas bool
	w.touchedCols, boundsFeas = applyNodeBoundsTracked(node, w.curLB, w.curUB, w.touchedCols[:0])
	if !boundsFeas {
		return false
	}
	if enablePresolve && len(w.touchedCols) > 0 {
		if c.rootPropConv {
			if len(w.touchedCols) == 1 {
				return propagateFromCol(p, w.curLB, w.curUB, w.touchedCols[0], &w.propQueue, w.propInQueue)
			}
			return propagateFromCols(p, w.curLB, w.curUB, w.touchedCols, &w.propQueue, w.propInQueue)
		}
		return propagateBounds(p, w.curLB, w.curUB, nodePropPasses)
	}
	return true
}

func (w *parWorker) run() {
	defer w.close()
	c := w.coord
	b := c.b
	p := b.p
	n := p.numVars
	hasCont := len(p.intCols) < n

	prevKind := nodeOutcomeNone
	prevBound := math.Inf(1)
	prevMinPruned := math.Inf(1)
	prevStatus := StatusNotSolved

	for {
		plunge := w.plungeNode
		w.plungeNode = nil
		node, fromPlunge := c.nextNode(
			w.id, plunge, prevKind, prevBound, prevMinPruned, prevStatus,
			w.nodesProcessed%plungeQueuePeriod != 0,
		)
		if node == nil {
			return
		}
		w.nodesProcessed++
		prevKind = nodeOutcomeNone
		prevBound = math.Inf(1)
		prevMinPruned = math.Inf(1)
		prevStatus = StatusNotSolved

		if !w.applyNodeBounds(node, fromPlunge) {
			continue
		}

		nodeLPIters := 0
		nodeRes, stop := solveNodeLP(
			w.lpSolver, w.curLB, w.curUB, b.defaultLPIter,
			func(s *LPSolver, base int) bool { return c.capLPIter(s, base, nodeLPIters) },
			b.budgetBound, &nodeLPIters,
		)
		c.addIters(nodeLPIters)
		if stop != StatusNotSolved {
			c.abortOnLimit(w.id, node, stop, prevMinPruned)
			return
		}
		if nodeRes.Status == StatusInfeasible {
			continue
		}
		if nodeRes.Status != StatusOptimal {
			prevKind = nodeOutcomeUnsolved
			prevBound = node.lowerBound
			prevStatus = nodeRes.Status
			continue
		}
		nodeMin := lpMinWithConst(p, nodeRes)
		if math.IsNaN(nodeMin) || math.IsInf(nodeMin, 0) {
			prevKind = nodeOutcomeUnsolved
			prevBound = node.lowerBound
			prevStatus = StatusNumericalError
			continue
		}
		if w.isPruned(nodeMin) {
			updateCutAgesState(p, w.lpSolver, &w.cutAges, &w.nodesSincePurge, true)
			prevKind = nodeOutcomePruned
			prevBound = nodeMin
			continue
		}

		updateCutAgesState(p, w.lpSolver, &w.cutAges, &w.nodesSincePurge, node.depth > strongBranchMaxDepth)

		if node.hasDelta && !node.orbitalDown {
			gain := max(0.0, nodeMin-node.lowerBound)
			w.pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		nodeIntFeas := isIntegerFeasible(p, nodeRes.Primal)
		if nodeIntFeas {
			if cand, ok := w.polishIncumbent(nodeRes.Primal); ok {
				if c.acceptIncumbent(cand, minObjective(p, cand)) {
					w.heurMisses = 0
				}
				if w.isPruned(nodeMin) {
					prevKind = nodeOutcomePruned
					prevBound = nodeMin
					continue
				}
			}
		}

		if !nodeIntFeas && c.rootCutsHelped {
			prunedByCut, cutStop := w.tryInTreeCuts(node, &nodeRes, &nodeMin)
			if cutStop != StatusNotSolved {
				c.abortOnLimit(w.id, node, cutStop, prevMinPruned)
				return
			}
			if prunedByCut {
				if w.isPruned(nodeMin) {
					prevKind = nodeOutcomePruned
					prevBound = nodeMin
				}
				continue
			}
		}

		stride := nodeHeurStride(n, w.heurMisses)
		shallowHeur := !b.postCutSubMIPFound && w.heurMisses < heurMedMissCap && node.depth <= heurShallowDepth && (n <= largeModelHeurVarCap || node.depth <= 1)
		runHeur := shallowHeur || w.nodesProcessed%stride == 0
		if b.opts.EnableHeuristics && !b.timeExpired() && runHeur {
			var hs *LPSolver
			if hasCont {
				hs = w.ensureHeurSolver()
			}
			if sol, objMin, ok := tryPropagatedRounding(p, nodeRes.Primal, w.curLB, w.curUB, hs, c.isContLinked); ok && c.acceptIncumbent(sol, objMin) {
				w.heurMisses = 0
				if w.isPruned(nodeMin) {
					prevKind = nodeOutcomePruned
					prevBound = nodeMin
					continue
				}
			} else {
				w.heurMisses++
			}
		}

		var bVar int
		var bVal float64
		prunedBySB := false
		sbRounds := 0
		for {
			hasInc, bestObjMin := c.bestObj()
			sbLPIters := 0
			sb := selectBranchVarCore(
				p, w.lpSolver, nodeRes, w.curLB, w.curUB, w.pCosts, node.depth,
				&sbLPIters,
				func() int { return c.iterLeft(sbLPIters) },
				&w.sbCands,
				sbRounds < maxSBDomainRedRounds,
				sbConfig{
					sym:          &w.sym,
					deadline:     b.deadline,
					done:         b.done,
					hasIncumbent: hasInc,
					bestObjMin:   bestObjMin,
					pruneRelTol:  c.pruneRelTol,
					absGapTol:    b.opts.AbsGapTol,
					objConstMin:  c.objConstMin,
				},
			)
			c.addIters(sbLPIters)
			prevMinPruned = min(prevMinPruned, sb.prunedBound)
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
				w.touchedCols = w.touchedCols[:0]
				for _, d := range sb.fixedDeltas {
					w.touchedCols = append(w.touchedCols, d.col)
				}
				var propOK bool
				if c.rootPropConv {
					if len(w.touchedCols) == 1 {
						propOK = propagateFromCol(p, w.curLB, w.curUB, w.touchedCols[0], &w.propQueue, w.propInQueue)
					} else {
						propOK = propagateFromCols(p, w.curLB, w.curUB, w.touchedCols, &w.propQueue, w.propInQueue)
					}
				} else {
					propOK = propagateBounds(p, w.curLB, w.curUB, nodePropPasses)
				}
				if !propOK {
					prunedBySB = true
					break
				}
			}
			reIters := 0
			reRes, reStop := solveNodeLP(
				w.lpSolver, w.curLB, w.curUB, b.defaultLPIter,
				func(s *LPSolver, base int) bool { return c.capLPIter(s, base, reIters) },
				b.budgetBound, &reIters,
			)
			c.addIters(reIters)
			if reStop != StatusNotSolved {
				node.lowerBound = max(node.lowerBound, nodeMin)
				c.abortOnLimit(w.id, node, reStop, prevMinPruned)
				return
			}
			if reRes.Status == StatusInfeasible {
				prunedBySB = true
				break
			}
			if reRes.Status != StatusOptimal {
				prevKind = nodeOutcomeUnsolved
				prevBound = nodeMin
				prevStatus = reRes.Status
				prunedBySB = true
				break
			}
			reMin := lpMinWithConst(p, reRes)
			if math.IsNaN(reMin) || math.IsInf(reMin, 0) {
				prevKind = nodeOutcomeUnsolved
				prevBound = nodeMin
				prevStatus = StatusNumericalError
				prunedBySB = true
				break
			}
			nodeRes = reRes
			nodeMin = max(nodeMin, reMin)
			if w.isPruned(nodeMin) {
				prevKind = nodeOutcomePruned
				prevBound = nodeMin
				prunedBySB = true
				break
			}
			if isIntegerFeasible(p, nodeRes.Primal) {
				if cand, ok := w.polishIncumbent(nodeRes.Primal); ok {
					if c.acceptIncumbent(cand, minObjective(p, cand)) {
						w.heurMisses = 0
					}
					if w.isPruned(nodeMin) {
						prevKind = nodeOutcomePruned
						prevBound = nodeMin
						prunedBySB = true
						break
					}
				}
			}
		}
		if prunedBySB {
			continue
		}

		if bVar == -1 {
			bVar, bVal = selectNearIntegralBranchVar(p, nodeRes.Primal, w.curLB, w.curUB)
		}
		if bVar == -1 {
			prevKind = nodeOutcomeUnsolved
			prevBound = nodeMin
			prevStatus = StatusNumericalError
			continue
		}

		cDown, cUp := createChildNodesWithOrbitState(
			p, &w.sym, b.deadline, b.done, &w.orbitBuf,
			nodeMin, node.depth, node, bVar, bVal, w.curLB, w.curUB, w.pCosts[bVar], &w.localNodeCounter,
		)
		var pushChild, nextPlunge *searchNode
		if bVal-math.Floor(bVal) >= halfFracTarget {
			nextPlunge = cUp
			pushChild = cDown
		} else {
			nextPlunge = cDown
			pushChild = cUp
		}
		w.plungeNode = nextPlunge
		c.publishBranch(w.id, pushChild, nextPlunge, prevMinPruned)
		prevMinPruned = math.Inf(1)
	}
}

func (b *babEngine) exploreTreeParallel(
	workers int,
	pCosts []pseudoCost,
	baseLB, baseUB []float64,
	rootPropConv bool,
	isContLinked []bool,
	rootCutsHelped bool,
	inTreePoolCap, inTreeBatchCap int,
	pruneRelTol, objConstMin float64,
	nodeCounter *int,
	lastIncumbentNode, heurMisses int,
) (Result, bool) {
	if b.plungeNode != nil {
		heap.Push(&b.pq, b.plungeNode)
		b.plungeNode = nil
	}
	_ = lastIncumbentNode
	if workers > b.pq.Len() {
		workers = b.pq.Len()
	}
	if workers < 2 {
		workers = 2
	}

	coord := &parCoordinator{
		b:                b,
		baseLB:           baseLB,
		baseUB:           baseUB,
		rootPropConv:     rootPropConv,
		isContLinked:     isContLinked,
		rootCutsHelped:   rootCutsHelped,
		inTreePoolCap:    inTreePoolCap,
		inTreeBatchCap:   inTreeBatchCap,
		pruneRelTol:      pruneRelTol,
		objConstMin:      objConstMin,
		pq:               b.pq,
		numWorkers:       workers,
		activeBounds:     make([]float64, workers),
		bestObjMin:       b.bestObjMin,
		bestPrimal:       b.bestPrimal,
		nodesExplored:    b.nodesExplored,
		unsolvedMinBound: b.unsolvedMinBound,
		prunedMinBound:   b.prunedMinBound,
		hitTimeLimit:     b.hitTimeLimit,
		hitIterLimit:     b.hitIterLimit,
		hitNodeLimit:     b.hitNodeLimit,
		hitLPLimitStatus: b.hitLPLimitStatus,
	}
	for i := range coord.activeBounds {
		coord.activeBounds[i] = math.Inf(1)
	}
	coord.cond = sync.NewCond(&coord.mu)
	coord.bestObjBits.Store(math.Float64bits(b.bestObjMin))
	coord.hasIncumbent.Store(b.bestPrimal != nil)
	coord.totalIters.Store(int64(b.totalLPIters))
	coord.cutsAdded.Store(int64(b.cutsAdded))

	baseCounter := *nodeCounter
	m := b.p.numConstraints
	queueCap := 0
	if b.opts.EnablePresolve {
		queueCap = min(rowQueueCapMult*m, initNodeQueueCap)
	}

	workerList := make([]parWorker, workers)
	for w := range workers {
		var lpS *LPSolver
		ownsLP := false
		if w == 0 {
			lpS = b.lpSolver
		} else {
			lpS = b.lpSolver.cloneForWorker()
			ownsLP = true
		}
		var propQ []int
		var propInQ []bool
		if b.opts.EnablePresolve {
			propQ = make([]int, 0, queueCap)
			propInQ = make([]bool, m)
		}
		workerList[w] = parWorker{
			id:               w,
			coord:            coord,
			lpSolver:         lpS,
			ownsLPSolver:     ownsLP,
			pCosts:           slices.Clone(pCosts),
			sym:              b.sym.cloneForWorker(),
			curLB:            slices.Clone(baseLB),
			curUB:            slices.Clone(baseUB),
			propQueue:        propQ,
			propInQueue:      propInQ,
			cutAges:          slices.Clone(b.cutAges),
			nodesSincePurge:  b.nodesSincePurge,
			inTreeCutMisses:  b.inTreeCutMisses,
			inTreeCutRounds:  b.inTreeCutRounds,
			localNodeCounter: baseCounter + (w+1)*100000,
			nodesProcessed:   b.nodesExplored,
			heurMisses:       heurMisses,
		}
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		wk := &workerList[w]
		go func() {
			defer wg.Done()
			wk.run()
		}()
	}
	wg.Wait()

	b.pq = coord.pq
	b.plungeNode = nil
	b.nodesExplored = coord.nodesExplored
	b.totalLPIters = int(coord.totalIters.Load())
	b.cutsAdded = int(coord.cutsAdded.Load())
	b.bestObjMin = coord.bestObjMin
	b.bestPrimal = coord.bestPrimal
	b.unsolvedMinBound = coord.unsolvedMinBound
	b.prunedMinBound = coord.prunedMinBound
	b.hitTimeLimit = coord.hitTimeLimit
	b.hitIterLimit = coord.hitIterLimit
	b.hitNodeLimit = coord.hitNodeLimit
	b.hitLPLimitStatus = coord.hitLPLimitStatus
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

// createChildNodesWithOrbit creates the down/up child pair and, when branching on a binary
// variable inside an active symmetry orbit, fixes the entire orbit to 0 on the down branch.
func (b *babEngine) createChildNodesWithOrbit(
	parentLB float64,
	parentDepth int,
	parent *searchNode,
	brVar int,
	brVal float64,
	curLB, curUB []float64,
	pc pseudoCost,
	counter *int,
) (downNode, upNode *searchNode) {
	return createChildNodesWithOrbitState(
		b.p, &b.sym, b.deadline, b.done, &b.orbitBuf,
		parentLB, parentDepth, parent, brVar, brVal, curLB, curUB, pc, counter,
	)
}

func createChildNodesWithOrbitState(
	p *CompiledProblem,
	sym *nodeSymmetry,
	deadline time.Time,
	done <-chan struct{},
	orbitBuf *[]int,
	parentLB float64,
	parentDepth int,
	parent *searchNode,
	brVar int,
	brVal float64,
	curLB, curUB []float64,
	pc pseudoCost,
	counter *int,
) (downNode, upNode *searchNode) {
	downNode, upNode = createChildNodes(parentLB, parentDepth+1, parent, brVar, brVal, curLB[brVar], curUB[brVar], pc, counter)
	if p.varCat[brVar] == Binary && curLB[brVar] == 0 && curUB[brVar] == 1 && downNode.delta.ub == 0 && upNode.delta.lb == 1 {
		*orbitBuf = sym.activeOrbit(p, curLB, curUB, brVar, parentDepth, deadline, done, *orbitBuf)
		if len(*orbitBuf) >= 2 {
			downNode.base = slices.Clip(downNode.base)
			for _, c := range *orbitBuf {
				if c != brVar {
					downNode.base = append(downNode.base, boundDelta{col: c, lb: 0, ub: 0})
				}
			}
			downNode.orbitalDown = true
		}
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
	return selectBranchVarCore(p, lpSolver, lpRes, curLB, curUB, pCosts, depth, totalIters, iterLeft, candsBuf, allowDomainRed, sbConfig{})
}

func selectBranchVarCore(
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
	cfg sbConfig,
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
		return sbOutcome{bestCol: bestCol, bestVal: bestVal, prunedBound: math.Inf(1)}
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
		return sbOutcome{bestCol: -1, prunedBound: math.Inf(1)}
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
		probeCutOff := func(st Status, bound float64) (cut bool, boundMin float64) {
			if !cfg.hasIncumbent || (st != StatusOptimal && st != StatusIterationLimit) || math.IsNaN(bound) {
				return false, math.Inf(1)
			}
			boundMin = bound + cfg.objConstMin
			cut = boundMin >= cfg.bestObjMin-cfg.absGapTol || isGapClosed(boundMin, cfg.bestObjMin, cfg.pruneRelTol, cfg.absGapTol)
			return cut, boundMin
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
				downCut, downMin := probeCutOff(downSt, downBound)
				downPruned := downInfeas || downCut
				upInfeas := upProbed && upSt == StatusInfeasible
				upCut, upMin := false, math.Inf(1)
				if upProbed {
					upCut, upMin = probeCutOff(upSt, upBound)
				}
				upPruned := upInfeas || upCut
				pBound := math.Inf(1)
				if downCut {
					pBound = min(pBound, downMin)
				}
				if upCut {
					pBound = min(pBound, upMin)
				}
				if downPruned && upPruned {
					lpSolver.restoreCheckpoint(sbCP)
					return sbOutcome{bestCol: -1, pruned: true, prunedBound: pBound}
				}
				// Apply single-branch bound-cutoff domain fixing only at shallow nodes (depth <= 1)
				// so deeper nodes do not stall strong branching with non-propagating bound-cutoff re-solves;
				// LP infeasibility always fixes immediately at every strong-branching depth.
				downFix := downInfeas || (downCut && depth <= 1)
				upFix := upInfeas || (upCut && depth <= 1)
				if downFix {
					curLB[col] = cl
					lpSolver.restoreCheckpoint(sbCP)
					if curLB[col] > curUB[col]+intFeasTol {
						return sbOutcome{bestCol: -1, pruned: true, prunedBound: pBound}
					}
					return sbOutcome{
						bestCol:     -1,
						prunedBound: pBound,
						fixedDeltas: []boundDelta{{col: col, lb: cl, ub: origU}},
					}
				}
				if upFix {
					var orbit []int
					if cfg.sym != nil && p.varCat[col] == Binary && origL == 0 && origU == 1 && fl == 0 {
						orbit = cfg.sym.activeOrbit(p, curLB, curUB, col, depth, cfg.deadline, cfg.done, nil)
					}
					curUB[col] = fl
					deltas := make([]boundDelta, 0, max(1, len(orbit)))
					deltas = append(deltas, boundDelta{col: col, lb: origL, ub: fl})
					for _, c := range orbit {
						if c == col {
							continue
						}
						curUB[c] = 0
						if curLB[c] > curUB[c]+intFeasTol {
							lpSolver.restoreCheckpoint(sbCP)
							return sbOutcome{bestCol: -1, pruned: true, prunedBound: pBound}
						}
						deltas = append(deltas, boundDelta{col: c, lb: 0, ub: 0})
					}
					lpSolver.restoreCheckpoint(sbCP)
					if curLB[col] > curUB[col]+intFeasTol {
						return sbOutcome{bestCol: -1, pruned: true, prunedBound: pBound}
					}
					return sbOutcome{
						bestCol:     -1,
						prunedBound: pBound,
						fixedDeltas: deltas,
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
	return sbOutcome{bestCol: bestCol, bestVal: bestVal, prunedBound: math.Inf(1)}
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
	return polishIncumbentWith(b.p, b.deadline, b.done, x, &b.fixScratchLB, &b.fixScratchUB, b.ensureHeurSolver)
}

func polishIncumbentWith(
	p *CompiledProblem,
	deadline time.Time,
	done <-chan struct{},
	x []float64,
	fixScratchLB, fixScratchUB *[]float64,
	ensureHeur func() *LPSolver,
) ([]float64, bool) {
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
	if len(p.intCols) < p.numVars && !stopRequested(deadline, done) && (bV > primalTol || rV > primalTol) {
		n := p.numVars
		if len(*fixScratchLB) < n {
			*fixScratchLB = make([]float64, n)
			*fixScratchUB = make([]float64, n)
		}
		fixLB := (*fixScratchLB)[:n]
		fixUB := (*fixScratchUB)[:n]
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
			hs := ensureHeur()
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
