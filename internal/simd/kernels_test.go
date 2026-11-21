package simd_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

// ExhaustiveSizes builds every length from 0 through 65 plus larger AVX2/AVX-512
// dispatch thresholds and non-multiple tail lengths up to 2063.
func exhaustiveSizes() []int {
	sizes := make([]int, 0, 81)
	for n := range 66 {
		sizes = append(sizes, n)
	}
	return append(sizes, 100, 128, 255, 256, 257, 258, 259, 263, 264, 272, 512, 1031, 2048, 2055, 2063)
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

// availableTiers returns all hardware tiers executable on this host (always includes scalar;
// includes avx2 when AVX2+FMA is available; includes avx512 when AVX-512 is available).
func availableTiers() []string {
	switch simd.HardwareSIMDTier() {
	case simd.TierAVX512:
		return []string{simd.TierScalar, simd.TierAVX2, simd.TierAVX512}
	case simd.TierAVX2:
		return []string{simd.TierScalar, simd.TierAVX2}
	default:
		return []string{simd.TierScalar}
	}
}

func TestTierIntrospectionAndCaps(t *testing.T) {
	defer func() {
		_ = simd.SetMaxTier("auto")
		simd.SetDisabled(false)
	}()

	hw := simd.HardwareSIMDTier()
	if hw != simd.TierScalar && hw != simd.TierAVX2 && hw != simd.TierAVX512 {
		t.Fatalf("unexpected HardwareSIMDTier: %q", hw)
	}
	if err := simd.SetMaxTier("bogus"); err == nil {
		t.Fatal("expected error for unknown tier")
	}

	if err := simd.SetMaxTier(simd.TierScalar); err != nil {
		t.Fatalf("SetMaxTier(scalar): %v", err)
	}
	if simd.ActiveSIMDTier() != simd.TierScalar || simd.UsingHardwareSIMD() {
		t.Fatalf("expected scalar active tier, got %q (using=%v)", simd.ActiveSIMDTier(), simd.UsingHardwareSIMD())
	}

	if err := simd.SetMaxTier(simd.TierAVX2); err != nil {
		t.Fatalf("SetMaxTier(avx2): %v", err)
	}
	wantAVX2 := simd.TierScalar
	if hw != simd.TierScalar {
		wantAVX2 = simd.TierAVX2
	}
	if simd.ActiveSIMDTier() != wantAVX2 {
		t.Fatalf("SetMaxTier(avx2): active=%q want=%q (hw=%q)", simd.ActiveSIMDTier(), wantAVX2, hw)
	}

	if err := simd.SetMaxTier("auto"); err != nil {
		t.Fatalf("SetMaxTier(auto): %v", err)
	}
	if simd.ActiveSIMDTier() != hw || simd.MaxTier() != simd.TierAVX512 {
		t.Fatalf("SetMaxTier(auto): active=%q want=%q max=%q", simd.ActiveSIMDTier(), hw, simd.MaxTier())
	}
	if simd.ActiveSIMDTierWith(true) != simd.TierScalar {
		t.Fatalf("expected ActiveSIMDTierWith(true)=scalar, got %q", simd.ActiveSIMDTierWith(true))
	}
}

func TestSelectDualLeaveRow64Parity(t *testing.T) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	for _, tier := range availableTiers() {
		t.Run(tier, func(t *testing.T) {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			rng := rand.New(rand.NewPCG(777, 888))
			for _, m := range kernelSizes {
				xb := make([]float64, m)
				lb := make([]float64, m)
				ub := make([]float64, m)
				tol := make([]float64, m)
				dse := make([]float64, m)

				for r := range m {
					lb[r] = -10.0 + rng.Float64()*5.0
					ub[r] = lb[r] + 1.0 + rng.Float64()*20.0
					tol[r] = 1e-7 * (1.0 + rng.Float64())
					dse[r] = 0.25 + rng.Float64()*10.0
					xb[r] = 0.5 * (lb[r] + ub[r]) // feasible default
				}
				if m > 4 {
					lb[0] = math.Inf(-1)
					ub[1] = math.Inf(1)
					xb[0] = -1e12
					xb[1] = 1e12
				}

				// Case 1: all feasible.
				gr, gd, gv := simd.SelectDualLeaveRow64With(xb, lb, ub, tol, dse, false)
				wr, wd, wv := simd.SelectDualLeaveRow64Scalar(xb, lb, ub, tol, dse)
				if gr != wr || !sameFloat64Bits(gd, wd) || !sameFloat64Bits(gv, wv) {
					t.Fatalf("m=%d all-feasible: got=(%d,%v,%v) want=(%d,%v,%v)", m, gr, gd, gv, wr, wd, wv)
				}

				// Case 2: sparse violations at boundary/tail positions.
				for _, pos := range samplePositions(m) {
					if pos%2 == 0 {
						xb[pos] = lb[pos] - tol[pos] - (0.01 + rng.Float64())
					} else {
						xb[pos] = ub[pos] + tol[pos] + (0.01 + rng.Float64())
					}
				}
				gr, gd, gv = simd.SelectDualLeaveRow64With(xb, lb, ub, tol, dse, false)
				wr, wd, wv = simd.SelectDualLeaveRow64Scalar(xb, lb, ub, tol, dse)
				if gr != wr || !sameFloat64Bits(gd, wd) || !sameFloat64Bits(gv, wv) {
					t.Fatalf("m=%d sparse-viol: got=(%d,%v,%v) want=(%d,%v,%v)", m, gr, gd, gv, wr, wd, wv)
				}

				// Case 3: dense violations (>64 infeasible rows to exercise multi-chunk collection).
				for r := range m {
					switch r % 3 {
					case 0:
						xb[r] = lb[r] - tol[r] - float64(r+1)*1e-3
					case 1:
						xb[r] = ub[r] + tol[r] + float64(r+1)*1e-3
					}
				}
				gr, gd, gv = simd.SelectDualLeaveRow64With(xb, lb, ub, tol, dse, false)
				wr, wd, wv = simd.SelectDualLeaveRow64Scalar(xb, lb, ub, tol, dse)
				if gr != wr || !sameFloat64Bits(gd, wd) || !sameFloat64Bits(gv, wv) {
					t.Fatalf("m=%d dense-viol: got=(%d,%v,%v) want=(%d,%v,%v)", m, gr, gd, gv, wr, wd, wv)
				}
			}
		})
	}
}

func TestScanDualRatioCand64Parity(t *testing.T) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	const (
		pivotTol   = 1e-8
		harrisTol  = 1e-9
		guardDelta = 2e-12
	)
	type candRec struct {
		col   int
		ratio float64
		absA  float64
	}
	type passOut struct {
		thetaMax float64
		minRatio float64
		subTol   bool
		hasNaN   bool
		cands    []candRec
	}
	runPass := func(tab, dj []float64, status []uint8, chunks []int, leaveDir float64, bland, useVec bool) passOut {
		n := len(tab)
		negLeave := -leaveDir
		thetaMax := math.Inf(1)
		minRatio := math.Inf(1)
		var out passOut
		evalCol := func(k int) bool {
			tv := tab[k]
			if tv == 0 {
				return false
			}
			alphaBar := negLeave * tv
			st := status[k]
			if tv > pivotTol || tv < -pivotTol {
				var absA, djSign float64
				super := false
				switch st {
				case 0:
					if alphaBar <= 0 {
						return false
					}
					absA, djSign = alphaBar, 1.0
				case 1:
					if alphaBar >= 0 {
						return false
					}
					absA, djSign = -alphaBar, -1.0
				case 4:
					absA = math.Abs(alphaBar)
					super = true
				default:
					return false
				}
				if super {
					thetaMax, minRatio = 0, 0
					out.cands = append(out.cands, candRec{col: k, ratio: 0, absA: absA})
					return false
				}
				d := djSign * dj[k]
				if math.IsNaN(d) {
					out.hasNaN = true
					return true
				}
				if d < 0 {
					d = 0
				}
				if bland {
					if r0 := d / absA; r0 < minRatio {
						minRatio = r0
					}
				}
				if d <= (thetaMax+guardDelta)*absA {
					invA := 1.0 / absA
					r0 := d * invA
					if hr := (d + harrisTol) * invA; hr < thetaMax {
						thetaMax = hr
					}
					out.cands = append(out.cands, candRec{col: k, ratio: r0, absA: absA})
				}
				return false
			}
			if math.IsNaN(tv) {
				out.hasNaN = true
				return true
			}
			if (st == 0 && alphaBar > 0) || (st == 1 && alphaBar < 0) || st == 4 {
				out.subTol = true
			}
			return false
		}
		if !useVec {
			for _, ch := range chunks {
				start := ch << 6
				end := min(start+64, n)
				for k := start; k < end; k++ {
					if evalCol(k) {
						return out
					}
				}
			}
			out.thetaMax, out.minRatio = thetaMax, minRatio
			return out
		}
		var candBuf [64]int
		ci, elem := 0, 0
		for ci < len(chunks) {
			batchCap := len(candBuf)
			if !bland && math.IsInf(thetaMax, 1) {
				batchCap = 8
			}
			var cnt int
			cnt, ci, elem = simd.ScanDualRatioCand64With(
				tab, dj, status, chunks, ci, elem,
				negLeave, pivotTol, thetaMax+guardDelta, bland, false, candBuf[:batchCap],
			)
			if slices.ContainsFunc(candBuf[:cnt], evalCol) {
				return out
			}
		}
		out.thetaMax, out.minRatio = thetaMax, minRatio
		return out
	}

	equalOut := func(a, b passOut) bool {
		if a.hasNaN != b.hasNaN || a.subTol != b.subTol || len(a.cands) != len(b.cands) {
			return false
		}
		if !a.hasNaN && (!sameFloat64Bits(a.thetaMax, b.thetaMax) || !sameFloat64Bits(a.minRatio, b.minRatio)) {
			return false
		}
		for i := range a.cands {
			if a.cands[i].col != b.cands[i].col ||
				!sameFloat64Bits(a.cands[i].ratio, b.cands[i].ratio) ||
				!sameFloat64Bits(a.cands[i].absA, b.cands[i].absA) {
				return false
			}
		}
		return true
	}

	sizes := []int{1, 2, 3, 4, 7, 8, 15, 63, 64, 65, 130, 1090, 2048, 2055, 2063, 4036, 8192, 8205}
	for _, tier := range availableTiers() {
		t.Run(tier, func(t *testing.T) {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			for _, n := range sizes {
				rng := rand.New(rand.NewPCG(777, uint64(n)))
				tab := make([]float64, n)
				dj := make([]float64, n)
				st := make([]uint8, n)
				numChunks := (n + 63) >> 6
				chunks := make([]int, numChunks)
				for c := range numChunks {
					chunks[c] = c
				}
				for j := range n {
					st[j] = uint8(rng.IntN(4))
					if rng.IntN(3) != 0 {
						tab[j] = (rng.Float64() - 0.5) * 4.0
					}
					switch st[j] {
					case 0:
						dj[j] = rng.Float64()*10.0 - 0.05
					case 1:
						dj[j] = -(rng.Float64()*10.0 - 0.05)
					}
				}
				if n > 10 {
					tab[5] = math.Copysign(0, -1)
					tab[6] = 0.5 * pivotTol
					st[6] = 0
					tab[7] = -0.5 * pivotTol
					st[7] = 1
					st[n-1] = 4
					tab[n-1] = -2.5
				}
				for _, leaveDir := range []float64{1.0, -1.0} {
					for _, bland := range []bool{false, true} {
						got := runPass(tab, dj, st, chunks, leaveDir, bland, true)
						want := runPass(tab, dj, st, chunks, leaveDir, bland, false)
						if !equalOut(got, want) {
							t.Fatalf("n=%d dir=%v bland=%v mismatch: got=(%.6g,%.6g,sub=%v,nan=%v,len=%d) want=(%.6g,%.6g,sub=%v,nan=%v,len=%d)",
								n, leaveDir, bland, got.thetaMax, got.minRatio, got.subTol, got.hasNaN, len(got.cands),
								want.thetaMax, want.minRatio, want.subTol, want.hasNaN, len(want.cands))
						}
					}
				}
				// Inject NaN in tab on a fixed column and NaN in dj on a blocking column.
				if n > 16 {
					tabNaN := append([]float64(nil), tab...)
					stNaN := append([]uint8(nil), st...)
					stNaN[n-3] = 3
					tabNaN[n-3] = math.NaN()
					got := runPass(tabNaN, dj, stNaN, chunks, 1.0, false, true)
					want := runPass(tabNaN, dj, stNaN, chunks, 1.0, false, false)
					if !equalOut(got, want) || !got.hasNaN {
						t.Fatalf("n=%d tab-NaN mismatch: got=%+v want=%+v", n, got.hasNaN, want.hasNaN)
					}

					djNaN := append([]float64(nil), dj...)
					tabDJ := append([]float64(nil), tab...)
					stDJ := append([]uint8(nil), st...)
					stDJ[n-2] = 0
					tabDJ[n-2] = -2.0
					djNaN[n-2] = math.NaN()
					got = runPass(tabDJ, djNaN, stDJ, chunks, 1.0, false, true)
					want = runPass(tabDJ, djNaN, stDJ, chunks, 1.0, false, false)
					if !equalOut(got, want) || !got.hasNaN {
						t.Fatalf("n=%d dj-NaN mismatch: got=%+v want=%+v", n, got.hasNaN, want.hasNaN)
					}
				}
			}
		})
	}
}

func TestMultiTierCoreKernelsParity(t *testing.T) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	for _, tier := range availableTiers() {
		t.Run(tier, func(t *testing.T) {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			TestDot64BitParity(t)
			TestAxpy64BitParity(t)
			TestScale64AndAbsMax64(t)
			TestRowActivityBoundsFinite(t)
			TestInitReducedRow64BitParity(t)
			TestMin2ReducedRow64BitParity(t)
			TestMinReducedSlice64AndAllAbsLE64(t)
			TestNaNParity(t)
			TestSpecialIEEE754Parity(t)
		})
	}
}

func BenchmarkKernelCrossover(b *testing.B) {
	defer func() { _ = simd.SetMaxTier("auto") }()

	sizes := []int{16, 32, 64, 128, 256, 512, 1024, 2048, 4096}
	tiers := availableTiers()

	for _, n := range sizes {
		a := make([]float64, n)
		v := make([]float64, n)
		d := make([]float64, n)
		lb := make([]float64, n)
		ub := make([]float64, n)
		tol := make([]float64, n)
		dse := make([]float64, n)
		pred := make([]int, n)
		rc := make([]float64, n)
		dRelax := make([]float64, n)
		tab := make([]float64, n)
		dj := make([]float64, n)
		status := make([]uint8, n)
		chunks := make([]int, (n+63)/64)
		for ci := range chunks {
			chunks[ci] = ci
		}
		rng := rand.New(rand.NewPCG(42, uint64(n)))
		for i := range n {
			a[i] = rng.Float64() * 100.0
			v[i] = rng.Float64() * 50.0
			d[i] = 100.0 + rng.Float64()*50.0
			rc[i] = 100.0 + rng.Float64()*50.0
			dRelax[i] = 60.0
			lb[i] = 0.0
			ub[i] = 100.0
			tol[i] = 1e-7
			dse[i] = 1.0 + rng.Float64()
			// 5% active ratio-test candidates to mirror dense MAD dualRatioPass1 scans.
			switch i % 20 {
			case 0:
				tab[i] = 1.5
				dj[i] = 0.5
				status[i] = 0
			case 1:
				tab[i] = -1.5
				dj[i] = -0.5
				status[i] = 1
			default:
				tab[i] = 0
				dj[i] = 5.0
				status[i] = 0
			}
		}
		// Inject 2% infeasible rows for SelectDualLeaveRow64.
		for i := 0; i < n; i += 50 {
			a[i] = -0.5
		}

		for _, tier := range tiers {
			if err := simd.SetMaxTier(tier); err != nil {
				b.Fatalf("SetMaxTier(%s): %v", tier, err)
			}
			b.Run(fmt.Sprintf("Dot64/N=%d/%s", n, tier), func(b *testing.B) {
				var sink float64
				for range b.N {
					sink = simd.Dot64(a, v)
				}
				_ = sink
			})
			b.Run(fmt.Sprintf("Axpy64/N=%d/%s", n, tier), func(b *testing.B) {
				for range b.N {
					simd.Axpy64(d, v, 0.0)
					simd.Axpy64(d, v, 1e-12)
				}
			})
			b.Run(fmt.Sprintf("InitReducedRow64/N=%d/%s", n, tier), func(b *testing.B) {
				var sink float64
				for range b.N {
					sink = simd.InitReducedRow64(d, a, v, 1.5)
				}
				_ = sink
			})
			b.Run(fmt.Sprintf("Min2ReducedRow64/N=%d/%s", n, tier), func(b *testing.B) {
				var sink float64
				for range b.N {
					u1, _, _, _ := simd.Min2ReducedRow64(a, v)
					sink = u1
				}
				_ = sink
			})
			b.Run(fmt.Sprintf("RelaxRow64/N=%d/%s", n, tier), func(b *testing.B) {
				var sink int
				for range b.N {
					sink = simd.RelaxRow64(rc, v, dRelax, pred, 0, 1, 0.5, 1.0)
				}
				_ = sink
			})
			b.Run(fmt.Sprintf("SelectDualLeaveRow64/N=%d/%s", n, tier), func(b *testing.B) {
				var sink int
				for range b.N {
					r, _, _ := simd.SelectDualLeaveRow64With(a, lb, ub, tol, dse, false)
					sink = r
				}
				_ = sink
			})
			if n >= 256 {
				b.Run(fmt.Sprintf("ScanDualRatioCand64/N=%d/%s", n, tier), func(b *testing.B) {
					var out [128]int
					var sink int
					for range b.N {
						cnt, ci, elem := simd.ScanDualRatioCand64With(tab, dj, status, chunks, 0, 0, 1.0, 1e-9, 1.0, false, false, out[:])
						for ci < len(chunks) {
							var c2 int
							c2, ci, elem = simd.ScanDualRatioCand64With(tab, dj, status, chunks, ci, elem, 1.0, 1e-9, 1.0, false, false, out[:])
							cnt += c2
						}
						sink = cnt
					}
					_ = sink
				})
			}
		}
	}
}
