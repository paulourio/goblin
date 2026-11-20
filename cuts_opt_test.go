package goblin

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestFinalizeSparseCutEquivalenceAndZeroAlloc(t *testing.T) {
	t.Run("ExactParityWithDense", func(t *testing.T) {
		const n = 40
		m := NewModel("sparse_cut_parity", Minimize)
		for j := range n {
			if j%5 == 0 {
				_ = m.AddContVar(fmt.Sprintf("u%d", j), 0, math.Inf(1))
			} else {
				_ = m.AddContVar(fmt.Sprintf("x%d", j), -5.0-float64(j%7), 10.0+float64(j%11))
			}
		}
		p := m.Compile()

		rng := rand.New(rand.NewPCG(42, 99))
		lpPrimal := make([]float64, n)
		for j := range n {
			lpPrimal[j] = 0.25 + float64(j%5)*0.7
		}

		var ws cutGenWorkspace
		dense := make([]float64, n)

		for trial := range 150 {
			clear(dense)
			var idx []int
			var val []float64

			numTerms := 1 + rng.IntN(16)
			for range numTerms {
				col := rng.IntN(n)
				var v float64
				switch rng.IntN(8) {
				case 0:
					v = (rng.Float64()*2 - 1) * 1e-12 // tests cutDropTol relaxation
				case 1:
					v = 0.0
				case 2:
					v = (rng.Float64()*2 - 1) * 1e3
				default:
					v = (rng.Float64()*2 - 1) * 10.0
				}
				idx = append(idx, col)
				val = append(val, v)
				dense[col] += v
			}

			// Inject duplicate column cancellation on some trials.
			if trial%5 == 0 && len(idx) > 0 {
				c0 := idx[0]
				v0 := val[0]
				idx = append(idx, c0)
				val = append(val, -v0)
				dense[c0] += -v0
			}

			// Compute a violated RHS on most trials, and an unviolated RHS on some.
			act := 0.0
			for j := range n {
				act += dense[j] * lpPrimal[j]
			}
			rhs := act - 0.5
			if trial%7 == 0 {
				rhs = act + 1.0
			}

			refCut, refOK := finalizeCut(p, dense, rhs, lpPrimal)
			gotCut, gotOK := finalizeSparseCut(p, idx, val, rhs, lpPrimal, 0, &ws)
			if gotOK != refOK {
				t.Fatalf("trial %d: ok=%v, want %v (ref=%+v got=%+v)", trial, gotOK, refOK, refCut, gotCut)
			}
			if !refOK {
				continue
			}
			if !slices.Equal(gotCut.ColIdx, refCut.ColIdx) {
				t.Fatalf("trial %d: ColIdx=%v, want %v", trial, gotCut.ColIdx, refCut.ColIdx)
			}
			if !slices.Equal(gotCut.Val, refCut.Val) {
				t.Fatalf("trial %d: Val=%v, want %v", trial, gotCut.Val, refCut.Val)
			}
			if gotCut.RHS != refCut.RHS || gotCut.Efficacy != refCut.Efficacy || gotCut.Norm != refCut.Norm {
				t.Fatalf("trial %d: scalar mismatch got=(rhs=%g eff=%g norm=%g) want=(rhs=%g eff=%g norm=%g)",
					trial, gotCut.RHS, gotCut.Efficacy, gotCut.Norm, refCut.RHS, refCut.Efficacy, refCut.Norm)
			}

			var sumSq float64
			for _, sv := range gotCut.Val {
				sumSq += sv * sv
			}
			wantNorm := math.Sqrt(sumSq)
			if math.Abs(gotCut.Norm-wantNorm) > 1e-13*wantNorm {
				t.Fatalf("trial %d: cached Norm=%g differs from sqrt(sum v^2)=%g", trial, gotCut.Norm, wantNorm)
			}
		}

		// Verify edge cases: non-finite RHS, non-finite coeff, norm overflow, extreme scale.
		if _, ok := finalizeSparseCut(p, []int{1}, []float64{1.0}, math.NaN(), lpPrimal, 0, &ws); ok {
			t.Fatal("finalizeSparseCut accepted NaN rhs")
		}
		if _, ok := finalizeSparseCut(p, []int{1}, []float64{math.Inf(1)}, 0.0, lpPrimal, 0, &ws); ok {
			t.Fatal("finalizeSparseCut accepted Inf coeff")
		}
		if _, ok := finalizeSparseCut(p, []int{1, 2}, []float64{1.5e308, 1.5e308}, 0.0, lpPrimal, 0, &ws); ok {
			t.Fatal("finalizeSparseCut accepted norm-overflowing cut")
		}
		// Unbounded column dropped below cutDropTol must be rejected (col 0 has UB=+Inf).
		if _, ok := finalizeSparseCut(p, []int{0, 1}, []float64{-1e-12, 1.0}, 0.0, lpPrimal, 0, &ws); ok {
			t.Fatal("finalizeSparseCut accepted cut dropping tiny negative coeff on +Inf column")
		}
		// Calling finalizeSparseCut with nil workspace must still succeed.
		cNil, okNil := finalizeSparseCut(p, []int{2, 1}, []float64{1.0, 2.0}, 0.0, lpPrimal, 0, nil)
		if !okNil || !slices.Equal(cNil.ColIdx, []int{1, 2}) {
			t.Fatalf("nil-workspace finalizeSparseCut failed: ok=%v cols=%v", okNil, cNil.ColIdx)
		}
	})

	t.Run("CachedNormCosineSimilarity", func(t *testing.T) {
		m := NewModel("cos_norm", Minimize)
		for j := range 6 {
			_ = m.AddContVar(fmt.Sprintf("x%d", j), 0, 10)
		}
		p := m.Compile()
		lp := []float64{2, 2, 2, 2, 2, 2}
		var ws cutGenWorkspace

		c1, ok1 := finalizeSparseCut(p, []int{0, 2, 4}, []float64{1, 2, 3}, 1.0, lp, 0, &ws)
		c2, ok2 := finalizeSparseCut(p, []int{0, 2, 5}, []float64{2, 1, 4}, 1.0, lp, 0, &ws)
		if !ok1 || !ok2 {
			t.Fatalf("expected valid cuts, got ok1=%v ok2=%v", ok1, ok2)
		}
		cached := cosineSimilarity(c1, c2)
		c1NoNorm, c2NoNorm := c1, c2
		c1NoNorm.Norm = 0
		c2NoNorm.Norm = 0
		uncached := cosineSimilarity(c1NoNorm, c2NoNorm)
		if math.Abs(cached-uncached) > 1e-14 {
			t.Fatalf("cosineSimilarity cached=%g vs uncached=%g", cached, uncached)
		}
		if self := cosineSimilarity(c1, c1); math.Abs(self-1.0) > 1e-14 {
			t.Fatalf("self cosineSimilarity=%g, want 1.0", self)
		}
	})

	t.Run("DensityCapAndZeroAllocOnReject", func(t *testing.T) {
		const n = 200
		m := NewModel("sparse_alloc", Minimize)
		for j := range n {
			_ = m.AddContVar(fmt.Sprintf("x%d", j), 0, 10)
		}
		p := m.Compile()
		lp := make([]float64, n)
		for j := range n {
			lp[j] = 1.0
		}

		var ws cutGenWorkspace
		idx := []int{50, 12, 180, 7, 99}
		val := []float64{1, 1, 1, 1, 1}

		if _, ok := finalizeSparseCut(p, idx, val, 1.0, lp, 4, &ws); ok {
			t.Fatal("expected cut with nnz=5 to be rejected when maxNNZ=4")
		}
		if c, ok := finalizeSparseCut(p, idx, val, 1.0, lp, 5, &ws); !ok || len(c.ColIdx) != 5 {
			t.Fatalf("expected cut with nnz=5 to be accepted when maxNNZ=5, got ok=%v len=%d", ok, len(c.ColIdx))
		}

		// Warm up workspace slices, then verify rejected cuts do zero heap allocations.
		_, _ = finalizeSparseCut(p, idx, val, 100.0, lp, 0, &ws)
		allocs := testing.AllocsPerRun(50, func() {
			if _, ok := finalizeSparseCut(p, idx, val, 100.0, lp, 0, &ws); ok {
				t.Fatal("expected unviolated cut rejection")
			}
			if _, ok := finalizeSparseCut(p, idx, val, 1.0, lp, 3, &ws); ok {
				t.Fatal("expected dense cut rejection")
			}
		})
		if allocs != 0 {
			t.Fatalf("rejected finalizeSparseCut allocated %.1f objects/run, want 0", allocs)
		}
	})
}

func TestPropagateFromColEquivalence(t *testing.T) {
	buildSparseChainMIP := func() *CompiledProblem {
		m := NewModel("prop_sparse_chain", Minimize)
		const T = 6
		y := make([]Var, T)
		x := make([]Var, T)
		for i := range T {
			y[i] = m.AddBinaryVar(fmt.Sprintf("y%d", i))
			x[i] = m.AddContVar(fmt.Sprintf("x%d", i), 0, 20)
		}
		m.SetObjective(NewExpr(y[0].T(1)))
		for i := range T {
			m.AddConstraint(fmt.Sprintf("vub%d", i), NewExpr(x[i].T(1), y[i].T(-20)).Le(0))
		}
		for i := range T - 1 {
			m.AddConstraint(fmt.Sprintf("link%d", i), NewExpr(x[i].T(1), x[i+1].T(-1)).Ge(2))
		}
		m.AddConstraint("tot", NewExpr(y[0].T(1), y[1].T(1), y[2].T(1)).Ge(2))
		return m.Compile()
	}

	buildDenseIntMIP := func() *CompiledProblem {
		m := NewModel("prop_dense_int", Minimize)
		const n = 10
		vars := make([]Var, n)
		for j := range n {
			vars[j] = m.AddIntVar(fmt.Sprintf("z%d", j), 0, 4)
		}
		m.SetObjective(NewExpr(vars[0].T(1)))
		rng := rand.New(rand.NewPCG(7, 13))
		// Construct constraints feasible at witness z_j = 2 so root propagation always succeeds.
		for i := range 8 {
			terms := make([]Term, 0, n)
			witVal := 0.0
			for j := range n {
				coeff := float64(rng.IntN(5) - 2)
				if coeff != 0 {
					terms = append(terms, vars[j].T(coeff))
					witVal += coeff * 2.0
				}
			}
			switch i % 3 {
			case 0:
				m.AddConstraint(fmt.Sprintf("le%d", i), NewExpr(terms...).Le(witVal+2.0))
			case 1:
				m.AddConstraint(fmt.Sprintf("ge%d", i), NewExpr(terms...).Ge(witVal-2.0))
			default:
				m.AddConstraint(fmt.Sprintf("eq%d", i), NewExpr(terms...).Eq(witVal))
			}
		}
		return m.Compile()
	}

	for _, tc := range []struct {
		name string
		p    *CompiledProblem
	}{
		{"sparse_chain", buildSparseChainMIP()},
		{"dense_int", buildDenseIntMIP()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.p
			baseLB := slices.Clone(p.varLB)
			baseUB := slices.Clone(p.varUB)
			feas, conv := propagateBoundsChecked(p, baseLB, baseUB, rootPropPasses)
			if !feas || !conv {
				t.Fatalf("root propagation failed: feas=%v conv=%v", feas, conv)
			}

			queue := make([]int, 0, 2*p.numConstraints)
			inQueue := make([]bool, p.numConstraints)

			for col := range p.numVars {
				span := baseUB[col] - baseLB[col]
				if span < 0.5 {
					continue
				}
				mid := math.Floor(baseLB[col] + 0.5*span)
				if p.varCat[col] == Continuous {
					mid = baseLB[col] + 0.5*span
				}
				for _, tightenUB := range []bool{true, false} {
					lbRef := slices.Clone(baseLB)
					ubRef := slices.Clone(baseUB)
					lbGot := slices.Clone(baseLB)
					ubGot := slices.Clone(baseUB)
					lbMulti := slices.Clone(baseLB)
					ubMulti := slices.Clone(baseUB)

					if tightenUB {
						ubRef[col] = mid
						ubGot[col] = mid
						ubMulti[col] = mid
					} else {
						lbRef[col] = mid + 1.0
						if p.varCat[col] == Continuous {
							lbRef[col] = mid
						}
						if lbRef[col] > ubRef[col] {
							lbRef[col] = ubRef[col]
						}
						lbGot[col] = lbRef[col]
						lbMulti[col] = lbRef[col]
					}

					okRef := propagateBounds(p, lbRef, ubRef, rootPropPasses)
					okGot := propagateFromCol(p, lbGot, ubGot, col, &queue, inQueue)
					for r, in := range inQueue {
						if in {
							t.Fatalf("col %d: stale inQueue[%d]=true after propagateFromCol", col, r)
						}
					}
					okMulti := propagateFromCols(p, lbMulti, ubMulti, []int{col}, &queue, inQueue)
					for r, in := range inQueue {
						if in {
							t.Fatalf("col %d: stale inQueue[%d]=true after propagateFromCols", col, r)
						}
					}

					if okGot != okRef || okMulti != okRef {
						t.Fatalf("col %d tightenUB=%v: feas mismatch got=%v multi=%v ref=%v",
							col, tightenUB, okGot, okMulti, okRef)
					}
					if okRef {
						for j := range p.numVars {
							if math.Abs(lbGot[j]-lbRef[j]) > 1e-9 || math.Abs(ubGot[j]-ubRef[j]) > 1e-9 {
								t.Fatalf("col %d tightenUB=%v var %d: got=[%g,%g] ref=[%g,%g]",
									col, tightenUB, j, lbGot[j], ubGot[j], lbRef[j], ubRef[j])
							}
							if math.Abs(lbMulti[j]-lbRef[j]) > 1e-9 || math.Abs(ubMulti[j]-ubRef[j]) > 1e-9 {
								t.Fatalf("col %d tightenUB=%v var %d: multi=[%g,%g] ref=[%g,%g]",
									col, tightenUB, j, lbMulti[j], ubMulti[j], lbRef[j], ubRef[j])
							}
						}
					}
				}
			}

			// Verify multi-node applyNodeBoundsTracked + infeasibility check.
			root := &searchNode{
				hasDelta: true,
				delta:    boundDelta{col: 0, lb: baseLB[0], ub: baseLB[0]},
			}
			child := &searchNode{
				parent:   root,
				hasDelta: true,
				delta:    boundDelta{col: 0, lb: baseLB[0] + 1, ub: baseUB[0]},
			}
			if baseUB[0] > baseLB[0] {
				lbTest := slices.Clone(baseLB)
				ubTest := slices.Clone(baseUB)
				_, ok := applyNodeBoundsTracked(child, lbTest, ubTest, nil)
				if ok {
					t.Fatal("applyNodeBoundsTracked failed to detect conflicting ancestor bounds")
				}
			}
		})
	}

	t.Run("ContinuousCycleConvergenceBound", func(t *testing.T) {
		m := NewModel("cont_cycle", Minimize)
		x := m.AddContVar("x", 0, 1e6)
		y := m.AddContVar("y", 0, 1e6)
		m.SetObjective(NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("r0", NewExpr(x.T(1), y.T(-0.9999)).Le(0))
		m.AddConstraint("r1", NewExpr(y.T(1), x.T(-0.9999)).Le(0))
		p := m.Compile()

		lb := slices.Clone(p.varLB)
		ub := slices.Clone(p.varUB)
		ub[0] = 1e5
		queue := make([]int, 0, 8)
		inQueue := make([]bool, p.numConstraints)

		if !propagateFromCol(p, lb, ub, 0, &queue, inQueue) {
			t.Fatal("propagateFromCol falsely declared feasible continuous cycle infeasible")
		}
		for r, in := range inQueue {
			if in {
				t.Fatalf("stale inQueue[%d]=true after step-capped propagation", r)
			}
		}
		if ub[0] >= 1e5 || ub[1] >= 1e5 {
			t.Fatalf("expected bounded progress on continuous cycle, got ub=%v", ub)
		}
	})
}

func TestStrongBranchingBasisRestoreAndDomainReduction(t *testing.T) {
	t.Run("ParentBasisRestoration", func(t *testing.T) {
		m := NewModel("sb_basis_restore", Minimize)
		vars := make([]Var, 8)
		for j := range 8 {
			vars[j] = m.AddBinaryVar(fmt.Sprintf("x%d", j))
		}
		m.SetObjective(NewExpr(
			vars[0].T(-7), vars[1].T(-6), vars[2].T(-5), vars[3].T(-8),
			vars[4].T(-4), vars[5].T(-7), vars[6].T(-5), vars[7].T(-6),
		))
		m.AddConstraint("c0", NewExpr(vars[0].T(2), vars[1].T(2), vars[2].T(2), vars[3].T(2)).Le(3))
		m.AddConstraint("c1", NewExpr(vars[2].T(2), vars[3].T(2), vars[4].T(2), vars[5].T(2)).Le(3))
		m.AddConstraint("c2", NewExpr(vars[4].T(2), vars[5].T(2), vars[6].T(2), vars[7].T(2)).Le(3))
		m.AddConstraint("c3", NewExpr(vars[0].T(3), vars[3].T(2), vars[5].T(3), vars[7].T(2)).Le(5))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v, want Optimal", lpRes.Status)
		}

		n := p.numVars
		totM := lp.TotalRows()
		origBasis := slices.Clone(lp.ws.basis[:totM])
		origStatus := slices.Clone(lp.ws.status[:n+totM])

		curLB := slices.Clone(p.varLB)
		curUB := slices.Clone(p.varUB)
		pCosts := make([]pseudoCost, n)
		for _, col := range p.intCols {
			pCosts[col] = pseudoCost{downSum: 0.1, upSum: 0.1}
		}

		iters := 0
		var buf []branchCand
		col, _ := selectBranchVarWithBuf(p, lp, lpRes, curLB, curUB, pCosts, 0, &iters, func() int { return 1000 }, &buf)
		if col < 0 || iters == 0 {
			t.Fatalf("expected strong branching probes to run, got col=%d iters=%d", col, iters)
		}
		if !slices.Equal(lp.ws.basis[:totM], origBasis) {
			t.Fatalf("basis not restored after strong branching: got %v, want %v", lp.ws.basis[:totM], origBasis)
		}
		if !slices.Equal(lp.ws.status[:n+totM], origStatus) {
			t.Fatalf("status not restored after strong branching: got %v, want %v", lp.ws.status[:n+totM], origStatus)
		}
	})

	t.Run("DomainReductionOneBranchInfeasible", func(t *testing.T) {
		// x0 and x1 are fractional (0.5, 0.5) at the LP optimum.
		// Fixing x0 = 0 forces x2 >= 1.5, violating x2 <= 1, so down-branch is LP-infeasible.
		m := NewModel("sb_domain_red", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		x2 := m.AddContVar("x2", 0, 1)
		m.SetObjective(NewExpr(x0.T(10), x1.T(-1), x2.T(0)))
		m.AddConstraint("r0", NewExpr(x0.T(1), x2.T(1)).Ge(1.5))
		m.AddConstraint("r1", NewExpr(x1.T(2)).Le(1.0))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("LP status=%v, want Optimal", lpRes.Status)
		}
		if math.Abs(lpRes.Primal[0]-0.5) > 1e-6 || math.Abs(lpRes.Primal[1]-0.5) > 1e-6 {
			t.Fatalf("unexpected LP primal %v, want x0=0.5, x1=0.5", lpRes.Primal)
		}

		curLB := slices.Clone(p.varLB)
		curUB := slices.Clone(p.varUB)
		pCosts := make([]pseudoCost, p.numVars)
		// Bias candidate ordering so x0 is probed first.
		pCosts[0] = pseudoCost{downSum: 5.0, upSum: 5.0}
		pCosts[1] = pseudoCost{downSum: 0.1, upSum: 0.1}

		iters := 0
		var buf []branchCand
		out := selectBranchVarDetailed(p, lp, lpRes, curLB, curUB, pCosts, 0, &iters, func() int { return 1000 }, &buf, true)
		if out.pruned {
			t.Fatal("expected domain reduction, not full node prune")
		}
		if len(out.fixedDeltas) != 1 || out.fixedDeltas[0].col != 0 || out.fixedDeltas[0].lb != 1.0 {
			t.Fatalf("fixedDeltas=%+v, want col=0 lb=1", out.fixedDeltas)
		}
		if curLB[0] != 1.0 {
			t.Fatalf("curLB[0]=%g, want 1.0", curLB[0])
		}

		res, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(false), WithHeuristics(false))
		if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-10.0) > 1e-6 {
			t.Fatalf("SolveCompiled = (%v, obj=%g, err=%v), want Optimal 10.0", res.Status, res.Objective, err)
		}
	})

	t.Run("ImmediatePruneBothBranchesInfeasible", func(t *testing.T) {
		// 2*x0 == 1 with x0 binary makes both x0 <= 0 and x0 >= 1 LP-infeasible,
		// while x1 = 0.5 ensures multiple fractional candidates trigger strong branching.
		m := NewModel("sb_prune_both", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		m.SetObjective(NewExpr(x0.T(1), x1.T(-1)))
		m.AddConstraint("eq_half", NewExpr(x0.T(2)).Eq(1))
		m.AddConstraint("le_half", NewExpr(x1.T(2)).Le(1))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("LP status=%v, want Optimal", lpRes.Status)
		}

		curLB := slices.Clone(p.varLB)
		curUB := slices.Clone(p.varUB)
		pCosts := make([]pseudoCost, p.numVars)
		pCosts[0] = pseudoCost{downSum: 5.0, upSum: 5.0}
		pCosts[1] = pseudoCost{downSum: 0.1, upSum: 0.1}

		iters := 0
		var buf []branchCand
		out := selectBranchVarDetailed(p, lp, lpRes, curLB, curUB, pCosts, 0, &iters, func() int { return 1000 }, &buf, true)
		if !out.pruned {
			t.Fatalf("expected pruned=true when both branches are LP-infeasible, got %+v", out)
		}

		// End-to-end solve with presolve/cuts/heuristics disabled must detect infeasibility via strong branching.
		res, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(false), WithHeuristics(false))
		if err != nil || res.Status != StatusInfeasible {
			t.Fatalf("SolveCompiled = (%v, %v), want StatusInfeasible", res.Status, err)
		}
	})

	t.Run("NoDomainReductionOnIterLimit", func(t *testing.T) {
		// When probe budget is exhausted (0 iterations left), strong branching must not tighten or prune.
		m := NewModel("sb_iter_limit", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		x2 := m.AddContVar("x2", 0, 1)
		m.SetObjective(NewExpr(x0.T(10), x1.T(-1), x2.T(0)))
		m.AddConstraint("r0", NewExpr(x0.T(1), x2.T(1)).Ge(1.5))
		m.AddConstraint("r1", NewExpr(x1.T(2)).Le(1.0))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()

		curLB := slices.Clone(p.varLB)
		curUB := slices.Clone(p.varUB)
		pCosts := make([]pseudoCost, p.numVars)
		iters := 0
		out := selectBranchVarDetailed(p, lp, lpRes, curLB, curUB, pCosts, 0, &iters, func() int { return 0 }, nil, true)
		if out.pruned || len(out.fixedDeltas) != 0 || curLB[0] != 0 || out.bestCol < 0 {
			t.Fatalf("unexpected modification with zero iter budget: out=%+v curLB=%v", out, curLB)
		}
	})
}

func TestCutDensityCapAndStaticCache(t *testing.T) {
	t.Run("MaxCutNNZAndDensityPenalty", func(t *testing.T) {
		if got := maxCutNNZ(33); got != 33 {
			t.Fatalf("maxCutNNZ(33) = %d, want 33", got)
		}
		if got := maxCutNNZ(89); got != 89 {
			t.Fatalf("maxCutNNZ(89) = %d, want 89", got)
		}
		if got := maxCutNNZ(201); got != 50 {
			t.Fatalf("maxCutNNZ(201) = %d, want 50", got)
		}
		if got := maxCutNNZ(300); got != 75 {
			t.Fatalf("maxCutNNZ(300) = %d, want 75", got)
		}
		if got := maxCutNNZ(548); got != 100 {
			t.Fatalf("maxCutNNZ(548) = %d, want 100", got)
		}

		sparseCut := cutPlane{ColIdx: make([]int, 10), Efficacy: 0.5}
		denseCut := cutPlane{ColIdx: make([]int, 60), Efficacy: 0.5}
		if cutSelectionScore(sparseCut, 200) <= cutSelectionScore(denseCut, 200) {
			t.Fatalf("expected sparse cut score (%g) > dense cut score (%g)",
				cutSelectionScore(sparseCut, 200), cutSelectionScore(denseCut, 200))
		}
		// Small models (n <= cutMaxNNZSmallN) must preserve pure efficacy ordering.
		if cutSelectionScore(denseCut, 80) != denseCut.Efficacy {
			t.Fatalf("small-model score altered: got %g, want %g", cutSelectionScore(denseCut, 80), denseCut.Efficacy)
		}

		// Build a 150-variable model with a 70-variable MIR row; MIR cuts must respect maxCutNNZ(150)=50.
		const nVars = 150
		m := NewModel("dense_mir_cap", Minimize)
		vs := make([]Var, nVars)
		for j := range nVars {
			vs[j] = m.AddBinaryVar(fmt.Sprintf("x%d", j))
		}
		terms := make([]Term, 70)
		for j := range 70 {
			terms[j] = vs[j].T(2.0)
		}
		m.AddConstraint("dense_row", NewExpr(terms...).Ge(3.0))
		p := m.Compile()
		lp := make([]float64, nVars)
		for j := range 70 {
			lp[j] = 3.0 / 140.0
		}
		mirCuts := separateRowMIRCuts(p, lp)
		limit := maxCutNNZ(nVars)
		for _, c := range mirCuts {
			if len(c.ColIdx) > limit {
				t.Fatalf("separateRowMIRCuts emitted dense cut with nnz=%d > limit=%d", len(c.ColIdx), limit)
			}
		}
	})

	t.Run("StaticCacheReuseAndFlowVUBParity", func(t *testing.T) {
		// Combine a 4-period lot-sizing flow chain, a disaggregated VUB row,
		// a 4-binary knapsack row, and a mixed-integer MIR row.
		m := NewModel("static_cache_flow_vub", Minimize)
		const T = 4
		y := make([]Var, T)
		pVar := make([]Var, T)
		inv := make([]Var, T)
		for tIdx := range T {
			y[tIdx] = m.AddBinaryVar(fmt.Sprintf("y%d", tIdx))
			pVar[tIdx] = m.AddContVar(fmt.Sprintf("p%d", tIdx), 0, 50)
			inv[tIdx] = m.AddContVar(fmt.Sprintf("s%d", tIdx), 0, 50)
		}
		z0 := m.AddBinaryVar("z0")
		z1 := m.AddContVar("z1", 0, 6)
		z2 := m.AddContVar("z2", 0, 6)
		m.SetObjective(NewExpr(y[0].T(10), y[1].T(10), y[2].T(10), y[3].T(10)))

		// VUBs p_t - 40*y_t <= 0 and flow balance s_{t-1} + p_t - s_t == 10.
		for tIdx := range T {
			m.AddConstraint(fmt.Sprintf("vub%d", tIdx), NewExpr(pVar[tIdx].T(1), y[tIdx].T(-40)).Le(0))
			if tIdx == 0 {
				m.AddConstraint("bal0", NewExpr(pVar[0].T(1), inv[0].T(-1)).Eq(10))
			} else {
				m.AddConstraint(fmt.Sprintf("bal%d", tIdx), NewExpr(inv[tIdx-1].T(1), pVar[tIdx].T(1), inv[tIdx].T(-1)).Eq(10))
			}
		}
		// Disaggregated VUB row: z1 + 2*z2 - 12*z0 <= 0.
		m.AddConstraint("disagg_vub", NewExpr(z1.T(1), z2.T(2), z0.T(-12)).Le(0))
		// Knapsack cover row over y0..y3.
		m.AddConstraint("knap", NewExpr(y[0].T(3), y[1].T(3), y[2].T(3), y[3].T(3)).Le(5))

		p := m.Compile()

		var cachedWS cutGenWorkspace
		lp1 := make([]float64, p.numVars)
		// Fractional LP point violating flow (l, S), disaggregated VUB, and cover cuts.
		for tIdx := range T {
			lp1[y[tIdx].Index()] = 0.25
			lp1[pVar[tIdx].Index()] = 10.0
			lp1[inv[tIdx].Index()] = 0.0
		}
		lp1[z0.Index()] = 0.2
		lp1[z1.Index()] = 4.0
		lp1[z2.Index()] = 1.0

		cuts1 := separateFlowAndVUBCuts(p, lp1, &cachedWS)
		if len(cuts1) == 0 {
			t.Fatal("expected flow and disaggregated VUB cuts")
		}
		cache := &cachedWS.staticCache
		if cache.prob != p || len(cache.flowChains) == 0 || len(cache.disaggVUBs) == 0 || len(cache.coverRows) == 0 || len(cache.mirRows) == 0 {
			t.Fatalf("static cache incomplete: chains=%d disaggVUBs=%d coverRows=%d mirRows=%d",
				len(cache.flowChains), len(cache.disaggVUBs), len(cache.coverRows), len(cache.mirRows))
		}
		chainsPtr := &cache.flowChains[0]

		// Second call with a different LP point must reuse the cached flowChains slice and match an uncached call.
		lp2 := slices.Clone(lp1)
		lp2[y[1].Index()] = 0.5
		lp2[z0.Index()] = 0.3
		cutsCached := separateFlowAndVUBCuts(p, lp2, &cachedWS)
		cutsFresh := separateFlowAndVUBCuts(p, lp2, nil)
		if &cachedWS.staticCache.flowChains[0] != chainsPtr {
			t.Fatal("static flowChains cache was rebuilt across rounds")
		}
		if len(cutsCached) != len(cutsFresh) {
			t.Fatalf("cached flow/VUB cut count=%d, fresh=%d", len(cutsCached), len(cutsFresh))
		}
		for k := range cutsFresh {
			if !slices.Equal(cutsCached[k].ColIdx, cutsFresh[k].ColIdx) ||
				!slices.Equal(cutsCached[k].Val, cutsFresh[k].Val) ||
				cutsCached[k].RHS != cutsFresh[k].RHS {
				t.Fatalf("cut %d mismatch between cached and fresh workspace", k)
			}
		}

		// Verify knapsack cover cuts with cached coverRows match uncached calls.
		lpCover := make([]float64, p.numVars)
		lpCover[y[0].Index()] = 0.8
		lpCover[y[1].Index()] = 0.8
		lpCover[y[2].Index()] = 0.05
		lpCover[y[3].Index()] = 0.0
		covCached := separateKnapsackCoverCuts(p, lpCover, &cachedWS)
		covFresh := separateKnapsackCoverCuts(p, lpCover, nil)
		if len(covCached) == 0 || len(covCached) != len(covFresh) {
			t.Fatalf("cover cuts cached=%d fresh=%d", len(covCached), len(covFresh))
		}
	})

	t.Run("InTreeCutAgingAndPurging", func(t *testing.T) {
		m := NewModel("intree_cut_aging", Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(NewExpr(x0.T(-3), x1.T(-2)))
		m.AddConstraint("cap", NewExpr(x0.T(1), x1.T(1)).Le(6))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		if res := lp.Solve(); res.Status != StatusOptimal {
			t.Fatalf("initial LP status=%v", res.Status)
		}

		// Add 2 tight cuts and 36 loose cuts (> inTreeCutPurgeMinCuts).
		lp.AddCut([]int{0}, []float64{1.0}, 4.0)
		lp.AddCut([]int{1}, []float64{1.0}, 2.0)
		for k := range 36 {
			lp.AddCut([]int{0, 1}, []float64{1.0, 1.0}, 10.0+float64(k))
		}
		if res := lp.Solve(); res.Status != StatusOptimal || math.Abs(res.Objective - -16.0) > 1e-8 {
			t.Fatalf("cut LP status=%v obj=%g, want Optimal -16", res.Status, res.Objective)
		}

		eng := &babEngine{p: p, lpSolver: lp}

		// Before inTreeCutPurgePeriod, no cuts are purged.
		for range inTreeCutPurgePeriod - 1 {
			eng.updateCutAgesAndMaybePurge(true)
		}
		if lp.NumCuts() != 38 {
			t.Fatalf("premature purge before period: NumCuts=%d, want 38", lp.NumCuts())
		}

		// With allowPurge=false (strong branching guard), still no purge at period.
		eng.updateCutAgesAndMaybePurge(false)
		if lp.NumCuts() != 38 {
			t.Fatalf("purged while allowPurge=false: NumCuts=%d, want 38", lp.NumCuts())
		}

		// With allowPurge=true, the 36 aged loose cuts are purged and the 2 tight cuts remain.
		eng.updateCutAgesAndMaybePurge(true)
		if lp.NumCuts() != 2 {
			t.Fatalf("after aged purge NumCuts=%d, want 2", lp.NumCuts())
		}
		if res := lp.Solve(); res.Status != StatusOptimal || math.Abs(res.Objective - -16.0) > 1e-8 {
			t.Fatalf("post-purge LP status=%v obj=%g, want Optimal -16", res.Status, res.Objective)
		}
	})

	t.Run("GMISparseWorkspaceAndRootCutRollback", func(t *testing.T) {
		m := NewModel("gmi_ws_rollback", Minimize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddIntVar("x1", 0, 10)
		x2 := m.AddIntVar("x2", 0, 10)
		m.SetObjective(NewExpr(x0.T(-5), x1.T(-4), x2.T(-3)))
		m.AddConstraint("c0", NewExpr(x0.T(2), x1.T(3), x2.T(2)).Le(7))
		m.AddConstraint("c1", NewExpr(x0.T(3), x1.T(2), x2.T(4)).Le(7))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		rootRes := lp.Solve()
		if rootRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v, want Optimal", rootRes.Status)
		}

		// Verify separateGMICuts with shared cutGenWorkspace matches a fresh workspace.
		var ws cutGenWorkspace
		gmiA := separateGMICuts(lp, rootRes.Primal, &ws)
		gmiB := separateGMICuts(lp, rootRes.Primal, nil)
		if len(gmiA) == 0 || len(gmiA) != len(gmiB) {
			t.Fatalf("GMI cut count mismatch: ws=%d fresh=%d", len(gmiA), len(gmiB))
		}
		for k := range gmiA {
			if !slices.Equal(gmiA[k].ColIdx, gmiB[k].ColIdx) ||
				!slices.Equal(gmiA[k].Val, gmiB[k].Val) ||
				gmiA[k].RHS != gmiB[k].RHS {
				t.Fatalf("GMI cut %d mismatch between shared and fresh workspace", k)
			}
		}

		// Verify runRootCutLoop rolls back cleanly when MaxIter is exhausted mid-cut-loop.
		resFull, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithHeuristics(false))
		if err != nil || resFull.Status != StatusOptimal {
			t.Fatalf("full solve = (%v, %v), want Optimal", resFull.Status, err)
		}
		resLim, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithHeuristics(false), WithMaxIter(rootRes.Iterations+1))
		if err != nil {
			t.Fatalf("limited solve err=%v", err)
		}
		if resLim.Status != StatusIterationLimit && resLim.Status != StatusOptimal {
			t.Fatalf("limited solve status=%v, want IterationLimit or Optimal", resLim.Status)
		}
		if resLim.BestBound > resFull.Objective+1e-6 {
			t.Fatalf("invalid lower bound after cut rollback: bestBound=%g > opt=%g", resLim.BestBound, resFull.Objective)
		}
	})
}
