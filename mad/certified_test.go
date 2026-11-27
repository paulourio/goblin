package mad

import (
	"context"
	"errors"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func withMADCertFault(ctx context.Context, fn madFaultFunc) context.Context {
	return context.WithValue(ctx, certFaultKey{}, fn)
}

func limbsToBigRat(limbs []uint64, exp int) *big.Rat {
	sum := new(big.Int)
	for _, w := range slices.Backward(limbs) {
		sum.Lsh(sum, 64)
		sum.Or(sum, new(big.Int).SetUint64(w))
	}
	num := new(big.Int).Set(sum)
	den := big.NewInt(1)
	if exp >= 0 {
		num.Lsh(num, uint(exp))
	} else {
		den.Lsh(den, uint(-exp))
	}
	return new(big.Rat).SetFrac(num, den)
}

func TestCertifiedMADDyadicArithmeticEquivalenceWithBigRat(t *testing.T) {
	t.Parallel()

	samples := []float64{
		0,
		math.Copysign(0, -1),
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		1e-310,
		-1e-310,
		0.1,
		-0.3,
		1.0,
		-1.0,
		math.Ldexp(1.0, -53),
		1.0 + math.Ldexp(1.0, -52),
		1e14,
		-1e14,
	}

	for _, v := range samples {
		neg, mant, exp, isZero := decomposeFloat64(v)
		r := new(big.Rat).SetFloat64(v)
		if isZero {
			if r.Sign() != 0 {
				t.Fatalf("decomposeFloat64(%v) reported zero for non-zero rat %v", v, r)
			}
			continue
		}
		got := new(big.Rat).SetInt(new(big.Int).SetUint64(mant))
		if exp >= 0 {
			got.Mul(got, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(exp))))
		} else {
			got.Quo(got, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(-exp))))
		}
		if neg {
			got.Neg(got)
		}
		if got.Cmp(r) != 0 {
			t.Fatalf("decomposeFloat64(%v) = %v, want %v", v, got, r)
		}
	}

	// Verify limbsMagToFloat64 and limbsMagToFloat64Floor on halfway ties and subnormals.
	tieProb := Problem{
		Baselines:   [][]float64{{1.0, math.Ldexp(1.0, -53), math.SmallestNonzeroFloat64}},
		Step:        0,
		RoundWeight: 0,
	}
	ev, err := EvaluateAssignmentDyadic(tieProb, []int{})
	if err != nil {
		t.Fatalf("EvaluateAssignmentDyadic: %v", err)
	}
	exactRat := limbsToBigRat(ev.Limbs, ev.Exp)
	wantRat := bigRatEvalAssignment(t, tieProb, []int{})
	if exactRat.Cmp(wantRat) != 0 {
		t.Fatalf("dyadic exact = %v, want %v", exactRat, wantRat)
	}
	rf, _ := wantRat.Float64()
	if ev.Result.Objective != rf {
		t.Fatalf("Objective = %.17g, want %.17g", ev.Result.Objective, rf)
	}
	boundRat := new(big.Rat).SetFloat64(ev.Result.BestBound)
	if boundRat.Cmp(exactRat) > 0 {
		t.Fatalf("BestBound %.17g exceeds exact rational %v", ev.Result.BestBound, exactRat)
	}
}

func TestCertifiedMADParityAcrossCases(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	items50 := make([][]float64, 50)
	for i := range items50 {
		items50[i] = []float64{0.1 * float64((i%5)+1), 0.3 * float64((i%3)+1), 0.7}
	}

	cases := []struct {
		name  string
		prob  Problem
		short bool
	}{
		{
			name: "ZeroItems",
			prob: Problem{
				Baselines:   [][]float64{{10.1, 20.3, 30.7}, {5.5, 5.5, 5.5}},
				Step:        10,
				RoundWeight: 1.5,
			},
			short: true,
		},
		{
			name: "SingleGroup50Items",
			prob: Problem{
				Groups:      1,
				Items:       items50,
				Step:        0.3,
				RoundWeight: 0.5,
			},
			short: true,
		},
		{
			name: "UnsortedDesignatedNilBaselines",
			prob: Problem{
				Groups:          2,
				Items:           [][]float64{{10, 20, 30}, {40, 10, 5}, {5, 35, 15}, {25, 15, 25}},
				Step:            10,
				RoundWeight:     2,
				DesignatedItems: []int{3, 0},
			},
			short: true,
		},
		{
			name: "UnsortedDesignatedDistinctBaselines",
			prob: Problem{
				Baselines:       [][]float64{{100, 10, 5}, {5, 50, 90}},
				Items:           [][]float64{{10, 20, 30}, {40, 10, 5}, {5, 35, 15}, {25, 15, 25}},
				Step:            10,
				RoundWeight:     2,
				DesignatedItems: []int{3, 0},
			},
			short: true,
		},
		{
			name: "StreamingManyDistinctBaselines",
			prob: Problem{
				Baselines: [][]float64{
					{10, 20, 30},
					{40, 30, 20},
					{15, 15, 15},
					{5, 50, 5},
					{25, 25, 10},
					{100, 0, 100},
				},
				Items: [][]float64{
					{17, 3, 29},
					{4, 41, 12},
					{22, 9, 31},
				},
				Step:        10,
				RoundWeight: 1.5,
			},
			short: true,
		},
		{
			name: "SubnormalInputs",
			prob: Problem{
				Baselines:   [][]float64{{1e-310, 2e-310}, {3e-310, 1e-310}},
				Items:       [][]float64{{5e-311, 1e-310}, {2e-310, 5e-311}},
				Step:        1e-310,
				RoundWeight: 0.5,
			},
			short: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			certRes, err := Solve(ctx, tc.prob, WithCertified(true))
			if err != nil {
				t.Fatalf("Solve certified: %v", err)
			}
			dpRes, err := Solve(ctx, tc.prob)
			if err != nil {
				t.Fatalf("Solve uncertified: %v", err)
			}
			ev, err := EvaluateAssignmentDyadic(tc.prob, certRes.Assignment)
			if err != nil {
				t.Fatalf("EvaluateAssignmentDyadic: %v", err)
			}
			exactRat := limbsToBigRat(ev.Limbs, ev.Exp)
			wantRat := bigRatEvalAssignment(t, tc.prob, certRes.Assignment)
			if exactRat.Cmp(wantRat) != 0 {
				t.Fatalf("dyadic objective %v != big.Rat %v", exactRat, wantRat)
			}
			if certRes.Objective != dpRes.Objective {
				t.Fatalf("certified %v != uncertified %v", certRes.Objective, dpRes.Objective)
			}
			boundRat := new(big.Rat).SetFloat64(certRes.BestBound)
			if boundRat.Cmp(exactRat) > 0 {
				t.Fatalf("BestBound %.17g > exact %v", certRes.BestBound, exactRat)
			}
		})
	}
}

func TestCertifiedMADTenPowFourteenTieStress(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	items := make([][]float64, 14)
	for i := range items {
		items[i] = []float64{0.3, 0.1, 0.4, 0.7}
	}
	prob := Problem{
		Groups:      10,
		Items:       items,
		Step:        0.3,
		RoundWeight: 0.1,
	}

	res, err := Solve(ctx, prob, WithStrategy(StrategyDP), WithCertified(true))
	if err != nil {
		t.Fatalf("Solve 10^14-tie certified DP: %v", err)
	}
	if res.Status != StatusOptimal || res.Strategy != StrategyDP {
		t.Fatalf("unexpected status/strategy: %+v", res)
	}

	ev, err := EvaluateAssignmentDyadic(prob, res.Assignment)
	if err != nil {
		t.Fatalf("EvaluateAssignmentDyadic: %v", err)
	}
	exactRat := limbsToBigRat(ev.Limbs, ev.Exp)
	wantRat := bigRatEvalAssignment(t, prob, res.Assignment)
	if exactRat.Cmp(wantRat) != 0 {
		t.Fatalf("dyadic exact %v != big.Rat %v", exactRat, wantRat)
	}
}

func TestCertifiedMADNegativeControlsFailClosed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	rawCertOpt := func(cfg *Options) {
		cfg.Certified = true
	}

	prob := pmadBaseProblem()
	zeroItemProb := Problem{Baselines: [][]float64{{10, 20}}, Step: 10, RoundWeight: 1}
	singleGroupProb := Problem{Groups: 1, Items: [][]float64{{10, 20}}, Step: 10, RoundWeight: 1}

	for idx, tc := range []struct {
		name string
		p    Problem
		opts []Option
	}{
		{"PMADRawOptions", prob, []Option{WithOptions(Options{Certified: true})}},
		{"PMADMutatedOpt", prob, []Option{rawCertOpt}},
		{"ZeroItemsMutatedOpt", zeroItemProb, []Option{rawCertOpt}},
		{"SingleGroupMutatedOpt", singleGroupProb, []Option{rawCertOpt}},
		{"MILPMutatedOpt", prob, []Option{WithStrategy(StrategyMILP), rawCertOpt}},
		{"NegativeTimeLimitMutatedOpt", prob, []Option{WithTimeLimit(-time.Millisecond), rawCertOpt}},
		{"MILPCertifiedWithoutCertFallback", prob, []Option{
			WithStrategy(StrategyMILP),
			WithFallback(func(_ context.Context, _ Problem, _ Options) (Result, error) {
				return Result{Objective: 1, Status: StatusOptimal}, nil
			}),
			WithCertified(true),
		}},
	} {
		_, err := Solve(ctx, tc.p, tc.opts...)
		if !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("case[%d] %s: err = %v, want ErrCertificationFailed", idx, tc.name, err)
		}
	}

	// Chaining WithCertified(true) after a raw Certified literal installs the hook and succeeds.
	okRes, okErr := Solve(ctx, prob, WithOptions(Options{Certified: true}), WithCertified(true))
	if okErr != nil || okRes.Objective != 24137 {
		t.Fatalf("chained WithCertified(true) = (%+v, %v), want 24137", okRes, okErr)
	}

	// Tampered-witness negative controls via certFaultKey.
	faults := []struct {
		name string
		fn   madFaultFunc
	}{
		{
			name: "SuboptimalAssignment",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Assignment[0] = 0
				res.Assignment[1] = 0
			},
		},
		{
			name: "WrongAssignmentLength",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Assignment = res.Assignment[:1]
			},
		},
		{
			name: "TamperedSteps",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Steps[0]++
			},
		},
		{
			name: "TamperedLevels",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Levels[0] = math.Nextafter(res.Levels[0], math.Inf(1))
			},
		},
		{
			name: "TamperedObjective",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Objective = math.Nextafter(res.Objective, math.Inf(-1))
			},
		},
		{
			name: "TamperedBestBound",
			fn: func(_ Problem, res *Result, _ *error) {
				res.BestBound = math.Nextafter(res.BestBound, math.Inf(1))
			},
		},
		{
			name: "TamperedStatus",
			fn: func(_ Problem, res *Result, _ *error) {
				res.Status = StatusTimeLimit
			},
		},
		{
			name: "TamperedSolveError",
			fn: func(_ Problem, _ *Result, err *error) {
				*err = errors.New("injected solver failure")
			},
		},
	}

	for _, fc := range faults {
		t.Run("Fault_"+fc.name, func(t *testing.T) {
			t.Parallel()
			fCtx := withMADCertFault(ctx, fc.fn)
			_, err := Solve(fCtx, prob, WithCertified(true))
			if !errors.Is(err, ErrCertificationFailed) {
				t.Fatalf("fault %s: err = %v, want ErrCertificationFailed", fc.name, err)
			}
		})
	}
}

func TestCertifiedMADOptionsApplyZeroAllocs(t *testing.T) {
	allocs := testing.AllocsPerRun(50, func() {
		cfg := ApplyOptions()
		if cfg.Certified || cfg.Strategy != StrategyAuto {
			t.Fatalf("unexpected default options: %+v", cfg)
		}
	})
	if allocs != 0 {
		t.Fatalf("ApplyOptions() allocs = %v, want 0", allocs)
	}
}

func TestMADBinaryDeadCodeElimination(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tmp := t.TempDir()

	checkProfile := func(name string, certified bool, wantObj string) {
		t.Helper()
		dir := filepath.Join(tmp, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		mainPath := filepath.Join(dir, "main.go")
		binPath := filepath.Join(dir, name)

		optLine := ""
		if certified {
			optLine = ", mad.WithCertified(true)"
		}
		src := `package main

import (
	"context"
	"fmt"
	"github.com/paulourio/goblin/mad"
)

func main() {
	prob := mad.Problem{
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
		Step:        100,
		RoundWeight: 6,
	}
	res, err := mad.Solve(context.Background(), prob` + optLine + `)
	fmt.Println(res.Objective, err)
}
`
		if err := os.WriteFile(mainPath, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s main.go: %v", name, err)
		}

		buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, mainPath)
		buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", name, err, out)
		}

		runOut, err := exec.CommandContext(ctx, binPath).CombinedOutput()
		if err != nil {
			t.Fatalf("run %s: %v\n%s", name, err, runOut)
		}
		if got := strings.TrimSpace(string(runOut)); got != wantObj {
			t.Fatalf("%s output = %q, want %q", name, got, wantObj)
		}

		nmOut, err := exec.CommandContext(ctx, "go", "tool", "nm", "-size", binPath).CombinedOutput()
		if err != nil {
			t.Fatalf("go tool nm %s: %v\n%s", name, err, nmOut)
		}

		reCert := regexp.MustCompile(`(certifyMAD|defaultMADCertHooks|setMADCertifiedTrue|setMADCertifiedFalse|EvaluateAssignmentDyadic|evalGroupExactDyadic)`)
		reBanned := regexp.MustCompile(`(math/big|encoding/json)`)

		var certSyms, bannedSyms []string
		for line := range strings.SplitSeq(string(nmOut), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			sym := fields[len(fields)-1]
			if reBanned.MatchString(sym) {
				bannedSyms = append(bannedSyms, sym)
			}
			if reCert.MatchString(sym) {
				certSyms = append(certSyms, sym)
			}
		}
		if len(bannedSyms) > 0 {
			t.Fatalf("%s linked math/big or encoding/json symbols (%d): %v", name, len(bannedSyms), bannedSyms[:min(len(bannedSyms), 10)])
		}
		if !certified && len(certSyms) > 0 {
			t.Fatalf("%s uncertified binary linked certification symbols (%d): %v", name, len(certSyms), certSyms[:min(len(certSyms), 10)])
		}
		if certified && len(certSyms) == 0 {
			t.Fatalf("%s certified binary missing certification symbols", name)
		}
	}

	checkProfile("mad_default", false, "24137 <nil>")
	checkProfile("mad_certified", true, "24137 <nil>")
}

func TestCertifiedMADLargeGroupsNoPanic(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	for _, g := range []int{1 << 60, maxProblemGroups + 1} {
		prob := Problem{
			Groups: g,
			Items:  [][]float64{{1}, {1}},
		}
		for _, opts := range [][]Option{
			{WithCertified(true)},
			{WithStrategy(StrategyDP), WithCertified(true)},
			{WithStrategy(StrategyBruteForce), WithCertified(true)},
			{WithStrategy(StrategyMILP), WithCertified(true), WithCertFallback(func(_ context.Context, _ Problem, _ Options) (Result, error) {
				return Result{}, nil
			})},
		} {
			if _, err := Solve(ctx, prob, opts...); !errors.Is(err, ErrInvalidProblem) {
				t.Fatalf("G=%d Solve certified err = %v, want ErrInvalidProblem", g, err)
			}
		}
		if _, err := EvaluateAssignmentDyadic(prob, []int{0, 1}); !errors.Is(err, ErrInvalidProblem) {
			t.Fatalf("G=%d EvaluateAssignmentDyadic err = %v, want ErrInvalidProblem", g, err)
		}
	}
}

func TestCertifiedMADNilBaselines100kGroupsAndBudget(t *testing.T) {
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

	res, err := Solve(ctx, prob, WithCertified(true))
	if err != nil {
		t.Fatalf("Solve certified 100k: %v", err)
	}
	if res.Status != StatusOptimal || len(res.Levels) != numGroups || len(res.Steps) != numGroups {
		t.Fatalf("unexpected certified result: status=%v len(Levels)=%d len(Steps)=%d", res.Status, len(res.Levels), len(res.Steps))
	}

	budgetStart := time.Now()
	budgetRes, err := Solve(ctx, prob, WithCertified(true), WithTimeLimit(100*time.Millisecond))
	budgetElapsed := time.Since(budgetStart)
	if err != nil {
		t.Fatalf("Solve certified 100k (100ms): %v", err)
	}
	if budgetRes.Status != StatusOptimal && (budgetRes.Status != StatusTimeLimit || budgetRes.BestBound != 0) {
		t.Fatalf("unexpected certified 100k (100ms) result: %+v", budgetRes)
	}
	if budgetElapsed > 2*time.Second {
		t.Fatalf("Solve certified 100k (100ms) overran time limit: %v", budgetElapsed)
	}

	ev, err := EvaluateAssignmentDyadic(prob, res.Assignment)
	if err != nil || ev.Result.Objective != res.Objective || ev.Result.BestBound != res.BestBound {
		t.Fatalf("EvaluateAssignmentDyadic mismatch: ev=(%+v, %v) res=(%v, %v)", ev.Result.Objective, err, res.Objective, res.BestBound)
	}

	tlRes, err := Solve(ctx, prob, WithCertified(true), WithTimeLimit(time.Nanosecond))
	if err != nil {
		t.Fatalf("Solve certified 100k (1ns) err = %v", err)
	}
	if tlRes.Status != StatusTimeLimit || tlRes.BestBound != 0 || len(tlRes.Assignment) != numItems {
		t.Fatalf("unexpected 1ns certified timeout result: %+v", tlRes)
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Solve(cctx, prob, WithCertified(true)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled certified Solve err = %v, want context.Canceled", err)
	}
}

func TestCertifiedMADLargeBaselinesCapRejection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	makeProb := func(distinct bool) Problem {
		const (
			numGroups  = 10_000
			numItems   = 5
			numPeriods = 1000
		)
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
			prob := makeProb(tc.distinct)
			for _, s := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce} {
				_, err := Solve(ctx, prob, WithStrategy(s), WithCertified(true))
				if !errors.Is(err, ErrCombinatorialLimit) {
					t.Fatalf("%s certified Solve(%v) err = %v, want ErrCombinatorialLimit", tc.name, s, err)
				}
			}
			called := false
			cf := func(_ context.Context, _ Problem, _ Options) (Result, error) {
				called = true
				return Result{Status: StatusOptimal, Strategy: StrategyMILP}, nil
			}
			res, err := Solve(ctx, prob, WithCertified(true), WithCertFallback(cf))
			if err != nil || !called || res.Strategy != StrategyMILP {
				t.Fatalf("%s certified fallback = (%+v, %v) called=%v", tc.name, res, err, called)
			}
		})
	}
}

func TestCertifiedMADBudgetInVerifyAndEval(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const (
		numGroups  = 500
		numItems   = 2
		numPeriods = 200
	)
	baselines := make([][]float64, numGroups)
	for g := range numGroups {
		row := make([]float64, numPeriods)
		for p := range numPeriods {
			row[p] = float64((g + p) % 100)
		}
		baselines[g] = row
	}
	prob := Problem{
		Baselines:   baselines,
		Items:       [][]float64{make([]float64, numPeriods), make([]float64, numPeriods)},
		Step:        10,
		RoundWeight: 1,
	}

	cctx, cancel := context.WithCancel(ctx)
	fCtx := withMADCertFault(cctx, func(_ Problem, _ *Result, _ *error) {
		cancel()
	})
	if _, err := Solve(fCtx, prob, WithCertified(true)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel before verifyCertifiedResult: err = %v, want context.Canceled", err)
	}
}

func TestCertifiedMADNegativeTimeLimitAndBudgetPaths(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("NegativeTimeLimitLargeProblemsParity", func(t *testing.T) {
		t.Parallel()
		for _, tc := range largeNegativeTimeLimitCases() {
			for _, strategy := range []Strategy{StrategyAuto, StrategyDP, StrategyBruteForce} {
				uncRes, uncErr := Solve(ctx, tc.prob, WithStrategy(strategy), WithTimeLimit(-time.Nanosecond))
				if uncErr != nil {
					t.Fatalf("%s uncertified (%v): err = %v", tc.name, strategy, uncErr)
				}
				start := time.Now()
				certRes, certErr := Solve(ctx, tc.prob, WithStrategy(strategy), WithTimeLimit(-time.Nanosecond), WithCertified(true))
				elapsed := time.Since(start)
				if certErr != nil {
					t.Fatalf("%s certified (%v): err = %v", tc.name, strategy, certErr)
				}
				assertValidTimeLimitResult(t, tc.prob, tc.numGroups, tc.numItems, certRes)
				if certRes.Strategy != uncRes.Strategy || certRes.Status != uncRes.Status || certRes.BestBound != uncRes.BestBound {
					t.Fatalf("%s (%v) mismatch: cert=(%v,%v,%v) unc=(%v,%v,%v)",
						tc.name, strategy, certRes.Strategy, certRes.Status, certRes.BestBound,
						uncRes.Strategy, uncRes.Status, uncRes.BestBound)
				}
				if elapsed > 2*time.Second {
					t.Fatalf("%s certified (%v): negative TimeLimit took %v, want < 2s", tc.name, strategy, elapsed)
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
			res, err := Solve(ctx, prob, WithStrategy(strategy), WithTimeLimit(-time.Nanosecond), WithCertified(true))
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("I=0 certified negative TL (%v): err = %v", strategy, err)
			}
			assertValidTimeLimitResult(t, prob, 100_000, 0, res)
			if elapsed > 2*time.Second {
				t.Fatalf("I=0 certified negative TL (%v) took %v, want < 2s", strategy, elapsed)
			}
		}

		tlStart := time.Now()
		tlRes, err := Solve(ctx, prob, WithTimeLimit(20*time.Millisecond), WithCertified(true))
		tlElapsed := time.Since(tlStart)
		if err != nil {
			t.Fatalf("I=0 certified 20ms TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 100_000, 0, tlRes)
		if tlElapsed > time.Second {
			t.Fatalf("I=0 certified 20ms TL overran budget: %v", tlElapsed)
		}

		tCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		cStart := time.Now()
		if _, err := Solve(tCtx, prob, WithCertified(true)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("I=0 certified ctx timeout err = %v, want DeadlineExceeded", err)
		}
		if time.Since(cStart) > time.Second {
			t.Fatalf("I=0 certified ctx timeout overran budget: %v", time.Since(cStart))
		}

		var ops uint64
		if _, err := buildDyadicSpecBudget(prob, 100_000, 8760, 1000, time.Now(), true, nil, nil, &ops); !errors.Is(err, errTimeLimit) {
			t.Fatalf("buildDyadicSpecBudget expired err = %v, want errTimeLimit", err)
		}
		ops = 0
		cCtx, cancelNow := context.WithCancel(ctx)
		cancelNow()
		if _, err := buildDyadicSpecBudget(prob, 100_000, 8760, 1000, time.Time{}, false, cCtx.Done(), cCtx.Err, &ops); !errors.Is(err, context.Canceled) {
			t.Fatalf("buildDyadicSpecBudget canceled err = %v, want Canceled", err)
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
		negRes, err := Solve(ctx, prob, WithTimeLimit(-time.Nanosecond), WithCertified(true))
		negElapsed := time.Since(negStart)
		if err != nil {
			t.Fatalf("G=1 certified negative TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 1, 20_000, negRes)
		if negElapsed > 2*time.Second {
			t.Fatalf("G=1 certified negative TL took %v, want < 2s", negElapsed)
		}

		tlStart := time.Now()
		tlRes, err := Solve(ctx, prob, WithTimeLimit(10*time.Millisecond), WithCertified(true))
		tlElapsed := time.Since(tlStart)
		if err != nil {
			t.Fatalf("G=1 certified 10ms TL: err = %v", err)
		}
		assertValidTimeLimitResult(t, prob, 1, 20_000, tlRes)
		if tlElapsed > time.Second {
			t.Fatalf("G=1 certified 10ms TL overran budget: %v", tlElapsed)
		}

		tCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		cStart := time.Now()
		if _, err := Solve(tCtx, prob, WithCertified(true)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("G=1 certified ctx timeout err = %v, want DeadlineExceeded", err)
		}
		if time.Since(cStart) > time.Second {
			t.Fatalf("G=1 certified ctx timeout overran budget: %v", time.Since(cStart))
		}
	})
}
