package goblin

import (
	"bytes"
	"encoding/json/v2"
	"math"
	"math/big"
	"math/rand/v2"
	"os/exec"
	"slices"
	"strconv"
	"testing"
	"time"
)

func runRulesCheck(tb testing.TB, exe, subcmd string, req any, dst any) {
	tb.Helper()
	payload, err := json.Marshal(req)
	if err != nil {
		tb.Fatalf("marshal %s payload: %v", subcmd, err)
	}
	cmd := exec.CommandContext(tb.Context(), exe, subcmd)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tb.Fatalf("rules-check %s failed: %v\nstderr: %s\nstdout: %s", subcmd, err, stderr.String(), stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), dst); err != nil {
		tb.Fatalf("unmarshal %s output: %v\nraw: %s", subcmd, err, stdout.String())
	}
}

func ratPayloadToBigRat(tb testing.TB, p *ratPayload) *big.Rat {
	tb.Helper()
	if p == nil {
		tb.Fatal("unexpected nil ratPayload")
	}
	num, ok := new(big.Int).SetString(p.Num, 10)
	if !ok {
		tb.Fatalf("invalid rat num %q", p.Num)
	}
	den, ok := new(big.Int).SetString(p.Den, 10)
	if !ok || den.Sign() <= 0 {
		tb.Fatalf("invalid rat den %q", p.Den)
	}
	return new(big.Rat).SetFrac(num, den)
}

func float64ToBigRat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	if math.IsNaN(v) || math.IsInf(v, 0) {
		tb.Fatalf("non-finite float64 %v", v)
	}
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("SetFloat64(%v) returned nil", v)
	}
	return r
}

func mustFloat64Payload(tb testing.TB, v float64) ratPayload {
	tb.Helper()
	p, err := float64ToPayload(v)
	if err != nil {
		tb.Fatalf("float64ToPayload(%v): %v", v, err)
	}
	return p
}

func optFloat64ToAny(tb testing.TB, v float64, infSign int) any {
	tb.Helper()
	if math.IsInf(v, infSign) {
		return nil
	}
	return mustFloat64Payload(tb, v)
}

func extFloat64ToAny(tb testing.TB, v float64) any {
	tb.Helper()
	if math.IsInf(v, -1) {
		return "negInf"
	}
	if math.IsInf(v, 1) {
		return "posInf"
	}
	return mustFloat64Payload(tb, v)
}

func senseToStr(s ConstraintSense) string {
	switch s {
	case LessEqual:
		return "le"
	case GreaterEqual:
		return "ge"
	default:
		return "eq"
	}
}

type divCeilReq struct {
	A int64 `json:"a"`
	D int64 `json:"d"`
}

type divCeilResp struct {
	Q     int64 `json:"q"`
	Floor int64 `json:"floor"`
}

type gcdInfeasReq struct {
	Coeffs     []int64 `json:"coeffs"`
	Lo         int64   `json:"lo"`
	Hi         int64   `json:"hi"`
	CandidateG *int64  `json:"candidate_g"`
}

type gcdInfeasResp struct {
	GCD                 int64 `json:"gcd"`
	DividesAll          bool  `json:"divides_all"`
	Infeasible          bool  `json:"infeasible"`
	CandidateInfeasible bool  `json:"candidate_infeasible"`
}

type activityTermReq struct {
	A     ratPayload `json:"a"`
	L     any        `json:"l"`
	U     any        `json:"u"`
	IsInt bool       `json:"is_int"`
}

type activitySummaryReq struct {
	Op    string            `json:"op"`
	RHS   ratPayload        `json:"rhs"`
	HasLo bool              `json:"has_lo"`
	HasHi bool              `json:"has_hi"`
	Terms []activityTermReq `json:"terms"`
}

type activityTermResp struct {
	QuotUpper *ratPayload `json:"quot_upper"`
	QuotLower *ratPayload `json:"quot_lower"`
	ImpliedUB *ratPayload `json:"implied_ub"`
	ImpliedLB *ratPayload `json:"implied_lb"`
}

type activitySummaryResp struct {
	MinInfCount int                `json:"min_inf_count"`
	MaxInfCount int                `json:"max_inf_count"`
	MinAct      *ratPayload        `json:"min_act"`
	MaxAct      *ratPayload        `json:"max_act"`
	Slack       *ratPayload        `json:"slack"`
	Deficit     *ratPayload        `json:"deficit"`
	Terms       []activityTermResp `json:"terms"`
}

type activitySkipReq struct {
	Op          string     `json:"op"`
	SlackLo     ratPayload `json:"slack_lo"`
	AbsA        ratPayload `json:"abs_a"`
	L           ratPayload `json:"l"`
	U           ratPayload `json:"u"`
	TargetBound ratPayload `json:"target_bound"`
	IsInt       bool       `json:"is_int"`
}

type activitySkipResp struct {
	CanSkip          bool `json:"can_skip"`
	RawStrictExceeds bool `json:"raw_strict_exceeds"`
}

type binStrengthenReq struct {
	RHS      ratPayload  `json:"rhs"`
	A        ratPayload  `json:"a"`
	MaxOther ratPayload  `json:"max_other"`
	Surplus  ratPayload  `json:"surplus"`
	NewA     *ratPayload `json:"new_a"`
	NewRHS   *ratPayload `json:"new_rhs"`
}

type binStrengthenResp struct {
	Applicable    bool        `json:"applicable"`
	SpecNewA      *ratPayload `json:"spec_new_a"`
	SpecNewRHS    *ratPayload `json:"spec_new_rhs"`
	ProposedValid bool        `json:"proposed_valid"`
	ExactIff      bool        `json:"exact_iff"`
}

type orbitRowReq struct {
	Active bool         `json:"active"`
	Cols   []int        `json:"cols"`
	Vals   []ratPayload `json:"vals"`
	Sense  string       `json:"sense"`
	RHS    ratPayload   `json:"rhs"`
}

type orbitCheckReq struct {
	NumVars   int           `json:"num_vars"`
	IsInt     []bool        `json:"is_int"`
	LB        []any         `json:"lb"`
	UB        []any         `json:"ub"`
	Obj       []ratPayload  `json:"obj"`
	Rows      []orbitRowReq `json:"rows"`
	GCount    int           `json:"g_count"`
	Orbits    []int         `json:"orbits"`
	RowTuples []int         `json:"row_tuples"`
	ColOrbit  []int         `json:"col_orbit"`
	ColGroup  []int         `json:"col_group"`
	RowState  []int         `json:"row_state"`
}

type orbitCheckResp struct {
	Valid          bool `json:"valid"`
	ExactObj       bool `json:"exact_obj"`
	MonotoneNonNeg bool `json:"monotone_non_neg"`
}

func TestFormalTrack2A(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	exe := formalExe(t, "rules-check")

	// Verify no-argument usage invocation exits 0 cleanly.
	if out, err := exec.CommandContext(t.Context(), exe).CombinedOutput(); err != nil {
		t.Fatalf("rules-check usage failed: %v\n%s", err, out)
	}

	t.Run("DivCeilAndFloor", func(t *testing.T) {
		t.Parallel()
		edges := []divCeilReq{
			{A: 0, D: 1},
			{A: 0, D: 7},
			{A: 1, D: 1},
			{A: -1, D: 1},
			{A: -1, D: 2},
			{A: 1, D: 2},
			{A: -7, D: 3},
			{A: 7, D: 3},
			{A: -9, D: 3},
			{A: 9, D: 3},
			{A: math.MinInt64, D: 1},
			{A: math.MinInt64, D: 2},
			{A: math.MinInt64 + 1, D: 2},
			{A: math.MinInt64, D: 7},
			{A: math.MinInt64 + 1, D: 7},
			{A: math.MaxInt64, D: 1},
			{A: math.MaxInt64, D: 2},
			{A: math.MaxInt64 - 1, D: 2},
			{A: math.MaxInt64, D: 7},
			{A: -1000000007, D: 1000000007},
			{A: -1000000008, D: 1000000007},
		}
		rng := rand.New(rand.NewPCG(0x2a01, 0x9e3779b9))
		reqs := make([]divCeilReq, 0, len(edges)+160)
		reqs = append(reqs, edges...)
		for i := range 160 {
			var a, d int64
			switch i % 4 {
			case 0:
				a = int64(rng.IntN(2001)) - 1000
				d = int64(rng.IntN(97)) + 1
			case 1:
				d = int64(rng.IntN(1_000_000)) + 1
				a = (int64(rng.IntN(200)) - 100) * d
			case 2:
				a = int64(rng.Uint64() >> 1)
				if rng.IntN(2) == 0 {
					a = -a - 1
				}
				d = int64(rng.IntN(1_000_000_007)) + 1
			default:
				a = int64(rng.Uint64() >> 1)
				if rng.IntN(2) == 0 {
					a = -a
				}
				d = int64(rng.Uint64()>>2) + 1
			}
			reqs = append(reqs, divCeilReq{A: a, D: d})
		}

		var got []divCeilResp
		runRulesCheck(t, exe, "divceil", reqs, &got)
		if len(got) != len(reqs) {
			t.Fatalf("len(got)=%d, want %d", len(got), len(reqs))
		}
		for i, r := range reqs {
			wantQ := divCeilInt64(r.A, r.D)
			wantF := divFloorInt64(r.A, r.D)
			if got[i].Q != wantQ || got[i].Floor != wantF {
				t.Fatalf("case %d (%d, %d): lean=(%d,%d), go=(%d,%d)",
					i, r.A, r.D, got[i].Q, got[i].Floor, wantQ, wantF)
			}
		}
	})

	t.Run("GCDAndInfeasibleIntEqRow", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(0x2a02, 0x517cc1b7))

		type eqCase struct {
			coeffs   []int64
			rhs      float64
			candG    *int64
			goInfeas bool
			goGCD    int64
		}
		cases := make([]eqCase, 0, 150)

		buildEq := func(coeffs []int64, rhs float64, candG *int64) eqCase {
			m := NewModel("eq_gcd", Minimize)
			terms := make([]Term, len(coeffs))
			for j, c := range coeffs {
				v := m.AddIntVar("x_"+strconv.Itoa(j), -1000, 1000)
				terms[j] = v.T(float64(c))
			}
			m.AddConstraint("eq", NewExpr(terms...).Eq(rhs))
			cp := m.Compile()
			var g int64
			for _, c := range coeffs {
				if c < 0 {
					c = -c
				}
				if c > 0 {
					g = gcdInt64(g, c)
				}
			}
			return eqCase{
				coeffs:   coeffs,
				rhs:      rhs,
				candG:    candG,
				goInfeas: hasInfeasibleIntegerEqRow(cp),
				goGCD:    g,
			}
		}

		// Structured positive/negative/boundary cases.
		nonDiv := int64(3)
		cases = append(cases,
			buildEq([]int64{6, 10, 14}, 5.0, &nonDiv),
			buildEq([]int64{6, 10, 14}, 6.0, nil),
			buildEq([]int64{-4, 6}, -5.0, nil),
			buildEq([]int64{-4, 6}, -4.0-1e-7, nil),
			buildEq([]int64{10, -15}, 7.0, nil),
			buildEq([]int64{10, -15}, 5.0+5e-7, nil),
			buildEq([]int64{12, 18, 30}, 17.0, nil),
			buildEq([]int64{12, 18, 30}, 18.0, nil),
		)

		for i := range 142 {
			k := 2 + rng.IntN(5)
			baseG := int64(2 + rng.IntN(17))
			if i%5 == 0 {
				baseG = 1
			}
			coeffs := make([]int64, k)
			for j := range k {
				mult := int64(rng.IntN(41)) - 20
				if j == 0 && mult == 0 {
					mult = 1
				}
				coeffs[j] = mult * baseG
			}
			q := int64(rng.IntN(201)) - 100
			rem := int64(0)
			if baseG > 1 && i%2 == 0 {
				rem = int64(1 + rng.IntN(int(baseG-1)))
			}
			rhs := float64(q*baseG + rem)
			if i%7 == 0 {
				rhs += 1e-7 * float64(rng.IntN(5)-2)
			}
			var cand *int64
			if i%6 == 0 && baseG > 1 {
				bad := baseG + 1
				cand = &bad
			}
			cases = append(cases, buildEq(coeffs, rhs, cand))
		}

		reqs := make([]gcdInfeasReq, len(cases))
		for i, c := range cases {
			reqs[i] = gcdInfeasReq{
				Coeffs:     c.coeffs,
				Lo:         int64(math.Ceil(c.rhs - presolveFeasTol)),
				Hi:         int64(math.Floor(c.rhs + presolveFeasTol)),
				CandidateG: c.candG,
			}
		}

		var got []gcdInfeasResp
		runRulesCheck(t, exe, "gcd-infeas", reqs, &got)
		if len(got) != len(cases) {
			t.Fatalf("len(got)=%d, want %d", len(got), len(cases))
		}
		for i, c := range cases {
			if got[i].GCD != c.goGCD {
				t.Fatalf("case %d gcd mismatch: lean=%d go=%d coeffs=%v", i, got[i].GCD, c.goGCD, c.coeffs)
			}
			if got[i].Infeasible != c.goInfeas {
				t.Fatalf("case %d infeas mismatch: lean=%v go=%v coeffs=%v rhs=%v",
					i, got[i].Infeasible, c.goInfeas, c.coeffs, c.rhs)
			}
			if c.candG != nil && got[i].DividesAll {
				t.Fatalf("case %d expected non-divisor candidate_g=%d to fail divides_all on coeffs=%v",
					i, *c.candG, c.coeffs)
			}
		}

		// Verify live Presolve catches an infeasible GCD row.
		mInf := NewModel("presolve_gcd_inf", Minimize)
		x0 := mInf.AddIntVar("x0", -10, 10)
		x1 := mInf.AddIntVar("x1", -10, 10)
		mInf.AddConstraint("eq", NewExpr(x0.T(6), x1.T(10)).Eq(5))
		if pres := Presolve(mInf.Compile(), 3); !pres.Infeasible {
			t.Fatal("expected Presolve to detect GCD infeasibility on 6*x0 + 10*x1 = 5")
		}
	})

	t.Run("ActivitySummaryAndSkipTerm", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(0x2a03, 0xbf58476d))

		type rowCheck struct {
			cols   []int
			vals   []float64
			lb     []float64
			ub     []float64
			isInt  []bool
			rhs    float64
			sense  ConstraintSense
			goSum  rowActivitySummary
			hasLo  bool
			hasHi  bool
			qUVals []float64
			qUEBs  []float64
			qUOKs  []bool
			qLVals []float64
			qLEBs  []float64
			qLOKs  []bool
			rowTol float64
			bTols  []float64
		}

		rowCases := make([]rowCheck, 0, 100)
		for i := range 100 {
			nTerms := 1 + rng.IntN(6)
			cols := make([]int, nTerms)
			vals := make([]float64, nTerms)
			lb := make([]float64, nTerms)
			ub := make([]float64, nTerms)
			isInt := make([]bool, nTerms)
			for j := range nTerms {
				cols[j] = j
				v := float64(rng.IntN(21)-10) + 0.25*float64(rng.IntN(4))
				if v == 0 {
					v = 1.5
				}
				vals[j] = v
				isInt[j] = (i+j)%2 == 0
				l := float64(rng.IntN(11) - 5)
				u := l + float64(rng.IntN(9))
				if !isInt[j] && rng.IntN(3) == 0 {
					l += 0.125
					u += 0.375
				}
				if i%10 == 0 && j == 0 {
					if v > 0 {
						l = math.Inf(-1)
					} else {
						u = math.Inf(1)
					}
				} else if i%15 == 0 && j == nTerms-1 {
					if v > 0 {
						u = math.Inf(1)
					} else {
						l = math.Inf(-1)
					}
				}
				lb[j] = l
				ub[j] = u
			}
			sense := LessEqual
			switch i % 3 {
			case 1:
				sense = GreaterEqual
			case 2:
				sense = Equal
			}
			hasLo := sense == GreaterEqual || sense == Equal
			hasHi := sense == LessEqual || sense == Equal
			rhs := float64(rng.IntN(41) - 20)
			if i%4 == 0 {
				rhs += 0.5
			}
			s, _ := evalRowActivitySummary(cols, vals, lb, ub, rhs, false, true, true)
			rowScale := max(1.0, max(s.l1Min, s.l1Max))
			rowTol := max(presolveFeasTol, presolveRelScaleTol*rowScale)
			qUVals := make([]float64, nTerms)
			qUEBs := make([]float64, nTerms)
			qUOKs := make([]bool, nTerms)
			qLVals := make([]float64, nTerms)
			qLEBs := make([]float64, nTerms)
			qLOKs := make([]bool, nTerms)
			bTols := make([]float64, nTerms)
			for j := range nTerms {
				absA := math.Abs(vals[j])
				bTols[j] = max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
				if hasHi && s.minInfCount <= 1 && !(s.minInfCount == 1 && ((vals[j] > 0 && !math.IsInf(lb[j], -1)) || (vals[j] < 0 && !math.IsInf(ub[j], 1)))) {
					qv, qe, ok := s.residualQuotient(cols, vals, lb, ub, rhs, j, true)
					qUVals[j], qUEBs[j], qUOKs[j] = qv, qe, ok
				}
				if hasLo && s.maxInfCount <= 1 && !(s.maxInfCount == 1 && ((vals[j] > 0 && !math.IsInf(ub[j], 1)) || (vals[j] < 0 && !math.IsInf(lb[j], -1)))) {
					qv, qe, ok := s.residualQuotient(cols, vals, lb, ub, rhs, j, false)
					qLVals[j], qLEBs[j], qLOKs[j] = qv, qe, ok
				}
			}
			rowCases = append(rowCases, rowCheck{
				cols: cols, vals: vals, lb: lb, ub: ub, isInt: isInt,
				rhs: rhs, sense: sense, goSum: s, hasLo: hasLo, hasHi: hasHi,
				qUVals: qUVals, qUEBs: qUEBs, qUOKs: qUOKs,
				qLVals: qLVals, qLEBs: qLEBs, qLOKs: qLOKs,
				rowTol: rowTol, bTols: bTols,
			})
		}

		sumReqs := make([]activitySummaryReq, len(rowCases))
		for i, rc := range rowCases {
			terms := make([]activityTermReq, len(rc.cols))
			for j := range rc.cols {
				terms[j] = activityTermReq{
					A:     mustFloat64Payload(t, rc.vals[j]),
					L:     optFloat64ToAny(t, rc.lb[j], -1),
					U:     optFloat64ToAny(t, rc.ub[j], 1),
					IsInt: rc.isInt[j],
				}
			}
			sumReqs[i] = activitySummaryReq{
				Op:    "summary",
				RHS:   mustFloat64Payload(t, rc.rhs),
				HasLo: rc.hasLo,
				HasHi: rc.hasHi,
				Terms: terms,
			}
		}

		var sumGot []activitySummaryResp
		runRulesCheck(t, exe, "activity", sumReqs, &sumGot)
		if len(sumGot) != len(rowCases) {
			t.Fatalf("len(sumGot)=%d, want %d", len(sumGot), len(rowCases))
		}
		for i, rc := range rowCases {
			gs := sumGot[i]
			if gs.MinInfCount != rc.goSum.minInfCount || gs.MaxInfCount != rc.goSum.maxInfCount {
				t.Fatalf("row %d infCount mismatch: lean=(%d,%d) go=(%d,%d)",
					i, gs.MinInfCount, gs.MaxInfCount, rc.goSum.minInfCount, rc.goSum.maxInfCount)
			}
			if rc.goSum.extreme {
				continue
			}
			if rc.goSum.minInfCount == 0 {
				exactSlack := ratPayloadToBigRat(t, gs.Slack)
				lo := float64ToBigRat(t, rc.goSum.slackLo)
				hi := float64ToBigRat(t, rc.goSum.slackHi)
				if exactSlack.Cmp(lo) < 0 || exactSlack.Cmp(hi) > 0 {
					t.Fatalf("row %d slack %s not bracketed by [%v, %v]", i, exactSlack.RatString(), rc.goSum.slackLo, rc.goSum.slackHi)
				}
			}
			if rc.goSum.maxInfCount == 0 {
				exactDef := ratPayloadToBigRat(t, gs.Deficit)
				lo := float64ToBigRat(t, rc.goSum.deficitLo)
				hi := float64ToBigRat(t, rc.goSum.deficitHi)
				if exactDef.Cmp(lo) < 0 || exactDef.Cmp(hi) > 0 {
					t.Fatalf("row %d deficit %s not bracketed by [%v, %v]", i, exactDef.RatString(), rc.goSum.deficitLo, rc.goSum.deficitHi)
				}
			}
			checkQuot := func(i, j int, label string, qOK bool, qVal, qEB float64, leanQ *ratPayload,
				hasSide, impliesUB, isInt bool, rowTol, bTol, coeff float64, leanUB, leanLB *ratPayload) {
				if !qOK || leanQ == nil {
					return
				}
				exactQ := ratPayloadToBigRat(t, leanQ)
				qv := float64ToBigRat(t, qVal)
				qe := float64ToBigRat(t, qEB)
				qLo := new(big.Rat).Sub(qv, qe)
				qHi := new(big.Rat).Add(qv, qe)
				if exactQ.Cmp(qLo) < 0 || exactQ.Cmp(qHi) > 0 {
					t.Fatalf("row %d term %d %s %s not bracketed by [%v-%v, %v+%v]",
						i, j, label, exactQ.RatString(), qVal, qEB, qVal, qEB)
				}
				if !hasSide {
					return
				}
				absA := math.Abs(coeff)
				if impliesUB && leanUB != nil {
					exactUB := ratPayloadToBigRat(t, leanUB)
					goUB := qVal + qEB + rowTol/absA
					if isInt {
						goUB = math.Floor(qVal + qEB + bTol)
					}
					if float64ToBigRat(t, goUB).Cmp(exactUB) < 0 {
						t.Fatalf("row %d term %d %s Go impliedUB %v < Lean exactUB %s", i, j, label, goUB, exactUB.RatString())
					}
				}
				if !impliesUB && leanLB != nil {
					exactLB := ratPayloadToBigRat(t, leanLB)
					goLB := qVal - qEB - rowTol/absA
					if isInt {
						goLB = math.Ceil(qVal - qEB - bTol)
					}
					if float64ToBigRat(t, goLB).Cmp(exactLB) > 0 {
						t.Fatalf("row %d term %d %s Go impliedLB %v > Lean exactLB %s", i, j, label, goLB, exactLB.RatString())
					}
				}
			}
			for j := range rc.cols {
				checkQuot(i, j, "quotUpper", rc.qUOKs[j], rc.qUVals[j], rc.qUEBs[j], gs.Terms[j].QuotUpper,
					rc.hasHi, rc.vals[j] > 0, rc.isInt[j], rc.rowTol, rc.bTols[j], rc.vals[j],
					gs.Terms[j].ImpliedUB, gs.Terms[j].ImpliedLB)
				checkQuot(i, j, "quotLower", rc.qLOKs[j], rc.qLVals[j], rc.qLEBs[j], gs.Terms[j].QuotLower,
					rc.hasLo, rc.vals[j] < 0, rc.isInt[j], rc.rowTol, rc.bTols[j], rc.vals[j],
					gs.Terms[j].ImpliedUB, gs.Terms[j].ImpliedLB)
			}
		}

		// Differential parity & soundness for presolveCanSkipTerm (120+ cases).
		type skipCase struct {
			slackLo, absA, l, u, target float64
			isInt                       bool
			nearBoundary                bool
		}
		skipCases := []skipCase{
			{slackLo: -1, absA: 2, l: 0, u: 1, target: 1, isInt: true},
			{slackLo: 0, absA: 2, l: 0, u: 1, target: 1, isInt: true},
			{slackLo: 3, absA: 2, l: 0, u: 1.5, target: 1.5, isInt: true},
			{slackLo: 3, absA: 2, l: 0, u: 1.5, target: 1.5, isInt: false},
			{slackLo: 2.0, absA: 2, l: 0, u: 1, target: 1, isInt: true},
			{slackLo: 2.0 * (1 + 0x1p-29), absA: 2, l: 0, u: 1, target: 1, isInt: true},
			{slackLo: 2.0 * (1 + 0x1p-31), absA: 2, l: 0, u: 1, target: 1, isInt: true, nearBoundary: true},
			{slackLo: 5, absA: 2, l: 3, u: 1, target: 1, isInt: false},
		}
		for i := range 120 {
			l := float64(rng.IntN(21)-10) * 0.5
			u := l + float64(rng.IntN(16))*0.5
			absA := float64(1+rng.IntN(20)) * 0.5
			prod := absA * (u - l)
			var slackLo float64
			switch i % 4 {
			case 0:
				slackLo = prod * 1.05
			case 1:
				slackLo = prod * 0.95
			case 2:
				slackLo = prod*(1+0x1p-28) + 0.01
			default:
				slackLo = prod - 0.01
			}
			isInt := i%2 == 0
			target := u
			if i%3 == 0 {
				target = l
			}
			skipCases = append(skipCases, skipCase{
				slackLo: slackLo, absA: absA, l: l, u: u, target: target, isInt: isInt,
			})
		}

		skipReqs := make([]activitySkipReq, len(skipCases))
		for i, sc := range skipCases {
			skipReqs[i] = activitySkipReq{
				Op:          "skip",
				SlackLo:     mustFloat64Payload(t, sc.slackLo),
				AbsA:        mustFloat64Payload(t, sc.absA),
				L:           mustFloat64Payload(t, sc.l),
				U:           mustFloat64Payload(t, sc.u),
				TargetBound: mustFloat64Payload(t, sc.target),
				IsInt:       sc.isInt,
			}
		}
		var skipGot []activitySkipResp
		runRulesCheck(t, exe, "activity", skipReqs, &skipGot)
		if len(skipGot) != len(skipCases) {
			t.Fatalf("len(skipGot)=%d, want %d", len(skipGot), len(skipCases))
		}
		for i, sc := range skipCases {
			goSkip := presolveCanSkipTerm(sc.slackLo, sc.absA, sc.l, sc.u, sc.target, sc.isInt)
			if goSkip && !skipGot[i].RawStrictExceeds {
				t.Fatalf("skip case %d unsound: Go skipped without raw strict inequality", i)
			}
			if !sc.nearBoundary && goSkip != skipGot[i].CanSkip {
				t.Fatalf("skip case %d parity mismatch: go=%v lean=%v (%+v)", i, goSkip, skipGot[i].CanSkip, sc)
			}
		}
	})

	t.Run("BinaryCoeffStrengthening", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(0x2a04, 0x94d049bb))

		type binLiveCase struct {
			coeffs   []float64
			rhs      float64
			k        int
			maxOther float64
			surplus  float64
			newA     float64
			newRHS   float64
		}

		var liveCases []binLiveCase
		for i := range 60 {
			var coeffs []float64
			var rhs float64
			var k int
			if i%2 == 0 {
				// Positive coefficient strengthening without Phase 1 GCD or Phase 2 variable fixing.
				c1 := float64(3 + rng.IntN(3))
				c2 := float64(4 + rng.IntN(3))
				for gcdInt64(int64(c1), int64(c2)) != 1 {
					c2++
				}
				a0 := max(c1, c2) + 2 + float64(rng.IntN(3))
				rhs = max(a0, c1+c2+1)
				if i%4 == 2 {
					a0 += 0.3
					rhs = max(math.Ceil(a0), c1+c2+1)
				}
				coeffs = []float64{a0, c1, c2}
				k = 0
			} else {
				// Negative coefficient strengthening without Phase 1 GCD or Phase 2 variable fixing.
				c1 := float64(3 + rng.IntN(3))
				c2 := float64(4 + rng.IntN(3))
				for gcdInt64(int64(c1), int64(c2)) != 1 {
					c2++
				}
				negA := max(c1, c2) + 2 + float64(rng.IntN(3))
				if i%4 == 3 {
					negA += 0.3
				}
				rhs = c1 + c2 - max(c1, c2) - 1
				coeffs = []float64{-negA, c1, c2}
				k = 0
			}

			mod := NewModel("bin_str_"+strconv.Itoa(i), Minimize)
			vars := make([]Var, len(coeffs))
			terms := make([]Term, len(coeffs))
			for j, a := range coeffs {
				vars[j] = mod.AddBinaryVar("b_" + strconv.Itoa(j))
				terms[j] = vars[j].T(a)
			}
			mod.AddConstraint("c0", NewExpr(terms...).Le(rhs))
			mod.SetObjective(NewExpr(vars[0].T(1), vars[1].T(1), vars[2].T(1)))
			cp := mod.Compile()

			res := Presolve(cp, 1)
			if res.Infeasible || res.StrengthenedRows == 0 || res.Reduced.numConstraints != 1 {
				continue
			}

			var s rowActivitySummary
			lb := cp.varLB
			ub := cp.varUB
			evalRowActivitySummaryInto(&s, cp.colIdx[cp.rowPtr[0]:cp.rowPtr[1]], cp.rowVal[cp.rowPtr[0]:cp.rowPtr[1]], lb, ub, cp.rowRHS[0], false, false, true)
			surplusHi := -s.deficitLo

			red := res.Reduced
			var newA float64
			redCoeffs := make([]float64, len(coeffs))
			for p := red.rowPtr[0]; p < red.rowPtr[1]; p++ {
				col := red.colIdx[p]
				redCoeffs[col] = red.rowVal[p]
				if col == k {
					newA = red.rowVal[p]
				}
			}
			if res.StrengthenedRows != 1 {
				continue
			}
			maxOther := 0.0
			for j, a := range coeffs {
				if j != k && a > 0 {
					maxOther += a
				}
			}
			liveCases = append(liveCases, binLiveCase{
				coeffs:   coeffs,
				rhs:      rhs,
				k:        k,
				maxOther: maxOther,
				surplus:  surplusHi,
				newA:     newA,
				newRHS:   red.rowRHS[0],
			})

			// Exhaustively verify feasible binary solutions match before and after strengthening.
			for mask := range 8 {
				origSum := 0.0
				newSum := 0.0
				inRedBounds := true
				for j := range 3 {
					xj := float64((mask >> j) & 1)
					if xj < red.varLB[j]-1e-9 || xj > red.varUB[j]+1e-9 {
						inRedBounds = false
					}
					origSum += coeffs[j] * xj
					newSum += redCoeffs[j] * xj
				}
				origFeas := origSum <= rhs+1e-9
				newFeas := inRedBounds && newSum <= red.rowRHS[0]+1e-9
				if origFeas != newFeas {
					t.Fatalf("case %d mask %d feasibility mismatch: orig=%v new=%v", i, mask, origFeas, newFeas)
				}
			}
		}

		if len(liveCases) < 30 {
			t.Fatalf("expected at least 30 live strengthened binary rows, got %d", len(liveCases))
		}

		reqs := make([]binStrengthenReq, 0, len(liveCases)+40)
		for _, lc := range liveCases {
			na := mustFloat64Payload(t, lc.newA)
			nr := mustFloat64Payload(t, lc.newRHS)
			reqs = append(reqs, binStrengthenReq{
				RHS:      mustFloat64Payload(t, lc.rhs),
				A:        mustFloat64Payload(t, lc.coeffs[lc.k]),
				MaxOther: mustFloat64Payload(t, lc.maxOther),
				Surplus:  mustFloat64Payload(t, lc.surplus),
				NewA:     &na,
				NewRHS:   &nr,
			})
		}

		// Randomized direct checks including exact_iff and invalid surplus >= |a| / surplus < maxAct - rhs.
		for i := range 60 {
			pos := i%2 == 0
			absA := float64(5+rng.IntN(20)) * 0.5
			maxOther := float64(rng.IntN(30)) * 0.5
			if pos {
				surplus := float64(1+rng.IntN(int(absA*2)-1)) * 0.5
				rhs := maxOther + absA - surplus
				if i%5 == 0 {
					// Negative control: surplus >= a.
					badSurp := absA + 0.5
					reqs = append(reqs, binStrengthenReq{
						RHS:      mustFloat64Payload(t, rhs),
						A:        mustFloat64Payload(t, absA),
						MaxOther: mustFloat64Payload(t, maxOther),
						Surplus:  mustFloat64Payload(t, badSurp),
					})
					continue
				}
				na := mustFloat64Payload(t, surplus)
				nr := mustFloat64Payload(t, rhs-(absA-surplus))
				reqs = append(reqs, binStrengthenReq{
					RHS:      mustFloat64Payload(t, rhs),
					A:        mustFloat64Payload(t, absA),
					MaxOther: mustFloat64Payload(t, maxOther),
					Surplus:  mustFloat64Payload(t, surplus),
					NewA:     &na,
					NewRHS:   &nr,
				})
			} else {
				surplus := float64(1+rng.IntN(int(absA*2)-1)) * 0.5
				rhs := maxOther - surplus
				if i%5 == 1 {
					// Negative control: surplus < maxOther - rhs.
					badSurp := surplus - 0.5
					if badSurp <= 0 {
						badSurp = 0
					}
					reqs = append(reqs, binStrengthenReq{
						RHS:      mustFloat64Payload(t, rhs),
						A:        mustFloat64Payload(t, -absA),
						MaxOther: mustFloat64Payload(t, maxOther),
						Surplus:  mustFloat64Payload(t, badSurp),
					})
					continue
				}
				na := mustFloat64Payload(t, -surplus)
				nr := mustFloat64Payload(t, rhs)
				reqs = append(reqs, binStrengthenReq{
					RHS:      mustFloat64Payload(t, rhs),
					A:        mustFloat64Payload(t, -absA),
					MaxOther: mustFloat64Payload(t, maxOther),
					Surplus:  mustFloat64Payload(t, surplus),
					NewA:     &na,
					NewRHS:   &nr,
				})
			}
		}

		// Negative controls: surplus >= a_k, surplus <= 0, surplus < maxOther + a_k - rhs, and too-small newRHS.
		negStart := len(reqs)
		negTests := []binStrengthenReq{
			{
				RHS:      int64ToPayload(4),
				A:        int64ToPayload(5),
				MaxOther: int64ToPayload(4),
				Surplus:  int64ToPayload(5), // surplus >= a_k
			},
			{
				RHS:      int64ToPayload(4),
				A:        int64ToPayload(5),
				MaxOther: int64ToPayload(2),
				Surplus:  int64ToPayload(2), // surplus < maxOther + a_k - rhs (= 3)
			},
			{
				RHS:      int64ToPayload(10),
				A:        int64ToPayload(5),
				MaxOther: int64ToPayload(3),
				Surplus:  int64ToPayload(0), // surplus <= 0
			},
			{
				RHS:      int64ToPayload(2),
				A:        int64ToPayload(-5),
				MaxOther: int64ToPayload(7),
				Surplus:  int64ToPayload(5), // surplus >= -a_k
			},
		}
		reqs = append(reqs, negTests...)
		badNewA := int64ToPayload(3)
		badNewRHS := int64ToPayload(1) // exact newRHS is 2
		reqs = append(reqs, binStrengthenReq{
			RHS:      int64ToPayload(4),
			A:        int64ToPayload(5),
			MaxOther: int64ToPayload(2),
			Surplus:  int64ToPayload(3),
			NewA:     &badNewA,
			NewRHS:   &badNewRHS,
		})

		var got []binStrengthenResp
		runRulesCheck(t, exe, "bin-strengthen", reqs, &got)
		if len(got) != len(reqs) {
			t.Fatalf("len(got)=%d, want %d", len(got), len(reqs))
		}
		for i := range liveCases {
			if !got[i].Applicable || !got[i].ProposedValid {
				t.Fatalf("live binary strengthening %d (%+v) rejected by Lean: %+v", i, liveCases[i], got[i])
			}
		}
		for i := range 60 {
			idx := len(liveCases) + i
			pos := i%2 == 0
			expectInvalid := (pos && i%5 == 0) || (!pos && i%5 == 1)
			if expectInvalid {
				if got[idx].ProposedValid {
					t.Fatalf("randomized negative control %d unexpectedly accepted: %+v", i, got[idx])
				}
			} else if !got[idx].Applicable || !got[idx].ProposedValid || !got[idx].ExactIff {
				t.Fatalf("randomized binary strengthening %d failed: %+v", i, got[idx])
			}
		}
		for i := negStart; i < len(got); i++ {
			if got[i].ProposedValid {
				t.Fatalf("negative control %d unexpectedly accepted by Lean: %+v", i-negStart, got[i])
			}
		}
	})

	t.Run("OrbitSymmetryVerification", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(0x2a05, 0x12345678))

		buildOrbitPayload := func(p *CompiledProblem, rows []presolveWorkRow, lb, ub []float64, gCount int, sc *orbitTrialScratch) orbitCheckReq {
			isInt := make([]bool, p.numVars)
			lbAny := make([]any, p.numVars)
			ubAny := make([]any, p.numVars)
			obj := make([]ratPayload, p.numVars)
			for j := range p.numVars {
				isInt[j] = p.varCat[j] != Continuous
				lbAny[j] = extFloat64ToAny(t, lb[j])
				ubAny[j] = extFloat64ToAny(t, ub[j])
				obj[j] = mustFloat64Payload(t, p.objMin[j])
			}
			rowReqs := make([]orbitRowReq, len(rows))
			for i := range rows {
				r := &rows[i]
				vals := make([]ratPayload, len(r.vals))
				for k, v := range r.vals {
					vals[k] = mustFloat64Payload(t, v)
				}
				rowReqs[i] = orbitRowReq{
					Active: r.active,
					Cols:   slices.Clone(r.cols),
					Vals:   vals,
					Sense:  senseToStr(r.sense),
					RHS:    mustFloat64Payload(t, r.rhs),
				}
			}
			return orbitCheckReq{
				NumVars:   p.numVars,
				IsInt:     isInt,
				LB:        lbAny,
				UB:        ubAny,
				Obj:       obj,
				Rows:      rowReqs,
				GCount:    gCount,
				Orbits:    slices.Clone(sc.orbits),
				RowTuples: slices.Clone(sc.rowTuples),
				ColOrbit:  slices.Clone(sc.colOrbit),
				ColGroup:  slices.Clone(sc.colGroup),
				RowState:  slices.Clone(sc.rowState),
			}
		}

		makeWorkRows := func(p *CompiledProblem) []presolveWorkRow {
			rows := make([]presolveWorkRow, p.numConstraints)
			for i := range p.numConstraints {
				s, e := p.rowPtr[i], p.rowPtr[i+1]
				rows[i] = presolveWorkRow{
					name:   p.rowNames[i],
					cols:   slices.Clone(p.colIdx[s:e]),
					vals:   slices.Clone(p.rowVal[s:e]),
					rhs:    p.rowRHS[i],
					sense:  p.rowSense[i],
					active: true,
				}
			}
			return rows
		}

		var reqs []orbitCheckReq
		var wantValid []bool
		var wantExact []bool
		var wantMono []bool

		// Generate 30 symmetric coloring/assignment/monotone-block instances across G=2..5.
		for idx := range 30 {
			gCount := 2 + (idx % 4)
			mod := NewModel("orb_"+strconv.Itoa(idx), Minimize)
			y := make([]Var, gCount)
			x := make([][]Var, 2)
			for g := range gCount {
				y[g] = mod.AddBinaryVar("y_" + strconv.Itoa(g))
			}
			for r := range 2 {
				x[r] = make([]Var, gCount)
				for g := range gCount {
					x[r][g] = mod.AddBinaryVar("x_" + strconv.Itoa(r) + "_" + strconv.Itoa(g))
				}
			}
			// Seed partitioning row and invariant row.
			mod.AddConstraint("seed_part", NewExpr(func() []Term {
				ts := make([]Term, gCount)
				for g := range gCount {
					ts[g] = x[0][g].T(1)
				}
				return ts
			}()...).Eq(1))
			mod.AddConstraint("part_1", NewExpr(func() []Term {
				ts := make([]Term, gCount)
				for g := range gCount {
					ts[g] = x[1][g].T(1)
				}
				return ts
			}()...).Eq(1))
			// Tuple constraints linking x[0][g], x[1][g], y[g].
			for g := range gCount {
				mod.AddConstraint("link0_"+strconv.Itoa(g), NewExpr(x[0][g].T(2), x[1][g].T(1), y[g].T(-3)).Le(0))
			}
			// Invariant row across y[g] with non-negative continuous slack variable z.
			z := mod.AddVar("z", 0, math.Inf(1), Continuous)
			invTerms := make([]Term, gCount+1)
			for g := range gCount {
				invTerms[g] = y[g].T(1)
			}
			invTerms[gCount] = z.T(-1)
			mod.AddConstraint("inv_sum", NewExpr(invTerms...).Ge(1))

			isExact := idx%2 == 0
			objTerms := make([]Term, 0, gCount+1)
			if isExact {
				c := float64(1 + rng.IntN(10))
				for g := range gCount {
					objTerms = append(objTerms, y[g].T(c))
				}
			} else {
				baseC := float64(1 + rng.IntN(5))
				for g := range gCount {
					objTerms = append(objTerms, y[g].T(baseC+float64(g)))
				}
			}
			objTerms = append(objTerms, z.T(0.5))
			mod.SetObjective(NewExpr(objTerms...))

			cp := mod.Compile()
			rows := makeWorkRows(cp)
			lb := slices.Clone(cp.varLB)
			ub := slices.Clone(cp.varUB)
			colPtr, colRow, colVal := buildPresolveCSC(cp.numVars, rows)
			rowHash := computeOrbitRowHashes(cp, rows, lb, ub, colPtr)
			sc := newOrbitTrialScratch(cp.numVars, len(rows))

			gOut, ok, to := proposeAndVerifyOrbits(cp, rows, lb, ub, colPtr, colRow, colVal, rowHash, 0, &sc, time.Time{}, nil)
			if to || !ok || gOut != gCount {
				t.Fatalf("case %d proposeAndVerifyOrbits failed: gOut=%d ok=%v to=%v", idx, gOut, ok, to)
			}
			if !verifyOrbitSymmetry(cp, rows, lb, ub, gOut, &sc) {
				t.Fatalf("case %d verifyOrbitSymmetry returned false on valid symmetric model", idx)
			}
			reqs = append(reqs, buildOrbitPayload(cp, rows, lb, ub, gOut, &sc))
			wantValid = append(wantValid, true)
			wantExact = append(wantExact, isExact)
			wantMono = append(wantMono, true)

			// Add negative controls perturbing one aspect of the verified symmetry.
			switch idx % 5 {
			case 0:
				// Perturb a tuple row coefficient.
				badRows := makeWorkRows(cp)
				badRows[2].vals[0] += 0.5
				if verifyOrbitSymmetry(cp, badRows, lb, ub, gOut, &sc) {
					t.Fatalf("case %d negative control (coeff perturb) accepted by Go", idx)
				}
				reqs = append(reqs, buildOrbitPayload(cp, badRows, lb, ub, gOut, &sc))
			case 1:
				// Perturb an orbit variable upper bound.
				badUB := slices.Clone(ub)
				badUB[sc.orbits[1]]++
				if verifyOrbitSymmetry(cp, rows, lb, badUB, gOut, &sc) {
					t.Fatalf("case %d negative control (ub perturb) accepted by Go", idx)
				}
				reqs = append(reqs, buildOrbitPayload(cp, rows, lb, badUB, gOut, &sc))
			case 2:
				// Perturb an orbit variable category from integer/binary to continuous.
				badCP := *cp
				badCat := append([]VarCategory(nil), cp.varCat...)
				badCat[sc.orbits[1]] = Continuous
				badCP.varCat = badCat
				if verifyOrbitSymmetry(&badCP, rows, lb, ub, gOut, &sc) {
					t.Fatalf("case %d negative control (varCat perturb) accepted by Go", idx)
				}
				reqs = append(reqs, buildOrbitPayload(&badCP, rows, lb, ub, gOut, &sc))
			case 3:
				// Perturb a tuple row RHS.
				badRows := makeWorkRows(cp)
				badRows[2].rhs++
				if verifyOrbitSymmetry(cp, badRows, lb, ub, gOut, &sc) {
					t.Fatalf("case %d negative control (rhs perturb) accepted by Go", idx)
				}
				reqs = append(reqs, buildOrbitPayload(cp, badRows, lb, ub, gOut, &sc))
			default:
				// Perturb an invariant row coefficient.
				badRows := makeWorkRows(cp)
				badRows[2+gCount].vals[0] += 2
				if verifyOrbitSymmetry(cp, badRows, lb, ub, gOut, &sc) {
					t.Fatalf("case %d negative control (invariant row coeff) accepted by Go", idx)
				}
				reqs = append(reqs, buildOrbitPayload(cp, badRows, lb, ub, gOut, &sc))
			}
			wantValid = append(wantValid, false)
			wantExact = append(wantExact, false)
			wantMono = append(wantMono, false)
		}

		var got []orbitCheckResp
		runRulesCheck(t, exe, "orbit-check", reqs, &got)
		if len(got) != len(reqs) {
			t.Fatalf("len(got)=%d, want %d", len(got), len(reqs))
		}
		for i := range reqs {
			if got[i].Valid != wantValid[i] || got[i].ExactObj != wantExact[i] || got[i].MonotoneNonNeg != wantMono[i] {
				t.Fatalf("orbit case %d mismatch: got=%+v want=(valid=%v exact=%v mono=%v)",
					i, got[i], wantValid[i], wantExact[i], wantMono[i])
			}
		}
	})
}
