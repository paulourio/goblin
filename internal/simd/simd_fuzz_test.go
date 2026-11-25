package simd_test

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

var fuzzCriticalLengths = []int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17,
	31, 32, 33, 34, 35,
	63, 64, 65, 66, 67,
	127, 128, 129, 130, 131,
	255, 256, 257, 258, 259,
	2047, 2048, 2049, 2050, 2051, 2052,
	8191, 8192, 8193, 8194, 8195, 8196, 8197, 8198, 8199,
}

var ieee754Specials = []float64{
	0.0,
	math.Copysign(0, -1),
	math.NaN(),
	math.Float64frombits(0x7ff0000000000001), // signaling NaN
	math.Float64frombits(0xfff0000000000001), // negative signaling NaN
	math.Float64frombits(0xfff8000000000001), // negative quiet NaN
	math.Inf(1),
	math.Inf(-1),
	math.SmallestNonzeroFloat64,
	-math.SmallestNonzeroFloat64,
	math.Float64frombits(0x000fffffffffffff), // largest positive subnormal
	1e308,
	-1e308,
	1.0,
	-1.0,
}

type fuzzByteReader struct {
	data []byte
	pos  int
}

func (r *fuzzByteReader) nextByte() byte {
	if len(r.data) == 0 {
		return 0
	}
	b := r.data[r.pos%len(r.data)]
	r.pos++
	return b
}

func (r *fuzzByteReader) nextUint64() uint64 {
	if len(r.data) == 0 {
		return 0
	}
	var buf [8]byte
	for i := range buf {
		buf[i] = r.nextByte()
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func (r *fuzzByteReader) nextLen() int {
	sel := r.nextByte()
	if sel < 224 {
		return fuzzCriticalLengths[int(r.nextByte())%len(fuzzCriticalLengths)]
	}
	v := int(r.nextUint64() % 8205)
	return v
}

func (r *fuzzByteReader) nextSpecialOrFloat(mode byte) float64 {
	tag := r.nextByte()
	switch mode % 4 {
	case 0:
		if tag%3 == 0 {
			return ieee754Specials[int(tag/3)%len(ieee754Specials)]
		}
		return float64(int(tag)-128) * 0.25
	case 1:
		if tag < 64 {
			return ieee754Specials[int(tag)%len(ieee754Specials)]
		}
		return math.Float64frombits(r.nextUint64())
	case 2:
		// Quantized near-ties around integer and subnormal multiples.
		k := int(tag&7) - 3
		if tag >= 240 {
			return math.SmallestNonzeroFloat64 * float64(k)
		}
		return 10.0 + float64(k)*1e-12
	default:
		// Well-scaled finite values in [-1e6, 1e6].
		raw := int32(r.nextUint64() & 0xfffffff)
		return float64(raw-(1<<27)) * 1e-4
	}
}

func fillFuzzFloatSlice(dst []float64, r *fuzzByteReader, mode byte) {
	n := len(dst)
	if n == 0 {
		return
	}
	if mode%4 == 2 && n >= 8 {
		base := 1.0
		for i := range dst {
			dst[i] = base + float64(i&7)*0.5
		}
		tieVal := -5.0
		lane := int(r.nextByte() & 7)
		dst[lane] = tieVal
		if n > 16 {
			dst[n-8+lane] = tieVal
		}
		return
	}
	if n > 128 && r.nextByte()%4 != 0 {
		// Pattern-fill long slices directly from a compact seed cycle to bound per-iter fuzz work.
		var cycle [32]float64
		for i := range cycle {
			cycle[i] = r.nextSpecialOrFloat(mode)
		}
		for i := range dst {
			dst[i] = cycle[(i*5+(i>>4))&31]
		}
		// Place boundary tie/special values in leading lanes and trailing elements.
		for lane := range 8 {
			dst[lane] = r.nextSpecialOrFloat(mode)
			dst[n-1-lane] = r.nextSpecialOrFloat(mode)
		}
		return
	}
	for i := range dst {
		dst[i] = r.nextSpecialOrFloat(mode)
	}
}

func appendParitySeed(f *testing.F, n int, mode, tieLane, outCap, extra byte) {
	f.Helper()
	var lenIdx byte
	for idx, v := range fuzzCriticalLengths {
		if v == n {
			lenIdx = byte(idx)
			break
		}
	}
	buf := []byte{
		0, lenIdx, mode, tieLane, outCap, extra,
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x7f,
		0x80, 0x91, 0xa2, 0xb3, 0xc4, 0xd5, 0xe6, 0xff,
	}
	f.Add(buf)
}

// TestGOBLINSIMDMaxTierEnvInit verifies that init applies GOBLIN_SIMD_MAX_TIER
// when set to valid tier names and ignores unrecognized values.
func TestGOBLINSIMDMaxTierEnvInit(t *testing.T) {
	if os.Getenv("GOBLIN_SIMD_SUBPROC_CHECK") == "1" {
		t.Logf("ACTIVE=%s MAX=%s HW=%s", simd.ActiveSIMDTier(), simd.MaxTier(), simd.HardwareSIMDTier())
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	baseEnv := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GOBLIN_SIMD_MAX_TIER=") && !strings.HasPrefix(kv, "GOBLIN_SIMD_SUBPROC_CHECK=") {
			baseEnv = append(baseEnv, kv)
		}
	}

	hw := simd.HardwareSIMDTier()
	cases := []struct {
		envVal     string
		wantMax    string
		wantActive string
	}{
		{envVal: simd.TierScalar, wantMax: simd.TierScalar, wantActive: simd.TierScalar},
		{envVal: simd.TierAVX2, wantMax: simd.TierAVX2, wantActive: minTierForHW(simd.TierAVX2, hw)},
		{envVal: simd.TierAVX512, wantMax: simd.TierAVX512, wantActive: hw},
		{envVal: "invalid-tier", wantMax: simd.TierAVX512, wantActive: hw},
	}

	for _, tc := range cases {
		cmd := exec.CommandContext(t.Context(), exe, "-test.v", "-test.run=^TestGOBLINSIMDMaxTierEnvInit$")
		cmd.Env = append(slices.Clone(baseEnv),
			"GOBLIN_SIMD_SUBPROC_CHECK=1",
			"GOBLIN_SIMD_MAX_TIER="+tc.envVal,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("subproc GOBLIN_SIMD_MAX_TIER=%q failed: %v\n%s", tc.envVal, err, out)
		}
		wantNeedle := "ACTIVE=" + tc.wantActive + " MAX=" + tc.wantMax + " HW=" + hw
		if !strings.Contains(string(out), wantNeedle) {
			t.Fatalf("GOBLIN_SIMD_MAX_TIER=%q: output missing %q\n%s", tc.envVal, wantNeedle, out)
		}
	}
}

func minTierForHW(capTier, hw string) string {
	rank := map[string]int{
		simd.TierScalar: 0,
		simd.TierAVX2:   1,
		simd.TierAVX512: 2,
	}
	if rank[capTier] <= rank[hw] {
		return capTier
	}
	return hw
}

func dotOrderDependentInf(a, b []float64) (l1Scale float64, hasNaN, hasInf, orderSensitive bool) {
	n := min(len(a), len(b))
	var hasPos, hasNeg, hasPosInf, hasNegInf bool
	for i := range n {
		ai, bi := a[i], b[i]
		p := ai * bi
		if math.IsNaN(p) {
			hasNaN = true
		}
		if math.IsInf(ai, 1) || math.IsInf(bi, 1) || math.IsInf(ai, -1) || math.IsInf(bi, -1) {
			if math.IsInf(p, 1) {
				hasPosInf = true
			} else if math.IsInf(p, -1) {
				hasNegInf = true
			}
		} else if math.IsInf(p, 0) {
			// FMA avoids intermediate a[i]*b[i] overflow that separate mul+add triggers.
			orderSensitive = true
		}
		l1Scale += math.Abs(p)
		if p > 0 {
			hasPos = true
		} else if p < 0 {
			hasNeg = true
		}
	}
	hasInf = hasPosInf || hasNegInf
	if hasPosInf && hasNegInf {
		orderSensitive = true
	}
	if (math.IsNaN(l1Scale) || math.IsInf(l1Scale, 0) || l1Scale >= 1e305) && hasPos && hasNeg {
		orderSensitive = true
	}
	return l1Scale, hasNaN, hasInf, orderSensitive
}

func activityOrderDependentInf(row, lb, ub []float64) (l1Scale float64, hasNaN, hasInf, orderSensitive bool) {
	n := min(len(row), min(len(lb), len(ub)))
	var minPos, minNeg, maxPos, maxNeg bool
	var minPInf, minNInf, maxPInf, maxNInf bool
	for i := range n {
		a := row[i]
		var lo, hi float64
		if a >= 0 {
			lo = a * lb[i]
			hi = a * ub[i]
		} else {
			lo = a * ub[i]
			hi = a * lb[i]
		}
		if math.IsNaN(lo) || math.IsNaN(hi) {
			hasNaN = true
		}
		if math.IsInf(lo, 1) {
			minPInf = true
		} else if math.IsInf(lo, -1) {
			minNInf = true
		}
		if math.IsInf(hi, 1) {
			maxPInf = true
		} else if math.IsInf(hi, -1) {
			maxNInf = true
		}
		l1Scale += math.Abs(lo) + math.Abs(hi)
		if lo > 0 {
			minPos = true
		} else if lo < 0 {
			minNeg = true
		}
		if hi > 0 {
			maxPos = true
		} else if hi < 0 {
			maxNeg = true
		}
	}
	hasInf = minPInf || minNInf || maxPInf || maxNInf
	if (minPInf && minNInf) || (maxPInf && maxNInf) {
		orderSensitive = true
	}
	if (math.IsNaN(l1Scale) || math.IsInf(l1Scale, 0) || l1Scale >= 1e305) && ((minPos && minNeg) || (maxPos && maxNeg)) {
		orderSensitive = true
	}
	return l1Scale, hasNaN, hasInf, orderSensitive
}

func float64SliceBitsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameFloat64Bits(a[i], b[i]) {
			return false
		}
	}
	return true
}

func checkAccumParity(tb testing.TB, op, tier string, n int, got, want, l1Scale float64, hasNaN, hasInf, orderSensitive bool) {
	tb.Helper()
	if hasNaN || math.IsNaN(want) || math.IsNaN(got) {
		if hasNaN && (!math.IsNaN(got) || !math.IsNaN(want)) {
			tb.Fatalf("tier=%s n=%d %s NaN propagation mismatch: got=%v want=%v", tier, n, op, got, want)
		}
		if !orderSensitive && (math.IsNaN(got) != math.IsNaN(want)) {
			tb.Fatalf("tier=%s n=%d %s unexpected NaN divergence: got=%v want=%v", tier, n, op, got, want)
		}
		return
	}
	if orderSensitive {
		return
	}
	if hasInf || math.IsInf(want, 0) || math.IsInf(got, 0) {
		if (hasInf || l1Scale < 1e305) && !sameFloat64Bits(got, want) {
			tb.Fatalf("tier=%s n=%d %s Inf mismatch: got=%v want=%v", tier, n, op, got, want)
		}
		return
	}
	if l1Scale < 1e305 {
		if diff := math.Abs(got - want); diff > 1e-12*max(1, l1Scale) {
			tb.Fatalf("tier=%s n=%d %s mismatch: got=%v want=%v scale=%v", tier, n, op, got, want, l1Scale)
		}
	}
}

type simdScanPassOut struct {
	thetaMax float64
	minRatio float64
	subTol   bool
	hasNaN   bool
	cols     []int
	ratios   []float64
	absA     []float64
}

func runFuzzScanDualRatioPass(tab, dj []float64, status []uint8, chunks []int, leaveDir float64, bland bool, batchCap int, useScalarDirect bool) simdScanPassOut {
	const (
		pivotTol   = 1e-8
		harrisTol  = 1e-9
		guardDelta = 2e-12
	)
	n := len(tab)
	negLeave := -leaveDir
	thetaMax := math.Inf(1)
	minRatio := math.Inf(1)
	var out simdScanPassOut

	evalCol := func(k int) bool {
		tv := tab[k]
		if tv == 0 {
			return false
		}
		alphaBar := negLeave * tv
		st := status[k]
		if tv > pivotTol || tv < -pivotTol {
			var absA, djSign float64
			super := false
			switch st {
			case 0:
				if alphaBar <= 0 {
					return false
				}
				absA, djSign = alphaBar, 1.0
			case 1:
				if alphaBar >= 0 {
					return false
				}
				absA, djSign = -alphaBar, -1.0
			case 4:
				absA = math.Abs(alphaBar)
				super = true
			default:
				return false
			}
			if super {
				thetaMax, minRatio = 0, 0
				out.cols = append(out.cols, k)
				out.ratios = append(out.ratios, 0)
				out.absA = append(out.absA, absA)
				return false
			}
			d := djSign * dj[k]
			if math.IsNaN(d) {
				out.hasNaN = true
				return true
			}
			if d < 0 {
				d = 0
			}
			if bland {
				if r0 := d / absA; r0 < minRatio {
					minRatio = r0
				}
			}
			if d <= (thetaMax+guardDelta)*absA {
				invA := 1.0 / absA
				r0 := d * invA
				if hr := (d + harrisTol) * invA; hr < thetaMax {
					thetaMax = hr
				}
				out.cols = append(out.cols, k)
				out.ratios = append(out.ratios, r0)
				out.absA = append(out.absA, absA)
			}
			return false
		}
		if math.IsNaN(tv) {
			out.hasNaN = true
			return true
		}
		if (st == 0 && alphaBar > 0) || (st == 1 && alphaBar < 0) || st == 4 {
			out.subTol = true
		}
		return false
	}

	bufCap := max(1, batchCap)
	candBuf := make([]int, bufCap)
	ci, elem := 0, 0
	for ci < len(chunks) {
		var cnt int
		if useScalarDirect {
			cnt, ci, elem = simd.ScanDualRatioCand64Scalar(
				tab, dj, status, chunks, ci, elem,
				negLeave, pivotTol, thetaMax+guardDelta, bland, candBuf,
			)
		} else {
			cnt, ci, elem = simd.ScanDualRatioCand64With(
				tab, dj, status, chunks, ci, elem,
				negLeave, pivotTol, thetaMax+guardDelta, bland, false, candBuf,
			)
		}
		if cnt < 0 || cnt > len(candBuf) {
			out.hasNaN = true
			out.cols = []int{-1}
			return out
		}
		prev := -1
		for idx := range cnt {
			col := candBuf[idx]
			if col < 0 || col >= n || col <= prev {
				out.hasNaN = true
				out.cols = []int{-2}
				return out
			}
			prev = col
			if evalCol(col) {
				return out
			}
		}
	}
	out.thetaMax = thetaMax
	out.minRatio = minRatio
	return out
}

func scanPassOutEqual(a, b simdScanPassOut) bool {
	if !sameFloat64Bits(a.thetaMax, b.thetaMax) ||
		!sameFloat64Bits(a.minRatio, b.minRatio) ||
		a.subTol != b.subTol ||
		a.hasNaN != b.hasNaN {
		return false
	}
	return slices.Equal(a.cols, b.cols) &&
		float64SliceBitsEqual(a.ratios, b.ratios) &&
		float64SliceBitsEqual(a.absA, b.absA)
}

type simdRCPair struct {
	r int
	c int
}

func sortScanPairs(rows, cols []int, count int) []simdRCPair {
	out := make([]simdRCPair, count)
	for i := range count {
		out[i] = simdRCPair{r: rows[i], c: cols[i]}
	}
	slices.SortFunc(out, func(a, b simdRCPair) int {
		if a.r != b.r {
			return a.r - b.r
		}
		return a.c - b.c
	})
	return out
}

// FuzzSIMDKernelTierParity checks parity across scalar, AVX2, and AVX-512 dispatch tiers
// against the direct scalar reference kernels for all exported LP and assignment SIMD entry points.
func FuzzSIMDKernelTierParity(f *testing.F) {
	for _, n := range fuzzCriticalLengths {
		appendParitySeed(f, n, 0, byte(n&7), byte(n&7), 0)
	}
	for mode := range byte(4) {
		for tieLane := range byte(8) {
			appendParitySeed(f, 35, mode, tieLane, tieLane, mode^tieLane)
			appendParitySeed(f, 67, mode, tieLane, 2, mode+tieLane)
			appendParitySeed(f, 259, mode, tieLane, 7, mode+1)
			appendParitySeed(f, 2051, mode, tieLane, 8, mode)
		}
	}

	tiers := []string{simd.TierScalar, simd.TierAVX2, simd.TierAVX512}
	truncatedCaps := []int{0, 1, 2, 3, 4, 5, 7, 8, 15, 64}

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() { _ = simd.SetMaxTier("auto") }()
		r := fuzzByteReader{data: data}

		n := r.nextLen()
		mode := r.nextByte()
		tieLane := int(r.nextByte() & 7)
		outCap := truncatedCaps[int(r.nextByte())%len(truncatedCaps)]
		negateFlag := r.nextByte()&1 == 1
		startIdx := 0
		if n > 0 {
			switch r.nextByte() % 4 {
			case 0:
				startIdx = 0
			case 1:
				startIdx = min(tieLane, n-1)
			case 2:
				startIdx = max(0, n-1-tieLane)
			default:
				startIdx = int(r.nextUint64() % uint64(n))
			}
		}

		a := make([]float64, n)
		b := make([]float64, n)
		lb := make([]float64, n)
		ub := make([]float64, n)
		tol := make([]float64, n)
		dse := make([]float64, n)
		cost := make([]float64, n)
		v := make([]float64, n)
		dInit := make([]float64, n)
		predInit := make([]int, n)
		status := make([]uint8, n)
		fillFuzzFloatSlice(a, &r, mode)
		fillFuzzFloatSlice(b, &r, mode+1)
		fillFuzzFloatSlice(cost, &r, mode+2)
		fillFuzzFloatSlice(v, &r, mode+3)
		fillFuzzFloatSlice(dInit, &r, mode)

		for i := range n {
			lb[i] = float64((i%11)-5) * 0.5
			ub[i] = lb[i] + 1.0 + float64(i%7)*0.5
			tol[i] = 1e-7 * (1.0 + float64(i&3))
			dse[i] = 0.25 + float64(i%9)*0.5
			predInit[i] = i + 100
			status[i] = uint8((int(r.nextByte()) + i) % 5)
			if i == tieLane || (n > 8 && i == n-8+tieLane) {
				cost[i] = v[i] + 1.5
				if n > 0 && tieLane < n {
					cost[tieLane] = v[tieLane] + 1.5
				}
			}
		}
		if n > 1 && r.nextByte()%3 == 0 {
			// Inject exact lane boundary tie across two elements.
			posA := min(tieLane, n-1)
			posB := max(0, min(n-1, posA^4))
			a[posA] = lb[posA] - tol[posA] - 0.5
			a[posB] = lb[posB] - tol[posB] - 0.5
			dse[posA] = dse[posB]
			cost[posB] = v[posB] + (cost[posA] - v[posA])
		}

		alpha := r.nextSpecialOrFloat(mode)
		ui := r.nextSpecialOrFloat(mode + 1)
		limit := math.Abs(r.nextSpecialOrFloat(mode + 2))
		intLim := 1e9
		if r.nextByte()%5 == 0 {
			limit = r.nextSpecialOrFloat(mode)
			intLim = r.nextSpecialOrFloat(mode + 1)
		}
		skipIdx := -1
		if n > 0 {
			switch r.nextByte() % 4 {
			case 0:
				skipIdx = min(tieLane, n-1)
			case 1:
				skipIdx = n - 1
			case 2:
				skipIdx = n
			default:
				skipIdx = -1
			}
		}

		// Matrix dimensions capped so total elements remain fast at huge vector lengths.
		matRows := 1 + int(r.nextByte()%3)
		if n <= 259 {
			matRows = 1 + int(r.nextByte()%8)
		}
		if n > 0 && matRows*n > 32768 {
			matRows = max(1, 32768/n)
		}
		matCols := n
		matCost := make([]float64, matRows*matCols)
		fillFuzzFloatSlice(matCost, &r, mode+1)
		matU := make([]float64, matRows)
		matLimits := make([]float64, matRows)
		col4row := make([]int, matRows)
		for row := range matRows {
			matU[row] = r.nextSpecialOrFloat(mode)
			matLimits[row] = 1e-12 * float64(row+1)
			if matCols > 0 {
				col4row[row] = (row*3 + tieLane) % matCols
			}
		}

		// Precompute scalar references once per fuzz case.
		dotScale, dotHasNaN, dotHasInf, dotMixedInf := dotOrderDependentInf(a, b)
		wantDot := simd.Dot64Scalar(a, b)

		wantAxpy := slices.Clone(a)
		simd.Axpy64Scalar(wantAxpy, b, alpha)

		wantScale := slices.Clone(a)
		simd.Scale64Scalar(wantScale, alpha)

		wantAbsMax := simd.AbsMax64Scalar(a)
		wantAllAbs := simd.AllAbsLE64Scalar(a, limit)
		wantAbsIntV, wantAbsIntI := simd.AllAbsLECheckInt64Scalar(a, limit, intLim)

		actScale, actHasNaN, actHasInf, actMixedInf := activityOrderDependentInf(a, lb, ub)
		wantActMin, wantActMax := simd.RowActivityBoundsFiniteScalar(a, lb, ub)

		wantMinRed := simd.MinReducedSlice64Scalar(cost, v)
		wantInitDst := make([]float64, n)
		wantInitRet := simd.InitReducedRow64Scalar(wantInitDst, cost, v, ui)
		wantU1, wantU2, wantJ1, wantJ2 := simd.Min2ReducedRow64Scalar(cost, v)
		wantLR, wantLD, wantLI := simd.SelectDualLeaveRow64Scalar(a, lb, ub, tol, dse)

		numChunks := (n + 63) / 64
		chunks := make([]int, numChunks)
		for ci := range chunks {
			chunks[ci] = ci
		}
		bland := r.nextByte()&1 == 1
		leaveDir := 1.0
		if r.nextByte()&1 == 1 {
			leaveDir = -1.0
		}
		wantPass := runFuzzScanDualRatioPass(cost, v, status, chunks, leaveDir, bland, outCap, true)

		wantRelD, wantRelPred, wantRelTights := relaxFullStep(t, cost, v, dInit, predInit, 1, ui, 1.0, true)
		wantMinNoNaN := simd.MinNoNaN64Scalar(a)

		wantCollOut := make([]int, outCap)
		wantCollNext, wantCollCnt := simd.CollectLE64Scalar(a, startIdx, limit, wantCollOut)

		fillVal := int(r.nextUint64())
		wantFill := make([]int, n)
		simd.FillIntScalar(wantFill, fillVal)

		wantMinSkip := simd.MinReducedSkip64Scalar(cost, v, skipIdx)

		wantNegDst := make([]float64, n)
		wantNegOK := simd.ValidateAndNegate64Scalar(wantNegDst, cost, limit)

		wantNegIntDst := make([]float64, n)
		wantNegIntV, wantNegIntI := simd.ValidateAndNegateCheckInt64Scalar(wantNegIntDst, cost, limit, intLim)

		rowStart := 0
		if matRows > 1 && r.nextByte()%3 == 0 {
			rowStart = 1
		}
		wantColV := make([]float64, matCols)
		wantColR := make([]int, matCols)
		fillFuzzFloatSlice(wantColV, &r, mode)
		for j := range wantColR {
			wantColR[j] = -1
		}
		colVInit := slices.Clone(wantColV)
		colRInit := slices.Clone(wantColR)

		wantColValOK := simd.ColReduceValidateRows64Scalar(matCols, rowStart, matRows, matCost, wantColV, wantColR, limit)

		wantColIntV := slices.Clone(colVInit)
		wantColIntR := slices.Clone(colRInit)
		wantColIntOK, wantColIntAll := simd.ColReduceValidateCheckIntRows64Scalar(matCols, rowStart, matRows, matCost, wantColIntV, wantColIntR, limit, intLim)

		wantColOnlyV := slices.Clone(colVInit)
		wantColOnlyR := slices.Clone(colRInit)
		simd.ColReduceRows64Scalar(matCols, rowStart, matRows, matCost, wantColOnlyV, wantColOnlyR)

		wantTransDst := make([]float64, matRows*matCols)
		simd.Transpose64Scalar(wantTransDst, matCost, matRows, matCols, negateFlag)

		wantScanRowOut := make([]int, outCap)
		wantScanRowNext, wantScanRowCnt := simd.ScanNearTightRow64Scalar(cost, v, startIdx, ui, limit, wantScanRowOut)

		wantScanMatRows := make([]int, outCap)
		wantScanMatCols := make([]int, outCap)
		wantScanMatCnt, wantScanMatOv := simd.ScanNearTightMatrix64Scalar(matRows, matCols, matCost, matU, v, matLimits, col4row, wantScanMatRows, wantScanMatCols)
		var wantScanMatPairs []simdRCPair
		if !wantScanMatOv {
			wantScanMatPairs = sortScanPairs(wantScanMatRows, wantScanMatCols, wantScanMatCnt)
		}

		for _, tier := range tiers {
			if err := simd.SetMaxTier(tier); err != nil {
				t.Fatalf("SetMaxTier(%q): %v", tier, err)
			}

			gotDot := simd.Dot64(a, b)
			checkAccumParity(t, "Dot64", tier, n, gotDot, wantDot, dotScale, dotHasNaN, dotHasInf, dotMixedInf)

			gotAxpy := slices.Clone(a)
			simd.Axpy64(gotAxpy, b, alpha)
			if !float64SliceBitsEqual(gotAxpy, wantAxpy) {
				t.Fatalf("tier=%s n=%d alpha=%v Axpy64 mismatch", tier, n, alpha)
			}

			gotScale := slices.Clone(a)
			simd.Scale64(gotScale, alpha)
			if !float64SliceBitsEqual(gotScale, wantScale) {
				t.Fatalf("tier=%s n=%d alpha=%v Scale64 mismatch", tier, n, alpha)
			}

			if got := simd.AbsMax64(a); !sameFloat64Bits(got, wantAbsMax) {
				t.Fatalf("tier=%s n=%d AbsMax64 got=%x want=%x", tier, n, math.Float64bits(got), math.Float64bits(wantAbsMax))
			}

			if got := simd.AllAbsLE64With(a, limit, false); got != wantAllAbs {
				t.Fatalf("tier=%s n=%d AllAbsLE64With got=%v want=%v", tier, n, got, wantAllAbs)
			}

			gotAbsIntV, gotAbsIntI := simd.AllAbsLECheckInt64With(a, limit, intLim, false)
			if gotAbsIntV != wantAbsIntV || gotAbsIntI != wantAbsIntI {
				t.Fatalf("tier=%s n=%d AllAbsLECheckInt64With got=(%v,%v) want=(%v,%v)",
					tier, n, gotAbsIntV, gotAbsIntI, wantAbsIntV, wantAbsIntI)
			}

			gotActMin, gotActMax := simd.RowActivityBoundsFinite(a, lb, ub)
			checkAccumParity(t, "RowActivityBoundsFinite(min)", tier, n, gotActMin, wantActMin, actScale, actHasNaN, actHasInf, actMixedInf)
			checkAccumParity(t, "RowActivityBoundsFinite(max)", tier, n, gotActMax, wantActMax, actScale, actHasNaN, actHasInf, actMixedInf)

			if got := simd.MinReducedSlice64With(cost, v, false); !sameFloat64Bits(got, wantMinRed) {
				t.Fatalf("tier=%s n=%d MinReducedSlice64With got=%x want=%x", tier, n, math.Float64bits(got), math.Float64bits(wantMinRed))
			}

			gotInitDst := make([]float64, n)
			gotInitRet := simd.InitReducedRow64(gotInitDst, cost, v, ui)
			if !sameFloat64Bits(gotInitRet, wantInitRet) || !float64SliceBitsEqual(gotInitDst, wantInitDst) {
				t.Fatalf("tier=%s n=%d InitReducedRow64 mismatch", tier, n)
			}
			gotInitDstW := make([]float64, n)
			gotInitRetW := simd.InitReducedRow64With(gotInitDstW, cost, v, ui, false)
			if !sameFloat64Bits(gotInitRetW, wantInitRet) || !float64SliceBitsEqual(gotInitDstW, wantInitDst) {
				t.Fatalf("tier=%s n=%d InitReducedRow64With mismatch", tier, n)
			}

			gotU1, gotU2, gotJ1, gotJ2 := simd.Min2ReducedRow64(cost, v)
			if !sameFloat64Bits(gotU1, wantU1) || !sameFloat64Bits(gotU2, wantU2) || gotJ1 != wantJ1 || gotJ2 != wantJ2 {
				t.Fatalf("tier=%s n=%d Min2ReducedRow64 got=(%v,%v,%d,%d) want=(%v,%v,%d,%d)",
					tier, n, gotU1, gotU2, gotJ1, gotJ2, wantU1, wantU2, wantJ1, wantJ2)
			}
			gotU1W, gotU2W, gotJ1W, gotJ2W := simd.Min2ReducedRow64With(cost, v, false)
			if !sameFloat64Bits(gotU1W, wantU1) || !sameFloat64Bits(gotU2W, wantU2) || gotJ1W != wantJ1 || gotJ2W != wantJ2 {
				t.Fatalf("tier=%s n=%d Min2ReducedRow64With mismatch", tier, n)
			}

			gotLR, gotLD, gotLI := simd.SelectDualLeaveRow64With(a, lb, ub, tol, dse, false)
			if gotLR != wantLR || !sameFloat64Bits(gotLD, wantLD) || !sameFloat64Bits(gotLI, wantLI) {
				t.Fatalf("tier=%s n=%d SelectDualLeaveRow64With got=(%d,%x,%x) want=(%d,%x,%x)",
					tier, n, gotLR, math.Float64bits(gotLD), math.Float64bits(gotLI), wantLR, math.Float64bits(wantLD), math.Float64bits(wantLI))
			}

			gotPass := runFuzzScanDualRatioPass(cost, v, status, chunks, leaveDir, bland, outCap, false)
			if !scanPassOutEqual(gotPass, wantPass) {
				t.Fatalf("tier=%s n=%d ScanDualRatioCand64With mismatch (bland=%v cap=%d)", tier, n, bland, outCap)
			}

			gotRelD, gotRelPred, gotRelTights := relaxFullStep(t, cost, v, dInit, predInit, 1, ui, 1.0, false)
			if !float64SliceBitsEqual(gotRelD, wantRelD) || !slices.Equal(gotRelPred, wantRelPred) || !slices.Equal(gotRelTights, wantRelTights) {
				t.Fatalf("tier=%s n=%d RelaxRow64With mismatch", tier, n)
			}
			// Exercise RelaxRow64 direct wrapper on starting position 0.
			directD := slices.Clone(dInit)
			directPred := slices.Clone(predInit)
			gotNextRel := simd.RelaxRow64(cost, v, directD, directPred, 0, 1, ui, 1.0)
			if gotNextRel < 0 || gotNextRel > n {
				t.Fatalf("tier=%s n=%d RelaxRow64 out-of-range next=%d", tier, n, gotNextRel)
			}

			if got := simd.MinNoNaN64(a); !sameFloat64Bits(got, wantMinNoNaN) {
				t.Fatalf("tier=%s n=%d MinNoNaN64 got=%x want=%x", tier, n, math.Float64bits(got), math.Float64bits(wantMinNoNaN))
			}
			if got := simd.MinNoNaN64With(a, false); !sameFloat64Bits(got, wantMinNoNaN) {
				t.Fatalf("tier=%s n=%d MinNoNaN64With got=%x want=%x", tier, n, math.Float64bits(got), math.Float64bits(wantMinNoNaN))
			}

			gotCollOut := make([]int, outCap)
			gotCollNext, gotCollCnt := simd.CollectLE64(a, startIdx, limit, gotCollOut)
			if gotCollNext != wantCollNext || gotCollCnt != wantCollCnt || !slices.Equal(gotCollOut[:gotCollCnt], wantCollOut[:wantCollCnt]) {
				t.Fatalf("tier=%s n=%d CollectLE64 got=(%d,%d,%v) want=(%d,%d,%v)",
					tier, n, gotCollNext, gotCollCnt, gotCollOut[:gotCollCnt], wantCollNext, wantCollCnt, wantCollOut[:wantCollCnt])
			}
			gotCollOutW := make([]int, outCap)
			gotCollNextW, gotCollCntW := simd.CollectLE64With(a, startIdx, limit, gotCollOutW, false)
			if gotCollNextW != wantCollNext || gotCollCntW != wantCollCnt || !slices.Equal(gotCollOutW[:gotCollCntW], wantCollOut[:wantCollCnt]) {
				t.Fatalf("tier=%s n=%d CollectLE64With mismatch", tier, n)
			}

			gotFill := make([]int, n)
			simd.FillInt(gotFill, fillVal)
			if !slices.Equal(gotFill, wantFill) {
				t.Fatalf("tier=%s n=%d FillInt mismatch", tier, n)
			}
			gotFillW := make([]int, n)
			simd.FillIntWith(gotFillW, fillVal, false)
			if !slices.Equal(gotFillW, wantFill) {
				t.Fatalf("tier=%s n=%d FillIntWith mismatch", tier, n)
			}

			if got := simd.MinReducedSkip64(cost, v, skipIdx); !sameFloat64Bits(got, wantMinSkip) {
				t.Fatalf("tier=%s n=%d skip=%d MinReducedSkip64 got=%x want=%x", tier, n, skipIdx, math.Float64bits(got), math.Float64bits(wantMinSkip))
			}
			if got := simd.MinReducedSkip64With(cost, v, skipIdx, false); !sameFloat64Bits(got, wantMinSkip) {
				t.Fatalf("tier=%s n=%d skip=%d MinReducedSkip64With got=%x want=%x", tier, n, skipIdx, math.Float64bits(got), math.Float64bits(wantMinSkip))
			}

			gotNegDst := make([]float64, n)
			gotNegOK := simd.ValidateAndNegate64(gotNegDst, cost, limit)
			if gotNegOK != wantNegOK || !float64SliceBitsEqual(gotNegDst, wantNegDst) {
				t.Fatalf("tier=%s n=%d ValidateAndNegate64 mismatch gotOK=%v wantOK=%v", tier, n, gotNegOK, wantNegOK)
			}
			gotNegDstW := make([]float64, n)
			gotNegOKW := simd.ValidateAndNegate64With(gotNegDstW, cost, limit, false)
			if gotNegOKW != wantNegOK || !float64SliceBitsEqual(gotNegDstW, wantNegDst) {
				t.Fatalf("tier=%s n=%d ValidateAndNegate64With mismatch", tier, n)
			}

			gotNegIntDst := make([]float64, n)
			gotNegIntV, gotNegIntI := simd.ValidateAndNegateCheckInt64With(gotNegIntDst, cost, limit, intLim, false)
			if gotNegIntV != wantNegIntV || gotNegIntI != wantNegIntI || !float64SliceBitsEqual(gotNegIntDst, wantNegIntDst) {
				t.Fatalf("tier=%s n=%d ValidateAndNegateCheckInt64With mismatch got=(%v,%v) want=(%v,%v)",
					tier, n, gotNegIntV, gotNegIntI, wantNegIntV, wantNegIntI)
			}

			gotColV := slices.Clone(colVInit)
			gotColR := slices.Clone(colRInit)
			gotColValOK := simd.ColReduceValidateRows64(matCols, rowStart, matRows, matCost, gotColV, gotColR, limit)
			if gotColValOK != wantColValOK {
				t.Fatalf("tier=%s m=%d n=%d ColReduceValidateRows64 ok=%v want=%v", tier, matRows, matCols, gotColValOK, wantColValOK)
			}
			if gotColValOK && (!float64SliceBitsEqual(gotColV, wantColV) || !slices.Equal(gotColR, wantColR)) {
				t.Fatalf("tier=%s m=%d n=%d ColReduceValidateRows64 valid output mismatch", tier, matRows, matCols)
			}

			gotColIntV := slices.Clone(colVInit)
			gotColIntR := slices.Clone(colRInit)
			gotColIntOK, gotColIntAll := simd.ColReduceValidateCheckIntRows64With(matCols, rowStart, matRows, matCost, gotColIntV, gotColIntR, limit, intLim, false)
			if gotColIntOK != wantColIntOK || gotColIntAll != wantColIntAll {
				t.Fatalf("tier=%s m=%d n=%d ColReduceValidateCheckIntRows64With got=(%v,%v) want=(%v,%v)",
					tier, matRows, matCols, gotColIntOK, gotColIntAll, wantColIntOK, wantColIntAll)
			}
			if gotColIntOK && (!float64SliceBitsEqual(gotColIntV, wantColIntV) || !slices.Equal(gotColIntR, wantColIntR)) {
				t.Fatalf("tier=%s m=%d n=%d ColReduceValidateCheckIntRows64With valid output mismatch", tier, matRows, matCols)
			}

			gotColOnlyV := slices.Clone(colVInit)
			gotColOnlyR := slices.Clone(colRInit)
			simd.ColReduceRows64(matCols, rowStart, matRows, matCost, gotColOnlyV, gotColOnlyR)
			if !float64SliceBitsEqual(gotColOnlyV, wantColOnlyV) || !slices.Equal(gotColOnlyR, wantColOnlyR) {
				t.Fatalf("tier=%s m=%d n=%d ColReduceRows64 mismatch", tier, matRows, matCols)
			}

			gotTransDst := make([]float64, matRows*matCols)
			simd.Transpose64(gotTransDst, matCost, matRows, matCols, negateFlag)
			if !float64SliceBitsEqual(gotTransDst, wantTransDst) {
				t.Fatalf("tier=%s m=%d n=%d neg=%v Transpose64 mismatch", tier, matRows, matCols, negateFlag)
			}
			gotTransDstW := make([]float64, matRows*matCols)
			simd.Transpose64With(gotTransDstW, matCost, matRows, matCols, negateFlag, false)
			if !float64SliceBitsEqual(gotTransDstW, wantTransDst) {
				t.Fatalf("tier=%s m=%d n=%d neg=%v Transpose64With mismatch", tier, matRows, matCols, negateFlag)
			}

			gotScanRowOut := make([]int, outCap)
			gotScanRowNext, gotScanRowCnt := simd.ScanNearTightRow64(cost, v, startIdx, ui, limit, gotScanRowOut)
			if gotScanRowNext != wantScanRowNext || gotScanRowCnt != wantScanRowCnt || !slices.Equal(gotScanRowOut[:gotScanRowCnt], wantScanRowOut[:wantScanRowCnt]) {
				t.Fatalf("tier=%s n=%d ScanNearTightRow64 got=(%d,%d,%v) want=(%d,%d,%v)",
					tier, n, gotScanRowNext, gotScanRowCnt, gotScanRowOut[:gotScanRowCnt], wantScanRowNext, wantScanRowCnt, wantScanRowOut[:wantScanRowCnt])
			}

			gotScanMatRows := make([]int, outCap)
			gotScanMatCols := make([]int, outCap)
			gotScanMatCnt, gotScanMatOv := simd.ScanNearTightMatrix64(matRows, matCols, matCost, matU, v, matLimits, col4row, gotScanMatRows, gotScanMatCols)
			if gotScanMatOv != wantScanMatOv {
				t.Fatalf("tier=%s m=%d n=%d ScanNearTightMatrix64 overflow=%v want=%v", tier, matRows, matCols, gotScanMatOv, wantScanMatOv)
			}
			if !wantScanMatOv {
				if gotScanMatCnt != wantScanMatCnt || !slices.Equal(sortScanPairs(gotScanMatRows, gotScanMatCols, gotScanMatCnt), wantScanMatPairs) {
					t.Fatalf("tier=%s m=%d n=%d ScanNearTightMatrix64 pairs mismatch (cnt=%d want=%d)",
						tier, matRows, matCols, gotScanMatCnt, wantScanMatCnt)
				}
			} else {
				if gotScanMatCnt != outCap {
					t.Fatalf("tier=%s m=%d n=%d ScanNearTightMatrix64 overflow cnt=%d want=%d",
						tier, matRows, matCols, gotScanMatCnt, outCap)
				}
				seen := make(map[simdRCPair]bool, gotScanMatCnt)
				for idx := range gotScanMatCnt {
					pr := gotScanMatRows[idx]
					pc := gotScanMatCols[idx]
					if pr < 0 || pr >= matRows || pc < 0 || pc >= matCols || pc == col4row[pr] {
						t.Fatalf("tier=%s m=%d n=%d invalid overflow pair (%d,%d)", tier, matRows, matCols, pr, pc)
					}
					pair := simdRCPair{r: pr, c: pc}
					if seen[pair] {
						t.Fatalf("tier=%s duplicate overflow pair (%d,%d)", tier, pr, pc)
					}
					seen[pair] = true
					if (matCost[pr*matCols+pc]-matU[pr])-v[pc] > matLimits[pr] {
						t.Fatalf("tier=%s invalid non-tight overflow pair (%d,%d)", tier, pr, pc)
					}
				}
			}
		}
	})
}
