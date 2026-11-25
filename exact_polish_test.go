package goblin

import (
	"encoding/json/v2"
	"math"
	"math/big"
	"os"
	"testing"
	"time"
)

func buildMIPSmallSeed341() *CompiledProblem {
	m := NewModel("mip_small_k0_341_repro", Maximize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddIntVar("x1", 0, math.Inf(1))
	x2 := m.AddContVar("x2", 0, 12)
	x3 := m.AddBinaryVar("x3")
	x4 := m.AddIntVar("x4", 0, math.Inf(1))
	m.SetObjective(NewExpr(x0.T(-6), x1.T(4), x2.T(-3), x3.T(-6), x4.T(1)))
	m.AddConstraint("r0", Eq(NewExpr(x0.T(-3), x1.T(2), x3.T(-2), x4.T(-2)), 11))
	m.AddConstraint("r1", Ge(NewExpr(x1.T(5), x3.T(4), x4.T(-2)), -2))
	return m.Compile()
}

func buildSubULPInfeasiblePresolveLP() *CompiledProblem {
	base := math.Ldexp(1, 53)
	m := NewModel("sub_ulp_presolve_infeas", Minimize)
	x0 := m.AddContVar("x0", base+2, base+102)
	m.SetObjective(NewExpr(x0.T(1)))
	m.AddConstraint("r0", Le(NewExpr(x0.T(5e-6)), 4.503599627370522e10))
	m.AddConstraint("r1", Le(NewExpr(x0.T(5)), 4.503599627370522e16))
	m.AddConstraint("r2", Ge(NewExpr(x0.T(5e-6)), 4.503599627370522e10))
	for i := range 6 {
		m.AddConstraint("empty_"+string(rune('0'+i)), Le(NewExpr(), 0))
	}
	return m.Compile()
}

func buildSubULPNonRedundantDropPresolveLP() *CompiledProblem {
	m := NewModel("sub_ulp_drop_nonredundant", Minimize)
	x0 := m.AddContVar("x0", 2.5, 2.5)
	m.SetObjective(NewExpr(x0.T(1)))
	m.AddConstraint("r0", Ge(NewExpr(x0.T(-5e-6)), -1.25e-5))
	for i := range 8 {
		m.AddConstraint("empty_"+string(rune('0'+i)), Le(NewExpr(), 0))
	}
	return m.Compile()
}

func TestExactUnboundedMIPParityAndFreeVars(t *testing.T) {
	t.Parallel()

	t.Run("seed_341_parity_unbounded_mip", func(t *testing.T) {
		t.Parallel()
		p := buildMIPSmallSeed341()

		cert, ok := ExtractUnboundedCertificate(t.Context(), p, nil)
		if !ok || !cert.IsMIP || !VerifyUnboundedCertificate(p, cert) {
			t.Fatalf("ExtractUnboundedCertificate failed on seed 341: ok=%v cert=%+v", ok, cert)
		}
		mode, pay, okPay := ExportUnboundedCheckPayload(p, nil, nil)
		if !okPay || mode != "milp-unbounded" || len(pay) == 0 {
			t.Fatalf("ExportUnboundedCheckPayload failed: mode=%q ok=%v", mode, okPay)
		}

		for _, pre := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(pre), WithMaxNodes(400), WithTimeLimit(2*time.Second))
			if err != nil {
				t.Fatalf("pre=%v SolveCompiled err: %v", pre, err)
			}
			if res.Status != StatusUnbounded {
				t.Fatalf("pre=%v status=%v, want StatusUnbounded", pre, res.Status)
			}
		}

		var wit CertifiedWitness
		resCert, errCert := SolveCompiled(t.Context(), p, WithPureGo(), WithCertifiedWitness(&wit), WithMaxNodes(400))
		if errCert != nil || resCert.Status != StatusUnbounded || wit.Status != StatusUnbounded {
			t.Fatalf("certified solve failed: status=(%v, %v) err=%v", resCert.Status, wit.Status, errCert)
		}
		if wit.Unbounded == nil || !VerifyUnboundedCertificate(p, *wit.Unbounded) {
			t.Fatalf("invalid certified unbounded witness: %+v", wit.Unbounded)
		}
	})

	t.Run("doubly_unbounded_and_negative_semibounded_parity", func(t *testing.T) {
		t.Parallel()
		mFree := NewModel("free_parity_unb", Maximize)
		xf0 := mFree.AddBinaryVar("x0")
		xf1 := mFree.AddIntVar("x1", math.Inf(-1), math.Inf(1))
		xf2 := mFree.AddIntVar("x2", math.Inf(-1), math.Inf(1))
		mFree.SetObjective(NewExpr(xf1.T(4), xf2.T(1)))
		mFree.AddConstraint("eq", Eq(NewExpr(xf0.T(-3), xf1.T(2), xf2.T(-2)), 11))
		mFree.AddConstraint("ge", Ge(NewExpr(xf1.T(5), xf2.T(-2)), -2))
		pFree := mFree.Compile()

		certFree, okFree := ExtractUnboundedCertificate(t.Context(), pFree, nil)
		if !okFree || !certFree.IsMIP || !VerifyUnboundedCertificate(pFree, certFree) {
			t.Fatalf("ExtractUnboundedCertificate failed on doubly-unbounded parity MIP: ok=%v", okFree)
		}
		for _, pre := range []bool{false, true} {
			res, err := SolveCompiled(t.Context(), pFree, WithPureGo(), WithPresolve(pre), WithMaxNodes(400))
			if err != nil || res.Status != StatusUnbounded {
				t.Fatalf("free parity pre=%v: status=%v err=%v, want StatusUnbounded", pre, res.Status, err)
			}
		}

		mNeg := NewModel("neg_semibounded_parity_unb", Maximize)
		xn0 := mNeg.AddBinaryVar("x0")
		xn1 := mNeg.AddIntVar("x1", math.Inf(-1), 0)
		xn2 := mNeg.AddIntVar("x2", math.Inf(-1), 0)
		mNeg.SetObjective(NewExpr(xn1.T(-4), xn2.T(-1)))
		mNeg.AddConstraint("eq", Eq(NewExpr(xn0.T(-3), xn1.T(-2), xn2.T(2)), 11))
		pNeg := mNeg.Compile()

		certNeg, okNeg := ExtractUnboundedCertificate(t.Context(), pNeg, nil)
		if !okNeg || !certNeg.IsMIP || !VerifyUnboundedCertificate(pNeg, certNeg) {
			t.Fatalf("ExtractUnboundedCertificate failed on negative semibounded parity MIP: ok=%v", okNeg)
		}
		if res, err := SolveCompiled(t.Context(), pNeg, WithPureGo(), WithPresolve(false), WithMaxNodes(400)); err != nil || res.Status != StatusUnbounded {
			t.Fatalf("neg parity solve: status=%v err=%v, want StatusUnbounded", res.Status, err)
		}
	})

	t.Run("huge_finite_bound_scale_and_infeasible_free_parity", func(t *testing.T) {
		t.Parallel()
		mHuge := NewModel("huge_lb_unb", Minimize)
		xh := mHuge.AddIntVar("xh", -1e200, math.Inf(1))
		mHuge.SetObjective(NewExpr(xh.T(-1)))
		mHuge.AddConstraint("ge", Ge(NewExpr(xh.T(1)), -1e200))
		pHuge := mHuge.Compile()
		fpHuge := pHuge.semiBoundedFeasObjCopy()
		if fpHuge.objMin[0] <= 0 || math.IsInf(fpHuge.objMin[0]*(-1e200), 0) {
			t.Fatalf("unexpected scaled feas coefficient %g", fpHuge.objMin[0])
		}

		mInf := NewModel("infeas_free_parity", Maximize)
		xi1 := mInf.AddIntVar("x1", math.Inf(-1), math.Inf(1))
		xi2 := mInf.AddIntVar("x2", math.Inf(-1), math.Inf(1))
		mInf.SetObjective(NewExpr(xi1.T(1)))
		mInf.AddConstraint("odd", Eq(NewExpr(xi1.T(2), xi2.T(-2)), 1))
		pInf := mInf.Compile()
		if _, ok := ExtractUnboundedCertificate(t.Context(), pInf, nil); ok {
			t.Fatal("ExtractUnboundedCertificate unexpectedly certified infeasible parity MIP")
		}
		if res, err := SolveCompiled(t.Context(), pInf, WithPureGo(), WithPresolve(false), WithMaxNodes(400)); err != nil || res.Status != StatusInfeasible {
			t.Fatalf("infeasible free parity solve: status=%v err=%v, want StatusInfeasible", res.Status, err)
		}
	})

	t.Run("large_magnitude_free_cont_and_int_unbounded", func(t *testing.T) {
		t.Parallel()
		// Reproduce #77: free continuous x4 forced <= -1e17 must not add L1 aux t_k (which caused 1e17 LP violation).
		// Also test free integer x4 forced <= -1e15 (within representable integer precision < 2^53).
		for _, tc := range []struct {
			name    string
			intFree bool
			rhs     float64
		}{
			{name: "cont_1e17", intFree: false, rhs: -5e11},
			{name: "int_1e15", intFree: true, rhs: -5e9},
		} {
			mFree := NewModel("large_free_"+tc.name, Minimize)
			x0 := mFree.AddContVar("x0", 0.5, 4.5)
			x1 := mFree.AddBinaryVar("x1")
			x2 := mFree.AddBinaryVar("x2")
			x3 := mFree.AddBinaryVar("x3")
			var x4 Var
			if tc.intFree {
				x4 = mFree.AddIntVar("x4", math.Inf(-1), math.Inf(1))
			} else {
				x4 = mFree.AddContVar("x4", math.Inf(-1), math.Inf(1))
			}
			x5 := mFree.AddContVar("x5", 0.5, 4.5)
			mFree.SetObjective(NewExpr(x0.T(5e-6), x1.T(5e-6), x2.T(5e-6), x3.T(5e-6), x4.T(5e-6), x5.T(5e-6)).AddConstant(-0.1))
			mFree.AddConstraint("r0", Le(NewExpr(x4.T(5e-6)), tc.rhs))
			mFree.AddConstraint("r1", Le(NewExpr(), 0))
			pFree := mFree.Compile()

			fp, okFP := boundedFeasAnchorProblem(pFree)
			if !okFP {
				t.Fatalf("%s boundedFeasAnchorProblem failed", tc.name)
			}
			if !tc.intFree && fp.numVars != pFree.numVars {
				t.Fatalf("%s added unexpected aux vars for free continuous col: got %d, want %d", tc.name, fp.numVars, pFree.numVars)
			}
			if tc.intFree && fp.numVars != pFree.numVars+1 {
				t.Fatalf("%s expected 1 L1 aux var for free int col: got %d, want %d", tc.name, fp.numVars, pFree.numVars+1)
			}

			for _, optSet := range [][]Option{
				nil,
				{WithPureGo(), WithPresolve(true)},
				{WithPureGo(), WithPresolve(false), WithCuts(false), WithHeuristics(false)},
				{WithPureGo(), WithCertified(true)},
			} {
				res, err := SolveCompiled(t.Context(), pFree, optSet...)
				if err != nil || res.Status != StatusUnbounded {
					t.Fatalf("%s opts=%d status=%v err=%v, want StatusUnbounded", tc.name, len(optSet), res.Status, err)
				}
			}
			cert, ok := ExtractUnboundedCertificate(t.Context(), pFree, nil)
			if !ok || !cert.IsMIP || !VerifyUnboundedCertificate(pFree, cert) {
				t.Fatalf("%s ExtractUnboundedCertificate failed: ok=%v", tc.name, ok)
			}
		}
	})

	t.Run("sub_ulp_q_infeasible_lp_unbounded_mip_7baf", func(t *testing.T) {
		t.Parallel()
		// Reproduce #82 (7bafe3c71086e98f). Exact arithmetic rules out 5e-6*200000 = 1, leaving an empty integer hull despite a float-feasible LP-unbounded relaxation with free continuous x2.
		mSub := NewModel("sub_ulp_unbounded_infeas_7baf", Minimize)
		x0 := mSub.AddIntVar("x0", 1, math.Inf(1))
		x1 := mSub.AddBinaryVar("x1")
		x2 := mSub.AddContVar("x2", math.Inf(-1), math.Inf(1))
		mSub.SetObjective(NewExpr(x0.T(5e-6), x1.T(5e-6), x2.T(5e-6)).AddConstant(-0.1))
		mSub.AddConstraint("r0", Eq(NewExpr(x0.T(-5e-6), x1.T(1)), 0))
		pSub := mSub.Compile()

		for _, tc := range []struct {
			name string
			opts []Option
		}{
			{name: "default", opts: nil},
			{name: "default_pure_go", opts: []Option{WithPureGo()}},
			{name: "bare_zero_limits", opts: []Option{WithOptions(Options{})}},
			{name: "bare_default_nodes", opts: []Option{WithPureGo(), WithPresolve(false), WithCuts(false), WithHeuristics(false)}},
			{name: "certified", opts: []Option{WithPureGo(), WithCertified(true)}},
		} {
			res, err := SolveCompiled(t.Context(), pSub, tc.opts...)
			if err != nil {
				t.Fatalf("%s SolveCompiled err: %v", tc.name, err)
			}
			if res.Status != StatusInfeasible {
				t.Fatalf("%s status=%v, want StatusInfeasible", tc.name, res.Status)
			}
		}

		// Budget limits propagate cleanly.
		resIterLim, errIterLim := SolveCompiled(t.Context(), pSub, WithOptions(Options{MaxIter: 2}))
		if errIterLim != nil {
			t.Fatalf("MaxIter=2 err: %v", errIterLim)
		}
		if resIterLim.Status != StatusIterationLimit && resIterLim.Status != StatusInfeasible {
			t.Fatalf("MaxIter=2 status=%v, want StatusIterationLimit or StatusInfeasible", resIterLim.Status)
		}
		resNodeLim, errNodeLim := SolveCompiled(t.Context(), pSub, WithOptions(Options{MaxNodes: 2}))
		if errNodeLim != nil {
			t.Fatalf("MaxNodes=2 err: %v", errNodeLim)
		}
		if resNodeLim.Status != StatusNodeLimit && resNodeLim.Status != StatusInfeasible {
			t.Fatalf("MaxNodes=2 status=%v, want StatusNodeLimit or StatusInfeasible", resNodeLim.Status)
		}

		// Negative fractional branching proves StatusInfeasible or locates an exact anchor.
		if _, stSub, _, _ := solveExactMIPFeasibilityStop(exactStopFromCtx(t.Context()), pSub, 200, 0); stSub != StatusInfeasible {
			t.Fatalf("solveExactMIPFeasibilityStop(7baf) = %v, want StatusInfeasible", stSub)
		}
		if fl := floorExactRat(big.NewRat(-7, 3)); fl.Cmp(big.NewInt(-3)) != 0 {
			t.Fatalf("floorExactRat(-7/3) = %v, want -3", fl)
		}

		mNegPos := NewModel("neg_branch_exact_unbounded", Minimize)
		xn0 := mNegPos.AddIntVar("x0", math.Inf(-1), -1)
		xn1 := mNegPos.AddContVar("x1", math.Inf(-1), math.Inf(1))
		mNegPos.SetObjective(NewExpr(xn1.T(1)))
		mNegPos.AddConstraint("eq", Eq(NewExpr(xn0.T(3)), -9))
		pNegPos := mNegPos.Compile()
		anchor, stNeg, _, _ := solveExactMIPFeasibilityStop(exactStopFromCtx(t.Context()), pNegPos, 200, 0)
		if stNeg != StatusOptimal {
			t.Fatalf("solveExactMIPFeasibilityStop(neg_branch) = %v, want StatusOptimal", stNeg)
		}
		if certNeg, okCert := extractUnboundedCertificateWithRatAnchorStop(exactStopFromCtx(t.Context()), pNegPos, nil, anchor); !okCert || !VerifyUnboundedCertificate(pNegPos, certNeg) {
			t.Fatalf("extractUnboundedCertificateWithRatAnchorStop failed: ok=%v", okCert)
		}
	})
}

func TestExactUnboundedLPTinyCoeffAndHomogeneousReset(t *testing.T) {
	t.Parallel()

	t.Run("tiny_coeff_lp_anchor_recovery", func(t *testing.T) {
		t.Parallel()
		// Reproduce #44: 1e-20*x0 + x1 <= 0 with x0 in (-Inf, 5] keeps x0=5 within float primalTol.
		m := NewModel("gen_cbc_r3_min_tiny", Minimize)
		x0 := m.AddContVar("x0", math.Inf(-1), 5)
		x1 := m.AddContVar("x1", 0, 10)
		x2 := m.AddContVar("x2", 0, 10)
		x3 := m.AddContVar("x3", 0, 10)
		x4 := m.AddContVar("x4", 0, 10)
		_, _, _ = x2, x3, x4
		m.SetObjective(NewExpr(x0.T(144.5625)))
		m.AddConstraint("r0", Le(NewExpr(x0.T(1e-20), x1.T(1)), 0))
		p := m.Compile()

		xAnchor, okAnchor := solveExactLPFeasibleAnchor(exactStopFromCtx(t.Context()), p)
		if !okAnchor || !verifyExactPrimalRat(p, xAnchor, false) {
			t.Fatalf("solveExactLPFeasibleAnchor failed on tiny-coeff LP: ok=%v x=%v", okAnchor, xAnchor)
		}
		cert, okCert := ExtractUnboundedCertificate(t.Context(), p, nil)
		if !okCert || cert.IsMIP || !VerifyUnboundedCertificate(p, cert) {
			t.Fatalf("ExtractUnboundedCertificate failed on tiny-coeff LP: ok=%v", okCert)
		}
		mode, pay, okPay := ExportUnboundedCheckPayload(p, nil, nil)
		if !okPay || mode != "unbounded" || len(pay) == 0 {
			t.Fatalf("ExportUnboundedCheckPayload failed: mode=%q ok=%v", mode, okPay)
		}
	})

	t.Run("orthogonal_tiny_coeff_lp_and_integer_ray_step", func(t *testing.T) {
		t.Parallel()
		// Row violation is orthogonal to rayLP: x2 in [0, 10] violates 1e-20*x2 + x1 <= 0 at x2>0.
		mOrtho := NewModel("ortho_tiny_lp", Minimize)
		xo0 := mOrtho.AddContVar("x0", math.Inf(-1), 5)
		xo1 := mOrtho.AddContVar("x1", 0, 10)
		xo2 := mOrtho.AddContVar("x2", 0, 10)
		mOrtho.SetObjective(NewExpr(xo0.T(2)))
		mOrtho.AddConstraint("tiny_box", Le(NewExpr(xo2.T(1e-20), xo1.T(1)), 0))
		mOrtho.AddConstraint("ray_cap", Le(NewExpr(xo0.T(1)), 5))
		pOrtho := mOrtho.Compile()

		certOrtho, okOrtho := ExtractUnboundedCertificate(t.Context(), pOrtho, nil)
		if !okOrtho || !VerifyUnboundedCertificate(pOrtho, certOrtho) {
			t.Fatalf("ExtractUnboundedCertificate failed on orthogonal tiny LP: ok=%v", okOrtho)
		}

		// Integer variant of #44 where feasibility objective pulls x0 to ub=5 inside float tolerance.
		mInt := NewModel("tiny_coeff_int_unb", Minimize)
		xi0 := mInt.AddIntVar("x0", math.Inf(-1), 5)
		xi1 := mInt.AddIntVar("x1", 0, 10)
		mInt.SetObjective(NewExpr(xi0.T(3)))
		mInt.AddConstraint("r0", Le(NewExpr(xi0.T(1e-20), xi1.T(1)), 0))
		pInt := mInt.Compile()

		certInt, okInt := ExtractUnboundedCertificate(t.Context(), pInt, nil)
		if !okInt || !certInt.IsMIP || !VerifyUnboundedCertificate(pInt, certInt) {
			t.Fatalf("ExtractUnboundedCertificate failed on tiny-coeff integer MIP: ok=%v", okInt)
		}
	})

	t.Run("homogeneous_ray_resets_row_bounds_and_cache", func(t *testing.T) {
		t.Parallel()
		m := NewModel("homog_row_bounds", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1), y.T(-2)))
		m.AddConstraint("ge", Ge(NewExpr(x.T(1), y.T(-1)), 100))
		m.AddConstraint("le", Le(NewExpr(x.T(-1), y.T(2)), 50))
		p := m.Compile()

		// Populate p.lazy.scaled before invoking solveHomogeneousRecessionRayLP.
		lp := NewLPSolver(p, nil)
		res := lp.Solve()
		lp.Close()
		if res.Status != StatusUnbounded {
			t.Fatalf("status=%v, want StatusUnbounded", res.Status)
		}

		d, slope, okRay := solveHomogeneousRecessionRayLP(exactStopFromCtx(t.Context()), p)
		if !okRay || slope == nil || slope.Sign() >= 0 || len(d) != 2 {
			t.Fatalf("solveHomogeneousRecessionRayLP failed: ok=%v slope=%v d=%v", okRay, slope, d)
		}
		if d[0].Cmp(d[1]) < 0 {
			t.Fatalf("expected homogeneous ray d0 >= d1, got d=%v", d)
		}
		if _, okNeg := stepExactRayAnchor(p, nil, []*big.Rat{big.NewRat(0, 1)}, false); okNeg {
			t.Fatal("stepExactRayAnchor unexpectedly accepted mismatched ray length")
		}
	})

	t.Run("infeasible_lp_with_recession_ray_rejected", func(t *testing.T) {
		t.Parallel()
		m := NewModel("infeas_lp_ray", Minimize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(NewExpr(x.T(-1)))
		m.AddConstraint("ge", Ge(NewExpr(x.T(1), y.T(-1)), 1))
		m.AddConstraint("le", Le(NewExpr(x.T(1), y.T(-1)), 0))
		p := m.Compile()
		if _, ok := solveExactLPFeasibleAnchor(exactStopFromCtx(t.Context()), p); ok {
			t.Fatal("solveExactLPFeasibleAnchor unexpectedly succeeded on infeasible LP")
		}
		if _, ok := ExtractUnboundedCertificate(t.Context(), p, nil); ok {
			t.Fatal("ExtractUnboundedCertificate unexpectedly certified infeasible LP")
		}
	})
}

func TestExportLPPresolveRejectsInfeasibleSubULPReduced(t *testing.T) {
	t.Parallel()

	pInfeas := buildSubULPInfeasiblePresolveLP()
	if raw, ok := ExportLPPresolveCheckPayload(pInfeas); ok || len(raw) != 0 {
		t.Fatalf("ExportLPPresolveCheckPayload emitted invalid payload without xReduced (len=%d)", len(raw))
	}

	pDrop := buildSubULPNonRedundantDropPresolveLP()
	if raw, ok := ExportLPPresolveCheckPayload(pDrop); ok || len(raw) != 0 {
		t.Fatalf("ExportLPPresolveCheckPayload emitted uncertified dropRedundantRow (len=%d)", len(raw))
	}

	mOpt := NewModel("presolve_opt_ok", Minimize)
	x0 := mOpt.AddContVar("x0", 2, 2)
	x1 := mOpt.AddContVar("x1", 0, 10)
	mOpt.SetObjective(NewExpr(x0.T(3), x1.T(1)))
	mOpt.AddConstraint("c", Ge(NewExpr(x0.T(1), x1.T(1)), 5))
	rawOpt, okOpt := ExportLPPresolveCheckPayload(mOpt.Compile())
	if !okOpt || len(rawOpt) == 0 {
		t.Fatal("ExportLPPresolveCheckPayload failed on feasible presolved LP")
	}
	var parsed lpPresolveChainCertExport
	if err := json.Unmarshal(rawOpt, &parsed); err != nil {
		t.Fatalf("unmarshal lp-presolve payload: %v", err)
	}
	if len(parsed.XReduced) != 2 {
		t.Fatalf("expected len(XReduced)=2, got %d", len(parsed.XReduced))
	}
}

func TestFormalBugfix3ExactUnboundedAndPresolve(t *testing.T) {
	t.Parallel()
	if os.Getenv("GOBLIN_FORMAL") != "1" {
		t.Skip("set GOBLIN_FORMAL=1 to run Lean lp-check verification")
	}
	ctx := t.Context()

	p341 := buildMIPSmallSeed341()
	mode341, pay341, ok341 := ExportUnboundedCheckPayload(p341, nil, nil)
	if !ok341 || mode341 != "milp-unbounded" {
		t.Fatalf("seed 341 ExportUnboundedCheckPayload failed: mode=%q ok=%v", mode341, ok341)
	}
	if resp, err := FormalCheckLPJSONL(t, pay341); err != nil || !resp.OK || resp.Mode != "milp-unbounded" {
		t.Fatalf("Lean lp-check rejected seed 341 milp-unbounded payload: resp=%+v err=%v", resp, err)
	}

	m44 := NewModel("bug44_tiny_lp", Minimize)
	x0 := m44.AddContVar("x0", math.Inf(-1), 5)
	x1 := m44.AddContVar("x1", 0, 10)
	m44.SetObjective(NewExpr(x0.T(144.5625)))
	m44.AddConstraint("r0", Le(NewExpr(x0.T(1e-20), x1.T(1)), 0))
	mode44, pay44, ok44 := ExportUnboundedCheckPayload(m44.Compile(), nil, nil)
	if !ok44 || mode44 != "unbounded" {
		t.Fatalf("bug 44 ExportUnboundedCheckPayload failed: mode=%q ok=%v", mode44, ok44)
	}
	if resp, err := FormalCheckLPJSONL(t, pay44); err != nil || !resp.OK || resp.Mode != "unbounded" {
		t.Fatalf("Lean lp-check rejected bug 44 unbounded payload: resp=%+v err=%v", resp, err)
	}

	mFree := NewModel("free_parity_formal", Maximize)
	xf0 := mFree.AddBinaryVar("x0")
	xf1 := mFree.AddIntVar("x1", math.Inf(-1), math.Inf(1))
	xf2 := mFree.AddIntVar("x2", math.Inf(-1), math.Inf(1))
	mFree.SetObjective(NewExpr(xf1.T(4), xf2.T(1)))
	mFree.AddConstraint("eq", Eq(NewExpr(xf0.T(-3), xf1.T(2), xf2.T(-2)), 11))
	mFree.AddConstraint("ge", Ge(NewExpr(xf1.T(5), xf2.T(-2)), -2))
	pFree := mFree.Compile()
	if _, err := SolveCompiled(ctx, pFree, WithPureGo(), WithPresolve(true), WithMaxNodes(400)); err != nil {
		t.Fatalf("SolveCompiled free parity failed: %v", err)
	}
	modeFree, payFree, okFree := ExportUnboundedCheckPayload(pFree, nil, nil)
	if !okFree || modeFree != "milp-unbounded" {
		t.Fatalf("free parity ExportUnboundedCheckPayload failed: mode=%q ok=%v", modeFree, okFree)
	}
	if resp, err := FormalCheckLPJSONL(t, payFree); err != nil || !resp.OK || resp.Mode != "milp-unbounded" {
		t.Fatalf("Lean lp-check rejected free parity payload: resp=%+v err=%v", resp, err)
	}

	if _, okPre := ExportLPPresolveCheckPayload(buildSubULPInfeasiblePresolveLP()); okPre {
		t.Fatal("expected ExportLPPresolveCheckPayload to reject sub-ULP Q-infeasible reduced LP")
	}
	if _, okDrop := ExportLPPresolveCheckPayload(buildSubULPNonRedundantDropPresolveLP()); okDrop {
		t.Fatal("expected ExportLPPresolveCheckPayload to reject sub-ULP non-redundant dropRedundantRow")
	}
}
