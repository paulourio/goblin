package main

import (
	"bytes"
	"encoding/json/jsontext"
	"math"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

func TestDifftestSolveAndSolveAssign(t *testing.T) {
	m := gb.NewModel("diff_mip", gb.Minimize)
	x := m.AddIntVar("x", 0, 5)
	y := m.AddContVar("y", 0, 5)
	m.SetObjective(gb.NewExpr(x.T(3), y.T(1)))
	m.AddConstraint("c", gb.NewExpr(x.T(1), y.T(1)).Ge(2.4))
	var buf bytes.Buffer
	if err := format.ToPuLPJSON(m.Compile(), &buf); err != nil {
		t.Fatalf("ToPuLPJSON failed: %v", err)
	}

	res := solve("case_ok", jsontext.Value(buf.Bytes()), gb.WithPureGo())
	if res.Err != "" || res.Status != "Optimal" || res.Obj == nil || math.Abs(*res.Obj-2.4) > 1e-9 {
		t.Fatalf("solve(case_ok) = %+v, want Optimal 2.4", res)
	}
	if res.BViol == nil || *res.BViol > 1e-9 || res.RViol == nil || *res.RViol > 1e-9 {
		t.Fatalf("unexpected violation in solve(case_ok): %+v", res)
	}

	bad := solve("case_bad", jsontext.Value(`{"broken":`))
	if bad.Err == "" {
		t.Fatalf("expected parse error on malformed JSON, got %+v", bad)
	}

	arOpt := solveAssign("a_opt", 2, 2, 1, []any{4.0, 1.0, 2.0, 3.0})
	if arOpt.Status != "Optimal" || arOpt.Obj == nil || *arOpt.Obj != 3.0 {
		t.Fatalf("solveAssign(a_opt) = %+v, want Optimal 3.0", arOpt)
	}

	arInf := solveAssign("a_inf", 2, 2, 1, []any{"+Inf", "+Inf", 1.0, 2.0})
	if arInf.Status != "Infeasible" {
		t.Fatalf("solveAssign(a_inf) = %+v, want Infeasible", arInf)
	}

	arErr := solveAssign("a_err", 2, 2, 1, []any{"NaN", 1.0, 2.0, 3.0})
	if arErr.Status != "Error" || arErr.Err == "" {
		t.Fatalf("solveAssign(a_err) = %+v, want Error", arErr)
	}
}
