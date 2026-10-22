package gopt_test

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopt"
	"gopt/bench"
	"gopt/format"
)

func TestPuLPCanonicalLPsAndMIPs(t *testing.T) {
	// 1. std_lp: x=4, y=-1, z=6, obj=54.0
	{
		m := gopt.NewModel("std_lp", gopt.Minimize)
		x := m.AddContVar("x", 0, 4)
		y := m.AddContVar("y", -1, 1)
		z := m.AddContVar("z", 0, math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 4), gopt.T(z, 9)))
		m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 5))
		m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), 10))
		m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), 7))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-54.0) > 1e-6 {
			t.Fatalf("std_lp failed: status=%v obj=%v", res.Status, res.Objective)
		}
	}

	// 2. std_mip: x=3, y=-0.5, z=7 (integer), obj=64.0
	{
		m := gopt.NewModel("std_mip", gopt.Minimize)
		x := m.AddContVar("x", 0, 4)
		y := m.AddContVar("y", -1, 1)
		z := m.AddIntVar("z", 0, math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 4), gopt.T(z, 9)))
		m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 5))
		m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), 10))
		m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), 7.5))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-64.0) > 1e-6 {
			t.Fatalf("std_mip failed: status=%v obj=%v values=%v", res.Status, res.Objective, res.Values)
		}
		if math.Abs(res.Value(x)-3.0) > 1e-6 || math.Abs(res.Value(y)-(-0.5)) > 1e-6 || math.Abs(res.Value(z)-7.0) > 1e-6 {
			t.Fatalf("std_mip unexpected primal values: %v", res.Values)
		}
	}

	// 3. test_mip_floats_objective: obj=64.95
	{
		m := gopt.NewModel("mip_float_obj", gopt.Minimize)
		x := m.AddContVar("x", 0, 4)
		y := m.AddContVar("y", -1, 1)
		z := m.AddIntVar("z", 0, math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1.1), gopt.T(y, 4.1), gopt.T(z, 9.1)))
		m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 5))
		m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), 10))
		m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), 7.5))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-64.95) > 1e-6 {
			t.Fatalf("mip_float_obj failed: status=%v obj=%v", res.Status, res.Objective)
		}
	}

	// 4. test_integer_infeasible: x+y <= 5.2, x+z >= 10.3, -y+z == 7.4 with x,y,z integer
	{
		m := gopt.NewModel("integer_infeasible", gopt.Minimize)
		x := m.AddIntVar("x", 0, 4)
		y := m.AddIntVar("y", -1, 1)
		z := m.AddIntVar("z", 0, 10)
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 4), gopt.T(z, 9)))
		m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 5.2))
		m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), 10.3))
		m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), 7.4))

		res := m.Solve()
		if res.Status != gopt.StatusInfeasible {
			t.Fatalf("expected Infeasible on integer_infeasible, got %v", res.Status)
		}
	}
}

func TestFullCorpusMILPParityWithCBC(t *testing.T) {
	rawRef, err := os.ReadFile("bench/results/python_baseline.json")
	if err != nil {
		t.Skipf("skipping corpus parity test (no python_baseline.json): %v", err)
	}
	var pyRef map[string]struct {
		Status                string   `json:"status"`
		Objective             *float64 `json:"objective"`
		LPRelaxationObjective *float64 `json:"lp_relaxation_objective"`
	}
	if err := json.Unmarshal(rawRef, &pyRef); err != nil {
		t.Fatalf("unmarshal python_baseline.json: %v", err)
	}

	cases := bench.BuildCorpus()
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			ref, ok := pyRef[c.ID]
			if !ok {
				t.Fatalf("missing reference entry for %s", c.ID)
			}
			p := c.Model.Compile()
			opts := gopt.DefaultOptions()
			opts.TimeLimit = 15 * time.Second
			res := gopt.SolveCompiled(p, opts)
			t.Logf("%-18s | status=%-10s | obj=%12.4f | nodes=%5d | iters=%6d | cuts=%2d | time=%v",
				c.ID, res.Status, res.Objective, res.Nodes, res.Iterations, res.CutsAdded, res.SolveTime)

			if res.Status.String() != ref.Status {
				t.Fatalf("[%s] status mismatch: gopt=%s, CBC=%s", c.ID, res.Status, ref.Status)
			}
			if ref.Objective != nil {
				wantObj := *ref.Objective
				relDiff := math.Abs(res.Objective-wantObj) / max(1.0, math.Abs(wantObj))
				if relDiff > 1e-5 {
					t.Fatalf("[%s] objective mismatch: gopt=%.8f, CBC=%.8f (relDiff=%.2e, nodes=%d, cuts=%d)",
						c.ID, res.Objective, wantObj, relDiff, res.Nodes, res.CutsAdded)
				}
				bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP)
				if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
					t.Fatalf("[%s] primal solution violation: bound=%.2e, row=%.2e, int=%.2e",
						c.ID, bV, rV, iV)
				}
			}
		})
	}
}

// TestAllTrialParity solves every randomized trial instance in testdata/corpus/trials and
// compares status and objective with the frozen CBC 2.10.3 reference (python_baseline.json)
// and with goblin's in-process CBC results (goblin_cbc.json), and checks each gopt solution
// for feasibility independently of either reference.
func TestAllTrialParity(t *testing.T) {
	if testing.Short() {
		t.Skip("solves all 293 trial instances; skipped with -short")
	}
	var pyRef map[string]bench.PythonBaselineEntry
	raw, err := os.ReadFile("bench/results/python_baseline.json")
	if err != nil {
		t.Fatalf("read python baseline: %v", err)
	}
	if err := json.Unmarshal(raw, &pyRef); err != nil {
		t.Fatal(err)
	}
	var goblinRef map[string]bench.ProblemBenchResult
	raw, err = os.ReadFile("bench/results/goblin_cbc.json")
	if err != nil {
		t.Fatalf("read goblin telemetry: %v", err)
	}
	if err := json.Unmarshal(raw, &goblinRef); err != nil {
		t.Fatal(err)
	}

	solved := 0
	for _, c := range bench.BuildCorpus() {
		lines, err := os.ReadFile(filepath.Join("testdata", "corpus", "trials", c.ID+".jsonl"))
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		py := pyRef[c.ID]
		gb := goblinRef[c.ID]
		for k, line := range bytes.Split(bytes.TrimSpace(lines), []byte("\n")) {
			doc, err := format.ParsePuLPJSON(line)
			if err != nil {
				t.Fatalf("%s[%d]: %v", c.ID, k, err)
			}
			m, err := format.BuildModelFromPuLPJSON(doc)
			if err != nil {
				t.Fatalf("%s[%d]: %v", c.ID, k, err)
			}
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("%s[%d]: %v", c.ID, k, err)
			}
			opts := gopt.DefaultOptions()
			opts.TimeLimit = 30 * time.Second
			res := gopt.SolveCompiled(p, opts)
			solved++

			if k >= len(py.TrialObjectives) {
				t.Fatalf("%s[%d]: no CBC reference for this trial", c.ID, k)
			}
			want := py.TrialObjectives[k]
			if want == nil {
				if res.Status == gopt.StatusOptimal {
					t.Errorf("%s[%d]: gopt Optimal %.8g, CBC reported no optimum (%s)", c.ID, k, res.Objective, py.Status)
				}
				continue
			}
			if res.Status != gopt.StatusOptimal {
				t.Errorf("%s[%d]: gopt %v, CBC Optimal %.8g", c.ID, k, res.Status, *want)
				continue
			}
			if d := math.Abs(res.Objective-*want) / max(1.0, math.Abs(*want)); d > 1e-5 {
				t.Errorf("%s[%d]: gopt %.8g, CBC %.8g (rel %.1e)", c.ID, k, res.Objective, *want, d)
			}
			if k < len(gb.TrialObjectives) {
				if d := math.Abs(res.Objective-gb.TrialObjectives[k]) / max(1.0, math.Abs(gb.TrialObjectives[k])); d > 1e-5 {
					t.Errorf("%s[%d]: gopt %.8g, goblin %.8g (rel %.1e)", c.ID, k, res.Objective, gb.TrialObjectives[k], d)
				}
			}
			if bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP); bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
				t.Errorf("%s[%d]: infeasible solution: bound=%.1e row=%.1e int=%.1e", c.ID, k, bV, rV, iV)
			}
		}
	}
	if solved != 293 {
		t.Fatalf("solved %d trial instances, want 293", solved)
	}
}

func TestLimitTerminationStatusAndBounds(t *testing.T) {
	corpusByID := make(map[string]*gopt.CompiledProblem)
	for _, c := range bench.BuildCorpus() {
		switch c.ID {
		case "t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504":
			corpusByID[c.ID] = c.Model.Compile()
		}
	}

	for _, id := range []string{"t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504"} {
		p := corpusByID[id]
		optRes := gopt.SolveCompiled(p)
		if optRes.Status != gopt.StatusOptimal {
			t.Fatalf("[%s] expected Optimal with default options, got %v", id, optRes.Status)
		}
		for _, maxNodes := range []int{1, 2, 5, 20} {
			o := gopt.DefaultOptions()
			o.EnableCuts = false
			o.MaxNodes = maxNodes
			res := gopt.SolveCompiled(p, o)
			if res.Status != gopt.StatusNodeLimit {
				t.Fatalf("[%s] MaxNodes=%d: expected NodeLimit, got %v (obj=%.4f, bestBound=%.4f)",
					id, maxNodes, res.Status, res.Objective, res.BestBound)
			}
			if p.Sense == gopt.Maximize {
				if res.BestBound < optRes.Objective-1e-4 {
					t.Fatalf("[%s] MaxNodes=%d: invalid upper bound %.4f < optimal %.4f",
						id, maxNodes, res.BestBound, optRes.Objective)
				}
			} else {
				if res.BestBound > optRes.Objective+1e-4 {
					t.Fatalf("[%s] MaxNodes=%d: invalid lower bound %.4f > optimal %.4f",
						id, maxNodes, res.BestBound, optRes.Objective)
				}
			}
		}
	}
}

func TestPhase1IterationLimitStatus(t *testing.T) {
	m := gopt.NewModel("phase1_iter_limit", gopt.Minimize)
	x := m.AddContVar("x", 0, math.Inf(1))
	y := m.AddContVar("y", 0, math.Inf(1))
	f := m.AddContVar("f", math.Inf(-1), math.Inf(1))
	m.SetObjective(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 2), gopt.T(f, 1)))
	m.AddConstraint("a", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 2))
	m.AddConstraint("b", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, -1)), 1))
	m.AddConstraint("c", gopt.Ge(gopt.NewExpr(gopt.T(f, 1), gopt.T(x, -1)), -3))
	m.AddConstraint("d", gopt.Le(gopt.NewExpr(gopt.T(f, 1), gopt.T(y, 1)), 10))
	p := m.Compile()

	full := gopt.NewLPSolver(p, nil).Solve()
	if full.Status != gopt.StatusOptimal || math.Abs(full.Objective-1.0) > 1e-6 {
		t.Fatalf("expected Optimal obj=1.0, got status=%v obj=%v", full.Status, full.Objective)
	}

	for _, maxIter := range []int{1, 2, 3} {
		s := gopt.NewLPSolver(p, nil)
		s.MaxIter = maxIter
		res := s.Solve()
		if res.Status != gopt.StatusIterationLimit {
			t.Fatalf("MaxIter=%d: expected IterationLimit on feasible Phase-I LP, got %v", maxIter, res.Status)
		}
	}
}

func TestTimeLimitEnforcement(t *testing.T) {
	for _, c := range bench.BuildCorpus() {
		if c.ID != "t3_mad_g4_i1000" && c.ID != "t3_mkp_200" {
			continue
		}
		p := c.Model.Compile()
		opts := gopt.DefaultOptions()
		opts.TimeLimit = 5 * time.Millisecond
		t0 := time.Now()
		res := gopt.SolveCompiled(p, opts)
		elapsed := time.Since(t0)
		if res.Status != gopt.StatusTimeLimit && res.Status != gopt.StatusOptimal {
			t.Fatalf("[%s] expected TimeLimit or Optimal, got %v", c.ID, res.Status)
		}
		if elapsed > 50*time.Millisecond {
			t.Fatalf("[%s] TimeLimit=5ms overshot wall-clock budget: elapsed=%v", c.ID, elapsed)
		}
	}
}

func TestModelValidationForeignVarAndNaN(t *testing.T) {
	// 1. Foreign Var handle (both same index 0 and out-of-range index)
	a := gopt.NewModel("a", gopt.Maximize)
	b := gopt.NewModel("b", gopt.Maximize)
	xa := a.AddContVar("x", 0, 10)
	yb0 := b.AddContVar("y0", 0, 10)
	a.SetObjective(gopt.NewExpr(gopt.T(xa, 1), gopt.T(yb0, 100)))
	a.AddConstraint("c", gopt.Le(gopt.NewExpr(gopt.T(xa, 1), gopt.T(yb0, 1)), 3))
	if err := a.Validate(); err == nil {
		t.Fatalf("expected Validate() error on foreign Var handle, got nil")
	}
	if _, err := a.CompileChecked(); err == nil {
		t.Fatalf("expected CompileChecked() error on foreign Var handle, got nil")
	}
	if res := a.Solve(); res.Status != gopt.StatusUndefined {
		t.Fatalf("expected StatusUndefined when solving model with foreign Var, got %v", res.Status)
	}

	// 2. Unowned raw Var(999999) with zero coefficient
	mRaw := gopt.NewModel("raw_var", gopt.Minimize)
	x := mRaw.AddContVar("x", 0, 5)
	mRaw.SetObjective(gopt.NewExpr(gopt.T(x, 1), gopt.T(gopt.Var(999999), 0)))
	if err := mRaw.Validate(); err == nil {
		t.Fatalf("expected Validate() error on unowned raw Var(999999), got nil")
	}
	if res := mRaw.Solve(); res.Status != gopt.StatusUndefined {
		t.Fatalf("expected StatusUndefined on unowned raw Var(999999), got %v", res.Status)
	}

	// 3. NaN coefficient
	mNaN := gopt.NewModel("nan_obj", gopt.Minimize)
	v := mNaN.AddContVar("v", 0, 10)
	mNaN.SetObjective(gopt.NewExpr(gopt.T(v, math.NaN())))
	if err := mNaN.Validate(); err == nil {
		t.Fatalf("expected Validate() error on NaN objective coefficient, got nil")
	}
	if res := mNaN.Solve(); res.Status != gopt.StatusUndefined {
		t.Fatalf("expected StatusUndefined on NaN objective coefficient, got %v", res.Status)
	}

	// 4. Duplicate variable name
	mDup := gopt.NewModel("dup_var", gopt.Minimize)
	mDup.AddContVar("x", 0, 1)
	mDup.AddContVar("x", 0, 1)
	if err := mDup.Validate(); err == nil {
		t.Fatalf("expected Validate() error on duplicate variable name, got nil")
	}
	if res := mDup.Solve(); res.Status != gopt.StatusUndefined {
		t.Fatalf("expected StatusUndefined on duplicate variable name, got %v", res.Status)
	}
}

func TestRandomMADNegativeControls(t *testing.T) {
	for _, tc := range []struct {
		name string
		g, i int
		seed uint64
	}{
		{"mad_rand_g2_i4", 2, 4, 7000},
		{"mad_rand_g2_i8", 2, 8, 7001},
		{"mad_rand_g3_i9", 3, 9, 7002},
		{"mad_rand_g3_i9_s8011", 3, 9, 8011},
	} {
		m := bench.BuildMADRandom(tc.name, tc.g, tc.i, 6, tc.seed)
		p := m.Compile()
		res := gopt.SolveCompiled(p)
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("[%s] expected Optimal, got %v", tc.name, res.Status)
		}
		if res.Objective <= res.RootLPBound+1.0 {
			t.Fatalf("[%s] expected strictly positive integrality gap on random-weight MAD (rootLP=%.4f, opt=%.4f)",
				tc.name, res.RootLPBound, res.Objective)
		}
		if res.Nodes <= 1 {
			t.Fatalf("[%s] expected B&B tree exploration (Nodes > 1) on random-weight MAD, got %d", tc.name, res.Nodes)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
			t.Fatalf("[%s] violation: bound=%.2e row=%.2e int=%.2e", tc.name, bV, rV, iV)
		}
	}

	// On larger symmetric random-weight MAD (G=4, I=12) with a bounded node budget,
	// gopt must report StatusNodeLimit (never false StatusOptimal) while returning a valid incumbent.
	mSym := bench.BuildMADRandom("mad_rand_g4_i12", 4, 12, 6, 7003)
	pSym := mSym.Compile()
	opts := gopt.DefaultOptions()
	opts.MaxNodes = 200
	resSym := gopt.SolveCompiled(pSym, opts)
	if resSym.Status != gopt.StatusNodeLimit {
		t.Fatalf("[mad_rand_g4_i12] expected StatusNodeLimit at MaxNodes=200, got %v", resSym.Status)
	}
	if len(resSym.Values) == pSym.NumVars {
		bV, rV, iV := pSym.MaxViolation(resSym.Values, true)
		if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
			t.Fatalf("[mad_rand_g4_i12] incumbent violation: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
		}
	}
}
