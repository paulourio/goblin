package goblin

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/paulourio/goblin/mad"
)

func rootPMADProblem() MADProblem {
	return MADProblem{
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

func extractMADAssignmentFromModel(t *testing.T, vars MADModelVars, res Result) []int {
	t.Helper()
	numGroups := len(vars.X)
	if numGroups == 0 {
		t.Fatal("MADModelVars.X is empty")
	}
	numItems := len(vars.X[0])
	assign := make([]int, numItems)
	for i := range numItems {
		found := -1
		for g := range numGroups {
			v := res.Value(vars.X[g][i])
			if math.Round(v) == 1 {
				if found >= 0 {
					t.Fatalf("item %d assigned to both group %d and %d", i, found, g)
				}
				found = g
			}
		}
		if found < 0 {
			t.Fatalf("item %d unassigned in MILP result", i)
		}
		assign[i] = found
	}
	return assign
}

func assertDesignatedBijection(t *testing.T, prob MADProblem, assign []int) {
	t.Helper()
	if len(prob.DesignatedItems) == 0 {
		return
	}
	numGroups := prob.Groups
	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
	}
	seen := make([]bool, numGroups)
	for _, item := range prob.DesignatedItems {
		g := assign[item]
		if g < 0 || g >= numGroups {
			t.Fatalf("designated item %d mapped to out-of-range group %d", item, g)
		}
		if seen[g] {
			t.Fatalf("designated items collide on group %d (assign=%v)", g, assign)
		}
		seen[g] = true
	}
}

func assertMADClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	tol := 1e-9 * max(1.0, math.Abs(want))
	if math.Abs(got-want) > tol {
		t.Fatalf("%s objective = %.17g, want %.17g (diff=%g, tol=%g)", label, got, want, math.Abs(got-want), tol)
	}
}

func TestMADPMAD24137RootAndBuildModel(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	prob := rootPMADProblem()

	wantAssign := []int{6, 9}
	wantLevels := []float64{600, 400, 1300, 900, 900, 1100, 1800, 1500, 700, 2200}
	wantSteps := []int64{6, 4, 13, 9, 9, 11, 18, 15, 7, 22}

	configs := []struct {
		name         string
		opts         []MADOption
		wantStrategy MADStrategy
	}{
		{name: "Auto", opts: nil, wantStrategy: MADDP},
		{name: "BruteForce", opts: []MADOption{WithMADStrategy(MADBruteForce)}, wantStrategy: MADBruteForce},
		{name: "DP", opts: []MADOption{WithMADStrategy(MADDP)}, wantStrategy: MADDP},
		{name: "MILP_Go", opts: []MADOption{WithMADStrategy(MADMILP), WithMADBackend(BackendGo)}, wantStrategy: MADMILP},
		{name: "Auto_Certified", opts: []MADOption{WithMADCertified(true)}, wantStrategy: MADDP},
		{name: "MILP_Go_Certified", opts: []MADOption{WithMADStrategy(MADMILP), WithMADBackend(BackendGo), WithMADCertified(true)}, wantStrategy: MADMILP},
	}
	if CBCAvailable() {
		configs = append(configs,
			struct {
				name         string
				opts         []MADOption
				wantStrategy MADStrategy
			}{name: "MILP_CBC", opts: []MADOption{WithMADStrategy(MADMILP), WithMADBackend(BackendCBC)}, wantStrategy: MADMILP},
			struct {
				name         string
				opts         []MADOption
				wantStrategy MADStrategy
			}{name: "MILP_CBC_Certified", opts: []MADOption{WithMADStrategy(MADMILP), WithMADBackend(BackendCBC), WithMADCertified(true)}, wantStrategy: MADMILP},
		)
	}

	for _, tc := range configs {
		t.Run("SolveMAD_"+tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := SolveMAD(ctx, prob, tc.opts...)
			if err != nil {
				t.Fatalf("SolveMAD(%s): %v", tc.name, err)
			}
			if res.Status != MADStatusOptimal {
				t.Fatalf("Status = %v, want %v", res.Status, MADStatusOptimal)
			}
			if res.Strategy != tc.wantStrategy {
				t.Fatalf("Strategy = %v, want %v", res.Strategy, tc.wantStrategy)
			}
			if res.Objective != 24137 {
				t.Fatalf("Objective = %.17g, want 24137", res.Objective)
			}
			if res.BestBound != 24137 {
				t.Fatalf("BestBound = %.17g, want 24137", res.BestBound)
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
		})
	}

	directConfigs := []struct {
		name string
		opts []Option
	}{
		{name: "PresolveOn", opts: []Option{WithPresolve(true)}},
		{name: "PresolveOff", opts: []Option{WithPresolve(false)}},
		{name: "Certified", opts: []Option{WithCertified(true)}},
	}
	if CBCAvailable() {
		directConfigs = append(directConfigs, struct {
			name string
			opts []Option
		}{name: "CBC", opts: []Option{WithCBC()}})
	}

	for _, dc := range directConfigs {
		t.Run("BuildMADModel_"+dc.name, func(t *testing.T) {
			t.Parallel()
			m, vars, err := BuildMADModel(prob)
			if err != nil {
				t.Fatalf("BuildMADModel: %v", err)
			}
			res, err := m.Solve(ctx, dc.opts...)
			if err != nil {
				t.Fatalf("Model.Solve(%s): %v", dc.name, err)
			}
			if res.Status != StatusOptimal {
				t.Fatalf("Status = %v, want Optimal", res.Status)
			}
			assertMADClose(t, dc.name, res.Objective, 24137)
			assign := extractMADAssignmentFromModel(t, vars, res)
			if !slices.Equal(assign, wantAssign) {
				t.Fatalf("Assignment = %v, want %v", assign, wantAssign)
			}
			ev, evalErr := mad.EvaluateAssignment(prob, assign)
			if evalErr != nil {
				t.Fatalf("EvaluateAssignment: %v", evalErr)
			}
			if ev.Objective != 24137 {
				t.Fatalf("EvaluateAssignment Objective = %.17g, want 24137", ev.Objective)
			}
		})
	}
}

func TestMADRootParityAcrossStrategiesAndBackends(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cases := []struct {
		name           string
		prob           MADProblem
		expectDirect   bool
		expectUnround  bool
		skipBruteForce bool
	}{
		{
			name: "RoundWeightEqT_NilBaselines",
			prob: MADProblem{
				Groups: 3,
				Items: [][]float64{
					{15, 22, 18, 25},
					{11, 14, 19, 12},
					{33, 28, 35, 31},
					{9, 13, 8, 11},
					{27, 21, 24, 29},
				},
				Step:        10,
				RoundWeight: 4,
			},
			expectDirect: true,
		},
		{
			name: "RoundWeightGtT_NilBaselines",
			prob: MADProblem{
				Groups: 3,
				Items: [][]float64{
					{15, 22, 18, 25},
					{11, 14, 19, 12},
					{33, 28, 35, 31},
					{9, 13, 8, 11},
					{27, 21, 24, 29},
				},
				Step:        10,
				RoundWeight: 10,
			},
			expectDirect: true,
		},
		{
			name: "RoundWeightGeT_WithBaselines",
			prob: MADProblem{
				Baselines: [][]float64{
					{12, 18, 25, 9},
					{31, 7, 19, 24},
					{5, 42, 16, 13},
				},
				Items: [][]float64{
					{14, 6, 11, 19},
					{23, 15, 4, 8},
					{7, 28, 12, 17},
					{19, 9, 21, 5},
				},
				Step:        10,
				RoundWeight: 6,
			},
			expectDirect: true,
		},
		{
			name: "NegativeEntriesAndNonPositiveSums_PositiveStep",
			prob: MADProblem{
				Baselines: [][]float64{
					{-40, -25, -10},
					{15, -30, 5},
					{30, 20, 45},
				},
				Items: [][]float64{
					{-20, -15, -5},
					{-12, 8, -10},
					{25, -5, 18},
					{-8, -12, -6},
					{14, 19, -7},
				},
				Step:          10,
				RoundWeight:   1.5,
				AllowZeroStep: false,
			},
		},
		{
			name: "NegativeEntriesAndNonPositiveSums_AllowZeroStep",
			prob: MADProblem{
				Baselines: [][]float64{
					{-40, -25, -10},
					{15, -30, 5},
					{30, 20, 45},
				},
				Items: [][]float64{
					{-20, -15, -5},
					{-12, 8, -10},
					{25, -5, 18},
					{-8, -12, -6},
					{14, 19, -7},
				},
				Step:          10,
				RoundWeight:   1.5,
				AllowZeroStep: true,
			},
		},
		{
			name: "DesignatedItems_UnsortedNilBaselines",
			prob: MADProblem{
				Groups: 3,
				Items: [][]float64{
					{14, 19, 22, 11},
					{31, 27, 34, 29},
					{8, 12, 9, 15},
					{45, 41, 39, 48},
					{21, 18, 25, 23},
				},
				Step:            10,
				RoundWeight:     2,
				DesignatedItems: []int{4, 1, 3},
			},
		},
		{
			name: "DesignatedItems_UnsortedWithBaselines",
			prob: MADProblem{
				Baselines: [][]float64{
					{10, 20, 15, 25},
					{50, 45, 55, 40},
					{28, 32, 24, 36},
				},
				Items: [][]float64{
					{14, 19, 22, 11},
					{31, 27, 34, 29},
					{8, 12, 9, 15},
					{45, 41, 39, 48},
					{21, 18, 25, 23},
				},
				Step:            10,
				RoundWeight:     2,
				DesignatedItems: []int{4, 1, 3},
			},
		},
		{
			name: "Unrounded_StepZero",
			prob: MADProblem{
				Baselines: [][]float64{
					{11, 29, 14},
					{35, 18, 22},
				},
				Items: [][]float64{
					{7, 13, 9},
					{18, 5, 14},
					{12, 21, 6},
				},
				Step:        0,
				RoundWeight: 2,
			},
			expectUnround: true,
		},
		{
			name: "Unrounded_WeightZero",
			prob: MADProblem{
				Baselines: [][]float64{
					{11, 29, 14},
					{35, 18, 22},
				},
				Items: [][]float64{
					{7, 13, 9},
					{18, 5, 14},
					{12, 21, 6},
				},
				Step:        10,
				RoundWeight: 0,
			},
			expectUnround: true,
		},
		{
			name: "EdgeDimensions_ZeroItems",
			prob: MADProblem{
				Baselines: [][]float64{
					{104, 198, 151, 210},
					{55, 62, 48, 70},
				},
				Step:        50,
				RoundWeight: 1.5,
			},
		},
		{
			name: "EdgeDimensions_ZeroItemsUnrounded",
			prob: MADProblem{
				Baselines: [][]float64{
					{104, 198, 151, 210},
					{55, 62, 48, 70},
				},
				Step:        0,
				RoundWeight: 0,
			},
			expectUnround: true,
		},
		{
			name: "EdgeDimensions_SingleGroup",
			prob: MADProblem{
				Groups: 1,
				Items: [][]float64{
					{23, -11, 40},
					{15, 19, -8},
					{-6, 12, 17},
					{9, -4, 11},
					{12, 20, 5},
				},
				Step:        10,
				RoundWeight: 2,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dpRes, err := SolveMAD(ctx, tc.prob, WithMADStrategy(MADDP))
			if err != nil {
				t.Fatalf("SolveMAD(DP): %v", err)
			}
			if dpRes.Status != MADStatusOptimal {
				t.Fatalf("DP Status = %v, want Optimal", dpRes.Status)
			}

			verifyRes := func(label string, got MADResult, exactMatch bool) {
				t.Helper()
				if got.Status != MADStatusOptimal {
					t.Fatalf("%s Status = %v, want Optimal", label, got.Status)
				}
				if got.BestBound > got.Objective+1e-9*max(1.0, math.Abs(got.Objective)) {
					t.Fatalf("%s BestBound %.17g > Objective %.17g", label, got.BestBound, got.Objective)
				}
				if exactMatch {
					if got.Objective != dpRes.Objective {
						t.Fatalf("%s Objective = %.17g, want exact %.17g", label, got.Objective, dpRes.Objective)
					}
				} else {
					assertMADClose(t, label, got.Objective, dpRes.Objective)
				}
				assertDesignatedBijection(t, tc.prob, got.Assignment)
				if tc.expectUnround {
					for g, st := range got.Steps {
						if st != 0 {
							t.Fatalf("%s Steps[%d] = %d, want 0 sentinel for unrounded problem", label, g, st)
						}
					}
				}
			}

			if !tc.skipBruteForce {
				bfRes, err := SolveMAD(ctx, tc.prob, WithMADStrategy(MADBruteForce))
				if err != nil {
					t.Fatalf("SolveMAD(BruteForce): %v", err)
				}
				verifyRes("BruteForce", bfRes, true)
			}

			certRes, err := SolveMAD(ctx, tc.prob, WithMADCertified(true))
			if err != nil {
				t.Fatalf("SolveMAD(Certified): %v", err)
			}
			verifyRes("DP_Certified", certRes, true)

			milpGo, err := SolveMAD(ctx, tc.prob,
				WithMADStrategy(MADMILP), WithMADBackend(BackendGo),
				WithMADRelGapTol(0), WithMADAbsGapTol(0))
			if err != nil {
				t.Fatalf("SolveMAD(MILP Go): %v", err)
			}
			verifyRes("MILP_Go", milpGo, false)

			milpGoCert, err := SolveMAD(ctx, tc.prob,
				WithMADStrategy(MADMILP), WithMADBackend(BackendGo),
				WithMADCertified(true))
			if err != nil {
				t.Fatalf("SolveMAD(MILP Go Certified): %v", err)
			}
			verifyRes("MILP_Go_Certified", milpGoCert, false)

			if CBCAvailable() {
				milpCBC, err := SolveMAD(ctx, tc.prob,
					WithMADStrategy(MADMILP), WithMADBackend(BackendCBC),
					WithMADRelGapTol(0), WithMADAbsGapTol(0))
				if err != nil {
					t.Fatalf("SolveMAD(MILP CBC): %v", err)
				}
				verifyRes("MILP_CBC", milpCBC, false)
			}

			m, vars, err := BuildMADModel(tc.prob)
			if err != nil {
				t.Fatalf("BuildMADModel: %v", err)
			}
			if tc.expectDirect && (vars.M != nil || vars.R != nil || len(vars.N) == 0) {
				t.Fatalf("expected direct step substitution (M=nil, R=nil), got len(M)=%d len(R)=%d len(N)=%d",
					len(vars.M), len(vars.R), len(vars.N))
			}
			if tc.expectUnround && (vars.N != nil || vars.R != nil || len(vars.M) == 0) {
				t.Fatalf("expected unrounded variables (N=nil, R=nil), got len(N)=%d len(R)=%d len(M)=%d",
					len(vars.N), len(vars.R), len(vars.M))
			}
			for _, pre := range []bool{true, false} {
				rawRes, sErr := m.Solve(ctx, WithPresolve(pre), WithRelGapTol(0), WithAbsGapTol(0))
				if sErr != nil {
					t.Fatalf("Model.Solve(presolve=%v): %v", pre, sErr)
				}
				if rawRes.Status != StatusOptimal {
					t.Fatalf("Model.Solve(presolve=%v) Status = %v, want Optimal", pre, rawRes.Status)
				}
				assertMADClose(t, "BuildMADModel", rawRes.Objective, dpRes.Objective)
				assign := extractMADAssignmentFromModel(t, vars, rawRes)
				ev, eErr := mad.EvaluateAssignment(tc.prob, assign)
				if eErr != nil {
					t.Fatalf("EvaluateAssignment(presolve=%v): %v", pre, eErr)
				}
				assertMADClose(t, "BuildMADModel_Polished", ev.Objective, dpRes.Objective)
			}
		})
	}
}

func makeStepAlignedTwoGroupProblem(numItems int, designated []int) MADProblem {
	items := make([][]float64, numItems)
	for i := range numItems {
		if i%2 == 0 {
			items[i] = []float64{100, 100, 100, 100}
		} else {
			items[i] = []float64{200, 200, 200, 200}
		}
	}
	return MADProblem{
		Groups:          2,
		Items:           items,
		Step:            100,
		RoundWeight:     2,
		DesignatedItems: designated,
	}
}

func TestMADAutoMILPFallbackBeyondMaxDPItems(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cases := []struct {
		name       string
		prob       MADProblem
		wantStatus MADStatus
	}{
		{
			name: "I15_NoDesignated",
			prob: makeStepAlignedTwoGroupProblem(15, nil),
		},
		{
			name: "I17_WithDesignated",
			prob: makeStepAlignedTwoGroupProblem(17, []int{15, 16}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			autoRes, err := SolveMAD(ctx, tc.prob)
			if err != nil {
				t.Fatalf("SolveMAD(Auto): %v", err)
			}
			if autoRes.Strategy != MADMILP || autoRes.Status != MADStatusOptimal {
				t.Fatalf("Auto = (strategy=%v status=%v), want (MILP, Optimal)", autoRes.Strategy, autoRes.Status)
			}

			certRes, err := SolveMAD(ctx, tc.prob, WithMADCertified(true))
			if err != nil {
				t.Fatalf("SolveMAD(Auto Certified): %v", err)
			}
			if certRes.Strategy != MADMILP || certRes.Status != MADStatusOptimal {
				t.Fatalf("Auto Certified = (strategy=%v status=%v), want (MILP, Optimal)", certRes.Strategy, certRes.Status)
			}

			dpRes, err := SolveMAD(ctx, tc.prob, WithMADStrategy(MADDP))
			if err != nil {
				t.Fatalf("SolveMAD(DP): %v", err)
			}
			if dpRes.Strategy != MADDP || dpRes.Status != MADStatusOptimal {
				t.Fatalf("DP = (strategy=%v status=%v), want (DP, Optimal)", dpRes.Strategy, dpRes.Status)
			}

			assertMADClose(t, tc.name+"_Auto", autoRes.Objective, dpRes.Objective)
			assertMADClose(t, tc.name+"_Cert", certRes.Objective, dpRes.Objective)
			assertDesignatedBijection(t, tc.prob, autoRes.Assignment)
			assertDesignatedBijection(t, tc.prob, certRes.Assignment)
		})
	}
}

func makeMultiNodeRandomMADProblem(numGroups, numItems, numPeriods int, seed uint64) MADProblem {
	rng := rand.New(rand.NewPCG(seed^0x517cc1b727220a95, seed^0x94d049bb133111eb))
	baselines := make([][]float64, numGroups)
	for g := range numGroups {
		baselines[g] = make([]float64, numPeriods)
		for t := range numPeriods {
			baselines[g][t] = float64(100 + rng.IntN(1700))
		}
	}
	items := make([][]float64, numItems)
	for i := range numItems {
		items[i] = make([]float64, numPeriods)
		for t := range numPeriods {
			items[i][t] = float64(100 + rng.IntN(1900))
		}
	}
	return MADProblem{
		Baselines:   baselines,
		Items:       items,
		Step:        100,
		RoundWeight: 1,
	}
}

func TestMADOptionOrderAndToleranceForwarding(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	defOpt := DefaultMADOptions()
	if defOpt.RelGapTolSet() || defOpt.AbsGapTolSet() || defOpt.MaxNodesSet() || defOpt.MaxIterSet() {
		t.Fatalf("DefaultMADOptions unexpectedly marked limits/gaps set: %+v", defOpt)
	}

	pmadProb := rootPMADProblem()
	orderCases := []struct {
		name    string
		opts    []MADOption
		wantObj float64
	}{
		{
			name:    "CertifiedBeforeBackend",
			opts:    []MADOption{WithMADStrategy(MADMILP), WithMADCertified(true), WithMADBackend(BackendGo)},
			wantObj: 24137,
		},
		{
			name:    "BackendBeforeCertified",
			opts:    []MADOption{WithMADBackend(BackendGo), WithMADCertified(true), WithMADStrategy(MADMILP)},
			wantObj: 24137,
		},
		{
			name:    "CertifiedThenDefaultOptions",
			opts:    []MADOption{WithMADCertified(true), WithMADOptions(mad.DefaultOptions())},
			wantObj: 24137,
		},
		{
			name: "CertifiedThenApplyOptionsCertifiedMILP",
			opts: []MADOption{
				WithMADCertified(true),
				WithMADOptions(mad.ApplyOptions(mad.WithStrategy(MADMILP), mad.WithCertified(true))),
			},
			wantObj: 24137,
		},
		{
			name:    "CertifiedEnabledThenDisabled",
			opts:    []MADOption{WithMADCertified(true), WithMADCertified(false), WithMADStrategy(MADMILP)},
			wantObj: 24137,
		},
	}
	if CBCAvailable() {
		orderCases = append(orderCases,
			struct {
				name    string
				opts    []MADOption
				wantObj float64
			}{
				name:    "CBC_CertifiedBeforeBackend",
				opts:    []MADOption{WithMADStrategy(MADMILP), WithMADCertified(true), WithMADBackend(BackendCBC)},
				wantObj: 24137,
			},
			struct {
				name    string
				opts    []MADOption
				wantObj float64
			}{
				name:    "CBC_BackendBeforeCertified",
				opts:    []MADOption{WithMADBackend(BackendCBC), WithMADCertified(true), WithMADStrategy(MADMILP)},
				wantObj: 24137,
			},
		)
	}

	for _, tc := range orderCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := SolveMAD(ctx, pmadProb, tc.opts...)
			if err != nil {
				t.Fatalf("SolveMAD(%s): %v", tc.name, err)
			}
			if res.Status != MADStatusOptimal || res.Objective != tc.wantObj {
				t.Fatalf("SolveMAD(%s) = (status=%v obj=%.17g), want (Optimal, %.17g)",
					tc.name, res.Status, res.Objective, tc.wantObj)
			}
		})
	}

	hard := makeMultiNodeRandomMADProblem(3, 10, 6, 42)
	t.Run("MaxNodesForwarding", func(t *testing.T) {
		t.Parallel()
		res, err := SolveMAD(ctx, hard, WithMADStrategy(MADMILP), WithMADMaxNodes(1))
		if err != nil {
			t.Fatalf("SolveMAD(MaxNodes=1): %v", err)
		}
		if res.Status != MADStatusNodeLimit {
			t.Fatalf("Status = %v, want %v", res.Status, MADStatusNodeLimit)
		}
		if res.BestBound < 0 || res.BestBound > res.Objective {
			t.Fatalf("invalid BestBound %.17g vs Objective %.17g", res.BestBound, res.Objective)
		}
	})

	t.Run("MaxIterForwarding", func(t *testing.T) {
		t.Parallel()
		res, err := SolveMAD(ctx, hard, WithMADStrategy(MADMILP), WithMADMaxIter(1))
		if err != nil {
			t.Fatalf("SolveMAD(MaxIter=1): %v", err)
		}
		if res.Status != MADStatusIterationLimit {
			t.Fatalf("Status = %v, want %v", res.Status, MADStatusIterationLimit)
		}
		if res.BestBound < 0 || res.BestBound > res.Objective {
			t.Fatalf("invalid BestBound %.17g vs Objective %.17g", res.BestBound, res.Objective)
		}
	})

	t.Run("RelGapTolForwarding", func(t *testing.T) {
		t.Parallel()
		res, err := SolveMAD(ctx, hard, WithMADStrategy(MADMILP), WithMADRelGapTol(0.5))
		if err != nil {
			t.Fatalf("SolveMAD(RelGapTol=0.5): %v", err)
		}
		if res.Status != MADStatusOptimal {
			t.Fatalf("Status = %v, want Optimal", res.Status)
		}
		if res.BestBound < 0 || res.BestBound > res.Objective {
			t.Fatalf("invalid BestBound %.17g vs Objective %.17g", res.BestBound, res.Objective)
		}
		ev, eErr := mad.EvaluateAssignment(hard, res.Assignment)
		if eErr != nil {
			t.Fatalf("EvaluateAssignment: %v", eErr)
		}
		if res.Objective != ev.Objective {
			t.Fatalf("polished Objective = %.17g, want %.17g", res.Objective, ev.Objective)
		}
	})

	t.Run("AbsGapTolForwarding", func(t *testing.T) {
		t.Parallel()
		res, err := SolveMAD(ctx, hard, WithMADStrategy(MADMILP), WithMADAbsGapTol(10000))
		if err != nil {
			t.Fatalf("SolveMAD(AbsGapTol=10000): %v", err)
		}
		if res.Status != MADStatusOptimal {
			t.Fatalf("Status = %v, want Optimal", res.Status)
		}
		if res.BestBound < 0 || res.BestBound > res.Objective {
			t.Fatalf("invalid BestBound %.17g vs Objective %.17g", res.BestBound, res.Objective)
		}
	})
}

func TestMADCertifiedFailClosedWithoutHook(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pmadProb := rootPMADProblem()
	autoMILP := makeStepAlignedTwoGroupProblem(15, nil)

	failCases := []struct {
		name      string
		injectErr bool
		prob      MADProblem
		opts      []MADOption
	}{
		{
			name: "LiteralCertifiedAuto",
			prob: pmadProb,
			opts: []MADOption{WithMADOptions(mad.Options{Certified: true})},
		},
		{
			name: "LiteralCertifiedMILP",
			prob: pmadProb,
			opts: []MADOption{WithMADOptions(mad.Options{Strategy: MADMILP, Certified: true})},
		},
		{
			name: "MadWithCertifiedOnMILPWithoutCertFallback",
			prob: pmadProb,
			opts: []MADOption{WithMADOptions(mad.ApplyOptions(mad.WithStrategy(MADMILP), mad.WithCertified(true)))},
		},
		{
			name: "MadWithCertifiedOnAutoFallbackWithoutCertFallback",
			prob: autoMILP,
			opts: []MADOption{WithMADOptions(mad.ApplyOptions(mad.WithCertified(true)))},
		},
		{
			name: "WithMADCertifiedClearedByLiteralOptions",
			prob: pmadProb,
			opts: []MADOption{WithMADCertified(true), WithMADOptions(mad.Options{Strategy: MADMILP, Certified: true})},
		},
		{
			name:      "RootMILPVIPRCertFaultInjection",
			injectErr: true,
			prob:      pmadProb,
			opts:      []MADOption{WithMADStrategy(MADMILP), WithMADCertified(true)},
		},
	}

	for _, tc := range failCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			callCtx := ctx
			if tc.injectErr {
				callCtx = withGoblinCertFault(ctx, func(p *CompiledProblem, trace *VIPRTrace, _ *lpCertWitness, _ *Result) {
					trace.reset(p)
				})
			}
			_, err := SolveMAD(callCtx, tc.prob, tc.opts...)
			if !errors.Is(err, mad.ErrCertificationFailed) {
				t.Fatalf("%s error = %v, want errors.Is(_, mad.ErrCertificationFailed)", tc.name, err)
			}
		})
	}
}

func TestMADMILPNoIncumbentTimeLimitGreedyFallback(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	probs := []struct {
		name string
		prob MADProblem
	}{
		{name: "I15_NoDesignated", prob: makeMultiNodeRandomMADProblem(3, 15, 4, 7)},
		{name: "I15_Designated", prob: func() MADProblem {
			p := makeMultiNodeRandomMADProblem(3, 15, 4, 9)
			p.DesignatedItems = []int{12, 13, 14}
			return p
		}()},
	}

	for _, tc := range probs {
		greedy, gErr := mad.GreedyIncumbent(tc.prob)
		if gErr != nil {
			t.Fatalf("GreedyIncumbent(%s): %v", tc.name, gErr)
		}

		for _, cert := range []bool{false, true} {
			subName := tc.name
			if cert {
				subName += "_Certified"
			} else {
				subName += "_Default"
			}
			t.Run(subName, func(t *testing.T) {
				t.Parallel()
				res, err := SolveMAD(ctx, tc.prob,
					WithMADStrategy(MADMILP),
					WithMADTimeLimit(-time.Millisecond),
					WithMADCertified(cert),
				)
				if err != nil {
					t.Fatalf("SolveMAD(%s): %v", subName, err)
				}
				if res.Status != MADStatusTimeLimit || res.Strategy != MADMILP {
					t.Fatalf("got (status=%v strategy=%v), want (TimeLimit, MILP)", res.Status, res.Strategy)
				}
				if math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0) || res.BestBound != 0 {
					t.Fatalf("BestBound = %.17g, want finite 0 on expired MILP", res.BestBound)
				}
				if len(res.Assignment) != len(tc.prob.Items) {
					t.Fatalf("len(Assignment) = %d, want %d", len(res.Assignment), len(tc.prob.Items))
				}
				assertDesignatedBijection(t, tc.prob, res.Assignment)
				if res.Objective != greedy.Objective {
					t.Fatalf("Objective = %.17g, want GreedyIncumbent %.17g", res.Objective, greedy.Objective)
				}
			})
		}
	}

	t.Run("CanceledContext", func(t *testing.T) {
		t.Parallel()
		cCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := SolveMAD(cCtx, probs[0].prob, WithMADStrategy(MADMILP))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled err = %v, want context.Canceled", err)
		}
	})

	t.Run("DeadlineExceededContext", func(t *testing.T) {
		t.Parallel()
		dCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Millisecond))
		defer cancel()
		_, err := SolveMAD(dCtx, probs[0].prob, WithMADStrategy(MADMILP))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline err = %v, want context.DeadlineExceeded", err)
		}
	})
}

func TestMADMILPStatusToErrorMapping(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	prob := rootPMADProblem()

	t.Run("InvalidBackend", func(t *testing.T) {
		t.Parallel()
		_, err := SolveMAD(ctx, prob, WithMADStrategy(MADMILP), WithMADBackend(Backend(99)))
		if err == nil {
			t.Fatal("expected error for invalid backend")
		}
	})

	if !CBCAvailable() {
		t.Run("UnavailableCBC", func(t *testing.T) {
			t.Parallel()
			_, err := SolveMAD(ctx, prob, WithMADStrategy(MADMILP), WithMADBackend(BackendCBC))
			if err == nil {
				t.Fatal("expected error for unavailable CBC backend")
			}
		})
	}

	m, vars, err := BuildMADModel(prob)
	if err != nil {
		t.Fatalf("BuildMADModel: %v", err)
	}
	numVars := m.NumVars()

	errStatuses := []Status{
		StatusInfeasible,
		StatusUnbounded,
		StatusNumericalError,
		StatusUndefined,
		StatusNotSolved,
		StatusGapLimit,
	}
	for _, st := range errStatuses {
		t.Run("ErrorStatus_"+st.String(), func(t *testing.T) {
			t.Parallel()
			if _, mErr := madResultFromMILP(prob, vars, numVars, Result{Status: st}, time.Millisecond); mErr == nil {
				t.Fatalf("madResultFromMILP(%s) = nil error, want non-nil", st)
			}
			_, cErr := madResultFromCertifiedMILP(prob, vars, numVars, Result{Status: st}, CertifiedWitness{}, time.Millisecond)
			if !errors.Is(cErr, mad.ErrCertificationFailed) {
				t.Fatalf("madResultFromCertifiedMILP(%s) = %v, want ErrCertificationFailed", st, cErr)
			}
		})
	}

	greedy, gErr := mad.GreedyIncumbent(prob)
	if gErr != nil {
		t.Fatalf("GreedyIncumbent: %v", gErr)
	}

	limitCases := []struct {
		name      string
		st        Status
		wantSt    MADStatus
		inBound   float64
		wantBound float64
	}{
		{name: "TimeLimit_NegInf", st: StatusTimeLimit, wantSt: MADStatusTimeLimit, inBound: math.Inf(-1), wantBound: 0},
		{name: "NodeLimit_Positive", st: StatusNodeLimit, wantSt: MADStatusNodeLimit, inBound: 12.5, wantBound: 12.5},
		{name: "IterLimit_NaN", st: StatusIterationLimit, wantSt: MADStatusIterationLimit, inBound: math.NaN(), wantBound: 0},
		{name: "TimeLimit_PosInf", st: StatusTimeLimit, wantSt: MADStatusTimeLimit, inBound: math.Inf(1), wantBound: greedy.Objective},
	}
	for _, lc := range limitCases {
		t.Run("NoIncumbent_"+lc.name, func(t *testing.T) {
			t.Parallel()
			got, mErr := madResultFromMILP(prob, vars, numVars, Result{Status: lc.st, BestBound: lc.inBound}, time.Millisecond)
			if mErr != nil {
				t.Fatalf("madResultFromMILP(%s): %v", lc.name, mErr)
			}
			if got.Status != lc.wantSt || got.Strategy != MADMILP || got.BestBound != lc.wantBound || got.Objective != greedy.Objective {
				t.Fatalf("got (%v, %v, bound=%.17g, obj=%.17g), want (%v, MILP, %.17g, %.17g)",
					got.Status, got.Strategy, got.BestBound, got.Objective, lc.wantSt, lc.wantBound, greedy.Objective)
			}
			gotC, cErr := madResultFromCertifiedMILP(prob, vars, numVars, Result{Status: lc.st, BestBound: lc.inBound}, CertifiedWitness{}, time.Millisecond)
			if cErr != nil {
				t.Fatalf("madResultFromCertifiedMILP(%s): %v", lc.name, cErr)
			}
			if gotC.Status != lc.wantSt || gotC.Strategy != MADMILP || gotC.BestBound != lc.wantBound {
				t.Fatalf("certified got (%v, %v, bound=%.17g), want (%v, MILP, %.17g)",
					gotC.Status, gotC.Strategy, gotC.BestBound, lc.wantSt, lc.wantBound)
			}
		})
	}

	t.Run("OptimalNonFiniteBoundUsesOracleBound", func(t *testing.T) {
		t.Parallel()
		vals := make([]float64, numVars)
		vals[vars.X[6][0].Index()] = 1
		vals[vars.X[9][1].Index()] = 1
		for _, badBnd := range []float64{math.Inf(-1), math.NaN()} {
			got, mErr := madResultFromMILP(prob, vars, numVars, Result{
				Status:    StatusOptimal,
				Values:    vals,
				BestBound: badBnd,
			}, time.Millisecond)
			if mErr != nil {
				t.Fatalf("madResultFromMILP(Optimal, %v): %v", badBnd, mErr)
			}
			if got.BestBound != 24137 || got.Objective != 24137 {
				t.Fatalf("got (bound=%.17g, obj=%.17g), want (24137, 24137)", got.BestBound, got.Objective)
			}
		}
	})
}

func TestRootSolveMADBinaryDeadCodeElimination(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tmp := t.TempDir()

	reCert := regexp.MustCompile(`(certifyMAD|defaultMADCertHooks|setMADCertifiedTrue|setMADCertifiedFalse|EvaluateAssignmentDyadic|evalGroupExactDyadic|solveCertified|finalizeCertifiedSolve|verifyCertified|VerifyVIPRTrace|verifyMiniVIPR|certPresolve|certEvalProofStep|setCertifiedTrue|setRootCertifiedTrue|VIPR|vipr|certifiedMILPFallback|setMADAliasCertifiedTrue)`)
	reJSON := regexp.MustCompile(`encoding/json`)

	checkBinary := func(name string, certified bool) {
		t.Helper()
		dir := filepath.Join(tmp, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		mainPath := filepath.Join(dir, "main.go")
		binPath := filepath.Join(dir, name)

		optArg := ""
		if certified {
			optArg = ", gb.WithMADCertified(true)"
		}
		src := `package main

import (
	"context"
	"fmt"

	gb "github.com/paulourio/goblin"
)

func main() {
	prob := gb.MADProblem{
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
	res, err := gb.SolveMAD(context.Background(), prob` + optArg + `)
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
		if got := strings.TrimSpace(string(runOut)); got != "24137 <nil>" {
			t.Fatalf("%s output = %q, want %q", name, got, "24137 <nil>")
		}

		nmOut, err := exec.CommandContext(ctx, "go", "tool", "nm", "-size", binPath).CombinedOutput()
		if err != nil {
			t.Fatalf("go tool nm %s: %v\n%s", name, err, nmOut)
		}

		var certSyms, jsonSyms []string
		for line := range strings.SplitSeq(string(nmOut), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			sym := fields[len(fields)-1]
			if reJSON.MatchString(sym) {
				jsonSyms = append(jsonSyms, sym)
			}
			if reCert.MatchString(sym) {
				certSyms = append(certSyms, sym)
			}
		}
		if len(jsonSyms) > 0 {
			t.Fatalf("%s linked encoding/json symbols (%d): %v", name, len(jsonSyms), jsonSyms[:min(len(jsonSyms), 10)])
		}
		if !certified && len(certSyms) > 0 {
			t.Fatalf("%s uncertified binary linked certification symbols (%d): %v", name, len(certSyms), certSyms[:min(len(certSyms), 10)])
		}
		if certified && len(certSyms) == 0 {
			t.Fatalf("%s certified binary missing certification symbols", name)
		}
	}

	checkBinary("gb_mad_default", false)
	checkBinary("gb_mad_certified", true)
}
