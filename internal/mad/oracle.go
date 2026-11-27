package mad

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const (
	maxProblemScale  = 1e15
	maxObjScale      = 1e300
	maxStepCount     = float64(uint64(1) << 52)
	maxProblemGroups = 1 << 24
)

type dd struct {
	hi, lo float64
}

func twoSum(a, b float64) dd {
	// Explicit float64 casts enforce IEEE-754 rounding boundaries so FMA
	// fusion cannot merge adjacent ops and break the error-free transform.
	a = float64(a)
	b = float64(b)
	s := float64(a + b)
	bb := float64(s - a)
	err := float64(float64(a-float64(s-bb)) + float64(b-bb))
	return dd{hi: s, lo: err}
}

func twoProd(a, b float64) dd {
	p := float64(a * b)
	err := math.FMA(a, b, -p)
	return dd{hi: p, lo: err}
}

func ddAdd(a, b dd) dd {
	s := twoSum(a.hi, b.hi)
	t := twoSum(a.lo, b.lo)
	c := float64(s.lo + t.hi)
	v := twoSum(s.hi, c)
	w := float64(t.lo + v.lo)
	return twoSum(v.hi, w)
}

func ddNeg(a dd) dd {
	return dd{hi: -a.hi, lo: -a.lo}
}

func ddSub(a, b dd) dd {
	return ddAdd(a, ddNeg(b))
}

func ddAbs(a dd) dd {
	if a.hi < 0 || (a.hi == 0 && a.lo < 0) {
		return ddNeg(a)
	}
	return a
}

func ddCmp(a, b dd) int {
	if a.hi < b.hi {
		return -1
	}
	if a.hi > b.hi {
		return 1
	}
	if a.lo < b.lo {
		return -1
	}
	if a.lo > b.lo {
		return 1
	}
	return 0
}

func ddMulFloat(a dd, b float64) dd {
	p := twoProd(a.hi, b)
	t := twoProd(a.lo, b)
	c := twoSum(p.lo, t.hi)
	v := twoSum(p.hi, c.hi)
	return twoSum(v.hi, float64(v.lo+float64(c.lo+t.lo)))
}

func isFiniteDD(a dd) bool {
	return !math.IsNaN(a.hi) && !math.IsInf(a.hi, 0) && !math.IsNaN(a.lo) && !math.IsInf(a.lo, 0)
}

func normZeroFloat64(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}

func ddToFloat64(a dd) float64 {
	return normZeroFloat64(twoSum(a.hi, a.lo).hi)
}

func ddToFloat64Floor(a dd) float64 {
	s := twoSum(a.hi, a.lo)
	if math.IsInf(s.hi, 1) {
		return math.MaxFloat64
	}
	if s.lo < 0 {
		s.hi = math.Nextafter(s.hi, math.Inf(-1))
	}
	return normZeroFloat64(s.hi)
}

func ddToFloat64Ceil(a dd) float64 {
	s := twoSum(a.hi, a.lo)
	if s.lo > 0 {
		s.hi = math.Nextafter(s.hi, math.Inf(1))
	}
	return normZeroFloat64(s.hi)
}

func validateScalarParam(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxProblemScale {
		return fmt.Errorf("%w: invalid %s %g", ErrInvalidProblem, name, v)
	}
	return nil
}

func validateProblem(prob Problem) (numGroups, numItems, numPeriods int, uMaxF float64, err error) {
	return validateProblemBudget(prob, time.Time{}, false, nil, nil)
}

func validateProblemBudget(
	prob Problem,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (numGroups, numItems, numPeriods int, uMaxF float64, err error) {
	if err := validateScalarParam("Step", prob.Step); err != nil {
		return 0, 0, 0, 0, err
	}
	if err := validateScalarParam("RoundWeight", prob.RoundWeight); err != nil {
		return 0, 0, 0, 0, err
	}

	const (
		expMask64 = uint64(0x7ff) << 52
		absMask64 = (uint64(1) << 63) - 1
	)

	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
		if numGroups < 1 {
			return 0, 0, 0, 0, fmt.Errorf("%w: Baselines must have at least 1 group", ErrInvalidProblem)
		}
		if numGroups > maxProblemGroups {
			return 0, 0, 0, 0, fmt.Errorf("%w: Groups (%d) exceeds maximum %d", ErrInvalidProblem, numGroups, maxProblemGroups)
		}
		if prob.Groups != 0 && prob.Groups != numGroups {
			return 0, 0, 0, 0, fmt.Errorf("%w: Groups (%d) does not match len(Baselines) (%d)", ErrInvalidProblem, prob.Groups, numGroups)
		}
		numPeriods = len(prob.Baselines[0])
		if numPeriods < 1 {
			return 0, 0, 0, 0, fmt.Errorf("%w: Baselines must have at least 1 period", ErrInvalidProblem)
		}
		if numGroups > math.MaxInt/numPeriods {
			return 0, 0, 0, 0, fmt.Errorf("%w: group*period dimension (%d*%d) overflows int", ErrInvalidProblem, numGroups, numPeriods)
		}
		for g := range numGroups {
			if len(prob.Baselines[g]) != numPeriods {
				return 0, 0, 0, 0, fmt.Errorf("%w: ragged baseline row %d has length %d, want %d", ErrInvalidProblem, g, len(prob.Baselines[g]), numPeriods)
			}
		}
	} else {
		numGroups = prob.Groups
		if numGroups < 1 {
			return 0, 0, 0, 0, fmt.Errorf("%w: Groups must be at least 1", ErrInvalidProblem)
		}
		if numGroups > maxProblemGroups {
			return 0, 0, 0, 0, fmt.Errorf("%w: Groups (%d) exceeds maximum %d", ErrInvalidProblem, numGroups, maxProblemGroups)
		}
	}

	numItems = len(prob.Items)
	if prob.Baselines == nil && numItems == 0 {
		return 0, 0, 0, 0, fmt.Errorf("%w: empty problem with nil Baselines and zero Items", ErrInvalidProblem)
	}
	if numItems > 0 {
		if prob.Baselines == nil {
			numPeriods = len(prob.Items[0])
			if numPeriods < 1 {
				return 0, 0, 0, 0, fmt.Errorf("%w: Items must have at least 1 period", ErrInvalidProblem)
			}
			if numGroups > math.MaxInt/numPeriods {
				return 0, 0, 0, 0, fmt.Errorf("%w: group*period dimension (%d*%d) overflows int", ErrInvalidProblem, numGroups, numPeriods)
			}
		}
		for i := range numItems {
			if len(prob.Items[i]) != numPeriods {
				return 0, 0, 0, 0, fmt.Errorf("%w: ragged item row %d has length %d, want %d", ErrInvalidProblem, i, len(prob.Items[i]), numPeriods)
			}
		}
	}

	if len(prob.DesignatedItems) > 0 {
		if len(prob.DesignatedItems) != numGroups {
			return 0, 0, 0, 0, fmt.Errorf("%w: len(DesignatedItems) (%d) must equal Groups (%d)", ErrInvalidProblem, len(prob.DesignatedItems), numGroups)
		}
		seen := make([]bool, numItems)
		for g, idx := range prob.DesignatedItems {
			if idx < 0 || idx >= numItems {
				return 0, 0, 0, 0, fmt.Errorf("%w: DesignatedItems[%d]=%d out of range [0, %d)", ErrInvalidProblem, g, idx, numItems)
			}
			if seen[idx] {
				return 0, 0, 0, 0, fmt.Errorf("%w: duplicate DesignatedItems entry %d", ErrInvalidProblem, idx)
			}
			seen[idx] = true
		}
	}

	var ops uint64
	var baseMaxBits uint64
	if prob.Baselines != nil {
		for g := range numGroups {
			row := prob.Baselines[g]
			for t, v := range row {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return numGroups, numItems, numPeriods, 0, err
				}
				b := math.Float64bits(v)
				if b&expMask64 == expMask64 {
					return 0, 0, 0, 0, fmt.Errorf("%w: non-finite Baselines[%d][%d]", ErrInvalidProblem, g, t)
				}
				ab := b & absMask64
				if ab > baseMaxBits {
					baseMaxBits = ab
				}
			}
		}
	}

	itemAbsSum := make([]dd, numPeriods)
	for i := range numItems {
		row := prob.Items[i]
		for t, v := range row {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return numGroups, numItems, numPeriods, 0, err
			}
			if math.Float64bits(v)&expMask64 == expMask64 {
				return 0, 0, 0, 0, fmt.Errorf("%w: non-finite Items[%d][%d]", ErrInvalidProblem, i, t)
			}
			itemAbsSum[t] = ddAdd(itemAbsSum[t], dd{hi: math.Abs(v)})
		}
	}
	var itemAbsMax dd
	for t := range numPeriods {
		if ddCmp(itemAbsSum[t], itemAbsMax) > 0 {
			itemAbsMax = itemAbsSum[t]
		}
	}

	var uMax dd
	limitDD := dd{hi: maxProblemScale}
	usedFastUBound := false
	computeExactBaselinesUMax := func() (dd, error) {
		var exactMax dd
		for g := range numGroups {
			row := prob.Baselines[g]
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return dd{}, err
				}
				u := ddAdd(dd{hi: math.Abs(row[t])}, itemAbsSum[t])
				if !isFiniteDD(u) || ddCmp(u, limitDD) > 0 {
					return dd{}, fmt.Errorf("%w: series magnitude bound exceeds 1e15 at (%d, %d)", ErrInvalidProblem, g, t)
				}
				if ddCmp(u, exactMax) > 0 {
					exactMax = u
				}
			}
		}
		return exactMax, nil
	}

	if prob.Baselines != nil {
		baseMax := math.Float64frombits(baseMaxBits)
		uBound := ddAdd(dd{hi: baseMax}, itemAbsMax)
		const fastScaleThreshold = 0.99 * maxProblemScale
		if isFiniteDD(uBound) && ddToFloat64Ceil(uBound) <= fastScaleThreshold {
			uMax = uBound
			usedFastUBound = true
		} else {
			uMax, err = computeExactBaselinesUMax()
			if err != nil {
				return numGroups, numItems, numPeriods, 0, err
			}
		}
	} else {
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return numGroups, numItems, numPeriods, 0, err
			}
			u := itemAbsSum[t]
			if !isFiniteDD(u) || ddCmp(u, limitDD) > 0 {
				return 0, 0, 0, 0, fmt.Errorf("%w: series magnitude bound exceeds 1e15 at period %d", ErrInvalidProblem, t)
			}
			if ddCmp(u, uMax) > 0 {
				uMax = u
			}
		}
	}

	checkDerivedBounds := func(uVal dd) (float64, error) {
		uF := ddToFloat64Ceil(uVal)
		peak := uF
		if prob.Step > 0 && prob.RoundWeight > 0 {
			steps := uF / prob.Step
			if math.IsNaN(steps) || math.IsInf(steps, 0) || steps > maxStepCount {
				return 0, fmt.Errorf("%w: step count bound uMax/Step=%g exceeds 2^52", ErrInvalidProblem, steps)
			}
			nMax := max(1.0, math.Ceil(steps))
			stepPeak := prob.Step * nMax
			if math.IsNaN(stepPeak) || math.IsInf(stepPeak, 0) {
				return 0, fmt.Errorf("%w: step peak overflow", ErrInvalidProblem)
			}
			peak = max(peak, stepPeak)
		}

		perGroup := float64(numPeriods) + prob.RoundWeight
		gScale := float64(numGroups) * perGroup
		objScale := gScale * peak
		if math.IsNaN(objScale) || math.IsInf(objScale, 0) || objScale > maxObjScale {
			return 0, fmt.Errorf("%w: objective scale bound %g exceeds 1e300", ErrInvalidProblem, objScale)
		}
		return uF, nil
	}

	uMaxF, err = checkDerivedBounds(uMax)
	if err != nil && usedFastUBound {
		uMax, exactErr := computeExactBaselinesUMax()
		if exactErr != nil {
			return numGroups, numItems, numPeriods, 0, exactErr
		}
		uMaxF, err = checkDerivedBounds(uMax)
	}
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return numGroups, numItems, numPeriods, uMaxF, nil
}

func evalGroupForFixedN(s []dd, step, roundWeight float64, n int64) (cost, level dd) {
	numPeriods := len(s)
	p := twoProd(step, float64(n))

	var m dd
	if roundWeight >= float64(numPeriods) {
		m = p
	} else {
		k := sort.Search(numPeriods, func(t int) bool {
			return ddCmp(s[t], p) >= 0
		})
		kPrime := k + sort.Search(numPeriods-k, func(d int) bool {
			return ddCmp(s[k+d], p) > 0
		})
		switch {
		case float64(2*k-numPeriods) > roundWeight:
			j := max(0, min(numPeriods-1, int(math.Floor(0.5*(float64(numPeriods)+roundWeight)))-1))
			for j > 0 && float64(2*j-numPeriods) >= roundWeight {
				j--
			}
			for j+1 < numPeriods && float64(2*(j+1)-numPeriods) < roundWeight {
				j++
			}
			m = s[j]
			if ddCmp(m, dd{}) < 0 {
				m = dd{}
			}
		case float64(numPeriods-2*kPrime) > roundWeight:
			j := max(kPrime, min(numPeriods-1, int(math.Floor(0.5*(float64(numPeriods)-roundWeight)))-1))
			for j > kPrime && float64(numPeriods-2*j) <= roundWeight {
				j--
			}
			for j+1 < numPeriods && float64(numPeriods-2*(j+1)) > roundWeight {
				j++
			}
			m = s[j]
			if ddCmp(m, dd{}) < 0 {
				m = dd{}
			}
		default:
			m = p
		}
	}

	var sum dd
	for t := range numPeriods {
		sum = ddAdd(sum, ddAbs(ddSub(s[t], m)))
	}
	if ddCmp(m, p) != 0 {
		sum = ddAdd(sum, ddMulFloat(ddAbs(ddSub(m, p)), roundWeight))
	}
	return sum, m
}

func evalGroupExactDD(y []dd, step, roundWeight float64, allowZeroStep bool, scratch []dd) (cost, level dd, stepCount int64) {
	numPeriods := len(y)
	s := scratch[:numPeriods]
	copy(s, y)
	slices.SortFunc(s, ddCmp)

	uLo := s[(numPeriods-1)/2]
	if ddCmp(uLo, dd{}) < 0 {
		uLo = dd{}
	}
	uHi := s[numPeriods/2]
	if ddCmp(uHi, dd{}) < 0 {
		uHi = dd{}
	}

	if step == 0 || roundWeight == 0 {
		var sum dd
		for t := range numPeriods {
			sum = ddAdd(sum, ddAbs(ddSub(s[t], uLo)))
		}
		return sum, uLo, 0
	}

	nMin := int64(1)
	if allowZeroStep {
		nMin = 0
	}

	nLo := max(int64(0), int64(math.Ceil(uLo.hi/step)))
	for ddCmp(twoProd(step, float64(nLo)), uLo) < 0 {
		nLo++
	}
	for nLo > 0 && ddCmp(twoProd(step, float64(nLo-1)), uLo) >= 0 {
		nLo--
	}

	nHi := max(int64(0), int64(math.Floor(uHi.hi/step)))
	for nHi > 0 && ddCmp(twoProd(step, float64(nHi)), uHi) > 0 {
		nHi--
	}
	for ddCmp(twoProd(step, float64(nHi+1)), uHi) <= 0 {
		nHi++
	}

	if nLo < nMin {
		nLo = nMin
	}
	if nLo <= nHi {
		mStar := twoProd(step, float64(nLo))
		var sum dd
		for t := range numPeriods {
			sum = ddAdd(sum, ddAbs(ddSub(s[t], mStar)))
		}
		return sum, mStar, nLo
	}

	if nHi < nMin {
		c, m := evalGroupForFixedN(s, step, roundWeight, nMin)
		return c, m, nMin
	}

	cLo, mLo := evalGroupForFixedN(s, step, roundWeight, nHi)
	cHi, mHi := evalGroupForFixedN(s, step, roundWeight, nLo)
	if ddCmp(cHi, cLo) < 0 {
		return cHi, mHi, nLo
	}
	return cLo, mLo, nHi
}

func validateAssignment(prob Problem, numGroups, numItems int, assignment []int) error {
	if len(assignment) != numItems {
		return fmt.Errorf("%w: assignment length %d does not match Items (%d)", ErrInvalidProblem, len(assignment), numItems)
	}
	for i, g := range assignment {
		if g < 0 || g >= numGroups {
			return fmt.Errorf("%w: assignment[%d]=%d out of range [0, %d)", ErrInvalidProblem, i, g, numGroups)
		}
	}
	if len(prob.DesignatedItems) > 0 {
		usedGroup := make([]bool, numGroups)
		for _, item := range prob.DesignatedItems {
			g := assignment[item]
			if usedGroup[g] {
				return fmt.Errorf("%w: assignment violates DesignatedItems bijection on group %d", ErrInvalidProblem, g)
			}
			usedGroup[g] = true
		}
	}
	return nil
}

// EvaluateAssignment scores a discrete item map through the double-double single-group oracle.
func EvaluateAssignment(prob Problem, assignment []int) (Result, error) {
	numGroups, numItems, numPeriods, _, err := validateProblem(prob)
	if err != nil {
		return Result{}, err
	}
	if err := validateAssignment(prob, numGroups, numItems, assignment); err != nil {
		return Result{}, err
	}
	return evaluateAssignmentBudget(prob, numGroups, numItems, numPeriods, assignment, time.Time{}, false, nil, nil)
}

func evaluateAssignmentBudget(
	prob Problem,
	numGroups, numItems, numPeriods int,
	assignment []int,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
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

	yBuf := make([]dd, numPeriods)
	scratch := make([]dd, numPeriods)
	levels := make([]float64, numGroups)
	steps := make([]int64, numGroups)
	var totalDD dd
	var ops uint64

	if prob.Baselines == nil {
		emptyCost, emptyLvl, emptyStep := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
		emptyLvlF := ddToFloat64(emptyLvl)
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return Result{}, err
			}
			if head[g] < 0 {
				levels[g] = emptyLvlF
				steps[g] = emptyStep
				totalDD = ddAdd(totalDD, emptyCost)
				continue
			}
			clear(yBuf)
			for i := head[g]; i >= 0; i = nextItem[i] {
				row := prob.Items[i]
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return Result{}, err
					}
					yBuf[t] = ddAdd(yBuf[t], dd{hi: row[t]})
				}
			}
			cost, lvl, stepCnt := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
			levels[g] = ddToFloat64(lvl)
			steps[g] = stepCnt
			totalDD = ddAdd(totalDD, cost)
		}
	} else {
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return Result{}, err
			}
			baseRow := prob.Baselines[g]
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return Result{}, err
				}
				yBuf[t] = dd{hi: baseRow[t]}
			}
			for i := head[g]; i >= 0; i = nextItem[i] {
				row := prob.Items[i]
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						return Result{}, err
					}
					yBuf[t] = ddAdd(yBuf[t], dd{hi: row[t]})
				}
			}
			cost, lvl, stepCnt := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
			levels[g] = ddToFloat64(lvl)
			steps[g] = stepCnt
			totalDD = ddAdd(totalDD, cost)
		}
	}

	return Result{
		Assignment: slices.Clone(assignment),
		Levels:     levels,
		Steps:      steps,
		Objective:  ddToFloat64(totalDD),
		BestBound:  ddToFloat64Floor(totalDD),
		Status:     StatusOptimal,
	}, nil
}

// GreedyIncumbent builds a feasible discrete result by placing designated items
// bijectively and assigning remaining items by minimum marginal double-double cost.
func GreedyIncumbent(prob Problem) (Result, error) {
	numGroups, numItems, numPeriods, _, err := validateProblem(prob)
	if err != nil {
		return Result{}, err
	}
	res, gErr := greedyIncumbentBudget(prob, numGroups, numItems, numPeriods, time.Time{}, false, nil, nil)
	if gErr != nil {
		return Result{}, gErr
	}
	return res, nil
}

func greedyIncumbentBudget(
	prob Problem,
	numGroups, numItems, numPeriods int,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	assignment := make([]int, numItems)
	assignedItem := make([]bool, numItems)
	isDesignated := make([]bool, numItems)
	hasD := len(prob.DesignatedItems) == numGroups
	usedGroup := make([]bool, numGroups)
	if hasD {
		for _, item := range prob.DesignatedItems {
			isDesignated[item] = true
		}
	}

	finishTimeLimit := func() Result {
		if hasD {
			nextG := 0
			for _, item := range prob.DesignatedItems {
				if assignedItem[item] {
					continue
				}
				for nextG < numGroups && usedGroup[nextG] {
					nextG++
				}
				assignment[item] = nextG
				usedGroup[nextG] = true
				assignedItem[item] = true
			}
		}
		for i := range numItems {
			if !assignedItem[i] {
				assignment[i] = 0
				assignedItem[i] = true
			}
		}
		return Result{
			Assignment: assignment,
			Levels:     make([]float64, numGroups),
			Steps:      make([]int64, numGroups),
			Status:     StatusTimeLimit,
		}
	}

	scratch := make([]dd, numPeriods)
	candSeries := make([]dd, numPeriods)
	bestSeries := make([]dd, numPeriods)
	var ops uint64

	if prob.Baselines == nil {
		emptyCost, emptyLvl, emptyStep := evalGroupExactDD(candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
		maxActive := min(numGroups, numItems)
		groupActiveIdx := make([]int, numGroups)
		for g := range groupActiveIdx {
			groupActiveIdx[g] = -1
		}
		activeGroups := make([]int, 0, maxActive)
		activeCost := make([]dd, 0, maxActive)
		activeLevel := make([]dd, 0, maxActive)
		activeStep := make([]int64, 0, maxActive)
		activeSeries := make([][]dd, 0, maxActive)
		firstEmptyG := 0

		activateGroup := func(g int, series []dd, c, lvl dd, st int64) {
			idx := len(activeGroups)
			groupActiveIdx[g] = idx
			activeGroups = append(activeGroups, g)
			activeCost = append(activeCost, c)
			activeLevel = append(activeLevel, lvl)
			activeStep = append(activeStep, st)
			sCopy := make([]dd, numPeriods)
			copy(sCopy, series)
			activeSeries = append(activeSeries, sCopy)
			for firstEmptyG < numGroups && groupActiveIdx[firstEmptyG] >= 0 {
				firstEmptyG++
			}
		}

		if hasD {
			for _, item := range prob.DesignatedItems {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				g := firstEmptyG
				itemRow := prob.Items[item]
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return finishTimeLimit(), errTimeLimit
						}
						return Result{}, err
					}
					bestSeries[t] = dd{hi: itemRow[t]}
				}
				cCost, cLvl, cStep := evalGroupExactDD(bestSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
				usedGroup[g] = true
				assignment[item] = g
				assignedItem[item] = true
				activateGroup(g, bestSeries, cCost, cLvl, cStep)
			}
		}

		for i := range numItems {
			if isDesignated[i] {
				continue
			}
			itemRow := prob.Items[i]
			bestG := -1
			var bestDelta, bestCost, bestLvl dd
			var bestStep int64

			if firstEmptyG < numGroups {
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return finishTimeLimit(), errTimeLimit
						}
						return Result{}, err
					}
					candSeries[t] = dd{hi: itemRow[t]}
				}
				cCost, cLvl, cStep := evalGroupExactDD(candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
				bestG = firstEmptyG
				bestDelta = ddSub(cCost, emptyCost)
				bestCost = cCost
				bestLvl = cLvl
				bestStep = cStep
				copy(bestSeries, candSeries)
			}

			for agIdx, g := range activeGroups {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				gSeries := activeSeries[agIdx]
				for t := range numPeriods {
					if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return finishTimeLimit(), errTimeLimit
						}
						return Result{}, err
					}
					candSeries[t] = ddAdd(gSeries[t], dd{hi: itemRow[t]})
				}
				cCost, cLvl, cStep := evalGroupExactDD(candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
				delta := ddSub(cCost, activeCost[agIdx])
				cmp := ddCmp(delta, bestDelta)
				if bestG < 0 || cmp < 0 || (cmp == 0 && g < bestG) {
					bestG = g
					bestDelta = delta
					bestCost = cCost
					bestLvl = cLvl
					bestStep = cStep
					copy(bestSeries, candSeries)
				}
			}

			assignment[i] = bestG
			assignedItem[i] = true
			if idx := groupActiveIdx[bestG]; idx < 0 {
				activateGroup(bestG, bestSeries, bestCost, bestLvl, bestStep)
			} else {
				copy(activeSeries[idx], bestSeries)
				activeCost[idx] = bestCost
				activeLevel[idx] = bestLvl
				activeStep[idx] = bestStep
			}
		}

		levels := make([]float64, numGroups)
		steps := make([]int64, numGroups)
		emptyLvlF := ddToFloat64(emptyLvl)
		var totalDD dd
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return finishTimeLimit(), errTimeLimit
				}
				return Result{}, err
			}
			if idx := groupActiveIdx[g]; idx >= 0 {
				levels[g] = ddToFloat64(activeLevel[idx])
				steps[g] = activeStep[idx]
				totalDD = ddAdd(totalDD, activeCost[idx])
			} else {
				levels[g] = emptyLvlF
				steps[g] = emptyStep
				totalDD = ddAdd(totalDD, emptyCost)
			}
		}

		return Result{
			Assignment: assignment,
			Levels:     levels,
			Steps:      steps,
			Objective:  ddToFloat64(totalDD),
			BestBound:  ddToFloat64Floor(totalDD),
		}, nil
	}

	groupCost := make([]dd, numGroups)
	groupLevel := make([]dd, numGroups)
	groupStep := make([]int64, numGroups)

	useCachedSeries := satMul(satMul(int64(numGroups), int64(numPeriods)), 16) <= MaxDPBytes
	var groupSeries []dd
	var groupHead, groupTail, itemNext []int
	if useCachedSeries {
		groupSeries = make([]dd, numGroups*numPeriods)
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return finishTimeLimit(), errTimeLimit
				}
				return Result{}, err
			}
			row := prob.Baselines[g]
			base := g * numPeriods
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				groupSeries[base+t] = dd{hi: row[t]}
			}
			groupCost[g], groupLevel[g], groupStep[g] = evalGroupExactDD(
				groupSeries[base:base+numPeriods], prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch,
			)
		}
	} else {
		groupHead = make([]int, numGroups)
		groupTail = make([]int, numGroups)
		for g := range numGroups {
			groupHead[g] = -1
			groupTail[g] = -1
		}
		itemNext = make([]int, numItems)
		for i := range itemNext {
			itemNext[i] = -1
		}
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return finishTimeLimit(), errTimeLimit
				}
				return Result{}, err
			}
			row := prob.Baselines[g]
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				candSeries[t] = dd{hi: row[t]}
			}
			groupCost[g], groupLevel[g], groupStep[g] = evalGroupExactDD(
				candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch,
			)
		}
	}

	buildCandSeries := func(g int, itemRow []float64) error {
		if useCachedSeries {
			base := g * numPeriods
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return err
				}
				candSeries[t] = ddAdd(groupSeries[base+t], dd{hi: itemRow[t]})
			}
			return nil
		}
		baseRow := prob.Baselines[g]
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return err
			}
			candSeries[t] = dd{hi: baseRow[t]}
		}
		for k := groupHead[g]; k >= 0; k = itemNext[k] {
			kRow := prob.Items[k]
			for t := range numPeriods {
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					return err
				}
				candSeries[t] = ddAdd(candSeries[t], dd{hi: kRow[t]})
			}
		}
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				return err
			}
			candSeries[t] = ddAdd(candSeries[t], dd{hi: itemRow[t]})
		}
		return nil
	}

	commitGroupItem := func(g, item int, cCost, cLvl dd, cStep int64) {
		if useCachedSeries {
			copy(groupSeries[g*numPeriods:(g+1)*numPeriods], bestSeries)
		} else {
			if groupHead[g] < 0 {
				groupHead[g] = item
				groupTail[g] = item
			} else {
				itemNext[groupTail[g]] = item
				groupTail[g] = item
			}
		}
		groupCost[g] = cCost
		groupLevel[g] = cLvl
		groupStep[g] = cStep
	}

	if hasD {
		for _, item := range prob.DesignatedItems {
			itemRow := prob.Items[item]
			bestG := -1
			var bestDelta, bestCost, bestLvl dd
			var bestStep int64
			for g := range numGroups {
				if usedGroup[g] {
					continue
				}
				if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				if err := buildCandSeries(g, itemRow); err != nil {
					if errors.Is(err, errTimeLimit) {
						return finishTimeLimit(), errTimeLimit
					}
					return Result{}, err
				}
				cCost, cLvl, cStep := evalGroupExactDD(candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
				delta := ddSub(cCost, groupCost[g])
				if bestG < 0 || ddCmp(delta, bestDelta) < 0 {
					bestG = g
					bestDelta = delta
					bestCost = cCost
					bestLvl = cLvl
					bestStep = cStep
					if useCachedSeries {
						copy(bestSeries, candSeries)
					}
				}
			}
			usedGroup[bestG] = true
			assignment[item] = bestG
			assignedItem[item] = true
			commitGroupItem(bestG, item, bestCost, bestLvl, bestStep)
		}
	}

	for i := range numItems {
		if isDesignated[i] {
			continue
		}
		itemRow := prob.Items[i]
		bestG := 0
		var bestDelta, bestCost, bestLvl dd
		var bestStep int64
		for g := range numGroups {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return finishTimeLimit(), errTimeLimit
				}
				return Result{}, err
			}
			if err := buildCandSeries(g, itemRow); err != nil {
				if errors.Is(err, errTimeLimit) {
					return finishTimeLimit(), errTimeLimit
				}
				return Result{}, err
			}
			cCost, cLvl, cStep := evalGroupExactDD(candSeries, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
			delta := ddSub(cCost, groupCost[g])
			if g == 0 || ddCmp(delta, bestDelta) < 0 {
				bestG = g
				bestDelta = delta
				bestCost = cCost
				bestLvl = cLvl
				bestStep = cStep
				if useCachedSeries {
					copy(bestSeries, candSeries)
				}
			}
		}
		assignment[i] = bestG
		assignedItem[i] = true
		commitGroupItem(bestG, i, bestCost, bestLvl, bestStep)
	}

	levels := make([]float64, numGroups)
	steps := make([]int64, numGroups)
	var totalDD dd
	for g := range numGroups {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			if errors.Is(err, errTimeLimit) {
				return finishTimeLimit(), errTimeLimit
			}
			return Result{}, err
		}
		levels[g] = ddToFloat64(groupLevel[g])
		steps[g] = groupStep[g]
		totalDD = ddAdd(totalDD, groupCost[g])
	}

	return Result{
		Assignment: assignment,
		Levels:     levels,
		Steps:      steps,
		Objective:  ddToFloat64(totalDD),
		BestBound:  ddToFloat64Floor(totalDD),
	}, nil
}
