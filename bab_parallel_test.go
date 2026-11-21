package goblin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func checkMIPPrimalAndBound(t *testing.T, tag string, comp *CompiledProblem, sense Sense, res Result) {
	t.Helper()
	if len(res.Values) != comp.numVars {
		t.Fatalf("%s: len(Values) = %d, want %d", tag, len(res.Values), comp.numVars)
	}
	bV, rV, iV := comp.MaxViolation(res.Values, true)
	if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
		t.Fatalf("%s: primal violation bound=%.3e row=%.3e int=%.3e", tag, bV, rV, iV)
	}
	evalObj := comp.EvaluateObjective(res.Values)
	if math.Abs(evalObj-res.Objective) > 1e-5*max(1.0, math.Abs(res.Objective)) {
		t.Fatalf("%s: reported Objective=%.9f != EvaluateObjective=%.9f", tag, res.Objective, evalObj)
	}
	tol := 1e-5 * max(1.0, math.Abs(res.Objective))
	if sense == Minimize {
		if res.BestBound > res.Objective+tol {
			t.Fatalf("%s: Minimize BestBound=%.9f > Objective=%.9f", tag, res.BestBound, res.Objective)
		}
	} else {
		if res.BestBound < res.Objective-tol {
			t.Fatalf("%s: Maximize BestBound=%.9f < Objective=%.9f", tag, res.BestBound, res.Objective)
		}
	}
}

func TestParallelBranchAndBoundParity(t *testing.T) {
	cases := []struct {
		id     string
		public bool
	}{
		{id: "miplib_lseu", public: true},
		{id: "miplib_p0033", public: true},
		{id: "miplib_p0548", public: true},
		{id: "miplib_p0201", public: true},
		{id: "t3_mkp_200", public: false},
		{id: "t3_lotsizing_504", public: false},
		{id: "t2_mad_rand_g3_i9", public: false},
	}

	workerCounts := []int{1, 2, 4}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var m *Model
			if tc.public {
				m = loadPublicPuLPJSONForTest(t, tc.id)
			} else {
				m = loadJSONModelForTest(t, filepath.Join("testdata", "corpus", tc.id+".json"))
			}
			comp := m.Compile()
			ctx := context.Background()

			var refObj float64
			for _, w := range workerCounts {
				res, err := SolveCompiled(ctx, comp, WithWorkers(w))
				if err != nil {
					t.Fatalf("workers=%d SolveCompiled err: %v", w, err)
				}
				if res.Status != StatusOptimal {
					t.Fatalf("workers=%d Status = %v, want StatusOptimal", w, res.Status)
				}
				checkMIPPrimalAndBound(t, tc.id, comp, m.Sense(), res)
				if w == 1 {
					refObj = res.Objective
				} else {
					tol := 1e-5 * max(1.0, math.Abs(refObj))
					if math.Abs(res.Objective-refObj) > tol {
						t.Fatalf("workers=%d Objective=%.9f != ref(w=1)=%.9f (tol=%.3e)",
							w, res.Objective, refObj, tol)
					}
				}
			}
		})
	}
}

func buildHardMKPModel(n, m int) *Model {
	mdl := NewModel("hard_mkp", Maximize)
	vars := make([]Var, n)
	objTerms := make([]Term, n)
	for j := range n {
		vars[j] = mdl.AddVar(fmt.Sprintf("x%d", j), 0, 1, Binary)
		objTerms[j] = Term{Var: vars[j], Coeff: float64(100 + ((j*37 + 13) % 89))}
	}
	mdl.SetObjective(Expr{Terms: objTerms})
	for i := range m {
		terms := make([]Term, n)
		rhs := 0.0
		for j := range n {
			w := float64(10 + ((j*19 + i*41 + 7) % 43))
			terms[j] = Term{Var: vars[j], Coeff: w}
			rhs += w
		}
		mdl.AddConstraint(fmt.Sprintf("cap%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: LessEqual,
			RHS:   0.48 * rhs,
		})
	}
	return mdl
}

func TestParallelBranchAndBoundEdgeAndLimits(t *testing.T) {
	ctx := context.Background()

	t.Run("infeasible_mip", func(t *testing.T) {
		m := NewModel("infeas", Minimize)
		x := m.AddVar("x", 0, 1, Binary)
		y := m.AddVar("y", 0, 1, Binary)
		m.SetObjective(Expr{Terms: []Term{{Var: x, Coeff: 1}, {Var: y, Coeff: 2}}})
		m.AddConstraint("c1", Constraint{Expr: Expr{Terms: []Term{{Var: x, Coeff: 1}, {Var: y, Coeff: 1}}}, Sense: LessEqual, RHS: 1})
		m.AddConstraint("c2", Constraint{Expr: Expr{Terms: []Term{{Var: x, Coeff: 1}, {Var: y, Coeff: 1}}}, Sense: GreaterEqual, RHS: 3})

		res, err := m.Solve(ctx, WithWorkers(4))
		if err != nil {
			t.Fatalf("Solve err: %v", err)
		}
		if res.Status != StatusInfeasible {
			t.Fatalf("Status = %v, want StatusInfeasible", res.Status)
		}
	})

	t.Run("unbounded_mip", func(t *testing.T) {
		m := NewModel("unb", Maximize)
		x := m.AddVar("x", 0, math.Inf(1), Continuous)
		b := m.AddVar("b", 0, 1, Binary)
		m.SetObjective(Expr{Terms: []Term{{Var: x, Coeff: 1}}})
		m.AddConstraint("link", Constraint{Expr: Expr{Terms: []Term{{Var: x, Coeff: 1}, {Var: b, Coeff: -1}}}, Sense: GreaterEqual, RHS: 0})

		res, err := m.Solve(ctx, WithWorkers(4))
		if err != nil {
			t.Fatalf("Solve err: %v", err)
		}
		if res.Status != StatusUnbounded {
			t.Fatalf("Status = %v, want StatusUnbounded", res.Status)
		}
	})

	t.Run("pure_continuous_lp", func(t *testing.T) {
		m := NewModel("lp", Minimize)
		x := m.AddVar("x", 0, math.Inf(1), Continuous)
		y := m.AddVar("y", 0, math.Inf(1), Continuous)
		m.SetObjective(Expr{Terms: []Term{{Var: x, Coeff: 2}, {Var: y, Coeff: 3}}})
		m.AddConstraint("req", Constraint{Expr: Expr{Terms: []Term{{Var: x, Coeff: 1}, {Var: y, Coeff: 1}}}, Sense: GreaterEqual, RHS: 4})

		res, err := m.Solve(ctx, WithWorkers(4))
		if err != nil {
			t.Fatalf("Solve err: %v", err)
		}
		if res.Status != StatusOptimal || math.Abs(res.Objective-8.0) > 1e-6 {
			t.Fatalf("Status=%v Obj=%.6f, want Optimal 8.0", res.Status, res.Objective)
		}
	})

	t.Run("max_nodes_limit", func(t *testing.T) {
		m := loadPublicPuLPJSONForTest(t, "miplib_p0201")
		comp := m.Compile()
		res, err := SolveCompiled(ctx, comp, WithWorkers(4), WithMaxNodes(20))
		if err != nil {
			t.Fatalf("SolveCompiled err: %v", err)
		}
		if res.Status != StatusNodeLimit && res.Status != StatusOptimal {
			t.Fatalf("Status = %v, want StatusNodeLimit or StatusOptimal", res.Status)
		}
		if res.Nodes > 20 {
			t.Fatalf("Nodes = %d > MaxNodes(20)", res.Nodes)
		}
		if math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0) {
			t.Fatalf("invalid BestBound = %v", res.BestBound)
		}
		if len(res.Values) > 0 {
			checkMIPPrimalAndBound(t, "max_nodes", comp, m.Sense(), res)
		}
	})

	t.Run("max_iter_limit", func(t *testing.T) {
		m := loadPublicPuLPJSONForTest(t, "miplib_p0201")
		comp := m.Compile()
		res, err := SolveCompiled(ctx, comp, WithWorkers(4), WithMaxIter(500))
		if err != nil {
			t.Fatalf("SolveCompiled err: %v", err)
		}
		if res.Status != StatusIterationLimit && res.Status != StatusOptimal {
			t.Fatalf("Status = %v, want StatusIterationLimit or StatusOptimal", res.Status)
		}
		if math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0) {
			t.Fatalf("invalid BestBound = %v", res.BestBound)
		}
	})

	t.Run("time_limit", func(t *testing.T) {
		m := buildHardMKPModel(80, 12)
		res, err := m.Solve(ctx, WithWorkers(4), WithTimeLimit(15*time.Millisecond), WithMaxNodes(0))
		if err != nil {
			t.Fatalf("Solve err: %v", err)
		}
		if res.Status != StatusTimeLimit && res.Status != StatusOptimal {
			t.Fatalf("Status = %v, want StatusTimeLimit or StatusOptimal", res.Status)
		}
		if math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0) {
			t.Fatalf("invalid BestBound = %v", res.BestBound)
		}
	})

	t.Run("context_cancel_and_no_goroutine_leak", func(t *testing.T) {
		baseG := runtime.NumGoroutine()
		m := buildHardMKPModel(100, 16)
		comp := m.Compile()

		cctx, cancel := context.WithTimeout(ctx, 15*time.Millisecond)
		defer cancel()
		_, err := SolveCompiled(cctx, comp, WithWorkers(4), WithMaxNodes(0))
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected ctx err: %v", err)
		}

		// Also test already-canceled context.
		canceledCtx, cancelNow := context.WithCancel(ctx)
		cancelNow()
		_, err2 := SolveCompiled(canceledCtx, comp, WithWorkers(4))
		if !errors.Is(err2, context.Canceled) {
			t.Fatalf("canceled ctx err = %v, want context.Canceled", err2)
		}

		runtime.Gosched()
		time.Sleep(25 * time.Millisecond)
		afterG := runtime.NumGoroutine()
		if afterG > baseG {
			t.Fatalf("goroutine leak detected: base=%d after=%d", baseG, afterG)
		}
	})

	t.Run("rel_gap_tol", func(t *testing.T) {
		m := loadPublicPuLPJSONForTest(t, "miplib_p0201")
		comp := m.Compile()
		for _, tol := range []float64{0.05, 1.5} {
			res, err := SolveCompiled(ctx, comp, WithWorkers(4), WithRelGapTol(tol))
			if err != nil {
				t.Fatalf("RelGapTol=%v Solve err: %v", tol, err)
			}
			if res.Status != StatusOptimal {
				t.Fatalf("RelGapTol=%v Status = %v, want StatusOptimal", tol, res.Status)
			}
			checkMIPPrimalAndBound(t, fmt.Sprintf("relgap_%.2f", tol), comp, m.Sense(), res)
		}
	})
}

func TestSingleThreadedDeterminism(t *testing.T) {
	cases := []struct {
		id     string
		public bool
	}{
		{id: "miplib_p0033", public: true},
		{id: "t3_mkp_200", public: false},
	}

	ctx := context.Background()
	for _, tc := range cases {
		var m *Model
		if tc.public {
			m = loadPublicPuLPJSONForTest(t, tc.id)
		} else {
			m = loadJSONModelForTest(t, filepath.Join("testdata", "corpus", tc.id+".json"))
		}
		comp := m.Compile()

		for _, opts := range [][]Option{nil, {WithWorkers(1)}} {
			r1, err1 := SolveCompiled(ctx, comp, opts...)
			r2, err2 := SolveCompiled(ctx, comp, opts...)
			if err1 != nil || err2 != nil {
				t.Fatalf("%s solve err1=%v err2=%v", tc.id, err1, err2)
			}
			if r1.Status != r2.Status ||
				r1.Objective != r2.Objective ||
				r1.BestBound != r2.BestBound ||
				r1.Nodes != r2.Nodes ||
				r1.Iterations != r2.Iterations ||
				r1.CutsAdded != r2.CutsAdded {
				t.Fatalf("%s non-deterministic single-threaded solve: r1=%+v r2=%+v", tc.id, r1, r2)
			}
		}
	}
}
