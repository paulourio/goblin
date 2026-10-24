package goblin

import (
	"math"
	"slices"
	"time"
)

const (
	presolveFeasTol = 1e-6
	presolveIntTol  = 1e-7
	presolveZeroTol = 1e-12
)

// PresolveResult holds the reduced CompiledProblem or an immediate infeasible status flag.
type PresolveResult struct {
	Reduced          *CompiledProblem
	Infeasible       bool
	TimedOut         bool
	TightenedBounds  int
	RemovedRows      int
	StrengthenedRows int
}

// Presolve runs iterative bound tightening, Euclidean GCD integer row scaling,
// redundant row dropping, and binary coefficient strengthening on orig. When any
// variable bound interval becomes empty or an integer row proves unreachable, it
// returns Infeasible without allocating a reduced matrix.
func Presolve(orig *CompiledProblem, maxPasses int) PresolveResult {
	return presolveWithDeadline(orig, maxPasses, time.Time{}, nil)
}

func presolveWithDeadline(orig *CompiledProblem, maxPasses int, deadline time.Time, done <-chan struct{}) PresolveResult {
	if maxPasses <= 0 {
		maxPasses = defaultPresolveMaxPass
	}
	if stopRequested(deadline, done) {
		return PresolveResult{Reduced: orig, TimedOut: true}
	}
	n := orig.NumVars
	m := orig.NumConstraints
	if n == 0 {
		return PresolveResult{Reduced: orig}
	}

	lb := slices.Clone(orig.VarLB)
	ub := slices.Clone(orig.VarUB)
	tightened := 0
	for j := range n {
		if orig.VarCat[j] != Continuous {
			if !math.IsInf(lb[j], -1) {
				cl := math.Ceil(lb[j] - presolveIntTol)
				if cl > lb[j]+1e-9 {
					lb[j] = cl
					tightened++
				}
			}
			if !math.IsInf(ub[j], 1) {
				fl := math.Floor(ub[j] + presolveIntTol)
				if fl < ub[j]-1e-9 {
					ub[j] = fl
					tightened++
				}
			}
		}
		if lb[j] > ub[j]+presolveFeasTol {
			return PresolveResult{Infeasible: true}
		}
		if lb[j] > ub[j] {
			lb[j] = ub[j]
		}
	}
	if m == 0 && tightened == 0 {
		return PresolveResult{Reduced: orig}
	}

	type workRow struct {
		name   string
		sense  ConstraintSense
		rhs    float64
		cols   []int
		vals   []float64
		active bool
	}
	rows := make([]workRow, m)
	for i := range m {
		s, e := orig.RowPtr[i], orig.RowPtr[i+1]
		rows[i] = workRow{
			name:   orig.RowNames[i],
			sense:  orig.RowSense[i],
			rhs:    orig.RowRHS[i],
			cols:   slices.Clone(orig.ColIdx[s:e]),
			vals:   slices.Clone(orig.RowVal[s:e]),
			active: true,
		}
	}

	strengthened := 0
	removedRows := 0

	// 1. Euclidean GCD reduction on pure-integer rows
	if orig.IsMIP {
		for i := range m {
			r := &rows[i]
			if !r.active || len(r.cols) == 0 {
				continue
			}
			allInt := true
			var g int64
			for k, col := range r.cols {
				if orig.VarCat[col] == Continuous {
					allInt = false
					break
				}
				cVal := r.vals[k]
				rc := math.Round(cVal)
				if math.Abs(cVal-rc) > 1e-8 || math.Abs(rc) > 1e9 {
					allInt = false
					break
				}
				ic := int64(math.Abs(rc))
				if ic == 0 {
					continue
				}
				g = gcdInt64(g, ic)
			}
			if allInt && g > 1 {
				fg := float64(g)
				for k := range r.vals {
					r.vals[k] = math.Round(r.vals[k]) / fg
				}
				scaledRHS := r.rhs / fg
				switch r.sense {
				case LessEqual:
					r.rhs = math.Floor(scaledRHS + presolveIntTol)
				case GreaterEqual:
					r.rhs = math.Ceil(scaledRHS - presolveIntTol)
				case Equal:
					nearest := math.Round(scaledRHS)
					if math.Abs(scaledRHS-nearest) > presolveFeasTol {
						return PresolveResult{Infeasible: true}
					}
					r.rhs = nearest
				}
				strengthened++
			} else if allInt && g == 1 {
				switch r.sense {
				case LessEqual:
					fl := math.Floor(r.rhs + presolveIntTol)
					if fl < r.rhs-1e-9 {
						r.rhs = fl
						strengthened++
					}
				case GreaterEqual:
					cl := math.Ceil(r.rhs - presolveIntTol)
					if cl > r.rhs+1e-9 {
						r.rhs = cl
						strengthened++
					}
				case Equal:
					nearest := math.Round(r.rhs)
					if math.Abs(r.rhs-nearest) > presolveFeasTol {
						return PresolveResult{Infeasible: true}
					}
					r.rhs = nearest
				}
			}
		}
	}

	// 2. Iterative Savelsbergh activity bound tightening & redundant row dropping
	for range maxPasses {
		if stopRequested(deadline, done) {
			return PresolveResult{Reduced: orig, TimedOut: true}
		}
		changedInPass := false

		for i := range m {
			if (i&63 == 0) && stopRequested(deadline, done) {
				return PresolveResult{Reduced: orig, TimedOut: true}
			}
			r := &rows[i]
			if !r.active {
				continue
			}

			minFinite := 0.0
			maxFinite := 0.0
			minInfCount := 0
			maxInfCount := 0

			for k, col := range r.cols {
				a := r.vals[k]
				l, u := lb[col], ub[col]
				if a > 0 {
					if math.IsInf(l, -1) {
						minInfCount++
					} else {
						minFinite += a * l
					}
					if math.IsInf(u, 1) {
						maxInfCount++
					} else {
						maxFinite += a * u
					}
				} else if a < 0 {
					if math.IsInf(u, 1) {
						minInfCount++
					} else {
						minFinite += a * u
					}
					if math.IsInf(l, -1) {
						maxInfCount++
					} else {
						maxFinite += a * l
					}
				}
			}

			hasUpper := r.sense == LessEqual || r.sense == Equal
			hasLower := r.sense == GreaterEqual || r.sense == Equal

			if hasUpper && minInfCount == 0 && minFinite > r.rhs+presolveFeasTol {
				return PresolveResult{Infeasible: true}
			}
			if hasLower && maxInfCount == 0 && maxFinite < r.rhs-presolveFeasTol {
				return PresolveResult{Infeasible: true}
			}

			if r.sense == LessEqual && maxInfCount == 0 && maxFinite <= r.rhs+1e-9 {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == GreaterEqual && minInfCount == 0 && minFinite >= r.rhs-1e-9 {
				r.active = false
				removedRows++
				continue
			}

			if hasUpper && minInfCount <= 1 {
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					var resMin float64
					if a > 0 {
						if minInfCount == 1 {
							if !math.IsInf(l, -1) {
								continue
							}
							resMin = minFinite
						} else {
							resMin = minFinite - a*l
						}
						newUB := (r.rhs - resMin) / a
						if orig.VarCat[col] != Continuous {
							newUB = math.Floor(newUB + presolveIntTol)
						}
						if newUB < u-1e-7 {
							ub[col] = newUB
							if lb[col] > ub[col]+presolveFeasTol {
								return PresolveResult{Infeasible: true}
							}
							if lb[col] > ub[col] {
								ub[col] = lb[col]
							}
							tightened++
							changedInPass = true
						}
					} else {
						if minInfCount == 1 {
							if !math.IsInf(u, 1) {
								continue
							}
							resMin = minFinite
						} else {
							resMin = minFinite - a*u
						}
						newLB := (r.rhs - resMin) / a
						if orig.VarCat[col] != Continuous {
							newLB = math.Ceil(newLB - presolveIntTol)
						}
						if newLB > l+1e-7 {
							lb[col] = newLB
							if lb[col] > ub[col]+presolveFeasTol {
								return PresolveResult{Infeasible: true}
							}
							if lb[col] > ub[col] {
								lb[col] = ub[col]
							}
							tightened++
							changedInPass = true
						}
					}
				}
			}

			if hasLower && maxInfCount <= 1 {
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					var resMax float64
					if a > 0 {
						if maxInfCount == 1 {
							if !math.IsInf(u, 1) {
								continue
							}
							resMax = maxFinite
						} else {
							resMax = maxFinite - a*u
						}
						newLB := (r.rhs - resMax) / a
						if orig.VarCat[col] != Continuous {
							newLB = math.Ceil(newLB - presolveIntTol)
						}
						if newLB > l+1e-7 {
							lb[col] = newLB
							if lb[col] > ub[col]+presolveFeasTol {
								return PresolveResult{Infeasible: true}
							}
							if lb[col] > ub[col] {
								lb[col] = ub[col]
							}
							tightened++
							changedInPass = true
						}
					} else {
						if maxInfCount == 1 {
							if !math.IsInf(l, -1) {
								continue
							}
							resMax = maxFinite
						} else {
							resMax = maxFinite - a*l
						}
						newUB := (r.rhs - resMax) / a
						if orig.VarCat[col] != Continuous {
							newUB = math.Floor(newUB + presolveIntTol)
						}
						if newUB < u-1e-7 {
							ub[col] = newUB
							if lb[col] > ub[col]+presolveFeasTol {
								return PresolveResult{Infeasible: true}
							}
							if lb[col] > ub[col] {
								ub[col] = lb[col]
							}
							tightened++
							changedInPass = true
						}
					}
				}
			}
		}

		if !changedInPass {
			break
		}
	}

	// 3. Binary Coefficient Tightening (Savelsbergh)
	if orig.IsMIP {
		for i := range m {
			r := &rows[i]
			if !r.active || r.sense != LessEqual {
				continue
			}
			maxAct := 0.0
			finite := true
			for k, col := range r.cols {
				a := r.vals[k]
				l, u := lb[col], ub[col]
				if (a > 0 && math.IsInf(u, 1)) || (a < 0 && math.IsInf(l, -1)) {
					finite = false
					break
				}
				if a > 0 {
					maxAct += a * u
				} else {
					maxAct += a * l
				}
			}
			if finite && maxAct > r.rhs+1e-6 {
				for k, col := range r.cols {
					if orig.VarCat[col] == Binary && lb[col] == 0 && ub[col] == 1 {
						a := r.vals[k]
						if a > 1e-6 && maxAct-a < r.rhs-1e-6 {
							delta := r.rhs - (maxAct - a)
							r.vals[k] -= delta
							r.rhs -= delta
							maxAct -= delta
							strengthened++
						} else if a < -1e-6 && maxAct+a < r.rhs-1e-6 {
							delta := r.rhs - (maxAct + a)
							r.vals[k] += delta
							strengthened++
						}
					}
				}
			}
		}
	}

	if tightened == 0 && removedRows == 0 && strengthened == 0 {
		return PresolveResult{Reduced: orig}
	}

	mod := NewModel(orig.Name, orig.Sense)
	vars := make([]Var, n)
	var objTerms []Term
	for j := range n {
		vars[j] = mod.AddVar(orig.VarNames[j], lb[j], ub[j], orig.VarCat[j])
		if orig.ObjCoeffs[j] != 0 {
			objTerms = append(objTerms, T(vars[j], orig.ObjCoeffs[j]))
		}
	}
	mod.SetObjectiveNamed(orig.ObjName, Expr{
		Terms:    objTerms,
		Constant: orig.ObjConstant,
	})

	for i := range m {
		r := &rows[i]
		if !r.active {
			continue
		}
		terms := make([]Term, len(r.cols))
		for k, col := range r.cols {
			terms[k] = T(vars[col], r.vals[k])
		}
		mod.AddConstraint(r.name, Constraint{
			Expr:  Expr{Terms: terms},
			Sense: r.sense,
			RHS:   r.rhs,
		})
	}

	red := mod.Compile()
	red.modelID = orig.modelID
	return PresolveResult{
		Reduced:          red,
		TightenedBounds:  tightened,
		RemovedRows:      removedRows,
		StrengthenedRows: strengthened,
	}
}

func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
