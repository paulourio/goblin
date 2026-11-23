package goblin_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
)

type fuzzRatJSON struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

func ratCeilBigInt(r *big.Rat) *big.Int {
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}

func exactEvalMinObjRat(p *gb.CompiledProblem, x []float64) (totalMin *big.Rat, linScale float64, ok bool) {
	if p == nil || len(x) < p.NumVars() {
		return nil, 0, false
	}
	constMin := p.ObjConstant()
	if p.Sense() == gb.Maximize && constMin != 0 {
		constMin = -constMin
	}
	totalMin = new(big.Rat).SetFloat64(constMin)
	if totalMin == nil {
		return nil, 0, false
	}
	cRat := new(big.Rat)
	xRat := new(big.Rat)
	prod := new(big.Rat)
	for j := range p.NumVars() {
		cj := p.ObjMinCoeff(j)
		xj := x[j]
		if math.IsNaN(cj) || math.IsInf(cj, 0) || math.IsNaN(xj) || math.IsInf(xj, 0) {
			return nil, 0, false
		}
		if cj == 0 || xj == 0 {
			continue
		}
		if cRat.SetFloat64(cj) == nil || xRat.SetFloat64(xj) == nil {
			return nil, 0, false
		}
		prod.Mul(cRat, xRat)
		totalMin.Add(totalMin, prod)
		if pAbs := math.Abs(cj * xj); !math.IsInf(pAbs, 0) && !math.IsNaN(pAbs) {
			linScale += pAbs
		}
	}
	return totalMin, linScale, true
}

func exactEvalLinearMinRat(p *gb.CompiledProblem, x []float64) (*big.Rat, bool) {
	if p == nil || len(x) < p.NumVars() {
		return nil, false
	}
	sum := new(big.Rat)
	cRat := new(big.Rat)
	xRat := new(big.Rat)
	prod := new(big.Rat)
	for j := range p.NumVars() {
		cj := p.ObjMinCoeff(j)
		xj := x[j]
		if cj == 0 || xj == 0 {
			continue
		}
		if cRat.SetFloat64(cj) == nil || xRat.SetFloat64(xj) == nil {
			return nil, false
		}
		prod.Mul(cRat, xRat)
		sum.Add(sum, prod)
	}
	return sum, true
}

func userBoundFromMinRat(p *gb.CompiledProblem, minRat *big.Rat) float64 {
	if minRat == nil {
		return math.NaN()
	}
	v, _ := minRat.Float64()
	if p != nil && p.Sense() == gb.Maximize && v != 0 {
		return -v
	}
	return v
}

func isExactRatMIPFeasible(p *gb.CompiledProblem, x []float64) bool {
	if p == nil || len(x) < p.NumVars() {
		return false
	}
	n := p.NumVars()
	xRats := make([]*big.Rat, n)
	bndRat := new(big.Rat)
	for j := range n {
		xj := x[j]
		if math.IsNaN(xj) || math.IsInf(xj, 0) {
			return false
		}
		xr := new(big.Rat).SetFloat64(xj)
		if xr == nil {
			return false
		}
		lb, ub := p.VarBounds(j)
		if !math.IsInf(lb, -1) {
			if bndRat.SetFloat64(lb) == nil || xr.Cmp(bndRat) < 0 {
				return false
			}
		}
		if !math.IsInf(ub, 1) {
			if bndRat.SetFloat64(ub) == nil || xr.Cmp(bndRat) > 0 {
				return false
			}
		}
		xRats[j] = xr
	}
	for k := range p.NumIntVars() {
		if !xRats[p.IntCol(k)].IsInt() {
			return false
		}
	}
	act := new(big.Rat)
	aRat := new(big.Rat)
	prod := new(big.Rat)
	rhsRat := new(big.Rat)
	for i := range p.NumConstraints() {
		if rhsRat.SetFloat64(p.RowRHS(i)) == nil {
			return false
		}
		act.SetInt64(0)
		rowOK := true
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if !rowOK || coeff == 0 || x[col] == 0 {
				return
			}
			if aRat.SetFloat64(coeff) == nil {
				rowOK = false
				return
			}
			prod.Mul(aRat, xRats[col])
			act.Add(act, prod)
		})
		if !rowOK {
			return false
		}
		cmp := act.Cmp(rhsRat)
		switch p.RowSense(i) {
		case gb.LessEqual:
			if cmp > 0 {
				return false
			}
		case gb.GreaterEqual:
			if cmp < 0 {
				return false
			}
		case gb.Equal:
			if cmp != 0 {
				return false
			}
		}
	}
	return true
}

func verifyFormalLPResult(tb testing.TB, label string, p *gb.CompiledProblem, lp *gb.LPSolver, res gb.LPResult) {
	tb.Helper()
	if !formalFuzzEnabled() {
		return
	}
	if err := checkFormalLPResult(tb, label, p, lp, res); err != nil {
		tb.Fatal(err)
	}
}

func verifyFormalPresolvedLP(tb testing.TB, label string, p *gb.CompiledProblem, checkDualGap bool) gb.Result {
	tb.Helper()
	resPre, err := gb.SolveCompiled(tb.Context(), p, gb.WithPureGo(), gb.WithPresolve(true))
	if err != nil {
		tb.Fatalf("[%s] SolveCompiled presolve LP failed: %v", label, err)
	}
	if err := checkFormalPresolvedLPResult(tb, label, p, resPre, checkDualGap); err != nil {
		tb.Fatal(err)
	}
	return resPre
}

func verifyFormalPresolveRules(tb testing.TB, p *gb.CompiledProblem) ([][]byte, gb.PresolveResult) {
	tb.Helper()
	if !formalFuzzEnabled() || p == nil {
		return nil, gb.PresolveResult{}
	}
	items, preRes := gb.ExportPresolveRulesItems(p)
	for _, item := range items {
		_ = mustFormalRulesCheck(tb, item)
	}
	return items, preRes
}

func verifyFormalRootLP(tb testing.TB, label string, p *gb.CompiledProblem) {
	tb.Helper()
	if !formalFuzzEnabled() {
		return
	}
	if p == nil {
		tb.Fatalf("[%s] nil CompiledProblem", label)
	}
	if p.NumVars() == 0 {
		return
	}
	ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	lp := gb.NewLPSolver(p, ws)
	res := lp.Solve()
	verifyFormalLPResult(tb, label, p, lp, res)
}

func verifyFormalSeparatedCuts(tb testing.TB, label string, p *gb.CompiledProblem, sols [][]float64) {
	tb.Helper()
	if !formalFuzzEnabled() {
		return
	}
	if p == nil {
		tb.Fatalf("[%s] nil CompiledProblem", label)
	}
	items, err := gb.ExportSeparatedCutsItems(p, sols)
	if err != nil {
		tb.Fatalf("[%s] ExportSeparatedCutsItems failed: %v", label, err)
	}
	for _, item := range items {
		mustFormalCutsCheck(tb, item)
	}
}

func verifyFormalVIPRTrace(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	maxNodes int,
	timeLimit time.Duration,
	extraOpts ...gb.Option,
) (gb.Result, *gb.VIPRTrace) {
	tb.Helper()
	if !formalFuzzEnabled() {
		return gb.Result{}, nil
	}
	if p == nil {
		tb.Fatalf("[%s] nil CompiledProblem", label)
	}
	tr := gb.NewVIPRTrace()
	opts := make([]gb.Option, 0, 4+len(extraOpts))
	opts = append(opts,
		gb.WithPureGo(),
		gb.WithMaxNodes(maxNodes),
		gb.WithTimeLimit(timeLimit),
		gb.WithVIPRTrace(tr),
	)
	opts = append(opts, extraOpts...)
	cfg := gb.ApplyOptions(opts...)
	res, err := gb.SolveCompiled(tb.Context(), p, opts...)
	if err != nil {
		tb.Fatalf("[%s] SolveCompiled with VIPRTrace failed: %v", label, err)
	}
	if err := checkFormalVIPRResult(tb, label, p, res, tr, cfg.RelGapTol, cfg.AbsGapTol); err != nil {
		tb.Fatal(err)
	}
	return res, tr
}

func assertFullFeatureVsVIPR(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	resFull gb.Result,
	resVIPR gb.Result,
	tr *gb.VIPRTrace,
	relTol, absTol float64,
) {
	tb.Helper()
	if err := checkFullFeatureVsVIPR(tb, label, p, resFull, resVIPR, tr, relTol, absTol); err != nil {
		tb.Fatal(err)
	}
}

func runFullFeatureVsVIPRCheck(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	workers int,
	maxNodes int,
	timeLimit time.Duration,
	relTol, absTol float64,
) (gb.Result, gb.Result, *gb.VIPRTrace) {
	tb.Helper()
	if workers < 1 {
		workers = 1
	}
	fullOpts := []gb.Option{
		gb.WithPureGo(),
		gb.WithPresolve(true),
		gb.WithCuts(true),
		gb.WithHeuristics(true),
		gb.WithWorkers(workers),
		gb.WithRelGapTol(relTol),
		gb.WithAbsGapTol(absTol),
		gb.WithMaxNodes(maxNodes),
		gb.WithTimeLimit(timeLimit),
	}
	resFull, err := gb.SolveCompiled(tb.Context(), p, fullOpts...)
	if err != nil {
		tb.Fatalf("[%s] full-feature SolveCompiled failed: %v", label, err)
	}
	verifyMILPInvariants(tb, label+"/full", p, resFull)
	if !formalFuzzEnabled() {
		return resFull, gb.Result{}, nil
	}
	resVIPR, tr := verifyFormalVIPRTrace(
		tb,
		label+"/vipr",
		p,
		maxNodes,
		timeLimit,
		gb.WithPresolve(false),
		gb.WithCuts(true),
		gb.WithHeuristics(true),
		gb.WithRelGapTol(relTol),
		gb.WithAbsGapTol(absTol),
	)
	assertFullFeatureVsVIPR(tb, label, p, resFull, resVIPR, tr, relTol, absTol)
	return resFull, resVIPR, tr
}

// exactMADObjective computes the reference optimal objective of a MAD allocation model
// by exhaustive enumeration over all valid binary group assignments.
// Each item in 0..nI-1 maps to one group, and each group receives one designated item.
// For fixed group period sums S, the inner subproblem attains its minimum at an extreme point
// where m lies in S or B*n and n equals 1, floor(S[t]/B), or ceil(S[t]/B).
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
	if p.NumVars() != 24 || p.NumConstraints() != 30 {
		t.Fatalf("unexpected dimensions: vars=%d (want 24), rows=%d (want 30)", p.NumVars(), p.NumConstraints())
	}

	wantObj := exactMADObjective(2, 4, 5, 100.0, w)
	if math.Abs(wantObj-6510.0) > 1e-9 {
		t.Fatalf("ground-truth oracle returned %.4f, want 6510.0", wantObj)
	}

	for name, opts := range optionVariants() {
		assertOptimal(t, "mad_canonical/"+name, p, mustSolveCompiled(t, p, opts...), wantObj)
	}
	if gb.CBCAvailable() {
		assertOptimal(t, "mad_canonical/cbc", p, mustSolveCompiled(t, p, gb.WithCBC()), wantObj)
	}
}

func buildFuzzMAD(rng *rand.Rand, tag string) (*gb.Model, float64) {
	nG := 2 + rng.IntN(2)
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

func buildFuzzGeneralMILP(rng *rand.Rand, tag string) *gb.Model {
	n := 3 + rng.IntN(8)
	mRows := 2 + rng.IntN(6)
	sense := gb.Minimize
	if rng.IntN(2) == 1 {
		sense = gb.Maximize
	}
	mdl := gb.NewModel(tag, sense)
	xs := make([]gb.Var, n)
	obj := gb.NewExpr()
	for j := range n {
		vname := "x" + strconv.Itoa(j)
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
		e := gb.NewExpr()
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
		cname := "c" + strconv.Itoa(i)
		switch rng.IntN(3) {
		case 0:
			mdl.AddConstraint(cname, gb.Le(e, rhs))
		case 1:
			mdl.AddConstraint(cname, gb.Ge(e, rhs-6))
		default:
			mdl.AddConstraint(cname, gb.Eq(e, rhs))
		}
	}
	return mdl
}

func buildFuzzEqKnapsack(rng *rand.Rand, tag string) *gb.Model {
	n := 4 + rng.IntN(5)
	mdl := gb.NewModel(tag, gb.Minimize)
	xs := make([]gb.Var, n)
	obj := gb.NewExpr()
	eq := gb.NewExpr()
	for j := range n {
		xs[j] = mdl.AddIntVar("x"+strconv.Itoa(j), 0, float64(2+rng.IntN(5)))
		obj = obj.AddTerm(xs[j], float64(1+rng.IntN(15)))
		eq = eq.AddTerm(xs[j], float64(3+rng.IntN(30)))
	}
	mdl.SetObjective(obj)
	mdl.AddConstraint("eq", gb.Eq(eq, float64(20+rng.IntN(90))))
	if rng.IntN(2) == 1 {
		side := gb.NewExpr()
		for j := range n {
			a := float64(rng.IntN(17) - 8)
			if a != 0 {
				side = side.AddTerm(xs[j], a)
			}
		}
		if len(side.Terms) > 0 {
			mdl.AddConstraint("side", gb.Le(side, float64(rng.IntN(20))))
		}
	}
	return mdl
}

func buildFuzzScaledMILP(rng *rand.Rand, tag string) *gb.Model {
	n := 4 + rng.IntN(6)
	mRows := 2 + rng.IntN(4)
	mdl := gb.NewModel(tag, gb.Maximize)
	xs := make([]gb.Var, n)
	obj := gb.NewExpr()
	bounds := []float64{1, 5, 25}
	for j := range n {
		ub := bounds[rng.IntN(len(bounds))]
		if rng.IntN(2) == 0 {
			xs[j] = mdl.AddIntVar("x"+strconv.Itoa(j), 0, ub)
		} else {
			xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), 0, ub)
		}
		obj = obj.AddTerm(xs[j], math.Round((0.2+rng.Float64()*8.0)*1e3)/1e3)
	}
	mdl.SetObjective(obj)
	exps := []float64{1e-2, 1e-1, 1.0, 1e1, 1e2, 1e3}
	for i := range mRows {
		scale := exps[rng.IntN(len(exps))]
		e := gb.NewExpr()
		for j := range n {
			coeff := math.Round((0.1+rng.Float64()*4.0)*scale*1e4) / 1e4
			e = e.AddTerm(xs[j], coeff)
		}
		rhs := math.Round((3.0+rng.Float64()*30.0)*scale*1e4) / 1e4
		mdl.AddConstraint("r"+strconv.Itoa(i), gb.Le(e, rhs))
	}
	return mdl
}

func verifyMILPInvariants(tb testing.TB, label string, p *gb.CompiledProblem, res gb.Result) {
	tb.Helper()
	if err := checkMILPInvariantsErr(label, p, res); err != nil {
		tb.Fatal(err)
	}
}

func exactFixedChargeObjective(p *gb.CompiledProblem, y []gb.Var) (float64, bool) {
	nVars := p.NumVars()
	baseLB := make([]float64, nVars)
	baseUB := make([]float64, nVars)
	for j := range nVars {
		baseLB[j], baseUB[j] = p.VarBounds(j)
	}
	ws := gb.AcquireWorkspace(nVars, p.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	best := math.Inf(1)
	found := false
	for mask := 1; mask < 1<<len(y); mask++ {
		lb := slices.Clone(baseLB)
		ub := slices.Clone(baseUB)
		for i, v := range y {
			val := float64(mask >> i & 1)
			lb[v.Index()], ub[v.Index()] = val, val
		}
		s := gb.NewLPSolver(p, ws)
		s.SetStructuralBounds(lb, ub)
		if res := s.Solve(); res.Status == gb.StatusOptimal {
			best = min(best, res.Objective)
			found = true
		}
	}
	return best, found
}

func exactSmallMIPObjective(p *gb.CompiledProblem) (want float64, feasible, ok bool) {
	nInt := p.NumIntVars()
	if nInt == 0 || nInt > 10 {
		return 0, false, false
	}
	intCols := make([]int, nInt)
	loVals := make([]int, nInt)
	span := make([]int, nInt)
	total := 1
	for k := range nInt {
		col := p.IntCol(k)
		lb, ub := p.VarBounds(col)
		if math.IsInf(lb, 0) || math.IsInf(ub, 0) {
			return 0, false, false
		}
		lo := int(math.Ceil(lb - 1e-9))
		hi := int(math.Floor(ub + 1e-9))
		if hi < lo {
			return 0, false, true
		}
		cnt := hi - lo + 1
		if cnt > 64 || total*cnt > 64 {
			return 0, false, false
		}
		intCols[k] = col
		loVals[k] = lo
		span[k] = cnt
		total *= cnt
	}
	nVars := p.NumVars()
	baseLB := make([]float64, nVars)
	baseUB := make([]float64, nVars)
	for j := range nVars {
		baseLB[j], baseUB[j] = p.VarBounds(j)
	}
	ws := gb.AcquireWorkspace(nVars, p.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	best := math.Inf(1)
	if p.Sense() == gb.Maximize {
		best = math.Inf(-1)
	}
	lb := make([]float64, nVars)
	ub := make([]float64, nVars)
	for idx := range total {
		copy(lb, baseLB)
		copy(ub, baseUB)
		rem := idx
		for k := range nInt {
			v := float64(loVals[k] + rem%span[k])
			rem /= span[k]
			lb[intCols[k]] = v
			ub[intCols[k]] = v
		}
		s := gb.NewLPSolver(p, ws)
		s.SetStructuralBounds(lb, ub)
		res := s.Solve()
		if res.Status == gb.StatusUnbounded {
			return 0, false, false
		}
		if res.Status == gb.StatusOptimal {
			feasible = true
			if p.Sense() == gb.Maximize {
				best = max(best, res.Objective)
			} else {
				best = min(best, res.Objective)
			}
		}
	}
	return best, feasible, true
}

func FuzzMILPSolver(f *testing.F) {
	for seed := range uint64(12) {
		for fam := range uint8(5) {
			f.Add(seed, fam)
		}
	}

	f.Fuzz(func(t *testing.T, seed uint64, family uint8) {
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		tag := fmt.Sprintf("fuzz_%d_%d", family%5, seed)

		var mdl *gb.Model
		var fcDepots []gb.Var
		var exactWant float64
		hasExactWant := false

		switch family % 5 {
		case 0:
			mdl, exactWant = buildFuzzMAD(rng, tag)
			hasExactWant = true
		case 1:
			mdl = buildFuzzGeneralMILP(rng, tag)
		case 2:
			mdl, fcDepots, _ = buildFixedCharge(seed)
		case 3:
			mdl = buildFuzzEqKnapsack(rng, tag)
		default:
			mdl = buildFuzzScaledMILP(rng, tag)
		}

		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, err)
		}
		exactInfeasible := false
		if len(fcDepots) > 0 {
			if want, ok := exactFixedChargeObjective(p, fcDepots); ok {
				exactWant = want
				hasExactWant = true
			} else {
				exactInfeasible = true
			}
		} else if !hasExactWant {
			if want, feas, ok := exactSmallMIPObjective(p); ok {
				if feas {
					exactWant = want
					hasExactWant = true
				} else {
					exactInfeasible = true
				}
			}
		}

		baseLimits := []gb.Option{
			gb.WithTimeLimit(1500 * time.Millisecond),
			gb.WithMaxNodes(3000),
		}

		var firstOptName string
		var firstOptObj float64
		var optValues []float64
		sawInfeasible := ""
		sawUnbounded := ""

		for name, vOpts := range optionVariants() {
			opts := append(slices.Clone(baseLimits), vOpts...)
			res := mustSolveCompiled(t, p, opts...)
			verifyMILPInvariants(t, tag+"/"+name, p, res)

			switch res.Status {
			case gb.StatusOptimal:
				if exactInfeasible {
					t.Fatalf("[%s/%s] returned Optimal %.8f on enumerated-infeasible MIP", tag, name, res.Objective)
				}
				if hasExactWant {
					rel := math.Abs(res.Objective-exactWant) / max(1.0, math.Abs(exactWant))
					if rel > 1e-5 {
						t.Fatalf("[%s/%s] objective %.8f != exact ground truth %.8f (rel=%.2e)",
							tag, name, res.Objective, exactWant, rel)
					}
				}
				if firstOptName == "" {
					firstOptName = name
					firstOptObj = res.Objective
					optValues = res.Values
				} else {
					rel := math.Abs(res.Objective-firstOptObj) / max(1.0, math.Abs(firstOptObj))
					if rel > 1e-5 {
						t.Fatalf("[%s] objective disagreement between %s (%.8f) and %s (%.8f), rel=%.2e",
							tag, firstOptName, firstOptObj, name, res.Objective, rel)
					}
				}
			case gb.StatusInfeasible:
				if hasExactWant {
					t.Fatalf("[%s/%s] returned Infeasible when exact ground truth is %.8f", tag, name, exactWant)
				}
				sawInfeasible = name
			case gb.StatusUnbounded:
				if hasExactWant || exactInfeasible {
					t.Fatalf("[%s/%s] returned Unbounded on bounded/enumerated MIP", tag, name)
				}
				sawUnbounded = name
				if formalFuzzEnabled() {
					if err := checkFormalUnboundedMILP(t, tag+"/"+name, p, res.Values); err != nil {
						t.Fatal(err)
					}
				}
			}
		}

		if firstOptName != "" && sawInfeasible != "" {
			t.Fatalf("[%s] contradictory verdicts: %s returned Optimal (%.8f) while %s returned Infeasible",
				tag, firstOptName, firstOptObj, sawInfeasible)
		}
		if sawUnbounded != "" && (firstOptName != "" || sawInfeasible != "") {
			t.Fatalf("[%s] contradictory verdicts: %s returned Unbounded vs opt=%q infeas=%q",
				tag, sawUnbounded, firstOptName, sawInfeasible)
		}

		if formalFuzzEnabled() {
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, tag+"/rootLP", p)
			if len(optValues) == p.NumVars() {
				verifyFormalSeparatedCuts(t, tag+"/cuts", p, [][]float64{optValues})
			}
			if p.NumVars() <= 128 && p.NumConstraints() <= 128 {
				workers := [3]int{1, 2, 4}[int(seed)%3]
				_, _, _ = runFullFeatureVsVIPRCheck(t, tag+"/full_vs_vipr", p, workers, 1500, 1500*time.Millisecond, 1e-6, 1e-6)
			}
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

		sense := gb.Minimize
		if rng.IntN(2) == 1 {
			sense = gb.Maximize
		}
		m := gb.NewModel("fuzz_lp_"+strconv.FormatUint(seed, 10), sense)
		xs := make([]gb.Var, nVars)
		obj := gb.NewExpr()
		for j := range nVars {
			lo := float64(rng.IntN(7) - 3)
			hi := lo + float64(1+rng.IntN(10))
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), lo, hi)
			c := float64(rng.IntN(21)-10) * 0.5
			if c != 0 {
				obj = obj.AddTerm(xs[j], c)
			}
		}
		m.SetObjective(obj)

		for i := range nRows {
			e := gb.NewExpr()
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
				m.AddConstraint("r"+strconv.Itoa(i), gb.Le(e, rhs))
			case 1:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(e, rhs))
			default:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(e, rhs))
			}
		}

		p := m.Compile()
		ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws)

		lpSIMD := gb.NewLPSolver(p, ws)
		resSIMD := lpSIMD.Solve()

		wsScalar := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(wsScalar)
		lpScalar := gb.NewLPSolver(p, wsScalar)
		lpScalar.DisableSIMD = true
		resScalar := lpScalar.Solve()

		if resSIMD.Status != resScalar.Status {
			t.Fatalf("[seed=%d] LP status mismatch: simd=%v scalar=%v", seed, resSIMD.Status, resScalar.Status)
		}
		if resSIMD.Status == gb.StatusOptimal {
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

		label := "fuzz_lp_" + strconv.FormatUint(seed, 10)
		if formalFuzzEnabled() {
			verifyFormalLPResult(t, label+"/simd", p, lpSIMD, resSIMD)
			verifyFormalLPResult(t, label+"/scalar", p, lpScalar, resScalar)
		}
		resPre := verifyFormalPresolvedLP(t, label+"/presolve", p, true)
		if resPre.Status != resSIMD.Status {
			t.Fatalf("[seed=%d] presolved LP status %v != %v", seed, resPre.Status, resSIMD.Status)
		}
		if resPre.Status == gb.StatusOptimal {
			if math.Abs(resPre.Objective-resSIMD.Objective) > 1e-6*max(1.0, math.Abs(resSIMD.Objective)) {
				t.Fatalf("[seed=%d] presolved LP obj %.8f != %.8f", seed, resPre.Objective, resSIMD.Objective)
			}
		}
	})
}

func buildAdversarialLPModel(seed uint64, regimeByte, sizeByte, senseConstByte uint8) *gb.Model {
	rng := rand.New(rand.NewPCG(seed^0x6a09e667f3bcc908, seed^0xbb67ae8584caa73b))
	sense := gb.Minimize
	if senseConstByte&1 == 1 {
		sense = gb.Maximize
	}
	objConsts := []float64{0, 12.5, -250.0, 1e9, -1e12}
	objConst := objConsts[int(senseConstByte>>1)%len(objConsts)]
	tag := fmt.Sprintf("adv_lp_r%d_%d", regimeByte%4, seed)

	if sizeByte >= 128 {
		if sizeByte&1 == 0 {
			// Free and semi-bounded non-dyadic LP exercising ExactBasisDuals in lp-check.
			m := gb.NewModel(tag+"_free_nd", sense)
			xf := m.AddContVar("xf", math.Inf(-1), math.Inf(1))
			xp := m.AddContVar("xp", 0, math.Inf(1))
			xn := m.AddContVar("xn", math.Inf(-1), 0)
			xb := m.AddContVar("xb", 0, float64(3+rng.IntN(6)))
			primes := []float64{3, 5, 7, 11, 13}
			a0 := primes[rng.IntN(len(primes))]
			a1 := primes[rng.IntN(len(primes))]
			cSign := 1.0
			if sense == gb.Maximize {
				cSign = -1.0
			}
			m.SetObjective(gb.NewExpr(
				gb.T(xf, cSign*a0),
				gb.T(xp, cSign*(a0+a1+2)),
				gb.T(xn, -cSign*(a1+1)),
				gb.T(xb, cSign*2),
			).AddConstant(objConst))
			m.AddConstraint("eq_free", gb.Eq(gb.NewExpr(gb.T(xf, a0), gb.T(xp, 2), gb.T(xn, 1)), float64(4+rng.IntN(7))))
			m.AddConstraint("ge_semi", gb.Ge(gb.NewExpr(gb.T(xf, 1), gb.T(xp, a1), gb.T(xn, -2), gb.T(xb, 3)), float64(7+rng.IntN(9))))
			m.AddConstraint("le_cap", gb.Le(gb.NewExpr(gb.T(xp, 2), gb.T(xb, 7)), float64(14+rng.IntN(10))))
			return m
		}
		// Unbounded LP (structural, slack-driven, or free-variable recession ray).
		m := gb.NewModel(tag+"_unb", sense)
		dirSign := -1.0
		if sense == gb.Maximize {
			dirSign = 1.0
		}
		switch (sizeByte >> 1) % 3 {
		case 0:
			x := m.AddContVar("x", 0, math.Inf(1))
			y := m.AddContVar("y", 0, math.Inf(1))
			m.SetObjective(gb.NewExpr(gb.T(x, dirSign*float64(2+rng.IntN(5))), gb.T(y, -dirSign)).AddConstant(objConst))
			m.AddConstraint("c0", gb.Le(gb.NewExpr(gb.T(x, 1), gb.T(y, -1)), float64(2+rng.IntN(6))))
		case 1:
			x0 := m.AddContVar("x0", 0, math.Inf(1))
			x1 := m.AddContVar("x1", 0, 10)
			m.SetObjective(gb.NewExpr(gb.T(x0, dirSign*1e-6), gb.T(x1, -dirSign*2)).AddConstant(objConst))
			m.AddConstraint("ge", gb.Ge(gb.NewExpr(gb.T(x1, 1), gb.T(x0, 50)), float64(1+rng.IntN(4))))
		default:
			xf := m.AddContVar("xf", math.Inf(-1), math.Inf(1))
			yp := m.AddContVar("yp", 0, math.Inf(1))
			m.SetObjective(gb.NewExpr(gb.T(xf, -dirSign*5), gb.T(yp, -dirSign)).AddConstant(objConst))
			m.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(xf, 3), gb.T(yp, 1)), float64(3+rng.IntN(8))))
		}
		return m
	}

	switch regimeByte % 4 {
	case 0:
		// Multi-singleton scaled chains targeting postsolveLP dual recovery.
		if seed == 0 && sizeByte == 0 {
			m := gb.NewModel(tag, gb.Maximize)
			x := m.AddContVar("x", 0, 1000)
			m.SetObjective(gb.NewExpr(gb.T(x, 1e18)))
			m.AddConstraint("slack_sng", gb.Le(gb.NewExpr(gb.T(x, 1e-8)), 9e-6))
			m.AddConstraint("tight_sng", gb.Le(gb.NewExpr(gb.T(x, 3e9)), 15e9))
			return m
		}
		if seed == 1 && sizeByte == 0 {
			m := gb.NewModel(tag, gb.Maximize)
			x := m.AddContVar("x", 0, 6)
			m.SetObjective(gb.NewExpr(gb.T(x, 1e6)))
			m.AddConstraint("slack_sng", gb.Le(gb.NewExpr(gb.T(x, 1e-20)), 9e-6))
			m.AddConstraint("tight_sng", gb.Le(gb.NewExpr(gb.T(x, 3.0)), 15.0))
			return m
		}
		m := gb.NewModel(tag, sense)
		nVars := 1 + int(sizeByte%3)
		xs := make([]gb.Var, nVars)
		obj := gb.NewExpr().AddConstant(objConst)
		cScales := []float64{1e6, 1e9, 1e12, 1e15, 1e18}
		for j := range nVars {
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), 0, 1000)
			cMag := cScales[rng.IntN(len(cScales))]
			if sense == gb.Minimize {
				cMag = -cMag
			}
			obj = obj.AddTerm(xs[j], cMag/float64(1+j))
		}
		m.SetObjective(obj)
		xStar := float64(2 + rng.IntN(9))
		tinyScales := []float64{1e-20, 1e-16, 1e-9, 1e-8}
		hugeScales := []float64{3.0, 7.0, 3e8, 3e9}
		nSng := 2 + rng.IntN(3)
		tightIdx := rng.IntN(nSng)
		for k := range nSng {
			if k == tightIdx {
				a := hugeScales[rng.IntN(len(hugeScales))]
				m.AddConstraint("sng_"+strconv.Itoa(k), gb.Le(gb.NewExpr(gb.T(xs[0], a)), a*xStar))
			} else {
				a := tinyScales[rng.IntN(len(tinyScales))]
				slackBound := xStar + float64(100+rng.IntN(800))
				rhs := max(9e-6, a*slackBound)
				m.AddConstraint("sng_"+strconv.Itoa(k), gb.Le(gb.NewExpr(gb.T(xs[0], a)), rhs))
			}
		}
		if nVars > 1 {
			e := gb.NewExpr(gb.T(xs[0], 1.0), gb.T(xs[1], 2.0))
			m.AddConstraint("couple", gb.Le(e, xStar+20.0))
		}
		return m

	case 1:
		// Box-bounded Hilbert LP: H[i,j] = 1/(i+j+1).
		n := 3 + int(sizeByte%6)
		m := gb.NewModel(tag, sense)
		xs := make([]gb.Var, n)
		xRef := make([]float64, n)
		obj := gb.NewExpr().AddConstant(objConst)
		for j := range n {
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), 0, 10)
			xRef[j] = float64(1 + rng.IntN(5))
			obj = obj.AddTerm(xs[j], float64(rng.IntN(19)-9)+0.5)
		}
		m.SetObjective(obj)
		for i := range n {
			e := gb.NewExpr()
			rhs := 0.0
			for j := range n {
				hij := 1.0 / float64(i+j+1)
				e = e.AddTerm(xs[j], hij)
				rhs += hij * xRef[j]
			}
			switch (i + int(seed)) % 3 {
			case 0:
				m.AddConstraint("h"+strconv.Itoa(i), gb.Le(e, rhs+0.25))
			case 1:
				m.AddConstraint("h"+strconv.Itoa(i), gb.Ge(e, rhs-0.25))
			default:
				m.AddConstraint("h"+strconv.Itoa(i), gb.Eq(e, rhs))
			}
		}
		return m

	case 2:
		// Geometrically scaled / Vandermonde box-bounded LP.
		n := 3 + int(sizeByte%5)
		m := gb.NewModel(tag, sense)
		xs := make([]gb.Var, n)
		xRef := make([]float64, n)
		obj := gb.NewExpr().AddConstant(objConst)
		for j := range n {
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), -5, 5)
			xRef[j] = float64(rng.IntN(5) - 2)
			sc := math.Ldexp(1.0, (j-n/2)*4)
			obj = obj.AddTerm(xs[j], sc*float64(1+rng.IntN(7)))
		}
		m.SetObjective(obj)
		nRows := 2 + int(sizeByte%4)
		for i := range nRows {
			base := 1.5 + 0.5*float64(i)
			rowScale := math.Ldexp(1.0, (i-nRows/2)*6)
			e := gb.NewExpr()
			rhs := 0.0
			pVal := 1.0
			for j := range n {
				a := rowScale * pVal
				pVal *= base
				e = e.AddTerm(xs[j], a)
				rhs += a * xRef[j]
			}
			if i%2 == 0 {
				m.AddConstraint("v"+strconv.Itoa(i), gb.Le(e, rhs+math.Abs(rowScale)))
			} else {
				m.AddConstraint("v"+strconv.Itoa(i), gb.Ge(e, rhs-math.Abs(rowScale)))
			}
		}
		return m

	default:
		// Infeasible / near-infeasible Farkas LP systems on bounded boxes.
		n := 2 + int(sizeByte%5)
		m := gb.NewModel(tag, sense)
		xs := make([]gb.Var, n)
		obj := gb.NewExpr().AddConstant(objConst)
		for j := range n {
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), 0, 10)
			obj = obj.AddTerm(xs[j], float64(1+j))
		}
		m.SetObjective(obj)
		gaps := []float64{1e-4, 1e-3, 1e-2, 0.5, 2.0}
		gap := gaps[int(seed)%len(gaps)]
		e1 := gb.NewExpr()
		e2 := gb.NewExpr()
		for j := range n {
			a := float64(1 + rng.IntN(9))
			e1 = e1.AddTerm(xs[j], a)
			e2 = e2.AddTerm(xs[j], a)
		}
		rhs := float64(3 + rng.IntN(10))
		m.AddConstraint("ub_row", gb.Le(e1, rhs))
		m.AddConstraint("lb_row", gb.Ge(e2, rhs+gap))
		return m
	}
}

func FuzzAdversarialLPFormal(f *testing.F) {
	f.Add(uint64(0), uint8(0), uint8(0), uint8(1))
	f.Add(uint64(1), uint8(0), uint8(0), uint8(1))
	f.Add(uint64(0), uint8(1), uint8(2), uint8(0))
	for r := range uint8(4) {
		for s := range uint64(3) {
			f.Add(s+2, r, uint8(s+1), r^uint8(s))
		}
	}
	f.Add(uint64(10), uint8(1), uint8(128), uint8(0))
	f.Add(uint64(11), uint8(2), uint8(128), uint8(3))
	f.Add(uint64(12), uint8(0), uint8(129), uint8(0))
	f.Add(uint64(13), uint8(3), uint8(131), uint8(1))
	f.Add(uint64(14), uint8(1), uint8(133), uint8(2))

	f.Fuzz(func(t *testing.T, seed uint64, regimeByte, sizeByte, senseConstByte uint8) {
		mdl := buildAdversarialLPModel(seed, regimeByte, sizeByte, senseConstByte)
		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked failed: %v", err)
		}
		label := fmt.Sprintf("adv_lp_r%d_s%d", regimeByte%4, seed)

		ws1 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws1)
		lpSIMD := gb.NewLPSolver(p, ws1)
		resSIMD := lpSIMD.Solve()

		ws2 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws2)
		lpScalar := gb.NewLPSolver(p, ws2)
		lpScalar.DisableSIMD = true
		resScalar := lpScalar.Solve()

		if resSIMD.Status != resScalar.Status {
			t.Fatalf("[%s] SIMD status %v != Scalar status %v", label, resSIMD.Status, resScalar.Status)
		}
		if formalFuzzEnabled() {
			verifyFormalLPResult(t, label+"/simd", p, lpSIMD, resSIMD)
			verifyFormalLPResult(t, label+"/scalar", p, lpScalar, resScalar)
		}

		resPre := verifyFormalPresolvedLP(t, label+"/presolve", p, true)
		if resPre.Status != resSIMD.Status {
			t.Fatalf("[%s] SolveCompiled presolve status %v != LP status %v", label, resPre.Status, resSIMD.Status)
		}
	})
}

func runAdversarialMIPActivityRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed^0x51a7, uint64(paramA)*31+uint64(paramB)))
	label := fmt.Sprintf("adv_mip_act_%d_%d", paramA, seed)

	var mdl *gb.Model
	switch paramA {
	case 0:
		// Exact #42 Bug #1 reproducer: 1e-9*x <= 0, y - 1e9*x <= 0.
		mdl = gb.NewModel(label, gb.Maximize)
		x := mdl.AddBinaryVar("x")
		y := mdl.AddContVar("y", 0, 1e9)
		mdl.SetObjective(gb.NewExpr(gb.T(y, 1.0)))
		mdl.AddConstraint("tiny", gb.Le(gb.NewExpr(gb.T(x, 1e-9)), 0))
		mdl.AddConstraint("amp", gb.Le(gb.NewExpr(gb.T(y, 1.0), gb.T(x, -1e9)), 0))
	case 1:
		// Pure-integer #42 Bug #1 variant: 1e-10*x + 1e-10*y <= 0, x,y in [0,5].
		mdl = gb.NewModel(label, gb.Maximize)
		x := mdl.AddIntVar("x", 0, 5)
		y := mdl.AddIntVar("y", 0, 5)
		mdl.SetObjective(gb.NewExpr(gb.T(x, 1.0), gb.T(y, 1.0)))
		mdl.AddConstraint("tiny_int", gb.Le(gb.NewExpr(gb.T(x, 1e-10), gb.T(y, 1e-10)), 0))
	default:
		sense := gb.Maximize
		if paramB&1 == 1 {
			sense = gb.Minimize
		}
		mdl = gb.NewModel(label, sense)
		nVars := 2 + int(paramB%4)
		xs := make([]gb.Var, nVars)
		obj := gb.NewExpr()
		ubChoices := []float64{1, 5, 1e6, 1e9}
		for j := range nVars {
			switch j % 3 {
			case 0:
				xs[j] = mdl.AddBinaryVar("x" + strconv.Itoa(j))
			case 1:
				xs[j] = mdl.AddIntVar("x"+strconv.Itoa(j), 0, 5)
			default:
				xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), 0, ubChoices[rng.IntN(len(ubChoices))])
			}
			obj = obj.AddTerm(xs[j], float64(1+rng.IntN(5)))
		}
		mdl.SetObjective(obj)
		scales := []float64{math.Ldexp(1.0, -1000), 1e-12, 1e-10, 1e-9, 1e-6, 1.0, 1e6, 2e11, 1e12}
		rhsChoices := []float64{0, 1e-12, -1e-12, 1e-9, math.Ldexp(1.0, 52)}
		for r := range 2 {
			e := gb.NewExpr()
			for j := range nVars {
				s := scales[rng.IntN(len(scales))]
				if rng.IntN(3) == 0 {
					s = -s
				}
				e = e.AddTerm(xs[j], s)
			}
			rhs := rhsChoices[rng.IntN(len(rhsChoices))]
			if r == 0 {
				mdl.AddConstraint("r0", gb.Le(e, rhs))
			} else {
				mdl.AddConstraint("r1", gb.Ge(e, -math.Abs(rhs)))
			}
		}
	}

	p, err := mdl.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked: %v", label, err)
	}
	items, preRes := verifyFormalPresolveRules(t, p)

	if formalFuzzEnabled() && p.NumConstraints() == 1 && p.RowLen(0) >= 2 &&
		!preRes.Infeasible && preRes.TightenedBounds == 0 && preRes.StrengthenedRows == 0 && preRes.RemovedRows > 0 {
		for _, item := range items {
			var hdr struct {
				Subcommand string `json:"subcommand"`
				Op         string `json:"op"`
			}
			if json.Unmarshal(item, &hdr) != nil || hdr.Subcommand != "activity" || hdr.Op != "summary" {
				continue
			}
			resJSON := mustFormalRulesCheck(t, item)
			var parsed struct {
				Deficit *fuzzRatJSON `json:"deficit"`
			}
			if json.Unmarshal(resJSON, &parsed) == nil && parsed.Deficit != nil && p.RowSense(0) == gb.LessEqual {
				num, ok := new(big.Int).SetString(parsed.Deficit.Num, 10)
				if ok && num.Sign() < 0 {
					t.Fatalf("[%s] presolve removed non-redundant LE row with exact deficit %s/%s < 0",
						label, parsed.Deficit.Num, parsed.Deficit.Den)
				}
			}
		}
	}

	resPre := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(true), gb.WithMaxNodes(800), gb.WithTimeLimit(800*time.Millisecond))
	resNoPre := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(false), gb.WithMaxNodes(800), gb.WithTimeLimit(800*time.Millisecond))
	verifyMILPInvariants(t, label+"/pre", p, resPre)
	verifyMILPInvariants(t, label+"/nopre", p, resNoPre)

	if paramA <= 1 {
		if resPre.Status != gb.StatusOptimal || math.Abs(resPre.Objective) > 1e-6 {
			t.Fatalf("[%s] expected StatusOptimal with obj=0, got status=%v obj=%.8g",
				label, resPre.Status, resPre.Objective)
		}
	}
	if resPre.Status == gb.StatusInfeasible && resNoPre.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resNoPre.Values) {
		t.Fatalf("[%s] presolve falsely declared exact-feasible MIP infeasible", label)
	}
	if resPre.Status == gb.StatusOptimal && resNoPre.Status == gb.StatusOptimal {
		tol := 1e-5 * max(1.0, max(math.Abs(resPre.Objective), math.Abs(resNoPre.Objective)))
		preBetter := (p.Sense() == gb.Minimize && resPre.Objective < resNoPre.Objective-tol) ||
			(p.Sense() == gb.Maximize && resPre.Objective > resNoPre.Objective+tol)
		noPreBetter := (p.Sense() == gb.Minimize && resNoPre.Objective < resPre.Objective-tol) ||
			(p.Sense() == gb.Maximize && resNoPre.Objective > resPre.Objective+tol)
		if (preBetter && isExactRatMIPFeasible(p, resPre.Values)) ||
			(noPreBetter && isExactRatMIPFeasible(p, resNoPre.Values)) {
			t.Fatalf("[%s] presolve objective %.8g != no-presolve objective %.8g on exact-feasible witness",
				label, resPre.Objective, resNoPre.Objective)
		}
	}
	if formalFuzzEnabled() {
		w := [3]int{1, 2, 4}[int(seed+uint64(paramB))%3]
		resFull := mustSolveCompiled(t, p,
			gb.WithPureGo(),
			gb.WithPresolve(true),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
			gb.WithWorkers(w),
			gb.WithMaxNodes(800),
			gb.WithTimeLimit(800*time.Millisecond),
		)
		resVIPR, trNoPre := verifyFormalVIPRTrace(
			t,
			label+"/vipr_nopre",
			p,
			800,
			800*time.Millisecond,
			gb.WithPresolve(false),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
		)
		assertFullFeatureVsVIPR(t, label, p, resFull, resVIPR, trNoPre, 1e-6, 1e-6)
		if resPre.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resPre.Values) && trNoPre.HasCertificate() && trNoPre.TargetLB() != nil {
			if evalMinRat, linScale, okRat := exactEvalMinObjRat(p, resPre.Values); okRat {
				tolRat := new(big.Rat).SetFloat64(1e-5 * max(1.0, linScale))
				if tolRat != nil && trNoPre.TargetLB().Cmp(new(big.Rat).Add(evalMinRat, tolRat)) > 0 {
					t.Fatalf("[%s] presolved solution minObj %s violates VIPR lower bound %s",
						label, evalMinRat.RatString(), trNoPre.TargetLB().RatString())
				}
			}
		}
	}
}

func runAdversarialMIPDynamicOrbitRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	label := fmt.Sprintf("adv_mip_orbit_%d_%d", paramA, seed)
	gCount := 3
	fCoeff := 0.25
	rhsVal := math.Ldexp(1.0, 53)
	if paramA == 0 && paramB%2 == 1 {
		fCoeff = 0.75
	} else if paramA > 0 {
		gCount = 2 + int(paramA%3)
		rhsChoices := []float64{1e6, math.Ldexp(1.0, 51), math.Ldexp(1.0, 52), math.Ldexp(1.0, 53), 1e15}
		rhsVal = rhsChoices[int(paramB)%len(rhsChoices)]
		step := max(1.0, math.Abs(rhsVal-math.Nextafter(rhsVal, 0)))
		fMults := []float64{0, 0.25, 0.5, 0.75, 1.5}
		fCoeff = fMults[int(seed+uint64(paramB))%len(fMults)] * step
	}

	m := gb.NewModel(label, gb.Minimize)
	fVar := m.AddBinaryVar("f")
	vVar := m.AddContVar("v", 0, 1)
	ys := make([]gb.Var, gCount)
	xs := make([]gb.Var, gCount)
	part := gb.NewExpr()
	for g := range gCount {
		ys[g] = m.AddBinaryVar("y" + strconv.Itoa(g))
		xs[g] = m.AddIntVar("x"+strconv.Itoa(g), 0, rhsVal)
		part = part.AddTerm(ys[g], 1.0)
	}
	m.SetObjective(gb.NewExpr(gb.T(fVar, 10.0), gb.T(vVar, 1.0)))
	m.AddConstraint("force_f", gb.Ge(gb.NewExpr(gb.T(fVar, 2.0)), 1.0))
	m.AddConstraint("part_y", gb.Eq(part, 1.0))
	if paramB&4 == 0 {
		for g := range gCount {
			capRow := gb.NewExpr(gb.T(xs[g], 1.0))
			if g < gCount-1 && fCoeff != 0 {
				capRow = capRow.AddTerm(fVar, fCoeff)
			}
			m.AddConstraint("cap_"+strconv.Itoa(g), gb.Le(capRow, rhsVal))
		}
		for g := range gCount {
			m.AddConstraint("lb_"+strconv.Itoa(g), gb.Ge(gb.NewExpr(gb.T(xs[g], 1.0), gb.T(ys[g], -rhsVal)), 0.0))
			m.AddConstraint("v_"+strconv.Itoa(g), gb.Le(gb.NewExpr(gb.T(ys[g], 1.0), gb.T(vVar, -1.0)), 0.0))
		}
	} else {
		for g := range gCount {
			capRow := gb.NewExpr(gb.T(xs[g], 1.0))
			if g < gCount-1 && fCoeff != 0 {
				capRow = capRow.AddTerm(fVar, fCoeff)
			}
			m.AddConstraint("cap_"+strconv.Itoa(g), gb.Le(capRow, rhsVal))
			m.AddConstraint("lb_"+strconv.Itoa(g), gb.Ge(gb.NewExpr(gb.T(xs[g], 1.0), gb.T(ys[g], -rhsVal)), 0.0))
			m.AddConstraint("v_"+strconv.Itoa(g), gb.Le(gb.NewExpr(gb.T(ys[g], 1.0), gb.T(vVar, -1.0)), 0.0))
		}
	}

	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked: %v", label, err)
	}
	_, _ = verifyFormalPresolveRules(t, p)

	lastYCol := 2 + 2*(gCount-1)
	for _, pre := range []bool{true, false} {
		res := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(pre), gb.WithMaxNodes(500), gb.WithTimeLimit(time.Second))
		if res.Status == gb.StatusInfeasible || res.Status == gb.StatusUnbounded {
			t.Fatalf("[%s/presolve=%v] false %v on feasible dynamic-orbit MIP", label, pre, res.Status)
		}
		if res.Status == gb.StatusOptimal {
			if math.Abs(res.Objective-11.0) > 1e-5 {
				t.Fatalf("[%s/presolve=%v] objective=%.8g, want 11.0", label, pre, res.Objective)
			}
			if fCoeff > 1e-5 && res.Values[lastYCol] < 0.5 {
				t.Fatalf("[%s/presolve=%v] selected infeasible orbit group: vals=%v", label, pre, res.Values)
			}
			verifyMILPInvariants(t, label, p, res)
		}
	}

	if formalFuzzEnabled() {
		w := [3]int{1, 2, 4}[int(seed+uint64(paramB))%3]
		resFull, resVIPR, tr := runFullFeatureVsVIPRCheck(t, label+"/full_vs_vipr", p, w, 500, time.Second, 1e-6, 1e-6)
		if resFull.Status == gb.StatusInfeasible || resFull.Status == gb.StatusUnbounded {
			t.Fatalf("[%s] full-feature solve returned false %v on feasible dynamic-orbit MIP", label, resFull.Status)
		}
		if resVIPR.Status == gb.StatusInfeasible || resVIPR.Status == gb.StatusUnbounded {
			t.Fatalf("[%s] VIPR solve returned false %v on feasible dynamic-orbit MIP", label, resVIPR.Status)
		}
		if resVIPR.Status == gb.StatusOptimal && math.Abs(resVIPR.Objective-11.0) > 1e-6 {
			t.Fatalf("[%s] VIPR objective=%.8g, want 11.0", label, resVIPR.Objective)
		}
		requireOptimal := (paramA == 0 && paramB%2 == 1) || rhsVal <= 1e9
		if requireOptimal {
			if resVIPR.Status != gb.StatusOptimal {
				t.Fatalf("[%s] expected StatusOptimal on well-separated control, got %v", label, resVIPR.Status)
			}
			if !tr.HasCertificate() || tr.Mode() != "optimal" {
				t.Fatalf("[%s] expected optimal VIPR certificate, got hasCert=%v mode=%q",
					label, tr.HasCertificate(), tr.Mode())
			}
		}
	}
}

func runAdversarialMIPGCDAndBinRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed^0x9e3779b9, uint64(paramA)*17+uint64(paramB)))
	label := fmt.Sprintf("adv_mip_gcdbin_%d_%d", paramA, seed)

	var mdl *gb.Model
	if paramA%2 == 0 {
		// Integer equalities with coefficients near 2^52..2^60 and shared GCD g.
		mdl = gb.NewModel(label, gb.Minimize)
		gs := []int64{2, 3, 7, 1 << 20}
		g := gs[int(paramB)%len(gs)]
		x0 := mdl.AddIntVar("x0", -10, 10)
		x1 := mdl.AddIntVar("x1", -10, 10)
		x2 := mdl.AddIntVar("x2", 0, 5)
		mdl.SetObjective(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 2), gb.T(x2, 3)))

		baseMult := max(int64(2), (int64(1)<<(50+int(seed%8)))/g)
		c0 := float64(baseMult * g)
		c1 := float64(-(baseMult + 1) * g)
		c2 := float64(3 * g)
		offsets := []float64{0, 1.0, 0.5, 1e-7, 1e-6}
		off := offsets[int(seed+uint64(paramB))%len(offsets)]
		rhs := float64(2*g) + off
		mdl.AddConstraint("gcd_eq", gb.Eq(gb.NewExpr(gb.T(x0, c0), gb.T(x1, c1), gb.T(x2, c2)), rhs))
	} else {
		// Savelsbergh binary strengthening with large coefficients and tight surplus.
		mdl = gb.NewModel(label, gb.Minimize)
		nBin := 3 + int(paramB%3)
		bs := make([]gb.Var, nBin)
		obj := gb.NewExpr()
		row := gb.NewExpr()
		scales := []float64{1.0, 1e6, 1e10, 1e13, 1e14}
		sc := scales[int(seed)%len(scales)]
		a0 := 5.0 * sc
		bs[0] = mdl.AddBinaryVar("b0")
		obj = obj.AddTerm(bs[0], 3.0)
		row = row.AddTerm(bs[0], a0)
		sumOther := 0.0
		for j := 1; j < nBin; j++ {
			bs[j] = mdl.AddBinaryVar("b" + strconv.Itoa(j))
			aj := float64(2+rng.IntN(3)) * sc
			sumOther += aj
			obj = obj.AddTerm(bs[j], 1.0)
			row = row.AddTerm(bs[j], aj)
		}
		rhs := sumOther + 2.0*sc
		mdl.SetObjective(obj)
		mdl.AddConstraint("bin_str", gb.Le(row, rhs))
	}

	p, err := mdl.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked: %v", label, err)
	}
	_, _ = verifyFormalPresolveRules(t, p)
	if p.NumVars() <= 128 && p.NumConstraints() <= 128 {
		w := [3]int{1, 2, 4}[int(seed+uint64(paramB))%3]
		_, _, _ = runFullFeatureVsVIPRCheck(t, label, p, w, 400, 800*time.Millisecond, 1e-6, 1e-6)
	}
}

func runAdversarialMIPCMIRUnderflowRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	label := fmt.Sprintf("adv_mip_cmir_%d_%d", paramA, seed)

	a0 := 1e30
	a1 := -1e-300
	f0 := 0.5
	if paramA > 0 {
		a0s := []float64{1e10, 1e20, 1e30}
		a1s := []float64{-1e-305, -1e-300, -1e-250, -1e-12}
		f0s := []float64{0.5, 1e-4, 1.0 - 1e-4, 0.25}
		a0 = a0s[int(seed)%len(a0s)]
		a1 = a1s[int(paramA)%len(a1s)]
		f0 = f0s[int(paramB)%len(f0s)]
	}
	rhs := f0 * a0

	m := gb.NewModel(label, gb.Maximize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddIntVar("x1", 0, math.Inf(1))
	m.SetObjective(gb.NewExpr(gb.T(x0, 1.0), gb.T(x1, -1e-20)))
	m.AddConstraint("mir_row", gb.Le(gb.NewExpr(gb.T(x0, a0), gb.T(x1, a1)), rhs))

	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked: %v", label, err)
	}
	if !formalFuzzEnabled() {
		return
	}

	items, err := gb.ExportSeparatedCutsItems(p, [][]float64{{0, 0}})
	if err != nil {
		t.Fatalf("[%s] ExportSeparatedCutsItems: %v", label, err)
	}

	a0Rat := new(big.Rat).SetFloat64(a0)
	rhsRat := new(big.Rat).SetFloat64(rhs)
	absA1Rat := new(big.Rat).SetFloat64(-a1)
	need := new(big.Rat).Sub(a0Rat, rhsRat)
	x1Int := big.NewInt(0)
	if need.Sign() > 0 && absA1Rat != nil && absA1Rat.Sign() > 0 {
		x1Int = ratCeilBigInt(new(big.Rat).Quo(need, absA1Rat))
	}
	extraSol := []fuzzRatJSON{
		{Num: "1", Den: "1"},
		{Num: x1Int.String(), Den: "1"},
	}

	for _, item := range items {
		var doc map[string]jsontext.Value
		if err := json.Unmarshal(item, &doc); err != nil {
			t.Fatalf("[%s] unmarshal cut item: %v", label, err)
		}
		var sols [][]fuzzRatJSON
		if rawSols, ok := doc["solutions"]; ok {
			_ = json.Unmarshal(rawSols, &sols)
		}
		sols = append(sols, extraSol)
		if encSols, errS := json.Marshal(sols); errS == nil {
			doc["solutions"] = encSols
		}
		aug, errM := json.Marshal(doc)
		if errM != nil {
			t.Fatalf("[%s] marshal augmented cut item: %v", label, errM)
		}
		mustFormalCutsCheck(t, aug)
	}
}

func runAdversarialMIPVUBAndCutsRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed^0x4cf5ad43, uint64(paramA)*29+uint64(paramB)))
	label := fmt.Sprintf("adv_mip_vub_%d_%d", paramA, seed)
	workers := [3]int{1, 2, 4}[int(seed+uint64(paramB))%3]

	if paramA >= 4 {
		switch paramA % 4 {
		case 0:
			// Semi-bounded (l, S) lot-sizing + knapsack cover + multi-row agg-cMIR.
			m := gb.NewModel(label+"_track2c", gb.Minimize)
			T := 3 + int(paramB%2)
			ys := make([]gb.Var, T)
			ps := make([]gb.Var, T)
			ss := make([]gb.Var, T)
			obj := gb.NewExpr()
			for tau := range T {
				ys[tau] = m.AddBinaryVar("y" + strconv.Itoa(tau))
				ps[tau] = m.AddContVar("p"+strconv.Itoa(tau), 0, math.Inf(1))
				ss[tau] = m.AddContVar("s"+strconv.Itoa(tau), 0, math.Inf(1))
				capM := float64(35 + 10*(tau%2))
				m.AddConstraint("vub"+strconv.Itoa(tau), gb.Le(gb.NewExpr(gb.T(ps[tau], 1), gb.T(ys[tau], -capM)), 0))
				dem := float64(8 + rng.IntN(10))
				if tau == 0 {
					m.AddConstraint("bal0", gb.Eq(gb.NewExpr(gb.T(ps[0], 1), gb.T(ss[0], -1)), dem))
				} else {
					m.AddConstraint("bal"+strconv.Itoa(tau), gb.Eq(gb.NewExpr(gb.T(ss[tau-1], 1), gb.T(ps[tau], 1), gb.T(ss[tau], -1)), dem))
				}
				obj = obj.AddTerm(ys[tau], float64(12+rng.IntN(8))).AddTerm(ss[tau], 0.5)
			}
			bs := make([]gb.Var, 4)
			knap := gb.NewExpr()
			for k := range 4 {
				bs[k] = m.AddBinaryVar("b" + strconv.Itoa(k))
				w := float64(4 + (k % 3))
				knap = knap.AddTerm(bs[k], w)
				obj = obj.AddTerm(bs[k], -float64(3+k))
			}
			m.AddConstraint("knap", gb.Le(knap, 10))
			ix0 := m.AddIntVar("ix0", 0, 5)
			ix1 := m.AddIntVar("ix1", 0, 5)
			sc := m.AddContVar("sc", 0, math.Inf(1))
			m.AddConstraint("agg_eq", gb.Eq(gb.NewExpr(gb.T(ix0, 2), gb.T(sc, 1)), 3))
			m.AddConstraint("agg_le", gb.Le(gb.NewExpr(gb.T(sc, -1), gb.T(ix1, 2)), 2))
			obj = obj.AddTerm(ix0, -4).AddTerm(ix1, -3)
			m.SetObjective(obj)

			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked: %v", label, err)
			}
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, label+"/rootLP", p)
			resFull, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, 500, time.Second, 1e-6, 1e-6)
			var sols [][]float64
			if resFull.Status == gb.StatusOptimal && len(resFull.Values) == p.NumVars() {
				sols = append(sols, resFull.Values)
			}
			verifyFormalSeparatedCuts(t, label+"/cuts_universal", p, nil)
			if len(sols) > 0 {
				verifyFormalSeparatedCuts(t, label+"/cuts_witness", p, sols)
			}
			return

		case 1:
			// Complemented + continuous-relaxed binary cover with fractional integer bounds.
			sense := gb.Maximize
			if paramB&1 == 1 {
				sense = gb.Minimize
			}
			m := gb.NewModel(label+"_comp_cov", sense)
			x := make([]gb.Var, 5)
			for i := range 5 {
				x[i] = m.AddBinaryVar("x" + strconv.Itoa(i))
			}
			z := m.AddIntVar("z", 0.3, 4.7)
			c := m.AddContVar("c", 1.0, math.Inf(1))
			cSign := 1.0
			if sense == gb.Minimize {
				cSign = -1.0
			}
			m.SetObjective(gb.NewExpr(
				gb.T(x[0], cSign*3), gb.T(x[1], cSign*3), gb.T(x[2], cSign*4),
				gb.T(x[3], -cSign*2), gb.T(x[4], cSign*3), gb.T(z, cSign*2), gb.T(c, -cSign*0.5),
			))
			m.AddConstraint("cov", gb.Le(gb.NewExpr(
				gb.T(x[0], 4), gb.T(x[1], 4), gb.T(x[2], 6),
				gb.T(x[3], -3), gb.T(x[4], 5), gb.T(c, 2),
			), float64(8+rng.IntN(4))))
			m.AddConstraint("mir", gb.Le(gb.NewExpr(gb.T(z, 2), gb.T(x[0], 2)), 5.4))

			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked: %v", label, err)
			}
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, label+"/rootLP", p)
			resFull, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, 500, time.Second, 1e-6, 1e-6)
			var sols [][]float64
			if resFull.Status == gb.StatusOptimal && len(resFull.Values) == p.NumVars() {
				sols = append(sols, resFull.Values)
			}
			verifyFormalSeparatedCuts(t, label+"/cuts", p, sols)
			return

		case 2:
			// Meyer unbounded MILP with LCM ray scaling vs Diophantine-infeasible cliff.
			m := gb.NewModel(label+"_meyer", gb.Minimize)
			x := m.AddIntVar("x", 1, math.Inf(1))
			y := m.AddIntVar("y", 0, math.Inf(1))
			c := m.AddContVar("c", 0, math.Inf(1))
			m.SetObjective(gb.NewExpr(gb.T(x, -3), gb.T(y, 1)))
			if paramB&1 == 0 {
				pCoeff := float64(3 + 2*(rng.IntN(2)))
				qCoeff := float64(2)
				m.AddConstraint("dio", gb.Eq(gb.NewExpr(gb.T(x, pCoeff), gb.T(y, -qCoeff)), 0))
				m.AddConstraint("cont", gb.Eq(gb.NewExpr(gb.T(c, 3), gb.T(x, -1)), 0))
			} else {
				m.AddConstraint("dio_infeas", gb.Eq(gb.NewExpr(gb.T(x, 2), gb.T(y, -2)), 5e-7))
				m.AddConstraint("cont", gb.Le(gb.NewExpr(gb.T(c, 1), gb.T(x, -1)), 0))
			}
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked: %v", label, err)
			}
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, label+"/rootLP", p)
			resFull, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, 400, 800*time.Millisecond, 1e-6, 1e-6)
			if paramB&1 == 0 && resFull.Status != gb.StatusUnbounded {
				t.Fatalf("[%s] expected StatusUnbounded on Meyer MILP, got %v", label, resFull.Status)
			}
			if paramB&1 == 1 && resFull.Status != gb.StatusInfeasible {
				t.Fatalf("[%s] expected StatusInfeasible on 2x-2y=5e-7 cliff, got %v", label, resFull.Status)
			}
			return

		default:
			// Pure-integer GCD row division + semi-bounded cuts.
			m := gb.NewModel(label+"_gcd_div", gb.Minimize)
			g := float64(2 + int(paramB%5))
			x0 := m.AddIntVar("x0", 0, 12)
			x1 := m.AddIntVar("x1", 0, 12)
			s := m.AddContVar("s", 0, math.Inf(1))
			m.SetObjective(gb.NewExpr(gb.T(x0, -3), gb.T(x1, -4), gb.T(s, 1e-3)))
			m.AddConstraint("gcd_le", gb.Le(gb.NewExpr(gb.T(x0, 2*g), gb.T(x1, 3*g)), 17*g+0.5))
			m.AddConstraint("gcd_ge", gb.Ge(gb.NewExpr(gb.T(x0, g), gb.T(x1, 2*g)), 3*g+0.25))
			m.AddConstraint("semi", gb.Le(gb.NewExpr(gb.T(x0, 1), gb.T(s, -1)), 4))
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("[%s] CompileChecked: %v", label, err)
			}
			_, _ = verifyFormalPresolveRules(t, p)
			verifyFormalRootLP(t, label+"/rootLP", p)
			resFull, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, 400, 800*time.Millisecond, 1e-6, 1e-6)
			if resFull.Status == gb.StatusOptimal && len(resFull.Values) == p.NumVars() {
				verifyFormalSeparatedCuts(t, label+"/cuts", p, [][]float64{resFull.Values})
			}
			return
		}
	}

	aks := []float64{3.0, 7.0, 9.0, 11.0}
	caps := []float64{1.0, 2.0, 5.0, 10.0}
	ak := aks[int(paramA)%len(aks)]
	capVal := caps[int(paramB)%len(caps)]
	uBound := 1.0
	if seed%2 == 1 {
		uBound = 0.25
	}

	m := gb.NewModel(label, gb.Maximize)
	x0 := m.AddContVar("x0", 0, uBound)
	x1 := m.AddContVar("x1", 0, uBound)
	y := m.AddBinaryVar("y")
	z := m.AddIntVar("z", 0, 5)
	m.SetObjective(gb.NewExpr(gb.T(x0, 5.0), gb.T(x1, 4.0), gb.T(y, -2.0), gb.T(z, 1.5)))
	m.AddConstraint("vub", gb.Le(gb.NewExpr(gb.T(x0, ak), gb.T(x1, ak), gb.T(y, -capVal)), 0.0))
	m.AddConstraint("knap", gb.Le(gb.NewExpr(gb.T(z, 3.0), gb.T(y, 2.0), gb.T(x0, 1.0)), 6.5))

	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked: %v", label, err)
	}
	res, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, 500, time.Second, 1e-6, 1e-6)
	if !formalFuzzEnabled() {
		return
	}
	sols := [][]float64{{0, 0, 0, 0}}
	if res.Status == gb.StatusOptimal && len(res.Values) == p.NumVars() {
		sols = append(sols, res.Values)
	}
	verifyFormalSeparatedCuts(t, label+"/cuts", p, sols)
}

func runAdversarialMIPObjConstRegime(t *testing.T, seed uint64, paramA, paramB uint8) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed^0x27d4eb2f, uint64(paramA)*23+uint64(paramB)))
	label := fmt.Sprintf("adv_mip_objconst_%d_%d", paramA, seed)

	var sense gb.Sense
	var objConst, relTol, absTol float64
	buildPair := func(cConst float64) *gb.Model {
		switch paramA {
		case 0:
			sense = gb.Minimize
			relTol, absTol = 1e-6, 1e-6
			m := gb.NewModel(label, sense)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddBinaryVar("y")
			m.SetObjective(gb.NewExpr(gb.T(x, 4.0), gb.T(y, 10.0)).AddConstant(cConst))
			m.AddConstraint("c0", gb.Ge(gb.NewExpr(gb.T(x, 3.0), gb.T(y, 10.0)), 5.0))
			return m
		case 1:
			sense = gb.Minimize
			relTol, absTol = 0, 0
			m := gb.NewModel(label, sense)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddBinaryVar("y")
			m.SetObjective(gb.NewExpr(gb.T(x, 3.0), gb.T(y, 7.0)).AddConstant(cConst))
			m.AddConstraint("c0", gb.Ge(gb.NewExpr(gb.T(x, 3.0), gb.T(y, 10.0)), 5.0))
			return m
		case 2:
			sense = gb.Maximize
			relTol, absTol = 0, 0
			m := gb.NewModel(label, sense)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddBinaryVar("y")
			m.SetObjective(gb.NewExpr(gb.T(x, -3.0), gb.T(y, -7.0)).AddConstant(cConst))
			m.AddConstraint("c0", gb.Ge(gb.NewExpr(gb.T(x, 3.0), gb.T(y, 10.0)), 5.0))
			return m
		default:
			sense = gb.Minimize
			if paramB&1 == 1 {
				sense = gb.Maximize
			}
			relChoices := []float64{0, 1e-9, 1e-6}
			absChoices := []float64{0, 1e-9}
			relTol = relChoices[int(paramB>>1)%len(relChoices)]
			absTol = absChoices[int(paramB>>3)%len(absChoices)]
			rLocal := rand.New(rand.NewPCG(seed^0x13579bdf, uint64(paramA)))
			nVars := 2 + rLocal.IntN(4)
			m := gb.NewModel(label, sense)
			xs := make([]gb.Var, nVars)
			obj := gb.NewExpr().AddConstant(cConst)
			row := gb.NewExpr()
			for j := range nVars {
				if j%2 == 0 {
					xs[j] = m.AddIntVar("x"+strconv.Itoa(j), 0, 8)
				} else {
					xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
				}
				c := float64(1 + rLocal.IntN(9))
				if sense == gb.Maximize {
					c = -c
				}
				obj = obj.AddTerm(xs[j], c)
				row = row.AddTerm(xs[j], float64(2+rLocal.IntN(7)))
			}
			m.SetObjective(obj)
			m.AddConstraint("dem", gb.Ge(row, float64(5+rLocal.IntN(6))))
			return m
		}
	}

	switch paramA {
	case 0:
		objConst = 1e12
	case 1:
		objConst = 1e17
	case 2:
		objConst = -1e17
	default:
		consts := []float64{0, 1e8, -1e8, 1e12, -1e12, 1e17, -1e17}
		objConst = consts[rng.IntN(len(consts))]
	}

	pZero := buildPair(0).Compile()
	pShift := buildPair(objConst).Compile()

	resRef := mustSolveCompiled(t, pZero, gb.WithPureGo(),
		gb.WithPresolve(false), gb.WithRelGapTol(0), gb.WithAbsGapTol(0),
		gb.WithMaxNodes(500), gb.WithTimeLimit(time.Second))
	if resRef.Status != gb.StatusOptimal {
		t.Fatalf("[%s] zero-constant reference solve returned %v, want StatusOptimal", label, resRef.Status)
	}
	optLinMinRat, okRef := exactEvalLinearMinRat(pZero, resRef.Values)
	if !okRef {
		t.Fatalf("[%s] failed to evaluate reference linear objective", label)
	}
	var resTrShift gb.Result
	var trShift *gb.VIPRTrace
	if formalFuzzEnabled() {
		resRefTr, trRef := verifyFormalVIPRTrace(t, label+"/ref_zero", pZero, 500, time.Second,
			gb.WithPresolve(false), gb.WithCuts(true), gb.WithHeuristics(true), gb.WithRelGapTol(0), gb.WithAbsGapTol(0))
		if resRefTr.Status == gb.StatusOptimal && (!trRef.HasCertificate() || trRef.Mode() != "optimal") {
			t.Fatalf("[%s] expected optimal VIPR certificate on zero-constant reference, got mode=%q", label, trRef.Mode())
		}
		resTrShift, trShift = verifyFormalVIPRTrace(t, label+"/shift_vipr", pShift, 500, time.Second,
			gb.WithPresolve(false), gb.WithCuts(true), gb.WithHeuristics(true), gb.WithRelGapTol(relTol), gb.WithAbsGapTol(absTol))
		if relTol == 0 && absTol == 0 && resTrShift.Status == gb.StatusOptimal {
			if !trShift.HasCertificate() || trShift.Mode() != "optimal" {
				t.Fatalf("[%s] zero-tolerance Optimal solve produced VIPR mode=%q (want optimal)", label, trShift.Mode())
			}
		}
	}

	workers := [3]int{1, 2, 4}[int(seed+uint64(paramB))%3]
	resDefault := mustSolveCompiled(t, pShift, gb.WithPureGo(),
		gb.WithPresolve(true), gb.WithCuts(true), gb.WithHeuristics(true), gb.WithWorkers(workers),
		gb.WithRelGapTol(relTol), gb.WithAbsGapTol(absTol),
		gb.WithMaxNodes(500), gb.WithTimeLimit(time.Second))
	if formalFuzzEnabled() {
		assertFullFeatureVsVIPR(t, label+"/shift_full_vs_vipr", pShift, resDefault, resTrShift, trShift, relTol, absTol)
	}
	if resDefault.Status != gb.StatusOptimal {
		t.Fatalf("[%s] shifted solve returned %v, want StatusOptimal", label, resDefault.Status)
	}
	gotLinMinRat, okGot := exactEvalLinearMinRat(pShift, resDefault.Values)
	if !okGot {
		t.Fatalf("[%s] failed to evaluate shifted linear objective", label)
	}
	diffRat := new(big.Rat).Sub(gotLinMinRat, optLinMinRat)
	diffFloat, _ := diffRat.Float64()
	gotLinFloat, _ := gotLinMinRat.Float64()
	allowedGap := max(absTol, relTol*max(1.0, math.Abs(gotLinFloat))) + 1e-9
	if relTol == 0 && absTol == 0 {
		if diffRat.Sign() != 0 {
			t.Fatalf("[%s] zero-tolerance suboptimal linear objective: got %s, exact opt %s (ObjConstant=%g)",
				label, gotLinMinRat.RatString(), optLinMinRat.RatString(), objConst)
		}
	} else if diffFloat > allowedGap {
		t.Fatalf("[%s] linear MIP gap %.8g exceeds RelGapTol/AbsGapTol bound %.8g (gotLin=%s optLin=%s ObjConstant=%g)",
			label, diffFloat, allowedGap, gotLinMinRat.RatString(), optLinMinRat.RatString(), objConst)
	}
}

func FuzzAdversarialMIPFormal(f *testing.F) {
	seeds := []struct {
		seed   uint64
		regime uint8
		a, b   uint8
	}{
		{0, 0, 0, 0},
		{1, 0, 1, 0},
		{2, 0, 2, 1},
		{3, 1, 0, 0},
		{4, 1, 1, 1},
		{5, 1, 2, 6},
		{6, 2, 0, 0},
		{7, 2, 2, 1},
		{8, 2, 1, 0},
		{9, 3, 0, 0},
		{10, 3, 1, 1},
		{11, 4, 0, 0},
		{12, 4, 1, 2},
		{13, 5, 0, 0},
		{14, 5, 1, 0},
		{15, 5, 2, 1},
		{16, 5, 3, 2},
		{17, 1, 0, 1},
		{18, 1, 1, 0},
		{3, 18, 211, 3},
		{84, 96, 24, 72},
		{84, 96, 18, 20},
		{6, 0, 62, 6},
		{61, 0, 10, 102},
		{85, 0, 134, 247},
		{1, 6, 22, 98},
		{70, 198, 62, 0},
		{19, 4, 4, 0},
		{20, 4, 5, 1},
		{21, 4, 6, 0},
		{22, 4, 6, 1},
		{23, 4, 7, 2},
	}
	for _, s := range seeds {
		f.Add(s.seed, s.regime, s.a, s.b)
	}

	f.Fuzz(func(t *testing.T, seed uint64, regimeByte, paramA, paramB uint8) {
		switch regimeByte % 6 {
		case 0:
			runAdversarialMIPActivityRegime(t, seed, paramA, paramB)
		case 1:
			runAdversarialMIPDynamicOrbitRegime(t, seed, paramA, paramB)
		case 2:
			runAdversarialMIPGCDAndBinRegime(t, seed, paramA, paramB)
		case 3:
			runAdversarialMIPCMIRUnderflowRegime(t, seed, paramA, paramB)
		case 4:
			runAdversarialMIPVUBAndCutsRegime(t, seed, paramA, paramB)
		default:
			runAdversarialMIPObjConstRegime(t, seed, paramA, paramB)
		}
	})
}

func FuzzMIPSmall(f *testing.F) {
	for seed := range uint64(16) {
		f.Add(seed, uint8(seed%6), uint8(seed*3))
	}
	f.Add(uint64(375), uint8(0x14), uint8(0xe6))

	f.Fuzz(func(t *testing.T, seed uint64, kindByte, flagByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x94d049bb133111eb, uint64(kindByte)*37+uint64(flagByte)))
		sense := gb.Minimize
		if flagByte&1 == 1 {
			sense = gb.Maximize
		}
		tag := fmt.Sprintf("mip_small_k%d_%d", kindByte%5, seed)
		m := gb.NewModel(tag, sense)

		switch kindByte % 5 {
		case 0:
			// Semi-bounded mixed-integer system with non-dyadic coefficients and cover row.
			nVars := 4 + int(flagByte%4)
			xs := make([]gb.Var, nVars)
			obj := gb.NewExpr()
			for j := range nVars {
				switch j % 3 {
				case 0:
					xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
				case 1:
					hi := float64(4 + rng.IntN(6))
					if rng.IntN(3) == 0 {
						hi = math.Inf(1)
					}
					xs[j] = m.AddIntVar("x"+strconv.Itoa(j), 0, hi)
				default:
					hi := float64(8 + rng.IntN(10))
					if rng.IntN(2) == 0 {
						hi = math.Inf(1)
					}
					xs[j] = m.AddContVar("x"+strconv.Itoa(j), 0, hi)
				}
				c := float64(rng.IntN(13) - 6)
				if c != 0 {
					obj = obj.AddTerm(xs[j], c)
				}
			}
			m.SetObjective(obj)
			nRows := 2 + int((flagByte>>2)%4)
			for i := range nRows {
				e := gb.NewExpr()
				for j := range nVars {
					if rng.Float64() < 0.7 {
						a := float64(rng.IntN(11) - 5)
						if a != 0 {
							e = e.AddTerm(xs[j], a)
						}
					}
				}
				if len(e.Terms) == 0 {
					continue
				}
				rhs := float64(rng.IntN(17) - 3)
				switch rng.IntN(3) {
				case 0:
					m.AddConstraint("r"+strconv.Itoa(i), gb.Le(e, rhs))
				case 1:
					m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(e, rhs-4))
				default:
					m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(e, rhs))
				}
			}
		case 1:
			// Small (l, S) lot-sizing chain with semi-bounded continuous production & inventory.
			T := 3
			obj := gb.NewExpr()
			var prevS gb.Var
			for tau := range T {
				y := m.AddBinaryVar("y" + strconv.Itoa(tau))
				pv := m.AddContVar("p"+strconv.Itoa(tau), 0, math.Inf(1))
				sv := m.AddContVar("s"+strconv.Itoa(tau), 0, math.Inf(1))
				capM := float64(25 + 10*rng.IntN(3))
				m.AddConstraint("vub"+strconv.Itoa(tau), gb.Le(gb.NewExpr(gb.T(pv, 1), gb.T(y, -capM)), 0))
				dem := float64(5 + rng.IntN(9))
				if tau == 0 {
					m.AddConstraint("bal0", gb.Eq(gb.NewExpr(gb.T(pv, 1), gb.T(sv, -1)), dem))
				} else {
					m.AddConstraint("bal"+strconv.Itoa(tau), gb.Eq(gb.NewExpr(gb.T(prevS, 1), gb.T(pv, 1), gb.T(sv, -1)), dem))
				}
				prevS = sv
				cSign := 1.0
				if sense == gb.Maximize {
					cSign = -1.0
				}
				obj = obj.AddTerm(y, cSign*float64(8+rng.IntN(7))).AddTerm(sv, cSign*0.5)
			}
			m.SetObjective(obj)
		case 2:
			// Lifted knapsack cover + GCD inequality division.
			nBin := 4 + int(flagByte%3)
			bs := make([]gb.Var, nBin)
			knap := gb.NewExpr()
			gcdRow := gb.NewExpr()
			obj := gb.NewExpr()
			g := float64(2 + rng.IntN(4))
			sumW := 0.0
			for j := range nBin {
				bs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
				w := float64(3 + rng.IntN(6))
				sumW += w
				if j == 1 && flagByte&2 != 0 {
					knap = knap.AddTerm(bs[j], -w)
				} else {
					knap = knap.AddTerm(bs[j], w)
				}
				gcdRow = gcdRow.AddTerm(bs[j], g*float64(1+rng.IntN(3)))
				obj = obj.AddTerm(bs[j], float64(1+rng.IntN(8)))
			}
			m.SetObjective(obj)
			m.AddConstraint("knap", gb.Le(knap, math.Max(3, math.Floor(0.55*sumW))))
			m.AddConstraint("gcd_le", gb.Le(gcdRow, g*float64(nBin)+0.5))
		case 3:
			// Multi-row aggregated MIR with interior continuous variable.
			x0 := m.AddIntVar("x0", 0, 6)
			x1 := m.AddIntVar("x1", 0, 6)
			sc := m.AddContVar("sc", 0, math.Inf(1))
			cSign := 1.0
			if sense == gb.Minimize {
				cSign = -1.0
			}
			m.SetObjective(gb.NewExpr(gb.T(x0, cSign*3), gb.T(x1, cSign*4), gb.T(sc, -cSign*0.25)))
			m.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(x0, float64(2+rng.IntN(3))), gb.T(sc, 1)), float64(3+rng.IntN(5))))
			m.AddConstraint("le", gb.Le(gb.NewExpr(gb.T(sc, -1), gb.T(x1, float64(2+rng.IntN(3)))), float64(2+rng.IntN(4))))
		default:
			// Meyer unbounded MILP or Diophantine equality test.
			x := m.AddIntVar("x", 0, math.Inf(1))
			y := m.AddIntVar("y", 0, math.Inf(1))
			c := m.AddContVar("c", 0, math.Inf(1))
			dir := -1.0
			if sense == gb.Maximize {
				dir = 1.0
			}
			m.SetObjective(gb.NewExpr(gb.T(x, dir*2), gb.T(y, -dir*0.5)))
			a := float64(2 + rng.IntN(4))
			b := float64(2 + rng.IntN(4))
			rhs := float64(rng.IntN(3))
			m.AddConstraint("eq1", gb.Eq(gb.NewExpr(gb.T(x, a), gb.T(y, -b)), rhs))
			m.AddConstraint("eq2", gb.Eq(gb.NewExpr(gb.T(c, 3), gb.T(x, -1)), 0))
		}

		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked: %v", tag, err)
		}
		_, _ = verifyFormalPresolveRules(t, p)
		verifyFormalRootLP(t, tag+"/rootLP", p)
		workers := [3]int{1, 2, 4}[int(seed+uint64(flagByte))%3]
		resFull, _, _ := runFullFeatureVsVIPRCheck(t, tag, p, workers, 400, 800*time.Millisecond, 1e-6, 1e-6)
		var sols [][]float64
		if resFull.Status == gb.StatusOptimal && len(resFull.Values) == p.NumVars() {
			sols = append(sols, resFull.Values)
		}
		verifyFormalSeparatedCuts(t, tag+"/cuts", p, sols)
	})
}

func FuzzLPSmall(f *testing.F) {
	for seed := range uint64(20) {
		f.Add(seed, uint8(seed%6), uint8(seed*5))
	}
	f.Add(uint64(238), uint8(0x1d), uint8('D'))

	f.Fuzz(func(t *testing.T, seed uint64, kindByte, flagByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0xbf58476d1ce4e5b9, uint64(kindByte)*41+uint64(flagByte)))
		sense := gb.Minimize
		if flagByte&1 == 1 {
			sense = gb.Maximize
		}
		nVars := 2 + int(kindByte%6)
		nRows := 1 + int((flagByte>>1)%5)
		tag := fmt.Sprintf("lp_small_%d", seed)
		m := gb.NewModel(tag, sense)
		xs := make([]gb.Var, nVars)
		obj := gb.NewExpr().AddConstant(float64(int(flagByte>>4)-8) * 1.5)
		for j := range nVars {
			var lo, hi float64
			switch (int(kindByte) + j) % 4 {
			case 0:
				lo = math.Inf(-1)
				hi = math.Inf(1)
			case 1:
				lo = 0
				hi = math.Inf(1)
			case 2:
				lo = math.Inf(-1)
				hi = 0
			default:
				lo = float64(rng.IntN(5) - 2)
				hi = lo + float64(1+rng.IntN(8))
			}
			xs[j] = m.AddContVar("x"+strconv.Itoa(j), lo, hi)
			c := float64(rng.IntN(15) - 7)
			if c != 0 {
				obj = obj.AddTerm(xs[j], c)
			}
		}
		m.SetObjective(obj)

		primes := []float64{-7, -5, -3, -2, -1, 1, 2, 3, 5, 7, 11}
		for i := range nRows {
			e := gb.NewExpr()
			for j := range nVars {
				if rng.Float64() < 0.65 {
					e = e.AddTerm(xs[j], primes[rng.IntN(len(primes))])
				}
			}
			if len(e.Terms) == 0 {
				continue
			}
			rhs := float64(rng.IntN(19) - 6)
			switch rng.IntN(3) {
			case 0:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Le(e, rhs))
			case 1:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(e, rhs))
			default:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(e, rhs))
			}
		}

		p, err := m.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked: %v", tag, err)
		}

		ws1 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws1)
		lpSIMD := gb.NewLPSolver(p, ws1)
		resSIMD := lpSIMD.Solve()

		ws2 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws2)
		lpScalar := gb.NewLPSolver(p, ws2)
		lpScalar.DisableSIMD = true
		resScalar := lpScalar.Solve()

		if resSIMD.Status != resScalar.Status {
			t.Fatalf("[%s] SIMD status %v != Scalar status %v", tag, resSIMD.Status, resScalar.Status)
		}
		if resSIMD.Status == gb.StatusOptimal {
			bV, rV, _ := p.MaxViolation(resSIMD.Primal, false)
			if bV > 1e-5 || rV > 1e-5 {
				t.Fatalf("[%s] LP primal violation: bound=%.2e row=%.2e", tag, bV, rV)
			}
			if math.Abs(resSIMD.Objective-resScalar.Objective) > 1e-6*max(1.0, math.Abs(resScalar.Objective)) {
				t.Fatalf("[%s] SIMD obj %.8f != Scalar obj %.8f", tag, resSIMD.Objective, resScalar.Objective)
			}
		}
		if formalFuzzEnabled() {
			verifyFormalLPResult(t, tag+"/simd", p, lpSIMD, resSIMD)
			verifyFormalLPResult(t, tag+"/scalar", p, lpScalar, resScalar)
		}
		resPre := verifyFormalPresolvedLP(t, tag+"/presolve", p, true)
		if resPre.Status != resSIMD.Status {
			t.Fatalf("[%s] presolved LP status %v != LP status %v", tag, resPre.Status, resSIMD.Status)
		}
		if resPre.Status == gb.StatusOptimal && math.Abs(resPre.Objective-resSIMD.Objective) > 1e-5*max(1.0, math.Abs(resSIMD.Objective)) {
			t.Fatalf("[%s] presolved LP obj %.8f != %.8f", tag, resPre.Objective, resSIMD.Objective)
		}
	})
}

func FuzzPuLPJSON(f *testing.F) {
	// Compact seeds maximize mutation throughput across parser edge cases.
	for _, seed := range []string{
		`{"parameters":{"name":"s","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":1}]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":10}],"constraints":[{"name":"c","sense":-1,"constant":-5,"coefficients":[{"name":"x","value":1}]}],"sos1":[],"sos2":[]}`,
		`{"parameters":{"name":"m","sense":-1},"objective":{"name":null,"coefficients":[],"constant":2.5},"variables":[{"name":"i","cat":"Integer","lowBound":0,"upBound":-5}],"constraints":[]}`,
		`{"parameters":{"name":"e","sense":1,"sense":-1},"objective":{"name":"o","coefficients":[]},"variables":[{"name":"u\u0031","cat":"Binary","lowBound":null,"upBound":null,"varValue":NaN}],"constraints":[]}`,
	} {
		f.Add([]byte(seed))
	}
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
		m2, err := format.FromPuLPJSON(&jsonBuf)
		if err != nil {
			t.Fatalf("FromPuLPJSON failed on round-trip output: %v", err)
		}
		p2, err := m2.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked failed on round-trip model: %v", err)
		}
		if p2.NumVars() != p.NumVars() || p2.NumConstraints() != p.NumConstraints() ||
			p2.NNZ() != p.NNZ() || p2.Sense() != p.Sense() || p2.ObjConstant() != p.ObjConstant() {
			t.Fatalf("PuLP JSON round-trip metadata mismatch: got (%d,%d,%d,%v,%v), want (%d,%d,%d,%v,%v)",
				p2.NumVars(), p2.NumConstraints(), p2.NNZ(), p2.Sense(), p2.ObjConstant(),
				p.NumVars(), p.NumConstraints(), p.NNZ(), p.Sense(), p.ObjConstant())
		}
		for j := range p.NumVars() {
			lb1, ub1 := p.VarBounds(j)
			lb2, ub2 := p2.VarBounds(j)
			if lb1 != lb2 || ub1 != ub2 || p.VarCategory(j) != p2.VarCategory(j) || p.ObjCoeff(j) != p2.ObjCoeff(j) {
				t.Fatalf("PuLP JSON round-trip var %d mismatch", j)
			}
		}
		for i := range p.NumConstraints() {
			if p.RowSense(i) != p2.RowSense(i) || p.RowRHS(i) != p2.RowRHS(i) || p.RowLen(i) != p2.RowLen(i) {
				t.Fatalf("PuLP JSON round-trip row %d mismatch", i)
			}
		}
	})
}
