//go:build !(goexperiment.simd && amd64)

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

// RelaxRow64With relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64With(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64, _ bool) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
}

// RelaxRow64 relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
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
	return res
}

// MinNoNaN64With returns the minimum non-NaN value in x.
func MinNoNaN64With(x []float64, _ bool) float64 {
	return MinNoNaN64Scalar(x)
}

// MinNoNaN64 returns the minimum non-NaN value in x.
func MinNoNaN64(x []float64) float64 {
	return MinNoNaN64Scalar(x)
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

// CollectLE64With scans x[start:len(x)] for entries <= limit.
func CollectLE64With(x []float64, start int, limit float64, out []int, _ bool) (next, count int) {
	return CollectLE64Scalar(x, start, limit, out)
}

// CollectLE64 scans x[start:len(x)] for entries <= limit.
func CollectLE64(x []float64, start int, limit float64, out []int) (next, count int) {
	return CollectLE64Scalar(x, start, limit, out)
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

// FillIntWith sets every element of dst to val.
func FillIntWith(dst []int, val int, _ bool) {
	FillIntScalar(dst, val)
}

// FillInt sets every element of dst to val.
func FillInt(dst []int, val int) {
	FillIntScalar(dst, val)
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

// MinReducedSkip64With returns min_{j != skip} (rowCost[j] - v[j]).
func MinReducedSkip64With(rowCost, v []float64, skip int, _ bool) float64 {
	return MinReducedSkip64Scalar(rowCost, v, skip)
}

// MinReducedSkip64 returns min_{j != skip} (rowCost[j] - v[j]).
func MinReducedSkip64(rowCost, v []float64, skip int) float64 {
	return MinReducedSkip64Scalar(rowCost, v, skip)
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

// ValidateAndNegate64With writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy |src[i]| <= limit.
func ValidateAndNegate64With(dst, src []float64, limit float64, _ bool) bool {
	return ValidateAndNegate64Scalar(dst, src, limit)
}

// ValidateAndNegate64 writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy |src[i]| <= limit.
func ValidateAndNegate64(dst, src []float64, limit float64) bool {
	return ValidateAndNegate64Scalar(dst, src, limit)
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

// ColReduceValidateRows64With validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd).
func ColReduceValidateRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64, _ bool) bool {
	return ColReduceValidateRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit)
}

// ColReduceValidateRows64 validates |cost| <= limit and performs row-major column reduction
// over rows [rowStart, rowEnd).
func ColReduceValidateRows64(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit float64) bool {
	return ColReduceValidateRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit)
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

// ColReduceRows64With performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, _ bool) {
	ColReduceRows64Scalar(n, rowStart, rowEnd, cost, v, row4col)
}

// ColReduceRows64 performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceRows64(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	ColReduceRows64Scalar(n, rowStart, rowEnd, cost, v, row4col)
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

// Transpose64With transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64With(dst, src []float64, rows, cols int, negate, _ bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}

// Transpose64 transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64(dst, src []float64, rows, cols int, negate bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}
