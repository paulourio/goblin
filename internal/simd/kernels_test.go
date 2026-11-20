package simd_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

// ExhaustiveSizes builds every length from 0 through 65 plus larger AVX2
// dispatch thresholds and non-multiple tail lengths up to 1031.
func exhaustiveSizes() []int {
	sizes := make([]int, 0, 78)
	for n := range 66 {
		sizes = append(sizes, n)
	}
	return append(sizes, 100, 128, 255, 256, 257, 258, 259, 263, 264, 272, 512, 1031)
}

var kernelSizes = exhaustiveSizes()

func sameFloat64Bits(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Float64bits(a) == math.Float64bits(b)
}

func samplePositions(n int) []int {
	if n <= 0 {
		return nil
	}
	raw := []int{0, 1, 3, 4, 7, 8, 15, 16, 20, 31, 32, n / 2, n - 2, n - 1}
	seen := make(map[int]bool, len(raw))
	pos := make([]int, 0, len(raw))
	for _, idx := range raw {
		if idx >= 0 && idx < n && !seen[idx] {
			seen[idx] = true
			pos = append(pos, idx)
		}
	}
	return pos
}

func randVec(r *rand.Rand, n int) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = (r.Float64() - 0.5) * math.Pow(10, float64(r.IntN(7)-3))
	}
	return v
}

// ReferenceDot computes the reference dot product and its L1 magnitude scale.
func refDot(a, b []float64) (sum, scale float64) {
	n := min(len(a), len(b))
	for i := range n {
		p := a[i] * b[i]
		sum += p
		scale += math.Abs(p)
	}
	return sum, scale
}

func TestDot64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range kernelSizes {
		a, b := randVec(r, n), randVec(r, n)
		want, scale := refDot(a, b)
		tol := 1e-14 * max(1, scale)
		got := simd.Dot64(a, b)
		if math.Abs(got-want) > tol {
			t.Fatalf("n=%d (hardware=%v): Dot64=%v, reference=%v, tol=%v", n, simd.UsingHardwareSIMD(), got, want, tol)
		}
		gotScalar := simd.Dot64Scalar(a, b)
		if math.Abs(gotScalar-want) > tol {
			t.Fatalf("n=%d: Dot64Scalar=%v, reference=%v, tol=%v", n, gotScalar, want, tol)
		}
	}
}

func TestRuntimeSIMDDisableToggle(t *testing.T) {
	defer simd.SetDisabled(false)
	simd.SetDisabled(true)
	if !simd.IsDisabled() || simd.UsingHardwareSIMD() || simd.UsingHardwareSIMDWith(false) {
		t.Fatalf("expected SIMD disabled when SetDisabled(true)")
	}
	simd.SetDisabled(false)
	if simd.UsingHardwareSIMDWith(true) {
		t.Fatalf("expected UsingHardwareSIMDWith(true) to report false")
	}
}

func TestAxpy64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range kernelSizes {
		dst, src := randVec(r, n), randVec(r, n)
		dstScalar := append([]float64(nil), dst...)
		alpha := -2.5 + r.Float64()
		want := make([]float64, n)
		for i := range want {
			want[i] = dst[i] + float64(alpha*src[i])
		}
		simd.Axpy64(dst, src, alpha)
		simd.Axpy64Scalar(dstScalar, src, alpha)
		for i := range n {
			if !sameFloat64Bits(dst[i], want[i]) {
				t.Fatalf("n=%d i=%d: Axpy64=%x, reference=%x", n, i, math.Float64bits(dst[i]), math.Float64bits(want[i]))
			}
			if !sameFloat64Bits(dstScalar[i], want[i]) {
				t.Fatalf("n=%d i=%d: Axpy64Scalar=%x, reference=%x", n, i, math.Float64bits(dstScalar[i]), math.Float64bits(want[i]))
			}
		}
	}
}

func TestScale64AndAbsMax64(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, n := range kernelSizes {
		x := randVec(r, n)
		xScalar := append([]float64(nil), x...)
		wantMax := 0.0
		for _, v := range x {
			wantMax = max(wantMax, math.Abs(v))
		}
		if got := simd.AbsMax64(x); !sameFloat64Bits(got, wantMax) {
			t.Fatalf("n=%d: AbsMax64=%v, want %v", n, got, wantMax)
		}
		if got := simd.AbsMax64Scalar(x); !sameFloat64Bits(got, wantMax) {
			t.Fatalf("n=%d: AbsMax64Scalar=%v, want %v", n, got, wantMax)
		}
		want := make([]float64, n)
		for i := range x {
			want[i] = x[i] * -1.75
		}
		simd.Scale64(x, -1.75)
		simd.Scale64Scalar(xScalar, -1.75)
		for i := range n {
			if !sameFloat64Bits(x[i], want[i]) {
				t.Fatalf("n=%d i=%d: Scale64=%v, want %v", n, i, x[i], want[i])
			}
			if !sameFloat64Bits(xScalar[i], want[i]) {
				t.Fatalf("n=%d i=%d: Scale64Scalar=%v, want %v", n, i, xScalar[i], want[i])
			}
		}
	}
}

func TestRowActivityBoundsFinite(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	for _, n := range kernelSizes {
		row := randVec(r, n)
		lb, ub := make([]float64, n), make([]float64, n)
		var wantMin, wantMax, scale float64
		for i := range n {
			lb[i] = -1 - r.Float64()
			ub[i] = 1 + 3*r.Float64()
			p1, p2 := row[i]*lb[i], row[i]*ub[i]
			wantMin += min(p1, p2)
			wantMax += max(p1, p2)
			scale += math.Abs(p1) + math.Abs(p2)
		}
		gotMin, gotMax := simd.RowActivityBoundsFinite(row, lb, ub)
		scalMin, scalMax := simd.RowActivityBoundsFiniteScalar(row, lb, ub)
		tol := 1e-14 * max(1, scale) // AVX2 lanes sum products in a different order than the scalar loop.
		if math.Abs(gotMin-wantMin) > tol || math.Abs(gotMax-wantMax) > tol {
			t.Fatalf("n=%d: got (%v, %v), want (%v, %v)", n, gotMin, gotMax, wantMin, wantMax)
		}
		if !sameFloat64Bits(scalMin, wantMin) || !sameFloat64Bits(scalMax, wantMax) {
			t.Fatalf("n=%d: scalar got (%v, %v), want (%v, %v)", n, scalMin, scalMax, wantMin, wantMax)
		}
	}
}

func TestInitReducedRow64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	for _, n := range kernelSizes {
		costRow := randVec(r, n)
		v := randVec(r, n)
		ui := (r.Float64() - 0.5) * 10
		dstSIMD := make([]float64, n)
		dstScalar := make([]float64, n)
		want := make([]float64, n)
		wantMin := math.Inf(1)
		for i := range n {
			d := (costRow[i] - v[i]) - ui
			want[i] = d
			if d < wantMin {
				wantMin = d
			}
		}
		if wantMin == 0 {
			wantMin = 0
		}

		gotMin := simd.InitReducedRow64(dstSIMD, costRow, v, ui)
		gotScalarMin := simd.InitReducedRow64Scalar(dstScalar, costRow, v, ui)
		gotDisabledMin := simd.InitReducedRow64With(dstScalar, costRow, v, ui, true)
		if !sameFloat64Bits(gotMin, wantMin) {
			t.Fatalf("n=%d: InitReducedRow64 min=%v, want %v", n, gotMin, wantMin)
		}
		if !sameFloat64Bits(gotScalarMin, wantMin) {
			t.Fatalf("n=%d: InitReducedRow64Scalar min=%v, want %v", n, gotScalarMin, wantMin)
		}
		if !sameFloat64Bits(gotDisabledMin, wantMin) {
			t.Fatalf("n=%d: InitReducedRow64With(disable=true) min=%v, want %v", n, gotDisabledMin, wantMin)
		}
		for i := range n {
			if !sameFloat64Bits(dstSIMD[i], want[i]) {
				t.Fatalf("n=%d i=%d: dstSIMD=%v, want %v", n, i, dstSIMD[i], want[i])
			}
			if !sameFloat64Bits(dstScalar[i], want[i]) {
				t.Fatalf("n=%d i=%d: dstScalar=%v, want %v", n, i, dstScalar[i], want[i])
			}
		}
	}
}

func TestMin2ReducedRow64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	for _, n := range kernelSizes {
		for tied := range 2 {
			costRow := make([]float64, n)
			v := make([]float64, n)
			for i := range n {
				if tied == 1 {
					costRow[i] = float64(r.IntN(5))
					v[i] = float64(r.IntN(5))
				} else {
					costRow[i] = (r.Float64() - 0.5) * 100
					v[i] = (r.Float64() - 0.5) * 100
				}
			}
			wantU1, wantU2 := math.Inf(1), math.Inf(1)
			wantJ1, wantJ2 := 0, -1
			for j := range n {
				d := float64(costRow[j] - v[j])
				if d < wantU2 {
					if d < wantU1 {
						wantU2, wantJ2 = wantU1, wantJ1
						wantU1, wantJ1 = d, j
					} else {
						wantU2, wantJ2 = d, j
					}
				}
			}

			u1, u2, j1, j2 := simd.Min2ReducedRow64(costRow, v)
			if !sameFloat64Bits(u1, wantU1) || !sameFloat64Bits(u2, wantU2) || j1 != wantJ1 || j2 != wantJ2 {
				t.Fatalf("n=%d tied=%d: Min2ReducedRow64=(%v,%v,%d,%d), want (%v,%v,%d,%d)",
					n, tied, u1, u2, j1, j2, wantU1, wantU2, wantJ1, wantJ2)
			}
			su1, su2, sj1, sj2 := simd.Min2ReducedRow64Scalar(costRow, v)
			if !sameFloat64Bits(su1, wantU1) || !sameFloat64Bits(su2, wantU2) || sj1 != wantJ1 || sj2 != wantJ2 {
				t.Fatalf("n=%d tied=%d: Min2ReducedRow64Scalar=(%v,%v,%d,%d), want (%v,%v,%d,%d)",
					n, tied, su1, su2, sj1, sj2, wantU1, wantU2, wantJ1, wantJ2)
			}
			du1, du2, dj1, dj2 := simd.Min2ReducedRow64With(costRow, v, true)
			if !sameFloat64Bits(du1, wantU1) || !sameFloat64Bits(du2, wantU2) || dj1 != wantJ1 || dj2 != wantJ2 {
				t.Fatalf("n=%d tied=%d: Min2ReducedRow64With(disable=true)=(%v,%v,%d,%d), want (%v,%v,%d,%d)",
					n, tied, du1, du2, dj1, dj2, wantU1, wantU2, wantJ1, wantJ2)
			}
		}
	}
}

func TestMinReducedSlice64AndAllAbsLE64(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 14))
	const limit = 1e290
	for _, n := range kernelSizes {
		costRow := randVec(r, n)
		v := randVec(r, n)
		wantMin := math.Inf(1)
		for j := range n {
			if d := float64(costRow[j] - v[j]); d < wantMin {
				wantMin = d
			}
		}
		if wantMin == 0 {
			wantMin = 0
		}
		gotSIMD := simd.MinReducedSlice64With(costRow, v, false)
		gotScalar := simd.MinReducedSlice64Scalar(costRow, v)
		if !sameFloat64Bits(gotSIMD, wantMin) || !sameFloat64Bits(gotScalar, wantMin) {
			t.Fatalf("n=%d: MinReducedSlice64=(%v,%v), want %v", n, gotSIMD, gotScalar, wantMin)
		}
		if !simd.AllAbsLE64With(costRow, limit, false) || !simd.AllAbsLE64Scalar(costRow, limit) {
			t.Fatalf("n=%d: expected AllAbsLE64 true on normal finite slice", n)
		}
		if n > 0 {
			idx := n - 1
			saved := costRow[idx]
			for _, bad := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), 1e295, -1e295} {
				costRow[idx] = bad
				if simd.AllAbsLE64With(costRow, limit, false) || simd.AllAbsLE64Scalar(costRow, limit) {
					t.Fatalf("n=%d bad=%v: expected AllAbsLE64 false", n, bad)
				}
			}
			costRow[idx] = saved
		}
	}
}

func TestNaNParity(t *testing.T) {
	negNaN := math.Copysign(math.NaN(), -1)
	for _, n := range []int{1, 4, 7, 31, 32, 33, 39, 64, 65, 100, 128, 255, 256, 259, 272, 512} {
		for _, pos := range samplePositions(n) {
			for _, nanVal := range []float64{math.NaN(), negNaN} {
				x := make([]float64, n)
				y := make([]float64, n)
				ub := make([]float64, n)
				for i := range n {
					x[i] = float64(i+1) * 0.5
					y[i] = float64(i) * -0.25
					ub[i] = float64(i+2) * 1.25
				}
				x[pos] = nanVal

				if got, want := simd.Dot64(x, y), simd.Dot64Scalar(x, y); !(math.IsNaN(got) && math.IsNaN(want)) {
					t.Fatalf("n=%d pos=%d: Dot64 got=%v scalar=%v, want NaN", n, pos, got, want)
				}
				if got, want := simd.AbsMax64(x), simd.AbsMax64Scalar(x); !(math.IsNaN(got) && math.IsNaN(want)) {
					t.Fatalf("n=%d pos=%d: AbsMax64 got=%v scalar=%v, want NaN", n, pos, got, want)
				}
				if simd.AllAbsLE64With(x, math.Inf(1), false) || simd.AllAbsLE64Scalar(x, math.Inf(1)) {
					t.Fatalf("n=%d pos=%d: AllAbsLE64(+Inf) must be false on NaN", n, pos)
				}
				minSIMD, maxSIMD := simd.RowActivityBoundsFinite(x, y, ub)
				minScal, maxScal := simd.RowActivityBoundsFiniteScalar(x, y, ub)
				if !(math.IsNaN(minSIMD) && math.IsNaN(maxSIMD) && math.IsNaN(minScal) && math.IsNaN(maxScal)) {
					t.Fatalf("n=%d pos=%d: RowActivityBoundsFinite simd=(%v,%v) scalar=(%v,%v), want NaN",
						n, pos, minSIMD, maxSIMD, minScal, maxScal)
				}
				if got, want := simd.MinReducedSlice64With(x, y, false), simd.MinReducedSlice64Scalar(x, y); !(math.IsNaN(got) && math.IsNaN(want)) {
					t.Fatalf("n=%d pos=%d: MinReducedSlice64 got=%v scalar=%v, want NaN", n, pos, got, want)
				}
				dstSIMD := make([]float64, n)
				dstScal := make([]float64, n)
				rMinSIMD := simd.InitReducedRow64With(dstSIMD, x, y, 1.0, false)
				rMinScal := simd.InitReducedRow64Scalar(dstScal, x, y, 1.0)
				if !(math.IsNaN(rMinSIMD) && math.IsNaN(rMinScal)) {
					t.Fatalf("n=%d pos=%d: InitReducedRow64 min simd=%v scalar=%v, want NaN", n, pos, rMinSIMD, rMinScal)
				}
				for i := range n {
					if !sameFloat64Bits(dstSIMD[i], dstScal[i]) {
						t.Fatalf("n=%d pos=%d i=%d: InitReducedRow64 dst mismatch simd=%x scalar=%x",
							n, pos, i, math.Float64bits(dstSIMD[i]), math.Float64bits(dstScal[i]))
					}
				}
				u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(x, y, false)
				u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(x, y)
				if !(math.IsNaN(u1SIMD) && math.IsNaN(u2SIMD) && math.IsNaN(u1Scal) && math.IsNaN(u2Scal)) ||
					j1SIMD != pos || j1Scal != pos || j2SIMD != -1 || j2Scal != -1 {
					t.Fatalf("n=%d pos=%d: Min2ReducedRow64 simd=(%v,%v,%d,%d) scalar=(%v,%v,%d,%d), want NaN at %d",
						n, pos, u1SIMD, u2SIMD, j1SIMD, j2SIMD, u1Scal, u2Scal, j1Scal, j2Scal, pos)
				}
			}
		}
	}

	// Verify paired low-cost element at index 16 and NaN at index 20 in one 8-wide AVX2 step.
	for _, sz := range []int{64, 256, 263} {
		cost := make([]float64, sz)
		v := make([]float64, sz)
		for i := range cost {
			cost[i] = 10.0
		}
		cost[16] = 1.0
		cost[20] = math.NaN()
		u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(cost, v, false)
		u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(cost, v)
		if !(math.IsNaN(u1SIMD) && math.IsNaN(u2SIMD) && math.IsNaN(u1Scal) && math.IsNaN(u2Scal)) ||
			j1SIMD != 20 || j1Scal != 20 || j2SIMD != -1 || j2Scal != -1 {
			t.Fatalf("sz=%d paired NaN: simd=(%v,%v,%d,%d) scalar=(%v,%v,%d,%d), want NaN at 20",
				sz, u1SIMD, u2SIMD, j1SIMD, j2SIMD, u1Scal, u2Scal, j1Scal, j2Scal)
		}
	}
}

func TestSpecialIEEE754Parity(t *testing.T) {
	posInf := math.Inf(1)
	negInf := math.Inf(-1)
	negZero := math.Copysign(0, -1)
	subPos := math.SmallestNonzeroFloat64
	subNeg := -math.SmallestNonzeroFloat64
	specialVals := []float64{posInf, negInf, negZero, 0.0, subPos, subNeg, subPos * 17, -subPos * 17, math.NaN()}

	sizes := []int{0, 1, 3, 4, 7, 15, 31, 32, 33, 35, 39, 63, 64, 65, 100, 128, 256, 261, 512}

	t.Run("AxpyAndScaleSpecial", func(t *testing.T) {
		alphas := []float64{0.0, negZero, 1.0, -1.0, 0.5, -1.75, subPos, posInf, negInf, math.NaN()}
		for _, n := range sizes {
			baseDst := make([]float64, n)
			src := make([]float64, n)
			for i := range n {
				baseDst[i] = specialVals[i%len(specialVals)]
				src[i] = specialVals[(i*3+1)%len(specialVals)]
			}
			for _, alpha := range alphas {
				dSIMD := append([]float64(nil), baseDst...)
				dScal := append([]float64(nil), baseDst...)
				simd.Axpy64(dSIMD, src, alpha)
				simd.Axpy64Scalar(dScal, src, alpha)
				for i := range n {
					if !sameFloat64Bits(dSIMD[i], dScal[i]) {
						t.Fatalf("Axpy64 n=%d alpha=%v i=%d: simd=%x scalar=%x",
							n, alpha, i, math.Float64bits(dSIMD[i]), math.Float64bits(dScal[i]))
					}
				}

				sSIMD := append([]float64(nil), baseDst...)
				sScal := append([]float64(nil), baseDst...)
				simd.Scale64(sSIMD, alpha)
				simd.Scale64Scalar(sScal, alpha)
				for i := range n {
					if !sameFloat64Bits(sSIMD[i], sScal[i]) {
						t.Fatalf("Scale64 n=%d alpha=%v i=%d: simd=%x scalar=%x",
							n, alpha, i, math.Float64bits(sSIMD[i]), math.Float64bits(sScal[i]))
					}
				}
			}
		}
	})

	t.Run("AbsMaxAndAllAbsLESpecial", func(t *testing.T) {
		limits := []float64{math.NaN(), negInf, -1.0, subNeg, negZero, 0.0, subPos, 1e290, posInf}
		for _, n := range sizes {
			for _, fill := range []float64{0.0, negZero, subPos, subNeg, posInf, negInf} {
				x := make([]float64, n)
				for i := range x {
					x[i] = fill
				}
				if got, want := simd.AbsMax64(x), simd.AbsMax64Scalar(x); !sameFloat64Bits(got, want) {
					t.Fatalf("AbsMax64 n=%d fill=%v: simd=%x scalar=%x", n, fill, math.Float64bits(got), math.Float64bits(want))
				}
				for _, lim := range limits {
					got := simd.AllAbsLE64With(x, lim, false)
					want := simd.AllAbsLE64Scalar(x, lim)
					if got != want {
						t.Fatalf("AllAbsLE64 n=%d fill=%v lim=%v: simd=%v scalar=%v", n, fill, lim, got, want)
					}
				}
			}

			for _, pos := range samplePositions(n) {
				for _, inject := range []float64{posInf, negInf, subPos, subNeg, negZero, 1e295, -1e295} {
					x := make([]float64, n)
					for i := range x {
						x[i] = negZero
					}
					x[pos] = inject
					if got, want := simd.AbsMax64(x), simd.AbsMax64Scalar(x); !sameFloat64Bits(got, want) {
						t.Fatalf("AbsMax64 n=%d pos=%d inject=%v: simd=%x scalar=%x",
							n, pos, inject, math.Float64bits(got), math.Float64bits(want))
					}
					for _, lim := range limits {
						got := simd.AllAbsLE64With(x, lim, false)
						want := simd.AllAbsLE64Scalar(x, lim)
						if got != want {
							t.Fatalf("AllAbsLE64 n=%d pos=%d inject=%v lim=%v: simd=%v scalar=%v",
								n, pos, inject, lim, got, want)
						}
					}
				}
			}
		}
	})

	t.Run("DotAndRowActivitySpecial", func(t *testing.T) {
		for _, n := range sizes {
			for _, fill := range []float64{0.0, negZero, subPos, subNeg} {
				a := make([]float64, n)
				b := make([]float64, n)
				ub := make([]float64, n)
				for i := range n {
					a[i] = fill
					b[i] = 1.0
					ub[i] = 2.0
				}
				if got, want := simd.Dot64(a, b), simd.Dot64Scalar(a, b); !sameFloat64Bits(got, want) {
					t.Fatalf("Dot64 n=%d fill=%v: simd=%x scalar=%x", n, fill, math.Float64bits(got), math.Float64bits(want))
				}
				gMin, gMax := simd.RowActivityBoundsFinite(a, b, ub)
				sMin, sMax := simd.RowActivityBoundsFiniteScalar(a, b, ub)
				if !sameFloat64Bits(gMin, sMin) || !sameFloat64Bits(gMax, sMax) {
					t.Fatalf("RowActivity n=%d fill=%v: simd=(%x,%x) scalar=(%x,%x)",
						n, fill, math.Float64bits(gMin), math.Float64bits(gMax), math.Float64bits(sMin), math.Float64bits(sMax))
				}
			}

			for _, pos := range samplePositions(n) {
				for _, inject := range []float64{posInf, negInf} {
					a := make([]float64, n)
					b := make([]float64, n)
					ub := make([]float64, n)
					for i := range n {
						a[i] = 1.0
						b[i] = -1.0
						ub[i] = 2.0
					}
					a[pos] = inject
					if got, want := simd.Dot64(a, b), simd.Dot64Scalar(a, b); !sameFloat64Bits(got, want) {
						t.Fatalf("Dot64 Inf n=%d pos=%d inject=%v: simd=%v scalar=%v", n, pos, inject, got, want)
					}
					gMin, gMax := simd.RowActivityBoundsFinite(a, b, ub)
					sMin, sMax := simd.RowActivityBoundsFiniteScalar(a, b, ub)
					if !sameFloat64Bits(gMin, sMin) || !sameFloat64Bits(gMax, sMax) {
						t.Fatalf("RowActivity Inf n=%d pos=%d inject=%v: simd=(%v,%v) scalar=(%v,%v)",
							n, pos, inject, gMin, gMax, sMin, sMax)
					}
				}
			}

			if n >= 33 {
				// Combine +Inf in the AVX2 body with -Inf in the scalar tail so sums yield NaN.
				a := make([]float64, n)
				b := make([]float64, n)
				ub := make([]float64, n)
				for i := range n {
					a[i] = 1.0
					b[i] = 1.0
					ub[i] = 2.0
				}
				a[0] = posInf
				a[n-1] = negInf
				gMin, gMax := simd.RowActivityBoundsFinite(a, b, ub)
				sMin, sMax := simd.RowActivityBoundsFiniteScalar(a, b, ub)
				if !sameFloat64Bits(gMin, sMin) || !sameFloat64Bits(gMax, sMax) {
					t.Fatalf("RowActivity Inf cancellation n=%d: simd=(%v,%v) scalar=(%v,%v)", n, gMin, gMax, sMin, sMax)
				}
			}
		}
	})

	t.Run("ReducedCostSpecialAndSignedZero", func(t *testing.T) {
		for _, n := range sizes {
			// Verify signed-zero mixtures in both index orders.
			for _, negFirst := range []bool{false, true} {
				c := make([]float64, n)
				v := make([]float64, n)
				for i := range n {
					if (i%2 == 0) == negFirst {
						c[i] = negZero
					} else {
						c[i] = 0.0
					}
					v[i] = 0.0
				}
				gMin := simd.MinReducedSlice64With(c, v, false)
				sMin := simd.MinReducedSlice64Scalar(c, v)
				if !sameFloat64Bits(gMin, sMin) {
					t.Fatalf("MinReducedSlice64 signed zero n=%d negFirst=%v: simd=%x scalar=%x",
						n, negFirst, math.Float64bits(gMin), math.Float64bits(sMin))
				}

				dstSIMD := make([]float64, n)
				dstScal := make([]float64, n)
				rSIMD := simd.InitReducedRow64With(dstSIMD, c, v, 0.0, false)
				rScal := simd.InitReducedRow64Scalar(dstScal, c, v, 0.0)
				if !sameFloat64Bits(rSIMD, rScal) {
					t.Fatalf("InitReducedRow64 signed zero n=%d: simd=%x scalar=%x", n, math.Float64bits(rSIMD), math.Float64bits(rScal))
				}
				for i := range n {
					if !sameFloat64Bits(dstSIMD[i], dstScal[i]) {
						t.Fatalf("InitReducedRow64 signed zero dst n=%d i=%d: simd=%x scalar=%x",
							n, i, math.Float64bits(dstSIMD[i]), math.Float64bits(dstScal[i]))
					}
				}

				u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(c, v, false)
				u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(c, v)
				if !sameFloat64Bits(u1SIMD, u1Scal) || !sameFloat64Bits(u2SIMD, u2Scal) || j1SIMD != j1Scal || j2SIMD != j2Scal {
					t.Fatalf("Min2ReducedRow64 signed zero n=%d: simd=(%x,%x,%d,%d) scalar=(%x,%x,%d,%d)",
						n, math.Float64bits(u1SIMD), math.Float64bits(u2SIMD), j1SIMD, j2SIMD,
						math.Float64bits(u1Scal), math.Float64bits(u2Scal), j1Scal, j2Scal)
				}
			}

			// Verify Inf, Inf-Inf NaN, and subnormals at body and tail indices.
			for _, pos := range samplePositions(n) {
				for _, tc := range []struct {
					cVal, vVal, ui float64
				}{
					{posInf, 1.0, 0.5},
					{negInf, 1.0, 0.5},
					{posInf, posInf, 0.0},
					{negInf, negInf, 0.0},
					{subPos, 0.0, 0.0},
					{subNeg, 0.0, 0.0},
					{1.0, 1.0, posInf},
					{1.0, 1.0, negInf},
					{1.0, 1.0, math.NaN()},
				} {
					c := make([]float64, n)
					v := make([]float64, n)
					for i := range n {
						c[i] = float64(n-i) * 0.25
						v[i] = 0.125
					}
					c[pos] = tc.cVal
					v[pos] = tc.vVal

					gMin := simd.MinReducedSlice64With(c, v, false)
					sMin := simd.MinReducedSlice64Scalar(c, v)
					if !sameFloat64Bits(gMin, sMin) {
						t.Fatalf("MinReducedSlice64 n=%d pos=%d tc=%+v: simd=%v scalar=%v", n, pos, tc, gMin, sMin)
					}

					dstSIMD := make([]float64, n)
					dstScal := make([]float64, n)
					rSIMD := simd.InitReducedRow64With(dstSIMD, c, v, tc.ui, false)
					rScal := simd.InitReducedRow64Scalar(dstScal, c, v, tc.ui)
					if !sameFloat64Bits(rSIMD, rScal) {
						t.Fatalf("InitReducedRow64 n=%d pos=%d tc=%+v: simd=%v scalar=%v", n, pos, tc, rSIMD, rScal)
					}
					for i := range n {
						if !sameFloat64Bits(dstSIMD[i], dstScal[i]) {
							t.Fatalf("InitReducedRow64 dst n=%d pos=%d i=%d tc=%+v: simd=%x scalar=%x",
								n, pos, i, tc, math.Float64bits(dstSIMD[i]), math.Float64bits(dstScal[i]))
						}
					}

					u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(c, v, false)
					u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(c, v)
					if !sameFloat64Bits(u1SIMD, u1Scal) || !sameFloat64Bits(u2SIMD, u2Scal) || j1SIMD != j1Scal || j2SIMD != j2Scal {
						t.Fatalf("Min2ReducedRow64 n=%d pos=%d tc=%+v: simd=(%v,%v,%d,%d) scalar=(%v,%v,%d,%d)",
							n, pos, tc, u1SIMD, u2SIMD, j1SIMD, j2SIMD, u1Scal, u2Scal, j1Scal, j2Scal)
					}
				}
			}

			// Verify all-Inf slices produce identical sentinel outputs.
			cInf := make([]float64, n)
			vZero := make([]float64, n)
			for i := range cInf {
				cInf[i] = posInf
			}
			if got, want := simd.MinReducedSlice64With(cInf, vZero, false), simd.MinReducedSlice64Scalar(cInf, vZero); !sameFloat64Bits(got, want) {
				t.Fatalf("MinReducedSlice64 all +Inf n=%d: simd=%v scalar=%v", n, got, want)
			}
			u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(cInf, vZero, false)
			u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(cInf, vZero)
			if !sameFloat64Bits(u1SIMD, u1Scal) || !sameFloat64Bits(u2SIMD, u2Scal) || j1SIMD != j1Scal || j2SIMD != j2Scal {
				t.Fatalf("Min2ReducedRow64 all +Inf n=%d: simd=(%v,%v,%d,%d) scalar=(%v,%v,%d,%d)",
					n, u1SIMD, u2SIMD, j1SIMD, j2SIMD, u1Scal, u2Scal, j1Scal, j2Scal)
			}
		}
	})

	t.Run("UnequalSliceLengths", func(t *testing.T) {
		a := make([]float64, 45)
		b := make([]float64, 37)
		c := make([]float64, 51)
		for i := range a {
			a[i] = float64(i+1) * 0.5
		}
		for i := range b {
			b[i] = float64(i+2) * -0.25
		}
		for i := range c {
			c[i] = float64(i+3) * 0.75
		}
		if got, want := simd.MinReducedSlice64With(a, b, false), simd.MinReducedSlice64Scalar(a, b); !sameFloat64Bits(got, want) {
			t.Fatalf("MinReducedSlice64 unequal len: simd=%v scalar=%v", got, want)
		}
		dSIMD := make([]float64, 35)
		dScal := make([]float64, 35)
		if g, w := simd.InitReducedRow64With(dSIMD, a, b, 1.5, false), simd.InitReducedRow64Scalar(dScal, a, b, 1.5); !sameFloat64Bits(g, w) {
			t.Fatalf("InitReducedRow64 unequal len: simd=%v scalar=%v", g, w)
		}
		u1SIMD, u2SIMD, j1SIMD, j2SIMD := simd.Min2ReducedRow64With(c, a, false)
		u1Scal, u2Scal, j1Scal, j2Scal := simd.Min2ReducedRow64Scalar(c, a)
		if !sameFloat64Bits(u1SIMD, u1Scal) || !sameFloat64Bits(u2SIMD, u2Scal) || j1SIMD != j1Scal || j2SIMD != j2Scal {
			t.Fatalf("Min2ReducedRow64 unequal len: simd=(%v,%v,%d,%d) scalar=(%v,%v,%d,%d)",
				u1SIMD, u2SIMD, j1SIMD, j2SIMD, u1Scal, u2Scal, j1Scal, j2Scal)
		}
	})
}

func BenchmarkAxpy64_N56(b *testing.B) {
	dst := make([]float64, 56)
	src := make([]float64, 56)
	for i := range 56 {
		dst[i] = float64(i)
		src[i] = float64(i + 1)
	}
	b.ResetTimer()
	for range b.N {
		simd.Axpy64(dst, src, 1.25)
	}
}

func BenchmarkDot64_N56(b *testing.B) {
	x := make([]float64, 56)
	y := make([]float64, 56)
	for i := range 56 {
		x[i] = float64(i)
		y[i] = float64(i + 1)
	}
	b.ResetTimer()
	var sink float64
	for range b.N {
		sink = simd.Dot64(x, y)
	}
	_ = sink
}
