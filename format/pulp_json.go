// Package format provides serialization and deserialization for PuLP JSON and
// MPS problem representations.
package format

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"sync"

	gb "github.com/paulourio/goblin"
)

var reIgnoredPythonNonFinite = regexp.MustCompile(`("(?:varValue|dj|pi)"\s*:\s*)(?:-?Infinity|NaN)\b`)

var (
	tokParameters   = jsontext.String("parameters")
	tokName         = jsontext.String("name")
	tokSense        = jsontext.String("sense")
	tokStatus       = jsontext.String("status")
	tokSolStatus    = jsontext.String("sol_status")
	tokObjective    = jsontext.String("objective")
	tokCoefficients = jsontext.String("coefficients")
	tokValue        = jsontext.String("value")
	tokConstant     = jsontext.String("constant")
	tokVariables    = jsontext.String("variables")
	tokCat          = jsontext.String("cat")
	tokContinuous   = jsontext.String("Continuous")
	tokInteger      = jsontext.String("Integer")
	tokLowBound     = jsontext.String("lowBound")
	tokUpBound      = jsontext.String("upBound")
	tokVarValue     = jsontext.String("varValue")
	tokDj           = jsontext.String("dj")
	tokConstraints  = jsontext.String("constraints")
	tokPi           = jsontext.String("pi")
	tokSOS1         = jsontext.String("sos1")
	tokSOS2         = jsontext.String("sos2")
	tokZeroInt      = jsontext.Int(0)
	pulpIndentOpt   = jsontext.WithIndent("  ")
)

var pulpEncPool = sync.Pool{
	New: func() any {
		return jsontext.NewEncoder(io.Discard)
	},
}

// PuLPJSON represents the native JSON schema produced by PuLP's LpProblem.toJson / toDict
// and consumed by LpProblem.fromJson / fromDict.
type PuLPJSON struct {
	Parameters  PuLPParameters   `json:"parameters"`
	Objective   PuLPObjective    `json:"objective"`
	Variables   []PuLPVariable   `json:"variables"`
	Constraints []PuLPConstraint `json:"constraints"`
	SOS1        []any            `json:"sos1"`
	SOS2        []any            `json:"sos2"`
}

// PuLPParameters holds problem-level metadata in PuLP's JSON schema.
type PuLPParameters struct {
	Name      string `json:"name"`
	Sense     int    `json:"sense"`
	Status    int    `json:"status"`
	SolStatus int    `json:"sol_status"`
}

// PuLPCoefficient represents a single (variable_name, coefficient_value) pair.
type PuLPCoefficient struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// PuLPObjective holds the linear objective in PuLP's JSON schema.
// Constant is a goblin extension preserving nonzero Model objective offsets;
// Python PuLP's MPSObjective.fromDict ignores this field when loading JSON.
type PuLPObjective struct {
	Name         *string           `json:"name"`
	Coefficients []PuLPCoefficient `json:"coefficients"`
	Constant     float64           `json:"constant,omitzero"`
}

// PuLPVariable holds a variable specification in PuLP's JSON schema.
type PuLPVariable struct {
	Name     string   `json:"name"`
	Cat      string   `json:"cat"`
	LowBound *float64 `json:"lowBound"`
	UpBound  *float64 `json:"upBound"`
	VarValue *float64 `json:"varValue"`
	Dj       *float64 `json:"dj"`
}

// PuLPConstraint holds a constraint specification in PuLP's JSON schema.
// Note that in PuLP, constraints are stored as: sum(coeff * var) + constant {<=, =, >=} 0,
// meaning the right-hand side is RHS = -constant.
type PuLPConstraint struct {
	Name         string            `json:"name"`
	Sense        int               `json:"sense"`
	Coefficients []PuLPCoefficient `json:"coefficients"`
	Pi           *float64          `json:"pi"`
	Constant     float64           `json:"constant"`
}

type pulpLowBoundProbe struct {
	Variables []struct {
		LowBound jsontext.Value `json:"lowBound"`
	} `json:"variables"`
}

func parsePuLPJSONStd(data []byte) (*PuLPJSON, error) {
	var doc PuLPJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		if bytes.Contains(data, []byte("NaN")) || bytes.Contains(data, []byte("Infinity")) {
			sanitized := reIgnoredPythonNonFinite.ReplaceAll(data, []byte("${1}null"))
			if err2 := json.Unmarshal(sanitized, &doc); err2 == nil {
				data = sanitized
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("parse PuLP JSON: %w", err)
		}
	}
	hasNil := false
	for i := range doc.Variables {
		if doc.Variables[i].LowBound == nil {
			hasNil = true
			break
		}
	}
	if hasNil {
		var probe pulpLowBoundProbe
		if err := json.Unmarshal(data, &probe); err == nil && len(probe.Variables) == len(doc.Variables) {
			for i := range doc.Variables {
				if doc.Variables[i].LowBound == nil && len(probe.Variables[i].LowBound) == 0 {
					zero := 0.0
					doc.Variables[i].LowBound = &zero
				}
			}
		}
	}
	return &doc, nil
}

// ParsePuLPJSON decodes a PuLP JSON payload from bytes without building a gb.Model yet.
// Omitted variable lowBound keys default to 0 while explicit JSON null maps to nil (-Inf),
// matching Python PuLP's MPSVariable.fromDict.
func ParsePuLPJSON(data []byte) (*PuLPJSON, error) {
	if doc, ok := parsePuLPJSONFast(data); ok {
		return doc, nil
	}
	return parsePuLPJSONStd(data)
}

func parsePuLPSense(raw int) (gb.Sense, bool) {
	switch raw {
	case 1:
		return gb.Minimize, true
	case -1:
		return gb.Maximize, true
	default:
		return 0, false
	}
}

func parsePuLPConstraintSense(raw int) (gb.ConstraintSense, bool) {
	switch raw {
	case -1:
		return gb.LessEqual, true
	case 0:
		return gb.Equal, true
	case 1:
		return gb.GreaterEqual, true
	default:
		return 0, false
	}
}

func resolvePuLPVarSpec(cat string, lb, ub float64, lbNil, ubNil bool) (float64, float64, gb.VarCategory, bool) {
	switch cat {
	case "Continuous":
		return lb, ub, gb.Continuous, true
	case "Binary":
		if lbNil {
			lb = 0
		}
		if ubNil {
			ub = 1
		}
		if lb >= 0 && ub <= 1 && lb <= ub {
			return lb, ub, gb.Binary, true
		}
		return lb, ub, gb.Integer, true
	case "Integer":
		if lb >= 0 && ub <= 1 && lb <= ub {
			return lb, ub, gb.Binary, true
		}
		return lb, ub, gb.Integer, true
	default:
		return 0, 0, 0, false
	}
}

// BuildModelFromPuLPJSON constructs a *gb.Model from a decoded PuLPJSON structure.
func BuildModelFromPuLPJSON(doc *PuLPJSON) (*gb.Model, error) {
	if doc == nil {
		return nil, errors.New("nil PuLPJSON document")
	}
	if len(doc.SOS1) > 0 || len(doc.SOS2) > 0 {
		return nil, fmt.Errorf("PuLP JSON contains unsupported SOS constraints (sos1=%d, sos2=%d)", len(doc.SOS1), len(doc.SOS2))
	}
	sense, ok := parsePuLPSense(doc.Parameters.Sense)
	if !ok {
		return nil, fmt.Errorf("PuLP JSON has invalid objective sense %d", doc.Parameters.Sense)
	}
	m := gb.NewModel(doc.Parameters.Name, sense)
	m.Reserve(len(doc.Variables), len(doc.Constraints))

	for i := range doc.Variables {
		v := &doc.Variables[i]
		if v.Name == "" {
			return nil, errors.New("PuLP JSON contains variable with empty name")
		}
		if _, exists := m.VarByName(v.Name); exists {
			return nil, fmt.Errorf("PuLP JSON contains duplicate variable name %q", v.Name)
		}
		lb := math.Inf(-1)
		if v.LowBound != nil {
			lb = *v.LowBound
		}
		ub := math.Inf(1)
		if v.UpBound != nil {
			ub = *v.UpBound
		}
		lb, ub, cat, ok := resolvePuLPVarSpec(v.Cat, lb, ub, v.LowBound == nil, v.UpBound == nil)
		if !ok {
			return nil, fmt.Errorf("variable %q has invalid category %q", v.Name, v.Cat)
		}
		m.AddVar(v.Name, lb, ub, cat)
	}

	maxTerms := len(doc.Objective.Coefficients)
	for i := range doc.Constraints {
		if n := len(doc.Constraints[i].Coefficients); n > maxTerms {
			maxTerms = n
		}
	}
	var terms []gb.Term
	if maxTerms > 0 {
		terms = make([]gb.Term, 0, maxTerms)
	}

	for _, c := range doc.Objective.Coefficients {
		vid, ok := m.VarByName(c.Name)
		if !ok {
			return nil, fmt.Errorf("objective references unknown variable %q", c.Name)
		}
		terms = append(terms, gb.Term{Var: vid, Coeff: c.Value})
	}
	objName := ""
	if doc.Objective.Name != nil {
		objName = *doc.Objective.Name
	}
	m.SetObjectiveNamed(objName, gb.Expr{Terms: terms, Constant: doc.Objective.Constant})

	for i := range doc.Constraints {
		c := &doc.Constraints[i]
		terms = terms[:0]
		for _, coeff := range c.Coefficients {
			vid, ok := m.VarByName(coeff.Name)
			if !ok {
				return nil, fmt.Errorf("constraint %q references unknown variable %q", c.Name, coeff.Name)
			}
			terms = append(terms, gb.Term{Var: vid, Coeff: coeff.Value})
		}
		csense, ok := parsePuLPConstraintSense(c.Sense)
		if !ok {
			return nil, fmt.Errorf("constraint %q has invalid sense %d", c.Name, c.Sense)
		}
		rhs := -c.Constant
		m.AddConstraint(c.Name, gb.Constraint{
			Expr:  gb.Expr{Terms: terms},
			Sense: csense,
			RHS:   rhs,
		})
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	return m, nil
}

// FromPuLPJSONBytes decodes a PuLP JSON payload directly into a *gb.Model,
// bypassing the intermediate PuLPJSON AST on standard unescaped inputs.
func FromPuLPJSONBytes(data []byte) (*gb.Model, error) {
	if m, ok := fromPuLPJSONFast(data); ok {
		return m, nil
	}
	doc, err := ParsePuLPJSON(data)
	if err != nil {
		return nil, err
	}
	return BuildModelFromPuLPJSON(doc)
}

// FromPuLPJSON reads a PuLP JSON document from r and returns a *gb.Model.
func FromPuLPJSON(r io.Reader) (*gb.Model, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return FromPuLPJSONBytes(data)
}

type pulpExactCoeffJSON struct {
	Name  string         `json:"name"`
	Value jsontext.Value `json:"value"`
}

type pulpExactObjectiveJSON struct {
	Name         *string              `json:"name"`
	Coefficients []pulpExactCoeffJSON `json:"coefficients"`
	Constant     jsontext.Value       `json:"constant"`
}

type pulpExactVariableJSON struct {
	Name     string         `json:"name"`
	Cat      string         `json:"cat"`
	LowBound jsontext.Value `json:"lowBound"`
	UpBound  jsontext.Value `json:"upBound"`
	VarValue jsontext.Value `json:"varValue"`
	Dj       jsontext.Value `json:"dj"`
}

type pulpExactConstraintJSON struct {
	Name         string               `json:"name"`
	Sense        int                  `json:"sense"`
	Coefficients []pulpExactCoeffJSON `json:"coefficients"`
	Pi           jsontext.Value       `json:"pi"`
	Constant     jsontext.Value       `json:"constant"`
}

type pulpExactJSON struct {
	Parameters  PuLPParameters            `json:"parameters"`
	Objective   pulpExactObjectiveJSON    `json:"objective"`
	Variables   []pulpExactVariableJSON   `json:"variables"`
	Constraints []pulpExactConstraintJSON `json:"constraints"`
	SOS1        []any                     `json:"sos1"`
	SOS2        []any                     `json:"sos2"`
}

func isStrictJSONNumberToken(tok []byte) bool {
	i := 0
	n := len(tok)
	if i >= n {
		return false
	}
	if tok[i] == '-' {
		i++
		if i >= n {
			return false
		}
	}
	if tok[i] == '0' {
		i++
		if i < n && tok[i] >= '0' && tok[i] <= '9' {
			return false
		}
	} else if tok[i] >= '1' && tok[i] <= '9' {
		for i < n && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
	} else {
		return false
	}
	if i < n && tok[i] == '.' {
		i++
		if i >= n || tok[i] < '0' || tok[i] > '9' {
			return false
		}
		for i < n && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
	}
	if i < n && (tok[i] == 'e' || tok[i] == 'E') {
		i++
		if i < n && (tok[i] == '+' || tok[i] == '-') {
			i++
		}
		if i >= n || tok[i] < '0' || tok[i] > '9' {
			return false
		}
		for i < n && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
	}
	return i == n
}

func parsePuLPJSONRatToken(raw jsontext.Value, allowNull, defaultZero bool) (*big.Rat, bool, error) {
	tok := bytes.TrimSpace(raw)
	if len(tok) == 0 {
		if defaultZero {
			return new(big.Rat), false, nil
		}
		return nil, true, nil
	}
	if bytes.Equal(tok, []byte("null")) {
		if allowNull {
			return nil, false, nil
		}
		return nil, false, errors.New("unexpected null numeric value")
	}
	if !isStrictJSONNumberToken(tok) {
		return nil, false, fmt.Errorf("expected JSON number token, got %q", string(tok))
	}
	r, err := gb.ParseExactRat(string(tok))
	if err != nil {
		return nil, false, err
	}
	return r, false, nil
}

func resolvePuLPVarSpecExact(cat string, lb, ub *big.Rat) (*big.Rat, *big.Rat, gb.VarCategory, bool) {
	switch cat {
	case "Continuous":
		return lb, ub, gb.Continuous, true
	case "Binary":
		if lb == nil {
			lb = new(big.Rat)
		}
		if ub == nil {
			ub = big.NewRat(1, 1)
		}
		if lb.Sign() >= 0 && ub.Cmp(big.NewRat(1, 1)) <= 0 && lb.Cmp(ub) <= 0 {
			return lb, ub, gb.Binary, true
		}
		return lb, ub, gb.Integer, true
	case "Integer":
		if lb != nil && ub != nil && lb.Sign() >= 0 && ub.Cmp(big.NewRat(1, 1)) <= 0 && lb.Cmp(ub) <= 0 {
			return lb, ub, gb.Binary, true
		}
		return lb, ub, gb.Integer, true
	default:
		return nil, nil, 0, false
	}
}

func parsePuLPJSONExactStd(data []byte) (*gb.Model, error) {
	var doc pulpExactJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		if bytes.Contains(data, []byte("NaN")) || bytes.Contains(data, []byte("Infinity")) {
			sanitized := reIgnoredPythonNonFinite.ReplaceAll(data, []byte("${1}null"))
			if err2 := json.Unmarshal(sanitized, &doc); err2 == nil {
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("parse PuLP JSON: %w", err)
		}
	}
	if len(doc.SOS1) > 0 || len(doc.SOS2) > 0 {
		return nil, fmt.Errorf("PuLP JSON contains unsupported SOS constraints (sos1=%d, sos2=%d)", len(doc.SOS1), len(doc.SOS2))
	}
	sense, ok := parsePuLPSense(doc.Parameters.Sense)
	if !ok {
		return nil, fmt.Errorf("PuLP JSON has invalid objective sense %d", doc.Parameters.Sense)
	}
	m := gb.NewModel(doc.Parameters.Name, sense)
	m.Reserve(len(doc.Variables), len(doc.Constraints))

	for i := range doc.Variables {
		v := &doc.Variables[i]
		if v.Name == "" {
			return nil, errors.New("PuLP JSON contains variable with empty name")
		}
		if _, exists := m.VarByName(v.Name); exists {
			return nil, fmt.Errorf("PuLP JSON contains duplicate variable name %q", v.Name)
		}
		lb, lbOmitted, err := parsePuLPJSONRatToken(v.LowBound, true, false)
		if err != nil {
			return nil, fmt.Errorf("variable %q lowBound: %w", v.Name, err)
		}
		if lbOmitted {
			lb = new(big.Rat)
		}
		ub, _, err := parsePuLPJSONRatToken(v.UpBound, true, false)
		if err != nil {
			return nil, fmt.Errorf("variable %q upBound: %w", v.Name, err)
		}
		var cat gb.VarCategory
		lb, ub, cat, ok = resolvePuLPVarSpecExact(v.Cat, lb, ub)
		if !ok {
			return nil, fmt.Errorf("variable %q has invalid category %q", v.Name, v.Cat)
		}
		m.AddVarRat(v.Name, lb, ub, cat)
	}

	maxTerms := len(doc.Objective.Coefficients)
	for i := range doc.Constraints {
		if n := len(doc.Constraints[i].Coefficients); n > maxTerms {
			maxTerms = n
		}
	}
	var terms []gb.Term
	if maxTerms > 0 {
		terms = make([]gb.Term, 0, maxTerms)
	}

	for _, c := range doc.Objective.Coefficients {
		vid, exists := m.VarByName(c.Name)
		if !exists {
			return nil, fmt.Errorf("objective references unknown variable %q", c.Name)
		}
		r, _, err := parsePuLPJSONRatToken(c.Value, false, true)
		if err != nil {
			return nil, fmt.Errorf("objective coefficient for %q: %w", c.Name, err)
		}
		terms = append(terms, gb.TRat(vid, r))
	}
	objConst, _, err := parsePuLPJSONRatToken(doc.Objective.Constant, false, true)
	if err != nil {
		return nil, fmt.Errorf("objective constant: %w", err)
	}
	objName := ""
	if doc.Objective.Name != nil {
		objName = *doc.Objective.Name
	}
	objExpr := gb.NewExpr(terms...).AddConstantRat(objConst)
	m.SetObjectiveNamed(objName, objExpr)

	for i := range doc.Constraints {
		c := &doc.Constraints[i]
		terms = terms[:0]
		for _, coeff := range c.Coefficients {
			vid, exists := m.VarByName(coeff.Name)
			if !exists {
				return nil, fmt.Errorf("constraint %q references unknown variable %q", c.Name, coeff.Name)
			}
			r, _, err := parsePuLPJSONRatToken(coeff.Value, false, true)
			if err != nil {
				return nil, fmt.Errorf("constraint %q coefficient for %q: %w", c.Name, coeff.Name, err)
			}
			terms = append(terms, gb.TRat(vid, r))
		}
		csense, senseOK := parsePuLPConstraintSense(c.Sense)
		if !senseOK {
			return nil, fmt.Errorf("constraint %q has invalid sense %d", c.Name, c.Sense)
		}
		constRat, _, err := parsePuLPJSONRatToken(c.Constant, false, true)
		if err != nil {
			return nil, fmt.Errorf("constraint %q constant: %w", c.Name, err)
		}
		rhs := new(big.Rat).Neg(constRat)
		e := gb.NewExpr(terms...)
		switch csense {
		case gb.LessEqual:
			m.AddConstraint(c.Name, gb.LeRat(e, rhs))
		case gb.GreaterEqual:
			m.AddConstraint(c.Name, gb.GeRat(e, rhs))
		case gb.Equal:
			m.AddConstraint(c.Name, gb.EqRat(e, rhs))
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// FromPuLPJSONExactBytes decodes a PuLP JSON payload from bytes into a *gb.Model,
// preserving exact decimal/rational bounds, coefficients, and constants via big.Rat.
// Non-dyadic decimal coefficients such as 0.9 stay exact under certification.
func FromPuLPJSONExactBytes(data []byte) (*gb.Model, error) {
	if m, ok := fromPuLPJSONExactFast(data); ok {
		return m, nil
	}
	return parsePuLPJSONExactStd(data)
}

// FromPuLPJSONExact reads a PuLP JSON document from r and returns a *gb.Model
// preserving exact rational bounds, coefficients, and constants.
func FromPuLPJSONExact(r io.Reader) (*gb.Model, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return FromPuLPJSONExactBytes(data)
}

type pulpTokenWriter struct {
	enc *jsontext.Encoder
	err error
}

func (tw *pulpTokenWriter) tok(t jsontext.Token) {
	if tw.err != nil {
		return
	}
	tw.err = tw.enc.WriteToken(t)
}

func (tw *pulpTokenWriter) floatVal(v float64) {
	if tw.err != nil {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		tw.err = fmt.Errorf("cannot encode non-finite float %v in PuLP JSON", v)
		return
	}
	tw.err = tw.enc.WriteToken(jsontext.Float(v))
}

func (tw *pulpTokenWriter) coeffObj(name string, val float64) {
	if tw.err != nil {
		return
	}
	tw.tok(jsontext.BeginObject)
	tw.tok(tokName)
	tw.tok(jsontext.String(name))
	tw.tok(tokValue)
	tw.floatVal(val)
	tw.tok(jsontext.EndObject)
}

func writePuLPJSON(p *gb.CompiledProblem, w io.Writer, indent bool) (retErr error) {
	if p == nil {
		return gb.ErrNilProblem
	}
	if err := p.Err(); err != nil {
		return err
	}

	enc, _ := pulpEncPool.Get().(*jsontext.Encoder)
	if enc == nil {
		enc = jsontext.NewEncoder(io.Discard)
	}
	if indent {
		enc.Reset(w, pulpIndentOpt)
	} else {
		enc.Reset(w)
	}
	defer func() {
		enc.Reset(io.Discard)
		if retErr == nil {
			pulpEncPool.Put(enc)
		}
	}()

	tw := pulpTokenWriter{enc: enc}
	numVars := p.NumVars()
	numRows := p.NumConstraints()

	tw.tok(jsontext.BeginObject)

	// Write parameters section.
	tw.tok(tokParameters)
	tw.tok(jsontext.BeginObject)
	tw.tok(tokName)
	tw.tok(jsontext.String(p.Name()))
	tw.tok(tokSense)
	tw.tok(jsontext.Int(int64(p.Sense())))
	tw.tok(tokStatus)
	tw.tok(tokZeroInt)
	tw.tok(tokSolStatus)
	tw.tok(tokZeroInt)
	tw.tok(jsontext.EndObject)

	// Write objective section.
	tw.tok(tokObjective)
	tw.tok(jsontext.BeginObject)
	tw.tok(tokName)
	if objName := p.ObjName(); objName != "" {
		tw.tok(jsontext.String(objName))
	} else {
		tw.tok(jsontext.Null)
	}
	tw.tok(tokCoefficients)
	tw.tok(jsontext.BeginArray)
	for j := range numVars {
		if c := p.ObjCoeff(j); c != 0 {
			tw.coeffObj(p.VarName(j), c)
			if tw.err != nil {
				return tw.err
			}
		}
	}
	tw.tok(jsontext.EndArray)
	if objConst := p.ObjConstant(); objConst != 0 {
		tw.tok(tokConstant)
		tw.floatVal(objConst)
	}
	tw.tok(jsontext.EndObject)

	// Write variables section.
	tw.tok(tokVariables)
	tw.tok(jsontext.BeginArray)
	for j := range numVars {
		tw.tok(jsontext.BeginObject)
		tw.tok(tokName)
		tw.tok(jsontext.String(p.VarName(j)))
		tw.tok(tokCat)
		if cat := p.VarCategory(j); cat == gb.Integer || cat == gb.Binary {
			// PuLP stores both Binary and Integer as "Integer" with [0, 1] bounds in toDict().
			tw.tok(tokInteger)
		} else {
			tw.tok(tokContinuous)
		}
		lb, ub := p.VarBounds(j)
		tw.tok(tokLowBound)
		if math.IsInf(lb, -1) {
			tw.tok(jsontext.Null)
		} else {
			tw.floatVal(lb)
		}
		tw.tok(tokUpBound)
		if math.IsInf(ub, 1) {
			tw.tok(jsontext.Null)
		} else {
			tw.floatVal(ub)
		}
		tw.tok(tokVarValue)
		tw.tok(jsontext.Null)
		tw.tok(tokDj)
		tw.tok(jsontext.Null)
		tw.tok(jsontext.EndObject)
		if tw.err != nil {
			return tw.err
		}
	}
	tw.tok(jsontext.EndArray)

	// Write constraints section.
	tw.tok(tokConstraints)
	tw.tok(jsontext.BeginArray)
	rowTermFn := func(col int, val float64) {
		if tw.err != nil {
			return
		}
		tw.coeffObj(p.VarName(col), val)
	}
	for i := range numRows {
		tw.tok(jsontext.BeginObject)
		tw.tok(tokName)
		tw.tok(jsontext.String(p.RowName(i)))
		tw.tok(tokSense)
		tw.tok(jsontext.Int(int64(p.RowSense(i))))
		tw.tok(tokCoefficients)
		tw.tok(jsontext.BeginArray)
		p.ForEachRowTerm(i, rowTermFn)
		if tw.err != nil {
			return tw.err
		}
		tw.tok(jsontext.EndArray)
		tw.tok(tokPi)
		tw.tok(jsontext.Null)
		tw.tok(tokConstant)
		tw.floatVal(-p.RowRHS(i))
		tw.tok(jsontext.EndObject)
		if tw.err != nil {
			return tw.err
		}
	}
	tw.tok(jsontext.EndArray)

	// Write empty SOS1 and SOS2 sections.
	tw.tok(tokSOS1)
	tw.tok(jsontext.BeginArray)
	tw.tok(jsontext.EndArray)
	tw.tok(tokSOS2)
	tw.tok(jsontext.BeginArray)
	tw.tok(jsontext.EndArray)

	tw.tok(jsontext.EndObject)
	return tw.err
}

// ToPuLPJSON serializes a CompiledProblem into PuLP's JSON schema.
// Nonzero objective offsets are emitted under objective.constant (a goblin extension
// ignored by Python PuLP's LpProblem.fromJson).
func ToPuLPJSON(p *gb.CompiledProblem, w io.Writer) error {
	return writePuLPJSON(p, w, true)
}

// ToPuLPJSONCompact serializes a CompiledProblem into a single-line compact PuLP JSON record.
func ToPuLPJSONCompact(p *gb.CompiledProblem, w io.Writer) error {
	return writePuLPJSON(p, w, false)
}
