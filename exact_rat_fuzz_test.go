package goblin_test

import (
	"bytes"
	"encoding/json/v2"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

func verifyParseExactRatRoundTrip(tb testing.TB, raw string) {
	tb.Helper()
	r, err := gb.ParseExactRat(raw)
	if err == nil {
		if r == nil || r.Denom().Sign() <= 0 {
			tb.Fatalf("ParseExactRat(%q) returned invalid rational %v", raw, r)
		}
		canon := r.RatString()
		r2, err2 := gb.ParseExactRat(canon)
		if err2 != nil {
			tb.Fatalf("ParseExactRat(RatString(%q)=%q) failed: %v", raw, canon, err2)
		}
		if r2.Cmp(r) != 0 {
			tb.Fatalf("ParseExactRat round-trip mismatch for %q: %s != %s", raw, r2.RatString(), r.RatString())
		}
	}
}

func checkExactLPFormalOracles(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	wit gb.CertifiedWitness,
) {
	tb.Helper()
	if !formalFuzzEnabled() || p.IsMIP() {
		return
	}
	switch res.Status {
	case gb.StatusOptimal:
		dualMin := make([]*big.Rat, len(wit.Duals))
		for i, d := range wit.Duals {
			if d == nil {
				tb.Fatalf("[%s] nil LP witness dual at %d", label, i)
			}
			dualMin[i] = new(big.Rat).Set(d)
			if p.Sense() == gb.Maximize && dualMin[i].Sign() != 0 {
				dualMin[i].Neg(dualMin[i])
			}
		}
		raw, ok := encodeOptimalLPCheckJSON(p, wit.Values, dualMin)
		if !ok {
			tb.Fatalf("[%s] encodeOptimalLPCheckJSON failed on certified LP Optimal", label)
		}
		if _, err := checkFormalLPJSONLForProblem(tb, p, raw); err != nil {
			tb.Fatalf("[%s] exact LP optimal failed Lean lp-check: %v", label, err)
		}
	case gb.StatusInfeasible:
		if p.NumConstraints() > 0 && len(wit.Farkas) == p.NumConstraints() {
			raw, ok := encodeFarkasLPCheckJSON(p, wit.Farkas)
			if !ok {
				tb.Fatalf("[%s] encodeFarkasLPCheckJSON failed on certified LP Farkas", label)
			}
			if _, err := checkFormalLPJSONLForProblem(tb, p, raw); err != nil {
				tb.Fatalf("[%s] exact LP farkas failed Lean lp-check: %v", label, err)
			}
		}
	case gb.StatusUnbounded:
		raw, ok := encodeUnboundedLPCheckJSON(p, wit.Unbounded)
		if !ok {
			tb.Fatalf("[%s] encodeUnboundedLPCheckJSON failed on certified LP Unbounded", label)
		}
		if _, err := checkFormalLPJSONLForProblem(tb, p, raw); err != nil {
			tb.Fatalf("[%s] exact LP unbounded failed Lean lp-check: %v", label, err)
		}
	}
}

func verifyExactCertifiedSolve(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	presolve bool,
	cuts bool,
	strictFloatCheck bool,
) {
	tb.Helper()
	tr := gb.NewVIPRTrace()
	var wit gb.CertifiedWitness
	opts := []gb.Option{
		gb.WithPureGo(),
		gb.WithCertified(true),
		gb.WithCertifiedWitness(&wit),
		gb.WithVIPRTrace(tr),
		gb.WithPresolve(presolve),
		gb.WithCuts(cuts),
		gb.WithRelGapTol(0),
		gb.WithAbsGapTol(0),
		gb.WithMaxNodes(200),
		gb.WithMaxIter(4000),
		gb.WithTimeLimit(2 * time.Second),
	}
	applied := gb.ApplyOptions(opts...)
	res, err := gb.SolveCompiled(tb.Context(), p, opts...)
	if err != nil {
		assertFailClosedResultAndWitness(tb, label+"/fail_closed", res, err, wit)
	} else {
		assertAllowedStatus(tb, label, p, true, res)
		verifyWitnessRationalInvariants(tb, label, p, res, wit, 0, 0, true)

		if tr.HasCertificate() {
			startV := time.Now()
			if errV := gb.VerifyVIPRTrace(p, tr, res, applied); errV != nil {
				tb.Fatalf("[%s] VerifyVIPRTrace failed: %v", label, errV)
			}
			assertVIPRComplexityBudget(tb, label, p, tr, time.Since(startV))
			if formalFuzzEnabled() {
				if strictFloatCheck {
					if errF := checkFormalVIPRResult(tb, label, p, res, tr, 0, 0); errF != nil {
						tb.Fatal(errF)
					}
				} else {
					rawVIPR, errM := tr.MarshalVIPRJSON()
					if errM != nil {
						tb.Fatalf("[%s] MarshalVIPRJSON failed: %v", label, errM)
					}
					if errMatch := verifySerializedVIPRMatchesProblem(p, rawVIPR); errMatch != nil {
						tb.Fatalf("[%s] serialized VIPR mismatch: %v", label, errMatch)
					}
					if _, errLean := checkFormalVIPRJSONL(tb, rawVIPR); errLean != nil {
						tb.Fatalf("[%s] Lean vipr-check failed: %v", label, errLean)
					}
				}
			}
		} else if formalFuzzEnabled() && p.IsMIP() && (res.Status == gb.StatusOptimal || res.Status == gb.StatusInfeasible) {
			tb.Fatalf("[%s] certified MIP status=%s missing VIPRTrace certificate", label, res.Status)
		}
		checkExactLPFormalOracles(tb, label, p, res, wit)
	}
}

func verifyExactPresolveForVIPR(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	maxPasses int,
	strictFloatCheck bool,
) {
	tb.Helper()
	pre := gb.PresolveForVIPR(p, maxPasses)
	if pre.TimedOut {
		return
	}
	applied := gb.ApplyOptions(gb.WithPureGo(), gb.WithCertified(true), gb.WithRelGapTol(0), gb.WithAbsGapTol(0))
	if pre.Infeasible {
		// Confirm the unpresolved exact problem also rejects feasibility (or fails closed).
		resOrig, errOrig := gb.SolveCompiled(tb.Context(), p,
			gb.WithPureGo(), gb.WithCertified(true), gb.WithPresolve(false),
			gb.WithMaxNodes(200), gb.WithMaxIter(4000), gb.WithTimeLimit(2*time.Second))
		if errOrig == nil && resOrig.Status == gb.StatusOptimal {
			tb.Fatalf("[%s] PresolveForVIPR reported Infeasible while certified solve found Optimal=%g", label, resOrig.Objective)
		}
		return
	}
	if pre.Reduced == nil {
		tb.Fatalf("[%s] PresolveForVIPR returned nil Reduced without Infeasible/TimedOut", label)
	}
	trPre := gb.NewVIPRTrace()
	var witPre gb.CertifiedWitness
	resPre, errPre := gb.SolveCompiled(tb.Context(), pre.Reduced,
		gb.WithPureGo(),
		gb.WithCertified(true),
		gb.WithCertifiedWitness(&witPre),
		gb.WithVIPRTrace(trPre),
		gb.WithPresolve(false),
		gb.WithRelGapTol(0),
		gb.WithAbsGapTol(0),
		gb.WithMaxNodes(200),
		gb.WithMaxIter(4000),
		gb.WithTimeLimit(2*time.Second),
	)
	if errPre != nil {
		assertFailClosedResultAndWitness(tb, label+"/pre_fail_closed", resPre, errPre, witPre)
	} else if trPre.HasCertificate() {
		trPre.SetPresolveEnvelope(pre.Envelope)
		resCheck := resPre
		if !p.IsMIP() && len(resCheck.Duals) != p.NumConstraints() {
			// PresolveForVIPR drops redundant rows without postsolving LP duals to orig coordinates;
			// clear reduced-row duals before verifying the original-problem VIPR trace.
			resCheck.Duals = nil
		}
		if errV := gb.VerifyVIPRTrace(p, trPre, resCheck, applied); errV != nil {
			tb.Fatalf("[%s] VerifyVIPRTrace(PresolveForVIPR) failed (mip=%v origRows=%d redRows=%d status=%s): %v",
				label, p.IsMIP(), p.NumConstraints(), pre.Reduced.NumConstraints(), resPre.Status, errV)
		}
		if formalFuzzEnabled() {
			rawVIPR, errM := trPre.MarshalVIPRJSON()
			if errM != nil {
				tb.Fatalf("[%s] MarshalVIPRJSON(PresolveForVIPR) failed: %v", label, errM)
			}
			if errMatch := verifySerializedVIPRMatchesProblem(p, rawVIPR); errMatch != nil {
				tb.Fatalf("[%s] PresolveForVIPR serialized VIPR mismatch: %v", label, errMatch)
			}
			if _, errLean := checkFormalVIPRJSONL(tb, rawVIPR); errLean != nil {
				tb.Fatalf("[%s] PresolveForVIPR Lean vipr-check failed: %v", label, errLean)
			}
			if strictFloatCheck && (resPre.Status == gb.StatusOptimal || resPre.Status == gb.StatusInfeasible) {
				if errF := checkFormalVIPRResult(tb, label+"/pre_orig", p, resCheck, trPre, 0, 0); errF != nil {
					tb.Fatal(errF)
				}
			}
		}
	}
}

func exactRatFromChoices(cur *certFuzzCursor) (string, *big.Rat) {
	choices := []string{
		"0", "1", "-1", "2", "-3", "0.1", "0.2", "0.3", "0.9", "-0.7",
		"1.5", "-7.2", "-4.1", "7.2", "14.4", "1/3", "-2/7", "5/2", "1e-6", "1e-20",
	}
	s := choices[int(cur.u8())%len(choices)]
	r, err := gb.ParseExactRat(s)
	if err != nil {
		return "1", big.NewRat(1, 1)
	}
	return s, r
}

func buildExactPuLPJSONPayload(cur *certFuzzCursor, escapeVarName bool) []byte {
	sense := 1
	if cur.u8()%2 == 1 {
		sense = -1
	}
	n := cur.intRange(2, 4)
	mRows := cur.intRange(1, 3)

	var buf bytes.Buffer
	buf.WriteString(`{"parameters":{"name":"exact_pulp","sense":`)
	buf.WriteString(strconv.Itoa(sense))
	buf.WriteString(`,"status":0,"sol_status":0},"variables":[`)

	varNames := make([]string, n)
	varJSONNames := make([]string, n)
	cats := []string{"Continuous", "Integer", "Binary"}
	decChoices := []string{"0", "0.1", "0.2", "0.3", "0.9", "1", "2.5", "-1.2", "-4.1", "-7.2", "3", "5", "1e-3"}
	for j := range n {
		if j > 0 {
			buf.WriteByte(',')
		}
		name := "v" + strconv.Itoa(j)
		varNames[j] = name
		if escapeVarName && j == 0 {
			varJSONNames[j] = `\u00760`
		} else {
			varJSONNames[j] = name
		}
		cat := cats[int(cur.u8())%len(cats)]
		buf.WriteString(`{"name":"`)
		buf.WriteString(varJSONNames[j])
		buf.WriteString(`","cat":"`)
		buf.WriteString(cat)
		buf.WriteByte('"')
		switch cur.u8() % 4 {
		case 0:
			// Omitted lowBound defaults to 0.
			buf.WriteString(`,"upBound":`)
			if cat == "Binary" {
				buf.WriteString("1")
			} else {
				buf.WriteString(decChoices[int(cur.u8())%6+4])
			}
		case 1:
			buf.WriteString(`,"lowBound":null,"upBound":`)
			if cat == "Binary" {
				buf.WriteString("null")
			} else {
				buf.WriteString("5.5")
			}
		case 2:
			if cat == "Binary" {
				buf.WriteString(`,"lowBound":0,"upBound":1`)
			} else {
				buf.WriteString(`,"lowBound":0.1,"upBound":0.9`)
			}
		default:
			if cat == "Binary" {
				buf.WriteString(`,"lowBound":0,"upBound":1`)
			} else {
				buf.WriteString(`,"lowBound":0,"upBound":null`)
			}
		}
		buf.WriteByte('}')
	}

	buf.WriteString(`],"objective":{"name":"OBJ","coefficients":[`)
	for j := range n {
		if j > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"name":"`)
		buf.WriteString(varJSONNames[j])
		buf.WriteString(`","value":`)
		buf.WriteString(decChoices[int(cur.u8())%len(decChoices)])
		buf.WriteByte('}')
	}
	buf.WriteString(`],"constant":`)
	buf.WriteString(decChoices[int(cur.u8())%len(decChoices)])
	buf.WriteString(`},"constraints":[`)

	senses := []int{-1, 0, 1}
	for i := range mRows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"name":"c`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`","sense":`)
		buf.WriteString(strconv.Itoa(senses[int(cur.u8())%len(senses)]))
		buf.WriteString(`,"coefficients":[`)
		for j := range n {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(`{"name":"`)
			buf.WriteString(varJSONNames[j])
			buf.WriteString(`","value":`)
			buf.WriteString(decChoices[int(cur.u8())%len(decChoices)])
			buf.WriteByte('}')
		}
		buf.WriteString(`],"constant":`)
		buf.WriteString(decChoices[int(cur.u8())%len(decChoices)])
		buf.WriteByte('}')
	}
	buf.WriteString(`],"sos1":[],"sos2":[]}`)
	return buf.Bytes()
}

func assertExactCompiledParity(tb testing.TB, pFast, pStd *gb.CompiledProblem) {
	tb.Helper()
	ocF, objF, rhsF, colsF, valsF, _, okF := gb.ExactProblemRatsForTest(pFast)
	ocS, objS, rhsS, colsS, valsS, _, okS := gb.ExactProblemRatsForTest(pStd)
	lbF, ubF, okBF := gb.ExactVarBoundsRatsForTest(pFast)
	lbS, ubS, okBS := gb.ExactVarBoundsRatsForTest(pStd)
	if !okF || !okS || !okBF || !okBS {
		tb.Fatalf("ExactProblemRatsForTest parity check failed: okF=%v okS=%v okBF=%v okBS=%v", okF, okS, okBF, okBS)
	}
	if ocF.Cmp(ocS) != 0 || len(objF) != len(objS) || len(rhsF) != len(rhsS) {
		tb.Fatalf("fast vs std PuLP exact metadata mismatch: oc=%s/%s n=%d/%d m=%d/%d",
			ocF.RatString(), ocS.RatString(), len(objF), len(objS), len(rhsF), len(rhsS))
	}
	for j := range objF {
		if objF[j].Cmp(objS[j]) != 0 {
			tb.Fatalf("fast vs std exact obj[%d]: %s != %s", j, objF[j].RatString(), objS[j].RatString())
		}
		if (lbF[j] == nil) != (lbS[j] == nil) || (lbF[j] != nil && lbF[j].Cmp(lbS[j]) != 0) {
			tb.Fatalf("fast vs std exact lb[%d]: %v != %v", j, lbF[j], lbS[j])
		}
		if (ubF[j] == nil) != (ubS[j] == nil) || (ubF[j] != nil && ubF[j].Cmp(ubS[j]) != 0) {
			tb.Fatalf("fast vs std exact ub[%d]: %v != %v", j, ubF[j], ubS[j])
		}
	}
	for i := range rhsF {
		if rhsF[i].Cmp(rhsS[i]) != 0 || len(colsF[i]) != len(colsS[i]) {
			tb.Fatalf("fast vs std exact row[%d] mismatch", i)
		}
		for k := range colsF[i] {
			if colsF[i][k] != colsS[i][k] || valsF[i][k].Cmp(valsS[i][k]) != 0 {
				tb.Fatalf("fast vs std exact row[%d][%d] mismatch", i, k)
			}
		}
	}
}

// FuzzExactRatAndPuLPJSONVIPR exercises ParseExactRat, exact model builders (TRat, TDec,
// LeRat, GeRat, EqRat, AddVarRat, AddIntVarRat), >2^53 outward integer tightening,
// mixed rhsLo constraints, FromPuLPJSONExactBytes fast-vs-std parity, and PresolveForVIPR
// under WithCertified(true) and GOBLIN_FORMAL=1 Lean vipr-check and lp-check verification.
func FuzzExactRatAndPuLPJSONVIPR(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		{1, 0, 5, 4, 3, 2, 1, 0, 7, 6, 5, 4, 3, 2, 1, 0},
		{2, 3, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59},
		{3, 2, 1, 0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48},
		{4, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25, 27, 29},
		[]byte("0.9\x00-7.2\x001.5e-3\x009/10\x001e10001"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// 1. Direct ParseExactRat round-trip and malformed token fuzzing.
		if len(data) <= 256 {
			verifyParseExactRatRoundTrip(t, string(data))
		} else {
			verifyParseExactRatRoundTrip(t, string(data[:256]))
		}
		for part := range strings.SplitSeq(string(data[:min(len(data), 128)]), "\x00") {
			verifyParseExactRatRoundTrip(t, part)
		}

		cur := newCertFuzzCursor(data)
		regime := cur.u8() % 5
		presolveFlag := cur.u8()%2 == 0
		cutsFlag := cur.u8()%2 == 0

		switch regime {
		case 0:
			// Regime A: Non-dyadic Q_10 and Q_p LP/MIP via TRat, TDec, AddVarRat, AddIntVarRat, LeRat/GeRat/EqRat.
			sense := gb.Minimize
			if cur.u8()%2 == 1 {
				sense = gb.Maximize
			}
			m := gb.NewModel("exact_q_model", sense)
			isMIP := cur.u8()%2 == 0
			n := cur.intRange(2, 4)
			xs := make([]gb.Var, n)
			for j := range n {
				var lbRat, ubRat *big.Rat
				switch cur.u8() % 5 {
				case 0:
					lbRat = big.NewRat(0, 1)
					ubRat = big.NewRat(5, 1)
				case 1:
					lbRat, _ = gb.ParseExactRat("0.1")
					ubRat, _ = gb.ParseExactRat("0.9")
				case 2:
					lbRat, _ = gb.ParseExactRat("1e-20")
					ubRat = new(big.Rat).Sub(big.NewRat(1+int64(cur.u8()%3), 1), lbRat)
				case 3:
					lbRat = big.NewRat(0, 1)
					ubRat = nil
				default:
					lbRat = big.NewRat(-1, 3)
					ubRat = big.NewRat(7, 3)
				}
				name := "x" + strconv.Itoa(j)
				if isMIP && (j < n-1 || cur.u8()%2 == 0) {
					if cur.u8()%2 == 0 {
						xs[j] = m.AddIntVarRat(name, lbRat, ubRat)
					} else {
						xs[j] = m.AddVarRat(name, lbRat, ubRat, gb.Integer)
					}
				} else {
					xs[j] = m.AddContVarRat(name, lbRat, ubRat)
				}
				// Mutate caller rationals after builder call to confirm defensive cloning.
				if lbRat != nil {
					lbRat.SetInt64(9999)
				}
				if ubRat != nil {
					ubRat.SetInt64(-9999)
				}
			}

			mRows := cur.intRange(1, 3)
			for i := range mRows {
				expr := gb.NewExpr()
				for j := range n {
					decStr, ratVal := exactRatFromChoices(cur)
					if cur.u8()%2 == 0 && !strings.Contains(decStr, "/") {
						expr = gb.Sum(expr, gb.NewExpr(xs[j].TDec(decStr)))
					} else {
						expr = expr.AddTermRat(xs[j], ratVal)
						ratVal.SetInt64(777)
					}
				}
				// Inject occasional duplicate column term (e.g. 0.1*x0 + 0.2*x0).
				if cur.u8()%2 == 0 {
					expr = gb.Sum(expr, gb.NewExpr(xs[0].TDec("0.1"), xs[0].TDec("0.2")))
				}
				if cur.u8()%2 == 0 {
					_, cRat := exactRatFromChoices(cur)
					expr = expr.AddConstantRat(cRat)
				}
				_, rhsRat := exactRatFromChoices(cur)
				cName := "c" + strconv.Itoa(i)
				switch cur.u8() % 3 {
				case 0:
					m.AddConstraint(cName, gb.LeRat(expr, rhsRat))
				case 1:
					m.AddConstraint(cName, gb.GeRat(expr, rhsRat))
				default:
					m.AddConstraint(cName, gb.EqRat(expr, rhsRat))
				}
				rhsRat.SetInt64(-555)
			}

			objExpr := gb.NewExpr()
			for j := range n {
				decStr, ratVal := exactRatFromChoices(cur)
				if cur.u8()%2 == 0 && !strings.Contains(decStr, "/") {
					objExpr = gb.Sum(objExpr, gb.NewExpr(xs[j].TDec(decStr)))
				} else {
					objExpr = objExpr.AddTermRat(xs[j], ratVal)
				}
			}
			if cur.u8()%2 == 0 {
				_, ocRat := exactRatFromChoices(cur)
				objExpr = objExpr.AddConstantRat(ocRat)
			}
			m.SetObjective(objExpr)

			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("Regime A CompileChecked failed: %v", err)
			}
			verifyExactCertifiedSolve(t, "exact_q", p, presolveFlag, cutsFlag, true)
			if cur.u8()%2 == 0 {
				verifyExactPresolveForVIPR(t, "exact_q_pre", p, cur.intRange(1, 6), true)
			}

		case 1:
			// Regime B: >2^53 integer outward tightening (LeRat/GeRat/EqRat) and PresolveForVIPR.
			two53 := new(big.Int).Lsh(big.NewInt(1), 53)
			sense := gb.Maximize
			if cur.u8()%2 == 1 {
				sense = gb.Minimize
			}
			m := gb.NewModel("exact_big53", sense)
			x1Max := new(big.Int).Set(two53)
			strictFloat := true
			if cur.u8()%3 == 0 {
				// Bound strictly above 2^53 (non-representable odd integer in binary64).
				x1Max.Add(x1Max, big.NewInt(int64(cur.intRange(1, 3))))
				strictFloat = false
			}
			x1 := m.AddIntVarRat("x1", big.NewRat(0, 1), new(big.Rat).SetInt(x1Max))
			x2 := m.AddIntVarRat("x2", big.NewRat(0, 1), big.NewRat(int64(cur.intRange(1, 3)), 1))
			x3 := m.AddIntVarRat("x3", big.NewRat(0, 1), big.NewRat(int64(cur.intRange(1, 2)), 1))

			fracChoices := []*big.Rat{
				big.NewRat(1, 2),
				big.NewRat(1, 1),
				big.NewRat(3, 2),
				big.NewRat(5, 2),
				big.NewRat(5, 3),
				big.NewRat(2, 1),
			}
			delta := fracChoices[int(cur.u8())%len(fracChoices)]
			rhs := new(big.Rat).Add(new(big.Rat).SetInt(two53), delta)
			c1 := big.NewRat(1, 1)
			c2 := big.NewRat(int64(cur.intRange(1, 2)), 1)
			expr := gb.NewExpr(gb.TRat(x1, c1), gb.TRat(x2, c2))
			if cur.u8()%2 == 0 {
				expr = expr.AddTermRat(x3, big.NewRat(1, 1))
			}
			switch cur.u8() % 3 {
			case 0:
				m.AddConstraint("big_c", expr.LeRat(rhs))
			case 1:
				m.AddConstraint("big_c", expr.GeRat(rhs))
			default:
				m.AddConstraint("big_c", expr.EqRat(rhs))
			}
			m.SetObjective(gb.NewExpr(x1.T(1), x2.T(float64(cur.intRange(1, 3))), x3.T(float64(cur.intRange(-2, 2)))))

			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("Regime B CompileChecked failed: %v", err)
			}
			verifyExactCertifiedSolve(t, "exact_big53", p, presolveFlag, cutsFlag, strictFloat)
			verifyExactPresolveForVIPR(t, "exact_big53_pre", p, cur.intRange(1, 6), strictFloat)

		case 2:
			// Regime C: Mixed TDec/TRat + binary64 AddConstant/Scale/Le/Ge/Eq preserving rhsLo.
			sense := gb.Minimize
			if cur.u8()%2 == 1 {
				sense = gb.Maximize
			}
			m := gb.NewModel("exact_mixed_rhslo", sense)
			x := m.AddIntVar("x", 0, 10)
			y := m.AddContVarRat("y", big.NewRat(0, 1), big.NewRat(10, 1))
			floatConsts := []float64{0.1, 0.2, 0.3, -0.2, 1.2, 2.3, 0.7}
			floatRHS := []float64{1.2, 0.3, 1.5, 2.3, 3.1, -0.1}
			decStr, _ := exactRatFromChoices(cur)
			if strings.Contains(decStr, "/") {
				decStr = "1"
			}
			expr := gb.NewExpr(x.TDec(decStr)).AddTerm(y, cur.pickFloat([]float64{0, 0.1, 0.5, 1.0, -0.3}))
			expr = expr.AddConstant(cur.pickFloat(floatConsts))
			if cur.u8()%2 == 0 {
				expr = expr.Scale(cur.pickFloat([]float64{0.5, 2.0, -1.0, 3.0}))
			}
			rhs := cur.pickFloat(floatRHS)
			switch cur.u8() % 3 {
			case 0:
				m.AddConstraint("mix_c", expr.Eq(rhs))
			case 1:
				m.AddConstraint("mix_c", expr.Le(rhs))
			default:
				m.AddConstraint("mix_c", expr.Ge(rhs))
			}
			m.SetObjective(gb.NewExpr(x.TDec("1"), y.TDec("0.3")).AddConstant(cur.pickFloat(floatConsts)))
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("Regime C CompileChecked failed: %v", err)
			}
			verifyExactCertifiedSolve(t, "exact_mixed_rhslo", p, presolveFlag, cutsFlag, true)
			if cur.u8()%2 == 0 {
				verifyExactPresolveForVIPR(t, "exact_mixed_rhslo_pre", p, cur.intRange(1, 4), true)
			}

		case 3:
			// Regime D: FromPuLPJSONExactBytes fast-vs-std parity + certified VIPR + raw JSON fuzzing.
			curFast := newCertFuzzCursor(data)
			_ = curFast.u8()
			_ = curFast.u8()
			_ = curFast.u8()
			rawFast := buildExactPuLPJSONPayload(curFast, false)
			curStd := newCertFuzzCursor(data)
			_ = curStd.u8()
			_ = curStd.u8()
			_ = curStd.u8()
			rawStd := buildExactPuLPJSONPayload(curStd, true)

			mFast, errFast := format.FromPuLPJSONExactBytes(rawFast)
			mStd, errStd := format.FromPuLPJSONExactBytes(rawStd)
			if (errFast == nil) != (errStd == nil) {
				t.Fatalf("FromPuLPJSONExactBytes fast vs std error parity mismatch: fast=%v std=%v\nfast=%s\nstd=%s",
					errFast, errStd, rawFast, rawStd)
			}
			if errFast == nil {
				pFast, errCF := mFast.CompileChecked()
				pStd, errCS := mStd.CompileChecked()
				if (errCF == nil) != (errCS == nil) {
					t.Fatalf("CompileChecked fast vs std mismatch: fast=%v std=%v", errCF, errCS)
				}
				if errCF == nil {
					assertExactCompiledParity(t, pFast, pStd)
					verifyExactCertifiedSolve(t, "pulp_exact", pFast, presolveFlag, cutsFlag, true)
					if cur.u8()%2 == 0 {
						verifyExactPresolveForVIPR(t, "pulp_exact_pre", pFast, cur.intRange(1, 4), true)
					}
				}
			}

			// Feed raw/mutated bytes to FromPuLPJSONExactBytes to verify zero panics.
			rawMut := data
			if len(rawMut) > 4096 {
				rawMut = rawMut[:4096]
			}
			if mRaw, errRaw := format.FromPuLPJSONExactBytes(rawMut); errRaw == nil && mRaw != nil {
				_, _ = mRaw.CompileChecked()
			}
			var probe map[string]any
			_ = json.Unmarshal(rawFast, &probe)

		default:
			// Regime E: Deferred validation errors (invalid TDec, nil *big.Rat, overflowing 1e1000) never panic.
			mBad := gb.NewModel("exact_deferred_err", gb.Minimize)
			xb := mBad.AddContVar("xb", 0, 1)
			switch cur.u8() % 4 {
			case 0:
				mBad.AddConstraint("bad_dec", gb.Eq(gb.NewExpr(xb.TDec(" "+string(data[:min(len(data), 16)]))), 1))
			case 1:
				mBad.SetObjective(gb.NewExpr(gb.TRat(xb, nil)))
			case 2:
				mBad.AddConstraint("nil_rhs", gb.LeRat(gb.VarExpr(xb), nil))
			default:
				huge, errH := gb.ParseExactRat("1e1000")
				if errH != nil {
					t.Fatalf("ParseExactRat(1e1000): %v", errH)
				}
				mBad.AddContVarRat("xh", nil, huge)
			}
			if errV := mBad.Validate(); errV == nil {
				t.Fatalf("expected deferred Validate error on malformed exact model")
			}
			if _, errC := mBad.CompileChecked(); errC == nil {
				t.Fatalf("expected deferred CompileChecked error on malformed exact model")
			}
		}
	})
}
