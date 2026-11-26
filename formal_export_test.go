package goblin

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	exportMaxPresolveRows = 256
	exportMaxSkipPerRow   = 4
	exportMaxOrbitVars    = 256
	exportMaxOrbitRows    = 256
	exportMaxOrbitTrials  = 4
)

// FormalOracleItem pairs a Lean checker subcommand or mode with a single JSONL request body.
type FormalOracleItem struct {
	Op      string         `json:"op,omitzero"`
	Mode    string         `json:"mode,omitzero"`
	Payload jsontext.Value `json:"payload,omitzero"`
}

func exportOptRatPayload(v float64, infSign int) (*ratPayload, bool) {
	if math.IsInf(v, infSign) {
		return nil, true
	}
	rp, err := float64ToPayload(v)
	if err != nil {
		return nil, false
	}
	return &rp, true
}

func exportExtRatAny(v float64) (any, bool) {
	if math.IsInf(v, -1) {
		return "negInf", true
	}
	if math.IsInf(v, 1) {
		return "posInf", true
	}
	rp, err := float64ToPayload(v)
	if err != nil {
		return nil, false
	}
	return rp, true
}

func exportCompiledProblemToLPPayload(p *CompiledProblem, mode string) (lpCertPayload, bool) {
	if p == nil {
		return lpCertPayload{}, false
	}
	n := p.numVars
	m := p.numConstraints
	objConstRat, okC := exactObjConstMinRat(p)
	if !okC {
		return lpCertPayload{}, false
	}
	objConst := bigRatToPayload(objConstRat)
	objMinVec, okVec := exactMinObjVecRat(p)
	if !okVec || len(objMinVec) != n {
		return lpCertPayload{}, false
	}
	obj := make([]ratPayload, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		obj[j] = bigRatToPayload(objMinVec[j])
		hasLB, lr, okL := exactVarLBRat(p, j)
		if !okL {
			return lpCertPayload{}, false
		}
		if hasLB {
			lp := bigRatToPayload(lr)
			lb[j] = &lp
		}
		hasUB, ur, okU := exactVarUBRat(p, j)
		if !okU {
			return lpCertPayload{}, false
		}
		if hasUB {
			up := bigRatToPayload(ur)
			ub[j] = &up
		}
	}
	rows := make([]lpRowPayload, m)
	for i := range m {
		rhsRat, okR := exactRowRHSRat(p, i)
		if !okR {
			return lpCertPayload{}, false
		}
		rhs := bigRatToPayload(rhsRat)
		cols, vals, okT := exactCanonicalRowTermsRat(p, i)
		if !okT || len(cols) != len(vals) {
			return lpCertPayload{}, false
		}
		terms := make([]lpTermPayload, len(cols))
		for k, col := range cols {
			terms[k] = lpTermPayload{Col: col, Coeff: bigRatToPayload(vals[k])}
		}
		rows[i] = lpRowPayload{
			Sense: senseToStr(p.rowSense[i]),
			RHS:   rhs,
			Terms: terms,
		}
	}
	return lpCertPayload{
		Mode:     mode,
		NumVars:  n,
		NumRows:  m,
		ObjConst: objConst,
		Obj:      obj,
		LB:       lb,
		UB:       ub,
		Rows:     rows,
	}, true
}

// ExportUnboundedCheckPayload constructs a single-line JSON request for lp-check --jsonl
// ("unbounded" for LPs, "milp-unbounded" via Meyer's theorem for MILPs) using ExtractUnboundedCertificate.
func ExportUnboundedCheckPayload(p *CompiledProblem, lpSolver *LPSolver, primalHint []float64) (mode string, payload []byte, ok bool) {
	if p == nil {
		return "", nil, false
	}
	certUnb, okUnb := ExtractUnboundedCertificate(context.Background(), p, lpSolver, primalHint)
	if !okUnb {
		return "", nil, false
	}
	certMode := "unbounded"
	var intCols []int
	if certUnb.IsMIP {
		certMode = "milp-unbounded"
		intCols = slices.Clone(p.intCols)
	}
	base, okBase := exportCompiledProblemToLPPayload(p, certMode)
	if !okBase {
		return "", nil, false
	}
	req := lpUnboundedCertPayload{
		lpCertPayload: base,
		X0:            ratSliceToPayloads(certUnb.X0),
		D:             ratSliceToPayloads(certUnb.Ray),
		IntCols:       intCols,
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return "", nil, false
	}
	return certMode, raw, true
}

type lpExplicitOptimalPayload struct {
	Mode     string         `json:"mode"`
	NumVars  int            `json:"numVars"`
	NumRows  int            `json:"numRows"`
	ObjConst ratPayload     `json:"objConst"`
	Obj      []ratPayload   `json:"obj"`
	LB       []*ratPayload  `json:"lb"`
	UB       []*ratPayload  `json:"ub"`
	Rows     []lpRowPayload `json:"rows"`
	X        []ratPayload   `json:"x"`
	Y        []ratPayload   `json:"y"`
}

type lpExplicitSafeBoundPayload struct {
	Mode     string         `json:"mode"`
	NumVars  int            `json:"numVars"`
	NumRows  int            `json:"numRows"`
	ObjConst ratPayload     `json:"objConst"`
	Obj      []ratPayload   `json:"obj"`
	LB       []*ratPayload  `json:"lb"`
	UB       []*ratPayload  `json:"ub"`
	Rows     []lpRowPayload `json:"rows"`
	Y        []ratPayload   `json:"y"`
}

func projectTrack3DualCone(p *CompiledProblem, dualMin []*big.Rat) []*big.Rat {
	m := p.numConstraints
	out := make([]*big.Rat, m)
	for i := range m {
		yi := new(big.Rat)
		if i < len(dualMin) && dualMin[i] != nil {
			yi.Set(dualMin[i])
		}
		if p.rowSense[i] == LessEqual && yi.Sign() > 0 {
			yi.SetInt64(0)
		} else if p.rowSense[i] == GreaterEqual && yi.Sign() < 0 {
			yi.SetInt64(0)
		}
		out[i] = yi
	}
	return out
}

func isExactLPOptimalRat(p *CompiledProblem, xRat, yProj []*big.Rat) bool {
	if p == nil || len(xRat) != p.numVars || len(yProj) != p.numConstraints {
		return false
	}
	n := p.numVars
	m := p.numConstraints
	for j := range n {
		xj := xRat[j]
		if xj == nil {
			return false
		}
		hasLB, lr, okL := exactVarLBRat(p, j)
		hasUB, ur, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return false
		}
		if hasLB && hasUB && lr.Cmp(ur) > 0 {
			return false
		}
		if hasLB && xj.Cmp(lr) < 0 {
			return false
		}
		if hasUB && xj.Cmp(ur) > 0 {
			return false
		}
	}
	for i := range m {
		if yProj[i] == nil {
			return false
		}
		rhsRat, okR := exactRowRHSRat(p, i)
		if !okR {
			return false
		}
		act, okAct := exactEvalRowActivityRat(p, i, xRat)
		if !okAct {
			return false
		}
		cmpVal := act.Cmp(rhsRat)
		switch p.rowSense[i] {
		case LessEqual:
			if cmpVal > 0 || yProj[i].Sign() > 0 {
				return false
			}
		case GreaterEqual:
			if cmpVal < 0 || yProj[i].Sign() < 0 {
				return false
			}
		case Equal:
			if cmpVal != 0 {
				return false
			}
		}
	}
	safeBnd, okSafe := ExactSafeDualBound(p, yProj)
	if !okSafe {
		return false
	}
	objMin, okC := exactObjConstMinRat(p)
	if !okC {
		return false
	}
	objMinVec, okVec := exactMinObjVecRat(p)
	if !okVec || len(objMinVec) != n {
		return false
	}
	prod := new(big.Rat)
	for j := range n {
		cj := objMinVec[j]
		if cj == nil || cj.Sign() == 0 || xRat[j].Sign() == 0 {
			continue
		}
		prod.Mul(cj, xRat[j])
		objMin.Add(objMin, prod)
	}
	return objMin.Cmp(safeBnd) == 0
}

func marshalExplicitOptimalCert(p *CompiledProblem, xRat, yProj []*big.Rat) ([]byte, bool) {
	base, okBase := exportCompiledProblemToLPPayload(p, "optimal")
	if !okBase {
		return nil, false
	}
	raw, err := json.Marshal(lpExplicitOptimalPayload{
		Mode:     "optimal",
		NumVars:  base.NumVars,
		NumRows:  base.NumRows,
		ObjConst: base.ObjConst,
		Obj:      base.Obj,
		LB:       base.LB,
		UB:       base.UB,
		Rows:     base.Rows,
		X:        ratSliceToPayloads(xRat),
		Y:        ratSliceToPayloads(yProj),
	})
	if err != nil {
		return nil, false
	}
	return raw, true
}

func marshalExplicitSafeBoundCert(p *CompiledProblem, yProj []*big.Rat) ([]byte, bool) {
	base, okBase := exportCompiledProblemToLPPayload(p, "safe-bound")
	if !okBase {
		return nil, false
	}
	raw, err := json.Marshal(lpExplicitSafeBoundPayload{
		Mode:     "safe-bound",
		NumVars:  base.NumVars,
		NumRows:  base.NumRows,
		ObjConst: base.ObjConst,
		Obj:      base.Obj,
		LB:       base.LB,
		UB:       base.UB,
		Rows:     base.Rows,
		Y:        ratSliceToPayloads(yProj),
	})
	if err != nil {
		return nil, false
	}
	return raw, true
}

// ExportLPCheckPayload builds a single-line JSON request for lp-check --jsonl
// from an LP solve on p ("optimal" with exact zero-gap (x, y) when available or
// "safe-bound" on numerical fallback for StatusOptimal, "bound-conflict" / "farkas"
// on StatusInfeasible, and "unbounded" / "milp-unbounded" on StatusUnbounded).
func ExportLPCheckPayload(p *CompiledProblem, lpSolver *LPSolver, res LPResult) (mode string, payload []byte, ok bool) {
	if p == nil {
		return "", nil, false
	}
	rp, okRP := buildVIPRRatProblem(p)
	if !okRP {
		return "", nil, false
	}

	switch res.Status {
	case StatusOptimal:
		// 1. Prefer exact rational basis primal and dual vectors (zero gap over Q).
		if lpSolver != nil && lpSolver.TotalRows() == rp.m && solverMatchesProblemBounds(lpSolver, p) {
			_ = PolishExactBasisDuals(context.Background(), lpSolver, &res)
			if sol, okSol := ExactBasisSolution(context.Background(), lpSolver); okSol &&
				len(sol.Primal) == rp.n && len(sol.DualMin) == rp.m {
				yProj := projectTrack3DualCone(p, sol.DualMin)
				if isExactLPOptimalRat(p, sol.Primal, yProj) {
					if raw, okM := marshalExplicitOptimalCert(p, sol.Primal, yProj); okM {
						return "optimal", raw, true
					}
				}
			}
		}

		// 2. When lpSolver is nil or basis solve fell back, check if rational
		// (res.Primal, res.Dual) already closes the gap to zero over Q.
		var yClean []*big.Rat
		var yBits []uint64
		hasFloatDuals := false
		if rp.m == 0 {
			yClean = []*big.Rat{}
			hasFloatDuals = true
		} else if len(res.Dual) == rp.m {
			senseSign := 1.0
			if p.sense == Maximize {
				senseSign = -1.0
			}
			yFloat := make([]*big.Rat, rp.m)
			yBits = make([]uint64, rp.m)
			allFinite := true
			for i := range rp.m {
				yi := senseSign * res.Dual[i]
				if yi == 0 {
					yi = 0
				}
				r, okR := exactRatFromFloat(yi)
				if !okR {
					allFinite = false
					break
				}
				yFloat[i] = r
				yBits[i] = math.Float64bits(yi)
			}
			if allFinite {
				yClean = projectTrack3DualCone(p, yFloat)
				hasFloatDuals = true
			}
		}
		if hasFloatDuals && len(res.Primal) == rp.n {
			xRat := make([]*big.Rat, rp.n)
			allXFinite := true
			for j := range rp.n {
				xr, okX := exactRatFromFloat(res.Primal[j])
				if !okX {
					allXFinite = false
					break
				}
				xRat[j] = xr
			}
			if allXFinite && isExactLPOptimalRat(p, xRat, yClean) {
				if raw, okM := marshalExplicitOptimalCert(p, xRat, yClean); okM {
					return "optimal", raw, true
				}
			}
		}

		// 3. Numerical fallback: emit safe-bound certificate when duals certify a finite bound.
		if lpSolver != nil && lpSolver.TotalRows() == rp.m && solverMatchesProblemBounds(lpSolver, p) {
			if yMin, _, okExact := ExactBasisDuals(context.Background(), lpSolver); okExact && len(yMin) == rp.m {
				yProj := projectTrack3DualCone(p, yMin)
				if _, okSafe := ExactSafeDualBound(p, yProj); okSafe {
					if raw, okM := marshalExplicitSafeBoundCert(p, yProj); okM {
						return "safe-bound", raw, true
					}
				}
			}
		}
		if hasFloatDuals {
			if _, okDual := ExactSafeDualBound(p, yClean); okDual {
				if rp.m == 0 {
					if raw, okM := marshalExplicitSafeBoundCert(p, yClean); okM {
						return "safe-bound", raw, true
					}
				} else {
					cert, okBase := exportCompiledProblemToLPPayload(p, "safe-bound")
					if !okBase {
						return "", nil, false
					}
					cert.YBits = yBits
					raw, err := json.Marshal(cert)
					if err != nil {
						return "", nil, false
					}
					return "safe-bound", raw, true
				}
			}
		}
		if lpSolver != nil && lpSolver.ws != nil && lpSolver.TotalRows() == rp.m {
			yRat, _, okDual := rp.extractNodeSafeDual(lpSolver, res, rp.lb, rp.ub)
			if okDual {
				if raw, okM := marshalExplicitSafeBoundCert(p, yRat); okM {
					return "safe-bound", raw, true
				}
			}
		}
		return "", nil, false

	case StatusInfeasible:
		for j := range rp.n {
			if rp.lb[j] != nil && rp.ub[j] != nil && rp.lb[j].Cmp(rp.ub[j]) > 0 {
				cert, okBase := exportCompiledProblemToLPPayload(p, "bound-conflict")
				if !okBase {
					return "", nil, false
				}
				colIdx := j
				cert.Col = &colIdx
				raw, err := json.Marshal(cert)
				if err != nil {
					return "", nil, false
				}
				return "bound-conflict", raw, true
			}
		}
		if rp.m == 0 {
			return "", nil, false
		}
		wT4, okF := rp.extractNodeFarkas(lpSolver, res.Dual, rp.lb, rp.ub)
		if !okF {
			return "", nil, false
		}
		// Track 3 lp-check uses the negated dual cone relative to Track 4 Mini-VIPR.
		wT3 := make([]*big.Rat, rp.m)
		for i := range rp.m {
			wT3[i] = new(big.Rat).Neg(wT4[i])
		}
		if !isExactRatFarkasValid(p, wT3) {
			return "", nil, false
		}
		cert, okBase := exportCompiledProblemToLPPayload(p, "farkas")
		if !okBase {
			return "", nil, false
		}
		wPay := make([]ratPayload, rp.m)
		for i := range rp.m {
			wPay[i] = bigRatToPayload(wT3[i])
		}
		cert.W = wPay
		raw, err := json.Marshal(cert)
		if err != nil {
			return "", nil, false
		}
		return "farkas", raw, true

	case StatusUnbounded:
		if mMode, mRaw, okM := ExportUnboundedCheckPayload(p, lpSolver, res.Primal); okM {
			return mMode, mRaw, true
		}
		if p.isMIP {
			lpRelax := *p
			lpRelax.isMIP = false
			lpRelax.intCols = nil
			return ExportUnboundedCheckPayload(&lpRelax, lpSolver, res.Primal)
		}
		return "", nil, false

	default:
		return "", nil, false
	}
}

type lpPresolveChainStepExport struct {
	Kind       string        `json:"kind"`
	Col        *int          `json:"col,omitzero"`
	OrigRow    *int          `json:"origRow,omitzero"`
	DroppedRow *int          `json:"droppedRow,omitzero"`
	ToLower    *bool         `json:"toLower,omitzero"`
	Val        *ratPayload   `json:"val,omitzero"`
	OrigRowIdx []int         `json:"origRowIdx,omitzero"`
	After      lpCertPayload `json:"after"`
}

type lpPresolveChainCertExport struct {
	Mode     string                      `json:"mode"`
	Orig     lpCertPayload               `json:"orig"`
	Reduced  lpCertPayload               `json:"reduced"`
	Steps    []lpPresolveChainStepExport `json:"steps"`
	XReduced []ratPayload                `json:"xReduced,omitzero"`
	YReduced []ratPayload                `json:"yReduced,omitzero"`
}

// ExportLPPresolveCheckPayload runs presolveLPWithObserver on p, replays the live
// presolve events in exact rational math, checks that the final state matches preRes.Reduced,
// and emits a complete lp-presolve chain certificate (with exact reduced optimal primal/duals
// when Reduced solves to StatusOptimal).
func ExportLPPresolveCheckPayload(p *CompiledProblem) ([]byte, bool) {
	if p == nil || p.exactOrig != nil || p.numVars == 0 || p.numConstraints == 0 {
		return nil, false
	}
	var events []lpPresolveEvent
	obs := &presolveObserver{
		onLPStep: func(ev lpPresolveEvent) {
			events = append(events, ev)
		},
	}
	preRes := presolveLPWithObserver(p, defaultPresolveMaxPass, time.Time{}, nil, obs)
	if preRes.Infeasible || preRes.TimedOut || preRes.Reduced == nil || preRes.Reduced == p || len(events) == 0 {
		return nil, false
	}

	n := p.numVars
	m := p.numConstraints
	objConstRat, okOC := exactRatFromFloat(p.objConstMin())
	if !okOC {
		return nil, false
	}
	curObj := make([]*big.Rat, n)
	curLB := make([]*big.Rat, n)
	curUB := make([]*big.Rat, n)
	for j := range n {
		cj, okC := exactRatFromFloat(p.objMin[j])
		if !okC {
			return nil, false
		}
		curObj[j] = cj
		if !math.IsInf(p.varLB[j], -1) {
			lr, okL := exactRatFromFloat(p.varLB[j])
			if !okL {
				return nil, false
			}
			curLB[j] = lr
		}
		if !math.IsInf(p.varUB[j], 1) {
			ur, okU := exactRatFromFloat(p.varUB[j])
			if !okU {
				return nil, false
			}
			curUB[j] = ur
		}
	}

	type ratLPWorkRow struct {
		origIdx int
		sense   ConstraintSense
		rhs     *big.Rat
		coeffs  map[int]*big.Rat
	}
	curRows := make([]ratLPWorkRow, m)
	for i := range m {
		rhsR, okR := exactRatFromFloat(p.rowRHS[i])
		if !okR {
			return nil, false
		}
		cm := make(map[int]*big.Rat, p.rowPtr[i+1]-p.rowPtr[i])
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			v := p.rowVal[k]
			if v == 0 {
				continue
			}
			vr, okV := exactRatFromFloat(v)
			if !okV {
				return nil, false
			}
			col := p.colIdx[k]
			if prev, exists := cm[col]; exists {
				sum := new(big.Rat).Add(prev, vr)
				if sum.Sign() == 0 {
					delete(cm, col)
				} else {
					cm[col] = sum
				}
			} else {
				cm[col] = vr
			}
		}
		curRows[i] = ratLPWorkRow{
			origIdx: i,
			sense:   p.rowSense[i],
			rhs:     rhsR,
			coeffs:  cm,
		}
	}
	isRatRowRedundant := func(r ratLPWorkRow) bool {
		if r.sense == Equal {
			for _, vr := range r.coeffs {
				if vr != nil && vr.Sign() != 0 {
					return false
				}
			}
			return r.rhs.Sign() == 0
		}
		wantUpper := r.sense == LessEqual
		if !wantUpper && r.sense != GreaterEqual {
			return false
		}
		act := new(big.Rat)
		var prod big.Rat
		for col, vr := range r.coeffs {
			if vr == nil || vr.Sign() == 0 {
				continue
			}
			if uint(col) >= uint(n) {
				return false
			}
			var bnd *big.Rat
			if (vr.Sign() > 0) == wantUpper {
				bnd = curUB[col]
			} else {
				bnd = curLB[col]
			}
			if bnd == nil {
				return false
			}
			prod.Mul(vr, bnd)
			act.Add(act, &prod)
		}
		if wantUpper {
			return act.Cmp(r.rhs) <= 0
		}
		return r.rhs.Cmp(act) <= 0
	}
	canDualFixCol := func(col int, toLower bool) bool {
		if toLower {
			if curLB[col] == nil || (curUB[col] != nil && curLB[col].Cmp(curUB[col]) > 0) || curObj[col].Sign() < 0 {
				return false
			}
		} else {
			if curUB[col] == nil || (curLB[col] != nil && curLB[col].Cmp(curUB[col]) > 0) || curObj[col].Sign() > 0 {
				return false
			}
		}
		for _, r := range curRows {
			a := r.coeffs[col]
			if a == nil || a.Sign() == 0 {
				continue
			}
			switch r.sense {
			case LessEqual:
				if (toLower && a.Sign() < 0) || (!toLower && a.Sign() > 0) {
					return false
				}
			case GreaterEqual:
				if (toLower && a.Sign() > 0) || (!toLower && a.Sign() < 0) {
					return false
				}
			case Equal:
				return false
			default:
				return false
			}
		}
		return true
	}
	isEmptyColInRows := func(col int) bool {
		for _, r := range curRows {
			if a := r.coeffs[col]; a != nil && a.Sign() != 0 {
				return false
			}
		}
		return true
	}

	snapshotLP := func(mode string) lpCertPayload {
		objPay := make([]ratPayload, n)
		lbPay := make([]*ratPayload, n)
		ubPay := make([]*ratPayload, n)
		for j := range n {
			objPay[j] = bigRatToPayload(curObj[j])
			if curLB[j] != nil {
				lp := bigRatToPayload(curLB[j])
				lbPay[j] = &lp
			}
			if curUB[j] != nil {
				up := bigRatToPayload(curUB[j])
				ubPay[j] = &up
			}
		}
		rowsPay := make([]lpRowPayload, len(curRows))
		for q, r := range curRows {
			cols := make([]int, 0, len(r.coeffs))
			for col, vr := range r.coeffs {
				if vr.Sign() != 0 {
					cols = append(cols, col)
				}
			}
			slices.Sort(cols)
			terms := make([]lpTermPayload, len(cols))
			for ti, col := range cols {
				terms[ti] = lpTermPayload{Col: col, Coeff: bigRatToPayload(r.coeffs[col])}
			}
			rowsPay[q] = lpRowPayload{
				Sense: senseToStr(r.sense),
				RHS:   bigRatToPayload(r.rhs),
				Terms: terms,
			}
		}
		return lpCertPayload{
			Mode:     mode,
			NumVars:  n,
			NumRows:  len(curRows),
			ObjConst: bigRatToPayload(objConstRat),
			Obj:      objPay,
			LB:       lbPay,
			UB:       ubPay,
			Rows:     rowsPay,
		}
	}

	origPay := snapshotLP("lp-presolve")
	stepsOut := make([]lpPresolveChainStepExport, 0, len(events))

	makeSkipMap := func(total, skipIdx int) []int {
		out := make([]int, 0, total-1)
		for q := range total {
			if q != skipIdx {
				out = append(out, q)
			}
		}
		return out
	}

	for _, ev := range events {
		switch ev.kind {
		case lpStepFixedSubst:
			col := ev.col
			if uint(col) >= uint(n) || curLB[col] == nil || curUB[col] == nil || curLB[col].Cmp(curUB[col]) != 0 {
				return nil, false
			}
			val := new(big.Rat).Set(curLB[col])
			prod := new(big.Rat)
			for q := range curRows {
				if a, exists := curRows[q].coeffs[col]; exists && a.Sign() != 0 {
					prod.Mul(a, val)
					curRows[q].rhs = new(big.Rat).Sub(curRows[q].rhs, prod)
					delete(curRows[q].coeffs, col)
				} else {
					delete(curRows[q].coeffs, col)
				}
			}
			valPay := bigRatToPayload(val)
			cIdx := col
			stepsOut = append(stepsOut, lpPresolveChainStepExport{
				Kind:  "fixedSubst",
				Col:   &cIdx,
				Val:   &valPay,
				After: snapshotLP("lp-presolve"),
			})

		case lpStepSingletonRow:
			q := slices.IndexFunc(curRows, func(r ratLPWorkRow) bool { return r.origIdx == ev.row })
			col := ev.col
			if q < 0 || uint(col) >= uint(n) || len(curRows[q].coeffs) != 1 {
				return nil, false
			}
			a, exists := curRows[q].coeffs[col]
			if !exists || a.Sign() == 0 {
				return nil, false
			}
			imp := new(big.Rat).Quo(curRows[q].rhs, a)
			if curLB[col] != nil && imp.Cmp(curLB[col]) < 0 {
				return nil, false
			}
			if curUB[col] != nil && imp.Cmp(curUB[col]) > 0 {
				return nil, false
			}
			sense := curRows[q].sense
			switch sense {
			case Equal:
				curLB[col] = new(big.Rat).Set(imp)
				curUB[col] = new(big.Rat).Set(imp)
			case LessEqual:
				if a.Sign() > 0 {
					curUB[col] = new(big.Rat).Set(imp)
				} else {
					curLB[col] = new(big.Rat).Set(imp)
				}
			case GreaterEqual:
				if a.Sign() > 0 {
					curLB[col] = new(big.Rat).Set(imp)
				} else {
					curUB[col] = new(big.Rat).Set(imp)
				}
			default:
				return nil, false
			}
			rowMap := makeSkipMap(len(curRows), q)
			curRows = slices.Delete(curRows, q, q+1)
			rIdx := q
			cIdx := col
			stepsOut = append(stepsOut, lpPresolveChainStepExport{
				Kind:       "singletonRow",
				OrigRow:    &rIdx,
				Col:        &cIdx,
				OrigRowIdx: rowMap,
				After:      snapshotLP("lp-presolve"),
			})

		case lpStepDropRedundantRow:
			q := slices.IndexFunc(curRows, func(r ratLPWorkRow) bool { return r.origIdx == ev.row })
			if q < 0 || !isRatRowRedundant(curRows[q]) {
				return nil, false
			}
			rowMap := makeSkipMap(len(curRows), q)
			curRows = slices.Delete(curRows, q, q+1)
			rIdx := q
			stepsOut = append(stepsOut, lpPresolveChainStepExport{
				Kind:       "dropRedundantRow",
				DroppedRow: &rIdx,
				OrigRowIdx: rowMap,
				After:      snapshotLP("lp-presolve"),
			})

		case lpStepDualColFix:
			col := ev.col
			if uint(col) >= uint(n) || !canDualFixCol(col, ev.toLower) {
				return nil, false
			}
			if ev.toLower {
				curUB[col] = new(big.Rat).Set(curLB[col])
			} else {
				curLB[col] = new(big.Rat).Set(curUB[col])
			}
			cIdx := col
			toLo := ev.toLower
			stepsOut = append(stepsOut, lpPresolveChainStepExport{
				Kind:    "dualColFix",
				Col:     &cIdx,
				ToLower: &toLo,
				After:   snapshotLP("lp-presolve"),
			})

		case lpStepZeroObjEmptyColFix:
			col := ev.col
			if uint(col) >= uint(n) || curObj[col].Sign() != 0 || !isEmptyColInRows(col) {
				return nil, false
			}
			valR, okV := exactRatFromFloat(ev.val)
			if !okV {
				return nil, false
			}
			if curLB[col] != nil && valR.Cmp(curLB[col]) < 0 {
				return nil, false
			}
			if curUB[col] != nil && valR.Cmp(curUB[col]) > 0 {
				return nil, false
			}
			curLB[col] = new(big.Rat).Set(valR)
			curUB[col] = new(big.Rat).Set(valR)
			cIdx := col
			valPay := bigRatToPayload(valR)
			stepsOut = append(stepsOut, lpPresolveChainStepExport{
				Kind:  "zeroObjEmptyColFix",
				Col:   &cIdx,
				Val:   &valPay,
				After: snapshotLP("lp-presolve"),
			})

		default:
			return nil, false
		}
	}

	// Confirm the replayed rational state matches preRes.Reduced over Q.
	red := preRes.Reduced
	if red.numVars != n || red.numConstraints != len(curRows) {
		return nil, false
	}
	tmpRat := new(big.Rat)
	for j := range n {
		if math.IsInf(red.varLB[j], -1) {
			if curLB[j] != nil {
				return nil, false
			}
		} else if curLB[j] == nil || tmpRat.SetFloat64(red.varLB[j]) == nil || curLB[j].Cmp(tmpRat) != 0 {
			return nil, false
		}
		if math.IsInf(red.varUB[j], 1) {
			if curUB[j] != nil {
				return nil, false
			}
		} else if curUB[j] == nil || tmpRat.SetFloat64(red.varUB[j]) == nil || curUB[j].Cmp(tmpRat) != 0 {
			return nil, false
		}
	}
	for q, r := range curRows {
		if red.rowSense[q] != r.sense || tmpRat.SetFloat64(red.rowRHS[q]) == nil || r.rhs.Cmp(tmpRat) != 0 {
			return nil, false
		}
		nnz := red.rowPtr[q+1] - red.rowPtr[q]
		if nnz != len(r.coeffs) {
			return nil, false
		}
		for k := red.rowPtr[q]; k < red.rowPtr[q+1]; k++ {
			col := red.colIdx[k]
			vr, exists := r.coeffs[col]
			if !exists || tmpRat.SetFloat64(red.rowVal[k]) == nil || vr.Cmp(tmpRat) != 0 {
				return nil, false
			}
		}
	}

	redPay, okRed := exportCompiledProblemToLPPayload(red, "optimal")
	if !okRed {
		return nil, false
	}
	cert := lpPresolveChainCertExport{
		Mode:    "lp-presolve",
		Orig:    origPay,
		Reduced: redPay,
		Steps:   stepsOut,
	}

	lpRed := NewLPSolver(red, nil)
	defer lpRed.Close()
	if rSol := lpRed.Solve(); rSol.Status == StatusOptimal {
		if sol, okSol := ExactBasisSolution(context.Background(), lpRed); okSol &&
			len(sol.Primal) == red.numVars && len(sol.DualMin) == red.numConstraints {
			yProj := projectTrack3DualCone(red, sol.DualMin)
			if isExactLPOptimalRat(red, sol.Primal, yProj) {
				cert.XReduced = ratSliceToPayloads(sol.Primal)
				cert.YReduced = ratSliceToPayloads(yProj)
			}
		}
	}
	if len(cert.XReduced) != red.numVars {
		return nil, false
	}

	raw, err := json.Marshal(cert)
	if err != nil {
		return nil, false
	}
	return raw, true
}

type exportGcdInfeasItem struct {
	Subcommand   string  `json:"subcommand"`
	Op           string  `json:"op"`
	Coeffs       []int64 `json:"coeffs"`
	Lo           int64   `json:"lo"`
	Hi           int64   `json:"hi"`
	CandidateG   *int64  `json:"candidate_g,omitzero"`
	GoInfeasible bool    `json:"go_infeasible"`
}

type exportGcdDivCheckItem struct {
	Subcommand       string     `json:"subcommand"`
	Op               string     `json:"op"`
	Coeffs           []int64    `json:"coeffs"`
	Sense            string     `json:"sense"`
	RHS              ratPayload `json:"rhs"`
	Divisor          *int64     `json:"divisor,omitzero"`
	ExpectInfeasible *bool      `json:"expect_infeasible,omitzero"`
	DivCoeffs        []int64    `json:"div_coeffs,omitzero"`
	DivRHS           *int64     `json:"div_rhs,omitzero"`
}

func isValidRoundedGcdRelaxationRat(
	cols []int,
	origVals, lb, ub []float64,
	sense ConstraintSense,
	origRHS, rhsComp float64,
	intCoeffs []int64,
) bool {
	origRHSRat, okO := exactRatFromFloat(origRHS)
	rhsCompRat, okC := exactRatFromFloat(rhsComp)
	if !okO || !okC || len(cols) != len(origVals) || len(cols) != len(intCoeffs) {
		return false
	}
	maxShift := new(big.Rat)
	minShift := new(big.Rat)
	diff := new(big.Rat)
	tLo := new(big.Rat)
	tHi := new(big.Rat)
	for k, col := range cols {
		vRat, okV := exactRatFromFloat(origVals[k])
		if !okV {
			return false
		}
		rcRat := big.NewRat(intCoeffs[k], 1)
		diff.Sub(rcRat, vRat)
		if diff.Sign() == 0 {
			continue
		}
		loR, okL := exactRatFromFloat(lb[col])
		hiR, okU := exactRatFromFloat(ub[col])
		if !okL || !okU {
			return false
		}
		tLo.Mul(diff, loR)
		tHi.Mul(diff, hiR)
		if tLo.Cmp(tHi) >= 0 {
			maxShift.Add(maxShift, tLo)
			minShift.Add(minShift, tHi)
		} else {
			maxShift.Add(maxShift, tHi)
			minShift.Add(minShift, tLo)
		}
	}
	switch sense {
	case LessEqual:
		return new(big.Rat).Add(origRHSRat, maxShift).Cmp(rhsCompRat) <= 0
	case GreaterEqual:
		return new(big.Rat).Add(origRHSRat, minShift).Cmp(rhsCompRat) >= 0
	case Equal:
		return new(big.Rat).Add(origRHSRat, maxShift).Cmp(rhsCompRat) <= 0 &&
			new(big.Rat).Add(origRHSRat, minShift).Cmp(rhsCompRat) >= 0
	default:
		return false
	}
}

type exportActivityTermItem struct {
	A     ratPayload  `json:"a"`
	L     *ratPayload `json:"l"`
	U     *ratPayload `json:"u"`
	IsInt bool        `json:"is_int"`
	GoUB  *ratPayload `json:"go_ub,omitzero"`
	GoLB  *ratPayload `json:"go_lb,omitzero"`
}

type exportActivitySummaryItem struct {
	Subcommand string                   `json:"subcommand"`
	Op         string                   `json:"op"`
	RHS        ratPayload               `json:"rhs"`
	HasLo      bool                     `json:"has_lo"`
	HasHi      bool                     `json:"has_hi"`
	Terms      []exportActivityTermItem `json:"terms"`
}

type exportActivitySkipItem struct {
	Subcommand  string     `json:"subcommand"`
	Op          string     `json:"op"`
	SlackLo     ratPayload `json:"slack_lo"`
	AbsA        ratPayload `json:"abs_a"`
	L           ratPayload `json:"l"`
	U           ratPayload `json:"u"`
	TargetBound ratPayload `json:"target_bound"`
	IsInt       bool       `json:"is_int"`
	GoSkip      bool       `json:"go_skip"`
}

type exportBinStrengthenItem struct {
	Subcommand   string                   `json:"subcommand"`
	Op           string                   `json:"op"`
	Terms        []exportActivityTermItem `json:"terms"`
	TargetIdx    int                      `json:"target_idx"`
	RHS          ratPayload               `json:"rhs"`
	A            ratPayload               `json:"a"`
	MaxOther     ratPayload               `json:"max_other"`
	Surplus      ratPayload               `json:"surplus"`
	NewA         *ratPayload              `json:"new_a"`
	NewRHS       *ratPayload              `json:"new_rhs"`
	RequireValid bool                     `json:"require_valid"`
}

type exportOrbitCheckItem struct {
	Subcommand   string        `json:"subcommand"`
	Op           string        `json:"op"`
	NumVars      int           `json:"num_vars"`
	IsInt        []bool        `json:"is_int"`
	LB           []any         `json:"lb"`
	UB           []any         `json:"ub"`
	Obj          []ratPayload  `json:"obj"`
	Rows         []orbitRowReq `json:"rows"`
	GCount       int           `json:"g_count"`
	Orbits       []int         `json:"orbits"`
	RowTuples    []int         `json:"row_tuples"`
	ColOrbit     []int         `json:"col_orbit"`
	ColGroup     []int         `json:"col_group"`
	RowState     []int         `json:"row_state"`
	RequireValid bool          `json:"require_valid"`
}

func buildActivityTermsPayload(p *CompiledProblem, cols []int, vals, lb, ub []float64) ([]exportActivityTermItem, bool) {
	terms := make([]exportActivityTermItem, len(cols))
	for k, col := range cols {
		aPay, errA := float64ToPayload(vals[k])
		lPtr, okL := exportOptRatPayload(lb[col], -1)
		uPtr, okU := exportOptRatPayload(ub[col], 1)
		if errA != nil || !okL || !okU {
			return nil, false
		}
		terms[k] = exportActivityTermItem{
			A:     aPay,
			L:     lPtr,
			U:     uPtr,
			IsInt: p.varCat[col] != Continuous,
		}
	}
	return terms, true
}

// ExportPresolveRulesItems runs Presolve with a live observer on p and exports JSONL
// items for rules-check --jsonl covering observed GCD infeasible equalities, Euclidean GCD
// row scaling, Savelsbergh bound tightening, skip-term guards, binary coefficient
// strengthening, and orbit symmetry checks.
func ExportPresolveRulesItems(p *CompiledProblem) (items [][]byte, preRes PresolveResult) {
	if p == nil {
		return nil, PresolveResult{}
	}
	n := p.numVars
	m := p.numConstraints
	if n == 0 || m == 0 {
		return nil, Presolve(p, defaultPresolveMaxPass)
	}

	nonTightRowCount := 0
	skipCount := 0
	orbitCount := 0

	obs := &presolveObserver{
		onGcdInfeas: func(_ int, _ []int, origVals []float64, origRHS float64, loInt, hiInt, g int64, goInf bool, maxRoundErr float64) {
			coeffs := make([]int64, len(origVals))
			for k, v := range origVals {
				rv := math.Round(v)
				if maxRoundErr == 0 && v != rv {
					return
				}
				coeffs[k] = int64(rv)
			}
			candG := g
			req := exportGcdInfeasItem{
				Subcommand:   "gcd-infeas",
				Op:           "gcd-infeas",
				Coeffs:       coeffs,
				Lo:           loInt,
				Hi:           hiInt,
				CandidateG:   &candG,
				GoInfeasible: goInf,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
			}
			if maxRoundErr == 0 && g > 0 && math.Abs(origRHS) <= 0x1p52 {
				rhsPay, errRHS := float64ToPayload(origRHS)
				rhsRat := new(big.Rat).SetFloat64(origRHS)
				if errRHS == nil && rhsRat != nil {
					qRat := new(big.Rat).Quo(rhsRat, big.NewRat(g, 1))
					if !qRat.IsInt() {
						expInf := true
						divReq := exportGcdDivCheckItem{
							Subcommand:       "gcd-div-check",
							Op:               "gcd-div-check",
							Coeffs:           coeffs,
							Sense:            "eq",
							RHS:              rhsPay,
							Divisor:          &candG,
							ExpectInfeasible: &expInf,
						}
						if rawDiv, errDiv := json.Marshal(divReq); errDiv == nil {
							items = append(items, rawDiv)
						}
					}
				}
			}
		},
		onGcdDiv: func(_ int, cols []int, origVals, lb, ub []float64, sense ConstraintSense, origRHS, rhsComp float64, g int64, divCoeffs []int64, divRHS int64, maxRoundErr float64) {
			if len(origVals) == 0 || g <= 0 {
				return
			}
			intCoeffs := make([]int64, len(origVals))
			for k, v := range origVals {
				intCoeffs[k] = int64(math.Round(v))
			}
			// When coefficients were rounded (maxRoundErr > 0), check over exact Q that
			// rhsComp soundly bounds the domain shift of (origVals, origRHS).
			// If unsound, pass origRHS unadjusted so rules-check fails closed immediately.
			rhsToCheck := origRHS
			if maxRoundErr > 0 && isValidRoundedGcdRelaxationRat(cols, origVals, lb, ub, sense, origRHS, rhsComp, intCoeffs) {
				rhsToCheck = rhsComp
			}
			rhsPay, errRHS := float64ToPayload(rhsToCheck)
			if errRHS != nil {
				return
			}
			candG := g
			expInf := false
			dRhs := divRHS
			req := exportGcdDivCheckItem{
				Subcommand:       "gcd-div-check",
				Op:               "gcd-div-check",
				Coeffs:           intCoeffs,
				Sense:            senseToStr(sense),
				RHS:              rhsPay,
				Divisor:          &candG,
				ExpectInfeasible: &expInf,
				DivCoeffs:        slices.Clone(divCoeffs),
				DivRHS:           &dRhs,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
			}
		},
		onActivityTight: func(_, termIdx int, cols []int, vals []float64, rhs float64, hasLo, hasHi, isUpper bool, preLB, preUB []float64, newBound float64) {
			rhsPay, errRHS := float64ToPayload(rhs)
			bndPay, errBnd := float64ToPayload(newBound)
			if errRHS != nil || errBnd != nil || uint(termIdx) >= uint(len(cols)) {
				return
			}
			terms, okT := buildActivityTermsPayload(p, cols, vals, preLB, preUB)
			if !okT {
				return
			}
			if isUpper {
				terms[termIdx].GoUB = &bndPay
			} else {
				terms[termIdx].GoLB = &bndPay
			}
			req := exportActivitySummaryItem{
				Subcommand: "activity",
				Op:         "summary",
				RHS:        rhsPay,
				HasLo:      hasLo,
				HasHi:      hasHi,
				Terms:      terms,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
			}
		},
		onActivityRow: func(_ int, cols []int, vals []float64, rhs float64, hasLo, hasHi bool, lb, ub []float64) {
			if nonTightRowCount >= exportMaxPresolveRows || len(cols) == 0 {
				return
			}
			rhsPay, errRHS := float64ToPayload(rhs)
			if errRHS != nil {
				return
			}
			terms, okT := buildActivityTermsPayload(p, cols, vals, lb, ub)
			if !okT {
				return
			}
			req := exportActivitySummaryItem{
				Subcommand: "activity",
				Op:         "summary",
				RHS:        rhsPay,
				HasLo:      hasLo,
				HasHi:      hasHi,
				Terms:      terms,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
				nonTightRowCount++
			}
		},
		onSkipTerm: func(slackLo, absA, l, u, targetBound float64, isInt, goSkip bool) {
			if skipCount >= exportMaxPresolveRows*exportMaxSkipPerRow {
				return
			}
			sPay, errS := float64ToPayload(slackLo)
			aPay, errA := float64ToPayload(absA)
			lPay, errL := float64ToPayload(l)
			uPay, errU := float64ToPayload(u)
			tPay, errT := float64ToPayload(targetBound)
			if errS != nil || errA != nil || errL != nil || errU != nil || errT != nil {
				return
			}
			req := exportActivitySkipItem{
				Subcommand:  "activity",
				Op:          "skip",
				SlackLo:     sPay,
				AbsA:        aPay,
				L:           lPay,
				U:           uPay,
				TargetBound: tPay,
				IsInt:       isInt,
				GoSkip:      goSkip,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
				skipCount++
			}
		},
		onBinTight: func(_, targetIdx int, cols []int, preVals []float64, preRHS float64, lb, ub []float64, a, surplus, newA, newRHS float64) {
			if uint(targetIdx) >= uint(len(cols)) {
				return
			}
			terms, okT := buildActivityTermsPayload(p, cols, preVals, lb, ub)
			if !okT {
				return
			}
			maxOther := new(big.Rat)
			term := new(big.Rat)
			otherOK := true
			for jIdx, c2 := range cols {
				if jIdx == targetIdx {
					continue
				}
				a2, okA2 := exactRatFromFloat(preVals[jIdx])
				if !okA2 {
					otherOK = false
					break
				}
				var bd float64
				if a2.Sign() > 0 {
					bd = ub[c2]
				} else {
					bd = lb[c2]
				}
				br, okB := exactRatFromFloat(bd)
				if !okB {
					otherOK = false
					break
				}
				term.Mul(a2, br)
				maxOther.Add(maxOther, term)
			}
			if !otherOK {
				return
			}
			rhsPay, errRHS := float64ToPayload(preRHS)
			aPay, errA := float64ToPayload(a)
			surpPay, errS := float64ToPayload(surplus)
			naPay, errNA := float64ToPayload(newA)
			nrPay, errNR := float64ToPayload(newRHS)
			if errRHS != nil || errA != nil || errS != nil || errNA != nil || errNR != nil {
				return
			}
			req := exportBinStrengthenItem{
				Subcommand:   "bin-strengthen",
				Op:           "bin-strengthen",
				Terms:        terms,
				TargetIdx:    targetIdx,
				RHS:          rhsPay,
				A:            aPay,
				MaxOther:     bigRatToPayload(maxOther),
				Surplus:      surpPay,
				NewA:         &naPay,
				NewRHS:       &nrPay,
				RequireValid: true,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
			}
		},
		onOrbit: func(rows []presolveWorkRow, lb, ub []float64, gCount int, sc *orbitTrialScratch) {
			if orbitCount >= exportMaxOrbitTrials || n > exportMaxOrbitVars || len(rows) > exportMaxOrbitRows {
				return
			}
			if orbItem, okBuild := buildExportOrbitCheckItem(p, rows, lb, ub, gCount, sc); okBuild {
				if raw, err := json.Marshal(orbItem); err == nil {
					items = append(items, raw)
					orbitCount++
				}
			}
		},
	}

	preRes = presolveWithObserver(p, defaultPresolveMaxPass, time.Time{}, nil, obs)
	return items, preRes
}

func buildExportOrbitCheckItem(
	p *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	gCount int,
	sc *orbitTrialScratch,
) (exportOrbitCheckItem, bool) {
	n := p.numVars
	isInt := make([]bool, n)
	lbAny := make([]any, n)
	ubAny := make([]any, n)
	obj := make([]ratPayload, n)
	for j := range n {
		isInt[j] = p.varCat[j] != Continuous
		la, okL := exportExtRatAny(lb[j])
		ua, okU := exportExtRatAny(ub[j])
		op, errO := float64ToPayload(p.objMin[j])
		if !okL || !okU || errO != nil {
			return exportOrbitCheckItem{}, false
		}
		lbAny[j] = la
		ubAny[j] = ua
		obj[j] = op
	}
	rowReqs := make([]orbitRowReq, len(rows))
	for i := range rows {
		r := &rows[i]
		vals := make([]ratPayload, len(r.vals))
		for k, v := range r.vals {
			vp, errV := float64ToPayload(v)
			if errV != nil {
				return exportOrbitCheckItem{}, false
			}
			vals[k] = vp
		}
		rhsPay, errR := float64ToPayload(r.rhs)
		if errR != nil {
			return exportOrbitCheckItem{}, false
		}
		rowReqs[i] = orbitRowReq{
			Active: r.active,
			Cols:   slices.Clone(r.cols),
			Vals:   vals,
			Sense:  senseToStr(r.sense),
			RHS:    rhsPay,
		}
	}
	rowState := slices.Clone(sc.rowState)
	for i, rs := range rowState {
		if rs == rowStateUnvisited {
			rowState[i] = rowStateInvariant
		}
	}
	return exportOrbitCheckItem{
		Subcommand:   "orbit-check",
		Op:           "orbit-check",
		NumVars:      n,
		IsInt:        isInt,
		LB:           lbAny,
		UB:           ubAny,
		Obj:          obj,
		Rows:         rowReqs,
		GCount:       gCount,
		Orbits:       slices.Clone(sc.orbits),
		RowTuples:    slices.Clone(sc.rowTuples),
		ColOrbit:     slices.Clone(sc.colOrbit),
		ColGroup:     slices.Clone(sc.colGroup),
		RowState:     rowState,
		RequireValid: true,
	}, true
}

// ExportSeparatedCutsItems runs live production cut separation on p via cutCertObserver
// and emits universal algebraic cut certificates for cuts-check --jsonl.
func ExportSeparatedCutsItems(p *CompiledProblem, feasibleSols [][]float64) ([][]byte, error) {
	return ExportLiveSeparatedCutsItems(p, feasibleSols)
}

func (rp *viprRatProblem) clampDualCone(y []*big.Rat) []*big.Rat {
	out := make([]*big.Rat, rp.m)
	for i := range rp.m {
		val := new(big.Rat)
		if i < len(y) && y[i] != nil {
			val.Set(y[i])
		}
		if rp.rowSense[i] == LessEqual && val.Sign() > 0 {
			val.SetInt64(0)
		}
		if rp.rowSense[i] == GreaterEqual && val.Sign() < 0 {
			val.SetInt64(0)
		}
		out[i] = val
	}
	return out
}

func (rp *viprRatProblem) evalSafeDualRat(y []*big.Rat, nodeLB, nodeUB []*big.Rat) (*big.Rat, bool) {
	colDots := make([]*big.Rat, rp.n)
	for j := range rp.n {
		colDots[j] = new(big.Rat)
	}
	rhs := new(big.Rat)
	term := new(big.Rat)
	for i := range rp.m {
		if i >= len(y) || y[i] == nil || y[i].Sign() == 0 {
			continue
		}
		yi := y[i]
		if rp.rowSense[i] == LessEqual && yi.Sign() > 0 {
			return nil, false
		}
		if rp.rowSense[i] == GreaterEqual && yi.Sign() < 0 {
			return nil, false
		}
		term.Mul(yi, rp.rowRHS[i])
		rhs.Add(rhs, term)
		for k, col := range rp.rowCols[i] {
			term.Mul(yi, rp.rowVals[i][k])
			colDots[col].Add(colDots[col], term)
		}
	}
	rc := new(big.Rat)
	for j := range rp.n {
		rc.Sub(rp.objMin[j], colDots[j])
		if rc.Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, false
			}
			term.Mul(rc, nodeLB[j])
			rhs.Add(rhs, term)
		} else if rc.Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, false
			}
			term.Mul(rc, nodeUB[j])
			rhs.Add(rhs, term)
		}
	}
	return rhs, true
}

func (rp *viprRatProblem) extractNodeSafeDual(s *LPSolver, lpRes LPResult, nodeLB, nodeUB []*big.Rat) ([]*big.Rat, *big.Rat, bool) {
	var bestY []*big.Rat
	var bestRHS *big.Rat

	tryCand := func(yRaw []*big.Rat) {
		yClean := rp.clampDualCone(yRaw)
		rhs, ok := rp.evalSafeDualRat(yClean, nodeLB, nodeUB)
		if !ok {
			return
		}
		if bestRHS == nil || rhs.Cmp(bestRHS) > 0 {
			bestY = yClean
			bestRHS = rhs
		}
	}

	if s != nil && s.ws != nil && s.TotalRows() == rp.m && rp.m > 0 {
		rhsB := make([]*big.Rat, rp.m)
		for q := range rp.m {
			bVar := s.ws.basis[q]
			if bVar >= 0 && bVar < rp.n {
				rhsB[q] = new(big.Rat).Set(rp.objMin[bVar])
			} else {
				rhsB[q] = new(big.Rat)
			}
		}
		if yBasis, ok := solveVIPRBasisTranspose(rp, s.ws.basis[:rp.m], rhsB); ok {
			tryCand(yBasis)
		}
	}

	if len(lpRes.Dual) == rp.m {
		senseSign := 1.0
		if rp.p.sense == Maximize {
			senseSign = -1.0
		}
		yFloat := make([]*big.Rat, rp.m)
		allOK := true
		for i := range rp.m {
			r, ok := exactRatFromFloat(senseSign * lpRes.Dual[i])
			if !ok {
				allOK = false
				break
			}
			yFloat[i] = r
		}
		if allOK {
			tryCand(yFloat)
		}
	}

	if bestRHS == nil {
		zeroY := make([]*big.Rat, rp.m)
		for i := range rp.m {
			zeroY[i] = new(big.Rat)
		}
		tryCand(zeroY)
	}
	return bestY, bestRHS, bestRHS != nil
}

// ExactProblemRatsForTest returns exact unmerged rational coefficients and constants
// for p, preserving original exactOrig terms and RHS expressions when present.
func ExactProblemRatsForTest(p *CompiledProblem) (
	objConst *big.Rat,
	objMin []*big.Rat,
	rowRHS []*big.Rat,
	rowCols [][]int,
	rowVals [][]*big.Rat,
	hasInexactOrig bool,
	ok bool,
) {
	if p == nil {
		return nil, nil, nil, nil, nil, false, false
	}
	hasInexactOrig = p.exactOrig != nil
	objConst, okC := exactObjConstMinRat(p)
	if !okC {
		return nil, nil, nil, nil, nil, false, false
	}
	objVec, okO := exactMinObjVecRat(p)
	if !okO || len(objVec) != p.numVars {
		return nil, nil, nil, nil, nil, false, false
	}
	m := p.numConstraints
	rowRHS = make([]*big.Rat, m)
	rowCols = make([][]int, m)
	rowVals = make([][]*big.Rat, m)
	for i := range m {
		rhs, okR := exactRowRHSRat(p, i)
		if !okR {
			return nil, nil, nil, nil, nil, false, false
		}
		cols, vals, okT := exactCanonicalRowTermsRat(p, i)
		if !okT || len(cols) != len(vals) {
			return nil, nil, nil, nil, nil, false, false
		}
		rowRHS[i] = new(big.Rat).Set(rhs)
		rowCols[i] = slices.Clone(cols)
		rowVals[i] = cloneRatSlice(vals)
	}
	return objConst, cloneRatSlice(objVec), rowRHS, rowCols, rowVals, hasInexactOrig, true
}

// ExactVarBoundsRatsForTest returns exact rational lower and upper variable bounds for p,
// preserving exactOrig non-dyadic bounds when present; nil entries denote infinite bounds.
func ExactVarBoundsRatsForTest(p *CompiledProblem) (lb, ub []*big.Rat, ok bool) {
	if p == nil {
		return nil, nil, false
	}
	n := p.numVars
	lb = make([]*big.Rat, n)
	ub = make([]*big.Rat, n)
	for j := range n {
		hasLB, lr, okL := exactVarLBRat(p, j)
		if !okL {
			return nil, nil, false
		}
		if hasLB {
			lb[j] = new(big.Rat).Set(lr)
		}
		hasUB, ur, okU := exactVarUBRat(p, j)
		if !okU {
			return nil, nil, false
		}
		if hasUB {
			ub[j] = new(big.Rat).Set(ur)
		}
	}
	return lb, ub, true
}

// ExactPolishBasisSolutionForTest exports exactPolishBasisSolutionStop to package goblin_test.
func ExactPolishBasisSolutionForTest(ctx context.Context, s *LPSolver) (ExactLPSolution, bool) {
	return exactPolishBasisSolutionStop(exactStopFromCtx(ctx), s)
}

// BuildPhase1FarkasProblemForTest builds a continuous elastic Phase-I LP from p,
// preserving raw duplicate terms and (er.rhs, er.exprConst) on inexact rows so
// exact basis duals match p's original rational formulation.
func BuildPhase1FarkasProblemForTest(p *CompiledProblem) (*CompiledProblem, bool) {
	if p == nil || p.invalidErr != nil || p.numConstraints == 0 {
		return nil, false
	}
	m := NewModel("phase1_farkas", Minimize)
	vars := make([]Var, p.numVars)
	for j := range p.numVars {
		hasLB, lr, okL := exactVarLBRat(p, j)
		hasUB, ur, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return nil, false
		}
		if eo := p.exactOrig; eo != nil && (j < len(eo.varLBRat) || j < len(eo.varUBRat)) {
			var lbArg, ubArg *big.Rat
			if hasLB {
				lbArg = lr
			}
			if hasUB {
				ubArg = ur
			}
			vars[j] = m.AddContVarRat("x"+strconv.Itoa(j), lbArg, ubArg)
		} else {
			vars[j] = m.AddContVar("x"+strconv.Itoa(j), p.varLB[j], p.varUB[j])
		}
	}
	var objTerms []Term
	for i := range p.numConstraints {
		rName := "r" + strconv.Itoa(i)
		if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasRatTerms || eo.rows[i].rhsRat != nil || eo.rows[i].exprConstRat != nil) {
			cols, vals, okC := exactCanonicalRowTermsRat(p, i)
			rhsR, okR := exactRowRHSRat(p, i)
			if !okC || !okR {
				return nil, false
			}
			terms := make([]Term, 0, len(cols)+2)
			for k, col := range cols {
				if vals[k] != nil && vals[k].Sign() != 0 {
					terms = append(terms, TRat(vars[col], vals[k]))
				}
			}
			switch p.rowSense[i] {
			case LessEqual:
				sp := m.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
				terms = append(terms, Term{Var: sp, Coeff: -1})
				objTerms = append(objTerms, Term{Var: sp, Coeff: 1})
				m.AddConstraint(rName, LeRat(NewExpr(terms...), rhsR))
			case GreaterEqual:
				sn := m.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
				terms = append(terms, Term{Var: sn, Coeff: 1})
				objTerms = append(objTerms, Term{Var: sn, Coeff: 1})
				m.AddConstraint(rName, GeRat(NewExpr(terms...), rhsR))
			case Equal:
				sp := m.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
				sn := m.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
				terms = append(terms, Term{Var: sp, Coeff: -1}, Term{Var: sn, Coeff: 1})
				objTerms = append(objTerms, Term{Var: sp, Coeff: 1}, Term{Var: sn, Coeff: 1})
				m.AddConstraint(rName, EqRat(NewExpr(terms...), rhsR))
			default:
				return nil, false
			}
			continue
		}
		var terms []Term
		rhsVal := p.rowRHS[i]
		var exprConst float64
		if eo := p.exactOrig; eo != nil && i < len(eo.rows) && (eo.rows[i].hasDupLoss || eo.rows[i].rhsInexact) {
			er := eo.rows[i]
			if er.hasDupLoss {
				terms = make([]Term, 0, (er.termEnd-er.termStart)+2)
				for _, t := range eo.rowTerms[er.termStart:er.termEnd] {
					if t.col < 0 || t.col >= p.numVars {
						return nil, false
					}
					if t.coeff != 0 {
						terms = append(terms, Term{Var: vars[t.col], Coeff: t.coeff})
					}
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
			if er.rhsInexact {
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
			sp := m.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
			terms = append(terms, Term{Var: sp, Coeff: -1})
			objTerms = append(objTerms, Term{Var: sp, Coeff: 1})
			m.AddConstraint(rName, Le(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		case GreaterEqual:
			sn := m.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
			terms = append(terms, Term{Var: sn, Coeff: 1})
			objTerms = append(objTerms, Term{Var: sn, Coeff: 1})
			m.AddConstraint(rName, Ge(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		case Equal:
			sp := m.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
			sn := m.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
			terms = append(terms, Term{Var: sp, Coeff: -1}, Term{Var: sn, Coeff: 1})
			objTerms = append(objTerms, Term{Var: sp, Coeff: 1}, Term{Var: sn, Coeff: 1})
			m.AddConstraint(rName, Eq(NewExpr(terms...).AddConstant(exprConst), rhsVal))
		default:
			return nil, false
		}
	}
	m.SetObjective(NewExpr(objTerms...))
	ep := m.Compile()
	if ep == nil || ep.invalidErr != nil {
		return nil, false
	}
	return ep, true
}

type parsedProblemRow struct {
	sense  ConstraintSense
	rhs    *big.Rat
	coeffs map[int]*big.Rat
}

func isJSONNullToken(tok jsontext.Value) bool {
	s := strings.TrimSpace(string(tok))
	return s == "" || s == "null"
}

func parseJSONIntField(tok jsontext.Value) (*big.Int, error) {
	s := strings.TrimSpace(string(tok))
	if s == "" || s == "null" {
		return nil, errors.New("missing integer token")
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(tok, &str); err != nil {
			return nil, err
		}
		s = strings.TrimSpace(str)
	}
	val, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid base-10 integer %q", s)
	}
	return val, nil
}

func parseJSONUint64Field(tok jsontext.Value) (uint64, error) {
	s := strings.TrimSpace(string(tok))
	if s == "" || s == "null" {
		return 0, errors.New("missing uint64 token")
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(tok, &str); err != nil {
			return 0, err
		}
		s = strings.TrimSpace(str)
	}
	return strconv.ParseUint(s, 0, 64)
}

func parseFormalJSONRat(tok jsontext.Value, allowBits bool) (*big.Rat, error) {
	s := strings.TrimSpace(string(tok))
	if s == "" || s == "null" {
		return nil, errors.New("missing or null rational token")
	}
	switch s[0] {
	case '{':
		var obj map[string]jsontext.Value
		if err := json.Unmarshal(tok, &obj); err != nil {
			return nil, err
		}
		if bitsTok, hasBits := obj["bits"]; hasBits && !isJSONNullToken(bitsTok) {
			if !allowBits {
				return nil, errors.New("float64 bits rational not permitted here")
			}
			bits, err := parseJSONUint64Field(bitsTok)
			if err != nil {
				return nil, fmt.Errorf("invalid float64 bits: %w", err)
			}
			f := math.Float64frombits(bits)
			r := new(big.Rat).SetFloat64(f)
			if r == nil {
				return nil, fmt.Errorf("non-finite float64 bits %#x", bits)
			}
			return r, nil
		}
		num, err := parseJSONIntField(obj["num"])
		if err != nil {
			return nil, fmt.Errorf("rational num: %w", err)
		}
		den, err := parseJSONIntField(obj["den"])
		if err != nil {
			return nil, fmt.Errorf("rational den: %w", err)
		}
		if den.Sign() <= 0 {
			return nil, fmt.Errorf("non-positive denominator %s", den.String())
		}
		return new(big.Rat).SetFrac(num, den), nil
	case '"':
		var str string
		if err := json.Unmarshal(tok, &str); err != nil {
			return nil, err
		}
		str = strings.TrimSpace(str)
		r, ok := new(big.Rat).SetString(str)
		if !ok {
			return nil, fmt.Errorf("invalid rational string %q", str)
		}
		return r, nil
	default:
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			return nil, fmt.Errorf("invalid rational number %q", s)
		}
		return r, nil
	}
}

func parseConstraintSenseTokenStrict(s string) (ConstraintSense, error) {
	switch s {
	case "le", "<=":
		return LessEqual, nil
	case "ge", ">=":
		return GreaterEqual, nil
	case "eq", "=", "==":
		return Equal, nil
	default:
		return 0, fmt.Errorf("invalid constraint sense %q", s)
	}
}

func parseJSONNonNegInt(tok jsontext.Value, field string) (int, error) {
	if isJSONNullToken(tok) {
		return 0, fmt.Errorf("missing required field %q", field)
	}
	v, err := parseJSONIntField(tok)
	if err != nil || v.Sign() < 0 || !v.IsInt64() {
		return 0, fmt.Errorf("invalid non-negative integer field %q", field)
	}
	n := v.Int64()
	if n > int64(int(^uint(0)>>1)) {
		return 0, fmt.Errorf("integer overflow in field %q", field)
	}
	return int(n), nil
}

func addRatCoeff(dst map[int]*big.Rat, col, n int, r *big.Rat) error {
	if col < 0 || col >= n {
		return fmt.Errorf("column index %d out of range [0,%d)", col, n)
	}
	if r.Sign() == 0 {
		return nil
	}
	if prev, exists := dst[col]; exists {
		sum := new(big.Rat).Add(prev, r)
		if sum.Sign() == 0 {
			delete(dst, col)
		} else {
			dst[col] = sum
		}
		return nil
	}
	dst[col] = new(big.Rat).Set(r)
	return nil
}

func parseDenseOrSparseRatVec(tok jsontext.Value, n int, allowBits, allowSparse bool, field string) ([]*big.Rat, error) {
	if isJSONNullToken(tok) {
		return nil, fmt.Errorf("missing required vector field %q", field)
	}
	s := strings.TrimSpace(string(tok))
	if allowSparse && s[0] == '{' {
		var obj map[string]jsontext.Value
		if err := json.Unmarshal(tok, &obj); err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		var terms []jsontext.Value
		if err := json.Unmarshal(obj["terms"], &terms); err != nil || isJSONNullToken(obj["terms"]) {
			return nil, fmt.Errorf("%s: invalid sparse terms", field)
		}
		acc := make(map[int]*big.Rat, len(terms))
		for _, tTok := range terms {
			var tm map[string]jsontext.Value
			if err := json.Unmarshal(tTok, &tm); err != nil {
				return nil, fmt.Errorf("%s term: %w", field, err)
			}
			col, err := parseJSONNonNegInt(tm["col"], "col")
			if err != nil {
				return nil, err
			}
			cRat, err := parseFormalJSONRat(tm["coeff"], allowBits)
			if err != nil {
				return nil, fmt.Errorf("%s coeff: %w", field, err)
			}
			if err := addRatCoeff(acc, col, n, cRat); err != nil {
				return nil, err
			}
		}
		out := make([]*big.Rat, n)
		for j := range n {
			if r, ok := acc[j]; ok {
				out[j] = r
			} else {
				out[j] = new(big.Rat)
			}
		}
		return out, nil
	}
	var elems []jsontext.Value
	if err := json.Unmarshal(tok, &elems); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if len(elems) != n {
		return nil, fmt.Errorf("%s length %d != %d", field, len(elems), n)
	}
	out := make([]*big.Rat, n)
	for j, el := range elems {
		r, err := parseFormalJSONRat(el, allowBits)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, j, err)
		}
		out[j] = r
	}
	return out, nil
}

func parseOptRatSlice(tok jsontext.Value, n int, allowBits bool, field string) ([]*big.Rat, error) {
	if isJSONNullToken(tok) {
		return nil, fmt.Errorf("missing required bounds field %q", field)
	}
	var elems []jsontext.Value
	if err := json.Unmarshal(tok, &elems); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if len(elems) != n {
		return nil, fmt.Errorf("%s length %d != %d", field, len(elems), n)
	}
	out := make([]*big.Rat, n)
	for j, el := range elems {
		if isJSONNullToken(el) {
			continue
		}
		r, err := parseFormalJSONRat(el, allowBits)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, j, err)
		}
		out[j] = r
	}
	return out, nil
}

func parseBitsRatVec(tok jsontext.Value, n int, field string) ([]*big.Rat, error) {
	var elems []jsontext.Value
	if err := json.Unmarshal(tok, &elems); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if len(elems) != n {
		return nil, fmt.Errorf("%s length %d != %d", field, len(elems), n)
	}
	out := make([]*big.Rat, n)
	for j, el := range elems {
		bits, err := parseJSONUint64Field(el)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, j, err)
		}
		f := math.Float64frombits(bits)
		r := new(big.Rat).SetFloat64(f)
		if r == nil {
			return nil, fmt.Errorf("%s[%d]: non-finite bits %#x", field, j, bits)
		}
		out[j] = r
	}
	return out, nil
}

func parseSparseRowTermArray(tok jsontext.Value, n int, allowBits bool) (map[int]*big.Rat, error) {
	var terms []jsontext.Value
	if err := json.Unmarshal(tok, &terms); err != nil {
		return nil, err
	}
	acc := make(map[int]*big.Rat, len(terms))
	for _, tTok := range terms {
		var tm map[string]jsontext.Value
		if err := json.Unmarshal(tTok, &tm); err != nil {
			return nil, err
		}
		col, err := parseJSONNonNegInt(tm["col"], "col")
		if err != nil {
			return nil, err
		}
		cRat, err := parseFormalJSONRat(tm["coeff"], allowBits)
		if err != nil {
			return nil, err
		}
		if err := addRatCoeff(acc, col, n, cRat); err != nil {
			return nil, err
		}
	}
	return acc, nil
}

func verifyJSONProblemMatchesCompiled(
	p *CompiledProblem,
	gotVars, gotRows int,
	gotObjConst *big.Rat,
	gotObj, gotLB, gotUB []*big.Rat,
	gotCon []parsedProblemRow,
) error {
	if p == nil {
		return errors.New("nil CompiledProblem")
	}
	wantVars := p.numVars
	wantRows := p.numConstraints
	if gotVars != wantVars {
		return fmt.Errorf("serialized numVars=%d != CompiledProblem NumVars=%d", gotVars, wantVars)
	}
	if gotRows != wantRows || len(gotCon) != wantRows {
		return fmt.Errorf("serialized numRows=%d != CompiledProblem NumConstraints=%d", gotRows, wantRows)
	}
	wantObjConst, wantObj, wantRowRHS, wantRowCols, wantRowVals, _, okRats := ExactProblemRatsForTest(p)
	if !okRats || len(wantObj) != wantVars || len(wantRowRHS) != wantRows || len(wantRowCols) != wantRows || len(wantRowVals) != wantRows {
		return errors.New("failed to extract exact CompiledProblem rationals")
	}
	if gotObjConst.Cmp(wantObjConst) != 0 {
		return fmt.Errorf("serialized objConst=%s != CompiledProblem minObjConst=%s",
			gotObjConst.RatString(), wantObjConst.RatString())
	}
	if len(gotObj) != wantVars || len(gotLB) != wantVars || len(gotUB) != wantVars {
		return fmt.Errorf("serialized column vector lengths mismatch numVars=%d", wantVars)
	}
	for j := range wantVars {
		if gotObj[j].Cmp(wantObj[j]) != 0 {
			return fmt.Errorf("serialized obj[%d]=%s != CompiledProblem minObj=%s",
				j, gotObj[j].RatString(), wantObj[j].RatString())
		}
		hasLB, wantLB, okL := exactVarLBRat(p, j)
		hasUB, wantUB, okU := exactVarUBRat(p, j)
		if !okL || !okU {
			return fmt.Errorf("invalid CompiledProblem bounds [%v,%v] at col %d", p.varLB[j], p.varUB[j], j)
		}
		if !hasLB {
			if gotLB[j] != nil {
				return fmt.Errorf("serialized lb[%d]=%s != -Inf", j, gotLB[j].RatString())
			}
		} else if gotLB[j] == nil || gotLB[j].Cmp(wantLB) != 0 {
			return fmt.Errorf("serialized lb[%d] mismatch (want %s)", j, wantLB.RatString())
		}
		if !hasUB {
			if gotUB[j] != nil {
				return fmt.Errorf("serialized ub[%d]=%s != +Inf", j, gotUB[j].RatString())
			}
		} else if gotUB[j] == nil || gotUB[j].Cmp(wantUB) != 0 {
			return fmt.Errorf("serialized ub[%d] mismatch (want %s)", j, wantUB.RatString())
		}
	}
	for i := range wantRows {
		if gotCon[i].sense != p.rowSense[i] {
			return fmt.Errorf("serialized row[%d].sense=%v != %v", i, gotCon[i].sense, p.rowSense[i])
		}
		if gotCon[i].rhs.Cmp(wantRowRHS[i]) != 0 {
			return fmt.Errorf("serialized row[%d].rhs=%s != %s",
				i, gotCon[i].rhs.RatString(), wantRowRHS[i].RatString())
		}
		if len(wantRowCols[i]) != len(wantRowVals[i]) {
			return fmt.Errorf("exact row[%d] column/value length mismatch", i)
		}
		wantCoeffs := make(map[int]*big.Rat, len(wantRowCols[i]))
		for k, col := range wantRowCols[i] {
			if err := addRatCoeff(wantCoeffs, col, wantVars, wantRowVals[i][k]); err != nil {
				return err
			}
		}
		if len(gotCon[i].coeffs) != len(wantCoeffs) {
			return fmt.Errorf("serialized row[%d] nonzero count=%d != %d",
				i, len(gotCon[i].coeffs), len(wantCoeffs))
		}
		for col, wv := range wantCoeffs {
			gv, ok := gotCon[i].coeffs[col]
			if !ok || gv.Cmp(wv) != 0 {
				return fmt.Errorf("serialized row[%d] col %d coeff mismatch", i, col)
			}
		}
	}
	return nil
}

func verifySerializedVIPRMatchesProblem(p *CompiledProblem, raw []byte) error {
	if p == nil {
		return errors.New("nil CompiledProblem")
	}
	var top map[string]jsontext.Value
	if err := json.Unmarshal(raw, &top); err != nil {
		return fmt.Errorf("unmarshal VIPR JSON object: %w", err)
	}
	inst := top
	if origTok, hasOrig := top["orig"]; hasOrig && !isJSONNullToken(origTok) {
		var origMap map[string]jsontext.Value
		if err := json.Unmarshal(origTok, &origMap); err != nil {
			return fmt.Errorf("unmarshal VIPR orig instance: %w", err)
		}
		inst = origMap
	} else if preTok, hasPre := top["presolve"]; hasPre && !isJSONNullToken(preTok) {
		return errors.New("serialized VIPR contains non-null presolve without orig instance")
	}

	n, err := parseJSONNonNegInt(inst["n"], "n")
	if err != nil {
		return err
	}
	objConst := new(big.Rat)
	if ocTok, hasOC := inst["objConst"]; hasOC && !isJSONNullToken(ocTok) {
		objConst, err = parseFormalJSONRat(ocTok, false)
		if err != nil {
			return fmt.Errorf("VIPR objConst: %w", err)
		}
	}
	obj, err := parseDenseOrSparseRatVec(inst["obj"], n, false, true, "obj")
	if err != nil {
		return fmt.Errorf("VIPR %w", err)
	}
	var isInt []bool
	if err := json.Unmarshal(inst["isInt"], &isInt); err != nil || isJSONNullToken(inst["isInt"]) {
		return errors.New("VIPR missing or invalid isInt")
	}
	if len(isInt) != n {
		return fmt.Errorf("VIPR isInt length %d != n=%d", len(isInt), n)
	}
	lb, err := parseOptRatSlice(inst["lb"], n, false, "lb")
	if err != nil {
		return fmt.Errorf("VIPR %w", err)
	}
	ub, err := parseOptRatSlice(inst["ub"], n, false, "ub")
	if err != nil {
		return fmt.Errorf("VIPR %w", err)
	}
	var rowToks []jsontext.Value
	if err := json.Unmarshal(inst["baseRows"], &rowToks); err != nil || isJSONNullToken(inst["baseRows"]) {
		return errors.New("VIPR missing or invalid baseRows")
	}
	rows := make([]parsedProblemRow, len(rowToks))
	for i, rTok := range rowToks {
		var rm map[string]jsontext.Value
		if err := json.Unmarshal(rTok, &rm); err != nil {
			return fmt.Errorf("VIPR baseRows[%d]: %w", i, err)
		}
		var senseStr string
		if err := json.Unmarshal(rm["sense"], &senseStr); err != nil || isJSONNullToken(rm["sense"]) {
			return fmt.Errorf("VIPR baseRows[%d] invalid sense", i)
		}
		sense, err := parseConstraintSenseTokenStrict(senseStr)
		if err != nil {
			return fmt.Errorf("VIPR baseRows[%d]: %w", i, err)
		}
		rhs, err := parseFormalJSONRat(rm["rhs"], false)
		if err != nil {
			return fmt.Errorf("VIPR baseRows[%d].rhs: %w", i, err)
		}
		coeffsTok := rTok
		if ct, hasCoeffs := rm["coeffs"]; hasCoeffs && !isJSONNullToken(ct) {
			coeffsTok = ct
		}
		vec, err := parseDenseOrSparseRatVec(coeffsTok, n, false, true, "baseRows.coeffs")
		if err != nil {
			return fmt.Errorf("VIPR baseRows[%d]: %w", i, err)
		}
		cmap := make(map[int]*big.Rat, len(vec))
		for j, v := range vec {
			if err := addRatCoeff(cmap, j, n, v); err != nil {
				return err
			}
		}
		rows[i] = parsedProblemRow{sense: sense, rhs: rhs, coeffs: cmap}
	}
	if err := verifyJSONProblemMatchesCompiled(p, n, len(rows), objConst, obj, lb, ub, rows); err != nil {
		return fmt.Errorf("VIPR problem mismatch: %w", err)
	}
	for j := range n {
		wantInt := p.varCat[j] != Continuous
		if isInt[j] != wantInt {
			return fmt.Errorf("VIPR problem mismatch: isInt[%d]=%v != %v", j, isInt[j], wantInt)
		}
	}
	return nil
}

// VerifySerializedVIPRMatchesProblemForTest parses the exact VIPR problem instance
// inside raw and checks it against p's exact unmerged rational model.
func VerifySerializedVIPRMatchesProblemForTest(p *CompiledProblem, raw []byte) error {
	return verifySerializedVIPRMatchesProblem(p, raw)
}

func resolveLPCheckMode(top map[string]jsontext.Value) (string, error) {
	mode := ""
	setMode := func(next string) error {
		switch next {
		case "safe":
			next = "safe-bound"
		case "boundConflict", "bound_conflict":
			next = "bound-conflict"
		}
		if mode != "" && mode != next {
			return fmt.Errorf("conflicting LP check modes %q and %q", mode, next)
		}
		mode = next
		return nil
	}
	for _, key := range []string{"mode", "subcommand", "op"} {
		if tok, ok := top[key]; ok && !isJSONNullToken(tok) {
			var s string
			if err := json.Unmarshal(tok, &s); err != nil {
				return "", fmt.Errorf("invalid LP check %q: %w", key, err)
			}
			if err := setMode(s); err != nil {
				return "", err
			}
		}
	}
	if tok, ok := top["safe"]; ok && !isJSONNullToken(tok) {
		var b bool
		if err := json.Unmarshal(tok, &b); err != nil {
			return "", fmt.Errorf("invalid LP check safe flag: %w", err)
		}
		if b {
			if err := setMode("safe-bound"); err != nil {
				return "", err
			}
		} else if err := setMode("optimal"); err != nil {
			return "", err
		}
	}
	if mode != "" {
		return mode, nil
	}
	for _, key := range []string{"reduced", "lpReduced", "presolved"} {
		if tok, ok := top[key]; ok && !isJSONNullToken(tok) {
			return "lp-presolve", nil
		}
	}
	return "", errors.New("missing LP check mode")
}

func parseSerializedLPIntFlags(top map[string]jsontext.Value, n int) ([]bool, error) {
	parseMask := func(tok jsontext.Value, name string) ([]bool, error) {
		var mask []bool
		if err := json.Unmarshal(tok, &mask); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if len(mask) != n {
			return nil, fmt.Errorf("%s length %d != numVars=%d", name, len(mask), n)
		}
		return mask, nil
	}
	parseCols := func(tok jsontext.Value, name string) ([]bool, error) {
		var cols []int
		if err := json.Unmarshal(tok, &cols); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		mask := make([]bool, n)
		for _, c := range cols {
			if c < 0 || c >= n {
				return nil, fmt.Errorf("%s col %d out of range [0,%d)", name, c, n)
			}
			mask[c] = true
		}
		return mask, nil
	}
	var maskOut, colsOut []bool
	if t, ok := top["isInt"]; ok && !isJSONNullToken(t) {
		var err error
		if maskOut, err = parseMask(t, "isInt"); err != nil {
			return nil, err
		}
	}
	if t, ok := top["intMask"]; ok && !isJSONNullToken(t) {
		m2, err := parseMask(t, "intMask")
		if err != nil {
			return nil, err
		}
		if maskOut != nil {
			for j := range n {
				if maskOut[j] != m2[j] {
					return nil, errors.New("conflicting isInt and intMask")
				}
			}
		}
		maskOut = m2
	}
	if t, ok := top["intCols"]; ok && !isJSONNullToken(t) {
		var err error
		if colsOut, err = parseCols(t, "intCols"); err != nil {
			return nil, err
		}
	}
	if t, ok := top["integerCols"]; ok && !isJSONNullToken(t) {
		c2, err := parseCols(t, "integerCols")
		if err != nil {
			return nil, err
		}
		if colsOut != nil {
			for j := range n {
				if colsOut[j] != c2[j] {
					return nil, errors.New("conflicting intCols and integerCols")
				}
			}
		}
		colsOut = c2
	}
	if maskOut != nil && colsOut != nil {
		for j := range n {
			if maskOut[j] != colsOut[j] {
				return nil, errors.New("conflicting integer mask and column list")
			}
		}
	}
	final := maskOut
	if final == nil {
		final = colsOut
	}
	if final == nil {
		return nil, errors.New("milp-unbounded payload missing integer column specification")
	}
	return final, nil
}

func verifySerializedLPPayloadMatchesProblem(p *CompiledProblem, payload []byte) error {
	if p == nil {
		return errors.New("nil CompiledProblem")
	}
	var top map[string]jsontext.Value
	if err := json.Unmarshal(payload, &top); err != nil {
		return fmt.Errorf("unmarshal lp-check JSON object: %w", err)
	}
	mode, err := resolveLPCheckMode(top)
	if err != nil {
		return err
	}

	inst := top
	if mode == "lp-presolve" {
		if ot, ok := top["orig"]; ok && !isJSONNullToken(ot) {
			var om map[string]jsontext.Value
			if err := json.Unmarshal(ot, &om); err != nil {
				return fmt.Errorf("unmarshal lp-presolve orig: %w", err)
			}
			inst = om
		} else if lot, ok := top["lpOrig"]; ok && !isJSONNullToken(lot) {
			var om map[string]jsontext.Value
			if err := json.Unmarshal(lot, &om); err != nil {
				return fmt.Errorf("unmarshal lp-presolve lpOrig: %w", err)
			}
			inst = om
		}
	}

	numVars, err := parseJSONNonNegInt(inst["numVars"], "numVars")
	if err != nil {
		return err
	}
	numRows, err := parseJSONNonNegInt(inst["numRows"], "numRows")
	if err != nil {
		return err
	}
	objConst := new(big.Rat)
	if ocTok, hasOC := inst["objConst"]; hasOC && !isJSONNullToken(ocTok) {
		objConst, err = parseFormalJSONRat(ocTok, true)
		if err != nil {
			return fmt.Errorf("lp objConst: %w", err)
		}
	}
	var obj []*big.Rat
	if objTok, hasObj := inst["obj"]; hasObj && !isJSONNullToken(objTok) {
		obj, err = parseDenseOrSparseRatVec(objTok, numVars, true, false, "obj")
	} else if allowBits := mode == "lp-presolve" || mode == "unbounded" || mode == "milp-unbounded"; allowBits {
		if bitsTok, hasBits := inst["objBits"]; hasBits && !isJSONNullToken(bitsTok) {
			obj, err = parseBitsRatVec(bitsTok, numVars, "objBits")
		} else {
			err = errors.New("missing obj")
		}
	} else {
		err = errors.New("missing obj")
	}
	if err != nil {
		return fmt.Errorf("lp %w", err)
	}
	lb, err := parseOptRatSlice(inst["lb"], numVars, true, "lb")
	if err != nil {
		return fmt.Errorf("lp %w", err)
	}
	ub, err := parseOptRatSlice(inst["ub"], numVars, true, "ub")
	if err != nil {
		return fmt.Errorf("lp %w", err)
	}
	var rowToks []jsontext.Value
	if err := json.Unmarshal(inst["rows"], &rowToks); err != nil || isJSONNullToken(inst["rows"]) {
		return errors.New("lp missing or invalid rows")
	}
	if len(rowToks) != numRows {
		return fmt.Errorf("lp rows length %d != numRows=%d", len(rowToks), numRows)
	}
	rows := make([]parsedProblemRow, numRows)
	for i, rTok := range rowToks {
		var rm map[string]jsontext.Value
		if err := json.Unmarshal(rTok, &rm); err != nil {
			return fmt.Errorf("lp rows[%d]: %w", i, err)
		}
		var senseStr string
		if err := json.Unmarshal(rm["sense"], &senseStr); err != nil || isJSONNullToken(rm["sense"]) {
			return fmt.Errorf("lp rows[%d] invalid sense", i)
		}
		sense, err := parseConstraintSenseTokenStrict(senseStr)
		if err != nil {
			return fmt.Errorf("lp rows[%d]: %w", i, err)
		}
		rhs, err := parseFormalJSONRat(rm["rhs"], true)
		if err != nil {
			return fmt.Errorf("lp rows[%d].rhs: %w", i, err)
		}
		if isJSONNullToken(rm["terms"]) {
			return fmt.Errorf("lp rows[%d] missing terms", i)
		}
		coeffs, err := parseSparseRowTermArray(rm["terms"], numVars, true)
		if err != nil {
			return fmt.Errorf("lp rows[%d].terms: %w", i, err)
		}
		rows[i] = parsedProblemRow{sense: sense, rhs: rhs, coeffs: coeffs}
	}
	if err := verifyJSONProblemMatchesCompiled(p, numVars, numRows, objConst, obj, lb, ub, rows); err != nil {
		return fmt.Errorf("lp-check (%s) problem mismatch: %w", mode, err)
	}
	if mode == "milp-unbounded" {
		intFlags, err := parseSerializedLPIntFlags(top, numVars)
		if err != nil {
			return fmt.Errorf("milp-unbounded: %w", err)
		}
		for j := range numVars {
			wantInt := p.varCat[j] != Continuous
			if intFlags[j] != wantInt {
				return fmt.Errorf("milp-unbounded int flag[%d]=%v != %v", j, intFlags[j], wantInt)
			}
		}
	}
	return nil
}

// VerifySerializedLPPayloadMatchesProblemForTest parses the serialized problem inside
// an lp-check payload and checks it against p's exact unmerged rational model.
func VerifySerializedLPPayloadMatchesProblemForTest(p *CompiledProblem, payload []byte) error {
	return verifySerializedLPPayloadMatchesProblem(p, payload)
}
