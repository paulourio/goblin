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

// TestLotSizingSharedSetupAndCyclicFlowCuts checks that (l, S) flow cuts
// remain valid when multiple periods share one binary setup variable or when inventory
// balance rows form a cycle.
func TestLotSizingSharedSetupAndCyclicFlowCuts(t *testing.T) {
	variants := optionVariants()
	variants["cuts-no-presolve"] = []gb.Option{gb.WithPresolve(false), gb.WithCuts(true), gb.WithHeuristics(false)}

	for _, x0UB := range []float64{100, 5} {
		m := gb.NewModel("shared_vub_lotsizing", gb.Minimize)
		y0 := m.AddBinaryVar("y0")
		x0 := m.AddContVar("x0", 0, x0UB)
		s1 := m.AddContVar("s1", 0, 100)
		y := m.AddBinaryVar("y")
		x1 := m.AddContVar("x1", 0, 100)
		s2 := m.AddContVar("s2", 0, 100)
		x2 := m.AddContVar("x2", 0, 100)
		s3 := m.AddContVar("s3", 0, 100)

		m.SetObjective(gb.NewExpr(gb.T(s1, 1000), gb.T(y, 100), gb.T(s3, 1000)))
		m.AddConstraint("f0", gb.Eq(gb.NewExpr(gb.T(x0, 1), gb.T(s1, -1)), 1))
		m.AddConstraint("f1", gb.Eq(gb.NewExpr(gb.T(s1, 1), gb.T(x1, 1), gb.T(s2, -1)), 10))
		m.AddConstraint("f2", gb.Eq(gb.NewExpr(gb.T(s2, 1), gb.T(x2, 1), gb.T(s3, -1)), 10))
		m.AddConstraint("vub0", gb.Le(gb.NewExpr(gb.T(x0, 1), gb.T(y0, -x0UB)), 0))
		m.AddConstraint("vub1", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(y, -100)), 0))
		m.AddConstraint("vub2", gb.Le(gb.NewExpr(gb.T(x2, 1), gb.T(y, -100)), 0))

		p := m.Compile()
		for name, opts := range variants {
			label := fmt.Sprintf("shared_x0UB=%.0f/%s", x0UB, name)
			assertOptimal(t, label, p, mustSolveCompiled(t, p, opts...), 100.0)
		}
	}

	mc := gb.NewModel("cyclic_flow_lotsizing", gb.Minimize)
	y0 := mc.AddBinaryVar("y0")
	x0 := mc.AddContVar("x0", 0, 100)
	s1 := mc.AddContVar("s1", 0, 100)
	y1 := mc.AddBinaryVar("y1")
	x1 := mc.AddContVar("x1", 0, 100)
	s2 := mc.AddContVar("s2", 0, 100)
	y2 := mc.AddBinaryVar("y2")
	x2 := mc.AddContVar("x2", 0, 100)

	mc.SetObjective(gb.NewExpr(
		gb.T(y0, 100), gb.T(y1, 100), gb.T(y2, 100),
		gb.T(s1, 15), gb.T(s2, 15),
	))
	mc.AddConstraint("f0", gb.Eq(gb.NewExpr(gb.T(x0, 1), gb.T(s1, -1)), 2))
	mc.AddConstraint("f1", gb.Eq(gb.NewExpr(gb.T(s1, 1), gb.T(x1, 1), gb.T(s2, -1)), 10))
	mc.AddConstraint("f2", gb.Eq(gb.NewExpr(gb.T(s2, 1), gb.T(x2, 1), gb.T(s1, -1)), 10))
	mc.AddConstraint("vub0", gb.Le(gb.NewExpr(gb.T(x0, 1), gb.T(y0, -100)), 0))
	mc.AddConstraint("vub1", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(y1, -100)), 0))
	mc.AddConstraint("vub2", gb.Le(gb.NewExpr(gb.T(x2, 1), gb.T(y2, -100)), 0))

	pc := mc.Compile()
	for name, opts := range variants {
		assertOptimal(t, "cyclic/"+name, pc, mustSolveCompiled(t, pc, opts...), 300.0)
	}
}

// TestZeroVariableFeasibilityAndCuts verifies that zero-variable models validate row
// and cut bounds across Presolve, LPSolver, CBCLPSolver, and SolveCompiled.
func TestZeroVariableFeasibilityAndCuts(t *testing.T) {
	mInfeas := gb.NewModel("zero_var_infeas", gb.Minimize)
	mInfeas.SetObjective(gb.ConstExpr(7.5))
	mInfeas.AddConstraint("bad", gb.Le(gb.Expr{}, -10.0))
	pInfeas := mInfeas.Compile()

	if pre := gb.Presolve(pInfeas, 0); !pre.Infeasible {
		t.Fatal("Presolve failed to detect infeasible zero-variable row")
	}
	if res := mustSolveCompiled(t, pInfeas, gb.WithPureGo()); res.Status != gb.StatusInfeasible {
		t.Fatalf("PureGo status = %v, want Infeasible", res.Status)
	}
	if lpRes := gb.NewLPSolver(pInfeas, nil).Solve(); lpRes.Status != gb.StatusInfeasible {
		t.Fatalf("LPSolver status = %v, want Infeasible", lpRes.Status)
	}

	mFeas := gb.NewModel("zero_var_feas", gb.Maximize)
	mFeas.SetObjective(gb.ConstExpr(12.25))
	mFeas.AddConstraint("ok", gb.Le(gb.Expr{}, 5.0))
	pFeas := mFeas.Compile()

	resFeas := mustSolveCompiled(t, pFeas, gb.WithPureGo())
	if resFeas.Status != gb.StatusOptimal || math.Abs(resFeas.Objective-12.25) > 1e-9 {
		t.Fatalf("PureGo feasible zero-var: status=%v obj=%v, want Optimal 12.25", resFeas.Status, resFeas.Objective)
	}
	lpSolver := gb.NewLPSolver(pFeas, nil)
	lpSolver.AddCut(nil, nil, -1.0)
	if lpCut := lpSolver.Solve(); lpCut.Status != gb.StatusInfeasible {
		t.Fatalf("LPSolver with violated cut on zero-var model: status=%v, want Infeasible", lpCut.Status)
	}

	if gb.CBCAvailable() {
		if res := mustSolveCompiled(t, pInfeas, gb.WithCBC()); res.Status != gb.StatusInfeasible {
			t.Fatalf("CBC status = %v, want Infeasible", res.Status)
		}
		cbcInfeas := gb.NewCBCLPSolver(pInfeas)
		defer cbcInfeas.Close()
		if res := cbcInfeas.Solve(); res.Status != gb.StatusInfeasible {
			t.Fatalf("CBCLPSolver infeasible status = %v, want Infeasible", res.Status)
		}

		cbcFeas := gb.NewCBCLPSolver(pFeas)
		defer cbcFeas.Close()
		if res := cbcFeas.Solve(); res.Status != gb.StatusOptimal || math.Abs(res.Objective-12.25) > 1e-9 {
			t.Fatalf("CBCLPSolver feasible: status=%v obj=%v, want Optimal 12.25", res.Status, res.Objective)
		}
		cbcFeas.AddCut(nil, nil, -1.0)
		if res := cbcFeas.Solve(); res.Status != gb.StatusInfeasible {
			t.Fatalf("CBCLPSolver violated cut status = %v, want Infeasible", res.Status)
		}
	}
}

// TestDuplicateTermOverflowRejected ensures that finite duplicate coefficients whose sum
// overflows IEEE-754 float64 fail Model.Validate instead of producing NaN objectives.
func TestDuplicateTermOverflowRejected(t *testing.T) {
	var nilModel *gb.Model
	if err := nilModel.Validate(); err == nil {
		t.Fatal("nil Model.Validate() returned nil error")
	}
	if res, err := nilModel.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("nil Model.Solve() returned (%v, %v), want Undefined with error", res.Status, err)
	}

	mObj := gb.NewModel("obj_overflow", gb.Minimize)
	v := mObj.AddContVar("v", 0, 10)
	mObj.SetObjective(gb.NewExpr(gb.T(v, 1e308), gb.T(v, 1e308)))
	if err := mObj.Validate(); err == nil {
		t.Fatal("Validate accepted overflowing duplicate objective terms")
	}
	if _, err := mObj.CompileChecked(); err == nil {
		t.Fatal("CompileChecked accepted overflowing duplicate objective terms")
	}
	if res, err := mObj.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("Solve on overflowing objective returned (%v, %v), want Undefined with error", res.Status, err)
	}

	mRow := gb.NewModel("row_overflow", gb.Minimize)
	x := mRow.AddContVar("x", 0, 10)
	mRow.SetObjective(gb.NewExpr(gb.T(x, 1)))
	mRow.AddConstraint("c", gb.Le(gb.NewExpr(gb.T(x, 1e308), gb.T(x, 1e308)), 5))
	if err := mRow.Validate(); err == nil {
		t.Fatal("Validate accepted overflowing duplicate constraint terms")
	}
	if res, err := mRow.Solve(t.Context()); err == nil || res.Status != gb.StatusUndefined {
		t.Fatalf("Solve on overflowing constraint returned (%v, %v), want Undefined with error", res.Status, err)
	}
}

// TestLUFactorizationResetGuardOnSingularBasis verifies that FTRAN and BTRAN leave inputs
// unchanged after Factorize fails on a singular matrix rather than reading stale pivots.
func TestLUFactorizationResetGuardOnSingularBasis(t *testing.T) {
	lu := &gb.LUFactorization{}
	if err := lu.Factorize(2, func(col int, dst []float64) {
		if col == 0 {
			dst[0] = 2.0
		} else {
			dst[1] = 4.0
		}
	}); err != nil {
		t.Fatalf("unexpected error on nonsingular basis: %v", err)
	}
	vec := []float64{6.0, 8.0}
	lu.FTRAN(vec)
	if math.Abs(vec[0]-3.0) > 1e-12 || math.Abs(vec[1]-2.0) > 1e-12 {
		t.Fatalf("unexpected FTRAN result %v, want [3, 2]", vec)
	}

	if err := lu.Factorize(2, func(_ int, dst []float64) {
		dst[0] = 1.0
		dst[1] = 1.0
	}); err == nil {
		t.Fatal("expected singular basis error on rank-1 matrix")
	}

	rhs := []float64{6.0, 8.0}
	lu.FTRAN(rhs)
	if rhs[0] != 6.0 || rhs[1] != 8.0 {
		t.Fatalf("FTRAN mutated input on invalid factorization: got %v, want [6, 8]", rhs)
	}
	dual := []float64{5.0, 7.0}
	lu.BTRAN(dual)
	if dual[0] != 5.0 || dual[1] != 7.0 {
		t.Fatalf("BTRAN mutated input on invalid factorization: got %v, want [5, 7]", dual)
	}
}

// TestBackendCBCMaxIterAndUnconstrainedParity checks that BackendCBC honors WithMaxIter
// and matches PureGo on unconstrained free variables.
func TestBackendCBCMaxIterAndUnconstrainedParity(t *testing.T) {
	for _, c := range []float64{1e-6, 1e-8, 1e-10} {
		mu := gb.NewModel("unconstrained_free", gb.Minimize)
		x := mu.AddContVar("x", math.Inf(-1), math.Inf(1))
		mu.SetObjective(gb.NewExpr(gb.T(x, c)))
		resGo := mustSolve(t, mu, gb.WithPureGo())
		if resGo.Status != gb.StatusUnbounded {
			t.Fatalf("PureGo c=%g: status=%v, want Unbounded", c, resGo.Status)
		}
		if gb.CBCAvailable() {
			resCBC := mustSolve(t, mu, gb.WithCBC())
			if resCBC.Status != gb.StatusUnbounded {
				t.Fatalf("CBC c=%g: status=%v, want Unbounded", c, resCBC.Status)
			}
		}
	}

	if !gb.CBCAvailable() {
		return
	}

	m := gb.NewModel("cbc_maxiter", gb.Maximize)
	r := rand.New(rand.NewPCG(11, 22))
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

	full := mustSolveCompiled(t, p, gb.WithCBC())
	if full.Status != gb.StatusOptimal {
		t.Fatalf("CBC full solve: status=%v, want Optimal", full.Status)
	}
	resMIP := mustSolveCompiled(t, p, gb.WithCBC(), gb.WithPresolve(false), gb.WithHeuristics(false), gb.WithMaxIter(1))
	if resMIP.Status != gb.StatusIterationLimit {
		t.Fatalf("CBC MIP WithMaxIter(1): status=%v, want IterationLimit", resMIP.Status)
	}

	mLP := gb.NewModel("cbc_lp_maxiter", gb.Minimize)
	x0 := mLP.AddContVar("x0", 0, 100)
	x1 := mLP.AddContVar("x1", 0, 100)
	x2 := mLP.AddContVar("x2", 0, 100)
	mLP.SetObjective(gb.NewExpr(gb.T(x0, -3), gb.T(x1, -2), gb.T(x2, -4)))
	mLP.AddConstraint("c1", gb.Le(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1), gb.T(x2, 2)), 40))
	mLP.AddConstraint("c2", gb.Le(gb.NewExpr(gb.T(x0, 2), gb.T(x1, 1), gb.T(x2, 1)), 50))
	mLP.AddConstraint("c3", gb.Ge(gb.NewExpr(gb.T(x0, 1), gb.T(x2, 1)), 15))
	resLP := mustSolve(t, mLP, gb.WithCBC(), gb.WithPresolve(false), gb.WithMaxIter(1))
	if resLP.Status != gb.StatusIterationLimit {
		t.Fatalf("CBC LP WithMaxIter(1): status=%v, want IterationLimit", resLP.Status)
	}
}

// TestAddIntVarBoundsAndExprAddTermImmutability verifies that AddIntVar(0, 1) allows
// widening bounds via SetVarBounds while still compiling [0, 1] integers as Binary, and
// that branching Expr.AddTerm calls never alias shared backing slices.
func TestAddIntVarBoundsAndExprAddTermImmutability(t *testing.T) {
	m := gb.NewModel("int_widening", gb.Maximize)
	xWide := m.AddIntVar("x_wide", 0, 1)
	xBin := m.AddIntVar("x_bin", 0, 1)
	m.SetVarBounds(xWide, 0, 10)
	m.SetObjective(gb.NewExpr(gb.T(xWide, 1), gb.T(xBin, 1)))
	m.AddConstraint("cap", gb.Le(gb.NewExpr(gb.T(xWide, 1), gb.T(xBin, 1)), 8))

	if m.VarCategoryOf(xWide) != gb.Integer {
		t.Fatalf("AddIntVar category = %v, want Integer", m.VarCategoryOf(xWide))
	}
	p := m.Compile()
	if p.VarCat[xWide.Index()] != gb.Integer || p.VarUB[xWide.Index()] != 10 {
		t.Fatalf("widened variable compiled as cat=%v ub=%v, want Integer ub=10", p.VarCat[xWide.Index()], p.VarUB[xWide.Index()])
	}
	if p.VarCat[xBin.Index()] != gb.Binary {
		t.Fatalf("[0, 1] integer variable compiled as cat=%v, want Binary", p.VarCat[xBin.Index()])
	}
	res := mustSolveCompiled(t, p)
	if res.Status != gb.StatusOptimal || math.Abs(res.Objective-8.0) > 1e-6 {
		t.Fatalf("unexpected solve result: status=%v obj=%v, want Optimal 8.0", res.Status, res.Objective)
	}

	v0 := m.AddContVar("v0", 0, 10)
	v1 := m.AddContVar("v1", 0, 10)
	v2 := m.AddContVar("v2", 0, 10)
	base := gb.Dot([]gb.Var{v0, v1}, []float64{1.0, 0.0})
	e1 := base.AddTerm(v1, 2.0)
	e2 := base.AddTerm(v2, 99.0)
	if e1.Terms[1].Var != v1 || e1.Terms[1].Coeff != 2.0 {
		t.Fatalf("Expr.AddTerm mutated sibling branch: e1.Terms=%+v, e2.Terms=%+v", e1.Terms, e2.Terms)
	}
}

// Checks that LP-unbounded MILPs report StatusInfeasible when the integer hull is empty
// and StatusUnbounded when an integer-feasible point exists.
func TestLPUnboundedIntegerInfeasibleVsUnboundedMIP(t *testing.T) {
	buildOddCycle := func(minSum float64) *gb.Model {
		m := gb.NewModel("odd_cycle_unbounded", gb.Maximize)
		x1 := m.AddBinaryVar("x1")
		x2 := m.AddBinaryVar("x2")
		x3 := m.AddBinaryVar("x3")
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(gb.NewExpr(gb.T(y, 1)))
		m.AddConstraint("e12", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(x2, 1)), 1))
		m.AddConstraint("e23", gb.Le(gb.NewExpr(gb.T(x2, 1), gb.T(x3, 1)), 1))
		m.AddConstraint("e13", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(x3, 1)), 1))
		m.AddConstraint("sum", gb.Ge(gb.NewExpr(gb.T(x1, 1), gb.T(x2, 1), gb.T(x3, 1)), minSum))
		return m
	}

	pInfeas := buildOddCycle(1.4).Compile()
	pUnbnd := buildOddCycle(1.0).Compile()
	for name, opts := range optionVariants() {
		if res := mustSolveCompiled(t, pInfeas, opts...); res.Status != gb.StatusInfeasible {
			t.Fatalf("infeasible odd-cycle/%s: status=%v, want Infeasible", name, res.Status)
		}
		if res := mustSolveCompiled(t, pUnbnd, opts...); res.Status != gb.StatusUnbounded {
			t.Fatalf("unbounded odd-cycle/%s: status=%v, want Unbounded", name, res.Status)
		}
	}
	if gb.CBCAvailable() {
		if res := mustSolveCompiled(t, pInfeas, gb.WithCBC(), gb.WithPresolve(false)); res.Status != gb.StatusInfeasible {
			t.Fatalf("CBC infeasible odd-cycle: status=%v, want Infeasible", res.Status)
		}
		if res := mustSolveCompiled(t, pUnbnd, gb.WithCBC(), gb.WithPresolve(false)); res.Status != gb.StatusUnbounded {
			t.Fatalf("CBC unbounded odd-cycle: status=%v, want Unbounded", res.Status)
		}
	}
}
