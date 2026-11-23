package goblin

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"math"
	"math/big"
	"slices"
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
	objConst, err := float64ToPayload(p.objConstMin())
	if err != nil {
		return lpCertPayload{}, false
	}
	obj := make([]ratPayload, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		cRat, cErr := float64ToPayload(p.objMin[j])
		if cErr != nil {
			return lpCertPayload{}, false
		}
		obj[j] = cRat
		loPtr, okL := exportOptRatPayload(p.varLB[j], -1)
		if !okL {
			return lpCertPayload{}, false
		}
		hiPtr, okU := exportOptRatPayload(p.varUB[j], 1)
		if !okU {
			return lpCertPayload{}, false
		}
		lb[j] = loPtr
		ub[j] = hiPtr
	}
	rows := make([]lpRowPayload, m)
	for i := range m {
		rhs, rErr := float64ToPayload(p.rowRHS[i])
		if rErr != nil {
			return lpCertPayload{}, false
		}
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		terms := make([]lpTermPayload, 0, en-st)
		for k := st; k < en; k++ {
			coeff, tErr := float64ToPayload(p.rowVal[k])
			if tErr != nil {
				return lpCertPayload{}, false
			}
			terms = append(terms, lpTermPayload{Col: p.colIdx[k], Coeff: coeff})
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
	bndRat := new(big.Rat)
	for j := range n {
		xj := xRat[j]
		if xj == nil {
			return false
		}
		if !math.IsInf(p.varLB[j], -1) {
			if bndRat.SetFloat64(p.varLB[j]) == nil || xj.Cmp(bndRat) < 0 {
				return false
			}
		}
		if !math.IsInf(p.varUB[j], 1) {
			if bndRat.SetFloat64(p.varUB[j]) == nil || xj.Cmp(bndRat) > 0 {
				return false
			}
		}
	}
	act := new(big.Rat)
	aRat := new(big.Rat)
	prod := new(big.Rat)
	rhsRat := new(big.Rat)
	for i := range m {
		if rhsRat.SetFloat64(p.rowRHS[i]) == nil {
			return false
		}
		act.SetInt64(0)
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if val == 0 || xRat[col].Sign() == 0 {
				continue
			}
			if aRat.SetFloat64(val) == nil {
				return false
			}
			prod.Mul(aRat, xRat[col])
			act.Add(act, prod)
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
	objMin := new(big.Rat)
	if objMin.SetFloat64(p.objConstMin()) == nil {
		return false
	}
	for j := range n {
		cj := p.objMin[j]
		if cj == 0 || xRat[j].Sign() == 0 {
			continue
		}
		if aRat.SetFloat64(cj) == nil {
			return false
		}
		prod.Mul(aRat, xRat[j])
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
				r, okR := viprRatFromFloat(yi)
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
				xr, okX := viprRatFromFloat(res.Primal[j])
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
	if p == nil || p.numVars == 0 || p.numConstraints == 0 {
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
	objConstRat, okOC := viprRatFromFloat(p.objConstMin())
	if !okOC {
		return nil, false
	}
	curObj := make([]*big.Rat, n)
	curLB := make([]*big.Rat, n)
	curUB := make([]*big.Rat, n)
	for j := range n {
		cj, okC := viprRatFromFloat(p.objMin[j])
		if !okC {
			return nil, false
		}
		curObj[j] = cj
		if !math.IsInf(p.varLB[j], -1) {
			lr, okL := viprRatFromFloat(p.varLB[j])
			if !okL {
				return nil, false
			}
			curLB[j] = lr
		}
		if !math.IsInf(p.varUB[j], 1) {
			ur, okU := viprRatFromFloat(p.varUB[j])
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
		rhsR, okR := viprRatFromFloat(p.rowRHS[i])
		if !okR {
			return nil, false
		}
		cm := make(map[int]*big.Rat, p.rowPtr[i+1]-p.rowPtr[i])
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			v := p.rowVal[k]
			if v == 0 {
				continue
			}
			vr, okV := viprRatFromFloat(v)
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
			if q < 0 {
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
			if uint(col) >= uint(n) {
				return nil, false
			}
			if ev.toLower {
				if curLB[col] == nil {
					return nil, false
				}
				curUB[col] = new(big.Rat).Set(curLB[col])
			} else {
				if curUB[col] == nil {
					return nil, false
				}
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
			if uint(col) >= uint(n) || curObj[col].Sign() != 0 {
				return nil, false
			}
			valR, okV := viprRatFromFloat(ev.val)
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
	origRHSRat, okO := viprRatFromFloat(origRHS)
	rhsCompRat, okC := viprRatFromFloat(rhsComp)
	if !okO || !okC || len(cols) != len(origVals) || len(cols) != len(intCoeffs) {
		return false
	}
	maxShift := new(big.Rat)
	minShift := new(big.Rat)
	diff := new(big.Rat)
	tLo := new(big.Rat)
	tHi := new(big.Rat)
	for k, col := range cols {
		vRat, okV := viprRatFromFloat(origVals[k])
		if !okV {
			return false
		}
		rcRat := big.NewRat(intCoeffs[k], 1)
		diff.Sub(rcRat, vRat)
		if diff.Sign() == 0 {
			continue
		}
		loR, okL := viprRatFromFloat(lb[col])
		hiR, okU := viprRatFromFloat(ub[col])
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
				a2, okA2 := viprRatFromFloat(preVals[jIdx])
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
				br, okB := viprRatFromFloat(bd)
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
