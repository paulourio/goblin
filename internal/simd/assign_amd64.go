//go:build goexperiment.simd && amd64

package simd

import (
	"math"
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
	cSlice := rCost[start:n]
	vSlice := v[start:n]
	dSlice := d[start:n]
	pSlice := pred[start:n]
	j := start
	for len(cSlice) >= 4 && len(vSlice) >= 4 && len(dSlice) >= 4 && len(pSlice) >= 4 {
		r0 := minVal + ((cSlice[0] - vSlice[0]) - ui)
		r1 := minVal + ((cSlice[1] - vSlice[1]) - ui)
		r2 := minVal + ((cSlice[2] - vSlice[2]) - ui)
		r3 := minVal + ((cSlice[3] - vSlice[3]) - ui)
		if r0 < dSlice[0] || r1 < dSlice[1] || r2 < dSlice[2] || r3 < dSlice[3] {
			if r0 < dSlice[0] {
				if r0 <= minVal {
					return j
				}
				dSlice[0] = r0
				pSlice[0] = rowI
			}
			if r1 < dSlice[1] {
				if r1 <= minVal {
					return j + 1
				}
				dSlice[1] = r1
				pSlice[1] = rowI
			}
			if r2 < dSlice[2] {
				if r2 <= minVal {
					return j + 2
				}
				dSlice[2] = r2
				pSlice[2] = rowI
			}
			if r3 < dSlice[3] {
				if r3 <= minVal {
					return j + 3
				}
				dSlice[3] = r3
				pSlice[3] = rowI
			}
		}
		cSlice = cSlice[4:]
		vSlice = vSlice[4:]
		dSlice = dSlice[4:]
		pSlice = pSlice[4:]
		j += 4
	}
	for k, cj := range cSlice {
		r := minVal + ((cj - vSlice[k]) - ui)
		if r < dSlice[k] {
			if r <= minVal {
				return j + k
			}
			dSlice[k] = r
			pSlice[k] = rowI
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
	if disableSIMD || simdOff() || n-start < 4 {
		return RelaxRow64Scalar(rCost[:n], v[:n], d[:n], pred[:n], start, rowI, ui, minVal)
	}
	j := relaxRow64AVX2(rCost[:n], v[:n], d[:n], pred[:n], start, rowI, ui, minVal)
	if j+4 <= n {
		return j
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

// RelaxRow64 relaxes contiguous distances d[start:n] for row rowI using AVX2 when enabled.
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
	for ; j+4 <= n; j += 4 {
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
func MinNoNaN64Scalar(x []float64) float64 {
	res := math.Inf(1)
	for len(x) >= 4 {
		v0, v1, v2, v3 := x[0], x[1], x[2], x[3]
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
		x = x[4:]
	}
	for _, v := range x {
		if v < res {
			res = v
		}
	}
	return res
}

// MinNoNaN64With returns the minimum non-NaN value in x, honoring a per-call disableSIMD flag.
func MinNoNaN64With(x []float64, disableSIMD bool) float64 {
	if disableSIMD || simdOff() || len(x) < minAVX2Len {
		return MinNoNaN64Scalar(x)
	}
	return minNoNaN64AVX2(x)
}

// MinNoNaN64 returns the minimum non-NaN value in x using AVX2 when enabled.
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
	return res
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
	if disableSIMD || simdOff() || n < minAVX2Len {
		return MinReducedSkip64Scalar(rowCost, v, skip)
	}
	return minReducedSkip64AVX2(rowCost[:n], v[:n], skip)
}

// MinReducedSkip64 returns min_{j != skip} (rowCost[j] - v[j]) using AVX2 when enabled.
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
	return minRed
}

// ValidateAndNegate64Scalar writes dst[i] = 0.0 - src[i] for every index i
// and returns true if every element of src is finite with |src[i]| <= limit.
// Aliasing dst and src identically is safe because all n elements are always written.
func ValidateAndNegate64Scalar(dst, src []float64, limit float64) bool {
	n := min(len(dst), len(src))
	if n == 0 {
		return !(limit < 0 || math.IsNaN(limit))
	}
	dSlice := dst[:n]
	sSlice := src[:n]
	allOK := true
	for len(sSlice) >= 4 && len(dSlice) >= 4 {
		c0, c1, c2, c3 := sSlice[0], sSlice[1], sSlice[2], sSlice[3]
		if !(math.Abs(c0) <= limit &&
			math.Abs(c1) <= limit &&
			math.Abs(c2) <= limit &&
			math.Abs(c3) <= limit) {
			allOK = false
		}
		dSlice[0] = 0.0 - c0
		dSlice[1] = 0.0 - c1
		dSlice[2] = 0.0 - c2
		dSlice[3] = 0.0 - c3
		sSlice = sSlice[4:]
		dSlice = dSlice[4:]
	}
	for i, c := range sSlice {
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
	if disableSIMD || simdOff() || n < minAVX2Len {
		return ValidateAndNegate64Scalar(dst, src, limit)
	}
	vecEnd := n &^ 3
	allOK := validateAndNegate64AVX2(dst[:vecEnd], src[:vecEnd], limit)
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
		rSlice := cost[i*n:][:n]
		vSlice := v
		r4cSlice := row4col
		for len(rSlice) >= 4 && len(vSlice) >= 4 && len(r4cSlice) >= 4 {
			c0, c1, c2, c3 := rSlice[0], rSlice[1], rSlice[2], rSlice[3]
			if !(math.Abs(c0) <= limit &&
				math.Abs(c1) <= limit &&
				math.Abs(c2) <= limit &&
				math.Abs(c3) <= limit) {
				return false
			}
			v0, v1, v2, v3 := vSlice[0], vSlice[1], vSlice[2], vSlice[3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[0] = c0
					r4cSlice[0] = i
				}
				if c1 < v1 {
					vSlice[1] = c1
					r4cSlice[1] = i
				}
				if c2 < v2 {
					vSlice[2] = c2
					r4cSlice[2] = i
				}
				if c3 < v3 {
					vSlice[3] = c3
					r4cSlice[3] = i
				}
			}
			rSlice = rSlice[4:]
			vSlice = vSlice[4:]
			r4cSlice = r4cSlice[4:]
		}
		if len(rSlice) > 0 {
			vSlice = vSlice[:len(rSlice)]
			r4cSlice = r4cSlice[:len(rSlice)]
			for k, c := range rSlice {
				if !(math.Abs(c) <= limit) {
					return false
				}
				if c < vSlice[k] {
					vSlice[k] = c
					r4cSlice[k] = i
				}
			}
		}
	}
	return true
}

// ColReduceValidateRows64With validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd), honoring disableSIMD.
func ColReduceValidateRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64, disableSIMD bool) bool {
	if disableSIMD || simdOff() || n < minAVX2Len || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
		return ColReduceValidateRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit)
	}
	v = v[:n]
	row4col = row4col[:n]
	if rowStart == 0 {
		clear(row4col)
	}
	if !colReduceValidateRows64AVX2(n, rowStart, rowEnd, cost, v, row4col, limit) {
		return false
	}
	vecEnd := n &^ 3
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
// over rows [rowStart, rowEnd) using AVX2 when enabled.
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
		rSlice := cost[i*n:][:n]
		vSlice := v
		r4cSlice := row4col
		for len(rSlice) >= 4 && len(vSlice) >= 4 && len(r4cSlice) >= 4 {
			c0, c1, c2, c3 := rSlice[0], rSlice[1], rSlice[2], rSlice[3]
			v0, v1, v2, v3 := vSlice[0], vSlice[1], vSlice[2], vSlice[3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[0] = c0
					r4cSlice[0] = i
				}
				if c1 < v1 {
					vSlice[1] = c1
					r4cSlice[1] = i
				}
				if c2 < v2 {
					vSlice[2] = c2
					r4cSlice[2] = i
				}
				if c3 < v3 {
					vSlice[3] = c3
					r4cSlice[3] = i
				}
			}
			rSlice = rSlice[4:]
			vSlice = vSlice[4:]
			r4cSlice = r4cSlice[4:]
		}
		if len(rSlice) > 0 {
			vSlice = vSlice[:len(rSlice)]
			r4cSlice = r4cSlice[:len(rSlice)]
			for k, c := range rSlice {
				if c < vSlice[k] {
					vSlice[k] = c
					r4cSlice[k] = i
				}
			}
		}
	}
}

// ColReduceRows64With performs row-major column reduction over rows [rowStart, rowEnd),
// honoring disableSIMD.
func ColReduceRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, disableSIMD bool) {
	if disableSIMD || simdOff() || n < minAVX2Len || rowStart < 0 || rowEnd <= rowStart || len(v) < n || len(row4col) < n || len(cost) < rowEnd*n {
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
	colReduceRows64AVX2(n, i, rowEnd, cost, v, row4col)
	vecEnd := n &^ 3
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

// ColReduceRows64 performs row-major column reduction over rows [rowStart, rowEnd) using AVX2 when enabled.
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
	if negate {
		for c0 := 0; c0 < cols; c0 += tile {
			cLim := min(cols, c0+tile)
			for r0 := 0; r0 < rows; r0 += tile {
				rLim := min(rows, r0+tile)
				for c := c0; c < cLim; c++ {
					dstRow := dst[c*rows+r0 : c*rows+rLim]
					base := r0*cols + c
					for len(dstRow) >= 4 {
						dstRow[0] = 0.0 - src[base]
						dstRow[1] = 0.0 - src[base+cols]
						dstRow[2] = 0.0 - src[base+2*cols]
						dstRow[3] = 0.0 - src[base+3*cols]
						dstRow = dstRow[4:]
						base += 4 * cols
					}
					for k := range dstRow {
						dstRow[k] = 0.0 - src[base+k*cols]
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
				dstRow := dst[c*rows+r0 : c*rows+rLim]
				base := r0*cols + c
				for len(dstRow) >= 4 {
					dstRow[0] = src[base]
					dstRow[1] = src[base+cols]
					dstRow[2] = src[base+2*cols]
					dstRow[3] = src[base+3*cols]
					dstRow = dstRow[4:]
					base += 4 * cols
				}
				for k := range dstRow {
					dstRow[k] = src[base+k*cols]
				}
			}
		}
	}
}

// Transpose64With transposes a row-major rows-by-cols matrix src into cols-by-rows dst,
// optionally negating entries as 0.0 - val and honoring disableSIMD.
func Transpose64With(dst, src []float64, rows, cols int, negate, disableSIMD bool) {
	if disableSIMD || simdOff() || rows < 8 || cols < 8 {
		Transpose64Scalar(dst, src, rows, cols, negate)
		return
	}
	total := rows * cols
	if len(dst) < total || len(src) < total {
		return
	}
	dst = dst[:total]
	src = src[:total]
	transpose64AVX2(dst, src, rows, cols, negate)

	rows4 := rows &^ 3
	cols4 := cols &^ 3
	if negate {
		for c := cols4; c < cols; c++ {
			dstRow := dst[c*rows : (c+1)*rows]
			for r := range rows {
				dstRow[r] = 0.0 - src[r*cols+c]
			}
		}
		for c := range cols4 {
			dstTail := dst[c*rows+rows4 : (c+1)*rows]
			for k := range dstTail {
				dstTail[k] = 0.0 - src[(rows4+k)*cols+c]
			}
		}
		return
	}
	for c := cols4; c < cols; c++ {
		dstRow := dst[c*rows : (c+1)*rows]
		for r := range rows {
			dstRow[r] = src[r*cols+c]
		}
	}
	for c := range cols4 {
		dstTail := dst[c*rows+rows4 : (c+1)*rows]
		for k := range dstTail {
			dstTail[k] = src[(rows4+k)*cols+c]
		}
	}
}

// Transpose64 transposes a row-major rows-by-cols matrix src into cols-by-rows dst using AVX2 when enabled.
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
