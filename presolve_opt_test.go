package goblin

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"testing"
	"time"
)

func isRaceBuild() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range info.Settings {
		if s.Key == "-race" && s.Value == "true" {
			return true
		}
	}
	return false
}

type testPuLPCoeff struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type testPuLPVar struct {
	Name     string   `json:"name"`
	Cat      string   `json:"cat"`
	LowBound *float64 `json:"lowBound"`
	UpBound  *float64 `json:"upBound"`
}

type testPuLPCon struct {
	Name         string          `json:"name"`
	Sense        int             `json:"sense"`
	Constant     float64         `json:"constant"`
	Coefficients []testPuLPCoeff `json:"coefficients"`
}

type testPuLPJSON struct {
	Parameters struct {
		Name  string `json:"name"`
		Sense int    `json:"sense"`
	} `json:"parameters"`
	Objective struct {
		Name         *string         `json:"name"`
		Constant     float64         `json:"constant"`
		Coefficients []testPuLPCoeff `json:"coefficients"`
	} `json:"objective"`
	Variables   []testPuLPVar `json:"variables"`
	Constraints []testPuLPCon `json:"constraints"`
}

func loadJSONModelForTest(t *testing.T, relPath string) *Model {
	t.Helper()
	raw, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	var doc testPuLPJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", relPath, err)
	}
	sense := Minimize
	if doc.Parameters.Sense == -1 {
		sense = Maximize
	}
	m := NewModel(doc.Parameters.Name, sense)
	m.Reserve(len(doc.Variables), len(doc.Constraints))
	for i := range doc.Variables {
		v := &doc.Variables[i]
		lb := math.Inf(-1)
		if v.LowBound != nil {
			lb = *v.LowBound
		}
		ub := math.Inf(1)
		if v.UpBound != nil {
			ub = *v.UpBound
		}
		cat := Continuous
		switch v.Cat {
		case "Binary":
			if v.LowBound == nil {
				lb = 0
			}
			if v.UpBound == nil {
				ub = 1
			}
			if lb >= 0 && ub <= 1 && lb <= ub {
				cat = Binary
			} else {
				cat = Integer
			}
		case "Integer":
			if lb >= 0 && ub <= 1 && lb <= ub {
				cat = Binary
			} else {
				cat = Integer
			}
		}
		m.AddVar(v.Name, lb, ub, cat)
	}
	objTerms := make([]Term, 0, len(doc.Objective.Coefficients))
	for _, c := range doc.Objective.Coefficients {
		v, ok := m.VarByName(c.Name)
		if !ok {
			t.Fatalf("%s: unknown objective var %q", relPath, c.Name)
		}
		objTerms = append(objTerms, Term{Var: v, Coeff: c.Value})
	}
	objName := ""
	if doc.Objective.Name != nil {
		objName = *doc.Objective.Name
	}
	m.SetObjectiveNamed(objName, Expr{Terms: objTerms, Constant: doc.Objective.Constant})
	var terms []Term
	for i := range doc.Constraints {
		c := &doc.Constraints[i]
		terms = terms[:0]
		for _, coeff := range c.Coefficients {
			v, ok := m.VarByName(coeff.Name)
			if !ok {
				t.Fatalf("%s: unknown constraint var %q", relPath, coeff.Name)
			}
			terms = append(terms, Term{Var: v, Coeff: coeff.Value})
		}
		csense := Equal
		switch c.Sense {
		case -1:
			csense = LessEqual
		case 1:
			csense = GreaterEqual
		}
		m.AddConstraint(c.Name, Constraint{
			Expr:  Expr{Terms: terms},
			Sense: csense,
			RHS:   -c.Constant,
		})
	}
	return m
}

func sameFloat64BitsSlice(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func assertCompiledProblemsEqual(t *testing.T, label string, got, want *CompiledProblem) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s: nil mismatch got=%v want=%v", label, got == nil, want == nil)
	}
	if got == nil {
		return
	}
	if got.name != want.name {
		t.Fatalf("%s: name=%q want %q", label, got.name, want.name)
	}
	if got.objName != want.objName {
		t.Fatalf("%s: objName=%q want %q", label, got.objName, want.objName)
	}
	if got.sense != want.sense {
		t.Fatalf("%s: sense=%v want %v", label, got.sense, want.sense)
	}
	if got.modelID != want.modelID {
		t.Fatalf("%s: modelID=%d want %d", label, got.modelID, want.modelID)
	}
	if got.numVars != want.numVars || got.numConstraints != want.numConstraints || got.isMIP != want.isMIP {
		t.Fatalf("%s: dims=(%d,%d,%v) want (%d,%d,%v)",
			label, got.numVars, got.numConstraints, got.isMIP, want.numVars, want.numConstraints, want.isMIP)
	}
	if math.Float64bits(got.objConstant) != math.Float64bits(want.objConstant) ||
		math.Float64bits(got.objConstMinVal) != math.Float64bits(want.objConstMinVal) {
		t.Fatalf("%s: objConst=(%v,%v) want (%v,%v)",
			label, got.objConstant, got.objConstMinVal, want.objConstant, want.objConstMinVal)
	}
	if !slices.Equal(got.varNames, want.varNames) {
		t.Fatalf("%s: varNames mismatch", label)
	}
	if !sameFloat64BitsSlice(got.varLB, want.varLB) {
		t.Fatalf("%s: varLB mismatch", label)
	}
	if !sameFloat64BitsSlice(got.varUB, want.varUB) {
		t.Fatalf("%s: varUB mismatch", label)
	}
	if !slices.Equal(got.varCat, want.varCat) {
		t.Fatalf("%s: varCat mismatch", label)
	}
	if !slices.Equal(got.binaryMask, want.binaryMask) {
		t.Fatalf("%s: binaryMask mismatch", label)
	}
	if !slices.Equal(got.intCols, want.intCols) {
		t.Fatalf("%s: intCols mismatch", label)
	}
	if !sameFloat64BitsSlice(got.objCoeffs, want.objCoeffs) {
		t.Fatalf("%s: objCoeffs mismatch", label)
	}
	if !sameFloat64BitsSlice(got.objMin, want.objMin) {
		t.Fatalf("%s: objMin mismatch", label)
	}
	if !slices.Equal(got.rowNames, want.rowNames) {
		t.Fatalf("%s: rowNames mismatch", label)
	}
	if !slices.Equal(got.rowSense, want.rowSense) {
		t.Fatalf("%s: rowSense mismatch", label)
	}
	if !sameFloat64BitsSlice(got.rowRHS, want.rowRHS) {
		t.Fatalf("%s: rowRHS mismatch", label)
	}
	if !sameFloat64BitsSlice(got.rowLB, want.rowLB) {
		t.Fatalf("%s: rowLB mismatch", label)
	}
	if !sameFloat64BitsSlice(got.rowUB, want.rowUB) {
		t.Fatalf("%s: rowUB mismatch", label)
	}
	if !slices.Equal(got.rowPtr, want.rowPtr) || !slices.Equal(got.colIdx, want.colIdx) || !sameFloat64BitsSlice(got.rowVal, want.rowVal) {
		t.Fatalf("%s: CSR mismatch", label)
	}
	if !slices.Equal(got.colPtr, want.colPtr) || !slices.Equal(got.rowIdx, want.rowIdx) || !sameFloat64BitsSlice(got.colVal, want.colVal) {
		t.Fatalf("%s: CSC mismatch", label)
	}
	if got.RowStride() != want.RowStride() || !sameFloat64BitsSlice(got.denseRows, want.denseRows) {
		t.Fatalf("%s: denseRows mismatch stride=(%d,%d) len=(%d,%d)",
			label, got.RowStride(), want.RowStride(), len(got.denseRows), len(want.denseRows))
	}
}

// buildCompiledViaModelRef reproduces the pre-optimization Presolve symbolic rebuild path
// so tests can verify byte-for-byte structural equivalence with buildCompiledDirect.
func buildCompiledViaModelRef(orig *CompiledProblem, rows []presolveWorkRow, lb, ub []float64) (red *CompiledProblem, droppedEmpty int, infeas bool) {
	n := orig.numVars
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
	for i := range rows {
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
					return nil, 0, true
				}
			case GreaterEqual:
				if 0 < r.rhs-presolveFeasTol {
					return nil, 0, true
				}
			case Equal:
				if math.Abs(r.rhs) > presolveFeasTol {
					return nil, 0, true
				}
			}
			droppedEmpty++
			continue
		}
		mod.AddConstraint(r.name, Constraint{
			Expr:  Expr{Terms: terms},
			Sense: r.sense,
			RHS:   r.rhs,
		})
	}
	red = mod.Compile()
	red.modelID = orig.modelID
	return red, droppedEmpty, false
}

// runActivityTighteningUnguarded mirrors runActivityTightening without fastRowCanSkip
// or presolveCanSkipTerm so tests can verify exact equivalence across adversarial rows.
func runActivityTighteningUnguarded(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	maxPasses int,
) (tightened, removedRows int, infeas bool) {
	m := len(rows)
	for range maxPasses {
		changedInPass := false
		for i := range m {
			r := &rows[i]
			if !r.active {
				continue
			}
			hasUpper := r.sense == LessEqual || r.sense == Equal
			hasLower := r.sense == GreaterEqual || r.sense == Equal
			s, _ := evalRowActivitySummary(r.cols, r.vals, lb, ub, r.rhs, false, true, true)
			if s.extreme {
				continue
			}
			rowScale := max(1.0, max(s.l1Min, s.l1Max))
			rowFeasTol := max(presolveFeasTol, presolveRelScaleTol*rowScale)
			if hasUpper && s.minInfCount == 0 && s.slackHi < -rowFeasTol {
				return tightened, removedRows, true
			}
			if hasLower && s.maxInfCount == 0 && s.deficitLo > rowFeasTol {
				return tightened, removedRows, true
			}
			if r.sense == LessEqual && s.maxInfCount == 0 && s.deficitLo >= -presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == GreaterEqual && s.minInfCount == 0 && s.slackHi <= presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == Equal && s.minInfCount == 0 && s.maxInfCount == 0 && s.slackHi <= presolveBoundChangeTol && s.deficitLo >= -presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			upperTightenedRow := false
			if hasUpper && s.minInfCount <= 1 {
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					if a > 0 {
						if s.minInfCount == 1 && !math.IsInf(l, -1) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, true)
						if !qOK {
							continue
						}
						newUB := qVal + qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
						if orig.varCat[col] != Continuous {
							newUB = math.Floor(newUB + bTol)
						} else {
							newUB += rowFeasTol / a
						}
						if newUB < u-presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/a)
							if lb[col] > newUB+feasCheck {
								return tightened, removedRows, true
							}
							if lb[col] > newUB {
								newUB = lb[col]
							}
							if newUB < u-presolveIntTol {
								ub[col] = newUB
								tightened++
								changedInPass = true
								upperTightenedRow = true
							}
						}
					} else {
						if s.minInfCount == 1 && !math.IsInf(u, 1) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, true)
						if !qOK {
							continue
						}
						absA := -a
						newLB := qVal - qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
						if orig.varCat[col] != Continuous {
							newLB = math.Ceil(newLB - bTol)
						} else {
							newLB -= rowFeasTol / absA
						}
						if newLB > l+presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/absA)
							if newLB > ub[col]+feasCheck {
								return tightened, removedRows, true
							}
							if newLB > ub[col] {
								newLB = ub[col]
							}
							if newLB > l+presolveIntTol {
								lb[col] = newLB
								tightened++
								changedInPass = true
								upperTightenedRow = true
							}
						}
					}
				}
			}
			if hasLower && upperTightenedRow {
				s, _ = evalRowActivitySummary(r.cols, r.vals, lb, ub, r.rhs, false, false, true)
				if s.extreme {
					continue
				}
				rowScale = max(1.0, s.l1Max)
				rowFeasTol = max(presolveFeasTol, presolveRelScaleTol*rowScale)
				if s.maxInfCount == 0 && s.deficitLo > rowFeasTol {
					return tightened, removedRows, true
				}
			}
			if hasLower && s.maxInfCount <= 1 {
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					if a > 0 {
						if s.maxInfCount == 1 && !math.IsInf(u, 1) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, false)
						if !qOK {
							continue
						}
						newLB := qVal - qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
						if orig.varCat[col] != Continuous {
							newLB = math.Ceil(newLB - bTol)
						} else {
							newLB -= rowFeasTol / a
						}
						if newLB > l+presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/a)
							if newLB > ub[col]+feasCheck {
								return tightened, removedRows, true
							}
							if newLB > ub[col] {
								newLB = ub[col]
							}
							if newLB > l+presolveIntTol {
								lb[col] = newLB
								tightened++
								changedInPass = true
							}
						}
					} else {
						if s.maxInfCount == 1 && !math.IsInf(l, -1) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, false)
						if !qOK {
							continue
						}
						absA := -a
						newUB := qVal + qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
						if orig.varCat[col] != Continuous {
							newUB = math.Floor(newUB + bTol)
						} else {
							newUB += rowFeasTol / absA
						}
						if newUB < u-presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/absA)
							if lb[col] > newUB+feasCheck {
								return tightened, removedRows, true
							}
							if lb[col] > newUB {
								newUB = lb[col]
							}
							if newUB < u-presolveIntTol {
								ub[col] = newUB
								tightened++
								changedInPass = true
							}
						}
					}
				}
			}
		}
		if !changedInPass {
			break
		}
	}
	return tightened, removedRows, false
}

func TestPresolveCoWImmutability(t *testing.T) {
	t.Parallel()

	// Synthetic model triggering GCD scaling, unit-GCD rounding, positive and negative
	// binary coefficient tightening, bound tightening, and redundant row removal.
	mSyn := NewModel("cow_mutations", Maximize)
	x0 := mSyn.AddBinaryVar("x0")
	x1 := mSyn.AddBinaryVar("x1")
	x2 := mSyn.AddBinaryVar("x2")
	y0 := mSyn.AddIntVar("y0", 0, 10)
	y1 := mSyn.AddIntVar("y1", 0, 10)
	mSyn.SetObjective(NewExpr(x0.T(3), x1.T(5), x2.T(2), y0.T(1), y1.T(1)).AddConstant(7.5))
	// Pure integer row with GCD = 4: 4*y0 + 8*y1 <= 15 -> y0 + 2*y1 <= 3.
	mSyn.AddConstraint("gcd_row", NewExpr(y0.T(4), y1.T(8)).Le(15))
	// Near-integer unit-GCD row: (1 + 1e-10)*y0 + y1 <= 6.4.
	mSyn.AddConstraint("near_int_row", NewExpr(y0.T(1+1e-10), y1.T(1)).Le(6.4))
	// Positive binary coefficient tightening: 5*x0 + 2*x1 + 2*x2 <= 6 (maxAct=9, surplus=3 -> 5 shrinks to 3).
	mSyn.AddConstraint("pos_bin_tight", NewExpr(x0.T(5), x1.T(2), x2.T(2)).Le(6))
	// Negative binary coefficient tightening: -5*x0 + 2*x1 + 2*x2 <= 1.
	mSyn.AddConstraint("neg_bin_tight", NewExpr(x0.T(-5), x1.T(2), x2.T(2)).Le(1))
	// Redundant row: x0 + x1 <= 10.
	mSyn.AddConstraint("redundant_row", NewExpr(x0.T(1), x1.T(1)).Le(10))

	// Symmetric 3-group assignment model with a redundant row dropped before symmetry
	// breaking so both the flat two-pass CSC builder (reuseOrigCSC == false) and orbit
	// fixing are exercised.
	mSym := NewModel("sym_with_redundant", Minimize)
	xSym := make([][]Var, 4)
	var symObj []Term
	for item := range 4 {
		xSym[item] = make([]Var, 3)
		for grp := range 3 {
			v := mSym.AddBinaryVar(fmt.Sprintf("x_%d_%d", item, grp))
			xSym[item][grp] = v
			symObj = append(symObj, v.T(float64(item+1)))
		}
		mSym.AddConstraint(fmt.Sprintf("assign_%d", item), NewExpr(
			xSym[item][0].T(1), xSym[item][1].T(1), xSym[item][2].T(1),
		).Eq(1))
	}
	for grp := range 3 {
		mSym.AddConstraint(fmt.Sprintf("cap_%d", grp), NewExpr(
			xSym[0][grp].T(1), xSym[1][grp].T(1), xSym[2][grp].T(1), xSym[3][grp].T(1),
		).Le(2))
	}
	// Redundant row dropped in step 2, forcing applyOrbitSymmetryBreaking to build flat CSC.
	mSym.AddConstraint("redundant_drop", NewExpr(xSym[0][0].T(1), xSym[0][1].T(1)).Le(10))
	mSym.SetObjective(Expr{Terms: symObj})

	models := []*Model{
		mSyn,
		mSym,
		loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t3_mad_g4_i1000.json")),
		loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t3_mad_g10_i100.json")),
		loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t2_binpack_44.json")),
		loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t2_lotsizing_48.json")),
	}

	for _, m := range models {
		orig := m.Compile()
		snapColIdx := slices.Clone(orig.colIdx)
		snapRowVal := slices.Clone(orig.rowVal)
		snapRowPtr := slices.Clone(orig.rowPtr)
		snapColPtr := slices.Clone(orig.colPtr)
		snapRowIdx := slices.Clone(orig.rowIdx)
		snapColVal := slices.Clone(orig.colVal)
		snapVarLB := slices.Clone(orig.varLB)
		snapVarUB := slices.Clone(orig.varUB)
		snapRowRHS := slices.Clone(orig.rowRHS)
		snapRowLB := slices.Clone(orig.rowLB)
		snapRowUB := slices.Clone(orig.rowUB)
		snapDense := slices.Clone(orig.denseRows)

		res1 := Presolve(orig, defaultPresolveMaxPass)
		if res1.Infeasible || res1.TimedOut {
			t.Fatalf("%s: unexpected infeasible=%v timedOut=%v", orig.name, res1.Infeasible, res1.TimedOut)
		}
		if res1.TightenedBounds == 0 && res1.RemovedRows == 0 && res1.StrengthenedRows == 0 {
			t.Fatalf("%s: expected presolve to mutate at least one bound or row", orig.name)
		}

		// Verify input CompiledProblem arrays are byte-for-byte unchanged.
		if !slices.Equal(orig.colIdx, snapColIdx) ||
			!sameFloat64BitsSlice(orig.rowVal, snapRowVal) ||
			!slices.Equal(orig.rowPtr, snapRowPtr) ||
			!slices.Equal(orig.colPtr, snapColPtr) ||
			!slices.Equal(orig.rowIdx, snapRowIdx) ||
			!sameFloat64BitsSlice(orig.colVal, snapColVal) ||
			!sameFloat64BitsSlice(orig.varLB, snapVarLB) ||
			!sameFloat64BitsSlice(orig.varUB, snapVarUB) ||
			!sameFloat64BitsSlice(orig.rowRHS, snapRowRHS) ||
			!sameFloat64BitsSlice(orig.rowLB, snapRowLB) ||
			!sameFloat64BitsSlice(orig.rowUB, snapRowUB) ||
			!sameFloat64BitsSlice(orig.denseRows, snapDense) {
			t.Fatalf("%s: Presolve mutated input CompiledProblem", orig.name)
		}

		// Running Presolve a second time must produce an identical reduced problem.
		res2 := Presolve(orig, defaultPresolveMaxPass)
		assertCompiledProblemsEqual(t, orig.name+"/repeat", res2.Reduced, res1.Reduced)
	}
}

func TestBuildCompiledDirectEquivalence(t *testing.T) {
	t.Parallel()

	// 1. Synthetic edge cases: Maximize/Minimize, objConstant, fixed-zero vars, empty feasible/infeasible
	// rows, integer-to-binary promotion, unsorted 2-col symmetry rows, duplicate merged columns, zero drops.
	for _, sense := range []Sense{Minimize, Maximize} {
		m := NewModel(fmt.Sprintf("direct_%s", sense), sense)
		v0 := m.AddContVar("v0", 0, 10)
		v1 := m.AddIntVar("v1", 0, 5)
		v2 := m.AddBinaryVar("v2")
		v3 := m.AddIntVar("v3", -3, 3)
		m.SetObjectiveNamed("obj", NewExpr(v0.T(2.5), v1.T(-1.25), v2.T(4.0), v3.T(0)).AddConstant(-13.5))
		m.AddConstraint("r_le", NewExpr(v0.T(1), v1.T(2), v2.T(3)).Le(9))
		m.AddConstraint("r_ge", NewExpr(v1.T(1), v3.T(-1)).Ge(-1))
		m.AddConstraint("r_eq", NewExpr(v0.T(1), v2.T(1)).Eq(2))
		orig := m.Compile()

		// Prepare synthetic presolved rows including unsorted cols, duplicate cols (one cancelling to zero),
		// zero coeff, fixed-zero var v2, integer v1 tightened to [0,1] (Binary promotion), and empty row.
		lb := []float64{0, 0, 0, -2}
		ub := []float64{8, 1, 0, 2}
		rows := []presolveWorkRow{
			{name: "r_le", cols: []int{0, 1, 2}, vals: []float64{1, 2, 3}, rhs: 9, sense: LessEqual, active: true},
			{name: "r_ge", cols: []int{3, 1}, vals: []float64{-1, 1}, rhs: -1, sense: GreaterEqual, active: true},
			{name: "r_eq_dup", cols: []int{3, 0, 3, 1, 1}, vals: []float64{2, 1, -2, 0.5, 1.5}, rhs: 4, sense: Equal, active: true},
			{name: "r_empty_ok", cols: []int{2}, vals: []float64{5}, rhs: 1, sense: LessEqual, active: true},
			{name: "r_inactive", cols: []int{0}, vals: []float64{1}, rhs: 1, sense: LessEqual, active: false},
			{name: "__sym_ord_0", cols: []int{3, 1}, vals: []float64{1, -1}, rhs: 0, sense: GreaterEqual, active: true},
		}

		rowsA := make([]presolveWorkRow, len(rows))
		copy(rowsA, rows)
		rowsB := make([]presolveWorkRow, len(rows))
		copy(rowsB, rows)

		got, gotDrop, gotInf := buildCompiledDirect(orig, rowsA, slices.Clone(lb), slices.Clone(ub))
		want, wantDrop, wantInf := buildCompiledViaModelRef(orig, rowsB, slices.Clone(lb), slices.Clone(ub))
		if gotInf != wantInf || gotDrop != wantDrop {
			t.Fatalf("%s: inf=(%v,%v) drop=(%d,%d)", sense, gotInf, wantInf, gotDrop, wantDrop)
		}
		assertCompiledProblemsEqual(t, fmt.Sprintf("synthetic_%s", sense), got, want)

		// Empty infeasible row check for LessEqual, GreaterEqual, and Equal.
		for _, badRow := range []presolveWorkRow{
			{name: "bad_le", cols: []int{2}, vals: []float64{1}, rhs: -1, sense: LessEqual, active: true},
			{name: "bad_ge", cols: []int{2}, vals: []float64{1}, rhs: 1, sense: GreaterEqual, active: true},
			{name: "bad_eq", cols: []int{2}, vals: []float64{1}, rhs: 0.5, sense: Equal, active: true},
		} {
			g, _, gi := buildCompiledDirect(orig, []presolveWorkRow{badRow}, slices.Clone(lb), slices.Clone(ub))
			_, _, wi := buildCompiledViaModelRef(orig, []presolveWorkRow{badRow}, slices.Clone(lb), slices.Clone(ub))
			if !gi || !wi || g != nil {
				t.Fatalf("%s/%s: expected infeasible=true, got=(%v, %v) want=%v", sense, badRow.name, gi, g == nil, wi)
			}
		}
	}

	// 2. Corpus and public JSON models: compare buildCompiledDirect against buildCompiledViaModelRef
	// on the exact post-presolve work rows (including Rule B monotone symmetry-breaking added rows).
	files, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.json"))
	if err != nil {
		t.Fatalf("glob corpus: %v", err)
	}
	pubFiles, err := filepath.Glob(filepath.Join("testdata", "public", "*.json"))
	if err != nil {
		t.Fatalf("glob public: %v", err)
	}
	for _, p := range pubFiles {
		if filepath.Base(p) != "manifest.json" {
			files = append(files, p)
		}
	}
	for _, path := range files {
		m := loadJSONModelForTest(t, path)
		orig := m.Compile()
		if !orig.isMIP || orig.numVars == 0 {
			continue
		}
		lb := slices.Clone(orig.varLB)
		ub := slices.Clone(orig.varUB)
		rows := make([]presolveWorkRow, orig.numConstraints)
		for i := range orig.numConstraints {
			s, e := orig.rowPtr[i], orig.rowPtr[i+1]
			rows[i] = presolveWorkRow{
				name:   orig.rowNames[i],
				cols:   orig.colIdx[s:e:e],
				vals:   orig.rowVal[s:e:e],
				rhs:    orig.rowRHS[i],
				sense:  orig.rowSense[i],
				active: true,
				shared: true,
			}
		}
		_, _, infeas, _ := runActivityTightening(orig, rows, lb, ub, 2, time.Time{}, nil)
		if infeas {
			continue
		}
		symTight, symRows, _ := applyOrbitSymmetryBreaking(orig, rows, lb, ub, time.Time{}, nil)
		if len(symRows) > 0 {
			rows = slices.Concat(rows, symRows)
		}
		if symTight > 0 {
			_, _, infeas, _ = runActivityTightening(orig, rows, lb, ub, 2, time.Time{}, nil)
			if infeas {
				continue
			}
		}
		rowsA := make([]presolveWorkRow, len(rows))
		copy(rowsA, rows)
		rowsB := make([]presolveWorkRow, len(rows))
		copy(rowsB, rows)

		got, gotDrop, gotInf := buildCompiledDirect(orig, rowsA, slices.Clone(lb), slices.Clone(ub))
		want, wantDrop, wantInf := buildCompiledViaModelRef(orig, rowsB, slices.Clone(lb), slices.Clone(ub))
		if gotInf != wantInf || gotDrop != wantDrop {
			t.Fatalf("%s: inf=(%v,%v) drop=(%d,%d)", path, gotInf, wantInf, gotDrop, wantDrop)
		}
		assertCompiledProblemsEqual(t, filepath.Base(path), got, want)
	}
}

func TestRunActivityTighteningFastGuardEquivalence(t *testing.T) {
	t.Parallel()

	// 1. Extreme-scale and cancellation rows must fall back (fastRowCanSkip == false).
	mExt := NewModel("extreme_guard", Minimize)
	x0 := mExt.AddIntVar("x0", 0, 10)
	x1 := mExt.AddIntVar("x1", 0, 10)
	x2 := mExt.AddContVar("x2", 0, 1e15)
	mExt.SetObjective(NewExpr(x0.T(1), x1.T(1), x2.T(1)))
	mExt.AddConstraint("huge_coeff", NewExpr(x0.T(1e14), x1.T(1)).Le(1e14+5))
	mExt.AddConstraint("tiny_coeff", NewExpr(x0.T(1e-14), x1.T(1)).Le(5))
	mExt.AddConstraint("huge_prod", NewExpr(x2.T(1), x0.T(1)).Le(1e15+1))
	mExt.AddConstraint("near_thresh", NewExpr(x0.T(1), x1.T(1)).Le(10.0+1e-8))
	mExt.AddConstraint("huge_slack", NewExpr(x0.T(1), x1.T(1)).Le(500))
	pExt := mExt.Compile()

	rowsExt := make([]presolveWorkRow, pExt.numConstraints)
	for i := range pExt.numConstraints {
		s, e := pExt.rowPtr[i], pExt.rowPtr[i+1]
		rowsExt[i] = presolveWorkRow{
			name:   pExt.rowNames[i],
			cols:   pExt.colIdx[s:e:e],
			vals:   pExt.rowVal[s:e:e],
			rhs:    pExt.rowRHS[i],
			sense:  pExt.rowSense[i],
			active: true,
			shared: true,
		}
	}
	for i := range 4 {
		if fastRowCanSkip(pExt, &rowsExt[i], pExt.varLB, pExt.varUB) {
			t.Fatalf("row %q unexpectedly skipped by fastRowCanSkip", rowsExt[i].name)
		}
	}

	// 2. Compare guarded vs unguarded runActivityTightening across corpus MIPs.
	corpusIDs := []string{
		"t2_binpack_44.json",
		"t2_cflp_36.json",
		"t2_gap_45.json",
		"t2_lotsizing_48.json",
		"t2_mad_g4_i12.json",
		"t3_cflp_300.json",
		"t3_mad_g10_i100.json",
		"t3_mad_g4_i1000.json",
		"t3_sched_1000.json",
	}
	for _, fn := range corpusIDs {
		orig := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", fn)).Compile()
		makeRows := func() []presolveWorkRow {
			rows := make([]presolveWorkRow, orig.numConstraints)
			for i := range orig.numConstraints {
				s, e := orig.rowPtr[i], orig.rowPtr[i+1]
				rows[i] = presolveWorkRow{
					name:   orig.rowNames[i],
					cols:   orig.colIdx[s:e:e],
					vals:   orig.rowVal[s:e:e],
					rhs:    orig.rowRHS[i],
					sense:  orig.rowSense[i],
					active: true,
					shared: true,
				}
			}
			return rows
		}
		lbG, ubG := slices.Clone(orig.varLB), slices.Clone(orig.varUB)
		rowsG := makeRows()
		tG, rG, infG, _ := runActivityTightening(orig, rowsG, lbG, ubG, defaultPresolveMaxPass, time.Time{}, nil)

		lbU, ubU := slices.Clone(orig.varLB), slices.Clone(orig.varUB)
		rowsU := makeRows()
		tU, rU, infU := runActivityTighteningUnguarded(orig, rowsU, lbU, ubU, defaultPresolveMaxPass)

		if tG != tU || rG != rU || infG != infU {
			t.Fatalf("%s: guarded=(tight=%d rem=%d inf=%v) want=(tight=%d rem=%d inf=%v)", fn, tG, rG, infG, tU, rU, infU)
		}
		if !sameFloat64BitsSlice(lbG, lbU) || !sameFloat64BitsSlice(ubG, ubU) {
			t.Fatalf("%s: tightened bounds diverged between guarded and unguarded paths", fn)
		}
		for i := range rowsG {
			if rowsG[i].active != rowsU[i].active {
				t.Fatalf("%s: row %d active mismatch (%v vs %v)", fn, i, rowsG[i].active, rowsU[i].active)
			}
		}
	}
}

func TestDenseRowsGatingAndSparseSimplexSafety(t *testing.T) {
	t.Parallel()

	// 1. Sparse medium LP (n=100, mc=100, ~4% dense) containing a dense row with 40 nonzeros.
	// Must NOT build denseRows (density < 20% and mc > 32) and must solve cleanly without panicking.
	mSparse := NewModel("sparse_with_dense_row", Minimize)
	vars := make([]Var, 100)
	objTerms := make([]Term, 100)
	for j := range 100 {
		vars[j] = mSparse.AddContVar(fmt.Sprintf("x_%d", j), 0, 10)
		objTerms[j] = vars[j].T(float64(j%7 + 1))
	}
	mSparse.SetObjective(Expr{Terms: objTerms})
	// Row 0: dense coupling row with 40 nonzeros (> sparseRowCap under both scalar and AVX2).
	denseTerms := make([]Term, 40)
	for j := range 40 {
		denseTerms[j] = vars[j].T(1)
	}
	mSparse.AddConstraint("dense_row", Expr{Terms: denseTerms}.Ge(15))
	// Remaining 99 rows: 2-term sparse rows.
	for i := 1; i < 100; i++ {
		mSparse.AddConstraint(fmt.Sprintf("r_%d", i), NewExpr(vars[i-1].T(1), vars[i].T(1)).Ge(1))
	}
	pSparse := mSparse.Compile()
	if pSparse.DenseRowLen() != 0 || pSparse.RowStride() != 0 {
		t.Fatalf("expected sparse-only layout on 100x100 (~2.4%% dense), got DenseRowLen=%d RowStride=%d",
			pSparse.DenseRowLen(), pSparse.RowStride())
	}
	for _, opts := range [][]Option{{WithPureGo()}, {WithPureGo(), WithoutSIMD()}} {
		res, err := SolveCompiled(t.Context(), pSparse, opts...)
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("sparse medium LP solve failed: status=%v err=%v", res.Status, err)
		}
	}

	// 2. Dense medium LP (n=64, mc=64, 25% dense) must build denseRows.
	mDense := NewModel("dense_64", Minimize)
	dVars := make([]Var, 64)
	for j := range 64 {
		dVars[j] = mDense.AddContVar(fmt.Sprintf("d_%d", j), 0, 5)
	}
	mDense.SetObjective(VarExpr(dVars[0]))
	for i := range 64 {
		terms := make([]Term, 16)
		for k := range 16 {
			terms[k] = dVars[(i+k)%64].T(1)
		}
		mDense.AddConstraint(fmt.Sprintf("dr_%d", i), Expr{Terms: terms}.Ge(1))
	}
	pDense := mDense.Compile()
	if pDense.RowStride() != 64 || pDense.DenseRowLen() != 64*64 {
		t.Fatalf("expected dense layout (stride=64, len=4096) on 25%%-dense 64x64 matrix, got stride=%d len=%d",
			pDense.RowStride(), pDense.DenseRowLen())
	}
}

func TestEnsureCBCDataConcurrency(t *testing.T) {
	t.Parallel()

	m := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t2_mad_g4_i12.json"))
	p := m.Compile()

	// Freshly compiled problem must NOT allocate CBC arrays eagerly.
	if p.lazy == nil || p.lazy.cbc != nil {
		t.Fatalf("expected lazy CBC cache to be nil after pure-Go Compile()")
	}

	const workers = 16
	results := make([]*cbcMatrixData, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for w := range workers {
		wg.Go(func() {
			<-start
			if w%2 == 0 {
				results[w] = p.ensureCBCData()
			} else {
				// Shallow feasibility copy sharing p.lazy must also be race-free.
				feas := *p
				feas.objCoeffs = make([]float64, p.numVars)
				feas.objMin = make([]float64, p.numVars)
				results[w] = feas.ensureCBCData()
			}
		})
	}
	close(start)
	wg.Wait()

	ref := results[0]
	if ref == nil || !ref.valid {
		t.Fatalf("expected valid cbcMatrixData, got %+v", ref)
	}
	if len(ref.colPtr32) != p.numVars+1 || len(ref.rowIdx32) != len(p.colIdx) ||
		len(ref.colLBClp) != p.numVars || len(ref.colUBClp) != p.numVars ||
		len(ref.rowLBClp) != p.numConstraints || len(ref.rowUBClp) != p.numConstraints ||
		len(ref.intCols32) != len(p.intCols) {
		t.Fatalf("unexpected cbcMatrixData slice lengths")
	}
	for w := 1; w < workers; w++ {
		if results[w] != ref {
			t.Fatalf("worker %d got distinct cbcMatrixData pointer", w)
		}
	}

	// Zero-value CompiledProblem (p.lazy == nil) must remain safe under ensureCBCData.
	var zeroP CompiledProblem
	zc := zeroP.ensureCBCData()
	if zc == nil || zc.valid {
		t.Fatalf("expected invalid non-nil cbcMatrixData on zero CompiledProblem, got %+v", zc)
	}

	// When CBC is compiled in, verify concurrent SolveCompiled calls on one shared CompiledProblem.
	if CBCAvailable() {
		pLP := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t1_whiskas.json")).Compile()
		pMIP := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t1_knapsack_12.json")).Compile()
		for _, prob := range []*CompiledProblem{pLP, pMIP} {
			const cbcWorkers = 8
			objs := make([]float64, cbcWorkers)
			var cwg sync.WaitGroup
			cStart := make(chan struct{})
			for w := range cbcWorkers {
				cwg.Go(func() {
					<-cStart
					res, err := SolveCompiled(context.Background(), prob, WithCBC())
					if err != nil || res.Status != StatusOptimal {
						t.Errorf("%s worker %d: status=%v err=%v", prob.name, w, res.Status, err)
						return
					}
					objs[w] = res.Objective
				})
			}
			close(cStart)
			cwg.Wait()
			for w := 1; w < cbcWorkers; w++ {
				if math.Abs(objs[w]-objs[0]) > 1e-6 {
					t.Fatalf("%s concurrent CBC objective mismatch: %v vs %v", prob.name, objs[w], objs[0])
				}
			}
		}
	}
}

func TestCompiledSinglePassValidationParity(t *testing.T) {
	t.Parallel()

	// Nil model.
	var nilModel *Model
	if _, err := nilModel.CompileChecked(); !errors.Is(err, ErrNilModel) {
		t.Fatalf("nil CompileChecked err = %v, want ErrNilModel", err)
	}

	// Foreign Var in objective and constraint.
	other := NewModel("other", Minimize)
	foreignV := other.AddContVar("fv", 0, 1)

	cases := []struct {
		name  string
		build func() *Model
	}{
		{
			name: "invalid_sense",
			build: func() *Model {
				return NewModel("bad_sense", Sense(9))
			},
		},
		{
			name: "invalid_obj_const",
			build: func() *Model {
				m := NewModel("bad_const", Minimize)
				x := m.AddContVar("x", 0, 1)
				m.SetObjective(NewExpr(x.T(1)).AddConstant(math.Inf(1)))
				return m
			},
		},
		{
			name: "foreign_obj_var_zero_coeff",
			build: func() *Model {
				m := NewModel("foreign_obj", Minimize)
				_ = m.AddContVar("x", 0, 1)
				m.SetObjective(NewExpr(foreignV.T(0)))
				return m
			},
		},
		{
			name: "overflow_dup_obj",
			build: func() *Model {
				m := NewModel("overflow_obj", Minimize)
				x := m.AddContVar("x", 0, 1)
				m.SetObjective(NewExpr(x.T(1e308), x.T(1e308)))
				return m
			},
		},
		{
			name: "foreign_con_var_zero_coeff",
			build: func() *Model {
				m := NewModel("foreign_con", Minimize)
				x := m.AddContVar("x", 0, 1)
				m.SetObjective(VarExpr(x))
				m.AddConstraint("c", NewExpr(foreignV.T(0)).Le(1))
				return m
			},
		},
		{
			name: "overflow_dup_con",
			build: func() *Model {
				m := NewModel("overflow_con", Minimize)
				x := m.AddContVar("x", 0, 1)
				m.SetObjective(VarExpr(x))
				m.AddConstraint("c", NewExpr(x.T(1e308), x.T(1e308)).Le(1))
				return m
			},
		},
		{
			name: "nonfinite_rhs",
			build: func() *Model {
				m := NewModel("bad_rhs", Minimize)
				x := m.AddContVar("x", 0, 1)
				m.SetObjective(VarExpr(x))
				m.AddConstraint("c", NewExpr(x.T(1)).Le(math.NaN()))
				return m
			},
		},
	}

	for _, tc := range cases {
		m := tc.build()
		valErr := m.Validate()
		if valErr == nil {
			t.Fatalf("%s: expected Validate() error", tc.name)
		}
		if _, err := m.CompileChecked(); err == nil || err.Error() != valErr.Error() {
			t.Fatalf("%s: CompileChecked err = %v, want %v", tc.name, err, valErr)
		}
		p := m.Compile()
		if p == nil || p.Err() == nil || p.Err().Error() != valErr.Error() {
			t.Fatalf("%s: Compile().Err() = %v, want %v", tc.name, p.Err(), valErr)
		}
	}
}

func TestPresolveAndCompileAllocations(t *testing.T) {
	if isRaceBuild() {
		t.Skip("allocation counts include race instrumentation under -race")
	}

	measure := func(runs int, fn func()) (avgAllocs float64, avgBytes float64) {
		fn() // warm up
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for range runs {
			fn()
		}
		runtime.ReadMemStats(&after)
		avgAllocs = float64(after.Mallocs-before.Mallocs) / float64(runs)
		avgBytes = float64(after.TotalAlloc-before.TotalAlloc) / float64(runs)
		return avgAllocs, avgBytes
	}

	// 1. Presolve on t3_mad_g4_i1000 (Phase 1 baseline: 25,543 allocs / 8.87 MB).
	mMAD := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t3_mad_g4_i1000.json"))
	pMAD := mMAD.Compile()
	preAllocs, preBytes := measure(5, func() {
		res := Presolve(pMAD, defaultPresolveMaxPass)
		if res.Infeasible || res.Reduced == nil {
			t.Fatalf("unexpected presolve failure on t3_mad_g4_i1000")
		}
	})
	t.Logf("t3_mad_g4_i1000 Presolve: %.1f allocs/op, %.1f KB/op (baseline: 25543 allocs, 9083 KB)", preAllocs, preBytes/1024)
	if preAllocs > 2000 {
		t.Fatalf("t3_mad_g4_i1000 Presolve allocs/op = %.1f, want <= 2000", preAllocs)
	}

	// 2. Compile on t3_mad_g4_i1000, t3_sched_1000, t2_lotsizing_48 (lazy CBC + single-pass validation).
	for _, tc := range []struct {
		file      string
		maxAllocs float64
	}{
		{"t3_mad_g4_i1000.json", 40},
		{"t3_sched_1000.json", 40},
		{"t2_lotsizing_48.json", 40},
	} {
		mdl := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", tc.file))
		cAllocs, cBytes := measure(10, func() {
			cp := mdl.Compile()
			if cp.Err() != nil {
				t.Fatalf("%s: compile err %v", tc.file, cp.Err())
			}
		})
		t.Logf("%s Compile: %.1f allocs/op, %.1f KB/op", tc.file, cAllocs, cBytes/1024)
		if cAllocs > tc.maxAllocs {
			t.Fatalf("%s Compile allocs/op = %.1f, want <= %.0f", tc.file, cAllocs, tc.maxAllocs)
		}
	}

	// 3. Flat Model.conTerms arena avoids per-constraint slice allocations in AddConstraint.
	varNames := make([]string, 200)
	for j := range 200 {
		varNames[j] = fmt.Sprintf("v_%d", j)
	}
	conNames := make([]string, 1000)
	for i := range 1000 {
		conNames[i] = fmt.Sprintf("c_%d", i)
	}
	addAllocs, addBytes := measure(10, func() {
		m := NewModel("arena_test", Minimize)
		m.Reserve(200, 1000)
		vars := make([]Var, 200)
		for j := range 200 {
			vars[j] = m.AddContVar(varNames[j], 0, 10)
		}
		terms := make([]Term, 4)
		for i := range 1000 {
			terms[0] = vars[i%200].T(1)
			terms[1] = vars[(i+1)%200].T(2)
			terms[2] = vars[(i+2)%200].T(-1)
			terms[3] = vars[(i+3)%200].T(0.5)
			m.AddConstraint(conNames[i], Constraint{
				Expr:  Expr{Terms: terms},
				Sense: LessEqual,
				RHS:   10,
			})
		}
		_ = m.NumConstraints()
	})
	t.Logf("Model.AddConstraint(1000 rows, preallocated): %.1f allocs/op, %.1f KB/op", addAllocs, addBytes/1024)
	if addAllocs > 50 {
		t.Fatalf("Model.AddConstraint(1000) allocs/op = %.1f, want <= 50", addAllocs)
	}
}

// TestLPPresolveAndExactDualPostsolve verifies LP presolve reductions, reverse singleton
// dual postsolve, KKT consistency across Minimize and Maximize LPs, infeasible/unbounded
// status parity, and full dual/reduced-cost parity across all 14 public Netlib LPs.
func TestLPPresolveAndExactDualPostsolve(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	assertDualKKT := func(t *testing.T, label string, p *CompiledProblem, res Result) {
		t.Helper()
		n := p.NumVars()
		m := p.NumConstraints()
		if len(res.Values) != n || len(res.Duals) != m || len(res.ReducedCosts) != n {
			t.Fatalf("%s: vector lengths (x=%d, y=%d, rc=%d), want (%d, %d, %d)",
				label, len(res.Values), len(res.Duals), len(res.ReducedCosts), n, m, n)
		}
		bV, rV, _ := p.MaxViolation(res.Values, false)
		if bV > 1e-6 || rV > 1e-6 {
			t.Fatalf("%s: primal violation b=%.2e r=%.2e", label, bV, rV)
		}
		sign := 1.0
		if p.Sense() == Maximize {
			sign = -1.0
		}
		yMin := make([]float64, m)
		for i := range m {
			yMin[i] = sign * res.Duals[i]
		}
		for i := range m {
			var acc dot2Acc
			acc.addVal(-p.rowRHS[i])
			for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
				acc.addProd(p.rowVal[k], res.Values[p.colIdx[k]])
			}
			slack, _, _ := acc.result()
			switch p.rowSense[i] {
			case LessEqual:
				if yMin[i] > 1e-5*max(1.0, math.Abs(yMin[i])) {
					t.Fatalf("%s: row %d LE dual yMin=%.3e > 0", label, i, yMin[i])
				}
				if slack < -1e-5 && math.Abs(yMin[i]*slack) > 1e-3*max(1.0, math.Abs(p.rowRHS[i])) {
					t.Fatalf("%s: row %d LE comp slack y=%.3e slack=%.3e", label, i, yMin[i], slack)
				}
			case GreaterEqual:
				if yMin[i] < -1e-5*max(1.0, math.Abs(yMin[i])) {
					t.Fatalf("%s: row %d GE dual yMin=%.3e < 0", label, i, yMin[i])
				}
				if slack > 1e-5 && math.Abs(yMin[i]*slack) > 1e-3*max(1.0, math.Abs(p.rowRHS[i])) {
					t.Fatalf("%s: row %d GE comp slack y=%.3e slack=%.3e", label, i, yMin[i], slack)
				}
			}
		}
		for j := range n {
			wantRC := sign * evalOrigColDualMin(p, j, yMin)
			if math.Abs(res.ReducedCosts[j]-wantRC) > 1e-5*max(1.0, math.Abs(wantRC)) {
				t.Fatalf("%s: col %d rc=%.8g, want %.8g", label, j, res.ReducedCosts[j], wantRC)
			}
		}
	}

	// 1. Minimize LP exercising fixed non-zero var, singleton LE/GE/EQ (pos & neg coeffs),
	// redundant row, and dual-dominated column.
	mMin := NewModel("lp_pre_min", Minimize)
	xFix := mMin.AddContVar("x_fix", 3, 3)
	xLE := mMin.AddContVar("x_le", 0, 10)
	xGE := mMin.AddContVar("x_ge", 0, 10)
	xEq := mMin.AddContVar("x_eq", 0, 10)
	xDom := mMin.AddContVar("x_dom", 1, math.Inf(1))
	xFree := mMin.AddContVar("x_free", 0, 10)
	mMin.SetObjective(NewExpr(xFix.T(4), xLE.T(-3), xGE.T(5), xEq.T(7), xDom.T(2), xFree.T(1)))
	mMin.AddConstraint("s_le_pos", NewExpr(xLE.T(2)).Le(8))
	mMin.AddConstraint("s_le_neg", NewExpr(xGE.T(-2)).Le(-6))
	mMin.AddConstraint("s_eq", NewExpr(xEq.T(3)).Eq(6))
	mMin.AddConstraint("coupled", NewExpr(xFix.T(1), xLE.T(1), xGE.T(1), xFree.T(2)).Ge(10))
	mMin.AddConstraint("redund", NewExpr(xLE.T(1), xGE.T(1)).Le(50))
	mMin.AddConstraint("dom_row", NewExpr(xDom.T(1), xFree.T(1)).Le(20))

	pMin := mMin.Compile()
	lpPre := presolveLPWithDeadline(pMin, 6, time.Time{}, nil)
	if lpPre.Infeasible || lpPre.Reduced == pMin {
		t.Fatalf("expected non-trivial LP presolve reduction on lp_pre_min")
	}
	resOn, err := SolveCompiled(ctx, pMin, WithPresolve(true))
	if err != nil {
		t.Fatalf("lp_pre_min presolve=true err: %v", err)
	}
	resOff, err := SolveCompiled(ctx, pMin, WithPresolve(false))
	if err != nil {
		t.Fatalf("lp_pre_min presolve=false err: %v", err)
	}
	if resOn.Status != StatusOptimal || resOff.Status != StatusOptimal {
		t.Fatalf("lp_pre_min status=(%v, %v), want Optimal", resOn.Status, resOff.Status)
	}
	if math.Abs(resOn.Objective-resOff.Objective) > 1e-6 {
		t.Fatalf("lp_pre_min obj=%g vs no-presolve %g", resOn.Objective, resOff.Objective)
	}
	assertDualKKT(t, "lp_pre_min", pMin, resOn)

	// 2. Maximize LP with chained fixed-var substitution + singleton dual unwinding.
	mMax := NewModel("lp_pre_max", Maximize)
	y1 := mMax.AddContVar("y1", 0, 10)
	y2 := mMax.AddContVar("y2", 0, 10)
	y3 := mMax.AddContVar("y3", 0, 10)
	mMax.SetObjective(NewExpr(y1.T(6), y2.T(5), y3.T(3)))
	mMax.AddConstraint("fix_y1", NewExpr(y1.T(2)).Eq(4))
	mMax.AddConstraint("c_y2", NewExpr(y1.T(1), y2.T(1), y3.T(1)).Le(7))
	mMax.AddConstraint("c_y3", NewExpr(y3.T(-1)).Ge(-3))

	pMax := mMax.Compile()
	resMaxOn, err := SolveCompiled(ctx, pMax, WithPresolve(true))
	if err != nil {
		t.Fatalf("lp_pre_max presolve=true err: %v", err)
	}
	resMaxOff, err := SolveCompiled(ctx, pMax, WithPresolve(false))
	if err != nil {
		t.Fatalf("lp_pre_max presolve=false err: %v", err)
	}
	if resMaxOn.Status != StatusOptimal || math.Abs(resMaxOn.Objective-resMaxOff.Objective) > 1e-6 {
		t.Fatalf("lp_pre_max status=%v obj=%g vs %g", resMaxOn.Status, resMaxOn.Objective, resMaxOff.Objective)
	}
	assertDualKKT(t, "lp_pre_max", pMax, resMaxOn)

	// 3. Infeasible and Unbounded parity under LP presolve.
	mInf := NewModel("lp_pre_infeas", Minimize)
	xi := mInf.AddContVar("x", 0, 5)
	mInf.SetObjective(VarExpr(xi))
	mInf.AddConstraint("c1", NewExpr(xi.T(1)).Ge(7))
	resInf, err := mInf.Solve(ctx, WithPresolve(true))
	if err != nil || resInf.Status != StatusInfeasible {
		t.Fatalf("lp_pre_infeas status=%v err=%v, want Infeasible", resInf.Status, err)
	}

	mUnb := NewModel("lp_pre_unb", Minimize)
	xu := mUnb.AddContVar("xu", 0, math.Inf(1))
	xf := mUnb.AddContVar("xf", 2, 2)
	mUnb.SetObjective(NewExpr(xu.T(-1), xf.T(3)))
	mUnb.AddConstraint("c1", NewExpr(xu.T(1), xf.T(1)).Ge(2))
	resUnb, err := mUnb.Solve(ctx, WithPresolve(true))
	if err != nil || resUnb.Status != StatusUnbounded {
		t.Fatalf("lp_pre_unb status=%v err=%v, want Unbounded", resUnb.Status, err)
	}

	// 4. All 14 public Netlib LPs: objective and KKT parity with Presolve enabled vs disabled.
	rawManifest, err := os.ReadFile(filepath.Join("testdata", "public", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest []struct {
		ID                 string  `json:"id"`
		IsMIP              bool    `json:"is_mip"`
		ReferenceObjective float64 `json:"reference_objective"`
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	for _, inst := range manifest {
		if inst.IsMIP {
			continue
		}
		t.Run(inst.ID, func(t *testing.T) {
			t.Parallel()
			p := loadJSONModelForTest(t, filepath.Join("testdata", "public", inst.ID+".json")).Compile()
			resPre, err := SolveCompiled(ctx, p, WithPresolve(true))
			if err != nil {
				t.Fatalf("%s presolve=true err: %v", inst.ID, err)
			}
			if resPre.Status != StatusOptimal {
				t.Fatalf("%s status=%v, want Optimal", inst.ID, resPre.Status)
			}
			rel := math.Abs(resPre.Objective-inst.ReferenceObjective) / max(1.0, math.Abs(inst.ReferenceObjective))
			if rel > 1e-5 {
				t.Fatalf("%s obj=%.10g, want %.10g (rel=%.2e)", inst.ID, resPre.Objective, inst.ReferenceObjective, rel)
			}
			assertDualKKT(t, inst.ID, p, resPre)
		})
	}
}

func TestDynamicOrbitalBranchingIdenticalItems(t *testing.T) {
	ctx := context.Background()

	// 1. Identical binary items in a parity-infeasible knapsack (2*sum x_i = 9).
	// Without cuts or GCD presolve, LP relaxation is feasible (x_i = 0.375) and an
	// unbroken B&B tree explores hundreds of subsets; Tier A orbital branching closes in linear nodes.
	mInf := NewModel("orbital_identical_infeas", Maximize)
	const nItems = 12
	varsInf := make([]Var, nItems)
	termsInf := make([]Term, nItems)
	objInf := make([]Term, nItems)
	for i := range nItems {
		varsInf[i] = mInf.AddBinaryVar(fmt.Sprintf("x_%d", i))
		termsInf[i] = varsInf[i].T(2)
		objInf[i] = varsInf[i].T(1)
	}
	mInf.SetObjective(NewExpr(objInf...))
	mInf.AddConstraint("parity", NewExpr(termsInf...).Eq(9))

	resInf, err := mInf.Solve(ctx, WithPresolve(false), WithCuts(false), WithHeuristics(false))
	if err != nil {
		t.Fatalf("parity infeas solve err: %v", err)
	}
	if resInf.Status != StatusInfeasible {
		t.Fatalf("parity infeas status=%v, want Infeasible", resInf.Status)
	}
	if resInf.Nodes > 30 {
		t.Fatalf("expected orbital branching to prove infeasibility quickly, got %d nodes", resInf.Nodes)
	}

	// 2. Identical binary items optimization (sum 3*x_i <= 23, max sum 5*x_i -> opt=35).
	mOpt := NewModel("orbital_identical_opt", Maximize)
	const nOpt = 16
	termsOpt := make([]Term, nOpt)
	objOpt := make([]Term, nOpt)
	for i := range nOpt {
		v := mOpt.AddBinaryVar(fmt.Sprintf("y_%d", i))
		termsOpt[i] = v.T(3)
		objOpt[i] = v.T(5)
	}
	mOpt.SetObjective(NewExpr(objOpt...))
	mOpt.AddConstraint("cap", NewExpr(termsOpt...).Le(23))

	for _, usePre := range []bool{false, true} {
		res, err := mOpt.Solve(ctx, WithPresolve(usePre), WithCuts(false), WithHeuristics(false))
		if err != nil || res.Status != StatusOptimal {
			t.Fatalf("presolve=%v status=%v err=%v, want Optimal", usePre, res.Status, err)
		}
		if math.Abs(res.Objective-35.0) > 1e-6 {
			t.Fatalf("presolve=%v obj=%.8g, want 35", usePre, res.Objective)
		}
		if res.Nodes > 35 {
			t.Fatalf("presolve=%v expected <= 35 nodes with orbital branching, got %d", usePre, res.Nodes)
		}
	}
}

func TestDynamicOrbitalBranchingEmergentSymmetry(t *testing.T) {
	ctx := context.Background()

	// Root asymmetry via binary trigger z in an LP-feasible K_5 4-coloring model:
	//   - 5 vertices v in 0..4, 4 colors c in 0..3: sum_c x_{v,c} = 1 (root LP x=0.25)
	//   - Conflict rows: x_{u,c} + x_{v,c} <= 1, except (u,v,c)=(0,1,0) adds + z <= 1
	//   - Trigger row: 2*z + aux = 1, objective Maximize(z) => root LP sets z = 0.5 (obj = 0.5)
	// At root, static color symmetry is broken by z; once z = 0 is fixed in-tree (depth >= 1),
	// Tier C detectDynamicNodeOrbit dynamically discovers all 4 symmetric colors.
	m := NewModel("orbital_emergent_k5_4col", Maximize)
	const nVerts = 5
	const nColors = 4
	z := m.AddBinaryVar("z")
	aux := m.AddBinaryVar("aux")
	x := make([][]Var, nVerts)
	for v := range nVerts {
		x[v] = make([]Var, nColors)
		pTerms := make([]Term, nColors)
		for c := range nColors {
			x[v][c] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", v, c))
			pTerms[c] = x[v][c].T(1)
		}
		m.AddConstraint(fmt.Sprintf("part_%d", v), NewExpr(pTerms...).Eq(1))
	}
	for c := range nColors {
		for u := range nVerts {
			for v := u + 1; v < nVerts; v++ {
				terms := []Term{x[u][c].T(1), x[v][c].T(1)}
				if u == 0 && v == 1 && c == 0 {
					terms = append(terms, z.T(1))
				}
				m.AddConstraint(fmt.Sprintf("edge_%d_%d_%d", u, v, c), NewExpr(terms...).Le(1))
			}
		}
	}
	m.AddConstraint("asym_break", NewExpr(z.T(2), aux.T(1)).Eq(1))
	m.SetObjective(NewExpr(z.T(1)))

	p := m.Compile()
	colZ := z.Index()
	colX00 := x[0][0].Index()

	ns := initNodeSymmetry(p)
	if rootOrb := ns.activeOrbit(p, p.varLB, p.varUB, colX00, 0, time.Time{}, nil, nil); len(rootOrb) >= 2 {
		t.Fatalf("expected no root orbit for colX00 before z is fixed, got %v (symG=%d)", rootOrb, ns.symG)
	}
	curLB := slices.Clone(p.varLB)
	curUB := slices.Clone(p.varUB)
	curUB[colZ] = 0
	dynOrb := ns.activeOrbit(p, curLB, curUB, colX00, 1, time.Time{}, nil, nil)
	if len(dynOrb) != nColors {
		t.Fatalf("expected Tier C dynamic orbit size %d after fixing z=0, got %d (%v)", nColors, len(dynOrb), dynOrb)
	}

	// Verify both with presolve disabled (pure B&B + dynamic orbital branching) and default solver.
	resNoPre, err := SolveCompiled(ctx, p, WithPresolve(false), WithCuts(false), WithHeuristics(false))
	if err != nil {
		t.Fatalf("emergent symmetry no-presolve err: %v", err)
	}
	if resNoPre.Status != StatusInfeasible {
		t.Fatalf("emergent symmetry status=%v, want Infeasible", resNoPre.Status)
	}
	if resNoPre.Nodes <= 1 || resNoPre.Nodes > 150 {
		t.Fatalf("emergent symmetry nodes=%d, want in [2, 150]", resNoPre.Nodes)
	}

	resDef, err := SolveCompiled(ctx, p)
	if err != nil {
		t.Fatalf("emergent symmetry default err: %v", err)
	}
	if resDef.Status != StatusInfeasible {
		t.Fatalf("emergent symmetry default status=%v, want Infeasible", resDef.Status)
	}
}

func TestDynamicOrbitalBranchingAsymmetricControl(t *testing.T) {
	ctx := context.Background()

	// 1. Tiny objective perturbations: 5.0 + i*1e-5 over 16 items, cap 3*sum <= 23 -> max 7 items (9..15).
	mObj := NewModel("orbital_asym_obj", Maximize)
	const n = 16
	capTerms := make([]Term, n)
	objTerms := make([]Term, n)
	for i := range n {
		v := mObj.AddBinaryVar(fmt.Sprintf("x_%d", i))
		capTerms[i] = v.T(3)
		objTerms[i] = v.T(5.0 + float64(i)*1e-5)
	}
	mObj.SetObjective(NewExpr(objTerms...))
	mObj.AddConstraint("cap", NewExpr(capTerms...).Le(23))

	resObj, err := mObj.Solve(ctx, WithCuts(false), WithHeuristics(false))
	if err != nil || resObj.Status != StatusOptimal {
		t.Fatalf("asym obj status=%v err=%v", resObj.Status, err)
	}
	wantObj := 35.0 + 1e-5*(9+10+11+12+13+14+15)
	if math.Abs(resObj.Objective-wantObj) > 1e-8 {
		t.Fatalf("asym obj=%.10f, want %.10f (false symmetry breaking lost highest-obj items)", resObj.Objective, wantObj)
	}

	// 2. Tiny constraint perturbations: w_i = 3.0 - i*1e-5, cap = 20.9995, equal obj=5.
	// Items 9..15 have weight sum 21 - 1e-5*84 = 20.99916 <= 20.9995 (7 items -> obj=35).
	// Low-index items 0..6 have weight sum 21 - 1e-5*21 = 20.99979 > 20.9995 (at most 6 items -> obj=30).
	mCon := NewModel("orbital_asym_con", Maximize)
	capTerms2 := make([]Term, n)
	objTerms2 := make([]Term, n)
	for i := range n {
		v := mCon.AddBinaryVar(fmt.Sprintf("w_%d", i))
		capTerms2[i] = v.T(3.0 - float64(i)*1e-5)
		objTerms2[i] = v.T(5.0)
	}
	mCon.SetObjective(NewExpr(objTerms2...))
	mCon.AddConstraint("cap", NewExpr(capTerms2...).Le(20.9995))

	resCon, err := mCon.Solve(ctx, WithPresolve(false), WithCuts(false), WithHeuristics(false))
	if err != nil || resCon.Status != StatusOptimal {
		t.Fatalf("asym con status=%v err=%v", resCon.Status, err)
	}
	if math.Abs(resCon.Objective-35.0) > 1e-6 {
		t.Fatalf("asym con obj=%.10f, want 35.0 (false orbital pruning excluded feasible high-index items)", resCon.Objective)
	}
}
