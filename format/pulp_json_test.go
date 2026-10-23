package format_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

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
	if m.Sense() != goblin.Maximize {
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
