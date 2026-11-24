//go:build cgo && cbc

package goblin_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
)

// buildDifferentialFuzzMAD generates MAD group-item allocation models across five regimes
// that go beyond the tiny brute-force oracle size in FuzzMILPSolver:
//   - 0: medium unstructured random-weight MAD (genuine B&B gap)
//   - 1: fractional & high-dynamic-range MAD (scaling / preconditioning / LU stress)
//   - 2: planted and near-planted MAD at larger scale (presolve, root heuristics, larger bases)
//   - 3: degenerate / near-tie MAD (even T non-unique medians, cloned items, half-base ties)
//   - 4: boundary & extreme-scale MAD (T=1, I=G, all-zero rows/cols, 1e-4..1e5 dynamic range)
func buildDifferentialFuzzMAD(rng *rand.Rand, regime uint8, tag string) *gb.Model {
	switch regime % 5 {
	case 0:
		return buildDiffMADUnstructured(rng, tag)
	case 1:
		return buildDiffMADScaledFractional(rng, tag)
	case 2:
		return buildDiffMADNearPlanted(rng, tag)
	case 3:
		return buildDiffMADDegenerate(rng, tag)
	default:
		return buildDiffMADBoundaryExtreme(rng, tag)
	}
}

func buildDiffMADUnstructured(rng *rand.Rand, tag string) *gb.Model {
	nG := 2 + rng.IntN(3) // 2..4 groups
	maxExtra := min(8, 12-nG)
	nI := nG + 2 + rng.IntN(max(1, maxExtra-1)) // up to 12 items
	nT := 3 + rng.IntN(4)                       // 3..6 periods
	bases := []float64{10.0, 50.0, 100.0, 250.0}
	roundingBase := bases[rng.IntN(len(bases))]

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
		for i := range nI {
			switch rng.IntN(6) {
			case 0:
				w[t][i] = 0
			case 1:
				w[t][i] = roundingBase * float64(1+rng.IntN(15))
			default:
				w[t][i] = float64(1 + rng.IntN(2500))
			}
		}
	}
	return bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
}

func buildDiffMADScaledFractional(rng *rand.Rand, tag string) *gb.Model {
	nG := 2 + rng.IntN(2) // 2..3 groups
	nI := nG + 1 + rng.IntN(6)
	nT := 3 + rng.IntN(4)
	bases := []float64{1.0, 2.5, 10.0, 100.0, 1000.0}
	roundingBase := bases[rng.IntN(len(bases))]

	// Pick a per-instance scale factor to exercise dynamic range without exceeding safe float tolerances.
	scales := []float64{0.01, 0.1, 1.0, 10.0, 100.0}
	scale := scales[rng.IntN(len(scales))]

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
		for i := range nI {
			switch rng.IntN(5) {
			case 0:
				w[t][i] = 0
			case 1:
				// Exact fractional/scaled multiple of roundingBase.
				w[t][i] = roundingBase * float64(1+rng.IntN(8))
			case 2:
				// Small fractional weight.
				w[t][i] = math.Round((0.05+rng.Float64()*5.0)*100) / 100
			default:
				w[t][i] = math.Round((1.0+rng.Float64()*500.0)*scale*100) / 100
			}
		}
	}
	return bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
}

func buildDiffMADNearPlanted(rng *rand.Rand, tag string) *gb.Model {
	nG := 3 + rng.IntN(4) // 3..6 groups
	perGroup := 3 + rng.IntN(4)
	nI := nG * perGroup // 9..36 items
	nT := 6
	bases := []float64{50.0, 100.0, 200.0}
	roundingBase := bases[rng.IntN(len(bases))]

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
	}

	// Construct complementary zero-mean item patterns per group, hedged by the designated item
	// in D = [nI-nG, nI-1], so the unperturbed instance is near-zero-gap at larger scale.
	dStart := nI - nG
	groupSum := make([][]float64, nG)
	for g := range nG {
		groupSum[g] = make([]float64, nT)
	}
	for i := range dStart {
		gStar := i % nG
		baseVal := roundingBase * float64(2+rng.IntN(8))
		for t := range nT {
			delta := float64(rng.IntN(161) - 80)
			val := max(0.0, baseVal+delta)
			w[t][i] = val
			groupSum[gStar][t] += val
		}
	}

	// Designated item g hedges group g toward a common roundingBase multiple target.
	for g := range nG {
		maxSum := 0.0
		for t := range nT {
			maxSum = max(maxSum, groupSum[g][t])
		}
		target := math.Ceil((maxSum+roundingBase)/roundingBase) * roundingBase
		for t := range nT {
			w[t][dStart+g] = target - groupSum[g][t]
		}
	}

	// Half the time add a small sparse perturbation so there is a non-trivial but fast MIP gap.
	if rng.IntN(2) == 1 {
		nPerturb := 1 + rng.IntN(min(6, nI))
		for range nPerturb {
			t := rng.IntN(nT)
			i := rng.IntN(nI)
			w[t][i] = max(0.0, w[t][i]+float64(rng.IntN(21)-10))
		}
	}

	return bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
}

func buildDiffMADDegenerate(rng *rand.Rand, tag string) *gb.Model {
	nG := 2 + rng.IntN(3) // 2..4 groups
	nI := nG + 2 + rng.IntN(6)
	evenPeriods := []int{2, 4, 6}
	nT := evenPeriods[rng.IntN(len(evenPeriods))]
	bases := []float64{20.0, 100.0}
	roundingBase := bases[rng.IntN(len(bases))]
	halfBase := 0.5 * roundingBase

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
	}

	// Build several cloned or near-cloned columns and weights near half-multiples of roundingBase
	// to provoke symmetry, median degeneracy, and floor/ceil rounding ties.
	for i := range nI {
		if i > 0 && rng.IntN(3) == 0 {
			src := rng.IntN(i)
			for t := range nT {
				jitter := float64(rng.IntN(3) - 1) // -1, 0, or +1
				w[t][i] = max(0.0, w[t][src]+jitter)
			}
			continue
		}
		for t := range nT {
			k := float64(1 + rng.IntN(12))
			switch rng.IntN(4) {
			case 0:
				w[t][i] = k*roundingBase + halfBase
			case 1:
				w[t][i] = k * roundingBase
			case 2:
				w[t][i] = 0
			default:
				w[t][i] = k*roundingBase + float64(rng.IntN(11)-5)
			}
		}
	}
	return bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
}

func buildDiffMADBoundaryExtreme(rng *rand.Rand, tag string) *gb.Model {
	nG := 2 + rng.IntN(3) // 2..4 groups
	var nI int
	switch rng.IntN(3) {
	case 0:
		nI = nG // pure bijection of designated subset D
	case 1:
		nI = nG + 1 // 1 free item
	default:
		nI = nG + 2 + rng.IntN(4)
	}
	var nT int
	if rng.IntN(2) == 0 {
		nT = 1 // single-period boundary: z[g][0]=0, pure roundness penalty
	} else {
		nT = 2 + rng.IntN(5)
	}
	bases := []float64{0.1, 1.0, 100.0, 10000.0}
	roundingBase := bases[rng.IntN(len(bases))]

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
		// Occasionally create an all-zero period row.
		if nT > 1 && rng.IntN(6) == 0 {
			continue
		}
		for i := range nI {
			switch rng.IntN(6) {
			case 0:
				w[t][i] = 0
			case 1:
				w[t][i] = math.Round((1e-4+rng.Float64()*1e-2)*1e5) / 1e5
			case 2:
				w[t][i] = math.Round((1e3+rng.Float64()*1e5)*10) / 10
			case 3:
				w[t][i] = roundingBase * float64(1+rng.IntN(5))
			default:
				w[t][i] = math.Round((0.1+rng.Float64()*500.0)*100) / 100
			}
		}
	}
	// Occasionally zero out an entire item column.
	if rng.IntN(4) == 0 {
		zeroCol := rng.IntN(nI)
		for t := range nT {
			w[t][zeroCol] = 0
		}
	}
	return bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
}

// cbcOracleReliable reports whether upstream C++ COIN-OR CBC produced a clean,
// model-feasible reference result suitable for differential cross-checking against
// pure-Go goblin.
//
// Why we do not fail fuzz tests on CBC's own numerical issues:
// During multi-hour GCP Spot fuzz campaigns (e.g. mad_diff_r4_287, mad_diff_r4_507,
// mad_adv_cbc_g8_i30_t4_r3_107), upstream C++ CBC under Debian BLAS/GCC abandoned
// extreme-scale regime-3/4 MAD instances with StatusNumericalError or returned primal
// vectors whose internally scaled 1e-7 tolerance unscaled to bound=1e-5 / row=5e-4,
// while pure-Go goblin solved the exact same instances strictly within 1e-5.
// Differential fuzzers exist to verify pure-Go goblin (never to fail on upstream C++
// CBC quirks), so we always assert full invariants on pure-Go and only compare against
// CBC when CBC itself produced a numerically clean reference.
func cbcOracleReliable(p *gb.CompiledProblem, res gb.Result) (reliable, hasIncumbent bool) {
	if res.FellBackToGo {
		return false, false
	}
	switch res.Status {
	case gb.StatusInfeasible, gb.StatusUnbounded, gb.StatusNumericalError, gb.StatusUndefined:
		return false, false
	}
	if len(res.Values) == p.NumVars() {
		bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
		if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
			return false, false
		}
		evalObj := p.EvaluateObjective(res.Values)
		if math.Abs(evalObj-res.Objective) > 1e-5*max(1.0, math.Abs(res.Objective)) {
			return false, false
		}
		hasIncumbent = true
	} else if res.Status == gb.StatusOptimal {
		return false, false
	}
	if res.Status == gb.StatusOptimal {
		boundTol := 1e-5 * max(1.0, math.Abs(res.Objective))
		if p.Sense() == gb.Minimize && res.BestBound > res.Objective+boundTol {
			return false, false
		}
		if p.Sense() == gb.Maximize && res.BestBound < res.Objective-boundTol {
			return false, false
		}
	}
	return true, hasIncumbent
}

func assertMADDifferentialEquivalence(t *testing.T, tag, goName string, p *gb.CompiledProblem, resGo, resCBC gb.Result) {
	t.Helper()

	if resGo.Backend != gb.BackendGo {
		t.Fatalf("[%s/%s] expected pure-Go backend, got %v", tag, goName, resGo.Backend)
	}
	if resCBC.Backend != gb.BackendCBC {
		t.Fatalf("[%s] expected CBC backend, got %v", tag, resCBC.Backend)
	}

	// Always enforce strict invariants on pure-Go goblin; every MAD instance built by
	// BuildMADFromWeights is guaranteed feasible and bounded below by 0.
	switch resGo.Status {
	case gb.StatusInfeasible, gb.StatusUnbounded:
		t.Fatalf("[%s/%s] impossible status %v on guaranteed-feasible MAD model (go=%v obj=%.6f nodes=%d iters=%d; cbc=%v obj=%.6f nodes=%d iters=%d)",
			tag, goName, resGo.Status,
			resGo.Status, resGo.Objective, resGo.Nodes, resGo.Iterations,
			resCBC.Status, resCBC.Objective, resCBC.Nodes, resCBC.Iterations)
	case gb.StatusNumericalError, gb.StatusUndefined:
		t.Fatalf("[%s/%s] solver failure status %v (vars=%d rows=%d; go=%v obj=%.6f bound=%.6f nodes=%d iters=%d; cbc=%v obj=%.6f bound=%.6f nodes=%d iters=%d)",
			tag, goName, resGo.Status, p.NumVars(), p.NumConstraints(),
			resGo.Status, resGo.Objective, resGo.BestBound, resGo.Nodes, resGo.Iterations,
			resCBC.Status, resCBC.Objective, resCBC.BestBound, resCBC.Nodes, resCBC.Iterations)
	}
	verifyMILPInvariants(t, tag+"/"+goName, p, resGo)
	goHasInc := verifyOptionalIncumbent(t, tag+"/"+goName, p, resGo)

	// Only cross-check against upstream C++ CBC when CBC itself produced a reliable oracle.
	cbcOK, cbcHasInc := cbcOracleReliable(p, resCBC)
	if !cbcOK {
		return
	}

	// 1. Both proven optimal: objectives must agree within relative tolerance.
	if resGo.Status == gb.StatusOptimal && resCBC.Status == gb.StatusOptimal {
		tol := 1e-5 * max(1.0, max(math.Abs(resGo.Objective), math.Abs(resCBC.Objective)))
		if math.Abs(resGo.Objective-resCBC.Objective) > tol {
			t.Fatalf("[%s] optimal objective disagreement: %s=%.8f (nodes=%d) vs cbc=%.8f (nodes=%d) diff=%.2e",
				tag, goName, resGo.Objective, resGo.Nodes, resCBC.Objective, resCBC.Nodes,
				math.Abs(resGo.Objective-resCBC.Objective))
		}
		return
	}

	// 2. One proven optimal, the other hit a limit: cross-check lower bound and incumbent.
	checkOptVsLimited := func(optName string, optRes gb.Result, limName string, limRes gb.Result, limHasInc bool) {
		tol := 1e-5 * max(1.0, math.Abs(optRes.Objective))
		if hasFiniteBound(limRes.BestBound) && limRes.BestBound > optRes.Objective+tol {
			t.Fatalf("[%s] %s BestBound %.8f (status=%v) exceeds %s proven Optimal %.8f",
				tag, limName, limRes.BestBound, limRes.Status, optName, optRes.Objective)
		}
		if limHasInc && limRes.Objective < optRes.Objective-tol {
			t.Fatalf("[%s] %s feasible incumbent %.8f (status=%v) beats %s proven Optimal %.8f",
				tag, limName, limRes.Objective, limRes.Status, optName, optRes.Objective)
		}
	}
	if resGo.Status == gb.StatusOptimal {
		checkOptVsLimited(goName, resGo, "cbc", resCBC, cbcHasInc)
		return
	}
	if resCBC.Status == gb.StatusOptimal {
		checkOptVsLimited("cbc", resCBC, goName, resGo, goHasInc)
		return
	}

	// 3. Both hit time/node limits: neither solver's dual lower bound may exceed the other's feasible incumbent.
	if goHasInc && hasFiniteBound(resCBC.BestBound) {
		tol := 1e-5 * max(1.0, math.Abs(resGo.Objective))
		if resCBC.BestBound > resGo.Objective+tol {
			t.Fatalf("[%s] cbc BestBound %.8f exceeds %s feasible incumbent %.8f", tag, resCBC.BestBound, goName, resGo.Objective)
		}
	}
	if cbcHasInc && hasFiniteBound(resGo.BestBound) {
		tol := 1e-5 * max(1.0, math.Abs(resCBC.Objective))
		if resGo.BestBound > resCBC.Objective+tol {
			t.Fatalf("[%s] %s BestBound %.8f exceeds cbc feasible incumbent %.8f", tag, goName, resGo.BestBound, resCBC.Objective)
		}
	}
}

// FuzzMADDifferentialCBC differential-fuzzes the pure-Go MILP solver (default plus an extra
// randomized option variant) against the C++ COIN-OR CBC backend across unstructured,
// scaled/fractional, near-planted, degenerate, and boundary/extreme-scale MAD instances.
//
// Each Go fuzz worker runs in its own OS process with an isolated C++ CBC heap, so -parallel=N
// scales linearly across N cores without cbcSem contention. Pin away from benchmark cores via taskset:
//
//	taskset -c 26-29 go test -tags cbc -run=^$ -fuzz=^FuzzMADDifferentialCBC$ -fuzztime=5m -parallel=4 .
func FuzzMADDifferentialCBC(f *testing.F) {
	for seed := range uint64(4) {
		for regime := range uint8(5) {
			f.Add(seed, regime)
		}
	}

	f.Fuzz(func(t *testing.T, seed uint64, regime uint8) {
		rng := rand.New(rand.NewPCG(seed, seed^0x517cc1b727220a95))
		tag := fmt.Sprintf("mad_diff_r%d_%d", regime%5, seed)

		mdl := buildDifferentialFuzzMAD(rng, regime, tag)
		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		const (
			timeLimit = 1000 * time.Millisecond
			maxNodes  = 5000
		)

		resCBC := mustSolveCompiled(t, p,
			gb.WithCBC(),
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		if regime%5 != 4 && resCBC.FellBackToGo {
			t.Fatalf("[%s] unexpected pure-Go fallback on normal-scaled MAD model", tag)
		}

		resGo := mustSolveCompiled(t, p,
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		assertMADDifferentialEquivalence(t, tag, "go", p, resGo, resCBC)

		// Also cross-check one extra pure-Go option variant per fuzz case to isolate presolve,
		// cut generation, and primal heuristics independently against CBC.
		var altName string
		var altOpt gb.Option
		switch rng.IntN(3) {
		case 0:
			altName = "go-nopresolve"
			altOpt = gb.WithPresolve(false)
		case 1:
			altName = "go-nocuts"
			altOpt = gb.WithCuts(false)
		default:
			altName = "go-noheur"
			altOpt = gb.WithHeuristics(false)
		}
		resAlt := mustSolveCompiled(t, p,
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
			altOpt,
		)
		assertMADDifferentialEquivalence(t, tag, altName, p, resAlt, resCBC)
	})
}

// FuzzMADMediumDecomposedCBC differential-fuzzes the pure-Go solver against COIN-OR CBC
// on medium-size decomposed time-series MAD models (4..10 groups, 60..2000 items, varying
// sparsity and group-series variance). Each case checks cross-backend objective and bound
// consistency together with the exact feasible plantedObj upper bound.
//
//	taskset -c 26-29 go test -tags cbc -run=^$ -fuzz=^FuzzMADMediumDecomposedCBC$ -fuzztime=5m -parallel=4 .
func FuzzMADMediumDecomposedCBC(f *testing.F) {
	for _, s := range decomposedMADSeeds() {
		f.Add(s.seed, s.gByte, s.sizeRaw, s.regime, s.sparsity)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, regimeByte, sparsityByte uint8) {
		spec := decodeDecomposedMADSpec(seed, gByte, sizeRaw, regimeByte, sparsityByte)
		if raceEnabled && testing.Short() && spec.nI > 250 {
			t.Skipf("skipping large %d-item decomposed MAD CBC seed under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_med_cbc_g%d_i%d_t%d_r%d_s%d_%d",
			spec.nG, spec.nI, spec.nT, spec.regime, spec.sparsity, seed)
		inst := buildDecomposedMediumMAD(spec, tag)

		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 500 * time.Millisecond
		if spec.nI > 400 {
			timeLimit = 1000 * time.Millisecond
		}
		const maxNodes = 1500

		tCBC := time.Now()
		resCBC := mustSolveCompiled(t, p,
			gb.WithCBC(),
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		cbcElapsed := time.Since(tCBC)

		tGo := time.Now()
		resGo := mustSolveCompiled(t, p,
			gb.WithTimeLimit(timeLimit),
			gb.WithMaxNodes(maxNodes),
		)
		goElapsed := time.Since(tGo)

		// Always enforce strict soundness on pure-Go; skip CBC-vs-Go cross-oracle comparison
		// when BackendCBC delegated execution to pure-Go (FellBackToGo) or when upstream C++
		// CBC hit its own extreme-scale numerical tolerance drift (see cbcOracleReliable).
		if resCBC.Backend != gb.BackendCBC {
			t.Fatalf("[%s] expected CBC backend, got %v", tag, resCBC.Backend)
		}
		assertDecomposedMADSoundness(t, tag, "go", p, resGo, inst.plantedObj)
		if resCBC.FellBackToGo {
			verifyMILPInvariants(t, tag+"/go", p, resGo)
			_ = verifyOptionalIncumbent(t, tag+"/go", p, resGo)
			return
		}
		if cbcOK, _ := cbcOracleReliable(p, resCBC); cbcOK {
			assertDecomposedMADSoundness(t, tag, "cbc", p, resCBC, inst.plantedObj)
		}
		assertMADDifferentialEquivalence(t, tag, "go", p, resGo, resCBC)

		t.Logf("[%s] vars=%d rows=%d density=%.2f planted=%.1f | go=%v obj=%.1f bnd=%.1f nd=%d it=%d (%v) | cbc=%v obj=%.1f bnd=%.1f nd=%d it=%d (%v)",
			tag, p.NumVars(), p.NumConstraints(), inst.density, inst.plantedObj,
			resGo.Status, resGo.Objective, resGo.BestBound, resGo.Nodes, resGo.Iterations, goElapsed.Round(time.Millisecond),
			resCBC.Status, resCBC.Objective, resCBC.BestBound, resCBC.Nodes, resCBC.Iterations, cbcElapsed.Round(time.Millisecond))
	})
}

// FuzzMADExactOptimumCBC differential-fuzzes pure-Go against COIN-OR CBC on medium-to-large
// MAD models with analytically exact Root LP and integer MILP optima.
//
//	taskset -c 26-29 go test -tags cbc -run=^$ -fuzz=^FuzzMADExactOptimumCBC$ -fuzztime=30s -parallel=4 .
func FuzzMADExactOptimumCBC(f *testing.F) {
	for _, s := range exactOptMADSeeds() {
		f.Add(s.seed, s.gByte, s.size, s.modeSp, s.base)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, modeSparsityByte, baseByte uint8) {
		spec := decodeExactOptMADSpec(seed, gByte, sizeRaw, modeSparsityByte, baseByte)
		if raceEnabled && testing.Short() && spec.nI > 250 {
			t.Skipf("skipping large %d-item exact-opt MAD CBC seed under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_exopt_cbc_g%d_i%d_t%d_m%d_s%d_%d",
			spec.nG, spec.nI, spec.nT, spec.mode, spec.sparsity, seed)
		inst := buildExactOptimumMAD(spec, tag)
		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 500 * time.Millisecond
		if spec.nI > 350 {
			timeLimit = 900 * time.Millisecond
		}
		const maxNodes = 1500

		resCBC := mustSolveCompiled(t, p, gb.WithCBC(), gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes))
		if resCBC.FellBackToGo {
			t.Fatalf("[%s] unexpected pure-Go fallback on normal-scaled exact-opt MAD model", tag)
		}
		resGo := mustSolveCompiled(t, p, gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes))

		assertExactOptMADSoundness(t, tag, "cbc", p, resCBC, inst.exactLP, inst.exactMILP, false)
		assertExactOptMADSoundness(t, tag, "go", p, resGo, inst.exactLP, inst.exactMILP, false)
		assertMADDifferentialEquivalence(t, tag, "go", p, resGo, resCBC)
	})
}

// FuzzMADAdversarialCBC differential-fuzzes pure-Go against COIN-OR CBC across the five
// adversarial MAD regimes (whales+krill, near-collinear twins, staircase horizons,
// symmetry-orbit clones, and extreme-base cut stress).
//
//	taskset -c 26-29 go test -tags cbc -run=^$ -fuzz=^FuzzMADAdversarialCBC$ -fuzztime=30s -parallel=4 .
func FuzzMADAdversarialCBC(f *testing.F) {
	for _, s := range advMADSeeds() {
		f.Add(s.seed, s.gByte, s.size, s.regime)
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, regimeByte uint8) {
		spec := decodeAdvMADSpec(seed, gByte, sizeRaw, regimeByte)
		if raceEnabled && testing.Short() && spec.nI > 180 {
			t.Skipf("skipping large %d-item adversarial MAD CBC seed under -race -short", spec.nI)
		}

		tag := fmt.Sprintf("mad_adv_cbc_g%d_i%d_t%d_r%d_%d",
			spec.nG, spec.nI, spec.nT, spec.regime, seed)
		inst := buildAdversarialMAD(spec, tag)
		p, err := inst.model.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		timeLimit := 500 * time.Millisecond
		if spec.nI > 250 || spec.nT > 10 {
			timeLimit = 900 * time.Millisecond
		}
		const maxNodes = 1500

		resCBC := mustSolveCompiled(t, p, gb.WithCBC(), gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes))
		resGo := mustSolveCompiled(t, p, gb.WithTimeLimit(timeLimit), gb.WithMaxNodes(maxNodes))

		// Always enforce strict soundness on pure-Go; skip CBC-vs-Go cross-oracle comparison
		// when BackendCBC delegated execution to pure-Go (FellBackToGo) or when upstream C++
		// CBC hit its own extreme-scale numerical tolerance drift (see cbcOracleReliable).
		if resCBC.Backend != gb.BackendCBC {
			t.Fatalf("[%s] expected CBC backend, got %v", tag, resCBC.Backend)
		}
		assertDecomposedMADSoundness(t, tag, "go", p, resGo, inst.plantedObj)
		if resCBC.FellBackToGo {
			verifyMILPInvariants(t, tag+"/go", p, resGo)
			_ = verifyOptionalIncumbent(t, tag+"/go", p, resGo)
			return
		}
		if cbcOK, _ := cbcOracleReliable(p, resCBC); cbcOK {
			assertDecomposedMADSoundness(t, tag, "cbc", p, resCBC, inst.plantedObj)
		}
		assertMADDifferentialEquivalence(t, tag, "go", p, resGo, resCBC)
	})
}

// FuzzMADLimitsCBC exercises early-stop boundaries (short TimeLimit, MaxNodes in {0,1,2},
// MaxIter, and context.WithTimeout) across both BackendCBC and BackendGo to catch
// bound-reporting corruption when solving aborts before branching or root LP completion.
//
//	taskset -c 26-29 go test -tags cbc -run=^$ -fuzz=^FuzzMADLimitsCBC$ -fuzztime=30s -parallel=4 .
func FuzzMADLimitsCBC(f *testing.F) {
	for seed := range uint64(8) {
		f.Add(seed, uint8(seed%5), uint16(8000+seed*6000), uint8(seed%3))
	}

	f.Fuzz(func(t *testing.T, seed uint64, gByte uint8, sizeRaw uint16, limitMode uint8) {
		rng := rand.New(rand.NewPCG(seed^0x510e527fade682d1, seed^0x9b05688c2b3e6c1f))
		nI := logUniformItems(sizeRaw, 40, 600)
		if raceEnabled && testing.Short() && nI > 180 {
			t.Skipf("skipping %d-item limits CBC seed under -race -short", nI)
		}
		nG := 3 + int(gByte%5)
		periods := []int{4, 5, 6, 8}
		nT := periods[rng.IntN(len(periods))]

		var p *gb.CompiledProblem
		var plantedObj float64
		var exactLP float64
		var isExact bool
		tag := fmt.Sprintf("mad_lim_cbc_m%d_g%d_i%d_t%d_%d", limitMode%3, nG, nI, nT, seed)

		if rng.IntN(2) == 0 {
			spec := exactOptMADSpec{
				seed:     seed,
				nG:       nG,
				nI:       nI,
				nT:       nT,
				mode:     uint8(nT % 2),
				sparsity: uint8(rng.IntN(4)),
				baseIdx:  2 + rng.IntN(4),
			}
			inst := buildExactOptimumMAD(spec, tag)
			var err error
			p, err = inst.model.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
			}
			plantedObj = inst.exactMILP
			exactLP = inst.exactLP
			isExact = true
		} else {
			spec := decomposedMADSpec{
				seed:     seed,
				nG:       max(decomposedMADMinGroups, nG),
				nI:       max(decomposedMADMinItems, nI),
				nT:       nT,
				regime:   uint8(rng.IntN(4)),
				sparsity: uint8(rng.IntN(4)),
			}
			inst := buildDecomposedMediumMAD(spec, tag)
			var err error
			p, err = inst.model.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
			}
			plantedObj = inst.plantedObj
		}

		var limitOpt gb.Option
		switch limitMode % 3 {
		case 0:
			limitOpt = gb.WithTimeLimit(time.Duration(1+rng.IntN(30)) * time.Millisecond)
		case 1:
			limitOpt = gb.WithMaxNodes([]int{0, 1, 2}[rng.IntN(3)])
		default:
			limitOpt = gb.WithMaxIter([]int{10, 40, 120, 250}[rng.IntN(4)])
		}

		resCBC := mustSolveCompiled(t, p, gb.WithCBC(), limitOpt, gb.WithTimeLimit(400*time.Millisecond))
		resGo := mustSolveCompiled(t, p, limitOpt, gb.WithTimeLimit(400*time.Millisecond))

		if isExact {
			assertExactOptMADSoundness(t, tag, "cbc", p, resCBC, exactLP, plantedObj, false)
			assertExactOptMADSoundness(t, tag, "go", p, resGo, exactLP, plantedObj, false)
		} else {
			assertDecomposedMADSoundness(t, tag, "cbc", p, resCBC, plantedObj)
			assertDecomposedMADSoundness(t, tag, "go", p, resGo, plantedObj)
		}
		assertMADDifferentialEquivalence(t, tag, "go", p, resGo, resCBC)

		// Also exercise short context deadline cancellation against CBC.
		ctx, cancel := context.WithTimeout(t.Context(), time.Duration(2+rng.IntN(20))*time.Millisecond)
		defer cancel()
		rCtx, ctxErr := gb.SolveCompiled(ctx, p, gb.WithCBC())
		if ctxErr == nil {
			assertDecomposedMADSoundness(t, tag, "cbc-ctx", p, rCtx, plantedObj)
		} else if !errors.Is(ctxErr, context.DeadlineExceeded) {
			t.Fatalf("[%s] unexpected CBC context error: %v", tag, ctxErr)
		}
	})
}
