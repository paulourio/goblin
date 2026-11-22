package goblin

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
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

// ExportLPCheckPayload builds a single-line JSON request for lp-check --jsonl
// from an LP solve on p ("safe-bound" on StatusOptimal, "farkas" on StatusInfeasible,
// and "unbounded" / "milp-unbounded" on StatusUnbounded).
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
		cert, okBase := exportCompiledProblemToLPPayload(p, "safe-bound")
		if !okBase {
			return "", nil, false
		}
		if len(res.Dual) == rp.m {
			senseSign := 1.0
			if p.sense == Maximize {
				senseSign = -1.0
			}
			yFloat := make([]*big.Rat, rp.m)
			yBits := make([]uint64, rp.m)
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
				yClean := rp.clampDualCone(yFloat)
				if _, okDual := rp.evalSafeDualRat(yClean, rp.lb, rp.ub); okDual {
					cert.YBits = yBits
					raw, err := json.Marshal(cert)
					if err != nil {
						return "", nil, false
					}
					return "safe-bound", raw, true
				}
			}
		}
		if lpSolver != nil && lpSolver.TotalRows() == rp.m && solverMatchesProblemBounds(lpSolver, p) {
			if yMin, _, okExact := ExactBasisDuals(context.Background(), lpSolver); okExact && len(yMin) == rp.m {
				cert.Y = ratSliceToPayloads(yMin)
				raw, err := json.Marshal(cert)
				if err != nil {
					return "", nil, false
				}
				return "safe-bound", raw, true
			}
		}
		if lpSolver != nil && lpSolver.ws != nil && lpSolver.TotalRows() == rp.m {
			yRat, _, okDual := rp.extractNodeSafeDual(lpSolver, res, rp.lb, rp.ub)
			if okDual {
				yPay := make([]ratPayload, rp.m)
				for i := range rp.m {
					yPay[i] = bigRatToPayload(yRat[i])
				}
				cert.Y = yPay
				raw, err := json.Marshal(cert)
				if err != nil {
					return "", nil, false
				}
				return "safe-bound", raw, true
			}
		}
		return "", nil, false

	case StatusInfeasible:
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

func paginatedPresolveRowIndices(m, limit int) []int {
	if m <= 0 || limit <= 0 {
		return nil
	}
	if m <= limit {
		out := make([]int, m)
		for i := range m {
			out[i] = i
		}
		return out
	}
	if limit == 1 {
		return []int{0}
	}
	out := make([]int, 0, limit)
	prev := -1
	for k := range limit {
		idx := int(int64(k) * int64(m-1) / int64(limit-1))
		if idx > prev {
			out = append(out, idx)
			prev = idx
		}
	}
	return out
}

func buildExportGcdDivCheckItem(sense ConstraintSense, coeffs []int64, g int64, rhsVal float64) (exportGcdDivCheckItem, bool) {
	if len(coeffs) == 0 || g <= 0 {
		return exportGcdDivCheckItem{}, false
	}
	rhsPay, errRHS := float64ToPayload(rhsVal)
	if errRHS != nil {
		return exportGcdDivCheckItem{}, false
	}
	rhsRat := new(big.Rat).SetFloat64(rhsVal)
	if rhsRat == nil {
		return exportGcdDivCheckItem{}, false
	}
	qRat := new(big.Rat).Quo(rhsRat, big.NewRat(g, 1))
	senseStr := senseToStr(sense)
	candG := g

	switch sense {
	case Equal:
		if !qRat.IsInt() {
			expInf := true
			return exportGcdDivCheckItem{
				Subcommand:       "gcd-div-check",
				Op:               "gcd-div-check",
				Coeffs:           coeffs,
				Sense:            senseStr,
				RHS:              rhsPay,
				Divisor:          &candG,
				ExpectInfeasible: &expInf,
			}, true
		}
		if !qRat.Num().IsInt64() {
			return exportGcdDivCheckItem{}, false
		}
		divCoeffs := make([]int64, len(coeffs))
		for k, c := range coeffs {
			divCoeffs[k] = c / g
		}
		divRhs := qRat.Num().Int64()
		expInf := false
		return exportGcdDivCheckItem{
			Subcommand:       "gcd-div-check",
			Op:               "gcd-div-check",
			Coeffs:           coeffs,
			Sense:            senseStr,
			RHS:              rhsPay,
			Divisor:          &candG,
			ExpectInfeasible: &expInf,
			DivCoeffs:        divCoeffs,
			DivRHS:           &divRhs,
		}, true

	case LessEqual:
		flRat := viprRatFloor(qRat)
		if !flRat.Num().IsInt64() {
			return exportGcdDivCheckItem{}, false
		}
		exactFloor := flRat.Num().Int64()
		ubInt := int64(math.Floor(rhsVal + presolveIntTol))
		goDivRhs := divFloorInt64(ubInt, g)
		effRHS := rhsPay
		if goDivRhs > exactFloor {
			effRHS = int64ToPayload(ubInt)
		}
		divCoeffs := make([]int64, len(coeffs))
		for k, c := range coeffs {
			divCoeffs[k] = c / g
		}
		expInf := false
		return exportGcdDivCheckItem{
			Subcommand:       "gcd-div-check",
			Op:               "gcd-div-check",
			Coeffs:           coeffs,
			Sense:            senseStr,
			RHS:              effRHS,
			Divisor:          &candG,
			ExpectInfeasible: &expInf,
			DivCoeffs:        divCoeffs,
			DivRHS:           &goDivRhs,
		}, true

	case GreaterEqual:
		clRat := viprRatCeil(qRat)
		if !clRat.Num().IsInt64() {
			return exportGcdDivCheckItem{}, false
		}
		exactCeil := clRat.Num().Int64()
		lbInt := int64(math.Ceil(rhsVal - presolveIntTol))
		goDivRhs := divCeilInt64(lbInt, g)
		effRHS := rhsPay
		if goDivRhs < exactCeil {
			effRHS = int64ToPayload(lbInt)
		}
		divCoeffs := make([]int64, len(coeffs))
		for k, c := range coeffs {
			divCoeffs[k] = c / g
		}
		expInf := false
		return exportGcdDivCheckItem{
			Subcommand:       "gcd-div-check",
			Op:               "gcd-div-check",
			Coeffs:           coeffs,
			Sense:            senseStr,
			RHS:              effRHS,
			Divisor:          &candG,
			ExpectInfeasible: &expInf,
			DivCoeffs:        divCoeffs,
			DivRHS:           &goDivRhs,
		}, true

	default:
		return exportGcdDivCheckItem{}, false
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
	Subcommand   string      `json:"subcommand"`
	Op           string      `json:"op"`
	RHS          ratPayload  `json:"rhs"`
	A            ratPayload  `json:"a"`
	MaxOther     ratPayload  `json:"max_other"`
	Surplus      ratPayload  `json:"surplus"`
	NewA         *ratPayload `json:"new_a"`
	NewRHS       *ratPayload `json:"new_rhs"`
	RequireValid bool        `json:"require_valid"`
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

// ExportPresolveRulesItems runs Presolve on p and extracts real JSONL verification items
// for rules-check --jsonl covering GCD infeasibility, Euclidean GCD row division, row
// activity tightening, skip-term guards, Savelsbergh binary coefficient strengthening,
// and orbit symmetry verification.
func ExportPresolveRulesItems(p *CompiledProblem) (items [][]byte, preRes PresolveResult) {
	if p == nil {
		return nil, PresolveResult{}
	}
	preRes = Presolve(p, defaultPresolveMaxPass)
	n := p.numVars
	m := p.numConstraints
	if n == 0 || m == 0 {
		return nil, preRes
	}

	lb := slices.Clone(p.varLB)
	ub := slices.Clone(p.varUB)
	for j := range n {
		if lb[j] > ub[j] {
			return nil, preRes
		}
	}

	sampledRows := paginatedPresolveRowIndices(m, exportMaxPresolveRows)

	// 1. GCD infeasibility (gcd-infeas) and Euclidean GCD row division (gcd-div-check).
	if p.isMIP {
		gcdEmitted := 0
		for i := range m {
			if gcdEmitted >= exportMaxPresolveRows {
				break
			}
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			rhsVal := p.rowRHS[i]
			if st == en || math.Abs(rhsVal) > 0x1p62 || math.IsNaN(rhsVal) || math.IsInf(rhsVal, 0) {
				continue
			}
			allExactInt := true
			var g int64
			coeffs := make([]int64, 0, en-st)
			for k := st; k < en; k++ {
				col := p.colIdx[k]
				if p.varCat[col] == Continuous {
					allExactInt = false
					break
				}
				v := p.rowVal[k]
				rv := math.Round(v)
				if v != rv || math.Abs(rv) > presolveGCDMaxCoeff {
					allExactInt = false
					break
				}
				cInt := int64(rv)
				coeffs = append(coeffs, cInt)
				if iv := int64(math.Abs(rv)); iv > 0 {
					g = gcdInt64(g, iv)
				}
			}
			if !allExactInt || g <= 0 {
				continue
			}
			emittedRow := false
			if p.rowSense[i] == Equal {
				loInt := int64(math.Ceil(rhsVal - presolveFeasTol))
				hiInt := int64(math.Floor(rhsVal + presolveFeasTol))
				q := divCeilInt64(loInt, g)
				goInf := q*g > hiInt
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
					emittedRow = true
				}
			}
			if math.Abs(rhsVal) <= 0x1p52 {
				if divItem, okDiv := buildExportGcdDivCheckItem(p.rowSense[i], coeffs, g, rhsVal); okDiv {
					if rawDiv, errDiv := json.Marshal(divItem); errDiv == nil {
						items = append(items, rawDiv)
						emittedRow = true
					}
				}
			}
			if emittedRow {
				gcdEmitted++
			}
		}
	}

	// 2. Activity summary, implied bounds, and presolveCanSkipTerm guards.
	for _, i := range sampledRows {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		if st == en {
			continue
		}
		cols := p.colIdx[st:en]
		vals := p.rowVal[st:en]
		rhs := p.rowRHS[i]
		rhsPay, errRHS := float64ToPayload(rhs)
		if errRHS != nil {
			continue
		}
		var s rowActivitySummary
		evalRowActivitySummaryInto(&s, cols, vals, lb, ub, rhs, false, true, true)
		if s.extreme {
			continue
		}
		hasUpper := p.rowSense[i] == LessEqual || p.rowSense[i] == Equal
		hasLower := p.rowSense[i] == GreaterEqual || p.rowSense[i] == Equal
		rowScale := max(1.0, max(s.l1Min, s.l1Max))
		rowFeasTol := max(presolveFeasTol, presolveRelScaleTol*rowScale)

		terms := make([]exportActivityTermItem, 0, len(cols))
		allOK := true
		skipEmitted := 0
		for k, col := range cols {
			a := vals[k]
			aPay, errA := float64ToPayload(a)
			lPtr, okL := exportOptRatPayload(lb[col], -1)
			uPtr, okU := exportOptRatPayload(ub[col], 1)
			if errA != nil || !okL || !okU {
				allOK = false
				break
			}
			isInt := p.varCat[col] != Continuous
			item := exportActivityTermItem{
				A:     aPay,
				L:     lPtr,
				U:     uPtr,
				IsInt: isInt,
			}
			if math.Abs(a) > presolveZeroTol {
				l, u := lb[col], ub[col]
				if hasUpper && s.minInfCount <= 1 {
					canGuardUB := s.minInfCount == 0 && !s.useWideSlack
					if a > 0 && (s.minInfCount == 0 || math.IsInf(l, -1)) {
						if canGuardUB && skipEmitted < exportMaxSkipPerRow && !math.IsInf(l, -1) && !math.IsInf(u, 1) && s.slackLo > 0 {
							if sPay, errS := float64ToPayload(s.slackLo); errS == nil && lPtr != nil && uPtr != nil {
								goSkip := presolveCanSkipTerm(s.slackLo, a, l, u, u, isInt)
								if rawSkip, errM := json.Marshal(exportActivitySkipItem{
									Subcommand:  "activity",
									Op:          "skip",
									SlackLo:     sPay,
									AbsA:        aPay,
									L:           *lPtr,
									U:           *uPtr,
									TargetBound: *uPtr,
									IsInt:       isInt,
									GoSkip:      goSkip,
								}); errM == nil {
									items = append(items, rawSkip)
									skipEmitted++
								}
							}
						}
						if qVal, qEB, qOK := s.residualQuotient(cols, vals, lb, ub, rhs, k, true); qOK {
							newUB := qVal + qEB
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
							if isInt {
								newUB = math.Floor(newUB + bTol)
							} else {
								newUB += rowFeasTol / a
							}
							if newUB < u-presolveIntTol {
								if gPay, errG := float64ToPayload(newUB); errG == nil {
									item.GoUB = &gPay
								}
							}
						}
					} else if a < 0 && (s.minInfCount == 0 || math.IsInf(u, 1)) {
						absA := -a
						if canGuardUB && skipEmitted < exportMaxSkipPerRow && !math.IsInf(l, -1) && !math.IsInf(u, 1) && s.slackLo > 0 {
							if sPay, errS := float64ToPayload(s.slackLo); errS == nil && lPtr != nil && uPtr != nil {
								if absAPay, errAA := float64ToPayload(absA); errAA == nil {
									goSkip := presolveCanSkipTerm(s.slackLo, absA, l, u, l, isInt)
									if rawSkip, errM := json.Marshal(exportActivitySkipItem{
										Subcommand:  "activity",
										Op:          "skip",
										SlackLo:     sPay,
										AbsA:        absAPay,
										L:           *lPtr,
										U:           *uPtr,
										TargetBound: *lPtr,
										IsInt:       isInt,
										GoSkip:      goSkip,
									}); errM == nil {
										items = append(items, rawSkip)
										skipEmitted++
									}
								}
							}
						}
						if qVal, qEB, qOK := s.residualQuotient(cols, vals, lb, ub, rhs, k, true); qOK {
							newLB := qVal - qEB
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
							if isInt {
								newLB = math.Ceil(newLB - bTol)
							} else {
								newLB -= rowFeasTol / absA
							}
							if newLB > l+presolveIntTol {
								if gPay, errG := float64ToPayload(newLB); errG == nil {
									item.GoLB = &gPay
								}
							}
						}
					}
				}
				if hasLower && s.maxInfCount <= 1 {
					if a > 0 && (s.maxInfCount == 0 || math.IsInf(u, 1)) {
						if qVal, qEB, qOK := s.residualQuotient(cols, vals, lb, ub, rhs, k, false); qOK {
							newLB := qVal - qEB
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
							if isInt {
								newLB = math.Ceil(newLB - bTol)
							} else {
								newLB -= rowFeasTol / a
							}
							if newLB > l+presolveIntTol {
								if gPay, errG := float64ToPayload(newLB); errG == nil && item.GoLB == nil {
									item.GoLB = &gPay
								}
							}
						}
					} else if a < 0 && (s.maxInfCount == 0 || math.IsInf(l, -1)) {
						absA := -a
						if qVal, qEB, qOK := s.residualQuotient(cols, vals, lb, ub, rhs, k, false); qOK {
							newUB := qVal + qEB
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
							if isInt {
								newUB = math.Floor(newUB + bTol)
							} else {
								newUB += rowFeasTol / absA
							}
							if newUB < u-presolveIntTol {
								if gPay, errG := float64ToPayload(newUB); errG == nil && item.GoUB == nil {
									item.GoUB = &gPay
								}
							}
						}
					}
				}
			}
			terms = append(terms, item)
		}
		if allOK {
			req := exportActivitySummaryItem{
				Subcommand: "activity",
				Op:         "summary",
				RHS:        rhsPay,
				HasLo:      hasLower,
				HasHi:      hasUpper,
				Terms:      terms,
			}
			if raw, err := json.Marshal(req); err == nil {
				items = append(items, raw)
			}
		}
	}

	// 3. Savelsbergh binary coefficient strengthening matching presolve.go:295-362.
	if p.isMIP {
		binEmitted := 0
		for i := range m {
			if binEmitted >= exportMaxPresolveRows {
				break
			}
			if p.rowSense[i] != LessEqual {
				continue
			}
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			if st == en {
				continue
			}
			cols := p.colIdx[st:en]
			vals := p.rowVal[st:en]
			rhs := p.rowRHS[i]
			var s rowActivitySummary
			evalRowActivitySummaryInto(&s, cols, vals, lb, ub, rhs, false, false, true)
			if s.extreme || s.maxInfCount > 0 {
				continue
			}
			binTol := max(presolveFeasTol, presolveRelScaleTol*max(1.0, s.l1Max))
			surplusLo := -s.deficitHi
			surplusHi := -s.deficitLo
			if surplusLo <= binTol {
				continue
			}
			rhsPay, errRHS := float64ToPayload(rhs)
			surpPay, errSurp := float64ToPayload(surplusHi)
			if errRHS != nil || errSurp != nil {
				continue
			}
			for k, col := range cols {
				if p.varCat[col] != Binary || lb[col] != 0 || ub[col] != 1 {
					continue
				}
				a := vals[k]
				var newVal, newRHS float64
				triggered := false
				if a > binTol && surplusHi < a-binTol {
					candVal := surplusHi
					top, rem, ok := evalBinaryTightenRHSWide(rhs, a, candVal)
					if ok && !math.IsNaN(top) && !math.IsInf(top, 0) {
						if rem > 0 {
							top = nextUp(top)
						}
						if candVal > 0 && candVal < a-binTol && top < rhs-binTol && !isNonFinite(top) {
							newVal = candVal
							newRHS = top
							triggered = true
						}
					}
				} else if a < -binTol && surplusHi < -a-binTol {
					candVal := -surplusHi
					if candVal > a+binTol && candVal < 0 {
						newVal = candVal
						newRHS = rhs
						triggered = true
					}
				}
				if !triggered {
					continue
				}
				maxOther := new(big.Rat)
				term := new(big.Rat)
				bRat := new(big.Rat)
				otherOK := true
				for jIdx, c2 := range cols {
					if jIdx == k {
						continue
					}
					a2, okA2 := viprRatFromFloat(vals[jIdx])
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
					bRat.Set(br)
					term.Mul(a2, bRat)
					maxOther.Add(maxOther, term)
				}
				if !otherOK {
					continue
				}
				aPay, errA := float64ToPayload(a)
				naPay, errNA := float64ToPayload(newVal)
				nrPay, errNR := float64ToPayload(newRHS)
				if errA != nil || errNA != nil || errNR != nil {
					continue
				}
				req := exportBinStrengthenItem{
					Subcommand:   "bin-strengthen",
					Op:           "bin-strengthen",
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
					binEmitted++
				}
				break
			}
		}
	}

	// 4. Orbit symmetry verification matching proposeAndVerifyOrbits / verifyOrbitSymmetry.
	if p.isMIP && n <= exportMaxOrbitVars && m >= 2 && m <= exportMaxOrbitRows {
		rows := make([]presolveWorkRow, m)
		var seedRows []int
		for i := range m {
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			cols := slices.Clone(p.colIdx[st:en])
			vals := slices.Clone(p.rowVal[st:en])
			rows[i] = presolveWorkRow{
				name:   p.rowNames[i],
				cols:   cols,
				vals:   vals,
				rhs:    p.rowRHS[i],
				sense:  p.rowSense[i],
				active: true,
			}
			if len(cols) >= 2 && len(cols) <= maxOrbitGroupSize && (p.rowSense[i] == Equal || p.rowSense[i] == LessEqual) && p.rowRHS[i] == 1.0 {
				unitBin := true
				for k, c := range cols {
					if vals[k] != 1.0 || p.varCat[c] != Binary || lb[c] != 0 || ub[c] != 1 {
						unitBin = false
						break
					}
				}
				if unitBin {
					seedRows = append(seedRows, i)
				}
			}
		}
		if len(seedRows) > 0 {
			colPtr, colRow, colVal := buildPresolveCSC(n, rows)
			seedRows = filterAndSortOrbitSeeds(rows, colPtr, seedRows)
			if len(seedRows) > 0 {
				rowHash := computeOrbitRowHashes(p, rows, lb, ub, colPtr)
				sc := newOrbitTrialScratch(n, m)
				maxTrials := min(len(seedRows), exportMaxOrbitTrials)
				for sIdx := range maxTrials {
					if sIdx > 0 {
						sc.reset()
					}
					gOut, okOrb, to := proposeAndVerifyOrbits(p, rows, lb, ub, colPtr, colRow, colVal, rowHash, seedRows[sIdx], &sc, time.Time{}, nil)
					if to || !okOrb || gOut < 2 {
						continue
					}
					if !verifyOrbitSymmetry(p, rows, lb, ub, gOut, &sc) {
						continue
					}
					if orbItem, okBuild := buildExportOrbitCheckItem(p, rows, lb, ub, gOut, &sc); okBuild {
						if raw, err := json.Marshal(orbItem); err == nil {
							items = append(items, raw)
						}
					}
					break
				}
			}
		}
	}

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
		RowState:     slices.Clone(sc.rowState),
		RequireValid: true,
	}, true
}

type exportCutCheckItem struct {
	Subcommand string              `json:"subcommand"`
	Op         string              `json:"op"`
	MIP        track2BMIPPayload   `json:"mip"`
	Cuts       []track2BCutPayload `json:"cuts"`
	Solutions  [][]ratPayload      `json:"solutions"`
}

type exportCoverCertEntry struct {
	Col     int        `json:"col"`
	Weight  ratPayload `json:"weight"`
	Comp    bool       `json:"comp"`
	InCover bool       `json:"inCover"`
	Alpha   int        `json:"alpha"`
}

type exportCoverCheckItem struct {
	Subcommand string                 `json:"subcommand"`
	Op         string                 `json:"op"`
	NumVars    int                    `json:"numVars"`
	LB         []*ratPayload          `json:"lb"`
	UB         []*ratPayload          `json:"ub"`
	Row        track2BRowPayload      `json:"row"`
	Sign       ratPayload             `json:"sign"`
	Items      []exportCoverCertEntry `json:"items"`
	KnapRHS    ratPayload             `json:"knapRHS"`
	Scale      ratPayload             `json:"scale"`
	Cut        track2BCutPayload      `json:"cut"`
	Solutions  [][]ratPayload         `json:"solutions,omitzero"`
}

type exportLotSizingNode struct {
	ProdCol   int        `json:"prodCol"`
	InvOutCol int        `json:"invOutCol"`
	BinCol    int        `json:"binCol"`
	Demand    ratPayload `json:"demand"`
	VUBCap    ratPayload `json:"vubCap"`
	VUBRHS    ratPayload `json:"vubRHS"`
	InS       bool       `json:"inS"`
	CoeffY    ratPayload `json:"coeffY"`
}

type exportLotSizingCheckItem struct {
	Subcommand string                `json:"subcommand"`
	Op         string                `json:"op"`
	NumVars    int                   `json:"numVars"`
	InvInCol   *int                  `json:"invInCol"`
	Nodes      []exportLotSizingNode `json:"nodes"`
	CutDem     ratPayload            `json:"cutDem"`
	Scale      ratPayload            `json:"scale"`
	Cut        track2BCutPayload     `json:"cut"`
	Solutions  [][]ratPayload        `json:"solutions,omitzero"`
}

type exportAggCMIRCheckItem struct {
	Subcommand string                 `json:"subcommand"`
	Op         string                 `json:"op"`
	NumVars    int                    `json:"numVars"`
	IsInt      []bool                 `json:"isInt"`
	LB         []*ratPayload          `json:"lb"`
	UB         []*ratPayload          `json:"ub"`
	Rows       []track2BAggRowPayload `json:"rows"`
	AggTerms   []track2BTermPayload   `json:"aggTerms"`
	AggRHS     ratPayload             `json:"aggRHS"`
	Delta      ratPayload             `json:"delta"`
	UseLower   []bool                 `json:"useLower"`
	BetaHat    *ratPayload            `json:"betaHat,omitzero"`
	AlphaHat   []ratPayload           `json:"alphaHat,omitzero"`
	CInt       []ratPayload           `json:"cInt,omitzero"`
	CCont      []ratPayload           `json:"cCont,omitzero"`
	Scale      ratPayload             `json:"scale"`
	Cut        track2BCutPayload      `json:"cut"`
	Solutions  [][]ratPayload         `json:"solutions,omitzero"`
}

type exportDropCheckItem struct {
	Subcommand string            `json:"subcommand"`
	Op         string            `json:"op"`
	NumVars    int               `json:"numVars"`
	LB         []*ratPayload     `json:"lb"`
	UB         []*ratPayload     `json:"ub"`
	RawCut     track2BCutPayload `json:"rawCut"`
	Scale      ratPayload        `json:"scale"`
	FinalCut   track2BCutPayload `json:"finalCut"`
	Solutions  [][]ratPayload    `json:"solutions,omitzero"`
}

func exportCompiledToMIPPayload(p *CompiledProblem) (track2BMIPPayload, error) {
	mip, domainFeas, err := exportCompiledToMIPPayloadChecked(p)
	if err != nil {
		return track2BMIPPayload{}, err
	}
	if !domainFeas {
		return track2BMIPPayload{}, errors.New("infeasible or inverted variable bounds in CompiledProblem")
	}
	return mip, nil
}

func exportCompiledToMIPPayloadChecked(p *CompiledProblem) (track2BMIPPayload, bool, error) {
	n := p.numVars
	isInt := make([]bool, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		if math.IsNaN(p.varLB[j]) || math.IsNaN(p.varUB[j]) {
			return track2BMIPPayload{}, false, fmt.Errorf("NaN variable bound at col %d", j)
		}
		if p.varLB[j] > p.varUB[j] || math.IsInf(p.varLB[j], 1) || math.IsInf(p.varUB[j], -1) {
			return track2BMIPPayload{}, false, nil
		}
		isInt[j] = p.varCat[j] != Continuous
		lPtr, okL := exportOptRatPayload(p.varLB[j], -1)
		uPtr, okU := exportOptRatPayload(p.varUB[j], 1)
		if !okL || !okU {
			return track2BMIPPayload{}, false, fmt.Errorf("invalid variable bound at col %d", j)
		}
		lb[j] = lPtr
		ub[j] = uPtr
	}
	rows := make([]track2BRowPayload, p.numConstraints)
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		terms := make([]track2BTermPayload, 0, en-st)
		for k := st; k < en; k++ {
			vp, err := float64ToPayload(p.rowVal[k])
			if err != nil {
				return track2BMIPPayload{}, false, fmt.Errorf("row %d col %d: %w", i, p.colIdx[k], err)
			}
			terms = append(terms, track2BTermPayload{Col: p.colIdx[k], Val: vp})
		}
		rhsPay, err := float64ToPayload(p.rowRHS[i])
		if err != nil {
			return track2BMIPPayload{}, false, fmt.Errorf("row %d rhs: %w", i, err)
		}
		rows[i] = track2BRowPayload{
			Terms: terms,
			Sense: senseToStr(p.rowSense[i]),
			RHS:   rhsPay,
		}
	}
	return track2BMIPPayload{
		NumVars: n,
		IsInt:   isInt,
		LB:      lb,
		UB:      ub,
		Rows:    rows,
	}, true, nil
}

func exportCutsToPayloads(cuts []cutPlane) ([]track2BCutPayload, error) {
	out := make([]track2BCutPayload, len(cuts))
	for i, c := range cuts {
		terms := make([]track2BTermPayload, len(c.ColIdx))
		for k, col := range c.ColIdx {
			vp, err := float64ToPayload(c.Val[k])
			if err != nil {
				return nil, fmt.Errorf("cut %d term %d: %w", i, k, err)
			}
			terms[k] = track2BTermPayload{Col: col, Val: vp}
		}
		rhsPay, err := float64ToPayload(c.RHS)
		if err != nil {
			return nil, fmt.Errorf("cut %d rhs: %w", i, err)
		}
		out[i] = track2BCutPayload{Terms: terms, RHS: rhsPay}
	}
	return out, nil
}

func exportSparseCutPayload(cols []int, vals []float64, rhs float64) (track2BCutPayload, bool) {
	if len(cols) != len(vals) {
		return track2BCutPayload{}, false
	}
	rhsPay, err := float64ToPayload(rhs)
	if err != nil {
		return track2BCutPayload{}, false
	}
	terms := make([]track2BTermPayload, 0, len(cols))
	for k, col := range cols {
		if vals[k] == 0 {
			continue
		}
		vp, tErr := float64ToPayload(vals[k])
		if tErr != nil {
			return track2BCutPayload{}, false
		}
		terms = append(terms, track2BTermPayload{Col: col, Val: vp})
	}
	return track2BCutPayload{Terms: terms, RHS: rhsPay}, true
}

func computeFinalizeScaleAndExactMatch(n int, rawIdx []int, rawVal []float64, finalCut cutPlane) (scaleRat *big.Rat, exactScaled bool, ok bool) {
	if len(rawIdx) == 0 || len(rawIdx) != len(rawVal) || len(finalCut.ColIdx) == 0 {
		return nil, false, false
	}
	accMap := make(map[int]float64, len(rawIdx))
	hadMergeRem := false
	for k, col := range rawIdx {
		v := rawVal[k]
		if col < 0 || col >= n || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false, false
		}
		if cur, exists := accMap[col]; exists {
			sum, rem := twoSum(cur, v)
			if math.IsNaN(sum) || math.IsInf(sum, 0) {
				return nil, false, false
			}
			accMap[col] = sum
			if rem != 0 {
				hadMergeRem = true
			}
		} else {
			accMap[col] = v
		}
	}
	uniq := make([]int, 0, len(accMap))
	for col := range accMap {
		uniq = append(uniq, col)
	}
	slices.Sort(uniq)

	maxAbs := 0.0
	kept := make([]int, 0, len(uniq))
	droppedAny := false
	for _, j := range uniq {
		av := math.Abs(accMap[j])
		if av > cutDropTol {
			maxAbs = max(maxAbs, av)
			kept = append(kept, j)
		} else if accMap[j] != 0 {
			droppedAny = true
		}
	}
	if len(kept) == 0 || maxAbs == 0 {
		return nil, false, false
	}
	scaledNorm2 := 0.0
	for _, j := range kept {
		r := accMap[j] / maxAbs
		scaledNorm2 += r * r
	}
	norm := maxAbs * math.Sqrt(scaledNorm2)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, false, false
	}
	_, rawExp := math.Frexp(norm)
	normExp := rawExp - 1
	scaleRat = viprPow2Rat(-normExp)

	exactScaled = !hadMergeRem && !droppedAny && len(kept) == len(finalCut.ColIdx)
	if exactScaled {
		for k, j := range kept {
			if finalCut.ColIdx[k] != j {
				exactScaled = false
				break
			}
			v := accMap[j]
			sv := finalCut.Val[k]
			if math.Ldexp(sv, normExp) != v {
				exactScaled = false
				break
			}
		}
	}
	return scaleRat, exactScaled, true
}

func appendDropCheckCert(
	items *[][]byte,
	mip track2BMIPPayload,
	sols [][]ratPayload,
	rawIdx []int,
	rawVal []float64,
	rawRHS float64,
	finalCut cutPlane,
) (scaleRat *big.Rat, rawPay, finalPay track2BCutPayload, exactScaled, ok bool) {
	scaleRat, exactScaled, ok = computeFinalizeScaleAndExactMatch(mip.NumVars, rawIdx, rawVal, finalCut)
	if !ok {
		return nil, track2BCutPayload{}, track2BCutPayload{}, false, false
	}
	rawPay, ok = exportSparseCutPayload(rawIdx, rawVal, rawRHS)
	if !ok {
		return nil, track2BCutPayload{}, track2BCutPayload{}, false, false
	}
	finalPays, err := exportCutsToPayloads([]cutPlane{finalCut})
	if err != nil || len(finalPays) != 1 {
		return nil, track2BCutPayload{}, track2BCutPayload{}, false, false
	}
	finalPay = finalPays[0]
	dropItem := exportDropCheckItem{
		Subcommand: "drop-check",
		Op:         "drop-check",
		NumVars:    mip.NumVars,
		LB:         mip.LB,
		UB:         mip.UB,
		RawCut:     rawPay,
		Scale:      bigRatToPayload(scaleRat),
		FinalCut:   finalPay,
		Solutions:  sols,
	}
	if rawJSON, mErr := json.Marshal(dropItem); mErr == nil {
		*items = append(*items, rawJSON)
	}
	return scaleRat, rawPay, finalPay, exactScaled, true
}

func exportCoverCertFromSparseRow(
	items *[][]byte,
	p *CompiledProblem,
	mip track2BMIPPayload,
	sols [][]ratPayload,
	rowPay track2BRowPayload,
	cols []int,
	vals []float64,
	rawRHS, sign float64,
	lpPrimal []float64,
	maxNNZ int,
	ws *cutGenWorkspace,
) {
	if len(cols) < 3 || math.IsNaN(rawRHS) || math.IsInf(rawRHS, 0) {
		return
	}
	var rhsAcc dot2Acc
	rhsAcc.addVal(sign * rawRHS)

	validRelax := true
	covItems := make([]coverItem, 0, len(cols))
	minItemW := math.Inf(1)
	for k, col := range cols {
		a := sign * vals[k]
		if math.IsNaN(a) || math.IsInf(a, 0) {
			return
		}
		if a == 0 {
			continue
		}
		if p.varCat[col] != Binary || p.varLB[col] != 0 || p.varUB[col] != 1 || math.Abs(a) <= cutCoeffZeroTol {
			if a > 0 {
				if math.IsInf(p.varLB[col], -1) || math.IsNaN(p.varLB[col]) {
					validRelax = false
					break
				}
				rhsAcc.addProd(-a, p.varLB[col])
			} else {
				if math.IsInf(p.varUB[col], 1) || math.IsNaN(p.varUB[col]) {
					validRelax = false
					break
				}
				rhsAcc.addProd(-a, p.varUB[col])
			}
			continue
		}
		if a > 0 {
			minItemW = min(minItemW, a)
			covItems = append(covItems, coverItem{
				col:    col,
				weight: a,
				xVal:   lpPrimal[col],
				comp:   false,
			})
		} else {
			minItemW = min(minItemW, -a)
			rhsAcc.addVal(-a)
			covItems = append(covItems, coverItem{
				col:    col,
				weight: -a,
				xVal:   1.0 - lpPrimal[col],
				comp:   true,
			})
		}
	}

	rhsVal, rhsErr, okRHS := rhsAcc.result()
	rhs := rhsVal + rhsErr
	if rhsErr > 0 && !math.IsInf(rhs, 0) && !math.IsNaN(rhs) {
		rhs = nextUp(rhs)
	}
	if !validRelax || !okRHS || len(covItems) < 3 || !(rhs > 0) || math.IsInf(rhs, 0) || math.IsNaN(rhs) || rhsErr > 1e-4*minItemW {
		return
	}

	slices.SortFunc(covItems, func(a, b coverItem) int {
		if math.Abs(a.xVal-b.xVal) > cutCompareTol {
			return cmp.Compare(b.xVal, a.xVal)
		}
		return cmp.Compare(b.weight, a.weight)
	})

	inCover := make([]bool, len(covItems))
	var coverAcc dot2Acc
	coverWeight := 0.0
	coverSize := 0
	coverMargin := func(cw float64) float64 {
		return max(cutCompareTol, 16.0*unitRoundoff64*(cw+math.Abs(rhs)))
	}
	for idx, it := range covItems {
		if it.xVal <= minCoverItemVal {
			break
		}
		inCover[idx] = true
		coverAcc.addVal(it.weight)
		coverSize++
		cwVal, cwErr, okCW := coverAcc.result()
		if !okCW {
			return
		}
		cwLo := cwVal - cwErr
		if cwErr > 0 && !math.IsInf(cwLo, 0) && !math.IsNaN(cwLo) {
			cwLo = nextDown(cwLo)
		}
		coverWeight = cwLo
		if cwLo > rhs+coverMargin(cwVal) {
			break
		}
	}
	if !(coverWeight > rhs+coverMargin(coverWeight)) || coverSize < minPairNNZ {
		return
	}
	for idx := len(covItems) - 1; idx >= 0; idx-- {
		if !inCover[idx] {
			continue
		}
		candW := nextDown(coverWeight - covItems[idx].weight)
		if candW > rhs+coverMargin(coverWeight) {
			inCover[idx] = false
			coverWeight = candW
			coverSize--
		}
	}
	if coverSize < minPairNNZ {
		return
	}
	var verifyAcc dot2Acc
	for idx, it := range covItems {
		if inCover[idx] {
			verifyAcc.addVal(it.weight)
		}
	}
	cwVal, cwErr, okCW := verifyAcc.result()
	if !okCW {
		return
	}
	cwLo := cwVal - cwErr
	if cwErr > 0 && !math.IsInf(cwLo, 0) && !math.IsNaN(cwLo) {
		cwLo = nextDown(cwLo)
	}
	if !(cwLo > rhs) {
		return
	}

	coverWeights := make([]float64, 0, coverSize)
	for idx, it := range covItems {
		if inCover[idx] {
			coverWeights = append(coverWeights, it.weight)
		}
	}
	slices.Sort(coverWeights)
	minWeight := make([]float64, coverSize)
	for v := 1; v < coverSize; v++ {
		s := minWeight[v-1] + coverWeights[v-1]
		if math.IsInf(s, 1) {
			minWeight[v] = math.MaxFloat64
		} else {
			minWeight[v] = max(0.0, s-8.0*unitRoundoff64*s)
		}
	}

	cutCap := float64(coverSize - 1)
	finalRHSInt := int64(coverSize - 1)
	lhsStar := 0.0
	rawIdx := make([]int, 0, len(covItems))
	rawVal := make([]float64, 0, len(covItems))
	certEntries := make([]exportCoverCertEntry, len(covItems))

	for idx, it := range covItems {
		alpha := liftCoverCertificateItem(it.weight, rhs, coverSize, inCover[idx], minWeight)
		wPay, wErr := float64ToPayload(it.weight)
		if wErr != nil {
			return
		}
		certEntries[idx] = exportCoverCertEntry{
			Col:     it.col,
			Weight:  wPay,
			Comp:    it.comp,
			InCover: inCover[idx],
			Alpha:   alpha,
		}
		if alpha <= 0 {
			continue
		}
		fAlpha := float64(alpha)
		lhsStar += fAlpha * it.xVal
		rawIdx = append(rawIdx, it.col)
		if !it.comp {
			rawVal = append(rawVal, fAlpha)
		} else {
			rawVal = append(rawVal, -fAlpha)
			finalRHSInt -= int64(alpha)
		}
	}
	if lhsStar <= cutCap+minCutEfficacy {
		return
	}
	finalRHS := float64(finalRHSInt)
	if int64(finalRHS) < finalRHSInt {
		finalRHS = nextUp(finalRHS)
	}
	cut, ok := finalizeSparseCut(p, rawIdx, rawVal, finalRHS, lpPrimal, maxNNZ, ws)
	if !ok {
		return
	}
	scaleRat, rawPay, finalPay, exactScaled, okDrop := appendDropCheckCert(items, mip, sols, rawIdx, rawVal, finalRHS, cut)
	if !okDrop {
		return
	}
	signPay, errS := float64ToPayload(sign)
	knapPay, errK := float64ToPayload(rhs)
	if errS != nil || errK != nil {
		return
	}
	targetCut := rawPay
	targetScale := int64ToPayload(1)
	if exactScaled {
		targetCut = finalPay
		targetScale = bigRatToPayload(scaleRat)
	}
	covReq := exportCoverCheckItem{
		Subcommand: "cover-check",
		Op:         "cover-check",
		NumVars:    mip.NumVars,
		LB:         mip.LB,
		UB:         mip.UB,
		Row:        rowPay,
		Sign:       signPay,
		Items:      certEntries,
		KnapRHS:    knapPay,
		Scale:      targetScale,
		Cut:        targetCut,
		Solutions:  sols,
	}
	if rawJSON, mErr := json.Marshal(covReq); mErr == nil {
		*items = append(*items, rawJSON)
	}
}

func exportVUBAndLotSizingCerts(
	items *[][]byte,
	p *CompiledProblem,
	mip track2BMIPPayload,
	sols [][]ratPayload,
	lpPrimal []float64,
	ws *cutGenWorkspace,
) {
	cache := ws.ensureStaticCache(p)
	if len(cache.disaggVUBs) == 0 && len(cache.flowChains) == 0 {
		return
	}

	var vubIdx [2]int
	var vubVal [2]float64
	for _, term := range cache.disaggVUBs {
		if lpPrimal[term.col]-term.effUB*lpPrimal[term.binCol] > minCutEfficacy {
			vubIdx[0], vubIdx[1] = term.col, term.binCol
			vubVal[0], vubVal[1] = 1.0, -term.effUB
			if cut, ok := finalizeSparseCut(p, vubIdx[:], vubVal[:], 0, lpPrimal, 0, ws); ok {
				_, _, _, _, _ = appendDropCheckCert(items, mip, sols, vubIdx[:], vubVal[:], 0, cut)
			}
		}
	}

	if len(cache.flowChains) == 0 {
		return
	}

	vubCapByProd := make([]float64, p.numVars)
	vubRHSByProd := make([]float64, p.numVars)
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if p.rowSense[i] != LessEqual || !(p.rowRHS[i] <= 0) || math.IsInf(p.rowRHS[i], 0) || e-s != minPairNNZ {
			continue
		}
		binCol := -1
		capVal := 0.0
		negCount := 0
		allPosNonNeg := true
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if math.IsNaN(val) || math.IsInf(val, 0) {
				allPosNonNeg = false
				break
			}
			if val < 0 {
				negCount++
				if val < -cutCoeffZeroTol && p.varCat[col] == Binary && p.varLB[col] == 0 && p.varUB[col] == 1 {
					binCol = col
					capVal = -val
				}
			} else if val > 0 && !(p.varLB[col] >= 0) {
				allPosNonNeg = false
			}
		}
		if negCount != 1 || binCol == -1 || !allPosNonNeg || math.IsInf(capVal, 0) || math.IsNaN(capVal) {
			continue
		}
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if col != binCol && p.rowVal[k] == 1.0 {
				vubCapByProd[col] = capVal
				vubRHSByProd[col] = p.rowRHS[i]
			}
		}
	}

	var seenBins [maxFlowLookahead + 1]int
	var remDemHi [maxFlowLookahead + 1]float64
	var flowIdx [maxFlowLookahead + 2]int
	var flowVal [maxFlowLookahead + 2]float64
	for _, chain := range cache.flowChains {
		T := len(chain)
		for t := range T {
			maxQ := min(T-1, t+maxFlowLookahead)
			for q := t; q <= maxQ; q++ {
				var remAcc dot2Acc
				remOK := true
				for s := q; s >= t; s-- {
					remAcc.addVal(chain[s].demand)
					v, e, ok := remAcc.result()
					if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
						remOK = false
						break
					}
					hi := v + e
					if e > 0 && !math.IsInf(hi, 0) && !math.IsNaN(hi) {
						hi = nextUp(hi)
					}
					if !(hi > 0) || math.IsInf(hi, 0) || math.IsNaN(hi) {
						remOK = false
						break
					}
					remDemHi[s-t] = hi
				}
				if !remOK {
					continue
				}
				cumVal, cumErr, okCum := remAcc.result()
				if !okCum {
					continue
				}
				cumDemLo := cumVal - cumErr
				if cumErr > 0 && !math.IsInf(cumDemLo, 0) && !math.IsNaN(cumDemLo) {
					cumDemLo = nextDown(cumDemLo)
				}
				if !(cumDemLo > 0) || math.IsInf(cumDemLo, 0) || math.IsNaN(cumDemLo) {
					continue
				}

				lhsStar := 0.0
				if chain[t].invIn != -1 {
					lhsStar += lpPrimal[chain[t].invIn]
				}
				anyInS := false
				numSeen := 0
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					alreadyInS := false
					for k := range numSeen {
						if seenBins[k] == nd.binCol {
							alreadyInS = true
							break
						}
					}
					if alreadyInS {
						continue
					}
					yTerm := remDemHi[tau-t] * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-cutCompareTol {
						seenBins[numSeen] = nd.binCol
						numSeen++
						lhsStar += yTerm
						anyInS = true
					} else {
						lhsStar += pTerm
					}
				}
				if !anyInS || lhsStar >= cumDemLo-minCutEfficacy {
					continue
				}

				nf := 0
				if chain[t].invIn != -1 {
					flowIdx[nf] = chain[t].invIn
					flowVal[nf] = -1.0
					nf++
				}
				nodesCert := make([]exportLotSizingNode, 0, q-t+1)
				nodesOK := true
				numSeen = 0
				for tau := t; tau <= q; tau++ {
					nd := chain[tau]
					demPay, errD := float64ToPayload(nd.demand)
					capPay, errC := float64ToPayload(vubCapByProd[nd.prodCol])
					rhsPay, errR := float64ToPayload(vubRHSByProd[nd.prodCol])
					if errD != nil || errC != nil || errR != nil {
						nodesOK = false
						break
					}
					alreadyInS := false
					for k := range numSeen {
						if seenBins[k] == nd.binCol {
							alreadyInS = true
							break
						}
					}
					if alreadyInS {
						nodesCert = append(nodesCert, exportLotSizingNode{
							ProdCol:   nd.prodCol,
							InvOutCol: nd.invOut,
							BinCol:    nd.binCol,
							Demand:    demPay,
							VUBCap:    capPay,
							VUBRHS:    rhsPay,
							InS:       true,
							CoeffY:    int64ToPayload(0),
						})
						continue
					}
					rem := remDemHi[tau-t]
					yTerm := rem * lpPrimal[nd.binCol]
					pTerm := lpPrimal[nd.prodCol]
					if yTerm < pTerm-cutCompareTol {
						remPay, errRem := float64ToPayload(rem)
						if errRem != nil {
							nodesOK = false
							break
						}
						seenBins[numSeen] = nd.binCol
						numSeen++
						flowIdx[nf] = nd.binCol
						flowVal[nf] = -rem
						nf++
						nodesCert = append(nodesCert, exportLotSizingNode{
							ProdCol:   nd.prodCol,
							InvOutCol: nd.invOut,
							BinCol:    nd.binCol,
							Demand:    demPay,
							VUBCap:    capPay,
							VUBRHS:    rhsPay,
							InS:       true,
							CoeffY:    remPay,
						})
					} else {
						flowIdx[nf] = nd.prodCol
						flowVal[nf] = -1.0
						nf++
						nodesCert = append(nodesCert, exportLotSizingNode{
							ProdCol:   nd.prodCol,
							InvOutCol: nd.invOut,
							BinCol:    nd.binCol,
							Demand:    demPay,
							VUBCap:    capPay,
							VUBRHS:    rhsPay,
							InS:       false,
							CoeffY:    int64ToPayload(0),
						})
					}
				}
				if !nodesOK {
					continue
				}
				cut, ok := finalizeSparseCut(p, flowIdx[:nf], flowVal[:nf], -cumDemLo, lpPrimal, 0, ws)
				if !ok {
					continue
				}
				scaleRat, rawPay, finalPay, exactScaled, okDrop := appendDropCheckCert(items, mip, sols, flowIdx[:nf], flowVal[:nf], -cumDemLo, cut)
				if !okDrop {
					continue
				}
				cutDemPay, errCD := float64ToPayload(cumDemLo)
				if errCD != nil {
					continue
				}
				var invInPtr *int
				if chain[t].invIn != -1 {
					v := chain[t].invIn
					invInPtr = &v
				}
				targetCut := rawPay
				targetScale := int64ToPayload(1)
				if exactScaled {
					targetCut = finalPay
					targetScale = bigRatToPayload(scaleRat)
				}
				lotReq := exportLotSizingCheckItem{
					Subcommand: "lotsizing-check",
					Op:         "lotsizing-check",
					NumVars:    mip.NumVars,
					InvInCol:   invInPtr,
					Nodes:      nodesCert,
					CutDem:     cutDemPay,
					Scale:      targetScale,
					Cut:        targetCut,
					Solutions:  sols,
				}
				if rawJSON, mErr := json.Marshal(lotReq); mErr == nil {
					*items = append(*items, rawJSON)
				}
			}
		}
	}
}

func exportCMIRCertsFromSparseRow(
	items *[][]byte,
	p *CompiledProblem,
	mip track2BMIPPayload,
	sols [][]ratPayload,
	aggRows []track2BAggRowPayload,
	cols []int,
	vals []float64,
	rawRHS, sign float64,
	lpPrimal []float64,
	maxNNZ, maxDivs int,
	ws *cutGenWorkspace,
) {
	if len(cols) < minPairNNZ || len(aggRows) == 0 {
		return
	}
	n := p.numVars
	rhs := sign * rawRHS
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return
	}
	aggRHS, errAggRHS := float64ToPayload(rhs)
	if errAggRHS != nil {
		return
	}

	aggTerms := make([]track2BTermPayload, 0, len(cols))
	act := 0.0
	for k, col := range cols {
		if uint(col) >= uint(n) {
			return
		}
		v := vals[k]
		sv := sign * v
		if math.IsNaN(sv) || math.IsInf(sv, 0) {
			return
		}
		l, u := p.varLB[col], p.varUB[col]
		if math.IsNaN(l) || math.IsNaN(u) || (math.IsInf(l, -1) && math.IsInf(u, 1)) {
			return
		}
		if p.varCat[col] != Continuous {
			l = math.Ceil(l)
			u = math.Floor(u)
			if l > u {
				return
			}
		}
		if sv != 0 {
			tp, tErr := float64ToPayload(sv)
			if tErr != nil {
				return
			}
			aggTerms = append(aggTerms, track2BTermPayload{Col: col, Val: tp})
		}
		act += sv * lpPrimal[col]
	}
	if !(rhs-act <= 1.0+mirMaxRelSlack*math.Abs(rhs)) {
		return
	}

	var triedDeltas [8]float64
	triedDivs := 0
	betaComputed := false
	var betaVal, betaErr float64
	var okBeta bool

	for kDiv, divCol := range cols {
		if triedDivs >= maxDivs || triedDivs >= len(triedDeltas) {
			break
		}
		if uint(divCol) >= uint(n) || p.varCat[divCol] == Continuous {
			continue
		}
		xv := lpPrimal[divCol]
		fx := xv - math.Floor(xv)
		if fx < minIntInfeas || fx > 1.0-minIntInfeas {
			continue
		}
		delta := math.Abs(vals[kDiv])
		if delta <= cutCompareTol {
			continue
		}
		dupDelta := false
		for idx := range triedDivs {
			if triedDeltas[idx] == delta {
				dupDelta = true
				break
			}
		}
		if dupDelta {
			continue
		}
		triedDeltas[triedDivs] = delta
		triedDivs++

		if !betaComputed {
			var betaAcc dot2Acc
			betaAcc.addVal(rhs)
			for k, col := range cols {
				a := sign * vals[k]
				l, u := p.varLB[col], p.varUB[col]
				if p.varCat[col] != Continuous {
					l = math.Ceil(l)
					u = math.Floor(u)
				}
				if mirUseLower(lpPrimal[col], l, u) {
					betaAcc.addProd(-a, l)
				} else {
					betaAcc.addProd(-a, u)
				}
			}
			betaVal, betaErr, okBeta = betaAcc.result()
			betaComputed = true
		}
		if !okBeta {
			break
		}
		if betaErr > 1e-4*delta {
			continue
		}

		betaNum := betaVal + betaErr
		if betaErr > 0 && !math.IsInf(betaNum, 0) && !math.IsNaN(betaNum) {
			betaNum = nextUp(betaNum)
		}
		betaHat := betaNum / delta
		if math.IsNaN(betaHat) || math.IsInf(betaHat, 0) {
			continue
		}
		betaHat = nextUp(betaHat)
		if math.IsNaN(betaHat) || math.IsInf(betaHat, 0) {
			continue
		}
		b0 := math.Floor(betaHat)
		f0 := betaHat - b0
		if b0+f0 < betaHat {
			f0 = nextUp(f0)
		}
		if !(f0 >= minMIRFrac && f0 <= 1.0-minMIRFrac) {
			continue
		}

		var cutRHSAcc dot2Acc
		cutRHSAcc.addVal(b0)
		contDenLo := nextDown(delta * nextDown(1.0-f0))
		if !(contDenLo > 0) || math.IsInf(contDenLo, 0) || math.IsNaN(contDenLo) {
			continue
		}
		intDenHi := nextUp(1.0 - f0)

		useLower := make([]bool, n)
		alphaHat := make([]ratPayload, n)
		cInt := make([]ratPayload, n)
		cCont := make([]ratPayload, n)
		zeroPay := int64ToPayload(0)
		for j := range n {
			useLower[j] = true
			alphaHat[j] = zeroPay
			cInt[j] = zeroPay
			cCont[j] = zeroPay
		}

		rawIdx := make([]int, 0, len(cols))
		rawVal := make([]float64, 0, len(cols))
		validCut := true

		for k, col := range cols {
			a := sign * vals[k]
			if a == 0 {
				continue
			}
			l, u := p.varLB[col], p.varUB[col]
			if p.varCat[col] != Continuous {
				l = math.Ceil(l)
				u = math.Floor(u)
			}
			if l == u {
				if p.varCat[col] == Continuous {
					useLower[col] = a >= 0
				} else {
					atLow := mirUseLower(lpPrimal[col], l, u)
					useLower[col] = atLow
					num := a
					if !atLow {
						num = -a
					}
					alpha := num / delta
					if math.IsInf(alpha, 0) || math.IsNaN(alpha) {
						validCut = false
						break
					}
					if alpha != 0 || num < 0 {
						alpha = nextDown(alpha)
						if math.IsInf(alpha, 0) || math.IsNaN(alpha) {
							validCut = false
							break
						}
					}
					ap, errAP := float64ToPayload(alpha)
					if errAP != nil {
						validCut = false
						break
					}
					alphaHat[col] = ap
				}
				continue
			}

			atLower := mirUseLower(lpPrimal[col], l, u)
			useLower[col] = atLower

			if p.varCat[col] == Continuous {
				if a > 0 {
					if atLower {
						continue
					}
					g := nextUp(a / contDenLo)
					if math.IsInf(g, 0) || math.IsNaN(g) {
						validCut = false
						break
					}
					gp, errGP := float64ToPayload(g)
					if errGP != nil {
						validCut = false
						break
					}
					cCont[col] = gp
					if g != 0 {
						rawIdx = append(rawIdx, col)
						rawVal = append(rawVal, g)
					}
					cutRHSAcc.addProd(g, u)
				} else if a < 0 {
					if !atLower {
						continue
					}
					g := nextUp((-a) / contDenLo)
					if math.IsInf(g, 0) || math.IsNaN(g) {
						validCut = false
						break
					}
					gp, errGP := float64ToPayload(g)
					if errGP != nil {
						validCut = false
						break
					}
					cCont[col] = gp
					if g != 0 {
						rawIdx = append(rawIdx, col)
						rawVal = append(rawVal, -g)
					}
					cutRHSAcc.addProd(-g, l)
				}
			} else {
				comp := !atLower
				num := a
				if comp {
					num = -a
				}
				alpha := num / delta
				if math.IsInf(alpha, 0) || math.IsNaN(alpha) {
					validCut = false
					break
				}
				if alpha != 0 || num < 0 {
					alpha = nextDown(alpha)
					if math.IsInf(alpha, 0) || math.IsNaN(alpha) {
						validCut = false
						break
					}
				}
				flA := math.Floor(alpha)
				fj := alpha - flA
				if alpha < 0 && fj > 0 {
					fj = nextDown(fj)
				}
				coeff := flA
				if fj > f0 {
					numLo := nextDown(fj - f0)
					if numLo > 0 {
						fracLo := nextDown(numLo / intDenHi)
						if fracLo > 0 {
							coeff = nextDown(flA + fracLo)
						}
					}
				}
				ap, errAP := float64ToPayload(alpha)
				cp, errCP := float64ToPayload(coeff)
				if errAP != nil || errCP != nil {
					validCut = false
					break
				}
				alphaHat[col] = ap
				cInt[col] = cp
				if !comp {
					if coeff != 0 {
						rawIdx = append(rawIdx, col)
						rawVal = append(rawVal, coeff)
					}
					cutRHSAcc.addProd(coeff, l)
				} else {
					if coeff != 0 {
						rawIdx = append(rawIdx, col)
						rawVal = append(rawVal, -coeff)
					}
					cutRHSAcc.addProd(-coeff, u)
				}
			}
		}
		if !validCut || len(rawIdx) == 0 {
			continue
		}
		cutRHSVal, cutRHSErr, okCutRHS := cutRHSAcc.result()
		if !okCutRHS {
			continue
		}
		cutRHS := cutRHSVal + cutRHSErr
		if cutRHSErr > 0 && !math.IsInf(cutRHS, 0) && !math.IsNaN(cutRHS) {
			cutRHS = nextUp(cutRHS)
		}
		cut, ok := finalizeSparseCut(p, rawIdx, rawVal, cutRHS, lpPrimal, maxNNZ, ws)
		if !ok {
			continue
		}
		scaleRat, rawPay, finalPay, exactScaled, okDrop := appendDropCheckCert(items, mip, sols, rawIdx, rawVal, cutRHS, cut)
		if !okDrop {
			continue
		}
		deltaPay, errD := float64ToPayload(delta)
		if errD != nil {
			continue
		}
		b0Rat := new(big.Rat).SetFloat64(b0)
		f0Rat := new(big.Rat).SetFloat64(f0)
		if b0Rat == nil || f0Rat == nil {
			continue
		}
		betaCertPay := bigRatToPayload(new(big.Rat).Add(b0Rat, f0Rat))

		targetCut := rawPay
		targetScale := int64ToPayload(1)
		if exactScaled {
			targetCut = finalPay
			targetScale = bigRatToPayload(scaleRat)
		}
		aggReq := exportAggCMIRCheckItem{
			Subcommand: "agg-cmir-check",
			Op:         "agg-cmir-check",
			NumVars:    mip.NumVars,
			IsInt:      mip.IsInt,
			LB:         mip.LB,
			UB:         mip.UB,
			Rows:       aggRows,
			AggTerms:   aggTerms,
			AggRHS:     aggRHS,
			Delta:      deltaPay,
			UseLower:   useLower,
			BetaHat:    &betaCertPay,
			AlphaHat:   alphaHat,
			CInt:       cInt,
			CCont:      cCont,
			Scale:      targetScale,
			Cut:        targetCut,
			Solutions:  sols,
		}
		if rawJSON, mErr := json.Marshal(aggReq); mErr == nil {
			*items = append(*items, rawJSON)
		}
	}
}

func liftCoverCertificateItem(weight, rhs float64, coverSize int, inCov bool, minWeight []float64) int {
	if inCov {
		return 1
	}
	remCap := rhs - weight
	remTol := max(coverDPTol, 16.0*unitRoundoff64*(math.Abs(rhs)+weight))
	bestV := 0
	for v := coverSize - 1; v >= 1; v-- {
		if minWeight[v] <= remCap+max(remTol, 16.0*unitRoundoff64*minWeight[v]) {
			bestV = v
			break
		}
	}
	alpha := (coverSize - 1) - bestV
	if remCap < -remTol {
		alpha = coverSize - 1
	}
	if alpha <= 0 {
		return 0
	}
	for v := coverSize - 1; v >= 1; v-- {
		cand := minWeight[max(0, v-alpha)] + weight
		if math.IsInf(cand, 1) {
			cand = math.MaxFloat64
		} else {
			cand = max(0.0, cand-8.0*unitRoundoff64*cand)
		}
		minWeight[v] = min(minWeight[v], cand)
	}
	return alpha
}

type exportAggRowMeta struct {
	tight       bool
	hasFracInt  bool
	hasCont     bool
	hasInterior bool
	relSlack    float64
	dual        float64
}

func classifyExportAggRows(solver *LPSolver, cache *cutStaticCache, lpPrimal []float64) []exportAggRowMeta {
	p := solver.prob
	m := p.numConstraints
	meta := make([]exportAggRowMeta, m)
	hasDuals := solver.dualValid && solver.ws != nil && len(solver.ws.y) >= m
	for i := range m {
		if !cache.aggRowOK[i] {
			continue
		}
		var info exportAggRowMeta
		act := 0.0
		for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
			col := p.colIdx[k]
			xv := lpPrimal[col]
			act += p.rowVal[k] * xv
			info.hasInterior = info.hasInterior || (xv > p.varLB[col]+1e-5 && xv < p.varUB[col]-1e-5)
			if p.varCat[col] == Continuous {
				info.hasCont = true
			} else if f := xv - math.Floor(xv); f >= minIntInfeas && f <= 1.0-minIntInfeas {
				info.hasFracInt = true
			}
		}
		rhs := p.rowRHS[i]
		slack := rhs - act
		switch p.rowSense[i] {
		case Equal:
			slack = math.Abs(slack)
		case GreaterEqual:
			slack = -slack
		}
		scale := max(1.0, math.Abs(rhs))
		if rs := slack / scale; slack >= -1e-4*scale && rs <= aggMaxRelSlack {
			info.tight = true
			info.relSlack = max(0.0, rs)
			if hasDuals {
				info.dual = math.Abs(solver.ws.y[i] * solver.rowScaleOf(i))
			}
			meta[i] = info
		}
	}
	return meta
}

func exportAggPivotColTier(p *CompiledProblem, col int, xv float64) int {
	interior := xv > p.varLB[col]+1e-5 && xv < p.varUB[col]-1e-5
	switch {
	case p.varCat[col] == Continuous && interior:
		return 0
	case p.varCat[col] == Continuous:
		return 1
	case p.varCat[col] == Integer && p.varUB[col]-p.varLB[col] > 1.5 && interior:
		return 2
	default:
		return 99
	}
}

func selectNextExportAggPivot(
	p *CompiledProblem,
	cache *cutStaticCache,
	meta []exportAggRowMeta,
	ws *cutGenWorkspace,
	rowGen int,
	mirMaxNNZ int,
	outCols []int,
	outVals []float64,
	lpPrimal []float64,
) (bestRow int, bestMu float64) {
	bestRow = -1
	type candKey struct {
		tier  int
		eq    int
		nnz   int
		logMu float64
	}
	bestKey := candKey{tier: 99, eq: 99, nnz: math.MaxInt, logMu: math.Inf(1)}
	for idx, col := range outCols {
		curA := outVals[idx]
		if math.Abs(curA) <= cutCoeffZeroTol || p.varLB[col] == p.varUB[col] || p.colPtr[col+1]-p.colPtr[col] > maxAggColDeg {
			continue
		}
		tier := exportAggPivotColTier(p, col, lpPrimal[col])
		if tier > bestKey.tier {
			continue
		}
		for ck := p.colPtr[col]; ck < p.colPtr[col+1]; ck++ {
			r2 := p.rowIdx[ck]
			if ws.aggRowUsed[r2] == rowGen || !cache.aggRowOK[r2] || !meta[r2].tight {
				continue
			}
			r2NNZ := p.rowPtr[r2+1] - p.rowPtr[r2]
			if r2NNZ > mirMaxNNZ {
				continue
			}
			mu := -curA / p.colVal[ck]
			absMu := math.Abs(mu)
			if math.IsNaN(mu) || math.IsInf(mu, 0) || absMu < aggMinMu || absMu > aggMaxMu || !rowLegalMuSign(p.rowSense[r2], mu) {
				continue
			}
			eqScore := 1
			if p.rowSense[r2] == Equal {
				eqScore = 0
			}
			key := candKey{tier: tier, eq: eqScore, nnz: r2NNZ, logMu: math.Abs(math.Log(absMu))}
			if cmp.Or(
				cmp.Compare(key.tier, bestKey.tier),
				cmp.Compare(key.eq, bestKey.eq),
				cmp.Compare(key.nnz, bestKey.nnz),
				cmp.Compare(key.logMu, bestKey.logMu),
			) < 0 {
				bestKey = key
				bestRow = r2
				bestMu = mu
			}
		}
	}
	return bestRow, bestMu
}

func exportSingleAndAggCMIRCerts(
	items *[][]byte,
	solver *LPSolver,
	mip track2BMIPPayload,
	sols [][]ratPayload,
	lpPrimal []float64,
	ws *cutGenWorkspace,
) {
	p := solver.prob
	cache := ws.ensureStaticCache(p)
	n := p.numVars
	m := p.numConstraints
	mirMaxNNZ := maxCutNNZ(n)

	// 1. Single-row cMIR certificates via agg-cmir-check + drop-check.
	for _, r := range cache.mirRows {
		if p.rowPtr[r+1]-p.rowPtr[r] > mirMaxNNZ {
			continue
		}
		sign := 1.0
		if p.rowSense[r] == GreaterEqual {
			sign = -1.0
		}
		muPay, errMu := float64ToPayload(sign)
		if errMu != nil {
			continue
		}
		aggRow := track2BAggRowPayload{
			Terms: mip.Rows[r].Terms,
			Sense: mip.Rows[r].Sense,
			RHS:   mip.Rows[r].RHS,
			Mu:    muPay,
		}
		s, e := p.rowPtr[r], p.rowPtr[r+1]
		exportCMIRCertsFromSparseRow(items, p, mip, sols, []track2BAggRowPayload{aggRow}, p.colIdx[s:e], p.rowVal[s:e], p.rowRHS[r], sign, lpPrimal, mirMaxNNZ, maxMIRDivisors, ws)
	}

	if n < minPairNNZ || m < 2 {
		return
	}
	ws.ensureAgg(n, m)
	meta := classifyExportAggRows(solver, cache, lpPrimal)

	makeAggRowEntry := func(rIdx int, mu float64) (track2BAggRowPayload, bool) {
		mp, errM := float64ToPayload(mu)
		if errM != nil {
			return track2BAggRowPayload{}, false
		}
		return track2BAggRowPayload{
			Terms: mip.Rows[rIdx].Terms,
			Sense: mip.Rows[rIdx].Sense,
			RHS:   mip.Rows[rIdx].RHS,
			Mu:    mp,
		}, true
	}

	// 2. Mechanism A: Marchand-Wolsey variable-elimination chains.
	var baseRows []int
	for i := range m {
		info := meta[i]
		if info.tight && p.rowPtr[i+1]-p.rowPtr[i] <= mirMaxNNZ && (info.hasCont || info.hasFracInt || info.hasInterior) {
			baseRows = append(baseRows, i)
		}
	}
	boolRank := func(v bool) int {
		if v {
			return 0
		}
		return 1
	}
	slices.SortFunc(baseRows, func(a, b int) int {
		ma, mb := meta[a], meta[b]
		return cmp.Or(
			cmp.Compare(boolRank(p.rowSense[a] == Equal), boolRank(p.rowSense[b] == Equal)),
			cmp.Compare(boolRank(ma.hasCont), boolRank(mb.hasCont)),
			cmp.Compare(boolRank(ma.hasInterior || ma.hasFracInt), boolRank(mb.hasInterior || mb.hasFracInt)),
			cmp.Compare(p.rowPtr[a+1]-p.rowPtr[a], p.rowPtr[b+1]-p.rowPtr[b]),
			cmp.Compare(a, b),
		)
	})
	if len(baseRows) > maxAggBaseRows {
		baseRows = baseRows[:maxAggBaseRows]
	}

	var signsBuf [2]float64
	for _, r0 := range baseRows {
		nSigns := 1
		switch p.rowSense[r0] {
		case Equal:
			signsBuf[0], signsBuf[1] = 1.0, -1.0
			nSigns = 2
		case LessEqual:
			signsBuf[0] = 1.0
		case GreaterEqual:
			signsBuf[0] = -1.0
		}
		for _, mu0 := range signsBuf[:nSigns] {
			firstEntry, okE := makeAggRowEntry(r0, mu0)
			if !okE {
				continue
			}
			colGen := ws.nextAggColGen()
			rowGen := ws.nextAggRowGen()
			aggCols := ws.aggCols[:0]
			var rhsAcc dot2Acc
			ws.aggRowUsed[r0] = rowGen
			addScaledProblemRowToAgg(p, r0, mu0, &rhsAcc, ws, colGen, &aggCols)
			outCols, outVals, _, ok := materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws)
			if !ok {
				ws.aggCols = aggCols
				continue
			}
			chainRows := []track2BAggRowPayload{firstEntry}
			for range maxAggSteps {
				bestRow, bestMu := selectNextExportAggPivot(p, cache, meta, ws, rowGen, mirMaxNNZ, outCols, outVals, lpPrimal)
				if bestRow == -1 {
					break
				}
				nextEntry, okNE := makeAggRowEntry(bestRow, bestMu)
				if !okNE {
					break
				}
				chainRows = append(chainRows, nextEntry)
				ws.aggRowUsed[bestRow] = rowGen
				addScaledProblemRowToAgg(p, bestRow, bestMu, &rhsAcc, ws, colGen, &aggCols)
				var aggRHS float64
				outCols, outVals, aggRHS, ok = materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws)
				if !ok {
					break
				}
				colsCopy := slices.Clone(outCols)
				valsCopy := slices.Clone(outVals)
				exportCMIRCertsFromSparseRow(items, p, mip, sols, slices.Clone(chainRows), colsCopy, valsCopy, aggRHS, 1.0, lpPrimal, mirMaxNNZ, maxMIRDivisors+2, ws)
			}
			ws.aggCols = aggCols
		}
	}

	// 3. Mechanism C: VUB/capacity sum + demand row elimination.
	vubRows := cache.vubCapRows
	if len(vubRows) >= 2 && len(vubRows) <= maxVUBClusterLen {
		var tightVUBs []int
		for _, r := range vubRows {
			if cache.aggRowOK[r] && meta[r].tight {
				tightVUBs = append(tightVUBs, r)
			}
		}
		subsets := [2][]int{tightVUBs, nil}
		if len(tightVUBs) < len(vubRows) {
			allOK := make([]int, 0, len(vubRows))
			for _, r := range vubRows {
				if cache.aggRowOK[r] {
					allOK = append(allOK, r)
				}
			}
			subsets[1] = allOK
		}
		for _, sub := range subsets {
			if len(sub) < 2 {
				continue
			}
			colGen := ws.nextAggColGen()
			rowGen := ws.nextAggRowGen()
			aggCols := ws.aggCols[:0]
			var rhsAcc dot2Acc
			chainRows := make([]track2BAggRowPayload, 0, len(sub)+2)
			entriesOK := true
			for _, r := range sub {
				ent, okE := makeAggRowEntry(r, 1.0)
				if !okE {
					entriesOK = false
					break
				}
				chainRows = append(chainRows, ent)
				ws.aggRowUsed[r] = rowGen
				addScaledProblemRowToAgg(p, r, 1.0, &rhsAcc, ws, colGen, &aggCols)
			}
			if !entriesOK {
				ws.aggCols = aggCols
				continue
			}
			elimCount := 0
			for i := range m {
				if ws.aggRowUsed[i] == rowGen || !cache.aggRowOK[i] {
					continue
				}
				s, e := p.rowPtr[i], p.rowPtr[i+1]
				if e-s < minPairNNZ {
					continue
				}
				c0 := p.colIdx[s]
				a0 := p.rowVal[s]
				if ws.aggMark[c0] != colGen || math.Abs(a0) <= cutCoeffZeroTol {
					continue
				}
				cv0, _, ok0 := ws.aggAcc[c0].result()
				if !ok0 || math.Abs(cv0) <= cutCoeffZeroTol {
					continue
				}
				mu := -cv0 / a0
				absMu := math.Abs(mu)
				if math.IsNaN(mu) || math.IsInf(mu, 0) || absMu < aggMinMu || absMu > aggMaxMu || !rowLegalMuSign(p.rowSense[i], mu) {
					continue
				}
				canElim := true
				for k := s + 1; k < e; k++ {
					col := p.colIdx[k]
					if ws.aggMark[col] != colGen {
						canElim = false
						break
					}
					cv, _, okC := ws.aggAcc[col].result()
					if !okC || math.Abs(cv+mu*p.rowVal[k]) > 1e-9*max(1.0, math.Abs(cv)) {
						canElim = false
						break
					}
				}
				if !canElim {
					continue
				}
				ent, okE := makeAggRowEntry(i, mu)
				if !okE {
					continue
				}
				chainRows = append(chainRows, ent)
				ws.aggRowUsed[i] = rowGen
				addScaledProblemRowToAgg(p, i, mu, &rhsAcc, ws, colGen, &aggCols)
				elimCount++
			}
			if elimCount > 0 {
				if outCols, outVals, aggRHS, ok := materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws); ok {
					colsCopy := slices.Clone(outCols)
					valsCopy := slices.Clone(outVals)
					exportCMIRCertsFromSparseRow(items, p, mip, sols, chainRows, colsCopy, valsCopy, aggRHS, 1.0, lpPrimal, mirMaxNNZ, maxMIRDivisors+2, ws)
					if aggCutPay, okP := exportSparseCutPayload(colsCopy, valsCopy, aggRHS); okP {
						aggRowPay := track2BRowPayload{Terms: aggCutPay.Terms, Sense: "le", RHS: aggCutPay.RHS}
						exportCoverCertFromSparseRow(items, p, mip, sols, aggRowPay, colsCopy, valsCopy, aggRHS, 1.0, lpPrimal, mirMaxNNZ, ws)
					}
				}
			}
			ws.aggCols = aggCols
		}
	}

	// 4. Pairwise tight-inequality surrogates.
	var pairRows []int
	for i := range m {
		if meta[i].tight && p.rowSense[i] != Equal && meta[i].hasFracInt && p.rowPtr[i+1]-p.rowPtr[i] <= mirMaxNNZ {
			pairRows = append(pairRows, i)
		}
	}
	if len(pairRows) < 2 {
		return
	}
	slices.SortFunc(pairRows, func(a, b int) int {
		return cmp.Or(
			cmp.Compare(meta[a].relSlack, meta[b].relSlack),
			cmp.Compare(meta[b].dual, meta[a].dual),
			cmp.Compare(a, b),
		)
	})
	if len(pairRows) > maxPairAggRows {
		pairRows = pairRows[:maxPairAggRows]
	}

	isFracCol := make([]bool, n)
	for _, col := range p.intCols {
		f := lpPrimal[col] - math.Floor(lpPrimal[col])
		if f >= minIntInfeas && f <= 1.0-minIntInfeas {
			isFracCol[col] = true
		}
	}
	rowSign := func(r int) float64 {
		if p.rowSense[r] == GreaterEqual {
			return -1.0
		}
		return 1.0
	}
	rowMaxAbs := func(r int) float64 {
		mx := 0.0
		for k := p.rowPtr[r]; k < p.rowPtr[r+1]; k++ {
			mx = max(mx, math.Abs(p.rowVal[k]))
		}
		return mx
	}

	fracMark := make([]int, n)
	fracGen := 0
	var muBuf [2]float64
	for ai := range len(pairRows) - 1 {
		r1 := pairRows[ai]
		fracGen++
		for k := p.rowPtr[r1]; k < p.rowPtr[r1+1]; k++ {
			if c := p.colIdx[k]; isFracCol[c] {
				fracMark[c] = fracGen
			}
		}
		s1 := rowSign(r1)
		max1 := rowMaxAbs(r1)
		for bi := ai + 1; bi < len(pairRows); bi++ {
			r2 := pairRows[bi]
			sharedFrac := false
			for k := p.rowPtr[r2]; k < p.rowPtr[r2+1]; k++ {
				if fracMark[p.colIdx[k]] == fracGen {
					sharedFrac = true
					break
				}
			}
			if !sharedFrac {
				continue
			}
			s2 := rowSign(r2)
			max2 := rowMaxAbs(r2)
			muBuf[0] = 1.0
			nMu := 1
			if max1 > 0 && max2 > 0 {
				ratio := max1 / max2
				if (ratio < 0.95 || ratio > 1.05) && ratio >= 1e-3 && ratio <= 1e3 {
					muBuf[1] = ratio
					nMu = 2
				}
			}
			for _, mu := range muBuf[:nMu] {
				ent1, ok1 := makeAggRowEntry(r1, s1)
				ent2, ok2 := makeAggRowEntry(r2, s2*mu)
				if !ok1 || !ok2 {
					continue
				}
				colGen := ws.nextAggColGen()
				aggCols := ws.aggCols[:0]
				var rhsAcc dot2Acc
				addScaledProblemRowToAgg(p, r1, s1, &rhsAcc, ws, colGen, &aggCols)
				addScaledProblemRowToAgg(p, r2, s2*mu, &rhsAcc, ws, colGen, &aggCols)
				outCols, outVals, aggRHS, ok := materializeAggRow(p, rhsAcc, aggCols, mirMaxNNZ, ws)
				ws.aggCols = aggCols
				if !ok {
					continue
				}
				colsCopy := slices.Clone(outCols)
				valsCopy := slices.Clone(outVals)
				exportCMIRCertsFromSparseRow(items, p, mip, sols, []track2BAggRowPayload{ent1, ent2}, colsCopy, valsCopy, aggRHS, 1.0, lpPrimal, mirMaxNNZ, maxMIRDivisors, ws)
				if aggCutPay, okP := exportSparseCutPayload(colsCopy, valsCopy, aggRHS); okP {
					aggRowPay := track2BRowPayload{Terms: aggCutPay.Terms, Sense: "le", RHS: aggCutPay.RHS}
					exportCoverCertFromSparseRow(items, p, mip, sols, aggRowPay, colsCopy, valsCopy, aggRHS, 1.0, lpPrimal, mirMaxNNZ, ws)
				}
			}
		}
	}
}

// ExportSeparatedCutsItems solves the root LP of p, separates GMI, single-row and
// multi-row aggregated cMIR, knapsack cover, and disaggregated VUB / (l, S) lot-sizing
// cuts, and packages universal Track 2C certificates (cover-check, lotsizing-check,
// agg-cmir-check, drop-check) alongside optional exact-witness Track 2B batches for
// cuts-check --jsonl. Semi-bounded and unbounded MIPs export cuts without requiring
// finite variable boxes or pre-supplied witness solutions.
func ExportSeparatedCutsItems(p *CompiledProblem, feasibleSols [][]float64) ([][]byte, error) {
	if p == nil {
		return nil, errors.New("nil CompiledProblem")
	}
	rp, okRP := buildVIPRRatProblem(p)
	if !okRP {
		return nil, errors.New("non-finite coefficients in CompiledProblem")
	}

	mipPayload, domainFeas, err := exportCompiledToMIPPayloadChecked(p)
	if err != nil {
		return nil, err
	}
	if !domainFeas || p.numVars == 0 || p.numConstraints == 0 {
		return nil, nil
	}

	var scratchLP *LPSolver
	getScratch := func() *LPSolver {
		if scratchLP == nil {
			scratchLP = NewLPSolver(p, nil)
		}
		return scratchLP
	}
	defer func() {
		if scratchLP != nil {
			scratchLP.Close()
		}
	}()

	var solPayloads [][]ratPayload
	if len(feasibleSols) > 0 {
		solPayloads = make([][]ratPayload, 0, len(feasibleSols))
		for _, sol := range feasibleSols {
			if len(sol) != p.numVars {
				continue
			}
			xRat, _, ok := rp.refineExactIncumbent(sol, getScratch)
			if !ok {
				xRat, ok = refineExactMIPAnchor(exactStop{}, p, sol)
			}
			if !ok {
				continue
			}
			row := make([]ratPayload, p.numVars)
			for j := range p.numVars {
				row[j] = bigRatToPayload(xRat[j])
			}
			solPayloads = append(solPayloads, row)
		}
	}

	lp := NewLPSolver(p, nil)
	defer lp.Close()
	rootRes := lp.Solve()
	if rootRes.Status != StatusOptimal {
		return nil, nil
	}

	var ws cutGenWorkspace
	var items [][]byte

	if len(solPayloads) > 0 {
		appendCutBatch := func(op string, cuts []cutPlane) error {
			if len(cuts) == 0 {
				return nil
			}
			cutPays, cErr := exportCutsToPayloads(cuts)
			if cErr != nil {
				return cErr
			}
			raw, mErr := json.Marshal(exportCutCheckItem{
				Subcommand: op,
				Op:         op,
				MIP:        mipPayload,
				Cuts:       cutPays,
				Solutions:  solPayloads,
			})
			if mErr != nil {
				return mErr
			}
			items = append(items, raw)
			return nil
		}

		if err := appendCutBatch("gmi-check", separateGMICutsEx(lp, rootRes.Primal, &ws, false)); err != nil {
			return nil, err
		}
		if err := appendCutBatch("cmir-check", separateRowMIRCutsWithWS(p, rootRes.Primal, &ws)); err != nil {
			return nil, err
		}
		if err := appendCutBatch("vub-check", separateFlowAndVUBCuts(p, rootRes.Primal, &ws)); err != nil {
			return nil, err
		}
	}

	// Export universal Track 2C certificates (cover-check, lotsizing-check, agg-cmir-check, drop-check).
	cache := ws.ensureStaticCache(p)
	for _, r := range cache.coverRows {
		sign := 1.0
		if p.rowSense[r] == GreaterEqual {
			sign = -1.0
		}
		s, e := p.rowPtr[r], p.rowPtr[r+1]
		exportCoverCertFromSparseRow(&items, p, mipPayload, solPayloads, mipPayload.Rows[r], p.colIdx[s:e], p.rowVal[s:e], p.rowRHS[r], sign, rootRes.Primal, 0, &ws)
	}
	exportVUBAndLotSizingCerts(&items, p, mipPayload, solPayloads, rootRes.Primal, &ws)
	exportSingleAndAggCMIRCerts(&items, lp, mipPayload, solPayloads, rootRes.Primal, &ws)

	return items, nil
}
