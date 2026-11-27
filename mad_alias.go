package goblin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/paulourio/goblin/mad"
)

// MADProblem aliases mad.Problem for callers importing the root package.
type MADProblem = mad.Problem

// MADResult aliases mad.Result and holds the optimal group assignment, levels,
// rounding steps, objective, bound, and solve metadata.
type MADResult = mad.Result

// MADOption configures SolveMAD.
type MADOption = mad.Option

// MADOptions stores search strategy, limits, MILP backend, and proof flags.
type MADOptions = mad.Options

// MADStrategy selects the search engine used by SolveMAD.
type MADStrategy = mad.Strategy

// MADStatus reports the terminal outcome of SolveMAD.
type MADStatus = mad.Status

// MADModelVars holds the decision variables created by BuildMADModel.
type MADModelVars struct {
	X [][]Var // Binary group-item indicators [group][item].
	Z [][]Var // Continuous L1 slack variables [group][period].
	M []Var   // Continuous central-level variables [group]; nil when Step > 0 && RoundWeight >= float64(T).
	N []Var   // Integer rounding-step variables [group]; nil when unrounded.
	R []Var   // Continuous rounding-gap variables [group]; nil when unrounded or substituted.
}

const (
	// MADAuto dispatches to combinatorial search within caps and falls back to MILP otherwise.
	MADAuto MADStrategy = mad.StrategyAuto
	// MADBruteForce runs exact incremental double-double depth-first search.
	MADBruteForce MADStrategy = mad.StrategyBruteForce
	// MADDP runs exact subset bitmask dynamic programming.
	MADDP MADStrategy = mad.StrategyDP
	// MADMILP solves the algebraic MILP model via Model.Solve.
	MADMILP MADStrategy = mad.StrategyMILP
)

const (
	// MADStatusOptimal indicates a proven globally optimal discrete answer.
	MADStatusOptimal MADStatus = mad.StatusOptimal
	// MADStatusTimeLimit indicates WithMADTimeLimit expired before proving optimality.
	MADStatusTimeLimit MADStatus = mad.StatusTimeLimit
	// MADStatusNodeLimit indicates the MILP fallback reached WithMADMaxNodes.
	MADStatusNodeLimit MADStatus = mad.StatusNodeLimit
	// MADStatusIterationLimit indicates the MILP fallback reached WithMADMaxIter.
	MADStatusIterationLimit MADStatus = mad.StatusIterationLimit
)

// DefaultMADOptions returns default settings for SolveMAD.
func DefaultMADOptions() MADOptions {
	return mad.DefaultOptions()
}

// WithMADOptions replaces SolveMAD options with o while preserving installed fallback hooks
// when o leaves them unset.
func WithMADOptions(o MADOptions) MADOption {
	return mad.WithOptions(o)
}

// WithMADStrategy selects the search strategy for SolveMAD.
func WithMADStrategy(s MADStrategy) MADOption {
	return mad.WithStrategy(s)
}

// WithMADBackend sets the MILP solver backend used when SolveMAD invokes BuildMADModel.
func WithMADBackend(b Backend) MADOption {
	return mad.WithMILPBackend(uint8(b))
}

// WithMADTimeLimit sets the wall-clock search budget for SolveMAD.
func WithMADTimeLimit(d time.Duration) MADOption {
	return mad.WithTimeLimit(d)
}

// WithMADMaxNodes sets the MILP node limit forwarded when SolveMAD delegates to Model.Solve.
func WithMADMaxNodes(n int) MADOption {
	return mad.WithMaxNodes(n)
}

// WithMADMaxIter sets the MILP simplex pivot limit forwarded when SolveMAD delegates to Model.Solve.
func WithMADMaxIter(n int) MADOption {
	return mad.WithMaxIter(n)
}

// WithMADRelGapTol sets the relative MIP gap bound forwarded when SolveMAD delegates to Model.Solve.
func WithMADRelGapTol(tol float64) MADOption {
	return mad.WithRelGapTol(tol)
}

// WithMADAbsGapTol sets the absolute MIP gap bound forwarded when SolveMAD delegates to Model.Solve.
func WithMADAbsGapTol(tol float64) MADOption {
	return mad.WithAbsGapTol(tol)
}

func setMADAliasCertifiedTrue(cfg *mad.Options) {
	mad.WithCertified(true)(cfg)
	mad.WithCertFallback(certifiedMILPFallback)(cfg)
}

func setMADAliasCertifiedFalse(cfg *mad.Options) {
	mad.WithCertified(false)(cfg)
	mad.WithCertFallback(nil)(cfg)
}

// WithMADCertified enables or disables fail-closed exact rational checks for both
// combinatorial search and MILP fallback.
func WithMADCertified(enabled bool) MADOption {
	if enabled {
		return setMADAliasCertifiedTrue
	}
	return setMADAliasCertifiedFalse
}

// BuildMADModel compiles prob into a tightened algebraic *Model with outward step bounds
// and replaces m[g] with Step * n[g] when RoundWeight >= T.
func BuildMADModel(prob MADProblem) (*Model, MADModelVars, error) {
	if err := mad.ValidateProblem(prob); err != nil {
		return nil, MADModelVars{}, err
	}

	numGroups := prob.Groups
	var numPeriods int
	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
		numPeriods = len(prob.Baselines[0])
	} else {
		numPeriods = len(prob.Items[0])
	}
	numItems := len(prob.Items)

	rounded := prob.Step > 0 && prob.RoundWeight > 0
	directStep := rounded && prob.RoundWeight >= float64(numPeriods)
	nMin := 1.0
	if prob.AllowZeroStep {
		nMin = 0.0
	}

	negSum := make([]float64, numPeriods)
	posSum := make([]float64, numPeriods)
	for i := range numItems {
		row := prob.Items[i]
		for t := range numPeriods {
			d := row[t]
			if d < 0 {
				negSum[t] = math.Nextafter(negSum[t]+d, math.Inf(-1))
			} else if d > 0 {
				posSum[t] = math.Nextafter(posSum[t]+d, math.Inf(1))
			}
		}
	}

	m := NewModel("mad", Minimize)
	vars := MADModelVars{
		X: make([][]Var, numGroups),
		Z: make([][]Var, numGroups),
	}
	if rounded {
		vars.N = make([]Var, numGroups)
		if !directStep {
			vars.M = make([]Var, numGroups)
			vars.R = make([]Var, numGroups)
		}
	} else {
		vars.M = make([]Var, numGroups)
	}

	objTerms := make([]Term, 0, numGroups*(numPeriods+1))
	yMin := make([]float64, numPeriods)
	yMax := make([]float64, numPeriods)
	margin := float64(numItems + 2)

	for g := range numGroups {
		vars.X[g] = make([]Var, numItems)
		for i := range numItems {
			vars.X[g][i] = m.AddBinaryVar(fmt.Sprintf("x_%02d_%02d", g, i))
		}

		vars.Z[g] = make([]Var, numPeriods)
		for t := range numPeriods {
			vars.Z[g][t] = m.AddContVar(fmt.Sprintf("z_%02d_%02d", g, t), 0, math.Inf(1))
			objTerms = append(objTerms, vars.Z[g][t].T(1.0))
		}

		var maxPeriodY float64
		for t := range numPeriods {
			a := 0.0
			if prob.Baselines != nil {
				a = prob.Baselines[g][t]
			}
			lo := a
			if negSum[t] != 0 {
				lo = math.Nextafter(a+negSum[t], math.Inf(-1))
			}
			hi := a
			if posSum[t] != 0 {
				hi = math.Nextafter(a+posSum[t], math.Inf(1))
			}
			yMin[t] = lo
			yMax[t] = hi
			if t == 0 || hi > maxPeriodY {
				maxPeriodY = hi
			}
		}
		slices.Sort(yMin)
		slices.Sort(yMax)
		uMinG := yMin[(numPeriods-1)/2]
		uMaxG := yMax[numPeriods/2]

		if rounded {
			loDiv := 0.0
			if uMinG > 0 {
				loDiv = math.Nextafter(uMinG/prob.Step, math.Inf(-1))
			}
			hiDiv := 0.0
			if uMaxG > 0 {
				hiDiv = math.Nextafter(uMaxG/prob.Step, math.Inf(1))
			}
			nLB := max(nMin, math.Floor(loDiv)-margin)
			nUB := max(nMin, math.Nextafter(math.Ceil(hiDiv)+margin, math.Inf(1)))
			vars.N[g] = m.AddIntVar(fmt.Sprintf("n_%02d", g), nLB, nUB)
			if !directStep {
				mUB := max(0.0, maxPeriodY, math.Nextafter(prob.Step*nUB, math.Inf(1)))
				vars.M[g] = m.AddContVar(fmt.Sprintf("m_%02d", g), 0, mUB)
				vars.R[g] = m.AddContVar(fmt.Sprintf("r_%02d", g), 0, math.Inf(1))
				objTerms = append(objTerms, vars.R[g].T(prob.RoundWeight))
			}
		} else {
			mUB := max(0.0, maxPeriodY)
			vars.M[g] = m.AddContVar(fmt.Sprintf("m_%02d", g), 0, mUB)
		}
	}
	m.SetObjectiveNamed("total_mad", Expr{Terms: objTerms})

	for g := range numGroups {
		var posLevelTerm, negLevelTerm Term
		if directStep {
			posLevelTerm = vars.N[g].T(prob.Step)
			negLevelTerm = vars.N[g].T(-prob.Step)
		} else {
			posLevelTerm = vars.M[g].T(1.0)
			negLevelTerm = vars.M[g].T(-1.0)
		}
		for t := range numPeriods {
			a := 0.0
			if prob.Baselines != nil {
				a = prob.Baselines[g][t]
			}
			if a == 0 {
				a = 0
			}
			negA := -a
			if negA == 0 {
				negA = 0
			}
			posTerms := make([]Term, 0, numItems+2)
			negTerms := make([]Term, 0, numItems+2)
			posTerms = append(posTerms, vars.Z[g][t].T(1.0), posLevelTerm)
			negTerms = append(negTerms, vars.Z[g][t].T(1.0), negLevelTerm)
			for i := range numItems {
				d := prob.Items[i][t]
				if d != 0 {
					posTerms = append(posTerms, vars.X[g][i].T(-d))
					negTerms = append(negTerms, vars.X[g][i].T(d))
				}
			}
			m.AddConstraint(fmt.Sprintf("dev_pos_%02d_%02d", g, t), Ge(Expr{Terms: posTerms}, a))
			m.AddConstraint(fmt.Sprintf("dev_neg_%02d_%02d", g, t), Ge(Expr{Terms: negTerms}, negA))
		}

		if rounded && !directStep {
			m.AddConstraint(fmt.Sprintf("round_pos_%02d", g), Ge(NewExpr(
				vars.R[g].T(1.0),
				vars.M[g].T(-1.0),
				vars.N[g].T(prob.Step),
			), 0))
			m.AddConstraint(fmt.Sprintf("round_neg_%02d", g), Ge(NewExpr(
				vars.R[g].T(1.0),
				vars.M[g].T(1.0),
				vars.N[g].T(-prob.Step),
			), 0))
		}
	}

	for i := range numItems {
		terms := make([]Term, numGroups)
		for g := range numGroups {
			terms[g] = vars.X[g][i].T(1.0)
		}
		m.AddConstraint(fmt.Sprintf("item_%02d", i), Eq(Expr{Terms: terms}, 1.0))
	}

	if len(prob.DesignatedItems) > 0 {
		for g := range numGroups {
			terms := make([]Term, len(prob.DesignatedItems))
			for idx, item := range prob.DesignatedItems {
				terms[idx] = vars.X[g][item].T(1.0)
			}
			m.AddConstraint(fmt.Sprintf("designated_%02d", g), Eq(Expr{Terms: terms}, 1.0))
		}
	}

	return m, vars, nil
}

// SolveMAD solves prob using the combinatorial engines in goblin/mad when within caps
// and falls back to BuildMADModel + Model.Solve automatically when required.
func SolveMAD(ctx context.Context, prob MADProblem, opts ...MADOption) (MADResult, error) {
	cfg := mad.ApplyOptions(opts...)
	if !cfg.HasFallback() {
		withFallback := make([]MADOption, len(opts)+1)
		copy(withFallback, opts)
		withFallback[len(opts)] = mad.WithFallback(defaultMILPFallback)
		return mad.Solve(ctx, prob, withFallback...)
	}
	return mad.Solve(ctx, prob, opts...)
}

func buildAndSolveMADMILP(
	ctx context.Context,
	prob MADProblem,
	opt MADOptions,
	extraOpts ...Option,
) (*Model, MADModelVars, Result, time.Duration, error) {
	start := time.Now()
	m, vars, err := BuildMADModel(prob)
	if err != nil {
		return nil, MADModelVars{}, Result{}, 0, err
	}

	mOpts := make([]Option, 0, 6+len(extraOpts))
	mOpts = append(mOpts, WithBackend(Backend(opt.MILPBackend)))
	if opt.TimeLimit != 0 {
		mOpts = append(mOpts, WithTimeLimit(opt.TimeLimit))
	}
	if opt.MaxNodesSet() {
		mOpts = append(mOpts, WithMaxNodes(opt.MaxNodes))
	}
	if opt.MaxIterSet() {
		mOpts = append(mOpts, WithMaxIter(opt.MaxIter))
	}
	if opt.RelGapTolSet() {
		mOpts = append(mOpts, WithRelGapTol(opt.RelGapTol))
	}
	if opt.AbsGapTolSet() {
		mOpts = append(mOpts, WithAbsGapTol(opt.AbsGapTol))
	}
	mOpts = append(mOpts, extraOpts...)

	res, solveErr := m.Solve(ctx, mOpts...)
	return m, vars, res, time.Since(start), solveErr
}

func madStatusFromMILP(st Status) (MADStatus, bool) {
	switch st {
	case StatusTimeLimit:
		return MADStatusTimeLimit, true
	case StatusNodeLimit:
		return MADStatusNodeLimit, true
	case StatusIterationLimit:
		return MADStatusIterationLimit, true
	default:
		return 0, false
	}
}

func clampMILPBestBound(milpBound, upper float64) float64 {
	if math.IsNaN(milpBound) || math.IsInf(milpBound, -1) || milpBound <= 0 {
		return 0
	}
	if math.IsInf(milpBound, 1) || milpBound > upper {
		return upper
	}
	return milpBound
}

func resolvePolishedMILPBound(status Status, milpBound float64, oracleObj, oracleBound float64) float64 {
	if status == StatusOptimal && (math.IsNaN(milpBound) || math.IsInf(milpBound, -1)) {
		return oracleBound
	}
	return clampMILPBestBound(milpBound, oracleObj)
}

func extractRoundedMADAssignment(vars MADModelVars, values []float64) ([]int, error) {
	numGroups := len(vars.X)
	if numGroups == 0 {
		return nil, mad.ErrInvalidProblem
	}
	numItems := len(vars.X[0])
	assignment := make([]int, numItems)
	for i := range numItems {
		bestG := 0
		bestV := math.Inf(-1)
		for g := range numGroups {
			idx := vars.X[g][i].Index()
			if idx < 0 || idx >= len(values) {
				return nil, fmt.Errorf("goblin: MILP variable index %d out of range", idx)
			}
			v := values[idx]
			if math.IsNaN(v) {
				return nil, fmt.Errorf("goblin: MILP returned NaN for x[%d][%d]", g, i)
			}
			if v > bestV {
				bestV = v
				bestG = g
			}
		}
		assignment[i] = bestG
	}
	return assignment, nil
}

func extractExactMADAssignment(vars MADModelVars, values []float64) ([]int, error) {
	numGroups := len(vars.X)
	if numGroups == 0 {
		return nil, mad.ErrInvalidProblem
	}
	numItems := len(vars.X[0])
	assignment := make([]int, numItems)
	for i := range numItems {
		chosen := -1
		for g := range numGroups {
			idx := vars.X[g][i].Index()
			if idx < 0 || idx >= len(values) {
				return nil, fmt.Errorf("%w: MILP variable index %d out of range", mad.ErrCertificationFailed, idx)
			}
			switch values[idx] {
			case 1.0:
				if chosen >= 0 {
					return nil, fmt.Errorf("%w: item %d assigned to multiple groups", mad.ErrCertificationFailed, i)
				}
				chosen = g
			case 0.0:
			default:
				return nil, fmt.Errorf("%w: non-binary value %v for x[%d][%d]", mad.ErrCertificationFailed, values[idx], g, i)
			}
		}
		if chosen < 0 {
			return nil, fmt.Errorf("%w: item %d unassigned", mad.ErrCertificationFailed, i)
		}
		assignment[i] = chosen
	}
	return assignment, nil
}

func madResultFromMILP(
	prob MADProblem,
	vars MADModelVars,
	numVars int,
	res Result,
	elapsed time.Duration,
) (MADResult, error) {
	if res.Status == StatusOptimal {
		if len(res.Values) != numVars {
			return MADResult{}, fmt.Errorf("goblin: MAD MILP optimal result has %d values, want %d", len(res.Values), numVars)
		}
		assignment, err := extractRoundedMADAssignment(vars, res.Values)
		if err != nil {
			return MADResult{}, err
		}
		polRes, pErr := mad.EvaluateAssignment(prob, assignment)
		if pErr != nil {
			return MADResult{}, pErr
		}
		polRes.BestBound = resolvePolishedMILPBound(res.Status, res.BestBound, polRes.Objective, polRes.BestBound)
		polRes.Strategy = MADMILP
		polRes.Status = MADStatusOptimal
		polRes.SolveTime = elapsed
		return polRes, nil
	}

	madStatus, isLimit := madStatusFromMILP(res.Status)
	if !isLimit {
		return MADResult{}, fmt.Errorf("goblin: MAD MILP solve failed with status %s", res.Status)
	}

	if len(res.Values) == numVars {
		assignment, err := extractRoundedMADAssignment(vars, res.Values)
		if err != nil {
			return MADResult{}, err
		}
		polRes, pErr := mad.EvaluateAssignment(prob, assignment)
		if pErr != nil {
			return MADResult{}, pErr
		}
		polRes.BestBound = resolvePolishedMILPBound(res.Status, res.BestBound, polRes.Objective, polRes.BestBound)
		polRes.Strategy = MADMILP
		polRes.Status = madStatus
		polRes.SolveTime = elapsed
		return polRes, nil
	}
	if len(res.Values) != 0 {
		return MADResult{}, fmt.Errorf("goblin: unexpected MAD MILP solution length %d, want 0 or %d", len(res.Values), numVars)
	}

	gRes, gErr := mad.GreedyIncumbent(prob)
	if gErr != nil {
		return MADResult{}, gErr
	}
	gRes.BestBound = clampMILPBestBound(res.BestBound, gRes.Objective)
	gRes.Strategy = MADMILP
	gRes.Status = madStatus
	gRes.SolveTime = elapsed
	return gRes, nil
}

func defaultMILPFallback(ctx context.Context, prob MADProblem, opt MADOptions) (MADResult, error) {
	m, vars, res, elapsed, err := buildAndSolveMADMILP(ctx, prob, opt)
	if err != nil {
		return MADResult{}, err
	}
	return madResultFromMILP(prob, vars, m.NumVars(), res, elapsed)
}

func dyadicLimbsToRat(limbs []uint64, exp int) *big.Rat {
	num := new(big.Int)
	var word big.Int
	for _, limb := range slices.Backward(limbs) {
		num.Lsh(num, 64)
		if limb != 0 {
			word.SetUint64(limb)
			num.Or(num, &word)
		}
	}
	if num.Sign() == 0 {
		return new(big.Rat)
	}
	if exp > 0 {
		num.Lsh(num, uint(exp))
		return new(big.Rat).SetInt(num)
	}
	if exp < 0 {
		den := new(big.Int).Lsh(big.NewInt(1), uint(-exp))
		return new(big.Rat).SetFrac(num, den)
	}
	return new(big.Rat).SetInt(num)
}

func madResultFromCertifiedMILP(
	prob MADProblem,
	vars MADModelVars,
	numVars int,
	res Result,
	cw CertifiedWitness,
	elapsed time.Duration,
) (MADResult, error) {
	var assignment []int
	var outStatus MADStatus
	fromMILPIncumbent := false

	switch res.Status {
	case StatusOptimal:
		if len(res.Values) != numVars || cw.Objective == nil || cw.BestBound == nil {
			return MADResult{}, fmt.Errorf("%w: incomplete optimal MILP witness", mad.ErrCertificationFailed)
		}
		var err error
		assignment, err = extractExactMADAssignment(vars, res.Values)
		if err != nil {
			return MADResult{}, err
		}
		outStatus = MADStatusOptimal
		fromMILPIncumbent = true
	default:
		madStatus, isLimit := madStatusFromMILP(res.Status)
		if !isLimit {
			return MADResult{}, fmt.Errorf("%w: MILP status %s", mad.ErrCertificationFailed, res.Status)
		}
		outStatus = madStatus
		switch len(res.Values) {
		case numVars:
			var err error
			assignment, err = extractExactMADAssignment(vars, res.Values)
			if err != nil {
				return MADResult{}, err
			}
			fromMILPIncumbent = true
		case 0:
			gRes, gErr := mad.GreedyIncumbent(prob)
			if gErr != nil {
				return MADResult{}, fmt.Errorf("%w: %w", mad.ErrCertificationFailed, gErr)
			}
			assignment = gRes.Assignment
		default:
			return MADResult{}, fmt.Errorf("%w: unexpected MILP solution length %d", mad.ErrCertificationFailed, len(res.Values))
		}
	}

	ev, evalErr := mad.EvaluateAssignmentDyadic(prob, assignment)
	if evalErr != nil {
		return MADResult{}, fmt.Errorf("%w: %w", mad.ErrCertificationFailed, evalErr)
	}
	evRat := dyadicLimbsToRat(ev.Limbs, ev.Exp)
	if fromMILPIncumbent && cw.Objective != nil && evRat.Cmp(cw.Objective) > 0 {
		return MADResult{}, fmt.Errorf("%w: dyadic oracle %s exceeds MILP witness %s",
			mad.ErrCertificationFailed, evRat.RatString(), cw.Objective.RatString())
	}
	if cw.BestBound != nil && evRat.Cmp(cw.BestBound) < 0 {
		return MADResult{}, fmt.Errorf("%w: dyadic oracle %s below MILP dual bound %s",
			mad.ErrCertificationFailed, evRat.RatString(), cw.BestBound.RatString())
	}

	out := ev.Result
	out.BestBound = resolvePolishedMILPBound(res.Status, res.BestBound, out.Objective, out.BestBound)
	out.Strategy = MADMILP
	out.Status = outStatus
	out.SolveTime = elapsed
	return out, nil
}

func certifiedMILPFallback(ctx context.Context, prob MADProblem, opt MADOptions) (MADResult, error) {
	if !opt.Certified {
		return MADResult{}, fmt.Errorf("%w: certified fallback invoked without Certified=true", mad.ErrCertificationFailed)
	}
	var cw CertifiedWitness
	m, vars, res, elapsed, err := buildAndSolveMADMILP(ctx, prob, opt, WithCertifiedWitness(&cw))
	if err != nil {
		if ctx != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return MADResult{}, err
		}
		if errors.Is(err, ErrCertificationFailed) {
			return MADResult{}, fmt.Errorf("%w: %w", mad.ErrCertificationFailed, err)
		}
		return MADResult{}, err
	}
	return madResultFromCertifiedMILP(prob, vars, m.NumVars(), res, cw, elapsed)
}
