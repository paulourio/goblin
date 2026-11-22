package goblin_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
)

const (
	decomposedMADMinGroups = 4
	decomposedMADMaxGroups = 10
	decomposedMADMinItems  = 60
	decomposedMADMaxItems  = 2000
)

type decomposedMADSpec struct {
	seed     uint64
	nG       int
	nI       int
	nT       int
	regime   uint8
	sparsity uint8
}

type decomposedMADInstance struct {
	spec         decomposedMADSpec
	roundingBase float64
	model        *gb.Model
	weights      [][]float64
	groupSeries  [][]float64
	plantedGroup []int
	plantedObj   float64
	density      float64
}

func decodeDecomposedMADSpec(seed uint64, gByte uint8, sizeRaw uint16, regimeByte, sparsityByte uint8) decomposedMADSpec {
	rng := rand.New(rand.NewPCG(seed^0x9e3779b97f4a7c15, seed^0xbf58476d1ce4e5b9))
	nG := decomposedMADMinGroups + int(gByte%(decomposedMADMaxGroups-decomposedMADMinGroups+1))

	// Log-uniform sampling across [60, 2000] allocates equal fuzz effort across
	// small-medium (60..150), mid (150..500), and large (500..2000) item counts.
	u := float64(sizeRaw) / float64(math.MaxUint16)
	ratio := float64(decomposedMADMaxItems) / float64(decomposedMADMinItems)
	nI := int(math.Round(float64(decomposedMADMinItems) * math.Pow(ratio, u)))
	nI = max(decomposedMADMinItems, min(decomposedMADMaxItems, nI))

	periods := []int{4, 5, 6, 6, 6, 8}
	nT := periods[rng.IntN(len(periods))]

	return decomposedMADSpec{
		seed:     seed,
		nG:       nG,
		nI:       nI,
		nT:       nT,
		regime:   regimeByte % 4,
		sparsity: sparsityByte % 4,
	}
}

func sizeRawForItems(nI int) uint16 {
	clamped := max(decomposedMADMinItems, min(decomposedMADMaxItems, nI))
	u := math.Log(float64(clamped)/float64(decomposedMADMinItems)) /
		math.Log(float64(decomposedMADMaxItems)/float64(decomposedMADMinItems))
	return uint16(math.Round(u * float64(math.MaxUint16)))
}

// exactGroupMADSolution returns the exact minimum per-group MAD cost together with
// an attaining (m, n) pair for fixed period sums s[0..nT-1] and rounding base B.
func exactGroupMADSolution(s []float64, roundingBase float64) (bestCost, bestM, bestN float64) {
	candN := make([]float64, 0, 2*len(s)+1)
	candN = append(candN, 1.0)
	for _, v := range s {
		fl := max(1.0, math.Floor(v/roundingBase))
		cl := max(1.0, math.Ceil(v/roundingBase))
		candN = append(candN, fl, cl)
	}
	bestCost = math.Inf(1)
	bestM = roundingBase
	bestN = 1.0
	eval := func(mVal, nVal, bn float64) {
		if mVal < 0 {
			return
		}
		cost := math.Abs(mVal - bn)
		for _, v := range s {
			cost += math.Abs(v - mVal)
		}
		if cost < bestCost {
			bestCost = cost
			bestM = mVal
			bestN = nVal
		}
	}
	for _, nVal := range candN {
		bn := roundingBase * nVal
		eval(bn, nVal, bn)
		for _, v := range s {
			eval(v, nVal, bn)
		}
	}
	return bestCost, bestM, bestN
}

// buildDecomposedMediumMAD constructs a medium-size MAD group-item model by generating
// non-negative group time-series Y[g][t] across 4..10 groups and splitting each series
// into 60..2000 sparse or dense heterogeneous items before shuffling columns.
//
// Spread modes (spec.regime) control the aggregate group time-series:
//   - Mode 0 builds flat base-aligned group series with complementary anchor deficits.
//   - Mode 1 builds tight stationary AR(1) group series with anchor deficits.
//   - Mode 2 builds seasonal, trend, and noise series split directly across items.
//   - Mode 3 builds skewed group sizes with bursty spikes and post-split item noise.
//
// Fill modes (spec.sparsity) control how many periods each item covers:
//   - Mode 0 generates sparse event items covering 15..35% of period entries.
//   - Mode 1 generates medium seasonal-subset items covering 35..65% of entries.
//   - Mode 2 mixes dense base-load, medium seasonal, and 1..2-period burst items.
//   - Mode 3 generates dense heterogeneous items covering 80..100% of period entries.
func buildDecomposedMediumMAD(spec decomposedMADSpec, tag string) decomposedMADInstance {
	rng := rand.New(rand.NewPCG(spec.seed^0x243f6a8885a308d3, spec.seed^0x13198a2e03707344))
	nG, nI, nT := spec.nG, spec.nI, spec.nT

	bases := []float64{50.0, 100.0, 100.0, 200.0}
	roundingBase := bases[rng.IntN(len(bases))]

	// 1. Allocate item counts per group (at least 2 items per group: 1 anchor + >= 1 regular).
	groupCounts := allocateGroupItemCounts(rng, nG, nI, spec.regime == 3)

	// 2. Generate group time-series Y[g][t] >= 0.
	series := make([][]float64, nG)
	for g := range nG {
		series[g] = generateGroupTimeSeries(rng, groupCounts[g], nT, roundingBase, spec.regime)
	}

	// 3. Split each group's series into groupCounts[g] items with controlled fill.
	// Hold regular items and designated anchor items separately before shuffling.
	nReg := nI - nG
	regWeights := make([][]float64, nReg)
	regGroup := make([]int, nReg)
	anchorWeights := make([][]float64, nG)
	anchorGroup := make([]int, nG)

	regCursor := 0
	for g := range nG {
		m := groupCounts[g]
		if spec.regime <= 1 {
			// Split each group's target into a distinct non-flat anchor curve plus the
			// matching deficit series Y[g][t] - anchor[g][t] >= 0 for the remaining m-1 items.
			anchor, deficit := splitAnchorAndDeficitSeries(rng, series[g], g, nG, spec.regime == 0 && spec.sparsity <= 1)
			regItems := decomposeGroupSeriesIntoItems(rng, deficit, m-1, spec.sparsity, false)
			anchorWeights[g] = anchor
			anchorGroup[g] = g
			for k := range m - 1 {
				regWeights[regCursor] = regItems[k]
				regGroup[regCursor] = g
				regCursor++
			}
			continue
		}

		items := decomposeGroupSeriesIntoItems(rng, series[g], m, spec.sparsity, spec.regime == 3)
		// Pick either item 0 (the base anchor) or a random item as the group's
		// designated subset representative in D = [nI-nG, nI-1].
		anchorIdx := 0
		if rng.IntN(2) == 1 {
			anchorIdx = rng.IntN(m)
		}
		anchorWeights[g] = items[anchorIdx]
		anchorGroup[g] = g
		for k := range m {
			if k == anchorIdx {
				continue
			}
			regWeights[regCursor] = items[k]
			regGroup[regCursor] = g
			regCursor++
		}
	}

	// 4. Randomly permute regular items and designated items independently so column order
	// never leaks the planted grouping.
	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
	}
	plantedGroup := make([]int, nI)

	regPerm := rng.Perm(nReg)
	for dst, src := range regPerm {
		plantedGroup[dst] = regGroup[src]
		for t := range nT {
			w[t][dst] = regWeights[src][t]
		}
	}
	dStart := nI - nG
	desPerm := rng.Perm(nG)
	for dstOff, src := range desPerm {
		col := dStart + dstOff
		plantedGroup[col] = anchorGroup[src]
		for t := range nT {
			w[t][col] = anchorWeights[src][t]
		}
	}

	// 5. Compute exact planted grouping objective and nonzero fill ratio from final w.
	groupSums := make([][]float64, nG)
	for g := range nG {
		groupSums[g] = make([]float64, nT)
	}
	nnz := 0
	for t := range nT {
		for i := range nI {
			wt := w[t][i]
			if wt != 0 {
				nnz++
				groupSums[plantedGroup[i]][t] += wt
			}
		}
	}
	plantedObj := 0.0
	for g := range nG {
		cost, _, _ := exactGroupMADSolution(groupSums[g], roundingBase)
		plantedObj += cost
	}

	mdl := bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
	return decomposedMADInstance{
		spec:         spec,
		roundingBase: roundingBase,
		model:        mdl,
		weights:      w,
		groupSeries:  series,
		plantedGroup: plantedGroup,
		plantedObj:   plantedObj,
		density:      float64(nnz) / float64(nT*nI),
	}
}

func allocateGroupItemCounts(rng *rand.Rand, nG, nI int, skewed bool) []int {
	counts := make([]int, nG)
	for g := range nG {
		counts[g] = 2
	}
	rem := nI - 2*nG
	if rem <= 0 {
		return counts
	}

	weights := make([]float64, nG)
	sumW := 0.0
	for g := range nG {
		if skewed {
			// Span roughly a 5x ratio across groups.
			weights[g] = math.Exp(rng.Float64() * 1.6)
		} else {
			weights[g] = 0.75 + 0.5*rng.Float64()
		}
		sumW += weights[g]
	}

	assigned := 0
	maxG := 0
	for g := range nG {
		if weights[g] > weights[maxG] {
			maxG = g
		}
		extra := int(math.Floor(float64(rem) * weights[g] / sumW))
		counts[g] += extra
		assigned += extra
	}
	counts[maxG] += rem - assigned
	return counts
}

func generateGroupTimeSeries(rng *rand.Rand, groupItems, nT int, roundingBase float64, regime uint8) []float64 {
	// Keep average per-item weight in a realistic O(100..1000) range regardless of nI.
	mult := max(2, int(math.Round(float64(groupItems)*(2.5+4.5*rng.Float64()))))
	center := roundingBase * float64(mult)

	s := make([]float64, nT)
	switch regime {
	case 0:
		// Flat base-aligned group series: exact zero-MAD target.
		for t := range nT {
			s[t] = center
		}
	case 1:
		// Tight stationary AR(1) series around center plus a small base shift.
		offset := 0.0
		if rng.IntN(2) == 1 {
			offset = math.Round((rng.Float64()*0.4 - 0.2) * roundingBase)
		}
		amp := center * (0.005 + 0.02*rng.Float64())
		phi := 0.3 + 0.4*rng.Float64()
		state := (rng.Float64()*2.0 - 1.0) * amp
		for t := range nT {
			if t > 0 {
				state = phi*state + (1.0-phi)*(rng.Float64()*2.0-1.0)*amp
			}
			s[t] = max(roundingBase, math.Round(center+offset+state))
		}
	case 2:
		// Structured seasonal wave + linear trend + mild noise.
		amp := center * (0.02 + 0.06*rng.Float64())
		phase := rng.Float64() * 2.0 * math.Pi
		trend := (rng.Float64()*2.0 - 1.0) * center * 0.015
		mid := 0.5 * float64(nT-1)
		for t := range nT {
			wave := amp * math.Sin(2.0*math.Pi*float64(t)/float64(nT)+phase)
			noise := (rng.Float64()*2.0 - 1.0) * amp * 0.25
			val := center + wave + trend*(float64(t)-mid) + noise
			s[t] = max(roundingBase, math.Round(val))
		}
	default:
		// Bursty series with occasional single-period spikes.
		amp := center * (0.01 + 0.04*rng.Float64())
		spikeT := rng.IntN(nT)
		spikeMag := center * (0.02 + 0.05*rng.Float64())
		for t := range nT {
			val := center + (rng.Float64()*2.0-1.0)*amp
			if t == spikeT {
				val += spikeMag
			}
			s[t] = max(roundingBase, math.Round(val))
		}
	}
	return s
}

// splitAnchorAndDeficitSeries partitions a group's target series into a distinct non-flat
// base anchor portfolio curve and its exact complementary deficit series (anchor[t]+deficit[t] == series[t]).
// When sparseDeficit is true, each group leaves a distinct subset of periods strongly under-covered
// (creating sharper per-group temporal signatures while preserving exact series sums).
func splitAnchorAndDeficitSeries(rng *rand.Rand, series []float64, g, nG int, sparseDeficit bool) (anchor, deficit []float64) {
	nT := len(series)
	anchor = make([]float64, nT)
	deficit = make([]float64, nT)

	phase1 := 2.0*math.Pi*float64(g)/float64(max(1, nG)) + (rng.Float64()-0.5)*0.35
	phase2 := 2.0 * math.Pi * float64((2*g+1)%max(1, nG)) / float64(max(1, nG))
	harm2 := float64(1 + (g % 2))
	baseFrac := 0.42 + 0.16*rng.Float64()

	// Guarantee at least two periods carry substantial deficit so sparse item splits remain nontrivial.
	primaryT := g % nT
	secondaryT := (g + 1 + (g / nT)) % nT
	if secondaryT == primaryT {
		secondaryT = (primaryT + 1) % nT
	}

	for t := range nT {
		theta := 2.0 * math.Pi * float64(t) / float64(nT)
		wave := 0.26*math.Sin(theta+phase1) + 0.12*math.Cos(harm2*theta+phase2)
		frac := max(0.10, min(0.88, baseFrac+wave))
		if sparseDeficit {
			if t == primaryT || t == secondaryT {
				frac = 0.08 + 0.14*rng.Float64()
			} else {
				frac = max(frac, 0.72+0.20*rng.Float64())
			}
		}
		a := math.Round(series[t] * frac)
		a = max(0.0, min(series[t], a))
		anchor[t] = a
		deficit[t] = series[t] - a
	}
	return anchor, deficit
}

func decomposeGroupSeriesIntoItems(rng *rand.Rand, series []float64, nItems int, sparsity uint8, addNoise bool) [][]float64 {
	nT := len(series)
	items := make([][]float64, nItems)
	active := make([][]bool, nItems)
	itemScale := make([]float64, nItems)

	for k := range nItems {
		items[k] = make([]float64, nT)
		active[k] = make([]bool, nT)

		// Heterogeneous item scales (small, typical, and heavy-tail items).
		switch rng.IntN(10) {
		case 0:
			itemScale[k] = 3.0 + 5.0*rng.Float64()
		case 1, 2:
			itemScale[k] = 0.15 + 0.45*rng.Float64()
		default:
			itemScale[k] = 0.6 + 1.8*rng.Float64()
		}

		// Item 0 serves as an optional base-load anchor covering all periods in
		// sparse/mixed modes so every period with series[t] > 0 has active support.
		if k == 0 && sparsity != 0 {
			for t := range nT {
				active[k][t] = true
			}
			continue
		}
		assignItemSparsityMask(rng, active[k], sparsity)
	}

	// Ensure every positive period has at least one active item.
	for t := range nT {
		if series[t] <= 0 {
			continue
		}
		hasActive := false
		for k := range nItems {
			if active[k][t] {
				hasActive = true
				break
			}
		}
		if !hasActive {
			active[0][t] = true
		}
	}

	// Split each period target into exact integer shares so sum_k items[k][t] == series[t].
	activeIdx := make([]int, 0, nItems)
	rawShare := make([]float64, 0, nItems)
	for t := range nT {
		target := series[t]
		if target <= 0 {
			continue
		}
		activeIdx = activeIdx[:0]
		rawShare = rawShare[:0]
		sumShare := 0.0
		maxPos := 0
		maxShare := -1.0

		for k := range nItems {
			if !active[k][t] {
				continue
			}
			// Independent period multiplier gives items distinct temporal shapes.
			shape := 0.15 + 1.85*rng.Float64()
			if rng.IntN(3) == 0 {
				shape *= shape
			}
			u := itemScale[k] * shape
			if u > maxShare {
				maxShare = u
				maxPos = len(activeIdx)
			}
			activeIdx = append(activeIdx, k)
			rawShare = append(rawShare, u)
			sumShare += u
		}

		assigned := 0.0
		for pos, k := range activeIdx {
			share := math.Floor(target * rawShare[pos] / sumShare)
			items[k][t] = share
			assigned += share
		}
		rem := target - assigned
		if rem > 0 {
			items[activeIdx[maxPos]][t] += rem
		}
	}

	if addNoise {
		for k := range nItems {
			for t := range nT {
				if items[k][t] > 0 && rng.IntN(7) == 0 {
					jitter := math.Round(items[k][t] * (rng.Float64()*0.06 - 0.03))
					items[k][t] = max(0.0, items[k][t]+jitter)
				}
			}
		}
	}

	return items
}

func assignItemSparsityMask(rng *rand.Rand, mask []bool, sparsity uint8) {
	nT := len(mask)
	switch sparsity {
	case 0:
		// Very sparse: 1 or 2 active periods (contiguous burst or random spikes).
		kActive := 1
		if nT >= 4 && rng.IntN(2) == 1 {
			kActive = 2
		}
		if rng.IntN(2) == 0 {
			start := rng.IntN(nT)
			for off := range kActive {
				mask[(start+off)%nT] = true
			}
		} else {
			perm := rng.Perm(nT)
			for idx := range kActive {
				mask[perm[idx]] = true
			}
		}
	case 1:
		// Medium sparse: contiguous seasonal block or subset covering 35..65% of periods.
		minK := max(1, (nT+1)/3)
		maxK := max(minK, (2*nT)/3)
		kActive := minK + rng.IntN(maxK-minK+1)
		if rng.IntN(2) == 0 {
			start := rng.IntN(nT)
			for off := range kActive {
				mask[(start+off)%nT] = true
			}
		} else {
			perm := rng.Perm(nT)
			for idx := range kActive {
				mask[perm[idx]] = true
			}
		}
	case 2:
		// Mixed portfolio: 20% dense base-load, 45% medium seasonal, 35% 1..2-period bursts.
		roll := rng.IntN(20)
		switch {
		case roll < 4:
			for t := range nT {
				mask[t] = true
			}
		case roll < 13:
			assignItemSparsityMask(rng, mask, 1)
		default:
			assignItemSparsityMask(rng, mask, 0)
		}
	default:
		// Dense heterogeneous: 80..100% active periods.
		anySet := false
		for t := range nT {
			if rng.IntN(10) < 9 {
				mask[t] = true
				anySet = true
			}
		}
		if !anySet {
			mask[rng.IntN(nT)] = true
		}
	}
}

// buildPlantedPrimalVector constructs the explicit feasible primal vector corresponding
// to inst.plantedGroup to check bounds and rows directly against the compiled model.
func (inst decomposedMADInstance) buildPlantedPrimalVector(p *gb.CompiledProblem) []float64 {
	nG, nI, nT := inst.spec.nG, inst.spec.nI, inst.spec.nT
	vals := make([]float64, p.NumVars())
	stride := nI + nT + 3

	groupSums := make([][]float64, nG)
	for g := range nG {
		groupSums[g] = make([]float64, nT)
	}
	for i := range nI {
		g := inst.plantedGroup[i]
		vals[g*stride+i] = 1.0
		for t := range nT {
			groupSums[g][t] += inst.weights[t][i]
		}
	}
	for g := range nG {
		_, mVal, nVal := exactGroupMADSolution(groupSums[g], inst.roundingBase)
		base := g * stride
		for t := range nT {
			vals[base+nI+t] = math.Abs(groupSums[g][t] - mVal)
		}
		vals[base+nI+nT] = mVal
		vals[base+nI+nT+1] = nVal
		vals[base+nI+nT+2] = math.Abs(mVal - inst.roundingBase*nVal)
	}
	return vals
}

func verifyOptionalIncumbent(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result) bool {
	t.Helper()
	if len(res.Values) != p.NumVars() {
		return false
	}
	bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
	rowTol := 1e-5
	if res.Backend == gb.BackendCBC {
		// Match cbcPrimalWithinTol (1e-4 abs) so scaled CBC row residuals (~1e-9 rel on
		// large-activity adversarial MAD rows) do not false-fail differential fuzzers.
		rowTol = 1e-4
	}
	if bV > 1e-5 || rV > rowTol || iV > 1e-5 {
		t.Fatalf("[%s] incumbent (status=%v) violates model: bound=%.2e row=%.2e int=%.2e",
			label, res.Status, bV, rV, iV)
	}
	evalObj := p.EvaluateObjective(res.Values)
	if math.Abs(evalObj-res.Objective) > 1e-5*max(1.0, math.Abs(res.Objective)) {
		t.Fatalf("[%s] incumbent EvaluateObjective=%.8f != res.Objective=%.8f", label, evalObj, res.Objective)
	}
	return true
}

func hasFiniteBound(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func assertDecomposedMADSoundness(t *testing.T, tag, label string, p *gb.CompiledProblem, res gb.Result, plantedObj float64) bool {
	t.Helper()

	switch res.Status {
	case gb.StatusInfeasible, gb.StatusUnbounded:
		t.Fatalf("[%s/%s] impossible status %v on guaranteed-feasible decomposed MAD (plantedObj=%.6f, nodes=%d, iters=%d)",
			tag, label, res.Status, plantedObj, res.Nodes, res.Iterations)
	case gb.StatusUndefined:
		t.Fatalf("[%s/%s] solver failure status %v (vars=%d rows=%d plantedObj=%.6f obj=%.6f bound=%.6f nodes=%d iters=%d)",
			tag, label, res.Status, p.NumVars(), p.NumConstraints(), plantedObj,
			res.Objective, res.BestBound, res.Nodes, res.Iterations)
	case gb.StatusNumericalError:
		if label != "cbc" {
			t.Fatalf("[%s/%s] solver failure status %v (vars=%d rows=%d plantedObj=%.6f obj=%.6f bound=%.6f nodes=%d iters=%d)",
				tag, label, res.Status, p.NumVars(), p.NumConstraints(), plantedObj,
				res.Objective, res.BestBound, res.Nodes, res.Iterations)
		}
	}

	verifyMILPInvariants(t, tag+"/"+label, p, res)
	hasInc := verifyOptionalIncumbent(t, tag+"/"+label, p, res)

	tol := 1e-5 * max(1.0, plantedObj)
	if hasFiniteBound(res.BestBound) && res.BestBound > plantedObj+tol {
		t.Fatalf("[%s/%s] BestBound %.8f (status=%v rootLP=%.8f obj=%.8f hasInc=%v nodes=%d iters=%d) exceeds feasible plantedObj %.8f",
			tag, label, res.BestBound, res.Status, res.RootLPBound, res.Objective, hasInc, res.Nodes, res.Iterations, plantedObj)
	}
	if hasFiniteBound(res.RootLPBound) && res.RootLPBound > plantedObj+tol {
		t.Fatalf("[%s/%s] RootLPBound %.8f (status=%v nodes=%d iters=%d) exceeds feasible plantedObj %.8f",
			tag, label, res.RootLPBound, res.Status, res.Nodes, res.Iterations, plantedObj)
	}
	if res.Status == gb.StatusOptimal && res.Objective > plantedObj+tol {
		t.Fatalf("[%s/%s] Optimal objective %.8f (rootLP=%.8f bound=%.8f nodes=%d iters=%d) exceeds feasible plantedObj %.8f",
			tag, label, res.Objective, res.RootLPBound, res.BestBound, res.Nodes, res.Iterations, plantedObj)
	}
	return hasInc
}

func assertTwoGoSolvesConsistent(t *testing.T, tag, nameA string, resA gb.Result, hasIncA bool, nameB string, resB gb.Result, hasIncB bool) {
	t.Helper()

	if resA.Status == gb.StatusOptimal && resB.Status == gb.StatusOptimal {
		tol := 1e-5 * max(1.0, max(math.Abs(resA.Objective), math.Abs(resB.Objective)))
		if math.Abs(resA.Objective-resB.Objective) > tol {
			t.Fatalf("[%s] optimal objective disagreement: %s=%.8f (nodes=%d) vs %s=%.8f (nodes=%d)",
				tag, nameA, resA.Objective, resA.Nodes, nameB, resB.Objective, resB.Nodes)
		}
		return
	}
	if hasIncA && hasFiniteBound(resB.BestBound) {
		tol := 1e-5 * max(1.0, math.Abs(resA.Objective))
		if resB.BestBound > resA.Objective+tol {
			t.Fatalf("[%s] %s BestBound %.8f exceeds %s feasible incumbent %.8f",
				tag, nameB, resB.BestBound, nameA, resA.Objective)
		}
	}
	if hasIncB && hasFiniteBound(resA.BestBound) {
		tol := 1e-5 * max(1.0, math.Abs(resB.Objective))
		if resA.BestBound > resB.Objective+tol {
			t.Fatalf("[%s] %s BestBound %.8f exceeds %s feasible incumbent %.8f",
				tag, nameA, resA.BestBound, nameB, resB.Objective)
		}
	}
}

func decomposedMADSeeds() []struct {
	seed     uint64
	gByte    uint8
	sizeRaw  uint16
	regime   uint8
	sparsity uint8
} {
	// Curated seed grid spanning G in [4, 10], I in [60, 2000], all four spread modes,
	// and all four fill patterns.
	return []struct {
		seed     uint64
		gByte    uint8
		sizeRaw  uint16
		regime   uint8
		sparsity uint8
	}{
		{seed: 1, gByte: 0, sizeRaw: sizeRawForItems(60), regime: 0, sparsity: 0},
		{seed: 2, gByte: 1, sizeRaw: sizeRawForItems(90), regime: 1, sparsity: 1},
		{seed: 3, gByte: 2, sizeRaw: sizeRawForItems(150), regime: 2, sparsity: 2},
		{seed: 4, gByte: 3, sizeRaw: sizeRawForItems(240), regime: 3, sparsity: 3},
		{seed: 5, gByte: 4, sizeRaw: sizeRawForItems(500), regime: 0, sparsity: 2},
		{seed: 6, gByte: 5, sizeRaw: sizeRawForItems(900), regime: 1, sparsity: 0},
		{seed: 7, gByte: 6, sizeRaw: sizeRawForItems(1500), regime: 2, sparsity: 1},
		{seed: 8, gByte: 0, sizeRaw: sizeRawForItems(2000), regime: 0, sparsity: 3},
	}
}

func TestDecomposedMADGeneratorInvariants(t *testing.T) {
	t.Parallel()
	for _, tc := range decomposedMADSeeds() {
		spec := decodeDecomposedMADSpec(tc.seed, tc.gByte, tc.sizeRaw, tc.regime, tc.sparsity)
		tag := fmt.Sprintf("gen_g%d_i%d_r%d_s%d", spec.nG, spec.nI, spec.regime, spec.sparsity)
		inst := buildDecomposedMediumMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}
		wantVars := spec.nG * (spec.nI + spec.nT + 3)
		wantRows := spec.nG*(2*spec.nT+3) + spec.nI
		if p.NumVars() != wantVars || p.NumConstraints() != wantRows {
			t.Fatalf("[%s] dimensions = (%d, %d), want (%d, %d)",
				tag, p.NumVars(), p.NumConstraints(), wantVars, wantRows)
		}

		// Check that the generative grouping is an exact primal-feasible vector matching plantedObj.
		witness := inst.buildPlantedPrimalVector(p)
		bV, rV, iV := p.MaxViolation(witness, true)
		if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
			t.Fatalf("[%s] planted witness violates model: bound=%.2e row=%.2e int=%.2e", tag, bV, rV, iV)
		}
		evalObj := p.EvaluateObjective(witness)
		if math.Abs(evalObj-inst.plantedObj) > 1e-6*max(1.0, inst.plantedObj) {
			t.Fatalf("[%s] witness EvaluateObjective=%.8f != plantedObj=%.8f", tag, evalObj, inst.plantedObj)
		}

		// Check exact zero-MAD target for flat aligned series (regime 0).
		if spec.regime == 0 && inst.plantedObj > 1e-9 {
			t.Fatalf("[%s] regime 0 expected plantedObj=0, got %.8f", tag, inst.plantedObj)
		}

		// Check that fill modes produce distinct matrix nonzero fractions.
		switch spec.sparsity {
		case 0:
			if inst.density > 0.45 {
				t.Fatalf("[%s] very-sparse regime produced high density %.3f", tag, inst.density)
			}
		case 3:
			if inst.density < 0.75 {
				t.Fatalf("[%s] dense regime produced low density %.3f", tag, inst.density)
			}
		}

		// Check exact group series sums for non-noisy regimes (0..2).
		if spec.regime != 3 {
			sums := make([][]float64, spec.nG)
			for g := range spec.nG {
				sums[g] = make([]float64, spec.nT)
			}
			for i := range spec.nI {
				g := inst.plantedGroup[i]
				for tt := range spec.nT {
					sums[g][tt] += inst.weights[tt][i]
				}
			}
			for g := range spec.nG {
				for tt := range spec.nT {
					if math.Abs(sums[g][tt]-inst.groupSeries[g][tt]) > 1e-6 {
						t.Fatalf("[%s] group %d period %d sum %.4f != series %.4f",
							tag, g, tt, sums[g][tt], inst.groupSeries[g][tt])
					}
				}
			}
		}
	}
}

// TestDecomposedMADFuzzRegression9744 checks that the presolved Root LP of fuzz case
// 9744b7a44a117049 solves cleanly to its exact relaxation optimum instead of pivoting a
// Forrest-Tomlin roundoff ghost into a singular basis.
func TestDecomposedMADFuzzRegression9744(t *testing.T) {
	t.Parallel()
	spec := decodeDecomposedMADSpec(1, 0x01, 39676, byte('['), byte('o'))
	inst := buildDecomposedMediumMAD(spec, "repro_9744")
	p, err := inst.model.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}
	pr := gb.Presolve(p, 6)
	if pr.Infeasible || pr.Reduced == nil {
		t.Fatalf("unexpected presolve failure: infeasible=%v", pr.Infeasible)
	}
	ws := gb.AcquireWorkspace(pr.Reduced.NumVars(), pr.Reduced.NumConstraints())
	defer gb.ReleaseWorkspace(ws)

	for _, crash := range []bool{false, true} {
		lp := gb.NewLPSolver(pr.Reduced, ws)
		if crash {
			lp.InitCrashBasis()
		}
		res := lp.Solve()
		if res.Status != gb.StatusOptimal {
			t.Fatalf("presolved LP (crash=%v) status=%v iters=%d, want Optimal", crash, res.Status, res.Iterations)
		}
		if math.Abs(res.Objective-21174.0) > 1e-4 {
			t.Fatalf("presolved LP (crash=%v) obj=%.6f, want 21174.0", crash, res.Objective)
		}
		bV, rV, _ := pr.Reduced.MaxViolation(res.Primal, false)
		if bV > 1e-5 || rV > 1e-5 {
			t.Fatalf("presolved LP (crash=%v) primal violation: bound=%.2e row=%.2e", crash, bV, rV)
		}
	}
}

// FuzzMADMediumDecomposed fuzzes the pure-Go MILP solver on medium-size decomposed
// time-series MAD models spanning 4..10 groups, 60..2000 items, four group-series spread
// modes, and four fill patterns. Each case carries an exact feasible upper bound
// (plantedObj) from its generative grouping, enabling sound dual-bound and optimal-value
// checks at scale without requiring external solvers.
//
//	go test -run=^$ -fuzz=^FuzzMADMediumDecomposed$ -fuzztime=2m .
func FuzzMADMediumDecomposed(f *testing.F) {
	for _, s := range decomposedMADSeeds() {
		f.Add(s.seed, s.gByte, s.sizeRaw, s.regime, s.sparsity)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, regimeByte, sparsityByte uint8) {
		spec := decodeDecomposedMADSpec(seed, gByte, sizeRaw, regimeByte, sparsityByte)
		if raceEnabled && testing.Short() && spec.nI > 250 {
			t.Skipf("skipping large %d-item decomposed MAD seed under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_med_g%d_i%d_t%d_r%d_s%d_%d",
			spec.nG, spec.nI, spec.nT, spec.regime, spec.sparsity, seed)
		inst := buildDecomposedMediumMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 350 * time.Millisecond
		if spec.nI > 400 {
			timeLimit = 700 * time.Millisecond
		}
		const maxNodes = 1500

		t0 := time.Now()
		resGo := mustSolveCompiled(t, p,
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		goElapsed := time.Since(t0)
		hasIncGo := assertDecomposedMADSoundness(t, tag, "go", p, resGo, inst.plantedObj)

		// Cross-check one randomized solver variant (scalar, no-presolve, no-cuts, no-heur, or 2 workers).
		var altName string
		var altOpt gb.Option
		switch (seed ^ uint64(regimeByte) ^ uint64(sparsityByte)) % 5 {
		case 0:
			altName = "go-scalar"
			altOpt = gb.WithoutSIMD()
		case 1:
			altName = "go-nopresolve"
			altOpt = gb.WithPresolve(false)
		case 2:
			altName = "go-nocuts"
			altOpt = gb.WithCuts(false)
		case 3:
			altName = "go-noheur"
			altOpt = gb.WithHeuristics(false)
		default:
			altName = "go-w2"
			altOpt = gb.WithWorkers(2)
		}
		resAlt := mustSolveCompiled(t, p,
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
			altOpt,
		)
		hasIncAlt := assertDecomposedMADSoundness(t, tag, altName, p, resAlt, inst.plantedObj)
		assertTwoGoSolvesConsistent(t, tag, "go", resGo, hasIncGo, altName, resAlt, hasIncAlt)

		if formalFuzzEnabled() && p.NumVars() <= 600 {
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, tag+"/rootLP", p)
			sols := [][]float64{inst.buildPlantedPrimalVector(p)}
			if hasIncGo {
				sols = append(sols, resGo.Values)
			}
			verifyFormalSeparatedCuts(t, tag+"/cuts", p, sols)
			if p.NumVars() <= 300 {
				resTr, _ := verifyFormalVIPRTrace(t, tag+"/vipr", p, 40, 250*time.Millisecond)
				_ = assertDecomposedMADSoundness(t, tag, "vipr", p, resTr, inst.plantedObj)
			}
		}

		t.Logf("[%s] density=%.2f planted=%.1f | go=%v obj=%.1f bnd=%.1f nd=%d it=%d (%v) | %s=%v obj=%.1f bnd=%.1f",
			tag, inst.density, inst.plantedObj,
			resGo.Status, resGo.Objective, resGo.BestBound, resGo.Nodes, resGo.Iterations, goElapsed.Round(time.Millisecond),
			altName, resAlt.Status, resAlt.Objective, resAlt.BestBound)
	})
}
