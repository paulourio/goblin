package goblin

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"time"
)

const (
	viprMaxPropRowLen   = 512
	viprMaxRatDenomBits = 2048
	viprMaxNucleusSize  = 1024
)

var (
	errVIPRNoCertificate = errors.New("goblin: VIPRTrace holds no verified certificate")
	errVIPRNilTrace      = errors.New("goblin: nil VIPRTrace")
)

type viprRatPayload struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

func viprBigRatToPayload(r *big.Rat) viprRatPayload {
	if r == nil {
		return viprRatPayload{Num: "0", Den: "1"}
	}
	return viprRatPayload{
		Num: r.Num().String(),
		Den: r.Denom().String(),
	}
}

func viprInt64Payload(v int64) viprRatPayload {
	return viprRatPayload{
		Num: strconv.FormatInt(v, 10),
		Den: "1",
	}
}

type viprLinRefJSON struct {
	ID   int            `json:"id"`
	Mult viprRatPayload `json:"mult"`
}

type viprBoundRefJSON struct {
	Col  int            `json:"col"`
	Kind string         `json:"kind"`
	Mult viprRatPayload `json:"mult"`
}

type viprLinConJSON struct {
	Coeffs []viprRatPayload `json:"coeffs"`
	Sense  string           `json:"sense"`
	RHS    viprRatPayload   `json:"rhs"`
}

type viprOrbitWitnessJSON struct {
	Companion      int   `json:"companion"`
	ColPerm        []int `json:"colPerm"`
	RowPerm        []int `json:"rowPerm"`
	SupportEntries []int `json:"supportEntries,omitzero"`
}

type viprPresolveStepJSON struct {
	Op       string          `json:"op"`
	Row      *int            `json:"row,omitzero"`
	Col      *int            `json:"col,omitzero"`
	U        *int            `json:"u,omitzero"`
	V        *int            `json:"v,omitzero"`
	IsUpper  *bool           `json:"isUpper,omitzero"`
	RoundInt *bool           `json:"roundInt,omitzero"`
	Divisor  *int64          `json:"divisor,omitzero"`
	NewBound *viprRatPayload `json:"newBound,omitzero"`
	Val      *viprRatPayload `json:"val,omitzero"`
	Delta    *viprRatPayload `json:"delta,omitzero"`
	ColPerm  []int           `json:"colPerm,omitzero"`
	RowPerm  []int           `json:"rowPerm,omitzero"`
}

type viprInstanceJSON struct {
	N        int               `json:"n"`
	ObjConst viprRatPayload    `json:"objConst"`
	Obj      []viprRatPayload  `json:"obj"`
	IsInt    []bool            `json:"isInt"`
	LB       []*viprRatPayload `json:"lb"`
	UB       []*viprRatPayload `json:"ub"`
	BaseRows []viprLinConJSON  `json:"baseRows"`
}

type viprStepJSON struct {
	Rule        viprRule               `json:"rule"`
	Row         *int                   `json:"row,omitzero"`
	Col         *int                   `json:"col,omitzero"`
	Sense       string                 `json:"sense,omitzero"`
	Bound       viprRawJSON            `json:"bound,omitzero"`
	Refs        *[]viprLinRefJSON      `json:"refs,omitzero"`
	Bounds      []viprBoundRefJSON     `json:"bounds,omitzero"`
	BoundAsms   []int                  `json:"boundAsms,omitzero"`
	Ref         *int                   `json:"ref,omitzero"`
	SplitVal    viprRawJSON            `json:"splitVal,omitzero"`
	LeftAsm     *int                   `json:"leftAsm,omitzero"`
	RightAsm    *int                   `json:"rightAsm,omitzero"`
	Left        *int                   `json:"left,omitzero"`
	Right       *int                   `json:"right,omitzero"`
	OrbitAsms   []int                  `json:"orbitAsms,omitzero"`
	Witnesses   []viprOrbitWitnessJSON `json:"witnesses,omitzero"`
	ExtraAsms   []int                  `json:"extraAsms,omitzero"`
	LBRef       *int                   `json:"lbRef,omitzero"`
	UBRef       *int                   `json:"ubRef,omitzero"`
	Assumptions []int                  `json:"assumptions,omitzero"`
}

type viprCertJSON struct {
	Orig      *viprInstanceJSON      `json:"orig,omitzero"`
	Presolve  []viprPresolveStepJSON `json:"presolve,omitzero"`
	N         int                    `json:"n"`
	ObjConst  viprRatPayload         `json:"objConst"`
	Obj       []viprRatPayload       `json:"obj"`
	IsInt     []bool                 `json:"isInt"`
	LB        []*viprRatPayload      `json:"lb"`
	UB        []*viprRatPayload      `json:"ub"`
	BaseRows  []viprLinConJSON       `json:"baseRows"`
	Mode      viprMode               `json:"mode"`
	TargetLB  *viprRatPayload        `json:"targetLB,omitzero"`
	Incumbent []viprRatPayload       `json:"incumbent,omitzero"`
	Steps     []viprStepJSON         `json:"steps"`
	RootStep  int                    `json:"rootStep"`
}

func viprBoolPtr(v bool) *bool {
	b := v
	return &b
}

func viprInt64Ptr(v int64) *int64 {
	x := v
	return &x
}

func viprRatPtr(r *big.Rat) *viprRatPayload {
	if r == nil {
		return nil
	}
	p := viprBigRatToPayload(r)
	return &p
}

func viprIntPtr(v int) *int {
	p := v
	return &p
}

func viprBigIntJSON(v *big.Int) viprRawJSON {
	if v == nil {
		return nil
	}
	return viprRawJSON(v.String())
}

func viprFloorBigInt(v float64) (*big.Int, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, false
	}
	r, ok := exactRatFromFloat(math.Floor(v))
	if !ok || !r.IsInt() {
		return nil, false
	}
	return new(big.Int).Set(r.Num()), true
}

func viprCeilBigInt(v float64) (*big.Int, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, false
	}
	r, ok := exactRatFromFloat(math.Ceil(v))
	if !ok || !r.IsInt() {
		return nil, false
	}
	return new(big.Int).Set(r.Num()), true
}

func viprPow2Rat(exp int) *big.Rat {
	if exp >= 0 {
		num := new(big.Int).Lsh(big.NewInt(1), uint(exp))
		return new(big.Rat).SetInt(num)
	}
	den := new(big.Int).Lsh(big.NewInt(1), uint(-exp))
	return new(big.Rat).SetFrac(big.NewInt(1), den)
}

func viprOptNonNegPtr(v int) *int {
	if v < 0 {
		return nil
	}
	return viprIntPtr(v)
}

func viprOptRefs(r []viprLinRefJSON) *[]viprLinRefJSON {
	if r == nil {
		empty := []viprLinRefJSON{}
		return &empty
	}
	return &r
}

// VIPRTrace records an exact rational Mini-VIPR proof tree during a single-threaded
// BackendGo Branch-and-Bound solve configured with WithVIPRTrace.
//
// When MIP presolve reduces the input problem before Branch-and-Bound, VIPRTrace attaches
// an exact rational presolve proof envelope (Orig -> Reduced) so TracedProblem reports the
// original CompiledProblem. When presolve proves no feasible point exists before
// Branch-and-Bound, PresolveInfeasible reports true and VIPRTrace emits a presolve-enveloped
// or root proof against the original problem.
type VIPRTrace struct {
	tracedProb             *CompiledProblem
	presolveEnv            *VIPRPresolveEnvelope
	cert                   viprCertJSON
	targetLB               *big.Rat
	presolveInfeasible     bool
	hasCert                bool
	disableOrbit           bool
	rootCutsCertified      int
	inTreeCutsCertified    int
	rcfFixingsCertified    int
	unbTightCertified      int
	orbitBranchesCertified int
}

// NewVIPRTrace allocates an empty VIPRTrace for use with WithVIPRTrace.
func NewVIPRTrace() *VIPRTrace {
	return &VIPRTrace{}
}

// WithVIPRTrace enables single-threaded rational Mini-VIPR proof tracing into tr on BackendGo.
func WithVIPRTrace(tr *VIPRTrace) Option {
	return func(cfg *Options) {
		if tr == nil {
			cfg.viprTrace = nil
			return
		}
		cfg.viprTrace = tr
		cfg.Workers = 1
	}
}

func (tr *VIPRTrace) reset(p *CompiledProblem) {
	tr.tracedProb = p
	if tr.presolveEnv != nil && tr.presolveEnv.Reduced != p {
		tr.presolveEnv = nil
	}
	tr.cert = viprCertJSON{}
	tr.targetLB = nil
	tr.presolveInfeasible = false
	tr.hasCert = false
	tr.rootCutsCertified = 0
	tr.inTreeCutsCertified = 0
	tr.rcfFixingsCertified = 0
	tr.unbTightCertified = 0
	tr.orbitBranchesCertified = 0
}

// SetPresolveEnvelope attaches a verified presolve envelope (Orig -> Reduced) to
// the trace. It may be called either before solving Reduced or after a trace on
// Reduced has been finalized.
func (tr *VIPRTrace) SetPresolveEnvelope(env *VIPRPresolveEnvelope) {
	if tr == nil {
		return
	}
	tr.presolveEnv = env
	if tr.hasCert && env != nil && env.Reduced == tr.tracedProb {
		tr.attachPresolveEnvelope(tr.tracedProb)
	}
}

// PresolveEnvelope returns the attached VIPRPresolveEnvelope, or nil if none.
func (tr *VIPRTrace) PresolveEnvelope() *VIPRPresolveEnvelope {
	if tr == nil {
		return nil
	}
	return tr.presolveEnv
}

// PresolveStepCount returns the number of certified presolve steps in the envelope.
func (tr *VIPRTrace) PresolveStepCount() int {
	if tr == nil || tr.presolveEnv == nil {
		return 0
	}
	return len(tr.presolveEnv.Steps)
}

// OrbitBranchesCertified returns the count of exact-verified orbital branches recorded.
func (tr *VIPRTrace) OrbitBranchesCertified() int {
	if tr == nil {
		return 0
	}
	return tr.orbitBranchesCertified
}

// TracedProblem returns the CompiledProblem certified by this trace.
func (tr *VIPRTrace) TracedProblem() *CompiledProblem {
	if tr == nil {
		return nil
	}
	if tr.presolveEnv != nil && tr.presolveEnv.Orig != nil && tr.presolveEnv.Reduced == tr.tracedProb {
		if !tr.hasCert || tr.cert.Orig != nil {
			return tr.presolveEnv.Orig
		}
	}
	return tr.tracedProb
}

// PresolveInfeasible reports whether presolve proved an empty feasible set before Branch-and-Bound.
func (tr *VIPRTrace) PresolveInfeasible() bool {
	return tr != nil && tr.presolveInfeasible
}

// HasCertificate reports whether a complete Mini-VIPR certificate is available.
func (tr *VIPRTrace) HasCertificate() bool {
	return tr != nil && tr.hasCert
}

func (tr *VIPRTrace) hasCertificate() bool {
	return tr.HasCertificate()
}

func (tr *VIPRTrace) newBabRecorder(p *CompiledProblem, disableSIMD bool) babVIPRHook {
	return newBabVIPRRecorder(tr, p, disableSIMD)
}

func (tr *VIPRTrace) presolveMIP(
	ctx context.Context,
	p *CompiledProblem,
	backend Backend,
	disableSIMD bool,
	start, deadline time.Time,
	done <-chan struct{},
) (*CompiledProblem, Result, bool) {
	vPre := presolveForVIPRWithDeadline(p, defaultPresolveMaxPass, deadline, done)
	if ctx != nil && ctx.Err() != nil {
		return p, Result{}, true
	}
	if vPre.TimedOut {
		return p, Result{
			modelID:   p.modelID,
			Backend:   backend,
			Status:    StatusTimeLimit,
			BestBound: trivialDualBound(p.sense),
			SolveTime: time.Since(start),
		}, true
	}
	if vPre.Infeasible {
		if !tr.RecordPresolveInfeasibleEnvelope(vPre) {
			tr.recordPresolveInfeasible(p, disableSIMD)
		}
		return p, Result{
			modelID:   p.modelID,
			Backend:   backend,
			Status:    StatusInfeasible,
			SolveTime: time.Since(start),
		}, true
	}
	if vPre.Reduced != nil && vPre.Reduced.invalidErr == nil && vPre.Envelope != nil && len(vPre.Envelope.Steps) > 0 {
		tr.SetPresolveEnvelope(vPre.Envelope)
		return vPre.Reduced, Result{}, false
	}
	return p, Result{}, false
}

// Mode returns the Mini-VIPR certificate mode ("infeasible", "optimal", "bound", or "").
func (tr *VIPRTrace) Mode() string {
	if tr == nil || !tr.hasCert {
		return ""
	}
	return string(tr.cert.Mode)
}

// TargetLB returns a copy of the certified lower bound in shifted minimized form
// (including ObjConstant), or nil when infeasible or uncertified.
func (tr *VIPRTrace) TargetLB() *big.Rat {
	if tr == nil || tr.targetLB == nil {
		return nil
	}
	return new(big.Rat).Set(tr.targetLB)
}

// StepCount returns the number of emitted Mini-VIPR proof steps.
func (tr *VIPRTrace) StepCount() int {
	if tr == nil || !tr.hasCert {
		return 0
	}
	return len(tr.cert.Steps)
}

// MarshalVIPRJSON serializes the Mini-VIPR certificate JSON accepted by vipr-check.
func (tr *VIPRTrace) MarshalVIPRJSON() ([]byte, error) {
	if tr == nil {
		return nil, errVIPRNilTrace
	}
	if !tr.hasCert {
		return nil, errVIPRNoCertificate
	}
	return marshalVIPRCertJSON(tr.cert), nil
}

type viprRatProblem struct {
	p        *CompiledProblem
	n        int
	m        int
	objConst *big.Rat
	objMin   []*big.Rat
	isInt    []bool
	lb       []*big.Rat
	ub       []*big.Rat
	rowSense []ConstraintSense
	rowRHS   []*big.Rat
	rowCols  [][]int
	rowVals  [][]*big.Rat
	colRows  [][]int
	colVals  [][]*big.Rat
}

func (rp *viprRatProblem) ensureCols() {
	if rp.colRows != nil {
		return
	}
	colRows := make([][]int, rp.n)
	colVals := make([][]*big.Rat, rp.n)
	for i := range rp.m {
		cols := rp.rowCols[i]
		vals := rp.rowVals[i]
		for k, col := range cols {
			if col >= 0 && col < rp.n && vals[k] != nil && vals[k].Sign() != 0 {
				colRows[col] = append(colRows[col], i)
				colVals[col] = append(colVals[col], vals[k])
			}
		}
	}
	rp.colRows = colRows
	rp.colVals = colVals
}

func buildVIPRRatProblem(p *CompiledProblem) (*viprRatProblem, bool) {
	if p == nil {
		return nil, false
	}
	n := p.numVars
	m := p.numConstraints
	constMin := p.objConstMin()
	objConst, ok := exactRatFromFloat(constMin)
	if !ok {
		return nil, false
	}
	objMinVec, okObj := exactMinObjVecRat(p)
	if !okObj {
		return nil, false
	}
	rp := &viprRatProblem{
		p:        p,
		n:        n,
		m:        m,
		objConst: objConst,
		objMin:   objMinVec,
		isInt:    make([]bool, n),
		lb:       make([]*big.Rat, n),
		ub:       make([]*big.Rat, n),
		rowSense: make([]ConstraintSense, m),
		rowRHS:   make([]*big.Rat, m),
		rowCols:  make([][]int, m),
		rowVals:  make([][]*big.Rat, m),
	}
	for j := range n {
		rp.isInt[j] = p.varCat[j] != Continuous
		lo, hi := p.varLB[j], p.varUB[j]
		if !math.IsInf(lo, -1) {
			lr, okL := exactRatFromFloat(lo)
			if !okL {
				return nil, false
			}
			rp.lb[j] = lr
		}
		if !math.IsInf(hi, 1) {
			ur, okU := exactRatFromFloat(hi)
			if !okU {
				return nil, false
			}
			rp.ub[j] = ur
		}
	}
	for i := range m {
		rp.rowSense[i] = p.rowSense[i]
		rhs, okR := exactRowRHSRat(p, i)
		if !okR {
			return nil, false
		}
		rp.rowRHS[i] = rhs
		cols, vals, okC := exactCanonicalRowTermsRat(p, i)
		if !okC {
			return nil, false
		}
		rp.rowCols[i] = cols
		rp.rowVals[i] = vals
	}
	return rp, true
}

func (rp *viprRatProblem) buildBaseCert(mode viprMode) viprCertJSON {
	senseStr := func(s ConstraintSense) string {
		switch s {
		case LessEqual:
			return "<="
		case GreaterEqual:
			return ">="
		case Equal:
			return "="
		default:
			return "<="
		}
	}
	objPay := make([]viprRatPayload, rp.n)
	lbPay := make([]*viprRatPayload, rp.n)
	ubPay := make([]*viprRatPayload, rp.n)
	for j := range rp.n {
		objPay[j] = viprBigRatToPayload(rp.objMin[j])
		if rp.lb[j] != nil {
			lp := viprBigRatToPayload(rp.lb[j])
			lbPay[j] = &lp
		}
		if rp.ub[j] != nil {
			up := viprBigRatToPayload(rp.ub[j])
			ubPay[j] = &up
		}
	}
	rows := make([]viprLinConJSON, rp.m)
	zeroPay := viprInt64Payload(0)
	for i := range rp.m {
		coeffs := make([]viprRatPayload, rp.n)
		for j := range rp.n {
			coeffs[j] = zeroPay
		}
		for k, col := range rp.rowCols[i] {
			coeffs[col] = viprBigRatToPayload(rp.rowVals[i][k])
		}
		rows[i] = viprLinConJSON{
			Coeffs: coeffs,
			Sense:  senseStr(rp.rowSense[i]),
			RHS:    viprBigRatToPayload(rp.rowRHS[i]),
		}
	}
	return viprCertJSON{
		N:        rp.n,
		ObjConst: viprBigRatToPayload(rp.objConst),
		Obj:      objPay,
		IsInt:    slices.Clone(rp.isInt),
		LB:       lbPay,
		UB:       ubPay,
		BaseRows: rows,
		Mode:     mode,
	}
}

func (rp *viprRatProblem) buildInstanceJSON() viprInstanceJSON {
	c := rp.buildBaseCert("")
	return viprInstanceJSON{
		N:        c.N,
		ObjConst: c.ObjConst,
		Obj:      c.Obj,
		IsInt:    c.IsInt,
		LB:       c.LB,
		UB:       c.UB,
		BaseRows: c.BaseRows,
	}
}

func marshalVIPRPresolveStep(s VIPRPresolveStep) viprPresolveStepJSON {
	out := viprPresolveStepJSON{Op: s.Op}
	switch s.Op {
	case "roundBound":
		out.Col = viprIntPtr(s.Col)
		out.IsUpper = viprBoolPtr(s.IsUpper)
		out.NewBound = viprRatPtr(s.NewBound)
	case "activityBound":
		out.Row = viprIntPtr(s.Row)
		out.Col = viprIntPtr(s.Col)
		out.IsUpper = viprBoolPtr(s.IsUpper)
		out.RoundInt = viprBoolPtr(s.RoundInt)
		out.NewBound = viprRatPtr(s.NewBound)
	case "gcdRowDiv":
		out.Row = viprIntPtr(s.Row)
		out.Divisor = viprInt64Ptr(s.Divisor)
	case "substFixedRow":
		out.Row = viprIntPtr(s.Row)
		out.Col = viprIntPtr(s.Col)
		out.Val = viprRatPtr(s.Val)
	case "substFixedObj":
		out.Col = viprIntPtr(s.Col)
		out.Val = viprRatPtr(s.Val)
	case "singletonRowElim":
		out.Row = viprIntPtr(s.Row)
		out.Col = viprIntPtr(s.Col)
		out.RoundInt = viprBoolPtr(s.RoundInt)
	case "redundantRow":
		out.Row = viprIntPtr(s.Row)
	case "binStrengthen":
		out.Row = viprIntPtr(s.Row)
		out.Col = viprIntPtr(s.Col)
		out.Delta = viprRatPtr(s.Delta)
	case "symBreakLe":
		out.U = viprIntPtr(s.U)
		out.V = viprIntPtr(s.V)
		out.ColPerm = slices.Clone(s.ColPerm)
		out.RowPerm = slices.Clone(s.RowPerm)
	}
	return out
}

func (tr *VIPRTrace) attachPresolveEnvelope(p *CompiledProblem) {
	if tr == nil || tr.presolveEnv == nil || tr.presolveEnv.Orig == nil || tr.presolveEnv.Reduced != p {
		return
	}
	origRP, ok := buildVIPRRatProblem(tr.presolveEnv.Orig)
	if !ok {
		return
	}
	inst := origRP.buildInstanceJSON()
	tr.cert.Orig = &inst
	if len(tr.presolveEnv.Steps) > 0 {
		steps := make([]viprPresolveStepJSON, len(tr.presolveEnv.Steps))
		for i, st := range tr.presolveEnv.Steps {
			steps[i] = marshalVIPRPresolveStep(st)
		}
		tr.cert.Presolve = steps
	} else {
		tr.cert.Presolve = nil
	}
}

// RecordPresolveInfeasibleEnvelope records a complete presolve-enveloped
// infeasibility certificate from an infeasible VIPRPresolveResult.
func (tr *VIPRTrace) RecordPresolveInfeasibleEnvelope(res VIPRPresolveResult) bool {
	if tr == nil || !res.Infeasible || res.Envelope == nil || res.Reduced == nil {
		return false
	}
	tr.presolveEnv = res.Envelope
	tr.reset(res.Reduced)
	tr.presolveInfeasible = true
	rp, ok := buildVIPRRatProblem(res.Reduced)
	if !ok {
		return false
	}

	var steps []viprStepJSON
	rootStep := 0
	if res.InfeasConflictCol >= 0 && res.InfeasConflictCol < rp.n {
		col := res.InfeasConflictCol
		if rp.lb[col] != nil && rp.ub[col] != nil && rp.ub[col].Cmp(rp.lb[col]) < 0 {
			steps = []viprStepJSON{{Rule: viprRuleBoundConflict, Col: viprIntPtr(col)}}
		}
	}
	if len(steps) == 0 && res.InfeasRow >= 0 && res.InfeasRow < rp.m {
		row := res.InfeasRow
		cols := rp.rowCols[row]
		vals := rp.rowVals[row]
		if len(cols) == 0 {
			steps = []viprStepJSON{{Rule: viprRuleConIndex, Row: viprIntPtr(row)}}
		} else if res.InfeasGCDDivisor > 0 {
			if gcdSteps, gcdRoot, okGCD := buildGCDInfeasSteps(rp); okGCD {
				steps = gcdSteps
				rootStep = gcdRoot
			}
		} else {
			// Activity-infeasible row: derive 0 <= rhs - minAct < 0 (or maxAct - rhs < 0) via farkas.
			minAct := new(big.Rat)
			maxAct := new(big.Rat)
			minFin, maxFin := true, true
			var prod big.Rat
			for k, col := range cols {
				a := vals[k]
				if a.Sign() > 0 {
					if rp.lb[col] == nil {
						minFin = false
					} else {
						minAct.Add(minAct, prod.Mul(a, rp.lb[col]))
					}
					if rp.ub[col] == nil {
						maxFin = false
					} else {
						maxAct.Add(maxAct, prod.Mul(a, rp.ub[col]))
					}
				} else if a.Sign() < 0 {
					if rp.ub[col] == nil {
						minFin = false
					} else {
						minAct.Add(minAct, prod.Mul(a, rp.ub[col]))
					}
					if rp.lb[col] == nil {
						maxFin = false
					} else {
						maxAct.Add(maxAct, prod.Mul(a, rp.lb[col]))
					}
				}
			}
			var rowMult int64
			if (rp.rowSense[row] == LessEqual || rp.rowSense[row] == Equal) && minFin && minAct.Cmp(rp.rowRHS[row]) > 0 {
				rowMult = 1
			} else if (rp.rowSense[row] == GreaterEqual || rp.rowSense[row] == Equal) && maxFin && maxAct.Cmp(rp.rowRHS[row]) < 0 {
				rowMult = -1
			}
			if rowMult != 0 {
				mRat := big.NewRat(rowMult, 1)
				bounds := make([]viprBoundRefJSON, 0, len(cols))
				for k, col := range cols {
					effA := new(big.Rat).Mul(vals[k], mRat)
					if effA.Sign() == 0 {
						continue
					}
					negA := new(big.Rat).Neg(effA)
					kind := "lb"
					if effA.Sign() < 0 {
						kind = "ub"
					}
					bounds = append(bounds, viprBoundRefJSON{
						Col:  col,
						Kind: kind,
						Mult: viprBigRatToPayload(negA),
					})
				}
				steps = []viprStepJSON{{
					Rule:   viprRuleFarkas,
					Refs:   viprOptRefs([]viprLinRefJSON{{ID: row, Mult: viprInt64Payload(rowMult)}}),
					Bounds: bounds,
				}}
			}
		}
	}
	if len(steps) == 0 {
		if gcdSteps, gcdRoot, okGCD := buildGCDInfeasSteps(rp); okGCD {
			steps = gcdSteps
			rootStep = gcdRoot
		}
	}
	if len(steps) == 0 {
		return false
	}
	cert := rp.buildBaseCert(viprModeInfeasible)
	cert.Steps = steps
	cert.RootStep = rootStep
	tr.cert = cert
	tr.attachPresolveEnvelope(res.Reduced)
	tr.hasCert = true
	return true
}

func viprRatFloor(r *big.Rat) *big.Rat {
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() < 0 {
		q.Sub(q, big.NewInt(1))
	}
	return new(big.Rat).SetInt(q)
}

func viprRatCeil(r *big.Rat) *big.Rat {
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return new(big.Rat).SetInt(q)
}

func viprRatToOutwardFloat(r *big.Rat, isUpper bool) float64 {
	if r == nil {
		if isUpper {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}
	f, exact := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		if isUpper {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}
	if exact {
		return f
	}
	fr := new(big.Rat).SetFloat64(f)
	if fr == nil {
		if isUpper {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}
	if isUpper {
		if fr.Cmp(r) < 0 {
			f = math.Nextafter(f, math.Inf(1))
		}
	} else if fr.Cmp(r) > 0 {
		f = math.Nextafter(f, math.Inf(-1))
	}
	return f
}

func solveVIPRRectRat(mat [][]*big.Rat, rhs []*big.Rat, nCols int) ([]*big.Rat, bool) {
	nRows := len(rhs)
	if nCols == 0 {
		return nil, true
	}
	if nRows < nCols || len(mat) != nRows || nRows > viprMaxNucleusSize {
		return nil, false
	}
	rows := make([][]*big.Rat, nRows)
	b := make([]*big.Rat, nRows)
	for i := range nRows {
		if len(mat[i]) != nCols {
			return nil, false
		}
		row := make([]*big.Rat, nCols)
		for j := range nCols {
			row[j] = new(big.Rat).Set(mat[i][j])
		}
		rows[i] = row
		b[i] = new(big.Rat).Set(rhs[i])
	}
	factor := new(big.Rat)
	term := new(big.Rat)
	// Reduce rows into upper-triangular echelon form.
	for col := range nCols {
		piv := -1
		for r := col; r < nRows; r++ {
			if rows[r][col].Sign() != 0 {
				piv = r
				break
			}
		}
		if piv < 0 {
			return nil, false
		}
		if piv != col {
			rows[col], rows[piv] = rows[piv], rows[col]
			b[col], b[piv] = b[piv], b[col]
		}
		pivVal := rows[col][col]
		for r := col + 1; r < nRows; r++ {
			if rows[r][col].Sign() == 0 {
				continue
			}
			factor.Quo(rows[r][col], pivVal)
			rows[r][col].SetInt64(0)
			for c := col + 1; c < nCols; c++ {
				if rows[col][c].Sign() != 0 {
					term.Mul(factor, rows[col][c])
					rows[r][c].Sub(rows[r][c], term)
				}
			}
			if b[col].Sign() != 0 {
				term.Mul(factor, b[col])
				b[r].Sub(b[r], term)
			}
		}
	}
	// Back-solve the upper-triangular system.
	sol := make([]*big.Rat, nCols)
	for col := nCols - 1; col >= 0; col-- {
		acc := new(big.Rat).Set(b[col])
		for c := col + 1; c < nCols; c++ {
			if rows[col][c].Sign() != 0 && sol[c].Sign() != 0 {
				term.Mul(rows[col][c], sol[c])
				acc.Sub(acc, term)
			}
		}
		sol[col] = new(big.Rat).Quo(acc, rows[col][col])
	}
	return sol, true
}

func solveVIPRDenseRat(mat [][]*big.Rat, rhs []*big.Rat) ([]*big.Rat, bool) {
	return solveVIPRRectRat(mat, rhs, len(rhs))
}

type viprBasisTerm struct {
	col int
	val *big.Rat
}

type viprBackStep struct {
	eq  int
	col int
}

type viprSparsePeeler struct {
	eqTerms     [][]viprBasisTerm
	eqRHS       []*big.Rat
	colEqs      [][]int
	eqActive    []bool
	eqDeg       []int
	varDeg      []int
	varKnown    []bool
	varDeferred []bool
	w           []*big.Rat
	backStack   []viprBackStep
	rowQ        []int
	colQ        []int
	tmp         big.Rat
	acc         big.Rat
}

func (sp *viprSparsePeeler) peelRow(q int) bool {
	if !sp.eqActive[q] || sp.eqDeg[q] != 1 {
		return true
	}
	targetCol := -1
	var targetVal *big.Rat
	sp.acc.Set(sp.eqRHS[q])
	for _, ent := range sp.eqTerms[q] {
		switch {
		case sp.varKnown[ent.col]:
			if sp.w[ent.col].Sign() != 0 {
				sp.tmp.Mul(ent.val, sp.w[ent.col])
				sp.acc.Sub(&sp.acc, &sp.tmp)
			}
		case !sp.varDeferred[ent.col]:
			targetCol = ent.col
			targetVal = ent.val
		}
	}
	if targetCol < 0 || targetVal.Sign() == 0 {
		return false
	}
	sp.w[targetCol].Quo(&sp.acc, targetVal)
	sp.varKnown[targetCol] = true
	sp.eqActive[q] = false
	for _, q2 := range sp.colEqs[targetCol] {
		if sp.eqActive[q2] {
			sp.eqDeg[q2]--
			if sp.eqDeg[q2] == 1 {
				sp.rowQ = append(sp.rowQ, q2)
			}
		}
	}
	return true
}

func (sp *viprSparsePeeler) peelCol(c int) bool {
	if sp.varKnown[c] || sp.varDeferred[c] || sp.varDeg[c] != 1 {
		return true
	}
	actEq := -1
	for _, q := range sp.colEqs[c] {
		if sp.eqActive[q] {
			actEq = q
			break
		}
	}
	if actEq < 0 {
		return false
	}
	sp.backStack = append(sp.backStack, viprBackStep{eq: actEq, col: c})
	sp.varDeferred[c] = true
	sp.eqActive[actEq] = false
	for _, ent := range sp.eqTerms[actEq] {
		if !sp.varKnown[ent.col] && !sp.varDeferred[ent.col] {
			sp.varDeg[ent.col]--
			if sp.varDeg[ent.col] == 1 {
				sp.colQ = append(sp.colQ, ent.col)
			}
		}
	}
	return true
}

func solveVIPRPeelSystem(m int, eqTerms [][]viprBasisTerm, eqRHS []*big.Rat) ([]*big.Rat, bool) {
	if m == 0 {
		return nil, true
	}
	if len(eqTerms) < m || len(eqRHS) < m {
		return nil, false
	}

	sp := viprSparsePeeler{
		eqTerms:     eqTerms[:m],
		eqRHS:       eqRHS[:m],
		colEqs:      make([][]int, m),
		eqActive:    make([]bool, m),
		eqDeg:       make([]int, m),
		varDeg:      make([]int, m),
		varKnown:    make([]bool, m),
		varDeferred: make([]bool, m),
		w:           make([]*big.Rat, m),
	}
	for i := range m {
		sp.w[i] = new(big.Rat)
	}

	for q := range m {
		if len(sp.eqTerms[q]) == 0 {
			return nil, false
		}
		sp.eqActive[q] = true
		sp.eqDeg[q] = len(sp.eqTerms[q])
		for _, ent := range sp.eqTerms[q] {
			sp.colEqs[ent.col] = append(sp.colEqs[ent.col], q)
			sp.varDeg[ent.col]++
		}
	}

	for q := range m {
		if sp.eqDeg[q] == 1 {
			sp.rowQ = append(sp.rowQ, q)
		}
		if sp.varDeg[q] == 0 {
			return nil, false
		}
		if sp.varDeg[q] == 1 {
			sp.colQ = append(sp.colQ, q)
		}
	}

	for len(sp.rowQ) > 0 || len(sp.colQ) > 0 {
		for len(sp.rowQ) > 0 {
			q := sp.rowQ[len(sp.rowQ)-1]
			sp.rowQ = sp.rowQ[:len(sp.rowQ)-1]
			if !sp.peelRow(q) {
				return nil, false
			}
		}
		for len(sp.colQ) > 0 && len(sp.rowQ) == 0 {
			c := sp.colQ[len(sp.colQ)-1]
			sp.colQ = sp.colQ[:len(sp.colQ)-1]
			if !sp.peelCol(c) {
				return nil, false
			}
		}
	}

	var nucEqs, nucVars []int
	nucVarPos := make([]int, m)
	for i := range m {
		nucVarPos[i] = -1
		if sp.eqActive[i] {
			nucEqs = append(nucEqs, i)
		}
		if !sp.varKnown[i] && !sp.varDeferred[i] {
			nucVarPos[i] = len(nucVars)
			nucVars = append(nucVars, i)
		}
	}
	nNuc := len(nucVars)
	if len(nucEqs) != nNuc || nNuc > viprMaxNucleusSize {
		return nil, false
	}
	if nNuc > 0 {
		mat := make([][]*big.Rat, nNuc)
		rhsNuc := make([]*big.Rat, nNuc)
		for rowIdx, q := range nucEqs {
			row := make([]*big.Rat, nNuc)
			for k := range nNuc {
				row[k] = new(big.Rat)
			}
			rhsR := new(big.Rat).Set(eqRHS[q])
			for _, ent := range sp.eqTerms[q] {
				if sp.varKnown[ent.col] {
					if sp.w[ent.col].Sign() != 0 {
						sp.tmp.Mul(ent.val, sp.w[ent.col])
						rhsR.Sub(rhsR, &sp.tmp)
					}
				} else if pos := nucVarPos[ent.col]; pos >= 0 {
					row[pos].Set(ent.val)
				} else {
					return nil, false
				}
			}
			mat[rowIdx] = row
			rhsNuc[rowIdx] = rhsR
		}
		solNuc, ok := solveVIPRDenseRat(mat, rhsNuc)
		if !ok {
			return nil, false
		}
		for k, origCol := range nucVars {
			sp.w[origCol].Set(solNuc[k])
			sp.varKnown[origCol] = true
		}
	}

	for _, bs := range slices.Backward(sp.backStack) {
		sp.acc.Set(eqRHS[bs.eq])
		var pivVal *big.Rat
		for _, ent := range sp.eqTerms[bs.eq] {
			if ent.col == bs.col {
				pivVal = ent.val
			} else if sp.varKnown[ent.col] {
				if sp.w[ent.col].Sign() != 0 {
					sp.tmp.Mul(ent.val, sp.w[ent.col])
					sp.acc.Sub(&sp.acc, &sp.tmp)
				}
			} else {
				return nil, false
			}
		}
		if pivVal == nil || pivVal.Sign() == 0 {
			return nil, false
		}
		sp.w[bs.col].Quo(&sp.acc, pivVal)
		sp.varKnown[bs.col] = true
	}
	return sp.w, true
}

// solveVIPRBasisTranspose solves the unscaled basis transpose system B^T y = eqRHS
// in exact rational arithmetic by peeling singleton rows and columns before factoring
// any remaining nucleus.
func solveVIPRBasisTranspose(rp *viprRatProblem, basis []int, eqRHS []*big.Rat) ([]*big.Rat, bool) {
	n := rp.n
	m := rp.m
	if m == 0 {
		return nil, true
	}
	if len(basis) < m || len(eqRHS) < m {
		return nil, false
	}
	rp.ensureCols()

	eqTerms := make([][]viprBasisTerm, m)
	for q := range m {
		bVar := basis[q]
		if bVar >= 0 && bVar < n {
			rows := rp.colRows[bVar]
			vals := rp.colVals[bVar]
			ents := make([]viprBasisTerm, len(rows))
			for k, ri := range rows {
				ents[k] = viprBasisTerm{col: ri, val: new(big.Rat).Set(vals[k])}
			}
			eqTerms[q] = ents
		} else {
			si := bVar - n
			if si < 0 || si >= m {
				return nil, false
			}
			eqTerms[q] = []viprBasisTerm{{col: si, val: big.NewRat(1, 1)}}
		}
	}
	return solveVIPRPeelSystem(m, eqTerms, eqRHS)
}

// projectVIPRFarkas projects wRaw into the Mini-VIPR Farkas cone (target <=:
// LessEqual => w_i >= 0, GreaterEqual => w_i <= 0) and checks strict gap < 0.
func (rp *viprRatProblem) projectVIPRFarkas(wRaw []*big.Rat, nodeLB, nodeUB []*big.Rat) ([]*big.Rat, bool) {
	if len(wRaw) < rp.m {
		return nil, false
	}
	wProj := make([]*big.Rat, rp.m)
	anyNZ := false
	for i := range rp.m {
		wi := new(big.Rat)
		if wRaw[i] != nil {
			wi.Set(wRaw[i])
		}
		if rp.rowSense[i] == LessEqual && wi.Sign() < 0 {
			wi.SetInt64(0)
		}
		if rp.rowSense[i] == GreaterEqual && wi.Sign() > 0 {
			wi.SetInt64(0)
		}
		if wi.Sign() != 0 {
			anyNZ = true
		}
		wProj[i] = wi
	}
	if !anyNZ {
		return nil, false
	}
	v := make([]*big.Rat, rp.n)
	for j := range rp.n {
		v[j] = new(big.Rat)
	}
	gap := new(big.Rat)
	term := new(big.Rat)
	for i := range rp.m {
		if wProj[i].Sign() == 0 {
			continue
		}
		term.Mul(wProj[i], rp.rowRHS[i])
		gap.Add(gap, term)
		for k, col := range rp.rowCols[i] {
			term.Mul(wProj[i], rp.rowVals[i][k])
			v[col].Add(v[col], term)
		}
	}
	for j := range rp.n {
		if v[j].Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, false
			}
			term.Mul(v[j], nodeLB[j])
			gap.Sub(gap, term)
		} else if v[j].Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, false
			}
			term.Mul(v[j], nodeUB[j])
			gap.Sub(gap, term)
		}
	}
	if gap.Sign() >= 0 {
		return nil, false
	}
	return wProj, true
}

func extractFarkasFromBasis[T any](
	s *LPSolver,
	m int,
	tryFloatBTRAN func([]float64) (T, bool),
	tryExactBasis func([]float64, bool) (T, bool),
) (T, bool) {
	var zero T
	if s == nil || s.ws == nil || s.TotalRows() != m {
		return zero, false
	}
	d := make([]float64, m)
	if s.lastDualPhase1Used && !s.lastPrimalFallback && s.lastInfeasRow >= 0 && s.lastInfeasRow < m {
		sgn := -1.0
		if s.lastInfeasDir < 0 {
			sgn = 1.0
		}
		d[s.lastInfeasRow] = sgn
		if w, ok := tryFloatBTRAN(d); ok {
			return w, true
		}
		if w, ok := tryExactBasis(d, false); ok {
			return w, true
		}
		if w, ok := tryExactBasis(d, true); ok {
			return w, true
		}
		d[s.lastInfeasRow] = 0
	}
	if s.lastPrimalFallback {
		for q := range m {
			bVar := s.ws.basis[q]
			if bVar >= 0 && bVar < len(s.ws.workCost) {
				d[q] = s.ws.workCost[bVar]
			}
		}
		if w, ok := tryFloatBTRAN(d); ok {
			return w, true
		}
		if w, ok := tryExactBasis(d, true); ok {
			return w, true
		}
	}
	return zero, false
}

func extractFarkasFallback[T any](
	s *LPSolver,
	m int,
	tryFloatBTRAN func([]float64) (T, bool),
	tryExactBasis func([]float64, bool) (T, bool),
) (T, bool) {
	var zero T
	if s == nil || s.ws == nil || s.TotalRows() != m {
		return zero, false
	}
	d := make([]float64, m)
	for q := range m {
		d[q] = 1.0
		if w, ok := tryFloatBTRAN(d); ok {
			return w, true
		}
		if w, ok := tryExactBasis(d, false); ok {
			return w, true
		}
		d[q] = 0
	}
	for q := range m {
		bVar := s.ws.basis[q]
		if bVar >= 0 && bVar < len(s.ws.workCost) {
			d[q] = s.ws.workCost[bVar]
		}
	}
	if w, ok := tryFloatBTRAN(d); ok {
		return w, true
	}
	if w, ok := tryExactBasis(d, true); ok {
		return w, true
	}
	return zero, false
}

func (rp *viprRatProblem) extractNodeFarkas(s *LPSolver, lpDuals []float64, nodeLB, nodeUB []*big.Rat) ([]*big.Rat, bool) {
	m := rp.m
	if m == 0 {
		return nil, false
	}

	tryDualSigns := func(raw []*big.Rat) ([]*big.Rat, bool) {
		if w, ok := rp.projectVIPRFarkas(raw, nodeLB, nodeUB); ok {
			return w, true
		}
		neg := make([]*big.Rat, m)
		for i := range m {
			neg[i] = new(big.Rat).Neg(raw[i])
		}
		if w, ok := rp.projectVIPRFarkas(neg, nodeLB, nodeUB); ok {
			return w, true
		}
		return nil, false
	}

	tryFloatBTRAN := func(d []float64) ([]*big.Rat, bool) {
		if s == nil || s.ws == nil || !s.luValid || s.TotalRows() != m || len(d) < m {
			return nil, false
		}
		rho := slices.Clone(d[:m])
		s.ws.lu.BTRANExact(rho)
		w := make([]*big.Rat, m)
		for i := range m {
			r, ok := exactRatFromFloat(rho[i] * s.rowScaleOf(i))
			if !ok {
				return nil, false
			}
			w[i] = r
		}
		return tryDualSigns(w)
	}

	tryExactBasis := func(d []float64, useColScale bool) ([]*big.Rat, bool) {
		if s == nil || s.ws == nil || s.TotalRows() != m || len(d) < m {
			return nil, false
		}
		eqRHS := make([]*big.Rat, m)
		for q := range m {
			r := new(big.Rat)
			if d[q] != 0 {
				val := d[q]
				if useColScale && s.scaled {
					bVar := s.ws.basis[q]
					if bVar >= 0 && bVar < rp.n {
						val *= s.invColScale[bVar]
					} else if si := bVar - rp.n; si >= 0 && si < m {
						val *= s.rowScaleOf(si)
					}
				}
				rv, ok := exactRatFromFloat(val)
				if !ok {
					return nil, false
				}
				r = rv
			}
			eqRHS[q] = r
		}
		wRaw, ok := solveVIPRBasisTranspose(rp, s.ws.basis[:m], eqRHS)
		if !ok {
			return nil, false
		}
		return tryDualSigns(wRaw)
	}

	if w, ok := extractFarkasFromBasis(s, m, tryFloatBTRAN, tryExactBasis); ok {
		return w, true
	}

	if len(lpDuals) == m {
		wFloat := make([]*big.Rat, m)
		allOK := true
		for i := range m {
			r, ok := exactRatFromFloat(lpDuals[i])
			if !ok {
				allOK = false
				break
			}
			wFloat[i] = r
		}
		if allOK {
			if w, ok := tryDualSigns(wFloat); ok {
				return w, true
			}
		}
	}

	if w, ok := extractFarkasFallback(s, m, tryFloatBTRAN, tryExactBasis); ok {
		return w, true
	}

	return nil, false
}

func (rp *viprRatProblem) evalObjNoConst(x []*big.Rat) *big.Rat {
	sum := new(big.Rat)
	term := new(big.Rat)
	for j := range rp.n {
		if rp.objMin[j].Sign() == 0 || x[j].Sign() == 0 {
			continue
		}
		term.Mul(rp.objMin[j], x[j])
		sum.Add(sum, term)
	}
	return sum
}

func (rp *viprRatProblem) checkExactFeasible(x []*big.Rat) bool {
	if len(x) != rp.n {
		return false
	}
	for j := range rp.n {
		if x[j] == nil {
			return false
		}
		if rp.isInt[j] && !x[j].IsInt() {
			return false
		}
		if rp.lb[j] != nil && x[j].Cmp(rp.lb[j]) < 0 {
			return false
		}
		if rp.ub[j] != nil && x[j].Cmp(rp.ub[j]) > 0 {
			return false
		}
	}
	lhs := new(big.Rat)
	term := new(big.Rat)
	for i := range rp.m {
		lhs.SetInt64(0)
		for k, col := range rp.rowCols[i] {
			if x[col].Sign() == 0 {
				continue
			}
			term.Mul(rp.rowVals[i][k], x[col])
			lhs.Add(lhs, term)
		}
		ord := lhs.Cmp(rp.rowRHS[i])
		switch rp.rowSense[i] {
		case LessEqual:
			if ord > 0 {
				return false
			}
		case GreaterEqual:
			if ord < 0 {
				return false
			}
		case Equal:
			if ord != 0 {
				return false
			}
		}
	}
	return true
}

func (rp *viprRatProblem) refineExactIncumbent(approx []float64, scratchLP func() *LPSolver) ([]*big.Rat, *big.Rat, bool) {
	if len(approx) != rp.n {
		return nil, nil, false
	}
	xRat := make([]*big.Rat, rp.n)
	hasCont := false
	for j := range rp.n {
		if rp.isInt[j] {
			rv := math.Round(approx[j])
			r, ok := exactRatFromFloat(rv)
			if !ok || !r.IsInt() {
				return nil, nil, false
			}
			xRat[j] = r
		} else {
			hasCont = true
			r, ok := exactRatFromFloat(approx[j])
			if !ok {
				return nil, nil, false
			}
			xRat[j] = r
		}
	}
	unrefinedOK := rp.checkExactFeasible(xRat)
	if !hasCont {
		if !unrefinedOK {
			return nil, nil, false
		}
		return xRat, rp.evalObjNoConst(xRat), true
	}

	var fallbackX []*big.Rat
	var fallbackObj *big.Rat
	if unrefinedOK {
		fallbackX = make([]*big.Rat, rp.n)
		for j := range rp.n {
			fallbackX[j] = new(big.Rat).Set(xRat[j])
		}
		fallbackObj = rp.evalObjNoConst(fallbackX)
	}
	returnFallback := func() ([]*big.Rat, *big.Rat, bool) {
		if fallbackX != nil {
			return fallbackX, fallbackObj, true
		}
		return nil, nil, false
	}

	fixedLB := make([]float64, rp.n)
	fixedUB := make([]float64, rp.n)
	for j := range rp.n {
		if rp.isInt[j] {
			rv := math.Round(approx[j])
			fixedLB[j] = rv
			fixedUB[j] = rv
		} else {
			fixedLB[j], fixedUB[j] = rp.p.varLB[j], rp.p.varUB[j]
		}
	}
	s := scratchLP()
	if s == nil {
		return returnFallback()
	}
	s.SetStructuralBounds(fixedLB, fixedUB)
	lpRes := s.Solve()
	if lpRes.Status != StatusOptimal || s.TotalRows() != rp.m {
		return returnFallback()
	}

	isBasicStruct := make([]bool, rp.n)
	var basicContCols []int
	isBasicSlack := make([]bool, rp.m)
	for q := range rp.m {
		bv := s.ws.basis[q]
		if bv >= 0 && bv < rp.n {
			isBasicStruct[bv] = true
			if !rp.isInt[bv] {
				basicContCols = append(basicContCols, bv)
			}
		} else if si := bv - rp.n; si >= 0 && si < rp.m {
			isBasicSlack[si] = true
		}
	}
	var activeRows []int
	for i := range rp.m {
		if !isBasicSlack[i] {
			activeRows = append(activeRows, i)
		}
	}

	for j := range rp.n {
		if rp.isInt[j] || isBasicStruct[j] {
			continue
		}
		lo, hi := rp.p.varLB[j], rp.p.varUB[j]
		var chosen *big.Rat
		var val float64
		switch {
		case !math.IsInf(lo, -1) && math.Abs(lpRes.Primal[j]-lo) <= 1e-7:
			if rp.lb[j] != nil {
				chosen = new(big.Rat).Set(rp.lb[j])
			} else {
				val = lo
			}
		case !math.IsInf(hi, 1) && math.Abs(lpRes.Primal[j]-hi) <= 1e-7:
			if rp.ub[j] != nil {
				chosen = new(big.Rat).Set(rp.ub[j])
			} else {
				val = hi
			}
		case !math.IsInf(lo, -1) && lpRes.ReducedCosts[j] >= 0:
			if rp.lb[j] != nil {
				chosen = new(big.Rat).Set(rp.lb[j])
			} else {
				val = lo
			}
		case !math.IsInf(hi, 1) && lpRes.ReducedCosts[j] <= 0:
			if rp.ub[j] != nil {
				chosen = new(big.Rat).Set(rp.ub[j])
			} else {
				val = hi
			}
		default:
			val = lpRes.Primal[j]
		}
		if chosen == nil {
			r, ok := exactRatFromFloat(val)
			if !ok {
				return returnFallback()
			}
			chosen = r
		}
		xRat[j] = chosen
	}

	if xSol, ok := rp.solveContRowsRect(xRat, basicContCols, activeRows); ok {
		xRat = xSol
	}

	if !rp.checkExactFeasible(xRat) {
		if repaired, okRep := rp.repairContinuousExactIncumbent(xRat, basicContCols, isBasicStruct, activeRows); okRep {
			xRat = repaired
		} else {
			return returnFallback()
		}
	}
	refinedObj := rp.evalObjNoConst(xRat)
	if fallbackX != nil && fallbackObj.Cmp(refinedObj) <= 0 {
		return fallbackX, fallbackObj, true
	}
	return xRat, refinedObj, true
}

func (rp *viprRatProblem) solveContRowsRect(xBase []*big.Rat, freeCols, eqRows []int) ([]*big.Rat, bool) {
	kMat := len(freeCols)
	if kMat == 0 || len(eqRows) < kMat || len(eqRows) > viprMaxNucleusSize {
		return nil, false
	}
	colPos := make([]int, rp.n)
	for j := range rp.n {
		colPos[j] = -1
	}
	for pos, col := range freeCols {
		colPos[col] = pos
	}
	mat := make([][]*big.Rat, len(eqRows))
	rhs := make([]*big.Rat, len(eqRows))
	term := new(big.Rat)
	for rIdx, rowID := range eqRows {
		row := make([]*big.Rat, kMat)
		for c := range kMat {
			row[c] = new(big.Rat)
		}
		rRHS := new(big.Rat).Set(rp.rowRHS[rowID])
		for idx, col := range rp.rowCols[rowID] {
			coeff := rp.rowVals[rowID][idx]
			if pPos := colPos[col]; pPos >= 0 {
				row[pPos].Add(row[pPos], coeff)
			} else {
				term.Mul(coeff, xBase[col])
				rRHS.Sub(rRHS, term)
			}
		}
		mat[rIdx] = row
		rhs[rIdx] = rRHS
	}
	solB, ok := solveVIPRRectRat(mat, rhs, kMat)
	if !ok {
		return nil, false
	}
	xOut := make([]*big.Rat, rp.n)
	for j := range rp.n {
		xOut[j] = new(big.Rat).Set(xBase[j])
	}
	for pos, col := range freeCols {
		xOut[col] = solB[pos]
	}
	return xOut, true
}

// repairContinuousExactIncumbent recovers exact primal feasibility when 1-ULP outward
// presolved bounds or nonbasic continuous pins leave xRat slightly infeasible.
func (rp *viprRatProblem) repairContinuousExactIncumbent(
	xRat []*big.Rat,
	basicContCols []int,
	isBasicStruct []bool,
	activeRows []int,
) ([]*big.Rat, bool) {
	// Reject immediately if any integer coordinate itself violates integer bounds.
	for j := range rp.n {
		if !rp.isInt[j] {
			continue
		}
		if xRat[j] == nil || !xRat[j].IsInt() {
			return nil, false
		}
		if rp.lb[j] != nil && xRat[j].Cmp(rp.lb[j]) < 0 {
			return nil, false
		}
		if rp.ub[j] != nil && xRat[j].Cmp(rp.ub[j]) > 0 {
			return nil, false
		}
	}

	var violRows []int
	inViol := make([]bool, rp.m)
	act := new(big.Rat)
	term := new(big.Rat)
	for i := range rp.m {
		act.SetInt64(0)
		for idx, col := range rp.rowCols[i] {
			term.Mul(rp.rowVals[i][idx], xRat[col])
			act.Add(act, term)
		}
		ord := act.Cmp(rp.rowRHS[i])
		bad := (rp.rowSense[i] == LessEqual && ord > 0) ||
			(rp.rowSense[i] == GreaterEqual && ord < 0) ||
			(rp.rowSense[i] == Equal && ord != 0)
		if bad {
			violRows = append(violRows, i)
			inViol[i] = true
		}
	}

	isOOB := make([]bool, rp.n)
	oobClamp := make([]*big.Rat, rp.n)
	var oobCols []int
	for j := range rp.n {
		if rp.isInt[j] {
			continue
		}
		if rp.lb[j] != nil && xRat[j].Cmp(rp.lb[j]) < 0 {
			isOOB[j] = true
			oobClamp[j] = rp.lb[j]
			oobCols = append(oobCols, j)
		} else if rp.ub[j] != nil && xRat[j].Cmp(rp.ub[j]) > 0 {
			isOOB[j] = true
			oobClamp[j] = rp.ub[j]
			oobCols = append(oobCols, j)
		}
	}

	// Build candidate row subsets (active + violated, and equality + violated).
	dedupRows := func(groups ...[]int) []int {
		seen := make([]bool, rp.m)
		var out []int
		for _, g := range groups {
			for _, id := range g {
				if id >= 0 && id < rp.m && !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
		return out
	}
	var equalActive []int
	for _, id := range activeRows {
		if rp.rowSense[id] == Equal {
			equalActive = append(equalActive, id)
		}
	}
	rowSets := make([][]int, 0, 3)
	augRows := dedupRows(activeRows, violRows)
	if len(augRows) > 0 && len(augRows) <= viprMaxNucleusSize {
		rowSets = append(rowSets, augRows)
	}
	eqViolRows := dedupRows(equalActive, violRows)
	if len(eqViolRows) > 0 && len(eqViolRows) <= viprMaxNucleusSize && len(eqViolRows) != len(augRows) {
		rowSets = append(rowSets, eqViolRows)
	}
	if len(violRows) > 0 && len(violRows) <= viprMaxNucleusSize && len(violRows) != len(eqViolRows) {
		rowSets = append(rowSets, violRows)
	}

	// Rank nonbasic continuous variables: violated rows first, then oob-coupled rows, then active rows.
	inCand := make([]bool, rp.n)
	var candPromote []int
	addRowContCands := func(rowID int) {
		for _, col := range rp.rowCols[rowID] {
			if !rp.isInt[col] && !isBasicStruct[col] && !inCand[col] {
				inCand[col] = true
				candPromote = append(candPromote, col)
			}
		}
	}
	for _, rID := range violRows {
		addRowContCands(rID)
	}
	for i := range rp.m {
		touchesOOB := false
		for _, col := range rp.rowCols[i] {
			if isOOB[col] {
				touchesOOB = true
				break
			}
		}
		if touchesOOB {
			addRowContCands(i)
		}
	}
	for _, rID := range activeRows {
		addRowContCands(rID)
	}
	const maxPromote = 24
	if len(candPromote) > maxPromote {
		candPromote = candPromote[:maxPromote]
	}

	xClamped := make([]*big.Rat, rp.n)
	for j := range rp.n {
		if oobClamp[j] != nil {
			xClamped[j] = new(big.Rat).Set(oobClamp[j])
		} else {
			xClamped[j] = new(big.Rat).Set(xRat[j])
		}
	}
	if len(oobCols) > 0 && rp.checkExactFeasible(xClamped) {
		return xClamped, true
	}

	tryWithExtras := func(xBase []*big.Rat, baseFree []int) ([]*big.Rat, bool) {
		maxPair := min(len(candPromote), 12)
		for _, eqRows := range rowSets {
			if len(baseFree) > 0 && len(eqRows) >= len(baseFree) {
				if sol, ok := rp.solveContRowsRect(xBase, baseFree, eqRows); ok && rp.checkExactFeasible(sol) {
					return sol, true
				}
			}
			for _, c := range candPromote {
				free1 := append(slices.Clone(baseFree), c)
				if sol, ok := rp.solveContRowsRect(xBase, free1, eqRows); ok && rp.checkExactFeasible(sol) {
					return sol, true
				}
			}
			for i := range maxPair {
				for k := i + 1; k < maxPair; k++ {
					free2 := append(slices.Clone(baseFree), candPromote[i], candPromote[k])
					if sol, ok := rp.solveContRowsRect(xBase, free2, eqRows); ok && rp.checkExactFeasible(sol) {
						return sol, true
					}
				}
			}
		}
		return nil, false
	}

	if len(oobCols) > 0 {
		remFree := make([]int, 0, len(basicContCols))
		for _, col := range basicContCols {
			if !isOOB[col] {
				remFree = append(remFree, col)
			}
		}
		if sol, ok := tryWithExtras(xClamped, remFree); ok {
			return sol, true
		}
	}

	if len(violRows) > 0 {
		if sol, ok := tryWithExtras(xRat, basicContCols); ok {
			return sol, true
		}
	}

	// Single-column basis swap fallback on active/violated rows.
	maxOut := min(len(basicContCols), 12)
	maxIn := min(len(candPromote), 12)
	if maxOut > 0 && maxIn > 0 {
		for outIdx := range maxOut {
			swapped := make([]int, 0, len(basicContCols))
			for idx, col := range basicContCols {
				if idx != outIdx {
					swapped = append(swapped, col)
				}
			}
			for inIdx := range maxIn {
				cand := append(slices.Clone(swapped), candPromote[inIdx])
				for _, eqRows := range rowSets {
					if sol, ok := rp.solveContRowsRect(xRat, cand, eqRows); ok && rp.checkExactFeasible(sol) {
						return sol, true
					}
				}
			}
		}
	}
	return nil, false
}

type viprNodeKind uint8

const (
	viprNodeOpen viprNodeKind = iota
	viprNodeBranch
	viprNodeOrbitBranch
	viprNodeLeafBoundConflict
	viprNodeLeafFarkas
	viprNodeLeafDual
)

type viprOrbitWitness struct {
	companion int
	colPerm   []int
	rowPerm   []int
}

type viprPropStep struct {
	rowIdx   int // -1 denotes integer bound rounding from an existing bound.
	col      int
	isUpper  bool
	roundInt bool
	bound    *big.Rat
}

type viprRowMult struct {
	rowID int      // < rp.m => base row; >= rp.m => allCuts[rowID - rp.m]
	mult  *big.Rat // exact rational multiplier
}

type viprBoundTerm struct {
	col     int
	isUpper bool
	mult    *big.Rat
}

type viprCutProofKind uint8

const (
	viprCutLinComb viprCutProofKind = iota
	viprCutCG
	viprCutVUBBranch
)

type viprGlobalCut struct {
	cols          []int
	vals          []*big.Rat
	rhs           *big.Rat
	isGe          bool
	normExp       int
	lpCols        []int
	lpVals        []float64
	lpRHS         float64
	kind          viprCutProofKind
	refs          []viprRowMult
	bounds        []viprBoundTerm
	vubCol        int
	vubBinCol     int
	vubRowID      int
	vubInvA       *big.Rat
	vubDelta      *big.Rat
	vubUB         *big.Rat
	vubLeftBounds []viprBoundTerm
}

type viprCertifiedCut struct {
	plane cutPlane
	cut   viprGlobalCut
}

type viprTreeNode struct {
	parent         *viprTreeNode
	left           *viprTreeNode
	right          *viprTreeNode
	propSteps      []viprPropStep
	bestDual       []viprRowMult
	bestDualRHS    *big.Rat
	farkas         []viprRowMult
	asmBound       *big.Int
	splitVal       *big.Int
	orbitAsmCols   []int
	orbitWitnesses []viprOrbitWitness
	asmCol         int
	branchCol      int
	conflictCol    int
	kind           viprNodeKind
	hasAsm         bool
	asmIsUpper     bool
}

type babVIPRRecorder struct {
	trace                  *VIPRTrace
	rp                     *viprRatProblem
	root                   *viprTreeNode
	baseScratchLP          *LPSolver
	cutScratchLP           *LPSolver
	scratchCutIDs          []int
	allCuts                []viprGlobalCut
	roundedBaseRow         []int
	stagedCuts             []viprCertifiedCut
	activeCutIDs           []int
	cachedIncPtr           *float64
	cachedIncObjMin        float64
	cachedIncRat           []*big.Rat
	cachedIncObj           *big.Rat
	cachedIncOK            bool
	disableSIMD            bool
	invalid                bool
	deadline               time.Time
	done                   <-chan struct{}
	rootCutsCertified      int
	inTreeCutsCertified    int
	rcfFixingsCertified    int
	unbTightCertified      int
	orbitBranchesCertified int
}

func (r *babVIPRRecorder) setDeadline(deadline time.Time, done <-chan struct{}) {
	if r == nil {
		return
	}
	r.deadline = deadline
	r.done = done
}

func newBabVIPRRecorder(tr *VIPRTrace, p *CompiledProblem, disableSIMD bool) *babVIPRRecorder {
	tr.reset(p)
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return &babVIPRRecorder{trace: tr, invalid: true}
	}
	rec := &babVIPRRecorder{
		trace:       tr,
		rp:          rp,
		root:        &viprTreeNode{kind: viprNodeOpen},
		disableSIMD: disableSIMD,
	}
	if eo := p.exactOrig; eo != nil && len(eo.rows) == rp.m {
		var rounded []int
		for i := range rp.m {
			if !eo.rows[i].intTightened {
				continue
			}
			if rounded == nil {
				rounded = make([]int, rp.m)
				for j := range rounded {
					rounded[j] = -1
				}
			}
			tightRHS, okT := exactRatFromFloat(p.rowRHS[i])
			if !okT {
				return &babVIPRRecorder{trace: tr, invalid: true}
			}
			cutID := len(rec.allCuts)
			rec.allCuts = append(rec.allCuts, viprGlobalCut{
				cols: slices.Clone(rp.rowCols[i]),
				vals: cloneRatSlice(rp.rowVals[i]),
				rhs:  tightRHS,
				isGe: rp.rowSense[i] == GreaterEqual,
				kind: viprCutCG,
				refs: []viprRowMult{{rowID: i, mult: big.NewRat(1, 1)}},
			})
			rounded[i] = cutID
		}
		rec.roundedBaseRow = rounded
	}
	return rec
}

func (r *babVIPRRecorder) close() {
	if r == nil {
		return
	}
	if r.baseScratchLP != nil {
		r.baseScratchLP.Close()
		r.baseScratchLP = nil
	}
	if r.cutScratchLP != nil {
		r.cutScratchLP.Close()
		r.cutScratchLP = nil
		r.scratchCutIDs = nil
	}
}

func (r *babVIPRRecorder) rootNode() *viprTreeNode {
	if r == nil {
		return nil
	}
	return r.root
}

func (r *babVIPRRecorder) recordRootInfeasibleEqRow(p *CompiledProblem) {
	if r == nil || r.trace == nil {
		return
	}
	r.trace.recordRootInfeasibleEqRow(p)
}

func (r *babVIPRRecorder) newDisabledOrbitSubTrace() viprTraceHook {
	tr := NewVIPRTrace()
	tr.disableOrbit = true
	return tr
}

func (r *babVIPRRecorder) adoptSubTraceInfeas(p *CompiledProblem, sub viprTraceHook) {
	if r == nil || r.trace == nil {
		return
	}
	if subTr, ok := sub.(*VIPRTrace); ok && subTr != nil {
		r.trace.adoptInfeasSteps(p, subTr)
	}
}

func (r *babVIPRRecorder) stageCertifiedCutRound(
	s *LPSolver,
	lpPrimal []float64,
	maxCuts int,
	minEff float64,
	ws *cutGenWorkspace,
) bool {
	if r == nil {
		return false
	}
	r.stagedCuts = r.generateCertifiedCutRound(s, lpPrimal, maxCuts, minEff, ws)
	return len(r.stagedCuts) > 0
}

func (r *babVIPRRecorder) commitStagedCutPlanes(lpSolver *LPSolver, isRoot bool) int {
	if r == nil {
		return 0
	}
	cuts := r.stagedCuts
	r.stagedCuts = nil
	return r.addCertifiedCutPlanes(lpSolver, cuts, isRoot)
}

func (r *babVIPRRecorder) createChildNodes(
	p *CompiledProblem,
	sym *nodeSymmetry,
	deadline time.Time,
	done <-chan struct{},
	req branchNodeReq,
	parentVN *viprTreeNode,
) (downNode, upNode *searchNode) {
	downNode, upNode = createChildNodes(
		req.parentLB, req.parentDepth+1, req.parent, req.brVar, req.brVal,
		req.curLB[req.brVar], req.curUB[req.brVar], req.pc, req.counter,
	)
	fl := math.Floor(downNode.delta.ub)
	downNode.delta.ub = fl
	upNode.delta.lb = fl + 1.0
	if r != nil && (r.trace == nil || !r.trace.disableOrbit) && p.varCat[req.brVar] == Binary && req.curLB[req.brVar] == 0 && req.curUB[req.brVar] == 1 && fl == 0 {
		cands := sym.activeOrbitWithWitnesses(p, req.curLB, req.curUB, req.brVar, req.parentDepth, deadline, done)
		if verified := r.filterVerifiedOrbitCompanions(parentVN, req.brVar, cands); len(verified) > 0 {
			downNode.base = slices.Clip(downNode.base)
			for _, w := range verified {
				downNode.base = append(downNode.base, boundDelta{col: w.companion, lb: 0, ub: 0})
			}
			downNode.orbitalDown = true
			downNode.viprNode, upNode.viprNode = r.recordOrbitBranch(parentVN, req.brVar, verified)
			return downNode, upNode
		}
	}
	if r != nil {
		if splitInt, ok := viprFloorBigInt(downNode.delta.ub); !ok {
			r.invalid = true
		} else {
			downNode.viprNode, upNode.viprNode = r.recordBranch(parentVN, req.brVar, splitInt)
		}
	}
	return downNode, upNode
}

func (r *babVIPRRecorder) ensureBaseScratchLP() *LPSolver {
	if r == nil || r.rp == nil {
		return nil
	}
	if r.baseScratchLP == nil {
		s := NewLPSolver(r.rp.p, nil)
		s.DisableSIMD = r.disableSIMD
		r.baseScratchLP = s
	}
	r.baseScratchLP.Deadline = r.deadline
	r.baseScratchLP.Done = r.done
	return r.baseScratchLP
}

func (r *babVIPRRecorder) ensureCutScratchLP() *LPSolver {
	if r == nil || r.rp == nil {
		return nil
	}
	if len(r.activeCutIDs) == 0 {
		return r.ensureBaseScratchLP()
	}
	needRebuild := r.cutScratchLP == nil || len(r.scratchCutIDs) > len(r.activeCutIDs)
	if !needRebuild {
		for i, id := range r.scratchCutIDs {
			if r.activeCutIDs[i] != id {
				needRebuild = true
				break
			}
		}
	}
	if needRebuild {
		if r.cutScratchLP != nil {
			r.cutScratchLP.Close()
		}
		s := NewLPSolver(r.rp.p, nil)
		s.DisableSIMD = r.disableSIMD
		r.cutScratchLP = s
		r.scratchCutIDs = r.scratchCutIDs[:0]
	}
	r.cutScratchLP.Deadline = r.deadline
	r.cutScratchLP.Done = r.done
	if len(r.scratchCutIDs) < len(r.activeCutIDs) {
		add := make([]dynamicRow, 0, len(r.activeCutIDs)-len(r.scratchCutIDs))
		for _, cutID := range r.activeCutIDs[len(r.scratchCutIDs):] {
			c := &r.allCuts[cutID]
			dr, st := r.cutScratchLP.buildCutRow(c.lpCols, c.lpVals, c.lpRHS)
			if st != cutBuildOK {
				return nil
			}
			add = append(add, dr)
		}
		r.cutScratchLP.addCutRows(add)
		r.scratchCutIDs = append(r.scratchCutIDs, r.activeCutIDs[len(r.scratchCutIDs):]...)
	}
	return r.cutScratchLP
}

func (r *babVIPRRecorder) rowData(rowID int) (ConstraintSense, *big.Rat, []int, []*big.Rat) {
	if rowID < r.rp.m {
		return r.rp.rowSense[rowID], r.rp.rowRHS[rowID], r.rp.rowCols[rowID], r.rp.rowVals[rowID]
	}
	c := &r.allCuts[rowID-r.rp.m]
	if c.isGe {
		return GreaterEqual, c.rhs, c.cols, c.vals
	}
	return LessEqual, c.rhs, c.cols, c.vals
}

func (r *babVIPRRecorder) activeRowID(activeIdx int) int {
	if activeIdx < r.rp.m {
		if r.roundedBaseRow != nil && activeIdx >= 0 && r.roundedBaseRow[activeIdx] >= 0 {
			return r.rp.m + r.roundedBaseRow[activeIdx]
		}
		return activeIdx
	}
	return r.rp.m + r.activeCutIDs[activeIdx-r.rp.m]
}

func (r *babVIPRRecorder) activeRowLPScale(activeIdx int) *big.Rat {
	if activeIdx < r.rp.m {
		return big.NewRat(1, 1)
	}
	c := &r.allCuts[r.activeCutIDs[activeIdx-r.rp.m]]
	return viprPow2Rat(-c.normExp)
}

func (r *babVIPRRecorder) snapshotActiveCuts() int {
	if r == nil {
		return 0
	}
	return len(r.activeCutIDs)
}

func (r *babVIPRRecorder) restoreActiveCuts(n int) {
	if r == nil || n < 0 || n > len(r.activeCutIDs) {
		return
	}
	r.activeCutIDs = r.activeCutIDs[:n]
}

func (r *babVIPRRecorder) addCertifiedCutPlanes(lpSolver *LPSolver, cuts []viprCertifiedCut, isRoot bool) int {
	if r == nil || r.invalid || len(cuts) == 0 {
		return 0
	}
	rows := make([]dynamicRow, 0, len(cuts))
	accepted := make([]viprGlobalCut, 0, len(cuts))
	for _, cc := range cuts {
		dr, st := lpSolver.buildCutRow(cc.plane.ColIdx, cc.plane.Val, cc.plane.RHS)
		if st != cutBuildOK {
			continue
		}
		gc := cc.cut
		gc.lpCols = cc.plane.ColIdx
		gc.lpVals = cc.plane.Val
		gc.lpRHS = cc.plane.RHS
		rows = append(rows, dr)
		accepted = append(accepted, gc)
	}
	if len(rows) == 0 {
		return 0
	}
	lpSolver.addCutRows(rows)
	for _, gc := range accepted {
		cutID := len(r.allCuts)
		r.allCuts = append(r.allCuts, gc)
		r.activeCutIDs = append(r.activeCutIDs, cutID)
	}
	if isRoot {
		r.rootCutsCertified += len(accepted)
	} else {
		r.inTreeCutsCertified += len(accepted)
	}
	return len(rows)
}

func (r *babVIPRRecorder) purgeSlackCuts(s *LPSolver, slackTol float64) int {
	if s == nil || len(s.extraRows) == 0 {
		return 0
	}
	if r == nil || r.invalid || !s.luValid || s.ws == nil {
		return s.PurgeSlackCuts(slackTol)
	}
	p := s.prob
	n := p.numVars
	baseM := p.numConstraints
	oldExtra := len(s.extraRows)
	if len(r.activeCutIDs) != oldExtra {
		r.invalid = true
		return s.PurgeSlackCuts(slackTol)
	}
	oldM := baseM + oldExtra
	ws := s.ws
	kept := make([]int, 0, oldExtra)
	expectedDrop := 0
	for e := range oldExtra {
		slackVar := n + baseM + e
		dropped := false
		if ws.status[slackVar] == statusBasic {
			bSlot := ws.basicRow[slackVar]
			thresh := slackTol * s.rowScaleOf(baseM+e)
			if bSlot >= 0 && bSlot < oldM && ws.basis[bSlot] == slackVar && ws.xb[bSlot] > thresh {
				dropped = true
				expectedDrop++
			}
		}
		if !dropped {
			kept = append(kept, r.activeCutIDs[e])
		}
	}
	actualDrop := s.PurgeSlackCuts(slackTol)
	if actualDrop != expectedDrop || len(s.extraRows) != len(kept) {
		r.invalid = true
		return actualDrop
	}
	r.activeCutIDs = kept
	return actualDrop
}

func (r *babVIPRRecorder) solveBasisTransposeActive(basis []int, eqRHS []*big.Rat) ([]*big.Rat, bool) {
	rp := r.rp
	n := rp.n
	baseM := rp.m
	numExtra := len(r.activeCutIDs)
	m := baseM + numExtra
	if m == 0 {
		return nil, true
	}
	if len(basis) < m || len(eqRHS) < m {
		return nil, false
	}
	rp.ensureCols()

	var cutColTerms [][]viprBasisTerm
	if numExtra > 0 {
		cutColTerms = make([][]viprBasisTerm, n)
		for e, cutID := range r.activeCutIDs {
			c := &r.allCuts[cutID]
			actRow := baseM + e
			for k, col := range c.cols {
				if col >= 0 && col < n && c.vals[k].Sign() != 0 {
					cutColTerms[col] = append(cutColTerms[col], viprBasisTerm{col: actRow, val: c.vals[k]})
				}
			}
		}
	}

	eqTerms := make([][]viprBasisTerm, m)
	for q := range m {
		bVar := basis[q]
		if bVar >= 0 && bVar < n {
			rows := rp.colRows[bVar]
			vals := rp.colVals[bVar]
			extraLen := 0
			if cutColTerms != nil {
				extraLen = len(cutColTerms[bVar])
			}
			ents := make([]viprBasisTerm, 0, len(rows)+extraLen)
			for k, ri := range rows {
				ents = append(ents, viprBasisTerm{col: ri, val: new(big.Rat).Set(vals[k])})
			}
			if extraLen > 0 {
				ents = append(ents, cutColTerms[bVar]...)
			}
			eqTerms[q] = ents
		} else {
			si := bVar - n
			if si < 0 || si >= m {
				return nil, false
			}
			eqTerms[q] = []viprBasisTerm{{col: si, val: big.NewRat(1, 1)}}
		}
	}
	return solveVIPRPeelSystem(m, eqTerms, eqRHS)
}

func (r *babVIPRRecorder) solveBasisForwardActive(basis []int, rhsRow []*big.Rat) ([]*big.Rat, bool) {
	rp := r.rp
	n := rp.n
	baseM := rp.m
	numExtra := len(r.activeCutIDs)
	m := baseM + numExtra
	if m == 0 {
		return nil, true
	}
	if len(basis) < m || len(rhsRow) < m {
		return nil, false
	}
	rp.ensureCols()

	var cutColTerms [][]viprBasisTerm
	if numExtra > 0 {
		cutColTerms = make([][]viprBasisTerm, n)
		for e, cutID := range r.activeCutIDs {
			c := &r.allCuts[cutID]
			actRow := baseM + e
			for k, col := range c.cols {
				if col >= 0 && col < n && c.vals[k].Sign() != 0 {
					cutColTerms[col] = append(cutColTerms[col], viprBasisTerm{col: actRow, val: c.vals[k]})
				}
			}
		}
	}

	eqTerms := make([][]viprBasisTerm, m)
	for q := range m {
		bVar := basis[q]
		if bVar >= 0 && bVar < n {
			rows := rp.colRows[bVar]
			vals := rp.colVals[bVar]
			for k, ri := range rows {
				eqTerms[ri] = append(eqTerms[ri], viprBasisTerm{col: q, val: new(big.Rat).Set(vals[k])})
			}
			if cutColTerms != nil {
				for _, ct := range cutColTerms[bVar] {
					eqTerms[ct.col] = append(eqTerms[ct.col], viprBasisTerm{col: q, val: new(big.Rat).Set(ct.val)})
				}
			}
		} else {
			si := bVar - n
			if si < 0 || si >= m {
				return nil, false
			}
			eqTerms[si] = append(eqTerms[si], viprBasisTerm{col: q, val: big.NewRat(1, 1)})
		}
	}
	return solveVIPRPeelSystem(m, eqTerms, rhsRow)
}

func (r *babVIPRRecorder) polishNodeDualExactSimplex(vn *viprTreeNode, targetObj *big.Rat) (closed, infeas bool, xExact []*big.Rat) {
	if r == nil || r.invalid || vn == nil || r.rp == nil || stopRequested(time.Time{}, r.done) {
		return false, false, nil
	}
	rp := r.rp
	lbR, ubR := r.nodeBoundsRat(vn)
	for j := range rp.n {
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return false, true, nil
		}
	}
	sc, lpRes, ok := r.solveCutScratchNode(lbR, ubR, true)
	if !ok {
		return false, false, nil
	}
	switch lpRes.Status {
	case StatusInfeasible:
		if w, okF := r.extractNodeFarkasActive(sc, lpRes.Dual, lbR, ubR); okF {
			vn.kind = viprNodeLeafFarkas
			vn.farkas = w
			return false, true, nil
		}
		return false, false, nil
	case StatusOptimal:
		r.recordNodeDual(vn, sc, lpRes)
		if targetObj != nil && vn.bestDualRHS != nil && targetObj.Cmp(vn.bestDualRHS) <= 0 {
			return true, false, nil
		}
	default:
		return false, false, nil
	}

	n := rp.n
	m := rp.m + len(r.activeCutIDs)
	if m == 0 || sc.ws == nil || sc.TotalRows() != m || len(sc.ws.basis) < m || len(sc.ws.status) < n+m {
		return false, false, nil
	}

	tot := n + m
	basis := slices.Clone(sc.ws.basis[:m])
	status := slices.Clone(sc.ws.status[:tot])

	varLB := make([]*big.Rat, tot)
	varUB := make([]*big.Rat, tot)
	copy(varLB[:n], lbR)
	copy(varUB[:n], ubR)

	rowRHS := make([]*big.Rat, m)
	zeroRat := new(big.Rat)
	for i := range m {
		sense, rhs, _, _ := r.rowData(r.activeRowID(i))
		rowRHS[i] = rhs
		switch sense {
		case LessEqual:
			varLB[n+i] = zeroRat
		case GreaterEqual:
			varUB[n+i] = zeroRat
		case Equal:
			varLB[n+i] = zeroRat
			varUB[n+i] = zeroRat
		}
	}

	rp.ensureCols()
	allColRows := make([][]int, n)
	allColVals := make([][]*big.Rat, n)
	for j := range n {
		allColRows[j] = append(allColRows[j], rp.colRows[j]...)
		allColVals[j] = append(allColVals[j], rp.colVals[j]...)
	}
	for e, cutID := range r.activeCutIDs {
		c := &r.allCuts[cutID]
		actRow := rp.m + e
		for k, col := range c.cols {
			if col >= 0 && col < n && c.vals[k].Sign() != 0 {
				allColRows[col] = append(allColRows[col], actRow)
				allColVals[col] = append(allColVals[col], c.vals[k])
			}
		}
	}

	const maxExactDualPivots = 32
	cB := make([]*big.Rat, m)
	rc := make([]*big.Rat, tot)
	rhsAdj := make([]*big.Rat, m)
	eQ := make([]*big.Rat, m)
	for i := range m {
		eQ[i] = new(big.Rat)
	}
	var prod, alphaK, candRatio, bestRatio big.Rat

	for range maxExactDualPivots {
		if stopRequested(time.Time{}, r.done) {
			break
		}
		for q := range m {
			bVar := basis[q]
			if bVar >= 0 && bVar < n {
				cB[q] = rp.objMin[bVar]
			} else {
				cB[q] = zeroRat
			}
		}
		y, okY := r.solveBasisTransposeActive(basis, cB)
		if !okY || len(y) != m {
			break
		}

		for j := range n {
			dot := new(big.Rat)
			rows := allColRows[j]
			vals := allColVals[j]
			for k, ri := range rows {
				if y[ri].Sign() != 0 {
					prod.Mul(vals[k], y[ri])
					dot.Add(dot, &prod)
				}
			}
			rc[j] = new(big.Rat).Sub(rp.objMin[j], dot)
		}
		for i := range m {
			rc[n+i] = new(big.Rat).Neg(y[i])
		}

		dualFeas := true
		for v := range tot {
			if status[v] == statusBasic {
				continue
			}
			if varLB[v] != nil && varUB[v] != nil && varLB[v].Cmp(varUB[v]) == 0 {
				status[v] = statusFixed
				continue
			}
			sgn := rc[v].Sign()
			switch {
			case sgn > 0:
				if varLB[v] != nil {
					status[v] = statusAtLower
				} else {
					dualFeas = false
				}
			case sgn < 0:
				if varUB[v] != nil {
					status[v] = statusAtUpper
				} else {
					dualFeas = false
				}
			default:
				if status[v] == statusAtLower && varLB[v] == nil {
					if varUB[v] != nil {
						status[v] = statusAtUpper
					} else {
						status[v] = statusSuperBasic
					}
				} else if status[v] == statusAtUpper && varUB[v] == nil {
					if varLB[v] != nil {
						status[v] = statusAtLower
					} else {
						status[v] = statusSuperBasic
					}
				}
			}
			if !dualFeas {
				break
			}
		}

		mults := r.clampActiveDualToMults(y)
		if rhs, _, okE := r.evalSafeDualMults(mults, lbR, ubR); okE {
			if vn.bestDualRHS == nil || rhs.Cmp(vn.bestDualRHS) > 0 {
				vn.bestDual = mults
				vn.bestDualRHS = rhs
				if vn.kind == viprNodeOpen {
					vn.kind = viprNodeLeafDual
				}
			}
			if targetObj != nil && vn.bestDualRHS != nil && targetObj.Cmp(vn.bestDualRHS) <= 0 {
				return true, false, nil
			}
		}
		if !dualFeas {
			break
		}

		for i := range m {
			rhsAdj[i] = new(big.Rat).Set(rowRHS[i])
		}
		for j := range n {
			if status[j] == statusBasic {
				continue
			}
			var xj *big.Rat
			switch status[j] {
			case statusAtLower, statusFixed:
				xj = varLB[j]
			case statusAtUpper:
				xj = varUB[j]
			}
			if xj == nil || xj.Sign() == 0 {
				continue
			}
			rows := allColRows[j]
			vals := allColVals[j]
			for k, ri := range rows {
				prod.Mul(vals[k], xj)
				rhsAdj[ri].Sub(rhsAdj[ri], &prod)
			}
		}

		xB, okX := r.solveBasisForwardActive(basis, rhsAdj)
		if !okX || len(xB) != m {
			break
		}

		leaveQ := -1
		leaveVar := tot
		leaveAtLower := false
		for q := range m {
			v := basis[q]
			if varLB[v] != nil && xB[q].Cmp(varLB[v]) < 0 {
				if v < leaveVar {
					leaveQ = q
					leaveVar = v
					leaveAtLower = true
				}
			} else if varUB[v] != nil && xB[q].Cmp(varUB[v]) > 0 {
				if v < leaveVar {
					leaveQ = q
					leaveVar = v
					leaveAtLower = false
				}
			}
		}
		if leaveQ < 0 {
			xExact = make([]*big.Rat, n)
			for j := range n {
				switch status[j] {
				case statusAtLower, statusFixed:
					if varLB[j] != nil {
						xExact[j] = new(big.Rat).Set(varLB[j])
					} else {
						xExact[j] = new(big.Rat)
					}
				case statusAtUpper:
					if varUB[j] != nil {
						xExact[j] = new(big.Rat).Set(varUB[j])
					} else {
						xExact[j] = new(big.Rat)
					}
				default:
					xExact[j] = new(big.Rat)
				}
			}
			for q := range m {
				if bVar := basis[q]; bVar >= 0 && bVar < n {
					xExact[bVar] = new(big.Rat).Set(xB[q])
				}
			}
			break
		}

		eQ[leaveQ].SetInt64(1)
		rho, okRho := r.solveBasisTransposeActive(basis, eQ)
		eQ[leaveQ].SetInt64(0)
		if !okRho || len(rho) != m {
			break
		}

		enterK := -1
		for k := range tot {
			st := status[k]
			if st == statusBasic || st == statusFixed {
				continue
			}
			if k < n {
				alphaK.SetInt64(0)
				rows := allColRows[k]
				vals := allColVals[k]
				for idx, ri := range rows {
					if rho[ri].Sign() != 0 {
						prod.Mul(vals[idx], rho[ri])
						alphaK.Add(&alphaK, &prod)
					}
				}
			} else {
				alphaK.Set(rho[k-n])
			}
			aSgn := alphaK.Sign()
			if aSgn == 0 {
				continue
			}
			blocks := false
			if st == statusSuperBasic {
				blocks = true
				candRatio.SetInt64(0)
			} else if leaveAtLower {
				if st == statusAtLower && aSgn < 0 {
					blocks = true
					candRatio.Quo(rc[k], &alphaK)
					candRatio.Neg(&candRatio)
				} else if st == statusAtUpper && aSgn > 0 {
					blocks = true
					candRatio.Quo(rc[k], &alphaK)
					candRatio.Neg(&candRatio)
				}
			} else {
				if st == statusAtLower && aSgn > 0 {
					blocks = true
					candRatio.Quo(rc[k], &alphaK)
				} else if st == statusAtUpper && aSgn < 0 {
					blocks = true
					candRatio.Quo(rc[k], &alphaK)
				}
			}
			if !blocks || candRatio.Sign() < 0 {
				continue
			}
			if enterK < 0 || candRatio.Cmp(&bestRatio) < 0 {
				enterK = k
				bestRatio.Set(&candRatio)
			}
		}

		if enterK < 0 {
			wCand := make([]viprRowMult, 0, m)
			for i := range m {
				if rho[i].Sign() == 0 {
					continue
				}
				mv := new(big.Rat).Set(rho[i])
				if leaveAtLower {
					mv.Neg(mv)
				}
				wCand = append(wCand, viprRowMult{rowID: r.activeRowID(i), mult: mv})
			}
			if w, _, okF := r.projectFarkasMults(wCand, lbR, ubR); okF {
				vn.kind = viprNodeLeafFarkas
				vn.farkas = w
				return false, true, nil
			}
			break
		}

		basis[leaveQ] = enterK
		status[enterK] = statusBasic
		if varLB[leaveVar] != nil && varUB[leaveVar] != nil && varLB[leaveVar].Cmp(varUB[leaveVar]) == 0 {
			status[leaveVar] = statusFixed
		} else if leaveAtLower {
			status[leaveVar] = statusAtLower
		} else {
			status[leaveVar] = statusAtUpper
		}
	}

	return targetObj != nil && vn.bestDualRHS != nil && targetObj.Cmp(vn.bestDualRHS) <= 0, false, xExact
}

func (r *babVIPRRecorder) closeNearOptimalLeafExact(vn *viprTreeNode, targetObj *big.Rat, depth int, budget *int) bool {
	if r == nil || r.invalid || vn == nil || targetObj == nil || stopRequested(time.Time{}, r.done) {
		return false
	}
	closed, infeas, xExact := r.polishNodeDualExactSimplex(vn, targetObj)
	if closed || infeas {
		return true
	}
	const maxMicroBranchDepth = 8
	if depth >= maxMicroBranchDepth || budget == nil || *budget <= 0 || len(xExact) != r.rp.n {
		return false
	}

	// Select the most fractional integer variable from the exact rational LP optimum,
	// capturing even sub-1e-15 fractionality ignored by floating-point integrality tolerances.
	bestCol := -1
	var bestDist *big.Rat
	halfRat := big.NewRat(1, 2)
	oneRat := big.NewRat(1, 1)
	for j := range r.rp.n {
		if !r.rp.isInt[j] || xExact[j] == nil || xExact[j].IsInt() {
			continue
		}
		fl := viprRatFloor(xExact[j])
		frac := new(big.Rat).Sub(xExact[j], fl)
		dist := frac
		if frac.Cmp(halfRat) > 0 {
			dist = new(big.Rat).Sub(oneRat, frac)
		}
		if bestCol < 0 || dist.Cmp(bestDist) > 0 {
			bestCol = j
			bestDist = dist
		}
	}
	if bestCol < 0 {
		return false
	}

	splitRat := viprRatFloor(xExact[bestCol])
	if !splitRat.IsInt() {
		return false
	}
	splitVal := new(big.Int).Set(splitRat.Num())

	oldKind := vn.kind
	oldLeft := vn.left
	oldRight := vn.right
	oldBrCol := vn.branchCol
	oldSplit := vn.splitVal

	*budget -= 2
	left, right := r.recordBranch(vn, bestCol, splitVal)
	if left == nil || right == nil {
		vn.kind = oldKind
		vn.left = oldLeft
		vn.right = oldRight
		vn.branchCol = oldBrCol
		vn.splitVal = oldSplit
		return false
	}

	okL := r.closeNearOptimalLeafExact(left, targetObj, depth+1, budget)
	okR := r.closeNearOptimalLeafExact(right, targetObj, depth+1, budget)
	if okL && okR {
		return true
	}

	childHasProof := func(c *viprTreeNode) bool {
		if c == nil {
			return false
		}
		return c.kind == viprNodeLeafBoundConflict || c.kind == viprNodeLeafFarkas ||
			c.kind == viprNodeBranch || c.kind == viprNodeOrbitBranch || c.bestDualRHS != nil
	}
	if !childHasProof(left) || !childHasProof(right) {
		vn.kind = oldKind
		vn.left = oldLeft
		vn.right = oldRight
		vn.branchCol = oldBrCol
		vn.splitVal = oldSplit
	}
	return false
}

func (r *babVIPRRecorder) polishNearClosedLeavesExact(targetObj *big.Rat) {
	if r == nil || r.invalid || r.root == nil || targetObj == nil {
		return
	}
	const maxPolishedLeaves = 32
	polished := 0
	budget := 64
	var walk func(vn *viprTreeNode, bestAncRHS *big.Rat)
	walk = func(vn *viprTreeNode, bestAncRHS *big.Rat) {
		if vn == nil || polished >= maxPolishedLeaves || stopRequested(time.Time{}, r.done) {
			return
		}
		curBest := bestAncRHS
		if vn.bestDualRHS != nil && (curBest == nil || vn.bestDualRHS.Cmp(curBest) > 0) {
			curBest = vn.bestDualRHS
		}
		if curBest != nil && targetObj.Cmp(curBest) <= 0 {
			return
		}
		switch vn.kind {
		case viprNodeLeafBoundConflict, viprNodeLeafFarkas:
			return
		case viprNodeLeafDual, viprNodeOpen:
			polished++
			r.closeNearOptimalLeafExact(vn, targetObj, 0, &budget)
		case viprNodeBranch, viprNodeOrbitBranch:
			walk(vn.left, curBest)
			walk(vn.right, curBest)
		}
	}
	walk(r.root, nil)
}

func (r *babVIPRRecorder) clampActiveDualToMults(yActive []*big.Rat) []viprRowMult {
	m := r.rp.m + len(r.activeCutIDs)
	var out []viprRowMult
	for i := range min(m, len(yActive)) {
		if yActive[i] == nil || yActive[i].Sign() == 0 {
			continue
		}
		gID := r.activeRowID(i)
		sense, _, _, _ := r.rowData(gID)
		val := new(big.Rat).Set(yActive[i])
		if sense == LessEqual && val.Sign() > 0 {
			continue
		}
		if sense == GreaterEqual && val.Sign() < 0 {
			continue
		}
		out = append(out, viprRowMult{rowID: gID, mult: val})
	}
	return out
}

func (r *babVIPRRecorder) evalSafeDualMults(mults []viprRowMult, nodeLB, nodeUB []*big.Rat) (*big.Rat, []*big.Rat, bool) {
	rp := r.rp
	colDots := make([]*big.Rat, rp.n)
	for j := range rp.n {
		colDots[j] = new(big.Rat)
	}
	rhs := new(big.Rat)
	term := new(big.Rat)
	for _, rm := range mults {
		if rm.mult == nil || rm.mult.Sign() == 0 {
			continue
		}
		if rm.rowID < 0 || rm.rowID >= rp.m+len(r.allCuts) {
			return nil, nil, false
		}
		sense, rowRHS, cols, vals := r.rowData(rm.rowID)
		if sense == LessEqual && rm.mult.Sign() > 0 {
			return nil, nil, false
		}
		if sense == GreaterEqual && rm.mult.Sign() < 0 {
			return nil, nil, false
		}
		term.Mul(rm.mult, rowRHS)
		rhs.Add(rhs, term)
		for k, col := range cols {
			term.Mul(rm.mult, vals[k])
			colDots[col].Add(colDots[col], term)
		}
	}
	rc := make([]*big.Rat, rp.n)
	for j := range rp.n {
		rj := new(big.Rat).Sub(rp.objMin[j], colDots[j])
		rc[j] = rj
		if rj.Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, nil, false
			}
			term.Mul(rj, nodeLB[j])
			rhs.Add(rhs, term)
		} else if rj.Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, nil, false
			}
			term.Mul(rj, nodeUB[j])
			rhs.Add(rhs, term)
		}
	}
	return rhs, rc, true
}

func (r *babVIPRRecorder) projectFarkasMults(wRaw []viprRowMult, nodeLB, nodeUB []*big.Rat) ([]viprRowMult, []*big.Rat, bool) {
	rp := r.rp
	wProj := make([]viprRowMult, 0, len(wRaw))
	for _, rm := range wRaw {
		if rm.mult == nil || rm.mult.Sign() == 0 || rm.rowID < 0 || rm.rowID >= rp.m+len(r.allCuts) {
			continue
		}
		sense, _, _, _ := r.rowData(rm.rowID)
		if sense == LessEqual && rm.mult.Sign() < 0 {
			continue
		}
		if sense == GreaterEqual && rm.mult.Sign() > 0 {
			continue
		}
		wProj = append(wProj, viprRowMult{rowID: rm.rowID, mult: new(big.Rat).Set(rm.mult)})
	}
	if len(wProj) == 0 {
		return nil, nil, false
	}
	v := make([]*big.Rat, rp.n)
	for j := range rp.n {
		v[j] = new(big.Rat)
	}
	gap := new(big.Rat)
	term := new(big.Rat)
	for _, rm := range wProj {
		_, rowRHS, cols, vals := r.rowData(rm.rowID)
		term.Mul(rm.mult, rowRHS)
		gap.Add(gap, term)
		for k, col := range cols {
			term.Mul(rm.mult, vals[k])
			v[col].Add(v[col], term)
		}
	}
	for j := range rp.n {
		if v[j].Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, nil, false
			}
			term.Mul(v[j], nodeLB[j])
			gap.Sub(gap, term)
		} else if v[j].Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, nil, false
			}
			term.Mul(v[j], nodeUB[j])
			gap.Sub(gap, term)
		}
	}
	if gap.Sign() >= 0 {
		return nil, nil, false
	}
	return wProj, v, true
}

func (r *babVIPRRecorder) extractNodeSafeDualActive(s *LPSolver, lpRes LPResult, nodeLB, nodeUB []*big.Rat) ([]viprRowMult, *big.Rat, bool) {
	rp := r.rp
	mActive := rp.m + len(r.activeCutIDs)
	var bestMults []viprRowMult
	var bestRHS *big.Rat

	tryCand := func(mults []viprRowMult) {
		rhs, _, ok := r.evalSafeDualMults(mults, nodeLB, nodeUB)
		if !ok {
			return
		}
		if bestRHS == nil || rhs.Cmp(bestRHS) > 0 {
			bestMults = mults
			bestRHS = rhs
		}
	}

	if s != nil && s.ws != nil && s.TotalRows() == mActive && mActive > 0 {
		rhsB := make([]*big.Rat, mActive)
		for q := range mActive {
			bVar := s.ws.basis[q]
			if bVar >= 0 && bVar < rp.n {
				rhsB[q] = new(big.Rat).Set(rp.objMin[bVar])
			} else {
				rhsB[q] = new(big.Rat)
			}
		}
		if yBasis, ok := r.solveBasisTransposeActive(s.ws.basis[:mActive], rhsB); ok {
			tryCand(r.clampActiveDualToMults(yBasis))
		}
	}

	if len(lpRes.Dual) == mActive {
		senseSign := 1.0
		if rp.p.sense == Maximize {
			senseSign = -1.0
		}
		yFloat := make([]*big.Rat, mActive)
		allOK := true
		for i := range mActive {
			rv, ok := exactRatFromFloat(senseSign * lpRes.Dual[i])
			if !ok {
				allOK = false
				break
			}
			if i >= rp.m && rv.Sign() != 0 {
				rv.Mul(rv, r.activeRowLPScale(i))
			}
			yFloat[i] = rv
		}
		if allOK {
			tryCand(r.clampActiveDualToMults(yFloat))
		}
	}

	if bestRHS == nil {
		tryCand(nil)
	}
	return bestMults, bestRHS, bestRHS != nil
}

func (r *babVIPRRecorder) extractNodeFarkasActive(s *LPSolver, lpDuals []float64, nodeLB, nodeUB []*big.Rat) ([]viprRowMult, bool) {
	rp := r.rp
	m := rp.m + len(r.activeCutIDs)
	if m == 0 {
		return nil, false
	}

	tryDualSigns := func(raw []*big.Rat) ([]viprRowMult, bool) {
		pos := make([]viprRowMult, 0, len(raw))
		neg := make([]viprRowMult, 0, len(raw))
		for i := range min(m, len(raw)) {
			if raw[i] == nil || raw[i].Sign() == 0 {
				continue
			}
			gID := r.activeRowID(i)
			pos = append(pos, viprRowMult{rowID: gID, mult: raw[i]})
			neg = append(neg, viprRowMult{rowID: gID, mult: new(big.Rat).Neg(raw[i])})
		}
		if w, _, ok := r.projectFarkasMults(pos, nodeLB, nodeUB); ok {
			return w, true
		}
		if w, _, ok := r.projectFarkasMults(neg, nodeLB, nodeUB); ok {
			return w, true
		}
		return nil, false
	}

	tryFloatBTRAN := func(d []float64) ([]viprRowMult, bool) {
		if s == nil || s.ws == nil || !s.luValid || s.TotalRows() != m || len(d) < m {
			return nil, false
		}
		rho := slices.Clone(d[:m])
		s.ws.lu.BTRANExact(rho)
		w := make([]*big.Rat, m)
		for i := range m {
			rv, ok := exactRatFromFloat(rho[i] * s.rowScaleOf(i))
			if !ok {
				return nil, false
			}
			if i >= rp.m && rv.Sign() != 0 {
				rv.Mul(rv, r.activeRowLPScale(i))
			}
			w[i] = rv
		}
		return tryDualSigns(w)
	}

	tryExactBasis := func(d []float64, useColScale bool) ([]viprRowMult, bool) {
		if s == nil || s.ws == nil || s.TotalRows() != m || len(d) < m {
			return nil, false
		}
		eqRHS := make([]*big.Rat, m)
		for q := range m {
			rv := new(big.Rat)
			if d[q] != 0 {
				val := d[q]
				bVar := s.ws.basis[q]
				if useColScale && s.scaled {
					if bVar >= 0 && bVar < rp.n {
						val *= s.invColScale[bVar]
					} else if si := bVar - rp.n; si >= 0 && si < m {
						val *= s.rowScaleOf(si)
					}
				}
				parsed, ok := exactRatFromFloat(val)
				if !ok {
					return nil, false
				}
				if si := bVar - rp.n; si >= rp.m && si < m {
					parsed.Mul(parsed, r.activeRowLPScale(si))
				}
				rv = parsed
			}
			eqRHS[q] = rv
		}
		wRaw, ok := r.solveBasisTransposeActive(s.ws.basis[:m], eqRHS)
		if !ok {
			return nil, false
		}
		return tryDualSigns(wRaw)
	}

	if w, ok := extractFarkasFromBasis(s, m, tryFloatBTRAN, tryExactBasis); ok {
		return w, true
	}

	if len(lpDuals) == m {
		wFloat := make([]*big.Rat, m)
		allOK := true
		for i := range m {
			rv, ok := exactRatFromFloat(lpDuals[i])
			if !ok {
				allOK = false
				break
			}
			if i >= rp.m && rv.Sign() != 0 {
				rv.Mul(rv, r.activeRowLPScale(i))
			}
			wFloat[i] = rv
		}
		if allOK {
			if w, ok := tryDualSigns(wFloat); ok {
				return w, true
			}
		}
	}

	if w, ok := extractFarkasFallback(s, m, tryFloatBTRAN, tryExactBasis); ok {
		return w, true
	}

	for i := range m {
		gID := r.activeRowID(i)
		for _, sgn := range []int64{1, -1} {
			if w, _, ok := r.projectFarkasMults([]viprRowMult{{rowID: gID, mult: big.NewRat(sgn, 1)}}, nodeLB, nodeUB); ok {
				return w, true
			}
		}
	}

	return nil, false
}

func (r *babVIPRRecorder) nodeBoundsRat(vn *viprTreeNode) ([]*big.Rat, []*big.Rat) {
	var chain []*viprTreeNode
	for cur := vn; cur != nil; cur = cur.parent {
		chain = append(chain, cur)
	}
	lb := make([]*big.Rat, r.rp.n)
	ub := make([]*big.Rat, r.rp.n)
	for j := range r.rp.n {
		if r.rp.lb[j] != nil {
			lb[j] = new(big.Rat).Set(r.rp.lb[j])
		}
		if r.rp.ub[j] != nil {
			ub[j] = new(big.Rat).Set(r.rp.ub[j])
		}
	}
	for _, cur := range slices.Backward(chain) {
		if cur.hasAsm && cur.asmBound != nil {
			bRat := new(big.Rat).SetInt(cur.asmBound)
			if cur.asmIsUpper {
				if ub[cur.asmCol] == nil || bRat.Cmp(ub[cur.asmCol]) < 0 {
					ub[cur.asmCol] = bRat
				}
			} else if lb[cur.asmCol] == nil || bRat.Cmp(lb[cur.asmCol]) > 0 {
				lb[cur.asmCol] = bRat
			}
		}
		for _, c := range cur.orbitAsmCols {
			zeroRat := new(big.Rat)
			if ub[c] == nil || zeroRat.Cmp(ub[c]) < 0 {
				ub[c] = zeroRat
			}
		}
		for _, ps := range cur.propSteps {
			if ps.isUpper {
				if ub[ps.col] == nil || ps.bound.Cmp(ub[ps.col]) < 0 {
					ub[ps.col] = new(big.Rat).Set(ps.bound)
				}
			} else if lb[ps.col] == nil || ps.bound.Cmp(lb[ps.col]) > 0 {
				lb[ps.col] = new(big.Rat).Set(ps.bound)
			}
		}
	}
	return lb, ub
}

func (r *babVIPRRecorder) fillNodeBoundsFloat(vn *viprTreeNode, dstLB, dstUB []float64) {
	if r == nil || r.invalid || vn == nil {
		return
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	for j := range r.rp.n {
		dstLB[j] = viprRatToOutwardFloat(lbR[j], false)
		dstUB[j] = viprRatToOutwardFloat(ubR[j], true)
	}
}

func (r *babVIPRRecorder) recordNodeDual(vn *viprTreeNode, s *LPSolver, lpRes LPResult) {
	if r == nil || r.invalid || vn == nil {
		return
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	mults, rhs, ok := r.extractNodeSafeDualActive(s, lpRes, lbR, ubR)
	if !ok {
		return
	}
	if vn.bestDualRHS == nil || rhs.Cmp(vn.bestDualRHS) > 0 {
		vn.bestDual = mults
		vn.bestDualRHS = rhs
	}
	if vn.kind == viprNodeOpen {
		vn.kind = viprNodeLeafDual
	}
}

func (r *babVIPRRecorder) exactDualSearchMinFloor(vn *viprTreeNode, searchShift float64) (float64, bool) {
	if r == nil || r.invalid || r.rp == nil || vn == nil || vn.bestDualRHS == nil {
		return 0, false
	}
	bound := vn.bestDualRHS
	if searchShift != 0 {
		bound = new(big.Rat).Add(r.rp.objConst, vn.bestDualRHS)
	}
	return exactRatToFloatFloor(bound), true
}

func (r *babVIPRRecorder) exactNodeGapClosed(vn *viprTreeNode, bestPrimal []float64) bool {
	if r == nil || r.invalid || !r.cachedIncOK || vn == nil || vn.bestDualRHS == nil || len(bestPrimal) == 0 {
		return false
	}
	if r.cachedIncPtr != &bestPrimal[0] || r.cachedIncObj == nil {
		return false
	}
	return r.cachedIncObj.Cmp(vn.bestDualRHS) <= 0
}

func (r *babVIPRRecorder) exactSubtreeGapClosed(vn *viprTreeNode, bestPrimal []float64) bool {
	if r == nil || r.invalid || !r.cachedIncOK || vn == nil || len(bestPrimal) == 0 {
		return false
	}
	if r.cachedIncPtr != &bestPrimal[0] || r.cachedIncObj == nil {
		return false
	}
	for cur := vn; cur != nil; cur = cur.parent {
		if cur.bestDualRHS != nil {
			return r.cachedIncObj.Cmp(cur.bestDualRHS) <= 0
		}
	}
	return false
}

func (r *babVIPRRecorder) exactTreeGapClosed(bestPrimal []float64) bool {
	if r == nil || r.invalid || !r.cachedIncOK || r.root == nil || len(bestPrimal) == 0 {
		return false
	}
	if r.cachedIncPtr != &bestPrimal[0] || r.cachedIncObj == nil {
		return false
	}
	var walk func(vn *viprTreeNode, bestAncRHS *big.Rat) bool
	walk = func(vn *viprTreeNode, bestAncRHS *big.Rat) bool {
		if vn == nil {
			return false
		}
		curBest := bestAncRHS
		if vn.bestDualRHS != nil && (curBest == nil || vn.bestDualRHS.Cmp(curBest) > 0) {
			curBest = vn.bestDualRHS
		}
		if curBest != nil && r.cachedIncObj.Cmp(curBest) <= 0 {
			return true
		}
		switch vn.kind {
		case viprNodeLeafBoundConflict, viprNodeLeafFarkas:
			return true
		case viprNodeLeafDual, viprNodeOpen:
			return false
		case viprNodeBranch, viprNodeOrbitBranch:
			return walk(vn.left, curBest) && walk(vn.right, curBest)
		default:
			return false
		}
	}
	return walk(r.root, nil)
}

func (r *babVIPRRecorder) solveCutScratchNode(lbR, ubR []*big.Rat, strictDual bool) (*LPSolver, LPResult, bool) {
	sc := r.ensureCutScratchLP()
	if sc == nil {
		return nil, LPResult{}, false
	}
	lbF := make([]float64, r.rp.n)
	ubF := make([]float64, r.rp.n)
	for j := range r.rp.n {
		lbF[j] = viprRatToOutwardFloat(lbR[j], false)
		ubF[j] = viprRatToOutwardFloat(ubR[j], true)
	}
	sc.SetStructuralBounds(lbF, ubF)
	origDualTol := sc.effDualTol
	if strictDual {
		sc.effDualTol = min(origDualTol, 1e-13)
	}
	res := sc.Solve()
	sc.effDualTol = origDualTol
	return sc, res, true
}

func (r *babVIPRRecorder) tryTightenNodeDualForPrune(vn *viprTreeNode, bestPrimal []float64) (closed, infeas bool) {
	if r == nil || r.invalid || !r.cachedIncOK || vn == nil || len(bestPrimal) == 0 {
		return false, false
	}
	if r.exactNodeGapClosed(vn, bestPrimal) {
		return true, false
	}
	closed, infeas, _ = r.polishNodeDualExactSimplex(vn, r.cachedIncObj)
	return closed, infeas
}

func (r *babVIPRRecorder) certifyNodeFarkas(vn *viprTreeNode, s *LPSolver, lpRes LPResult) {
	if r == nil || r.invalid || vn == nil {
		return
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	for j := range r.rp.n {
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return
		}
	}
	if w, ok := r.extractNodeFarkasActive(s, lpRes.Dual, lbR, ubR); ok {
		vn.kind = viprNodeLeafFarkas
		vn.farkas = w
		return
	}
	// Retry once on a cut-synced scratch LP solver when warm-start state cannot yield an exact ray.
	if sc, res, ok := r.solveCutScratchNode(lbR, ubR, false); ok {
		switch res.Status {
		case StatusInfeasible:
			if w, ok := r.extractNodeFarkasActive(sc, res.Dual, lbR, ubR); ok {
				vn.kind = viprNodeLeafFarkas
				vn.farkas = w
				return
			}
		case StatusOptimal:
			r.recordNodeDual(vn, sc, res)
			return
		}
	}
	r.invalid = true
}

func (r *babVIPRRecorder) certifyRowActivityFarkas(vn *viprTreeNode, row int) bool {
	if r == nil || r.invalid || vn == nil || row < 0 || row >= r.rp.m {
		return false
	}
	gID := r.activeRowID(row)
	lbR, ubR := r.nodeBoundsRat(vn)
	for _, sgn := range []int64{1, -1} {
		cand := []viprRowMult{{rowID: gID, mult: big.NewRat(sgn, 1)}}
		if w, _, ok := r.projectFarkasMults(cand, lbR, ubR); ok {
			vn.kind = viprNodeLeafFarkas
			vn.farkas = w
			return true
		}
	}
	return false
}

func (r *babVIPRRecorder) canCertifyIncumbent(cand []float64, candMin float64) bool {
	if r == nil || r.invalid || r.rp == nil || len(cand) < r.rp.n {
		return false
	}
	incRat, incObj, ok := r.rp.refineExactIncumbent(cand[:r.rp.n], r.ensureBaseScratchLP)
	if ok && len(cand) > 0 {
		r.cachedIncPtr = &cand[0]
		r.cachedIncObjMin = candMin
		r.cachedIncRat = incRat
		r.cachedIncObj = incObj
		r.cachedIncOK = true
	}
	return ok
}

func (r *babVIPRRecorder) certifySubtreeLeaf(vn *viprTreeNode) {
	if r == nil || r.invalid || vn == nil {
		return
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	for j := range r.rp.n {
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return
		}
	}
	sc, res, ok := r.solveCutScratchNode(lbR, ubR, r.cachedIncOK)
	if !ok {
		return
	}
	switch res.Status {
	case StatusInfeasible:
		r.certifyNodeFarkas(vn, sc, res)
	case StatusOptimal:
		r.recordNodeDual(vn, sc, res)
	}
}

func (r *babVIPRRecorder) recordBranch(vn *viprTreeNode, col int, splitVal *big.Int) (left, right *viprTreeNode) {
	if r == nil || r.invalid || vn == nil {
		return nil, nil
	}
	if col < 0 || col >= r.rp.n || !r.rp.isInt[col] || splitVal == nil {
		r.invalid = true
		return nil, nil
	}
	sv := new(big.Int).Set(splitVal)
	svPlus := new(big.Int).Add(sv, big.NewInt(1))
	left = &viprTreeNode{
		parent:     vn,
		asmCol:     col,
		asmIsUpper: true,
		asmBound:   sv,
		hasAsm:     true,
		kind:       viprNodeOpen,
	}
	right = &viprTreeNode{
		parent:     vn,
		asmCol:     col,
		asmIsUpper: false,
		asmBound:   svPlus,
		hasAsm:     true,
		kind:       viprNodeOpen,
	}
	vn.kind = viprNodeBranch
	vn.branchCol = col
	vn.splitVal = sv
	vn.left = left
	vn.right = right
	return left, right
}

func viprIsBijection(n int, perm []int) bool {
	if len(perm) != n {
		return false
	}
	seen := make([]bool, n)
	for _, p := range perm {
		if p < 0 || p >= n || seen[p] {
			return false
		}
		seen[p] = true
	}
	return true
}

func viprSparseRatPermMatch(srcCols []int, srcVals []*big.Rat, dstCols []int, dstVals []*big.Rat, colPerm []int, negate bool) bool {
	srcNNZ := 0
	for _, v := range srcVals {
		if v != nil && v.Sign() != 0 {
			srcNNZ++
		}
	}
	dstNNZ := 0
	for _, v := range dstVals {
		if v != nil && v.Sign() != 0 {
			dstNNZ++
		}
	}
	if srcNNZ != dstNNZ {
		return false
	}
	srcMap := make(map[int]*big.Rat, srcNNZ)
	for k, c := range srcCols {
		if srcVals[k] != nil && srcVals[k].Sign() != 0 {
			srcMap[c] = srcVals[k]
		}
	}
	var neg big.Rat
	for k, c := range dstCols {
		if dstVals[k] == nil || dstVals[k].Sign() == 0 {
			continue
		}
		sv, ok := srcMap[colPerm[c]]
		if !ok {
			return false
		}
		if negate {
			neg.Neg(sv)
			if dstVals[k].Cmp(&neg) != 0 {
				return false
			}
		} else if dstVals[k].Cmp(sv) != 0 {
			return false
		}
	}
	return true
}

func viprRatBaseRowPermImplies(rp *viprRatProblem, srcIdx, dstIdx int, colPerm []int) bool {
	srcSense := rp.rowSense[srcIdx]
	dstSense := rp.rowSense[dstIdx]
	srcRHS := rp.rowRHS[srcIdx]
	dstRHS := rp.rowRHS[dstIdx]
	if viprSparseRatPermMatch(rp.rowCols[srcIdx], rp.rowVals[srcIdx], rp.rowCols[dstIdx], rp.rowVals[dstIdx], colPerm, false) {
		switch {
		case srcSense == LessEqual && dstSense == LessEqual:
			if srcRHS.Cmp(dstRHS) <= 0 {
				return true
			}
		case srcSense == GreaterEqual && dstSense == GreaterEqual:
			if dstRHS.Cmp(srcRHS) <= 0 {
				return true
			}
		case srcSense == Equal && dstSense == Equal:
			if srcRHS.Cmp(dstRHS) == 0 {
				return true
			}
		case srcSense == Equal && dstSense == LessEqual:
			if srcRHS.Cmp(dstRHS) <= 0 {
				return true
			}
		case srcSense == Equal && dstSense == GreaterEqual:
			if dstRHS.Cmp(srcRHS) <= 0 {
				return true
			}
		}
	}
	if viprSparseRatPermMatch(rp.rowCols[srcIdx], rp.rowVals[srcIdx], rp.rowCols[dstIdx], rp.rowVals[dstIdx], colPerm, true) {
		negSrcRHS := new(big.Rat).Neg(srcRHS)
		switch {
		case srcSense == LessEqual && dstSense == GreaterEqual:
			return dstRHS.Cmp(negSrcRHS) <= 0
		case srcSense == GreaterEqual && dstSense == LessEqual:
			return negSrcRHS.Cmp(dstRHS) <= 0
		case srcSense == Equal && dstSense == Equal:
			return negSrcRHS.Cmp(dstRHS) == 0
		case srcSense == Equal && dstSense == LessEqual:
			return negSrcRHS.Cmp(dstRHS) <= 0
		case srcSense == Equal && dstSense == GreaterEqual:
			return dstRHS.Cmp(negSrcRHS) <= 0
		}
	}
	return false
}

func viprCutPermImplies(src, dst *viprGlobalCut, colPerm []int) bool {
	if src.rhs == nil || dst.rhs == nil || src.rhs.Cmp(dst.rhs) > 0 {
		return false
	}
	return viprSparseRatPermMatch(src.cols, src.vals, dst.cols, dst.vals, colPerm, false)
}

// filterVerifiedOrbitCompanions checks every candidate companion against baseRows,
// objective, base bounds, exact node bounds, and active global cuts using exact
// *big.Rat arithmetic, returning only companions with verified witnesses.
func (r *babVIPRRecorder) filterVerifiedOrbitCompanions(
	vn *viprTreeNode,
	brVar int,
	cands []orbitCandidateWitness,
) []viprOrbitWitness {
	if r == nil || r.invalid || vn == nil || len(cands) == 0 {
		return nil
	}
	rp := r.rp
	if brVar < 0 || brVar >= rp.n || !rp.isInt[brVar] {
		return nil
	}
	nodeLB, nodeUB := r.nodeBoundsRat(vn)
	one := big.NewRat(1, 1)
	if nodeLB[brVar] == nil || nodeUB[brVar] == nil || nodeLB[brVar].Sign() != 0 || nodeUB[brVar].Cmp(one) != 0 {
		return nil
	}

	seenComp := make(map[int]bool, len(cands))
	var out []viprOrbitWitness
	for _, cand := range cands {
		c := cand.companion
		if c < 0 || c >= rp.n || c == brVar || seenComp[c] || !rp.isInt[c] {
			continue
		}
		if nodeLB[c] == nil || nodeUB[c] == nil || nodeLB[c].Sign() != 0 || nodeUB[c].Cmp(one) != 0 {
			continue
		}
		if !viprIsBijection(rp.n, cand.colPerm) || cand.colPerm[brVar] != c || len(cand.rowPerm) != rp.m {
			continue
		}
		colsOK := true
		for j := range rp.n {
			p := cand.colPerm[j]
			if (rp.isInt[j] && !rp.isInt[p]) || rp.objMin[j].Cmp(rp.objMin[p]) != 0 {
				colsOK = false
				break
			}
			if !viprOptRatLbLe(rp.lb[j], rp.lb[p]) || !viprOptRatUbLe(rp.ub[p], rp.ub[j]) {
				colsOK = false
				break
			}
			if !viprOptRatLbLe(nodeLB[j], nodeLB[p]) || !viprOptRatUbLe(nodeUB[p], nodeUB[j]) {
				colsOK = false
				break
			}
		}
		if !colsOK {
			continue
		}
		rowsOK := true
		for i := range rp.m {
			ri := cand.rowPerm[i]
			if ri < 0 || ri >= rp.m || !viprRatBaseRowPermImplies(rp, ri, i, cand.colPerm) {
				rowsOK = false
				break
			}
		}
		if !rowsOK {
			continue
		}
		cutsOK := true
		for _, cutID := range r.activeCutIDs {
			cDst := &r.allCuts[cutID]
			matched := false
			for _, otherID := range r.activeCutIDs {
				if viprCutPermImplies(&r.allCuts[otherID], cDst, cand.colPerm) {
					matched = true
					break
				}
			}
			if !matched {
				cutsOK = false
				break
			}
		}
		if !cutsOK {
			continue
		}
		seenComp[c] = true
		out = append(out, viprOrbitWitness{
			companion: c,
			colPerm:   slices.Clone(cand.colPerm),
			rowPerm:   slices.Clone(cand.rowPerm),
		})
	}
	return out
}

func (r *babVIPRRecorder) recordOrbitBranch(
	vn *viprTreeNode,
	brVar int,
	witnesses []viprOrbitWitness,
) (left, right *viprTreeNode) {
	if r == nil || r.invalid || vn == nil {
		return nil, nil
	}
	if len(witnesses) == 0 {
		return r.recordBranch(vn, brVar, big.NewInt(0))
	}
	if brVar < 0 || brVar >= r.rp.n || !r.rp.isInt[brVar] {
		r.invalid = true
		return nil, nil
	}
	orbCols := make([]int, len(witnesses))
	for i, w := range witnesses {
		orbCols[i] = w.companion
	}
	left = &viprTreeNode{
		parent:       vn,
		asmCol:       brVar,
		asmIsUpper:   true,
		asmBound:     big.NewInt(0),
		hasAsm:       true,
		orbitAsmCols: orbCols,
		kind:         viprNodeOpen,
	}
	right = &viprTreeNode{
		parent:     vn,
		asmCol:     brVar,
		asmIsUpper: false,
		asmBound:   big.NewInt(1),
		hasAsm:     true,
		kind:       viprNodeOpen,
	}
	vn.kind = viprNodeOrbitBranch
	vn.branchCol = brVar
	vn.splitVal = big.NewInt(0)
	vn.orbitWitnesses = witnesses
	vn.left = left
	vn.right = right
	r.orbitBranchesCertified++
	return left, right
}

func (r *babVIPRRecorder) recordSBPruned(vn *viprTreeNode, col int, splitFloat float64) {
	if r == nil || r.invalid || vn == nil {
		return
	}
	splitVal, ok := viprFloorBigInt(splitFloat)
	if !ok {
		r.invalid = true
		return
	}
	left, right := r.recordBranch(vn, col, splitVal)
	if left == nil || right == nil {
		return
	}
	r.certifySubtreeLeaf(left)
	r.certifySubtreeLeaf(right)
}

func (r *babVIPRRecorder) recordSBFix(vn *viprTreeNode, col int, splitFloat float64, downPruned bool) *viprTreeNode {
	if r == nil || r.invalid || vn == nil {
		return vn
	}
	splitVal, ok := viprFloorBigInt(splitFloat)
	if !ok {
		r.invalid = true
		return vn
	}
	left, right := r.recordBranch(vn, col, splitVal)
	if left == nil || right == nil {
		return vn
	}
	if downPruned {
		r.certifySubtreeLeaf(left)
		return right
	}
	r.certifySubtreeLeaf(right)
	return left
}

func (r *babVIPRRecorder) exactCutoffRat(cutoff mipCutoffState) (*big.Rat, bool) {
	if r == nil || r.invalid || r.rp == nil {
		return nil, false
	}
	if len(cutoff.bestPrimal) == r.rp.n {
		var ptr *float64
		if len(cutoff.bestPrimal) > 0 {
			ptr = &cutoff.bestPrimal[0]
		}
		if r.cachedIncPtr != ptr || r.cachedIncObjMin != cutoff.bestObjMin {
			incRat, incObj, ok := r.rp.refineExactIncumbent(cutoff.bestPrimal, r.ensureBaseScratchLP)
			r.cachedIncPtr = ptr
			r.cachedIncObjMin = cutoff.bestObjMin
			r.cachedIncRat = incRat
			r.cachedIncObj = incObj
			r.cachedIncOK = ok
		}
		if r.cachedIncOK && r.cachedIncObj != nil {
			return r.cachedIncObj, true
		}
		return nil, false
	}
	if cutoff.hasCutoff && !math.IsInf(cutoff.bestObjMin, 0) && !math.IsNaN(cutoff.bestObjMin) {
		return exactRatFromFloat(cutoff.bestObjMin - cutoff.searchShift)
	}
	return nil, false
}

func (r *babVIPRRecorder) bestDualForRCF(vn *viprTreeNode, s *LPSolver, lpRes LPResult, lbR, ubR []*big.Rat) ([]viprRowMult, *big.Rat, []*big.Rat, bool) {
	var bestMults []viprRowMult
	var bestRHS *big.Rat
	var bestRC []*big.Rat

	tryCand := func(mults []viprRowMult) {
		rhs, rc, ok := r.evalSafeDualMults(mults, lbR, ubR)
		if !ok {
			return
		}
		if bestRHS == nil || rhs.Cmp(bestRHS) > 0 {
			bestMults = mults
			bestRHS = rhs
			bestRC = rc
		}
	}

	if d, ok := nearestDual(vn); ok {
		tryCand(d)
	}
	if s != nil {
		if mults, rhs, ok := r.extractNodeSafeDualActive(s, lpRes, lbR, ubR); ok {
			if vn != nil && (vn.bestDualRHS == nil || rhs.Cmp(vn.bestDualRHS) > 0) {
				vn.bestDual = mults
				vn.bestDualRHS = rhs
			}
			tryCand(mults)
		}
	}
	return bestMults, bestRHS, bestRC, bestRHS != nil
}

// certifyAndApplyRCF verifies each reduced-cost fixing candidate in exact rational arithmetic
// using a branch split whose pruned child is closed by safeDual (or boundConflict). Candidates
// whose exact rational dual bound fails to reach targetCutoff are skipped individually.
func (r *babVIPRRecorder) certifyAndApplyRCF(
	vn *viprTreeNode,
	s *LPSolver,
	lpRes LPResult,
	cutoff mipCutoffState,
	origLB, origUB []float64,
	cands []boundDelta,
) (nextVN *viprTreeNode, applied []boundDelta, pruned bool) {
	if r == nil || r.invalid || vn == nil || len(cands) == 0 {
		return vn, nil, false
	}
	targetCutoff, okCut := r.exactCutoffRat(cutoff)
	if !okCut {
		return vn, nil, false
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	mults, baseRHS, rc, okDual := r.bestDualForRCF(vn, s, lpRes, lbR, ubR)
	if !okDual {
		return vn, nil, false
	}

	term := new(big.Rat)
	diff := new(big.Rat)
	curVN := vn
	applySide := func(col int, isUpper bool, kRat *big.Rat, splitBig *big.Int, prunedConflict bool, rhsPruned *big.Rat) (prunedNow, stop bool) {
		left, right := r.recordBranch(curVN, col, splitBig)
		if left == nil || right == nil {
			return false, true
		}
		prunedChild, survChild := left, right
		if isUpper {
			prunedChild, survChild = right, left
		}
		if prunedConflict {
			prunedChild.kind = viprNodeLeafBoundConflict
			prunedChild.conflictCol = col
		} else {
			prunedChild.kind = viprNodeLeafDual
			prunedChild.bestDual = mults
			prunedChild.bestDualRHS = rhsPruned
		}
		if isUpper {
			if rc[col].Sign() < 0 && ubR[col] != nil {
				diff.Sub(kRat, ubR[col])
				term.Mul(rc[col], diff)
				baseRHS.Add(baseRHS, term)
			}
			ubR[col] = kRat
		} else {
			if rc[col].Sign() > 0 && lbR[col] != nil {
				diff.Sub(kRat, lbR[col])
				term.Mul(rc[col], diff)
				baseRHS.Add(baseRHS, term)
			}
			lbR[col] = kRat
		}
		curVN = survChild
		r.rcfFixingsCertified++
		applied = append(applied, boundDelta{
			col: col,
			lb:  viprRatToOutwardFloat(lbR[col], false),
			ub:  viprRatToOutwardFloat(ubR[col], true),
		})
		if lbR[col] != nil && ubR[col] != nil && ubR[col].Cmp(lbR[col]) < 0 {
			curVN.kind = viprNodeLeafBoundConflict
			curVN.conflictCol = col
			return true, true
		}
		return false, false
	}
	for _, d := range cands {
		col := d.col
		if col < 0 || col >= r.rp.n || !r.rp.isInt[col] {
			continue
		}
		if d.ub < origUB[col]-0.5 {
			if kInt, okK := viprFloorBigInt(d.ub); okK {
				kRat := new(big.Rat).SetInt(kInt)
				if ubR[col] == nil || kRat.Cmp(ubR[col]) < 0 {
					kPlusRat := new(big.Rat).SetInt(new(big.Int).Add(kInt, big.NewInt(1)))
					prunedConflict := ubR[col] != nil && ubR[col].Cmp(kPlusRat) < 0
					var rhsRight *big.Rat
					certOK := prunedConflict
					if !certOK && rc[col].Sign() > 0 && lbR[col] != nil {
						newLB := kPlusRat
						if lbR[col].Cmp(newLB) > 0 {
							newLB = lbR[col]
						}
						diff.Sub(newLB, lbR[col])
						term.Mul(rc[col], diff)
						rhsRight = new(big.Rat).Add(baseRHS, term)
						certOK = rhsRight.Cmp(targetCutoff) >= 0
					}
					if certOK {
						if prunedNow, stop := applySide(col, true, kRat, kInt, prunedConflict, rhsRight); stop {
							return curVN, applied, prunedNow
						}
					}
				}
			}
		}
		if d.lb > origLB[col]+0.5 {
			if kInt, okK := viprCeilBigInt(d.lb); okK {
				kRat := new(big.Rat).SetInt(kInt)
				if lbR[col] == nil || kRat.Cmp(lbR[col]) > 0 {
					splitInt := new(big.Int).Sub(kInt, big.NewInt(1))
					splitRat := new(big.Rat).SetInt(splitInt)
					prunedConflict := lbR[col] != nil && splitRat.Cmp(lbR[col]) < 0
					var rhsLeft *big.Rat
					certOK := prunedConflict
					if !certOK && rc[col].Sign() < 0 && ubR[col] != nil {
						newUB := splitRat
						if ubR[col].Cmp(newUB) < 0 {
							newUB = ubR[col]
						}
						diff.Sub(newUB, ubR[col])
						term.Mul(rc[col], diff)
						rhsLeft = new(big.Rat).Add(baseRHS, term)
						certOK = rhsLeft.Cmp(targetCutoff) >= 0
					}
					if certOK {
						if prunedNow, stop := applySide(col, false, kRat, splitInt, prunedConflict, rhsLeft); stop {
							return curVN, applied, prunedNow
						}
					}
				}
			}
		}
	}
	return curVN, applied, false
}

func (r *babVIPRRecorder) certifyPrunedChildForCutoff(
	vn *viprTreeNode,
	childLB, childUB []*big.Rat,
	col int,
	targetCutoff *big.Rat,
) (kind viprNodeKind, mults []viprRowMult, rhs *big.Rat, farkas []viprRowMult, ok bool) {
	if childLB[col] != nil && childUB[col] != nil && childUB[col].Cmp(childLB[col]) < 0 {
		return viprNodeLeafBoundConflict, nil, nil, nil, true
	}
	// Fast path 1: zero-row objective bound (direct objective cutoff).
	if r0, _, ok0 := r.evalSafeDualMults(nil, childLB, childUB); ok0 && r0.Cmp(targetCutoff) >= 0 {
		return viprNodeLeafDual, nil, r0, nil, true
	}
	// Fast path 2: reuse ancestor LP dual multipliers.
	if d, hasD := nearestDual(vn); hasD {
		if rd, _, okD := r.evalSafeDualMults(d, childLB, childUB); okD && rd.Cmp(targetCutoff) >= 0 {
			return viprNodeLeafDual, d, rd, nil, true
		}
	}
	// Fallback 3: solve cut-synced scratch LP on the pruned child domain (handles propagated bounds).
	sc, res, ok := r.solveCutScratchNode(childLB, childUB, false)
	if !ok {
		return viprNodeOpen, nil, nil, nil, false
	}
	switch res.Status {
	case StatusInfeasible:
		if w, okF := r.extractNodeFarkasActive(sc, res.Dual, childLB, childUB); okF {
			return viprNodeLeafFarkas, nil, nil, w, true
		}
	case StatusOptimal:
		if d, rd, okD := r.extractNodeSafeDualActive(sc, res, childLB, childUB); okD && rd.Cmp(targetCutoff) >= 0 {
			return viprNodeLeafDual, d, rd, nil, true
		}
	}
	return viprNodeOpen, nil, nil, nil, false
}

// certifyAndApplyUnboundedIntTightening certifies finite bounds on previously unbounded
// integer columns via branch splits whose pruned sides are closed against targetCutoff.
func (r *babVIPRRecorder) certifyAndApplyUnboundedIntTightening(
	vn *viprTreeNode,
	cutoff mipCutoffState,
	cands []viprBoundCand,
) (nextVN *viprTreeNode, changed, pruned bool) {
	if r == nil || r.invalid || vn == nil || len(cands) == 0 {
		return vn, false, false
	}
	targetCutoff, okCut := r.exactCutoffRat(cutoff)
	if !okCut {
		return vn, false, false
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	curVN := vn
	bigOne := big.NewInt(1)
	for _, cand := range cands {
		col := cand.col
		if col < 0 || col >= r.rp.n || !r.rp.isInt[col] {
			continue
		}
		var kInt, splitInt *big.Int
		var okK bool
		if cand.isUpper {
			kInt, okK = viprFloorBigInt(cand.bound)
		} else {
			kInt, okK = viprCeilBigInt(cand.bound)
		}
		if !okK {
			continue
		}
		kRat := new(big.Rat).SetInt(kInt)
		childLB := slices.Clone(lbR)
		childUB := slices.Clone(ubR)
		if cand.isUpper {
			if ubR[col] != nil && kRat.Cmp(ubR[col]) >= 0 {
				continue
			}
			splitInt = kInt
			kPlusRat := new(big.Rat).SetInt(new(big.Int).Add(kInt, bigOne))
			if childLB[col] == nil || kPlusRat.Cmp(childLB[col]) > 0 {
				childLB[col] = kPlusRat
			}
		} else {
			if lbR[col] != nil && kRat.Cmp(lbR[col]) <= 0 {
				continue
			}
			splitInt = new(big.Int).Sub(kInt, bigOne)
			splitRat := new(big.Rat).SetInt(splitInt)
			if childUB[col] == nil || splitRat.Cmp(childUB[col]) < 0 {
				childUB[col] = splitRat
			}
		}
		pKind, pDual, pRHS, pFarkas, okCert := r.certifyPrunedChildForCutoff(curVN, childLB, childUB, col, targetCutoff)
		if !okCert {
			continue
		}
		left, right := r.recordBranch(curVN, col, splitInt)
		if left == nil || right == nil {
			return curVN, changed, false
		}
		prunedChild, survChild := left, right
		if cand.isUpper {
			prunedChild, survChild = right, left
			ubR[col] = kRat
		} else {
			lbR[col] = kRat
		}
		prunedChild.kind = pKind
		switch pKind {
		case viprNodeLeafBoundConflict:
			prunedChild.conflictCol = col
		case viprNodeLeafDual:
			prunedChild.bestDual = pDual
			prunedChild.bestDualRHS = pRHS
		case viprNodeLeafFarkas:
			prunedChild.farkas = pFarkas
		}
		curVN = survChild
		changed = true
		r.unbTightCertified++
		if lbR[col] != nil && ubR[col] != nil && ubR[col].Cmp(lbR[col]) < 0 {
			curVN.kind = viprNodeLeafBoundConflict
			curVN.conflictCol = col
			return curVN, changed, true
		}
	}
	return curVN, changed, false
}

func viprRatDenomOK(v *big.Rat) bool {
	return v != nil && v.Denom().BitLen() <= viprMaxRatDenomBits
}

func (r *babVIPRRecorder) verifyGlobalCutRat(cut viprGlobalCut) bool {
	rp := r.rp
	if len(cut.cols) == 0 || len(cut.cols) != len(cut.vals) || !viprRatDenomOK(cut.rhs) {
		return false
	}
	for k, col := range cut.cols {
		if col < 0 || col >= rp.n || cut.vals[k] == nil || cut.vals[k].Sign() == 0 || !viprRatDenomOK(cut.vals[k]) {
			return false
		}
		if k > 0 && col <= cut.cols[k-1] {
			return false
		}
	}

	evalLinLe := func(refs []viprRowMult, bounds []viprBoundTerm) (map[int]*big.Rat, *big.Rat, bool) {
		acc := make(map[int]*big.Rat, len(cut.cols)+4)
		rhs := new(big.Rat)
		term := new(big.Rat)
		for _, rm := range refs {
			if rm.mult == nil || rm.mult.Sign() == 0 {
				continue
			}
			if !viprRatDenomOK(rm.mult) || rm.rowID < 0 || rm.rowID >= rp.m+len(r.allCuts) {
				return nil, nil, false
			}
			sense, rowRHS, cols, vals := r.rowData(rm.rowID)
			if sense == LessEqual && rm.mult.Sign() < 0 {
				return nil, nil, false
			}
			if sense == GreaterEqual && rm.mult.Sign() > 0 {
				return nil, nil, false
			}
			term.Mul(rm.mult, rowRHS)
			rhs.Add(rhs, term)
			for k, col := range cols {
				term.Mul(rm.mult, vals[k])
				cur := acc[col]
				if cur == nil {
					cur = new(big.Rat)
					acc[col] = cur
				}
				cur.Add(cur, term)
			}
		}
		for _, bt := range bounds {
			if bt.mult == nil || bt.mult.Sign() == 0 {
				continue
			}
			if !viprRatDenomOK(bt.mult) || bt.col < 0 || bt.col >= rp.n {
				return nil, nil, false
			}
			var bd *big.Rat
			if bt.isUpper {
				if bt.mult.Sign() < 0 || rp.ub[bt.col] == nil {
					return nil, nil, false
				}
				bd = rp.ub[bt.col]
			} else {
				if bt.mult.Sign() > 0 || rp.lb[bt.col] == nil {
					return nil, nil, false
				}
				bd = rp.lb[bt.col]
			}
			term.Mul(bt.mult, bd)
			rhs.Add(rhs, term)
			cur := acc[bt.col]
			if cur == nil {
				cur = new(big.Rat)
				acc[bt.col] = cur
			}
			cur.Add(cur, bt.mult)
		}
		return acc, rhs, viprRatDenomOK(rhs)
	}

	matchSparse := func(acc map[int]*big.Rat) bool {
		nz := 0
		for _, v := range acc {
			if v != nil && v.Sign() != 0 {
				nz++
			}
		}
		if nz != len(cut.cols) {
			return false
		}
		for k, col := range cut.cols {
			got := acc[col]
			if got == nil || got.Cmp(cut.vals[k]) != 0 {
				return false
			}
		}
		return true
	}

	switch cut.kind {
	case viprCutLinComb:
		acc, rhs, ok := evalLinLe(cut.refs, cut.bounds)
		return ok && rhs.Cmp(cut.rhs) == 0 && matchSparse(acc)
	case viprCutCG:
		acc, rhs, ok := evalLinLe(cut.refs, cut.bounds)
		if !ok {
			return false
		}
		for col, v := range acc {
			if v == nil || v.Sign() == 0 {
				continue
			}
			if !rp.isInt[col] || !v.IsInt() {
				return false
			}
		}
		if viprRatFloor(rhs).Cmp(cut.rhs) != 0 {
			return false
		}
		return matchSparse(acc)
	case viprCutVUBBranch:
		if cut.vubRowID < 0 || cut.vubRowID >= rp.m || rp.rowSense[cut.vubRowID] != LessEqual {
			return false
		}
		if cut.vubCol < 0 || cut.vubCol >= rp.n || cut.vubBinCol < 0 || cut.vubBinCol >= rp.n || !rp.isInt[cut.vubBinCol] {
			return false
		}
		if cut.vubInvA == nil || cut.vubInvA.Sign() <= 0 || !viprRatDenomOK(cut.vubInvA) {
			return false
		}
		if cut.vubDelta == nil || cut.vubDelta.Sign() <= 0 || !viprRatDenomOK(cut.vubDelta) {
			return false
		}
		if cut.vubUB == nil || cut.vubUB.Sign() <= 0 || rp.ub[cut.vubCol] == nil || rp.ub[cut.vubCol].Cmp(cut.vubUB) != 0 {
			return false
		}
		accL, rhsL, okL := evalLinLe([]viprRowMult{{rowID: cut.vubRowID, mult: cut.vubInvA}}, cut.vubLeftBounds)
		if !okL {
			return false
		}
		curBin := accL[cut.vubBinCol]
		if curBin == nil {
			curBin = new(big.Rat)
			accL[cut.vubBinCol] = curBin
		}
		curBin.Add(curBin, cut.vubDelta)
		if !matchSparse(accL) {
			return false
		}
		// Right leaf derives x_col - vubUB * x_bin <= 0; branch merge takes max(rhsL, 0).
		mergedRHS := new(big.Rat)
		if rhsL.Sign() > 0 {
			mergedRHS.Set(rhsL)
		}
		return mergedRHS.Cmp(cut.rhs) == 0
	default:
		return false
	}
}

func buildFloatCutFromRat(
	rp *viprRatProblem,
	cols []int,
	vals []*big.Rat,
	rhs *big.Rat,
	lpPrimal []float64,
	maxNNZ int,
	minEff float64,
) (cutPlane, int, bool) {
	nnz := len(cols)
	if nnz == 0 || nnz != len(vals) || (maxNNZ > 0 && nnz > maxNNZ) {
		return cutPlane{}, 0, false
	}
	p := rp.p
	n := rp.n
	vf := make([]float64, nnz)
	maxAbs := 0.0
	minAbs := math.Inf(1)
	padRat := new(big.Rat)
	diffRat := new(big.Rat)
	termRat := new(big.Rat)
	for k, col := range cols {
		if uint(col) >= uint(n) || vals[k] == nil || vals[k].Sign() == 0 {
			return cutPlane{}, 0, false
		}
		f, exact := vals[k].Float64()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return cutPlane{}, 0, false
		}
		af := math.Abs(f)
		if af <= cutDropTol {
			return cutPlane{}, 0, false
		}
		if !exact {
			fr := new(big.Rat).SetFloat64(f)
			if fr == nil {
				return cutPlane{}, 0, false
			}
			diffRat.Sub(fr, vals[k])
			if diffRat.Sign() > 0 {
				if rp.ub[col] == nil {
					return cutPlane{}, 0, false
				}
				termRat.Mul(diffRat, rp.ub[col])
				padRat.Add(padRat, termRat)
			} else if diffRat.Sign() < 0 {
				if rp.lb[col] == nil {
					return cutPlane{}, 0, false
				}
				termRat.Mul(diffRat, rp.lb[col])
				padRat.Add(padRat, termRat)
			}
		}
		vf[k] = f
		if af > maxAbs {
			maxAbs = af
		}
		if af < minAbs {
			minAbs = af
		}
	}
	if maxAbs == 0 || maxAbs/minAbs > maxCutDynamism {
		return cutPlane{}, 0, false
	}

	scaledNorm2 := 0.0
	for _, v := range vf {
		q := v / maxAbs
		scaledNorm2 += q * q
	}
	norm := maxAbs * math.Sqrt(scaledNorm2)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return cutPlane{}, 0, false
	}
	_, rawExp := math.Frexp(norm)
	normExp := rawExp - 1
	scaledNorm := math.Ldexp(norm, -normExp)
	if !(scaledNorm > 0) || math.IsNaN(scaledNorm) || math.IsInf(scaledNorm, 0) {
		return cutPlane{}, 0, false
	}

	outVal := make([]float64, nnz)
	act := 0.0
	subScaleErr := 0.0
	for k, col := range cols {
		sv := math.Ldexp(vf[k], -normExp)
		if sv == 0 || math.IsNaN(sv) || math.IsInf(sv, 0) {
			return cutPlane{}, 0, false
		}
		if math.Ldexp(sv, normExp) != vf[k] {
			maxB := max(math.Abs(p.varLB[col]), math.Abs(p.varUB[col]))
			if math.IsInf(maxB, 0) || math.IsNaN(maxB) {
				return cutPlane{}, 0, false
			}
			subScaleErr += nextUp(math.SmallestNonzeroFloat64 * maxB)
		}
		outVal[k] = sv
		act += sv * lpPrimal[col]
	}

	effRHS := new(big.Rat).Add(rhs, padRat)
	effRHS.Mul(effRHS, viprPow2Rat(-normExp))
	rawRHS := viprRatToOutwardFloat(effRHS, true)
	scaledRHS := (rawRHS + subScaleErr) + rhsSafetyMargin*scaledNorm
	if math.IsNaN(scaledRHS) || math.IsInf(scaledRHS, 0) || math.IsNaN(act) || math.IsInf(act, 0) {
		return cutPlane{}, 0, false
	}
	efficacy := (act - scaledRHS) / scaledNorm
	if !(efficacy >= minEff) {
		return cutPlane{}, 0, false
	}

	return cutPlane{
		ColIdx:   slices.Clone(cols),
		Val:      outVal,
		RHS:      scaledRHS,
		Efficacy: efficacy,
		Norm:     scaledNorm,
	}, normExp, true
}

func (r *babVIPRRecorder) buildCertifiedCGCut(
	rowRefs []viprRowMult,
	lpPrimal []float64,
	maxNNZ int,
	minEff float64,
) (viprCertifiedCut, bool) {
	rp := r.rp
	if len(rowRefs) == 0 {
		return viprCertifiedCut{}, false
	}
	acc := make(map[int]*big.Rat, 16)
	touched := make([]int, 0, 16)
	linRHS := new(big.Rat)
	term := new(big.Rat)

	for _, rm := range rowRefs {
		if rm.mult == nil || rm.mult.Sign() == 0 {
			continue
		}
		if !viprRatDenomOK(rm.mult) {
			return viprCertifiedCut{}, false
		}
		sense, rowRHS, cols, vals := r.rowData(rm.rowID)
		if sense == LessEqual && rm.mult.Sign() < 0 {
			return viprCertifiedCut{}, false
		}
		if sense == GreaterEqual && rm.mult.Sign() > 0 {
			return viprCertifiedCut{}, false
		}
		term.Mul(rm.mult, rowRHS)
		linRHS.Add(linRHS, term)
		for k, col := range cols {
			term.Mul(rm.mult, vals[k])
			cur := acc[col]
			if cur == nil {
				cur = new(big.Rat)
				acc[col] = cur
				touched = append(touched, col)
			}
			cur.Add(cur, term)
		}
	}
	if len(touched) == 0 || !viprRatDenomOK(linRHS) {
		return viprCertifiedCut{}, false
	}
	slices.Sort(touched)

	var bounds []viprBoundTerm
	cutCols := make([]int, 0, len(touched))
	cutVals := make([]*big.Rat, 0, len(touched))

	for _, col := range touched {
		v := acc[col]
		if v == nil || v.Sign() == 0 {
			continue
		}
		if !viprRatDenomOK(v) {
			return viprCertifiedCut{}, false
		}
		if !rp.isInt[col] {
			if v.Sign() > 0 {
				if rp.lb[col] == nil {
					return viprCertifiedCut{}, false
				}
				mult := new(big.Rat).Neg(v)
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: false, mult: mult})
				term.Mul(mult, rp.lb[col])
				linRHS.Add(linRHS, term)
			} else {
				if rp.ub[col] == nil {
					return viprCertifiedCut{}, false
				}
				mult := new(big.Rat).Neg(v)
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: true, mult: mult})
				term.Mul(mult, rp.ub[col])
				linRHS.Add(linRHS, term)
			}
			continue
		}
		if !v.IsInt() {
			var useLower bool
			hasLB := rp.lb[col] != nil
			hasUB := rp.ub[col] != nil
			switch {
			case hasLB && hasUB:
				vf, _ := v.Float64()
				flF := math.Floor(vf)
				clF := math.Ceil(vf)
				xv := lpPrimal[col]
				lbSlack := (vf - flF) * max(0.0, xv-rp.p.varLB[col])
				ubSlack := (clF - vf) * max(0.0, rp.p.varUB[col]-xv)
				useLower = lbSlack <= ubSlack
			case hasLB:
				useLower = true
			case hasUB:
				useLower = false
			default:
				return viprCertifiedCut{}, false
			}
			if useLower {
				fl := viprRatFloor(v)
				delta := new(big.Rat).Sub(fl, v)
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: false, mult: delta})
				term.Mul(delta, rp.lb[col])
				linRHS.Add(linRHS, term)
				v = fl
			} else {
				cl := viprRatCeil(v)
				delta := new(big.Rat).Sub(cl, v)
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: true, mult: delta})
				term.Mul(delta, rp.ub[col])
				linRHS.Add(linRHS, term)
				v = cl
			}
		}
		if v.Sign() != 0 {
			cutCols = append(cutCols, col)
			cutVals = append(cutVals, new(big.Rat).Set(v))
		}
	}

	if len(cutCols) == 0 || !viprRatDenomOK(linRHS) {
		return viprCertifiedCut{}, false
	}
	roundedRHS := viprRatFloor(linRHS)
	if roundedRHS.Cmp(linRHS) >= 0 {
		return viprCertifiedCut{}, false
	}
	plane, normExp, okF := buildFloatCutFromRat(rp, cutCols, cutVals, roundedRHS, lpPrimal, maxNNZ, minEff)
	if !okF {
		return viprCertifiedCut{}, false
	}
	gc := viprGlobalCut{
		cols:    cutCols,
		vals:    cutVals,
		rhs:     roundedRHS,
		normExp: normExp,
		kind:    viprCutCG,
		refs:    rowRefs,
		bounds:  bounds,
	}
	if !r.verifyGlobalCutRat(gc) {
		return viprCertifiedCut{}, false
	}
	return viprCertifiedCut{plane: plane, cut: gc}, true
}

func (r *babVIPRRecorder) rowHasContCols(rowID int) bool {
	_, _, cols, _ := r.rowData(rowID)
	for _, col := range cols {
		if !r.rp.isInt[col] {
			return true
		}
	}
	return false
}

func (r *babVIPRRecorder) separateCertifiedTableauCG(
	s *LPSolver,
	lpPrimal []float64,
	minEff float64,
) []viprCertifiedCut {
	rp := r.rp
	n := rp.n
	mActive := rp.m + len(r.activeCutIDs)
	if s == nil || s.ws == nil || s.TotalRows() != mActive || mActive == 0 {
		return nil
	}
	basis := s.ws.basis[:mActive]
	type fracRow struct {
		r    int
		dist float64
	}
	var fracRows []fracRow
	for rowSlot, bVar := range basis {
		if uint(bVar) >= uint(n) || !rp.isInt[bVar] {
			continue
		}
		xVal := s.unscaledBasicVal(rowSlot)
		f0 := xVal - math.Floor(xVal)
		if f0 < minIntInfeas || f0 > 1.0-minIntInfeas {
			continue
		}
		fracRows = append(fracRows, fracRow{r: rowSlot, dist: math.Abs(f0 - halfFracTarget)})
	}
	if len(fracRows) == 0 {
		return nil
	}
	if len(fracRows) > maxGMIFraqRows {
		slices.SortFunc(fracRows, func(a, b fracRow) int {
			return cmp.Compare(a.dist, b.dist)
		})
		fracRows = fracRows[:maxGMIFraqRows]
	}

	maxNNZ := maxCutNNZ(n)
	eqRHS := make([]*big.Rat, mActive)
	for q := range mActive {
		eqRHS[q] = new(big.Rat)
	}
	var out []viprCertifiedCut

	for _, fr := range fracRows {
		if s.timeExpired() {
			break
		}
		eqRHS[fr.r].SetInt64(1)
		u, ok := r.solveBasisTransposeActive(basis, eqRHS)
		eqRHS[fr.r].SetInt64(0)
		if !ok {
			continue
		}

		for _, orient := range [2]int64{1, -1} {
			for _, reduceFrac := range [2]bool{true, false} {
				var refs []viprRowMult
				valid := true
				for i := range mActive {
					if u[i] == nil || u[i].Sign() == 0 {
						continue
					}
					gID := r.activeRowID(i)
					sense, _, _, _ := r.rowData(gID)
					mVal := new(big.Rat).Set(u[i])
					if orient < 0 {
						mVal.Neg(mVal)
					}
					hasCont := r.rowHasContCols(gID)
					switch sense {
					case LessEqual:
						if mVal.Sign() < 0 {
							if hasCont {
								valid = false
							} else {
								mVal.Sub(mVal, viprRatFloor(mVal))
							}
						} else if reduceFrac && !hasCont {
							mVal.Sub(mVal, viprRatFloor(mVal))
						}
					case GreaterEqual:
						if mVal.Sign() > 0 {
							if hasCont {
								valid = false
							} else {
								mVal.Sub(mVal, viprRatCeil(mVal))
							}
						} else if reduceFrac && !hasCont {
							mVal.Sub(mVal, viprRatCeil(mVal))
						}
					case Equal:
						if reduceFrac && !hasCont {
							mVal.Sub(mVal, viprRatFloor(mVal))
						}
					}
					if !valid {
						break
					}
					if mVal.Sign() != 0 {
						refs = append(refs, viprRowMult{rowID: gID, mult: mVal})
					}
				}
				if !valid || len(refs) == 0 {
					continue
				}
				if cc, okCut := r.buildCertifiedCGCut(refs, lpPrimal, maxNNZ, minEff); okCut {
					out = append(out, cc)
					break
				}
			}
		}
	}
	return out
}

func (r *babVIPRRecorder) separateCertifiedVUBCuts(lpPrimal []float64, minEff float64, ws *cutGenWorkspace) []viprCertifiedCut {
	rp := r.rp
	p := rp.p
	cache := ws.ensureStaticCache(p)
	if len(cache.vubCapRows) == 0 {
		return nil
	}
	var out []viprCertifiedCut
	for _, rowIdx := range cache.vubCapRows {
		if rp.rowSense[rowIdx] != LessEqual || rp.rowRHS[rowIdx].Sign() > 0 {
			continue
		}
		cols := rp.rowCols[rowIdx]
		vals := rp.rowVals[rowIdx]
		binCol := -1
		var capRat *big.Rat
		negCount := 0
		allPosNonNeg := true
		for k, col := range cols {
			v := vals[k]
			if v.Sign() < 0 {
				negCount++
				if p.varCat[col] == Binary && rp.lb[col] != nil && rp.ub[col] != nil &&
					rp.lb[col].Sign() == 0 && rp.ub[col].Cmp(big.NewRat(1, 1)) == 0 {
					binCol = col
					capRat = new(big.Rat).Neg(v)
				}
			} else if v.Sign() > 0 {
				if rp.lb[col] == nil || rp.lb[col].Sign() < 0 {
					allPosNonNeg = false
				}
			}
		}
		if negCount != 1 || binCol < 0 || capRat == nil || !allPosNonNeg {
			continue
		}

		for k, col := range cols {
			if col == binCol || vals[k].Sign() <= 0 {
				continue
			}
			ak := vals[k]
			ratio := new(big.Rat).Quo(capRat, ak)
			if !viprRatDenomOK(ratio) {
				continue
			}
			invA := new(big.Rat).Inv(ak)
			if !viprRatDenomOK(invA) {
				continue
			}
			var leftBounds []viprBoundTerm
			rhsL := new(big.Rat).Mul(rp.rowRHS[rowIdx], invA)
			term := new(big.Rat)
			boundsOK := true
			for k2, col2 := range cols {
				if col2 == col || col2 == binCol || vals[k2].Sign() == 0 {
					continue
				}
				if rp.lb[col2] == nil {
					boundsOK = false
					break
				}
				m2 := new(big.Rat).Mul(vals[k2], invA)
				m2.Neg(m2)
				if m2.Sign() > 0 || !viprRatDenomOK(m2) {
					boundsOK = false
					break
				}
				leftBounds = append(leftBounds, viprBoundTerm{col: col2, isUpper: false, mult: m2})
				term.Mul(m2, rp.lb[col2])
				rhsL.Add(rhsL, term)
			}
			if !boundsOK || !viprRatDenomOK(rhsL) {
				continue
			}

			var cutCols []int
			var cutVals []*big.Rat
			var cutRHS *big.Rat
			var gc viprGlobalCut

			if rp.ub[col] == nil || ratio.Cmp(rp.ub[col]) <= 0 {
				negRatio := new(big.Rat).Neg(ratio)
				if col < binCol {
					cutCols = []int{col, binCol}
					cutVals = []*big.Rat{big.NewRat(1, 1), negRatio}
				} else {
					cutCols = []int{binCol, col}
					cutVals = []*big.Rat{negRatio, big.NewRat(1, 1)}
				}
				cutRHS = rhsL
				gc = viprGlobalCut{
					cols:   cutCols,
					vals:   cutVals,
					rhs:    cutRHS,
					kind:   viprCutLinComb,
					refs:   []viprRowMult{{rowID: rowIdx, mult: invA}},
					bounds: leftBounds,
				}
			} else {
				uk := new(big.Rat).Set(rp.ub[col])
				if uk.Sign() <= 0 {
					continue
				}
				delta := new(big.Rat).Sub(ratio, uk)
				if delta.Sign() <= 0 || !viprRatDenomOK(delta) {
					continue
				}
				negUK := new(big.Rat).Neg(uk)
				if col < binCol {
					cutCols = []int{col, binCol}
					cutVals = []*big.Rat{big.NewRat(1, 1), negUK}
				} else {
					cutCols = []int{binCol, col}
					cutVals = []*big.Rat{negUK, big.NewRat(1, 1)}
				}
				cutRHS = new(big.Rat)
				if rhsL.Sign() > 0 {
					cutRHS.Set(rhsL)
				}
				gc = viprGlobalCut{
					cols:          cutCols,
					vals:          cutVals,
					rhs:           cutRHS,
					kind:          viprCutVUBBranch,
					vubCol:        col,
					vubBinCol:     binCol,
					vubRowID:      rowIdx,
					vubInvA:       invA,
					vubDelta:      delta,
					vubUB:         uk,
					vubLeftBounds: leftBounds,
				}
			}

			plane, normExp, okF := buildFloatCutFromRat(rp, cutCols, cutVals, cutRHS, lpPrimal, 0, minEff)
			if !okF {
				continue
			}
			gc.normExp = normExp
			if r.verifyGlobalCutRat(gc) {
				out = append(out, viprCertifiedCut{plane: plane, cut: gc})
			}
		}
	}
	return out
}

func (r *babVIPRRecorder) separateCertifiedRowCGCuts(lpPrimal []float64, minEff float64, ws *cutGenWorkspace) []viprCertifiedCut {
	rp := r.rp
	p := rp.p
	cache := ws.ensureStaticCache(p)
	if len(cache.mirRows) == 0 {
		return nil
	}
	mirMaxNNZ := maxCutNNZ(rp.n)
	var out []viprCertifiedCut

	for _, rowIdx := range cache.mirRows {
		cols := rp.rowCols[rowIdx]
		vals := rp.rowVals[rowIdx]
		if len(cols) < minPairNNZ {
			continue
		}
		signF := 1.0
		signRat := big.NewRat(1, 1)
		if rp.rowSense[rowIdx] == GreaterEqual {
			signF = -1.0
			signRat = big.NewRat(-1, 1)
		}
		rhsF := signF * p.rowRHS[rowIdx]
		act := 0.0
		for k, col := range cols {
			vf, _ := vals[k].Float64()
			act += signF * vf * lpPrimal[col]
		}
		if rhsF-act > 1.0+mirMaxRelSlack*math.Abs(rhsF) {
			continue
		}

		var triedDeltas []*big.Rat
		for kDiv, divCol := range cols {
			if len(triedDeltas) >= maxMIRDivisors {
				break
			}
			if !rp.isInt[divCol] {
				continue
			}
			xv := lpPrimal[divCol]
			fx := xv - math.Floor(xv)
			if fx < minIntInfeas || fx > 1.0-minIntInfeas {
				continue
			}
			delta := new(big.Rat).Abs(vals[kDiv])
			if delta.Sign() <= 0 {
				continue
			}
			dup := false
			for _, td := range triedDeltas {
				if td.Cmp(delta) == 0 {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			triedDeltas = append(triedDeltas, delta)
			mult := new(big.Rat).Quo(signRat, delta)
			if cc, okCut := r.buildCertifiedCGCut([]viprRowMult{{rowID: rowIdx, mult: mult}}, lpPrimal, mirMaxNNZ, minEff); okCut {
				out = append(out, cc)
			}
		}
	}
	return out
}

func (r *babVIPRRecorder) separateCertifiedCoverCuts(lpPrimal []float64, minEff float64, ws *cutGenWorkspace) []viprCertifiedCut {
	rp := r.rp
	p := rp.p
	cache := ws.ensureStaticCache(p)
	if len(cache.coverRows) == 0 {
		return nil
	}
	zeroRat := new(big.Rat)
	oneRat := big.NewRat(1, 1)

	type ratCoverItem struct {
		col    int
		weight *big.Rat
		xVal   float64
		comp   bool
	}

	var out []viprCertifiedCut
	for _, rowIdx := range cache.coverRows {
		cols := rp.rowCols[rowIdx]
		vals := rp.rowVals[rowIdx]
		if len(cols) < 3 {
			continue
		}
		signRat := oneRat
		if rp.rowSense[rowIdx] == GreaterEqual {
			signRat = big.NewRat(-1, 1)
		}
		effRHS := new(big.Rat).Mul(signRat, rp.rowRHS[rowIdx])
		term := new(big.Rat)
		items := make([]ratCoverItem, 0, len(cols))
		var nonBinCols []int
		var nonBinEff []*big.Rat
		valid := true

		for k, col := range cols {
			aEff := new(big.Rat).Mul(signRat, vals[k])
			if aEff.Sign() == 0 {
				continue
			}
			isBin := p.varCat[col] == Binary && rp.lb[col] != nil && rp.ub[col] != nil &&
				rp.lb[col].Cmp(zeroRat) == 0 && rp.ub[col].Cmp(oneRat) == 0
			if !isBin {
				if aEff.Sign() > 0 {
					if rp.lb[col] == nil {
						valid = false
						break
					}
					term.Mul(aEff, rp.lb[col])
					effRHS.Sub(effRHS, term)
				} else {
					if rp.ub[col] == nil {
						valid = false
						break
					}
					term.Mul(aEff, rp.ub[col])
					effRHS.Sub(effRHS, term)
				}
				nonBinCols = append(nonBinCols, col)
				nonBinEff = append(nonBinEff, aEff)
				continue
			}
			if aEff.Sign() > 0 {
				items = append(items, ratCoverItem{col: col, weight: aEff, xVal: lpPrimal[col], comp: false})
			} else {
				w := new(big.Rat).Neg(aEff)
				effRHS.Add(effRHS, w)
				items = append(items, ratCoverItem{col: col, weight: w, xVal: 1.0 - lpPrimal[col], comp: true})
			}
		}
		if !valid || len(items) < 3 || effRHS.Sign() <= 0 {
			continue
		}

		slices.SortFunc(items, func(a, b ratCoverItem) int {
			if math.Abs(a.xVal-b.xVal) > cutCompareTol {
				return cmp.Compare(b.xVal, a.xVal)
			}
			return b.weight.Cmp(a.weight)
		})

		inCover := make([]bool, len(items))
		coverW := new(big.Rat)
		coverSize := 0
		for idx, it := range items {
			if it.xVal <= minCoverItemVal {
				break
			}
			inCover[idx] = true
			coverW.Add(coverW, it.weight)
			coverSize++
			if coverW.Cmp(effRHS) > 0 {
				break
			}
		}
		if coverW.Cmp(effRHS) <= 0 || coverSize < minPairNNZ {
			continue
		}
		candW := new(big.Rat)
		for idx := len(items) - 1; idx >= 0; idx-- {
			if !inCover[idx] {
				continue
			}
			candW.Sub(coverW, items[idx].weight)
			if candW.Cmp(effRHS) > 0 {
				inCover[idx] = false
				coverW.Set(candW)
				coverSize--
			}
		}
		if coverSize < minPairNNZ {
			continue
		}

		var maxCoverW *big.Rat
		for idx, it := range items {
			if inCover[idx] && (maxCoverW == nil || it.weight.Cmp(maxCoverW) > 0) {
				maxCoverW = it.weight
			}
		}
		if maxCoverW == nil || maxCoverW.Sign() <= 0 {
			continue
		}

		rowMult := new(big.Rat).Quo(signRat, maxCoverW)
		if !viprRatDenomOK(rowMult) {
			continue
		}
		bounds := make([]viprBoundTerm, 0, len(cols))
		linRHS := new(big.Rat).Mul(rowMult, rp.rowRHS[rowIdx])
		type cutPair struct {
			col int
			val *big.Rat
		}
		pairs := make([]cutPair, 0, len(items))

		for k, col := range nonBinCols {
			scaled := new(big.Rat).Quo(nonBinEff[k], maxCoverW)
			if scaled.Sign() == 0 {
				continue
			}
			negScaled := new(big.Rat).Neg(scaled)
			if scaled.Sign() > 0 {
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: false, mult: negScaled})
				term.Mul(negScaled, rp.lb[col])
				linRHS.Add(linRHS, term)
			} else {
				bounds = append(bounds, viprBoundTerm{col: col, isUpper: true, mult: negScaled})
				term.Mul(negScaled, rp.ub[col])
				linRHS.Add(linRHS, term)
			}
		}

		for idx, it := range items {
			scaled := new(big.Rat).Quo(it.weight, maxCoverW)
			var alpha *big.Rat
			if inCover[idx] {
				alpha = oneRat
				diff := new(big.Rat).Sub(oneRat, scaled)
				if diff.Sign() > 0 {
					if !it.comp {
						bounds = append(bounds, viprBoundTerm{col: it.col, isUpper: true, mult: diff})
						linRHS.Add(linRHS, diff)
					} else {
						negDiff := new(big.Rat).Neg(diff)
						bounds = append(bounds, viprBoundTerm{col: it.col, isUpper: false, mult: negDiff})
					}
				}
			} else {
				alpha = viprRatFloor(scaled)
				diff := new(big.Rat).Sub(alpha, scaled) // <= 0
				if diff.Sign() < 0 {
					if !it.comp {
						bounds = append(bounds, viprBoundTerm{col: it.col, isUpper: false, mult: diff})
					} else {
						posDiff := new(big.Rat).Neg(diff)
						bounds = append(bounds, viprBoundTerm{col: it.col, isUpper: true, mult: posDiff})
						linRHS.Add(linRHS, posDiff)
					}
				}
			}
			if alpha.Sign() > 0 {
				val := new(big.Rat).Set(alpha)
				if it.comp {
					val.Neg(val)
				}
				pairs = append(pairs, cutPair{col: it.col, val: val})
			}
		}

		if len(pairs) == 0 || !viprRatDenomOK(linRHS) {
			continue
		}
		roundedRHS := viprRatFloor(linRHS)
		if roundedRHS.Cmp(linRHS) >= 0 {
			continue
		}
		slices.SortFunc(pairs, func(a, b cutPair) int { return cmp.Compare(a.col, b.col) })
		cutCols := make([]int, len(pairs))
		cutVals := make([]*big.Rat, len(pairs))
		for k, pr := range pairs {
			cutCols[k] = pr.col
			cutVals[k] = pr.val
		}
		plane, normExp, okF := buildFloatCutFromRat(rp, cutCols, cutVals, roundedRHS, lpPrimal, 0, minEff)
		if !okF {
			continue
		}
		gc := viprGlobalCut{
			cols:    cutCols,
			vals:    cutVals,
			rhs:     roundedRHS,
			normExp: normExp,
			kind:    viprCutCG,
			refs:    []viprRowMult{{rowID: rowIdx, mult: rowMult}},
			bounds:  bounds,
		}
		if r.verifyGlobalCutRat(gc) {
			out = append(out, viprCertifiedCut{plane: plane, cut: gc})
		}
	}
	return out
}

func selectCertifiedCutBatch(candidates []viprCertifiedCut, nVars, maxCuts int, minEfficacy float64) []viprCertifiedCut {
	if len(candidates) == 0 {
		return nil
	}
	if minEfficacy > minCutEfficacy {
		kept := candidates[:0]
		for _, c := range candidates {
			if c.plane.Efficacy >= minEfficacy {
				kept = append(kept, c)
			}
		}
		candidates = kept
		if len(candidates) == 0 {
			return nil
		}
	}
	slices.SortFunc(candidates, func(a, b viprCertifiedCut) int {
		if c := cmp.Compare(cutSelectionScore(b.plane, nVars), cutSelectionScore(a.plane, nVars)); c != 0 {
			return c
		}
		return cmp.Compare(b.plane.Efficacy, a.plane.Efficacy)
	})
	selected := make([]viprCertifiedCut, 0, min(maxCuts, len(candidates)))
	for _, cand := range candidates {
		if len(selected) >= maxCuts {
			break
		}
		tooParallel := false
		for _, sel := range selected {
			if cosineSimilarity(cand.plane, sel.plane) > maxOrthoCosine {
				tooParallel = true
				break
			}
		}
		if !tooParallel {
			selected = append(selected, cand)
		}
	}
	return selected
}

func (r *babVIPRRecorder) generateCertifiedCutRound(
	s *LPSolver,
	lpPrimal []float64,
	maxCuts int,
	minEff float64,
	ws *cutGenWorkspace,
) []viprCertifiedCut {
	if r == nil || r.invalid || s == nil || s.timeExpired() {
		return nil
	}
	if maxCuts <= 0 {
		maxCuts = defaultMaxCuts
	}
	if ws == nil {
		var localWS cutGenWorkspace
		ws = &localWS
	}
	var cands []viprCertifiedCut
	cands = append(cands, r.separateCertifiedTableauCG(s, lpPrimal, minEff)...)
	if s.timeExpired() {
		return nil
	}
	cands = append(cands, r.separateCertifiedCoverCuts(lpPrimal, minEff, ws)...)
	if s.timeExpired() {
		return nil
	}
	cands = append(cands, r.separateCertifiedVUBCuts(lpPrimal, minEff, ws)...)
	if s.timeExpired() {
		return nil
	}
	cands = append(cands, r.separateCertifiedRowCGCuts(lpPrimal, minEff, ws)...)
	if s.timeExpired() {
		return nil
	}
	return selectCertifiedCutBatch(cands, r.rp.n, maxCuts, minEff)
}

func (r *babVIPRRecorder) checkNodeBoundConflict(vn *viprTreeNode) bool {
	if r == nil || r.invalid || vn == nil {
		return false
	}
	lbR, ubR := r.nodeBoundsRat(vn)
	for j := range r.rp.n {
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return true
		}
	}
	return false
}

func (r *babVIPRRecorder) propagateRowSide(
	vn *viprTreeNode,
	rowIdx int,
	isUpperSide bool,
	lbR, ubR []*big.Rat,
	curLB, curUB []float64,
	term, num *big.Rat,
) (feasible, changed bool) {
	rp := r.rp
	gID := r.activeRowID(rowIdx)
	_, rowRHS, cols, vals := r.rowData(gID)

	infCnt := 0
	act := new(big.Rat)
	for k, col := range cols {
		a := vals[k]
		useLB := (a.Sign() > 0) == isUpperSide
		var bd *big.Rat
		if useLB {
			bd = lbR[col]
		} else {
			bd = ubR[col]
		}
		if bd == nil {
			infCnt++
		} else {
			term.Mul(a, bd)
			act.Add(act, term)
		}
	}

	if infCnt == 0 {
		ord := act.Cmp(rowRHS)
		if (isUpperSide && ord > 0) || (!isUpperSide && ord < 0) {
			mult := big.NewRat(1, 1)
			if !isUpperSide {
				mult = big.NewRat(-1, 1)
			}
			vn.kind = viprNodeLeafFarkas
			vn.farkas = []viprRowMult{{rowID: gID, mult: mult}}
			return false, false
		}
	}
	if infCnt > 1 {
		return true, false
	}

	for k, col := range cols {
		a := vals[k]
		useLB := (a.Sign() > 0) == isUpperSide
		var selfBD *big.Rat
		if useLB {
			selfBD = lbR[col]
		} else {
			selfBD = ubR[col]
		}
		if infCnt == 1 && selfBD != nil {
			continue
		}
		num.Sub(rowRHS, act)
		if selfBD != nil {
			term.Mul(a, selfBD)
			num.Add(num, term)
		}
		cand := new(big.Rat).Quo(num, a)
		targetUpper := useLB
		if rp.isInt[col] {
			if targetUpper {
				cand = viprRatFloor(cand)
			} else {
				cand = viprRatCeil(cand)
			}
		} else if cand.Denom().BitLen() > viprMaxRatDenomBits {
			continue
		}
		if targetUpper {
			if ubR[col] == nil || cand.Cmp(ubR[col]) < 0 {
				vn.propSteps = append(vn.propSteps, viprPropStep{
					rowIdx:   gID,
					col:      col,
					isUpper:  true,
					roundInt: rp.isInt[col],
					bound:    cand,
				})
				ubR[col] = cand
				if curUB != nil {
					curUB[col] = viprRatToOutwardFloat(cand, true)
				}
				changed = true
				if lbR[col] != nil && ubR[col].Cmp(lbR[col]) < 0 {
					vn.kind = viprNodeLeafBoundConflict
					vn.conflictCol = col
					return false, changed
				}
			}
		} else if lbR[col] == nil || cand.Cmp(lbR[col]) > 0 {
			vn.propSteps = append(vn.propSteps, viprPropStep{
				rowIdx:   gID,
				col:      col,
				isUpper:  false,
				roundInt: rp.isInt[col],
				bound:    cand,
			})
			lbR[col] = cand
			if curLB != nil {
				curLB[col] = viprRatToOutwardFloat(cand, false)
			}
			changed = true
			if ubR[col] != nil && ubR[col].Cmp(lbR[col]) < 0 {
				vn.kind = viprNodeLeafBoundConflict
				vn.conflictCol = col
				return false, changed
			}
		}
	}
	return true, changed
}

// propagateNode tightens exact rational bounds at vn, appends each deduced
// viprPropStep, and updates curLB/curUB outward.
func (r *babVIPRRecorder) propagateNode(vn *viprTreeNode, curLB, curUB []float64, maxPasses int) (feasible, converged bool) {
	if r == nil || r.invalid || vn == nil {
		return true, false
	}
	rp := r.rp
	lbR, ubR := r.nodeBoundsRat(vn)

	for j := range rp.n {
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return false, false
		}
	}

	for j := range rp.n {
		if !rp.isInt[j] {
			continue
		}
		if lbR[j] != nil && !lbR[j].IsInt() {
			cl := viprRatCeil(lbR[j])
			vn.propSteps = append(vn.propSteps, viprPropStep{
				rowIdx:   -1,
				col:      j,
				isUpper:  false,
				roundInt: true,
				bound:    cl,
			})
			lbR[j] = cl
			if curLB != nil {
				curLB[j] = viprRatToOutwardFloat(cl, false)
			}
		}
		if ubR[j] != nil && !ubR[j].IsInt() {
			fl := viprRatFloor(ubR[j])
			vn.propSteps = append(vn.propSteps, viprPropStep{
				rowIdx:   -1,
				col:      j,
				isUpper:  true,
				roundInt: true,
				bound:    fl,
			})
			ubR[j] = fl
			if curUB != nil {
				curUB[j] = viprRatToOutwardFloat(fl, true)
			}
		}
		if lbR[j] != nil && ubR[j] != nil && ubR[j].Cmp(lbR[j]) < 0 {
			vn.kind = viprNodeLeafBoundConflict
			vn.conflictCol = j
			return false, false
		}
	}

	term := new(big.Rat)
	num := new(big.Rat)
	for range maxPasses {
		anyChanged := false
		for i := range rp.m {
			gID := r.activeRowID(i)
			sense, rowRHS, cols, _ := r.rowData(gID)
			if len(cols) == 0 {
				rhsSign := rowRHS.Sign()
				if (sense == LessEqual || sense == Equal) && rhsSign < 0 {
					vn.kind = viprNodeLeafFarkas
					vn.farkas = []viprRowMult{{rowID: gID, mult: big.NewRat(1, 1)}}
					return false, false
				}
				if (sense == GreaterEqual || sense == Equal) && rhsSign > 0 {
					vn.kind = viprNodeLeafFarkas
					vn.farkas = []viprRowMult{{rowID: gID, mult: big.NewRat(-1, 1)}}
					return false, false
				}
				continue
			}
			if len(cols) > viprMaxPropRowLen {
				continue
			}
			if sense == LessEqual || sense == Equal {
				ok, chg := r.propagateRowSide(vn, i, true, lbR, ubR, curLB, curUB, term, num)
				if !ok {
					return false, false
				}
				if chg {
					anyChanged = true
				}
			}
			if sense == GreaterEqual || sense == Equal {
				ok, chg := r.propagateRowSide(vn, i, false, lbR, ubR, curLB, curUB, term, num)
				if !ok {
					return false, false
				}
				if chg {
					anyChanged = true
				}
			}
		}
		if !anyChanged {
			return true, true
		}
	}
	return true, false
}

type viprAsmMeta struct {
	col     int
	isUpper bool
	bound   *big.Rat
}

type viprEmitter struct {
	rec       *babVIPRRecorder
	rp        *viprRatProblem
	cutStepID []int
	steps     []viprStepJSON
	entryAsms [][]int
	asmMeta   map[int]viprAsmMeta
}

func (em *viprEmitter) canceled() bool {
	return em != nil && em.rec != nil && stopRequested(time.Time{}, em.rec.done)
}

func viprMergeAsms(lists ...[]int) []int {
	total := 0
	for _, ls := range lists {
		total += len(ls)
	}
	if total == 0 {
		return nil
	}
	out := make([]int, 0, total)
	if total > 32 {
		seen := make(map[int]struct{}, total)
		for _, ls := range lists {
			for _, a := range ls {
				if _, ok := seen[a]; !ok {
					seen[a] = struct{}{}
					out = append(out, a)
				}
			}
		}
		return out
	}
	for _, ls := range lists {
		for _, a := range ls {
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}

func viprFilterOutAsms(src []int, drop ...int) []int {
	if len(src) == 0 {
		return nil
	}
	var out []int
	for _, a := range src {
		if !slices.Contains(drop, a) {
			out = append(out, a)
		}
	}
	return out
}

func (em *viprEmitter) getAsms(id int) []int {
	idx := id - em.rp.m
	if idx < 0 || idx >= len(em.entryAsms) {
		return nil
	}
	return em.entryAsms[idx]
}

func (em *viprEmitter) computeStepAsms(id int, st viprStepJSON) []int {
	switch st.Rule {
	case viprRuleAssume:
		return []int{id}
	case viprRuleConIndex:
		return nil
	case viprRuleLinComb, viprRuleFarkas, viprRuleDualBound:
		if st.Refs == nil {
			return nil
		}
		var lists [][]int
		for _, ref := range *st.Refs {
			if a := em.getAsms(ref.ID); len(a) > 0 {
				lists = append(lists, a)
			}
		}
		return viprMergeAsms(lists...)
	case viprRuleRoundDown, viprRuleRoundUp:
		if st.Ref == nil {
			return nil
		}
		return slices.Clone(em.getAsms(*st.Ref))
	case viprRuleBoundConflict:
		var lbA, ubA []int
		if st.LBRef != nil {
			lbA = em.getAsms(*st.LBRef)
		}
		if st.UBRef != nil {
			ubA = em.getAsms(*st.UBRef)
		}
		return viprMergeAsms(lbA, ubA)
	case viprRuleSafeDual:
		var lists [][]int
		if st.Refs != nil {
			for _, ref := range *st.Refs {
				if a := em.getAsms(ref.ID); len(a) > 0 {
					lists = append(lists, a)
				}
			}
		}
		for _, bid := range st.BoundAsms {
			if a := em.getAsms(bid); len(a) > 0 {
				lists = append(lists, a)
			}
		}
		return viprMergeAsms(lists...)
	case viprRuleBranch:
		var leftRem, rightRem []int
		if st.Left != nil && st.LeftAsm != nil {
			leftRem = viprFilterOutAsms(em.getAsms(*st.Left), *st.LeftAsm)
		}
		if st.Right != nil && st.RightAsm != nil {
			rightRem = viprFilterOutAsms(em.getAsms(*st.Right), *st.RightAsm)
		}
		return viprMergeAsms(leftRem, rightRem)
	case viprRuleOrbitBranch:
		var leftRem, rightRem []int
		if st.Left != nil && st.LeftAsm != nil {
			drop := make([]int, 0, 1+len(st.OrbitAsms))
			drop = append(drop, *st.LeftAsm)
			drop = append(drop, st.OrbitAsms...)
			leftRem = viprFilterOutAsms(em.getAsms(*st.Left), drop...)
		}
		if st.Right != nil && st.RightAsm != nil {
			rightRem = viprFilterOutAsms(em.getAsms(*st.Right), *st.RightAsm)
		}
		return viprMergeAsms(leftRem, rightRem, st.ExtraAsms)
	default:
		return nil
	}
}

func (em *viprEmitter) addStep(st viprStepJSON) int {
	id := em.rp.m + len(em.steps)
	asms := em.computeStepAsms(id, st)
	em.steps = append(em.steps, st)
	em.entryAsms = append(em.entryAsms, asms)
	return id
}

func (em *viprEmitter) rowRefID(rowID int) (int, bool) {
	if rowID < em.rp.m {
		return rowID, rowID >= 0
	}
	cutIdx := rowID - em.rp.m
	if cutIdx < 0 || cutIdx >= len(em.cutStepID) || em.cutStepID[cutIdx] < 0 {
		return -1, false
	}
	return em.cutStepID[cutIdx], true
}

func (em *viprEmitter) emitAssume(col int, isUpper bool, bound *big.Int) int {
	sense := ">="
	if isUpper {
		sense = "<="
	}
	id := em.rp.m + len(em.steps)
	if em.asmMeta == nil {
		em.asmMeta = make(map[int]viprAsmMeta)
	}
	em.asmMeta[id] = viprAsmMeta{
		col:     col,
		isUpper: isUpper,
		bound:   new(big.Rat).SetInt(bound),
	}
	return em.addStep(viprStepJSON{
		Rule:        viprRuleAssume,
		Col:         viprIntPtr(col),
		Sense:       sense,
		Bound:       viprBigIntJSON(bound),
		Assumptions: []int{id},
	})
}

func (em *viprEmitter) emitGlobalCut(cut viprGlobalCut) (int, bool) {
	if em.canceled() {
		return -1, false
	}
	toBoundJSON := func(bts []viprBoundTerm) []viprBoundRefJSON {
		if len(bts) == 0 {
			return nil
		}
		out := make([]viprBoundRefJSON, 0, len(bts))
		for _, bt := range bts {
			if bt.mult == nil || bt.mult.Sign() == 0 {
				continue
			}
			kind := "lb"
			if bt.isUpper {
				kind = "ub"
			}
			out = append(out, viprBoundRefJSON{
				Col:  bt.col,
				Kind: kind,
				Mult: viprBigRatToPayload(bt.mult),
			})
		}
		return out
	}

	switch cut.kind {
	case viprCutLinComb, viprCutCG:
		refs := make([]viprLinRefJSON, 0, len(cut.refs))
		for _, rm := range cut.refs {
			if rm.mult == nil || rm.mult.Sign() == 0 {
				continue
			}
			id, ok := em.rowRefID(rm.rowID)
			if !ok {
				return -1, false
			}
			refs = append(refs, viprLinRefJSON{
				ID:   id,
				Mult: viprBigRatToPayload(rm.mult),
			})
		}
		sense := "<="
		roundRule := viprRuleRoundDown
		if cut.kind == viprCutCG && cut.isGe {
			sense = ">="
			roundRule = viprRuleRoundUp
		}
		linID := em.addStep(viprStepJSON{
			Rule:   viprRuleLinComb,
			Sense:  sense,
			Refs:   viprOptRefs(refs),
			Bounds: toBoundJSON(cut.bounds),
		})
		if cut.kind == viprCutCG {
			return em.addStep(viprStepJSON{
				Rule: roundRule,
				Ref:  viprIntPtr(linID),
			}), true
		}
		return linID, true

	case viprCutVUBBranch:
		zeroInt := big.NewInt(0)
		oneInt := big.NewInt(1)
		leftAsmID := em.emitAssume(cut.vubBinCol, true, zeroInt)
		leftRefs := []viprLinRefJSON{
			{ID: cut.vubRowID, Mult: viprBigRatToPayload(cut.vubInvA)},
			{ID: leftAsmID, Mult: viprBigRatToPayload(cut.vubDelta)},
		}
		leftID := em.addStep(viprStepJSON{
			Rule:   viprRuleLinComb,
			Sense:  "<=",
			Refs:   viprOptRefs(leftRefs),
			Bounds: toBoundJSON(cut.vubLeftBounds),
		})

		rightAsmID := em.emitAssume(cut.vubBinCol, false, oneInt)
		negUB := new(big.Rat).Neg(cut.vubUB)
		rightID := em.addStep(viprStepJSON{
			Rule:   viprRuleLinComb,
			Sense:  "<=",
			Refs:   viprOptRefs([]viprLinRefJSON{{ID: rightAsmID, Mult: viprBigRatToPayload(negUB)}}),
			Bounds: []viprBoundRefJSON{{Col: cut.vubCol, Kind: "ub", Mult: viprInt64Payload(1)}},
		})
		return em.addStep(viprStepJSON{
			Rule:     viprRuleBranch,
			Col:      viprIntPtr(cut.vubBinCol),
			SplitVal: viprBigIntJSON(zeroInt),
			LeftAsm:  viprIntPtr(leftAsmID),
			RightAsm: viprIntPtr(rightAsmID),
			Left:     viprIntPtr(leftID),
			Right:    viprIntPtr(rightID),
		}), true

	default:
		return -1, false
	}
}

func appendBoundOrStepRef(
	refs *[]viprLinRefJSON,
	bounds *[]viprBoundRefJSON,
	col int,
	isUpper bool,
	stepID int,
	mult viprRatPayload,
) {
	if stepID >= 0 {
		*refs = append(*refs, viprLinRefJSON{ID: stepID, Mult: mult})
		return
	}
	kind := "lb"
	if isUpper {
		kind = "ub"
	}
	*bounds = append(*bounds, viprBoundRefJSON{Col: col, Kind: kind, Mult: mult})
}

func (em *viprEmitter) emitPropStep(
	ps viprPropStep,
	lbStepID, ubStepID []int,
) (int, bool) {
	var refs []viprLinRefJSON
	var bounds []viprBoundRefJSON
	if ps.rowIdx < 0 {
		stepID := lbStepID[ps.col]
		if ps.isUpper {
			stepID = ubStepID[ps.col]
		}
		appendBoundOrStepRef(&refs, &bounds, ps.col, ps.isUpper, stepID, viprInt64Payload(1))
	} else {
		_, _, cols, vals := em.rec.rowData(ps.rowIdx)
		var aTarget *big.Rat
		for k, col := range cols {
			if col == ps.col {
				aTarget = vals[k]
				break
			}
		}
		if aTarget == nil || aTarget.Sign() == 0 {
			return -1, false
		}
		refID, okRef := em.rowRefID(ps.rowIdx)
		if !okRef {
			return -1, false
		}
		invA := new(big.Rat).Inv(aTarget)
		refs = []viprLinRefJSON{{ID: refID, Mult: viprBigRatToPayload(invA)}}
		scaled := new(big.Rat)
		negScaled := new(big.Rat)
		for k, col := range cols {
			if col == ps.col {
				continue
			}
			scaled.Mul(vals[k], invA)
			if scaled.Sign() == 0 {
				continue
			}
			negScaled.Neg(scaled)
			needLB := (scaled.Sign() > 0) == ps.isUpper
			stepID := ubStepID[col]
			if needLB {
				stepID = lbStepID[col]
			}
			appendBoundOrStepRef(&refs, &bounds, col, !needLB, stepID, viprBigRatToPayload(negScaled))
		}
	}
	sense := ">="
	if ps.isUpper {
		sense = "<="
	}
	linID := em.addStep(viprStepJSON{
		Rule:   viprRuleLinComb,
		Sense:  sense,
		Refs:   viprOptRefs(refs),
		Bounds: bounds,
	})
	if ps.roundInt {
		rule := viprRuleRoundUp
		if ps.isUpper {
			rule = viprRuleRoundDown
		}
		return em.addStep(viprStepJSON{
			Rule: rule,
			Ref:  viprIntPtr(linID),
		}), true
	}
	return linID, true
}

func (em *viprEmitter) emitFarkas(w []viprRowMult, activeLB, activeUB []*big.Rat, lbStepID, ubStepID []int) (int, bool) {
	wProj, v, ok := em.rec.projectFarkasMults(w, activeLB, activeUB)
	if !ok {
		return -1, false
	}
	refs := make([]viprLinRefJSON, 0, len(wProj))
	var bounds []viprBoundRefJSON
	for _, rm := range wProj {
		id, okID := em.rowRefID(rm.rowID)
		if !okID {
			return -1, false
		}
		refs = append(refs, viprLinRefJSON{ID: id, Mult: viprBigRatToPayload(rm.mult)})
	}
	neg := new(big.Rat)
	for j := range em.rp.n {
		if v[j].Sign() == 0 {
			continue
		}
		neg.Neg(v[j])
		useUB := v[j].Sign() < 0
		stepID := lbStepID[j]
		if useUB {
			stepID = ubStepID[j]
		}
		appendBoundOrStepRef(&refs, &bounds, j, useUB, stepID, viprBigRatToPayload(neg))
	}
	return em.addStep(viprStepJSON{
		Rule:   viprRuleFarkas,
		Refs:   viprOptRefs(refs),
		Bounds: bounds,
	}), true
}

func (em *viprEmitter) emitSafeDual(mults []viprRowMult, activeLB, activeUB []*big.Rat, boundAsms []int) (int, *big.Rat, bool) {
	rhs, _, ok := em.rec.evalSafeDualMults(mults, activeLB, activeUB)
	if !ok {
		return -1, nil, false
	}
	refs := make([]viprLinRefJSON, 0, len(mults))
	for _, rm := range mults {
		if rm.mult == nil || rm.mult.Sign() == 0 {
			continue
		}
		id, okID := em.rowRefID(rm.rowID)
		if !okID {
			return -1, nil, false
		}
		refs = append(refs, viprLinRefJSON{ID: id, Mult: viprBigRatToPayload(rm.mult)})
	}
	return em.addStep(viprStepJSON{
		Rule:      viprRuleSafeDual,
		Refs:      viprOptRefs(refs),
		BoundAsms: slices.Clone(boundAsms),
	}), rhs, true
}

func nearestDual(vn *viprTreeNode) ([]viprRowMult, bool) {
	var bestMults []viprRowMult
	var bestRHS *big.Rat
	for cur := vn; cur != nil; cur = cur.parent {
		if cur.bestDualRHS != nil && (bestRHS == nil || cur.bestDualRHS.Cmp(bestRHS) > 0) {
			bestMults = cur.bestDual
			bestRHS = cur.bestDualRHS
		}
	}
	return bestMults, bestRHS != nil
}

type viprEmitState struct {
	lb   []*big.Rat
	ub   []*big.Rat
	lbID []int
	ubID []int
	asms []int
}

func (st viprEmitState) clone() viprEmitState {
	return viprEmitState{
		lb:   slices.Clone(st.lb),
		ub:   slices.Clone(st.ub),
		lbID: slices.Clone(st.lbID),
		ubID: slices.Clone(st.ubID),
		asms: slices.Clone(st.asms),
	}
}

func (st *viprEmitState) tightenUpper(col int, bound *big.Rat, stepID int) {
	if st.ub[col] == nil || bound.Cmp(st.ub[col]) < 0 {
		st.ub[col] = new(big.Rat).Set(bound)
		st.ubID[col] = stepID
	}
}

func (st *viprEmitState) tightenLower(col int, bound *big.Rat, stepID int) {
	if st.lb[col] == nil || bound.Cmp(st.lb[col]) > 0 {
		st.lb[col] = new(big.Rat).Set(bound)
		st.lbID[col] = stepID
	}
}

func (st viprEmitState) withBranchBound(col int, isUpper bool, bound *big.Rat, asmID int) viprEmitState {
	next := st.clone()
	if isUpper {
		next.tightenUpper(col, bound, asmID)
	} else {
		next.tightenLower(col, bound, asmID)
	}
	next.asms = append(next.asms, asmID)
	return next
}

func isCollapsibleFringeBranch(vn *viprTreeNode) bool {
	return vn.left != nil && vn.right != nil &&
		vn.left.kind == viprNodeOpen && len(vn.left.propSteps) == 0 &&
		vn.right.kind == viprNodeOpen && len(vn.right.propSteps) == 0
}

func mergeBranchOutcome(stepID int, leftUnsat, rightUnsat bool, leftRHS, rightRHS *big.Rat) (int, bool, *big.Rat, bool) {
	switch {
	case leftUnsat && rightUnsat:
		return stepID, true, nil, true
	case leftUnsat:
		return stepID, false, rightRHS, true
	case rightUnsat:
		return stepID, false, leftRHS, true
	default:
		if leftRHS.Cmp(rightRHS) <= 0 {
			return stepID, false, leftRHS, true
		}
		return stepID, false, rightRHS, true
	}
}

func (em *viprEmitter) buildOrbitWitnessesJSON(
	witnesses []viprOrbitWitness,
	rightRem []int,
	st viprEmitState,
) ([]viprOrbitWitnessJSON, []int, bool) {
	var extraAsms []int
	wJSON := make([]viprOrbitWitnessJSON, len(witnesses))
	for idx, w := range witnesses {
		var supports []int
		for _, a := range rightRem {
			meta, okM := em.asmMeta[a]
			if !okM {
				return nil, nil, false
			}
			kp := w.colPerm[meta.col]
			if meta.isUpper {
				if em.rp.ub[kp] != nil && em.rp.ub[kp].Cmp(meta.bound) <= 0 {
					continue
				}
			} else if em.rp.lb[kp] != nil && meta.bound.Cmp(em.rp.lb[kp]) <= 0 {
				continue
			}
			if kp == meta.col {
				continue
			}
			sid := st.lbID[kp]
			if meta.isUpper {
				sid = st.ubID[kp]
			}
			if sid < 0 {
				return nil, nil, false
			}
			if !slices.Contains(supports, sid) {
				supports = append(supports, sid)
			}
			for _, dep := range em.getAsms(sid) {
				if !slices.Contains(extraAsms, dep) {
					extraAsms = append(extraAsms, dep)
				}
			}
		}
		wJSON[idx] = viprOrbitWitnessJSON{
			Companion:      w.companion,
			ColPerm:        slices.Clone(w.colPerm),
			RowPerm:        slices.Clone(w.rowPerm),
			SupportEntries: supports,
		}
	}
	return wJSON, extraAsms, true
}

func (em *viprEmitter) emitNode(
	vn *viprTreeNode,
	inState viprEmitState,
) (stepID int, unsat bool, boundRHS *big.Rat, ok bool) {
	if vn == nil || em.canceled() {
		return -1, false, nil, false
	}
	st := inState.clone()

	for _, ps := range vn.propSteps {
		pid, pOK := em.emitPropStep(ps, st.lbID, st.ubID)
		if !pOK {
			return -1, false, nil, false
		}
		if ps.isUpper {
			st.tightenUpper(ps.col, ps.bound, pid)
		} else {
			st.tightenLower(ps.col, ps.bound, pid)
		}
		st.asms = append(st.asms, pid)
	}

	switch vn.kind {
	case viprNodeLeafBoundConflict:
		col := vn.conflictCol
		if col < 0 || col >= em.rp.n || st.lb[col] == nil || st.ub[col] == nil || st.ub[col].Cmp(st.lb[col]) >= 0 {
			return -1, false, nil, false
		}
		id := em.addStep(viprStepJSON{
			Rule:  viprRuleBoundConflict,
			Col:   viprIntPtr(col),
			LBRef: viprOptNonNegPtr(st.lbID[col]),
			UBRef: viprOptNonNegPtr(st.ubID[col]),
		})
		return id, true, nil, true

	case viprNodeLeafFarkas:
		id, fOK := em.emitFarkas(vn.farkas, st.lb, st.ub, st.lbID, st.ubID)
		return id, true, nil, fOK

	case viprNodeLeafDual, viprNodeOpen:
		mults, hasD := nearestDual(vn)
		if !hasD {
			return -1, false, nil, false
		}
		id, rhs, dOK := em.emitSafeDual(mults, st.lb, st.ub, st.asms)
		return id, false, rhs, dOK

	case viprNodeBranch:
		// Collapse fringe branches whose children were never evaluated to keep proofs compact.
		if isCollapsibleFringeBranch(vn) {
			if mults, hasD := nearestDual(vn); hasD {
				id, rhs, dOK := em.emitSafeDual(mults, st.lb, st.ub, st.asms)
				return id, false, rhs, dOK
			}
		}

		leftAsmID := em.emitAssume(vn.branchCol, true, vn.splitVal)
		leftState := st.withBranchBound(vn.branchCol, true, new(big.Rat).SetInt(vn.splitVal), leftAsmID)
		leftID, leftUnsat, leftRHS, leftOK := em.emitNode(vn.left, leftState)
		if !leftOK {
			return -1, false, nil, false
		}

		splitPlus := new(big.Int).Add(vn.splitVal, big.NewInt(1))
		rightAsmID := em.emitAssume(vn.branchCol, false, splitPlus)
		rightState := st.withBranchBound(vn.branchCol, false, new(big.Rat).SetInt(splitPlus), rightAsmID)
		rightID, rightUnsat, rightRHS, rightOK := em.emitNode(vn.right, rightState)
		if !rightOK {
			return -1, false, nil, false
		}

		brID := em.addStep(viprStepJSON{
			Rule:     viprRuleBranch,
			Col:      viprIntPtr(vn.branchCol),
			SplitVal: viprBigIntJSON(vn.splitVal),
			LeftAsm:  viprIntPtr(leftAsmID),
			RightAsm: viprIntPtr(rightAsmID),
			Left:     viprIntPtr(leftID),
			Right:    viprIntPtr(rightID),
		})
		return mergeBranchOutcome(brID, leftUnsat, rightUnsat, leftRHS, rightRHS)

	case viprNodeOrbitBranch:
		if isCollapsibleFringeBranch(vn) {
			if mults, hasD := nearestDual(vn); hasD {
				id, rhs, dOK := em.emitSafeDual(mults, st.lb, st.ub, st.asms)
				return id, false, rhs, dOK
			}
		}

		zeroInt := big.NewInt(0)
		oneInt := big.NewInt(1)
		zeroRat := new(big.Rat)
		leftAsmID := em.emitAssume(vn.branchCol, true, zeroInt)
		leftState := st.withBranchBound(vn.branchCol, true, zeroRat, leftAsmID)
		orbitAsmIDs := make([]int, len(vn.orbitWitnesses))
		for idx, w := range vn.orbitWitnesses {
			oid := em.emitAssume(w.companion, true, zeroInt)
			orbitAsmIDs[idx] = oid
			leftState.tightenUpper(w.companion, zeroRat, oid)
			leftState.asms = append(leftState.asms, oid)
		}
		leftID, leftUnsat, leftRHS, leftOK := em.emitNode(vn.left, leftState)
		if !leftOK {
			return -1, false, nil, false
		}

		rightAsmID := em.emitAssume(vn.branchCol, false, oneInt)
		rightState := st.withBranchBound(vn.branchCol, false, big.NewRat(1, 1), rightAsmID)
		rightID, rightUnsat, rightRHS, rightOK := em.emitNode(vn.right, rightState)
		if !rightOK {
			return -1, false, nil, false
		}

		rightRem := viprFilterOutAsms(em.getAsms(rightID), rightAsmID)
		wJSON, extraAsms, okW := em.buildOrbitWitnessesJSON(vn.orbitWitnesses, rightRem, st)
		if !okW {
			return -1, false, nil, false
		}

		obID := em.addStep(viprStepJSON{
			Rule:      viprRuleOrbitBranch,
			Col:       viprIntPtr(vn.branchCol),
			LeftAsm:   viprIntPtr(leftAsmID),
			RightAsm:  viprIntPtr(rightAsmID),
			Left:      viprIntPtr(leftID),
			Right:     viprIntPtr(rightID),
			OrbitAsms: orbitAsmIDs,
			Witnesses: wJSON,
			ExtraAsms: extraAsms,
		})
		return mergeBranchOutcome(obID, leftUnsat, rightUnsat, leftRHS, rightRHS)

	default:
		return -1, false, nil, false
	}
}

func (r *babVIPRRecorder) collectNeededCuts() []bool {
	nCuts := len(r.allCuts)
	if nCuts == 0 {
		return nil
	}
	need := make([]bool, nCuts)
	baseM := r.rp.m
	markMults := func(mults []viprRowMult) {
		for _, rm := range mults {
			if rm.mult != nil && rm.mult.Sign() != 0 && rm.rowID >= baseM {
				if cutIdx := rm.rowID - baseM; cutIdx < nCuts {
					need[cutIdx] = true
				}
			}
		}
	}
	var walk func(vn *viprTreeNode)
	walk = func(vn *viprTreeNode) {
		if vn == nil {
			return
		}
		for _, ps := range vn.propSteps {
			if ps.rowIdx >= baseM {
				if cutIdx := ps.rowIdx - baseM; cutIdx < nCuts {
					need[cutIdx] = true
				}
			}
		}
		markMults(vn.bestDual)
		markMults(vn.farkas)
		walk(vn.left)
		walk(vn.right)
	}
	walk(r.root)
	// Transitive closure over cut-to-cut derivations (cuts only reference strictly earlier cuts).
	for k := nCuts - 1; k >= 0; k-- {
		if !need[k] {
			continue
		}
		for _, rm := range r.allCuts[k].refs {
			if rm.mult != nil && rm.mult.Sign() != 0 && rm.rowID >= baseM {
				if dep := rm.rowID - baseM; dep < k {
					need[dep] = true
				}
			}
		}
	}
	return need
}

func (r *babVIPRRecorder) collectNeededCutsForMults(mults []viprRowMult) []bool {
	nCuts := len(r.allCuts)
	if nCuts == 0 {
		return nil
	}
	need := make([]bool, nCuts)
	baseM := r.rp.m
	anyNeed := false
	for _, rm := range mults {
		if rm.mult != nil && rm.mult.Sign() != 0 && rm.rowID >= baseM {
			if cutIdx := rm.rowID - baseM; cutIdx < nCuts {
				need[cutIdx] = true
				anyNeed = true
			}
		}
	}
	if !anyNeed {
		return nil
	}
	for k := nCuts - 1; k >= 0; k-- {
		if !need[k] {
			continue
		}
		for _, rm := range r.allCuts[k].refs {
			if rm.mult != nil && rm.mult.Sign() != 0 && rm.rowID >= baseM {
				if dep := rm.rowID - baseM; dep < k {
					need[dep] = true
				}
			}
		}
	}
	return need
}

func (r *babVIPRRecorder) emitRootDualFallback() (*viprEmitter, int, *big.Rat, bool) {
	if r == nil || r.rp == nil || r.root == nil || r.root.bestDualRHS == nil {
		return nil, -1, nil, false
	}
	rp := r.rp
	em := &viprEmitter{
		rec:       r,
		rp:        rp,
		cutStepID: make([]int, len(r.allCuts)),
	}
	for k := range em.cutStepID {
		em.cutStepID[k] = -1
	}
	if need := r.collectNeededCutsForMults(r.root.bestDual); need != nil {
		for k, want := range need {
			if !want {
				continue
			}
			sid, okCut := em.emitGlobalCut(r.allCuts[k])
			if !okCut {
				return nil, -1, nil, false
			}
			em.cutStepID[k] = sid
		}
	}
	rootID, rootRHS, okD := em.emitSafeDual(r.root.bestDual, rp.lb, rp.ub, nil)
	if !okD || len(em.steps) == 0 || rootID < rp.m {
		return nil, -1, nil, false
	}
	return em, rootID, rootRHS, true
}

func (r *babVIPRRecorder) finalize(res Result) {
	if r == nil || r.trace == nil || r.rp == nil || r.root == nil || stopRequested(time.Time{}, r.done) {
		return
	}
	if r.invalid && (!isLimitStatus(res.Status) || r.root.bestDualRHS == nil) {
		return
	}
	defer r.close()

	// Clear solver time-limit deadline so post-solve emission and exact refinement
	// finish cleanly while still honoring context cancellation via r.done.
	r.deadline = time.Time{}
	if r.baseScratchLP != nil {
		r.baseScratchLP.Deadline = time.Time{}
	}
	if r.cutScratchLP != nil {
		r.cutScratchLP.Deadline = time.Time{}
	}

	rp := r.rp
	r.trace.rootCutsCertified = r.rootCutsCertified
	r.trace.inTreeCutsCertified = r.inTreeCutsCertified
	r.trace.rcfFixingsCertified = r.rcfFixingsCertified
	r.trace.unbTightCertified = r.unbTightCertified
	r.trace.orbitBranchesCertified = r.orbitBranchesCertified

	var incRat []*big.Rat
	var incObjNoConst *big.Rat
	var hasExactInc bool
	if len(res.Values) == rp.n {
		var ptr *float64
		if len(res.Values) > 0 {
			ptr = &res.Values[0]
		}
		if r.cachedIncPtr == ptr && r.cachedIncOK {
			incRat = r.cachedIncRat
			incObjNoConst = r.cachedIncObj
			hasExactInc = true
		} else {
			incRat, incObjNoConst, hasExactInc = rp.refineExactIncumbent(res.Values, r.ensureBaseScratchLP)
		}
	}

	if !r.invalid && res.Status == StatusOptimal && hasExactInc && !r.exactTreeGapClosed(res.Values) {
		r.polishNearClosedLeavesExact(incObjNoConst)
	}

	var em *viprEmitter
	var rootID int
	var rootUnsat bool
	var rootRHS *big.Rat
	var ok bool

	if !r.invalid {
		lbStepID := make([]int, rp.n)
		ubStepID := make([]int, rp.n)
		for j := range rp.n {
			lbStepID[j] = -1
			ubStepID[j] = -1
		}

		treeEm := &viprEmitter{
			rec:       r,
			rp:        rp,
			cutStepID: make([]int, len(r.allCuts)),
		}
		for k := range treeEm.cutStepID {
			treeEm.cutStepID[k] = -1
		}
		cutsOK := true
		if need := r.collectNeededCuts(); need != nil {
			for k, want := range need {
				if !want {
					continue
				}
				sid, okCut := treeEm.emitGlobalCut(r.allCuts[k])
				if !okCut {
					cutsOK = false
					break
				}
				treeEm.cutStepID[k] = sid
			}
		}
		if cutsOK {
			rootID, rootUnsat, rootRHS, ok = treeEm.emitNode(r.root, viprEmitState{
				lb:   rp.lb,
				ub:   rp.ub,
				lbID: lbStepID,
				ubID: ubStepID,
			})
			if ok && len(treeEm.steps) > 0 && rootID >= rp.m {
				em = treeEm
			} else {
				ok = false
			}
		}
	}

	if !ok {
		if isLimitStatus(res.Status) || r.root.bestDualRHS != nil {
			fbEm, fbID, fbRHS, fbOK := r.emitRootDualFallback()
			if !fbOK {
				return
			}
			em = fbEm
			rootID = fbID
			rootUnsat = false
			rootRHS = fbRHS
		} else {
			return
		}
	} else if !rootUnsat && r.root.bestDualRHS != nil && rootRHS.Cmp(r.root.bestDualRHS) <= 0 {
		if fbEm, fbID, fbRHS, fbOK := r.emitRootDualFallback(); fbOK && fbRHS.Cmp(rootRHS) >= 0 {
			em = fbEm
			rootID = fbID
			rootRHS = fbRHS
		}
	}

	if rootUnsat {
		cert := rp.buildBaseCert(viprModeInfeasible)
		cert.Steps = em.steps
		cert.RootStep = rootID - rp.m
		r.trace.cert = cert
		r.trace.attachPresolveEnvelope(rp.p)
		r.trace.targetLB = nil
		r.trace.hasCert = true
		return
	}

	targetLB := new(big.Rat).Add(rp.objConst, rootRHS)

	mode := viprModeBound
	if hasExactInc && incObjNoConst.Cmp(rootRHS) <= 0 {
		mode = viprModeOptimal
	}

	cert := rp.buildBaseCert(mode)
	cert.Steps = em.steps
	cert.RootStep = rootID - rp.m
	if hasExactInc {
		incPay := make([]viprRatPayload, rp.n)
		for j := range rp.n {
			incPay[j] = viprBigRatToPayload(incRat[j])
		}
		cert.Incumbent = incPay
	}
	if mode == viprModeBound {
		lbPay := viprBigRatToPayload(targetLB)
		cert.TargetLB = &lbPay
	}
	r.trace.cert = cert
	r.trace.attachPresolveEnvelope(rp.p)
	r.trace.targetLB = targetLB
	r.trace.hasCert = true
}

func (r *babVIPRRecorder) certifyRootInfeasible(s *LPSolver, lpRes LPResult) {
	if r == nil || r.invalid {
		return
	}
	r.certifyNodeFarkas(r.root, s, lpRes)
	r.finalize(Result{Status: StatusInfeasible})
}

func (tr *VIPRTrace) recordRootInfeasibleEqRow(p *CompiledProblem) {
	if tr == nil || p == nil {
		return
	}
	tr.reset(p)
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return
	}
	steps, rootStep, ok := buildGCDInfeasSteps(rp)
	if !ok {
		return
	}
	cert := rp.buildBaseCert(viprModeInfeasible)
	cert.Steps = steps
	cert.RootStep = rootStep
	tr.cert = cert
	tr.attachPresolveEnvelope(p)
	tr.hasCert = true
}

func buildGCDInfeasSteps(rp *viprRatProblem) ([]viprStepJSON, int, bool) {
	var gcdSc exactGCDScratch
	for i := range rp.m {
		cols := rp.rowCols[i]
		vals := rp.rowVals[i]
		if len(cols) == 0 {
			sgn := rp.rowRHS[i].Sign()
			infeas := (rp.rowSense[i] == LessEqual && sgn < 0) ||
				(rp.rowSense[i] == GreaterEqual && sgn > 0) ||
				(rp.rowSense[i] == Equal && sgn != 0)
			if !infeas {
				continue
			}
			mult := int64(1)
			if rp.rowSense[i] == GreaterEqual || (rp.rowSense[i] == Equal && sgn > 0) {
				mult = -1
			}
			return []viprStepJSON{
				{
					Rule: viprRuleFarkas,
					Refs: viprOptRefs([]viprLinRefJSON{{ID: i, Mult: viprInt64Payload(mult)}}),
				},
			}, 0, true
		}
		if rp.rowSense[i] != Equal {
			continue
		}
		allInt := true
		for _, col := range cols {
			if !rp.isInt[col] {
				allInt = false
				break
			}
		}
		if !allInt {
			continue
		}
		scaleMult, okScale := gcdSc.rowScale(vals, viprMaxRatDenomBits)
		if !okScale {
			continue
		}
		rhsScaled := new(big.Rat).Mul(rp.rowRHS[i], scaleMult)
		if rhsScaled.IsInt() {
			continue
		}
		multPay := viprBigRatToPayload(scaleMult)
		steps := []viprStepJSON{
			{Rule: viprRuleLinComb, Sense: "<=", Refs: viprOptRefs([]viprLinRefJSON{{ID: i, Mult: multPay}})},
			{Rule: viprRuleRoundDown, Ref: viprIntPtr(rp.m + 0)},
			{Rule: viprRuleLinComb, Sense: ">=", Refs: viprOptRefs([]viprLinRefJSON{{ID: i, Mult: multPay}})},
			{Rule: viprRuleRoundUp, Ref: viprIntPtr(rp.m + 2)},
			{Rule: viprRuleFarkas, Refs: viprOptRefs([]viprLinRefJSON{
				{ID: rp.m + 1, Mult: viprInt64Payload(1)},
				{ID: rp.m + 3, Mult: viprInt64Payload(-1)},
			})},
		}
		return steps, 4, true
	}
	return nil, 0, false
}

// recordPresolveInfeasible marks pre-B&B presolve empty-set status and builds
// a root VIPR proof against p from bound conflicts, GCD equality rows, exact
// rational bound tightening, or a root LP Farkas ray.
func (tr *VIPRTrace) recordPresolveInfeasible(p *CompiledProblem, disableSIMD bool) {
	if tr == nil || p == nil {
		return
	}
	tr.reset(p)
	tr.presolveInfeasible = true
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return
	}
	if steps, rootStep, okGCD := buildGCDInfeasSteps(rp); okGCD {
		cert := rp.buildBaseCert(viprModeInfeasible)
		cert.Steps = steps
		cert.RootStep = rootStep
		tr.cert = cert
		tr.hasCert = true
		return
	}
	rec := &babVIPRRecorder{
		trace:       tr,
		rp:          rp,
		root:        &viprTreeNode{kind: viprNodeOpen},
		disableSIMD: disableSIMD,
	}
	defer rec.close()
	if feas, _ := rec.propagateNode(rec.root, nil, nil, rootPropPasses); !feas {
		rec.finalize(Result{Status: StatusInfeasible})
		if tr.hasCert {
			return
		}
	}
	sc := rec.ensureBaseScratchLP()
	if sc == nil {
		return
	}
	lbF := make([]float64, rp.n)
	ubF := make([]float64, rp.n)
	rec.fillNodeBoundsFloat(rec.root, lbF, ubF)
	sc.SetStructuralBounds(lbF, ubF)
	lpRes := sc.Solve()
	if lpRes.Status == StatusInfeasible {
		rec.certifyNodeFarkas(rec.root, sc, lpRes)
		rec.finalize(Result{Status: StatusInfeasible})
	}
}

// adoptInfeasSteps copies a Mini-VIPR empty-set proof from a zero-objective
// probe onto p while preserving p's objective vector and constant.
func (tr *VIPRTrace) adoptInfeasSteps(p *CompiledProblem, sub *VIPRTrace) {
	if tr == nil || p == nil || sub == nil || !sub.hasCert || sub.cert.Mode != viprModeInfeasible {
		return
	}
	tr.reset(p)
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return
	}
	cert := rp.buildBaseCert(viprModeInfeasible)
	cert.Steps = slices.Clone(sub.cert.Steps)
	cert.RootStep = sub.cert.RootStep
	tr.cert = cert
	tr.attachPresolveEnvelope(p)
	tr.hasCert = true
}

func populateLPVIPRTraceFromWitness(p *CompiledProblem, trace *VIPRTrace, res Result, wit *lpCertWitness) {
	if p == nil || trace == nil || wit == nil {
		return
	}
	rp, ok := buildVIPRRatProblem(p)
	if !ok {
		return
	}
	rec := &babVIPRRecorder{rp: rp}
	em := &viprEmitter{rec: rec, rp: rp}

	switch res.Status {
	case StatusOptimal:
		if len(wit.primal) != rp.n || len(wit.dualMin) != rp.m {
			return
		}
		mults := make([]viprRowMult, 0, rp.m)
		for i, yi := range wit.dualMin {
			if yi != nil && yi.Sign() != 0 {
				mults = append(mults, viprRowMult{rowID: i, mult: yi})
			}
		}
		id, rhs, okD := em.emitSafeDual(mults, rp.lb, rp.ub, nil)
		if !okD || len(em.steps) == 0 || id < rp.m {
			return
		}
		incObjNoConst := rp.evalObjNoConst(wit.primal)
		targetLB := new(big.Rat).Add(rp.objConst, rhs)
		mode := viprModeBound
		if incObjNoConst.Cmp(rhs) <= 0 {
			mode = viprModeOptimal
		}
		cert := rp.buildBaseCert(mode)
		cert.Steps = em.steps
		cert.RootStep = id - rp.m
		incPay := make([]viprRatPayload, rp.n)
		for j := range rp.n {
			incPay[j] = viprBigRatToPayload(wit.primal[j])
		}
		cert.Incumbent = incPay
		if mode == viprModeBound {
			lbPay := viprBigRatToPayload(targetLB)
			cert.TargetLB = &lbPay
		}
		trace.reset(p)
		trace.presolveEnv = nil
		trace.cert = cert
		trace.targetLB = targetLB
		trace.hasCert = true

	case StatusInfeasible:
		if wit.hasDirectInfeas {
			for j := range rp.n {
				if rp.lb[j] != nil && rp.ub[j] != nil && rp.ub[j].Cmp(rp.lb[j]) < 0 {
					cert := rp.buildBaseCert(viprModeInfeasible)
					cert.Steps = []viprStepJSON{{Rule: viprRuleBoundConflict, Col: viprIntPtr(j)}}
					cert.RootStep = 0
					trace.reset(p)
					trace.presolveEnv = nil
					trace.cert = cert
					trace.targetLB = nil
					trace.hasCert = true
					return
				}
			}
			if steps, rootStep, okGCD := buildGCDInfeasSteps(rp); okGCD {
				cert := rp.buildBaseCert(viprModeInfeasible)
				cert.Steps = steps
				cert.RootStep = rootStep
				trace.reset(p)
				trace.presolveEnv = nil
				trace.cert = cert
				trace.targetLB = nil
				trace.hasCert = true
				return
			}
		}
		if len(wit.farkasW) != rp.m {
			return
		}
		mults := make([]viprRowMult, 0, rp.m)
		for i, wi := range wit.farkasW {
			if wi != nil && wi.Sign() != 0 {
				mults = append(mults, viprRowMult{rowID: i, mult: wi})
			}
		}
		lbID := make([]int, rp.n)
		ubID := make([]int, rp.n)
		for j := range rp.n {
			lbID[j] = -1
			ubID[j] = -1
		}
		id, okF := em.emitFarkas(mults, rp.lb, rp.ub, lbID, ubID)
		if !okF || len(em.steps) == 0 || id < rp.m {
			return
		}
		cert := rp.buildBaseCert(viprModeInfeasible)
		cert.Steps = em.steps
		cert.RootStep = id - rp.m
		trace.reset(p)
		trace.presolveEnv = nil
		trace.cert = cert
		trace.targetLB = nil
		trace.hasCert = true

	case StatusTimeLimit, StatusNodeLimit, StatusIterationLimit:
		if len(wit.dualMin) != rp.m {
			return
		}
		mults := make([]viprRowMult, 0, rp.m)
		for i, yi := range wit.dualMin {
			if yi != nil && yi.Sign() != 0 {
				mults = append(mults, viprRowMult{rowID: i, mult: yi})
			}
		}
		id, rhs, okD := em.emitSafeDual(mults, rp.lb, rp.ub, nil)
		if !okD || len(em.steps) == 0 || id < rp.m {
			return
		}
		targetLB := new(big.Rat).Add(rp.objConst, rhs)
		cert := rp.buildBaseCert(viprModeBound)
		cert.Steps = em.steps
		cert.RootStep = id - rp.m
		lbPay := viprBigRatToPayload(targetLB)
		cert.TargetLB = &lbPay
		if len(wit.primal) == rp.n {
			incPay := make([]viprRatPayload, rp.n)
			for j := range rp.n {
				incPay[j] = viprBigRatToPayload(wit.primal[j])
			}
			cert.Incumbent = incPay
		}
		trace.reset(p)
		trace.presolveEnv = nil
		trace.cert = cert
		trace.targetLB = targetLB
		trace.hasCert = true
	}
}

func (tr *VIPRTrace) String() string {
	if tr == nil {
		return "VIPRTrace(nil)"
	}
	if !tr.hasCert {
		return fmt.Sprintf("VIPRTrace(hasCert=false, presolveInfeasible=%v)", tr.presolveInfeasible)
	}
	return fmt.Sprintf("VIPRTrace(mode=%s, steps=%d)", tr.cert.Mode, len(tr.cert.Steps))
}
