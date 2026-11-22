//go:build goexperiment.simd && amd64

package simd

import (
	"math"
	"math/bits"
	"simd/archsimd"
	"unsafe"
)

// RelaxRow64Scalar scans columns j in [start, n) and updates d[j] and pred[j]
// whenever minVal+((rCost[j]-v[j])-ui) < d[j] and the candidate exceeds minVal.
// It returns the first index j where a tight edge satisfies r < d[j] and r <= minVal
// without modifying d[j] or pred[j], or returns n when no tight edge occurs.
func RelaxRow64Scalar(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	n := min(len(rCost), min(len(v), min(len(d), len(pred))))
	if start < 0 {
		start = 0
	}
	if start >= n {
		return n
	}
	// Tie sub-slices to len(cSlice) for BCE.
	cSlice := rCost[:n]
	vSlice := v[:len(cSlice)]
	dSlice := d[:len(cSlice)]
	pSlice := pred[:len(cSlice)]
	j := start
	for ; j <= len(cSlice)-4; j += 4 {
		r0 := minVal + ((cSlice[j] - vSlice[j]) - ui)
		r1 := minVal + ((cSlice[j+1] - vSlice[j+1]) - ui)
		r2 := minVal + ((cSlice[j+2] - vSlice[j+2]) - ui)
		r3 := minVal + ((cSlice[j+3] - vSlice[j+3]) - ui)
		if r0 < dSlice[j] || r1 < dSlice[j+1] || r2 < dSlice[j+2] || r3 < dSlice[j+3] {
			if r0 < dSlice[j] {
				if r0 <= minVal {
					return j
				}
				dSlice[j] = r0
				pSlice[j] = rowI
			}
			if r1 < dSlice[j+1] {
				if r1 <= minVal {
					return j + 1
				}
				dSlice[j+1] = r1
				pSlice[j+1] = rowI
			}
			if r2 < dSlice[j+2] {
				if r2 <= minVal {
					return j + 2
				}
				dSlice[j+2] = r2
				pSlice[j+2] = rowI
			}
			if r3 < dSlice[j+3] {
				if r3 <= minVal {
					return j + 3
				}
				dSlice[j+3] = r3
				pSlice[j+3] = rowI
			}
		}
	}
	for ; j < len(cSlice); j++ {
		r := minVal + ((cSlice[j] - vSlice[j]) - ui)
		if r < dSlice[j] {
			if r <= minVal {
				return j
			}
			dSlice[j] = r
			pSlice[j] = rowI
		}
	}
	return n
}

// RelaxRow64With relaxes contiguous distances d[start:n] for row rowI,
// honoring a per-call disableSIMD flag in addition to the process-wide flag.
// When returning ret < n, all columns in [start, ret) are already updated,
// columns in [ret, n) are unmodified, and a tight edge exists in [ret, min(ret+4, n)).
func RelaxRow64With(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64, disableSIMD bool) int {
	n := min(len(rCost), min(len(v), min(len(d), len(pred))))
	if start < 0 {
		start = 0
	}
	if start >= n {
		return n
	}
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n-start < 4 {
		return RelaxRow64Scalar(rCost[:n], v[:n], d[:n], pred[:n], start, rowI, ui, minVal)
	}
	j := start
	if tier == tierAVX512 && n >= minAVX512Len && n-j >= 8 {
		var found bool
		j, found = relaxRow64AVX512(rCost[:n], v[:n], d[:n], pred[:n], j, rowI, ui, minVal)
		if found {
			return j
		}
	} else {
		j = relaxRow64AVX2(rCost[:n], v[:n], d[:n], pred[:n], j, rowI, ui, minVal)
		if j+4 <= n {
			return j
		}
	}
	for ; j < n; j++ {
		r := minVal + ((rCost[j] - v[j]) - ui)
		if r < d[j] {
			if r <= minVal {
				return j
			}
			d[j] = r
			pred[j] = rowI
		}
	}
	return n
}

// RelaxRow64 relaxes contiguous distances d[start:n] for row rowI using AVX-512 or AVX2 when enabled.
func RelaxRow64(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	return RelaxRow64With(rCost, v, d, pred, start, rowI, ui, minVal, false)
}

//go:noinline
func relaxRow64AVX2(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	n := len(rCost)
	cp := unsafe.Pointer(&rCost[0])
	vp := unsafe.Pointer(&v[0])
	dp := unsafe.Pointer(&d[0])
	pp := unsafe.Pointer(&pred[0])

	vui := archsimd.BroadcastFloat64x4(ui)
	vmin := archsimd.BroadcastFloat64x4(minVal)
	vrow := archsimd.BroadcastInt64x4(int64(rowI))

	j := start
	lim := n - 3
	for ; j < lim; j += 4 {
		vc := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8)))
		vv := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8)))
		vdPtr := (*[4]float64)(unsafe.Add(dp, j*8))
		vd := archsimd.LoadFloat64x4Array(vdPtr)

		vr := vc.Sub(vv).Sub(vui).Add(vmin)
		lt := vr.Less(vd)
		if vr.LessEqual(vmin).And(lt).ToBits() != 0 {
			archsimd.ClearAVXUpperBits()
			return j
		}
		vr.IfElse(lt, vd).StoreArray(vdPtr)
		vpPtr := (*[4]int64)(unsafe.Add(pp, j*8))
		vpOld := archsimd.LoadInt64x4Array(vpPtr)
		vrow.IfElse(lt, vpOld).StoreArray(vpPtr)
	}
	archsimd.ClearAVXUpperBits()
	return j
}

// MinNoNaN64Scalar returns the minimum non-NaN value in x, or +Inf if x is empty or all-NaN.
// Keep i <= len(x)-4 for BCE.
func MinNoNaN64Scalar(x []float64) float64 {
	res := math.Inf(1)
	i := 0
	for ; i <= len(x)-4; i += 4 {
		v0, v1, v2, v3 := x[i], x[i+1], x[i+2], x[i+3]
		if v0 < res {
			res = v0
		}
		if v1 < res {
			res = v1
		}
		if v2 < res {
			res = v2
		}
		if v3 < res {
			res = v3
		}
	}
	for ; i < len(x); i++ {
		if x[i] < res {
			res = x[i]
		}
	}
	if res == 0 {
		res = 0
	}
	return res
}

// MinNoNaN64With returns the minimum non-NaN value in x, honoring a per-call disableSIMD flag.
func MinNoNaN64With(x []float64, disableSIMD bool) float64 {
	tier := currentTier(disableSIMD)
	if tier == tierAVX512 && len(x) >= minAVX512Len {
		return minNoNaN64AVX512(x)
	}
	if tier >= tierAVX2 && len(x) >= minAVX2Len {
		return minNoNaN64AVX2(x)
	}
	return MinNoNaN64Scalar(x)
}

// MinNoNaN64 returns the minimum non-NaN value in x using AVX-512 or AVX2 when enabled.
func MinNoNaN64(x []float64) float64 {
	return MinNoNaN64With(x, false)
}

//go:noinline
func minNoNaN64AVX2(x []float64) float64 {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	vInf := archsimd.BroadcastFloat64x4(math.Inf(1))
	vmin0 := vInf
	vmin1 := vInf
	j := 0
	for ; j+8 <= n; j += 8 {
		vd0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, j*8)))
		vd1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, (j+4)*8)))
		c0 := vInf.IfElse(vd0.IsNaN(), vd0)
		c1 := vInf.IfElse(vd1.IsNaN(), vd1)
		vmin0 = vmin0.Min(c0)
		vmin1 = vmin1.Min(c1)
	}
	for ; j+4 <= n; j += 4 {
		vd0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, j*8)))
		c0 := vInf.IfElse(vd0.IsNaN(), vd0)
		vmin0 = vmin0.Min(c0)
	}
	vmin0 = vmin0.Min(vmin1)
	var lanes [4]float64
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
	if res == 0 {
		res = 0
	}
	return res
}

// CollectLE64Scalar scans x[start:len(x)] and writes up to len(out) indices j where x[j] <= limit
// (ignoring NaN entries), returning the next unscanned index and the number of indices written.
func CollectLE64Scalar(x []float64, start int, limit float64, out []int) (next, count int) {
	n := len(x)
	if start < 0 {
		start = 0
	}
	if start >= n || len(out) == 0 {
		return min(max(start, 0), n), 0
	}
	xTail := x[start:n]
	outSlice := out
	cnt := 0
	i := 0
	for ; i <= len(xTail)-4; i += 4 {
		v0, v1, v2, v3 := xTail[i], xTail[i+1], xTail[i+2], xTail[i+3]
		if v0 <= limit || v1 <= limit || v2 <= limit || v3 <= limit {
			if v0 <= limit {
				if cnt >= len(outSlice) {
					return start + i, cnt
				}
				outSlice[cnt] = start + i
				cnt++
			}
			if v1 <= limit {
				if cnt >= len(outSlice) {
					return start + i + 1, cnt
				}
				outSlice[cnt] = start + i + 1
				cnt++
			}
			if v2 <= limit {
				if cnt >= len(outSlice) {
					return start + i + 2, cnt
				}
				outSlice[cnt] = start + i + 2
				cnt++
			}
			if v3 <= limit {
				if cnt >= len(outSlice) {
					return start + i + 3, cnt
				}
				outSlice[cnt] = start + i + 3
				cnt++
			}
		}
	}
	for ; i < len(xTail); i++ {
		if xTail[i] <= limit {
			if cnt >= len(outSlice) {
				return start + i, cnt
			}
			outSlice[cnt] = start + i
			cnt++
		}
	}
	return n, cnt
}

// CollectLE64With scans x[start:len(x)] and writes up to len(out) indices j where x[j] <= limit,
// dispatching to AVX-512 or AVX2 when enabled.
func CollectLE64With(x []float64, start int, limit float64, out []int, disableSIMD bool) (next, count int) {
	n := len(x)
	if start < 0 {
		start = 0
	}
	if start >= n || len(out) == 0 {
		return min(max(start, 0), n), 0
	}
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n-start < minAVX2Len {
		return CollectLE64Scalar(x, start, limit, out)
	}
	var j int
	if tier == tierAVX512 && n-start >= minAVX512Len {
		j, count = collectLE64AVX512(x, start, limit, out)
	} else {
		j, count = collectLE64AVX2(x, start, limit, out)
	}
	if j >= n {
		return j, count
	}
	for ; j < n; j++ {
		if x[j] <= limit {
			if count == len(out) {
				return j, count
			}
			out[count] = j
			count++
		}
	}
	return n, count
}

// CollectLE64 scans x[start:len(x)] for entries <= limit using AVX-512 or AVX2 when enabled.
func CollectLE64(x []float64, start int, limit float64, out []int) (next, count int) {
	return CollectLE64With(x, start, limit, out, false)
}

//go:noinline
func collectLE64AVX2(x []float64, start int, limit float64, out []int) (int, int) {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	vlim := archsimd.BroadcastFloat64x4(limit)
	capOut := len(out)
	out = out[:capOut]
	count := 0
	j := start
	for ; j+8 <= n; j += 8 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, j*8)))
		v1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, (j+4)*8)))
		m0 := v0.LessEqual(vlim)
		m1 := v1.LessEqual(vlim)
		if m0.Or(m1).ToBits() != 0 {
			bm := (m0.ToBits() & 0xF) | ((m1.ToBits() & 0xF) << 4)
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
	}
	for ; j+4 <= n; j += 4 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(xp, j*8)))
		bm := v0.LessEqual(vlim).ToBits() & 0xF
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

// FillIntScalar sets every element of dst to val using 4-way scalar unrolling.
func FillIntScalar(dst []int, val int) {
	i := 0
	for ; i <= len(dst)-4; i += 4 {
		dst[i] = val
		dst[i+1] = val
		dst[i+2] = val
		dst[i+3] = val
	}
	for ; i < len(dst); i++ {
		dst[i] = val
	}
}

// FillIntWith sets every element of dst to val, dispatching to AVX-512 or AVX2 when enabled.
func FillIntWith(dst []int, val int, disableSIMD bool) {
	n := len(dst)
	tier := currentTier(disableSIMD)
	if tier == tierAVX512 && n >= minAVX512Len {
		fillIntAVX512(dst, val)
		return
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		fillIntAVX2(dst, val)
		return
	}
	FillIntScalar(dst, val)
}

// FillInt sets every element of dst to val using AVX-512 or AVX2 when enabled.
func FillInt(dst []int, val int) {
	FillIntWith(dst, val, false)
}

//go:noinline
func fillIntAVX2(dst []int, val int) {
	n := len(dst)
	dp := unsafe.Pointer(&dst[0])
	v := archsimd.BroadcastInt64x4(int64(val))
	i := 0
	for ; i+8 <= n; i += 8 {
		v.StoreArray((*[4]int64)(unsafe.Add(dp, i*8)))
		v.StoreArray((*[4]int64)(unsafe.Add(dp, (i+4)*8)))
	}
	for ; i+4 <= n; i += 4 {
		v.StoreArray((*[4]int64)(unsafe.Add(dp, i*8)))
	}
	archsimd.ClearAVXUpperBits()
	for ; i < n; i++ {
		*(*int)(unsafe.Add(dp, i*8)) = val
	}
}

// MinReducedSkip64Scalar returns min_{j != skip} (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)).
func MinReducedSkip64Scalar(rowCost, v []float64, skip int) float64 {
	n := min(len(rowCost), len(v))
	if n == 0 {
		return math.Inf(1)
	}
	if skip < 0 || skip >= n {
		return MinReducedSlice64Scalar(rowCost[:n], v[:n])
	}
	return min(
		MinReducedSlice64Scalar(rowCost[:skip], v[:skip]),
		MinReducedSlice64Scalar(rowCost[skip+1:n], v[skip+1:n]),
	)
}

// MinReducedSkip64With returns min_{j != skip} (rowCost[j] - v[j]), honoring disableSIMD.
func MinReducedSkip64With(rowCost, v []float64, skip int, disableSIMD bool) float64 {
	n := min(len(rowCost), len(v))
	tier := currentTier(disableSIMD)
	if tier == tierAVX512 && n >= minAVX512Len {
		return minReducedSkip64AVX512(rowCost[:n], v[:n], skip)
	}
	if tier >= tierAVX2 && n >= minAVX2Len {
		return minReducedSkip64AVX2(rowCost[:n], v[:n], skip)
	}
	return MinReducedSkip64Scalar(rowCost, v, skip)
}

// MinReducedSkip64 returns min_{j != skip} (rowCost[j] - v[j]) using AVX-512 or AVX2 when enabled.
func MinReducedSkip64(rowCost, v []float64, skip int) float64 {
	return MinReducedSkip64With(rowCost, v, skip, false)
}

//go:noinline
func minReducedSkip64AVX2(rowCost, v []float64, skip int) float64 {
	n := len(rowCost)
	vecEnd := n &^ 3
	skipChunk := vecEnd
	hasSkipChunk := skip >= 0 && skip < vecEnd
	if hasSkipChunk {
		skipChunk = skip &^ 3
	}

	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vInf := archsimd.BroadcastFloat64x4(math.Inf(1))
	vmin0 := vInf
	vmin1 := vInf
	hasNaN0 := vInf.IsNaN()
	hasNaN1 := hasNaN0

	j := 0
	for ; j+8 <= skipChunk; j += 8 {
		r0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8))))
		r1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, (j+4)*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, (j+4)*8))))
		hasNaN0 = hasNaN0.Or(r0.IsNaN())
		hasNaN1 = hasNaN1.Or(r1.IsNaN())
		vmin0 = r0.Min(vmin0)
		vmin1 = r1.Min(vmin1)
	}
	for ; j+4 <= skipChunk; j += 4 {
		r0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8))))
		hasNaN0 = hasNaN0.Or(r0.IsNaN())
		vmin0 = r0.Min(vmin0)
	}

	if hasSkipChunk {
		k := skipChunk + 4
		for ; k+8 <= vecEnd; k += 8 {
			r0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, k*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, k*8))))
			r1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, (k+4)*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, (k+4)*8))))
			hasNaN0 = hasNaN0.Or(r0.IsNaN())
			hasNaN1 = hasNaN1.Or(r1.IsNaN())
			vmin0 = r0.Min(vmin0)
			vmin1 = r1.Min(vmin1)
		}
		for ; k+4 <= vecEnd; k += 4 {
			r0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, k*8))).Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, k*8))))
			hasNaN0 = hasNaN0.Or(r0.IsNaN())
			vmin0 = r0.Min(vmin0)
		}
	}

	vmin0 = vmin1.Min(vmin0)
	var lanes [4]float64
	vmin0.StoreArray(&lanes)
	if hasNaN0.Or(hasNaN1).ToBits() != 0 {
		archsimd.ClearAVXUpperBits()
		return math.NaN()
	}
	archsimd.ClearAVXUpperBits()

	minRed := min(min(lanes[0], lanes[1]), min(lanes[2], lanes[3]))
	if hasSkipChunk {
		for k := skipChunk; k < skipChunk+4; k++ {
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

// ValidateAndNegate64Scalar writes dst[i] = 0.0 - src[i] for every index i
// and returns true if every element of src is finite with |src[i]| <= limit.
// Aliasing dst and src identically is safe because all n elements are always written.
// Tie dSlice to len(sSlice) for BCE.
func ValidateAndNegate64Scalar(dst, src []float64, limit float64) bool {
	n := min(len(dst), len(src))
	if n == 0 {
		return !(limit < 0 || math.IsNaN(limit))
	}
	sSlice := src[:n]
	dSlice := dst[:len(sSlice)]
	allOK := true
	i := 0
	for ; i <= len(sSlice)-4; i += 4 {
		c0, c1, c2, c3 := sSlice[i], sSlice[i+1], sSlice[i+2], sSlice[i+3]
		if !(math.Abs(c0) <= limit &&
			math.Abs(c1) <= limit &&
			math.Abs(c2) <= limit &&
			math.Abs(c3) <= limit) {
			allOK = false
		}
		dSlice[i] = 0.0 - c0
		dSlice[i+1] = 0.0 - c1
		dSlice[i+2] = 0.0 - c2
		dSlice[i+3] = 0.0 - c3
	}
	for ; i < len(sSlice); i++ {
		c := sSlice[i]
		if !(math.Abs(c) <= limit) {
			allOK = false
		}
		dSlice[i] = 0.0 - c
	}
	return allOK
}

// ValidateAndNegate64With writes dst[i] = 0.0 - src[i] (preserving +0.0) and reports
// whether all entries satisfy |src[i]| <= limit, honoring disableSIMD.
func ValidateAndNegate64With(dst, src []float64, limit float64, disableSIMD bool) bool {
	n := min(len(dst), len(src))
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < minAVX2Len {
		return ValidateAndNegate64Scalar(dst, src, limit)
	}
	var vecEnd int
	var allOK bool
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		allOK = validateAndNegate64AVX512(dst[:vecEnd], src[:vecEnd], limit)
	} else {
		vecEnd = n &^ 3
		allOK = validateAndNegate64AVX2(dst[:vecEnd], src[:vecEnd], limit)
	}
	for i := vecEnd; i < n; i++ {
		c := src[i]
		if !(math.Abs(c) <= limit) {
			allOK = false
		}
		dst[i] = 0.0 - c
	}
	return allOK
}

// ValidateAndNegate64 writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy |src[i]| <= limit.
func ValidateAndNegate64(dst, src []float64, limit float64) bool {
	return ValidateAndNegate64With(dst, src, limit, false)
}

//go:noinline
func validateAndNegate64AVX2(dst, src []float64, limit float64) bool {
	n := len(src)
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])

	vOne := archsimd.BroadcastFloat64x4(1.0)
	vZero := vOne.Add(archsimd.BroadcastFloat64x4(-1.0))
	vlim := archsimd.BroadcastFloat64x4(limit)
	vNegLim := archsimd.BroadcastFloat64x4(-limit)
	ok0 := vlim.GreaterEqual(vNegLim)
	ok1 := ok0

	i := 0
	for ; i+8 <= n; i += 8 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, i*8)))
		v1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, (i+4)*8)))
		ok0 = ok0.And(v0.LessEqual(vlim)).And(v0.GreaterEqual(vNegLim))
		ok1 = ok1.And(v1.LessEqual(vlim)).And(v1.GreaterEqual(vNegLim))
		vZero.Sub(v0).StoreArray((*[4]float64)(unsafe.Add(dp, i*8)))
		vZero.Sub(v1).StoreArray((*[4]float64)(unsafe.Add(dp, (i+4)*8)))
	}
	for ; i+4 <= n; i += 4 {
		v0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, i*8)))
		ok0 = ok0.And(v0.LessEqual(vlim)).And(v0.GreaterEqual(vNegLim))
		vZero.Sub(v0).StoreArray((*[4]float64)(unsafe.Add(dp, i*8)))
	}

	if ok0.And(ok1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return false
	}
	archsimd.ClearAVXUpperBits()
	return true
}

// ColReduceValidateRows64Scalar validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd) of an n-column matrix, initializing v and row4col when rowStart == 0.
func ColReduceValidateRows64Scalar(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64) bool {
	if n <= 0 || rowEnd <= rowStart {
		return !(limit < 0 || math.IsNaN(limit))
	}
	if rowStart < 0 || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
		return false
	}
	v = v[:n]
	row4col = row4col[:n]
	i := rowStart
	if i == 0 {
		r0 := cost[:n]
		for j, c := range r0 {
			if !(math.Abs(c) <= limit) {
				return false
			}
			v[j] = c
			row4col[j] = 0
		}
		i = 1
	}
	for ; i < rowEnd; i++ {
		rOff := i * n
		// Tie vSlice and r4c to len(rSlice) for BCE.
		rSlice := cost[rOff : rOff+n]
		vSlice := v[:len(rSlice)]
		r4c := row4col[:len(rSlice)]
		j := 0
		for ; j <= len(rSlice)-4; j += 4 {
			c0, c1, c2, c3 := rSlice[j], rSlice[j+1], rSlice[j+2], rSlice[j+3]
			if !(math.Abs(c0) <= limit &&
				math.Abs(c1) <= limit &&
				math.Abs(c2) <= limit &&
				math.Abs(c3) <= limit) {
				return false
			}
			v0, v1, v2, v3 := vSlice[j], vSlice[j+1], vSlice[j+2], vSlice[j+3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[j] = c0
					r4c[j] = i
				}
				if c1 < v1 {
					vSlice[j+1] = c1
					r4c[j+1] = i
				}
				if c2 < v2 {
					vSlice[j+2] = c2
					r4c[j+2] = i
				}
				if c3 < v3 {
					vSlice[j+3] = c3
					r4c[j+3] = i
				}
			}
		}
		for ; j < len(rSlice); j++ {
			c := rSlice[j]
			if !(math.Abs(c) <= limit) {
				return false
			}
			if c < vSlice[j] {
				vSlice[j] = c
				r4c[j] = i
			}
		}
	}
	return true
}

// ColReduceValidateRows64With validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd), honoring disableSIMD.
func ColReduceValidateRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64, disableSIMD bool) bool {
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < minAVX2Len || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
		return ColReduceValidateRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit)
	}
	v = v[:n]
	row4col = row4col[:n]
	if rowStart == 0 {
		clear(row4col)
	}
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		if !colReduceValidateRows64AVX512(n, rowStart, rowEnd, cost, v, row4col, limit) {
			return false
		}
		vecEnd = n &^ 7
	} else {
		if !colReduceValidateRows64AVX2(n, rowStart, rowEnd, cost, v, row4col, limit) {
			return false
		}
		vecEnd = n &^ 3
	}
	if vecEnd < n {
		r := rowStart
		if r == 0 {
			for j := vecEnd; j < n; j++ {
				c := cost[j]
				if !(math.Abs(c) <= limit) {
					return false
				}
				v[j] = c
			}
			r = 1
		}
		for ; r < rowEnd; r++ {
			rowOff := r * n
			for j := vecEnd; j < n; j++ {
				c := cost[rowOff+j]
				if !(math.Abs(c) <= limit) {
					return false
				}
				if c < v[j] {
					v[j] = c
					row4col[j] = r
				}
			}
		}
	}
	return true
}

// ColReduceValidateRows64 validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd) using AVX-512 or AVX2 when enabled.
func ColReduceValidateRows64(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64) bool {
	return ColReduceValidateRows64With(n, rowStart, rowEnd, cost, v, row4col, limit, false)
}

//go:noinline
func colReduceValidateRows64AVX2(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64) bool {
	vecEnd := n &^ 3
	vp := unsafe.Pointer(&v[0])
	rp := unsafe.Pointer(&row4col[0])

	vlim := archsimd.BroadcastFloat64x4(limit)
	vNegLim := archsimd.BroadcastFloat64x4(-limit)
	ok0 := vlim.GreaterEqual(vNegLim)
	ok1 := ok0

	i := rowStart
	if i == 0 {
		r0p := unsafe.Pointer(&cost[0])
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(r0p, j*8)))
			vc1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(r0p, (j+4)*8)))
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			ok1 = ok1.And(vc1.LessEqual(vlim)).And(vc1.GreaterEqual(vNegLim))
			vc0.StoreArray((*[4]float64)(unsafe.Add(vp, j*8)))
			vc1.StoreArray((*[4]float64)(unsafe.Add(vp, (j+4)*8)))
		}
		for ; j+4 <= vecEnd; j += 4 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(r0p, j*8)))
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			vc0.StoreArray((*[4]float64)(unsafe.Add(vp, j*8)))
		}
		i = 1
	}

	for ; i < rowEnd; i++ {
		rowPtr := unsafe.Pointer(&cost[i*n])
		vrow := archsimd.BroadcastInt64x4(int64(i))
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, j*8)))
			vc1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, (j+4)*8)))
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv1Ptr := (*[4]float64)(unsafe.Add(vp, (j+4)*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			vv1 := archsimd.LoadFloat64x4Array(vv1Ptr)
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			ok1 = ok1.And(vc1.LessEqual(vlim)).And(vc1.GreaterEqual(vNegLim))
			lt0 := vc0.Less(vv0)
			lt1 := vc1.Less(vv1)
			if lt0.Or(lt1).ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				vc1.IfElse(lt1, vv1).StoreArray(vv1Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				r1Ptr := (*[4]int64)(unsafe.Add(rp, (j+4)*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
				vrow.IfElse(lt1, archsimd.LoadInt64x4Array(r1Ptr)).StoreArray(r1Ptr)
			}
		}
		for ; j+4 <= vecEnd; j += 4 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, j*8)))
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			ok0 = ok0.And(vc0.LessEqual(vlim)).And(vc0.GreaterEqual(vNegLim))
			lt0 := vc0.Less(vv0)
			if lt0.ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
			}
		}
	}

	if ok0.And(ok1).ToBits() != 0xf {
		archsimd.ClearAVXUpperBits()
		return false
	}
	archsimd.ClearAVXUpperBits()
	return true
}

// ColReduceRows64Scalar performs row-major column reduction over rows [rowStart, rowEnd)
// of an n-column already-validated matrix, initializing v and row4col when rowStart == 0.
func ColReduceRows64Scalar(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	if n <= 0 || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
		return
	}
	v = v[:n]
	row4col = row4col[:n]
	i := rowStart
	if i == 0 {
		copy(v, cost[:n])
		clear(row4col)
		i = 1
	}
	for ; i < rowEnd; i++ {
		rOff := i * n
		// Tie vSlice and r4c to len(rSlice) for BCE.
		rSlice := cost[rOff : rOff+n]
		vSlice := v[:len(rSlice)]
		r4c := row4col[:len(rSlice)]
		j := 0
		for ; j <= len(rSlice)-4; j += 4 {
			c0, c1, c2, c3 := rSlice[j], rSlice[j+1], rSlice[j+2], rSlice[j+3]
			v0, v1, v2, v3 := vSlice[j], vSlice[j+1], vSlice[j+2], vSlice[j+3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[j] = c0
					r4c[j] = i
				}
				if c1 < v1 {
					vSlice[j+1] = c1
					r4c[j+1] = i
				}
				if c2 < v2 {
					vSlice[j+2] = c2
					r4c[j+2] = i
				}
				if c3 < v3 {
					vSlice[j+3] = c3
					r4c[j+3] = i
				}
			}
		}
		for ; j < len(rSlice); j++ {
			c := rSlice[j]
			if c < vSlice[j] {
				vSlice[j] = c
				r4c[j] = i
			}
		}
	}
}

// ColReduceRows64With performs row-major column reduction over rows [rowStart, rowEnd),
// honoring disableSIMD.
func ColReduceRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, disableSIMD bool) {
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < minAVX2Len || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
		ColReduceRows64Scalar(n, rowStart, rowEnd, cost, v, row4col)
		return
	}
	v = v[:n]
	row4col = row4col[:n]
	i := rowStart
	if i == 0 {
		copy(v, cost[:n])
		clear(row4col)
		i = 1
	}
	if i >= rowEnd {
		return
	}
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		colReduceRows64AVX512(n, i, rowEnd, cost, v, row4col)
		vecEnd = n &^ 7
	} else {
		colReduceRows64AVX2(n, i, rowEnd, cost, v, row4col)
		vecEnd = n &^ 3
	}
	if vecEnd < n {
		for r := i; r < rowEnd; r++ {
			rowOff := r * n
			for j := vecEnd; j < n; j++ {
				if c := cost[rowOff+j]; c < v[j] {
					v[j] = c
					row4col[j] = r
				}
			}
		}
	}
}

// ColReduceRows64 performs row-major column reduction over rows [rowStart, rowEnd) using AVX-512 or AVX2 when enabled.
func ColReduceRows64(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	ColReduceRows64With(n, rowStart, rowEnd, cost, v, row4col, false)
}

//go:noinline
func colReduceRows64AVX2(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	vecEnd := n &^ 3
	vp := unsafe.Pointer(&v[0])
	rp := unsafe.Pointer(&row4col[0])

	for r := rowStart; r < rowEnd; r++ {
		rowPtr := unsafe.Pointer(&cost[r*n])
		vrow := archsimd.BroadcastInt64x4(int64(r))
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, j*8)))
			vc1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, (j+4)*8)))
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv1Ptr := (*[4]float64)(unsafe.Add(vp, (j+4)*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			vv1 := archsimd.LoadFloat64x4Array(vv1Ptr)
			lt0 := vc0.Less(vv0)
			lt1 := vc1.Less(vv1)
			if lt0.Or(lt1).ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				vc1.IfElse(lt1, vv1).StoreArray(vv1Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				r1Ptr := (*[4]int64)(unsafe.Add(rp, (j+4)*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
				vrow.IfElse(lt1, archsimd.LoadInt64x4Array(r1Ptr)).StoreArray(r1Ptr)
			}
		}
		for ; j+4 <= vecEnd; j += 4 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(rowPtr, j*8)))
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			lt0 := vc0.Less(vv0)
			if lt0.ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
			}
		}
	}
	archsimd.ClearAVXUpperBits()
}

// Transpose64Scalar transposes a row-major rows-by-cols matrix src into cols-by-rows dst
// using cache-tiled contiguous destination writes. When negate is true, entries are stored as 0.0 - val.
func Transpose64Scalar(dst, src []float64, rows, cols int, negate bool) {
	if rows <= 0 || cols <= 0 {
		return
	}
	total := rows * cols
	if len(dst) < total || len(src) < total {
		return
	}
	src = src[:total]
	dst = dst[:total]
	const tile = 16
	c2 := cols * 2
	c3 := cols * 3
	c4 := cols * 4
	if negate {
		for c0 := 0; c0 < cols; c0 += tile {
			cLim := min(cols, c0+tile)
			for r0 := 0; r0 < rows; r0 += tile {
				rLim := min(rows, r0+tile)
				for c := c0; c < cLim; c++ {
					dOff := c * rows
					dstRow := dst[dOff+r0 : dOff+rLim]
					base := r0*cols + c
					k := 0
					for ; k <= len(dstRow)-4; k += 4 {
						dstRow[k] = 0.0 - src[base]
						dstRow[k+1] = 0.0 - src[base+cols]
						dstRow[k+2] = 0.0 - src[base+c2]
						dstRow[k+3] = 0.0 - src[base+c3]
						base += c4
					}
					for ; k < len(dstRow); k++ {
						dstRow[k] = 0.0 - src[base]
						base += cols
					}
				}
			}
		}
		return
	}
	for c0 := 0; c0 < cols; c0 += tile {
		cLim := min(cols, c0+tile)
		for r0 := 0; r0 < rows; r0 += tile {
			rLim := min(rows, r0+tile)
			for c := c0; c < cLim; c++ {
				dOff := c * rows
				dstRow := dst[dOff+r0 : dOff+rLim]
				base := r0*cols + c
				k := 0
				for ; k <= len(dstRow)-4; k += 4 {
					dstRow[k] = src[base]
					dstRow[k+1] = src[base+cols]
					dstRow[k+2] = src[base+c2]
					dstRow[k+3] = src[base+c3]
					base += c4
				}
				for ; k < len(dstRow); k++ {
					dstRow[k] = src[base]
					base += cols
				}
			}
		}
	}
}

// Transpose64With transposes a row-major rows-by-cols matrix src into cols-by-rows dst,
// optionally negating entries as 0.0 - val and honoring disableSIMD.
func Transpose64With(dst, src []float64, rows, cols int, negate, disableSIMD bool) {
	tier := currentTier(disableSIMD)
	if tier == tierScalar || rows < 8 || cols < 8 {
		Transpose64Scalar(dst, src, rows, cols, negate)
		return
	}
	total := rows * cols
	if len(dst) < total || len(src) < total {
		return
	}
	dst = dst[:total]
	src = src[:total]

	var rowsVec, colsVec int
	if tier == tierAVX512 {
		transpose64AVX512(dst, src, rows, cols, negate)
		rowsVec = rows &^ 7
		colsVec = cols &^ 7
	} else {
		transpose64AVX2(dst, src, rows, cols, negate)
		rowsVec = rows &^ 3
		colsVec = cols &^ 3
	}

	if negate {
		for c := colsVec; c < cols; c++ {
			dstRow := dst[c*rows : (c+1)*rows]
			for r := range rows {
				dstRow[r] = 0.0 - src[r*cols+c]
			}
		}
		for c := range colsVec {
			dstTail := dst[c*rows+rowsVec : (c+1)*rows]
			for k := range dstTail {
				dstTail[k] = 0.0 - src[(rowsVec+k)*cols+c]
			}
		}
		return
	}
	for c := colsVec; c < cols; c++ {
		dstRow := dst[c*rows : (c+1)*rows]
		for r := range rows {
			dstRow[r] = src[r*cols+c]
		}
	}
	for c := range colsVec {
		dstTail := dst[c*rows+rowsVec : (c+1)*rows]
		for k := range dstTail {
			dstTail[k] = src[(rowsVec+k)*cols+c]
		}
	}
}

// Transpose64 transposes a row-major rows-by-cols matrix src into cols-by-rows dst using AVX-512 or AVX2 when enabled.
func Transpose64(dst, src []float64, rows, cols int, negate bool) {
	Transpose64With(dst, src, rows, cols, negate, false)
}

//go:noinline
func transpose64AVX2(dst, src []float64, rows, cols int, negate bool) {
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])
	rows4 := rows &^ 3
	cols4 := cols &^ 3
	const tile = 16

	if negate {
		vOne := archsimd.BroadcastFloat64x4(1.0)
		vZero := vOne.Sub(vOne)
		for c0 := 0; c0 < cols4; c0 += tile {
			cLim := min(cols4, c0+tile)
			for r0 := 0; r0 < rows4; r0 += tile {
				rLim := min(rows4, r0+tile)
				for c := c0; c < cLim; c += 4 {
					for r := r0; r < rLim; r += 4 {
						a0 := vZero.Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, (r*cols+c)*8))))
						a1 := vZero.Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+1)*cols+c)*8))))
						a2 := vZero.Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+2)*cols+c)*8))))
						a3 := vZero.Sub(archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+3)*cols+c)*8))))
						t0 := a0.ConcatPermuteScalarsGrouped(0, 2, a1)
						t1 := a0.ConcatPermuteScalarsGrouped(1, 3, a1)
						t2 := a2.ConcatPermuteScalarsGrouped(0, 2, a3)
						t3 := a2.ConcatPermuteScalarsGrouped(1, 3, a3)
						t0.ConcatPermute128Scalars(0, 2, t2).StoreArray((*[4]float64)(unsafe.Add(dp, (c*rows+r)*8)))
						t1.ConcatPermute128Scalars(0, 2, t3).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+1)*rows+r)*8)))
						t0.ConcatPermute128Scalars(1, 3, t2).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+2)*rows+r)*8)))
						t1.ConcatPermute128Scalars(1, 3, t3).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+3)*rows+r)*8)))
					}
				}
			}
		}
		archsimd.ClearAVXUpperBits()
		return
	}

	for c0 := 0; c0 < cols4; c0 += tile {
		cLim := min(cols4, c0+tile)
		for r0 := 0; r0 < rows4; r0 += tile {
			rLim := min(rows4, r0+tile)
			for c := c0; c < cLim; c += 4 {
				for r := r0; r < rLim; r += 4 {
					a0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, (r*cols+c)*8)))
					a1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+1)*cols+c)*8)))
					a2 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+2)*cols+c)*8)))
					a3 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(sp, ((r+3)*cols+c)*8)))
					t0 := a0.ConcatPermuteScalarsGrouped(0, 2, a1)
					t1 := a0.ConcatPermuteScalarsGrouped(1, 3, a1)
					t2 := a2.ConcatPermuteScalarsGrouped(0, 2, a3)
					t3 := a2.ConcatPermuteScalarsGrouped(1, 3, a3)
					t0.ConcatPermute128Scalars(0, 2, t2).StoreArray((*[4]float64)(unsafe.Add(dp, (c*rows+r)*8)))
					t1.ConcatPermute128Scalars(0, 2, t3).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+1)*rows+r)*8)))
					t0.ConcatPermute128Scalars(1, 3, t2).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+2)*rows+r)*8)))
					t1.ConcatPermute128Scalars(1, 3, t3).StoreArray((*[4]float64)(unsafe.Add(dp, ((c+3)*rows+r)*8)))
				}
			}
		}
	}
	archsimd.ClearAVXUpperBits()
}

const nearTightFiniteCap = 0x1p1020

// ScanNearTightRow64Scalar scans columns j in [start, min(len(rowCost), len(v))) and writes up
// to len(out) column indices where rowCost[j] != +Inf and !((rowCost[j]-ui)-v[j] > limit &&
// (rowCost[j]-ui)-v[j] < 0x1p1020), returning the next unscanned index and the number written.
func ScanNearTightRow64Scalar(rowCost, v []float64, start int, ui, limit float64, out []int) (next, count int) {
	n := min(len(rowCost), len(v))
	if start < 0 {
		start = 0
	}
	if start >= n || len(out) == 0 {
		return min(max(start, 0), n), 0
	}
	cSlice := rowCost[start:n]
	vSlice := v[start:n]
	vSlice = vSlice[:len(cSlice)]
	outSlice := out
	posInf := math.Inf(1)
	cnt := 0
	i := 0
	for ; i <= len(cSlice)-4; i += 4 {
		c0, c1, c2, c3 := cSlice[i], cSlice[i+1], cSlice[i+2], cSlice[i+3]
		d0 := (c0 - ui) - vSlice[i]
		d1 := (c1 - ui) - vSlice[i+1]
		d2 := (c2 - ui) - vSlice[i+2]
		d3 := (c3 - ui) - vSlice[i+3]
		s0 := c0 == posInf || (d0 > limit && d0 < nearTightFiniteCap)
		s1 := c1 == posInf || (d1 > limit && d1 < nearTightFiniteCap)
		s2 := c2 == posInf || (d2 > limit && d2 < nearTightFiniteCap)
		s3 := c3 == posInf || (d3 > limit && d3 < nearTightFiniteCap)
		if !s0 || !s1 || !s2 || !s3 {
			if !s0 {
				if uint(cnt) >= uint(len(outSlice)) {
					return start + i, cnt
				}
				outSlice[cnt] = start + i
				cnt++
			}
			if !s1 {
				if uint(cnt) >= uint(len(outSlice)) {
					return start + i + 1, cnt
				}
				outSlice[cnt] = start + i + 1
				cnt++
			}
			if !s2 {
				if uint(cnt) >= uint(len(outSlice)) {
					return start + i + 2, cnt
				}
				outSlice[cnt] = start + i + 2
				cnt++
			}
			if !s3 {
				if uint(cnt) >= uint(len(outSlice)) {
					return start + i + 3, cnt
				}
				outSlice[cnt] = start + i + 3
				cnt++
			}
		}
	}
	for ; i < len(cSlice); i++ {
		c := cSlice[i]
		if c == posInf {
			continue
		}
		d := (c - ui) - vSlice[i]
		if !(d > limit && d < nearTightFiniteCap) {
			if uint(cnt) >= uint(len(outSlice)) {
				return start + i, cnt
			}
			outSlice[cnt] = start + i
			cnt++
		}
	}
	return n, cnt
}

// ScanNearTightRow64With scans rowCost[start:n] against column duals v[start:n] and row dual ui,
// writing up to len(out) indices j whose reduced cost cannot be proven > limit and finite without
// exact compensation, dispatching to AVX-512 or AVX2 when enabled.
func ScanNearTightRow64With(rowCost, v []float64, start int, ui, limit float64, out []int, disableSIMD bool) (next, count int) {
	n := min(len(rowCost), len(v))
	if start < 0 {
		start = 0
	}
	if start >= n || len(out) == 0 {
		return min(max(start, 0), n), 0
	}
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n-start < minAVX2Len {
		return ScanNearTightRow64Scalar(rowCost[:n], v[:n], start, ui, limit, out)
	}
	var j int
	if tier == tierAVX512 && n-start >= minAVX512Len {
		j, count = scanNearTightRow64AVX512(rowCost[:n], v[:n], start, ui, limit, out)
	} else {
		j, count = scanNearTightRow64AVX2(rowCost[:n], v[:n], start, ui, limit, out)
	}
	if j >= n {
		return j, count
	}
	posInf := math.Inf(1)
	for ; j < n; j++ {
		c := rowCost[j]
		if c == posInf {
			continue
		}
		d := (c - ui) - v[j]
		if !(d > limit && d < nearTightFiniteCap) {
			if count == len(out) {
				return j, count
			}
			out[count] = j
			count++
		}
	}
	return n, count
}

// ScanNearTightRow64 scans rowCost[start:n] for near-tight or potentially overflowing reduced-cost
// columns using AVX-512 or AVX2 when enabled.
func ScanNearTightRow64(rowCost, v []float64, start int, ui, limit float64, out []int) (next, count int) {
	return ScanNearTightRow64With(rowCost, v, start, ui, limit, out, false)
}

//go:noinline
func scanNearTightRow64AVX2(rowCost, v []float64, start int, ui, limit float64, out []int) (int, int) {
	n := len(rowCost)
	cp := unsafe.Pointer(&rowCost[0])
	vp := unsafe.Pointer(&v[0])
	vui := archsimd.BroadcastFloat64x4(ui)
	vlim := archsimd.BroadcastFloat64x4(limit)
	vInf := archsimd.BroadcastFloat64x4(math.Inf(1))
	vCap := archsimd.BroadcastFloat64x4(nearTightFiniteCap)
	capOut := len(out)
	out = out[:capOut]
	count := 0
	j := start
	for ; j+8 <= n; j += 8 {
		vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8)))
		vv0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8)))
		vc1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, (j+4)*8)))
		vv1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, (j+4)*8)))
		vd0 := vc0.Sub(vui).Sub(vv0)
		vd1 := vc1.Sub(vui).Sub(vv1)
		skip0 := vc0.Equal(vInf).Or(vd0.Greater(vlim).And(vd0.Less(vCap)))
		skip1 := vc1.Equal(vInf).Or(vd1.Greater(vlim).And(vd1.Less(vCap)))
		if skip0.And(skip1).ToBits() != 0xF {
			bm := ((^skip0.ToBits()) & 0xF) | (((^skip1.ToBits()) & 0xF) << 4)
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
	}
	for ; j+4 <= n; j += 4 {
		vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8)))
		vv0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8)))
		vd0 := vc0.Sub(vui).Sub(vv0)
		skip0 := vc0.Equal(vInf).Or(vd0.Greater(vlim).And(vd0.Less(vCap)))
		bm := (^skip0.ToBits()) & 0xF
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

// ScanNearTightMatrix64Scalar scans all m rows of the row-major m x n matrix cost against
// row duals u[:m], column duals v[:n], per-row limits rowLimits[:m], and matched columns
// col4row[:m]. For every cell (i, j) with j != col4row[i] where !((cost[i*n+j]-u[i])-v[j] > rowLimits[i]),
// it appends (i, j) to (outRow, outCol). If output capacity is exceeded, it returns (count, true).
func ScanNearTightMatrix64Scalar(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int) (count int, overflow bool) {
	if m <= 0 || n <= 0 {
		return 0, false
	}
	if len(cost) < m*n || len(u) < m || len(v) < n || len(rowLimits) < m || len(col4row) < m {
		return 0, true
	}
	capOut := min(len(outRow), len(outCol))
	outRow = outRow[:capOut]
	outCol = outCol[:len(outRow)]
	u = u[:m]
	v = v[:n]
	rowLimits = rowLimits[:m]
	col4row = col4row[:m]
	cnt := 0
	for i := range m {
		ui := u[i]
		lim := rowLimits[i]
		mj := col4row[i]
		rOff := i * n
		// Tie vSlice to len(rSlice) and use j <= len(rSlice)-4 so Go SSA prove eliminates loop BCE.
		rSlice := cost[rOff : rOff+n]
		vSlice := v[:len(rSlice)]
		j := 0
		for ; j <= len(rSlice)-4; j += 4 {
			d0 := (rSlice[j] - ui) - vSlice[j]
			d1 := (rSlice[j+1] - ui) - vSlice[j+1]
			d2 := (rSlice[j+2] - ui) - vSlice[j+2]
			d3 := (rSlice[j+3] - ui) - vSlice[j+3]
			if !(d0 > lim) || !(d1 > lim) || !(d2 > lim) || !(d3 > lim) {
				if !(d0 > lim) && j != mj {
					if uint(cnt) >= uint(len(outRow)) {
						return cnt, true
					}
					outRow[cnt] = i
					outCol[cnt] = j
					cnt++
				}
				if !(d1 > lim) && j+1 != mj {
					if uint(cnt) >= uint(len(outRow)) {
						return cnt, true
					}
					outRow[cnt] = i
					outCol[cnt] = j + 1
					cnt++
				}
				if !(d2 > lim) && j+2 != mj {
					if uint(cnt) >= uint(len(outRow)) {
						return cnt, true
					}
					outRow[cnt] = i
					outCol[cnt] = j + 2
					cnt++
				}
				if !(d3 > lim) && j+3 != mj {
					if uint(cnt) >= uint(len(outRow)) {
						return cnt, true
					}
					outRow[cnt] = i
					outCol[cnt] = j + 3
					cnt++
				}
			}
		}
		for ; j < len(rSlice); j++ {
			if !((rSlice[j]-ui)-vSlice[j] > lim) && j != mj {
				if uint(cnt) >= uint(len(outRow)) {
					return cnt, true
				}
				outRow[cnt] = i
				outCol[cnt] = j
				cnt++
			}
		}
	}
	return cnt, false
}

// ScanNearTightMatrix64With scans an m x n cost matrix for non-matched cells whose reduced cost
// cannot be proven > rowLimits[i], dispatching across the entire matrix with one VZEROUPPER.
func ScanNearTightMatrix64With(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int, disableSIMD bool) (count int, overflow bool) {
	if m <= 0 || n <= 0 {
		return 0, false
	}
	if len(cost) < m*n || len(u) < m || len(v) < n || len(rowLimits) < m || len(col4row) < m {
		return 0, true
	}
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < 8 {
		return ScanNearTightMatrix64Scalar(m, n, cost, u, v, rowLimits, col4row, outRow, outCol)
	}
	capOut := min(len(outRow), len(outCol))
	outRow = outRow[:capOut]
	outCol = outCol[:capOut]
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		count, overflow = scanNearTightMatrix64AVX512(m, n, cost, u, v, rowLimits, col4row, outRow, outCol)
	} else {
		vecEnd = n &^ 3
		count, overflow = scanNearTightMatrix64AVX2(m, n, cost, u, v, rowLimits, col4row, outRow, outCol)
	}
	if overflow {
		return count, true
	}
	if vecEnd < n {
		for i := range m {
			ui := u[i]
			lim := rowLimits[i]
			mj := col4row[i]
			rowOff := i * n
			for j := vecEnd; j < n; j++ {
				if !((cost[rowOff+j]-ui)-v[j] > lim) && j != mj {
					if uint(count) >= uint(capOut) {
						return count, true
					}
					outRow[count] = i
					outCol[count] = j
					count++
				}
			}
		}
	}
	return count, false
}

// ScanNearTightMatrix64 scans an m x n cost matrix for near-tight non-matched cells using AVX-512 or AVX2.
func ScanNearTightMatrix64(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int) (count int, overflow bool) {
	return ScanNearTightMatrix64With(m, n, cost, u, v, rowLimits, col4row, outRow, outCol, false)
}

//go:noinline
func scanNearTightMatrix64AVX2(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int) (int, bool) {
	vecEnd := n &^ 3
	cpBase := unsafe.Pointer(&cost[0])
	vp := unsafe.Pointer(&v[0])
	up := unsafe.Pointer(&u[0])
	lp := unsafe.Pointer(&rowLimits[0])
	mp := unsafe.Pointer(&col4row[0])
	capOut := len(outRow)
	outRow = outRow[:capOut]
	outCol = outCol[:capOut]
	rowStride := uintptr(n) * 8
	count := 0
	for i := range m {
		cp := unsafe.Add(cpBase, uintptr(i)*rowStride)
		vui := archsimd.BroadcastUint64x4(*(*uint64)(unsafe.Add(up, uintptr(i)*8))).BitsToFloat64()
		vlim := archsimd.BroadcastUint64x4(*(*uint64)(unsafe.Add(lp, uintptr(i)*8))).BitsToFloat64()
		mj := *(*int)(unsafe.Add(mp, uintptr(i)*8))
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8)))
			vv0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8)))
			vc1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, (j+4)*8)))
			vv1 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, (j+4)*8)))
			ok0 := vc0.Sub(vui).Sub(vv0).Greater(vlim)
			ok1 := vc1.Sub(vui).Sub(vv1).Greater(vlim)
			if ok0.And(ok1).ToBits() != 0xF {
				bm := ((^ok0.ToBits()) & 0xF) | (((^ok1.ToBits()) & 0xF) << 4)
				for bm != 0 {
					k := bits.TrailingZeros8(bm)
					col := j + k
					if col != mj {
						if uint(count) >= uint(capOut) {
							archsimd.ClearAVXUpperBits()
							return count, true
						}
						outRow[count] = i
						outCol[count] = col
						count++
					}
					bm &= bm - 1
				}
			}
		}
		for ; j+4 <= vecEnd; j += 4 {
			vc0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(cp, j*8)))
			vv0 := archsimd.LoadFloat64x4Array((*[4]float64)(unsafe.Add(vp, j*8)))
			bm := (^vc0.Sub(vui).Sub(vv0).Greater(vlim).ToBits()) & 0xF
			for bm != 0 {
				k := bits.TrailingZeros8(bm)
				col := j + k
				if col != mj {
					if uint(count) >= uint(capOut) {
						archsimd.ClearAVXUpperBits()
						return count, true
					}
					outRow[count] = i
					outCol[count] = col
					count++
				}
				bm &= bm - 1
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return count, false
}

// AllAbsLECheckInt64Scalar reports whether every element x[i] satisfies |x[i]| <= limit and
// simultaneously whether every element is an exact integer with |x[i]| <= intLim.
// Keep i <= len(x)-4 so Go SSA prove eliminates loop BCE.
func AllAbsLECheckInt64Scalar(x []float64, limit, intLim float64) (valid, allInt bool) {
	if !(0 <= limit) {
		return false, false
	}
	allInt = 0 <= intLim
	i := 0
	for ; i <= len(x)-4; i += 4 {
		c0, c1, c2, c3 := x[i], x[i+1], x[i+2], x[i+3]
		a0, a1, a2, a3 := math.Abs(c0), math.Abs(c1), math.Abs(c2), math.Abs(c3)
		if !(a0 <= limit && a1 <= limit && a2 <= limit && a3 <= limit) {
			return false, false
		}
		if allInt && !(a0 <= intLim && math.Trunc(c0) == c0 &&
			a1 <= intLim && math.Trunc(c1) == c1 &&
			a2 <= intLim && math.Trunc(c2) == c2 &&
			a3 <= intLim && math.Trunc(c3) == c3) {
			allInt = false
		}
	}
	for ; i < len(x); i++ {
		c := x[i]
		a := math.Abs(c)
		if !(a <= limit) {
			return false, false
		}
		if allInt && !(a <= intLim && math.Trunc(c) == c) {
			allInt = false
		}
	}
	return true, allInt
}

// AllAbsLECheckInt64With reports whether every element of x satisfies |x[i]| <= limit and whether
// all entries are exact integers with |x[i]| <= intLim, dispatching to AVX-512 or AVX2 when enabled.
func AllAbsLECheckInt64With(x []float64, limit, intLim float64, disableSIMD bool) (valid, allInt bool) {
	tier := currentTier(disableSIMD)
	n := len(x)
	if tier == tierScalar || n < minAVX2Len {
		return AllAbsLECheckInt64Scalar(x, limit, intLim)
	}
	if !(0 <= limit) {
		return false, false
	}
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		valid, allInt = allAbsLECheckInt64AVX512(x[:vecEnd], limit, intLim)
	} else {
		vecEnd = n &^ 3
		valid, allInt = allAbsLECheckInt64AVX2(x[:vecEnd], limit, intLim)
	}
	if !valid {
		return false, false
	}
	for i := vecEnd; i < n; i++ {
		c := x[i]
		a := math.Abs(c)
		if !(a <= limit) {
			return false, false
		}
		if allInt && !(a <= intLim && math.Trunc(c) == c) {
			allInt = false
		}
	}
	return true, allInt
}

//go:noinline
func allAbsLECheckInt64AVX2(x []float64, limit, intLim float64) (bool, bool) {
	n := len(x)
	xp := unsafe.Pointer(&x[0])
	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	vZero := signMask.Xor(signMask).BitsToFloat64()
	vlim := archsimd.BroadcastFloat64x4(limit)
	vIntLim := archsimd.BroadcastFloat64x4(intLim)
	ok0 := vlim.GreaterEqual(vZero)
	ok1 := ok0
	int0 := vIntLim.GreaterEqual(vZero)
	int1 := int0
	i := 0
	for ; i+8 <= n; i += 8 {
		p0 := unsafe.Add(xp, i*8)
		r0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0))
		r1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32)))
		v0 := r0.BitsToFloat64()
		v1 := r1.BitsToFloat64()
		a0 := r0.And(signMask).BitsToFloat64()
		a1 := r1.And(signMask).BitsToFloat64()
		ok0 = ok0.And(a0.LessEqual(vlim))
		ok1 = ok1.And(a1.LessEqual(vlim))
		int0 = int0.And(a0.LessEqual(vIntLim)).And(v0.Equal(v0.Trunc()))
		int1 = int1.And(a1.LessEqual(vIntLim)).And(v1.Equal(v1.Trunc()))
	}
	for ; i+4 <= n; i += 4 {
		r0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(xp, i*8)))
		v0 := r0.BitsToFloat64()
		a0 := r0.And(signMask).BitsToFloat64()
		ok0 = ok0.And(a0.LessEqual(vlim))
		int0 = int0.And(a0.LessEqual(vIntLim)).And(v0.Equal(v0.Trunc()))
	}
	valid := ok0.And(ok1).ToBits() == 0xf
	allInt := valid && int0.And(int1).ToBits() == 0xf
	archsimd.ClearAVXUpperBits()
	return valid, allInt
}

// ValidateAndNegateCheckInt64Scalar writes dst[i] = 0.0 - src[i] for every index i and reports
// whether all entries satisfy |src[i]| <= limit and whether all entries are exact integers <= intLim.
// Keep dSlice tied to len(sSlice) and use i <= len(sSlice)-4 so Go SSA prove eliminates loop BCE.
func ValidateAndNegateCheckInt64Scalar(dst, src []float64, limit, intLim float64) (valid, allInt bool) {
	n := min(len(dst), len(src))
	if n == 0 {
		v := !(limit < 0 || math.IsNaN(limit))
		return v, v && 0 <= intLim
	}
	sSlice := src[:n]
	dSlice := dst[:len(sSlice)]
	valid = 0 <= limit
	allInt = valid && 0 <= intLim
	i := 0
	for ; i <= len(sSlice)-4; i += 4 {
		c0, c1, c2, c3 := sSlice[i], sSlice[i+1], sSlice[i+2], sSlice[i+3]
		a0, a1, a2, a3 := math.Abs(c0), math.Abs(c1), math.Abs(c2), math.Abs(c3)
		if !(a0 <= limit && a1 <= limit && a2 <= limit && a3 <= limit) {
			valid = false
			allInt = false
		} else if allInt && !(a0 <= intLim && math.Trunc(c0) == c0 &&
			a1 <= intLim && math.Trunc(c1) == c1 &&
			a2 <= intLim && math.Trunc(c2) == c2 &&
			a3 <= intLim && math.Trunc(c3) == c3) {
			allInt = false
		}
		dSlice[i] = 0.0 - c0
		dSlice[i+1] = 0.0 - c1
		dSlice[i+2] = 0.0 - c2
		dSlice[i+3] = 0.0 - c3
	}
	for ; i < len(sSlice); i++ {
		c := sSlice[i]
		a := math.Abs(c)
		if !(a <= limit) {
			valid = false
			allInt = false
		} else if allInt && !(a <= intLim && math.Trunc(c) == c) {
			allInt = false
		}
		dSlice[i] = 0.0 - c
	}
	return valid, allInt
}

// ValidateAndNegateCheckInt64With writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy
// |src[i]| <= limit and whether all entries are exact integers <= intLim, dispatching to AVX-512 or AVX2.
func ValidateAndNegateCheckInt64With(dst, src []float64, limit, intLim float64, disableSIMD bool) (valid, allInt bool) {
	n := min(len(dst), len(src))
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < minAVX2Len {
		return ValidateAndNegateCheckInt64Scalar(dst, src, limit, intLim)
	}
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		vecEnd = n &^ 7
		valid, allInt = validateAndNegateCheckInt64AVX512(dst[:vecEnd], src[:vecEnd], limit, intLim)
	} else {
		vecEnd = n &^ 3
		valid, allInt = validateAndNegateCheckInt64AVX2(dst[:vecEnd], src[:vecEnd], limit, intLim)
	}
	for i := vecEnd; i < n; i++ {
		c := src[i]
		a := math.Abs(c)
		if !(a <= limit) {
			valid = false
			allInt = false
		} else if allInt && !(a <= intLim && math.Trunc(c) == c) {
			allInt = false
		}
		dst[i] = 0.0 - c
	}
	return valid, allInt
}

//go:noinline
func validateAndNegateCheckInt64AVX2(dst, src []float64, limit, intLim float64) (bool, bool) {
	n := len(src)
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])

	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	vZero := signMask.Xor(signMask).BitsToFloat64()
	vlim := archsimd.BroadcastFloat64x4(limit)
	vIntLim := archsimd.BroadcastFloat64x4(intLim)
	ok0 := vlim.GreaterEqual(vZero)
	ok1 := ok0
	int0 := vIntLim.GreaterEqual(vZero)
	int1 := int0

	i := 0
	for ; i+8 <= n; i += 8 {
		p0 := unsafe.Add(sp, i*8)
		r0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0))
		r1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32)))
		v0 := r0.BitsToFloat64()
		v1 := r1.BitsToFloat64()
		a0 := r0.And(signMask).BitsToFloat64()
		a1 := r1.And(signMask).BitsToFloat64()
		ok0 = ok0.And(a0.LessEqual(vlim))
		ok1 = ok1.And(a1.LessEqual(vlim))
		int0 = int0.And(a0.LessEqual(vIntLim)).And(v0.Equal(v0.Trunc()))
		int1 = int1.And(a1.LessEqual(vIntLim)).And(v1.Equal(v1.Trunc()))
		vZero.Sub(v0).StoreArray((*[4]float64)(unsafe.Add(dp, i*8)))
		vZero.Sub(v1).StoreArray((*[4]float64)(unsafe.Add(dp, (i+4)*8)))
	}
	for ; i+4 <= n; i += 4 {
		r0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(sp, i*8)))
		v0 := r0.BitsToFloat64()
		a0 := r0.And(signMask).BitsToFloat64()
		ok0 = ok0.And(a0.LessEqual(vlim))
		int0 = int0.And(a0.LessEqual(vIntLim)).And(v0.Equal(v0.Trunc()))
		vZero.Sub(v0).StoreArray((*[4]float64)(unsafe.Add(dp, i*8)))
	}

	valid := ok0.And(ok1).ToBits() == 0xf
	allInt := valid && int0.And(int1).ToBits() == 0xf
	archsimd.ClearAVXUpperBits()
	return valid, allInt
}

// ColReduceValidateCheckIntRows64Scalar validates |cost| <= limit, checks whether all entries are
// exact integers with |cost| <= intLim, and performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceValidateCheckIntRows64Scalar(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit, intLim float64) (valid, allInt bool) {
	if n <= 0 || rowEnd <= rowStart {
		ok := !(limit < 0 || math.IsNaN(limit))
		return ok, ok && 0 <= intLim
	}
	if rowStart < 0 || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n || !(0 <= limit) {
		return false, false
	}
	v = v[:n]
	row4col = row4col[:n]
	allInt = 0 <= intLim
	i := rowStart
	if i == 0 {
		r0 := cost[:n]
		for j, c := range r0 {
			a := math.Abs(c)
			if !(a <= limit) {
				return false, false
			}
			if allInt && !(a <= intLim && math.Trunc(c) == c) {
				allInt = false
			}
			v[j] = c
			row4col[j] = 0
		}
		i = 1
	}
	for ; i < rowEnd; i++ {
		rOff := i * n
		// Keep vSlice and r4c tied to len(rSlice) with j <= len(rSlice)-4 so Go SSA prove eliminates loop BCE.
		rSlice := cost[rOff : rOff+n]
		vSlice := v[:len(rSlice)]
		r4c := row4col[:len(rSlice)]
		j := 0
		for ; j <= len(rSlice)-4; j += 4 {
			c0, c1, c2, c3 := rSlice[j], rSlice[j+1], rSlice[j+2], rSlice[j+3]
			a0, a1, a2, a3 := math.Abs(c0), math.Abs(c1), math.Abs(c2), math.Abs(c3)
			if !(a0 <= limit && a1 <= limit && a2 <= limit && a3 <= limit) {
				return false, false
			}
			if allInt && !(a0 <= intLim && math.Trunc(c0) == c0 &&
				a1 <= intLim && math.Trunc(c1) == c1 &&
				a2 <= intLim && math.Trunc(c2) == c2 &&
				a3 <= intLim && math.Trunc(c3) == c3) {
				allInt = false
			}
			v0, v1, v2, v3 := vSlice[j], vSlice[j+1], vSlice[j+2], vSlice[j+3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[j] = c0
					r4c[j] = i
				}
				if c1 < v1 {
					vSlice[j+1] = c1
					r4c[j+1] = i
				}
				if c2 < v2 {
					vSlice[j+2] = c2
					r4c[j+2] = i
				}
				if c3 < v3 {
					vSlice[j+3] = c3
					r4c[j+3] = i
				}
			}
		}
		for ; j < len(rSlice); j++ {
			c := rSlice[j]
			a := math.Abs(c)
			if !(a <= limit) {
				return false, false
			}
			if allInt && !(a <= intLim && math.Trunc(c) == c) {
				allInt = false
			}
			if c < vSlice[j] {
				vSlice[j] = c
				r4c[j] = i
			}
		}
	}
	return true, allInt
}

// ColReduceValidateCheckIntRows64With validates |cost| <= limit, checks exact-integer membership
// within intLim, and performs row-major column reduction over rows [rowStart, rowEnd), honoring disableSIMD.
func ColReduceValidateCheckIntRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit, intLim float64, disableSIMD bool) (valid, allInt bool) {
	tier := currentTier(disableSIMD)
	if tier == tierScalar || n < minAVX2Len || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n || !(0 <= limit) {
		return ColReduceValidateCheckIntRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit, intLim)
	}
	v = v[:n]
	row4col = row4col[:n]
	if rowStart == 0 {
		clear(row4col)
	}
	var vecEnd int
	if tier == tierAVX512 && n >= minAVX512Len {
		valid, allInt = colReduceValidateCheckIntRows64AVX512(n, rowStart, rowEnd, cost, v, row4col, limit, intLim)
		if !valid {
			return false, false
		}
		vecEnd = n &^ 7
	} else {
		valid, allInt = colReduceValidateCheckIntRows64AVX2(n, rowStart, rowEnd, cost, v, row4col, limit, intLim)
		if !valid {
			return false, false
		}
		vecEnd = n &^ 3
	}
	if vecEnd < n {
		r := rowStart
		if r == 0 {
			for j := vecEnd; j < n; j++ {
				c := cost[j]
				a := math.Abs(c)
				if !(a <= limit) {
					return false, false
				}
				if allInt && !(a <= intLim && math.Trunc(c) == c) {
					allInt = false
				}
				v[j] = c
			}
			r = 1
		}
		for ; r < rowEnd; r++ {
			rowOff := r * n
			for j := vecEnd; j < n; j++ {
				c := cost[rowOff+j]
				a := math.Abs(c)
				if !(a <= limit) {
					return false, false
				}
				if allInt && !(a <= intLim && math.Trunc(c) == c) {
					allInt = false
				}
				if c < v[j] {
					v[j] = c
					row4col[j] = r
				}
			}
		}
	}
	return true, allInt
}

//go:noinline
func colReduceValidateCheckIntRows64AVX2(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit, intLim float64) (bool, bool) {
	vecEnd := n &^ 3
	vp := unsafe.Pointer(&v[0])
	rp := unsafe.Pointer(&row4col[0])

	signMask := archsimd.BroadcastUint64x4(0x7fffffffffffffff)
	vZero := signMask.Xor(signMask).BitsToFloat64()
	vlim := archsimd.BroadcastFloat64x4(limit)
	vIntLim := archsimd.BroadcastFloat64x4(intLim)
	ok0 := vlim.GreaterEqual(vZero)
	ok1 := ok0
	int0 := vIntLim.GreaterEqual(vZero)
	int1 := int0

	i := rowStart
	if i == 0 {
		r0p := unsafe.Pointer(&cost[0])
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			p0 := unsafe.Add(r0p, j*8)
			r0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0))
			r1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32)))
			vc0 := r0.BitsToFloat64()
			vc1 := r1.BitsToFloat64()
			a0 := r0.And(signMask).BitsToFloat64()
			a1 := r1.And(signMask).BitsToFloat64()
			ok0 = ok0.And(a0.LessEqual(vlim))
			ok1 = ok1.And(a1.LessEqual(vlim))
			int0 = int0.And(a0.LessEqual(vIntLim)).And(vc0.Equal(vc0.Trunc()))
			int1 = int1.And(a1.LessEqual(vIntLim)).And(vc1.Equal(vc1.Trunc()))
			vc0.StoreArray((*[4]float64)(unsafe.Add(vp, j*8)))
			vc1.StoreArray((*[4]float64)(unsafe.Add(vp, (j+4)*8)))
		}
		for ; j+4 <= vecEnd; j += 4 {
			r0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(r0p, j*8)))
			vc0 := r0.BitsToFloat64()
			a0 := r0.And(signMask).BitsToFloat64()
			ok0 = ok0.And(a0.LessEqual(vlim))
			int0 = int0.And(a0.LessEqual(vIntLim)).And(vc0.Equal(vc0.Trunc()))
			vc0.StoreArray((*[4]float64)(unsafe.Add(vp, j*8)))
		}
		i = 1
	}

	for ; i < rowEnd; i++ {
		rowPtr := unsafe.Pointer(&cost[i*n])
		vrow := archsimd.BroadcastInt64x4(int64(i))
		j := 0
		for ; j+8 <= vecEnd; j += 8 {
			p0 := unsafe.Add(rowPtr, j*8)
			r0 := archsimd.LoadUint64x4Array((*[4]uint64)(p0))
			r1 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(p0, 32)))
			vc0 := r0.BitsToFloat64()
			vc1 := r1.BitsToFloat64()
			a0 := r0.And(signMask).BitsToFloat64()
			a1 := r1.And(signMask).BitsToFloat64()
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv1Ptr := (*[4]float64)(unsafe.Add(vp, (j+4)*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			vv1 := archsimd.LoadFloat64x4Array(vv1Ptr)
			ok0 = ok0.And(a0.LessEqual(vlim))
			ok1 = ok1.And(a1.LessEqual(vlim))
			int0 = int0.And(a0.LessEqual(vIntLim)).And(vc0.Equal(vc0.Trunc()))
			int1 = int1.And(a1.LessEqual(vIntLim)).And(vc1.Equal(vc1.Trunc()))
			lt0 := vc0.Less(vv0)
			lt1 := vc1.Less(vv1)
			if lt0.Or(lt1).ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				vc1.IfElse(lt1, vv1).StoreArray(vv1Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				r1Ptr := (*[4]int64)(unsafe.Add(rp, (j+4)*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
				vrow.IfElse(lt1, archsimd.LoadInt64x4Array(r1Ptr)).StoreArray(r1Ptr)
			}
		}
		for ; j+4 <= vecEnd; j += 4 {
			r0 := archsimd.LoadUint64x4Array((*[4]uint64)(unsafe.Add(rowPtr, j*8)))
			vc0 := r0.BitsToFloat64()
			a0 := r0.And(signMask).BitsToFloat64()
			vv0Ptr := (*[4]float64)(unsafe.Add(vp, j*8))
			vv0 := archsimd.LoadFloat64x4Array(vv0Ptr)
			ok0 = ok0.And(a0.LessEqual(vlim))
			int0 = int0.And(a0.LessEqual(vIntLim)).And(vc0.Equal(vc0.Trunc()))
			lt0 := vc0.Less(vv0)
			if lt0.ToBits() != 0 {
				vc0.IfElse(lt0, vv0).StoreArray(vv0Ptr)
				r0Ptr := (*[4]int64)(unsafe.Add(rp, j*8))
				vrow.IfElse(lt0, archsimd.LoadInt64x4Array(r0Ptr)).StoreArray(r0Ptr)
			}
		}
	}

	valid := ok0.And(ok1).ToBits() == 0xf
	allInt := valid && int0.And(int1).ToBits() == 0xf
	archsimd.ClearAVXUpperBits()
	return valid, allInt
}
