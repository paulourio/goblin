//go:build goexperiment.simd && amd64

// Package simd provides hardware-accelerated AVX2/FMA vector kernels via Go 1.27's
// simd/archsimd package, paired with a pure-Go scalar fallback when built without
// GOEXPERIMENT=simd.
package simd

import (
	"math"
	"simd/archsimd"
	"unsafe"
)

// UsingHardwareSIMD returns true when compiled with GOEXPERIMENT=simd on amd64.
func UsingHardwareSIMD() bool { return true }

// Dot64 computes the dot product sum_i (a[i] * b[i]).
// Slices of length < 256 use 4-way scalar accumulators without YMM transition overhead;
// slices of length >= 256 dispatch to 256-bit AVX2 vector accumulators with identical reduction order.
func Dot64(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	a = a[:n]
	b = b[:n]
	if n >= 256 {
		return dot64AVX2(a, b)
	}
	var s0, s1, s2, s3 float64
	i := 0
	for ; i+4 <= n; i += 4 {
		s0 += float64(a[i] * b[i])
		s1 += float64(a[i+1] * b[i+1])
		s2 += float64(a[i+2] * b[i+2])
		s3 += float64(a[i+3] * b[i+3])
	}
	sum := (s0 + s1) + (s2 + s3)
	for ; i < n; i++ {
		sum += float64(a[i] * b[i])
	}
	return sum
}

func dot64AVX2(a, b []float64) float64 {
	n := len(a)
	ap := unsafe.Pointer(&a[0])
	bp := unsafe.Pointer(&b[0])

	acc := archsimd.BroadcastFloat64x4(0)
	i := 0
	for ; i+4 <= n; i += 4 {
		va := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(ap, i*8)))
		vb := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(bp, i*8)))
		acc = acc.Add(va.Mul(vb))
	}

	lo := acc.GetLo()
	hi := acc.GetHi()
	sum := (lo.GetElem(0) + lo.GetElem(1)) + (hi.GetElem(0) + hi.GetElem(1))

	for ; i < n; i++ {
		sum += float64(a[i] * b[i])
	}
	return sum
}

// Axpy64 computes dst[i] += alpha * src[i].
// Slices of length < 256 use 4-way scalar unrolling without YMM transition overhead;
// slices of length >= 256 dispatch to 256-bit AVX2 vector instructions.
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
	if n >= 256 {
		axpy64AVX2(dst, src, alpha)
		return
	}
	i := 0
	for ; i+4 <= n; i += 4 {
		dst[i] += float64(alpha * src[i])
		dst[i+1] += float64(alpha * src[i+1])
		dst[i+2] += float64(alpha * src[i+2])
		dst[i+3] += float64(alpha * src[i+3])
	}
	for ; i < n; i++ {
		dst[i] += float64(alpha * src[i])
	}
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
	for ; i < n; i++ {
		dst[i] += float64(alpha * src[i])
	}
}

// Scale64 computes dst[i] *= alpha using AVX2 vector multiplication.
func Scale64(dst []float64, alpha float64) {
	n := len(dst)
	if n == 0 || alpha == 1 {
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
	for ; i < n; i++ {
		dst[i] *= alpha
	}
}

// AbsMax64 returns max_i |x[i]| using AVX2 Abs and Max instructions.
func AbsMax64(x []float64) float64 {
	n := len(x)
	if n == 0 {
		return 0
	}
	xp := unsafe.Pointer(&x[0])
	vmax := archsimd.BroadcastFloat64x4(0)
	i := 0
	for ; i+4 <= n; i += 4 {
		v := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, i*8))).Abs()
		vmax = vmax.Max(v)
	}
	pair := vmax.GetLo().Max(vmax.GetHi())
	res := max(pair.GetElem(0), pair.GetElem(1))
	for ; i < n; i++ {
		res = max(res, math.Abs(x[i]))
	}
	return res
}

// RowActivityBoundsFinite computes the minimum and maximum row activity
// minAct = sum_j min(row[j]*lb[j], row[j]*ub[j]) and
// maxAct = sum_j max(row[j]*lb[j], row[j]*ub[j])
// assuming all lb[j] and ub[j] are finite.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	if n == 0 {
		return 0, 0
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

	minPair := vminAcc.GetLo().Add(vminAcc.GetHi())
	maxPair := vmaxAcc.GetLo().Add(vmaxAcc.GetHi())
	minAct = minPair.GetElem(0) + minPair.GetElem(1)
	maxAct = maxPair.GetElem(0) + maxPair.GetElem(1)

	for ; i < n; i++ {
		p1 := row[i] * lb[i]
		p2 := row[i] * ub[i]
		minAct += min(p1, p2)
		maxAct += max(p1, p2)
	}
	return minAct, maxAct
}
