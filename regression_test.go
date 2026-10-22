package gopt_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"gopt"
	"gopt/format"
)

// optionVariants exercises the bare branch-and-bound as well as the full configuration:
// primal heuristics previously masked the node-discard defect, so correctness must not
// depend on them.
func optionVariants() map[string]gopt.Options {
	full := gopt.DefaultOptions()
	noHeur := full
	noHeur.EnableHeuristics = false
	bare := noHeur
	bare.EnableCuts = false
	barest := bare
	barest.EnablePresolve = false
	return map[string]gopt.Options{"default": full, "no-heuristics": noHeur, "bare": bare, "bare-no-presolve": barest}
}

func assertOptimal(t *testing.T, label string, p *gopt.CompiledProblem, res gopt.Result, want float64) {
	t.Helper()
	if res.Status != gopt.StatusOptimal {
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
	// An incumbent may violate rows by up to the feasibility tolerance, so its objective can
	// undercut the LP bound by a comparable amount; allow the default gap tolerance.
	boundTol := 1e-6 * max(1.0, math.Abs(res.Objective))
	if p.Sense == gopt.Minimize && res.BestBound > res.Objective+boundTol ||
		p.Sense == gopt.Maximize && res.BestBound < res.Objective-boundTol {
		t.Fatalf("%s: BestBound %.8f is not a valid bound for objective %.8f", label, res.BestBound, res.Objective)
	}
}

// TestBigMNearIntegralRoot: the root LP is integral within 1e-6 (y1 = 5e-7) but rounding
// y1 to 0 violates the demand row by 0.05. The solver previously returned NumericalError.
func TestBigMNearIntegralRoot(t *testing.T) {
	m := gopt.NewModel("bigm_root", gopt.Minimize)
	y1 := m.AddBinaryVar("y1")
	y2 := m.AddBinaryVar("y2")
	x1 := m.AddContVar("x1", 0, math.Inf(1))
	x2 := m.AddContVar("x2", 0, math.Inf(1))
	m.SetObjective(gopt.NewExpr(gopt.T(y1, 10), gopt.T(y2, 12), gopt.T(x1, 1), gopt.T(x2, 1)))
	m.AddConstraint("dem", gopt.Ge(gopt.NewExpr(gopt.T(x1, 1), gopt.T(x2, 1)), 0.05))
	m.AddConstraint("vub1", gopt.Le(gopt.NewExpr(gopt.T(x1, 1), gopt.T(y1, -1e5)), 0))
	m.AddConstraint("vub2", gopt.Le(gopt.NewExpr(gopt.T(x2, 1), gopt.T(y2, -1e5)), 0))
	p := m.Compile()
	for name, o := range optionVariants() {
		assertOptimal(t, name, p, gopt.SolveCompiled(p, o), 10.05)
	}
}

// TestBigMUnderBranching embeds the near-integral big-M structure under a fractional
// knapsack so that the search must branch. Nodes whose LP is integral within tolerance but
// whose rounding is infeasible were previously discarded, which produced a false Infeasible.
func TestBigMUnderBranching(t *testing.T) {
	m := gopt.NewModel("bigm_node", gopt.Minimize)
	y1 := m.AddBinaryVar("y1")
	y2 := m.AddBinaryVar("y2")
	x1 := m.AddContVar("x1", 0, math.Inf(1))
	x2 := m.AddContVar("x2", 0, math.Inf(1))
	w := []float64{7, 5, 4, 3, 2}
	val := []float64{9, 6, 5, 3, 2.5}
	obj := gopt.NewExpr(gopt.T(y1, 10), gopt.T(y2, 12), gopt.T(x1, 1), gopt.T(x2, 1))
	kn := gopt.NewExpr()
	for i := range w {
		z := m.AddBinaryVar(fmt.Sprintf("z%d", i))
		obj = obj.AddTerm(z, -val[i])
		kn = kn.AddTerm(z, w[i])
	}
	m.SetObjective(obj)
	m.AddConstraint("knap", gopt.Le(kn, 10.5))
	m.AddConstraint("dem", gopt.Ge(gopt.NewExpr(gopt.T(x1, 1), gopt.T(x2, 1)), 0.05))
	m.AddConstraint("vub1", gopt.Le(gopt.NewExpr(gopt.T(x1, 1), gopt.T(y1, -1e5)), 0))
	m.AddConstraint("vub2", gopt.Le(gopt.NewExpr(gopt.T(x2, 1), gopt.T(y2, -1e5)), 0))
	p := m.Compile()

	// Ground truth by enumeration: open the cheaper facility, pick the best knapsack subset.
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
		assertOptimal(t, name, p, gopt.SolveCompiled(p, o), best)
	}
}

// buildFixedCharge builds a capacitated-by-big-M facility location model with small demands.
func buildFixedCharge(seed uint64) (m *gopt.Model, y []gopt.Var, fixed []float64) {
	r := rand.New(rand.NewPCG(seed, 2026))
	nf, nc := 3+r.IntN(3), 3+r.IntN(5)
	bigM := []float64{1e4, 1e5, 1e6}[r.IntN(3)]
	demands := []float64{0.01, 0.05, 0.2, 1, 3}
	m = gopt.NewModel(fmt.Sprintf("fc_%d", seed), gopt.Minimize)
	y = make([]gopt.Var, nf)
	fixed = make([]float64, nf)
	x := make([][]gopt.Var, nf)
	obj := gopt.NewExpr()
	for i := range nf {
		y[i] = m.AddBinaryVar(fmt.Sprintf("y%d", i))
		fixed[i] = float64(5 + r.IntN(36))
		obj = obj.AddTerm(y[i], fixed[i])
		x[i] = make([]gopt.Var, nc)
		for j := range nc {
			x[i][j] = m.AddContVar(fmt.Sprintf("x%d_%d", i, j), 0, math.Inf(1))
			obj = obj.AddTerm(x[i][j], float64(1+r.IntN(20)))
		}
	}
	m.SetObjective(obj)
	for j := range nc {
		e := gopt.NewExpr()
		for i := range nf {
			e = e.AddTerm(x[i][j], 1)
		}
		m.AddConstraint(fmt.Sprintf("d%d", j), gopt.Ge(e, demands[r.IntN(len(demands))]))
	}
	for i := range nf {
		e := gopt.NewExpr(gopt.T(y[i], -bigM))
		for j := range nc {
			e = e.AddTerm(x[i][j], 1)
		}
		m.AddConstraint(fmt.Sprintf("vub%d", i), gopt.Le(e, 0))
	}
	return m, y, fixed
}

// TestFixedChargeGroundTruth checks branch-and-bound against exhaustive enumeration of the
// open-facility subsets; each subset's continuous transportation LP is solved by the LP
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
			s := gopt.NewLPSolver(p, nil)
			s.SetStructuralBounds(lb, ub)
			if res := s.Solve(); res.Status == gopt.StatusOptimal {
				best = min(best, res.Objective)
			}
		}
		for name, o := range optionVariants() {
			assertOptimal(t, fmt.Sprintf("seed=%d/%s", seed, name), p, gopt.SolveCompiled(p, o), best)
		}
	}
}

// TestRegressionInstances replays fuzz instances on which gopt or CBC 2.10.3 failed. The
// expected objective of each file is independently verified (see its "provenance" field).
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
			assertOptimal(t, fx.ID+"/"+name, p, gopt.SolveCompiled(p, o), fx.ExpectedObjective)
		}
	}
}

// TestInvalidConstraintSenseRejected: an out-of-range ConstraintSense was previously solved
// as an equality while MaxViolation treated it as 0 <= a'x <= 0.
func TestInvalidConstraintSenseRejected(t *testing.T) {
	m := gopt.NewModel("badsense", gopt.Maximize)
	x := m.AddContVar("x", 0, 10)
	m.SetObjective(gopt.NewExpr(gopt.T(x, 1)))
	m.AddConstraint("c", gopt.Constraint{Expr: gopt.NewExpr(gopt.T(x, 1)), Sense: gopt.ConstraintSense(7), RHS: 3})
	if err := m.Validate(); err == nil {
		t.Fatal("Validate accepted ConstraintSense(7)")
	}
	if res := m.Solve(); res.Status != gopt.StatusUndefined {
		t.Fatalf("Solve on invalid model returned %v, want Undefined", res.Status)
	}
}

// TestEmptyIntegerDomainIsInfeasible: rounding [0.5, 0.7] leaves an empty integer domain.
func TestEmptyIntegerDomainIsInfeasible(t *testing.T) {
	m := gopt.NewModel("emptyint", gopt.Minimize)
	x := m.AddIntVar("x", 0.5, 0.7)
	m.SetObjective(gopt.NewExpr(gopt.T(x, 1)))
	if cat := m.VarCategoryOf(x); cat != gopt.Integer {
		t.Fatalf("empty-domain integer variable recategorized as %v", cat)
	}
	for name, o := range optionVariants() {
		if res := m.Solve(o); res.Status != gopt.StatusInfeasible {
			t.Fatalf("%s: status=%v, want Infeasible", name, res.Status)
		}
	}
}

// TestMaxIterLimit: Options.MaxIter caps Result.Iterations and reports IterationLimit with
// a valid global bound.
func TestMaxIterLimit(t *testing.T) {
	m := gopt.NewModel("maxiter", gopt.Maximize)
	r := rand.New(rand.NewPCG(5, 6))
	e, k1, k2 := gopt.NewExpr(), gopt.NewExpr(), gopt.NewExpr()
	for i := range 40 {
		v := m.AddIntVar(fmt.Sprintf("x%d", i), 0, 3)
		e = e.AddTerm(v, float64(1+r.IntN(20)))
		k1 = k1.AddTerm(v, float64(1+r.IntN(15)))
		k2 = k2.AddTerm(v, float64(1+r.IntN(15)))
	}
	m.SetObjective(e)
	m.AddConstraint("a", gopt.Le(k1, 90))
	m.AddConstraint("b", gopt.Le(k2, 95))
	p := m.Compile()
	full := gopt.SolveCompiled(p)
	if full.Status != gopt.StatusOptimal {
		t.Fatalf("reference solve: %v", full.Status)
	}
	if full.Iterations < 50 {
		t.Skipf("instance too easy for an iteration-limit test (%d iterations)", full.Iterations)
	}
	o := gopt.DefaultOptions()
	o.MaxIter = full.Iterations / 4
	res := gopt.SolveCompiled(p, o)
	if res.Status != gopt.StatusIterationLimit {
		t.Fatalf("MaxIter=%d: status=%v (iters=%d), want IterationLimit", o.MaxIter, res.Status, res.Iterations)
	}
	if res.Iterations > o.MaxIter {
		t.Fatalf("Iterations=%d exceed MaxIter=%d", res.Iterations, o.MaxIter)
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
		res := gopt.SolveCompiled(p)
		if res.Status != gopt.StatusOptimal {
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
