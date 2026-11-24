package goblin

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func withGoblinCertFault(ctx context.Context, fn goblinCertFaultFunc) context.Context {
	return context.WithValue(ctx, goblinCertFaultKey{}, fn)
}

func TestCertifiedPureLPModes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("OptimalMinimizeMaximizeNonDyadicAndObjConst", func(t *testing.T) {
		t.Parallel()
		// 3*x + 7*y = 5, x >= 0, y >= 0, minimize (0.1/0.3)*x + 2*y + 123.75 -> y=0, x=5/3.
		mMin := NewModel("lp_nondyadic_min", Minimize)
		x := mMin.AddContVar("x", 0, math.Inf(1))
		y := mMin.AddContVar("y", 0, 10)
		mMin.AddConstraint("eq", Eq(NewExpr(T(x, 3), T(y, 7)), 5))
		mMin.AddConstraint("ub", Le(NewExpr(T(x, 0.1/0.3), T(y, 0.2/0.7)), 10))
		mMin.SetObjective(NewExpr(T(x, 1), T(y, 5)).AddConstant(123.75))

		for _, pre := range []bool{true, false} {
			for _, simdOn := range []bool{true, false} {
				res, err := mMin.Solve(ctx, WithCertified(true), WithPresolve(pre), WithSIMD(simdOn), WithRelGapTol(0), WithAbsGapTol(0))
				if err != nil {
					t.Fatalf("lp_nondyadic_min (pre=%v, simd=%v): %v", pre, simdOn, err)
				}
				if res.Status != StatusOptimal {
					t.Fatalf("status=%v, want Optimal", res.Status)
				}
				wantExact := big.NewRat(3*12375+500, 300)
				wantObj := exactRatToFloat(wantExact)
				wantBound := exactRatToOutwardBound(Minimize, wantExact)
				if res.Objective != wantObj || res.BestBound != wantBound {
					t.Fatalf("obj=%.17g bound=%.17g, want (%.17g, %.17g)", res.Objective, res.BestBound, wantObj, wantBound)
				}
			}
		}

		// Maximize with free variable (-Inf, +Inf) and negative ObjConstant.
		// max 2*u - v - 50.5 s.t. u + v = 1, u <= 5/3, u - 2*v <= 4 -> at u = 5/3, v = -2/3 -> 2*(5/3) - (-2/3) = 4 -> 4 - 50.5 = -46.5.
		mMax := NewModel("lp_free_max", Maximize)
		u := mMax.AddContVar("u", math.Inf(-1), 5.0/3.0)
		v := mMax.AddContVar("v", math.Inf(-1), math.Inf(1))
		mMax.AddConstraint("sum", Eq(NewExpr(T(u, 1), T(v, 1)), 1))
		mMax.AddConstraint("side", Le(NewExpr(T(u, 1), T(v, -2)), 10))
		mMax.SetObjective(NewExpr(T(u, 2), T(v, -1)).AddConstant(-50.5))

		resMax, err := mMax.Solve(ctx, WithCertified(true), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil {
			t.Fatalf("lp_free_max: %v", err)
		}
		if resMax.Status != StatusOptimal {
			t.Fatalf("lp_free_max status=%v, want Optimal", resMax.Status)
		}
		if math.Abs(resMax.Objective - -46.5) > 1e-12 {
			t.Fatalf("lp_free_max obj=%.17g, want -46.5", resMax.Objective)
		}
	})

	t.Run("UnconstrainedAndZeroVarLP", func(t *testing.T) {
		t.Parallel()
		// 0-constraint bounded LP with a zero-cost free variable.
		m0Con := NewModel("lp_0con", Minimize)
		x := m0Con.AddContVar("x", 2.5, 8.0)
		freeZero := m0Con.AddContVar("fz", math.Inf(-1), math.Inf(1))
		m0Con.SetObjective(NewExpr(T(x, 3), T(freeZero, 0)).AddConstant(4.0))
		res0, err := m0Con.Solve(ctx, WithCertified(true))
		if err != nil || res0.Status != StatusOptimal || res0.Objective != 11.5 {
			t.Fatalf("0-con LP: status=%v obj=%v err=%v", res0.Status, res0.Objective, err)
		}

		// 0-variable LP (feasible and infeasible).
		mEmptyFeas := NewModel("lp_0var_feas", Maximize)
		mEmptyFeas.AddConstraint("ok", Le(ConstExpr(0), 2.0))
		mEmptyFeas.SetObjective(ConstExpr(7.25))
		resEF, err := mEmptyFeas.Solve(ctx, WithCertified(true))
		if err != nil || resEF.Status != StatusOptimal || resEF.Objective != 7.25 {
			t.Fatalf("0-var feasible LP: status=%v obj=%v err=%v", resEF.Status, resEF.Objective, err)
		}

		mEmptyInf := NewModel("lp_0var_inf", Minimize)
		mEmptyInf.AddConstraint("bad", Ge(ConstExpr(0), 1.0))
		resEI, err := mEmptyInf.Solve(ctx, WithCertified(true))
		if err != nil || resEI.Status != StatusInfeasible {
			t.Fatalf("0-var infeasible LP: status=%v err=%v", resEI.Status, err)
		}
	})

	t.Run("InfeasibleAndUnboundedLP", func(t *testing.T) {
		t.Parallel()
		// Infeasible LP with free variable: u + v <= 1, u + v >= 2, u free, v free.
		mInf := NewModel("lp_infeas_free", Minimize)
		u := mInf.AddContVar("u", math.Inf(-1), math.Inf(1))
		v := mInf.AddContVar("v", math.Inf(-1), math.Inf(1))
		mInf.AddConstraint("c1", Le(NewExpr(T(u, 1), T(v, 1)), 1))
		mInf.AddConstraint("c2", Ge(NewExpr(T(u, 1), T(v, 1)), 2))
		mInf.SetObjective(NewExpr(T(u, 0.3), T(v, -0.7)))
		for _, pre := range []bool{true, false} {
			res, err := mInf.Solve(ctx, WithCertified(true), WithPresolve(pre))
			if err != nil || res.Status != StatusInfeasible {
				t.Fatalf("infeasible LP (pre=%v): status=%v err=%v", pre, res.Status, err)
			}
		}

		// Unbounded Maximize LP with constraints.
		mUnb := NewModel("lp_unbounded_max", Maximize)
		x := mUnb.AddContVar("x", 0, math.Inf(1))
		y := mUnb.AddContVar("y", 0, math.Inf(1))
		mUnb.AddConstraint("cone", Le(NewExpr(T(x, 1), T(y, -2)), 3))
		mUnb.SetObjective(NewExpr(T(x, 1), T(y, 1)).AddConstant(10))
		for _, pre := range []bool{true, false} {
			res, err := mUnb.Solve(ctx, WithCertified(true), WithPresolve(pre))
			if err != nil || res.Status != StatusUnbounded {
				t.Fatalf("unbounded LP (pre=%v): status=%v err=%v", pre, res.Status, err)
			}
		}

		// Pure LP resource-limit exits (early negative TimeLimit and MaxIter).
		mLim := NewModel("lp_limits", Minimize)
		lx := mLim.AddContVar("lx", 0, 10)
		ly := mLim.AddContVar("ly", 0, 10)
		mLim.AddConstraint("c1", Ge(NewExpr(T(lx, 1), T(ly, 2)), 4))
		mLim.AddConstraint("c2", Ge(NewExpr(T(lx, 3), T(ly, 1)), 5))
		mLim.SetObjective(NewExpr(T(lx, 1), T(ly, 1)))
		resLPTL, err := mLim.Solve(ctx, WithCertified(true), WithTimeLimit(-time.Millisecond))
		if err != nil || resLPTL.Status != StatusTimeLimit {
			t.Fatalf("pure LP negative TimeLimit: status=%v err=%v", resLPTL.Status, err)
		}
		resLPIL, err := mLim.Solve(ctx, WithCertified(true), WithPresolve(false), WithMaxIter(1))
		if err != nil || (resLPIL.Status != StatusIterationLimit && resLPIL.Status != StatusOptimal) {
			t.Fatalf("pure LP MaxIter=1: status=%v err=%v", resLPIL.Status, err)
		}
	})
}

func TestCertifiedMILPAndVerifyVIPRTrace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("MultiNodeOptimalCutsPresolveAndMixedNonDyadic", func(t *testing.T) {
		t.Parallel()
		// Mixed-integer problem with cuts, branching, and continuous non-dyadic completion:
		// max 5.5*x + 5*y + 3*z + 100.5 s.t. 6*x + 4*y <= 24, x + 2*y <= 6, -x + y <= 1, y <= 2, 3*z + x = 4.
		// Unique optimum is x=2, y=2, z=2/3 -> 11 + 10 + 2 + 100.5 = 123.5.
		m := NewModel("mip_mixed_max", Maximize)
		x := m.AddIntVar("x", 0, 10)
		y := m.AddIntVar("y", 0, 10)
		z := m.AddContVar("z", 0, 10)
		m.AddConstraint("c1", Le(NewExpr(T(x, 6), T(y, 4)), 24))
		m.AddConstraint("c2", Le(NewExpr(T(x, 1), T(y, 2)), 6))
		m.AddConstraint("c3", Le(NewExpr(T(x, -1), T(y, 1)), 1))
		m.AddConstraint("c4", Le(VarExpr(y), 2))
		m.AddConstraint("eq_cont", Eq(NewExpr(T(z, 3), T(x, 1)), 4))
		m.SetObjective(NewExpr(T(x, 5.5), T(y, 5), T(z, 3)).AddConstant(100.5))

		userTrace := NewVIPRTrace()
		res, err := m.Solve(
			ctx,
			WithCertified(true),
			WithPresolve(true),
			WithCuts(true),
			WithHeuristics(true),
			WithWorkers(4),
			WithVIPRTrace(userTrace),
			WithRelGapTol(0),
			WithAbsGapTol(0),
		)
		if err != nil {
			t.Fatalf("mip_mixed_max certified solve: %v", err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want Optimal", res.Status)
		}
		if res.Objective != 123.5 || res.Values[2] != exactRatToFloat(big.NewRat(2, 3)) {
			t.Fatalf("obj=%.17g z=%.17g, want 123.5 and 2/3", res.Objective, res.Values[2])
		}
		if !userTrace.HasCertificate() {
			t.Fatal("expected caller-supplied VIPRTrace to hold verified certificate")
		}
		p := m.Compile()
		if errV := VerifyVIPRTrace(p, userTrace, res, ApplyOptions(WithRelGapTol(0), WithAbsGapTol(0))); errV != nil {
			t.Fatalf("standalone VerifyVIPRTrace failed: %v", errV)
		}
	})

	t.Run("PresolveEnvelopeAndOrbitalSymmetry", func(t *testing.T) {
		t.Parallel()
		// Exercise all 9 verified presolve ops + B&B under default EnablePresolve=true and WithCertified(true).
		mAll := NewModel("cert_all_presolve_ops", Minimize)
		x0 := mAll.AddIntVar("x0", 0.25, 4.75) // roundBound
		x1 := mAll.AddIntVar("x1", 0, 5)       // gcdRowDiv + activityBound
		x2 := mAll.AddIntVar("x2", 2, 2)       // substFixedRow + substFixedObj
		x3 := mAll.AddIntVar("x3", 0, 10)      // singletonRowElim
		b0 := mAll.AddBinaryVar("b0")          // binStrengthen
		s0 := mAll.AddBinaryVar("s0")          // symBreakLe
		s1 := mAll.AddBinaryVar("s1")
		s2 := mAll.AddBinaryVar("s2")
		mAll.SetObjective(NewExpr(x0.T(1), x1.T(1), x2.T(3), x3.T(1), b0.T(2), s0.T(-1), s1.T(-1), s2.T(-1)).AddConstant(5))
		mAll.AddConstraint("gcd", Le(NewExpr(x0.T(2), x1.T(2)), 7))
		mAll.AddConstraint("sing", Le(NewExpr(x3.T(2)), 7))
		mAll.AddConstraint("use_fix", Le(NewExpr(x1.T(1), x2.T(1)), 5))
		mAll.AddConstraint("red", Le(NewExpr(x0.T(1), x1.T(1)), 100))
		mAll.AddConstraint("act", Ge(NewExpr(x0.T(1), x1.T(1)), 2))
		mAll.AddConstraint("binst", Le(NewExpr(x1.T(1), b0.T(5)), 6))
		mAll.AddConstraint("sym_cap", Le(NewExpr(s0.T(2), s1.T(2), s2.T(2)), 3))
		pAll := mAll.Compile()
		pAll.varLB[0] = 0.25
		pAll.varUB[0] = 4.75

		refRes, err := SolveCompiled(ctx, pAll, WithPresolve(false), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil || refRes.Status != StatusOptimal {
			t.Fatalf("reference solve: status=%v err=%v", refRes.Status, err)
		}
		trAll := NewVIPRTrace()
		certRes, err := SolveCompiled(ctx, pAll, WithCertified(true), WithVIPRTrace(trAll), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil {
			t.Fatalf("certified presolved solve: %v", err)
		}
		if certRes.Status != StatusOptimal || certRes.Objective != refRes.Objective {
			t.Fatalf("got (%v, %.17g), want (%v, %.17g)", certRes.Status, certRes.Objective, refRes.Status, refRes.Objective)
		}
		if trAll.TracedProblem() != pAll {
			t.Fatal("expected TracedProblem() == original CompiledProblem pAll")
		}
		if trAll.PresolveStepCount() == 0 {
			t.Fatal("expected non-empty presolve envelope steps")
		}
		if errV := VerifyVIPRTrace(pAll, trAll, certRes, ApplyOptions(WithRelGapTol(0), WithAbsGapTol(0))); errV != nil {
			t.Fatalf("VerifyVIPRTrace on presolve envelope failed: %v", errV)
		}

		// Combined presolve symBreakLe + live B&B orbitBranch on S_4 coloring + GCD-1 twin block.
		mOrb := buildSymmetricColoringMIP(4)
		t0 := mOrb.AddBinaryVar("t0")
		t1 := mOrb.AddBinaryVar("t1")
		t2 := mOrb.AddBinaryVar("t2")
		u := mOrb.AddBinaryVar("u")
		origTerms := append([]Term(nil), mOrb.objTerms...)
		origTerms = append(origTerms, t0.T(-5), t1.T(-5), t2.T(-5), u.T(-1))
		mOrb.SetObjective(NewExpr(origTerms...))
		mOrb.AddConstraint("twin_cap", Le(NewExpr(t0.T(2), t1.T(2), t2.T(2), u.T(1)), 3))
		pOrb := mOrb.Compile()

		trOrb := NewVIPRTrace()
		resOrb, err := SolveCompiled(ctx, pOrb,
			WithCertified(true), WithCuts(false), WithHeuristics(false),
			WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(trOrb),
		)
		if err != nil {
			t.Fatalf("certified presolve+orbitBranch solve: %v", err)
		}
		if resOrb.Status != StatusOptimal || resOrb.Objective != -2.0 {
			t.Fatalf("got (%v, %v), want (Optimal, -2)", resOrb.Status, resOrb.Objective)
		}
		if trOrb.TracedProblem() != pOrb || trOrb.PresolveStepCount() == 0 || trOrb.OrbitBranchesCertified() == 0 {
			t.Fatalf("expected TracedProblem==pOrb, presolve>0, orbitBranches>0; got match=%v pre=%d orb=%d",
				trOrb.TracedProblem() == pOrb, trOrb.PresolveStepCount(), trOrb.OrbitBranchesCertified())
		}
		if errV := VerifyVIPRTrace(pOrb, trOrb, resOrb, ApplyOptions(WithRelGapTol(0), WithAbsGapTol(0))); errV != nil {
			t.Fatalf("VerifyVIPRTrace on presolve+orbitBranch failed: %v", errV)
		}

		// Presolve-infeasible envelopes under WithCertified(true).
		mInfAct := NewModel("cert_pre_inf_act", Minimize)
		ia := mInfAct.AddIntVar("ia", 2, 5)
		ib := mInfAct.AddIntVar("ib", 2, 5)
		mInfAct.AddConstraint("bad", Le(NewExpr(ia.T(1), ib.T(1)), 3))
		mInfAct.SetObjective(VarExpr(ia))
		pInfAct := mInfAct.Compile()
		trInfAct := NewVIPRTrace()
		resInfAct, err := SolveCompiled(ctx, pInfAct, WithCertified(true), WithVIPRTrace(trInfAct))
		if err != nil || resInfAct.Status != StatusInfeasible {
			t.Fatalf("presolve infeasible certified: status=%v err=%v", resInfAct.Status, err)
		}
		if trInfAct.TracedProblem() != pInfAct || !trInfAct.PresolveInfeasible() {
			t.Fatalf("expected TracedProblem==pInfAct and PresolveInfeasible==true")
		}
		if errV := VerifyVIPRTrace(pInfAct, trInfAct, resInfAct, DefaultOptions()); errV != nil {
			t.Fatalf("VerifyVIPRTrace on presolve-infeasible failed: %v", errV)
		}
	})

	t.Run("EarlyRelGapStopWithLargeObjConstant", func(t *testing.T) {
		t.Parallel()
		// Multi-knapsack MIP with large ObjConstant and loose RelGapTol exercising "bound" VIPR mode.
		m := NewModel("mip_bound_mode", Minimize)
		vars := make([]Var, 6)
		var objTerms []Term
		var c1Terms []Term
		weights := []float64{5, 7, 9, 11, 13, 15}
		costs := []float64{6, 8, 10, 12, 14, 17}
		for i := range 6 {
			vars[i] = m.AddBinaryVar("")
			objTerms = append(objTerms, T(vars[i], costs[i]))
			c1Terms = append(c1Terms, T(vars[i], weights[i]))
		}
		m.AddConstraint("cover", Ge(NewExpr(c1Terms...), 23))
		m.SetObjective(NewExpr(objTerms...).AddConstant(1e6))

		res, err := m.Solve(ctx, WithCertified(true), WithCuts(false), WithRelGapTol(0.5))
		if err != nil {
			t.Fatalf("mip_bound_mode: %v", err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want Optimal", res.Status)
		}
	})

	t.Run("MILPInfeasibleAndUnboundedMeyer", func(t *testing.T) {
		t.Parallel()
		// Integrality-infeasible parity equality row: 2*x + 4*y = 3.
		mGCD := NewModel("mip_gcd_infeas", Minimize)
		x := mGCD.AddIntVar("x", 0, 10)
		y := mGCD.AddIntVar("y", 0, 10)
		mGCD.AddConstraint("parity", Eq(NewExpr(T(x, 2), T(y, 4)), 3))
		mGCD.SetObjective(NewExpr(T(x, 1), T(y, 1)))
		resGCD, err := mGCD.Solve(ctx, WithCertified(true))
		if err != nil || resGCD.Status != StatusInfeasible {
			t.Fatalf("mip_gcd_infeas: status=%v err=%v", resGCD.Status, err)
		}

		// Multi-node B&B infeasible MIP (LP feasible at 0.5, no integer point): 2*x + 2*y >= 1, 2*x + 2*y <= 1 on binaries.
		mTreeInf := NewModel("mip_tree_infeas", Maximize)
		b0 := mTreeInf.AddBinaryVar("b0")
		b1 := mTreeInf.AddBinaryVar("b1")
		mTreeInf.AddConstraint("lo", Ge(NewExpr(T(b0, 2), T(b1, 2)), 1))
		mTreeInf.AddConstraint("hi", Le(NewExpr(T(b0, 2), T(b1, 2)), 1))
		mTreeInf.SetObjective(NewExpr(T(b0, 3), T(b1, 5)))
		resTI, err := mTreeInf.Solve(ctx, WithCertified(true), WithCuts(false))
		if err != nil || resTI.Status != StatusInfeasible {
			t.Fatalf("mip_tree_infeas: status=%v err=%v", resTI.Status, err)
		}

		// Unbounded MILP with fractional LP ray requiring integer LCM scaling (2*x - 3*y = 1, x,y >= 0 int, min -x - y).
		mUnb := NewModel("mip_meyer_unb", Minimize)
		ux := mUnb.AddIntVar("ux", 0, math.Inf(1))
		uy := mUnb.AddIntVar("uy", 0, math.Inf(1))
		mUnb.AddConstraint("dio", Eq(NewExpr(T(ux, 2), T(uy, -3)), 1))
		mUnb.SetObjective(NewExpr(T(ux, -1), T(uy, -1)))
		resUnb, err := mUnb.Solve(ctx, WithCertified(true))
		if err != nil || resUnb.Status != StatusUnbounded {
			t.Fatalf("mip_meyer_unb: status=%v err=%v", resUnb.Status, err)
		}
	})

	t.Run("LimitStatusesWithAndWithoutIncumbent", func(t *testing.T) {
		t.Parallel()
		m := NewModel("mip_nodelimit", Minimize)
		v := make([]Var, 5)
		var obj, con []Term
		for i := range 5 {
			v[i] = m.AddBinaryVar("")
			obj = append(obj, T(v[i], 1))
			con = append(con, T(v[i], 2))
		}
		m.AddConstraint("odd_ge", Ge(NewExpr(con...), 3))
		m.SetObjective(NewExpr(obj...))

		resNL, err := m.Solve(ctx, WithCertified(true), WithCuts(false), WithMaxNodes(1), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil {
			t.Fatalf("mip_nodelimit: %v", err)
		}
		if resNL.Status != StatusNodeLimit && resNL.Status != StatusOptimal {
			t.Fatalf("unexpected status=%v", resNL.Status)
		}

		resTL, err := m.Solve(ctx, WithCertified(true), WithTimeLimit(-time.Millisecond))
		if err != nil || resTL.Status != StatusTimeLimit {
			t.Fatalf("negative TimeLimit: status=%v err=%v", resTL.Status, err)
		}
	})

	t.Run("BackendCBCIntegration", func(t *testing.T) {
		t.Parallel()
		mLP := NewModel("cbc_cert_lp", Minimize)
		x := mLP.AddContVar("x", 0, 10)
		mLP.AddConstraint("ge", Ge(NewExpr(T(x, 3)), 5))
		mLP.SetObjective(VarExpr(x))

		mMIP := NewModel("cbc_cert_mip", Minimize)
		ix := mMIP.AddIntVar("ix", 0, 10)
		mMIP.AddConstraint("ge", Ge(NewExpr(T(ix, 2)), 5))
		mMIP.SetObjective(VarExpr(ix))

		mLPMax := NewModel("cbc_cert_lp_max", Maximize)
		u := mLPMax.AddContVar("u", math.Inf(-1), 5.0/3.0)
		v := mLPMax.AddContVar("v", math.Inf(-1), math.Inf(1))
		mLPMax.AddConstraint("sum", Eq(NewExpr(T(u, 1), T(v, 1)), 1))
		mLPMax.AddConstraint("side", Le(NewExpr(T(u, 1), T(v, -2)), 10))
		mLPMax.SetObjective(NewExpr(T(u, 2), T(v, -1)).AddConstant(12.5))

		mLPInf := NewModel("cbc_cert_lp_inf", Minimize)
		ix0 := mLPInf.AddContVar("x0", 0, 1)
		mLPInf.AddConstraint("ge", Ge(VarExpr(ix0), 2))
		mLPInf.SetObjective(VarExpr(ix0))

		mLPUnb := NewModel("cbc_cert_lp_unb", Maximize)
		ux0 := mLPUnb.AddContVar("ux0", 0, math.Inf(1))
		mLPUnb.AddConstraint("ge", Ge(VarExpr(ux0), 1))
		mLPUnb.SetObjective(VarExpr(ux0))

		if CBCAvailable() {
			resLP, errLP := mLP.Solve(ctx, WithCBC(), WithCertified(true))
			if errLP != nil || resLP.Status != StatusOptimal || resLP.Backend != BackendCBC {
				t.Fatalf("CBC LP certified: status=%v backend=%v err=%v", resLP.Status, resLP.Backend, errLP)
			}
			resLPMax, errLPMax := mLPMax.Solve(ctx, WithCBC(), WithCertified(true))
			if errLPMax != nil || resLPMax.Status != StatusOptimal || resLPMax.Backend != BackendCBC || math.Abs(resLPMax.Objective-16.5) > 1e-12 {
				t.Fatalf("CBC LP max certified: status=%v obj=%v err=%v", resLPMax.Status, resLPMax.Objective, errLPMax)
			}
			resLPInf, errLPInf := mLPInf.Solve(ctx, WithCBC(), WithCertified(true))
			if errLPInf != nil || resLPInf.Status != StatusInfeasible {
				t.Fatalf("CBC LP infeas certified: status=%v err=%v", resLPInf.Status, errLPInf)
			}
			resLPUnb, errLPUnb := mLPUnb.Solve(ctx, WithCBC(), WithCertified(true))
			if errLPUnb != nil || resLPUnb.Status != StatusUnbounded {
				t.Fatalf("CBC LP unb certified: status=%v err=%v", resLPUnb.Status, errLPUnb)
			}
			resMIP, errMIP := mMIP.Solve(ctx, WithCBC(), WithCertified(true))
			if errMIP != nil || resMIP.Status != StatusOptimal || resMIP.Backend != BackendCBC || resMIP.Objective != 3 {
				t.Fatalf("CBC MIP certified: status=%v backend=%v obj=%v err=%v", resMIP.Status, resMIP.Backend, resMIP.Objective, errMIP)
			}
		} else {
			resLP, errLP := mLP.Solve(ctx, WithCBC(), WithCertified(true))
			if !errors.Is(errLP, ErrCertificationFailed) || resLP.Status != StatusNumericalError || resLP.Backend != BackendCBC {
				t.Fatalf("stub CBC LP certified: status=%v backend=%v err=%v", resLP.Status, resLP.Backend, errLP)
			}
			resMIP, errMIP := mMIP.Solve(ctx, WithCBC(), WithCertified(true))
			if !errors.Is(errMIP, ErrCertificationFailed) || resMIP.Status != StatusNumericalError || resMIP.Backend != BackendCBC {
				t.Fatalf("stub CBC MIP certified: status=%v backend=%v err=%v", resMIP.Status, resMIP.Backend, errMIP)
			}
		}
	})
}

func TestMiniVIPRFixturesAndOrbitBranchVerifier(t *testing.T) {
	t.Parallel()

	sampleRaw, err := os.ReadFile(filepath.Join("testdata", "formal", "vipr_sample.json"))
	if err != nil {
		t.Fatalf("read vipr_sample.json: %v", err)
	}
	if _, err := verifyMiniVIPRBytes(sampleRaw, nil); err != nil {
		t.Fatalf("vipr_sample.json unexpectedly rejected: %v", err)
	}

	// Verify targeted rule-level negative mutations on vipr_sample.json all fail closed.
	invalidCerts := []struct {
		name string
		raw  string
	}{
		{
			name: "wrong_lincomb_sign",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [{"coeffs": [1], "sense": "<=", "rhs": 3}],
				"mode": "infeasible",
				"steps": [{"rule": "lincomb", "sense": "<=", "refs": [{"id": 0, "mult": -1}]}],
				"rootStep": 0
			}`,
		},
		{
			name: "round_down_continuous_var",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [false], "lb": [0], "ub": [5],
				"baseRows": [{"coeffs": [1], "sense": "<=", "rhs": "3/2"}],
				"mode": "infeasible",
				"steps": [{"rule": "roundDown", "ref": 0}],
				"rootStep": 0
			}`,
		},
		{
			name: "assume_continuous_var",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [false], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [{"rule": "assume", "col": 0, "sense": "<=", "bound": -1}],
				"rootStep": 0
			}`,
		},
		{
			name: "undischarged_root_assumption",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [5],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [
					{"rule": "assume", "col": 0, "sense": "<=", "bound": 1},
					{"rule": "boundConflict", "col": 0, "ubRef": 0}
				],
				"rootStep": 1
			}`,
		},
		{
			name: "branch_split_gap_hole",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [
					{"rule": "assume", "col": 0, "sense": "<=", "bound": 1},
					{"rule": "assume", "col": 0, "sense": ">=", "bound": 3},
					{"rule": "branch", "col": 0, "splitVal": 1, "leftAsm": 0, "rightAsm": 1, "left": 0, "right": 1}
				],
				"rootStep": 2
			}`,
		},
		{
			name: "bound_conflict_non_empty",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [2],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [{"rule": "boundConflict", "col": 0}],
				"rootStep": 0
			}`,
		},
		{
			name: "safe_dual_unbounded_col",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [null], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
		},
	}
	for _, tc := range invalidCerts {
		if _, err := verifyMiniVIPRBytes([]byte(tc.raw), nil); !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("%s: expected ErrCertificationFailed, got %v", tc.name, err)
		}
	}

	// Verify orbitBranch rule acceptance on a symmetric 2-binary MIP and rejection under asymmetric tampering.
	orbitCert := `{
		"n": 2,
		"objConst": 0,
		"obj": [1, 1],
		"isInt": [true, true],
		"lb": [0, 0],
		"ub": [1, 1],
		"baseRows": [{"coeffs": [1, 1], "sense": ">=", "rhs": 1}],
		"mode": "optimal",
		"incumbent": [1, 0],
		"steps": [
			{"rule": "assume", "col": 0, "sense": "<=", "bound": 0},
			{"rule": "assume", "col": 1, "sense": "<=", "bound": 0},
			{"rule": "farkas", "refs": [{"id": 0, "mult": -1}, {"id": 1, "mult": 1}, {"id": 2, "mult": 1}]},
			{"rule": "assume", "col": 0, "sense": ">=", "bound": 1},
			{"rule": "safeDual", "refs": [], "boundAsms": [4]},
			{
				"rule": "orbitBranch",
				"col": 0,
				"leftAsm": 1,
				"rightAsm": 4,
				"left": 3,
				"right": 5,
				"orbitAsms": [2],
				"witnesses": [{"companion": 1, "colPerm": [1, 0], "rowPerm": [0]}]
			}
		],
		"rootStep": 5
	}`
	if _, err := verifyMiniVIPRBytes([]byte(orbitCert), nil); err != nil {
		t.Fatalf("valid orbitBranch certificate rejected: %v", err)
	}

	// Break symmetry in objective ([1, 2]) so orbitBranch automorphism witness must fail closed.
	badOrbitCert := `{
		"n": 2,
		"objConst": 0,
		"obj": [1, 2],
		"isInt": [true, true],
		"lb": [0, 0],
		"ub": [1, 1],
		"baseRows": [{"coeffs": [1, 1], "sense": ">=", "rhs": 1}],
		"mode": "optimal",
		"incumbent": [1, 0],
		"steps": [
			{"rule": "assume", "col": 0, "sense": "<=", "bound": 0},
			{"rule": "assume", "col": 1, "sense": "<=", "bound": 0},
			{"rule": "farkas", "refs": [{"id": 0, "mult": -1}, {"id": 1, "mult": 1}, {"id": 2, "mult": 1}]},
			{"rule": "assume", "col": 0, "sense": ">=", "bound": 1},
			{"rule": "safeDual", "refs": [], "boundAsms": [4]},
			{
				"rule": "orbitBranch",
				"col": 0,
				"leftAsm": 1,
				"rightAsm": 4,
				"left": 3,
				"right": 5,
				"orbitAsms": [2],
				"witnesses": [{"companion": 1, "colPerm": [1, 0], "rowPerm": [0]}]
			}
		],
		"rootStep": 5
	}`
	if _, err := verifyMiniVIPRBytes([]byte(badOrbitCert), nil); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("asymmetric orbitBranch certificate got %v, want ErrCertificationFailed", err)
	}
}

func TestCertifiedNegativeControlsFailClosed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	assertFailClosed := func(t *testing.T, name string, res Result, err error) {
		t.Helper()
		if !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("%s: got err=%v, want ErrCertificationFailed", name, err)
		}
		if res.Status != StatusNumericalError {
			t.Fatalf("%s: got status=%v, want StatusNumericalError", name, res.Status)
		}
		if res.Values != nil || res.Duals != nil || res.ReducedCosts != nil {
			t.Fatalf("%s: unsafe vectors not cleared: values=%v duals=%v rc=%v", name, res.Values, res.Duals, res.ReducedCosts)
		}
		if res.Objective != 0 || res.BestBound != 0 {
			t.Fatalf("%s: unsafe objective/bound not cleared: obj=%v bound=%v", name, res.Objective, res.BestBound)
		}
	}

	// Base LP model: min x + y s.t. x + 2*y >= 4, x, y >= 0 -> optimal (4,0) or (0,2), obj = 2.
	buildLP := func() *Model {
		m := NewModel("neg_lp", Minimize)
		x := m.AddContVar("x", 0, 10)
		y := m.AddContVar("y", 0, 10)
		m.AddConstraint("c", Ge(NewExpr(T(x, 1), T(y, 2)), 4))
		m.SetObjective(NewExpr(T(x, 1), T(y, 1)))
		return m
	}

	// 1. Corrupted LP primal (by 1 ULP).
	{
		m := buildLP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Values[1] = math.Nextafter(res.Values[1], 100.0)
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "lp_corrupted_primal_1ulp", res, err)
	}

	// 2. Tampered LP dual (by 1 ULP).
	{
		m := buildLP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Duals[0] = math.Nextafter(res.Duals[0], 100.0)
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "lp_tampered_dual_1ulp", res, err)
	}

	// 3. Corrupted LP BestBound overclaiming > optimal on Minimize.
	{
		m := buildLP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.BestBound = res.Objective + 1e-3
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "lp_overclaimed_best_bound", res, err)
	}

	// 4. False LP StatusInfeasible on a feasible LP.
	{
		m := buildLP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, wit *lpCertWitness, res *Result) {
			res.Status = StatusInfeasible
			res.Values = nil
			res.Duals = nil
			res.ReducedCosts = nil
			wit.farkasW = []*big.Rat{big.NewRat(-1, 1)}
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "lp_false_infeasible", res, err)
	}

	// 5. False LP StatusUnbounded / tampered recession ray.
	{
		mUnb := NewModel("neg_lp_unb", Minimize)
		x := mUnb.AddContVar("x", 0, math.Inf(1))
		mUnb.AddConstraint("c", Ge(VarExpr(x), 1))
		mUnb.SetObjective(NewExpr(T(x, -1)))
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, wit *lpCertWitness, _ *Result) {
			wit.unbounded.Ray[0] = big.NewRat(-1, 1) // Violates x >= 0 recession cone.
		})
		res, err := mUnb.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "lp_tampered_unbounded_ray", res, err)
	}

	// Base MILP model: min 3*x + 2*y + 1e9 s.t. 2*x + 2*y >= 3, x, y in {0..3} int -> optimal (0,2), linear=4.
	buildMIP := func() *Model {
		m := NewModel("neg_mip", Minimize)
		x := m.AddIntVar("x", 0, 3)
		y := m.AddIntVar("y", 0, 3)
		m.AddConstraint("c", Ge(NewExpr(T(x, 2), T(y, 2)), 3))
		m.SetObjective(NewExpr(T(x, 3), T(y, 2)).AddConstant(1e9))
		return m
	}

	// 6. Corrupted MILP primal (integer column shifted or fractional).
	{
		m := buildMIP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Values[1] += 1e-4
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "mip_fractional_int_incumbent", res, err)
	}

	// 7. Missing / cleared VIPRTrace on MILP Optimal.
	{
		m := buildMIP()
		fCtx := withGoblinCertFault(ctx, func(p *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
			trace.reset(p)
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "mip_missing_vipr_trace", res, err)
	}

	// 8. Tampered VIPRTrace step (corrupting a multiplier) and problem-header mismatch.
	{
		m := buildMIP()
		fCtxStep := withGoblinCertFault(ctx, func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
			if len(trace.cert.Steps) > 0 {
				trace.cert.Steps[0].Rule = "unknownRule"
			}
		})
		res1, err1 := m.Solve(fCtxStep, WithCertified(true))
		assertFailClosed(t, "mip_tampered_vipr_step", res1, err1)

		fCtxHdr := withGoblinCertFault(ctx, func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
			trace.cert.BaseRows[0].RHS = viprInt64Payload(1) // Weakens row RHS from 3 to 1!
		})
		res2, err2 := m.Solve(fCtxHdr, WithCertified(true))
		assertFailClosed(t, "mip_tampered_vipr_problem_header", res2, err2)
	}

	// 9. Suboptimal MILP bound masked by huge ObjConstant = 1e9 (linear gap 2/4 = 0.5 > RelGapTol = 1e-3).
	{
		m := buildMIP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, res *Result) {
			// Force "bound" mode claiming targetLB = 1e9 + 2 (linear bound 2 vs incumbent linear 4).
			trace.cert.Mode = "bound"
			lbWeak := viprInt64Payload(1_000_000_002)
			trace.cert.TargetLB = &lbWeak
			// Replace proof with trivial lower bound 3*x + 2*y >= 0 so targetLB=1e9+0; use 1e9+0 to pass VIPR step check!
			trace.cert.Steps = []viprStepJSON{{Rule: "safeDual", Refs: viprOptRefs(nil)}}
			trace.cert.RootStep = 0
			lbZeroLin := viprInt64Payload(1_000_000_000)
			trace.cert.TargetLB = &lbZeroLin
			res.BestBound = 1e9
		})
		res, err := m.Solve(fCtx, WithCertified(true), WithRelGapTol(1e-3), WithAbsGapTol(1e-3))
		assertFailClosed(t, "mip_objconst_masked_relgap", res, err)
	}

	// 10. Tampered non-integral Meyer ray on MILP Unbounded.
	{
		mUnb := NewModel("neg_mip_unb", Minimize)
		ux := mUnb.AddIntVar("ux", 0, math.Inf(1))
		uy := mUnb.AddIntVar("uy", 0, math.Inf(1))
		mUnb.AddConstraint("dio", Eq(NewExpr(T(ux, 2), T(uy, -3)), 1))
		mUnb.SetObjective(NewExpr(T(ux, -1), T(uy, -1)))
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, wit *lpCertWitness, _ *Result) {
			// Scale ray by 1/2 so integer column has non-integer ray entry 3/2.
			wit.unbounded.Ray[0] = big.NewRat(3, 2)
			wit.unbounded.Ray[1] = big.NewRat(1, 1)
			wit.unbounded.ObjSlope = big.NewRat(-5, 2)
		})
		res, err := mUnb.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "mip_fractional_meyer_ray", res, err)
	}

	// 11. Limit status (StatusNodeLimit) with injected infeasible incumbent.
	{
		m := buildMIP()
		fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Status = StatusNodeLimit
			res.Values = []float64{0, 0} // Violates 2*x + 2*y >= 3.
			res.Objective = 1e9
		})
		res, err := m.Solve(fCtx, WithCertified(true))
		assertFailClosed(t, "mip_nodelimit_infeasible_incumbent", res, err)
	}

	// 12. Standalone VerifyVIPRTrace negative check on mutated JSON step.
	{
		m := buildMIP()
		p := m.Compile()
		tr := NewVIPRTrace()
		res, err := SolveCompiled(ctx, p, WithPresolve(false), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("baseline VIPR solve failed: status=%v err=%v", res.Status, err)
		}
		if errV := VerifyVIPRTrace(p, tr, res, DefaultOptions()); errV != nil {
			t.Fatalf("baseline VerifyVIPRTrace failed: %v", errV)
		}

		// Tamper with claimed constraint in step 0 via JSON round-trip.
		raw, errM := tr.MarshalVIPRJSON()
		if errM != nil {
			t.Fatalf("MarshalVIPRJSON: %v", errM)
		}
		var doc map[string]jsontext.Value
		if errU := json.Unmarshal(raw, &doc); errU != nil {
			t.Fatalf("Unmarshal: %v", errU)
		}
		doc["rootStep"] = jsontext.Value("999999")
		badRaw, _ := json.Marshal(doc)
		if _, errBad := verifyMiniVIPRProblem(p, badRaw); !errors.Is(errBad, ErrCertificationFailed) {
			t.Fatalf("expected out-of-range rootStep rejection, got %v", errBad)
		}
	}

	// 13. Corrupted LP ReducedCosts (NaN, 1e9, and 1-ULP shift across Minimize & Maximize) and spurious non-optimal Duals/ReducedCosts.
	{
		for _, sense := range []Sense{Minimize, Maximize} {
			mRC := NewModel("neg_lp_rc", sense)
			x := mRC.AddContVar("x", 0, 10)
			y := mRC.AddContVar("y", 0, 10)
			mRC.AddConstraint("c", Ge(NewExpr(T(x, 1), T(y, 2)), 4))
			if sense == Maximize {
				mRC.SetObjective(NewExpr(T(x, -1), T(y, -3)).AddConstant(5))
			} else {
				mRC.SetObjective(NewExpr(T(x, 1), T(y, 3)).AddConstant(5))
			}

			for _, corrupt := range []struct {
				name string
				fn   func(*Result)
			}{
				{"nan", func(r *Result) { r.ReducedCosts[0] = math.NaN() }},
				{"huge", func(r *Result) { r.ReducedCosts[0] = 1e9 }},
				{"1ulp", func(r *Result) { r.ReducedCosts[1] = math.Nextafter(r.ReducedCosts[1], 100.0) }},
			} {
				fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
					corrupt.fn(res)
				})
				res, err := mRC.Solve(fCtx, WithCertified(true))
				assertFailClosed(t, "lp_rc_"+corrupt.name, res, err)
			}
		}

		// Spurious Duals/ReducedCosts on non-optimal LP exit.
		mInf := NewModel("neg_lp_inf_duals", Minimize)
		ix := mInf.AddContVar("ix", 0, 1)
		mInf.AddConstraint("ge", Ge(VarExpr(ix), 2))
		mInf.SetObjective(VarExpr(ix))
		fCtxInf := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Duals = []float64{1.0}
		})
		resInf, errInf := mInf.Solve(fCtxInf, WithCertified(true))
		assertFailClosed(t, "lp_infeasible_spurious_duals", resInf, errInf)
	}

	// 14. Limit-status BestBound corruptions on MILP (Minimize & Maximize) and pure LP.
	{
		build5Bin := func(sense Sense) *Model {
			m := NewModel("mip_5bin_limit", sense)
			v := make([]Var, 5)
			var obj, con []Term
			w := 1.0
			if sense == Maximize {
				w = -1.0
			}
			for i := range 5 {
				v[i] = m.AddBinaryVar("")
				obj = append(obj, T(v[i], w))
				con = append(con, T(v[i], 2))
			}
			m.AddConstraint("odd_ge", Ge(NewExpr(con...), 3))
			m.SetObjective(NewExpr(obj...))
			return m
		}

		mMin := build5Bin(Minimize)
		pMin := mMin.Compile()
		trMin := NewVIPRTrace()
		resMinNL, err := SolveCompiled(ctx, pMin, WithCuts(false), WithPresolve(false), WithMaxNodes(1), WithVIPRTrace(trMin))
		if err != nil || resMinNL.Status != StatusNodeLimit {
			t.Fatalf("expected StatusNodeLimit on 5-bin MIP, got status=%v err=%v", resMinNL.Status, err)
		}
		if errV := VerifyVIPRTrace(pMin, trMin, resMinNL, DefaultOptions()); errV != nil {
			t.Fatalf("valid NodeLimit VerifyVIPRTrace failed: %v", errV)
		}

		// Corruptions against VerifyVIPRTrace on NodeLimit: BestBound=1e9, NaN, overclaiming 1.6 > 1.5, and nil trace with non-zero BestBound.
		for _, badBnd := range []float64{1e9, math.NaN(), math.Inf(1), 1.6} {
			badRes := resMinNL
			badRes.BestBound = badBnd
			if errV := VerifyVIPRTrace(pMin, trMin, badRes, DefaultOptions()); !errors.Is(errV, ErrCertificationFailed) {
				t.Fatalf("expected ErrCertificationFailed for NodeLimit BestBound=%v, got %v", badBnd, errV)
			}
		}
		if errV := VerifyVIPRTrace(pMin, nil, resMinNL, DefaultOptions()); !errors.Is(errV, ErrCertificationFailed) {
			t.Fatalf("expected ErrCertificationFailed for nil trace with BestBound=%v, got %v", resMinNL.BestBound, errV)
		}

		// Runtime fault injection on Minimize & Maximize NodeLimit BestBound.
		for _, tc := range []struct {
			name  string
			sense Sense
			bound float64
		}{
			{"mip_min_nl_huge_bound", Minimize, 1e9},
			{"mip_min_nl_nan_bound", Minimize, math.NaN()},
			{"mip_min_nl_overclaim_bound", Minimize, 1.5001},
			{"mip_max_nl_overclaim_bound", Maximize, -1.5001},
			{"mip_max_nl_neginf_bound", Maximize, math.Inf(-1)},
		} {
			m := build5Bin(tc.sense)
			fCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
				res.BestBound = tc.bound
			})
			res, errS := m.Solve(fCtx, WithCertified(true), WithPresolve(false), WithCuts(false), WithMaxNodes(1))
			assertFailClosed(t, tc.name, res, errS)
		}

		// Pure LP TimeLimit with injected uncertified BestBound.
		mLP := buildLP()
		for _, badLPBnd := range []float64{1e9, math.NaN(), math.Inf(1)} {
			fCtxLP := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
				res.BestBound = badLPBnd
			})
			resLP, errLP := mLP.Solve(fCtxLP, WithCertified(true), WithTimeLimit(-time.Millisecond))
			assertFailClosed(t, "lp_timelimit_bad_bestbound", resLP, errLP)
		}
	}

	// 15. Tampered "orig" and "presolve" envelope negative controls under WithCertified(true) and VerifyVIPRTrace.
	{
		buildEnvMIP := func() *CompiledProblem {
			m := NewModel("neg_presolve_env", Minimize)
			x0 := m.AddIntVar("x0", 0.25, 4.75)
			x1 := m.AddIntVar("x1", 0, 5)
			x2 := m.AddIntVar("x2", 2, 2)
			s0 := m.AddBinaryVar("s0")
			s1 := m.AddBinaryVar("s1")
			m.SetObjective(NewExpr(x0.T(1), x1.T(1), x2.T(3), s0.T(-1), s1.T(-1)))
			m.AddConstraint("gcd", Le(NewExpr(x0.T(2), x1.T(2)), 7))
			m.AddConstraint("use_fix", Le(NewExpr(x1.T(1), x2.T(1)), 5))
			m.AddConstraint("sym", Le(NewExpr(s0.T(1), s1.T(1)), 1))
			p := m.Compile()
			p.varLB[0] = 0.25
			p.varUB[0] = 4.75
			return p
		}

		// Baseline check: presolve envelope is present and verifies cleanly.
		pEnv := buildEnvMIP()
		trBase := NewVIPRTrace()
		resBase, errBase := SolveCompiled(ctx, pEnv, WithCertified(true), WithVIPRTrace(trBase), WithRelGapTol(0), WithAbsGapTol(0))
		if errBase != nil || resBase.Status != StatusOptimal || trBase.cert.Orig == nil || len(trBase.cert.Presolve) == 0 {
			t.Fatalf("baseline presolve-envelope certified solve failed: status=%v err=%v orig=%v pre=%d",
				resBase.Status, errBase, trBase.cert.Orig != nil, len(trBase.cert.Presolve))
		}

		// Runtime fail-closed checks on tampered "orig" and "presolve" fields.
		faults := []struct {
			name string
			fn   goblinCertFaultFunc
		}{
			{
				name: "tampered_orig_row_rhs",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.cert.Orig.BaseRows[0].RHS = viprInt64Payload(100)
				},
			},
			{
				name: "tampered_orig_obj_coeff",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.cert.Orig.Obj[0] = viprInt64Payload(99)
				},
			},
			{
				name: "missing_orig_with_presolve_steps",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.cert.Orig = nil
				},
			},
			{
				name: "tampered_presolve_roundBound",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					for i := range trace.cert.Presolve {
						if trace.cert.Presolve[i].Op == "roundBound" && !*trace.cert.Presolve[i].IsUpper {
							p2 := viprInt64Payload(2)
							trace.cert.Presolve[i].NewBound = &p2
							return
						}
					}
				},
			},
			{
				name: "tampered_presolve_gcdRowDiv",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					for i := range trace.cert.Presolve {
						if trace.cert.Presolve[i].Op == "gcdRowDiv" {
							trace.cert.Presolve[i].Divisor = viprInt64Ptr(3)
							return
						}
					}
				},
			},
			{
				name: "tampered_presolve_substFixedRow",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					for i := range trace.cert.Presolve {
						if trace.cert.Presolve[i].Op == "substFixedRow" {
							trace.cert.Presolve[i].Col = viprIntPtr(0)
							return
						}
					}
				},
			},
			{
				name: "tampered_presolve_symBreakLe_perm",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					for i := range trace.cert.Presolve {
						if trace.cert.Presolve[i].Op == "symBreakLe" {
							for j := range trace.cert.Presolve[i].ColPerm {
								trace.cert.Presolve[i].ColPerm[j] = j
							}
							return
						}
					}
				},
			},
			{
				name: "tampered_presolve_injected_redundantRow",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.cert.Presolve = append([]viprPresolveStepJSON{{
						Op:  "redundantRow",
						Row: viprIntPtr(0),
					}}, trace.cert.Presolve...)
				},
			},
			{
				name: "tampered_reduced_objConst_mismatch",
				fn: func(_ *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.cert.ObjConst = viprInt64Payload(999)
				},
			},
		}
		for _, tc := range faults {
			fCtx := withGoblinCertFault(ctx, tc.fn)
			resBad, errBad := SolveCompiled(fCtx, buildEnvMIP(), WithCertified(true), WithRelGapTol(0), WithAbsGapTol(0))
			assertFailClosed(t, tc.name, resBad, errBad)
		}

		// Standalone VerifyVIPRTrace rejection when verified certificate belongs to a different CompiledProblem.
		pOther := NewModel("other_mip", Minimize)
		ox := pOther.AddIntVar("ox", 0, 5)
		pOther.SetObjective(VarExpr(ox))
		if errV := VerifyVIPRTrace(pOther.Compile(), trBase, resBase, DefaultOptions()); !errors.Is(errV, ErrCertificationFailed) {
			t.Fatalf("expected ErrCertificationFailed for mismatched CompiledProblem against orig envelope, got %v", errV)
		}
	}
}

func TestGoblinSolveZeroOverheadOptionAllocation(t *testing.T) {
	allocsDefault := testing.AllocsPerRun(20, func() {
		opt := ApplyOptions()
		if opt.Certified {
			t.Fatal("expected Certified=false")
		}
	})
	if allocsDefault != 0 {
		t.Fatalf("ApplyOptions() allocs = %v, want 0", allocsDefault)
	}

	optFalse := WithCertified(false)
	optTrue := WithCertified(true)
	allocsCtor := testing.AllocsPerRun(20, func() {
		if WithCertified(false) == nil || optFalse == nil || optTrue == nil {
			t.Fatal("unexpected nil Option")
		}
	})
	if allocsCtor != 0 {
		t.Fatalf("WithCertified() constructor allocs = %v, want 0", allocsCtor)
	}
}

func TestWithOptionsCertifiedStructLiteralFailsClosedWithoutHook(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	m := NewModel("hook_guard", Minimize)
	x := m.AddBinaryVar("x")
	y := m.AddBinaryVar("y")
	m.AddConstraint("c1", Ge(NewExpr(T(x, 1), T(y, 1)), 1))
	m.SetObjective(NewExpr(T(x, 2), T(y, 3)))

	// Passing Certified: true in a raw Options struct without WithCertified(true) must fail closed.
	rawOpt := DefaultOptions()
	rawOpt.Certified = true
	resBad, errBad := m.Solve(ctx, WithOptions(rawOpt))
	if !errors.Is(errBad, ErrCertificationFailed) || resBad.Status != StatusNumericalError || len(resBad.Values) != 0 {
		t.Fatalf("WithOptions(Certified: true) without WithCertified(true) = (%+v, %v), want StatusNumericalError + ErrCertificationFailed", resBad, errBad)
	}

	// Chaining WithCertified(true) after WithOptions installs solveCertified and succeeds.
	baseOpt := DefaultOptions()
	baseOpt.RelGapTol = 0
	baseOpt.AbsGapTol = 0
	resOK, errOK := m.Solve(ctx, WithOptions(baseOpt), WithCertified(true))
	if errOK != nil || resOK.Status != StatusOptimal || resOK.Objective != 2 {
		t.Fatalf("WithOptions + WithCertified(true) = (%+v, %v), want Optimal obj=2", resOK, errOK)
	}

	// WithVIPRTrace(nil) must leave cfg.viprTrace == nil (never a typed-nil *VIPRTrace interface).
	cfgNil := ApplyOptions(WithVIPRTrace(NewVIPRTrace()), WithVIPRTrace(nil))
	if cfgNil.viprTrace != nil {
		t.Fatalf("WithVIPRTrace(nil) left non-nil interface %#v", cfgNil.viprTrace)
	}
}

func TestVIPRJSONByteIdenticalToEncodingJSONV2(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// 1. Presolve + OrbitBranch optimal certificate.
	mOpt := NewModel("json_eq_opt", Minimize)
	f := mOpt.AddIntVar("f", 0.6, 1.4)
	b0 := mOpt.AddBinaryVar("b0")
	b1 := mOpt.AddBinaryVar("b1")
	b2 := mOpt.AddBinaryVar("b2")
	mOpt.AddConstraint("pigeon", Ge(NewExpr(T(f, 1), T(b0, 1), T(b1, 1), T(b2, 1)), 3))
	mOpt.SetObjective(NewExpr(T(f, 5), T(b0, 2), T(b1, 2), T(b2, 2)))

	trOpt := NewVIPRTrace()
	resOpt, errOpt := mOpt.Solve(ctx, WithVIPRTrace(trOpt), WithRelGapTol(0), WithAbsGapTol(0))
	if errOpt != nil || resOpt.Status != StatusOptimal || !trOpt.HasCertificate() {
		t.Fatalf("solve opt failed: status=%v err=%v hasCert=%v", resOpt.Status, errOpt, trOpt.HasCertificate())
	}
	gotOpt, errM := trOpt.MarshalVIPRJSON()
	if errM != nil {
		t.Fatalf("MarshalVIPRJSON: %v", errM)
	}
	// Compare parsed semantic structure via encoding/json/v2 round-trip on raw map/structs.
	type mirrorStep struct {
		Rule        string                 `json:"rule"`
		Row         *int                   `json:"row,omitzero"`
		Col         *int                   `json:"col,omitzero"`
		Sense       string                 `json:"sense,omitzero"`
		Bound       jsontext.Value         `json:"bound,omitzero"`
		Refs        *[]viprLinRefJSON      `json:"refs,omitzero"`
		Bounds      []viprBoundRefJSON     `json:"bounds,omitzero"`
		BoundAsms   []int                  `json:"boundAsms,omitzero"`
		Ref         *int                   `json:"ref,omitzero"`
		SplitVal    jsontext.Value         `json:"splitVal,omitzero"`
		LeftAsm     *int                   `json:"leftAsm,omitzero"`
		RightAsm    *int                   `json:"rightAsm,omitzero"`
		Left        *int                   `json:"left,omitzero"`
		Right       *int                   `json:"right,omitzero"`
		OrbitAsms   []int                  `json:"orbitAsms,omitzero"`
		Witnesses   []viprOrbitWitnessJSON `json:"witnesses,omitzero"`
		ExtraAsms   []int                  `json:"extraAsms,omitzero"`
		LBRef       *int                   `json:"lbRef,omitzero"`
		UBRef       *int                   `json:"ubRef,omitzero"`
		Assumptions []int                  `json:"assumptions,omitzero"`
	}
	type mirrorCert struct {
		Orig      *viprInstanceJSON      `json:"orig,omitzero"`
		Presolve  []viprPresolveStepJSON `json:"presolve,omitzero"`
		N         int                    `json:"n"`
		ObjConst  viprRatPayload         `json:"objConst"`
		Obj       []viprRatPayload       `json:"obj"`
		IsInt     []bool                 `json:"isInt"`
		LB        []*viprRatPayload      `json:"lb"`
		UB        []*viprRatPayload      `json:"ub"`
		BaseRows  []viprLinConJSON       `json:"baseRows"`
		Mode      string                 `json:"mode"`
		TargetLB  *viprRatPayload        `json:"targetLB,omitzero"`
		Incumbent []viprRatPayload       `json:"incumbent,omitzero"`
		Steps     []mirrorStep           `json:"steps"`
		RootStep  int                    `json:"rootStep"`
	}
	mc := mirrorCert{
		Orig:      trOpt.cert.Orig,
		Presolve:  trOpt.cert.Presolve,
		N:         trOpt.cert.N,
		ObjConst:  trOpt.cert.ObjConst,
		Obj:       trOpt.cert.Obj,
		IsInt:     trOpt.cert.IsInt,
		LB:        trOpt.cert.LB,
		UB:        trOpt.cert.UB,
		BaseRows:  trOpt.cert.BaseRows,
		Mode:      string(trOpt.cert.Mode),
		TargetLB:  trOpt.cert.TargetLB,
		Incumbent: trOpt.cert.Incumbent,
		Steps:     make([]mirrorStep, len(trOpt.cert.Steps)),
		RootStep:  trOpt.cert.RootStep,
	}
	for i, s := range trOpt.cert.Steps {
		mc.Steps[i] = mirrorStep{
			Rule:        string(s.Rule),
			Row:         s.Row,
			Col:         s.Col,
			Sense:       s.Sense,
			Bound:       jsontext.Value(s.Bound),
			Refs:        s.Refs,
			Bounds:      s.Bounds,
			BoundAsms:   s.BoundAsms,
			Ref:         s.Ref,
			SplitVal:    jsontext.Value(s.SplitVal),
			LeftAsm:     s.LeftAsm,
			RightAsm:    s.RightAsm,
			Left:        s.Left,
			Right:       s.Right,
			OrbitAsms:   s.OrbitAsms,
			Witnesses:   s.Witnesses,
			ExtraAsms:   s.ExtraAsms,
			LBRef:       s.LBRef,
			UBRef:       s.UBRef,
			Assumptions: s.Assumptions,
		}
	}
	wantOpt, errWant := json.Marshal(mc)
	if errWant != nil {
		t.Fatalf("json.Marshal mirrorCert: %v", errWant)
	}
	if !bytes.Equal(gotOpt, wantOpt) {
		t.Fatalf("MarshalVIPRJSON byte mismatch vs encoding/json/v2:\ngot:  %s\nwant: %s", gotOpt, wantOpt)
	}

	// 2. Malformed/adversarial JSON rejection in unmarshalCertVIPRDoc (duplicate key, trailing bytes, leading zero, control char, invalid UTF-8).
	badPayloads := [][]byte{
		[]byte(`{"n":1,"n":1}`),
		[]byte(`{"n":1} trailing`),
		[]byte(`{"n":01}`),
		[]byte("{\"mode\":\"opt\x01\"}"),
		{'{', '"', 'm', '"', ':', 0xff, '}'},
	}
	for idx, bad := range badPayloads {
		if _, ok := unmarshalCertVIPRDoc(bad); ok {
			t.Fatalf("badPayload[%d] (%q) unexpectedly parsed ok", idx, bad)
		}
	}
}

func TestGoblinBinaryDeadCodeElimination(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "main.go")
	binPath := filepath.Join(tmp, "goblin_default_bin")
	src := `package main

import (
	"context"
	"fmt"
	gb "github.com/paulourio/goblin"
)

func main() {
	m := gb.NewModel("t", gb.Minimize)
	x := m.AddBinaryVar("x")
	y := m.AddBinaryVar("y")
	m.AddConstraint("c1", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 1))
	m.SetObjective(gb.NewExpr(gb.T(x, 2), gb.T(y, 3)))
	res, err := m.Solve(context.Background(), gb.WithOptions(gb.DefaultOptions()))
	fmt.Println(res.Objective, err)
}
`
	if err := os.WriteFile(mainPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, mainPath)
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	runOut, err := exec.CommandContext(ctx, binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run built binary: %v\n%s", err, runOut)
	}
	if got := strings.TrimSpace(string(runOut)); got != "2 <nil>" {
		t.Fatalf("binary output = %q, want %q", got, "2 <nil>")
	}

	nmOut, err := exec.CommandContext(ctx, "go", "tool", "nm", "-size", binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm failed: %v\n%s", err, nmOut)
	}

	forbidden := regexp.MustCompile(`(solveCertified|VerifyVIPRTrace|verifyMiniVIPR|VIPRTrace|babVIPRRecorder|viprEmitter|viprPresolveState|PresolveForVIPR|activeOrbitWithWitnesses|encoding/json)`)
	var leaked []string
	for line := range strings.SplitSeq(string(nmOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sym := fields[len(fields)-1]
		if forbidden.MatchString(sym) {
			leaked = append(leaked, sym)
		}
	}
	if len(leaked) > 0 {
		t.Fatalf("unreferenced certified/VIPR/json symbols linked into default binary (%d): %v", len(leaked), leaked[:min(len(leaked), 10)])
	}
}

func TestCertifiedExactZeroGapDefaultAndScaledKnapsack(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Unset gap tolerances default to 0 under WithCertified(true), independent of option order.
	cfgCert := ApplyOptions(WithCertified(true))
	if cfgCert.RelGapTol != 0 || cfgCert.AbsGapTol != 0 {
		t.Fatalf("ApplyOptions(WithCertified(true)) gap tols = (%g, %g), want (0, 0)", cfgCert.RelGapTol, cfgCert.AbsGapTol)
	}
	cfgRelFirst := ApplyOptions(WithRelGapTol(0.25), WithCertified(true))
	if cfgRelFirst.RelGapTol != 0.25 || cfgRelFirst.AbsGapTol != 0 {
		t.Fatalf("WithRelGapTol(0.25)+WithCertified(true) gap tols = (%g, %g), want (0.25, 0)", cfgRelFirst.RelGapTol, cfgRelFirst.AbsGapTol)
	}
	cfgAbsAfter := ApplyOptions(WithCertified(true), WithAbsGapTol(1e-4))
	if cfgAbsAfter.RelGapTol != 0 || cfgAbsAfter.AbsGapTol != 1e-4 {
		t.Fatalf("WithCertified(true)+WithAbsGapTol(1e-4) gap tols = (%g, %g), want (0, 1e-4)", cfgAbsAfter.RelGapTol, cfgAbsAfter.AbsGapTol)
	}
	cfgDisabled := ApplyOptions(WithCertified(true), WithCertified(false))
	if cfgDisabled.RelGapTol != defaultRelGapTol || cfgDisabled.AbsGapTol != defaultAbsGapTol {
		t.Fatalf("WithCertified(false) gap tols = (%g, %g), want (%g, %g)",
			cfgDisabled.RelGapTol, cfgDisabled.AbsGapTol, defaultRelGapTol, defaultAbsGapTol)
	}

	// Tiny-scaled 6-variable integer knapsack: default WithCertified(true) must prove exact mode="optimal".
	for _, scale := range []float64{1, 1e-6, 1e-7, 1e-8} {
		m := NewModel("tiny_knap", Maximize)
		var objTerms, rowTerms []Term
		for i := range 6 {
			v := m.AddIntVar("", 0, 10)
			objTerms = append(objTerms, v.T(scale*float64(i+3)))
			rowTerms = append(rowTerms, v.T(float64(2*i+3)))
		}
		m.SetObjective(NewExpr(objTerms...))
		m.AddConstraint("cap", Le(NewExpr(rowTerms...), 37))

		tr := NewVIPRTrace()
		res, err := m.Solve(ctx, WithCertified(true), WithVIPRTrace(tr))
		if err != nil {
			t.Fatalf("scale=%g certified solve failed: %v", scale, err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("scale=%g status=%v, want StatusOptimal", scale, res.Status)
		}
		if tr.Mode() != "optimal" {
			t.Fatalf("scale=%g trace mode=%q, want \"optimal\"", scale, tr.Mode())
		}
		// Optimal integer vector has objective 35 * scale (x0=1, x5=2 -> 3 + 2*8 = 19? No: x0=9, x1=2 -> 27+8=35, weight 27+10=37).
		wantRat := new(big.Rat).Mul(big.NewRat(35, 1), new(big.Rat).SetFloat64(scale))
		// Exact objective is sum_i x_i * float64(scale*(i+3)): x0=9, x1=2 gives 9*(3*scale) + 2*(4*scale).
		c0, _ := exactRatFromFloat(scale * 3)
		c1, _ := exactRatFromFloat(scale * 4)
		wantExact := new(big.Rat).Add(new(big.Rat).Mul(big.NewRat(9, 1), c0), new(big.Rat).Mul(big.NewRat(2, 1), c1))
		_ = wantRat
		wantF := exactRatToFloat(wantExact)
		wantBound := exactRatToOutwardBound(Maximize, wantExact)
		if res.Objective != wantF || res.BestBound != wantBound {
			t.Fatalf("scale=%g got (obj=%.17g, bound=%.17g), want (%.17g, %.17g)", scale, res.Objective, res.BestBound, wantF, wantBound)
		}
	}

	// Pure LP sub-1e-6 dual gap must fail closed under default WithCertified(true), while explicit WithAbsGapTol honors caller tolerance.
	mLP := NewModel("lp_zero_gap_gate", Minimize)
	x := mLP.AddContVar("x", 0, 10)
	mLP.AddConstraint("ge", Ge(VarExpr(x), 2))
	mLP.SetObjective(VarExpr(x))
	subTolDual := 1.0 - 1e-8 // Dual bound 2*(1 - 1e-8) leaves 2e-8 gap < 1e-6.
	injectSubTolDual := func(_ *CompiledProblem, _ *VIPRTrace, wit *lpCertWitness, res *Result) {
		r, _ := exactRatFromFloat(subTolDual)
		wit.dualMin[0] = r
		res.Duals[0] = subTolDual
		res.ReducedCosts[0] = 1.0 - subTolDual
		res.BestBound = 2.0 * subTolDual
		res.RootLPBound = res.BestBound
		res.RootCutBound = res.BestBound
	}
	resFail, errFail := mLP.Solve(withGoblinCertFault(ctx, injectSubTolDual), WithCertified(true))
	if !errors.Is(errFail, ErrCertificationFailed) || resFail.Status != StatusNumericalError {
		t.Fatalf("expected pure LP sub-1e-6 gap to fail closed under default WithCertified(true), got (%v, %v)", resFail.Status, errFail)
	}
	resExplicit, errExplicit := mLP.Solve(withGoblinCertFault(ctx, injectSubTolDual), WithCertified(true), WithAbsGapTol(1e-6))
	if errExplicit != nil || resExplicit.Status != StatusOptimal {
		t.Fatalf("expected explicit WithAbsGapTol(1e-6) to succeed, got (%v, %v)", resExplicit.Status, errExplicit)
	}
}

func TestStrictIntegerBoundRoundingAndNearIntRHS(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Fractional integer UB 2.9999999999995 must floor strictly to 2 in both default and certified modes.
	for _, cert := range []bool{false, true} {
		m := NewModel("strict_int_ub", Maximize)
		x := m.AddIntVar("x", 0, 2.9999999999995)
		y := m.AddIntVar("y", 0, 10)
		m.SetObjective(NewExpr(x.T(1), y.T(0)))
		m.AddConstraint("c", Le(NewExpr(x.T(1), y.T(1)), 20))
		tr := NewVIPRTrace()
		var opts []Option
		if cert {
			opts = append(opts, WithCertified(true), WithVIPRTrace(tr))
		}
		res, err := m.Solve(ctx, opts...)
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("cert=%v strict_int_ub: status=%v err=%v", cert, res.Status, err)
		}
		if res.Values[0] != 2 || res.Objective != 2 {
			t.Fatalf("cert=%v strict_int_ub: got x=%.17g obj=%.17g, want 2", cert, res.Values[0], res.Objective)
		}
		if cert && tr.Mode() != "optimal" {
			t.Fatalf("strict_int_ub trace mode=%q, want \"optimal\"", tr.Mode())
		}
	}

	// Symmetric strict rounding on fractional integer LB, Binary UB, and SetVarBounds.
	mLB := NewModel("strict_int_lb", Minimize)
	xl := mLB.AddIntVar("xl", 0.0000000000005, 5)
	xb := mLB.AddVar("xb", 0, 0.9999999999995, Binary)
	xs := mLB.AddIntVar("xs", 0, 5)
	mLB.SetVarBounds(xs, 0, 2.9999999999995)
	mLB.SetObjective(NewExpr(xl.T(1), xb.T(-10), xs.T(-1)))
	resLB, errLB := mLB.Solve(ctx, WithCertified(true))
	if errLB != nil || resLB.Status != StatusOptimal {
		t.Fatalf("strict_int_lb: status=%v err=%v", resLB.Status, errLB)
	}
	if resLB.Values[0] != 1 || resLB.Values[1] != 0 || resLB.Values[2] != 2 || resLB.Objective != -1 {
		t.Fatalf("strict_int_lb: got vals=%v obj=%v, want [1 0 2] obj=-1", resLB.Values, resLB.Objective)
	}

	// Near-integer RHS on exact-integer rows: max x+y s.t. 2x + 2y <= 3.99999995 -> obj = 1.
	for _, rhs := range []float64{3.99999995, 3.9999999} {
		for _, tc := range []struct {
			name string
			opts []Option
		}{
			{"default", nil},
			{"no_presolve", []Option{WithPresolve(false)}},
			{"no_presolve_no_cuts", []Option{WithPresolve(false), WithCuts(false)}},
			{"parallel_no_presolve", []Option{WithPresolve(false), WithCuts(false), WithWorkers(4)}},
			{"certified_default", []Option{WithCertified(true)}},
			{"certified_no_presolve", []Option{WithCertified(true), WithPresolve(false)}},
			{"certified_no_presolve_no_cuts", []Option{WithCertified(true), WithPresolve(false), WithCuts(false)}},
		} {
			m := NewModel("gcd_near_int", Maximize)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddIntVar("y", 0, 10)
			m.SetObjective(NewExpr(x.T(1), y.T(1)))
			m.AddConstraint("r", Le(NewExpr(x.T(2), y.T(2)), rhs))
			res, err := m.Solve(ctx, tc.opts...)
			if err != nil || res.Status != StatusOptimal || res.Objective != 1 {
				t.Fatalf("rhs=%.10g %s: got (status=%v, obj=%v, vals=%v, err=%v), want (Optimal, 1)",
					rhs, tc.name, res.Status, res.Objective, res.Values, err)
			}
		}
	}
}

func TestVerifyVIPRTraceStrictPolishedVsUnpolished(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	m := NewModel("verify_strict_vs_approx", Maximize)
	x := m.AddIntVar("x", 0, 10)
	y := m.AddIntVar("y", 0, 10)
	z := m.AddContVar("z", 0, 10)
	m.AddConstraint("c1", Le(NewExpr(T(x, 6), T(y, 4)), 24))
	m.AddConstraint("c2", Le(NewExpr(T(x, 1), T(y, 2)), 6))
	m.AddConstraint("eq_cont", Eq(NewExpr(T(z, 3), T(x, 1)), 4))
	m.SetObjective(NewExpr(T(x, 5.5), T(y, 5), T(z, 3)))
	p := m.Compile()

	tr := NewVIPRTrace()
	res, err := SolveCompiled(ctx, p, WithCertified(true), WithVIPRTrace(tr))
	if err != nil || res.Status != StatusOptimal {
		t.Fatalf("certified solve failed: status=%v err=%v", res.Status, err)
	}
	if errV := VerifyVIPRTrace(p, tr, res, ApplyOptions(WithCertified(true))); errV != nil {
		t.Fatalf("strict VerifyVIPRTrace failed on polished certified result: %v", errV)
	}

	// 1e-8 perturbations on continuous Values, Objective, and BestBound fail strict WithCertified(true) check
	// while remaining within unpolished DefaultOptions() float tolerance.
	for _, tc := range []struct {
		name   string
		mutate func(*Result)
	}{
		{"cont_val_1e-8", func(r *Result) { r.Values[2] += 1e-8 }},
		{"obj_1e-8", func(r *Result) { r.Objective -= 1e-8 }},
		{"best_bound_1e-8", func(r *Result) {
			r.BestBound -= 1e-8
			r.Objective -= 1e-8
		}},
	} {
		mut := res
		mut.Values = append([]float64(nil), res.Values...)
		tc.mutate(&mut)
		if errStrict := VerifyVIPRTrace(p, tr, mut, ApplyOptions(WithCertified(true))); !errors.Is(errStrict, ErrCertificationFailed) {
			t.Fatalf("%s: strict VerifyVIPRTrace got %v, want ErrCertificationFailed", tc.name, errStrict)
		}
		if errUnpol := VerifyVIPRTrace(p, tr, mut, DefaultOptions()); errUnpol != nil {
			t.Fatalf("%s: unpolished VerifyVIPRTrace unexpectedly rejected 1e-8 float drift: %v", tc.name, errUnpol)
		}
	}
}

func TestCertifiedExactModelOrigVerification(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	mInt := NewModel("cert_exact_int_tiny", Maximize)
	xi := mInt.AddIntVar("x", 0, 10)
	mInt.SetObjective(NewExpr(xi.T(1)))
	mInt.AddConstraint("c", Le(NewExpr(xi.T(1)).AddConstant(1e-17), 3))
	pInt := mInt.Compile()
	if verifyExactPrimalRat(pInt, []*big.Rat{big.NewRat(3, 1)}, true) {
		t.Fatal("x=3 unexpectedly passed verifyExactPrimalRat on x + 1e-17 <= 3")
	}
	if !verifyExactPrimalRat(pInt, []*big.Rat{big.NewRat(2, 1)}, true) {
		t.Fatal("x=2 unexpectedly failed verifyExactPrimalRat on x + 1e-17 <= 3")
	}
	if res, err := SolveCompiled(ctx, pInt, WithCertified(true)); err != nil || res.Status != StatusOptimal || res.Objective != 2 {
		t.Fatalf("SolveCompiled int tiny got status=%v obj=%v err=%v, want Optimal obj=2", res.Status, res.Objective, err)
	}

	mCont := NewModel("cert_exact_cont_dup", Maximize)
	xc := mCont.AddContVar("x", 0, 10)
	mCont.SetObjective(NewExpr(xc.T(1)))
	mCont.AddConstraint("c", Le(NewExpr(xc.T(0.1), xc.T(0.2)), 0.3))
	pCont := mCont.Compile()
	sumCoeff := new(big.Rat).Add(new(big.Rat).SetFloat64(0.1), new(big.Rat).SetFloat64(0.2))
	exactMaxX := new(big.Rat).Quo(new(big.Rat).SetFloat64(0.3), sumCoeff)
	if !verifyExactPrimalRat(pCont, []*big.Rat{exactMaxX}, false) {
		t.Fatal("exact rat(0.3)/(rat(0.1)+rat(0.2)) failed verifyExactPrimalRat")
	}
	roundedAbove := new(big.Rat).SetFloat64(0.3 / (0.1 + 0.2))
	// Check: rounded float64(0.3) / float64(0.1+0.2) is 0.9999999999999998 (below) while x=1 violates 0.1+0.2<=0.3:
	_ = roundedAbove
	if verifyExactPrimalRat(pCont, []*big.Rat{big.NewRat(1, 1)}, false) {
		t.Fatal("x=1 unexpectedly passed verifyExactPrimalRat on 0.1x + 0.2x <= 0.3")
	}
}

func TestCertifiedLimitExitRejectsZeroBestBound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	mLP := NewModel("limit_zero_bb_lp", Minimize)
	x0 := mLP.AddContVar("x0", 0, 10)
	x1 := mLP.AddContVar("x1", 0, 10)
	x2 := mLP.AddContVar("x2", 0, 10)
	x3 := mLP.AddContVar("x3", 0, 10)
	mLP.SetObjective(NewExpr(x0.T(-1.4), x1.T(-0.8), x2.T(-0.6), x3.T(-0.4)))
	mLP.AddConstraint("c0", Le(NewExpr(x0.T(1), x1.T(1)), 2))
	mLP.AddConstraint("c1", Le(NewExpr(x1.T(1), x2.T(1)), 2))
	mLP.AddConstraint("c2", Le(NewExpr(x2.T(1), x3.T(1)), 2))

	faultCtx := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
		res.BestBound = 0
	})
	res, err := mLP.Solve(faultCtx, WithCertified(true), WithMaxIter(1))
	if !errors.Is(err, ErrCertificationFailed) || res.Status != StatusNumericalError {
		t.Fatalf("faulted LP limit exit got status=%v err=%v, want StatusNumericalError + ErrCertificationFailed", res.Status, err)
	}

	mMIP := NewModel("limit_zero_bb_mip", Minimize)
	xm := mMIP.AddIntVar("x", 0, 10)
	ym := mMIP.AddContVar("y", 0, 10)
	mMIP.SetObjective(NewExpr(xm.T(-1.4), ym.T(0.5)))
	mMIP.AddConstraint("c", Le(NewExpr(xm.T(1), ym.T(1)), 2))
	resM, errM := mMIP.Solve(faultCtx, WithCertified(true), WithTimeLimit(-1))
	if !errors.Is(errM, ErrCertificationFailed) || resM.Status != StatusNumericalError {
		t.Fatalf("faulted MIP limit exit got status=%v err=%v, want StatusNumericalError + ErrCertificationFailed", resM.Status, errM)
	}
}

func TestCertifiedBestBoundOutwardRoundingAndULPSlack(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	twoThirds := big.NewRat(2, 3)
	fl := exactRatToFloatFloor(twoThirds)
	cl := exactRatToFloatCeil(twoThirds)
	if new(big.Rat).SetFloat64(fl).Cmp(twoThirds) > 0 {
		t.Fatalf("floor(2/3) = %.17g exceeded 2/3", fl)
	}
	if new(big.Rat).SetFloat64(cl).Cmp(twoThirds) < 0 {
		t.Fatalf("ceil(2/3) = %.17g below 2/3", cl)
	}
	huge := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 1025))
	if got := exactRatToFloatFloor(huge); got != math.MaxFloat64 {
		t.Fatalf("floor(2^1025) = %v, want math.MaxFloat64", got)
	}
	if got := exactRatToFloatCeil(huge); !math.IsInf(got, 1) {
		t.Fatalf("ceil(2^1025) = %v, want +Inf", got)
	}

	mMin := NewModel("outward_min_2_3", Minimize)
	xMin := mMin.AddContVar("x", 0, 10)
	mMin.SetObjective(NewExpr(xMin.T(1)))
	mMin.AddConstraint("c", Ge(NewExpr(xMin.T(3)), 2))
	resMin, errMin := mMin.Solve(ctx, WithCertified(true))
	if errMin != nil || resMin.Status != StatusOptimal || new(big.Rat).SetFloat64(resMin.BestBound).Cmp(twoThirds) > 0 {
		t.Fatalf("min 2/3 got status=%v bb=%.17g err=%v", resMin.Status, resMin.BestBound, errMin)
	}

	mMax := NewModel("outward_max_2_3", Maximize)
	xMax := mMax.AddContVar("x", 0, 10)
	mMax.SetObjective(NewExpr(xMax.T(1)))
	mMax.AddConstraint("c", Le(NewExpr(xMax.T(3)), 2))
	resMax, errMax := mMax.Solve(ctx, WithCertified(true))
	if errMax != nil || resMax.Status != StatusOptimal || new(big.Rat).SetFloat64(resMax.BestBound).Cmp(twoThirds) < 0 {
		t.Fatalf("max 2/3 got status=%v bb=%.17g err=%v", resMax.Status, resMax.BestBound, errMax)
	}

	// Verify ulpSlack scaling on large constant offset (1e16 + x) with RelGapTol=1e-6.
	mConst := NewModel("large_const_ulp_slack", Minimize)
	xb := mConst.AddBinaryVar("x")
	mConst.SetObjective(NewExpr(xb.T(1)).AddConstant(1e16))
	mConst.AddConstraint("c", Ge(NewExpr(xb.T(1)), 0))
	resC, errC := mConst.Solve(ctx, WithCertified(true), WithRelGapTol(1e-6))
	if errC != nil || resC.Status != StatusOptimal || resC.Objective != 1e16 {
		t.Fatalf("large const MIP got status=%v obj=%.17g err=%v", resC.Status, resC.Objective, errC)
	}
}

func TestCertifiedHardeningAndParserParity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("CRIT04PreAllocDimensionAndRatLengthCaps", func(t *testing.T) {
		t.Parallel()
		hugeNJSON := []byte(`{"n":1000000000000,"obj":{"terms":[]},"isInt":[],"lb":[],"ub":[],"baseRows":[],"mode":"optimal"}`)
		if _, err := verifyMiniVIPRBytes(hugeNJSON, nil); !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("huge n sparse obj got err=%v, want ErrCertificationFailed", err)
		}
		if _, _, ok := decodeVIPRVec(maxVIPRVerifyVars+1, viprRawJSON(`{"terms":[]}`)); ok {
			t.Fatal("decodeVIPRVec accepted n > maxVIPRVerifyVars")
		}
		longDigits := strings.Repeat("9", maxVIPRRatStrLen+10)
		if _, ok := parseVIPRRatValue(viprRawJSON(`"` + longDigits + `/1"`)); ok {
			t.Fatal("parseVIPRRatValue accepted string exceeding maxVIPRRatStrLen")
		}
		hugeBits := strings.Repeat("9", 5000)
		if _, ok := parseVIPRRatValue(viprRawJSON(`"` + hugeBits + `/1"`)); ok {
			t.Fatal("parseVIPRRatValue accepted numerator exceeding maxVIPRRatBits")
		}
	})

	t.Run("MAJ02PureLPVIPRPresolveAndRootBoundsAndHardening", func(t *testing.T) {
		t.Parallel()
		// Pure LP with presolve absorbing a constraint into variable bounds must emit a valid VIPR trace on the original problem.
		mPre := NewModel("lp_vipr_presolve_orig", Minimize)
		x := mPre.AddContVar("x", 0, 10)
		mPre.SetObjective(NewExpr(x.T(1)))
		mPre.AddConstraint("c1", Ge(NewExpr(x.T(1)), 2))
		pPre := mPre.Compile()
		trPre := NewVIPRTrace()
		resPre, errPre := SolveCompiled(ctx, pPre, WithCertified(true), WithVIPRTrace(trPre))
		if errPre != nil || resPre.Status != StatusOptimal || !trPre.HasCertificate() {
			t.Fatalf("presolved LP certified+VIPR failed: status=%v err=%v hasCert=%v", resPre.Status, errPre, trPre.HasCertificate())
		}
		if errV := VerifyVIPRTrace(pPre, trPre, resPre, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace failed on presolved pure LP: %v", errV)
		}

		// Infeasible pure LP with VIPR trace must emit a valid infeasible certificate on the original problem.
		mInf := NewModel("lp_vipr_infeas", Minimize)
		xi := mInf.AddContVar("x", 0, 10)
		mInf.SetObjective(NewExpr(xi.T(1)))
		mInf.AddConstraint("lo", Ge(NewExpr(xi.T(1)), 6))
		mInf.AddConstraint("hi", Le(NewExpr(xi.T(1)), 4))
		pInf := mInf.Compile()
		trInf := NewVIPRTrace()
		resInf, errInf := SolveCompiled(ctx, pInf, WithCertified(true), WithVIPRTrace(trInf))
		if errInf != nil || resInf.Status != StatusInfeasible || !trInf.HasCertificate() {
			t.Fatalf("infeasible LP certified+VIPR failed: status=%v err=%v hasCert=%v", resInf.Status, errInf, trInf.HasCertificate())
		}
		if resInf.RootLPBound != 0 || resInf.RootCutBound != 0 || resInf.Objective != 0 {
			t.Fatalf("infeasible LP bounds not zeroed: obj=%v rootLP=%v rootCut=%v", resInf.Objective, resInf.RootLPBound, resInf.RootCutBound)
		}
		if errV := VerifyVIPRTrace(pInf, trInf, resInf, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace failed on infeasible pure LP: %v", errV)
		}

		// RootLPBound and RootCutBound must match BestBound and stay outward of exact 4/5.
		mOut := NewModel("lp_root_bound_outward", Minimize)
		xo := mOut.AddContVar("x", 0, 10)
		yo := mOut.AddContVar("y", 0, 0)
		mOut.SetObjective(NewExpr(xo.T(1)))
		mOut.AddConstraint("c", Ge(NewExpr(xo.T(5), yo.T(5)), 4))
		pOut := mOut.Compile()
		trOut := NewVIPRTrace()
		resOut, errOut := SolveCompiled(ctx, pOut, WithCertified(true), WithVIPRTrace(trOut))
		if errOut != nil || resOut.Status != StatusOptimal {
			t.Fatalf("outward LP failed: status=%v err=%v", resOut.Status, errOut)
		}
		rTrue := big.NewRat(4, 5)
		if resOut.RootLPBound != resOut.BestBound || resOut.RootCutBound != resOut.BestBound {
			t.Fatalf("expected RootLPBound==RootCutBound==BestBound, got (%g, %g, %g)",
				resOut.RootLPBound, resOut.RootCutBound, resOut.BestBound)
		}
		if new(big.Rat).SetFloat64(resOut.RootLPBound).Cmp(rTrue) > 0 {
			t.Fatalf("RootLPBound %.17g exceeded exact 4/5", resOut.RootLPBound)
		}

		// VerifyVIPRTrace must reject NaN/Inf Duals, ReducedCosts, RootLPBound, length mismatches, and nil trace on optimal LP.
		strictOpt := ApplyOptions(WithCertified(true))
		for _, tc := range []struct {
			name   string
			mutate func(*Result)
		}{
			{"nan_duals", func(r *Result) { r.Duals = []float64{math.NaN()} }},
			{"bad_len_duals", func(r *Result) { r.Duals = []float64{0, 0} }},
			{"inf_rc", func(r *Result) { r.ReducedCosts = []float64{math.Inf(1), 0} }},
			{"bad_len_rc", func(r *Result) { r.ReducedCosts = []float64{0} }},
			{"nan_root_lp", func(r *Result) { r.RootLPBound = math.NaN() }},
			{"overshoot_root_lp", func(r *Result) { r.RootLPBound = math.Nextafter(r.BestBound, math.Inf(1)) }},
			{"overshoot_root_cut", func(r *Result) { r.RootCutBound = math.Nextafter(r.BestBound, math.Inf(1)) }},
		} {
			mut := resOut
			mut.Duals = append([]float64(nil), resOut.Duals...)
			mut.ReducedCosts = append([]float64(nil), resOut.ReducedCosts...)
			tc.mutate(&mut)
			if errV := VerifyVIPRTrace(pOut, trOut, mut, strictOpt); !errors.Is(errV, ErrCertificationFailed) {
				t.Fatalf("%s: VerifyVIPRTrace got %v, want ErrCertificationFailed", tc.name, errV)
			}
		}
		if errNil := VerifyVIPRTrace(pOut, nil, resOut, strictOpt); !errors.Is(errNil, ErrCertificationFailed) {
			t.Fatalf("nil trace on StatusOptimal got %v, want ErrCertificationFailed", errNil)
		}

		// Fault-injected missing ReducedCosts on optimal LP must fail verifyCertifiedLP.
		faultNoRC := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.ReducedCosts = nil
		})
		if _, errNoRC := SolveCompiled(faultNoRC, pOut, WithCertified(true)); !errors.Is(errNoRC, ErrCertificationFailed) {
			t.Fatalf("missing ReducedCosts got %v, want ErrCertificationFailed", errNoRC)
		}
		// Fault-injected non-zero Objective on infeasible LP must fail verifyCertifiedLP.
		faultInfObj := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Objective = 1.0
		})
		if _, errInfObj := SolveCompiled(faultInfObj, pInf, WithCertified(true)); !errors.Is(errInfObj, ErrCertificationFailed) {
			t.Fatalf("non-zero Objective on infeasible LP got %v, want ErrCertificationFailed", errInfObj)
		}
	})

	t.Run("MIN02VIPRJSONParserParityWithLean", func(t *testing.T) {
		t.Parallel()
		// Check negative id/col rejected even with zero multiplier.
		inst := &certVIPRInstance{
			n:        1,
			objConst: new(big.Rat),
			objDense: []*big.Rat{big.NewRat(1, 1)},
			objTerms: []certRatTerm{{col: 0, val: big.NewRat(1, 1)}},
			isInt:    []bool{true},
			lb:       []*big.Rat{big.NewRat(0, 1)},
			ub:       []*big.Rat{big.NewRat(1, 1)},
		}
		negID := -1
		if _, ok := certEvalLinComb(inst, nil, GreaterEqual, []certLinRefDoc{{ID: &negID, Mult: viprRawJSON("0")}}, nil); ok {
			t.Fatal("certEvalLinComb accepted negative ref ID with mult=0")
		}
		isUp := true
		if _, ok := certEvalLinComb(inst, nil, GreaterEqual, nil, []certBoundRefDoc{{Col: &negID, IsUpper: &isUp, Mult: viprRawJSON("0")}}); ok {
			t.Fatal("certEvalLinComb accepted negative bound col with mult=0")
		}

		// Check empty/null sense and kind rejected when key is present alongside isUpper.
		for _, rawStep := range []string{
			`{"rule":"assume","col":0,"bound":1,"sense":"","isUpper":true}`,
			`{"rule":"assume","col":0,"bound":1,"sense":null,"isUpper":true}`,
			`{"rule":"lincomb","sense":"<=","refs":[],"bounds":null}`,
			`{"rule":"lincomb","sense":"<=","refs":[],"bounds":[{"col":0,"kind":"","isUpper":true,"mult":1}]}`,
			`{"rule":"lincomb","sense":"<=","refs":[],"bounds":[{"col":0,"kind":null,"isUpper":true,"mult":1}]}`,
			`{"rule":"safeDual","refs":[],"boundAsms":null}`,
		} {
			st, okU := unmarshalCertStepDoc(viprRawJSON(rawStep))
			if okU {
				if _, okE := certEvalProofStep(inst, nil, st); okE {
					t.Fatalf("step %s unexpectedly succeeded", rawStep)
				}
			}
		}

		// Check null rootStep rejected.
		docNullRoot := `{"n":1,"obj":[1],"isInt":[true],"lb":[0],"ub":[1],"baseRows":[],"mode":"optimal","incumbent":[0],"steps":[{"rule":"safeDual","refs":[]}],"rootStep":null}`
		if _, err := verifyMiniVIPRBytes([]byte(docNullRoot), nil); !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("null rootStep got err=%v, want ErrCertificationFailed", err)
		}

		// Check leading plus sign and whitespace rejected in parseVIPRBigInt.
		for _, bad := range []string{`"+1"`, `" 1"`, `"1 "`, `" +2 "`} {
			if _, ok := parseVIPRBigInt(viprRawJSON(bad)); ok {
				t.Fatalf("parseVIPRBigInt accepted %s", bad)
			}
		}

		// Check unpaired UTF-16 surrogates rejected in scanVIPRJSONString while valid pairs succeed.
		for _, bad := range []string{`"\uD800"`, `"\uDC00"`, `"\uD800abc"`, `"\uD800\u0041"`} {
			if _, _, ok := scanVIPRJSONString([]byte(bad), 0); ok {
				t.Fatalf("scanVIPRJSONString accepted unpaired surrogate %s", bad)
			}
		}
		if s, _, ok := scanVIPRJSONString([]byte(`"\uD83D\uDE00"`), 0); !ok || s != "😀" {
			t.Fatalf("scanVIPRJSONString failed valid surrogate pair: got (%q, %v)", s, ok)
		}
	})

	t.Run("MAJ03SubToleranceFractionalIntLeafAndBoundFallback", func(t *testing.T) {
		t.Parallel()
		// Construct a small MILP where binary variable y takes 1 - 1.2e-16 in the exact LP
		// relaxation, matching the miplib_egout near-degenerate capacity pattern.
		mFrac := NewModel("sub_tol_frac_binary", Minimize)
		y := mFrac.AddBinaryVar("y")
		f := mFrac.AddContVar("f", 0, 100)
		s := mFrac.AddContVar("s", 0, 100)
		mFrac.SetObjective(NewExpr(y.T(15.82), f.T(0.025), s.T(100.0)))
		mFrac.AddConstraint("demand", Eq(NewExpr(f.T(1), s.T(1)), 22.59))
		mFrac.AddConstraint("cap", Le(NewExpr(f.T(1), y.T(-math.Nextafter(22.59, 23.0))), 0))
		pFrac := mFrac.Compile()
		trFrac := NewVIPRTrace()
		resFrac, errFrac := SolveCompiled(ctx, pFrac, WithCertified(true), WithVIPRTrace(trFrac))
		if errFrac != nil || resFrac.Status != StatusOptimal || trFrac.Mode() != "optimal" {
			t.Fatalf("sub-tolerance fractional binary MILP failed: status=%v mode=%q err=%v", resFrac.Status, trFrac.Mode(), errFrac)
		}
		if errV := VerifyVIPRTrace(pFrac, trFrac, resFrac, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace failed on sub-tolerance fractional binary MILP: %v", errV)
		}

		// Verify post-root limit exit emits a valid bound-mode certificate under WithCertified(true).
		mLim := NewModel("cert_limit_bound_mode", Maximize)
		const nVars = 14
		vars := make([]Var, nVars)
		objTerms := make([]Term, nVars)
		capTerms := make([]Term, nVars)
		for j := range nVars {
			vars[j] = mLim.AddBinaryVar(string(rune('a' + j)))
			objTerms[j] = vars[j].T(float64(17 + (j*13)%19))
			capTerms[j] = vars[j].T(float64(7 + (j*11)%17))
		}
		mLim.SetObjective(NewExpr(objTerms...))
		mLim.AddConstraint("cap", Le(NewExpr(capTerms...), 53.5))
		pLim := mLim.Compile()
		trLim := NewVIPRTrace()
		resLim, errLim := SolveCompiled(ctx, pLim, WithCertified(true), WithVIPRTrace(trLim), WithMaxNodes(2), WithCuts(false))
		if errLim != nil || (resLim.Status != StatusNodeLimit && resLim.Status != StatusOptimal) {
			t.Fatalf("certified node-limit MIP failed: status=%v err=%v", resLim.Status, errLim)
		}
		if !trLim.HasCertificate() {
			t.Fatal("expected certified node-limit MIP to emit a VIPR certificate")
		}
		if errV := VerifyVIPRTrace(pLim, trLim, resLim, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace failed on certified node-limit MIP: %v", errV)
		}
	})
}

func assertWitnessZeroed(t *testing.T, label string, w CertifiedWitness) {
	t.Helper()
	if w.Status != 0 || w.Values != nil || w.Duals != nil || w.ReducedCosts != nil ||
		w.Objective != nil || w.BestBound != nil || w.Farkas != nil || w.Unbounded != nil {
		t.Fatalf("%s: expected zeroed CertifiedWitness, got %+v", label, w)
	}
}

func TestCertifiedWitnessAndDeterministicBudget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("OptionWiringAndNilHandling", func(t *testing.T) {
		t.Parallel()
		if cfg := ApplyOptions(WithCertifiedWitness(nil)); cfg.Certified || cfg.certWitness != nil {
			t.Fatalf("WithCertifiedWitness(nil) unexpectedly enabled certification: %+v", cfg)
		}
		var w CertifiedWitness
		if cfg := ApplyOptions(WithCertifiedWitness(&w), WithCertified(false)); cfg.Certified || cfg.certWitness != nil {
			t.Fatalf("WithCertified(false) did not clear certWitness: %+v", cfg)
		}
	})

	t.Run("LPNonDyadicOneThirdAndOutwardMax", func(t *testing.T) {
		t.Parallel()
		m := NewModel("lp_witness_one_third", Minimize)
		z := m.AddContVar("z", 0, math.Inf(1))
		m.SetObjective(VarExpr(z))
		m.AddConstraint("c", Ge(NewExpr(z.T(3)), 1))

		var wit CertifiedWitness
		res, err := m.Solve(ctx, WithCertifiedWitness(&wit))
		if err != nil || res.Status != StatusOptimal || wit.Status != StatusOptimal {
			t.Fatalf("lp 1/3 solve failed: res.Status=%v wit.Status=%v err=%v", res.Status, wit.Status, err)
		}
		oneThird := big.NewRat(1, 3)
		if len(wit.Values) != 1 || wit.Values[0].Cmp(oneThird) != 0 {
			t.Fatalf("wit.Values = %v, want [1/3]", wit.Values)
		}
		threeZ := new(big.Rat).Mul(big.NewRat(3, 1), wit.Values[0])
		if threeZ.Cmp(big.NewRat(1, 1)) != 0 {
			t.Fatalf("3 * wit.Values[0] = %v, want 1", threeZ)
		}
		if wit.Objective == nil || wit.Objective.Cmp(oneThird) != 0 {
			t.Fatalf("wit.Objective = %v, want 1/3", wit.Objective)
		}
		if wit.BestBound == nil || wit.BestBound.Cmp(oneThird) != 0 {
			t.Fatalf("wit.BestBound = %v, want 1/3", wit.BestBound)
		}
		if len(wit.Duals) != 1 || wit.Duals[0].Cmp(oneThird) != 0 {
			t.Fatalf("wit.Duals = %v, want [1/3]", wit.Duals)
		}
		if len(wit.ReducedCosts) != 1 || wit.ReducedCosts[0].Sign() != 0 {
			t.Fatalf("wit.ReducedCosts = %v, want [0]", wit.ReducedCosts)
		}
		if wit.Farkas != nil || wit.Unbounded != nil {
			t.Fatalf("unexpected non-nil Farkas/Unbounded on optimal LP: %+v", wit)
		}
		if res.Values[0] != exactRatToFloat(wit.Values[0]) ||
			res.Objective != exactRatToFloat(wit.Objective) ||
			res.BestBound != exactRatToOutwardBound(Minimize, wit.BestBound) {
			t.Fatalf("float projection mismatch: res=(val=%g, obj=%g, bb=%g)",
				res.Values[0], res.Objective, res.BestBound)
		}

		// Maximize 2/3 tests ceil outward BestBound and negated user dual sign.
		mMax := NewModel("lp_witness_max_two_thirds", Maximize)
		zm := mMax.AddContVar("z", 0, math.Inf(1))
		mMax.SetObjective(VarExpr(zm))
		mMax.AddConstraint("c", Le(NewExpr(zm.T(3)), 2))

		var witMax CertifiedWitness
		resMax, errMax := mMax.Solve(ctx, WithCertifiedWitness(&witMax))
		if errMax != nil || resMax.Status != StatusOptimal || witMax.Status != StatusOptimal {
			t.Fatalf("max 2/3 solve failed: res.Status=%v wit.Status=%v err=%v", resMax.Status, witMax.Status, errMax)
		}
		twoThirds := big.NewRat(2, 3)
		if len(witMax.Values) != 1 || witMax.Values[0].Cmp(twoThirds) != 0 {
			t.Fatalf("witMax.Values = %v, want [2/3]", witMax.Values)
		}
		if witMax.BestBound == nil || witMax.BestBound.Cmp(twoThirds) != 0 {
			t.Fatalf("witMax.BestBound = %v, want 2/3", witMax.BestBound)
		}
		if new(big.Rat).SetFloat64(resMax.BestBound).Cmp(twoThirds) < 0 {
			t.Fatalf("resMax.BestBound = %.17g below exact 2/3", resMax.BestBound)
		}
		if len(witMax.Duals) != 1 || witMax.Duals[0].Cmp(oneThird) != 0 {
			t.Fatalf("witMax.Duals = %v, want [1/3]", witMax.Duals)
		}
	})

	t.Run("MILPNonDyadicWitnessAndCloneIndependence", func(t *testing.T) {
		t.Parallel()
		m := NewModel("milp_witness_one_third", Minimize)
		z := m.AddContVar("z", 0, math.Inf(1))
		x := m.AddIntVar("x", 1, 5)
		m.SetObjective(VarExpr(z))
		m.AddConstraint("c", Ge(NewExpr(z.T(3), x.T(-1)), 0))
		p := m.Compile()

		var wit CertifiedWitness
		tr := NewVIPRTrace()
		res, err := SolveCompiled(ctx, p, WithCertifiedWitness(&wit), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusOptimal || wit.Status != StatusOptimal {
			t.Fatalf("milp 1/3 solve failed: res.Status=%v wit.Status=%v err=%v", res.Status, wit.Status, err)
		}
		oneThird := big.NewRat(1, 3)
		if len(wit.Values) != 2 || wit.Values[0].Cmp(oneThird) != 0 || wit.Values[1].Cmp(big.NewRat(1, 1)) != 0 {
			t.Fatalf("wit.Values = %v, want [1/3, 1]", wit.Values)
		}
		lhs := new(big.Rat).Sub(new(big.Rat).Mul(big.NewRat(3, 1), wit.Values[0]), wit.Values[1])
		if lhs.Sign() != 0 {
			t.Fatalf("3*z - x = %v, want 0", lhs)
		}
		if wit.Objective == nil || wit.Objective.Cmp(oneThird) != 0 ||
			wit.BestBound == nil || wit.BestBound.Cmp(oneThird) != 0 {
			t.Fatalf("wit obj/bound = (%v, %v), want (1/3, 1/3)", wit.Objective, wit.BestBound)
		}
		if wit.Duals != nil || wit.ReducedCosts != nil || wit.Farkas != nil || wit.Unbounded != nil {
			t.Fatalf("expected nil LP/proof-specific fields on optimal MILP: %+v", wit)
		}

		// Mutating returned witness must not affect internal trace verification.
		wit.Values[0].SetInt64(999)
		wit.Objective.SetInt64(999)
		if errV := VerifyVIPRTrace(p, tr, res, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace failed after caller mutated witness: %v", errV)
		}
	})

	t.Run("InfeasibleFarkasAndEmptyRowAndMILP", func(t *testing.T) {
		t.Parallel()
		mLP := NewModel("lp_infeas_farkas", Minimize)
		x := mLP.AddContVar("x", 0, 10)
		y := mLP.AddContVar("y", 0, 10)
		mLP.SetObjective(NewExpr(x.T(1), y.T(1)))
		mLP.AddConstraint("le", Le(NewExpr(x.T(1), y.T(1)), 1))
		mLP.AddConstraint("ge", Ge(NewExpr(x.T(1), y.T(1)), 2))
		pLP := mLP.Compile()

		var witLP CertifiedWitness
		resLP, errLP := SolveCompiled(ctx, pLP, WithCertifiedWitness(&witLP))
		if errLP != nil || resLP.Status != StatusInfeasible || witLP.Status != StatusInfeasible {
			t.Fatalf("infeas LP failed: status=(%v, %v) err=%v", resLP.Status, witLP.Status, errLP)
		}
		if len(witLP.Farkas) != 2 || !verifyExactFarkasRayRat(pLP, witLP.Farkas) {
			t.Fatalf("invalid LP Farkas witness: %v", witLP.Farkas)
		}
		if witLP.Values != nil || witLP.Duals != nil || witLP.Objective != nil || witLP.BestBound != nil {
			t.Fatalf("unexpected fields on infeasible LP: %+v", witLP)
		}

		// Empty infeasible row produces a valid Farkas ray.
		mEmpty := NewModel("lp_empty_infeas", Minimize)
		xe := mEmpty.AddContVar("x", 0, 1)
		mEmpty.SetObjective(VarExpr(xe))
		mEmpty.AddConstraint("empty", Le(Expr{}, -1))
		pEmpty := mEmpty.Compile()
		var witEmpty CertifiedWitness
		resEmpty, errEmpty := SolveCompiled(ctx, pEmpty, WithCertifiedWitness(&witEmpty))
		if errEmpty != nil || resEmpty.Status != StatusInfeasible || witEmpty.Status != StatusInfeasible {
			t.Fatalf("empty infeas LP failed: status=(%v, %v) err=%v", resEmpty.Status, witEmpty.Status, errEmpty)
		}
		if len(witEmpty.Farkas) != 1 || !verifyExactFarkasRayRat(pEmpty, witEmpty.Farkas) {
			t.Fatalf("invalid empty-row Farkas witness: %v", witEmpty.Farkas)
		}

		// Pure bound conflict leaves Farkas nil.
		mBnd := NewModel("lp_bnd_infeas", Minimize)
		mBnd.AddContVar("x", 3, 2)
		var witBnd CertifiedWitness
		resBnd, errBnd := mBnd.Solve(ctx, WithCertifiedWitness(&witBnd))
		if errBnd != nil || resBnd.Status != StatusInfeasible || witBnd.Status != StatusInfeasible || witBnd.Farkas != nil {
			t.Fatalf("bound conflict witness got status=(%v, %v) farkas=%v err=%v", resBnd.Status, witBnd.Status, witBnd.Farkas, errBnd)
		}

		// Infeasible MILP populates StatusInfeasible cleanly.
		mMIP := NewModel("mip_infeas_wit", Minimize)
		xm := mMIP.AddIntVar("x", 0, 5)
		mMIP.SetObjective(VarExpr(xm))
		mMIP.AddConstraint("odd", Eq(NewExpr(xm.T(2)), 1))
		var witMIP CertifiedWitness
		resMIP, errMIP := mMIP.Solve(ctx, WithCertifiedWitness(&witMIP))
		if errMIP != nil || resMIP.Status != StatusInfeasible || witMIP.Status != StatusInfeasible || witMIP.Values != nil || witMIP.Objective != nil {
			t.Fatalf("infeas MILP witness got status=(%v, %v) wit=%+v err=%v", resMIP.Status, witMIP.Status, witMIP, errMIP)
		}
	})

	t.Run("UnboundedLPAndMILP", func(t *testing.T) {
		t.Parallel()
		mLP := NewModel("lp_unb_wit", Maximize)
		x := mLP.AddContVar("x", 1, math.Inf(1))
		mLP.SetObjective(VarExpr(x))
		pLP := mLP.Compile()

		var witLP CertifiedWitness
		resLP, errLP := SolveCompiled(ctx, pLP, WithCertifiedWitness(&witLP))
		if errLP != nil || resLP.Status != StatusUnbounded || witLP.Status != StatusUnbounded {
			t.Fatalf("unbounded LP failed: status=(%v, %v) err=%v", resLP.Status, witLP.Status, errLP)
		}
		if witLP.Unbounded == nil || witLP.Unbounded.IsMIP || !VerifyUnboundedCertificate(pLP, *witLP.Unbounded) {
			t.Fatalf("invalid LP unbounded witness: %+v", witLP.Unbounded)
		}

		mMIP := NewModel("mip_unb_wit", Minimize)
		ux := mMIP.AddIntVar("ux", 0, math.Inf(1))
		uy := mMIP.AddIntVar("uy", 0, math.Inf(1))
		mMIP.AddConstraint("dio", Eq(NewExpr(ux.T(2), uy.T(-3)), 1))
		mMIP.SetObjective(NewExpr(ux.T(-1), uy.T(-1)))
		pMIP := mMIP.Compile()

		var witMIP CertifiedWitness
		resMIP, errMIP := SolveCompiled(ctx, pMIP, WithCertifiedWitness(&witMIP))
		if errMIP != nil || resMIP.Status != StatusUnbounded || witMIP.Status != StatusUnbounded {
			t.Fatalf("unbounded MILP failed: status=(%v, %v) err=%v", resMIP.Status, witMIP.Status, errMIP)
		}
		if witMIP.Unbounded == nil || !witMIP.Unbounded.IsMIP || !VerifyUnboundedCertificate(pMIP, *witMIP.Unbounded) {
			t.Fatalf("invalid MILP unbounded witness: %+v", witMIP.Unbounded)
		}
	})

	t.Run("LimitBestBoundAndDeterministicBudget", func(t *testing.T) {
		t.Parallel()
		mLim := NewModel("wit_limit_bound_mode", Maximize)
		const nVars = 14
		vars := make([]Var, nVars)
		objTerms := make([]Term, nVars)
		capTerms := make([]Term, nVars)
		for j := range nVars {
			vars[j] = mLim.AddBinaryVar(string(rune('a' + j)))
			objTerms[j] = vars[j].T(float64(17 + (j*13)%19))
			capTerms[j] = vars[j].T(float64(7 + (j*11)%17))
		}
		mLim.SetObjective(NewExpr(objTerms...))
		mLim.AddConstraint("cap", Le(NewExpr(capTerms...), 53.5))
		pLim := mLim.Compile()

		var witLim CertifiedWitness
		trLim := NewVIPRTrace()
		resLim, errLim := SolveCompiled(ctx, pLim, WithCertifiedWitness(&witLim), WithVIPRTrace(trLim), WithMaxNodes(2), WithCuts(false))
		if errLim != nil || resLim.Status != StatusNodeLimit || witLim.Status != StatusNodeLimit {
			t.Fatalf("limit MILP failed: status=(%v, %v) err=%v", resLim.Status, witLim.Status, errLim)
		}
		if witLim.BestBound == nil || resLim.BestBound != exactRatToOutwardBound(Maximize, witLim.BestBound) {
			t.Fatalf("limit MILP BestBound mismatch: wit=%v res=%.17g", witLim.BestBound, resLim.BestBound)
		}
		if len(resLim.Values) > 0 {
			if len(witLim.Values) != nVars || witLim.Objective == nil || resLim.Objective != exactRatToFloat(witLim.Objective) {
				t.Fatalf("limit MILP incumbent mismatch: witObj=%v resObj=%.17g len(witVals)=%d",
					witLim.Objective, resLim.Objective, len(witLim.Values))
			}
		}

		// Negative TimeLimit on LP yields trivial infinite BestBound (nil rational bound).
		mLP := NewModel("wit_lp_tl", Minimize)
		xl := mLP.AddContVar("x", 0, 10)
		mLP.SetObjective(VarExpr(xl))
		mLP.AddConstraint("c", Ge(VarExpr(xl), 1))
		var witTL CertifiedWitness
		resTL, errTL := mLP.Solve(ctx, WithCertifiedWitness(&witTL), WithTimeLimit(-time.Millisecond))
		if errTL != nil || resTL.Status != StatusTimeLimit || witTL.Status != StatusTimeLimit {
			t.Fatalf("LP time limit failed: status=(%v, %v) err=%v", resTL.Status, witTL.Status, errTL)
		}
		if witTL.BestBound != nil || witTL.Values != nil || witTL.Objective != nil {
			t.Fatalf("expected nil finite fields on trivial LP time limit, got %+v", witTL)
		}

		// Verify deterministic work-budget enforcement on exactStop.
		pLP := mLP.Compile()
		lp := NewLPSolver(pLP, nil)
		defer lp.Close()
		detStop := exactStop{maxLPIter: 1, maxMIPNodes: 1}
		detStop.applyToLPSolver(lp)
		if lp.MaxIter != 1 || !lp.Deadline.IsZero() {
			t.Fatalf("exactStop.applyToLPSolver got MaxIter=%d Deadline=%v, want MaxIter=1 and zero Deadline",
				lp.MaxIter, lp.Deadline)
		}
		// Verify findExactMIPFeasibleAnchor respects maxMIPNodes / maxLPIter deterministically.
		mHard := NewModel("hard_anchor", Minimize)
		hVars := make([]Var, 10)
		var oddTerms []Term
		for j := range hVars {
			hVars[j] = mHard.AddBinaryVar(string(rune('a' + j)))
			oddTerms = append(oddTerms, hVars[j].T(2))
		}
		mHard.SetObjective(VarExpr(hVars[0]))
		mHard.AddConstraint("odd_eq", Eq(NewExpr(oddTerms...), 3))
		if _, okAnchor := findExactMIPFeasibleAnchor(exactStop{maxLPIter: 1, maxMIPNodes: 1}, mHard.Compile()); okAnchor {
			t.Fatal("findExactMIPFeasibleAnchor unexpectedly succeeded on infeasible odd equality")
		}
		if errV := VerifyVIPRTrace(pLim, trLim, resLim, ApplyOptions(WithCertified(true))); errV != nil {
			t.Fatalf("VerifyVIPRTrace with deterministic budget failed: %v", errV)
		}
	})

	t.Run("EntryAndFaultInjectionFailClosedClearsWitness", func(t *testing.T) {
		t.Parallel()
		dirtyWitness := func() CertifiedWitness {
			return CertifiedWitness{
				Status:       StatusOptimal,
				Values:       []*big.Rat{big.NewRat(7, 1)},
				Duals:        []*big.Rat{big.NewRat(2, 1)},
				ReducedCosts: []*big.Rat{big.NewRat(3, 1)},
				Objective:    big.NewRat(9, 1),
				BestBound:    big.NewRat(9, 1),
				Farkas:       []*big.Rat{big.NewRat(1, 1)},
				Unbounded:    &UnboundedCertificate{X0: []*big.Rat{big.NewRat(1, 1)}},
			}
		}

		// Fault injection on LP must fail closed and clear *w.
		mLP := NewModel("fault_lp", Minimize)
		x := mLP.AddContVar("x", 0, 10)
		mLP.SetObjective(VarExpr(x))
		mLP.AddConstraint("c", Ge(VarExpr(x), 1))
		faultLP := withGoblinCertFault(ctx, func(_ *CompiledProblem, _ *VIPRTrace, _ *lpCertWitness, res *Result) {
			res.Objective = 999
		})
		wit := dirtyWitness()
		res, err := mLP.Solve(faultLP, WithCertifiedWitness(&wit))
		if !errors.Is(err, ErrCertificationFailed) || res.Status != StatusNumericalError {
			t.Fatalf("faulted LP got status=%v err=%v, want StatusNumericalError + ErrCertificationFailed", res.Status, err)
		}
		assertWitnessZeroed(t, "fault_lp", wit)

		// Fault injection on MILP must fail closed and clear *w.
		mMIP := NewModel("fault_mip", Minimize)
		xi := mMIP.AddIntVar("x", 0, 10)
		mMIP.SetObjective(VarExpr(xi))
		mMIP.AddConstraint("c", Ge(NewExpr(xi.T(2)), 3))
		witM := dirtyWitness()
		resM, errM := mMIP.Solve(faultLP, WithCertifiedWitness(&witM))
		if !errors.Is(errM, ErrCertificationFailed) || resM.Status != StatusNumericalError {
			t.Fatalf("faulted MILP got status=%v err=%v, want StatusNumericalError + ErrCertificationFailed", resM.Status, errM)
		}
		assertWitnessZeroed(t, "fault_mip", witM)

		// Early-exit errors (nil model, invalid model, canceled context) must also clear *w.
		witNil := dirtyWitness()
		if _, errNil := (*Model)(nil).Solve(ctx, WithCertifiedWitness(&witNil)); !errors.Is(errNil, ErrNilModel) {
			t.Fatalf("nil model got err=%v, want ErrNilModel", errNil)
		}
		assertWitnessZeroed(t, "nil_model", witNil)

		mInv := NewModel("invalid_const", Minimize)
		xiInv := mInv.AddContVar("x", 0, 1)
		mInv.SetObjective(NewExpr(xiInv.T(1)).AddConstant(math.NaN()))
		witInv := dirtyWitness()
		if _, errInv := mInv.Solve(ctx, WithCertifiedWitness(&witInv)); errInv == nil {
			t.Fatal("expected invalid model error")
		}
		assertWitnessZeroed(t, "invalid_model", witInv)

		cancCtx, cancel := context.WithCancel(ctx)
		cancel()
		witCanc := dirtyWitness()
		if _, errCanc := mLP.Solve(cancCtx, WithCertifiedWitness(&witCanc)); !errors.Is(errCanc, context.Canceled) {
			t.Fatalf("canceled ctx got err=%v, want context.Canceled", errCanc)
		}
		assertWitnessZeroed(t, "canceled_ctx", witCanc)
	})
}
