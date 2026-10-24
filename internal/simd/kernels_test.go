package simd_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

// kernelSizes covers the scalar-only range, the n = 32 dispatch threshold, and AVX2 vector
// bodies with every tail length (n mod 4 and n mod 8).
var kernelSizes = []int{0, 1, 3, 4, 7, 15, 31, 32, 33, 34, 35, 39, 40, 53, 255, 256, 257, 258, 259, 263, 264, 512, 1031}

func randVec(r *rand.Rand, n int) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = (r.Float64() - 0.5) * math.Pow(10, float64(r.IntN(7)-3))
	}
	return v
}

// refDot computes the reference dot product and its L1 magnitude scale.
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
		alpha := -2.5 + r.Float64()
		want := make([]float64, n)
		for i := range want {
			want[i] = dst[i] + float64(alpha*src[i])
		}
		simd.Axpy64(dst, src, alpha)
		for i := range n {
			if dst[i] != want[i] {
				t.Fatalf("n=%d i=%d: Axpy64=%x, reference=%x", n, i, math.Float64bits(dst[i]), math.Float64bits(want[i]))
			}
		}
	}
}

func TestScale64AndAbsMax64(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, n := range kernelSizes {
		x := randVec(r, n)
		wantMax := 0.0
		for _, v := range x {
			wantMax = max(wantMax, math.Abs(v))
		}
		if got := simd.AbsMax64(x); got != wantMax {
			t.Fatalf("n=%d: AbsMax64=%v, want %v", n, got, wantMax)
		}
		want := make([]float64, n)
		for i := range x {
			want[i] = x[i] * -1.75
		}
		simd.Scale64(x, -1.75)
		for i := range n {
			if x[i] != want[i] {
				t.Fatalf("n=%d i=%d: Scale64=%v, want %v", n, i, x[i], want[i])
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
		tol := 1e-14 * max(1, scale) // the AVX2 path sums lanes in a different order
		if math.Abs(gotMin-wantMin) > tol || math.Abs(gotMax-wantMax) > tol {
			t.Fatalf("n=%d: got (%v, %v), want (%v, %v)", n, gotMin, gotMax, wantMin, wantMax)
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
		if math.Float64bits(gotMin) != math.Float64bits(wantMin) {
			t.Fatalf("n=%d: InitReducedRow64 min=%v, want %v", n, gotMin, wantMin)
		}
		if math.Float64bits(gotScalarMin) != math.Float64bits(wantMin) {
			t.Fatalf("n=%d: InitReducedRow64Scalar min=%v, want %v", n, gotScalarMin, wantMin)
		}
		if math.Float64bits(gotDisabledMin) != math.Float64bits(wantMin) {
			t.Fatalf("n=%d: InitReducedRow64With(disable=true) min=%v, want %v", n, gotDisabledMin, wantMin)
		}
		for i := range n {
			if math.Float64bits(dstSIMD[i]) != math.Float64bits(want[i]) {
				t.Fatalf("n=%d i=%d: dstSIMD=%v, want %v", n, i, dstSIMD[i], want[i])
			}
			if math.Float64bits(dstScalar[i]) != math.Float64bits(want[i]) {
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
			if math.Float64bits(u1) != math.Float64bits(wantU1) || math.Float64bits(u2) != math.Float64bits(wantU2) || j1 != wantJ1 || j2 != wantJ2 {
				t.Fatalf("n=%d tied=%d: Min2ReducedRow64=(%v,%v,%d,%d), want (%v,%v,%d,%d)",
					n, tied, u1, u2, j1, j2, wantU1, wantU2, wantJ1, wantJ2)
			}
			su1, su2, sj1, sj2 := simd.Min2ReducedRow64Scalar(costRow, v)
			if math.Float64bits(su1) != math.Float64bits(wantU1) || math.Float64bits(su2) != math.Float64bits(wantU2) || sj1 != wantJ1 || sj2 != wantJ2 {
				t.Fatalf("n=%d tied=%d: Min2ReducedRow64Scalar=(%v,%v,%d,%d), want (%v,%v,%d,%d)",
					n, tied, su1, su2, sj1, sj2, wantU1, wantU2, wantJ1, wantJ2)
			}
			du1, du2, dj1, dj2 := simd.Min2ReducedRow64With(costRow, v, true)
			if math.Float64bits(du1) != math.Float64bits(wantU1) || math.Float64bits(du2) != math.Float64bits(wantU2) || dj1 != wantJ1 || dj2 != wantJ2 {
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
		gotSIMD := simd.MinReducedSlice64With(costRow, v, false)
		gotScalar := simd.MinReducedSlice64Scalar(costRow, v)
		if math.Float64bits(gotSIMD) != math.Float64bits(wantMin) || math.Float64bits(gotScalar) != math.Float64bits(wantMin) {
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
