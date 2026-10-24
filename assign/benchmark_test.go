package assign_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/paulourio/goblin/assign"
)

// buildBenchMatrix populates a row-major cost matrix of size rows by cols from a deterministic PCG seed.
func buildBenchMatrix(rows, cols int, seed uint64, tied bool) []float64 {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	cost := make([]float64, rows*cols)
	if tied {
		for i := range cost {
			cost[i] = float64(rng.IntN(16))
		}
	} else {
		for i := range cost {
			cost[i] = rng.Float64() * 10000.0
		}
	}
	return cost
}

func BenchmarkLinearSumSquare(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{10, 32, 64, 100, 256, 500, 1000} {
		cost := buildBenchMatrix(n, n, uint64(1000+n), false)
		b.Run(fmt.Sprintf("N=%d/LinearSumFlat", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				res, err := assign.LinearSumFlat(ctx, n, n, cost, assign.Minimize)
				if err != nil || len(res.RowInd) != n {
					b.Fatalf("unexpected result: err=%v len=%d", err, len(res.RowInd))
				}
			}
		})

		b.Run(fmt.Sprintf("N=%d/SolveIntoWarm", n), func(b *testing.B) {
			rowInd := make([]int, n)
			colInd := make([]int, n)
			ws := assign.AcquireWorkspace(n, n)
			defer assign.ReleaseWorkspace(ws)
			_, _ = assign.SolveInto(ctx, n, n, cost, assign.Minimize, rowInd, colInd, ws)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, err := assign.SolveInto(ctx, n, n, cost, assign.Minimize, rowInd, colInd, ws)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLinearSumRectangularAndTied(b *testing.B) {
	ctx := context.Background()
	cases := []struct {
		name       string
		rows, cols int
		tied       bool
	}{
		{"Wide_50x200", 50, 200, false},
		{"Tall_200x50", 200, 50, false},
		{"Wide_250x1000", 250, 1000, false},
		{"Tall_1000x250", 1000, 250, false},
		{"Tied_100x100", 100, 100, true},
		{"Tied_500x500", 500, 500, true},
	}

	for _, tc := range cases {
		cost := buildBenchMatrix(tc.rows, tc.cols, uint64(tc.rows*1000+tc.cols), tc.tied)
		b.Run(tc.name, func(b *testing.B) {
			k := min(tc.rows, tc.cols)
			rowInd := make([]int, k)
			colInd := make([]int, k)
			ws := assign.AcquireWorkspace(tc.rows, tc.cols)
			defer assign.ReleaseWorkspace(ws)
			_, _ = assign.SolveInto(ctx, tc.rows, tc.cols, cost, assign.Minimize, rowInd, colInd, ws)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, err := assign.SolveInto(ctx, tc.rows, tc.cols, cost, assign.Minimize, rowInd, colInd, ws)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLinearSumSIMDvsScalar(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{256, 500, 1000} {
		cost := buildBenchMatrix(n, n, uint64(9000+n), false)
		for _, mode := range []struct {
			name   string
			noSIMD bool
		}{
			{"SIMD", false},
			{"Scalar", true},
		} {
			b.Run(fmt.Sprintf("N=%d/%s", n, mode.name), func(b *testing.B) {
				rowInd := make([]int, n)
				colInd := make([]int, n)
				ws := assign.AcquireWorkspace(n, n)
				defer assign.ReleaseWorkspace(ws)
				opt := assign.WithSIMD(!mode.noSIMD)
				_, _ = assign.SolveInto(ctx, n, n, cost, assign.Minimize, rowInd, colInd, ws, opt)

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					_, err := assign.SolveInto(ctx, n, n, cost, assign.Minimize, rowInd, colInd, ws, opt)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkAssignContextOverhead(b *testing.B) {
	bgCtx := context.Background()
	cancelCtx := b.Context()
	for _, n := range []int{10, 64, 256} {
		cost := buildBenchMatrix(n, n, uint64(5000+n), false)
		rowInd := make([]int, n)
		colInd := make([]int, n)
		ws := assign.AcquireWorkspace(n, n)
		defer assign.ReleaseWorkspace(ws)

		b.Run(fmt.Sprintf("N=%d/Background", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := assign.SolveInto(bgCtx, n, n, cost, assign.Minimize, rowInd, colInd, ws); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("N=%d/Cancellable", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := assign.SolveInto(cancelCtx, n, n, cost, assign.Minimize, rowInd, colInd, ws); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
