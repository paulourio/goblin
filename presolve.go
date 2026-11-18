package goblin

import (
	"cmp"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"time"
)

const (
	presolveFeasTol        = 1e-6
	presolveIntTol         = 1e-7
	presolveBoundChangeTol = 1e-9
	presolveGCDIntTol      = 1e-8
	presolveGCDMaxRowErr   = 1e-9
	presolveGCDMaxCoeff    = 1e9
	presolveRelScaleTol    = 1e-12
	presolveZeroTol        = 1e-12
	presolveQuantScale     = 1e6
	presolveQuantOffset    = 0.1234567
	maxOrbitGroupSize      = 64
	maxOrbitSeedTrials     = 64
	orbitUnassigned        = -1
	rowStateUnvisited      = -1
	rowStateInvariant      = -2
)

type presolveWorkRow struct {
	name   string
	sense  ConstraintSense
	rhs    float64
	cols   []int
	vals   []float64
	active bool
}

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
	n := orig.numVars
	m := orig.numConstraints
	if n == 0 {
		for i := range m {
			if 0 < orig.rowLB[i]-presolveFeasTol || 0 > orig.rowUB[i]+presolveFeasTol {
				return PresolveResult{Infeasible: true}
			}
		}
		return PresolveResult{Reduced: orig}
	}

	lb := slices.Clone(orig.varLB)
	ub := slices.Clone(orig.varUB)
	tightened := 0
	for j := range n {
		if orig.varCat[j] != Continuous {
			if !math.IsInf(lb[j], -1) {
				cl := math.Ceil(lb[j] - presolveIntTol)
				if cl > lb[j]+presolveBoundChangeTol {
					lb[j] = cl
					tightened++
				}
			}
			if !math.IsInf(ub[j], 1) {
				fl := math.Floor(ub[j] + presolveIntTol)
				if fl < ub[j]-presolveBoundChangeTol {
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

	rows := make([]presolveWorkRow, m)
	for i := range m {
		s, e := orig.rowPtr[i], orig.rowPtr[i+1]
		rows[i] = presolveWorkRow{
			name:   orig.rowNames[i],
			sense:  orig.rowSense[i],
			rhs:    orig.rowRHS[i],
			cols:   slices.Clone(orig.colIdx[s:e]),
			vals:   slices.Clone(orig.rowVal[s:e]),
			active: true,
		}
	}

	strengthened := 0
	removedRows := 0

	// 1. Euclidean GCD reduction on pure-integer rows with domain-bounded rounding error.
	if orig.isMIP {
		for i := range m {
			r := &rows[i]
			if !r.active || len(r.cols) == 0 {
				continue
			}
			allInt := true
			var g int64
			maxRoundErr := 0.0
			for k, col := range r.cols {
				if orig.varCat[col] == Continuous {
					allInt = false
					break
				}
				cVal := r.vals[k]
				rc := math.Round(cVal)
				diff := math.Abs(cVal - rc)
				if diff > presolveGCDIntTol || math.Abs(rc) > presolveGCDMaxCoeff {
					allInt = false
					break
				}
				if diff > 0 {
					maxDom := max(math.Abs(lb[col]), math.Abs(ub[col]))
					if math.IsInf(maxDom, 0) || math.IsNaN(maxDom) {
						allInt = false
						break
					}
					maxRoundErr += diff * maxDom
					if maxRoundErr > presolveGCDMaxRowErr {
						allInt = false
						break
					}
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
				errScaled := maxRoundErr / fg
				intTolScaled := presolveIntTol / fg
				feasTolScaled := presolveFeasTol / fg
				scaledRHS := r.rhs / fg
				switch r.sense {
				case LessEqual:
					r.rhs = math.Floor(scaledRHS + errScaled + intTolScaled)
				case GreaterEqual:
					r.rhs = math.Ceil(scaledRHS - errScaled - intTolScaled)
				case Equal:
					nearest := math.Round(scaledRHS)
					if math.Abs(scaledRHS-nearest) > feasTolScaled+errScaled {
						return PresolveResult{Infeasible: true}
					}
					r.rhs = nearest
				}
				strengthened++
			} else if allInt && g == 1 {
				if maxRoundErr > 0 {
					for k := range r.vals {
						r.vals[k] = math.Round(r.vals[k])
					}
				}
				switch r.sense {
				case LessEqual:
					fl := math.Floor(r.rhs + maxRoundErr + presolveIntTol)
					if fl < r.rhs-presolveBoundChangeTol {
						r.rhs = fl
						strengthened++
					}
				case GreaterEqual:
					cl := math.Ceil(r.rhs - maxRoundErr - presolveIntTol)
					if cl > r.rhs+presolveBoundChangeTol {
						r.rhs = cl
						strengthened++
					}
				case Equal:
					nearest := math.Round(r.rhs)
					if math.Abs(r.rhs-nearest) > presolveFeasTol+maxRoundErr {
						return PresolveResult{Infeasible: true}
					}
					r.rhs = nearest
				}
			}
		}
	}

	// 2. Iterative Savelsbergh activity bound tightening and redundant row dropping
	actTight, actRem, infeas, timedOut := runActivityTightening(orig, rows, lb, ub, maxPasses, deadline, done)
	if infeas {
		return PresolveResult{Infeasible: true}
	}
	if timedOut {
		return PresolveResult{Reduced: orig, TimedOut: true}
	}
	tightened += actTight
	removedRows += actRem

	// 3. Algebraic orbit symmetry breaking on symmetric group formulations
	if orig.isMIP {
		symTight, symRows, symTimedOut := applyOrbitSymmetryBreaking(orig, rows, lb, ub, deadline, done)
		if symTimedOut {
			return PresolveResult{Reduced: orig, TimedOut: true}
		}
		if len(symRows) > 0 {
			rows = slices.Concat(rows, symRows)
			m = len(rows)
			strengthened += len(symRows)
		}
		if symTight > 0 {
			tightened += symTight
			postTight, postRem, postInfeas, postTimedOut := runActivityTightening(orig, rows, lb, ub, maxPasses, deadline, done)
			if postInfeas {
				return PresolveResult{Infeasible: true}
			}
			if postTimedOut {
				return PresolveResult{Reduced: orig, TimedOut: true}
			}
			tightened += postTight
			removedRows += postRem
		}
	}

	// 4. Binary Coefficient Tightening (Savelsbergh)
	if orig.isMIP {
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
			if finite && maxAct > r.rhs+presolveFeasTol {
				for k, col := range r.cols {
					if orig.varCat[col] == Binary && lb[col] == 0 && ub[col] == 1 {
						a := r.vals[k]
						if a > presolveFeasTol && maxAct-a < r.rhs-presolveFeasTol {
							delta := r.rhs - (maxAct - a)
							r.vals[k] -= delta
							r.rhs -= delta
							maxAct -= delta
							strengthened++
						} else if a < -presolveFeasTol && maxAct+a < r.rhs-presolveFeasTol {
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

	mod := NewModel(orig.name, orig.sense)
	vars := make([]Var, n)
	var objTerms []Term
	for j := range n {
		vars[j] = mod.AddVar(orig.varNames[j], lb[j], ub[j], orig.varCat[j])
		if orig.objCoeffs[j] != 0 {
			objTerms = append(objTerms, T(vars[j], orig.objCoeffs[j]))
		}
	}
	mod.SetObjectiveNamed(orig.objName, Expr{
		Terms:    objTerms,
		Constant: orig.objConstant,
	})

	for i := range m {
		r := &rows[i]
		if !r.active {
			continue
		}
		terms := make([]Term, 0, len(r.cols))
		for k, col := range r.cols {
			if lb[col] == 0 && ub[col] == 0 {
				continue
			}
			terms = append(terms, T(vars[col], r.vals[k]))
		}
		if len(terms) == 0 {
			switch r.sense {
			case LessEqual:
				if 0 > r.rhs+presolveFeasTol {
					return PresolveResult{Infeasible: true}
				}
			case GreaterEqual:
				if 0 < r.rhs-presolveFeasTol {
					return PresolveResult{Infeasible: true}
				}
			case Equal:
				if math.Abs(r.rhs) > presolveFeasTol {
					return PresolveResult{Infeasible: true}
				}
			}
			removedRows++
			continue
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

// runActivityTightening executes up to maxPasses rounds of Savelsbergh row-activity bound tightening and drops redundant rows.
func runActivityTightening(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	maxPasses int,
	deadline time.Time,
	done <-chan struct{},
) (tightened, removedRows int, infeas, timedOut bool) {
	m := len(rows)
	for range maxPasses {
		if stopRequested(deadline, done) {
			return tightened, removedRows, false, true
		}
		changedInPass := false

		for i := range m {
			if (i&63 == 0) && stopRequested(deadline, done) {
				return tightened, removedRows, false, true
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
			rowScale := max(1.0, max(math.Abs(r.rhs), max(math.Abs(minFinite), math.Abs(maxFinite))))
			rowFeasTol := max(presolveFeasTol, presolveRelScaleTol*rowScale)

			if hasUpper && minInfCount == 0 && minFinite > r.rhs+rowFeasTol {
				return tightened, removedRows, true, false
			}
			if hasLower && maxInfCount == 0 && maxFinite < r.rhs-rowFeasTol {
				return tightened, removedRows, true, false
			}

			if r.sense == LessEqual && maxInfCount == 0 && maxFinite <= r.rhs+presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == GreaterEqual && minInfCount == 0 && minFinite >= r.rhs-presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == Equal && minInfCount == 0 && maxInfCount == 0 && minFinite >= r.rhs-presolveBoundChangeTol && maxFinite <= r.rhs+presolveBoundChangeTol {
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
						if orig.varCat[col] != Continuous {
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
							newUB = math.Floor(newUB + bTol)
						}
						if newUB < u-presolveIntTol {
							ub[col] = newUB
							if lb[col] > ub[col]+presolveFeasTol {
								return tightened, removedRows, true, false
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
						if orig.varCat[col] != Continuous {
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/(-a))
							newLB = math.Ceil(newLB - bTol)
						}
						if newLB > l+presolveIntTol {
							lb[col] = newLB
							if lb[col] > ub[col]+presolveFeasTol {
								return tightened, removedRows, true, false
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
						if orig.varCat[col] != Continuous {
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
							newLB = math.Ceil(newLB - bTol)
						}
						if newLB > l+presolveIntTol {
							lb[col] = newLB
							if lb[col] > ub[col]+presolveFeasTol {
								return tightened, removedRows, true, false
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
						if orig.varCat[col] != Continuous {
							bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/(-a))
							newUB = math.Floor(newUB + bTol)
						}
						if newUB < u-presolveIntTol {
							ub[col] = newUB
							if lb[col] > ub[col]+presolveFeasTol {
								return tightened, removedRows, true, false
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
	return tightened, removedRows, false, false
}

type presolveColEntry struct {
	row int
	val float64
}

func presolveSplitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func presolveQuantize(v float64) int64 {
	if math.IsInf(v, -1) {
		return math.MinInt64
	}
	if math.IsInf(v, 1) {
		return math.MaxInt64
	}
	if math.Abs(v) <= presolveZeroTol {
		return 0
	}
	return int64(math.Round(v*presolveQuantScale + presolveQuantOffset))
}

func samePresolveBound(a, b float64) bool {
	if math.IsInf(a, -1) || math.IsInf(b, -1) {
		return math.IsInf(a, -1) && math.IsInf(b, -1)
	}
	if math.IsInf(a, 1) || math.IsInf(b, 1) {
		return math.IsInf(a, 1) && math.IsInf(b, 1)
	}
	return math.Abs(a-b) <= presolveZeroTol
}

// applyOrbitSymmetryBreaking detects exact symmetric group actions on constraint rows and
// variable orbits, applying static orbitope/bijection fixings when objective costs are
// group-invariant or empty-block ordering inequalities when block costs are monotone and
// gated by a binary block-open indicator.
func applyOrbitSymmetryBreaking(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	deadline time.Time,
	done <-chan struct{},
) (tightened int, addedRows []presolveWorkRow, timedOut bool) {
	n := orig.numVars
	m := len(rows)
	if n < 4 || m < 2 {
		return 0, nil, false
	}

	var seedRows []int
	for i := range m {
		r := &rows[i]
		g := len(r.cols)
		if !r.active || g < 2 || g > maxOrbitGroupSize || g*2 > n {
			continue
		}
		if r.sense != Equal && r.sense != LessEqual {
			continue
		}
		if math.Abs(r.rhs-1.0) > presolveBoundChangeTol {
			continue
		}
		unitBin := true
		for k, col := range r.cols {
			if math.Abs(r.vals[k]-1.0) > presolveBoundChangeTol || orig.varCat[col] != Binary || lb[col] != 0 || ub[col] != 1 {
				unitBin = false
				break
			}
		}
		if unitBin {
			seedRows = append(seedRows, i)
		}
	}
	if len(seedRows) == 0 {
		return 0, nil, false
	}
	if stopRequested(deadline, done) {
		return 0, nil, true
	}

	colRows := make([][]presolveColEntry, n)
	for i := range m {
		r := &rows[i]
		if !r.active {
			continue
		}
		for k, col := range r.cols {
			colRows[col] = append(colRows[col], presolveColEntry{row: i, val: r.vals[k]})
		}
	}

	filteredSeeds := seedRows[:0]
	for _, sr := range seedRows {
		cols := rows[sr].cols
		deg0 := len(colRows[cols[0]])
		if deg0 < 2 {
			continue
		}
		sameDeg := true
		for _, c := range cols[1:] {
			if len(colRows[c]) != deg0 {
				sameDeg = false
				break
			}
		}
		if sameDeg {
			filteredSeeds = append(filteredSeeds, sr)
		}
	}
	seedRows = filteredSeeds
	if len(seedRows) == 0 {
		return 0, nil, false
	}
	slices.SortStableFunc(seedRows, func(a, b int) int {
		sa := rows[a].sense == Equal
		sb := rows[b].sense == Equal
		if sa != sb {
			if sa {
				return -1
			}
			return 1
		}
		return cmp.Compare(len(rows[b].cols), len(rows[a].cols))
	})

	rowHash := make([]uint64, m)
	for i := range m {
		r := &rows[i]
		if !r.active {
			continue
		}
		h := presolveSplitmix64(uint64(r.sense) ^ (uint64(presolveQuantize(r.rhs)) << 3) ^ uint64(len(r.cols))*0x517cc1b727220a95)
		var sumH, xorH uint64
		for k, col := range r.cols {
			th := presolveSplitmix64(uint64(presolveQuantize(r.vals[k])))
			th ^= presolveSplitmix64(uint64(orig.varCat[col]) + 0x100)
			th ^= presolveSplitmix64(uint64(presolveQuantize(lb[col])) + 0x200)
			th ^= presolveSplitmix64(uint64(presolveQuantize(ub[col])) + 0x300)
			th ^= presolveSplitmix64(uint64(len(colRows[col])) + 0x400)
			sumH += th
			xorH ^= bits.RotateLeft64(th, 17)
		}
		rowHash[i] = h ^ sumH ^ xorH
	}

	maxTrials := min(len(seedRows), maxOrbitSeedTrials)
	for sIdx := range maxTrials {
		if stopRequested(deadline, done) {
			return 0, nil, true
		}
		orbits, colOrbit, rowTuples, ok, to := proposeAndVerifyOrbits(
			orig, rows, lb, ub, colRows, rowHash, seedRows[sIdx], deadline, done,
		)
		if to {
			return 0, nil, true
		}
		if !ok || len(orbits) == 0 {
			continue
		}
		t, add := reduceVerifiedOrbits(orig, rows, lb, ub, orbits, colOrbit, rowTuples)
		if t > 0 || len(add) > 0 {
			return t, add, false
		}
	}
	return 0, nil, false
}

// proposeAndVerifyOrbits expands a seed partitioning or packing row into G-way variable orbits
// and row tuples by BFS over the row-column incidence graph, then verifies exact matrix invariance.
// Sentinel encodings: colOrbit[j] and colGroup[j] hold orbitUnassigned (-1) until column j joins an orbit;
// rowState[i] holds rowStateUnvisited (-1), rowStateInvariant (-2) for rows invariant across all groups,
// or group index g in [0 .. G-1] when row i belongs to a G-row orbit tuple.
func proposeAndVerifyOrbits(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	colRows [][]presolveColEntry,
	rowHash []uint64,
	seedIdx int,
	deadline time.Time,
	done <-chan struct{},
) (orbits [][]int, colOrbit []int, rowTuples [][]int, ok, timedOut bool) {
	n := orig.numVars
	m := len(rows)
	seedCols := slices.Clone(rows[seedIdx].cols)
	slices.Sort(seedCols)
	gCount := len(seedCols)
	for g := 1; g < gCount; g++ {
		if seedCols[g] == seedCols[g-1] {
			return nil, nil, nil, false, false
		}
	}

	colOrbit = make([]int, n)
	colGroup := make([]int, n)
	for j := range n {
		colOrbit[j] = orbitUnassigned
		colGroup[j] = orbitUnassigned
	}
	orbits = [][]int{seedCols}
	for g, c := range seedCols {
		colOrbit[c] = 0
		colGroup[c] = g
	}

	rowState := make([]int, m)
	for i := range m {
		rowState[i] = rowStateUnvisited
	}
	rowState[seedIdx] = rowStateInvariant

	orders := make([][]int, gCount)
	for q := 0; q < len(orbits); q++ {
		if (q&31 == 0) && stopRequested(deadline, done) {
			return nil, nil, nil, false, true
		}
		orb := orbits[q]
		for _, ce := range colRows[orb[0]] {
			r0 := ce.row
			a0 := ce.val
			if rowState[r0] != rowStateUnvisited {
				continue
			}
			rr0 := &rows[r0]
			if isRowInvariantOnOrbit(rr0, orb, a0) {
				rowState[r0] = rowStateInvariant
				continue
			}
			for _, c := range rr0.cols {
				if colGroup[c] > 0 {
					return nil, nil, nil, false, false
				}
			}

			tuple := make([]int, gCount)
			tuple[0] = r0
			for g := 1; g < gCount; g++ {
				vg := orb[g]
				matched := -1
				for _, ceG := range colRows[vg] {
					rg := ceG.row
					if rowState[rg] != rowStateUnvisited || rg == r0 || rowHash[rg] != rowHash[r0] {
						continue
					}
					if math.Abs(ceG.val-a0) > presolveZeroTol {
						continue
					}
					rrg := &rows[rg]
					if rrg.sense != rr0.sense || len(rrg.cols) != len(rr0.cols) || math.Abs(rrg.rhs-rr0.rhs) > presolveZeroTol {
						continue
					}
					dup := false
					for prev := 1; prev < g; prev++ {
						if tuple[prev] == rg {
							dup = true
							break
						}
					}
					if !dup {
						matched = rg
						break
					}
				}
				if matched == -1 {
					return nil, nil, nil, false, false
				}
				tuple[g] = matched
			}
			for g, rg := range tuple {
				rowState[rg] = g
			}
			rowTuples = append(rowTuples, tuple)

			rowLen := len(rr0.cols)
			for g := range gCount {
				rg := &rows[tuple[g]]
				ord := orders[g]
				if cap(ord) < rowLen {
					ord = make([]int, rowLen)
				} else {
					ord = ord[:rowLen]
				}
				for idx := range rowLen {
					ord[idx] = idx
				}
				grp := g
				slices.SortFunc(ord, func(a, b int) int {
					ca, cb := rg.cols[a], rg.cols[b]
					ga, gb := colGroup[ca] == grp, colGroup[cb] == grp
					if ga != gb {
						if ga {
							return -1
						}
						return 1
					}
					if ga {
						return cmp.Compare(colOrbit[ca], colOrbit[cb])
					}
					if c := cmp.Compare(presolveQuantize(rg.vals[a]), presolveQuantize(rg.vals[b])); c != 0 {
						return c
					}
					if c := cmp.Compare(orig.varCat[ca], orig.varCat[cb]); c != 0 {
						return c
					}
					if c := cmp.Compare(presolveQuantize(lb[ca]), presolveQuantize(lb[cb])); c != 0 {
						return c
					}
					if c := cmp.Compare(presolveQuantize(ub[ca]), presolveQuantize(ub[cb])); c != 0 {
						return c
					}
					if c := cmp.Compare(len(colRows[ca]), len(colRows[cb])); c != 0 {
						return c
					}
					return cmp.Compare(ca, cb)
				})
				orders[g] = ord
			}

			for pos := range rowLen {
				c0 := rr0.cols[orders[0][pos]]
				v0 := rr0.vals[orders[0][pos]]
				allSameCol := true
				for g := 1; g < gCount; g++ {
					rg := &rows[tuple[g]]
					cg := rg.cols[orders[g][pos]]
					vg := rg.vals[orders[g][pos]]
					if math.Abs(vg-v0) > presolveZeroTol {
						return nil, nil, nil, false, false
					}
					if cg != c0 {
						allSameCol = false
					}
				}
				if allSameCol {
					if colOrbit[c0] != orbitUnassigned {
						return nil, nil, nil, false, false
					}
					continue
				}
				if colOrbit[c0] >= 0 {
					orbID := colOrbit[c0]
					if colGroup[c0] != 0 {
						return nil, nil, nil, false, false
					}
					for g := 1; g < gCount; g++ {
						cg := rows[tuple[g]].cols[orders[g][pos]]
						if colOrbit[cg] != orbID || colGroup[cg] != g {
							return nil, nil, nil, false, false
						}
					}
					continue
				}
				newOrb := make([]int, gCount)
				newOrb[0] = c0
				for g := 1; g < gCount; g++ {
					cg := rows[tuple[g]].cols[orders[g][pos]]
					if colOrbit[cg] != orbitUnassigned {
						return nil, nil, nil, false, false
					}
					if orig.varCat[cg] != orig.varCat[c0] || !samePresolveBound(lb[cg], lb[c0]) || !samePresolveBound(ub[cg], ub[c0]) || len(colRows[cg]) != len(colRows[c0]) {
						return nil, nil, nil, false, false
					}
					for prev := range g {
						if newOrb[prev] == cg {
							return nil, nil, nil, false, false
						}
					}
					newOrb[g] = cg
				}
				orbID := len(orbits)
				for g, cg := range newOrb {
					colOrbit[cg] = orbID
					colGroup[cg] = g
				}
				orbits = append(orbits, newOrb)
			}
		}
	}

	if !verifyOrbitSymmetry(orig, rows, lb, ub, orbits, colOrbit, colGroup, rowState, rowTuples) {
		return nil, nil, nil, false, false
	}
	return orbits, colOrbit, rowTuples, true, false
}

func isRowInvariantOnOrbit(r *presolveWorkRow, orb []int, targetVal float64) bool {
	matched := 0
	for k, c := range r.cols {
		for _, oc := range orb {
			if c == oc {
				if math.Abs(r.vals[k]-targetVal) > presolveZeroTol {
					return false
				}
				matched++
				break
			}
		}
	}
	return matched == len(orb)
}

// verifyOrbitSymmetry confirms that every active row is either mapped identically across rowTuples
// under all group permutations or invariant on every orbit it intersects.
func verifyOrbitSymmetry(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	orbits [][]int,
	colOrbit, colGroup, rowState []int,
	rowTuples [][]int,
) bool {
	if len(orbits) == 0 {
		return false
	}
	gCount := len(orbits[0])
	n := orig.numVars

	for orbID, orb := range orbits {
		if len(orb) != gCount {
			return false
		}
		c0 := orb[0]
		for g, cg := range orb {
			if colOrbit[cg] != orbID || colGroup[cg] != g {
				return false
			}
			if g > 0 {
				if orig.varCat[cg] != orig.varCat[c0] || !samePresolveBound(lb[cg], lb[c0]) || !samePresolveBound(ub[cg], ub[c0]) {
					return false
				}
			}
		}
	}

	expectedStamp := make([]int, n)
	expectedVal := make([]float64, n)
	stamp := 0

	for _, tuple := range rowTuples {
		if len(tuple) != gCount {
			return false
		}
		r0 := &rows[tuple[0]]
		if !r0.active || rowState[tuple[0]] != 0 {
			return false
		}
		for _, c0 := range r0.cols {
			if colOrbit[c0] >= 0 && colGroup[c0] != 0 {
				return false
			}
		}
		for g := 1; g < gCount; g++ {
			rg := &rows[tuple[g]]
			if !rg.active || rowState[tuple[g]] != g || rg.sense != r0.sense || len(rg.cols) != len(r0.cols) || math.Abs(rg.rhs-r0.rhs) > presolveZeroTol {
				return false
			}
			stamp++
			for k, c0 := range r0.cols {
				cg := c0
				if orb := colOrbit[c0]; orb >= 0 {
					cg = orbits[orb][g]
				}
				expectedStamp[cg] = stamp
				expectedVal[cg] = r0.vals[k]
			}
			for k, cg := range rg.cols {
				if expectedStamp[cg] != stamp || math.Abs(rg.vals[k]-expectedVal[cg]) > presolveZeroTol {
					return false
				}
				expectedStamp[cg] = 0
			}
		}
	}

	numOrbits := len(orbits)
	orbSeen := make([]int, numOrbits)
	orbCount := make([]int, numOrbits)
	orbVal := make([]float64, numOrbits)
	invStamp := 0

	for i := range rows {
		r := &rows[i]
		if !r.active || rowState[i] >= 0 {
			continue
		}
		invStamp++
		for k, c := range r.cols {
			orb := colOrbit[c]
			if orb < 0 {
				continue
			}
			if orbSeen[orb] != invStamp {
				orbSeen[orb] = invStamp
				orbCount[orb] = 1
				orbVal[orb] = r.vals[k]
			} else {
				if math.Abs(r.vals[k]-orbVal[orb]) > presolveZeroTol {
					return false
				}
				orbCount[orb]++
			}
		}
		for _, c := range r.cols {
			if orb := colOrbit[c]; orb >= 0 && orbCount[orb] != gCount {
				return false
			}
		}
	}
	return true
}

// reduceVerifiedOrbits breaks a verified S_G symmetry either by fixing bijective/orbitope binary entries
// when costs are group-invariant or by appending monotone block-open ordering inequalities.
func reduceVerifiedOrbits(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	orbits [][]int,
	colOrbit []int,
	rowTuples [][]int,
) (tightened int, addedRows []presolveWorkRow) {
	gCount := len(orbits[0])
	numOrbits := len(orbits)

	exactObj := true
	monotoneNonNeg := true
	for _, orb := range orbits {
		if lb[orb[0]] < -presolveZeroTol {
			monotoneNonNeg = false
		}
		c0 := orig.objMin[orb[0]]
		for g := 1; g < gCount; g++ {
			cg := orig.objMin[orb[g]]
			if math.Abs(cg-c0) > presolveZeroTol {
				exactObj = false
			}
			if orig.objMin[orb[g-1]] > cg+presolveZeroTol {
				monotoneNonNeg = false
			}
		}
	}

	if exactObj {
		isUnitPartOrb := make([]bool, numOrbits)
		isEqualPartOrb := make([]bool, numOrbits)
		var eqPartOrbits, lePartOrbits []int
		for i := range rows {
			r := &rows[i]
			if !r.active || len(r.cols) != gCount || math.Abs(r.rhs-1.0) > presolveBoundChangeTol {
				continue
			}
			if r.sense != Equal && r.sense != LessEqual {
				continue
			}
			orb := colOrbit[r.cols[0]]
			if orb < 0 || orig.varCat[orbits[orb][0]] != Binary || lb[orbits[orb][0]] != 0 || ub[orbits[orb][0]] != 1 {
				continue
			}
			valid := true
			for k, c := range r.cols {
				if colOrbit[c] != orb || math.Abs(r.vals[k]-1.0) > presolveBoundChangeTol {
					valid = false
					break
				}
			}
			if valid && !isUnitPartOrb[orb] {
				isUnitPartOrb[orb] = true
				if r.sense == Equal {
					isEqualPartOrb[orb] = true
					eqPartOrbits = append(eqPartOrbits, orb)
				} else {
					lePartOrbits = append(lePartOrbits, orb)
				}
			} else if valid && r.sense == Equal && !isEqualPartOrb[orb] {
				isEqualPartOrb[orb] = true
			}
		}

		// Subcase A1: Bijective designated subset (G x G permutation subarray -> identity matrix)
		for _, tuple := range rowTuples {
			r0 := &rows[tuple[0]]
			if !r0.active || r0.sense != Equal || len(r0.cols) != gCount || math.Abs(r0.rhs-1.0) > presolveBoundChangeTol {
				continue
			}
			dOrbs := make([]int, 0, gCount)
			validBij := true
			for k, c := range r0.cols {
				orb := colOrbit[c]
				if orb < 0 || !isUnitPartOrb[orb] || math.Abs(r0.vals[k]-1.0) > presolveBoundChangeTol {
					validBij = false
					break
				}
				if slices.Contains(dOrbs, orb) {
					validBij = false
					break
				}
				dOrbs = append(dOrbs, orb)
			}
			if validBij && len(dOrbs) == gCount {
				slices.Sort(dOrbs)
				for d, orb := range dOrbs {
					for g := range gCount {
						col := orbits[orb][g]
						if g == d {
							if lb[col] < 1 {
								lb[col] = 1
								tightened++
							}
						} else {
							if ub[col] > 0 {
								ub[col] = 0
								tightened++
							}
						}
					}
				}
				return tightened, nil
			}
		}

		// Subcase A2: General partitioning/packing orbitope upper-triangular zero fixings
		partOrbs := make([]int, 0, len(eqPartOrbits)+len(lePartOrbits))
		partOrbs = append(partOrbs, eqPartOrbits...)
		partOrbs = append(partOrbs, lePartOrbits...)
		if len(partOrbs) > 0 {
			if isEqualPartOrb[partOrbs[0]] {
				c00 := orbits[partOrbs[0]][0]
				if lb[c00] < 1 {
					lb[c00] = 1
					tightened++
				}
			}
			limit := min(len(partOrbs), gCount-1)
			for rIdx := range limit {
				orb := partOrbs[rIdx]
				for g := rIdx + 1; g < gCount; g++ {
					col := orbits[orb][g]
					if ub[col] > 0 {
						ub[col] = 0
						tightened++
					}
				}
			}
			return tightened, nil
		}
		return 0, nil
	}

	if !monotoneNonNeg {
		return 0, nil
	}

	// Rule B: Monotone cost-dominated empty-block ordering (y_0 >= y_1 >= ... >= y_{G-1})
	forcedZero := make([]bool, numOrbits)
	for openOrb := range numOrbits {
		cOpen0 := orbits[openOrb][0]
		if orig.varCat[cOpen0] != Binary || lb[cOpen0] != 0 || ub[cOpen0] != 1 {
			continue
		}
		clear(forcedZero)
		forcedZero[openOrb] = true
		for _, tuple := range rowTuples {
			r0 := &rows[tuple[0]]
			if !r0.active || (r0.sense != LessEqual && r0.sense != GreaterEqual) {
				continue
			}
			sign := 1.0
			if r0.sense == GreaterEqual {
				sign = -1.0
			}
			if sign*r0.rhs > presolveZeroTol {
				continue
			}
			hasOpenNeg := false
			allOtherPos := true
			for k, c := range r0.cols {
				a := sign * r0.vals[k]
				if c == cOpen0 {
					if a < -presolveBoundChangeTol {
						hasOpenNeg = true
					} else {
						allOtherPos = false
						break
					}
				} else if a <= presolveBoundChangeTol || lb[c] < -presolveZeroTol {
					allOtherPos = false
					break
				}
			}
			if hasOpenNeg && allOtherPos {
				for _, c := range r0.cols {
					if orb := colOrbit[c]; orb >= 0 {
						forcedZero[orb] = true
					}
				}
			}
		}

		dominatesAll := true
		for orb := range numOrbits {
			if math.Abs(orig.objMin[orbits[orb][gCount-1]]-orig.objMin[orbits[orb][0]]) > presolveZeroTol && !forcedZero[orb] {
				dominatesAll = false
				break
			}
		}
		if dominatesAll {
			addedRows = make([]presolveWorkRow, 0, gCount-1)
			for g := range gCount - 1 {
				addedRows = append(addedRows, presolveWorkRow{
					name:   "__sym_ord_" + strconv.Itoa(g),
					sense:  GreaterEqual,
					rhs:    0,
					cols:   []int{orbits[openOrb][g], orbits[openOrb][g+1]},
					vals:   []float64{1.0, -1.0},
					active: true,
				})
			}
			return 0, addedRows
		}
	}

	return 0, nil
}

func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
