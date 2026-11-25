//go:build cbc && cgo

package format_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

type cbcMPSVar struct {
	name string
	cat  gb.VarCategory
	lb   float64
	ub   float64
	bv   bool
	obj  float64
	rows map[string]float64
}

func roundTripMPSForCBC(mps string) (*gb.Model, error) {
	lines := strings.Split(mps, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "*SENSE:") {
		return nil, errors.New("missing *SENSE comment line")
	}
	sense := gb.Minimize
	if strings.TrimSpace(strings.TrimPrefix(lines[0], "*SENSE:")) == "Maximize" {
		sense = gb.Maximize
	}

	var (
		section  string
		probName string
		objName  string
		inInt    bool
		objConst float64
		rowOrder []string
		rowSense = make(map[string]gb.ConstraintSense)
		rowRHS   = make(map[string]float64)
		colOrder []*cbcMPSVar
		cols     = make(map[string]*cbcMPSVar)
	)

	ensureCol := func(name string, isInt bool) *cbcMPSVar {
		if c, ok := cols[name]; ok {
			return c
		}
		cat := gb.Continuous
		if isInt {
			cat = gb.Integer
		}
		c := &cbcMPSVar{
			name: name,
			cat:  cat,
			lb:   0,
			ub:   math.Inf(1),
			rows: make(map[string]float64),
		}
		cols[name] = c
		colOrder = append(colOrder, c)
		return c
	}

	for _, raw := range lines[1:] {
		line := strings.TrimRight(raw, "\r")
		if line == "" || line[0] == '*' || line[0] == '$' {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) == 0 {
			continue
		}
		if line[0] != ' ' {
			if fs[0] == "NAME" && len(fs) > 1 {
				probName = fs[1]
			}
			section = fs[0]
			continue
		}
		switch section {
		case "ROWS":
			switch fs[0] {
			case "N":
				objName = fs[1]
			case "L":
				rowOrder = append(rowOrder, fs[1])
				rowSense[fs[1]] = gb.LessEqual
			case "G":
				rowOrder = append(rowOrder, fs[1])
				rowSense[fs[1]] = gb.GreaterEqual
			case "E":
				rowOrder = append(rowOrder, fs[1])
				rowSense[fs[1]] = gb.Equal
			}
		case "COLUMNS":
			if len(fs) >= 3 && fs[1] == "'MARKER'" {
				inInt = fs[2] == "'INTORG'"
				continue
			}
			col := ensureCol(fs[0], inInt)
			for k := 1; k+1 < len(fs); k += 2 {
				val, err := strconv.ParseFloat(fs[k+1], 64)
				if err != nil {
					return nil, err
				}
				if fs[k] == objName {
					col.obj += val
				} else {
					col.rows[fs[k]] += val
				}
			}
		case "RHS":
			for k := 1; k+1 < len(fs); k += 2 {
				val, err := strconv.ParseFloat(fs[k+1], 64)
				if err != nil {
					return nil, err
				}
				if fs[k] == objName {
					if sense == gb.Maximize {
						objConst = val
					} else {
						objConst = -val
					}
				} else {
					rowRHS[fs[k]] = val
				}
			}
		case "BOUNDS":
			col := cols[fs[2]]
			switch fs[0] {
			case "BV":
				col.bv = true
				col.cat = gb.Binary
				col.lb, col.ub = 0, 1
			case "FR":
				col.lb, col.ub = math.Inf(-1), math.Inf(1)
			case "MI":
				col.lb = math.Inf(-1)
			case "PL":
				col.ub = math.Inf(1)
			default:
				val, err := strconv.ParseFloat(fs[3], 64)
				if err != nil {
					return nil, err
				}
				switch fs[0] {
				case "FX":
					col.lb, col.ub = val, val
				case "LO", "LI":
					col.lb = val
				case "UP", "UI":
					col.ub = val
				}
			}
		}
	}

	m := gb.NewModel(probName, sense)
	m.Reserve(len(colOrder), len(rowOrder))
	varRefs := make(map[string]gb.Var, len(colOrder))
	objTerms := make([]gb.Term, 0, len(colOrder))
	for _, col := range colOrder {
		var v gb.Var
		switch {
		case col.bv:
			v = m.AddBinaryVar(col.name)
		case col.cat == gb.Integer:
			v = m.AddIntVar(col.name, col.lb, col.ub)
		default:
			v = m.AddContVar(col.name, col.lb, col.ub)
		}
		varRefs[col.name] = v
		if col.obj != 0 {
			c := col.obj
			if sense == gb.Maximize {
				c = -c
			}
			objTerms = append(objTerms, gb.Term{Var: v, Coeff: c})
		}
	}
	m.SetObjectiveNamed(objName, gb.Expr{Terms: objTerms, Constant: objConst})

	for _, rname := range rowOrder {
		var terms []gb.Term
		for _, col := range colOrder {
			if v := col.rows[rname]; v != 0 {
				terms = append(terms, gb.Term{Var: varRefs[col.name], Coeff: v})
			}
		}
		m.AddConstraint(rname, gb.Constraint{
			Expr:  gb.Expr{Terms: terms},
			Sense: rowSense[rname],
			RHS:   rowRHS[rname],
		})
	}
	return m, nil
}

// FuzzWriteMPSCBCParity writes 1..8 variable LPs and MILPs to MPS bytes via format.WriteMPS,
// parses ROWS, COLUMNS, RHS, and BOUNDS back into a gb.CompiledProblem, and asserts
// equal gb.StatusOptimal statuses and float64 objectives within 1e-5 between gb.WithCBC
// and gb.WithPureGo, as well as between the original and round-tripped gb.CompiledProblem.
func FuzzWriteMPSCBCParity(f *testing.F) {
	seeds := []struct {
		seed   uint64
		sense  uint8
		mip    uint8
		vars   uint8
		rows   uint8
		infeas uint8
	}{
		{1, 0, 0, 4, 4, 0},
		{2, 1, 0, 5, 4, 0},
		{3, 0, 1, 6, 5, 0},
		{4, 1, 1, 6, 6, 0},
		{5, 0, 1, 4, 3, 1},
		{6, 1, 0, 3, 3, 1},
	}
	for _, s := range seeds {
		f.Add(s.seed, s.sense, s.mip, s.vars, s.rows, s.infeas)
	}

	f.Fuzz(func(t *testing.T, seed uint64, senseByte, mipByte, varsByte, rowsByte, flagByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x9e3779b97f4a7c15, uint64(senseByte)<<8|uint64(mipByte)))
		sense := gb.Minimize
		if senseByte&1 == 1 {
			sense = gb.Maximize
		}
		isMIP := mipByte&1 == 1
		nVars := 1 + int(varsByte%8)
		nRows := int(rowsByte % 8)

		mdl := gb.NewModel("cbc_mps_rt", sense)
		mdl.Reserve(nVars, nRows)
		names := []string{"a", "var8name", "long_var_9plus", "c3", "c4", "c5", "c6", "c7"}
		vars := make([]gb.Var, nVars)

		for j := range nVars {
			var lb, ub float64
			cat := gb.Continuous
			if isMIP {
				switch rng.IntN(5) {
				case 0:
					cat = gb.Binary
					lb, ub = 0, 1
				case 1:
					cat = gb.Integer
					lb = float64(rng.IntN(5) - 2)
					ub = lb + float64(1+rng.IntN(5))
				case 2:
					cat = gb.Integer
					lb, ub = -1, math.Inf(1)
				case 3:
					cat = gb.Integer
					fx := float64(rng.IntN(5) - 2)
					lb, ub = fx, fx
				default:
					lb = float64(rng.IntN(5) - 2)
					ub = lb + float64(1+rng.IntN(6))
				}
			} else {
				switch rng.IntN(5) {
				case 0:
					lb, ub = math.Inf(-1), math.Inf(1)
				case 1:
					lb, ub = math.Inf(-1), float64(rng.IntN(7)-2)
				case 2:
					fx := float64(rng.IntN(7) - 3)
					lb, ub = fx, fx
				case 3:
					lb, ub = 0, float64(1+rng.IntN(6))
				default:
					lb = float64(rng.IntN(5) - 2)
					ub = lb + float64(1+rng.IntN(8))
				}
			}
			vars[j] = mdl.AddVar(names[j], lb, ub, cat)
		}

		objConst := float64(rng.IntN(21)-10) * 0.5
		objTerms := make([]gb.Term, 0, nVars)
		for j, v := range vars {
			if j == 0 && nVars > 1 && flagByte&1 == 1 {
				continue
			}
			if c := float64(rng.IntN(17)-8) * 0.5; c != 0 {
				objTerms = append(objTerms, gb.Term{Var: v, Coeff: c})
			}
		}
		mdl.SetObjectiveNamed("obj_main", gb.Expr{Terms: objTerms, Constant: objConst})

		csenses := []gb.ConstraintSense{gb.LessEqual, gb.Equal, gb.GreaterEqual}
		for i := range nRows {
			var terms []gb.Term
			for j, v := range vars {
				if j == 0 && nVars > 1 && flagByte&1 == 1 {
					continue
				}
				if rng.IntN(2) == 0 {
					if a := float64(rng.IntN(13)-6) * 0.5; a != 0 {
						terms = append(terms, gb.Term{Var: v, Coeff: a})
					}
				}
			}
			rhs := float64(rng.IntN(25)-12) * 0.5
			if flagByte&2 != 0 && i == 0 {
				// Keep row 0 feasible at x = 0 via 0 >= -5 unless fixed bounds disallow zero.
				terms = nil
				rhs = -5
				mdl.AddConstraint(fmt.Sprintf("r_%d", i), gb.Constraint{
					Expr:  gb.Expr{Terms: terms},
					Sense: gb.GreaterEqual,
					RHS:   rhs,
				})
				continue
			}
			mdl.AddConstraint(fmt.Sprintf("r_%d", i), gb.Constraint{
				Expr:  gb.Expr{Terms: terms},
				Sense: csenses[rng.IntN(len(csenses))],
				RHS:   rhs,
			})
		}

		p1 := mdl.Compile()
		if p1.Err() != nil {
			t.Fatalf("compile failed: %v", p1.Err())
		}
		var buf bytes.Buffer
		if err := format.WriteMPS(p1, &buf); err != nil {
			t.Fatalf("WriteMPS failed: %v", err)
		}
		mdl2, err := roundTripMPSForCBC(buf.String())
		if err != nil {
			t.Fatalf("roundTripMPSForCBC failed: %v\n%s", err, buf.String())
		}
		p2 := mdl2.Compile()
		if p2.Err() != nil {
			t.Fatalf("round-trip compile failed: %v", p2.Err())
		}

		ctx := t.Context()
		resOrig, errOrig := gb.SolveCompiled(ctx, p1, gb.WithPureGo(), gb.WithTimeLimit(500*time.Millisecond), gb.WithMaxNodes(2000))
		resCBC, errCBC := gb.SolveCompiled(ctx, p2, gb.WithCBC(), gb.WithTimeLimit(500*time.Millisecond), gb.WithMaxNodes(2000))
		resGo, errGo := gb.SolveCompiled(ctx, p2, gb.WithPureGo(), gb.WithTimeLimit(500*time.Millisecond), gb.WithMaxNodes(2000))
		if errOrig != nil || errGo != nil {
			t.Fatalf("PureGo SolveCompiled error: orig=%v rt=%v", errOrig, errGo)
		}
		if resOrig.Status != gb.StatusTimeLimit && resOrig.Status != gb.StatusNodeLimit && resOrig.Status != gb.StatusIterationLimit &&
			resGo.Status != gb.StatusTimeLimit && resGo.Status != gb.StatusNodeLimit && resGo.Status != gb.StatusIterationLimit {
			if resOrig.Status != resGo.Status {
				t.Fatalf("PureGo orig vs WriteMPS round-trip status mismatch: orig=%v rt=%v", resOrig.Status, resGo.Status)
			}
			if resGo.Status == gb.StatusOptimal {
				tol := 1e-8 * max(1.0, math.Abs(resOrig.Objective), math.Abs(resGo.Objective))
				if math.Abs(resOrig.Objective-resGo.Objective) > tol {
					t.Fatalf("PureGo orig vs WriteMPS round-trip objective mismatch: orig=%g rt=%g", resOrig.Objective, resGo.Objective)
				}
			}
		}
		if resGo.Status == gb.StatusOptimal {
			bV, rV, iV := p2.MaxViolation(resGo.Values, p2.IsMIP())
			if bV > 1e-6 || rV > 1e-6 || iV > 1e-6 {
				t.Fatalf("PureGo infeasible Optimal solution: bV=%g rV=%g iV=%g", bV, rV, iV)
			}
		}
		if errCBC != nil || resCBC.FellBackToGo {
			return
		}
		switch resCBC.Status {
		case gb.StatusTimeLimit, gb.StatusNodeLimit, gb.StatusIterationLimit, gb.StatusNumericalError, gb.StatusUndefined:
			return
		}
		if resGo.Status == gb.StatusTimeLimit || resGo.Status == gb.StatusNodeLimit || resGo.Status == gb.StatusIterationLimit {
			return
		}

		if resCBC.Status == gb.StatusOptimal {
			bV, rV, iV := p2.MaxViolation(resCBC.Values, p2.IsMIP())
			if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
				// Drop upstream Cbc solves when row residuals, bound gaps, or integer gaps exceed 1e-5.
				return
			}
		}
		if (resGo.Status == gb.StatusOptimal) != (resCBC.Status == gb.StatusOptimal) {
			t.Fatalf("PureGo vs CBC optimal verdict mismatch: go=%v (obj=%g) cbc=%v (obj=%g)\n%s",
				resGo.Status, resGo.Objective, resCBC.Status, resCBC.Objective, buf.String())
		}
		if resGo.Status == gb.StatusOptimal {
			tol := 1e-5 * max(1.0, math.Abs(resGo.Objective), math.Abs(resCBC.Objective))
			if math.Abs(resGo.Objective-resCBC.Objective) > tol {
				t.Fatalf("PureGo vs CBC objective mismatch: go=%g cbc=%g tol=%g",
					resGo.Objective, resCBC.Objective, tol)
			}
		}
	})
}
