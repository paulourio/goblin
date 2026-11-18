package format_test

import (
	"bytes"
	"math"
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
	if p.NumVars != 3 || p.NumConstraints != 1 {
		t.Fatalf("unexpected compiled size: %d vars, %d rows", p.NumVars, p.NumConstraints)
	}
	if p.RowRHS[0] != 12.5 {
		t.Fatalf("expected RHS=12.5 (from constant=-12.5), got %v", p.RowRHS[0])
	}
	if !math.IsInf(p.VarLB[2], -1) || p.VarUB[2] != 5.5 {
		t.Fatalf("unexpected bounds on z: [%v, %v]", p.VarLB[2], p.VarUB[2])
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
	if p2.NumVars != p.NumVars || p2.NumConstraints != p.NumConstraints || p2.RowRHS[0] != p.RowRHS[0] {
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
	if pOmit.VarLB[0] != 0 || pOmit.VarUB[0] != 10 {
		t.Fatalf("expected omitted lowBound -> [0, 10], got [%v, %v]", pOmit.VarLB[0], pOmit.VarUB[0])
	}
	if !math.IsInf(pOmit.VarLB[1], -1) || pOmit.VarUB[1] != 5 {
		t.Fatalf("expected explicit null lowBound -> [-Inf, 5], got [%v, %v]", pOmit.VarLB[1], pOmit.VarUB[1])
	}

	if err := format.ToPuLPJSON(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when serializing nil CompiledProblem to PuLP JSON")
	}
}

// Verifies lossless float output, objective constant RHS entries, and errors on
// whitespace or colliding MPS identifiers.
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
}
