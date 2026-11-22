package goblin

import (
	"math"
	"math/big"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"time"
)

type parAdvPlantedMIP struct {
	name       string
	sense      Sense
	comp       *CompiledProblem
	exactOpt   *big.Rat
	exactOptF  float64
	witness    []float64
	witnessRat []*big.Rat
}

func parAdvMustRat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

// parAdvBuildPlantedTreeMIP builds a mixed-integer model with nBin binary variables,
// 2 general integer variables in [0, 3], and 2 non-negative continuous epigraph variables.
// Pairwise packing triangles create an exponential integrality gap so parallel workers
// explore a non-trivial search tree while exact enumeration certifies exactOpt in Q.
func parAdvBuildPlantedTreeMIP(tb testing.TB, name string, sense Sense, nTri int, objConst float64, seed uint64) parAdvPlantedMIP {
	tb.Helper()
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	nBin := 3 * nTri
	m := NewModel(name, sense)

	bins := make([]Var, nBin)
	binCost := make([]int64, nBin)
	binW1 := make([]int64, nBin)
	binW2 := make([]int64, nBin)
	for j := range nBin {
		bins[j] = m.AddBinaryVar("b_" + strconv.Itoa(j))
		binCost[j] = int64(3 + rng.IntN(17))
		binW1[j] = int64(2 + rng.IntN(11))
		binW2[j] = int64(1 + rng.IntN(9))
	}

	g0 := m.AddIntVar("g0", 0, 3)
	g1 := m.AddIntVar("g1", 0, 3)
	c0 := m.AddContVar("c0", 0, math.Inf(1))
	c1 := m.AddContVar("c1", 0, 50)

	gCost := [2]int64{int64(4 + rng.IntN(9)), int64(5 + rng.IntN(9))}
	sign := 1.0
	if sense == Maximize {
		sign = -1.0
	}

	// Normalized minimization costs (before sense sign):
	// min sum(-binCost[j]*b[j]) - gCost[0]*g0 - gCost[1]*g1 + 7*c0 + 5*c1
	// Subject to triangle packings, knapsack cap, and epigraph rows on c0, c1.
	objTerms := make([]Term, 0, nBin+4)
	for j := range nBin {
		objTerms = append(objTerms, bins[j].T(sign*float64(-binCost[j])))
	}
	objTerms = append(objTerms,
		g0.T(sign*float64(-gCost[0])),
		g1.T(sign*float64(-gCost[1])),
		c0.T(sign*7.0),
		c1.T(sign*5.0),
	)
	m.SetObjective(Expr{Terms: objTerms, Constant: objConst})

	for g := range nTri {
		a, b, c := bins[3*g], bins[3*g+1], bins[3*g+2]
		m.AddConstraint("tri_ab_"+strconv.Itoa(g), NewExpr(a.T(1), b.T(1)).Le(1))
		m.AddConstraint("tri_bc_"+strconv.Itoa(g), NewExpr(b.T(1), c.T(1)).Le(1))
		m.AddConstraint("tri_ac_"+strconv.Itoa(g), NewExpr(a.T(1), c.T(1)).Le(1))
	}

	var sumW1, sumW2 int64
	knapTerms := make([]Term, 0, nBin+2)
	epi0Terms := make([]Term, 0, nBin+2)
	epi1Terms := make([]Term, 0, nBin+2)
	for j := range nBin {
		sumW1 += binW1[j]
		sumW2 += binW2[j]
		knapTerms = append(knapTerms, bins[j].T(float64(binW1[j])))
		epi0Terms = append(epi0Terms, bins[j].T(float64(binW1[j])))
		epi1Terms = append(epi1Terms, bins[j].T(float64(binW2[j])))
	}
	knapCap := max(int64(8), sumW1/3+4)
	knapTerms = append(knapTerms, g0.T(3), g1.T(4))
	m.AddConstraint("knap_cap", Constraint{Expr: Expr{Terms: knapTerms}, Sense: LessEqual, RHS: float64(knapCap)})

	// Epigraph rows: 2*c0 + sum(w1*b) + 2*g0 >= epi0RHS, and 4*c1 - sum(w2*b) - 3*g1 >= -epi1Shift.
	epi0RHS := max(int64(5), sumW1/4+3)
	epi0Terms = append(epi0Terms, g0.T(2), c0.T(2))
	m.AddConstraint("epi0", Constraint{Expr: Expr{Terms: epi0Terms}, Sense: GreaterEqual, RHS: float64(epi0RHS)})

	epi1Shift := max(int64(3), sumW2/6)
	for i := range epi1Terms {
		epi1Terms[i].Coeff = -epi1Terms[i].Coeff
	}
	epi1Terms = append(epi1Terms, g1.T(-3), c1.T(4))
	m.AddConstraint("epi1", Constraint{Expr: Expr{Terms: epi1Terms}, Sense: GreaterEqual, RHS: float64(-epi1Shift)})

	comp := m.Compile()

	// Enumerate all 4^nTri * 16 feasible integer states (at most 4^4 * 16 = 4096) in exact Q.
	// Each triangle allows 4 valid binary assignments: (0,0,0), (1,0,0), (0,1,0), (0,0,1).
	totalTriStates := 1 << (2 * nTri)
	var bestMinRat *big.Rat
	var bestVecRat []*big.Rat
	nVars := nBin + 4
	bAssign := make([]int64, nBin)

	for st := range totalTriStates {
		var w1Acc, w2Acc, linIntCost int64
		tmp := st
		for g := range nTri {
			sel := tmp & 3
			tmp >>= 2
			bAssign[3*g], bAssign[3*g+1], bAssign[3*g+2] = 0, 0, 0
			if sel > 0 {
				idx := 3*g + (sel - 1)
				bAssign[idx] = 1
				w1Acc += binW1[idx]
				w2Acc += binW2[idx]
				linIntCost -= binCost[idx]
			}
		}
		for v0 := range int64(4) {
			for v1 := range int64(4) {
				if w1Acc+3*v0+4*v1 > knapCap {
					continue
				}
				// Minimum c0 >= max(0, (epi0RHS - w1Acc - 2*v0) / 2).
				numC0 := max(int64(0), epi0RHS-w1Acc-2*v0)
				c0Rat := big.NewRat(numC0, 2)

				// Minimum c1 >= max(0, (w2Acc + 3*v1 - epi1Shift) / 4), must satisfy c1 <= 50.
				numC1 := max(int64(0), w2Acc+3*v1-epi1Shift)
				if numC1 > 200 {
					continue
				}
				c1Rat := big.NewRat(numC1, 4)

				candMin := big.NewRat(linIntCost-gCost[0]*v0-gCost[1]*v1, 1)
				candMin.Add(candMin, new(big.Rat).Mul(big.NewRat(7, 1), c0Rat))
				candMin.Add(candMin, new(big.Rat).Mul(big.NewRat(5, 1), c1Rat))

				if bestMinRat == nil || candMin.Cmp(bestMinRat) < 0 {
					bestMinRat = candMin
					vec := make([]*big.Rat, nVars)
					for j := range nBin {
						vec[j] = big.NewRat(bAssign[j], 1)
					}
					vec[nBin] = big.NewRat(v0, 1)
					vec[nBin+1] = big.NewRat(v1, 1)
					vec[nBin+2] = c0Rat
					vec[nBin+3] = c1Rat
					bestVecRat = vec
				}
			}
		}
	}
	if bestMinRat == nil {
		tb.Fatalf("%s: planted model unexpectedly infeasible", name)
	}

	constRat := parAdvMustRat(tb, objConst)
	exactUserOpt := new(big.Rat).Set(bestMinRat)
	if sense == Maximize {
		exactUserOpt.Neg(exactUserOpt)
	}
	exactUserOpt.Add(exactUserOpt, constRat)
	optF, _ := exactUserOpt.Float64()

	witness := make([]float64, nVars)
	for j, r := range bestVecRat {
		witness[j], _ = r.Float64()
	}
	bV, rV, iV := comp.MaxViolation(witness, true)
	if bV > 1e-12 || rV > 1e-12 || iV > 1e-12 {
		tb.Fatalf("%s: planted witness violation=(%g,%g,%g)", name, bV, rV, iV)
	}

	return parAdvPlantedMIP{
		name:       name,
		sense:      sense,
		comp:       comp,
		exactOpt:   exactUserOpt,
		exactOptF:  optF,
		witness:    witness,
		witnessRat: bestVecRat,
	}
}

func parAdvCheckResultInvariants(tb testing.TB, tag string, pm parAdvPlantedMIP, res Result, requireOptimal bool) {
	tb.Helper()
	if res.Status == StatusInfeasible || res.Status == StatusUnbounded {
		tb.Fatalf("%s: false status=%v on feasible bounded MIP (exactOpt=%v)", tag, res.Status, pm.exactOpt)
	}
	if requireOptimal && res.Status != StatusOptimal {
		tb.Fatalf("%s: status=%v, want StatusOptimal", tag, res.Status)
	}

	boundTol := 1e-5 * max(1.0, math.Abs(pm.exactOptF))
	hasValidBound := res.Status == StatusOptimal || res.Status == StatusNodeLimit ||
		((res.Status == StatusIterationLimit || res.Status == StatusTimeLimit) &&
			(len(res.Values) > 0 || res.Nodes > 1 || res.RootLPBound != 0))

	if hasValidBound {
		if math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0) {
			tb.Fatalf("%s: non-finite BestBound=%v on status=%v", tag, res.BestBound, res.Status)
		}
		if pm.sense == Minimize {
			if res.BestBound > pm.exactOptF+boundTol {
				tb.Fatalf("%s: Minimize BestBound=%.12g exceeds exactOpt=%.12g (tol=%.3e)",
					tag, res.BestBound, pm.exactOptF, boundTol)
			}
		} else {
			if res.BestBound < pm.exactOptF-boundTol {
				tb.Fatalf("%s: Maximize BestBound=%.12g below exactOpt=%.12g (tol=%.3e)",
					tag, res.BestBound, pm.exactOptF, boundTol)
			}
		}
	}

	if len(res.Values) > 0 {
		if len(res.Values) != pm.comp.numVars {
			tb.Fatalf("%s: len(Values)=%d, want %d", tag, len(res.Values), pm.comp.numVars)
		}
		bV, rV, iV := pm.comp.MaxViolation(res.Values, true)
		if bV > incumbentTol || rV > incumbentTol || iV > incumbentTol {
			tb.Fatalf("%s: primal violation bound=%.3e row=%.3e int=%.3e", tag, bV, rV, iV)
		}
		evalObj := pm.comp.EvaluateObjective(res.Values)
		objTol := 1e-5 * max(1.0, math.Abs(evalObj))
		if math.Abs(evalObj-res.Objective) > objTol {
			tb.Fatalf("%s: Objective=%.12g != EvaluateObjective=%.12g", tag, res.Objective, evalObj)
		}
		if pm.sense == Minimize {
			if res.Objective < pm.exactOptF-boundTol {
				tb.Fatalf("%s: Minimize primal Objective=%.12g < exactOpt=%.12g", tag, res.Objective, pm.exactOptF)
			}
			if hasValidBound && res.BestBound > res.Objective+boundTol {
				tb.Fatalf("%s: Minimize BestBound=%.12g > Objective=%.12g", tag, res.BestBound, res.Objective)
			}
		} else {
			if res.Objective > pm.exactOptF+boundTol {
				tb.Fatalf("%s: Maximize primal Objective=%.12g > exactOpt=%.12g", tag, res.Objective, pm.exactOptF)
			}
			if hasValidBound && res.BestBound < res.Objective-boundTol {
				tb.Fatalf("%s: Maximize BestBound=%.12g < Objective=%.12g", tag, res.BestBound, res.Objective)
			}
		}
	}

	if res.Status == StatusOptimal {
		if len(res.Values) != pm.comp.numVars {
			tb.Fatalf("%s: StatusOptimal missing Values", tag)
		}
		if math.Abs(res.Objective-pm.exactOptF) > boundTol {
			tb.Fatalf("%s: StatusOptimal Objective=%.12g != exactOpt=%.12g (tol=%.3e)",
				tag, res.Objective, pm.exactOptF, boundTol)
		}
	}
}

func parAdvVerifyVIPROracle(tb testing.TB, tag string, tr *VIPRTrace, sense Sense, exactOpt *big.Rat) {
	tb.Helper()
	if !tr.HasCertificate() {
		if exactOpt == nil && tr.PresolveInfeasible() {
			return
		}
		tb.Fatalf("%s: expected VIPR certificate, got none (presolveInfeasible=%v)", tag, tr.PresolveInfeasible())
	}
	raw, err := tr.MarshalVIPRJSON()
	if err != nil {
		tb.Fatalf("%s: MarshalVIPRJSON: %v", tag, err)
	}
	if !FormalOracleEnabled() {
		return
	}
	resp, err := FormalCheckVIPRJSONL(tb, raw)
	if err != nil || !resp.OK {
		tb.Fatalf("%s: vipr-check rejected certificate (mode=%s): resp=%+v err=%v", tag, tr.Mode(), resp, err)
	}
	if exactOpt != nil && (tr.Mode() == "optimal" || tr.Mode() == "bound") {
		lbMin := tr.TargetLB()
		if lbMin == nil {
			tb.Fatalf("%s: nil TargetLB on mode=%s", tag, tr.Mode())
		}
		// TargetLB is in shifted minimized form: userLB = lbMin (Minimize) or -lbMin (Maximize).
		if sense == Minimize {
			if lbMin.Cmp(exactOpt) > 0 {
				tb.Fatalf("%s: VIPR TargetLB=%v > Minimize exactOpt=%v", tag, lbMin, exactOpt)
			}
		} else {
			userUB := new(big.Rat).Neg(lbMin)
			if userUB.Cmp(exactOpt) < 0 {
				tb.Fatalf("%s: VIPR -TargetLB=%v < Maximize exactOpt=%v", tag, userUB, exactOpt)
			}
		}
	}
}

func parAdvCheckCutsAgainstWitnesses(tb testing.TB, tag string, cuts []cutPlane, witnesses [][]*big.Rat) {
	tb.Helper()
	for ci, c := range cuts {
		rhs := parAdvMustRat(tb, c.RHS)
		for wi, w := range witnesses {
			lhs := new(big.Rat)
			for k, col := range c.ColIdx {
				vr := parAdvMustRat(tb, c.Val[k])
				lhs.Add(lhs, new(big.Rat).Mul(vr, w[col]))
			}
			if lhs.Cmp(rhs) > 0 {
				tb.Fatalf("%s: cut %d (%+v) cuts off exact feasible witness %d: lhs=%v > rhs=%v",
					tag, ci, c, wi, lhs, rhs)
			}
		}
	}
}

func TestFormalParallelBoundAndOptimumInvariants(t *testing.T) {
	t.Parallel()

	workerCounts := []int{1, 2, 4, 8}

	t.Run("planted_exact_rat_full_and_early_limits", func(t *testing.T) {
		t.Parallel()
		specs := []struct {
			name     string
			sense    Sense
			nTri     int
			objConst float64
			seed     uint64
		}{
			{"min_tri4_const_pos", Minimize, 4, 125.5, 101},
			{"max_tri4_const_neg", Maximize, 4, -42.25, 202},
			{"min_tri3_zero_const", Minimize, 3, 0, 303},
			{"max_tri3_const_pos", Maximize, 3, 88.75, 404},
		}

		nodeBudgets := []int{1, 2, 3, 5, 8, 12, 15, 22}
		iterBudgets := []int{4, 12, 30, 80}

		for _, sp := range specs {
			pm := parAdvBuildPlantedTreeMIP(t, sp.name, sp.sense, sp.nTri, sp.objConst, sp.seed)

			for _, w := range workerCounts {
				tagFull := sp.name + "_full_w" + strconv.Itoa(w)
				resFull, err := SolveCompiled(t.Context(), pm.comp,
					WithPresolve(true), WithCuts(true), WithHeuristics(true), WithWorkers(w),
				)
				if err != nil {
					t.Fatalf("%s: SolveCompiled err: %v", tagFull, err)
				}
				parAdvCheckResultInvariants(t, tagFull, pm, resFull, true)

				for _, maxN := range nodeBudgets {
					tagN := sp.name + "_nodes" + strconv.Itoa(maxN) + "_w" + strconv.Itoa(w)
					resN, errN := SolveCompiled(t.Context(), pm.comp,
						WithPresolve(true), WithCuts(true), WithHeuristics(true),
						WithWorkers(w), WithMaxNodes(maxN),
					)
					if errN != nil {
						t.Fatalf("%s: err: %v", tagN, errN)
					}
					if resN.Nodes > maxN {
						t.Fatalf("%s: Nodes=%d exceeds MaxNodes(%d)", tagN, resN.Nodes, maxN)
					}
					if resN.Status != StatusOptimal && resN.Status != StatusNodeLimit {
						t.Fatalf("%s: unexpected status=%v", tagN, resN.Status)
					}
					parAdvCheckResultInvariants(t, tagN, pm, resN, false)
				}

				for _, maxIt := range iterBudgets {
					tagIt := sp.name + "_iter" + strconv.Itoa(maxIt) + "_w" + strconv.Itoa(w)
					resIt, errIt := SolveCompiled(t.Context(), pm.comp,
						WithPresolve(true), WithCuts(true), WithHeuristics(true),
						WithWorkers(w), WithMaxIter(maxIt),
					)
					if errIt != nil {
						t.Fatalf("%s: err: %v", tagIt, errIt)
					}
					if resIt.Status != StatusOptimal && resIt.Status != StatusIterationLimit {
						t.Fatalf("%s: unexpected status=%v", tagIt, resIt.Status)
					}
					parAdvCheckResultInvariants(t, tagIt, pm, resIt, false)
				}
			}
		}
	})

	t.Run("seeded_planted_adversarial_sweep", func(t *testing.T) {
		t.Parallel()
		for seed := range uint64(8) {
			sense := Minimize
			if seed%2 == 1 {
				sense = Maximize
			}
			objConst := float64(int64(seed)*17 - 50)
			pm := parAdvBuildPlantedTreeMIP(t, "seed_"+strconv.FormatUint(seed, 10), sense, 3, objConst, 1000+seed*37)

			for _, w := range workerCounts {
				res, err := SolveCompiled(t.Context(), pm.comp,
					WithPresolve(true), WithCuts(true), WithHeuristics(true), WithWorkers(w),
				)
				if err != nil {
					t.Fatalf("seed=%d w=%d full err: %v", seed, w, err)
				}
				parAdvCheckResultInvariants(t, "seed_full_w"+strconv.Itoa(w), pm, res, true)

				resLim, errLim := SolveCompiled(t.Context(), pm.comp,
					WithPresolve(true), WithCuts(true), WithHeuristics(true),
					WithWorkers(w), WithMaxNodes(6),
				)
				if errLim != nil {
					t.Fatalf("seed=%d w=%d maxNodes(6) err: %v", seed, w, errLim)
				}
				if resLim.Nodes > 6 {
					t.Fatalf("seed=%d w=%d Nodes=%d > 6", seed, w, resLim.Nodes)
				}
				parAdvCheckResultInvariants(t, "seed_lim_w"+strconv.Itoa(w), pm, resLim, false)
			}

			tr := NewVIPRTrace()
			resVIPR, errV := SolveCompiled(t.Context(), pm.comp,
				WithPresolve(true), WithCuts(true), WithHeuristics(true), WithVIPRTrace(tr),
			)
			if errV != nil {
				t.Fatalf("seed=%d VIPR err: %v", seed, errV)
			}
			parAdvCheckResultInvariants(t, "seed_vipr", pm, resVIPR, true)
			if tr.TracedProblem() == pm.comp {
				parAdvVerifyVIPROracle(t, "seed_vipr", tr, sense, pm.exactOpt)
			} else if FormalOracleEnabled() && tr.HasCertificate() {
				raw, mErr := tr.MarshalVIPRJSON()
				if mErr != nil {
					t.Fatalf("seed=%d MarshalVIPRJSON: %v", seed, mErr)
				}
				if resp, cErr := FormalCheckVIPRJSONL(t, raw); cErr != nil || !resp.OK {
					t.Fatalf("seed=%d vipr-check failed: resp=%+v err=%v", seed, resp, cErr)
				}
			}
		}
	})

	t.Run("parallel_time_limit_and_deep_work_stealing_race_safety", func(t *testing.T) {
		t.Parallel()
		// 9 disjoint odd triangles (27 binaries, LP max = 13.5, integer max = 9.0)
		// force > 256 nodes without cuts so Workers = 1, 2, 4, 8 enter exploreTreeParallel.
		m := NewModel("deep_parallel_odd_triangles", Maximize)
		bins := make([]Var, 27)
		objTerms := make([]Term, 0, 29)
		for j := range 27 {
			bins[j] = m.AddBinaryVar("b_" + strconv.Itoa(j))
			objTerms = append(objTerms, bins[j].T(1))
		}
		c0 := m.AddContVar("c0", 0, 10)
		c1 := m.AddContVar("c1", 0, math.Inf(1))
		objTerms = append(objTerms, c0.T(0.25), c1.T(-0.5))
		m.SetObjective(Expr{Terms: objTerms})
		for g := range 9 {
			a, b, c := bins[3*g], bins[3*g+1], bins[3*g+2]
			m.AddConstraint("ab_"+strconv.Itoa(g), NewExpr(a.T(1), b.T(1)).Le(1))
			m.AddConstraint("bc_"+strconv.Itoa(g), NewExpr(b.T(1), c.T(1)).Le(1))
			m.AddConstraint("ac_"+strconv.Itoa(g), NewExpr(a.T(1), c.T(1)).Le(1))
		}
		m.AddConstraint("link_c", NewExpr(c0.T(1), c1.T(-1), bins[0].T(2)).Le(4))
		comp := m.Compile()
		// Exact optimal value: 9 triangles * 1 + c0(4 at b0=0, c1=0? b0=0 still lets b1=1 in group 0!) = 9 + 0.25*4 = 10.0.
		exactOpt := big.NewRat(10, 1)
		pm := parAdvPlantedMIP{
			name:      "deep_parallel_odd_triangles",
			sense:     Maximize,
			comp:      comp,
			exactOpt:  exactOpt,
			exactOptF: 10.0,
		}

		for _, w := range workerCounts {
			resNode, err := SolveCompiled(t.Context(), comp,
				WithPresolve(true), WithCuts(false), WithHeuristics(true),
				WithWorkers(w), WithMaxNodes(35),
			)
			if err != nil {
				t.Fatalf("w=%d MaxNodes(35) err: %v", w, err)
			}
			if resNode.Nodes > 35 {
				t.Fatalf("w=%d Nodes=%d > 35", w, resNode.Nodes)
			}
			parAdvCheckResultInvariants(t, "deep_maxnodes_w"+strconv.Itoa(w), pm, resNode, false)

			resTime, errT := SolveCompiled(t.Context(), comp,
				WithPresolve(true), WithCuts(false), WithHeuristics(true),
				WithWorkers(w), WithTimeLimit(2*time.Millisecond),
			)
			if errT != nil {
				t.Fatalf("w=%d TimeLimit err: %v", w, errT)
			}
			if resTime.Status != StatusOptimal && resTime.Status != StatusTimeLimit {
				t.Fatalf("w=%d unexpected status=%v", w, resTime.Status)
			}
			parAdvCheckResultInvariants(t, "deep_timelimit_w"+strconv.Itoa(w), pm, resTime, false)
		}
	})

	t.Run("repro_bug_heur_solver_unscale_pool_state", func(t *testing.T) {
		t.Parallel()
		// Reproduces bab.go:298-308 + simplex.go:1067-1073: when p.getScaledLP().scaled is true
		// and lpSolver unscales in-tree via SetStructuralBounds (e.g. node bound 1e306 overflows
		// Ldexp(1e306, -colExp)), newHeurSolverFrom passes initBasis=false to newLPSolver while
		// CopyRootBasisFrom returns early on hs.scaled != lpSolver.scaled, leaving hs.ws uninitialized.
		m := NewModel("unscale_heur_basis_bug", Maximize)
		c0 := m.AddContVar("c0", 0, math.Inf(1))
		c1 := m.AddContVar("c1", 0, 10)
		b0 := m.AddBinaryVar("b0")
		m.SetObjective(NewExpr(c0.T(-1e-300), c1.T(1), b0.T(1)))
		m.AddConstraint("scale_row", NewExpr(c0.T(math.Ldexp(1, 30)), c1.T(1)).Ge(5))
		m.AddConstraint("couple_row", NewExpr(c0.T(1), c1.T(1)).Le(8))
		m.AddConstraint("cap_huge", NewExpr(c0.T(1), b0.T(1e306)).Le(1e306))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		if st := lp.Solve().Status; st != StatusOptimal {
			t.Fatalf("root LP status=%v, want StatusOptimal", st)
		}
		ubNode := slices.Clone(p.varUB)
		ubNode[0] = 1e306
		lp.SetStructuralBounds(p.varLB, ubNode)
		if st := lp.Solve().Status; st != StatusOptimal {
			t.Fatalf("node LP status=%v, want StatusOptimal", st)
		}

		hs := newHeurSolverFrom(p, lp, time.Time{}, nil, false)
		defer hs.Close()

		basisDistinct := hs.ws != nil && len(hs.ws.basis) >= p.numConstraints
		if basisDistinct {
			seen := make(map[int]bool, p.numConstraints)
			for i := range p.numConstraints {
				bv := hs.ws.basis[i]
				if bv < 0 || bv >= p.numVars+p.numConstraints || seen[bv] {
					basisDistinct = false
					break
				}
				seen[bv] = true
			}
		}
		if !basisDistinct {
			t.Fatalf("newHeurSolverFrom left hs.ws.basis uninitialized (%v) when hs.scaled=%v lp.scaled=%v",
				hs.ws.basis[:p.numConstraints], hs.scaled, lp.scaled)
		}
	})
}

func TestFormalMultiFeatureCutAndOrbitStress(t *testing.T) {
	t.Parallel()

	t.Run("knapsack_cover_extreme_scale_and_comp", func(t *testing.T) {
		t.Parallel()
		bigW := math.Ldexp(1, 46)
		smallW := math.Ldexp(1, -7) + math.Ldexp(1, -20)
		rhs := bigW + 0.5

		m := NewModel("adv_knapsack_cover", Maximize)
		x0 := m.AddBinaryVar("x0")
		xs := make([]Var, 60)
		objTerms := []Term{x0.T(100)}
		rowTerms := []Term{x0.T(bigW)}
		for i := range 60 {
			xs[i] = m.AddBinaryVar("x_" + strconv.Itoa(i+1))
			objTerms = append(objTerms, xs[i].T(1))
			rowTerms = append(rowTerms, xs[i].T(smallW))
		}
		// Complemented binary item (-2*xc) and semi-bounded continuous slack yc in [0, +Inf).
		xc := m.AddBinaryVar("xc")
		yc := m.AddContVar("yc", 0, math.Inf(1))
		z0 := m.AddBinaryVar("z0")
		z1 := m.AddBinaryVar("z1")
		z2 := m.AddBinaryVar("z2")
		objTerms = append(objTerms, xc.T(-0.25), yc.T(-1), z0.T(0.5), z1.T(0.5), z2.T(0.5))
		rowTerms = append(rowTerms, xc.T(-2.0), yc.T(1.0))

		m.SetObjective(Expr{Terms: objTerms})
		m.AddConstraint("knap", Constraint{Expr: Expr{Terms: rowTerms}, Sense: LessEqual, RHS: rhs - 2.0})
		m.AddConstraint("t01", NewExpr(z0.T(1), z1.T(1)).Le(1))
		m.AddConstraint("t12", NewExpr(z1.T(1), z2.T(1)).Le(1))
		m.AddConstraint("t02", NewExpr(z0.T(1), z2.T(1)).Le(1))
		p := m.Compile()

		// Exact feasible witness: x0=1, xs[0..59]=1, xc=1, yc=0, z=(1,0,0) -> LHS = 2^46 + 60*(2^-7+2^-20) - 2 < 2^46 - 1.5, obj = 160.25.
		n := p.numVars
		witRat := make([]*big.Rat, n)
		primalProbe := make([]float64, n)
		witRat[0] = big.NewRat(1, 1)
		primalProbe[0] = 0.999
		for i := range 60 {
			witRat[1+i] = big.NewRat(1, 1)
			primalProbe[1+i] = 0.998
		}
		witRat[61] = big.NewRat(1, 1) // xc = 1
		primalProbe[61] = 0.001
		witRat[62] = big.NewRat(0, 1) // yc = 0
		witRat[63] = big.NewRat(1, 1) // z0 = 1
		witRat[64] = big.NewRat(0, 1)
		witRat[65] = big.NewRat(0, 1)

		var ws cutGenWorkspace
		coverCuts := separateKnapsackCoverCuts(p, primalProbe, &ws)
		parAdvCheckCutsAgainstWitnesses(t, "knapsack_cover_direct", coverCuts, [][]*big.Rat{witRat})

		exactOpt := big.NewRat(641, 4) // 160.25
		pm := parAdvPlantedMIP{name: "adv_knapsack_cover", sense: Maximize, comp: p, exactOpt: exactOpt, exactOptF: 160.25}

		resNoCuts, err := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(false), WithHeuristics(true))
		if err != nil {
			t.Fatalf("no-cuts err: %v", err)
		}
		parAdvCheckResultInvariants(t, "cover_nocuts", pm, resNoCuts, true)

		for _, w := range []int{1, 4} {
			resAll, errA := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(true), WithHeuristics(true), WithWorkers(w))
			if errA != nil {
				t.Fatalf("all-on w=%d err: %v", w, errA)
			}
			parAdvCheckResultInvariants(t, "cover_all_w"+strconv.Itoa(w), pm, resAll, true)
		}

		tr := NewVIPRTrace()
		resVIPR, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithHeuristics(true), WithVIPRTrace(tr))
		if errV != nil {
			t.Fatalf("VIPR err: %v", errV)
		}
		parAdvCheckResultInvariants(t, "cover_vipr", pm, resVIPR, true)
		parAdvVerifyVIPROracle(t, "cover_vipr", tr, Maximize, exactOpt)
	})

	t.Run("lotsizing_flow_chain_extreme_demand_and_vub", func(t *testing.T) {
		t.Parallel()
		dBig := math.Ldexp(1, 53) // 9007199254740992
		m := NewModel("adv_lotsizing_flow", Minimize)
		y0 := m.AddBinaryVar("y0")
		p0 := m.AddContVar("p0", 0, 1000)
		i0 := m.AddContVar("i0", 0, math.Inf(1))
		y1 := m.AddBinaryVar("y1")
		p1 := m.AddContVar("p1", 0, dBig)
		i1 := m.AddContVar("i1", 0, math.Inf(1))
		y2 := m.AddBinaryVar("y2")
		p2 := m.AddContVar("p2", 0, 50000)
		i2 := m.AddContVar("i2", 0, math.Inf(1))
		// Disaggregated VUB cluster coupled to binary yv.
		yv := m.AddBinaryVar("yv")
		v0 := m.AddContVar("v0", 0, 10)
		v1 := m.AddContVar("v1", 0, 10)

		m.SetObjective(NewExpr(
			y0.T(10), i0.T(1), y1.T(10), i1.T(1), y2.T(1000), i2.T(1e-10),
			yv.T(4), v0.T(-3), v1.T(-7),
		))
		m.AddConstraint("vub0", NewExpr(p0.T(1), y0.T(-1000)).Le(0))
		m.AddConstraint("vub1", NewExpr(p1.T(1), y1.T(-dBig)).Le(0))
		m.AddConstraint("vub2", NewExpr(p2.T(1), y2.T(-50000)).Le(0))
		m.AddConstraint("bal0", NewExpr(p0.T(1), i0.T(-1)).Eq(100))
		m.AddConstraint("bal1", NewExpr(i0.T(1), p1.T(1), i1.T(-1)).Eq(dBig))
		m.AddConstraint("bal2", NewExpr(i1.T(1), p2.T(1), i2.T(-1)).Eq(160))
		m.AddConstraint("disagg_vub", NewExpr(v0.T(3), v1.T(7), yv.T(-5)).Le(0))
		p := m.Compile()

		// Exact optimal witness: y0=1, p0=260, i0=160, y1=1, p1=2^53, i1=160, y2=0, p2=0, i2=0 (340),
		// plus yv=1, v0=0, v1=5/7 (4 - 5 = -1) -> exactOpt = 339.
		witF := []float64{1, 260, 160, 1, dBig, 160, 0, 0, 0, 1, 0, 5.0 / 7.0}
		witRat := []*big.Rat{
			big.NewRat(1, 1), big.NewRat(260, 1), big.NewRat(160, 1),
			big.NewRat(1, 1), parAdvMustRat(t, dBig), big.NewRat(160, 1),
			big.NewRat(0, 1), big.NewRat(0, 1), big.NewRat(0, 1),
			big.NewRat(1, 1), big.NewRat(0, 1), big.NewRat(5, 7),
		}
		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", lpRes.Status)
		}
		var ws cutGenWorkspace
		flowCuts := separateFlowAndVUBCuts(p, lpRes.Primal, &ws)
		if len(flowCuts) == 0 {
			t.Fatal("expected non-empty lot-sizing / VUB cuts")
		}
		parAdvCheckCutsAgainstWitnesses(t, "lotsizing_flow_direct", flowCuts, [][]*big.Rat{witRat})

		if FormalOracleEnabled() {
			items, expErr := ExportSeparatedCutsItems(p, [][]float64{witF})
			if expErr != nil {
				t.Fatalf("ExportSeparatedCutsItems: %v", expErr)
			}
			for idx, raw := range items {
				resp, callErr := FormalCheckCutsJSONL(t, raw)
				if callErr != nil || !resp.OK {
					t.Fatalf("cuts-check item %d failed: resp=%+v err=%v", idx, resp, callErr)
				}
			}
		}

		exactOpt := big.NewRat(339, 1)
		pm := parAdvPlantedMIP{name: "adv_lotsizing_flow", sense: Minimize, comp: p, exactOpt: exactOpt, exactOptF: 339.0}

		resNoCuts, err := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(false), WithHeuristics(true))
		if err != nil {
			t.Fatalf("no-cuts err: %v", err)
		}
		parAdvCheckResultInvariants(t, "lotsizing_nocuts", pm, resNoCuts, true)

		for _, w := range []int{1, 4} {
			resAll, errA := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(true), WithHeuristics(true), WithWorkers(w))
			if errA != nil {
				t.Fatalf("all-on w=%d err: %v", w, errA)
			}
			parAdvCheckResultInvariants(t, "lotsizing_all_w"+strconv.Itoa(w), pm, resAll, true)
		}

		tr := NewVIPRTrace()
		resVIPR, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithHeuristics(true), WithVIPRTrace(tr))
		if errV != nil {
			t.Fatalf("VIPR err: %v", errV)
		}
		parAdvCheckResultInvariants(t, "lotsizing_vipr", pm, resVIPR, true)
		parAdvVerifyVIPROracle(t, "lotsizing_vipr", tr, Minimize, exactOpt)
	})

	t.Run("aggregated_mir_and_pairwise_surrogate_wide_domain", func(t *testing.T) {
		t.Parallel()
		// Build n = 106 (> cutMaxNNZSmallN = 100) so generateCutRound activates fullAgg
		// (separateAggregatedCore variable-elimination chains + VUB cluster sum + separatePairwiseSurrogateCuts),
		// mixing scales 1e15, 2^53, 1e-10, semi-bounded [0, +Inf), and a free (-Inf, +Inf) continuous variable.
		m := NewModel("adv_agg_mir_surrogate", Minimize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddIntVar("x1", 0, 10)
		midCont := m.AddContVar("mid_cont", 0, 1e15)
		semiCont := m.AddContVar("semi_cont", 0, math.Inf(1))
		freeCont := m.AddContVar("free_cont", math.Inf(-1), math.Inf(1))
		freePartner := m.AddContVar("free_partner", 0, 10)

		yOpen0 := m.AddBinaryVar("y_open0")
		yOpen1 := m.AddBinaryVar("y_open1")
		f00 := m.AddContVar("f00", 0, 10)
		f01 := m.AddContVar("f01", 0, 10)
		f10 := m.AddContVar("f10", 0, 10)
		f11 := m.AddContVar("f11", 0, 10)

		// 94 auxiliary binaries (total vars = 12 + 94 = 106 > 100) in 47 bipartite pairs (totally unimodular).
		auxBins := make([]Var, 94)
		objTerms := []Term{
			x0.T(3), x1.T(4),
			midCont.T(1e-10), semiCont.T(1),
			freeCont.T(0), freePartner.T(1),
			yOpen0.T(6), yOpen1.T(6),
		}
		for j := range 94 {
			auxBins[j] = m.AddBinaryVar("ab_" + strconv.Itoa(j))
			if j%2 == 0 {
				objTerms = append(objTerms, auxBins[j].T(-0.5))
			} else {
				objTerms = append(objTerms, auxBins[j].T(-0.25))
			}
		}
		m.SetObjective(Expr{Terms: objTerms})

		// Equality + inequality elimination chain through midCont:
		// midCont - 3*x0 == 0 and -midCont - 4*x1 - semiCont <= -11 -> 3*x0 + 4*x1 + semiCont >= 11.
		m.AddConstraint("elim_eq", NewExpr(midCont.T(1), x0.T(-3)).Eq(0))
		m.AddConstraint("elim_le", NewExpr(midCont.T(-1), x1.T(-4), semiCont.T(-1)).Le(-11))

		// Pairwise tight fractional-integer inequalities sharing x0, x1 at scale ratio 10:
		m.AddConstraint("pair_a", NewExpr(x0.T(2), x1.T(3)).Ge(7))
		m.AddConstraint("pair_b", NewExpr(x0.T(20), x1.T(10)).Ge(45))

		// VUB capacity cluster + demand partitioning rows (Mechanism C in separateAggregatedCore):
		m.AddConstraint("vub_cap0", NewExpr(f00.T(1), f01.T(1), yOpen0.T(-8)).Le(0))
		m.AddConstraint("vub_cap1", NewExpr(f10.T(1), f11.T(1), yOpen1.T(-8)).Le(0))
		m.AddConstraint("dem0", NewExpr(f00.T(1), f10.T(1)).Eq(5))
		m.AddConstraint("dem1", NewExpr(f01.T(1), f11.T(1)).Eq(5))

		// Row containing free (-Inf, +Inf) continuous variable must be safely skipped by aggRowOK/mirValid.
		m.AddConstraint("free_eq", NewExpr(freeCont.T(1), freePartner.T(-1)).Eq(0))
		m.AddConstraint("free_ub", NewExpr(freeCont.T(1)).Le(0))
		m.AddConstraint("free_lb", NewExpr(freeCont.T(1)).Ge(0))

		for g := range 46 {
			m.AddConstraint("bp_"+strconv.Itoa(g), NewExpr(auxBins[2*g].T(1), auxBins[2*g+1].T(1)).Le(1))
		}
		m.AddConstraint("ab_penult", NewExpr(auxBins[92].T(1), auxBins[93].T(1)).Le(1))
		m.AddConstraint("ab_last", NewExpr(auxBins[93].T(math.Ldexp(1, 53))).Le(0))
		p := m.Compile()
		if p.numVars <= cutMaxNNZSmallN {
			t.Fatalf("expected p.numVars=%d > %d", p.numVars, cutMaxNNZSmallN)
		}

		// Exact feasible optimum:
		// (x0=2, x1=1, midCont=6, semiCont=1) -> 11 + 6e-10; VUB cluster -> 12; 47 pairs (even=1, odd=0) -> 47*(-0.5) = -23.5.
		// Total exactOpt = 11 + 12 - 23.5 + 6e-10 = -0.5 + 6e-10.
		witRat := make([]*big.Rat, p.numVars)
		witRat[0] = big.NewRat(2, 1)
		witRat[1] = big.NewRat(1, 1)
		witRat[2] = big.NewRat(6, 1)
		witRat[3] = big.NewRat(1, 1)
		witRat[4] = big.NewRat(0, 1)
		witRat[5] = big.NewRat(0, 1)
		witRat[6] = big.NewRat(1, 1)
		witRat[7] = big.NewRat(1, 1)
		witRat[8] = big.NewRat(5, 1)
		witRat[9] = big.NewRat(3, 1)
		witRat[10] = big.NewRat(0, 1)
		witRat[11] = big.NewRat(2, 1)
		for j := range 94 {
			if j%2 == 0 {
				witRat[12+j] = big.NewRat(1, 1)
			} else {
				witRat[12+j] = big.NewRat(0, 1)
			}
		}

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", lpRes.Status)
		}
		var ws cutGenWorkspace
		aggCuts := separateAggregatedMIRCuts(lp, lpRes.Primal, &ws)
		if len(aggCuts) == 0 {
			t.Fatal("expected non-empty aggregated MIR / surrogate cuts")
		}
		parAdvCheckCutsAgainstWitnesses(t, "agg_mir_surrogate_direct", aggCuts, [][]*big.Rat{witRat})

		exactOpt := new(big.Rat).Add(big.NewRat(-1, 2), new(big.Rat).Mul(big.NewRat(6, 1), parAdvMustRat(t, 1e-10)))
		exactOptF, _ := exactOpt.Float64()
		pm := parAdvPlantedMIP{name: "adv_agg_mir_surrogate", sense: Minimize, comp: p, exactOpt: exactOpt, exactOptF: exactOptF}

		resNoCuts, err := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(false), WithHeuristics(true))
		if err != nil {
			t.Fatalf("no-cuts err: %v", err)
		}
		parAdvCheckResultInvariants(t, "agg_nocuts", pm, resNoCuts, true)

		for _, w := range []int{1, 4} {
			resAll, errA := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(true), WithHeuristics(true), WithWorkers(w))
			if errA != nil {
				t.Fatalf("all-on w=%d err: %v", w, errA)
			}
			parAdvCheckResultInvariants(t, "agg_all_w"+strconv.Itoa(w), pm, resAll, true)
		}

		tr := NewVIPRTrace()
		resVIPR, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithHeuristics(true), WithVIPRTrace(tr))
		if errV != nil {
			t.Fatalf("VIPR err: %v", errV)
		}
		parAdvCheckResultInvariants(t, "agg_vipr", pm, resVIPR, true)
		parAdvVerifyVIPROracle(t, "agg_vipr", tr, Minimize, exactOpt)
	})

	t.Run("orbital_symmetry_static_and_dynamic_parallel", func(t *testing.T) {
		t.Parallel()
		// 4 groups: when global switch f=1 (forced in integers by 2*f >= 1), groups {0,1,2} have
		// exact cap 2^53 - 0.75 = 2^53 - 1 (infeasible for x_g >= 2^53*y_g when y_g=1), leaving
		// group 3 as the unique feasible group; coupled with 4 symmetric odd triangles so parallel
		// workers (w = 1, 2, 4, 8) exercise dynamic activeOrbit, twin classes, and RCF under race.
		bigM := math.Ldexp(1, 53)
		m := NewModel("adv_orbital_parallel", Minimize)
		f := m.AddBinaryVar("f")
		v := m.AddContVar("v", 0, math.Inf(1))
		var yg, xg [4]Var
		for g := range 4 {
			yg[g] = m.AddBinaryVar("y_" + strconv.Itoa(g))
			xg[g] = m.AddIntVar("x_"+strconv.Itoa(g), 0, bigM)
		}
		twins := make([]Var, 12)
		objTerms := []Term{f.T(10), v.T(1)}
		for j := range 12 {
			twins[j] = m.AddBinaryVar("tw_" + strconv.Itoa(j))
			objTerms = append(objTerms, twins[j].T(-1))
		}
		m.SetObjective(Expr{Terms: objTerms})
		m.AddConstraint("force_f", NewExpr(f.T(2)).Ge(1))
		m.AddConstraint("part_y", NewExpr(yg[0].T(1), yg[1].T(1), yg[2].T(1), yg[3].T(1)).Eq(1))
		for g := range 3 {
			m.AddConstraint("cap_"+strconv.Itoa(g), NewExpr(xg[g].T(1), f.T(0.75)).Le(bigM))
		}
		m.AddConstraint("cap_3", NewExpr(xg[3].T(1)).Le(bigM))
		for g := range 4 {
			m.AddConstraint("link_x_"+strconv.Itoa(g), NewExpr(xg[g].T(1), yg[g].T(-bigM)).Ge(0))
			m.AddConstraint("link_v_"+strconv.Itoa(g), NewExpr(yg[g].T(1), v.T(-1)).Le(0))
		}
		for g := range 4 {
			a, b, c := twins[3*g], twins[3*g+1], twins[3*g+2]
			m.AddConstraint("sym_ab_"+strconv.Itoa(g), NewExpr(a.T(1), b.T(1)).Le(1))
			m.AddConstraint("sym_bc_"+strconv.Itoa(g), NewExpr(b.T(1), c.T(1)).Le(1))
			m.AddConstraint("sym_ac_"+strconv.Itoa(g), NewExpr(a.T(1), c.T(1)).Le(1))
		}
		p := m.Compile()
		// Exact optimum: f=1 (10) + v=1 (1) + 4 triangles * (-1) = 7.0, with yg[3] == 1.
		exactOpt := big.NewRat(7, 1)
		pm := parAdvPlantedMIP{name: "adv_orbital_parallel", sense: Minimize, comp: p, exactOpt: exactOpt, exactOptF: 7.0}

		for _, preOn := range []bool{false, true} {
			for _, w := range []int{1, 2, 4, 8} {
				res, err := SolveCompiled(t.Context(), p,
					WithPresolve(preOn), WithCuts(true), WithHeuristics(true), WithWorkers(w),
				)
				if err != nil {
					t.Fatalf("pre=%v w=%d err: %v", preOn, w, err)
				}
				parAdvCheckResultInvariants(t, "orbit_w"+strconv.Itoa(w), pm, res, true)
				if res.Values[8] < 0.5 { // f=0, v=1, y0=2,x0=3, y1=4,x1=5, y2=6,x2=7, y3=8,x3=9
					t.Fatalf("pre=%v w=%d expected y_3 (col 8) == 1, got %g", preOn, w, res.Values[8])
				}
			}
		}

		resNoCuts, errNC := SolveCompiled(t.Context(), p, WithPresolve(true), WithCuts(false), WithHeuristics(true), WithWorkers(4))
		if errNC != nil {
			t.Fatalf("no-cuts w=4 err: %v", errNC)
		}
		parAdvCheckResultInvariants(t, "orbit_nocuts_w4", pm, resNoCuts, true)

		tr := NewVIPRTrace()
		resVIPR, errV := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true), WithHeuristics(true), WithVIPRTrace(tr))
		if errV != nil {
			t.Fatalf("VIPR err: %v", errV)
		}
		parAdvCheckResultInvariants(t, "orbit_vipr", pm, resVIPR, true)
		parAdvVerifyVIPROracle(t, "orbit_vipr", tr, Minimize, exactOpt)

		if FormalOracleEnabled() {
			items, _ := ExportPresolveRulesItems(p)
			for idx, raw := range items {
				resp, cErr := FormalCheckRulesJSONL(t, raw)
				if cErr != nil || !resp.OK {
					t.Fatalf("orbit rules-check item %d failed: resp=%+v err=%v", idx, resp, cErr)
				}
			}
		}
	})

	t.Run("finalize_sparse_cut_wide_and_unbounded_domains", func(t *testing.T) {
		t.Parallel()
		bigB := math.Ldexp(1, 53)
		m := NewModel("finalize_cut_domains", Minimize)
		x0 := m.AddIntVar("x0", 0, 10)
		xWide := m.AddContVar("x_wide", -1e15, 1e15)
		xSemiPos := m.AddContVar("x_semi_pos", 0, math.Inf(1))
		xSemiNeg := m.AddContVar("x_semi_neg", math.Inf(-1), 0)
		xFree := m.AddContVar("x_free", math.Inf(-1), math.Inf(1))
		xBig := m.AddIntVar("x_big", 0, bigB)
		m.SetObjective(NewExpr(x0.T(1), xWide.T(0), xSemiPos.T(0), xSemiNeg.T(0), xFree.T(0), xBig.T(0)))
		p := m.Compile()

		primal := []float64{3.5, 0, 0, 0, 0, 0}
		var ws cutGenWorkspace

		// 1. Positive tiny coeff on [0, +Inf) has finite worst-case -min(v*x) = 0 -> must succeed and remain valid.
		c1, ok1 := finalizeSparseCut(p, []int{0, 2}, []float64{1.0, 1e-13}, 0.5, primal, 0, &ws)
		if !ok1 {
			t.Fatal("expected positive tiny coeff on [0, +Inf) to succeed")
		}
		parAdvCheckCutsAgainstWitnesses(t, "semi_pos_drop", []cutPlane{c1}, [][]*big.Rat{
			{big.NewRat(0, 1), big.NewRat(0, 1), big.NewRat(1e18, 1), big.NewRat(0, 1), big.NewRat(0, 1), big.NewRat(0, 1)},
		})

		// 2. Negative tiny coeff on [0, +Inf) has unbounded worst-case -v*x -> +Inf -> must reject.
		if _, ok := finalizeSparseCut(p, []int{0, 2}, []float64{1.0, -1e-13}, 0.5, primal, 0, &ws); ok {
			t.Fatal("expected negative tiny coeff on [0, +Inf) to be rejected")
		}

		// 3. Positive tiny coeff on (-Inf, 0] has unbounded worst-case -v*x -> +Inf -> must reject.
		if _, ok := finalizeSparseCut(p, []int{0, 3}, []float64{1.0, 1e-13}, 0.5, primal, 0, &ws); ok {
			t.Fatal("expected positive tiny coeff on (-Inf, 0] to be rejected")
		}

		// 4. Any non-zero tiny coeff on free (-Inf, +Inf) variable must be rejected.
		if _, ok := finalizeSparseCut(p, []int{0, 4}, []float64{1.0, 1e-13}, 0.5, primal, 0, &ws); ok {
			t.Fatal("expected tiny coeff on free variable to be rejected")
		}

		// 5. Duplicate column entries on xBig in [0, 2^53] cancelling to -2^-52 relax RHS from 0.5 to 2.5 (< primal 3.5).
		// Valid base inequality: x0 + 1.0*xBig - (1.0 + 2^-52)*xBig <= 0.5, satisfied at (x0=2, xBig=2^53) since 2 - 2 = 0 <= 0.5.
		cDup, okDup := finalizeSparseCut(p, []int{0, 5, 5}, []float64{1.0, 1.0, -(1.0 + math.Ldexp(1, -52))}, 0.5, primal, 0, &ws)
		if !okDup {
			t.Fatal("expected bounded duplicate cancellation cut to succeed")
		}
		parAdvCheckCutsAgainstWitnesses(t, "dup_wide_drop", []cutPlane{cDup}, [][]*big.Rat{
			{big.NewRat(2, 1), parAdvMustRat(t, 1e15), big.NewRat(0, 1), big.NewRat(0, 1), big.NewRat(0, 1), parAdvMustRat(t, bigB)},
		})
	})

	t.Run("repro_bug_delayed_objconst_cancellation_k5", func(t *testing.T) {
		t.Parallel()
		// Reproduces bab.go:493-499 & 2970-2975: on a K5 clique with -2^56 binary costs and ObjConstant=+2^56,
		// root LP has sum(z)=2.5 (|rootShiftedMin| = 1.5*2^56 >= 0.5*|rootLinMin| = 1.25*2^56), leaving searchShift=0.
		// Every integer solution has sum(z)=1, where -2^56 cancels +2^56; unshifted float64 rounding returns
		// suboptimal Objective=7 (vs exactOpt=6) AND inverted BestBound=8 > Objective=7 on Minimize!
		bigC := math.Ldexp(1, 56)
		m := NewModel("delayed_shift_k5_bug", Minimize)
		var z [5]Var
		for i := range 5 {
			z[i] = m.AddBinaryVar("z_" + strconv.Itoa(i))
		}
		x := m.AddIntVar("x", 0, 10)
		y := m.AddBinaryVar("y")
		m.SetObjective(Expr{
			Terms: []Term{
				z[0].T(-bigC), z[1].T(-bigC), z[2].T(-bigC), z[3].T(-bigC), z[4].T(-bigC),
				x.T(3), y.T(7),
			},
			Constant: bigC,
		})
		for i := range 5 {
			for j := i + 1; j < 5; j++ {
				m.AddConstraint("clq_"+strconv.Itoa(i)+"_"+strconv.Itoa(j), NewExpr(z[i].T(1), z[j].T(1)).Le(1))
			}
		}
		m.AddConstraint("z_ge1", NewExpr(z[0].T(1), z[1].T(1), z[2].T(1), z[3].T(1), z[4].T(1)).Ge(1))
		m.AddConstraint("xy_cov", NewExpr(x.T(3), y.T(10)).Ge(5))
		p := m.Compile()

		res, err := SolveCompiled(t.Context(), p,
			WithPresolve(true), WithCuts(true), WithHeuristics(true),
			WithRelGapTol(0), WithAbsGapTol(0),
		)
		if err != nil {
			t.Fatalf("SolveCompiled err: %v", err)
		}
		if res.Status != StatusOptimal || math.Abs(res.Objective-6.0) > 1e-6 || res.BestBound > 6.0+1e-6 {
			t.Fatalf("delayed ObjConstant cancellation returned status=%v Objective=%.12g BestBound=%.12g (want Optimal Objective=6 BestBound<=6)",
				res.Status, res.Objective, res.BestBound)
		}
	})

	t.Run("repro_bug_sb_probe_cutoff_shift_cancellation", func(t *testing.T) {
		t.Parallel()
		// Reproduces bab.go:3461 (probeCutOff in selectBranchVarCore): SolveBoundOnly returns unshifted
		// float64 bound fl(2^56 + 9.5) = 2^56 + 16, and bound + cfg.objConstMin (-2^56) yields +16.0,
		// pruning the y <= 0 branch containing the true optimum 11.0 against incumbent 14.0.
		bigC := math.Ldexp(1, 56)
		m := NewModel("sb_probe_cutoff_shift_bug", Minimize)
		z1 := m.AddBinaryVar("z1")
		z2 := m.AddBinaryVar("z2")
		x := m.AddIntVar("x", 0, 10)
		y := m.AddBinaryVar("y")
		u1 := m.AddBinaryVar("u1")
		u2 := m.AddBinaryVar("u2")
		u3 := m.AddBinaryVar("u3")
		m.SetObjective(NewExpr(
			z1.T(bigC), z2.T(bigC),
			x.T(4), y.T(15),
			u1.T(-1), u2.T(-1), u3.T(-1),
		).AddConstant(-bigC))
		m.AddConstraint("z_part", NewExpr(z1.T(1), z2.T(1)).Eq(1))
		m.AddConstraint("cover", NewExpr(x.T(4), y.T(16)).Ge(11))
		m.AddConstraint("u12", NewExpr(u1.T(1), u2.T(1)).Le(1))
		m.AddConstraint("u23", NewExpr(u2.T(1), u3.T(1)).Le(1))
		m.AddConstraint("u13", NewExpr(u1.T(1), u3.T(1)).Le(1))
		p := m.Compile()

		res, err := SolveCompiled(t.Context(), p,
			WithPresolve(false), WithCuts(false), WithHeuristics(true),
			WithRelGapTol(0), WithAbsGapTol(0),
		)
		if err != nil {
			t.Fatalf("SolveCompiled err: %v", err)
		}
		if res.Status != StatusOptimal || math.Abs(res.Objective-11.0) > 1e-6 || res.BestBound > 11.0+1e-6 {
			t.Fatalf("probeCutOff returned status=%v Objective=%.12g BestBound=%.12g (want Optimal Objective=11 BestBound<=11)",
				res.Status, res.Objective, res.BestBound)
		}
	})
}

func TestFormalUnboundedMeyerCliffs(t *testing.T) {
	t.Parallel()

	workerCounts := []int{1, 2, 4, 8}

	t.Run("diophantine_3x_3y_eq_1_and_gcd_traps", func(t *testing.T) {
		t.Parallel()
		// 1. Classic LP-unbounded Diophantine trap: min -x s.t. 3x - 3y = 1, x, y >= 0 Int.
		m1 := NewModel("meyer_3x_3y_1", Minimize)
		x1 := m1.AddIntVar("x", 0, math.Inf(1))
		y1 := m1.AddIntVar("y", 0, math.Inf(1))
		m1.SetObjective(NewExpr(x1.T(-1)))
		m1.AddConstraint("eq", NewExpr(x1.T(3), y1.T(-3)).Eq(1))

		// 2. 3-var Maximize LP-unbounded Diophantine trap: max x + 2y s.t. 6x - 9y + 15z = 2.
		m2 := NewModel("meyer_6_9_15_2", Maximize)
		x2 := m2.AddIntVar("x", 0, math.Inf(1))
		y2 := m2.AddIntVar("y", 0, math.Inf(1))
		z2 := m2.AddIntVar("z", 0, math.Inf(1))
		m2.SetObjective(NewExpr(x2.T(1), y2.T(2)))
		m2.AddConstraint("eq", NewExpr(x2.T(6), y2.T(-9), z2.T(15)).Eq(2))

		// 3. Coupled 2-row Diophantine trap where row 0 has unit GCD (x - 2y = 0) and row 1 (2y - 2z = 1) is infeasible.
		m3 := NewModel("meyer_coupled_rows", Minimize)
		x3 := m3.AddIntVar("x", 0, math.Inf(1))
		y3 := m3.AddIntVar("y", 0, math.Inf(1))
		z3 := m3.AddIntVar("z", 0, math.Inf(1))
		m3.SetObjective(NewExpr(x3.T(-1)))
		m3.AddConstraint("r0", NewExpr(x3.T(1), y3.T(-2)).Eq(0))
		m3.AddConstraint("r1", NewExpr(y3.T(2), z3.T(-2)).Eq(1))

		for _, mdl := range []*Model{m1, m2, m3} {
			p := mdl.Compile()
			for _, preOn := range []bool{false, true} {
				for _, w := range workerCounts {
					res, err := SolveCompiled(t.Context(), p,
						WithPresolve(preOn), WithCuts(true), WithHeuristics(true), WithWorkers(w),
					)
					if err != nil || res.Status != StatusInfeasible {
						t.Fatalf("%s pre=%v w=%d: status=%v err=%v, want StatusInfeasible",
							mdl.Name(), preOn, w, res.Status, err)
					}
				}
				tr := NewVIPRTrace()
				resV, errV := SolveCompiled(t.Context(), p, WithPresolve(preOn), WithVIPRTrace(tr))
				if errV != nil || resV.Status != StatusInfeasible {
					t.Fatalf("%s VIPR pre=%v: status=%v err=%v, want StatusInfeasible",
						mdl.Name(), preOn, resV.Status, errV)
				}
				parAdvVerifyVIPROracle(t, mdl.Name()+"_vipr", tr, mdl.Sense(), nil)
			}
		}
	})

	t.Run("mixed_int_continuous_diophantine_unbounded_lp_traps", func(t *testing.T) {
		t.Parallel()
		// Continuous variable s in [0, 0.25] prevents pure-integer root GCD shortcuts on 3x - 3y + s = 1.5,
		// while unbounded continuous c -> +Inf makes the LP relaxation unbounded (-Inf).
		m1 := NewModel("mixed_diophantine_bounded_xy", Minimize)
		x1 := m1.AddIntVar("x", 0, 200)
		y1 := m1.AddIntVar("y", 0, 200)
		s1 := m1.AddContVar("s", 0, 0.25)
		c1 := m1.AddContVar("c", 0, math.Inf(1))
		m1.SetObjective(NewExpr(c1.T(-1), x1.T(0.1)))
		m1.AddConstraint("mix_eq", NewExpr(x1.T(3), y1.T(-3), s1.T(1)).Eq(1.5))

		// Unbounded integer ray x, y in [0, +Inf) coupled to an infeasible mixed odd-cycle + continuous slack.
		m2 := NewModel("mixed_odd_cycle_unbounded_ray", Minimize)
		x2 := m2.AddIntVar("x", 0, math.Inf(1))
		y2 := m2.AddIntVar("y", 0, math.Inf(1))
		b0 := m2.AddBinaryVar("b0")
		b1 := m2.AddBinaryVar("b1")
		b2 := m2.AddBinaryVar("b2")
		s2 := m2.AddContVar("s", 0, 0.25)
		m2.SetObjective(NewExpr(x2.T(-1)))
		m2.AddConstraint("ray", NewExpr(x2.T(1), y2.T(-1)).Le(0))
		m2.AddConstraint("e01", NewExpr(b0.T(1), b1.T(1)).Le(1))
		m2.AddConstraint("e12", NewExpr(b1.T(1), b2.T(1)).Le(1))
		m2.AddConstraint("e02", NewExpr(b0.T(1), b2.T(1)).Le(1))
		m2.AddConstraint("forbid", NewExpr(b0.T(1), b1.T(1), b2.T(1), s2.T(1)).Ge(1.5))

		for _, mdl := range []*Model{m1, m2} {
			p := mdl.Compile()
			for _, preOn := range []bool{false, true} {
				for _, cutsOn := range []bool{false, true} {
					for _, w := range workerCounts {
						res, err := SolveCompiled(t.Context(), p,
							WithPresolve(preOn), WithCuts(cutsOn), WithHeuristics(true), WithWorkers(w),
						)
						if err != nil || res.Status != StatusInfeasible {
							t.Fatalf("%s pre=%v cuts=%v w=%d: status=%v err=%v, want StatusInfeasible",
								mdl.Name(), preOn, cutsOn, w, res.Status, err)
						}
					}
				}
				tr := NewVIPRTrace()
				resV, errV := SolveCompiled(t.Context(), p, WithPresolve(preOn), WithCuts(true), WithVIPRTrace(tr))
				if errV != nil || resV.Status != StatusInfeasible {
					t.Fatalf("%s VIPR pre=%v: status=%v err=%v, want StatusInfeasible",
						mdl.Name(), preOn, resV.Status, errV)
				}
				if !preOn && !tr.HasCertificate() {
					t.Fatalf("%s VIPR pre=false: expected B&C certificate from solveUnboundedMIPFeasibility", mdl.Name())
				}
				parAdvVerifyVIPROracle(t, mdl.Name()+"_vipr", tr, mdl.Sense(), nil)
			}
		}
	})

	t.Run("huge_scale_unbounded_rays_feasible_vs_infeasible", func(t *testing.T) {
		t.Parallel()
		// Feasible rational-ray unbounded MILP: 3x - 7y = 1, x - 1e9*b >= 0 (witness b=1, y=3e9+2, x=7e9+5; ray (7, 3, 0)).
		mUnb := NewModel("meyer_huge_feasible_ray", Minimize)
		xu := mUnb.AddIntVar("x", 0, math.Inf(1))
		yu := mUnb.AddIntVar("y", 0, math.Inf(1))
		bu := mUnb.AddBinaryVar("b")
		mUnb.SetObjective(NewExpr(xu.T(-1)))
		mUnb.AddConstraint("dioph", NewExpr(xu.T(3), yu.T(-7)).Eq(1))
		mUnb.AddConstraint("big_lb", NewExpr(xu.T(1), bu.T(-1e9)).Ge(0))
		pUnb := mUnb.Compile()

		// Companion infeasible Diophantine ray: 6x - 14y = 1, x - 1e9*b >= 0 (gcd(6, 14) = 2 does not divide 1).
		mInf := NewModel("meyer_huge_infeasible_ray", Minimize)
		xi := mInf.AddIntVar("x", 0, math.Inf(1))
		yi := mInf.AddIntVar("y", 0, math.Inf(1))
		bi := mInf.AddBinaryVar("b")
		mInf.SetObjective(NewExpr(xi.T(-1)))
		mInf.AddConstraint("dioph", NewExpr(xi.T(6), yi.T(-14)).Eq(1))
		mInf.AddConstraint("big_lb", NewExpr(xi.T(1), bi.T(-1e9)).Ge(0))
		pInf := mInf.Compile()

		for _, preOn := range []bool{false, true} {
			for _, w := range workerCounts {
				resU, errU := SolveCompiled(t.Context(), pUnb, WithPresolve(preOn), WithWorkers(w))
				if errU != nil || resU.Status != StatusUnbounded {
					t.Fatalf("feasible ray pre=%v w=%d: status=%v err=%v, want StatusUnbounded",
						preOn, w, resU.Status, errU)
				}
				resI, errI := SolveCompiled(t.Context(), pInf, WithPresolve(preOn), WithWorkers(w))
				if errI != nil || resI.Status != StatusInfeasible {
					t.Fatalf("infeasible ray pre=%v w=%d: status=%v err=%v, want StatusInfeasible",
						preOn, w, resI.Status, errI)
				}
			}
			tr := NewVIPRTrace()
			resVI, errVI := SolveCompiled(t.Context(), pInf, WithPresolve(preOn), WithVIPRTrace(tr))
			if errVI != nil || resVI.Status != StatusInfeasible {
				t.Fatalf("infeasible ray VIPR pre=%v: status=%v err=%v", preOn, resVI.Status, errVI)
			}
			parAdvVerifyVIPROracle(t, "huge_infeasible_ray_vipr", tr, Minimize, nil)
		}
	})

	t.Run("repro_bug_fake_unbounded_ray_on_bounded_mip", func(t *testing.T) {
		t.Parallel()
		// Reproduces simplex.go:5924,6821 + lu.go:2392 + bab.go:612:
		// Exact elimination of y = x and w = (1 + 2^-54)*x into 2^24*w - 2^24*y - 2^-31*x <= 1
		// gives +2^-31*x <= 1 (0 <= x <= 2^31 = 2147483648, strictly bounded!).
		// Plain float64 ftranDense rounds 1 + 2^-54 to 1.0, flipping fCol[slack_2] to -2^-31 < 0,
		// and verifyUnboundedRay accepts ray=[1,1,1] within 1e-9*freeL1, falsely certifying StatusUnbounded.
		m := NewModel("fake_unbounded_ray_bounded_mip", Minimize)
		x := m.AddIntVar("x", 0, math.Inf(1))
		y := m.AddIntVar("y", 0, math.Inf(1))
		w := m.AddContVar("w", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("r0", NewExpr(y.T(1), x.T(-1)).Eq(0))
		m.AddConstraint("r1", NewExpr(w.T(1), y.T(-1), x.T(-math.Ldexp(1, -54))).Eq(0))
		m.AddConstraint("r2", NewExpr(w.T(math.Ldexp(1, 24)), y.T(-math.Ldexp(1, 24)), x.T(-math.Ldexp(1, -31))).Le(1))
		p := m.Compile()

		res, err := SolveCompiled(t.Context(), p, WithPresolve(true))
		if err != nil {
			t.Fatalf("SolveCompiled err: %v", err)
		}
		if res.Status == StatusUnbounded {
			t.Fatal("bounded MILP (0 <= x <= 2^31) falsely certified StatusUnbounded")
		}
		if res.Status == StatusInfeasible {
			t.Fatal("bounded feasible MILP falsely certified StatusInfeasible")
		}
	})

	t.Run("repro_bug_meyer_2x_2y_5e_7_cliff", func(t *testing.T) {
		t.Parallel()
		// Control scaled by 4 (8x - 8y = 2e-6) exceeds 1e-6 tolerance and reports StatusInfeasible.
		mCtrl := NewModel("meyer_8x_8y_2e_6_ctrl", Minimize)
		xc := mCtrl.AddIntVar("x", 0, math.Inf(1))
		yc := mCtrl.AddIntVar("y", 0, math.Inf(1))
		mCtrl.SetObjective(NewExpr(xc.T(-1)))
		mCtrl.AddConstraint("eq", NewExpr(xc.T(8), yc.T(-8)).Eq(2e-6))
		resCtrl, errCtrl := SolveCompiled(t.Context(), mCtrl.Compile(), WithPresolve(true))
		if errCtrl != nil || resCtrl.Status != StatusInfeasible {
			t.Fatalf("control 8x-8y=2e-6: status=%v err=%v, want StatusInfeasible", resCtrl.Status, errCtrl)
		}

		// Equivalent exact-Q infeasible MILP (2x - 2y = 5e-7) hits presolveFeasTol/incumbentTol (1e-6)
		// in presolve.go:180-206 and bab.go:352,612, returning StatusUnbounded instead of StatusInfeasible.
		m := NewModel("meyer_2x_2y_5e_7", Minimize)
		x := m.AddIntVar("x", 0, math.Inf(1))
		y := m.AddIntVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("eq", NewExpr(x.T(2), y.T(-2)).Eq(5e-7))
		p := m.Compile()

		for _, preOn := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), p, WithPresolve(preOn))
			if err != nil {
				t.Fatalf("presolve=%v err: %v", preOn, err)
			}
			if res.Status != StatusInfeasible {
				t.Fatalf("exact-Q infeasible MILP 2x-2y=5e-7 (presolve=%v) returned status=%v, want StatusInfeasible",
					preOn, res.Status)
			}
		}
	})
}
