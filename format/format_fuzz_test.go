package format

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

type fuzzTerm struct {
	col int
	val float64
}

func sameCompiledFloat(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	if a == 0 && b == 0 {
		return true
	}
	return math.Float64bits(a) == math.Float64bits(b)
}

func assertCompiledProblemsParity(tb testing.TB, label string, got, want *gb.CompiledProblem, allowObjNameDefault bool) {
	tb.Helper()
	if (got.Err() != nil) != (want.Err() != nil) {
		tb.Fatalf("%s: compile error mismatch got=%v want=%v", label, got.Err(), want.Err())
	}
	if got.Err() != nil {
		return
	}
	if got.Name() != want.Name() {
		tb.Fatalf("%s: Name mismatch got=%q want=%q", label, got.Name(), want.Name())
	}
	wantObjName := want.ObjName()
	if allowObjNameDefault && wantObjName == "" {
		wantObjName = "OBJ"
	}
	if got.ObjName() != wantObjName {
		tb.Fatalf("%s: ObjName mismatch got=%q want=%q", label, got.ObjName(), wantObjName)
	}
	if got.Sense() != want.Sense() {
		tb.Fatalf("%s: Sense mismatch got=%v want=%v", label, got.Sense(), want.Sense())
	}
	if !sameCompiledFloat(got.ObjConstant(), want.ObjConstant()) {
		tb.Fatalf("%s: ObjConstant mismatch got=%g want=%g", label, got.ObjConstant(), want.ObjConstant())
	}
	if got.NumVars() != want.NumVars() || got.NumConstraints() != want.NumConstraints() {
		tb.Fatalf("%s: dimensions mismatch got=(%d,%d) want=(%d,%d)",
			label, got.NumVars(), got.NumConstraints(), want.NumVars(), want.NumConstraints())
	}
	for j := range got.NumVars() {
		if got.VarName(j) != want.VarName(j) {
			tb.Fatalf("%s: VarName(%d) mismatch got=%q want=%q", label, j, got.VarName(j), want.VarName(j))
		}
		if got.VarCategory(j) != want.VarCategory(j) {
			tb.Fatalf("%s: VarCategory(%d) mismatch got=%v want=%v", label, j, got.VarCategory(j), want.VarCategory(j))
		}
		gLB, gUB := got.VarBounds(j)
		wLB, wUB := want.VarBounds(j)
		if !sameCompiledFloat(gLB, wLB) || !sameCompiledFloat(gUB, wUB) {
			tb.Fatalf("%s: VarBounds(%d) mismatch got=[%g,%g] want=[%g,%g]", label, j, gLB, gUB, wLB, wUB)
		}
		if !sameCompiledFloat(got.ObjCoeff(j), want.ObjCoeff(j)) || !sameCompiledFloat(got.ObjMinCoeff(j), want.ObjMinCoeff(j)) {
			tb.Fatalf("%s: ObjCoeff(%d) mismatch got=(%g,%g) want=(%g,%g)",
				label, j, got.ObjCoeff(j), got.ObjMinCoeff(j), want.ObjCoeff(j), want.ObjMinCoeff(j))
		}
	}
	var gotTerms, wantTerms []fuzzTerm
	for i := range got.NumConstraints() {
		if got.RowName(i) != want.RowName(i) || got.RowSense(i) != want.RowSense(i) {
			tb.Fatalf("%s: row %d metadata mismatch got=(%q,%v) want=(%q,%v)",
				label, i, got.RowName(i), got.RowSense(i), want.RowName(i), want.RowSense(i))
		}
		if !sameCompiledFloat(got.RowRHS(i), want.RowRHS(i)) {
			tb.Fatalf("%s: RowRHS(%d) mismatch got=%g want=%g", label, i, got.RowRHS(i), want.RowRHS(i))
		}
		gotTerms = gotTerms[:0]
		wantTerms = wantTerms[:0]
		got.ForEachRowTerm(i, func(col int, val float64) {
			gotTerms = append(gotTerms, fuzzTerm{col: col, val: val})
		})
		want.ForEachRowTerm(i, func(col int, val float64) {
			wantTerms = append(wantTerms, fuzzTerm{col: col, val: val})
		})
		if len(gotTerms) != len(wantTerms) {
			tb.Fatalf("%s: row %d len mismatch got=%d want=%d", label, i, len(gotTerms), len(wantTerms))
		}
		for k := range gotTerms {
			if gotTerms[k].col != wantTerms[k].col || !sameCompiledFloat(gotTerms[k].val, wantTerms[k].val) {
				tb.Fatalf("%s: row %d term %d mismatch got=%+v want=%+v", label, i, k, gotTerms[k], wantTerms[k])
			}
		}
	}
}

func makeFNVCollisionNames(count int) []string {
	// costarring and liquid share an exact 32-bit FNV-1a digest (0x4b0b...);
	// extend with names colliding in lower open-addressing hash buckets (mask 0x3f).
	names := []string{"costarring", "liquid"}
	targetBucket := hashString("costarring") & 63
	for idx := 0; len(names) < count; idx++ {
		cand := fmt.Sprintf("hc_%d", idx)
		if (hashString(cand) & 63) == targetBucket {
			names = append(names, cand)
		}
	}
	return names
}

func buildHashCollisionPuLPJSON() string {
	cols := makeFNVCollisionNames(10)
	var b strings.Builder
	b.WriteString(`{"parameters":{"name":"hash_coll","sense":-1,"status":1,"sol_status":1},"objective":{"name":"obj","constant":3.5,"coefficients":[`)
	for i, name := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":%q,"value":%g}`, name, float64(i+1)*1.25)
	}
	b.WriteString(`]},"variables":[`)
	for i, name := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":%q,"cat":"Continuous","lowBound":-1,"upBound":10}`, name)
	}
	b.WriteString(`],"constraints":[{"name":"c0","sense":-1,"constant":-5,"coefficients":[`)
	for i, name := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":%q,"value":%g}`, name, 0.5*float64((i%3)+1))
	}
	b.WriteString(`]}],"sos1":[],"sos2":[]}`)
	return b.String()
}

// FuzzPuLPJSONFastVsStd differentially fuzzes parsePuLPJSONFast against parsePuLPJSONStd
// and FromPuLPJSONBytes against ParsePuLPJSON + BuildModelFromPuLPJSON on arbitrary JSON payloads.
func FuzzPuLPJSONFastVsStd(f *testing.F) {
	seeds := []string{
		`{"parameters":{"name":"min_basic","sense":1,"status":1,"sol_status":1},"objective":{"name":"OBJ","coefficients":[{"name":"x","value":2.5}],"constant":1.5},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":5}],"constraints":[{"name":"r0","sense":-1,"coefficients":[{"name":"x","value":1}],"constant":-2}],"sos1":[],"sos2":[]}`,
		`{"parameters":{"name":"max_negzero","sense":-1},"objective":{"name":null,"coefficients":[{"name":"b","value":-0.0}]},"variables":[{"name":"b","cat":"Binary"}],"constraints":[{"name":"eq","sense":0,"coefficients":[{"name":"b","value":1}],"constant":-1}]}`,
		`{"parameters":{"name":"u\u0041\u0062","sense":1},"objective":{"name":"O\u0042","coefficients":[{"name":"x\u0031","value":1}]},"variables":[{"name":"x\u0031","cat":"Integer","lowBound":0,"upBound":3}],"constraints":[]}`,
		`{"parameters":{"name":"surr_\uD83D\uDE00","sense":1},"objective":{"name":"obj","coefficients":[]},"variables":[{"name":"v","cat":"Continuous"}],"constraints":[]}`,
		`{"parameters":{"name":"lone_surr_\uD800","sense":1},"objective":{"name":"obj","coefficients":[]},"variables":[],"constraints":[]}`,
		`{"parameters":{"name":"dup","sense":1,"sense":-1},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`,
		`{"parameters":{"name":"dupvar","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"lowBound":2}],"constraints":[]}`,
		`{"parameters":{"name":"py_nonfinite","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":1}]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":2,"varValue":NaN,"dj":Infinity}],"constraints":[{"name":"c","sense":0,"coefficients":[{"name":"x","value":1}],"pi":-Infinity,"constant":-1}]}`,
		`{"parameters":{"name":"bad_nonfinite","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":NaN}]},"variables":[{"name":"x","cat":"Continuous","lowBound":0}],"constraints":[]}`,
		`{"parameters":{"name":"subnormal_ovf","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":5e-324},{"name":"y","value":1.7976931348623157e+308}],"constant":-1e-308},"variables":[{"name":"x","cat":"Continuous","lowBound":null,"upBound":null},{"name":"y","cat":"Integer","lowBound":0,"upBound":5}],"constraints":[]}`,
		`{"parameters":{"name":"huge_exp","sense":1},"objective":{"name":"o","coefficients":[{"name":"x","value":1e400}]},"variables":[{"name":"x","cat":"Continuous"}],"constraints":[]}`,
		`{"parameters":{"name":"huge_int","sense":99999999999999999999},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`,
		`{"parameters":{"name":"u32_ovf","sense":4294967297},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[]}`,
		`{"parameters":{"name":"sos1_nonempty","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[],"sos1":[1],"sos2":[]}`,
		`{"parameters":{"name":"sos2_nonempty","sense":1},"objective":{"name":"o","coefficients":[]},"variables":[],"constraints":[],"sos1":[],"sos2":[{"name":"s"}]}`,
		buildHashCollisionPuLPJSON(),
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	for _, corpusFile := range []string{"t2_lotsizing_48.json"} {
		if raw, err := os.ReadFile(filepath.Join("..", "testdata", "corpus", corpusFile)); err == nil {
			f.Add(raw)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		raw := bytes.Clone(data)

		fastDoc, fastOK := parsePuLPJSONFast(bytes.Clone(raw))
		stdDoc, stdErr := parsePuLPJSONStd(bytes.Clone(raw))
		pubDoc, pubErr := ParsePuLPJSON(bytes.Clone(raw))

		if fastOK && stdErr != nil {
			t.Fatalf("parsePuLPJSONFast accepted payload rejected by std: stdErr=%v", stdErr)
		}
		if (pubErr != nil) != (stdErr != nil) {
			t.Fatalf("ParsePuLPJSON error mismatch: pub=%v std=%v", pubErr, stdErr)
		}
		if stdErr == nil {
			if fastOK && !reflect.DeepEqual(fastDoc, stdDoc) {
				t.Fatalf("parsePuLPJSONFast AST mismatch against std")
			}
			if !reflect.DeepEqual(pubDoc, stdDoc) {
				t.Fatalf("ParsePuLPJSON AST mismatch against std")
			}
		}

		fastMdl, fastMdlOK := fromPuLPJSONFast(bytes.Clone(raw))
		if fastMdlOK && fastMdl == nil {
			t.Fatalf("fromPuLPJSONFast returned ok=true with nil model")
		}
		if fastMdlOK && !fastOK && stdErr != nil {
			t.Fatalf("fromPuLPJSONFast succeeded where std parser failed: %v", stdErr)
		}

		gotMdl, gotErr := FromPuLPJSONBytes(bytes.Clone(raw))
		var wantMdl *gb.Model
		var wantErr error
		if pubErr != nil {
			wantErr = pubErr
		} else {
			wantMdl, wantErr = BuildModelFromPuLPJSON(pubDoc)
		}
		if (gotErr != nil) != (wantErr != nil) {
			t.Fatalf("FromPuLPJSONBytes err=%v vs Parse+Build err=%v", gotErr, wantErr)
		}
		if gotErr == nil {
			gotComp := gotMdl.Compile()
			wantComp := wantMdl.Compile()
			assertCompiledProblemsParity(t, "FromPuLPJSONBytes", gotComp, wantComp, false)
		}
	})
}

type parsedMPSVar struct {
	name   string
	cat    gb.VarCategory
	lb     float64
	ub     float64
	hasBV  bool
	hasRow map[string]float64
	objMin float64
}

type parsedMPSRow struct {
	name  string
	sense gb.ConstraintSense
	rhs   float64
}

func parseTestMPS(mpsText string) (*gb.Model, gb.Sense, error) {
	lines := strings.Split(mpsText, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "*SENSE:") {
		return nil, 0, errors.New("missing *SENSE header line")
	}
	var modelSense gb.Sense
	switch strings.TrimSpace(strings.TrimPrefix(lines[0], "*SENSE:")) {
	case "Minimize":
		modelSense = gb.Minimize
	case "Maximize":
		modelSense = gb.Maximize
	default:
		return nil, 0, fmt.Errorf("unknown sense line %q", lines[0])
	}

	var (
		section    string
		probName   string
		objName    string
		hasObj     bool
		inIntBlock bool
		rows       []parsedMPSRow
		rowIdx     = make(map[string]int)
		vars       []*parsedMPSVar
		varIdx     = make(map[string]*parsedMPSVar)
		objConst   float64
		seenEndata bool
	)

	getOrAddVar := func(name string, isInt bool) *parsedMPSVar {
		if pv, ok := varIdx[name]; ok {
			return pv
		}
		cat := gb.Continuous
		if isInt {
			cat = gb.Integer
		}
		pv := &parsedMPSVar{
			name:   name,
			cat:    cat,
			lb:     0,
			ub:     math.Inf(1),
			hasRow: make(map[string]float64),
		}
		vars = append(vars, pv)
		varIdx[name] = pv
		return pv
	}

	for _, rawLine := range lines[1:] {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" || line[0] == '*' || line[0] == '$' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if line[0] != ' ' {
			switch fields[0] {
			case "NAME":
				if len(fields) > 1 {
					probName = fields[1]
				}
				section = "NAME"
			case "ROWS", "COLUMNS", "RHS", "BOUNDS":
				section = fields[0]
			case "ENDATA":
				seenEndata = true
				section = "ENDATA"
			default:
				return nil, 0, fmt.Errorf("unexpected MPS section %q", fields[0])
			}
			continue
		}

		switch section {
		case "ROWS":
			if len(fields) < 2 {
				return nil, 0, fmt.Errorf("malformed ROWS line %q", line)
			}
			rname := fields[1]
			switch fields[0] {
			case "N":
				if !hasObj {
					objName = rname
					hasObj = true
				}
			case "L":
				rowIdx[rname] = len(rows)
				rows = append(rows, parsedMPSRow{name: rname, sense: gb.LessEqual})
			case "G":
				rowIdx[rname] = len(rows)
				rows = append(rows, parsedMPSRow{name: rname, sense: gb.GreaterEqual})
			case "E":
				rowIdx[rname] = len(rows)
				rows = append(rows, parsedMPSRow{name: rname, sense: gb.Equal})
			default:
				return nil, 0, fmt.Errorf("unknown row sense %q", fields[0])
			}
		case "COLUMNS":
			if len(fields) >= 3 && fields[1] == "'MARKER'" {
				switch fields[2] {
				case "'INTORG'":
					inIntBlock = true
				case "'INTEND'":
					inIntBlock = false
				}
				continue
			}
			if len(fields) < 3 || len(fields)%2 != 1 {
				return nil, 0, fmt.Errorf("malformed COLUMNS line %q", line)
			}
			pv := getOrAddVar(fields[0], inIntBlock)
			for i := 1; i+1 < len(fields); i += 2 {
				rname := fields[i]
				val, err := strconv.ParseFloat(fields[i+1], 64)
				if err != nil {
					return nil, 0, fmt.Errorf("parse column value %q: %w", fields[i+1], err)
				}
				if hasObj && rname == objName {
					pv.objMin += val
				} else {
					pv.hasRow[rname] += val
				}
			}
		case "RHS":
			if len(fields) < 3 || len(fields)%2 != 1 {
				return nil, 0, fmt.Errorf("malformed RHS line %q", line)
			}
			for i := 1; i+1 < len(fields); i += 2 {
				rname := fields[i]
				val, err := strconv.ParseFloat(fields[i+1], 64)
				if err != nil {
					return nil, 0, fmt.Errorf("parse RHS value %q: %w", fields[i+1], err)
				}
				if hasObj && rname == objName {
					if modelSense == gb.Maximize {
						objConst = val
					} else {
						objConst = -val
					}
				} else if ri, ok := rowIdx[rname]; ok {
					rows[ri].rhs = val
				} else {
					return nil, 0, fmt.Errorf("RHS references unknown row %q", rname)
				}
			}
		case "BOUNDS":
			if len(fields) < 3 {
				return nil, 0, fmt.Errorf("malformed BOUNDS line %q", line)
			}
			bndType := fields[0]
			vname := fields[2]
			pv, ok := varIdx[vname]
			if !ok {
				return nil, 0, fmt.Errorf("BOUNDS references unknown var %q", vname)
			}
			switch bndType {
			case "BV":
				pv.hasBV = true
				pv.cat = gb.Binary
				pv.lb = 0
				pv.ub = 1
			case "FR":
				pv.lb = math.Inf(-1)
				pv.ub = math.Inf(1)
			case "MI":
				pv.lb = math.Inf(-1)
			case "PL":
				pv.ub = math.Inf(1)
			case "FX", "LO", "LI", "UP", "UI":
				if len(fields) < 4 {
					return nil, 0, fmt.Errorf("missing value on BOUNDS line %q", line)
				}
				val, err := strconv.ParseFloat(fields[3], 64)
				if err != nil {
					return nil, 0, fmt.Errorf("parse bound value %q: %w", fields[3], err)
				}
				switch bndType {
				case "FX":
					pv.lb = val
					pv.ub = val
				case "LO", "LI":
					pv.lb = val
				case "UP", "UI":
					pv.ub = val
				}
			default:
				return nil, 0, fmt.Errorf("unknown bound type %q", bndType)
			}
		default:
			return nil, 0, fmt.Errorf("unexpected content outside section %q: %q", section, line)
		}
	}
	if !seenEndata {
		return nil, 0, errors.New("missing ENDATA terminator")
	}

	m := gb.NewModel(probName, modelSense)
	m.Reserve(len(vars), len(rows))
	varMap := make(map[string]gb.Var, len(vars))
	varOrder := make([]gb.Var, len(vars))
	objTerms := make([]gb.Term, 0, len(vars))
	for j, pv := range vars {
		var v gb.Var
		switch {
		case pv.hasBV:
			v = m.AddBinaryVar(pv.name)
		case pv.cat == gb.Integer:
			v = m.AddIntVar(pv.name, pv.lb, pv.ub)
		default:
			v = m.AddContVar(pv.name, pv.lb, pv.ub)
		}
		varMap[pv.name] = v
		varOrder[j] = v
		if pv.objMin != 0 {
			c := pv.objMin
			if modelSense == gb.Maximize && c != 0 {
				c = -c
			}
			objTerms = append(objTerms, gb.Term{Var: v, Coeff: c})
		}
	}
	m.SetObjectiveNamed(objName, gb.Expr{Terms: objTerms, Constant: objConst})

	rowTerms := make([][]gb.Term, len(rows))
	for _, pv := range vars {
		v := varMap[pv.name]
		for rname, val := range pv.hasRow {
			if val == 0 {
				continue
			}
			ri, ok := rowIdx[rname]
			if !ok {
				return nil, 0, fmt.Errorf("column %q references unknown row %q", pv.name, rname)
			}
			rowTerms[ri] = append(rowTerms[ri], gb.Term{Var: v, Coeff: val})
		}
	}
	for i, rw := range rows {
		m.AddConstraint(rw.name, gb.Constraint{
			Expr:  gb.Expr{Terms: rowTerms[i]},
			Sense: rw.sense,
			RHS:   rw.rhs,
		})
	}
	return m, modelSense, nil
}

type mpsFuzzCursor struct {
	data []byte
	pos  int
}

func (c *mpsFuzzCursor) next() byte {
	if len(c.data) == 0 {
		return 0
	}
	b := c.data[c.pos%len(c.data)]
	c.pos++
	return b
}

func decodeBoundaryMPSModel(data []byte) (mdl *gb.Model, hasConflict, hasExtremeScale bool) {
	c := mpsFuzzCursor{data: data}
	sense := gb.Minimize
	if c.next()&1 == 1 {
		sense = gb.Maximize
	}
	probNames := []string{"", "p1", "prob1234", "prob12345", "boundary_problem_name"}
	objNames := []string{"", "O", "OBJ12345", "obj_nine9", "custom_obj_row"}
	pName := probNames[int(c.next())%len(probNames)]
	oName := objNames[int(c.next())%len(objNames)]

	nVars := int(c.next() % 11)
	nRows := int(c.next() % 10)
	extremeFlag := c.next()%6 == 0
	infeasFlag := c.next()%7 == 0
	hasExtremeScale = extremeFlag

	constPalette := []float64{0, math.Copysign(0, -1), 1.5, -2.25, 100.0, -1e300, 1e300, 1e-300}
	objConst := constPalette[int(c.next())%len(constPalette)]
	if !extremeFlag && math.Abs(objConst) > 1e10 {
		objConst = -3.75
	}

	mdl = gb.NewModel(pName, sense)
	mdl.Reserve(nVars, nRows)
	vars := make([]gb.Var, nVars)
	varNameSamples := []string{
		"a", "b2", "len7nam", "len8name", "len9name_", "very_long_var_name_boundary",
		"v#6", "v.7", "v_8", "v@9",
	}

	for j := range nVars {
		vname := fmt.Sprintf("%s_%d", varNameSamples[j%len(varNameSamples)], j)
		if j < 5 {
			// Exercise exact 1/2/7/8/9+ fixed-vs-free MPS column widths without suffixes.
			vname = varNameSamples[j]
		}
		var lb, ub float64
		cat := gb.Continuous
		mode := c.next() % 10
		if infeasFlag && j == nVars-1 {
			mode = 9
		}
		switch mode {
		case 0: // FR
			lb, ub = math.Inf(-1), math.Inf(1)
		case 1: // MI + finite ub
			lb, ub = math.Inf(-1), float64(int(c.next()%21)-10)
		case 2: // PL integer
			cat = gb.Integer
			lb, ub = float64(int(c.next()%7)-3), math.Inf(1)
		case 3: // FX
			fx := float64(int(c.next()%11) - 5)
			if extremeFlag && j == 0 {
				fx = 1e300
			}
			lb, ub = fx, fx
		case 4: // BV
			cat = gb.Binary
			lb, ub = 0, 1
		case 5: // LI/UI Integer
			cat = gb.Integer
			lb = float64(int(c.next()%9) - 4)
			ub = lb + float64(c.next()%8)
		case 6: // Continuous [0, +Inf] default bounds
			lb, ub = 0, math.Inf(1)
		case 7: // Continuous negative bounds
			lb, ub = -10, -float64(1+c.next()%5)
		case 8: // Extreme 1e+-300 Continuous
			if extremeFlag {
				lb, ub = -1e300, 1e300
			} else {
				lb, ub = -12.5, 12.5
			}
		default:
			if infeasFlag && j == nVars-1 {
				lb, ub = 5, 2
			} else {
				cat = gb.Integer
				lb, ub = math.Inf(-1), math.Inf(1) // FR integer
			}
		}
		if lb > ub {
			hasConflict = true
		}
		vars[j] = mdl.AddVar(vname, lb, ub, cat)
	}

	valPalette := []float64{0, 1, -1, 2.5, -3.5, 0.125, -4, 1e-300, 1e300, -1e300}
	pickVal := func() float64 {
		v := valPalette[int(c.next())%len(valPalette)]
		if !extremeFlag && math.Abs(v) > 1e10 {
			return 2.25
		}
		if extremeFlag && math.Abs(v) > 1e10 {
			hasExtremeScale = true
		}
		return v
	}

	objTerms := make([]gb.Term, 0, nVars)
	for j, v := range vars {
		if j == 0 && nVars > 1 && c.next()%2 == 0 {
			// Keep column 0 empty to exercise unused-column registration.
			continue
		}
		if coeff := pickVal(); coeff != 0 {
			objTerms = append(objTerms, gb.Term{Var: v, Coeff: coeff})
		}
	}
	mdl.SetObjectiveNamed(oName, gb.Expr{Terms: objTerms, Constant: objConst})

	senses := []gb.ConstraintSense{gb.LessEqual, gb.Equal, gb.GreaterEqual}
	rowNameSamples := []string{"r", "row1234", "row12345", "row_boundary_long_name"}
	for i := range nRows {
		rname := fmt.Sprintf("%s_%d", rowNameSamples[i%len(rowNameSamples)], i)
		if i < len(rowNameSamples) && nRows <= len(rowNameSamples) {
			rname = rowNameSamples[i]
		}
		var terms []gb.Term
		if i != 0 || c.next()%2 != 0 {
			for j, v := range vars {
				if j == 0 && nVars > 1 {
					continue
				}
				if c.next()%3 == 0 {
					if val := pickVal(); val != 0 {
						terms = append(terms, gb.Term{Var: v, Coeff: val})
					}
				}
			}
		}
		rhs := float64(int(c.next()%25) - 12)
		if extremeFlag && c.next()%4 == 0 {
			rhs = 1e300
		}
		mdl.AddConstraint(rname, gb.Constraint{
			Expr:  gb.Expr{Terms: terms},
			Sense: senses[int(c.next())%len(senses)],
			RHS:   rhs,
		})
	}
	return mdl, hasConflict, hasExtremeScale
}

// FuzzWriteMPSRoundTrip verifies WriteMPS serialization and round-trip parsing back into
// a CompiledProblem across all bound types, Maximize/Minimize negation, ObjConstant,
// boundary names, empty rows/cols, and Pure-Go solve parity.
func FuzzWriteMPSRoundTrip(f *testing.F) {
	seeds := [][]byte{
		{0, 0, 1, 6, 4, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 2, 8, 6, 1, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		{1, 3, 3, 0, 0, 0, 0},                         // empty problem (0 vars, 0 rows)
		{0, 4, 4, 5, 3, 0, 1, 9, 9, 9, 9, 2, 3, 4, 5}, // inverted bounds conflict row
		{1, 1, 0, 7, 5, 1, 0, 2, 5, 6, 7, 8, 1, 2, 3}, // Maximize + extreme scales
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		mdl, hasConflict, hasExtreme := decodeBoundaryMPSModel(data)
		p1 := mdl.Compile()
		if p1.Err() != nil {
			t.Fatalf("generated model compile failed: %v", p1.Err())
		}

		var buf bytes.Buffer
		if err := WriteMPS(p1, &buf); err != nil {
			t.Fatalf("WriteMPS failed: %v", err)
		}

		mdl2, rtSense, err := parseTestMPS(buf.String())
		if err != nil {
			t.Fatalf("parseTestMPS failed: %v\n%s", err, buf.String())
		}
		if rtSense != p1.Sense() {
			t.Fatalf("sense mismatch got=%v want=%v", rtSense, p1.Sense())
		}
		p2 := mdl2.Compile()
		if p2.Err() != nil {
			t.Fatalf("round-trip model compile failed: %v", p2.Err())
		}

		if !hasConflict {
			assertCompiledProblemsParity(t, "WriteMPS", p2, p1, true)
		} else if p2.NumConstraints() <= p1.NumConstraints() {
			t.Fatalf("expected conflict rows in round-trip MPS: p2=%d <= p1=%d", p2.NumConstraints(), p1.NumConstraints())
		}

		if hasExtreme {
			return
		}
		ctx := t.Context()
		r1, e1 := gb.SolveCompiled(ctx, p1, gb.WithPureGo(), gb.WithTimeLimit(150*time.Millisecond))
		r2, e2 := gb.SolveCompiled(ctx, p2, gb.WithPureGo(), gb.WithTimeLimit(150*time.Millisecond))
		if (e1 != nil) != (e2 != nil) {
			t.Fatalf("PureGo solve error mismatch e1=%v e2=%v", e1, e2)
		}
		if e1 != nil {
			return
		}
		if r1.Status == gb.StatusTimeLimit || r2.Status == gb.StatusTimeLimit {
			return
		}
		if r1.Status != r2.Status {
			t.Fatalf("PureGo status mismatch after WriteMPS round-trip: orig=%v rt=%v (conflict=%v)", r1.Status, r2.Status, hasConflict)
		}
		if r1.Status == gb.StatusOptimal {
			tol := 1e-8 * max(1.0, math.Abs(r1.Objective), math.Abs(r2.Objective))
			if math.Abs(r1.Objective-r2.Objective) > tol {
				t.Fatalf("PureGo objective mismatch after WriteMPS round-trip: orig=%g rt=%g tol=%g",
					r1.Objective, r2.Objective, tol)
			}
		}
	})
}
