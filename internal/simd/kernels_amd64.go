//go:build goexperiment.simd && amd64

// Package simd provides hardware-accelerated AVX2/FMA vector kernels via Go 1.27's
// simd/archsimd package, paired with a pure-Go scalar fallback when built without
// GOEXPERIMENT=simd or when SIMD is disabled at runtime via Option.
package simd

import (
	"math"
	"simd/archsimd"
	"sync/atomic"
	"unsafe"
)

const minAVX2Len = 32

var simdDisabled atomic.Bool

// HardwareSIMDAvailable returns true when compiled with GOEXPERIMENT=simd on amd64.
func HardwareSIMDAvailable() bool { return true }

// SetDisabled toggles the process-wide SIMD disable flag.
func SetDisabled(disabled bool) { simdDisabled.Store(disabled) }

// IsDisabled reports whether SIMD has been disabled via SetDisabled.
func IsDisabled() bool { return simdDisabled.Load() }

// UsingHardwareSIMD returns true when compiled with GOEXPERIMENT=simd on amd64 and not disabled.
func UsingHardwareSIMD() bool { return !simdDisabled.Load() }

// UsingHardwareSIMDWith reports whether hardware SIMD is active given a per-solve disable flag.
func UsingHardwareSIMDWith(disable bool) bool { return !disable && !simdDisabled.Load() }

// Dot64Scalar computes the dot product sum_i (a[i] * b[i]) using 4-way scalar loop unrolling.
func Dot64Scalar(a, b []float64) float64 {
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

// Dot64 computes the dot product sum_i (a[i] * b[i]).
// Slices of length < 32 use 4-way scalar accumulators; slices of length >= 32 dispatch to
// dual 256-bit AVX2/FMA vector accumulators with VZEROUPPER cleanup.
func Dot64(a, b []float64) float64 {
	if simdDisabled.Load() {
		return Dot64Scalar(a, b)
	}
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	a = a[:n]
	b = b[:n]
	if n >= minAVX2Len {
		return dot64AVX2(a, b)
	}
	return Dot64Scalar(a, b)
}

func dot64AVX2(a, b []float64) float64 {
	n := len(a)
	ap := unsafe.Pointer(&a[0])
	bp := unsafe.Pointer(&b[0])

	acc0 := archsimd.BroadcastFloat64x4(0)
	acc1 := archsimd.BroadcastFloat64x4(0)
	i := 0
	for ; i+8 <= n; i += 8 {
		va0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(ap, i*8)))
		vb0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(bp, i*8)))
		va1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(ap, (i+4)*8)))
		vb1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(bp, (i+4)*8)))
		acc0 = va0.MulAdd(vb0, acc0)
		acc1 = va1.MulAdd(vb1, acc1)
	}
	for ; i+4 <= n; i += 4 {
		va := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(ap, i*8)))
		vb := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(bp, i*8)))
		acc0 = va.MulAdd(vb, acc0)
	}
	acc0 = acc0.Add(acc1)

	var lanes [4]float64
	acc0.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()
	sum := (lanes[0] + lanes[1]) + (lanes[2] + lanes[3])

	for ; i < n; i++ {
		sum += *(*float64)(unsafe.Add(ap, i*8)) * *(*float64)(unsafe.Add(bp, i*8))
	}
	return sum
}

// Axpy64Scalar computes dst[i] += alpha * src[i] using 4-way scalar loop unrolling.
func Axpy64Scalar(dst, src []float64, alpha float64) {
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

// Axpy64 computes dst[i] += alpha * src[i].
// Slices of length < 32 use 4-way scalar unrolling; slices of length >= 32 dispatch to
// 256-bit AVX2 vector instructions with VZEROUPPER cleanup.
func Axpy64(dst, src []float64, alpha float64) {
	if simdDisabled.Load() {
		Axpy64Scalar(dst, src, alpha)
		return
	}
	if alpha == 0 {
		return
	}
	n := min(len(dst), len(src))
	if n == 0 {
		return
	}
	dst = dst[:n]
	src = src[:n]
	if n >= minAVX2Len {
		axpy64AVX2(dst, src, alpha)
		return
	}
	Axpy64Scalar(dst, src, alpha)
}

func axpy64AVX2(dst, src []float64, alpha float64) {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	sp := unsafe.Pointer(&src[0])

	valpha := archsimd.BroadcastFloat64x4(alpha)
	i := 0
	for ; i+8 <= n; i += 8 {
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))
		s0 := (*[4]float64)(unsafe.Add(sp, i*8))
		d1 := (*[4]float64)(unsafe.Add(dp, (i+4)*8))
		s1 := (*[4]float64)(unsafe.Add(sp, (i+4)*8))

		vs0 := archsimd.LoadFloat64x4Array(s0)
		vd0 := archsimd.LoadFloat64x4Array(d0)
		vs1 := archsimd.LoadFloat64x4Array(s1)
		vd1 := archsimd.LoadFloat64x4Array(d1)

		vd0.Add(vs0.Mul(valpha)).StoreArray(d0)
		vd1.Add(vs1.Mul(valpha)).StoreArray(d1)
	}
	for ; i+4 <= n; i += 4 {
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))
		s0 := (*[4]float64)(unsafe.Add(sp, i*8))
		vs := archsimd.LoadFloat64x4Array(s0)
		vd := archsimd.LoadFloat64x4Array(d0)
		vd.Add(vs.Mul(valpha)).StoreArray(d0)
	}
	archsimd.ClearAVXUpperBits()
	for ; i < n; i++ {
		dpElem := (*float64)(unsafe.Add(dp, i*8))
		*dpElem += float64(alpha * *(*float64)(unsafe.Add(sp, i*8)))
	}
}

// Scale64Scalar computes dst[i] *= alpha using 4-way scalar loop unrolling.
func Scale64Scalar(dst []float64, alpha float64) {
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

// Scale64 computes dst[i] *= alpha using AVX2 vector multiplication.
func Scale64(dst []float64, alpha float64) {
	if simdDisabled.Load() {
		Scale64Scalar(dst, alpha)
		return
	}
	n := len(dst)
	if n == 0 || alpha == 1 {
		return
	}
	if n < minAVX2Len {
		Scale64Scalar(dst, alpha)
		return
	}
	dp := unsafe.Pointer(&dst[0])
	valpha := archsimd.BroadcastFloat64x4(alpha)
	i := 0
	for ; i+8 <= n; i += 8 {
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))
		archsimd.LoadFloat64x4Array(d0).Mul(valpha).StoreArray(d0)

		d1 := (*[4]float64)(unsafe.Add(dp, (i+4)*8))
		archsimd.LoadFloat64x4Array(d1).Mul(valpha).StoreArray(d1)
	}
	for ; i+4 <= n; i += 4 {
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))
		archsimd.LoadFloat64x4Array(d0).Mul(valpha).StoreArray(d0)
	}
	archsimd.ClearAVXUpperBits()
	for ; i < n; i++ {
		*(*float64)(unsafe.Add(dp, i*8)) *= alpha
	}
}

// AbsMax64Scalar returns max_i |x[i]| using a scalar loop.
func AbsMax64Scalar(x []float64) float64 {
	var res float64
	for _, v := range x {
		res = max(res, math.Abs(v))
	}
	return res
}

// AbsMax64 returns max_i |x[i]| using AVX2 Abs and Max instructions.
func AbsMax64(x []float64) float64 {
	n := len(x)
	if simdDisabled.Load() || n < minAVX2Len {
		return AbsMax64Scalar(x)
	}
	xp := unsafe.Pointer(&x[0])
	vmax := archsimd.BroadcastFloat64x4(0)
	i := 0
	for ; i+4 <= n; i += 4 {
		v := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, i*8))).Abs()
		vmax = vmax.Max(v)
	}
	var lanes [4]float64
	vmax.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()
	res := max(max(lanes[0], lanes[1]), max(lanes[2], lanes[3]))
	for ; i < n; i++ {
		res = max(res, math.Abs(*(*float64)(unsafe.Add(xp, i*8))))
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
func AllAbsLE64With(x []float64, limit float64, disableSIMD bool) bool {
	n := len(x)
	if disableSIMD || simdDisabled.Load() || n < minAVX2Len {
		return AllAbsLE64Scalar(x, limit)
	}
	xp := unsafe.Pointer(&x[0])
	vlim := archsimd.BroadcastFloat64x4(limit)
	i := 0
	for ; i+8 <= n; i += 8 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, i*8))).Abs()
		v1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, (i+4)*8))).Abs()
		if v0.LessEqual(vlim).And(v1.LessEqual(vlim)).ToBits() != 0xf {
			archsimd.ClearAVXUpperBits()
			return false
		}
	}
	for ; i+4 <= n; i += 4 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, i*8))).Abs()
		if v0.LessEqual(vlim).ToBits() != 0xf {
			archsimd.ClearAVXUpperBits()
			return false
		}
	}
	archsimd.ClearAVXUpperBits()
	for ; i < n; i++ {
		if !(math.Abs(*(*float64)(unsafe.Add(xp, i*8))) <= limit) {
			return false
		}
	}
	return true
}

// RowActivityBoundsFiniteScalar computes minimum and maximum row activity using a scalar loop.
func RowActivityBoundsFiniteScalar(row, lb, ub []float64) (minAct, maxAct float64) {
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

// RowActivityBoundsFinite computes the minimum and maximum row activity
// minAct = sum_j min(row[j]*lb[j], row[j]*ub[j]) and
// maxAct = sum_j max(row[j]*lb[j], row[j]*ub[j])
// assuming all lb[j] and ub[j] are finite.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	if simdDisabled.Load() || n < minAVX2Len {
		return RowActivityBoundsFiniteScalar(row, lb, ub)
	}
	rp := unsafe.Pointer(&row[0])
	lp := unsafe.Pointer(&lb[0])
	up := unsafe.Pointer(&ub[0])

	vminAcc := archsimd.BroadcastFloat64x4(0)
	vmaxAcc := archsimd.BroadcastFloat64x4(0)

	i := 0
	for ; i+4 <= n; i += 4 {
		vr := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rp, i*8)))
		vl := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(lp, i*8)))
		vu := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(up, i*8)))
		p1 := vr.Mul(vl)
		p2 := vr.Mul(vu)
		vminAcc = vminAcc.Add(p1.Min(p2))
		vmaxAcc = vmaxAcc.Add(p1.Max(p2))
	}

	var minLanes, maxLanes [4]float64
	vminAcc.StoreArray(&minLanes)
	vmaxAcc.StoreArray(&maxLanes)
	archsimd.ClearAVXUpperBits()
	minAct = (minLanes[0] + minLanes[1]) + (minLanes[2] + minLanes[3])
	maxAct = (maxLanes[0] + maxLanes[1]) + (maxLanes[2] + maxLanes[3])

	for ; i < n; i++ {
		rv := *(*float64)(unsafe.Add(rp, i*8))
		p1 := rv * *(*float64)(unsafe.Add(lp, i*8))
		p2 := rv * *(*float64)(unsafe.Add(up, i*8))
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

// MinReducedSlice64With returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)),
// dispatching to AVX2 when n >= 32 and SIMD is enabled.
func MinReducedSlice64With(rowCost, v []float64, disableSIMD bool) float64 {
	n := min(len(rowCost), len(v))
	if disableSIMD || simdDisabled.Load() || n < minAVX2Len {
		return MinReducedSlice64Scalar(rowCost, v)
	}
	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vmin0 := archsimd.BroadcastFloat64x4(math.Inf(1))
	vmin1 := vmin0
	j := 0
	for ; j+8 <= n; j += 8 {
		c0 := (*[4]float64)(unsafe.Add(cp, j*8))
		v0 := (*[4]float64)(unsafe.Add(vp, j*8))
		c1 := (*[4]float64)(unsafe.Add(cp, (j+4)*8))
		v1 := (*[4]float64)(unsafe.Add(vp, (j+4)*8))
		vmin0 = vmin0.Min(archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0)))
		vmin1 = vmin1.Min(archsimd.LoadFloat64x4Array(c1).Sub(archsimd.LoadFloat64x4Array(v1)))
	}
	for ; j+4 <= n; j += 4 {
		c0 := (*[4]float64)(unsafe.Add(cp, j*8))
		v0 := (*[4]float64)(unsafe.Add(vp, j*8))
		vmin0 = vmin0.Min(archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0)))
	}
	vmin0 = vmin0.Min(vmin1)
	var lanes [4]float64
	vmin0.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()
	minRed := min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))
	for ; j < n; j++ {
		r := float64(*(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8)))
		if r < minRed {
			minRed = r
		}
	}
	return minRed
}

// InitReducedRow64Scalar computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j]
// using 4-way scalar loop unrolling.
func InitReducedRow64Scalar(dst, costRow, v []float64, ui float64) float64 {
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

// InitReducedRow64With computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j],
// honoring a per-call disableSIMD flag in addition to the process-wide flag.
func InitReducedRow64With(dst, costRow, v []float64, ui float64, disableSIMD bool) float64 {
	if disableSIMD || simdDisabled.Load() {
		return InitReducedRow64Scalar(dst, costRow, v, ui)
	}
	n := min(len(dst), min(len(costRow), len(v)))
	if n >= minAVX2Len {
		return initReducedRow64AVX2(dst[:n], costRow[:n], v[:n], ui)
	}
	return InitReducedRow64Scalar(dst, costRow, v, ui)
}

// InitReducedRow64 computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
// Slices of length < 32 use 4-way scalar unrolling; slices of length >= 32 dispatch to AVX2.
func InitReducedRow64(dst, costRow, v []float64, ui float64) float64 {
	return InitReducedRow64With(dst, costRow, v, ui, false)
}

func initReducedRow64AVX2(dst, costRow, v []float64, ui float64) float64 {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	cp := unsafe.Pointer(&costRow[0])
	vp := unsafe.Pointer(&v[0])

	vui := archsimd.BroadcastFloat64x4(ui)
	vmin := archsimd.BroadcastFloat64x4(math.Inf(1))

	i := 0
	for ; i+8 <= n; i += 8 {
		c0 := (*[4]float64)(unsafe.Add(cp, i*8))
		v0 := (*[4]float64)(unsafe.Add(vp, i*8))
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))

		c1 := (*[4]float64)(unsafe.Add(cp, (i+4)*8))
		v1 := (*[4]float64)(unsafe.Add(vp, (i+4)*8))
		d1 := (*[4]float64)(unsafe.Add(dp, (i+4)*8))

		vd0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0)).Sub(vui)
		vd1 := archsimd.LoadFloat64x4Array(c1).Sub(archsimd.LoadFloat64x4Array(v1)).Sub(vui)

		vd0.StoreArray(d0)
		vd1.StoreArray(d1)

		vmin = vmin.Min(vd0).Min(vd1)
	}
	for ; i+4 <= n; i += 4 {
		c0 := (*[4]float64)(unsafe.Add(cp, i*8))
		v0 := (*[4]float64)(unsafe.Add(vp, i*8))
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))

		vd0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0)).Sub(vui)
		vd0.StoreArray(d0)
		vmin = vmin.Min(vd0)
	}

	var lanes [4]float64
	vmin.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()
	minVal := min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))

	for ; i < n; i++ {
		d := (*(*float64)(unsafe.Add(cp, i*8)) - *(*float64)(unsafe.Add(vp, i*8))) - ui
		*(*float64)(unsafe.Add(dp, i*8)) = d
		if d < minVal {
			minVal = d
		}
	}
	if minVal == 0 {
		minVal = 0
	}
	return minVal
}

// Min2ReducedRow64Scalar computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j] using 4-way unrolled scalar fast-filtering.
func Min2ReducedRow64Scalar(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
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

// Min2ReducedRow64With computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j], honoring a per-call disableSIMD flag.
func Min2ReducedRow64With(costRow, v []float64, disableSIMD bool) (u1, u2 float64, j1, j2 int) {
	if disableSIMD || simdDisabled.Load() {
		return Min2ReducedRow64Scalar(costRow, v)
	}
	n := min(len(costRow), len(v))
	if n >= 64 {
		return min2ReducedRow64AVX2(costRow[:n], v[:n])
	}
	return Min2ReducedRow64Scalar(costRow, v)
}

// Min2ReducedRow64 computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j].
func Min2ReducedRow64(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64With(costRow, v, false)
}

func min2ReducedRow64AVX2(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	n := len(costRow)
	cp := unsafe.Pointer(&costRow[0])
	vp := unsafe.Pointer(&v[0])

	// Seed the first 16 elements directly via pointer arithmetic so u2 is tight before AVX2 streaming.
	u1, u2 = math.Inf(1), math.Inf(1)
	j1, j2 = 0, -1
	for j := range 16 {
		r := *(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8))
		if r < u2 {
			if r < u1 {
				u2, j2 = u1, j1
				u1, j1 = r, j
			} else {
				u2, j2 = r, j
			}
		}
	}

	vu2 := archsimd.BroadcastFloat64x4(u2)
	j := 16
	for ; j+8 <= n; j += 8 {
		c0 := (*[4]float64)(unsafe.Add(cp, j*8))
		v0 := (*[4]float64)(unsafe.Add(vp, j*8))
		c1 := (*[4]float64)(unsafe.Add(cp, (j+4)*8))
		v1 := (*[4]float64)(unsafe.Add(vp, (j+4)*8))

		vr0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0))
		vr1 := archsimd.LoadFloat64x4Array(c1).Sub(archsimd.LoadFloat64x4Array(v1))
		if vr0.Min(vr1).Less(vu2).ToBits() != 0 {
			m0 := vr0.Less(vu2).ToBits()
			m1 := vr1.Less(vu2).ToBits()
			var b0, b1 [4]float64
			if m0 != 0 {
				vr0.StoreArray(&b0)
			}
			if m1 != 0 {
				vr1.StoreArray(&b1)
			}
			archsimd.ClearAVXUpperBits()
			if m0 != 0 {
				for k, r := range b0 {
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
			if m1 != 0 {
				for k, r := range b1 {
					if r < u2 {
						idx := j + 4 + k
						if r < u1 {
							u2, j2 = u1, j1
							u1, j1 = r, idx
						} else {
							u2, j2 = r, idx
						}
					}
				}
			}
			vu2 = archsimd.BroadcastFloat64x4(u2)
		}
	}
	for ; j+4 <= n; j += 4 {
		c0 := (*[4]float64)(unsafe.Add(cp, j*8))
		v0 := (*[4]float64)(unsafe.Add(vp, j*8))
		vr0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0))
		if vr0.Less(vu2).ToBits() != 0 {
			var b0 [4]float64
			vr0.StoreArray(&b0)
			archsimd.ClearAVXUpperBits()
			for k, r := range b0 {
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
			vu2 = archsimd.BroadcastFloat64x4(u2)
		}
	}
	archsimd.ClearAVXUpperBits()
	for ; j < n; j++ {
		r := *(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8))
		if r < u2 {
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
