package format_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

// TestPuLPJSONRoundTrip verifies that a PuLP JSON model serializes and reloads with identical bounds and MPS markers.
func TestPuLPJSONRoundTrip(t *testing.T) {
	rawJSON := `{
  "parameters": {
    "name": "test_prob",
    "sense": -1
  },
  "objective": {
    "name": "profit",
    "coefficients": [
      {"name": "x", "value": 3.5},
      {"name": "y", "value": 2.0}
    ]
  },
  "variables": [
    {"name": "x", "cat": "Integer", "lowBound": 0, "upBound": 10},
    {"name": "y", "cat": "Binary", "lowBound": 0, "upBound": 1},
    {"name": "z", "cat": "Continuous", "lowBound": null, "upBound": 5.5}
  ],
  "constraints": [
    {
      "name": "cap",
      "sense": -1,
      "coefficients": [
        {"name": "x", "value": 2.0},
        {"name": "y", "value": 1.0},
        {"name": "z", "value": 0.5}
      ],
      "constant": -12.5
    }
  ],
  "sos1": [],
  "sos2": []
}`

	m, err := format.FromPuLPJSON(strings.NewReader(rawJSON))
	if err != nil {
		t.Fatalf("FromPuLPJSON failed: %v", err)
	}
	if m.Sense() != gb.Maximize {
		t.Fatalf("expected Maximize sense, got %v", m.Sense())
	}
	p := m.Compile()
	if p.NumVars() != 3 || p.NumConstraints() != 1 {
		t.Fatalf("unexpected compiled size: %d vars, %d rows", p.NumVars(), p.NumConstraints())
	}
	if p.RowRHS(0) != 12.5 {
		t.Fatalf("expected RHS=12.5 (from constant=-12.5), got %v", p.RowRHS(0))
	}
	lb2, ub2 := p.VarBounds(2)
	if !math.IsInf(lb2, -1) || ub2 != 5.5 {
		t.Fatalf("unexpected bounds on z: [%v, %v]", lb2, ub2)
	}

	var buf bytes.Buffer
	if err := format.ToPuLPJSON(p, &buf); err != nil {
		t.Fatalf("ToPuLPJSON failed: %v", err)
	}

	m2, err := format.FromPuLPJSON(&buf)
	if err != nil {
		t.Fatalf("second FromPuLPJSON failed: %v", err)
	}
	p2 := m2.Compile()
	if p2.NumVars() != p.NumVars() || p2.NumConstraints() != p.NumConstraints() || p2.RowRHS(0) != p.RowRHS(0) {
		t.Fatalf("round-trip mismatch")
	}

	var mpsBuf bytes.Buffer
	if err := format.WriteMPS(p2, &mpsBuf); err != nil {
		t.Fatalf("WriteMPS failed: %v", err)
	}
	if !strings.Contains(mpsBuf.String(), "INTORG") {
		t.Fatalf("expected INTORG marker in MPS output:\n%s", mpsBuf.String())
	}
}

// Checks objective constant round-tripping and confirms that the builder rejects
// SOS1, SOS2, empty names, and duplicate variable names.
func TestPuLPJSONObjectiveConstantAndValidation(t *testing.T) {
	for _, sense := range []gb.Sense{gb.Minimize, gb.Maximize} {
		m := gb.NewModel("offset_rt", sense)
		x := m.AddContVar("x", 2, 5)
		m.SetObjective(gb.NewExpr(gb.T(x, 3.0)).AddConstant(42.5))
		m.AddConstraint("c0", gb.Ge(gb.NewExpr(gb.T(x, 1.0)), 2.0))
		p := m.Compile()

		origRes, err := gb.SolveCompiled(t.Context(), p)
		if err != nil || origRes.Status != gb.StatusOptimal {
			t.Fatalf("orig solve failed: status=%v err=%v", origRes.Status, err)
		}

		var buf bytes.Buffer
		if err := format.ToPuLPJSONCompact(p, &buf); err != nil {
			t.Fatalf("ToPuLPJSONCompact failed: %v", err)
		}
		rtModel, err := format.FromPuLPJSON(&buf)
		if err != nil {
			t.Fatalf("FromPuLPJSON failed: %v", err)
		}
		rtRes, err := rtModel.Solve(t.Context())
		if err != nil || rtRes.Status != gb.StatusOptimal || math.Abs(rtRes.Objective-origRes.Objective) > 1e-9 {
			t.Fatalf("sense=%v round-trip objective=%v, want %v (err=%v)", sense, rtRes.Objective, origRes.Objective, err)
		}
	}

	badDocs := []struct {
		name string
		raw  string
	}{
		{
			name: "sos1",
			raw:  `{"parameters":{"name":"s1","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[],"sos1":[{"x":1}]}`,
		},
		{
			name: "sos2",
			raw:  `{"parameters":{"name":"s2","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[],"sos2":[{"x":1}]}`,
		},
		{
			name: "duplicate_var",
			raw:  `{"parameters":{"name":"dup","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1},{"name":"x","cat":"Continuous","lowBound":0,"upBound":2}],"constraints":[]}`,
		},
		{
			name: "empty_var",
			raw:  `{"parameters":{"name":"emp","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[]}`,
		},
		{
			name: "bad_obj_sense",
			raw:  `{"parameters":{"name":"bs","sense":0},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[]}`,
		},
		{
			name: "bad_var_cat",
			raw:  `{"parameters":{"name":"bc","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"integer","lowBound":0,"upBound":1}],"constraints":[]}`,
		},
		{
			name: "bad_con_sense",
			raw:  `{"parameters":{"name":"bcs","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[{"name":"c1","sense":5,"coefficients":[{"name":"x","value":1}],"constant":-1}]}`,
		},
		{
			name: "dup_con_name",
			raw:  `{"parameters":{"name":"dc","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[{"name":"c1","sense":-1,"coefficients":[{"name":"x","value":1}],"constant":-1},{"name":"c1","sense":-1,"coefficients":[{"name":"x","value":1}],"constant":-2}]}`,
		},
	}
	for _, tc := range badDocs {
		if _, err := format.FromPuLPJSON(strings.NewReader(tc.raw)); err == nil {
			t.Fatalf("expected error on %s, got nil", tc.name)
		}
	}

	// Verify omitted lowBound defaults to 0 while explicit null maps to -Inf.
	omittedLB := `{"parameters":{"name":"omit_lb","sense":1},"objective":{"name":"OBJ","coefficients":[{"name":"x","value":1}]},"variables":[{"name":"x","cat":"Continuous","upBound":10},{"name":"z","cat":"Continuous","lowBound":null,"upBound":5}],"constraints":[]}`
	mOmit, err := format.FromPuLPJSON(strings.NewReader(omittedLB))
	if err != nil {
		t.Fatalf("FromPuLPJSON omitted lowBound failed: %v", err)
	}
	pOmit := mOmit.Compile()
	lb0, ub0 := pOmit.VarBounds(0)
	if lb0 != 0 || ub0 != 10 {
		t.Fatalf("expected omitted lowBound -> [0, 10], got [%v, %v]", lb0, ub0)
	}
	lb1, ub1 := pOmit.VarBounds(1)
	if !math.IsInf(lb1, -1) || ub1 != 5 {
		t.Fatalf("expected explicit null lowBound -> [-Inf, 5], got [%v, %v]", lb1, ub1)
	}

	if err := format.ToPuLPJSON(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when serializing nil CompiledProblem to PuLP JSON")
	}
}

// Verifies lossless float output, objective constant RHS entries, and errors on
// whitespace or colliding MPS names.
func TestWriteMPSPrecisionOffsetAndValidation(t *testing.T) {
	exactVal := 1.0 + 7e-13
	m := gb.NewModel("mps_prec", gb.Maximize)
	x := m.AddContVar("x", 1, 4)
	m.SetObjective(gb.NewExpr(gb.T(x, exactVal)).AddConstant(42.5))
	m.AddConstraint("c0", gb.Le(gb.NewExpr(gb.T(x, 1.0)), 4.0))

	var mpsBuf bytes.Buffer
	if err := format.WriteMPS(m.Compile(), &mpsBuf); err != nil {
		t.Fatalf("WriteMPS failed: %v", err)
	}
	out := mpsBuf.String()
	if !strings.Contains(out, "1.0000000000007") {
		t.Fatalf("WriteMPS truncated 17-digit precision:\n%s", out)
	}
	if !strings.Contains(out, "42.5") {
		t.Fatalf("WriteMPS omitted objective constant RHS:\n%s", out)
	}

	mSpace := gb.NewModel("mps_bad_var", gb.Minimize)
	vBad := mSpace.AddContVar("bad name", 0, 1)
	mSpace.SetObjective(gb.NewExpr(gb.T(vBad, 1.0)))
	if err := format.WriteMPS(mSpace.Compile(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected WriteMPS error on variable name containing whitespace")
	}

	mCollide := gb.NewModel("mps_collide", gb.Minimize)
	vOk := mCollide.AddContVar("x", 0, 1)
	mCollide.SetObjective(gb.NewExpr(gb.T(vOk, 1.0)))
	mCollide.AddConstraint("OBJ", gb.Le(gb.NewExpr(gb.T(vOk, 1.0)), 1.0))
	if err := format.WriteMPS(mCollide.Compile(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected WriteMPS error when row name collides with objective name OBJ")
	}

	// Unused columns must still appear in COLUMNS, and lb=0, ub<0 must emit explicit LO BND 0.
	mUnused := gb.NewModel("mps_unused", gb.Minimize)
	_ = mUnused.AddIntVar("unused_int", 1, 5)
	_ = mUnused.AddContVar("infeas_neg_ub", 0, -1)
	yUsed := mUnused.AddContVar("y", 0, 10)
	mUnused.SetObjective(gb.VarExpr(yUsed))
	mUnused.AddConstraint("c", gb.VarExpr(yUsed).Ge(2))
	var unusedBuf bytes.Buffer
	if err := format.WriteMPS(mUnused.Compile(), &unusedBuf); err != nil {
		t.Fatalf("WriteMPS unused failed: %v", err)
	}
	unusedMPS := unusedBuf.String()
	if !strings.Contains(unusedMPS, "unused_int OBJ") {
		t.Fatalf("expected unused_int in COLUMNS section:\n%s", unusedMPS)
	}
	if !strings.Contains(unusedMPS, "LO BND       infeas_neg_ub") {
		t.Fatalf("expected explicit LO BND 0 for lb=0, ub=-1:\n%s", unusedMPS)
	}

	// Reserved 'MARKER' token must be rejected as a variable or row identifier.
	mMarkerVar := gb.NewModel("mps_marker_var", gb.Minimize)
	mv := mMarkerVar.AddIntVar("'MARKER'", 0, 1)
	mMarkerVar.SetObjective(gb.VarExpr(mv))
	if err := format.WriteMPS(mMarkerVar.Compile(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected WriteMPS error on reserved 'MARKER' variable name")
	}
	mMarkerRow := gb.NewModel("mps_marker_row", gb.Minimize)
	mr := mMarkerRow.AddContVar("x", 0, 1)
	mMarkerRow.SetObjective(gb.VarExpr(mr))
	mMarkerRow.AddConstraint("'MARKER'", gb.VarExpr(mr).Le(1))
	if err := format.WriteMPS(mMarkerRow.Compile(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected WriteMPS error on reserved 'MARKER' row name")
	}

	// Identifiers starting with '$' or '*' must be rejected so MPS comment prefixes cannot truncate cards.
	for _, badPrefix := range []string{"$", "*"} {
		mBadProb := gb.NewModel(badPrefix+"prob", gb.Minimize)
		xp := mBadProb.AddContVar("x", 0, 1)
		mBadProb.SetObjective(gb.VarExpr(xp))
		if err := format.WriteMPS(mBadProb.Compile(), &bytes.Buffer{}); err == nil {
			t.Fatalf("expected WriteMPS error on problem name starting with %q", badPrefix)
		}

		mBadObj := gb.NewModel("mps_bad_obj", gb.Minimize)
		xo := mBadObj.AddContVar("x", 0, 1)
		mBadObj.SetObjectiveNamed(badPrefix+"obj", gb.VarExpr(xo))
		if err := format.WriteMPS(mBadObj.Compile(), &bytes.Buffer{}); err == nil {
			t.Fatalf("expected WriteMPS error on objective name starting with %q", badPrefix)
		}

		mBadVar := gb.NewModel("mps_bad_var_pfx", gb.Minimize)
		xv := mBadVar.AddContVar(badPrefix+"x", 0, 1)
		mBadVar.SetObjective(gb.VarExpr(xv))
		if err := format.WriteMPS(mBadVar.Compile(), &bytes.Buffer{}); err == nil {
			t.Fatalf("expected WriteMPS error on variable name starting with %q", badPrefix)
		}

		mBadRow := gb.NewModel("mps_bad_row_pfx", gb.Minimize)
		xr := mBadRow.AddContVar("x", 0, 1)
		mBadRow.SetObjective(gb.VarExpr(xr))
		mBadRow.AddConstraint(badPrefix+"cap", gb.VarExpr(xr).Le(5))
		if err := format.WriteMPS(mBadRow.Compile(), &bytes.Buffer{}); err == nil {
			t.Fatalf("expected WriteMPS error on row name starting with %q", badPrefix)
		}
	}

	// Verify fixed-MPS 1-based column offsets (5, 15, 25 -> 0-based 4, 14, 24) for <=8-char names
	// and space-separated fallback when names exceed 8 characters.
	mAlign := gb.NewModel("mps_align", gb.Minimize)
	xShort := mAlign.AddContVar("x1", 2.5, 9.5)
	xLong := mAlign.AddContVar("long_var_01", 1.25, 4.75)
	mAlign.SetObjective(gb.NewExpr(xShort.T(3.5), xLong.T(-2.0)))
	mAlign.AddConstraint("r1", gb.NewExpr(xShort.T(1.5), xLong.T(0.5)).Le(12.5))
	mAlign.AddConstraint("long_row_01", gb.VarExpr(xShort).Ge(3.0))
	var alignBuf bytes.Buffer
	if err := format.WriteMPS(mAlign.Compile(), &alignBuf); err != nil {
		t.Fatalf("WriteMPS align failed: %v", err)
	}
	alignMPS := alignBuf.String()
	for _, wantLine := range []string{
		"    x1        OBJ       3.5",
		"    x1        r1        1.5",
		"    x1        long_row_01 1",
		"    long_var_01 OBJ       -2",
		"    RHS       r1        12.5",
		"    RHS       long_row_01 3",
		" LO BND       x1        2.5",
		" UP BND       x1        9.5",
		" LO BND       long_var_01 1.25",
	} {
		if !strings.Contains(alignMPS, wantLine+"\n") {
			t.Fatalf("missing aligned MPS line %q in:\n%s", wantLine, alignMPS)
		}
	}
	fixedColLine := "    x1        r1        1.5"
	if strings.Index(fixedColLine, "x1") != 4 || strings.Index(fixedColLine, "r1") != 14 || strings.Index(fixedColLine, "1.5") != 24 {
		t.Fatalf("unexpected fixed-MPS column offsets in %q", fixedColLine)
	}

	// Sub-tolerance bound conflict (lb = 5 + 1e-9 > ub = 5) must also normalize to (0, 1).
	mConfTiny := gb.NewModel("mps_conf_tiny", gb.Minimize)
	xConfTiny := mConfTiny.AddContVar("x_tiny", 5.0+1e-9, 5.0)
	mConfTiny.SetObjective(gb.VarExpr(xConfTiny))
	var confTinyBuf bytes.Buffer
	if err := format.WriteMPS(mConfTiny.Compile(), &confTinyBuf); err != nil {
		t.Fatalf("WriteMPS tiny conflicting bounds failed: %v", err)
	}
	if !strings.Contains(confTinyBuf.String(), " LO BND       x_tiny    1\n") {
		t.Fatalf("expected normalized LO BND 1 for tiny bound conflict:\n%s", confTinyBuf.String())
	}

	// Conflicting integer bounds (lb > ub) must lower cleanly without emitting invalid UI < LI BOUNDS.
	mConfInt := gb.NewModel("mps_conf_int", gb.Minimize)
	xConf := mConfInt.AddIntVar("x_conf", 5, 3)
	mConfInt.SetObjective(gb.VarExpr(xConf))
	var confBuf bytes.Buffer
	if err := format.WriteMPS(mConfInt.Compile(), &confBuf); err != nil {
		t.Fatalf("WriteMPS conflicting integer bounds failed: %v", err)
	}
	if strings.Contains(confBuf.String(), "UI BND       x_conf") {
		t.Fatalf("WriteMPS emitted invalid UI < LI bound:\n%s", confBuf.String())
	}
	cbcBin := filepath.Join("..", "third_party", "Cbc", "dist", "bin", "cbc")
	if _, err := os.Stat(cbcBin); err == nil {
		mpsFile := filepath.Join(t.TempDir(), "conf_int.mps")
		if err := os.WriteFile(mpsFile, confBuf.Bytes(), 0o600); err != nil {
			t.Fatalf("write temp MPS: %v", err)
		}
		out, err := exec.CommandContext(t.Context(), cbcBin, mpsFile, "-solve").CombinedOutput()
		if err != nil {
			t.Fatalf("cbc failed on conflicting-bound MPS: %v\n%s", err, string(out))
		}
		lowerOut := strings.ToLower(string(out))
		if strings.Contains(lowerOut, "errors on input") || !strings.Contains(lowerOut, "infeasible") {
			t.Fatalf("expected clean Infeasible from cbc on conflicting-bound MPS, got:\n%s", string(out))
		}

		mConfBig := gb.NewModel("mps_conf_big", gb.Minimize)
		xConfBig := mConfBig.AddContVar("x_big", 1e305, 1e304)
		mConfBig.SetObjective(gb.VarExpr(xConfBig))
		var confBigBuf bytes.Buffer
		if err := format.WriteMPS(mConfBig.Compile(), &confBigBuf); err != nil {
			t.Fatalf("WriteMPS conflicting 1e305 > 1e304 bounds failed: %v", err)
		}
		mpsBigFile := filepath.Join(t.TempDir(), "conf_big.mps")
		if err := os.WriteFile(mpsBigFile, confBigBuf.Bytes(), 0o600); err != nil {
			t.Fatalf("write temp big MPS: %v", err)
		}
		outBig, err := exec.CommandContext(t.Context(), cbcBin, mpsBigFile, "-solve", "-quit").CombinedOutput()
		if err != nil {
			t.Fatalf("cbc failed on conflicting 1e305 > 1e304 MPS: %v\n%s", err, string(outBig))
		}
		lowerBig := strings.ToLower(string(outBig))
		if strings.Contains(lowerBig, "errors on input") || !strings.Contains(lowerBig, "infeasible") {
			t.Fatalf("expected clean Infeasible from cbc on conflicting 1e305 > 1e304 MPS, got:\n%s", string(outBig))
		}

		mpsTinyFile := filepath.Join(t.TempDir(), "conf_tiny.mps")
		if err := os.WriteFile(mpsTinyFile, confTinyBuf.Bytes(), 0o600); err != nil {
			t.Fatalf("write temp tiny MPS: %v", err)
		}
		outTiny, err := exec.CommandContext(t.Context(), cbcBin, mpsTinyFile, "-solve", "-quit").CombinedOutput()
		if err != nil {
			t.Fatalf("cbc failed on conflicting 5+1e-9 > 5 MPS: %v\n%s", err, string(outTiny))
		}
		lowerTiny := strings.ToLower(string(outTiny))
		if strings.Contains(lowerTiny, "errors on input") || !strings.Contains(lowerTiny, "infeasible") {
			t.Fatalf("expected clean Infeasible from cbc on conflicting 5+1e-9 > 5 MPS, got:\n%s", string(outTiny))
		}
	}
}

// Verifies that WriteMPS emits the *SENSE:Maximize comment card on line 1 while
// COLUMNS and RHS already negate Maximize objectives into standard minimization form.
func TestWriteMPSMaximizeMinimizationForm(t *testing.T) {
	buildAndWrite := func(sense gb.Sense) string {
		m := gb.NewModel("mps_sense_form", sense)
		x := m.AddContVar("x", 0, 5)
		y := m.AddIntVar("y", 1, 4)
		m.SetObjective(gb.NewExpr(x.T(3.5), y.T(-2.0)).AddConstant(42.5))
		m.AddConstraint("cap", gb.NewExpr(x.T(1.0), y.T(2.0)).Le(10.0))

		var buf bytes.Buffer
		if err := format.WriteMPS(m.Compile(), &buf); err != nil {
			t.Fatalf("WriteMPS(sense=%v) failed: %v", sense, err)
		}
		return buf.String()
	}

	maxMPS := buildAndWrite(gb.Maximize)
	maxLines := strings.Split(maxMPS, "\n")
	if len(maxLines) == 0 || maxLines[0] != "*SENSE:Maximize" {
		t.Fatalf("line 1 = %q, want *SENSE:Maximize comment card", maxLines[0])
	}
	if strings.Contains(maxMPS, "OBJSENSE") {
		t.Fatalf("unexpected OBJSENSE card in PuLP-compatible MPS:\n%s", maxMPS)
	}
	for _, want := range []string{
		"    x         OBJ       -3.5\n",
		"    y         OBJ       2\n",
		"    RHS       OBJ       42.5\n",
		"    RHS       cap       10\n",
	} {
		if !strings.Contains(maxMPS, want) {
			t.Fatalf("Maximize MPS missing negated minimization card %q:\n%s", want, maxMPS)
		}
	}
	if strings.Contains(maxMPS, "    x         OBJ       3.5\n") || strings.Contains(maxMPS, "    RHS       OBJ       -42.5\n") {
		t.Fatalf("Maximize MPS emitted unnegated objective card:\n%s", maxMPS)
	}

	minMPS := buildAndWrite(gb.Minimize)
	minLines := strings.Split(minMPS, "\n")
	if len(minLines) == 0 || minLines[0] != "*SENSE:Minimize" {
		t.Fatalf("line 1 = %q, want *SENSE:Minimize comment card", minLines[0])
	}
	for _, want := range []string{
		"    x         OBJ       3.5\n",
		"    y         OBJ       -2\n",
		"    RHS       OBJ       -42.5\n",
	} {
		if !strings.Contains(minMPS, want) {
			t.Fatalf("Minimize MPS missing objective card %q:\n%s", want, minMPS)
		}
	}
}

// Checks non-standard JSON floats, signed zeros, and explicit binary bounds.
// Specifically, the fast scanner must skip Python `NaN` and `Infinity` tokens on primal/dual
// outputs, preserve `-0.0` sign bits on input numbers, and honor explicit bounds on `Binary`
// entries while still rejecting non-finite objective or row numbers.
func TestPuLPJSONEdgeCases(t *testing.T) {
	// Python json.dumps emits bare `NaN`, `Infinity`, and `-Infinity` on `varValue`, `dj`, and `pi`.
	rawPyTokens := `{
  "parameters": {"name": "py_nan", "sense": 1},
  "objective": {"name": "OBJ", "coefficients": [{"name": "x", "value": 2.0}]},
  "variables": [
    {"name": "x", "cat": "Binary", "lowBound": 0, "upBound": 0, "varValue": NaN, "dj": Infinity},
    {"name": "y", "cat": "Binary", "lowBound": 0, "upBound": 5, "varValue": -Infinity, "dj": -0.0}
  ],
  "constraints": [
    {"name": "c0", "sense": 1, "coefficients": [{"name": "y", "value": 1.0}], "constant": -3.0, "pi": NaN}
  ],
  "sos1": [],
  "sos2": []
}`
	doc, err := format.ParsePuLPJSON([]byte(rawPyTokens))
	if err != nil {
		t.Fatalf("ParsePuLPJSON failed on Python NaN/Infinity solution tokens: %v", err)
	}
	m, err := format.BuildModelFromPuLPJSON(doc)
	if err != nil {
		t.Fatalf("BuildModelFromPuLPJSON failed: %v", err)
	}
	p, err := m.CompileChecked()
	if err != nil {
		t.Fatalf("CompileChecked failed: %v", err)
	}
	lb0, ub0 := p.VarBounds(0)
	if lb0 != 0 || ub0 != 0 {
		t.Fatalf("Binary var with upBound=0 got bounds [%v, %v], want [0, 0]", lb0, ub0)
	}
	lb1, ub1 := p.VarBounds(1)
	if lb1 != 0 || ub1 != 5 || p.VarCategory(1) != gb.Integer {
		t.Fatalf("Binary var with explicit [0, 5] got bounds [%v, %v] cat=%v, want [0, 5] Integer",
			lb1, ub1, p.VarCategory(1))
	}

	// Fast JSON scanner preserves `-0.0` sign bits identically to `encoding/json/v2`.
	negZeroJSON := `{"parameters":{"name":"nz","sense":1},"objective":{"name":"OBJ","coefficients":[{"name":"x","value":-0.0}]},"variables":[{"name":"x","cat":"Continuous","lowBound":-0.0,"upBound":1.0}],"constraints":[]}`
	nzDoc, err := format.ParsePuLPJSON([]byte(negZeroJSON))
	if err != nil {
		t.Fatalf("ParsePuLPJSON(-0.0) failed: %v", err)
	}
	if nzDoc.Variables[0].LowBound == nil || !math.Signbit(*nzDoc.Variables[0].LowBound) {
		t.Fatalf("expected LowBound=-0.0 signbit preserved, got %v", nzDoc.Variables[0].LowBound)
	}
	if len(nzDoc.Objective.Coefficients) != 1 || !math.Signbit(nzDoc.Objective.Coefficients[0].Value) {
		t.Fatalf("expected objective coeff -0.0 signbit preserved")
	}

	// Structural bounds and weights must still reject non-finite tokens.
	badPyCoef := `{"parameters":{"name":"bad","sense":1},"objective":{"name":"OBJ","coefficients":[{"name":"x","value":NaN}]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[]}`
	if _, err := format.ParsePuLPJSON([]byte(badPyCoef)); err == nil {
		t.Fatal("expected ParsePuLPJSON to reject NaN in objective coefficient")
	}

	// MIN-1: Zero-value 0-variable Model and CompiledProblem default to "NoName" and Minimize,
	// round-tripping cleanly through ToPuLPJSON and FromPuLPJSON.
	var zeroM gb.Model
	if zeroM.Name() != "NoName" || zeroM.Sense() != gb.Minimize {
		t.Fatalf("zero-value Model Name=%q Sense=%v, want NoName Minimize", zeroM.Name(), zeroM.Sense())
	}
	zeroM.SetObjective(gb.Expr{Constant: 7.5})
	pZero := zeroM.Compile()
	if pZero.Name() != "NoName" || pZero.Sense() != gb.Minimize {
		t.Fatalf("compiled zero-value Model Name=%q Sense=%v, want NoName Minimize", pZero.Name(), pZero.Sense())
	}
	for _, pCase := range []*gb.CompiledProblem{pZero, {}} {
		var zBuf bytes.Buffer
		if err := format.ToPuLPJSON(pCase, &zBuf); err != nil {
			t.Fatalf("ToPuLPJSON zero-value problem failed: %v", err)
		}
		mRT, err := format.FromPuLPJSON(&zBuf)
		if err != nil {
			t.Fatalf("FromPuLPJSON zero-value round-trip failed: %v", err)
		}
		if mRT.Sense() != gb.Minimize || mRT.Name() != "NoName" {
			t.Fatalf("FromPuLPJSON zero-value round-trip Name=%q Sense=%v, want NoName Minimize", mRT.Name(), mRT.Sense())
		}
	}
}

func referenceMPSNameField(s string) string {
	if len(s) <= 8 {
		return s + strings.Repeat(" ", 10-len(s))
	}
	return s + " "
}

func referenceWriteMPS(p *gb.CompiledProblem, w io.Writer) error {
	probName := p.Name()
	objName := p.ObjName()
	if objName == "" {
		objName = "OBJ"
	}
	numRows := p.NumConstraints()
	numVars := p.NumVars()
	seenRows := make(map[string]struct{}, numRows)
	for i := range numRows {
		seenRows[p.RowName(i)] = struct{}{}
	}
	var conflictRows []string
	conflictRowByCol := make(map[int]string)
	for j := range numVars {
		lb, ub := p.VarBounds(j)
		if lb > ub {
			rname := fmt.Sprintf("_INFEAS_UB_%d", j)
			for idx := 0; ; idx++ {
				if idx > 0 {
					rname = fmt.Sprintf("_INFEAS_UB_%d_%d", j, idx)
				}
				if rname != objName {
					if _, exists := seenRows[rname]; !exists {
						break
					}
				}
			}
			seenRows[rname] = struct{}{}
			conflictRows = append(conflictRows, rname)
			conflictRowByCol[j] = rname
		}
	}
	fmtFloat := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	sense := p.Sense()
	_, _ = fmt.Fprintf(w, "*SENSE:%s\n", sense.String())
	_, _ = fmt.Fprintf(w, "NAME          %s\n", probName)
	_, _ = io.WriteString(w, "ROWS\n")
	_, _ = fmt.Fprintf(w, " N  %s\n", objName)
	for i := range numRows {
		senseChar := "E"
		switch p.RowSense(i) {
		case gb.LessEqual:
			senseChar = "L"
		case gb.GreaterEqual:
			senseChar = "G"
		}
		_, _ = fmt.Fprintf(w, " %s  %s\n", senseChar, p.RowName(i))
	}
	for _, rname := range conflictRows {
		_, _ = fmt.Fprintf(w, " L  %s\n", rname)
	}
	_, _ = io.WriteString(w, "COLUMNS\n")
	inIntBlock := false
	for j := range numVars {
		isInt := p.VarCategory(j) != gb.Continuous
		if isInt && !inIntBlock {
			_, _ = io.WriteString(w, "    MARK      'MARKER'                 'INTORG'\n")
			inIntBlock = true
		} else if !isInt && inIntBlock {
			_, _ = io.WriteString(w, "    MARKEND   'MARKER'                 'INTEND'\n")
			inIntBlock = false
		}
		vField := referenceMPSNameField(p.VarName(j))
		hasEntry := false
		if objMin := p.ObjMinCoeff(j); objMin != 0 {
			_, _ = fmt.Fprintf(w, "    %s%s%s\n", vField, referenceMPSNameField(objName), fmtFloat(objMin))
			hasEntry = true
		}
		p.ForEachColTerm(j, func(r int, val float64) {
			_, _ = fmt.Fprintf(w, "    %s%s%s\n", vField, referenceMPSNameField(p.RowName(r)), fmtFloat(val))
			hasEntry = true
		})
		if crName, ok := conflictRowByCol[j]; ok {
			_, _ = fmt.Fprintf(w, "    %s%s1\n", vField, referenceMPSNameField(crName))
			hasEntry = true
		}
		if !hasEntry {
			_, _ = fmt.Fprintf(w, "    %s%s0\n", vField, referenceMPSNameField(objName))
		}
	}
	if inIntBlock {
		_, _ = io.WriteString(w, "    MARKEND   'MARKER'                 'INTEND'\n")
	}
	_, _ = io.WriteString(w, "RHS\n")
	if objConst := p.ObjConstant(); objConst != 0 {
		senseSign := 1.0
		if sense == gb.Maximize {
			senseSign = -1.0
		}
		_, _ = fmt.Fprintf(w, "    RHS       %s%s\n", referenceMPSNameField(objName), fmtFloat(-senseSign*objConst))
	}
	for i := range numRows {
		if rhs := p.RowRHS(i); rhs != 0 {
			_, _ = fmt.Fprintf(w, "    RHS       %s%s\n", referenceMPSNameField(p.RowName(i)), fmtFloat(rhs))
		}
	}
	_, _ = io.WriteString(w, "BOUNDS\n")
	for j := range numVars {
		vname := p.VarName(j)
		vField := referenceMPSNameField(vname)
		lb, ub := p.VarBounds(j)
		cat := p.VarCategory(j)
		if _, isConflict := conflictRowByCol[j]; isConflict {
			bndType := "LO"
			if cat != gb.Continuous {
				bndType = "LI"
			}
			_, _ = fmt.Fprintf(w, " %s BND       %s1\n", bndType, vField)
			if cat != gb.Continuous {
				_, _ = fmt.Fprintf(w, " PL BND       %s\n", vname)
			}
			continue
		}
		if cat == gb.Binary && lb == 0 && ub == 1 {
			_, _ = fmt.Fprintf(w, " BV BND       %s\n", vname)
			continue
		}
		if math.IsInf(lb, -1) && math.IsInf(ub, 1) {
			_, _ = fmt.Fprintf(w, " FR BND       %s\n", vname)
			continue
		}
		if lb == ub {
			_, _ = fmt.Fprintf(w, " FX BND       %s%s\n", vField, fmtFloat(lb))
			continue
		}
		if math.IsInf(lb, -1) {
			_, _ = fmt.Fprintf(w, " MI BND       %s\n", vname)
		} else if lb != 0 || ub < 0 || cat != gb.Continuous {
			bndType := "LO"
			if cat != gb.Continuous {
				bndType = "LI"
			}
			_, _ = fmt.Fprintf(w, " %s BND       %s%s\n", bndType, vField, fmtFloat(lb))
		}
		if !math.IsInf(ub, 1) {
			bndType := "UP"
			if cat != gb.Continuous {
				bndType = "UI"
			}
			_, _ = fmt.Fprintf(w, " %s BND       %s%s\n", bndType, vField, fmtFloat(ub))
		} else if cat != gb.Continuous {
			_, _ = fmt.Fprintf(w, " PL BND       %s\n", vname)
		}
	}
	_, _ = io.WriteString(w, "ENDATA\n")
	return nil
}

type failingMPSWriter struct {
	limit int
	wrote int
	err   error
}

func (f *failingMPSWriter) Write(p []byte) (int, error) {
	if f.wrote+len(p) > f.limit {
		return 0, f.err
	}
	f.wrote += len(p)
	return len(p), nil
}

// TestWriteMPSByteForByteCompatibility confirms that buffered WriteMPS produces byte-for-byte
// identical output to the reference fmt.Fprintf implementation across all corpus models,
// every MPS bound type, conflicting bounds, long identifiers, and chunk flush boundaries.
func TestWriteMPSByteForByteCompatibility(t *testing.T) {
	verify := func(t *testing.T, label string, p *gb.CompiledProblem) {
		t.Helper()
		var got, want bytes.Buffer
		if err := format.WriteMPS(p, &got); err != nil {
			t.Fatalf("%s: WriteMPS failed: %v", label, err)
		}
		if err := referenceWriteMPS(p, &want); err != nil {
			t.Fatalf("%s: referenceWriteMPS failed: %v", label, err)
		}
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatalf("%s: WriteMPS byte mismatch (got %d bytes, want %d bytes)", label, got.Len(), want.Len())
		}
	}

	matches, err := filepath.Glob(filepath.Join("..", "testdata", "corpus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corpus JSON files, got %d (err=%v)", len(matches), err)
	}
	for _, path := range matches {
		f, oErr := os.Open(path)
		if oErr != nil {
			t.Fatalf("open %s: %v", path, oErr)
		}
		mdl, pErr := format.FromPuLPJSON(f)
		_ = f.Close()
		if pErr != nil {
			t.Fatalf("FromPuLPJSON %s: %v", path, pErr)
		}
		verify(t, filepath.Base(path), mdl.Compile())
	}

	// Exercise every bound classification, marker transition, long name, and conflict row.
	m := gb.NewModel("mps_all_bounds", gb.Maximize)
	vFree := m.AddContVar("v_free", math.Inf(-1), math.Inf(1))
	vFixed := m.AddContVar("v_fixed_long_name_01", 3.25, 3.25)
	vNegInf := m.AddContVar("v_mi", math.Inf(-1), -2.5)
	vNegUB := m.AddContVar("v_negub", 0, -4.0)
	vBin := m.AddBinaryVar("b0")
	vIntPL := m.AddIntVar("i_pl_unbounded", 2, math.Inf(1))
	vIntBox := m.AddIntVar("i_box", -3, 9)
	_ = m.AddContVar("unused_cont", 0, 10)
	_ = m.AddIntVar("unused_int_trailing", 1, 5)
	vConfSmall := m.AddContVar("c_conf_small", 5.0+1e-9, 5.0)
	vConfInt := m.AddIntVar("i_conf_large", 8, 2)
	m.SetObjectiveNamed("PROFIT_OBJ_LONG", gb.NewExpr(
		gb.T(vFree, 1.0+7e-13),
		gb.T(vBin, -1.25e-14),
		gb.T(vIntPL, 2.5),
		gb.T(vConfSmall, 1.0),
		gb.T(vConfInt, -1.0),
	).AddConstant(17.5))
	m.AddConstraint("r_le_very_long_name", gb.Le(gb.NewExpr(gb.T(vFree, 1), gb.T(vFixed, 2)), 10))
	m.AddConstraint("r_eq", gb.Eq(gb.NewExpr(gb.T(vNegInf, -1), gb.T(vNegUB, 1)), 0))
	m.AddConstraint("r_ge", gb.Ge(gb.NewExpr(gb.T(vBin, 3), gb.T(vIntBox, -2)), -5))
	m.AddConstraint("_INFEAS_UB_9", gb.Le(gb.VarExpr(vFree), 100))
	verify(t, "all_bounds_and_conflicts", m.Compile())

	// Verify writer errors propagate cleanly across chunk flushes.
	wantErr := errors.New("mps write failure")
	fw := &failingMPSWriter{limit: 32, err: wantErr}
	if err := format.WriteMPS(m.Compile(), fw); !errors.Is(err, wantErr) {
		t.Fatalf("WriteMPS error = %v, want %v", err, wantErr)
	}
}

func isRaceBuild() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range info.Settings {
		if s.Key == "-race" && s.Value == "true" {
			return true
		}
	}
	return false
}

// TestCodecLowAllocations guards against allocation regressions in ParsePuLPJSON,
// WriteMPS, ToPuLPJSON, and ToPuLPJSONCompact on a Tier 3 corpus model.
func TestCodecLowAllocations(t *testing.T) {
	if isRaceBuild() {
		t.Skip("allocation counts are inflated under -race because sync.Pool drops entries")
	}
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "corpus", "t3_sched_1000.json"))
	if err != nil {
		t.Fatalf("read t3_sched_1000.json: %v", err)
	}
	doc, err := format.ParsePuLPJSON(raw)
	if err != nil {
		t.Fatalf("ParsePuLPJSON: %v", err)
	}
	mdl, err := format.BuildModelFromPuLPJSON(doc)
	if err != nil {
		t.Fatalf("BuildModelFromPuLPJSON: %v", err)
	}
	p := mdl.Compile()

	parseAllocs := testing.AllocsPerRun(5, func() {
		if _, pErr := format.ParsePuLPJSON(raw); pErr != nil {
			t.Fatalf("ParsePuLPJSON: %v", pErr)
		}
	})
	if parseAllocs > 30 {
		t.Fatalf("ParsePuLPJSON allocs/run = %.1f, want <= 30", parseAllocs)
	}

	var mpsBuf bytes.Buffer
	mpsBuf.Grow(512 * 1024)
	mpsAllocs := testing.AllocsPerRun(5, func() {
		mpsBuf.Reset()
		if wErr := format.WriteMPS(p, &mpsBuf); wErr != nil {
			t.Fatalf("WriteMPS: %v", wErr)
		}
	})
	if mpsAllocs > 35 {
		t.Fatalf("WriteMPS allocs/run = %.1f, want <= 35", mpsAllocs)
	}

	var jsonBuf bytes.Buffer
	jsonBuf.Grow(512 * 1024)
	compactAllocs := testing.AllocsPerRun(5, func() {
		jsonBuf.Reset()
		if wErr := format.ToPuLPJSONCompact(p, &jsonBuf); wErr != nil {
			t.Fatalf("ToPuLPJSONCompact: %v", wErr)
		}
	})
	if compactAllocs > 15 {
		t.Fatalf("ToPuLPJSONCompact allocs/run = %.1f, want <= 15", compactAllocs)
	}

	prettyAllocs := testing.AllocsPerRun(5, func() {
		jsonBuf.Reset()
		if wErr := format.ToPuLPJSON(p, &jsonBuf); wErr != nil {
			t.Fatalf("ToPuLPJSON: %v", wErr)
		}
	})
	if prettyAllocs > 15 {
		t.Fatalf("ToPuLPJSON allocs/run = %.1f, want <= 15", prettyAllocs)
	}
}

func TestFromPuLPJSONExactBytesAndReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Fast unescaped path: omitted lowBound=0, null upBound=+Inf, non-dyadic decimals, obj constant.
	// Minimizing 2*x1 + x2 + x3 + 0.1 s.t. -7.2*x1 - 4.1*x2 = -1, x1,x2>=0, x3>=0 yields
	// exact x* = (0, 10/41, 0) and objective 10/41 + 1/10 = 141/410.
	fastJSON := `{
  "parameters": {"name": "q10_fast", "sense": 1, "status": 0, "sol_status": 0},
  "objective": {
    "name": "cost",
    "coefficients": [{"name": "x1", "value": 2}, {"name": "x2", "value": 1}, {"name": "x3", "value": 1.5e-3}],
    "constant": 0.1
  },
  "variables": [
    {"name": "x1", "cat": "Continuous", "upBound": null, "varValue": null, "dj": null},
    {"name": "x2", "cat": "Continuous", "lowBound": 0, "upBound": null},
    {"name": "x3", "cat": "Binary"}
  ],
  "constraints": [
    {
      "name": "ANZ3",
      "sense": 0,
      "coefficients": [{"name": "x1", "value": -7.2}, {"name": "x2", "value": -4.1}, {"name": "x3", "value": 0.9}],
      "pi": null,
      "constant": 1
    }
  ],
  "sos1": [],
  "sos2": []
}`
	mFast, err := format.FromPuLPJSONExactBytes([]byte(fastJSON))
	if err != nil {
		t.Fatalf("FromPuLPJSONExactBytes(fast): %v", err)
	}
	pFast, err := mFast.CompileChecked()
	if err != nil {
		t.Fatalf("pFast.CompileChecked: %v", err)
	}
	if lb0, ub0 := pFast.VarBounds(0); lb0 != 0 || !math.IsInf(ub0, 1) {
		t.Fatalf("x1 omitted lowBound / null upBound = [%v, %v], want [0, +Inf]", lb0, ub0)
	}
	if lb2, ub2 := pFast.VarBounds(2); lb2 != 0 || ub2 != 1 || pFast.VarCategory(2) != gb.Binary {
		t.Fatalf("x3 Binary defaults = ([%v, %v], cat=%v), want ([0,1], Binary)", lb2, ub2, pFast.VarCategory(2))
	}
	var witFast gb.CertifiedWitness
	resFast, err := gb.SolveCompiled(ctx, pFast, gb.WithCertified(true), gb.WithCertifiedWitness(&witFast))
	if err != nil || resFast.Status != gb.StatusOptimal {
		t.Fatalf("pFast certified solve = (%v, %v), want StatusOptimal", resFast.Status, err)
	}
	wantX2 := big.NewRat(10, 41)
	wantObjFast := big.NewRat(141, 410)
	if witFast.Values[0].Sign() != 0 || witFast.Values[1].Cmp(wantX2) != 0 || witFast.Values[2].Sign() != 0 {
		t.Fatalf("fast exact primal = (%s, %s, %s), want (0, 10/41, 0)",
			witFast.Values[0].RatString(), witFast.Values[1].RatString(), witFast.Values[2].RatString())
	}
	if witFast.Objective.Cmp(wantObjFast) != 0 || witFast.BestBound.Cmp(wantObjFast) != 0 {
		t.Fatalf("fast exact obj/bound = (%s, %s), want 141/410", witFast.Objective.RatString(), witFast.BestBound.RatString())
	}

	// Std fallback path: constraints before variables, JSON string escapes, explicit null lowBound (-Inf), Python NaN.
	// Maximize 0.3*x1 - 0.25 s.t. 0.9*x1 <= 1.8 => x1 = 2, obj = 0.6 - 0.25 = 7/20.
	stdJSON := `{
  "sos1": [],
  "sos2": [],
  "objective": {
    "constant": -0.25,
    "coefficients": [{"value": 0.3, "name": "\u00781"}],
    "name": "obj"
  },
  "constraints": [
    {
      "constant": -1.8,
      "coefficients": [{"name": "x1", "value": 0.9}],
      "name": "c1",
      "pi": NaN,
      "sense": -1
    }
  ],
  "variables": [
    {"cat": "Continuous", "name": "x1", "lowBound": null, "upBound": 2.5, "varValue": -Infinity, "dj": Infinity}
  ],
  "parameters": {"sense": -1, "name": "q10_std"}
}`
	mStd, err := format.FromPuLPJSONExact(strings.NewReader(stdJSON))
	if err != nil {
		t.Fatalf("FromPuLPJSONExact(std): %v", err)
	}
	pStd, err := mStd.CompileChecked()
	if err != nil {
		t.Fatalf("pStd.CompileChecked: %v", err)
	}
	if lb, ub := pStd.VarBounds(0); !math.IsInf(lb, -1) || ub != 2.5 {
		t.Fatalf("std x1 bounds = [%v, %v], want [-Inf, 2.5]", lb, ub)
	}
	var witStd gb.CertifiedWitness
	resStd, err := gb.SolveCompiled(ctx, pStd, gb.WithCertified(true), gb.WithCertifiedWitness(&witStd))
	if err != nil || resStd.Status != gb.StatusOptimal {
		t.Fatalf("pStd certified solve = (%v, %v), want StatusOptimal", resStd.Status, err)
	}
	wantObjStd := big.NewRat(7, 20)
	if witStd.Values[0].Cmp(big.NewRat(2, 1)) != 0 || witStd.Objective.Cmp(wantObjStd) != 0 {
		t.Fatalf("std witness = (x=%s, obj=%s), want (2, 7/20)", witStd.Values[0].RatString(), witStd.Objective.RatString())
	}

	// Verify std fallback vs fast path on explicit exact fractional continuous bounds [0.1, 0.9].
	bndJSON := `{
  "variables": [
    {"name": "\u0078", "cat": "Continuous", "lowBound": 0.1, "upBound": 0.9}
  ],
  "parameters": {"name": "bnd", "sense": 1},
  "objective": {"coefficients": [{"name": "x", "value": 1}]},
  "constraints": [],
  "sos1": [],
  "sos2": []
}`
	mBnd, err := format.FromPuLPJSONExactBytes([]byte(bndJSON))
	if err != nil {
		t.Fatalf("FromPuLPJSONExactBytes(bnd): %v", err)
	}
	var witBnd gb.CertifiedWitness
	resBnd, err := gb.SolveCompiled(ctx, mBnd.Compile(), gb.WithCertified(true), gb.WithCertifiedWitness(&witBnd))
	if err != nil || resBnd.Status != gb.StatusOptimal {
		t.Fatalf("pBnd certified solve = (%v, %v)", resBnd.Status, err)
	}
	if witBnd.Values[0].Cmp(big.NewRat(1, 10)) != 0 || witBnd.Objective.Cmp(big.NewRat(1, 10)) != 0 {
		t.Fatalf("bnd witness = (%s, %s), want (1/10, 1/10)", witBnd.Values[0].RatString(), witBnd.Objective.RatString())
	}

	// Negative controls: non-number strings, null coefficient, out-of-range exp10, unknown var.
	badInputs := []string{
		`{"parameters":{"name":"b","sense":1},"objective":{"coefficients":[{"name":"x","value":"0.9"}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[],"sos1":[],"sos2":[]}`,
		`{"parameters":{"name":"b","sense":1},"objective":{"coefficients":[{"name":"x","value":null}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[],"sos1":[],"sos2":[]}`,
		`{"parameters":{"name":"b","sense":1},"objective":{"coefficients":[{"name":"x","value":1e20000}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[],"sos1":[],"sos2":[]}`,
		`{"parameters":{"name":"b","sense":1},"objective":{"coefficients":[{"name":"missing","value":1}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[],"sos1":[],"sos2":[]}`,
	}
	for i, bad := range badInputs {
		if _, err := format.FromPuLPJSONExactBytes([]byte(bad)); err == nil {
			t.Fatalf("badInputs[%d] unexpectedly succeeded", i)
		}
	}
}
