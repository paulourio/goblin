package mad

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"time"
)

const (
	// MaxDPItems bounds the default StrategyAuto bitmask DP item count before MILP fallback.
	MaxDPItems = 14
	// MaxDPCeiling bounds explicit StrategyDP item count.
	MaxDPCeiling = 20
	// MaxDPWork caps estimated oracle plus DP transition work before allocation.
	MaxDPWork = 64_000_000
	// MaxDPBytes caps estimated bitmask DP memory in bytes before allocation.
	MaxDPBytes = 32 << 20
	// MaxDFSStates bounds automatic StrategyBruteForce state selection.
	MaxDFSStates = 250_000
	// MaxBruteForceStates bounds explicit StrategyBruteForce state count.
	MaxBruteForceStates = 16_000_000
	// MaxBruteForceWork caps explicit StrategyBruteForce work.
	MaxBruteForceWork = 64_000_000

	cancelCheckStride = 4096
	satLimit          = math.MaxInt64 / 2
)

var errTimeLimit = errors.New("mad: time limit")

func ctxErrFn(ctx context.Context) func() error {
	if ctx == nil {
		return nil
	}
	return ctx.Err
}

func checkBudget(done <-chan struct{}, ctxErr func() error, deadline time.Time, hasDeadline bool, ops *uint64) error {
	if done == nil && !hasDeadline {
		return nil
	}
	*ops++
	if (*ops & (cancelCheckStride - 1)) != 0 {
		return nil
	}
	if done != nil {
		select {
		case <-done:
			if ctxErr != nil {
				if err := ctxErr(); err != nil {
					return err
				}
			}
			return context.Canceled
		default:
		}
	}
	if hasDeadline && !time.Now().Before(deadline) {
		return errTimeLimit
	}
	return nil
}

func satAdd(a, b int64) int64 {
	if a >= satLimit || b >= satLimit {
		return satLimit
	}
	if a > satLimit-b {
		return satLimit
	}
	return a + b
}

func satMul(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a >= satLimit || b >= satLimit {
		return satLimit
	}
	if a > satLimit/b {
		return satLimit
	}
	return a * b
}

func satPow(base int64, exp int) int64 {
	if exp <= 0 {
		return 1
	}
	if base <= 0 {
		return 0
	}
	res := int64(1)
	b := base
	e := exp
	for e > 0 {
		if e&1 != 0 {
			res = satMul(res, b)
		}
		e >>= 1
		if e > 0 {
			b = satMul(b, b)
		}
	}
	return res
}

func satFact(n int) int64 {
	res := int64(1)
	for i := 2; i <= n; i++ {
		res = satMul(res, int64(i))
		if res >= satLimit {
			return satLimit
		}
	}
	return res
}

func satStirling2BellSum(n, maxK int) int64 {
	if n <= 0 || maxK <= 0 {
		return 1
	}
	kLim := min(n, maxK)
	prev := make([]int64, kLim+1)
	cur := make([]int64, kLim+1)
	prev[0] = 1
	for i := 1; i <= n; i++ {
		clear(cur)
		up := min(i, kLim)
		for k := 1; k <= up; k++ {
			cur[k] = satAdd(prev[k-1], satMul(int64(k), prev[k]))
		}
		prev, cur = cur, prev
	}
	var total int64
	for k := 1; k <= kLim; k++ {
		total = satAdd(total, prev[k])
	}
	return total
}

func satBinom(n, k int) int64 {
	if k < 0 || k > n {
		return 0
	}
	if k == 0 || k == n {
		return 1
	}
	if k > n-k {
		k = n - k
	}
	res := int64(1)
	for i := 1; i <= k; i++ {
		num := int64(n - k + i)
		if res >= satLimit/num {
			// Divide first when possible, otherwise saturate.
			g := gcdInt64(res, int64(i))
			rDiv := res / g
			iDiv := int64(i) / g
			nDiv := num / iDiv
			if num%iDiv == 0 && rDiv <= satLimit/nDiv {
				res = rDiv * nDiv
				continue
			}
			return satLimit
		}
		res = (res * num) / int64(i)
	}
	return res
}

func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

type baselinePlan struct {
	uCount        int
	groupToUnique []int
	uniqueRep     []int
	uniqueGroups  [][]int
}

func hashBaselineRowBudget(
	row []float64,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
	ops *uint64,
) (uint64, error) {
	const (
		offset64 = uint64(14695981039346656037)
		prime64  = uint64(1099511628211)
	)
	h := offset64
	for _, v := range row {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, ops); err != nil {
			return 0, err
		}
		if v == 0 {
			v = 0
		}
		h ^= math.Float64bits(v)
		h *= prime64
		h ^= h >> 32
	}
	return h, nil
}

func equalBaselineRowsBudget(
	a, b []float64,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
	ops *uint64,
) (bool, error) {
	for t := range a {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, ops); err != nil {
			return false, err
		}
		if a[t] != b[t] {
			return false, nil
		}
	}
	return true, nil
}

func dedupBaselinesBudget(
	baselines [][]float64,
	numGroups int,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
	ops *uint64,
) (baselinePlan, error) {
	if baselines == nil {
		return baselinePlan{
			uCount:    1,
			uniqueRep: []int{0},
		}, nil
	}

	groupToUnique := make([]int, numGroups)
	uniqueRep := make([]int, 0, min(numGroups, 16))
	uniqueGroups := make([][]int, 0, min(numGroups, 16))
	buckets := make(map[uint64][]int, min(numGroups, 64))

	for g := range numGroups {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, ops); err != nil {
			return baselinePlan{}, err
		}
		row := baselines[g]
		h, err := hashBaselineRowBudget(row, deadline, hasDeadline, done, ctxErr, ops)
		if err != nil {
			return baselinePlan{}, err
		}
		matched := -1
		for _, u := range buckets[h] {
			eq, eqErr := equalBaselineRowsBudget(row, baselines[uniqueRep[u]], deadline, hasDeadline, done, ctxErr, ops)
			if eqErr != nil {
				return baselinePlan{}, eqErr
			}
			if eq {
				matched = u
				break
			}
		}
		if matched < 0 {
			matched = len(uniqueRep)
			uniqueRep = append(uniqueRep, g)
			uniqueGroups = append(uniqueGroups, []int{g})
			buckets[h] = append(buckets[h], matched)
		} else {
			uniqueGroups[matched] = append(uniqueGroups[matched], g)
		}
		groupToUnique[g] = matched
	}

	return baselinePlan{
		uCount:        len(uniqueRep),
		groupToUnique: groupToUnique,
		uniqueRep:     uniqueRep,
		uniqueGroups:  uniqueGroups,
	}, nil
}

type combPlan struct {
	bp            baselinePlan
	numGroups     int
	numItems      int
	numPeriods    int
	hasD          bool
	canPinD       bool
	freeItems     []int
	dfsStates     int64
	dfsWork       int64
	dpBits        int
	dpOverCeiling bool
	dpWork        int64
	dpBytes       int64
	storedSubsets int64
	activeGroups  int64
	streamingDP   bool
}

func buildCombPlanBudget(
	prob Problem,
	numGroups, numItems, numPeriods int,
	deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
	ops *uint64,
) (combPlan, error) {
	evalCost := satMul(int64(numPeriods), int64(max(1, bits.Len(uint(numPeriods)))))
	resultBytes := satMul(int64(numGroups), 16)
	finalEvalWork := satMul(int64(numGroups), evalCost)
	if prob.Baselines != nil && (finalEvalWork > MaxDPWork || resultBytes > MaxDPBytes) {
		return combPlan{
			numGroups:     numGroups,
			numItems:      numItems,
			numPeriods:    numPeriods,
			dfsStates:     satLimit,
			dfsWork:       satLimit,
			dpOverCeiling: true,
			dpWork:        finalEvalWork,
			dpBytes:       resultBytes,
		}, nil
	}

	bp, err := dedupBaselinesBudget(prob.Baselines, numGroups, deadline, hasDeadline, done, ctxErr, ops)
	if err != nil {
		return combPlan{}, err
	}
	hasD := len(prob.DesignatedItems) == numGroups
	canPinD := hasD && bp.uCount == 1

	var freeItems []int
	if hasD {
		isD := make([]bool, numItems)
		for _, idx := range prob.DesignatedItems {
			isD[idx] = true
		}
		freeItems = make([]int, 0, numItems-numGroups)
		for i := range numItems {
			if !isD[i] {
				freeItems = append(freeItems, i)
			}
		}
	}

	var dfsStates int64
	switch {
	case !hasD && prob.Baselines == nil:
		dfsStates = satStirling2BellSum(numItems, numGroups)
	case canPinD:
		dfsStates = satPow(int64(numGroups), numItems-numGroups)
	case hasD && !canPinD:
		dfsStates = satMul(satFact(numGroups), satPow(int64(numGroups), numItems-numGroups))
	default:
		dfsStates = satPow(int64(numGroups), numItems)
	}
	dfsWork := satMul(dfsStates, finalEvalWork)

	dpBits := numItems
	if canPinD {
		dpBits = numItems - numGroups
	}
	dpOverCeiling := dpBits > MaxDPCeiling
	streamingDP := !hasD && bp.uCount > 1 && numGroups > numItems+1

	var dpWork, dpBytes, storedSubsets, activeGroups int64
	if !dpOverCeiling {
		numMasks := int64(1) << uint(dpBits)
		var oracleEvals, dpTransWork int64

		switch {
		case !hasD && bp.uCount == 1:
			effG := min(numGroups, numItems)
			oracleEvals = numMasks
			dpTransWork = satMul(int64(effG), satPow(3, dpBits))
			storedSubsets = numMasks
			activeGroups = int64(effG)
		case !hasD && !streamingDP:
			oracleEvals = satMul(int64(bp.uCount), numMasks)
			dpTransWork = satMul(int64(numGroups), satPow(3, dpBits))
			storedSubsets = oracleEvals
			activeGroups = int64(numGroups)
		case !hasD && streamingDP:
			oracleEvals = satMul(int64(bp.uCount), numMasks)
			var candPairs int64
			for k := 1; k <= numItems; k++ {
				b := satBinom(numItems, k)
				capK := int64(min(numGroups, numItems-k+1))
				candPairs = satAdd(candPairs, satMul(b, capK))
				remPow := int64(1) << uint(numItems-k)
				dpTransWork = satAdd(dpTransWork, satMul(satMul(b, capK), remPow))
			}
			storedSubsets = satAdd(int64(bp.uCount), candPairs)
			activeGroups = min(int64(numGroups), candPairs)
		case canPinD:
			oracleEvals = satMul(int64(numGroups), numMasks)
			dpTransWork = satMul(int64(numGroups), satPow(3, dpBits))
			storedSubsets = oracleEvals
			activeGroups = int64(numGroups)
		default:
			// hasD && !canPinD
			freeMasks := int64(1) << uint(numItems-numGroups)
			oracleEvals = satMul(int64(bp.uCount), satMul(int64(numGroups), freeMasks))
			dSubsetHalf := int64(1) << uint(numGroups-1)
			dpTransWork = satMul(int64(numGroups), satMul(dSubsetHalf, satPow(3, numItems-numGroups)))
			storedSubsets = oracleEvals
			activeGroups = int64(numGroups)
		}

		oracleWork := satMul(oracleEvals, evalCost)
		postEvalWork := finalEvalWork
		if prob.Baselines == nil {
			postEvalWork = satAdd(int64(numGroups), satMul(int64(1+min(numGroups, numItems)), evalCost))
		}
		dpWork = satAdd(satAdd(oracleWork, dpTransWork), postEvalWork)
		dpAndNextBytes := satMul(satMul(2, numMasks), 16)
		storedBytes := satMul(storedSubsets, 16)
		parentBytes := satMul(satMul(activeGroups, numMasks), 4)
		dpBytes = satAdd(resultBytes, satAdd(dpAndNextBytes, satAdd(storedBytes, parentBytes)))
	}

	return combPlan{
		bp:            bp,
		numGroups:     numGroups,
		numItems:      numItems,
		numPeriods:    numPeriods,
		hasD:          hasD,
		canPinD:       canPinD,
		freeItems:     freeItems,
		dfsStates:     dfsStates,
		dfsWork:       dfsWork,
		dpBits:        dpBits,
		dpOverCeiling: dpOverCeiling,
		dpWork:        dpWork,
		dpBytes:       dpBytes,
		storedSubsets: storedSubsets,
		activeGroups:  activeGroups,
		streamingDP:   streamingDP,
	}, nil
}

func solve(ctx context.Context, prob Problem, opts ...Option) (Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}

	opt := ApplyOptions(opts...)
	if !opt.Strategy.valid() {
		return Result{}, fmt.Errorf("%w: invalid strategy %d", ErrInvalidProblem, opt.Strategy)
	}

	start := time.Now()
	var done <-chan struct{}
	ctxErr := ctxErrFn(ctx)
	if ctx != nil {
		done = ctx.Done()
	}
	var deadline time.Time
	hasDeadline := false
	expired := false
	if opt.TimeLimit < 0 {
		deadline = start
		hasDeadline = true
		expired = true
	} else if opt.TimeLimit > 0 {
		deadline = start.Add(opt.TimeLimit)
		hasDeadline = true
		expired = !start.Before(deadline)
	}

	valHasDeadline := hasDeadline && opt.Strategy != StrategyMILP
	numGroups, numItems, numPeriods, uMaxF, err := validateProblemBudget(prob, deadline, valHasDeadline, done, ctxErr)
	valTimedOut := false
	if err != nil {
		if !errors.Is(err, errTimeLimit) {
			return Result{}, err
		}
		valTimedOut = true
		expired = true
	}

	if opt.Certified && opt.cert == nil {
		return Result{}, fmt.Errorf("%w: Certified option requires WithCertified(true)", ErrCertificationFailed)
	}

	if hasDeadline && !expired && !time.Now().Before(deadline) {
		expired = true
	}

	if opt.Certified {
		info := validatedProblem{
			numGroups:   numGroups,
			numItems:    numItems,
			numPeriods:  numPeriods,
			uMaxF:       uMaxF,
			valTimedOut: valTimedOut,
		}
		return opt.cert.certify(ctx, prob, opt, info, start, deadline, hasDeadline, expired, done)
	}

	if opt.Strategy == StrategyMILP {
		if opt.fallback == nil {
			return Result{}, ErrCombinatorialLimit
		}
		return opt.fallback(ctx, prob, opt)
	}

	if expired {
		if valTimedOut && numItems == 0 {
			return zeroTimeoutResult(numGroups, opt.Strategy, start), nil
		}
		return greedyTimeoutResult(prob, numGroups, numItems, numPeriods, opt.Strategy, start, deadline, hasDeadline, done, ctxErr)
	}

	if numItems == 0 {
		return solveZeroItems(prob, numGroups, numPeriods, opt.Strategy, start, deadline, hasDeadline, done, ctxErr)
	}
	if numGroups == 1 {
		return solveSingleGroup(prob, numItems, numPeriods, opt.Strategy, start, deadline, hasDeadline, done, ctxErr)
	}

	var ops uint64
	plan, planErr := buildCombPlanBudget(prob, numGroups, numItems, numPeriods, deadline, hasDeadline, done, ctxErr, &ops)
	if planErr != nil {
		if errors.Is(planErr, errTimeLimit) {
			return greedyTimeoutResult(prob, numGroups, numItems, numPeriods, opt.Strategy, start, deadline, hasDeadline, done, ctxErr)
		}
		return Result{}, planErr
	}

	switch opt.Strategy {
	case StrategyBruteForce:
		if plan.dfsWork > MaxBruteForceWork || plan.dfsStates > MaxBruteForceStates {
			return Result{}, ErrCombinatorialLimit
		}
		return solveDFS(ctx, prob, plan, start, deadline, hasDeadline, done)
	case StrategyDP:
		if plan.dpOverCeiling || plan.dpWork > MaxDPWork || plan.dpBytes > MaxDPBytes {
			return Result{}, ErrCombinatorialLimit
		}
		return solveDP(ctx, prob, plan, start, deadline, hasDeadline, done)
	case StrategyAuto:
		dpAutoOK := !plan.dpOverCeiling && plan.dpBits <= MaxDPItems && plan.dpWork <= MaxDPWork && plan.dpBytes <= MaxDPBytes
		dfsAutoOK := plan.dfsStates <= MaxDFSStates && plan.dfsWork <= MaxBruteForceWork
		if dpAutoOK {
			if dfsAutoOK && plan.dfsWork <= plan.dpWork {
				return solveDFS(ctx, prob, plan, start, deadline, hasDeadline, done)
			}
			return solveDP(ctx, prob, plan, start, deadline, hasDeadline, done)
		}
		if opt.fallback != nil {
			return opt.fallback(ctx, prob, opt)
		}
		return Result{}, ErrCombinatorialLimit
	default:
		return Result{}, fmt.Errorf("%w: unsupported strategy %d", ErrInvalidProblem, opt.Strategy)
	}
}

func greedyTimeoutResult(
	prob Problem,
	numGroups, numItems, numPeriods int,
	s Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	res, err := greedyIncumbentBudget(prob, numGroups, numItems, numPeriods, deadline, hasDeadline, done, ctxErr)
	if err != nil && !errors.Is(err, errTimeLimit) {
		return Result{}, err
	}
	res.BestBound = 0
	res.Strategy = resolveTimeoutStrategy(s)
	res.Status = StatusTimeLimit
	res.SolveTime = time.Since(start)
	return res, nil
}

func finishEvaluatedResult(
	prob Problem,
	numGroups, numItems, numPeriods int,
	assign []int,
	s Strategy,
	status Status,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	res, err := evaluateAssignmentBudget(prob, numGroups, numItems, numPeriods, assign, deadline, hasDeadline, done, ctxErr)
	if err != nil {
		if errors.Is(err, errTimeLimit) {
			return Result{
				Assignment: slices.Clone(assign),
				Levels:     make([]float64, numGroups),
				Steps:      make([]int64, numGroups),
				BestBound:  0,
				Strategy:   s,
				Status:     StatusTimeLimit,
				SolveTime:  time.Since(start),
			}, nil
		}
		return Result{}, err
	}
	res.Strategy = s
	res.Status = status
	if status == StatusTimeLimit {
		res.BestBound = 0
	}
	res.SolveTime = time.Since(start)
	return res, nil
}

func resolveTimeoutStrategy(s Strategy) Strategy {
	if s == StrategyAuto {
		return StrategyDP
	}
	return s
}

func resolveFastPathStrategy(s Strategy) Strategy {
	if s == StrategyAuto {
		return StrategyDP
	}
	return s
}

func zeroTimeoutResult(numGroups int, s Strategy, start time.Time) Result {
	return Result{
		Assignment: []int{},
		Levels:     make([]float64, numGroups),
		Steps:      make([]int64, numGroups),
		BestBound:  0,
		Strategy:   resolveTimeoutStrategy(s),
		Status:     StatusTimeLimit,
		SolveTime:  time.Since(start),
	}
}

func solveZeroItems(
	prob Problem,
	numGroups, numPeriods int,
	s Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	var ops uint64
	bp, err := dedupBaselinesBudget(prob.Baselines, numGroups, deadline, hasDeadline, done, ctxErr, &ops)
	if err != nil {
		if errors.Is(err, errTimeLimit) {
			return zeroTimeoutResult(numGroups, s, start), nil
		}
		return Result{}, err
	}

	yBuf := make([]dd, numPeriods)
	scratch := make([]dd, numPeriods)
	uLevels := make([]float64, bp.uCount)
	uSteps := make([]int64, bp.uCount)
	uCosts := make([]dd, bp.uCount)

	for u, rep := range bp.uniqueRep {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			if errors.Is(err, errTimeLimit) {
				return zeroTimeoutResult(numGroups, s, start), nil
			}
			return Result{}, err
		}
		row := prob.Baselines[rep]
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return zeroTimeoutResult(numGroups, s, start), nil
				}
				return Result{}, err
			}
			yBuf[t] = dd{hi: row[t]}
		}
		c, lvl, st := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
		uCosts[u] = c
		uLevels[u] = ddToFloat64(lvl)
		uSteps[u] = st
	}

	levels := make([]float64, numGroups)
	steps := make([]int64, numGroups)
	var totalDD dd
	for g := range numGroups {
		if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
			if errors.Is(err, errTimeLimit) {
				return zeroTimeoutResult(numGroups, s, start), nil
			}
			return Result{}, err
		}
		u := bp.groupToUnique[g]
		levels[g] = uLevels[u]
		steps[g] = uSteps[u]
		totalDD = ddAdd(totalDD, uCosts[u])
	}

	return Result{
		Assignment: []int{},
		Levels:     levels,
		Steps:      steps,
		Objective:  ddToFloat64(totalDD),
		BestBound:  ddToFloat64Floor(totalDD),
		SolveTime:  time.Since(start),
		Strategy:   resolveFastPathStrategy(s),
		Status:     StatusOptimal,
	}, nil
}

func solveSingleGroup(
	prob Problem,
	numItems, numPeriods int,
	s Strategy,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	ctxErr func() error,
) (Result, error) {
	var ops uint64
	singleTimeout := func() Result {
		return Result{
			Assignment: make([]int, numItems),
			Levels:     make([]float64, 1),
			Steps:      make([]int64, 1),
			BestBound:  0,
			Strategy:   resolveTimeoutStrategy(s),
			Status:     StatusTimeLimit,
			SolveTime:  time.Since(start),
		}
	}

	yBuf := make([]dd, numPeriods)
	if prob.Baselines != nil {
		row := prob.Baselines[0]
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return singleTimeout(), nil
				}
				return Result{}, err
			}
			yBuf[t] = dd{hi: row[t]}
		}
	}
	for i := range numItems {
		row := prob.Items[i]
		for t := range numPeriods {
			if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return singleTimeout(), nil
				}
				return Result{}, err
			}
			yBuf[t] = ddAdd(yBuf[t], dd{hi: row[t]})
		}
	}
	if err := checkBudget(done, ctxErr, deadline, hasDeadline, &ops); err != nil {
		if errors.Is(err, errTimeLimit) {
			return singleTimeout(), nil
		}
		return Result{}, err
	}
	scratch := make([]dd, numPeriods)
	cost, lvl, stepCnt := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)

	return Result{
		Assignment: make([]int, numItems),
		Levels:     []float64{ddToFloat64(lvl)},
		Steps:      []int64{stepCnt},
		Objective:  ddToFloat64(cost),
		BestBound:  ddToFloat64Floor(cost),
		SolveTime:  time.Since(start),
		Strategy:   resolveFastPathStrategy(s),
		Status:     StatusOptimal,
	}, nil
}

func solveDFS(
	ctx context.Context,
	prob Problem,
	plan combPlan,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
) (Result, error) {
	numGroups := plan.numGroups
	numItems := plan.numItems
	numPeriods := plan.numPeriods

	useSymBreak := prob.Baselines == nil && !plan.hasD
	seriesGroups := numGroups
	if useSymBreak {
		seriesGroups = min(numGroups, numItems)
	}
	series := make([]dd, seriesGroups*numPeriods)
	if prob.Baselines != nil {
		for g := range numGroups {
			row := prob.Baselines[g]
			base := g * numPeriods
			for t := range numPeriods {
				series[base+t] = dd{hi: row[t]}
			}
		}
	}

	curAssign := make([]int, numItems)
	bestAssign := make([]int, numItems)
	var branchItems []int
	if plan.canPinD {
		for g, item := range prob.DesignatedItems {
			curAssign[item] = g
			row := prob.Items[item]
			base := g * numPeriods
			for t := range numPeriods {
				series[base+t] = ddAdd(series[base+t], dd{hi: row[t]})
			}
		}
		branchItems = plan.freeItems
	} else if plan.hasD {
		branchItems = make([]int, 0, numItems)
		branchItems = append(branchItems, prob.DesignatedItems...)
		branchItems = append(branchItems, plan.freeItems...)
	} else {
		branchItems = make([]int, numItems)
		for i := range numItems {
			branchItems[i] = i
		}
	}

	depthLimit := len(branchItems)
	saved := make([]dd, depthLimit*numPeriods)
	evalScratch := make([]dd, numPeriods)
	usedGroup := make([]bool, numGroups)
	var emptyCost dd
	emptyCostZero := true
	if useSymBreak && seriesGroups < numGroups {
		clear(evalScratch)
		emptyCost, _, _ = evalGroupExactDD(make([]dd, numPeriods), prob.Step, prob.RoundWeight, prob.AllowZeroStep, evalScratch)
		emptyCostZero = emptyCost.hi == 0 && emptyCost.lo == 0
	}

	var bestDD dd
	hasBest := false
	var ops uint64

	var dfs func(depth, maxGroup int) error
	dfs = func(depth, maxGroup int) error {
		if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
			return err
		}
		if depth == depthLimit {
			var total dd
			for g := range seriesGroups {
				base := g * numPeriods
				c, _, _ := evalGroupExactDD(series[base:base+numPeriods], prob.Step, prob.RoundWeight, prob.AllowZeroStep, evalScratch)
				total = ddAdd(total, c)
			}
			if !emptyCostZero {
				for g := seriesGroups; g < numGroups; g++ {
					total = ddAdd(total, emptyCost)
				}
			}
			if !hasBest || ddCmp(total, bestDD) < 0 {
				hasBest = true
				bestDD = total
				copy(bestAssign, curAssign)
			}
			return nil
		}

		itemIdx := branchItems[depth]
		itemRow := prob.Items[itemIdx]
		saveSlice := saved[depth*numPeriods : (depth+1)*numPeriods]

		if plan.hasD && !plan.canPinD && depth < numGroups {
			for g := range numGroups {
				if usedGroup[g] {
					continue
				}
				usedGroup[g] = true
				curAssign[itemIdx] = g
				gSlice := series[g*numPeriods : (g+1)*numPeriods]
				copy(saveSlice, gSlice)
				for t := range numPeriods {
					gSlice[t] = ddAdd(gSlice[t], dd{hi: itemRow[t]})
				}
				err := dfs(depth+1, numGroups-1)
				copy(gSlice, saveSlice)
				usedGroup[g] = false
				if err != nil {
					return err
				}
			}
			return nil
		}

		gLimit := numGroups - 1
		if useSymBreak {
			gLimit = min(numGroups-1, maxGroup+1)
		}
		for g := 0; g <= gLimit; g++ {
			curAssign[itemIdx] = g
			gSlice := series[g*numPeriods : (g+1)*numPeriods]
			copy(saveSlice, gSlice)
			for t := range numPeriods {
				gSlice[t] = ddAdd(gSlice[t], dd{hi: itemRow[t]})
			}
			nextMax := max(maxGroup, g)
			err := dfs(depth+1, nextMax)
			copy(gSlice, saveSlice)
			if err != nil {
				return err
			}
		}
		return nil
	}

	err := dfs(0, -1)
	if err != nil {
		if errors.Is(err, errTimeLimit) {
			if hasBest {
				return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, bestAssign, StrategyBruteForce, StatusTimeLimit, start, deadline, hasDeadline, done, ctxErrFn(ctx))
			}
			return greedyTimeoutResult(prob, numGroups, numItems, numPeriods, StrategyBruteForce, start, deadline, hasDeadline, done, ctxErrFn(ctx))
		}
		return Result{}, err
	}

	return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, bestAssign, StrategyBruteForce, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
}

type streamCand struct {
	delta dd
	cost  dd
	group int
}

func streamCandGreater(a, b streamCand) bool {
	cmp := ddCmp(a.delta, b.delta)
	if cmp != 0 {
		return cmp > 0
	}
	return a.group > b.group
}

func streamHeapPushCap(h []streamCand, kCap int, item streamCand) ([]streamCand, bool) {
	if len(h) < kCap {
		h = append(h, item)
		idx := len(h) - 1
		for idx > 0 {
			p := (idx - 1) / 2
			if !streamCandGreater(h[idx], h[p]) {
				break
			}
			h[idx], h[p] = h[p], h[idx]
			idx = p
		}
		return h, true
	}
	if !streamCandGreater(h[0], item) {
		return h, false
	}
	h[0] = item
	idx := 0
	n := len(h)
	for {
		left := 2*idx + 1
		if left >= n {
			break
		}
		largest := left
		right := left + 1
		if right < n && streamCandGreater(h[right], h[left]) {
			largest = right
		}
		if !streamCandGreater(h[largest], h[idx]) {
			break
		}
		h[idx], h[largest] = h[largest], h[idx]
		idx = largest
	}
	return h, true
}

func evalSubsetCostDD(
	baseline []float64,
	items [][]float64,
	itemSubset []int,
	mask int,
	step, roundWeight float64,
	allowZeroStep bool,
	yBuf, scratch []dd,
) dd {
	numPeriods := len(yBuf)
	if baseline != nil {
		for t := range numPeriods {
			yBuf[t] = dd{hi: baseline[t]}
		}
	} else {
		clear(yBuf)
	}
	for m := uint(mask); m != 0; {
		bit := bits.TrailingZeros(m)
		m &= m - 1
		row := items[itemSubset[bit]]
		for t := range numPeriods {
			yBuf[t] = ddAdd(yBuf[t], dd{hi: row[t]})
		}
	}
	cost, _, _ := evalGroupExactDD(yBuf, step, roundWeight, allowZeroStep, scratch)
	return cost
}

func solveDP(
	ctx context.Context,
	prob Problem,
	plan combPlan,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
) (Result, error) {
	numGroups := plan.numGroups
	numItems := plan.numItems
	numPeriods := plan.numPeriods
	numMasks := 1 << uint(plan.dpBits)
	fullMask := numMasks - 1

	yBuf := make([]dd, numPeriods)
	scratch := make([]dd, numPeriods)
	var ops uint64

	timeoutRes := func() (Result, error) {
		return greedyTimeoutResult(prob, numGroups, numItems, numPeriods, StrategyDP, start, deadline, hasDeadline, done, ctxErrFn(ctx))
	}

	if plan.hasD && !plan.canPinD {
		return solveDPDesignatedDistinct(ctx, prob, plan, start, deadline, hasDeadline, done, yBuf, scratch, &ops)
	}

	dp := make([]dd, numMasks)
	next := make([]dd, numMasks)
	dpValid := make([]bool, numMasks)
	nextValid := make([]bool, numMasks)
	dpValid[0] = true

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
		subCost := make([]dd, numMasks)
		for s := range numMasks {
			if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return timeoutRes()
				}
				return Result{}, err
			}
			subCost[s] = evalSubsetCostDD(baseRow, prob.Items, allItemsIdx, s, prob.Step, prob.RoundWeight, prob.AllowZeroStep, yBuf, scratch)
		}

		parent := make([]uint32, effG*numMasks)
		emptyCost := subCost[0]
		emptyZero := emptyCost.hi == 0 && emptyCost.lo == 0

		for g := range effG {
			if emptyZero {
				copy(next, dp)
				copy(nextValid, dpValid)
			} else {
				for m := range numMasks {
					nextValid[m] = dpValid[m]
					if dpValid[m] {
						next[m] = ddAdd(dp[m], emptyCost)
					}
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				c := subCost[s]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return timeoutRes()
						}
						return Result{}, err
					}
					if dpValid[prev] {
						cand := ddAdd(dp[prev], c)
						m := prev | s
						if !nextValid[m] || ddCmp(cand, next[m]) < 0 {
							nextValid[m] = true
							next[m] = cand
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dp, next = next, dp
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
		return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, assign, StrategyDP, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
	}

	if plan.canPinD {
		var baseRow []float64
		if prob.Baselines != nil {
			baseRow = prob.Baselines[0]
		}
		pinY := make([]dd, numPeriods)
		gCosts := make([]dd, numMasks)
		parent := make([]uint32, numGroups*numMasks)

		for g := range numGroups {
			dRow := prob.Items[prob.DesignatedItems[g]]
			if baseRow != nil {
				for t := range numPeriods {
					pinY[t] = ddAdd(dd{hi: baseRow[t]}, dd{hi: dRow[t]})
				}
			} else {
				for t := range numPeriods {
					pinY[t] = dd{hi: dRow[t]}
				}
			}
			for s := range numMasks {
				if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return timeoutRes()
					}
					return Result{}, err
				}
				copy(yBuf, pinY)
				for m := uint(s); m != 0; {
					bit := bits.TrailingZeros(m)
					m &= m - 1
					row := prob.Items[plan.freeItems[bit]]
					for t := range numPeriods {
						yBuf[t] = ddAdd(yBuf[t], dd{hi: row[t]})
					}
				}
				c, _, _ := evalGroupExactDD(yBuf, prob.Step, prob.RoundWeight, prob.AllowZeroStep, scratch)
				gCosts[s] = c
			}

			emptyCost := gCosts[0]
			for m := range numMasks {
				nextValid[m] = dpValid[m]
				if dpValid[m] {
					next[m] = ddAdd(dp[m], emptyCost)
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				c := gCosts[s]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return timeoutRes()
						}
						return Result{}, err
					}
					if dpValid[prev] {
						cand := ddAdd(dp[prev], c)
						m := prev | s
						if !nextValid[m] || ddCmp(cand, next[m]) < 0 {
							nextValid[m] = true
							next[m] = cand
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dp, next = next, dp
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
		return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, assign, StrategyDP, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
	}

	if !plan.streamingDP {
		uniqueCosts := make([]dd, plan.bp.uCount*numMasks)
		for u, rep := range plan.bp.uniqueRep {
			baseRow := prob.Baselines[rep]
			uOff := u * numMasks
			for s := range numMasks {
				if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return timeoutRes()
					}
					return Result{}, err
				}
				uniqueCosts[uOff+s] = evalSubsetCostDD(baseRow, prob.Items, allItemsIdx, s, prob.Step, prob.RoundWeight, prob.AllowZeroStep, yBuf, scratch)
			}
		}

		parent := make([]uint32, numGroups*numMasks)
		for g := range numGroups {
			uOff := plan.bp.groupToUnique[g] * numMasks
			emptyCost := uniqueCosts[uOff]
			for m := range numMasks {
				nextValid[m] = dpValid[m]
				if dpValid[m] {
					next[m] = ddAdd(dp[m], emptyCost)
				}
			}
			rowOff := g * numMasks
			for s := 1; s < numMasks; s++ {
				c := uniqueCosts[uOff+s]
				comp := fullMask ^ s
				for prev := comp; ; prev = (prev - 1) & comp {
					if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
						if errors.Is(err, errTimeLimit) {
							return timeoutRes()
						}
						return Result{}, err
					}
					if dpValid[prev] {
						cand := ddAdd(dp[prev], c)
						m := prev | s
						if !nextValid[m] || ddCmp(cand, next[m]) < 0 {
							nextValid[m] = true
							next[m] = cand
							parent[rowOff+m] = uint32(s)
						}
					}
					if prev == 0 {
						break
					}
				}
			}
			dp, next = next, dp
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
		return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, assign, StrategyDP, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
	}

	// Streaming top-(I - |S| + 1) mode when !hasD && U > 1 && G > I + 1.
	emptyCosts := make([]dd, plan.bp.uCount)
	for u, rep := range plan.bp.uniqueRep {
		emptyCosts[u] = evalSubsetCostDD(prob.Baselines[rep], prob.Items, allItemsIdx, 0, prob.Step, prob.RoundWeight, prob.AllowZeroStep, yBuf, scratch)
	}

	candSub := make([][]int, numGroups)
	candCost := make([][]dd, numGroups)
	heap := make([]streamCand, 0, numItems+1)

	for s := 1; s < numMasks; s++ {
		kCap := min(numGroups, numItems-bits.OnesCount(uint(s))+1)
		heap = heap[:0]
		for u, rep := range plan.bp.uniqueRep {
			if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
				if errors.Is(err, errTimeLimit) {
					return timeoutRes()
				}
				return Result{}, err
			}
			cU := evalSubsetCostDD(prob.Baselines[rep], prob.Items, allItemsIdx, s, prob.Step, prob.RoundWeight, prob.AllowZeroStep, yBuf, scratch)
			delta := ddSub(cU, emptyCosts[u])
			for _, g := range plan.bp.uniqueGroups[u] {
				var kept bool
				heap, kept = streamHeapPushCap(heap, kCap, streamCand{delta: delta, cost: cU, group: g})
				if !kept {
					break
				}
			}
		}
		for _, item := range heap {
			candSub[item.group] = append(candSub[item.group], s)
			candCost[item.group] = append(candCost[item.group], item.cost)
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
		emptyCost := emptyCosts[plan.bp.groupToUnique[g]]
		for m := range numMasks {
			nextValid[m] = dpValid[m]
			if dpValid[m] {
				next[m] = ddAdd(dp[m], emptyCost)
			}
		}
		rowOff := ai * numMasks
		subs := candSub[g]
		costs := candCost[g]
		for idx, s := range subs {
			c := costs[idx]
			comp := fullMask ^ s
			for prev := comp; ; prev = (prev - 1) & comp {
				if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, &ops); err != nil {
					if errors.Is(err, errTimeLimit) {
						return timeoutRes()
					}
					return Result{}, err
				}
				if dpValid[prev] {
					cand := ddAdd(dp[prev], c)
					m := prev | s
					if !nextValid[m] || ddCmp(cand, next[m]) < 0 {
						nextValid[m] = true
						next[m] = cand
						//nolint:gosec // G115: s is bounded by 2^MaxDPCeiling, fits in uint32.
						parent[rowOff+m] = uint32(s)
					}
				}
				if prev == 0 {
					break
				}
			}
		}
		dp, next = next, dp
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
	return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, assign, StrategyDP, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
}

func solveDPDesignatedDistinct(
	ctx context.Context,
	prob Problem,
	plan combPlan,
	start, deadline time.Time,
	hasDeadline bool,
	done <-chan struct{},
	yBuf, scratch []dd,
	ops *uint64,
) (Result, error) {
	numGroups := plan.numGroups
	numItems := plan.numItems
	numPeriods := plan.numPeriods
	numMasks := 1 << uint(numItems)
	freeCount := numItems - numGroups
	numFreeMasks := 1 << uint(freeCount)
	fullFreeMask := numFreeMasks - 1

	timeoutRes := func() (Result, error) {
		return greedyTimeoutResult(prob, numGroups, numItems, numPeriods, StrategyDP, start, deadline, hasDeadline, done, ctxErrFn(ctx))
	}

	expandedFree := make([]int, numFreeMasks)
	for f := 1; f < numFreeMasks; f++ {
		lsb := bits.TrailingZeros(uint(f))
		expandedFree[f] = expandedFree[f^(1<<lsb)] | (1 << plan.freeItems[lsb])
	}

	dBits := make([]int, numGroups)
	for g, idx := range prob.DesignatedItems {
		dBits[g] = 1 << idx
	}

	// Precompute exact cost for (u, dIdx, freeMask).
	strideD := numFreeMasks
	strideU := numGroups * strideD
	uCosts := make([]dd, plan.bp.uCount*strideU)
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
					if errors.Is(err, errTimeLimit) {
						return timeoutRes()
					}
					return Result{}, err
				}
				s := dBit | expandedFree[freeS]
				uCosts[u*strideU+dIdx*strideD+freeS] = evalSubsetCostDD(
					baseRow, prob.Items, allItemsIdx, s, prob.Step, prob.RoundWeight, prob.AllowZeroStep, yBuf, scratch,
				)
			}
		}
	}

	// Group designated-item masks by cardinality for O(1) iteration per stage g.
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

	dp := make([]dd, numMasks)
	next := make([]dd, numMasks)
	dpValid := make([]bool, numMasks)
	nextValid := make([]bool, numMasks)
	dpValid[0] = true
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
				c := uCosts[costOff+freeS]
				s := dBit | expandedFree[freeS]
				compFree := fullFreeMask ^ freeS
				for _, dm := range dPrevList {
					if dm&dBitFlag != 0 {
						continue
					}
					dExp := dMaskExpanded[dm]
					for prevFree := compFree; ; prevFree = (prevFree - 1) & compFree {
						if err := checkBudget(done, ctxErrFn(ctx), deadline, hasDeadline, ops); err != nil {
							if errors.Is(err, errTimeLimit) {
								return timeoutRes()
							}
							return Result{}, err
						}
						prev := dExp | expandedFree[prevFree]
						if dpValid[prev] {
							cand := ddAdd(dp[prev], c)
							m := prev | s
							if !nextValid[m] || ddCmp(cand, next[m]) < 0 {
								nextValid[m] = true
								next[m] = cand
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
		dp, next = next, dp
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
	return finishEvaluatedResult(prob, numGroups, numItems, numPeriods, assign, StrategyDP, StatusOptimal, start, deadline, hasDeadline, done, ctxErrFn(ctx))
}
