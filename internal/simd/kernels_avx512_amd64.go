//go:build goexperiment.simd && amd64

package simd

import (
	"math"
	"math/bits"
	"simd/archsimd"
	"unsafe"
)

// orderedMask64x8 returns an 8-lane mask whose lanes are set where v is ordered (not NaN).
func orderedMask64x8(v archsimd.Float64x8) archsimd.Mask64x8 {
	//nolint:gocritic // IEEE-754 self-equality emits a single EVEX VCMPPD to detect NaN lanes.
	return v.Equal(v)
}

//go:noinline
func dot64AVX512(a, b []float64) float64 {
	n := len(a)
	ap := unsafe.Pointer(&a[0])
	bp := unsafe.Pointer(&b[0])

	zMask := archsimd.BroadcastUint64x8(0x7fffffffffffffff)
	acc0 := zMask.Xor(zMask).BitsToFloat64()
	acc1 := acc0
	acc2 := acc0
	acc3 := acc0
	i := 0
	for ; i+32 <= n; i += 32 {
		p0 := unsafe.Add(ap, i*8)
		q0 := unsafe.Add(bp, i*8)
		acc0 = archsimd.LoadFloat64x8Array((*[8]float64)(p0)).MulAdd(archsimd.LoadFloat64x8Array((*[8]float64)(q0)), acc0)
		acc1 = archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(p0, 64))).MulAdd(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(q0, 64))), acc1)
		acc2 = archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(p0, 128))).MulAdd(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(q0, 128))), acc2)
		acc3 = archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(p0, 192))).MulAdd(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(q0, 192))), acc3)
	}
	for ; i+8 <= n; i += 8 {
		va := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(ap, i*8)))
		vb := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(bp, i*8)))
		acc0 = va.MulAdd(vb, acc0)
	}
	acc0 = acc0.Add(acc1).Add(acc2.Add(acc3))

	var lanes [8]float64
	acc0.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()
	sum := ((lanes[0] + lanes[1]) + (lanes[2] + lanes[3])) +
		((lanes[4] + lanes[5]) + (lanes[6] + lanes[7]))

	for ; i < n; i++ {
		sum += *(*float64)(unsafe.Add(ap, i*8)) * *(*float64)(unsafe.Add(bp, i*8))
	}
	return sum
}

//go:noinline
func axpy64AVX512(dst, src []float64, alpha float64) {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	sp := unsafe.Pointer(&src[0])

	valpha := archsimd.BroadcastFloat64x8(alpha)
	i := 0
	for ; i+16 <= n; i += 16 {
		d0 := (*[8]float64)(unsafe.Add(dp, i*8))
		s0 := (*[8]float64)(unsafe.Add(sp, i*8))
		d1 := (*[8]float64)(unsafe.Add(unsafe.Pointer(d0), 64))
		s1 := (*[8]float64)(unsafe.Add(unsafe.Pointer(s0), 64))

		archsimd.LoadFloat64x8Array(d0).Add(archsimd.LoadFloat64x8Array(s0).Mul(valpha)).StoreArray(d0)
		archsimd.LoadFloat64x8Array(d1).Add(archsimd.LoadFloat64x8Array(s1).Mul(valpha)).StoreArray(d1)
	}
	for ; i+8 <= n; i += 8 {
		d0 := (*[8]float64)(unsafe.Add(dp, i*8))
		s0 := (*[8]float64)(unsafe.Add(sp, i*8))
		archsimd.LoadFloat64x8Array(d0).Add(archsimd.LoadFloat64x8Array(s0).Mul(valpha)).StoreArray(d0)
	}
	archsimd.ClearAVXUpperBits()
}

//go:noinline
func scale64AVX512(dst []float64, alpha float64) {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	valpha := archsimd.BroadcastFloat64x8(alpha)
	i := 0
	for ; i+16 <= n; i += 16 {
		d0 := (*[8]float64)(unsafe.Add(dp, i*8))
		archsimd.LoadFloat64x8Array(d0).Mul(valpha).StoreArray(d0)

		d1 := (*[8]float64)(unsafe.Add(unsafe.Pointer(d0), 64))
		archsimd.LoadFloat64x8Array(d1).Mul(valpha).StoreArray(d1)
	}
	for ; i+8 <= n; i += 8 {
		d0 := (*[8]float64)(unsafe.Add(dp, i*8))
		archsimd.LoadFloat64x8Array(d0).Mul(valpha).StoreArray(d0)
	}
	archsimd.ClearAVXUpperBits()
}

//go:noinline
func absMax64AVX512(x []float64) float64 {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	signMask := archsimd.BroadcastUint64x8(0x7fffffffffffffff)
	vmax0 := signMask.Xor(signMask).BitsToFloat64()
	vmax1 := vmax0
	ord0 := orderedMask64x8(vmax0)
	ord1 := ord0
	i := 0
	for ; i+16 <= n; i += 16 {
		p0 := unsafe.Add(xp, i*8)
		v0 := archsimd.LoadUint64x8Array((*[8]uint64)(p0)).And(signMask).BitsToFloat64()
		v1 := archsimd.LoadUint64x8Array((*[8]uint64)(unsafe.Add(p0, 64))).And(signMask).BitsToFloat64()
		vmax0 = vmax0.Max(v0)
		vmax1 = vmax1.Max(v1)
		ord0 = ord0.And(orderedMask64x8(v0))
		ord1 = ord1.And(orderedMask64x8(v1))
	}
	for ; i+8 <= n; i += 8 {
		v0 := archsimd.LoadUint64x8Array((*[8]uint64)(unsafe.Add(xp, i*8))).And(signMask).BitsToFloat64()
		vmax0 = vmax0.Max(v0)
		ord0 = ord0.And(orderedMask64x8(v0))
	}
	vmax0 = vmax0.Max(vmax1)
	var lanes [8]float64
	vmax0.StoreArray(&lanes)
	if ord0.And(ord1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	res := max(
		max(max(lanes[0], lanes[1]), max(lanes[2], lanes[3])),
		max(max(lanes[4], lanes[5]), max(lanes[6], lanes[7])),
	)
	for ; i < n; i++ {
		res = max(res, math.Abs(*(*float64)(unsafe.Add(xp, i*8))))
	}
	return res
}

//go:noinline
func allAbsLE64AVX512(x []float64, limit float64) bool {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	vlim := archsimd.BroadcastFloat64x8(limit)
	signMask := archsimd.BroadcastUint64x8(0x7fffffffffffffff)
	allOK0 := orderedMask64x8(vlim)
	allOK1 := allOK0
	i := 0
	for ; i+16 <= n; i += 16 {
		p0 := unsafe.Add(xp, i*8)
		v0 := archsimd.LoadUint64x8Array((*[8]uint64)(p0)).And(signMask).BitsToFloat64()
		v1 := archsimd.LoadUint64x8Array((*[8]uint64)(unsafe.Add(p0, 64))).And(signMask).BitsToFloat64()
		allOK0 = allOK0.And(v0.LessEqual(vlim))
		allOK1 = allOK1.And(v1.LessEqual(vlim))
	}
	for ; i+8 <= n; i += 8 {
		v0 := archsimd.LoadUint64x8Array((*[8]uint64)(unsafe.Add(xp, i*8))).And(signMask).BitsToFloat64()
		allOK0 = allOK0.And(v0.LessEqual(vlim))
	}
	if allOK0.And(allOK1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return false
	}
	archsimd.ClearAVXUpperBits()
	return true
}

//go:noinline
func rowActivityBoundsFiniteAVX512(row, lb, ub []float64) (minAct, maxAct float64) {
	n := len(row)
	rp := unsafe.Pointer(&row[0])
	lp := unsafe.Pointer(&lb[0])
	up := unsafe.Pointer(&ub[0])

	zMask := archsimd.BroadcastUint64x8(0x7fffffffffffffff)
	vminAcc0 := zMask.Xor(zMask).BitsToFloat64()
	vminAcc1 := vminAcc0
	vmaxAcc0 := vminAcc0
	vmaxAcc1 := vminAcc0
	ord0 := orderedMask64x8(vminAcc0)
	ord1 := ord0

	i := 0
	for ; i+16 <= n; i += 16 {
		r0 := unsafe.Add(rp, i*8)
		l0 := unsafe.Add(lp, i*8)
		u0 := unsafe.Add(up, i*8)

		vr0 := archsimd.LoadFloat64x8Array((*[8]float64)(r0))
		p10 := vr0.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(l0)))
		p20 := vr0.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(u0)))
		ord0 = ord0.And(orderedMask64x8(p10)).And(orderedMask64x8(p20))
		vminAcc0 = vminAcc0.Add(p10.Min(p20))
		vmaxAcc0 = vmaxAcc0.Add(p10.Max(p20))

		vr1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(r0, 64)))
		p11 := vr1.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(l0, 64))))
		p21 := vr1.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(u0, 64))))
		ord1 = ord1.And(orderedMask64x8(p11)).And(orderedMask64x8(p21))
		vminAcc1 = vminAcc1.Add(p11.Min(p21))
		vmaxAcc1 = vmaxAcc1.Add(p11.Max(p21))
	}
	for ; i+8 <= n; i += 8 {
		vr := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rp, i*8)))
		p1 := vr.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(lp, i*8))))
		p2 := vr.Mul(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(up, i*8))))
		ord0 = ord0.And(orderedMask64x8(p1)).And(orderedMask64x8(p2))
		vminAcc0 = vminAcc0.Add(p1.Min(p2))
		vmaxAcc0 = vmaxAcc0.Add(p1.Max(p2))
	}

	vminAcc0 = vminAcc0.Add(vminAcc1)
	vmaxAcc0 = vmaxAcc0.Add(vmaxAcc1)
	var minLanes, maxLanes [8]float64
	vminAcc0.StoreArray(&minLanes)
	vmaxAcc0.StoreArray(&maxLanes)
	if ord0.And(ord1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return math.NaN(), math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	minAct = ((minLanes[0] + minLanes[1]) + (minLanes[2] + minLanes[3])) +
		((minLanes[4] + minLanes[5]) + (minLanes[6] + minLanes[7]))
	maxAct = ((maxLanes[0] + maxLanes[1]) + (maxLanes[2] + maxLanes[3])) +
		((maxLanes[4] + maxLanes[5]) + (maxLanes[6] + maxLanes[7]))

	for ; i < n; i++ {
		rv := *(*float64)(unsafe.Add(rp, i*8))
		p1 := rv * *(*float64)(unsafe.Add(lp, i*8))
		p2 := rv * *(*float64)(unsafe.Add(up, i*8))
		minAct += min(p1, p2)
		maxAct += max(p1, p2)
	}
	return minAct, maxAct
}

//go:noinline
func minReducedSlice64AVX512(rowCost, v []float64) float64 {
	n := len(rowCost)
	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vmin0 := archsimd.BroadcastFloat64x8(math.Inf(1))
	vmin1 := vmin0
	ord0 := orderedMask64x8(vmin0)
	ord1 := ord0
	j := 0
	for ; j+16 <= n; j += 16 {
		c0 := unsafe.Add(cp, j*8)
		v0 := unsafe.Add(vp, j*8)
		r0 := archsimd.LoadFloat64x8Array((*[8]float64)(c0)).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(v0)))
		r1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(c0, 64))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(v0, 64))))
		ord0 = ord0.And(orderedMask64x8(r0))
		ord1 = ord1.And(orderedMask64x8(r1))
		vmin0 = r0.Min(vmin0)
		vmin1 = r1.Min(vmin1)
	}
	for ; j+8 <= n; j += 8 {
		c0 := (*[8]float64)(unsafe.Add(cp, j*8))
		v0 := (*[8]float64)(unsafe.Add(vp, j*8))
		r0 := archsimd.LoadFloat64x8Array(c0).Sub(archsimd.LoadFloat64x8Array(v0))
		ord0 = ord0.And(orderedMask64x8(r0))
		vmin0 = r0.Min(vmin0)
	}
	vmin0 = vmin1.Min(vmin0)
	var lanes [8]float64
	vmin0.StoreArray(&lanes)
	if ord0.And(ord1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	minRed := min(
		min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3])),
		min(min(lanes[4], lanes[5]), min(lanes[6], lanes[7])),
	)
	for ; j < n; j++ {
		r := float64(*(*float64)(unsafe.Add(cp, j*8)) - *(*float64)(unsafe.Add(vp, j*8)))
		minRed = min(minRed, r)
	}
	if minRed == 0 {
		minRed = 0
	}
	return minRed
}

//go:noinline
func initReducedRow64AVX512(dst, costRow, v []float64, ui float64) float64 {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	cp := unsafe.Pointer(&costRow[0])
	vp := unsafe.Pointer(&v[0])

	vui := archsimd.BroadcastFloat64x8(ui)
	vmin0 := archsimd.BroadcastFloat64x8(math.Inf(1))
	vmin1 := vmin0
	ord0 := orderedMask64x8(vmin0)
	ord1 := ord0

	i := 0
	for ; i+16 <= n; i += 16 {
		c0 := unsafe.Add(cp, i*8)
		v0 := unsafe.Add(vp, i*8)
		d0 := unsafe.Add(dp, i*8)

		vd0 := archsimd.LoadFloat64x8Array((*[8]float64)(c0)).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(v0))).Sub(vui)
		vd1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(c0, 64))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(v0, 64)))).Sub(vui)

		vd0.StoreArray((*[8]float64)(d0))
		vd1.StoreArray((*[8]float64)(unsafe.Add(d0, 64)))

		ord0 = ord0.And(orderedMask64x8(vd0))
		ord1 = ord1.And(orderedMask64x8(vd1))
		vmin0 = vd0.Min(vmin0)
		vmin1 = vd1.Min(vmin1)
	}
	for ; i+8 <= n; i += 8 {
		c0 := (*[8]float64)(unsafe.Add(cp, i*8))
		v0 := (*[8]float64)(unsafe.Add(vp, i*8))
		d0 := (*[8]float64)(unsafe.Add(dp, i*8))

		vd0 := archsimd.LoadFloat64x8Array(c0).Sub(archsimd.LoadFloat64x8Array(v0)).Sub(vui)
		vd0.StoreArray(d0)
		ord0 = ord0.And(orderedMask64x8(vd0))
		vmin0 = vd0.Min(vmin0)
	}

	vmin0 = vmin1.Min(vmin0)
	var lanes [8]float64
	vmin0.StoreArray(&lanes)
	if ord0.And(ord1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()
	return min(
		min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3])),
		min(min(lanes[4], lanes[5]), min(lanes[6], lanes[7])),
	)
}

//go:noinline
func min2ScanAVX512(cp, vp unsafe.Pointer, j, n int, u2 float64) (int, uint16) {
	vu2 := archsimd.BroadcastFloat64x8(u2)
	for ; j+16 <= n; j += 16 {
		c0 := unsafe.Add(cp, j*8)
		v0 := unsafe.Add(vp, j*8)
		vr0 := archsimd.LoadFloat64x8Array((*[8]float64)(c0)).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(v0)))
		vr1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(c0, 64))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(v0, 64))))
		ge0 := vr0.GreaterEqual(vu2)
		ge1 := vr1.GreaterEqual(vu2)
		if ge0.And(ge1).ToBits() != 0xFF {
			bad := uint16(^ge0.ToBits()) | (uint16(^ge1.ToBits()) << 8)
			archsimd.ClearAVXUpperBits()
			return j, bad
		}
	}
	archsimd.ClearAVXUpperBits()
	return j, 0
}

func min2ReducedRow64AVX512(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	n := len(costRow)
	cp := unsafe.Pointer(&costRow[0])
	vp := unsafe.Pointer(&v[0])

	// Seed the first 16 elements directly via pointer arithmetic so u2 is tight before AVX-512 streaming.
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
	for j+16 <= n {
		var bad uint16
		j, bad = min2ScanAVX512(cp, vp, j, n, u2)
		if bad == 0 {
			break
		}
		for bad != 0 {
			k := bits.TrailingZeros16(bad)
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
		j += 16
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

//go:noinline
func scanBetterInfeasAVX512(xp, lp, up, tp, dp unsafe.Pointer, r, m int, bestScore float64) (int, uint16) {
	vBest := archsimd.BroadcastFloat64x8(bestScore)
	for ; r+16 <= m; r += 16 {
		x0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, r*8)))
		l0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(lp, r*8)))
		u0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(up, r*8)))
		t0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(tp, r*8)))
		lo0 := x0.Less(l0.Sub(t0))
		hi0 := x0.Greater(u0.Add(t0))
		m0 := lo0.Or(hi0)

		x1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, (r+8)*8)))
		l1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(lp, (r+8)*8)))
		u1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(up, (r+8)*8)))
		t1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(tp, (r+8)*8)))
		lo1 := x1.Less(l1.Sub(t1))
		hi1 := x1.Greater(u1.Add(t1))
		m1 := lo1.Or(hi1)

		if m0.Or(m1).ToBits() != 0 {
			var bm uint16
			if m0.ToBits() != 0 {
				v0 := l0.Sub(x0).IfElse(lo0, x0.Sub(u0))
				w0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(dp, r*8)))
				bm = uint16(v0.Mul(v0).Greater(vBest.Mul(w0)).And(m0).ToBits())
			}
			if m1.ToBits() != 0 {
				v1 := l1.Sub(x1).IfElse(lo1, x1.Sub(u1))
				w1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(dp, (r+8)*8)))
				bm |= uint16(v1.Mul(v1).Greater(vBest.Mul(w1)).And(m1).ToBits()) << 8
			}
			if bm != 0 {
				archsimd.ClearAVXUpperBits()
				return r, bm
			}
		}
	}
	for ; r+8 <= m; r += 8 {
		x0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, r*8)))
		l0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(lp, r*8)))
		u0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(up, r*8)))
		t0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(tp, r*8)))
		lo0 := x0.Less(l0.Sub(t0))
		hi0 := x0.Greater(u0.Add(t0))
		m0 := lo0.Or(hi0)
		if m0.ToBits() != 0 {
			v0 := l0.Sub(x0).IfElse(lo0, x0.Sub(u0))
			w0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(dp, r*8)))
			if bm := uint16(v0.Mul(v0).Greater(vBest.Mul(w0)).And(m0).ToBits()); bm != 0 {
				archsimd.ClearAVXUpperBits()
				return r, bm
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return r, 0
}

//go:noinline
func scanDualRatioCandAVX512(
	tabPtr, djPtr, stPtr unsafe.Pointer,
	n int,
	chunks []int,
	chunkPos, elemPos int,
	negLeaveDir, pivotTol, thetaGuard float64,
	filterByTheta bool,
	dst []int,
) (hitCount, nextChunk, nextElem int) {
	signMask := archsimd.BroadcastUint64x8(0x7fffffffffffffff)
	vZero := signMask.Xor(signMask).BitsToFloat64()
	vNegLeave := archsimd.BroadcastFloat64x8(negLeaveDir)
	vPivotTol := archsimd.BroadcastFloat64x8(pivotTol)
	vNegPivotTol := archsimd.BroadcastFloat64x8(-pivotTol)
	vThetaGuard := archsimd.BroadcastFloat64x8(thetaGuard)

	capDst := len(dst)
	cnt := 0

	for ci := chunkPos; ci < len(chunks); ci++ {
		ch := chunks[ci]
		start := ch << 6
		if start >= n {
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
			uTab := archsimd.LoadUint64x8Array((*[8]uint64)(unsafe.Add(tp, i*8)))
			vTab := uTab.BitsToFloat64()
			nz := vTab.NotEqual(vZero).ToBits()
			if nz == 0 {
				continue
			}
			vAlpha := vNegLeave.Mul(vTab)
			gtTol := vAlpha.Greater(vPivotTol).ToBits()
			ltNegTol := vAlpha.Less(vNegPivotTol).ToBits()
			big := gtTol | ltNegTol
			subOrNaN := nz &^ big

			st8 := *(*uint64)(unsafe.Add(sp, i))
			lowB := ((st8 | (st8 >> 1) | (st8 >> 2)) ^ 0x0101010101010101) & 0x0101010101010101
			uppB := (st8 &^ (st8 >> 1)) & 0x0101010101010101
			supB := (st8 >> 2) & 0x0101010101010101
			isLower := uint8(((lowB * 0x0102040810204080) >> 56) & 0xFF)
			isUpper := uint8(((uppB * 0x0102040810204080) >> 56) & 0xFF)
			isSuper := uint8(((supB * 0x0102040810204080) >> 56) & 0xFF)

			blkLower := gtTol & isLower
			blkUpper := ltNegTol & isUpper
			blkSuper := big & isSuper
			blk := blkLower | blkUpper
			if (blk | blkSuper | subOrNaN) == 0 {
				continue
			}
			if blk != 0 && filterByTheta {
				vDJ := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(dp, i*8)))
				vAbs := uTab.And(signMask).BitsToFloat64()
				vLim := vThetaGuard.Mul(vAbs)
				if blkLower != 0 {
					blkLower &^= vDJ.Greater(vLim).ToBits()
				}
				if blkUpper != 0 {
					blkUpper &^= vDJ.Less(vZero.Sub(vLim)).ToBits()
				}
				blk = blkLower | blkUpper
			}
			if hit := blk | blkSuper | subOrNaN; hit != 0 {
				if cnt+8 > capDst {
					archsimd.ClearAVXUpperBits()
					return cnt, ci, i
				}
				base := start + i
				for hit != 0 {
					off := bits.TrailingZeros8(hit)
					hit &= hit - 1
					dst[cnt] = base + off
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
		if i < size {
			rem := size - i
			if cnt+rem > capDst {
				archsimd.ClearAVXUpperBits()
				return cnt, ci, i
			}
			for ; i < size; i++ {
				dst[cnt] = start + i
				cnt++
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return cnt, len(chunks), 0
}
