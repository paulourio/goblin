package goblin

import (
	"fmt"
	"math"
	"math/big"
	"testing"
)

func mustTrack2CRat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func assertCutValidExactRat(tb testing.TB, cut cutPlane, x []*big.Rat, label string) {
	tb.Helper()
	if math.IsNaN(cut.RHS) || math.IsInf(cut.RHS, 0) || math.IsNaN(cut.Efficacy) || math.IsInf(cut.Efficacy, 0) {
		tb.Fatalf("%s: emitted cut has non-finite RHS=%v or Efficacy=%v", label, cut.RHS, cut.Efficacy)
	}
	if len(cut.ColIdx) != len(cut.Val) || len(cut.ColIdx) == 0 {
		tb.Fatalf("%s: malformed cut lengths col=%d val=%d", label, len(cut.ColIdx), len(cut.Val))
	}
	lhs := new(big.Rat)
	term := new(big.Rat)
	for k, col := range cut.ColIdx {
		v := cut.Val[k]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			tb.Fatalf("%s: cut term %d has non-finite val %v", label, k, v)
		}
		term.Mul(mustTrack2CRat(tb, v), x[col])
		lhs.Add(lhs, term)
	}
	rhs := mustTrack2CRat(tb, cut.RHS)
	if lhs.Cmp(rhs) > 0 {
		diff := new(big.Rat).Sub(lhs, rhs)
		fDiff, _ := diff.Float64()
		tb.Fatalf("%s: cut violated in exact *big.Rat by %g (lhs=%s > rhs=%s)", label, fDiff, lhs.RatString(), rhs.RatString())
	}
}

func assertCutDropUniversalExactRat(
	tb testing.TB,
	p *CompiledProblem,
	rawIdx []int,
	rawVal []float64,
	rawRHS float64,
	cut cutPlane,
	label string,
) {
	tb.Helper()
	n := p.numVars
	rawDense := make([]*big.Rat, n)
	outDense := make([]*big.Rat, n)
	for j := range n {
		rawDense[j] = new(big.Rat)
		outDense[j] = new(big.Rat)
	}
	for k, col := range rawIdx {
		rawDense[col].Add(rawDense[col], mustTrack2CRat(tb, rawVal[k]))
	}
	for k, col := range cut.ColIdx {
		outDense[col].Add(outDense[col], mustTrack2CRat(tb, cut.Val[k]))
	}

	// Recover the exact power-of-two scale factor s = out[k] / raw[k] from the largest surviving term.
	var scale *big.Rat
	bestAbs := -1.0
	for k, col := range cut.ColIdx {
		if rawDense[col].Sign() == 0 {
			continue
		}
		if av := math.Abs(cut.Val[k]); av > bestAbs {
			bestAbs = av
			q := new(big.Rat).Quo(outDense[col], rawDense[col])
			if q.Sign() > 0 {
				scale = q
			}
		}
	}
	if scale == nil || scale.Sign() <= 0 {
		tb.Fatalf("%s: failed to recover positive scale factor from finalized cut", label)
	}

	// Verify exact universal dropMinTerm condition (Track2C.finalize_cut_drop_universal_sound):
	// for every column j, diff = s*raw[j] - out[j] must have a finite required extremum bound lo_j,
	// and s*rawRHS - sum_j lo_j <= finalCut.RHS.
	loSum := new(big.Rat)
	diff := new(big.Rat)
	scaledRaw := new(big.Rat)
	for j := range n {
		scaledRaw.Mul(scale, rawDense[j])
		diff.Sub(scaledRaw, outDense[j])
		switch diff.Sign() {
		case 0:
			continue
		case 1:
			if math.IsInf(p.varLB[j], 0) || math.IsNaN(p.varLB[j]) {
				tb.Fatalf("%s: column %d has positive dropped/perturbed diff=%s with non-finite lb=%v",
					label, j, diff.RatString(), p.varLB[j])
			}
			loSum.Add(loSum, new(big.Rat).Mul(diff, mustTrack2CRat(tb, p.varLB[j])))
		case -1:
			if math.IsInf(p.varUB[j], 0) || math.IsNaN(p.varUB[j]) {
				tb.Fatalf("%s: column %d has negative dropped/perturbed diff=%s with non-finite ub=%v",
					label, j, diff.RatString(), p.varUB[j])
			}
			loSum.Add(loSum, new(big.Rat).Mul(diff, mustTrack2CRat(tb, p.varUB[j])))
		}
	}

	reqRHS := new(big.Rat).Mul(scale, mustTrack2CRat(tb, rawRHS))
	reqRHS.Sub(reqRHS, loSum)
	gotRHS := mustTrack2CRat(tb, cut.RHS)
	if reqRHS.Cmp(gotRHS) > 0 {
		gap, _ := new(big.Rat).Sub(reqRHS, gotRHS).Float64()
		tb.Fatalf("%s: universal finalized RHS violated by %g (req=%s > got=%s)",
			label, gap, reqRHS.RatString(), gotRHS.RatString())
	}
}

func TestFormalTrack2CKnapsackCoverSoundnessAndRegressions(t *testing.T) {
	t.Parallel()

	t.Run("cover_weight_drift_71_items_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug A: 71 binary items with w0 = 2^53 and w1..w70 = 1.0000000000000002.
		// Exact sum = 2^53 + 70.000000000000014 < 2^53 + 90 = rhs, so x = (1,...,1) is feasible!
		// Naive float64 accumulation drifted upward by +70 to 2^53 + 140, exceeding coverMargin=32
		// and emitting an invalid cover cut sum x_i <= 70.
		const nItems = 71
		w0 := math.Ldexp(1, 53)
		wSmall := math.Nextafter(1.0, 2.0)
		rhsFeas := w0 + 90.0

		m := NewModel("cover_drift_71", Maximize)
		xs := make([]Var, nItems)
		terms := make([]Term, nItems)
		objTerms := make([]Term, nItems)
		for i := range nItems {
			xs[i] = m.AddBinaryVar(fmt.Sprintf("x%d", i))
			w := wSmall
			if i == 0 {
				w = w0
			}
			terms[i] = xs[i].T(w)
			objTerms[i] = xs[i].T(1)
		}
		m.SetObjective(NewExpr(objTerms...))
		m.AddConstraint("knap", NewExpr(terms...).Le(rhsFeas))
		p := m.Compile()

		lpPrimal := make([]float64, nItems)
		allOneRat := make([]*big.Rat, nItems)
		for i := range nItems {
			lpPrimal[i] = 0.99
			allOneRat[i] = big.NewRat(1, 1)
		}

		// Confirm x = (1,...,1) is strictly feasible in exact *big.Rat.
		exactSum := new(big.Rat)
		for i := range nItems {
			w := wSmall
			if i == 0 {
				w = w0
			}
			exactSum.Add(exactSum, mustTrack2CRat(t, w))
		}
		if exactSum.Cmp(mustTrack2CRat(t, rhsFeas)) > 0 {
			t.Fatalf("test setup error: exactSum=%s > rhs=%g", exactSum.RatString(), rhsFeas)
		}

		var ws cutGenWorkspace
		cuts := separateKnapsackCoverCuts(p, lpPrimal, &ws)
		for idx, cut := range cuts {
			assertCutValidExactRat(t, cut, allOneRat, fmt.Sprintf("cover_drift_71_cut_%d", idx))
		}
		if len(cuts) != 0 {
			t.Fatalf("expected 0 cover cuts when all 71 items fit in exact arithmetic, got %d", len(cuts))
		}

		// Positive control: tighten RHS to 2^53 + 30 so exact sum 2^53+70 exceeds rhs + coverMargin (2^53+62).
		m2 := NewModel("cover_drift_71_valid", Maximize)
		xs2 := make([]Var, nItems)
		terms2 := make([]Term, nItems)
		obj2 := make([]Term, nItems)
		for i := range nItems {
			xs2[i] = m2.AddBinaryVar(fmt.Sprintf("x%d", i))
			w := wSmall
			if i == 0 {
				w = w0
			}
			terms2[i] = xs2[i].T(w)
			obj2[i] = xs2[i].T(1)
		}
		m2.SetObjective(NewExpr(obj2...))
		m2.AddConstraint("knap", NewExpr(terms2...).Le(w0+30.0))
		p2 := m2.Compile()

		cuts2 := separateKnapsackCoverCuts(p2, lpPrimal, &ws)
		if len(cuts2) == 0 {
			t.Fatal("expected valid lifted cover cut when exact sum 2^53+70 exceeds rhs+margin (2^53+62)")
		}
		// Feasible points for rhs = 2^53 + 30:
		// 1) x0 = 0, x1..x70 = 1 -> exact sum 70 * nextUp(1) < 2^53 + 30.
		// 2) x0 = 1, x1..x29 = 1, x30..x70 = 0 -> exact sum 2^53 + 29*nextUp(1) < 2^53 + 30.
		dropItem0 := make([]*big.Rat, nItems)
		dropItem0[0] = big.NewRat(0, 1)
		for i := 1; i < nItems; i++ {
			dropItem0[i] = big.NewRat(1, 1)
		}
		keep29Small := make([]*big.Rat, nItems)
		for i := range nItems {
			if i <= 29 {
				keep29Small[i] = big.NewRat(1, 1)
			} else {
				keep29Small[i] = big.NewRat(0, 1)
			}
		}
		for idx, cut := range cuts2 {
			assertCutValidExactRat(t, cut, dropItem0, fmt.Sprintf("cover_valid_drop0_%d", idx))
			assertCutValidExactRat(t, cut, keep29Small, fmt.Sprintf("cover_valid_keep29_%d", idx))
		}
	})

	t.Run("complemented_relaxed_dp_lifted_exhaustive_exact_rat", func(t *testing.T) {
		t.Parallel()
		// Mixed binary + continuous + complemented negative-weight binary row (both <= and >=).
		// Row (<=): 4*x0 + 4*x1 + 6*x2 - 3*x3 + 5*x4 + 2*c >=? Let's test both <= and >=.
		// Case 1 (<=): 4*x0 + 4*x1 + 6*x2 - 3*x3 + 5*x4 + 2*c <= 8, c in [1, 10]
		// Relax c >= 1: 4*x0 + 4*x1 + 6*x2 - 3*x3 + 5*x4 <= 6
		// Complement x3 -> z3 = 1 - x3: 4*x0 + 4*x1 + 6*x2 + 3*z3 + 5*x4 <= 9.
		for _, useGE := range []bool{false, true} {
			m := NewModel(fmt.Sprintf("cover_comp_ge_%v", useGE), Maximize)
			x := make([]Var, 5)
			for i := range 5 {
				x[i] = m.AddBinaryVar(fmt.Sprintf("x%d", i))
			}
			c := m.AddContVar("c", 1, 10)
			m.SetObjective(NewExpr(x[0].T(1), x[1].T(1), x[2].T(1), x[3].T(-1), x[4].T(1)))
			if !useGE {
				m.AddConstraint("row", NewExpr(x[0].T(4), x[1].T(4), x[2].T(6), x[3].T(-3), x[4].T(5), c.T(2)).Le(8))
			} else {
				m.AddConstraint("row", NewExpr(x[0].T(-4), x[1].T(-4), x[2].T(-6), x[3].T(3), x[4].T(-5), c.T(-2)).Ge(-8))
			}
			p := m.Compile()

			lpPrimal := []float64{0.9, 0.9, 0.9, 0.1, 0.8, 1.0}
			var ws cutGenWorkspace
			cuts := separateKnapsackCoverCuts(p, lpPrimal, &ws)
			if len(cuts) == 0 {
				t.Fatalf("useGE=%v: expected complemented lifted cover cut", useGE)
			}

			// Exhaustively verify every emitted cut over all 2^5 binary vectors with c in {1, 2, 10}.
			coeffs := []int64{4, 4, 6, -3, 5, 2}
			for mask := range 1 << 5 {
				for _, cVal := range []int64{1, 2, 10} {
					pt := make([]*big.Rat, 6)
					rowSum := int64(0)
					for j := range 5 {
						bit := int64((mask >> j) & 1)
						pt[j] = big.NewRat(bit, 1)
						rowSum += coeffs[j] * bit
					}
					pt[5] = big.NewRat(cVal, 1)
					rowSum += coeffs[5] * cVal
					if rowSum <= 8 {
						for idx, cut := range cuts {
							assertCutValidExactRat(t, cut, pt, fmt.Sprintf("useGE=%v mask=%d c=%d cut=%d", useGE, mask, cVal, idx))
						}
					}
				}
			}
		}
	})

	t.Run("dp_minweight_inf_overflow_no_nan_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug A3: cover weights near 0.7*MaxFloat64 cause minWeight[v-1]+w to overflow +Inf.
		// Previously s - 8*u*s evaluated +Inf - +Inf = NaN, causing minWeight[v] <= remCap+dpTol to be false
		// and over-lifting alpha!
		bigW := 0.6 * math.MaxFloat64
		medW := 0.35 * math.MaxFloat64
		rhs := 0.9 * math.MaxFloat64
		m := NewModel("cover_inf_dp", Maximize)
		x := make([]Var, 5)
		for i := range 5 {
			x[i] = m.AddBinaryVar(fmt.Sprintf("x%d", i))
		}
		m.SetObjective(NewExpr(x[0].T(1), x[1].T(1), x[2].T(1), x[3].T(1), x[4].T(1)))
		m.AddConstraint("huge", NewExpr(x[0].T(bigW), x[1].T(bigW), x[2].T(bigW), x[3].T(medW), x[4].T(medW)).Le(rhs))
		p := m.Compile()

		lpPrimal := []float64{0.6, 0.6, 0.6, 0.4, 0.4}
		var ws cutGenWorkspace
		cuts := separateKnapsackCoverCuts(p, lpPrimal, &ws)

		wRats := []*big.Rat{
			mustTrack2CRat(t, bigW),
			mustTrack2CRat(t, bigW),
			mustTrack2CRat(t, bigW),
			mustTrack2CRat(t, medW),
			mustTrack2CRat(t, medW),
		}
		rhsRat := mustTrack2CRat(t, rhs)
		for mask := range 1 << 5 {
			sum := new(big.Rat)
			pt := make([]*big.Rat, 5)
			for j := range 5 {
				bit := int64((mask >> j) & 1)
				pt[j] = big.NewRat(bit, 1)
				if bit == 1 {
					sum.Add(sum, wRats[j])
				}
			}
			if sum.Cmp(rhsRat) <= 0 {
				for idx, cut := range cuts {
					assertCutValidExactRat(t, cut, pt, fmt.Sprintf("inf_dp mask=%d cut=%d", mask, idx))
				}
			}
		}
	})
}

func TestFormalTrack2CLotSizingChainSoundnessAndRegressions(t *testing.T) {
	t.Parallel()

	t.Run("cumdem_upward_drift_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug B1: d0 = 2^53, d1..d6 = 1.0001.
		// Naive cumDem drifted UP from exact 2^53 + 6.0006 to 2^53 + 12.0,
		// violating the feasible integer schedule (y0..y5=1, y6=0, p5=2.0002, s5=1.0001) by ~6.0!
		const T = 7
		d0 := math.Ldexp(1, 53)
		dSmall := 1.0001
		bigM := math.Ldexp(1, 54)

		m := NewModel("lotsizing_cumdem_drift", Minimize)
		y := make([]Var, T)
		prod := make([]Var, T)
		inv := make([]Var, T)
		objTerms := make([]Term, T)
		for tau := range T {
			y[tau] = m.AddBinaryVar(fmt.Sprintf("y%d", tau))
			prod[tau] = m.AddContVar(fmt.Sprintf("p%d", tau), 0, bigM)
			inv[tau] = m.AddContVar(fmt.Sprintf("s%d", tau), 0, bigM)
			objTerms[tau] = y[tau].T(10)
			m.AddConstraint(fmt.Sprintf("vub%d", tau), NewExpr(prod[tau].T(1), y[tau].T(-bigM)).Le(0))
			dem := dSmall
			if tau == 0 {
				dem = d0
			}
			if tau == 0 {
				m.AddConstraint("bal0", NewExpr(prod[0].T(1), inv[0].T(-1)).Eq(dem))
			} else {
				m.AddConstraint(fmt.Sprintf("bal%d", tau), NewExpr(inv[tau-1].T(1), prod[tau].T(1), inv[tau].T(-1)).Eq(dem))
			}
		}
		m.SetObjective(NewExpr(objTerms...))
		p := m.Compile()

		lpPrimal := make([]float64, p.numVars)
		for tau := range T {
			dem := dSmall
			if tau == 0 {
				dem = d0
			}
			lpPrimal[prod[tau].Index()] = dem
			if tau < T-1 {
				lpPrimal[y[tau].Index()] = 1.0
			} else {
				lpPrimal[y[tau].Index()] = 0.001
			}
		}

		var ws cutGenWorkspace
		cuts := separateFlowAndVUBCuts(p, lpPrimal, &ws)
		if len(cuts) == 0 {
			t.Fatal("expected (l, S) lot-sizing cuts on cumdem drift instance")
		}

		// Exact *big.Rat feasible schedule: y0..y5 = 1, y6 = 0, p0 = d0, p1..p4 = dSmall, p5 = 2*dSmall, s5 = dSmall.
		pt := make([]*big.Rat, p.numVars)
		for i := range pt {
			pt[i] = new(big.Rat)
		}
		d0Rat := mustTrack2CRat(t, d0)
		dSmallRat := mustTrack2CRat(t, dSmall)
		pt[y[0].Index()] = big.NewRat(1, 1)
		pt[prod[0].Index()] = new(big.Rat).Set(d0Rat)
		for tau := 1; tau <= 4; tau++ {
			pt[y[tau].Index()] = big.NewRat(1, 1)
			pt[prod[tau].Index()] = new(big.Rat).Set(dSmallRat)
		}
		pt[y[5].Index()] = big.NewRat(1, 1)
		pt[prod[5].Index()] = new(big.Rat).Mul(big.NewRat(2, 1), dSmallRat)
		pt[inv[5].Index()] = new(big.Rat).Set(dSmallRat)
		// tau=6: y6=0, p6=0, s6=0.

		for idx, cut := range cuts {
			assertCutValidExactRat(t, cut, pt, fmt.Sprintf("cumdem_drift_cut_%d", idx))
		}
	})

	t.Run("remdem_negative_drift_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug B2: d0 = 2^53, d1..d5 = 0.9999, d6 = 1.0.
		// Naive cumDem rounded DOWN to 2^53, then remDem -= d0..d5 drove remDem NEGATIVE (-4.9995),
		// emitting positive coefficients on y and violating the feasible schedule by ~15.0!
		const T = 7
		d0 := math.Ldexp(1, 53)
		dMid := 0.9999
		dLast := 1.0
		bigM := math.Ldexp(1, 54)

		m := NewModel("lotsizing_remdem_neg", Minimize)
		y := make([]Var, T)
		prod := make([]Var, T)
		inv := make([]Var, T)
		objTerms := make([]Term, T)
		dems := []float64{d0, dMid, dMid, dMid, dMid, dMid, dLast}
		for tau := range T {
			y[tau] = m.AddBinaryVar(fmt.Sprintf("y%d", tau))
			prod[tau] = m.AddContVar(fmt.Sprintf("p%d", tau), 0, bigM)
			inv[tau] = m.AddContVar(fmt.Sprintf("s%d", tau), 0, bigM)
			objTerms[tau] = y[tau].T(10)
			m.AddConstraint(fmt.Sprintf("vub%d", tau), NewExpr(prod[tau].T(1), y[tau].T(-bigM)).Le(0))
			if tau == 0 {
				m.AddConstraint("bal0", NewExpr(prod[0].T(1), inv[0].T(-1)).Eq(dems[0]))
			} else {
				m.AddConstraint(fmt.Sprintf("bal%d", tau), NewExpr(inv[tau-1].T(1), prod[tau].T(1), inv[tau].T(-1)).Eq(dems[tau]))
			}
		}
		m.SetObjective(NewExpr(objTerms...))
		p := m.Compile()

		lpPrimal := make([]float64, p.numVars)
		for tau := range T {
			lpPrimal[prod[tau].Index()] = dems[tau]
			lpPrimal[y[tau].Index()] = 0.01
		}

		var ws cutGenWorkspace
		cuts := separateFlowAndVUBCuts(p, lpPrimal, &ws)

		// Feasible schedule: produce exact demand in every period with all y_tau = 1.
		ptAll := make([]*big.Rat, p.numVars)
		for i := range ptAll {
			ptAll[i] = new(big.Rat)
		}
		for tau := range T {
			ptAll[y[tau].Index()] = big.NewRat(1, 1)
			ptAll[prod[tau].Index()] = mustTrack2CRat(t, dems[tau])
		}
		for idx, cut := range cuts {
			for k, col := range cut.ColIdx {
				if p.varCat[col] == Binary && cut.Val[k] > 0 {
					t.Fatalf("cut %d emitted positive binary coeff %g on col %d (remDem went negative!)", idx, cut.Val[k], col)
				}
			}
			assertCutValidExactRat(t, cut, ptAll, fmt.Sprintf("remdem_neg_cut_%d", idx))
		}
	})

	t.Run("shared_bincol_and_invIn_exhaustive_exact_rat", func(t *testing.T) {
		t.Parallel()
		// 4-period chain with initial inventory invIn (not -1) AND periods 1 & 3 sharing the SAME binary setup variable yShared!
		const T = 4
		dems := []int64{10, 12, 8, 15}
		m := NewModel("lotsizing_shared_bin", Minimize)
		inv0 := m.AddContVar("inv_init", 0, 100)
		y0 := m.AddBinaryVar("y0")
		yShared := m.AddBinaryVar("yShared")
		y2 := m.AddBinaryVar("y2")
		yByPer := []Var{y0, yShared, y2, yShared}
		prod := make([]Var, T)
		inv := make([]Var, T)
		for tau := range T {
			prod[tau] = m.AddContVar(fmt.Sprintf("p%d", tau), 0, 100)
			inv[tau] = m.AddContVar(fmt.Sprintf("s%d", tau), 0, 100)
			m.AddConstraint(fmt.Sprintf("vub%d", tau), NewExpr(prod[tau].T(1), yByPer[tau].T(-60)).Le(0))
			if tau == 0 {
				m.AddConstraint("bal0", NewExpr(inv0.T(1), prod[0].T(1), inv[0].T(-1)).Eq(float64(dems[0])))
			} else {
				m.AddConstraint(fmt.Sprintf("bal%d", tau), NewExpr(inv[tau-1].T(1), prod[tau].T(1), inv[tau].T(-1)).Eq(float64(dems[tau])))
			}
		}
		m.SetObjective(NewExpr(inv0.T(5), y0.T(10), yShared.T(12), y2.T(10)))
		p := m.Compile()

		lpPrimal := make([]float64, p.numVars)
		lpPrimal[y0.Index()] = 0.2
		lpPrimal[yShared.Index()] = 0.25
		lpPrimal[y2.Index()] = 0.15
		for tau := range T {
			lpPrimal[prod[tau].Index()] = float64(dems[tau])
		}

		var ws cutGenWorkspace
		cuts := separateFlowAndVUBCuts(p, lpPrimal, &ws)
		if len(cuts) == 0 {
			t.Fatal("expected (l, S) cuts on shared-binCol chain")
		}

		// Enumerate all 2^3 binary setup combinations (y0, yShared, y2) with feasible schedules.
		for mask := range 1 << 3 {
			b0 := int64((mask >> 0) & 1)
			bS := int64((mask >> 1) & 1)
			b2 := int64((mask >> 2) & 1)
			active := []int64{b0, bS, b2, bS}

			// Build exact feasible schedule by supplying any uncovered prefix from inv0,
			// and covering each period from its most recent active setup (or inv0).
			pt := make([]*big.Rat, p.numVars)
			for i := range pt {
				pt[i] = new(big.Rat)
			}
			pt[y0.Index()] = big.NewRat(b0, 1)
			pt[yShared.Index()] = big.NewRat(bS, 1)
			pt[y2.Index()] = big.NewRat(b2, 1)
			prodVals := make([]int64, T)
			var invInitVal int64
			for tau := range T {
				assigned := false
				for s := tau; s >= 0; s-- {
					if active[s] == 1 {
						prodVals[s] += dems[tau]
						assigned = true
						break
					}
				}
				if !assigned {
					invInitVal += dems[tau]
				}
			}
			// Check VUB capacities (<= 60) and compute exact inventories.
			valid := invInitVal <= 100
			curInv := invInitVal
			invVals := make([]int64, T)
			for tau := range T {
				if prodVals[tau] > 60 {
					valid = false
					break
				}
				curInv = curInv + prodVals[tau] - dems[tau]
				if curInv < 0 || curInv > 100 {
					valid = false
					break
				}
				invVals[tau] = curInv
			}
			if !valid {
				continue
			}
			pt[inv0.Index()] = big.NewRat(invInitVal, 1)
			for tau := range T {
				pt[prod[tau].Index()] = big.NewRat(prodVals[tau], 1)
				pt[inv[tau].Index()] = big.NewRat(invVals[tau], 1)
			}
			for idx, cut := range cuts {
				assertCutValidExactRat(t, cut, pt, fmt.Sprintf("shared_bin mask=%d cut=%d", mask, idx))
			}
		}
	})
}

func TestFormalTrack2CAggregatedMIRAndPairwiseSurrogate(t *testing.T) {
	t.Parallel()

	t.Run("cmir_fractional_int_bounds_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug C1: integer variable x in [0.4, 5.4], y in [0, 5], row 2x + 2y <= 4.6.
		// Previously separateCMIRFromSparseRow shifted x by l=0.4 (non-integer!) and emitted x + y <= 1.4,
		// cutting off the feasible integer solutions (1, 1) and (2, 0)!
		m := NewModel("cmir_frac_int_bnd", Maximize)
		x := m.AddIntVar("x", 0.4, 5.4)
		y := m.AddIntVar("y", 0, 5)
		m.SetObjective(NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("r", NewExpr(x.T(2), y.T(2)).Le(4.6))
		p := m.Compile()

		lpPrimal := []float64{0.4, 1.9}
		cuts := separateRowMIRCuts(p, lpPrimal)
		feasPts := [][]*big.Rat{
			{big.NewRat(1, 1), big.NewRat(1, 1)},
			{big.NewRat(2, 1), big.NewRat(0, 1)},
			{big.NewRat(1, 1), big.NewRat(0, 1)},
		}
		for idx, cut := range cuts {
			for pIdx, pt := range feasPts {
				assertCutValidExactRat(t, cut, pt, fmt.Sprintf("frac_int_bnd cut=%d pt=%d", idx, pIdx))
			}
		}

		res, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true))
		if err != nil || res.Status != StatusOptimal || math.Abs(res.Objective-2.0) > 1e-6 {
			t.Fatalf("SolveCompiled with fractional int bounds: status=%v obj=%g vals=%v err=%v, want Optimal obj=2",
				res.Status, res.Objective, res.Values, err)
		}
	})

	t.Run("cmir_neg_betaHat_f0_underrounding_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug C2: -0.5 < betaHat < 0 where fl(betaHat - (-1)) rounds DOWN by 2^-55.
		// Choose delta = 1.0, rawRHS = -(2^-5 - 2^-55) = -(0.03125 - 2^-55),
		// row: 1*x0 + (31/32 + 2^-53)*x1 - 1*s <= rawRHS, x0,x1 int >= 0, s cont >= 0.
		// Exact feasible point: x0 = 0, x1 = 1, s = 1 (LHS = 31/32 + 2^-53 - 1 = -2^-5 + 2^-53? Wait:
		// s = 2 gives LHS = 31/32 + 2^-53 - 2 <= rawRHS; also x0=0, x1=0, s=1/16 gives LHS = -1/16 < -1/32!).
		targetBeta := -(math.Ldexp(1, -5) - math.Ldexp(1, -55))
		a1 := 31.0/32.0 + math.Ldexp(1, -53)

		m := NewModel("cmir_neg_beta_f0", Maximize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddIntVar("x1", 0, 10)
		s := m.AddContVar("s", 0, 100)
		m.SetObjective(NewExpr(x0.T(1), x1.T(1)))
		m.AddConstraint("r", NewExpr(x0.T(1), x1.T(a1), s.T(-1)).Le(targetBeta))
		p := m.Compile()

		lpPrimal := []float64{0.0, 0.5, 0.52}
		cuts := separateRowMIRCuts(p, lpPrimal)

		// Exact feasible mixed-integer points including tight s = (a1*x1 - targetBeta).
		a1Rat := mustTrack2CRat(t, a1)
		rhsRat := mustTrack2CRat(t, targetBeta)
		sTight := new(big.Rat).Sub(a1Rat, rhsRat) // x0=0, x1=1, s = a1 - targetBeta (exact tight!)
		sTight0 := new(big.Rat).Neg(rhsRat)       // x0=0, x1=0, s = -targetBeta (exact tight!)
		feasPts := [][]*big.Rat{
			{big.NewRat(0, 1), big.NewRat(1, 1), sTight},
			{big.NewRat(0, 1), big.NewRat(0, 1), sTight0},
			{big.NewRat(0, 1), big.NewRat(1, 1), big.NewRat(2, 1)},
		}
		for idx, cut := range cuts {
			for pIdx, pt := range feasPts {
				assertCutValidExactRat(t, cut, pt, fmt.Sprintf("neg_beta_f0 cut=%d pt=%d", idx, pIdx))
			}
		}
	})

	t.Run("cmir_extreme_alpha_and_nan_guards", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug C3: alpha = -MaxFloat64 -> nextDown(-MaxFloat64) = -Inf -> fj = NaN,
		// and NaN coefficient on variable with l = 0 (which dot2Acc.addProd early-returns on!).
		m := NewModel("cmir_extreme_guards", Maximize)
		x := m.AddIntVar("x", 0, 10)
		y := m.AddIntVar("y", 0, 10)
		s := m.AddContVar("s", 0, 10)
		m.SetObjective(NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("r", NewExpr(x.T(1), y.T(1), s.T(-1)).Le(1.5))
		p := m.Compile()

		var ws cutGenWorkspace
		lpPrimal := []float64{0.5, 0.5, 0.0}
		cutsInf := separateCMIRFromSparseRow(p, []int{0, 1, 2}, []float64{1.0, -math.MaxFloat64, -1.0}, 1.5, 1.0, lpPrimal, 0, 4, &ws, nil)
		for idx, cut := range cutsInf {
			assertCutValidExactRat(t, cut, []*big.Rat{big.NewRat(0, 1), big.NewRat(0, 1), big.NewRat(0, 1)}, fmt.Sprintf("extreme_alpha_%d", idx))
		}
		cutsNaN := separateCMIRFromSparseRow(p, []int{0, 1, 2}, []float64{1.0, math.NaN(), -1.0}, 1.5, 1.0, lpPrimal, 0, 4, &ws, nil)
		if len(cutsNaN) != 0 {
			t.Fatalf("expected 0 cuts for NaN coefficient row, got %d", len(cutsNaN))
		}
	})

	t.Run("multi_row_agg_mechA_mechC_and_pairwise_surrogate_exact_rat", func(t *testing.T) {
		t.Parallel()
		// Build a MIP triggering:
		// 1) Mechanism A: equality elimination of an interior semi-bounded continuous variable s in [0, +Inf)
		//    Row 0: 2*x0 + s = 3 (eq), Row 1: -s + 2*x1 <= 2 (le) -> Agg: 2*x0 + 2*x1 <= 5 -> cMIR x0 + x1 <= 2!
		// 2) Pairwise tight-inequality surrogates: two tight <= rows sharing fractional integer vars.
		// 3) Mechanism C: two VUB capacity rows + demand equality elimination.
		m := NewModel("agg_cmir_full", Maximize)
		x0 := m.AddIntVar("x0", 0, 5)
		x1 := m.AddIntVar("x1", 0, 5)
		s := m.AddContVar("s", 0, math.Inf(1)) // semi-bounded [0, +Inf)!
		y0 := m.AddBinaryVar("y0")
		y1 := m.AddBinaryVar("y1")
		f0 := m.AddContVar("f0", 0, 10)
		f1 := m.AddContVar("f1", 0, 10)
		f2 := m.AddContVar("f2", 0, 10)
		f3 := m.AddContVar("f3", 0, 10)

		m.SetObjective(NewExpr(x0.T(3), x1.T(3), y0.T(-1), y1.T(-1)))
		// Mechanism A rows
		m.AddConstraint("eq_s", NewExpr(x0.T(2), s.T(1)).Eq(3))
		m.AddConstraint("le_s", NewExpr(s.T(-1), x1.T(2)).Le(2))
		// Pairwise tight rows on (x0, x1)
		m.AddConstraint("p1", NewExpr(x0.T(3), x1.T(1)).Le(7))
		m.AddConstraint("p2", NewExpr(x0.T(1), x1.T(3)).Le(7))
		// Mechanism C VUB capacity rows + demand partition
		m.AddConstraint("vub0", NewExpr(f0.T(1), f1.T(1), y0.T(-3)).Le(0))
		m.AddConstraint("vub1", NewExpr(f2.T(1), f3.T(1), y1.T(-3)).Le(0))
		m.AddConstraint("dem", NewExpr(f0.T(1), f1.T(1), f2.T(1), f3.T(1)).Ge(4))
		p := m.Compile()

		solver := NewLPSolver(p, nil)
		defer solver.Close()
		lpRes := solver.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP failed: status=%v", lpRes.Status)
		}

		var ws cutGenWorkspace
		aggCuts := separateAggregatedMIRCuts(solver, lpRes.Primal, &ws)
		if len(aggCuts) == 0 {
			t.Fatal("expected aggregated MIR / pairwise surrogate cuts")
		}

		// Exact feasible mixed-integer points satisfying all constraints:
		// (x0, x1, s, y0, y1, f0, f1, f2, f3):
		feasPts := [][]*big.Rat{
			{big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(2, 1), big.NewRat(0, 1), big.NewRat(2, 1), big.NewRat(0, 1)},
			{big.NewRat(0, 1), big.NewRat(2, 1), big.NewRat(3, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(3, 1), big.NewRat(0, 1), big.NewRat(1, 1), big.NewRat(0, 1)},
			{big.NewRat(1, 1), big.NewRat(0, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(1, 1), big.NewRat(2, 1), big.NewRat(1, 1), big.NewRat(0, 1)},
		}
		for idx, cut := range aggCuts {
			for pIdx, pt := range feasPts {
				assertCutValidExactRat(t, cut, pt, fmt.Sprintf("agg_full cut=%d pt=%d", idx, pIdx))
			}
		}

		// End-to-end MIP parity with and without cuts.
		resOn, errOn := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(true))
		resOff, errOff := SolveCompiled(t.Context(), p, WithPresolve(false), WithCuts(false))
		if errOn != nil || errOff != nil || resOn.Status != StatusOptimal || resOff.Status != StatusOptimal {
			t.Fatalf("MIP parity solve failed: on=(%v,%v) off=(%v,%v)", resOn.Status, errOn, resOff.Status, errOff)
		}
		if math.Abs(resOn.Objective-resOff.Objective) > 1e-6 {
			t.Fatalf("cuts changed optimal objective: on=%g off=%g", resOn.Objective, resOff.Objective)
		}
	})

	t.Run("gmi_near_integer_fractional_bound_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug E: integer variable with non-integer lower bound 0.5e-6 (< cutCompareTol=1e-6).
		// Previously separateGMICutsEx treated x - 0.5e-6 as an integer variable!
		m := NewModel("gmi_frac_bnd_e", Maximize)
		x := m.AddIntVar("x", 5e-7, 10)
		y := m.AddIntVar("y", 0, 10)
		m.SetObjective(NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("c1", NewExpr(x.T(2), y.T(2)).Le(5.0))
		p := m.Compile()

		solver := NewLPSolver(p, nil)
		defer solver.Close()
		lpRes := solver.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("LP failed: %v", lpRes.Status)
		}
		var ws cutGenWorkspace
		gmiCuts := separateGMICutsEx(solver, lpRes.Primal, &ws, false)
		feasPts := [][]*big.Rat{
			{big.NewRat(1, 1), big.NewRat(1, 1)},
			{big.NewRat(2, 1), big.NewRat(0, 1)},
		}
		for idx, cut := range gmiCuts {
			for pIdx, pt := range feasPts {
				assertCutValidExactRat(t, cut, pt, fmt.Sprintf("gmi_frac_bnd cut=%d pt=%d", idx, pIdx))
			}
		}
	})
}

func TestFormalTrack2CFinalizeSparseCutUniversal(t *testing.T) {
	t.Parallel()

	t.Run("wide_semibounded_and_unbounded_drop_universal_exact_rat", func(t *testing.T) {
		t.Parallel()
		m := NewModel("drop_domains", Minimize)
		xFree := m.AddContVar("xFree", math.Inf(-1), math.Inf(1))
		xSemiLo := m.AddContVar("xSemiLo", 1e8, math.Inf(1))
		xSemiHi := m.AddContVar("xSemiHi", math.Inf(-1), 2e8)
		xWide := m.AddContVar("xWide", -1e200, 1e200)
		xZeroLo := m.AddContVar("xZeroLo", 0, math.Inf(1))
		m.SetObjective(NewExpr(xFree.T(1), xSemiLo.T(1), xSemiHi.T(1), xWide.T(1), xZeroLo.T(1)))
		p := m.Compile()

		lpPrimal := []float64{10.0, 1e8, 2e8, 0.0, 0.0}
		var ws cutGenWorkspace

		// 1. Keep xFree (coeff 1.0), drop tiny +1e-12 on xSemiLo (lb=1e8 finite!),
		//    drop tiny -1e-12 on xSemiHi (ub=2e8 finite!), drop tiny +1e-12 on xZeroLo (lb=0 exact!).
		rawIdx := []int{0, 1, 2, 4}
		rawVal := []float64{1.0, 1e-12, -1e-12, 1e-12}
		rawRHS := 5.0
		cut, ok := finalizeSparseCut(p, rawIdx, rawVal, rawRHS, lpPrimal, 0, &ws)
		if !ok {
			t.Fatal("expected finalizeSparseCut to accept valid semi-bounded drops")
		}
		assertCutDropUniversalExactRat(t, p, rawIdx, rawVal, rawRHS, cut, "semibounded_sparse")

		dense := []float64{1.0, 1e-12, -1e-12, 0.0, 1e-12}
		cutDense, okDense := finalizeCut(p, dense, rawRHS, lpPrimal)
		if !okDense {
			t.Fatal("expected finalizeCut to accept valid semi-bounded drops")
		}
		assertCutDropUniversalExactRat(t, p, rawIdx, rawVal, rawRHS, cutDense, "semibounded_dense")

		// 2. Negative controls: dropping tiny coeff whose required bound side is infinite MUST be rejected!
		rejectCases := []struct {
			name string
			idx  []int
			val  []float64
		}{
			{"neg_tiny_on_semilo", []int{0, 1}, []float64{1.0, -1e-12}},
			{"pos_tiny_on_semihi", []int{0, 2}, []float64{1.0, 1e-12}},
			{"pos_tiny_on_free", []int{0, 1}, []float64{1e-12, 1.0}},
			{"neg_tiny_on_free", []int{0, 1}, []float64{-1e-12, 1.0}},
			{"neg_tiny_on_zerolo", []int{0, 4}, []float64{1.0, -1e-12}},
		}
		for _, tc := range rejectCases {
			if _, okBad := finalizeSparseCut(p, tc.idx, tc.val, 5.0, lpPrimal, 0, &ws); okBad {
				t.Fatalf("%s: finalizeSparseCut accepted drop on unbounded side!", tc.name)
			}
			d := make([]float64, p.numVars)
			for k, c := range tc.idx {
				d[c] = tc.val[k]
			}
			if _, okBad := finalizeCut(p, d, 5.0, lpPrimal); okBad {
				t.Fatalf("%s: finalizeCut accepted drop on unbounded side!", tc.name)
			}
		}

		// 3. Wide finite domain [-1e200, 1e200] with dropped tiny coeff: must stay universally valid.
		wideIdx := []int{0, 3}
		wideVal := []float64{1.0, 1e-12}
		wideRHS := 5.0
		// At xFree=10, xWide=0, rhs becomes ~1e188 so efficacy < 0 -> shift lpPrimal[0] to 1e190 to emit cut!
		lpWide := []float64{1e190, 1e8, 2e8, 0.0, 0.0}
		cutWide, okWide := finalizeSparseCut(p, wideIdx, wideVal, wideRHS, lpWide, 0, &ws)
		if !okWide {
			t.Fatal("expected finalizeSparseCut to emit cut on wide finite domain when violated")
		}
		assertCutDropUniversalExactRat(t, p, wideIdx, wideVal, wideRHS, cutWide, "wide_finite_drop")
	})

	t.Run("duplicate_column_inexact_merge_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug D1: duplicate column entries [-1e16, -0.4] on x0 in [0, 1e6].
		// fl(-1e16 + (-0.4)) rounds UP to -1e16 (losing -0.4*x0 which can be -4e5!).
		// With twoSum compensation, RHS is bumped outward by +4e5 so universal validity holds!
		m := NewModel("dup_merge", Minimize)
		x0 := m.AddContVar("x0", 0, 1e6)
		xFree := m.AddContVar("xFree", math.Inf(-1), math.Inf(1))
		m.SetObjective(NewExpr(x0.T(1), xFree.T(1)))
		p := m.Compile()

		var ws cutGenWorkspace
		rawIdx := []int{0, 0}
		rawVal := []float64{-1e16, -0.4}
		rawRHS := -1e18
		lpPrimal := []float64{0.0, 0.0}
		cut, ok := finalizeSparseCut(p, rawIdx, rawVal, rawRHS, lpPrimal, 0, &ws)
		if !ok {
			t.Fatal("expected finalizeSparseCut to accept bounded duplicate merge")
		}
		assertCutDropUniversalExactRat(t, p, rawIdx, rawVal, rawRHS, cut, "dup_merge_bounded")

		// Inexact duplicate merge on unbounded xFree (rem = -0.4 < 0, ub = +Inf) MUST be rejected!
		if _, okFree := finalizeSparseCut(p, []int{1, 1}, []float64{-1e16, -0.4}, -1e18, lpPrimal, 0, &ws); okFree {
			t.Fatal("finalizeSparseCut accepted inexact duplicate merge on unbounded column!")
		}
	})

	t.Run("negative_rhs_ldexp_overflow_and_neg_zero_regression", func(t *testing.T) {
		t.Parallel()
		// Regression for Bug D2: rhs = -1e305 with norm = 1e-10 (normExp = -34) causes
		// math.Ldexp(rhs, -normExp) = -Inf; previously nextUp(-Inf) turned -Inf into -MaxFloat64
		// and emitted a bogus cut with Efficacy = +Inf!
		m := NewModel("ldexp_overflow", Minimize)
		x := m.AddContVar("x", 0, 10)
		m.SetObjective(VarExpr(x))
		p := m.Compile()

		var ws cutGenWorkspace
		if cut, ok := finalizeSparseCut(p, []int{0}, []float64{1e-10}, -1e305, []float64{1.0}, 0, &ws); ok {
			t.Fatalf("finalizeSparseCut emitted bogus cut on -Inf scaled RHS: %+v", cut)
		}
		if cut, ok := finalizeCut(p, []float64{1e-10}, -1e305, []float64{1.0}); ok {
			t.Fatalf("finalizeCut emitted bogus cut on -Inf scaled RHS: %+v", cut)
		}

		// Signed -0.0 coefficient must be ignored cleanly without emitting a NaN/Inf cut.
		negZero := math.Copysign(0, -1)
		rawIdx := []int{0}
		rawVal := []float64{negZero}
		if _, ok := finalizeSparseCut(p, rawIdx, rawVal, 0.0, []float64{1.0}, 0, &ws); ok {
			t.Fatal("finalizeSparseCut should reject all-zero (-0.0) cut")
		}
		if _, ok := finalizeSparseCut(p, []int{0}, []float64{math.NaN()}, 0.0, []float64{1.0}, 0, &ws); ok {
			t.Fatal("finalizeSparseCut should reject NaN coefficient")
		}
		if _, ok := finalizeSparseCut(p, []int{0}, []float64{1.0}, math.Inf(-1), []float64{1.0}, 0, &ws); ok {
			t.Fatal("finalizeSparseCut should reject -Inf RHS")
		}
	})
}
