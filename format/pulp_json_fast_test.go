package format

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	gb "github.com/paulourio/goblin"
)

// TestParsePuLPJSONFastParityAndFallback verifies bit-exact structural parity between the
// zero-reflection fast parser and encoding/json/v2 across all 31 benchmark corpus files, scientific
// notation, 17-digit floats, omitted lowBound keys, escaped strings, and malformed inputs.
func TestParsePuLPJSONFastParityAndFallback(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "testdata", "corpus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corpus JSON files, got %d (err=%v)", len(matches), err)
	}
	for _, p := range matches {
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			t.Fatalf("read %s: %v", p, rErr)
		}
		fastDoc, ok := parsePuLPJSONFast(raw)
		if !ok {
			t.Fatalf("expected parsePuLPJSONFast to succeed on corpus file %s", p)
		}
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if sErr != nil {
			t.Fatalf("parsePuLPJSONStd failed on %s: %v", p, sErr)
		}
		if !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("fast vs std mismatch on %s", p)
		}
	}

	specialPayloads := []struct {
		name     string
		raw      string
		wantFast bool
	}{
		{
			name:     "exponents_and_17digit_and_omitted_lb",
			raw:      `{"parameters":{"name":"sci","sense":-1,"status":1,"sol_status":1},"objective":{"name":null,"coefficients":[{"name":"x","value":1.0000000000007},{"name":"y","value":-1.25e-14}],"constant":1e+2},"variables":[{"name":"x","cat":"Continuous","upBound":1e20},{"name":"y","cat":"Integer","lowBound":null,"upBound":-0.0,"varValue":3,"dj":-2.5}],"constraints":[{"name":"r0","sense":0,"coefficients":[{"name":"x","value":9999999999999999},{"name":"y","value":-42}],"pi":0.5,"constant":-10.75}],"sos1":[],"sos2":[]}`,
			wantFast: true,
		},
		{
			name:     "escaped_string_falls_back_cleanly",
			raw:      `{"parameters":{"name":"esc\u0041","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[],"sos1":[],"sos2":[]}`,
			wantFast: false,
		},
	}
	for _, tc := range specialPayloads {
		raw := []byte(tc.raw)
		fastDoc, ok := parsePuLPJSONFast(raw)
		if ok != tc.wantFast {
			t.Fatalf("%s: parsePuLPJSONFast ok=%v, want %v", tc.name, ok, tc.wantFast)
		}
		gotDoc, pErr := ParsePuLPJSON(raw)
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if (pErr != nil) != (sErr != nil) {
			t.Fatalf("%s: ParsePuLPJSON err=%v vs std err=%v", tc.name, pErr, sErr)
		}
		if !reflect.DeepEqual(gotDoc, stdDoc) {
			t.Fatalf("%s: ParsePuLPJSON mismatch:\ngot:  %+v\nwant: %+v", tc.name, gotDoc, stdDoc)
		}
		if ok && !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("%s: fastDoc mismatch", tc.name)
		}
	}

	malformed := []string{
		`{"parameters":{"name":"x","sense":01}}`,
		`{"parameters":{"name":"x","sense":1}} trailing`,
		`{"parameters":{"name":"x","sense":1`,
		`{"parameters":{"name":"x","sense":1},"variables":[{"name":"v","cat":"Continuous","lowBound":1.}]}`,
	}
	for _, bad := range malformed {
		if _, ok := parsePuLPJSONFast([]byte(bad)); ok {
			t.Fatalf("expected parsePuLPJSONFast to reject malformed payload %q", bad)
		}
		if _, err := ParsePuLPJSON([]byte(bad)); err == nil {
			t.Fatalf("expected ParsePuLPJSON error on malformed payload %q", bad)
		}
	}
}

// TestPuLPJSONFastDuplicateKeysOverflowAndInvertedIntBounds verifies parity with
// encoding/json/v2 on duplicate keys, bounded readInt overflow handling on 32-bit
// and 64-bit platforms, and inverted integer variable bounds on round-trip.
func TestPuLPJSONFastDuplicateKeysOverflowAndInvertedIntBounds(t *testing.T) {
	duplicates := []string{
		`{"parameters":{"name":"dup","sense":1,"sense":-1},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`,
		`{"parameters":{"name":"dup","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":1,"value":2}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[]}`,
		`{"parameters":{"name":"dup","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"lowBound":1}],"constraints":[]}`,
		`{"parameters":{"name":"dup","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[{"name":"r","sense":0,"sense":-1,"coefficients":[]}]}`,
		`{"parameters":{"name":"dup","sense":1},"parameters":{"name":"dup2","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`,
	}
	for i, payload := range duplicates {
		raw := []byte(payload)
		if _, ok := parsePuLPJSONFast(raw); ok {
			t.Fatalf("case %d: expected parsePuLPJSONFast to fall back on duplicate key", i)
		}
		_, fastErr := ParsePuLPJSON(raw)
		_, stdErr := parsePuLPJSONStd(raw)
		if (fastErr != nil) != (stdErr != nil) {
			t.Fatalf("case %d: ParsePuLPJSON err=%v vs std err=%v", i, fastErr, stdErr)
		}
		if fastErr == nil {
			t.Fatalf("case %d: expected duplicate key rejection", i)
		}
	}

	// Overflow beyond math.MaxInt (including 4294967297 on 32-bit architectures).
	overflowPayload := `{"parameters":{"name":"ovf","sense":4294967297},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`
	if math.MaxInt <= math.MaxInt32 {
		if _, ok := parsePuLPJSONFast([]byte(overflowPayload)); ok {
			t.Fatal("expected parsePuLPJSONFast to reject sense=4294967297 on 32-bit int")
		}
		if _, err := ParsePuLPJSON([]byte(overflowPayload)); err == nil {
			t.Fatal("expected ParsePuLPJSON error for sense=4294967297 on 32-bit int")
		}
	}
	hugePayload := `{"parameters":{"name":"huge","sense":99999999999999999999},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`
	if _, ok := parsePuLPJSONFast([]byte(hugePayload)); ok {
		t.Fatal("expected parsePuLPJSONFast to reject huge integer overflow")
	}
	if _, err := ParsePuLPJSON([]byte(hugePayload)); err == nil {
		t.Fatal("expected ParsePuLPJSON error on huge integer overflow")
	}

	// Inverted integer bounds [0, -5] must remain Integer rather than Binary across PuLP JSON round-trip.
	m := gb.NewModel("inv_int_rt", gb.Minimize)
	x := m.AddIntVar("x", 0, -5)
	m.SetObjective(gb.VarExpr(x))
	var buf bytes.Buffer
	if err := ToPuLPJSON(m.Compile(), &buf); err != nil {
		t.Fatalf("ToPuLPJSON: %v", err)
	}
	rt, err := FromPuLPJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("FromPuLPJSON: %v", err)
	}
	rtVar, ok := rt.VarByName("x")
	if !ok {
		t.Fatal("round-tripped variable x missing")
	}
	if rt.VarCategoryOf(rtVar) != gb.Integer {
		t.Fatalf("round-tripped inverted integer var category = %v, want Integer", rt.VarCategoryOf(rtVar))
	}
	lb, ub := rt.VarBounds(rtVar)
	if lb != 0 || ub != -5 {
		t.Fatalf("round-tripped bounds = [%v, %v], want [0, -5]", lb, ub)
	}
}

// TestParsePuLPJSONFastZeroPinOutOfOrderAndWhitespace verifies compact corpus parity,
// zero input payload pinning, SWAR whitespace handling, out-of-order sections, and
// UTF-8 or escaped string fallback semantics.
func TestParsePuLPJSONFastZeroPinOutOfOrderAndWhitespace(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "testdata", "corpus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corpus JSON files, got %d (err=%v)", len(matches), err)
	}
	for _, p := range matches {
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			t.Fatalf("read %s: %v", p, rErr)
		}
		// Verify zero payload pinning by overwriting a mutable copy right after parsing.
		mutableCopy := append([]byte(nil), raw...)
		fastDoc, ok := parsePuLPJSONFast(mutableCopy)
		if !ok {
			t.Fatalf("expected parsePuLPJSONFast to succeed on %s", p)
		}
		for i := range mutableCopy {
			mutableCopy[i] = 'X'
		}
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if sErr != nil {
			t.Fatalf("parsePuLPJSONStd failed on %s: %v", p, sErr)
		}
		if !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("post-mutation fast vs std mismatch on %s (payload pinned)", p)
		}

		// Verify compact JSON serialization round-trips through the fast path identically.
		mdl, bErr := BuildModelFromPuLPJSON(stdDoc)
		if bErr != nil {
			t.Fatalf("BuildModelFromPuLPJSON failed on %s: %v", p, bErr)
		}
		var compactBuf bytes.Buffer
		if cErr := ToPuLPJSONCompact(mdl.Compile(), &compactBuf); cErr != nil {
			t.Fatalf("ToPuLPJSONCompact failed on %s: %v", p, cErr)
		}
		compactRaw := compactBuf.Bytes()
		cFast, cOk := parsePuLPJSONFast(compactRaw)
		if !cOk {
			t.Fatalf("expected parsePuLPJSONFast to succeed on compact %s", p)
		}
		cStd, csErr := parsePuLPJSONStd(compactRaw)
		if csErr != nil {
			t.Fatalf("parsePuLPJSONStd failed on compact %s: %v", p, csErr)
		}
		if !reflect.DeepEqual(cFast, cStd) {
			t.Fatalf("compact fast vs std mismatch on %s", p)
		}
	}

	wsPad := strings.Repeat(" ", 40)
	cases := []struct {
		name     string
		raw      string
		wantFast bool
	}{
		{
			name: "heavy_swar_whitespace_and_crlf",
			raw: wsPad + "{\r\n\t\t" + wsPad +
				`"parameters"` + wsPad + `:` + wsPad + `{"name": "ws_test", "sense": 1},` + "\r\n" +
				wsPad + `"objective": {` + "\n" + wsPad + `"coefficients": [` + wsPad + `{"name": "x1", "value": 2.5}], "name": "OBJ"},` + "\n" +
				wsPad + `"variables": [` + wsPad + `{"name": "x1", "cat": "Continuous", "lowBound": 0, "upBound": 10}],` + "\n" +
				wsPad + `"constraints": [` + wsPad + `{"name": "c1", "sense": -1, "coefficients": [` + wsPad + `{"name": "x1", "value": 1.25}], "constant": -5}],` + "\n" +
				wsPad + `"sos1": [], "sos2": []}` + wsPad + "\r\n",
			wantFast: true,
		},
		{
			name:     "out_of_order_constraints_before_variables",
			raw:      `{"constraints":[{"name":"r1","sense":-1,"coefficients":[{"name":"x","value":2},{"name":"y","value":-3.5}],"constant":-4},{"name":"r2","sense":0,"coefficients":[],"constant":0},{"name":"r3","sense":1,"coefficients":null,"constant":1}],"variables":[{"name":"x","cat":"Binary","lowBound":0,"upBound":1},{"name":"y","cat":"CustomCat","lowBound":null,"upBound":5}],"objective":{"coefficients":[{"name":"y","value":7}],"name":""},"parameters":{"name":"reordered","sense":-1},"sos1":[],"sos2":[]}`,
			wantFast: true,
		},
		{
			name:     "variables_before_objective_with_utf8",
			raw:      `{"variables":[{"name":"café_α","cat":"Integer","lowBound":0,"upBound":3}],"objective":{"name":"π_obj","coefficients":[{"name":"café_α","value":1.5}]},"constraints":[{"name":"r_β","sense":0,"coefficients":[{"name":"café_α","value":1},{"name":"missing_var","value":2}],"constant":-1}],"parameters":{"name":"utf8_model","sense":1},"sos1":null,"sos2":[]}`,
			wantFast: true,
		},
		{
			name:     "omitted_names_and_empty_objects",
			raw:      `{"parameters":{},"objective":{},"variables":[{}],"constraints":[{}]}`,
			wantFast: true,
		},
		{
			name:     "escaped_variable_name_falls_back",
			raw:      `{"parameters":{"name":"esc","sense":1},"objective":{"name":"O","coefficients":[{"name":"a\"b","value":1}]},"variables":[{"name":"a\"b","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[],"sos1":[],"sos2":[]}`,
			wantFast: false,
		},
		{
			name:     "invalid_utf8_falls_back",
			raw:      "{\"parameters\":{\"name\":\"bad\xff\",\"sense\":1},\"objective\":{\"name\":\"O\",\"coefficients\":[]},\"variables\":[],\"constraints\":[]}",
			wantFast: false,
		},
	}
	for _, tc := range cases {
		raw := []byte(tc.raw)
		fastDoc, ok := parsePuLPJSONFast(raw)
		if ok != tc.wantFast {
			t.Fatalf("%s: parsePuLPJSONFast ok=%v, want %v", tc.name, ok, tc.wantFast)
		}
		gotDoc, gErr := ParsePuLPJSON(raw)
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if (gErr != nil) != (sErr != nil) {
			t.Fatalf("%s: ParsePuLPJSON err=%v vs std err=%v", tc.name, gErr, sErr)
		}
		if gErr == nil && !reflect.DeepEqual(gotDoc, stdDoc) {
			t.Fatalf("%s: ParsePuLPJSON mismatch:\ngot:  %+v\nwant: %+v", tc.name, gotDoc, stdDoc)
		}
		if ok && !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("%s: fastDoc mismatch", tc.name)
		}
	}
}

// TestFastReadFloatBitExact verifies bit-for-bit IEEE-754 parity between pulpScanner.readFloat
// and strconv.ParseFloat across integer, fractional, boundary, signed-zero, and scientific inputs.
func TestFastReadFloatBitExact(t *testing.T) {
	validTokens := []string{
		"0", "-0", "0.0", "-0.0",
		"0.0000000000000000000000", "-0.0000000000000000000000",
		"0.00000000000000000000000", "-0.00000000000000000000000",
		"1", "-1", "42", "-99999",
		"0.1", "0.2", "0.3", "-0.25", "12.3456789012345",
		"1.0000000000007", "3.14159265358979", "3.141592653589793",
		"9007199254740991", "9007199254740992", "9007199254740993",
		"-9007199254740991", "-9007199254740992", "-9007199254740993",
		"900719925474099.1", "90071992547409.91", "1234567890123456.0",
		"0.0000000000000000000001", "0.00000000000000000000001",
		"1e0", "-1e-14", "1.25e+20", "1e308", "5e-324", "-0e0",
	}
	for _, tok := range validTokens {
		sc := pulpScanner{b: []byte("  " + tok + ",")}
		got, ok := sc.readFloat()
		if !ok {
			t.Fatalf("readFloat(%q) returned ok=false", tok)
		}
		if sc.pos != 2+len(tok) {
			t.Fatalf("readFloat(%q) stopped at pos=%d, want %d", tok, sc.pos, 2+len(tok))
		}
		want, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			t.Fatalf("strconv.ParseFloat(%q): %v", tok, err)
		}
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("readFloat(%q) = %v (%#x), want %v (%#x)",
				tok, got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}

	invalidTokens := []string{"", "-", "+1", "01", "-01", "1.", ".5", "1e", "1e+", "1e9999", "NaN", "Infinity"}
	for _, tok := range invalidTokens {
		sc := pulpScanner{b: []byte(tok)}
		if v, ok := sc.readFloat(); ok {
			t.Fatalf("readFloat(%q) unexpectedly succeeded with %v", tok, v)
		}
	}
}

func referencePuLPJSONDoc(p *gb.CompiledProblem) PuLPJSON {
	numVars := p.NumVars()
	numRows := p.NumConstraints()
	doc := PuLPJSON{
		Parameters: PuLPParameters{
			Name:  p.Name(),
			Sense: int(p.Sense()),
		},
		Variables:   make([]PuLPVariable, numVars),
		Constraints: make([]PuLPConstraint, numRows),
		SOS1:        []any{},
		SOS2:        []any{},
	}
	if objName := p.ObjName(); objName != "" {
		nameCopy := objName
		doc.Objective.Name = &nameCopy
	}
	doc.Objective.Constant = p.ObjConstant()
	doc.Objective.Coefficients = make([]PuLPCoefficient, 0, numVars)
	for j := range numVars {
		vname := p.VarName(j)
		if c := p.ObjCoeff(j); c != 0 {
			doc.Objective.Coefficients = append(doc.Objective.Coefficients, PuLPCoefficient{
				Name:  vname,
				Value: c,
			})
		}
		lb, ub := p.VarBounds(j)
		var lbPtr, ubPtr *float64
		if !math.IsInf(lb, -1) {
			v := lb
			lbPtr = &v
		}
		if !math.IsInf(ub, 1) {
			v := ub
			ubPtr = &v
		}
		catStr := "Continuous"
		if cat := p.VarCategory(j); cat == gb.Integer || cat == gb.Binary {
			catStr = "Integer"
		}
		doc.Variables[j] = PuLPVariable{
			Name:     vname,
			Cat:      catStr,
			LowBound: lbPtr,
			UpBound:  ubPtr,
		}
	}
	for i := range numRows {
		coeffs := make([]PuLPCoefficient, 0, p.RowLen(i))
		p.ForEachRowTerm(i, func(col int, val float64) {
			coeffs = append(coeffs, PuLPCoefficient{
				Name:  p.VarName(col),
				Value: val,
			})
		})
		doc.Constraints[i] = PuLPConstraint{
			Name:         p.RowName(i),
			Sense:        int(p.RowSense(i)),
			Coefficients: coeffs,
			Constant:     -p.RowRHS(i),
		}
	}
	return doc
}

type failAfterWriter struct {
	limit int
	wrote int
	err   error
}

func (f *failAfterWriter) Write(p []byte) (int, error) {
	if f.wrote+len(p) > f.limit {
		return 0, f.err
	}
	f.wrote += len(p)
	return len(p), nil
}

// TestToPuLPJSONExactByteParity verifies that streaming ToPuLPJSON and ToPuLPJSONCompact
// produce byte-for-byte identical output to encoding/json/v2 marshaling of PuLPJSON across
// all corpus files, special-character models, and writer error propagation paths.
func TestToPuLPJSONExactByteParity(t *testing.T) {
	checkProblem := func(t *testing.T, label string, p *gb.CompiledProblem) {
		t.Helper()
		ref := referencePuLPJSONDoc(p)
		var wantPretty, wantCompact bytes.Buffer
		if err := json.MarshalWrite(&wantPretty, &ref, jsontext.WithIndent("  ")); err != nil {
			t.Fatalf("%s: reference pretty marshal: %v", label, err)
		}
		wantPretty.WriteByte('\n')
		if err := json.MarshalWrite(&wantCompact, &ref); err != nil {
			t.Fatalf("%s: reference compact marshal: %v", label, err)
		}
		wantCompact.WriteByte('\n')

		var gotPretty, gotCompact bytes.Buffer
		if err := ToPuLPJSON(p, &gotPretty); err != nil {
			t.Fatalf("%s: ToPuLPJSON: %v", label, err)
		}
		if err := ToPuLPJSONCompact(p, &gotCompact); err != nil {
			t.Fatalf("%s: ToPuLPJSONCompact: %v", label, err)
		}
		if !bytes.Equal(gotPretty.Bytes(), wantPretty.Bytes()) {
			t.Fatalf("%s: ToPuLPJSON byte mismatch\ngot:\n%s\nwant:\n%s", label, gotPretty.String(), wantPretty.String())
		}
		if !bytes.Equal(gotCompact.Bytes(), wantCompact.Bytes()) {
			t.Fatalf("%s: ToPuLPJSONCompact byte mismatch\ngot:\n%s\nwant:\n%s", label, gotCompact.String(), wantCompact.String())
		}
	}

	matches, err := filepath.Glob(filepath.Join("..", "testdata", "corpus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corpus JSON files, got %d (err=%v)", len(matches), err)
	}
	for _, path := range matches {
		raw, rErr := os.ReadFile(path)
		if rErr != nil {
			t.Fatalf("read %s: %v", path, rErr)
		}
		doc, pErr := ParsePuLPJSON(raw)
		if pErr != nil {
			t.Fatalf("ParsePuLPJSON %s: %v", path, pErr)
		}
		mdl, bErr := BuildModelFromPuLPJSON(doc)
		if bErr != nil {
			t.Fatalf("BuildModelFromPuLPJSON %s: %v", path, bErr)
		}
		checkProblem(t, filepath.Base(path), mdl.Compile())
	}

	// Verify synthetic edge cases: special JSON characters, empty constraints, offsets, and zero RHS.
	mSpecial := gb.NewModel("model \"quoted\" \\ \n café", gb.Maximize)
	x := mSpecial.AddContVar("x\"1", math.Inf(-1), math.Inf(1))
	y := mSpecial.AddIntVar("y\\2", -3, 7)
	z := mSpecial.AddBinaryVar("z_bin")
	mSpecial.SetObjectiveNamed("obj\tname", gb.NewExpr(gb.T(x, 1.0+7e-13), gb.T(z, -2.5)).AddConstant(13.25))
	mSpecial.AddConstraint("eq_zero_rhs", gb.Eq(gb.NewExpr(gb.T(x, 1), gb.T(y, -1)), 0))
	mSpecial.AddConstraint("empty_row", gb.Le(gb.Expr{}, 5))
	checkProblem(t, "special_chars_and_offsets", mSpecial.Compile())

	// Verify zero-value CompiledProblem parity.
	checkProblem(t, "zero_value_problem", &gb.CompiledProblem{})

	// Verify mid-stream write errors propagate cleanly without leaving corrupted pooled encoders.
	wantErr := errors.New("injected write failure")
	fw := &failAfterWriter{limit: 64, err: wantErr}
	if err := ToPuLPJSON(mSpecial.Compile(), fw); !errors.Is(err, wantErr) {
		t.Fatalf("ToPuLPJSON write error = %v, want %v", err, wantErr)
	}
	var healthy bytes.Buffer
	if err := ToPuLPJSON(mSpecial.Compile(), &healthy); err != nil {
		t.Fatalf("ToPuLPJSON after failed write returned error: %v", err)
	}
}

// BenchmarkParsePuLPJSON compares the arena-backed fast scanner against the encoding/json/v2
// fallback across Tier 1, Tier 2, and Tier 3 PuLP JSON payloads.
func BenchmarkParsePuLPJSON(b *testing.B) {
	workloads := []struct {
		name string
		file string
	}{
		{"T1_Whiskas", "t1_whiskas.json"},
		{"T2_Lotsizing48", "t2_lotsizing_48.json"},
		{"T3_CFLP300", "t3_cflp_300.json"},
		{"T3_Sched1000", "t3_sched_1000.json"},
	}
	for _, wl := range workloads {
		raw, err := os.ReadFile(filepath.Join("..", "testdata", "corpus", wl.file))
		if err != nil {
			raw, err = os.ReadFile(filepath.Join("testdata", "corpus", wl.file))
		}
		if err != nil {
			b.Fatalf("read %s: %v", wl.file, err)
		}
		b.Run(wl.name+"/pulp_json_fast", func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				doc, ok := parsePuLPJSONFast(raw)
				if !ok || doc == nil {
					b.Fatal("parsePuLPJSONFast failed")
				}
			}
		})
		b.Run(wl.name+"/json_v2", func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				doc, err := parsePuLPJSONStd(raw)
				if err != nil || doc == nil {
					b.Fatal(err)
				}
			}
		})
	}
}
