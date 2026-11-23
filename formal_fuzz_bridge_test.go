package goblin_test

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	gb "github.com/paulourio/goblin"
)

type lpBridgeTermJSON struct {
	Col   int         `json:"col"`
	Coeff fuzzRatJSON `json:"coeff"`
}

type lpBridgeRowJSON struct {
	Sense string             `json:"sense"`
	RHS   fuzzRatJSON        `json:"rhs"`
	Terms []lpBridgeTermJSON `json:"terms"`
}

type lpFarkasReqJSON struct {
	Mode     string            `json:"mode"`
	NumVars  int               `json:"numVars"`
	NumRows  int               `json:"numRows"`
	ObjConst fuzzRatJSON       `json:"objConst"`
	Obj      []fuzzRatJSON     `json:"obj"`
	LB       []*fuzzRatJSON    `json:"lb"`
	UB       []*fuzzRatJSON    `json:"ub"`
	Rows     []lpBridgeRowJSON `json:"rows"`
	W        []fuzzRatJSON     `json:"w"`
}

type lpUnboundedReqJSON struct {
	Mode     string            `json:"mode"`
	NumVars  int               `json:"numVars"`
	NumRows  int               `json:"numRows"`
	ObjConst fuzzRatJSON       `json:"objConst"`
	Obj      []fuzzRatJSON     `json:"obj"`
	LB       []*fuzzRatJSON    `json:"lb"`
	UB       []*fuzzRatJSON    `json:"ub"`
	Rows     []lpBridgeRowJSON `json:"rows"`
	X0       []fuzzRatJSON     `json:"x0"`
	D        []fuzzRatJSON     `json:"d"`
}

func formalFuzzEnabled() bool {
	return gb.FormalOracleEnabled()
}

func callFormalOracleJSONL(tb testing.TB, exeName string, payload []byte) (gb.FormalOracleResp, error) {
	tb.Helper()
	return gb.CallFormalOracleJSONL(tb, exeName, payload)
}

func callFormalOracleSubcmdJSONL(tb testing.TB, exeName, subcmd string, payload []byte) (gb.FormalOracleResp, error) {
	tb.Helper()
	return gb.CallFormalOracleSubcmdJSONL(tb, exeName, subcmd, payload)
}

func formalOracleCall(tb testing.TB, exeName string, reqJSON []byte) (ok bool, boundRat [2]string, detailOrErr string) {
	tb.Helper()
	return gb.FormalOracleCall(tb, exeName, reqJSON)
}

func checkFormalLPJSONL(tb testing.TB, reqJSON []byte) (*big.Rat, error) {
	tb.Helper()
	resp, err := gb.FormalCheckLPJSONL(tb, reqJSON)
	if err != nil {
		return nil, fmt.Errorf("lp-check --jsonl transport error: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("lp-check --jsonl rejected certificate: %s", resp.Error)
	}
	if resp.Bound == nil {
		return nil, nil
	}
	b, bErr := resp.Bound.BigRat()
	if bErr != nil {
		return nil, fmt.Errorf("lp-check --jsonl invalid bound: %w", bErr)
	}
	return b, nil
}

// checkFormalLPJSONLForProblem verifies independently that the problem encoded
// inside reqJSON matches p before accepting an lp-check verdict on reqJSON.
func checkFormalLPJSONLForProblem(tb testing.TB, p *gb.CompiledProblem, reqJSON []byte) (*big.Rat, error) {
	tb.Helper()
	if err := verifySerializedLPPayloadMatchesProblem(p, reqJSON); err != nil {
		return nil, err
	}
	return checkFormalLPJSONL(tb, reqJSON)
}

func mustFormalLPCheck(tb testing.TB, reqJSON []byte) *big.Rat {
	tb.Helper()
	b, err := checkFormalLPJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

func mustFormalCutsCheck(tb testing.TB, reqJSON []byte) {
	tb.Helper()
	resp, err := gb.FormalCheckCutsJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("cuts-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("cuts-check --jsonl rejected cut: %s", resp.Error)
	}
}

func mustFormalRulesCheck(tb testing.TB, reqJSON []byte) jsontext.Value {
	tb.Helper()
	resp, err := gb.FormalCheckRulesJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("rules-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("rules-check --jsonl rejected presolve rule: %s", resp.Error)
	}
	return resp.Result
}

func checkFormalVIPRJSONL(tb testing.TB, reqJSON []byte) (*big.Rat, error) {
	tb.Helper()
	resp, err := gb.FormalCheckVIPRJSONL(tb, reqJSON)
	if err != nil {
		return nil, fmt.Errorf("vipr-check --jsonl transport error: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("vipr-check --jsonl rejected certificate: %s", resp.Error)
	}
	if resp.Bound == nil {
		return nil, nil
	}
	b, bErr := resp.Bound.BigRat()
	if bErr != nil {
		return nil, fmt.Errorf("vipr-check --jsonl invalid bound: %w", bErr)
	}
	return b, nil
}

func mustFormalVIPRCheck(tb testing.TB, reqJSON []byte) *big.Rat {
	tb.Helper()
	b, err := checkFormalVIPRJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

func isResourceLimitStatus(st gb.Status) bool {
	return st == gb.StatusTimeLimit || st == gb.StatusNodeLimit || st == gb.StatusIterationLimit
}

func isTerminalFailureStatus(st gb.Status) bool {
	switch st {
	case gb.StatusOptimal, gb.StatusInfeasible, gb.StatusUnbounded,
		gb.StatusTimeLimit, gb.StatusNodeLimit, gb.StatusIterationLimit:
		return false
	default:
		return true
	}
}

func fuzzRatFromFloat(v float64) (fuzzRatJSON, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fuzzRatJSON{}, false
	}
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		return fuzzRatJSON{}, false
	}
	return fuzzRatJSON{Num: r.Num().String(), Den: r.Denom().String()}, true
}

func fuzzRatFromBigRat(r *big.Rat) fuzzRatJSON {
	if r == nil {
		return fuzzRatJSON{Num: "0", Den: "1"}
	}
	return fuzzRatJSON{Num: r.Num().String(), Den: r.Denom().String()}
}

func constraintSenseToken(s gb.ConstraintSense) (string, bool) {
	switch s {
	case gb.LessEqual:
		return "le", true
	case gb.GreaterEqual:
		return "ge", true
	case gb.Equal:
		return "eq", true
	default:
		return "", false
	}
}

func encodeProblemHeaderForLPCheck(p *gb.CompiledProblem) (
	objConst fuzzRatJSON,
	obj []fuzzRatJSON,
	lb []*fuzzRatJSON,
	ub []*fuzzRatJSON,
	rows []lpBridgeRowJSON,
	err error,
) {
	if p == nil {
		return fuzzRatJSON{}, nil, nil, nil, nil, errors.New("nil CompiledProblem")
	}
	constMin := p.ObjConstant()
	if p.Sense() == gb.Maximize && constMin != 0 {
		constMin = -constMin
	}
	oc, ok := fuzzRatFromFloat(constMin)
	if !ok {
		return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("non-finite objective constant %v", constMin)
	}
	n := p.NumVars()
	m := p.NumConstraints()
	obj = make([]fuzzRatJSON, n)
	lb = make([]*fuzzRatJSON, n)
	ub = make([]*fuzzRatJSON, n)
	for j := range n {
		cj, okC := fuzzRatFromFloat(p.ObjMinCoeff(j))
		if !okC {
			return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("non-finite obj coeff at col %d", j)
		}
		obj[j] = cj
		lo, hi := p.VarBounds(j)
		if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 1) || math.IsInf(hi, -1) {
			return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("invalid bounds [%v, %v] at col %d", lo, hi, j)
		}
		if !math.IsInf(lo, -1) {
			lr, okL := fuzzRatFromFloat(lo)
			if !okL {
				return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("invalid lower bound %v at col %d", lo, j)
			}
			lb[j] = &lr
		}
		if !math.IsInf(hi, 1) {
			ur, okU := fuzzRatFromFloat(hi)
			if !okU {
				return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("invalid upper bound %v at col %d", hi, j)
			}
			ub[j] = &ur
		}
	}
	rows = make([]lpBridgeRowJSON, m)
	for i := range m {
		sTok, okS := constraintSenseToken(p.RowSense(i))
		if !okS {
			return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("invalid row sense at row %d", i)
		}
		rhs, okR := fuzzRatFromFloat(p.RowRHS(i))
		if !okR {
			return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("non-finite RHS at row %d", i)
		}
		terms := make([]lpBridgeTermJSON, 0, p.RowLen(i))
		termOK := true
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if !termOK {
				return
			}
			cr, okT := fuzzRatFromFloat(coeff)
			if !okT {
				termOK = false
				return
			}
			terms = append(terms, lpBridgeTermJSON{Col: col, Coeff: cr})
		})
		if !termOK {
			return fuzzRatJSON{}, nil, nil, nil, nil, fmt.Errorf("non-finite row term at row %d", i)
		}
		rows[i] = lpBridgeRowJSON{Sense: sTok, RHS: rhs, Terms: terms}
	}
	return oc, obj, lb, ub, rows, nil
}

type parsedProblemRow struct {
	sense  gb.ConstraintSense
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

func parseConstraintSenseTokenStrict(s string) (gb.ConstraintSense, error) {
	switch s {
	case "le", "<=":
		return gb.LessEqual, nil
	case "ge", ">=":
		return gb.GreaterEqual, nil
	case "eq", "=", "==":
		return gb.Equal, nil
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
	p *gb.CompiledProblem,
	gotVars, gotRows int,
	gotObjConst *big.Rat,
	gotObj, gotLB, gotUB []*big.Rat,
	gotCon []parsedProblemRow,
) error {
	if p == nil {
		return errors.New("nil CompiledProblem")
	}
	wantVars := p.NumVars()
	wantRows := p.NumConstraints()
	if gotVars != wantVars {
		return fmt.Errorf("serialized numVars=%d != CompiledProblem NumVars=%d", gotVars, wantVars)
	}
	if gotRows != wantRows || len(gotCon) != wantRows {
		return fmt.Errorf("serialized numRows=%d != CompiledProblem NumConstraints=%d", gotRows, wantRows)
	}
	constMin := p.ObjConstant()
	if p.Sense() == gb.Maximize && constMin != 0 {
		constMin = -constMin
	}
	wantObjConst := new(big.Rat).SetFloat64(constMin)
	if wantObjConst == nil {
		return fmt.Errorf("non-finite CompiledProblem ObjConstant %v", constMin)
	}
	if gotObjConst.Cmp(wantObjConst) != 0 {
		return fmt.Errorf("serialized objConst=%s != CompiledProblem minObjConst=%s",
			gotObjConst.RatString(), wantObjConst.RatString())
	}
	if len(gotObj) != wantVars || len(gotLB) != wantVars || len(gotUB) != wantVars {
		return fmt.Errorf("serialized column vector lengths mismatch numVars=%d", wantVars)
	}
	for j := range wantVars {
		wantC := new(big.Rat).SetFloat64(p.ObjMinCoeff(j))
		if wantC == nil {
			return fmt.Errorf("non-finite CompiledProblem ObjMinCoeff[%d]", j)
		}
		if gotObj[j].Cmp(wantC) != 0 {
			return fmt.Errorf("serialized obj[%d]=%s != CompiledProblem minObj=%s",
				j, gotObj[j].RatString(), wantC.RatString())
		}
		lo, hi := p.VarBounds(j)
		if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 1) || math.IsInf(hi, -1) {
			return fmt.Errorf("invalid CompiledProblem bounds [%v,%v] at col %d", lo, hi, j)
		}
		if math.IsInf(lo, -1) {
			if gotLB[j] != nil {
				return fmt.Errorf("serialized lb[%d]=%s != -Inf", j, gotLB[j].RatString())
			}
		} else {
			wantLB := new(big.Rat).SetFloat64(lo)
			if wantLB == nil || gotLB[j] == nil || gotLB[j].Cmp(wantLB) != 0 {
				return fmt.Errorf("serialized lb[%d] mismatch (want %v)", j, lo)
			}
		}
		if math.IsInf(hi, 1) {
			if gotUB[j] != nil {
				return fmt.Errorf("serialized ub[%d]=%s != +Inf", j, gotUB[j].RatString())
			}
		} else {
			wantUB := new(big.Rat).SetFloat64(hi)
			if wantUB == nil || gotUB[j] == nil || gotUB[j].Cmp(wantUB) != 0 {
				return fmt.Errorf("serialized ub[%d] mismatch (want %v)", j, hi)
			}
		}
	}
	for i := range wantRows {
		if gotCon[i].sense != p.RowSense(i) {
			return fmt.Errorf("serialized row[%d].sense=%v != %v", i, gotCon[i].sense, p.RowSense(i))
		}
		wantRHS := new(big.Rat).SetFloat64(p.RowRHS(i))
		if wantRHS == nil {
			return fmt.Errorf("non-finite CompiledProblem RowRHS[%d]", i)
		}
		if gotCon[i].rhs.Cmp(wantRHS) != 0 {
			return fmt.Errorf("serialized row[%d].rhs=%s != %s",
				i, gotCon[i].rhs.RatString(), wantRHS.RatString())
		}
		wantCoeffs := make(map[int]*big.Rat, p.RowLen(i))
		var rowErr error
		p.ForEachRowTerm(i, func(col int, val float64) {
			if rowErr != nil {
				return
			}
			vr := new(big.Rat).SetFloat64(val)
			if vr == nil {
				rowErr = fmt.Errorf("non-finite CompiledProblem coeff at row %d col %d", i, col)
				return
			}
			rowErr = addRatCoeff(wantCoeffs, col, wantVars, vr)
		})
		if rowErr != nil {
			return rowErr
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

// verifySerializedVIPRMatchesProblem parses the exact VIPR problem instance inside raw
// ("orig" when non-null, otherwise top-level instance) and checks it against p.
func verifySerializedVIPRMatchesProblem(p *gb.CompiledProblem, raw []byte) error {
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
		wantInt := p.VarCategory(j) != gb.Continuous
		if isInt[j] != wantInt {
			return fmt.Errorf("VIPR problem mismatch: isInt[%d]=%v != %v", j, isInt[j], wantInt)
		}
	}
	return nil
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

// verifySerializedLPPayloadMatchesProblem parses the serialized problem inside an
// lp-check payload (resolving "orig"/"lpOrig" for lp-presolve) and checks it against p.
func verifySerializedLPPayloadMatchesProblem(p *gb.CompiledProblem, payload []byte) error {
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
			wantInt := p.VarCategory(j) != gb.Continuous
			if intFlags[j] != wantInt {
				return fmt.Errorf("milp-unbounded int flag[%d]=%v != %v", j, intFlags[j], wantInt)
			}
		}
	}
	return nil
}

func buildPhase1FarkasJSON(ctx context.Context, p *gb.CompiledProblem) ([]byte, error) {
	if p == nil || p.NumConstraints() == 0 {
		return nil, errors.New("Phase-I Farkas builder requires at least one row")
	}
	n := p.NumVars()
	m := p.NumConstraints()
	mdl := gb.NewModel("phase1_farkas", gb.Minimize)
	xs := make([]gb.Var, n)
	for j := range n {
		lo, hi := p.VarBounds(j)
		xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), lo, hi)
	}
	obj := gb.NewExpr()
	for i := range m {
		e := gb.NewExpr()
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if coeff != 0 {
				e = e.AddTerm(xs[col], coeff)
			}
		})
		rhs := p.RowRHS(i)
		rName := "r" + strconv.Itoa(i)
		switch p.RowSense(i) {
		case gb.LessEqual:
			sp := mdl.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
			e = e.AddTerm(sp, -1.0)
			obj = obj.AddTerm(sp, 1.0)
			mdl.AddConstraint(rName, gb.Le(e, rhs))
		case gb.GreaterEqual:
			sm := mdl.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
			e = e.AddTerm(sm, 1.0)
			obj = obj.AddTerm(sm, 1.0)
			mdl.AddConstraint(rName, gb.Ge(e, rhs))
		case gb.Equal:
			sp := mdl.AddContVar("sp"+strconv.Itoa(i), 0, math.Inf(1))
			sm := mdl.AddContVar("sm"+strconv.Itoa(i), 0, math.Inf(1))
			e = e.AddTerm(sp, -1.0).AddTerm(sm, 1.0)
			obj = obj.AddTerm(sp, 1.0).AddTerm(sm, 1.0)
			mdl.AddConstraint(rName, gb.Eq(e, rhs))
		default:
			return nil, fmt.Errorf("unsupported row sense at row %d", i)
		}
	}
	mdl.SetObjective(obj)
	pFeas := mdl.Compile()
	ws := gb.AcquireWorkspace(pFeas.NumVars(), pFeas.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	lpFeas := gb.NewLPSolver(pFeas, ws)
	resFeas := lpFeas.Solve()
	if resFeas.Status != gb.StatusOptimal {
		return nil, fmt.Errorf("Phase-I Farkas LP returned status %v", resFeas.Status)
	}
	sol, ok := gb.ExactBasisSolution(ctx, lpFeas)
	if !ok || len(sol.DualMin) != m {
		return nil, fmt.Errorf("Phase-I Farkas ExactBasisSolution failed (ok=%v len=%d want=%d)", ok, len(sol.DualMin), m)
	}
	if sol.ObjectiveMin == nil || sol.ObjectiveMin.Sign() <= 0 {
		return nil, errors.New("Phase-I Farkas slack is zero; original LP is feasible")
	}
	yMin := sol.DualMin
	oc, objPay, lbPay, ubPay, rowsPay, err := encodeProblemHeaderForLPCheck(p)
	if err != nil {
		return nil, err
	}
	wPay := make([]fuzzRatJSON, m)
	for i := range m {
		wPay[i] = fuzzRatFromBigRat(yMin[i])
	}
	return json.Marshal(lpFarkasReqJSON{
		Mode:     "farkas",
		NumVars:  n,
		NumRows:  m,
		ObjConst: oc,
		Obj:      objPay,
		LB:       lbPay,
		UB:       ubPay,
		Rows:     rowsPay,
		W:        wPay,
	})
}

func buildExactFarkasCertJSON(tb testing.TB, p *gb.CompiledProblem, dualHint []float64) ([]byte, error) {
	tb.Helper()
	if p == nil || p.NumConstraints() == 0 {
		return nil, errors.New("Farkas certificate requires non-empty constraint set")
	}
	if len(dualHint) == p.NumConstraints() {
		if mode, pay, ok := gb.ExportLPCheckPayload(p, nil, gb.LPResult{Status: gb.StatusInfeasible, Dual: dualHint}); ok && mode == "farkas" {
			return pay, nil
		}
	}
	ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	lp := gb.NewLPSolver(p, ws)
	res := lp.Solve()
	if res.Status == gb.StatusInfeasible {
		if mode, pay, ok := gb.ExportLPCheckPayload(p, lp, res); ok && mode == "farkas" {
			return pay, nil
		}
	}
	return buildPhase1FarkasJSON(tb.Context(), p)
}

func zeroVarProblemFeasibleRat(p *gb.CompiledProblem) (bool, error) {
	rhsRat := new(big.Rat)
	for i := range p.NumConstraints() {
		if rhsRat.SetFloat64(p.RowRHS(i)) == nil {
			return false, fmt.Errorf("non-finite RHS on 0-var row %d", i)
		}
		switch p.RowSense(i) {
		case gb.LessEqual:
			if rhsRat.Sign() < 0 {
				return false, nil
			}
		case gb.GreaterEqual:
			if rhsRat.Sign() > 0 {
				return false, nil
			}
		case gb.Equal:
			if rhsRat.Sign() != 0 {
				return false, nil
			}
		}
	}
	return true, nil
}

func absFiniteFloat(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	return math.Abs(v)
}

func checkLPPrimalAndDualKKT(label string, p *gb.CompiledProblem, primal, dual []float64, requireDual bool) error {
	n := p.NumVars()
	m := p.NumConstraints()
	if len(primal) != n {
		return fmt.Errorf("[%s] LP primal length=%d, want %d", label, len(primal), n)
	}
	for j := range n {
		xj := primal[j]
		if math.IsNaN(xj) || math.IsInf(xj, 0) {
			return fmt.Errorf("[%s] non-finite primal x[%d]=%v", label, j, xj)
		}
		lo, hi := p.VarBounds(j)
		bScale := max(1.0, max(absFiniteFloat(lo), max(absFiniteFloat(hi), math.Abs(xj))))
		if !math.IsInf(lo, -1) && xj < lo-1e-5*bScale {
			return fmt.Errorf("[%s] primal x[%d]=%g < lb=%g", label, j, xj, lo)
		}
		if !math.IsInf(hi, 1) && xj > hi+1e-5*bScale {
			return fmt.Errorf("[%s] primal x[%d]=%g > ub=%g", label, j, xj, hi)
		}
	}
	for i := range m {
		rhs := p.RowRHS(i)
		act := 0.0
		rowScale := max(1.0, math.Abs(rhs))
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			prod := coeff * primal[col]
			act += prod
			if pAbs := math.Abs(prod); !math.IsInf(pAbs, 0) && !math.IsNaN(pAbs) {
				rowScale += pAbs
			}
		})
		if math.IsNaN(act) || math.IsInf(act, 0) {
			return fmt.Errorf("[%s] non-finite activity on row %d", label, i)
		}
		tol := 1e-5 * rowScale
		switch p.RowSense(i) {
		case gb.LessEqual:
			if act-rhs > tol {
				return fmt.Errorf("[%s] row %d LE violation: act=%g > rhs=%g (tol=%g)", label, i, act, rhs, tol)
			}
		case gb.GreaterEqual:
			if rhs-act > tol {
				return fmt.Errorf("[%s] row %d GE violation: act=%g < rhs=%g (tol=%g)", label, i, act, rhs, tol)
			}
		case gb.Equal:
			if math.Abs(act-rhs) > tol {
				return fmt.Errorf("[%s] row %d EQ violation: act=%g != rhs=%g (tol=%g)", label, i, act, rhs, tol)
			}
		}
	}
	if !requireDual || m == 0 {
		return nil
	}
	if len(dual) != m {
		return fmt.Errorf("[%s] LP dual length=%d, want %d", label, len(dual), m)
	}
	senseSign := 1.0
	if p.Sense() == gb.Maximize {
		senseSign = -1.0
	}
	yMin := make([]float64, m)
	for i := range m {
		yi := senseSign * dual[i]
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			return fmt.Errorf("[%s] non-finite dual[%d]=%v", label, i, dual[i])
		}
		yScale := max(1.0, math.Abs(yi))
		switch p.RowSense(i) {
		case gb.LessEqual:
			if yi > 1e-5*yScale {
				return fmt.Errorf("[%s] dual[%d] violates LE cone: yMin=%g > 0", label, i, yi)
			}
		case gb.GreaterEqual:
			if yi < -1e-5*yScale {
				return fmt.Errorf("[%s] dual[%d] violates GE cone: yMin=%g < 0", label, i, yi)
			}
		}
		yMin[i] = yi
	}
	colDot := make([]float64, n)
	colScale := make([]float64, n)
	for j := range n {
		colScale[j] = max(1.0, math.Abs(p.ObjMinCoeff(j)))
	}
	for i := range m {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			prod := yi * coeff
			colDot[col] += prod
			if pAbs := math.Abs(prod); !math.IsInf(pAbs, 0) && !math.IsNaN(pAbs) {
				colScale[col] += pAbs
			}
		})
	}
	for j := range n {
		rc := p.ObjMinCoeff(j) - colDot[j]
		lo, hi := p.VarBounds(j)
		tol := 1e-5 * colScale[j]
		if math.IsInf(lo, -1) && rc > tol {
			return fmt.Errorf("[%s] col %d has -Inf lb with positive reduced cost rc=%g (tol=%g)", label, j, rc, tol)
		}
		if math.IsInf(hi, 1) && rc < -tol {
			return fmt.Errorf("[%s] col %d has +Inf ub with negative reduced cost rc=%g (tol=%g)", label, j, rc, tol)
		}
	}
	return nil
}

func lpDualGapTolRat(p *gb.CompiledProblem, linScale float64, evalMinRat, bnd *big.Rat) *big.Rat {
	boundSpan := 0.0
	for j := range p.NumVars() {
		lb, ub := p.VarBounds(j)
		span := max(absFiniteFloat(lb), absFiniteFloat(ub)) * max(1.0, math.Abs(p.ObjMinCoeff(j)))
		if !math.IsInf(span, 0) && !math.IsNaN(span) {
			boundSpan += span
		}
	}
	evalAbs := 0.0
	if evalMinRat != nil {
		if f, _ := evalMinRat.Float64(); !math.IsInf(f, 0) && !math.IsNaN(f) {
			evalAbs = math.Abs(f)
		}
	}
	bndAbs := 0.0
	if bnd != nil {
		if f, _ := bnd.Float64(); !math.IsInf(f, 0) && !math.IsNaN(f) {
			bndAbs = math.Abs(f)
		}
	}
	scale := max(1.0, max(linScale, max(boundSpan, max(evalAbs, bndAbs))))
	return new(big.Rat).SetFloat64(1e-4 * scale)
}

func hasAllFiniteVarBounds(p *gb.CompiledProblem) bool {
	if p == nil {
		return false
	}
	for j := range p.NumVars() {
		lo, hi := p.VarBounds(j)
		if math.IsInf(lo, 0) || math.IsInf(hi, 0) || math.IsNaN(lo) || math.IsNaN(hi) {
			return false
		}
	}
	return true
}

func solveElasticFeasibleBaseRat(ctx context.Context, p *gb.CompiledProblem, boxInf bool) ([]*big.Rat, bool) {
	n := p.NumVars()
	m := p.NumConstraints()
	mdl := gb.NewModel("feas_base", gb.Minimize)
	xs := make([]gb.Var, n)
	for j := range n {
		lo, hi := p.VarBounds(j)
		if boxInf {
			if math.IsInf(lo, -1) {
				lo = -1e9
			}
			if math.IsInf(hi, 1) {
				hi = 1e9
			}
		}
		xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), lo, hi)
	}
	obj := gb.NewExpr()
	for i := range m {
		e := gb.NewExpr()
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if coeff != 0 {
				e = e.AddTerm(xs[col], coeff)
			}
		})
		rhs := p.RowRHS(i)
		rName := "r" + strconv.Itoa(i)
		sCap := math.Inf(1)
		if boxInf {
			sCap = 1e12
		}
		switch p.RowSense(i) {
		case gb.LessEqual:
			sp := mdl.AddContVar("sp"+strconv.Itoa(i), 0, sCap)
			e = e.AddTerm(sp, -1)
			obj = obj.AddTerm(sp, 1)
			mdl.AddConstraint(rName, gb.Le(e, rhs))
		case gb.GreaterEqual:
			sm := mdl.AddContVar("sm"+strconv.Itoa(i), 0, sCap)
			e = e.AddTerm(sm, 1)
			obj = obj.AddTerm(sm, 1)
			mdl.AddConstraint(rName, gb.Ge(e, rhs))
		case gb.Equal:
			sp := mdl.AddContVar("sp"+strconv.Itoa(i), 0, sCap)
			sm := mdl.AddContVar("sm"+strconv.Itoa(i), 0, sCap)
			e = e.AddTerm(sp, -1).AddTerm(sm, 1)
			obj = obj.AddTerm(sp, 1).AddTerm(sm, 1)
			mdl.AddConstraint(rName, gb.Eq(e, rhs))
		default:
			return nil, false
		}
	}
	mdl.SetObjective(obj)
	pFeas := mdl.Compile()
	lpFeas := gb.NewLPSolver(pFeas, nil)
	defer lpFeas.Close()
	if lpFeas.Solve().Status != gb.StatusOptimal {
		return nil, false
	}
	sol, ok := gb.ExactBasisSolution(ctx, lpFeas)
	if !ok || sol.ObjectiveMin == nil || sol.ObjectiveMin.Sign() != 0 || len(sol.Primal) < n {
		return nil, false
	}
	return sol.Primal[:n], true
}

func buildSubTolUnboundedCertJSON(ctx context.Context, p *gb.CompiledProblem) ([]byte, error) {
	n := p.NumVars()
	m := p.NumConstraints()
	mRay := gb.NewModel("recession_ray", gb.Minimize)
	ds := make([]gb.Var, n)
	objR := gb.NewExpr()
	for j := range n {
		lo, hi := p.VarBounds(j)
		dLo, dHi := 0.0, 0.0
		if math.IsInf(lo, -1) {
			dLo = -1.0
		}
		if math.IsInf(hi, 1) {
			dHi = 1.0
		}
		ds[j] = mRay.AddContVar("d"+strconv.Itoa(j), dLo, dHi)
		if c := p.ObjMinCoeff(j); c != 0 {
			objR = objR.AddTerm(ds[j], c)
		}
	}
	mRay.SetObjective(objR)
	for i := range m {
		e := gb.NewExpr()
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if coeff != 0 {
				e = e.AddTerm(ds[col], coeff)
			}
		})
		rName := "r" + strconv.Itoa(i)
		switch p.RowSense(i) {
		case gb.LessEqual:
			mRay.AddConstraint(rName, gb.Le(e, 0))
		case gb.GreaterEqual:
			mRay.AddConstraint(rName, gb.Ge(e, 0))
		case gb.Equal:
			mRay.AddConstraint(rName, gb.Eq(e, 0))
		default:
			return nil, fmt.Errorf("invalid row sense at row %d", i)
		}
	}
	pRay := mRay.Compile()
	lpRay := gb.NewLPSolver(pRay, nil)
	defer lpRay.Close()
	if lpRay.Solve().Status != gb.StatusOptimal {
		return nil, errors.New("recession ray LP did not solve to StatusOptimal")
	}
	solRay, okR := gb.ExactBasisSolution(ctx, lpRay)
	if !okR || solRay.ObjectiveMin == nil || solRay.ObjectiveMin.Sign() >= 0 || len(solRay.Primal) < n {
		return nil, errors.New("no strictly improving recession ray exists over Q")
	}
	x0, okX := solveElasticFeasibleBaseRat(ctx, p, false)
	if !okX {
		x0, okX = solveElasticFeasibleBaseRat(ctx, p, true)
	}
	if !okX {
		return nil, errors.New("failed to extract exact feasible base point x0")
	}
	oc, objPay, lbPay, ubPay, rowsPay, err := encodeProblemHeaderForLPCheck(p)
	if err != nil {
		return nil, err
	}
	x0Pay := make([]fuzzRatJSON, n)
	dPay := make([]fuzzRatJSON, n)
	for j := range n {
		x0Pay[j] = fuzzRatFromBigRat(x0[j])
		dPay[j] = fuzzRatFromBigRat(solRay.Primal[j])
	}
	return json.Marshal(lpUnboundedReqJSON{
		Mode:     "unbounded",
		NumVars:  n,
		NumRows:  m,
		ObjConst: oc,
		Obj:      objPay,
		LB:       lbPay,
		UB:       ubPay,
		Rows:     rowsPay,
		X0:       x0Pay,
		D:        dPay,
	})
}

func certifySubTolLPNumericalError(tb testing.TB, label string, p *gb.CompiledProblem, iters, primalLen int) error {
	tb.Helper()
	if iters <= 0 || primalLen != 0 || p.NumVars() == 0 || p.NumConstraints() == 0 {
		return fmt.Errorf("[%s] uncertified LP terminal status NumericalError (iters=%d primalLen=%d)", label, iters, primalLen)
	}
	if !isStandardScaledProblem(p) {
		return nil
	}
	if hasAllFiniteVarBounds(p) {
		return fmt.Errorf("[%s] uncertified LP terminal status NumericalError on bounded standard LP", label)
	}
	boundedDim := p.NumVars() <= 128 && p.NumConstraints() <= 128
	if unbPay, errU := buildSubTolUnboundedCertJSON(tb.Context(), p); errU == nil {
		if formalFuzzEnabled() && boundedDim {
			if _, err := checkFormalLPJSONLForProblem(tb, p, unbPay); err != nil {
				return fmt.Errorf("[%s] %w", label, err)
			}
		}
		return nil
	}
	if fPay, errF := buildPhase1FarkasJSON(tb.Context(), p); errF == nil {
		if formalFuzzEnabled() && boundedDim {
			if _, err := checkFormalLPJSONLForProblem(tb, p, fPay); err != nil {
				return fmt.Errorf("[%s] %w", label, err)
			}
		}
		return nil
	}
	return fmt.Errorf("[%s] uncertified LP terminal status NumericalError on bounded-optimal LP", label)
}

func checkFormalLPResult(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	lp *gb.LPSolver,
	res gb.LPResult,
) error {
	tb.Helper()
	if !formalFuzzEnabled() {
		return errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	if p == nil {
		return fmt.Errorf("[%s] nil CompiledProblem", label)
	}
	if res.Status == gb.StatusNumericalError {
		return certifySubTolLPNumericalError(tb, label, p, res.Iterations, len(res.Primal))
	}
	if isTerminalFailureStatus(res.Status) {
		return fmt.Errorf("[%s] uncertified LP terminal status %v", label, res.Status)
	}
	if isResourceLimitStatus(res.Status) {
		return nil
	}
	if p.NumVars() == 0 {
		feas, err := zeroVarProblemFeasibleRat(p)
		if err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		if feas && res.Status != gb.StatusOptimal {
			return fmt.Errorf("[%s] 0-var feasible LP returned %v, want StatusOptimal", label, res.Status)
		}
		if !feas && res.Status != gb.StatusInfeasible {
			return fmt.Errorf("[%s] 0-var infeasible LP returned %v, want StatusInfeasible", label, res.Status)
		}
		return nil
	}

	switch res.Status {
	case gb.StatusOptimal:
		if err := checkLPPrimalAndDualKKT(label, p, res.Primal, res.Dual, p.NumConstraints() > 0); err != nil {
			return err
		}
		evalObj := p.EvaluateObjective(res.Primal)
		if math.Abs(evalObj-res.Objective) > 1e-5*max(1.0, math.Abs(res.Objective)) {
			return fmt.Errorf("[%s] LP EvaluateObjective=%.8g != Objective=%.8g", label, evalObj, res.Objective)
		}
		mode, payload, ok := gb.ExportLPCheckPayload(p, lp, res)
		if !ok {
			return fmt.Errorf("[%s] ExportLPCheckPayload failed on StatusOptimal LP (vars=%d rows=%d)",
				label, p.NumVars(), p.NumConstraints())
		}
		if mode != "optimal" && mode != "safe-bound" {
			return fmt.Errorf("[%s] expected optimal or safe-bound LP certificate, got mode=%q", label, mode)
		}
		bnd, err := checkFormalLPJSONLForProblem(tb, p, payload)
		if err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		if bnd == nil {
			return fmt.Errorf("[%s] lp-check returned nil bound for StatusOptimal LP", label)
		}
		evalMinRat, linScale, okRat := exactEvalMinObjRat(p, res.Primal)
		if !okRat {
			return fmt.Errorf("[%s] failed to rationalize LP primal objective", label)
		}
		if tolRat := new(big.Rat).SetFloat64(1e-4 * max(1.0, linScale)); tolRat != nil {
			limit := new(big.Rat).Add(evalMinRat, tolRat)
			if bnd.Cmp(limit) > 0 {
				return fmt.Errorf("[%s] %s bound %s exceeds primal objective %s + tol",
					label, mode, bnd.RatString(), evalMinRat.RatString())
			}
		}
		if gapTolRat := lpDualGapTolRat(p, linScale, evalMinRat, bnd); gapTolRat != nil {
			gap := new(big.Rat).Sub(evalMinRat, bnd)
			if gap.Cmp(gapTolRat) > 0 {
				return fmt.Errorf("[%s] LP duality gap %s exceeds gapTol %s (primal=%s bound=%s)",
					label, gap.RatString(), gapTolRat.RatString(), evalMinRat.RatString(), bnd.RatString())
			}
		}
		return nil

	case gb.StatusInfeasible:
		mode, payload, ok := gb.ExportLPCheckPayload(p, lp, res)
		if !ok {
			if p.NumConstraints() == 0 {
				return fmt.Errorf("[%s] 0-constraint LP without bound conflict declared StatusInfeasible", label)
			}
			if lp != nil && res.Iterations > 0 {
				p1Pay, p1Err := buildPhase1FarkasJSON(tb.Context(), p)
				if p1Err == nil {
					mode, payload, ok = "farkas", p1Pay, true
				}
			}
			if !ok {
				return fmt.Errorf("[%s] ExportLPCheckPayload failed on StatusInfeasible LP (vars=%d rows=%d)",
					label, p.NumVars(), p.NumConstraints())
			}
		}
		if mode != "farkas" && mode != "bound-conflict" {
			return fmt.Errorf("[%s] expected farkas or bound-conflict LP certificate, got mode=%q", label, mode)
		}
		if _, err := checkFormalLPJSONLForProblem(tb, p, payload); err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		return nil

	case gb.StatusUnbounded:
		for j, v := range res.Primal {
			if math.IsNaN(v) {
				return fmt.Errorf("[%s] StatusUnbounded primal[%d] is NaN", label, j)
			}
		}
		mode, payload, ok := gb.ExportLPCheckPayload(p, lp, res)
		if !ok {
			return fmt.Errorf("[%s] ExportLPCheckPayload failed on StatusUnbounded LP (vars=%d rows=%d)",
				label, p.NumVars(), p.NumConstraints())
		}
		if mode != "unbounded" && mode != "milp-unbounded" {
			return fmt.Errorf("[%s] expected unbounded LP certificate, got mode=%q", label, mode)
		}
		if _, err := checkFormalLPJSONLForProblem(tb, p, payload); err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		return nil

	default:
		return fmt.Errorf("[%s] unexpected LP status %v", label, res.Status)
	}
}

func checkFormalPresolvedLPResult(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	resPre gb.Result,
	checkDualGap bool,
) error {
	tb.Helper()
	if p == nil {
		return fmt.Errorf("[%s] nil CompiledProblem", label)
	}
	if resPre.Status == gb.StatusNumericalError {
		return certifySubTolLPNumericalError(tb, label, p, resPre.Iterations, len(resPre.Values))
	}
	if isTerminalFailureStatus(resPre.Status) {
		return fmt.Errorf("[%s] uncertified presolved LP terminal status %v", label, resPre.Status)
	}
	if isResourceLimitStatus(resPre.Status) {
		return nil
	}
	if p.NumVars() == 0 {
		feas, err := zeroVarProblemFeasibleRat(p)
		if err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		if feas && resPre.Status != gb.StatusOptimal {
			return fmt.Errorf("[%s] 0-var feasible presolved LP returned %v, want StatusOptimal", label, resPre.Status)
		}
		if !feas && resPre.Status != gb.StatusInfeasible {
			return fmt.Errorf("[%s] 0-var infeasible presolved LP returned %v, want StatusInfeasible", label, resPre.Status)
		}
		return nil
	}

	switch resPre.Status {
	case gb.StatusUnbounded:
		if formalFuzzEnabled() {
			uMode, uPayload, okU := gb.ExportUnboundedCheckPayload(p, nil, resPre.Values)
			if !okU || (uMode != "unbounded" && uMode != "milp-unbounded") {
				return fmt.Errorf("[%s] ExportUnboundedCheckPayload failed on presolved unbounded LP: ok=%v mode=%q",
					label, okU, uMode)
			}
			if _, err := checkFormalLPJSONLForProblem(tb, p, uPayload); err != nil {
				return fmt.Errorf("[%s] %w", label, err)
			}
		}
		return nil

	case gb.StatusInfeasible:
		if !formalFuzzEnabled() {
			return nil
		}
		if mode, bcPayload, okBC := gb.ExportLPCheckPayload(p, nil, gb.LPResult{Status: gb.StatusInfeasible, Dual: resPre.Duals}); okBC &&
			(mode == "bound-conflict" || mode == "farkas") {
			if _, err := checkFormalLPJSONLForProblem(tb, p, bcPayload); err != nil {
				return fmt.Errorf("[%s] %w", label, err)
			}
			return nil
		}
		if p.NumConstraints() == 0 {
			return fmt.Errorf("[%s] presolved 0-constraint LP without bound conflict declared StatusInfeasible", label)
		}
		fPayload, err := buildExactFarkasCertJSON(tb, p, resPre.Duals)
		if err != nil {
			return fmt.Errorf("[%s] presolved StatusInfeasible LP failed Farkas certification on original model: %w", label, err)
		}
		if _, err := checkFormalLPJSONLForProblem(tb, p, fPayload); err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		return nil

	case gb.StatusOptimal:
		reqDual := formalFuzzEnabled() && p.NumConstraints() > 0
		if err := checkLPPrimalAndDualKKT(label, p, resPre.Values, resPre.Duals, reqDual); err != nil {
			return err
		}
		evalObj := p.EvaluateObjective(resPre.Values)
		if math.Abs(evalObj-resPre.Objective) > 1e-5*max(1.0, math.Abs(resPre.Objective)) {
			return fmt.Errorf("[%s] presolved LP EvaluateObjective=%.8g != Objective=%.8g", label, evalObj, resPre.Objective)
		}
		if !formalFuzzEnabled() {
			return nil
		}

		if lpPrePay, okPre := gb.ExportLPPresolveCheckPayload(p); okPre {
			if _, err := checkFormalLPJSONLForProblem(tb, p, lpPrePay); err != nil {
				return fmt.Errorf("[%s] lp-presolve chain rejected: %w", label, err)
			}
		}

		lpRes := gb.LPResult{
			Status:    gb.StatusOptimal,
			Objective: evalObj,
			Primal:    resPre.Values,
			Dual:      resPre.Duals,
		}
		mode, payload, ok := gb.ExportLPCheckPayload(p, nil, lpRes)
		if !ok || mode != "optimal" {
			ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
			defer gb.ReleaseWorkspace(ws)
			auxLP := gb.NewLPSolver(p, ws)
			auxRes := auxLP.Solve()
			if auxRes.Status != gb.StatusOptimal {
				return fmt.Errorf("[%s] presolved StatusOptimal LP solved to %v on original model", label, auxRes.Status)
			}
			auxMode, auxPayload, okAux := gb.ExportLPCheckPayload(p, auxLP, auxRes)
			if okAux {
				mode, payload, ok = auxMode, auxPayload, true
			}
		}
		if !ok {
			return fmt.Errorf("[%s] failed to export certificate for presolved LP", label)
		}
		if mode != "optimal" && mode != "safe-bound" {
			return fmt.Errorf("[%s] expected mode optimal or safe-bound for presolved LP, got %q", label, mode)
		}
		bnd, err := checkFormalLPJSONLForProblem(tb, p, payload)
		if err != nil {
			return fmt.Errorf("[%s] %w", label, err)
		}
		if bnd == nil {
			return fmt.Errorf("[%s] lp-check returned nil bound", label)
		}
		evalMinRat, linScale, okRat := exactEvalMinObjRat(p, resPre.Values)
		if !okRat {
			return fmt.Errorf("[%s] failed to rationalize presolved LP primal objective", label)
		}
		if tolRat := new(big.Rat).SetFloat64(1e-4 * max(1.0, linScale)); tolRat != nil {
			if bnd.Cmp(new(big.Rat).Add(evalMinRat, tolRat)) > 0 {
				return fmt.Errorf("[%s] presolved LP bound %s > evalObj %s + tol",
					label, bnd.RatString(), evalMinRat.RatString())
			}
		}
		if checkDualGap {
			if gapTolRat := lpDualGapTolRat(p, linScale, evalMinRat, bnd); gapTolRat != nil {
				gap := new(big.Rat).Sub(evalMinRat, bnd)
				if gap.Cmp(gapTolRat) > 0 {
					return fmt.Errorf("[%s] presolved LP duality gap %s > gapTol %s",
						label, gap.RatString(), gapTolRat.RatString())
				}
			}
			if err := checkSingletonDualSlack(label, p, resPre.Values, resPre.Duals); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("[%s] unexpected presolved LP status %v", label, resPre.Status)
	}
}

func checkSingletonDualSlack(label string, p *gb.CompiledProblem, values, duals []float64) error {
	if len(duals) != p.NumConstraints() {
		return nil
	}
	senseSign := 1.0
	if p.Sense() == gb.Maximize {
		senseSign = -1.0
	}
	for i := range p.NumConstraints() {
		if p.RowLen(i) != 1 {
			continue
		}
		yi := senseSign * duals[i]
		if yi == 0 {
			continue
		}
		var col int
		var coeff float64
		p.ForEachRowTerm(i, func(c int, v float64) {
			col = c
			coeff = v
		})
		if coeff == 0 {
			continue
		}
		xVal := values[col]
		impBound := p.RowRHS(i) / coeff
		act := coeff * xVal
		rowSlack := p.RowRHS(i) - act
		switch p.RowSense(i) {
		case gb.LessEqual:
			if (coeff > 0 && impBound-xVal > 1e-2*max(1.0, math.Abs(xVal)) && rowSlack > 0) ||
				(coeff < 0 && xVal-impBound > 1e-2*max(1.0, math.Abs(xVal)) && rowSlack > 0) {
				if math.Abs(yi) > 1e-6 {
					return fmt.Errorf("[%s] slack singleton row %d has non-zero dual y=%g (x=%g impBound=%g)",
						label, i, yi, xVal, impBound)
				}
			}
		case gb.GreaterEqual:
			if (coeff > 0 && xVal-impBound > 1e-2*max(1.0, math.Abs(xVal)) && rowSlack < 0) ||
				(coeff < 0 && impBound-xVal > 1e-2*max(1.0, math.Abs(xVal)) && rowSlack < 0) {
				if math.Abs(yi) > 1e-6 {
					return fmt.Errorf("[%s] slack singleton row %d has non-zero dual y=%g (x=%g impBound=%g)",
						label, i, yi, xVal, impBound)
				}
			}
		}
	}
	return nil
}

func isStandardScaledProblem(p *gb.CompiledProblem) bool {
	if p == nil || p.NumVars() == 0 || p.NumVars() > 128 || p.NumConstraints() > 128 {
		return false
	}
	for j := range p.NumVars() {
		lo, hi := p.VarBounds(j)
		if (!math.IsInf(lo, -1) && (math.Abs(lo) > 1e12 || (lo != 0 && math.Abs(lo) < 1e-6))) ||
			(!math.IsInf(hi, 1) && (math.Abs(hi) > 1e12 || (hi != 0 && math.Abs(hi) < 1e-6))) {
			return false
		}
		cj := p.ObjMinCoeff(j)
		if cj != 0 && (math.Abs(cj) > 1e12 || math.Abs(cj) < 1e-6) {
			return false
		}
	}
	for i := range p.NumConstraints() {
		rhs := p.RowRHS(i)
		if rhs != 0 && (math.Abs(rhs) > 1e12 || math.Abs(rhs) < 1e-6) {
			return false
		}
		rowOK := true
		p.ForEachRowTerm(i, func(_ int, coeff float64) {
			if !rowOK || coeff == 0 {
				return
			}
			if math.Abs(coeff) > 1e12 || math.Abs(coeff) < 1e-6 {
				rowOK = false
			}
		})
		if !rowOK {
			return false
		}
	}
	return true
}

func isExactRatSliceMIPFeasible(p *gb.CompiledProblem, xRats []*big.Rat) bool {
	if p == nil || len(xRats) < p.NumVars() {
		return false
	}
	n := p.NumVars()
	bndRat := new(big.Rat)
	for j := range n {
		xr := xRats[j]
		if xr == nil {
			return false
		}
		lb, ub := p.VarBounds(j)
		if !math.IsInf(lb, -1) {
			if bndRat.SetFloat64(lb) == nil || xr.Cmp(bndRat) < 0 {
				return false
			}
		}
		if !math.IsInf(ub, 1) {
			if bndRat.SetFloat64(ub) == nil || xr.Cmp(bndRat) > 0 {
				return false
			}
		}
	}
	for k := range p.NumIntVars() {
		if !xRats[p.IntCol(k)].IsInt() {
			return false
		}
	}
	act := new(big.Rat)
	aRat := new(big.Rat)
	prod := new(big.Rat)
	rhsRat := new(big.Rat)
	for i := range p.NumConstraints() {
		if rhsRat.SetFloat64(p.RowRHS(i)) == nil {
			return false
		}
		act.SetInt64(0)
		rowOK := true
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			if !rowOK || coeff == 0 || xRats[col].Sign() == 0 {
				return
			}
			if aRat.SetFloat64(coeff) == nil {
				rowOK = false
				return
			}
			prod.Mul(aRat, xRats[col])
			act.Add(act, prod)
		})
		if !rowOK {
			return false
		}
		cmp := act.Cmp(rhsRat)
		switch p.RowSense(i) {
		case gb.LessEqual:
			if cmp > 0 {
				return false
			}
		case gb.GreaterEqual:
			if cmp < 0 {
				return false
			}
		case gb.Equal:
			if cmp != 0 {
				return false
			}
		}
	}
	return true
}

func checkTightContPrimalResiduals(p *gb.CompiledProblem, x []float64, relTol float64) error {
	for j := range p.NumVars() {
		lo, hi := p.VarBounds(j)
		bScale := max(1.0, max(absFiniteFloat(lo), max(absFiniteFloat(hi), math.Abs(x[j]))))
		if !math.IsInf(lo, -1) && x[j] < lo-relTol*bScale {
			return errors.New("lower bound residual exceeds tight threshold")
		}
		if !math.IsInf(hi, 1) && x[j] > hi+relTol*bScale {
			return errors.New("upper bound residual exceeds tight threshold")
		}
	}
	for i := range p.NumConstraints() {
		rhs := p.RowRHS(i)
		act := 0.0
		rowScale := max(1.0, math.Abs(rhs))
		p.ForEachRowTerm(i, func(col int, coeff float64) {
			prod := coeff * x[col]
			act += prod
			if pAbs := math.Abs(prod); !math.IsInf(pAbs, 0) && !math.IsNaN(pAbs) {
				rowScale += pAbs
			}
		})
		tol := relTol * rowScale
		switch p.RowSense(i) {
		case gb.LessEqual:
			if act-rhs > tol {
				return errors.New("LE residual exceeds tight threshold")
			}
		case gb.GreaterEqual:
			if rhs-act > tol {
				return errors.New("GE residual exceeds tight threshold")
			}
		case gb.Equal:
			if math.Abs(act-rhs) > tol {
				return errors.New("EQ residual exceeds tight threshold")
			}
		}
	}
	return nil
}

// exactMIPIncumbentRat checks that x maps to an exact rational MIP point on p,
// refines continuous columns via ExactBasisSolution on non-dyadic rows, and returns
// the exact minimized objective over Q.
func exactMIPIncumbentRat(ctx context.Context, p *gb.CompiledProblem, x []float64) (*big.Rat, bool) {
	if p == nil || len(x) != p.NumVars() {
		return nil, false
	}
	n := p.NumVars()
	for j := range n {
		if math.IsNaN(x[j]) || math.IsInf(x[j], 0) {
			return nil, false
		}
	}
	for k := range p.NumIntVars() {
		col := p.IntCol(k)
		if math.Abs(x[col]-math.Round(x[col])) > 1e-9 {
			return nil, false
		}
	}
	if isExactRatMIPFeasible(p, x) {
		minRat, _, ok := exactEvalMinObjRat(p, x)
		return minRat, ok
	}
	xRound := make([]float64, n)
	copy(xRound, x)
	for k := range p.NumIntVars() {
		col := p.IntCol(k)
		xRound[col] = math.Round(x[col])
	}
	if isExactRatMIPFeasible(p, xRound) {
		minRat, _, ok := exactEvalMinObjRat(p, xRound)
		return minRat, ok
	}
	if p.NumVars() == p.NumIntVars() || p.NumVars() > 256 || p.NumConstraints() > 256 {
		return nil, false
	}
	// Require xRound to satisfy simplex primal tolerances before exact basis polish.
	if err := checkTightContPrimalResiduals(p, xRound, 1e-5); err != nil {
		return nil, false
	}
	rawMinRat, linScale, okRaw := exactEvalMinObjRat(p, x)
	if !okRaw {
		return nil, false
	}

	lbFix := make([]float64, n)
	ubFix := make([]float64, n)
	for j := range n {
		lbFix[j], ubFix[j] = p.VarBounds(j)
	}
	for k := range p.NumIntVars() {
		col := p.IntCol(k)
		iv := math.Round(x[col])
		if iv < lbFix[col] || iv > ubFix[col] {
			return nil, false
		}
		lbFix[col] = iv
		ubFix[col] = iv
	}
	ws := gb.AcquireWorkspace(n, p.NumConstraints())
	defer gb.ReleaseWorkspace(ws)
	lp := gb.NewLPSolver(p, ws)
	lp.SetStructuralBounds(lbFix, ubFix)
	lpRes := lp.Solve()
	if lpRes.Status != gb.StatusOptimal {
		return nil, false
	}
	sol, okSol := gb.ExactBasisSolution(ctx, lp)
	if !okSol || !isExactRatSliceMIPFeasible(p, sol.Primal) {
		return nil, false
	}
	diff := new(big.Rat).Sub(sol.ObjectiveMin, rawMinRat)
	diff.Abs(diff)
	if tolRat := new(big.Rat).SetFloat64(1e-4 * max(1.0, linScale)); tolRat != nil && diff.Cmp(tolRat) > 0 {
		return nil, false
	}
	return sol.ObjectiveMin, true
}

func checkMILPInvariantsErr(label string, p *gb.CompiledProblem, res gb.Result) error {
	if p == nil {
		return fmt.Errorf("[%s] nil CompiledProblem", label)
	}
	if isTerminalFailureStatus(res.Status) {
		if res.Status == gb.StatusNumericalError {
			if res.Backend == gb.BackendCBC {
				return nil
			}
			if !isStandardScaledProblem(p) {
				if len(res.Values) == p.NumVars() {
					bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
					if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
						return fmt.Errorf("[%s] NumericalError returned invalid incumbent: bound=%.2e row=%.2e int=%.2e",
							label, bV, rV, iV)
					}
				}
				return nil
			}
		}
		return fmt.Errorf("[%s] uncertified MILP terminal status %v", label, res.Status)
	}
	if res.Status != gb.StatusOptimal {
		return nil
	}
	if len(res.Values) != p.NumVars() {
		return fmt.Errorf("[%s] Optimal status with len(Values)=%d, want %d", label, len(res.Values), p.NumVars())
	}
	bV, rV, iV := p.MaxViolation(res.Values, p.IsMIP())
	rowTol := 1e-5
	if res.Backend == gb.BackendCBC {
		rowTol = 1e-4
	}
	if bV > 1e-5 || rV > rowTol || iV > 1e-5 {
		return fmt.Errorf("[%s] Optimal solution violates model: bound=%.2e row=%.2e int=%.2e", label, bV, rV, iV)
	}
	evalObj := p.EvaluateObjective(res.Values)
	if math.Abs(evalObj-res.Objective) > 1e-6*max(1.0, math.Abs(res.Objective)) {
		return fmt.Errorf("[%s] EvaluateObjective=%.8f != res.Objective=%.8f", label, evalObj, res.Objective)
	}
	boundTol := 1e-5 * max(1.0, math.Abs(res.Objective))
	if p.Sense() == gb.Minimize {
		if res.BestBound > res.Objective+boundTol {
			return fmt.Errorf("[%s] Minimize BestBound %.8f > Objective %.8f", label, res.BestBound, res.Objective)
		}
		if !math.IsNaN(res.RootLPBound) && res.BestBound < res.RootLPBound-boundTol {
			return fmt.Errorf("[%s] Minimize BestBound %.8f < RootLPBound %.8f", label, res.BestBound, res.RootLPBound)
		}
	} else {
		if res.BestBound < res.Objective-boundTol {
			return fmt.Errorf("[%s] Maximize BestBound %.8f < Objective %.8f", label, res.BestBound, res.Objective)
		}
		if !math.IsNaN(res.RootLPBound) && res.BestBound > res.RootLPBound+boundTol {
			return fmt.Errorf("[%s] Maximize BestBound %.8f > RootLPBound %.8f", label, res.BestBound, res.RootLPBound)
		}
	}
	return nil
}

func evalSerializedVIPRIncumbentMinObj(p *gb.CompiledProblem, rawVIPR []byte) (*big.Rat, error) {
	var parsed struct {
		Incumbent []fuzzRatJSON `json:"incumbent"`
	}
	if err := json.Unmarshal(rawVIPR, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal VIPR incumbent: %w", err)
	}
	if len(parsed.Incumbent) != p.NumVars() {
		return nil, fmt.Errorf("VIPR optimal incumbent len=%d, want %d", len(parsed.Incumbent), p.NumVars())
	}
	constMin := p.ObjConstant()
	if p.Sense() == gb.Maximize && constMin != 0 {
		constMin = -constMin
	}
	total := new(big.Rat).SetFloat64(constMin)
	if total == nil {
		return nil, errors.New("non-finite ObjConstant")
	}
	cRat := new(big.Rat)
	xRat := new(big.Rat)
	prod := new(big.Rat)
	num := new(big.Int)
	den := new(big.Int)
	for j, rj := range parsed.Incumbent {
		if _, ok := num.SetString(rj.Num, 10); !ok {
			return nil, fmt.Errorf("invalid VIPR incumbent num %q", rj.Num)
		}
		if _, ok := den.SetString(rj.Den, 10); !ok || den.Sign() <= 0 {
			return nil, fmt.Errorf("invalid VIPR incumbent den %q", rj.Den)
		}
		xRat.SetFrac(num, den)
		cj := p.ObjMinCoeff(j)
		if cj == 0 || xRat.Sign() == 0 {
			continue
		}
		if cRat.SetFloat64(cj) == nil {
			return nil, fmt.Errorf("non-finite ObjMinCoeff at %d", j)
		}
		prod.Mul(cRat, xRat)
		total.Add(total, prod)
	}
	return total, nil
}

func checkFormalUnboundedMILP(tb testing.TB, label string, p *gb.CompiledProblem, values []float64) error {
	tb.Helper()
	if p.NumVars() == 0 {
		return fmt.Errorf("[%s] 0-var problem cannot be StatusUnbounded", label)
	}
	for j, v := range values {
		if math.IsNaN(v) {
			return fmt.Errorf("[%s] StatusUnbounded values[%d] is NaN", label, j)
		}
	}
	uMode, uPay, okU := gb.ExportUnboundedCheckPayload(p, nil, values)
	if !okU {
		return fmt.Errorf("[%s] StatusUnbounded failed ExportUnboundedCheckPayload", label)
	}
	if p.IsMIP() && p.NumIntVars() > 0 {
		if uMode != "milp-unbounded" {
			return fmt.Errorf("[%s] expected milp-unbounded certificate for MIP, got %q", label, uMode)
		}
	} else if uMode != "unbounded" {
		return fmt.Errorf("[%s] expected unbounded certificate for LP, got %q", label, uMode)
	}
	if _, err := checkFormalLPJSONLForProblem(tb, p, uPay); err != nil {
		return fmt.Errorf("[%s] %w", label, err)
	}
	return nil
}

func checkFormalVIPRResult(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	tr *gb.VIPRTrace,
	relTol, absTol float64,
) error {
	tb.Helper()
	if !formalFuzzEnabled() {
		return errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	if err := checkMILPInvariantsErr(label, p, res); err != nil {
		return err
	}
	if res.Status == gb.StatusUnbounded {
		return checkFormalUnboundedMILP(tb, label, p, res.Values)
	}

	boundedDim := p.NumVars() > 0 && p.NumVars() <= 512 && p.NumConstraints() <= 512
	if tr == nil || !tr.HasCertificate() {
		if boundedDim && (res.Status == gb.StatusOptimal || res.Status == gb.StatusInfeasible) {
			return fmt.Errorf("[%s] status=%v missing VIPRTrace certificate (vars=%d rows=%d)",
				label, res.Status, p.NumVars(), p.NumConstraints())
		}
		return nil
	}

	raw, err := tr.MarshalVIPRJSON()
	if err != nil {
		return fmt.Errorf("[%s] MarshalVIPRJSON failed: %w", label, err)
	}
	return checkFormalVIPRSerializedResult(tb, label, p, res, tr, raw, relTol, absTol)
}

func checkFormalVIPRSerializedResult(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	tr *gb.VIPRTrace,
	raw []byte,
	relTol, absTol float64,
) error {
	tb.Helper()
	if !formalFuzzEnabled() {
		return errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	if err := checkMILPInvariantsErr(label, p, res); err != nil {
		return err
	}
	if tr == nil || !tr.HasCertificate() {
		return fmt.Errorf("[%s] missing VIPRTrace certificate", label)
	}
	if tr.TracedProblem() != p {
		return fmt.Errorf("[%s] VIPRTrace.TracedProblem() does not match target CompiledProblem", label)
	}
	if err := verifySerializedVIPRMatchesProblem(p, raw); err != nil {
		return fmt.Errorf("[%s] %w", label, err)
	}
	bnd, err := checkFormalVIPRJSONL(tb, raw)
	if err != nil {
		return fmt.Errorf("[%s] %w", label, err)
	}

	switch res.Status {
	case gb.StatusInfeasible:
		if tr.Mode() != "infeasible" {
			return fmt.Errorf("[%s] StatusInfeasible emitted VIPR mode %q, want infeasible", label, tr.Mode())
		}
		return nil

	case gb.StatusOptimal, gb.StatusNodeLimit, gb.StatusTimeLimit, gb.StatusIterationLimit, gb.StatusNumericalError:
		targetLB := tr.TargetLB()
		if targetLB == nil {
			return fmt.Errorf("[%s] status=%v missing TargetLB in VIPRTrace (mode=%q)", label, res.Status, tr.Mode())
		}
		switch tr.Mode() {
		case "bound":
			if bnd == nil {
				return fmt.Errorf("[%s] vipr-check returned nil bound for mode=bound", label)
			}
			if bnd.Cmp(targetLB) != 0 {
				return fmt.Errorf("[%s] vipr-check bound %s != TargetLB %s", label, bnd.RatString(), targetLB.RatString())
			}
		case "optimal":
			if bnd != nil && bnd.Cmp(targetLB) != 0 {
				return fmt.Errorf("[%s] vipr-check bound %s != TargetLB %s", label, bnd.RatString(), targetLB.RatString())
			}
			incObj, errInc := evalSerializedVIPRIncumbentMinObj(p, raw)
			if errInc != nil {
				return fmt.Errorf("[%s] %w", label, errInc)
			}
			if incObj.Cmp(targetLB) != 0 {
				return fmt.Errorf("[%s] VIPR optimal incumbent minObj %s != TargetLB %s",
					label, incObj.RatString(), targetLB.RatString())
			}
		default:
			return fmt.Errorf("[%s] unexpected VIPR mode %q for status %v", label, tr.Mode(), res.Status)
		}
		if len(res.Values) == p.NumVars() {
			minRat, okExact := exactMIPIncumbentRat(tb.Context(), p, res.Values)
			if res.Status == gb.StatusOptimal && isStandardScaledProblem(p) && !okExact {
				return fmt.Errorf("[%s] StatusOptimal solution is not exact-Q MIP feasible", label)
			}
			if okExact && minRat.Cmp(targetLB) < 0 {
				return fmt.Errorf("[%s] exact-Q MIP objective %s < certified VIPR TargetLB %s",
					label, minRat.RatString(), targetLB.RatString())
			}
		}
		if !math.IsNaN(res.BestBound) && !math.IsInf(res.BestBound, 0) {
			userLB := userBoundFromMinRat(p, targetLB)
			if tr.Mode() == "optimal" {
				tol := 1e-4 * max(1.0, max(math.Abs(res.BestBound), math.Abs(userLB)))
				if p.Sense() == gb.Minimize && res.BestBound > userLB+tol {
					return fmt.Errorf("[%s] Minimize BestBound %.8g > VIPR optimal %.8g", label, res.BestBound, userLB)
				}
				if p.Sense() == gb.Maximize && res.BestBound < userLB-tol {
					return fmt.Errorf("[%s] Maximize BestBound %.8g < VIPR optimal %.8g", label, res.BestBound, userLB)
				}
			}
			if res.Status == gb.StatusOptimal && isStandardScaledProblem(p) {
				linScale := math.Abs(res.Objective - p.ObjConstant())
				gapAllow := max(absTol, relTol*max(1.0, linScale)) + 1e-4*max(1.0, max(math.Abs(res.Objective), math.Abs(userLB)))
				if math.Abs(res.Objective-userLB) > gapAllow {
					return fmt.Errorf("[%s] StatusOptimal %.8g differs from VIPR certified bound %.8g by > %.2e",
						label, res.Objective, userLB, gapAllow)
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("[%s] unexpected VIPR solve status %v", label, res.Status)
	}
}

func checkFullFeatureVsVIPR(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	resFull gb.Result,
	resVIPR gb.Result,
	tr *gb.VIPRTrace,
	relTol, absTol float64,
) error {
	tb.Helper()
	if p == nil {
		return fmt.Errorf("[%s] nil CompiledProblem", label)
	}
	if err := checkMILPInvariantsErr(label+"/full", p, resFull); err != nil {
		return err
	}
	if err := checkMILPInvariantsErr(label+"/vipr", p, resVIPR); err != nil {
		return err
	}

	if resFull.Status == gb.StatusUnbounded {
		if resVIPR.Status == gb.StatusOptimal || resVIPR.Status == gb.StatusInfeasible {
			return fmt.Errorf("[%s] full-feature returned StatusUnbounded while VIPR returned %v", label, resVIPR.Status)
		}
		if tr != nil && tr.HasCertificate() && (tr.Mode() == "optimal" || tr.Mode() == "infeasible") {
			return fmt.Errorf("[%s] full-feature returned StatusUnbounded contradicting VIPR certificate mode %q", label, tr.Mode())
		}
		if formalFuzzEnabled() {
			if err := checkFormalUnboundedMILP(tb, label+"/full", p, resFull.Values); err != nil {
				return err
			}
		}
	}
	if resVIPR.Status == gb.StatusUnbounded && (resFull.Status == gb.StatusOptimal || resFull.Status == gb.StatusInfeasible) {
		return fmt.Errorf("[%s] VIPR returned StatusUnbounded while full-feature returned %v", label, resFull.Status)
	}

	var fullMinRat *big.Rat
	okExactFull := false
	if len(resFull.Values) == p.NumVars() {
		fullMinRat, okExactFull = exactMIPIncumbentRat(tb.Context(), p, resFull.Values)
		if resFull.Status == gb.StatusOptimal && isStandardScaledProblem(p) && !okExactFull {
			return fmt.Errorf("[%s] full-feature StatusOptimal solution is not exact-Q MIP feasible", label)
		}
	}

	var viprMinRat *big.Rat
	okExactVIPR := false
	if len(resVIPR.Values) == p.NumVars() {
		viprMinRat, okExactVIPR = exactMIPIncumbentRat(tb.Context(), p, resVIPR.Values)
	}

	if resFull.Status == gb.StatusInfeasible {
		if resVIPR.Status == gb.StatusUnbounded {
			return fmt.Errorf("[%s] full-feature returned StatusInfeasible while VIPR returned StatusUnbounded", label)
		}
		if tr != nil && tr.HasCertificate() && tr.Mode() == "optimal" {
			return fmt.Errorf("[%s] full-feature returned StatusInfeasible contradicting VIPR optimal certificate", label)
		}
		if okExactVIPR {
			return fmt.Errorf("[%s] full-feature falsely declared exact-feasible MIP infeasible", label)
		}
	}
	if resVIPR.Status == gb.StatusInfeasible && okExactFull {
		return fmt.Errorf("[%s] VIPR falsely declared exact-feasible MIP infeasible", label)
	}

	if tr != nil && tr.HasCertificate() && tr.TracedProblem() == p && tr.TargetLB() != nil {
		targetLB := tr.TargetLB()
		if okExactFull && fullMinRat.Cmp(targetLB) < 0 {
			return fmt.Errorf("[%s] full-feature exact-Q MIP minObj %s < certified VIPR TargetLB %s",
				label, fullMinRat.RatString(), targetLB.RatString())
		}
		if tr.Mode() == "optimal" && !math.IsNaN(resFull.BestBound) && !math.IsInf(resFull.BestBound, 0) {
			userOpt := userBoundFromMinRat(p, targetLB)
			linOpt := math.Abs(userOpt - p.ObjConstant())
			bndTol := max(absTol, relTol*max(1.0, linOpt)) + 1e-4*max(1.0, max(math.Abs(resFull.BestBound), math.Abs(userOpt)))
			if p.Sense() == gb.Minimize && resFull.BestBound > userOpt+bndTol {
				return fmt.Errorf("[%s] full-feature Minimize BestBound %.8g > certified VIPR optimal %.8g",
					label, resFull.BestBound, userOpt)
			}
			if p.Sense() == gb.Maximize && resFull.BestBound < userOpt-bndTol {
				return fmt.Errorf("[%s] full-feature Maximize BestBound %.8g < certified VIPR optimal %.8g",
					label, resFull.BestBound, userOpt)
			}
			if resFull.Status == gb.StatusOptimal && okExactFull {
				if math.Abs(resFull.Objective-userOpt) > bndTol {
					return fmt.Errorf("[%s] full-feature Optimal %.8g != certified VIPR optimal %.8g (tol=%.2e)",
						label, resFull.Objective, userOpt, bndTol)
				}
			}
		}
	}

	if okExactVIPR && !math.IsNaN(resFull.BestBound) && !math.IsInf(resFull.BestBound, 0) {
		viprObj := userBoundFromMinRat(p, viprMinRat)
		linVIPR := math.Abs(viprObj - p.ObjConstant())
		bndTol := max(absTol, relTol*max(1.0, linVIPR)) + 1e-4*max(1.0, max(math.Abs(resFull.BestBound), math.Abs(viprObj)))
		if p.Sense() == gb.Minimize && resFull.BestBound > viprObj+bndTol {
			return fmt.Errorf("[%s] full-feature Minimize BestBound %.8g > VIPR feasible witness %.8g",
				label, resFull.BestBound, viprObj)
		}
		if p.Sense() == gb.Maximize && resFull.BestBound < viprObj-bndTol {
			return fmt.Errorf("[%s] full-feature Maximize BestBound %.8g < VIPR feasible witness %.8g",
				label, resFull.BestBound, viprObj)
		}
	}

	if resFull.Status == gb.StatusOptimal && resVIPR.Status == gb.StatusOptimal {
		linScale := max(1.0, max(math.Abs(resFull.Objective-p.ObjConstant()), math.Abs(resVIPR.Objective-p.ObjConstant())))
		scale := max(1.0, max(math.Abs(resFull.Objective), math.Abs(resVIPR.Objective)))
		gapAllow := max(absTol, relTol*linScale) + 1e-5*scale
		if math.Abs(resFull.Objective-resVIPR.Objective) > gapAllow {
			fullBetter := (p.Sense() == gb.Minimize && resFull.Objective < resVIPR.Objective-gapAllow) ||
				(p.Sense() == gb.Maximize && resFull.Objective > resVIPR.Objective+gapAllow)
			viprBetter := (p.Sense() == gb.Minimize && resVIPR.Objective < resFull.Objective-gapAllow) ||
				(p.Sense() == gb.Maximize && resVIPR.Objective > resFull.Objective+gapAllow)
			if (fullBetter && okExactFull) || (viprBetter && okExactVIPR) {
				return fmt.Errorf("[%s] full-feature Optimal %.8g != VIPR Optimal %.8g (tol=%.2e)",
					label, resFull.Objective, resVIPR.Objective, gapAllow)
			}
		}
	}
	return nil
}

func TestFormalFuzzBridgeOffline(t *testing.T) {
	t.Parallel()
	want := os.Getenv("GOBLIN_FORMAL") == "1"
	if got := formalFuzzEnabled(); got != want {
		t.Fatalf("formalFuzzEnabled()=%v, want %v", got, want)
	}
	if !formalFuzzEnabled() {
		if _, err := callFormalOracleJSONL(t, "lp-check", []byte(`{}`)); err == nil {
			t.Fatal("expected error calling oracle when GOBLIN_FORMAL != 1")
		}
	}
}

func TestFormalFuzzBridgeConcurrent(t *testing.T) {
	t.Parallel()
	if !formalFuzzEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run formal fuzz bridge live checks")
	}

	lpFarkas, err := os.ReadFile(filepath.Join("testdata", "formal", "lp_farkas_sample.json"))
	if err != nil {
		t.Fatalf("read lp_farkas_sample.json: %v", err)
	}
	viprCert, err := os.ReadFile(filepath.Join("testdata", "formal", "vipr_sample.json"))
	if err != nil {
		t.Fatalf("read vipr_sample.json: %v", err)
	}

	lpSafe := []byte(`{
		"mode": "safe-bound",
		"numVars": 2,
		"numRows": 1,
		"objConst": "1/2",
		"obj": ["2/1", "3/1"],
		"lb": ["0/1", "0/1"],
		"ub": ["5/1", "5/1"],
		"rows": [{"sense": "ge", "rhs": "4/1", "terms": [{"col": 0, "coeff": "1/1"}, {"col": 1, "coeff": "1/1"}]}],
		"y": ["2/1"]
	}`)
	cutsReq := []byte(`{"op":"row-legal-mu","sense":"ge","mu":"-3/2","expected":true}`)
	dropReq := []byte(`{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"1/1000"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"5/1"}}`)
	rulesReq := []byte(`{"op":"divceil","a":-7,"d":3,"expect_q":-2,"expect_floor":-3}`)
	gcdDivReq := []byte(`{"op":"gcd-div-check","coeffs":[6,-9],"sense":"le","rhs":"17/2","div_coeffs":[2,-3],"div_rhs":2}`)
	lpUnbReq := []byte(`{"mode":"unbounded","numVars":1,"numRows":1,"obj":["-1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}`)
	badRulesReq := []byte(`{"op":"divceil","a":-7,"d":3,"expect_q":-99}`)

	bnd := mustFormalLPCheck(t, lpSafe)
	if bnd == nil || bnd.Cmp(big.NewRat(17, 2)) != 0 {
		t.Fatalf("mustFormalLPCheck bound=%v, want 17/2", bnd)
	}
	mustFormalLPCheck(t, lpFarkas)
	mustFormalLPCheck(t, lpUnbReq)
	mustFormalCutsCheck(t, cutsReq)
	mustFormalCutsCheck(t, dropReq)
	if res := mustFormalRulesCheck(t, rulesReq); len(res) == 0 {
		t.Fatal("expected non-empty result from mustFormalRulesCheck")
	}
	if res := mustFormalRulesCheck(t, gcdDivReq); len(res) == 0 {
		t.Fatal("expected non-empty result from gcd-div-check")
	}
	if rSub, subErr := callFormalOracleSubcmdJSONL(t, "cuts-check", "drop-check", dropReq); subErr != nil || !rSub.OK {
		t.Fatalf("callFormalOracleSubcmdJSONL drop-check failed: resp=%+v err=%v", rSub, subErr)
	}
	mustFormalVIPRCheck(t, viprCert)

	const workers = 4
	const iters = 30
	errCh := make(chan error, workers*iters)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iters {
				switch (id + i) % 5 {
				case 0:
					ok, ratStr, msg := formalOracleCall(t, "lp-check", lpSafe)
					if !ok || ratStr != [2]string{"17", "2"} {
						errCh <- fmt.Errorf("lpSafe failed: ok=%v rat=%v msg=%s", ok, ratStr, msg)
					}
				case 1:
					ok, _, msg := formalOracleCall(t, "cuts-check", cutsReq)
					if !ok {
						errCh <- fmt.Errorf("cutsReq failed: %s", msg)
					}
				case 2:
					ok, _, msg := formalOracleCall(t, "rules-check", rulesReq)
					if !ok {
						errCh <- fmt.Errorf("rulesReq failed: %s", msg)
					}
				case 3:
					ok, _, _ := formalOracleCall(t, "rules-check", badRulesReq)
					if ok {
						errCh <- errors.New("badRulesReq unexpectedly succeeded")
					}
				case 4:
					ok, _, msg := formalOracleCall(t, "vipr-check", viprCert)
					if !ok {
						errCh <- fmt.Errorf("viprCert failed: %s", msg)
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}
}

func TestFormalFuzzBridgeSerializedProblemMismatch(t *testing.T) {
	t.Parallel()

	mTarget := gb.NewModel("vipr_problem_match", gb.Minimize)
	x := mTarget.AddIntVar("x", 0, 5)
	y := mTarget.AddIntVar("y", 0, 5)
	mTarget.SetObjective(gb.NewExpr(x.T(2), y.T(3)))
	mTarget.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(1)).Ge(4))
	pTarget := mTarget.Compile()

	trTarget := gb.NewVIPRTrace()
	resTarget, err := gb.SolveCompiled(t.Context(), pTarget, gb.WithPureGo(), gb.WithPresolve(false), gb.WithVIPRTrace(trTarget))
	if err != nil || resTarget.Status != gb.StatusOptimal {
		t.Fatalf("solve target failed: status=%v err=%v", resTarget.Status, err)
	}
	rawTarget, err := trTarget.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("MarshalVIPRJSON target: %v", err)
	}
	if err := verifySerializedVIPRMatchesProblem(pTarget, rawTarget); err != nil {
		t.Fatalf("valid VIPR trace rejected by verifySerializedVIPRMatchesProblem: %v", err)
	}

	mWeak := gb.NewModel("vipr_problem_weak", gb.Minimize)
	xw := mWeak.AddIntVar("x", 0, 5)
	yw := mWeak.AddIntVar("y", 0, 5)
	mWeak.SetObjective(gb.NewExpr(xw.T(2), yw.T(3)))
	mWeak.AddConstraint("c1", gb.NewExpr(xw.T(1), yw.T(1)).Ge(1))
	pWeak := mWeak.Compile()

	trWeak := gb.NewVIPRTrace()
	resWeak, err := gb.SolveCompiled(t.Context(), pWeak, gb.WithPureGo(), gb.WithPresolve(false), gb.WithVIPRTrace(trWeak))
	if err != nil || resWeak.Status != gb.StatusOptimal {
		t.Fatalf("solve weak failed: status=%v err=%v", resWeak.Status, err)
	}
	rawWeak, err := trWeak.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("MarshalVIPRJSON weak: %v", err)
	}
	if err := verifySerializedVIPRMatchesProblem(pTarget, rawWeak); err == nil {
		t.Fatal("expected verifySerializedVIPRMatchesProblem to reject weakened constraint RHS")
	}

	// Verify duplicate JSON keys are rejected strictly by encoding/json/v2.
	dupJSON := []byte(strings.Replace(string(rawTarget), `"n":2,`, `"n":2,"n":99,`, 1))
	if len(dupJSON) > len(rawTarget) {
		if err := verifySerializedVIPRMatchesProblem(pTarget, dupJSON); err == nil {
			t.Fatal("expected duplicate JSON key to be rejected")
		}
	}

	// Offline checks for mutated presolve-enveloped VIPR JSON and lp-check payloads.
	mPre := gb.NewModel("pre_vipr_match", gb.Minimize)
	xp := mPre.AddBinaryVar("x")
	yp := mPre.AddIntVar("y", 0, 5)
	mPre.SetObjective(gb.NewExpr(xp.T(3), yp.T(2)))
	mPre.AddConstraint("fix", gb.NewExpr(xp.T(1)).Le(0))
	mPre.AddConstraint("dem", gb.NewExpr(xp.T(1), yp.T(1)).Ge(2))
	pPre := mPre.Compile()

	trPre := gb.NewVIPRTrace()
	resPre, err := gb.SolveCompiled(t.Context(), pPre, gb.WithPureGo(), gb.WithPresolve(true), gb.WithVIPRTrace(trPre))
	if err != nil || resPre.Status != gb.StatusOptimal {
		t.Fatalf("solve presolve MIP failed: status=%v err=%v", resPre.Status, err)
	}
	rawPre, err := trPre.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("MarshalVIPRJSON presolved: %v", err)
	}
	if err := verifySerializedVIPRMatchesProblem(pPre, rawPre); err != nil {
		t.Fatalf("valid presolved VIPR rejected: %v", err)
	}
	if err := verifySerializedVIPRMatchesProblem(pTarget, rawPre); err == nil {
		t.Fatal("expected presolved VIPR for pPre to fail against pTarget")
	}
	noOrigPre := []byte(strings.Replace(string(rawPre), `"orig":`, `"ignoredOrig":`, 1))
	if len(noOrigPre) > len(rawPre) {
		if err := verifySerializedVIPRMatchesProblem(pPre, noOrigPre); err == nil {
			t.Fatal("expected presolve field without orig to be rejected")
		}
	}

	mPreWeak := gb.NewModel("pre_vipr_weak", gb.Minimize)
	xpw := mPreWeak.AddBinaryVar("x")
	ypw := mPreWeak.AddIntVar("y", 0, 5)
	mPreWeak.SetObjective(gb.NewExpr(xpw.T(3), ypw.T(2)))
	mPreWeak.AddConstraint("fix", gb.NewExpr(xpw.T(1)).Le(0))
	mPreWeak.AddConstraint("dem", gb.NewExpr(xpw.T(1), ypw.T(1)).Ge(1))
	pPreWeak := mPreWeak.Compile()
	trPreWeak := gb.NewVIPRTrace()
	if _, err := gb.SolveCompiled(t.Context(), pPreWeak, gb.WithPureGo(), gb.WithPresolve(true), gb.WithVIPRTrace(trPreWeak)); err != nil {
		t.Fatalf("solve weak presolve MIP failed: %v", err)
	}
	rawPreWeak, err := trPreWeak.MarshalVIPRJSON()
	if err != nil {
		t.Fatalf("MarshalVIPRJSON weak presolved: %v", err)
	}

	// LP presolve and MILP-unbounded payload verification against pTarget/pLP.
	mLPPre := gb.NewModel("lp_pre_match", gb.Minimize)
	xfl := mLPPre.AddContVar("xf", 1, 1)
	yfl := mLPPre.AddContVar("yf", 0, 10)
	mLPPre.SetObjective(gb.NewExpr(xfl.T(1), yfl.T(2)))
	mLPPre.AddConstraint("dem", gb.NewExpr(xfl.T(1), yfl.T(1)).Ge(4))
	pLPPre := mLPPre.Compile()
	lpPrePay, okLPPre := gb.ExportLPPresolveCheckPayload(pLPPre)
	if !okLPPre {
		t.Fatal("ExportLPPresolveCheckPayload failed")
	}
	if err := verifySerializedLPPayloadMatchesProblem(pLPPre, lpPrePay); err != nil {
		t.Fatalf("valid lp-presolve payload rejected: %v", err)
	}

	mLPPreWeak := gb.NewModel("lp_pre_weak", gb.Minimize)
	xflw := mLPPreWeak.AddContVar("xf", 1, 1)
	yflw := mLPPreWeak.AddContVar("yf", 0, 10)
	mLPPreWeak.SetObjective(gb.NewExpr(xflw.T(1), yflw.T(2)))
	mLPPreWeak.AddConstraint("dem", gb.NewExpr(xflw.T(1), yflw.T(1)).Ge(2))
	pLPPreWeak := mLPPreWeak.Compile()
	lpPrePayWeak, okLPPreW := gb.ExportLPPresolveCheckPayload(pLPPreWeak)
	if !okLPPreW {
		t.Fatal("ExportLPPresolveCheckPayload weak failed")
	}
	if err := verifySerializedLPPayloadMatchesProblem(pLPPre, lpPrePayWeak); err == nil {
		t.Fatal("expected verifySerializedLPPayloadMatchesProblem to reject weakened lp-presolve orig")
	}
	mLP := gb.NewModel("lp_match", gb.Minimize)
	xl := mLP.AddContVar("x", 0, 10)
	mLP.SetObjective(gb.NewExpr(xl.T(3)))
	mLP.AddConstraint("lb", gb.NewExpr(xl.T(1)).Ge(4))
	pLP := mLP.Compile()
	ws := gb.AcquireWorkspace(pLP.NumVars(), pLP.NumConstraints())
	t.Cleanup(func() { gb.ReleaseWorkspace(ws) })
	lpSolver := gb.NewLPSolver(pLP, ws)
	lpRes := lpSolver.Solve()
	if lpRes.Status != gb.StatusOptimal {
		t.Fatalf("expected optimal LP, got %v", lpRes.Status)
	}
	_, lpPay, okLP := gb.ExportLPCheckPayload(pLP, lpSolver, lpRes)
	if !okLP {
		t.Fatal("ExportLPCheckPayload failed")
	}
	if err := verifySerializedLPPayloadMatchesProblem(pLP, lpPay); err != nil {
		t.Fatalf("valid LP payload rejected: %v", err)
	}

	mLPWeak := gb.NewModel("lp_weak", gb.Minimize)
	xlw := mLPWeak.AddContVar("x", 0, 10)
	mLPWeak.SetObjective(gb.NewExpr(xlw.T(3)))
	mLPWeak.AddConstraint("lb", gb.NewExpr(xlw.T(1)).Ge(1))
	pLPWeak := mLPWeak.Compile()
	wsW := gb.AcquireWorkspace(pLPWeak.NumVars(), pLPWeak.NumConstraints())
	t.Cleanup(func() { gb.ReleaseWorkspace(wsW) })
	lpSolverW := gb.NewLPSolver(pLPWeak, wsW)
	lpResW := lpSolverW.Solve()
	_, lpPayWeak, okLPW := gb.ExportLPCheckPayload(pLPWeak, lpSolverW, lpResW)
	if !okLPW {
		t.Fatal("ExportLPCheckPayload weak failed")
	}
	if err := verifySerializedLPPayloadMatchesProblem(pLP, lpPayWeak); err == nil {
		t.Fatal("expected verifySerializedLPPayloadMatchesProblem to reject weakened LP RHS")
	}

	mUnb := gb.NewModel("milp_unb_match", gb.Minimize)
	xu := mUnb.AddIntVar("x", 0, math.Inf(1))
	yu := mUnb.AddContVar("y", 0, math.Inf(1))
	mUnb.SetObjective(gb.NewExpr(xu.T(-1)))
	mUnb.AddConstraint("c", gb.NewExpr(xu.T(1), yu.T(-1)).Le(0))
	pUnb := mUnb.Compile()
	uMode, uPay, okU := gb.ExportUnboundedCheckPayload(pUnb, nil, []float64{0, 0})
	if !okU || uMode != "milp-unbounded" {
		t.Fatalf("ExportUnboundedCheckPayload failed: mode=%q ok=%v", uMode, okU)
	}
	if err := verifySerializedLPPayloadMatchesProblem(pUnb, uPay); err != nil {
		t.Fatalf("valid milp-unbounded payload rejected: %v", err)
	}
	if err := verifySerializedLPPayloadMatchesProblem(pTarget, uPay); err == nil {
		t.Fatal("expected verifySerializedLPPayloadMatchesProblem to reject mismatched milp-unbounded problem")
	}
	var uMap map[string]jsontext.Value
	if err := json.Unmarshal(uPay, &uMap); err != nil {
		t.Fatalf("unmarshal milp-unbounded: %v", err)
	}
	uMap["intCols"] = jsontext.Value(`[1]`)
	mutIntPay, err := json.Marshal(uMap)
	if err != nil {
		t.Fatalf("marshal mutated milp-unbounded: %v", err)
	}
	if err := verifySerializedLPPayloadMatchesProblem(pUnb, mutIntPay); err == nil {
		t.Fatal("expected verifySerializedLPPayloadMatchesProblem to reject mutated intCols")
	}

	t.Run("LiveMutatedProblemPassesOracleYetRejectedByBridge", func(t *testing.T) {
		t.Parallel()
		if !formalFuzzEnabled() {
			t.Skip("set GOBLIN_FORMAL=1 to run live negative oracle control")
		}
		if _, err := checkFormalVIPRJSONL(t, rawWeak); err != nil {
			t.Fatalf("expected weakened VIPR JSON to pass vipr-check standalone, got: %v", err)
		}
		if err := checkFormalVIPRSerializedResult(t, "mutated_vipr_raw", pTarget, resTarget, trTarget, rawWeak, 1e-6, 1e-6); err == nil {
			t.Fatal("checkFormalVIPRSerializedResult accepted mutated VIPR problem JSON")
		}
		if err := checkFormalVIPRResult(t, "wrong_traced_problem", pTarget, resTarget, trWeak, 1e-6, 1e-6); err == nil {
			t.Fatal("checkFormalVIPRResult accepted VIPRTrace traced on a different CompiledProblem")
		}
		if _, err := checkFormalLPJSONL(t, lpPayWeak); err != nil {
			t.Fatalf("expected weakened LP JSON to pass lp-check standalone, got: %v", err)
		}
		if _, err := checkFormalLPJSONLForProblem(t, pLP, lpPayWeak); err == nil {
			t.Fatal("checkFormalLPJSONLForProblem accepted weakened LP problem JSON")
		}
		if _, err := checkFormalVIPRJSONL(t, rawPreWeak); err != nil {
			t.Fatalf("expected weakened presolved VIPR JSON to pass vipr-check standalone, got: %v", err)
		}
		if err := checkFormalVIPRSerializedResult(t, "mutated_presolved_vipr_orig", pPre, resPre, trPre, rawPreWeak, 1e-6, 1e-6); err == nil {
			t.Fatal("checkFormalVIPRSerializedResult accepted mutated presolved orig JSON")
		}
		if _, err := checkFormalLPJSONL(t, lpPrePayWeak); err != nil {
			t.Fatalf("expected weakened lp-presolve JSON to pass lp-check standalone, got: %v", err)
		}
		if _, err := checkFormalLPJSONLForProblem(t, pLPPre, lpPrePayWeak); err == nil {
			t.Fatal("checkFormalLPJSONLForProblem accepted weakened lp-presolve orig JSON")
		}
	})
}
