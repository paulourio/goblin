// Package goblin provides an idiomatic Go linear and mixed-integer linear
// programming (MILP) modeling API and branch-and-cut solver with selectable
// pure-Go and C++ COIN-OR CBC/CLP backends.
package goblin

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
)

// ErrNilModel indicates that a nil *Model receiver was passed to Validate.
var ErrNilModel = errors.New("goblin: nil Model")

var nextModelID atomic.Uint64

// Sense defines the optimization direction (Minimize or Maximize).
type Sense int8

// Optimization directions supported by Model.
const (
	Minimize Sense = 1
	Maximize Sense = -1
)

// String returns "Minimize" or "Maximize".
func (s Sense) String() string {
	if s == Maximize {
		return "Maximize"
	}
	return "Minimize"
}

// VarCategory defines the domain type of a decision variable.
type VarCategory uint8

// Variable domain categories supported by Model.
const (
	Continuous VarCategory = iota
	Integer
	Binary
)

// String returns the variable category name ("Continuous", "Integer", or "Binary").
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

func (c VarCategory) valid() bool {
	return c == Continuous || c == Integer || c == Binary
}

// ConstraintSense defines the relational operator of a linear constraint.
type ConstraintSense int8

// Relational operators supported for linear constraints.
const (
	LessEqual    ConstraintSense = -1
	Equal        ConstraintSense = 0
	GreaterEqual ConstraintSense = 1
)

// String returns the relational symbol ("<=", "=", or ">=").
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

// Status represents the termination status of a solve call. Codes 0, 1, -1, -2, and -3
// match PuLP's LpStatus codes; codes -4 through -12 report resource limits and numerical errors.
type Status int8

// Termination status codes returned in Result.Status and LPResult.Status.
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

// String returns the canonical status label (such as "Optimal" or "Infeasible").
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
	case StatusNotSolved:
		return "NotSolved"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// Var is a lightweight 64-bit value handle identifying a variable inside a Model
// (upper 32 bits encode a non-zero owning Model tag; lower 32 bits encode the 0-based variable index).
type Var uint64

const (
	varModelShift    = 32
	defaultModelCap  = 64
	intBoundRoundTol = 1e-12
)

func modelVarTag(modelID uint64) uint32 {
	if modelID == 0 {
		return 0
	}
	return uint32((modelID-1)%uint64(math.MaxUint32)) + 1
}

func makeVar(modelID uint64, idx int) Var {
	return Var(uint64(modelVarTag(modelID))<<varModelShift | uint64(uint32(idx)))
}

// Index returns the 0-based column index of v within its parent Model.
func (v Var) Index() int {
	return int(uint32(v))
}

func (v Var) modelTag() uint32 {
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
// Its terms reside in Model.conTerms[termStart:termEnd].
type constraintDef struct {
	name      string
	rhs       float64
	termStart int
	termEnd   int
	sense     ConstraintSense
}

// Model represents an algebraic LP or MILP optimization problem.
type Model struct {
	name        string
	objName     string
	err         error
	vars        []varDef
	varByName   map[string]Var
	autoVarName map[string]int
	constraints []constraintDef
	conTerms    []Term
	conByName   map[string]int
	autoConName map[string]int
	objTerms    []Term
	id          uint64
	objConst    float64
	varTag      uint32
	sense       Sense
}

// NewModel creates a new optimization Model with the given name and objective sense.
func NewModel(name string, sense Sense) *Model {
	var initErr error
	if sense != Minimize && sense != Maximize {
		initErr = fmt.Errorf("model %q has invalid optimization sense %d", name, int8(sense))
		sense = Minimize
	}
	if name == "" {
		name = "NoName"
	}
	id := nextModelID.Add(1)
	return &Model{
		id:        id,
		varTag:    modelVarTag(id),
		name:      name,
		sense:     sense,
		vars:      make([]varDef, 0, defaultModelCap),
		varByName: make(map[string]Var, defaultModelCap),
		conByName: make(map[string]int, defaultModelCap),
		err:       initErr,
	}
}

func (m *Model) recordErr(err error) {
	if m.err == nil && err != nil {
		m.err = err
	}
}

func (m *Model) ensureInit() {
	if m.id == 0 {
		m.id = nextModelID.Add(1)
		m.varTag = modelVarTag(m.id)
	}
	if m.sense == 0 {
		m.sense = Minimize
	}
	if m.name == "" {
		m.name = "NoName"
	}
}

func (m *Model) validVar(v Var) (int, bool) {
	idx := v.Index()
	if v == 0 || v.modelTag() != m.varTag || idx < 0 || idx >= len(m.vars) {
		return -1, false
	}
	return idx, true
}

// Name returns the model's name ("NoName" on an unnamed zero-value Model).
func (m *Model) Name() string {
	if m == nil {
		return ""
	}
	if m.name == "" {
		return "NoName"
	}
	return m.name
}

// Sense returns the model's optimization sense (Minimize on a zero-value Model).
func (m *Model) Sense() Sense {
	if m == nil || m.sense == 0 {
		return Minimize
	}
	return m.sense
}

// NumVars returns the number of variables in the model.
func (m *Model) NumVars() int { return len(m.vars) }

// NumConstraints returns the number of constraints in the model.
func (m *Model) NumConstraints() int { return len(m.constraints) }

// Reserve preallocates internal variable and constraint storage when the model dimensions are known ahead of time.
func (m *Model) Reserve(numVars, numConstraints int) {
	m.ensureInit()
	if numVars > cap(m.vars) {
		vars := make([]varDef, len(m.vars), numVars)
		copy(vars, m.vars)
		m.vars = vars
		if len(m.varByName) == 0 {
			m.varByName = make(map[string]Var, numVars)
		}
	}
	if numConstraints > cap(m.constraints) {
		cons := make([]constraintDef, len(m.constraints), numConstraints)
		copy(cons, m.constraints)
		m.constraints = cons
		if len(m.conByName) == 0 {
			m.conByName = make(map[string]int, numConstraints)
		}
		if len(m.conTerms) == 0 {
			termCap := min(numConstraints*4, 65536)
			if termCap > cap(m.conTerms) {
				m.conTerms = make([]Term, 0, termCap)
			}
		}
	}
}

func roundBoundsForCategory(lb, ub float64, cat VarCategory) (float64, float64) {
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
	if lb == 0 {
		lb = 0
	}
	if ub == 0 {
		ub = 0
	}
	return lb, ub
}

func (m *Model) nextAutoVarName(idx int) string {
	cand := fmt.Sprintf("x_%d", idx)
	for seq := 1; ; seq++ {
		if _, exists := m.varByName[cand]; !exists {
			return cand
		}
		cand = fmt.Sprintf("x_%d_%d", idx, seq)
	}
}

func (m *Model) nextAutoConName(idx int) string {
	base := idx + 1
	cand := fmt.Sprintf("_C%d", base)
	for seq := 1; ; seq++ {
		if _, exists := m.conByName[cand]; !exists {
			return cand
		}
		cand = fmt.Sprintf("_C%d_%d", base, seq)
	}
}

// AddVar adds a decision variable with explicit lower bound, upper bound, and category.
// Use math.Inf(-1) for no lower bound and math.Inf(1) for no upper bound.
func (m *Model) AddVar(name string, lb, ub float64, cat VarCategory) Var {
	m.ensureInit()
	if !cat.valid() {
		m.recordErr(fmt.Errorf("variable %q has invalid category %d", name, uint8(cat)))
	}
	if math.IsNaN(lb) || math.IsNaN(ub) || math.IsInf(lb, 1) || math.IsInf(ub, -1) {
		m.recordErr(fmt.Errorf("variable %q has invalid bounds [%v, %v]", name, lb, ub))
	}
	lb, ub = roundBoundsForCategory(lb, ub, cat)
	if m.varByName == nil {
		m.varByName = make(map[string]Var, defaultModelCap)
	}
	idx := len(m.vars)
	id := makeVar(m.id, idx)
	if name == "" {
		name = m.nextAutoVarName(idx)
		if m.autoVarName == nil {
			m.autoVarName = make(map[string]int)
		}
		m.autoVarName[name] = idx
	} else if prevIdx, isAuto := m.autoVarName[name]; isAuto {
		delete(m.autoVarName, name)
		m.varByName[name] = id
		newAuto := m.nextAutoVarName(prevIdx)
		m.vars[prevIdx].name = newAuto
		m.varByName[newAuto] = makeVar(m.id, prevIdx)
		m.autoVarName[newAuto] = prevIdx
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
		return
	}
	lb, ub = roundBoundsForCategory(lb, ub, m.vars[idx].cat)
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
	m.ensureInit()
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
	m.ensureInit()
	cIdx := len(m.constraints)
	if m.conByName == nil {
		m.conByName = make(map[string]int, defaultModelCap)
	}
	if name == "" {
		name = m.nextAutoConName(cIdx)
		if m.autoConName == nil {
			m.autoConName = make(map[string]int)
		}
		m.autoConName[name] = cIdx
	} else if prevIdx, isAuto := m.autoConName[name]; isAuto {
		delete(m.autoConName, name)
		m.conByName[name] = cIdx
		newAuto := m.nextAutoConName(prevIdx)
		m.constraints[prevIdx].name = newAuto
		m.conByName[newAuto] = prevIdx
		m.autoConName[newAuto] = prevIdx
	} else if _, exists := m.conByName[name]; exists {
		m.recordErr(fmt.Errorf("duplicate constraint name %q", name))
	}
	if !c.Sense.valid() {
		m.recordErr(fmt.Errorf("constraint %q has invalid sense %d", name, int8(c.Sense)))
	}
	// Shift Expr.Constant to the right-hand side so Terms (<=, =, >=) RHS - Expr.Constant.
	rhs := c.RHS - c.Expr.Constant
	termStart := len(m.conTerms)
	m.conTerms = append(m.conTerms, c.Expr.Terms...)
	termEnd := len(m.conTerms)
	m.constraints = append(m.constraints, constraintDef{
		name:      name,
		rhs:       rhs,
		termStart: termStart,
		termEnd:   termEnd,
		sense:     c.Sense,
	})
	m.conByName[name] = cIdx
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
	if m == nil {
		return ErrNilModel
	}
	if m.err != nil {
		return m.err
	}
	for _, v := range m.vars {
		if !v.cat.valid() {
			return fmt.Errorf("variable %q has invalid category %d", v.name, uint8(v.cat))
		}
		if math.IsNaN(v.lb) || math.IsNaN(v.ub) || math.IsInf(v.lb, 1) || math.IsInf(v.ub, -1) {
			return fmt.Errorf("variable %q has invalid bounds [%v, %v]", v.name, v.lb, v.ub)
		}
	}
	if math.IsNaN(m.objConst) || math.IsInf(m.objConst, 0) {
		return fmt.Errorf("objective constant is non-finite (%v)", m.objConst)
	}
	counts := make([]uint8, len(m.vars))
	touched := make([]int, 0, min(len(m.vars), defaultModelCap))
	objHasDup := false
	for _, t := range m.objTerms {
		col, ok := m.validVar(t.Var)
		if !ok {
			return fmt.Errorf("objective references invalid or foreign Var handle %v", t.Var)
		}
		if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
			return fmt.Errorf("objective has non-finite coefficient %v", t.Coeff)
		}
		if counts[col] == 0 {
			counts[col] = 1
			touched = append(touched, col)
		} else {
			counts[col] = 2
			objHasDup = true
		}
	}
	if objHasDup {
		wideMap := make(map[int]*wideDotAcc)
		for _, col := range touched {
			if counts[col] > 1 {
				wideMap[col] = &wideDotAcc{}
			}
		}
		for _, t := range m.objTerms {
			if col, ok := m.validVar(t.Var); ok {
				if w, exists := wideMap[col]; exists {
					w.addVal(t.Coeff)
				}
			}
		}
		for _, col := range touched {
			if w, exists := wideMap[col]; exists {
				v := w.finish(math.NaN())
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Errorf("objective has non-finite accumulated coefficient for variable %q", m.vars[col].name)
				}
			}
		}
	}
	for _, col := range touched {
		counts[col] = 0
	}
	for _, c := range m.constraints {
		if !c.sense.valid() {
			return fmt.Errorf("constraint %q has invalid sense %d", c.name, int8(c.sense))
		}
		if math.IsNaN(c.rhs) || math.IsInf(c.rhs, 0) {
			return fmt.Errorf("constraint %q has non-finite RHS %v", c.name, c.rhs)
		}
		touched = touched[:0]
		rowHasDup := false
		cTerms := m.conTerms[c.termStart:c.termEnd]
		for _, t := range cTerms {
			col, ok := m.validVar(t.Var)
			if !ok {
				return fmt.Errorf("constraint %q references invalid or foreign Var handle %v", c.name, t.Var)
			}
			if math.IsNaN(t.Coeff) || math.IsInf(t.Coeff, 0) {
				return fmt.Errorf("constraint %q has non-finite coefficient %v", c.name, t.Coeff)
			}
			if counts[col] == 0 {
				counts[col] = 1
				touched = append(touched, col)
			} else {
				counts[col] = 2
				rowHasDup = true
			}
		}
		if rowHasDup {
			wideMap := make(map[int]*wideDotAcc)
			for _, col := range touched {
				if counts[col] > 1 {
					wideMap[col] = &wideDotAcc{}
				}
			}
			for _, t := range cTerms {
				if col, ok := m.validVar(t.Var); ok {
					if w, exists := wideMap[col]; exists {
						w.addVal(t.Coeff)
					}
				}
			}
			for _, col := range touched {
				if w, exists := wideMap[col]; exists {
					v := w.finish(math.NaN())
					if math.IsNaN(v) || math.IsInf(v, 0) {
						return fmt.Errorf("constraint %q has non-finite accumulated coefficient for variable %q", c.name, m.vars[col].name)
					}
				}
			}
		}
		for _, col := range touched {
			counts[col] = 0
		}
	}
	return nil
}
