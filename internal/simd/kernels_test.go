package simd_test

import (
	"math"
	"testing"

	"gopt/internal/simd"
)

func TestSIMDKernels(t *testing.T) {
	n := 53 // non-multiple of 4 and 8 to test vector + tail paths
	a := make([]float64, n)
	b := make([]float64, n)
	lb := make([]float64, n)
	ub := make([]float64, n)

	var wantDot float64
	var wantMinAct, wantMaxAct float64
	for i := range n {
		a[i] = float64(i-20) * 0.5
		b[i] = float64(i+1) * 0.25
		lb[i] = -1.0
		ub[i] = 3.0
		wantDot += a[i] * b[i]
		p1 := a[i] * lb[i]
		p2 := a[i] * ub[i]
		wantMinAct += min(p1, p2)
		wantMaxAct += max(p1, p2)
	}

	gotDot := simd.Dot64(a, b)
	if math.Abs(gotDot-wantDot) > 1e-11 {
		t.Fatalf("Dot64 mismatch: got %v, want %v", gotDot, wantDot)
	}

	gotMinAct, gotMaxAct := simd.RowActivityBoundsFinite(a, lb, ub)
	if math.Abs(gotMinAct-wantMinAct) > 1e-11 || math.Abs(gotMaxAct-wantMaxAct) > 1e-11 {
		t.Fatalf("RowActivityBoundsFinite mismatch: got (%v, %v), want (%v, %v)",
			gotMinAct, gotMaxAct, wantMinAct, wantMaxAct)
	}

	dst := append([]float64(nil), b...)
	simd.Axpy64(dst, a, -2.5)
	for i := range n {
		want := b[i] - 2.5*a[i]
		if math.Abs(dst[i]-want) > 1e-11 {
			t.Fatalf("Axpy64 mismatch at %d: got %v, want %v", i, dst[i], want)
		}
	}

	absMax := simd.AbsMax64(a)
	if math.Abs(absMax-16.0) > 1e-12 {
		t.Fatalf("AbsMax64 mismatch: got %v, want 16.0", absMax)
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
