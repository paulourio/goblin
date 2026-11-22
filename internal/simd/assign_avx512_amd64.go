//go:build goexperiment.simd && amd64

package simd

import (
	"math"
	"math/bits"
	"simd/archsimd"
	"unsafe"
)

var (
	transposeIdxLo4 = [8]uint64{0, 1, 8, 9, 4, 5, 12, 13}
	transposeIdxHi4 = [8]uint64{2, 3, 10, 11, 6, 7, 14, 15}
	transposeIdxLo8 = [8]uint64{0, 1, 2, 3, 8, 9, 10, 11}
	transposeIdxHi8 = [8]uint64{4, 5, 6, 7, 12, 13, 14, 15}
)

//go:noinline
func relaxRow64AVX512(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) (int, bool) {
	n := len(rCost)
	cp := unsafe.Pointer(&rCost[0])
	vp := unsafe.Pointer(&v[0])
	dp := unsafe.Pointer(&d[0])
	pp := unsafe.Pointer(&pred[0])

	vui := archsimd.BroadcastFloat64x8(ui)
	vmin := archsimd.BroadcastFloat64x8(minVal)
	vrow := archsimd.BroadcastInt64x8(int64(rowI))

	j := start
	for ; j+8 <= n; j += 8 {
		vc := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, j*8)))
		vv := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, j*8)))
		vdPtr := (*[8]float64)(unsafe.Add(dp, j*8))
		vd := archsimd.LoadFloat64x8Array(vdPtr)

		vr := vc.Sub(vv).Sub(vui).Add(vmin)
		lt := vr.Less(vd)
		tightBits := vr.LessEqual(vmin).And(lt).ToBits()
		if tightBits != 0 {
			first := bits.TrailingZeros8(tightBits)
			if first > 0 {
				preMask := lt.And(archsimd.Mask64x8FromBits(uint8((1 << first) - 1)))
				if preMask.ToBits() != 0 {
					vr.StoreArrayMasked(vdPtr, preMask)
					vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(pp, j*8)), preMask)
				}
			}
			archsimd.ClearAVXUpperBits()
			return j + first, true
		}
		if lt.ToBits() != 0 {
			vr.StoreArrayMasked(vdPtr, lt)
			vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(pp, j*8)), lt)
		}
	}
	archsimd.ClearAVXUpperBits()
	return j, false
}

//go:noinline
func minNoNaN64AVX512(x []float64) float64 {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	vInf := archsimd.BroadcastFloat64x8(math.Inf(1))
	vmin0 := vInf
	vmin1 := vInf
	j := 0
	for ; j+16 <= n; j += 16 {
		vd0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, j*8)))
		vd1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, (j+8)*8)))
		c0 := vInf.IfElse(vd0.IsNaN(), vd0)
		c1 := vInf.IfElse(vd1.IsNaN(), vd1)
		vmin0 = vmin0.Min(c0)
		vmin1 = vmin1.Min(c1)
	}
	for ; j+8 <= n; j += 8 {
		vd0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, j*8)))
		c0 := vInf.IfElse(vd0.IsNaN(), vd0)
		vmin0 = vmin0.Min(c0)
	}
	vmin0 = vmin0.Min(vmin1)
	var lanes [8]float64
	vmin0.StoreArray(&lanes)
	archsimd.ClearAVXUpperBits()

	res := math.Inf(1)
	for _, v := range lanes {
		if v < res {
			res = v
		}
	}
	for ; j < n; j++ {
		if v := *(*float64)(unsafe.Add(xp, j*8)); v < res {
			res = v
		}
	}
	return res
}

//go:noinline
func collectLE64AVX512(x []float64, start int, limit float64, out []int) (int, int) {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	vlim := archsimd.BroadcastFloat64x8(limit)
	capOut := len(out)
	out = out[:capOut]
	count := 0
	j := start
	for ; j+16 <= n; j += 16 {
		v0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, j*8)))
		v1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, (j+8)*8)))
		m0 := v0.LessEqual(vlim)
		m1 := v1.LessEqual(vlim)
		if m0.Or(m1).ToBits() != 0 {
			bm := uint16(m0.ToBits()) | (uint16(m1.ToBits()) << 8)
			for bm != 0 {
				k := bits.TrailingZeros16(bm)
				if uint(count) >= uint(capOut) {
					archsimd.ClearAVXUpperBits()
					return j + k, count
				}
				out[count] = j + k
				count++
				bm &= bm - 1
			}
		}
	}
	for ; j+8 <= n; j += 8 {
		v0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(xp, j*8)))
		bm := v0.LessEqual(vlim).ToBits()
		for bm != 0 {
			k := bits.TrailingZeros8(bm)
			if uint(count) >= uint(capOut) {
				archsimd.ClearAVXUpperBits()
				return j + k, count
			}
			out[count] = j + k
			count++
			bm &= bm - 1
		}
	}
	archsimd.ClearAVXUpperBits()
	return j, count
}

//go:noinline
func fillIntAVX512(dst []int, val int) {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	v := archsimd.BroadcastInt64x8(int64(val))
	i := 0
	for ; i+16 <= n; i += 16 {
		v.StoreArray((*[8]int64)(unsafe.Add(dp, i*8)))
		v.StoreArray((*[8]int64)(unsafe.Add(dp, (i+8)*8)))
	}
	for ; i+8 <= n; i += 8 {
		v.StoreArray((*[8]int64)(unsafe.Add(dp, i*8)))
	}
	archsimd.ClearAVXUpperBits()
	for ; i < n; i++ {
		*(*int)(unsafe.Add(dp, i*8)) = val
	}
}

//go:noinline
func minReducedSkip64AVX512(rowCost, v []float64, skip int) float64 {
	n := len(rowCost)
	vecEnd := n &^ 7
	skipChunk := vecEnd
	hasSkipChunk := skip >= 0 && skip < vecEnd
	if hasSkipChunk {
		skipChunk = skip &^ 7
	}

	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vInf := archsimd.BroadcastFloat64x8(math.Inf(1))
	vmin0 := vInf
	vmin1 := vInf
	hasNaN0 := vInf.IsNaN()
	hasNaN1 := hasNaN0

	j := 0
	for ; j+16 <= skipChunk; j += 16 {
		r0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, j*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, j*8))))
		r1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, (j+8)*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, (j+8)*8))))
		hasNaN0 = hasNaN0.Or(r0.IsNaN())
		hasNaN1 = hasNaN1.Or(r1.IsNaN())
		vmin0 = r0.Min(vmin0)
		vmin1 = r1.Min(vmin1)
	}
	for ; j+8 <= skipChunk; j += 8 {
		r0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, j*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, j*8))))
		hasNaN0 = hasNaN0.Or(r0.IsNaN())
		vmin0 = r0.Min(vmin0)
	}

	if hasSkipChunk {
		k := skipChunk + 8
		for ; k+16 <= vecEnd; k += 16 {
			r0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, k*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, k*8))))
			r1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, (k+8)*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, (k+8)*8))))
			hasNaN0 = hasNaN0.Or(r0.IsNaN())
			hasNaN1 = hasNaN1.Or(r1.IsNaN())
			vmin0 = r0.Min(vmin0)
			vmin1 = r1.Min(vmin1)
		}
		for ; k+8 <= vecEnd; k += 8 {
			r0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(cp, k*8))).Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(vp, k*8))))
			hasNaN0 = hasNaN0.Or(r0.IsNaN())
			vmin0 = r0.Min(vmin0)
		}
	}

	vmin0 = vmin1.Min(vmin0)
	var lanes [8]float64
	vmin0.StoreArray(&lanes)
	if hasNaN0.Or(hasNaN1).ToBits() != 0 {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()

	minRed := min(
		min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3])),
		min(min(lanes[4], lanes[5]), min(lanes[6], lanes[7])),
	)
	if hasSkipChunk {
		for k := skipChunk; k < skipChunk+8; k++ {
			if k != skip {
				minRed = min(minRed, *(*float64)(unsafe.Add(cp, k*8))-*(*float64)(unsafe.Add(vp, k*8)))
			}
		}
	}
	for k := vecEnd; k < n; k++ {
		if k != skip {
			minRed = min(minRed, *(*float64)(unsafe.Add(cp, k*8))-*(*float64)(unsafe.Add(vp, k*8)))
		}
	}
	if minRed == 0 {
		minRed = 0
	}
	return minRed
}

//go:noinline
func validateAndNegate64AVX512(dst, src []float64, limit float64) bool {
	n := len(src)
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])

	vOne := archsimd.BroadcastFloat64x8(1.0)
	vZero := vOne.Sub(vOne)
	vlim := archsimd.BroadcastFloat64x8(limit)
	vNegLim := archsimd.BroadcastFloat64x8(-limit)
	ok0 := vlim.GreaterEqual(vNegLim)
	ok1 := ok0

	i := 0
	for ; i+16 <= n; i += 16 {
		v0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(sp, i*8)))
		v1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(sp, (i+8)*8)))
		ok0 = ok0.And(v0.LessEqual(vlim)).And(v0.GreaterEqual(vNegLim))
		ok1 = ok1.And(v1.LessEqual(vlim)).And(v1.GreaterEqual(vNegLim))
		vZero.Sub(v0).StoreArray((*[8]float64)(unsafe.Add(dp, i*8)))
		vZero.Sub(v1).StoreArray((*[8]float64)(unsafe.Add(dp, (i+8)*8)))
	}
	for ; i+8 <= n; i += 8 {
		v0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(sp, i*8)))
		ok0 = ok0.And(v0.LessEqual(vlim)).And(v0.GreaterEqual(vNegLim))
		vZero.Sub(v0).StoreArray((*[8]float64)(unsafe.Add(dp, i*8)))
	}

	if ok0.And(ok1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return false
	}
	archsimd.ClearAVXUpperBits()
	return true
}

//go:noinline
func colReduceValidateRows64AVX512(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64) bool {
	vecEnd := n &^ 7
	vp := unsafe.Pointer(&v[0])
	rp := unsafe.Pointer(&row4col[0])

	vlim := archsimd.BroadcastFloat64x8(limit)
	vNegLim := archsimd.BroadcastFloat64x8(-limit)
	ok0 := vlim.GreaterEqual(vNegLim)
	ok1 := ok0

	i := rowStart
	if i == 0 {
		r0p := unsafe.Pointer(&cost[0])
		j := 0
		for ; j+16 <= vecEnd; j += 16 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(r0p, j*8)))
			vc1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(r0p, (j+8)*8)))
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			ok1 = ok1.And(vc1.LessEqual(vlim)).And(vc1.GreaterEqual(vNegLim))
			vc0.StoreArray((*[8]float64)(unsafe.Add(vp, j*8)))
			vc1.StoreArray((*[8]float64)(unsafe.Add(vp, (j+8)*8)))
		}
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(r0p, j*8)))
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			vc0.StoreArray((*[8]float64)(unsafe.Add(vp, j*8)))
		}
		i = 1
	}

	for ; i < rowEnd; i++ {
		rowPtr := unsafe.Pointer(&cost[i*n])
		vrow := archsimd.BroadcastInt64x8(int64(i))
		j := 0
		for ; j+16 <= vecEnd; j += 16 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, j*8)))
			vc1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, (j+8)*8)))
			vv0Ptr := (*[8]float64)(unsafe.Add(vp, j*8))
			vv1Ptr := (*[8]float64)(unsafe.Add(vp, (j+8)*8))
			vv0 := archsimd.LoadFloat64x8Array(vv0Ptr)
			vv1 := archsimd.LoadFloat64x8Array(vv1Ptr)
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			ok1 = ok1.And(vc1.LessEqual(vlim)).And(vc1.GreaterEqual(vNegLim))
			lt0 := vc0.Less(vv0)
			lt1 := vc1.Less(vv1)
			if lt0.Or(lt1).ToBits() != 0 {
				vc0.StoreArrayMasked(vv0Ptr, lt0)
				vc1.StoreArrayMasked(vv1Ptr, lt1)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, j*8)), lt0)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, (j+8)*8)), lt1)
			}
		}
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, j*8)))
			vv0Ptr := (*[8]float64)(unsafe.Add(vp, j*8))
			vv0 := archsimd.LoadFloat64x8Array(vv0Ptr)
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			lt0 := vc0.Less(vv0)
			if lt0.ToBits() != 0 {
				vc0.StoreArrayMasked(vv0Ptr, lt0)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, j*8)), lt0)
			}
		}
	}

	if ok0.And(ok1).ToBits() != 0xff {
		archsimd.ClearAVXUpperBits()
		return false
	}
	archsimd.ClearAVXUpperBits()
	return true
}

//go:noinline
func colReduceRows64AVX512(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	vecEnd := n &^ 7
	vp := unsafe.Pointer(&v[0])
	rp := unsafe.Pointer(&row4col[0])

	for r := rowStart; r < rowEnd; r++ {
		rowPtr := unsafe.Pointer(&cost[r*n])
		vrow := archsimd.BroadcastInt64x8(int64(r))
		j := 0
		for ; j+16 <= vecEnd; j += 16 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, j*8)))
			vc1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, (j+8)*8)))
			vv0Ptr := (*[8]float64)(unsafe.Add(vp, j*8))
			vv1Ptr := (*[8]float64)(unsafe.Add(vp, (j+8)*8))
			vv0 := archsimd.LoadFloat64x8Array(vv0Ptr)
			vv1 := archsimd.LoadFloat64x8Array(vv1Ptr)
			lt0 := vc0.Less(vv0)
			lt1 := vc1.Less(vv1)
			if lt0.Or(lt1).ToBits() != 0 {
				vc0.StoreArrayMasked(vv0Ptr, lt0)
				vc1.StoreArrayMasked(vv1Ptr, lt1)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, j*8)), lt0)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, (j+8)*8)), lt1)
			}
		}
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(rowPtr, j*8)))
			vv0Ptr := (*[8]float64)(unsafe.Add(vp, j*8))
			vv0 := archsimd.LoadFloat64x8Array(vv0Ptr)
			lt0 := vc0.Less(vv0)
			if lt0.ToBits() != 0 {
				vc0.StoreArrayMasked(vv0Ptr, lt0)
				vrow.StoreArrayMasked((*[8]int64)(unsafe.Add(rp, j*8)), lt0)
			}
		}
	}
	archsimd.ClearAVXUpperBits()
}

//go:noinline
func transpose64AVX512(dst, src []float64, rows, cols int, negate bool) {
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])
	rows8 := rows &^ 7
	cols8 := cols &^ 7
	const tile = 16
	srcRowBytes := cols * 8
	srcRowBytes2 := cols * 16
	srcRowBytes3 := cols * 24
	srcRowBytes4 := cols * 32
	dstRowBytes := rows * 8
	dstRowBytes2 := rows * 16
	dstRowBytes3 := rows * 24
	dstRowBytes4 := rows * 32

	idxLo4 := archsimd.LoadUint64x8Array(&transposeIdxLo4)
	idxHi4 := archsimd.LoadUint64x8Array(&transposeIdxHi4)
	idxLo8 := archsimd.LoadUint64x8Array(&transposeIdxLo8)
	idxHi8 := archsimd.LoadUint64x8Array(&transposeIdxHi8)

	if negate {
		vOne := archsimd.BroadcastFloat64x8(1.0)
		vZero := vOne.Sub(vOne)
		for c0 := 0; c0 < cols8; c0 += tile {
			cLim := min(cols8, c0+tile)
			for r0 := 0; r0 < rows8; r0 += tile {
				rLim := min(rows8, r0+tile)
				for c := c0; c < cLim; c += 8 {
					sCol := unsafe.Add(sp, c*8)
					dCol := unsafe.Add(dp, c*dstRowBytes)
					for r := r0; r < rLim; r += 8 {
						s0 := unsafe.Add(sCol, r*srcRowBytes)
						s4 := unsafe.Add(s0, srcRowBytes4)
						a0 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(s0)))
						a1 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes))))
						a2 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes2))))
						a3 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes3))))
						a4 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(s4)))
						a5 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes))))
						a6 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes2))))
						a7 := vZero.Sub(archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes3))))

						t0 := a0.ConcatPermuteScalarsGrouped(0, 2, a1)
						t1 := a0.ConcatPermuteScalarsGrouped(1, 3, a1)
						t2 := a2.ConcatPermuteScalarsGrouped(0, 2, a3)
						t3 := a2.ConcatPermuteScalarsGrouped(1, 3, a3)
						t4 := a4.ConcatPermuteScalarsGrouped(0, 2, a5)
						t5 := a4.ConcatPermuteScalarsGrouped(1, 3, a5)
						t6 := a6.ConcatPermuteScalarsGrouped(0, 2, a7)
						t7 := a6.ConcatPermuteScalarsGrouped(1, 3, a7)

						q0 := t0.ConcatPermute(t2, idxLo4)
						q1 := t1.ConcatPermute(t3, idxLo4)
						q2 := t0.ConcatPermute(t2, idxHi4)
						q3 := t1.ConcatPermute(t3, idxHi4)
						q4 := t4.ConcatPermute(t6, idxLo4)
						q5 := t5.ConcatPermute(t7, idxLo4)
						q6 := t4.ConcatPermute(t6, idxHi4)
						q7 := t5.ConcatPermute(t7, idxHi4)

						d0 := unsafe.Add(dCol, r*8)
						d4 := unsafe.Add(d0, dstRowBytes4)
						q0.ConcatPermute(q4, idxLo8).StoreArray((*[8]float64)(d0))
						q1.ConcatPermute(q5, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes)))
						q2.ConcatPermute(q6, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes2)))
						q3.ConcatPermute(q7, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes3)))
						q0.ConcatPermute(q4, idxHi8).StoreArray((*[8]float64)(d4))
						q1.ConcatPermute(q5, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes)))
						q2.ConcatPermute(q6, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes2)))
						q3.ConcatPermute(q7, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes3)))
					}
				}
			}
		}
		archsimd.ClearAVXUpperBits()
		return
	}

	for c0 := 0; c0 < cols8; c0 += tile {
		cLim := min(cols8, c0+tile)
		for r0 := 0; r0 < rows8; r0 += tile {
			rLim := min(rows8, r0+tile)
			for c := c0; c < cLim; c += 8 {
				sCol := unsafe.Add(sp, c*8)
				dCol := unsafe.Add(dp, c*dstRowBytes)
				for r := r0; r < rLim; r += 8 {
					s0 := unsafe.Add(sCol, r*srcRowBytes)
					s4 := unsafe.Add(s0, srcRowBytes4)
					a0 := archsimd.LoadFloat64x8Array((*[8]float64)(s0))
					a1 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes)))
					a2 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes2)))
					a3 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s0, srcRowBytes3)))
					a4 := archsimd.LoadFloat64x8Array((*[8]float64)(s4))
					a5 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes)))
					a6 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes2)))
					a7 := archsimd.LoadFloat64x8Array((*[8]float64)(unsafe.Add(s4, srcRowBytes3)))

					t0 := a0.ConcatPermuteScalarsGrouped(0, 2, a1)
					t1 := a0.ConcatPermuteScalarsGrouped(1, 3, a1)
					t2 := a2.ConcatPermuteScalarsGrouped(0, 2, a3)
					t3 := a2.ConcatPermuteScalarsGrouped(1, 3, a3)
					t4 := a4.ConcatPermuteScalarsGrouped(0, 2, a5)
					t5 := a4.ConcatPermuteScalarsGrouped(1, 3, a5)
					t6 := a6.ConcatPermuteScalarsGrouped(0, 2, a7)
					t7 := a6.ConcatPermuteScalarsGrouped(1, 3, a7)

					q0 := t0.ConcatPermute(t2, idxLo4)
					q1 := t1.ConcatPermute(t3, idxLo4)
					q2 := t0.ConcatPermute(t2, idxHi4)
					q3 := t1.ConcatPermute(t3, idxHi4)
					q4 := t4.ConcatPermute(t6, idxLo4)
					q5 := t5.ConcatPermute(t7, idxLo4)
					q6 := t4.ConcatPermute(t6, idxHi4)
					q7 := t5.ConcatPermute(t7, idxHi4)

					d0 := unsafe.Add(dCol, r*8)
					d4 := unsafe.Add(d0, dstRowBytes4)
					q0.ConcatPermute(q4, idxLo8).StoreArray((*[8]float64)(d0))
					q1.ConcatPermute(q5, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes)))
					q2.ConcatPermute(q6, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes2)))
					q3.ConcatPermute(q7, idxLo8).StoreArray((*[8]float64)(unsafe.Add(d0, dstRowBytes3)))
					q0.ConcatPermute(q4, idxHi8).StoreArray((*[8]float64)(d4))
					q1.ConcatPermute(q5, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes)))
					q2.ConcatPermute(q6, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes2)))
					q3.ConcatPermute(q7, idxHi8).StoreArray((*[8]float64)(unsafe.Add(d4, dstRowBytes3)))
				}
			}
		}
	}
	archsimd.ClearAVXUpperBits()
}
