package goblin

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type lpUnboundedCertPayload struct {
	lpCertPayload
	X0      []ratPayload `json:"x0"`
	D       []ratPayload `json:"d"`
	IntCols []int        `json:"intCols,omitzero"`
}

type lpPresolveStepPayload struct {
	Kind       string      `json:"kind"`
	Col        *int        `json:"col,omitzero"`
	OrigRow    *int        `json:"origRow,omitzero"`
	DroppedRow *int        `json:"droppedRow,omitzero"`
	ToLower    *bool       `json:"toLower,omitzero"`
	Val        *ratPayload `json:"val,omitzero"`
	OrigRowIdx []int       `json:"origRowIdx,omitzero"`
}

type lpPresolveCertPayload struct {
	Orig     lpCertPayload         `json:"orig"`
	Reduced  lpCertPayload         `json:"reduced"`
	Step     lpPresolveStepPayload `json:"step"`
	XReduced []ratPayload          `json:"xReduced,omitzero"`
	YReduced []ratPayload          `json:"yReduced,omitzero"`
}

type gcdDivCheckPayload struct {
	Coeffs           []int64    `json:"coeffs"`
	Sense            string     `json:"sense"`
	RHS              ratPayload `json:"rhs"`
	ExpectInfeasible *bool      `json:"expect_infeasible,omitzero"`
	DivCoeffs        []int64    `json:"div_coeffs,omitzero"`
	DivRHS           *int64     `json:"div_rhs,omitzero"`
}

func ratSliceToPayloads(src []*big.Rat) []ratPayload {
	out := make([]ratPayload, len(src))
	for i, r := range src {
		out[i] = bigRatToPayload(r)
	}
	return out
}

func locateLeanBinary3B(tb testing.TB) string {
	tb.Helper()
	if p, err := exec.LookPath("lean"); err == nil {
		return p
	}
	lakePath := locateLakeBinary(tb)
	cand := filepath.Join(filepath.Dir(lakePath), "lean")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	tb.Fatalf("lean binary not found near %s", lakePath)
	return ""
}

func TestFormalTrack3BExactBasisDualsAndSolution(t *testing.T) {
	t.Parallel()

	t.Run("free_and_semibounded_nondyadic_lp", func(t *testing.T) {
		t.Parallel()
		// Minimize 5*x_free + 7*x_pos - 3*x_neg + 2*x_box + 12.5
		// subject to:
		//   r0 (Equal):        3*x_free + 2*x_pos + 1*x_neg = 4
		//   r1 (GreaterEqual): 1*x_free + 5*x_pos - 2*x_neg + 3*x_box >= 9
		//   r2 (LessEqual):    2*x_pos + 7*x_box <= 15
		// with x_free in (-Inf, +Inf), x_pos in [0, +Inf), x_neg in (-Inf, 0], x_box in [0, 4].
		m := NewModel("free_semibounded_nondyadic", Minimize)
		xFree := m.AddContVar("x_free", math.Inf(-1), math.Inf(1))
		xPos := m.AddContVar("x_pos", 0, math.Inf(1))
		xNeg := m.AddContVar("x_neg", math.Inf(-1), 0)
		xBox := m.AddContVar("x_box", 0, 4)
		m.SetObjective(NewExpr(xFree.T(5), xPos.T(7), xNeg.T(-3), xBox.T(2)).AddConstant(12.5))
		m.AddConstraint("r0", NewExpr(xFree.T(3), xPos.T(2), xNeg.T(1)).Eq(4))
		m.AddConstraint("r1", NewExpr(xFree.T(1), xPos.T(5), xNeg.T(-2), xBox.T(3)).Ge(9))
		m.AddConstraint("r2", NewExpr(xPos.T(2), xBox.T(7)).Le(15))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		res := lp.Solve()
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want StatusOptimal", res.Status)
		}

		dualMin, dualUser, ok := ExactBasisDuals(t.Context(), lp)
		if !ok {
			t.Fatal("ExactBasisDuals returned false")
		}
		if len(dualMin) != 3 || len(dualUser) != 3 {
			t.Fatalf("len(dualMin)=%d len(dualUser)=%d, want 3", len(dualMin), len(dualUser))
		}
		if dualMin[1].Sign() < 0 {
			t.Fatalf("GreaterEqual row dualMin[1]=%v < 0", dualMin[1])
		}
		if dualMin[2].Sign() > 0 {
			t.Fatalf("LessEqual row dualMin[2]=%v > 0", dualMin[2])
		}

		sol, ok := ExactBasisSolution(t.Context(), lp)
		if !ok {
			t.Fatal("ExactBasisSolution returned false")
		}
		if sol.ReducedCostMin[0].Sign() != 0 {
			t.Fatalf("free variable exact reduced cost=%v, want 0", sol.ReducedCostMin[0])
		}
		if sol.ReducedCostMin[1].Sign() < 0 {
			t.Fatalf("[0,+Inf) variable exact reduced cost=%v < 0", sol.ReducedCostMin[1])
		}
		if sol.ReducedCostMin[2].Sign() > 0 {
			t.Fatalf("(-Inf,0] variable exact reduced cost=%v > 0", sol.ReducedCostMin[2])
		}

		safeBnd, ok := ExactSafeDualBound(p, sol.DualMin)
		if !ok {
			t.Fatal("ExactSafeDualBound failed on free/semi-bounded LP")
		}
		if safeBnd.Cmp(sol.ObjectiveMin) != 0 {
			t.Fatalf("safeDualBound=%v != exact ObjectiveMin=%v", safeBnd, sol.ObjectiveMin)
		}

		if !PolishExactBasisDuals(t.Context(), lp, &res) {
			t.Fatal("PolishExactBasisDuals returned false")
		}
		wantObj, _ := sol.ObjectiveUser.Float64()
		if math.Abs(res.Objective-wantObj) > 1e-12 {
			t.Fatalf("polished Objective=%.16g, want %.16g", res.Objective, wantObj)
		}

		if os.Getenv("GOBLIN_FORMAL") == "1" {
			formalDir := findFormalDir(t)
			lpCheckBin := filepath.Join(formalDir, ".lake", "build", "bin", "lp-check")
			if info, err := os.Stat(lpCheckBin); err == nil && !info.IsDir() {
				payload := compiledProblemToLPPayload(t, p, "optimal")
				payload.X = ratSliceToPayloads(sol.Primal)
				payload.Y = ratSliceToPayloads(sol.DualMin)
				out, runErr := runLPCheck(t, lpCheckBin, "free_nondyadic_opt", payload, true)
				if runErr != nil || !strings.Contains(out, "VERIFIED") {
					t.Fatalf("lp-check rejected exact polished free LP: err=%v out=%s", runErr, out)
				}
			}
		}
	})

	t.Run("maximize_hilbert_nucleus_and_cuts", func(t *testing.T) {
		t.Parallel()
		// 4x4 dense non-dyadic system with free and one-sided variables under Maximize
		// and non-zero ObjConstant so dense nucleus elimination and Maximize sign conventions
		// are exercised together.
		m := NewModel("maximize_hilbert_nucleus", Maximize)
		x0 := m.AddContVar("x0", math.Inf(-1), math.Inf(1))
		x1 := m.AddContVar("x1", 0, math.Inf(1))
		x2 := m.AddContVar("x2", 0, math.Inf(1))
		x3 := m.AddContVar("x3", 0, 10)
		m.SetObjective(NewExpr(x0.T(2), x1.T(5), x2.T(7), x3.T(3)).AddConstant(-17.25))
		m.AddConstraint("c0", NewExpr(x0.T(3), x1.T(2), x2.T(1)).Eq(10))
		m.AddConstraint("c1", NewExpr(x0.T(2), x1.T(5), x2.T(3), x3.T(1)).Le(25))
		m.AddConstraint("c2", NewExpr(x0.T(1), x1.T(3), x2.T(7), x3.T(2)).Le(31))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		res := lp.Solve()
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want StatusOptimal", res.Status)
		}
		sol, ok := ExactBasisSolution(t.Context(), lp)
		if !ok {
			t.Fatal("ExactBasisSolution failed on Maximize Hilbert nucleus")
		}
		for i := range 3 {
			negMin := new(big.Rat).Neg(sol.DualMin[i])
			if sol.DualUser[i].Cmp(negMin) != 0 {
				t.Fatalf("Maximize DualUser[%d]=%v != -DualMin[%d]=%v", i, sol.DualUser[i], i, negMin)
			}
		}
		negObjMin := new(big.Rat).Neg(sol.ObjectiveMin)
		if sol.ObjectiveUser.Cmp(negObjMin) != 0 {
			t.Fatalf("Maximize ObjectiveUser=%v != -ObjectiveMin=%v", sol.ObjectiveUser, negObjMin)
		}

		// Add a tightening dynamic cut row x1 + x2 <= 4 and re-verify exact basis solution.
		lp.AddCut([]int{1, 2}, []float64{1, 1}, 4.0)
		resCut := lp.Solve()
		if resCut.Status != StatusOptimal {
			t.Fatalf("post-cut status=%v, want StatusOptimal", resCut.Status)
		}
		solCut, okCut := ExactBasisSolution(t.Context(), lp)
		if !okCut {
			t.Fatal("ExactBasisSolution failed after AddCut")
		}
		if len(solCut.DualMin) != 4 {
			t.Fatalf("len(solCut.DualMin)=%d, want 4", len(solCut.DualMin))
		}
		if solCut.DualMin[3].Sign() > 0 {
			t.Fatalf("cut row dualMin=%v > 0", solCut.DualMin[3])
		}
	})

	t.Run("unconstrained_and_negative_controls", func(t *testing.T) {
		t.Parallel()
		m0 := NewModel("unconstrained_boxed", Minimize)
		u0 := m0.AddContVar("u0", -2, 5)
		u1 := m0.AddContVar("u1", 1, math.Inf(1))
		m0.SetObjective(NewExpr(u0.T(-3), u1.T(4)).AddConstant(1.5))
		p0 := m0.Compile()

		lp0 := NewLPSolver(p0, nil)
		defer lp0.Close()
		r0 := lp0.Solve()
		if r0.Status != StatusOptimal {
			t.Fatalf("unconstrained status=%v, want StatusOptimal", r0.Status)
		}
		sol0, ok0 := ExactBasisSolution(t.Context(), lp0)
		if !ok0 {
			t.Fatal("ExactBasisSolution failed on m == 0 LP")
		}
		// -3 * 5 + 4 * 1 + 1.5 = -9.5 = -19/2.
		if sol0.ObjectiveUser.Cmp(big.NewRat(-19, 2)) != 0 {
			t.Fatalf("ObjectiveUser=%v, want -19/2", sol0.ObjectiveUser)
		}

		ctxCanceled, cancel := context.WithCancel(t.Context())
		cancel()
		if _, _, ok := ExactBasisDuals(ctxCanceled, lp0); ok {
			t.Fatal("expected canceled context to reject ExactBasisDuals")
		}
		if _, _, ok := ExactBasisDuals(t.Context(), nil); ok {
			t.Fatal("expected nil solver to reject ExactBasisDuals")
		}

		// Unbounded LP must not claim dual feasibility in ExactBasisDuals.
		mUnb := NewModel("unb_neg_ctrl", Minimize)
		xu := mUnb.AddContVar("x", 0, math.Inf(1))
		mUnb.SetObjective(NewExpr(xu.T(-1)))
		mUnb.AddConstraint("c", NewExpr(xu.T(1)).Ge(1))
		lpUnb := NewLPSolver(mUnb.Compile(), nil)
		defer lpUnb.Close()
		_ = lpUnb.Solve()
		if _, _, ok := ExactBasisDuals(t.Context(), lpUnb); ok {
			t.Fatal("expected ExactBasisDuals to return false on unbounded LP")
		}
	})
}

func TestFormalTrack3BUnboundedCertificates(t *testing.T) {
	t.Parallel()

	t.Run("lp_structural_slack_free_and_unconstrained_rays", func(t *testing.T) {
		t.Parallel()
		// 1. Structural ray under Maximize: max 2*x + y s.t. x - y <= 3, x, y >= 0.
		m1 := NewModel("lp_unb_struct_max", Maximize)
		x1 := m1.AddContVar("x", 0, math.Inf(1))
		y1 := m1.AddContVar("y", 0, math.Inf(1))
		m1.SetObjective(NewExpr(x1.T(2), y1.T(1)))
		m1.AddConstraint("c", NewExpr(x1.T(1), y1.T(-1)).Le(3))
		p1 := m1.Compile()

		cert1, ok1 := ExtractUnboundedCertificate(t.Context(), p1, nil)
		if !ok1 || cert1.IsMIP || !VerifyUnboundedCertificate(p1, cert1) {
			t.Fatalf("p1 certificate failed: ok=%v cert=%+v", ok1, cert1)
		}

		// 2. Slack-driven ray: min -5e-13*x0 + x1 s.t. x1 + 500*x0 >= 1, x0 >= 0, x1 in [0, 10].
		m2 := NewModel("lp_unb_slack_ray", Minimize)
		kx0 := m2.AddContVar("x0", 0, math.Inf(1))
		kx1 := m2.AddContVar("x1", 0, 10)
		m2.SetObjective(NewExpr(kx0.T(-5e-13), kx1.T(1)))
		m2.AddConstraint("ge", NewExpr(kx1.T(1), kx0.T(500)).Ge(1))
		p2 := m2.Compile()

		lp2 := NewLPSolver(p2, nil)
		defer lp2.Close()
		if r2 := lp2.Solve(); r2.Status != StatusUnbounded {
			t.Fatalf("lp2 status=%v, want StatusUnbounded", r2.Status)
		}
		cert2, ok2 := ExtractUnboundedCertificate(t.Context(), p2, lp2)
		if !ok2 || !VerifyUnboundedCertificate(p2, cert2) {
			t.Fatalf("slack-driven LP ray extraction failed: ok=%v cert=%+v", ok2, cert2)
		}

		// 3. Free variable decreasing to -Inf: min 5*xf s.t. 3*xf - y = 6, y in [0, +Inf).
		// Ray: xf = -1, y = -3 is blocked since y >= 0; wait: 3*xf + y = 6 with y >= 0 allows xf -> -Inf, y -> +Inf!
		m3 := NewModel("lp_unb_free_neg", Minimize)
		xf := m3.AddContVar("xf", math.Inf(-1), math.Inf(1))
		yp := m3.AddContVar("yp", 0, math.Inf(1))
		m3.SetObjective(NewExpr(xf.T(5), yp.T(1)))
		m3.AddConstraint("eq", NewExpr(xf.T(3), yp.T(1)).Eq(6))
		p3 := m3.Compile()

		cert3, ok3 := ExtractUnboundedCertificate(t.Context(), p3, nil)
		if !ok3 || !VerifyUnboundedCertificate(p3, cert3) {
			t.Fatalf("free-var LP ray extraction failed: ok=%v cert=%+v", ok3, cert3)
		}
		if cert3.Ray[0].Sign() >= 0 {
			t.Fatalf("expected xf ray < 0, got %v", cert3.Ray[0])
		}

		// 4. Unconstrained m == 0 LP.
		m4 := NewModel("lp_unb_m0", Minimize)
		u := m4.AddContVar("u", math.Inf(-1), 5)
		m4.SetObjective(NewExpr(u.T(2)))
		p4 := m4.Compile()
		cert4, ok4 := ExtractUnboundedCertificate(t.Context(), p4, nil)
		if !ok4 || !VerifyUnboundedCertificate(p4, cert4) {
			t.Fatalf("m==0 LP ray extraction failed: ok=%v cert=%+v", ok4, cert4)
		}
	})

	t.Run("milp_meyer_lcm_ray_scaling_and_mixed_anchor", func(t *testing.T) {
		t.Parallel()
		// 1. Rational LP ray (1, 3/2) scaled by LCM(1, 2) = 2 to integer ray (2, 3):
		// min -x s.t. 3*x - 2*y = 0, x >= 1, y >= 0, x, y integer.
		m1 := NewModel("milp_lcm_2_3", Minimize)
		x1 := m1.AddIntVar("x", 1, math.Inf(1))
		y1 := m1.AddIntVar("y", 0, math.Inf(1))
		m1.SetObjective(NewExpr(x1.T(-1)))
		m1.AddConstraint("eq", NewExpr(x1.T(3), y1.T(-2)).Eq(0))
		p1 := m1.Compile()

		cert1, ok1 := ExtractUnboundedCertificate(t.Context(), p1, nil)
		if !ok1 || !cert1.IsMIP || !VerifyUnboundedCertificate(p1, cert1) {
			t.Fatalf("milp_lcm_2_3 certificate failed: ok=%v cert=%+v", ok1, cert1)
		}
		if !cert1.X0[0].IsInt() || !cert1.X0[1].IsInt() || !cert1.Ray[0].IsInt() || !cert1.Ray[1].IsInt() {
			t.Fatalf("expected integral X0 and Ray, got X0=%v Ray=%v", cert1.X0, cert1.Ray)
		}

		// 2. Multi-column LCM scaling + non-dyadic continuous variable c = x / 3:
		// max x + y s.t. 3*x - 5*y = 1, 2*x - 7*z = 0, 3*c - x = 0, x,y,z in Z_{>=0}, c in [0,+Inf).
		// Feasible anchor e.g. x=7, y=4, z=2, c=7/3; integer ray proportional to (35, 21, 10, 35/3).
		m2 := NewModel("milp_lcm_multi_mixed", Maximize)
		x2 := m2.AddIntVar("x", 0, math.Inf(1))
		y2 := m2.AddIntVar("y", 0, math.Inf(1))
		z2 := m2.AddIntVar("z", 0, math.Inf(1))
		c2 := m2.AddContVar("c", 0, math.Inf(1))
		m2.SetObjective(NewExpr(x2.T(1), y2.T(1)))
		m2.AddConstraint("e1", NewExpr(x2.T(3), y2.T(-5)).Eq(1))
		m2.AddConstraint("e2", NewExpr(x2.T(2), z2.T(-7)).Eq(0))
		m2.AddConstraint("ec", NewExpr(c2.T(3), x2.T(-1)).Eq(0))
		p2 := m2.Compile()

		cert2, ok2 := ExtractUnboundedCertificate(t.Context(), p2, nil)
		if !ok2 || !cert2.IsMIP || !VerifyUnboundedCertificate(p2, cert2) {
			t.Fatalf("milp_lcm_multi_mixed certificate failed: ok=%v cert=%+v", ok2, cert2)
		}
		for _, intIdx := range []int{0, 1, 2} {
			if !cert2.X0[intIdx].IsInt() || !cert2.Ray[intIdx].IsInt() {
				t.Fatalf("col %d not integral: x0=%v ray=%v", intIdx, cert2.X0[intIdx], cert2.Ray[intIdx])
			}
		}
	})

	t.Run("negative_controls_bounded_and_diophantine_cliffs", func(t *testing.T) {
		t.Parallel()
		// 1. Strictly bounded MILP where float64 cancellation 1 + 2^-54 looks unbounded.
		mFake := NewModel("fake_unb_bounded", Minimize)
		xf := mFake.AddIntVar("x", 0, math.Inf(1))
		yf := mFake.AddIntVar("y", 0, math.Inf(1))
		wf := mFake.AddContVar("w", 0, math.Inf(1))
		mFake.SetObjective(NewExpr(xf.T(-1)))
		mFake.AddConstraint("r0", NewExpr(yf.T(1), xf.T(-1)).Eq(0))
		mFake.AddConstraint("r1", NewExpr(wf.T(1), yf.T(-1), xf.T(-math.Ldexp(1, -54))).Eq(0))
		mFake.AddConstraint("r2", NewExpr(wf.T(math.Ldexp(1, 24)), yf.T(-math.Ldexp(1, 24)), xf.T(-math.Ldexp(1, -31))).Le(1))
		if _, ok := ExtractUnboundedCertificate(t.Context(), mFake.Compile(), nil); ok {
			t.Fatal("expected bounded fake-ray MILP to reject ExtractUnboundedCertificate")
		}

		// 2. Diophantine-infeasible LP-unbounded MILP: 3x - 3y = 1.
		mDio := NewModel("dio_3x_3y_1", Minimize)
		xd := mDio.AddIntVar("x", 0, math.Inf(1))
		yd := mDio.AddIntVar("y", 0, math.Inf(1))
		mDio.SetObjective(NewExpr(xd.T(-1)))
		mDio.AddConstraint("eq", NewExpr(xd.T(3), yd.T(-3)).Eq(1))
		if _, ok := ExtractUnboundedCertificate(t.Context(), mDio.Compile(), nil); ok {
			t.Fatal("expected Diophantine-infeasible 3x-3y=1 to reject ExtractUnboundedCertificate")
		}

		// 3. Tolerance-cliff Diophantine-infeasible MILP: 2x - 2y = 5e-7 has empty integer hull over Q.
		mCliff := NewModel("dio_2x_2y_5e7", Minimize)
		xc := mCliff.AddIntVar("x", 0, math.Inf(1))
		yc := mCliff.AddIntVar("y", 0, math.Inf(1))
		mCliff.SetObjective(NewExpr(xc.T(-1)))
		mCliff.AddConstraint("eq", NewExpr(xc.T(2), yc.T(-2)).Eq(5e-7))
		pCliff := mCliff.Compile()
		if _, ok := ExtractUnboundedCertificate(t.Context(), pCliff, nil, []float64{0, 0}); ok {
			t.Fatal("expected exact-Q infeasible 2x-2y=5e-7 to reject ExtractUnboundedCertificate")
		}

		// 4. Tampered fractional ray must fail VerifyUnboundedCertificate on MILP.
		mValid := NewModel("valid_milp_tamper", Minimize)
		xv := mValid.AddIntVar("x", 0, math.Inf(1))
		yv := mValid.AddIntVar("y", 0, math.Inf(1))
		mValid.SetObjective(NewExpr(xv.T(-1)))
		mValid.AddConstraint("eq", NewExpr(xv.T(3), yv.T(-2)).Eq(0))
		pValid := mValid.Compile()
		badCert := UnboundedCertificate{
			IsMIP:    true,
			X0:       []*big.Rat{big.NewRat(0, 1), big.NewRat(0, 1)},
			Ray:      []*big.Rat{big.NewRat(1, 1), big.NewRat(3, 2)},
			ObjSlope: big.NewRat(-1, 1),
		}
		if VerifyUnboundedCertificate(pValid, badCert) {
			t.Fatal("VerifyUnboundedCertificate accepted fractional MILP ray")
		}
	})
}

func TestFormalTrack3BGcdTinyCoeffAndPresolveFallback(t *testing.T) {
	t.Parallel()

	t.Run("gcd_row_div_must_not_zero_tiny_nonzero_coeffs", func(t *testing.T) {
		t.Parallel()
		// Regression for presolve.go GCD reduction: 2*z + 1e-10*x + 1e-10*y <= 0 with z in {0,1}
		// and x, y in {0..5} forces z = x = y = 0 over Q, and linking y_big - 1e9*x <= 0 scales
		// any false x > 0 to Objective >= 1e9. Rounding 1e-10 to 0 during division by g = 2
		// previously erased x and y before Savelsbergh tightening and returned Objective = 5e9.
		m := NewModel("gcd_tiny_coeff_g2", Maximize)
		z := m.AddBinaryVar("z")
		x := m.AddIntVar("x", 0, 5)
		y := m.AddIntVar("y", 0, 5)
		yBig := m.AddContVar("y_big", 0, 5e9)
		m.SetObjective(NewExpr(yBig.T(1), y.T(1)))
		m.AddConstraint("c", NewExpr(z.T(2), x.T(1e-10), y.T(1e-10)).Le(0))
		m.AddConstraint("link", NewExpr(yBig.T(1), x.T(-1e9)).Le(0))
		p := m.Compile()

		pre := Presolve(p, defaultPresolveMaxPass)
		if pre.Infeasible {
			t.Fatal("unexpected presolve infeasibility on gcd_tiny_coeff_g2")
		}
		if pre.Reduced != nil && (pre.Reduced.varUB[1] > 0 || pre.Reduced.varUB[2] > 0) {
			bV, rV, _ := pre.Reduced.MaxViolation([]float64{0, 5, 5, 5e9}, true)
			if bV == 0 && rV == 0 {
				t.Fatal("GCD division erased 1e-10 coefficients on x and y")
			}
		}

		res, err := SolveCompiled(t.Context(), p, WithPresolve(true))
		if err != nil {
			t.Fatalf("g2 presolve=true err: %v", err)
		}
		if res.Status != StatusOptimal || math.Abs(res.Objective) > 1e-9 {
			t.Fatalf("g2 presolve=true: status=%v obj=%.12g, want Optimal 0", res.Status, res.Objective)
		}

		tr := NewVIPRTrace()
		resVIPR, err := SolveCompiled(t.Context(), p, WithPresolve(true), WithVIPRTrace(tr))
		if err != nil || resVIPR.Status != StatusOptimal || math.Abs(resVIPR.Objective) > 1e-9 {
			t.Fatalf("g2 VIPR solve: status=%v obj=%.12g err=%v", resVIPR.Status, resVIPR.Objective, err)
		}
		assertVIPRVerifiedIfFormal(t, tr, "optimal")

		// Companion g == 1 case: z + 1e-10*x <= 0 with z in {0,1}, x in {0..5}, y_big <= 1e9*x -> 0.
		m1 := NewModel("gcd_tiny_coeff_g1", Maximize)
		z1 := m1.AddBinaryVar("z")
		x1 := m1.AddIntVar("x", 0, 5)
		yBig1 := m1.AddContVar("y_big", 0, 5e9)
		m1.SetObjective(NewExpr(yBig1.T(1)))
		m1.AddConstraint("c", NewExpr(z1.T(1), x1.T(1e-10)).Le(0))
		m1.AddConstraint("link", NewExpr(yBig1.T(1), x1.T(-1e9)).Le(0))
		p1 := m1.Compile()

		res1, err := SolveCompiled(t.Context(), p1, WithPresolve(true))
		if err != nil {
			t.Fatalf("g1 presolve=true err: %v", err)
		}
		if res1.Status != StatusOptimal || math.Abs(res1.Objective) > 1e-9 {
			t.Fatalf("g1 presolve=true: status=%v obj=%.12g, want Optimal 0", res1.Status, res1.Objective)
		}
	})

	t.Run("lp_presolve_postsolve_duals_and_vipr_trace_reset", func(t *testing.T) {
		t.Parallel()
		// Exercises fixed substitution (x_fix == 3), singleton row absorption (2*x_sng <= 10),
		// dual unblocked column fixing (x_dual in [2, 8] with positive cost and >= row coeff <= 0),
		// and redundant row removal (x_sng <= 100) under Maximize with ObjConstant.
		m := NewModel("lp_presolve_full_postsolve", Maximize)
		xFix := m.AddContVar("x_fix", 3, 3)
		xSng := m.AddContVar("x_sng", 0, 20)
		xDual := m.AddContVar("x_dual", 2, 8)
		xMain := m.AddContVar("x_main", 0, 10)
		m.SetObjective(NewExpr(xFix.T(4), xSng.T(3), xDual.T(-2), xMain.T(5)).AddConstant(7.5))
		m.AddConstraint("sng", NewExpr(xSng.T(2)).Le(10))
		m.AddConstraint("couple", NewExpr(xFix.T(1), xSng.T(1), xMain.T(2)).Le(16))
		m.AddConstraint("ge_dual", NewExpr(xDual.T(-1), xMain.T(1)).Ge(-6))
		m.AddConstraint("red", NewExpr(xMain.T(1)).Le(100))
		p := m.Compile()

		resOff, err := SolveCompiled(t.Context(), p, WithPresolve(false))
		if err != nil || resOff.Status != StatusOptimal {
			t.Fatalf("presolve=false: status=%v err=%v", resOff.Status, err)
		}
		resOn, err := SolveCompiled(t.Context(), p, WithPresolve(true))
		if err != nil || resOn.Status != StatusOptimal {
			t.Fatalf("presolve=true: status=%v err=%v", resOn.Status, err)
		}
		if math.Abs(resOn.Objective-resOff.Objective) > 1e-9 {
			t.Fatalf("presolve Objective=%.12g != unpresolved=%.12g", resOn.Objective, resOff.Objective)
		}
		if resOn.BestBound != resOn.Objective || resOn.RootLPBound != resOn.Objective || resOn.RootCutBound != resOn.Objective {
			t.Fatalf("LP bounds out of sync after postsolve: obj=%g best=%g rootLP=%g rootCut=%g",
				resOn.Objective, resOn.BestBound, resOn.RootLPBound, resOn.RootCutBound)
		}
		if len(resOn.Duals) != p.numConstraints {
			t.Fatalf("len(Duals)=%d, want %d", len(resOn.Duals), p.numConstraints)
		}
		// Under Maximize, canonical yMin = -Duals: LessEqual => Duals >= 0, GreaterEqual => Duals <= 0.
		if resOn.Duals[0] < 0 || resOn.Duals[1] < 0 || resOn.Duals[3] < 0 {
			t.Fatalf("Maximize LessEqual duals negative: %v", resOn.Duals)
		}
		if resOn.Duals[2] > 0 {
			t.Fatalf("Maximize GreaterEqual dual positive: %v", resOn.Duals)
		}

		// Reusing a populated VIPRTrace on an immediate TimeLimit exit must clear stale state.
		tr := NewVIPRTrace()
		resTr, err := SolveCompiled(t.Context(), p, WithPresolve(false), WithVIPRTrace(tr))
		if err != nil || resTr.Status != StatusOptimal || !tr.HasCertificate() {
			t.Fatalf("initial VIPR LP solve failed: status=%v hasCert=%v err=%v", resTr.Status, tr.HasCertificate(), err)
		}
		resTL, err := SolveCompiled(t.Context(), p, WithTimeLimit(-time.Second), WithVIPRTrace(tr))
		if err != nil || resTL.Status != StatusTimeLimit {
			t.Fatalf("negative TimeLimit solve: status=%v err=%v", resTL.Status, err)
		}
		if tr.HasCertificate() {
			t.Fatal("stale VIPR certificate retained after StatusTimeLimit early return")
		}
	})
}

func TestFormalTrack3BLeanOracleEndToEnd(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)

	// 1. Generate LP unboundedness certificate JSON.
	mLP := NewModel("lean_lp_unb", Minimize)
	lx := mLP.AddContVar("x", 0, math.Inf(1))
	ly := mLP.AddContVar("y", 0, 10)
	mLP.SetObjective(NewExpr(lx.T(-3), ly.T(2)).AddConstant(5))
	mLP.AddConstraint("ge", NewExpr(lx.T(5), ly.T(1)).Ge(2))
	pLP := mLP.Compile()
	certLP, ok := ExtractUnboundedCertificate(t.Context(), pLP, nil)
	if !ok {
		t.Fatal("ExtractUnboundedCertificate failed on pLP")
	}
	lpUnbJSON, err := json.Marshal(lpUnboundedCertPayload{
		lpCertPayload: compiledProblemToLPPayload(t, pLP, "unbounded"),
		X0:            ratSliceToPayloads(certLP.X0),
		D:             ratSliceToPayloads(certLP.Ray),
	})
	if err != nil {
		t.Fatalf("marshal lpUnbJSON: %v", err)
	}

	// 2. Generate MILP Meyer unboundedness certificate JSON (with LCM scaling 3x - 2y = 0).
	mMIP := NewModel("lean_milp_unb", Minimize)
	mx := mMIP.AddIntVar("x", 2, math.Inf(1))
	my := mMIP.AddIntVar("y", 0, math.Inf(1))
	mc := mMIP.AddContVar("c", 0, math.Inf(1))
	mMIP.SetObjective(NewExpr(mx.T(-2), my.T(1), mc.T(0)))
	mMIP.AddConstraint("dio", NewExpr(mx.T(3), my.T(-2)).Eq(0))
	mMIP.AddConstraint("cont", NewExpr(mc.T(3), mx.T(-1)).Eq(0))
	pMIP := mMIP.Compile()
	certMIP, ok := ExtractUnboundedCertificate(t.Context(), pMIP, nil)
	if !ok {
		t.Fatal("ExtractUnboundedCertificate failed on pMIP")
	}
	mipUnbJSON, err := json.Marshal(lpUnboundedCertPayload{
		lpCertPayload: compiledProblemToLPPayload(t, pMIP, "milp-unbounded"),
		X0:            ratSliceToPayloads(certMIP.X0),
		D:             ratSliceToPayloads(certMIP.Ray),
		IntCols:       []int{0, 1},
	})
	if err != nil {
		t.Fatalf("marshal mipUnbJSON: %v", err)
	}

	// Tampered MILP payload with fractional ray (1, 3/2, 1/3) must be rejected by Lean.
	mipBadJSON, err := json.Marshal(lpUnboundedCertPayload{
		lpCertPayload: compiledProblemToLPPayload(t, pMIP, "milp-unbounded"),
		X0: []ratPayload{
			bigRatToPayload(big.NewRat(2, 1)),
			bigRatToPayload(big.NewRat(3, 1)),
			bigRatToPayload(big.NewRat(2, 3)),
		},
		D: []ratPayload{
			bigRatToPayload(big.NewRat(1, 1)),
			bigRatToPayload(big.NewRat(3, 2)),
			bigRatToPayload(big.NewRat(1, 3)),
		},
		IntCols: []int{0, 1},
	})
	if err != nil {
		t.Fatalf("marshal mipBadJSON: %v", err)
	}

	// 3. LP presolve singletonRow + postsolve optimal certificate JSON.
	mOrig := NewModel("lean_presolve_orig", Minimize)
	ox0 := mOrig.AddContVar("x0", 0, 10)
	ox1 := mOrig.AddContVar("x1", 0, 10)
	mOrig.SetObjective(NewExpr(ox0.T(-2), ox1.T(-1)).AddConstant(4))
	mOrig.AddConstraint("sng", NewExpr(ox0.T(2)).Le(6))
	mOrig.AddConstraint("cap", NewExpr(ox0.T(1), ox1.T(1)).Le(8))
	pOrig := mOrig.Compile()

	mRed := NewModel("lean_presolve_red", Minimize)
	rx0 := mRed.AddContVar("x0", 0, 3)
	rx1 := mRed.AddContVar("x1", 0, 10)
	mRed.SetObjective(NewExpr(rx0.T(-2), rx1.T(-1)).AddConstant(4))
	mRed.AddConstraint("cap", NewExpr(rx0.T(1), rx1.T(1)).Le(8))
	pRed := mRed.Compile()

	lpRed := NewLPSolver(pRed, nil)
	defer lpRed.Close()
	if r := lpRed.Solve(); r.Status != StatusOptimal {
		t.Fatalf("lpRed status=%v", r.Status)
	}
	solRed, ok := ExactBasisSolution(t.Context(), lpRed)
	if !ok {
		t.Fatal("ExactBasisSolution failed on pRed")
	}
	idxZero := 0
	presolveJSON, err := json.Marshal(lpPresolveCertPayload{
		Orig:    compiledProblemToLPPayload(t, pOrig, "lp-presolve"),
		Reduced: compiledProblemToLPPayload(t, pRed, "optimal"),
		Step: lpPresolveStepPayload{
			Kind:       "singletonRow",
			OrigRow:    &idxZero,
			Col:        &idxZero,
			OrigRowIdx: []int{1},
		},
		XReduced: ratSliceToPayloads(solRed.Primal),
		YReduced: ratSliceToPayloads(solRed.DualMin),
	})
	if err != nil {
		t.Fatalf("marshal presolveJSON: %v", err)
	}

	// 3b. LP presolve dualColFix (toLower=false -> fixing to upper bound u=5) + postsolve optimal JSON.
	mDualOrig := NewModel("lean_dual_fix_orig", Minimize)
	dx0 := mDualOrig.AddContVar("x0", 0, 5)
	dx1 := mDualOrig.AddContVar("x1", 0, 4)
	mDualOrig.SetObjective(NewExpr(dx0.T(-3), dx1.T(2)).AddConstant(1))
	mDualOrig.AddConstraint("le", NewExpr(dx0.T(-2), dx1.T(1)).Le(6))
	pDualOrig := mDualOrig.Compile()

	mDualRed := NewModel("lean_dual_fix_red", Minimize)
	drx0 := mDualRed.AddContVar("x0", 5, 5)
	drx1 := mDualRed.AddContVar("x1", 0, 4)
	mDualRed.SetObjective(NewExpr(drx0.T(-3), drx1.T(2)).AddConstant(1))
	mDualRed.AddConstraint("le", NewExpr(drx0.T(-2), drx1.T(1)).Le(6))
	pDualRed := mDualRed.Compile()

	lpDualRed := NewLPSolver(pDualRed, nil)
	defer lpDualRed.Close()
	if r := lpDualRed.Solve(); r.Status != StatusOptimal {
		t.Fatalf("lpDualRed status=%v", r.Status)
	}
	solDualRed, ok := ExactBasisSolution(t.Context(), lpDualRed)
	if !ok {
		t.Fatal("ExactBasisSolution failed on pDualRed")
	}
	boolFalseStep := false
	dualFixJSON, err := json.Marshal(lpPresolveCertPayload{
		Orig:    compiledProblemToLPPayload(t, pDualOrig, "lp-presolve"),
		Reduced: compiledProblemToLPPayload(t, pDualRed, "optimal"),
		Step: lpPresolveStepPayload{
			Kind:    "dualColFix",
			Col:     &idxZero,
			ToLower: &boolFalseStep,
		},
		XReduced: ratSliceToPayloads(solDualRed.Primal),
		YReduced: ratSliceToPayloads(solDualRed.DualMin),
	})
	if err != nil {
		t.Fatalf("marshal dualFixJSON: %v", err)
	}

	// 4. GCD row division JSON (both feasible inequality tightening and infeasible equality).
	boolFalse := false
	boolTrue := true
	divRhsWant := int64(2)
	gcdFeasJSON, err := json.Marshal(gcdDivCheckPayload{
		Coeffs:           []int64{6, -9, 15},
		Sense:            "le",
		RHS:              bigRatToPayload(big.NewRat(17, 2)), // 8.5 / 3 -> floor = 2
		ExpectInfeasible: &boolFalse,
		DivCoeffs:        []int64{2, -3, 5},
		DivRHS:           &divRhsWant,
	})
	if err != nil {
		t.Fatalf("marshal gcdFeasJSON: %v", err)
	}
	gcdInfeasJSON, err := json.Marshal(gcdDivCheckPayload{
		Coeffs:           []int64{2, -2},
		Sense:            "eq",
		RHS:              bigRatToPayload(big.NewRat(1, 2000000)), // 5e-7
		ExpectInfeasible: &boolTrue,
	})
	if err != nil {
		t.Fatalf("marshal gcdInfeasJSON: %v", err)
	}

	tmpDir := t.TempDir()
	writeCase := func(name string, data []byte) string {
		p := filepath.Join(tmpDir, name)
		if wErr := os.WriteFile(p, data, 0o600); wErr != nil {
			t.Fatalf("write %s: %v", name, wErr)
		}
		return filepath.ToSlash(p)
	}
	p1Path := writeCase("lp_unb.json", lpUnbJSON)
	p2Path := writeCase("mip_unb.json", mipUnbJSON)
	pBadPath := writeCase("mip_bad.json", mipBadJSON)
	p3Path := writeCase("presolve.json", presolveJSON)
	p3bPath := writeCase("dual_fix.json", dualFixJSON)
	p4Path := writeCase("gcd_feas.json", gcdFeasJSON)
	p5Path := writeCase("gcd_infeas.json", gcdInfeasJSON)

	formalDir := findFormalDir(t)
	track3BSrc, err := os.ReadFile(filepath.Join(formalDir, "GoblinFormal", "Track3B.lean"))
	if err != nil {
		t.Fatalf("read Track3B.lean: %v", err)
	}
	leanHarness := string(track3BSrc) + "\n" + strings.Join([]string{
		"open GoblinFormal",
		"open GoblinFormal.Track3B",
		"def runChecks : IO Unit := do",
		"  let s1 ← IO.FS.readFile \"" + p1Path + "\"",
		"  match parseJson s1 >>= verifyLPUnboundedRootJson with",
		"  | .ok () => pure ()",
		"  | .error e => throw (IO.userError s!\"lp_unb failed: {e}\")",
		"  let s2 ← IO.FS.readFile \"" + p2Path + "\"",
		"  match parseJson s2 >>= verifyMILPUnboundedRootJson with",
		"  | .ok () => pure ()",
		"  | .error e => throw (IO.userError s!\"mip_unb failed: {e}\")",
		"  let sBad ← IO.FS.readFile \"" + pBadPath + "\"",
		"  match parseJson sBad >>= verifyMILPUnboundedRootJson with",
		"  | .ok () => throw (IO.userError \"mip_bad unexpectedly accepted\")",
		"  | .error _ => pure ()",
		"  let s3 ← IO.FS.readFile \"" + p3Path + "\"",
		"  match parseJson s3 >>= verifyLPPresolveJson with",
		"  | .ok (some b) =>",
		"    if b != (-7 : Rat) then throw (IO.userError s!\"presolve bound {b} != -7\")",
		"  | .ok none => throw (IO.userError \"presolve returned none bound\")",
		"  | .error e => throw (IO.userError s!\"presolve failed: {e}\")",
		"  let s3b ← IO.FS.readFile \"" + p3bPath + "\"",
		"  match parseJson s3b >>= verifyLPPresolveJson with",
		"  | .ok (some b) =>",
		"    if b != (-14 : Rat) then throw (IO.userError s!\"dual_fix bound {b} != -14\")",
		"  | .ok none => throw (IO.userError \"dual_fix returned none bound\")",
		"  | .error e => throw (IO.userError s!\"dual_fix failed: {e}\")",
		"  let s4 ← IO.FS.readFile \"" + p4Path + "\"",
		"  match parseJson s4 >>= handleGcdRowDivJson with",
		"  | .ok _ => pure ()",
		"  | .error e => throw (IO.userError s!\"gcd_feas failed: {e}\")",
		"  let s5 ← IO.FS.readFile \"" + p5Path + "\"",
		"  match parseJson s5 >>= handleGcdRowDivJson with",
		"  | .ok _ => pure ()",
		"  | .error e => throw (IO.userError s!\"gcd_infeas failed: {e}\")",
		"  IO.println \"TRACK3B_ORACLE_VERIFIED\"",
		"#eval runChecks",
		"",
	}, "\n")

	lakeBin := locateLakeBinary(t)
	_ = locateLeanBinary3B(t)
	cmd := exec.CommandContext(t.Context(), lakeBin, "env", "lean", "--stdin")
	cmd.Dir = formalDir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(lakeBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = bytes.NewBufferString(leanHarness)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "TRACK3B_ORACLE_VERIFIED") {
		t.Fatalf("Track3B Lean end-to-end oracle failed: err=%v\n%s", err, out)
	}
}

func TestFormalTrack3BLiveLPPresolveAndNegativeControls(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	lpExe := formalExe(t, "lp-check")

	runRawLPCheck := func(name string, subcmd string, raw []byte, printBound bool) (string, error) {
		p := filepath.Join(t.TempDir(), name+".json")
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		var args []string
		if subcmd != "" {
			args = append(args, subcmd)
		}
		if printBound {
			args = append(args, "--print-bound")
		}
		args = append(args, p)
		cmd := exec.CommandContext(t.Context(), lpExe, args...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("live_multistep_lp_presolve_chains", func(t *testing.T) {
		t.Parallel()
		// 1. Multi-pass Maximize LP with ObjConstant = 7.5 exercising fixedSubst,
		// singletonRow, dropRedundantRow, and dualColFix -> second-pass fixedSubst + dropRedundantRow.
		mMax := NewModel("live_lp_presolve_max_chain", Maximize)
		xFix := mMax.AddContVar("x_fix", 3, 3)
		xSng := mMax.AddContVar("x_sng", 0, 20)
		xDual := mMax.AddContVar("x_dual", 2, 8)
		xMain := mMax.AddContVar("x_main", 0, 10)
		mMax.SetObjective(NewExpr(xFix.T(4), xSng.T(3), xDual.T(-2), xMain.T(5)).AddConstant(7.5))
		mMax.AddConstraint("sng", NewExpr(xSng.T(2)).Le(10))
		mMax.AddConstraint("couple", NewExpr(xFix.T(1), xSng.T(1), xMain.T(2)).Le(16))
		mMax.AddConstraint("ge_dual", NewExpr(xDual.T(-1), xMain.T(1)).Ge(-6))
		mMax.AddConstraint("red", NewExpr(xMain.T(1)).Le(100))
		pMax := mMax.Compile()

		rawMax, okMax := ExportLPPresolveCheckPayload(pMax)
		if !okMax {
			t.Fatal("ExportLPPresolveCheckPayload returned false on pMax")
		}
		var parsedMax lpPresolveChainCertExport
		if err := json.Unmarshal(rawMax, &parsedMax); err != nil {
			t.Fatalf("unmarshal rawMax: %v", err)
		}
		if len(parsedMax.Steps) < 5 {
			t.Fatalf("expected multi-step chain (>=5 steps), got %d", len(parsedMax.Steps))
		}
		outMax, errMax := runRawLPCheck("live_max_chain", "", rawMax, true)
		if errMax != nil {
			t.Fatalf("lp-check rejected live Maximize presolve chain: err=%v out=%s", errMax, outMax)
		}
		assertLPCheckVerifiedMode(t, "live_max_chain", outMax, "lp-presolve")

		// Check certified bound matches exact basis optimum on original LP.
		lpOrigMax := NewLPSolver(pMax, nil)
		defer lpOrigMax.Close()
		if r := lpOrigMax.Solve(); r.Status != StatusOptimal {
			t.Fatalf("lpOrigMax status=%v", r.Status)
		}
		solOrigMax, okSol := ExactBasisSolution(t.Context(), lpOrigMax)
		if !okSol {
			t.Fatal("ExactBasisSolution failed on pMax")
		}
		gotBnd := parseBoundFromLPCheck(t, outMax)
		if gotBnd.Cmp(solOrigMax.ObjectiveMin) != 0 {
			t.Fatalf("lp-presolve bound=%v != exact ObjectiveMin=%v", gotBnd, solOrigMax.ObjectiveMin)
		}

		// 2. Multi-step Minimize LP exercising zeroObjEmptyColFix, Equal singletonRow,
		// negative-coeff GreaterEqual singletonRow, dualColFix (toLower=false), fixedSubst, and dropRedundantRow.
		mMin := NewModel("live_lp_presolve_min_all_ops", Minimize)
		vZero := mMin.AddContVar("v_zero", -3, 4)
		vEq := mMin.AddContVar("v_eq", 0, 10)
		vNegGe := mMin.AddContVar("v_neg_ge", 0, 10)
		vDualHi := mMin.AddContVar("v_dual_hi", 1, 6)
		vCore := mMin.AddContVar("v_core", 0, 10)
		mMin.SetObjective(NewExpr(vZero.T(0), vEq.T(2), vNegGe.T(-1), vDualHi.T(-3), vCore.T(4)).AddConstant(-2.25))
		mMin.AddConstraint("eq_sng", NewExpr(vEq.T(2)).Eq(4))
		mMin.AddConstraint("neg_ge_sng", NewExpr(vNegGe.T(-2)).Ge(-8))
		mMin.AddConstraint("dual_le", NewExpr(vDualHi.T(-1), vCore.T(1)).Le(20))
		mMin.AddConstraint("main", NewExpr(vEq.T(1), vNegGe.T(1), vCore.T(2)).Ge(9))
		mMin.AddConstraint("red_ge", NewExpr(vCore.T(1)).Ge(-5))
		pMin := mMin.Compile()

		rawMin, okMin := ExportLPPresolveCheckPayload(pMin)
		if !okMin {
			t.Fatal("ExportLPPresolveCheckPayload returned false on pMin")
		}
		var parsedMin lpPresolveChainCertExport
		if err := json.Unmarshal(rawMin, &parsedMin); err != nil {
			t.Fatalf("unmarshal rawMin: %v", err)
		}
		seenKinds := make(map[string]bool)
		for _, st := range parsedMin.Steps {
			seenKinds[st.Kind] = true
		}
		for _, wantKind := range []string{"zeroObjEmptyColFix", "singletonRow", "dualColFix", "fixedSubst", "dropRedundantRow"} {
			if !seenKinds[wantKind] {
				t.Fatalf("expected live chain to contain %q, got kinds=%v", wantKind, seenKinds)
			}
		}
		outMin, errMin := runRawLPCheck("live_min_all_ops", "", rawMin, true)
		if errMin != nil {
			t.Fatalf("lp-check rejected live Minimize all-ops chain: err=%v out=%s", errMin, outMin)
		}
		assertLPCheckVerifiedMode(t, "live_min_all_ops", outMin, "lp-presolve")
	})

	t.Run("negative_controls_t3_6_t3_8_t3_9_t3_10_t3_7", func(t *testing.T) {
		t.Parallel()
		// Setup valid 1-step presolve certificate for T3-6 mutation testing.
		mPre := NewModel("t3_6_base", Minimize)
		px0 := mPre.AddContVar("x0", 0, 10)
		px1 := mPre.AddContVar("x1", 0, 10)
		mPre.SetObjective(NewExpr(px0.T(1), px1.T(1)))
		mPre.AddConstraint("sng", NewExpr(px0.T(1)).Ge(4))
		mPre.AddConstraint("cap", NewExpr(px0.T(1), px1.T(1)).Ge(6))
		rawBase, okBase := ExportLPPresolveCheckPayload(mPre.Compile())
		if !okBase {
			t.Fatal("ExportLPPresolveCheckPayload failed on t3_6_base")
		}

		// T3-6 positive control: feasible xReduced without yReduced succeeds.
		var feasNoY lpPresolveChainCertExport
		if err := json.Unmarshal(rawBase, &feasNoY); err != nil {
			t.Fatalf("unmarshal rawBase: %v", err)
		}
		feasNoY.YReduced = nil
		rawFeasNoY, _ := json.Marshal(feasNoY)
		if out, err := runRawLPCheck("t3_6_feas_no_y", "", rawFeasNoY, false); err != nil {
			t.Fatalf("expected feasible xReduced without yReduced to pass, got err=%v out=%s", err, out)
		}

		// T3-6 negative control: primal-infeasible xReduced without yReduced MUST fail.
		badNoY := feasNoY
		badNoY.XReduced = []ratPayload{int64ToPayload(0), int64ToPayload(0)}
		rawBadNoY, _ := json.Marshal(badNoY)
		if out, err := runRawLPCheck("t3_6_infeas_no_y", "", rawBadNoY, false); err == nil {
			t.Fatalf("T3-6 regression: lp-check accepted infeasible xReduced without yReduced: %s", out)
		}

		// T3-8 negative controls: safe-bound with primalObj/gapTol without x, or negative gapTol.
		mSB := NewModel("t3_8_sb", Minimize)
		sx := mSB.AddContVar("x", 0, 5)
		mSB.SetObjective(NewExpr(sx.T(2)))
		mSB.AddConstraint("ge", NewExpr(sx.T(1)).Ge(2))
		pSB := mSB.Compile()
		sbNoX := compiledProblemToLPPayload(t, pSB, "safe-bound")
		sbNoX.Y = []ratPayload{int64ToPayload(2)}
		pObj := int64ToPayload(4)
		gTol := int64ToPayload(0)
		sbNoX.PrimalObj = &pObj
		sbNoX.GapTol = &gTol
		rawSBNoX, _ := json.Marshal(sbNoX)
		if out, err := runRawLPCheck("t3_8_no_x", "", rawSBNoX, false); err == nil {
			t.Fatalf("T3-8 regression: safe-bound accepted primalObj/gapTol without x: %s", out)
		}

		sbNegGap := sbNoX
		sbNegGap.X = []ratPayload{int64ToPayload(2)}
		negTol := int64ToPayload(-1)
		sbNegGap.GapTol = &negTol
		rawSBNegGap, _ := json.Marshal(sbNegGap)
		if out, err := runRawLPCheck("t3_8_neg_gaptol", "", rawSBNegGap, false); err == nil {
			t.Fatalf("T3-8 regression: safe-bound accepted negative gapTol: %s", out)
		}

		// T3-9 negative controls: CLI subcommand vs JSON mode mismatch & duplicate keys.
		sbPlain := compiledProblemToLPPayload(t, pSB, "safe-bound")
		sbPlain.Y = []ratPayload{int64ToPayload(2)}
		rawSBPlain, _ := json.Marshal(sbPlain)
		if out, err := runRawLPCheck("t3_9_cli_mismatch", "optimal", rawSBPlain, false); err == nil {
			t.Fatalf("T3-9 regression: lp-check optimal accepted JSON mode=safe-bound: %s", out)
		}
		dupKeyJSON := []byte(`{"mode":"farkas","mode":"safe-bound","numVars":1,"numRows":0,"objConst":"0/1","obj":["1/1"],"lb":["0/1"],"ub":["1/1"],"rows":[],"y":[]}`)
		if out, err := runRawLPCheck("t3_9_dup_key", "", dupKeyJSON, false); err == nil {
			t.Fatalf("T3-9 regression: lp-check accepted duplicate JSON key: %s", out)
		}

		// T3-10 negative controls: milp-unbounded with 0 int cols and conflicting isInt/intCols.
		zeroIntUnb := []byte(`{"mode":"milp-unbounded","numVars":1,"numRows":0,"objConst":"0/1","obj":["-1/1"],"lb":["0/1"],"ub":[null],"rows":[],"isInt":[false],"x0":["0/1"],"d":["1/1"]}`)
		if out, err := runRawLPCheck("t3_10_zero_int", "", zeroIntUnb, false); err == nil {
			t.Fatalf("T3-10 regression: milp-unbounded accepted 0 integer columns: %s", out)
		}
		conflictIntUnb := []byte(`{"mode":"milp-unbounded","numVars":2,"numRows":0,"objConst":"0/1","obj":["-1/1","0/1"],"lb":["0/1","0/1"],"ub":[null,"1/1"],"rows":[],"isInt":[true,false],"intCols":[1],"x0":["0/1","0/1"],"d":["1/1","0/1"]}`)
		if out, err := runRawLPCheck("t3_10_conflict_int", "", conflictIntUnb, false); err == nil {
			t.Fatalf("T3-10 regression: milp-unbounded accepted conflicting isInt and intCols: %s", out)
		}

		// T3-7 negative controls on gcd-div-check via rules-check --jsonl.
		for _, badGCD := range [][]byte{
			[]byte(`{"subcommand":"gcd-div-check","op":"gcd-div-check","coeffs":[2,2],"sense":"le","rhs":"399999995/100000000","div_coeffs":[1,1],"div_rhs":2}`),
			[]byte(`{"subcommand":"gcd-div-check","op":"gcd-div-check","coeffs":[6,9],"sense":"le","rhs":"17/2","div_coeffs":[2,3]}`),
			[]byte(`{"subcommand":"gcd-div-check","op":"gcd-div-check","coeffs":[6,9],"sense":"le","rhs":"17/2","divisor":2}`),
		} {
			resp, err := FormalCheckRulesJSONL(t, badGCD)
			if err == nil && resp.OK {
				t.Fatalf("T3-7 regression: rules-check accepted invalid gcd-div-check %s", badGCD)
			}
		}
	})
}
