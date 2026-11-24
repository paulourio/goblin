package goblin

import (
	"context"
	"math"
	"math/big"
	"slices"
	"strings"
	"time"
)

func setRootCertifiedTrue(cfg *Options) {
	cfg.Certified = true
	cfg.certifyFn = solveCertified
}

func setRootCertifiedFalse(cfg *Options) {
	cfg.Certified = false
	cfg.certifyFn = nil
}

// WithCertified enables or disables fail-closed exact rational checking at runtime.
// When enabled is true, any gap threshold not explicitly set via WithRelGapTol or
// WithAbsGapTol (or overridden in WithOptions) defaults to 0, requiring zero rational
// dual gap on pure LPs and exact optimal Mini-VIPR proofs on MILPs.
func WithCertified(enabled bool) Option {
	if enabled {
		return setRootCertifiedTrue
	}
	return setRootCertifiedFalse
}

func optVIPRTrace(opt Options) *VIPRTrace {
	tr, _ := opt.viprTrace.(*VIPRTrace)
	return tr
}

type goblinCertFaultKey struct{}

type lpCertWitness struct {
	primal          []*big.Rat
	dualMin         []*big.Rat
	farkasW         []*big.Rat
	unbounded       UnboundedCertificate
	hasUnbounded    bool
	hasDirectInfeas bool
	polished        bool
}

type goblinCertFaultFunc func(p *CompiledProblem, trace *VIPRTrace, lpWit *lpCertWitness, res *Result)

func failClosedResult(p *CompiledProblem, res Result, backend Backend, start time.Time) (Result, error) {
	var modelID uint64
	if p != nil {
		modelID = p.modelID
	} else {
		modelID = res.modelID
	}
	solveTime := res.SolveTime
	if solveTime <= 0 && !start.IsZero() {
		solveTime = time.Since(start)
	}
	return Result{
		modelID:    modelID,
		Backend:    backend,
		Status:     StatusNumericalError,
		Nodes:      res.Nodes,
		Iterations: res.Iterations,
		CutsAdded:  res.CutsAdded,
		SolveTime:  solveTime,
	}, ErrCertificationFailed
}

func solveCertified(ctx context.Context, p *CompiledProblem, opt Options, start time.Time, done <-chan struct{}) (Result, error) {
	opt.RelGapTol, opt.AbsGapTol = opt.effectiveGapTols()
	var deadline time.Time
	if opt.TimeLimit < 0 {
		trivBnd := trivialDualBound(p.sense)
		res := Result{
			modelID:      p.modelID,
			Backend:      opt.Backend,
			Status:       StatusTimeLimit,
			BestBound:    trivBnd,
			RootLPBound:  trivBnd,
			RootCutBound: trivBnd,
			SolveTime:    time.Since(start),
		}
		var wit *lpCertWitness
		if !p.isMIP {
			wit = &lpCertWitness{}
		}
		return finalizeCertifiedSolve(ctx, p, opt, optVIPRTrace(opt), wit, res, start)
	}
	if opt.TimeLimit > 0 {
		deadline = start.Add(opt.TimeLimit)
		if !start.Before(deadline) {
			trivBnd := trivialDualBound(p.sense)
			res := Result{
				modelID:      p.modelID,
				Backend:      opt.Backend,
				Status:       StatusTimeLimit,
				BestBound:    trivBnd,
				RootLPBound:  trivBnd,
				RootCutBound: trivBnd,
				SolveTime:    time.Since(start),
			}
			var wit *lpCertWitness
			if !p.isMIP {
				wit = &lpCertWitness{}
			}
			return finalizeCertifiedSolve(ctx, p, opt, optVIPRTrace(opt), wit, res, start)
		}
	}

	if p.isMIP {
		if opt.Backend == BackendCBC && !CBCAvailable() {
			res := solveCBC(ctx, p, opt, start, deadline, done)
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return Result{}, err
				}
			}
			return finalizeCertifiedSolve(ctx, p, opt, optVIPRTrace(opt), nil, res, start)
		}

		execOpt := opt
		trace := optVIPRTrace(execOpt)
		if trace == nil {
			trace = NewVIPRTrace()
			execOpt.viprTrace = trace
		} else {
			trace.reset(p)
		}
		execOpt.Workers = 1
		if opt.Backend == BackendCBC {
			if opt.defaultMaxNodesUsed {
				execOpt.MaxNodes = 0
			}
			if opt.defaultCutPassesUsed {
				execOpt.MaxCutPasses = 30
			}
		}
		if execOpt.RelGapTol > 0 && !math.IsInf(execOpt.RelGapTol, 1) {
			execOpt.RelGapTol *= 0.999
		}
		if execOpt.AbsGapTol > 0 && !math.IsInf(execOpt.AbsGapTol, 1) {
			execOpt.AbsGapTol *= 0.999
		}

		stop := exactStop{deadline: deadline, done: done}
		res := solveCompensatedEngine(ctx, p, execOpt, opt.Backend, start, deadline, done)
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}

		var mipWit *lpCertWitness
		switch res.Status {
		case StatusOptimal, StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
			polishMIPResultFromTrace(p, trace, &res)
		case StatusInfeasible:
			scrubNonFiniteRootBounds(&res)
		case StatusUnbounded:
			scrubNonFiniteRootBounds(&res)
			certUnb, okUnb := extractUnboundedCertificateStop(stop, p, nil, res.Values)
			mipWit = &lpCertWitness{unbounded: certUnb, hasUnbounded: okUnb}
		}

		if execOpt.EnablePresolve && !stop.canceled() &&
			res.Status != StatusTimeLimit && res.Status != StatusNodeLimit && res.Status != StatusIterationLimit &&
			verifyCertifiedMIP(stop, p, trace, mipWit, res, opt, true) != nil {
			noPreOpt := execOpt
			noPreOpt.EnablePresolve = false
			canRetry := true
			if noPreOpt.MaxIter > 0 {
				if rem := noPreOpt.MaxIter - res.Iterations; rem > 0 {
					noPreOpt.MaxIter = rem
				} else {
					canRetry = false
				}
			}
			if noPreOpt.MaxNodes > 0 {
				if remNodes := noPreOpt.MaxNodes - res.Nodes; remNodes > 0 {
					noPreOpt.MaxNodes = remNodes
				} else {
					canRetry = false
				}
			}
			if canRetry {
				prevNodes, prevIters := res.Nodes, res.Iterations
				trace.reset(p)
				mipWit = nil
				res = solveCompensatedEngine(ctx, p, noPreOpt, opt.Backend, start, deadline, done)
				if ctx != nil {
					if err := ctx.Err(); err != nil {
						return Result{}, err
					}
				}
				res.Nodes += prevNodes
				res.Iterations += prevIters
				switch res.Status {
				case StatusOptimal, StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
					polishMIPResultFromTrace(p, trace, &res)
				case StatusInfeasible:
					scrubNonFiniteRootBounds(&res)
				case StatusUnbounded:
					scrubNonFiniteRootBounds(&res)
					certUnb, okUnb := extractUnboundedCertificateStop(stop, p, nil, res.Values)
					mipWit = &lpCertWitness{unbounded: certUnb, hasUnbounded: okUnb}
				}
			}
		}
		return finalizeCertifiedSolve(ctx, p, opt, trace, mipWit, res, start)
	}

	var res Result
	if opt.Backend == BackendCBC {
		res = solveCBC(ctx, p, opt, start, deadline, done)
	} else {
		res = solveCompensatedEngine(ctx, p, opt, BackendGo, start, deadline, done)
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}

	stop := exactStop{deadline: deadline, done: done}
	lpWit := buildCertifiedLPWitness(stop, p, opt, &res)
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	if lpWit == nil && opt.Backend != BackendCBC && opt.EnablePresolve && !stop.canceled() &&
		res.Status != StatusTimeLimit && res.Status != StatusNodeLimit && res.Status != StatusIterationLimit {
		noPreOpt := opt
		noPreOpt.EnablePresolve = false
		res = solveCompensatedEngine(ctx, p, noPreOpt, BackendGo, start, deadline, done)
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		lpWit = buildCertifiedLPWitness(stop, p, opt, &res)
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
	}
	trace := optVIPRTrace(opt)
	if trace != nil {
		if res.Status == StatusUnbounded || (isLimitStatus(res.Status) && isTrivialInfBound(p.sense, res.BestBound)) {
			if trace.TracedProblem() != p {
				trace.reset(p)
			}
		} else if lpWit != nil {
			populateLPVIPRTraceFromWitness(p, trace, res, lpWit)
		}
	}
	return finalizeCertifiedSolve(ctx, p, opt, trace, lpWit, res, start)
}

func finalizeCertifiedSolve(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
	trace *VIPRTrace,
	wit *lpCertWitness,
	res Result,
	start time.Time,
) (Result, error) {
	backend := res.Backend
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if fn, ok := ctx.Value(goblinCertFaultKey{}).(goblinCertFaultFunc); ok && fn != nil {
			fn(p, trace, wit, &res)
		}
	}

	if p.isMIP {
		if err := verifyCertifiedMIP(exactStopFromCtx(ctx), p, trace, wit, res, opt, true); err != nil {
			return failClosedResult(p, res, backend, start)
		}
		return res, nil
	}

	if err := verifyCertifiedLP(p, opt, res, wit); err != nil {
		return failClosedResult(p, res, backend, start)
	}
	if trace != nil && res.Status != StatusUnbounded && !(isLimitStatus(res.Status) && isTrivialInfBound(p.sense, res.BestBound)) {
		if err := VerifyVIPRTrace(p, trace, res, opt); err != nil {
			return failClosedResult(p, res, backend, start)
		}
	}
	return res, nil
}

// VerifyVIPRTrace checks trace against p in exact rational arithmetic, covering
// Mini-VIPR proof steps, presolve envelopes, orbital-symmetry witnesses, feasible
// incumbents, rational gap closure under opt, and Meyer MILP unbounded rays.
// Integer coordinates in res.Values must match the certificate incumbent exactly.
// When opt.Certified is false, res.Objective, res.BestBound, and continuous coordinates
// in res.Values allow 1e-6 relative floating-point slack; passing WithCertified(true)
// in opt requires bit-exact float64 matches against the rounded rational witness
// values and defaults unset gap thresholds to zero.
func VerifyVIPRTrace(p *CompiledProblem, trace *VIPRTrace, res Result, opt Options) error {
	if p == nil {
		return ErrCertificationFailed
	}
	if trace == nil && res.Status != StatusUnbounded && !(isLimitStatus(res.Status) && isTrivialInfBound(p.sense, res.BestBound)) {
		return ErrCertificationFailed
	}
	stop := exactStop{deadline: time.Now().Add(5 * time.Second)}
	return verifyCertifiedMIP(stop, p, trace, nil, res, opt, opt.Certified)
}

func scrubNonFiniteRootBounds(res *Result) {
	if isNonFinite(res.RootLPBound) {
		res.RootLPBound = 0
	}
	if isNonFinite(res.RootCutBound) {
		res.RootCutBound = 0
	}
}

func clampRootBoundToBestBound(sense Sense, rootB, bestB float64) float64 {
	if math.IsNaN(rootB) || isOppositeInfBound(sense, rootB) {
		return bestB
	}
	if isTrivialInfBound(sense, bestB) {
		return bestB
	}
	if isTrivialInfBound(sense, rootB) {
		return rootB
	}
	if math.IsNaN(bestB) || math.IsInf(bestB, 0) {
		return rootB
	}
	if sense == Maximize {
		if rootB < bestB {
			return bestB
		}
		return rootB
	}
	if rootB > bestB {
		return bestB
	}
	return rootB
}

func verifyRootBoundAgainstBest(sense Sense, rootB, bestB float64, strictPolished bool) bool {
	if math.IsNaN(rootB) || math.IsNaN(bestB) {
		return false
	}
	if isTrivialInfBound(sense, rootB) {
		return true
	}
	if math.IsInf(rootB, 0) {
		return false
	}
	if isTrivialInfBound(sense, bestB) {
		return !strictPolished && rootB == 0
	}
	if math.IsInf(bestB, 0) {
		return false
	}
	tol := 0.0
	if !strictPolished {
		tol = 1e-6 * max(1.0, math.Abs(bestB))
	}
	if sense == Maximize {
		return rootB >= bestB-tol
	}
	return rootB <= bestB+tol
}

func polishMIPResultFromTrace(p *CompiledProblem, trace *VIPRTrace, res *Result) {
	if p == nil || res == nil {
		return
	}
	if trace == nil || !trace.HasCertificate() {
		if isLimitStatus(res.Status) {
			res.BestBound = trivialDualBound(p.sense)
			res.RootLPBound = res.BestBound
			res.RootCutBound = res.BestBound
			if len(res.Values) == 0 {
				res.Objective = 0
			}
		}
		return
	}
	raw, err := trace.MarshalVIPRJSON()
	if err != nil {
		return
	}
	doc, okDoc := unmarshalCertVIPRDoc(raw)
	if !okDoc {
		return
	}

	var objUser *big.Rat
	hasPolishedInc := false
	if len(res.Values) == p.numVars && len(doc.Incumbent) == p.numVars {
		incRat := make([]*big.Rat, p.numVars)
		okInc := true
		contDrifted := false
		for j := range p.numVars {
			r, ok := parseVIPRRatValue(doc.Incumbent[j])
			if !ok {
				okInc = false
				break
			}
			v := res.Values[j]
			if isNonFinite(v) {
				okInc = false
				break
			}
			if p.varCat[j] != Continuous {
				if math.Floor(v) != v {
					okInc = false
					break
				}
				vr, okV := exactRatFromFloat(v)
				if !okV || vr.Cmp(r) != 0 {
					okInc = false
					break
				}
			} else {
				rf := exactRatToFloat(r)
				if math.Abs(v-rf) > 1e-5*max(1.0, math.Abs(rf)) {
					contDrifted = true
				}
			}
			incRat[j] = r
		}
		if okInc && verifyExactPrimalRat(p, incRat, true) {
			if objMin, _, okObj := evalExactMinAndLinObjRat(p, incRat); okObj {
				ou := objMin
				if p.sense == Maximize {
					ou = new(big.Rat).Neg(objMin)
				}
				objF := exactRatToFloat(ou)
				lbMin := trace.TargetLB()
				acceptDrifted := contDrifted &&
					doc.Mode == "optimal" &&
					res.Status == StatusOptimal &&
					lbMin != nil &&
					objMin.Cmp(lbMin) <= 0 &&
					!isNonFinite(res.Objective) &&
					math.Abs(res.Objective-objF) <= 1e-5*max(1.0, math.Abs(objF))
				if !contDrifted || acceptDrifted {
					for j := range p.numVars {
						res.Values[j] = exactRatToFloat(incRat[j])
					}
					if !isNonFinite(res.Objective) && math.Abs(res.Objective-objF) <= 1e-5*max(1.0, math.Abs(objF)) {
						res.Objective = objF
					}
					objUser = ou
					hasPolishedInc = true
				}
			}
		}
	}

	switch res.Status {
	case StatusOptimal, StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
		var bUser *big.Rat
		if doc.Mode == "optimal" {
			if objUser != nil {
				bUser = objUser
			} else if len(doc.Incumbent) == p.numVars {
				incTmp := make([]*big.Rat, p.numVars)
				okAll := true
				for j := range p.numVars {
					r, ok := parseVIPRRatValue(doc.Incumbent[j])
					if !ok {
						okAll = false
						break
					}
					incTmp[j] = r
				}
				if okAll {
					if ou, okO := evalExactUserObjRat(p, incTmp); okO {
						bUser = ou
					}
				}
			}
		} else if lbMin := trace.TargetLB(); lbMin != nil {
			bUser = userBoundFromMinRat(p, lbMin)
		}
		if bUser != nil {
			bf := exactRatToOutwardBound(p.sense, bUser)
			if isLimitStatus(res.Status) {
				if isNonFinite(res.BestBound) ||
					(p.sense == Maximize && res.BestBound < bf) ||
					(p.sense == Minimize && res.BestBound > bf) {
					res.BestBound = bf
				}
			} else if !isNonFinite(res.BestBound) {
				maxSnap := 1e-5 * max(1.0, math.Abs(bf))
				if doc.Mode == "optimal" {
					if math.Abs(res.BestBound-bf) <= maxSnap {
						res.BestBound = bf
					}
				} else if p.sense == Maximize {
					if res.BestBound < bf && bf-res.BestBound <= maxSnap {
						res.BestBound = bf
					}
				} else {
					if res.BestBound > bf && res.BestBound-bf <= maxSnap {
						res.BestBound = bf
					}
				}
			}
			if hasPolishedInc && !isNonFinite(res.Objective) {
				res.BestBound = clampBestBoundToObj(p.sense, res.BestBound, res.Objective)
			}
		}
		res.RootLPBound = clampRootBoundToBestBound(p.sense, res.RootLPBound, res.BestBound)
		res.RootCutBound = clampRootBoundToBestBound(p.sense, res.RootCutBound, res.BestBound)
	}
}

func isTrivialInfBound(sense Sense, b float64) bool {
	if sense == Maximize {
		return math.IsInf(b, 1)
	}
	return math.IsInf(b, -1)
}

func verifyLimitBestBoundAgainstCert(p *CompiledProblem, certMinLB *big.Rat, claimed float64, strictPolished bool) bool {
	if p == nil || certMinLB == nil || math.IsNaN(claimed) {
		return false
	}
	if isTrivialInfBound(p.sense, claimed) {
		return true
	}
	if math.IsInf(claimed, 0) {
		return false
	}
	bf := exactRatToOutwardBound(p.sense, userBoundFromMinRat(p, certMinLB))
	tol := 0.0
	if !strictPolished {
		tol = 1e-6 * max(1.0, math.Abs(bf))
	}
	if p.sense == Maximize {
		return claimed >= bf-tol
	}
	return claimed <= bf+tol
}

func exactUserReducedCosts(p *CompiledProblem, yMin []*big.Rat) ([]*big.Rat, bool) {
	if p == nil || len(yMin) != p.numConstraints || len(p.objMin) < p.numVars {
		return nil, false
	}
	n := p.numVars
	dots := make([]*big.Rat, n)
	for j := range n {
		dots[j] = new(big.Rat)
	}
	var prod big.Rat
	for i, yi := range yMin {
		if yi == nil {
			return nil, false
		}
		if yi.Sign() == 0 {
			continue
		}
		if eo := p.exactOrig; eo != nil && i < len(eo.rows) && eo.rows[i].hasDupLoss {
			er := eo.rows[i]
			for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
				if t.coeff == 0 {
					continue
				}
				if t.col < 0 || t.col >= len(dots) {
					return nil, false
				}
				av, okA := exactRatFromFloat(t.coeff)
				if !okA {
					return nil, false
				}
				prod.Mul(yi, av)
				dots[t.col].Add(dots[t.col], &prod)
			}
			continue
		}
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			fv := p.rowVal[k]
			if fv == 0 {
				continue
			}
			col := p.colIdx[k]
			if col < 0 || col >= len(dots) {
				return nil, false
			}
			av, okA := exactRatFromFloat(fv)
			if !okA {
				return nil, false
			}
			prod.Mul(yi, av)
			dots[col].Add(dots[col], &prod)
		}
	}
	rcUser := make([]*big.Rat, n)
	neg := p.sense == Maximize
	var objMinVec []*big.Rat
	if p.exactOrig != nil && p.exactOrig.objHasLoss {
		var okVec bool
		objMinVec, okVec = exactMinObjVecRat(p)
		if !okVec {
			return nil, false
		}
	}
	for j := range n {
		var cj *big.Rat
		if objMinVec != nil {
			cj = objMinVec[j]
		} else {
			var okC bool
			cj, okC = exactRatFromFloat(p.objMin[j])
			if !okC {
				return nil, false
			}
		}
		rcj := new(big.Rat).Sub(cj, dots[j])
		if neg {
			rcj.Neg(rcj)
		}
		rcUser[j] = rcj
	}
	return rcUser, true
}

func userBoundFromMinRat(p *CompiledProblem, minBnd *big.Rat) *big.Rat {
	if minBnd == nil {
		return nil
	}
	if p.sense == Maximize {
		return new(big.Rat).Neg(minBnd)
	}
	return new(big.Rat).Set(minBnd)
}

func evalExactMinAndLinObjRat(p *CompiledProblem, x []*big.Rat) (objMin, linAbs *big.Rat, ok bool) {
	if p == nil || len(x) != p.numVars {
		return nil, nil, false
	}
	cMin, okC := exactRatFromFloat(p.objConstMin())
	if !okC {
		return nil, nil, false
	}
	lin := new(big.Rat)
	var prod big.Rat
	if eo := p.exactOrig; eo != nil && eo.objHasLoss {
		for j := range p.numVars {
			if x[j] == nil {
				return nil, nil, false
			}
		}
		for _, t := range eo.objTerms {
			if t.col < 0 || t.col >= p.numVars {
				return nil, nil, false
			}
			if t.coeff == 0 || x[t.col].Sign() == 0 {
				continue
			}
			cj, okJ := exactRatFromFloat(t.coeff)
			if !okJ {
				return nil, nil, false
			}
			prod.Mul(cj, x[t.col])
			lin.Add(lin, &prod)
		}
		if p.sense == Maximize && lin.Sign() != 0 {
			lin.Neg(lin)
		}
	} else {
		for j := range p.numVars {
			if x[j] == nil {
				return nil, nil, false
			}
			if p.objMin[j] == 0 || x[j].Sign() == 0 {
				continue
			}
			cj, okJ := exactRatFromFloat(p.objMin[j])
			if !okJ {
				return nil, nil, false
			}
			prod.Mul(cj, x[j])
			lin.Add(lin, &prod)
		}
	}
	objMin = new(big.Rat).Add(cMin, lin)
	linAbs = new(big.Rat).Abs(lin)
	return objMin, linAbs, true
}

func evalExactUserObjRat(p *CompiledProblem, x []*big.Rat) (*big.Rat, bool) {
	objMin, _, ok := evalExactMinAndLinObjRat(p, x)
	if !ok {
		return nil, false
	}
	if p.sense == Maximize {
		return new(big.Rat).Neg(objMin), true
	}
	return objMin, true
}

func exactGapClosedRat(p *CompiledProblem, x []*big.Rat, safeMinBnd *big.Rat, relTol, absTol float64) bool {
	if safeMinBnd == nil {
		return false
	}
	objMin, linAbs, ok := evalExactMinAndLinObjRat(p, x)
	if !ok {
		return false
	}
	gap := new(big.Rat).Sub(objMin, safeMinBnd)
	if gap.Sign() < 0 {
		return false
	}
	if gap.Sign() == 0 {
		return true
	}

	cleanAbs := max(0.0, absTol)
	if math.IsNaN(absTol) {
		cleanAbs = 0
	}
	if math.IsInf(cleanAbs, 1) {
		return true
	}
	if absRat, okA := exactRatFromFloat(cleanAbs); okA && gap.Cmp(absRat) <= 0 {
		return true
	}

	cleanRel := max(0.0, relTol)
	if math.IsNaN(relTol) {
		cleanRel = 0
	}
	if math.IsInf(cleanRel, 1) {
		return true
	}
	relRat, okR := exactRatFromFloat(cleanRel)
	if !okR {
		return false
	}
	denom := big.NewRat(1, 1)
	if linAbs.Cmp(denom) > 0 {
		denom.Set(linAbs)
	}
	rhs := new(big.Rat).Mul(relRat, denom)
	return gap.Cmp(rhs) <= 0
}

func floatBestBoundValidAndClosed(p *CompiledProblem, x []*big.Rat, safeMinBnd *big.Rat, res Result, opt Options, strictFloat bool) bool {
	if isNonFinite(res.Objective) || isNonFinite(res.BestBound) {
		return false
	}
	bUser := userBoundFromMinRat(p, safeMinBnd)
	bUserFloat := exactRatToOutwardBound(p.sense, bUser)
	tol := 1e-6 * max(1.0, math.Abs(bUserFloat))
	if strictFloat {
		tol = 0
	}
	// Reject bounds stronger than certified: Minimize cannot exceed bUserFloat; Maximize cannot fall below bUserFloat.
	if p.sense == Maximize {
		if res.BestBound < bUserFloat-tol {
			return false
		}
	} else {
		if res.BestBound > bUserFloat+tol {
			return false
		}
	}

	var diff float64
	if p.sense == Maximize {
		diff = res.BestBound - res.Objective
	} else {
		diff = res.Objective - res.BestBound
	}
	ulpSlack := 4.0 * 0x1p-53 * max(1.0, math.Abs(res.Objective))
	if diff < -ulpSlack {
		return false
	}
	cleanRel, cleanAbs := opt.effectiveGapTols()
	_, linAbs, ok := evalExactMinAndLinObjRat(p, x)
	if !ok {
		return false
	}
	linMag := max(1.0, exactRatToFloat(linAbs))
	if diff <= cleanAbs+ulpSlack {
		return true
	}
	if math.IsInf(cleanRel, 1) {
		return true
	}
	if cleanRel == 0 {
		return diff <= ulpSlack
	}
	if math.IsInf(linMag, 1) {
		return true
	}
	return diff <= cleanRel*linMag+ulpSlack
}

func approxLPPrimalFeasible(p *CompiledProblem, vals []float64) bool {
	if p == nil || len(vals) != p.numVars {
		return false
	}
	for j := range p.numVars {
		v := vals[j]
		if isNonFinite(v) {
			return false
		}
		if !math.IsInf(p.varLB[j], -1) {
			tol := 1e-5 * max(1.0, math.Abs(p.varLB[j]))
			if v < p.varLB[j]-tol {
				return false
			}
		}
		if !math.IsInf(p.varUB[j], 1) {
			tol := 1e-5 * max(1.0, math.Abs(p.varUB[j]))
			if v > p.varUB[j]+tol {
				return false
			}
		}
	}
	for i := range p.numConstraints {
		var sum, l1 float64
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			prod := p.rowVal[k] * vals[p.colIdx[k]]
			sum += prod
			l1 += math.Abs(prod)
		}
		rhs := p.rowRHS[i]
		tol := 1e-5 * max(1.0, math.Abs(rhs)+l1)
		switch p.rowSense[i] {
		case LessEqual:
			if sum > rhs+tol {
				return false
			}
		case GreaterEqual:
			if sum < rhs-tol {
				return false
			}
		case Equal:
			if math.Abs(sum-rhs) > tol {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func approxLPPrimalDualValid(p *CompiledProblem, res Result, opt Options) bool {
	n := p.numVars
	m := p.numConstraints
	if len(res.Values) != n || len(res.Duals) != m {
		return false
	}
	if len(res.ReducedCosts) != 0 && len(res.ReducedCosts) != n {
		return false
	}
	if isNonFinite(res.Objective) || isNonFinite(res.BestBound) {
		return false
	}
	if !approxLPPrimalFeasible(p, res.Values) {
		return false
	}
	if slices.ContainsFunc(res.ReducedCosts, isNonFinite) {
		return false
	}
	evalObj := p.EvaluateObjective(res.Values)
	if isNonFinite(evalObj) || math.Abs(res.Objective-evalObj) > 1e-5*max(1.0, math.Abs(evalObj)) {
		return false
	}

	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	yMin := make([]float64, m)
	dualBnd := p.objConstMin()
	for i := range m {
		d := res.Duals[i]
		if isNonFinite(d) {
			return false
		}
		yi := senseSign * d
		tolY := 1e-5 * max(1.0, math.Abs(yi))
		if p.rowSense[i] == LessEqual && yi > tolY {
			return false
		}
		if p.rowSense[i] == GreaterEqual && yi < -tolY {
			return false
		}
		if p.rowSense[i] == LessEqual && yi > 0 {
			yi = 0
		}
		if p.rowSense[i] == GreaterEqual && yi < 0 {
			yi = 0
		}
		yMin[i] = yi
		dualBnd += yi * p.rowRHS[i]
	}

	for j := range n {
		var dot, dotL1 float64
		for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
			prod := yMin[p.rowIdx[idx]] * p.colVal[idx]
			dot += prod
			dotL1 += math.Abs(prod)
		}
		rcMin := p.objMin[j] - dot
		rcTol := 1e-5 * max(1.0, math.Abs(p.objMin[j])+dotL1)
		if len(res.ReducedCosts) == n {
			rcClaimMin := senseSign * res.ReducedCosts[j]
			if math.Abs(rcClaimMin-rcMin) > 1e-4*max(1.0, math.Abs(rcMin)+dotL1) {
				return false
			}
		}
		if rcMin > rcTol {
			if math.IsInf(p.varLB[j], -1) {
				return false
			}
			dualBnd += rcMin * p.varLB[j]
		} else if rcMin < -rcTol {
			if math.IsInf(p.varUB[j], 1) {
				return false
			}
			dualBnd += rcMin * p.varUB[j]
		} else {
			if rcMin > 0 && !math.IsInf(p.varLB[j], -1) {
				dualBnd += rcMin * p.varLB[j]
			} else if rcMin < 0 && !math.IsInf(p.varUB[j], 1) {
				dualBnd += rcMin * p.varUB[j]
			}
		}
	}

	primalMin := minObjective(p, res.Values)
	scale := max(1.0, math.Abs(primalMin)+math.Abs(dualBnd))
	cleanRel, cleanAbs := opt.effectiveGapTols()
	allowed := max(1e-4*scale, max(cleanAbs, cleanRel*scale))
	return math.Abs(primalMin-dualBnd) <= allowed
}

func userDualsToCanonicalMinRat(p *CompiledProblem, duals []float64) ([]*big.Rat, bool) {
	m := p.numConstraints
	if len(duals) != m {
		return nil, false
	}
	neg := p.sense == Maximize
	out := make([]*big.Rat, m)
	for i := range m {
		r, ok := exactRatFromFloat(duals[i])
		if !ok {
			return nil, false
		}
		if neg {
			r.Neg(r)
		}
		if p.rowSense[i] == LessEqual && r.Sign() > 0 {
			r.SetInt64(0)
		}
		if p.rowSense[i] == GreaterEqual && r.Sign() < 0 {
			r.SetInt64(0)
		}
		out[i] = r
	}
	return out, true
}

func verifyDirectInfeas(p *CompiledProblem) bool {
	if p == nil {
		return false
	}
	for j := range p.numVars {
		lo, hi := p.varLB[j], p.varUB[j]
		if math.IsInf(lo, 1) || math.IsInf(hi, -1) {
			return true
		}
		if !math.IsInf(lo, -1) && !math.IsInf(hi, 1) {
			lr, okL := exactRatFromFloat(lo)
			ur, okU := exactRatFromFloat(hi)
			if okL && okU && lr.Cmp(ur) > 0 {
				return true
			}
		}
	}
	for i := range p.numConstraints {
		cols, _, okC := exactCanonicalRowTermsRat(p, i)
		if !okC || len(cols) > 0 {
			continue
		}
		rhs, ok := exactRowRHSRat(p, i)
		if !ok {
			continue
		}
		switch p.rowSense[i] {
		case LessEqual:
			if rhs.Sign() < 0 {
				return true
			}
		case GreaterEqual:
			if rhs.Sign() > 0 {
				return true
			}
		case Equal:
			if rhs.Sign() != 0 {
				return true
			}
		}
	}
	return false
}

func verifyExactFarkasRayRat(p *CompiledProblem, w []*big.Rat) bool {
	if p == nil || p.invalidErr != nil || p.numConstraints == 0 || len(w) != p.numConstraints {
		return false
	}
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return false
	}
	anyNZ := false
	for i := range rp.m {
		wi := w[i]
		if wi == nil {
			return false
		}
		if rp.rowSense[i] == LessEqual && wi.Sign() < 0 {
			return false
		}
		if rp.rowSense[i] == GreaterEqual && wi.Sign() > 0 {
			return false
		}
		if wi.Sign() != 0 {
			anyNZ = true
		}
	}
	if !anyNZ {
		return false
	}
	_, okProj := rp.projectVIPRFarkas(w, rp.lb, rp.ub)
	return okProj
}

func extractElasticPhase1Farkas(stop exactStop, p *CompiledProblem, rp *viprRatProblem) ([]*big.Rat, bool) {
	if stop.canceled() || p == nil || rp == nil || rp.m == 0 {
		return nil, false
	}
	m := NewModel("cert_phase1", Minimize)
	vars := make([]Var, p.numVars)
	for j := range p.numVars {
		vars[j] = m.AddContVar("", p.varLB[j], p.varUB[j])
	}
	var objTerms []Term
	for i := range p.numConstraints {
		var terms []Term
		rhsVal := p.rowRHS[i]
		var exprConst float64
		if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasDupLoss || (eo.rows[i].rhsInexact && !eo.rows[i].intTightened)) {
			er := eo.rows[i]
			if er.hasDupLoss {
				terms = make([]Term, 0, (er.termEnd-er.termStart)+2)
				for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
					terms = append(terms, Term{Var: vars[t.col], Coeff: t.coeff})
				}
			} else {
				st, en := p.rowPtr[i], p.rowPtr[i+1]
				terms = make([]Term, 0, (en-st)+2)
				for k := st; k < en; k++ {
					if p.rowVal[k] != 0 {
						terms = append(terms, Term{Var: vars[p.colIdx[k]], Coeff: p.rowVal[k]})
					}
				}
			}
			if er.rhsInexact && !er.intTightened {
				rhsVal = er.rhs
				exprConst = er.exprConst
			}
		} else {
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			terms = make([]Term, 0, (en-st)+2)
			for k := st; k < en; k++ {
				if p.rowVal[k] != 0 {
					terms = append(terms, Term{Var: vars[p.colIdx[k]], Coeff: p.rowVal[k]})
				}
			}
		}
		switch p.rowSense[i] {
		case LessEqual:
			sp := m.AddContVar("", 0, math.Inf(1))
			terms = append(terms, Term{Var: sp, Coeff: -1})
			objTerms = append(objTerms, Term{Var: sp, Coeff: 1})
			m.AddConstraint("", Le(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		case GreaterEqual:
			sn := m.AddContVar("", 0, math.Inf(1))
			terms = append(terms, Term{Var: sn, Coeff: 1})
			objTerms = append(objTerms, Term{Var: sn, Coeff: 1})
			m.AddConstraint("", Ge(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		case Equal:
			sp := m.AddContVar("", 0, math.Inf(1))
			sn := m.AddContVar("", 0, math.Inf(1))
			terms = append(terms, Term{Var: sp, Coeff: -1}, Term{Var: sn, Coeff: 1})
			objTerms = append(objTerms, Term{Var: sp, Coeff: 1}, Term{Var: sn, Coeff: 1})
			m.AddConstraint("", Eq(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		}
	}
	m.SetObjective(NewExpr(objTerms...))
	ep := m.Compile()
	if ep == nil || ep.invalidErr != nil {
		return nil, false
	}
	lp := NewLPSolver(ep, nil)
	defer lp.Close()
	stop.applyToLPSolver(lp)
	lpRes := lp.Solve()
	if lpRes.Status != StatusOptimal {
		return nil, false
	}
	snap, okSnap := buildExactSolverSnapshot(stop, lp)
	if okSnap {
		if yMin, _, _, _, okD := snap.extractExactDuals(stop); okD && len(yMin) >= rp.m {
			neg := make([]*big.Rat, rp.m)
			for i := range rp.m {
				neg[i] = new(big.Rat).Neg(yMin[i])
			}
			if w, okW := rp.projectVIPRFarkas(neg, rp.lb, rp.ub); okW {
				return w, true
			}
		}
	}
	if len(lpRes.Dual) >= rp.m {
		cand := make([]*big.Rat, rp.m)
		allOK := true
		for i := range rp.m {
			r, okR := exactRatFromFloat(-lpRes.Dual[i])
			if !okR {
				allOK = false
				break
			}
			cand[i] = r
		}
		if allOK {
			if w, okW := rp.projectVIPRFarkas(cand, rp.lb, rp.ub); okW {
				return w, true
			}
		}
	}
	return nil, false
}

func buildCertifiedLPWitness(stop exactStop, p *CompiledProblem, opt Options, res *Result) *lpCertWitness {
	if p == nil || p.invalidErr != nil || res == nil {
		return nil
	}
	if res.Status == StatusTimeLimit || res.Status == StatusNodeLimit || res.Status == StatusIterationLimit {
		res.RootLPBound = clampRootBoundToBestBound(p.sense, res.RootLPBound, res.BestBound)
		res.RootCutBound = clampRootBoundToBestBound(p.sense, res.RootCutBound, res.BestBound)
		if len(res.Values) == 0 {
			return &lpCertWitness{polished: true}
		}
		if len(res.Values) != p.numVars || !approxLPPrimalFeasible(p, res.Values) {
			return nil
		}
		if xDirect, okP := exactPrimalIfFeasible(p, res.Values, false); okP {
			if objU, okO := evalExactUserObjRat(p, xDirect); okO {
				res.Objective = exactRatToFloat(objU)
				return &lpCertWitness{primal: xDirect, polished: true}
			}
		}
		return nil
	}
	if stop.canceled() {
		return nil
	}
	switch res.Status {
	case StatusOptimal:
		if !approxLPPrimalDualValid(p, *res, opt) {
			return nil
		}
		if p.numVars == 0 {
			emptyX := make([]*big.Rat, p.numVars)
			if !verifyExactPrimalRat(p, emptyX, false) {
				return nil
			}
			zeroDuals := make([]*big.Rat, p.numConstraints)
			for i := range p.numConstraints {
				zeroDuals[i] = new(big.Rat)
			}
			for i := range res.Duals {
				res.Duals[i] = 0
			}
			res.ReducedCosts = nil
			if objU, okO := evalExactUserObjRat(p, emptyX); okO {
				res.Objective = exactRatToFloat(objU)
				res.BestBound = exactRatToOutwardBound(p.sense, objU)
			} else {
				res.Objective = p.objConstant
				res.BestBound = p.objConstant
			}
			res.RootLPBound = res.BestBound
			res.RootCutBound = res.BestBound
			return &lpCertWitness{primal: emptyX, dualMin: zeroDuals, polished: true}
		}

		relTol, absTol := opt.effectiveGapTols()
		xDirect, okP := exactPrimalIfFeasible(p, res.Values, false)
		yDirect, okY := userDualsToCanonicalMinRat(p, res.Duals)
		if okP && okY {
			if safeBnd, okS := ExactSafeDualBound(p, yDirect); okS && exactGapClosedRat(p, xDirect, safeBnd, relTol, absTol) {
				objU, okO := evalExactUserObjRat(p, xDirect)
				rcUser, okRC := exactUserReducedCosts(p, yDirect)
				if okO && okRC {
					res.Objective = exactRatToFloat(objU)
					res.BestBound = exactRatToOutwardBound(p.sense, userBoundFromMinRat(p, safeBnd))
					res.BestBound = clampBestBoundToObj(p.sense, res.BestBound, res.Objective)
					res.RootLPBound = res.BestBound
					res.RootCutBound = res.BestBound
					neg := p.sense == Maximize
					for i, yi := range yDirect {
						f := exactRatToFloat(yi)
						if neg && f != 0 {
							f = -f
						}
						res.Duals[i] = f
					}
					if len(res.ReducedCosts) != p.numVars {
						res.ReducedCosts = make([]float64, p.numVars)
					}
					for j, r := range rcUser {
						res.ReducedCosts[j] = exactRatToFloat(r)
					}
					return &lpCertWitness{primal: xDirect, dualMin: yDirect, polished: true}
				}
			}
		}

		lp := NewLPSolver(p, nil)
		defer lp.Close()
		lp.DisableSIMD = opt.DisableSIMD
		stop.applyToLPSolver(lp)
		lpRes := lp.Solve()
		if lpRes.Status != StatusOptimal {
			return nil
		}
		if sol, okSol := exactBasisSolutionStop(stop, lp); okSol {
			solObjF := exactRatToFloat(sol.ObjectiveUser)
			if math.Abs(res.Objective-solObjF) > 1e-4*max(1.0, math.Abs(solObjF)) {
				return nil
			}
			for j, r := range sol.Primal {
				res.Values[j] = exactRatToFloat(r)
			}
			for i, r := range sol.DualUser {
				res.Duals[i] = exactRatToFloat(r)
			}
			if len(res.ReducedCosts) != p.numVars {
				res.ReducedCosts = make([]float64, p.numVars)
			}
			for j, r := range sol.ReducedCost {
				res.ReducedCosts[j] = exactRatToFloat(r)
			}
			res.Objective = solObjF
			res.BestBound = exactRatToOutwardBound(p.sense, sol.ObjectiveUser)
			res.RootLPBound = res.BestBound
			res.RootCutBound = res.BestBound
			return &lpCertWitness{primal: sol.Primal, dualMin: sol.DualMin, polished: true}
		}

		if okP {
			if snap, okSnap := buildExactSolverSnapshot(stop, lp); okSnap {
				if yMin, _, dualUser, rcUser, okD := snap.extractExactDuals(stop); okD {
					if safeBnd, okS := ExactSafeDualBound(p, yMin); okS && exactGapClosedRat(p, xDirect, safeBnd, relTol, absTol) {
						for i, r := range dualUser {
							res.Duals[i] = exactRatToFloat(r)
						}
						if len(res.ReducedCosts) != p.numVars {
							res.ReducedCosts = make([]float64, p.numVars)
						}
						for j, r := range rcUser {
							res.ReducedCosts[j] = exactRatToFloat(r)
						}
						objU, _ := evalExactUserObjRat(p, xDirect)
						res.Objective = exactRatToFloat(objU)
						res.BestBound = exactRatToOutwardBound(p.sense, userBoundFromMinRat(p, safeBnd))
						res.BestBound = clampBestBoundToObj(p.sense, res.BestBound, res.Objective)
						res.RootLPBound = res.BestBound
						res.RootCutBound = res.BestBound
						return &lpCertWitness{primal: xDirect, dualMin: yMin, polished: true}
					}
				}
			}
		}
		return nil

	case StatusInfeasible:
		res.RootLPBound = 0
		res.RootCutBound = 0
		if verifyDirectInfeas(p) {
			return &lpCertWitness{hasDirectInfeas: true}
		}
		rp, okRP := buildVIPRRatProblem(p)
		if !okRP || rp.m == 0 {
			return nil
		}
		if len(res.Duals) == rp.m {
			if w, okW := rp.extractNodeFarkas(nil, res.Duals, rp.lb, rp.ub); okW {
				return &lpCertWitness{farkasW: w}
			}
		}
		lp := NewLPSolver(p, nil)
		lp.DisableSIMD = opt.DisableSIMD
		stop.applyToLPSolver(lp)
		lpRes := lp.Solve()
		if lpRes.Status == StatusInfeasible {
			if w, okW := rp.extractNodeFarkas(lp, lpRes.Dual, rp.lb, rp.ub); okW {
				lp.Close()
				return &lpCertWitness{farkasW: w}
			}
		}
		lp.Close()

		if w, okEl := extractElasticPhase1Farkas(stop, p, rp); okEl {
			return &lpCertWitness{farkasW: w}
		}
		return nil

	case StatusUnbounded:
		res.RootLPBound = 0
		res.RootCutBound = 0
		certUnb, okUnb := extractUnboundedCertificateStop(stop, p, nil, res.Values)
		if !okUnb {
			return nil
		}
		return &lpCertWitness{unbounded: certUnb, hasUnbounded: true}

	default:
		return nil
	}
}

func verifyCertifiedLP(p *CompiledProblem, opt Options, res Result, wit *lpCertWitness) error {
	if p == nil || p.invalidErr != nil || wit == nil {
		return ErrCertificationFailed
	}
	n := p.numVars
	m := p.numConstraints

	switch res.Status {
	case StatusOptimal:
		if len(res.Values) != n || len(res.Duals) != m || len(res.ReducedCosts) != n || len(wit.primal) != n || len(wit.dualMin) != m {
			return ErrCertificationFailed
		}
		if !verifyExactPrimalRat(p, wit.primal, false) {
			return ErrCertificationFailed
		}
		for j := range n {
			if isNonFinite(res.Values[j]) || res.Values[j] != exactRatToFloat(wit.primal[j]) {
				return ErrCertificationFailed
			}
		}
		neg := p.sense == Maximize
		for i := range m {
			d := res.Duals[i]
			if isNonFinite(d) {
				return ErrCertificationFailed
			}
			want := exactRatToFloat(wit.dualMin[i])
			if neg && want != 0 {
				want = -want
			}
			if d != want {
				return ErrCertificationFailed
			}
		}
		if n > 0 {
			rcUser, okRC := exactUserReducedCosts(p, wit.dualMin)
			if !okRC {
				return ErrCertificationFailed
			}
			for j := range n {
				rc := res.ReducedCosts[j]
				if isNonFinite(rc) || rc != exactRatToFloat(rcUser[j]) {
					return ErrCertificationFailed
				}
			}
		}
		safeMinBnd, okSafe := ExactSafeDualBound(p, wit.dualMin)
		if !okSafe {
			return ErrCertificationFailed
		}
		relTol, absTol := opt.effectiveGapTols()
		if !exactGapClosedRat(p, wit.primal, safeMinBnd, relTol, absTol) {
			return ErrCertificationFailed
		}
		objUser, okObj := evalExactUserObjRat(p, wit.primal)
		if !okObj {
			return ErrCertificationFailed
		}
		if isNonFinite(res.Objective) || res.Objective != exactRatToFloat(objUser) {
			return ErrCertificationFailed
		}
		if !floatBestBoundValidAndClosed(p, wit.primal, safeMinBnd, res, opt, wit.polished) {
			return ErrCertificationFailed
		}
		if isNonFinite(res.RootLPBound) || isNonFinite(res.RootCutBound) ||
			res.RootLPBound != res.BestBound || res.RootCutBound != res.BestBound {
			return ErrCertificationFailed
		}
		return nil

	case StatusInfeasible:
		if len(res.Values) != 0 || len(res.Duals) != 0 || len(res.ReducedCosts) != 0 ||
			res.Objective != 0 || math.IsNaN(res.BestBound) ||
			res.RootLPBound != 0 || res.RootCutBound != 0 {
			return ErrCertificationFailed
		}
		if wit.hasDirectInfeas && verifyDirectInfeas(p) {
			return nil
		}
		if verifyExactFarkasRayRat(p, wit.farkasW) {
			return nil
		}
		return ErrCertificationFailed

	case StatusUnbounded:
		if len(res.Values) != 0 || len(res.Duals) != 0 || len(res.ReducedCosts) != 0 ||
			res.Objective != 0 || math.IsNaN(res.BestBound) ||
			res.RootLPBound != 0 || res.RootCutBound != 0 {
			return ErrCertificationFailed
		}
		if !wit.hasUnbounded || !VerifyUnboundedCertificate(p, wit.unbounded) {
			return ErrCertificationFailed
		}
		return nil

	case StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
		if len(res.Duals) != 0 || len(res.ReducedCosts) != 0 || math.IsNaN(res.BestBound) || math.IsNaN(res.Objective) {
			return ErrCertificationFailed
		}
		if !verifyRootBoundAgainstBest(p.sense, res.RootLPBound, res.BestBound, wit.polished) ||
			!verifyRootBoundAgainstBest(p.sense, res.RootCutBound, res.BestBound, wit.polished) {
			return ErrCertificationFailed
		}
		if len(wit.dualMin) == m && m > 0 {
			safeMinBnd, okSafe := ExactSafeDualBound(p, wit.dualMin)
			if !okSafe || !verifyLimitBestBoundAgainstCert(p, safeMinBnd, res.BestBound, wit.polished) {
				return ErrCertificationFailed
			}
		} else if !isTrivialInfBound(p.sense, res.BestBound) {
			return ErrCertificationFailed
		}
		if len(res.Values) == 0 {
			if res.Objective != 0 {
				return ErrCertificationFailed
			}
			return nil
		}
		if len(res.Values) != n || len(wit.primal) != n {
			return ErrCertificationFailed
		}
		if !verifyExactPrimalRat(p, wit.primal, false) {
			return ErrCertificationFailed
		}
		for j := range n {
			if isNonFinite(res.Values[j]) || res.Values[j] != exactRatToFloat(wit.primal[j]) {
				return ErrCertificationFailed
			}
		}
		objUser, okObj := evalExactUserObjRat(p, wit.primal)
		if !okObj || isNonFinite(res.Objective) || res.Objective != exactRatToFloat(objUser) {
			return ErrCertificationFailed
		}
		return nil

	default:
		return ErrCertificationFailed
	}
}

func verifyCertifiedMIP(
	stop exactStop,
	p *CompiledProblem,
	trace *VIPRTrace,
	wit *lpCertWitness,
	res Result,
	opt Options,
	strictPolished bool,
) error {
	if p == nil || p.invalidErr != nil {
		return ErrCertificationFailed
	}
	n := p.numVars

	switch res.Status {
	case StatusInfeasible:
		if len(res.Values) != 0 || len(res.Duals) != 0 || len(res.ReducedCosts) != 0 ||
			math.IsNaN(res.Objective) || math.IsNaN(res.BestBound) ||
			isNonFinite(res.RootLPBound) || isNonFinite(res.RootCutBound) {
			return ErrCertificationFailed
		}
		if strictPolished && res.Objective != 0 {
			return ErrCertificationFailed
		}
		if !p.isMIP && strictPolished && (res.RootLPBound != 0 || res.RootCutBound != 0) {
			return ErrCertificationFailed
		}
		if trace == nil || !trace.HasCertificate() {
			return ErrCertificationFailed
		}
		raw, err := trace.MarshalVIPRJSON()
		if err != nil {
			return ErrCertificationFailed
		}
		chk, errChk := verifyMiniVIPRProblem(p, raw)
		if errChk != nil || chk.mode != "infeasible" {
			return ErrCertificationFailed
		}
		return nil

	case StatusOptimal:
		if len(res.Values) != n || isNonFinite(res.Objective) || isNonFinite(res.BestBound) {
			return ErrCertificationFailed
		}
		if p.isMIP {
			if len(res.Duals) != 0 || len(res.ReducedCosts) != 0 {
				return ErrCertificationFailed
			}
		} else {
			if len(res.Duals) != 0 {
				if len(res.Duals) != p.numConstraints || slices.ContainsFunc(res.Duals, isNonFinite) {
					return ErrCertificationFailed
				}
			}
			if len(res.ReducedCosts) != 0 {
				if len(res.ReducedCosts) != n || slices.ContainsFunc(res.ReducedCosts, isNonFinite) {
					return ErrCertificationFailed
				}
			}
		}
		if !verifyRootBoundAgainstBest(p.sense, res.RootLPBound, res.BestBound, strictPolished) ||
			!verifyRootBoundAgainstBest(p.sense, res.RootCutBound, res.BestBound, strictPolished) {
			return ErrCertificationFailed
		}
		if !p.isMIP && strictPolished && (res.RootLPBound != res.BestBound || res.RootCutBound != res.BestBound) {
			return ErrCertificationFailed
		}
		if trace == nil || !trace.HasCertificate() {
			return ErrCertificationFailed
		}
		raw, err := trace.MarshalVIPRJSON()
		if err != nil {
			return ErrCertificationFailed
		}
		chk, errChk := verifyMiniVIPRProblem(p, raw)
		if errChk != nil || (chk.mode != "optimal" && chk.mode != "bound") {
			return ErrCertificationFailed
		}
		if len(chk.incumbent) != n || chk.certifiedMinLB == nil {
			return ErrCertificationFailed
		}
		if !verifyExactIncumbent(p, res.Values, chk.incumbent, strictPolished) {
			return ErrCertificationFailed
		}
		relTol, absTol := opt.effectiveGapTols()
		if relTol == 0 && absTol == 0 && chk.mode != "optimal" {
			return ErrCertificationFailed
		}
		if !exactGapClosedRat(p, chk.incumbent, chk.certifiedMinLB, relTol, absTol) {
			return ErrCertificationFailed
		}
		objUser, okObj := evalExactUserObjRat(p, chk.incumbent)
		if !okObj {
			return ErrCertificationFailed
		}
		objF := exactRatToFloat(objUser)
		if strictPolished {
			if res.Objective != objF {
				return ErrCertificationFailed
			}
		} else if math.Abs(res.Objective-objF) > 1e-6*max(1.0, math.Abs(objF)) {
			return ErrCertificationFailed
		}
		if !floatBestBoundValidAndClosed(p, chk.incumbent, chk.certifiedMinLB, res, opt, strictPolished) {
			return ErrCertificationFailed
		}
		return nil

	case StatusUnbounded:
		if len(res.Values) != 0 || len(res.Duals) != 0 || len(res.ReducedCosts) != 0 ||
			math.IsNaN(res.Objective) || math.IsNaN(res.BestBound) ||
			isNonFinite(res.RootLPBound) || isNonFinite(res.RootCutBound) {
			return ErrCertificationFailed
		}
		if strictPolished && res.Objective != 0 {
			return ErrCertificationFailed
		}
		if !p.isMIP && strictPolished && (res.RootLPBound != 0 || res.RootCutBound != 0) {
			return ErrCertificationFailed
		}
		if wit != nil {
			if !wit.hasUnbounded || !VerifyUnboundedCertificate(p, wit.unbounded) {
				return ErrCertificationFailed
			}
			return nil
		}
		certUnb, okUnb := extractUnboundedCertificateStop(stop, p, nil, nil)
		if !okUnb || !VerifyUnboundedCertificate(p, certUnb) {
			return ErrCertificationFailed
		}
		return nil

	case StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
		if len(res.Duals) != 0 || len(res.ReducedCosts) != 0 || math.IsNaN(res.BestBound) || math.IsNaN(res.Objective) {
			return ErrCertificationFailed
		}
		if !verifyRootBoundAgainstBest(p.sense, res.RootLPBound, res.BestBound, strictPolished) ||
			!verifyRootBoundAgainstBest(p.sense, res.RootCutBound, res.BestBound, strictPolished) {
			return ErrCertificationFailed
		}
		var traceInc []*big.Rat
		if trace != nil && trace.HasCertificate() {
			raw, err := trace.MarshalVIPRJSON()
			if err != nil {
				return ErrCertificationFailed
			}
			chk, errChk := verifyMiniVIPRProblem(p, raw)
			if errChk != nil {
				return ErrCertificationFailed
			}
			traceInc = chk.incumbent
			if chk.mode == "infeasible" {
				if len(res.Values) != 0 || res.Objective != 0 {
					return ErrCertificationFailed
				}
				return nil
			}
			if !verifyLimitBestBoundAgainstCert(p, chk.certifiedMinLB, res.BestBound, strictPolished) {
				return ErrCertificationFailed
			}
		} else if !isTrivialInfBound(p.sense, res.BestBound) {
			return ErrCertificationFailed
		}
		if len(res.Values) == 0 {
			if res.Objective != 0 {
				return ErrCertificationFailed
			}
			return nil
		}
		if len(res.Values) != n || isNonFinite(res.Objective) {
			return ErrCertificationFailed
		}
		for j := range n {
			v := res.Values[j]
			if isNonFinite(v) {
				return ErrCertificationFailed
			}
			if p.varCat[j] != Continuous && math.Floor(v) != v {
				return ErrCertificationFailed
			}
		}
		var xVerified []*big.Rat
		if len(traceInc) == n && verifyExactIncumbent(p, res.Values, traceInc, strictPolished) && verifyExactPrimalRat(p, traceInc, true) {
			xVerified = traceInc
		} else if xCand, okCand := floatSliceToExactPrimal(p, res.Values, true, false); okCand {
			xVerified = xCand
		} else if !strictPolished && approxLPPrimalFeasible(p, res.Values) {
			if xRef, okRef := refineExactMIPAnchor(stop, p, res.Values); okRef &&
				verifyExactIncumbent(p, res.Values, xRef, false) &&
				verifyExactPrimalRat(p, xRef, true) {
				xVerified = xRef
			}
		}
		if xVerified == nil {
			return ErrCertificationFailed
		}
		objUser, okObj := evalExactUserObjRat(p, xVerified)
		if !okObj {
			return ErrCertificationFailed
		}
		objF := exactRatToFloat(objUser)
		if strictPolished {
			if res.Objective != objF {
				return ErrCertificationFailed
			}
		} else if math.Abs(res.Objective-objF) > 1e-6*max(1.0, math.Abs(objF)) {
			return ErrCertificationFailed
		}
		return nil

	default:
		return ErrCertificationFailed
	}
}

func verifyExactIncumbent(p *CompiledProblem, vals []float64, xExact []*big.Rat, strictPolished bool) bool {
	if len(vals) != p.numVars || len(xExact) != p.numVars {
		return false
	}
	for j := range p.numVars {
		v := vals[j]
		if isNonFinite(v) || xExact[j] == nil {
			return false
		}
		if p.varCat[j] != Continuous {
			if math.Floor(v) != v || !xExact[j].IsInt() {
				return false
			}
			vr, ok := exactRatFromFloat(v)
			if !ok || vr.Cmp(xExact[j]) != 0 {
				return false
			}
		} else {
			want := exactRatToFloat(xExact[j])
			if strictPolished {
				if v != want {
					return false
				}
			} else if math.Abs(v-want) > 1e-6*max(1.0, math.Abs(want)) {
				return false
			}
		}
	}
	if !strictPolished && !approxLPPrimalFeasible(p, vals) {
		return false
	}
	return true
}

// Pure-Go exact rational Mini-VIPR verifier mirroring GoblinFormal.Track4.checkMiniVIPR.

type certRatTerm struct {
	col int
	val *big.Rat
}

type certLinCon struct {
	terms []certRatTerm
	sense ConstraintSense
	rhs   *big.Rat
}

type certDerivedEntry struct {
	con         certLinCon
	assumptions []int
}

type certVIPRInstance struct {
	n        int
	objConst *big.Rat
	objDense []*big.Rat
	objTerms []certRatTerm
	isInt    []bool
	lb       []*big.Rat
	ub       []*big.Rat
	baseRows []certLinCon
}

type certVIPRVerified struct {
	mode           string
	incumbent      []*big.Rat
	certifiedMinLB *big.Rat
}

const (
	maxVIPRVerifyVars = 10_000_000
	maxVIPRRatStrLen  = 8192
	maxVIPRRatBits    = 16384
)

type certLinRefDoc struct {
	ID   *int        `json:"id"`
	Mult viprRawJSON `json:"mult"`
}

type certBoundRefDoc struct {
	Col     *int        `json:"col"`
	Kind    *string     `json:"kind"`
	IsUpper *bool       `json:"isUpper"`
	Mult    viprRawJSON `json:"mult"`
}

type certOrbitWitDoc struct {
	Companion      *int        `json:"companion"`
	ColPerm        viprRawJSON `json:"colPerm"`
	RowPerm        viprRawJSON `json:"rowPerm"`
	SupportEntries viprRawJSON `json:"supportEntries"`
}

type certStepDoc struct {
	Rule        string             `json:"rule"`
	Row         *int               `json:"row"`
	Col         *int               `json:"col"`
	Sense       *string            `json:"sense"`
	IsUpper     *bool              `json:"isUpper"`
	Bound       viprRawJSON        `json:"bound"`
	Refs        *[]certLinRefDoc   `json:"refs"`
	Bounds      *[]certBoundRefDoc `json:"bounds"`
	BoundAsms   viprRawJSON        `json:"boundAsms"`
	Ref         *int               `json:"ref"`
	SplitVal    viprRawJSON        `json:"splitVal"`
	LeftAsm     *int               `json:"leftAsm"`
	RightAsm    *int               `json:"rightAsm"`
	Left        *int               `json:"left"`
	Right       *int               `json:"right"`
	OrbitAsms   viprRawJSON        `json:"orbitAsms"`
	Witnesses   *[]certOrbitWitDoc `json:"witnesses"`
	ExtraAsms   viprRawJSON        `json:"extraAsms"`
	LBRef       viprRawJSON        `json:"lbRef"`
	UBRef       viprRawJSON        `json:"ubRef"`
	Con         viprRawJSON        `json:"con"`
	Assumptions viprRawJSON        `json:"assumptions"`
}

type certVIPRDoc struct {
	Orig      viprRawJSON   `json:"orig"`
	Presolve  viprRawJSON   `json:"presolve"`
	N         *int          `json:"n"`
	ObjConst  viprRawJSON   `json:"objConst"`
	Obj       viprRawJSON   `json:"obj"`
	IsInt     []bool        `json:"isInt"`
	LB        []viprRawJSON `json:"lb"`
	UB        []viprRawJSON `json:"ub"`
	BaseRows  []viprRawJSON `json:"baseRows"`
	Mode      string        `json:"mode"`
	TargetLB  viprRawJSON   `json:"targetLB"`
	Incumbent []viprRawJSON `json:"incumbent"`
	Steps     []certStepDoc `json:"steps"`
	RootStep  *int          `json:"rootStep"`
}

type certRatObjJSON struct {
	Num viprRawJSON `json:"num"`
	Den viprRawJSON `json:"den"`
}

type certSparseTermJSON struct {
	Col   *int        `json:"col"`
	Coeff viprRawJSON `json:"coeff"`
}

type certSparseVecJSON struct {
	Terms []certSparseTermJSON `json:"terms"`
}

type certLinConJSON struct {
	Coeffs viprRawJSON `json:"coeffs"`
	Sense  string      `json:"sense"`
	RHS    viprRawJSON `json:"rhs"`
}

func isStrictVIPRDecimalInt(s string, allowNeg bool) bool {
	if len(s) == 0 || len(s) > maxVIPRRatStrLen {
		return false
	}
	i := 0
	if s[0] == '-' {
		if !allowNeg || len(s) == 1 {
			return false
		}
		i = 1
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseVIPRBigInt(raw viprRawJSON) (*big.Int, bool) {
	t, okT := trimVIPRJSON(raw)
	if !okT {
		return nil, false
	}
	s := string(t)
	if s == "" || s == "null" {
		return nil, false
	}
	if strings.HasPrefix(s, "\"") {
		unq, okU := viprUnmarshalString(t)
		if !okU {
			return nil, false
		}
		s = unq
	}
	if !isStrictVIPRDecimalInt(s, true) {
		return nil, false
	}
	z, ok := new(big.Int).SetString(s, 10)
	if !ok || z.BitLen() > maxVIPRRatBits {
		return nil, false
	}
	return z, true
}

func parseVIPRRatValue(raw viprRawJSON) (*big.Rat, bool) {
	t, okT := trimVIPRJSON(raw)
	if !okT {
		return nil, false
	}
	s := string(t)
	if s == "" || s == "null" {
		return nil, false
	}
	switch s[0] {
	case '{':
		obj, okO := unmarshalCertRatObjJSON(t)
		if !okO {
			return nil, false
		}
		num, okN := parseVIPRBigInt(obj.Num)
		den, okD := parseVIPRBigInt(obj.Den)
		if !okN || !okD || den.Sign() <= 0 {
			return nil, false
		}
		return new(big.Rat).SetFrac(num, den), true
	case '"':
		str, okS := viprUnmarshalString(t)
		if !okS || len(str) > maxVIPRRatStrLen {
			return nil, false
		}
		if parts := strings.Split(str, "/"); len(parts) == 1 {
			if !isStrictVIPRDecimalInt(parts[0], true) {
				return nil, false
			}
			num, ok := new(big.Int).SetString(parts[0], 10)
			if !ok || num.BitLen() > maxVIPRRatBits {
				return nil, false
			}
			return new(big.Rat).SetInt(num), true
		} else if len(parts) == 2 {
			if !isStrictVIPRDecimalInt(parts[0], true) || !isStrictVIPRDecimalInt(parts[1], false) {
				return nil, false
			}
			num, okN := new(big.Int).SetString(parts[0], 10)
			den, okD := new(big.Int).SetString(parts[1], 10)
			if !okN || !okD || den.Sign() <= 0 || num.BitLen() > maxVIPRRatBits || den.BitLen() > maxVIPRRatBits {
				return nil, false
			}
			return new(big.Rat).SetFrac(num, den), true
		}
		return nil, false
	default:
		if len(s) > 4096 {
			return nil, false
		}
		r, ok := new(big.Rat).SetString(s)
		if !ok || r.Num().BitLen() > maxVIPRRatBits || r.Denom().BitLen() > maxVIPRRatBits {
			return nil, false
		}
		return r, true
	}
}

func parseVIPRSense(s string) (ConstraintSense, bool) {
	switch s {
	case "<=", "le":
		return LessEqual, true
	case "=", "==", "eq":
		return Equal, true
	case ">=", "ge":
		return GreaterEqual, true
	default:
		return 0, false
	}
}

func parseVIPRNatSlice(raw viprRawJSON, allowMissing bool) ([]int, bool) {
	if !certHasNonNullJSON(raw) {
		return nil, allowMissing
	}
	items, isNull, okA := viprUnmarshalRawArray(raw, false)
	if !okA || isNull {
		return nil, false
	}
	out := make([]int, len(items))
	for i, it := range items {
		v, ok := parseVIPRReqNat(it)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func parseVIPROptNat(raw viprRawJSON) (*int, bool) {
	if !certHasNonNullJSON(raw) {
		return nil, true
	}
	v, ok := parseVIPRReqNat(raw)
	if !ok {
		return nil, false
	}
	return &v, true
}

func canonicalCertTerms(raw []certRatTerm) []certRatTerm {
	if len(raw) == 0 {
		return nil
	}
	slices.SortFunc(raw, func(a, b certRatTerm) int {
		return a.col - b.col
	})
	out := make([]certRatTerm, 0, len(raw))
	for _, t := range raw {
		if t.val == nil || t.val.Sign() == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1].col == t.col {
			out[len(out)-1].val.Add(out[len(out)-1].val, t.val)
			if out[len(out)-1].val.Sign() == 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, certRatTerm{col: t.col, val: new(big.Rat).Set(t.val)})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeVIPRVec(n int, raw viprRawJSON) (dense []*big.Rat, terms []certRatTerm, ok bool) {
	if n < 0 || n > maxVIPRVerifyVars {
		return nil, nil, false
	}
	t, okT := trimVIPRJSON(raw)
	if !okT || len(t) == 0 {
		return nil, nil, false
	}
	if t[0] == '[' {
		elems, isNull, okA := viprUnmarshalRawArray(t, false)
		if !okA || isNull || len(elems) != n {
			return nil, nil, false
		}
		dense = make([]*big.Rat, n)
		for j, el := range elems {
			r, okR := parseVIPRRatValue(el)
			if !okR {
				return nil, nil, false
			}
			dense[j] = r
			if r.Sign() != 0 {
				terms = append(terms, certRatTerm{col: j, val: new(big.Rat).Set(r)})
			}
		}
		return dense, terms, true
	}
	if t[0] == '{' {
		sp, okS := unmarshalCertSparseVecJSON(t)
		if !okS {
			return nil, nil, false
		}
		dense = make([]*big.Rat, n)
		for j := range n {
			dense[j] = new(big.Rat)
		}
		for _, tm := range sp.Terms {
			if tm.Col == nil || *tm.Col < 0 || *tm.Col >= n {
				return nil, nil, false
			}
			r, okR := parseVIPRRatValue(tm.Coeff)
			if !okR {
				return nil, nil, false
			}
			dense[*tm.Col].Add(dense[*tm.Col], r)
		}
		for j, r := range dense {
			if r.Sign() != 0 {
				terms = append(terms, certRatTerm{col: j, val: new(big.Rat).Set(r)})
			}
		}
		return dense, terms, true
	}
	return nil, nil, false
}

func decodeVIPRLinCon(n int, raw viprRawJSON) (certLinCon, bool) {
	lc, okL := unmarshalCertLinConJSON(raw)
	if !okL {
		return certLinCon{}, false
	}
	sense, okS := parseVIPRSense(lc.Sense)
	if !okS {
		return certLinCon{}, false
	}
	rhs, okR := parseVIPRRatValue(lc.RHS)
	if !okR {
		return certLinCon{}, false
	}
	coeffsRaw := lc.Coeffs
	if len(strings.TrimSpace(string(coeffsRaw))) == 0 {
		coeffsRaw = raw
	}
	_, terms, okV := decodeVIPRVec(n, coeffsRaw)
	if !okV {
		return certLinCon{}, false
	}
	return certLinCon{terms: terms, sense: sense, rhs: rhs}, true
}

func buildCertVIPRInstance(p *CompiledProblem) (*certVIPRInstance, bool) {
	if p == nil || p.invalidErr != nil {
		return nil, false
	}
	n := p.numVars
	m := p.numConstraints
	objConst, okC := exactRatFromFloat(p.objConstMin())
	if !okC {
		return nil, false
	}
	inst := &certVIPRInstance{
		n:        n,
		objConst: objConst,
		objDense: make([]*big.Rat, n),
		isInt:    make([]bool, n),
		lb:       make([]*big.Rat, n),
		ub:       make([]*big.Rat, n),
		baseRows: make([]certLinCon, m),
	}
	objMinVec, okVec := exactMinObjVecRat(p)
	if !okVec {
		return nil, false
	}
	for j := range n {
		cj := objMinVec[j]
		inst.objDense[j] = cj
		if cj.Sign() != 0 {
			inst.objTerms = append(inst.objTerms, certRatTerm{col: j, val: new(big.Rat).Set(cj)})
		}
		inst.isInt[j] = p.varCat[j] != Continuous
		if !math.IsInf(p.varLB[j], -1) {
			lr, okL := exactRatFromFloat(p.varLB[j])
			if !okL {
				return nil, false
			}
			inst.lb[j] = lr
		}
		if !math.IsInf(p.varUB[j], 1) {
			ur, okU := exactRatFromFloat(p.varUB[j])
			if !okU {
				return nil, false
			}
			inst.ub[j] = ur
		}
	}
	for i := range m {
		rhs, okR := exactRowRHSRat(p, i)
		if !okR {
			return nil, false
		}
		cols, vals, okC := exactCanonicalRowTermsRat(p, i)
		if !okC {
			return nil, false
		}
		rawTerms := make([]certRatTerm, len(cols))
		for k, col := range cols {
			rawTerms[k] = certRatTerm{col: col, val: vals[k]}
		}
		inst.baseRows[i] = certLinCon{
			terms: canonicalCertTerms(rawTerms),
			sense: p.rowSense[i],
			rhs:   rhs,
		}
	}
	return inst, true
}

func certTermsEqual(a, b []certRatTerm) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].col != b[i].col || a[i].val.Cmp(b[i].val) != 0 {
			return false
		}
	}
	return true
}

func certConEqual(a, b certLinCon) bool {
	return a.sense == b.sense && a.rhs.Cmp(b.rhs) == 0 && certTermsEqual(a.terms, b.terms)
}

func certOptRatEqual(a, b *big.Rat) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(b) == 0
}

func isRatInt64(r *big.Rat, v int64) bool {
	return r != nil && r.IsInt() && r.Num().IsInt64() && r.Num().Int64() == v
}

func certIsUnitTerms(terms []certRatTerm, col int) bool {
	return len(terms) == 1 && terms[0].col == col && isRatInt64(terms[0].val, 1)
}

func certMatchesUnitBound(con certLinCon, col int, wantUpper bool) bool {
	var okSense bool
	if wantUpper {
		okSense = con.sense == LessEqual || con.sense == Equal
	} else {
		okSense = con.sense == GreaterEqual || con.sense == Equal
	}
	return okSense && certIsUnitTerms(con.terms, col)
}

func certIsUnsatCon(con certLinCon) bool {
	if len(con.terms) != 0 {
		return false
	}
	switch con.sense {
	case LessEqual:
		return con.rhs.Sign() < 0
	case GreaterEqual:
		return con.rhs.Sign() > 0
	case Equal:
		return con.rhs.Sign() != 0
	default:
		return false
	}
}

func certLegalMult(target, src ConstraintSense, m *big.Rat) bool {
	switch target {
	case LessEqual:
		switch src {
		case LessEqual:
			return m.Sign() >= 0
		case GreaterEqual:
			return m.Sign() <= 0
		case Equal:
			return true
		}
	case GreaterEqual:
		switch src {
		case LessEqual:
			return m.Sign() <= 0
		case GreaterEqual:
			return m.Sign() >= 0
		case Equal:
			return true
		}
	case Equal:
		return src == Equal
	}
	return false
}

func certMergeAsms(a, b []int) []int {
	if len(b) == 0 {
		return slices.Clone(a)
	}
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func certFilterAsm(asms []int, drop int) []int {
	out := make([]int, 0, len(asms))
	for _, a := range asms {
		if a != drop {
			out = append(out, a)
		}
	}
	return out
}

func certAssumptionsMatch(actual, claimed []int) bool {
	for _, a := range actual {
		if !slices.Contains(claimed, a) {
			return false
		}
	}
	for _, c := range claimed {
		if !slices.Contains(actual, c) {
			return false
		}
	}
	return true
}

func certDotTerms(terms []certRatTerm, x []*big.Rat) *big.Rat {
	sum := new(big.Rat)
	var prod big.Rat
	for _, t := range terms {
		if x[t.col].Sign() != 0 {
			prod.Mul(t.val, x[t.col])
			sum.Add(sum, &prod)
		}
	}
	return sum
}

func certCheckIncumbentFeasible(inst *certVIPRInstance, x []*big.Rat) bool {
	if len(x) != inst.n {
		return false
	}
	for j := range inst.n {
		xj := x[j]
		if xj == nil {
			return false
		}
		if inst.lb[j] != nil && xj.Cmp(inst.lb[j]) < 0 {
			return false
		}
		if inst.ub[j] != nil && xj.Cmp(inst.ub[j]) > 0 {
			return false
		}
		if inst.isInt[j] && !xj.IsInt() {
			return false
		}
	}
	for _, r := range inst.baseRows {
		lhs := certDotTerms(r.terms, x)
		cmp := lhs.Cmp(r.rhs)
		switch r.sense {
		case LessEqual:
			if cmp > 0 {
				return false
			}
		case GreaterEqual:
			if cmp < 0 {
				return false
			}
		case Equal:
			if cmp != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func certEvalLinComb(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	sense ConstraintSense,
	refs []certLinRefDoc,
	bounds []certBoundRefDoc,
) (certDerivedEntry, bool) {
	var accTerms []certRatTerm
	rhs := new(big.Rat)
	var asms []int
	var prod big.Rat

	for _, rf := range refs {
		if rf.ID == nil || *rf.ID < 0 {
			return certDerivedEntry{}, false
		}
		mult, okM := parseVIPRRatValue(rf.Mult)
		if !okM {
			return certDerivedEntry{}, false
		}
		if mult.Sign() == 0 {
			continue
		}
		id := *rf.ID
		if id >= len(entries) {
			return certDerivedEntry{}, false
		}
		ent := entries[id]
		if !certLegalMult(sense, ent.con.sense, mult) {
			return certDerivedEntry{}, false
		}
		for _, t := range ent.con.terms {
			accTerms = append(accTerms, certRatTerm{
				col: t.col,
				val: new(big.Rat).Mul(mult, t.val),
			})
		}
		prod.Mul(mult, ent.con.rhs)
		rhs.Add(rhs, &prod)
		asms = certMergeAsms(asms, ent.assumptions)
	}

	for _, br := range bounds {
		if br.Col == nil || *br.Col < 0 {
			return certDerivedEntry{}, false
		}
		mult, okM := parseVIPRRatValue(br.Mult)
		if !okM {
			return certDerivedEntry{}, false
		}
		var isUpper bool
		if br.Kind != nil {
			switch *br.Kind {
			case "ub", "<=":
				isUpper = true
			case "lb", ">=":
				isUpper = false
			default:
				return certDerivedEntry{}, false
			}
		} else if br.IsUpper != nil {
			isUpper = *br.IsUpper
		} else {
			return certDerivedEntry{}, false
		}
		if mult.Sign() == 0 {
			continue
		}
		col := *br.Col
		if col >= inst.n {
			return certDerivedEntry{}, false
		}
		if isUpper {
			if inst.ub[col] == nil || !certLegalMult(sense, LessEqual, mult) {
				return certDerivedEntry{}, false
			}
			accTerms = append(accTerms, certRatTerm{col: col, val: new(big.Rat).Set(mult)})
			prod.Mul(mult, inst.ub[col])
			rhs.Add(rhs, &prod)
		} else {
			if inst.lb[col] == nil || !certLegalMult(sense, GreaterEqual, mult) {
				return certDerivedEntry{}, false
			}
			accTerms = append(accTerms, certRatTerm{col: col, val: new(big.Rat).Set(mult)})
			prod.Mul(mult, inst.lb[col])
			rhs.Add(rhs, &prod)
		}
	}

	return certDerivedEntry{
		con: certLinCon{
			terms: canonicalCertTerms(accTerms),
			sense: sense,
			rhs:   rhs,
		},
		assumptions: asms,
	}, true
}

func certIsIntegralLinVec(inst *certVIPRInstance, terms []certRatTerm) bool {
	for _, t := range terms {
		if !inst.isInt[t.col] || !t.val.IsInt() {
			return false
		}
	}
	return true
}

func certMergeBranchCon(cL, cR certLinCon) (certLinCon, bool) {
	uL := certIsUnsatCon(cL)
	uR := certIsUnsatCon(cR)
	switch {
	case uL && uR:
		return certLinCon{terms: nil, sense: LessEqual, rhs: big.NewRat(-1, 1)}, true
	case uL:
		return cR, true
	case uR:
		return cL, true
	}
	if cL.sense != cR.sense || !certTermsEqual(cL.terms, cR.terms) {
		return certLinCon{}, false
	}
	switch cL.sense {
	case LessEqual:
		r := new(big.Rat).Set(cL.rhs)
		if cR.rhs.Cmp(r) > 0 {
			r.Set(cR.rhs)
		}
		return certLinCon{terms: cL.terms, sense: LessEqual, rhs: r}, true
	case GreaterEqual:
		r := new(big.Rat).Set(cL.rhs)
		if cR.rhs.Cmp(r) < 0 {
			r.Set(cR.rhs)
		}
		return certLinCon{terms: cL.terms, sense: GreaterEqual, rhs: r}, true
	case Equal:
		if cL.rhs.Cmp(cR.rhs) != 0 {
			return certLinCon{}, false
		}
		return cL, true
	default:
		return certLinCon{}, false
	}
}

func certFindTightestBound(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	boundAsms []int,
	col int,
	wantUpper bool,
) (*big.Rat, []int, bool) {
	var best *big.Rat
	var bestAsms []int
	if wantUpper {
		if inst.ub[col] != nil {
			best = new(big.Rat).Set(inst.ub[col])
		}
	} else {
		if inst.lb[col] != nil {
			best = new(big.Rat).Set(inst.lb[col])
		}
	}
	for _, id := range boundAsms {
		if id < 0 || id >= len(entries) {
			continue
		}
		ent := entries[id]
		if !certMatchesUnitBound(ent.con, col, wantUpper) {
			continue
		}
		cand := ent.con.rhs
		if best == nil {
			best = new(big.Rat).Set(cand)
			bestAsms = ent.assumptions
		} else if wantUpper && cand.Cmp(best) < 0 {
			best = new(big.Rat).Set(cand)
			bestAsms = ent.assumptions
		} else if !wantUpper && cand.Cmp(best) > 0 {
			best = new(big.Rat).Set(cand)
			bestAsms = ent.assumptions
		}
	}
	if best == nil {
		return nil, nil, false
	}
	return best, bestAsms, true
}

func certEvalSafeDual(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	refs []certLinRefDoc,
	boundAsms []int,
) (certDerivedEntry, bool) {
	rowEnt, ok := certEvalLinComb(inst, entries, GreaterEqual, refs, nil)
	if !ok {
		return certDerivedEntry{}, false
	}
	rowDense := make([]*big.Rat, inst.n)
	for _, t := range rowEnt.con.terms {
		rowDense[t.col] = t.val
	}
	accRHS := new(big.Rat).Set(rowEnt.con.rhs)
	accAsms := slices.Clone(rowEnt.assumptions)
	var rj, prod big.Rat
	for j := range inst.n {
		if rowDense[j] != nil {
			rj.Sub(inst.objDense[j], rowDense[j])
		} else {
			rj.Set(inst.objDense[j])
		}
		sgn := rj.Sign()
		if sgn == 0 {
			continue
		}
		if sgn > 0 {
			l, asmsL, okL := certFindTightestBound(inst, entries, boundAsms, j, false)
			if !okL {
				return certDerivedEntry{}, false
			}
			prod.Mul(&rj, l)
			accRHS.Add(accRHS, &prod)
			accAsms = certMergeAsms(accAsms, asmsL)
		} else {
			u, asmsU, okU := certFindTightestBound(inst, entries, boundAsms, j, true)
			if !okU {
				return certDerivedEntry{}, false
			}
			prod.Mul(&rj, u)
			accRHS.Add(accRHS, &prod)
			accAsms = certMergeAsms(accAsms, asmsU)
		}
	}
	return certDerivedEntry{
		con: certLinCon{
			terms: inst.objTerms,
			sense: GreaterEqual,
			rhs:   accRHS,
		},
		assumptions: accAsms,
	}, true
}

func certGetBoundRef(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	col int,
	ref *int,
	wantUpper bool,
) (*big.Rat, []int, bool) {
	if ref == nil {
		if wantUpper {
			if inst.ub[col] == nil {
				return nil, nil, false
			}
			return inst.ub[col], nil, true
		}
		if inst.lb[col] == nil {
			return nil, nil, false
		}
		return inst.lb[col], nil, true
	}
	id := *ref
	if id < 0 || id >= len(entries) {
		return nil, nil, false
	}
	ent := entries[id]
	if !certMatchesUnitBound(ent.con, col, wantUpper) {
		return nil, nil, false
	}
	return ent.con.rhs, ent.assumptions, true
}

func certTermAt(n int, terms []certRatTerm, col int) *big.Rat {
	if col < 0 || col >= n {
		return new(big.Rat)
	}
	idx, found := slices.BinarySearchFunc(terms, col, func(t certRatTerm, target int) int {
		return t.col - target
	})
	if !found {
		return new(big.Rat)
	}
	return terms[idx].val
}

func certCoeffsPermMatch(n int, dst, src []certRatTerm, perm []int, negate bool) bool {
	if len(perm) != n {
		return false
	}
	var negVal big.Rat
	for j := range n {
		dj := certTermAt(n, dst, j)
		sj := certTermAt(n, src, perm[j])
		if negate {
			negVal.Neg(sj)
			if dj.Cmp(&negVal) != 0 {
				return false
			}
		} else if dj.Cmp(sj) != 0 {
			return false
		}
	}
	return true
}

func certConPermImplies(n int, src, dst certLinCon, perm []int) bool {
	if certCoeffsPermMatch(n, dst.terms, src.terms, perm, false) {
		switch dst.sense {
		case LessEqual:
			if (src.sense == LessEqual || src.sense == Equal) && src.rhs.Cmp(dst.rhs) <= 0 {
				return true
			}
		case GreaterEqual:
			if (src.sense == GreaterEqual || src.sense == Equal) && dst.rhs.Cmp(src.rhs) <= 0 {
				return true
			}
		case Equal:
			if src.sense == Equal && src.rhs.Cmp(dst.rhs) == 0 {
				return true
			}
		}
	}
	if !certCoeffsPermMatch(n, dst.terms, src.terms, perm, true) {
		return false
	}
	negSrcRHS := new(big.Rat).Neg(src.rhs)
	switch dst.sense {
	case LessEqual:
		return (src.sense == GreaterEqual || src.sense == Equal) && negSrcRHS.Cmp(dst.rhs) <= 0
	case GreaterEqual:
		return (src.sense == LessEqual || src.sense == Equal) && dst.rhs.Cmp(negSrcRHS) <= 0
	case Equal:
		return src.sense == Equal && dst.rhs.Cmp(negSrcRHS) == 0
	default:
		return false
	}
}

func certBaseBoundImpliesPerm(inst *certVIPRInstance, dst certLinCon, perm []int) bool {
	if len(dst.terms) != 1 || !isRatInt64(dst.terms[0].val, 1) {
		return false
	}
	k := dst.terms[0].col
	if k < 0 || k >= len(perm) {
		return false
	}
	pk := perm[k]
	if pk < 0 || pk >= inst.n {
		return false
	}
	switch dst.sense {
	case LessEqual:
		return inst.ub[pk] != nil && inst.ub[pk].Cmp(dst.rhs) <= 0
	case GreaterEqual:
		return inst.lb[pk] != nil && dst.rhs.Cmp(inst.lb[pk]) <= 0
	case Equal:
		return inst.lb[pk] != nil && inst.ub[pk] != nil &&
			dst.rhs.Cmp(inst.lb[pk]) <= 0 && inst.ub[pk].Cmp(dst.rhs) <= 0
	default:
		return false
	}
}

func certCheckOrbitAutomorphism(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	outAsms, rightRem []int,
	br, comp int,
	rightCon certLinCon,
	colPerm, rowPerm, supportEntries []int,
) bool {
	if !viprIsBijection(inst.n, colPerm) {
		return false
	}
	if br < 0 || br >= len(colPerm) || colPerm[br] != comp {
		return false
	}
	if len(rowPerm) != len(inst.baseRows) {
		return false
	}
	for i := range inst.n {
		pi := colPerm[i]
		if inst.isInt[i] && !inst.isInt[pi] {
			return false
		}
		if inst.objDense[i].Cmp(inst.objDense[pi]) != 0 {
			return false
		}
		if inst.lb[i] != nil && (inst.lb[pi] == nil || inst.lb[i].Cmp(inst.lb[pi]) > 0) {
			return false
		}
		if inst.ub[i] != nil && (inst.ub[pi] == nil || inst.ub[pi].Cmp(inst.ub[i]) > 0) {
			return false
		}
	}
	for i := range inst.baseRows {
		rIdx := rowPerm[i]
		if rIdx < 0 || rIdx >= len(inst.baseRows) {
			return false
		}
		if !certConPermImplies(inst.n, inst.baseRows[rIdx], inst.baseRows[i], colPerm) {
			return false
		}
	}
	if !certIsUnsatCon(rightCon) && !certCoeffsPermMatch(inst.n, rightCon.terms, rightCon.terms, colPerm, false) {
		return false
	}
	for _, b := range supportEntries {
		if b < 0 || b >= len(entries) {
			return false
		}
		for _, d := range entries[b].assumptions {
			if !slices.Contains(outAsms, d) {
				return false
			}
		}
	}
	for _, a := range rightRem {
		if a < 0 || a >= len(entries) {
			return false
		}
		conA := entries[a].con
		if certBaseBoundImpliesPerm(inst, conA, colPerm) || certConPermImplies(inst.n, conA, conA, colPerm) {
			continue
		}
		matched := false
		for _, b := range outAsms {
			if b >= 0 && b < len(entries) && certConPermImplies(inst.n, entries[b].con, conA, colPerm) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		for _, b := range supportEntries {
			if b >= 0 && b < len(entries) && certConPermImplies(inst.n, entries[b].con, conA, colPerm) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func certEvalRoundStep(inst *certVIPRInstance, entries []certDerivedEntry, ref *int, isDown bool) (certDerivedEntry, bool) {
	if ref == nil || *ref < 0 || *ref >= len(entries) {
		return certDerivedEntry{}, false
	}
	ent := entries[*ref]
	wantSense := GreaterEqual
	if isDown {
		wantSense = LessEqual
	}
	if (ent.con.sense != wantSense && ent.con.sense != Equal) || !certIsIntegralLinVec(inst, ent.con.terms) {
		return certDerivedEntry{}, false
	}
	var rhs *big.Rat
	if isDown {
		rhs = viprRatFloor(ent.con.rhs)
	} else {
		rhs = viprRatCeil(ent.con.rhs)
	}
	return certDerivedEntry{
		con: certLinCon{
			terms: ent.con.terms,
			sense: wantSense,
			rhs:   rhs,
		},
		assumptions: slices.Clone(ent.assumptions),
	}, true
}

func certCheckBranchPair(
	inst *certVIPRInstance,
	entries []certDerivedEntry,
	step certStepDoc,
	splitInt *big.Int,
) (col, la, ra, lIdx, rIdx int, cOut certLinCon, ok bool) {
	if step.Col == nil || step.LeftAsm == nil || step.RightAsm == nil || step.Left == nil || step.Right == nil {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	col = *step.Col
	la, ra, lIdx, rIdx = *step.LeftAsm, *step.RightAsm, *step.Left, *step.Right
	if col < 0 || col >= inst.n || !inst.isInt[col] {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	if la < 0 || la >= len(entries) || ra < 0 || ra >= len(entries) || lIdx < 0 || lIdx >= len(entries) || rIdx < 0 || rIdx >= len(entries) {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	wantLA := certLinCon{
		terms: []certRatTerm{{col: col, val: big.NewRat(1, 1)}},
		sense: LessEqual,
		rhs:   new(big.Rat).SetInt(splitInt),
	}
	splitPlus := new(big.Int).Add(splitInt, big.NewInt(1))
	wantRA := certLinCon{
		terms: []certRatTerm{{col: col, val: big.NewRat(1, 1)}},
		sense: GreaterEqual,
		rhs:   new(big.Rat).SetInt(splitPlus),
	}
	if !certConEqual(entries[la].con, wantLA) || !certConEqual(entries[ra].con, wantRA) {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	if len(entries[la].assumptions) != 1 || entries[la].assumptions[0] != la {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	if len(entries[ra].assumptions) != 1 || entries[ra].assumptions[0] != ra {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	cOut, okM := certMergeBranchCon(entries[lIdx].con, entries[rIdx].con)
	if !okM {
		return 0, 0, 0, 0, 0, certLinCon{}, false
	}
	return col, la, ra, lIdx, rIdx, cOut, true
}

func certEvalProofStep(inst *certVIPRInstance, entries []certDerivedEntry, step certStepDoc) (certDerivedEntry, bool) {
	var derived certDerivedEntry
	var ok bool

	switch step.Rule {
	case "conIndex":
		if step.Row == nil || *step.Row < 0 || *step.Row >= len(inst.baseRows) {
			return certDerivedEntry{}, false
		}
		derived = certDerivedEntry{con: inst.baseRows[*step.Row]}
		ok = true

	case "assume":
		if step.Col == nil || *step.Col < 0 || *step.Col >= inst.n || !inst.isInt[*step.Col] {
			return certDerivedEntry{}, false
		}
		bndInt, okB := parseVIPRBigInt(step.Bound)
		if !okB {
			return certDerivedEntry{}, false
		}
		var isUpper bool
		if step.Sense != nil {
			switch *step.Sense {
			case "<=", "le":
				isUpper = true
			case ">=", "ge":
				isUpper = false
			default:
				return certDerivedEntry{}, false
			}
		} else if step.IsUpper != nil {
			isUpper = *step.IsUpper
		} else {
			return certDerivedEntry{}, false
		}
		sense := GreaterEqual
		if isUpper {
			sense = LessEqual
		}
		selfID := len(entries)
		derived = certDerivedEntry{
			con: certLinCon{
				terms: []certRatTerm{{col: *step.Col, val: big.NewRat(1, 1)}},
				sense: sense,
				rhs:   new(big.Rat).SetInt(bndInt),
			},
			assumptions: []int{selfID},
		}
		ok = true

	case "lincomb":
		if step.Sense == nil || step.Refs == nil {
			return certDerivedEntry{}, false
		}
		sense, okS := parseVIPRSense(*step.Sense)
		if !okS {
			return certDerivedEntry{}, false
		}
		var bnds []certBoundRefDoc
		if step.Bounds != nil {
			bnds = *step.Bounds
		}
		derived, ok = certEvalLinComb(inst, entries, sense, *step.Refs, bnds)

	case "roundDown":
		derived, ok = certEvalRoundStep(inst, entries, step.Ref, true)

	case "roundUp":
		derived, ok = certEvalRoundStep(inst, entries, step.Ref, false)

	case "branch":
		splitInt, okS := parseVIPRBigInt(step.SplitVal)
		if !okS {
			return certDerivedEntry{}, false
		}
		_, la, ra, lIdx, rIdx, cOut, okP := certCheckBranchPair(inst, entries, step, splitInt)
		if !okP {
			return certDerivedEntry{}, false
		}
		asms := certMergeAsms(certFilterAsm(entries[lIdx].assumptions, la), certFilterAsm(entries[rIdx].assumptions, ra))
		derived = certDerivedEntry{con: cOut, assumptions: asms}
		ok = true

	case "orbitBranch":
		if step.Witnesses == nil {
			return certDerivedEntry{}, false
		}
		col, la, ra, lIdx, rIdx, cOut, okP := certCheckBranchPair(inst, entries, step, big.NewInt(0))
		if !okP {
			return certDerivedEntry{}, false
		}
		orbitAsms, okOA := parseVIPRNatSlice(step.OrbitAsms, false)
		extraAsms, okEA := parseVIPRNatSlice(step.ExtraAsms, true)
		if !okOA || !okEA || len(orbitAsms) != len(*step.Witnesses) {
			return certDerivedEntry{}, false
		}
		for _, ea := range extraAsms {
			if ea < 0 || ea >= len(entries) {
				return certDerivedEntry{}, false
			}
		}
		var leftRem []int
		for _, a := range entries[lIdx].assumptions {
			if a != la && !slices.Contains(orbitAsms, a) {
				leftRem = append(leftRem, a)
			}
		}
		rightRem := certFilterAsm(entries[rIdx].assumptions, ra)
		outAsms := certMergeAsms(certMergeAsms(leftRem, rightRem), extraAsms)
		for idx, oa := range orbitAsms {
			if oa < 0 || oa >= len(entries) {
				return certDerivedEntry{}, false
			}
			wd := (*step.Witnesses)[idx]
			if wd.Companion == nil || *wd.Companion < 0 || *wd.Companion >= inst.n || !inst.isInt[*wd.Companion] {
				return certDerivedEntry{}, false
			}
			colPerm, okCP := parseVIPRNatSlice(wd.ColPerm, false)
			rowPerm, okRP := parseVIPRNatSlice(wd.RowPerm, false)
			supEnts, okSE := parseVIPRNatSlice(wd.SupportEntries, true)
			if !okCP || !okRP || !okSE {
				return certDerivedEntry{}, false
			}
			wantOA := certLinCon{
				terms: []certRatTerm{{col: *wd.Companion, val: big.NewRat(1, 1)}},
				sense: LessEqual,
				rhs:   new(big.Rat),
			}
			if !certConEqual(entries[oa].con, wantOA) || len(entries[oa].assumptions) != 1 || entries[oa].assumptions[0] != oa {
				return certDerivedEntry{}, false
			}
			if !certCheckOrbitAutomorphism(inst, entries, outAsms, rightRem, col, *wd.Companion, entries[rIdx].con, colPerm, rowPerm, supEnts) {
				return certDerivedEntry{}, false
			}
		}
		derived = certDerivedEntry{con: cOut, assumptions: outAsms}
		ok = true

	case "farkas":
		if step.Refs == nil {
			return certDerivedEntry{}, false
		}
		var bnds []certBoundRefDoc
		if step.Bounds != nil {
			bnds = *step.Bounds
		}
		e, okL := certEvalLinComb(inst, entries, LessEqual, *step.Refs, bnds)
		if !okL || !certIsUnsatCon(e.con) {
			return certDerivedEntry{}, false
		}
		derived = e
		ok = true

	case "boundConflict":
		if step.Col == nil || *step.Col < 0 || *step.Col >= inst.n {
			return certDerivedEntry{}, false
		}
		col := *step.Col
		lbRef, okLR := parseVIPROptNat(step.LBRef)
		ubRef, okUR := parseVIPROptNat(step.UBRef)
		if !okLR || !okUR {
			return certDerivedEntry{}, false
		}
		l, asmsL, okL := certGetBoundRef(inst, entries, col, lbRef, false)
		u, asmsU, okU := certGetBoundRef(inst, entries, col, ubRef, true)
		if !okL || !okU || u.Cmp(l) >= 0 {
			return certDerivedEntry{}, false
		}
		derived = certDerivedEntry{
			con: certLinCon{
				terms: nil,
				sense: LessEqual,
				rhs:   new(big.Rat).Sub(u, l),
			},
			assumptions: certMergeAsms(asmsL, asmsU),
		}
		ok = true

	case "dualBound":
		if step.Refs == nil {
			return certDerivedEntry{}, false
		}
		var bnds []certBoundRefDoc
		if step.Bounds != nil {
			bnds = *step.Bounds
		}
		e, okL := certEvalLinComb(inst, entries, GreaterEqual, *step.Refs, bnds)
		if !okL || !certTermsEqual(e.con.terms, inst.objTerms) {
			return certDerivedEntry{}, false
		}
		derived = e
		ok = true

	case "safeDual":
		if step.Refs == nil {
			return certDerivedEntry{}, false
		}
		var boundAsms []int
		if len(strings.TrimSpace(string(step.BoundAsms))) > 0 {
			var okBA bool
			boundAsms, okBA = parseVIPRNatSlice(step.BoundAsms, false)
			if !okBA {
				return certDerivedEntry{}, false
			}
		}
		derived, ok = certEvalSafeDual(inst, entries, *step.Refs, boundAsms)

	default:
		return certDerivedEntry{}, false
	}

	if !ok {
		return certDerivedEntry{}, false
	}

	if certHasNonNullJSON(step.Con) {
		claimedCon, okC := decodeVIPRLinCon(inst.n, step.Con)
		if !okC || !certConEqual(derived.con, claimedCon) {
			return certDerivedEntry{}, false
		}
	}
	if certHasNonNullJSON(step.Assumptions) {
		claimedAsms, okA := parseVIPRNatSlice(step.Assumptions, false)
		if !okA || !certAssumptionsMatch(derived.assumptions, claimedAsms) {
			return certDerivedEntry{}, false
		}
	}
	return derived, true
}

func decodeMiniVIPRInstanceDoc(doc certVIPRDoc) (*certVIPRInstance, bool) {
	if doc.N == nil {
		return nil, false
	}
	n := *doc.N
	if n < 0 || n > maxVIPRVerifyVars || len(doc.IsInt) != n || len(doc.LB) != n || len(doc.UB) != n {
		return nil, false
	}
	objConst := new(big.Rat)
	if certHasNonNullJSON(doc.ObjConst) {
		r, ok := parseVIPRRatValue(doc.ObjConst)
		if !ok {
			return nil, false
		}
		objConst = r
	}
	objDense, objTerms, okObj := decodeVIPRVec(n, doc.Obj)
	if !okObj {
		return nil, false
	}
	lb := make([]*big.Rat, n)
	ub := make([]*big.Rat, n)
	for j := range n {
		if certHasNonNullJSON(doc.LB[j]) {
			r, ok := parseVIPRRatValue(doc.LB[j])
			if !ok {
				return nil, false
			}
			lb[j] = r
		}
		if certHasNonNullJSON(doc.UB[j]) {
			r, ok := parseVIPRRatValue(doc.UB[j])
			if !ok {
				return nil, false
			}
			ub[j] = r
		}
	}
	baseRows := make([]certLinCon, len(doc.BaseRows))
	for i, rv := range doc.BaseRows {
		rc, ok := decodeVIPRLinCon(n, rv)
		if !ok {
			return nil, false
		}
		baseRows[i] = rc
	}
	return &certVIPRInstance{
		n:        n,
		objConst: objConst,
		objDense: objDense,
		objTerms: objTerms,
		isInt:    slices.Clone(doc.IsInt),
		lb:       lb,
		ub:       ub,
		baseRows: baseRows,
	}, true
}

func verifyInstancesMatch(a, b *certVIPRInstance) bool {
	if a.n != b.n || a.objConst.Cmp(b.objConst) != 0 || len(a.baseRows) != len(b.baseRows) {
		return false
	}
	if !certTermsEqual(a.objTerms, b.objTerms) || !slices.Equal(a.isInt, b.isInt) {
		return false
	}
	for j := range a.n {
		if !certOptRatEqual(a.lb[j], b.lb[j]) || !certOptRatEqual(a.ub[j], b.ub[j]) {
			return false
		}
	}
	for i := range a.baseRows {
		if !certConEqual(a.baseRows[i], b.baseRows[i]) {
			return false
		}
	}
	return true
}

func certHasNonNullJSON(v viprRawJSON) bool {
	s := strings.TrimSpace(string(v))
	return s != "" && s != "null"
}

type certPresolveStepDoc struct {
	Op       string      `json:"op"`
	Row      viprRawJSON `json:"row"`
	Col      viprRawJSON `json:"col"`
	U        viprRawJSON `json:"u"`
	V        viprRawJSON `json:"v"`
	IsUpper  viprRawJSON `json:"isUpper"`
	RoundInt viprRawJSON `json:"roundInt"`
	Divisor  viprRawJSON `json:"divisor"`
	NewBound viprRawJSON `json:"newBound"`
	Val      viprRawJSON `json:"val"`
	Delta    viprRawJSON `json:"delta"`
	ColPerm  viprRawJSON `json:"colPerm"`
	RowPerm  viprRawJSON `json:"rowPerm"`
}

type certPresolveStep struct {
	op       string
	row      int
	col      int
	u        int
	v        int
	isUpper  bool
	roundInt bool
	divisor  *big.Int
	ratVal   *big.Rat
	colPerm  []int
	rowPerm  []int
}

type certPresolveState struct {
	n        int
	objConst *big.Rat
	obj      []*big.Rat
	isInt    []bool
	lb       []*big.Rat
	ub       []*big.Rat
	rows     []*certLinCon
}

func parseVIPRReqNat(raw viprRawJSON) (int, bool) {
	z, ok := parseVIPRBigInt(raw)
	if !ok || z.Sign() < 0 || !z.IsInt64() {
		return 0, false
	}
	v := z.Int64()
	if v > int64(math.MaxInt32) {
		return 0, false
	}
	return int(v), true
}

func parseVIPRReqBool(raw viprRawJSON) (bool, bool) {
	if !certHasNonNullJSON(raw) {
		return false, false
	}
	return viprUnmarshalBool(raw)
}

func parseVIPROptBool(raw viprRawJSON, def bool) (bool, bool) {
	if !certHasNonNullJSON(raw) {
		return def, true
	}
	return viprUnmarshalBool(raw)
}

func decodeVIPRInstanceRaw(raw viprRawJSON) (*certVIPRInstance, bool) {
	if !certHasNonNullJSON(raw) {
		return nil, false
	}
	instDoc, okD := unmarshalCertVIPRDoc(raw)
	if !okD {
		return nil, false
	}
	return decodeMiniVIPRInstanceDoc(instDoc)
}

func decodeVIPRPresolveStep(raw viprRawJSON) (certPresolveStep, bool) {
	sd, okS := unmarshalCertPresolveStepDoc(raw)
	if !okS {
		return certPresolveStep{}, false
	}
	switch sd.Op {
	case "roundBound":
		col, okC := parseVIPRReqNat(sd.Col)
		isUp, okU := parseVIPRReqBool(sd.IsUpper)
		nb, okB := parseVIPRRatValue(sd.NewBound)
		if !okC || !okU || !okB {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, col: col, isUpper: isUp, ratVal: nb}, true

	case "activityBound":
		row, okR := parseVIPRReqNat(sd.Row)
		col, okC := parseVIPRReqNat(sd.Col)
		isUp, okU := parseVIPRReqBool(sd.IsUpper)
		rnd, okI := parseVIPROptBool(sd.RoundInt, false)
		nb, okB := parseVIPRRatValue(sd.NewBound)
		if !okR || !okC || !okU || !okI || !okB {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row, col: col, isUpper: isUp, roundInt: rnd, ratVal: nb}, true

	case "gcdRowDiv":
		row, okR := parseVIPRReqNat(sd.Row)
		div, okD := parseVIPRBigInt(sd.Divisor)
		if !okR || !okD {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row, divisor: div}, true

	case "substFixedRow":
		row, okR := parseVIPRReqNat(sd.Row)
		col, okC := parseVIPRReqNat(sd.Col)
		val, okV := parseVIPRRatValue(sd.Val)
		if !okR || !okC || !okV {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row, col: col, ratVal: val}, true

	case "substFixedObj":
		col, okC := parseVIPRReqNat(sd.Col)
		val, okV := parseVIPRRatValue(sd.Val)
		if !okC || !okV {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, col: col, ratVal: val}, true

	case "singletonRowElim":
		row, okR := parseVIPRReqNat(sd.Row)
		col, okC := parseVIPRReqNat(sd.Col)
		rnd, okI := parseVIPROptBool(sd.RoundInt, false)
		if !okR || !okC || !okI {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row, col: col, roundInt: rnd}, true

	case "redundantRow":
		row, okR := parseVIPRReqNat(sd.Row)
		if !okR {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row}, true

	case "binStrengthen":
		row, okR := parseVIPRReqNat(sd.Row)
		col, okC := parseVIPRReqNat(sd.Col)
		delta, okD := parseVIPRRatValue(sd.Delta)
		if !okR || !okC || !okD {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, row: row, col: col, ratVal: delta}, true

	case "symBreakLe":
		u, okU := parseVIPRReqNat(sd.U)
		v, okV := parseVIPRReqNat(sd.V)
		cp, okCP := parseVIPRNatSlice(sd.ColPerm, false)
		rp, okRP := parseVIPRNatSlice(sd.RowPerm, false)
		if !okU || !okV || !okCP || !okRP {
			return certPresolveStep{}, false
		}
		return certPresolveStep{op: sd.Op, u: u, v: v, colPerm: cp, rowPerm: rp}, true

	default:
		return certPresolveStep{}, false
	}
}

func decodeVIPRPresolveSteps(raw viprRawJSON) ([]certPresolveStep, bool) {
	if !certHasNonNullJSON(raw) {
		return nil, true
	}
	items, isNull, okA := viprUnmarshalRawArray(raw, false)
	if !okA || isNull {
		return nil, false
	}
	out := make([]certPresolveStep, len(items))
	for i, it := range items {
		ps, ok := decodeVIPRPresolveStep(it)
		if !ok {
			return nil, false
		}
		out[i] = ps
	}
	return out, true
}

func newCertPresolveState(orig *certVIPRInstance) *certPresolveState {
	n := orig.n
	st := &certPresolveState{
		n:        n,
		objConst: new(big.Rat).Set(orig.objConst),
		obj:      make([]*big.Rat, n),
		isInt:    slices.Clone(orig.isInt),
		lb:       make([]*big.Rat, n),
		ub:       make([]*big.Rat, n),
		rows:     make([]*certLinCon, len(orig.baseRows)),
	}
	for j := range n {
		st.obj[j] = new(big.Rat).Set(orig.objDense[j])
		if orig.lb[j] != nil {
			st.lb[j] = new(big.Rat).Set(orig.lb[j])
		}
		if orig.ub[j] != nil {
			st.ub[j] = new(big.Rat).Set(orig.ub[j])
		}
	}
	for i, r := range orig.baseRows {
		rc := certLinCon{
			terms: canonicalCertTerms(r.terms),
			sense: r.sense,
			rhs:   new(big.Rat).Set(r.rhs),
		}
		st.rows[i] = &rc
	}
	return st
}

func (st *certPresolveState) toInstance() *certVIPRInstance {
	inst := &certVIPRInstance{
		n:        st.n,
		objConst: new(big.Rat).Set(st.objConst),
		objDense: make([]*big.Rat, st.n),
		isInt:    slices.Clone(st.isInt),
		lb:       make([]*big.Rat, st.n),
		ub:       make([]*big.Rat, st.n),
		baseRows: make([]certLinCon, 0, len(st.rows)),
	}
	for j := range st.n {
		inst.objDense[j] = new(big.Rat).Set(st.obj[j])
		if st.obj[j].Sign() != 0 {
			inst.objTerms = append(inst.objTerms, certRatTerm{col: j, val: new(big.Rat).Set(st.obj[j])})
		}
		if st.lb[j] != nil {
			inst.lb[j] = new(big.Rat).Set(st.lb[j])
		}
		if st.ub[j] != nil {
			inst.ub[j] = new(big.Rat).Set(st.ub[j])
		}
	}
	for _, r := range st.rows {
		if r == nil {
			continue
		}
		inst.baseRows = append(inst.baseRows, certLinCon{
			terms: canonicalCertTerms(r.terms),
			sense: r.sense,
			rhs:   new(big.Rat).Set(r.rhs),
		})
	}
	return inst
}

func certSumRowAct(st *certPresolveState, terms []certRatTerm, skipCol int, wantMax bool) (*big.Rat, bool) {
	acc := new(big.Rat)
	var prod big.Rat
	for _, t := range terms {
		if t.col == skipCol || t.val.Sign() == 0 {
			continue
		}
		var bnd *big.Rat
		if (t.val.Sign() > 0) == wantMax {
			bnd = st.ub[t.col]
		} else {
			bnd = st.lb[t.col]
		}
		if bnd == nil {
			return nil, false
		}
		prod.Mul(t.val, bnd)
		acc.Add(acc, &prod)
	}
	return acc, true
}

func certTightenLBOpt(old, cand *big.Rat) *big.Rat {
	if old == nil || cand.Cmp(old) > 0 {
		return new(big.Rat).Set(cand)
	}
	return new(big.Rat).Set(old)
}

func certTightenUBOpt(old, cand *big.Rat) *big.Rat {
	if old == nil || cand.Cmp(old) < 0 {
		return new(big.Rat).Set(cand)
	}
	return new(big.Rat).Set(old)
}

func certCheckPermInvolution(n int, perm []int) bool {
	if len(perm) != n {
		return false
	}
	for i, p := range perm {
		if p < 0 || p >= n || perm[p] != i {
			return false
		}
	}
	return true
}

func certCheckSymChainRow(n int, row certLinCon, v int, colPerm []int) bool {
	if len(colPerm) != n || row.rhs.Sign() != 0 || len(row.terms) != 2 {
		return false
	}
	posCol, negCol := -1, -1
	for _, t := range row.terms {
		switch {
		case isRatInt64(t.val, 1):
			posCol = t.col
		case isRatInt64(t.val, -1):
			negCol = t.col
		default:
			return false
		}
	}
	if posCol < 0 || negCol < 0 {
		return false
	}
	switch row.sense {
	case LessEqual:
		return negCol == v && posCol != v && posCol < n && colPerm[posCol] == posCol
	case GreaterEqual:
		return posCol == v && negCol != v && negCol < n && colPerm[negCol] == negCol
	default:
		return false
	}
}

func certPresolveActivityBound(st *certPresolveState, step certPresolveStep) bool {
	if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil || step.col < 0 || step.col >= st.n {
		return false
	}
	con := st.rows[step.row]
	a := certTermAt(st.n, con.terms, step.col)
	if a.Sign() == 0 {
		return false
	}
	needLE := (a.Sign() > 0) == step.isUpper
	if needLE {
		if con.sense != LessEqual && con.sense != Equal {
			return false
		}
	} else {
		if con.sense != GreaterEqual && con.sense != Equal {
			return false
		}
	}
	boundSum, okSum := certSumRowAct(st, con.terms, step.col, !needLE)
	if !okSum {
		return false
	}
	imp := new(big.Rat).Sub(con.rhs, boundSum)
	imp.Quo(imp, a)
	effImp := imp
	if step.roundInt {
		if !st.isInt[step.col] {
			return false
		}
		if step.isUpper {
			effImp = viprRatFloor(imp)
		} else {
			effImp = viprRatCeil(imp)
		}
	}
	if step.isUpper {
		if effImp.Cmp(step.ratVal) > 0 || !viprOptRatUbLe(step.ratVal, st.ub[step.col]) {
			return false
		}
		st.ub[step.col] = new(big.Rat).Set(step.ratVal)
		return true
	}
	if step.ratVal.Cmp(effImp) > 0 || !viprOptRatLbLe(st.lb[step.col], step.ratVal) {
		return false
	}
	st.lb[step.col] = new(big.Rat).Set(step.ratVal)
	return true
}

func certPresolveBinStrengthen(st *certPresolveState, step certPresolveStep) bool {
	if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil || step.col < 0 || step.col >= st.n {
		return false
	}
	if !st.isInt[step.col] || st.lb[step.col] == nil || st.lb[step.col].Sign() != 0 ||
		!isRatInt64(st.ub[step.col], 1) || step.ratVal.Sign() < 0 {
		return false
	}
	con := st.rows[step.row]
	if con.sense != LessEqual {
		return false
	}
	a := certTermAt(st.n, con.terms, step.col)
	if a.Sign() == 0 {
		return false
	}
	maxExcl, ok := certSumRowAct(st, con.terms, step.col, true)
	if !ok {
		return false
	}
	var newA, newRHS *big.Rat
	if a.Sign() > 0 {
		rhsMinusDelta := new(big.Rat).Sub(con.rhs, step.ratVal)
		if maxExcl.Cmp(rhsMinusDelta) > 0 {
			return false
		}
		newA = new(big.Rat).Sub(a, step.ratVal)
		newRHS = rhsMinusDelta
	} else {
		aPlusDelta := new(big.Rat).Add(a, step.ratVal)
		lhsMax := new(big.Rat).Add(maxExcl, aPlusDelta)
		if lhsMax.Cmp(con.rhs) > 0 {
			return false
		}
		newA = aPlusDelta
		newRHS = new(big.Rat).Set(con.rhs)
	}
	newTerms := make([]certRatTerm, 0, len(con.terms))
	for _, t := range con.terms {
		if t.col == step.col {
			if newA.Sign() != 0 {
				newTerms = append(newTerms, certRatTerm{col: step.col, val: newA})
			}
		} else {
			newTerms = append(newTerms, certRatTerm{col: t.col, val: new(big.Rat).Set(t.val)})
		}
	}
	st.rows[step.row] = &certLinCon{
		terms: canonicalCertTerms(newTerms),
		sense: LessEqual,
		rhs:   newRHS,
	}
	return true
}

func certPresolveSymBreakLe(st *certPresolveState, step certPresolveStep) bool {
	n := st.n
	if step.u < 0 || step.u >= n || step.v < 0 || step.v >= n || step.u == step.v {
		return false
	}
	if !certCheckPermInvolution(n, step.colPerm) || step.colPerm[step.u] != step.v {
		return false
	}
	if len(step.rowPerm) != len(st.rows) {
		return false
	}
	if st.obj[step.v].Cmp(st.obj[step.u]) > 0 {
		return false
	}
	for i := range n {
		pi := step.colPerm[i]
		if st.isInt[i] && !st.isInt[pi] {
			return false
		}
		if !viprOptRatLbLe(st.lb[i], st.lb[pi]) || !viprOptRatUbLe(st.ub[pi], st.ub[i]) {
			return false
		}
		if i != step.u && i != step.v && st.obj[i].Cmp(st.obj[pi]) != 0 {
			return false
		}
	}
	for i, rowI := range st.rows {
		if rowI == nil {
			continue
		}
		rIdx := step.rowPerm[i]
		directOK := rIdx >= 0 && rIdx < len(st.rows) && st.rows[rIdx] != nil &&
			certConPermImplies(n, *st.rows[rIdx], *rowI, step.colPerm)
		if !directOK && !certCheckSymChainRow(n, *rowI, step.v, step.colPerm) {
			return false
		}
	}
	newCon := &certLinCon{
		terms: canonicalCertTerms([]certRatTerm{
			{col: step.u, val: big.NewRat(1, 1)},
			{col: step.v, val: big.NewRat(-1, 1)},
		}),
		sense: LessEqual,
		rhs:   new(big.Rat),
	}
	st.rows = append(st.rows, newCon)
	return true
}

func certApplyPresolveStep(st *certPresolveState, step certPresolveStep) bool {
	n := st.n
	switch step.op {
	case "roundBound":
		if step.col < 0 || step.col >= n || !st.isInt[step.col] {
			return false
		}
		if step.isUpper {
			oldU := st.ub[step.col]
			if oldU == nil {
				return false
			}
			fl := viprRatFloor(oldU)
			if fl.Cmp(step.ratVal) > 0 || step.ratVal.Cmp(oldU) > 0 {
				return false
			}
			st.ub[step.col] = new(big.Rat).Set(step.ratVal)
			return true
		}
		oldL := st.lb[step.col]
		if oldL == nil {
			return false
		}
		cl := viprRatCeil(oldL)
		if step.ratVal.Cmp(cl) > 0 || oldL.Cmp(step.ratVal) > 0 {
			return false
		}
		st.lb[step.col] = new(big.Rat).Set(step.ratVal)
		return true

	case "activityBound":
		return certPresolveActivityBound(st, step)

	case "gcdRowDiv":
		if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil || step.divisor == nil || step.divisor.Sign() <= 0 {
			return false
		}
		con := st.rows[step.row]
		dRat := new(big.Rat).SetInt(step.divisor)
		newTerms := make([]certRatTerm, len(con.terms))
		for k, t := range con.terms {
			if !st.isInt[t.col] {
				return false
			}
			q := new(big.Rat).Quo(t.val, dRat)
			if !q.IsInt() {
				return false
			}
			newTerms[k] = certRatTerm{col: t.col, val: q}
		}
		qRHS := new(big.Rat).Quo(con.rhs, dRat)
		var newRHS *big.Rat
		switch con.sense {
		case LessEqual:
			newRHS = viprRatFloor(qRHS)
		case GreaterEqual:
			newRHS = viprRatCeil(qRHS)
		case Equal:
			if !qRHS.IsInt() {
				return false
			}
			newRHS = qRHS
		default:
			return false
		}
		st.rows[step.row] = &certLinCon{
			terms: canonicalCertTerms(newTerms),
			sense: con.sense,
			rhs:   newRHS,
		}
		return true

	case "substFixedRow":
		if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil || step.col < 0 || step.col >= n {
			return false
		}
		if st.lb[step.col] == nil || st.ub[step.col] == nil ||
			st.lb[step.col].Cmp(step.ratVal) != 0 || st.ub[step.col].Cmp(step.ratVal) != 0 {
			return false
		}
		con := st.rows[step.row]
		a := certTermAt(n, con.terms, step.col)
		newTerms := make([]certRatTerm, 0, len(con.terms))
		for _, t := range con.terms {
			if t.col != step.col {
				newTerms = append(newTerms, certRatTerm{col: t.col, val: new(big.Rat).Set(t.val)})
			}
		}
		prod := new(big.Rat).Mul(a, step.ratVal)
		newRHS := new(big.Rat).Sub(con.rhs, prod)
		st.rows[step.row] = &certLinCon{
			terms: newTerms,
			sense: con.sense,
			rhs:   newRHS,
		}
		return true

	case "substFixedObj":
		if step.col < 0 || step.col >= n {
			return false
		}
		if st.lb[step.col] == nil || st.ub[step.col] == nil ||
			st.lb[step.col].Cmp(step.ratVal) != 0 || st.ub[step.col].Cmp(step.ratVal) != 0 {
			return false
		}
		prod := new(big.Rat).Mul(st.obj[step.col], step.ratVal)
		st.objConst.Add(st.objConst, prod)
		st.obj[step.col] = new(big.Rat)
		return true

	case "singletonRowElim":
		if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil || step.col < 0 || step.col >= n {
			return false
		}
		con := st.rows[step.row]
		if len(con.terms) != 1 || con.terms[0].col != step.col || con.terms[0].val.Sign() == 0 {
			return false
		}
		a := con.terms[0].val
		b := new(big.Rat).Quo(con.rhs, a)
		switch con.sense {
		case Equal:
			if step.roundInt && (!st.isInt[step.col] || !b.IsInt()) {
				return false
			}
			st.lb[step.col] = certTightenLBOpt(st.lb[step.col], b)
			st.ub[step.col] = certTightenUBOpt(st.ub[step.col], b)
		case LessEqual, GreaterEqual:
			isUpper := (con.sense == LessEqual) == (a.Sign() > 0)
			bnd := b
			if step.roundInt {
				if !st.isInt[step.col] {
					return false
				}
				if isUpper {
					bnd = viprRatFloor(b)
				} else {
					bnd = viprRatCeil(b)
				}
			}
			if isUpper {
				st.ub[step.col] = certTightenUBOpt(st.ub[step.col], bnd)
			} else {
				st.lb[step.col] = certTightenLBOpt(st.lb[step.col], bnd)
			}
		default:
			return false
		}
		st.rows[step.row] = nil
		return true

	case "redundantRow":
		if step.row < 0 || step.row >= len(st.rows) || st.rows[step.row] == nil {
			return false
		}
		con := st.rows[step.row]
		switch con.sense {
		case LessEqual:
			M, ok := certSumRowAct(st, con.terms, -1, true)
			if !ok || M.Cmp(con.rhs) > 0 {
				return false
			}
		case GreaterEqual:
			m, ok := certSumRowAct(st, con.terms, -1, false)
			if !ok || con.rhs.Cmp(m) > 0 {
				return false
			}
		case Equal:
			m, ok1 := certSumRowAct(st, con.terms, -1, false)
			M, ok2 := certSumRowAct(st, con.terms, -1, true)
			if !ok1 || !ok2 || con.rhs.Cmp(m) > 0 || M.Cmp(con.rhs) > 0 {
				return false
			}
		default:
			return false
		}
		st.rows[step.row] = nil
		return true

	case "binStrengthen":
		return certPresolveBinStrengthen(st, step)

	case "symBreakLe":
		return certPresolveSymBreakLe(st, step)

	default:
		return false
	}
}

func certRunPresolveSteps(orig *certVIPRInstance, steps []certPresolveStep) (*certVIPRInstance, bool) {
	st := newCertPresolveState(orig)
	for _, step := range steps {
		if !certApplyPresolveStep(st, step) {
			return nil, false
		}
	}
	return st.toInstance(), true
}

func verifyMiniVIPRBytes(raw []byte, expectedInst *certVIPRInstance) (certVIPRVerified, error) {
	doc, okDoc := unmarshalCertVIPRDoc(raw)
	if !okDoc {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	var origEnvInst *certVIPRInstance
	if certHasNonNullJSON(doc.Orig) {
		var okOrig bool
		origEnvInst, okOrig = decodeVIPRInstanceRaw(doc.Orig)
		if !okOrig {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		if expectedInst != nil && !verifyInstancesMatch(origEnvInst, expectedInst) {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		preSteps, okPre := decodeVIPRPresolveSteps(doc.Presolve)
		if !okPre {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		finalPreInst, okRun := certRunPresolveSteps(origEnvInst, preSteps)
		if !okRun {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		inst, okInst := decodeMiniVIPRInstanceDoc(doc)
		if !okInst || !verifyInstancesMatch(finalPreInst, inst) {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		return verifyMiniVIPRCore(doc, inst, origEnvInst)
	}
	if certHasNonNullJSON(doc.Presolve) {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	inst, okInst := decodeMiniVIPRInstanceDoc(doc)
	if !okInst {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	if expectedInst != nil && !verifyInstancesMatch(inst, expectedInst) {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	return verifyMiniVIPRCore(doc, inst, nil)
}

func certParseRatSlice(raw []viprRawJSON, n int) ([]*big.Rat, bool) {
	if len(raw) != n {
		return nil, false
	}
	out := make([]*big.Rat, n)
	for j := range n {
		r, ok := parseVIPRRatValue(raw[j])
		if !ok {
			return nil, false
		}
		out[j] = r
	}
	return out, true
}

func certVerifyIncumbentAndObj(inst, origEnvInst *certVIPRInstance, incumbent []*big.Rat) (incLin, certMinObj *big.Rat, ok bool) {
	if !certCheckIncumbentFeasible(inst, incumbent) {
		return nil, nil, false
	}
	incLin = certDotTerms(inst.objTerms, incumbent)
	certMinObj = new(big.Rat).Add(inst.objConst, incLin)
	if origEnvInst != nil {
		if !certCheckIncumbentFeasible(origEnvInst, incumbent) {
			return nil, nil, false
		}
		origObj := new(big.Rat).Add(origEnvInst.objConst, certDotTerms(origEnvInst.objTerms, incumbent))
		if origObj.Cmp(certMinObj) != 0 {
			return nil, nil, false
		}
	}
	return incLin, certMinObj, true
}

func verifyMiniVIPRCore(doc certVIPRDoc, inst, origEnvInst *certVIPRInstance) (certVIPRVerified, error) {
	var incumbent []*big.Rat
	var targetLB *big.Rat
	switch doc.Mode {
	case "infeasible":
	case "optimal":
		var okInc bool
		incumbent, okInc = certParseRatSlice(doc.Incumbent, inst.n)
		if !okInc {
			return certVIPRVerified{}, ErrCertificationFailed
		}
	case "bound":
		rLB, okLB := parseVIPRRatValue(doc.TargetLB)
		if !okLB {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		targetLB = rLB
		if len(doc.Incumbent) > 0 {
			var okInc bool
			incumbent, okInc = certParseRatSlice(doc.Incumbent, inst.n)
			if !okInc {
				return certVIPRVerified{}, ErrCertificationFailed
			}
		}
	default:
		return certVIPRVerified{}, ErrCertificationFailed
	}

	rootStep := len(doc.Steps) - 1
	if doc.RootStep != nil {
		rootStep = *doc.RootStep
	}
	if rootStep < 0 || rootStep >= len(doc.Steps) {
		return certVIPRVerified{}, ErrCertificationFailed
	}

	entries := make([]certDerivedEntry, len(inst.baseRows), len(inst.baseRows)+len(doc.Steps))
	for i, r := range inst.baseRows {
		entries[i] = certDerivedEntry{con: r}
	}
	for _, st := range doc.Steps {
		ent, okStep := certEvalProofStep(inst, entries, st)
		if !okStep {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		entries = append(entries, ent)
	}

	rootIdx := len(inst.baseRows) + rootStep
	if rootIdx < 0 || rootIdx >= len(entries) {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	rootEnt := entries[rootIdx]
	if len(rootEnt.assumptions) != 0 {
		return certVIPRVerified{}, ErrCertificationFailed
	}

	switch doc.Mode {
	case "infeasible":
		if !certIsUnsatCon(rootEnt.con) {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		return certVIPRVerified{mode: "infeasible"}, nil

	case "optimal":
		incLin, certMinLB, okInc := certVerifyIncumbentAndObj(inst, origEnvInst, incumbent)
		if !okInc {
			return certVIPRVerified{}, ErrCertificationFailed
		}
		if !certIsUnsatCon(rootEnt.con) {
			okObj := certTermsEqual(rootEnt.con.terms, inst.objTerms) &&
				(rootEnt.con.sense == GreaterEqual || rootEnt.con.sense == Equal) &&
				incLin.Cmp(rootEnt.con.rhs) <= 0
			if !okObj {
				return certVIPRVerified{}, ErrCertificationFailed
			}
		}
		return certVIPRVerified{
			mode:           "optimal",
			incumbent:      incumbent,
			certifiedMinLB: certMinLB,
		}, nil

	case "bound":
		if incumbent != nil {
			if _, _, okInc := certVerifyIncumbentAndObj(inst, origEnvInst, incumbent); !okInc {
				return certVIPRVerified{}, ErrCertificationFailed
			}
		}
		if !certIsUnsatCon(rootEnt.con) {
			derivedLB := new(big.Rat).Add(inst.objConst, rootEnt.con.rhs)
			okBnd := certTermsEqual(rootEnt.con.terms, inst.objTerms) &&
				(rootEnt.con.sense == GreaterEqual || rootEnt.con.sense == Equal) &&
				targetLB.Cmp(derivedLB) <= 0
			if !okBnd {
				return certVIPRVerified{}, ErrCertificationFailed
			}
		}
		return certVIPRVerified{
			mode:           "bound",
			incumbent:      incumbent,
			certifiedMinLB: targetLB,
		}, nil
	}
	return certVIPRVerified{}, ErrCertificationFailed
}

func verifyMiniVIPRProblem(p *CompiledProblem, raw []byte) (certVIPRVerified, error) {
	origInst, ok := buildCertVIPRInstance(p)
	if !ok {
		return certVIPRVerified{}, ErrCertificationFailed
	}
	return verifyMiniVIPRBytes(raw, origInst)
}
