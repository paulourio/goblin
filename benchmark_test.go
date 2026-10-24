package goblin_test

import (
	"bytes"
	"context"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
)

// corpusLookup indexes the canonical benchmark corpus by instance ID for fast lookup in benchmarks.
func corpusLookup(tb testing.TB) map[string]*gb.Model {
	tb.Helper()
	cases := bench.BuildCorpus()
	m := make(map[string]*gb.Model, len(cases))
	for _, c := range cases {
		m[c.ID] = c.Model
	}
	return m
}

func canonicalMADSmallModel() *gb.Model {
	w := [][]float64{
		{8313, 0, 5258, 10400},
		{4344, 50, 8917, 10350},
		{3716, 70, 9308, 10330},
		{13203, 70, 3331, 10330},
		{7885, 700, 8347, 9700},
	}
	return bench.BuildMADFromWeights("mad_canonical_g2_i4_t5", 2, 4, 5, 100.0, w)
}

func BenchmarkSolveMAD(b *testing.B) {
	byID := corpusLookup(b)
	ids := []string{
		"t1_mad_g2_i4",
		"t1_mad_rand_g2_i4",
		"t2_mad_g2_i8",
		"t2_mad_rand_g2_i8",
		"t2_mad_g3_i9",
		"t2_mad_g4_i12",
		"t3_mad_g5_i20",
		"t3_mad_g6_i30",
	}

	b.Run("mad_canonical_g2_i4_t5", func(b *testing.B) {
		ctx := b.Context()
		p := canonicalMADSmallModel().Compile()
		b.ReportAllocs()
		b.ResetTimer()
		var last gb.Result
		for range b.N {
			var err error
			last, err = gb.SolveCompiled(ctx, p)
			if err != nil {
				b.Fatal(err)
			}
		}
		if last.Status != gb.StatusOptimal {
			b.Fatalf("expected Optimal, got %v", last.Status)
		}
		b.ReportMetric(float64(last.Nodes), "nodes/op")
		b.ReportMetric(float64(last.Iterations), "iters/op")
	})

	for _, id := range ids {
		mdl, ok := byID[id]
		if !ok {
			b.Fatalf("missing corpus model %s", id)
		}
		p := mdl.Compile()
		b.Run(id, func(b *testing.B) {
			ctx := b.Context()
			b.ReportAllocs()
			b.ResetTimer()
			var last gb.Result
			for range b.N {
				var err error
				last, err = gb.SolveCompiled(ctx, p)
				if err != nil {
					b.Fatal(err)
				}
			}
			if last.Status != gb.StatusOptimal {
				b.Fatalf("[%s] expected Optimal, got %v", id, last.Status)
			}
			b.ReportMetric(float64(last.Nodes), "nodes/op")
			b.ReportMetric(float64(last.Iterations), "iters/op")
		})
	}
}

func BenchmarkSolveCorpus(b *testing.B) {
	byID := corpusLookup(b)
	ids := []string{
		"t1_whiskas",
		"t1_knapsack_12",
		"t2_mkp_30",
		"t2_cflp_36",
		"t2_gap_45",
		"t2_lotsizing_48",
		"t2_binpack_44",
		"t2_setcover_50",
		"t3_cflp_300",
		"t3_sched_1000",
	}

	for _, id := range ids {
		mdl, ok := byID[id]
		if !ok {
			b.Fatalf("missing corpus model %s", id)
		}
		p := mdl.Compile()
		b.Run(id, func(b *testing.B) {
			ctx := b.Context()
			b.ReportAllocs()
			b.ResetTimer()
			var last gb.Result
			for range b.N {
				var err error
				last, err = gb.SolveCompiled(ctx, p)
				if err != nil {
					b.Fatal(err)
				}
			}
			if last.Status != gb.StatusOptimal {
				b.Fatalf("[%s] expected Optimal, got %v", id, last.Status)
			}
			b.ReportMetric(float64(last.Nodes), "nodes/op")
			b.ReportMetric(float64(last.Iterations), "iters/op")
		})
	}
}

func BenchmarkRootLP(b *testing.B) {
	byID := corpusLookup(b)
	ids := []string{
		"t1_whiskas",
		"t1_mad_g2_i4",
		"t2_mad_g4_i12",
		"t2_lotsizing_48",
		"t3_cflp_300",
		"t3_sched_1000",
	}

	for _, id := range ids {
		mdl, ok := byID[id]
		if !ok {
			b.Fatalf("missing corpus model %s", id)
		}
		p := mdl.Compile()
		b.Run(id, func(b *testing.B) {
			ws := gb.AcquireWorkspace(p.NumVars, p.NumConstraints)
			defer gb.ReleaseWorkspace(ws)
			b.ReportAllocs()
			b.ResetTimer()
			var last gb.LPResult
			for range b.N {
				s := gb.NewLPSolver(p, ws)
				last = s.Solve()
			}
			if last.Status != gb.StatusOptimal {
				b.Fatalf("[%s] expected Optimal root LP, got %v", id, last.Status)
			}
			b.ReportMetric(float64(last.Iterations), "iters/op")
		})
	}
}

func BenchmarkCompileAndFormats(b *testing.B) {
	byID := corpusLookup(b)
	for _, id := range []string{"t1_mad_g2_i4", "t2_mad_g4_i12", "t3_sched_1000"} {
		mdl := byID[id]
		p := mdl.Compile()
		var jsonBuf bytes.Buffer
		if err := format.ToPuLPJSONCompact(p, &jsonBuf); err != nil {
			b.Fatal(err)
		}
		rawJSON := jsonBuf.Bytes()

		b.Run(id+"/Compile", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var cp *gb.CompiledProblem
			for range b.N {
				cp = mdl.Compile()
			}
			_ = cp
		})

		b.Run(id+"/ParseAndBuildPuLPJSON", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				doc, err := format.ParsePuLPJSON(rawJSON)
				if err != nil {
					b.Fatal(err)
				}
				built, err := format.BuildModelFromPuLPJSON(doc)
				if err != nil {
					b.Fatal(err)
				}
				_ = built
			}
		})

		b.Run(id+"/WriteMPS", func(b *testing.B) {
			var mpsBuf bytes.Buffer
			mpsBuf.Grow(1 << 16)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				mpsBuf.Reset()
				if err := format.WriteMPS(p, &mpsBuf); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkContextOverhead(b *testing.B) {
	byID := corpusLookup(b)
	bgCtx := context.Background()
	for _, id := range []string{"t1_whiskas", "t2_mad_g4_i12", "t2_cflp_36", "t3_sched_1000"} {
		p := byID[id].Compile()
		cancelCtx := b.Context()

		b.Run(id+"/Background", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := gb.SolveCompiled(bgCtx, p)
				if err != nil || res.Status != gb.StatusOptimal {
					b.Fatalf("err=%v status=%v", err, res.Status)
				}
			}
		})
		b.Run(id+"/Cancellable", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := gb.SolveCompiled(cancelCtx, p)
				if err != nil || res.Status != gb.StatusOptimal {
					b.Fatalf("err=%v status=%v", err, res.Status)
				}
			}
		})
	}
}
