//go:build goexperiment.simd && amd64

// Package simd provides runtime-dispatched AVX-512 (512-bit) and AVX2/FMA (256-bit)
// vector kernels via Go 1.27's simd/archsimd package, paired with pure-Go 4-way
// unrolled scalar fallbacks when built without GOEXPERIMENT=simd, on CPUs lacking
// AVX2+FMA, or when SIMD is disabled or capped at runtime.
package simd

import (
	"fmt"
	"math"
	"math/bits"
	"os"
	"simd/archsimd"
	"sync/atomic"
	"unsafe"
)

const (
	minAVX2Len               = 32
	minAVX512Len             = 64
	minAVX512LPLen           = 2048
	minAVX2Min2Len           = 256
	minAVX512Min2Len         = 128
	minLeaveRowAVX2          = 32
	minLeaveRowAVX512        = 2048
	minDualRatioAVX512Chunks = 128 // 8,192 cols: keeps Emerald Rapids LP scans on AVX2 to avoid License-2 downclocking.
)

var (
	hasAVX2FMA = archsimd.X86.AVX2() && archsimd.X86.FMA()
	hasAVX512  = hasAVX2FMA && archsimd.X86.AVX512()
	hwTier     = detectHWTier()
	activeTier atomic.Uint32
)

func detectHWTier() uint32 {
	if hasAVX512 {
		return tierAVX512
	}
	if hasAVX2FMA {
		return tierAVX2
	}
	return tierScalar
}

func init() {
	maxTierCap.Store(tierAVX512)
	refreshActiveTier()
	if envTier := os.Getenv("GOBLIN_SIMD_MAX_TIER"); envTier != "" {
		_ = SetMaxTier(envTier)
	}
}

func refreshActiveTier() {
	if simdDisabled.Load() {
		activeTier.Store(tierScalar)
		return
	}
	t := min(hwTier, maxTierCap.Load())
	activeTier.Store(t)
}

func currentTier(disable bool) uint32 {
	if disable {
		return tierScalar
	}
	return activeTier.Load()
}

// HardwareSIMDAvailable reports whether the binary was compiled with GOEXPERIMENT=simd on amd64
// and the host CPU advertises at least AVX2 and FMA support.
func HardwareSIMDAvailable() bool { return hwTier > tierScalar }

// HardwareSIMDTier returns the highest SIMD tier supported by this binary and host CPU
// ("avx512", "avx2", or "scalar").
func HardwareSIMDTier() string { return tierName(hwTier) }

// ActiveSIMDTier returns the currently active SIMD dispatch tier after applying
// process-wide disable and max-tier settings ("avx512", "avx2", or "scalar").
func ActiveSIMDTier() string { return tierName(activeTier.Load()) }

// ActiveSIMDTierWith returns the active SIMD dispatch tier given a per-call disable flag.
func ActiveSIMDTierWith(disable bool) string { return tierName(currentTier(disable)) }

// SetMaxTier caps the active SIMD dispatch tier to "auto" (or "", uncapped),
// "avx512", "avx2", or "scalar". It never raises dispatch above HardwareSIMDTier.
func SetMaxTier(tier string) error {
	switch tier {
	case "", "auto", TierAVX512:
		maxTierCap.Store(tierAVX512)
	case TierAVX2:
		maxTierCap.Store(tierAVX2)
	case TierScalar:
		maxTierCap.Store(tierScalar)
	default:
		return fmt.Errorf("simd: unknown tier %q (expected auto, avx512, avx2, or scalar)", tier)
	}
	refreshActiveTier()
	return nil
}

// SetDisabled toggles the process-wide SIMD disable flag.
func SetDisabled(disabled bool) {
	simdDisabled.Store(disabled)
	refreshActiveTier()
}

// UsingHardwareSIMD reports whether hardware SIMD kernels (AVX-512 or AVX2) are active.
func UsingHardwareSIMD() bool { return activeTier.Load() > tierScalar }

// UsingHardwareSIMDWith reports whether hardware SIMD is active given a per-solve disable flag.
func UsingHardwareSIMDWith(disable bool) bool { return currentTier(disable) > tierScalar }

// orderedMask64x4 returns a 4-lane mask whose lanes are set where v is ordered (not NaN).
func orderedMask64x4(v archsimd.Float64x4) archsimd.Mask64x4 {
	//nolint:gocritic // IEEE-754 self-equality emits a single VCMPEQPD to detect NaN lanes.
	return v.Equal(v)
}

// Dot64 computes the dot product sum_i (a[i] * b[i]).
// Slices of length < 32 use 4-way scalar accumulators; slices of length >= 32 dispatch
// to AVX-512 (Float64x8) or AVX2/FMA (Float64x4) accumulators with VZEROUPPER cleanup.
func Dot64(a, b []float64) float64 {
	tier := activeTier.Load()
	if tier == tierScalar {
		return Dot64Scalar(a, b)
	}
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	a = a[:n]
	b = b[:n]
	if tier == tierAVX512 && n >= minAVX512LPLen {
		return dot64AVX512(a, b)
	}
	if n >= minAVX2Len {
		return dot64AVX2(a, b)
	}
	return Dot64Scalar(a, b)
}

//go:noinline
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

// Axpy64 computes dst[i] += alpha * src[i] using sign-symmetric separate Mul+Add.
func Axpy64(dst, src []float64, alpha float64) {
	tier := activeTier.Load()
	if tier == tierScalar {
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
	if tier == tierAVX512 && n >= minAVX512LPLen {
		vecEnd := n &^ 7
		axpy64AVX512(dst[:vecEnd], src[:vecEnd], alpha)
		for i := vecEnd; i < n; i++ {
			dst[i] += float64(alpha * src[i])
		}
		return
	}
	if n >= minAVX2Len {
		vecEnd := n &^ 3
		axpy64AVX2(dst[:vecEnd], src[:vecEnd], alpha)
		for i := vecEnd; i < n; i++ {
			dst[i] += float64(alpha * src[i])
		}
		return
	}
	Axpy64Scalar(dst, src, alpha)
}

//go:noinline
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
}

// Scale64 computes dst[i] *= alpha using AVX-512 or AVX2 vector multiplication.
func Scale64(dst []float64, alpha float64) {
	tier := activeTier.Load()
	n := len(dst)
	if n == 0 || alpha == 1 {
		return
	}
	if tier == tierAVX512 && n >= minAVX512LPLen {
		vecEnd := n &^ 7
		scale64AVX512(dst[:vecEnd], alpha)
		for i := vecEnd; i < n; i++ {
			dst[i] *= alpha
		}
		return
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		vecEnd := n &^ 3
		scale64AVX2(dst[:vecEnd], alpha)
		for i := vecEnd; i < n; i++ {
			dst[i] *= alpha
		}
		return
	}
	Scale64Scalar(dst, alpha)
}

//go:noinline
func scale64AVX2(dst []float64, alpha float64) {
	n := len(dst)
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
}

// AbsMax64 returns max_i |x[i]| using AVX-512 or AVX2 sign-bit masking and dual Max accumulators.
func AbsMax64(x []float64) float64 {
	tier := activeTier.Load()
	n := len(x)
	if tier == tierAVX512 && n >= minAVX512LPLen {
		return absMax64AVX512(x)
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		return absMax64AVX2(x)
	}
	return AbsMax64Scalar(x)
}

//go:noinline
func absMax64AVX2(x []float64) float64 {
	n := len(x)
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

// AllAbsLE64With reports whether every element x[i] satisfies |x[i]| <= limit.
func AllAbsLE64With(x []float64, limit float64, disableSIMD bool) bool {
	tier := currentTier(disableSIMD)
	n := len(x)
	if tier == tierScalar || n < minAVX2Len {
		return AllAbsLE64Scalar(x, limit)
	}
	if !(0 <= limit) {
		return false
	}
	var vecEnd int
	var ok bool
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		ok = allAbsLE64AVX512(x[:vecEnd], limit)
	} else {
		vecEnd = n &^ 3
		ok = allAbsLE64AVX2(x[:vecEnd], limit)
	}
	if !ok {
		return false
	}
	for i := vecEnd; i < n; i++ {
		if !(math.Abs(x[i]) <= limit) {
			return false
		}
	}
	return true
}

//go:noinline
func allAbsLE64AVX2(x []float64, limit float64) bool {
	n := len(x)
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
	return true
}

// RowActivityBoundsFinite computes the minimum and maximum row activity
// minAct = sum_j min(row[j]*lb[j], row[j]*ub[j]) and
// maxAct = sum_j max(row[j]*lb[j], row[j]*ub[j])
// assuming all lb[j] and ub[j] are finite.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	tier := activeTier.Load()
	if tier == tierAVX512 && n >= minAVX512LPLen {
		return rowActivityBoundsFiniteAVX512(row[:n], lb[:n], ub[:n])
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		return rowActivityBoundsFiniteAVX2(row[:n], lb[:n], ub[:n])
	}
	return RowActivityBoundsFiniteScalar(row, lb, ub)
}

//go:noinline
func rowActivityBoundsFiniteAVX2(row, lb, ub []float64) (minAct, maxAct float64) {
	n := len(row)
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

// MinReducedSlice64With returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)),
// dispatching to AVX-512 or AVX2 when n >= 32 and SIMD is enabled.
func MinReducedSlice64With(rowCost, v []float64, disableSIMD bool) float64 {
	n := min(len(rowCost), len(v))
	tier := currentTier(disableSIMD)
	if tier == tierAVX512 && n >= minAVX512Len {
		return minReducedSlice64AVX512(rowCost[:n], v[:n])
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		return minReducedSlice64AVX2(rowCost[:n], v[:n])
	}
	return MinReducedSlice64Scalar(rowCost, v)
}

//go:noinline
func minReducedSlice64AVX2(rowCost, v []float64) float64 {
	n := len(rowCost)
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
	tier := currentTier(disableSIMD)
	if tier == tierScalar {
		return InitReducedRow64Scalar(dst, costRow, v, ui)
	}
	n := min(len(dst), min(len(costRow), len(v)))
	var vecEnd int
	var minVal float64
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		minVal = initReducedRow64AVX512(dst[:vecEnd], costRow[:vecEnd], v[:vecEnd], ui)
	} else if n >= minAVX2Len {
		vecEnd = n &^ 3
		minVal = initReducedRow64AVX2(dst[:vecEnd], costRow[:vecEnd], v[:vecEnd], ui)
	} else {
		return InitReducedRow64Scalar(dst, costRow, v, ui)
	}
	for i := vecEnd; i < n; i++ {
		d := (costRow[i] - v[i]) - ui
		dst[i] = d
		minVal = min(minVal, d)
	}
	if minVal == 0 {
		minVal = 0
	}
	return minVal
}

// InitReducedRow64 computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
func InitReducedRow64(dst, costRow, v []float64, ui float64) float64 {
	return InitReducedRow64With(dst, costRow, v, ui, false)
}

//go:noinline
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
	if ord0.And(ord1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	return min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))
}

// Min2ReducedRow64With computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j], honoring a per-call disableSIMD flag.
func Min2ReducedRow64With(costRow, v []float64, disableSIMD bool) (u1, u2 float64, j1, j2 int) {
	tier := currentTier(disableSIMD)
	if tier == tierScalar {
		return Min2ReducedRow64Scalar(costRow, v)
	}
	n := min(len(costRow), len(v))
	if tier == tierAVX512 && n >= minAVX512Min2Len {
		return min2ReducedRow64AVX512(costRow[:n], v[:n])
	}
	if n >= minAVX2Min2Len {
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
func min2ScanAVX2(cp, vp unsafe.Pointer, j, n int, u2 float64) (int, uint8) {
	vu2 := archsimd.BroadcastFloat64x4(u2)
	for ; j+8 <= n; j += 8 {
		c0 := unsafe.Add(cp, j*8)
		v0 := unsafe.Add(vp, j*8)
		vr0 := archsimd.LoadFloat64x4Array((*[4]float64)(c0)).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(v0)))
		vr1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(c0, 32))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(v0, 32))))
		ge0 := vr0.GreaterEqual(vu2)
		ge1 := vr1.GreaterEqual(vu2)
		if ge0.And(ge1).ToBits() != 0xF {
			bad := (^ge0.ToBits() & 0xF) | ((^ge1.ToBits() & 0xF) << 4)
			// Branch on bad before ClearAVXUpperBits so Go SSA cannot reorder YMM ToBits reads past VZEROUPPER.
			if bad != 0 {
				archsimd.ClearAVXUpperBits()
				return j, bad
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return j, 0
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

	j := 16
	for j+8 <= n {
		var bad uint8
		j, bad = min2ScanAVX2(cp, vp, j, n, u2)
		if bad == 0 {
			break
		}
		for bad != 0 {
			k := bits.TrailingZeros8(bad)
			bad &= bad - 1
			idx := j + k
			r := *(*float64)(unsafe.Add(cp, idx*8)) - *(*float64)(unsafe.Add(vp, idx*8))
			if !(r >= u2) {
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

// SelectDualLeaveRow64With selects the dual steepest-edge leaving row using
// AVX-512 or AVX2 feasibility filtering when enabled and m >= 32.
func SelectDualLeaveRow64With(xb, basicLB, basicUB, basicTol, dse []float64, disableSIMD bool) (leaveRow int, leaveDir, primalInfeas float64) {
	m := len(xb)
	tier := currentTier(disableSIMD)
	if tier == tierScalar || m == 0 ||
		len(basicLB) < m || len(basicUB) < m || len(basicTol) < m || len(dse) < m {
		return SelectDualLeaveRow64Scalar(xb, basicLB, basicUB, basicTol, dse)
	}
	if tier == tierAVX512 && m >= minLeaveRowAVX512 {
		return selectDualLeaveRow64Vec(xb, basicLB, basicUB, basicTol, dse, true)
	}
	if m >= minLeaveRowAVX2 {
		return selectDualLeaveRow64Vec(xb, basicLB, basicUB, basicTol, dse, false)
	}
	return SelectDualLeaveRow64Scalar(xb, basicLB, basicUB, basicTol, dse)
}

// evalDualLeaveRow stays within the Go inline budget (cost <= 80) to avoid closure captures in selectDualLeaveRow64Vec.
func evalDualLeaveRow(xb, basicLB, basicUB, basicTol []float64, r int) (viol, dir float64) {
	val, tol := xb[r], basicTol[r]
	if lbr := basicLB[r]; val < lbr-tol {
		return lbr - val, 1.0
	}
	if ubr := basicUB[r]; val > ubr+tol {
		return val - ubr, -1.0
	}
	return 0, 0
}

func selectDualLeaveRow64Vec(xb, basicLB, basicUB, basicTol, dse []float64, useAVX512 bool) (leaveRow int, leaveDir, primalInfeas float64) {
	m := len(xb)
	xb = xb[:m]
	basicLB = basicLB[:m]
	basicUB = basicUB[:m]
	basicTol = basicTol[:m]
	dse = dse[:m]
	xp := unsafe.Pointer(&xb[0])
	lp := unsafe.Pointer(&basicLB[0])
	up := unsafe.Pointer(&basicUB[0])
	tp := unsafe.Pointer(&basicTol[0])
	dp := unsafe.Pointer(&dse[0])

	leaveRow = -1
	bestScore := -1.0

	pos := 0
	if useAVX512 {
		for pos+8 <= m {
			r, bm := scanBetterInfeasAVX512(xp, lp, up, tp, dp, pos, m, bestScore)
			if bm == 0 {
				pos = r
				break
			}
			for bm != 0 {
				k := bits.TrailingZeros16(bm)
				bm &= bm - 1
				row := r + k
				if uint(row) >= uint(m) {
					continue
				}
				if viol, dir := evalDualLeaveRow(xb, basicLB, basicUB, basicTol, row); viol > 0 {
					w := dse[row]
					v2 := viol * viol
					if v2 > bestScore*w {
						if score := v2 / w; leaveRow == -1 || score > bestScore {
							leaveRow, leaveDir, bestScore, primalInfeas = row, dir, score, viol
						}
					}
				}
			}
			if r+16 <= m {
				pos = r + 16
			} else {
				pos = r + 8
			}
		}
	} else {
		for pos+4 <= m {
			r, bm := scanBetterInfeasAVX2(xp, lp, up, tp, dp, pos, m, bestScore)
			if bm == 0 {
				pos = r
				break
			}
			for bm != 0 {
				k := bits.TrailingZeros8(bm)
				bm &= bm - 1
				row := r + k
				if uint(row) >= uint(m) {
					continue
				}
				if viol, dir := evalDualLeaveRow(xb, basicLB, basicUB, basicTol, row); viol > 0 {
					w := dse[row]
					v2 := viol * viol
					if v2 > bestScore*w {
						if score := v2 / w; leaveRow == -1 || score > bestScore {
							leaveRow, leaveDir, bestScore, primalInfeas = row, dir, score, viol
						}
					}
				}
			}
			if r+8 <= m {
				pos = r + 8
			} else {
				pos = r + 4
			}
		}
	}
	for ; pos < m; pos++ {
		if viol, dir := evalDualLeaveRow(xb, basicLB, basicUB, basicTol, pos); viol > 0 {
			w := dse[pos]
			v2 := viol * viol
			if v2 > bestScore*w {
				if score := v2 / w; leaveRow == -1 || score > bestScore {
					leaveRow, leaveDir, bestScore, primalInfeas = pos, dir, score, viol
				}
			}
		}
	}
	return leaveRow, leaveDir, primalInfeas
}

//go:noinline
func scanBetterInfeasAVX2(xp, lp, up, tp, dp unsafe.Pointer, r, m int, bestScore float64) (int, uint8) {
	vBest := archsimd.BroadcastFloat64x4(bestScore)
	for ; r+8 <= m; r += 8 {
		x0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, r*8)))
		l0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(lp, r*8)))
		u0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(up, r*8)))
		t0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(tp, r*8)))
		lo0 := x0.Less(l0.Sub(t0))
		hi0 := x0.Greater(u0.Add(t0))
		m0 := lo0.Or(hi0)

		x1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, (r+4)*8)))
		l1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(lp, (r+4)*8)))
		u1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(up, (r+4)*8)))
		t1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(tp, (r+4)*8)))
		lo1 := x1.Less(l1.Sub(t1))
		hi1 := x1.Greater(u1.Add(t1))
		m1 := lo1.Or(hi1)

		if m0.Or(m1).ToBits() != 0 {
			var bm uint8
			if m0.ToBits() != 0 {
				v0 := l0.Sub(x0).IfElse(lo0, x0.Sub(u0))
				w0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, r*8)))
				bm = v0.Mul(v0).Greater(vBest.Mul(w0)).And(m0).ToBits() & 0xF
			}
			if m1.ToBits() != 0 {
				v1 := l1.Sub(x1).IfElse(lo1, x1.Sub(u1))
				w1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, (r+4)*8)))
				bm |= (v1.Mul(v1).Greater(vBest.Mul(w1)).And(m1).ToBits() & 0xF) << 4
			}
			if bm != 0 {
				archsimd.ClearAVXUpperBits()
				return r, bm
			}
		}
	}
	for ; r+4 <= m; r += 4 {
		x0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, r*8)))
		l0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(lp, r*8)))
		u0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(up, r*8)))
		t0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(tp, r*8)))
		lo0 := x0.Less(l0.Sub(t0))
		hi0 := x0.Greater(u0.Add(t0))
		m0 := lo0.Or(hi0)
		if m0.ToBits() != 0 {
			v0 := l0.Sub(x0).IfElse(lo0, x0.Sub(u0))
			w0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, r*8)))
			if bm := v0.Mul(v0).Greater(vBest.Mul(w0)).And(m0).ToBits() & 0xF; bm != 0 {
				archsimd.ClearAVXUpperBits()
				return r, bm
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return r, 0
}

// ScanDualRatioCand64With scans 64-column structural chunks starting at (chunkPos, elemPos)
// using AVX-512 (when active and len(chunks) >= minDualRatioAVX512Chunks), AVX2, or scalar fallback, appending
// qualifying column indices into dst (which must have len(dst) >= 8 for vector execution).
// It returns (hitCount, nextChunk, nextElem) so callers process dst[:hitCount] with exact
// scalar ratio logic and resume from (nextChunk, nextElem) with an updated thetaGuard.
func ScanDualRatioCand64With[S ~uint8](
	tab, dj []float64,
	status []S,
	chunks []int,
	chunkPos, elemPos int,
	negLeaveDir, pivotTol, thetaGuard float64,
	bland, disableSIMD bool,
	dst []int,
) (hitCount, nextChunk, nextElem int) {
	n := min(len(tab), min(len(dj), len(status)))
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n == 0 || len(dst) < 8 || chunkPos >= len(chunks) || math.Abs(negLeaveDir) != 1.0 {
		return ScanDualRatioCand64Scalar(tab, dj, status, chunks, chunkPos, elemPos, negLeaveDir, pivotTol, thetaGuard, bland, dst)
	}
	tabPtr := unsafe.Pointer(&tab[0])
	djPtr := unsafe.Pointer(&dj[0])
	stPtr := unsafe.Pointer(&status[0])
	filterByTheta := !bland && !math.IsInf(thetaGuard, 1)
	if tier == tierAVX512 && len(chunks) >= minDualRatioAVX512Chunks {
		return scanDualRatioCandAVX512(tabPtr, djPtr, stPtr, n, chunks, chunkPos, elemPos, negLeaveDir, pivotTol, thetaGuard, filterByTheta, dst)
	}
	return scanDualRatioCandAVX2(tabPtr, djPtr, stPtr, n, chunks, chunkPos, elemPos, negLeaveDir, pivotTol, thetaGuard, filterByTheta, dst)
}

//go:noinline
func scanDualRatioCandAVX2(
	tabPtr, djPtr, stPtr unsafe.Pointer,
	n int,
	chunks []int,
	chunkPos, elemPos int,
	negLeaveDir, pivotTol, thetaGuard float64,
	filterByTheta bool,
	dst []int,
) (hitCount, nextChunk, nextElem int) {
	capDst := len(dst)
	if chunkPos < 0 {
		chunkPos = 0
	}
	if elemPos < 0 {
		elemPos = 0
	}
	if capDst == 0 {
		return 0, chunkPos, elemPos
	}
	dstPtr := unsafe.Pointer(&dst[0])

	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	vZero := signMask.Xor(signMask).BitsToFloat64()
	vNegLeave := archsimd.BroadcastFloat64x4(negLeaveDir)
	vPivotTol := archsimd.BroadcastFloat64x4(pivotTol)
	vThetaGuard := archsimd.BroadcastFloat64x4(thetaGuard)

	cnt := 0

	for ci := chunkPos; ci < len(chunks); ci++ {
		ch := chunks[ci]
		start := ch << 6
		if start < 0 || start >= n {
			elemPos = 0
			continue
		}
		end := min(start+64, n)
		size := end - start
		i := elemPos
		elemPos = 0
		if i >= size {
			continue
		}
		tp := unsafe.Add(tabPtr, start*8)
		dp := unsafe.Add(djPtr, start*8)
		sp := unsafe.Add(stPtr, start)

		for ; i+8 <= size; i += 8 {
			u0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(tp, i*8)))
			u1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(tp, (i+4)*8)))
			t0 := u0.BitsToFloat64()
			t1 := u1.BitsToFloat64()
			nz0 := t0.NotEqual(vZero).ToBits() & 0xF
			nz1 := t1.NotEqual(vZero).ToBits() & 0xF
			if (nz0 | nz1) == 0 {
				continue
			}
			var bm uint8
			if nz0 != 0 {
				vAlpha := vNegLeave.Mul(t0)
				vAbs := u0.And(signMask).BitsToFloat64()
				big := vAbs.Greater(vPivotTol).ToBits() & 0xF
				gtTol := big & (vAlpha.Greater(vZero).ToBits() & 0xF)
				ltNegTol := big &^ gtTol
				subOrNaN := nz0 &^ big

				st4 := *(*uint32)(unsafe.Add(sp, i))
				lowB := ((st4 | (st4 >> 1) | (st4 >> 2)) ^ 0x01010101) & 0x01010101
				uppB := (st4 &^ (st4 >> 1)) & 0x01010101
				supB := (st4 >> 2) & 0x01010101
				blkLower := gtTol & uint8(((lowB*0x01020408)>>24)&0xF)
				blkUpper := ltNegTol & uint8(((uppB*0x01020408)>>24)&0xF)
				blkSuper := big & uint8(((supB*0x01020408)>>24)&0xF)
				blk := blkLower | blkUpper
				if blk != 0 && filterByTheta {
					vDJ := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, i*8)))
					vLim := vThetaGuard.Mul(vAbs)
					if blkLower != 0 {
						blkLower &^= vDJ.Greater(vLim).ToBits() & 0xF
					}
					if blkUpper != 0 {
						blkUpper &^= vDJ.Less(vZero.Sub(vLim)).ToBits() & 0xF
					}
					blk = blkLower | blkUpper
				}
				bm = blk | blkSuper | subOrNaN
			}
			if nz1 != 0 {
				vAlpha := vNegLeave.Mul(t1)
				vAbs := u1.And(signMask).BitsToFloat64()
				big := vAbs.Greater(vPivotTol).ToBits() & 0xF
				gtTol := big & (vAlpha.Greater(vZero).ToBits() & 0xF)
				ltNegTol := big &^ gtTol
				subOrNaN := nz1 &^ big

				st4 := *(*uint32)(unsafe.Add(sp, i+4))
				lowB := ((st4 | (st4 >> 1) | (st4 >> 2)) ^ 0x01010101) & 0x01010101
				uppB := (st4 &^ (st4 >> 1)) & 0x01010101
				supB := (st4 >> 2) & 0x01010101
				blkLower := gtTol & uint8(((lowB*0x01020408)>>24)&0xF)
				blkUpper := ltNegTol & uint8(((uppB*0x01020408)>>24)&0xF)
				blkSuper := big & uint8(((supB*0x01020408)>>24)&0xF)
				blk := blkLower | blkUpper
				if blk != 0 && filterByTheta {
					vDJ := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, (i+4)*8)))
					vLim := vThetaGuard.Mul(vAbs)
					if blkLower != 0 {
						blkLower &^= vDJ.Greater(vLim).ToBits() & 0xF
					}
					if blkUpper != 0 {
						blkUpper &^= vDJ.Less(vZero.Sub(vLim)).ToBits() & 0xF
					}
					blk = blkLower | blkUpper
				}
				bm |= (blk | blkSuper | subOrNaN) << 4
			}
			if bm != 0 {
				if cnt+8 > capDst {
					archsimd.ClearAVXUpperBits()
					return cnt, ci, i
				}
				base := start + i
				for bm != 0 {
					off := bits.TrailingZeros8(bm)
					bm &= bm - 1
					*(*int)(unsafe.Add(dstPtr, cnt*8)) = base + off
					cnt++
				}
				if cnt+8 > capDst {
					archsimd.ClearAVXUpperBits()
					if i+8 >= size {
						return cnt, ci + 1, 0
					}
					return cnt, ci, i + 8
				}
			}
		}
		for ; i+4 <= size; i += 4 {
			u0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(tp, i*8)))
			t0 := u0.BitsToFloat64()
			nz0 := t0.NotEqual(vZero).ToBits() & 0xF
			if nz0 == 0 {
				continue
			}
			vAlpha := vNegLeave.Mul(t0)
			vAbs := u0.And(signMask).BitsToFloat64()
			big := vAbs.Greater(vPivotTol).ToBits() & 0xF
			gtTol := big & (vAlpha.Greater(vZero).ToBits() & 0xF)
			ltNegTol := big &^ gtTol
			subOrNaN := nz0 &^ big

			st4 := *(*uint32)(unsafe.Add(sp, i))
			lowB := ((st4 | (st4 >> 1) | (st4 >> 2)) ^ 0x01010101) & 0x01010101
			uppB := (st4 &^ (st4 >> 1)) & 0x01010101
			supB := (st4 >> 2) & 0x01010101
			blkLower := gtTol & uint8(((lowB*0x01020408)>>24)&0xF)
			blkUpper := ltNegTol & uint8(((uppB*0x01020408)>>24)&0xF)
			blkSuper := big & uint8(((supB*0x01020408)>>24)&0xF)
			blk := blkLower | blkUpper
			if blk != 0 && filterByTheta {
				vDJ := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(dp, i*8)))
				vLim := vThetaGuard.Mul(vAbs)
				if blkLower != 0 {
					blkLower &^= vDJ.Greater(vLim).ToBits() & 0xF
				}
				if blkUpper != 0 {
					blkUpper &^= vDJ.Less(vZero.Sub(vLim)).ToBits() & 0xF
				}
				blk = blkLower | blkUpper
			}
			if bm := blk | blkSuper | subOrNaN; bm != 0 {
				if cnt+4 > capDst {
					archsimd.ClearAVXUpperBits()
					return cnt, ci, i
				}
				base := start + i
				for bm != 0 {
					off := bits.TrailingZeros8(bm)
					bm &= bm - 1
					*(*int)(unsafe.Add(dstPtr, cnt*8)) = base + off
					cnt++
				}
				if cnt+8 > capDst {
					archsimd.ClearAVXUpperBits()
					if i+4 >= size {
						return cnt, ci + 1, 0
					}
					return cnt, ci, i + 4
				}
			}
		}
		if i < size {
			rem := size - i
			if cnt+rem > capDst {
				archsimd.ClearAVXUpperBits()
				return cnt, ci, i
			}
			for ; i < size; i++ {
				*(*int)(unsafe.Add(dstPtr, cnt*8)) = start + i
				cnt++
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return cnt, len(chunks), 0
}
