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
	"regexp"

	gb "github.com/paulourio/goblin"
)

var reIgnoredPythonNonFinite = regexp.MustCompile(`("(?:varValue|dj|pi)"\s*:\s*)(?:-?Infinity|NaN)\b`)

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

// BuildModelFromPuLPJSON constructs a *gb.Model from a decoded PuLPJSON structure.
func BuildModelFromPuLPJSON(doc *PuLPJSON) (*gb.Model, error) {
	if doc == nil {
		return nil, errors.New("nil PuLPJSON document")
	}
	if len(doc.SOS1) > 0 || len(doc.SOS2) > 0 {
		return nil, fmt.Errorf("PuLP JSON contains unsupported SOS constraints (sos1=%d, sos2=%d)", len(doc.SOS1), len(doc.SOS2))
	}
	var sense gb.Sense
	switch doc.Parameters.Sense {
	case 1:
		sense = gb.Minimize
	case -1:
		sense = gb.Maximize
	default:
		return nil, fmt.Errorf("PuLP JSON has invalid objective sense %d", doc.Parameters.Sense)
	}
	m := gb.NewModel(doc.Parameters.Name, sense)
	m.Reserve(len(doc.Variables), len(doc.Constraints))

	varMap := make(map[string]gb.Var, len(doc.Variables))
	for _, v := range doc.Variables {
		if v.Name == "" {
			return nil, errors.New("PuLP JSON contains variable with empty name")
		}
		if _, exists := varMap[v.Name]; exists {
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
		var cat gb.VarCategory
		switch v.Cat {
		case "Continuous":
			cat = gb.Continuous
		case "Binary":
			if v.LowBound == nil {
				lb = 0
			}
			if v.UpBound == nil {
				ub = 1
			}
			if lb >= 0 && ub <= 1 {
				cat = gb.Binary
			} else {
				cat = gb.Integer
			}
		case "Integer":
			if lb >= 0 && ub <= 1 {
				cat = gb.Binary
			} else {
				cat = gb.Integer
			}
		default:
			return nil, fmt.Errorf("variable %q has invalid category %q", v.Name, v.Cat)
		}
		id := m.AddVar(v.Name, lb, ub, cat)
		varMap[v.Name] = id
	}

	objTerms := make([]gb.Term, 0, len(doc.Objective.Coefficients))
	for _, c := range doc.Objective.Coefficients {
		vid, ok := varMap[c.Name]
		if !ok {
			return nil, fmt.Errorf("objective references unknown variable %q", c.Name)
		}
		objTerms = append(objTerms, gb.Term{Var: vid, Coeff: c.Value})
	}
	objName := ""
	if doc.Objective.Name != nil {
		objName = *doc.Objective.Name
	}
	m.SetObjectiveNamed(objName, gb.Expr{Terms: objTerms, Constant: doc.Objective.Constant})

	for _, c := range doc.Constraints {
		terms := make([]gb.Term, 0, len(c.Coefficients))
		for _, coeff := range c.Coefficients {
			vid, ok := varMap[coeff.Name]
			if !ok {
				return nil, fmt.Errorf("constraint %q references unknown variable %q", c.Name, coeff.Name)
			}
			terms = append(terms, gb.Term{Var: vid, Coeff: coeff.Value})
		}
		var csense gb.ConstraintSense
		switch c.Sense {
		case -1:
			csense = gb.LessEqual
		case 0:
			csense = gb.Equal
		case 1:
			csense = gb.GreaterEqual
		default:
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

// FromPuLPJSON reads a PuLP JSON document from r and returns a *gb.Model.
func FromPuLPJSON(r io.Reader) (*gb.Model, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	doc, err := ParsePuLPJSON(data)
	if err != nil {
		return nil, err
	}
	return BuildModelFromPuLPJSON(doc)
}

func toPuLPJSONDoc(p *gb.CompiledProblem) PuLPJSON {
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
		cat := p.VarCategory(j)
		if cat == gb.Integer || cat == gb.Binary {
			// PuLP stores both Binary and Integer as "Integer" with [0, 1] bounds in toDict()
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

var trailingNewline = [1]byte{'\n'}

// ToPuLPJSON serializes a CompiledProblem into PuLP's JSON schema.
// Nonzero objective offsets are emitted under objective.constant (a goblin extension
// ignored by Python PuLP's LpProblem.fromJson).
func ToPuLPJSON(p *gb.CompiledProblem, w io.Writer) error {
	if p == nil {
		return gb.ErrNilProblem
	}
	if err := p.Err(); err != nil {
		return err
	}
	doc := toPuLPJSONDoc(p)
	if err := json.MarshalWrite(w, &doc, jsontext.WithIndent("  ")); err != nil {
		return err
	}
	_, err := w.Write(trailingNewline[:])
	return err
}

// ToPuLPJSONCompact serializes a CompiledProblem into a single-line compact PuLP JSON record.
func ToPuLPJSONCompact(p *gb.CompiledProblem, w io.Writer) error {
	if p == nil {
		return gb.ErrNilProblem
	}
	if err := p.Err(); err != nil {
		return err
	}
	doc := toPuLPJSONDoc(p)
	if err := json.MarshalWrite(w, &doc); err != nil {
		return err
	}
	_, err := w.Write(trailingNewline[:])
	return err
}
