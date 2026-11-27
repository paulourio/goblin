package mad

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"math"
	"math/big"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func pmadBaseProblem() Problem {
	return Problem{
		Baselines: [][]float64{
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
		Items: [][]float64{
			{149, 566, 1479, 128, 1950, 1713},
			{491, 1179, 1737, 1103, 990, 1847},
		},
		Step:          100,
		RoundWeight:   6,
		AllowZeroStep: false,
	}
}

func bigRatFromFloat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func bigRatAbs(r *big.Rat) *big.Rat {
	if r.Sign() < 0 {
		return new(big.Rat).Neg(r)
	}
	return new(big.Rat).Set(r)
}

func bigRatEvalGroupFixed(tb testing.TB, y []*big.Rat, step, roundWeight float64, n int64) (*big.Rat, *big.Rat) {
	tb.Helper()
	numPeriods := len(y)
	bRat := bigRatFromFloat(tb, step)
	wrRat := bigRatFromFloat(tb, roundWeight)
	pRat := new(big.Rat).Mul(bRat, big.NewRat(n, 1))

	candidates := []*big.Rat{new(big.Rat), pRat}
	for _, v := range y {
		if v.Sign() >= 0 {
			candidates = append(candidates, v)
		}
	}

	var bestCost, bestM *big.Rat
	for _, m := range candidates {
		cost := new(big.Rat)
		for idx := range numPeriods {
			cost.Add(cost, bigRatAbs(new(big.Rat).Sub(y[idx], m)))
		}
		if step > 0 && roundWeight > 0 {
			diff := bigRatAbs(new(big.Rat).Sub(m, pRat))
			cost.Add(cost, new(big.Rat).Mul(wrRat, diff))
		}
		if bestCost == nil || cost.Cmp(bestCost) < 0 {
			bestCost = cost
			bestM = new(big.Rat).Set(m)
		}
	}
	return bestCost, bestM
}

func bigRatEvalAssignment(tb testing.TB, prob Problem, assign []int) *big.Rat {
	tb.Helper()
	numGroups, _, numPeriods, _, err := validateProblem(prob)
	if err != nil {
		tb.Fatalf("validateProblem: %v", err)
	}
	total := new(big.Rat)
	for g := range numGroups {
		y := make([]*big.Rat, numPeriods)
		for idx := range numPeriods {
			y[idx] = new(big.Rat)
			if prob.Baselines != nil {
				y[idx].Set(bigRatFromFloat(tb, prob.Baselines[g][idx]))
			}
		}
		for i, ag := range assign {
			if ag != g {
				continue
			}
			for idx := range numPeriods {
				y[idx].Add(y[idx], bigRatFromFloat(tb, prob.Items[i][idx]))
			}
		}

		if prob.Step == 0 || prob.RoundWeight == 0 {
			c, _ := bigRatEvalGroupFixed(tb, y, 0, 0, 0)
			total.Add(total, c)
			continue
		}

		// Collect sorted non-negative medians to bound candidate step range.
		sortedY := make([]*big.Rat, numPeriods)
		for idx := range numPeriods {
			sortedY[idx] = new(big.Rat).Set(y[idx])
		}
		slices.SortFunc(sortedY, func(a, b *big.Rat) int { return a.Cmp(b) })
		uLo := sortedY[(numPeriods-1)/2]
		if uLo.Sign() < 0 {
			uLo = new(big.Rat)
		}
		uHi := sortedY[numPeriods/2]
		if uHi.Sign() < 0 {
			uHi = new(big.Rat)
		}
		bRat := bigRatFromFloat(tb, prob.Step)
		loF, _ := new(big.Rat).Quo(uLo, bRat).Float64()
		hiF, _ := new(big.Rat).Quo(uHi, bRat).Float64()
		nMin := int64(1)
		if prob.AllowZeroStep {
			nMin = 0
		}
		nStart := max(nMin, int64(math.Floor(loF))-2)
		nEnd := max(nMin+2, int64(math.Ceil(hiF))+2)

		var bestGroup *big.Rat
		for n := nStart; n <= nEnd; n++ {
			c, _ := bigRatEvalGroupFixed(tb, y, prob.Step, prob.RoundWeight, n)
			if bestGroup == nil || c.Cmp(bestGroup) < 0 {
				bestGroup = c
			}
		}
		total.Add(total, bestGroup)
	}
	return total
}

func TestMADPMADBaseExact24137(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	prob := pmadBaseProblem()

	wantAssign := []int{6, 9}
	wantLevels := []float64{600, 400, 1300, 900, 900, 1100, 1800, 1500, 700, 2200}
	wantSteps := []int64{6, 4, 13, 9, 9, 11, 18, 15, 7, 22}

	cases := []struct {
		name         string
		opts         []Option
		wantStrategy Strategy
	}{
		{"Auto", nil, StrategyDP},
		{"BruteForce", []Option{WithStrategy(StrategyBruteForce)}, StrategyBruteForce},
		{"DP", []Option{WithStrategy(StrategyDP)}, StrategyDP},
		{"CertifiedAuto", []Option{WithCertified(true)}, StrategyDP},
		{"CertifiedDP", []Option{WithStrategy(StrategyDP), WithCertified(true)}, StrategyDP},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Solve(ctx, prob, tc.opts...)
			if err != nil {
				t.Fatalf("Solve(%s): %v", tc.name, err)
			}
			if res.Status != StatusOptimal {
				t.Fatalf("Status = %v, want Optimal", res.Status)
			}
			if res.Strategy != tc.wantStrategy {
				t.Fatalf("Strategy = %v, want %v", res.Strategy, tc.wantStrategy)
			}
			if res.Objective != 24137 || res.BestBound != 24137 {
				t.Fatalf("Objective=%v BestBound=%v, want 24137", res.Objective, res.BestBound)
			}
			if !slices.Equal(res.Assignment, wantAssign) {
				t.Fatalf("Assignment = %v, want %v", res.Assignment, wantAssign)
			}
			if !slices.Equal(res.Levels, wantLevels) {
				t.Fatalf("Levels = %v, want %v", res.Levels, wantLevels)
			}
			if !slices.Equal(res.Steps, wantSteps) {
				t.Fatalf("Steps = %v, want %v", res.Steps, wantSteps)
			}

			evalRes, evalErr := EvaluateAssignment(prob, res.Assignment)
			if evalErr != nil || evalRes.Objective != 24137 {
				t.Fatalf("EvaluateAssignment = (%+v, %v), want Objective 24137", evalRes, evalErr)
			}
		})
	}
}

func TestMADTwoSumTwoProdFMASafety(t *testing.T) {
	t.Parallel()

	a := 1.0 + math.Ldexp(1.0, -27)
	b := 1.0 + math.Ldexp(1.0, -27)
	p := twoProd(a, b)
	wantPE := math.Ldexp(1.0, -54)
	if p.hi != 1.0+math.Ldexp(1.0, -26) || p.lo != wantPE {
		t.Fatalf("twoProd(%g, %g) = (%g, %g), want (%g, %g)", a, b, p.hi, p.lo, 1.0+math.Ldexp(1.0, -26), wantPE)
	}

	cancel := ddAdd(dd{hi: -(1.0 + math.Ldexp(1.0, -26))}, p)
	if cancel.hi != wantPE || cancel.lo != 0 {
		t.Fatalf("ddAdd exact cancellation = (%g, %g), want (%g, 0)", cancel.hi, cancel.lo, wantPE)
	}

	// Verify twoSum preserves half-ULP residual when added to 2^48.
	base := math.Ldexp(1.0, 48)
	tiny := 0.03125 // 0.5 * ulp(2^48)
	sum := twoSum(base, tiny)
	if sum.hi != base || sum.lo != tiny {
		t.Fatalf("twoSum(2^48, 0.03125) = (%g, %g), want (%g, %g)", sum.hi, sum.lo, base, tiny)
	}
}

func TestMADAdversarialNumericalCases(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("DoubleDoubleAccumulationDrift", func(t *testing.T) {
		t.Parallel()
		base := math.Ldexp(1.0, 48)
		items := make([][]float64, 20)
		for i := range items {
			items[i] = []float64{0.03125, 0.03125, 0.03125}
		}
		prob := Problem{
			Baselines:   [][]float64{{base, base, base}},
			Items:       items,
			Step:        0.125,
			RoundWeight: 1,
		}
		wantStep := (int64(1) << 51) + 5
		for _, opts := range [][]Option{nil, {WithCertified(true)}} {
			res, err := Solve(ctx, prob, opts...)
			if err != nil {
				t.Fatalf("Solve: %v", err)
			}
			if res.Steps[0] != wantStep {
				t.Fatalf("Steps[0] = %d, want %d", res.Steps[0], wantStep)
			}
			if res.Objective != 0 {
				t.Fatalf("Objective = %v, want 0", res.Objective)
			}
		}
	})

	t.Run("LargeNonIntegerTwoProd", func(t *testing.T) {
		t.Parallel()
		step := 0.3 * math.Ldexp(1.0, 48)
		// Construct period values bracketing 3*step where float64(step*3) is inexact.
		p3 := twoProd(step, 3)
		prob := Problem{
			Baselines: [][]float64{
				{p3.hi, p3.hi + 0.25*math.Ldexp(1.0, 48), p3.hi},
			},
			Items: [][]float64{
				{0, 0, 0},
			},
			Step:        step,
			RoundWeight: 1,
		}
		wantRat := bigRatEvalAssignment(t, prob, []int{0})
		wantObj, _ := wantRat.Float64()

		res, err := Solve(ctx, prob)
		if err != nil {
			t.Fatalf("Solve: %v", err)
		}
		certRes, err := Solve(ctx, prob, WithCertified(true))
		if err != nil {
			t.Fatalf("Solve certified: %v", err)
		}
		if res.Steps[0] != 3 || certRes.Steps[0] != 3 {
			t.Fatalf("Steps = %v / %v, want [3]", res.Steps, certRes.Steps)
		}
		if res.Objective != wantObj || certRes.Objective != wantObj {
			t.Fatalf("Objective = %v / %v, want %v", res.Objective, certRes.Objective, wantObj)
		}
	})

	t.Run("SubULPRoundWeight", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines:   [][]float64{{10, 20}},
			Step:        100,
			RoundWeight: 1e-17,
		}
		res, err := Solve(ctx, prob)
		if err != nil {
			t.Fatalf("Solve: %v", err)
		}
		certRes, err := Solve(ctx, prob, WithCertified(true))
		if err != nil {
			t.Fatalf("Solve certified: %v", err)
		}
		if res.Levels[0] != 20 || certRes.Levels[0] != 20 {
			t.Fatalf("Levels = %v / %v, want [20]", res.Levels, certRes.Levels)
		}
		if res.Steps[0] != 1 || certRes.Steps[0] != 1 {
			t.Fatalf("Steps = %v / %v, want [1]", res.Steps, certRes.Steps)
		}
	})

	t.Run("NegativeEntriesAndClampedMedian", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines: [][]float64{
				{-50, -40, 10, 20},
				{-10, -20, -30, -40},
			},
			Items: [][]float64{
				{5, 5, -15, 0},
				{-5, 10, 5, -5},
			},
			Step:          10,
			RoundWeight:   1.5,
			AllowZeroStep: true,
		}
		dfsRes, err := Solve(ctx, prob, WithStrategy(StrategyBruteForce))
		if err != nil {
			t.Fatalf("DFS: %v", err)
		}
		dpRes, err := Solve(ctx, prob, WithStrategy(StrategyDP))
		if err != nil {
			t.Fatalf("DP: %v", err)
		}
		certRes, err := Solve(ctx, prob, WithCertified(true))
		if err != nil {
			t.Fatalf("Certified: %v", err)
		}
		wantRat := bigRatEvalAssignment(t, prob, certRes.Assignment)
		wantObj, _ := wantRat.Float64()
		if dfsRes.Objective != wantObj || dpRes.Objective != wantObj || certRes.Objective != wantObj {
			t.Fatalf("mismatch: dfs=%v dp=%v cert=%v want=%v", dfsRes.Objective, dpRes.Objective, certRes.Objective, wantObj)
		}
	})
}

func TestMADDesignatedItemsAndParity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	items := [][]float64{
		{10, 20, 30},
		{40, 10, 5},
		{5, 35, 15},
		{25, 15, 25},
	}

	t.Run("UnsortedDesignatedNilBaselines", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Groups:          2,
			Items:           items,
			Step:            10,
			RoundWeight:     2,
			DesignatedItems: []int{3, 0},
		}
		assertParityAllEngines(ctx, t, prob)
	})

	t.Run("UnsortedDesignatedDistinctBaselines", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines: [][]float64{
				{100, 10, 5},
				{5, 50, 90},
			},
			Items:           items,
			Step:            10,
			RoundWeight:     2,
			DesignatedItems: []int{3, 0},
		}
		assertParityAllEngines(ctx, t, prob)
	})

	t.Run("StreamingDPManyDistinctBaselines", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines: [][]float64{
				{10, 20, 30, 40},
				{40, 30, 20, 10},
				{15, 15, 15, 15},
				{5, 50, 5, 50},
				{25, 25, 10, 10},
				{100, 0, 100, 0},
				{12, 18, 24, 30},
				{33, 22, 11, 44},
			},
			Items: [][]float64{
				{17, 3, 29, 8},
				{4, 41, 12, 19},
				{22, 9, 31, 6},
			},
			Step:        10,
			RoundWeight: 1.5,
		}
		assertParityAllEngines(ctx, t, prob)
	})

	t.Run("UniformBaselineMoreGroupsThanItems", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines: [][]float64{
				{15, 25, 35},
				{15, 25, 35},
				{15, 25, 35},
				{15, 25, 35},
				{15, 25, 35},
			},
			Items: [][]float64{
				{10, 0, 5},
				{0, 15, 10},
			},
			Step:        10,
			RoundWeight: 1,
		}
		assertParityAllEngines(ctx, t, prob)
	})
}

func assertParityAllEngines(ctx context.Context, t *testing.T, prob Problem) {
	t.Helper()
	dfsRes, err := Solve(ctx, prob, WithStrategy(StrategyBruteForce))
	if err != nil {
		t.Fatalf("DFS: %v", err)
	}
	dpRes, err := Solve(ctx, prob, WithStrategy(StrategyDP))
	if err != nil {
		t.Fatalf("DP: %v", err)
	}
	certRes, err := Solve(ctx, prob, WithCertified(true))
	if err != nil {
		t.Fatalf("Certified: %v", err)
	}

	wantRat := bigRatEvalAssignment(t, prob, certRes.Assignment)
	wantObj, _ := wantRat.Float64()
	if dfsRes.Objective != wantObj || dpRes.Objective != wantObj || certRes.Objective != wantObj {
		t.Fatalf("parity mismatch: dfs=%v dp=%v cert=%v want=%v", dfsRes.Objective, dpRes.Objective, certRes.Objective, wantObj)
	}
	if certRes.BestBound > wantObj {
		t.Fatalf("BestBound %v exceeds exact optimum %v", certRes.BestBound, wantObj)
	}
	if len(prob.DesignatedItems) > 0 {
		for _, res := range []Result{dfsRes, dpRes, certRes} {
			used := make([]bool, len(prob.DesignatedItems))
			for _, item := range prob.DesignatedItems {
				g := res.Assignment[item]
				if used[g] {
					t.Fatalf("DesignatedItems bijection violated: %v", res.Assignment)
				}
				used[g] = true
			}
		}
	}
}

func TestMADEdgeCasesAndValidation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("ZeroItemsFastPath", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines: [][]float64{
				{10, 20, 30},
				{10, 20, 30},
				{40, 50, 60},
			},
			Step:        10,
			RoundWeight: 1,
		}
		res, err := Solve(ctx, prob)
		if err != nil {
			t.Fatalf("Solve: %v", err)
		}
		if len(res.Assignment) != 0 || res.Status != StatusOptimal || res.Objective != 60 {
			t.Fatalf("unexpected zero-items result: %+v", res)
		}
	})

	t.Run("SingleGroupLargeItemCountFastPath", func(t *testing.T) {
		t.Parallel()
		items := make([][]float64, 50)
		for i := range items {
			items[i] = []float64{1, 2, 3, 4}
		}
		prob := Problem{
			Groups:      1,
			Items:       items,
			Step:        25,
			RoundWeight: 2,
		}
		res, err := Solve(ctx, prob)
		if err != nil {
			t.Fatalf("Solve(G=1, I=50): %v", err)
		}
		if res.Status != StatusOptimal || len(res.Assignment) != 50 || res.Steps[0] != 4 || res.Objective != 200 {
			t.Fatalf("unexpected G=1 result: %+v", res)
		}
	})

	t.Run("OddEvenTAndUnroundedSentinel", func(t *testing.T) {
		t.Parallel()
		for tLen := 1; tLen <= 6; tLen++ {
			row := make([]float64, tLen)
			for idx := range tLen {
				row[idx] = float64(idx*10 + 1)
			}
			probZeroStep := Problem{
				Baselines:   [][]float64{row},
				Step:        0,
				RoundWeight: 5,
			}
			r0, err := Solve(ctx, probZeroStep)
			if err != nil || r0.Steps[0] != 0 {
				t.Fatalf("T=%d Step=0: res=%+v err=%v", tLen, r0, err)
			}

			probZeroWeight := Problem{
				Baselines:   [][]float64{row},
				Step:        50,
				RoundWeight: 0,
			}
			r1, err := Solve(ctx, probZeroWeight)
			if err != nil || r1.Steps[0] != 0 {
				t.Fatalf("T=%d RoundWeight=0: res=%+v err=%v", tLen, r1, err)
			}
		}
	})

	t.Run("AllowZeroStep", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines:     [][]float64{{1, 2, 3}},
			Step:          100,
			RoundWeight:   5,
			AllowZeroStep: true,
		}
		rZero, err := Solve(ctx, prob)
		if err != nil || rZero.Steps[0] != 0 {
			t.Fatalf("AllowZeroStep=true: res=%+v err=%v", rZero, err)
		}
		prob.AllowZeroStep = false
		rOne, err := Solve(ctx, prob)
		if err != nil || rOne.Steps[0] != 1 {
			t.Fatalf("AllowZeroStep=false: res=%+v err=%v", rOne, err)
		}
	})

	t.Run("TransposePeriodWeights", func(t *testing.T) {
		t.Parallel()
		got, err := TransposePeriodWeights([][]float64{{1, 2, 3}, {4, 5, 6}})
		if err != nil {
			t.Fatalf("TransposePeriodWeights: %v", err)
		}
		want := [][]float64{{1, 4}, {2, 5}, {3, 6}}
		if !slices.EqualFunc(got, want, slices.Equal) {
			t.Fatalf("TransposePeriodWeights = %v, want %v", got, want)
		}
		emptyItems, err := TransposePeriodWeights([][]float64{{}, {}})
		if err != nil || len(emptyItems) != 0 {
			t.Fatalf("TransposePeriodWeights empty items = (%v, %v)", emptyItems, err)
		}
		if _, err := TransposePeriodWeights(nil); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("empty matrix err = %v, want ErrInvalidProblem", err)
		}
		if _, err := TransposePeriodWeights([][]float64{{1, 2}, {3}}); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("ragged matrix err = %v, want ErrInvalidProblem", err)
		}
	})

	t.Run("InvalidProblemCases", func(t *testing.T) {
		t.Parallel()
		badProblems := []Problem{
			{Groups: 2},
			{Groups: 0, Items: [][]float64{{1}}},
			{Groups: 2, Baselines: [][]float64{{1}}, Items: [][]float64{{1}}},
			{Baselines: [][]float64{{1, 2}, {3}}},
			{Groups: 1, Items: [][]float64{{1, 2}, {3}}},
			{Groups: 1, Items: [][]float64{{math.NaN()}}},
			{Baselines: [][]float64{{math.Inf(1)}}},
			{Groups: 1, Items: [][]float64{{1}}, Step: -1},
			{Groups: 1, Items: [][]float64{{1}}, Step: 1e16},
			{Groups: 1, Items: [][]float64{{1}}, RoundWeight: -1},
			{Groups: 1, Items: [][]float64{{1}}, RoundWeight: 1e16},
			{Groups: 1, Items: [][]float64{{1e15 + 1e10}}},
			{Groups: 1, Items: [][]float64{{1e15}}, Step: 1e-5, RoundWeight: 1},
			{Groups: 2, Items: [][]float64{{1}, {2}}, DesignatedItems: []int{0}},
			{Groups: 2, Items: [][]float64{{1}, {2}}, DesignatedItems: []int{0, 2}},
			{Groups: 2, Items: [][]float64{{1}, {2}}, DesignatedItems: []int{1, 1}},
		}
		for idx, bp := range badProblems {
			if err := ValidateProblem(bp); !errors.Is(err, ErrInvalidProblem) {
				t.Fatalf("badProblems[%d]: err = %v, want ErrInvalidProblem", idx, err)
			}
			if _, err := Solve(ctx, bp); !errors.Is(err, ErrInvalidProblem) {
				t.Fatalf("Solve(badProblems[%d]): err = %v, want ErrInvalidProblem", idx, err)
			}
		}
		if _, err := Solve(ctx, Problem{Groups: 1, Items: [][]float64{{1}}}, WithStrategy(Strategy(99))); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("invalid strategy should return ErrInvalidProblem")
		}

		// Fast uBound overshoot must fall back to exact uMax before rejecting valid step bounds.
		exactFallbackOK := Problem{
			Baselines:   [][]float64{{1e14, 0}},
			Items:       [][]float64{{0, 1e14}},
			Step:        0.03125,
			RoundWeight: 1,
		}
		if err := ValidateProblem(exactFallbackOK); err != nil {
			t.Fatalf("exact uMax fallback should pass ValidateProblem, got %v", err)
		}
		overScale := Problem{
			Baselines: [][]float64{{9.95e14, 0}},
			Items:     [][]float64{{1e13, 0}},
		}
		if err := ValidateProblem(overScale); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("overScale should return ErrInvalidProblem, got %v", err)
		}
	})

	t.Run("CombinatorialLimitAndHooks", func(t *testing.T) {
		t.Parallel()
		items := make([][]float64, MaxDPItems+1)
		for i := range items {
			items[i] = []float64{float64(i + 1), 2}
		}
		prob := Problem{Groups: 2, Items: items}
		if _, err := Solve(ctx, prob); !errors.Is(err, ErrCombinatorialLimit) {
			t.Fatalf("over MaxDPItems without fallback: err=%v, want ErrCombinatorialLimit", err)
		}

		called := false
		sentinel := Result{Objective: 42, Strategy: StrategyMILP, Status: StatusOptimal}
		hook := func(_ context.Context, _ Problem, opt Options) (Result, error) {
			called = true
			if !opt.HasFallback() {
				t.Fatalf("expected HasFallback() == true inside hook")
			}
			return sentinel, nil
		}
		res, err := Solve(ctx, prob, WithFallback(hook))
		if err != nil || !called || res.Objective != 42 {
			t.Fatalf("fallback res=(%+v, %v) called=%v", res, err, called)
		}

		// WithOptions preserves pre-installed fallback hooks while leaving nil cert unset.
		cfg := ApplyOptions(
			WithFallback(hook),
			WithOptions(Options{RelGapTol: 1e-4, MaxNodes: 100}),
		)
		if !cfg.HasFallback() || !cfg.RelGapTolSet() || !cfg.MaxNodesSet() || cfg.AbsGapTolSet() || cfg.MaxIterSet() {
			t.Fatalf("unexpected option flags: %+v", cfg)
		}
	})

	t.Run("TimeLimitAndContextCancellation", func(t *testing.T) {
		t.Parallel()
		prob := pmadBaseProblem()
		res, err := Solve(ctx, prob, WithTimeLimit(-time.Millisecond))
		if err != nil {
			t.Fatalf("negative TimeLimit err = %v", err)
		}
		if res.Status != StatusTimeLimit || res.BestBound != 0 || len(res.Assignment) != 2 {
			t.Fatalf("unexpected TimeLimit result: %+v", res)
		}

		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := Solve(cctx, prob); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled ctx err = %v, want context.Canceled", err)
		}

		// GreedyIncumbent produces valid designated assignment.
		gProb := Problem{
			Baselines: [][]float64{{10, 20}, {30, 40}},
			Items:     [][]float64{{1, 2}, {3, 4}, {5, 6}, {7, 8}},
			DesignatedItems: []int{
				3, 0,
			},
			Step:        10,
			RoundWeight: 1,
		}
		gRes, err := GreedyIncumbent(gProb)
		if err != nil {
			t.Fatalf("GreedyIncumbent: %v", err)
		}
		if err := validateAssignment(gProb, 2, 4, gRes.Assignment); err != nil {
			t.Fatalf("GreedyIncumbent invalid assignment: %v", err)
		}
	})
}

func TestMADSubpackageZeroForbiddenImportsAST(t *testing.T) {
	t.Parallel()

	prodFiles := []string{"mad.go", "oracle.go", "solver.go", "certified.go"}
	fset := token.NewFileSet()
	for _, name := range prodFiles {
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == "math/big" || strings.HasPrefix(p, "encoding/json") || strings.Contains(p, "paulourio/gob"+"lin") {
				t.Fatalf("%s imports forbidden package %q", name, p)
			}
		}
	}
}

func TestMADLargeGroupsNoPanic(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	for _, g := range []int{1 << 60, maxProblemGroups + 1} {
		prob := Problem{
			Groups: g,
			Items:  [][]float64{{1}, {1}},
		}
		if err := ValidateProblem(prob); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("G=%d ValidateProblem = %v, want ErrInvalidProblem", g, err)
		}
		for _, s := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce, StrategyMILP} {
			if _, err := Solve(ctx, prob, WithStrategy(s)); !errors.Is(err, ErrInvalidProblem) {
				t.Fatalf("G=%d Solve(%v) = %v, want ErrInvalidProblem", g, s, err)
			}
		}
		if _, err := GreedyIncumbent(prob); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("G=%d GreedyIncumbent = %v, want ErrInvalidProblem", g, err)
		}
		if _, err := EvaluateAssignment(prob, []int{0, 1}); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("G=%d EvaluateAssignment = %v, want ErrInvalidProblem", g, err)
		}
	}
}

func TestMADNilBaselines100kGroupsNoOOMAndBudget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const (
		numGroups  = 100_000
		numItems   = 5
		numPeriods = 8760
	)
	items := make([][]float64, numItems)
	for i := range items {
		row := make([]float64, numPeriods)
		for t := range numPeriods {
			row[t] = float64(100 + ((i*131 + t*17) % 1900))
		}
		items[i] = row
	}
	prob := Problem{
		Groups:      numGroups,
		Items:       items,
		Step:        100,
		RoundWeight: 6,
	}

	if err := ValidateProblem(prob); err != nil {
		t.Fatalf("ValidateProblem(100k nil baselines): %v", err)
	}

	autoRes, err := Solve(ctx, prob)
	if err != nil {
		t.Fatalf("Solve Auto: %v", err)
	}
	if autoRes.Status != StatusOptimal || len(autoRes.Levels) != numGroups || len(autoRes.Steps) != numGroups {
		t.Fatalf("unexpected Auto result: status=%v len(Levels)=%d len(Steps)=%d", autoRes.Status, len(autoRes.Levels), len(autoRes.Steps))
	}

	budgetStart := time.Now()
	budgetRes, err := Solve(ctx, prob, WithTimeLimit(100*time.Millisecond))
	budgetElapsed := time.Since(budgetStart)
	if err != nil {
		t.Fatalf("Solve Auto(100ms): %v", err)
	}
	if budgetRes.Status != StatusOptimal && (budgetRes.Status != StatusTimeLimit || budgetRes.BestBound != 0) {
		t.Fatalf("unexpected Auto(100ms) result: %+v", budgetRes)
	}
	if budgetElapsed > 2*time.Second {
		t.Fatalf("Solve Auto(100ms) overran time limit: %v", budgetElapsed)
	}

	dpRes, err := Solve(ctx, prob, WithStrategy(StrategyDP))
	if err != nil {
		t.Fatalf("Solve DP: %v", err)
	}
	if dpRes.Status != StatusOptimal || dpRes.Objective != autoRes.Objective {
		t.Fatalf("DP result mismatch: status=%v obj=%v want=%v", dpRes.Status, dpRes.Objective, autoRes.Objective)
	}

	evalRes, err := EvaluateAssignment(prob, autoRes.Assignment)
	if err != nil || evalRes.Objective != autoRes.Objective {
		t.Fatalf("EvaluateAssignment = (%v, %v), want %v", evalRes.Objective, err, autoRes.Objective)
	}

	gRes, err := GreedyIncumbent(prob)
	if err != nil {
		t.Fatalf("GreedyIncumbent: %v", err)
	}
	gEval, err := EvaluateAssignment(prob, gRes.Assignment)
	if err != nil || gEval.Objective != gRes.Objective {
		t.Fatalf("GreedyIncumbent eval mismatch: g=%v eval=%v err=%v", gRes.Objective, gEval.Objective, err)
	}

	if _, err := Solve(ctx, prob, WithStrategy(StrategyBruteForce)); !errors.Is(err, ErrCombinatorialLimit) {
		t.Fatalf("BruteForce err = %v, want ErrCombinatorialLimit", err)
	}

	tlRes, err := Solve(ctx, prob, WithTimeLimit(time.Nanosecond))
	if err != nil {
		t.Fatalf("Solve(1ns) err = %v", err)
	}
	if tlRes.Status != StatusTimeLimit || tlRes.BestBound != 0 || len(tlRes.Assignment) != numItems {
		t.Fatalf("unexpected 1ns timeout result: %+v", tlRes)
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Solve(cctx, prob); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Solve err = %v, want context.Canceled", err)
	}
}

func TestMADLargeBaselinesCapRejectionAndGreedyOnTheFly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	makeProb := func(numGroups, numItems, numPeriods int, distinct bool) Problem {
		baselines := make([][]float64, numGroups)
		shared0 := make([]float64, numPeriods)
		shared1 := make([]float64, numPeriods)
		for t := range numPeriods {
			shared0[t] = float64(50 + (t % 100))
			shared1[t] = float64(80 + (t % 100))
		}
		for g := range numGroups {
			if !distinct {
				if g == 0 {
					baselines[g] = shared0
				} else {
					baselines[g] = shared1
				}
				continue
			}
			row := make([]float64, numPeriods)
			copy(row, shared0)
			row[0] = float64(100 + g)
			baselines[g] = row
		}
		items := make([][]float64, numItems)
		for i := range numItems {
			row := make([]float64, numPeriods)
			for t := range numPeriods {
				row[t] = float64(10 + ((i + t) % 50))
			}
			items[i] = row
		}
		return Problem{
			Baselines:   baselines,
			Items:       items,
			Step:        100,
			RoundWeight: 2,
		}
	}

	for _, tc := range []struct {
		name     string
		distinct bool
	}{
		{"U2", false},
		{"U10000", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prob := makeProb(10_000, 5, 1000, tc.distinct)
			for _, s := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce} {
				_, err := Solve(ctx, prob, WithStrategy(s))
				if !errors.Is(err, ErrCombinatorialLimit) {
					t.Fatalf("%s Solve(%v) err = %v, want ErrCombinatorialLimit", tc.name, s, err)
				}
			}
			called := false
			fb := func(_ context.Context, _ Problem, _ Options) (Result, error) {
				called = true
				return Result{Status: StatusOptimal, Strategy: StrategyMILP}, nil
			}
			res, err := Solve(ctx, prob, WithFallback(fb))
			if err != nil || !called || res.Strategy != StrategyMILP {
				t.Fatalf("%s fallback = (%+v, %v) called=%v", tc.name, res, err, called)
			}
		})
	}

	// Exercise GreedyIncumbent on-the-fly path (G * T * 16 > MaxDPBytes).
	greedyProb := makeProb(3000, 2, 1000, true)
	gRes, err := GreedyIncumbent(greedyProb)
	if err != nil {
		t.Fatalf("GreedyIncumbent on-the-fly: %v", err)
	}
	evalRes, err := EvaluateAssignment(greedyProb, gRes.Assignment)
	if err != nil || evalRes.Objective != gRes.Objective {
		t.Fatalf("GreedyIncumbent on-the-fly parity: g=%v eval=%v err=%v", gRes.Objective, evalRes.Objective, err)
	}
}

func makeAliasedRows(numRows, numPeriods, distinct int, offset float64) [][]float64 {
	pool := make([][]float64, distinct)
	for d := range distinct {
		row := make([]float64, numPeriods)
		for t := range numPeriods {
			row[t] = offset + float64((d*37+t*13)%1000)
		}
		pool[d] = row
	}
	rows := make([][]float64, numRows)
	for r := range numRows {
		rows[r] = pool[r%distinct]
	}
	return rows
}

func assertValidTimeLimitResult(t *testing.T, prob Problem, numGroups, numItems int, res Result) {
	t.Helper()
	if res.Status != StatusTimeLimit {
		t.Fatalf("Status = %v, want StatusTimeLimit", res.Status)
	}
	if res.BestBound != 0 {
		t.Fatalf("BestBound = %v, want 0", res.BestBound)
	}
	if res.Assignment == nil || len(res.Assignment) != numItems {
		t.Fatalf("Assignment len = %d (nil=%v), want %d", len(res.Assignment), res.Assignment == nil, numItems)
	}
	if err := validateAssignment(prob, numGroups, numItems, res.Assignment); err != nil {
		t.Fatalf("invalid Assignment: %v", err)
	}
	if len(res.Levels) != numGroups || len(res.Steps) != numGroups {
		t.Fatalf("Levels/Steps len = %d/%d, want %d", len(res.Levels), len(res.Steps), numGroups)
	}
	for g := range numGroups {
		if math.IsNaN(res.Levels[g]) || math.IsInf(res.Levels[g], 0) || res.Levels[g] < 0 {
			t.Fatalf("Levels[%d] = %v, want finite non-negative", g, res.Levels[g])
		}
		if res.Steps[g] < 0 {
			t.Fatalf("Steps[%d] = %d, want non-negative", g, res.Steps[g])
		}
	}
	if math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) || res.Objective < 0 {
		t.Fatalf("Objective = %v, want finite non-negative", res.Objective)
	}
}

type largeTimeLimitCase struct {
	name      string
	prob      Problem
	numGroups int
	numItems  int
}

func largeNegativeTimeLimitCases() []largeTimeLimitCase {
	des := make([]int, 500)
	for g := range des {
		des[g] = 599 - g
	}
	return []largeTimeLimitCase{
		{
			name: "Baselines5000x25x500",
			prob: Problem{
				Baselines:   makeAliasedRows(5000, 500, 64, 50),
				Items:       makeAliasedRows(25, 500, 25, 10),
				Step:        100,
				RoundWeight: 2,
			},
			numGroups: 5000,
			numItems:  25,
		},
		{
			name: "NilBaselines100kx1000x100",
			prob: Problem{
				Groups:      100_000,
				Items:       makeAliasedRows(1000, 100, 64, 15),
				Step:        100,
				RoundWeight: 2,
			},
			numGroups: 100_000,
			numItems:  1000,
		},
		{
			name: "DesignatedBaselines500x600x200",
			prob: Problem{
				Baselines:       makeAliasedRows(500, 200, 32, 40),
				Items:           makeAliasedRows(600, 200, 64, 12),
				Step:            100,
				RoundWeight:     2,
				DesignatedItems: des,
			},
			numGroups: 500,
			numItems:  600,
		},
	}
}

func TestMADNegativeTimeLimitAndBudgetPaths(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("NegativeTimeLimitLargeProblems", func(t *testing.T) {
		t.Parallel()
		for _, tc := range largeNegativeTimeLimitCases() {
			for _, strategy := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce} {
				start := time.Now()
				res, err := Solve(ctx, tc.prob, WithStrategy(strategy), WithTimeLimit(-time.Nanosecond))
				elapsed := time.Since(start)
				if err != nil {
					t.Fatalf("%s (%v): err = %v", tc.name, strategy, err)
				}
				assertValidTimeLimitResult(t, tc.prob, tc.numGroups, tc.numItems, res)
				if elapsed > 2*time.Second {
					t.Fatalf("%s (%v): negative TimeLimit took %v, want < 2s", tc.name, strategy, elapsed)
				}
			}
		}
	})

	t.Run("ZeroItemsTimeLimitAndCancel", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Baselines:   makeAliasedRows(100_000, 8760, 100, 100),
			Step:        100,
			RoundWeight: 6,
		}

		for _, strategy := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce} {
			start := time.Now()
			res, err := Solve(ctx, prob, WithStrategy(strategy), WithTimeLimit(-time.Nanosecond))
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("I=0 negative TL (%v): err = %v", strategy, err)
			}
			assertValidTimeLimitResult(t, prob, 100_000, 0, res)
			if elapsed > 2*time.Second {
				t.Fatalf("I=0 negative TL (%v) took %v, want < 2s", strategy, elapsed)
			}
		}

		tlStart := time.Now()
		tlRes, err := Solve(ctx, prob, WithTimeLimit(20*time.Millisecond))
		tlElapsed := time.Since(tlStart)
		if err != nil {
			t.Fatalf("I=0 20ms TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 100_000, 0, tlRes)
		if tlElapsed > time.Second {
			t.Fatalf("I=0 20ms TL overran budget: %v", tlElapsed)
		}

		tCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		cStart := time.Now()
		if _, err := Solve(tCtx, prob); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("I=0 ctx timeout err = %v, want DeadlineExceeded", err)
		}
		if time.Since(cStart) > time.Second {
			t.Fatalf("I=0 ctx timeout overran budget: %v", time.Since(cStart))
		}

		var ops uint64
		if _, err := dedupBaselinesBudget(prob.Baselines, 100_000, time.Now(), true, nil, nil, &ops); !errors.Is(err, errTimeLimit) {
			t.Fatalf("dedupBaselinesBudget expired err = %v, want errTimeLimit", err)
		}
		ops = 0
		cCtx, cancelNow := context.WithCancel(ctx)
		cancelNow()
		if _, err := dedupBaselinesBudget(prob.Baselines, 100_000, time.Time{}, false, cCtx.Done(), cCtx.Err, &ops); !errors.Is(err, context.Canceled) {
			t.Fatalf("dedupBaselinesBudget canceled err = %v, want Canceled", err)
		}
	})

	t.Run("SingleGroupLargeItemsTimeLimitAndCancel", func(t *testing.T) {
		t.Parallel()
		prob := Problem{
			Groups:      1,
			Items:       makeAliasedRows(20_000, 1000, 50, 10),
			Step:        100,
			RoundWeight: 2,
		}

		negStart := time.Now()
		negRes, err := Solve(ctx, prob, WithTimeLimit(-time.Nanosecond))
		negElapsed := time.Since(negStart)
		if err != nil {
			t.Fatalf("G=1 negative TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 1, 20_000, negRes)
		if negElapsed > 2*time.Second {
			t.Fatalf("G=1 negative TL took %v, want < 2s", negElapsed)
		}

		tlStart := time.Now()
		tlRes, err := Solve(ctx, prob, WithTimeLimit(10*time.Millisecond))
		tlElapsed := time.Since(tlStart)
		if err != nil {
			t.Fatalf("G=1 10ms TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 1, 20_000, tlRes)
		if tlElapsed > time.Second {
			t.Fatalf("G=1 10ms TL overran budget: %v", tlElapsed)
		}

		tCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		cStart := time.Now()
		if _, err := Solve(tCtx, prob); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("G=1 ctx timeout err = %v, want DeadlineExceeded", err)
		}
		if time.Since(cStart) > time.Second {
			t.Fatalf("G=1 ctx timeout overran budget: %v", time.Since(cStart))
		}
	})
}
