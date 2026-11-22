package goblin_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
)

const (
	exactOptMADMinGroups = 3
	exactOptMADMaxGroups = 10
	exactOptMADMinItems  = 30
	exactOptMADMaxItems  = 1200

	advMADMinGroups = 3
	advMADMaxGroups = 8
	advMADMinItems  = 30
	advMADMaxItems  = 500
)

type exactOptMADSpec struct {
	seed     uint64
	nG       int
	nI       int
	nT       int
	mode     uint8
	sparsity uint8
	baseIdx  int
}

type exactOptMADInstance struct {
	spec         exactOptMADSpec
	roundingBase float64
	quantum      float64
	model        *gb.Model
	weights      [][]float64
	groupSeries  [][]float64
	plantedGroup []int
	plantedMG    []float64
	plantedNG    []float64
	plantedLPMG  []float64
	plantedLPNG  []float64
	exactLP      float64
	exactMILP    float64
	density      float64
}

func logUniformItems(sizeRaw uint16, minItems, maxItems int) int {
	u := float64(sizeRaw) / float64(math.MaxUint16)
	ratio := float64(maxItems) / float64(minItems)
	nI := int(math.Round(float64(minItems) * math.Pow(ratio, u)))
	return max(minItems, min(maxItems, nI))
}

func decodeExactOptMADSpec(seed uint64, gByte uint8, sizeRaw uint16, modeSparsityByte, baseByte uint8) exactOptMADSpec {
	rng := rand.New(rand.NewPCG(seed^0x9e3779b97f4a7c15, seed^0x517cc1b727220a95))
	nG := exactOptMADMinGroups + int(gByte%(exactOptMADMaxGroups-exactOptMADMinGroups+1))
	nI := logUniformItems(sizeRaw, exactOptMADMinItems, exactOptMADMaxItems)
	mode := modeSparsityByte % 3
	sparsity := (modeSparsityByte >> 2) % 4

	var nT int
	if mode == 0 {
		evenPeriods := []int{4, 6, 8}
		nT = evenPeriods[rng.IntN(len(evenPeriods))]
	} else {
		oddPeriods := []int{3, 5, 7}
		nT = oddPeriods[rng.IntN(len(oddPeriods))]
	}
	return exactOptMADSpec{
		seed:     seed,
		nG:       nG,
		nI:       nI,
		nT:       nT,
		mode:     mode,
		sparsity: sparsity,
		baseIdx:  int(baseByte % 7),
	}
}

// decomposeQuantumSeries partitions each group target series Y[t] into nItems item
// columns so every entry is an integer multiple of quantum q and sum_k items[k][t] == Y[t].
func decomposeQuantumSeries(rng *rand.Rand, series []float64, nItems int, sparsity uint8, q float64) [][]float64 {
	nT := len(series)
	items := make([][]float64, nItems)
	active := make([][]bool, nItems)
	itemScale := make([]float64, nItems)

	for k := range nItems {
		items[k] = make([]float64, nT)
		active[k] = make([]bool, nT)
		switch rng.IntN(8) {
		case 0:
			itemScale[k] = 2.5 + 4.5*rng.Float64()
		case 1:
			itemScale[k] = 0.2 + 0.4*rng.Float64()
		default:
			itemScale[k] = 0.7 + 1.6*rng.Float64()
		}
		if k == 0 && sparsity != 0 {
			for t := range nT {
				active[k][t] = true
			}
			continue
		}
		assignItemSparsityMask(rng, active[k], sparsity)
	}

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

	activeIdx := make([]int, 0, nItems)
	rawShare := make([]float64, 0, nItems)
	for t := range nT {
		totalTicks := int64(math.Round(series[t] / q))
		if totalTicks <= 0 {
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
			u := itemScale[k] * (0.2 + 1.8*rng.Float64())
			if u > maxShare {
				maxShare = u
				maxPos = len(activeIdx)
			}
			activeIdx = append(activeIdx, k)
			rawShare = append(rawShare, u)
			sumShare += u
		}

		assigned := int64(0)
		for pos, k := range activeIdx {
			share := int64(math.Floor(float64(totalTicks) * rawShare[pos] / sumShare))
			items[k][t] = float64(share) * q
			assigned += share
		}
		if rem := totalTicks - assigned; rem > 0 {
			items[activeIdx[maxPos]][t] += float64(rem) * q
		}
	}
	return items
}

// permuteAndPackMADColumns independently permutes regular and designated columns and
// writes the final nT x nI weight matrix together with the planted group assignment.
func permuteAndPackMADColumns(rng *rand.Rand, nG, nI, nT int, regWeights [][]float64, regGroup []int, anchorWeights [][]float64, anchorGroup []int) (w [][]float64, plantedGroup []int, density float64) {
	nReg := nI - nG
	w = make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
	}
	plantedGroup = make([]int, nI)

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

	nnz := 0
	for t := range nT {
		for i := range nI {
			if w[t][i] != 0 {
				nnz++
			}
		}
	}
	density = float64(nnz) / float64(nT*nI)
	return w, plantedGroup, density
}

// splitExactSeriesAndPack constructs group items from per-group target series, selects a
// designated anchor per group, permutes columns, and computes plantedObj.
func splitExactSeriesAndPack(rng *rand.Rand, nG, nI, nT int, roundingBase, quantum float64, series [][]float64, groupCounts []int, sparsity uint8, useQuantum bool) (w [][]float64, plantedGroup []int, plantedObj, density float64) {
	nReg := nI - nG
	regWeights := make([][]float64, nReg)
	regGroup := make([]int, nReg)
	anchorWeights := make([][]float64, nG)
	anchorGroup := make([]int, nG)

	regCursor := 0
	for g := range nG {
		m := groupCounts[g]
		var items [][]float64
		if useQuantum {
			items = decomposeQuantumSeries(rng, series[g], m, sparsity, quantum)
		} else {
			items = decomposeGroupSeriesIntoItems(rng, series[g], m, sparsity, false)
		}
		anchorIdx := rng.IntN(m)
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

	w, plantedGroup, density = permuteAndPackMADColumns(rng, nG, nI, nT, regWeights, regGroup, anchorWeights, anchorGroup)

	groupSums := make([][]float64, nG)
	for g := range nG {
		groupSums[g] = make([]float64, nT)
	}
	for i := range nI {
		g := plantedGroup[i]
		for t := range nT {
			groupSums[g][t] += w[t][i]
		}
	}
	for g := range nG {
		cost, _, _ := exactGroupMADSolution(groupSums[g], roundingBase)
		plantedObj += cost
	}
	return w, plantedGroup, plantedObj, density
}

// buildExactOptimumMAD constructs a medium-to-large MAD instance whose LP relaxation
// optimum (exactLP) and integer MILP optimum (exactMILP) are known analytically:
//   - Mode 0 (even T): exact nonzero optimum with zero integrality gap.
//   - Mode 1 (odd T): exact nonzero optimum with guaranteed gap Delta in (0, B/2].
//   - Mode 2 (odd T): exact nonzero optimum with binding n_g >= 1 lower bound.
func buildExactOptimumMAD(spec exactOptMADSpec, tag string) exactOptMADInstance {
	rng := rand.New(rand.NewPCG(spec.seed^0x3c6ef372fe94f82b, spec.seed^0xa54ff53a5f1d36f1))
	nG, nI, nT := spec.nG, spec.nI, spec.nT

	bases := []float64{0.25, 1.0, 10.0, 50.0, 100.0, 1000.0, 10000.0}
	quanta := []float64{0.0625, 0.125, 1.0, 1.0, 1.0, 1.0, 1.0}
	idx := spec.baseIdx % len(bases)
	roundingBase := bases[idx]
	q := quanta[idx]
	baseTicks := int64(math.Round(roundingBase / q))

	groupCounts := allocateGroupItemCounts(rng, nG, nI, false)

	// Randomly permute periods into upper set T_+, lower set T_-, and optional pivot t0.
	tPerm := rng.Perm(nT)
	var tPlus, tMinus []int
	tZero := -1
	if spec.mode == 0 {
		half := nT / 2
		tPlus = tPerm[:half]
		tMinus = tPerm[half:]
	} else {
		half := (nT - 1) / 2
		tPlus = tPerm[:half]
		tMinus = tPerm[half : 2*half]
		tZero = tPerm[2*half]
	}

	series := make([][]float64, nG)
	plantedMG := make([]float64, nG)
	plantedNG := make([]float64, nG)
	lpMG := make([]float64, nG)
	lpNG := make([]float64, nG)

	var deltaTotal float64
	negShift := spec.mode == 1 && rng.IntN(2) == 1
	deltaTicks := make([]int64, nG)
	if spec.mode == 1 {
		halfBaseTicks := baseTicks / 2
		var totalTicks int64
		if rng.IntN(2) == 0 {
			totalTicks = halfBaseTicks
		} else {
			lo := max(int64(1), halfBaseTicks/4)
			totalTicks = lo + rng.Int64N(halfBaseTicks-lo+1)
		}
		rem := totalTicks
		for g := range nG {
			if g == nG-1 {
				deltaTicks[g] = rem
			} else if rem > 0 {
				take := rng.Int64N(rem + 1)
				deltaTicks[g] = take
				rem -= take
			}
		}
		deltaTotal = float64(totalTicks) * q
	}

	for g := range nG {
		series[g] = make([]float64, nT)
		spreadTicks := max(int64(2), baseTicks/2+rng.Int64N(baseTicks))

		switch spec.mode {
		case 0:
			k := int64(2 + rng.IntN(max(2, groupCounts[g]/2)))
			mTicks := k * baseTicks
			plantedNG[g] = float64(k)
			plantedMG[g] = float64(mTicks) * q
			lpNG[g] = plantedNG[g]
			lpMG[g] = plantedMG[g]
			for _, t := range tPlus {
				extra := 1 + rng.Int64N(spreadTicks)
				series[g][t] = float64(mTicks+extra) * q
			}
			for _, t := range tMinus {
				sub := rng.Int64N(min(mTicks, spreadTicks) + 1)
				series[g][t] = float64(mTicks-sub) * q
			}
		case 1:
			minK := 2
			if negShift {
				minK = 3
			}
			k := int64(minK + rng.IntN(max(2, groupCounts[g]/2)))
			mTicks := k * baseTicks
			y0Ticks := mTicks + deltaTicks[g]
			if negShift {
				y0Ticks = mTicks - deltaTicks[g]
			}
			series[g][tZero] = float64(y0Ticks) * q
			plantedNG[g] = float64(k)
			plantedMG[g] = float64(mTicks) * q
			lpMG[g] = series[g][tZero]
			lpNG[g] = lpMG[g] / roundingBase

			hiFloor := max(mTicks, y0Ticks)
			loCeil := min(mTicks, y0Ticks)
			for _, t := range tPlus {
				extra := 1 + rng.Int64N(spreadTicks)
				series[g][t] = float64(hiFloor+extra) * q
			}
			for _, t := range tMinus {
				sub := rng.Int64N(min(loCeil, spreadTicks) + 1)
				series[g][t] = float64(loCeil-sub) * q
			}
		default:
			// Mode 2: every group has 0 < Y[g][tZero] < B so n_g >= 1 binds at optimality.
			y0Ticks := int64(1) + rng.Int64N(max(int64(1), baseTicks-1))
			series[g][tZero] = float64(y0Ticks) * q
			plantedNG[g] = 1.0
			plantedMG[g] = roundingBase
			lpNG[g] = 1.0
			lpMG[g] = roundingBase
			for _, t := range tPlus {
				extra := 1 + rng.Int64N(spreadTicks)
				series[g][t] = float64(baseTicks+extra) * q
			}
			for _, t := range tMinus {
				sub := rng.Int64N(baseTicks + 1)
				series[g][t] = float64(baseTicks-sub) * q
			}
		}
	}

	w, plantedGroup, plantedObj, density := splitExactSeriesAndPack(
		rng, nG, nI, nT, roundingBase, q, series, groupCounts, spec.sparsity, true,
	)

	var sumPlus, sumMinus, sumZero float64
	for _, t := range tPlus {
		for i := range nI {
			sumPlus += w[t][i]
		}
	}
	for _, t := range tMinus {
		for i := range nI {
			sumMinus += w[t][i]
		}
	}
	if tZero >= 0 {
		for i := range nI {
			sumZero += w[tZero][i]
		}
	}

	var exactLP, exactMILP float64
	switch spec.mode {
	case 0:
		exactLP = sumPlus - sumMinus
		exactMILP = exactLP
	case 1:
		exactLP = sumPlus - sumMinus
		exactMILP = exactLP + deltaTotal
	default:
		exactLP = (sumPlus - sumMinus) + (float64(nG)*roundingBase - sumZero)
		exactMILP = exactLP
	}

	if math.Abs(plantedObj-exactMILP) > 1e-7*max(1.0, exactMILP) {
		panic(fmt.Sprintf("exactOptMAD construction mismatch: planted=%.12g exactMILP=%.12g", plantedObj, exactMILP))
	}

	mdl := bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
	inst := exactOptMADInstance{
		spec:         spec,
		roundingBase: roundingBase,
		quantum:      q,
		model:        mdl,
		weights:      w,
		groupSeries:  series,
		plantedGroup: plantedGroup,
		plantedMG:    plantedMG,
		plantedNG:    plantedNG,
		plantedLPMG:  lpMG,
		plantedLPNG:  lpNG,
		exactLP:      exactLP,
		exactMILP:    exactMILP,
		density:      density,
	}
	return inst
}

func (inst exactOptMADInstance) buildWitness(p *gb.CompiledProblem, mg, ng []float64) []float64 {
	nG, nI, nT := inst.spec.nG, inst.spec.nI, inst.spec.nT
	vals := make([]float64, p.NumVars())
	stride := nI + nT + 3
	for i := range nI {
		vals[inst.plantedGroup[i]*stride+i] = 1.0
	}
	for g := range nG {
		base := g * stride
		for t := range nT {
			vals[base+nI+t] = math.Abs(inst.groupSeries[g][t] - mg[g])
		}
		vals[base+nI+nT] = mg[g]
		vals[base+nI+nT+1] = ng[g]
		vals[base+nI+nT+2] = math.Abs(mg[g] - inst.roundingBase*ng[g])
	}
	return vals
}

func assertExactOptMADSoundness(t *testing.T, tag, label string, p *gb.CompiledProblem, res gb.Result, exactLP, exactMILP float64, presolveDisabled bool) bool {
	t.Helper()
	hasInc := assertDecomposedMADSoundness(t, tag, label, p, res, exactMILP)
	tol := 1e-5 * max(1.0, exactMILP)

	if hasInc && res.Objective < exactMILP-tol {
		t.Fatalf("[%s/%s] feasible incumbent %.8f undercuts analytical global minimum exactMILP %.8f",
			tag, label, res.Objective, exactMILP)
	}
	if res.Status == gb.StatusOptimal && math.Abs(res.Objective-exactMILP) > tol {
		t.Fatalf("[%s/%s] Optimal %.8f != exactMILP %.8f (rootLP=%.8f bound=%.8f nodes=%d)",
			tag, label, res.Objective, exactMILP, res.RootLPBound, res.BestBound, res.Nodes)
	}
	if res.Backend == gb.BackendGo && (res.Status == gb.StatusOptimal || (res.Nodes > 0 && res.RootLPBound != 0)) {
		if res.RootLPBound < exactLP-tol || res.RootLPBound > exactMILP+tol {
			t.Fatalf("[%s/%s] RootLPBound %.8f outside [exactLP=%.8f, exactMILP=%.8f]",
				tag, label, res.RootLPBound, exactLP, exactMILP)
		}
		if presolveDisabled || math.Abs(exactLP-exactMILP) <= tol {
			if math.Abs(res.RootLPBound-exactLP) > tol {
				t.Fatalf("[%s/%s] RootLPBound %.8f != exactLP %.8f", tag, label, res.RootLPBound, exactLP)
			}
		}
	}
	if res.Backend == gb.BackendCBC && res.RootLPBound != 0 {
		if res.RootLPBound < exactLP-tol || res.RootLPBound > exactMILP+tol {
			t.Fatalf("[%s/%s] CBC RootLPBound %.8f outside [exactLP=%.8f, exactMILP=%.8f]",
				tag, label, res.RootLPBound, exactLP, exactMILP)
		}
	}
	return hasInc
}

func exactOptMADSeeds() []struct {
	seed   uint64
	gByte  uint8
	size   uint16
	modeSp uint8
	base   uint8
} {
	return []struct {
		seed   uint64
		gByte  uint8
		size   uint16
		modeSp uint8
		base   uint8
	}{
		{11, 0, sizeRawForItems(32), 0 | (0 << 2), 0},
		{22, 1, sizeRawForItems(45), 1 | (1 << 2), 1},
		{33, 2, sizeRawForItems(60), 2 | (2 << 2), 2},
		{44, 3, sizeRawForItems(140), 0 | (3 << 2), 3},
		{55, 4, sizeRawForItems(240), 1 | (0 << 2), 4},
		{66, 5, sizeRawForItems(400), 2 | (1 << 2), 5},
		{77, 6, sizeRawForItems(700), 0 | (2 << 2), 6},
		{88, 7, sizeRawForItems(950), 1 | (3 << 2), 3},
		{99, 2, sizeRawForItems(1200), 2 | (0 << 2), 4},
	}
}

func TestExactOptimumMADGeneratorInvariants(t *testing.T) {
	t.Parallel()
	for _, s := range exactOptMADSeeds() {
		spec := decodeExactOptMADSpec(s.seed, s.gByte, s.size, s.modeSp, s.base)
		tag := fmt.Sprintf("exact_mad_g%d_i%d_m%d_s%d", spec.nG, spec.nI, spec.mode, spec.sparsity)
		inst := buildExactOptimumMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}
		milpW := inst.buildWitness(p, inst.plantedMG, inst.plantedNG)
		bV, rV, iV := p.MaxViolation(milpW, true)
		if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
			t.Fatalf("[%s] integer witness violated model: b=%.2e r=%.2e i=%.2e", tag, bV, rV, iV)
		}
		if got := p.EvaluateObjective(milpW); math.Abs(got-inst.exactMILP) > 1e-7*max(1.0, inst.exactMILP) {
			t.Fatalf("[%s] integer witness obj=%.12g != exactMILP=%.12g", tag, got, inst.exactMILP)
		}

		lpW := inst.buildWitness(p, inst.plantedLPMG, inst.plantedLPNG)
		bV, rV, _ = p.MaxViolation(lpW, false)
		if bV > 1e-9 || rV > 1e-9 {
			t.Fatalf("[%s] LP witness violated model: b=%.2e r=%.2e", tag, bV, rV)
		}
		if gotLP := p.EvaluateObjective(lpW); math.Abs(gotLP-inst.exactLP) > 1e-7*max(1.0, inst.exactLP) {
			t.Fatalf("[%s] LP witness obj=%.12g != exactLP=%.12g", tag, gotLP, inst.exactLP)
		}

		if spec.nI <= 75 {
			res := mustSolveCompiled(t, p, gb.WithPresolve(false), gb.WithTimeLimit(3*time.Second))
			assertExactOptMADSoundness(t, tag, "go-nopre", p, res, inst.exactLP, inst.exactMILP, true)
		}
	}
}

// FuzzMADExactOptimum fuzzes pure-Go MILP on medium-to-large MAD models whose Root LP
// optimum and integer MILP optimum are both exact by analytical construction.
//
//	go test -run=^$ -fuzz=^FuzzMADExactOptimum$ -fuzztime=30s .
func FuzzMADExactOptimum(f *testing.F) {
	for _, s := range exactOptMADSeeds() {
		f.Add(s.seed, s.gByte, s.size, s.modeSp, s.base)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, modeSparsityByte, baseByte uint8) {
		spec := decodeExactOptMADSpec(seed, gByte, sizeRaw, modeSparsityByte, baseByte)
		if raceEnabled && testing.Short() && spec.nI > 250 {
			t.Skipf("skipping large %d-item exact-opt MAD under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_exopt_g%d_i%d_t%d_m%d_s%d_%d",
			spec.nG, spec.nI, spec.nT, spec.mode, spec.sparsity, seed)
		inst := buildExactOptimumMAD(spec, tag)
		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 350 * time.Millisecond
		if spec.nI > 350 {
			timeLimit = 700 * time.Millisecond
		}
		const maxNodes = 1500

		wFull := [3]int{1, 2, 4}[int(seed)%3]
		resGo := mustSolveCompiled(t, p,
			gb.WithPureGo(),
			gb.WithPresolve(true),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
			gb.WithWorkers(wFull),
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		hasIncGo := assertExactOptMADSoundness(t, tag, "go", p, resGo, inst.exactLP, inst.exactMILP, false)

		var altName string
		var altOpt gb.Option
		noPre := false
		switch (seed ^ uint64(modeSparsityByte)) % 5 {
		case 0:
			altName = "go-scalar"
			altOpt = gb.WithoutSIMD()
		case 1:
			altName = "go-nopresolve"
			altOpt = gb.WithPresolve(false)
			noPre = true
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

		resAlt := mustSolveCompiled(t, p, gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes), altOpt)
		hasIncAlt := assertExactOptMADSoundness(t, tag, altName, p, resAlt, inst.exactLP, inst.exactMILP, noPre)
		assertTwoGoSolvesConsistent(t, tag, "go", resGo, hasIncGo, altName, resAlt, hasIncAlt)

		if formalFuzzEnabled() && p.NumVars() <= 600 {
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, tag+"/rootLP", p)
			sols := [][]float64{inst.buildWitness(p, inst.plantedMG, inst.plantedNG)}
			if hasIncGo {
				sols = append(sols, resGo.Values)
			}
			verifyFormalSeparatedCuts(t, tag+"/cuts", p, sols)
			if p.NumVars() <= 240 {
				resTr, tr := verifyFormalVIPRTrace(
					t,
					tag+"/vipr",
					p,
					50,
					250*time.Millisecond,
					gb.WithPresolve(false),
					gb.WithCuts(true),
					gb.WithHeuristics(true),
				)
				_ = assertExactOptMADSoundness(t, tag, "vipr", p, resTr, inst.exactLP, inst.exactMILP, true)
				assertFullFeatureVsVIPR(t, tag+"/full_vs_vipr", p, resGo, resTr, tr, 1e-6, 1e-6)
			}
		}
	})
}

// =============================================================================
// Adversarial Conditioning, Symmetry-Orbit & Cut-Stress MAD
// =============================================================================

type advMADSpec struct {
	seed   uint64
	nG     int
	nI     int
	nT     int
	regime uint8
}

type advMADInstance struct {
	spec         advMADSpec
	roundingBase float64
	model        *gb.Model
	weights      [][]float64
	plantedGroup []int
	plantedObj   float64
}

func decodeAdvMADSpec(seed uint64, gByte uint8, sizeRaw uint16, regimeByte uint8) advMADSpec {
	rng := rand.New(rand.NewPCG(seed^0x27d4eb2f165667c5, seed^0x165667b19e3779f9))
	nG := advMADMinGroups + int(gByte%(advMADMaxGroups-advMADMinGroups+1))
	nI := logUniformItems(sizeRaw, advMADMinItems, advMADMaxItems)
	regime := regimeByte % 5

	var nT int
	switch regime {
	case 2:
		horizons := []int{10, 12, 16}
		nT = horizons[rng.IntN(len(horizons))]
	case 4:
		odd := []int{3, 5, 7}
		nT = odd[rng.IntN(len(odd))]
	default:
		periods := []int{4, 5, 6, 8}
		nT = periods[rng.IntN(len(periods))]
	}
	return advMADSpec{
		seed:   seed,
		nG:     nG,
		nI:     nI,
		nT:     nT,
		regime: regime,
	}
}

func buildAdversarialMAD(spec advMADSpec, tag string) advMADInstance {
	rng := rand.New(rand.NewPCG(spec.seed^0x6a09e667f3bcc908, spec.seed^0xbb67ae8584caa73b))
	nG, nI, nT := spec.nG, spec.nI, spec.nT
	groupCounts := allocateGroupItemCounts(rng, nG, nI, false)

	var roundingBase float64
	nReg := nI - nG
	regWeights := make([][]float64, nReg)
	regGroup := make([]int, nReg)
	anchorWeights := make([][]float64, nG)
	anchorGroup := make([]int, nG)
	regCursor := 0

	switch spec.regime {
	case 0:
		// Whales + krill multi-scale decomposition spanning 1e-2 to 1e6 within dev rows.
		bases := []float64{100.0, 1000.0}
		roundingBase = bases[rng.IntN(len(bases))]
		for g := range nG {
			m := groupCounts[g]
			target := roundingBase * float64(500+rng.IntN(4500))
			cols := make([][]float64, m)
			for k := range m {
				cols[k] = make([]float64, nT)
			}
			for t := range nT {
				krillSum := 0.0
				for k := 2; k < m; k++ {
					val := math.Round((0.01+rng.Float64()*4.99)*100.0) / 100.0
					cols[k][t] = val
					krillSum += val
				}
				rem := max(100.0, target-krillSum)
				split := 0.35 + 0.30*rng.Float64()
				w0 := math.Round(rem*split*100.0) / 100.0
				w1 := math.Round((rem-w0)*100.0) / 100.0
				cols[0][t] = w0
				cols[1][t] = w1
			}
			anchorWeights[g] = cols[0]
			anchorGroup[g] = g
			for k := 1; k < m; k++ {
				regWeights[regCursor] = cols[k]
				regGroup[regCursor] = g
				regCursor++
			}
		}

	case 1:
		// Near-collinear split-twin items hedged by designated anchors.
		bases := []float64{50.0, 100.0}
		roundingBase = bases[rng.IntN(len(bases))]
		for g := range nG {
			m := groupCounts[g]
			cols := make([][]float64, m)
			for k := range m {
				cols[k] = make([]float64, nT)
			}
			groupSum := make([]float64, nT)
			for k := 0; k < m-1; {
				if k+2 < m && rng.IntN(3) != 0 {
					baseVec := make([]float64, nT)
					for t := range nT {
						baseVec[t] = 40.0 + float64(rng.IntN(400))
					}
					alpha := 0.2 + 0.6*rng.Float64()
					for t := range nT {
						eps1 := (rng.Float64()*2.0 - 1.0) * 1e-3
						eps2 := (rng.Float64()*2.0 - 1.0) * 1e-3
						cols[k][t] = math.Max(0, math.Round((alpha*baseVec[t]+eps1)*1e4)/1e4)
						cols[k+1][t] = math.Max(0, math.Round(((1.0-alpha)*baseVec[t]+eps2)*1e4)/1e4)
						groupSum[t] += cols[k][t] + cols[k+1][t]
					}
					k += 2
					continue
				}
				for t := range nT {
					cols[k][t] = float64(10 + rng.IntN(300))
					groupSum[t] += cols[k][t]
				}
				k++
			}
			maxSum := 0.0
			for t := range nT {
				maxSum = max(maxSum, groupSum[t])
			}
			target := math.Ceil((maxSum+roundingBase)/roundingBase) * roundingBase
			for t := range nT {
				cols[m-1][t] = target - groupSum[t]
			}
			anchorWeights[g] = cols[m-1]
			anchorGroup[g] = g
			for k := range m - 1 {
				regWeights[regCursor] = cols[k]
				regGroup[regCursor] = g
				regCursor++
			}
		}

	case 2:
		// Staircase / sliding-window support on longer horizons T in {10, 12, 16}.
		roundingBase = 100.0
		for g := range nG {
			m := groupCounts[g]
			cols := make([][]float64, m)
			for k := range m {
				cols[k] = make([]float64, nT)
			}
			groupSum := make([]float64, nT)
			for k := range m - 1 {
				startT := k % nT
				win := 2 + (k % 2)
				for off := range win {
					t := (startT + off) % nT
					v := float64(50 + rng.IntN(450))
					cols[k][t] = v
					groupSum[t] += v
				}
			}
			maxSum := 0.0
			for t := range nT {
				maxSum = max(maxSum, groupSum[t])
			}
			target := math.Ceil((maxSum+roundingBase)/roundingBase) * roundingBase
			for t := range nT {
				cols[m-1][t] = target - groupSum[t]
			}
			anchorWeights[g] = cols[m-1]
			anchorGroup[g] = g
			for k := range m - 1 {
				regWeights[regCursor] = cols[k]
				regGroup[regCursor] = g
				regCursor++
			}
		}

	case 3:
		// Symmetry-orbit trap: cloned designated anchors plus exact-clone and near-clone clusters.
		roundingBase = 100.0
		sharedAnchor := make([]float64, nT)
		for t := range nT {
			sharedAnchor[t] = float64(200 + rng.IntN(600))
		}
		numSymAnchors := min(nG, 2+rng.IntN(max(1, nG-1)))
		for g := range nG {
			m := groupCounts[g]
			anchorWeights[g] = make([]float64, nT)
			anchorGroup[g] = g
			if g < numSymAnchors {
				copy(anchorWeights[g], sharedAnchor)
				if g == numSymAnchors-1 && rng.IntN(2) == 1 {
					anchorWeights[g][0] += 1e-6
				}
			} else {
				for t := range nT {
					anchorWeights[g][t] = float64(150 + rng.IntN(700))
				}
			}
			for k := 0; k < m-1; {
				cluster := min(m-1-k, 2+rng.IntN(4))
				proto := make([]float64, nT)
				for t := range nT {
					proto[t] = float64(20 + rng.IntN(300))
				}
				for c := range cluster {
					col := slices.Clone(proto)
					if c == cluster-1 && rng.IntN(2) == 1 {
						col[tMod(c, nT)] += 1e-5
					}
					regWeights[regCursor] = col
					regGroup[regCursor] = g
					regCursor++
					k++
				}
			}
		}

	default:
		// Extreme rounding base with binding n_g = 1 and half-base fractional peaks.
		bases := []float64{0.1, 0.5, 1.0, 7.0, 5000.0, 50000.0}
		roundingBase = bases[rng.IntN(len(bases))]
		halfBase := 0.5 * roundingBase
		series := make([][]float64, nG)
		for g := range nG {
			series[g] = make([]float64, nT)
			if g%2 == 0 {
				// Binding n_g = 1 below roundingBase.
				for t := range nT {
					frac := 0.2 + 0.65*rng.Float64()
					series[g][t] = roundingBase * frac
				}
			} else {
				// Half-base peak across odd periods.
				k := float64(1 + rng.IntN(6))
				peak := k*roundingBase + halfBase
				for t := range nT {
					switch {
					case t == nT-1:
						series[g][t] = peak
					case t%2 == 0:
						series[g][t] = peak + roundingBase*(0.3+rng.Float64())
					default:
						series[g][t] = max(0.0, peak-roundingBase*(0.3+rng.Float64()))
					}
				}
			}
		}
		w, plantedGroup, plantedObj, _ := splitExactSeriesAndPack(
			rng, nG, nI, nT, roundingBase, 1.0, series, groupCounts, 1, false,
		)
		return advMADInstance{
			spec:         spec,
			roundingBase: roundingBase,
			model:        bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w),
			weights:      w,
			plantedGroup: plantedGroup,
			plantedObj:   plantedObj,
		}
	}

	w, plantedGroup, _ := permuteAndPackMADColumns(rng, nG, nI, nT, regWeights, regGroup, anchorWeights, anchorGroup)
	groupSums := make([][]float64, nG)
	for g := range nG {
		groupSums[g] = make([]float64, nT)
	}
	for i := range nI {
		g := plantedGroup[i]
		for t := range nT {
			groupSums[g][t] += w[t][i]
		}
	}
	plantedObj := 0.0
	for g := range nG {
		cost, _, _ := exactGroupMADSolution(groupSums[g], roundingBase)
		plantedObj += cost
	}

	return advMADInstance{
		spec:         spec,
		roundingBase: roundingBase,
		model:        bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w),
		weights:      w,
		plantedGroup: plantedGroup,
		plantedObj:   plantedObj,
	}
}

func tMod(a, b int) int {
	return a % b
}

func (inst advMADInstance) buildWitness(p *gb.CompiledProblem) []float64 {
	return decomposedMADInstance{
		spec: decomposedMADSpec{
			nG: inst.spec.nG,
			nI: inst.spec.nI,
			nT: inst.spec.nT,
		},
		roundingBase: inst.roundingBase,
		weights:      inst.weights,
		plantedGroup: inst.plantedGroup,
	}.buildPlantedPrimalVector(p)
}

func advMADSeeds() []struct {
	seed   uint64
	gByte  uint8
	size   uint16
	regime uint8
} {
	return []struct {
		seed   uint64
		gByte  uint8
		size   uint16
		regime uint8
	}{
		{101, 0, sizeRawForItems(32), 0},
		{102, 2, sizeRawForItems(240), 0},
		{103, 1, sizeRawForItems(36), 1},
		{104, 3, sizeRawForItems(220), 1},
		{105, 2, sizeRawForItems(48), 2},
		{106, 4, sizeRawForItems(300), 2},
		{107, 1, sizeRawForItems(40), 3},
		{108, 5, sizeRawForItems(280), 3},
		{109, 0, sizeRawForItems(36), 4},
		{110, 4, sizeRawForItems(260), 4},
	}
}

func TestAdversarialMADGeneratorInvariants(t *testing.T) {
	t.Parallel()
	for _, s := range advMADSeeds() {
		spec := decodeAdvMADSpec(s.seed, s.gByte, s.size, s.regime)
		tag := fmt.Sprintf("adv_mad_g%d_i%d_r%d", spec.nG, spec.nI, spec.regime)
		inst := buildAdversarialMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}
		w := inst.buildWitness(p)
		bV, rV, iV := p.MaxViolation(w, true)
		if bV > 1e-8 || rV > 1e-8 || iV > 1e-8 {
			t.Fatalf("[%s] adversarial witness violated model: b=%.2e r=%.2e i=%.2e", tag, bV, rV, iV)
		}
		evalObj := p.EvaluateObjective(w)
		if math.Abs(evalObj-inst.plantedObj) > 1e-5*max(1.0, inst.plantedObj) {
			t.Fatalf("[%s] witness obj=%.10g != plantedObj=%.10g", tag, evalObj, inst.plantedObj)
		}
	}
}

// FuzzMADAdversarialRegimes fuzzes pure-Go MILP on five adversarial MAD structures:
// multi-scale whales+krill, near-collinear split twins, staircase horizons, symmetry-orbit
// clones, and extreme-base cut stress.
//
//	go test -run=^$ -fuzz=^FuzzMADAdversarialRegimes$ -fuzztime=30s .
func FuzzMADAdversarialRegimes(f *testing.F) {
	for _, s := range advMADSeeds() {
		f.Add(s.seed, s.gByte, s.size, s.regime)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, regimeByte uint8) {
		spec := decodeAdvMADSpec(seed, gByte, sizeRaw, regimeByte)
		if raceEnabled && testing.Short() && spec.nI > 180 {
			t.Skipf("skipping large %d-item adversarial MAD under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_adv_g%d_i%d_t%d_r%d_%d",
			spec.nG, spec.nI, spec.nT, spec.regime, seed)
		inst := buildAdversarialMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 350 * time.Millisecond
		if spec.nI > 250 || spec.nT > 10 {
			timeLimit = 650 * time.Millisecond
		}
		const maxNodes = 1500

		wFull := [3]int{1, 2, 4}[int(seed)%3]
		resGo := mustSolveCompiled(t, p,
			gb.WithPureGo(),
			gb.WithPresolve(true),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
			gb.WithWorkers(wFull),
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		hasIncGo := assertDecomposedMADSoundness(t, tag, "go", p, resGo, inst.plantedObj)

		var altName string
		var altOpt gb.Option
		switch spec.regime {
		case 3:
			altName = "go-nopresolve"
			altOpt = gb.WithPresolve(false)
		case 4:
			altName = "go-nocuts"
			altOpt = gb.WithCuts(false)
		default:
			switch seed % 4 {
			case 0:
				altName = "go-scalar"
				altOpt = gb.WithoutSIMD()
			case 1:
				altName = "go-nopresolve"
				altOpt = gb.WithPresolve(false)
			case 2:
				altName = "go-nocuts"
				altOpt = gb.WithCuts(false)
			default:
				altName = "go-noheur"
				altOpt = gb.WithHeuristics(false)
			}
		}

		resAlt := mustSolveCompiled(t, p, gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes), altOpt)
		hasIncAlt := assertDecomposedMADSoundness(t, tag, altName, p, resAlt, inst.plantedObj)
		assertTwoGoSolvesConsistent(t, tag, "go", resGo, hasIncGo, altName, resAlt, hasIncAlt)

		if formalFuzzEnabled() && p.NumVars() <= 600 {
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, tag+"/rootLP", p)
			sols := [][]float64{inst.buildWitness(p)}
			if hasIncGo {
				sols = append(sols, resGo.Values)
			}
			verifyFormalSeparatedCuts(t, tag+"/cuts", p, sols)
			if p.NumVars() <= 240 {
				resTr, tr := verifyFormalVIPRTrace(
					t,
					tag+"/vipr",
					p,
					40,
					250*time.Millisecond,
					gb.WithPresolve(false),
					gb.WithCuts(true),
					gb.WithHeuristics(true),
				)
				_ = assertDecomposedMADSoundness(t, tag, "vipr", p, resTr, inst.plantedObj)
				assertFullFeatureVsVIPR(t, tag+"/full_vs_vipr", p, resGo, resTr, tr, 1e-6, 1e-6)
			}
		}
	})
}

// =============================================================================
// Metamorphic Invariance & Interrupt-Boundary MAD Fuzzing
// =============================================================================

func scaleMADWeights(w [][]float64, alpha float64) [][]float64 {
	out := make([][]float64, len(w))
	for t, row := range w {
		out[t] = make([]float64, len(row))
		for i, v := range row {
			out[t][i] = v * alpha
		}
	}
	return out
}

func permuteMADWeights(rng *rand.Rand, w [][]float64, nG int) [][]float64 {
	nT := len(w)
	nI := len(w[0])
	nReg := nI - nG
	tPerm := rng.Perm(nT)
	regPerm := rng.Perm(nReg)
	desPerm := rng.Perm(nG)

	out := make([][]float64, nT)
	for dstT, srcT := range tPerm {
		out[dstT] = make([]float64, nI)
		for dstI, srcI := range regPerm {
			out[dstT][dstI] = w[srcT][srcI]
		}
		for dstOff, srcOff := range desPerm {
			out[dstT][nReg+dstOff] = w[srcT][nReg+srcOff]
		}
	}
	return out
}

func shiftDesignatedWeights(rng *rand.Rand, w [][]float64, nG int, roundingBase float64) [][]float64 {
	nT := len(w)
	nI := len(w[0])
	dStart := nI - nG
	shifts := make([]float64, nG)
	mults := []float64{1, 5, 50, 500}
	for g := range nG {
		shifts[g] = mults[rng.IntN(len(mults))] * roundingBase
	}
	out := make([][]float64, nT)
	for t := range nT {
		out[t] = slices.Clone(w[t])
		for g := range nG {
			out[t][dStart+g] += shifts[g]
		}
	}
	return out
}

// FuzzMADMetamorphicAndLimits checks algebraic MAD invariances (column/period permutation,
// scaling homogeneity, and designated-anchor constant base shift) and tests randomized tight
// time, node, iteration, and context-deadline stops without breaking bound soundness.
//
//	go test -run=^$ -fuzz=^FuzzMADMetamorphicAndLimits$ -fuzztime=30s .
func FuzzMADMetamorphicAndLimits(f *testing.F) {
	for seed := range uint64(4) {
		for xform := range uint8(4) {
			f.Add(seed, uint8(seed%4), uint16(seed*7000), xform)
		}
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, xformByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x9b05688c2b3e6c1f, seed^0x5be0cd19137e2179))
		nG := 3 + int(gByte%4) // 3..6 groups
		nI := logUniformItems(sizeRaw, 20, 250)
		if raceEnabled && testing.Short() && nI > 140 {
			t.Skipf("skipping %d-item metamorphic MAD under -race -short", nI)
		}
		periods := []int{3, 4, 5, 6}
		nT := periods[rng.IntN(len(periods))]
		xform := xformByte % 4

		spec := exactOptMADSpec{
			seed:     seed,
			nG:       nG,
			nI:       nI,
			nT:       nT,
			mode:     uint8(nT % 2), // even T => mode 0, odd T => mode 1
			sparsity: uint8(seed % 4),
			baseIdx:  int((seed / 7) % 5),
		}
		tag := fmt.Sprintf("mad_meta_x%d_g%d_i%d_t%d_%d", xform, nG, nI, nT, seed)
		inst := buildExactOptimumMAD(spec, tag)
		pBase, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] base CompileChecked failed: %v", tag, err)
		}

		const baseTime = 350 * time.Millisecond
		const maxNodes = 1500

		var resTrMeta gb.Result
		var trMeta *gb.VIPRTrace
		if formalFuzzEnabled() && pBase.NumVars() <= 300 {
			_, _ = verifyFormalPresolveRules(t, pBase)
			verifyFormalRootLP(t, tag+"/rootLP", pBase)
			verifyFormalSeparatedCuts(t, tag+"/cuts", pBase, [][]float64{inst.buildWitness(pBase, inst.plantedMG, inst.plantedNG)})
			if pBase.NumVars() <= 180 {
				resTrMeta, trMeta = verifyFormalVIPRTrace(
					t,
					tag+"/vipr",
					pBase,
					20,
					200*time.Millisecond,
					gb.WithPresolve(false),
					gb.WithCuts(true),
					gb.WithHeuristics(true),
				)
				_ = assertExactOptMADSoundness(t, tag, "vipr", pBase, resTrMeta, inst.exactLP, inst.exactMILP, true)
			}
		}

		if xform == 3 {
			// Early-stop and interrupt-boundary fuzzing across 3 distinct limit modes.
			ms := time.Duration(1+rng.IntN(25)) * time.Millisecond
			maxN := []int{0, 1, 2, 5}[rng.IntN(4)]
			maxIt := []int{10, 50, 150}[rng.IntN(3)]

			rTime := mustSolveCompiled(t, pBase, gb.WithTimeLimit(ms))
			rNodes := mustSolveCompiled(t, pBase, gb.WithMaxNodes(maxN), gb.WithTimeLimit(baseTime))
			rIter := mustSolveCompiled(t, pBase, gb.WithMaxIter(maxIt), gb.WithTimeLimit(baseTime))

			incT := assertExactOptMADSoundness(t, tag, "limit-time", pBase, rTime, inst.exactLP, inst.exactMILP, false)
			incN := assertExactOptMADSoundness(t, tag, "limit-nodes", pBase, rNodes, inst.exactLP, inst.exactMILP, false)
			incI := assertExactOptMADSoundness(t, tag, "limit-iter", pBase, rIter, inst.exactLP, inst.exactMILP, false)

			assertTwoGoSolvesConsistent(t, tag, "limit-time", rTime, incT, "limit-nodes", rNodes, incN)
			assertTwoGoSolvesConsistent(t, tag, "limit-nodes", rNodes, incN, "limit-iter", rIter, incI)
			assertTwoGoSolvesConsistent(t, tag, "limit-time", rTime, incT, "limit-iter", rIter, incI)

			// Exercise context deadline abort path cleanly.
			ctx, cancel := context.WithTimeout(t.Context(), ms)
			defer cancel()
			rCtx, ctxErr := gb.SolveCompiled(ctx, pBase)
			if ctxErr == nil {
				assertExactOptMADSoundness(t, tag, "limit-ctx", pBase, rCtx, inst.exactLP, inst.exactMILP, false)
			} else if !errors.Is(ctxErr, context.DeadlineExceeded) {
				t.Fatalf("[%s] unexpected context solve error: %v", tag, ctxErr)
			}
			return
		}

		wBase := [3]int{1, 2, 4}[int(seed)%3]
		resBase := mustSolveCompiled(t, pBase,
			gb.WithPureGo(),
			gb.WithPresolve(true),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
			gb.WithWorkers(wBase),
			gb.WithTimeLimit(baseTime),
			gb.WithMaxNodes(maxNodes),
		)
		hasIncBase := assertExactOptMADSoundness(t, tag, "base", pBase, resBase, inst.exactLP, inst.exactMILP, false)
		if trMeta != nil {
			assertFullFeatureVsVIPR(t, tag+"/base_vs_vipr", pBase, resBase, resTrMeta, trMeta, 1e-6, 1e-6)
		}

		var wTrans [][]float64
		var bTrans float64
		alpha := 1.0
		switch xform {
		case 0:
			wTrans = permuteMADWeights(rng, inst.weights, nG)
			bTrans = inst.roundingBase
		case 1:
			alphas := []float64{1.0 / 256.0, 0.01, 0.37, 3.14159, 100.0, 256.0}
			alpha = alphas[rng.IntN(len(alphas))]
			wTrans = scaleMADWeights(inst.weights, alpha)
			bTrans = inst.roundingBase * alpha
		default:
			wTrans = shiftDesignatedWeights(rng, inst.weights, nG, inst.roundingBase)
			bTrans = inst.roundingBase
		}

		mTrans := bench.BuildMADFromWeights(tag+"_xf", nG, nI, nT, bTrans, wTrans)
		pTrans, err := mTrans.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] transformed CompileChecked failed: %v", tag, err)
		}
		resTrans := mustSolveCompiled(t, pTrans, gb.WithTimeLimit(baseTime), gb.WithMaxNodes(maxNodes))
		hasIncTrans := assertExactOptMADSoundness(t, tag, "xform", pTrans, resTrans, inst.exactLP*alpha, inst.exactMILP*alpha, false)

		if alpha != 1.0 {
			resScaledBack := resTrans
			resScaledBack.Objective = resTrans.Objective / alpha
			resScaledBack.BestBound = resTrans.BestBound / alpha
			assertTwoGoSolvesConsistent(t, tag, "base", resBase, hasIncBase, "xform/unscaled", resScaledBack, hasIncTrans)
		} else {
			assertTwoGoSolvesConsistent(t, tag, "base", resBase, hasIncBase, "xform", resTrans, hasIncTrans)
		}
	})
}

// TestMADAdversarialSymmetryRootLPRegression pins the near-clone symmetry root LP
// recovery and presolved MILP solve for FuzzMADAdversarialRegimes/6b6c62dfc29297f2.
func TestMADAdversarialSymmetryRootLPRegression(t *testing.T) {
	t.Parallel()
	spec := decodeAdvMADSpec(49, 'G', 25909, '\r')
	inst := buildAdversarialMAD(spec, "reg_6b6c62")
	p, err := inst.model.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}

	pre := gb.Presolve(p, 5)
	if pre.Infeasible || pre.Reduced == nil {
		t.Fatalf("expected feasible presolved model, got infeasible=%v reduced=%v", pre.Infeasible, pre.Reduced != nil)
	}
	lpRes := gb.NewLPSolver(pre.Reduced, nil).Solve()
	if lpRes.Status != gb.StatusOptimal {
		t.Fatalf("presolved root LP status=%v, want Optimal", lpRes.Status)
	}
	if math.Abs(lpRes.Objective-13755.000069) > 1e-3 {
		t.Fatalf("presolved root LP obj=%.10g, want 13755.000069", lpRes.Objective)
	}

	res := mustSolveCompiled(t, p, gb.WithTimeLimit(2*time.Second), gb.WithMaxNodes(1500))
	_ = assertDecomposedMADSoundness(t, "reg_6b6c62", "go", p, res, inst.plantedObj)
}
