package goblin_test

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
)

func TestPuLPCanonicalLPsAndMIPs(t *testing.T) {
	backends := []struct {
		name string
		opt  goblin.Option
	}{
		{"PureGo", goblin.WithPureGo()},
		{"PureGo_NoSIMD", goblin.WithoutSIMD()},
	}
	if goblin.CBCAvailable() {
		backends = append(backends, struct {
			name string
			opt  goblin.Option
		}{"CBC", goblin.WithCBC()})
	}

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			buildCanonical := func(name string, intZ bool, cx, cy, cz, rhs3 float64) (*goblin.Model, goblin.Var, goblin.Var, goblin.Var) {
				m := goblin.NewModel(name, goblin.Minimize)
				x := m.AddContVar("x", 0, 4)
				y := m.AddContVar("y", -1, 1)
				var z goblin.Var
				if intZ {
					z = m.AddIntVar("z", 0, math.Inf(1))
				} else {
					z = m.AddContVar("z", 0, math.Inf(1))
				}
				m.SetObjective(goblin.NewExpr(goblin.T(x, cx), goblin.T(y, cy), goblin.T(z, cz)))
				m.AddConstraint("c1", goblin.Le(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, 1)), 5))
				m.AddConstraint("c2", goblin.Ge(goblin.NewExpr(goblin.T(x, 1), goblin.T(z, 1)), 10))
				m.AddConstraint("c3", goblin.Eq(goblin.NewExpr(goblin.T(y, -1), goblin.T(z, 1)), rhs3))
				return m, x, y, z
			}

			// 1. std_lp: x=4, y=-1, z=6, obj=54.0
			{
				m, _, _, _ := buildCanonical("std_lp", false, 1, 4, 9, 7)
				res := m.Solve(b.opt)
				if res.Status != goblin.StatusOptimal || math.Abs(res.Objective-54.0) > 1e-6 {
					t.Fatalf("std_lp failed: status=%v obj=%v", res.Status, res.Objective)
				}
			}

			// 2. std_mip: x=3, y=-0.5, z=7 (integer), obj=64.0
			{
				m, x, y, z := buildCanonical("std_mip", true, 1, 4, 9, 7.5)
				res := m.Solve(b.opt)
				if res.Status != goblin.StatusOptimal || math.Abs(res.Objective-64.0) > 1e-6 {
					t.Fatalf("std_mip failed: status=%v obj=%v values=%v", res.Status, res.Objective, res.Values)
				}
				if math.Abs(res.Value(x)-3.0) > 1e-6 || math.Abs(res.Value(y)-(-0.5)) > 1e-6 || math.Abs(res.Value(z)-7.0) > 1e-6 {
					t.Fatalf("std_mip unexpected primal values: %v", res.Values)
				}
			}

			// 3. test_mip_floats_objective: obj=64.95
			{
				m, _, _, _ := buildCanonical("mip_float_obj", true, 1.1, 4.1, 9.1, 7.5)
				res := m.Solve(b.opt)
				if res.Status != goblin.StatusOptimal || math.Abs(res.Objective-64.95) > 1e-6 {
					t.Fatalf("mip_float_obj failed: status=%v obj=%v", res.Status, res.Objective)
				}
			}

			// 4. test_integer_infeasible: x+y <= 5.2, x+z >= 10.3, -y+z == 7.4 with x,y,z integer
			{
				m := goblin.NewModel("integer_infeasible", goblin.Minimize)
				x := m.AddIntVar("x", 0, 4)
				y := m.AddIntVar("y", -1, 1)
				z := m.AddIntVar("z", 0, 10)
				m.SetObjective(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, 4), goblin.T(z, 9)))
				m.AddConstraint("c1", goblin.Le(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, 1)), 5.2))
				m.AddConstraint("c2", goblin.Ge(goblin.NewExpr(goblin.T(x, 1), goblin.T(z, 1)), 10.3))
				m.AddConstraint("c3", goblin.Eq(goblin.NewExpr(goblin.T(y, -1), goblin.T(z, 1)), 7.4))

				res := m.Solve(b.opt)
				if res.Status != goblin.StatusInfeasible {
					t.Fatalf("expected Infeasible on integer_infeasible, got %v", res.Status)
				}
			}
		})
	}
}

func TestUnifiedOptionsAndBackends(t *testing.T) {
	// Verify that solver flags compose cleanly. PureGo, PureGo WithoutSIMD, and CBC
	// must agree on t3_sched_1000 where AVX2 vector kernels run.
	var target *goblin.Model
	for _, c := range bench.BuildCorpus() {
		if c.ID == "t3_sched_1000" {
			target = c.Model
			break
		}
	}
	if target == nil {
		t.Fatal("missing t3_sched_1000 in corpus")
	}
	p := target.Compile()

	resSIMD := goblin.SolveCompiled(p, goblin.WithPureGo(), goblin.WithSIMD(true), goblin.WithTimeLimit(10*time.Second))
	resScalar := goblin.SolveCompiled(p, goblin.WithPureGo(), goblin.WithoutSIMD(), goblin.WithTimeLimit(10*time.Second))
	if resSIMD.Status != goblin.StatusOptimal || resScalar.Status != goblin.StatusOptimal {
		t.Fatalf("expected Optimal, got simd=%v scalar=%v", resSIMD.Status, resScalar.Status)
	}
	if math.Abs(resSIMD.Objective-resScalar.Objective) > 1e-6 {
		t.Fatalf("objective mismatch between WithSIMD(true) and WithoutSIMD(): %.8f vs %.8f",
			resSIMD.Objective, resScalar.Objective)
	}

	if goblin.CBCAvailable() {
		resCBC := goblin.SolveCompiled(p, goblin.WithCBC(), goblin.WithTimeLimit(10*time.Second))
		if resCBC.Status != goblin.StatusOptimal {
			t.Fatalf("CBC backend status = %v, want Optimal", resCBC.Status)
		}
		rel := math.Abs(resCBC.Objective-resScalar.Objective) / max(1.0, math.Abs(resScalar.Objective))
		if rel > 1e-5 {
			t.Fatalf("CBC backend objective %.8f != PureGo %.8f (rel=%.2e)",
				resCBC.Objective, resScalar.Objective, rel)
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
			res := goblin.SolveCompiled(p, goblin.WithTimeLimit(15*time.Second))
			t.Logf("%-18s | status=%-10s | obj=%12.4f | nodes=%5d | iters=%6d | cuts=%2d | time=%v",
				c.ID, res.Status, res.Objective, res.Nodes, res.Iterations, res.CutsAdded, res.SolveTime)

			if res.Status.String() != ref.Status {
				t.Fatalf("[%s] status mismatch: goblin=%s, CBC=%s", c.ID, res.Status, ref.Status)
			}
			if ref.Objective != nil {
				wantObj := *ref.Objective
				relDiff := math.Abs(res.Objective-wantObj) / max(1.0, math.Abs(wantObj))
				if relDiff > 1e-5 {
					t.Fatalf("[%s] objective mismatch: goblin=%.8f, CBC=%.8f (relDiff=%.2e, nodes=%d, cuts=%d)",
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

// TestAllTrialParity solves all 293 randomized trial problems in testdata/corpus/trials.
// It checks status and objective against the frozen CBC 2.10.3 baseline (python_baseline.json)
// and against goblin's in-process CBC run (goblin_cbc.json), and verifies primal and row bounds
// on every result independently of both baselines.
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
		t.Fatalf("read goblin cbc telemetry: %v", err)
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
			res := goblin.SolveCompiled(p, goblin.WithTimeLimit(30*time.Second))
			solved++

			if k >= len(py.TrialObjectives) {
				t.Fatalf("%s[%d]: no CBC reference for this trial", c.ID, k)
			}
			want := py.TrialObjectives[k]
			if want == nil {
				if res.Status == goblin.StatusOptimal {
					t.Errorf("%s[%d]: goblin Optimal %.8g, CBC reported no optimum (%s)", c.ID, k, res.Objective, py.Status)
				}
				continue
			}
			if res.Status != goblin.StatusOptimal {
				t.Errorf("%s[%d]: goblin %v, CBC Optimal %.8g", c.ID, k, res.Status, *want)
				continue
			}
			if d := math.Abs(res.Objective-*want) / max(1.0, math.Abs(*want)); d > 1e-5 {
				t.Errorf("%s[%d]: goblin %.8g, CBC %.8g (rel %.1e)", c.ID, k, res.Objective, *want, d)
			}
			if k < len(gb.TrialObjectives) {
				if d := math.Abs(res.Objective-gb.TrialObjectives[k]) / max(1.0, math.Abs(gb.TrialObjectives[k])); d > 1e-5 {
					t.Errorf("%s[%d]: goblin %.8g, goblin_cbc %.8g (rel %.1e)", c.ID, k, res.Objective, gb.TrialObjectives[k], d)
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
	corpusByID := make(map[string]*goblin.CompiledProblem)
	for _, c := range bench.BuildCorpus() {
		switch c.ID {
		case "t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504":
			corpusByID[c.ID] = c.Model.Compile()
		}
	}

	for _, id := range []string{"t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504"} {
		p := corpusByID[id]
		optRes := goblin.SolveCompiled(p)
		if optRes.Status != goblin.StatusOptimal {
			t.Fatalf("[%s] expected Optimal with default options, got %v", id, optRes.Status)
		}
		for _, maxNodes := range []int{1, 2, 5, 20} {
			res := goblin.SolveCompiled(p, goblin.WithCuts(false), goblin.WithMaxNodes(maxNodes))
			if res.Status != goblin.StatusNodeLimit {
				t.Fatalf("[%s] MaxNodes=%d: expected NodeLimit, got %v (obj=%.4f, bestBound=%.4f)",
					id, maxNodes, res.Status, res.Objective, res.BestBound)
			}
			if p.Sense == goblin.Maximize {
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
	m := goblin.NewModel("phase1_iter_limit", goblin.Minimize)
	x := m.AddContVar("x", 0, math.Inf(1))
	y := m.AddContVar("y", 0, math.Inf(1))
	f := m.AddContVar("f", math.Inf(-1), math.Inf(1))
	m.SetObjective(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, 2), goblin.T(f, 1)))
	m.AddConstraint("a", goblin.Ge(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, 1)), 2))
	m.AddConstraint("b", goblin.Ge(goblin.NewExpr(goblin.T(x, 1), goblin.T(y, -1)), 1))
	m.AddConstraint("c", goblin.Ge(goblin.NewExpr(goblin.T(f, 1), goblin.T(x, -1)), -3))
	m.AddConstraint("d", goblin.Le(goblin.NewExpr(goblin.T(f, 1), goblin.T(y, 1)), 10))
	p := m.Compile()

	full := goblin.NewLPSolver(p, nil).Solve()
	if full.Status != goblin.StatusOptimal || math.Abs(full.Objective-1.0) > 1e-6 {
		t.Fatalf("expected Optimal obj=1.0, got status=%v obj=%v", full.Status, full.Objective)
	}

	for _, maxIter := range []int{1, 2, 3} {
		s := goblin.NewLPSolver(p, nil)
		s.MaxIter = maxIter
		res := s.Solve()
		if res.Status != goblin.StatusIterationLimit {
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
		t0 := time.Now()
		res := goblin.SolveCompiled(p, goblin.WithTimeLimit(5*time.Millisecond))
		elapsed := time.Since(t0)
		if res.Status != goblin.StatusTimeLimit && res.Status != goblin.StatusOptimal {
			t.Fatalf("[%s] expected TimeLimit or Optimal, got %v", c.ID, res.Status)
		}
		if elapsed > 50*time.Millisecond {
			t.Fatalf("[%s] TimeLimit=5ms overshot wall-clock budget: elapsed=%v", c.ID, elapsed)
		}
	}
}

func TestModelValidationForeignVarAndNaN(t *testing.T) {
	// 1. Foreign Var handle (both same index 0 and out-of-range index)
	a := goblin.NewModel("a", goblin.Maximize)
	b := goblin.NewModel("b", goblin.Maximize)
	xa := a.AddContVar("x", 0, 10)
	yb0 := b.AddContVar("y0", 0, 10)
	a.SetObjective(goblin.NewExpr(goblin.T(xa, 1), goblin.T(yb0, 100)))
	a.AddConstraint("c", goblin.Le(goblin.NewExpr(goblin.T(xa, 1), goblin.T(yb0, 1)), 3))
	if err := a.Validate(); err == nil {
		t.Fatalf("expected Validate() error on foreign Var handle, got nil")
	}
	if _, err := a.CompileChecked(); err == nil {
		t.Fatalf("expected CompileChecked() error on foreign Var handle, got nil")
	}
	if res := a.Solve(); res.Status != goblin.StatusUndefined {
		t.Fatalf("expected StatusUndefined when solving model with foreign Var, got %v", res.Status)
	}

	// 2. Unowned raw Var(999999) with zero coefficient
	mRaw := goblin.NewModel("raw_var", goblin.Minimize)
	x := mRaw.AddContVar("x", 0, 5)
	mRaw.SetObjective(goblin.NewExpr(goblin.T(x, 1), goblin.T(goblin.Var(999999), 0)))
	if err := mRaw.Validate(); err == nil {
		t.Fatalf("expected Validate() error on unowned raw Var(999999), got nil")
	}
	if res := mRaw.Solve(); res.Status != goblin.StatusUndefined {
		t.Fatalf("expected StatusUndefined on unowned raw Var(999999), got %v", res.Status)
	}

	// 3. NaN coefficient
	mNaN := goblin.NewModel("nan_obj", goblin.Minimize)
	v := mNaN.AddContVar("v", 0, 10)
	mNaN.SetObjective(goblin.NewExpr(goblin.T(v, math.NaN())))
	if err := mNaN.Validate(); err == nil {
		t.Fatalf("expected Validate() error on NaN objective coefficient, got nil")
	}
	if res := mNaN.Solve(); res.Status != goblin.StatusUndefined {
		t.Fatalf("expected StatusUndefined on NaN objective coefficient, got %v", res.Status)
	}

	// 4. Duplicate variable name
	mDup := goblin.NewModel("dup_var", goblin.Minimize)
	mDup.AddContVar("x", 0, 1)
	mDup.AddContVar("x", 0, 1)
	if err := mDup.Validate(); err == nil {
		t.Fatalf("expected Validate() error on duplicate variable name, got nil")
	}
	if res := mDup.Solve(); res.Status != goblin.StatusUndefined {
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
		res := goblin.SolveCompiled(p)
		if res.Status != goblin.StatusOptimal {
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
	// goblin must report StatusNodeLimit (never false StatusOptimal) while returning a valid incumbent.
	mSym := bench.BuildMADRandom("mad_rand_g4_i12", 4, 12, 6, 7003)
	pSym := mSym.Compile()
	resSym := goblin.SolveCompiled(pSym, goblin.WithMaxNodes(200))
	if resSym.Status != goblin.StatusNodeLimit {
		t.Fatalf("[mad_rand_g4_i12] expected StatusNodeLimit at MaxNodes=200, got %v", resSym.Status)
	}
	if len(resSym.Values) == pSym.NumVars {
		bV, rV, iV := pSym.MaxViolation(resSym.Values, true)
		if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
			t.Fatalf("[mad_rand_g4_i12] incumbent violation: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
		}
	}
}
