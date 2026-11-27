package mad

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sort"
	"time"
)

const maxDyadicWords = 36

type certFaultKey struct{}

type madFaultFunc func(prob Problem, res *Result, err *error)

var defaultMADCertHooks = madCertHooks{
	certify: certifyMAD,
}

func setMADCertifiedTrue(cfg *Options) {
	cfg.Certified = true
	cfg.cert = &defaultMADCertHooks
}

func setMADCertifiedFalse(cfg *Options) {
	cfg.Certified = false
	cfg.cert = nil
}

// WithCertified enables or disables fail-closed exact polynomial fixed-width
// dyadic checks on the original IEEE-754 inputs.
func WithCertified(enabled bool) Option {
	if enabled {
		return setMADCertifiedTrue
	}
	return setMADCertifiedFalse
}

// CertifiedWitnessEval holds an exact fixed-width dyadic score for an item map.
// Total exact objective equals (sum_k Limbs[k] * 2^(64*k)) * 2^Exp.
type CertifiedWitnessEval struct {
	Result Result
	Limbs  []uint64
	Exp    int
}

type dyadicSpec struct {
	wSeries          int
	wObj             int
	eM               int
	eMin             int
	shiftSeriesToObj int
	rounded          bool
	step             float64
	roundWeight      float64
	mB               uint64
	eB               int
	shiftBToSeries   int
	mWr              uint64
	eWr              int
	shiftWrProdToObj int
	nMin             int64
}

func decomposeFloat64(v float64) (neg bool, mant uint64, exp int, isZero bool) {
	if v == 0 {
		return false, 0, 0, true
	}
	b := math.Float64bits(v)
	neg = (b >> 63) != 0
	expBits := int((b >> 52) & 0x7ff)
	frac := b & ((uint64(1) << 52) - 1)
	if expBits == 0 {
		mant = frac
		exp = -1074
	} else {
		mant = (uint64(1) << 52) | frac
		exp = expBits - 1075
	}
	tz := bits.TrailingZeros64(mant)
	mant >>= tz
	exp += tz
	return neg, mant, exp, false
}

func buildDyadicSpec(prob Problem, numGroups, numPeriods int, uMaxF float64) (dyadicSpec, error) {
	var ops uint64
	return buildDyadicSpecBudget(prob, numGroups, numPeriods, uMaxF, time.Time{}, false, nil, nil, &ops)
}

func buildDyadicSpecBudget(
	prob Problem,
	numGroups, numPeriods int,
	uMaxF float64,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
	ops *uint64,
) (dyadicSpec, error) {
	eSeries := 0
	haveSeries := false
	for _, row := range prob.Baselines {
		for _, v := range row {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, ops); err != nil {
				return dyadicSpec{}, err
			}
			if v == 0 {
				continue
			}
			_, _, e, _ := decomposeFloat64(v)
			if !haveSeries || e < eSeries {
				eSeries = e
				haveSeries = true
			}
		}
	}
	for _, row := range prob.Items {
		for _, v := range row {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, ops); err != nil {
				return dyadicSpec{}, err
			}
			if v == 0 {
				continue
			}
			_, _, e, _ := decomposeFloat64(v)
			if !haveSeries || e < eSeries {
				eSeries = e
				haveSeries = true
			}
		}
	}

	nMin := int64(1)
	if prob.AllowZeroStep {
		nMin = 0
	}

	rounded := prob.Step > 0 && prob.RoundWeight > 0
	eM := eSeries
	eMin := eSeries
	var mB, mWr uint64
	var eB, eWr int
	peakLevel := uMaxF

	if rounded {
		_, mB, eB, _ = decomposeFloat64(prob.Step)
		_, mWr, eWr, _ = decomposeFloat64(prob.RoundWeight)
		if haveSeries {
			eM = min(eSeries, eB)
		} else {
			eM = eB
		}
		eMin = eM + min(0, eWr)
		nMax := max(float64(nMin), math.Ceil(uMaxF/prob.Step))
		peakLevel = max(peakLevel, prob.Step*nMax)
	}

	topExp := 0
	if peakLevel > 0 {
		_, topExp = math.Frexp(peakLevel)
	}

	seriesBits := max(1, (topExp+1)-eM+2)
	wSeries := max(1, (seriesBits+63)/64)

	//nolint:gosec // G115: numGroups is validated >= 1.
	gBits := bits.Len64(uint64(numGroups))
	_, twExp := math.Frexp(float64(numPeriods) + prob.RoundWeight)
	objTopExp := gBits + (twExp + 1) + (topExp + 1)
	objBits := max(1, objTopExp-eMin+2)
	wObj := max(wSeries, (objBits+63)/64)
	if wObj > maxDyadicWords {
		return dyadicSpec{}, fmt.Errorf("%w: dyadic limb width %d exceeds %d", ErrCertificationFailed, wObj, maxDyadicWords)
	}

	return dyadicSpec{
		wSeries:          wSeries,
		wObj:             wObj,
		eM:               eM,
		eMin:             eMin,
		shiftSeriesToObj: eM - eMin,
		rounded:          rounded,
		step:             prob.Step,
		roundWeight:      prob.RoundWeight,
		mB:               mB,
		eB:               eB,
		shiftBToSeries:   eB - eM,
		mWr:              mWr,
		eWr:              eWr,
		shiftWrProdToObj: (eM + eWr) - eMin,
		nMin:             nMin,
	}, nil
}

func limbsAdd(dst, a, b []uint64) {
	var carry uint64
	for i := range dst {
		dst[i], carry = bits.Add64(a[i], b[i], carry)
	}
}

func limbsAddTo(dst, src []uint64) {
	var carry uint64
	for i := range dst {
		dst[i], carry = bits.Add64(dst[i], src[i], carry)
	}
}

func limbsSub(dst, a, b []uint64) {
	var borrow uint64
	for i := range dst {
		dst[i], borrow = bits.Sub64(a[i], b[i], borrow)
	}
}

func limbsNegInPlace(dst []uint64) {
	var borrow uint64
	for i := range dst {
		dst[i], borrow = bits.Sub64(0, dst[i], borrow)
	}
}

func limbsCmpSigned(a, b []uint64) int {
	const signBit = uint64(1) << 63
	top := len(a) - 1
	hiA := a[top] ^ signBit
	hiB := b[top] ^ signBit
	if hiA < hiB {
		return -1
	}
	if hiA > hiB {
		return 1
	}
	for i := top - 1; i >= 0; i-- {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func limbsCmpUnsigned(a, b []uint64) int {
	for i := range slices.Backward(a) {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func limbsIsZero(a []uint64) bool {
	for _, w := range a {
		if w != 0 {
			return false
		}
	}
	return true
}

func limbsAbsDiffSigned(dst, a, b []uint64) {
	if limbsCmpSigned(a, b) >= 0 {
		limbsSub(dst, a, b)
		return
	}
	limbsSub(dst, b, a)
}

func limbsAddFloatScaled(dst []uint64, v float64, gridExp int, tmp []uint64) {
	neg, mant, exp, isZero := decomposeFloat64(v)
	if isZero {
		return
	}
	clear(tmp)
	shift := exp - gridExp
	wIdx := shift / 64
	bOff := uint(shift % 64)
	tmp[wIdx] = mant << bOff
	if bOff > 0 && wIdx+1 < len(tmp) {
		tmp[wIdx+1] = mant >> (64 - bOff)
	}
	if neg {
		limbsNegInPlace(tmp)
	}
	limbsAddTo(dst, tmp)
}

func limbsSetFloatScaled(dst []uint64, v float64, gridExp int) {
	clear(dst)
	neg, mant, exp, isZero := decomposeFloat64(v)
	if isZero {
		return
	}
	shift := exp - gridExp
	wIdx := shift / 64
	bOff := uint(shift % 64)
	dst[wIdx] = mant << bOff
	if bOff > 0 && wIdx+1 < len(dst) {
		dst[wIdx+1] = mant >> (64 - bOff)
	}
	if neg {
		limbsNegInPlace(dst)
	}
}

func limbsSetProdU64Scaled(dst []uint64, factor, n uint64, shift int) {
	clear(dst)
	if n == 0 || factor == 0 {
		return
	}
	hi, lo := bits.Mul64(factor, n)
	wIdx := shift / 64
	bOff := uint(shift % 64)
	dst[wIdx] = lo << bOff
	if bOff == 0 {
		if wIdx+1 < len(dst) {
			dst[wIdx+1] = hi
		}
		return
	}
	if wIdx+1 < len(dst) {
		dst[wIdx+1] = (hi << bOff) | (lo >> (64 - bOff))
	}
	if wIdx+2 < len(dst) {
		dst[wIdx+2] = hi >> (64 - bOff)
	}
}

func limbsAddShifted(dst, src []uint64, shift int) {
	wOff := shift / 64
	bOff := uint(shift % 64)
	var prevHi, carry uint64
	for i := range src {
		loPart := (src[i] << bOff) | prevHi
		if bOff > 0 {
			prevHi = src[i] >> (64 - bOff)
		}
		idx := wOff + i
		if idx < len(dst) {
			dst[idx], carry = bits.Add64(dst[idx], loPart, carry)
		}
	}
	idx := wOff + len(src)
	if idx < len(dst) && (prevHi != 0 || carry != 0) {
		dst[idx], carry = bits.Add64(dst[idx], prevHi, carry)
		idx++
	}
	for carry != 0 && idx < len(dst) {
		dst[idx], carry = bits.Add64(dst[idx], 0, carry)
		idx++
	}
}

func limbsAddMulU64Shifted(dst, src []uint64, factor uint64, shift int, mulTmp []uint64) {
	var mulCarry uint64
	for i := range src {
		hi, lo := bits.Mul64(src[i], factor)
		sum, c1 := bits.Add64(lo, mulCarry, 0)
		mulTmp[i] = sum
		mulCarry = hi + c1
	}
	mulTmp[len(src)] = mulCarry
	limbsAddShifted(dst, mulTmp[:len(src)+1], shift)
}

func limbsMagToFloat64(mag []uint64, gridExp int) (val float64, exact, roundedUp bool) {
	hiWord := len(mag) - 1
	for hiWord > 0 && mag[hiWord] == 0 {
		hiWord--
	}
	if mag[hiWord] == 0 {
		return 0, true, false
	}
	bitLen := hiWord*64 + (64 - bits.LeadingZeros64(mag[hiWord]))
	msbExp := (bitLen - 1) + gridExp
	if msbExp > 1023 {
		return math.Inf(1), false, false
	}

	if msbExp >= -1022 {
		// Normal IEEE-754 float64.
		var sig uint64
		expBits := uint64(msbExp + 1023)
		if bitLen <= 53 {
			sig = mag[0] << uint(53-bitLen)
			exact = true
		} else {
			shift := bitLen - 53
			wIdx := shift / 64
			bOff := uint(shift % 64)
			sig = mag[wIdx] >> bOff
			if bOff > 0 && wIdx+1 < len(mag) {
				sig |= mag[wIdx+1] << (64 - bOff)
			}
			sig &= (uint64(1) << 53) - 1

			gPos := shift - 1
			gWord := gPos / 64
			gBit := uint(gPos % 64)
			guard := ((mag[gWord] >> gBit) & 1) != 0
			sticky := (mag[gWord] & ((uint64(1) << gBit) - 1)) != 0
			for k := 0; k < gWord && !sticky; k++ {
				if mag[k] != 0 {
					sticky = true
				}
			}
			exact = !guard && !sticky
			if guard && (sticky || (sig&1) == 1) {
				roundedUp = true
				sig++
				if sig == (uint64(1) << 53) {
					sig >>= 1
					expBits++
				}
			}
		}
		if expBits >= 0x7ff {
			return math.Inf(1), false, true
		}
		fBits := (expBits << 52) | (sig & ((uint64(1) << 52) - 1))
		return math.Float64frombits(fBits), exact, roundedUp
	}

	// Subnormal IEEE-754 float64 (LSB at 2^-1074).
	shift := -1074 - gridExp
	if shift <= 0 {
		sig := mag[0] << uint(-shift)
		return math.Float64frombits(sig), true, false
	}
	var sig uint64
	if shift < bitLen {
		wIdx := shift / 64
		bOff := uint(shift % 64)
		sig = mag[wIdx] >> bOff
		if bOff > 0 && wIdx+1 < len(mag) {
			sig |= mag[wIdx+1] << (64 - bOff)
		}
		sig &= (uint64(1) << 52) - 1
	}
	gPos := shift - 1
	var guard, sticky bool
	if gPos < bitLen {
		gWord := gPos / 64
		gBit := uint(gPos % 64)
		guard = ((mag[gWord] >> gBit) & 1) != 0
		sticky = (mag[gWord] & ((uint64(1) << gBit) - 1)) != 0
		for k := 0; k < gWord && !sticky; k++ {
			if mag[k] != 0 {
				sticky = true
			}
		}
	} else {
		sticky = true
	}
	exact = !guard && !sticky
	if guard && (sticky || (sig&1) == 1) {
		roundedUp = true
		sig++
	}
	return math.Float64frombits(sig), exact, roundedUp
}

func limbsMagToFloat64Floor(mag []uint64, gridExp int) float64 {
	f, exact, roundedUp := limbsMagToFloat64(mag, gridExp)
	if exact {
		return f
	}
	if math.IsInf(f, 1) {
		return math.MaxFloat64
	}
	if roundedUp {
		return math.Nextafter(f, math.Inf(-1))
	}
	return f
}

type dyadicScratch struct {
	sorted     []uint64
	sortIdx    []int
	sortTmp    []uint64
	pLimbs     []uint64
	pAltLimbs  []uint64
	mLimbs     []uint64
	mBestLimbs []uint64
	costBest   []uint64
	costCand   []uint64
	diffSeries []uint64
	mulTmp     []uint64
	zeroSeries []uint64
}

func newDyadicScratch(spec dyadicSpec, numPeriods int) *dyadicScratch {
	return &dyadicScratch{
		sorted:     make([]uint64, numPeriods*spec.wSeries),
		sortIdx:    make([]int, numPeriods),
		sortTmp:    make([]uint64, numPeriods*spec.wSeries),
		pLimbs:     make([]uint64, spec.wSeries),
		pAltLimbs:  make([]uint64, spec.wSeries),
		mLimbs:     make([]uint64, spec.wSeries),
		mBestLimbs: make([]uint64, spec.wSeries),
		costBest:   make([]uint64, spec.wObj),
		costCand:   make([]uint64, spec.wObj),
		diffSeries: make([]uint64, spec.wSeries),
		mulTmp:     make([]uint64, spec.wSeries+1),
		zeroSeries: make([]uint64, spec.wSeries),
	}
}

func sortDyadicSeries(sorted []uint64, numPeriods, wSeries int, idx []int, tmp []uint64) {
	for i := range numPeriods {
		idx[i] = i
	}
	// Use quadratic sort for small T, slices.SortFunc otherwise.
	if numPeriods <= 16 {
		for i := 1; i < numPeriods; i++ {
			cur := idx[i]
			curSlice := sorted[cur*wSeries : (cur+1)*wSeries]
			j := i - 1
			for j >= 0 {
				prev := idx[j]
				if limbsCmpSigned(sorted[prev*wSeries:(prev+1)*wSeries], curSlice) <= 0 {
					break
				}
				idx[j+1] = prev
				j--
			}
			idx[j+1] = cur
		}
	} else {
		slices.SortFunc(idx, func(a, b int) int {
			return limbsCmpSigned(sorted[a*wSeries:(a+1)*wSeries], sorted[b*wSeries:(b+1)*wSeries])
		})
	}
	for outPos, srcPos := range idx {
		copy(tmp[outPos*wSeries:(outPos+1)*wSeries], sorted[srcPos*wSeries:(srcPos+1)*wSeries])
	}
	copy(sorted, tmp)
}

func evalGroupDyadicForFixedN(
	sorted []uint64,
	numPeriods int,
	spec dyadicSpec,
	n int64,
	costOut, levelOut []uint64,
	sc *dyadicScratch,
) {
	wS := spec.wSeries
	//nolint:gosec // G115: n >= 0 for all feasible step counts.
	limbsSetProdU64Scaled(sc.pLimbs, spec.mB, uint64(n), spec.shiftBToSeries)

	if spec.roundWeight >= float64(numPeriods) {
		copy(levelOut, sc.pLimbs)
	} else {
		k := sort.Search(numPeriods, func(t int) bool {
			return limbsCmpSigned(sorted[t*wS:(t+1)*wS], sc.pLimbs) >= 0
		})
		kPrime := k + sort.Search(numPeriods-k, func(d int) bool {
			return limbsCmpSigned(sorted[(k+d)*wS:(k+d+1)*wS], sc.pLimbs) > 0
		})
		switch {
		case float64(2*k-numPeriods) > spec.roundWeight:
			j := max(0, min(numPeriods-1, int(math.Floor(0.5*(float64(numPeriods)+spec.roundWeight)))-1))
			for j > 0 && float64(2*j-numPeriods) >= spec.roundWeight {
				j--
			}
			for j+1 < numPeriods && float64(2*(j+1)-numPeriods) < spec.roundWeight {
				j++
			}
			sSlice := sorted[j*wS : (j+1)*wS]
			if limbsCmpSigned(sSlice, sc.zeroSeries) < 0 {
				clear(levelOut)
			} else {
				copy(levelOut, sSlice)
			}
		case float64(numPeriods-2*kPrime) > spec.roundWeight:
			j := max(kPrime, min(numPeriods-1, int(math.Floor(0.5*(float64(numPeriods)-spec.roundWeight)))-1))
			for j > kPrime && float64(numPeriods-2*j) <= spec.roundWeight {
				j--
			}
			for j+1 < numPeriods && float64(numPeriods-2*(j+1)) > spec.roundWeight {
				j++
			}
			sSlice := sorted[j*wS : (j+1)*wS]
			if limbsCmpSigned(sSlice, sc.zeroSeries) < 0 {
				clear(levelOut)
			} else {
				copy(levelOut, sSlice)
			}
		default:
			copy(levelOut, sc.pLimbs)
		}
	}

	clear(costOut)
	for t := range numPeriods {
		limbsAbsDiffSigned(sc.diffSeries, sorted[t*wS:(t+1)*wS], levelOut)
		limbsAddShifted(costOut, sc.diffSeries, spec.shiftSeriesToObj)
	}
	if limbsCmpSigned(levelOut, sc.pLimbs) != 0 {
		limbsAbsDiffSigned(sc.diffSeries, levelOut, sc.pLimbs)
		limbsAddMulU64Shifted(costOut, sc.diffSeries, spec.mWr, spec.shiftWrProdToObj, sc.mulTmp)
	}
}

func evalGroupExactDyadic(
	y []uint64,
	numPeriods int,
	spec dyadicSpec,
	costOut, levelOut []uint64,
	sc *dyadicScratch,
) int64 {
	wS := spec.wSeries
	copy(sc.sorted, y[:numPeriods*wS])
	sortDyadicSeries(sc.sorted, numPeriods, wS, sc.sortIdx, sc.sortTmp)

	uLo := sc.sorted[((numPeriods-1)/2)*wS : (((numPeriods-1)/2)+1)*wS]
	if limbsCmpSigned(uLo, sc.zeroSeries) < 0 {
		uLo = sc.zeroSeries
	}
	uHi := sc.sorted[(numPeriods/2)*wS : ((numPeriods/2)+1)*wS]
	if limbsCmpSigned(uHi, sc.zeroSeries) < 0 {
		uHi = sc.zeroSeries
	}

	if !spec.rounded {
		copy(levelOut, uLo)
		clear(costOut)
		for t := range numPeriods {
			limbsAbsDiffSigned(sc.diffSeries, sc.sorted[t*wS:(t+1)*wS], levelOut)
			limbsAddShifted(costOut, sc.diffSeries, spec.shiftSeriesToObj)
		}
		return 0
	}

	uLoF, _, _ := limbsMagToFloat64(uLo, spec.eM)
	nLo := max(int64(0), int64(math.Ceil(uLoF/spec.step)))
	limbsSetProdU64Scaled(sc.pLimbs, spec.mB, uint64(nLo), spec.shiftBToSeries)
	for limbsCmpSigned(sc.pLimbs, uLo) < 0 {
		nLo++
		limbsSetProdU64Scaled(sc.pLimbs, spec.mB, uint64(nLo), spec.shiftBToSeries)
	}
	for nLo > 0 {
		limbsSetProdU64Scaled(sc.pAltLimbs, spec.mB, uint64(nLo-1), spec.shiftBToSeries)
		if limbsCmpSigned(sc.pAltLimbs, uLo) < 0 {
			break
		}
		nLo--
		copy(sc.pLimbs, sc.pAltLimbs)
	}

	uHiF, _, _ := limbsMagToFloat64(uHi, spec.eM)
	nHi := max(int64(0), int64(math.Floor(uHiF/spec.step)))
	limbsSetProdU64Scaled(sc.pLimbs, spec.mB, uint64(nHi), spec.shiftBToSeries)
	for nHi > 0 && limbsCmpSigned(sc.pLimbs, uHi) > 0 {
		nHi--
		limbsSetProdU64Scaled(sc.pLimbs, spec.mB, uint64(nHi), spec.shiftBToSeries)
	}
	for {
		//nolint:gosec // G115: nHi >= 0 so nHi + 1 >= 1.
		limbsSetProdU64Scaled(sc.pAltLimbs, spec.mB, uint64(nHi+1), spec.shiftBToSeries)
		if limbsCmpSigned(sc.pAltLimbs, uHi) > 0 {
			break
		}
		nHi++
		copy(sc.pLimbs, sc.pAltLimbs)
	}

	if nLo < spec.nMin {
		nLo = spec.nMin
	}
	if nLo <= nHi {
		//nolint:gosec // G115: nLo >= 0.
		limbsSetProdU64Scaled(levelOut, spec.mB, uint64(nLo), spec.shiftBToSeries)
		clear(costOut)
		for t := range numPeriods {
			limbsAbsDiffSigned(sc.diffSeries, sc.sorted[t*wS:(t+1)*wS], levelOut)
			limbsAddShifted(costOut, sc.diffSeries, spec.shiftSeriesToObj)
		}
		return nLo
	}

	if nHi < spec.nMin {
		evalGroupDyadicForFixedN(sc.sorted, numPeriods, spec, spec.nMin, costOut, levelOut, sc)
		return spec.nMin
	}

	evalGroupDyadicForFixedN(sc.sorted, numPeriods, spec, nHi, sc.costBest, sc.mBestLimbs, sc)
	evalGroupDyadicForFixedN(sc.sorted, numPeriods, spec, nLo, sc.costCand, sc.mLimbs, sc)
	if limbsCmpUnsigned(sc.costCand, sc.costBest) < 0 {
		copy(costOut, sc.costCand)
		copy(levelOut, sc.mLimbs)
		return nLo
	}
	copy(costOut, sc.costBest)
	copy(levelOut, sc.mBestLimbs)
	return nHi
}

func evalAssignmentDyadicBudget(
	prob Problem,
	spec dyadicSpec,
	numGroups, numItems, numPeriods int,
	assignment []int,
	stepsOverride []int64,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (totalObj []uint64, levels []float64, steps []int64, err error) {
	wS := spec.wSeries
	wO := spec.wObj

	head := make([]int, numGroups)
	for g := range head {
		head[g] = -1
	}
	nextItem := make([]int, numItems)
	// Traverse in reverse so each group's linked list preserves ascending item order.
	for i := numItems - 1; i >= 0; i-- {
		g := assignment[i]
		nextItem[i] = head[g]
		head[g] = i
	}

	yBuf := make([]uint64, numPeriods*wS)
	tmpSeries := make([]uint64, wS)
	sc := newDyadicScratch(spec, numPeriods)
	totalObj = make([]uint64, wO)
	groupCost := make([]uint64, wO)
	groupLevel := make([]uint64, wS)
	levels = make([]float64, numGroups)
	steps = make([]int64, numGroups)
	var ops uint64

	evalGroupBuffer := func(g int) int64 {
		if stepsOverride != nil {
			stepCnt := stepsOverride[g]
			if !spec.rounded {
				return evalGroupExactDyadic(yBuf, numPeriods, spec, groupCost, groupLevel, sc)
			}
			copy(sc.sorted, yBuf)
			sortDyadicSeries(sc.sorted, numPeriods, wS, sc.sortIdx, sc.sortTmp)
			evalGroupDyadicForFixedN(sc.sorted, numPeriods, spec, stepCnt, groupCost, groupLevel, sc)
			return stepCnt
		}
		return evalGroupExactDyadic(yBuf, numPeriods, spec, groupCost, groupLevel, sc)
	}

	if prob.Baselines == nil {
		emptyStep := evalGroupExactDyadic(yBuf, numPeriods, spec, groupCost, groupLevel, sc)
		emptyCost := slices.Clone(groupCost)
		emptyLvlF, _, _ := limbsMagToFloat64(groupLevel, spec.eM)
		emptyLvlF = normZeroFloat64(emptyLvlF)

		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return nil, nil, nil, err
			}
			if head[g] < 0 {
				if stepsOverride == nil || stepsOverride[g] == emptyStep {
					levels[g] = emptyLvlF
					steps[g] = emptyStep
					limbsAddTo(totalObj, emptyCost)
					continue
				}
				clear(sc.sorted)
				evalGroupDyadicForFixedN(sc.sorted, numPeriods, spec, stepsOverride[g], groupCost, groupLevel, sc)
				fLvl, _, _ := limbsMagToFloat64(groupLevel, spec.eM)
				levels[g] = normZeroFloat64(fLvl)
				steps[g] = stepsOverride[g]
				limbsAddTo(totalObj, groupCost)
				continue
			}
			clear(yBuf)
			for i := head[g]; i >= 0; i = nextItem[i] {
				row := prob.Items[i]
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return nil, nil, nil, err
					}
					limbsAddFloatScaled(yBuf[t*wS:(t+1)*wS], row[t], spec.eM, tmpSeries)
				}
			}
			stepCnt := evalGroupBuffer(g)
			fLvl, _, _ := limbsMagToFloat64(groupLevel, spec.eM)
			levels[g] = normZeroFloat64(fLvl)
			steps[g] = stepCnt
			limbsAddTo(totalObj, groupCost)
		}
		return totalObj, levels, steps, nil
	}

	for g := range numGroups {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			return nil, nil, nil, err
		}
		row := prob.Baselines[g]
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return nil, nil, nil, err
			}
			limbsSetFloatScaled(yBuf[t*wS:(t+1)*wS], row[t], spec.eM)
		}
		for i := head[g]; i >= 0; i = nextItem[i] {
			iRow := prob.Items[i]
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return nil, nil, nil, err
				}
				limbsAddFloatScaled(yBuf[t*wS:(t+1)*wS], iRow[t], spec.eM, tmpSeries)
			}
		}
		stepCnt := evalGroupBuffer(g)
		fLvl, _, _ := limbsMagToFloat64(groupLevel, spec.eM)
		levels[g] = normZeroFloat64(fLvl)
		steps[g] = stepCnt
		limbsAddTo(totalObj, groupCost)
	}
	return totalObj, levels, steps, nil
}

// EvaluateAssignmentDyadic scores assignment in exact fixed-width dyadic arithmetic
// and returns the polished Result plus little-endian objective limbs scaled by 2^Exp.
func EvaluateAssignmentDyadic(prob Problem, assignment []int) (CertifiedWitnessEval, error) {
	numGroups, numItems, numPeriods, uMaxF, err := validateProblem(prob)
	if err != nil {
		return CertifiedWitnessEval{}, err
	}
	if err := validateAssignment(prob, numGroups, numItems, assignment); err != nil {
		return CertifiedWitnessEval{}, err
	}
	spec, err := buildDyadicSpec(prob, numGroups, numPeriods, uMaxF)
	if err != nil {
		return CertifiedWitnessEval{}, err
	}

	totalObj, levels, steps, evalErr := evalAssignmentDyadicBudget(
		prob, spec, numGroups, numItems, numPeriods, assignment, nil, time.Time{}, false, nil, nil,
	)
	if evalErr != nil {
		return CertifiedWitnessEval{}, evalErr
	}
	objFloat, _, _ := limbsMagToFloat64(totalObj, spec.eMin)
	boundFloat := limbsMagToFloat64Floor(totalObj, spec.eMin)

	return CertifiedWitnessEval{
		Result: Result{
			Assignment: slices.Clone(assignment),
			Levels:     levels,
			Steps:      steps,
			Objective:  normZeroFloat64(objFloat),
			BestBound:  normZeroFloat64(boundFloat),
			Status:     StatusOptimal,
		},
		Limbs: slices.Clone(totalObj),
		Exp:   spec.eMin,
	}, nil
}

func certifyTimeoutResult(
	prob Problem,
	spec dyadicSpec,
	hasSpec bool,
	uMaxF float64,
	valTimedOut bool,
	numGroups, numItems, numPeriods int,
	presetAssign []int,
	s Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	assign := presetAssign
	if assign == nil {
		if numItems == 0 {
			assign = []int{}
		} else {
			gRes, gErr := greedyIncumbentBudget(prob, numGroups, numItems, numPeriods, deadline, hasDeadline, done, ctxErr)
			if gErr != nil {
				if errors.Is(gErr, errTimeLimit) {
					gRes.Strategy = resolveTimeoutStrategy(s)
					gRes.SolveTime = time.Since(start)
					return gRes, nil
				}
				return Result{}, gErr
			}
			assign = gRes.Assignment
		}
	}
	emptyTimeout := func() Result {
		return Result{
			Assignment: slices.Clone(assign),
			Levels:     make([]float64, numGroups),
			Steps:      make([]int64, numGroups),
			BestBound:  0,
			Strategy:   resolveTimeoutStrategy(s),
			Status:     StatusTimeLimit,
			SolveTime:  time.Since(start),
		}
	}
	if valTimedOut {
		return emptyTimeout(), nil
	}
	var ops uint64
	if !hasSpec {
		var specErr error
		spec, specErr = buildDyadicSpecBudget(prob, numGroups, numPeriods, uMaxF, deadline, hasDeadline, done, ctxErr, &ops)
		if specErr != nil {
			if errors.Is(specErr, errTimeLimit) {
				return emptyTimeout(), nil
			}
			return Result{}, specErr
		}
	}
	totalObj, levels, steps, evalErr := evalAssignmentDyadicBudget(
		prob, spec, numGroups, numItems, numPeriods, assign, nil, deadline, hasDeadline, done, ctxErr,
	)
	if evalErr != nil {
		if errors.Is(evalErr, errTimeLimit) {
			return emptyTimeout(), nil
		}
		return Result{}, evalErr
	}
	objF, _, _ := limbsMagToFloat64(totalObj, spec.eMin)
	return Result{
		Assignment: slices.Clone(assign),
		Levels:     levels,
		Steps:      steps,
		Objective:  normZeroFloat64(objF),
		BestBound:  0,
		Strategy:   resolveTimeoutStrategy(s),
		Status:     StatusTimeLimit,
		SolveTime:  time.Since(start),
	}, nil
}

func certifyMAD(
	ctx context.Context,
	prob Problem,
	opt Options,
	info validatedProblem,
	start, deadline time.Time,
	hasDeadline, expired bool,
	done <-chan struct{},
) (Result, error) {
	ctxErr := ctxErrFn(ctx)
	if info.numGroups <= 0 || info.numPeriods <= 0 {
		if start.IsZero() {
			start = time.Now()
		}
		valHasDeadline := hasDeadline && opt.Strategy != StrategyMILP
		numGroups, numItems, numPeriods, uMaxF, err := validateProblemBudget(prob, deadline, valHasDeadline, done, ctxErr)
		if err != nil {
			if !errors.Is(err, errTimeLimit) {
				return Result{}, err
			}
			info.valTimedOut = true
			expired = true
		}
		info.numGroups = numGroups
		info.numItems = numItems
		info.numPeriods = numPeriods
		info.uMaxF = uMaxF
	}
	numGroups := info.numGroups
	numItems := info.numItems
	numPeriods := info.numPeriods
	uMaxF := info.uMaxF
	valTimedOut := info.valTimedOut

	if opt.Strategy == StrategyMILP {
		if opt.certFallback == nil {
			return Result{}, fmt.Errorf("%w: certified MILP fallback hook not installed", ErrCertificationFailed)
		}
		return opt.certFallback(ctx, prob, opt)
	}

	if expired && deadline.IsZero() {
		deadline = start
	}
	hasDeadline = hasDeadline || !deadline.IsZero() || expired
	if hasDeadline && !expired && !time.Now().Before(deadline) {
		expired = true
	}

	if expired {
		return certifyTimeoutResult(
			prob, dyadicSpec{}, false, uMaxF, valTimedOut, numGroups, numItems, numPeriods,
			nil, opt.Strategy, start, deadline, hasDeadline, done, ctxErr,
		)
	}

	var ops uint64
	var plan combPlan
	if numItems > 0 && numGroups > 1 {
		var planErr error
		plan, planErr = buildCombPlanBudget(prob, numGroups, numItems, numPeriods, deadline, hasDeadline, done, ctxErr, &ops)
		if planErr != nil {
			if errors.Is(planErr, errTimeLimit) {
				return certifyTimeoutResult(
					prob, dyadicSpec{}, false, uMaxF, false, numGroups, numItems, numPeriods,
					nil, opt.Strategy, start, deadline, hasDeadline, done, ctxErr,
				)
			}
			return Result{}, planErr
		}
		dpAutoBlocked := plan.dpOverCeiling || plan.dpBits > MaxDPItems || plan.dpWork > MaxDPWork || plan.dpBytes > MaxDPBytes
		dpHardBlocked := plan.dpOverCeiling || plan.dpWork > MaxDPWork || plan.dpBytes > MaxDPBytes

		if opt.Strategy == StrategyAuto && dpAutoBlocked {
			if opt.certFallback != nil {
				return opt.certFallback(ctx, prob, opt)
			}
			if opt.fallback != nil {
				return Result{}, fmt.Errorf("%w: certified MILP fallback hook not installed", ErrCertificationFailed)
			}
			return Result{}, ErrCombinatorialLimit
		}
		if (opt.Strategy == StrategyDP || opt.Strategy == StrategyBruteForce) && dpHardBlocked {
			return Result{}, ErrCombinatorialLimit
		}
	}

	spec, err := buildDyadicSpecBudget(prob, numGroups, numPeriods, uMaxF, deadline, hasDeadline, done, ctxErr, &ops)
	if err != nil {
		if errors.Is(err, errTimeLimit) {
			return certifyTimeoutResult(
				prob, dyadicSpec{}, false, uMaxF, true, numGroups, numItems, numPeriods,
				nil, opt.Strategy, start, deadline, hasDeadline, done, ctxErr,
			)
		}
		return Result{}, err
	}

	if numItems > 0 && numGroups > 1 {
		// Account for exact W-limb storage alongside valid flags, uint32 backpointers, and Result slices.
		numMasks := int64(1) << uint(plan.dpBits)
		wordBytes := int64(spec.wObj * 8)
		dpAndNextBytes := satMul(satMul(2, numMasks), wordBytes+1)
		storedBytes := satMul(plan.storedSubsets, wordBytes)
		parentBytes := satMul(satMul(plan.activeGroups, numMasks), 4)
		resultBytes := satMul(int64(numGroups), 16)
		dpBytesScaled := satAdd(resultBytes, satAdd(dpAndNextBytes, satAdd(storedBytes, parentBytes)))
		if dpBytesScaled > MaxDPBytes {
			if opt.Strategy == StrategyAuto {
				if opt.certFallback != nil {
					return opt.certFallback(ctx, prob, opt)
				}
				if opt.fallback != nil {
					return Result{}, fmt.Errorf("%w: certified MILP fallback hook not installed", ErrCertificationFailed)
				}
			}
			return Result{}, ErrCombinatorialLimit
		}
	}

	var res Result
	var optObjLimbs []uint64
	var solveErr error
	reportStrategy := resolveFastPathStrategy(opt.Strategy)

	switch {
	case numItems == 0:
		optObjLimbs, res, solveErr = certifyZeroItems(prob, spec, numGroups, numPeriods, reportStrategy, start, deadline, hasDeadline, done, ctxErr)
		if errors.Is(solveErr, errTimeLimit) {
			return zeroTimeoutResult(numGroups, opt.Strategy, start), nil
		}
		if solveErr != nil && ctx != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
			return Result{}, solveErr
		}
	case numGroups == 1:
		optObjLimbs, res, solveErr = certifySingleGroup(prob, spec, numItems, numPeriods, reportStrategy, start, deadline, hasDeadline, done, ctxErr)
		if errors.Is(solveErr, errTimeLimit) {
			return Result{
				Assignment: make([]int, numItems),
				Levels:     make([]float64, 1),
				Steps:      make([]int64, 1),
				BestBound:  0,
				Strategy:   resolveTimeoutStrategy(opt.Strategy),
				Status:     StatusTimeLimit,
				SolveTime:  time.Since(start),
			}, nil
		}
		if solveErr != nil && ctx != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
			return Result{}, solveErr
		}
	default:
		optObjLimbs, res, solveErr = certifyDP(ctx, prob, spec, plan, start, deadline, hasDeadline, done)
		if errors.Is(solveErr, errTimeLimit) {
			return certifyTimeoutResult(
				prob, spec, true, uMaxF, false, numGroups, numItems, numPeriods,
				res.Assignment, StrategyDP, start, deadline, hasDeadline, done, ctxErr,
			)
		}
		if solveErr != nil && ctx != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
			return Result{}, solveErr
		}
	}

	if ctx != nil {
		if fn, ok := ctx.Value(certFaultKey{}).(madFaultFunc); ok && fn != nil {
			fn(prob, &res, &solveErr)
		}
	}

	if err := verifyCertifiedResult(prob, spec, numGroups, numItems, numPeriods, optObjLimbs, res, solveErr, deadline, hasDeadline, done, ctxErr); err != nil {
		if errors.Is(err, errTimeLimit) {
			return certifyTimeoutResult(
				prob, spec, true, uMaxF, false, numGroups, numItems, numPeriods,
				res.Assignment, opt.Strategy, start, deadline, hasDeadline, done, ctxErr,
			)
		}
		if ctx != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return Result{}, err
		}
		return Result{}, err
	}
	return res, nil
}

func certifyZeroItems(
	prob Problem,
	spec dyadicSpec,
	numGroups, numPeriods int,
	reportStrategy Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) ([]uint64, Result, error) {
	assign := []int{}
	totalObj, levels, steps, err := evalAssignmentDyadicBudget(prob, spec, numGroups, 0, numPeriods, assign, nil, deadline, hasDeadline, done, ctxErr)
	if err != nil {
		return nil, Result{}, err
	}
	objF, _, _ := limbsMagToFloat64(totalObj, spec.eMin)
	boundF := limbsMagToFloat64Floor(totalObj, spec.eMin)
	return totalObj, Result{
		Assignment: assign,
		Levels:     levels,
		Steps:      steps,
		Objective:  normZeroFloat64(objF),
		BestBound:  normZeroFloat64(boundF),
		SolveTime:  time.Since(start),
		Strategy:   reportStrategy,
		Status:     StatusOptimal,
	}, nil
}

func certifySingleGroup(
	prob Problem,
	spec dyadicSpec,
	numItems, numPeriods int,
	reportStrategy Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) ([]uint64, Result, error) {
	assign := make([]int, numItems)
	totalObj, levels, steps, err := evalAssignmentDyadicBudget(prob, spec, 1, numItems, numPeriods, assign, nil, deadline, hasDeadline, done, ctxErr)
	if err != nil {
		return nil, Result{}, err
	}
	objF, _, _ := limbsMagToFloat64(totalObj, spec.eMin)
	boundF := limbsMagToFloat64Floor(totalObj, spec.eMin)
	return totalObj, Result{
		Assignment: assign,
		Levels:     levels,
		Steps:      steps,
		Objective:  normZeroFloat64(objF),
		BestBound:  normZeroFloat64(boundF),
		SolveTime:  time.Since(start),
		Strategy:   reportStrategy,
		Status:     StatusOptimal,
	}, nil
}

type dyadicStreamCand struct {
	deltaOff int
	costOff  int
	group    int
}

func evalSubsetCostDyadic(
	baseline []float64,
	items [][]float64,
	itemSubset []int,
	mask int,
	spec dyadicSpec,
	yBuf, tmpSeries, costOut, levelOut []uint64,
	sc *dyadicScratch,
) {
	numPeriods := len(yBuf) / spec.wSeries
	wS := spec.wSeries
	if baseline != nil {
		for t := range numPeriods {
			limbsSetFloatScaled(yBuf[t*wS:(t+1)*wS], baseline[t], spec.eM)
		}
	} else {
		clear(yBuf)
	}
	for m := uint(mask); m != 0; {
		bit := bits.TrailingZeros(m)
		m &= m - 1
		row := items[itemSubset[bit]]
		for t := range numPeriods {
			limbsAddFloatScaled(yBuf[t*wS:(t+1)*wS], row[t], spec.eM, tmpSeries)
		}
	}
	evalGroupExactDyadic(yBuf, numPeriods, spec, costOut, levelOut, sc)
}

func certifyDP(
	ctx context.Context,
	prob Problem,
	spec dyadicSpec,
	plan combPlan,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
) ([]uint64, Result, error) {
	numGroups := plan.numGroups
	numItems := plan.numItems
	numPeriods := plan.numPeriods
	wS := spec.wSeries
	wO := spec.wObj
	numMasks := 1 << uint(plan.dpBits)
	fullMask := numMasks - 1

	yBuf := make([]uint64, numPeriods*wS)
	tmpSeries := make([]uint64, wS)
	levelScratch := make([]uint64, wS)
	sc := newDyadicScratch(spec, numPeriods)
	var ops uint64
	ctxErr := ctxErrFn(ctx)

	if plan.hasD && !plan.canPinD {
		return certifyDPDesignatedDistinct(ctx, prob, spec, plan, start, deadline, hasDeadline, done, yBuf, tmpSeries, levelScratch, sc, &ops)
	}

	dpArena := make([]uint64, numMasks*wO)
	nextArena := make([]uint64, numMasks*wO)
	dpValid := make([]bool, numMasks)
	nextValid := make([]bool, numMasks)
	dpValid[0] = true

	candSum := make([]uint64, wO)
	allItemsIdx := make([]int, numItems)
	for i := range numItems {
		allItemsIdx[i] = i
	}

	if !plan.hasD && plan.bp.uCount == 1 {
		effG := min(numGroups, numItems)
		var baseRow []float64
		if prob.Baselines != nil {
			baseRow = prob.Baselines[0]
		}
		subCostArena := make([]uint64, numMasks*wO)
		for s := range numMasks {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return nil, Result{}, err
			}
			evalSubsetCostDyadic(baseRow, prob.Items, allItemsIdx, s, spec, yBuf, tmpSeries, subCostArena[s*wO:(s+1)*wO], levelScratch, sc)
		}

		parent := make([]uint32, effG*numMasks)
		emptyCost := subCostArena[:wO]
		emptyZero := limbsIsZero(emptyCost)

		for g := range effG {
			if emptyZero {
				copy(nextArena, dpArena)
				copy(nextValid, dpValid)
			} else {
				for m := range numMasks {
					nextValid[m] = dpValid[m]
					if dpValid[m] {
						limbsAdd(nextArena[m*wO:(m+1)*wO], dpArena[m*wO:(m+1)*wO], emptyCost)
					}
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				cSlice := subCostArena[s*wO : (s+1)*wO]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return nil, Result{}, err
					}
					if dpValid[prev] {
						limbsAdd(candSum, dpArena[prev*wO:(prev+1)*wO], cSlice)
						m := prev | s
						if !nextValid[m] || limbsCmpUnsigned(candSum, nextArena[m*wO:(m+1)*wO]) < 0 {
							nextValid[m] = true
							copy(nextArena[m*wO:(m+1)*wO], candSum)
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dpArena, nextArena = nextArena, dpArena
			dpValid, nextValid = nextValid, dpValid
		}

		assign := make([]int, numItems)
		mask := fullMask
		for g := effG - 1; g >= 0; g-- {
			s := int(parent[g*numMasks+mask])
			for m := uint(s); m != 0; {
				bit := bits.TrailingZeros(m)
				m &= m - 1
				assign[bit] = g
			}
			mask ^= s
		}
		return finishCertifiedDP(prob, spec, numGroups, numItems, numPeriods, assign, start, deadline, hasDeadline, done, ctxErr)
	}

	if plan.canPinD {
		var baseRow []float64
		if prob.Baselines != nil {
			baseRow = prob.Baselines[0]
		}
		pinY := make([]uint64, numPeriods*wS)
		gCostArena := make([]uint64, numMasks*wO)
		parent := make([]uint32, numGroups*numMasks)

		for g := range numGroups {
			dRow := prob.Items[prob.DesignatedItems[g]]
			if baseRow != nil {
				for t := range numPeriods {
					limbsSetFloatScaled(pinY[t*wS:(t+1)*wS], baseRow[t], spec.eM)
					limbsAddFloatScaled(pinY[t*wS:(t+1)*wS], dRow[t], spec.eM, tmpSeries)
				}
			} else {
				for t := range numPeriods {
					limbsSetFloatScaled(pinY[t*wS:(t+1)*wS], dRow[t], spec.eM)
				}
			}
			for s := range numMasks {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return nil, Result{}, err
				}
				copy(yBuf, pinY)
				for m := uint(s); m != 0; {
					bit := bits.TrailingZeros(m)
					m &= m - 1
					row := prob.Items[plan.freeItems[bit]]
					for t := range numPeriods {
						limbsAddFloatScaled(yBuf[t*wS:(t+1)*wS], row[t], spec.eM, tmpSeries)
					}
				}
				evalGroupExactDyadic(yBuf, numPeriods, spec, gCostArena[s*wO:(s+1)*wO], levelScratch, sc)
			}

			emptyCost := gCostArena[:wO]
			for m := range numMasks {
				nextValid[m] = dpValid[m]
				if dpValid[m] {
					limbsAdd(nextArena[m*wO:(m+1)*wO], dpArena[m*wO:(m+1)*wO], emptyCost)
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				cSlice := gCostArena[s*wO : (s+1)*wO]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return nil, Result{}, err
					}
					if dpValid[prev] {
						limbsAdd(candSum, dpArena[prev*wO:(prev+1)*wO], cSlice)
						m := prev | s
						if !nextValid[m] || limbsCmpUnsigned(candSum, nextArena[m*wO:(m+1)*wO]) < 0 {
							nextValid[m] = true
							copy(nextArena[m*wO:(m+1)*wO], candSum)
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dpArena, nextArena = nextArena, dpArena
			dpValid, nextValid = nextValid, dpValid
		}

		assign := make([]int, numItems)
		for g, item := range prob.DesignatedItems {
			assign[item] = g
		}
		mask := fullMask
		for g := numGroups - 1; g >= 0; g-- {
			s := int(parent[g*numMasks+mask])
			for m := uint(s); m != 0; {
				bit := bits.TrailingZeros(m)
				m &= m - 1
				assign[plan.freeItems[bit]] = g
			}
			mask ^= s
		}
		return finishCertifiedDP(prob, spec, numGroups, numItems, numPeriods, assign, start, deadline, hasDeadline, done, ctxErr)
	}

	if !plan.streamingDP {
		uniqueCostArena := make([]uint64, plan.bp.uCount*numMasks*wO)
		for u, rep := range plan.bp.uniqueRep {
			baseRow := prob.Baselines[rep]
			uOff := u * numMasks
			for s := range numMasks {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return nil, Result{}, err
				}
				idx := (uOff + s) * wO
				evalSubsetCostDyadic(baseRow, prob.Items, allItemsIdx, s, spec, yBuf, tmpSeries, uniqueCostArena[idx:idx+wO], levelScratch, sc)
			}
		}

		parent := make([]uint32, numGroups*numMasks)
		for g := range numGroups {
			uOff := plan.bp.groupToUnique[g] * numMasks
			emptyCost := uniqueCostArena[uOff*wO : (uOff+1)*wO]
			for m := range numMasks {
				nextValid[m] = dpValid[m]
				if dpValid[m] {
					limbsAdd(nextArena[m*wO:(m+1)*wO], dpArena[m*wO:(m+1)*wO], emptyCost)
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				cIdx := (uOff + s) * wO
				cSlice := uniqueCostArena[cIdx : cIdx+wO]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return nil, Result{}, err
					}
					if dpValid[prev] {
						limbsAdd(candSum, dpArena[prev*wO:(prev+1)*wO], cSlice)
						m := prev | s
						if !nextValid[m] || limbsCmpUnsigned(candSum, nextArena[m*wO:(m+1)*wO]) < 0 {
							nextValid[m] = true
							copy(nextArena[m*wO:(m+1)*wO], candSum)
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dpArena, nextArena = nextArena, dpArena
			dpValid, nextValid = nextValid, dpValid
		}

		assign := make([]int, numItems)
		mask := fullMask
		for g := numGroups - 1; g >= 0; g-- {
			s := int(parent[g*numMasks+mask])
			for m := uint(s); m != 0; {
				bit := bits.TrailingZeros(m)
				m &= m - 1
				assign[bit] = g
			}
			mask ^= s
		}
		return finishCertifiedDP(prob, spec, numGroups, numItems, numPeriods, assign, start, deadline, hasDeadline, done, ctxErr)
	}

	// Streaming top-(I - |S| + 1) dyadic DP when !hasD && U > 1 && G > I + 1.
	emptyCostArena := make([]uint64, plan.bp.uCount*wO)
	for u, rep := range plan.bp.uniqueRep {
		evalSubsetCostDyadic(prob.Baselines[rep], prob.Items, allItemsIdx, 0, spec, yBuf, tmpSeries, emptyCostArena[u*wO:(u+1)*wO], levelScratch, sc)
	}

	candSub := make([][]int, numGroups)
	candCostOff := make([][]int, numGroups)
	keptCostArena := make([]uint64, 0, numMasks*wO)
	uCostScratch := make([]uint64, plan.bp.uCount*wO)
	uDeltaScratch := make([]uint64, plan.bp.uCount*wO)
	heap := make([]dyadicStreamCand, 0, numItems+1)

	candGreater := func(a, b dyadicStreamCand) bool {
		cmp := limbsCmpSigned(uDeltaScratch[a.deltaOff:a.deltaOff+wO], uDeltaScratch[b.deltaOff:b.deltaOff+wO])
		if cmp != 0 {
			return cmp > 0
		}
		return a.group > b.group
	}

	for s := 1; s < numMasks; s++ {
		kCap := min(numGroups, numItems-bits.OnesCount(uint(s))+1)
		heap = heap[:0]
		for u, rep := range plan.bp.uniqueRep {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return nil, Result{}, err
			}
			cOff := u * wO
			evalSubsetCostDyadic(prob.Baselines[rep], prob.Items, allItemsIdx, s, spec, yBuf, tmpSeries, uCostScratch[cOff:cOff+wO], levelScratch, sc)
			limbsSub(uDeltaScratch[cOff:cOff+wO], uCostScratch[cOff:cOff+wO], emptyCostArena[cOff:cOff+wO])
			for _, g := range plan.bp.uniqueGroups[u] {
				item := dyadicStreamCand{deltaOff: cOff, costOff: cOff, group: g}
				if len(heap) < kCap {
					heap = append(heap, item)
					idx := len(heap) - 1
					for idx > 0 {
						p := (idx - 1) / 2
						if !candGreater(heap[idx], heap[p]) {
							break
						}
						heap[idx], heap[p] = heap[p], heap[idx]
						idx = p
					}
					continue
				}
				if !candGreater(heap[0], item) {
					break
				}
				heap[0] = item
				idx := 0
				n := len(heap)
				for {
					left := 2*idx + 1
					if left >= n {
						break
					}
					largest := left
					right := left + 1
					if right < n && candGreater(heap[right], heap[left]) {
						largest = right
					}
					if !candGreater(heap[largest], heap[idx]) {
						break
					}
					heap[idx], heap[largest] = heap[largest], heap[idx]
					idx = largest
				}
			}
		}
		for _, item := range heap {
			off := len(keptCostArena)
			keptCostArena = append(keptCostArena, uCostScratch[item.costOff:item.costOff+wO]...)
			candSub[item.group] = append(candSub[item.group], s)
			candCostOff[item.group] = append(candCostOff[item.group], off)
		}
	}

	activeGroups := make([]int, 0, min(numGroups, numItems*(1<<min(numItems, 4))))
	for g := range numGroups {
		if len(candSub[g]) > 0 {
			activeGroups = append(activeGroups, g)
		}
	}

	parent := make([]uint32, len(activeGroups)*numMasks)
	for ai, g := range activeGroups {
		u := plan.bp.groupToUnique[g]
		emptyCost := emptyCostArena[u*wO : (u+1)*wO]
		for m := range numMasks {
			nextValid[m] = dpValid[m]
			if dpValid[m] {
				limbsAdd(nextArena[m*wO:(m+1)*wO], dpArena[m*wO:(m+1)*wO], emptyCost)
			}
		}
		rowOff := ai * numMasks
		subs := candSub[g]
		offs := candCostOff[g]
		for idx, s := range subs {
			cOff := offs[idx]
			cSlice := keptCostArena[cOff : cOff+wO]
			comp := fullMask ^ s
			for prev := comp; ; prev = (prev - 1) & comp {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return nil, Result{}, err
				}
				if dpValid[prev] {
					limbsAdd(candSum, dpArena[prev*wO:(prev+1)*wO], cSlice)
					m := prev | s
					if !nextValid[m] || limbsCmpUnsigned(candSum, nextArena[m*wO:(m+1)*wO]) < 0 {
						nextValid[m] = true
						copy(nextArena[m*wO:(m+1)*wO], candSum)
						//nolint:gosec // G115: s is bounded by 2^MaxDPCeiling, fits in uint32.
						parent[rowOff+m] = uint32(s)
					}
				}
				if prev == 0 {
					break
				}
			}
		}
		dpArena, nextArena = nextArena, dpArena
		dpValid, nextValid = nextValid, dpValid
	}

	assign := make([]int, numItems)
	mask := fullMask
	for ai, g := range slices.Backward(activeGroups) {
		s := int(parent[ai*numMasks+mask])
		for m := uint(s); m != 0; {
			bit := bits.TrailingZeros(m)
			m &= m - 1
			assign[bit] = g
		}
		mask ^= s
	}
	return finishCertifiedDP(prob, spec, numGroups, numItems, numPeriods, assign, start, deadline, hasDeadline, done, ctxErr)
}

func certifyDPDesignatedDistinct(
	ctx context.Context,
	prob Problem,
	spec dyadicSpec,
	plan combPlan,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	yBuf, tmpSeries, levelScratch []uint64,
	sc *dyadicScratch,
	ops *uint64,
) ([]uint64, Result, error) {
	numGroups := plan.numGroups
	numItems := plan.numItems
	numPeriods := plan.numPeriods
	wO := spec.wObj
	numMasks := 1 << uint(numItems)
	freeCount := numItems - numGroups
	numFreeMasks := 1 << uint(freeCount)
	fullFreeMask := numFreeMasks - 1

	expandedFree := make([]int, numFreeMasks)
	for f := 1; f < numFreeMasks; f++ {
		lsb := bits.TrailingZeros(uint(f))
		expandedFree[f] = expandedFree[f^(1<<lsb)] | (1 << plan.freeItems[lsb])
	}

	dBits := make([]int, numGroups)
	for g, idx := range prob.DesignatedItems {
		dBits[g] = 1 << idx
	}

	strideD := numFreeMasks
	strideU := numGroups * strideD
	uCostArena := make([]uint64, plan.bp.uCount*strideU*wO)
	allItemsIdx := make([]int, numItems)
	for i := range numItems {
		allItemsIdx[i] = i
	}
	for u, rep := range plan.bp.uniqueRep {
		baseRow := prob.Baselines[rep]
		for dIdx := range numGroups {
			dBit := dBits[dIdx]
			for freeS := range numFreeMasks {
				if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, ops); err != nil {
					return nil, Result{}, err
				}
				s := dBit | expandedFree[freeS]
				idx := (u*strideU + dIdx*strideD + freeS) * wO
				evalSubsetCostDyadic(baseRow, prob.Items, allItemsIdx, s, spec, yBuf, tmpSeries, uCostArena[idx:idx+wO], levelScratch, sc)
			}
		}
	}

	numDMasks := 1 << uint(numGroups)
	dMasksByCard := make([][]int, numGroups+1)
	dMaskExpanded := make([]int, numDMasks)
	for dm := 1; dm < numDMasks; dm++ {
		lsb := bits.TrailingZeros(uint(dm))
		dMaskExpanded[dm] = dMaskExpanded[dm^(1<<lsb)] | dBits[lsb]
	}
	for dm := range numDMasks {
		c := bits.OnesCount(uint(dm))
		dMasksByCard[c] = append(dMasksByCard[c], dm)
	}

	dpArena := make([]uint64, numMasks*wO)
	nextArena := make([]uint64, numMasks*wO)
	dpValid := make([]bool, numMasks)
	nextValid := make([]bool, numMasks)
	dpValid[0] = true
	candSum := make([]uint64, wO)
	parent := make([]uint32, numGroups*numMasks)

	for g := range numGroups {
		clear(nextValid)
		uOff := plan.bp.groupToUnique[g] * strideU
		rowOff := g * numMasks
		dPrevList := dMasksByCard[g]

		for dIdx := range numGroups {
			dBit := dBits[dIdx]
			dBitFlag := 1 << dIdx
			costOff := uOff + dIdx*strideD
			for freeS := range numFreeMasks {
				cIdx := (costOff + freeS) * wO
				cSlice := uCostArena[cIdx : cIdx+wO]
				s := dBit | expandedFree[freeS]
				compFree := fullFreeMask ^ freeS
				for _, dm := range dPrevList {
					if dm&dBitFlag != 0 {
						continue
					}
					dExp := dMaskExpanded[dm]
					for prevFree := compFree; ; prevFree = (prevFree - 1) & compFree {
						if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, ops); err != nil {
							return nil, Result{}, err
						}
						prev := dExp | expandedFree[prevFree]
						if dpValid[prev] {
							limbsAdd(candSum, dpArena[prev*wO:(prev+1)*wO], cSlice)
							m := prev | s
							if !nextValid[m] || limbsCmpUnsigned(candSum, nextArena[m*wO:(m+1)*wO]) < 0 {
								nextValid[m] = true
								copy(nextArena[m*wO:(m+1)*wO], candSum)
								//nolint:gosec // G115: s is bounded by 2^MaxDPCeiling, fits in uint32.
								parent[rowOff+m] = uint32(s)
							}
						}
						if prevFree == 0 {
							break
						}
					}
				}
			}
		}
		dpArena, nextArena = nextArena, dpArena
		dpValid, nextValid = nextValid, dpValid
	}

	assign := make([]int, numItems)
	mask := numMasks - 1
	for g := numGroups - 1; g >= 0; g-- {
		s := int(parent[g*numMasks+mask])
		for m := uint(s); m != 0; {
			bit := bits.TrailingZeros(m)
			m &= m - 1
			assign[bit] = g
		}
		mask ^= s
	}
	return finishCertifiedDP(prob, spec, numGroups, numItems, numPeriods, assign, start, deadline, hasDeadline, done, ctxErrFn(ctx))
}

func finishCertifiedDP(
	prob Problem,
	spec dyadicSpec,
	numGroups, numItems, numPeriods int,
	assign []int,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) ([]uint64, Result, error) {
	totalObj, levels, steps, err := evalAssignmentDyadicBudget(
		prob, spec, numGroups, numItems, numPeriods, assign, nil, deadline, hasDeadline, done, ctxErr,
	)
	if err != nil {
		return nil, Result{Assignment: assign}, err
	}
	objF, _, _ := limbsMagToFloat64(totalObj, spec.eMin)
	boundF := limbsMagToFloat64Floor(totalObj, spec.eMin)
	return totalObj, Result{
		Assignment: assign,
		Levels:     levels,
		Steps:      steps,
		Objective:  normZeroFloat64(objF),
		BestBound:  normZeroFloat64(boundF),
		SolveTime:  time.Since(start),
		Strategy:   StrategyDP,
		Status:     StatusOptimal,
	}, nil
}

func verifyCertifiedResult(
	prob Problem,
	spec dyadicSpec,
	numGroups, numItems, numPeriods int,
	optObjLimbs []uint64,
	res Result,
	solveErr error,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) error {
	if solveErr != nil {
		return fmt.Errorf("%w: %w", ErrCertificationFailed, solveErr)
	}
	if res.Status != StatusOptimal {
		return fmt.Errorf("%w: non-optimal status %v", ErrCertificationFailed, res.Status)
	}
	if len(res.Assignment) != numItems || len(res.Levels) != numGroups || len(res.Steps) != numGroups {
		return fmt.Errorf("%w: witness slice length mismatch", ErrCertificationFailed)
	}
	if err := validateAssignment(prob, numGroups, numItems, res.Assignment); err != nil {
		return fmt.Errorf("%w: %w", ErrCertificationFailed, err)
	}
	var ops uint64
	for g, st := range res.Steps {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			return err
		}
		if !spec.rounded {
			if st != 0 {
				return fmt.Errorf("%w: unrounded step[%d]=%d must be 0", ErrCertificationFailed, g, st)
			}
			continue
		}
		if st < spec.nMin || float64(st) > maxStepCount+2 {
			return fmt.Errorf("%w: step[%d]=%d out of feasible bounds", ErrCertificationFailed, g, st)
		}
	}

	evalObj, evalLevels, _, evalErr := evalAssignmentDyadicBudget(
		prob, spec, numGroups, numItems, numPeriods, res.Assignment, res.Steps, deadline, hasDeadline, done, ctxErr,
	)
	if evalErr != nil {
		return evalErr
	}
	if limbsCmpUnsigned(evalObj, optObjLimbs) != 0 {
		return fmt.Errorf("%w: witness objective differs from exact optimum", ErrCertificationFailed)
	}
	for g := range numGroups {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			return err
		}
		if math.Float64bits(res.Levels[g]) != math.Float64bits(evalLevels[g]) {
			return fmt.Errorf("%w: witness level[%d] mismatch", ErrCertificationFailed, g)
		}
	}

	wantObjF, _, _ := limbsMagToFloat64(optObjLimbs, spec.eMin)
	wantObjF = normZeroFloat64(wantObjF)
	if math.Float64bits(res.Objective) != math.Float64bits(wantObjF) {
		return fmt.Errorf("%w: witness objective float64 mismatch", ErrCertificationFailed)
	}

	wantBoundF := normZeroFloat64(limbsMagToFloat64Floor(optObjLimbs, spec.eMin))
	if math.Float64bits(res.BestBound) != math.Float64bits(wantBoundF) {
		return fmt.Errorf("%w: witness BestBound mismatch", ErrCertificationFailed)
	}
	return nil
}
