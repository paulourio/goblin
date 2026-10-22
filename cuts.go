package gopt

import (
	"cmp"
	"math"
	"slices"

	"gopt/internal/simd"
)

const (
	minIntInfeas    = 0.01
	minCutEfficacy  = 1e-3
	maxCutDynamism  = 1e5
	rhsSafetyMargin = 1e-7
	maxOrthoCosine  = 0.92
)

type cutPlane struct {
	ColIdx   []int
	Val      []float64
	RHS      float64
	Efficacy float64
}

func generateCutRound(solver *LPSolver, lpPrimal []float64, maxCuts int) []cutPlane {
	if maxCuts <= 0 {
		maxCuts = 15
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

	coverCuts := separateKnapsackCoverCuts(solver.Prob, lpPrimal)
	candidates = append(candidates, coverCuts...)
	if solver.timeExpired() {
		return nil
	}

	flowCuts := separateFlowAndVUBCuts(solver.Prob, lpPrimal)
	candidates = append(candidates, flowCuts...)
	if solver.timeExpired() {
		return nil
	}

	mirCuts := separateRowMIRCuts(solver.Prob, lpPrimal)
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

func separateGMICuts(solver *LPSolver, lpPrimal []float64) []cutPlane {
	p := solver.Prob
	ws := solver.WS
	n := p.NumVars
	m := solver.TotalRows()

	type gmiRow struct {
		r    int
		bVar int
		f0   float64
		dist float64
	}
	var fracRows []gmiRow
	for r := range m {
		bVar := ws.Basis[r]
		if bVar >= n || p.VarCat[bVar] == Continuous {
			continue
		}
		xVal := ws.XB[r]
		f0 := xVal - math.Floor(xVal)
		if f0 < minIntInfeas || f0 > 1.0-minIntInfeas {
			continue
		}
		fracRows = append(fracRows, gmiRow{
			r:    r,
			bVar: bVar,
			f0:   f0,
			dist: math.Abs(f0 - 0.5),
		})
	}
	if len(fracRows) == 0 {
		return nil
	}
	if len(fracRows) > 25 {
		slices.SortFunc(fracRows, func(a, b gmiRow) int {
			return cmp.Compare(a.dist, b.dist)
		})
		fracRows = fracRows[:25]
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
			st := ws.Status[k]
			if st == statusBasic || st == statusFixed {
				continue
			}
			if st == statusSuperBasic {
				validRow = false
				break
			}
			alpha := ws.TabRow[k]
			if math.Abs(alpha) <= 1e-13 {
				continue
			}

			alphaTilde := alpha
			if st == statusAtUpper {
				alphaTilde = -alpha
			}

			var gamma float64
			isIntVar := k < n && p.VarCat[k] != Continuous
			if isIntVar {
				bnd := ws.LB[k]
				if st == statusAtUpper {
					bnd = ws.UB[k]
				}
				if math.Abs(bnd-math.Round(bnd)) > 1e-6 {
					isIntVar = false
				}
			}

			if isIntVar {
				fk := alphaTilde - math.Floor(alphaTilde)
				if fk <= 1e-13 || fk >= 1.0-1e-13 {
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

			if gamma <= 1e-13 {
				continue
			}

			if k < n {
				if st == statusAtLower {
					structCoeffs[k] += gamma
					rhs += gamma * ws.LB[k]
				} else {
					structCoeffs[k] -= gamma
					rhs -= gamma * ws.UB[k]
				}
			} else {
				rowIdx := k - n
				var rowRHS float64
				if st == statusAtLower {
					rowRHS = solver.rowRHS(rowIdx) - ws.LB[k]
					rhs -= gamma * rowRHS
					addScaledRow(p, solver.ExtraRows, rowIdx, -gamma, structCoeffs)
				} else {
					rowRHS = ws.UB[k] - solver.rowRHS(rowIdx)
					rhs -= gamma * rowRHS
					addScaledRow(p, solver.ExtraRows, rowIdx, +gamma, structCoeffs)
				}
			}
		}

		if !validRow {
			continue
		}

		simd.Scale64(structCoeffs, -1.0)
		cutRHS := -rhs

		if cut, ok := finalizeCut(structCoeffs, cutRHS, lpPrimal); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func addScaledRow(p *CompiledProblem, extra []dynamicRow, rowIdx int, scale float64, dst []float64) {
	if rowIdx < p.NumConstraints {
		for k := p.RowPtr[rowIdx]; k < p.RowPtr[rowIdx+1]; k++ {
			dst[p.ColIdx[k]] += scale * p.RowVal[k]
		}
		return
	}
	dr := extra[rowIdx-p.NumConstraints]
	for k, col := range dr.ColIdx {
		dst[col] += scale * dr.Val[k]
	}
}

func separateKnapsackCoverCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.NumVars
	m := p.NumConstraints
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
		if p.RowSense[i] == Equal {
			continue
		}
		sign := 1.0
		if p.RowSense[i] == GreaterEqual {
			sign = -1.0
		}
		rhs := sign * p.RowRHS[i]

		validRelax := true
		items = items[:0]
		for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
			col := p.ColIdx[k]
			a := sign * p.RowVal[k]
			if math.Abs(a) <= 1e-12 {
				continue
			}
			if p.VarCat[col] != Binary || p.VarLB[col] != 0 || p.VarUB[col] != 1 {
				if a > 0 {
					if math.IsInf(p.VarLB[col], -1) {
						validRelax = false
						break
					}
					rhs -= a * p.VarLB[col]
				} else {
					if math.IsInf(p.VarUB[col], 1) {
						validRelax = false
						break
					}
					rhs -= a * p.VarUB[col]
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
			if math.Abs(a.xVal-b.xVal) > 1e-6 {
				return cmp.Compare(b.xVal, a.xVal)
			}
			return cmp.Compare(b.weight, a.weight)
		})

		inCover := make([]bool, len(items))
		coverWeight := 0.0
		coverSize := 0
		for idx, it := range items {
			if it.xVal <= 1e-4 {
				break
			}
			inCover[idx] = true
			coverWeight += it.weight
			coverSize++
			if coverWeight > rhs+1e-6 {
				break
			}
		}
		if coverWeight <= rhs+1e-6 || coverSize < 2 {
			continue
		}

		for idx := len(items) - 1; idx >= 0; idx-- {
			if !inCover[idx] {
				continue
			}
			if coverWeight-items[idx].weight > rhs+1e-6 {
				inCover[idx] = false
				coverWeight -= items[idx].weight
				coverSize--
			}
		}
		if coverSize < 2 {
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
			alpha := 0
			if inCover[idx] {
				alpha = 1
			} else {
				remCap := rhs - it.weight
				zMax := 0
				for v := coverSize - 1; v >= 1; v-- {
					if minWeight[v] <= remCap+1e-7 {
						zMax = v
						break
					}
				}
				if remCap < -1e-7 {
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
		if lhsStar <= cutCap+1e-3 {
			continue
		}

		if cut, ok := finalizeCut(denseCut, finalRHS, lpPrimal); ok {
			cuts = append(cuts, cut)
		}
	}

	return cuts
}

func separateFlowAndVUBCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.NumVars
	m := p.NumConstraints
	var cuts []cutPlane

	vubBinCol := make([]int, n)
	vubCap := make([]float64, n)
	for j := range n {
		vubBinCol[j] = -1
	}

	denseCut := make([]float64, n)

	// 1. Detect VUB constraints (p - M*y <= 0) and Disaggregated Implied VUB constraints (sum_k a_k x_k - C*y <= 0)
	for i := range m {
		if p.RowSense[i] != LessEqual || math.Abs(p.RowRHS[i]) > 1e-9 {
			continue
		}
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		nnz := e - s
		if nnz < 2 {
			continue
		}

		binCol := -1
		capVal := 0.0
		negCount := 0
		allPosNonNeg := true

		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			val := p.RowVal[k]
			if val < -1e-12 {
				negCount++
				if p.VarCat[col] == Binary && p.VarLB[col] == 0 && p.VarUB[col] == 1 {
					binCol = col
					capVal = -val
				}
			} else if val > 1e-12 {
				if p.VarLB[col] < -1e-12 {
					allPosNonNeg = false
				}
			}
		}

		if negCount != 1 || binCol == -1 || !allPosNonNeg {
			continue
		}

		if nnz == 2 {
			for k := s; k < e; k++ {
				col := p.ColIdx[k]
				val := p.RowVal[k]
				if col != binCol && math.Abs(val-1.0) <= 1e-9 {
					vubBinCol[col] = binCol
					vubCap[col] = capVal
				}
			}
			continue
		}

		// Disaggregated VUB cuts: for each x_k in sum_k a_k x_k - C*y <= 0, x_k - min(u_k, C/a_k)*y <= 0
		yStar := lpPrimal[binCol]
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			if col == binCol {
				continue
			}
			ak := p.RowVal[k]
			if ak <= 1e-12 {
				continue
			}
			effUB := min(p.VarUB[col], capVal/ak)
			if math.IsInf(effUB, 1) || effUB <= 0 {
				continue
			}
			if lpPrimal[col]-effUB*yStar > 1e-3 {
				clear(denseCut)
				denseCut[col] = 1.0
				denseCut[binCol] = -effUB
				if cut, ok := finalizeCut(denseCut, 0, lpPrimal); ok {
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
	nodeByInvIn := make(map[int]flowNode)
	var startNodes []flowNode

	for i := range m {
		if p.RowSense[i] != Equal || p.RowRHS[i] <= 1e-9 {
			continue
		}
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		nnz := e - s
		if nnz != 2 && nnz != 3 {
			continue
		}

		prodCol := -1
		invIn := -1
		invOut := -1
		valid := true

		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			val := p.RowVal[k]
			if math.Abs(val-1.0) <= 1e-9 {
				if vubBinCol[col] != -1 && prodCol == -1 {
					prodCol = col
				} else if p.VarLB[col] >= -1e-12 && invIn == -1 {
					invIn = col
				} else {
					valid = false
					break
				}
			} else if math.Abs(val+1.0) <= 1e-9 && p.VarLB[col] >= -1e-12 && invOut == -1 {
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
			demand:  p.RowRHS[i],
		}
		if invIn == -1 {
			startNodes = append(startNodes, fn)
		} else {
			nodeByInvIn[invIn] = fn
		}
	}

	var chain []flowNode
	for _, root := range startNodes {
		chain = chain[:0]
		cur := root
		for {
			chain = append(chain, cur)
			next, ok := nodeByInvIn[cur.invOut]
			if !ok || len(chain) >= 64 {
				break
			}
			cur = next
		}

		T := len(chain)
		for t := range T {
			maxQ := min(T-1, t+6)
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
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					yTerm := remDem * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-1e-6 {
						lhsStar += yTerm
						anyInS = true
					} else {
						lhsStar += pTerm
					}
					remDem -= nd.demand
				}

				if !anyInS || lhsStar >= cumDem-1e-3 {
					continue
				}

				clear(denseCut)
				if chain[t].invIn != -1 {
					denseCut[chain[t].invIn] = -1.0
				}
				remDem = cumDem
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					yTerm := remDem * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-1e-6 {
						denseCut[nd.binCol] = -remDem
					} else {
						denseCut[nd.prodCol] = -1.0
					}
					remDem -= nd.demand
				}

				if cut, ok := finalizeCut(denseCut, -cumDem, lpPrimal); ok {
					cuts = append(cuts, cut)
				}
			}
		}
	}

	return cuts
}

func separateRowMIRCuts(p *CompiledProblem, lpPrimal []float64) []cutPlane {
	n := p.NumVars
	m := p.NumConstraints
	var cuts []cutPlane
	denseCut := make([]float64, n)

	for i := range m {
		if p.RowSense[i] == Equal {
			continue
		}
		sign := 1.0
		if p.RowSense[i] == GreaterEqual {
			sign = -1.0
		}
		rhs := sign * p.RowRHS[i]
		s, e := p.RowPtr[i], p.RowPtr[i+1]
		if e-s < 2 {
			continue
		}

		// Only separate on rows with small slack in the LP relaxation
		act := 0.0
		validBounds := true
		for k := s; k < e; k++ {
			col := p.ColIdx[k]
			act += sign * p.RowVal[k] * lpPrimal[col]
			if math.IsInf(p.VarLB[col], -1) || math.IsInf(p.VarUB[col], 1) {
				validBounds = false
				break
			}
		}
		if !validBounds || rhs-act > 1.0+0.05*math.Abs(rhs) {
			continue
		}

		// Try Marchand-Wolsey c-MIR scaling with divisor delta = |a_k| for fractional integer variables
		triedDivs := 0
		for kDiv := s; kDiv < e && triedDivs < 4; kDiv++ {
			divCol := p.ColIdx[kDiv]
			if p.VarCat[divCol] == Continuous {
				continue
			}
			xv := lpPrimal[divCol]
			fx := xv - math.Floor(xv)
			if fx < minIntInfeas || fx > 1.0-minIntInfeas {
				continue
			}
			delta := math.Abs(p.RowVal[kDiv])
			if delta <= 1e-6 {
				continue
			}
			triedDivs++

			// Shift variables to closer bound
			beta := rhs
			for k := s; k < e; k++ {
				col := p.ColIdx[k]
				a := sign * p.RowVal[k]
				l, u := p.VarLB[col], p.VarUB[col]
				if lpPrimal[col]-l <= u-lpPrimal[col] {
					beta -= a * l
				} else {
					beta -= a * u
				}
			}

			betaHat := beta / delta
			b0 := math.Floor(betaHat)
			f0 := betaHat - b0
			if f0 < 0.02 || f0 > 0.98 {
				continue
			}

			clear(denseCut)
			cutRHS := b0
			for k := s; k < e; k++ {
				col := p.ColIdx[k]
				a := sign * p.RowVal[k]
				l, u := p.VarLB[col], p.VarUB[col]
				atLower := lpPrimal[col]-l <= u-lpPrimal[col]

				if p.VarCat[col] == Continuous {
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
			if cut, ok := finalizeCut(denseCut, cutRHS, lpPrimal); ok {
				cuts = append(cuts, cut)
			}
		}
	}

	return cuts
}

func finalizeCut(dense []float64, rhs float64, lpPrimal []float64) (cutPlane, bool) {
	norm2 := 0.0
	maxAbs := 0.0
	minAbs := math.Inf(1)
	nnz := 0

	for _, v := range dense {
		av := math.Abs(v)
		if av > 1e-11 {
			norm2 += v * v
			maxAbs = max(maxAbs, av)
			minAbs = min(minAbs, av)
			nnz++
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
		if math.Abs(v) > 1e-11 {
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
