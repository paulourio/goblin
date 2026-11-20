//go:build !goexperiment.simd || !amd64

package simd

import (
	"math"
	"sync/atomic"
)

var simdDisabled atomic.Bool

// HardwareSIMDAvailable returns false when compiled without GOEXPERIMENT=simd on amd64.
func HardwareSIMDAvailable() bool { return false }

// SetDisabled toggles the process-wide SIMD disable flag.
func SetDisabled(disabled bool) { simdDisabled.Store(disabled) }

// IsDisabled reports whether SIMD has been disabled via SetDisabled.
func IsDisabled() bool { return simdDisabled.Load() }

// UsingHardwareSIMD returns false when compiled without GOEXPERIMENT=simd on amd64.
func UsingHardwareSIMD() bool { return false }

// UsingHardwareSIMDWith reports whether hardware SIMD is active given a per-solve disable flag.
func UsingHardwareSIMDWith(_ bool) bool { return false }

// Dot64Scalar computes the dot product sum_i (a[i] * b[i]) using 4-way loop unrolling.
func Dot64Scalar(a, b []float64) float64 {
	return Dot64(a, b)
}

// Dot64 computes the dot product sum_i (a[i] * b[i]) using 4-way loop unrolling.
func Dot64(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	a = a[:n]
	b = b[:len(a)]

	var s0, s1, s2, s3 float64
	i := 0
	for ; i <= len(a)-4; i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	sum := (s0 + s1) + (s2 + s3)
	for ; i < len(a); i++ {
		sum += a[i] * b[i]
	}
	return sum
}

// Axpy64Scalar computes dst[i] += alpha * src[i] using 4-way scalar loop unrolling.
func Axpy64Scalar(dst, src []float64, alpha float64) {
	Axpy64(dst, src, alpha)
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
	src = src[:len(dst)]

	i := 0
	for ; i <= len(dst)-4; i += 4 {
		dst[i] += float64(alpha * src[i])
		dst[i+1] += float64(alpha * src[i+1])
		dst[i+2] += float64(alpha * src[i+2])
		dst[i+3] += float64(alpha * src[i+3])
	}
	for ; i < len(dst); i++ {
		dst[i] += float64(alpha * src[i])
	}
}

// Scale64Scalar computes dst[i] *= alpha using 4-way scalar loop unrolling.
func Scale64Scalar(dst []float64, alpha float64) {
	Scale64(dst, alpha)
}

// Scale64 computes dst[i] *= alpha using 4-way loop unrolling.
func Scale64(dst []float64, alpha float64) {
	if len(dst) == 0 || alpha == 1 {
		return
	}
	i := 0
	for ; i <= len(dst)-4; i += 4 {
		dst[i] *= alpha
		dst[i+1] *= alpha
		dst[i+2] *= alpha
		dst[i+3] *= alpha
	}
	for ; i < len(dst); i++ {
		dst[i] *= alpha
	}
}

// AbsMax64Scalar returns max_i |x[i]| using 4-way scalar loop unrolling.
func AbsMax64Scalar(x []float64) float64 {
	return AbsMax64(x)
}

// AbsMax64 returns max_i |x[i]|.
func AbsMax64(x []float64) float64 {
	var m0, m1, m2, m3 float64
	i := 0
	for ; i <= len(x)-4; i += 4 {
		m0 = max(m0, math.Abs(x[i]))
		m1 = max(m1, math.Abs(x[i+1]))
		m2 = max(m2, math.Abs(x[i+2]))
		m3 = max(m3, math.Abs(x[i+3]))
	}
	res := max(max(m0, m1), max(m2, m3))
	for ; i < len(x); i++ {
		res = max(res, math.Abs(x[i]))
	}
	return res
}

// AllAbsLE64Scalar reports whether every element x[i] satisfies |x[i]| <= limit.
func AllAbsLE64Scalar(x []float64, limit float64) bool {
	i := 0
	for ; i <= len(x)-4; i += 4 {
		if !(math.Abs(x[i]) <= limit &&
			math.Abs(x[i+1]) <= limit &&
			math.Abs(x[i+2]) <= limit &&
			math.Abs(x[i+3]) <= limit) {
			return false
		}
	}
	for ; i < len(x); i++ {
		if !(math.Abs(x[i]) <= limit) {
			return false
		}
	}
	return true
}

// AllAbsLE64With reports whether every element x[i] satisfies |x[i]| <= limit.
func AllAbsLE64With(x []float64, limit float64, _ bool) bool {
	return AllAbsLE64Scalar(x, limit)
}

// RowActivityBoundsFiniteScalar computes minimum and maximum row activity using a scalar loop.
func RowActivityBoundsFiniteScalar(row, lb, ub []float64) (minAct, maxAct float64) {
	return RowActivityBoundsFinite(row, lb, ub)
}

// RowActivityBoundsFinite computes the minimum and maximum row activity
// assuming all lb[j] and ub[j] are finite.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	if n == 0 {
		return 0, 0
	}
	row = row[:n]
	lb = lb[:n]
	ub = ub[:n]
	for i, r := range row {
		p1 := r * lb[i]
		p2 := r * ub[i]
		minAct += min(p1, p2)
		maxAct += max(p1, p2)
	}
	return minAct, maxAct
}

// MinReducedSlice64Scalar returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)).
func MinReducedSlice64Scalar(rowCost, v []float64) float64 {
	n := min(len(rowCost), len(v))
	inf := math.Inf(1)
	if n == 0 {
		return inf
	}
	cSlice := rowCost[:n]
	vSlice := v[:len(cSlice)]
	m0, m1, m2, m3 := inf, inf, inf, inf
	j := 0
	for ; j <= len(cSlice)-4; j += 4 {
		m0 = min(m0, float64(cSlice[j]-vSlice[j]))
		m1 = min(m1, float64(cSlice[j+1]-vSlice[j+1]))
		m2 = min(m2, float64(cSlice[j+2]-vSlice[j+2]))
		m3 = min(m3, float64(cSlice[j+3]-vSlice[j+3]))
	}
	minRed := min(min(m0, m1), min(m2, m3))
	for ; j < len(cSlice); j++ {
		minRed = min(minRed, float64(cSlice[j]-vSlice[j]))
	}
	if minRed == 0 {
		minRed = 0
	}
	return minRed
}

// MinReducedSlice64With returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)).
func MinReducedSlice64With(rowCost, v []float64, _ bool) float64 {
	return MinReducedSlice64Scalar(rowCost, v)
}

// InitReducedRow64Scalar computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
func InitReducedRow64Scalar(dst, costRow, v []float64, ui float64) float64 {
	return InitReducedRow64(dst, costRow, v, ui)
}

// InitReducedRow64With computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
func InitReducedRow64With(dst, costRow, v []float64, ui float64, _ bool) float64 {
	return InitReducedRow64(dst, costRow, v, ui)
}

// InitReducedRow64 computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j]
// using 4-way scalar loop unrolling.
func InitReducedRow64(dst, costRow, v []float64, ui float64) float64 {
	n := min(len(dst), min(len(costRow), len(v)))
	if n == 0 {
		return math.Inf(1)
	}
	dSlice := dst[:n]
	cSlice := costRow[:len(dSlice)]
	vSlice := v[:len(dSlice)]

	inf := math.Inf(1)
	m0, m1, m2, m3 := inf, inf, inf, inf
	i := 0
	for ; i <= len(dSlice)-4; i += 4 {
		d0 := (cSlice[i] - vSlice[i]) - ui
		d1 := (cSlice[i+1] - vSlice[i+1]) - ui
		d2 := (cSlice[i+2] - vSlice[i+2]) - ui
		d3 := (cSlice[i+3] - vSlice[i+3]) - ui
		dSlice[i] = d0
		dSlice[i+1] = d1
		dSlice[i+2] = d2
		dSlice[i+3] = d3
		m0 = min(m0, d0)
		m1 = min(m1, d1)
		m2 = min(m2, d2)
		m3 = min(m3, d3)
	}
	minVal := min(min(m0, m1), min(m2, m3))
	for ; i < len(dSlice); i++ {
		d := (cSlice[i] - vSlice[i]) - ui
		dSlice[i] = d
		minVal = min(minVal, d)
	}
	if minVal == 0 {
		minVal = 0
	}
	return minVal
}

// Min2ReducedRow64Scalar computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j] over 0 <= j < min(len(costRow), len(v)).
func Min2ReducedRow64Scalar(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64(costRow, v)
}

// Min2ReducedRow64With computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j].
func Min2ReducedRow64With(costRow, v []float64, _ bool) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64(costRow, v)
}

// Min2ReducedRow64 computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j] using 4-way unrolled fast-filtering.
func Min2ReducedRow64(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	n := min(len(costRow), len(v))
	u1, u2 = math.Inf(1), math.Inf(1)
	j1, j2 = 0, -1
	if n == 0 {
		return u1, u2, j1, j2
	}
	cSlice := costRow[:n]
	vSlice := v[:len(cSlice)]

	j := 0
	for ; j <= len(cSlice)-4; j += 4 {
		r0 := cSlice[j] - vSlice[j]
		r1 := cSlice[j+1] - vSlice[j+1]
		r2 := cSlice[j+2] - vSlice[j+2]
		r3 := cSlice[j+3] - vSlice[j+3]
		if !(r0 >= u2 && r1 >= u2 && r2 >= u2 && r3 >= u2) {
			if !(r0 >= u2) {
				if math.IsNaN(r0) {
					return r0, r0, j, -1
				}
				if r0 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r0, j
				} else {
					u2, j2 = r0, j
				}
			}
			if !(r1 >= u2) {
				if math.IsNaN(r1) {
					return r1, r1, j + 1, -1
				}
				if r1 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r1, j+1
				} else {
					u2, j2 = r1, j+1
				}
			}
			if !(r2 >= u2) {
				if math.IsNaN(r2) {
					return r2, r2, j + 2, -1
				}
				if r2 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r2, j+2
				} else {
					u2, j2 = r2, j+2
				}
			}
			if !(r3 >= u2) {
				if math.IsNaN(r3) {
					return r3, r3, j + 3, -1
				}
				if r3 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r3, j+3
				} else {
					u2, j2 = r3, j+3
				}
			}
		}
	}
	for ; j < len(cSlice); j++ {
		r := cSlice[j] - vSlice[j]
		if !(r >= u2) {
			if math.IsNaN(r) {
				return r, r, j, -1
			}
			if r < u1 {
				u2, j2 = u1, j1
				u1, j1 = r, j
			} else {
				u2, j2 = r, j
			}
		}
	}
	return u1, u2, j1, j2
}
