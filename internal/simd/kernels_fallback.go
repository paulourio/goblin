//go:build !goexperiment.simd || !amd64

package simd

import (
	"fmt"
	"math"
	"os"
)

func init() {
	maxTierCap.Store(tierAVX512)
	if envTier := os.Getenv("GOBLIN_SIMD_MAX_TIER"); envTier != "" {
		_ = SetMaxTier(envTier)
	}
}

// HardwareSIMDAvailable reports false on scalar fallback builds.
func HardwareSIMDAvailable() bool { return false }

// HardwareSIMDTier returns "scalar" on scalar fallback builds.
func HardwareSIMDTier() string { return TierScalar }

// ActiveSIMDTier returns "scalar".
func ActiveSIMDTier() string { return TierScalar }

// ActiveSIMDTierWith returns "scalar" regardless of the per-solve disable flag.
func ActiveSIMDTierWith(_ bool) string { return TierScalar }

// SetMaxTier stores the runtime SIMD tier cap; scalar fallback builds ignore the cap and always run scalar kernels.
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
	return nil
}

// SetDisabled toggles the process-wide SIMD disable flag.
func SetDisabled(disabled bool) { simdDisabled.Store(disabled) }

// UsingHardwareSIMD returns false.
func UsingHardwareSIMD() bool { return false }

// UsingHardwareSIMDWith returns false on non-SIMD builds regardless of the per-solve disable flag.
func UsingHardwareSIMDWith(_ bool) bool { return false }

// Dot64 computes the dot product sum_i (a[i] * b[i]) via Dot64Scalar.
func Dot64(a, b []float64) float64 {
	return Dot64Scalar(a, b)
}

// Axpy64 computes dst[i] += alpha * src[i] via Axpy64Scalar.
func Axpy64(dst, src []float64, alpha float64) {
	Axpy64Scalar(dst, src, alpha)
}

// Scale64 computes dst[i] *= alpha via Scale64Scalar.
func Scale64(dst []float64, alpha float64) {
	Scale64Scalar(dst, alpha)
}

// AbsMax64 returns max_i |x[i]|.
func AbsMax64(x []float64) float64 {
	return AbsMax64Scalar(x)
}

// AllAbsLE64With reports whether every element x[i] satisfies |x[i]| <= limit.
func AllAbsLE64With(x []float64, limit float64, _ bool) bool {
	return AllAbsLE64Scalar(x, limit)
}

// RowActivityBoundsFinite returns the minimum and maximum row sum over finite [lb[j], ub[j]] bounds.
func RowActivityBoundsFinite(row, lb, ub []float64) (minAct, maxAct float64) {
	n := min(len(row), min(len(lb), len(ub)))
	if n == 0 {
		return 0, 0
	}
	row = row[:n]
	lb = lb[:n]
	ub = ub[:n]
	for i, r := range row {
		p1 := r * lb[i]
		p2 := r * ub[i]
		minAct += min(p1, p2)
		maxAct += max(p1, p2)
	}
	return minAct, maxAct
}

// MinReducedSlice64With returns min_j (rowCost[j] - v[j]) over 0 <= j < min(len(rowCost), len(v)).
func MinReducedSlice64With(rowCost, v []float64, _ bool) float64 {
	return MinReducedSlice64Scalar(rowCost, v)
}

// InitReducedRow64Scalar computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
func InitReducedRow64Scalar(dst, costRow, v []float64, ui float64) float64 {
	return InitReducedRow64(dst, costRow, v, ui)
}

// InitReducedRow64With computes dst[j] = (costRow[j] - v[j]) - ui and returns min_j dst[j].
func InitReducedRow64With(dst, costRow, v []float64, ui float64, _ bool) float64 {
	return InitReducedRow64(dst, costRow, v, ui)
}

// InitReducedRow64 writes dst[j] = (costRow[j] - v[j]) - ui across 4 scalar lanes and returns min_j dst[j].
func InitReducedRow64(dst, costRow, v []float64, ui float64) float64 {
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

// Min2ReducedRow64With computes the smallest (u1, j1) and second-smallest (u2, j2)
// values of costRow[j] - v[j].
func Min2ReducedRow64With(costRow, v []float64, _ bool) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64Scalar(costRow, v)
}

// Min2ReducedRow64 returns the smallest (u1, j1) and second-smallest (u2, j2) entries of costRow[j] - v[j].
func Min2ReducedRow64(costRow, v []float64) (u1, u2 float64, j1, j2 int) {
	return Min2ReducedRow64Scalar(costRow, v)
}

// SelectDualLeaveRow64With selects the dual steepest-edge leaving row.
func SelectDualLeaveRow64With(xb, basicLB, basicUB, basicTol, dse []float64, _ bool) (leaveRow int, leaveDir, primalInfeas float64) {
	return SelectDualLeaveRow64Scalar(xb, basicLB, basicUB, basicTol, dse)
}

// ScanDualRatioCand64With scans 64-column structural chunks starting at (chunkPos, elemPos).
func ScanDualRatioCand64With[S ~uint8](
	tab, dj []float64,
	status []S,
	chunks []int,
	chunkPos, elemPos int,
	negLeaveDir, pivotTol, thetaGuard float64,
	bland, _ bool,
	dst []int,
) (hitCount, nextChunk, nextElem int) {
	return ScanDualRatioCand64Scalar(tab, dj, status, chunks, chunkPos, elemPos, negLeaveDir, pivotTol, thetaGuard, bland, dst)
}
