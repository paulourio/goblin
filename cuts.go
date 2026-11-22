package goblin

import (
	"cmp"
	"math"
	"slices"
)

const (
	minIntInfeas     = 0.01
	minCutEfficacy   = 1e-3
	maxCutDynamism   = 1e5
	rhsSafetyMargin  = 1e-7
	coverDPTol       = 1e-7
	maxOrthoCosine   = 0.92
	halfFracTarget   = 0.5
	maxGMIFraqRows   = 25
	cutCoeffZeroTol  = 1e-12
	cutDropTol       = 1e-11
	unitCoeffTol     = 1e-9
	cutCompareTol    = 1e-6
	minCoverItemVal  = 1e-4
	minPairNNZ       = 2
	maxFlowLookahead = 6
	maxFlowChainLen  = 64
	maxMIRDivisors   = 4
	minMIRFrac       = 0.02
	mirMaxRelSlack   = 0.05
	aggMaxMu         = 1e4
	aggMinMu         = 1e-4
	aggMaxRelSlack   = 0.05
	maxAggBaseRows   = 96
	maxAggSteps      = 8
	maxAggColDeg     = 64
	maxPairAggRows   = 16
	maxVUBClusterLen = 64
	defaultMaxCuts   = 15
	cutMaxNNZSmallN  = 100
	cutMaxNNZFloor   = 50
	cutMaxNNZCap     = 100
	cutMaxNNZVarDiv  = 4
	cutDenseSoftNNZ  = 24
	cutDensityPen    = 0.01
)

// nextUp stays within the Go inline budget (cost <= 80) to avoid math.Nextafter calls in cut loops.
func nextUp(x float64) float64 {
	if x == 0 {
		return math.SmallestNonzeroFloat64
	}
	if x > 0 {
		if x > math.MaxFloat64 {
			return x
		}
		return math.Float64frombits(math.Float64bits(x) + 1)
	}
	if x < 0 {
		return math.Float64frombits(math.Float64bits(x) - 1)
	}
	return x
}

// nextDown stays within the Go inline budget (cost <= 80) to avoid math.Nextafter calls in cut loops.
func nextDown(x float64) float64 {
	if x == 0 {
		return -math.SmallestNonzeroFloat64
	}
	if x < 0 {
		if x < -math.MaxFloat64 {
			return x
		}
		return math.Float64frombits(math.Float64bits(x) + 1)
	}
	if x > 0 {
		return math.Float64frombits(math.Float64bits(x) - 1)
	}
	return x
}

type cutPlane struct {
	ColIdx   []int
	Val      []float64
	RHS      float64
	Efficacy float64
	Norm     float64
}

type disaggVUBTerm struct {
	col    int
	binCol int
	effUB  float64
}

type flowNode struct {
	invIn   int
	prodCol int
	binCol  int
	invOut  int
	demand  float64
}

type coverItem struct {
	col    int
	weight float64
	xVal   float64
	comp   bool
}

type cutStaticCache struct {
	prob       *CompiledProblem
	disaggVUBs []disaggVUBTerm
	flowChains [][]flowNode
	coverRows  []int
	mirRows    []int
	aggRowOK   []bool
	vubCapRows []int
}

// cutGenWorkspace holds reusable scratch buffers and static structural caches
// across root cutting-plane rounds.
type cutGenWorkspace struct {
	staticCache cutStaticCache

	// Generation-marked sparse column accumulator for finalizeSparseCut.
	mark     []int
	acc      []float64
	gen      int
	uniqCols []int

	// Generation-marked compensated accumulator for GMI cut generation.
	gmiMark []int
	gmiAcc  []dot2Acc
	gmiGen  int
	gmiCols []int

	// Generation-marked compensated accumulator for multi-row cut aggregation.
	aggMark    []int
	aggAcc     []dot2Acc
	aggGen     int
	aggCols    []int
	aggRowUsed []int
	aggRowGen  int
	aggOutCols []int
	aggOutVals []float64

	// Reusable sparse (col, val) buffers shared by sparse separators.
	tmpIdx []int
	tmpVal []float64

	// Reusable scratch slices for knapsack cover DP lifting.
	coverItems   []coverItem
	coverIn      []bool
	coverWeights []float64
	coverMinW    []float64
}

func (w *cutGenWorkspace) ensureVars(n int) {
	if len(w.mark) < n {
		w.mark = make([]int, n)
		w.acc = make([]float64, n)
		w.gen = 0
	}
}

func (w *cutGenWorkspace) nextGen() int {
	w.gen++
	if w.gen <= 0 {
		clear(w.mark)
		w.gen = 1
	}
	return w.gen
}

func (w *cutGenWorkspace) ensureGMIVars(n int) {
	if len(w.gmiMark) < n {
		w.gmiMark = make([]int, n)
		w.gmiAcc = make([]dot2Acc, n)
		w.gmiGen = 0
	}
}

func (w *cutGenWorkspace) nextGMIGen() int {
	w.gmiGen++
	if w.gmiGen <= 0 {
		clear(w.gmiMark)
		w.gmiGen = 1
	}
	return w.gmiGen
}

func (w *cutGenWorkspace) ensureAgg(n, m int) {
	if len(w.aggMark) < n {
		w.aggMark = make([]int, n)
		w.aggAcc = make([]dot2Acc, n)
		w.aggGen = 0
	}
	if len(w.aggRowUsed) < m {
		w.aggRowUsed = make([]int, m)
		w.aggRowGen = 0
	}
}

func (w *cutGenWorkspace) nextAggColGen() int {
	w.aggGen++
	if w.aggGen <= 0 {
		clear(w.aggMark)
		w.aggGen = 1
	}
	return w.aggGen
}

func (w *cutGenWorkspace) nextAggRowGen() int {
	w.aggRowGen++
	if w.aggRowGen <= 0 {
		clear(w.aggRowUsed)
		w.aggRowGen = 1
	}
	return w.aggRowGen
}

func (w *cutGenWorkspace) ensureStaticCache(p *CompiledProblem) *cutStaticCache {
	if w.staticCache.prob != p {
		w.staticCache = buildCutStaticCache(p)
	}
	return &w.staticCache
}

// buildCutStaticCache precomputes static problem structures used across cut rounds:
// disaggregated VUB terms, lot-sizing flow chains, and candidate cover and MIR row indices.
func buildCutStaticCache(p *CompiledProblem) cutStaticCache {
	cache := cutStaticCache{prob: p}
	if p == nil {
		return cache
	}
	n := p.numVars
	m := p.numConstraints

	vubBinCol := make([]int, n)
	for j := range n {
		vubBinCol[j] = -1
	}
	cache.aggRowOK = make([]bool, m)

	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		nnz := e - s
		sense := p.rowSense[i]

		if nnz >= minPairNNZ && !math.IsNaN(p.rowRHS[i]) && !math.IsInf(p.rowRHS[i], 0) {
			rowFiniteNoFree := true
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				rawV := p.rowVal[k]
				if math.IsNaN(rawV) || math.IsInf(rawV, 0) || (math.IsInf(p.varLB[col], -1) && math.IsInf(p.varUB[col], 1)) {
					rowFiniteNoFree = false
					break
				}
			}
			cache.aggRowOK[i] = rowFiniteNoFree
		}

		// Scan inequality rows for VUBs, knapsack covers, and MIR candidates in one pass.
		if sense != Equal && nnz >= minPairNNZ {
			// Check MIR eligibility: no doubly unbounded variable and at least one integer term.
			mirValid := true
			mirHasInt := false
			// Check cover eligibility: at least 3 binary [0,1] items and finite non-binary relax bounds.
			sign := 1.0
			if sense == GreaterEqual {
				sign = -1.0
			}
			coverValid := true
			coverBinCnt := 0

			for k := s; k < e; k++ {
				col := p.colIdx[k]
				rawV := p.rowVal[k]
				lb, ub := p.varLB[col], p.varUB[col]
				if math.IsInf(lb, -1) && math.IsInf(ub, 1) {
					mirValid = false
				}
				if p.varCat[col] != Continuous && math.Abs(rawV) > cutCompareTol {
					mirHasInt = true
				}
				if coverValid {
					a := sign * rawV
					if a != 0 {
						if p.varCat[col] != Binary || lb != 0 || ub != 1 || math.Abs(a) <= cutCoeffZeroTol {
							if a > 0 {
								if math.IsInf(lb, -1) {
									coverValid = false
								}
							} else if math.IsInf(ub, 1) {
								coverValid = false
							}
						} else {
							coverBinCnt++
						}
					}
				}
			}
			if mirValid && mirHasInt {
				cache.mirRows = append(cache.mirRows, i)
			}
			if coverValid && coverBinCnt >= 3 {
				cache.coverRows = append(cache.coverRows, i)
			}
		}

		// Detect VUB constraints (p - M*y <= 0) and disaggregated VUB constraints (sum_k a_k x_k - C*y <= 0).
		if sense != LessEqual || p.rowRHS[i] > 0 || nnz < minPairNNZ {
			continue
		}
		binCol := -1
		capVal := 0.0
		negCount := 0
		allPosNonNeg := true

		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if val < 0 {
				negCount++
				if val < -cutCoeffZeroTol && p.varCat[col] == Binary && p.varLB[col] == 0 && p.varUB[col] == 1 {
					binCol = col
					capVal = -val
				}
			} else if val > 0 {
				if p.varLB[col] < 0 {
					allPosNonNeg = false
				}
			}
		}

		if negCount != 1 || binCol == -1 || !allPosNonNeg {
			continue
		}

		if nnz == minPairNNZ {
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				val := p.rowVal[k]
				if col != binCol && val == 1.0 {
					vubBinCol[col] = binCol
				}
			}
			continue
		}

		cache.vubCapRows = append(cache.vubCapRows, i)

		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if col == binCol {
				continue
			}
			ak := p.rowVal[k]
			if ak <= cutCoeffZeroTol {
				continue
			}
			effUB := min(p.varUB[col], capVal/ak)
			if math.IsInf(effUB, 1) || effUB <= 0 {
				continue
			}
			cache.disaggVUBs = append(cache.disaggVUBs, disaggVUBTerm{
				col:    col,
				binCol: binCol,
				effUB:  effUB,
			})
		}
	}

	// Detect Barany-Van Roy-Wolsey (l, S) lot-sizing flow chains once for the static matrix.
	var candNodes []flowNode
	var invInCount map[int]int
	var invOutCount map[int]int

	for i := range m {
		if p.rowSense[i] != Equal || p.rowRHS[i] <= unitCoeffTol {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		nnz := e - s
		if nnz != 2 && nnz != 3 {
			continue
		}

		prodCol := -1
		invIn := -1
		invOut := -1
		valid := true

		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if val == 1.0 {
				if vubBinCol[col] != -1 && p.varLB[col] >= 0 && prodCol == -1 {
					prodCol = col
				} else if p.varLB[col] >= 0 && invIn == -1 {
					invIn = col
				} else {
					valid = false
					break
				}
			} else if val == -1.0 && p.varLB[col] >= 0 && invOut == -1 {
				invOut = col
			} else {
				valid = false
				break
			}
		}

		if !valid || prodCol == -1 || invOut == -1 {
			continue
		}
		if nnz == 3 && invIn == -1 {
			continue
		}

		if invOutCount == nil {
			invInCount = make(map[int]int)
			invOutCount = make(map[int]int)
		}
		fn := flowNode{
			invIn:   invIn,
			prodCol: prodCol,
			binCol:  vubBinCol[prodCol],
			invOut:  invOut,
			demand:  p.rowRHS[i],
		}
		candNodes = append(candNodes, fn)
		if invIn != -1 {
			invInCount[invIn]++
		}
		invOutCount[invOut]++
	}

	if len(candNodes) == 0 {
		return cache
	}

	nodeByInvIn := make(map[int]flowNode, len(candNodes))
	for _, fn := range candNodes {
		if fn.invIn != -1 && invInCount[fn.invIn] == 1 && invOutCount[fn.invIn] == 1 {
			nodeByInvIn[fn.invIn] = fn
		}
	}

	var startNodes []flowNode
	for _, fn := range candNodes {
		if fn.invIn == -1 || invOutCount[fn.invIn] != 1 || invInCount[fn.invIn] != 1 {
			startNodes = append(startNodes, fn)
		}
	}

	var chain []flowNode
	for _, root := range startNodes {
		chain = chain[:0]
		cur := root
		for {
			chain = append(chain, cur)
			if len(chain) >= maxFlowChainLen || invOutCount[cur.invOut] != 1 || invInCount[cur.invOut] != 1 {
				break
			}
			next, ok := nodeByInvIn[cur.invOut]
			if !ok {
				break
			}
			collision := next.invOut == root.invIn || next.prodCol == root.invIn
			if !collision {
				for _, prev := range chain {
					if next.invOut == prev.invOut || next.invOut == prev.prodCol ||
						next.prodCol == prev.invOut || next.prodCol == prev.prodCol {
						collision = true
						break
					}
				}
			}
			if collision {
				break
			}
			cur = next
		}
		cache.flowChains = append(cache.flowChains, slices.Clone(chain))
	}

	return cache
}

// maxCutNNZ returns the dynamic nonzero cap applied to GMI and MIR cuts to prevent
// dense cuts from filling the sparse LU factors in branch-and-bound.
func maxCutNNZ(n int) int {
	if n <= cutMaxNNZSmallN {
		return n
	}
	return max(cutMaxNNZFloor, min(cutMaxNNZCap, n/cutMaxNNZVarDiv))
}

func cutSelectionScore(c cutPlane, n int) float64 {
	if n <= cutMaxNNZSmallN {
		return c.Efficacy
	}
	excess := len(c.ColIdx) - cutDenseSoftNNZ
	if excess <= 0 {
		return c.Efficacy
	}
	return c.Efficacy / (1.0 + cutDensityPen*float64(excess))
}

// generateCutRound separates GMI, lifted knapsack cover, flow/VUB, single-row MIR, and multi-row aggregated cuts,
// returning up to maxCuts pairwise-orthogonal cuts ordered by descending density-weighted efficacy.
func generateCutRound(solver *LPSolver, lpPrimal []float64, maxCuts int, ws *cutGenWorkspace) []cutPlane {
	return generateCutRoundEx(solver, lpPrimal, maxCuts, minCutEfficacy, ws, false)
}

// generateInTreeCutRound separates globally valid cuts at a branch-and-bound node above minEfficacy.
func generateInTreeCutRound(solver *LPSolver, lpPrimal []float64, maxCuts int, minEfficacy float64, ws *cutGenWorkspace) []cutPlane {
	return generateCutRoundEx(solver, lpPrimal, maxCuts, minEfficacy, ws, true)
}

func generateCutRoundEx(solver *LPSolver, lpPrimal []float64, maxCuts int, minEfficacy float64, ws *cutGenWorkspace, globalOnly bool) []cutPlane {
	if maxCuts <= 0 {
		maxCuts = defaultMaxCuts
	}
	if solver.timeExpired() {
		return nil
	}
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	var candidates []cutPlane

	gmiCuts := separateGMICutsEx(solver, lpPrimal, ws, globalOnly)
	candidates = append(candidates, gmiCuts...)
	if solver.timeExpired() {
		return nil
	}

	coverCuts := separateKnapsackCoverCuts(solver.prob, lpPrimal, ws)
	candidates = append(candidates, coverCuts...)
	if solver.timeExpired() {
		return nil
	}

	flowCuts := separateFlowAndVUBCuts(solver.prob, lpPrimal, ws)
	candidates = append(candidates, flowCuts...)
	if solver.timeExpired() {
		return nil
	}

	mirCuts := separateRowMIRCutsWithWS(solver.prob, lpPrimal, ws)
	candidates = append(candidates, mirCuts...)
	if solver.timeExpired() {
		return nil
	}

	fullAgg := solver.prob.numVars > cutMaxNNZSmallN
	if !globalOnly || fullAgg {
		aggCuts, pairCtx := separateAggregatedCore(solver, lpPrimal, ws, fullAgg)
		candidates = append(candidates, aggCuts...)
		if solver.timeExpired() {
			return nil
		}
		if fullAgg && pairCtx != nil {
			candidates = append(candidates, separatePairwiseSurrogateCuts(pairCtx)...)
		}
	}

	return selectCutBatch(candidates, solver.prob.numVars, maxCuts, minEfficacy)
}

func selectCutBatch(candidates []cutPlane, nVars, maxCuts int, minEfficacy float64) []cutPlane {
	if len(candidates) == 0 {
		return nil
	}
	if minEfficacy > minCutEfficacy {
		kept := candidates[:0]
		for _, c := range candidates {
			if c.Efficacy >= minEfficacy {
				kept = append(kept, c)
			}
		}
		candidates = kept
		if len(candidates) == 0 {
			return nil
		}
	}

	slices.SortFunc(candidates, func(a, b cutPlane) int {
		if c := cmp.Compare(cutSelectionScore(b, nVars), cutSelectionScore(a, nVars)); c != 0 {
			return c
		}
		return cmp.Compare(b.Efficacy, a.Efficacy)
	})

	selected := make([]cutPlane, 0, min(maxCuts, len(candidates)))
	for _, cand := range candidates {
		if len(selected) >= maxCuts {
			break
		}
		tooParallel := false
		for _, sel := range selected {
			if cosineSimilarity(cand, sel) > maxOrthoCosine {
				tooParallel = true
				break
			}
		}
		if !tooParallel {
			selected = append(selected, cand)
		}
	}
	return selected
}

// separateGMICuts derives Gomory mixed-integer cuts from up to maxGMIFraqRows fractional basic integer rows.
func separateGMICuts(solver *LPSolver, lpPrimal []float64, cutWS *cutGenWorkspace) []cutPlane {
	return separateGMICutsEx(solver, lpPrimal, cutWS, false)
}

func separateGMICutsEx(solver *LPSolver, lpPrimal []float64, cutWS *cutGenWorkspace, globalOnly bool) []cutPlane {
	if cutWS == nil {
		var localWS cutGenWorkspace
		cutWS = &localWS
	}
	p := solver.prob
	ws := solver.ws
	n := p.numVars
	m := solver.TotalRows()

	basis := ws.basis[:m]
	varCat := p.varCat[:n]
	varLB := p.varLB[:n]
	varUB := p.varUB[:n]
	status := ws.status[:n+m]

	type gmiRow struct {
		r    int
		bVar int
		f0   float64
		dist float64
	}
	var fracRows []gmiRow
	for r, bVar := range basis {
		if uint(bVar) >= uint(n) || varCat[bVar] == Continuous {
			continue
		}
		xVal := solver.unscaledBasicVal(r)
		f0 := xVal - math.Floor(xVal)
		if f0 < minIntInfeas || f0 > 1.0-minIntInfeas {
			continue
		}
		fracRows = append(fracRows, gmiRow{
			r:    r,
			bVar: bVar,
			f0:   f0,
			dist: math.Abs(f0 - halfFracTarget),
		})
	}
	if len(fracRows) == 0 {
		return nil
	}
	if len(fracRows) > maxGMIFraqRows {
		slices.SortFunc(fracRows, func(a, b gmiRow) int {
			return cmp.Compare(a.dist, b.dist)
		})
		fracRows = fracRows[:maxGMIFraqRows]
	}

	cutWS.ensureGMIVars(n)
	gmiMark := cutWS.gmiMark[:n]
	gmiAcc := cutWS.gmiAcc[:n]
	unscaledLB, unscaledUB := ws.lb[:n], ws.ub[:n]
	if solver.scaled && len(solver.origLB) >= n && len(solver.origUB) >= n {
		unscaledLB, unscaledUB = solver.origLB[:n], solver.origUB[:n]
	}
	gmiMaxNNZ := maxCutNNZ(n)
	var cuts []cutPlane

	evalGMIInt := func(a float64, f0 float64) float64 {
		fk := a - math.Floor(a)
		var v float64
		if fk <= f0 {
			v = fk / f0
		} else {
			v = (1.0 - fk) / (1.0 - f0)
		}
		return nextUp(v)
	}
	evalGMICont := func(a float64, f0 float64) float64 {
		var v float64
		if a >= 0 {
			v = a / f0
		} else {
			v = (-a) / (1.0 - f0)
		}
		return nextUp(v)
	}

	for _, fr := range fracRows {
		r, bVar := fr.r, fr.bVar
		exactXVal, ok := solver.computeExactTableauRow(r)
		if !ok {
			continue
		}
		f0 := exactXVal - math.Floor(exactXVal)
		if f0 < minIntInfeas || f0 > 1.0-minIntInfeas {
			continue
		}

		gen := cutWS.nextGMIGen()
		gmiCols := cutWS.gmiCols[:0]
		var rhsAcc dot2Acc
		rhsAcc.addVal(1.0)
		validRow := true

		for k := range n + m {
			if k == bVar {
				continue
			}
			st := status[k]
			if st == statusBasic {
				continue
			}
			if k >= n && st == statusFixed {
				continue
			}
			if !globalOnly && st == statusFixed {
				continue
			}
			if globalOnly && k < n && varLB[k] == varUB[k] {
				continue
			}
			if st == statusSuperBasic {
				validRow = false
				break
			}
			alpha, alphaErr := solver.unscaledTabEntryWithErr(bVar, k)
			if alpha == 0 && alphaErr == 0 {
				continue
			}
			if math.IsNaN(alpha) || math.IsInf(alpha, 0) || math.IsNaN(alphaErr) || math.IsInf(alphaErr, 0) {
				validRow = false
				break
			}

			effSt := st
			var bndVal float64
			if k < n {
				if st == statusAtUpper {
					bndVal = unscaledUB[k]
				} else {
					bndVal = unscaledLB[k]
				}
				if globalOnly {
					switch bndVal {
					case varLB[k]:
						effSt = statusAtLower
					case varUB[k]:
						effSt = statusAtUpper
					default:
						validRow = false
					}
					if !validRow {
						break
					}
				}
			} else {
				bndVal = solver.unscaledBound(k, st == statusAtUpper)
			}

			alphaTilde := alpha
			if effSt == statusAtUpper {
				alphaTilde = -alpha
			}

			var gamma float64
			isIntVar := k < n && varCat[k] != Continuous
			if isIntVar {
				if math.Abs(bndVal-math.Round(bndVal)) > cutCompareTol {
					isIntVar = false
				}
			}

			if alphaErr == 0 {
				if isIntVar {
					gamma = evalGMIInt(alphaTilde, f0)
				} else {
					gamma = evalGMICont(alphaTilde, f0)
				}
			} else {
				loA := nextDown(alphaTilde - alphaErr)
				hiA := nextUp(alphaTilde + alphaErr)
				if isIntVar {
					if hiA-loA >= 1.0 || math.Floor(hiA-f0) >= math.Ceil(loA-f0) {
						gamma = 1.0
					} else {
						gamma = max(evalGMIInt(loA, f0), evalGMIInt(hiA, f0))
					}
				} else {
					gamma = max(evalGMICont(loA, f0), evalGMICont(hiA, f0))
				}
			}

			if math.IsNaN(gamma) || math.IsInf(gamma, 0) {
				validRow = false
				break
			}
			if gamma <= 0 {
				continue
			}

			if k < n {
				if gmiMark[k] != gen {
					gmiMark[k] = gen
					gmiAcc[k] = dot2Acc{}
					gmiCols = append(gmiCols, k)
				}
				if effSt == statusAtLower {
					gmiAcc[k].addVal(gamma)
					rhsAcc.addProd(gamma, bndVal)
				} else {
					gmiAcc[k].addVal(-gamma)
					rhsAcc.addProd(-gamma, bndVal)
				}
			} else {
				rowIdx := k - n
				if effSt == statusAtLower {
					rhsAcc.addProd(-gamma, solver.unscaledRowRHS(rowIdx))
					rhsAcc.addProd(gamma, bndVal)
					addScaledRowAccSparse(p, solver.extraRows, rowIdx, -gamma, cutWS, gen, &gmiCols)
				} else {
					rhsAcc.addProd(gamma, solver.unscaledRowRHS(rowIdx))
					rhsAcc.addProd(-gamma, bndVal)
					addScaledRowAccSparse(p, solver.extraRows, rowIdx, +gamma, cutWS, gen, &gmiCols)
				}
			}
		}

		cutWS.gmiCols = gmiCols
		rhsVal, rhsErr, okRHS := rhsAcc.result()
		if !validRow || !okRHS || len(gmiCols) == 0 {
			continue
		}

		slices.Sort(gmiCols)
		extraLHSErr := 0.0
		needCols := len(gmiCols)
		if cap(cutWS.tmpIdx) < needCols {
			cutWS.tmpIdx = make([]int, needCols)
		}
		if cap(cutWS.tmpVal) < needCols {
			cutWS.tmpVal = make([]float64, needCols)
		}
		idxBuf := cutWS.tmpIdx[:needCols]
		valBuf := cutWS.tmpVal[:needCols]
		outLen := 0
		for _, j := range gmiCols {
			if uint(j) >= uint(n) {
				continue
			}
			cv, ce, okC := gmiAcc[j].result()
			if !okC {
				validRow = false
				break
			}
			if cv != 0 && outLen < len(idxBuf) {
				idxBuf[outLen] = j
				valBuf[outLen] = -cv
				outLen++
			}
			if ce > 0 {
				maxB := max(math.Abs(varLB[j]), math.Abs(varUB[j]))
				if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
					validRow = false
					break
				}
				extraLHSErr += nextUp(ce * maxB)
				if math.IsInf(extraLHSErr, 0) || math.IsNaN(extraLHSErr) {
					validRow = false
					break
				}
			}
		}
		idxBuf = idxBuf[:outLen]
		valBuf = valBuf[:outLen]
		cutWS.tmpIdx = idxBuf
		cutWS.tmpVal = valBuf
		if !validRow || outLen == 0 {
			continue
		}

		cutRHS := (-rhsVal + rhsErr) + extraLHSErr
		if rhsErr+extraLHSErr > 0 && !math.IsInf(cutRHS, 0) && !math.IsNaN(cutRHS) {
			cutRHS = nextUp(cutRHS)
		}

		if cut, ok := finalizeSparseCut(p, idxBuf, valBuf, cutRHS, lpPrimal, gmiMaxNNZ, cutWS); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func addScaledRowAccSparse(p *CompiledProblem, extra []dynamicRow, rowIdx int, scale float64, cutWS *cutGenWorkspace, gen int, cols *[]int) {
	n := p.numVars
	gmiMark := cutWS.gmiMark[:n]
	gmiAcc := cutWS.gmiAcc[:n]
	if rowIdx < p.numConstraints {
		st, en := p.rowPtr[rowIdx], p.rowPtr[rowIdx+1]
		rCols := p.colIdx[st:en]
		rVals := p.rowVal[st:en]
		rVals = rVals[:len(rCols)]
		for k, col := range rCols {
			if uint(col) >= uint(n) {
				continue
			}
			if gmiMark[col] != gen {
				gmiMark[col] = gen
				gmiAcc[col] = dot2Acc{}
				*cols = append(*cols, col)
			}
			gmiAcc[col].addProd(scale, rVals[k])
		}
		return
	}
	dr := extra[rowIdx-p.numConstraints]
	vals := dr.Val
	if dr.OrigVal != nil {
		vals = dr.OrigVal
	}
	dCols := dr.ColIdx
	vals = vals[:len(dCols)]
	for k, col := range dCols {
		if uint(col) >= uint(n) {
			continue
		}
		if gmiMark[col] != gen {
			gmiMark[col] = gen
			gmiAcc[col] = dot2Acc{}
			*cols = append(*cols, col)
		}
		gmiAcc[col].addProd(scale, vals[k])
	}
}

// separateKnapsackCoverCuts relaxes inequality rows to 0-1 knapsacks and sequentially lifts minimal covers via dynamic programming.
func separateKnapsackCoverCuts(p *CompiledProblem, lpPrimal []float64, ws *cutGenWorkspace) []cutPlane {
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	cache := ws.ensureStaticCache(p)
	if len(cache.coverRows) == 0 {
		return nil
	}
	var cuts []cutPlane

	for _, i := range cache.coverRows {
		sign := 1.0
		if p.rowSense[i] == GreaterEqual {
			sign = -1.0
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if cut, ok := separateCoverFromSparseRow(p, p.colIdx[s:e], p.rowVal[s:e], p.rowRHS[i], sign, lpPrimal, 0, ws); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func separateCoverFromSparseRow(
	p *CompiledProblem,
	cols []int,
	vals []float64,
	rawRHS, sign float64,
	lpPrimal []float64,
	maxNNZ int,
	ws *cutGenWorkspace,
) (cutPlane, bool) {
	if len(cols) < 3 {
		return cutPlane{}, false
	}
	var rhsAcc dot2Acc
	rhsAcc.addVal(sign * rawRHS)

	validRelax := true
	items := ws.coverItems[:0]
	minItemW := math.Inf(1)
	for k, col := range cols {
		a := sign * vals[k]
		if a == 0 {
			continue
		}
		if p.varCat[col] != Binary || p.varLB[col] != 0 || p.varUB[col] != 1 || math.Abs(a) <= cutCoeffZeroTol {
			if a > 0 {
				if math.IsInf(p.varLB[col], -1) {
					validRelax = false
					break
				}
				rhsAcc.addProd(-a, p.varLB[col])
			} else {
				if math.IsInf(p.varUB[col], 1) {
					validRelax = false
					break
				}
				rhsAcc.addProd(-a, p.varUB[col])
			}
			continue
		}
		if a > 0 {
			minItemW = min(minItemW, a)
			items = append(items, coverItem{
				col:    col,
				weight: a,
				xVal:   lpPrimal[col],
				comp:   false,
			})
		} else {
			minItemW = min(minItemW, -a)
			rhsAcc.addVal(-a)
			items = append(items, coverItem{
				col:    col,
				weight: -a,
				xVal:   1.0 - lpPrimal[col],
				comp:   true,
			})
		}
	}
	ws.coverItems = items

	rhsVal, rhsErr, okRHS := rhsAcc.result()
	rhs := rhsVal + rhsErr
	if !validRelax || !okRHS || len(items) < 3 || !(rhs > 0) || math.IsInf(rhs, 0) || rhsErr > 1e-4*minItemW {
		return cutPlane{}, false
	}

	slices.SortFunc(items, func(a, b coverItem) int {
		if math.Abs(a.xVal-b.xVal) > cutCompareTol {
			return cmp.Compare(b.xVal, a.xVal)
		}
		return cmp.Compare(b.weight, a.weight)
	})

	if len(ws.coverIn) < len(items) {
		ws.coverIn = make([]bool, len(items))
	}
	inCover := ws.coverIn[:len(items)]
	clear(inCover)

	coverWeight := 0.0
	coverSize := 0
	coverMargin := func(cw float64) float64 {
		return max(cutCompareTol, 16.0*unitRoundoff64*(cw+math.Abs(rhs)))
	}
	for idx, it := range items {
		if it.xVal <= minCoverItemVal {
			break
		}
		inCover[idx] = true
		coverWeight += it.weight
		coverSize++
		if coverWeight > rhs+coverMargin(coverWeight) {
			break
		}
	}
	if coverWeight <= rhs+coverMargin(coverWeight) || coverSize < minPairNNZ {
		return cutPlane{}, false
	}

	for idx := len(items) - 1; idx >= 0; idx-- {
		if !inCover[idx] {
			continue
		}
		candW := coverWeight - items[idx].weight
		if candW > rhs+coverMargin(coverWeight) {
			inCover[idx] = false
			coverWeight = candW
			coverSize--
		}
	}
	if coverSize < minPairNNZ {
		return cutPlane{}, false
	}

	// Initialize sequential DP up-lifting table over the minimal cover C:
	// minWeight[v] = minimum knapsack weight required to reach cut LHS activity >= v.
	coverWeights := ws.coverWeights[:0]
	for idx, it := range items {
		if inCover[idx] {
			coverWeights = append(coverWeights, it.weight)
		}
	}
	ws.coverWeights = coverWeights
	slices.Sort(coverWeights)

	if len(ws.coverMinW) < coverSize {
		ws.coverMinW = make([]float64, coverSize)
	}
	minWeight := ws.coverMinW[:coverSize]
	minWeight[0] = 0
	for v := 1; v < coverSize; v++ {
		s := minWeight[v-1] + coverWeights[v-1]
		minWeight[v] = max(0.0, s-8.0*unitRoundoff64*s)
	}

	cutCap := float64(coverSize - 1)
	finalRHS := cutCap
	lhsStar := 0.0
	idxBuf := ws.tmpIdx[:0]
	valBuf := ws.tmpVal[:0]

	for idx, it := range items {
		var alpha int
		if inCover[idx] {
			alpha = 1
		} else {
			remCap := rhs - it.weight
			remTol := max(coverDPTol, 16.0*unitRoundoff64*(math.Abs(rhs)+it.weight))
			zMax := 0
			for v := coverSize - 1; v >= 1; v-- {
				dpTol := max(remTol, 16.0*unitRoundoff64*minWeight[v])
				if minWeight[v] <= remCap+dpTol {
					zMax = v
					break
				}
			}
			if remCap < -remTol {
				alpha = coverSize - 1
			} else {
				alpha = (coverSize - 1) - zMax
			}
			if alpha > 0 {
				for v := coverSize - 1; v >= 1; v-- {
					prev := max(0, v-alpha)
					cand := minWeight[prev] + it.weight
					cand = max(0.0, cand-8.0*unitRoundoff64*cand)
					if cand < minWeight[v] {
						minWeight[v] = cand
					}
				}
			}
		}
		if alpha <= 0 {
			continue
		}
		fAlpha := float64(alpha)
		lhsStar += fAlpha * it.xVal
		idxBuf = append(idxBuf, it.col)
		if !it.comp {
			valBuf = append(valBuf, fAlpha)
		} else {
			valBuf = append(valBuf, -fAlpha)
			finalRHS -= fAlpha
		}
	}
	ws.tmpIdx = idxBuf
	ws.tmpVal = valBuf
	if lhsStar <= cutCap+minCutEfficacy {
		return cutPlane{}, false
	}

	return finalizeSparseCut(p, idxBuf, valBuf, finalRHS, lpPrimal, maxNNZ, ws)
}

// separateFlowAndVUBCuts separates disaggregated variable-upper-bound inequalities and Barany-Van Roy-Wolsey (l, S) lot-sizing cuts.
func separateFlowAndVUBCuts(p *CompiledProblem, lpPrimal []float64, ws *cutGenWorkspace) []cutPlane {
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	cache := ws.ensureStaticCache(p)
	if len(cache.disaggVUBs) == 0 && len(cache.flowChains) == 0 {
		return nil
	}
	var cuts []cutPlane

	// 1. Disaggregated VUB cuts: for each x_k in sum_k a_k x_k - C*y <= 0, x_k - min(u_k, C/a_k)*y <= 0.
	var vubIdx [2]int
	var vubVal [2]float64
	for _, term := range cache.disaggVUBs {
		if lpPrimal[term.col]-term.effUB*lpPrimal[term.binCol] > minCutEfficacy {
			vubIdx[0], vubIdx[1] = term.col, term.binCol
			vubVal[0], vubVal[1] = 1.0, -term.effUB
			if cut, ok := finalizeSparseCut(p, vubIdx[:], vubVal[:], 0, lpPrimal, 0, ws); ok {
				cuts = append(cuts, cut)
			}
		}
	}

	// 2. Barany-Van Roy-Wolsey (l, S) lot-sizing / flow-conservation cuts over cached chains.
	var seenBins [maxFlowLookahead + 1]int
	var flowIdx [maxFlowLookahead + 2]int
	var flowVal [maxFlowLookahead + 2]float64
	for _, chain := range cache.flowChains {
		T := len(chain)
		for t := range T {
			maxQ := min(T-1, t+maxFlowLookahead)
			for q := t; q <= maxQ; q++ {
				cumDem := 0.0
				for s := t; s <= q; s++ {
					cumDem += chain[s].demand
				}

				lhsStar := 0.0
				if chain[t].invIn != -1 {
					lhsStar += lpPrimal[chain[t].invIn]
				}
				remDem := cumDem
				anyInS := false
				numSeen := 0
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					alreadyInS := false
					for k := range numSeen {
						if seenBins[k] == nd.binCol {
							alreadyInS = true
							break
						}
					}
					if alreadyInS {
						remDem -= nd.demand
						continue
					}
					yTerm := remDem * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-cutCompareTol {
						seenBins[numSeen] = nd.binCol
						numSeen++
						lhsStar += yTerm
						anyInS = true
					} else {
						lhsStar += pTerm
					}
					remDem -= nd.demand
				}

				if !anyInS || lhsStar >= cumDem-minCutEfficacy {
					continue
				}

				nf := 0
				if chain[t].invIn != -1 {
					flowIdx[nf] = chain[t].invIn
					flowVal[nf] = -1.0
					nf++
				}
				remDem = cumDem
				numSeen = 0
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					alreadyInS := false
					for k := range numSeen {
						if seenBins[k] == nd.binCol {
							alreadyInS = true
							break
						}
					}
					if alreadyInS {
						remDem -= nd.demand
						continue
					}
					yTerm := remDem * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-cutCompareTol {
						seenBins[numSeen] = nd.binCol
						numSeen++
						flowIdx[nf] = nd.binCol
						flowVal[nf] = -remDem
						nf++
					} else {
						flowIdx[nf] = nd.prodCol
						flowVal[nf] = -1.0
						nf++
					}
					remDem -= nd.demand
				}

				if cut, ok := finalizeSparseCut(p, flowIdx[:nf], flowVal[:nf], -cumDem, lpPrimal, 0, ws); ok {
					cuts = append(cuts, cut)
				}
			}
		}
	}

	return cuts
}

func mirUseLower(x, l, u float64) bool {
	if u == math.Inf(1) {
		return true
	}
	if l == math.Inf(-1) {
		return false
	}
	return x-l <= u-x
}

// separateRowMIRCuts separates single-row Marchand-Wolsey complemented Mixed-Integer Rounding cuts over bounded and one-sided rows.
func separateRowMIRCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	var localWS cutGenWorkspace
	return separateRowMIRCutsWithWS(p, lpPrimal, &localWS)
}

func separateRowMIRCutsWithWS(p *CompiledProblem, lpPrimal []float64, ws *cutGenWorkspace) []cutPlane {
	cache := ws.ensureStaticCache(p)
	if len(cache.mirRows) == 0 {
		return nil
	}
	mirMaxNNZ := maxCutNNZ(p.numVars)
	var cuts []cutPlane

	for _, i := range cache.mirRows {
		sign := 1.0
		if p.rowSense[i] == GreaterEqual {
			sign = -1.0
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		cuts = separateCMIRFromSparseRow(p, p.colIdx[s:e], p.rowVal[s:e], p.rowRHS[i], sign, lpPrimal, mirMaxNNZ, maxMIRDivisors, ws, cuts)
	}

	return cuts
}

func separateCMIRFromSparseRow(
	p *CompiledProblem,
	cols []int,
	vals []float64,
	rawRHS, sign float64,
	lpPrimal []float64,
	maxNNZ, maxDivs int,
	ws *cutGenWorkspace,
	cuts []cutPlane,
) []cutPlane {
	if len(cols) < minPairNNZ {
		return cuts
	}
	n := p.numVars
	vals = vals[:len(cols)]
	varLB := p.varLB[:n]
	varUB := p.varUB[:n]
	varCat := p.varCat[:n]
	if len(lpPrimal) >= n {
		lpPrimal = lpPrimal[:n]
	}

	rhs := sign * rawRHS
	act := 0.0
	for k, col := range cols {
		if uint(col) >= uint(n) {
			return cuts
		}
		if math.IsInf(varLB[col], -1) && math.IsInf(varUB[col], 1) {
			return cuts
		}
		act += sign * vals[k] * lpPrimal[col]
	}
	if rhs-act > 1.0+mirMaxRelSlack*math.Abs(rhs) {
		return cuts
	}

	// Try Marchand-Wolsey c-MIR scaling with divisor delta = |a_k| for fractional integer variables.
	// Hoist shifted-RHS betaAcc out of the kDiv loop since it does not depend on delta.
	var triedDeltas [8]float64
	triedDivs := 0
	betaComputed := false
	var betaVal, betaErr float64
	var okBeta bool
	for kDiv, divCol := range cols {
		if triedDivs >= maxDivs || triedDivs >= len(triedDeltas) {
			break
		}
		if uint(divCol) >= uint(n) || varCat[divCol] == Continuous {
			continue
		}
		xv := lpPrimal[divCol]
		fx := xv - math.Floor(xv)
		if fx < minIntInfeas || fx > 1.0-minIntInfeas {
			continue
		}
		delta := math.Abs(vals[kDiv])
		if delta <= cutCompareTol {
			continue
		}
		dupDelta := false
		for idx := range triedDivs {
			if triedDeltas[idx] == delta {
				dupDelta = true
				break
			}
		}
		if dupDelta {
			continue
		}
		triedDeltas[triedDivs] = delta
		triedDivs++

		if !betaComputed {
			var betaAcc dot2Acc
			betaAcc.addVal(rhs)
			for k, col := range cols {
				a := sign * vals[k]
				l, u := varLB[col], varUB[col]
				if mirUseLower(lpPrimal[col], l, u) {
					betaAcc.addProd(-a, l)
				} else {
					betaAcc.addProd(-a, u)
				}
			}
			betaVal, betaErr, okBeta = betaAcc.result()
			betaComputed = true
		}
		if !okBeta {
			break
		}
		if betaErr > 1e-4*delta {
			continue
		}

		betaHat := (betaVal + betaErr) / delta
		betaHat += 4.0 * unitRoundoff64 * math.Abs(betaHat)
		if math.IsNaN(betaHat) || math.IsInf(betaHat, 0) {
			continue
		}
		b0 := math.Floor(betaHat)
		f0 := betaHat - b0
		if !(f0 >= minMIRFrac && f0 <= 1.0-minMIRFrac) {
			continue
		}

		var cutRHSAcc dot2Acc
		cutRHSAcc.addVal(b0)
		contDenLo := nextDown(delta * nextDown(1.0-f0))
		if !(contDenLo > 0) || math.IsInf(contDenLo, 0) || math.IsNaN(contDenLo) {
			continue
		}
		intDenHi := nextUp(1.0 - f0)
		needCols := len(cols)
		if cap(ws.tmpIdx) < needCols {
			ws.tmpIdx = make([]int, needCols)
		}
		if cap(ws.tmpVal) < needCols {
			ws.tmpVal = make([]float64, needCols)
		}
		idxBuf := ws.tmpIdx[:needCols]
		valBuf := ws.tmpVal[:needCols]
		outLen := 0
		for k, col := range cols {
			a := sign * vals[k]
			l, u := varLB[col], varUB[col]
			if l == u {
				continue
			}
			atLower := mirUseLower(lpPrimal[col], l, u)

			if varCat[col] == Continuous {
				if a > 0 {
					if atLower {
						continue
					}
					g := nextUp(a / contDenLo)
					if g != 0 && outLen < len(idxBuf) {
						idxBuf[outLen] = col
						valBuf[outLen] = g
						outLen++
					}
					cutRHSAcc.addProd(g, u)
				} else if a < 0 {
					if !atLower {
						continue
					}
					g := nextUp((-a) / contDenLo)
					if g != 0 && outLen < len(idxBuf) {
						idxBuf[outLen] = col
						valBuf[outLen] = -g
						outLen++
					}
					cutRHSAcc.addProd(-g, l)
				}
			} else {
				comp := !atLower
				num := a
				if comp {
					num = -a
				}
				alpha := num / delta
				if alpha != 0 {
					alpha = nextDown(alpha)
				}
				flA := math.Floor(alpha)
				fj := alpha - flA
				coeff := flA
				if fj > f0 {
					numLo := nextDown(fj - f0)
					if numLo > 0 {
						fracLo := nextDown(numLo / intDenHi)
						if fracLo > 0 {
							coeff = nextDown(flA + fracLo)
						}
					}
				}
				if !comp {
					if coeff != 0 && outLen < len(idxBuf) {
						idxBuf[outLen] = col
						valBuf[outLen] = coeff
						outLen++
					}
					cutRHSAcc.addProd(coeff, l)
				} else {
					if coeff != 0 && outLen < len(idxBuf) {
						idxBuf[outLen] = col
						valBuf[outLen] = -coeff
						outLen++
					}
					cutRHSAcc.addProd(-coeff, u)
				}
			}
		}
		idxBuf = idxBuf[:outLen]
		valBuf = valBuf[:outLen]
		ws.tmpIdx = idxBuf
		ws.tmpVal = valBuf
		if outLen == 0 {
			continue
		}
		cutRHSVal, cutRHSErr, okCutRHS := cutRHSAcc.result()
		if !okCutRHS {
			continue
		}
		cutRHS := cutRHSVal + cutRHSErr
		if cutRHSErr > 0 && !math.IsInf(cutRHS, 0) && !math.IsNaN(cutRHS) {
			cutRHS = nextUp(cutRHS)
		}
		if cut, ok := finalizeSparseCut(p, idxBuf, valBuf, cutRHS, lpPrimal, maxNNZ, ws); ok {
			cuts = append(cuts, cut)
		}
	}
	return cuts
}

func rowLegalMuSign(sense ConstraintSense, mu float64) bool {
	switch sense {
	case Equal:
		return mu != 0
	case LessEqual:
		return mu > 0
	case GreaterEqual:
		return mu < 0
	default:
		return false
	}
}

func addScaledProblemRowToAgg(
	p *CompiledProblem,
	r int,
	mu float64,
	rhsAcc *dot2Acc,
	ws *cutGenWorkspace,
	colGen int,
	cols *[]int,
) {
	rhsAcc.addProd(mu, p.rowRHS[r])
	n := p.numVars
	st, en := p.rowPtr[r], p.rowPtr[r+1]
	rCols := p.colIdx[st:en]
	rVals := p.rowVal[st:en]
	rVals = rVals[:len(rCols)]
	aggMark := ws.aggMark[:n]
	aggAcc := ws.aggAcc[:n]
	for k, col := range rCols {
		if uint(col) >= uint(n) {
			continue
		}
		if aggMark[col] != colGen {
			aggMark[col] = colGen
			aggAcc[col] = dot2Acc{}
			*cols = append(*cols, col)
		}
		aggAcc[col].addProd(mu, rVals[k])
	}
}

func materializeAggRow(
	p *CompiledProblem,
	rhsAcc dot2Acc,
	cols []int,
	maxNNZ int,
	ws *cutGenWorkspace,
) (outCols []int, outVals []float64, aggRHS float64, ok bool) {
	rhsVal, rhsErr, okRHS := rhsAcc.result()
	if !okRHS || math.IsNaN(rhsVal) || math.IsInf(rhsVal, 0) || math.IsNaN(rhsErr) || math.IsInf(rhsErr, 0) {
		return nil, nil, 0, false
	}
	n := p.numVars
	aggAcc := ws.aggAcc[:n]
	varLB := p.varLB[:n]
	varUB := p.varUB[:n]
	outCols = ws.aggOutCols[:0]
	outVals = ws.aggOutVals[:0]
	extraLHSErr := 0.0
	for _, j := range cols {
		if uint(j) >= uint(n) {
			continue
		}
		cv, ce, okC := aggAcc[j].result()
		if !okC || math.IsNaN(cv) || math.IsInf(cv, 0) {
			return nil, nil, 0, false
		}
		if ce > 0 {
			maxB := max(math.Abs(varLB[j]), math.Abs(varUB[j]))
			if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
				return nil, nil, 0, false
			}
			extraLHSErr += nextUp(ce * maxB)
			if !math.IsInf(extraLHSErr, 0) && !math.IsNaN(extraLHSErr) {
				extraLHSErr = nextUp(extraLHSErr)
			} else {
				return nil, nil, 0, false
			}
		}
		if cv != 0 {
			outCols = append(outCols, j)
			outVals = append(outVals, cv)
			if maxNNZ > 0 && len(outCols) > maxNNZ {
				ws.aggOutCols = outCols
				ws.aggOutVals = outVals
				return nil, nil, 0, false
			}
		}
	}
	ws.aggOutCols = outCols
	ws.aggOutVals = outVals
	if len(outCols) < minPairNNZ {
		return nil, nil, 0, false
	}
	errBound := rhsErr + extraLHSErr
	if errBound > 0 && !math.IsInf(errBound, 0) && !math.IsNaN(errBound) {
		errBound = nextUp(errBound)
	}
	aggRHS = rhsVal + errBound
	if errBound > 0 && !math.IsInf(aggRHS, 0) && !math.IsNaN(aggRHS) {
		aggRHS = nextUp(aggRHS)
	}
	if math.IsNaN(aggRHS) || math.IsInf(aggRHS, 0) {
		return nil, nil, 0, false
	}
	return outCols, outVals, aggRHS, true
}

type aggCutContext struct {
	solver        *LPSolver
	p             *CompiledProblem
	cache         *cutStaticCache
	lpPrimal      []float64
	ws            *cutGenWorkspace
	mirMaxNNZ     int
	isTight       []bool
	relSlack      []float64
	rowDual       []float64
	rowHasFracInt []bool
}

// separateAggregatedMIRCuts derives multi-row aggregated MIR and cover cuts via
// variable-elimination chains, pairwise tight-inequality surrogates, and VUB capacity sums.
func separateAggregatedMIRCuts(solver *LPSolver, lpPrimal []float64, ws *cutGenWorkspace) []cutPlane {
	cuts, ctx := separateAggregatedCore(solver, lpPrimal, ws, true)
	if ctx != nil && !solver.timeExpired() {
		cuts = append(cuts, separatePairwiseSurrogateCuts(ctx)...)
	}
	return cuts
}

func separateAggregatedCore(solver *LPSolver, lpPrimal []float64, ws *cutGenWorkspace, fullAgg bool) ([]cutPlane, *aggCutContext) {
	if solver.timeExpired() {
		return nil, nil
	}
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	p := solver.prob
	cache := ws.ensureStaticCache(p)
	n := p.numVars
	m := p.numConstraints
	if n < minPairNNZ || m < 2 {
		return nil, nil
	}
	mirMaxNNZ := maxCutNNZ(n)
	ws.ensureAgg(n, m)

	isTight := make([]bool, m)
	relSlack := make([]float64, m)
	rowDual := make([]float64, m)
	rowHasFracInt := make([]bool, m)
	rowHasCont := make([]bool, m)
	rowHasInterior := make([]bool, m)
	hasDuals := solver.dualValid && solver.ws != nil && len(solver.ws.y) >= m

	for i := range m {
		if !cache.aggRowOK[i] {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		act := 0.0
		hasFracInt := false
		hasCont := false
		hasInterior := false
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			xv := lpPrimal[col]
			act += p.rowVal[k] * xv
			if xv > p.varLB[col]+1e-5 && xv < p.varUB[col]-1e-5 {
				hasInterior = true
			}
			if p.varCat[col] == Continuous {
				hasCont = true
			} else {
				f := xv - math.Floor(xv)
				if f >= minIntInfeas && f <= 1.0-minIntInfeas {
					hasFracInt = true
				}
			}
		}
		var slack float64
		switch p.rowSense[i] {
		case Equal:
			slack = math.Abs(p.rowRHS[i] - act)
		case LessEqual:
			slack = p.rowRHS[i] - act
		case GreaterEqual:
			slack = act - p.rowRHS[i]
		}
		scale := max(1.0, math.Abs(p.rowRHS[i]))
		rs := slack / scale
		if slack >= -1e-4*scale && rs <= aggMaxRelSlack {
			isTight[i] = true
			relSlack[i] = max(0.0, rs)
			rowHasFracInt[i] = hasFracInt
			rowHasCont[i] = hasCont
			rowHasInterior[i] = hasInterior
			if hasDuals {
				rowDual[i] = math.Abs(solver.ws.y[i] * solver.rowScaleOf(i))
			}
		}
	}

	var cuts []cutPlane

	// Mechanism A: Marchand-Wolsey variable-elimination chains.
	var baseRows []int
	for i := range m {
		if !isTight[i] || p.rowPtr[i+1]-p.rowPtr[i] > mirMaxNNZ {
			continue
		}
		if !fullAgg && (p.rowSense[i] != Equal || !rowHasCont[i]) {
			continue
		}
		if rowHasCont[i] || rowHasFracInt[i] || rowHasInterior[i] {
			baseRows = append(baseRows, i)
		}
	}
	if len(baseRows) > 1 {
		slices.SortFunc(baseRows, func(a, b int) int {
			aEq := p.rowSense[a] == Equal
			bEq := p.rowSense[b] == Equal
			if aEq != bEq {
				if aEq {
					return -1
				}
				return 1
			}
			if rowHasCont[a] != rowHasCont[b] {
				if rowHasCont[a] {
					return -1
				}
				return 1
			}
			aInt := rowHasInterior[a] || rowHasFracInt[a]
			bInt := rowHasInterior[b] || rowHasFracInt[b]
			if aInt != bInt {
				if aInt {
					return -1
				}
				return 1
			}
			nnzA := p.rowPtr[a+1] - p.rowPtr[a]
			nnzB := p.rowPtr[b+1] - p.rowPtr[b]
			if nnzA != nnzB {
				return cmp.Compare(nnzA, nnzB)
			}
			return cmp.Compare(a, b)
		})
	}
	if len(baseRows) > maxAggBaseRows {
		baseRows = baseRows[:maxAggBaseRows]
	}

	var signsBuf [2]float64
	for baseIdx, r0 := range baseRows {
		if (baseIdx&7 == 0) && solver.timeExpired() {
			return cuts, nil
		}
		nSigns := 1
		switch p.rowSense[r0] {
		case Equal:
			signsBuf[0], signsBuf[1] = 1.0, -1.0
			nSigns = 2
		case LessEqual:
			signsBuf[0] = 1.0
		case GreaterEqual:
			signsBuf[0] = -1.0
		}

		for _, mu0 := range signsBuf[:nSigns] {
			colGen := ws.nextAggColGen()
			rowGen := ws.nextAggRowGen()
			aggCols := ws.aggCols[:0]
			var rhsAcc dot2Acc

			ws.aggRowUsed[r0] = rowGen
			addScaledProblemRowToAgg(p, r0, mu0, &rhsAcc, ws, colGen, &aggCols)
			outCols, outVals, _, ok := materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws)
			if !ok {
				ws.aggCols = aggCols
				continue
			}

			for range maxAggSteps {
				bestTier := 99
				bestEq := 99
				bestNNZ := math.MaxInt
				bestLogMu := math.Inf(1)
				bestRow := -1
				bestMu := 0.0

				for idx, col := range outCols {
					curA := outVals[idx]
					if math.Abs(curA) <= cutCoeffZeroTol || p.varLB[col] == p.varUB[col] {
						continue
					}
					if p.colPtr[col+1]-p.colPtr[col] > maxAggColDeg {
						continue
					}
					xv := lpPrimal[col]
					interior := xv > p.varLB[col]+1e-5 && xv < p.varUB[col]-1e-5
					var tier int
					switch {
					case p.varCat[col] == Continuous && interior:
						tier = 0
					case p.varCat[col] == Continuous:
						tier = 1
					case p.varCat[col] == Integer && p.varUB[col]-p.varLB[col] > 1.5 && interior:
						tier = 2
					default:
						continue
					}
					if tier > bestTier {
						continue
					}

					for ck := p.colPtr[col]; ck < p.colPtr[col+1]; ck++ {
						r2 := p.rowIdx[ck]
						if ws.aggRowUsed[r2] == rowGen || !cache.aggRowOK[r2] || !isTight[r2] {
							continue
						}
						r2NNZ := p.rowPtr[r2+1] - p.rowPtr[r2]
						if r2NNZ > mirMaxNNZ {
							continue
						}
						mu := -curA / p.colVal[ck]
						absMu := math.Abs(mu)
						if math.IsNaN(mu) || math.IsInf(mu, 0) || absMu < aggMinMu || absMu > aggMaxMu || !rowLegalMuSign(p.rowSense[r2], mu) {
							continue
						}
						eqScore := 1
						if p.rowSense[r2] == Equal {
							eqScore = 0
						}
						logMu := math.Abs(math.Log(absMu))
						if tier < bestTier ||
							(tier == bestTier && (eqScore < bestEq ||
								(eqScore == bestEq && (r2NNZ < bestNNZ ||
									(r2NNZ == bestNNZ && logMu < bestLogMu))))) {
							bestTier = tier
							bestEq = eqScore
							bestNNZ = r2NNZ
							bestLogMu = logMu
							bestRow = r2
							bestMu = mu
						}
					}
				}
				if bestRow == -1 {
					break
				}
				ws.aggRowUsed[bestRow] = rowGen
				addScaledProblemRowToAgg(p, bestRow, bestMu, &rhsAcc, ws, colGen, &aggCols)
				var aggRHS float64
				outCols, outVals, aggRHS, ok = materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws)
				if !ok {
					break
				}
				cuts = separateCMIRFromSparseRow(p, outCols, outVals, aggRHS, 1.0, lpPrimal, mirMaxNNZ, maxMIRDivisors+2, ws, cuts)
			}
			ws.aggCols = aggCols
		}
	}

	if solver.timeExpired() {
		return cuts, nil
	}

	// Mechanism C: VUB/capacity sum followed by unit partitioning/demand row elimination.
	vubRows := cache.vubCapRows
	if len(vubRows) >= 2 && len(vubRows) <= maxVUBClusterLen {
		var tightVUBs []int
		for _, r := range vubRows {
			if cache.aggRowOK[r] && isTight[r] {
				tightVUBs = append(tightVUBs, r)
			}
		}
		subsets := [2][]int{tightVUBs, nil}
		if len(tightVUBs) < len(vubRows) {
			allOK := make([]int, 0, len(vubRows))
			for _, r := range vubRows {
				if cache.aggRowOK[r] {
					allOK = append(allOK, r)
				}
			}
			subsets[1] = allOK
		}
		for _, sub := range subsets {
			if len(sub) < 2 {
				continue
			}
			colGen := ws.nextAggColGen()
			rowGen := ws.nextAggRowGen()
			aggCols := ws.aggCols[:0]
			var rhsAcc dot2Acc
			for _, r := range sub {
				ws.aggRowUsed[r] = rowGen
				addScaledProblemRowToAgg(p, r, 1.0, &rhsAcc, ws, colGen, &aggCols)
			}
			elimCount := 0
			for i := range m {
				if ws.aggRowUsed[i] == rowGen || !cache.aggRowOK[i] {
					continue
				}
				s, e := p.rowPtr[i], p.rowPtr[i+1]
				if e-s < minPairNNZ {
					continue
				}
				c0 := p.colIdx[s]
				a0 := p.rowVal[s]
				if ws.aggMark[c0] != colGen || math.Abs(a0) <= cutCoeffZeroTol {
					continue
				}
				cv0, _, ok0 := ws.aggAcc[c0].result()
				if !ok0 || math.Abs(cv0) <= cutCoeffZeroTol {
					continue
				}
				mu := -cv0 / a0
				absMu := math.Abs(mu)
				if math.IsNaN(mu) || math.IsInf(mu, 0) || absMu < aggMinMu || absMu > aggMaxMu || !rowLegalMuSign(p.rowSense[i], mu) {
					continue
				}
				canElim := true
				for k := s + 1; k < e; k++ {
					col := p.colIdx[k]
					if ws.aggMark[col] != colGen {
						canElim = false
						break
					}
					cv, _, okC := ws.aggAcc[col].result()
					if !okC || math.Abs(cv+mu*p.rowVal[k]) > 1e-9*max(1.0, math.Abs(cv)) {
						canElim = false
						break
					}
				}
				if !canElim {
					continue
				}
				ws.aggRowUsed[i] = rowGen
				addScaledProblemRowToAgg(p, i, mu, &rhsAcc, ws, colGen, &aggCols)
				elimCount++
			}
			if elimCount > 0 {
				if outCols, outVals, aggRHS, ok := materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws); ok {
					cuts = separateCMIRFromSparseRow(p, outCols, outVals, aggRHS, 1.0, lpPrimal, mirMaxNNZ, maxMIRDivisors+2, ws, cuts)
					if cov, covOK := separateCoverFromSparseRow(p, outCols, outVals, aggRHS, 1.0, lpPrimal, mirMaxNNZ, ws); covOK {
						cuts = append(cuts, cov)
					}
				}
			}
			ws.aggCols = aggCols
		}
	}

	ctx := &aggCutContext{
		solver:        solver,
		p:             p,
		cache:         cache,
		lpPrimal:      lpPrimal,
		ws:            ws,
		mirMaxNNZ:     mirMaxNNZ,
		isTight:       isTight,
		relSlack:      relSlack,
		rowDual:       rowDual,
		rowHasFracInt: rowHasFracInt,
	}
	return cuts, ctx
}

// separatePairwiseSurrogateCuts forms pairwise tight-inequality surrogates when primary
// separators leave cut budget remaining.
func separatePairwiseSurrogateCuts(ctx *aggCutContext) []cutPlane {
	p := ctx.p
	n := p.numVars
	m := p.numConstraints
	ws := ctx.ws
	pairNNZCap := ctx.mirMaxNNZ

	var pairRows []int
	for i := range m {
		if !ctx.isTight[i] || p.rowSense[i] == Equal || !ctx.rowHasFracInt[i] || p.rowPtr[i+1]-p.rowPtr[i] > pairNNZCap {
			continue
		}
		pairRows = append(pairRows, i)
	}
	if len(pairRows) < 2 {
		return nil
	}
	slices.SortFunc(pairRows, func(a, b int) int {
		if c := cmp.Compare(ctx.relSlack[a], ctx.relSlack[b]); c != 0 {
			return c
		}
		if c := cmp.Compare(ctx.rowDual[b], ctx.rowDual[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	if len(pairRows) > maxPairAggRows {
		pairRows = pairRows[:maxPairAggRows]
	}

	isFracCol := make([]bool, n)
	for _, col := range p.intCols {
		f := ctx.lpPrimal[col] - math.Floor(ctx.lpPrimal[col])
		if f >= minIntInfeas && f <= 1.0-minIntInfeas {
			isFracCol[col] = true
		}
	}
	rowSign := func(r int) float64 {
		if p.rowSense[r] == GreaterEqual {
			return -1.0
		}
		return 1.0
	}
	rowMaxAbs := func(r int) float64 {
		mx := 0.0
		for k := p.rowPtr[r]; k < p.rowPtr[r+1]; k++ {
			mx = max(mx, math.Abs(p.rowVal[k]))
		}
		return mx
	}

	var cuts []cutPlane
	fracMark := make([]int, n)
	fracGen := 0
	var muBuf [2]float64
	for ai := range len(pairRows) - 1 {
		if ctx.solver.timeExpired() {
			return cuts
		}
		r1 := pairRows[ai]
		fracGen++
		for k := p.rowPtr[r1]; k < p.rowPtr[r1+1]; k++ {
			if c := p.colIdx[k]; isFracCol[c] {
				fracMark[c] = fracGen
			}
		}
		s1 := rowSign(r1)
		max1 := rowMaxAbs(r1)
		for bi := ai + 1; bi < len(pairRows); bi++ {
			r2 := pairRows[bi]
			sharedFrac := false
			for k := p.rowPtr[r2]; k < p.rowPtr[r2+1]; k++ {
				if fracMark[p.colIdx[k]] == fracGen {
					sharedFrac = true
					break
				}
			}
			if !sharedFrac {
				continue
			}
			s2 := rowSign(r2)
			max2 := rowMaxAbs(r2)
			muBuf[0] = 1.0
			nMu := 1
			if max1 > 0 && max2 > 0 {
				ratio := max1 / max2
				if (ratio < 0.95 || ratio > 1.05) && ratio >= 1e-3 && ratio <= 1e3 {
					muBuf[1] = ratio
					nMu = 2
				}
			}
			for _, mu := range muBuf[:nMu] {
				colGen := ws.nextAggColGen()
				aggCols := ws.aggCols[:0]
				var rhsAcc dot2Acc
				addScaledProblemRowToAgg(p, r1, s1, &rhsAcc, ws, colGen, &aggCols)
				addScaledProblemRowToAgg(p, r2, s2*mu, &rhsAcc, ws, colGen, &aggCols)
				outCols, outVals, aggRHS, ok := materializeAggRow(p, rhsAcc, aggCols, pairNNZCap, ws)
				ws.aggCols = aggCols
				if !ok {
					continue
				}
				cuts = separateCMIRFromSparseRow(p, outCols, outVals, aggRHS, 1.0, ctx.lpPrimal, pairNNZCap, maxMIRDivisors, ws, cuts)
				if cov, covOK := separateCoverFromSparseRow(p, outCols, outVals, aggRHS, 1.0, ctx.lpPrimal, pairNNZCap, ws); covOK {
					cuts = append(cuts, cov)
				}
			}
		}
	}
	return cuts
}

// finalizeSparseCut converts a sparse cut sum_k val[k]*x[idx[k]] <= rhs into a power-of-two
// normalized sparse cut row in O(nnz) time without scanning all variables. Duplicate column
// entries are summed and output columns are ordered ascending. When maxNNZ > 0, cuts whose
// surviving nonzero count exceeds maxNNZ are rejected before output slice allocation.
func finalizeSparseCut(
	p *CompiledProblem,
	idx []int,
	val []float64,
	rhs float64,
	lpPrimal []float64,
	maxNNZ int,
	ws *cutGenWorkspace,
) (cutPlane, bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) || len(idx) == 0 || len(idx) != len(val) {
		return cutPlane{}, false
	}
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	n := p.numVars
	ws.ensureVars(n)
	gen := ws.nextGen()
	uniq := ws.uniqCols[:0]
	mark := ws.mark[:n]
	acc := ws.acc[:n]
	varLB := p.varLB[:n]
	varUB := p.varUB[:n]
	val = val[:len(idx)]
	sorted := true
	prevCol := -1

	for k, col := range idx {
		v := val[k]
		if uint(col) >= uint(n) || math.IsNaN(v) || math.IsInf(v, 0) {
			return cutPlane{}, false
		}
		if mark[col] != gen {
			mark[col] = gen
			acc[col] = v
			if col < prevCol {
				sorted = false
			}
			prevCol = col
			uniq = append(uniq, col)
		} else {
			acc[col] += v
			if math.IsNaN(acc[col]) || math.IsInf(acc[col], 0) {
				return cutPlane{}, false
			}
		}
	}
	ws.uniqCols = uniq
	if !sorted {
		if len(uniq) == 2 {
			uniq[0], uniq[1] = uniq[1], uniq[0]
		} else {
			slices.Sort(uniq)
		}
	}

	maxAbs := 0.0
	minAbs := math.Inf(1)
	nnz := 0
	for _, j := range uniq {
		if uint(j) >= uint(n) {
			return cutPlane{}, false
		}
		v := acc[j]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return cutPlane{}, false
		}
		av := math.Abs(v)
		if av > cutDropTol {
			maxAbs = max(maxAbs, av)
			minAbs = min(minAbs, av)
			uniq[nnz] = j
			nnz++
		} else if v != 0 {
			// Dropping v*x_j relaxes the cut only after adding max(-v*x_j) = -min(v*l_j, v*u_j).
			p1 := nextDown(v * varLB[j])
			p2 := nextDown(v * varUB[j])
			lo := min(p1, p2)
			if math.IsInf(lo, 0) || math.IsNaN(lo) {
				return cutPlane{}, false
			}
			rhs = nextUp(rhs - lo)
			if math.IsInf(rhs, 0) || math.IsNaN(rhs) {
				return cutPlane{}, false
			}
		}
	}
	if nnz == 0 || maxAbs == 0 || maxAbs/minAbs > maxCutDynamism {
		return cutPlane{}, false
	}
	if maxNNZ > 0 && nnz > maxNNZ {
		return cutPlane{}, false
	}
	kept := uniq[:nnz]

	scaledNorm2 := 0.0
	for _, j := range kept {
		r := ws.acc[j] / maxAbs
		scaledNorm2 += r * r
	}
	norm := maxAbs * math.Sqrt(scaledNorm2)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return cutPlane{}, false
	}
	_, rawExp := math.Frexp(norm)
	normExp := rawExp - 1
	scaledNorm := math.Ldexp(norm, -normExp)
	if !(scaledNorm > 0) || math.IsNaN(scaledNorm) || math.IsInf(scaledNorm, 0) {
		return cutPlane{}, false
	}

	act := 0.0
	subScaleErr := 0.0
	for _, j := range kept {
		if uint(j) >= uint(n) {
			return cutPlane{}, false
		}
		v := acc[j]
		sv := math.Ldexp(v, -normExp)
		if sv == 0 || math.IsNaN(sv) || math.IsInf(sv, 0) {
			return cutPlane{}, false
		}
		if math.Ldexp(sv, normExp) != v {
			maxB := max(math.Abs(varLB[j]), math.Abs(varUB[j]))
			if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
				return cutPlane{}, false
			}
			subScaleErr += nextUp(math.SmallestNonzeroFloat64 * maxB)
		}
		acc[j] = sv
		act += sv * lpPrimal[j]
	}
	rawRHS := math.Ldexp(rhs, -normExp)
	if rhs != 0 && math.Ldexp(rawRHS, normExp) != rhs {
		rawRHS = nextUp(rawRHS)
	}
	scaledRHS := (rawRHS + subScaleErr) + rhsSafetyMargin*scaledNorm
	if math.IsNaN(scaledRHS) || math.IsInf(scaledRHS, 0) || math.IsNaN(act) || math.IsInf(act, 0) {
		return cutPlane{}, false
	}
	efficacy := (act - scaledRHS) / scaledNorm
	if !(efficacy >= minCutEfficacy) {
		return cutPlane{}, false
	}

	colIdx := make([]int, nnz)
	outVal := make([]float64, nnz)
	for k, j := range kept {
		colIdx[k] = j
		outVal[k] = ws.acc[j]
	}
	return cutPlane{
		ColIdx:   colIdx,
		Val:      outVal,
		RHS:      scaledRHS,
		Efficacy: efficacy,
		Norm:     scaledNorm,
	}, true
}

// finalizeCut converts the dense cut dense*x <= rhs into a power-of-two normalized sparse cut row.
// Coefficients with magnitude in (0, cutDropTol] are removed; to keep the cut valid, the
// largest possible value of each dropped term -a_j*x_j over the global bounds of x_j is added
// to the right-hand side, and the cut is rejected if that bound is infinite.
func finalizeCut(p *CompiledProblem, dense []float64, rhs float64, lpPrimal []float64) (cutPlane, bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return cutPlane{}, false
	}
	maxAbs := 0.0
	minAbs := math.Inf(1)
	nnz := 0

	for j, v := range dense {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return cutPlane{}, false
		}
		av := math.Abs(v)
		if av > cutDropTol {
			maxAbs = max(maxAbs, av)
			minAbs = min(minAbs, av)
			nnz++
		} else if v != 0 {
			// Dropping v*x_j relaxes the cut only after adding max(-v*x_j) = -min(v*l_j, v*u_j).
			p1 := nextDown(v * p.varLB[j])
			p2 := nextDown(v * p.varUB[j])
			lo := min(p1, p2)
			if math.IsInf(lo, 0) || math.IsNaN(lo) {
				return cutPlane{}, false
			}
			rhs = nextUp(rhs - lo)
			if math.IsInf(rhs, 0) || math.IsNaN(rhs) {
				return cutPlane{}, false
			}
		}
	}
	if nnz == 0 || maxAbs == 0 || maxAbs/minAbs > maxCutDynamism {
		return cutPlane{}, false
	}

	scaledNorm2 := 0.0
	for _, v := range dense {
		if math.Abs(v) > cutDropTol {
			r := v / maxAbs
			scaledNorm2 += r * r
		}
	}
	norm := maxAbs * math.Sqrt(scaledNorm2)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return cutPlane{}, false
	}
	_, rawExp := math.Frexp(norm)
	normExp := rawExp - 1
	scaledNorm := math.Ldexp(norm, -normExp)
	if !(scaledNorm > 0) || math.IsNaN(scaledNorm) || math.IsInf(scaledNorm, 0) {
		return cutPlane{}, false
	}

	colIdx := make([]int, 0, nnz)
	val := make([]float64, 0, nnz)
	act := 0.0
	subScaleErr := 0.0
	for j, v := range dense {
		if math.Abs(v) > cutDropTol {
			sv := math.Ldexp(v, -normExp)
			if sv == 0 || math.IsNaN(sv) || math.IsInf(sv, 0) {
				return cutPlane{}, false
			}
			if math.Ldexp(sv, normExp) != v {
				maxB := max(math.Abs(p.varLB[j]), math.Abs(p.varUB[j]))
				if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
					return cutPlane{}, false
				}
				subScaleErr += nextUp(math.SmallestNonzeroFloat64 * maxB)
			}
			colIdx = append(colIdx, j)
			val = append(val, sv)
			act += sv * lpPrimal[j]
		}
	}
	rawRHS := math.Ldexp(rhs, -normExp)
	if rhs != 0 && math.Ldexp(rawRHS, normExp) != rhs {
		rawRHS = nextUp(rawRHS)
	}
	scaledRHS := (rawRHS + subScaleErr) + rhsSafetyMargin*scaledNorm
	if math.IsNaN(scaledRHS) || math.IsInf(scaledRHS, 0) || math.IsNaN(act) || math.IsInf(act, 0) {
		return cutPlane{}, false
	}
	efficacy := (act - scaledRHS) / scaledNorm
	if !(efficacy >= minCutEfficacy) {
		return cutPlane{}, false
	}

	return cutPlane{
		ColIdx:   colIdx,
		Val:      val,
		RHS:      scaledRHS,
		Efficacy: efficacy,
		Norm:     scaledNorm,
	}, true
}

func cutPlaneNorm(c cutPlane) float64 {
	if c.Norm > 0 {
		return c.Norm
	}
	var norm2 float64
	for _, v := range c.Val {
		norm2 += v * v
	}
	return math.Sqrt(norm2)
}

func cosineSimilarity(a, b cutPlane) float64 {
	denom := cutPlaneNorm(a) * cutPlaneNorm(b)
	if !(denom > 0) || math.IsNaN(denom) || math.IsInf(denom, 0) {
		return 0
	}
	i, j := 0, 0
	dot := 0.0
	for i < len(a.ColIdx) && j < len(b.ColIdx) {
		ca, cb := a.ColIdx[i], b.ColIdx[j]
		if ca == cb {
			dot += a.Val[i] * b.Val[j]
			i++
			j++
		} else if ca < cb {
			i++
		} else {
			j++
		}
	}
	return math.Abs(dot) / denom
}
