package goblin_test

import (
	"math"
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
