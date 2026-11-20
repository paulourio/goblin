package simd_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

var assignKernelSizes = []int{0, 1, 2, 3, 4, 5, 7, 8, 15, 16, 31, 32, 33, 34, 35, 39, 40, 63, 64, 65, 100, 127, 131, 259}

// relaxFullStep exercises the caller contract of RelaxRow64With by stepping through all
// tight edges and returning the final d, pred, and list of tight-edge indices.
func relaxFullStep(tb testing.TB, rCost, v, dIn []float64, predIn []int, rowI int, ui, minVal float64, disableSIMD bool) ([]float64, []int, []int) {
	tb.Helper()
	n := len(rCost)
	d := slices.Clone(dIn)
	pred := slices.Clone(predIn)
	var tights []int
	nan := math.NaN()

	for j := 0; j < n; {
		j = simd.RelaxRow64With(rCost, v, d, pred, j, rowI, ui, minVal, disableSIMD)
		if j >= n {
			break
		}
		lim := min(j+4, n)
		for ; j < lim; j++ {
			r := minVal + ((rCost[j] - v[j]) - ui)
			if r < d[j] {
				pred[j] = rowI
				if r <= minVal {
					tights = append(tights, j)
					d[j] = nan
				} else {
					d[j] = r
				}
			}
		}
	}
	return d, pred, tights
}

func TestRelaxRow64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(101, 202))
	nan := math.NaN()

	for _, n := range assignKernelSizes {
		rCost := make([]float64, n)
		v := make([]float64, n)
		dInit := make([]float64, n)
		predInit := make([]int, n)
		for j := range n {
			rCost[j] = (rng.Float64() - 0.5) * 100.0
			v[j] = (rng.Float64() - 0.5) * 20.0
			switch rng.IntN(5) {
			case 0:
				dInit[j] = nan
			case 1:
				dInit[j] = math.Inf(1)
			default:
				dInit[j] = 10.0 + rng.Float64()*50.0
			}
			predInit[j] = j + 1000
		}
		// Inject tight edges and forbidden +Inf costs across vector chunks and scalar tail.
		if n > 0 {
			for _, idx := range []int{0, n / 3, n / 2, n - 1} {
				if !math.IsNaN(dInit[idx]) {
					rCost[idx] = v[idx] - 5.0
				}
			}
			if n >= 5 {
				rCost[n-2] = math.Inf(1)
			}
		}

		ui := 2.5
		minVal := 15.0
		rowI := 77

		dSIMD, pSIMD, tSIMD := relaxFullStep(t, rCost, v, dInit, predInit, rowI, ui, minVal, false)
		dScal, pScal, tScal := relaxFullStep(t, rCost, v, dInit, predInit, rowI, ui, minVal, true)

		if !slices.Equal(tSIMD, tScal) {
			t.Fatalf("n=%d: tight indices SIMD=%v != Scalar=%v", n, tSIMD, tScal)
		}
		if !slices.Equal(pSIMD, pScal) {
			t.Fatalf("n=%d: pred mismatch SIMD=%v != Scalar=%v", n, pSIMD, pScal)
		}
		for j := range n {
			if math.IsNaN(dInit[j]) {
				if !math.IsNaN(dSIMD[j]) || pSIMD[j] != predInit[j] {
					t.Fatalf("n=%d j=%d: NaN sentinel overwritten: d=%v pred=%d (want NaN, %d)", n, j, dSIMD[j], pSIMD[j], predInit[j])
				}
			}
			if math.IsNaN(dSIMD[j]) != math.IsNaN(dScal[j]) {
				t.Fatalf("n=%d j=%d: NaN mismatch SIMD=%v vs Scalar=%v", n, j, dSIMD[j], dScal[j])
			}
			if !math.IsNaN(dSIMD[j]) && math.Float64bits(dSIMD[j]) != math.Float64bits(dScal[j]) {
				t.Fatalf("n=%d j=%d: d mismatch SIMD=%v vs Scalar=%v", n, j, dSIMD[j], dScal[j])
			}
		}
	}

	// Verify direct RelaxRow64 call with non-zero start and out-of-range start.
	rCost := make([]float64, 40)
	v := make([]float64, 40)
	d := make([]float64, 40)
	pred := make([]int, 40)
	for j := range 40 {
		rCost[j] = float64(j) + 20.0
		d[j] = 100.0
	}
	if got := simd.RelaxRow64(rCost, v, d, pred, -3, 1, 0, 1.0); got != 40 {
		t.Fatalf("expected RelaxRow64 with negative start to return 40, got %d", got)
	}
	if got := simd.RelaxRow64(rCost, v, d, pred, 40, 1, 0, 1.0); got != 40 {
		t.Fatalf("expected RelaxRow64 with start==n to return 40, got %d", got)
	}
}

func TestMinNoNaN64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(303, 404))
	nan := math.NaN()

	for _, n := range assignKernelSizes {
		x := make([]float64, n)
		for i := range n {
			if rng.IntN(3) == 0 {
				x[i] = nan
			} else {
				x[i] = (rng.Float64() - 0.5) * 1000.0
			}
		}
		gotSIMD := simd.MinNoNaN64(x)
		gotScal := simd.MinNoNaN64Scalar(x)
		gotOff := simd.MinNoNaN64With(x, true)
		if gotSIMD != gotScal || gotOff != gotScal {
			t.Fatalf("n=%d: MinNoNaN64=(%v, scalar=%v, off=%v)", n, gotSIMD, gotScal, gotOff)
		}

		// All-NaN input must return +Inf.
		allNaN := make([]float64, n)
		for i := range n {
			allNaN[i] = nan
		}
		if got := simd.MinNoNaN64(allNaN); !math.IsInf(got, 1) {
			t.Fatalf("n=%d all-NaN: got %v, want +Inf", n, got)
		}

		// Single finite value placed at every position among NaNs.
		if n > 0 && n <= 65 {
			for pos := range n {
				allNaN[pos] = -42.25
				got := simd.MinNoNaN64(allNaN)
				if got != -42.25 {
					t.Fatalf("n=%d pos=%d: got %v, want -42.25", n, pos, got)
				}
				allNaN[pos] = nan
			}
		}
	}
}

func TestMinReducedSkip64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(505, 606))
	for _, n := range assignKernelSizes {
		row := make([]float64, n)
		v := make([]float64, n)
		for j := range n {
			row[j] = (rng.Float64() - 0.5) * 500.0
			v[j] = (rng.Float64() - 0.5) * 500.0
		}
		skips := []int{-1, 0, 1, 3, 4, n / 2, n - 4, n - 1, n}
		for _, skip := range skips {
			gotSIMD := simd.MinReducedSkip64(row, v, skip)
			gotScal := simd.MinReducedSkip64Scalar(row, v, skip)
			gotOff := simd.MinReducedSkip64With(row, v, skip, true)
			if math.Float64bits(gotSIMD) != math.Float64bits(gotScal) || math.Float64bits(gotOff) != math.Float64bits(gotScal) {
				t.Fatalf("n=%d skip=%d: SIMD=%v Scalar=%v Off=%v", n, skip, gotSIMD, gotScal, gotOff)
			}
		}

		// Verify NaN at skip is ignored while NaN at any other index returns NaN.
		if n >= 2 {
			for _, skip := range []int{0, 3, n / 2, n - 1} {
				if skip >= n {
					continue
				}
				saved := row[skip]
				row[skip] = math.NaN()
				gotSIMD := simd.MinReducedSkip64(row, v, skip)
				gotScal := simd.MinReducedSkip64Scalar(row, v, skip)
				if math.IsNaN(gotSIMD) || math.Float64bits(gotSIMD) != math.Float64bits(gotScal) {
					t.Fatalf("n=%d NaN at skip=%d: SIMD=%v Scalar=%v", n, skip, gotSIMD, gotScal)
				}
				row[skip] = saved

				other := (skip + 1) % n
				savedOther := row[other]
				row[other] = math.NaN()
				gotSIMD = simd.MinReducedSkip64(row, v, skip)
				gotScal = simd.MinReducedSkip64Scalar(row, v, skip)
				if !math.IsNaN(gotSIMD) || !math.IsNaN(gotScal) {
					t.Fatalf("n=%d NaN at other=%d (skip=%d): SIMD=%v Scalar=%v, want NaN", n, other, skip, gotSIMD, gotScal)
				}
				row[other] = savedOther
			}
		}
	}
}

func TestValidateAndNegate64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(707, 808))
	const limit = 1e290
	negZero := math.Copysign(0, -1)

	for _, n := range assignKernelSizes {
		src := make([]float64, n)
		for i := range n {
			switch i % 5 {
			case 0:
				src[i] = 0.0
			case 1:
				src[i] = negZero
			default:
				src[i] = (rng.Float64() - 0.5) * 1e6
			}
		}

		dstSIMD := make([]float64, n)
		dstScal := make([]float64, n)
		okSIMD := simd.ValidateAndNegate64(dstSIMD, src, limit)
		okScal := simd.ValidateAndNegate64Scalar(dstScal, src, limit)
		if !okSIMD || !okScal {
			t.Fatalf("n=%d: expected true on finite in-range input (simd=%v scal=%v)", n, okSIMD, okScal)
		}
		for i := range n {
			if src[i] == 0 && math.Float64bits(dstSIMD[i]) != 0 {
				t.Fatalf("n=%d i=%d: zero negation did not yield +0.0 (bits=%x)", n, i, math.Float64bits(dstSIMD[i]))
			}
			if math.Float64bits(dstSIMD[i]) != math.Float64bits(dstScal[i]) {
				t.Fatalf("n=%d i=%d: dstSIMD=%x != dstScal=%x", n, i, math.Float64bits(dstSIMD[i]), math.Float64bits(dstScal[i]))
			}
		}

		// Verify aliased in-place operation and out-of-range/NaN/Inf handling.
		if n > 0 {
			for _, badPos := range []int{0, n / 2, n - 1} {
				for _, badVal := range []float64{1e295, -1e295, math.Inf(1), math.Inf(-1), math.NaN()} {
					inSIMD := slices.Clone(src)
					inScal := slices.Clone(src)
					inSIMD[badPos] = badVal
					inScal[badPos] = badVal
					okS := simd.ValidateAndNegate64(inSIMD, inSIMD, limit)
					okC := simd.ValidateAndNegate64Scalar(inScal, inScal, limit)
					if okS || okC {
						t.Fatalf("n=%d pos=%d bad=%v: expected false (simd=%v scal=%v)", n, badPos, badVal, okS, okC)
					}
					for i := range n {
						if math.IsNaN(inSIMD[i]) != math.IsNaN(inScal[i]) {
							t.Fatalf("n=%d pos=%d i=%d: NaN mismatch in-place", n, badPos, i)
						}
						if !math.IsNaN(inSIMD[i]) && math.Float64bits(inSIMD[i]) != math.Float64bits(inScal[i]) {
							t.Fatalf("n=%d pos=%d i=%d: in-place mismatch SIMD=%v vs Scalar=%v", n, badPos, i, inSIMD[i], inScal[i])
						}
					}
				}
			}
		}

		// Verify negative and NaN limits return false.
		if simd.ValidateAndNegate64(dstSIMD, src, -1.0) || simd.ValidateAndNegate64Scalar(dstScal, src, -1.0) {
			t.Fatalf("n=%d: expected false for negative limit", n)
		}
		if simd.ValidateAndNegate64(dstSIMD, src, math.NaN()) || simd.ValidateAndNegate64Scalar(dstScal, src, math.NaN()) {
			t.Fatalf("n=%d: expected false for NaN limit", n)
		}
	}
}

func TestColReduceRows64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(909, 1010))
	const limit = 1e290

	for _, n := range []int{1, 3, 4, 7, 16, 31, 32, 33, 35, 64, 67, 100, 131} {
		rows := min(n, 40)
		cost := make([]float64, rows*n)
		for i := range cost {
			// Include frequent ties to verify strict '<' smallest-row-index tie breaking.
			cost[i] = float64(rng.IntN(17) - 8)
		}

		vSIMD := make([]float64, n)
		rSIMD := make([]int, n)
		vScal := make([]float64, n)
		rScal := make([]int, n)

		okSIMD := simd.ColReduceValidateRows64(n, 0, rows, cost, vSIMD, rSIMD, limit)
		okScal := simd.ColReduceValidateRows64Scalar(n, 0, rows, cost, vScal, rScal, limit)
		if !okSIMD || !okScal {
			t.Fatalf("n=%d: expected valid reduction", n)
		}
		if !slices.Equal(vSIMD, vScal) || !slices.Equal(rSIMD, rScal) {
			t.Fatalf("n=%d: ColReduceValidateRows64 mismatch between SIMD and Scalar", n)
		}

		// Verify chunked execution produces identical results.
		if rows >= 4 {
			vChunk := make([]float64, n)
			rChunk := make([]int, n)
			mid := rows / 2
			ok1 := simd.ColReduceValidateRows64(n, 0, mid, cost, vChunk, rChunk, limit)
			ok2 := simd.ColReduceValidateRows64(n, mid, rows, cost, vChunk, rChunk, limit)
			if !ok1 || !ok2 || !slices.Equal(vChunk, vScal) || !slices.Equal(rChunk, rScal) {
				t.Fatalf("n=%d: chunked ColReduceValidateRows64 mismatch", n)
			}
		}

		// Verify out-of-range detection in row 0, middle row, and last tail column.
		for _, badIdx := range []int{0, n - 1, (rows - 1) * n, rows*n - 1} {
			saved := cost[badIdx]
			for _, badVal := range []float64{1e295, -1e295, math.Inf(1), math.Inf(-1), math.NaN()} {
				cost[badIdx] = badVal
				if simd.ColReduceValidateRows64(n, 0, rows, cost, vSIMD, rSIMD, limit) {
					t.Fatalf("n=%d badIdx=%d badVal=%v: expected SIMD false", n, badIdx, badVal)
				}
				if simd.ColReduceValidateRows64Scalar(n, 0, rows, cost, vScal, rScal, limit) {
					t.Fatalf("n=%d badIdx=%d badVal=%v: expected Scalar false", n, badIdx, badVal)
				}
			}
			cost[badIdx] = saved
		}

		// Verify ColReduceRows64 handles +Inf forbidden entries identically.
		cost[0] = math.Inf(1)
		cost[rows*n-1] = math.Inf(1)
		simd.ColReduceRows64(n, 0, rows, cost, vSIMD, rSIMD)
		simd.ColReduceRows64Scalar(n, 0, rows, cost, vScal, rScal)
		if !slices.Equal(vSIMD, vScal) || !slices.Equal(rSIMD, rScal) {
			t.Fatalf("n=%d: ColReduceRows64 (+Inf) mismatch between SIMD and Scalar", n)
		}
	}
}

func TestTranspose64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(1111, 1212))
	negZero := math.Copysign(0, -1)
	shapes := [][2]int{
		{1, 1}, {3, 5}, {5, 3}, {7, 7}, {8, 8}, {9, 9},
		{15, 17}, {16, 16}, {17, 15}, {31, 33}, {32, 32},
		{131, 37}, {37, 131}, {64, 19}, {19, 64},
	}
	for _, sh := range shapes {
		rows, cols := sh[0], sh[1]
		src := make([]float64, rows*cols)
		for i := range src {
			switch i % 7 {
			case 0:
				src[i] = 0.0
			case 1:
				src[i] = negZero
			default:
				src[i] = (rng.Float64() - 0.5) * 1000.0
			}
		}
		for _, negate := range []bool{false, true} {
			dstSIMD := make([]float64, rows*cols)
			dstScal := make([]float64, rows*cols)
			simd.Transpose64(dstSIMD, src, rows, cols, negate)
			simd.Transpose64Scalar(dstScal, src, rows, cols, negate)
			for c := range cols {
				for r := range rows {
					gotSIMD := dstSIMD[c*rows+r]
					gotScal := dstScal[c*rows+r]
					want := src[r*cols+c]
					if negate {
						want = 0.0 - want
					}
					if math.Float64bits(gotSIMD) != math.Float64bits(want) {
						t.Fatalf("%dx%d negate=%v (%d,%d): SIMD=%x != want=%x", rows, cols, negate, r, c, math.Float64bits(gotSIMD), math.Float64bits(want))
					}
					if math.Float64bits(gotScal) != math.Float64bits(want) {
						t.Fatalf("%dx%d negate=%v (%d,%d): Scalar=%x != want=%x", rows, cols, negate, r, c, math.Float64bits(gotScal), math.Float64bits(want))
					}
				}
			}
		}
	}
}
