//go:build cgo && cbc

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

func verifyOptionalIncumbent(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result) bool {
	t.Helper()
	if len(res.Values) != p.NumVars() {
		return false
	}
	bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
	if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
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

func assertMADDifferentialEquivalence(t *testing.T, tag, goName string, p *gb.CompiledProblem, resGo, resCBC gb.Result) {
	t.Helper()

	if resGo.Backend != gb.BackendGo {
		t.Fatalf("[%s/%s] expected pure-Go backend, got %v", tag, goName, resGo.Backend)
	}
	if resCBC.Backend != gb.BackendCBC {
		t.Fatalf("[%s] expected CBC backend, got %v", tag, resCBC.Backend)
	}

	// Every MAD instance built by BuildMADFromWeights is guaranteed feasible and bounded below by 0.
	for _, entry := range []struct {
		name string
		res  gb.Result
	}{
		{goName, resGo},
		{"cbc", resCBC},
	} {
		switch entry.res.Status {
		case gb.StatusInfeasible, gb.StatusUnbounded:
			t.Fatalf("[%s/%s] impossible status %v on guaranteed-feasible MAD model (go=%v obj=%.6f nodes=%d iters=%d; cbc=%v obj=%.6f nodes=%d iters=%d)",
				tag, entry.name, entry.res.Status,
				resGo.Status, resGo.Objective, resGo.Nodes, resGo.Iterations,
				resCBC.Status, resCBC.Objective, resCBC.Nodes, resCBC.Iterations)
		case gb.StatusNumericalError, gb.StatusUndefined:
			t.Fatalf("[%s/%s] solver failure status %v (vars=%d rows=%d; go=%v obj=%.6f bound=%.6f nodes=%d iters=%d; cbc=%v obj=%.6f bound=%.6f nodes=%d iters=%d)",
				tag, entry.name, entry.res.Status, p.NumVars(), p.NumConstraints(),
				resGo.Status, resGo.Objective, resGo.BestBound, resGo.Nodes, resGo.Iterations,
				resCBC.Status, resCBC.Objective, resCBC.BestBound, resCBC.Nodes, resCBC.Iterations)
		}
		verifyMILPInvariants(t, tag+"/"+entry.name, p, entry.res)
	}

	goHasInc := verifyOptionalIncumbent(t, tag+"/"+goName, p, resGo)
	cbcHasInc := verifyOptionalIncumbent(t, tag+"/cbc", p, resCBC)

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
