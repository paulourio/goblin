package goblin

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/paulourio/goblin/internal/mad"
)

func buildTestPMADModelWithVars(
	name string,
	baselines, items [][]float64,
	step, lambda float64,
	allowZeroStep bool,
	useLeSense bool,
	objConst float64,
) (*Model, [][]Var) {
	numGroups := len(baselines)
	numItems := len(items)
	numPeriods := len(baselines[0])

	m := NewModel(name, Minimize)
	x := make([][]Var, numGroups)
	for g := range numGroups {
		x[g] = make([]Var, numItems)
		for i := range numItems {
			x[g][i] = m.AddBinaryVar("")
		}
	}

	var obj []Term
	for g, lim := range baselines {
		reach := 0.0
		for t, a := range lim {
			s := a
			for _, d := range items {
				if d[t] > 0 {
					s += d[t]
				}
			}
			reach = max(reach, s)
		}
		var steps float64
		if step > 0 {
			steps = max(1.0, math.Ceil(reach/step)+2)
		}
		mUB := math.Inf(1)
		if step > 0 {
			mUB = step * steps
		}
		mg := m.AddContVar("", 0, mUB)
		for t, a := range lim {
			z := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, z.T(1))
			for _, sg := range []float64{1, -1} {
				terms := []Term{z.T(1), mg.T(sg)}
				for i, d := range items {
					if d[t] != 0 {
						terms = append(terms, x[g][i].T(-sg*d[t]))
					}
				}
				if useLeSense {
					neg := make([]Term, len(terms))
					for k, tm := range terms {
						neg[k] = Term{Var: tm.Var, Coeff: -tm.Coeff}
					}
					m.AddConstraint("", NewExpr(neg...).Le(-sg*a))
				} else {
					m.AddConstraint("", NewExpr(terms...).Ge(sg*a))
				}
			}
		}
		if step > 0 && lambda > 0 {
			nMin := 1.0
			if allowZeroStep {
				nMin = 0.0
			}
			n := m.AddIntVar("", nMin, steps)
			r := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, r.T(float64(numPeriods)*lambda))
			m.AddConstraint("", NewExpr(r.T(1), mg.T(-1), n.T(step)).Ge(0))
			m.AddConstraint("", NewExpr(r.T(1), mg.T(1), n.T(-step)).Ge(0))
		}
	}
	for i := range numItems {
		terms := make([]Term, numGroups)
		for g := range numGroups {
			terms[g] = x[g][i].T(1)
		}
		m.AddConstraint("", NewExpr(terms...).Eq(1))
	}
	expr := NewExpr(obj...)
	expr.Constant = objConst
	m.SetObjective(expr)
	return m, x
}

func buildTestPMADModel(
	name string,
	baselines, items [][]float64,
	step, lambda float64,
	allowZeroStep bool,
	useLeSense bool,
	objConst float64,
) *Model {
	m, _ := buildTestPMADModelWithVars(name, baselines, items, step, lambda, allowZeroStep, useLeSense, objConst)
	return m
}

func buildTestDirectStepMADModel(baselines, items [][]float64, step float64, allowZeroStep bool) *Model {
	numGroups := len(baselines)
	numItems := len(items)
	numPeriods := len(baselines[0])

	m := NewModel("direct_step_mad", Minimize)
	x := make([][]Var, numGroups)
	n := make([]Var, numGroups)
	nMin := 1.0
	if allowZeroStep {
		nMin = 0.0
	}
	var obj []Term
	for g := range numGroups {
		x[g] = make([]Var, numItems)
		for i := range numItems {
			x[g][i] = m.AddBinaryVar("")
		}
		n[g] = m.AddIntVar("", nMin, math.Inf(1))
		for t := range numPeriods {
			z := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, z.T(1))
			a := baselines[g][t]
			for _, sg := range []float64{1, -1} {
				terms := []Term{z.T(1), n[g].T(sg * step)}
				for i := range numItems {
					if d := items[i][t]; d != 0 {
						terms = append(terms, x[g][i].T(-sg*d))
					}
				}
				m.AddConstraint("", NewExpr(terms...).Ge(sg*a))
			}
		}
	}
	for i := range numItems {
		terms := make([]Term, numGroups)
		for g := range numGroups {
			terms[g] = x[g][i].T(1)
		}
		m.AddConstraint("", NewExpr(terms...).Eq(1))
	}
	m.SetObjective(NewExpr(obj...))
	return m
}

func buildTestDesignatedMADModel(items [][]float64, numGroups int, step, roundWeight float64, designated []int) *Model {
	numItems := len(items)
	numPeriods := len(items[0])
	m := NewModel("designated_mad", Minimize)
	x := make([][]Var, numGroups)
	var obj []Term
	for g := range numGroups {
		x[g] = make([]Var, numItems)
		for i := range numItems {
			x[g][i] = m.AddBinaryVar("")
		}
		mg := m.AddContVar("", 0, math.Inf(1))
		for t := range numPeriods {
			z := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, z.T(1))
			pos := []Term{z.T(1), mg.T(1)}
			neg := []Term{z.T(1), mg.T(-1)}
			for i := range numItems {
				if d := items[i][t]; d != 0 {
					pos = append(pos, x[g][i].T(-d))
					neg = append(neg, x[g][i].T(d))
				}
			}
			m.AddConstraint("", NewExpr(pos...).Ge(0))
			m.AddConstraint("", NewExpr(neg...).Ge(0))
		}
		if step > 0 && roundWeight > 0 {
			ng := m.AddIntVar("", 1, math.Inf(1))
			rg := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, rg.T(roundWeight))
			m.AddConstraint("", NewExpr(rg.T(1), mg.T(-1), ng.T(step)).Ge(0))
			m.AddConstraint("", NewExpr(rg.T(1), mg.T(1), ng.T(-step)).Ge(0))
		}
	}
	for i := range numItems {
		terms := make([]Term, numGroups)
		for g := range numGroups {
			terms[g] = x[g][i].T(1)
		}
		m.AddConstraint("", NewExpr(terms...).Eq(1))
	}
	for g := range numGroups {
		terms := make([]Term, len(designated))
		for k, it := range designated {
			terms[k] = x[g][it].T(1)
		}
		m.AddConstraint("", NewExpr(terms...).Eq(1))
	}
	m.SetObjective(NewExpr(obj...))
	return m
}

func pmadBaseData() (baselines, items [][]float64) {
	baselines = [][]float64{
		{508, 465, 706, 1213, 704, 238},
		{1360, 379, 1778, 763, 386, 243},
		{1428, 1832, 1301, 1388, 1021, 832},
		{542, 1930, 1721, 1019, 240, 838},
		{1585, 111, 1967, 886, 880, 781},
		{1216, 494, 1417, 1804, 300, 1037},
		{1370, 1761, 314, 637, 457, 244},
		{1614, 1452, 1244, 736, 1645, 1701},
		{396, 1810, 656, 659, 1201, 1528},
		{1009, 210, 447, 1748, 1746, 655},
	}
	items = [][]float64{
		{149, 566, 1479, 128, 1950, 1713},
		{491, 1179, 1737, 1103, 990, 1847},
	}
	return baselines, items
}

func TestMADDetectPMAD24137AndAlgebraicVariants(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	baselines, items := pmadBaseData()

	t.Run("DefaultModelSolveExact24137", func(t *testing.T) {
		t.Parallel()
		m := buildTestPMADModel("pmad_base", baselines, items, 100, 1.0, false, false, 0)
		p := m.Compile()
		res, err := SolveCompiled(ctx, p)
		if err != nil {
			t.Fatalf("SolveCompiled failed: %v", err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("status=%v, want Optimal", res.Status)
		}
		if res.Nodes != 0 || res.Iterations != 0 {
			t.Fatalf("expected combinatorial dispatch (Nodes=0, Iters=0), got Nodes=%d Iters=%d", res.Nodes, res.Iterations)
		}
		if math.Abs(res.Objective-24137) > 1e-9 || math.Abs(res.BestBound-24137) > 1e-9 {
			t.Fatalf("obj=%.12g bound=%.12g, want 24137", res.Objective, res.BestBound)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
			t.Fatalf("primal violation bV=%.2e rV=%.2e iV=%.2e", bV, rV, iV)
		}
	})

	t.Run("LeEpigraphAndObjConstant", func(t *testing.T) {
		t.Parallel()
		const offset = 12.5
		m := buildTestPMADModel("pmad_le_offset", baselines, items, 100, 1.0, false, true, offset)
		p := m.Compile()
		res, err := SolveCompiled(ctx, p)
		if err != nil {
			t.Fatalf("SolveCompiled failed: %v", err)
		}
		if res.Status != StatusOptimal || res.Nodes != 0 {
			t.Fatalf("status=%v nodes=%d, want Optimal/0", res.Status, res.Nodes)
		}
		want := 24137.0 + offset
		if math.Abs(res.Objective-want) > 1e-9 || math.Abs(res.BestBound-want) > 1e-9 {
			t.Fatalf("obj=%.12g bound=%.12g, want %.12g", res.Objective, res.BestBound, want)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
			t.Fatalf("primal violation bV=%.2e rV=%.2e iV=%.2e", bV, rV, iV)
		}
	})

	t.Run("CertifiedWitnessWithoutVIPR", func(t *testing.T) {
		t.Parallel()
		m := buildTestPMADModel("pmad_cert", baselines, items, 100, 1.0, false, false, 7.25)
		p := m.Compile()
		var cw CertifiedWitness
		res, err := SolveCompiled(ctx, p, WithCertifiedWitness(&cw))
		if err != nil {
			t.Fatalf("SolveCompiled(WithCertifiedWitness) failed: %v", err)
		}
		if res.Status != StatusOptimal || res.Nodes != 0 {
			t.Fatalf("status=%v nodes=%d, want Optimal/0", res.Status, res.Nodes)
		}
		wantRat := big.NewRat(24137*4+29, 4)
		if cw.Status != StatusOptimal || cw.Objective == nil || cw.Objective.Cmp(wantRat) != 0 {
			t.Fatalf("cw=(status=%v obj=%v), want Optimal %s", cw.Status, cw.Objective, wantRat.RatString())
		}
		if cw.BestBound == nil || cw.BestBound.Cmp(wantRat) != 0 {
			t.Fatalf("cw.BestBound=%v, want %s", cw.BestBound, wantRat.RatString())
		}
		if !verifyExactPrimalRat(p, cw.Values, true) {
			t.Fatal("exact rational witness failed verifyExactPrimalRat")
		}
	})

	t.Run("WithCombinatorialSearchFalseUsesBranchAndCut", func(t *testing.T) {
		t.Parallel()
		m := buildTestPMADModel("pmad_bnb", baselines, items, 100, 1.0, false, false, 0)
		p := m.Compile()
		res, err := SolveCompiled(ctx, p, WithCombinatorialSearch(false), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil {
			t.Fatalf("SolveCompiled(WithCombinatorialSearch(false)) failed: %v", err)
		}
		if res.Status != StatusOptimal || res.Nodes <= 1 {
			t.Fatalf("expected B&B exploration (Nodes > 1), got status=%v nodes=%d", res.Status, res.Nodes)
		}
		if math.Abs(res.Objective-24137) > 1e-6 {
			t.Fatalf("obj=%.12g, want 24137", res.Objective)
		}
	})

	t.Run("ExplicitVIPRSkipsCombinatorial", func(t *testing.T) {
		t.Parallel()
		smallBase := [][]float64{{110, 290, 180}, {410, 590, 490}}
		smallItems := [][]float64{{45, 130, 80}, {120, 60, 190}}
		m := buildTestPMADModel("pmad_vipr", smallBase, smallItems, 100, 0.5, false, false, 0)
		p := m.Compile()
		tr := NewVIPRTrace()
		res, err := SolveCompiled(ctx, p, WithCertified(true), WithVIPRTrace(tr))
		if err != nil {
			t.Fatalf("SolveCompiled(WithVIPRTrace) failed: %v", err)
		}
		if res.Status != StatusOptimal || !tr.HasCertificate() {
			t.Fatalf("status=%v hasCert=%v, want Optimal/true", res.Status, tr.HasCertificate())
		}
		combRes, err := SolveCompiled(ctx, p)
		if err != nil || math.Abs(combRes.Objective-res.Objective) > 1e-6 {
			t.Fatalf("combinatorial obj=%.12g vs VIPR obj=%.12g (err=%v)", combRes.Objective, res.Objective, err)
		}
	})

	t.Run("DirectStepUnroundedAndDesignatedParity", func(t *testing.T) {
		t.Parallel()
		smallBase := [][]float64{{110, 290, 180}, {410, 590, 490}}
		smallItems := [][]float64{{45, 130, 80}, {120, 60, 190}, {30, 90, 40}}

		models := []*Model{
			buildTestDirectStepMADModel(smallBase, smallItems, 100, false),
			buildTestDirectStepMADModel(smallBase, smallItems, 100, true),
			buildTestPMADModel("unrounded", smallBase, smallItems, 0, 0, false, false, 0),
			buildTestDesignatedMADModel(
				[][]float64{
					{8313, 4344, 3716, 13203, 7885},
					{0, 50, 70, 70, 700},
					{5258, 8917, 9308, 3331, 8347},
					{10400, 10350, 10330, 10330, 9700},
				},
				2, 100, 1.0, []int{2, 3},
			),
		}
		for _, m := range models {
			p := m.Compile()
			combRes, err := SolveCompiled(ctx, p)
			if err != nil || combRes.Status != StatusOptimal || combRes.Nodes != 0 {
				t.Fatalf("[%s] combinatorial solve=(%v, nodes=%d, err=%v)", m.name, combRes.Status, combRes.Nodes, err)
			}
			milpRes, err := SolveCompiled(ctx, p, WithCombinatorialSearch(false), WithRelGapTol(0), WithAbsGapTol(0))
			if err != nil || milpRes.Status != StatusOptimal {
				t.Fatalf("[%s] MILP solve=(%v, err=%v)", m.name, milpRes.Status, err)
			}
			if math.Abs(combRes.Objective-milpRes.Objective) > 1e-6*max(1.0, math.Abs(milpRes.Objective)) {
				t.Fatalf("[%s] comb=%.12g != milp=%.12g", m.name, combRes.Objective, milpRes.Objective)
			}
		}
	})
}

func TestMADDetectClosureNegativeControls(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	smallBase := [][]float64{{110, 290, 180}, {410, 590, 490}}
	smallItems := [][]float64{{45, 130, 80}, {120, 60, 190}}

	t.Run("ExtraSideConstraintRejects", func(t *testing.T) {
		t.Parallel()
		m, x := buildTestPMADModelWithVars("extra_row", smallBase, smallItems, 100, 1.0, false, false, 0)
		// Force item 0 into group 0 (extra side constraint).
		m.AddConstraint("force_x00", NewExpr(x[0][0].T(1)).Eq(1))
		p := m.Compile()
		if _, ok := detectMADStructure(p); ok {
			t.Fatal("detectMADStructure accepted model with extra side constraint")
		}
		res, err := SolveCompiled(ctx, p)
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("fallback B&B failed: status=%v err=%v", res.Status, err)
		}
		if res.Values[0] < 0.5 {
			t.Fatalf("extra side constraint violated: x[0]=%v", res.Values[0])
		}
	})

	t.Run("BindingFiniteUpperBoundFallsBackSafely", func(t *testing.T) {
		t.Parallel()
		m := NewModel("binding_m_ub", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		mg0 := m.AddContVar("m0", 0, 50) // Binding upper bound!
		mg1 := m.AddContVar("m1", 0, 50)
		z0 := m.AddContVar("z0", 0, math.Inf(1))
		z1 := m.AddContVar("z1", 0, math.Inf(1))
		m.AddConstraint("d0p", NewExpr(z0.T(1), mg0.T(1), x0.T(-200)).Ge(100))
		m.AddConstraint("d0n", NewExpr(z0.T(1), mg0.T(-1), x0.T(200)).Ge(-100))
		m.AddConstraint("d1p", NewExpr(z1.T(1), mg1.T(1), x1.T(-200)).Ge(100))
		m.AddConstraint("d1n", NewExpr(z1.T(1), mg1.T(-1), x1.T(200)).Ge(-100))
		m.AddConstraint("part", NewExpr(x0.T(1), x1.T(1)).Eq(1))
		m.SetObjective(NewExpr(z0.T(1), z1.T(1)))

		p := m.Compile()
		resComb, err := SolveCompiled(ctx, p)
		if err != nil || resComb.Status != StatusOptimal {
			t.Fatalf("default solve failed: status=%v err=%v", resComb.Status, err)
		}
		resBnB, err := SolveCompiled(ctx, p, WithCombinatorialSearch(false), WithRelGapTol(0), WithAbsGapTol(0))
		if err != nil || resBnB.Status != StatusOptimal {
			t.Fatalf("B&B solve failed: status=%v err=%v", resBnB.Status, err)
		}
		if math.Abs(resComb.Objective-resBnB.Objective) > 1e-6 {
			t.Fatalf("binding m UB mismatch: default=%.12g vs B&B=%.12g", resComb.Objective, resBnB.Objective)
		}
		bV, rV, iV := p.MaxViolation(resComb.Values, true)
		if bV > 1e-9 || rV > 1e-9 || iV > 1e-9 {
			t.Fatalf("binding UB primal violation bV=%.2e rV=%.2e iV=%.2e", bV, rV, iV)
		}
	})

	t.Run("AsymmetricGroupItemWeightsReject", func(t *testing.T) {
		t.Parallel()
		m := NewModel("asym_weights", Minimize)
		x0 := m.AddBinaryVar("x0")
		x1 := m.AddBinaryVar("x1")
		m0 := m.AddContVar("m0", 0, math.Inf(1))
		m1 := m.AddContVar("m1", 0, math.Inf(1))
		z0 := m.AddContVar("z0", 0, math.Inf(1))
		z1 := m.AddContVar("z1", 0, math.Inf(1))
		m.AddConstraint("d0p", NewExpr(z0.T(1), m0.T(1), x0.T(-10)).Ge(5))
		m.AddConstraint("d0n", NewExpr(z0.T(1), m0.T(-1), x0.T(10)).Ge(-5))
		m.AddConstraint("d1p", NewExpr(z1.T(1), m1.T(1), x1.T(-20)).Ge(5))
		m.AddConstraint("d1n", NewExpr(z1.T(1), m1.T(-1), x1.T(20)).Ge(-5))
		m.AddConstraint("part", NewExpr(x0.T(1), x1.T(1)).Eq(1))
		m.SetObjective(NewExpr(z0.T(1), z1.T(1)))

		p := m.Compile()
		if _, ok := detectMADStructure(p); ok {
			t.Fatal("detectMADStructure accepted asymmetric group item weights")
		}
	})
}

func TestMADDetectFastScreenZeroAllocs(t *testing.T) {
	mLP := NewModel("lp", Minimize)
	x := mLP.AddContVar("x", 0, 10)
	mLP.AddConstraint("c", NewExpr(x.T(1)).Ge(2))
	mLP.SetObjective(NewExpr(x.T(3)))
	pLP := mLP.Compile()

	mKP := NewModel("knapsack", Minimize)
	b0 := mKP.AddBinaryVar("b0")
	b1 := mKP.AddBinaryVar("b1")
	mKP.AddConstraint("cap", NewExpr(b0.T(2), b1.T(3)).Ge(3))
	mKP.SetObjective(NewExpr(b0.T(4), b1.T(5)))
	pKP := mKP.Compile()

	if allocs := testing.AllocsPerRun(50, func() {
		if _, ok := detectMADStructure(pLP); ok {
			t.Fatal("LP matched MAD")
		}
		if _, ok := detectMADStructure(pKP); ok {
			t.Fatal("knapsack matched MAD")
		}
	}); allocs != 0 {
		t.Fatalf("detectMADStructure on non-MAD models allocated %v times, want 0", allocs)
	}
}

func TestMADDetectFallbackBeyondMaxDPItems(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// I = MaxDPItems + 1 (15) with step-aligned zero-cost solution so B&B solves cleanly after fallback.
	numItems := mad.MaxDPItems + 1
	items := make([][]float64, numItems)
	for i := range numItems {
		items[i] = []float64{100, 100}
	}
	baselines := [][]float64{{0, 0}, {0, 0}}
	m := buildTestPMADModel("mad_over_dp_cap", baselines, items, 100, 1.0, true, false, 0)
	p := m.Compile()

	if _, ok := detectMADStructure(p); !ok {
		t.Fatal("expected detectMADStructure to recognize 15-item MAD problem before delegating fallback")
	}
	res, err := SolveCompiled(ctx, p, WithTimeLimit(5*time.Second))
	if err != nil {
		t.Fatalf("SolveCompiled beyond MaxDPItems failed: %v", err)
	}
	if res.Status != StatusOptimal {
		t.Fatalf("status=%v, want Optimal", res.Status)
	}
	if math.Abs(res.Objective) > 1e-6 {
		t.Fatalf("obj=%.12g, want 0", res.Objective)
	}
}
