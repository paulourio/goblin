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

// RelaxRow64With relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64With(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64, _ bool) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
}

// RelaxRow64 relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
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
	capOut := len(out)
	j := start
	for ; j+4 <= n; j += 4 {
		if x[j] <= limit || x[j+1] <= limit || x[j+2] <= limit || x[j+3] <= limit {
			for k := range 4 {
				if x[j+k] <= limit {
					if count == capOut {
						return j + k, count
					}
					out[count] = j + k
					count++
				}
			}
		}
	}
	for ; j < n; j++ {
		if x[j] <= limit {
			if count == capOut {
				return j, count
			}
			out[count] = j
			count++
		}
	}
	return n, count
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

// Transpose64With transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64With(dst, src []float64, rows, cols int, negate, _ bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}

// Transpose64 transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64(dst, src []float64, rows, cols int, negate bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}
