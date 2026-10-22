//go:build !goexperiment.simd || !amd64

package simd

import "math"

// UsingHardwareSIMD returns false when compiled without GOEXPERIMENT=simd on amd64.
func UsingHardwareSIMD() bool { return false }

// Dot64 computes the dot product sum_i (a[i] * b[i]) using 4-way loop unrolling.
func Dot64(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	a = a[:n]
	b = b[:n]

	var s0, s1, s2, s3 float64
	i := 0
	for ; i+4 <= n; i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	sum := (s0 + s1) + (s2 + s3)
	for ; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

// Axpy64 computes dst[i] += alpha * src[i] using 4-way loop unrolling.
func Axpy64(dst, src []float64, alpha float64) {
	if alpha == 0 {
		return
	}
	n := min(len(dst), len(src))
	if n == 0 {
		return
	}
	dst = dst[:n]
	src = src[:n]

	i := 0
	for ; i+4 <= n; i += 4 {
		dst[i] += alpha * src[i]
		dst[i+1] += alpha * src[i+1]
		dst[i+2] += alpha * src[i+2]
		dst[i+3] += alpha * src[i+3]
	}
	for ; i < n; i++ {
		dst[i] += alpha * src[i]
	}
}

// Scale64 computes dst[i] *= alpha using 4-way loop unrolling.
func Scale64(dst []float64, alpha float64) {
	n := len(dst)
	if n == 0 || alpha == 1 {
		return
	}
	i := 0
	for ; i+4 <= n; i += 4 {
		dst[i] *= alpha
		dst[i+1] *= alpha
		dst[i+2] *= alpha
		dst[i+3] *= alpha
	}
	for ; i < n; i++ {
		dst[i] *= alpha
	}
}

// AbsMax64 returns max_i |x[i]|.
func AbsMax64(x []float64) float64 {
	var res float64
	for _, v := range x {
		res = max(res, math.Abs(v))
	}
	return res
}

// RowActivityBoundsFinite computes the minimum and maximum row activity
// assuming all lb[j] and ub[j] are finite.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	for i := range n {
		p1 := row[i] * lb[i]
		p2 := row[i] * ub[i]
		minAct += min(p1, p2)
		maxAct += max(p1, p2)
	}
	return minAct, maxAct
}
