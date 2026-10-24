package goblin_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

// optionVariants exercises the bare branch-and-bound as well as the full setup:
// primal heuristics previously masked the node-discard defect, so correctness must not
// depend on them.
func optionVariants() map[string][]gb.Option {
	return map[string][]gb.Option{
		"default":          {},
		"no-heuristics":    {gb.WithHeuristics(false)},
		"bare":             {gb.WithHeuristics(false), gb.WithCuts(false)},
		"bare-no-presolve": {gb.WithHeuristics(false), gb.WithCuts(false), gb.WithPresolve(false)},
	}
}

func mustSolve(tb testing.TB, m *gb.Model, opts ...gb.Option) gb.Result {
	tb.Helper()
	res, err := m.Solve(tb.Context(), opts...)
	if err != nil {
		tb.Fatalf("unexpected Solve error: %v", err)
	}
	return res
}

func mustSolveCompiled(tb testing.TB, p *gb.CompiledProblem, opts ...gb.Option) gb.Result {
	tb.Helper()
	res, err := gb.SolveCompiled(tb.Context(), p, opts...)
	if err != nil {
		tb.Fatalf("unexpected SolveCompiled error: %v", err)
	}
	return res
}

func assertOptimal(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result, want float64) {
	t.Helper()
	if res.Status != gb.StatusOptimal {
		t.Fatalf("%s: status=%v (obj=%.6f bound=%.6f nodes=%d), want Optimal %.6f",
			label, res.Status, res.Objective, res.BestBound, res.Nodes, want)
	}
	if rel := math.Abs(res.Objective-want) / max(1.0, math.Abs(want)); rel > 1e-6 {
		t.Fatalf("%s: objective=%.8f, want %.8f (rel=%.2e)", label, res.Objective, want, rel)
	}
	bV, rV, iV := p.MaxViolation(res.Values, true)
	if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
		t.Fatalf("%s: solution violates the model: bound=%.2e row=%.2e int=%.2e", label, bV, rV, iV)
	}
	// An incumbent may violate rows by up to the primal tolerance, so its objective can
	// undercut the LP bound by a comparable amount; allow the default gap threshold.
	boundTol := 1e-6 * max(1.0, math.Abs(res.Objective))
	if p.Sense == gb.Minimize && res.BestBound > res.Objective+boundTol ||
		p.Sense == gb.Maximize && res.BestBound < res.Objective-boundTol {
		t.Fatalf("%s: BestBound %.8f is not a valid bound for objective %.8f", label, res.BestBound, res.Objective)
	}
}

// TestBigMNearIntegralRoot: the root LP is integral within 1e-6 (y1 = 5e-7) but rounding
// y1 to 0 violates the demand row by 0.05. The solver previously returned NumericalError.
func TestBigMNearIntegralRoot(t *testing.T) {
	m := gb.NewModel("bigm_root", gb.Minimize)
	y1 := m.AddBinaryVar("y1")
	y2 := m.AddBinaryVar("y2")
	x1 := m.AddContVar("x1", 0, math.Inf(1))
	x2 := m.AddContVar("x2", 0, math.Inf(1))
	m.SetObjective(gb.NewExpr(gb.T(y1, 10), gb.T(y2, 12), gb.T(x1, 1), gb.T(x2, 1)))
	m.AddConstraint("dem", gb.Ge(gb.NewExpr(gb.T(x1, 1), gb.T(x2, 1)), 0.05))
	m.AddConstraint("vub1", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(y1, -1e5)), 0))
	m.AddConstraint("vub2", gb.Le(gb.NewExpr(gb.T(x2, 1), gb.T(y2, -1e5)), 0))
	p := m.Compile()
	for name, o := range optionVariants() {
		assertOptimal(t, name, p, mustSolveCompiled(t, p, o...), 10.05)
	}
}

// TestBigMUnderBranching embeds the near-integral big-M structure under a fractional
// knapsack so that the search must branch. Nodes whose LP is integral within tolerance but
// whose rounding is infeasible were previously discarded, which produced a false Infeasible.
func TestBigMUnderBranching(t *testing.T) {
	m := gb.NewModel("bigm_node", gb.Minimize)
	y1 := m.AddBinaryVar("y1")
	y2 := m.AddBinaryVar("y2")
	x1 := m.AddContVar("x1", 0, math.Inf(1))
	x2 := m.AddContVar("x2", 0, math.Inf(1))
	w := []float64{7, 5, 4, 3, 2}
	val := []float64{9, 6, 5, 3, 2.5}
	obj := gb.NewExpr(gb.T(y1, 10), gb.T(y2, 12), gb.T(x1, 1), gb.T(x2, 1))
	kn := gb.NewExpr()
	for i := range w {
		z := m.AddBinaryVar(fmt.Sprintf("z%d", i))
		obj = obj.AddTerm(z, -val[i])
		kn = kn.AddTerm(z, w[i])
	}
	m.SetObjective(obj)
	m.AddConstraint("knap", gb.Le(kn, 10.5))
	m.AddConstraint("dem", gb.Ge(gb.NewExpr(gb.T(x1, 1), gb.T(x2, 1)), 0.05))
	m.AddConstraint("vub1", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(y1, -1e5)), 0))
	m.AddConstraint("vub2", gb.Le(gb.NewExpr(gb.T(x2, 1), gb.T(y2, -1e5)), 0))
	p := m.Compile()

	// Ground truth by brute-force search: open the cheaper depot, pick the best knapsack subset.
	best := math.Inf(1)
	for mask := range 1 << len(w) {
		wt, o := 0.0, 0.0
		for i := range w {
			if mask>>i&1 == 1 {
				wt += w[i]
				o -= val[i]
			}
		}
		if wt <= 10.5 {
			best = min(best, o+10+0.05)
		}
	}
	for name, o := range optionVariants() {
		assertOptimal(t, name, p, mustSolveCompiled(t, p, o...), best)
	}
}

// buildFixedCharge builds a capacitated-by-big-M depot siting model with small demands.
func buildFixedCharge(seed uint64) (m *gb.Model, y []gb.Var, fixed []float64) {
	r := rand.New(rand.NewPCG(seed, 2026))
	nf, nc := 3+r.IntN(3), 3+r.IntN(5)
	bigM := []float64{1e4, 1e5, 1e6}[r.IntN(3)]
	demands := []float64{0.01, 0.05, 0.2, 1, 3}
	m = gb.NewModel(fmt.Sprintf("fc_%d", seed), gb.Minimize)
	y = make([]gb.Var, nf)
	fixed = make([]float64, nf)
	x := make([][]gb.Var, nf)
	obj := gb.NewExpr()
	for i := range nf {
		y[i] = m.AddBinaryVar(fmt.Sprintf("y%d", i))
		fixed[i] = float64(5 + r.IntN(36))
		obj = obj.AddTerm(y[i], fixed[i])
		x[i] = make([]gb.Var, nc)
		for j := range nc {
			x[i][j] = m.AddContVar(fmt.Sprintf("x%d_%d", i, j), 0, math.Inf(1))
			obj = obj.AddTerm(x[i][j], float64(1+r.IntN(20)))
		}
	}
	m.SetObjective(obj)
	for j := range nc {
		e := gb.NewExpr()
		for i := range nf {
			e = e.AddTerm(x[i][j], 1)
		}
		m.AddConstraint(fmt.Sprintf("d%d", j), gb.Ge(e, demands[r.IntN(len(demands))]))
	}
	for i := range nf {
		e := gb.NewExpr(gb.T(y[i], -bigM))
		for j := range nc {
			e = e.AddTerm(x[i][j], 1)
		}
		m.AddConstraint(fmt.Sprintf("vub%d", i), gb.Le(e, 0))
	}
	return m, y, fixed
}

// TestFixedChargeGroundTruth checks branch-and-bound against exhaustive search over the
// open-depot subsets; each subset's continuous shipping LP is solved by the LP
// solver alone, independently of branching, cuts, presolve, and heuristics.
func TestFixedChargeGroundTruth(t *testing.T) {
	for seed := range uint64(40) {
		m, y, _ := buildFixedCharge(seed)
		p := m.Compile()
		best := math.Inf(1)
		for mask := 1; mask < 1<<len(y); mask++ {
			lb := append([]float64(nil), p.VarLB...)
			ub := append([]float64(nil), p.VarUB...)
			for i, v := range y {
				val := float64(mask >> i & 1)
				lb[v.Index()], ub[v.Index()] = val, val
			}
			s := gb.NewLPSolver(p, nil)
			s.SetStructuralBounds(lb, ub)
			if res := s.Solve(); res.Status == gb.StatusOptimal {
				best = min(best, res.Objective)
			}
		}
		for name, o := range optionVariants() {
			assertOptimal(t, fmt.Sprintf("seed=%d/%s", seed, name), p, mustSolveCompiled(t, p, o...), best)
		}
	}
}

// TestRegressionInstances replays fuzz cases on which goblin or CBC 2.10.3 failed. The
// expected objective of each file is independently verified in its metadata.
func TestRegressionInstances(t *testing.T) {
	paths, err := filepath.Glob("testdata/regression/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no regression fixtures found: %v", err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			ID                string          `json:"id"`
			ExpectedObjective float64         `json:"expected_objective"`
			Doc               json.RawMessage `json:"doc"`
		}
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		doc, err := format.ParsePuLPJSON(fx.Doc)
		if err != nil {
			t.Fatal(err)
		}
		m, err := format.BuildModelFromPuLPJSON(doc)
		if err != nil {
			t.Fatal(err)
		}
		p, err := m.CompileChecked()
		if err != nil {
			t.Fatal(err)
		}
		for name, o := range optionVariants() {
			assertOptimal(t, fx.ID+"/"+name, p, mustSolveCompiled(t, p, o...), fx.ExpectedObjective)
		}
	}
}

// TestInvalidConstraintSenseRejected: an out-of-range ConstraintSense was previously solved
// as an equation while MaxViolation treated it as 0 <= a'x <= 0.
func TestInvalidConstraintSenseRejected(t *testing.T) {
	m := gb.NewModel("badsense", gb.Maximize)
	x := m.AddContVar("x", 0, 10)
	m.SetObjective(gb.NewExpr(gb.T(x, 1)))
	m.AddConstraint("c", gb.Constraint{Expr: gb.NewExpr(gb.T(x, 1)), Sense: gb.ConstraintSense(7), RHS: 3})
	if err := m.Validate(); err == nil {
		t.Fatal("Validate accepted ConstraintSense(7)")
	}
	if res, err := m.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("Solve on invalid model returned (%v, %v), want Undefined with error", res.Status, err)
	}
}

// TestEmptyIntegerDomainIsInfeasible: rounding [0.5, 0.7] leaves an empty integer domain.
func TestEmptyIntegerDomainIsInfeasible(t *testing.T) {
	m := gb.NewModel("emptyint", gb.Minimize)
	x := m.AddIntVar("x", 0.5, 0.7)
	m.SetObjective(gb.NewExpr(gb.T(x, 1)))
	if cat := m.VarCategoryOf(x); cat != gb.Integer {
		t.Fatalf("empty-domain integer variable recategorized as %v", cat)
	}
	for name, o := range optionVariants() {
		if res := mustSolve(t, m, o...); res.Status != gb.StatusInfeasible {
			t.Fatalf("%s: status=%v, want Infeasible", name, res.Status)
		}
	}
}

// TestMaxIterLimit: Options.MaxIter caps Result.Iterations and reports IterationLimit with
// a valid global bound.
func TestMaxIterLimit(t *testing.T) {
	m := gb.NewModel("maxiter", gb.Maximize)
	r := rand.New(rand.NewPCG(5, 6))
	e, k1, k2 := gb.NewExpr(), gb.NewExpr(), gb.NewExpr()
	for i := range 40 {
		v := m.AddIntVar(fmt.Sprintf("x%d", i), 0, 3)
		e = e.AddTerm(v, float64(1+r.IntN(20)))
		k1 = k1.AddTerm(v, float64(1+r.IntN(15)))
		k2 = k2.AddTerm(v, float64(1+r.IntN(15)))
	}
	m.SetObjective(e)
	m.AddConstraint("a", gb.Le(k1, 90))
	m.AddConstraint("b", gb.Le(k2, 95))
	p := m.Compile()
	full := mustSolveCompiled(t, p)
	if full.Status != gb.StatusOptimal {
		t.Fatalf("reference solve: %v", full.Status)
	}
	if full.Iterations < 50 {
		t.Skipf("instance too easy for an iteration-limit test (%d iterations)", full.Iterations)
	}
	maxIter := full.Iterations / 4
	res := mustSolveCompiled(t, p, gb.WithMaxIter(maxIter))
	if res.Status != gb.StatusIterationLimit {
		t.Fatalf("MaxIter=%d: status=%v (iters=%d), want IterationLimit", maxIter, res.Status, res.Iterations)
	}
	if res.Iterations > maxIter {
		t.Fatalf("Iterations=%d exceed MaxIter=%d", res.Iterations, maxIter)
	}
	if res.BestBound < full.Objective-1e-6 {
		t.Fatalf("BestBound %.6f below the optimum %.6f of a maximization", res.BestBound, full.Objective)
	}
}

// TestBestBoundIsTrueDualBound: BestBound must not be overwritten with the objective; it is
// the global dual bound and therefore never better than the incumbent.
func TestBestBoundIsTrueDualBound(t *testing.T) {
	for seed := range uint64(10) {
		m, _, _ := buildFixedCharge(100 + seed)
		p := m.Compile()
		res := mustSolveCompiled(t, p)
		if res.Status != gb.StatusOptimal {
			t.Fatalf("seed %d: %v", seed, res.Status)
		}
		if res.BestBound > res.Objective+1e-9 {
			t.Fatalf("seed %d: BestBound %.9f exceeds objective %.9f in a minimization", seed, res.BestBound, res.Objective)
		}
		if res.BestBound < res.RootLPBound-1e-9 {
			t.Fatalf("seed %d: BestBound %.9f is weaker than the root LP bound %.9f", seed, res.BestBound, res.RootLPBound)
		}
	}
}
