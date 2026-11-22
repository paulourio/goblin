package goblin

import "math"

// Term represents a single linear term (Coeff * Var).
type Term struct {
	Var   Var
	Coeff float64
}

// T constructs a single linear term Coeff * Var.
func T(v Var, coeff float64) Term {
	return Term{Var: v, Coeff: coeff}
}

// T constructs a single linear term coeff * v without repeating the package prefix.
func (v Var) T(coeff float64) Term {
	return Term{Var: v, Coeff: coeff}
}

// Expr represents an affine linear form: sum_i (Coeff_i * Var_i) + Constant.
// Duplicate variable terms are preserved in slice order until Model.Compile merges
// matching column indices and drops zero entries.
type Expr struct {
	Terms    []Term
	Constant float64
}

// NewExpr builds an Expr from terms.
func NewExpr(terms ...Term) Expr {
	return Expr{Terms: terms}
}

// VarExpr wraps one Var with unit weight.
func VarExpr(v Var) Expr {
	return Expr{Terms: []Term{{Var: v, Coeff: 1.0}}}
}

// ConstExpr builds a constant Expr.
func ConstExpr(c float64) Expr {
	return Expr{Constant: c}
}

// Sum concatenates all terms and sums the constants of exprs into a single Expr.
func Sum(exprs ...Expr) Expr {
	totalTerms := 0
	var acc dot2Acc
	for i := range exprs {
		totalTerms += len(exprs[i].Terms)
		acc.addVal(exprs[i].Constant)
	}
	var constant float64
	if val, eb, ok := acc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		constant = val
	} else {
		hasNaN := false
		hasPosInf := false
		hasNegInf := false
		for i := range exprs {
			c := exprs[i].Constant
			if math.IsNaN(c) {
				hasNaN = true
				break
			}
			if math.IsInf(c, 1) {
				hasPosInf = true
			} else if math.IsInf(c, -1) {
				hasNegInf = true
			}
		}
		switch {
		case hasNaN || (hasPosInf && hasNegInf):
			constant = math.NaN()
		case hasPosInf:
			constant = math.Inf(1)
		case hasNegInf:
			constant = math.Inf(-1)
		default:
			var w wideDotAcc
			for i := range exprs {
				w.addVal(exprs[i].Constant)
			}
			constant = w.finish(math.NaN())
		}
	}
	terms := make([]Term, 0, totalTerms)
	for i := range exprs {
		terms = append(terms, exprs[i].Terms...)
	}
	return Expr{Terms: terms, Constant: constant}
}

// SumTerms creates an Expr from a slice of Terms.
func SumTerms(terms ...Term) Expr {
	out := make([]Term, len(terms))
	copy(out, terms)
	return Expr{Terms: out}
}

// SumVars creates an Expr with unit coefficients (1.0) for each variable in vars.
func SumVars(vars ...Var) Expr {
	terms := make([]Term, len(vars))
	for i, v := range vars {
		terms[i] = Term{Var: v, Coeff: 1.0}
	}
	return Expr{Terms: terms}
}

// Dot computes the inner product sum_i coeffs[i] * vars[i] up to min(len(vars), len(coeffs)),
// skipping any entry whose coefficient is zero. Mismatched slice lengths silently truncate
// to min(len(vars), len(coeffs)), and duplicate variables or non-finite coefficients are
// retained until Model.Compile merges or validates terms.
func Dot(vars []Var, coeffs []float64) Expr {
	n := min(len(vars), len(coeffs))
	terms := make([]Term, 0, n)
	for i := range n {
		if coeffs[i] != 0 {
			terms = append(terms, Term{Var: vars[i], Coeff: coeffs[i]})
		}
	}
	return Expr{Terms: terms}
}

// AddTerm appends a term to the expression without mutating shared backing storage.
func (e Expr) AddTerm(v Var, coeff float64) Expr {
	n := len(e.Terms)
	e.Terms = append(e.Terms[:n:n], Term{Var: v, Coeff: coeff})
	return e
}

// AddConstant adds a scalar constant to the expression.
func (e Expr) AddConstant(c float64) Expr {
	e.Constant += c
	return e
}

// Scale multiplies all terms and the constant of e by factor.
func (e Expr) Scale(factor float64) Expr {
	terms := make([]Term, len(e.Terms))
	for i, t := range e.Terms {
		terms[i] = Term{Var: t.Var, Coeff: t.Coeff * factor}
	}
	return Expr{Terms: terms, Constant: e.Constant * factor}
}

// Constraint represents a linear inequality or equality: Expr (<=, =, >=) RHS.
type Constraint struct {
	Expr  Expr
	Sense ConstraintSense
	RHS   float64
}

// Le creates a less-than-or-equal constraint: expr <= rhs.
func Le(expr Expr, rhs float64) Constraint {
	return Constraint{Expr: expr, Sense: LessEqual, RHS: rhs}
}

// Le creates a less-than-or-equal constraint: e <= rhs.
func (e Expr) Le(rhs float64) Constraint {
	return Constraint{Expr: e, Sense: LessEqual, RHS: rhs}
}

// Ge creates a greater-than-or-equal constraint: expr >= rhs.
func Ge(expr Expr, rhs float64) Constraint {
	return Constraint{Expr: expr, Sense: GreaterEqual, RHS: rhs}
}

// Ge creates a greater-than-or-equal constraint: e >= rhs.
func (e Expr) Ge(rhs float64) Constraint {
	return Constraint{Expr: e, Sense: GreaterEqual, RHS: rhs}
}

// Eq creates an equality constraint: expr == rhs.
func Eq(expr Expr, rhs float64) Constraint {
	return Constraint{Expr: expr, Sense: Equal, RHS: rhs}
}

// Eq creates an equality constraint: e == rhs.
func (e Expr) Eq(rhs float64) Constraint {
	return Constraint{Expr: e, Sense: Equal, RHS: rhs}
}
