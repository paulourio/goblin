package simd_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"gopt/internal/simd"
)

// kernelSizes covers the scalar-only range, the n = 256 dispatch threshold, and AVX2 vector
// bodies with every tail length (n mod 4 and n mod 8).
var kernelSizes = []int{0, 1, 3, 4, 7, 53, 255, 256, 257, 258, 259, 263, 264, 512, 1031}

func randVec(r *rand.Rand, n int) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = (r.Float64() - 0.5) * math.Pow(10, float64(r.IntN(7)-3))
	}
	return v
}

// refDot is the documented reduction order: four lanes over i mod 4, combined as
// (s0+s1)+(s2+s3), then the scalar tail. Explicit conversions forbid FMA contraction.
func refDot(a, b []float64) float64 {
	n := min(len(a), len(b))
	var s [4]float64
	i := 0
	for ; i+4 <= n; i += 4 {
		for k := range 4 {
			s[k] = float64(s[k] + float64(a[i+k]*b[i+k]))
		}
	}
	sum := float64(float64(s[0]+s[1]) + float64(s[2]+s[3]))
	for ; i < n; i++ {
		sum = float64(sum + float64(a[i]*b[i]))
	}
	return sum
}

func TestDot64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range kernelSizes {
		a, b := randVec(r, n), randVec(r, n)
		got, want := simd.Dot64(a, b), refDot(a, b)
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("n=%d (hardware=%v): Dot64=%v, reference=%v", n, simd.UsingHardwareSIMD(), got, want)
		}
	}
}

func TestAxpy64BitParity(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range kernelSizes {
		dst, src := randVec(r, n), randVec(r, n)
		alpha := -2.5 + r.Float64()
		want := make([]float64, n)
		for i := range want {
			want[i] = float64(dst[i] + float64(alpha*src[i]))
		}
		simd.Axpy64(dst, src, alpha)
		for i := range n {
			if math.Float64bits(dst[i]) != math.Float64bits(want[i]) {
				t.Fatalf("n=%d i=%d: Axpy64=%v, reference=%v", n, i, dst[i], want[i])
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
