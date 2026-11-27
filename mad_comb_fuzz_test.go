package goblin_test

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/internal/mad"
)

func buildCombFuzzMADProblem(
	seed uint64,
	mode, gByte, iByte, tByte, flags uint8,
	raceShort bool,
) (mad.Problem, string) {
	rng := rand.New(rand.NewPCG(seed^0x517cc1b727220a95, seed^0x94d049bb133111eb))

	numPeriods := 1 + int(tByte%6)
	regime := mode % 11
	allowZeroStep := (flags & 0x01) != 0
	allowNeg := (flags & 0x02) != 0
	roundGeT := (flags & 0x04) != 0
	hasD := (flags & 0x08) != 0
	stepZero := (flags & 0x10) != 0
	weightZero := (flags & 0x20) != 0
	nonDyadic := (flags & 0x40) != 0
	cloneBase := (flags & 0x80) != 0
	useBaselines := (gByte & 1) == 0

	var numGroups, numItems int
	switch regime {
	case 0:
		useBaselines = true
		hasD = false
		numGroups = 2 + int(gByte%5)
		numItems = 1 + int(iByte%2)
	case 1:
		allowNeg = true
		allowZeroStep = true
	case 2:
		roundGeT = true
		stepZero = false
		weightZero = false
	case 3:
		hasD = true
		useBaselines = false
	case 4:
		hasD = true
		useBaselines = true
	case 5:
		stepZero = true
	case 6:
		weightZero = true
	case 7:
		numItems = 0
		numGroups = 1 + int(gByte%5)
		useBaselines = true
		hasD = false
	case 8:
		numGroups = 1
		maxItems := 7
		if raceShort {
			maxItems = 4
		}
		numItems = 1 + int(iByte)%maxItems
		hasD = false
	case 9:
		hasD = false
		numGroups = 5 + int(gByte%2)
		numItems = 1 + int(iByte%3)
	case 10:
		nonDyadic = true
		stepZero = false
		weightZero = false
	}

	if numGroups == 0 {
		if hasD {
			if useBaselines {
				numGroups = 2 + int(gByte%2)
			} else {
				numGroups = 2 + int(gByte%3)
			}
			extraMax := 3
			if raceShort || numGroups >= 4 {
				extraMax = 2
			}
			numItems = numGroups + int(iByte)%(extraMax+1)
		} else {
			numGroups = 2 + int(gByte%5)
			if nonDyadic && numGroups > 3 {
				numGroups = 2 + int(gByte%2)
			}
			switch numGroups {
			case 5, 6:
				numItems = 1 + int(iByte%2)
			case 4:
				maxItems := 4
				if raceShort {
					maxItems = 3
				}
				numItems = 1 + int(iByte)%maxItems
			case 3:
				maxItems := 5
				if raceShort || nonDyadic {
					maxItems = 3
				}
				numItems = 1 + int(iByte)%maxItems
			default:
				maxItems := 6
				if raceShort {
					maxItems = 4
				}
				if nonDyadic && maxItems > 3 {
					maxItems = 3
				}
				numItems = 1 + int(iByte)%maxItems
			}
		}
	}
	if nonDyadic && numGroups > 3 {
		numGroups = 2 + int(gByte%2)
	}
	if nonDyadic && numItems > 3 {
		numItems = 1 + int(iByte%3)
	}
	if numGroups >= 5 && numItems > 2 {
		numItems = 1 + int(iByte%2)
	}

	var designated []int
	if hasD && numGroups >= 2 && numItems >= numGroups {
		perm := rng.Perm(numItems)
		designated = slices.Clone(perm[:numGroups])
	}

	sampleVal := func(forceNeg bool) float64 {
		var v float64
		if nonDyadic {
			choices := []float64{0.1, 0.3, 0.7, 1.3, 2.7, 5.1, 10.3}
			v = choices[rng.IntN(len(choices))]
		} else if rng.IntN(4) == 0 {
			choices := []float64{0.1, 0.25, 0.5, 1.5, 12.5, 25.0}
			v = choices[rng.IntN(len(choices))]
		} else {
			v = float64(rng.IntN(300))
		}
		if forceNeg || (allowNeg && rng.IntN(3) == 0) {
			v = -v
		}
		return v
	}

	var baselines [][]float64
	if useBaselines {
		baselines = make([][]float64, numGroups)
		for g := range numGroups {
			baselines[g] = make([]float64, numPeriods)
			forceNeg := allowNeg && g == 0 && (seed&1) != 0
			for t := range numPeriods {
				baselines[g][t] = sampleVal(forceNeg)
			}
		}
		if cloneBase && numGroups >= 2 {
			copy(baselines[1], baselines[0])
		}
	}

	var items [][]float64
	if numItems > 0 {
		items = make([][]float64, numItems)
		for i := range numItems {
			items[i] = make([]float64, numPeriods)
			for t := range numPeriods {
				items[i][t] = sampleVal(false)
			}
		}
	}

	step := 0.0
	if !stepZero {
		if nonDyadic {
			step = 0.3
		} else {
			steps := []float64{10, 50, 100}
			step = steps[rng.IntN(len(steps))]
		}
	}

	roundWeight := 0.0
	if !weightZero {
		switch {
		case roundGeT:
			if rng.IntN(2) == 0 {
				roundWeight = float64(numPeriods)
			} else {
				roundWeight = float64(numPeriods + 2)
			}
		case nonDyadic:
			roundWeight = 0.1
		default:
			weights := []float64{0.1, 0.5, 1.0, 2.0}
			roundWeight = weights[rng.IntN(len(weights))]
		}
	}

	prob := mad.Problem{
		Groups:          numGroups,
		Baselines:       baselines,
		Items:           items,
		Step:            step,
		RoundWeight:     roundWeight,
		AllowZeroStep:   allowZeroStep,
		DesignatedItems: designated,
	}
	tag := fmt.Sprintf("mad_comb_r%d_g%d_i%d_t%d_f%02x_%d", regime, numGroups, numItems, numPeriods, flags, seed)
	return prob, tag
}

func fuzzDyadicLimbsToRat(limbs []uint64, exp int) *big.Rat {
	num := new(big.Int)
	var word big.Int
	for _, limb := range slices.Backward(limbs) {
		num.Lsh(num, 64)
		if limb != 0 {
			word.SetUint64(limb)
			num.Or(num, &word)
		}
	}
	if num.Sign() == 0 {
		return new(big.Rat)
	}
	if exp > 0 {
		num.Lsh(num, uint(exp))
		return new(big.Rat).SetInt(num)
	}
	if exp < 0 {
		den := new(big.Int).Lsh(big.NewInt(1), uint(-exp))
		return new(big.Rat).SetFrac(num, den)
	}
	return new(big.Rat).SetInt(num)
}

func madFuzzRatFromFloat(t *testing.T, v float64) *big.Rat {
	t.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		t.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func madFuzzRatAbs(r *big.Rat) *big.Rat {
	if r.Sign() < 0 {
		return new(big.Rat).Neg(r)
	}
	return new(big.Rat).Set(r)
}

func madFuzzRatFloor(q *big.Rat) int64 {
	num := q.Num()
	den := q.Denom()
	quo, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() < 0 {
		quo.Sub(quo, big.NewInt(1))
	}
	return quo.Int64()
}

func madFuzzRatCeil(q *big.Rat) int64 {
	num := q.Num()
	den := q.Denom()
	quo, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() > 0 {
		quo.Add(quo, big.NewInt(1))
	}
	return quo.Int64()
}

func madFuzzBigRatEvalAssignment(t *testing.T, prob mad.Problem, assign []int) *big.Rat {
	t.Helper()
	numGroups := prob.Groups
	numPeriods := 0
	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
		numPeriods = len(prob.Baselines[0])
	} else {
		numPeriods = len(prob.Items[0])
	}

	bRat := madFuzzRatFromFloat(t, prob.Step)
	wrRat := madFuzzRatFromFloat(t, prob.RoundWeight)
	rounded := prob.Step > 0 && prob.RoundWeight > 0
	nMin := int64(1)
	if prob.AllowZeroStep {
		nMin = 0
	}

	total := new(big.Rat)
	for g := range numGroups {
		y := make([]*big.Rat, numPeriods)
		for idx := range numPeriods {
			y[idx] = new(big.Rat)
			if prob.Baselines != nil {
				y[idx].Set(madFuzzRatFromFloat(t, prob.Baselines[g][idx]))
			}
		}
		for i, ag := range assign {
			if ag != g {
				continue
			}
			for idx := range numPeriods {
				y[idx].Add(y[idx], madFuzzRatFromFloat(t, prob.Items[i][idx]))
			}
		}

		sorted := slices.Clone(y)
		slices.SortFunc(sorted, func(a, b *big.Rat) int { return a.Cmp(b) })

		evalGroupN := func(n int64, hasN bool) *big.Rat {
			var pRat *big.Rat
			cands := make([]*big.Rat, 0, numPeriods+2)
			cands = append(cands, new(big.Rat))
			if hasN {
				pRat = new(big.Rat).Mul(bRat, big.NewRat(n, 1))
				cands = append(cands, pRat)
			}
			for _, v := range sorted {
				if v.Sign() >= 0 {
					cands = append(cands, v)
				}
			}
			var best *big.Rat
			for _, m := range cands {
				cost := new(big.Rat)
				for _, v := range sorted {
					cost.Add(cost, madFuzzRatAbs(new(big.Rat).Sub(v, m)))
				}
				if hasN {
					diff := madFuzzRatAbs(new(big.Rat).Sub(m, pRat))
					cost.Add(cost, new(big.Rat).Mul(wrRat, diff))
				}
				if best == nil || cost.Cmp(best) < 0 {
					best = cost
				}
			}
			return best
		}

		if !rounded {
			total.Add(total, evalGroupN(0, false))
			continue
		}

		uLo := new(big.Rat).Set(sorted[(numPeriods-1)/2])
		if uLo.Sign() < 0 {
			uLo.SetInt64(0)
		}
		uHi := new(big.Rat).Set(sorted[numPeriods/2])
		if uHi.Sign() < 0 {
			uHi.SetInt64(0)
		}
		nLo := max(nMin, madFuzzRatCeil(new(big.Rat).Quo(uLo, bRat)))
		nHi := madFuzzRatFloor(new(big.Rat).Quo(uHi, bRat))
		switch {
		case nLo <= nHi:
			total.Add(total, evalGroupN(nLo, true))
		case nHi < nMin:
			total.Add(total, evalGroupN(nMin, true))
		default:
			cHi := evalGroupN(nHi, true)
			cLo := evalGroupN(nLo, true)
			if cLo.Cmp(cHi) < 0 {
				total.Add(total, cLo)
			} else {
				total.Add(total, cHi)
			}
		}
	}
	return total
}

func verifyMADFuzzDyadicExact(t *testing.T, tag string, prob mad.Problem, certRes mad.Result) {
	t.Helper()
	ev, err := mad.EvaluateAssignmentDyadic(prob, certRes.Assignment)
	if err != nil {
		t.Fatalf("[%s] EvaluateAssignmentDyadic failed: %v", tag, err)
	}
	evRat := fuzzDyadicLimbsToRat(ev.Limbs, ev.Exp)
	wantRat := madFuzzBigRatEvalAssignment(t, prob, certRes.Assignment)
	if evRat.Cmp(wantRat) != 0 {
		t.Fatalf("[%s] dyadic exact objective %s != big.Rat reference %s", tag, evRat.RatString(), wantRat.RatString())
	}
	if certRes.Objective != ev.Result.Objective {
		t.Fatalf("[%s] certified Objective %.17g != dyadic eval %.17g", tag, certRes.Objective, ev.Result.Objective)
	}
	boundRat := new(big.Rat).SetFloat64(certRes.BestBound)
	if boundRat == nil {
		t.Fatalf("[%s] non-finite BestBound %v", tag, certRes.BestBound)
	}
	if boundRat.Cmp(evRat) > 0 {
		t.Fatalf("[%s] certified BestBound %.17g > exact rational %s", tag, certRes.BestBound, evRat.RatString())
	}
}

func verifyMADFuzzParity(
	t *testing.T,
	tag string,
	prob mad.Problem,
	ref, got mad.Result,
	gotErr error,
	exactBitMatch bool,
) {
	t.Helper()
	if gotErr != nil {
		t.Fatalf("[%s] mad.Solve failed: %v", tag, gotErr)
	}
	if math.IsNaN(got.Objective) || math.IsInf(got.Objective, 0) {
		t.Fatalf("[%s] non-finite Objective %v", tag, got.Objective)
	}
	if math.IsNaN(got.BestBound) || math.IsInf(got.BestBound, 0) || got.BestBound < 0 {
		t.Fatalf("[%s] invalid BestBound %v", tag, got.BestBound)
	}
	tol := 1e-9 * max(1.0, math.Abs(ref.Objective))
	if got.BestBound > got.Objective+tol {
		t.Fatalf("[%s] BestBound %.17g > Objective %.17g", tag, got.BestBound, got.Objective)
	}
	if exactBitMatch {
		if got.Status != mad.StatusOptimal {
			t.Fatalf("[%s] Status = %v, want %v", tag, got.Status, mad.StatusOptimal)
		}
		u := math.Abs(math.Nextafter(ref.Objective, math.Inf(1)) - ref.Objective)
		if u == 0 {
			u = math.SmallestNonzeroFloat64
		}
		if math.Abs(got.Objective-ref.Objective) > 4*u {
			t.Fatalf("[%s] Objective %.17g != certified %.17g (diff=%g > 4 ulp=%g)",
				tag, got.Objective, ref.Objective, math.Abs(got.Objective-ref.Objective), 4*u)
		}
	} else if got.Status == mad.StatusOptimal {
		if math.Abs(got.Objective-ref.Objective) > tol {
			t.Fatalf("[%s] Objective %.17g != certified %.17g (diff=%g, tol=%g)",
				tag, got.Objective, ref.Objective, math.Abs(got.Objective-ref.Objective), tol)
		}
	} else if got.Status == mad.StatusNodeLimit || got.Status == mad.StatusTimeLimit {
		if got.BestBound > ref.Objective+tol {
			t.Fatalf("[%s] limit BestBound %.17g > certified %.17g", tag, got.BestBound, ref.Objective)
		}
		if got.Objective+tol < ref.Objective {
			t.Fatalf("[%s] limit Objective %.17g < certified %.17g", tag, got.Objective, ref.Objective)
		}
	} else {
		t.Fatalf("[%s] unexpected Status = %v", tag, got.Status)
	}

	numGroups := prob.Groups
	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
	}
	if len(got.Assignment) != len(prob.Items) {
		t.Fatalf("[%s] len(Assignment) = %d, want %d", tag, len(got.Assignment), len(prob.Items))
	}
	if len(got.Levels) != numGroups || len(got.Steps) != numGroups {
		t.Fatalf("[%s] len(Levels)=%d len(Steps)=%d, want %d", tag, len(got.Levels), len(got.Steps), numGroups)
	}
	if len(prob.DesignatedItems) > 0 {
		seen := make([]bool, numGroups)
		for _, item := range prob.DesignatedItems {
			g := got.Assignment[item]
			if g < 0 || g >= numGroups {
				t.Fatalf("[%s] designated item %d assigned to out-of-range group %d", tag, item, g)
			}
			if seen[g] {
				t.Fatalf("[%s] designated items collide on group %d (assign=%v)", tag, g, got.Assignment)
			}
			seen[g] = true
		}
	}
	if prob.Step == 0 || prob.RoundWeight == 0 {
		for g, st := range got.Steps {
			if st != 0 {
				t.Fatalf("[%s] unrounded Steps[%d] = %d, want 0", tag, g, st)
			}
		}
	}
}

func verifyMADFuzzModelResult(
	t *testing.T,
	tag string,
	p *gb.CompiledProblem,
	ref mad.Result,
	res gb.Result,
	err error,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("[%s] SolveCompiled failed: %v", tag, err)
	}
	tol := 1e-6 * max(1.0, math.Abs(ref.Objective))
	switch res.Status {
	case gb.StatusOptimal:
		if len(res.Values) != p.NumVars() {
			t.Fatalf("[%s] Optimal len(Values)=%d, want %d", tag, len(res.Values), p.NumVars())
		}
		bV, rV, iV := p.MaxViolation(res.Values, true)
		if bV > 1e-6 || rV > 1e-5 || iV > 1e-6 {
			t.Fatalf("[%s] primal violation bV=%.2e rV=%.2e iV=%.2e", tag, bV, rV, iV)
		}
		if math.Abs(res.Objective-ref.Objective) > tol {
			t.Fatalf("[%s] Objective %.17g != certified %.17g (tol=%g)", tag, res.Objective, ref.Objective, tol)
		}
		if res.BestBound > res.Objective+tol {
			t.Fatalf("[%s] BestBound %.17g > Objective %.17g", tag, res.BestBound, res.Objective)
		}
	case gb.StatusNodeLimit, gb.StatusTimeLimit, gb.StatusIterationLimit:
		if !math.IsInf(res.BestBound, 0) && res.BestBound > ref.Objective+tol {
			t.Fatalf("[%s] limit BestBound %.17g > certified %.17g", tag, res.BestBound, ref.Objective)
		}
	default:
		t.Fatalf("[%s] unexpected Status = %v", tag, res.Status)
	}
}

// FuzzMADCombinatorialVsMILP differentially fuzzes internal/mad and automatic algebraic
// Model.Solve combinatorial detection against pure Branch-and-Cut (BackendGo and BackendCBC).
func FuzzMADCombinatorialVsMILP(f *testing.F) {
	f.Add(uint64(1), uint8(0), uint8(9), uint8(2), uint8(5), uint8(0))
	f.Add(uint64(2), uint8(1), uint8(2), uint8(4), uint8(2), uint8(0x03))
	f.Add(uint64(3), uint8(2), uint8(2), uint8(4), uint8(3), uint8(0x04))
	f.Add(uint64(4), uint8(3), uint8(1), uint8(2), uint8(3), uint8(0x08))
	f.Add(uint64(5), uint8(4), uint8(0), uint8(2), uint8(3), uint8(0x08))
	f.Add(uint64(6), uint8(5), uint8(2), uint8(3), uint8(2), uint8(0x10))
	f.Add(uint64(7), uint8(6), uint8(2), uint8(3), uint8(2), uint8(0x20))
	f.Add(uint64(8), uint8(7), uint8(2), uint8(0), uint8(3), uint8(0x00))
	f.Add(uint64(9), uint8(8), uint8(0), uint8(5), uint8(3), uint8(0x02))
	f.Add(uint64(10), uint8(9), uint8(5), uint8(2), uint8(2), uint8(0x00))
	f.Add(uint64(11), uint8(10), uint8(2), uint8(3), uint8(3), uint8(0x40))
	f.Add(uint64(52), uint8(7), uint8(2), uint8(0), uint8(12), uint8(0x42))
	f.Add(uint64(91), uint8(2), uint8(98), uint8(104), uint8(90), uint8(0x54))

	f.Fuzz(func(t *testing.T, seed uint64, mode, gByte, iByte, tByte, flags uint8) {
		prob, tag := buildCombFuzzMADProblem(seed, mode, gByte, iByte, tByte, flags, raceEnabled && testing.Short())
		ctx := t.Context()

		certRes, certErr := mad.Solve(ctx, prob, mad.WithCertified(true))
		if certErr != nil {
			t.Fatalf("[%s/cert] mad.Solve failed: %v", tag, certErr)
		}
		verifyMADFuzzParity(t, tag+"/cert", prob, certRes, certRes, nil, true)
		verifyMADFuzzDyadicExact(t, tag+"/dyadic", prob, certRes)

		dpRes, dpErr := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyDP))
		verifyMADFuzzParity(t, tag+"/dp", prob, certRes, dpRes, dpErr, true)

		bfRes, bfErr := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyBruteForce))
		verifyMADFuzzParity(t, tag+"/bf", prob, certRes, bfRes, bfErr, true)

		m, buildErr := bench.BuildMADModel(prob)
		if buildErr != nil {
			t.Fatalf("[%s] BuildMADModel failed: %v", tag, buildErr)
		}
		p, compErr := m.CompileChecked()
		if compErr != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", tag, compErr)
		}

		// Default Model.Solve must detect MAD structure and match certified optimum.
		autoRes, autoErr := gb.SolveCompiled(ctx, p)
		verifyMADFuzzModelResult(t, tag+"/model_auto", p, certRes, autoRes, autoErr)
		if autoRes.Status != gb.StatusOptimal {
			t.Fatalf("[%s/model_auto] status=%v, want Optimal", tag, autoRes.Status)
		}

		// Certified Model.Solve (without explicit VIPR trace) must certify exact rational witness.
		var cw gb.CertifiedWitness
		modelCertRes, modelCertErr := gb.SolveCompiled(ctx, p, gb.WithCertifiedWitness(&cw))
		verifyMADFuzzModelResult(t, tag+"/model_cert", p, certRes, modelCertRes, modelCertErr)
		if modelCertRes.Status != gb.StatusOptimal || cw.Objective == nil {
			t.Fatalf("[%s/model_cert] status=%v cw.Objective=%v", tag, modelCertRes.Status, cw.Objective)
		}

		// Pure Branch-and-Cut with combinatorial search disabled.
		milpGoRes, milpGoErr := gb.SolveCompiled(ctx, p,
			gb.WithPureGo(),
			gb.WithCombinatorialSearch(false),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
		)
		verifyMADFuzzModelResult(t, tag+"/milp_go", p, certRes, milpGoRes, milpGoErr)

		if gb.CBCAvailable() {
			milpCBCRes, milpCBCErr := gb.SolveCompiled(ctx, p,
				gb.WithCBC(),
				gb.WithRelGapTol(0),
				gb.WithAbsGapTol(0),
			)
			verifyMADFuzzModelResult(t, tag+"/milp_cbc", p, certRes, milpCBCRes, milpCBCErr)
		}

		if formalFuzzEnabled() {
			verifyFormalRootLP(t, tag+"/rootLP", p)
		}
	})
}
