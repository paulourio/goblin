package mad_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/paulourio/goblin/mad"
)

func buildBenchProblem(numGroups, numItems, numPeriods int, seed uint64, withBaselines bool) mad.Problem {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	items := make([][]float64, numItems)
	for i := range numItems {
		items[i] = make([]float64, numPeriods)
		for t := range numPeriods {
			items[i][t] = float64(100 + rng.IntN(1900))
		}
	}
	var baselines [][]float64
	if withBaselines {
		baselines = make([][]float64, numGroups)
		for g := range numGroups {
			baselines[g] = make([]float64, numPeriods)
			for t := range numPeriods {
				baselines[g][t] = float64(100 + rng.IntN(1900))
			}
		}
	}
	return mad.Problem{
		Groups:      numGroups,
		Baselines:   baselines,
		Items:       items,
		Step:        100,
		RoundWeight: float64(numPeriods),
	}
}

func pmadBaseBenchProblem() mad.Problem {
	return mad.Problem{
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
}

func BenchmarkPMAD24137(b *testing.B) {
	ctx := b.Context()
	prob := pmadBaseBenchProblem()

	cases := []struct {
		name string
		opts []mad.Option
	}{
		{"Auto", nil},
		{"DFS", []mad.Option{mad.WithStrategy(mad.StrategyBruteForce)}},
		{"DP", []mad.Option{mad.WithStrategy(mad.StrategyDP)}},
		{"CertifiedDP", []mad.Option{mad.WithStrategy(mad.StrategyDP), mad.WithCertified(true)}},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := mad.Solve(ctx, prob, tc.opts...)
				if err != nil || res.Objective != 24137 {
					b.Fatalf("Solve(%s): res=%+v err=%v", tc.name, res, err)
				}
			}
		})
	}
}

func BenchmarkCombScaling(b *testing.B) {
	ctx := b.Context()
	configs := []struct {
		g, i, t   int
		baselines bool
	}{
		{4, 8, 6, false},
		{6, 10, 6, true},
		{10, 12, 6, false},
	}

	for _, cfg := range configs {
		prob := buildBenchProblem(cfg.g, cfg.i, cfg.t, uint64(cfg.g*100+cfg.i), cfg.baselines)
		label := fmt.Sprintf("G=%d_I=%d_T=%d_Base=%v", cfg.g, cfg.i, cfg.t, cfg.baselines)
		b.Run(label+"/DP", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyDP))
				if err != nil || res.Status != mad.StatusOptimal {
					b.Fatalf("DP failed: res=%+v err=%v", res, err)
				}
			}
		})
		b.Run(label+"/CertifiedDP", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyDP), mad.WithCertified(true))
				if err != nil || res.Status != mad.StatusOptimal {
					b.Fatalf("CertifiedDP failed: res=%+v err=%v", res, err)
				}
			}
		})
	}
}

func BenchmarkSingleGroupFastPath(b *testing.B) {
	ctx := b.Context()
	prob := buildBenchProblem(1, 100, 12, 777, false)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		res, err := mad.Solve(ctx, prob)
		if err != nil || res.Status != mad.StatusOptimal {
			b.Fatalf("SingleGroup failed: res=%+v err=%v", res, err)
		}
	}
}

func BenchmarkEvaluateAssignment(b *testing.B) {
	prob := buildBenchProblem(10, 14, 6, 999, true)
	assign := make([]int, 14)
	for i := range assign {
		assign[i] = i % 10
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		res, err := mad.EvaluateAssignment(prob, assign)
		if err != nil || res.Status != mad.StatusOptimal {
			b.Fatalf("EvaluateAssignment failed: res=%+v err=%v", res, err)
		}
	}
}
