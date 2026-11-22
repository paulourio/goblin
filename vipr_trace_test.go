package goblin

import (
	"bytes"
	"encoding/json/v2"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func runVIPRCheckBytes(tb testing.TB, exe, name string, raw []byte) (string, error) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), name+".json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		tb.Fatalf("write vipr cert %s: %v", name, err)
	}
	cmd := exec.CommandContext(tb.Context(), exe, path)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func requireVIPRBytesVerified(tb testing.TB, exe, name string, raw []byte) {
	tb.Helper()
	out, err := runVIPRCheckBytes(tb, exe, name, raw)
	if err != nil || strings.TrimSpace(out) != "VERIFIED" {
		tb.Fatalf("%s failed vipr-check: err=%v out=%q", name, err, out)
	}
}

func runJSONLItems(tb testing.TB, exe string, items [][]byte) {
	tb.Helper()
	if len(items) == 0 {
		tb.Fatalf("runJSONLItems(%s): no items to verify", exe)
	}
	var stdinBuf bytes.Buffer
	for _, it := range items {
		stdinBuf.Write(it)
		stdinBuf.WriteByte('\n')
	}
	cmd := exec.CommandContext(tb.Context(), exe, "--jsonl")
	cmd.Stdin = &stdinBuf
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("%s --jsonl failed: %v\noutput: %s", exe, err, string(out))
	}
	type jsonlResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(items) {
		tb.Fatalf("%s --jsonl returned %d lines, want %d\nraw: %s", exe, len(lines), len(items), string(out))
	}
	for i, line := range lines {
		var resp jsonlResp
		if uErr := json.Unmarshal([]byte(line), &resp); uErr != nil {
			tb.Fatalf("%s --jsonl line %d invalid JSON %q: %v", exe, i, line, uErr)
		}
		if !resp.OK {
			tb.Fatalf("%s --jsonl item %d rejected: %s\nreq=%s", exe, i, resp.Error, string(items[i]))
		}
	}
}

func decodeVIPRJSONForTest(tb testing.TB, raw []byte) viprCertJSON {
	tb.Helper()
	var cert viprCertJSON
	if err := json.Unmarshal(raw, &cert); err != nil {
		tb.Fatalf("unmarshal VIPR JSON: %v", err)
	}
	if len(cert.Steps) == 0 {
		tb.Fatalf("expected non-empty VIPR steps")
	}
	if cert.RootStep < 0 || cert.RootStep >= len(cert.Steps) {
		tb.Fatalf("rootStep %d out of range [0, %d)", cert.RootStep, len(cert.Steps))
	}
	return cert
}

func buildTwoSideInequalityParityMIP() *Model {
	// Separate <= and >= rows bypass single-row Equal GCD checks and force B&B splits.
	m := NewModel("two_side_parity_inf", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddBinaryVar("x1")
	m.SetObjective(NewExpr(x0.T(1), x1.T(2)))
	m.AddConstraint("le_row", Le(NewExpr(x0.T(2), x1.T(2)), 3))
	m.AddConstraint("ge_row", Ge(NewExpr(x0.T(2), x1.T(2)), 3))
	return m
}

func buildSymmetricBinaryMIP() *Model {
	// Symmetric coloring/partitioning MIP that triggers node orbit symmetry when tracing is off.
	m := NewModel("sym_orbit_vipr", Minimize)
	const g = 3
	y := make([]Var, g)
	x := make([][]Var, 2)
	for k := range g {
		y[k] = m.AddBinaryVar("y_" + strconv.Itoa(k))
	}
	for i := range 2 {
		x[i] = make([]Var, g)
		for k := range g {
			x[i][k] = m.AddBinaryVar("x_" + strconv.Itoa(i) + "_" + strconv.Itoa(k))
		}
		m.AddConstraint("part_"+strconv.Itoa(i), Eq(NewExpr(x[i][0].T(1), x[i][1].T(1), x[i][2].T(1)), 1))
	}
	for k := range g {
		m.AddConstraint("cap_"+strconv.Itoa(k), Le(NewExpr(x[0][k].T(1), x[1][k].T(1), y[k].T(-1)), 0))
	}
	m.SetObjective(NewExpr(y[0].T(3), y[1].T(3), y[2].T(3)))
	return m
}

func buildBoundLimitKnapsackMIP() *Model {
	// Three-row 0-1 knapsack requiring >2 B&B nodes so MaxNodes=2 hits StatusNodeLimit.
	m := NewModel("bound_limit_mkp", Maximize)
	w0 := []float64{11, 13, 17, 19, 23, 29, 31, 37, 41, 43}
	w1 := []float64{29, 23, 19, 17, 13, 11, 37, 31, 43, 41}
	w2 := []float64{17, 31, 11, 43, 29, 19, 23, 41, 13, 37}
	values := []float64{25, 27, 22, 33, 29, 26, 38, 45, 41, 49}
	xs := make([]Var, len(values))
	objTerms := make([]Term, len(values))
	r0Terms := make([]Term, len(values))
	r1Terms := make([]Term, len(values))
	r2Terms := make([]Term, len(values))
	for i := range values {
		xs[i] = m.AddBinaryVar("k_" + strconv.Itoa(i))
		objTerms[i] = xs[i].T(values[i])
		r0Terms[i] = xs[i].T(w0[i])
		r1Terms[i] = xs[i].T(w1[i])
		r2Terms[i] = xs[i].T(w2[i])
	}
	m.SetObjective(NewExpr(objTerms...).AddConstant(5.0))
	m.AddConstraint("cap0", Le(NewExpr(r0Terms...), 115))
	m.AddConstraint("cap1", Le(NewExpr(r1Terms...), 115))
	m.AddConstraint("cap2", Le(NewExpr(r2Terms...), 115))
	return m
}

func TestVIPRTraceNilAndParity(t *testing.T) {
	t.Parallel()

	var nilTrace *VIPRTrace
	if nilTrace.HasCertificate() || nilTrace.PresolveInfeasible() || nilTrace.TracedProblem() != nil || nilTrace.Mode() != "" || nilTrace.TargetLB() != nil || nilTrace.StepCount() != 0 {
		t.Fatal("nil VIPRTrace returned non-zero state")
	}
	if _, err := nilTrace.MarshalVIPRJSON(); err == nil {
		t.Fatal("expected error from nil VIPRTrace.MarshalVIPRJSON")
	}

	emptyTrace := NewVIPRTrace()
	if emptyTrace.HasCertificate() || emptyTrace.StepCount() != 0 {
		t.Fatal("empty VIPRTrace unexpectedly reports certificate")
	}
	if _, err := emptyTrace.MarshalVIPRJSON(); err == nil {
		t.Fatal("expected error from empty VIPRTrace.MarshalVIPRJSON")
	}

	p := buildMultiNodeBranchMIP1().Compile()
	resBase, err := SolveCompiled(t.Context(), p, WithPureGo(), WithWorkers(1))
	if err != nil {
		t.Fatalf("baseline solve failed: %v", err)
	}
	resNilOpt, err := SolveCompiled(t.Context(), p, WithPureGo(), WithWorkers(1), WithVIPRTrace(nil))
	if err != nil {
		t.Fatalf("nil-trace solve failed: %v", err)
	}
	if resNilOpt.Status != resBase.Status || math.Abs(resNilOpt.Objective-resBase.Objective) > 1e-9 || math.Abs(resNilOpt.BestBound-resBase.BestBound) > 1e-9 {
		t.Fatalf("WithVIPRTrace(nil) diverged: base=(%v, %g, %g) nil=(%v, %g, %g)",
			resBase.Status, resBase.Objective, resBase.BestBound,
			resNilOpt.Status, resNilOpt.Objective, resNilOpt.BestBound)
	}

	tr := NewVIPRTrace()
	resTraced, err := SolveCompiled(t.Context(), p, WithPureGo(), WithWorkers(4), WithVIPRTrace(tr))
	if err != nil {
		t.Fatalf("traced solve failed: %v", err)
	}
	if resTraced.Status != resBase.Status || math.Abs(resTraced.Objective-resBase.Objective) > 1e-7 {
		t.Fatalf("traced solve diverged: base=(%v, %g) traced=(%v, %g)",
			resBase.Status, resBase.Objective, resTraced.Status, resTraced.Objective)
	}
	if !tr.HasCertificate() || tr.TracedProblem() == nil {
		t.Fatalf("expected valid VIPR certificate on %s", p.Name())
	}
}

func TestVIPRTraceGCDInfeasible(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	m := NewModel("gcd_inf_trace", Minimize)
	x0 := m.AddIntVar("x0", -10, 10)
	x1 := m.AddIntVar("x1", -10, 10)
	m.SetObjective(NewExpr(x0.T(1), x1.T(1)))
	m.AddConstraint("eq", Eq(NewExpr(x0.T(6), x1.T(10)), 5))
	p := m.Compile()

	for _, presolve := range []bool{false, true} {
		name := "presolve_" + strconv.FormatBool(presolve)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tr := NewVIPRTrace()
			res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(presolve), WithVIPRTrace(tr))
			if err != nil {
				t.Fatalf("solve error: %v", err)
			}
			if res.Status != StatusInfeasible {
				t.Fatalf("status=%v, want StatusInfeasible", res.Status)
			}
			if presolve && !tr.PresolveInfeasible() {
				t.Fatal("expected PresolveInfeasible() == true when presolve detects GCD contradiction")
			}
			if !tr.HasCertificate() || tr.Mode() != "infeasible" {
				t.Fatalf("expected infeasible VIPR certificate, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
			}
			raw, err := tr.MarshalVIPRJSON()
			if err != nil {
				t.Fatalf("MarshalVIPRJSON: %v", err)
			}
			cert := decodeVIPRJSONForTest(t, raw)
			if cert.Mode != "infeasible" {
				t.Fatalf("cert.Mode=%q, want infeasible", cert.Mode)
			}
			if formalOn {
				requireVIPRBytesVerified(t, viprExe, "gcd_"+name, raw)
			}
		})
	}
}

func TestVIPRTraceLPAndMultiNodeInfeasible(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	mLPInf := NewModel("lp_inf_trace", Minimize)
	a := mLPInf.AddIntVar("a", 0, 10)
	b := mLPInf.AddContVar("b", 0, 10)
	mLPInf.SetObjective(NewExpr(a.T(1), b.T(2)))
	mLPInf.AddConstraint("c1", Le(NewExpr(a.T(1), b.T(1)), 1))
	mLPInf.AddConstraint("c2", Ge(NewExpr(a.T(1), b.T(1)), 3))

	cases := []struct {
		name       string
		model      *Model
		presolve   bool
		wantBranch bool
	}{
		{name: "root_lp_infeasible", model: mLPInf, presolve: false, wantBranch: false},
		{name: "two_side_parity_no_presolve", model: buildTwoSideInequalityParityMIP(), presolve: false, wantBranch: true},
		{name: "two_side_parity_with_presolve", model: buildTwoSideInequalityParityMIP(), presolve: true, wantBranch: false},
		{name: "parity_eq_no_presolve", model: buildMultiNodeInfeasibleMIP(), presolve: false, wantBranch: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := tc.model.Compile()
			tr := NewVIPRTrace()
			res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(tc.presolve), WithVIPRTrace(tr))
			if err != nil {
				t.Fatalf("solve error: %v", err)
			}
			if res.Status != StatusInfeasible {
				t.Fatalf("status=%v, want StatusInfeasible", res.Status)
			}
			if !tr.HasCertificate() || tr.Mode() != "infeasible" {
				t.Fatalf("expected infeasible VIPR certificate, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
			}
			raw, err := tr.MarshalVIPRJSON()
			if err != nil {
				t.Fatalf("MarshalVIPRJSON: %v", err)
			}
			cert := decodeVIPRJSONForTest(t, raw)
			if tc.wantBranch {
				hasBranch := false
				for _, st := range cert.Steps {
					if st.Rule == "branch" {
						hasBranch = true
						break
					}
				}
				if !hasBranch {
					t.Fatalf("%s expected branch step in VIPR proof, got %d steps", tc.name, len(cert.Steps))
				}
			}
			if formalOn {
				requireVIPRBytesVerified(t, viprExe, tc.name, raw)
			}
		})
	}
}

func TestVIPRTraceRootAndMultiNodeOptimal(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	mLPMax := NewModel("std_lp_max_const", Maximize)
	mx := mLPMax.AddContVar("x", 0, 4)
	my := mLPMax.AddContVar("y", -1, 1)
	mz := mLPMax.AddContVar("z", 0, 100)
	mLPMax.SetObjective(NewExpr(mx.T(1), my.T(4), mz.T(9)).AddConstant(3.5))
	mLPMax.AddConstraint("c1", Le(NewExpr(mx.T(1), my.T(1)), 5))
	mLPMax.AddConstraint("c2", Ge(NewExpr(mx.T(1), mz.T(1)), 10))
	mLPMax.AddConstraint("c3", Eq(NewExpr(my.T(-1), mz.T(1)), 7))

	cases := []struct {
		name     string
		relPath  string
		build    func() *Model
		presolve bool
		exactGap bool
	}{
		{name: "pure_lp_min", build: func() *Model { return buildStdLPModel("std_lp_min", Minimize) }, presolve: false, exactGap: true},
		{name: "pure_lp_max_const", build: func() *Model { return mLPMax }, presolve: false, exactGap: true},
		{name: "t1_furniture", relPath: filepath.Join("testdata", "corpus", "t1_furniture.json"), presolve: true, exactGap: true},
		{name: "t1_std_mip_no_presolve", relPath: filepath.Join("testdata", "corpus", "t1_std_mip.json"), presolve: false, exactGap: true},
		{name: "t1_std_mip_presolve", relPath: filepath.Join("testdata", "corpus", "t1_std_mip.json"), presolve: true, exactGap: true},
		{name: "t1_knapsack_12", relPath: filepath.Join("testdata", "corpus", "t1_knapsack_12.json"), presolve: false, exactGap: true},
		{name: "multinode_mixed_3v", build: buildMultiNodeBranchMIP1, presolve: false, exactGap: true},
		{name: "multinode_mixed_3v_presolve", build: buildMultiNodeBranchMIP1, presolve: true, exactGap: true},
		{name: "multinode_double_cg_bin", build: buildMultiNodeBranchMIP2, presolve: false, exactGap: true},
		{name: "symmetric_binary_mip", build: buildSymmetricBinaryMIP, presolve: false, exactGap: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var mdl *Model
			if tc.relPath != "" {
				mdl = loadJSONModelForTest(t, tc.relPath)
			} else {
				mdl = tc.build()
			}
			p := mdl.Compile()
			tr := NewVIPRTrace()
			opts := []Option{WithPureGo(), WithPresolve(tc.presolve), WithVIPRTrace(tr)}
			if tc.exactGap {
				opts = append(opts, WithRelGapTol(0), WithAbsGapTol(0))
			}
			res, err := SolveCompiled(t.Context(), p, opts...)
			if err != nil {
				t.Fatalf("solve error: %v", err)
			}
			if res.Status != StatusOptimal {
				t.Fatalf("status=%v, want StatusOptimal", res.Status)
			}
			if !tr.HasCertificate() {
				t.Fatalf("expected VIPR certificate for %s", tc.name)
			}
			if tc.exactGap && tr.Mode() != "optimal" {
				lbF, _ := tr.TargetLB().Float64()
				t.Fatalf("%s mode=%q, want optimal (obj=%.17g bestBound=%.17g targetLB=%s [%.17g] nodes=%d)",
					tc.name, tr.Mode(), res.Objective, res.BestBound, tr.TargetLB().RatString(), lbF, res.Nodes)
			}
			if tr.TargetLB() == nil {
				t.Fatalf("%s expected non-nil TargetLB", tc.name)
			}
			raw, err := tr.MarshalVIPRJSON()
			if err != nil {
				t.Fatalf("MarshalVIPRJSON: %v", err)
			}
			_ = decodeVIPRJSONForTest(t, raw)
			if formalOn {
				requireVIPRBytesVerified(t, viprExe, tc.name, raw)
			}
		})
	}
}

func TestVIPRTraceEarlyNodeLimitBound(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	p := buildBoundLimitKnapsackMIP().Compile()
	tr := NewVIPRTrace()
	res, err := SolveCompiled(
		t.Context(),
		p,
		WithPureGo(),
		WithPresolve(false),
		WithHeuristics(false),
		WithMaxNodes(2),
		WithWorkers(4),
		WithVIPRTrace(tr),
	)
	if err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if res.Status != StatusNodeLimit {
		t.Fatalf("status=%v, want StatusNodeLimit", res.Status)
	}
	if !tr.HasCertificate() {
		t.Fatal("expected VIPR bound certificate on StatusNodeLimit")
	}
	if tr.Mode() != "bound" {
		t.Fatalf("mode=%q, want bound", tr.Mode())
	}
	lbRat := tr.TargetLB()
	if lbRat == nil {
		t.Fatal("expected non-nil TargetLB in bound mode")
	}
	raw, err := tr.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("MarshalVIPRJSON: %v", err)
	}
	cert := decodeVIPRJSONForTest(t, raw)
	if cert.Mode != "bound" || cert.TargetLB == nil {
		t.Fatalf("expected bound mode with targetLB, got mode=%q targetLB=%v", cert.Mode, cert.TargetLB)
	}

	// Match shifted minimized targetLB against maximized Result.BestBound.
	lbFloat, _ := lbRat.Float64()
	userBound := -lbFloat
	if math.Abs(userBound-res.BestBound) > 1e-4 {
		t.Fatalf("VIPR targetLB user bound %.8g differs from res.BestBound %.8g", userBound, res.BestBound)
	}

	if formalOn {
		requireVIPRBytesVerified(t, viprExe, "early_node_limit_bound", raw)

		// Negative control: vipr-check must reject targetLB shifted upward by +1.
		badLB := new(big.Rat).Add(lbRat, big.NewRat(1, 1))
		badPay := viprBigRatToPayload(badLB)
		cert.TargetLB = &badPay
		badRaw, mErr := json.Marshal(cert)
		if mErr != nil {
			t.Fatalf("marshal negative bound cert: %v", mErr)
		}
		out, runErr := runVIPRCheckBytes(t, viprExe, "early_node_limit_bad_lb", badRaw)
		if runErr == nil || strings.TrimSpace(out) == "VERIFIED" {
			t.Fatalf("negative bound cert unexpectedly passed vipr-check: err=%v out=%q", runErr, out)
		}
	}
}

func TestFormalExportHelpers(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"

	t.Run("LPCheckPayloads", func(t *testing.T) {
		t.Parallel()
		var lpItems [][]byte

		for _, mdl := range []*Model{
			buildStdLPModel("exp_lp_min", Minimize),
			buildStdLPModel("exp_lp_max", Maximize),
		} {
			p := mdl.Compile()
			lp := NewLPSolver(p, nil)
			res := lp.Solve()
			mode, payload, ok := ExportLPCheckPayload(p, lp, res)
			lp.Close()
			if !ok || mode != "safe-bound" || len(payload) == 0 {
				t.Fatalf("%s ExportLPCheckPayload failed: mode=%q ok=%v len=%d", mdl.Name(), mode, ok, len(payload))
			}
			lpItems = append(lpItems, payload)
		}

		mInf := NewModel("exp_lp_inf", Minimize)
		x := mInf.AddContVar("x", 0, 5)
		y := mInf.AddContVar("y", 0, 5)
		mInf.SetObjective(NewExpr(x.T(1), y.T(1)))
		mInf.AddConstraint("c1", Le(NewExpr(x.T(1), y.T(1)), 2))
		mInf.AddConstraint("c2", Ge(NewExpr(x.T(1), y.T(1)), 4))
		pInf := mInf.Compile()
		lpInf := NewLPSolver(pInf, nil)
		resInf := lpInf.Solve()
		mode, payload, ok := ExportLPCheckPayload(pInf, lpInf, resInf)
		lpInf.Close()
		if !ok || mode != "farkas" || len(payload) == 0 {
			t.Fatalf("infeasible ExportLPCheckPayload failed: mode=%q ok=%v len=%d", mode, ok, len(payload))
		}
		lpItems = append(lpItems, payload)

		if formalOn {
			runJSONLItems(t, formalExe(t, "lp-check"), lpItems)
		}
	})

	t.Run("PresolveRulesItems", func(t *testing.T) {
		t.Parallel()
		var ruleItems [][]byte

		// GCD + activity model.
		mGCD := NewModel("exp_gcd", Minimize)
		g0 := mGCD.AddIntVar("g0", 0, 10)
		g1 := mGCD.AddIntVar("g1", 0, 10)
		mGCD.AddConstraint("eq", Eq(NewExpr(g0.T(6), g1.T(10)), 5))
		mGCD.AddConstraint("ub", Le(NewExpr(g0.T(2), g1.T(3)), 7))
		itemsGCD, preGCD := ExportPresolveRulesItems(mGCD.Compile())
		if !preGCD.Infeasible || len(itemsGCD) < 2 {
			t.Fatalf("expected infeasible presolve and >=2 rule items, got infeas=%v len=%d", preGCD.Infeasible, len(itemsGCD))
		}
		ruleItems = append(ruleItems, itemsGCD...)

		// Savelsbergh binary strengthening model: 5*b0 + 2*b1 + 2*b2 <= 6 (maxAct=9, surplus=3 < 5).
		mBin := NewModel("exp_bin", Minimize)
		b0 := mBin.AddBinaryVar("b0")
		b1 := mBin.AddBinaryVar("b1")
		b2 := mBin.AddBinaryVar("b2")
		mBin.SetObjective(NewExpr(b0.T(1), b1.T(1), b2.T(1)))
		mBin.AddConstraint("c0", Le(NewExpr(b0.T(5), b1.T(2), b2.T(2)), 6))
		itemsBin, preBin := ExportPresolveRulesItems(mBin.Compile())
		if preBin.StrengthenedRows == 0 || len(itemsBin) < 2 {
			t.Fatalf("expected strengthened binary row and >=2 rule items, got strengthened=%d len=%d", preBin.StrengthenedRows, len(itemsBin))
		}
		ruleItems = append(ruleItems, itemsBin...)

		// Symmetric orbit model.
		itemsOrb, _ := ExportPresolveRulesItems(buildSymmetricBinaryMIP().Compile())
		if len(itemsOrb) == 0 {
			t.Fatal("expected non-empty rule items on symmetric model")
		}
		ruleItems = append(ruleItems, itemsOrb...)

		seenOps := make(map[string]bool)
		for _, raw := range ruleItems {
			var header struct {
				Subcommand string `json:"subcommand"`
				Op         string `json:"op"`
			}
			if err := json.Unmarshal(raw, &header); err != nil {
				t.Fatalf("invalid rule item JSON: %v", err)
			}
			seenOps[header.Subcommand+":"+header.Op] = true
		}
		for _, want := range []string{"gcd-infeas:gcd-infeas", "activity:summary", "bin-strengthen:bin-strengthen", "orbit-check:orbit-check"} {
			if !seenOps[want] {
				t.Fatalf("missing expected rule item %q in %v", want, seenOps)
			}
		}

		if formalOn {
			runJSONLItems(t, formalExe(t, "rules-check"), ruleItems)
		}
	})

	t.Run("SeparatedCutsItems", func(t *testing.T) {
		t.Parallel()
		var cutItems [][]byte
		for _, relPath := range []string{
			filepath.Join("testdata", "corpus", "t1_std_mip.json"),
			filepath.Join("testdata", "corpus", "t1_knapsack_12.json"),
			filepath.Join("testdata", "corpus", "t2_cflp_36.json"),
		} {
			p := loadLocalPuLPJSONModel(t, relPath)
			res, err := SolveCompiled(t.Context(), p)
			if err != nil || res.Status != StatusOptimal {
				t.Fatalf("solve %s: status=%v err=%v", relPath, res.Status, err)
			}
			sol := snapExactMIPSolution(t, p, res.Values)
			items, err := ExportSeparatedCutsItems(p, [][]float64{sol})
			if err != nil {
				t.Fatalf("ExportSeparatedCutsItems(%s): %v", relPath, err)
			}
			cutItems = append(cutItems, items...)
		}
		if len(cutItems) == 0 {
			t.Fatal("expected separated cut items across corpus MIPs")
		}
		if formalOn {
			runJSONLItems(t, formalExe(t, "cuts-check"), cutItems)
		}
	})
}

func TestVIPRTraceEdgeCasesAndRandomizedSweep(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"

	var batchCerts [][]byte

	// Unbounded LP with empty integer hull exercises solveUnboundedMIPFeasibility.
	mUnbInf := NewModel("unbounded_lp_infeas_mip", Minimize)
	xc := mUnbInf.AddContVar("xc", 0, math.Inf(1))
	y0 := mUnbInf.AddBinaryVar("y0")
	y1 := mUnbInf.AddBinaryVar("y1")
	mUnbInf.SetObjective(NewExpr(xc.T(-1), y0.T(1), y1.T(1)))
	mUnbInf.AddConstraint("le_par", Le(NewExpr(y0.T(2), y1.T(2)), 3))
	mUnbInf.AddConstraint("ge_par", Ge(NewExpr(y0.T(2), y1.T(2)), 3))
	trUnb := NewVIPRTrace()
	resUnb, err := SolveCompiled(t.Context(), mUnbInf.Compile(), WithPureGo(), WithPresolve(false), WithVIPRTrace(trUnb))
	if err != nil || resUnb.Status != StatusInfeasible {
		t.Fatalf("unbounded-LP infeasible MIP: status=%v err=%v", resUnb.Status, err)
	}
	if !trUnb.HasCertificate() || trUnb.Mode() != "infeasible" {
		t.Fatalf("expected infeasible VIPR cert on unbounded-LP infeasible MIP, got hasCert=%v mode=%q", trUnb.HasCertificate(), trUnb.Mode())
	}
	rawUnb, err := trUnb.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("marshal unbounded-LP infeasible VIPR: %v", err)
	}
	batchCerts = append(batchCerts, rawUnb)

	// Fractional initial bounds on integer columns with and without presolve.
	for _, presolve := range []bool{false, true} {
		mFrac := NewModel("frac_int_bounds", Minimize)
		z0 := mFrac.AddIntVar("z0", 0.4, 2.6)
		z1 := mFrac.AddIntVar("z1", 0.6, 0.8)
		mFrac.SetObjective(NewExpr(z0.T(2), z1.T(3)))
		mFrac.AddConstraint("c0", Ge(NewExpr(z0.T(1), z1.T(1)), 1))
		trFrac := NewVIPRTrace()
		resFrac, errF := SolveCompiled(t.Context(), mFrac.Compile(), WithPureGo(), WithPresolve(presolve), WithVIPRTrace(trFrac))
		if errF != nil || resFrac.Status != StatusInfeasible {
			t.Fatalf("frac bounds presolve=%v: status=%v err=%v", presolve, resFrac.Status, errF)
		}
		if !trFrac.HasCertificate() || trFrac.Mode() != "infeasible" {
			t.Fatalf("frac bounds presolve=%v expected infeasible cert, got hasCert=%v mode=%q", presolve, trFrac.HasCertificate(), trFrac.Mode())
		}
		rawF, mErr := trFrac.MarshalVIPRJSON()
		if mErr != nil {
			t.Fatalf("marshal frac bounds cert: %v", mErr)
		}
		batchCerts = append(batchCerts, rawF)
	}

	// Deterministic 24-model mixed-integer sweep across StatusOptimal, StatusInfeasible, and StatusNodeLimit.
	rng := rand.New(rand.NewPCG(0x51a7, 0x9e3779b9))
	for idx := range 24 {
		sense := Minimize
		if idx%2 == 1 {
			sense = Maximize
		}
		m := NewModel("rand_vipr_"+strconv.Itoa(idx), sense)
		nInt := 3 + rng.IntN(3)
		nCont := rng.IntN(2)
		nVars := nInt + nCont
		vars := make([]Var, nVars)
		objTerms := make([]Term, 0, nVars)
		for j := range nVars {
			if j < nInt {
				if rng.IntN(2) == 0 {
					vars[j] = m.AddBinaryVar("b_" + strconv.Itoa(j))
				} else {
					vars[j] = m.AddIntVar("i_"+strconv.Itoa(j), 0, float64(2+rng.IntN(3)))
				}
			} else {
				vars[j] = m.AddContVar("c_"+strconv.Itoa(j), 0, 5)
			}
			c := float64(rng.IntN(11)-5) * 0.5
			if c != 0 {
				objTerms = append(objTerms, vars[j].T(c))
			}
		}
		m.SetObjective(NewExpr(objTerms...).AddConstant(float64(idx%3) * 1.25))
		nRows := 2 + rng.IntN(3)
		for r := range nRows {
			terms := make([]Term, 0, nVars)
			for j := range nVars {
				a := float64(rng.IntN(7) - 3)
				if a != 0 {
					terms = append(terms, vars[j].T(a))
				}
			}
			if len(terms) == 0 {
				terms = append(terms, vars[0].T(1))
			}
			rhs := float64(rng.IntN(9)-2) * 0.5
			switch r % 3 {
			case 0:
				m.AddConstraint("r_"+strconv.Itoa(r), Le(NewExpr(terms...), rhs))
			case 1:
				m.AddConstraint("r_"+strconv.Itoa(r), Ge(NewExpr(terms...), rhs))
			default:
				m.AddConstraint("r_"+strconv.Itoa(r), Le(NewExpr(terms...), rhs+2.0))
			}
		}

		p := m.Compile()
		tr := NewVIPRTrace()
		opts := []Option{
			WithPureGo(),
			WithPresolve(idx%3 != 0),
			WithVIPRTrace(tr),
		}
		switch idx % 4 {
		case 0:
			opts = append(opts, WithMaxNodes(3))
		case 1:
			opts = append(opts, WithRelGapTol(0.05), WithAbsGapTol(0.1))
		default:
			opts = append(opts, WithRelGapTol(0), WithAbsGapTol(0))
		}
		res, sErr := SolveCompiled(t.Context(), p, opts...)
		if sErr != nil {
			t.Fatalf("case %d solve error: %v", idx, sErr)
		}
		if res.Status == StatusOptimal || res.Status == StatusInfeasible || res.Status == StatusNodeLimit {
			if !tr.HasCertificate() {
				if res.Status != StatusInfeasible || !tr.PresolveInfeasible() {
					t.Fatalf("case %d (status=%v) missing VIPR certificate", idx, res.Status)
				}
				continue
			}
			raw, mErr := tr.MarshalVIPRJSON()
			if mErr != nil {
				t.Fatalf("case %d MarshalVIPRJSON: %v", idx, mErr)
			}
			_ = decodeVIPRJSONForTest(t, raw)
			batchCerts = append(batchCerts, raw)
		}
	}

	if len(batchCerts) < 20 {
		t.Fatalf("expected >= 20 VIPR certificates in randomized sweep, got %d", len(batchCerts))
	}
	if formalOn {
		runJSONLItems(t, formalExe(t, "vipr-check"), batchCerts)
	}
}

func TestVIPRTraceRootCutsVUBAndCover(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	t.Run("cg_root_optimality", func(t *testing.T) {
		t.Parallel()
		// Maximize x0 + x1 subject to 2*x0 + 2*x1 <= 3 over binaries:
		// root LP = 1.5, CG cut x0 + x1 <= 1 closes optimality at the root without branching.
		m := NewModel("cg_root_opt", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		m.SetObjective(NewExpr(x0.T(1), x1.T(1)))
		m.AddConstraint("cap", Le(NewExpr(x0.T(2), x1.T(2)), 3))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(false), WithCuts(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("solve failed: status=%v err=%v", res.Status, err)
		}
		if math.Abs(res.Objective-1.0) > 1e-7 {
			t.Fatalf("objective=%g, want 1", res.Objective)
		}
		if tr.rootCutsCertified == 0 || res.CutsAdded == 0 {
			t.Fatalf("expected certified root CG cuts, got rootCutsCertified=%d CutsAdded=%d", tr.rootCutsCertified, res.CutsAdded)
		}
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		cert := decodeVIPRJSONForTest(t, raw)
		hasRoundDown := false
		for _, st := range cert.Steps {
			if st.Rule == "roundDown" {
				hasRoundDown = true
				break
			}
		}
		if !hasRoundDown {
			t.Fatalf("expected roundDown CG cut step in root certificate, got %d steps", len(cert.Steps))
		}
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "cg_root_opt", raw)
		}
	})

	t.Run("vub_disagg_both_cases", func(t *testing.T) {
		t.Parallel()
		// Capacity row x0 + 3*x1 - 10*y <= 0 with u0 = 4 (< C/a0 = 10, Case 2 branch proof)
		// and u1 = 10 (>= C/a1 = 10/3, Case 1 lincomb proof).
		m := NewModel("vub_disagg_cases", Maximize)
		y := m.AddBinaryVar("y")
		x0 := m.AddContVar("x0", 0, 4)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(NewExpr(x0.T(5), x1.T(4), y.T(-12)))
		m.AddConstraint("vub", Le(NewExpr(x0.T(1), x1.T(3), y.T(-10)), 0))
		m.AddConstraint("dem", Le(NewExpr(x0.T(1), x1.T(1)), 5))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(false), WithCuts(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("solve failed: status=%v err=%v", res.Status, err)
		}
		if tr.rootCutsCertified == 0 {
			t.Fatalf("expected certified VUB cuts at root, got %d", tr.rootCutsCertified)
		}
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "vub_disagg_cases", raw)
		}
	})

	t.Run("knapsack_cover_root_cuts", func(t *testing.T) {
		t.Parallel()
		// 6-item 0-1 knapsack with fractional LP optimum violated by minimal cover cuts.
		m := NewModel("cover_root", Maximize)
		weights := []float64{6, 6, 6, 5, 5, 5}
		values := []float64{10, 10, 10, 8, 8, 8}
		xs := make([]Var, len(weights))
		var objT, rowT []Term
		for i := range weights {
			xs[i] = m.AddBinaryVar("x_" + strconv.Itoa(i))
			objT = append(objT, xs[i].T(values[i]))
			rowT = append(rowT, xs[i].T(weights[i]))
		}
		m.SetObjective(NewExpr(objT...))
		m.AddConstraint("kp", Le(NewExpr(rowT...), 16))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(false), WithCuts(true), WithRelGapTol(0), WithAbsGapTol(0), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("solve failed: status=%v err=%v", res.Status, err)
		}
		if tr.rootCutsCertified == 0 || res.RootCutBound >= res.RootLPBound-1e-6 {
			t.Fatalf("expected root cover/CG cut tightening: certified=%d rootLP=%g rootCut=%g",
				tr.rootCutsCertified, res.RootLPBound, res.RootCutBound)
		}
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "cover_root", raw)
		}
	})

	t.Run("corpus_t2_with_cuts_and_rcf", func(t *testing.T) {
		t.Parallel()
		var batch [][]byte
		totalRootCuts := 0
		for _, name := range []string{
			"t1_std_mip.json",
			"t1_knapsack_12.json",
			"t2_cflp_36.json",
			"t2_mkp_30.json",
			"t2_binpack_44.json",
			"t2_gap_45.json",
			"t2_lotsizing_48.json",
			"t2_setcover_50.json",
		} {
			mdl := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", name))
			p := mdl.Compile()
			tr := NewVIPRTrace()
			res, err := SolveCompiled(
				t.Context(),
				p,
				WithPureGo(),
				WithPresolve(true),
				WithCuts(true),
				WithMaxNodes(120),
				WithRelGapTol(0),
				WithAbsGapTol(0),
				WithVIPRTrace(tr),
			)
			if err != nil {
				t.Fatalf("%s solve error: %v", name, err)
			}
			if !tr.HasCertificate() {
				t.Fatalf("%s missing VIPR certificate (status=%v nodes=%d)", name, res.Status, res.Nodes)
			}
			totalRootCuts += tr.rootCutsCertified
			raw, mErr := tr.MarshalVIPRJSON()
			if mErr != nil {
				t.Fatalf("%s MarshalVIPRJSON: %v", name, mErr)
			}
			_ = decodeVIPRJSONForTest(t, raw)
			batch = append(batch, raw)
		}
		if totalRootCuts == 0 {
			t.Fatal("expected certified root cuts across T1/T2 corpus models")
		}
		if formalOn {
			runJSONLItems(t, viprExe, batch)
		}
	})
}

func TestVIPRTraceInTreeCutsAndPurge(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	t.Run("crafted_120v_intree_cuts", func(t *testing.T) {
		t.Parallel()
		// 120 binary variables split into 40 coupled 3-variable knapsack blocks with
		// MaxCutPasses=1 so root cuts tighten the root relaxation while child nodes still
		// expose violated minimal covers and CG cuts in tryInTreeCuts (numVars >= 120).
		const numBlocks = 40
		m := NewModel("intree_120v", Maximize)
		xs := make([]Var, numBlocks*3)
		objTerms := make([]Term, 0, len(xs))
		for b := range numBlocks {
			base := b * 3
			for k := range 3 {
				xs[base+k] = m.AddBinaryVar("b_" + strconv.Itoa(b) + "_" + strconv.Itoa(k))
				objTerms = append(objTerms, xs[base+k].T(float64(7+k)+0.1*float64(b%5)))
			}
			m.AddConstraint(
				"blk_"+strconv.Itoa(b),
				Le(NewExpr(xs[base].T(4), xs[base+1].T(4), xs[base+2].T(4)), 7),
			)
		}
		// Light coupling across consecutive blocks to force multi-node branching.
		for b := range numBlocks - 1 {
			m.AddConstraint(
				"link_"+strconv.Itoa(b),
				Le(NewExpr(xs[b*3].T(2), xs[b*3+1].T(2), xs[(b+1)*3].T(2)), 3),
			)
		}
		m.SetObjective(NewExpr(objTerms...))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(
			t.Context(),
			p,
			WithPureGo(),
			WithPresolve(false),
			WithCuts(true),
			WithMaxCutPasses(1),
			WithHeuristics(false),
			WithMaxNodes(12),
			WithVIPRTrace(tr),
		)
		if err != nil {
			t.Fatalf("solve error: %v", err)
		}
		if tr.rootCutsCertified == 0 {
			t.Fatalf("expected root cuts on 120-var model, got 0 (status=%v nodes=%d)", res.Status, res.Nodes)
		}
		if tr.inTreeCutsCertified == 0 {
			t.Fatalf("expected in-tree cuts on 120-var model, got 0 (status=%v nodes=%d cuts=%d)", res.Status, res.Nodes, res.CutsAdded)
		}
		if !tr.HasCertificate() {
			t.Fatalf("expected valid VIPR certificate with in-tree cuts, status=%v", res.Status)
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "intree_120v", raw)
		}
	})

	t.Run("corpus_t3_mkp_200_bound", func(t *testing.T) {
		t.Parallel()
		mdl := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t3_mkp_200.json"))
		p := mdl.Compile()
		tr := NewVIPRTrace()
		res, err := SolveCompiled(
			t.Context(),
			p,
			WithPureGo(),
			WithPresolve(true),
			WithCuts(true),
			WithMaxNodes(6),
			WithVIPRTrace(tr),
		)
		if err != nil {
			t.Fatalf("t3_mkp_200 solve error: %v", err)
		}
		if !tr.HasCertificate() {
			t.Fatalf("t3_mkp_200 missing VIPR certificate (status=%v nodes=%d)", res.Status, res.Nodes)
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "t3_mkp_200", raw)
		}
	})

	t.Run("purge_slack_cuts_sync", func(t *testing.T) {
		t.Parallel()
		// Add two certified cuts, record a dual referencing cut 0, then tighten bounds
		// so cut 0 becomes strictly slack and purge it via purgeSlackCuts. Verify that
		// activeCutIDs compacts cleanly and finalize still emits cut 0 for the earlier leaf.
		m := NewModel("purge_sync", Maximize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		x2 := m.AddBinaryVar("x2")
		m.SetObjective(NewExpr(x0.T(3), x1.T(3), x2.T(2)))
		m.AddConstraint("r0", Le(NewExpr(x0.T(2), x1.T(2)), 3))
		m.AddConstraint("r1", Le(NewExpr(x1.T(2), x2.T(2)), 3))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		rootRes := lp.Solve()
		if rootRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", rootRes.Status)
		}
		tr := NewVIPRTrace()
		rec := newBabVIPRRecorder(tr, p, false)
		defer rec.close()
		rec.recordNodeDual(rec.root, lp, rootRes)

		var cutWS cutGenWorkspace
		cuts := rec.generateCertifiedCutRound(lp, rootRes.Primal, 8, minCutEfficacy, &cutWS)
		if len(cuts) == 0 {
			t.Fatal("expected certified root cuts")
		}
		added := rec.addCertifiedCutPlanes(lp, cuts, true)
		if added < 2 {
			t.Fatalf("expected at least 2 added certified cuts, got %d", added)
		}
		left, right := rec.recordBranch(rec.root, 0, big.NewInt(0))
		// Solve right (x0 = 1) before purging so right.bestDual references cut x0+x1<=1.
		lp.SetStructuralBounds([]float64{1, 0, 0}, []float64{1, 1, 1})
		rightRes := lp.Solve()
		if rightRes.Status != StatusOptimal {
			t.Fatalf("right LP status=%v", rightRes.Status)
		}
		rec.recordNodeDual(right, lp, rightRes)

		// Fix x0=0, x1=0 in LP so cut x0+x1<=1 has slack 1.0 and purges while x1+x2<=1 stays tight.
		lp.SetStructuralBounds([]float64{0, 0, 0}, []float64{0, 0, 1})
		slackRes := lp.Solve()
		if slackRes.Status != StatusOptimal {
			t.Fatalf("slack LP status=%v", slackRes.Status)
		}
		purged := rec.purgeSlackCuts(lp, slackCutPurgeTol)
		if purged == 0 || rec.invalid {
			t.Fatalf("expected purged > 0 and valid recorder, got purged=%d invalid=%v", purged, rec.invalid)
		}
		if len(rec.activeCutIDs) != len(lp.extraRows) || len(rec.activeCutIDs) == 0 {
			t.Fatalf("activeCutIDs len=%d extraRows len=%d", len(rec.activeCutIDs), len(lp.extraRows))
		}
		// Solve left (x0 = 0) on the post-purge LP using the shifted surviving cut row.
		lp.SetStructuralBounds([]float64{0, 0, 0}, []float64{0, 1, 1})
		leftRes := lp.Solve()
		if leftRes.Status != StatusOptimal {
			t.Fatalf("left LP status=%v", leftRes.Status)
		}
		rec.recordNodeDual(left, lp, leftRes)

		rightUsesPurged := false
		for _, rm := range right.bestDual {
			if rm.rowID >= p.numConstraints && !slices.Contains(rec.activeCutIDs, rm.rowID-p.numConstraints) {
				rightUsesPurged = true
				break
			}
		}
		leftUsesSurviving := false
		for _, rm := range left.bestDual {
			if rm.rowID >= p.numConstraints && slices.Contains(rec.activeCutIDs, rm.rowID-p.numConstraints) {
				leftUsesSurviving = true
				break
			}
		}
		if !rightUsesPurged || !leftUsesSurviving {
			t.Fatalf("expected purged cut on right and surviving cut on left, got rightUsesPurged=%v leftUsesSurviving=%v", rightUsesPurged, leftUsesSurviving)
		}

		rec.finalize(Result{Status: StatusOptimal, Values: []float64{1, 0, 1}})
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert after purge, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "purge_sync", raw)
		}
	})
}

func TestVIPRTraceReducedCostFixingAndUnboundedInt(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	t.Run("root_and_intree_rcf", func(t *testing.T) {
		t.Parallel()
		// Multi-level fractional core on (b0..b4) so branching finds a feasible incumbent
		// while sibling nodes remain open and fractional, exercising both root RCF (heur=true)
		// and in-tree RCF (heur=false) on both upper-bound (z0, z1) and lower-bound (w0, w1)
		// nonbasic integer columns.
		m := NewModel("rcf_both_sides", Minimize)
		b0 := m.AddBinaryVar("b0")
		b1 := m.AddBinaryVar("b1")
		b2 := m.AddBinaryVar("b2")
		b3 := m.AddBinaryVar("b3")
		b4 := m.AddBinaryVar("b4")
		z0 := m.AddIntVar("z0", 0, 5)
		z1 := m.AddIntVar("z1", 0, 5)
		w0 := m.AddIntVar("w0", 0, 4)
		w1 := m.AddIntVar("w1", 0, 4)
		m.SetObjective(NewExpr(
			b0.T(2), b1.T(2.5), b2.T(3), b3.T(2.2), b4.T(2.8),
			z0.T(20), z1.T(25), w0.T(-15), w1.T(-18),
		))
		m.AddConstraint("c0", Ge(NewExpr(b0.T(2), b1.T(2), b2.T(2)), 3))
		m.AddConstraint("c1", Ge(NewExpr(b2.T(2), b3.T(2), b4.T(2)), 3))
		m.AddConstraint("c2", Ge(NewExpr(b0.T(1), b1.T(1), z0.T(1)), 1))
		m.AddConstraint("c3", Le(NewExpr(b3.T(1), b4.T(1), z1.T(-1)), 1))
		m.AddConstraint("c4", Le(NewExpr(w0.T(1), w1.T(1)), 8))
		p := m.Compile()

		for _, heur := range []bool{true, false} {
			name := "heur_" + strconv.FormatBool(heur)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				tr := NewVIPRTrace()
				res, err := SolveCompiled(
					t.Context(),
					p,
					WithPureGo(),
					WithPresolve(false),
					WithCuts(false),
					WithHeuristics(heur),
					WithRelGapTol(0),
					WithAbsGapTol(0),
					WithVIPRTrace(tr),
				)
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("%s solve failed: status=%v err=%v", name, res.Status, err)
				}
				if tr.rcfFixingsCertified == 0 {
					t.Fatalf("%s expected certified RCF fixings > 0, got 0 (nodes=%d)", name, res.Nodes)
				}
				if !tr.HasCertificate() || tr.Mode() != "optimal" {
					t.Fatalf("%s expected optimal VIPR cert, got hasCert=%v mode=%q", name, tr.HasCertificate(), tr.Mode())
				}
				raw, mErr := tr.MarshalVIPRJSON()
				if mErr != nil {
					t.Fatalf("%s MarshalVIPRJSON: %v", name, mErr)
				}
				_ = decodeVIPRJSONForTest(t, raw)
				if formalOn {
					requireVIPRBytesVerified(t, viprExe, "rcf_"+name, raw)
				}
			})
		}
	})

	t.Run("skip_uncertifiable_rcf_candidate", func(t *testing.T) {
		t.Parallel()
		// Provide one valid RCF candidate (z0 <= 0, dj=20 > gap=2) alongside one bogus
		// candidate (b0 <= 0, dj=0) that cannot be certified rationally; only the uncertifiable
		// candidate must be skipped while the valid one is certified and verified.
		m := NewModel("rcf_partial_skip", Minimize)
		b0 := m.AddBinaryVar("b0")
		b1 := m.AddBinaryVar("b1")
		z0 := m.AddIntVar("z0", 0, 5)
		m.SetObjective(NewExpr(b0.T(2), b1.T(2), z0.T(20)))
		m.AddConstraint("c0", Ge(NewExpr(b0.T(2), b1.T(2)), 3))
		p := m.Compile()

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("root LP status=%v", lpRes.Status)
		}
		tr := NewVIPRTrace()
		rec := newBabVIPRRecorder(tr, p, false)
		defer rec.close()
		rec.recordNodeDual(rec.root, lp, lpRes)

		inc := []float64{1, 1, 0}
		cands := []boundDelta{
			{col: 0, lb: 0, ub: 0}, // uncertifiable: fixing b0 <= 0 does not close b0 >= 1 against obj=4
			{col: 2, lb: 0, ub: 0}, // certifiable: z0 >= 1 has dual bound 3 + 20 = 23 >= 4
		}
		nextVN, applied, pruned := rec.certifyAndApplyRCF(rec.root, lp, lpRes, inc, 4.0, 0, false, p.varLB, p.varUB, cands)
		if pruned || len(applied) != 1 || applied[0].col != 2 || rec.rcfFixingsCertified != 1 {
			t.Fatalf("expected only col 2 certified, got pruned=%v applied=%+v count=%d", pruned, applied, rec.rcfFixingsCertified)
		}
		// Finish the remaining subtree on b0 in {0, 1} to obtain a complete optimal proof.
		l, r := rec.recordBranch(nextVN, 0, big.NewInt(0))
		rec.certifySubtreeLeaf(l)
		if feas, _ := rec.propagateNode(r, nil, nil, nodePropPasses); !feas {
			t.Fatal("expected right branch b0=1 to remain feasible after propagation")
		}
		rec.certifySubtreeLeaf(r)
		rec.finalize(Result{Status: StatusOptimal, Values: inc})
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert after partial RCF skip, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "rcf_partial_skip", raw)
		}
	})

	t.Run("unbounded_int_direct_and_propagated", func(t *testing.T) {
		t.Parallel()
		// Direct objective tightening on unbounded integers (u0, u1) plus propagated
		// tightening on zero-cost unbounded integer u2 linked to continuous variable yc.
		m := NewModel("unb_int_tighten", Minimize)
		u0 := m.AddIntVar("u0", 0, math.Inf(1))
		u1 := m.AddIntVar("u1", 0, math.Inf(1))
		u2 := m.AddIntVar("u2", 0, math.Inf(1))
		yc := m.AddContVar("yc", 0, math.Inf(1))
		m.SetObjective(NewExpr(u0.T(3), u1.T(4), yc.T(2)))
		m.AddConstraint("r0", Ge(NewExpr(u0.T(2), u1.T(2)), 5))
		m.AddConstraint("r1", Le(NewExpr(u2.T(1), yc.T(-1)), 0))
		m.AddConstraint("r2", Ge(NewExpr(u2.T(2), u0.T(1)), 3))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(
			t.Context(),
			p,
			WithPureGo(),
			WithPresolve(false),
			WithCuts(false),
			WithHeuristics(true),
			WithRelGapTol(0),
			WithAbsGapTol(0),
			WithVIPRTrace(tr),
		)
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("solve failed: status=%v err=%v", res.Status, err)
		}
		if tr.unbTightCertified == 0 {
			t.Fatalf("expected certified unbounded-int tightenings > 0, got 0 (nodes=%d)", res.Nodes)
		}
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal VIPR cert, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, mErr := tr.MarshalVIPRJSON()
		if mErr != nil {
			t.Fatalf("MarshalVIPRJSON: %v", mErr)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "unb_int_tighten", raw)
		}
	})
}

func TestVIPRTraceRaisedCapsAndLargeSplitBounds(t *testing.T) {
	t.Parallel()
	formalOn := os.Getenv("GOBLIN_FORMAL") == "1"
	var viprExe string
	if formalOn {
		viprExe = formalExe(t, "vipr-check")
	}

	t.Run("wide_row_propagation_over_64_cols", func(t *testing.T) {
		t.Parallel()
		// 100-column single constraint row (> old viprMaxPropRowLen=64, <= 512)
		// whose activity contradiction is detected directly by propagateNode.
		const nCols = 100
		m := NewModel("wide_row_100", Minimize)
		xs := make([]Var, nCols)
		terms := make([]Term, nCols)
		for j := range nCols {
			xs[j] = m.AddBinaryVar("x_" + strconv.Itoa(j))
			terms[j] = xs[j].T(1)
		}
		m.SetObjective(NewExpr(terms...))
		m.AddConstraint("wide_ge", Ge(NewExpr(terms...), float64(nCols+1)))
		p := m.Compile()

		tr := NewVIPRTrace()
		res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(true), WithVIPRTrace(tr))
		if err != nil || res.Status != StatusInfeasible {
			t.Fatalf("solve failed: status=%v err=%v", res.Status, err)
		}
		if !tr.HasCertificate() || tr.Mode() != "infeasible" {
			t.Fatalf("expected infeasible VIPR cert on 100-col row, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "wide_row_100", raw)
		}
	})

	t.Run("high_bit_denom_propagation_over_256_bits", func(t *testing.T) {
		t.Parallel()
		// Chain 3*y_{k+1} <= y_k across 170 steps so y_170's propagated rational upper bound
		// has denominator 3^170 (~270 bits > old viprMaxRatDenomBits=256, <= 2048), then
		// contradicts y_170 >= 1/2.
		const steps = 170
		m := NewModel("denom_chain_270bits", Minimize)
		ys := make([]Var, steps+1)
		ys[0] = m.AddContVar("y_0", 0, 1)
		for k := 1; k < steps; k++ {
			ys[k] = m.AddContVar("y_"+strconv.Itoa(k), 0, 10)
		}
		ys[steps] = m.AddContVar("y_end", 0.5, 10)
		m.SetObjective(NewExpr(ys[0].T(1)))
		for k := range steps {
			m.AddConstraint("c_"+strconv.Itoa(k), Le(NewExpr(ys[k+1].T(3), ys[k].T(-1)), 0))
		}
		p := m.Compile()

		tr := NewVIPRTrace()
		rec := newBabVIPRRecorder(tr, p, false)
		defer rec.close()
		feas, _ := rec.propagateNode(rec.root, nil, nil, 1)
		if feas {
			t.Fatal("expected infeasibility via exact rational bound propagation past 256-bit denominator")
		}
		rec.finalize(Result{Status: StatusInfeasible})
		if !tr.HasCertificate() || tr.Mode() != "infeasible" {
			t.Fatalf("expected infeasible VIPR cert past 256-bit denom, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		_ = decodeVIPRJSONForTest(t, raw)
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "denom_chain_270bits", raw)
		}
	})

	t.Run("arbitrary_precision_split_above_2p62", func(t *testing.T) {
		t.Parallel()
		// Exercise exact *big.Int branch split above 2^62 (2^70) directly through
		// babVIPRRecorder -> MarshalVIPRJSON -> vipr-check.
		m := NewModel("huge_split_2p70", Minimize)
		bigUB := math.Ldexp(1.0, 72)
		x := m.AddIntVar("x", 0, bigUB)
		m.SetObjective(NewExpr(x.T(1)))
		m.AddConstraint("c0", Ge(NewExpr(x.T(1)), 0))
		p := m.Compile()

		tr := NewVIPRTrace()
		rec := newBabVIPRRecorder(tr, p, false)
		defer rec.close()
		splitVal := new(big.Int).Lsh(big.NewInt(1), 70)
		left, right := rec.recordBranch(rec.root, 0, splitVal)
		if left == nil || right == nil || rec.invalid {
			t.Fatal("recordBranch failed on 2^70 split value")
		}
		left.kind = viprNodeLeafDual
		left.bestDual = nil
		left.bestDualRHS = big.NewRat(0, 1)
		right.kind = viprNodeLeafDual
		right.bestDual = nil
		right.bestDualRHS = new(big.Rat).SetInt(new(big.Int).Add(splitVal, big.NewInt(1)))
		rec.finalize(Result{Status: StatusOptimal, Values: []float64{0}})
		if !tr.HasCertificate() || tr.Mode() != "optimal" {
			t.Fatalf("expected optimal cert on 2^70 split, got hasCert=%v mode=%q", tr.HasCertificate(), tr.Mode())
		}
		raw, err := tr.MarshalVIPRJSON()
		if err != nil {
			t.Fatalf("MarshalVIPRJSON: %v", err)
		}
		cert := decodeVIPRJSONForTest(t, raw)
		foundBigSplit := false
		wantTok := splitVal.String()
		for _, st := range cert.Steps {
			if st.Rule == "branch" && string(st.SplitVal) == wantTok {
				foundBigSplit = true
				break
			}
		}
		if !foundBigSplit {
			t.Fatalf("expected branch splitVal %s in serialized VIPR JSON", wantTok)
		}
		if formalOn {
			requireVIPRBytesVerified(t, viprExe, "huge_split_2p70", raw)
		}
	})
}
