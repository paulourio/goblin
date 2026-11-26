package goblin

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"math"
	"os"
	"testing"
	"time"
)

func parsePuLPJSONBytesForTest(t *testing.T, raw []byte) *Model {
	t.Helper()
	var doc testPuLPJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal PuLP JSON: %v", err)
	}
	sense := Minimize
	if doc.Parameters.Sense == -1 {
		sense = Maximize
	}
	m := NewModel(doc.Parameters.Name, sense)
	varsByName := make(map[string]Var, len(doc.Variables))
	for _, vd := range doc.Variables {
		lb := math.Inf(-1)
		if vd.LowBound != nil {
			lb = *vd.LowBound
		}
		ub := math.Inf(1)
		if vd.UpBound != nil {
			ub = *vd.UpBound
		}
		cat := Continuous
		switch vd.Cat {
		case "Binary":
			cat = Binary
		case "Integer":
			cat = Integer
		}
		varsByName[vd.Name] = m.AddVar(vd.Name, lb, ub, cat)
	}
	objTerms := make([]Term, 0, len(doc.Objective.Coefficients))
	for _, oc := range doc.Objective.Coefficients {
		v, ok := varsByName[oc.Name]
		if !ok {
			t.Fatalf("unknown objective variable %q", oc.Name)
		}
		objTerms = append(objTerms, T(v, oc.Value))
	}
	m.SetObjective(Expr{Terms: objTerms, Constant: doc.Objective.Constant})
	for _, con := range doc.Constraints {
		terms := make([]Term, 0, len(con.Coefficients))
		for _, cc := range con.Coefficients {
			v, ok := varsByName[cc.Name]
			if !ok {
				t.Fatalf("unknown constraint %q variable %q", con.Name, cc.Name)
			}
			terms = append(terms, T(v, cc.Value))
		}
		expr := Expr{Terms: terms}
		rhs := -con.Constant
		switch con.Sense {
		case -1:
			m.AddConstraint(con.Name, expr.Le(rhs))
		case 0:
			m.AddConstraint(con.Name, expr.Eq(rhs))
		case 1:
			m.AddConstraint(con.Name, expr.Ge(rhs))
		default:
			t.Fatalf("unsupported constraint sense %d", con.Sense)
		}
	}
	return m
}

func TestGMIRelaxCutDynamismTerms(t *testing.T) {
	// Absorb bounded tiny terms into RHS.
	m := NewModel("gmi_dyn_bounded", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddVar("x1", 0, 5, Continuous)
	m.SetObjective(NewExpr(x0.T(1), x1.T(1)))
	m.AddConstraint("c0", NewExpr(x0.T(1), x1.T(1)).Ge(0.5))
	p := m.Compile()

	idx := []int{0, 1}
	val := []float64{1.0, 2e-9}
	gotIdx, gotVal, gotRHS, ok := relaxCutDynamismTerms(p, idx, val, 0.5)
	if !ok {
		t.Fatalf("expected bounded tiny term relaxation to succeed")
	}
	if len(gotIdx) != 1 || gotIdx[0] != 0 || gotVal[0] != 1.0 {
		t.Fatalf("unexpected compacted terms: idx=%v val=%v", gotIdx, gotVal)
	}
	if gotRHS != 0.5 {
		t.Fatalf("positive bounded tiny coeff with lb=0 should keep RHS=0.5, got %.17g", gotRHS)
	}

	// Negative coefficients shift outward.
	idxNeg := []int{0, 1}
	valNeg := []float64{1.0, -2e-9}
	gotIdx, gotVal, gotRHS, ok = relaxCutDynamismTerms(p, idxNeg, valNeg, 0.5)
	if !ok || len(gotIdx) != 1 || gotIdx[0] != 0 {
		t.Fatalf("expected negative bounded term relaxation to keep col 0, got ok=%v idx=%v", ok, gotIdx)
	}
	if gotRHS <= 0.5+9e-9 || gotRHS > 0.5+2e-8 {
		t.Fatalf("expected relaxed RHS around 0.5+1e-8, got %.17g", gotRHS)
	}
	var ws cutGenWorkspace
	lpPrimal := []float64{0.9, 0.0}
	cut, finOK := finalizeSparseCut(p, gotIdx, gotVal, gotRHS, lpPrimal, 0, &ws)
	if !finOK || len(cut.ColIdx) != 1 || cut.ColIdx[0] != 0 {
		t.Fatalf("expected finalizeSparseCut to accept relaxed cut, got ok=%v idx=%v", finOK, cut.ColIdx)
	}

	// Reject terms exceeding maxCutDynamismRelaxTerm, leaving caller slices and RHS unchanged.
	idxTerm := []int{0, 1}
	valTerm := []float64{1.0, 5e-6}
	gotIdx, gotVal, gotRHS, ok = relaxCutDynamismTerms(p, idxTerm, valTerm, 0.5)
	if ok || len(gotIdx) != 2 || gotVal[1] != 5e-6 || gotRHS != 0.5 {
		t.Fatalf("expected term-ratio cap rejection without mutation, got ok=%v idx=%v val=%v rhs=%g", ok, gotIdx, gotVal, gotRHS)
	}

	// Wide bounds fail.
	mWide := NewModel("gmi_dyn_wide", Minimize)
	w0 := mWide.AddBinaryVar("w0")
	w1 := mWide.AddVar("w1", 0, 1e6, Continuous)
	mWide.SetObjective(NewExpr(w0.T(1), w1.T(1)))
	mWide.AddConstraint("c0", NewExpr(w0.T(1), w1.T(1)).Ge(0.5))
	pWide := mWide.Compile()

	wideIdx := []int{0, 1}
	wideVal := []float64{1.0, -2e-9}
	gotIdx, gotVal, gotRHS, ok = relaxCutDynamismTerms(pWide, wideIdx, wideVal, 0.5)
	if ok || len(gotIdx) != 2 || gotVal[1] != -2e-9 || gotRHS != 0.5 {
		t.Fatalf("expected wide-bound RHS weakening cap rejection without mutation, got ok=%v idx=%v val=%v rhs=%g", ok, gotIdx, gotVal, gotRHS)
	}

	// Unbounded tiny terms in the blocking sign cannot fold into finite bounds, so relaxCutDynamismTerms returns false without changing slices.
	mUnb := NewModel("gmi_dyn_unbounded", Minimize)
	u0 := mUnb.AddBinaryVar("u0")
	u1 := mUnb.AddVar("u1", 0, math.Inf(1), Continuous)
	mUnb.SetObjective(NewExpr(u0.T(1), u1.T(1)))
	mUnb.AddConstraint("c0", NewExpr(u0.T(1), u1.T(1)).Ge(0.5))
	pUnb := mUnb.Compile()

	origIdx := []int{0, 1}
	origVal := []float64{1.0, -2e-9}
	gotIdx, gotVal, gotRHS, ok = relaxCutDynamismTerms(pUnb, origIdx, origVal, 0.5)
	if ok {
		t.Fatalf("expected unbounded tiny term relaxation to fail")
	}
	if len(gotIdx) != 2 || gotIdx[0] != 0 || gotIdx[1] != 1 || gotVal[0] != 1.0 || gotVal[1] != -2e-9 || gotRHS != 0.5 {
		t.Fatalf("failed relaxation mutated slices/RHS: idx=%v val=%v rhs=%g", gotIdx, gotVal, gotRHS)
	}
}

func TestBinpack44AllTrialsGMIRootSolveRegression(t *testing.T) {
	raw, err := os.ReadFile("testdata/corpus/trials/t2_binpack_44.jsonl")
	if err != nil {
		t.Fatalf("read t2_binpack_44 trials: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 11 {
		t.Fatalf("expected 11 t2_binpack_44 trials, got %d", len(lines))
	}

	totNodes := 0
	totIters := 0
	limit := 15 * time.Second
	if isRaceBuild() {
		limit = 60 * time.Second
	}
	for i, line := range lines {
		m := parsePuLPJSONBytesForTest(t, line)
		p := m.Compile()
		res, err := SolveCompiled(context.Background(), p, WithWorkers(1), WithTimeLimit(limit))
		if err != nil {
			t.Fatalf("trial %d solve error: %v", i, err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("trial %d status=%v, want Optimal", i, res.Status)
		}
		wantObj := 30.319
		if i == 9 {
			wantObj = 40.619
		}
		if rel := math.Abs(res.Objective-wantObj) / wantObj; rel > 1e-5 {
			t.Fatalf("trial %d obj=%.6f, want %.6f (rel=%.2e)", i, res.Objective, wantObj, rel)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
			t.Fatalf("trial %d violation bound=%.2e row=%.2e int=%.2e", i, bV, rV, iV)
		}
		if i == 9 && (res.Nodes > 10 || res.RootCutBound < 40.6) {
			t.Fatalf("trial 9 regressed: nodes=%d rootCut=%.6f iters=%d", res.Nodes, res.RootCutBound, res.Iterations)
		}
		totNodes += res.Nodes
		totIters += res.Iterations
	}
	if totNodes > 150 || totIters > 3000 {
		t.Fatalf("t2_binpack_44 total search regressed: nodes=%d iters=%d", totNodes, totIters)
	}
}

func TestSpeculativeRootCutRollbackAndNonRegression(t *testing.T) {
	limit := 15 * time.Second
	if isRaceBuild() {
		limit = 60 * time.Second
	}

	// MKP skips speculative lookahead.
	rawMKP, err := os.ReadFile("testdata/corpus/trials/t2_mkp_30.jsonl")
	if err != nil {
		t.Fatalf("read t2_mkp_30 trials: %v", err)
	}
	mkpLines := bytes.Split(bytes.TrimSpace(rawMKP), []byte("\n"))
	pMKP := parsePuLPJSONBytesForTest(t, mkpLines[1]).Compile()
	resMKP, err := SolveCompiled(context.Background(), pMKP, WithWorkers(1), WithTimeLimit(limit))
	if err != nil {
		t.Fatalf("t2_mkp_30 trial 1 solve error: %v", err)
	}
	if resMKP.Status != StatusOptimal {
		t.Fatalf("t2_mkp_30 trial 1 status=%v, want Optimal", resMKP.Status)
	}
	if resMKP.CutsAdded > 17 || resMKP.Nodes > 27 || resMKP.Iterations > 900 {
		t.Fatalf("t2_mkp_30 trial 1 regressed: cuts=%d nodes=%d iters=%d", resMKP.CutsAdded, resMKP.Nodes, resMKP.Iterations)
	}

	// Lotsizing 504 stays fast.
	if !testing.Short() {
		rawLS, err := os.ReadFile("testdata/corpus/trials/t3_lotsizing_504.jsonl")
		if err != nil {
			t.Fatalf("read t3_lotsizing_504 trials: %v", err)
		}
		lsLines := bytes.Split(bytes.TrimSpace(rawLS), []byte("\n"))
		totLSNodes, totLSIters := 0, 0
		for i, line := range lsLines {
			pLS := parsePuLPJSONBytesForTest(t, line).Compile()
			resLS, err := SolveCompiled(context.Background(), pLS, WithWorkers(1), WithTimeLimit(limit))
			if err != nil {
				t.Fatalf("t3_lotsizing_504[%d] err: %v", i, err)
			}
			if resLS.Status != StatusOptimal {
				t.Fatalf("t3_lotsizing_504[%d] status=%v", i, resLS.Status)
			}
			totLSNodes += resLS.Nodes
			totLSIters += resLS.Iterations
		}
		if totLSNodes > 680 || totLSIters > 19800 {
			t.Fatalf("t3_lotsizing_504 regressed: nodes=%d iters=%d", totLSNodes, totLSIters)
		}
	}
}

func TestRescuedGMICutYieldAndCapplanSanity(t *testing.T) {
	// Rescued GMI cuts retain at least five cuts on t2_binpack_44 trial 0.
	rawBP, err := os.ReadFile("testdata/corpus/trials/t2_binpack_44.jsonl")
	if err != nil {
		t.Fatalf("read t2_binpack_44 trials: %v", err)
	}
	bpLines := bytes.Split(bytes.TrimSpace(rawBP), []byte("\n"))
	pBP := parsePuLPJSONBytesForTest(t, bpLines[0]).Compile()
	rpBP := Presolve(pBP, defaultPresolveMaxPass).Reduced
	if rpBP == nil {
		rpBP = pBP
	}
	lpBP := NewLPSolver(rpBP, nil)
	defer lpBP.Close()
	rBP := lpBP.Solve()
	if rBP.Status != StatusOptimal {
		t.Fatalf("binpack trial 0 root LP status=%v, want Optimal", rBP.Status)
	}
	var wsBP cutGenWorkspace
	gmiBP := separateGMICutsEx(lpBP, rBP.Primal, &wsBP, false)
	if len(gmiBP) < 5 {
		t.Fatalf("expected rescued GMI separator to keep >=5 cuts on binpack trial 0, got %d", len(gmiBP))
	}

	// Capplan1 root LP and GMI cuts stay well-formed when bounded RHS shifts limit how far weak rows loosen.
	m := loadJSONModelForTest(t, "testdata/public/coinor_capplan1.json")
	p := m.Compile()
	pre := Presolve(p, defaultPresolveMaxPass)
	rp := pre.Reduced
	if rp == nil {
		rp = p
	}
	lp := NewLPSolver(rp, nil)
	defer lp.Close()
	r := lp.Solve()
	if r.Status != StatusOptimal {
		t.Fatalf("capplan1 root LP status=%v, want Optimal", r.Status)
	}
	var ws cutGenWorkspace
	gmi := separateGMICutsEx(lp, r.Primal, &ws, false)
	if len(gmi) < 3 {
		t.Fatalf("expected >=3 valid GMI cuts at capplan1 root, got %d", len(gmi))
	}
	for idx, c := range gmi {
		if len(c.ColIdx) == 0 || len(c.ColIdx) != len(c.Val) || math.IsNaN(c.RHS) || math.IsInf(c.RHS, 0) {
			t.Fatalf("invalid cut %d returned by separateGMICutsEx", idx)
		}
	}
}
