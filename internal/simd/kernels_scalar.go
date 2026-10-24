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
	b = b[:n]

	var s0, s1, s2, s3 float64
	for len(a) >= 4 && len(b) >= 4 {
		s0 += a[0] * b[0]
		s1 += a[1] * b[1]
		s2 += a[2] * b[2]
		s3 += a[3] * b[3]
		a = a[4:]
		b = b[4:]
	}
	sum := (s0 + s1) + (s2 + s3)
	if len(a) > 0 {
		b = b[:len(a)]
		for i, ai := range a {
			sum += ai * b[i]
		}
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
	src = src[:n]

	for len(dst) >= 4 && len(src) >= 4 {
		dst[0] += float64(alpha * src[0])
		dst[1] += float64(alpha * src[1])
		dst[2] += float64(alpha * src[2])
		dst[3] += float64(alpha * src[3])
		dst = dst[4:]
		src = src[4:]
	}
	if len(dst) > 0 {
		src = src[:len(dst)]
		for i, sv := range src {
			dst[i] += float64(alpha * sv)
		}
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
	for len(dst) >= 4 {
		dst[0] *= alpha
		dst[1] *= alpha
		dst[2] *= alpha
		dst[3] *= alpha
		dst = dst[4:]
	}
	for i := range dst {
		dst[i] *= alpha
	}
}

// AbsMax64Scalar returns max_i |x[i]| using a scalar loop.
func AbsMax64Scalar(x []float64) float64 {
	return AbsMax64(x)
}

// AbsMax64 returns max_i |x[i]|.
func AbsMax64(x []float64) float64 {
	var res float64
	for _, v := range x {
		res = max(res, math.Abs(v))
	}
	return res
}

// AllAbsLE64Scalar reports whether every element x[i] is finite with |x[i]| <= limit.
func AllAbsLE64Scalar(x []float64, limit float64) bool {
	for len(x) >= 4 {
		if !(math.Abs(x[0]) <= limit &&
			math.Abs(x[1]) <= limit &&
			math.Abs(x[2]) <= limit &&
			math.Abs(x[3]) <= limit) {
			return false
		}
		x = x[4:]
	}
	for _, v := range x {
		if !(math.Abs(v) <= limit) {
			return false
		}
	}
	return true
}

// AllAbsLE64With reports whether every element x[i] is finite with |x[i]| <= limit.
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
	minRed := math.Inf(1)
	if n == 0 {
		return minRed
	}
	cSlice := rowCost[:n]
	vSlice := v[:n]
	for len(cSlice) >= 4 && len(vSlice) >= 4 {
		r0 := float64(cSlice[0] - vSlice[0])
		r1 := float64(cSlice[1] - vSlice[1])
		r2 := float64(cSlice[2] - vSlice[2])
		r3 := float64(cSlice[3] - vSlice[3])
		if r1 < r0 {
			r0 = r1
		}
		if r3 < r2 {
			r2 = r3
		}
		if r2 < r0 {
			r0 = r2
		}
		if r0 < minRed {
			minRed = r0
		}
		cSlice = cSlice[4:]
		vSlice = vSlice[4:]
	}
	if len(cSlice) > 0 {
		vSlice = vSlice[:len(cSlice)]
		for j, cj := range cSlice {
			if r := float64(cj - vSlice[j]); r < minRed {
				minRed = r
			}
		}
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
	cSlice := costRow[:n]
	vSlice := v[:n]

	inf := math.Inf(1)
	m0, m1, m2, m3 := inf, inf, inf, inf
	for len(dSlice) >= 4 && len(cSlice) >= 4 && len(vSlice) >= 4 {
		d0 := (cSlice[0] - vSlice[0]) - ui
		d1 := (cSlice[1] - vSlice[1]) - ui
		d2 := (cSlice[2] - vSlice[2]) - ui
		d3 := (cSlice[3] - vSlice[3]) - ui
		dSlice[0] = d0
		dSlice[1] = d1
		dSlice[2] = d2
		dSlice[3] = d3
		if d0 < m0 {
			m0 = d0
		}
		if d1 < m1 {
			m1 = d1
		}
		if d2 < m2 {
			m2 = d2
		}
		if d3 < m3 {
			m3 = d3
		}
		dSlice = dSlice[4:]
		cSlice = cSlice[4:]
		vSlice = vSlice[4:]
	}
	minVal := min(min(m0, m1), min(m2, m3))
	if len(dSlice) > 0 {
		cSlice = cSlice[:len(dSlice)]
		vSlice = vSlice[:len(dSlice)]
		for i, ci := range cSlice {
			d := (ci - vSlice[i]) - ui
			dSlice[i] = d
			if d < minVal {
				minVal = d
			}
		}
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
	vSlice := v[:n]

	j := 0
	for len(cSlice) >= 4 && len(vSlice) >= 4 {
		r0 := cSlice[0] - vSlice[0]
		r1 := cSlice[1] - vSlice[1]
		r2 := cSlice[2] - vSlice[2]
		r3 := cSlice[3] - vSlice[3]
		if r0 < u2 || r1 < u2 || r2 < u2 || r3 < u2 {
			if r0 < u2 {
				if r0 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r0, j
				} else {
					u2, j2 = r0, j
				}
			}
			if r1 < u2 {
				if r1 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r1, j+1
				} else {
					u2, j2 = r1, j+1
				}
			}
			if r2 < u2 {
				if r2 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r2, j+2
				} else {
					u2, j2 = r2, j+2
				}
			}
			if r3 < u2 {
				if r3 < u1 {
					u2, j2 = u1, j1
					u1, j1 = r3, j+3
				} else {
					u2, j2 = r3, j+3
				}
			}
		}
		cSlice = cSlice[4:]
		vSlice = vSlice[4:]
		j += 4
	}
	if len(cSlice) > 0 {
		vSlice = vSlice[:len(cSlice)]
		for k, ck := range cSlice {
			r := ck - vSlice[k]
			if r < u2 {
				idx := j + k
				if r < u1 {
					u2, j2 = u1, j1
					u1, j1 = r, idx
				} else {
					u2, j2 = r, idx
				}
			}
		}
	}
	return u1, u2, j1, j2
}
