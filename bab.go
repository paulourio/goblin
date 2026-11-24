package goblin

import (
	"cmp"
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
	reliabilityMaxDepth      = 5
	reliabilityMaxVars       = 250
	reliabilityMaxRows       = 200
	reliabilityDeepCandCap   = 2
	reliabilityDeepMaxIter   = 12
	reliabilityDeepRelCnt    = 1
	reliabilityCandScanCap   = 16
	sbBudgetBaseIters        = 400
	sbBudgetRatioNum         = 25
	sbBudgetRatioDen         = 100
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
	rcMinDualMag             = 1e-5
	rcDualNoiseRel           = 1e-10
	rcSafetyAbs              = 1e-4
	rcSafetyRel              = 1e-5
	rcMaxBoundMag            = 1e15
)

type boundDelta struct {
	col int
	lb  float64
	ub  float64
}

type searchNode struct {
	parent     *searchNode
	viprNode   *viprTreeNode
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

func searchNodeLess(ni, nj *searchNode) bool {
	if dl := ni.lowerBound - nj.lowerBound; dl > nodeQueueCmpTol || dl < -nodeQueueCmpTol {
		return dl < 0
	}
	if de := ni.estimate - nj.estimate; de > nodeQueueCmpTol || de < -nodeQueueCmpTol {
		return de < 0
	}
	return ni.depth > nj.depth
}

func (pq *nodeQueue) push(node *searchNode) {
	q := append(*pq, node)
	j := len(q) - 1
	for j > 0 {
		i := (j - 1) / 2
		if !searchNodeLess(q[j], q[i]) {
			break
		}
		q[i], q[j] = q[j], q[i]
		j = i
	}
	*pq = q
}

func (pq *nodeQueue) pop() *searchNode {
	q := *pq
	n := len(q) - 1
	item := q[0]
	if n > 0 {
		q[0] = q[n]
		q[n] = nil
		i := 0
		for {
			j1 := 2*i + 1
			if j1 >= n || j1 < 0 {
				break
			}
			j := j1
			if j2 := j1 + 1; j2 < n && searchNodeLess(q[j2], q[j1]) {
				j = j2
			}
			if !searchNodeLess(q[j], q[i]) {
				break
			}
			q[i], q[j] = q[j], q[i]
			i = j
		}
	} else {
		q[0] = nil
	}
	*pq = q[:n]
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

type branchSelectReq struct {
	p              *CompiledProblem
	lpSolver       *LPSolver
	lpRes          LPResult
	curLB, curUB   []float64
	pCosts         []pseudoCost
	depth          int
	totalIters     *int
	iterLeft       func() int
	candsBuf       *[]branchCand
	allowDomainRed bool
	sym            *nodeSymmetry
	deadline       time.Time
	done           <-chan struct{}
	hasIncumbent   bool
	bestObjMin     float64
	pruneRelTol    float64
	absGapTol      float64
	objConstMin    float64
	sbBudgetLeft   int
	strictExactGap bool
}

type parallelTreeReq struct {
	workers         int
	pCosts          []pseudoCost
	baseLB, baseUB  []float64
	rootPropConv    bool
	isContLinked    []bool
	rootCutsHelped  bool
	inTreePoolCap   int
	inTreeBatchCap  int
	pruneRelTol     float64
	objConstMin     float64
	baseNodeCounter int
	heurMisses      int
}

type polishIncumbentReq struct {
	p            *CompiledProblem
	exactIntRows []int
	deadline     time.Time
	done         <-chan struct{}
	x            []float64
	dualAmp      []float64
	fixScratchLB *[]float64
	fixScratchUB *[]float64
	ensureHeur   func() *LPSolver
	iterLeft     func() int
	addIters     func(int)
}

type sbOutcome struct {
	bestCol     int
	bestVal     float64
	pruned      bool
	prunedBound float64
	prunedCol   int
	prunedSplit float64
	downPruned  bool
	upPruned    bool
	fixedDeltas []boundDelta
	probeIters  int
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
	sbIters            int
	searchShift        float64
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
	rcScratch          []boundDelta
	maxObjAbs          float64
	rootDualAmp        []float64
	exactIntRows       []int
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
	vipr               babVIPRHook
}

type viprBoundCand struct {
	col     int
	isUpper bool
	bound   float64
}

type mipCutoffState struct {
	bestPrimal  []float64
	bestObjMin  float64
	searchShift float64
	hasCutoff   bool
}

type branchNodeReq struct {
	parentLB    float64
	parentDepth int
	parent      *searchNode
	brVar       int
	brVal       float64
	curLB       []float64
	curUB       []float64
	pc          pseudoCost
	counter     *int
}

type babVIPRHook interface {
	close()
	setDeadline(deadline time.Time, done <-chan struct{})
	finalize(res Result)
	rootNode() *viprTreeNode
	recordRootInfeasibleEqRow(p *CompiledProblem)
	newDisabledOrbitSubTrace() viprTraceHook
	adoptSubTraceInfeas(p *CompiledProblem, sub viprTraceHook)
	certifyRootInfeasible(s *LPSolver, lpRes LPResult)
	recordNodeDual(vn *viprTreeNode, s *LPSolver, lpRes LPResult)
	exactDualSearchMinFloor(vn *viprTreeNode, searchShift float64) (float64, bool)
	exactNodeGapClosed(vn *viprTreeNode, bestPrimal []float64) bool
	exactSubtreeGapClosed(vn *viprTreeNode, bestPrimal []float64) bool
	exactTreeGapClosed(bestPrimal []float64) bool
	tryTightenNodeDualForPrune(vn *viprTreeNode, bestPrimal []float64) (closed, infeas bool)
	certifyNodeFarkas(vn *viprTreeNode, s *LPSolver, lpRes LPResult)
	certifyRowActivityFarkas(vn *viprTreeNode, row int) bool
	canCertifyIncumbent(cand []float64, candMin float64) bool
	stageCertifiedCutRound(s *LPSolver, lpPrimal []float64, maxCuts int, minEff float64, ws *cutGenWorkspace) bool
	commitStagedCutPlanes(lpSolver *LPSolver, isRoot bool) int
	snapshotActiveCuts() int
	restoreActiveCuts(n int)
	purgeSlackCuts(s *LPSolver, slackTol float64) int
	certifyAndApplyRCF(
		vn *viprTreeNode,
		s *LPSolver,
		lpRes LPResult,
		cutoff mipCutoffState,
		origLB, origUB []float64,
		cands []boundDelta,
	) (nextVN *viprTreeNode, applied []boundDelta, pruned bool)
	certifyAndApplyUnboundedIntTightening(
		vn *viprTreeNode,
		cutoff mipCutoffState,
		cands []viprBoundCand,
	) (nextVN *viprTreeNode, changed, pruned bool)
	fillNodeBoundsFloat(vn *viprTreeNode, dstLB, dstUB []float64)
	propagateNode(vn *viprTreeNode, curLB, curUB []float64, maxPasses int) (feasible, converged bool)
	checkNodeBoundConflict(vn *viprTreeNode) bool
	recordSBPruned(vn *viprTreeNode, col int, splitFloat float64)
	recordSBFix(vn *viprTreeNode, col int, splitFloat float64, downPruned bool) *viprTreeNode
	createChildNodes(
		p *CompiledProblem,
		sym *nodeSymmetry,
		deadline time.Time,
		done <-chan struct{},
		req branchNodeReq,
		parentVN *viprTreeNode,
	) (downNode, upNode *searchNode)
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

func (b *babEngine) finishVIPR(res Result) Result {
	if b.vipr != nil {
		b.vipr.finalize(res)
	}
	return res
}

func (b *babEngine) evalSearchMin(x []float64) float64 {
	return evalLinearSum(b.searchShift, b.p.objMin, x, b.p.numVars)
}

func (b *babEngine) lpSearchMin(res LPResult) float64 {
	return lpMinWithShift(b.p, res, b.searchShift)
}

func (b *babEngine) userObjFromSearchMin(searchMin float64) float64 {
	if b.searchShift == 0 {
		return b.p.ObjectiveFromMin(searchMin)
	}
	return b.p.userObjFromShiftedMin(searchMin)
}

func (b *babEngine) gapClosed(lowerBound, upperBound, relTol float64) bool {
	return isGapClosed(lowerBound, upperBound, b.searchShift, relTol, b.opts.AbsGapTol)
}

func newHeurSolverFrom(p *CompiledProblem, lpSolver *LPSolver, deadline time.Time, done <-chan struct{}, disableSIMD bool) *LPSolver {
	canCopyRoot := lpSolver != nil && lpSolver.ws != nil && lpSolver.prob == p && lpSolver.dualValid && len(lpSolver.extraRows) == 0
	hs := newLPSolver(p, nil, !canCopyRoot)
	hs.Deadline = deadline
	hs.Done = done
	hs.DisableSIMD = disableSIMD
	hs.SkipExactFarkas = true
	if canCopyRoot {
		hs.CopyRootBasisFrom(lpSolver)
		if hs.scaled != lpSolver.scaled {
			hs.InitCrashBasis()
		}
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
	if opts.viprTrace != nil {
		opts.Workers = 1
	}
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
			BestBound: trivialDualBound(p.sense),
			SolveTime: time.Since(startTime),
		}
	}
	if (!opts.EnablePresolve && hasInfeasibleIntegerEqRow(p)) || (p.isMIP && p.exactOrig != nil && p.exactOrig.intEqInfeas) || hasEmptyInfeasibleRow(p) {
		if opts.viprTrace != nil {
			opts.viprTrace.recordRootInfeasibleEqRow(p)
		}
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
	if opts.viprTrace != nil {
		b.vipr = opts.viprTrace.newBabRecorder(p, opts.DisableSIMD)
		if b.vipr != nil {
			b.vipr.setDeadline(deadline, done)
		}
	}
	defer func() {
		b.heurSolver.Close()
		if b.vipr != nil {
			b.vipr.close()
		}
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
		if rootRes.Status == StatusInfeasible && b.vipr != nil {
			b.vipr.certifyRootInfeasible(b.lpSolver, rootRes)
		}
		bestBound := 0.0
		if isLimitStatus(rootRes.Status) {
			bestBound = trivialDualBound(p.sense)
		}
		return Result{
			modelID:    p.modelID,
			Status:     rootRes.Status,
			BestBound:  bestBound,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}

	rootLinMin := rootRes.ObjectiveMin
	rootShiftedMin := lpMinWithConst(p, rootRes)
	if math.IsNaN(rootLinMin) || math.IsInf(rootLinMin, 0) || math.IsNaN(rootShiftedMin) || math.IsInf(rootShiftedMin, 0) {
		return Result{
			modelID:    p.modelID,
			Status:     StatusNumericalError,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(startTime),
		}
	}
	if shouldUseObjConstShift(p, rootLinMin, rootShiftedMin) {
		b.searchShift = p.objConstMin()
		b.rootLPMin = rootShiftedMin
	} else {
		b.searchShift = 0
		b.rootLPMin = rootLinMin
	}
	b.rootLPUser = b.userObjFromSearchMin(b.rootLPMin)
	if b.vipr != nil {
		b.vipr.recordNodeDual(b.vipr.rootNode(), b.lpSolver, rootRes)
		if f, ok := b.vipr.exactDualSearchMinFloor(b.vipr.rootNode(), b.searchShift); ok && f > b.rootLPMin {
			b.rootLPMin = f
			b.rootLPUser = b.userObjFromSearchMin(b.rootLPMin)
		}
	}

	if !p.isMIP || len(p.intCols) == 0 {
		return b.finishVIPR(b.optimalOrNumErr(Result{
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
		}))
	}

	b.maxObjAbs = maxAbsFloat64(p.objMin)
	b.rootDualAmp = computeRootDualAmp(p.numConstraints, rootRes.Dual, b.maxObjAbs)
	b.exactIntRows = findExactIntRows(p)

	if opts.hasCutoff {
		b.bestObjMin = opts.cutoffMin + b.searchShift
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

	var rootVN *viprTreeNode
	if b.vipr != nil {
		rootVN = b.vipr.rootNode()
	}

	if isIntegerFeasible(p, rootRes.Primal) {
		if cand, ok := b.polishIncumbent(rootRes.Primal); ok {
			b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand))
			if b.rootGapClosed(rootVN, &b.rootLPMin) {
				b.rootLPUser = b.userObjFromSearchMin(b.rootLPMin)
				return b.finishVIPR(b.optimalRootResult(min(b.bestObjMin, b.rootLPMin), b.rootLPUser, 1, 0, true))
			}
		}
	}

	if opts.EnableHeuristics {
		b.ensureHeurSolver()
		b.runPreCutHeuristics(rootRes.Primal)
	}

	if b.rootGapClosed(rootVN, &b.rootLPMin) {
		b.rootLPUser = b.userObjFromSearchMin(b.rootLPMin)
		return b.finishVIPR(b.optimalRootResult(min(b.bestObjMin, b.rootLPMin), b.rootLPUser, b.nodesExplored, 0, true))
	}

	curLPRes := b.runRootCutLoop(rootRes)
	b.rootCutMin = b.lpSearchMin(curLPRes)
	if b.vipr != nil {
		if f, ok := b.vipr.exactDualSearchMinFloor(b.vipr.rootNode(), b.searchShift); ok && f > b.rootCutMin {
			b.rootCutMin = f
		}
	}
	b.rootCutUser = b.userObjFromSearchMin(b.rootCutMin)

	b.runPostCutHeuristics(curLPRes.Primal)
	if !opts.hasCutoff && b.searchShift == 0 && p.objConstant != 0 {
		cutShifted := lpMinWithConst(p, curLPRes)
		needUpgrade := shouldUseObjConstShift(p, curLPRes.ObjectiveMin, cutShifted)
		if !needUpgrade && b.bestPrimal != nil {
			incLin := evalLinearSum(0, p.objMin, b.bestPrimal, p.numVars)
			incShift := evalLinearSum(p.objConstMin(), p.objMin, b.bestPrimal, p.numVars)
			needUpgrade = math.Abs(incShift) < 0.5*math.Abs(incLin)
		}
		if needUpgrade && !isNonFinite(cutShifted) {
			b.searchShift = p.objConstMin()
			b.rootLPMin = rootShiftedMin
			if b.vipr != nil {
				if f, ok := b.vipr.exactDualSearchMinFloor(b.vipr.rootNode(), b.searchShift); ok && f > b.rootLPMin {
					b.rootLPMin = f
				}
			}
			b.rootLPUser = b.userObjFromSearchMin(b.rootLPMin)
			b.rootCutMin = cutShifted
			if b.vipr != nil {
				if f, ok := b.vipr.exactDualSearchMinFloor(b.vipr.rootNode(), b.searchShift); ok && f > b.rootCutMin {
					b.rootCutMin = f
				}
			}
			b.rootCutUser = b.userObjFromSearchMin(b.rootCutMin)
			if b.bestPrimal != nil {
				b.bestObjMin = b.evalSearchMin(b.bestPrimal)
			}
		}
	}
	if b.rootGapClosed(rootVN, &b.rootCutMin) {
		b.rootCutUser = b.userObjFromSearchMin(b.rootCutMin)
		return b.finishVIPR(b.optimalRootResult(min(b.bestObjMin, b.rootCutMin), b.rootCutUser, b.nodesExplored, b.cutsAdded, true))
	}

	if earlyRes, doneEarly := b.exploreTree(curLPRes); doneEarly {
		return b.finishVIPR(earlyRes)
	}
	return b.finishVIPR(b.finalizeResult())
}

// solveUnboundedMIPFeasibility resolves an LP-unbounded MILP by checking whether its integer hull is non-empty.
// By Meyer's theorem for rational MILPs, an LP-unbounded MILP is unbounded if and only if an integer-feasible point exists.
func (b *babEngine) solveUnboundedMIPFeasibility() Result {
	p := b.p
	if hasExactGCDInfeasibleEqRow(p) {
		if b.vipr != nil {
			b.vipr.recordRootInfeasibleEqRow(p)
		}
		return Result{
			modelID:    p.modelID,
			Status:     StatusInfeasible,
			Nodes:      b.nodesExplored,
			Iterations: b.totalLPIters,
			SolveTime:  time.Since(b.startTime),
		}
	}
	feasProb := p.zeroObjCopy()
	feasOpts := b.opts
	feasOpts.Workers = 1
	var subTrace viprTraceHook
	if b.vipr != nil {
		subTrace = b.vipr.newDisabledOrbitSubTrace()
		feasOpts.viprTrace = subTrace
	}
	if b.opts.MaxIter > 0 {
		rem := b.opts.MaxIter - b.totalLPIters
		if rem <= 0 {
			return Result{
				modelID:    p.modelID,
				Status:     StatusIterationLimit,
				BestBound:  trivialDualBound(p.sense),
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
				BestBound:  trivialDualBound(p.sense),
				Nodes:      b.nodesExplored,
				Iterations: b.totalLPIters,
				SolveTime:  time.Since(b.startTime),
			}
		}
		feasOpts.MaxNodes = remNodes
	}
	feasRes := solveBranchAndCutWithDone(feasProb, feasOpts, b.startTime, b.deadline, b.done)
	st := feasRes.Status
	if st == StatusOptimal {
		stop := exactStop{deadline: b.deadline, done: b.done}
		if _, okCert := extractUnboundedCertificateStop(stop, p, b.lpSolver, feasRes.Values); okCert {
			st = StatusUnbounded
		} else if b.timeExpired() {
			st = StatusTimeLimit
		} else {
			_, exactPureInt := exactPrimalIfFeasible(p, feasRes.Values, true)
			if p.numConstraints <= defaultExactPolishMaxNucleus || (len(p.intCols) == p.numVars && !exactPureInt) {
				st = StatusNumericalError
			} else {
				st = StatusUnbounded
			}
		}
	} else if st == StatusInfeasible && b.vipr != nil && subTrace != nil {
		b.vipr.adoptSubTraceInfeas(p, subTrace)
	}
	bestBound := 0.0
	if isLimitStatus(st) {
		bestBound = trivialDualBound(p.sense)
	}
	return Result{
		modelID:    p.modelID,
		Status:     st,
		BestBound:  bestBound,
		Nodes:      b.nodesExplored + feasRes.Nodes,
		Iterations: b.totalLPIters + feasRes.Iterations,
		CutsAdded:  feasRes.CutsAdded,
		SolveTime:  time.Since(b.startTime),
	}
}

func (b *babEngine) runFeasibilityPump(primal []float64, lbMin float64, hs *LPSolver, isContLinked []bool) {
	if b.opts.isSubMIP || b.p.numVars < fpMinVars || b.timeExpired() || b.gapClosed(lbMin, b.bestObjMin, b.opts.RelGapTol) {
		return
	}
	remIter := b.iterLeft()
	if remIter <= 0 {
		return
	}
	sol, objMin, usedIt, ok := tryFeasibilityPump(&feasPumpReq{
		p:             b.p,
		lpPrimal:      primal,
		curLB:         b.p.varLB,
		curUB:         b.p.varUB,
		heurSolver:    hs,
		bestObjMin:    b.bestObjMin,
		lowerBoundMin: lbMin,
		searchShift:   b.searchShift,
		relGapTol:     b.opts.RelGapTol,
		absGapTol:     b.opts.AbsGapTol,
		deadline:      b.deadline,
		done:          b.done,
		maxIterCap:    remIter,
		isContLinked:  isContLinked,
	})
	b.totalLPIters += usedIt
	if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
		b.hitIterLimit = true
	}
	if ok {
		b.acceptIncumbent(sol, objMin)
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
	sol, objMin, usedN, usedIt, ok := tryRootSubMIP(&rootSubMIPReq{
		p:            b.p,
		lpSolver:     hs,
		rootStatus:   rootStatus,
		lpPrimal:     primal,
		curLB:        b.p.varLB,
		curUB:        b.p.varUB,
		bestObjMin:   b.bestObjMin,
		searchShift:  b.searchShift,
		deadline:     b.deadline,
		isSubMIP:     b.opts.isSubMIP,
		maxNodesCap:  remNodes,
		maxIterCap:   remIter,
		isContLinked: isContLinked,
		enableCuts:   enableCuts,
	})
	b.nodesExplored += usedN
	b.totalLPIters += usedIt
	if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
		b.hitIterLimit = true
	}
	return ok && b.acceptIncumbent(sol, objMin)
}

func (b *babEngine) runPropagatedRounding(primal, curLB, curUB []float64, hs *LPSolver, isContLinked []bool) bool {
	remIter := b.iterLeft()
	if remIter <= 0 {
		return false
	}
	sol, objMin, usedIt, ok := tryPropagatedRounding(b.p, primal, curLB, curUB, hs, b.searchShift, isContLinked, remIter)
	b.totalLPIters += usedIt
	if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
		b.hitIterLimit = true
	}
	return ok && b.acceptIncumbent(sol, objMin)
}

func (b *babEngine) runFractionalDiving(primal []float64, hs *LPSolver, isContLinked []bool, onlyEqSupport bool) {
	remIter := b.iterLeft()
	if remIter <= 0 {
		return
	}
	usedIt := 0
	sol, objMin, ok := tryFractionalDiving(&divingReq{
		p:             b.p,
		rootPrimal:    primal,
		baseLB:        b.p.varLB,
		baseUB:        b.p.varUB,
		diveSolver:    hs,
		bestObjMin:    b.bestObjMin,
		searchShift:   b.searchShift,
		maxIterCap:    remIter,
		itersUsed:     &usedIt,
		onlyEqSupport: onlyEqSupport,
		isContLinked:  isContLinked,
	})
	b.totalLPIters += usedIt
	if b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter {
		b.hitIterLimit = true
	}
	if ok {
		b.acceptIncumbent(sol, objMin)
	}
}

func (b *babEngine) runPreCutHeuristics(rootPrimal []float64) {
	p := b.p
	isContLinked := b.contLinkedIntCols()
	b.heurSolver.CopyRootBasisFrom(b.lpSolver)
	if !b.timeExpired() {
		b.runPropagatedRounding(rootPrimal, p.varLB, p.varUB, b.heurSolver, isContLinked)
	}
	// Defer fractional diving and root sub-MIP until after root cuts when cutting is enabled.
	if b.cutsEnabled() {
		return
	}
	if !b.timeExpired() && !b.gapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol) {
		b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		b.runFractionalDiving(rootPrimal, b.heurSolver, isContLinked, true)
	}
	if !b.timeExpired() && !b.gapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol) {
		b.heurSolver.CopyRootBasisFrom(b.lpSolver)
		b.runFeasibilityPump(rootPrimal, b.rootLPMin, b.heurSolver, isContLinked)
	}
	if !b.timeExpired() && !b.gapClosed(b.rootLPMin, b.bestObjMin, b.opts.RelGapTol) {
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

// rootCutZeroGainTol returns the minimum root-bound improvement considered significant,
// scaling with RelGapTol when tight gap tolerances (such as 0 in certified mode) are active.
func (b *babEngine) rootCutZeroGainTol(base float64) float64 {
	effGainRel := zeroGainRelTol
	if b.opts.RelGapTol < effGainRel {
		effGainRel = max(1e-12, 0.1*b.opts.RelGapTol)
	}
	return max(zeroGainAbsTol, effGainRel*math.Abs(base))
}

func (b *babEngine) strictExactVIPRGap() bool {
	return b.vipr != nil && b.opts.RelGapTol == 0 && b.opts.AbsGapTol == 0
}

func (b *babEngine) rootGapClosed(rootVN *viprTreeNode, boundMin *float64) bool {
	if b.bestPrimal == nil {
		return false
	}
	if b.strictExactVIPRGap() {
		prune, _ := b.shouldPruneNode(rootVN, boundMin, b.opts.RelGapTol)
		return prune
	}
	return b.gapClosed(*boundMin, b.bestObjMin, b.opts.RelGapTol) ||
		(b.vipr != nil && b.vipr.exactNodeGapClosed(rootVN, b.bestPrimal))
}

func (b *babEngine) shouldPruneNode(vn *viprTreeNode, nodeMin *float64, pruneRelTol float64) (prune, infeas bool) {
	floatPrune := *nodeMin >= b.bestObjMin-b.opts.AbsGapTol || b.gapClosed(*nodeMin, b.bestObjMin, pruneRelTol)
	if b.strictExactVIPRGap() {
		if b.bestPrimal == nil {
			return false, false
		}
		if b.vipr.exactSubtreeGapClosed(vn, b.bestPrimal) {
			if f, ok := b.vipr.exactDualSearchMinFloor(vn, b.searchShift); ok && f > *nodeMin {
				*nodeMin = f
			}
			return true, false
		}
		nearCutoff := floatPrune || *nodeMin >= b.bestObjMin-1e-9*max(1.0, math.Abs(b.bestObjMin))
		if !nearCutoff {
			return false, false
		}
		closed, inf := b.vipr.tryTightenNodeDualForPrune(vn, b.bestPrimal)
		if inf {
			return false, true
		}
		if closed {
			if f, ok := b.vipr.exactDualSearchMinFloor(vn, b.searchShift); ok && f > *nodeMin {
				*nodeMin = f
			}
			return true, false
		}
		return false, false
	}
	if !floatPrune && (b.vipr == nil || !b.vipr.exactNodeGapClosed(vn, b.bestPrimal)) {
		return false, false
	}
	return true, false
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
	anchorCutSnap := 0
	anchorLPRes := curLPRes
	anchorCuts := 0

	for range b.opts.MaxCutPasses {
		if b.timeExpired() {
			break
		}
		curMin := b.lpSearchMin(curLPRes)
		if curMin >= b.bestObjMin-b.opts.AbsGapTol || b.gapClosed(curMin, b.bestObjMin, b.opts.RelGapTol) {
			break
		}
		var newCuts []cutPlane
		if b.vipr != nil {
			if !b.vipr.stageCertifiedCutRound(lpSolver, curLPRes.Primal, maxCutsPerRound, minCutEfficacy, &b.cutWS) || b.timeExpired() {
				break
			}
		} else {
			newCuts = generateCutRound(lpSolver, curLPRes.Primal, maxCutsPerRound, &b.cutWS)
			if len(newCuts) == 0 || b.timeExpired() {
				break
			}
		}
		if !b.capLPIter(lpSolver, b.defaultLPIter) {
			b.hitIterLimit = true
			break
		}
		prevObj := curMin
		// Take one snapshot per round. Share immutable slices when still at anchor.
		curSnap := lpSolver.snapshot()
		var curCutSnap int
		if b.vipr != nil {
			curCutSnap = b.vipr.snapshotActiveCuts()
		}
		if b.cutsAdded == anchorCuts {
			anchorSnap = curSnap
			anchorCutSnap = curCutSnap
		}
		var addedBatch int
		if b.vipr != nil {
			addedBatch = b.vipr.commitStagedCutPlanes(lpSolver, true)
		} else {
			addedBatch = addDynamicCutPlanes(lpSolver, newCuts)
		}
		if addedBatch == 0 {
			break
		}
		nextRes := lpSolver.Solve()
		b.totalLPIters += nextRes.Iterations
		nextMin := b.lpSearchMin(nextRes)
		if nextRes.Status != StatusOptimal || math.IsNaN(nextMin) || math.IsInf(nextMin, 0) {
			// Valid cuts never make a feasible LP infeasible, and a failed re-solve
			// leaves no usable basis. Roll the round back so branch-and-bound starts
			// from the last optimal LP state.
			lpSolver.restore(curSnap)
			if b.vipr != nil {
				b.vipr.restoreActiveCuts(curCutSnap)
			}
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
		if b.vipr != nil {
			b.vipr.recordNodeDual(b.vipr.rootNode(), lpSolver, nextRes)
		}

		if isIntegerFeasible(p, curLPRes.Primal) {
			if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
				b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand))
				if b.bestPrimal != nil && !b.strictExactVIPRGap() && b.gapClosed(nextMin, b.bestObjMin, b.opts.RelGapTol) {
					anchorLPRes = curLPRes
					anchorCuts = b.cutsAdded
					break
				}
			}
		}

		zeroGainTol := b.rootCutZeroGainTol(prevObj)
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
	curEndMin := b.lpSearchMin(curLPRes)
	anchorEndMin := b.lpSearchMin(anchorLPRes)
	if curEndMin-anchorEndMin <= b.rootCutZeroGainTol(anchorEndMin) && b.cutsAdded > anchorCuts {
		lpSolver.restore(anchorSnap)
		if b.vipr != nil {
			b.vipr.restoreActiveCuts(anchorCutSnap)
		}
		curLPRes = anchorLPRes
		b.cutsAdded = anchorCuts
	}
	if len(lpSolver.extraRows) > 0 {
		if b.vipr != nil {
			b.cutsAdded -= b.vipr.purgeSlackCuts(lpSolver, slackCutPurgeTol)
		} else {
			b.cutsAdded -= lpSolver.PurgeSlackCuts(slackCutPurgeTol)
		}
	}
	return curLPRes
}

func (b *babEngine) runPostCutHeuristics(cutPrimal []float64) {
	p := b.p
	if !b.opts.EnableHeuristics || !b.cutsEnabled() || b.timeExpired() || b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
		return
	}
	cutsImproved := b.cutsAdded > 0 && b.rootCutMin-b.rootLPMin > b.rootCutZeroGainTol(b.rootLPMin)
	hs := b.ensureHeurSolver()
	isContLinked := b.contLinkedIntCols()
	if !cutsImproved {
		// Cuts did not tighten the root relaxation, so run deferred diving and the
		// feasibility pump before resorting to a root sub-MIP.
		hs.CopyRootBasisFrom(b.lpSolver)
		b.runFractionalDiving(cutPrimal, hs, isContLinked, true)
		if !b.timeExpired() && !b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
			hs.CopyRootBasisFrom(b.lpSolver)
			b.runFeasibilityPump(cutPrimal, b.rootCutMin, hs, isContLinked)
		}
		if !b.timeExpired() && !b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
			hs.CopyRootBasisFrom(b.lpSolver)
			b.runRootSubMIP(cutPrimal, hs, isContLinked, false)
		}
		return
	}
	if b.runRootSubMIP(cutPrimal, hs, isContLinked, true) {
		b.postCutSubMIPFound = true
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
		b.runPropagatedRounding(cutPrimal, p.varLB, p.varUB, hs, isContLinked)
	}
	if !b.timeExpired() && !b.postCutSubMIPFound && !b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
		b.runFractionalDiving(cutPrimal, hs, isContLinked, p.numVars > postCutDiveVarMin)
	}
	if b.bestPrimal == nil && !b.timeExpired() && !b.gapClosed(b.rootCutMin, b.bestObjMin, b.opts.RelGapTol) {
		b.runFeasibilityPump(cutPrimal, b.rootCutMin, hs, isContLinked)
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
	updateCutAgesState(b.p, b.lpSolver, b.vipr, &b.cutAges, &b.nodesSincePurge, allowPurge)
}

func updateCutAgesState(p *CompiledProblem, s *LPSolver, vipr babVIPRHook, cutAges *[]int, nodesSincePurge *int, allowPurge bool) {
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
		var purged int
		if vipr != nil {
			purged = vipr.purgeSlackCuts(s, inTreeCutSlackTol)
		} else {
			purged = s.PurgeSlackCuts(inTreeCutSlackTol)
		}
		if purged > 0 {
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
	var newCuts []cutPlane
	if b.vipr != nil {
		if !b.vipr.stageCertifiedCutRound(b.lpSolver, nodeRes.Primal, batchCap, inTreeCutMinEff, &b.cutWS) {
			b.inTreeCutMisses++
			return false, false
		}
	} else {
		newCuts = generateInTreeCutRound(b.lpSolver, nodeRes.Primal, batchCap, inTreeCutMinEff, &b.cutWS)
		if len(newCuts) == 0 {
			b.inTreeCutMisses++
			return false, false
		}
	}
	if !b.capLPIter(b.lpSolver, b.defaultLPIter) {
		node.lowerBound = max(node.lowerBound, *nodeMin)
		b.pq.push(node)
		b.hitIterLimit = true
		return false, true
	}
	snap := b.lpSolver.snapshot()
	var cutSnap int
	if b.vipr != nil {
		cutSnap = b.vipr.snapshotActiveCuts()
	}
	var added int
	if b.vipr != nil {
		added = b.vipr.commitStagedCutPlanes(b.lpSolver, false)
	} else {
		added = addDynamicCutPlanes(b.lpSolver, newCuts)
	}
	if added == 0 {
		b.inTreeCutMisses++
		return false, false
	}
	nextRes := b.lpSolver.Solve()
	b.totalLPIters += nextRes.Iterations
	if nextRes.Status == StatusInfeasible {
		if b.vipr != nil {
			b.vipr.certifyNodeFarkas(node.viprNode, b.lpSolver, nextRes)
		}
		b.cutsAdded += added
		b.inTreeCutRounds++
		b.inTreeCutMisses = 0
		return true, false
	}
	if nextRes.Status == StatusTimeLimit || (nextRes.Status == StatusIterationLimit && b.budgetBound(b.lpSolver)) {
		b.lpSolver.restore(snap)
		if b.vipr != nil {
			b.vipr.restoreActiveCuts(cutSnap)
		}
		node.lowerBound = max(node.lowerBound, *nodeMin)
		b.pq.push(node)
		if nextRes.Status == StatusTimeLimit {
			b.hitTimeLimit = true
		} else {
			b.hitIterLimit = true
		}
		return false, true
	}
	nextMin := b.lpSearchMin(nextRes)
	gainTol := max(inTreeCutMinGainAbs, inTreeCutMinGainRel*math.Abs(*nodeMin))
	if nextRes.Status != StatusOptimal || math.IsNaN(nextMin) || math.IsInf(nextMin, 0) || nextMin-*nodeMin <= gainTol {
		b.lpSolver.restore(snap)
		if b.vipr != nil {
			b.vipr.restoreActiveCuts(cutSnap)
		}
		b.inTreeCutMisses++
		return false, false
	}
	b.cutsAdded += added
	b.inTreeCutRounds++
	b.inTreeCutMisses = 0
	*nodeRes = nextRes
	*nodeMin = max(*nodeMin, nextMin)
	if b.vipr != nil {
		b.vipr.recordNodeDual(node.viprNode, b.lpSolver, nextRes)
	}
	if prune, inf := b.shouldPruneNode(node.viprNode, nodeMin, pruneRelTol); prune || inf {
		if prune {
			b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
		}
		return true, false
	}
	if isIntegerFeasible(b.p, nodeRes.Primal) {
		if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
			if b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
				*lastIncumbentNode = b.nodesExplored
				*heurMisses = 0
			}
			if prune, inf := b.shouldPruneNode(node.viprNode, nodeMin, pruneRelTol); prune || inf {
				if prune {
					b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
				}
				return true, false
			}
		}
	}
	return false, false
}

type postFixOutcome uint8

const (
	postFixContinue postFixOutcome = iota
	postFixPruned
	postFixLimitStop
)

func (b *babEngine) optimalRootResult(boundMin, rootCutBound float64, nodes, cutsAdded int, clampBound bool) Result {
	obj := b.p.EvaluateObjective(b.bestPrimal)
	bestBound := b.userObjFromSearchMin(boundMin)
	if clampBound {
		bestBound = clampBestBoundToObj(b.p.sense, bestBound, obj)
	}
	return b.optimalOrNumErr(Result{
		modelID:      b.p.modelID,
		Status:       StatusOptimal,
		Objective:    obj,
		BestBound:    bestBound,
		RootLPBound:  b.rootLPUser,
		RootCutBound: rootCutBound,
		Values:       b.bestPrimal,
		Nodes:        nodes,
		Iterations:   b.totalLPIters,
		CutsAdded:    cutsAdded,
		SolveTime:    time.Since(b.startTime),
	})
}

func (b *babEngine) cutoffState() mipCutoffState {
	return mipCutoffState{
		bestPrimal:  b.bestPrimal,
		bestObjMin:  b.bestObjMin,
		searchShift: b.searchShift,
		hasCutoff:   b.opts.hasCutoff,
	}
}

func (b *babEngine) runNodeRCF(vn **viprTreeNode, lpRes LPResult, lpMin float64, lb, ub []float64) bool {
	var rcInfeas bool
	if b.vipr == nil {
		b.rcScratch, rcInfeas = applyReducedCostFixing(b.p, lpRes, lpMin, b.bestObjMin, b.maxObjAbs, &b.sym, lb, ub, b.rcScratch[:0])
	} else {
		candLB := slices.Clone(lb)
		candUB := slices.Clone(ub)
		b.rcScratch, _ = applyReducedCostFixing(b.p, lpRes, lpMin, b.bestObjMin, b.maxObjAbs, nil, candLB, candUB, b.rcScratch[:0])
		*vn, b.rcScratch, rcInfeas = b.vipr.certifyAndApplyRCF(
			*vn, b.lpSolver, lpRes, b.cutoffState(), lb, ub, b.rcScratch,
		)
	}
	if len(b.rcScratch) > 0 || rcInfeas {
		b.prunedMinBound = min(b.prunedMinBound, b.bestObjMin)
	}
	return rcInfeas
}

func (b *babEngine) resolveRootLP(
	rootVN *viprTreeNode,
	curLB, curUB []float64,
	curLPRes *LPResult,
	rootNodeMin *float64,
) (earlyRes Result, doneEarly, abort bool) {
	reRes, stop := solveNodeLP(b.lpSolver, curLB, curUB, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters)
	if stop != StatusNotSolved {
		if stop == StatusTimeLimit {
			b.hitTimeLimit = true
		} else {
			b.hitIterLimit = true
		}
		b.unsolvedMinBound = *rootNodeMin
		return Result{}, false, true
	}
	if reRes.Status == StatusInfeasible {
		if b.vipr != nil {
			b.vipr.certifyNodeFarkas(rootVN, b.lpSolver, reRes)
		}
		return Result{}, false, true
	}
	if reRes.Status != StatusOptimal {
		b.unsolvedMinBound = *rootNodeMin
		if b.hitLPLimitStatus == StatusNotSolved {
			b.hitLPLimitStatus = reRes.Status
		}
		return Result{}, false, true
	}
	reMin := b.lpSearchMin(reRes)
	if math.IsNaN(reMin) || math.IsInf(reMin, 0) {
		b.unsolvedMinBound = *rootNodeMin
		b.hitLPLimitStatus = StatusNumericalError
		return Result{}, false, true
	}
	*curLPRes = reRes
	*rootNodeMin = max(*rootNodeMin, reMin)
	if b.vipr != nil {
		b.vipr.recordNodeDual(rootVN, b.lpSolver, reRes)
	}
	if isIntegerFeasible(b.p, curLPRes.Primal) {
		if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
			b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand))
		}
	}
	if b.rootGapClosed(rootVN, rootNodeMin) {
		return b.optimalRootResult(min(b.bestObjMin, *rootNodeMin), b.rootCutUser, b.nodesExplored, b.cutsAdded, false), true, true
	}
	if !b.strictExactVIPRGap() && *rootNodeMin >= b.bestObjMin-b.opts.AbsGapTol {
		b.prunedMinBound = min(b.prunedMinBound, *rootNodeMin)
		return Result{}, false, true
	}
	return Result{}, false, false
}

func (b *babEngine) resolveNodeAfterFix(
	node *searchNode,
	curLB, curUB []float64,
	nodeRes *LPResult,
	nodeMin *float64,
	pruneRelTol float64,
	lastIncumbentNode, heurMisses *int,
) postFixOutcome {
	reRes, reMin, stop, lpSt, infeas, fail := resolveNodeAfterBoundFix(
		b.p, b.lpSolver, curLB, curUB, b.searchShift, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters,
	)
	if stop != StatusNotSolved {
		node.lowerBound = max(node.lowerBound, *nodeMin)
		b.pq.push(node)
		if stop == StatusTimeLimit {
			b.hitTimeLimit = true
		} else {
			b.hitIterLimit = true
		}
		return postFixLimitStop
	}
	if infeas {
		if b.vipr != nil {
			b.vipr.certifyNodeFarkas(node.viprNode, b.lpSolver, reRes)
		}
		return postFixPruned
	}
	if fail {
		b.unsolvedMinBound = min(b.unsolvedMinBound, *nodeMin)
		if b.hitLPLimitStatus == StatusNotSolved {
			b.hitLPLimitStatus = lpSt
		}
		return postFixPruned
	}
	*nodeRes = reRes
	*nodeMin = max(*nodeMin, reMin)
	if b.vipr != nil {
		b.vipr.recordNodeDual(node.viprNode, b.lpSolver, reRes)
	}
	if prune, inf := b.shouldPruneNode(node.viprNode, nodeMin, pruneRelTol); prune || inf {
		if prune {
			b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
		}
		return postFixPruned
	}
	if isIntegerFeasible(b.p, nodeRes.Primal) {
		if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
			if b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
				*lastIncumbentNode = b.nodesExplored
				*heurMisses = 0
			}
			if prune, inf := b.shouldPruneNode(node.viprNode, nodeMin, pruneRelTol); prune || inf {
				if prune {
					b.prunedMinBound = min(b.prunedMinBound, *nodeMin)
				}
				return postFixPruned
			}
		}
	}
	return postFixContinue
}

func applyAndPropagateNodeBounds(
	p *CompiledProblem,
	node *searchNode,
	fromPlunge, enablePresolve, rootPropConv bool,
	baseLB, baseUB, curLB, curUB []float64,
	touchedCols, propQueue *[]int,
	propInQueue []bool,
) bool {
	if fromPlunge && node.hasDelta && len(node.base) == 0 {
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
			return false
		}
		if enablePresolve && tightened {
			if rootPropConv {
				return propagateFromCol(p, curLB, curUB, d.col, propQueue, propInQueue)
			}
			return propagateBounds(p, curLB, curUB, nodePropPasses)
		}
		return true
	}
	copy(curLB, baseLB)
	copy(curUB, baseUB)
	var boundsFeas bool
	*touchedCols, boundsFeas = applyNodeBoundsTracked(node, curLB, curUB, (*touchedCols)[:0])
	if !boundsFeas {
		return false
	}
	if enablePresolve && len(*touchedCols) > 0 {
		if rootPropConv {
			return propagateFromCols(p, curLB, curUB, *touchedCols, propQueue, propInQueue)
		}
		return propagateBounds(p, curLB, curUB, nodePropPasses)
	}
	return true
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
	b.sym = initNodeSymmetry(p)
	var symPtr *nodeSymmetry
	var rootVN *viprTreeNode
	if b.vipr == nil {
		symPtr = &b.sym
	} else {
		rootVN = b.vipr.rootNode()
	}

	baseLB := slices.Clone(p.varLB)
	baseUB := slices.Clone(p.varUB)
	if b.bestPrimal != nil {
		if b.runNodeRCF(&rootVN, curLPRes, b.rootCutMin, baseLB, baseUB) {
			return Result{}, false
		}
		if b.vipr != nil && len(b.rcScratch) > 0 {
			b.vipr.fillNodeBoundsFloat(rootVN, baseLB, baseUB)
		}
	}
	if b.vipr == nil {
		tightenUnboundedIntCols(p, baseLB, baseUB, b.bestObjMin, b.searchShift)
	} else if !math.IsInf(b.bestObjMin, 1) && (b.bestPrimal != nil || b.opts.hasCutoff) {
		hasInfInt := false
		for _, col := range p.intCols {
			if math.IsInf(baseLB[col], -1) || math.IsInf(baseUB[col], 1) {
				hasInfInt = true
				break
			}
		}
		if hasInfInt {
			candLB := slices.Clone(baseLB)
			candUB := slices.Clone(baseUB)
			tightenUnboundedIntCols(p, candLB, candUB, b.bestObjMin, b.searchShift)
			var unbCands []viprBoundCand
			for _, col := range p.intCols {
				if math.IsInf(baseUB[col], 1) && !math.IsInf(candUB[col], 1) {
					unbCands = append(unbCands, viprBoundCand{col: col, isUpper: true, bound: candUB[col]})
				}
				if math.IsInf(baseLB[col], -1) && !math.IsInf(candLB[col], -1) {
					unbCands = append(unbCands, viprBoundCand{col: col, isUpper: false, bound: candLB[col]})
				}
			}
			var unbChanged, unbPruned bool
			rootVN, unbChanged, unbPruned = b.vipr.certifyAndApplyUnboundedIntTightening(
				rootVN, b.cutoffState(), unbCands,
			)
			if unbChanged || unbPruned {
				b.prunedMinBound = min(b.prunedMinBound, b.bestObjMin)
			}
			if unbPruned {
				return Result{}, false
			}
			if unbChanged {
				b.vipr.fillNodeBoundsFloat(rootVN, baseLB, baseUB)
			}
		}
	}

	rootPropConv := false
	if b.vipr != nil {
		if b.opts.EnablePresolve {
			var rootFeas bool
			rootFeas, rootPropConv = b.vipr.propagateNode(rootVN, baseLB, baseUB, rootPropPasses)
			if !rootFeas {
				return Result{}, false
			}
		} else if b.vipr.checkNodeBoundConflict(rootVN) {
			return Result{}, false
		}
	} else if b.opts.EnablePresolve {
		var rootFeas bool
		rootFeas, rootPropConv = propagateBoundsChecked(p, baseLB, baseUB, rootPropPasses)
		if !rootFeas {
			return Result{}, false
		}
		b.ensurePropBuffers()
	}

	curLB := slices.Clone(baseLB)
	curUB := slices.Clone(baseUB)
	if row, conflict := hasExactIntRowActivityConflict(p, b.exactIntRows, curLB, curUB); conflict {
		if b.vipr == nil || b.vipr.certifyRowActivityFarkas(rootVN, row) {
			return Result{}, false
		}
	}

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

	// Re-solve the root LP if reduced-cost fixing, unbounded-column tightening,
	// or root bound propagation cut off curLPRes.Primal.
	if boundsCutPrimal(curLPRes.Primal, curLB, curUB, n) {
		if earlyRes, doneEarly, abort := b.resolveRootLP(rootVN, curLB, curUB, &curLPRes, &rootNodeMin); abort {
			return earlyRes, doneEarly
		}
	}
	// Cap per-node pruning relative tolerance at 1.0 so upper - r*max(1,|upper|) remains
	// monotonically non-decreasing when bestObjMin improves; when RelGapTol > 1.0, check the
	// global bound against the full RelGapTol directly.
	pruneRelTol := min(b.opts.RelGapTol, 1.0)
	objConstMin := b.searchShift
	var brVar int
	var brVal float64
	rootSBRounds := 0
	for {
		sb := selectBranchVar(&branchSelectReq{
			p:              p,
			lpSolver:       b.lpSolver,
			lpRes:          curLPRes,
			curLB:          curLB,
			curUB:          curUB,
			pCosts:         pCosts,
			depth:          0,
			totalIters:     &b.totalLPIters,
			iterLeft:       b.iterLeft,
			candsBuf:       &b.sbCands,
			allowDomainRed: rootSBRounds < maxSBDomainRedRounds,
			sym:            symPtr,
			deadline:       b.deadline,
			done:           b.done,
			hasIncumbent:   b.bestPrimal != nil,
			bestObjMin:     b.bestObjMin,
			pruneRelTol:    pruneRelTol,
			absGapTol:      b.opts.AbsGapTol,
			objConstMin:    objConstMin,
			strictExactGap: b.strictExactVIPRGap(),
		})
		b.sbIters += sb.probeIters
		b.prunedMinBound = min(b.prunedMinBound, sb.prunedBound)
		if sb.pruned {
			if b.vipr != nil {
				b.vipr.recordSBPruned(rootVN, sb.prunedCol, sb.prunedSplit)
			}
			return Result{}, false
		}
		if len(sb.fixedDeltas) == 0 {
			brVar, brVal = sb.bestCol, sb.bestVal
			break
		}
		rootSBRounds++
		if b.vipr != nil {
			rootVN = b.vipr.recordSBFix(rootVN, sb.prunedCol, sb.prunedSplit, sb.downPruned)
		}
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
		if b.vipr != nil {
			b.vipr.fillNodeBoundsFloat(rootVN, baseLB, baseUB)
			if b.vipr.checkNodeBoundConflict(rootVN) {
				return Result{}, false
			}
			if b.opts.EnablePresolve {
				if propOK, _ := b.vipr.propagateNode(rootVN, baseLB, baseUB, nodePropPasses); !propOK {
					return Result{}, false
				}
			}
			copy(curLB, baseLB)
			copy(curUB, baseUB)
		} else if b.opts.EnablePresolve {
			var propOK bool
			if rootPropConv {
				propOK = propagateFromCols(p, baseLB, baseUB, b.touchedCols, &b.propQueue, b.propInQueue)
			} else {
				propOK = propagateBounds(p, baseLB, baseUB, nodePropPasses)
			}
			if !propOK {
				return Result{}, false
			}
			copy(curLB, baseLB)
			copy(curUB, baseUB)
		}
		if earlyRes, doneEarly, abort := b.resolveRootLP(rootVN, curLB, curUB, &curLPRes, &rootNodeMin); abort {
			return earlyRes, doneEarly
		}
	}

	if brVar == -1 {
		if cand, ok := b.polishIncumbent(curLPRes.Primal); ok {
			b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand))
			if b.rootGapClosed(rootVN, &rootNodeMin) {
				return b.optimalRootResult(min(b.bestObjMin, rootNodeMin), b.rootCutUser, b.nodesExplored, b.cutsAdded, false), true
			}
		}
		brVar, brVal = selectNearIntegralBranchVar(p, curLPRes.Primal, curLB, curUB)
	}
	if brVar == -1 {
		leafRes, leafMin, leafInfeas, leafOK := resolveFixedIntLeaf(
			p, curLB, curUB, b.exactIntRows, b.searchShift, b.deadline, b.done, b.ensureHeurSolver, b.iterLeft(), b.vipr, rootVN,
		)
		b.totalLPIters += leafRes.Iterations
		if leafOK {
			if leafInfeas {
				return Result{}, false
			}
			rootNodeMin = max(rootNodeMin, leafMin)
			if cand, ok := b.polishIncumbent(leafRes.Primal); ok {
				b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand))
			}
			if prune, inf := b.shouldPruneNode(rootVN, &rootNodeMin, b.opts.RelGapTol); prune || inf {
				if prune && b.bestPrimal != nil {
					return b.optimalRootResult(min(b.bestObjMin, rootNodeMin), b.rootCutUser, b.nodesExplored, b.cutsAdded, false), true
				}
				return Result{}, false
			}
		}
		b.unsolvedMinBound = rootNodeMin
		switch {
		case !leafOK && (leafRes.Status == StatusTimeLimit || b.timeExpired()):
			b.hitTimeLimit = true
		case !leafOK && (leafRes.Status == StatusIterationLimit || (b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter)):
			b.hitIterLimit = true
		case b.timeExpired():
			b.hitTimeLimit = true
		case b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter:
			b.hitIterLimit = true
		default:
			b.hitLPLimitStatus = StatusNumericalError
		}
		return Result{}, false
	}

	nodeCounter := 1
	downNode, upNode := b.createChildNodesWithOrbit(branchNodeReq{
		parentLB:    rootNodeMin,
		parentDepth: 0,
		parent:      nil,
		brVar:       brVar,
		brVal:       brVal,
		curLB:       curLB,
		curUB:       curUB,
		pc:          pCosts[brVar],
		counter:     &nodeCounter,
	}, rootVN)
	b.pq.push(downNode)
	b.pq.push(upNode)

	lastIncumbentNode := 1
	heurMisses := 0
	rootCutsHelped := b.cutsAdded > 0 && b.rootCutMin-b.rootLPMin > b.rootCutZeroGainTol(b.rootLPMin)
	inTreePoolCap := min(inTreeCutMaxPoolCap, max(inTreeCutMinPoolCap, p.numConstraints))
	inTreeBatchCap := min(inTreeCutMaxBatch, max(inTreeCutMinBatch, p.numConstraints/inTreeCutBatchDiv))
	var isContLinked []bool
	if b.opts.EnableHeuristics {
		isContLinked = b.contLinkedIntCols()
	}
	for len(b.pq) > 0 || b.plungeNode != nil {
		if b.vipr != nil && b.vipr.exactNodeGapClosed(b.vipr.rootNode(), b.bestPrimal) {
			break
		}
		if b.opts.RelGapTol > 1.0 && b.bestPrimal != nil && b.gapClosed(b.currentGlobalBound(), b.bestObjMin, b.opts.RelGapTol) {
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
			openCount := len(b.pq)
			if b.plungeNode != nil {
				openCount++
			}
			workers := min(b.opts.Workers, maxParallelWorkers)
			rampTarget := min(max(2*workers, 8), 32)
			if openCount >= rampTarget {
				return b.exploreTreeParallel(&parallelTreeReq{
					workers:         workers,
					pCosts:          pCosts,
					baseLB:          baseLB,
					baseUB:          baseUB,
					rootPropConv:    rootPropConv,
					isContLinked:    isContLinked,
					rootCutsHelped:  rootCutsHelped,
					inTreePoolCap:   inTreePoolCap,
					inTreeBatchCap:  inTreeBatchCap,
					pruneRelTol:     pruneRelTol,
					objConstMin:     objConstMin,
					baseNodeCounter: nodeCounter,
					heurMisses:      heurMisses,
				})
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
				b.pq.push(b.plungeNode)
				b.plungeNode = nil
			}
			node = b.pq.pop()
		}

		if (node.lowerBound >= b.bestObjMin-b.opts.AbsGapTol || b.gapClosed(node.lowerBound, b.bestObjMin, pruneRelTol)) &&
			(b.vipr == nil || (!b.strictExactVIPRGap() && b.bestPrimal != nil) || b.vipr.exactSubtreeGapClosed(node.viprNode, b.bestPrimal)) {
			b.prunedMinBound = min(b.prunedMinBound, node.lowerBound)
			continue
		}

		b.nodesExplored++

		if b.vipr != nil {
			b.vipr.fillNodeBoundsFloat(node.viprNode, curLB, curUB)
			if b.vipr.checkNodeBoundConflict(node.viprNode) {
				continue
			}
			if b.opts.EnablePresolve {
				if propOK, _ := b.vipr.propagateNode(node.viprNode, curLB, curUB, nodePropPasses); !propOK {
					continue
				}
			}
		} else if !applyAndPropagateNodeBounds(
			p, node, fromPlunge, b.opts.EnablePresolve, rootPropConv,
			baseLB, baseUB, curLB, curUB, &b.touchedCols, &b.propQueue, b.propInQueue,
		) {
			continue
		}
		if row, conflict := hasExactIntRowActivityConflict(p, b.exactIntRows, curLB, curUB); conflict {
			if b.vipr == nil || b.vipr.certifyRowActivityFarkas(node.viprNode, row) {
				continue
			}
		}

		nodeRes, stop := solveNodeLP(b.lpSolver, curLB, curUB, b.defaultLPIter, b.capLPIter, b.budgetBound, &b.totalLPIters)
		if stop != StatusNotSolved {
			// The node was not evaluated: keep it open so its bound stays in the global bound.
			b.pq.push(node)
			if stop == StatusTimeLimit {
				b.hitTimeLimit = true
			} else {
				b.hitIterLimit = true
			}
			break
		}
		if nodeRes.Status == StatusInfeasible {
			if b.vipr != nil {
				b.vipr.certifyNodeFarkas(node.viprNode, b.lpSolver, nodeRes)
			}
			continue
		}
		if nodeRes.Status != StatusOptimal {
			b.unsolvedMinBound = min(b.unsolvedMinBound, node.lowerBound)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = nodeRes.Status
			}
			continue
		}
		nodeMin := b.lpSearchMin(nodeRes)
		if math.IsNaN(nodeMin) || math.IsInf(nodeMin, 0) {
			b.unsolvedMinBound = min(b.unsolvedMinBound, node.lowerBound)
			if b.hitLPLimitStatus == StatusNotSolved {
				b.hitLPLimitStatus = StatusNumericalError
			}
			continue
		}
		if b.vipr != nil {
			b.vipr.recordNodeDual(node.viprNode, b.lpSolver, nodeRes)
			if f, ok := b.vipr.exactDualSearchMinFloor(node.viprNode, b.searchShift); ok && f > nodeMin {
				nodeMin = f
			}
		}
		if prune, inf := b.shouldPruneNode(node.viprNode, &nodeMin, pruneRelTol); prune || inf {
			b.updateCutAgesAndMaybePurge(true)
			if prune {
				b.prunedMinBound = min(b.prunedMinBound, nodeMin)
			}
			continue
		}

		// Age inactive root cuts; only purge when not about to enter strong or reliability probing
		// so the live LU factorization remains warm for active depth-bounded probes.
		sbBudgetLeft := sbBudgetRemaining(b.totalLPIters, b.sbIters)
		deepSBEligible := p.numVars <= reliabilityMaxVars && b.lpSolver.TotalRows() <= reliabilityMaxRows && sbBudgetLeft > 0
		b.updateCutAgesAndMaybePurge(node.depth > reliabilityMaxDepth || (node.depth > strongBranchMaxDepth && !deepSBEligible))

		if node.hasDelta && !node.orbitalDown {
			gain := max(0.0, nodeMin-node.lowerBound)
			pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		nodeIntFeas := isIntegerFeasible(p, nodeRes.Primal)
		if nodeIntFeas {
			if cand, ok := b.polishIncumbent(nodeRes.Primal); ok {
				if b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
					lastIncumbentNode = b.nodesExplored
					heurMisses = 0
				}
				if prune, inf := b.shouldPruneNode(node.viprNode, &nodeMin, pruneRelTol); prune || inf {
					if prune {
						b.prunedMinBound = min(b.prunedMinBound, nodeMin)
					}
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
		if b.opts.EnableHeuristics && !b.timeExpired() && b.iterLeft() > 0 && runHeur {
			if b.runPropagatedRounding(nodeRes.Primal, curLB, curUB, b.heurSolver, isContLinked) {
				lastIncumbentNode = b.nodesExplored
				heurMisses = 0
				if prune, inf := b.shouldPruneNode(node.viprNode, &nodeMin, pruneRelTol); prune || inf {
					if prune {
						b.prunedMinBound = min(b.prunedMinBound, nodeMin)
					}
					continue
				}
			} else {
				heurMisses++
			}
		}

		if b.bestPrimal != nil {
			if b.runNodeRCF(&node.viprNode, nodeRes, nodeMin, curLB, curUB) {
				continue
			}
			if len(b.rcScratch) > 0 {
				node.base = append(slices.Clip(node.base), b.rcScratch...)
				if b.vipr != nil {
					b.vipr.fillNodeBoundsFloat(node.viprNode, curLB, curUB)
					if b.vipr.checkNodeBoundConflict(node.viprNode) {
						continue
					}
					if b.opts.EnablePresolve {
						if propOK, _ := b.vipr.propagateNode(node.viprNode, curLB, curUB, nodePropPasses); !propOK {
							continue
						}
					}
				} else if b.opts.EnablePresolve && !propagateNodeDeltas(p, b.rcScratch, curLB, curUB, rootPropConv, &b.touchedCols, &b.propQueue, b.propInQueue) {
					continue
				}
				if boundsCutPrimal(nodeRes.Primal, curLB, curUB, n) {
					out := b.resolveNodeAfterFix(node, curLB, curUB, &nodeRes, &nodeMin, pruneRelTol, &lastIncumbentNode, &heurMisses)
					if out == postFixLimitStop {
						break
					}
					if out == postFixPruned {
						continue
					}
				}
			}
		}

		var bVar int
		var bVal float64
		prunedBySB := false
		stoppedByLimit := false
		sbRounds := 0
		for {
			sbBudgetLeft = sbBudgetRemaining(b.totalLPIters, b.sbIters)
			sb := selectBranchVar(&branchSelectReq{
				p:              p,
				lpSolver:       b.lpSolver,
				lpRes:          nodeRes,
				curLB:          curLB,
				curUB:          curUB,
				pCosts:         pCosts,
				depth:          node.depth,
				totalIters:     &b.totalLPIters,
				iterLeft:       b.iterLeft,
				candsBuf:       &b.sbCands,
				allowDomainRed: sbRounds < maxSBDomainRedRounds,
				sym:            symPtr,
				deadline:       b.deadline,
				done:           b.done,
				hasIncumbent:   b.bestPrimal != nil,
				bestObjMin:     b.bestObjMin,
				pruneRelTol:    pruneRelTol,
				absGapTol:      b.opts.AbsGapTol,
				objConstMin:    objConstMin,
				sbBudgetLeft:   sbBudgetLeft,
				strictExactGap: b.strictExactVIPRGap(),
			})
			b.sbIters += sb.probeIters
			b.prunedMinBound = min(b.prunedMinBound, sb.prunedBound)
			if sb.pruned {
				if b.vipr != nil {
					b.vipr.recordSBPruned(node.viprNode, sb.prunedCol, sb.prunedSplit)
				}
				prunedBySB = true
				break
			}
			if len(sb.fixedDeltas) == 0 {
				bVar, bVal = sb.bestCol, sb.bestVal
				break
			}
			sbRounds++
			if b.vipr != nil {
				node.viprNode = b.vipr.recordSBFix(node.viprNode, sb.prunedCol, sb.prunedSplit, sb.downPruned)
			}
			node.base = append(slices.Clip(node.base), sb.fixedDeltas...)
			if b.vipr != nil {
				b.vipr.fillNodeBoundsFloat(node.viprNode, curLB, curUB)
				if b.vipr.checkNodeBoundConflict(node.viprNode) {
					prunedBySB = true
					break
				}
				if b.opts.EnablePresolve {
					if propOK, _ := b.vipr.propagateNode(node.viprNode, curLB, curUB, nodePropPasses); !propOK {
						prunedBySB = true
						break
					}
				}
			} else if b.opts.EnablePresolve && !propagateNodeDeltas(p, sb.fixedDeltas, curLB, curUB, rootPropConv, &b.touchedCols, &b.propQueue, b.propInQueue) {
				prunedBySB = true
				break
			}
			out := b.resolveNodeAfterFix(node, curLB, curUB, &nodeRes, &nodeMin, pruneRelTol, &lastIncumbentNode, &heurMisses)
			if out == postFixLimitStop {
				stoppedByLimit = true
				break
			}
			if out == postFixPruned {
				prunedBySB = true
				break
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
			leafRes, leafMin, leafInfeas, leafOK := resolveFixedIntLeaf(
				p, curLB, curUB, b.exactIntRows, b.searchShift, b.deadline, b.done, b.ensureHeurSolver, b.iterLeft(), b.vipr, node.viprNode,
			)
			b.totalLPIters += leafRes.Iterations
			if leafOK {
				if leafInfeas {
					continue
				}
				nodeMin = max(nodeMin, leafMin)
				if cand, ok := b.polishIncumbent(leafRes.Primal); ok {
					if b.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
						lastIncumbentNode = b.nodesExplored
						heurMisses = 0
					}
				}
				if prune, inf := b.shouldPruneNode(node.viprNode, &nodeMin, pruneRelTol); prune || inf {
					if prune {
						b.prunedMinBound = min(b.prunedMinBound, nodeMin)
					}
					continue
				}
			}
			// Nothing can be branched on: keep the node's bound so the final status
			// never claims an optimal proof over an unexplored region.
			b.unsolvedMinBound = min(b.unsolvedMinBound, nodeMin)
			switch {
			case !leafOK && (leafRes.Status == StatusTimeLimit || b.timeExpired()):
				b.hitTimeLimit = true
			case !leafOK && (leafRes.Status == StatusIterationLimit || (b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter)):
				b.hitIterLimit = true
			case b.timeExpired():
				b.hitTimeLimit = true
			case b.opts.MaxIter > 0 && b.totalLPIters >= b.opts.MaxIter:
				b.hitIterLimit = true
			default:
				if b.hitLPLimitStatus == StatusNotSolved {
					b.hitLPLimitStatus = StatusNumericalError
				}
			}
			if b.hitTimeLimit || b.hitIterLimit {
				break
			}
			continue
		}

		cDown, cUp := b.createChildNodesWithOrbit(branchNodeReq{
			parentLB:    nodeMin,
			parentDepth: node.depth,
			parent:      node,
			brVar:       bVar,
			brVal:       bVal,
			curLB:       curLB,
			curUB:       curUB,
			pc:          pCosts[bVar],
			counter:     &nodeCounter,
		}, node.viprNode)

		if bVal-math.Floor(bVal) >= halfFracTarget {
			b.plungeNode = cUp
			b.pq.push(cDown)
		} else {
			b.plungeNode = cDown
			b.pq.push(cUp)
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

func (c *parCoordinator) acceptIncumbentWith(cand []float64, candMin float64, dualChecked bool) bool {
	if isNonFinite(candMin) {
		return false
	}
	if c.hasIncumbent.Load() && candMin >= math.Float64frombits(c.bestObjBits.Load()) {
		return false
	}
	if !c.b.isValidIncumbentWith(cand, candMin, dualChecked) {
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
	if c.b.opts.RelGapTol > 1.0 && c.b.gapClosed(c.currentGlobalBoundLocked(), c.bestObjMin, c.b.opts.RelGapTol) {
		c.stopAll.Store(true)
		c.cond.Broadcast()
	}
	return true
}

func (c *parCoordinator) acceptIncumbent(cand []float64, candMin float64) bool {
	return c.acceptIncumbentWith(cand, candMin, false)
}

func (c *parCoordinator) acceptPolishedIncumbent(cand []float64, candMin float64) bool {
	return c.acceptIncumbentWith(cand, candMin, true)
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
	c.pq.push(pushChild)
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
		c.pq.push(node)
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
			c.pq.push(localPlunge)
			localPlunge = nil
		}
		c.activeBounds[workerID] = math.Inf(1)
	}

	for {
		if c.stopAll.Load() {
			parkPlunge()
			return nil, false
		}
		if c.b.opts.RelGapTol > 1.0 && c.bestPrimal != nil && c.b.gapClosed(c.currentGlobalBoundLocked(), c.bestObjMin, c.b.opts.RelGapTol) {
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
				c.pq.push(localPlunge)
				localPlunge = nil
			}
		}
		if node == nil {
			if len(c.pq) == 0 {
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
			node = c.pq.pop()
			fromPlunge = false
		}

		if node.lowerBound >= c.bestObjMin-c.b.opts.AbsGapTol || c.b.gapClosed(node.lowerBound, c.bestObjMin, c.pruneRelTol) {
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
	rcScratch        []boundDelta
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
	lpIters          int
	sbIters          int
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
	c := w.coord
	b := c.b
	var iterLeftFn func() int
	if b.opts.MaxIter > 0 {
		iterLeftFn = func() int { return c.iterLeft(0) }
	}
	return polishIncumbentWith(&polishIncumbentReq{
		p:            b.p,
		exactIntRows: b.exactIntRows,
		deadline:     b.deadline,
		done:         b.done,
		x:            x,
		dualAmp:      b.rootDualAmp,
		fixScratchLB: &w.fixScratchLB,
		fixScratchUB: &w.fixScratchUB,
		ensureHeur:   w.ensureHeurSolver,
		iterLeft:     iterLeftFn,
		addIters:     c.addIters,
	})
}

func (w *parWorker) isPruned(bound float64) bool {
	hasInc, bestObjMin := w.coord.bestObj()
	if !hasInc {
		return false
	}
	return bound >= bestObjMin-w.coord.b.opts.AbsGapTol || w.coord.b.gapClosed(bound, bestObjMin, w.coord.pruneRelTol)
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
	nextMin := b.lpSearchMin(nextRes)
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
			if c.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
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
	return applyAndPropagateNodeBounds(
		c.b.p, node, fromPlunge, c.b.opts.EnablePresolve, c.rootPropConv,
		c.baseLB, c.baseUB, w.curLB, w.curUB, &w.touchedCols, &w.propQueue, w.propInQueue,
	)
}

func (w *parWorker) resolveNodeAfterFix(
	node *searchNode,
	nodeRes *LPResult,
	nodeMin *float64,
	prevMinPruned float64,
	prevKind *nodeOutcomeKind,
	prevBound *float64,
	prevStatus *Status,
) postFixOutcome {
	c := w.coord
	b := c.b
	p := b.p
	reIters := 0
	reRes, reMin, reStop, lpSt, infeas, fail := resolveNodeAfterBoundFix(
		p, w.lpSolver, w.curLB, w.curUB, b.searchShift, b.defaultLPIter,
		func(s *LPSolver, base int) bool { return c.capLPIter(s, base, reIters) },
		b.budgetBound, &reIters,
	)
	w.lpIters += reIters
	c.addIters(reIters)
	if reStop != StatusNotSolved {
		node.lowerBound = max(node.lowerBound, *nodeMin)
		c.abortOnLimit(w.id, node, reStop, prevMinPruned)
		return postFixLimitStop
	}
	if infeas {
		return postFixPruned
	}
	if fail {
		*prevKind = nodeOutcomeUnsolved
		*prevBound = *nodeMin
		*prevStatus = lpSt
		return postFixPruned
	}
	*nodeRes = reRes
	*nodeMin = max(*nodeMin, reMin)
	if w.isPruned(*nodeMin) {
		*prevKind = nodeOutcomePruned
		*prevBound = *nodeMin
		return postFixPruned
	}
	if isIntegerFeasible(p, nodeRes.Primal) {
		if cand, ok := w.polishIncumbent(nodeRes.Primal); ok {
			if c.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
				w.heurMisses = 0
			}
			if w.isPruned(*nodeMin) {
				*prevKind = nodeOutcomePruned
				*prevBound = *nodeMin
				return postFixPruned
			}
		}
	}
	return postFixContinue
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
		if _, conflict := hasExactIntRowActivityConflict(p, b.exactIntRows, w.curLB, w.curUB); conflict {
			continue
		}

		nodeLPIters := 0
		nodeRes, stop := solveNodeLP(
			w.lpSolver, w.curLB, w.curUB, b.defaultLPIter,
			func(s *LPSolver, base int) bool { return c.capLPIter(s, base, nodeLPIters) },
			b.budgetBound, &nodeLPIters,
		)
		w.lpIters += nodeLPIters
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
		nodeMin := b.lpSearchMin(nodeRes)
		if math.IsNaN(nodeMin) || math.IsInf(nodeMin, 0) {
			prevKind = nodeOutcomeUnsolved
			prevBound = node.lowerBound
			prevStatus = StatusNumericalError
			continue
		}
		if w.isPruned(nodeMin) {
			updateCutAgesState(p, w.lpSolver, nil, &w.cutAges, &w.nodesSincePurge, true)
			prevKind = nodeOutcomePruned
			prevBound = nodeMin
			continue
		}

		sbBudgetLeft := sbBudgetRemaining(w.lpIters, w.sbIters)
		deepSBEligible := p.numVars <= reliabilityMaxVars && w.lpSolver.TotalRows() <= reliabilityMaxRows && sbBudgetLeft > 0
		updateCutAgesState(p, w.lpSolver, nil, &w.cutAges, &w.nodesSincePurge, node.depth > reliabilityMaxDepth || (node.depth > strongBranchMaxDepth && !deepSBEligible))

		if node.hasDelta && !node.orbitalDown {
			gain := max(0.0, nodeMin-node.lowerBound)
			w.pCosts[node.delta.col].record(node.up, gain, node.brDist)
		}

		nodeIntFeas := isIntegerFeasible(p, nodeRes.Primal)
		if nodeIntFeas {
			if cand, ok := w.polishIncumbent(nodeRes.Primal); ok {
				if c.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
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
		if b.opts.EnableHeuristics && !b.timeExpired() && c.iterLeft(0) > 0 && runHeur {
			var hs *LPSolver
			if hasCont {
				hs = w.ensureHeurSolver()
			}
			sol, objMin, usedIt, ok := tryPropagatedRounding(p, nodeRes.Primal, w.curLB, w.curUB, hs, b.searchShift, c.isContLinked, c.iterLeft(0))
			c.addIters(usedIt)
			if ok && c.acceptIncumbent(sol, objMin) {
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

		if hasInc, bestObjMin := c.bestObj(); hasInc {
			var rcInfeas bool
			w.rcScratch, rcInfeas = applyReducedCostFixing(p, nodeRes, nodeMin, bestObjMin, b.maxObjAbs, &w.sym, w.curLB, w.curUB, w.rcScratch[:0])
			if len(w.rcScratch) > 0 || rcInfeas {
				prevMinPruned = min(prevMinPruned, bestObjMin)
			}
			if rcInfeas {
				continue
			}
			if len(w.rcScratch) > 0 {
				node.base = append(slices.Clip(node.base), w.rcScratch...)
				if b.opts.EnablePresolve && !propagateNodeDeltas(p, w.rcScratch, w.curLB, w.curUB, c.rootPropConv, &w.touchedCols, &w.propQueue, w.propInQueue) {
					continue
				}
				if boundsCutPrimal(nodeRes.Primal, w.curLB, w.curUB, n) {
					out := w.resolveNodeAfterFix(node, &nodeRes, &nodeMin, prevMinPruned, &prevKind, &prevBound, &prevStatus)
					if out == postFixLimitStop {
						return
					}
					if out == postFixPruned {
						continue
					}
				}
			}
		}

		var bVar int
		var bVal float64
		prunedBySB := false
		sbRounds := 0
		for {
			hasInc, bestObjMin := c.bestObj()
			sbBudgetLeft = sbBudgetRemaining(w.lpIters, w.sbIters)
			sbLPIters := 0
			sb := selectBranchVar(&branchSelectReq{
				p:              p,
				lpSolver:       w.lpSolver,
				lpRes:          nodeRes,
				curLB:          w.curLB,
				curUB:          w.curUB,
				pCosts:         w.pCosts,
				depth:          node.depth,
				totalIters:     &sbLPIters,
				iterLeft:       func() int { return c.iterLeft(sbLPIters) },
				candsBuf:       &w.sbCands,
				allowDomainRed: sbRounds < maxSBDomainRedRounds,
				sym:            &w.sym,
				deadline:       b.deadline,
				done:           b.done,
				hasIncumbent:   hasInc,
				bestObjMin:     bestObjMin,
				pruneRelTol:    c.pruneRelTol,
				absGapTol:      b.opts.AbsGapTol,
				objConstMin:    c.objConstMin,
				sbBudgetLeft:   sbBudgetLeft,
			})
			w.lpIters += sbLPIters
			w.sbIters += sb.probeIters
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
			if b.opts.EnablePresolve && !propagateNodeDeltas(p, sb.fixedDeltas, w.curLB, w.curUB, c.rootPropConv, &w.touchedCols, &w.propQueue, w.propInQueue) {
				prunedBySB = true
				break
			}
			out := w.resolveNodeAfterFix(node, &nodeRes, &nodeMin, prevMinPruned, &prevKind, &prevBound, &prevStatus)
			if out == postFixLimitStop {
				return
			}
			if out == postFixPruned {
				prunedBySB = true
				break
			}
		}
		if prunedBySB {
			continue
		}

		if bVar == -1 {
			bVar, bVal = selectNearIntegralBranchVar(p, nodeRes.Primal, w.curLB, w.curUB)
		}
		if bVar == -1 {
			leafRes, leafMin, leafInfeas, leafOK := resolveFixedIntLeaf(
				p, w.curLB, w.curUB, b.exactIntRows, b.searchShift, b.deadline, b.done, w.ensureHeurSolver, c.iterLeft(0), nil, nil,
			)
			c.addIters(leafRes.Iterations)
			if leafOK {
				if leafInfeas {
					continue
				}
				nodeMin = max(nodeMin, leafMin)
				if cand, ok := w.polishIncumbent(leafRes.Primal); ok {
					if c.acceptPolishedIncumbent(cand, b.evalSearchMin(cand)) {
						w.heurMisses = 0
					}
				}
				if w.isPruned(nodeMin) {
					prevKind = nodeOutcomePruned
					prevBound = nodeMin
					continue
				}
			}
			if (!leafOK && (leafRes.Status == StatusTimeLimit || b.timeExpired())) || b.timeExpired() {
				node.lowerBound = max(node.lowerBound, nodeMin)
				c.abortOnLimit(w.id, node, StatusTimeLimit, prevMinPruned)
				return
			}
			if (!leafOK && (leafRes.Status == StatusIterationLimit || c.iterLeft(0) <= 0)) || c.iterLeft(0) <= 0 {
				node.lowerBound = max(node.lowerBound, nodeMin)
				c.abortOnLimit(w.id, node, StatusIterationLimit, prevMinPruned)
				return
			}
			prevKind = nodeOutcomeUnsolved
			prevBound = nodeMin
			prevStatus = StatusNumericalError
			continue
		}

		cDown, cUp := createChildNodesWithOrbitState(
			p, &w.sym, b.deadline, b.done, &w.orbitBuf,
			branchNodeReq{
				parentLB:    nodeMin,
				parentDepth: node.depth,
				parent:      node,
				brVar:       bVar,
				brVal:       bVal,
				curLB:       w.curLB,
				curUB:       w.curUB,
				pc:          w.pCosts[bVar],
				counter:     &w.localNodeCounter,
			},
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

func (b *babEngine) exploreTreeParallel(req *parallelTreeReq) (Result, bool) {
	if b.plungeNode != nil {
		b.pq.push(b.plungeNode)
		b.plungeNode = nil
	}
	workers := max(min(req.workers, len(b.pq)), 2)

	coord := &parCoordinator{
		b:                b,
		baseLB:           req.baseLB,
		baseUB:           req.baseUB,
		rootPropConv:     req.rootPropConv,
		isContLinked:     req.isContLinked,
		rootCutsHelped:   req.rootCutsHelped,
		inTreePoolCap:    req.inTreePoolCap,
		inTreeBatchCap:   req.inTreeBatchCap,
		pruneRelTol:      req.pruneRelTol,
		objConstMin:      req.objConstMin,
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

	baseCounter := req.baseNodeCounter
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
			pCosts:           slices.Clone(req.pCosts),
			sym:              b.sym.cloneForWorker(),
			curLB:            slices.Clone(req.baseLB),
			curUB:            slices.Clone(req.baseUB),
			propQueue:        propQ,
			propInQueue:      propInQ,
			cutAges:          slices.Clone(b.cutAges),
			nodesSincePurge:  b.nodesSincePurge,
			inTreeCutMisses:  b.inTreeCutMisses,
			inTreeCutRounds:  b.inTreeCutRounds,
			localNodeCounter: baseCounter + (w+1)*100000,
			nodesProcessed:   b.nodesExplored,
			heurMisses:       req.heurMisses,
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
	case b.hitNodeLimit || len(b.pq) > 0 || b.plungeNode != nil:
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
	if b.bestPrimal != nil &&
		math.IsInf(b.prunedMinBound, 1) &&
		math.IsInf(b.unsolvedMinBound, 1) &&
		b.limitStatus() == StatusOptimal &&
		!incumbentFeasibleScaled(p, b.bestPrimal, 0) &&
		!exactMIPFeasibleRat(p, b.bestPrimal) {
		// Every leaf of the search tree proved infeasible without relying on the incumbent bound;
		// discard an inexact heuristic ghost so Status and VIPR stay consistent.
		b.bestPrimal = nil
	}
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
			bestBoundUser = b.userObjFromSearchMin(bestBoundMin)
			if isLimitStatus(status) && (math.IsNaN(bestBoundUser) || isOppositeInfBound(p.sense, bestBoundUser)) {
				bestBoundUser = trivialDualBound(p.sense)
			}
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
	exactTreeClosed := b.vipr != nil &&
		!b.hitTimeLimit && !b.hitIterLimit && !b.hitNodeLimit &&
		math.IsInf(b.unsolvedMinBound, 1) && b.hitLPLimitStatus == StatusNotSolved &&
		b.vipr.exactTreeGapClosed(b.bestPrimal)
	if exactTreeClosed {
		bestBoundMin = b.bestObjMin
	} else if (b.strictExactVIPRGap() && b.limitStatus() != StatusOptimal) || !b.gapClosed(bestBoundMin, b.bestObjMin, b.opts.RelGapTol) {
		status = b.limitStatus()
		if status == StatusOptimal {
			// Unreachable in exact bookkeeping: an open gap implies an open or unsolved node.
			status = StatusUndefined
		}
	}

	obj := p.EvaluateObjective(b.bestPrimal)
	bestBoundUser := clampBestBoundToObj(p.sense, b.userObjFromSearchMin(bestBoundMin), obj)
	if isLimitStatus(status) && (math.IsNaN(bestBoundUser) || isOppositeInfBound(p.sense, bestBoundUser)) {
		bestBoundUser = trivialDualBound(p.sense)
	}
	return b.optimalOrNumErr(Result{
		modelID:      p.modelID,
		Status:       status,
		Objective:    obj,
		BestBound:    bestBoundUser,
		RootLPBound:  b.rootLPUser,
		RootCutBound: b.rootCutUser,
		Values:       b.bestPrimal,
		Nodes:        b.nodesExplored,
		Iterations:   b.totalLPIters,
		CutsAdded:    b.cutsAdded,
		SolveTime:    time.Since(b.startTime),
	})
}

// computeRootDualAmp computes per-row root dual scale factors |y_i|/max(1, ||c||_inf).
// It returns nil when every row stays at or below 1.0.
func computeRootDualAmp(m int, dual []float64, maxObjAbs float64) []float64 {
	lim := min(m, len(dual))
	objScale := max(1.0, maxObjAbs)
	var amp []float64
	for i := range lim {
		d := math.Abs(dual[i])
		if isNonFinite(d) || d <= objScale {
			continue
		}
		ratio := d / objScale
		if isNonFinite(ratio) || ratio <= 1.0 {
			continue
		}
		if amp == nil {
			amp = make([]float64, m)
		}
		amp[i] = ratio
	}
	return amp
}

// isValidIncumbentWith rejects non-finite, dual-amplified infeasible, or root-LP-violating
// inexact candidates before they can tighten the primal bound or poison BestBound.
// Callers that already verified incumbentFeasibleWithDual(b.p, cand, b.rootDualAmp, tol <= incumbentTol)
// (such as polishIncumbent) pass dualChecked=true to skip re-scanning all rows.
func (b *babEngine) isValidIncumbentWith(cand []float64, candMin float64, dualChecked bool) bool {
	if isNonFinite(candMin) {
		return false
	}
	if !dualChecked && violatesExactIntRows(b.p, b.exactIntRows, cand) {
		return false
	}
	if !dualChecked && len(b.rootDualAmp) > 0 && !incumbentFeasibleWithDual(b.p, cand, b.rootDualAmp, incumbentTol) {
		return false
	}
	if !isNonFinite(b.rootLPMin) {
		tol := max(incumbentTol, incumbentTol*max(1.0, math.Abs(b.rootLPMin)))
		if candMin < b.rootLPMin-tol && !incumbentFeasibleScaled(b.p, cand, 0) && !exactMIPFeasibleRat(b.p, cand) {
			return false
		}
	}
	if b.vipr != nil && !b.vipr.canCertifyIncumbent(cand, candMin) {
		return false
	}
	return true
}

func (b *babEngine) acceptIncumbentWith(cand []float64, candMin float64, dualChecked bool) bool {
	if isNonFinite(candMin) || candMin >= b.bestObjMin || !b.isValidIncumbentWith(cand, candMin, dualChecked) {
		return false
	}
	b.bestObjMin = candMin
	b.bestPrimal = cand
	return true
}

func (b *babEngine) acceptIncumbent(cand []float64, candMin float64) bool {
	return b.acceptIncumbentWith(cand, candMin, false)
}

func (b *babEngine) acceptPolishedIncumbent(cand []float64, candMin float64) bool {
	return b.acceptIncumbentWith(cand, candMin, true)
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
func (b *babEngine) createChildNodesWithOrbit(req branchNodeReq, parentVN *viprTreeNode) (downNode, upNode *searchNode) {
	if b.vipr == nil {
		return createChildNodesWithOrbitState(b.p, &b.sym, b.deadline, b.done, &b.orbitBuf, req)
	}
	return b.vipr.createChildNodes(b.p, &b.sym, b.deadline, b.done, req, parentVN)
}

func createChildNodesWithOrbitState(
	p *CompiledProblem,
	sym *nodeSymmetry,
	deadline time.Time,
	done <-chan struct{},
	orbitBuf *[]int,
	req branchNodeReq,
) (downNode, upNode *searchNode) {
	downNode, upNode = createChildNodes(
		req.parentLB, req.parentDepth+1, req.parent, req.brVar, req.brVal,
		req.curLB[req.brVar], req.curUB[req.brVar], req.pc, req.counter,
	)
	if sym != nil && p.varCat[req.brVar] == Binary && req.curLB[req.brVar] == 0 && req.curUB[req.brVar] == 1 && downNode.delta.ub == 0 && upNode.delta.lb == 1 {
		*orbitBuf = sym.activeOrbit(p, req.curLB, req.curUB, req.brVar, req.parentDepth, deadline, done, *orbitBuf)
		if len(*orbitBuf) >= 2 {
			downNode.base = slices.Clip(downNode.base)
			for _, c := range *orbitBuf {
				if c != req.brVar {
					downNode.base = append(downNode.base, boundDelta{col: c, lb: 0, ub: 0})
				}
			}
			downNode.orbitalDown = true
		}
	}
	return downNode, upNode
}

// probeBranchBound evaluates one strong-branching probe in place.
// It updates col via SetColBound and runs a zero-allocation shifted SolveBoundOnly solve.
func probeBranchBound(
	lpSolver *LPSolver,
	col int,
	origL, origU, probeLB, probeUB float64,
	maxIter int,
	shift float64,
) (Status, float64, int) {
	lpSolver.SetColBound(col, probeLB, probeUB)
	st, bound, iters := lpSolver.solveBoundOnlyWithShift(maxIter, shift)
	lpSolver.SetColBound(col, origL, origU)
	return st, bound, iters
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

func sbBudgetRemaining(lpIters, sbIters int) int {
	allowed := max(sbBudgetBaseIters, lpIters) * sbBudgetRatioNum / sbBudgetRatioDen
	if allowed <= sbIters {
		return 0
	}
	return allowed - sbIters
}

func reliabilityParamsForDepth(depth, numVars, totalRows, sbBudgetLeft int) (relCnt, candCap, probeIterCap, scanCap int, canProbe bool) {
	switch {
	case depth <= strongBranchMaxDepth:
		return strongBranchReliableCnt, strongBranchCandCap, strongBranchMaxIter, strongBranchCandCap, true
	case depth > reliabilityMaxDepth || numVars > reliabilityMaxVars || totalRows > reliabilityMaxRows || sbBudgetLeft <= 0:
		return 0, 0, 0, 0, false
	default:
		return reliabilityDeepRelCnt, reliabilityDeepCandCap, reliabilityDeepMaxIter, reliabilityCandScanCap, true
	}
}

func candNeedsProbe(p *CompiledProblem, col int, pc pseudoCost, depth, relCnt int) bool {
	if depth <= strongBranchMaxDepth {
		return pc.downCount < relCnt || pc.upCount < relCnt
	}
	// Beyond depth 2, only probe unmeasured variables lacking an objective coefficient prior.
	return pc.downCount < relCnt && pc.upCount < relCnt && math.Abs(p.objMin[col]) <= zeroGainAbsTol
}

func selectBranchVar(req *branchSelectReq) sbOutcome {
	p := req.p
	lpSolver := req.lpSolver
	lpRes := req.lpRes
	curLB, curUB := req.curLB, req.curUB
	pCosts := req.pCosts
	depth := req.depth
	totalIters := req.totalIters
	iterLeft := req.iterLeft
	candsBuf := req.candsBuf
	allowDomainRed := req.allowDomainRed

	bestCol := -1
	bestVal := 0.0
	bestScore := -1.0

	totalRows := 0
	if lpSolver != nil {
		totalRows = lpSolver.TotalRows()
	}
	relCnt, candCap, probeIterCap, scanCap, canProbe := reliabilityParamsForDepth(depth, p.numVars, totalRows, req.sbBudgetLeft)
	if !canProbe || lpSolver == nil {
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

	unreliableCount := 0
	for _, col := range p.intCols {
		if c, ok := evalBranchCand(p, col, lpRes.Primal[col], curLB[col], curUB[col], pCosts[col]); ok {
			cands = append(cands, c)
			if c.score > bestScore {
				bestScore = c.score
				bestCol = c.col
				bestVal = c.val
			}
			if candNeedsProbe(p, col, pCosts[col], depth, relCnt) {
				unreliableCount++
			}
		}
	}
	if candsBuf != nil {
		*candsBuf = cands
	}

	if len(cands) == 0 {
		return sbOutcome{bestCol: -1, prunedBound: math.Inf(1)}
	}
	if len(cands) == 1 || (depth > strongBranchMaxDepth && unreliableCount == 0) {
		return sbOutcome{bestCol: bestCol, bestVal: bestVal, prunedBound: math.Inf(1)}
	}

	probeItersUsed := 0
	slices.SortFunc(cands, func(a, b branchCand) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		af := a.val - math.Floor(a.val)
		bf := b.val - math.Floor(b.val)
		ad := min(af, 1.0-af)
		bd := min(bf, 1.0-bf)
		if c := cmp.Compare(bd, ad); c != 0 {
			return c
		}
		return cmp.Compare(a.col, b.col)
	})

	sbScanLimit := min(scanCap, len(cands))
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
	baseObjMin := lpMinWithShift(p, lpRes, req.objConstMin)
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
		if !req.hasIncumbent || (st != StatusOptimal && st != StatusIterationLimit) || math.IsNaN(bound) {
			return false, math.Inf(1)
		}
		boundMin = bound
		if req.strictExactGap {
			margin := max(1e-6, 1e-7*max(1.0, math.Abs(req.bestObjMin)))
			return boundMin > req.bestObjMin+margin, boundMin
		}
		cut = boundMin >= req.bestObjMin-req.absGapTol || isGapClosed(boundMin, req.bestObjMin, req.objConstMin, req.pruneRelTol, req.absGapTol)
		return cut, boundMin
	}

	probedCount := 0
	deep := depth > strongBranchMaxDepth
	for idx := 0; idx < sbScanLimit && probedCount < candCap; idx++ {
		if lpSolver.timeExpired() {
			break
		}
		c := &cands[idx]
		col := c.col
		if !candNeedsProbe(p, col, pCosts[col], depth, relCnt) {
			continue
		}
		v := c.val
		fl := math.Floor(v)
		cl := math.Ceil(v)
		origL, origU := curLB[col], curUB[col]

		sbIter := min(probeIterCap, iterLeft())
		if deep {
			sbIter = min(sbIter, req.sbBudgetLeft-probeItersUsed)
		}
		if sbIter <= 0 {
			break
		}
		if !hasSnap {
			lpSolver.captureCheckpoint(sbCP)
			hasSnap = true
		}
		downSt, downBound, downIters := probeBranchBound(lpSolver, col, origL, origU, origL, fl, sbIter, req.objConstMin)
		*totalIters += downIters
		probeItersUsed += downIters
		probedCount++

		var upSt Status
		upBound := math.NaN()
		upProbed := false
		if !lpSolver.timeExpired() {
			sbIter = min(probeIterCap, iterLeft())
			if deep {
				sbIter = min(sbIter, req.sbBudgetLeft-probeItersUsed)
			}
			if sbIter > 0 {
				var upIters int
				upSt, upBound, upIters = probeBranchBound(lpSolver, col, origL, origU, cl, origU, sbIter, req.objConstMin)
				*totalIters += upIters
				probeItersUsed += upIters
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
			pCol := col
			pSplit := fl
			if math.IsNaN(fl) || math.IsInf(fl, 0) {
				pCol = -1
				pSplit = 0
			}
			prunedOut := func() sbOutcome {
				return sbOutcome{
					bestCol:     -1,
					pruned:      true,
					prunedBound: pBound,
					prunedCol:   pCol,
					prunedSplit: pSplit,
					downPruned:  true,
					upPruned:    true,
					probeIters:  probeItersUsed,
				}
			}
			if downPruned && upPruned {
				lpSolver.restoreCheckpoint(sbCP)
				return prunedOut()
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
					return prunedOut()
				}
				return sbOutcome{
					bestCol:     -1,
					prunedBound: pBound,
					prunedCol:   pCol,
					prunedSplit: pSplit,
					downPruned:  true,
					fixedDeltas: []boundDelta{{col: col, lb: cl, ub: origU}},
					probeIters:  probeItersUsed,
				}
			}
			if upFix {
				var orbit []int
				if req.sym != nil && p.varCat[col] == Binary && origL == 0 && origU == 1 && fl == 0 {
					orbit = req.sym.activeOrbit(p, curLB, curUB, col, depth, req.deadline, req.done, nil)
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
						return prunedOut()
					}
					deltas = append(deltas, boundDelta{col: c, lb: 0, ub: 0})
				}
				lpSolver.restoreCheckpoint(sbCP)
				if curLB[col] > curUB[col]+intFeasTol {
					return prunedOut()
				}
				return sbOutcome{
					bestCol:     -1,
					prunedBound: pBound,
					prunedCol:   pCol,
					prunedSplit: pSplit,
					upPruned:    true,
					fixedDeltas: deltas,
					probeIters:  probeItersUsed,
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

	bestScore = -1.0
	for _, c := range cands {
		if c.score > bestScore {
			bestScore = c.score
			bestCol = c.col
			bestVal = c.val
		}
	}
	return sbOutcome{bestCol: bestCol, bestVal: bestVal, prunedBound: math.Inf(1), probeIters: probeItersUsed}
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
	var iterLeftFn func() int
	if b.opts.MaxIter > 0 {
		iterLeftFn = b.iterLeft
	}
	return polishIncumbentWith(&polishIncumbentReq{
		p:            b.p,
		exactIntRows: b.exactIntRows,
		deadline:     b.deadline,
		done:         b.done,
		x:            x,
		dualAmp:      b.rootDualAmp,
		fixScratchLB: &b.fixScratchLB,
		fixScratchUB: &b.fixScratchUB,
		ensureHeur:   b.ensureHeurSolver,
		iterLeft:     iterLeftFn,
		addIters:     func(it int) { b.totalLPIters += it },
	})
}

func polishIncumbentWith(req *polishIncumbentReq) ([]float64, bool) {
	p := req.p
	cand := slices.Clone(req.x)
	for _, col := range p.intCols {
		v := cand[col]
		if isNonFinite(v) {
			return nil, false
		}
		rv := math.Round(v)
		diff := math.Abs(v - rv)
		if diff <= intSnapTol {
			cand[col] = rv
		} else if diff > incumbentTol {
			return nil, false
		}
	}
	if violatesExactIntRows(p, req.exactIntRows, cand) {
		return nil, false
	}
	if len(p.intCols) < p.numVars && !stopRequested(req.deadline, req.done) {
		if incumbentFeasibleWithDual(p, cand, req.dualAmp, primalTol) {
			return cand, true
		}
		n := p.numVars
		if len(*req.fixScratchLB) < n {
			*req.fixScratchLB = make([]float64, n)
			*req.fixScratchUB = make([]float64, n)
		}
		fixLB := (*req.fixScratchLB)[:n]
		fixUB := (*req.fixScratchUB)[:n]
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
			if req.iterLeft != nil && req.iterLeft() <= 0 {
				return nil, false
			}
			hs := req.ensureHeur()
			prevMaxIter := hs.MaxIter
			if req.iterLeft != nil {
				rem := req.iterLeft()
				if rem <= 0 {
					return nil, false
				}
				if prevMaxIter <= 0 || rem < prevMaxIter {
					hs.MaxIter = rem
				}
			}
			hs.SetStructuralBounds(fixLB, fixUB)
			polRes := hs.Solve()
			hs.MaxIter = prevMaxIter
			if req.addIters != nil {
				req.addIters(polRes.Iterations)
			}
			if polRes.Status == StatusInfeasible {
				return nil, false
			}
			if polRes.Status == StatusOptimal && len(polRes.Primal) == n {
				copy(cand, polRes.Primal)
				snapToIntegers(p, cand)
				if violatesExactIntRows(p, req.exactIntRows, cand) {
					return nil, false
				}
			}
		}
	}
	return cand, incumbentFeasibleWithDual(p, cand, req.dualAmp, incumbentTol)
}

// resolveFixedIntLeaf re-solves a node whose integer variables are all fixed to singletons
// on a cut-free fresh basis so relaxed dynamic cuts or warm-start primal tolerances cannot
// leave an artificial gap below the node's exact continuous optimum.
func resolveFixedIntLeaf(
	p *CompiledProblem,
	curLB, curUB []float64,
	exactIntRows []int,
	searchShift float64,
	deadline time.Time,
	done <-chan struct{},
	ensureHeur func() *LPSolver,
	maxIterCap int,
	vipr babVIPRHook,
	vn *viprTreeNode,
) (res LPResult, objMin float64, infeas, ok bool) {
	if stopRequested(deadline, done) {
		return LPResult{}, 0, false, false
	}
	for _, col := range p.intCols {
		lb, ub := curLB[col], curUB[col]
		if isNonFinite(lb) || isNonFinite(ub) || lb != ub || math.Floor(lb) != lb {
			return LPResult{}, 0, false, false
		}
	}
	if row, conflict := hasExactIntRowActivityConflict(p, exactIntRows, curLB, curUB); conflict {
		if vipr == nil || (vn != nil && vipr.certifyRowActivityFarkas(vn, row)) {
			return LPResult{Status: StatusInfeasible}, 0, true, true
		}
	}
	if maxIterCap <= 0 {
		return LPResult{Status: StatusIterationLimit}, 0, false, false
	}
	hs := ensureHeur()
	prevSkip := hs.SkipExactFarkas
	prevMaxIter := hs.MaxIter
	hs.SkipExactFarkas = false
	if prevMaxIter <= 0 || maxIterCap < prevMaxIter {
		hs.MaxIter = maxIterCap
	}
	hs.InitCrashBasis()
	hs.SetStructuralBounds(curLB, curUB)
	res = hs.Solve()
	hs.SkipExactFarkas = prevSkip
	hs.MaxIter = prevMaxIter
	if res.Status == StatusInfeasible {
		if vipr != nil && vn != nil {
			vipr.certifyNodeFarkas(vn, hs, res)
		}
		return res, 0, true, true
	}
	if res.Status != StatusOptimal || len(res.Primal) != p.numVars {
		return res, 0, false, false
	}
	objMin = lpMinWithShift(p, res, searchShift)
	if isNonFinite(objMin) {
		return res, 0, false, false
	}
	if vipr != nil && vn != nil {
		vipr.recordNodeDual(vn, hs, res)
	}
	return res, objMin, false, true
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

func lpMinWithShift(p *CompiledProblem, res LPResult, shift float64) float64 {
	if shift == 0 {
		return res.ObjectiveMin
	}
	return lpMinWithConst(p, res)
}

func shouldUseObjConstShift(p *CompiledProblem, linMin, shiftedMin float64) bool {
	if p == nil || p.objConstant == 0 {
		return false
	}
	cMin := p.objConstMin()
	absC := math.Abs(cMin)
	if math.Abs(shiftedMin) < 0.5*math.Abs(linMin) {
		return true
	}
	// Opposite-sign relaxation values at the scale of ObjConstant cancel once cuts or
	// branching tighten the relaxation toward the integer hull.
	if (linMin < 0) != (cMin < 0) && math.Abs(linMin) >= 0.25*absC {
		return true
	}
	// When cMin < 0, branching can only raise c^T x toward |cMin|. If bounded variables
	// carry opposing-scale objective terms whose finite range reaches |cMin|, shifting
	// preserves low-order bits once branching activates those terms.
	if cMin < 0 {
		oppCap := 0.0
		for j := range p.numVars {
			cj := p.objMin[j]
			if cj == 0 {
				continue
			}
			if cj > 0 {
				ub := p.varUB[j]
				if !math.IsInf(ub, 1) && ub > 0 && ub <= 1e6 && cj*ub >= 0.01*absC {
					oppCap += cj * ub
				}
			} else {
				lb := p.varLB[j]
				if !math.IsInf(lb, -1) && lb < 0 && lb >= -1e6 && cj*lb >= 0.01*absC {
					oppCap += cj * lb
				}
			}
		}
		if oppCap >= 0.25*absC {
			return true
		}
	}
	return false
}

func clampBestBoundToObj(sense Sense, bestBound, obj float64) float64 {
	if math.IsNaN(bestBound) || math.IsNaN(obj) {
		return bestBound
	}
	if sense == Minimize {
		if bestBound > obj {
			return obj
		}
		return bestBound
	}
	if bestBound < obj {
		return obj
	}
	return bestBound
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

func isGapClosed(lowerBound, upperBound, shift, relTol, absTol float64) bool {
	if math.IsInf(upperBound, 1) || math.IsInf(lowerBound, -1) {
		return false
	}
	diff := upperBound - lowerBound
	if diff <= absTol {
		return true
	}
	upperMag := math.Abs(upperBound)
	if shift != 0 {
		if linMag := math.Abs(upperBound - shift); linMag < upperMag {
			upperMag = linMag
		}
	}
	denom := max(1.0, upperMag)
	return (diff / denom) <= relTol
}

func tightenUnboundedIntCols(p *CompiledProblem, baseLB, baseUB []float64, bestObjMin, searchShift float64) {
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
	slackAcc.addVal(-searchShift)
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
		w.addVal(-searchShift)
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

func maxAbsFloat64(vals []float64) float64 {
	m := 0.0
	for _, v := range vals {
		if a := math.Abs(v); a > m {
			m = a
		}
	}
	return m
}

func boundsCutPrimal(primal, lb, ub []float64, n int) bool {
	if len(primal) < n {
		return false
	}
	for j := range n {
		if primal[j] < lb[j]-primalTol || primal[j] > ub[j]+primalTol {
			return true
		}
	}
	return false
}

func propagateNodeDeltas(
	p *CompiledProblem,
	deltas []boundDelta,
	curLB, curUB []float64,
	rootPropConv bool,
	touchedCols *[]int,
	propQueue *[]int,
	propInQueue []bool,
) bool {
	*touchedCols = (*touchedCols)[:0]
	for _, d := range deltas {
		*touchedCols = append(*touchedCols, d.col)
	}
	if !rootPropConv {
		return propagateBounds(p, curLB, curUB, nodePropPasses)
	}
	if len(*touchedCols) == 1 {
		return propagateFromCol(p, curLB, curUB, (*touchedCols)[0], propQueue, propInQueue)
	}
	return propagateFromCols(p, curLB, curUB, *touchedCols, propQueue, propInQueue)
}

func resolveNodeAfterBoundFix(
	p *CompiledProblem,
	s *LPSolver,
	curLB, curUB []float64,
	searchShift float64,
	defaultLPIter int,
	capLPIter func(*LPSolver, int) bool,
	budgetBound func(*LPSolver) bool,
	totalIters *int,
) (res LPResult, objMin float64, stop, lpStatus Status, infeas, fail bool) {
	res, stop = solveNodeLP(s, curLB, curUB, defaultLPIter, capLPIter, budgetBound, totalIters)
	if stop != StatusNotSolved {
		return res, 0, stop, StatusNotSolved, false, false
	}
	if res.Status == StatusInfeasible {
		return res, 0, StatusNotSolved, StatusInfeasible, true, false
	}
	if res.Status != StatusOptimal {
		return res, 0, StatusNotSolved, res.Status, false, true
	}
	objMin = lpMinWithShift(p, res, searchShift)
	if math.IsNaN(objMin) || math.IsInf(objMin, 0) {
		return res, 0, StatusNotSolved, StatusNumericalError, false, true
	}
	return res, objMin, StatusNotSolved, StatusOptimal, false, false
}

func symmetricGroupPeers(p *CompiledProblem, sym *nodeSymmetry, curLB, curUB []float64) ([64]uint64, bool) {
	var peerMask [64]uint64
	if sym == nil || sym.symG < 2 || sym.symG > 64 || len(sym.symOrbits) == 0 {
		return peerMask, false
	}
	numOrbs := len(sym.symOrbits) / sym.symG
	anyPeer := false
	for g0 := range sym.symG - 1 {
		for g1 := g0 + 1; g1 < sym.symG; g1++ {
			ok := true
			for o := range numOrbs {
				a := sym.symOrbits[o*sym.symG+g0]
				b := sym.symOrbits[o*sym.symG+g1]
				if p.objMin[a] != p.objMin[b] || curLB[a] != curLB[b] || curUB[a] != curUB[b] {
					ok = false
					break
				}
			}
			if ok {
				peerMask[g0] |= uint64(1) << g1
				peerMask[g1] |= uint64(1) << g0
				anyPeer = true
			}
		}
	}
	return peerMask, anyPeer
}

// applyReducedCostFixing tightens integer column bounds in curLB and curUB using
// LP reduced costs against bestObjMin, preserving active group symmetry across sym.
func applyReducedCostFixing(
	p *CompiledProblem,
	lpRes LPResult,
	nodeMin, bestObjMin, maxObjAbs float64,
	sym *nodeSymmetry,
	curLB, curUB []float64,
	dst []boundDelta,
) ([]boundDelta, bool) {
	dst = dst[:0]
	if p == nil || math.IsInf(bestObjMin, 1) || math.IsNaN(bestObjMin) || math.IsNaN(nodeMin) || math.IsInf(nodeMin, 0) {
		return dst, false
	}
	n := p.numVars
	if len(lpRes.ReducedCosts) < n || len(lpRes.Primal) < n {
		return dst, false
	}
	gap := bestObjMin - nodeMin
	if gap < 0 || math.IsInf(gap, 0) || math.IsNaN(gap) {
		return dst, false
	}
	peerMask, hasSymPeers := symmetricGroupPeers(p, sym, curLB, curUB)
	scale := max(1.0, max(maxObjAbs, max(math.Abs(bestObjMin), math.Abs(nodeMin))))
	rcSafety := max(rcSafetyAbs, rcSafetyRel*scale)
	minDual := max(rcMinDualMag, rcDualNoiseRel*maxObjAbs)
	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	allowedGap := gap + rcSafety
	propagateSymBound := func(col int, isUpper bool, bound float64) bool {
		if !hasSymPeers || col >= len(sym.symColOrbit) {
			return true
		}
		orbID := sym.symColOrbit[col]
		if orbID < 0 {
			return true
		}
		g0 := sym.symColGroup[col]
		mask := peerMask[g0]
		if mask == 0 {
			return true
		}
		baseIdx := orbID * sym.symG
		for g := range sym.symG {
			if mask&(uint64(1)<<g) == 0 {
				continue
			}
			cg := sym.symOrbits[baseIdx+g]
			cl, cu := curLB[cg], curUB[cg]
			if isUpper {
				if bound < cu-0.5 {
					if bound < cl-intFeasTol {
						return false
					}
					nb := max(cl, bound)
					curUB[cg] = nb
					dst = append(dst, boundDelta{col: cg, lb: cl, ub: nb})
				}
			} else if bound > cl+0.5 {
				if bound > cu+intFeasTol {
					return false
				}
				nb := min(cu, bound)
				curLB[cg] = nb
				dst = append(dst, boundDelta{col: cg, lb: nb, ub: cu})
			}
		}
		return true
	}
	for _, col := range p.intCols {
		l, u := curLB[col], curUB[col]
		if u-l <= 0.5 {
			continue
		}
		x := lpRes.Primal[col]
		djMin := senseSign * lpRes.ReducedCosts[col]
		if math.IsNaN(djMin) || math.IsInf(djMin, 0) {
			continue
		}
		if !math.IsInf(l, 0) && !isFractionalFloat64(l) && x <= l+primalTol && djMin > minDual {
			rawUB := l + allowedGap/djMin
			if !math.IsNaN(rawUB) && math.Abs(rawUB) < rcMaxBoundMag && (math.IsInf(u, 1) || rawUB < u) {
				newUB := math.Floor(rawUB)
				if newUB < u-0.5 && newUB >= l {
					curUB[col] = newUB
					dst = append(dst, boundDelta{col: col, lb: l, ub: newUB})
					if !propagateSymBound(col, true, newUB) {
						return dst, true
					}
				}
			}
		} else if !math.IsInf(u, 0) && !isFractionalFloat64(u) && x >= u-primalTol && djMin < -minDual {
			rawLB := u - allowedGap/(-djMin)
			if !math.IsNaN(rawLB) && math.Abs(rawLB) < rcMaxBoundMag && (math.IsInf(l, -1) || rawLB > l) {
				newLB := math.Ceil(rawLB)
				if newLB > l+0.5 && newLB <= u {
					curLB[col] = newLB
					dst = append(dst, boundDelta{col: col, lb: newLB, ub: u})
					if !propagateSymBound(col, false, newLB) {
						return dst, true
					}
				}
			}
		}
	}
	return dst, false
}

func findExactIntRows(p *CompiledProblem) []int {
	if p == nil || !p.isMIP {
		return nil
	}
	var rows []int
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if s == e || p.rowSense[i] == Equal {
			continue
		}
		allExact := true
		for k := s; k < e; k++ {
			if p.varCat[p.colIdx[k]] == Continuous || isFractionalFloat64(p.rowVal[k]) {
				allExact = false
				break
			}
		}
		if allExact {
			rows = append(rows, i)
		}
	}
	return rows
}

//go:noinline
func evalExactIntRowViolWide(p *CompiledProblem, row int, x []float64) (viol float64, ok bool) {
	s, e := p.rowPtr[row], p.rowPtr[row+1]
	rhs := p.rowRHS[row]
	if isNonFinite(rhs) {
		return 0, false
	}
	var w wideDotAcc
	w.addVal(-rhs)
	for k := s; k < e; k++ {
		rv := math.Round(x[p.colIdx[k]])
		if isNonFinite(rv) {
			return 0, false
		}
		w.addProd(p.rowVal[k], rv)
	}
	res := w.finish(math.NaN())
	if isNonFinite(res) {
		return 0, false
	}
	switch p.rowSense[row] {
	case LessEqual:
		return max(0.0, res), true
	case GreaterEqual:
		return max(0.0, -res), true
	case Equal:
		return math.Abs(res), true
	default:
		return 0, false
	}
}

func violatesExactIntRows(p *CompiledProblem, exactRows []int, x []float64) bool {
	if len(exactRows) == 0 || len(x) < p.numVars {
		return false
	}
	for _, r := range exactRows {
		s, e := p.rowPtr[r], p.rowPtr[r+1]
		var sum, l1 float64
		nonFin := false
		for k := s; k < e; k++ {
			rv := math.Round(x[p.colIdx[k]])
			if isNonFinite(rv) {
				nonFin = true
				break
			}
			prod := p.rowVal[k] * rv
			sum += prod
			l1 += math.Abs(prod)
		}
		if nonFin {
			return true
		}
		if l1 > 0x1p52 {
			viol, ok := evalExactIntRowViolWide(p, r, x)
			if !ok || viol > 0 {
				return true
			}
			continue
		}
		rhs := p.rowRHS[r]
		switch p.rowSense[r] {
		case LessEqual:
			if sum > rhs {
				return true
			}
		case GreaterEqual:
			if sum < rhs {
				return true
			}
		case Equal:
			if sum != rhs {
				return true
			}
		}
	}
	return false
}

//go:noinline
func exactIntRowActConflictWide(p *CompiledProblem, row int, curLB, curUB []float64, checkMin bool) bool {
	s, e := p.rowPtr[row], p.rowPtr[row+1]
	rhs := p.rowRHS[row]
	if isNonFinite(rhs) {
		return false
	}
	var w wideDotAcc
	w.addVal(-rhs)
	for k := s; k < e; k++ {
		a := p.rowVal[k]
		if a == 0 {
			continue
		}
		col := p.colIdx[k]
		var b float64
		if (a > 0) == checkMin {
			b = math.Ceil(curLB[col])
		} else {
			b = math.Floor(curUB[col])
		}
		if isNonFinite(b) {
			return false
		}
		w.addProd(a, b)
	}
	res := w.finish(math.NaN())
	if isNonFinite(res) {
		return false
	}
	if checkMin {
		return res > 0
	}
	return res < 0
}

func exactIntRowSideConflict(p *CompiledProblem, r int, curLB, curUB []float64, checkMin bool) bool {
	s, e := p.rowPtr[r], p.rowPtr[r+1]
	var act, l1 float64
	for k := s; k < e; k++ {
		a := p.rowVal[k]
		if a == 0 {
			continue
		}
		col := p.colIdx[k]
		var b float64
		if (a > 0) == checkMin {
			b = math.Ceil(curLB[col])
		} else {
			b = math.Floor(curUB[col])
		}
		if isNonFinite(b) {
			return false
		}
		prod := a * b
		act += prod
		l1 += math.Abs(prod)
	}
	if l1 > 0x1p52 {
		return exactIntRowActConflictWide(p, r, curLB, curUB, checkMin)
	}
	if checkMin {
		return act > p.rowRHS[r]
	}
	return act < p.rowRHS[r]
}

func hasExactIntRowActivityConflict(p *CompiledProblem, exactRows []int, curLB, curUB []float64) (int, bool) {
	if len(exactRows) == 0 || len(curLB) < p.numVars || len(curUB) < p.numVars {
		return -1, false
	}
	for _, r := range exactRows {
		sense := p.rowSense[r]
		if isNonFinite(p.rowRHS[r]) {
			continue
		}
		if (sense == LessEqual || sense == Equal) && exactIntRowSideConflict(p, r, curLB, curUB, true) {
			return r, true
		}
		if (sense == GreaterEqual || sense == Equal) && exactIntRowSideConflict(p, r, curLB, curUB, false) {
			return r, true
		}
	}
	return -1, false
}
