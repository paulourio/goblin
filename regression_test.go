package goblin_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if p.Sense() == gb.Minimize && res.BestBound > res.Objective+boundTol ||
		p.Sense() == gb.Maximize && res.BestBound < res.Objective-boundTol {
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
		nVars := p.NumVars()
		baseLB := make([]float64, nVars)
		baseUB := make([]float64, nVars)
		for j := range nVars {
			baseLB[j], baseUB[j] = p.VarBounds(j)
		}
		best := math.Inf(1)
		for mask := 1; mask < 1<<len(y); mask++ {
			lb := append([]float64(nil), baseLB...)
			ub := append([]float64(nil), baseUB...)
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
			ID                string         `json:"id"`
			ExpectedObjective float64        `json:"expected_objective"`
			Doc               jsontext.Value `json:"doc"`
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
	_, ubWide := p.VarBounds(xWide.Index())
	if p.VarCategory(xWide.Index()) != gb.Integer || ubWide != 10 {
		t.Fatalf("widened variable compiled as cat=%v ub=%v, want Integer ub=10", p.VarCategory(xWide.Index()), ubWide)
	}
	if p.VarCategory(xBin.Index()) != gb.Binary {
		t.Fatalf("[0, 1] integer variable compiled as cat=%v, want Binary", p.VarCategory(xBin.Index()))
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
		if preRes.Reduced.NumConstraints() >= p.NumConstraints() {
			t.Fatalf("Presolve constraints=%d, want < %d after eliminating fixed designated equations",
				preRes.Reduced.NumConstraints(), p.NumConstraints())
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
		preRes := gb.Presolve(p, 5)
		if preRes.Infeasible {
			t.Fatalf("Presolve reported infeasible on feasible sym_makespan_3")
		}
		if preRes.Reduced.NumVars() >= p.NumVars() && preRes.Reduced.NumConstraints() >= p.NumConstraints() {
			t.Fatalf("Presolve failed to fix orbitope variables on sym_makespan_3: vars %d->%d, rows %d->%d",
				p.NumVars(), preRes.Reduced.NumVars(), p.NumConstraints(), preRes.Reduced.NumConstraints())
		}
		for name, opts := range optionVariants() {
			assertOptimal(t, "orbitope/"+name, p, mustSolveCompiled(t, p, opts...), 26.0)
		}
		resNoPre := mustSolveCompiled(t, p, gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false))
		resBare := mustSolveCompiled(t, p, gb.WithCuts(false), gb.WithHeuristics(false))
		if resBare.Nodes > 60 {
			t.Fatalf("orbitope symmetry breaking nodes=%d, want <= 60", resBare.Nodes)
		}
		if resNoPre.Nodes > 60 {
			t.Fatalf("orbital branching no-presolve nodes=%d, want <= 60", resNoPre.Nodes)
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

	ws := gb.AcquireWorkspace(lpProb.NumVars(), lpProb.NumConstraints())
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
	if got := lp.PurgeSlackCuts(0.05); got != 2 || lp.NumCuts() != 1 {
		t.Fatalf("PurgeSlackCuts removed %d rows (remaining=%d), want 2 removed and 1 remaining",
			got, lp.NumCuts())
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

// Verifies that near-integral big-M solutions whose rounded polishing point is strictly
// suboptimal never terminate or fathom prematurely at the root or at interior B&B nodes.
func TestPeerReviewRegressions_BigMPolishAndLPNumerics(t *testing.T) {
	ctx := t.Context()

	// 1. Root big-M facility location where fy = (1, 1e-7) rounds to feasible fy = (1, 0)
	// with cost 2011, whereas opening depot 1 achieves optimal cost 521.
	mFL := gb.NewModel("fl_3depot", gb.Minimize)
	fy0 := mFL.AddBinaryVar("y0")
	fy1 := mFL.AddBinaryVar("y1")
	fx00 := mFL.AddContVar("x00", 0, 10)
	fx01 := mFL.AddContVar("x01", 0, 10)
	fx11 := mFL.AddContVar("x11", 0, 10)
	mFL.SetObjective(gb.NewExpr(fy0.T(10), fy1.T(500), fx00.T(1), fx01.T(200), fx11.T(1)))
	mFL.AddConstraint("d0", gb.VarExpr(fx00).Ge(1))
	mFL.AddConstraint("d1", gb.NewExpr(fx01.T(1), fx11.T(1)).Ge(10))
	mFL.AddConstraint("v00", gb.NewExpr(fx00.T(1), fy0.T(-1)).Le(0))
	mFL.AddConstraint("v0", gb.NewExpr(fx00.T(1), fx01.T(1), fy0.T(-20)).Le(0))
	mFL.AddConstraint("v1", gb.NewExpr(fx11.T(1), fy1.T(-1e8)).Le(0))
	pFL := mFL.Compile()
	for name, opts := range optionVariants() {
		assertOptimal(t, "fl_root/"+name, pFL, mustSolveCompiled(t, pFL, opts...), 521.0)
	}

	// 2. Interior tree node big-M fathoming: embed fl_3depot behind a fractional binary split z0 + z1 == 1
	// so root must branch on z first and only encounters the near-integral big-M LP at depth >= 1.
	mNodeFL := gb.NewModel("fl_node", gb.Minimize)
	z0 := mNodeFL.AddBinaryVar("z0")
	z1 := mNodeFL.AddBinaryVar("z1")
	z2 := mNodeFL.AddBinaryVar("z2")
	ny0 := mNodeFL.AddBinaryVar("y0")
	ny1 := mNodeFL.AddBinaryVar("y1")
	nx00 := mNodeFL.AddContVar("x00", 0, 10)
	nx01 := mNodeFL.AddContVar("x01", 0, 10)
	nx11 := mNodeFL.AddContVar("x11", 0, 10)
	mNodeFL.SetObjective(gb.NewExpr(
		z0.T(2), z1.T(2), z2.T(2),
		ny0.T(10), ny1.T(500), nx00.T(1), nx01.T(200), nx11.T(1),
	))
	mNodeFL.AddConstraint("odd1", gb.NewExpr(z0.T(1), z1.T(1)).Ge(1))
	mNodeFL.AddConstraint("odd2", gb.NewExpr(z1.T(1), z2.T(1)).Ge(1))
	mNodeFL.AddConstraint("odd3", gb.NewExpr(z0.T(1), z2.T(1)).Ge(1))
	mNodeFL.AddConstraint("d0", gb.VarExpr(nx00).Ge(1))
	mNodeFL.AddConstraint("d1", gb.NewExpr(nx01.T(1), nx11.T(1)).Ge(10))
	mNodeFL.AddConstraint("v00", gb.NewExpr(nx00.T(1), ny0.T(-1)).Le(0))
	mNodeFL.AddConstraint("v0", gb.NewExpr(nx00.T(1), nx01.T(1), ny0.T(-20)).Le(0))
	mNodeFL.AddConstraint("v1", gb.NewExpr(nx11.T(1), ny1.T(-1e8)).Le(0))
	pNodeFL := mNodeFL.Compile()
	for name, opts := range optionVariants() {
		assertOptimal(t, "fl_node/"+name, pNodeFL, mustSolveCompiled(t, pNodeFL, opts...), 525.0)
	}

	// 3. LP scaling & Farkas certificate verification across extreme row/column/cost scales.
	// Uniformly tiny constraints (1e-8 * x >= 1e-8) must trigger scaling and solve accurately.
	mTiny := gb.NewModel("lp_uniform_tiny", gb.Minimize)
	tx := mTiny.AddContVar("tx", 0, 100)
	ty := mTiny.AddContVar("ty", 0, 100)
	mTiny.SetObjective(gb.NewExpr(tx.T(1e-8), ty.T(2e-8)))
	mTiny.AddConstraint("r0", gb.NewExpr(tx.T(1e-8), ty.T(1e-8)).Ge(3e-8))
	mTiny.AddConstraint("r1", gb.NewExpr(tx.T(2e-8), ty.T(1e-8)).Ge(4e-8))
	resTiny, err := mTiny.Solve(ctx)
	if err != nil || resTiny.Status != gb.StatusOptimal || math.Abs(resTiny.Objective-3e-8) > 1e-13 {
		t.Fatalf("lp_uniform_tiny: status=%v obj=%v err=%v, want Optimal 3e-8", resTiny.Status, resTiny.Objective, err)
	}

	// Huge intra-row / intra-objective dynamic range (1 vs 1e-8) with large variable bounds.
	mSpan := gb.NewModel("lp_wide_span", gb.Maximize)
	sx := mSpan.AddContVar("sx", 0, 1)
	sy := mSpan.AddContVar("sy", 0, 1e9)
	mSpan.SetObjective(gb.NewExpr(sx.T(1.0), sy.T(1e-7)))
	mSpan.AddConstraint("cap", gb.NewExpr(sx.T(1.0), sy.T(1e-8)).Le(2.0))
	resSpan, err := mSpan.Solve(ctx)
	if err != nil || resSpan.Status != gb.StatusOptimal || math.Abs(resSpan.Objective-20.0) > 1e-4 {
		t.Fatalf("lp_wide_span: status=%v obj=%v err=%v, want Optimal 20.0", resSpan.Status, resSpan.Objective, err)
	}

	// 4. Exact-zero compilation preserves sub-1e-14 nonzero coefficients.
	mSub := gb.NewModel("sub_1e15_coeff", gb.Minimize)
	subX := mSub.AddContVar("x", 0, 1e16)
	mSub.SetObjective(gb.VarExpr(subX))
	mSub.AddConstraint("c", gb.NewExpr(subX.T(1e-15)).Ge(1.0))
	pSub := mSub.Compile()
	if pSub.NNZ() != 1 {
		t.Fatalf("expected 1e-15 constraint coefficient to be preserved (NNZ=1), got NNZ=%d", pSub.NNZ())
	}

	// 5. DenseRows memory cap: n=256 with 5000 sparse 2-term rows stays sparse (DenseRowLen == 0).
	mTall := gb.NewModel("tall_sparse_256", gb.Minimize)
	tallVars := make([]gb.Var, 256)
	for j := range 256 {
		tallVars[j] = mTall.AddContVar(fmt.Sprintf("v_%d", j), 0, 10)
	}
	mTall.SetObjective(gb.VarExpr(tallVars[0]))
	for i := range 5000 {
		a := tallVars[i%256]
		b := tallVars[(i+1)%256]
		mTall.AddConstraint(fmt.Sprintf("r_%d", i), gb.NewExpr(a.T(1), b.T(1)).Ge(1))
	}
	pTall := mTall.Compile()
	if pTall.DenseRowLen() != 0 {
		t.Fatalf("expected sparse-only layout (DenseRowLen=0) on 5000x256 sparse matrix, got %d", pTall.DenseRowLen())
	}

	// 6. Presolve GCD normalization with large g (1e7) must not round unscaled fractional RHS.
	mGCD := gb.NewModel("gcd_large_scale", gb.Minimize)
	gx := mGCD.AddBinaryVar("gx")
	gy := mGCD.AddBinaryVar("gy")
	mGCD.SetObjective(gb.NewExpr(gx.T(1), gy.T(1)))
	// 1e7 * gx + 1e7 * gy == 1e7 + 20 -> gx + gy == 1.000002 -> integer infeasible!
	mGCD.AddConstraint("eq", gb.NewExpr(gx.T(1e7), gy.T(1e7)).Eq(1e7+20.0))
	pGCD := mGCD.Compile()
	for name, opts := range optionVariants() {
		if res := mustSolveCompiled(t, pGCD, opts...); res.Status != gb.StatusInfeasible {
			t.Fatalf("gcd_large_scale/%s: status=%v, want Infeasible", name, res.Status)
		}
	}

	// 7. API edge cases: invalid Sense, Status(99).String(), Result.Lookup, double ReleaseWorkspace,
	// and non-CGO CBCLPSolver nil safety.
	mBadSense := gb.NewModel("bad_sense", gb.Sense(42))
	_ = mBadSense.AddContVar("x", 0, 1)
	if err := mBadSense.Validate(); err == nil {
		t.Fatal("expected Validate() error on invalid Sense(42)")
	}
	if got := gb.Status(99).String(); got != "Status(99)" {
		t.Fatalf("Status(99).String() = %q, want \"Status(99)\"", got)
	}
	mLookup := gb.NewModel("lookup", gb.Minimize)
	lx := mLookup.AddIntVar("lx", -0.4, 0.0) // Tests -0.0 bound normalization as well.
	lbLx, ubLx := mLookup.VarBounds(lx)
	if math.Signbit(lbLx) || math.Signbit(ubLx) {
		t.Fatalf("expected +0.0 after integer bound rounding on [-0.4, 0], got [%v, %v]", lbLx, ubLx)
	}
	mLookup.SetObjective(gb.VarExpr(lx))
	resLookup, err := mLookup.Solve(ctx)
	if err != nil || resLookup.Status != gb.StatusOptimal {
		t.Fatalf("lookup solve: %v %v", resLookup.Status, err)
	}
	if val, ok := resLookup.Lookup(lx); !ok || val != 0 {
		t.Fatalf("Lookup(owned var) = (%v, %v), want (0, true)", val, ok)
	}
	otherM := gb.NewModel("other", gb.Minimize)
	ox := otherM.AddContVar("ox", 0, 1)
	if _, ok := resLookup.Lookup(ox); ok {
		t.Fatal("expected Lookup(foreign var) to return ok=false")
	}
	ws := gb.AcquireWorkspace(4, 4)
	gb.ReleaseWorkspace(ws)
	gb.ReleaseWorkspace(ws) // Double release must be a safe no-op.

	var nilCBC *gb.CBCLPSolver
	if nilCBC.TotalRows() != 0 || nilCBC.NumVars() != 0 || nilCBC.NumCuts() != 0 {
		t.Fatal("expected zero dimensions on nil *CBCLPSolver")
	}
	nilCBC.AddCut([]int{0}, []float64{1.0}, 1.0)
	nilCBC.SetStructuralBounds([]float64{0}, []float64{1})
	nilCBC.Close()
}

// TestCriticalLPAndCutSoundnessRegressions verifies C1 through C8 across default and bare
// solver configurations: tiny basis entries and Farkas verification, Ruiz bound overflow,
// GMI/knapsack/VUB/lot-sizing cut soundness, Rule B symmetry tightening, continuous bound
// propagation scaling, and epigraph heuristic candidate guards.
func TestCriticalLPAndCutSoundnessRegressions(t *testing.T) {
	ctx := t.Context()

	// C1: Tiny basis coefficient (5e-14) multiplied by x2 = 1e15 must neither be dropped in LU
	// factorization nor yield false Infeasible / false Optimal results.
	t.Run("C1_tiny_basis_coeff", func(t *testing.T) {
		// Case 1a: 2-row basis with x2 == 1e15, x1 + 5e-14*x2 == 55, x1 in [0, 10] -> witness (5, 1e15), obj=5.
		m1a := gb.NewModel("c1a", gb.Minimize)
		x1 := m1a.AddContVar("x1", 0, 10)
		x2 := m1a.AddContVar("x2", 0, math.Inf(1))
		m1a.SetObjective(gb.VarExpr(x1))
		m1a.AddConstraint("fix_x2", gb.VarExpr(x2).Eq(1e15))
		m1a.AddConstraint("eq", gb.NewExpr(x1.T(1), x2.T(5e-14)).Eq(55))
		p1a := m1a.Compile()
		assertOptimal(t, "c1a", p1a, mustSolveCompiled(t, p1a, gb.WithPresolve(false)), 5.0)

		// Case 1b: x1 in [0, 100], x2 == 1e15, x1 + 5e-14*x2 == 55 -> true opt x1=5 (not 55!).
		m1b := gb.NewModel("c1b", gb.Minimize)
		bx1 := m1b.AddContVar("x1", 0, 100)
		bx2 := m1b.AddContVar("x2", 0, math.Inf(1))
		m1b.SetObjective(gb.VarExpr(bx1))
		m1b.AddConstraint("fix_x2", gb.VarExpr(bx2).Eq(1e15))
		m1b.AddConstraint("eq", gb.NewExpr(bx1.T(1), bx2.T(5e-14)).Eq(55))
		p1b := m1b.Compile()
		assertOptimal(t, "c1b", p1b, mustSolveCompiled(t, p1b, gb.WithPresolve(false)), 5.0)

		// Case 1c: Phase-I primal path with cost[x2]=-1, x2 in [0, +Inf), x1 in [0, 10], x2 == 1e15.
		m1c := gb.NewModel("c1c", gb.Minimize)
		cx1 := m1c.AddContVar("x1", 0, 10)
		cx2 := m1c.AddContVar("x2", 0, math.Inf(1))
		m1c.SetObjective(gb.NewExpr(cx2.T(-1)))
		m1c.AddConstraint("fix_x2", gb.VarExpr(cx2).Eq(1e15))
		m1c.AddConstraint("eq", gb.NewExpr(cx1.T(1), cx2.T(5e-14)).Eq(55))
		p1c := m1c.Compile()
		assertOptimal(t, "c1c", p1c, mustSolveCompiled(t, p1c, gb.WithPresolve(false)), -1e15)
	})

	// C2: Ruiz scaling must not overflow finite bounds (1e305) to +Inf.
	t.Run("C2_ruiz_bound_overflow", func(t *testing.T) {
		m := gb.NewModel("c2_ruiz_ovf", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 1e305)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(gb.NewExpr(x0.T(-1)))
		m.AddConstraint("r0", gb.NewExpr(x0.T(1e20), x1.T(1)).Le(0))
		m.AddConstraint("r1", gb.VarExpr(x1).Le(10))
		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		res := mustSolveCompiled(t, p, gb.WithPresolve(false))
		if res.Status == gb.StatusUnbounded {
			t.Fatalf("bounded LP with ub=1e305 returned StatusUnbounded")
		}
		if res.Status != gb.StatusOptimal || math.Abs(res.Objective) > 1e-6 {
			t.Fatalf("c2_ruiz_ovf: status=%v obj=%v, want Optimal 0", res.Status, res.Objective)
		}
	})

	// C3: separateGMICuts must not drop unbounded coefficients on Ruiz-scaled tableaux.
	t.Run("C3_gmi_unbounded_drop", func(t *testing.T) {
		m := gb.NewModel("c3_gmi", gb.Minimize)
		x1 := m.AddIntVar("x1", 0, 10)
		x3 := m.AddContVar("x3", 0, 0.2)
		x4 := m.AddContVar("x4", 0, 0.6)
		z := m.AddContVar("z", 0, math.Inf(1))
		u := m.AddContVar("u", 0, 1)
		m.SetObjective(gb.NewExpr(x3.T(100), x4.T(1000), z.T(1e-14)))
		m.AddConstraint("eq", gb.NewExpr(x1.T(1), x3.T(1), x4.T(-1), z.T(8e-14)).Eq(0.6))
		m.AddConstraint("link", gb.NewExpr(u.T(1), z.T(-1e-8)).Le(0.5))
		p := m.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c3_gmi/"+name, p, mustSolveCompiled(t, p, opts...), 0.075)
		}
	})

	// C4: Knapsack cover and disaggregated VUB cuts must not silently drop tiny terms or
	// ignore nonzero RHS / negative lower bounds.
	t.Run("C4_knapsack_and_vub_cuts", func(t *testing.T) {
		// Knapsack cover with small term on huge-magnitude variable z in [-1e15, 0].
		mKC := gb.NewModel("c4_knapsack", gb.Maximize)
		k1 := mKC.AddBinaryVar("x1")
		k2 := mKC.AddBinaryVar("x2")
		k3 := mKC.AddBinaryVar("x3")
		kz := mKC.AddContVar("z", -1e15, 0)
		mKC.SetObjective(gb.NewExpr(k1.T(1), k2.T(1), k3.T(1)))
		mKC.AddConstraint("kc", gb.NewExpr(k1.T(6), k2.T(6), k3.T(6), kz.T(9e-13)).Le(10))
		pKC := mKC.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c4_kc/"+name, pKC, mustSolveCompiled(t, pKC, opts...), 3.0)
		}

		// Disaggregated VUB with tiny coefficient on z in [-1e15, 0].
		mVUB := gb.NewModel("c4_vub_tiny", gb.Minimize)
		vx1 := mVUB.AddContVar("x1", 0, 100)
		vx2 := mVUB.AddContVar("x2", 0, 100)
		vz := mVUB.AddContVar("z", -1e15, 0)
		vy := mVUB.AddBinaryVar("y")
		mVUB.SetObjective(gb.NewExpr(vx1.T(-1), vx2.T(-1), vy.T(10)))
		mVUB.AddConstraint("vub", gb.NewExpr(vx1.T(1), vx2.T(1), vz.T(9e-13), vy.T(-10)).Le(0))
		pVUB := mVUB.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c4_vub_tiny/"+name, pVUB, mustSolveCompiled(t, pVUB, opts...), -200.0)
		}

		// Disaggregated VUB with small positive RHS (1e-9) and small coeff (1e-10).
		mVUB2 := gb.NewModel("c4_vub_rhs", gb.Minimize)
		rx1 := mVUB2.AddContVar("x1", 0, 100)
		rx2 := mVUB2.AddContVar("x2", 0, 100)
		ry := mVUB2.AddBinaryVar("y")
		mVUB2.SetObjective(gb.NewExpr(ry.T(10)))
		mVUB2.AddConstraint("vub", gb.NewExpr(rx1.T(1e-10), rx2.T(1e-10), ry.T(-1)).Le(1e-9))
		mVUB2.AddConstraint("dem", gb.VarExpr(rx1).Ge(5))
		pVUB2 := mVUB2.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c4_vub_rhs/"+name, pVUB2, mustSolveCompiled(t, pVUB2, opts...), 0.0)
		}
	})

	// C5: Lot-sizing (l, S) cuts must require exact +-1.0 flow coefficients.
	t.Run("C5_lotsizing_unit_coeff", func(t *testing.T) {
		mLS := gb.NewModel("c5_lotsizing", gb.Minimize)
		p1 := mLS.AddContVar("p1", 0, 1e10)
		p2 := mLS.AddContVar("p2", 0, 10)
		inv0 := mLS.AddContVar("inv0", 0, 0)
		inv1 := mLS.AddContVar("inv1", 0, 1e10)
		inv2 := mLS.AddContVar("inv2", 0, 1e10)
		y1 := mLS.AddBinaryVar("y1")
		y2 := mLS.AddBinaryVar("y2")
		mLS.SetObjective(gb.NewExpr(y1.T(182), y2.T(1000)))
		// Period 1: inv0 + (1 + 9e-10)*p1 - inv1 == 1e10 (when p1=1e10, extra 9 units carry to inv1!)
		mLS.AddConstraint("bal1", gb.NewExpr(inv0.T(1), p1.T(1+9e-10), inv1.T(-1)).Eq(1e10))
		// Period 2: inv1 + p2 - inv2 == 9
		mLS.AddConstraint("bal2", gb.NewExpr(inv1.T(1), p2.T(1), inv2.T(-1)).Eq(9))
		mLS.AddConstraint("cap1", gb.NewExpr(p1.T(1), y1.T(-1e10)).Le(0))
		mLS.AddConstraint("cap2", gb.NewExpr(p2.T(1), y2.T(-10)).Le(0))
		pLS := mLS.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c5_lotsizing/"+name, pLS, mustSolveCompiled(t, pLS, opts...), 182.0)
		}
	})

	// C6: Symmetry breaking Rule B must require exact rhs <= 0 and exact lb >= 0.
	t.Run("C6_symmetry_rule_b", func(t *testing.T) {
		// Case 6a: Positive RHS slack 1e-12 amplified by small coefficient 2e-9.
		m6a := gb.NewModel("c6a_rhs", gb.Minimize)
		x0 := m6a.AddContVar("x0", 0, 1)
		x1 := m6a.AddContVar("x1", 0, 1)
		y0 := m6a.AddBinaryVar("y0")
		y1 := m6a.AddBinaryVar("y1")
		m6a.SetObjective(gb.NewExpr(y0.T(500), y1.T(1)))
		m6a.AddConstraint("sym_r0", gb.NewExpr(x0.T(2e-9), y0.T(-1)).Le(1e-12))
		m6a.AddConstraint("sym_r1", gb.NewExpr(x1.T(2e-9), y1.T(-1)).Le(1e-12))
		m6a.AddConstraint("link", gb.NewExpr(x0.T(1), y1.T(1)).Ge(1.0005))
		p6a := m6a.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c6a/"+name, p6a, mustSolveCompiled(t, p6a, opts...), 1.0)
		}

		// Case 6b: Non-orbit variable z in [-1e-12, 0] with huge coefficient 1e12.
		m6b := gb.NewModel("c6b_neg_lb", gb.Minimize)
		bx0 := m6b.AddContVar("x0", 0, 1)
		bx1 := m6b.AddContVar("x1", 0, 1)
		by0 := m6b.AddBinaryVar("y0")
		by1 := m6b.AddBinaryVar("y1")
		z := m6b.AddContVar("z", -1e-12, 0)
		m6b.SetObjective(gb.NewExpr(by0.T(100), by1.T(10), bx0.T(10)))
		m6b.AddConstraint("sym_r0", gb.NewExpr(z.T(1e12), bx0.T(1), by0.T(-1)).Le(0))
		m6b.AddConstraint("sym_r1", gb.NewExpr(z.T(1e12), bx1.T(1), by1.T(-1)).Le(0))
		m6b.AddConstraint("dem", gb.NewExpr(bx0.T(1), by1.T(1)).Ge(2))
		p6b := m6b.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c6b/"+name, p6b, mustSolveCompiled(t, p6b, opts...), 20.0)
		}
	})

	// C7: Continuous bound tightening with cancelling large terms must remain feasible in
	// presolve and inside B&B node propagation.
	t.Run("C7_continuous_bound_prop", func(t *testing.T) {
		m := gb.NewModel("c7_prop", gb.Minimize)
		b := m.AddBinaryVar("b")
		x := m.AddContVar("x", 0, 1)
		u := m.AddContVar("u", 1e14, 1e14+1)
		v := m.AddContVar("v", 1e14, 1e14+1)
		m.SetObjective(gb.NewExpr(b.T(1), x.T(1)))
		// Cancelling large terms 1e14 - 1e14 leave ~1e-2 roundoff; dividing by tiny coeff 1e-8 on x
		// would create >1e-6 false bound violation without scale-aware rowFeasTol/|a|.
		m.AddConstraint("r_le", gb.NewExpr(u.T(1), v.T(-1), x.T(1e-8), b.T(-1)).Le(0))
		m.AddConstraint("r_ge", gb.NewExpr(u.T(1), v.T(-1), x.T(1e-8), b.T(-1)).Ge(-1e-8))
		m.AddConstraint("b_force", gb.NewExpr(b.T(1), x.T(0.5)).Ge(0.6))
		p := m.Compile()
		for name, opts := range optionVariants() {
			res := mustSolveCompiled(t, p, opts...)
			if res.Status != gb.StatusOptimal {
				t.Fatalf("c7_prop/%s: status=%v, want Optimal", name, res.Status)
			}
		}
	})

	// C8: solveEpigraphComponent must not panic when nCol is unbounded with no finite breakpoints.
	t.Run("C8_epigraph_unbounded_ncands", func(t *testing.T) {
		m := gb.NewModel("c8_epigraph", gb.Minimize)
		b := m.AddBinaryVar("b")
		mVar := m.AddContVar("m", 0, 10)
		nVar := m.AddContVar("n", math.Inf(-1), math.Inf(1))
		e1 := m.AddContVar("e1", math.Inf(-1), 10)
		e2 := m.AddContVar("e2", math.Inf(-1), 10)
		m.SetObjective(gb.NewExpr(e1.T(1), e2.T(1), b.T(1)))
		m.AddConstraint("r1", gb.NewExpr(mVar.T(1), nVar.T(1e-13), e1.T(-1), b.T(-1)).Le(0))
		m.AddConstraint("r2", gb.NewExpr(mVar.T(2), nVar.T(-1e-13), e2.T(-1), b.T(-1)).Le(0))
		m.AddConstraint("lb", gb.VarExpr(e1).Ge(-10))
		m.AddConstraint("lb2", gb.VarExpr(e2).Ge(-10))
		res, err := m.Solve(ctx)
		if err != nil {
			t.Fatalf("c8_epigraph Solve error: %v", err)
		}
		if res.Status != gb.StatusOptimal && res.Status != gb.StatusUnbounded {
			t.Fatalf("c8_epigraph unexpected status: %v", res.Status)
		}
	})
}

// TestMajorAndMinorAPIRegressions verifies M1, M2, M5, M7, M8, M10, m3, and m4 on the pure-Go API.
func TestMajorAndMinorAPIRegressions(t *testing.T) {
	ctx := t.Context()

	// M1: WithOptions honors caller overrides rather than retaining DefaultOptions sentinel flags.
	t.Run("M1_with_options", func(t *testing.T) {
		opt := gb.DefaultOptions()
		opt.MaxNodes = 1
		opt.MaxCutPasses = 0
		opt.EnablePresolve = false
		opt.EnableHeuristics = false
		m := gb.NewModel("m1_opts", gb.Minimize)
		x := m.AddBinaryVar("x")
		y := m.AddBinaryVar("y")
		m.SetObjective(gb.NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("lo", gb.NewExpr(x.T(2), y.T(2)).Ge(1))
		m.AddConstraint("hi", gb.NewExpr(x.T(2), y.T(2)).Le(1))
		res := mustSolve(t, m, gb.WithOptions(opt))
		if res.CutsAdded != 0 {
			t.Fatalf("WithOptions MaxCutPasses=0 added %d cuts", res.CutsAdded)
		}
		if res.Nodes > 1 {
			t.Fatalf("WithOptions MaxNodes=1 explored %d nodes", res.Nodes)
		}
	})

	// M2: User constraint named __sym_ord_0 collides safely with Rule B disambiguation.
	t.Run("M2_sym_ord_name_collision", func(t *testing.T) {
		m := gb.NewModel("m2_sym_collision", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 1)
		x1 := m.AddContVar("x1", 0, 1)
		y0 := m.AddBinaryVar("y0")
		y1 := m.AddBinaryVar("y1")
		m.SetObjective(gb.NewExpr(y0.T(5), y1.T(5), x0.T(1), x1.T(1)))
		m.AddConstraint("__sym_ord_0", gb.NewExpr(x0.T(1), y0.T(-1)).Le(0))
		m.AddConstraint("r1", gb.NewExpr(x1.T(1), y1.T(-1)).Le(0))
		m.AddConstraint("dem", gb.NewExpr(x0.T(1), x1.T(1)).Ge(1))
		p := m.Compile()
		assertOptimal(t, "m2_sym_collision", p, mustSolveCompiled(t, p), 6.0)
	})

	// M5: MaxViolation returns +Inf on NaN continuous primal entries.
	t.Run("M5_max_violation_nan", func(t *testing.T) {
		m := gb.NewModel("m5_nan", gb.Minimize)
		x := m.AddContVar("x", 0, 10)
		y := m.AddContVar("y", 0, 10)
		m.SetObjective(gb.NewExpr(x.T(1), y.T(1)))
		m.AddConstraint("c", gb.NewExpr(x.T(1), y.T(1)).Ge(2))
		p := m.Compile()
		bV, rV, _ := p.MaxViolation([]float64{math.NaN(), 2.0}, true)
		if !math.IsInf(bV, 1) || !math.IsInf(rV, 1) {
			t.Fatalf("MaxViolation([NaN, 2]) = (%v, %v), want (+Inf, +Inf)", bV, rV)
		}
	})

	// M7 & M8: LPSolver rejects NaN/invalid bounds and non-finite/overflowing cuts cleanly.
	t.Run("M7_M8_lp_bounds_and_cuts_validation", func(t *testing.T) {
		m := gb.NewModel("m78", gb.Minimize)
		x := m.AddContVar("x", 0, 10)
		m.SetObjective(gb.VarExpr(x))
		m.AddConstraint("c", gb.VarExpr(x).Ge(1))
		p := m.Compile()

		// NaN bound -> StatusUndefined.
		s1 := gb.NewLPSolver(p, nil)
		s1.SetStructuralBounds([]float64{math.NaN()}, []float64{10})
		if r := s1.Solve(); r.Status != gb.StatusUndefined {
			t.Fatalf("NaN LB Solve status = %v, want StatusUndefined", r.Status)
		}
		// Recovering with valid bounds clears invalidBounds.
		s1.SetStructuralBounds([]float64{0}, []float64{10})
		if r := s1.Solve(); r.Status != gb.StatusOptimal {
			t.Fatalf("recovered LP Solve status = %v, want StatusOptimal", r.Status)
		}
		s1.Close()

		// +Inf LB -> StatusUndefined.
		s2 := gb.NewLPSolver(p, nil)
		s2.SetStructuralBounds([]float64{math.Inf(1)}, []float64{math.Inf(1)})
		if r := s2.Solve(); r.Status != gb.StatusUndefined {
			t.Fatalf("+Inf LB Solve status = %v, want StatusUndefined", r.Status)
		}
		s2.Close()

		// Non-finite cut RHS (NaN, +Inf) and non-finite val -> StatusUndefined.
		for _, tc := range []struct {
			name string
			cols []int
			vals []float64
			rhs  float64
			want gb.Status
		}{
			{"nan_rhs", []int{0}, []float64{1}, math.NaN(), gb.StatusUndefined},
			{"pos_inf_rhs", []int{0}, []float64{1}, math.Inf(1), gb.StatusOptimal},
			{"neg_inf_rhs", []int{0}, []float64{1}, math.Inf(-1), gb.StatusInfeasible},
			{"nan_val", []int{0}, []float64{math.NaN()}, 5, gb.StatusUndefined},
			{"dup_overflow", []int{0, 0}, []float64{1e308, 1e308}, 5, gb.StatusUndefined},
		} {
			s := gb.NewLPSolver(p, nil)
			_ = s.Solve()
			s.AddCut(tc.cols, tc.vals, tc.rhs)
			r := s.Solve()
			s.Close()
			if r.Status != tc.want {
				t.Fatalf("AddCut(%s): status=%v, want %v", tc.name, r.Status, tc.want)
			}
		}
	})

	// M10: Zero-value &Workspace{} and post-Close() Solve() must not panic.
	t.Run("M10_workspace_zero_and_post_close", func(t *testing.T) {
		m := gb.NewModel("m10", gb.Minimize)
		x := m.AddContVar("x", 0, 5)
		m.SetObjective(gb.VarExpr(x))
		m.AddConstraint("c", gb.VarExpr(x).Ge(2))
		p := m.Compile()

		var ws gb.Workspace
		s := gb.NewLPSolver(p, &ws)
		r := s.Solve()
		if r.Status != gb.StatusOptimal || math.Abs(r.Objective-2.0) > 1e-8 {
			t.Fatalf("zero-value Workspace LP solve: status=%v obj=%v", r.Status, r.Objective)
		}
		s.Close()
		if rClosed := s.Solve(); rClosed.Status != gb.StatusUndefined {
			t.Fatalf("post-Close Solve status = %v, want StatusUndefined", rClosed.Status)
		}
	})

	// m3: RelGapTol > 1.0 must not return StatusUndefined, and MaxNodes(1) bounds root sub-MIP.
	t.Run("m3_relgaptol_gt_1_and_maxnodes", func(t *testing.T) {
		m := gb.NewModel("m3_gap", gb.Minimize)
		x := m.AddIntVar("x", 0, 10)
		y := m.AddIntVar("y", 0, 10)
		m.SetObjective(gb.NewExpr(x.T(10), y.T(-50)))
		m.AddConstraint("c0", gb.NewExpr(x.T(2), y.T(3)).Le(5))
		m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(-1)).Ge(0))
		p := m.Compile()
		res := mustSolveCompiled(t, p, gb.WithRelGapTol(2.0), gb.WithHeuristics(false), gb.WithPresolve(false))
		if res.Status == gb.StatusUndefined {
			t.Fatalf("RelGapTol=2.0 returned StatusUndefined")
		}
		if res.Status != gb.StatusOptimal {
			t.Fatalf("RelGapTol=2.0 status = %v, want StatusOptimal", res.Status)
		}
	})

	// m4: Defensive nil / zero-value / negative-argument API guards.
	t.Run("m4_defensive_api_guards", func(t *testing.T) {
		var nilM *gb.Model
		if _, err := nilM.Solve(ctx); !errors.Is(err, gb.ErrNilModel) {
			t.Fatalf("(*Model)(nil).Solve err = %v, want ErrNilModel", err)
		}

		var zeroM gb.Model
		zv := zeroM.AddContVar("z", 0, 5)
		zeroM.SetObjective(gb.VarExpr(zv))
		if r, err := zeroM.Solve(ctx); err != nil || r.Status != gb.StatusOptimal {
			t.Fatalf("zero-value &Model{} Solve = (%v, %v), want Optimal", r.Status, err)
		}

		// Negative time limit terminates immediately with StatusTimeLimit.
		mTL := gb.NewModel("neg_tl", gb.Minimize)
		vx := mTL.AddContVar("x", 0, 5)
		mTL.SetObjective(gb.VarExpr(vx))
		resTL, err := mTL.Solve(ctx, gb.WithTimeLimit(-time.Second))
		if err != nil || resTL.Status != gb.StatusTimeLimit {
			t.Fatalf("negative TimeLimit: status=%v err=%v, want StatusTimeLimit", resTL.Status, err)
		}

		// Unknown Backend returns an error.
		if _, err := mTL.Solve(ctx, gb.WithBackend(gb.Backend(99))); err == nil {
			t.Fatal("expected error for unknown Backend(99)")
		}

		// Nil CompiledProblem helpers must not panic.
		var nilP *gb.CompiledProblem
		if nilP.ObjectiveFromMin(3.0) != 0 || nilP.EvaluateObjective(nil) != 0 {
			t.Fatal("unexpected nil CompiledProblem objective helper return")
		}
		if b, r, iv := nilP.MaxViolation(nil, true); !math.IsInf(b, 1) || !math.IsInf(r, 1) || !math.IsInf(iv, 1) {
			t.Fatalf("nil CompiledProblem MaxViolation = (%v, %v, %v), want (+Inf, +Inf, +Inf)", b, r, iv)
		}

		// Negative AcquireWorkspace dimensions must not panic.
		wsNeg := gb.AcquireWorkspace(-5, -3)
		gb.ReleaseWorkspace(wsNeg)

		// Root LSAP package aliases.
		if rA, err := gb.LinearSumAssignment(ctx, [][]float64{{4, 1}, {2, 3}}, gb.Minimize); err != nil || math.Abs(rA.Objective-3.0) > 1e-12 {
			t.Fatalf("LinearSumAssignment = (%+v, %v), want obj=3", rA, err)
		}
		if rF, err := gb.LinearSumAssignmentFlat(ctx, 2, 2, []float64{4, 1, 2, 3}, gb.Maximize); err != nil || math.Abs(rF.Objective-7.0) > 1e-12 {
			t.Fatalf("LinearSumAssignmentFlat = (%+v, %v), want obj=7", rF, err)
		}
	})
}

// TestAdversarialReviewRegressions covers Farkas cancellation on unbounded columns,
// zero-drop FTRANExact/BTRANExact optimality and primal degenerate fallback, exact
// orbit symmetry comparison, bound-shift cut relaxation, and shared sub-MIP budgets.
func TestAdversarialReviewRegressions(t *testing.T) {
	ctx := t.Context()

	// CRIT-1a: Moderate-scale feasible LPs where (w^T A)_j cancels to a tiny positive
	// residual on x >= 0 must solve to Optimal rather than false Infeasible.
	t.Run("CRIT1_farkas_unbounded_cancellation", func(t *testing.T) {
		for _, exp := range []int{20, 24, 26} {
			scaleM := math.Ldexp(1.0, exp)
			delta := math.Ldexp(scaleM, -48)
			m := gb.NewModel(fmt.Sprintf("farkas_m_%d", exp), gb.Minimize)
			x := m.AddContVar("x", 0, math.Inf(1))
			y := m.AddContVar("y", 0, math.Inf(1))
			m.AddConstraint("c1", gb.NewExpr(x.T(scaleM), y.T(-1)).Ge(10))
			m.AddConstraint("c2", gb.NewExpr(x.T(scaleM-delta), y.T(-1)).Le(0))
			p := m.Compile()
			witness := []float64{10.0 / delta, (scaleM - delta) * (10.0 / delta)}
			if bV, rV, _ := p.MaxViolation(witness, false); bV > 1e-7 || rV > 1e-7 {
				t.Fatalf("exp=%d witness violation (%e, %e)", exp, bV, rV)
			}
			res := mustSolveCompiled(t, p)
			if res.Status == gb.StatusInfeasible {
				t.Fatalf("exp=%d: false StatusInfeasible on feasible LP", exp)
			}
			if res.Status == gb.StatusOptimal {
				if bV, rV, _ := p.MaxViolation(res.Values, false); bV > 1e-5 || rV > 1e-5 {
					t.Fatalf("exp=%d: infeasible primal values (%e, %e)", exp, bV, rV)
				}
			}
		}

		// CRIT-1b: Three-row exact IEEE-754 cancellation (2^53 + 0.5 - 2^53) on x >= 0.
		bigM := math.Ldexp(1.0, 53)
		m3 := gb.NewModel("farkas_3row_cancel", gb.Minimize)
		x3 := m3.AddContVar("x", 0, math.Inf(1))
		y1 := m3.AddContVar("y1", 0, math.Inf(1))
		y2 := m3.AddContVar("y2", 0, math.Inf(1))
		m3.AddConstraint("r1", gb.NewExpr(x3.T(bigM), y1.T(-1), y2.T(1)).Ge(1))
		m3.AddConstraint("r2", gb.NewExpr(x3.T(0.5), y2.T(-1)).Ge(0))
		m3.AddConstraint("r3", gb.NewExpr(x3.T(bigM), y1.T(-1)).Le(0))
		p3 := m3.Compile()
		if res3 := mustSolveCompiled(t, p3); res3.Status == gb.StatusInfeasible {
			t.Fatalf("3-row exact-zero cancellation returned false StatusInfeasible")
		}

		// CRIT-1c: Genuinely infeasible LP with non-dyadic recession line (3.5*(x3-x4) vs -7.75*(x3-x4))
		// and free continuous variables constrained by row bounds must still certify StatusInfeasible.
		mRec := gb.NewModel("farkas_nondyadic_recession", gb.Minimize)
		rx0 := mRec.AddContVar("x0", 0, 1)
		rx3 := mRec.AddContVar("x3", 0, math.Inf(1))
		rx4 := mRec.AddContVar("x4", 0, math.Inf(1))
		mRec.AddConstraint("e0", gb.NewExpr(rx0.T(1), rx3.T(3.5), rx4.T(-3.5)).Eq(0))
		mRec.AddConstraint("g1", gb.NewExpr(rx0.T(-1), rx3.T(-7.75), rx4.T(7.75)).Ge(10))
		if resRec := mustSolveCompiled(t, mRec.Compile()); resRec.Status != gb.StatusInfeasible {
			t.Fatalf("farkas_nondyadic_recession status=%v, want StatusInfeasible", resRec.Status)
		}
	})

	// CRIT-2: Zero-drop FTRANExact on chained equalities and degenerate primal fallback
	// on large/infinite-span columns with sub-effDualTol reduced costs.
	t.Run("CRIT2_ftran_exact_and_degenerate_primal_fallback", func(t *testing.T) {
		mChain := gb.NewModel("ftran_chain_4", gb.Minimize)
		x1 := mChain.AddContVar("x1", math.Inf(-1), math.Inf(1))
		x2 := mChain.AddContVar("x2", math.Inf(-1), math.Inf(1))
		x3 := mChain.AddContVar("x3", math.Inf(-1), math.Inf(1))
		x4 := mChain.AddContVar("x4", math.Inf(-1), math.Inf(1))
		mChain.SetObjective(gb.VarExpr(x1))
		mChain.AddConstraint("e1", gb.NewExpr(x1.T(1e-4), x2.T(-1)).Eq(0))
		mChain.AddConstraint("e2", gb.NewExpr(x2.T(1e-4), x3.T(-1)).Eq(0))
		mChain.AddConstraint("e3", gb.NewExpr(x3.T(1e-4), x4.T(-1)).Eq(0))
		mChain.AddConstraint("e4", gb.NewExpr(x4.T(1e-4)).Eq(1e-13))
		pChain := mChain.Compile()
		resChain := mustSolveCompiled(t, pChain)
		if resChain.Status != gb.StatusOptimal || math.Abs(resChain.Objective-1000.0) > 1e-4 {
			t.Fatalf("ftran_chain_4: status=%v obj=%.6f, want Optimal 1000.0", resChain.Status, resChain.Objective)
		}

		// Non-negative chain under Maximize also pivots x1..x4 into the basis and requires FTRANExact.
		mChainMax := gb.NewModel("ftran_chain_4_max", gb.Maximize)
		mx1 := mChainMax.AddContVar("x1", 0, math.Inf(1))
		mx2 := mChainMax.AddContVar("x2", 0, math.Inf(1))
		mx3 := mChainMax.AddContVar("x3", 0, math.Inf(1))
		mx4 := mChainMax.AddContVar("x4", 0, math.Inf(1))
		mChainMax.SetObjective(gb.VarExpr(mx1))
		mChainMax.AddConstraint("e1", gb.NewExpr(mx1.T(1e-4), mx2.T(-1)).Eq(0))
		mChainMax.AddConstraint("e2", gb.NewExpr(mx2.T(1e-4), mx3.T(-1)).Eq(0))
		mChainMax.AddConstraint("e3", gb.NewExpr(mx3.T(1e-4), mx4.T(-1)).Eq(0))
		mChainMax.AddConstraint("e4", gb.NewExpr(mx4.T(1e-4)).Eq(1e-13))
		resChainMax := mustSolveCompiled(t, mChainMax.Compile())
		if resChainMax.Status != gb.StatusOptimal || math.Abs(resChainMax.Objective-1000.0) > 1e-4 {
			t.Fatalf("ftran_chain_4_max: status=%v obj=%.6f, want Optimal 1000.0", resChainMax.Status, resChainMax.Objective)
		}

		// Degenerate pivot on infinite-span column z with reduced cost -1e-8 (< effDualTol)
		// that improves objective from 1.3 to 0.4 once past the degenerate row.
		mDeg := gb.NewModel("degen_fallback_lp", gb.Minimize)
		u0 := mDeg.AddContVar("u0", 0, 1)
		u1 := mDeg.AddContVar("u1", 0, 1)
		z := mDeg.AddContVar("z", 0, math.Inf(1))
		mDeg.SetObjective(gb.NewExpr(u0.T(1), u1.T(1), z.T(-1e-8)).AddConstant(1.3))
		mDeg.AddConstraint("deg", gb.NewExpr(u0.T(1), u1.T(-1), z.T(-1e-9)).Le(0))
		mDeg.AddConstraint("cap", gb.NewExpr(z.T(1)).Le(9e7))
		pDeg := mDeg.Compile()
		resDeg := mustSolveCompiled(t, pDeg)
		if resDeg.Status != gb.StatusOptimal || math.Abs(resDeg.Objective-0.4) > 1e-4 {
			t.Fatalf("degen_fallback_lp: status=%v obj=%.6f, want Optimal 0.4", resDeg.Status, resDeg.Objective)
		}

		// MILP variant: RootLPBound must never exceed the optimal Minimize objective.
		mMilp := gb.NewModel("degen_root_bound_mip", gb.Minimize)
		b := mMilp.AddBinaryVar("b")
		mu0 := mMilp.AddContVar("u0", 0, 1)
		mu1 := mMilp.AddContVar("u1", 0, 1)
		mz := mMilp.AddContVar("z", 0, math.Inf(1))
		mMilp.SetObjective(gb.NewExpr(b.T(100), mu0.T(1), mu1.T(1), mz.T(-1e-8)).AddConstant(30.81))
		mMilp.AddConstraint("bmin", gb.VarExpr(b).Ge(1))
		mMilp.AddConstraint("deg", gb.NewExpr(mu0.T(1), mu1.T(-1), mz.T(-1e-9)).Le(0))
		mMilp.AddConstraint("cap", gb.NewExpr(mz.T(1)).Le(3.09e9))
		pMilp := mMilp.Compile()
		resMilp := mustSolveCompiled(t, pMilp)
		if resMilp.Status != gb.StatusOptimal || math.Abs(resMilp.Objective-99.91) > 1e-3 {
			t.Fatalf("degen_root_bound_mip: status=%v obj=%.6f, want Optimal 99.91", resMilp.Status, resMilp.Objective)
		}
		if resMilp.RootLPBound > resMilp.Objective+1e-5 || resMilp.RootCutBound > resMilp.Objective+1e-5 {
			t.Fatalf("degen_root_bound_mip inverted root bound: RootLP=%.6f RootCut=%.6f > Obj=%.6f",
				resMilp.RootLPBound, resMilp.RootCutBound, resMilp.Objective)
		}
	})

	// CRIT-3: Orbit symmetry detection and reduction require exact IEEE-754 equality so
	// 1e-12 perturbations scaled by 1e15 continuous domains never cut off global optima.
	t.Run("CRIT3_orbit_symmetry_exact_float_equality", func(t *testing.T) {
		// 3B: Objective perturbation 9e-13 <= 1e-12 across groups with 1e15 continuous variables.
		mObj := gb.NewModel("sym_obj_pert", gb.Minimize)
		y0 := mObj.AddBinaryVar("y0")
		y1 := mObj.AddBinaryVar("y1")
		x0 := mObj.AddContVar("x0", 0, 1e15)
		x1 := mObj.AddContVar("x1", 0, 1e15)
		mObj.SetObjective(gb.NewExpr(x0.T(-1e-13), x1.T(-1e-12)))
		mObj.AddConstraint("part", gb.NewExpr(y0.T(1), y1.T(1)).Eq(1))
		mObj.AddConstraint("vub0", gb.NewExpr(x0.T(1), y0.T(-1e15)).Le(0))
		mObj.AddConstraint("vub1", gb.NewExpr(x1.T(1), y1.T(-1e15)).Le(0))
		pObj := mObj.Compile()
		assertOptimal(t, "sym_obj_pert", pObj, mustSolveCompiled(t, pObj), -1000.0)

		// 3C: Group-invariant objective with 5e-13 matrix perturbation on 1e15 continuous columns.
		mMat := gb.NewModel("sym_mat_pert", gb.Minimize)
		my0 := mMat.AddBinaryVar("y0")
		my1 := mMat.AddBinaryVar("y1")
		mx0 := mMat.AddContVar("x0", 0, 1e15)
		mx1 := mMat.AddContVar("x1", 0, 1e15)
		mw := mMat.AddContVar("w", 0, 2000)
		mMat.SetObjective(gb.NewExpr(mw.T(-1)))
		mMat.AddConstraint("part", gb.NewExpr(my0.T(1), my1.T(1)).Eq(1))
		mMat.AddConstraint("vub0", gb.NewExpr(mx0.T(1), my0.T(-1e15)).Le(0))
		mMat.AddConstraint("vub1", gb.NewExpr(mx1.T(1), my1.T(-1e15)).Le(0))
		mMat.AddConstraint("link", gb.NewExpr(mw.T(1), mx0.T(-0.5e-12), mx1.T(-1.0e-12)).Le(0))
		pMat := mMat.Compile()
		assertOptimal(t, "sym_mat_pert", pMat, mustSolveCompiled(t, pMat), -1000.0)
	})

	// CRIT-4: Bound-shift cancellation at +-2^54 in knapsack cover and c-MIR separators
	// must never emit invalid cuts that cut off feasible integer optima.
	t.Run("CRIT4_cut_bound_shift_cancellation", func(t *testing.T) {
		shift := math.Ldexp(1.0, 54)

		// 4A: Knapsack cover with (18.0 - 2^54) + 2^54 naive rounding to 16.0.
		mKC := gb.NewModel("kc_cancel", gb.Maximize)
		kx1 := mKC.AddBinaryVar("x1")
		kx2 := mKC.AddBinaryVar("x2")
		kx3 := mKC.AddBinaryVar("x3")
		ksPos := mKC.AddContVar("sp", shift, shift)
		ksNeg := mKC.AddContVar("sn", shift, shift)
		mKC.SetObjective(gb.NewExpr(kx1.T(1), kx2.T(1), kx3.T(1)))
		mKC.AddConstraint("knap", gb.NewExpr(kx1.T(9), kx2.T(9), kx3.T(9), ksPos.T(1), ksNeg.T(-1)).Le(18))
		pKC := mKC.Compile()
		for _, useHeur := range []bool{true, false} {
			resKC := mustSolveCompiled(t, pKC, gb.WithPresolve(false), gb.WithHeuristics(useHeur))
			assertOptimal(t, fmt.Sprintf("kc_cancel/heur=%v", useHeur), pKC, resKC, 2.0)
			if resKC.RootCutBound < 2.0-1e-6 {
				t.Fatalf("kc_cancel/heur=%v invalid RootCutBound %.6f < 2.0", useHeur, resKC.RootCutBound)
			}
		}

		// 4B: c-MIR with (10.0 - 2^54) + 2^54 naive rounding to 8.0 (8/5 = 1.6 < 2).
		mMIR := gb.NewModel("mir_cancel", gb.Maximize)
		mx1 := mMIR.AddBinaryVar("x1")
		mx2 := mMIR.AddBinaryVar("x2")
		mx3 := mMIR.AddContVar("x3", 0, 1)
		msPos := mMIR.AddContVar("sp", shift, shift)
		msNeg := mMIR.AddContVar("sn", shift, shift)
		mMIR.SetObjective(gb.NewExpr(mx1.T(1), mx2.T(1), mx3.T(0.01)))
		mMIR.AddConstraint("row", gb.NewExpr(mx1.T(5), mx2.T(5), mx3.T(5), msPos.T(1), msNeg.T(-1)).Le(10))
		pMIR := mMIR.Compile()
		for _, useHeur := range []bool{true, false} {
			resMIR := mustSolveCompiled(t, pMIR, gb.WithPresolve(false), gb.WithHeuristics(useHeur))
			assertOptimal(t, fmt.Sprintf("mir_cancel/heur=%v", useHeur), pMIR, resMIR, 2.0)
			if resMIR.RootCutBound < 2.0-1e-6 {
				t.Fatalf("mir_cancel/heur=%v invalid RootCutBound %.6f < 2.0", useHeur, resMIR.RootCutBound)
			}
		}
	})

	// MAJ-4 & MIN-4: tryRootSubMIP shares MaxNodes and MaxIter with the parent search, and
	// solveUnboundedMIPFeasibility includes the root LP node in Result.Nodes.
	t.Run("MAJ4_MIN4_shared_submip_and_unbounded_mip_budgets", func(t *testing.T) {
		f, err := os.Open(filepath.Join("testdata", "corpus", "t2_assign_quad_lin.json"))
		if err == nil {
			mSub, pErr := format.FromPuLPJSON(f)
			_ = f.Close()
			if pErr == nil {
				pSub := mSub.Compile()
				resN, sErr := gb.SolveCompiled(ctx, pSub, gb.WithMaxNodes(8))
				if sErr != nil {
					t.Fatalf("submip MaxNodes(8) err: %v", sErr)
				}
				if resN.Nodes > 8 {
					t.Fatalf("submip MaxNodes(8) explored %d nodes (> 8)", resN.Nodes)
				}
				resIt, sErr := gb.SolveCompiled(ctx, pSub, gb.WithMaxIter(12))
				if sErr != nil {
					t.Fatalf("submip MaxIter(12) err: %v", sErr)
				}
				if resIt.Iterations > 12 {
					t.Fatalf("submip MaxIter(12) used %d iterations (> 12)", resIt.Iterations)
				}
			}
		}

		// LP-unbounded MIP where integer hull is infeasible after branching: Nodes must include root LP node.
		mUnb := gb.NewModel("unbounded_lp_infeas_mip", gb.Minimize)
		uz := mUnb.AddContVar("z", 0, math.Inf(1))
		ux := mUnb.AddIntVar("x", 0, 3)
		uy := mUnb.AddIntVar("y", 0, 3)
		mUnb.SetObjective(gb.NewExpr(uz.T(-1)))
		mUnb.AddConstraint("split_lo", gb.NewExpr(ux.T(2), uy.T(2)).Ge(3))
		mUnb.AddConstraint("split_hi", gb.NewExpr(ux.T(2), uy.T(2)).Le(3))
		pUnb := mUnb.Compile()
		resUnbLim, uErr := gb.SolveCompiled(ctx, pUnb, gb.WithPresolve(false), gb.WithCuts(false), gb.WithHeuristics(false), gb.WithMaxNodes(5))
		if uErr != nil || resUnbLim.Status != gb.StatusNodeLimit || resUnbLim.Nodes != 5 {
			t.Fatalf("unbounded_lp_infeas_mip MaxNodes(5): status=%v nodes=%d err=%v, want NodeLimit 5", resUnbLim.Status, resUnbLim.Nodes, uErr)
		}
	})

	// Nonbasic reduced-cost and GMI tableau cancellation at 2^-53 with 2^52 variables must never
	// certify suboptimal LPs as Optimal or emit GMI cuts that render feasible MILPs Infeasible.
	t.Run("CRIT2_CRIT4_nonbasic_cancellation_soundness", func(t *testing.T) {
		eps := math.Ldexp(1.0, -53)
		zMax := math.Ldexp(1.0, 53)

		mLP := gb.NewModel("lp_nonbasic_cancel", gb.Minimize)
		z := mLP.AddContVar("z", 0, zMax)
		s2 := mLP.AddContVar("s2", -10, zMax)
		s1 := mLP.AddContVar("s1", -10, 10)
		x := mLP.AddContVar("x", 0, 1)
		mLP.SetObjective(gb.NewExpr(x.T(100), z.T(1e-20)))
		mLP.AddConstraint("r0", gb.NewExpr(z.T(1.0), s2.T(-1), s1.T(-1), x.T(1)).Eq(0.5))
		mLP.AddConstraint("r1", gb.NewExpr(s1.T(1), z.T(eps)).Eq(0))
		mLP.AddConstraint("r2", gb.NewExpr(s2.T(1), z.T(-1.0)).Eq(0))
		pLP := mLP.Compile()
		wLP := []float64{math.Ldexp(1.0, 52), math.Ldexp(1.0, 52), -0.5, 0}
		if bV, rV, iV := pLP.MaxViolation(wLP, false); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("expected exact LP witness, got viol=(%g, %g, %g)", bV, rV, iV)
		}
		wLPObj := pLP.EvaluateObjective(wLP)
		resLP := mustSolveCompiled(t, pLP, gb.WithPureGo())
		if resLP.Status == gb.StatusOptimal && resLP.Objective > wLPObj+1e-3 {
			t.Fatalf("lp_nonbasic_cancel certified suboptimal obj=%.6f as Optimal (witness=%.6f)", resLP.Objective, wLPObj)
		}

		mMIP := gb.NewModel("milp_nonbasic_cancel", gb.Minimize)
		mz := mMIP.AddContVar("z", 0, zMax)
		ms2 := mMIP.AddContVar("s2", -10, zMax)
		ms1 := mMIP.AddContVar("s1", -10, 10)
		mx := mMIP.AddBinaryVar("x")
		my := mMIP.AddBinaryVar("y")
		mMIP.SetObjective(gb.NewExpr(mx.T(100), my.T(1), mz.T(1e-20)))
		mMIP.AddConstraint("pick", gb.NewExpr(mx.T(1), my.T(1)).Ge(1))
		mMIP.AddConstraint("r0", gb.NewExpr(mz.T(1.0), ms2.T(-1), ms1.T(-1), mx.T(1)).Eq(0.5))
		mMIP.AddConstraint("r1", gb.NewExpr(ms1.T(1), mz.T(eps)).Eq(0))
		mMIP.AddConstraint("r2", gb.NewExpr(ms2.T(1), mz.T(-1.0)).Eq(0))
		pMIP := mMIP.Compile()
		wMIP := []float64{math.Ldexp(1.0, 52), math.Ldexp(1.0, 52), -0.5, 0, 1}
		if bV, rV, iV := pMIP.MaxViolation(wMIP, true); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("expected exact MILP witness, got viol=(%g, %g, %g)", bV, rV, iV)
		}
		resMIP := mustSolveCompiled(t, pMIP)
		if resMIP.Status == gb.StatusInfeasible {
			t.Fatal("milp_nonbasic_cancel returned false StatusInfeasible on feasible MILP")
		}
	})

	// Upward IEEE-754 rounding of a/delta in separateRowMIRCuts must not cut off integer optima.
	t.Run("CRIT4_mir_quotient_rounding_soundness", func(t *testing.T) {
		delta := 5.0 + math.Ldexp(1.0, -50)
		a1 := 15.0 + math.Ldexp(1.0, -49)
		a2 := -(10.0 + math.Ldexp(1.0, -49))
		u1 := math.Ldexp(1.0, 49)
		u2 := 3.0 * math.Ldexp(1.0, 48)
		c1 := -20.0 * math.Ldexp(1.0, -49)

		mMIR := gb.NewModel("mir_quotient_round", gb.Maximize)
		y0 := mMIR.AddBinaryVar("x0")
		y1 := mMIR.AddIntVar("x1", 0, u1)
		y2 := mMIR.AddIntVar("x2", 0, u2)
		mMIR.SetObjective(gb.NewExpr(y0.T(100), y1.T(c1)))
		mMIR.AddConstraint("r", gb.NewExpr(y0.T(delta), y1.T(a1), y2.T(a2)).Le(4.9))
		mMIR.AddConstraint("cone", gb.NewExpr(y1.T(3), y2.T(-2)).Ge(0))
		pMIR := mMIR.Compile()
		w := []float64{1, u1, u2}
		if bV, rV, iV := pMIR.MaxViolation(w, true); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("expected exact MIR witness, got viol=(%g, %g, %g)", bV, rV, iV)
		}
		for _, useHeur := range []bool{true, false} {
			res := mustSolveCompiled(t, pMIR, gb.WithPresolve(false), gb.WithHeuristics(useHeur))
			if res.Status == gb.StatusInfeasible || (res.Status == gb.StatusOptimal && res.Objective < 80.0-1e-5) {
				t.Fatalf("mir_quotient/heur=%v cut off optimum: status=%v obj=%.6f", useHeur, res.Status, res.Objective)
			}
			if res.RootCutBound < 80.0-1e-5 || res.BestBound < 80.0-1e-5 {
				t.Fatalf("mir_quotient/heur=%v invalid dual bound: rootCut=%.6f best=%.6f (< 80.0)",
					useHeur, res.RootCutBound, res.BestBound)
			}
		}
	})

	// Exact rational Farkas singleton peeling certifies m > 64 infeasible LPs with non-dyadic free rays,
	// and EvaluateObjective handles canceling +-1.5e308 partial sums without intermediate overflow.
	t.Run("CRIT1_large_m_rational_farkas_and_huge_obj_sum", func(t *testing.T) {
		mF := gb.NewModel("farkas_m66_3_7", gb.Minimize)
		fx := mF.AddContVar("x", math.Inf(-1), math.Inf(1))
		fy := mF.AddContVar("y", math.Inf(-1), math.Inf(1))
		mF.AddConstraint("c0", gb.NewExpr(fx.T(3), fy.T(-3)).Le(0))
		mF.AddConstraint("c1", gb.NewExpr(fx.T(7), fy.T(-7)).Ge(1))
		for k := range 64 {
			fz := mF.AddContVar(fmt.Sprintf("z_%d", k), 0, 10)
			mF.AddConstraint(fmt.Sprintf("dz_%d", k), gb.NewExpr(fz.T(1)).Le(5))
		}
		resF := mustSolveCompiled(t, mF.Compile(), gb.WithPureGo())
		if resF.Status != gb.StatusInfeasible {
			t.Fatalf("farkas_m66_3_7: status=%v, want Infeasible", resF.Status)
		}

		cBig := 1.5e308
		mBig := gb.NewModel("go_recomp_overflow", gb.Minimize)
		bx0 := mBig.AddIntVar("x0", 1, 1)
		bx1 := mBig.AddIntVar("x1", 1, 1)
		bx2 := mBig.AddIntVar("x2", 1, 1)
		mBig.SetObjective(gb.NewExpr(bx0.T(cBig), bx1.T(cBig), bx2.T(-cBig)))
		mBig.AddConstraint("r", gb.NewExpr(bx0.T(1), bx1.T(1), bx2.T(1)).Ge(3))
		resBig := mustSolveCompiled(t, mBig.Compile(), gb.WithPureGo())
		if resBig.Status != gb.StatusOptimal || math.IsNaN(resBig.Objective) || math.Abs(resBig.Objective-cBig) > 1e295 {
			t.Fatalf("go_recomp_overflow: status=%v obj=%v, want Optimal %.5e", resBig.Status, resBig.Objective, cBig)
		}
	})

	// Small negative reduced costs on genuinely unbounded LPs/MILPs must certify StatusUnbounded
	// rather than being masked by unconditional 1e-14 / 1e-12 / 1e-11*l1 dual tolerances, while
	// bounded degenerate models with tiny objective coefficients still solve to StatusOptimal.
	t.Run("unbounded_small_negative_reduced_costs", func(t *testing.T) {
		for _, cTiny := range []float64{-5e-13, -5e-15, -5e-20} {
			mRay := gb.NewModel("coupled_unb_ray", gb.Minimize)
			x0 := mRay.AddContVar("x0", 0, math.Inf(1))
			z := mRay.AddContVar("z", 0, math.Inf(1))
			x1 := mRay.AddContVar("x1", 0, 10)
			x2 := mRay.AddContVar("x2", 0, 10)
			mRay.SetObjective(gb.NewExpr(x0.T(cTiny), x1.T(1), x2.T(1)))
			mRay.AddConstraint("r0", gb.NewExpr(x1.T(1), x0.T(500), z.T(-500)).Ge(1))
			mRay.AddConstraint("r1", gb.NewExpr(x2.T(1), x0.T(-500), z.T(500)).Ge(1))
			resRay := mustSolveCompiled(t, mRay.Compile(), gb.WithPureGo())
			if resRay.Status != gb.StatusUnbounded {
				t.Fatalf("coupled_unb_ray(c=%.2e): status=%v, want Unbounded", cTiny, resRay.Status)
			}
		}

		mSimple := gb.NewModel("simple_unb_5e15", gb.Minimize)
		sx0 := mSimple.AddContVar("x0", 0, math.Inf(1))
		sx1 := mSimple.AddContVar("x1", 0, 1)
		mSimple.SetObjective(gb.NewExpr(sx0.T(-5e-15), sx1.T(1)))
		mSimple.AddConstraint("c", gb.NewExpr(sx1.T(1)).Le(1))
		if res := mustSolveCompiled(t, mSimple.Compile(), gb.WithPureGo()); res.Status != gb.StatusUnbounded {
			t.Fatalf("simple_unb_5e15: status=%v, want Unbounded", res.Status)
		}

		// Slack-driven unbounded ray where after x0 enters basis, slack s0 in (-Inf, 0] has dj = +1e-15.
		mSlack := gb.NewModel("slack_unb_ray", gb.Minimize)
		kx0 := mSlack.AddContVar("x0", 0, math.Inf(1))
		kx1 := mSlack.AddContVar("x1", 0, 10)
		mSlack.SetObjective(gb.NewExpr(kx0.T(-5e-13), kx1.T(1)))
		mSlack.AddConstraint("ge", gb.NewExpr(kx1.T(1), kx0.T(500)).Ge(1))
		if res := mustSolveCompiled(t, mSlack.Compile(), gb.WithPureGo()); res.Status != gb.StatusUnbounded {
			t.Fatalf("slack_unb_ray: status=%v, want Unbounded", res.Status)
		}

		// Negative control: x0 in [0, +Inf) is bounded above by x1 in [0, 1] via x0 - x1 <= 0.
		mBounded := gb.NewModel("bounded_tiny_neg_obj", gb.Minimize)
		bx0 := mBounded.AddContVar("x0", 0, math.Inf(1))
		bx1 := mBounded.AddContVar("x1", 0, 1)
		mBounded.SetObjective(gb.NewExpr(bx0.T(-5e-15), bx1.T(1)))
		mBounded.AddConstraint("cap", gb.NewExpr(bx0.T(1), bx1.T(-1)).Le(0))
		resBnd := mustSolveCompiled(t, mBounded.Compile(), gb.WithPureGo())
		if resBnd.Status != gb.StatusOptimal || math.Abs(resBnd.Objective) > 1e-12 {
			t.Fatalf("bounded_tiny_neg_obj: status=%v obj=%g, want Optimal 0", resBnd.Status, resBnd.Objective)
		}

		// Decoupled high-dual row (y = 1e20) must not leak a global ddFloor into independent
		// coupled or slack-driven unbounded rays.
		mDecoupled := gb.NewModel("decoupled_high_dual_ray", gb.Minimize)
		dx0 := mDecoupled.AddContVar("x0", 0, math.Inf(1))
		dz := mDecoupled.AddContVar("z", 0, math.Inf(1))
		dx1 := mDecoupled.AddContVar("x1", 0, 1e10)
		dxw := mDecoupled.AddContVar("xw", 0, 1e305)
		mDecoupled.SetObjective(gb.NewExpr(dx0.T(-5e-12), dx1.T(1e14), dxw.T(1e25)))
		mDecoupled.AddConstraint("big_dual", gb.NewExpr(dx1.T(1e-6), dxw.T(1.0)).Eq(1))
		mDecoupled.AddConstraint("r1", gb.NewExpr(dx0.T(500), dz.T(-500)).Le(1))
		mDecoupled.AddConstraint("r2", gb.NewExpr(dx0.T(-500), dz.T(500)).Le(1))
		if res := mustSolveCompiled(t, mDecoupled.Compile(), gb.WithPureGo()); res.Status != gb.StatusUnbounded {
			t.Fatalf("decoupled_high_dual_ray: status=%v, want Unbounded", res.Status)
		}

		mSlackDec := gb.NewModel("decoupled_slack_ray", gb.Minimize)
		ux0 := mSlackDec.AddContVar("x0", 0, math.Inf(1))
		ux1 := mSlackDec.AddContVar("x1", 0, 10)
		uxw := mSlackDec.AddContVar("xw", 0, 1e305)
		mSlackDec.SetObjective(gb.NewExpr(ux0.T(-5e-12), ux1.T(1e20), uxw.T(1e25)))
		mSlackDec.AddConstraint("ge0", gb.VarExpr(ux0).Ge(1))
		mSlackDec.AddConstraint("eq1", gb.NewExpr(ux1.T(1), uxw.T(1e-5)).Eq(1))
		if res := mustSolveCompiled(t, mSlackDec.Compile(), gb.WithPureGo()); res.Status != gb.StatusUnbounded {
			t.Fatalf("decoupled_slack_ray: status=%v, want Unbounded", res.Status)
		}
	})

	// Row activities with canceling +-1.5e308 terms must not overflow to +-Inf or NaN in
	// MaxViolation, LPSolver residual checks, or MIP presolve row dropping.
	t.Run("canceling_huge_row_activity_no_overflow", func(t *testing.T) {
		cBig := 1.5e308
		mFeas := gb.NewModel("huge_row_feas", gb.Minimize)
		x0 := mFeas.AddContVar("x0", 1, 1)
		x1 := mFeas.AddContVar("x1", 1, 1)
		x2 := mFeas.AddContVar("x2", 0, 2)
		// Include a wide-bound tiny-coeff dummy column so Ruiz scaling falls back to unscaled LP.
		x3 := mFeas.AddContVar("x3", 0, 1e305)
		mFeas.SetObjective(gb.NewExpr(x2.T(1)))
		mFeas.AddConstraint("cancel_eq", gb.NewExpr(x0.T(cBig), x1.T(cBig), x2.T(-cBig)).Eq(cBig))
		mFeas.AddConstraint("tiny_cap", gb.NewExpr(x3.T(1e-10)).Le(1))
		pFeas := mFeas.Compile()

		if bV, rV, iV := pFeas.MaxViolation([]float64{1, 1, 1, 0}, false); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("MaxViolation on exact canceling 1.5e308 witness = (%v, %v, %v), want (0, 0, 0)", bV, rV, iV)
		}
		resFeas := mustSolveCompiled(t, pFeas, gb.WithPureGo())
		if resFeas.Status != gb.StatusOptimal || math.Abs(resFeas.Objective-1.0) > 1e-6 {
			t.Fatalf("huge_row_feas LP: status=%v obj=%v, want Optimal 1.0", resFeas.Status, resFeas.Objective)
		}

		// Infeasible MIP row (-1.5e308 - 1.5e308 + 1.7e308 = -1.3e308 > -1.4e308): naive float64
		// evaluates -Inf + 1.7e308 = -Inf <= -1.4e308, which previously hid a 1e307 violation.
		mInfMIP := gb.NewModel("huge_row_inf_mip", gb.Minimize)
		ix0 := mInfMIP.AddIntVar("x0", 1, 1)
		ix1 := mInfMIP.AddIntVar("x1", 1, 1)
		ix2 := mInfMIP.AddIntVar("x2", 1, 1)
		mInfMIP.SetObjective(gb.NewExpr(ix0.T(1)))
		mInfMIP.AddConstraint("bad_le", gb.NewExpr(ix0.T(-cBig), ix1.T(-cBig), ix2.T(1.7e308)).Le(-1.4e308))
		pInfMIP := mInfMIP.Compile()
		if _, rV, _ := pInfMIP.MaxViolation([]float64{1, 1, 1}, true); !(rV > 1e306) {
			t.Fatalf("MaxViolation on violated canceling row = %v, want > 1e306", rV)
		}
		resInfMIP := mustSolveCompiled(t, pInfMIP, gb.WithPureGo())
		if resInfMIP.Status != gb.StatusInfeasible {
			t.Fatalf("huge_row_inf_mip: status=%v, want Infeasible", resInfMIP.Status)
		}

		// Canceling 1e200 * 1e200 = 1e400 products (exceeding 2^1024 * 2^32) must evaluate
		// exactly in MaxViolation, EvaluateObjective, and SolveCompiled.
		m1e400 := gb.NewModel("cancel_1e400_products", gb.Minimize)
		px0 := m1e400.AddContVar("x0", 1e200, 1e200)
		px1 := m1e400.AddContVar("x1", 1e200, 1e200)
		px2 := m1e400.AddContVar("x2", 3, 5)
		m1e400.SetObjective(gb.NewExpr(px0.T(1e200), px1.T(-1e200), px2.T(1)))
		m1e400.AddConstraint("ge4", gb.NewExpr(px0.T(1e200), px1.T(-1e200), px2.T(1)).Ge(4))
		p1e400 := m1e400.Compile()
		if bV, rV, iV := p1e400.MaxViolation([]float64{1e200, 1e200, 4}, false); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("1e400 witness MaxViolation = (%v, %v, %v), want (0, 0, 0)", bV, rV, iV)
		}
		if obj := p1e400.EvaluateObjective([]float64{1e200, 1e200, 4}); obj != 4 {
			t.Fatalf("1e400 witness EvaluateObjective = %v, want 4", obj)
		}
		res1e400 := mustSolveCompiled(t, p1e400, gb.WithPureGo())
		if res1e400.Status != gb.StatusOptimal || math.Abs(res1e400.Objective-4.0) > 1e-8 {
			t.Fatalf("cancel_1e400_products: status=%v obj=%v, want Optimal 4.0", res1e400.Status, res1e400.Objective)
		}

		// Order-independent duplicate +-1.5e308 terms in Validate, Compile, and LPSolver.AddCut.
		mDup := gb.NewModel("dup_terms_order_indep", gb.Minimize)
		dv := mDup.AddContVar("x", 0, 1)
		mDup.SetObjective(gb.NewExpr(dv.T(cBig), dv.T(cBig), dv.T(-cBig)))
		mDup.AddConstraint("c", gb.NewExpr(dv.T(cBig), dv.T(cBig), dv.T(-cBig)).Ge(cBig))
		if err := mDup.Validate(); err != nil {
			t.Fatalf("Validate rejected canceling duplicate 1.5e308 terms: %v", err)
		}
		pDup := mDup.Compile()
		lpDup := gb.NewLPSolver(pDup, nil)
		lpDup.AddCut([]int{0, 0, 0}, []float64{cBig, cBig, -cBig}, cBig)
		resDup := lpDup.Solve()
		lpDup.Close()
		if resDup.Status != gb.StatusOptimal || math.Abs(resDup.Objective-cBig) > 1e295 {
			t.Fatalf("lpDup Solve: status=%v obj=%v, want Optimal %.5e", resDup.Status, resDup.Objective, cBig)
		}
	})
}

// TestPeerReviewV2Regressions covers the v1.0.0 peer-review numerical, presolve, heuristic,
// and simplex dual-refinement findings on pure-Go LP and MILP paths.
func TestPeerReviewV2Regressions(t *testing.T) {
	t.Run("c1_presolve_activity_and_cancellation_milp", func(t *testing.T) {
		// Presolve activity tightening and redundant-row detection must not absorb O(1) terms
		// into +-1e17 cancellation (exact optimum is z = 0, not z = 10).
		mPre := gb.NewModel("c1_presolve_false_opt", gb.Maximize)
		px0 := mPre.AddContVar("x0", 1, 1)
		px1 := mPre.AddContVar("x1", 1, 1)
		pz := mPre.AddIntVar("z", 0, 10)
		mPre.SetObjective(gb.VarExpr(pz))
		mPre.AddConstraint("c", gb.NewExpr(px0.T(1e17), px1.T(-1e17), pz.T(7)).Le(0))
		pPre := mPre.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c1_presolve/"+name, pPre, mustSolveCompiled(t, pPre, opts...), 0.0)
		}

		// Duplicate-variable +-1e17 cancellation in Validate, Compile, MaxViolation, and AddCut.
		mDup := gb.NewModel("c1_dup_1e17", gb.Minimize)
		dx := mDup.AddContVar("x", 0, 10)
		mDup.SetObjective(gb.NewExpr(dx.T(1e17), dx.T(9), dx.T(-1e17)))
		mDup.AddConstraint("ge9", gb.NewExpr(dx.T(1e17), dx.T(9), dx.T(-1e17)).Ge(9))
		if err := mDup.Validate(); err != nil {
			t.Fatalf("Validate duplicate +-1e17 error: %v", err)
		}
		pDup := mDup.Compile()
		if obj := pDup.EvaluateObjective([]float64{1}); obj != 9 {
			t.Fatalf("EvaluateObjective duplicate +-1e17 = %v, want 9", obj)
		}
		if bV, rV, iV := pDup.MaxViolation([]float64{1}, false); bV != 0 || rV != 0 || iV != 0 {
			t.Fatalf("MaxViolation duplicate +-1e17 = (%v, %v, %v), want (0, 0, 0)", bV, rV, iV)
		}
		lpDup := gb.NewLPSolver(pDup, nil)
		lpDup.AddCut([]int{0, 0, 0}, []float64{1e17, -1, -1e17}, -2)
		rDup := lpDup.Solve()
		lpDup.Close()
		if rDup.Status != gb.StatusOptimal || math.Abs(rDup.Objective-18) > 1e-9 {
			t.Fatalf("LPSolver duplicate +-1e17: status=%v obj=%v, want Optimal 18", rDup.Status, rDup.Objective)
		}

		// Binary knapsack under +-1e17 fixed-column cancellation: 5*b0 + 7*b1 + 8*b2 <= 14 -> -22
		// (since 7 + 8 = 15 > 14 is infeasible, optimal is b0=1, b2=1 with 5 + 8 = 13 <= 14).
		mBin := gb.NewModel("c1_bin_cancel", gb.Minimize)
		bx0 := mBin.AddContVar("x0", 1, 1)
		bx1 := mBin.AddContVar("x1", 1, 1)
		b0 := mBin.AddBinaryVar("b0")
		b1 := mBin.AddBinaryVar("b1")
		b2 := mBin.AddBinaryVar("b2")
		mBin.SetObjective(gb.NewExpr(b0.T(-10), b1.T(-11), b2.T(-12)))
		mBin.AddConstraint("cap", gb.NewExpr(bx0.T(1e17), bx1.T(-1e17), b0.T(5), b1.T(7), b2.T(8)).Le(14))
		pBin := mBin.Compile()
		for _, name := range []string{"default", "no-heuristics"} {
			assertOptimal(t, "c1_bin/"+name, pBin, mustSolveCompiled(t, pBin, optionVariants()[name]...), -22.0)
		}
		for _, name := range []string{"bare", "bare-no-presolve"} {
			res := mustSolveCompiled(t, pBin, optionVariants()[name]...)
			if res.Status == gb.StatusOptimal {
				assertOptimal(t, "c1_bin/"+name, pBin, res, -22.0)
			} else if res.Status != gb.StatusNumericalError || res.BestBound > -22.0+1e-6 {
				t.Fatalf("c1_bin/%s: status=%v obj=%v bound=%v, want Optimal -22 or NumericalError with bound <= -22",
					name, res.Status, res.Objective, res.BestBound)
			}
		}

		// Unbounded integer tightening under +-1e17 cancellation: z <= 16 -> max z = 16.
		mUnbInt := gb.NewModel("c1_unb_int_cancel", gb.Maximize)
		ux0 := mUnbInt.AddContVar("x0", 1, 1)
		ux1 := mUnbInt.AddContVar("x1", 1, 1)
		uz := mUnbInt.AddIntVar("z", 0, math.Inf(1))
		mUnbInt.SetObjective(gb.VarExpr(uz))
		mUnbInt.AddConstraint("cap", gb.NewExpr(ux0.T(1e17), ux1.T(-1e17), uz.T(1)).Le(16))
		pUnbInt := mUnbInt.Compile()
		for name, opts := range optionVariants() {
			assertOptimal(t, "c1_unb_int/"+name, pUnbInt, mustSolveCompiled(t, pUnbInt, opts...), 16.0)
		}

		// Savelsbergh binary coefficient tightening with 1e17 upper bound must not absorb O(1) delta.
		mSav := gb.NewModel("c1_savelsbergh_large_rhs", gb.Maximize)
		sx := mSav.AddContVar("x", 0, 1e17)
		sb := mSav.AddBinaryVar("b")
		mSav.SetObjective(gb.NewExpr(sx.T(1), sb.T(100)))
		mSav.AddConstraint("c", gb.NewExpr(sx.T(1), sb.T(10)).Le(1e17))
		pSav := mSav.Compile()
		resSav := mustSolveCompiled(t, pSav)
		if resSav.Status != gb.StatusOptimal {
			t.Fatalf("c1_savelsbergh_large_rhs: status=%v, want Optimal", resSav.Status)
		}
		if bV, rV, iV := pSav.MaxViolation(resSav.Values, true); bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
			t.Fatalf("c1_savelsbergh_large_rhs violation = (%v, %v, %v)", bV, rV, iV)
		}
	})

	t.Run("c2_heuristic_partial_sum_overflow", func(t *testing.T) {
		for _, sense := range []gb.Sense{gb.Minimize, gb.Maximize} {
			sign := 1.0
			if sense == gb.Maximize {
				sign = -1.0
			}
			m := gb.NewModel("c2_heur_overflow", sense)
			x0 := m.AddIntVar("x0", 1, 1)
			x1 := m.AddIntVar("x1", 1, 1)
			x2 := m.AddIntVar("x2", 1, 1)
			y0 := m.AddIntVar("y0", 1, 1)
			y1 := m.AddIntVar("y1", 1, 1)
			m.SetObjective(gb.NewExpr(
				x0.T(sign*-0.7e308),
				x1.T(sign*-0.7e308),
				x2.T(sign*-0.7e308),
				y0.T(sign*1.1e308),
				y1.T(sign*1.1e308),
			))
			m.AddConstraint("c", gb.NewExpr(x0.T(1), x1.T(1), x2.T(1), y0.T(1), y1.T(1)).Eq(5))
			res := mustSolve(t, m, gb.WithPresolve(false), gb.WithHeuristics(true))
			want := sign * 1e307
			if res.Status != gb.StatusOptimal || math.IsInf(res.Objective, 0) || math.IsInf(res.BestBound, 0) {
				t.Fatalf("sense=%v: status=%v obj=%v bound=%v, want finite Optimal %.3e",
					sense, res.Status, res.Objective, res.BestBound, want)
			}
			if math.Abs(res.Objective-want) > 1e295 || math.Abs(res.BestBound-want) > 1e295 {
				t.Fatalf("sense=%v: obj=%v bound=%v, want %.3e", sense, res.Objective, res.BestBound, want)
			}
		}
	})

	t.Run("c3_working_dual_refinement_component_bound", func(t *testing.T) {
		// Synthetic disconnected model: 120-row equality component with O(1000) duals plus an
		// independent GreaterEqual row whose basic slack carries a 0x1p-101 roundoff artifact.
		m := gb.NewModel("c3_disconnected_dual_floor", gb.Minimize)
		var prev gb.Var
		obj := gb.NewExpr()
		for k := range 120 {
			v := m.AddContVar(fmt.Sprintf("v_%d", k), 0, 10)
			obj = obj.AddTerm(v, 1000.0)
			if k > 0 {
				m.AddConstraint(fmt.Sprintf("eq_%d", k), gb.NewExpr(v.T(1), prev.T(-1)).Eq(0))
			} else {
				m.AddConstraint("eq_0", gb.VarExpr(v).Ge(1))
			}
			prev = v
		}
		freeSlack := m.AddContVar("indep_hi", 0, math.Inf(1))
		obj = obj.AddTerm(freeSlack, 1.0)
		m.SetObjective(obj)
		m.AddConstraint("indep_ge", gb.VarExpr(freeSlack).Ge(0))
		lp := gb.NewLPSolver(m.Compile(), nil)
		rLP := lp.Solve()
		lp.Close()
		if rLP.Status != gb.StatusOptimal {
			t.Fatalf("disconnected LP status=%v, want Optimal", rLP.Status)
		}

		if !testing.Short() {
			rawMAD, err := os.ReadFile(filepath.Join("testdata", "corpus", "t3_mad_g4_i1000.json"))
			if err != nil {
				t.Fatalf("read t3_mad_g4_i1000.json: %v", err)
			}
			docMAD, err := format.ParsePuLPJSON(rawMAD)
			if err != nil {
				t.Fatalf("parse t3_mad_g4_i1000.json: %v", err)
			}
			mMAD, err := format.BuildModelFromPuLPJSON(docMAD)
			if err != nil {
				t.Fatalf("build t3_mad_g4_i1000: %v", err)
			}
			lpMAD := gb.NewLPSolver(mMAD.Compile(), nil)
			resMAD := lpMAD.Solve()
			lpMAD.Close()
			if resMAD.Status != gb.StatusOptimal || math.Abs(resMAD.Objective-208000.0) > 1e-2 {
				t.Fatalf("t3_mad_g4_i1000 LPSolver: status=%v obj=%v, want Optimal 208000",
					resMAD.Status, resMAD.Objective)
			}
		}
	})

	t.Run("m2_objective_constant_overflow", func(t *testing.T) {
		for _, isMIP := range []bool{false, true} {
			m := gb.NewModel("m2_const_overflow", gb.Minimize)
			var x gb.Var
			if isMIP {
				x = m.AddIntVar("x", 1, 1)
			} else {
				x = m.AddContVar("x", 1, 1)
			}
			m.SetObjective(gb.NewExpr(x.T(1e308)).AddConstant(1e308))
			m.AddConstraint("c", gb.VarExpr(x).Ge(1))
			for _, pre := range []bool{false, true} {
				res := mustSolve(t, m, gb.WithPureGo(), gb.WithPresolve(pre))
				if res.Status != gb.StatusNumericalError {
					t.Fatalf("isMIP=%v pre=%v: status=%v obj=%v, want StatusNumericalError",
						isMIP, pre, res.Status, res.Objective)
				}
			}
		}
	})

	t.Run("m1_minor_root_submip_postcut_basis", func(t *testing.T) {
		m := gb.NewModel("root_submip_postcut", gb.Minimize)
		var xs []gb.Var
		obj := gb.NewExpr()
		capExpr := gb.NewExpr()
		eqExpr := gb.NewExpr()
		for i := range 24 {
			v := m.AddBinaryVar(fmt.Sprintf("x_%d", i))
			xs = append(xs, v)
			obj = obj.AddTerm(v, -float64(10+(i*7)%13))
			capExpr = capExpr.AddTerm(v, float64(3+(i*5)%9))
			if i < 6 {
				eqExpr = eqExpr.AddTerm(v, 1.0)
			}
		}
		y := m.AddContVar("y", 0, 10)
		obj = obj.AddTerm(y, 0.5)
		capExpr = capExpr.AddTerm(y, 1.0)
		m.SetObjective(obj)
		m.AddConstraint("cap", capExpr.Le(37))
		m.AddConstraint("eq6", eqExpr.Eq(3))
		_ = xs
		p := m.Compile()
		want := mustSolveCompiled(t, p, gb.WithHeuristics(false)).Objective
		res := mustSolveCompiled(t, p, gb.WithHeuristics(true), gb.WithCuts(true))
		assertOptimal(t, "root_submip_postcut", p, res, want)
	})

	t.Run("c1_cross_bucket_and_lo_preservation_and_rhs_residual", func(t *testing.T) {
		a0 := math.Ldexp(1.0+math.Ldexp(1.0, -52), 500)
		x0Val := math.Ldexp(1.0+math.Ldexp(1.0, -52), 460)
		m := gb.NewModel("cross_bucket_3limb", gb.Maximize)
		x0 := m.AddContVar("x0", x0Val, x0Val)
		x1 := m.AddContVar("x1", 1, 1)
		x2 := m.AddContVar("x2", 1, 1)
		x3 := m.AddContVar("x3", 1, 1)
		x4 := m.AddContVar("x4", 1, 1)
		z := m.AddBinaryVar("z")
		m.SetObjective(gb.VarExpr(z))
		m.AddConstraint("c", gb.NewExpr(
			x0.T(a0),
			x1.T(-math.Ldexp(1, 959)),
			x2.T(-math.Ldexp(1, 959)),
			x3.T(-math.Ldexp(1, 909)),
			x4.T(-math.Ldexp(1, 856)),
			z.T(7.0),
		).Le(0))
		p := m.Compile()
		if _, rViol, _ := p.MaxViolation([]float64{x0Val, 1, 1, 1, 1, 1}, true); math.Abs(rViol-7.0) > 1e-12 {
			t.Fatalf("cross_bucket_3limb MaxViolation = %g, want 7", rViol)
		}
		assertOptimal(t, "cross_bucket_3limb", p, mustSolveCompiled(t, p, gb.WithPureGo()), 0)

		wantLo := math.Ldexp(1, -970)
		mDup := gb.NewModel("dup_lo_survives", gb.Minimize)
		xd := mDup.AddContVar("x", 0, 1)
		mDup.SetObjective(gb.NewExpr(
			xd.T(math.Ldexp(1, 960)),
			xd.T(-math.Ldexp(1, 958)),
			xd.T(-math.Ldexp(1, 958)),
			xd.T(-math.Ldexp(1, 958)),
			xd.T(-math.Ldexp(1, 958)),
			xd.T(wantLo),
		))
		pDup := mDup.Compile()
		if got := pDup.EvaluateObjective([]float64{1}); got != wantLo {
			t.Fatalf("dup_lo_survives EvaluateObjective = %g, want %g", got, wantLo)
		}

		mRHS := gb.NewModel("rhs_cancel_1e17", gb.Maximize)
		xr := mRHS.AddContVar("x0", 1e17, 1e17)
		zr := mRHS.AddBinaryVar("z")
		mRHS.SetObjective(gb.VarExpr(zr))
		mRHS.AddConstraint("c", gb.NewExpr(xr.T(1), zr.T(7)).Le(1e17))
		pRHS := mRHS.Compile()
		if _, rViol, _ := pRHS.MaxViolation([]float64{1e17, 1}, true); math.Abs(rViol-7.0) > 1e-12 {
			t.Fatalf("rhs_cancel_1e17 MaxViolation = %g, want 7", rViol)
		}

		mGCD := gb.NewModel("gcd_3x_2p56", gb.Minimize)
		xg := mGCD.AddIntVar("x", 0, 1e18)
		mGCD.SetObjective(gb.VarExpr(xg))
		mGCD.AddConstraint("eq", gb.NewExpr(xg.T(3)).Eq(math.Ldexp(1, 56)))
		pGCD := mGCD.Compile()
		for _, pre := range []bool{true, false} {
			res := mustSolveCompiled(t, pGCD, gb.WithPureGo(), gb.WithPresolve(pre))
			if res.Status != gb.StatusInfeasible {
				t.Fatalf("gcd_3x_2p56 (pre=%v): status=%v obj=%g, want Infeasible", pre, res.Status, res.Objective)
			}
		}

		if e := gb.Sum(gb.ConstExpr(1e17), gb.ConstExpr(9), gb.ConstExpr(-1e17)); e.Constant != 9.0 {
			t.Fatalf("Sum(1e17, 9, -1e17).Constant = %g, want 9", e.Constant)
		}
		if e := gb.Sum(gb.ConstExpr(1.5e308), gb.ConstExpr(1.5e308), gb.ConstExpr(-1.5e308)); e.Constant != 1.5e308 {
			t.Fatalf("Sum(1.5e308, 1.5e308, -1.5e308).Constant = %g, want 1.5e308", e.Constant)
		}

		c57 := math.Ldexp(1, 57)
		mConstMax := gb.NewModel("obj_const_max_cancel", gb.Maximize)
		cs := mConstMax.AddContVar("s", 1, 1)
		cz := mConstMax.AddBinaryVar("z")
		mConstMax.SetObjective(gb.NewExpr(cs.T(1e17), cz.T(7)).AddConstant(-1e17))
		mConstMax.AddConstraint("c", gb.VarExpr(cz).Ge(1))
		pConstMax := mConstMax.Compile()
		for _, pre := range []bool{true, false} {
			res := mustSolveCompiled(t, pConstMax, gb.WithPureGo(), gb.WithPresolve(pre))
			assertOptimal(t, fmt.Sprintf("obj_const_max_cancel(pre=%v)", pre), pConstMax, res, 7.0)
			if math.Abs(res.BestBound-7.0) > 1e-6 || math.Abs(res.RootLPBound-7.0) > 1e-6 {
				t.Fatalf("obj_const_max_cancel(pre=%v): BestBound=%g RootLPBound=%g, want 7", pre, res.BestBound, res.RootLPBound)
			}
		}
		lpConst := gb.NewLPSolver(pConstMax, nil)
		rLPConst := lpConst.Solve()
		lpConst.Close()
		if rLPConst.Status != gb.StatusOptimal || math.Abs(rLPConst.Objective-7.0) > 1e-9 {
			t.Fatalf("obj_const_max_cancel NewLPSolver: status=%v obj=%g, want Optimal 7", rLPConst.Status, rLPConst.Objective)
		}

		mConstBin := gb.NewModel("obj_const_bin_cancel", gb.Minimize)
		cbx0 := mConstBin.AddContVar("x0", 1, 2)
		cbx1 := mConstBin.AddContVar("x1", 0, 17)
		cbu := mConstBin.AddContVar("u", 0, 3)
		cby := mConstBin.AddBinaryVar("y")
		cbw := mConstBin.AddBinaryVar("w")
		mConstBin.SetObjective(gb.NewExpr(cbx0.T(c57), cbx1.T(1), cbu.T(100), cby.T(-40), cbw.T(-17)).AddConstant(-c57))
		mConstBin.AddConstraint("ub_x0", gb.VarExpr(cbx0).Le(1))
		mConstBin.AddConstraint("eq_x1", gb.NewExpr(cbx1.T(1), cby.T(-17)).Eq(0))
		mConstBin.AddConstraint("cap", gb.NewExpr(cby.T(5), cbw.T(2), cbu.T(1)).Le(5))
		pConstBin := mConstBin.Compile()
		for _, pre := range []bool{true, false} {
			res := mustSolveCompiled(t, pConstBin, gb.WithPureGo(), gb.WithPresolve(pre))
			assertOptimal(t, fmt.Sprintf("obj_const_bin_cancel(pre=%v)", pre), pConstBin, res, -23.0)
		}

		mLPBox := gb.NewModel("lp_box_blocked", gb.Maximize)
		bsp := mLPBox.AddContVar("s_pos", 1e17, 2e17)
		bz := mLPBox.AddContVar("z", 0, 1)
		bsn := mLPBox.AddContVar("s_neg", 0, 1e17)
		mLPBox.SetObjective(gb.VarExpr(bz))
		mLPBox.AddConstraint("le", gb.NewExpr(bsp.T(1), bz.T(7), bsn.T(-1)).Le(0))
		pLPBox := mLPBox.Compile()
		resLPBox := mustSolveCompiled(t, pLPBox, gb.WithPureGo())
		if resLPBox.Status == gb.StatusOptimal {
			assertOptimal(t, "lp_box_blocked", pLPBox, resLPBox, 0.0)
		} else if resLPBox.Status != gb.StatusNumericalError {
			t.Fatalf("lp_box_blocked: status=%v, want Optimal 0 or NumericalError", resLPBox.Status)
		}
	})
}
