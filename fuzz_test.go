package goblin_test

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
)

// exactMADObjective computes the exact ground-truth optimal objective of a MAD group-item
// allocation problem with nG groups, nI items, nT periods, rounding base B, and non-negative
// weights w by exhaustive enumeration over all valid binary assignments x (each item in {0..nI-1}
// assigned to 1 group, and each group receiving 1 item from D = {nI-nG..nI-1}).
//
// For a fixed assignment of items to group g with period sums S[0..nT-1], the subproblem
//
//	min_{m >= 0, n in Z_{>=1}} sum_{t=0..nT-1} |S[t] - m| + |m - B*n|
//
// attains its minimum at an extreme point where m in {S[0], ..., S[nT-1], B*n} and
// n in {max(1, floor(S[t]/B)), max(1, ceil(S[t]/B)) : 0 <= t < nT}.
func exactMADObjective(nG, nI, nT int, roundingBase float64, w [][]float64) float64 {
	dStart := nI - nG
	assign := make([]int, nI)
	usedD := make([]bool, nG)
	sums := make([][]float64, nG)
	for g := range nG {
		sums[g] = make([]float64, nT)
	}

	groupCost := func(s []float64) float64 {
		candN := make([]float64, 0, 2*nT+1)
		candN = append(candN, 1.0)
		for _, v := range s {
			fl := max(1.0, math.Floor(v/roundingBase))
			cl := max(1.0, math.Ceil(v/roundingBase))
			candN = append(candN, fl, cl)
		}
		best := math.Inf(1)
		evalM := func(mVal, bn float64) {
			if mVal < 0 {
				return
			}
			cost := math.Abs(mVal - bn)
			for _, v := range s {
				cost += math.Abs(v - mVal)
			}
			if cost < best {
				best = cost
			}
		}
		for _, nVal := range candN {
			bn := roundingBase * nVal
			evalM(bn, bn)
			for _, v := range s {
				evalM(v, bn)
			}
		}
		return best
	}

	bestTotal := math.Inf(1)
	var dfs func(i int)
	dfs = func(i int) {
		if i == nI {
			for g := range nG {
				clear(sums[g])
			}
			for item, g := range assign {
				for t := range nT {
					sums[g][t] += w[t][item]
				}
			}
			total := 0.0
			for g := range nG {
				total += groupCost(sums[g])
				if total >= bestTotal {
					return
				}
			}
			bestTotal = total
			return
		}
		if i >= dStart {
			for g := range nG {
				if !usedD[g] {
					usedD[g] = true
					assign[i] = g
					dfs(i + 1)
					usedD[g] = false
				}
			}
			return
		}
		// Break group symmetry on item 0 by restricting it to group 0 would not apply
		// unless designated items are also permuted, which dfs handles over all G! permutations.
		limitG := nG
		if i == 0 {
			limitG = 1
		}
		for g := range limitG {
			assign[i] = g
			dfs(i + 1)
		}
	}
	dfs(0)
	return bestTotal
}

func TestCanonicalMADFormulation(t *testing.T) {
	w := [][]float64{
		{8313, 0, 5258, 10400},
		{4344, 50, 8917, 10350},
		{3716, 70, 9308, 10330},
		{13203, 70, 3331, 10330},
		{7885, 700, 8347, 9700},
	}
	m := canonicalMADSmallModel()
	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}
	// G=2, I=4, T=5:
	// Vars = G*(I + T + 3) = 2*(4 + 5 + 3) = 24
	// Rows = G*(2*T + 3) + I = 2*(10 + 3) + 4 = 30
	if p.NumVars != 24 || p.NumConstraints != 30 {
		t.Fatalf("unexpected dimensions: vars=%d (want 24), rows=%d (want 30)", p.NumVars, p.NumConstraints)
	}

	wantObj := exactMADObjective(2, 4, 5, 100.0, w)
	if math.Abs(wantObj-6510.0) > 1e-9 {
		t.Fatalf("ground-truth oracle returned %.4f, want 6510.0", wantObj)
	}

	for name, opts := range optionVariants() {
		assertOptimal(t, "mad_canonical/"+name, p, goblin.SolveCompiled(p, opts...), wantObj)
	}
	if goblin.CBCAvailable() {
		assertOptimal(t, "mad_canonical/cbc", p, goblin.SolveCompiled(p, goblin.WithCBC()), wantObj)
	}
}

func buildFuzzMAD(rng *rand.Rand, tag string) (*goblin.Model, float64) {
	nG := 2 + rng.IntN(2) // 2 or 3 groups
	nI := nG + 1 + rng.IntN(4)
	nT := 3 + rng.IntN(4)
	bases := []float64{10.0, 50.0, 100.0}
	roundingBase := bases[rng.IntN(len(bases))]

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
		for i := range nI {
			switch rng.IntN(5) {
			case 0:
				w[t][i] = 0
			case 1:
				w[t][i] = roundingBase * float64(1+rng.IntN(20))
			default:
				w[t][i] = float64(rng.IntN(2500))
			}
		}
	}
	mdl := bench.BuildMADFromWeights(tag, nG, nI, nT, roundingBase, w)
	want := exactMADObjective(nG, nI, nT, roundingBase, w)
	return mdl, want
}

func buildFuzzGeneralMILP(rng *rand.Rand, tag string) *goblin.Model {
	n := 3 + rng.IntN(8)
	mRows := 2 + rng.IntN(6)
	sense := goblin.Minimize
	if rng.IntN(2) == 1 {
		sense = goblin.Maximize
	}
	mdl := goblin.NewModel(tag, sense)
	xs := make([]goblin.Var, n)
	obj := goblin.NewExpr()
	for j := range n {
		vname := fmt.Sprintf("x%d", j)
		switch rng.IntN(3) {
		case 0:
			xs[j] = mdl.AddBinaryVar(vname)
		case 1:
			lo := float64(rng.IntN(7) - 3)
			hi := lo + float64(1+rng.IntN(6))
			xs[j] = mdl.AddIntVar(vname, lo, hi)
		default:
			lo := float64(rng.IntN(9) - 4)
			hi := lo + float64(2+rng.IntN(12))
			xs[j] = mdl.AddContVar(vname, lo, hi)
		}
		c := float64(rng.IntN(19)-9) + 0.25*float64(rng.IntN(4))
		if c != 0 {
			obj = obj.AddTerm(xs[j], c)
		}
	}
	mdl.SetObjective(obj)

	for i := range mRows {
		e := goblin.NewExpr()
		for j := range n {
			if rng.Float64() < 0.65 {
				a := float64(rng.IntN(13)-6) + 0.5*float64(rng.IntN(2))
				if a != 0 {
					e = e.AddTerm(xs[j], a)
				}
			}
		}
		if len(e.Terms) == 0 {
			continue
		}
		rhs := float64(rng.IntN(25)-8) + 0.5*float64(rng.IntN(2))
		cname := fmt.Sprintf("c%d", i)
		switch rng.IntN(3) {
		case 0:
			mdl.AddConstraint(cname, goblin.Le(e, rhs))
		case 1:
			mdl.AddConstraint(cname, goblin.Ge(e, rhs-6))
		default:
			mdl.AddConstraint(cname, goblin.Eq(e, rhs))
		}
	}
	return mdl
}

func buildFuzzEqKnapsack(rng *rand.Rand, tag string) *goblin.Model {
	n := 4 + rng.IntN(5)
	mdl := goblin.NewModel(tag, goblin.Minimize)
	xs := make([]goblin.Var, n)
	obj := goblin.NewExpr()
	eq := goblin.NewExpr()
	for j := range n {
		xs[j] = mdl.AddIntVar(fmt.Sprintf("x%d", j), 0, float64(2+rng.IntN(5)))
		obj = obj.AddTerm(xs[j], float64(1+rng.IntN(15)))
		eq = eq.AddTerm(xs[j], float64(3+rng.IntN(30)))
	}
	mdl.SetObjective(obj)
	mdl.AddConstraint("eq", goblin.Eq(eq, float64(20+rng.IntN(90))))
	if rng.IntN(2) == 1 {
		side := goblin.NewExpr()
		for j := range n {
			a := float64(rng.IntN(17) - 8)
			if a != 0 {
				side = side.AddTerm(xs[j], a)
			}
		}
		if len(side.Terms) > 0 {
			mdl.AddConstraint("side", goblin.Le(side, float64(rng.IntN(20))))
		}
	}
	return mdl
}

func buildFuzzScaledMILP(rng *rand.Rand, tag string) *goblin.Model {
	n := 4 + rng.IntN(6)
	mRows := 2 + rng.IntN(4)
	mdl := goblin.NewModel(tag, goblin.Maximize)
	xs := make([]goblin.Var, n)
	obj := goblin.NewExpr()
	bounds := []float64{1, 5, 25}
	for j := range n {
		ub := bounds[rng.IntN(len(bounds))]
		if rng.IntN(2) == 0 {
			xs[j] = mdl.AddIntVar(fmt.Sprintf("x%d", j), 0, ub)
		} else {
			xs[j] = mdl.AddContVar(fmt.Sprintf("x%d", j), 0, ub)
		}
		obj = obj.AddTerm(xs[j], math.Round((0.2+rng.Float64()*8.0)*1e3)/1e3)
	}
	mdl.SetObjective(obj)
	exps := []float64{1e-2, 1e-1, 1.0, 1e1, 1e2, 1e3}
	for i := range mRows {
		scale := exps[rng.IntN(len(exps))]
		e := goblin.NewExpr()
		for j := range n {
			coeff := math.Round((0.1+rng.Float64()*4.0)*scale*1e4) / 1e4
			e = e.AddTerm(xs[j], coeff)
		}
		rhs := math.Round((3.0+rng.Float64()*30.0)*scale*1e4) / 1e4
		mdl.AddConstraint(fmt.Sprintf("r%d", i), goblin.Le(e, rhs))
	}
	return mdl
}

func verifyMILPInvariants(t *testing.T, label string, p *goblin.CompiledProblem, res goblin.Result) {
	t.Helper()
	if res.Status != goblin.StatusOptimal {
		return
	}
	if len(res.Values) != p.NumVars {
		t.Fatalf("[%s] Optimal status with len(Values)=%d, want %d", label, len(res.Values), p.NumVars)
	}
	bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP)
	if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
		t.Fatalf("[%s] Optimal solution violates model: bound=%.2e row=%.2e int=%.2e", label, bV, rV, iV)
	}
	evalObj := p.EvaluateObjective(res.Values)
	if math.Abs(evalObj-res.Objective) > 1e-6*max(1.0, math.Abs(res.Objective)) {
		t.Fatalf("[%s] EvaluateObjective=%.8f != res.Objective=%.8f", label, evalObj, res.Objective)
	}
	boundTol := 1e-5 * max(1.0, math.Abs(res.Objective))
	if p.Sense == goblin.Minimize {
		if res.BestBound > res.Objective+boundTol {
			t.Fatalf("[%s] Minimize BestBound %.8f > Objective %.8f", label, res.BestBound, res.Objective)
		}
		if !math.IsNaN(res.RootLPBound) && res.BestBound < res.RootLPBound-boundTol {
			t.Fatalf("[%s] Minimize BestBound %.8f < RootLPBound %.8f", label, res.BestBound, res.RootLPBound)
		}
	} else {
		if res.BestBound < res.Objective-boundTol {
			t.Fatalf("[%s] Maximize BestBound %.8f < Objective %.8f", label, res.BestBound, res.Objective)
		}
		if !math.IsNaN(res.RootLPBound) && res.BestBound > res.RootLPBound+boundTol {
			t.Fatalf("[%s] Maximize BestBound %.8f > RootLPBound %.8f", label, res.BestBound, res.RootLPBound)
		}
	}
}

func FuzzMILPSolver(f *testing.F) {
	for seed := range uint64(25) {
		for fam := range uint8(5) {
			f.Add(seed, fam)
		}
	}

	f.Fuzz(func(t *testing.T, seed uint64, family uint8) {
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		tag := fmt.Sprintf("fuzz_%d_%d", family%5, seed)

		var mdl *goblin.Model
		var exactWant float64
		hasExactWant := false

		switch family % 5 {
		case 0:
			mdl, exactWant = buildFuzzMAD(rng, tag)
			hasExactWant = true
		case 1:
			mdl = buildFuzzGeneralMILP(rng, tag)
		case 2:
			mdl, _, _ = buildFixedCharge(seed)
		case 3:
			mdl = buildFuzzEqKnapsack(rng, tag)
		default:
			mdl = buildFuzzScaledMILP(rng, tag)
		}

		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}

		baseLimits := []goblin.Option{
			goblin.WithTimeLimit(1500 * time.Millisecond),
			goblin.WithMaxNodes(3000),
		}

		results := make(map[string]goblin.Result, 4)
		var firstOptName string
		var firstOptObj float64
		sawInfeasible := ""

		for name, vOpts := range optionVariants() {
			opts := append(append([]goblin.Option(nil), baseLimits...), vOpts...)
			res := goblin.SolveCompiled(p, opts...)
			results[name] = res
			verifyMILPInvariants(t, tag+"/"+name, p, res)

			switch res.Status {
			case goblin.StatusOptimal:
				if hasExactWant {
					rel := math.Abs(res.Objective-exactWant) / max(1.0, math.Abs(exactWant))
					if rel > 1e-5 {
						t.Fatalf("[%s/%s] MAD objective %.8f != exact ground truth %.8f (rel=%.2e)",
							tag, name, res.Objective, exactWant, rel)
					}
				}
				if firstOptName == "" {
					firstOptName = name
					firstOptObj = res.Objective
				} else {
					rel := math.Abs(res.Objective-firstOptObj) / max(1.0, math.Abs(firstOptObj))
					if rel > 1e-5 {
						t.Fatalf("[%s] objective disagreement between %s (%.8f) and %s (%.8f), rel=%.2e",
							tag, firstOptName, firstOptObj, name, res.Objective, rel)
					}
				}
			case goblin.StatusInfeasible:
				sawInfeasible = name
			}
		}

		if firstOptName != "" && sawInfeasible != "" {
			t.Fatalf("[%s] contradictory verdicts: %s returned Optimal (%.8f) while %s returned Infeasible",
				tag, firstOptName, firstOptObj, sawInfeasible)
		}
	})
}

func FuzzLPSolver(f *testing.F) {
	for seed := range uint64(30) {
		f.Add(seed, uint8(4+seed%10), uint8(3+seed%8))
	}

	f.Fuzz(func(t *testing.T, seed uint64, rawVars, rawRows uint8) {
		nVars := 2 + int(rawVars%14)
		nRows := 1 + int(rawRows%10)
		rng := rand.New(rand.NewPCG(seed, seed^0x517cc1b727220a95))

		sense := goblin.Minimize
		if rng.IntN(2) == 1 {
			sense = goblin.Maximize
		}
		m := goblin.NewModel(fmt.Sprintf("fuzz_lp_%d", seed), sense)
		xs := make([]goblin.Var, nVars)
		obj := goblin.NewExpr()
		for j := range nVars {
			lo := float64(rng.IntN(7) - 3)
			hi := lo + float64(1+rng.IntN(10))
			xs[j] = m.AddContVar(fmt.Sprintf("x%d", j), lo, hi)
			c := float64(rng.IntN(21)-10) * 0.5
			if c != 0 {
				obj = obj.AddTerm(xs[j], c)
			}
		}
		m.SetObjective(obj)

		for i := range nRows {
			e := goblin.NewExpr()
			for j := range nVars {
				if rng.Float64() < 0.55 {
					a := float64(rng.IntN(15)-7) * 0.5
					if a != 0 {
						e = e.AddTerm(xs[j], a)
					}
				}
			}
			if len(e.Terms) == 0 {
				continue
			}
			rhs := float64(rng.IntN(21) - 5)
			switch rng.IntN(3) {
			case 0:
				m.AddConstraint(fmt.Sprintf("r%d", i), goblin.Le(e, rhs))
			case 1:
				m.AddConstraint(fmt.Sprintf("r%d", i), goblin.Ge(e, rhs))
			default:
				m.AddConstraint(fmt.Sprintf("r%d", i), goblin.Eq(e, rhs))
			}
		}

		p := m.Compile()
		ws := goblin.AcquireWorkspace(p.NumVars, p.NumConstraints)
		defer goblin.ReleaseWorkspace(ws)

		lpSIMD := goblin.NewLPSolver(p, ws)
		resSIMD := lpSIMD.Solve()

		wsScalar := goblin.AcquireWorkspace(p.NumVars, p.NumConstraints)
		defer goblin.ReleaseWorkspace(wsScalar)
		lpScalar := goblin.NewLPSolver(p, wsScalar)
		lpScalar.DisableSIMD = true
		resScalar := lpScalar.Solve()

		if resSIMD.Status != resScalar.Status {
			t.Fatalf("[seed=%d] LP status mismatch: simd=%v scalar=%v", seed, resSIMD.Status, resScalar.Status)
		}
		if resSIMD.Status == goblin.StatusOptimal {
			bV, rV, _ := p.MaxViolation(resSIMD.Primal, false)
			if bV > 1e-5 || rV > 1e-5 {
				t.Fatalf("[seed=%d] LP primal violation: bound=%.2e row=%.2e", seed, bV, rV)
			}
			evalObj := p.EvaluateObjective(resSIMD.Primal)
			if math.Abs(evalObj-resSIMD.Objective) > 1e-6*max(1.0, math.Abs(resSIMD.Objective)) {
				t.Fatalf("[seed=%d] LP EvaluateObjective=%.8f != Objective=%.8f", seed, evalObj, resSIMD.Objective)
			}
			if math.Abs(resSIMD.Objective-resScalar.Objective) > 1e-7*max(1.0, math.Abs(resScalar.Objective)) {
				t.Fatalf("[seed=%d] LP SIMD vs Scalar objective mismatch: %.8f vs %.8f",
					seed, resSIMD.Objective, resScalar.Objective)
			}
		}
	})
}

func FuzzPuLPJSON(f *testing.F) {
	paths, _ := filepath.Glob("testdata/corpus/t1_*.json")
	for _, p := range paths {
		if raw, err := os.ReadFile(p); err == nil {
			f.Add(raw)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := format.ParsePuLPJSON(data)
		if err != nil {
			return
		}
		if len(doc.Variables) > 200 || len(doc.Constraints) > 200 {
			return
		}
		m, err := format.BuildModelFromPuLPJSON(doc)
		if err != nil {
			return
		}
		p, err := m.CompileChecked()
		if err != nil {
			return
		}
		var mpsBuf bytes.Buffer
		_ = format.WriteMPS(p, &mpsBuf)
		var jsonBuf bytes.Buffer
		if err := format.ToPuLPJSONCompact(p, &jsonBuf); err != nil {
			t.Fatalf("ToPuLPJSONCompact failed on validated CompiledProblem: %v", err)
		}
	})
}
