package simd

import "math"

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
	// Keep sub-slices tied to len(cSlice) and use j <= len(cSlice)-4 so Go SSA prove eliminates loop BCE.
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

// MinNoNaN64Scalar returns the minimum non-NaN value in x, or +Inf if x is empty or all-NaN.
// Keep the i <= len(x)-4 bound form so Go SSA prove eliminates loop bounds checks.
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

// ValidateAndNegate64Scalar writes dst[i] = 0.0 - src[i] for every index i
// and returns true if every element of src is finite with |src[i]| <= limit.
// Aliasing dst and src identically is safe because all n elements are always written.
// Keep dSlice tied to len(sSlice) and use i <= len(sSlice)-4 so Go SSA prove eliminates loop BCE.
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
		// Keep vSlice and r4c tied to len(rSlice) with j <= len(rSlice)-4 so Go SSA prove eliminates loop BCE.
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
		// Keep vSlice and r4c tied to len(rSlice) with j <= len(rSlice)-4 so Go SSA prove eliminates loop BCE.
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
