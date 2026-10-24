package goblin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
)

func TestPuLPCanonicalLPsAndMIPs(t *testing.T) {
	backends := []struct {
		name string
		opt  gb.Option
	}{
		{"PureGo", gb.WithPureGo()},
		{"PureGo_NoSIMD", gb.WithoutSIMD()},
	}
	if gb.CBCAvailable() {
		backends = append(backends, struct {
			name string
			opt  gb.Option
		}{"CBC", gb.WithCBC()})
	}

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			buildCanonical := func(name string, intZ bool, cx, cy, cz, rhs3 float64) (*gb.Model, gb.Var, gb.Var, gb.Var) {
				m := gb.NewModel(name, gb.Minimize)
				x := m.AddContVar("x", 0, 4)
				y := m.AddContVar("y", -1, 1)
				var z gb.Var
				if intZ {
					z = m.AddIntVar("z", 0, math.Inf(1))
				} else {
					z = m.AddContVar("z", 0, math.Inf(1))
				}
				m.SetObjective(gb.NewExpr(gb.T(x, cx), gb.T(y, cy), gb.T(z, cz)))
				m.AddConstraint("c1", gb.Le(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 5))
				m.AddConstraint("c2", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(z, 1)), 10))
				m.AddConstraint("c3", gb.Eq(gb.NewExpr(gb.T(y, -1), gb.T(z, 1)), rhs3))
				return m, x, y, z
			}

			// 1. std_lp: x=4, y=-1, z=6, obj=54.0
			{
				m, _, _, _ := buildCanonical("std_lp", false, 1, 4, 9, 7)
				res := mustSolve(t, m, b.opt)
				if res.Status != gb.StatusOptimal || math.Abs(res.Objective-54.0) > 1e-6 {
					t.Fatalf("std_lp failed: status=%v obj=%v", res.Status, res.Objective)
				}
			}

			// 2. std_mip: x=3, y=-0.5, z=7 (integer), obj=64.0
			{
				m, x, y, z := buildCanonical("std_mip", true, 1, 4, 9, 7.5)
				res := mustSolve(t, m, b.opt)
				if res.Status != gb.StatusOptimal || math.Abs(res.Objective-64.0) > 1e-6 {
					t.Fatalf("std_mip failed: status=%v obj=%v values=%v", res.Status, res.Objective, res.Values)
				}
				if math.Abs(res.Value(x)-3.0) > 1e-6 || math.Abs(res.Value(y)-(-0.5)) > 1e-6 || math.Abs(res.Value(z)-7.0) > 1e-6 {
					t.Fatalf("std_mip unexpected primal values: %v", res.Values)
				}
			}

			// 3. test_mip_floats_objective: obj=64.95
			{
				m, _, _, _ := buildCanonical("mip_float_obj", true, 1.1, 4.1, 9.1, 7.5)
				res := mustSolve(t, m, b.opt)
				if res.Status != gb.StatusOptimal || math.Abs(res.Objective-64.95) > 1e-6 {
					t.Fatalf("mip_float_obj failed: status=%v obj=%v", res.Status, res.Objective)
				}
			}

			// 4. test_integer_infeasible: x+y <= 5.2, x+z >= 10.3, -y+z == 7.4 with x,y,z integer
			{
				m := gb.NewModel("integer_infeasible", gb.Minimize)
				x := m.AddIntVar("x", 0, 4)
				y := m.AddIntVar("y", -1, 1)
				z := m.AddIntVar("z", 0, 10)
				m.SetObjective(gb.NewExpr(gb.T(x, 1), gb.T(y, 4), gb.T(z, 9)))
				m.AddConstraint("c1", gb.Le(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 5.2))
				m.AddConstraint("c2", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(z, 1)), 10.3))
				m.AddConstraint("c3", gb.Eq(gb.NewExpr(gb.T(y, -1), gb.T(z, 1)), 7.4))

				res := mustSolve(t, m, b.opt)
				if res.Status != gb.StatusInfeasible {
					t.Fatalf("expected Infeasible on integer_infeasible, got %v", res.Status)
				}
			}
		})
	}
}

func TestUnifiedOptionsAndBackends(t *testing.T) {
	// Verify that solver flags compose cleanly. PureGo, PureGo WithoutSIMD, and CBC
	// must agree on t3_sched_1000 where AVX2 vector kernels run.
	var target *gb.Model
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

	resSIMD := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithSIMD(true), gb.WithTimeLimit(10*time.Second))
	resScalar := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithoutSIMD(), gb.WithTimeLimit(10*time.Second))
	if resSIMD.Status != gb.StatusOptimal || resScalar.Status != gb.StatusOptimal {
		t.Fatalf("expected Optimal, got simd=%v scalar=%v", resSIMD.Status, resScalar.Status)
	}
	if math.Abs(resSIMD.Objective-resScalar.Objective) > 1e-6 {
		t.Fatalf("objective mismatch between WithSIMD(true) and WithoutSIMD(): %.8f vs %.8f",
			resSIMD.Objective, resScalar.Objective)
	}

	if gb.CBCAvailable() {
		resCBC := mustSolveCompiled(t, p, gb.WithCBC(), gb.WithTimeLimit(10*time.Second))
		if resCBC.Status != gb.StatusOptimal {
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
			res := mustSolveCompiled(t, p, gb.WithTimeLimit(15*time.Second))
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
		cbcRef := goblinRef[c.ID]
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
			res := mustSolveCompiled(t, p, gb.WithTimeLimit(30*time.Second))
			solved++

			if k >= len(py.TrialObjectives) {
				t.Fatalf("%s[%d]: no CBC reference for this trial", c.ID, k)
			}
			want := py.TrialObjectives[k]
			if want == nil {
				if res.Status == gb.StatusOptimal {
					t.Errorf("%s[%d]: goblin Optimal %.8g, CBC reported no optimum (%s)", c.ID, k, res.Objective, py.Status)
				}
				continue
			}
			if res.Status != gb.StatusOptimal {
				t.Errorf("%s[%d]: goblin %v, CBC Optimal %.8g", c.ID, k, res.Status, *want)
				continue
			}
			if d := math.Abs(res.Objective-*want) / max(1.0, math.Abs(*want)); d > 1e-5 {
				t.Errorf("%s[%d]: goblin %.8g, CBC %.8g (rel %.1e)", c.ID, k, res.Objective, *want, d)
			}
			if k < len(cbcRef.TrialObjectives) {
				if d := math.Abs(res.Objective-cbcRef.TrialObjectives[k]) / max(1.0, math.Abs(cbcRef.TrialObjectives[k])); d > 1e-5 {
					t.Errorf("%s[%d]: goblin %.8g, goblin_cbc %.8g (rel %.1e)", c.ID, k, res.Objective, cbcRef.TrialObjectives[k], d)
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
	corpusByID := make(map[string]*gb.CompiledProblem)
	for _, c := range bench.BuildCorpus() {
		switch c.ID {
		case "t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504":
			corpusByID[c.ID] = c.Model.Compile()
		}
	}

	for _, id := range []string{"t3_mkp_200", "t3_sched_1000", "t3_lotsizing_504"} {
		p := corpusByID[id]
		optRes := mustSolveCompiled(t, p)
		if optRes.Status != gb.StatusOptimal {
			t.Fatalf("[%s] expected Optimal with default options, got %v", id, optRes.Status)
		}
		for _, maxNodes := range []int{1, 2, 5, 20} {
			res := mustSolveCompiled(t, p, gb.WithCuts(false), gb.WithMaxNodes(maxNodes))
			if res.Status != gb.StatusNodeLimit {
				t.Fatalf("[%s] MaxNodes=%d: expected NodeLimit, got %v (obj=%.4f, bestBound=%.4f)",
					id, maxNodes, res.Status, res.Objective, res.BestBound)
			}
			if p.Sense == gb.Maximize {
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
	m := gb.NewModel("phase1_iter_limit", gb.Minimize)
	x := m.AddContVar("x", 0, math.Inf(1))
	y := m.AddContVar("y", 0, math.Inf(1))
	f := m.AddContVar("f", math.Inf(-1), math.Inf(1))
	m.SetObjective(gb.NewExpr(gb.T(x, 1), gb.T(y, 2), gb.T(f, 1)))
	m.AddConstraint("a", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 2))
	m.AddConstraint("b", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, -1)), 1))
	m.AddConstraint("c", gb.Ge(gb.NewExpr(gb.T(f, 1), gb.T(x, -1)), -3))
	m.AddConstraint("d", gb.Le(gb.NewExpr(gb.T(f, 1), gb.T(y, 1)), 10))
	p := m.Compile()

	full := gb.NewLPSolver(p, nil).Solve()
	if full.Status != gb.StatusOptimal || math.Abs(full.Objective-1.0) > 1e-6 {
		t.Fatalf("expected Optimal obj=1.0, got status=%v obj=%v", full.Status, full.Objective)
	}

	for _, maxIter := range []int{1, 2, 3} {
		s := gb.NewLPSolver(p, nil)
		s.MaxIter = maxIter
		res := s.Solve()
		if res.Status != gb.StatusIterationLimit {
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
		res, err := gb.SolveCompiled(t.Context(), p, gb.WithTimeLimit(5*time.Millisecond))
		elapsed := time.Since(t0)
		if err != nil {
			t.Fatalf("[%s] unexpected error on WithTimeLimit: %v", c.ID, err)
		}
		if res.Status != gb.StatusTimeLimit && res.Status != gb.StatusOptimal {
			t.Fatalf("[%s] expected TimeLimit or Optimal, got %v", c.ID, res.Status)
		}
		if elapsed > 50*time.Millisecond {
			t.Fatalf("[%s] TimeLimit=5ms overshot wall-clock budget: elapsed=%v", c.ID, elapsed)
		}
	}

	// Verify that WithTimeLimit on a MILP where root heuristics find a feasible incumbent
	// returns (res, nil) with StatusTimeLimit and preserves the best incumbent and bound.
	mSym := bench.BuildMADRandom("mad_rand_g4_i12_tl", 4, 12, 6, 7003)
	pSym := mSym.Compile()
	resSym, err := gb.SolveCompiled(t.Context(), pSym, gb.WithTimeLimit(15*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error on WithTimeLimit: %v", err)
	}
	if resSym.Status != gb.StatusTimeLimit && resSym.Status != gb.StatusOptimal {
		t.Fatalf("expected TimeLimit or Optimal on mad_rand_g4_i12_tl, got %v", resSym.Status)
	}
	if len(resSym.Values) != pSym.NumVars {
		t.Fatalf("expected best incumbent Values (len=%d) on TimeLimit, got len=%d", pSym.NumVars, len(resSym.Values))
	}
	bV, rV, iV := pSym.MaxViolation(resSym.Values, true)
	if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
		t.Fatalf("TimeLimit incumbent violation: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
	}
}

func TestContextCancellation(t *testing.T) {
	var smallModel, largeMIPModel *gb.Model
	for _, c := range bench.BuildCorpus() {
		switch c.ID {
		case "t1_std_lp":
			smallModel = c.Model
		case "t3_mad_g4_i1000":
			largeMIPModel = c.Model
		}
	}
	if smallModel == nil || largeMIPModel == nil {
		t.Fatal("missing required corpus models for context cancellation test")
	}
	smallLP := smallModel.Compile()
	largeMIP := largeMIPModel.Compile()

	backends := []struct {
		name string
		opt  gb.Option
	}{
		{"PureGo", gb.WithPureGo()},
	}
	if gb.CBCAvailable() {
		backends = append(backends, struct {
			name string
			opt  gb.Option
		}{"CBC", gb.WithCBC()})
	}

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			// 1. Pre-canceled context returns (Result{}, context.Canceled) immediately with 0 iterations.
			canceledCtx, cancelNow := context.WithCancel(context.Background())
			cancelNow()

			if res, err := smallModel.Solve(canceledCtx, b.opt); !errors.Is(err, context.Canceled) || res.Status != gb.StatusNotSolved || len(res.Values) != 0 || res.Iterations != 0 {
				t.Fatalf("[%s] pre-canceled Solve on LP: err=%v status=%v iters=%d, want context.Canceled and empty Result",
					b.name, err, res.Status, res.Iterations)
			}
			if res, err := gb.SolveCompiled(canceledCtx, largeMIP, b.opt); !errors.Is(err, context.Canceled) || res.Status != gb.StatusNotSolved || len(res.Values) != 0 || res.Iterations != 0 {
				t.Fatalf("[%s] pre-canceled SolveCompiled on MIP: err=%v status=%v iters=%d, want context.Canceled and empty Result",
					b.name, err, res.Status, res.Iterations)
			}

			// 2. Pre-expired deadline context returns (Result{}, context.DeadlineExceeded).
			expiredCtx, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancelExpired()
			if res, err := gb.SolveCompiled(expiredCtx, largeMIP, b.opt); !errors.Is(err, context.DeadlineExceeded) || res.Status != gb.StatusNotSolved || len(res.Values) != 0 {
				t.Fatalf("[%s] pre-expired SolveCompiled on MIP: err=%v status=%v, want context.DeadlineExceeded and empty Result",
					b.name, err, res.Status)
			}

			// 3. Uncanceled context solves to optimality identically to background.
			activeCtx := t.Context()
			if res, err := gb.SolveCompiled(activeCtx, smallLP, b.opt); err != nil || res.Status != gb.StatusOptimal {
				t.Fatalf("[%s] uncanceled SolveCompiled on LP: got err=%v status=%v, want Optimal", b.name, err, res.Status)
			}

			// 4. Asynchronous cancellation mid-solve halts promptly with context.Canceled and discards partial results.
			asyncCtx, cancelAsync := context.WithCancel(context.Background())
			go func() {
				time.Sleep(5 * time.Millisecond)
				cancelAsync()
			}()
			t0 := time.Now()
			resAsync, errAsync := gb.SolveCompiled(asyncCtx, largeMIP, b.opt)
			elapsed := time.Since(t0)
			if !errors.Is(errAsync, context.Canceled) {
				t.Fatalf("[%s] async cancel on t3_mad_g4_i1000: got err=%v status=%v, want context.Canceled", b.name, errAsync, resAsync.Status)
			}
			if resAsync.Status != gb.StatusNotSolved || len(resAsync.Values) != 0 || resAsync.Objective != 0 {
				t.Fatalf("[%s] async cancel did not discard partial result: %+v", b.name, resAsync)
			}
			if elapsed > 100*time.Millisecond {
				t.Fatalf("[%s] async cancel took %v (want <= 100ms)", b.name, elapsed)
			}

			// 5. Context deadline via context.WithTimeout halts promptly with context.DeadlineExceeded and discards partial results.
			timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Millisecond)
			defer cancelTimeout()
			t1 := time.Now()
			resTimeout, errTimeout := gb.SolveCompiled(timeoutCtx, largeMIP, b.opt)
			elapsedTimeout := time.Since(t1)
			if !errors.Is(errTimeout, context.DeadlineExceeded) {
				t.Fatalf("[%s] timeout context on t3_mad_g4_i1000: got err=%v status=%v, want context.DeadlineExceeded", b.name, errTimeout, resTimeout.Status)
			}
			if resTimeout.Status != gb.StatusNotSolved || len(resTimeout.Values) != 0 || resTimeout.Objective != 0 {
				t.Fatalf("[%s] timeout context did not discard partial result: %+v", b.name, resTimeout)
			}
			if elapsedTimeout > 100*time.Millisecond {
				t.Fatalf("[%s] timeout context took %v (want <= 100ms)", b.name, elapsedTimeout)
			}

			// 6. Coexistence: WithTimeLimit shorter than ctx timeout returns (res, nil) with StatusTimeLimit.
			longCtx, cancelLong := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelLong()
			resTL, errTL := gb.SolveCompiled(longCtx, largeMIP, b.opt, gb.WithTimeLimit(5*time.Millisecond))
			if errTL != nil || resTL.Status != gb.StatusTimeLimit {
				t.Fatalf("[%s] WithTimeLimit(5ms) under 5s context: got err=%v status=%v, want nil and StatusTimeLimit",
					b.name, errTL, resTL.Status)
			}
		})
	}
}

func TestModelValidationForeignVarAndNaN(t *testing.T) {
	// 0. Nil CompiledProblem returns ErrNilProblem and StatusUndefined.
	if res, err := gb.SolveCompiled(t.Context(), nil); !errors.Is(err, gb.ErrNilProblem) || res.Status != gb.StatusUndefined {
		t.Fatalf("expected (StatusUndefined, ErrNilProblem) on nil CompiledProblem, got (%v, %v)", res.Status, err)
	}

	// 1. Foreign Var handle (both same index 0 and out-of-range index)
	a := gb.NewModel("a", gb.Maximize)
	b := gb.NewModel("b", gb.Maximize)
	xa := a.AddContVar("x", 0, 10)
	yb0 := b.AddContVar("y0", 0, 10)
	a.SetObjective(gb.NewExpr(gb.T(xa, 1), gb.T(yb0, 100)))
	a.AddConstraint("c", gb.Le(gb.NewExpr(gb.T(xa, 1), gb.T(yb0, 1)), 3))
	if err := a.Validate(); err == nil {
		t.Fatalf("expected Validate() error on foreign Var handle, got nil")
	}
	if _, err := a.CompileChecked(); err == nil {
		t.Fatalf("expected CompileChecked() error on foreign Var handle, got nil")
	}
	if res, err := a.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("expected StatusUndefined and error when solving model with foreign Var, got (%v, %v)", res.Status, err)
	}

	// 2. Unowned raw Var(999999) with zero coefficient
	mRaw := gb.NewModel("raw_var", gb.Minimize)
	x := mRaw.AddContVar("x", 0, 5)
	mRaw.SetObjective(gb.NewExpr(gb.T(x, 1), gb.T(gb.Var(999999), 0)))
	if err := mRaw.Validate(); err == nil {
		t.Fatalf("expected Validate() error on unowned raw Var(999999), got nil")
	}
	if res, err := mRaw.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("expected StatusUndefined and error on unowned raw Var(999999), got (%v, %v)", res.Status, err)
	}

	// 3. NaN coefficient
	mNaN := gb.NewModel("nan_obj", gb.Minimize)
	v := mNaN.AddContVar("v", 0, 10)
	mNaN.SetObjective(gb.NewExpr(gb.T(v, math.NaN())))
	if err := mNaN.Validate(); err == nil {
		t.Fatalf("expected Validate() error on NaN objective coefficient, got nil")
	}
	if res, err := mNaN.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("expected StatusUndefined and error on NaN objective coefficient, got (%v, %v)", res.Status, err)
	}

	// 4. Duplicate variable name
	mDup := gb.NewModel("dup_var", gb.Minimize)
	mDup.AddContVar("x", 0, 1)
	mDup.AddContVar("x", 0, 1)
	if err := mDup.Validate(); err == nil {
		t.Fatalf("expected Validate() error on duplicate variable name, got nil")
	}
	if res, err := mDup.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("expected StatusUndefined and error on duplicate variable name, got (%v, %v)", res.Status, err)
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
		res := mustSolveCompiled(t, p)
		if res.Status != gb.StatusOptimal {
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
	resSym := mustSolveCompiled(t, pSym, gb.WithMaxNodes(200))
	if resSym.Status != gb.StatusNodeLimit {
		t.Fatalf("[mad_rand_g4_i12] expected StatusNodeLimit at MaxNodes=200, got %v", resSym.Status)
	}
	if len(resSym.Values) == pSym.NumVars {
		bV, rV, iV := pSym.MaxViolation(resSym.Values, true)
		if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
			t.Fatalf("[mad_rand_g4_i12] incumbent violation: bound=%.2e row=%.2e int=%.2e", bV, rV, iV)
		}
	}
}

func TestAssignCorpusSciPyParity(t *testing.T) {
	rawRef, err := os.ReadFile("bench/results/scipy_assign_baseline.json")
	if err != nil {
		t.Fatalf("failed to read bench/results/scipy_assign_baseline.json: %v", err)
	}
	var scipyRef map[string]bench.SciPyAssignEntry
	if err := json.Unmarshal(rawRef, &scipyRef); err != nil {
		t.Fatalf("failed to unmarshal scipy_assign_baseline.json: %v", err)
	}

	cases := bench.BuildAssignCorpus()
	for _, ac := range cases {
		ref, ok := scipyRef[ac.ID]
		if !ok {
			t.Fatalf("missing %q in scipy_assign_baseline.json", ac.ID)
		}
		if len(ref.TrialObjectives) != ac.Trials {
			t.Fatalf("[%s] expected %d trial objectives, got %d", ac.ID, ac.Trials, len(ref.TrialObjectives))
		}
		for k := range ac.Trials {
			mat := bench.GenerateAssignMatrix(ac, k)
			if k == 0 {
				gotSHA := bench.MatrixSHA256(mat)
				if gotSHA != ac.Trial0SHA {
					t.Fatalf("[%s] Trial 0 SHA256 mismatch: got %s, want %s", ac.ID, gotSHA, ac.Trial0SHA)
				}
			}
			wantObj := ref.TrialObjectives[k]
			for _, disableSIMD := range []bool{false, true} {
				var opt assign.Option
				if disableSIMD {
					opt = assign.WithoutSIMD()
				} else {
					opt = assign.WithSIMD(true)
				}
				res, err := assign.LinearSumFlat(t.Context(), ac.Rows, ac.Cols, mat, ac.Sense, opt)
				if err != nil {
					t.Fatalf("[%s trial %d disableSIMD=%v] unexpected error: %v", ac.ID, k, disableSIMD, err)
				}
				rel := math.Abs(res.Objective-wantObj) / max(1.0, math.Abs(wantObj))
				if rel > 1e-9 {
					t.Fatalf("[%s trial %d disableSIMD=%v] objective mismatch: got %.10f, SciPy %.10f (rel=%.2e)",
						ac.ID, k, disableSIMD, res.Objective, wantObj, rel)
				}
			}
		}
	}
}
