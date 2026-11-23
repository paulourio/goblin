//go:build !(goexperiment.simd && amd64)

package simd

// RelaxRow64With relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64With(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64, _ bool) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
}

// RelaxRow64 relaxes contiguous distances d[start:n] for row rowI.
func RelaxRow64(rCost, v, d []float64, pred []int, start, rowI int, ui, minVal float64) int {
	return RelaxRow64Scalar(rCost, v, d, pred, start, rowI, ui, minVal)
}

// MinNoNaN64With returns the minimum non-NaN value in x.
func MinNoNaN64With(x []float64, _ bool) float64 {
	return MinNoNaN64Scalar(x)
}

// MinNoNaN64 returns the minimum non-NaN value in x.
func MinNoNaN64(x []float64) float64 {
	return MinNoNaN64Scalar(x)
}

// CollectLE64With scans x[start:len(x)] for entries <= limit.
func CollectLE64With(x []float64, start int, limit float64, out []int, _ bool) (next, count int) {
	return CollectLE64Scalar(x, start, limit, out)
}

// CollectLE64 scans x[start:len(x)] for entries <= limit.
func CollectLE64(x []float64, start int, limit float64, out []int) (next, count int) {
	return CollectLE64Scalar(x, start, limit, out)
}

// FillIntWith sets every element of dst to val.
func FillIntWith(dst []int, val int, _ bool) {
	FillIntScalar(dst, val)
}

// FillInt sets every element of dst to val.
func FillInt(dst []int, val int) {
	FillIntScalar(dst, val)
}

// MinReducedSkip64With returns min_{j != skip} (rowCost[j] - v[j]).
func MinReducedSkip64With(rowCost, v []float64, skip int, _ bool) float64 {
	return MinReducedSkip64Scalar(rowCost, v, skip)
}

// MinReducedSkip64 returns min_{j != skip} (rowCost[j] - v[j]).
func MinReducedSkip64(rowCost, v []float64, skip int) float64 {
	return MinReducedSkip64Scalar(rowCost, v, skip)
}

// ValidateAndNegate64With writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy |src[i]| <= limit.
func ValidateAndNegate64With(dst, src []float64, limit float64, _ bool) bool {
	return ValidateAndNegate64Scalar(dst, src, limit)
}

// ValidateAndNegate64 writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy |src[i]| <= limit.
func ValidateAndNegate64(dst, src []float64, limit float64) bool {
	return ValidateAndNegate64Scalar(dst, src, limit)
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

// ColReduceRows64With performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, _ bool) {
	ColReduceRows64Scalar(n, rowStart, rowEnd, cost, v, row4col)
}

// ColReduceRows64 performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceRows64(n, rowStart, rowEnd int, cost, v []float64, row4col []int) {
	ColReduceRows64Scalar(n, rowStart, rowEnd, cost, v, row4col)
}

// Transpose64With transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64With(dst, src []float64, rows, cols int, negate, _ bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}

// Transpose64 transposes a row-major rows-by-cols matrix src into cols-by-rows dst.
func Transpose64(dst, src []float64, rows, cols int, negate bool) {
	Transpose64Scalar(dst, src, rows, cols, negate)
}

// ScanNearTightRow64With scans rowCost[start:n] against column duals v[start:n] and row dual ui,
// writing up to len(out) indices j whose reduced cost cannot be proven > limit and finite.
func ScanNearTightRow64With(rowCost, v []float64, start int, ui, limit float64, out []int, _ bool) (next, count int) {
	return ScanNearTightRow64Scalar(rowCost, v, start, ui, limit, out)
}

// ScanNearTightRow64 scans rowCost[start:n] for near-tight or potentially overflowing reduced-cost columns.
func ScanNearTightRow64(rowCost, v []float64, start int, ui, limit float64, out []int) (next, count int) {
	return ScanNearTightRow64Scalar(rowCost, v, start, ui, limit, out)
}

// ScanNearTightMatrix64With scans an m x n cost matrix for non-matched cells whose reduced cost
// cannot be proven > rowLimits[i] with a single compare per lane.
func ScanNearTightMatrix64With(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int, _ bool) (count int, overflow bool) {
	return ScanNearTightMatrix64Scalar(m, n, cost, u, v, rowLimits, col4row, outRow, outCol)
}

// ScanNearTightMatrix64 scans an m x n cost matrix for near-tight non-matched cells.
func ScanNearTightMatrix64(m, n int, cost, u, v, rowLimits []float64, col4row, outRow, outCol []int) (count int, overflow bool) {
	return ScanNearTightMatrix64Scalar(m, n, cost, u, v, rowLimits, col4row, outRow, outCol)
}

// AllAbsLECheckInt64With reports whether every element of x satisfies |x[i]| <= limit and whether
// all entries are exact integers with |x[i]| <= intLim.
func AllAbsLECheckInt64With(x []float64, limit, intLim float64, _ bool) (valid, allInt bool) {
	return AllAbsLECheckInt64Scalar(x, limit, intLim)
}

// ValidateAndNegateCheckInt64With writes dst[i] = 0.0 - src[i] and reports whether all entries satisfy
// |src[i]| <= limit and whether all entries are exact integers with |src[i]| <= intLim.
func ValidateAndNegateCheckInt64With(dst, src []float64, limit, intLim float64, _ bool) (valid, allInt bool) {
	return ValidateAndNegateCheckInt64Scalar(dst, src, limit, intLim)
}

// ColReduceValidateCheckIntRows64With validates |cost| <= limit, checks exact-integer membership
// within intLim, and performs row-major column reduction over rows [rowStart, rowEnd).
func ColReduceValidateCheckIntRows64With(n, rowStart, rowEnd int, cost, v []float64, row4col []int, limit, intLim float64, _ bool) (valid, allInt bool) {
	return ColReduceValidateCheckIntRows64Scalar(n, rowStart, rowEnd, cost, v, row4col, limit, intLim)
}
