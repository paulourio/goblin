package goblin

import (
	"math"
	"math/big"
	"os"
	"strings"
	"testing"
)

func buildDD47Model() *Model {
	m := NewModel("dd47", Maximize)
	x0 := m.AddIntVar("x0", 0, math.Inf(1))
	x1 := m.AddBinaryVar("x1")
	x2 := m.AddBinaryVar("x2")
	x3 := m.AddBinaryVar("x3")
	x4 := m.AddContVar("x4", 0.5, 4.5)
	x5 := m.AddBinaryVar("x5")
	x6 := m.AddContVar("x6", 0.5, 4.5)
	x7 := m.AddContVar("x7", math.Inf(-1), math.Inf(1))

	m.SetObjective(
		NewExpr(
			x0.T(2), x1.T(-1), x2.T(-1), x3.T(-1),
			x4.T(-1), x5.T(-1), x6.T(-1), x7.T(2),
		).AddConstant(-0.1),
	)

	m.AddConstraint("r0", Ge(NewExpr(x7.T(-5e-6), x1.T(-1), x2.T(-1)), -1))
	m.AddConstraint("r1", Le(NewExpr(x0.T(5e-6), x1.T(-1), x7.T(-5e-6)), 0))
	for _, name := range []string{"r2", "r3", "r4", "r5", "r6", "r7", "r8"} {
		m.AddConstraint(name, Le(NewExpr(), 0))
	}
	return m
}

func TestVIPRFixedLeafDD47Regression(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	m := buildDD47Model()
	p := m.Compile()

	var viprPayloads [][]byte
	for _, presolve := range []bool{false, true} {
		for _, cuts := range []bool{false, true} {
			for _, cert := range []bool{false, true} {
				for _, workers := range []int{1, 4} {
					tr := NewVIPRTrace()
					var wit CertifiedWitness
					opts := []Option{
						WithPureGo(),
						WithPresolve(presolve),
						WithCuts(cuts),
						WithHeuristics(true),
						WithWorkers(workers),
						WithRelGapTol(1e-6),
						WithAbsGapTol(1e-6),
						WithVIPRTrace(tr),
					}
					if cert {
						opts = append(opts, WithCertifiedWitness(&wit))
					}
					res, err := SolveCompiled(ctx, p, opts...)
					if err != nil {
						t.Fatalf("presolve=%v cuts=%v cert=%v workers=%d: SolveCompiled failed: %v",
							presolve, cuts, cert, workers, err)
					}
					if res.Status != StatusOptimal {
						t.Fatalf("presolve=%v cuts=%v cert=%v workers=%d: status=%v, want Optimal",
							presolve, cuts, cert, workers, res.Status)
					}
					if len(res.Values) != 8 || math.Round(res.Values[0]) != 199999 {
						t.Fatalf("presolve=%v cuts=%v cert=%v workers=%d: values=%v, want x0=199999",
							presolve, cuts, cert, workers, res.Values)
					}
					if math.Abs(res.Objective-799996.9) > 1e-6 || math.Abs(res.BestBound-799996.9) > 1e-6 {
						t.Fatalf("presolve=%v cuts=%v cert=%v workers=%d: obj=%.10g bound=%.10g, want 799996.9",
							presolve, cuts, cert, workers, res.Objective, res.BestBound)
					}
					if !tr.HasCertificate() || tr.Mode() != string(viprModeOptimal) {
						t.Fatalf("presolve=%v cuts=%v cert=%v workers=%d: hasCert=%v mode=%q, want optimal",
							presolve, cuts, cert, workers, tr.HasCertificate(), tr.Mode())
					}
					raw, jErr := tr.MarshalVIPRJSON()
					if jErr != nil {
						t.Fatalf("MarshalVIPRJSON failed: %v", jErr)
					}
					if !presolve && !strings.Contains(string(raw), `"rule":"farkas"`) {
						t.Fatalf("presolve=false cuts=%v cert=%v: expected multi-row farkas leaf in VIPR JSON",
							cuts, cert)
					}
					viprPayloads = append(viprPayloads, raw)

					if cert {
						if len(wit.Values) != p.numVars || wit.Objective == nil || wit.BestBound == nil {
							t.Fatalf("missing CertifiedWitness fields for presolve=%v cuts=%v", presolve, cuts)
						}
						if wit.Objective.Cmp(wit.BestBound) != 0 {
							t.Fatalf("CertifiedWitness obj %s != bound %s", wit.Objective.RatString(), wit.BestBound.RatString())
						}
						if !verifyExactPrimalRat(p, wit.Values, true) {
							t.Fatalf("CertifiedWitness primal fails verifyExactPrimalRat")
						}
					}
				}
			}
		}
	}

	if len(viprPayloads) > 0 {
		if exe, ok := tryFormalExe(t, "vipr-check"); ok {
			runJSONLItems(t, exe, viprPayloads)
		}
	}
}

func tryFormalExe(tb testing.TB, exeName string) (string, bool) {
	tb.Helper()
	if os.Getenv("GOBLIN_FORMAL") != "1" {
		return "", false
	}
	return formalExe(tb, exeName), true
}

func TestFormalVIPRFixedLeafDD47(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	ctx := t.Context()
	p := buildDD47Model().Compile()
	var items [][]byte
	for _, presolve := range []bool{false, true} {
		for _, cuts := range []bool{false, true} {
			for _, cert := range []bool{false, true} {
				tr := NewVIPRTrace()
				res, err := SolveCompiled(
					ctx, p,
					WithPureGo(),
					WithPresolve(presolve),
					WithCuts(cuts),
					WithHeuristics(true),
					WithRelGapTol(1e-6),
					WithAbsGapTol(1e-6),
					WithCertified(cert),
					WithVIPRTrace(tr),
				)
				if err != nil || res.Status != StatusOptimal || !tr.HasCertificate() || tr.Mode() != string(viprModeOptimal) {
					t.Fatalf("presolve=%v cuts=%v cert=%v: status=%v err=%v hasCert=%v mode=%q",
						presolve, cuts, cert, res.Status, err, tr.HasCertificate(), tr.Mode())
				}
				raw, jErr := tr.MarshalVIPRJSON()
				if jErr != nil {
					t.Fatalf("MarshalVIPRJSON failed: %v", jErr)
				}
				items = append(items, raw)
			}
		}
	}
	runJSONLItems(t, formalExe(t, "vipr-check"), items)
}

func TestVIPRFixedLeafExtractNodeElasticPhase1FarkasDD47(t *testing.T) {
	t.Parallel()
	m := buildDD47Model()
	p := m.Compile()
	tr := NewVIPRTrace()
	hook := tr.newBabRecorder(p, true)
	rec, ok := hook.(*babVIPRRecorder)
	if !ok || rec == nil {
		t.Fatalf("expected *babVIPRRecorder, got %T", hook)
	}
	defer rec.close()

	fixCols := []struct {
		col int
		val int64
	}{
		{0, 200000},
		{1, 0},
		{2, 0},
		{3, 0},
		{5, 0},
	}
	cur := rec.root
	for _, fc := range fixCols {
		left, _ := rec.recordBranch(cur, fc.col, big.NewInt(fc.val))
		cur = left
		if fc.val > 0 {
			_, right := rec.recordBranch(cur, fc.col, big.NewInt(fc.val-1))
			cur = right
		}
	}
	vn := cur

	lbR, ubR := rec.nodeBoundsRat(vn)
	w, okF := rec.extractNodeElasticPhase1Farkas(lbR, ubR)
	if !okF || len(w) < 2 {
		t.Fatalf("extractNodeElasticPhase1Farkas failed on dd47 leaf: ok=%v len=%d", okF, len(w))
	}
	if _, _, okProj := rec.projectFarkasMults(w, lbR, ubR); !okProj {
		t.Fatalf("extracted Phase-1 Farkas multipliers failed projectFarkasMults")
	}

	lbF := make([]float64, p.numVars)
	ubF := make([]float64, p.numVars)
	rec.fillNodeBoundsFloat(vn, lbF, ubF)
	hs := NewLPSolver(p, nil)
	defer hs.Close()
	hs.InitCrashBasis()
	hs.SetStructuralBounds(lbF, ubF)
	lpRes := hs.Solve()
	if lpRes.Status != StatusOptimal {
		t.Fatalf("expected float64 LP StatusOptimal on sub-ULP leaf, got %v", lpRes.Status)
	}
	if exactMIPFeasibleRat(p, lpRes.Primal) {
		t.Fatalf("expected float64 leaf primal to fail exactMIPFeasibleRat")
	}
	for row := range p.numConstraints {
		if rec.certifyRowActivityFarkas(vn, row) {
			t.Fatalf("single row %d unexpectedly certified multi-row dd47 leaf", row)
		}
	}
	_, infeas, okLeaf := rec.certifyOrPolishFixedLeaf(vn, hs, &lpRes, 0)
	if !okLeaf || !infeas || lpRes.Status != StatusInfeasible || vn.kind != viprNodeLeafFarkas {
		t.Fatalf("certifyOrPolishFixedLeaf got (infeas=%v, ok=%v, status=%v, kind=%v), want (true, true, Infeasible, viprNodeLeafFarkas)",
			infeas, okLeaf, lpRes.Status, vn.kind)
	}
}

func TestVIPRFixedLeafExactFeasibleContinuousPolish(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	m := NewModel("exact_cont_polish_leaf", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddContVar("x1", 0, 1)
	m.SetObjective(NewExpr(x0.T(2), x1.T(3)))
	m.AddConstraint("eq_third", Eq(NewExpr(x1.T(3), x0.T(-1)), 0))
	m.AddConstraint("min_x0", Ge(NewExpr(x0.T(1)), 1))
	p := m.Compile()

	tr := NewVIPRTrace()
	hook := tr.newBabRecorder(p, true)
	rec, ok := hook.(*babVIPRRecorder)
	if !ok || rec == nil {
		t.Fatalf("expected *babVIPRRecorder, got %T", hook)
	}
	defer rec.close()

	left, right := rec.recordBranch(rec.root, 0, big.NewInt(0))
	left.kind = viprNodeLeafFarkas
	left.farkas = []viprRowMult{{rowID: 1, mult: big.NewRat(-1, 1)}}
	lbF := []float64{1, 0}
	ubF := []float64{1, 1}
	hs := NewLPSolver(p, nil)
	defer hs.Close()
	hs.InitCrashBasis()
	hs.SetStructuralBounds(lbF, ubF)
	lpRes := hs.Solve()
	if lpRes.Status != StatusOptimal {
		t.Fatalf("expected StatusOptimal, got %v", lpRes.Status)
	}
	if exactMIPFeasibleRat(p, lpRes.Primal) {
		t.Fatalf("expected float64 1/3 primal to fail exactMIPFeasibleRat")
	}
	objMin, infeas, okLeaf := rec.certifyOrPolishFixedLeaf(right, hs, &lpRes, 0)
	if !okLeaf || infeas || math.Abs(objMin-3.0) > 1e-12 || right.kind != viprNodeLeafDual || right.bestDualRHS == nil {
		t.Fatalf("certifyOrPolishFixedLeaf got (objMin=%.10g infeas=%v ok=%v kind=%v)",
			objMin, infeas, okLeaf, right.kind)
	}
	if right.bestDualRHS.Cmp(big.NewRat(3, 1)) != 0 {
		t.Fatalf("polished bestDualRHS = %s, want 3", right.bestDualRHS.RatString())
	}

	solveTr := NewVIPRTrace()
	var wit CertifiedWitness
	res, err := SolveCompiled(
		ctx, p,
		WithPureGo(),
		WithPresolve(false),
		WithCuts(false),
		WithHeuristics(false),
		WithCertifiedWitness(&wit),
		WithVIPRTrace(solveTr),
	)
	if err != nil || res.Status != StatusOptimal {
		t.Fatalf("SolveCompiled failed: status=%v err=%v", res.Status, err)
	}
	if !solveTr.HasCertificate() || solveTr.Mode() != string(viprModeOptimal) {
		t.Fatalf("expected optimal VIPR cert, got hasCert=%v mode=%q", solveTr.HasCertificate(), solveTr.Mode())
	}
	if wit.Objective == nil || wit.Objective.Cmp(big.NewRat(3, 1)) != 0 {
		t.Fatalf("CertifiedWitness obj = %v, want 3", wit.Objective)
	}
	if exe, ok := tryFormalExe(t, "vipr-check"); ok {
		raw, jErr := solveTr.MarshalVIPRJSON()
		if jErr != nil {
			t.Fatalf("MarshalVIPRJSON failed: %v", jErr)
		}
		runJSONLItems(t, exe, [][]byte{raw})
	}
}

func TestVIPRFixedLeafMinimizeShiftAndNonSingletonInt(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Minimize variant with large ObjConstant triggering searchShift != 0.
	mMin := NewModel("dd47_min_shift", Minimize)
	x0 := mMin.AddIntVar("x0", 0, math.Inf(1))
	x1 := mMin.AddBinaryVar("x1")
	x2 := mMin.AddBinaryVar("x2")
	x7 := mMin.AddContVar("x7", math.Inf(-1), math.Inf(1))
	mMin.SetObjective(
		NewExpr(x0.T(-2), x1.T(1), x2.T(1), x7.T(-2)).AddConstant(800000),
	)
	mMin.AddConstraint("r0", Ge(NewExpr(x7.T(-5e-6), x1.T(-1), x2.T(-1)), -1))
	mMin.AddConstraint("r1", Le(NewExpr(x0.T(5e-6), x1.T(-1), x7.T(-5e-6)), 0))
	pMin := mMin.Compile()

	trMin := NewVIPRTrace()
	resMin, errMin := SolveCompiled(
		ctx, pMin,
		WithPureGo(),
		WithPresolve(false),
		WithCuts(false),
		WithCertified(true),
		WithVIPRTrace(trMin),
	)
	if errMin != nil || resMin.Status != StatusOptimal {
		t.Fatalf("dd47_min_shift failed: status=%v err=%v", resMin.Status, errMin)
	}
	if math.Abs(resMin.Objective-2.0) > 1e-6 || math.Abs(resMin.BestBound-2.0) > 1e-6 {
		t.Fatalf("dd47_min_shift obj=%.10g bound=%.10g, want 2.0", resMin.Objective, resMin.BestBound)
	}
	if !trMin.HasCertificate() || trMin.Mode() != string(viprModeOptimal) {
		t.Fatalf("dd47_min_shift hasCert=%v mode=%q, want optimal", trMin.HasCertificate(), trMin.Mode())
	}

	// Non-singleton fractional integer interval [200000.0, 200000.4] (< halfFracTarget) with sub-ULP infeasibility.
	mFrac := NewModel("non_singleton_subulp_infeas", Maximize)
	fx0 := mFrac.AddIntVar("x0", 200000.0, 200000.4)
	fx1 := mFrac.AddContVar("x1", math.Inf(-1), math.Inf(1))
	mFrac.SetObjective(NewExpr(fx0.T(2), fx1.T(2)))
	mFrac.AddConstraint("r0", Ge(NewExpr(fx1.T(-5e-6)), -1))
	mFrac.AddConstraint("r1", Le(NewExpr(fx0.T(5e-6), fx1.T(-5e-6)), 0))
	pFrac := mFrac.Compile()

	trFrac := NewVIPRTrace()
	resFrac, errFrac := SolveCompiled(
		ctx, pFrac,
		WithPureGo(),
		WithPresolve(false),
		WithCuts(false),
		WithHeuristics(false),
		WithCertified(true),
		WithVIPRTrace(trFrac),
	)
	if errFrac != nil || resFrac.Status != StatusInfeasible {
		t.Fatalf("non_singleton_subulp_infeas got status=%v err=%v, want StatusInfeasible", resFrac.Status, errFrac)
	}
	if !trFrac.HasCertificate() || trFrac.Mode() != string(viprModeInfeasible) {
		t.Fatalf("non_singleton_subulp_infeas hasCert=%v mode=%q, want infeasible", trFrac.HasCertificate(), trFrac.Mode())
	}

	if exe, ok := tryFormalExe(t, "vipr-check"); ok {
		rawMin, err1 := trMin.MarshalVIPRJSON()
		if err1 != nil {
			t.Fatalf("MarshalVIPRJSON(trMin) failed: %v", err1)
		}
		rawFrac, err2 := trFrac.MarshalVIPRJSON()
		if err2 != nil {
			t.Fatalf("MarshalVIPRJSON(trFrac) failed: %v", err2)
		}
		runJSONLItems(t, exe, [][]byte{rawMin, rawFrac})
	}
}
