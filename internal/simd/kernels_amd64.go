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

var (
	simdDisabled atomic.Bool
	hasAVX2FMA   = archsimd.X86.AVX2() && archsimd.X86.FMA()
)

func simdOff() bool { return !hasAVX2FMA || simdDisabled.Load() }

// orderedMask64x4 returns a 4-lane mask whose lanes are set where v is ordered (not NaN).
func orderedMask64x4(v archsimd.Float64x4) archsimd.Mask64x4 {
	//nolint:gocritic // IEEE-754 self-equality emits a single VCMPEQPD to detect NaN lanes.
	return v.Equal(v)
}

// HardwareSIMDAvailable reports whether the binary was compiled with GOEXPERIMENT=simd on amd64
// and the host CPU advertises AVX2 and FMA support.
func HardwareSIMDAvailable() bool { return hasAVX2FMA }

// SetDisabled toggles the process-wide SIMD disable flag.
func SetDisabled(disabled bool) { simdDisabled.Store(disabled) }

// IsDisabled reports whether SIMD has been disabled via SetDisabled.
func IsDisabled() bool { return simdDisabled.Load() }

// UsingHardwareSIMD reports whether hardware AVX2/FMA kernels are available and enabled.
func UsingHardwareSIMD() bool { return !simdOff() }

// UsingHardwareSIMDWith reports whether hardware SIMD is active given a per-solve disable flag.
func UsingHardwareSIMDWith(disable bool) bool { return !disable && !simdOff() }

// Dot64Scalar computes the dot product sum_i (a[i] * b[i]) using 4-way scalar loop unrolling.
func Dot64Scalar(a, b []float64) float64 {
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

// Dot64 computes the dot product sum_i (a[i] * b[i]).
// Slices of length < 32 use 4-way scalar accumulators; slices of length >= 32 dispatch to
// four 256-bit AVX2/FMA vector accumulators with VZEROUPPER cleanup.
func Dot64(a, b []float64) float64 {
	if simdOff() {
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
	acc1 := acc0
	acc2 := acc0
	acc3 := acc0
	i := 0
	for ; i+16 <= n; i += 16 {
		p0 := unsafe.Add(ap, i*8)
		q0 := unsafe.Add(bp, i*8)
		acc0 = archsimd.LoadFloat64x4Array((*[4]float64)(p0)).MulAdd(archsimd.LoadFloat64x4Array((*[4]float64)(q0)), acc0)
		acc1 = archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(p0, 32))).MulAdd(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(q0, 32))), acc1)
		acc2 = archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(p0, 64))).MulAdd(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(q0, 64))), acc2)
		acc3 = archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(p0, 96))).MulAdd(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(q0, 96))), acc3)
	}
	for ; i+4 <= n; i += 4 {
		va := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(ap, i*8)))
		vb := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(bp, i*8)))
		acc0 = va.MulAdd(vb, acc0)
	}
	acc0 = acc0.Add(acc1).Add(acc2.Add(acc3))

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

// Axpy64 computes dst[i] += alpha * src[i].
// Slices of length < 32 use 4-way scalar unrolling; slices of length >= 32 dispatch to
// 256-bit AVX2 vector instructions with VZEROUPPER cleanup.
func Axpy64(dst, src []float64, alpha float64) {
	if simdOff() {
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
		d1 := (*[4]float64)(unsafe.Add(unsafe.Pointer(d0), 32))
		s1 := (*[4]float64)(unsafe.Add(unsafe.Pointer(s0), 32))

		archsimd.LoadFloat64x4Array(d0).Add(archsimd.LoadFloat64x4Array(s0).Mul(valpha)).StoreArray(d0)
		archsimd.LoadFloat64x4Array(d1).Add(archsimd.LoadFloat64x4Array(s1).Mul(valpha)).StoreArray(d1)
	}
	for ; i+4 <= n; i += 4 {
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))
		s0 := (*[4]float64)(unsafe.Add(sp, i*8))
		archsimd.LoadFloat64x4Array(d0).Add(archsimd.LoadFloat64x4Array(s0).Mul(valpha)).StoreArray(d0)
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

// Scale64 computes dst[i] *= alpha using AVX2 vector multiplication.
func Scale64(dst []float64, alpha float64) {
	if simdOff() {
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

		d1 := (*[4]float64)(unsafe.Add(unsafe.Pointer(d0), 32))
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

// AbsMax64Scalar returns max_i |x[i]| using 4-way scalar loop unrolling.
func AbsMax64Scalar(x []float64) float64 {
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

// AbsMax64 returns max_i |x[i]| using AVX2 sign-bit masking and dual Max accumulators.
func AbsMax64(x []float64) float64 {
	n := len(x)
	if simdOff() || n < minAVX2Len {
		return AbsMax64Scalar(x)
	}
	xp := unsafe.Pointer(&x[0])
	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	vmax0 := signMask.Xor(signMask).BitsToFloat64()
	vmax1 := vmax0
	ord0 := orderedMask64x4(vmax0)
	ord1 := ord0
	i := 0
	for ; i+8 <= n; i += 8 {
		p0 := unsafe.Add(xp, i*8)
		v0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0)).And(signMask).BitsToFloat64()
		v1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32))).And(signMask).BitsToFloat64()
		vmax0 = vmax0.Max(v0)
		vmax1 = vmax1.Max(v1)
		ord0 = ord0.And(orderedMask64x4(v0))
		ord1 = ord1.And(orderedMask64x4(v1))
	}
	for ; i+4 <= n; i += 4 {
		v0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(xp, i*8))).And(signMask).BitsToFloat64()
		vmax0 = vmax0.Max(v0)
		ord0 = ord0.And(orderedMask64x4(v0))
	}
	vmax0 = vmax0.Max(vmax1)
	var lanes [4]float64
	vmax0.StoreArray(&lanes)
	if ord0.And(ord1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	res := max(max(lanes[0], lanes[1]), max(lanes[2], lanes[3]))
	for ; i < n; i++ {
		res = max(res, math.Abs(*(*float64)(unsafe.Add(xp, i*8))))
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
func AllAbsLE64With(x []float64, limit float64, disableSIMD bool) bool {
	n := len(x)
	if disableSIMD || simdOff() || n < minAVX2Len {
		return AllAbsLE64Scalar(x, limit)
	}
	if !(0 <= limit) {
		return false
	}
	xp := unsafe.Pointer(&x[0])
	vlim := archsimd.BroadcastFloat64x4(limit)
	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	allOK0 := orderedMask64x4(vlim)
	allOK1 := allOK0
	i := 0
	for ; i+8 <= n; i += 8 {
		p0 := unsafe.Add(xp, i*8)
		v0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0)).And(signMask).BitsToFloat64()
		v1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32))).And(signMask).BitsToFloat64()
		allOK0 = allOK0.And(v0.LessEqual(vlim))
		allOK1 = allOK1.And(v1.LessEqual(vlim))
	}
	for ; i+4 <= n; i += 4 {
		v0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(xp, i*8))).And(signMask).BitsToFloat64()
		allOK0 = allOK0.And(v0.LessEqual(vlim))
	}
	if allOK0.And(allOK1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return false
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
	if simdOff() || n < minAVX2Len {
		return RowActivityBoundsFiniteScalar(row, lb, ub)
	}
	rp := unsafe.Pointer(&row[0])
	lp := unsafe.Pointer(&lb[0])
	up := unsafe.Pointer(&ub[0])

	vminAcc0 := archsimd.BroadcastFloat64x4(0)
	vminAcc1 := vminAcc0
	vmaxAcc0 := vminAcc0
	vmaxAcc1 := vminAcc0
	ord0 := orderedMask64x4(vminAcc0)
	ord1 := ord0

	i := 0
	for ; i+8 <= n; i += 8 {
		r0 := unsafe.Add(rp, i*8)
		l0 := unsafe.Add(lp, i*8)
		u0 := unsafe.Add(up, i*8)

		vr0 := archsimd.LoadFloat64x4Array((*[4]float64)(r0))
		p10 := vr0.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(l0)))
		p20 := vr0.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(u0)))
		ord0 = ord0.And(orderedMask64x4(p10)).And(orderedMask64x4(p20))
		vminAcc0 = vminAcc0.Add(p10.Min(p20))
		vmaxAcc0 = vmaxAcc0.Add(p10.Max(p20))

		vr1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(r0, 32)))
		p11 := vr1.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(l0, 32))))
		p21 := vr1.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(u0, 32))))
		ord1 = ord1.And(orderedMask64x4(p11)).And(orderedMask64x4(p21))
		vminAcc1 = vminAcc1.Add(p11.Min(p21))
		vmaxAcc1 = vmaxAcc1.Add(p11.Max(p21))
	}
	for ; i+4 <= n; i += 4 {
		vr := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rp, i*8)))
		p1 := vr.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(lp, i*8))))
		p2 := vr.Mul(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(up, i*8))))
		ord0 = ord0.And(orderedMask64x4(p1)).And(orderedMask64x4(p2))
		vminAcc0 = vminAcc0.Add(p1.Min(p2))
		vmaxAcc0 = vmaxAcc0.Add(p1.Max(p2))
	}

	vminAcc0 = vminAcc0.Add(vminAcc1)
	vmaxAcc0 = vmaxAcc0.Add(vmaxAcc1)
	var minLanes, maxLanes [4]float64
	vminAcc0.StoreArray(&minLanes)
	vmaxAcc0.StoreArray(&maxLanes)
	if ord0.And(ord1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return math.NaN(), math.NaN()
	}
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

// MinReducedSlice64With returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)),
// dispatching to AVX2 when n >= 32 and SIMD is enabled.
func MinReducedSlice64With(rowCost, v []float64, disableSIMD bool) float64 {
	n := min(len(rowCost), len(v))
	if disableSIMD || simdOff() || n < minAVX2Len {
		return MinReducedSlice64Scalar(rowCost, v)
	}
	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vmin0 := archsimd.BroadcastFloat64x4(math.Inf(1))
	vmin1 := vmin0
	ord0 := orderedMask64x4(vmin0)
	ord1 := ord0
	j := 0
	for ; j+8 <= n; j += 8 {
		c0 := unsafe.Add(cp, j*8)
		v0 := unsafe.Add(vp, j*8)
		r0 := archsimd.LoadFloat64x4Array((*[4]float64)(c0)).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(v0)))
		r1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(c0, 32))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(v0, 32))))
		ord0 = ord0.And(orderedMask64x4(r0))
		ord1 = ord1.And(orderedMask64x4(r1))
		vmin0 = r0.Min(vmin0)
		vmin1 = r1.Min(vmin1)
	}
	for ; j+4 <= n; j += 4 {
		c0 := (*[4]float64)(unsafe.Add(cp, j*8))
		v0 := (*[4]float64)(unsafe.Add(vp, j*8))
		r0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0))
		ord0 = ord0.And(orderedMask64x4(r0))
		vmin0 = r0.Min(vmin0)
	}
	vmin0 = vmin1.Min(vmin0)
	var lanes [4]float64
	vmin0.StoreArray(&lanes)
	if ord0.And(ord1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	minRed := min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))
	for ; j < n; j++ {
		r := float64(*(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8)))
		minRed = min(minRed, r)
	}
	if minRed == 0 {
		minRed = 0
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

// InitReducedRow64With computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j],
// honoring a per-call disableSIMD flag in addition to the process-wide flag.
func InitReducedRow64With(dst, costRow, v []float64, ui float64, disableSIMD bool) float64 {
	if disableSIMD || simdOff() {
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
	vmin0 := archsimd.BroadcastFloat64x4(math.Inf(1))
	vmin1 := vmin0
	ord0 := orderedMask64x4(vmin0)
	ord1 := ord0

	i := 0
	for ; i+8 <= n; i += 8 {
		c0 := unsafe.Add(cp, i*8)
		v0 := unsafe.Add(vp, i*8)
		d0 := unsafe.Add(dp, i*8)

		vd0 := archsimd.LoadFloat64x4Array((*[4]float64)(c0)).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(v0))).Sub(vui)
		vd1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(c0, 32))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(v0, 32)))).Sub(vui)

		vd0.StoreArray((*[4]float64)(d0))
		vd1.StoreArray((*[4]float64)(unsafe.Add(d0, 32)))

		ord0 = ord0.And(orderedMask64x4(vd0))
		ord1 = ord1.And(orderedMask64x4(vd1))
		vmin0 = vd0.Min(vmin0)
		vmin1 = vd1.Min(vmin1)
	}
	for ; i+4 <= n; i += 4 {
		c0 := (*[4]float64)(unsafe.Add(cp, i*8))
		v0 := (*[4]float64)(unsafe.Add(vp, i*8))
		d0 := (*[4]float64)(unsafe.Add(dp, i*8))

		vd0 := archsimd.LoadFloat64x4Array(c0).Sub(archsimd.LoadFloat64x4Array(v0)).Sub(vui)
		vd0.StoreArray(d0)
		ord0 = ord0.And(orderedMask64x4(vd0))
		vmin0 = vd0.Min(vmin0)
	}

	vmin0 = vmin1.Min(vmin0)
	var lanes [4]float64
	vmin0.StoreArray(&lanes)
	var minVal float64
	if ord0.And(ord1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		minVal = math.NaN()
	} else {
		archsimd.ClearAVXUpperBits()
		minVal = min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))
	}

	for ; i < n; i++ {
		d := (*(*float64)(unsafe.Add(cp, i*8)) - *(*float64)(unsafe.Add(vp, i*8))) - ui
		*(*float64)(unsafe.Add(dp, i*8)) = d
		minVal = min(minVal, d)
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

// Min2ReducedRow64With computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j], honoring a per-call disableSIMD flag.
func Min2ReducedRow64With(costRow, v []float64, disableSIMD bool) (u1, u2 float64, j1, j2 int) {
	if disableSIMD || simdOff() {
		return Min2ReducedRow64Scalar(costRow, v)
	}
	n := min(len(costRow), len(v))
	if n >= 256 {
		return min2ReducedRow64AVX2(costRow[:n], v[:n])
	}
	return Min2ReducedRow64Scalar(costRow, v)
}

// Min2ReducedRow64 computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j].
func Min2ReducedRow64(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64With(costRow, v, false)
}

//go:noinline
func min2ScanAVX2(cp, vp unsafe.Pointer, j, n int, u2 float64, out *[8]float64) int {
	vu2 := archsimd.BroadcastFloat64x4(u2)
	for ; j+8 <= n; j += 8 {
		c0 := unsafe.Add(cp, j*8)
		v0 := unsafe.Add(vp, j*8)
		vr0 := archsimd.LoadFloat64x4Array((*[4]float64)(c0)).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(v0)))
		vr1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(c0, 32))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(v0, 32))))
		if vr0.GreaterEqual(vu2).And(vr1.GreaterEqual(vu2)).ToBits() != 0xF {
			vr0.StoreArray((*[4]float64)(unsafe.Pointer(&out[0])))
			vr1.StoreArray((*[4]float64)(unsafe.Pointer(&out[4])))
			archsimd.ClearAVXUpperBits()
			return j
		}
	}
	archsimd.ClearAVXUpperBits()
	return j
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

	var buf [8]float64
	j := 16
	for j+8 <= n {
		j = min2ScanAVX2(cp, vp, j, n, u2, &buf)
		if j+8 > n {
			break
		}
		for k := range buf {
			r := buf[k]
			if !(r >= u2) {
				idx := j + k
				if math.IsNaN(r) {
					return r, r, idx, -1
				}
				if r < u1 {
					u2, j2 = u1, j1
					u1, j1 = r, idx
				} else {
					u2, j2 = r, idx
				}
			}
		}
		j += 8
	}
	for ; j < n; j++ {
		r := *(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8))
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
