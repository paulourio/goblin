package goblin_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"math/big"
	"strconv"
	"sync"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

type babFuzzCursor struct {
	data []byte
	pos  int
}

func newBABFuzzCursor(data []byte) *babFuzzCursor {
	return &babFuzzCursor{data: data}
}

func (c *babFuzzCursor) u8() byte {
	if len(c.data) == 0 {
		return 0
	}
	b := c.data[c.pos%len(c.data)]
	c.pos++
	return b
}

func (c *babFuzzCursor) u64() uint64 {
	var v uint64
	for i := range 8 {
		v |= uint64(c.u8()) << (8 * i)
	}
	return v
}

func (c *babFuzzCursor) intRange(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + int(c.u64()%uint64(hi-lo+1))
}

func (c *babFuzzCursor) pickFloat(choices []float64) float64 {
	if len(choices) == 0 {
		return 0
	}
	return choices[int(c.u64()%uint64(len(choices)))]
}

func (c *babFuzzCursor) pickInt(choices []int) int {
	if len(choices) == 0 {
		return 0
	}
	return choices[int(c.u64()%uint64(len(choices)))]
}

var parallelRelGaps = []float64{
	0, 1e-12, 1e-9, 1e-6, 1e-4, 0.25, 0.5, 1.0, 2.5, math.Inf(1),
}

var parallelAbsGaps = []float64{
	0, 1e-9, 1e-6, 0.5,
}

func buildParallelBABModel(cur *babFuzzCursor, forRace bool, relTol, absTol float64) (*gb.CompiledProblem, bool) {
	family := cur.u8() % 4
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("par_bab_sm", sense)

	switch family {
	case 0:
		// Branch-heavy multi-dimensional binary knapsack/covering model (8..36 int vars).
		maxInt := 36
		if forRace {
			maxInt = 18
		}
		nInt := cur.intRange(8, maxInt)
		xs := make([]gb.Var, nInt)
		for j := range nInt {
			if j%5 == 0 && cur.u8()%2 == 0 {
				xs[j] = m.AddIntVar("x"+strconv.Itoa(j), 0, 2)
			} else {
				xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
			}
		}
		// Add odd-parity half-sum constraints forcing fractional root LP and rapid branching.
		nRows := cur.intRange(3, 6)
		for r := range nRows {
			terms := make([]gb.Term, 0, nInt)
			sumW := 0.0
			for j := range nInt {
				if (j+r)%4 == 0 && r > 1 {
					continue
				}
				w := float64(cur.intRange(2, 9))
				terms = append(terms, gb.T(xs[j], w))
				sumW += w
			}
			if len(terms) == 0 {
				terms = append(terms, gb.T(xs[0], 2))
				sumW = 2
			}
			rhs := math.Floor(0.5*sumW) + 0.5*float64(cur.intRange(0, 1))
			if r%2 == 0 {
				m.AddConstraint("knap_"+strconv.Itoa(r), gb.Le(gb.NewExpr(terms...), rhs))
			} else {
				m.AddConstraint("cov_"+strconv.Itoa(r), gb.Ge(gb.NewExpr(terms...), math.Max(1, math.Floor(0.35*sumW))))
			}
		}
		objTerms := make([]gb.Term, nInt)
		for j := range nInt {
			c := float64(cur.intRange(-15, 15))
			if c == 0 {
				c = -3
			}
			objTerms[j] = gb.T(xs[j], c)
		}
		objExpr := gb.NewExpr(objTerms...)
		if cur.u8()%2 == 0 {
			objExpr = objExpr.AddConstant(cur.pickFloat([]float64{-1e8, 1e8, 1e5, -250}))
		}
		m.SetObjective(objExpr)
		return m.Compile(), true

	case 1:
		// Planted gap-boundary optima: integer levels spaced by k*tol above LP bound.
		nCore := cur.intRange(8, 14)
		xs := make([]gb.Var, nCore)
		sumTerms := make([]gb.Term, nCore)
		for j := range nCore {
			xs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
			sumTerms[j] = gb.T(xs[j], 2)
		}
		// Odd RHS on even coefficients splits LP=floor(nCore/2)+0.5 while forcing branching.
		target := 2*cur.intRange(2, nCore-2) + 1
		m.AddConstraint("branch_seed", gb.Ge(gb.NewExpr(sumTerms...), float64(target)))
		m.AddConstraint("branch_cap", gb.Le(gb.NewExpr(sumTerms...), float64(target+3)))

		sel := m.AddIntVar("lvl", 0, 3)
		gapStep := 1e-4
		if relTol > 0 && !math.IsInf(relTol, 1) {
			gapStep = max(gapStep, relTol)
		}
		if absTol > 0 && !math.IsInf(absTol, 1) {
			gapStep = max(gapStep, absTol)
		}
		k := cur.pickFloat([]float64{0.5, 1.0, 1.0 + 1e-6, 2.0})
		stepCoeff := k * gapStep * 100.0
		if sense == gb.Maximize {
			stepCoeff = -stepCoeff
		}
		objTerms := make([]gb.Term, 0, nCore+1)
		for j := range nCore {
			c := 10.0 + float64(j%3)
			if sense == gb.Maximize {
				c = -c
			}
			objTerms = append(objTerms, gb.T(xs[j], c))
		}
		objTerms = append(objTerms, gb.T(sel, stepCoeff))
		objExpr := gb.NewExpr(objTerms...)
		if cur.u8()%2 == 0 {
			// Cancelling ObjConstant around planted optimum ~ 100.
			objExpr = objExpr.AddConstant(cur.pickFloat([]float64{-1e9, 1e9, -100}))
		}
		m.SetObjective(objExpr)
		return m.Compile(), false

	case 2:
		// Worker in-tree cut regime (>= 120 vars with VUB + knapsack rows).
		nCore := cur.intRange(10, 16)
		nTotal := 122
		if forRace {
			// Smaller model under tight node cap when racing.
			nTotal = 20
		}
		nPairs := (nTotal - nCore) / 2
		core := make([]gb.Var, nCore)
		coreTerms := make([]gb.Term, nCore)
		coreSum := 0.0
		for j := range nCore {
			core[j] = m.AddBinaryVar("k" + strconv.Itoa(j))
			w := float64(cur.intRange(3, 8))
			coreTerms[j] = gb.T(core[j], w)
			coreSum += w
		}
		m.AddConstraint("core_knap", gb.Le(gb.NewExpr(coreTerms...), math.Floor(0.5*coreSum)+0.5))
		m.AddConstraint("core_cov", gb.Ge(gb.NewExpr(coreTerms...), math.Floor(0.25*coreSum)))

		objTerms := make([]gb.Term, 0, nTotal)
		for j := range nCore {
			c := float64(cur.intRange(4, 15))
			if sense == gb.Minimize {
				c = -c
			}
			objTerms = append(objTerms, gb.T(core[j], c))
		}
		for p := range nPairs {
			bp := m.AddBinaryVar("vub_b_" + strconv.Itoa(p))
			cp := m.AddContVar("vub_c_"+strconv.Itoa(p), 0, 5)
			capVal := 2.5 + float64(p%3)
			m.AddConstraint("vub_"+strconv.Itoa(p), gb.Le(gb.NewExpr(gb.T(cp, 1), gb.T(bp, -capVal)), 0))
			if p < 8 {
				m.AddConstraint("link_"+strconv.Itoa(p), gb.Le(gb.NewExpr(gb.T(bp, 1), gb.T(core[p%nCore], -1)), 0))
			}
			cCont := 1.0
			cBin := -0.5
			if sense == gb.Minimize {
				cCont = -1.0
				cBin = 0.5
			}
			objTerms = append(objTerms, gb.T(cp, cCont), gb.T(bp, cBin))
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		return m.Compile(), nTotal >= 120

	default:
		// Cancelling ObjConstant and mixed continuous/integer linking across 10..28 int vars.
		maxInt := 28
		if forRace {
			maxInt = 14
		}
		nInt := cur.intRange(10, maxInt)
		xs := make([]gb.Var, nInt)
		for j := range nInt {
			if cur.u8()%3 == 0 {
				xs[j] = m.AddIntVar("i"+strconv.Itoa(j), 0, 3)
			} else {
				xs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
			}
		}
		y0 := m.AddContVar("y0", 0, 10)
		y1 := m.AddContVar("y1", 0, 10)
		for r := range 4 {
			terms := make([]gb.Term, 0, nInt+2)
			for j := range nInt {
				if (j+r)%3 != 0 {
					terms = append(terms, gb.T(xs[j], float64(cur.intRange(1, 5))))
				}
			}
			terms = append(terms, gb.T(y0, float64(r%2+1)), gb.T(y1, -1))
			rhs := float64(nInt + cur.intRange(1, 6))
			if r%2 == 0 {
				m.AddConstraint("mix_le_"+strconv.Itoa(r), gb.Le(gb.NewExpr(terms...), rhs))
			} else {
				m.AddConstraint("mix_ge_"+strconv.Itoa(r), gb.Ge(gb.NewExpr(terms...), math.Floor(0.3*rhs)))
			}
		}
		objTerms := make([]gb.Term, 0, nInt+2)
		linBias := 0.0
		for j := range nInt {
			c := float64(cur.intRange(-8, 8))
			if c == 0 {
				c = 2
			}
			objTerms = append(objTerms, gb.T(xs[j], c))
			linBias += c
		}
		objTerms = append(objTerms, gb.T(y0, 1.5), gb.T(y1, 2.0))
		objExpr := gb.NewExpr(objTerms...)
		cancelBase := -0.5 * linBias
		if sense == gb.Maximize {
			cancelBase = -cancelBase
		}
		objExpr = objExpr.AddConstant(cancelBase + cur.pickFloat([]float64{-1e9, 1e9, 0}))
		m.SetObjective(objExpr)
		return m.Compile(), true
	}
}

func checkBoundConsistencyVsRef(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	refObjUser float64,
) {
	tb.Helper()
	if math.IsInf(res.BestBound, 0) || math.IsNaN(res.BestBound) || math.IsNaN(refObjUser) || math.IsInf(refObjUser, 0) {
		return
	}
	tol := 1e-5 * max(1.0, math.Max(math.Abs(res.BestBound), math.Abs(refObjUser)))
	if p.Sense() == gb.Maximize {
		if res.BestBound < refObjUser-tol {
			tb.Fatalf("[%s] Maximize BestBound %.12g fell below reference objective %.12g (tol=%g)",
				label, res.BestBound, refObjUser, tol)
		}
	} else if res.BestBound > refObjUser+tol {
		tb.Fatalf("[%s] Minimize BestBound %.12g exceeded reference objective %.12g (tol=%g)",
			label, res.BestBound, refObjUser, tol)
	}
}

func runParallelBABCase(tb testing.TB, data []byte, forRace bool) {
	tb.Helper()
	cur := newBABFuzzCursor(data)
	relTol := cur.pickFloat(parallelRelGaps)
	absTol := cur.pickFloat(parallelAbsGaps)
	p, largeHard := buildParallelBABModel(cur, forRace, relTol, absTol)

	workerPool := []int{2, 4, 8}
	if forRace {
		workerPool = []int{2, 4}
	}
	workers := cur.pickInt(workerPool)
	cutPasses := cur.pickInt([]int{-1, 0, 1})

	opts := []gb.Option{
		gb.WithPureGo(),
		gb.WithWorkers(workers),
		gb.WithRelGapTol(relTol),
		gb.WithAbsGapTol(absTol),
		gb.WithPresolve(cur.u8()%2 == 0),
		gb.WithCuts(cur.u8()%2 == 0),
		gb.WithHeuristics(cur.u8()%2 == 0),
		gb.WithSIMD(cur.u8()%2 == 0),
	}
	if cutPasses >= 0 {
		opts = append(opts, gb.WithMaxCutPasses(cutPasses))
	}

	regime := cur.u8() % 4
	if forRace || largeHard {
		// Bound exponential runtime on large branch-heavy problems.
		if regime == 0 {
			regime = 1
		}
	}
	hasLimits := false
	switch regime {
	case 0:
		// Unlimited run.
	case 1:
		hasLimits = true
		maxNodes := cur.intRange(4, 128)
		if forRace {
			maxNodes = cur.intRange(4, 48)
		}
		opts = append(opts, gb.WithMaxNodes(maxNodes), gb.WithMaxIter(cur.intRange(64, 1500)))
	case 2:
		hasLimits = true
		opts = append(opts, gb.WithTimeLimit(time.Duration(cur.intRange(1, 5))*time.Millisecond))
	case 3:
		hasLimits = true
		opts = append(opts, gb.WithMaxNodes(cur.intRange(8, 256)), gb.WithMaxIter(cur.intRange(50, 1200)))
	}

	// Context cancellation path on regime 3 (subset of fuzz inputs).
	useCancel := regime == 3 && cur.u8()%2 == 0
	if useCancel {
		ctx, cancel := context.WithCancel(tb.Context())
		delay := time.Duration(cur.intRange(100, 1500)) * time.Microsecond
		timer := time.AfterFunc(delay, cancel)
		res, err := gb.SolveCompiled(ctx, p, opts...)
		timer.Stop()
		cancel()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				tb.Fatalf("parallel B&B cancel returned unexpected error: %v", err)
			}
			return
		}
		assertAllowedStatus(tb, "par_bab_cancel_done", p, true, res)
		if !isResourceLimitStatus(res.Status) {
			if errChk := checkMILPInvariantsErr("par_bab_cancel_done", p, res); errChk != nil {
				tb.Fatalf("%v", errChk)
			}
		}
		return
	}

	res, err := gb.SolveCompiled(tb.Context(), p, opts...)
	if err != nil {
		tb.Fatalf("SolveCompiled parallel B&B failed: %v", err)
	}
	assertAllowedStatus(tb, "par_bab", p, hasLimits, res)
	if res.Status == gb.StatusOptimal || res.Status == gb.StatusInfeasible || res.Status == gb.StatusUnbounded {
		if errChk := checkMILPInvariantsErr("par_bab", p, res); errChk != nil {
			tb.Fatalf("%v", errChk)
		}
	}

	if !forRace && p.NumVars() <= 16 && res.Status == gb.StatusOptimal {
		// Cross-check BestBound soundness against tight single-worker reference when fast.
		refOpts := []gb.Option{
			gb.WithPureGo(),
			gb.WithWorkers(1),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
			gb.WithMaxNodes(512),
			gb.WithTimeLimit(15 * time.Millisecond),
		}
		resRef, errRef := gb.SolveCompiled(tb.Context(), p, refOpts...)
		if errRef == nil && resRef.Status == gb.StatusOptimal {
			checkBoundConsistencyVsRef(tb, "par_vs_seq_ref", p, res, resRef.Objective)
			checkBoundConsistencyVsRef(tb, "seq_vs_par_ref", p, resRef, res.Objective)
		}
	}
}

// FuzzParallelBABStateMachine exercises exploreTreeParallel across Workers {2,4,8},
// wide RelGapTol/AbsGapTol sweeps, planted gap-boundary optima, cancelling ObjConstant,
// worker in-tree cuts, resource limits, and asynchronous context cancellation.
func FuzzParallelBABStateMachine(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		{2, 0, 4, 1, 8, 3, 2, 7, 5, 9, 1, 0},
		{3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		runParallelBABCase(t, data, false)
	})
}

// FuzzParallelBABRace runs the parallel Branch-and-Bound state-machine fuzz harness
// under race-suited worker counts (2 or 4) and compact node budgets.
func FuzzParallelBABRace(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 3, 5, 7, 9, 11, 13, 15, 17, 19},
		{2, 4, 6, 8, 10, 12, 14, 16, 18, 20},
		{3, 1, 4, 1, 5, 9, 2, 6, 5, 3},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		runParallelBABCase(t, data, true)
	})
}

func buildSmallExactReferenceMIP(cur *babFuzzCursor) *gb.CompiledProblem {
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("exact_ref_mip", sense)
	nInt := cur.intRange(2, 6)
	nCont := cur.intRange(0, 2)
	vars := make([]gb.Var, 0, nInt+nCont)
	for j := range nInt {
		if cur.u8()%2 == 0 {
			vars = append(vars, m.AddBinaryVar("b"+strconv.Itoa(j)))
		} else {
			ub := float64(cur.intRange(1, 2))
			vars = append(vars, m.AddIntVar("i"+strconv.Itoa(j), 0, ub))
		}
	}
	for k := range nCont {
		vars = append(vars, m.AddContVar("c"+strconv.Itoa(k), 0, float64(cur.intRange(2, 6))))
	}
	nTot := len(vars)
	mRows := cur.intRange(1, 4)
	for r := range mRows {
		terms := make([]gb.Term, 0, nTot)
		for j := range nTot {
			c := float64(cur.intRange(-4, 5))
			if c != 0 {
				terms = append(terms, gb.T(vars[j], c))
			}
		}
		if len(terms) == 0 {
			terms = append(terms, gb.T(vars[0], 1))
		}
		rhs := float64(cur.intRange(-2, 7))
		if cur.u8()%4 == 0 {
			rhs += 0.5
		}
		switch cur.u8() % 3 {
		case 0:
			m.AddConstraint("r"+strconv.Itoa(r), gb.Le(gb.NewExpr(terms...), rhs))
		case 1:
			m.AddConstraint("r"+strconv.Itoa(r), gb.Ge(gb.NewExpr(terms...), rhs))
		default:
			m.AddConstraint("r"+strconv.Itoa(r), gb.Eq(gb.NewExpr(terms...), rhs))
		}
	}
	objTerms := make([]gb.Term, 0, nTot)
	for j := range nTot {
		c := float64(cur.intRange(-6, 6))
		if c != 0 {
			objTerms = append(objTerms, gb.T(vars[j], c))
		}
	}
	objExpr := gb.NewExpr(objTerms...)
	if cur.u8()%3 == 0 {
		objExpr = objExpr.AddConstant(float64(cur.pickInt([]int{-1000, -15, 0, 25, 1000})))
	}
	m.SetObjective(objExpr)
	return m.Compile()
}

func checkSolutionAgainstExactOptMin(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	optMin *big.Rat,
	feas bool,
	relTol, absTol float64,
) {
	tb.Helper()
	if !feas {
		if res.Status != gb.StatusInfeasible {
			tb.Fatalf("[%s] solver status %s, exact reference is Infeasible", label, res.Status)
		}
		return
	}
	if res.Status != gb.StatusOptimal {
		tb.Fatalf("[%s] solver status %s, exact reference is Optimal", label, res.Status)
	}
	if err := checkMILPInvariantsErr(label, p, res); err != nil {
		tb.Fatalf("%v", err)
	}
	objConst, objMin, _, _, _, _, okProb := gb.ExactProblemRatsForTest(p)
	if !okProb {
		tb.Fatalf("[%s] ExactProblemRatsForTest failed", label)
	}
	valRats := make([]*big.Rat, p.NumVars())
	for j, v := range res.Values {
		r := new(big.Rat).SetFloat64(v)
		if r == nil {
			tb.Fatalf("[%s] non-finite Values[%d]=%g", label, j, v)
		}
		valRats[j] = r
	}
	candMin := exactEvalProblemObjRat(objConst, objMin, valRats)
	exactUserOpt := userBoundFromMinRat(p, optMin)
	checkBoundConsistencyVsRef(tb, label, p, res, exactUserOpt)

	// Verify candMin is within allowed optimality gap relative to exact optMin.
	candF, _ := candMin.Float64()
	optF, _ := optMin.Float64()
	cMinF, _ := objConst.Float64()
	linScale := math.Max(1.0, math.Abs(optF-cMinF))
	allowed := math.Max(1e-5*linScale, math.Max(absTol, relTol*linScale))
	if candF-optF > allowed {
		tb.Fatalf("[%s] suboptimal objective min=%s vs exact opt=%s (allowed=%.6g rel=%g abs=%g)",
			label, candMin.RatString(), optMin.RatString(), allowed, relTol, absTol)
	}
}

// FuzzOptionMatrixAndExactReference cross-checks option combinations across
// Presolve, Cuts, Heuristics, Workers {1,2,4}, SIMD on/off, MaxCutPasses {0,1,50},
// Certified, and CertifiedWitness against exactSmallMIPObjectiveRat and
// verifyCutsAndPresolveAgainstAllIntPoints.
func FuzzOptionMatrixAndExactReference(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 4, 2, 8, 5, 7, 1, 4, 2, 8},
		{2, 2, 5, 3, 1, 9, 4, 6, 0, 7},
		{3, 7, 1, 6, 4, 2, 8, 5, 9, 0},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := newBABFuzzCursor(data)
		p := buildSmallExactReferenceMIP(cur)
		optMin, feas, okExact := exactSmallMIPObjectiveRat(t.Context(), p, 1024)
		verifyCutsAndPresolveAgainstAllIntPoints(t, "opt_matrix", p, 512)

		cutPassChoices := []int{0, 1, 50}
		workerChoices := []int{1, 2, 4}
		relTol := cur.pickFloat([]float64{0, 1e-6})
		absTol := cur.pickFloat([]float64{0, 1e-6})

		const matrixConfigs = 3
		for cfgIdx := range matrixConfigs {
			usePresolve := cur.u8()%2 == 0
			useCuts := cur.u8()%2 == 0
			useHeur := cur.u8()%2 == 0
			useSIMD := cur.u8()%2 == 0
			certified := cfgIdx == 0 || cur.u8()%2 == 0
			workers := cur.pickInt(workerChoices)
			if certified {
				// Certified mode requires single-worker tracing.
				workers = 1
			}
			cutPasses := cur.pickInt(cutPassChoices)

			opts := []gb.Option{
				gb.WithPureGo(),
				gb.WithPresolve(usePresolve),
				gb.WithCuts(useCuts),
				gb.WithHeuristics(useHeur),
				gb.WithWorkers(workers),
				gb.WithSIMD(useSIMD),
				gb.WithMaxCutPasses(cutPasses),
				gb.WithRelGapTol(relTol),
				gb.WithAbsGapTol(absTol),
			}
			var tr *gb.VIPRTrace
			var wit gb.CertifiedWitness
			if certified {
				tr = gb.NewVIPRTrace()
				opts = append(opts, gb.WithCertified(true), gb.WithVIPRTrace(tr), gb.WithCertifiedWitness(&wit))
			}
			applied := gb.ApplyOptions(opts...)
			res, err := gb.SolveCompiled(t.Context(), p, opts...)
			if err != nil {
				t.Fatalf("config #%d SolveCompiled failed (cert=%v workers=%d simd=%v): %v",
					cfgIdx, certified, workers, useSIMD, err)
			}
			label := "matrix_" + strconv.Itoa(cfgIdx)
			assertAllowedStatus(t, label, p, false, res)
			if okExact && res.Status != gb.StatusUnbounded {
				checkSolutionAgainstExactOptMin(t, label, p, res, optMin, feas, applied.RelGapTol, applied.AbsGapTol)
			} else if errChk := checkMILPInvariantsErr(label, p, res); errChk != nil {
				t.Fatalf("%v", errChk)
			}
			if certified {
				if wit.Status != res.Status {
					t.Fatalf("[%s] witness status %s != result %s", label, wit.Status, res.Status)
				}
				startV := time.Now()
				if errV := gb.VerifyVIPRTrace(p, tr, res, applied); errV != nil {
					t.Fatalf("[%s] VerifyVIPRTrace failed: %v", label, errV)
				}
				assertVIPRComplexityBudget(t, label, p, tr, time.Since(startV))
				if formalFuzzEnabled() && tr.HasCertificate() {
					if errF := checkFormalVIPRResult(t, label, p, res, tr, applied.RelGapTol, applied.AbsGapTol); errF != nil {
						t.Fatal(errF)
					}
				}
			}
		}
	})
}

// TestCompiledProblemConcurrentSolveAndExport runs 8 concurrent goroutines against
// one shared CompiledProblem performing mixed Pure-Go, Certified, parallel Workers=2
// solves alongside format.WriteMPS and format.ToPuLPJSONCompact exports.
func TestCompiledProblemConcurrentSolveAndExport(t *testing.T) {
	m := gb.NewModel("concurrent_shared", gb.Minimize)
	bins := make([]gb.Var, 10)
	capTerms := make([]gb.Term, 0, 12)
	covTerms := make([]gb.Term, 0, 10)
	objTerms := make([]gb.Term, 0, 12)
	for j := range bins {
		bins[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
		capTerms = append(capTerms, gb.T(bins[j], float64(2+(j%3))))
		covTerms = append(covTerms, gb.T(bins[j], 2))
		objTerms = append(objTerms, gb.T(bins[j], float64(4+(j%5))))
	}
	xInt := m.AddIntVar("x_int", 0, 4)
	c0 := m.AddContVar("c0", 0, 5)
	capTerms = append(capTerms, gb.T(xInt, 2), gb.T(c0, 1))
	objTerms = append(objTerms, gb.T(xInt, 3), gb.T(c0, -1))
	m.AddConstraint("cap", gb.Le(gb.NewExpr(capTerms...), 19.5))
	m.AddConstraint("cov", gb.Ge(gb.NewExpr(covTerms...), 9))
	m.AddConstraint("link", gb.Le(gb.NewExpr(gb.T(c0, 1), gb.T(bins[0], -3)), 1))
	m.SetObjective(gb.NewExpr(objTerms...).AddConstant(10))
	p := m.Compile()

	var baseMPS bytes.Buffer
	if err := format.WriteMPS(p, &baseMPS); err != nil {
		t.Fatalf("baseline WriteMPS failed: %v", err)
	}
	var basePuLP bytes.Buffer
	if err := format.ToPuLPJSONCompact(p, &basePuLP); err != nil {
		t.Fatalf("baseline ToPuLPJSONCompact failed: %v", err)
	}
	refRes, errRef := gb.SolveCompiled(t.Context(), p, gb.WithPureGo(), gb.WithCertified(true))
	if errRef != nil || refRes.Status != gb.StatusOptimal {
		t.Fatalf("baseline SolveCompiled failed: status=%s err=%v", refRes.Status, errRef)
	}

	const numGoroutines = 8
	const itersPerGoroutine = 6
	start := make(chan struct{})
	errCh := make(chan error, numGoroutines)
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			<-start
			for it := range itersPerGoroutine {
				switch (id + it) % 5 {
				case 0:
					res, err := gb.SolveCompiled(t.Context(), p, gb.WithPureGo(), gb.WithWorkers(1))
					if err != nil {
						errCh <- err
						return
					}
					if res.Status != gb.StatusOptimal || math.Abs(res.Objective-refRes.Objective) > 1e-9 {
						errCh <- errors.New("pure-go concurrent solve mismatch")
						return
					}
				case 1:
					tr := gb.NewVIPRTrace()
					var wit gb.CertifiedWitness
					res, err := gb.SolveCompiled(t.Context(), p, gb.WithPureGo(), gb.WithCertified(true), gb.WithVIPRTrace(tr), gb.WithCertifiedWitness(&wit))
					if err != nil {
						errCh <- err
						return
					}
					if res.Status != gb.StatusOptimal || wit.Status != gb.StatusOptimal || res.Objective != refRes.Objective {
						errCh <- errors.New("certified concurrent solve mismatch")
						return
					}
					if errV := gb.VerifyVIPRTrace(p, tr, res, gb.ApplyOptions(gb.WithCertified(true))); errV != nil {
						errCh <- errV
						return
					}
				case 2:
					res, err := gb.SolveCompiled(t.Context(), p, gb.WithPureGo(), gb.WithWorkers(2))
					if err != nil {
						errCh <- err
						return
					}
					if res.Status != gb.StatusOptimal || math.Abs(res.Objective-refRes.Objective) > 1e-9 {
						errCh <- errors.New("parallel workers=2 concurrent solve mismatch")
						return
					}
				case 3:
					var mpsBuf bytes.Buffer
					if err := format.WriteMPS(p, &mpsBuf); err != nil {
						errCh <- err
						return
					}
					if !bytes.Equal(mpsBuf.Bytes(), baseMPS.Bytes()) {
						errCh <- errors.New("concurrent WriteMPS diverged from deterministic baseline")
						return
					}
				case 4:
					var pulpBuf bytes.Buffer
					if err := format.ToPuLPJSONCompact(p, &pulpBuf); err != nil {
						errCh <- err
						return
					}
					if !bytes.Equal(pulpBuf.Bytes(), basePuLP.Bytes()) {
						errCh <- errors.New("concurrent ToPuLPJSONCompact diverged from deterministic baseline")
						return
					}
				}
			}
		}(g)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent shared CompiledProblem failure: %v", err)
	}
}
