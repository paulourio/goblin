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

// Verifies layout-independent S_G orbit symmetry breaking in Presolve across designated
// bijection subgroups, general partitioning orbitopes, monotone-cost empty-block ordering,
// and asymmetric negative controls.
func TestOrbitSymmetryBreaking(t *testing.T) {
	t.Run("designated_bijection_interleaved_layout", func(t *testing.T) {
		const numGroups = 3
		const numItems = 7
		const numPeriods = 4
		weights := [numPeriods][numItems]float64{
			{310, 470, 520, 680, 190, 410, 620},
			{440, 290, 610, 350, 530, 220, 580},
			{260, 510, 390, 640, 480, 330, 490},
			{590, 360, 450, 280, 370, 610, 310},
		}
		m := gb.NewModel("sym_bijection_interleaved", gb.Minimize)
		var x [numGroups][numItems]gb.Var
		// Interleave item-major binaries with auxiliary continuous variables so variable
		// indices do not follow contiguous group-major blocks.
		var z [numGroups][numPeriods]gb.Var
		var mean [numGroups]gb.Var
		for i := range numItems {
			for g := range numGroups {
				x[g][i] = m.AddBinaryVar(fmt.Sprintf("item_%d_grp_%d", i, g))
			}
			if i < numGroups {
				mean[i] = m.AddContVar(fmt.Sprintf("mean_%d", i), 0, math.Inf(1))
				for pIdx := range numPeriods {
					z[i][pIdx] = m.AddContVar(fmt.Sprintf("dev_%d_%d", i, pIdx), 0, math.Inf(1))
				}
			}
		}
		obj := gb.NewExpr()
		for g := range numGroups {
			for pIdx := range numPeriods {
				obj = obj.AddTerm(z[g][pIdx], 1.0)
				pos := gb.NewExpr(gb.T(z[g][pIdx], 1.0), gb.T(mean[g], 1.0))
				neg := gb.NewExpr(gb.T(z[g][pIdx], 1.0), gb.T(mean[g], -1.0))
				for i := range numItems {
					pos = pos.AddTerm(x[g][i], -weights[pIdx][i])
					neg = neg.AddTerm(x[g][i], weights[pIdx][i])
				}
				m.AddConstraint(fmt.Sprintf("l1_pos_%d_%d", g, pIdx), gb.Ge(pos, 0))
				m.AddConstraint(fmt.Sprintf("l1_neg_%d_%d", g, pIdx), gb.Ge(neg, 0))
			}
		}
		m.SetObjective(obj)
		for i := range numItems {
			part := gb.NewExpr()
			for g := range numGroups {
				part = part.AddTerm(x[g][i], 1.0)
			}
			m.AddConstraint(fmt.Sprintf("part_%d", i), gb.Eq(part, 1.0))
		}
		for g := range numGroups {
			desig := gb.NewExpr()
			for i := numItems - numGroups; i < numItems; i++ {
				desig = desig.AddTerm(x[g][i], 1.0)
			}
			m.AddConstraint(fmt.Sprintf("desig_%d", g), gb.Eq(desig, 1.0))
		}

		p := m.Compile()
		preRes := gb.Presolve(p, 5)
		if preRes.Infeasible {
			t.Fatalf("Presolve reported infeasible on feasible symmetric model")
		}
		if preRes.Reduced.NumConstraints >= p.NumConstraints {
			t.Fatalf("Presolve constraints=%d, want < %d after eliminating fixed designated equations",
				preRes.Reduced.NumConstraints, p.NumConstraints)
		}

		resNoPre := mustSolveCompiled(t, p, gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false))
		if resNoPre.Status != gb.StatusOptimal {
			t.Fatalf("no-presolve baseline status=%v, want Optimal", resNoPre.Status)
		}
		wantObj := resNoPre.Objective
		for name, opts := range optionVariants() {
			assertOptimal(t, "bijection/"+name, p, mustSolveCompiled(t, p, opts...), wantObj)
		}
		resBare := mustSolveCompiled(t, p, gb.WithCuts(false), gb.WithHeuristics(false))
		if resBare.Nodes >= resNoPre.Nodes {
			t.Fatalf("presolve symmetry breaking nodes=%d, want < %d (no-presolve)", resBare.Nodes, resNoPre.Nodes)
		}
	})

	t.Run("partition_orbitope_and_edge_statuses", func(t *testing.T) {
		weights := []float64{19, 17, 13, 11, 7, 5, 3, 2}
		m := gb.NewModel("sym_makespan_3", gb.Minimize)
		makespan := m.AddContVar("makespan", 0, math.Inf(1))
		m.SetObjective(gb.NewExpr(gb.T(makespan, 1.0)))
		var x [3][]gb.Var
		for g := range 3 {
			x[g] = make([]gb.Var, len(weights))
			load := gb.NewExpr(gb.T(makespan, 1.0))
			for i, w := range weights {
				x[g][i] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", g, i))
				load = load.AddTerm(x[g][i], -w)
			}
			m.AddConstraint(fmt.Sprintf("load_%d", g), gb.Ge(load, 0))
		}
		for i := range weights {
			m.AddConstraint(fmt.Sprintf("assign_%d", i),
				gb.Eq(gb.NewExpr(gb.T(x[0][i], 1), gb.T(x[1][i], 1), gb.T(x[2][i], 1)), 1.0))
		}
		p := m.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "orbitope/"+name, p, mustSolveCompiled(t, p, opts...), 26.0)
		}
		resNoPre := mustSolveCompiled(t, p, gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false))
		resBare := mustSolveCompiled(t, p, gb.WithCuts(false), gb.WithHeuristics(false))
		if resBare.Nodes >= resNoPre.Nodes {
			t.Fatalf("orbitope symmetry breaking nodes=%d, want < %d", resBare.Nodes, resNoPre.Nodes)
		}

		// Symmetric infeasible partitioning model: 3 groups with capacity 1 each cannot hold 4 items.
		mInf := gb.NewModel("sym_infeasible", gb.Minimize)
		var xInf [3][4]gb.Var
		for g := range 3 {
			capExpr := gb.NewExpr()
			for i := range 4 {
				xInf[g][i] = mInf.AddBinaryVar(fmt.Sprintf("xi_%d_%d", g, i))
				capExpr = capExpr.AddTerm(xInf[g][i], 1.0)
			}
			mInf.AddConstraint(fmt.Sprintf("cap_%d", g), gb.Le(capExpr, 1.0))
		}
		for i := range 4 {
			mInf.AddConstraint(fmt.Sprintf("part_%d", i),
				gb.Eq(gb.NewExpr(gb.T(xInf[0][i], 1), gb.T(xInf[1][i], 1), gb.T(xInf[2][i], 1)), 1.0))
		}
		pInf := mInf.Compile()
		for name, opts := range optionVariants() {
			if res := mustSolveCompiled(t, pInf, opts...); res.Status != gb.StatusInfeasible {
				t.Fatalf("sym_infeasible/%s: status=%v, want Infeasible", name, res.Status)
			}
		}

		// Symmetric unbounded partitioning model: free continuous bonus coupled symmetrically.
		mUnb := gb.NewModel("sym_unbounded", gb.Minimize)
		u := mUnb.AddContVar("u", math.Inf(-1), 0)
		mUnb.SetObjective(gb.NewExpr(gb.T(u, 1.0)))
		var xUnb [2][2]gb.Var
		for g := range 2 {
			for i := range 2 {
				xUnb[g][i] = mUnb.AddBinaryVar(fmt.Sprintf("xu_%d_%d", g, i))
			}
			mUnb.AddConstraint(fmt.Sprintf("g_%d", g),
				gb.Eq(gb.NewExpr(gb.T(xUnb[g][0], 1), gb.T(xUnb[g][1], 1)), 1.0))
		}
		for i := range 2 {
			mUnb.AddConstraint(fmt.Sprintf("p_%d", i),
				gb.Eq(gb.NewExpr(gb.T(xUnb[0][i], 1), gb.T(xUnb[1][i], 1)), 1.0))
		}
		pUnb := mUnb.Compile()
		for name, opts := range optionVariants() {
			if res := mustSolveCompiled(t, pUnb, opts...); res.Status != gb.StatusUnbounded {
				t.Fatalf("sym_unbounded/%s: status=%v, want Unbounded", name, res.Status)
			}
		}
	})

	t.Run("monotone_empty_block_and_asymmetric_control", func(t *testing.T) {
		sizes := []float64{6, 6, 5, 3}
		openCosts := []float64{1.00, 1.02, 1.07, 1.25, 1.40}
		m := gb.NewModel("perturbed_binpack", gb.Minimize)
		var y [5]gb.Var
		var x [5][]gb.Var
		obj := gb.NewExpr()
		for b := range 5 {
			y[b] = m.AddBinaryVar(fmt.Sprintf("y_%d", b))
			obj = obj.AddTerm(y[b], openCosts[b])
			x[b] = make([]gb.Var, len(sizes))
			capExpr := gb.NewExpr(gb.T(y[b], -10.0))
			for i, sz := range sizes {
				x[b][i] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", b, i))
				capExpr = capExpr.AddTerm(x[b][i], sz)
			}
			m.AddConstraint(fmt.Sprintf("cap_%d", b), gb.Le(capExpr, 0))
		}
		m.SetObjective(obj)
		for i := range sizes {
			m.AddConstraint(fmt.Sprintf("assign_%d", i),
				gb.Eq(gb.NewExpr(
					gb.T(x[0][i], 1), gb.T(x[1][i], 1), gb.T(x[2][i], 1),
					gb.T(x[3][i], 1), gb.T(x[4][i], 1),
				), 1.0))
		}
		p := m.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "perturbed_binpack/"+name, p, mustSolveCompiled(t, p, opts...), 3.09)
		}

		// Negative control: bin 0 has capacity 8 and cost 1.0, while bin 1 has capacity 10 and
		// cost 1.1. A single item of size 9 requires bin 1 to open while bin 0 stays closed, so
		// presolve must reject y_0 >= y_1 because the constraint capacities are asymmetric.
		mAsym := gb.NewModel("asym_binpack_control", gb.Minimize)
		y0 := mAsym.AddBinaryVar("y0")
		y1 := mAsym.AddBinaryVar("y1")
		x0 := mAsym.AddBinaryVar("x0")
		x1 := mAsym.AddBinaryVar("x1")
		mAsym.SetObjective(gb.NewExpr(gb.T(y0, 1.0), gb.T(y1, 1.1)))
		mAsym.AddConstraint("c0", gb.Le(gb.NewExpr(gb.T(x0, 9.0), gb.T(y0, -8.0)), 0))
		mAsym.AddConstraint("c1", gb.Le(gb.NewExpr(gb.T(x1, 9.0), gb.T(y1, -10.0)), 0))
		mAsym.AddConstraint("assign", gb.Eq(gb.NewExpr(gb.T(x0, 1.0), gb.T(x1, 1.0)), 1.0))
		pAsym := mAsym.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "asym_control/"+name, pAsym, mustSolveCompiled(t, pAsym, opts...), 1.1)
		}
	})
}

// Verifies single-row c-MIR cut separation on one-sided bounded variables and slack-cut
// compaction in LPSolver.PurgeSlackCuts followed by warm-started bound updates.
func TestOneSidedCMIRCutsAndPurgeSlackCuts(t *testing.T) {
	m := gb.NewModel("onesided_cmir", gb.Minimize)
	z := m.AddIntVar("z", 0, math.Inf(1))
	s := m.AddContVar("s", 0, math.Inf(1))
	w := m.AddContVar("w", math.Inf(-1), 2.0)
	m.SetObjective(gb.NewExpr(gb.T(z, 10.0), gb.T(s, 10.0), gb.T(w, -1.0)))
	m.AddConstraint("lower_mix", gb.Ge(gb.NewExpr(gb.T(z, 1.0), gb.T(s, 0.25)), 2.4))
	m.AddConstraint("upper_mix", gb.Le(gb.NewExpr(gb.T(z, -1.0), gb.T(w, 0.5)), -1.3))
	p := m.Compile()

	for name, opts := range optionVariants() {
		assertOptimal(t, "onesided_cmir/"+name, p, mustSolveCompiled(t, p, opts...), 28.0)
	}
	rootOnly := mustSolveCompiled(t, p, gb.WithPresolve(false), gb.WithHeuristics(false), gb.WithMaxNodes(1))
	if rootOnly.RootCutBound <= rootOnly.RootLPBound+1.0 {
		t.Fatalf("expected one-sided c-MIR to tighten RootCutBound > RootLPBound+1.0, got LP=%.4f Cut=%.4f",
			rootOnly.RootLPBound, rootOnly.RootCutBound)
	}

	lpModel := gb.NewModel("purge_slack_cuts", gb.Minimize)
	x0 := lpModel.AddContVar("x0", 0, 10)
	x1 := lpModel.AddContVar("x1", 0, 10)
	lpModel.SetObjective(gb.NewExpr(gb.T(x0, -3.0), gb.T(x1, -2.0)))
	lpModel.AddConstraint("cap", gb.Le(gb.NewExpr(gb.T(x0, 1.0), gb.T(x1, 1.0)), 6.0))
	lpProb := lpModel.Compile()

	ws := gb.AcquireWorkspace(lpProb.NumVars, lpProb.NumConstraints)
	defer gb.ReleaseWorkspace(ws)
	lp := gb.NewLPSolver(lpProb, ws)
	if res0 := lp.Solve(); res0.Status != gb.StatusOptimal || math.Abs(res0.Objective - -18.0) > 1e-8 {
		t.Fatalf("initial LP status=%v obj=%.6f, want Optimal -18.0", res0.Status, res0.Objective)
	}

	// Add one binding cut (x0 <= 4.0) and two strictly slack cuts (x0 + 2*x1 <= 20, x1 <= 9).
	lp.AddCut([]int{0}, []float64{1.0}, 4.0)
	lp.AddCut([]int{0, 1}, []float64{1.0, 2.0}, 20.0)
	lp.AddCut([]int{1}, []float64{1.0}, 9.0)
	resCut := lp.Solve()
	if resCut.Status != gb.StatusOptimal || math.Abs(resCut.Objective - -16.0) > 1e-8 {
		t.Fatalf("cut LP status=%v obj=%.6f, want Optimal -16.0", resCut.Status, resCut.Objective)
	}
	if got := lp.PurgeSlackCuts(0.05); got != 2 || len(lp.ExtraRows) != 1 {
		t.Fatalf("PurgeSlackCuts removed %d rows (remaining=%d), want 2 removed and 1 remaining",
			got, len(lp.ExtraRows))
	}

	// Tighten x1 <= 1.5 and warm-start; optimal is x0 = 4.0 (from kept cut), x1 = 1.5 -> obj = -15.0.
	lp.SetStructuralBounds([]float64{0, 0}, []float64{10, 1.5})
	resWarm := lp.Solve()
	if resWarm.Status != gb.StatusOptimal || math.Abs(resWarm.Objective - -15.0) > 1e-8 {
		t.Fatalf("warm LP after PurgeSlackCuts: status=%v obj=%.6f, want Optimal -15.0",
			resWarm.Status, resWarm.Objective)
	}
}

// Verifies LP Ruiz scaling, LU singleton drop alignment, presolve near-integer rounding guards,
// root Diophantine GCD infeasibility without presolve, incumbent polishing, and model validation.
func TestReviewAuditRegressions(t *testing.T) {
	ctx := t.Context()

	// 1. Extreme row/column scaling (1e-9 coefficients) must not return false Infeasible or Unbounded.
	mSmallGe := gb.NewModel("lp_scale_ge", gb.Minimize)
	xs := mSmallGe.AddContVar("x", 0, 1e4)
	mSmallGe.SetObjective(gb.VarExpr(xs))
	mSmallGe.AddConstraint("c", gb.NewExpr(xs.T(1e-9)).Ge(1e-6))
	resGe, err := mSmallGe.Solve(ctx)
	if err != nil || resGe.Status != gb.StatusOptimal || math.Abs(resGe.Objective-1000.0) > 1e-5 {
		t.Fatalf("lp_scale_ge: status=%v obj=%v err=%v, want Optimal 1000", resGe.Status, resGe.Objective, err)
	}
	if len(resGe.Duals) != 1 || math.Abs(resGe.Duals[0]-1e9) > 1e2 {
		t.Fatalf("lp_scale_ge dual=%v, want ~1e9", resGe.Duals)
	}

	mSmallMax := gb.NewModel("lp_scale_max", gb.Maximize)
	ym := mSmallMax.AddContVar("y", 0, math.Inf(1))
	mSmallMax.SetObjective(gb.VarExpr(ym))
	mSmallMax.AddConstraint("c", gb.NewExpr(ym.T(1e-9)).Le(1.0))
	resMax, err := mSmallMax.Solve(ctx)
	if err != nil || resMax.Status != gb.StatusOptimal || math.Abs(resMax.Objective-1e9)/1e9 > 1e-9 {
		t.Fatalf("lp_scale_max: status=%v obj=%v err=%v, want Optimal 1e9", resMax.Status, resMax.Objective, err)
	}

	// 2. Unconstrained Maximize reduced-cost sign must match constrained Maximize.
	mUnc := gb.NewModel("unc_max_rc", gb.Maximize)
	zu := mUnc.AddContVar("z", 0, 3)
	mUnc.SetObjective(gb.NewExpr(zu.T(2)))
	resUnc, err := mUnc.Solve(ctx)
	if err != nil || len(resUnc.ReducedCosts) != 1 || resUnc.ReducedCosts[0] != 2.0 {
		t.Fatalf("unc_max_rc: rc=%v err=%v, want [2]", resUnc.ReducedCosts, err)
	}
	if resUnc.Value(gb.Var(0)) != 0 {
		t.Fatalf("expected zero Var handle to return 0, got %v", resUnc.Value(gb.Var(0)))
	}

	// 3. Presolve near-integer GCD triggers must neither cut off valid optima nor violate original rows.
	mPreA := gb.NewModel("pre_near_int_a", gb.Maximize)
	xa := mPreA.AddIntVar("x", 0, 2000)
	mPreA.SetObjective(gb.VarExpr(xa))
	mPreA.AddConstraint("c", gb.NewExpr(xa.T(1.000000005)).Le(1000.000005))
	pPreA := mPreA.Compile()
	assertOptimal(t, "pre_near_int_a", pPreA, mustSolveCompiled(t, pPreA), 1000.0)

	mPreB := gb.NewModel("pre_near_int_b", gb.Maximize)
	xb := mPreB.AddIntVar("x", 0, 2e6)
	mPreB.SetObjective(gb.VarExpr(xb))
	mPreB.AddConstraint("c", gb.NewExpr(xb.T(2.00000001)).Le(2e6))
	pPreB := mPreB.Compile()
	resPreB := mustSolveCompiled(t, pPreB)
	assertOptimal(t, "pre_near_int_b", pPreB, resPreB, 999999.0)
	if bV, rV, iV := pPreB.MaxViolation(resPreB.Values, true); bV > 1e-7 || rV > 1e-7 || iV > 1e-7 {
		t.Fatalf("pre_near_int_b original violation: b=%e r=%e i=%e", bV, rV, iV)
	}

	// 4. Diophantine parity-infeasible equality over free integers must return Infeasible even with presolve disabled.
	mParity := gb.NewModel("parity_free_ints", gb.Minimize)
	px := mParity.AddIntVar("x", math.Inf(-1), math.Inf(1))
	py := mParity.AddIntVar("y", math.Inf(-1), math.Inf(1))
	mParity.AddConstraint("eq", gb.NewExpr(px.T(2), py.T(2)).Eq(1))
	for name, opts := range optionVariants() {
		rPar := mustSolveCompiled(t, mParity.Compile(), opts...)
		if rPar.Status != gb.StatusInfeasible {
			t.Fatalf("parity_free_ints/%s: status=%v, want Infeasible", name, rPar.Status)
		}
	}

	// 5. Deep general-integer B&B tree (split inequality 1 <= 2x + 2y <= 1) must explore 5,000 nodes rapidly.
	mDeep := gb.NewModel("deep_tree_nodes", gb.Minimize)
	dx := mDeep.AddIntVar("x", math.Inf(-1), math.Inf(1))
	dy := mDeep.AddIntVar("y", math.Inf(-1), math.Inf(1))
	mDeep.AddConstraint("lo", gb.NewExpr(dx.T(2), dy.T(2)).Ge(1))
	mDeep.AddConstraint("hi", gb.NewExpr(dx.T(2), dy.T(2)).Le(1))
	resDeep, err := mDeep.Solve(ctx, gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false), gb.WithMaxNodes(5000))
	if err != nil || resDeep.Status != gb.StatusNodeLimit || resDeep.Nodes != 5000 {
		t.Fatalf("deep_tree_nodes: status=%v nodes=%d err=%v", resDeep.Status, resDeep.Nodes, err)
	}

	// 6. Mixed-MILP incumbent polishing and sound BestBound on Maximize.
	mPol := gb.NewModel("polish_max", gb.Maximize)
	v0 := mPol.AddIntVar("x0", 2, 2)
	v1 := mPol.AddIntVar("x1", -2, 0)
	v2 := mPol.AddContVar("x2", 1, 5)
	v3 := mPol.AddIntVar("x3", -1, 3)
	v4 := mPol.AddIntVar("x4", 2, 6)
	mPol.SetObjective(gb.NewExpr(v0.T(3.5), v1.T(-9), v2.T(-10), v3.T(9), v4.T(3.5)).AddConstant(-2))
	mPol.AddConstraint("r0", gb.NewExpr(v0.T(3), v1.T(1), v2.T(2), v3.T(-7), v4.T(-1)).Eq(-3))
	mPol.AddConstraint("r1", gb.NewExpr(v1.T(4), v2.T(4), v3.T(4), v4.T(-7)).Ge(5))
	mPol.AddConstraint("r2", gb.NewExpr(v2.T(-3)).Le(-10))
	resPol := mustSolveCompiled(t, mPol.Compile())
	if resPol.Status != gb.StatusOptimal || math.Abs(resPol.Objective - -1.0) > 1e-9 || resPol.BestBound < resPol.Objective-1e-9 {
		t.Fatalf("polish_max: status=%v obj=%.12g bound=%.12g, want -1", resPol.Status, resPol.Objective, resPol.BestBound)
	}

	// 7. Auto-generated names yield to later explicit user names, while duplicate explicit names fail validation.
	mNames := gb.NewModel("auto_names", gb.Minimize)
	uAuto := mNames.AddContVar("", 0, 5)
	uExp := mNames.AddContVar("x_0", 0, 5)
	mNames.AddConstraint("", gb.VarExpr(uAuto).Le(4))
	mNames.AddConstraint("_C1", gb.VarExpr(uExp).Le(4))
	if err := mNames.Validate(); err != nil {
		t.Fatalf("expected auto-names to yield cleanly to explicit names, got %v", err)
	}
	mNames.AddConstraint("_C1", gb.VarExpr(uExp).Le(3))
	if err := mNames.Validate(); err == nil {
		t.Fatal("expected duplicate explicit constraint name error")
	}

	// 8. Invalid VarCategory and negative gap tolerances.
	mBadCat := gb.NewModel("bad_cat", gb.Minimize)
	_ = mBadCat.AddVar("w", 0.3, 4.7, gb.VarCategory(9))
	if err := mBadCat.Validate(); err == nil {
		t.Fatal("expected invalid VarCategory error")
	}
	resNegGap, err := mUnc.Solve(ctx, gb.WithRelGapTol(-1), gb.WithAbsGapTol(-1))
	if err != nil || resNegGap.Status != gb.StatusOptimal {
		t.Fatalf("negative gap tolerances should clamp to 0 and return Optimal, got %v (%v)", resNegGap.Status, err)
	}

	// 9. LPSolver.AddCut canonicalizes unsorted and duplicate column indices.
	lpCutM := gb.NewModel("lp_cut_canon", gb.Maximize)
	cx0 := lpCutM.AddContVar("cx0", 0, 10)
	cx1 := lpCutM.AddContVar("cx1", 0, 10)
	lpCutM.SetObjective(gb.NewExpr(cx0.T(1), cx1.T(1)))
	lpCutM.AddConstraint("base", gb.NewExpr(cx0.T(1), cx1.T(1)).Le(15))
	lpS := gb.NewLPSolver(lpCutM.Compile(), nil)
	defer lpS.Close()
	if r0 := lpS.Solve(); r0.Status != gb.StatusOptimal || math.Abs(r0.Objective-15.0) > 1e-8 {
		t.Fatalf("lp_cut_canon base: %v %v", r0.Status, r0.Objective)
	}
	lpS.AddCut([]int{1, 0, 1}, []float64{0.5, 1.0, 0.5}, 8.0)
	if r1 := lpS.Solve(); r1.Status != gb.StatusOptimal || math.Abs(r1.Objective-8.0) > 1e-8 {
		t.Fatalf("lp_cut_canon after unsorted duplicate cut: status=%v obj=%v, want 8", r1.Status, r1.Objective)
	}
}
