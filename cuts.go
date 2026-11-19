package goblin

import (
	"cmp"
	"math"
	"slices"

	"github.com/paulourio/goblin/internal/simd"
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
	defaultMaxCuts   = 15
)

type cutPlane struct {
	ColIdx   []int
	Val      []float64
	RHS      float64
	Efficacy float64
}

// generateCutRound separates GMI, lifted knapsack cover, flow/VUB, and single-row MIR cuts,
// returning up to maxCuts pairwise-orthogonal cuts ordered by descending efficacy.
func generateCutRound(solver *LPSolver, lpPrimal []float64, maxCuts int) []cutPlane {
	if maxCuts <= 0 {
		maxCuts = defaultMaxCuts
	}
	if solver.timeExpired() {
		return nil
	}
	var candidates []cutPlane

	gmiCuts := separateGMICuts(solver, lpPrimal)
	candidates = append(candidates, gmiCuts...)
	if solver.timeExpired() {
		return nil
	}

	coverCuts := separateKnapsackCoverCuts(solver.prob, lpPrimal)
	candidates = append(candidates, coverCuts...)
	if solver.timeExpired() {
		return nil
	}

	flowCuts := separateFlowAndVUBCuts(solver.prob, lpPrimal)
	candidates = append(candidates, flowCuts...)
	if solver.timeExpired() {
		return nil
	}

	mirCuts := separateRowMIRCuts(solver.prob, lpPrimal)
	candidates = append(candidates, mirCuts...)

	if len(candidates) == 0 {
		return nil
	}

	slices.SortFunc(candidates, func(a, b cutPlane) int {
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
func separateGMICuts(solver *LPSolver, lpPrimal []float64) []cutPlane {
	p := solver.prob
	ws := solver.ws
	n := p.numVars
	m := solver.TotalRows()

	type gmiRow struct {
		r    int
		bVar int
		f0   float64
		dist float64
	}
	var fracRows []gmiRow
	for r := range m {
		bVar := ws.basis[r]
		if bVar >= n || p.varCat[bVar] == Continuous {
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

	var cuts []cutPlane
	structCoeffs := make([]float64, n)
	coeffAcc := make([]dot2Acc, n)

	evalGMIInt := func(a float64, f0 float64) float64 {
		fk := a - math.Floor(a)
		if fk <= f0 {
			return math.Nextafter(fk/f0, math.Inf(1))
		}
		return math.Nextafter((1.0-fk)/(1.0-f0), math.Inf(1))
	}
	evalGMICont := func(a float64, f0 float64) float64 {
		if a >= 0 {
			return math.Nextafter(a/f0, math.Inf(1))
		}
		return math.Nextafter((-a)/(1.0-f0), math.Inf(1))
	}

	for _, fr := range fracRows {
		r, bVar, f0 := fr.r, fr.bVar, fr.f0
		if !solver.computeExactTableauRow(r) {
			continue
		}

		clear(coeffAcc)
		var rhsAcc dot2Acc
		rhsAcc.addVal(1.0)
		validRow := true

		for k := range n + m {
			if k == bVar {
				continue
			}
			st := ws.status[k]
			if st == statusBasic || st == statusFixed {
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

			alphaTilde := alpha
			if st == statusAtUpper {
				alphaTilde = -alpha
			}

			var gamma float64
			isIntVar := k < n && p.varCat[k] != Continuous
			if isIntVar {
				bnd := solver.unscaledBound(k, st == statusAtUpper)
				if math.Abs(bnd-math.Round(bnd)) > cutCompareTol {
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
				loA := math.Nextafter(alphaTilde-alphaErr, math.Inf(-1))
				hiA := math.Nextafter(alphaTilde+alphaErr, math.Inf(1))
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
				if st == statusAtLower {
					coeffAcc[k].addVal(gamma)
					rhsAcc.addProd(gamma, solver.unscaledBound(k, false))
				} else {
					coeffAcc[k].addVal(-gamma)
					rhsAcc.addProd(-gamma, solver.unscaledBound(k, true))
				}
			} else {
				rowIdx := k - n
				if st == statusAtLower {
					rhsAcc.addProd(-gamma, solver.unscaledRowRHS(rowIdx))
					rhsAcc.addProd(gamma, solver.unscaledBound(k, false))
					addScaledRowAcc(p, solver.extraRows, rowIdx, -gamma, coeffAcc)
				} else {
					rhsAcc.addProd(gamma, solver.unscaledRowRHS(rowIdx))
					rhsAcc.addProd(-gamma, solver.unscaledBound(k, true))
					addScaledRowAcc(p, solver.extraRows, rowIdx, +gamma, coeffAcc)
				}
			}
		}

		rhsVal, rhsErr, okRHS := rhsAcc.result()
		if !validRow || !okRHS {
			continue
		}

		extraLHSErr := 0.0
		for j := range n {
			cv, ce, okC := coeffAcc[j].result()
			if !okC {
				validRow = false
				break
			}
			structCoeffs[j] = cv
			if ce > 0 {
				maxB := max(math.Abs(p.varLB[j]), math.Abs(p.varUB[j]))
				if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
					validRow = false
					break
				}
				extraLHSErr += math.Nextafter(ce*maxB, math.Inf(1))
				if math.IsInf(extraLHSErr, 0) || math.IsNaN(extraLHSErr) {
					validRow = false
					break
				}
			}
		}
		if !validRow {
			continue
		}

		if solver.DisableSIMD {
			simd.Scale64Scalar(structCoeffs, -1.0)
		} else {
			simd.Scale64(structCoeffs, -1.0)
		}
		cutRHS := (-rhsVal + rhsErr) + extraLHSErr
		if rhsErr+extraLHSErr > 0 && !math.IsInf(cutRHS, 0) && !math.IsNaN(cutRHS) {
			cutRHS = math.Nextafter(cutRHS, math.Inf(1))
		}

		if cut, ok := finalizeCut(p, structCoeffs, cutRHS, lpPrimal); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func addScaledRowAcc(p *CompiledProblem, extra []dynamicRow, rowIdx int, scale float64, dst []dot2Acc) {
	if rowIdx < p.numConstraints {
		for k := p.rowPtr[rowIdx]; k < p.rowPtr[rowIdx+1]; k++ {
			dst[p.colIdx[k]].addProd(scale, p.rowVal[k])
		}
		return
	}
	dr := extra[rowIdx-p.numConstraints]
	vals := dr.Val
	if dr.OrigVal != nil {
		vals = dr.OrigVal
	}
	for k, col := range dr.ColIdx {
		dst[col].addProd(scale, vals[k])
	}
}

// separateKnapsackCoverCuts relaxes inequality rows to 0-1 knapsacks and sequentially lifts minimal covers via dynamic programming.
func separateKnapsackCoverCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.numVars
	m := p.numConstraints
	var cuts []cutPlane

	type item struct {
		col    int
		weight float64
		xVal   float64
		comp   bool
	}
	var items []item
	denseCut := make([]float64, n)

	for i := range m {
		if p.rowSense[i] == Equal {
			continue
		}
		sign := 1.0
		if p.rowSense[i] == GreaterEqual {
			sign = -1.0
		}
		var rhsAcc dot2Acc
		rhsAcc.addVal(sign * p.rowRHS[i])

		validRelax := true
		items = items[:0]
		minItemW := math.Inf(1)
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			a := sign * p.rowVal[k]
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
				items = append(items, item{
					col:    col,
					weight: a,
					xVal:   lpPrimal[col],
					comp:   false,
				})
			} else {
				minItemW = min(minItemW, -a)
				rhsAcc.addVal(-a)
				items = append(items, item{
					col:    col,
					weight: -a,
					xVal:   1.0 - lpPrimal[col],
					comp:   true,
				})
			}
		}

		rhsVal, rhsErr, okRHS := rhsAcc.result()
		rhs := rhsVal + rhsErr
		if !validRelax || !okRHS || len(items) < 3 || !(rhs > 0) || math.IsInf(rhs, 0) || rhsErr > 1e-4*minItemW {
			continue
		}

		slices.SortFunc(items, func(a, b item) int {
			if math.Abs(a.xVal-b.xVal) > cutCompareTol {
				return cmp.Compare(b.xVal, a.xVal)
			}
			return cmp.Compare(b.weight, a.weight)
		})

		inCover := make([]bool, len(items))
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
			continue
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
			continue
		}

		// Initialize sequential DP up-lifting table over the minimal cover C:
		// minWeight[v] = minimum knapsack weight required to reach cut LHS activity >= v.
		coverWeights := make([]float64, 0, coverSize)
		for idx, it := range items {
			if inCover[idx] {
				coverWeights = append(coverWeights, it.weight)
			}
		}
		slices.Sort(coverWeights)
		minWeight := make([]float64, coverSize)
		for v := 1; v < coverSize; v++ {
			s := minWeight[v-1] + coverWeights[v-1]
			minWeight[v] = max(0.0, s-8.0*unitRoundoff64*s)
		}

		cutCap := float64(coverSize - 1)
		clear(denseCut)
		finalRHS := cutCap
		lhsStar := 0.0

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
			if !it.comp {
				denseCut[it.col] = fAlpha
			} else {
				denseCut[it.col] = -fAlpha
				finalRHS -= fAlpha
			}
		}
		if lhsStar <= cutCap+minCutEfficacy {
			continue
		}

		if cut, ok := finalizeCut(p, denseCut, finalRHS, lpPrimal); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

// separateFlowAndVUBCuts separates disaggregated variable-upper-bound inequalities and Barany-Van Roy-Wolsey (l, S) lot-sizing cuts.
func separateFlowAndVUBCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.numVars
	m := p.numConstraints
	var cuts []cutPlane

	vubBinCol := make([]int, n)
	for j := range n {
		vubBinCol[j] = -1
	}

	denseCut := make([]float64, n)

	// 1. Detect VUB constraints (p - M*y <= 0) and Disaggregated Implied VUB constraints (sum_k a_k x_k - C*y <= 0)
	for i := range m {
		if p.rowSense[i] != LessEqual || p.rowRHS[i] > 0 {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		nnz := e - s
		if nnz < minPairNNZ {
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

		// Disaggregated VUB cuts: for each x_k in sum_k a_k x_k - C*y <= 0, x_k - min(u_k, C/a_k)*y <= 0
		yStar := lpPrimal[binCol]
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
			if lpPrimal[col]-effUB*yStar > minCutEfficacy {
				clear(denseCut)
				denseCut[col] = 1.0
				denseCut[binCol] = -effUB
				if cut, ok := finalizeCut(p, denseCut, 0, lpPrimal); ok {
					cuts = append(cuts, cut)
				}
			}
		}
	}

	// 2. Barany-Van Roy-Wolsey (l, S) Lot-Sizing / Flow-Conservation cuts
	type flowNode struct {
		invIn   int
		prodCol int
		binCol  int
		invOut  int
		demand  float64
	}
	var candNodes []flowNode
	invInCount := make(map[int]int)
	invOutCount := make(map[int]int)

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
	var seenBins [maxFlowLookahead + 1]int
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

				clear(denseCut)
				if chain[t].invIn != -1 {
					denseCut[chain[t].invIn] -= 1.0
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
						denseCut[nd.binCol] -= remDem
					} else {
						denseCut[nd.prodCol] -= 1.0
					}
					remDem -= nd.demand
				}

				if cut, ok := finalizeCut(p, denseCut, -cumDem, lpPrimal); ok {
					cuts = append(cuts, cut)
				}
			}
		}
	}

	return cuts
}

func mirUseLower(x, l, u float64) bool {
	if math.IsInf(u, 1) {
		return true
	}
	if math.IsInf(l, -1) {
		return false
	}
	return x-l <= u-x
}

// separateRowMIRCuts separates single-row Marchand-Wolsey complemented Mixed-Integer Rounding cuts over bounded and one-sided rows.
func separateRowMIRCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.numVars
	m := p.numConstraints
	var cuts []cutPlane
	denseCut := make([]float64, n)

	for i := range m {
		if p.rowSense[i] == Equal {
			continue
		}
		sign := 1.0
		if p.rowSense[i] == GreaterEqual {
			sign = -1.0
		}
		rhs := sign * p.rowRHS[i]
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if e-s < minPairNNZ {
			continue
		}

		// Only separate on rows with small slack in the LP relaxation
		act := 0.0
		validBounds := true
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			act += sign * p.rowVal[k] * lpPrimal[col]
			if math.IsInf(p.varLB[col], -1) && math.IsInf(p.varUB[col], 1) {
				validBounds = false
				break
			}
		}
		if !validBounds || rhs-act > 1.0+mirMaxRelSlack*math.Abs(rhs) {
			continue
		}

		// Try Marchand-Wolsey c-MIR scaling with divisor delta = |a_k| for fractional integer variables
		triedDivs := 0
		for kDiv := s; kDiv < e && triedDivs < maxMIRDivisors; kDiv++ {
			divCol := p.colIdx[kDiv]
			if p.varCat[divCol] == Continuous {
				continue
			}
			xv := lpPrimal[divCol]
			fx := xv - math.Floor(xv)
			if fx < minIntInfeas || fx > 1.0-minIntInfeas {
				continue
			}
			delta := math.Abs(p.rowVal[kDiv])
			if delta <= cutCompareTol {
				continue
			}
			triedDivs++

			// Shift variables to closer finite bound using compensated summation.
			var betaAcc dot2Acc
			betaAcc.addVal(rhs)
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				a := sign * p.rowVal[k]
				l, u := p.varLB[col], p.varUB[col]
				if mirUseLower(lpPrimal[col], l, u) {
					betaAcc.addProd(-a, l)
				} else {
					betaAcc.addProd(-a, u)
				}
			}
			betaVal, betaErr, okBeta := betaAcc.result()
			if !okBeta || betaErr > 1e-4*delta {
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

			clear(denseCut)
			var cutRHSAcc dot2Acc
			cutRHSAcc.addVal(b0)
			contDenLo := math.Nextafter(delta*math.Nextafter(1.0-f0, math.Inf(-1)), math.Inf(-1))
			if !(contDenLo > 0) || math.IsInf(contDenLo, 0) || math.IsNaN(contDenLo) {
				continue
			}
			intDenHi := math.Nextafter(1.0-f0, math.Inf(1))
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				a := sign * p.rowVal[k]
				l, u := p.varLB[col], p.varUB[col]
				if l == u {
					continue
				}
				atLower := mirUseLower(lpPrimal[col], l, u)

				if p.varCat[col] == Continuous {
					if a > 0 {
						if atLower {
							// Relax a*(x - l) >= 0 from LHS: coefficient 0
							continue
						}
						// x = u - s (s >= 0) => -a*s on LHS => -g*(u - x) in MIR; rounding g up relaxes the cut.
						g := math.Nextafter(a/contDenLo, math.Inf(1))
						denseCut[col] = g
						cutRHSAcc.addProd(g, u)
					} else if a < 0 {
						if !atLower {
							// Relax a*(x - u) = |a|*(u - x) >= 0 from LHS: coefficient 0
							continue
						}
						// x = l + s (s >= 0) => -|a|*s on LHS => -g*(x - l) in MIR; rounding g up relaxes the cut.
						g := math.Nextafter((-a)/contDenLo, math.Inf(1))
						denseCut[col] = -g
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
						alpha = math.Nextafter(alpha, math.Inf(-1))
					}
					flA := math.Floor(alpha)
					fj := alpha - flA
					coeff := flA
					if fj > f0 {
						numLo := math.Nextafter(fj-f0, math.Inf(-1))
						if numLo > 0 {
							fracLo := math.Nextafter(numLo/intDenHi, math.Inf(-1))
							if fracLo > 0 {
								coeff = math.Nextafter(flA+fracLo, math.Inf(-1))
							}
						}
					}
					if !comp {
						denseCut[col] = coeff
						cutRHSAcc.addProd(coeff, l)
					} else {
						denseCut[col] = -coeff
						cutRHSAcc.addProd(-coeff, u)
					}
				}
			}
			cutRHSVal, cutRHSErr, okCutRHS := cutRHSAcc.result()
			if !okCutRHS {
				continue
			}
			cutRHS := cutRHSVal + cutRHSErr
			if cutRHSErr > 0 && !math.IsInf(cutRHS, 0) && !math.IsNaN(cutRHS) {
				cutRHS = math.Nextafter(cutRHS, math.Inf(1))
			}
			if cut, ok := finalizeCut(p, denseCut, cutRHS, lpPrimal); ok {
				cuts = append(cuts, cut)
			}
		}
	}

	return cuts
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
			p1 := math.Nextafter(v*p.varLB[j], math.Inf(-1))
			p2 := math.Nextafter(v*p.varUB[j], math.Inf(-1))
			lo := min(p1, p2)
			if math.IsInf(lo, 0) || math.IsNaN(lo) {
				return cutPlane{}, false
			}
			rhs = math.Nextafter(rhs-lo, math.Inf(1))
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
				subScaleErr += math.Nextafter(math.SmallestNonzeroFloat64*maxB, math.Inf(1))
			}
			colIdx = append(colIdx, j)
			val = append(val, sv)
			act += sv * lpPrimal[j]
		}
	}
	rawRHS := math.Ldexp(rhs, -normExp)
	if rhs != 0 && math.Ldexp(rawRHS, normExp) != rhs {
		rawRHS = math.Nextafter(rawRHS, math.Inf(1))
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
	}, true
}

func cosineSimilarity(a, b cutPlane) float64 {
	var norm2A, norm2B float64
	for _, va := range a.Val {
		norm2A += va * va
	}
	for _, vb := range b.Val {
		norm2B += vb * vb
	}
	denom := math.Sqrt(norm2A * norm2B)
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
