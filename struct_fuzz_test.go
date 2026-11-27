package goblin_test

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

type structByteCursor struct {
	data []byte
	pos  int
}

func (c *structByteCursor) u8() uint8 {
	if c.pos >= len(c.data) {
		return 0
	}
	b := c.data[c.pos]
	c.pos++
	return b
}

func (c *structByteCursor) pick(n int) int {
	if n <= 1 {
		return 0
	}
	return int(c.u8()) % n
}

func (c *structByteCursor) boolBit() bool {
	return (c.u8() & 1) == 1
}

type structVarSpec struct {
	name string
	lb   float64
	ub   float64
	cat  gb.VarCategory
	as01 bool // true when declared as AddIntVar(..., 0, 1) instead of AddBinaryVar
}

type structRowSpec struct {
	name      string
	sense     gb.ConstraintSense
	rhs       float64
	exprConst float64
	cols      []int
	vals      []float64
}

type structModelSpec struct {
	name     string
	sense    gb.Sense
	objConst float64
	vars     []structVarSpec
	obj      []float64
	rows     []structRowSpec
}

func compileStructModelSpec(spec *structModelSpec, forceBinaryFor01 bool) (*gb.Model, []gb.Var) {
	m := gb.NewModel(spec.name, spec.sense)
	xs := make([]gb.Var, len(spec.vars))
	for j, v := range spec.vars {
		vname := v.name
		if vname == "" {
			vname = "x" + strconv.Itoa(j)
		}
		switch {
		case v.as01 && forceBinaryFor01:
			xs[j] = m.AddBinaryVar(vname)
		case v.cat == gb.Binary:
			xs[j] = m.AddBinaryVar(vname)
		case v.cat == gb.Integer:
			xs[j] = m.AddIntVar(vname, v.lb, v.ub)
		default:
			xs[j] = m.AddContVar(vname, v.lb, v.ub)
		}
	}
	obj := gb.NewExpr().AddConstant(spec.objConst)
	for j, cj := range spec.obj {
		if cj != 0 {
			obj = obj.AddTerm(xs[j], cj)
		}
	}
	m.SetObjective(obj)
	for i, r := range spec.rows {
		e := gb.NewExpr().AddConstant(r.exprConst)
		for k, col := range r.cols {
			val := r.vals[k]
			if val != 0 && col >= 0 && col < len(xs) {
				e = e.AddTerm(xs[col], val)
			}
		}
		rname := r.name
		if rname == "" {
			rname = "r" + strconv.Itoa(i)
		}
		switch r.sense {
		case gb.LessEqual:
			m.AddConstraint(rname, gb.Le(e, r.rhs))
		case gb.GreaterEqual:
			m.AddConstraint(rname, gb.Ge(e, r.rhs))
		default:
			m.AddConstraint(rname, gb.Eq(e, r.rhs))
		}
	}
	return m, xs
}

func decodeBoundaryCoeffForProfile(c *structByteCursor, profile int, allowZero bool) float64 {
	var table []float64
	switch profile {
	case 0:
		// Micro-tolerance standard: <= 5 magnitudes paired with 1e-6..1e-5 boundary coeffs.
		table = []float64{
			0, 1, -1, 2, -2, 3, -3, 5, -5,
			0.5, -0.5, 0.25, -0.25, 0.1, -0.1, 0.2, -0.2,
			1e-6, -1e-6, 5e-6, -5e-6, 1e-5, -1e-5,
		}
	case 1:
		// Macro-scale standard: 0.1..1e7 magnitudes keeping intra-row ratios <= 1e8.
		table = []float64{
			0, 1, -1, 2, -2, 3, -3, 5, -5,
			0.5, -0.5, 0.25, -0.25, 0.1, -0.1, 0.2, -0.2,
			1e4, -1e4, 1e6, -1e6, 1e7, -1e7,
		}
	default:
		// Extreme ill-conditioned: 1e-8..1e14 scale span outside standard float64 conditioning.
		table = []float64{
			0, 1, -1, 1e-8, -1e-8, 5e-6, -5e-6,
			5e11, -5e11, 1e12, -1e12, 1e14, -1e14,
		}
	}
	idx := c.pick(len(table))
	if !allowZero && idx == 0 {
		idx = 1
	}
	return table[idx]
}

func isOutOfStandardVal(v float64, allowInf bool) bool {
	if math.IsInf(v, 0) {
		return !allowInf
	}
	if v == 0 {
		return false
	}
	av := math.Abs(v)
	return av > 1e12 || av < 1e-6
}

func ensureExtremeOutOfStandardScale(c *structByteCursor, spec *structModelSpec) {
	for _, v := range spec.vars {
		if (!math.IsInf(v.lb, -1) && isOutOfStandardVal(v.lb, true)) ||
			(!math.IsInf(v.ub, 1) && isOutOfStandardVal(v.ub, true)) {
			return
		}
	}
	for _, cj := range spec.obj {
		if isOutOfStandardVal(cj, false) {
			return
		}
	}
	for _, r := range spec.rows {
		if isOutOfStandardVal(r.rhs, false) {
			return
		}
		for _, val := range r.vals {
			if isOutOfStandardVal(val, false) {
				return
			}
		}
	}
	extVal := 1e14
	if c.boolBit() {
		extVal = 1e-8
	}
	switch c.pick(3) {
	case 0:
		if len(spec.rows) > 0 && len(spec.rows[0].vals) > 0 {
			spec.rows[0].vals[0] = extVal
			return
		}
		spec.obj[0] = extVal
	case 1:
		spec.obj[0] = -extVal
	default:
		switch spec.vars[0].cat {
		case gb.Continuous:
			spec.vars[0].lb, spec.vars[0].ub = 0, 1e20
		case gb.Integer:
			spec.vars[0].lb, spec.vars[0].ub = 1e15, 1e15+100
			spec.vars[0].as01 = false
		default:
			spec.obj[0] = extVal
		}
	}
}

func decodeBoundaryObjConst(c *structByteCursor) float64 {
	consts := []float64{0, 1.5, -2.5, 4.3, -0.1, 1e6, -1e6, 1e9, -1e9, 1e12, -1e12}
	return consts[c.pick(len(consts))]
}

func decodeVarRegime(c *structByteCursor, j int) structVarSpec {
	name := "x" + strconv.Itoa(j)
	catMode := c.pick(4)
	if catMode == 2 {
		return structVarSpec{name: name, lb: 0, ub: 1, cat: gb.Binary}
	}
	if catMode == 3 {
		return structVarSpec{name: name, lb: 0, ub: 1, cat: gb.Integer, as01: true}
	}
	cat := gb.Continuous
	if catMode == 1 {
		cat = gb.Integer
	}

	regByte := c.u8()
	regime := regByte % 11
	var lb, ub float64
	switch regime {
	case 0:
		lb = 0
		ub = float64(1 + c.pick(12))
	case 1:
		// Negative integer/continuous domain [-50, -3].
		lb = -50
		ub = -3
		if c.boolBit() {
			lb = float64(-50 + c.pick(20))
			ub = float64(-3 - c.pick(10))
			if lb > ub {
				lb, ub = -50, -3
			}
		}
	case 2:
		// Semi-infinite upper-bounded [-Inf, 5].
		lb = math.Inf(-1)
		ub = 5
		if c.boolBit() {
			ub = float64(5 + c.pick(6))
		}
	case 3:
		// Semi-infinite lower-bounded [lo, +Inf].
		lb = float64(c.pick(4))
		ub = math.Inf(1)
	case 4:
		// Fully free variable [-Inf, +Inf].
		lb = math.Inf(-1)
		ub = math.Inf(1)
	case 5:
		// Fractional bounds on integer/continuous variables (4.5, [-2.3, 4.5], [1.2, 1.8]).
		switch c.pick(3) {
		case 0:
			lb, ub = 0.5, 4.5
		case 1:
			lb, ub = -2.3, 4.5
		default:
			lb, ub = 1.2, 1.8
		}
	case 6:
		// Bounds near the 53-bit float step limit: [1e15, 1e15+100] and [2^53+2, 2^53+102].
		if c.boolBit() {
			lb = 1e15
			ub = 1e15 + 100
		} else {
			lb = math.Ldexp(1.0, 53) + 2
			ub = math.Ldexp(1.0, 53) + 102
		}
	case 7:
		// Wide >2^53 domain [0, 1e20].
		lb = 0
		ub = 1e20
	case 8:
		// Fixed variable [v, v].
		fixedVals := []float64{0, -4, 3, 2.5, 1e15}
		v := fixedVals[c.pick(len(fixedVals))]
		if cat == gb.Integer && v == 2.5 {
			v = 2
		}
		lb, ub = v, v
	case 9:
		// Symmetric integer/continuous box [-k1, k2].
		lb = -float64(1 + c.pick(8))
		ub = float64(1 + c.pick(8))
	default:
		if regByte&0x80 != 0 {
			// Explicit inverted bound conflict.
			lb, ub = 5, 2
		} else {
			lb, ub = 0, float64(2+c.pick(6))
		}
	}
	return structVarSpec{name: name, lb: lb, ub: ub, cat: cat}
}

func buildRareExitSpec(c *structByteCursor, sub int) structModelSpec {
	sense := gb.Minimize
	if c.boolBit() {
		sense = gb.Maximize
	}
	objC := decodeBoundaryObjConst(c)

	switch sub {
	case 0:
		// All variables fixed (feasible or infeasible depending on RHS).
		n := 1 + c.pick(4)
		vars := make([]structVarSpec, n)
		obj := make([]float64, n)
		cols := make([]int, n)
		vals := make([]float64, n)
		act := 0.0
		for j := range n {
			v := float64(c.pick(7) - 3)
			cat := gb.Continuous
			if j%2 == 0 {
				cat = gb.Integer
			}
			vars[j] = structVarSpec{name: "fx" + strconv.Itoa(j), lb: v, ub: v, cat: cat}
			obj[j] = float64(j + 1)
			cols[j] = j
			vals[j] = float64((j%3)+1) * 0.5
			act += vals[j] * v
		}
		rhs := act
		if c.boolBit() {
			rhs = act - 2.0
		}
		return structModelSpec{
			name:     "rare_all_fixed",
			sense:    sense,
			objConst: objC,
			vars:     vars,
			obj:      obj,
			rows: []structRowSpec{
				{name: "r0", sense: gb.LessEqual, rhs: rhs, cols: cols, vals: vals},
			},
		}

	case 1:
		// Single free variable: 0 rows, singleton eq, or unbounded half-line.
		objVal := 0.0
		if c.boolBit() {
			objVal = float64(1 + c.pick(5))
			if c.boolBit() {
				objVal = -objVal
			}
		}
		spec := structModelSpec{
			name:     "rare_single_free",
			sense:    sense,
			objConst: objC,
			vars:     []structVarSpec{{name: "xf", lb: math.Inf(-1), ub: math.Inf(1), cat: gb.Continuous}},
			obj:      []float64{objVal},
		}
		switch c.pick(3) {
		case 1:
			spec.rows = []structRowSpec{{name: "eq", sense: gb.Equal, rhs: 7, cols: []int{0}, vals: []float64{3}}}
		case 2:
			spec.rows = []structRowSpec{{name: "le", sense: gb.LessEqual, rhs: 5, cols: []int{0}, vals: []float64{2}}}
		}
		return spec

	case 2:
		// Empty and singleton rows testing empty bounds and integer rounding.
		xCat := gb.Integer
		if c.boolBit() {
			xCat = gb.Continuous
		}
		rSense := [3]gb.ConstraintSense{gb.LessEqual, gb.GreaterEqual, gb.Equal}[c.pick(3)]
		emptyRHS := []float64{0, 2.5, -2.5, 1e-5, -1e-5}[c.pick(5)]
		rows := []structRowSpec{
			{name: "empty", sense: rSense, rhs: emptyRHS},
		}
		if c.boolBit() {
			rows = append(rows, structRowSpec{
				name:  "sing",
				sense: gb.LessEqual,
				rhs:   7,
				cols:  []int{0},
				vals:  []float64{2},
			})
		}
		return structModelSpec{
			name:     "rare_empty_singleton",
			sense:    sense,
			objConst: objC,
			vars:     []structVarSpec{{name: "x", lb: 0, ub: 10, cat: xCat}},
			obj:      []float64{3},
			rows:     rows,
		}

	case 3:
		// Slack-only optimal basis: structural vars stay nonbasic at 0, slacks carry basis.
		n := 3 + c.pick(4)
		mRows := 2 + c.pick(3)
		vars := make([]structVarSpec, n)
		obj := make([]float64, n)
		cSign := 1.0
		if sense == gb.Maximize {
			cSign = -1.0
		}
		for j := range n {
			vars[j] = structVarSpec{name: "s" + strconv.Itoa(j), lb: 0, ub: 10, cat: gb.Continuous}
			obj[j] = cSign * float64(1+c.pick(7))
		}
		rows := make([]structRowSpec, mRows)
		for i := range mRows {
			cols := make([]int, n)
			vals := make([]float64, n)
			for j := range n {
				cols[j] = j
				vals[j] = float64(1 + (i+j)%4)
			}
			rows[i] = structRowSpec{
				name:  "cap" + strconv.Itoa(i),
				sense: gb.LessEqual,
				rhs:   float64(2 + i*3 + c.pick(5)),
				cols:  cols,
				vals:  vals,
			}
		}
		return structModelSpec{name: "rare_slack_basis", sense: sense, objConst: objC, vars: vars, obj: obj, rows: rows}

	case 4:
		// Root closed by cuts alone (2-var Chvatal/GMI or 4-var lifted knapsack cover).
		if c.boolBit() {
			return structModelSpec{
				name:     "rare_root_cut_closed_2v",
				sense:    gb.Maximize,
				objConst: objC,
				vars: []structVarSpec{
					{name: "b0", lb: 0, ub: 1, cat: gb.Binary},
					{name: "b1", lb: 0, ub: 1, cat: gb.Binary},
				},
				obj: []float64{1, 1},
				rows: []structRowSpec{
					{name: "chv", sense: gb.LessEqual, rhs: 3, cols: []int{0, 1}, vals: []float64{2, 2}},
				},
			}
		}
		return structModelSpec{
			name:     "rare_root_cut_closed_cov",
			sense:    gb.Maximize,
			objConst: objC,
			vars: []structVarSpec{
				{name: "b0", lb: 0, ub: 1, cat: gb.Binary},
				{name: "b1", lb: 0, ub: 1, cat: gb.Binary},
				{name: "b2", lb: 0, ub: 1, cat: gb.Binary},
				{name: "b3", lb: 0, ub: 1, cat: gb.Binary},
			},
			obj: []float64{1, 1, 1, 1},
			rows: []structRowSpec{
				{name: "cov", sense: gb.LessEqual, rhs: 7, cols: []int{0, 1, 2, 3}, vals: []float64{3, 3, 4, 4}},
			},
		}

	case 5:
		// LP-unbounded + MIP-infeasible Diophantine cylinder.
		cSign := -1.0
		if sense == gb.Maximize {
			cSign = 1.0
		}
		rows := []structRowSpec{
			{name: "dio", sense: gb.Equal, rhs: 1, cols: []int{0, 1}, vals: []float64{2, -2}},
		}
		if c.boolBit() {
			rows = []structRowSpec{
				{name: "ge", sense: gb.GreaterEqual, rhs: 0.5, cols: []int{0, 1}, vals: []float64{2, -2}},
				{name: "le", sense: gb.LessEqual, rhs: 1.5, cols: []int{0, 1}, vals: []float64{2, -2}},
			}
		}
		return structModelSpec{
			name:     "rare_lp_unb_mip_infeas",
			sense:    sense,
			objConst: objC,
			vars: []structVarSpec{
				{name: "ix", lb: 0, ub: math.Inf(1), cat: gb.Integer},
				{name: "iy", lb: 0, ub: math.Inf(1), cat: gb.Integer},
			},
			obj:  []float64{cSign, 0},
			rows: rows,
		}

	case 6:
		// Presolve-time infeasibles (GCD parity or inverted rounded integer bounds).
		if c.boolBit() {
			return structModelSpec{
				name:     "rare_infeas_presolve_gcd",
				sense:    sense,
				objConst: objC,
				vars: []structVarSpec{
					{name: "x0", lb: 0, ub: 5, cat: gb.Integer},
					{name: "x1", lb: 0, ub: 5, cat: gb.Integer},
				},
				obj: []float64{1, 2},
				rows: []structRowSpec{
					{name: "parity", sense: gb.Equal, rhs: 3, cols: []int{0, 1}, vals: []float64{2, 4}},
				},
			}
		}
		return structModelSpec{
			name:     "rare_infeas_presolve_frac",
			sense:    sense,
			objConst: objC,
			vars:     []structVarSpec{{name: "x", lb: 1.2, ub: 1.8, cat: gb.Integer}},
			obj:      []float64{1},
		}

	case 7:
		// Models bypassing presolve bound passes and failing at the root LP.
		return structModelSpec{
			name:     "rare_infeas_root_lp",
			sense:    sense,
			objConst: objC,
			vars: []structVarSpec{
				{name: "x0", lb: 0, ub: 2, cat: gb.Continuous},
				{name: "x1", lb: 0, ub: 2, cat: gb.Continuous},
				{name: "x2", lb: 0, ub: 2, cat: gb.Continuous},
			},
			obj: []float64{1, 1, 1},
			rows: []structRowSpec{
				{name: "cap", sense: gb.LessEqual, rhs: 4, cols: []int{0, 1, 2}, vals: []float64{1, 1, 1}},
				{name: "p01", sense: gb.GreaterEqual, rhs: 3, cols: []int{0, 1}, vals: []float64{1, 1}},
				{name: "p12", sense: gb.GreaterEqual, rhs: 3, cols: []int{1, 2}, vals: []float64{1, 1}},
				{name: "p02", sense: gb.GreaterEqual, rhs: 3, cols: []int{0, 2}, vals: []float64{1, 1}},
			},
		}

	case 8:
		// Tree-proved infeasible models surviving presolve and root LP.
		if c.boolBit() {
			// 2*(x0 + x1 + x2) + c == 3 with c in [0.1, 0.9] solves to LP (1, 0.3, 0, 0.4)
			// while no integer point fits 2*Z in [2.1, 2.9].
			return structModelSpec{
				name:     "rare_infeas_tree_parity",
				sense:    sense,
				objConst: objC,
				vars: []structVarSpec{
					{name: "x0", lb: 0, ub: 1, cat: gb.Binary},
					{name: "x1", lb: 0, ub: 1, cat: gb.Binary},
					{name: "x2", lb: 0, ub: 1, cat: gb.Binary},
					{name: "c", lb: 0.1, ub: 0.9, cat: gb.Continuous},
				},
				obj: []float64{1, 2, 3, 1},
				rows: []structRowSpec{
					{name: "eq", sense: gb.Equal, rhs: 3, cols: []int{0, 1, 2, 3}, vals: []float64{2, 2, 2, 1}},
				},
			}
		}
		// K4 3-coloring: root LP x_{v,color}=1/3 is feasible; tree proves unsatisfiable.
		const nVerts = 4
		const nColors = 3
		vars := make([]structVarSpec, 0, nVerts*nColors)
		obj := make([]float64, nVerts*nColors)
		for v := range nVerts {
			for col := range nColors {
				vars = append(vars, structVarSpec{
					name: fmt.Sprintf("x%d_%d", v, col),
					lb:   0,
					ub:   1,
					cat:  gb.Binary,
				})
				obj[v*nColors+col] = float64(col + 1)
			}
		}
		var rows []structRowSpec
		for v := range nVerts {
			cols := []int{v * nColors, v*nColors + 1, v*nColors + 2}
			rows = append(rows, structRowSpec{
				name:  fmt.Sprintf("assign_%d", v),
				sense: gb.Equal,
				rhs:   1,
				cols:  cols,
				vals:  []float64{1, 1, 1},
			})
		}
		for u := range nVerts {
			for v := u + 1; v < nVerts; v++ {
				for col := range nColors {
					rows = append(rows, structRowSpec{
						name:  fmt.Sprintf("edge_%d_%d_%d", u, v, col),
						sense: gb.LessEqual,
						rhs:   1,
						cols:  []int{u*nColors + col, v*nColors + col},
						vals:  []float64{1, 1},
					})
				}
			}
		}
		return structModelSpec{name: "rare_infeas_tree_k4_3col", sense: sense, objConst: objC, vars: vars, obj: obj, rows: rows}

	default:
		// Int[0,1] vs Binary check + >2^53 bounds + edge coefficients (1e11..1e12, 1e-6..1e-5).
		return structModelSpec{
			name:     "rare_int01_big_bounds",
			sense:    sense,
			objConst: objC,
			vars: []structVarSpec{
				{name: "b01", lb: 0, ub: 1, cat: gb.Integer, as01: true},
				{name: "xbig", lb: 1e15, ub: 1e15 + 100, cat: gb.Integer},
				{name: "xwide", lb: 0, ub: 1e20, cat: gb.Continuous},
			},
			obj: []float64{3, 1e-5, 1e-6},
			rows: []structRowSpec{
				{
					name:  "r0",
					sense: gb.GreaterEqual,
					rhs:   1e11,
					cols:  []int{0, 1, 2},
					vals:  []float64{1e11, 1e-5, 1e-6},
				},
				{
					name:  "r1",
					sense: gb.LessEqual,
					rhs:   1e15 + 50,
					cols:  []int{1, 0},
					vals:  []float64{1, 20},
				},
			},
		}
	}
}

func decodeGeneralStructSpec(c *structByteCursor) structModelSpec {
	profile := c.pick(3)
	sense := gb.Minimize
	if c.boolBit() {
		sense = gb.Maximize
	}
	nVars := 1 + c.pick(12)
	nRows := c.pick(13)
	objConst := decodeBoundaryObjConst(c)

	vars := make([]structVarSpec, nVars)
	obj := make([]float64, nVars)
	for j := range nVars {
		vars[j] = decodeVarRegime(c, j)
		obj[j] = decodeBoundaryCoeffForProfile(c, profile, true)
	}

	rows := make([]structRowSpec, nRows)
	for i := range nRows {
		rSense := [3]gb.ConstraintSense{gb.LessEqual, gb.GreaterEqual, gb.Equal}[c.pick(3)]
		nnz := c.pick(min(nVars, 6) + 1)
		cols := make([]int, 0, nnz)
		vals := make([]float64, 0, nnz)
		used := make([]bool, nVars)
		for range nnz {
			col := c.pick(nVars)
			if used[col] {
				continue
			}
			used[col] = true
			cols = append(cols, col)
			vals = append(vals, decodeBoundaryCoeffForProfile(c, profile, false))
		}
		exprConst := 0.0
		if c.u8()&3 == 3 {
			exprConst = []float64{0.1, -0.5, 1.0}[c.pick(3)]
		}
		rhs := decodeBoundaryCoeffForProfile(c, profile, true)
		if c.boolBit() && len(cols) > 0 {
			// Anchor RHS near a feasible/boundary sample point so non-trivial bases occur frequently.
			est := exprConst
			for k, col := range cols {
				lo, hi := vars[col].lb, vars[col].ub
				pt := 0.0
				switch {
				case !math.IsInf(lo, -1) && !math.IsInf(hi, 1):
					pt = 0.5 * (lo + hi)
				case !math.IsInf(lo, -1):
					pt = lo + 1.0
				case !math.IsInf(hi, 1):
					pt = hi - 1.0
				}
				if prod := vals[k] * pt; !math.IsInf(prod, 0) && !math.IsNaN(prod) && math.Abs(est+prod) < 1e25 {
					est += prod
				}
			}
			rhs = est
		}
		rows[i] = structRowSpec{
			name:      "r" + strconv.Itoa(i),
			sense:     rSense,
			rhs:       rhs,
			exprConst: exprConst,
			cols:      cols,
			vals:      vals,
		}
	}
	spec := structModelSpec{
		name:     "struct_gen",
		sense:    sense,
		objConst: objConst,
		vars:     vars,
		obj:      obj,
		rows:     rows,
	}
	if profile == 2 {
		ensureExtremeOutOfStandardScale(c, &spec)
	}
	return spec
}

// verifyCertifiedWitnessExactRat checks exact rational feasibility, integrality,
// objective value, and dual-bound ordering for certified witnesses when extreme
// coefficients induce sub-ULP float64 projection residuals above 1e-5.
func verifyCertifiedWitnessExactRat(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result, wit *gb.CertifiedWitness) {
	t.Helper()
	if res.Status != gb.StatusOptimal {
		return
	}
	if len(wit.Values) != p.NumVars() || wit.Objective == nil || wit.BestBound == nil {
		t.Fatalf("[%s] missing exact CertifiedWitness payload for Optimal result", label)
	}
	if !isExactRatSliceMIPFeasible(p, wit.Values) {
		t.Fatalf("[%s] exact CertifiedWitness fails isExactRatSliceMIPFeasible", label)
	}
	if p.Sense() == gb.Minimize && wit.BestBound.Cmp(wit.Objective) > 0 {
		t.Fatalf("[%s] Minimize wit.BestBound %s > wit.Objective %s", label, wit.BestBound.RatString(), wit.Objective.RatString())
	}
	if p.Sense() == gb.Maximize && wit.BestBound.Cmp(wit.Objective) < 0 {
		t.Fatalf("[%s] Maximize wit.BestBound %s < wit.Objective %s", label, wit.BestBound.RatString(), wit.Objective.RatString())
	}
	if err := checkTightContPrimalResiduals(p, res.Values, 1e-5); err != nil {
		t.Fatalf("[%s] scaled primal residual check failed: %v", label, err)
	}
	_, _, iV := p.MaxViolation(res.Values, p.IsMIP())
	if iV > 1e-5 {
		t.Fatalf("[%s] Optimal integer violation %.2e > 1e-5", label, iV)
	}
}

// verifyCertifiedAndVIPRMIP cross-checks certified exact-Q solves against
// VIPR trace certificates, permitting fail-closed ErrCertificationFailed only
// when problem scaling falls outside standard bounds.
func verifyCertifiedAndVIPRMIP(t *testing.T, label string, p *gb.CompiledProblem, maxNodes int, timeLimit time.Duration) {
	t.Helper()
	// Non-certified SolveCompiled permits 1e-7 float slack, while
	// WithCertified(true) and WithVIPRTrace enforce exact rational Q.
	var wit gb.CertifiedWitness
	resCert, errCert := gb.SolveCompiled(t.Context(), p,
		gb.WithPureGo(),
		gb.WithPresolve(true),
		gb.WithCuts(true),
		gb.WithHeuristics(true),
		gb.WithWorkers(1),
		gb.WithRelGapTol(1e-6),
		gb.WithAbsGapTol(1e-6),
		gb.WithMaxNodes(maxNodes),
		gb.WithTimeLimit(timeLimit),
		gb.WithCertifiedWitness(&wit),
	)
	if errCert != nil && (isStandardScaledProblem(p) || !errors.Is(errCert, gb.ErrCertificationFailed)) {
		t.Fatalf("[%s/cert] WithCertified(true) failed: status=%v err=%v", label, resCert.Status, errCert)
	}
	if isStandardScaledProblem(p) || resCert.Status != gb.StatusOptimal {
		verifyMILPInvariants(t, label+"/cert", p, resCert)
	} else {
		verifyCertifiedWitnessExactRat(t, label+"/cert_wit", p, resCert, &wit)
	}
	if formalFuzzEnabled() {
		resVIPR, tr := verifyFormalVIPRTrace(t, label+"/vipr", p, maxNodes, timeLimit,
			gb.WithPresolve(false),
			gb.WithCuts(true),
			gb.WithHeuristics(true),
			gb.WithRelGapTol(1e-6),
			gb.WithAbsGapTol(1e-6),
		)
		if errCert == nil && isStandardScaledProblem(p) {
			assertFullFeatureVsVIPR(t, label+"/cert_vs_vipr", p, resCert, resVIPR, tr, 1e-6, 1e-6)
		} else if errCert == nil && resCert.Status == gb.StatusOptimal && tr != nil && tr.HasCertificate() && tr.TargetLB() != nil {
			certMinRat := new(big.Rat).Set(wit.Objective)
			if p.Sense() == gb.Maximize {
				certMinRat.Neg(certMinRat)
			}
			if certMinRat.Cmp(tr.TargetLB()) < 0 {
				t.Fatalf("[%s] certified exact minObj %s < VIPR TargetLB %s", label, certMinRat.RatString(), tr.TargetLB().RatString())
			}
		}
	}
}

func hasExactBoundConflict(p *gb.CompiledProblem) bool {
	for j := range p.NumVars() {
		lo, hi := p.VarBounds(j)
		if !math.IsInf(lo, -1) && !math.IsInf(hi, 1) && lo > hi {
			return true
		}
	}
	return false
}

func checkLPExactInfeasibleCert(t *testing.T, label string, p *gb.CompiledProblem, dualInf []float64, stA, stB gb.Status) {
	t.Helper()
	if hasExactBoundConflict(p) {
		return
	}
	if p.NumConstraints() == 0 {
		t.Fatalf("[%s] 0-constraint LP without bound conflict declared Infeasible (%v vs %v)", label, stA, stB)
	}
	if _, err := buildExactFarkasCertJSON(t, p, dualInf); err != nil {
		t.Fatalf("[%s] LP %v vs %v lacks exact Farkas certificate: %v", label, stA, stB, err)
	}
}

func verifyCompiledLPResult(t *testing.T, label string, p *gb.CompiledProblem, res gb.Result) {
	t.Helper()
	switch {
	case res.Status == gb.StatusOptimal:
		if err := checkLPPrimalAndDualKKT(label, p, res.Values, res.Duals, p.NumConstraints() > 0); err != nil {
			t.Fatal(err)
		}
		evalObj := p.EvaluateObjective(res.Values)
		if math.Abs(evalObj-res.Objective) > 1e-5*max(1.0, math.Abs(res.Objective)) {
			t.Fatalf("[%s] EvaluateObjective=%.8g != Objective=%.8g", label, evalObj, res.Objective)
		}
	case res.Status == gb.StatusNumericalError:
		if err := certifySubTolLPNumericalError(t, label, p, res.Iterations, len(res.Values)); err != nil {
			t.Fatal(err)
		}
	case isTerminalFailureStatus(res.Status):
		t.Fatalf("[%s] terminal failure status %v", label, res.Status)
	}
}

// assertLPStatusConsistent verifies status agreement between two LP solve paths,
// permitting StatusInfeasible divergence against StatusOptimal or
// StatusUnbounded only when exact-Q infeasibility holds and the opposing
// status lacks an exact-Q certificate.
func assertLPStatusConsistent(t *testing.T, label string, p *gb.CompiledProblem, stA, stB gb.Status, xA, xB, dualA, dualB []float64) {
	t.Helper()
	if stA == gb.StatusNumericalError || stB == gb.StatusNumericalError ||
		isResourceLimitStatus(stA) || isResourceLimitStatus(stB) || stA == stB {
		return
	}
	switch {
	case (stA == gb.StatusInfeasible && stB == gb.StatusOptimal) ||
		(stB == gb.StatusInfeasible && stA == gb.StatusOptimal):
		xOpt, dualInf := xB, dualA
		if stB == gb.StatusInfeasible {
			xOpt, dualInf = xA, dualB
		}
		if isExactRatMIPFeasible(p, xOpt) {
			t.Fatalf("[%s] exact-Q LP optimal (%v vs %v) contradicts StatusInfeasible", label, stA, stB)
		}
		checkLPExactInfeasibleCert(t, label, p, dualInf, stA, stB)
	case (stA == gb.StatusInfeasible && stB == gb.StatusUnbounded) ||
		(stB == gb.StatusInfeasible && stA == gb.StatusUnbounded):
		xUnb, dualInf := xB, dualA
		if stB == gb.StatusInfeasible {
			xUnb, dualInf = xA, dualB
		}
		if _, okU := gb.ExtractUnboundedCertificate(t.Context(), p, nil, xUnb); okU {
			t.Fatalf("[%s] exact-Q unbounded LP (%v vs %v) contradicts StatusInfeasible", label, stA, stB)
		}
		if _, errU := buildSubTolUnboundedCertJSON(t.Context(), p); errU == nil {
			t.Fatalf("[%s] exact-Q sub-tol unbounded LP (%v vs %v) contradicts StatusInfeasible", label, stA, stB)
		}
		checkLPExactInfeasibleCert(t, label, p, dualInf, stA, stB)
	default:
		t.Fatalf("[%s] LP status %v != %v", label, stA, stB)
	}
}

func checkStructSpecResult(t *testing.T, label string, spec *structModelSpec) {
	t.Helper()
	mdl, _ := compileStructModelSpec(spec, false)
	p, err := mdl.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked failed: %v", label, err)
	}

	hasInt01 := false
	for j, vs := range spec.vars {
		if vs.as01 {
			hasInt01 = true
			if p.VarCategory(j) != gb.Binary || !p.IsBinaryVar(j) {
				t.Fatalf("[%s] Int[0,1] var %d not normalized to Binary (cat=%v isBin=%v)",
					label, j, p.VarCategory(j), p.IsBinaryVar(j))
			}
		}
	}

	if hasInt01 {
		mdlBin, _ := compileStructModelSpec(spec, true)
		pBin, errB := mdlBin.CompileChecked()
		if errB != nil {
			t.Fatalf("[%s] Binary-equivalent CompileChecked failed: %v", label, errB)
		}
		resA := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithMaxNodes(500), gb.WithTimeLimit(500*time.Millisecond))
		resB := mustSolveCompiled(t, pBin, gb.WithPureGo(), gb.WithMaxNodes(500), gb.WithTimeLimit(500*time.Millisecond))
		verifyMILPInvariants(t, label+"/int01_a", p, resA)
		verifyMILPInvariants(t, label+"/int01_b", pBin, resB)
		if !isResourceLimitStatus(resA.Status) && !isResourceLimitStatus(resB.Status) &&
			resA.Status != gb.StatusNumericalError && resB.Status != gb.StatusNumericalError {
			if resA.Status != resB.Status {
				t.Fatalf("[%s] Int[0,1] (%v) != Binary (%v) status", label, resA.Status, resB.Status)
			}
			if resA.Status == gb.StatusOptimal {
				tol := 1e-5 * max(1.0, max(math.Abs(resA.Objective), math.Abs(resB.Objective)))
				if math.Abs(resA.Objective-resB.Objective) > tol {
					t.Fatalf("[%s] Int[0,1] obj %.8g != Binary obj %.8g", label, resA.Objective, resB.Objective)
				}
			}
		}
	}

	if !p.IsMIP() {
		ws1 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws1)
		lpSIMD := gb.NewLPSolver(p, ws1)
		resSIMD := lpSIMD.Solve()

		ws2 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws2)
		lpScalar := gb.NewLPSolver(p, ws2)
		lpScalar.DisableSIMD = true
		resScalar := lpScalar.Solve()

		verifyFormalLPResult(t, label+"/lp_simd", p, lpSIMD, resSIMD)
		verifyFormalLPResult(t, label+"/lp_scalar", p, lpScalar, resScalar)
		resPre := verifyFormalPresolvedLP(t, label+"/lp_pre", p, true)

		assertLPStatusConsistent(t, label+"/simd_vs_scalar", p, resSIMD.Status, resScalar.Status, resSIMD.Primal, resScalar.Primal, resSIMD.Dual, resScalar.Dual)
		assertLPStatusConsistent(t, label+"/pre_vs_simd", p, resPre.Status, resSIMD.Status, resPre.Values, resSIMD.Primal, resPre.Duals, resSIMD.Dual)
		resGo := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithMaxNodes(500), gb.WithTimeLimit(500*time.Millisecond))
		verifyCompiledLPResult(t, label+"/lp_go", p, resGo)
		assertLPStatusConsistent(t, label+"/go_vs_simd", p, resGo.Status, resSIMD.Status, resGo.Values, resSIMD.Primal, resGo.Duals, resSIMD.Dual)
		return
	}

	_, _ = verifyFormalPresolveRules(t, p)
	verifyFormalRootLP(t, label+"/rootLP", p)
	// Non-certified SolveCompiled permits 1e-7 float slack, while
	// WithCertified(true) and WithVIPRTrace enforce exact rational Q.
	resFull := mustSolveCompiled(t, p,
		gb.WithPureGo(),
		gb.WithPresolve(true),
		gb.WithCuts(true),
		gb.WithHeuristics(true),
		gb.WithWorkers(1),
		gb.WithRelGapTol(1e-6),
		gb.WithAbsGapTol(1e-6),
		gb.WithMaxNodes(600),
		gb.WithTimeLimit(700*time.Millisecond),
	)
	verifyMILPInvariants(t, label+"/full", p, resFull)
	verifyCertifiedAndVIPRMIP(t, label, p, 600, 700*time.Millisecond)

	resBare := mustSolveCompiled(t, p,
		gb.WithPureGo(),
		gb.WithPresolve(false),
		gb.WithCuts(false),
		gb.WithHeuristics(false),
		gb.WithoutSIMD(),
		gb.WithMaxNodes(600),
		gb.WithTimeLimit(700*time.Millisecond),
	)
	verifyMILPInvariants(t, label+"/bare", p, resBare)
	if resFull.Status == gb.StatusOptimal && resBare.Status == gb.StatusInfeasible {
		if _, okQ := exactMIPIncumbentRat(t.Context(), p, resFull.Values); okQ {
			t.Fatalf("[%s] exact-Q full Optimal (%.8g) contradicts bare Infeasible", label, resFull.Objective)
		}
	}
	if resBare.Status == gb.StatusOptimal && resFull.Status == gb.StatusInfeasible {
		if _, okQ := exactMIPIncumbentRat(t.Context(), p, resBare.Values); okQ {
			t.Fatalf("[%s] exact-Q bare Optimal (%.8g) contradicts full Infeasible", label, resBare.Objective)
		}
	}
}

func FuzzStructLPAndMILP(f *testing.F) {
	// Seed general regimes and every rare-exit-path sub-mode (192..201).
	f.Add([]byte{0, 4, 3, 1, 0, 1, 0, 2, 1, 3, 2, 4, 5, 6, 7})
	f.Add([]byte{1, 6, 5, 2, 1, 5, 2, 6, 3, 7, 4, 8, 5, 9, 10})
	f.Add([]byte{2, 8, 4, 3, 3, 6, 1, 7, 2, 5, 0, 1, 2, 3, 4})
	for sub := range 10 {
		f.Add([]byte{byte(192 + sub), 0, 0, 1, 2, 3, 4, 5})
		f.Add([]byte{byte(192 + sub), 1, 1, 2, 3, 4, 5, 6})
	}
	f.Add([]byte("00000(0110001"))          // 1e20 wide bound + 5e-6 coeff near float64 ULP boundary
	f.Add([]byte("000019111010001"))        // 5e-6*x0 <= 2.5e-5 near float64 ULP vs exact-Q MILP boundary
	f.Add([]byte("01000(02021000010101"))   // 1e20 wide bound MILP fail-closed certification boundary
	f.Add([]byte("01000$00120102071"))      // 1e15 bound * 5e11 coeff exact CertifiedWitness vs float64 rounding
	f.Add([]byte("0000222170\xc0"))         // 1e-6 float-feasible vs exact-Q Infeasible parity boundary
	f.Add([]byte("1200000200(0212A700101")) // 2.5e20 exactOrig unrounded exprConst RHS vs CertifiedWitness
	f.Add([]byte{1, 0, 3, 2, 1, 0, 0, 17, 1, 1, 18, 2, 19, 3, 20, 0, 2, 0, 17, 1, 18, 0, 0, 1, 1, 2, 2, 19, 3, 20, 0, 1, 0})
	f.Add([]byte{2, 0, 2, 1, 0, 0, 5, 0, 5, 0, 4, 5, 2, 3, 2, 2, 0, 8, 1, 5, 0, 0, 0}) // Profile 2 extreme 5e11+5e-6 (-5e11*x0 + 5e-6*x1 == 0)
	f.Add([]byte("2010002000000217000"))                                               // Profile 2 extreme-scale LP NumericalError certification boundary

	f.Fuzz(func(t *testing.T, data []byte) {
		c := &structByteCursor{data: data}
		modeByte := c.u8()
		var spec structModelSpec
		var label string
		if modeByte >= 192 {
			sub := int(modeByte-192) % 10
			spec = buildRareExitSpec(c, sub)
			label = fmt.Sprintf("rare_%d", sub)
		} else {
			c.pos = 0
			spec = decodeGeneralStructSpec(c)
			label = "gen"
		}
		checkStructSpecResult(t, label, &spec)
	})
}

type publicBaseEntry struct {
	id         string
	isMIP      bool
	goMaxNodes int
	compiled   *gb.CompiledProblem
}

var (
	publicBenchOnce sync.Once
	publicBenchList []publicBaseEntry
	publicBenchErr  error
)

type publicManifestEntry struct {
	ID         string `json:"id"`
	IsMIP      bool   `json:"is_mip"`
	GoMaxNodes int    `json:"go_max_nodes,omitzero"`
}

func loadPublicBenchSuite() ([]publicBaseEntry, error) {
	publicBenchOnce.Do(func() {
		metaMap := make(map[string]publicManifestEntry)
		if rawMan, err := os.ReadFile(filepath.Join("testdata", "public", "manifest.json")); err == nil {
			var entries []publicManifestEntry
			if err := json.Unmarshal(rawMan, &entries); err == nil {
				for _, e := range entries {
					metaMap[e.ID] = e
				}
			}
		}
		paths, err := filepath.Glob(filepath.Join("testdata", "public", "*.json"))
		if err != nil {
			publicBenchErr = err
			return
		}
		slices.Sort(paths)
		for _, pth := range paths {
			base := filepath.Base(pth)
			if base == "manifest.json" {
				continue
			}
			f, errOpen := os.Open(pth)
			if errOpen != nil {
				publicBenchErr = errOpen
				return
			}
			mdl, errParse := format.FromPuLPJSON(f)
			_ = f.Close()
			if errParse != nil {
				publicBenchErr = fmt.Errorf("FromPuLPJSON(%s): %w", pth, errParse)
				return
			}
			cp, errComp := mdl.CompileChecked()
			if errComp != nil {
				publicBenchErr = fmt.Errorf("CompileChecked(%s): %w", pth, errComp)
				return
			}
			id := base[:len(base)-len(".json")]
			meta := metaMap[id]
			publicBenchList = append(publicBenchList, publicBaseEntry{
				id:         id,
				isMIP:      cp.IsMIP() || meta.IsMIP,
				goMaxNodes: meta.GoMaxNodes,
				compiled:   cp,
			})
		}
		if len(publicBenchList) == 0 {
			publicBenchErr = errors.New("no public benchmark JSONs found under testdata/public")
		}
	})
	return publicBenchList, publicBenchErr
}

func cloneSpecFromCompiled(p *gb.CompiledProblem, tag string) structModelSpec {
	n := p.NumVars()
	m := p.NumConstraints()
	vars := make([]structVarSpec, n)
	obj := make([]float64, n)
	for j := range n {
		lb, ub := p.VarBounds(j)
		vars[j] = structVarSpec{
			name: p.VarName(j),
			lb:   lb,
			ub:   ub,
			cat:  p.VarCategory(j),
		}
		obj[j] = p.ObjCoeff(j)
	}
	rows := make([]structRowSpec, m)
	for i := range m {
		cols := make([]int, 0, p.RowLen(i))
		vals := make([]float64, 0, p.RowLen(i))
		p.ForEachRowTerm(i, func(col int, val float64) {
			cols = append(cols, col)
			vals = append(vals, val)
		})
		rows[i] = structRowSpec{
			name:  p.RowName(i),
			sense: p.RowSense(i),
			rhs:   p.RowRHS(i),
			cols:  cols,
			vals:  vals,
		}
	}
	return structModelSpec{
		name:     tag,
		sense:    p.Sense(),
		objConst: p.ObjConstant(),
		vars:     vars,
		obj:      obj,
		rows:     rows,
	}
}

func nudgeFloatULP(v float64, steps int, up bool) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v == 0 {
		return v
	}
	dir := math.Inf(-1)
	if up {
		dir = math.Inf(1)
	}
	cur := v
	for range steps {
		next := math.Nextafter(cur, dir)
		if math.IsInf(next, 0) || math.IsNaN(next) {
			return cur
		}
		cur = next
	}
	return cur
}

func safeLdexp(v float64, k int) (float64, bool) {
	if math.IsInf(v, 0) || math.IsNaN(v) || v == 0 {
		return v, true
	}
	s := math.Ldexp(v, k)
	if math.IsInf(s, 0) || math.IsNaN(s) {
		return v, false
	}
	return s, true
}

func applyPublicMutation(c *structByteCursor, spec *structModelSpec) {
	n := len(spec.vars)
	m := len(spec.rows)
	if n == 0 {
		return
	}
	switch c.pick(6) {
	case 0:
		// 1..4 ULP nudge on objective coefficient, row coefficient, or RHS.
		steps := 1 + c.pick(4)
		up := c.boolBit()
		switch {
		case m > 0 && c.pick(3) == 0:
			ri := c.pick(m)
			if len(spec.rows[ri].vals) > 0 {
				ti := c.pick(len(spec.rows[ri].vals))
				spec.rows[ri].vals[ti] = nudgeFloatULP(spec.rows[ri].vals[ti], steps, up)
			}
		case m > 0 && c.boolBit():
			ri := c.pick(m)
			spec.rows[ri].rhs = nudgeFloatULP(spec.rows[ri].rhs, steps, up)
		default:
			ji := c.pick(n)
			spec.obj[ji] = nudgeFloatULP(spec.obj[ji], steps, up)
		}

	case 1:
		// 2^k row or continuous-column scaling.
		k := c.pick(17) - 8 // [-8..8]
		if k == 0 {
			k = 2
		}
		if m > 0 && c.boolBit() {
			ri := c.pick(m)
			rhsS, okR := safeLdexp(spec.rows[ri].rhs, k)
			if !okR {
				return
			}
			scaledVals := make([]float64, len(spec.rows[ri].vals))
			for idx, v := range spec.rows[ri].vals {
				sv, okV := safeLdexp(v, k)
				if !okV {
					return
				}
				scaledVals[idx] = sv
			}
			spec.rows[ri].rhs = rhsS
			spec.rows[ri].vals = scaledVals
			return
		}
		col := -1
		start := c.pick(n)
		for off := range n {
			j := (start + off) % n
			if spec.vars[j].cat == gb.Continuous {
				col = j
				break
			}
		}
		if col == -1 {
			return
		}
		newObj, okO := safeLdexp(spec.obj[col], k)
		newLB, okL := safeLdexp(spec.vars[col].lb, -k)
		newUB, okU := safeLdexp(spec.vars[col].ub, -k)
		if !okO || !okL || !okU {
			return
		}
		spec.obj[col] = newObj
		spec.vars[col].lb = newLB
		spec.vars[col].ub = newUB
		for ri := range spec.rows {
			for ti, rc := range spec.rows[ri].cols {
				if rc == col {
					if sv, okV := safeLdexp(spec.rows[ri].vals[ti], k); okV {
						spec.rows[ri].vals[ti] = sv
					}
				}
			}
		}

	case 2:
		// Constraint sense flip or model min/max sense flip.
		if m > 0 && c.u8()&3 != 0 {
			ri := c.pick(m)
			switch spec.rows[ri].sense {
			case gb.LessEqual:
				spec.rows[ri].sense = gb.GreaterEqual
			case gb.GreaterEqual:
				spec.rows[ri].sense = gb.LessEqual
			default:
				if c.boolBit() {
					spec.rows[ri].sense = gb.LessEqual
				} else {
					spec.rows[ri].sense = gb.GreaterEqual
				}
			}
		} else {
			if spec.sense == gb.Minimize {
				spec.sense = gb.Maximize
			} else {
				spec.sense = gb.Minimize
			}
		}

	case 3:
		// Continuous <-> Integer variable category toggle.
		ji := c.pick(n)
		if spec.vars[ji].cat == gb.Continuous {
			spec.vars[ji].cat = gb.Integer
		} else {
			spec.vars[ji].cat = gb.Continuous
		}

	case 4:
		// Cancelling ObjConstant offset.
		cancels := []float64{-1e6, 1e6, -1e12, 1e12, 0.1, -0.1}
		oc := cancels[c.pick(len(cancels))]
		if j := c.pick(n); spec.obj[j] != 0 {
			lo, hi := spec.vars[j].lb, spec.vars[j].ub
			if !math.IsInf(lo, -1) && !math.IsInf(hi, 1) {
				mid := 0.5 * (lo + hi)
				if est := -spec.obj[j] * mid; !math.IsInf(est, 0) && !math.IsNaN(est) {
					oc = est
				}
			}
		}
		spec.objConst = oc

	default:
		// Tighten or drop variable bound.
		ji := c.pick(n)
		v := &spec.vars[ji]
		if c.boolBit() {
			// Drop one finite bound to +/-Inf.
			if c.boolBit() {
				v.lb = math.Inf(-1)
			} else {
				v.ub = math.Inf(1)
			}
			if v.cat == gb.Binary {
				v.cat = gb.Integer
			}
			return
		}
		switch {
		case !math.IsInf(v.lb, -1) && !math.IsInf(v.ub, 1):
			mid := 0.5 * (v.lb + v.ub)
			if v.cat != gb.Continuous {
				mid = math.Floor(mid)
			}
			if c.boolBit() {
				v.lb = min(v.ub, mid)
			} else {
				v.ub = max(v.lb, mid)
			}
		case !math.IsInf(v.lb, -1):
			v.ub = v.lb + 5
		case !math.IsInf(v.ub, 1):
			v.lb = v.ub - 5
		default:
			v.lb, v.ub = -5, 5
		}
	}
}

func FuzzMutatePublicBenchmarks(f *testing.F) {
	for idx := range 21 {
		f.Add([]byte{byte(idx), 1, byte(idx % 6), 0, 1, 2, 3})
	}
	f.Add([]byte{0, 3, 0, 1, 1, 2, 2, 3, 4, 5})
	f.Add([]byte{17, 2, 4, 1, 3, 2, 5, 0})
	f.Add([]byte("C0001"))

	f.Fuzz(func(t *testing.T, data []byte) {
		suite, err := loadPublicBenchSuite()
		if err != nil {
			t.Fatalf("loadPublicBenchSuite: %v", err)
		}
		c := &structByteCursor{data: data}
		base := suite[c.pick(len(suite))]
		spec := cloneSpecFromCompiled(base.compiled, "mut_"+base.id)

		nMut := 1 + c.pick(6)
		for range nMut {
			applyPublicMutation(c, &spec)
		}

		mdl, _ := compileStructModelSpec(&spec, false)
		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked after mutation failed: %v", base.id, err)
		}

		timeLimit := 150 * time.Millisecond
		if raceEnabled {
			timeLimit = 450 * time.Millisecond
		}
		maxNodes := 80
		if p.NumVars() > 150 {
			maxNodes = 30
		}
		if p.NumVars() > 400 {
			maxNodes = 12
		}
		if base.goMaxNodes > 0 {
			maxNodes = min(maxNodes, base.goMaxNodes)
		}
		label := "mut_" + base.id

		if !p.IsMIP() {
			ws1 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
			defer gb.ReleaseWorkspace(ws1)
			lpSIMD := gb.NewLPSolver(p, ws1)
			lpSIMD.MaxIter = 8000
			lpSIMD.Deadline = time.Now().Add(timeLimit)
			resSIMD := lpSIMD.Solve()

			ws2 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
			defer gb.ReleaseWorkspace(ws2)
			lpScalar := gb.NewLPSolver(p, ws2)
			lpScalar.DisableSIMD = true
			lpScalar.MaxIter = 8000
			lpScalar.Deadline = time.Now().Add(timeLimit)
			resScalar := lpScalar.Solve()

			verifyFormalLPResult(t, label+"/simd", p, lpSIMD, resSIMD)
			verifyFormalLPResult(t, label+"/scalar", p, lpScalar, resScalar)
			var resPre gb.Result
			if p.NumVars() <= 64 && p.NumConstraints() <= 40 {
				resPre = verifyFormalPresolvedLP(t, label+"/pre", p, true)
			} else {
				resPre = mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(true), gb.WithTimeLimit(timeLimit))
				verifyCompiledLPResult(t, label+"/pre", p, resPre)
			}

			assertLPStatusConsistent(t, label+"/simd_vs_scalar", p, resSIMD.Status, resScalar.Status, resSIMD.Primal, resScalar.Primal, resSIMD.Dual, resScalar.Dual)
			if resSIMD.Status == gb.StatusOptimal && resScalar.Status == gb.StatusOptimal {
				tol := 1e-4 * max(1.0, max(math.Abs(resSIMD.Objective), math.Abs(resScalar.Objective)))
				if math.Abs(resSIMD.Objective-resScalar.Objective) > tol {
					t.Fatalf("[%s] SIMD obj %.8g != Scalar obj %.8g", label, resSIMD.Objective, resScalar.Objective)
				}
			}
			assertLPStatusConsistent(t, label+"/pre_vs_simd", p, resPre.Status, resSIMD.Status, resPre.Values, resSIMD.Primal, resPre.Duals, resSIMD.Dual)
			if resPre.Status == gb.StatusOptimal && resSIMD.Status == gb.StatusOptimal {
				tol := 1e-4 * max(1.0, max(math.Abs(resPre.Objective), math.Abs(resSIMD.Objective)))
				if math.Abs(resPre.Objective-resSIMD.Objective) > tol {
					t.Fatalf("[%s] Presolve obj %.8g != LP obj %.8g", label, resPre.Objective, resSIMD.Objective)
				}
			}
			return
		}

		resPre := mustSolveCompiled(t, p,
			gb.WithPureGo(), gb.WithPresolve(true), gb.WithSIMD(true),
			gb.WithMaxNodes(maxNodes), gb.WithTimeLimit(timeLimit))
		resNoPre := mustSolveCompiled(t, p,
			gb.WithPureGo(), gb.WithPresolve(false), gb.WithSIMD(true),
			gb.WithMaxNodes(maxNodes), gb.WithTimeLimit(timeLimit))
		resScalar := mustSolveCompiled(t, p,
			gb.WithPureGo(), gb.WithPresolve(true), gb.WithoutSIMD(),
			gb.WithMaxNodes(maxNodes), gb.WithTimeLimit(timeLimit))

		verifyMILPInvariants(t, label+"/pre", p, resPre)
		verifyMILPInvariants(t, label+"/nopre", p, resNoPre)
		verifyMILPInvariants(t, label+"/scalar", p, resScalar)

		assertMIPPairConsistent(t, label+"/pre_vs_nopre", p, resPre, resNoPre)
		assertMIPPairConsistent(t, label+"/simd_vs_scalar", p, resPre, resScalar)

		if p.NumVars() <= 250 {
			verifyFormalRootLP(t, label+"/rootLP", p)
		}
		if p.NumVars() <= 512 && p.NumConstraints() <= 512 {
			verifyCertifiedAndVIPRMIP(t, label, p, maxNodes, timeLimit)
		}
	})
}

func assertMIPPairConsistent(t *testing.T, label string, p *gb.CompiledProblem, a, b gb.Result) {
	t.Helper()
	if a.Status == gb.StatusNumericalError || b.Status == gb.StatusNumericalError {
		return
	}
	if !isResourceLimitStatus(a.Status) && !isResourceLimitStatus(b.Status) {
		if (a.Status == gb.StatusOptimal && b.Status == gb.StatusInfeasible) ||
			(a.Status == gb.StatusInfeasible && b.Status == gb.StatusOptimal) {
			t.Fatalf("[%s] contradictory verdicts %v vs %v", label, a.Status, b.Status)
		}
		if a.Status == gb.StatusOptimal && b.Status == gb.StatusOptimal {
			linA := math.Abs(a.Objective - p.ObjConstant())
			linB := math.Abs(b.Objective - p.ObjConstant())
			tol := 1e-4*max(1.0, max(math.Abs(a.Objective), math.Abs(b.Objective))) + 1e-5*max(1.0, max(linA, linB))
			if math.Abs(a.Objective-b.Objective) > tol {
				t.Fatalf("[%s] objective mismatch %.8g vs %.8g (tol=%.2e)", label, a.Objective, b.Objective, tol)
			}
		}
		return
	}
	if a.Status == gb.StatusOptimal && isResourceLimitStatus(b.Status) && !math.IsInf(b.BestBound, 0) {
		tol := 1e-4 * max(1.0, max(math.Abs(a.Objective), math.Abs(b.BestBound)))
		if p.Sense() == gb.Minimize && b.BestBound > a.Objective+tol {
			t.Fatalf("[%s] limit BestBound %.8g > optimal %.8g", label, b.BestBound, a.Objective)
		}
		if p.Sense() == gb.Maximize && b.BestBound < a.Objective-tol {
			t.Fatalf("[%s] limit BestBound %.8g < optimal %.8g", label, b.BestBound, a.Objective)
		}
	}
	if b.Status == gb.StatusOptimal && isResourceLimitStatus(a.Status) && !math.IsInf(a.BestBound, 0) {
		tol := 1e-4 * max(1.0, max(math.Abs(b.Objective), math.Abs(a.BestBound)))
		if p.Sense() == gb.Minimize && a.BestBound > b.Objective+tol {
			t.Fatalf("[%s] limit BestBound %.8g > optimal %.8g", label, a.BestBound, b.Objective)
		}
		if p.Sense() == gb.Maximize && a.BestBound < b.Objective-tol {
			t.Fatalf("[%s] limit BestBound %.8g < optimal %.8g", label, a.BestBound, b.Objective)
		}
	}
}

func cloneModelSpec(src *structModelSpec, name string) structModelSpec {
	rows := make([]structRowSpec, len(src.rows))
	for i, r := range src.rows {
		rows[i] = structRowSpec{
			name:      r.name,
			sense:     r.sense,
			rhs:       r.rhs,
			exprConst: r.exprConst,
			cols:      slices.Clone(r.cols),
			vals:      slices.Clone(r.vals),
		}
	}
	return structModelSpec{
		name:     name,
		sense:    src.sense,
		objConst: src.objConst,
		vars:     slices.Clone(src.vars),
		obj:      slices.Clone(src.obj),
		rows:     rows,
	}
}

func buildWellScaledMetamorphicBase(c *structByteCursor) structModelSpec {
	sense := gb.Minimize
	if c.boolBit() {
		sense = gb.Maximize
	}
	n := 2 + c.pick(5)
	m := 1 + c.pick(5)
	objConst := float64(c.pick(11)-5) * 2.5

	vars := make([]structVarSpec, n)
	obj := make([]float64, n)
	hasEq := false
	for j := range n {
		vname := "v" + strconv.Itoa(j)
		switch c.pick(3) {
		case 0:
			vars[j] = structVarSpec{name: vname, lb: 0, ub: 1, cat: gb.Binary}
		case 1:
			lo := float64(c.pick(5) - 2)
			hi := lo + float64(1+c.pick(5))
			vars[j] = structVarSpec{name: vname, lb: lo, ub: hi, cat: gb.Integer}
		default:
			lo := float64(c.pick(5) - 2)
			hi := lo + float64(2+c.pick(7))
			if c.pick(5) == 0 {
				hi = math.Inf(1)
			}
			vars[j] = structVarSpec{name: vname, lb: lo, ub: hi, cat: gb.Continuous}
		}
		obj[j] = float64(c.pick(13)-6) * 0.5
	}

	rows := make([]structRowSpec, m)
	for i := range m {
		rSense := [3]gb.ConstraintSense{gb.LessEqual, gb.GreaterEqual, gb.Equal}[c.pick(3)]
		if i == 0 && !hasEq && c.boolBit() {
			rSense = gb.Equal
			hasEq = true
		}
		var cols []int
		var vals []float64
		for j := range n {
			if c.pick(3) != 0 {
				a := float64(c.pick(9) - 4)
				if a != 0 {
					cols = append(cols, j)
					vals = append(vals, a)
				}
			}
		}
		if len(cols) == 0 {
			cols = []int{i % n}
			vals = []float64{1}
		}
		rhs := float64(c.pick(13) - 4)
		rows[i] = structRowSpec{
			name:  "r" + strconv.Itoa(i),
			sense: rSense,
			rhs:   rhs,
			cols:  cols,
			vals:  vals,
		}
	}
	return structModelSpec{name: "meta_base", sense: sense, objConst: objConst, vars: vars, obj: obj, rows: rows}
}

func runMetamorphicChecks(t *testing.T, c *structByteCursor) {
	t.Helper()
	baseSpec := buildWellScaledMetamorphicBase(c)
	mdlBase, _ := compileStructModelSpec(&baseSpec, false)
	pBase, err := mdlBase.CompileChecked()
	if err != nil {
		t.Fatalf("base CompileChecked failed: %v", err)
	}
	resBase := mustSolveCompiled(t, pBase, gb.WithPureGo(), gb.WithMaxNodes(600), gb.WithTimeLimit(400*time.Millisecond))
	verifyMILPInvariants(t, "meta_base", pBase, resBase)

	txSpec := cloneModelSpec(&baseSpec, "meta_tx")
	txKind := c.pick(8)
	objFactor := 1.0
	isExactDyadic := true

	switch txKind {
	case 0:
		// Exact 2^k (k in [-25, 25]) row, continuous-col, and obj scaling across Ruiz [1e-4, 1e6].
		kObj := max(-15, min(15, c.pick(51)-25))
		objFactor = math.Ldexp(1.0, kObj)
		txSpec.objConst = math.Ldexp(txSpec.objConst, kObj)
		for j := range txSpec.obj {
			txSpec.obj[j] = math.Ldexp(txSpec.obj[j], kObj)
		}
		for i := range txSpec.rows {
			kr := c.pick(51) - 25
			txSpec.rows[i].rhs = math.Ldexp(txSpec.rows[i].rhs, kr)
			for tIdx := range txSpec.rows[i].vals {
				txSpec.rows[i].vals[tIdx] = math.Ldexp(txSpec.rows[i].vals[tIdx], kr)
			}
		}
		for j := range txSpec.vars {
			if txSpec.vars[j].cat != gb.Continuous {
				continue
			}
			kc := max(-16, min(16, c.pick(51)-25))
			txSpec.obj[j] = math.Ldexp(txSpec.obj[j], kc)
			if !math.IsInf(txSpec.vars[j].lb, -1) {
				txSpec.vars[j].lb = math.Ldexp(txSpec.vars[j].lb, -kc)
			}
			if !math.IsInf(txSpec.vars[j].ub, 1) {
				txSpec.vars[j].ub = math.Ldexp(txSpec.vars[j].ub, -kc)
			}
			for i := range txSpec.rows {
				for tIdx, col := range txSpec.rows[i].cols {
					if col == j {
						txSpec.rows[i].vals[tIdx] = math.Ldexp(txSpec.rows[i].vals[tIdx], kc)
					}
				}
			}
		}

	case 1:
		// Non-dyadic 3^k / 7 scaling on rows and objective.
		isExactDyadic = false
		kObj := c.pick(5) - 2
		objFactor = math.Pow(3.0, float64(kObj)) / 7.0
		txSpec.objConst *= objFactor
		for j := range txSpec.obj {
			txSpec.obj[j] *= objFactor
		}
		for i := range txSpec.rows {
			kr := c.pick(7) - 3
			fRow := math.Pow(3.0, float64(kr)) / 7.0
			txSpec.rows[i].rhs *= fRow
			for tIdx := range txSpec.rows[i].vals {
				txSpec.rows[i].vals[tIdx] *= fRow
			}
		}

	case 2:
		// Row and column permutations.
		n := len(txSpec.vars)
		perm := make([]int, n)
		inv := make([]int, n)
		for j := range n {
			perm[j] = j
		}
		for j := n - 1; j > 0; j-- {
			sw := c.pick(j + 1)
			perm[j], perm[sw] = perm[sw], perm[j]
		}
		for newIdx, oldIdx := range perm {
			inv[oldIdx] = newIdx
		}
		newVars := make([]structVarSpec, n)
		newObj := make([]float64, n)
		for newIdx, oldIdx := range perm {
			newVars[newIdx] = txSpec.vars[oldIdx]
			newObj[newIdx] = txSpec.obj[oldIdx]
		}
		txSpec.vars = newVars
		txSpec.obj = newObj
		for i := range txSpec.rows {
			for k := range txSpec.rows[i].cols {
				txSpec.rows[i].cols[k] = inv[txSpec.rows[i].cols[k]]
			}
		}
		m := len(txSpec.rows)
		for i := m - 1; i > 0; i-- {
			sw := c.pick(i + 1)
			txSpec.rows[i], txSpec.rows[sw] = txSpec.rows[sw], txSpec.rows[i]
		}

	case 3:
		// Sign-flip (-x) and affine shifts (u - x, x + k).
		for j := range txSpec.vars {
			v := &txSpec.vars[j]
			if math.IsInf(v.lb, -1) || math.IsInf(v.ub, 1) {
				continue
			}
			op := c.pick(3)
			switch op {
			case 0:
				// x' = -x
				v.lb, v.ub = -v.ub, -v.lb
				if v.cat == gb.Binary {
					v.cat = gb.Integer
				}
				txSpec.obj[j] = -txSpec.obj[j]
				for i := range txSpec.rows {
					for k, col := range txSpec.rows[i].cols {
						if col == j {
							txSpec.rows[i].vals[k] = -txSpec.rows[i].vals[k]
						}
					}
				}
			case 1:
				// x' = u - x => x = u - x'
				u := v.ub
				l := v.lb
				v.lb = 0
				v.ub = u - l
				cUser := txSpec.obj[j]
				txSpec.objConst += cUser * u
				txSpec.obj[j] = -cUser
				for i := range txSpec.rows {
					for k, col := range txSpec.rows[i].cols {
						if col == j {
							a := txSpec.rows[i].vals[k]
							txSpec.rows[i].rhs -= a * u
							txSpec.rows[i].vals[k] = -a
						}
					}
				}
			default:
				// x' = x + k => x = x' - k
				kShift := float64(c.pick(5) - 2)
				v.lb += kShift
				v.ub += kShift
				if v.cat == gb.Binary && kShift != 0 {
					v.cat = gb.Integer
				}
				txSpec.objConst -= txSpec.obj[j] * kShift
				for i := range txSpec.rows {
					for k, col := range txSpec.rows[i].cols {
						if col == j {
							txSpec.rows[i].rhs += txSpec.rows[i].vals[k] * kShift
						}
					}
				}
			}
		}

	case 4:
		// Equal -> Le + Ge splitting.
		var splitRows []structRowSpec
		for _, r := range txSpec.rows {
			if r.sense == gb.Equal {
				rLe := r
				rLe.name = r.name + "_le"
				rLe.sense = gb.LessEqual
				rLe.cols = slices.Clone(r.cols)
				rLe.vals = slices.Clone(r.vals)
				rGe := r
				rGe.name = r.name + "_ge"
				rGe.sense = gb.GreaterEqual
				rGe.cols = slices.Clone(r.cols)
				rGe.vals = slices.Clone(r.vals)
				splitRows = append(splitRows, rLe, rGe)
			} else {
				splitRows = append(splitRows, r)
			}
		}
		txSpec.rows = splitRows

	case 5:
		// Redundant positive-sum row.
		if len(txSpec.rows) >= 2 {
			r0 := txSpec.rows[0]
			r1 := txSpec.rows[1]
			var outSense gb.ConstraintSense
			okCombo := true
			switch {
			case r0.sense == r1.sense:
				outSense = r0.sense
			case r0.sense == gb.Equal:
				outSense = r1.sense
			case r1.sense == gb.Equal:
				outSense = r0.sense
			default:
				okCombo = false
			}
			if okCombo {
				acc := make(map[int]float64)
				for k, col := range r0.cols {
					acc[col] += 2.0 * r0.vals[k]
				}
				for k, col := range r1.cols {
					acc[col] += 3.0 * r1.vals[k]
				}
				cols := make([]int, 0, len(acc))
				vals := make([]float64, 0, len(acc))
				for col, val := range acc {
					if val != 0 {
						cols = append(cols, col)
						vals = append(vals, val)
					}
				}
				txSpec.rows = append(txSpec.rows, structRowSpec{
					name:  "red_combo",
					sense: outSense,
					rhs:   2.0*r0.rhs + 3.0*r1.rhs,
					cols:  cols,
					vals:  vals,
				})
			}
		}

	case 6:
		// Optimum-pinning and binary no-good cutoff when base MIP solved to Optimal.
		if pBase.IsMIP() && resBase.Status == gb.StatusOptimal && len(resBase.Values) == pBase.NumVars() {
			pinSpec := cloneModelSpec(&baseSpec, "meta_pin")
			for k := range pBase.NumIntVars() {
				col := pBase.IntCol(k)
				v := math.Round(resBase.Values[col])
				pinSpec.vars[col].lb = v
				pinSpec.vars[col].ub = v
			}
			mdlPin, _ := compileStructModelSpec(&pinSpec, false)
			pPin, errP := mdlPin.CompileChecked()
			if errP != nil {
				t.Fatalf("pin CompileChecked failed: %v", errP)
			}
			resPin := mustSolveCompiled(t, pPin, gb.WithPureGo(), gb.WithMaxNodes(400), gb.WithTimeLimit(300*time.Millisecond))
			verifyMILPInvariants(t, "meta_pin", pPin, resPin)
			if resPin.Status != gb.StatusOptimal {
				t.Fatalf("optimum-pinning returned %v, want StatusOptimal", resPin.Status)
			}
			tol := 1e-4 * max(1.0, math.Abs(resBase.Objective))
			if math.Abs(resPin.Objective-resBase.Objective) > tol {
				t.Fatalf("optimum-pinning obj %.8g != base %.8g", resPin.Objective, resBase.Objective)
			}

			// No-good cutoff over binary variables.
			var binCols []int
			for j := range pBase.NumVars() {
				if pBase.IsBinaryVar(j) {
					binCols = append(binCols, j)
				}
			}
			if len(binCols) > 0 {
				ngSpec := cloneModelSpec(&baseSpec, "meta_nogood")
				var cols []int
				var vals []float64
				ones := 0
				for _, col := range binCols {
					cols = append(cols, col)
					if math.Round(resBase.Values[col]) >= 0.5 {
						vals = append(vals, -1)
						ones++
					} else {
						vals = append(vals, 1)
					}
				}
				ngSpec.rows = append(ngSpec.rows, structRowSpec{
					name:  "nogood",
					sense: gb.GreaterEqual,
					rhs:   float64(1 - ones),
					cols:  cols,
					vals:  vals,
				})
				mdlNG, _ := compileStructModelSpec(&ngSpec, false)
				pNG, errNG := mdlNG.CompileChecked()
				if errNG != nil {
					t.Fatalf("nogood CompileChecked failed: %v", errNG)
				}
				resNG := mustSolveCompiled(t, pNG, gb.WithPureGo(), gb.WithMaxNodes(400), gb.WithTimeLimit(300*time.Millisecond))
				verifyMILPInvariants(t, "meta_nogood", pNG, resNG)
				if resNG.Status == gb.StatusOptimal {
					tolNG := 1e-4 * max(1.0, max(math.Abs(resBase.Objective), math.Abs(resNG.Objective)))
					if pBase.Sense() == gb.Minimize && resNG.Objective < resBase.Objective-tolNG {
						t.Fatalf("no-good cut produced strictly better Minimize obj %.8g < %.8g", resNG.Objective, resBase.Objective)
					}
					if pBase.Sense() == gb.Maximize && resNG.Objective > resBase.Objective+tolNG {
						t.Fatalf("no-good cut produced strictly better Maximize obj %.8g > %.8g", resNG.Objective, resBase.Objective)
					}
				}
			}
		}
		return

	default:
		// Combined row split and min <-> max flip.
		if txSpec.sense == gb.Minimize {
			txSpec.sense = gb.Maximize
		} else {
			txSpec.sense = gb.Minimize
		}
		objFactor = -1.0
		txSpec.objConst = -txSpec.objConst
		for j := range txSpec.obj {
			txSpec.obj[j] = -txSpec.obj[j]
		}
	}

	mdlTx, _ := compileStructModelSpec(&txSpec, false)
	pTx, errTx := mdlTx.CompileChecked()
	if errTx != nil {
		t.Fatalf("tx=%d CompileChecked failed: %v", txKind, errTx)
	}
	resTx := mustSolveCompiled(t, pTx, gb.WithPureGo(), gb.WithMaxNodes(600), gb.WithTimeLimit(400*time.Millisecond))
	verifyMILPInvariants(t, fmt.Sprintf("meta_tx_%d", txKind), pTx, resTx)

	if isResourceLimitStatus(resBase.Status) || isResourceLimitStatus(resTx.Status) ||
		resBase.Status == gb.StatusNumericalError || resTx.Status == gb.StatusNumericalError {
		return
	}
	if resBase.Status != resTx.Status {
		if !isExactDyadic && (resBase.Status == gb.StatusInfeasible || resTx.Status == gb.StatusInfeasible) {
			// Non-dyadic 3^k/7 float64 rounding can flip tight integer bounds across exact Q;
			// accept divergence only when WithCertified(true) confirms both compiled models.
			cBase, errCB := gb.SolveCompiled(t.Context(), pBase, gb.WithPureGo(), gb.WithMaxNodes(600), gb.WithTimeLimit(400*time.Millisecond), gb.WithCertified(true))
			cTx, errCT := gb.SolveCompiled(t.Context(), pTx, gb.WithPureGo(), gb.WithMaxNodes(600), gb.WithTimeLimit(400*time.Millisecond), gb.WithCertified(true))
			if errCB == nil && errCT == nil && !isResourceLimitStatus(cBase.Status) && !isResourceLimitStatus(cTx.Status) &&
				cBase.Status == resBase.Status && cTx.Status == resTx.Status {
				return
			}
		}
		t.Fatalf("tx=%d: status mismatch base=%v tx=%v", txKind, resBase.Status, resTx.Status)
	}
	if resBase.Status == gb.StatusOptimal {
		wantObj := resBase.Objective * objFactor
		tol := 1e-4 * max(1.0, max(math.Abs(wantObj), math.Abs(resTx.Objective)))
		if !isExactDyadic {
			tol = 1e-3 * max(1.0, max(math.Abs(wantObj), math.Abs(resTx.Objective)))
		}
		if math.Abs(resTx.Objective-wantObj) > tol {
			t.Fatalf("tx=%d: objective %.8g != expected %.8g (base=%.8g factor=%.4g)",
				txKind, resTx.Objective, wantObj, resBase.Objective, objFactor)
		}
	}
}

func buildPresolveParityModel(c *structByteCursor) structModelSpec {
	regime := c.pick(5)
	sense := gb.Minimize
	if c.boolBit() {
		sense = gb.Maximize
	}
	switch regime {
	case 0:
		// Straddle presolveFeasTol=1e-6 on integer eq, empty row, and bound gap.
		offs := []float64{-2e-6, -1.1e-6, -9e-7, -5e-7, 0, 5e-7, 9e-7, 1.1e-6, 2e-6}
		off := offs[c.pick(len(offs))]
		switch c.pick(3) {
		case 0:
			return structModelSpec{
				name:  "pre_feastol_eq",
				sense: sense,
				vars: []structVarSpec{
					{name: "x0", lb: 0, ub: 5, cat: gb.Integer},
					{name: "x1", lb: 0, ub: 5, cat: gb.Integer},
				},
				obj: []float64{1, 2},
				rows: []structRowSpec{
					{name: "eq", sense: gb.Equal, rhs: off, cols: []int{0, 1}, vals: []float64{2, -2}},
				},
			}
		case 1:
			return structModelSpec{
				name:  "pre_feastol_empty",
				sense: sense,
				vars:  []structVarSpec{{name: "x0", lb: 0, ub: 4, cat: gb.Integer}},
				obj:   []float64{1},
				rows: []structRowSpec{
					{name: "empty", sense: gb.LessEqual, rhs: off},
				},
			}
		default:
			return structModelSpec{
				name:  "pre_feastol_act",
				sense: sense,
				vars: []structVarSpec{
					{name: "x0", lb: 1, ub: 4, cat: gb.Integer},
					{name: "x1", lb: 1, ub: 4, cat: gb.Integer},
				},
				obj: []float64{1, 2},
				rows: []structRowSpec{
					{name: "le", sense: gb.LessEqual, rhs: 2 + off, cols: []int{0, 1}, vals: []float64{1, 1}},
				},
			}
		}

	case 1:
		// Straddle presolveGCDMaxRowErr=1e-9 on near-integer coefficients over bounded integers.
		epsList := []float64{0, 1e-11, 9e-11, 1.1e-10, 5e-10, -9e-11}
		eps0 := epsList[c.pick(len(epsList))]
		eps1 := epsList[c.pick(len(epsList))]
		rSense := [3]gb.ConstraintSense{gb.LessEqual, gb.GreaterEqual, gb.Equal}[c.pick(3)]
		rhs := []float64{5, 5.5, 6, 6.2}[c.pick(4)]
		return structModelSpec{
			name:  "pre_gcd_err",
			sense: sense,
			vars: []structVarSpec{
				{name: "x0", lb: -5, ub: 5, cat: gb.Integer},
				{name: "x1", lb: -5, ub: 5, cat: gb.Integer},
			},
			obj: []float64{1, 2},
			rows: []structRowSpec{
				{name: "gcd", sense: rSense, rhs: rhs, cols: []int{0, 1}, vals: []float64{2 + eps0, 4 + eps1}},
			},
		}

	case 2:
		// Straddle presolveFastMaxAbs=1e12 between fastRowCanSkip and exact row bounds.
		mag := []float64{5e11, 1e12, 1.01e12, 2e12}[c.pick(4)]
		return structModelSpec{
			name:  "pre_fast_max_abs",
			sense: sense,
			vars: []structVarSpec{
				{name: "x0", lb: 0, ub: 5, cat: gb.Integer},
				{name: "x1", lb: 0, ub: 5, cat: gb.Continuous},
			},
			obj: []float64{1, 3},
			rows: []structRowSpec{
				{name: "big", sense: gb.LessEqual, rhs: 3.5 * mag, cols: []int{0, 1}, vals: []float64{mag, 0.5 * mag}},
				{name: "red", sense: gb.LessEqual, rhs: 10 * mag, cols: []int{0, 1}, vals: []float64{mag, mag}},
			},
		}

	case 3:
		// Binary coefficient strengthening with positive and negative coefficients.
		surplus := []float64{-1e-6, 1e-6, 0.5, 1.5, 2.0}[c.pick(5)]
		a0 := 5.0
		a1 := 3.0
		a2 := -4.0
		// Max sum = a0 + a1 = 8; rhs = 8 - surplus strengthens coeffs when surplus > binTol.
		return structModelSpec{
			name:  "pre_bin_strengthen",
			sense: sense,
			vars: []structVarSpec{
				{name: "b0", lb: 0, ub: 1, cat: gb.Binary},
				{name: "b1", lb: 0, ub: 1, cat: gb.Binary},
				{name: "b2", lb: 0, ub: 1, cat: gb.Binary},
			},
			obj: []float64{4, 3, -2},
			rows: []structRowSpec{
				{name: "str", sense: gb.LessEqual, rhs: 8 - surplus, cols: []int{0, 1, 2}, vals: []float64{a0, a1, a2}},
			},
		}

	default:
		// General mixed-integer model exercising combined Presolve vs PresolveForVIPR passes.
		n := 2 + c.pick(4)
		vars := make([]structVarSpec, n)
		obj := make([]float64, n)
		for j := range n {
			cat := gb.Integer
			lo, hi := 0.0, float64(2+c.pick(5))
			switch c.pick(3) {
			case 0:
				cat = gb.Binary
				hi = 1
			case 2:
				cat = gb.Continuous
			}
			vars[j] = structVarSpec{name: "p" + strconv.Itoa(j), lb: lo, ub: hi, cat: cat}
			obj[j] = float64(c.pick(9) - 4)
		}
		m := 1 + c.pick(3)
		rows := make([]structRowSpec, m)
		for i := range m {
			cols := make([]int, 0, n)
			vals := make([]float64, 0, n)
			for j := range n {
				if c.boolBit() || j == i%n {
					v := float64(c.pick(7) - 3)
					if v != 0 {
						cols = append(cols, j)
						vals = append(vals, v)
					}
				}
			}
			if len(cols) == 0 {
				cols = []int{0}
				vals = []float64{2}
			}
			rows[i] = structRowSpec{
				name:  "pr" + strconv.Itoa(i),
				sense: [3]gb.ConstraintSense{gb.LessEqual, gb.GreaterEqual, gb.Equal}[c.pick(3)],
				rhs:   float64(c.pick(9) - 2),
				cols:  cols,
				vals:  vals,
			}
		}
		return structModelSpec{name: "pre_gen", sense: sense, vars: vars, obj: obj, rows: rows}
	}
}

func runPresolveParityChecks(t *testing.T, c *structByteCursor) {
	t.Helper()
	spec := buildPresolveParityModel(c)
	mdl, _ := compileStructModelSpec(&spec, false)
	p, err := mdl.CompileChecked()
	if err != nil {
		t.Fatalf("[%s] CompileChecked failed: %v", spec.name, err)
	}

	preRes := gb.Presolve(p, 6)
	viprRes := gb.PresolveForVIPR(p, 6)
	_, _ = verifyFormalPresolveRules(t, p)

	resNoPre := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(false), gb.WithMaxNodes(500), gb.WithTimeLimit(300*time.Millisecond))
	verifyMILPInvariants(t, spec.name+"/orig_nopre", p, resNoPre)

	if preRes.Infeasible {
		if resNoPre.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resNoPre.Values) {
			t.Fatalf("[%s] Presolve falsely declared exact-Q feasible model infeasible", spec.name)
		}
		if !viprRes.Infeasible && viprRes.Reduced != nil {
			resVR := mustSolveCompiled(t, viprRes.Reduced, gb.WithPureGo(), gb.WithPresolve(false), gb.WithMaxNodes(500), gb.WithTimeLimit(300*time.Millisecond))
			if resVR.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resVR.Values) {
				t.Fatalf("[%s] Presolve declared infeasible while PresolveForVIPR reduced solved to exact-Q Optimal", spec.name)
			}
		}
		return
	}

	if viprRes.Infeasible {
		resPreSolve := mustSolveCompiled(t, p, gb.WithPureGo(), gb.WithPresolve(true), gb.WithMaxNodes(500), gb.WithTimeLimit(300*time.Millisecond))
		if resPreSolve.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resPreSolve.Values) {
			t.Fatalf("[%s] SolveCompiled returned exact-Q Optimal while PresolveForVIPR proved infeasible", spec.name)
		}
		if formalFuzzEnabled() && p.IsMIP() {
			tr := gb.NewVIPRTrace()
			if !tr.RecordPresolveInfeasibleEnvelope(viprRes) {
				t.Fatalf("[%s] RecordPresolveInfeasibleEnvelope failed", spec.name)
			}
			if err := checkFormalVIPRResult(t, spec.name+"/vipr_pre_inf", p, gb.Result{Status: gb.StatusInfeasible, Backend: gb.BackendGo}, tr, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
		return
	}

	if preRes.Reduced == nil || viprRes.Reduced == nil {
		return
	}
	resPre := mustSolveCompiled(t, preRes.Reduced, gb.WithPureGo(), gb.WithPresolve(false), gb.WithMaxNodes(500), gb.WithTimeLimit(300*time.Millisecond))
	resVIPR := mustSolveCompiled(t, viprRes.Reduced, gb.WithPureGo(), gb.WithPresolve(false), gb.WithMaxNodes(500), gb.WithTimeLimit(300*time.Millisecond))
	verifyMILPInvariants(t, spec.name+"/pre_red", preRes.Reduced, resPre)
	verifyMILPInvariants(t, spec.name+"/vipr_red", viprRes.Reduced, resVIPR)

	// An exact point on one reduced problem must never contradict Infeasible on the other.
	if resPre.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resPre.Values) && resVIPR.Status == gb.StatusInfeasible {
		t.Fatalf("[%s] Presolve reduced exact-Q Optimal contradicts PresolveForVIPR reduced Infeasible", spec.name)
	}
	if resVIPR.Status == gb.StatusOptimal && isExactRatMIPFeasible(p, resVIPR.Values) && resPre.Status == gb.StatusInfeasible {
		t.Fatalf("[%s] PresolveForVIPR reduced exact-Q Optimal contradicts Presolve reduced Infeasible", spec.name)
	}
	if resPre.Status == gb.StatusOptimal && resVIPR.Status == gb.StatusOptimal {
		tol := 1e-4 * max(1.0, max(math.Abs(resPre.Objective), math.Abs(resVIPR.Objective)))
		if math.Abs(resPre.Objective-resVIPR.Objective) > tol {
			// Boundary-straddling rhs (e.g. 5e-7 < 1e-6) only differs if one point breaks exact Q.
			if isExactRatMIPFeasible(p, resPre.Values) && isExactRatMIPFeasible(p, resVIPR.Values) {
				t.Fatalf("[%s] Presolve obj %.8g != PresolveForVIPR obj %.8g on exact-Q feasible witnesses",
					spec.name, resPre.Objective, resVIPR.Objective)
			}
		}
	}

	if formalFuzzEnabled() && p.IsMIP() {
		tr := gb.NewVIPRTrace()
		tr.SetPresolveEnvelope(viprRes.Envelope)
		resEnv, errEnv := gb.SolveCompiled(t.Context(), viprRes.Reduced,
			gb.WithPureGo(),
			gb.WithWorkers(1),
			gb.WithPresolve(false),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
			gb.WithMaxNodes(400),
			gb.WithTimeLimit(300*time.Millisecond),
			gb.WithVIPRTrace(tr),
		)
		if errEnv != nil {
			t.Fatalf("[%s] enveloped SolveCompiled failed: %v", spec.name, errEnv)
		}
		if errC := checkFormalVIPRResult(t, spec.name+"/vipr_env", p, resEnv, tr, 0, 0); errC != nil {
			t.Fatal(errC)
		}
	}
}

func FuzzGeneralMetamorphicAndPresolveParity(f *testing.F) {
	for tx := range 8 {
		f.Add([]byte{0, byte(tx), 2, 1, 0, 1, 2, 3, 4, 5, 6, 7})
	}
	for reg := range 5 {
		f.Add([]byte{1, byte(reg), 3, 0, 1, 2, 3, 4, 5, 6})
		f.Add([]byte{2, byte(reg), 4, 1, 2, 3, 4, 5, 6, 7})
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		c := &structByteCursor{data: data}
		sel := c.pick(3)
		switch sel {
		case 0:
			runMetamorphicChecks(t, c)
		case 1:
			runPresolveParityChecks(t, c)
		default:
			cCopy := &structByteCursor{data: data, pos: c.pos}
			runMetamorphicChecks(t, c)
			runPresolveParityChecks(t, cCopy)
		}
	})
}
