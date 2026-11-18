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
	gmiZeroTol       = 1e-13
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

	for _, fr := range fracRows {
		r, bVar, f0 := fr.r, fr.bVar, fr.f0
		solver.ComputeTableauRow(r)

		clear(structCoeffs)
		rhs := 1.0
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
			alpha := solver.unscaledTabEntry(bVar, k)
			oppFinite := false
			if k < n {
				if st == statusAtLower {
					oppFinite = !math.IsInf(solver.unscaledBound(k, true), 1)
				} else {
					oppFinite = !math.IsInf(solver.unscaledBound(k, false), -1)
				}
			}
			if alpha == 0 || (!oppFinite && math.Abs(alpha) <= gmiZeroTol) {
				continue
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

			if isIntVar {
				fk := alphaTilde - math.Floor(alphaTilde)
				if !oppFinite && (fk <= gmiZeroTol || fk >= 1.0-gmiZeroTol) {
					continue
				}
				if fk <= f0 {
					gamma = fk / f0
				} else {
					gamma = (1.0 - fk) / (1.0 - f0)
				}
			} else {
				if alphaTilde >= 0 {
					gamma = alphaTilde / f0
				} else {
					gamma = (-alphaTilde) / (1.0 - f0)
				}
			}

			if gamma <= 0 || (!oppFinite && gamma <= gmiZeroTol) {
				continue
			}

			if k < n {
				if st == statusAtLower {
					structCoeffs[k] += gamma
					rhs += gamma * solver.unscaledBound(k, false)
				} else {
					structCoeffs[k] -= gamma
					rhs -= gamma * solver.unscaledBound(k, true)
				}
			} else {
				rowIdx := k - n
				var rowRHS float64
				if st == statusAtLower {
					rowRHS = solver.unscaledRowRHS(rowIdx) - solver.unscaledBound(k, false)
					rhs -= gamma * rowRHS
					addScaledRow(p, solver.extraRows, rowIdx, -gamma, structCoeffs)
				} else {
					rowRHS = solver.unscaledBound(k, true) - solver.unscaledRowRHS(rowIdx)
					rhs -= gamma * rowRHS
					addScaledRow(p, solver.extraRows, rowIdx, +gamma, structCoeffs)
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
		cutRHS := -rhs

		if cut, ok := finalizeCut(p, structCoeffs, cutRHS, lpPrimal); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func addScaledRow(p *CompiledProblem, extra []dynamicRow, rowIdx int, scale float64, dst []float64) {
	if rowIdx < p.numConstraints {
		for k := p.rowPtr[rowIdx]; k < p.rowPtr[rowIdx+1]; k++ {
			dst[p.colIdx[k]] += scale * p.rowVal[k]
		}
		return
	}
	dr := extra[rowIdx-p.numConstraints]
	vals := dr.Val
	if dr.OrigVal != nil {
		vals = dr.OrigVal
	}
	for k, col := range dr.ColIdx {
		dst[col] += scale * vals[k]
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
		rhs := sign * p.rowRHS[i]

		validRelax := true
		items = items[:0]
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			a := sign * p.rowVal[k]
			if math.Abs(a) <= cutCoeffZeroTol {
				continue
			}
			if p.varCat[col] != Binary || p.varLB[col] != 0 || p.varUB[col] != 1 {
				if a > 0 {
					if math.IsInf(p.varLB[col], -1) {
						validRelax = false
						break
					}
					rhs -= a * p.varLB[col]
				} else {
					if math.IsInf(p.varUB[col], 1) {
						validRelax = false
						break
					}
					rhs -= a * p.varUB[col]
				}
				continue
			}
			if a > 0 {
				items = append(items, item{
					col:    col,
					weight: a,
					xVal:   lpPrimal[col],
					comp:   false,
				})
			} else {
				rhs -= a
				items = append(items, item{
					col:    col,
					weight: -a,
					xVal:   1.0 - lpPrimal[col],
					comp:   true,
				})
			}
		}

		if !validRelax || len(items) < 3 || rhs <= 0 {
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
		for idx, it := range items {
			if it.xVal <= minCoverItemVal {
				break
			}
			inCover[idx] = true
			coverWeight += it.weight
			coverSize++
			if coverWeight > rhs+cutCompareTol {
				break
			}
		}
		if coverWeight <= rhs+cutCompareTol || coverSize < minPairNNZ {
			continue
		}

		for idx := len(items) - 1; idx >= 0; idx-- {
			if !inCover[idx] {
				continue
			}
			if coverWeight-items[idx].weight > rhs+cutCompareTol {
				inCover[idx] = false
				coverWeight -= items[idx].weight
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
			minWeight[v] = minWeight[v-1] + coverWeights[v-1]
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
				zMax := 0
				for v := coverSize - 1; v >= 1; v-- {
					if minWeight[v] <= remCap+coverDPTol {
						zMax = v
						break
					}
				}
				if remCap < -coverDPTol {
					alpha = coverSize - 1
				} else {
					alpha = (coverSize - 1) - zMax
				}
				if alpha > 0 {
					for v := coverSize - 1; v >= 1; v-- {
						prev := max(0, v-alpha)
						if minWeight[prev]+it.weight < minWeight[v] {
							minWeight[v] = minWeight[prev] + it.weight
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
	vubCap := make([]float64, n)
	for j := range n {
		vubBinCol[j] = -1
	}

	denseCut := make([]float64, n)

	// 1. Detect VUB constraints (p - M*y <= 0) and Disaggregated Implied VUB constraints (sum_k a_k x_k - C*y <= 0)
	for i := range m {
		if p.rowSense[i] != LessEqual || math.Abs(p.rowRHS[i]) > unitCoeffTol {
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
			if val < -cutCoeffZeroTol {
				negCount++
				if p.varCat[col] == Binary && p.varLB[col] == 0 && p.varUB[col] == 1 {
					binCol = col
					capVal = -val
				}
			} else if val > cutCoeffZeroTol {
				if p.varLB[col] < -cutCoeffZeroTol {
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
				if col != binCol && math.Abs(val-1.0) <= unitCoeffTol {
					vubBinCol[col] = binCol
					vubCap[col] = capVal
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
			if math.Abs(val-1.0) <= unitCoeffTol {
				if vubBinCol[col] != -1 && p.varLB[col] >= 0 && prodCol == -1 {
					prodCol = col
				} else if p.varLB[col] >= 0 && invIn == -1 {
					invIn = col
				} else {
					valid = false
					break
				}
			} else if math.Abs(val+1.0) <= unitCoeffTol && p.varLB[col] >= 0 && invOut == -1 {
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

			// Shift variables to closer finite bound
			beta := rhs
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				a := sign * p.rowVal[k]
				l, u := p.varLB[col], p.varUB[col]
				if mirUseLower(lpPrimal[col], l, u) {
					beta -= a * l
				} else {
					beta -= a * u
				}
			}

			betaHat := beta / delta
			b0 := math.Floor(betaHat)
			f0 := betaHat - b0
			if f0 < minMIRFrac || f0 > 1.0-minMIRFrac {
				continue
			}

			clear(denseCut)
			cutRHS := b0
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				a := sign * p.rowVal[k]
				l, u := p.varLB[col], p.varUB[col]
				atLower := mirUseLower(lpPrimal[col], l, u)

				if p.varCat[col] == Continuous {
					if a > 0 {
						if atLower {
							// Relax a*(x - l) >= 0 from LHS: coefficient 0
							continue
						}
						// x = u - s (s >= 0) => -a*s on LHS => -g*(u - x) in MIR
						g := a / (delta * (1.0 - f0))
						denseCut[col] = g
						cutRHS += g * u
					} else if a < 0 {
						if !atLower {
							// Relax a*(x - u) = |a|*(u - x) >= 0 from LHS: coefficient 0
							continue
						}
						// x = l + s (s >= 0) => -|a|*s on LHS => -g*(x - l) in MIR
						g := (-a) / (delta * (1.0 - f0))
						denseCut[col] = -g
						cutRHS -= g * l
					}
				} else {
					comp := !atLower
					alpha := a / delta
					if comp {
						alpha = -alpha
					}
					flA := math.Floor(alpha)
					fj := alpha - flA
					coeff := flA
					if fj > f0 {
						coeff += (fj - f0) / (1.0 - f0)
					}
					if !comp {
						denseCut[col] = coeff
						cutRHS += coeff * l
					} else {
						denseCut[col] = -coeff
						cutRHS -= coeff * u
					}
				}
			}
			if cut, ok := finalizeCut(p, denseCut, cutRHS, lpPrimal); ok {
				cuts = append(cuts, cut)
			}
		}
	}

	return cuts
}

// finalizeCut converts the dense cut dense*x <= rhs into a normalized sparse cut row.
// Coefficients with magnitude in (0, cutDropTol] are removed; to keep the cut valid, the
// largest possible value of each dropped term -a_j*x_j over the global bounds of x_j is added
// to the right-hand side, and the cut is rejected if that bound is infinite.
func finalizeCut(p *CompiledProblem, dense []float64, rhs float64, lpPrimal []float64) (cutPlane, bool) {
	norm2 := 0.0
	maxAbs := 0.0
	minAbs := math.Inf(1)
	nnz := 0

	for j, v := range dense {
		av := math.Abs(v)
		if av > cutDropTol {
			norm2 += v * v
			maxAbs = max(maxAbs, av)
			minAbs = min(minAbs, av)
			nnz++
		} else if v != 0 {
			// Dropping v*x_j relaxes the cut only after adding max(-v*x_j) = -min(v*l_j, v*u_j).
			lo := min(v*p.varLB[j], v*p.varUB[j])
			if math.IsInf(lo, 0) || math.IsNaN(lo) {
				return cutPlane{}, false
			}
			rhs -= lo
		}
	}
	if nnz == 0 || maxAbs/minAbs > maxCutDynamism {
		return cutPlane{}, false
	}

	norm := math.Sqrt(norm2)
	invNorm := 1.0 / norm

	colIdx := make([]int, 0, nnz)
	val := make([]float64, 0, nnz)
	act := 0.0
	for j, v := range dense {
		if math.Abs(v) > cutDropTol {
			sv := v * invNorm
			colIdx = append(colIdx, j)
			val = append(val, sv)
			act += sv * lpPrimal[j]
		}
	}
	scaledRHS := rhs*invNorm + rhsSafetyMargin
	efficacy := act - scaledRHS
	if efficacy < minCutEfficacy {
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
	return math.Abs(dot)
}
