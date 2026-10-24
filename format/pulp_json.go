// Package format provides serialization and deserialization for PuLP JSON,
// MPS, and CPLEX LP problem representations.
package format

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	gb "github.com/paulourio/goblin"
)

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
type PuLPObjective struct {
	Name         *string           `json:"name"`
	Coefficients []PuLPCoefficient `json:"coefficients"`
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

// ParsePuLPJSON decodes a PuLP JSON payload from bytes without building a gb.Model yet.
func ParsePuLPJSON(data []byte) (*PuLPJSON, error) {
	var doc PuLPJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse PuLP JSON: %w", err)
	}
	return &doc, nil
}

// BuildModelFromPuLPJSON constructs a *gb.Model from a decoded PuLPJSON structure.
func BuildModelFromPuLPJSON(doc *PuLPJSON) (*gb.Model, error) {
	sense := gb.Minimize
	if doc.Parameters.Sense == -1 {
		sense = gb.Maximize
	}
	m := gb.NewModel(doc.Parameters.Name, sense)

	varMap := make(map[string]gb.Var, len(doc.Variables))
	for _, v := range doc.Variables {
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
		case "Binary":
			cat = gb.Binary
		case "Integer":
			if lb >= 0 && ub <= 1 {
				cat = gb.Binary
			} else {
				cat = gb.Integer
			}
		default:
			cat = gb.Continuous
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
	m.SetObjectiveNamed(objName, gb.Expr{Terms: objTerms})

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
		case 1:
			csense = gb.GreaterEqual
		default:
			csense = gb.Equal
		}
		rhs := -c.Constant
		m.AddConstraint(c.Name, gb.Constraint{
			Expr:  gb.Expr{Terms: terms},
			Sense: csense,
			RHS:   rhs,
		})
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
	doc := PuLPJSON{
		Parameters: PuLPParameters{
			Name:  p.Name,
			Sense: int(p.Sense),
		},
		Variables:   make([]PuLPVariable, p.NumVars),
		Constraints: make([]PuLPConstraint, p.NumConstraints),
		SOS1:        []any{},
		SOS2:        []any{},
	}
	if p.ObjName != "" {
		nameCopy := p.ObjName
		doc.Objective.Name = &nameCopy
	}
	doc.Objective.Coefficients = make([]PuLPCoefficient, 0, p.NumVars)
	for j := range p.NumVars {
		if p.ObjCoeffs[j] != 0 {
			doc.Objective.Coefficients = append(doc.Objective.Coefficients, PuLPCoefficient{
				Name:  p.VarNames[j],
				Value: p.ObjCoeffs[j],
			})
		}
		var lbPtr, ubPtr *float64
		if !math.IsInf(p.VarLB[j], -1) {
			v := p.VarLB[j]
			lbPtr = &v
		}
		if !math.IsInf(p.VarUB[j], 1) {
			v := p.VarUB[j]
			ubPtr = &v
		}
		catStr := "Continuous"
		if p.VarCat[j] == gb.Integer || p.VarCat[j] == gb.Binary {
			// PuLP stores both Binary and Integer as "Integer" with [0, 1] bounds in toDict()
			catStr = "Integer"
		}
		doc.Variables[j] = PuLPVariable{
			Name:     p.VarNames[j],
			Cat:      catStr,
			LowBound: lbPtr,
			UpBound:  ubPtr,
		}
	}

	for i := range p.NumConstraints {
		rowLen := p.RowPtr[i+1] - p.RowPtr[i]
		coeffs := make([]PuLPCoefficient, 0, rowLen)
		for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
			coeffs = append(coeffs, PuLPCoefficient{
				Name:  p.VarNames[p.ColIdx[k]],
				Value: p.RowVal[k],
			})
		}
		doc.Constraints[i] = PuLPConstraint{
			Name:         p.RowNames[i],
			Sense:        int(p.RowSense[i]),
			Coefficients: coeffs,
			Constant:     -p.RowRHS[i],
		}
	}
	return doc
}

// ToPuLPJSON serializes a CompiledProblem into PuLP's native JSON schema so both
// Python PuLP (LpProblem.fromJson) and goblin can consume the exact same JSON file.
func ToPuLPJSON(p *gb.CompiledProblem, w io.Writer) error {
	doc := toPuLPJSONDoc(p)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&doc)
}

// ToPuLPJSONCompact serializes a CompiledProblem into a single-line compact PuLP JSON record.
func ToPuLPJSONCompact(p *gb.CompiledProblem, w io.Writer) error {
	doc := toPuLPJSONDoc(p)
	enc := json.NewEncoder(w)
	return enc.Encode(&doc)
}
