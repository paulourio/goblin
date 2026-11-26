package goblin

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

const maxExactRatExp10 = 10_000

func cloneRat(r *big.Rat) *big.Rat {
	if r == nil {
		return nil
	}
	return new(big.Rat).Set(r)
}

func ratToFiniteFloat(r *big.Rat) (float64, bool) {
	if r == nil {
		return 0, false
	}
	f, _ := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return f, false
	}
	if f == 0 {
		f = 0
	}
	return f, true
}

func stripSignedLeadingZeros(s string) (neg bool, digits string, ok bool) {
	if s == "" {
		return false, "", false
	}
	i := 0
	switch s[0] {
	case '-':
		neg = true
		i = 1
	case '+':
		i = 1
	}
	if i >= len(s) {
		return false, "", false
	}
	for k := i; k < len(s); k++ {
		if s[k] < '0' || s[k] > '9' {
			return false, "", false
		}
	}
	d := s[i:]
	for len(d) > 1 && d[0] == '0' {
		d = d[1:]
	}
	if d == "0" {
		neg = false
	}
	return neg, d, true
}

// ParseExactRat parses a strict decimal ("0.9", "-7.2", "1.5e-3") or rational
// ("9/10") string into an exact normalized *big.Rat, rejecting whitespace,
// non-finite tokens, zero or signed denominators, and base-10 exponents exceeding
// 10000 in magnitude.
func ParseExactRat(s string) (*big.Rat, error) {
	if s == "" {
		return nil, errors.New("ParseExactRat: empty input")
	}
	if numStr, denStr, hasSlash := strings.Cut(s, "/"); hasSlash {
		if strings.IndexByte(denStr, '/') >= 0 {
			return nil, fmt.Errorf("ParseExactRat: invalid rational %q", s)
		}
		negNum, numDigits, okN := stripSignedLeadingZeros(numStr)
		if !okN || denStr == "" || denStr[0] == '+' || denStr[0] == '-' {
			return nil, fmt.Errorf("ParseExactRat: invalid rational %q", s)
		}
		_, denDigits, okD := stripSignedLeadingZeros(denStr)
		if !okD {
			return nil, fmt.Errorf("ParseExactRat: invalid rational %q", s)
		}
		if denDigits == "0" {
			return nil, fmt.Errorf("ParseExactRat: zero denominator in %q", s)
		}
		clean := numDigits + "/" + denDigits
		if negNum {
			clean = "-" + clean
		}
		r, ok := new(big.Rat).SetString(clean)
		if !ok {
			return nil, fmt.Errorf("ParseExactRat: invalid rational %q", s)
		}
		return r, nil
	}

	i := 0
	n := len(s)
	if s[i] == '-' || s[i] == '+' {
		i++
	}
	intDigits := 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		intDigits++
		i++
	}
	fracDigits := 0
	if i < n && s[i] == '.' {
		i++
		for i < n && s[i] >= '0' && s[i] <= '9' {
			fracDigits++
			i++
		}
	}
	if intDigits+fracDigits == 0 {
		return nil, fmt.Errorf("ParseExactRat: invalid decimal %q", s)
	}
	var expVal int64
	if i < n && (s[i] == 'e' || s[i] == 'E') {
		i++
		expNeg := false
		if i < n && (s[i] == '+' || s[i] == '-') {
			expNeg = s[i] == '-'
			i++
		}
		if i >= n || s[i] < '0' || s[i] > '9' {
			return nil, fmt.Errorf("ParseExactRat: invalid exponent in %q", s)
		}
		const maxRawExp = 100_000
		var v int64
		for i < n && s[i] >= '0' && s[i] <= '9' {
			v = v*10 + int64(s[i]-'0')
			if v > maxRawExp {
				return nil, fmt.Errorf("ParseExactRat: exponent out of range in %q", s)
			}
			i++
		}
		if expNeg {
			expVal = -v
		} else {
			expVal = v
		}
	}
	if i != n {
		return nil, fmt.Errorf("ParseExactRat: invalid decimal %q", s)
	}
	if fracDigits > 2*maxExactRatExp10 {
		return nil, fmt.Errorf("ParseExactRat: base-10 exponent out of range in %q", s)
	}
	exp10 := expVal - int64(fracDigits)
	if exp10 < -maxExactRatExp10 || exp10 > maxExactRatExp10 {
		return nil, fmt.Errorf("ParseExactRat: base-10 exponent %d exceeds limit %d in %q", exp10, maxExactRatExp10, s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("ParseExactRat: invalid decimal %q", s)
	}
	return r, nil
}

// Term represents a single linear term (Coeff * Var).
type Term struct {
	Var   Var
	Coeff float64
	rat   *big.Rat
}

// T constructs a single linear term Coeff * Var.
func T(v Var, coeff float64) Term {
	return Term{Var: v, Coeff: coeff}
}

// T constructs a single linear term coeff * v without repeating the package prefix.
func (v Var) T(coeff float64) Term {
	return Term{Var: v, Coeff: coeff}
}

// TRat constructs a single linear term coeff * v using an exact rational
// coefficient cloned from coeff.
func TRat(v Var, coeff *big.Rat) Term {
	if coeff == nil {
		return Term{Var: invalidRatVar, Coeff: math.NaN()}
	}
	f, _ := coeff.Float64()
	if f == 0 {
		f = 0
	}
	return Term{Var: v, Coeff: f, rat: cloneRat(coeff)}
}

// TRat constructs a single linear term coeff * v using an exact rational
// coefficient cloned from coeff.
func (v Var) TRat(coeff *big.Rat) Term {
	return TRat(v, coeff)
}

// TDec constructs a single linear term from a decimal or rational literal via
// ParseExactRat. Invalid strings record a deferred validation error surfaced by
// Model.Validate and Model.CompileChecked without panicking.
func (v Var) TDec(decimal string) Term {
	r, err := ParseExactRat(decimal)
	if err != nil {
		return Term{Var: invalidDecVar, Coeff: math.NaN()}
	}
	f, _ := r.Float64()
	if f == 0 {
		f = 0
	}
	return Term{Var: v, Coeff: f, rat: r}
}

// Expr represents an affine linear form: sum_i (Coeff_i * Var_i) + Constant.
// Duplicate variable terms are preserved in slice order until Model.Compile merges
// matching column indices and drops zero entries.
type Expr struct {
	Terms    []Term
	Constant float64
	rat      *big.Rat
	err      error
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

//go:noinline
func sumExprConstantsWide(exprs []Expr) float64 {
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
		return math.NaN()
	case hasPosInf:
		return math.Inf(1)
	case hasNegInf:
		return math.Inf(-1)
	default:
		var w wideDotAcc
		for i := range exprs {
			w.addVal(exprs[i].Constant)
		}
		return w.finish(math.NaN())
	}
}

//go:noinline
func sumExprConstantsRat(exprs []Expr) (*big.Rat, float64) {
	sum := new(big.Rat)
	for i := range exprs {
		if exprs[i].rat != nil {
			if _, ok := ratToFiniteFloat(exprs[i].rat); !ok {
				return nil, sumExprConstantsWide(exprs)
			}
			sum.Add(sum, exprs[i].rat)
			continue
		}
		c := exprs[i].Constant
		if math.IsNaN(c) || math.IsInf(c, 0) {
			return nil, sumExprConstantsWide(exprs)
		}
		if c != 0 {
			sum.Add(sum, new(big.Rat).SetFloat64(c))
		}
	}
	f, _ := sum.Float64()
	if f == 0 {
		f = 0
	}
	return sum, f
}

// Sum concatenates all terms and sums the constants of exprs into a single Expr.
func Sum(exprs ...Expr) Expr {
	totalTerms := 0
	var acc dot2Acc
	hasRat := false
	var firstErr error
	for i := range exprs {
		totalTerms += len(exprs[i].Terms)
		acc.addVal(exprs[i].Constant)
		if exprs[i].rat != nil {
			hasRat = true
		}
		if firstErr == nil && exprs[i].err != nil {
			firstErr = exprs[i].err
		}
	}
	var constant float64
	var constRat *big.Rat
	if hasRat {
		constRat, constant = sumExprConstantsRat(exprs)
		if (math.IsInf(constant, 0) || math.IsNaN(constant)) && firstErr == nil {
			firstErr = fmt.Errorf("Sum: constant overflows finite float64 (%v)", constant)
		}
	} else if !acc.nonFin && acc.lowCount == 0 && acc.errL1 == 0 && acc.c == 0 && !isNonFinite(acc.s) {
		constant = acc.s
	} else {
		constant = sumExprConstantsWide(exprs)
	}
	terms := make([]Term, 0, totalTerms)
	for i := range exprs {
		terms = append(terms, exprs[i].Terms...)
	}
	return Expr{Terms: terms, Constant: constant, rat: constRat, err: firstErr}
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

// Sentinel variable IDs emitted on deferred expression construction errors so
// Model.Validate and Model.CompileChecked surface deterministic errors without
// panicking or expanding Term with an error interface.
const (
	invalidDotVar = Var(^uint64(0))
	invalidDecVar = Var(^uint64(0) - 1)
	invalidRatVar = Var(^uint64(0) - 2)
)

func isSentinelErrVar(v Var) bool {
	return v == invalidDotVar || v == invalidDecVar || v == invalidRatVar
}

// Dot computes the inner product sum_i coeffs[i] * vars[i], skipping any entry whose
// coefficient is zero. If len(vars) != len(coeffs), it records an invalid sentinel term
// so Model.Validate and Model.Compile return an error instead of silently truncating;
// duplicate variables or non-finite coefficients are retained until compilation.
func Dot(vars []Var, coeffs []float64) Expr {
	if len(vars) != len(coeffs) {
		return Expr{Terms: []Term{{Var: invalidDotVar, Coeff: 1}}}
	}
	terms := make([]Term, 0, len(vars))
	for i, v := range vars {
		if coeffs[i] != 0 {
			terms = append(terms, Term{Var: v, Coeff: coeffs[i]})
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

// AddTermRat appends an exact rational term coeff*v to the expression without
// mutating shared backing storage or aliasing coeff.
func (e Expr) AddTermRat(v Var, coeff *big.Rat) Expr {
	n := len(e.Terms)
	e.Terms = append(e.Terms[:n:n], TRat(v, coeff))
	return e
}

// AddConstant adds a scalar constant to the expression.
func (e Expr) AddConstant(c float64) Expr {
	if e.rat != nil {
		if math.IsNaN(c) || math.IsInf(c, 0) {
			e.rat = nil
			e.Constant += c
			return e
		}
		sum := new(big.Rat).Add(e.rat, new(big.Rat).SetFloat64(c))
		f, ok := ratToFiniteFloat(sum)
		if !ok && e.err == nil {
			e.err = fmt.Errorf("AddConstant: rational constant overflows finite float64 (%v)", f)
		}
		e.rat = sum
		e.Constant = f
		return e
	}
	e.Constant += c
	return e
}

// AddConstantRat adds an exact rational constant cloned from c to the expression.
func (e Expr) AddConstantRat(c *big.Rat) Expr {
	if c == nil {
		if e.err == nil {
			e.err = errors.New("AddConstantRat: nil rational constant")
		}
		return e
	}
	cf, okC := ratToFiniteFloat(c)
	if !okC {
		if e.err == nil {
			e.err = fmt.Errorf("AddConstantRat: rational constant overflows finite float64 (%v)", cf)
		}
		e.Constant = cf
		e.rat = cloneRat(c)
		return e
	}
	var base *big.Rat
	if e.rat != nil {
		base = e.rat
	} else if e.Constant != 0 {
		if math.IsNaN(e.Constant) || math.IsInf(e.Constant, 0) {
			e.Constant += cf
			return e
		}
		base = new(big.Rat).SetFloat64(e.Constant)
	}
	var sum *big.Rat
	if base != nil {
		sum = new(big.Rat).Add(base, c)
	} else {
		sum = cloneRat(c)
	}
	f, okS := ratToFiniteFloat(sum)
	if !okS && e.err == nil {
		e.err = fmt.Errorf("AddConstantRat: rational constant overflows finite float64 (%v)", f)
	}
	e.rat = sum
	e.Constant = f
	return e
}

// Scale multiplies all terms and the constant of e by factor.
func (e Expr) Scale(factor float64) Expr {
	hasRat := e.rat != nil
	if !hasRat {
		for i := range e.Terms {
			if e.Terms[i].rat != nil {
				hasRat = true
				break
			}
		}
	}
	if !hasRat {
		terms := make([]Term, len(e.Terms))
		for i, t := range e.Terms {
			terms[i] = Term{Var: t.Var, Coeff: t.Coeff * factor}
		}
		return Expr{Terms: terms, Constant: e.Constant * factor, err: e.err}
	}
	if math.IsNaN(factor) || math.IsInf(factor, 0) {
		terms := make([]Term, len(e.Terms))
		for i, t := range e.Terms {
			if isSentinelErrVar(t.Var) {
				terms[i] = t
			} else {
				terms[i] = Term{Var: t.Var, Coeff: t.Coeff * factor}
			}
		}
		return Expr{Terms: terms, Constant: e.Constant * factor, err: e.err}
	}
	fRat := new(big.Rat).SetFloat64(factor)
	terms := make([]Term, len(e.Terms))
	errOut := e.err
	for i, t := range e.Terms {
		if isSentinelErrVar(t.Var) {
			terms[i] = t
			continue
		}
		if t.rat != nil {
			sr := new(big.Rat).Mul(t.rat, fRat)
			sf, okS := ratToFiniteFloat(sr)
			if !okS && errOut == nil {
				errOut = fmt.Errorf("Scale: rational coefficient overflows finite float64 (%v)", sf)
			}
			terms[i] = Term{Var: t.Var, Coeff: sf, rat: sr}
		} else {
			terms[i] = Term{Var: t.Var, Coeff: t.Coeff * factor}
		}
	}
	var constRat *big.Rat
	constVal := e.Constant * factor
	if e.rat != nil {
		constRat = new(big.Rat).Mul(e.rat, fRat)
		var okC bool
		constVal, okC = ratToFiniteFloat(constRat)
		if !okC && errOut == nil {
			errOut = fmt.Errorf("Scale: rational constant overflows finite float64 (%v)", constVal)
		}
	}
	return Expr{Terms: terms, Constant: constVal, rat: constRat, err: errOut}
}

// Constraint represents a linear inequality or equality: Expr (<=, =, >=) RHS.
type Constraint struct {
	Expr   Expr
	Sense  ConstraintSense
	RHS    float64
	rhsRat *big.Rat
	err    error
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

func makeConstraintRat(expr Expr, sense ConstraintSense, rhs *big.Rat) Constraint {
	if rhs == nil {
		return Constraint{
			Expr:  expr,
			Sense: sense,
			RHS:   math.NaN(),
			err:   errors.New("constraint RHS: nil *big.Rat"),
		}
	}
	f, ok := ratToFiniteFloat(rhs)
	var err error
	if !ok {
		err = fmt.Errorf("constraint RHS overflows finite float64 (%v)", f)
	}
	return Constraint{
		Expr:   expr,
		Sense:  sense,
		RHS:    f,
		rhsRat: cloneRat(rhs),
		err:    err,
	}
}

// LeRat creates a less-than-or-equal constraint expr <= rhs using an exact
// rational right-hand side cloned from rhs.
func LeRat(expr Expr, rhs *big.Rat) Constraint {
	return makeConstraintRat(expr, LessEqual, rhs)
}

// LeRat creates a less-than-or-equal constraint e <= rhs using an exact
// rational right-hand side cloned from rhs.
func (e Expr) LeRat(rhs *big.Rat) Constraint {
	return makeConstraintRat(e, LessEqual, rhs)
}

// GeRat creates a greater-than-or-equal constraint expr >= rhs using an exact
// rational right-hand side cloned from rhs.
func GeRat(expr Expr, rhs *big.Rat) Constraint {
	return makeConstraintRat(expr, GreaterEqual, rhs)
}

// GeRat creates a greater-than-or-equal constraint e >= rhs using an exact
// rational right-hand side cloned from rhs.
func (e Expr) GeRat(rhs *big.Rat) Constraint {
	return makeConstraintRat(e, GreaterEqual, rhs)
}

// EqRat creates an equality constraint expr == rhs using an exact rational
// right-hand side cloned from rhs.
func EqRat(expr Expr, rhs *big.Rat) Constraint {
	return makeConstraintRat(expr, Equal, rhs)
}

// EqRat creates an equality constraint e == rhs using an exact rational
// right-hand side cloned from rhs.
func (e Expr) EqRat(rhs *big.Rat) Constraint {
	return makeConstraintRat(e, Equal, rhs)
}
