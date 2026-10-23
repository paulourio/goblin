// Package goblin provides an idiomatic Go linear and mixed-integer linear
// programming (MILP) modeling API and branch-and-cut solver with selectable
// pure-Go and C++ COIN-OR CBC/CLP backends.
package goblin

import (
	"fmt"
	"math"
	"sync/atomic"
)

var nextModelID atomic.Uint32

// Sense defines the optimization direction (Minimize or Maximize).
type Sense int8

const (
	Minimize Sense = 1
	Maximize Sense = -1
)

func (s Sense) String() string {
	if s == Maximize {
		return "Maximize"
	}
	return "Minimize"
}

// VarCategory defines the domain type of a decision variable.
type VarCategory uint8

const (
	Continuous VarCategory = iota
	Integer
	Binary
)

func (c VarCategory) String() string {
	switch c {
	case Integer:
		return "Integer"
	case Binary:
		return "Binary"
	default:
		return "Continuous"
	}
}

// ConstraintSense defines the relational operator of a linear constraint.
type ConstraintSense int8

const (
	LessEqual    ConstraintSense = -1
	Equal        ConstraintSense = 0
	GreaterEqual ConstraintSense = 1
)

func (s ConstraintSense) String() string {
	switch s {
	case LessEqual:
		return "<="
	case GreaterEqual:
		return ">="
	default:
		return "="
	}
}

func (s ConstraintSense) valid() bool {
	return s == LessEqual || s == Equal || s == GreaterEqual
}

// Status represents the termination status of a solve call, matching PuLP's LpSolveStatus codes.
type Status int8

const (
	StatusNotSolved      Status = 0
	StatusOptimal        Status = 1
	StatusInfeasible     Status = -1
	StatusUnbounded      Status = -2
	StatusUndefined      Status = -3
	StatusTimeLimit      Status = -4
	StatusNodeLimit      Status = -6
	StatusGapLimit       Status = -7
	StatusIterationLimit Status = -8
	StatusNumericalError Status = -12
)

func (s Status) String() string {
	switch s {
	case StatusOptimal:
		return "Optimal"
	case StatusInfeasible:
		return "Infeasible"
	case StatusUnbounded:
		return "Unbounded"
	case StatusTimeLimit:
		return "TimeLimit"
	case StatusNodeLimit:
		return "NodeLimit"
	case StatusGapLimit:
		return "GapLimit"
	case StatusIterationLimit:
		return "IterationLimit"
	case StatusNumericalError:
		return "NumericalError"
	case StatusUndefined:
		return "Undefined"
	default:
		return "NotSolved"
	}
}

// Var is a lightweight 64-bit value handle identifying a variable inside a Model
// (upper 32 bits encode the owning Model ID; lower 32 bits encode the 0-based variable index).
type Var uint64

const (
	varModelShift    = 32
	defaultModelCap  = 64
	intBoundRoundTol = 1e-12
)

func makeVar(modelID uint32, idx int) Var {
	return Var(uint64(modelID)<<varModelShift | uint64(uint32(idx)))
}

// Index returns the 0-based column index of v within its parent Model.
func (v Var) Index() int {
	return int(uint32(v))
}

func (v Var) modelID() uint32 {
	return uint32(v >> varModelShift)
}

// varDef holds the internal specification of a decision variable.
type varDef struct {
	name string
	lb   float64
	ub   float64
	cat  VarCategory
}

// constraintDef holds the internal specification of a linear constraint.
type constraintDef struct {
	name  string
	terms []Term
	sense ConstraintSense
	rhs   float64
}

// Model represents an algebraic LP or MILP optimization problem.
type Model struct {
	id          uint32
	name        string
	sense       Sense
	vars        []varDef
	varByName   map[string]Var
	constraints []constraintDef
	objTerms    []Term
	objConst    float64
	objName     string
	err         error
}

// NewModel creates a new optimization Model with the given name and objective sense.
func NewModel(name string, sense Sense) *Model {
	if sense != Maximize {
		sense = Minimize
	}
	if name == "" {
		name = "NoName"
	}
	return &Model{
		id:        nextModelID.Add(1),
		name:      name,
		sense:     sense,
		vars:      make([]varDef, 0, defaultModelCap),
		varByName: make(map[string]Var, defaultModelCap),
	}
}

func (m *Model) recordErr(err error) {
	if m.err == nil && err != nil {
		m.err = err
	}
}

func (m *Model) validVar(v Var) (int, bool) {
	idx := v.Index()
	if v.modelID() != m.id || idx < 0 || idx >= len(m.vars) {
		return -1, false
	}
	return idx, true
}

// Name returns the model's name.
func (m *Model) Name() string { return m.name }

// Sense returns the model's optimization sense.
func (m *Model) Sense() Sense { return m.sense }

// NumVars returns the number of variables in the model.
func (m *Model) NumVars() int { return len(m.vars) }

// NumConstraints returns the number of constraints in the model.
func (m *Model) NumConstraints() int { return len(m.constraints) }

// AddVar adds a decision variable with explicit lower bound, upper bound, and category.
// Use math.Inf(-1) for no lower bound and math.Inf(1) for no upper bound.
func (m *Model) AddVar(name string, lb, ub float64, cat VarCategory) Var {
	if math.IsNaN(lb) || math.IsNaN(ub) || math.IsInf(lb, 1) || math.IsInf(ub, -1) {
		m.recordErr(fmt.Errorf("variable %q has invalid bounds [%v, %v]", name, lb, ub))
	}
	switch cat {
	case Binary:
		if lb < 0 || math.IsInf(lb, -1) {
			lb = 0
		}
		if ub > 1 || math.IsInf(ub, 1) {
			ub = 1
		}
		if !math.IsNaN(lb) {
			lb = math.Ceil(lb - intBoundRoundTol)
		}
		if !math.IsNaN(ub) {
			ub = math.Floor(ub + intBoundRoundTol)
		}
	case Integer:
		if !math.IsInf(lb, 0) && !math.IsNaN(lb) {
			lb = math.Ceil(lb - intBoundRoundTol)
		}
		if !math.IsInf(ub, 0) && !math.IsNaN(ub) {
			ub = math.Floor(ub + intBoundRoundTol)
		}
		if lb >= 0 && ub <= 1 && lb <= ub {
			cat = Binary
		}
	}
	idx := len(m.vars)
	id := makeVar(m.id, idx)
	if name == "" {
		name = fmt.Sprintf("x_%d", idx)
		for seq := 1; ; seq++ {
			if _, exists := m.varByName[name]; !exists {
				break
			}
			name = fmt.Sprintf("x_%d_%d", idx, seq)
		}
	} else if _, exists := m.varByName[name]; exists {
		m.recordErr(fmt.Errorf("duplicate variable name %q", name))
	}
	m.vars = append(m.vars, varDef{
		name: name,
		lb:   lb,
		ub:   ub,
		cat:  cat,
	})
	m.varByName[name] = id
	return id
}

// AddContVar adds a continuous variable with bounds [lb, ub].
func (m *Model) AddContVar(name string, lb, ub float64) Var {
	return m.AddVar(name, lb, ub, Continuous)
}

// AddIntVar adds a general integer variable with bounds [lb, ub].
func (m *Model) AddIntVar(name string, lb, ub float64) Var {
	return m.AddVar(name, lb, ub, Integer)
}

// AddBinaryVar adds a 0-1 binary decision variable.
func (m *Model) AddBinaryVar(name string) Var {
	return m.AddVar(name, 0, 1, Binary)
}

// VarByName looks up a variable handle by name.
func (m *Model) VarByName(name string) (Var, bool) {
	v, ok := m.varByName[name]
	return v, ok
}

// VarName returns the name of variable v.
func (m *Model) VarName(v Var) string {
	idx, ok := m.validVar(v)
	if !ok {
		return ""
	}
	return m.vars[idx].name
}

// VarBounds returns the lower and upper bounds of variable v.
func (m *Model) VarBounds(v Var) (lb, ub float64) {
	idx, ok := m.validVar(v)
	if !ok {
		return 0, 0
	}
	d := m.vars[idx]
	return d.lb, d.ub
}

// SetVarBounds updates the lower and upper bounds of variable v.
func (m *Model) SetVarBounds(v Var, lb, ub float64) {
	idx, ok := m.validVar(v)
	if !ok {
		m.recordErr(fmt.Errorf("SetVarBounds called with invalid or foreign Var handle %v", v))
		return
	}
	if math.IsNaN(lb) || math.IsNaN(ub) || math.IsInf(lb, 1) || math.IsInf(ub, -1) {
		m.recordErr(fmt.Errorf("variable %q has invalid bounds [%v, %v]", m.vars[idx].name, lb, ub))
	}
	cat := m.vars[idx].cat
	switch cat {
	case Binary:
		if lb < 0 || math.IsInf(lb, -1) {
			lb = 0
		}
		if ub > 1 || math.IsInf(ub, 1) {
			ub = 1
		}
		if !math.IsNaN(lb) {
			lb = math.Ceil(lb - intBoundRoundTol)
		}
		if !math.IsNaN(ub) {
			ub = math.Floor(ub + intBoundRoundTol)
		}
	case Integer:
		if !math.IsInf(lb, 0) && !math.IsNaN(lb) {
			lb = math.Ceil(lb - intBoundRoundTol)
		}
		if !math.IsInf(ub, 0) && !math.IsNaN(ub) {
			ub = math.Floor(ub + intBoundRoundTol)
		}
	}
	m.vars[idx].lb = lb
	m.vars[idx].ub = ub
}

// VarCategoryOf returns the category of variable v.
func (m *Model) VarCategoryOf(v Var) VarCategory {
	idx, ok := m.validVar(v)
	if !ok {
		return Continuous
	}
	return m.vars[idx].cat
}

// SetObjective sets the linear objective expression of the model.
func (m *Model) SetObjective(e Expr) {
	m.objTerms = append(m.objTerms[:0], e.Terms...)
	m.objConst = e.Constant
}

// SetObjectiveNamed sets the linear objective expression and its name.
func (m *Model) SetObjectiveNamed(name string, e Expr) {
	m.objName = name
	m.SetObjective(e)
}

// AddConstraint adds a linear constraint to the model.
func (m *Model) AddConstraint(name string, c Constraint) {
	if name == "" {
		name = fmt.Sprintf("_C%d", len(m.constraints)+1)
	}
	if !c.Sense.valid() {
		m.recordErr(fmt.Errorf("constraint %q has invalid sense %d", name, int8(c.Sense)))
	}
	// Normalize: Expr + Constant (<=, =, >=) RHS  =>  Terms (<=, =, >=) RHS - Expr.Constant
	rhs := c.RHS - c.Expr.Constant
	termsCopy := make([]Term, len(c.Expr.Terms))
	copy(termsCopy, c.Expr.Terms)
	m.constraints = append(m.constraints, constraintDef{
		name:  name,
		terms: termsCopy,
		sense: c.Sense,
		rhs:   rhs,
	})
}

// IsMIP returns true if any variable in the model is Integer or Binary.
func (m *Model) IsMIP() bool {
	for i := range m.vars {
		if m.vars[i].cat != Continuous {
			return true
		}
	}
	return false
}

// Validate checks that all variable bounds, objective terms, and constraint terms
// belong to this Model and contain only finite coefficients and valid bounds.
func (m *Model) Validate() error {
	if m.err != nil {
		return m.err
	}
	for _, v := range m.vars {
		if math.IsNaN(v.lb) || math.IsNaN(v.ub) || math.IsInf(v.lb, 1) || math.IsInf(v.ub, -1) {
			return fmt.Errorf("variable %q has invalid bounds [%v, %v]", v.name, v.lb, v.ub)
		}
	}
	if math.IsNaN(m.objConst) || math.IsInf(m.objConst, 0) {
		return fmt.Errorf("objective constant is non-finite (%v)", m.objConst)
	}
	for _, t := range m.objTerms {
		if _, ok := m.validVar(t.Var); !ok {
			return fmt.Errorf("objective references invalid or foreign Var handle %v", t.Var)
		}
		if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
			return fmt.Errorf("objective has non-finite coefficient %v", t.Coeff)
		}
	}
	for _, c := range m.constraints {
		if !c.sense.valid() {
			return fmt.Errorf("constraint %q has invalid sense %d", c.name, int8(c.sense))
		}
		if math.IsNaN(c.rhs) || math.IsInf(c.rhs, 0) {
			return fmt.Errorf("constraint %q has non-finite RHS %v", c.name, c.rhs)
		}
		for _, t := range c.terms {
			if _, ok := m.validVar(t.Var); !ok {
				return fmt.Errorf("constraint %q references invalid or foreign Var handle %v", c.name, t.Var)
			}
			if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
				return fmt.Errorf("constraint %q has non-finite coefficient %v", c.name, t.Coeff)
			}
		}
	}
	return nil
}
