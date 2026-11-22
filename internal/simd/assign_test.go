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
		if math.Float64bits(gotSIMD) != math.Float64bits(gotScal) || math.Float64bits(gotOff) != math.Float64bits(gotScal) {
			t.Fatalf("n=%d: MinNoNaN64=(%v/0x%016x, scalar=%v/0x%016x, off=%v/0x%016x)",
				n, gotSIMD, math.Float64bits(gotSIMD), gotScal, math.Float64bits(gotScal), gotOff, math.Float64bits(gotOff))
		}

		// Signed-zero canonicalization parity: +0.0 before -0.0 and -0.0 before +0.0 must both
		// return canonical +0.0 across scalar, AVX2, and AVX-512 tiers.
		if n >= 2 {
			negZero := math.Copysign(0, -1)
			for _, negFirst := range []bool{false, true} {
				z := make([]float64, n)
				for i := range n {
					z[i] = 1.0
				}
				if negFirst {
					z[0] = negZero
					z[min(4, n-1)] = 0.0
				} else {
					z[0] = 0.0
					z[min(4, n-1)] = negZero
				}
				zSIMD := simd.MinNoNaN64(z)
				zScal := simd.MinNoNaN64Scalar(z)
				if math.Float64bits(zSIMD) != 0 || math.Float64bits(zScal) != 0 {
					t.Fatalf("n=%d negFirst=%v: MinNoNaN64=(0x%016x, scalar=0x%016x), want +0.0 (0x0)",
						n, negFirst, math.Float64bits(zSIMD), math.Float64bits(zScal))
				}
			}
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

func TestCollectLE64AndFillIntParity(t *testing.T) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	for _, tier := range availableTiers() {
		t.Run(tier, func(t *testing.T) {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			rng := rand.New(rand.NewPCG(1313, 1414))
			nan := math.NaN()

			for _, n := range assignKernelSizes {
				// FillInt parity
				gotFill := make([]int, n)
				wantFill := make([]int, n)
				simd.FillInt(gotFill, 42+n)
				simd.FillIntScalar(wantFill, 42+n)
				if !slices.Equal(gotFill, wantFill) {
					t.Fatalf("FillInt n=%d mismatch", n)
				}

				// CollectLE64 parity across sparse hits, NaN sentinels, and dense ties > cap(out).
				x := make([]float64, n)
				limit := 5.0
				for i := range n {
					switch rng.IntN(6) {
					case 0:
						x[i] = nan
					case 1:
						x[i] = limit - rng.Float64()*2.0
					case 2:
						x[i] = limit
					default:
						x[i] = limit + 0.1 + rng.Float64()*20.0
					}
				}

				for _, start := range []int{0, 1, 7, max(0, n-5)} {
					for _, bufCap := range []int{1, 4, 16, 64} {
						var gotAll, wantAll []int
						bufS := make([]int, bufCap)
						bufR := make([]int, bufCap)
						for pos := start; pos < n; {
							nextS, cntS := simd.CollectLE64(x, pos, limit, bufS)
							nextR, cntR := simd.CollectLE64Scalar(x, pos, limit, bufR)
							if nextS != nextR || cntS != cntR || !slices.Equal(bufS[:cntS], bufR[:cntR]) {
								t.Fatalf("CollectLE64 single-call n=%d start=%d pos=%d buf=%d: SIMD (next=%d,cnt=%d,%v) != Scalar (next=%d,cnt=%d,%v)",
									n, start, pos, bufCap, nextS, cntS, bufS[:cntS], nextR, cntR, bufR[:cntR])
							}
							gotAll = append(gotAll, bufS[:cntS]...)
							wantAll = append(wantAll, bufR[:cntR]...)
							if nextS <= pos && cntS == 0 {
								break
							}
							pos = nextS
						}
						if !slices.Equal(gotAll, wantAll) {
							t.Fatalf("CollectLE64 n=%d start=%d buf=%d: got=%v want=%v", n, start, bufCap, gotAll, wantAll)
						}
					}
				}

				// Explicit tail-boundary parity when out fills before the unaligned vector tail
				// and all trailing tail elements exceed limit.
				if n >= 35 {
					tailX := make([]float64, n)
					for i := range n {
						tailX[i] = limit + 1.0
					}
					tailX[0] = limit
					tailX[1] = limit - 1.0
					outS := make([]int, 2)
					outR := make([]int, 2)
					nextS, cntS := simd.CollectLE64(tailX, 0, limit, outS)
					nextR, cntR := simd.CollectLE64Scalar(tailX, 0, limit, outR)
					if nextS != n || nextR != n || cntS != 2 || cntR != 2 || !slices.Equal(outS, outR) {
						t.Fatalf("CollectLE64 tail-boundary n=%d: SIMD (next=%d,cnt=%d,%v) != Scalar (next=%d,cnt=%d,%v)",
							n, nextS, cntS, outS, nextR, cntR, outR)
					}
				}
			}
		})
	}
}

func TestScanNearTightRow64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(1313, 1717))
	negZero := math.Copysign(0, -1)

	for _, n := range assignKernelSizes {
		rCost := make([]float64, n)
		v := make([]float64, n)
		ui := 12.25
		limit := 1e-12
		for j := range n {
			v[j] = (rng.Float64() - 0.5) * 20.0
			switch j % 11 {
			case 0:
				rCost[j] = math.Inf(1) // forbidden edge: must be skipped
			case 1:
				rCost[j] = ui + v[j] // exact tight edge: must be emitted
			case 2:
				rCost[j] = ui + v[j] + limit // boundary == limit: must be emitted
			case 3:
				rCost[j] = ui + v[j] - 1e-13 // negative violation: must be emitted
			case 4:
				rCost[j] = math.NaN() // NaN: must be emitted
			case 5:
				rCost[j] = math.Inf(-1) // -Inf: must be emitted
			case 6:
				rCost[j] = math.MaxFloat64 // > nearTightFiniteCap: must be emitted
			case 7:
				rCost[j] = negZero
				v[j] = -ui
			case 8:
				rCost[j] = ui + v[j] + math.SmallestNonzeroFloat64 // subnormal slack: must be emitted
			default:
				rCost[j] = ui + v[j] + 1.0 + rng.Float64()*50.0 // clearly slack: skipped
			}
		}

		for _, start := range []int{-2, 0, 1, 3, 7, max(0, n-5), n} {
			for _, bufCap := range []int{0, 1, 2, 3, 7, 16, 64} {
				bufS := make([]int, bufCap)
				bufR := make([]int, bufCap)
				bufOff := make([]int, bufCap)
				pos := start
				var gotAll, wantAll []int
				for iter := 0; iter <= n+2; iter++ {
					nextS, cntS := simd.ScanNearTightRow64(rCost, v, pos, ui, limit, bufS)
					nextR, cntR := simd.ScanNearTightRow64Scalar(rCost, v, pos, ui, limit, bufR)
					nextOff, cntOff := simd.ScanNearTightRow64With(rCost, v, pos, ui, limit, bufOff, true)
					if nextS != nextR || cntS != cntR || !slices.Equal(bufS[:cntS], bufR[:cntR]) {
						t.Fatalf("ScanNearTightRow64 n=%d start=%d pos=%d buf=%d: SIMD=(next=%d,cnt=%d,%v) != Scalar=(next=%d,cnt=%d,%v)",
							n, start, pos, bufCap, nextS, cntS, bufS[:cntS], nextR, cntR, bufR[:cntR])
					}
					if nextOff != nextR || cntOff != cntR || !slices.Equal(bufOff[:cntOff], bufR[:cntR]) {
						t.Fatalf("ScanNearTightRow64With(off) n=%d start=%d pos=%d buf=%d: off=(%d,%d,%v) != scalar=(%d,%d,%v)",
							n, start, pos, bufCap, nextOff, cntOff, bufOff[:cntOff], nextR, cntR, bufR[:cntR])
					}
					gotAll = append(gotAll, bufS[:cntS]...)
					wantAll = append(wantAll, bufR[:cntR]...)
					if nextS >= n || bufCap == 0 {
						break
					}
					if nextS <= pos && cntS == 0 {
						t.Fatalf("ScanNearTightRow64 made no progress at n=%d pos=%d buf=%d", n, pos, bufCap)
					}
					pos = nextS
				}
				if !slices.Equal(gotAll, wantAll) {
					t.Fatalf("ScanNearTightRow64 aggregate n=%d start=%d buf=%d: got=%v want=%v", n, start, bufCap, gotAll, wantAll)
				}
			}
		}

		// Explicit tail-boundary parity when out fills before unaligned tail and trailing entries skip.
		if n >= 35 {
			tailC := make([]float64, n)
			tailV := make([]float64, n)
			for j := range n {
				tailC[j] = 100.0
			}
			tailC[0] = 0.0
			tailC[1] = -1.0
			outS := make([]int, 2)
			outR := make([]int, 2)
			nextS, cntS := simd.ScanNearTightRow64(tailC, tailV, 0, 0.0, 1e-12, outS)
			nextR, cntR := simd.ScanNearTightRow64Scalar(tailC, tailV, 0, 0.0, 1e-12, outR)
			if nextS != n || nextR != n || cntS != 2 || cntR != 2 || !slices.Equal(outS, outR) {
				t.Fatalf("ScanNearTightRow64 tail-boundary n=%d: SIMD=(%d,%d,%v) != Scalar=(%d,%d,%v)",
					n, nextS, cntS, outS, nextR, cntR, outR)
			}
		}
	}
}

func TestScanNearTightMatrix64Parity(t *testing.T) {
	rng := rand.New(rand.NewPCG(2027, 3037))
	negZero := math.Copysign(0, -1)

	type rcPair struct{ r, c int }
	collectPairs := func(rows, cols []int, cnt int) []rcPair {
		out := make([]rcPair, cnt)
		for k := range cnt {
			out[k] = rcPair{rows[k], cols[k]}
		}
		slices.SortFunc(out, func(a, b rcPair) int {
			if a.r != b.r {
				return a.r - b.r
			}
			return a.c - b.c
		})
		return out
	}

	for _, n := range assignKernelSizes {
		m := max(1, min(n, 9))
		cost := make([]float64, m*n)
		u := make([]float64, m)
		v := make([]float64, n)
		rowLimits := make([]float64, m)
		col4row := make([]int, m)
		for j := range n {
			v[j] = (rng.Float64() - 0.5) * 20.0
		}
		for i := range m {
			u[i] = (rng.Float64() - 0.5) * 15.0
			rowLimits[i] = 1e-12 * float64(i+1)
			col4row[i] = (i * 3) % max(1, n)
			for j := range n {
				idx := i*n + j
				switch (i*7 + j) % 11 {
				case 0:
					cost[idx] = math.Inf(1) // forbidden: skipped
				case 1:
					cost[idx] = u[i] + v[j] // tight
				case 2:
					cost[idx] = u[i] + v[j] + rowLimits[i] // exact boundary
				case 3:
					cost[idx] = u[i] + v[j] - 1e-13 // negative violation
				case 4:
					cost[idx] = math.NaN() // NaN: emitted unless matched
				case 5:
					cost[idx] = math.Inf(-1) // -Inf: emitted unless matched
				case 6:
					cost[idx] = negZero
					if j == col4row[i] {
						cost[idx] = u[i] + v[j]
					}
				case 7:
					cost[idx] = u[i] + v[j] + math.SmallestNonzeroFloat64
				default:
					cost[idx] = u[i] + v[j] + 1.0 + rng.Float64()*50.0
				}
			}
		}

		fullCap := m*n + 4
		rS, cS := make([]int, fullCap), make([]int, fullCap)
		rR, cR := make([]int, fullCap), make([]int, fullCap)
		rOff, cOff := make([]int, fullCap), make([]int, fullCap)
		cntS, ovS := simd.ScanNearTightMatrix64(m, n, cost, u, v, rowLimits, col4row, rS, cS)
		cntR, ovR := simd.ScanNearTightMatrix64Scalar(m, n, cost, u, v, rowLimits, col4row, rR, cR)
		cntOff, ovOff := simd.ScanNearTightMatrix64With(m, n, cost, u, v, rowLimits, col4row, rOff, cOff, true)
		if ovS || ovR || ovOff {
			t.Fatalf("ScanNearTightMatrix64 unexpected overflow m=%d n=%d: simd=%v scalar=%v off=%v", m, n, ovS, ovR, ovOff)
		}
		pS := collectPairs(rS, cS, cntS)
		pR := collectPairs(rR, cR, cntR)
		pOff := collectPairs(rOff, cOff, cntOff)
		if !slices.Equal(pS, pR) || !slices.Equal(pOff, pR) {
			t.Fatalf("ScanNearTightMatrix64 parity m=%d n=%d: simd=%d scalar=%d off=%d", m, n, len(pS), len(pR), len(pOff))
		}
		for _, p := range pS {
			if p.c == col4row[p.r] {
				t.Fatalf("ScanNearTightMatrix64 emitted matched cell (%d,%d)", p.r, p.c)
			}
		}

		// Overflow parity when capacity is smaller than total hits.
		if cntR > 0 {
			for _, capTry := range []int{0, 1, cntR / 2, cntR - 1, cntR} {
				orS, ocS := make([]int, capTry), make([]int, capTry)
				orR, ocR := make([]int, capTry), make([]int, capTry)
				kS, fS := simd.ScanNearTightMatrix64(m, n, cost, u, v, rowLimits, col4row, orS, ocS)
				kR, fR := simd.ScanNearTightMatrix64Scalar(m, n, cost, u, v, rowLimits, col4row, orR, ocR)
				wantOv := capTry < cntR
				if fS != wantOv || fR != wantOv {
					t.Fatalf("ScanNearTightMatrix64 overflow m=%d n=%d cap=%d cnt=%d: simd=%v scalar=%v want=%v",
						m, n, capTry, cntR, fS, fR, wantOv)
				}
				if !wantOv {
					if !slices.Equal(collectPairs(orS, ocS, kS), pR) {
						t.Fatalf("ScanNearTightMatrix64 exact-cap mismatch m=%d n=%d cap=%d", m, n, capTry)
					}
				} else if kS != capTry || kR != capTry {
					t.Fatalf("ScanNearTightMatrix64 overflow count m=%d n=%d cap=%d: simd=%d scalar=%d", m, n, capTry, kS, kR)
				}
			}
		}
	}
}

func TestCheckIntAssignKernelsParity(t *testing.T) {
	rng := rand.New(rand.NewPCG(4242, 7373))
	limit := 1e290
	intLim := 1e9
	negZero := math.Copysign(0, -1)

	for _, n := range assignKernelSizes {
		base := make([]float64, n)
		for i := range n {
			base[i] = float64(rng.IntN(2001) - 1000)
		}
		if n > 0 {
			base[0] = negZero
		}
		if n > 2 {
			base[n-1] = -intLim
		}

		variants := []struct {
			name   string
			mutate func([]float64)
		}{
			{"allInt", func(_ []float64) {}},
			{"fracFirst", func(s []float64) {
				if len(s) > 0 {
					s[0] = 1.5
				}
			}},
			{"fracMid", func(s []float64) {
				if len(s) > 0 {
					s[len(s)/2] = -0.25
				}
			}},
			{"fracLast", func(s []float64) {
				if len(s) > 0 {
					s[len(s)-1] = 3.125
				}
			}},
			{"aboveIntLim", func(s []float64) {
				if len(s) > 0 {
					s[len(s)-1] = intLim + 1
				}
			}},
			{"negAboveIntLim", func(s []float64) {
				if len(s) > 0 {
					s[0] = -(intLim + 1)
				}
			}},
			{"posInf", func(s []float64) {
				if len(s) > 0 {
					s[len(s)/2] = math.Inf(1)
				}
			}},
			{"negInf", func(s []float64) {
				if len(s) > 0 {
					s[0] = math.Inf(-1)
				}
			}},
			{"nan", func(s []float64) {
				if len(s) > 0 {
					s[len(s)-1] = math.NaN()
				}
			}},
			{"hugeFinite", func(s []float64) {
				if len(s) > 0 {
					s[0] = 1e300
				}
			}},
		}

		for _, tc := range variants {
			src := slices.Clone(base)
			tc.mutate(src)

			vS, iS := simd.AllAbsLECheckInt64With(src, limit, intLim, false)
			vR, iR := simd.AllAbsLECheckInt64Scalar(src, limit, intLim)
			vO, iO := simd.AllAbsLECheckInt64With(src, limit, intLim, true)
			if vS != vR || iS != iR || vO != vR || iO != iR {
				t.Fatalf("AllAbsLECheckInt64 n=%d %s: simd=(%v,%v) scalar=(%v,%v) off=(%v,%v)",
					n, tc.name, vS, iS, vR, iR, vO, iO)
			}

			dstS := make([]float64, n)
			dstR := make([]float64, n)
			nvS, niS := simd.ValidateAndNegateCheckInt64With(dstS, src, limit, intLim, false)
			nvR, niR := simd.ValidateAndNegateCheckInt64Scalar(dstR, src, limit, intLim)
			if nvS != nvR || niS != niR {
				t.Fatalf("ValidateAndNegateCheckInt64 n=%d %s: simd=(%v,%v) scalar=(%v,%v)",
					n, tc.name, nvS, niS, nvR, niR)
			}
			for j := range n {
				if math.Float64bits(dstS[j]) != math.Float64bits(dstR[j]) && !(math.IsNaN(dstS[j]) && math.IsNaN(dstR[j])) {
					t.Fatalf("ValidateAndNegateCheckInt64 dst mismatch n=%d %s j=%d: %x != %x",
						n, tc.name, j, math.Float64bits(dstS[j]), math.Float64bits(dstR[j]))
				}
			}
		}

		// ColReduceValidateCheckIntRows64 parity over multi-row matrices.
		if n > 0 {
			rows := 5
			mat := make([]float64, rows*n)
			for idx := range mat {
				mat[idx] = float64(rng.IntN(500) - 250)
			}
			for _, tc := range variants {
				cur := slices.Clone(mat)
				tc.mutate(cur[n:]) // put anomaly after row 0 as well as row 0
				if tc.name == "fracFirst" || tc.name == "negInf" {
					tc.mutate(cur)
				}
				vS, vR := make([]float64, n), make([]float64, n)
				rS, rR := make([]int, n), make([]int, n)
				okS, intS := simd.ColReduceValidateCheckIntRows64With(n, 0, rows, cur, vS, rS, limit, intLim, false)
				okR, intR := simd.ColReduceValidateCheckIntRows64Scalar(n, 0, rows, cur, vR, rR, limit, intLim)
				if okS != okR || intS != intR {
					t.Fatalf("ColReduceValidateCheckIntRows64 n=%d %s: simd=(%v,%v) scalar=(%v,%v)",
						n, tc.name, okS, intS, okR, intR)
				}
				if okS {
					for j := range n {
						if math.Float64bits(vS[j]) != math.Float64bits(vR[j]) || rS[j] != rR[j] {
							t.Fatalf("ColReduceValidateCheckIntRows64 out mismatch n=%d %s j=%d: simd=(%v,%d) scalar=(%v,%d)",
								n, tc.name, j, vS[j], rS[j], vR[j], rR[j])
						}
					}
				}
			}
		}
	}
}

func TestMultiTierAssignKernelsParity(t *testing.T) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	for _, tier := range availableTiers() {
		t.Run(tier, func(t *testing.T) {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			TestRelaxRow64Parity(t)
			TestMinNoNaN64Parity(t)
			TestMinReducedSkip64Parity(t)
			TestValidateAndNegate64Parity(t)
			TestColReduceRows64Parity(t)
			TestTranspose64Parity(t)
			TestScanNearTightRow64Parity(t)
			TestScanNearTightMatrix64Parity(t)
			TestCheckIntAssignKernelsParity(t)
		})
	}
}
