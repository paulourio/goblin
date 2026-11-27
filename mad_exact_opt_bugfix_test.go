package goblin_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

func verifyRootLPBothModes(t *testing.T, tag string, p *gb.CompiledProblem, wantObj float64, checkObj bool) {
	t.Helper()
	for _, disableSIMD := range []bool{false, true} {
		mode := "simd"
		if disableSIMD {
			mode = "scalar"
		}
		lp := gb.NewLPSolver(p, nil)
		lp.DisableSIMD = disableSIMD
		res := lp.Solve()
		lp.Close()
		if res.Status != gb.StatusOptimal {
			t.Fatalf("[%s/%s] root LP status=%v (iters=%d), want Optimal", tag, mode, res.Status, res.Iterations)
		}
		bV, rV, _ := p.MaxViolation(res.Primal, false)
		if bV > 1e-6 || rV > 1e-6 {
			t.Fatalf("[%s/%s] root LP primal violation b=%.3e r=%.3e", tag, mode, bV, rV)
		}
		if checkObj {
			tol := 1e-6 * max(1.0, math.Abs(wantObj))
			if math.Abs(res.Objective-wantObj) > tol {
				t.Fatalf("[%s/%s] root LP obj=%.10g != want=%.10g", tag, mode, res.Objective, wantObj)
			}
		}
	}
}

func TestExactOptMADRegression96(t *testing.T) {
	t.Parallel()
	seeds := []struct {
		id       string
		seed     uint64
		gByte    uint8
		sizeRaw  uint16
		modeSp   uint8
		baseByte uint8
	}{
		{"81616f8d12486485", 41, 'u', 155, 'C', '\x11'},
		{"ac2d077649fc0689", 41, '-', 155, 'C', '\x11'},
		{"c27a7ead70e6a2b2", 41, 'u', 134, 'C', '\x11'},
		{"8bedc459f655bd57", 21, '\x05', 35453, '1', '&'},
	}

	for _, tc := range seeds {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec := decodeExactOptMADSpec(tc.seed, tc.gByte, tc.sizeRaw, tc.modeSp, tc.baseByte)
			tag := "reg96_" + tc.id
			inst := buildExactOptimumMAD(spec, tag)
			p, err := inst.model.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
			}

			verifyRootLPBothModes(t, tag, p, inst.exactLP, true)

			timeLimit := 3 * time.Second
			const maxNodes = 1500
			resGo := mustSolveCompiled(t, p,
				gb.WithPureGo(),
				gb.WithPresolve(true),
				gb.WithCuts(true),
				gb.WithHeuristics(true),
				gb.WithTimeLimit(timeLimit),
				gb.WithMaxNodes(maxNodes),
			)
			hasIncGo := assertExactOptMADSoundness(t, tag, "go", p, resGo, inst.exactLP, inst.exactMILP, false)

			resNoPre := mustSolveCompiled(t, p,
				gb.WithPureGo(),
				gb.WithPresolve(false),
				gb.WithTimeLimit(timeLimit),
				gb.WithMaxNodes(maxNodes),
			)
			hasIncNoPre := assertExactOptMADSoundness(t, tag, "go-nopresolve", p, resNoPre, inst.exactLP, inst.exactMILP, true)
			assertTwoGoSolvesConsistent(t, tag, "go", resGo, hasIncGo, "go-nopresolve", resNoPre, hasIncNoPre)

			if formalFuzzEnabled() && p.NumVars() <= 600 {
				verifyFormalRootLP(t, tag+"/rootLP", p)
			}
		})
	}
}

func TestAdvMADRegression103(t *testing.T) {
	t.Parallel()
	seeds := []struct {
		id         string
		seed       uint64
		gByte      uint8
		sizeRaw    uint16
		regimeByte uint8
	}{
		{"01914124208b69ad", 183, 'K', 2, ','},
		{"7ba497c636d5d803", 193, 'K', 29, '\x04'},
	}

	for _, tc := range seeds {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec := decodeAdvMADSpec(tc.seed, tc.gByte, tc.sizeRaw, tc.regimeByte)
			tag := "reg103_" + tc.id
			inst := buildAdversarialMAD(spec, tag)
			p, err := inst.model.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
			}

			verifyRootLPBothModes(t, tag, p, 0, false)

			timeLimit := 3 * time.Second
			const maxNodes = 1500
			resGo := mustSolveCompiled(t, p,
				gb.WithPureGo(),
				gb.WithPresolve(true),
				gb.WithCuts(true),
				gb.WithHeuristics(true),
				gb.WithTimeLimit(timeLimit),
				gb.WithMaxNodes(maxNodes),
			)
			hasIncGo := assertDecomposedMADSoundness(t, tag, "go", p, resGo, inst.plantedObj)

			resNoPre := mustSolveCompiled(t, p,
				gb.WithPureGo(),
				gb.WithPresolve(false),
				gb.WithTimeLimit(timeLimit),
				gb.WithMaxNodes(maxNodes),
			)
			hasIncNoPre := assertDecomposedMADSoundness(t, tag, "go-nopresolve", p, resNoPre, inst.plantedObj)
			assertTwoGoSolvesConsistent(t, tag, "go", resGo, hasIncGo, "go-nopresolve", resNoPre, hasIncNoPre)

			if formalFuzzEnabled() && p.NumVars() <= 600 {
				verifyFormalRootLP(t, tag+"/rootLP", p)
				if p.NumVars() <= 240 {
					resTr, tr := verifyFormalVIPRTrace(
						t,
						tag+"/vipr",
						p,
						40,
						2*time.Second,
						gb.WithPresolve(false),
						gb.WithCuts(true),
						gb.WithHeuristics(true),
					)
					_ = assertDecomposedMADSoundness(t, tag, "vipr", p, resTr, inst.plantedObj)
					assertFullFeatureVsVIPR(t, tag+"/full_vs_vipr", p, resGo, resTr, tr, 1e-6, 1e-6)
				}
			}
		})
	}
}

func TestDualResidualConnectedComponentRegression(t *testing.T) {
	t.Parallel()

	t.Run("zero_dual_coupled_component", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("zero_dual_coupled", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		z0 := m.AddContVar("z0", 0, math.Inf(1))
		z1 := m.AddContVar("z1", 0, math.Inf(1))
		y0 := m.AddContVar("y0", 0, math.Inf(1))
		s0 := m.AddContVar("s0", 0, 10)

		m.SetObjective(gb.NewExpr(
			gb.T(z0, 1.0),
			gb.T(z1, 1.0),
			gb.T(s0, 1e-10),
		))
		m.AddConstraint("c_pos", gb.Ge(gb.NewExpr(gb.T(z0, 1), gb.T(y0, 1), gb.T(x0, -200), gb.T(x1, -50)), 0))
		m.AddConstraint("c_neg", gb.Ge(gb.NewExpr(gb.T(z0, 1), gb.T(y0, -1), gb.T(x0, 200), gb.T(x1, 50)), 0))
		m.AddConstraint("c_bal", gb.Ge(gb.NewExpr(gb.T(z1, 1), gb.T(x0, 1), gb.T(x1, 1)), 4))
		m.AddConstraint("c_eq", gb.Eq(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)), 3))
		m.AddConstraint("c_tiny", gb.Ge(gb.NewExpr(gb.T(s0, 1)), 2))

		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked failed: %v", err)
		}
		verifyRootLPBothModes(t, "zero_dual_coupled", p, 1.0+2e-10, true)
	})

	t.Run("multi_scale_coupled_rows", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("multi_scale_coupled", gb.Minimize)
		x1 := m.AddContVar("x1", 0, math.Inf(1))
		x2 := m.AddContVar("x2", 0, math.Inf(1))
		m.SetObjective(gb.NewExpr(gb.T(x1, 1.0), gb.T(x2, 1e-10)))
		m.AddConstraint("r1", gb.Eq(gb.NewExpr(gb.T(x1, 1e-10)), 1.0))
		m.AddConstraint("r2", gb.Eq(gb.NewExpr(gb.T(x1, 1e-10), gb.T(x2, 1e10)), 1e10))

		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked failed: %v", err)
		}
		wantObj := 1e10 + 1e-10*(1e10-1.0)/1e10
		verifyRootLPBothModes(t, "multi_scale_coupled", p, wantObj, true)
	})

	t.Run("unbounded_coupled_negative_control", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("unbounded_coupled", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		z0 := m.AddContVar("z0", 0, math.Inf(1))
		y0 := m.AddContVar("y0", 0, math.Inf(1))
		u0 := m.AddContVar("u0", 0, math.Inf(1))

		m.SetObjective(gb.NewExpr(
			gb.T(z0, 1.0),
			gb.T(u0, -1e-9),
		))
		m.AddConstraint("c_pos", gb.Ge(gb.NewExpr(gb.T(z0, 1), gb.T(y0, 1), gb.T(x0, -200), gb.T(x1, -50)), 0))
		m.AddConstraint("c_neg", gb.Ge(gb.NewExpr(gb.T(z0, 1), gb.T(y0, -1), gb.T(x0, 200), gb.T(x1, 50)), 0))
		m.AddConstraint("c_eq", gb.Eq(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)), 3))
		m.AddConstraint("c_ray", gb.Ge(gb.NewExpr(gb.T(u0, 1), gb.T(x0, 1)), 1))

		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked failed: %v", err)
		}
		for _, disableSIMD := range []bool{false, true} {
			lp := gb.NewLPSolver(p, nil)
			lp.DisableSIMD = disableSIMD
			res := lp.Solve()
			lp.Close()
			if res.Status != gb.StatusUnbounded {
				t.Fatalf("unbounded coupled (disableSIMD=%v): got status=%v, want Unbounded", disableSIMD, res.Status)
			}
		}
	})
}

// TestStruct62c646Phase1Regression covers seed 62c646cb3f1af6db from FuzzStructLPAndMILP,
// where phase-1 simplex must accept sub-1e-7 reduced costs and sub-pivotTol exact FTRAN blockers.
func TestStruct62c646Phase1Regression(t *testing.T) {
	t.Parallel()

	m := gb.NewModel("struct_62c646cb3f1af6db", gb.Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddContVar("x1", math.Inf(-1), math.Inf(1))
	x2 := m.AddContVar("x2", 0, math.Inf(1))
	x3 := m.AddContVar("x3", 0.5, 4.5)
	x4 := m.AddContVar("x4", 0.5, 4.5)
	x5 := m.AddBinaryVar("x5")

	m.SetObjective(gb.Expr{
		Terms: []gb.Term{
			gb.T(x0, -1), gb.T(x1, -1), gb.T(x2, -1),
			gb.T(x3, -1), gb.T(x4, -1), gb.T(x5, -1),
		},
		Constant: -0.1,
	})
	m.AddConstraint("r0", gb.Le(gb.NewExpr(gb.T(x1, 0.5), gb.T(x2, 1e6)), 1e6))
	m.AddConstraint("r1", gb.Le(gb.NewExpr(gb.T(x3, 1e6), gb.T(x0, 2), gb.T(x2, -1)), -1))
	m.AddConstraint("r2", gb.Le(gb.NewExpr(gb.T(x1, 2), gb.T(x0, 1)), 0))
	for _, name := range []string{"r3", "r4", "r5", "r6", "r7", "r8"} {
		m.AddConstraint(name, gb.Le(gb.NewExpr(), 0))
	}

	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}

	const wantObj = 1e12 - 500007.1
	const tag = "struct_62c646cb3f1af6db"
	verifyRootLPBothModes(t, tag, p, wantObj, true)

	tol := 1e-6 * wantObj
	for _, tc := range []struct {
		label string
		opts  []gb.Option
	}{
		{
			label: "go",
			opts: []gb.Option{
				gb.WithPureGo(),
				gb.WithPresolve(true),
				gb.WithCuts(true),
				gb.WithHeuristics(true),
				gb.WithTimeLimit(3 * time.Second),
				gb.WithMaxNodes(1500),
			},
		},
		{
			label: "go-nopresolve",
			opts: []gb.Option{
				gb.WithPureGo(),
				gb.WithPresolve(false),
				gb.WithTimeLimit(3 * time.Second),
				gb.WithMaxNodes(1500),
			},
		},
	} {
		res := mustSolveCompiled(t, p, tc.opts...)
		if res.Status != gb.StatusOptimal {
			t.Fatalf("[%s/%s] status=%v (nodes=%d iters=%d), want Optimal", tag, tc.label, res.Status, res.Nodes, res.Iterations)
		}
		if math.Abs(res.Objective-wantObj) > tol {
			t.Fatalf("[%s/%s] obj=%.10g != want=%.10g", tag, tc.label, res.Objective, wantObj)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
			t.Fatalf("[%s/%s] violation b=%.3e r=%.3e i=%.3e", tag, tc.label, bV, rV, iV)
		}
	}

	if formalFuzzEnabled() {
		verifyFormalRootLP(t, tag+"/rootLP", p)
		resTr, _ := verifyFormalVIPRTrace(
			t,
			tag+"/vipr",
			p,
			40,
			2*time.Second,
			gb.WithPresolve(false),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
		)
		if resTr.Status != gb.StatusOptimal {
			t.Fatalf("[%s/vipr] status=%v, want Optimal", tag, resTr.Status)
		}
	}
}

type pmadSpec struct {
	items      [][]float64
	baselines  [][]float64
	b          float64
	lambda     float64
	absTol     float64
	relTol     float64
	maxNodes   int
	workersOpt bool
}

func basePMADSpec() pmadSpec {
	return pmadSpec{
		items: [][]float64{
			{149, 566, 1479, 128, 1950, 1713},
			{491, 1179, 1737, 1103, 990, 1847},
		},
		baselines: [][]float64{
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
		},
		b:          100,
		lambda:     1,
		absTol:     1,
		relTol:     1e-3,
		maxNodes:   50000,
		workersOpt: true,
	}
}

func perturbPMADSpec(base pmadSpec, seed uint64) pmadSpec {
	out := base
	out.items = make([][]float64, len(base.items))
	for i, row := range base.items {
		out.items[i] = slices.Clone(row)
	}
	out.baselines = make([][]float64, len(base.baselines))
	for k, row := range base.baselines {
		out.baselines[k] = slices.Clone(row)
	}

	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	for _, row := range out.items {
		for tt, v := range row {
			row[tt] = max(0.0, v+float64(rng.IntN(11)-5))
		}
	}
	for _, row := range out.baselines {
		for tt, v := range row {
			row[tt] = max(0.0, v+float64(rng.IntN(11)-5))
		}
	}
	return out
}

func buildPMADModel(name string, in pmadSpec) *gb.Model {
	m := gb.NewModel(name, gb.Minimize)
	if len(in.baselines) == 0 || len(in.baselines[0]) == 0 {
		return m
	}

	tPeriods := len(in.baselines[0])
	x := make([][]gb.Var, len(in.baselines))
	for k := range x {
		for range in.items {
			x[k] = append(x[k], m.AddBinaryVar(""))
		}
	}

	var obj []gb.Term
	for k, base := range in.baselines {
		reach := 0.0
		for tt, a := range base {
			s := a
			for _, d := range in.items {
				s += d[tt]
			}
			reach = max(reach, s)
		}
		steps := max(1.0, math.Ceil(reach/in.b))
		mk := m.AddContVar("", 0, in.b*steps)
		for tt, a := range base {
			z := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, z.T(1))
			for _, sg := range []float64{1, -1} {
				terms := []gb.Term{z.T(1), mk.T(sg)}
				for i, d := range in.items {
					if d[tt] != 0 {
						terms = append(terms, x[k][i].T(-sg*d[tt]))
					}
				}
				m.AddConstraint("", gb.NewExpr(terms...).Ge(sg*a))
			}
		}
		if in.lambda > 0 {
			n := m.AddIntVar("", 1, steps)
			r := m.AddContVar("", 0, math.Inf(1))
			obj = append(obj, r.T(float64(tPeriods)*in.lambda))
			m.AddConstraint("", gb.NewExpr(r.T(1), mk.T(-1), n.T(in.b)).Ge(0))
			m.AddConstraint("", gb.NewExpr(r.T(1), mk.T(1), n.T(-in.b)).Ge(0))
		}
	}

	for i := range in.items {
		var terms []gb.Term
		for k := range in.baselines {
			terms = append(terms, x[k][i].T(1))
		}
		m.AddConstraint("", gb.NewExpr(terms...).Eq(1))
	}
	m.SetObjective(gb.NewExpr(obj...))
	return m
}

// exactPMADGroupCost computes the exact minimum group objective over continuous
// m in [0, b*steps] and integer roundingStep n in [1, max(1, floor(steps))].
func exactPMADGroupCost(y []float64, b, lambda, steps float64) (bestCost, bestM, bestN float64) {
	mMax := b * steps
	if len(y) == 0 {
		if lambda <= 0 {
			return 0, 0, 0
		}
		return 0, min(b, mMax), 1
	}

	evalM := func(m float64) float64 {
		cost := 0.0
		for _, v := range y {
			cost += math.Abs(v - m)
		}
		return cost
	}

	if lambda <= 0 {
		bestCost = evalM(0)
		bestM = 0
		if c := evalM(mMax); c < bestCost {
			bestCost = c
			bestM = mMax
		}
		for _, v := range y {
			if v < 0 || v > mMax {
				continue
			}
			if c := evalM(v); c < bestCost {
				bestCost = c
				bestM = v
			}
		}
		return bestCost, bestM, 0
	}

	w := float64(len(y)) * lambda
	maxN := max(1.0, math.Floor(steps))
	clampN := func(n float64) float64 {
		return min(maxN, max(1.0, n))
	}
	bestCost = math.Inf(1)
	consider := func(m, n float64) {
		if m < 0 || m > mMax {
			return
		}
		c := evalM(m) + w*math.Abs(m-b*n)
		if c < bestCost {
			bestCost = c
			bestM = m
			bestN = n
		}
	}
	considerM := func(m float64) {
		if m < 0 || m > mMax {
			return
		}
		q := m / b
		nLo := clampN(math.Floor(q))
		nHi := clampN(math.Ceil(q))
		consider(m, nLo)
		if nHi != nLo {
			consider(m, nHi)
		}
	}

	considerM(0)
	considerM(mMax)
	consider(min(b, mMax), 1)
	if maxN > 1 {
		consider(min(b*maxN, mMax), maxN)
	}
	for _, v := range y {
		considerM(v)
		q := v / b
		nLo := clampN(math.Floor(q))
		nHi := clampN(math.Ceil(q))
		consider(min(b*nLo, mMax), nLo)
		if nHi != nLo {
			consider(min(b*nHi, mMax), nHi)
		}
	}
	return bestCost, bestM, bestN
}

func validPMADMatrix(rows [][]float64, tPeriods int) bool {
	for _, row := range rows {
		if len(row) != tPeriods {
			return false
		}
		for _, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		}
	}
	return true
}

func exactPMADObjective(items, baselines [][]float64, b, lambda float64, maxComb int) (opt float64, ok bool) {
	if maxComb <= 0 || len(baselines) == 0 || len(baselines[0]) == 0 || len(items) > 12 {
		return 0, false
	}
	if b <= 0 || math.IsNaN(b) || math.IsInf(b, 0) || lambda < 0 || math.IsNaN(lambda) || math.IsInf(lambda, 0) {
		return 0, false
	}
	tPeriods := len(baselines[0])
	if !validPMADMatrix(baselines, tPeriods) || !validPMADMatrix(items, tPeriods) {
		return 0, false
	}

	nGroups := len(baselines)
	nItems := len(items)
	comb := 1
	for range nItems {
		if comb > maxComb/nGroups {
			return 0, false
		}
		comb *= nGroups
	}

	groupSteps := make([]float64, nGroups)
	for k, base := range baselines {
		reach := 0.0
		for tt, a := range base {
			s := a
			for _, d := range items {
				s += d[tt]
			}
			reach = max(reach, s)
		}
		if math.IsNaN(reach) || math.IsInf(reach, 0) {
			return 0, false
		}
		groupSteps[k] = max(1.0, math.Ceil(reach/b))
	}

	numMasks := 1 << nItems
	subsetCost := make([][]float64, nGroups)
	for k := range nGroups {
		row := make([]float64, numMasks)
		for mask := range row {
			row[mask] = -1.0
		}
		subsetCost[k] = row
	}

	yBuf := make([]float64, tPeriods)
	costFor := func(k, mask int) float64 {
		if c := subsetCost[k][mask]; c >= 0 {
			return c
		}
		copy(yBuf, baselines[k])
		for i := range nItems {
			if mask&(1<<i) != 0 {
				d := items[i]
				for tt := range tPeriods {
					yBuf[tt] += d[tt]
				}
			}
		}
		c, _, _ := exactPMADGroupCost(yBuf, b, lambda, groupSteps[k])
		subsetCost[k][mask] = c
		return c
	}

	groupMasks := make([]int, nGroups)
	best := math.Inf(1)
	var dfs func(itemIdx int)
	dfs = func(itemIdx int) {
		if itemIdx == nItems {
			total := 0.0
			for k := range nGroups {
				total += costFor(k, groupMasks[k])
			}
			if total < best {
				best = total
			}
			return
		}
		bit := 1 << itemIdx
		for k := range nGroups {
			groupMasks[k] |= bit
			dfs(itemIdx + 1)
			groupMasks[k] &^= bit
		}
	}
	dfs(0)
	return best, true
}

func verifyPMADResult(
	t *testing.T,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	exactOpt float64,
	hasExactOpt bool,
	absTol, relTol float64,
	requireOptimal, checkExactObj bool,
) bool {
	t.Helper()

	if res.Status == gb.StatusNumericalError {
		t.Fatalf("[%s] unexpected NumericalError (nodes=%d iters=%d obj=%.10g bound=%.10g)",
			label, res.Nodes, res.Iterations, res.Objective, res.BestBound)
	}
	if requireOptimal {
		if res.Status != gb.StatusOptimal {
			t.Fatalf("[%s] status=%v (nodes=%d iters=%d obj=%.10g bound=%.10g), want Optimal",
				label, res.Status, res.Nodes, res.Iterations, res.Objective, res.BestBound)
		}
	} else if res.Status != gb.StatusOptimal && !isResourceLimitStatus(res.Status) {
		t.Fatalf("[%s] unexpected status=%v (nodes=%d iters=%d)", label, res.Status, res.Nodes, res.Iterations)
	}

	verifyMILPInvariants(t, label, p, res)
	hasInc := verifyOptionalIncumbent(t, label, p, res)

	if math.IsNaN(res.BestBound) {
		t.Fatalf("[%s] non-finite BestBound=NaN (status=%v)", label, res.Status)
	}
	if math.IsNaN(res.RootLPBound) {
		t.Fatalf("[%s] non-finite RootLPBound=NaN (status=%v)", label, res.Status)
	}
	if (res.Status == gb.StatusOptimal || hasInc) && !hasFiniteBound(res.Objective) {
		t.Fatalf("[%s] non-finite Objective=%.10g (status=%v hasInc=%t)", label, res.Objective, res.Status, hasInc)
	}

	if res.Status == gb.StatusOptimal {
		if !hasInc {
			t.Fatalf("[%s] StatusOptimal missing primal vector (len=%d, want=%d)", label, len(res.Values), p.NumVars())
		}
		if !hasFiniteBound(res.BestBound) {
			t.Fatalf("[%s] StatusOptimal non-finite BestBound=%.10g", label, res.BestBound)
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
			t.Fatalf("[%s] primal violation bound=%.3e row=%.3e int=%.3e exceeds 1e-6", label, bV, rV, iV)
		}
		evalObj := p.EvaluateObjective(res.Values)
		evalTol := 1e-6 * max(1.0, math.Abs(res.Objective))
		if math.Abs(evalObj-res.Objective) > evalTol {
			t.Fatalf("[%s] EvaluateObjective=%.10g != res.Objective=%.10g", label, evalObj, res.Objective)
		}
		allowedGap := max(absTol, relTol*max(1.0, math.Abs(res.Objective)))
		gapTol := 1e-6 * max(1.0, math.Abs(res.Objective))
		if res.Objective-res.BestBound > allowedGap+gapTol {
			t.Fatalf("[%s] gap Objective=%.10g - BestBound=%.10g exceeds allowedGap=%.10g",
				label, res.Objective, res.BestBound, allowedGap)
		}
	}

	if hasExactOpt {
		eps := 1e-6 * max(1.0, math.Abs(exactOpt))
		if hasFiniteBound(res.BestBound) && res.BestBound > exactOpt+eps {
			t.Fatalf("[%s] BestBound=%.10g exceeds exactOpt=%.10g", label, res.BestBound, exactOpt)
		}
		if hasFiniteBound(res.RootLPBound) && res.RootLPBound > exactOpt+eps {
			t.Fatalf("[%s] RootLPBound=%.10g exceeds exactOpt=%.10g", label, res.RootLPBound, exactOpt)
		}
		if hasInc && res.Objective < exactOpt-eps {
			t.Fatalf("[%s] incumbent Objective=%.10g undercuts exactOpt=%.10g", label, res.Objective, exactOpt)
		}
		if res.Status == gb.StatusOptimal {
			if checkExactObj || (absTol == 0 && relTol == 0) {
				if math.Abs(res.Objective-exactOpt) > eps {
					t.Fatalf("[%s] Objective=%.10g != exactOpt=%.10g", label, res.Objective, exactOpt)
				}
			} else {
				allowedGap := max(absTol, relTol*max(1.0, math.Abs(res.Objective)))
				if res.Objective-exactOpt > allowedGap+eps {
					t.Fatalf("[%s] Objective=%.10g exceeds exactOpt=%.10g + allowedGap=%.10g",
						label, res.Objective, exactOpt, allowedGap)
				}
			}
		}
	}
	return hasInc
}

func runPMADPresolveAndToleranceMatrix(
	t *testing.T,
	tag string,
	p *gb.CompiledProblem,
	spec pmadSpec,
	exactOpt float64,
	forceExactOnPMADTols bool,
	skipSlowRaceShort bool,
) {
	t.Helper()
	tPeriods := len(spec.baselines[0])
	tols := []struct {
		name   string
		absTol float64
		relTol float64
		isZero bool
	}{
		{name: "pmad", absTol: float64(tPeriods) * spec.absTol, relTol: spec.relTol, isZero: false},
		{name: "zero", absTol: 0, relTol: 0, isZero: true},
	}
	for _, pre := range []bool{true, false} {
		for _, tol := range tols {
			subName := fmt.Sprintf("presolve_%t/%s", pre, tol.name)
			t.Run(subName, func(t *testing.T) {
				t.Parallel()
				if skipSlowRaceShort && raceEnabled && testing.Short() && pre && tol.isZero {
					t.Skip("skipping high-node zero-tolerance presolve branch under -race -short")
				}
				opts := []gb.Option{
					gb.WithPureGo(),
					gb.WithPresolve(pre),
					gb.WithMaxNodes(spec.maxNodes),
					gb.WithAbsGapTol(tol.absTol),
					gb.WithRelGapTol(tol.relTol),
				}
				if spec.workersOpt {
					opts = append(opts, gb.WithWorkers(1))
				}
				label := tag + "/" + subName
				res := mustSolveCompiled(t, p, opts...)
				checkExact := tol.isZero || forceExactOnPMADTols
				verifyPMADResult(t, label, p, res, exactOpt, true, tol.absTol, tol.relTol, true, checkExact)
			})
		}
	}
}

func TestPMADBaseRegression(t *testing.T) {
	t.Parallel()

	spec := basePMADSpec()
	exactOpt, ok := exactPMADObjective(spec.items, spec.baselines, spec.b, spec.lambda, 1000)
	if !ok || math.Abs(exactOpt-24137) > 1e-9 {
		t.Fatalf("exactPMADObjective=(%.10g, %t), want (24137, true)", exactOpt, ok)
	}

	m := buildPMADModel("pmad_base", spec)
	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}

	verifyRootLPBothModes(t, "pmad_base", p, 0, false)
	runPMADPresolveAndToleranceMatrix(t, "pmad_base", p, spec, exactOpt, true, false)
}

func TestPMADPerturbedSeedsRegression(t *testing.T) {
	t.Parallel()

	seeds := []struct {
		seed      uint64
		wantExact float64
		maxNodes  int
	}{
		{seed: 11, wantExact: 24124, maxNodes: 50000},
		{seed: 15, wantExact: 24193, maxNodes: 50000},
		{seed: 21, wantExact: 24155, maxNodes: 50000},
		{seed: 25, wantExact: 24137, maxNodes: 100000},
		{seed: 38, wantExact: 24220, maxNodes: 50000},
	}

	base := basePMADSpec()
	for _, tc := range seeds {
		t.Run(fmt.Sprintf("seed_%d", tc.seed), func(t *testing.T) {
			t.Parallel()

			spec := perturbPMADSpec(base, tc.seed)
			spec.maxNodes = tc.maxNodes
			exactOpt, ok := exactPMADObjective(spec.items, spec.baselines, spec.b, spec.lambda, 1000)
			if !ok || math.Abs(exactOpt-tc.wantExact) > 1e-9 {
				t.Fatalf("[seed_%d] exactPMADObjective=(%.10g, %t), want (%.10g, true)",
					tc.seed, exactOpt, ok, tc.wantExact)
			}

			tag := fmt.Sprintf("pmad_seed_%d", tc.seed)
			m := buildPMADModel(tag, spec)
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
			}

			verifyRootLPBothModes(t, tag, p, 0, false)
			runPMADPresolveAndToleranceMatrix(t, tag, p, spec, exactOpt, false, tc.seed == 25)
		})
	}
}

// TestPMADDualSimplexFMACyclingRegression guards against dual simplex cycling
// on degenerate zero-cost PMAD LPs under FMA (GOAMD64=v3 / arm64), discovered
// via FuzzPMADStructuralAndOracle seed 4a8e9a9cae07660b.
//
// Under Harris two-pass dual ratio testing, wrong-sign or sub-effHarris
// floating-point reduced-cost noise divided by small pivots can exceed
// degenStepTol in absolute value; resetting degenStreak/stallStreak on those
// steps previously prevented dual cost perturbation and Bland's rule from
// activating, causing the 90-variable root LP to hit the 50,000-iteration limit.
func TestPMADDualSimplexFMACyclingRegression(t *testing.T) {
	t.Parallel()

	meta := decodeFuzzPMADSpec(14, '"', '/', '\x17', 0, 0)
	tag := fmt.Sprintf("pmad_k%d_i%d_t%d_r%d_l%.2g_%d",
		meta.nGroups, meta.nItems, meta.nPeriods, meta.regime, meta.spec.lambda, uint64(14))
	m := buildPMADModel(tag, meta.spec)
	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
	}

	var objs [2]float64
	for idx, disableSIMD := range []bool{false, true} {
		mode := "simd"
		if disableSIMD {
			mode = "scalar"
		}
		lp := gb.NewLPSolver(p, nil)
		lp.DisableSIMD = disableSIMD
		lp.MaxIter = max(lp.MaxIter, 50000)
		res := lp.Solve()
		lp.Close()
		if res.Status != gb.StatusOptimal {
			t.Fatalf("[%s/%s] root LP status=%v (iters=%d vars=%d), want Optimal",
				tag, mode, res.Status, res.Iterations, p.NumVars())
		}
		if res.Iterations >= 2000 {
			t.Fatalf("[%s/%s] root LP took %d iterations (want < 2000; possible dual cycling)",
				tag, mode, res.Iterations)
		}
		bV, rV, _ := p.MaxViolation(res.Primal, false)
		if bV > 1e-6 || rV > 1e-6 {
			t.Fatalf("[%s/%s] root LP primal violation b=%.3e r=%.3e", tag, mode, bV, rV)
		}
		evalObj := p.EvaluateObjective(res.Primal)
		evalTol := 1e-5 * max(1.0, math.Abs(res.Objective))
		if !hasFiniteBound(evalObj) || math.Abs(evalObj-res.Objective) > evalTol {
			t.Fatalf("[%s/%s] root LP EvaluateObjective=%.10g != res.Objective=%.10g",
				tag, mode, evalObj, res.Objective)
		}
		objs[idx] = res.Objective
	}
	tol := 1e-5 * max(1.0, max(math.Abs(objs[0]), math.Abs(objs[1])))
	if math.Abs(objs[0]-objs[1]) > tol {
		t.Fatalf("[%s] root LP simd=%.10g != scalar=%.10g", tag, objs[0], objs[1])
	}

	resPre := mustSolveCompiled(t, p,
		gb.WithPureGo(),
		gb.WithPresolve(true),
		gb.WithCuts(true),
		gb.WithHeuristics(true),
		gb.WithWorkers(1),
		gb.WithAbsGapTol(meta.totalAbs),
		gb.WithRelGapTol(meta.spec.relTol),
		gb.WithMaxNodes(2000),
		gb.WithTimeLimit(2*time.Second),
	)
	hasIncPre := verifyPMADResult(t, tag+"/pre", p, resPre, meta.exactOpt, meta.hasExact, meta.totalAbs, meta.spec.relTol, false, false)

	resNoPre := mustSolveCompiled(t, p,
		gb.WithPureGo(),
		gb.WithPresolve(false),
		gb.WithWorkers(1),
		gb.WithAbsGapTol(meta.totalAbs),
		gb.WithRelGapTol(meta.spec.relTol),
		gb.WithMaxNodes(2000),
		gb.WithTimeLimit(2*time.Second),
	)
	hasIncNoPre := verifyPMADResult(t, tag+"/nopre", p, resNoPre, meta.exactOpt, meta.hasExact, meta.totalAbs, meta.spec.relTol, false, false)

	assertPMADSolvesConsistent(t, tag, "pre", resPre, hasIncPre, "nopre", resNoPre, hasIncNoPre, meta.totalAbs, meta.spec.relTol)
}
