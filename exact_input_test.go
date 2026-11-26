package goblin_test

import (
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

func TestParseExactRat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want *big.Rat
	}{
		{"0", big.NewRat(0, 1)},
		{"-0", big.NewRat(0, 1)},
		{"+0.0", big.NewRat(0, 1)},
		{"0.9", big.NewRat(9, 10)},
		{"-7.2", big.NewRat(-36, 5)},
		{"1.5e-3", big.NewRat(3, 2000)},
		{"1.5E+2", big.NewRat(150, 1)},
		{".5", big.NewRat(1, 2)},
		{"5.", big.NewRat(5, 1)},
		{"+2.5", big.NewRat(5, 2)},
		{"9/10", big.NewRat(9, 10)},
		{"-72/10", big.NewRat(-36, 5)},
		{"010/3", big.NewRat(10, 3)},
		{"-00/07", big.NewRat(0, 1)},
	}
	for _, tc := range cases {
		got, err := gb.ParseExactRat(tc.in)
		if err != nil {
			t.Fatalf("ParseExactRat(%q) unexpected error: %v", tc.in, err)
		}
		if got.Cmp(tc.want) != 0 {
			t.Fatalf("ParseExactRat(%q) = %s, want %s", tc.in, got.RatString(), tc.want.RatString())
		}
	}

	if _, err := gb.ParseExactRat("1e10000"); err != nil {
		t.Fatalf("ParseExactRat(1e10000) unexpected error: %v", err)
	}
	if _, err := gb.ParseExactRat("1e-10000"); err != nil {
		t.Fatalf("ParseExactRat(1e-10000) unexpected error: %v", err)
	}

	bad := []string{
		"",
		" ",
		" 0.9",
		"0.9 ",
		"0.9\n",
		"9/10 ",
		"9 / 10",
		"NaN",
		"Inf",
		"+Inf",
		"-Inf",
		"1/0",
		"0/0",
		"1/-2",
		"1/+2",
		"1/2/3",
		"/2",
		"2/",
		".",
		"-",
		"+",
		"1e",
		"1e+",
		"1e10001",
		"1e-10001",
		"0.1e10002",
		"0.1.2",
		"0x10",
		"1_000",
	}
	for _, s := range bad {
		if r, err := gb.ParseExactRat(s); err == nil {
			t.Fatalf("ParseExactRat(%q) = %s, want error", s, r.RatString())
		}
	}
}

func TestExactRatAPICloningAndZeroOverhead(t *testing.T) {
	t.Parallel()

	// Standard float64 builders must keep exactOrig == nil.
	mFloat := gb.NewModel("float_only", gb.Minimize)
	fx := mFloat.AddContVar("fx", 0, 10)
	fy := mFloat.AddIntVar("fy", 1, 5)
	mFloat.AddConstraint("fc1", gb.NewExpr(fx.T(0.5), fy.T(1.5)).AddConstant(0.25).Le(4.5))
	mFloat.SetObjective(gb.Dot([]gb.Var{fx, fy}, []float64{1, 2}).Scale(2).AddConstant(1))
	pFloat, err := mFloat.CompileChecked()
	if err != nil {
		t.Fatalf("mFloat.CompileChecked: %v", err)
	}
	_, _, _, _, _, hasInexactFloat, okFloat := gb.ExactProblemRatsForTest(pFloat)
	if !okFloat || hasInexactFloat {
		t.Fatalf("float64 model ExactProblemRatsForTest = (hasInexact=%v, ok=%v), want (false, true)", hasInexactFloat, okFloat)
	}

	// Mutating caller *big.Rat inputs after construction must not affect compiled problem.
	mutLB := big.NewRat(1, 10)
	mutUB := big.NewRat(9, 10)
	mutCoeff := big.NewRat(-72, 10)
	mutRHS := big.NewRat(-1, 1)
	mutConst := big.NewRat(3, 10)
	mutObj := big.NewRat(1, 3)

	m := gb.NewModel("cloned_rat", gb.Minimize)
	x := m.AddContVarRat("x", mutLB, mutUB)
	y := m.AddVarRat("y", big.NewRat(0, 1), nil, gb.Continuous)
	z := m.AddIntVarRat("z", nil, big.NewRat(5, 1))
	expr := x.TRat(mutCoeff).Var.TDec("-4.1")
	_ = expr
	cExpr := gb.NewExpr(gb.TRat(x, mutCoeff), y.TDec("-4.1")).AddTermRat(z, big.NewRat(2, 5)).AddConstantRat(mutConst)
	m.AddConstraint("c1", cExpr.EqRat(mutRHS))
	m.SetObjective(gb.NewExpr().AddTermRat(x, mutObj).AddConstantRat(mutObj).Scale(2).AddConstant(0.5))

	// Mutate every caller *big.Rat after construction.
	mutLB.SetInt64(999)
	mutUB.SetInt64(999)
	mutCoeff.SetInt64(999)
	mutRHS.SetInt64(999)
	mutConst.SetInt64(999)
	mutObj.SetInt64(999)

	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("m.CompileChecked: %v", err)
	}
	objConst, objMin, rowRHS, rowCols, rowVals, hasInexact, okRats := gb.ExactProblemRatsForTest(p)
	if !okRats || !hasInexact {
		t.Fatalf("ExactProblemRatsForTest = (hasInexact=%v, ok=%v), want (true, true)", hasInexact, okRats)
	}
	// objConst = (1/3)*2 + 1/2 = 7/6
	wantObjConst := big.NewRat(7, 6)
	if objConst.Cmp(wantObjConst) != 0 {
		t.Fatalf("objConst = %s, want %s", objConst.RatString(), wantObjConst.RatString())
	}
	// objMin[0] = (1/3)*2 = 2/3
	if objMin[0].Cmp(big.NewRat(2, 3)) != 0 {
		t.Fatalf("objMin[0] = %s, want 2/3", objMin[0].RatString())
	}
	// rowRHS = -1 - 3/10 = -13/10
	if rowRHS[0].Cmp(big.NewRat(-13, 10)) != 0 {
		t.Fatalf("rowRHS[0] = %s, want -13/10", rowRHS[0].RatString())
	}
	wantRow := map[int]*big.Rat{
		0: big.NewRat(-36, 5),
		1: big.NewRat(-41, 10),
		2: big.NewRat(2, 5),
	}
	for k, col := range rowCols[0] {
		if rowVals[0][k].Cmp(wantRow[col]) != 0 {
			t.Fatalf("rowVals[0][col=%d] = %s, want %s", col, rowVals[0][k].RatString(), wantRow[col].RatString())
		}
	}

	// SetVarBounds resets exact bounds cleanly.
	m.SetVarBounds(x, 0, 1)
	pReset, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("pReset.CompileChecked: %v", err)
	}
	lo, hi := pReset.VarBounds(0)
	if lo != 0 || hi != 1 {
		t.Fatalf("after SetVarBounds bounds = [%v, %v], want [0, 1]", lo, hi)
	}
}

func TestExactRatDeferredErrorsAndOverflow(t *testing.T) {
	t.Parallel()

	// Invalid TDec literal records deferred validation error without panicking.
	{
		m := gb.NewModel("bad_tdec", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.AddConstraint("c", gb.Eq(gb.NewExpr(x.TDec(" 0.9")), 1))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on bad TDec literal, want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on bad TDec literal, want error")
		}
	}

	// Nil *big.Rat arguments record deferred validation errors without panicking.
	{
		m := gb.NewModel("nil_trat", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.SetObjective(gb.NewExpr(gb.TRat(x, nil)))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on TRat(x, nil), want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on TRat(x, nil), want error")
		}
	}
	for _, mk := range []func(gb.Expr) gb.Constraint{
		func(e gb.Expr) gb.Constraint { return e.LeRat(nil) },
		func(e gb.Expr) gb.Constraint { return e.GeRat(nil) },
		func(e gb.Expr) gb.Constraint { return e.EqRat(nil) },
		func(e gb.Expr) gb.Constraint { return gb.LeRat(e, nil) },
		func(e gb.Expr) gb.Constraint { return gb.GeRat(e, nil) },
		func(e gb.Expr) gb.Constraint { return gb.EqRat(e, nil) },
	} {
		m := gb.NewModel("nil_rhs", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.AddConstraint("c", mk(gb.VarExpr(x)))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on nil RHS rat, want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on nil RHS rat, want error")
		}
	}
	{
		m := gb.NewModel("nil_const", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.SetObjective(gb.VarExpr(x).AddConstantRat(nil))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on AddConstantRat(nil), want error")
		}
	}

	// Huge *big.Rat overflowing finite float64 must fail Validate and CompileChecked.
	hugeRat, err := gb.ParseExactRat("1e1000")
	if err != nil {
		t.Fatalf("ParseExactRat(1e1000): %v", err)
	}
	{
		m := gb.NewModel("huge_coeff", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.SetObjective(gb.NewExpr(x.TRat(hugeRat)))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on overflowing TRat, want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on overflowing TRat, want error")
		}
	}
	{
		m := gb.NewModel("huge_bound", gb.Minimize)
		m.AddContVarRat("x", nil, hugeRat)
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on overflowing AddContVarRat bound, want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on overflowing AddContVarRat bound, want error")
		}
	}
	{
		m := gb.NewModel("huge_rhs", gb.Minimize)
		x := m.AddContVar("x", 0, 1)
		m.AddConstraint("c", gb.VarExpr(x).LeRat(hugeRat))
		if err := m.Validate(); err == nil {
			t.Fatal("Validate succeeded on overflowing LeRat RHS, want error")
		}
		if _, err := m.CompileChecked(); err == nil {
			t.Fatal("CompileChecked succeeded on overflowing LeRat RHS, want error")
		}
	}
}

func TestExactDecimalEquationCertificationQ10(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Solve -7.2*x1 - 4.1*x2 = -1 over exact Q_10: min 2*x1 + x2 has x* = (0, 10/41), obj = 10/41.
	mExact := gb.NewModel("q10_eq", gb.Minimize)
	x1 := mExact.AddContVarRat("x1", big.NewRat(0, 1), nil)
	x2 := mExact.AddContVarRat("x2", big.NewRat(0, 1), nil)
	mExact.AddConstraint("eq", gb.EqRat(gb.NewExpr(x1.TDec("-7.2"), x2.TDec("-4.1")), big.NewRat(-1, 1)))
	mExact.SetObjective(gb.NewExpr(x1.TDec("2"), x2.TDec("1")))
	pExact := mExact.Compile()

	var wit gb.CertifiedWitness
	var tr gb.VIPRTrace
	certOpts := []gb.Option{gb.WithCertified(true), gb.WithCertifiedWitness(&wit), gb.WithVIPRTrace(&tr)}
	res, err := gb.SolveCompiled(ctx, pExact, certOpts...)
	if err != nil {
		t.Fatalf("pExact.SolveCompiled: %v", err)
	}
	if res.Status != gb.StatusOptimal {
		t.Fatalf("status = %v, want StatusOptimal", res.Status)
	}
	wantVal := big.NewRat(10, 41)
	if wit.Values[0].Sign() != 0 || wit.Values[1].Cmp(wantVal) != 0 {
		t.Fatalf("wit.Values = (%s, %s), want (0, 10/41)", wit.Values[0].RatString(), wit.Values[1].RatString())
	}
	if wit.Objective.Cmp(wantVal) != 0 || wit.BestBound.Cmp(wantVal) != 0 {
		t.Fatalf("wit bounds = (%s, %s), want (10/41, 10/41)", wit.Objective.RatString(), wit.BestBound.RatString())
	}
	appliedCert := gb.ApplyOptions(gb.WithCertified(true))
	if err := gb.VerifyVIPRTrace(pExact, &tr, res, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pExact): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "q10_eq", pExact, res, &tr, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Negative control: binary64 model -7.2*x1 - 4.1*x2 = -1 must fail exact Q_10 VIPR check.
	mFloat := gb.NewModel("f64_eq", gb.Minimize)
	fx1 := mFloat.AddContVar("x1", 0, math.Inf(1))
	fx2 := mFloat.AddContVar("x2", 0, math.Inf(1))
	mFloat.AddConstraint("eq", gb.Eq(gb.NewExpr(fx1.T(-7.2), fx2.T(-4.1)), -1))
	mFloat.SetObjective(gb.NewExpr(fx1.T(2), fx2.T(1)))
	pFloat := mFloat.Compile()
	if err := gb.VerifyVIPRTrace(pFloat, &tr, res, appliedCert); err == nil {
		t.Fatal("VerifyVIPRTrace accepted exact Q_10 trace against binary64 CompiledProblem")
	}

	// Exact continuous bounds [0.1, 0.9] certify exact 1/10 (not binary64 0.1).
	mBnd := gb.NewModel("q10_bounds", gb.Minimize)
	lb01, _ := gb.ParseExactRat("0.1")
	ub09, _ := gb.ParseExactRat("0.9")
	xb := mBnd.AddContVarRat("xb", lb01, ub09)
	mBnd.SetObjective(gb.VarExpr(xb))
	pBnd := mBnd.Compile()
	var witB gb.CertifiedWitness
	var trB gb.VIPRTrace
	resB, err := gb.SolveCompiled(ctx, pBnd, gb.WithCertified(true), gb.WithCertifiedWitness(&witB), gb.WithVIPRTrace(&trB))
	if err != nil || resB.Status != gb.StatusOptimal {
		t.Fatalf("pBnd.SolveCompiled = (%v, %v), want StatusOptimal", resB.Status, err)
	}
	if witB.Values[0].Cmp(big.NewRat(1, 10)) != 0 || witB.Objective.Cmp(big.NewRat(1, 10)) != 0 {
		t.Fatalf("pBnd exact witness = (%s, %s), want (1/10, 1/10)", witB.Values[0].RatString(), witB.Objective.RatString())
	}
	if err := gb.VerifyVIPRTrace(pBnd, &trB, resB, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pBnd): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "q10_bounds", pBnd, resB, &trB, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Exact integer GCD infeasibility over Q_10: 7.2*x1 + 14.4*x2 = 1 -> 72*x1 + 144*x2 = 10.
	mGCD := gb.NewModel("q10_gcd_infeas", gb.Minimize)
	gx1 := mGCD.AddIntVarRat("gx1", big.NewRat(0, 1), nil)
	gx2 := mGCD.AddIntVarRat("gx2", big.NewRat(0, 1), nil)
	mGCD.AddConstraint("eq", gb.EqRat(gb.NewExpr(gx1.TDec("7.2"), gx2.TDec("14.4")), big.NewRat(1, 1)))
	mGCD.SetObjective(gb.NewExpr(gx1.T(1), gx2.T(1)))
	pGCD := mGCD.Compile()
	var trGCD gb.VIPRTrace
	resGCD, err := gb.SolveCompiled(ctx, pGCD, gb.WithCertified(true), gb.WithVIPRTrace(&trGCD))
	if err != nil || resGCD.Status != gb.StatusInfeasible {
		t.Fatalf("pGCD.SolveCompiled = (%v, %v), want StatusInfeasible", resGCD.Status, err)
	}
	if err := gb.VerifyVIPRTrace(pGCD, &trGCD, resGCD, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pGCD): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "q10_gcd_infeas", pGCD, resGCD, &trGCD, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Continuous Farkas infeasibility over Q_10: 0.1*x + 0.2*x = 0.9 (duplicate sum 3/10) and 0.3*x <= 0.8.
	mFark := gb.NewModel("q10_farkas", gb.Minimize)
	fx := mFark.AddContVarRat("fx", big.NewRat(0, 1), nil)
	rhs09, _ := gb.ParseExactRat("0.9")
	rhs08, _ := gb.ParseExactRat("0.8")
	mFark.AddConstraint("eq", gb.EqRat(gb.NewExpr(fx.TDec("0.1"), fx.TDec("0.2")), rhs09))
	mFark.AddConstraint("le", gb.LeRat(gb.NewExpr(fx.TDec("0.3")), rhs08))
	mFark.SetObjective(gb.VarExpr(fx))
	pFark := mFark.Compile()
	var witFark gb.CertifiedWitness
	var trFark gb.VIPRTrace
	resFark, err := gb.SolveCompiled(ctx, pFark, gb.WithCertified(true), gb.WithCertifiedWitness(&witFark), gb.WithVIPRTrace(&trFark))
	if err != nil || resFark.Status != gb.StatusInfeasible {
		t.Fatalf("pFark.SolveCompiled = (%v, %v), want StatusInfeasible", resFark.Status, err)
	}
	if err := gb.VerifyVIPRTrace(pFark, &trFark, resFark, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pFark): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "q10_farkas", pFark, resFark, &trFark, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Exact integer row RHS tightening: x1, x2 in Z_>=0, 1.0*x1 + 1.0*x2 <= 5/2 -> max 2.
	mRowTight := gb.NewModel("q10_row_tight", gb.Maximize)
	rx1 := mRowTight.AddIntVarRat("rx1", big.NewRat(0, 1), nil)
	rx2 := mRowTight.AddIntVarRat("rx2", big.NewRat(0, 1), nil)
	mRowTight.AddConstraint("c", gb.LeRat(gb.NewExpr(rx1.TDec("1"), rx2.TDec("1")), big.NewRat(5, 2)))
	mRowTight.SetObjective(gb.NewExpr(rx1.TDec("1"), rx2.TDec("1")))
	pRowTight := mRowTight.Compile()
	var witRT gb.CertifiedWitness
	var trRT gb.VIPRTrace
	resRT, err := gb.SolveCompiled(ctx, pRowTight, gb.WithCertified(true), gb.WithCertifiedWitness(&witRT), gb.WithVIPRTrace(&trRT))
	if err != nil || resRT.Status != gb.StatusOptimal {
		t.Fatalf("pRowTight.SolveCompiled = (%v, %v), want StatusOptimal", resRT.Status, err)
	}
	if witRT.Objective.Cmp(big.NewRat(2, 1)) != 0 {
		t.Fatalf("pRowTight wit.Objective = %s, want 2", witRT.Objective.RatString())
	}
	if err := gb.VerifyVIPRTrace(pRowTight, &trRT, resRT, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pRowTight): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "q10_row_tight", pRowTight, resRT, &trRT, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Regression 1 (>2^53 LessEqual outward rounding): x1 in [0, 2^53] int, x2 in {0,1},
	// x1 + x2 <= 2^53 + 1.5, Max x1 + 2*x2 -> (2^53, 1), obj = 2^53 + 2.
	two53 := new(big.Int).Lsh(big.NewInt(1), 53)
	rhsLE := new(big.Rat).Add(new(big.Rat).SetInt(two53), big.NewRat(3, 2))
	mBigLE := gb.NewModel("big_int_le_tight", gb.Maximize)
	bl1 := mBigLE.AddIntVarRat("bl1", big.NewRat(0, 1), new(big.Rat).SetInt(two53))
	bl2 := mBigLE.AddIntVarRat("bl2", big.NewRat(0, 1), big.NewRat(1, 1))
	mBigLE.AddConstraint("c", gb.LeRat(gb.NewExpr(bl1.TRat(big.NewRat(1, 1)), bl2.TRat(big.NewRat(1, 1))), rhsLE))
	mBigLE.SetObjective(gb.NewExpr(bl1.T(1), bl2.T(2)))
	pBigLE := mBigLE.Compile()
	var witBigLE gb.CertifiedWitness
	var trBigLE gb.VIPRTrace
	resBigLE, err := gb.SolveCompiled(ctx, pBigLE, gb.WithCertified(true), gb.WithCertifiedWitness(&witBigLE), gb.WithVIPRTrace(&trBigLE))
	if err != nil || resBigLE.Status != gb.StatusOptimal {
		t.Fatalf("pBigLE.SolveCompiled = (%v, %v), want StatusOptimal", resBigLE.Status, err)
	}
	wantBigObj := new(big.Rat).SetInt(new(big.Int).Add(two53, big.NewInt(2)))
	if witBigLE.Values[0].Cmp(new(big.Rat).SetInt(two53)) != 0 || witBigLE.Values[1].Cmp(big.NewRat(1, 1)) != 0 || witBigLE.Objective.Cmp(wantBigObj) != 0 {
		t.Fatalf("pBigLE witness = (%s, %s, obj=%s), want (2^53, 1, 2^53+2)",
			witBigLE.Values[0].RatString(), witBigLE.Values[1].RatString(), witBigLE.Objective.RatString())
	}
	if err := gb.VerifyVIPRTrace(pBigLE, &trBigLE, resBigLE, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pBigLE): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "big_int_le_tight", pBigLE, resBigLE, &trBigLE, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Regression 2 (>2^53 GreaterEqual outward rounding): x1 in [0, 2^53] int, x2 in {0,1},
	// x1 + x2 >= 2^53 + 0.5, Min x1 + 2*x2 -> (2^53, 1), obj = 2^53 + 2.
	rhsGE := new(big.Rat).Add(new(big.Rat).SetInt(two53), big.NewRat(1, 2))
	mBigGE := gb.NewModel("big_int_ge_tight", gb.Minimize)
	bg1 := mBigGE.AddIntVarRat("bg1", big.NewRat(0, 1), new(big.Rat).SetInt(two53))
	bg2 := mBigGE.AddIntVarRat("bg2", big.NewRat(0, 1), big.NewRat(1, 1))
	mBigGE.AddConstraint("c", gb.GeRat(gb.NewExpr(bg1.TRat(big.NewRat(1, 1)), bg2.TRat(big.NewRat(1, 1))), rhsGE))
	mBigGE.SetObjective(gb.NewExpr(bg1.T(1), bg2.T(2)))
	pBigGE := mBigGE.Compile()
	var witBigGE gb.CertifiedWitness
	var trBigGE gb.VIPRTrace
	resBigGE, err := gb.SolveCompiled(ctx, pBigGE, gb.WithCertified(true), gb.WithCertifiedWitness(&witBigGE), gb.WithVIPRTrace(&trBigGE))
	if err != nil || resBigGE.Status != gb.StatusOptimal {
		t.Fatalf("pBigGE.SolveCompiled = (%v, %v), want StatusOptimal", resBigGE.Status, err)
	}
	if witBigGE.Values[0].Cmp(new(big.Rat).SetInt(two53)) != 0 || witBigGE.Values[1].Cmp(big.NewRat(1, 1)) != 0 || witBigGE.Objective.Cmp(wantBigObj) != 0 {
		t.Fatalf("pBigGE witness = (%s, %s, obj=%s), want (2^53, 1, 2^53+2)",
			witBigGE.Values[0].RatString(), witBigGE.Values[1].RatString(), witBigGE.Objective.RatString())
	}
	if err := gb.VerifyVIPRTrace(pBigGE, &trBigGE, resBigGE, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pBigGE): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "big_int_ge_tight", pBigGE, resBigGE, &trBigGE, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Regression 3 (PresolveForVIPR outward rowLB/rowUB rounding on >2^53 RHS).
	mPreBig := gb.NewModel("big_int_le_pre", gb.Maximize)
	pbl1 := mPreBig.AddIntVarRat("pbl1", big.NewRat(0, 1), new(big.Rat).SetInt(two53))
	pbl2 := mPreBig.AddIntVarRat("pbl2", big.NewRat(0, 1), big.NewRat(2, 1))
	mPreBig.AddConstraint("c", gb.LeRat(gb.NewExpr(pbl1.TRat(big.NewRat(1, 1)), pbl2.TRat(big.NewRat(1, 1))), rhsLE))
	mPreBig.SetObjective(gb.NewExpr(pbl1.T(1), pbl2.T(2)))
	pPreBig := mPreBig.Compile()
	preBig := gb.PresolveForVIPR(pPreBig, 6)
	if preBig.Infeasible || preBig.TimedOut || preBig.Reduced == nil || preBig.Reduced.NumConstraints() != 1 {
		t.Fatalf("PresolveForVIPR(pPreBig): infeas=%v timedOut=%v rows=%d",
			preBig.Infeasible, preBig.TimedOut, preBig.Reduced.NumConstraints())
	}
	if preBig.Reduced.RowRHS(0) < float64(two53.Int64()+2) {
		t.Fatalf("PresolveForVIPR reduced LessEqual RowRHS(0) = %.17g < 2^53+2", preBig.Reduced.RowRHS(0))
	}
	var trPreBig gb.VIPRTrace
	resPreBig, err := gb.SolveCompiled(ctx, preBig.Reduced, gb.WithCertified(true), gb.WithVIPRTrace(&trPreBig))
	if err != nil || resPreBig.Status != gb.StatusOptimal {
		t.Fatalf("preBig.Reduced.SolveCompiled = (%v, %v), want StatusOptimal", resPreBig.Status, err)
	}
	trPreBig.SetPresolveEnvelope(preBig.Envelope)
	if err := gb.VerifyVIPRTrace(pPreBig, &trPreBig, resPreBig, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pPreBig): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "big_int_le_pre", pPreBig, resPreBig, &trPreBig, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Regression 3b (un-tightened >2^53 exact integer RHS <= 2^53+1 with x1 in [0, 2^53], x2 in {0,1}).
	rhsExactIntBig := new(big.Rat).SetInt(new(big.Int).Add(two53, big.NewInt(1)))
	mBigExactInt := gb.NewModel("big_int_le_exact", gb.Maximize)
	ei1 := mBigExactInt.AddIntVarRat("ei1", big.NewRat(0, 1), new(big.Rat).SetInt(two53))
	ei2 := mBigExactInt.AddIntVarRat("ei2", big.NewRat(0, 1), big.NewRat(1, 1))
	mBigExactInt.AddConstraint("c", gb.LeRat(gb.NewExpr(ei1.TRat(big.NewRat(1, 1)), ei2.TRat(big.NewRat(1, 1))), rhsExactIntBig))
	mBigExactInt.SetObjective(gb.NewExpr(ei1.T(1), ei2.T(2)))
	pBigExactInt := mBigExactInt.Compile()
	var witBigExactInt gb.CertifiedWitness
	var trBigExactInt gb.VIPRTrace
	resBigExactInt, err := gb.SolveCompiled(ctx, pBigExactInt, gb.WithCertified(true), gb.WithCertifiedWitness(&witBigExactInt), gb.WithVIPRTrace(&trBigExactInt))
	if err != nil || resBigExactInt.Status != gb.StatusOptimal || witBigExactInt.Objective.Cmp(wantBigObj) != 0 {
		t.Fatalf("pBigExactInt.SolveCompiled = (%v, obj=%v, err=%v), want 2^53+2", resBigExactInt.Status, witBigExactInt.Objective, err)
	}
	if err := gb.VerifyVIPRTrace(pBigExactInt, &trBigExactInt, resBigExactInt, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pBigExactInt): %v", err)
	}

	// Regression 4 (mixed TDec + float AddConstant/Eq preserving rhsLo):
	// NewExpr(x.TDec("1")).AddConstant(0.2).Eq(1.2) has netRat = exact(1.2) - exact(0.2) != 1.
	mMixed := gb.NewModel("mixed_tdec_float_const", gb.Minimize)
	mx := mMixed.AddIntVar("mx", 0, 10)
	mMixed.AddConstraint("eq", gb.NewExpr(mx.TDec("1")).AddConstant(0.2).Eq(1.2))
	mMixed.SetObjective(gb.VarExpr(mx))
	pMixed := mMixed.Compile()
	var trMixed gb.VIPRTrace
	resMixed, err := gb.SolveCompiled(ctx, pMixed, gb.WithCertified(true), gb.WithVIPRTrace(&trMixed))
	if err != nil || resMixed.Status != gb.StatusInfeasible {
		t.Fatalf("pMixed.SolveCompiled = (%v, %v), want StatusInfeasible", resMixed.Status, err)
	}
	if err := gb.VerifyVIPRTrace(pMixed, &trMixed, resMixed, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(pMixed): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "mixed_tdec_float_const", pMixed, resMixed, &trMixed, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExactIntegerBoundTighteningAndVIPR(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	appliedCert := gb.ApplyOptions(gb.WithCertified(true))

	// [1e-20, 1 - 1e-20] rounds to [1, 1] in binary64, but exact ceil(1e-20)=1 > floor(1-1e-20)=0.
	lbTiny, _ := gb.ParseExactRat("1e-20")
	oneMinusTiny := new(big.Rat).Sub(big.NewRat(1, 1), lbTiny)
	mInfeas := gb.NewModel("sub_ulp_int_infeas", gb.Minimize)
	xi := mInfeas.AddIntVarRat("xi", lbTiny, oneMinusTiny)
	mInfeas.SetObjective(gb.VarExpr(xi))
	pInfeas := mInfeas.Compile()

	// Uncertified float64 solve operates on float64 bounds [1, 1] without tightening error.
	resUncert, err := gb.SolveCompiled(ctx, pInfeas)
	if err != nil {
		t.Fatalf("uncertified solve: %v", err)
	}
	if resUncert.Status != gb.StatusOptimal || resUncert.Objective != 1 {
		t.Fatalf("uncertified solve = (%v, obj=%v), want (Optimal, 1)", resUncert.Status, resUncert.Objective)
	}

	var trInfeas gb.VIPRTrace
	resCert, err := gb.SolveCompiled(ctx, pInfeas, gb.WithCertified(true), gb.WithVIPRTrace(&trInfeas))
	if err != nil {
		t.Fatalf("certified sub-ULP bound solve: %v", err)
	}
	if resCert.Status != gb.StatusInfeasible {
		t.Fatalf("certified status = %v, want StatusInfeasible", resCert.Status)
	}
	if err := gb.VerifyVIPRTrace(pInfeas, &trInfeas, resCert, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(sub_ulp_int_infeas): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "sub_ulp_int_infeas", pInfeas, resCert, &trInfeas, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// [1 + 1e-20, 3 - 1e-20] tightens to [2, 2] in exactMode, certifying x = 2 (not 1).
	onePlusTiny := new(big.Rat).Add(big.NewRat(1, 1), lbTiny)
	threeMinusTiny := new(big.Rat).Sub(big.NewRat(3, 1), lbTiny)
	mOpt := gb.NewModel("sub_ulp_int_opt", gb.Minimize)
	xo := mOpt.AddIntVarRat("xo", onePlusTiny, threeMinusTiny)
	mOpt.SetObjective(gb.VarExpr(xo))
	pOpt := mOpt.Compile()

	var witOpt gb.CertifiedWitness
	var trOpt gb.VIPRTrace
	resOpt, err := gb.SolveCompiled(ctx, pOpt, gb.WithCertified(true), gb.WithCertifiedWitness(&witOpt), gb.WithVIPRTrace(&trOpt))
	if err != nil {
		t.Fatalf("sub_ulp_int_opt solve: %v", err)
	}
	if resOpt.Status != gb.StatusOptimal {
		t.Fatalf("sub_ulp_int_opt status = %v, want StatusOptimal", resOpt.Status)
	}
	if witOpt.Values[0].Cmp(big.NewRat(2, 1)) != 0 || witOpt.Objective.Cmp(big.NewRat(2, 1)) != 0 {
		t.Fatalf("sub_ulp_int_opt witness = (%s, %s), want (2, 2)", witOpt.Values[0].RatString(), witOpt.Objective.RatString())
	}
	if err := gb.VerifyVIPRTrace(pOpt, &trOpt, resOpt, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(sub_ulp_int_opt): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "sub_ulp_int_opt", pOpt, resOpt, &trOpt, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExactPuLPJSONFlugplCertifiedAndFormalVIPR(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	appliedCert := gb.ApplyOptions(gb.WithCertified(true))

	raw, err := os.ReadFile(filepath.Join("testdata", "public", "miplib_flugpl.json"))
	if err != nil {
		t.Fatalf("read miplib_flugpl.json: %v", err)
	}

	mExact, err := format.FromPuLPJSONExactBytes(raw)
	if err != nil {
		t.Fatalf("FromPuLPJSONExactBytes: %v", err)
	}
	pExact, err := mExact.CompileChecked()
	if err != nil {
		t.Fatalf("pExact.CompileChecked: %v", err)
	}
	_, _, _, rowCols, rowVals, hasInexact, okRats := gb.ExactProblemRatsForTest(pExact)
	if !okRats || !hasInexact {
		t.Fatalf("flugpl ExactProblemRatsForTest = (hasInexact=%v, ok=%v), want (true, true)", hasInexact, okRats)
	}
	// Verify ANZ3 constraint keeps exact 9/10 for STM2 rather than float64(0.9).
	foundNineTenths := false
	for i := range pExact.NumConstraints() {
		if pExact.RowName(i) != "ANZ3" {
			continue
		}
		for k, col := range rowCols[i] {
			if strings.HasPrefix(pExact.VarName(col), "STM2") && rowVals[i][k].Cmp(big.NewRat(9, 10)) == 0 {
				foundNineTenths = true
			}
		}
	}
	if !foundNineTenths {
		t.Fatal("expected exact 9/10 coefficient for STM2 in ANZ3 row of miplib_flugpl")
	}

	// Standard FromPuLPJSON must remain untouched with exactOrig == nil.
	mFloat, err := format.FromPuLPJSONBytes(raw)
	if err != nil {
		t.Fatalf("FromPuLPJSONBytes: %v", err)
	}
	pFloat := mFloat.Compile()
	_, _, _, _, _, floatHasInexact, okFloat := gb.ExactProblemRatsForTest(pFloat)
	if !okFloat || floatHasInexact {
		t.Fatalf("float flugpl hasInexact=%v ok=%v, want (false, true)", floatHasInexact, okFloat)
	}

	var wit gb.CertifiedWitness
	var tr gb.VIPRTrace
	res, err := gb.SolveCompiled(ctx, pExact, gb.WithCertified(true), gb.WithCertifiedWitness(&wit), gb.WithVIPRTrace(&tr))
	if err != nil {
		t.Fatalf("flugpl exact certified solve: %v", err)
	}
	if res.Status != gb.StatusOptimal || res.Objective != 1201500.0 {
		t.Fatalf("flugpl status=%v obj=%.17g, want (StatusOptimal, 1201500)", res.Status, res.Objective)
	}
	wantObj := big.NewRat(1201500, 1)
	if wit.Objective.Cmp(wantObj) != 0 || wit.BestBound.Cmp(wantObj) != 0 {
		t.Fatalf("flugpl witness (obj=%s, bestBound=%s), want 1201500", wit.Objective.RatString(), wit.BestBound.RatString())
	}
	if err := gb.VerifyVIPRTrace(pExact, &tr, res, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(miplib_flugpl): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "miplib_flugpl_exact", pExact, res, &tr, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Verify PresolveForVIPR preserves exact 9/10 rationals and certifies end-to-end.
	pre := gb.PresolveForVIPR(pExact, 6)
	if pre.Infeasible || pre.TimedOut || pre.Reduced == nil {
		t.Fatalf("PresolveForVIPR(flugpl): infeas=%v timedOut=%v reduced=%v", pre.Infeasible, pre.TimedOut, pre.Reduced != nil)
	}
	var trPre gb.VIPRTrace
	resPre, err := gb.SolveCompiled(ctx, pre.Reduced, gb.WithCertified(true), gb.WithVIPRTrace(&trPre))
	if err != nil || resPre.Status != gb.StatusOptimal || resPre.Objective != 1201500.0 {
		t.Fatalf("flugpl presolved solve: status=%v obj=%.17g err=%v", resPre.Status, resPre.Objective, err)
	}
	trPre.SetPresolveEnvelope(pre.Envelope)
	if err := gb.VerifyVIPRTrace(pExact, &trPre, resPre, appliedCert); err != nil {
		t.Fatalf("VerifyVIPRTrace(miplib_flugpl_presolved): %v", err)
	}
	if os.Getenv("GOBLIN_FORMAL") == "1" {
		if err := checkFormalVIPRResult(t, "miplib_flugpl_exact_presolved", pExact, resPre, &trPre, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
}
