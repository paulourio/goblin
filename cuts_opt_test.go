package goblin

import (
	"context"
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

			denseRHS := rhs
			seenDup := make([]bool, n)
			dupAcc := make([]float64, n)
			for k, col := range idx {
				v := val[k]
				if !seenDup[col] {
					seenDup[col] = true
					dupAcc[col] = v
					continue
				}
				sum, rem := twoSum(dupAcc[col], v)
				dupAcc[col] = sum
				if rem != 0 {
					bnd := p.varUB[col]
					if rem > 0 {
						bnd = p.varLB[col]
					}
					if bnd != 0 {
						denseRHS = nextUp(denseRHS - nextDown(rem*bnd))
					}
				}
			}

			refCut, refOK := finalizeCut(p, dense, denseRHS, lpPrimal)
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

func assertCutsRespectFeasiblePoints(t *testing.T, label string, cuts []cutPlane, points [][]float64) {
	t.Helper()
	for ci, cut := range cuts {
		for pi, x := range points {
			lhs := 0.0
			for k, col := range cut.ColIdx {
				lhs += cut.Val[k] * x[col]
			}
			if lhs > cut.RHS+1e-7 {
				t.Fatalf("%s: cut %d invalidated feasible point %d: lhs=%.10g > rhs=%.10g",
					label, ci, pi, lhs, cut.RHS)
			}
		}
	}
}

func TestMultiRowAggregatedCutsAndInTreeGMI(t *testing.T) {
	t.Parallel()

	t.Run("CapacitatedLotSizingMultiRowMIR", func(t *testing.T) {
		t.Parallel()
		// 3-period capacitated lot-sizing: d = (10, 10, 10), sum = 30 > C = 25,
		// so at least 2 setups are required even though single-row MIR and uncapacitated (l,S)
		// permit y_0 = 1, y_1 = 0, y_2 = 0.2 (sum y = 1.2) or distributed fractional setups.
		m := NewModel("cap_lot_agg", Minimize)
		const T = 3
		const capBatch = 25.0
		y := make([]Var, T)
		pVar := make([]Var, T)
		inv := make([]Var, T)
		var obj []Term
		for idx := range T {
			y[idx] = m.AddBinaryVar(fmt.Sprintf("y%d", idx))
			pVar[idx] = m.AddIntVar(fmt.Sprintf("p%d", idx), 0, capBatch)
			inv[idx] = m.AddContVar(fmt.Sprintf("s%d", idx), 0, 100)
			obj = append(obj, y[idx].T(100), pVar[idx].T(1), inv[idx].T(0.5))
		}
		m.SetObjective(Expr{Terms: obj})
		for idx := range T {
			if idx == 0 {
				m.AddConstraint("bal0", NewExpr(pVar[0].T(1), inv[0].T(-1)).Eq(10))
			} else {
				m.AddConstraint(fmt.Sprintf("bal%d", idx), NewExpr(inv[idx-1].T(1), pVar[idx].T(1), inv[idx].T(-1)).Eq(10))
			}
			m.AddConstraint(fmt.Sprintf("link%d", idx), NewExpr(pVar[idx].T(1), y[idx].T(-capBatch)).Le(0))
		}
		prob := m.Compile()

		lp := NewLPSolver(prob, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v, want Optimal", lpRes.Status)
		}

		var ws cutGenWorkspace
		aggCuts := separateAggregatedMIRCuts(lp, lpRes.Primal, &ws)
		if len(aggCuts) == 0 {
			t.Fatal("expected capacitated lot-sizing multi-row MIR cuts, got none")
		}

		// Enumerate all feasible setup combinations y in {0,1}^3, setting canonical valid schedules.
		var feasPts [][]float64
		for mask := range 1 << T {
			y0 := float64(mask & 1)
			y1 := float64((mask >> 1) & 1)
			y2 := float64((mask >> 2) & 1)
			if y0 == 0 || y0+y1+y2 < 2 {
				continue
			}
			for p0 := 10; p0 <= 25; p0++ {
				if p0 > 0 && y0 == 0 {
					continue
				}
				s0 := p0 - 10
				for p1 := max(0, 10-s0); p1 <= 25*int(y1); p1++ {
					s1 := s0 + p1 - 10
					p2 := 10 - s1
					if p2 < 0 || p2 > 25*int(y2) {
						continue
					}
					pt := make([]float64, prob.numVars)
					pt[y[0].Index()] = y0
					pt[y[1].Index()] = y1
					pt[y[2].Index()] = y2
					pt[pVar[0].Index()] = float64(p0)
					pt[pVar[1].Index()] = float64(p1)
					pt[pVar[2].Index()] = float64(p2)
					pt[inv[0].Index()] = float64(s0)
					pt[inv[1].Index()] = float64(s1)
					feasPts = append(feasPts, pt)
				}
			}
		}
		if len(feasPts) == 0 {
			t.Fatal("failed to construct feasible test points")
		}
		assertCutsRespectFeasiblePoints(t, "cap_lot_agg", aggCuts, feasPts)

		// Verify adding aggregated cuts raises LP bound.
		addDynamicCutPlanes(lp, aggCuts)
		cutRes := lp.Solve()
		if cutRes.Status != StatusOptimal || cutRes.Objective <= lpRes.Objective+1.0 {
			t.Fatalf("expected aggregated MIR cuts to raise LP bound from %g, got status=%v obj=%g",
				lpRes.Objective, cutRes.Status, cutRes.Objective)
		}
	})

	t.Run("PairwiseSurrogateAndVUBCapacitySum", func(t *testing.T) {
		t.Parallel()
		// Model combining tight VUB capacity constraints + partitioning rows (bin packing),
		// AND two coupled inequality knapsack rows needing surrogate aggregation.
		m := NewModel("vub_pair_agg", Minimize)
		b0 := m.AddBinaryVar("b0")
		b1 := m.AddBinaryVar("b1")
		b2 := m.AddBinaryVar("b2")
		x := make([][]Var, 3)
		var obj []Term
		obj = append(obj, b0.T(10), b1.T(10), b2.T(10))
		for b := range 3 {
			x[b] = make([]Var, 3)
			for i := range 3 {
				x[b][i] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", b, i))
				obj = append(obj, x[b][i].T(0.01*float64(b+1)))
			}
		}
		// Tight-coupling auxiliary binaries with pairwise knapsack inequalities.
		z0 := m.AddBinaryVar("z0")
		z1 := m.AddBinaryVar("z1")
		z2 := m.AddBinaryVar("z2")
		obj = append(obj, z0.T(1), z1.T(1), z2.T(1))
		m.SetObjective(Expr{Terms: obj})

		weights := []float64{6, 6, 6} // total = 18 > 14 = cap, needs >= 2 bins; LP gives 1.8 bins
		for b := range 3 {
			m.AddConstraint(fmt.Sprintf("cap_%d", b), NewExpr(
				x[b][0].T(weights[0]), x[b][1].T(weights[1]), x[b][2].T(weights[2]), b0.T(0),
			).Le(0))
			_ = b
		}
		// Rebuild cleanly without dummy b0 term on wrong bin:
		mClean := NewModel("vub_pair_clean", Minimize)
		bc := make([]Var, 3)
		xc := make([][]Var, 3)
		var oc []Term
		for b := range 3 {
			bc[b] = mClean.AddBinaryVar(fmt.Sprintf("b%d", b))
			oc = append(oc, bc[b].T(10))
			xc[b] = make([]Var, 3)
			for i := range 3 {
				xc[b][i] = mClean.AddBinaryVar(fmt.Sprintf("x_%d_%d", b, i))
				oc = append(oc, xc[b][i].T(0.01*float64(b+1)))
			}
		}
		zc0 := mClean.AddBinaryVar("z0")
		zc1 := mClean.AddBinaryVar("z1")
		zc2 := mClean.AddBinaryVar("z2")
		zc3 := mClean.AddBinaryVar("z3")
		oc = append(oc, zc0.T(-3), zc1.T(-3), zc2.T(-2), zc3.T(-2))
		mClean.SetObjective(Expr{Terms: oc})

		for b := range 3 {
			mClean.AddConstraint(fmt.Sprintf("bincap_%d", b), NewExpr(
				xc[b][0].T(6), xc[b][1].T(6), xc[b][2].T(6), bc[b].T(-14),
			).Le(0))
		}
		for i := range 3 {
			mClean.AddConstraint(fmt.Sprintf("item_%d", i), NewExpr(
				xc[0][i].T(1), xc[1][i].T(1), xc[2][i].T(1),
			).Eq(1))
		}
		// Surrogate knapsack pair:
		// 2*z0 + 2*z1 + z2 <= 3 and 2*z0 + z2 + 2*z3 <= 3 (sum: 4*z0 + 2*z1 + 2*z2 + 2*z3 <= 6)
		mClean.AddConstraint("kn1", NewExpr(zc0.T(2), zc1.T(2), zc2.T(1)).Le(3))
		mClean.AddConstraint("kn2", NewExpr(zc0.T(2), zc2.T(1), zc3.T(2)).Le(3))
		prob := mClean.Compile()

		lp := NewLPSolver(prob, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", lpRes.Status)
		}

		var ws cutGenWorkspace
		aggCuts := separateAggregatedMIRCuts(lp, lpRes.Primal, &ws)
		if len(aggCuts) == 0 {
			t.Fatal("expected aggregated VUB-capacity and pairwise surrogate cuts")
		}

		// Known optimal integer feasible point: bin 0 holds items 0,1; bin 1 holds item 2; z = (0,1,1,1) -> sum z in kn1/kn2
		pt := make([]float64, prob.numVars)
		pt[bc[0].Index()] = 1
		pt[bc[1].Index()] = 1
		pt[xc[0][0].Index()] = 1
		pt[xc[0][1].Index()] = 1
		pt[xc[1][2].Index()] = 1
		pt[zc1.Index()] = 1
		pt[zc2.Index()] = 1
		pt[zc3.Index()] = 1
		assertCutsRespectFeasiblePoints(t, "vub_pair_clean", aggCuts, [][]float64{pt})
	})

	t.Run("InTreeGlobalOnlyGMIRejectsInteriorFixedInt", func(t *testing.T) {
		t.Parallel()
		m := NewModel("intree_global_gmi", Minimize)
		y := m.AddBinaryVar("y")      // globally [0, 1]
		k := m.AddIntVar("k", 0, 5)   // can be branched to interior value [2, 2]
		z := m.AddIntVar("z", 0, 5)   // fractional basic integer
		w := m.AddContVar("w", 0, 10) // continuous
		m.SetObjective(NewExpr(z.T(-5), w.T(-1), k.T(0), y.T(1)))
		m.AddConstraint("c0", NewExpr(z.T(2), k.T(1), w.T(1), y.T(1)).Eq(5))
		m.AddConstraint("c1", NewExpr(w.T(1), z.T(-1)).Ge(0))
		prob := m.Compile()

		// Enumerate all globally feasible integer solutions (y in {0,1}, k in 0..5, z in 0..5, w = 5 - 2z - k - y >= z).
		var globalFeas [][]float64
		for yVal := range 2 {
			for kVal := range 6 {
				for zVal := range 6 {
					wVal := 5 - 2*zVal - kVal - yVal
					if wVal >= zVal && wVal <= 10 {
						globalFeas = append(globalFeas, []float64{float64(yVal), float64(kVal), float64(zVal), float64(wVal)})
					}
				}
			}
		}

		// Case 1: Branch binary y = 0 (global lower bound) and k = 2 (strictly interior to [0,5]).
		// Global-only GMI must NOT emit a cut that relies on k == 2 and violates global solutions with k != 2.
		lp := NewLPSolver(prob, nil)
		defer lp.Close()
		lb1 := []float64{0, 2, 0, 0}
		ub1 := []float64{0, 2, 5, 10}
		lp.SetStructuralBounds(lb1, ub1)
		res1 := lp.Solve()
		if res1.Status != StatusOptimal {
			t.Fatalf("interior node LP status=%v", res1.Status)
		}
		var ws cutGenWorkspace
		cuts1 := separateGMICutsEx(lp, res1.Primal, &ws, true)
		assertCutsRespectFeasiblePoints(t, "interior_fixed_int_gmi", cuts1, globalFeas)

		// Case 2: Branch y = 1 (at global upper bound) while k stays at global lower bound [0, 0] or free [0, 5].
		// Here k == 0 is at global lower bound and y == 1 is at global upper bound, so global-only GMI
		// safely emits a cut that cuts off node LP while remaining globally valid for all solutions.
		lb2 := []float64{1, 0, 0, 0}
		ub2 := []float64{1, 0, 5, 10}
		lp.SetStructuralBounds(lb2, ub2)
		res2 := lp.Solve()
		if res2.Status != StatusOptimal {
			t.Fatalf("global-bound node LP status=%v", res2.Status)
		}
		cuts2 := separateGMICutsEx(lp, res2.Primal, &ws, true)
		if len(cuts2) == 0 {
			t.Fatal("expected globally valid in-tree GMI cut when fixed nonbasics sit at global bounds")
		}
		assertCutsRespectFeasiblePoints(t, "global_bound_gmi", cuts2, globalFeas)
	})
}

func TestStrongBranchingBoundCutoffDomainReduction(t *testing.T) {
	ctx := context.Background()

	// 1. Direct unit test of selectBranchVarCore bound-cutoff domain reduction with
	// non-zero objConstant and non-zero RelGapTol:
	//   Minimize x0 + 5*x1 + 0*x2 + 0.5*x3 + 100 (objConstant=100)
	//   s.t. 2*x0 + x1 >= 1, 2*x2 + x3 >= 1, x0..x3 in {0, 1}
	// Root LP optimal: x0 = 0.5, x1 = 0, x2 = 0.5, x3 = 0 -> user obj = 100.5.
	// With incumbent bestObjMin = 105.0 and pruneRelTol = 0.02:
	//   - cands = [x0, x2]; x0 sorts first because |obj[x0]|=1 > |obj[x2]|=0.
	//   - branching down (x0 = 0): forces x1 >= 1 -> downBound(unshifted) = 5.0 -> shifted = 105.0 >= 105.0 (cut off!)
	//   - branching up   (x0 = 1): x1 = 0 -> upBound(unshifted) = 1.0 -> shifted = 101.0 (not cut off!)
	// selectBranchVarCore should fix x0 = 1 and report prunedBound == 105.0 (the shifted down bound).
	m := NewModel("sb_bound_cutoff_unit", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddBinaryVar("x1")
	x2 := m.AddBinaryVar("x2")
	x3 := m.AddBinaryVar("x3")
	m.SetObjective(NewExpr(x0.T(1), x1.T(5), x2.T(0), x3.T(0.5)).AddConstant(100))
	m.AddConstraint("c1", NewExpr(x0.T(2), x1.T(1)).Ge(1))
	m.AddConstraint("c2", NewExpr(x2.T(2), x3.T(1)).Ge(1))
	p := m.Compile()

	lp := NewLPSolver(p, nil)
	defer lp.Close()
	lpRes := lp.Solve()
	if lpRes.Status != StatusOptimal {
		t.Fatalf("root LP status=%v, want Optimal", lpRes.Status)
	}
	curLB := slices.Clone(p.varLB)
	curUB := slices.Clone(p.varUB)
	pCosts := make([]pseudoCost, p.numVars)
	totalIters := 0
	var cands []branchCand
	out := selectBranchVarCore(
		p, lp, lpRes, curLB, curUB, pCosts, 0,
		&totalIters, func() int { return 1000 }, &cands,
		true,
		sbConfig{
			hasIncumbent: true,
			bestObjMin:   105.0,
			pruneRelTol:  0.02,
			absGapTol:    1e-7,
			objConstMin:  p.objConstMin(),
		},
	)
	if out.pruned {
		t.Fatalf("expected only down branch cut off and x0 fixed to 1, got pruned=true")
	}
	if len(out.fixedDeltas) != 1 || out.fixedDeltas[0].col != 0 || out.fixedDeltas[0].lb != 1 {
		t.Fatalf("expected fixedDeltas=[{col:0 lb:1}], got %+v", out.fixedDeltas)
	}
	if math.Abs(out.prunedBound-105.0) > 1e-6 {
		t.Fatalf("expected shifted prunedBound=105.0, got %.10g", out.prunedBound)
	}
	if totalIters == 0 {
		t.Fatalf("expected strong-branching probe iterations to be recorded in totalIters")
	}

	// 2. End-to-end check with non-zero RelGapTol and objConstant: ensure BestBound
	// remains a valid lower bound <= true optimum (never overstated by pruned subtrees).
	m2 := NewModel("sb_cutoff_gap_e2e", Minimize)
	vars := make([]Var, 6)
	objT := make([]Term, 6)
	capT := make([]Term, 6)
	for i := range vars {
		vars[i] = m2.AddBinaryVar(fmt.Sprintf("v_%d", i))
		objT[i] = vars[i].T(float64(i + 2))
		capT[i] = vars[i].T(2)
	}
	m2.SetObjective(NewExpr(objT...).AddConstant(50))
	m2.AddConstraint("cover", NewExpr(capT...).Ge(5)) // must pick >= 3 items -> opt = 50 + 2 + 3 + 4 = 59
	resExact, err := m2.Solve(ctx)
	if err != nil || resExact.Status != StatusOptimal || math.Abs(resExact.Objective-59.0) > 1e-6 {
		t.Fatalf("exact solve got status=%v obj=%.6g err=%v, want 59", resExact.Status, resExact.Objective, err)
	}
	resGap, err := m2.Solve(ctx, WithRelGapTol(0.05), WithCuts(false))
	if err != nil || resGap.Status != StatusOptimal {
		t.Fatalf("gap solve got status=%v err=%v", resGap.Status, err)
	}
	if resGap.BestBound > 59.0+1e-6 {
		t.Fatalf("BestBound=%.10g overstated true optimum 59.0 under RelGapTol", resGap.BestBound)
	}
	if resGap.BestBound > resGap.Objective+1e-6 {
		t.Fatalf("BestBound=%.10g > Objective=%.10g", resGap.BestBound, resGap.Objective)
	}
}

func TestNextUpDownBitExact(t *testing.T) {
	posInf := math.Inf(1)
	negInf := math.Inf(-1)
	edgeCases := []float64{
		0.0,
		math.Copysign(0.0, -1.0),
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		math.Float64frombits(0x000fffffffffffff), // largest subnormal
		-math.Float64frombits(0x000fffffffffffff),
		math.Float64frombits(0x0010000000000000), // smallest normal
		-math.Float64frombits(0x0010000000000000),
		1.0,
		-1.0,
		math.Float64frombits(0x3ff0000000000001),
		math.Float64frombits(0x3fefffffffffffff),
		math.Float64frombits(0xbff0000000000001),
		math.Float64frombits(0xbfefffffffffffff),
		math.MaxFloat64,
		-math.MaxFloat64,
		posInf,
		negInf,
		math.NaN(),
	}
	checkVal := func(x float64) {
		gotUp := nextUp(x)
		wantUp := math.Nextafter(x, posInf)
		if math.IsNaN(wantUp) {
			if !math.IsNaN(gotUp) {
				t.Fatalf("nextUp(%v) = %v, want NaN", x, gotUp)
			}
		} else if math.Float64bits(gotUp) != math.Float64bits(wantUp) {
			t.Fatalf("nextUp(%x) = %x, want %x", math.Float64bits(x), math.Float64bits(gotUp), math.Float64bits(wantUp))
		}
		gotDown := nextDown(x)
		wantDown := math.Nextafter(x, negInf)
		if math.IsNaN(wantDown) {
			if !math.IsNaN(gotDown) {
				t.Fatalf("nextDown(%v) = %v, want NaN", x, gotDown)
			}
		} else if math.Float64bits(gotDown) != math.Float64bits(wantDown) {
			t.Fatalf("nextDown(%x) = %x, want %x", math.Float64bits(x), math.Float64bits(gotDown), math.Float64bits(wantDown))
		}
	}
	for _, x := range edgeCases {
		checkVal(x)
	}
	rng := rand.New(rand.NewPCG(12345, 67890))
	for range 10000 {
		bits := rng.Uint64()
		checkVal(math.Float64frombits(bits))
	}
}
