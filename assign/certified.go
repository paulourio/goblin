package assign

import (
	"context"
	"errors"
	"math"
	"math/bits"
)

type certFaultKey struct{}

type assignFaultFunc func(rows, cols int, sense Sense, res *Result, err *error)

var defaultAssignCertHooks = assignCertHooks{
	cert2D:   certifyLinearSum2D,
	certFlat: certifyLinearSumFlat,
	certInto: certifySolveInto,
}

func setCertifiedTrue(cfg *Options) {
	cfg.Certified = true
	cfg.cert = &defaultAssignCertHooks
}

func setCertifiedFalse(cfg *Options) {
	cfg.Certified = false
	cfg.cert = nil
}

// WithCertified enables or disables fail-closed exact rational checks at runtime.
// When enabled, the checker proves that the matching is optimal in 2304-bit fixed-point
// dyadic arithmetic over the original IEEE-754 cost matrix, or proves via a Hall witness
// that no finite matching exists. Result.Objective holds the nearest float64 to the exact
// rational primal sum. When the checker verifies or repairs duals in 2304-bit dyadic
// arithmetic, it writes finite float64 duals back to Result.U and Result.V (exact when
// they fit in 53-bit float64, or rounded down so U[i] + V[j] <= C[i,j] holds over Q).
// Because exact dyadic duals need not fit in 53-bit float64, Result.U and Result.V are
// float64 duals while exact 2304-bit dyadic arithmetic proves the matching is globally
// optimal.
func WithCertified(enabled bool) Option {
	if enabled {
		return setCertifiedTrue
	}
	return setCertifiedFalse
}

func isAdjacentFloat64(a, b float64) bool {
	if a == b {
		return true
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	return math.Nextafter(b, math.Inf(1)) == a || math.Nextafter(b, math.Inf(-1)) == a
}

const dyadicWords = 36

// dyadic2304 represents an exact signed dyadic rational M * 2^-1074 in 2304-bit
// two's-complement fixed-point form. Every finite IEEE-754 float64 converts losslessly,
// and linear combinations of up to 2^200 MaxFloat64 terms evaluate without overflow.
type dyadic2304 struct {
	w [dyadicWords]uint64
}

func dyadicAdd(a, b dyadic2304) dyadic2304 {
	var out dyadic2304
	var carry uint64
	for i := range dyadicWords {
		out.w[i], carry = bits.Add64(a.w[i], b.w[i], carry)
	}
	return out
}

func dyadicSub(a, b dyadic2304) dyadic2304 {
	var out dyadic2304
	var borrow uint64
	for i := range dyadicWords {
		out.w[i], borrow = bits.Sub64(a.w[i], b.w[i], borrow)
	}
	return out
}

func (d dyadic2304) sign() int {
	if (d.w[dyadicWords-1] >> 63) != 0 {
		return -1
	}
	for i := range dyadicWords {
		if d.w[i] != 0 {
			return 1
		}
	}
	return 0
}

func dyadicCmp(a, b dyadic2304) int {
	const signBit = uint64(1) << 63
	hiA := a.w[dyadicWords-1] ^ signBit
	hiB := b.w[dyadicWords-1] ^ signBit
	if hiA < hiB {
		return -1
	}
	if hiA > hiB {
		return 1
	}
	for i := dyadicWords - 2; i >= 0; i-- {
		if a.w[i] < b.w[i] {
			return -1
		}
		if a.w[i] > b.w[i] {
			return 1
		}
	}
	return 0
}

func dyadicFromFloat(v float64) (dyadic2304, bool) {
	b := math.Float64bits(v)
	expBits := (b >> 52) & 0x7ff
	if expBits == 0x7ff {
		return dyadic2304{}, false
	}
	frac := b & ((uint64(1) << 52) - 1)
	if expBits == 0 && frac == 0 {
		return dyadic2304{}, true
	}
	var m uint64
	var shift int
	if expBits == 0 {
		m = frac
		shift = 0
	} else {
		m = (uint64(1) << 52) | frac
		shift = int(expBits) - 1
	}
	wIdx := shift / 64
	bOff := uint(shift % 64)
	var out dyadic2304
	out.w[wIdx] = m << bOff
	if bOff > 0 {
		out.w[wIdx+1] = m >> (64 - bOff)
	}
	if (b >> 63) != 0 {
		out = dyadicSub(dyadic2304{}, out)
	}
	return out, true
}

func (d dyadic2304) toFloat64() (float64, bool) {
	sgn := d.sign()
	if sgn == 0 {
		return 0, true
	}
	neg := sgn < 0
	mag := d
	if neg {
		mag = dyadicSub(dyadic2304{}, d)
	}
	hiWord := dyadicWords - 1
	for hiWord > 0 && mag.w[hiWord] == 0 {
		hiWord--
	}
	bitLen := hiWord*64 + (64 - bits.LeadingZeros64(mag.w[hiWord]))
	if bitLen <= 52 {
		fBits := mag.w[0]
		if neg {
			fBits |= uint64(1) << 63
		}
		return math.Float64frombits(fBits), true
	}
	shift := bitLen - 53
	wIdx := shift / 64
	bOff := uint(shift % 64)
	sig := mag.w[wIdx] >> bOff
	if bOff > 0 && wIdx+1 < dyadicWords {
		sig |= mag.w[wIdx+1] << (64 - bOff)
	}
	sig &= (uint64(1) << 53) - 1
	expBits := uint64(shift + 1)
	exact := true
	if shift > 0 {
		gPos := shift - 1
		gWord := gPos / 64
		gBit := uint(gPos % 64)
		guard := ((mag.w[gWord] >> gBit) & 1) != 0
		sticky := (mag.w[gWord] & ((uint64(1) << gBit) - 1)) != 0
		for k := 0; k < gWord && !sticky; k++ {
			if mag.w[k] != 0 {
				sticky = true
			}
		}
		exact = !guard && !sticky
		if guard && (sticky || (sig&1) == 1) {
			sig++
			if sig == (uint64(1) << 53) {
				sig >>= 1
				expBits++
			}
		}
	}
	if expBits >= 0x7ff {
		if neg {
			return math.Inf(-1), false
		}
		return math.Inf(1), false
	}
	fBits := (expBits << 52) | (sig & ((uint64(1) << 52) - 1))
	if neg {
		fBits |= uint64(1) << 63
	}
	return math.Float64frombits(fBits), exact
}

func isForbiddenAssignEntry(val float64, sense Sense) (forbidden, valid bool) {
	if math.IsNaN(val) {
		return false, false
	}
	if sense == Minimize {
		if math.IsInf(val, -1) {
			return false, false
		}
		return math.IsInf(val, 1), true
	}
	if math.IsInf(val, 1) {
		return false, false
	}
	return math.IsInf(val, -1), true
}

func certifyLinearSumWith(
	ctx context.Context,
	rows, cols int,
	costAt func(int, int) float64,
	sense Sense,
	res Result,
	solveErr error,
) (Result, error) {
	if ctx != nil {
		if solveErr != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
			return Result{}, solveErr
		}
		if fn, ok := ctx.Value(certFaultKey{}).(assignFaultFunc); ok && fn != nil {
			fn(rows, cols, sense, &res, &solveErr)
		}
	}
	exactObj, err := certifyAssignCore(ctx, certAssignReq{
		rows:          rows,
		cols:          cols,
		costAt:        costAt,
		sense:         sense,
		rowInd:        res.RowInd,
		colInd:        res.ColInd,
		uRaw:          res.U,
		vRaw:          res.V,
		checkDualVecs: true,
		writeDuals:    true,
		claimedObj:    res.Objective,
		solveErr:      solveErr,
	})
	if err != nil {
		return Result{}, err
	}
	res.Objective = exactObj
	return res, nil
}

func certifyLinearSum2D(ctx context.Context, rows, cols int, cost [][]float64, sense Sense, res Result, solveErr error) (Result, error) {
	return certifyLinearSumWith(ctx, rows, cols, func(r, c int) float64 {
		return cost[r][c]
	}, sense, res, solveErr)
}

func certifyLinearSumFlat(ctx context.Context, rows, cols int, origCost []float64, sense Sense, res Result, solveErr error) (Result, error) {
	return certifyLinearSumWith(ctx, rows, cols, func(r, c int) float64 {
		return origCost[r*cols+c]
	}, sense, res, solveErr)
}

func certifySolveInto(ctx context.Context, rows, cols int, origCost []float64, sense Sense, rowInd, colInd []int, ws *Workspace, obj float64, solveErr error) (float64, error) {
	if ctx != nil && solveErr != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
		return 0, solveErr
	}
	var uHint, vHint []float64
	hasDuals := false
	if solveErr == nil && ws != nil && rows > 0 && cols > 0 {
		hasDuals = true
		if rows <= cols {
			if len(ws.u) >= rows && len(ws.v) >= cols {
				uHint = ws.u[:rows]
				vHint = ws.v[:cols]
			} else {
				hasDuals = false
			}
		} else {
			if len(ws.v) >= rows && len(ws.u) >= cols {
				uHint = ws.v[:rows]
				vHint = ws.u[:cols]
			} else {
				hasDuals = false
			}
		}
	} else if solveErr == nil && (rows == 0 || cols == 0) {
		uHint = make([]float64, rows)
		vHint = make([]float64, cols)
		hasDuals = true
	}

	if ctx != nil {
		if fn, ok := ctx.Value(certFaultKey{}).(assignFaultFunc); ok && fn != nil {
			tmpRes := Result{
				RowInd:    rowInd,
				ColInd:    colInd,
				Objective: obj,
				U:         append([]float64(nil), uHint...),
				V:         append([]float64(nil), vHint...),
			}
			fn(rows, cols, sense, &tmpRes, &solveErr)
			rowInd = tmpRes.RowInd
			colInd = tmpRes.ColInd
			obj = tmpRes.Objective
			uHint = tmpRes.U
			vHint = tmpRes.V
			hasDuals = true
		}
	}

	costAt := func(r, c int) float64 {
		return origCost[r*cols+c]
	}
	exactObj, err := certifyAssignCore(ctx, certAssignReq{
		rows:          rows,
		cols:          cols,
		costAt:        costAt,
		sense:         sense,
		rowInd:        rowInd,
		colInd:        colInd,
		uRaw:          uHint,
		vRaw:          vHint,
		checkDualVecs: hasDuals,
		writeDuals:    false,
		claimedObj:    obj,
		solveErr:      solveErr,
	})
	if err != nil {
		if errors.Is(err, ErrCertificationFailed) {
			for i := range rowInd {
				rowInd[i] = -1
			}
			for i := range colInd {
				colInd[i] = -1
			}
		}
		return 0, err
	}
	return exactObj, nil
}

func hasInvalidMatrixEntry(rows, cols int, costAt func(int, int) float64, sense Sense) bool {
	for r := range rows {
		for c := range cols {
			if _, valid := isForbiddenAssignEntry(costAt(r, c), sense); !valid {
				return true
			}
		}
	}
	return false
}

func (d dyadic2304) toFloat64Floor() float64 {
	f, exact := d.toFloat64()
	if exact || math.IsNaN(f) || math.IsInf(f, 0) {
		return f
	}
	fd, ok := dyadicFromFloat(f)
	if ok && dyadicCmp(fd, d) > 0 {
		return math.Nextafter(f, math.Inf(-1))
	}
	return f
}

func convertDyadicSliceFloor(src []dyadic2304) ([]float64, bool) {
	out := make([]float64, len(src))
	for i, d := range src {
		f := d.toFloat64Floor()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, false
		}
		if f == 0 {
			f = 0
		}
		out[i] = f
	}
	return out, true
}

func writeRepairedDualsFast(uExact, vExact []dyadic2304, uNormOut, vNormOut []float64) {
	if len(uNormOut) != len(uExact) || len(vNormOut) != len(vExact) {
		return
	}
	// Floor rounding preserves uOut[i] + vOut[j] <= C[i,j] over Q
	// when vExact is already exact in float64.
	uTmp, okU := convertDyadicSliceFloor(uExact)
	if !okU {
		return
	}
	vTmp := make([]float64, len(vExact))
	for j, d := range vExact {
		f, _ := d.toFloat64()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return
		}
		if f == 0 {
			f = 0
		}
		vTmp[j] = f
	}
	copy(uNormOut, uTmp)
	copy(vNormOut, vTmp)
}

func writeRepairedDualsFromMatrix(
	m, n int,
	cMat []dyadic2304,
	cFin []bool,
	uExact, vExact []dyadic2304,
	uNormOut, vNormOut []float64,
) {
	if len(uNormOut) != m || len(vNormOut) != n {
		return
	}
	vTmp := make([]float64, n)
	vTmpD := make([]dyadic2304, n)
	vAllExact := true
	for j, d := range vExact {
		f, exact := d.toFloat64()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return
		}
		if f == 0 {
			f = 0
		}
		if !exact {
			vAllExact = false
		}
		vTmp[j] = f
		fd, ok := dyadicFromFloat(f)
		if !ok {
			return
		}
		vTmpD[j] = fd
	}
	var uTmp []float64
	if vAllExact {
		var okU bool
		uTmp, okU = convertDyadicSliceFloor(uExact)
		if !okU {
			return
		}
	} else {
		// Recompute uSafe[i] = min_{j finite} (C[i,j] - vOut[j]) floored to float64 so the
		// returned float64 pair (uOut, vOut) remains strictly dual-feasible over Q.
		uTmp = make([]float64, m)
		for i := range m {
			base := i * n
			var minU dyadic2304
			hasFin := false
			for j := range n {
				if !cFin[base+j] {
					continue
				}
				cand := dyadicSub(cMat[base+j], vTmpD[j])
				if !hasFin || dyadicCmp(cand, minU) < 0 {
					minU = cand
					hasFin = true
				}
			}
			if !hasFin {
				return
			}
			f := minU.toFloat64Floor()
			if math.IsInf(f, 0) || math.IsNaN(f) {
				return
			}
			if f == 0 {
				f = 0
			}
			uTmp[i] = f
		}
	}
	copy(uNormOut, uTmp)
	copy(vNormOut, vTmp)
}

type certAssignReq struct {
	rows          int
	cols          int
	costAt        func(int, int) float64
	sense         Sense
	rowInd        []int
	colInd        []int
	uRaw          []float64
	vRaw          []float64
	checkDualVecs bool
	writeDuals    bool
	claimedObj    float64
	solveErr      error
}

func certifyAssignCore(ctx context.Context, req certAssignReq) (float64, error) {
	rows, cols := req.rows, req.cols
	costAt := req.costAt
	sense := req.sense
	rowInd, colInd := req.rowInd, req.colInd
	uRaw, vRaw := req.uRaw, req.vRaw
	checkDualVecs, writeDuals := req.checkDualVecs, req.writeDuals
	claimedObj, solveErr := req.claimedObj, req.solveErr
	if !sense.valid() || rows < 0 || cols < 0 {
		return 0, ErrCertificationFailed
	}

	if solveErr != nil {
		if ctx != nil && ctx.Err() != nil && errors.Is(solveErr, ctx.Err()) {
			return 0, solveErr
		}
		if errors.Is(solveErr, ErrInvalidCost) {
			if hasInvalidMatrixEntry(rows, cols, costAt, sense) {
				return 0, ErrInvalidCost
			}
			return 0, ErrCertificationFailed
		}
		if errors.Is(solveErr, ErrInfeasible) {
			if err := certifyAssignInfeasible(ctx, rows, cols, costAt, sense); err != nil {
				return 0, err
			}
			return 0, ErrInfeasible
		}
		return 0, ErrCertificationFailed
	}

	k := min(rows, cols)
	if len(rowInd) != k || len(colInd) != k {
		return 0, ErrCertificationFailed
	}
	rowInd = rowInd[:k]
	colInd = colInd[:k]
	if checkDualVecs {
		if len(uRaw) != rows || len(vRaw) != cols {
			return 0, ErrCertificationFailed
		}
		for _, u := range uRaw {
			if math.IsNaN(u) || math.IsInf(u, 0) {
				return 0, ErrCertificationFailed
			}
		}
		for _, v := range vRaw {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, ErrCertificationFailed
			}
		}
	}

	if k == 0 {
		if math.IsNaN(claimedObj) || claimedObj != 0 {
			return 0, ErrCertificationFailed
		}
		return 0, nil
	}

	maxAbsFinite := 0.0
	for r := range rows {
		if ctx != nil && r&63 == 0 && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		for c := range cols {
			val := costAt(r, c)
			forb, valid := isForbiddenAssignEntry(val, sense)
			if !valid {
				return 0, ErrCertificationFailed
			}
			if !forb {
				if av := math.Abs(val); av > maxAbsFinite {
					maxAbsFinite = av
				}
			}
		}
	}

	transposed := rows > cols
	m := k
	n := max(rows, cols)
	seenRow := make([]bool, rows)
	seenCol := make([]bool, cols)
	col4row := make([]int, m)
	row4col := make([]int, n)
	for i := range m {
		col4row[i] = -1
	}
	for j := range n {
		row4col[j] = -1
	}

	var sumUser dyadic2304
	prevRow := -1
	for t, r := range rowInd {
		if t >= len(colInd) {
			return 0, ErrCertificationFailed
		}
		c := colInd[t]
		if r < 0 || r >= len(seenRow) || c < 0 || c >= len(seenCol) {
			return 0, ErrCertificationFailed
		}
		if t > 0 && r <= prevRow {
			return 0, ErrCertificationFailed
		}
		prevRow = r
		if seenRow[r] || seenCol[c] {
			return 0, ErrCertificationFailed
		}
		seenRow[r] = true
		seenCol[c] = true

		val := costAt(r, c)
		rDyadic, ok := dyadicFromFloat(val)
		if !ok {
			return 0, ErrCertificationFailed
		}
		sumUser = dyadicAdd(sumUser, rDyadic)

		if !transposed {
			if r >= len(col4row) || c >= len(row4col) {
				return 0, ErrCertificationFailed
			}
			col4row[r] = c
			row4col[c] = r
		} else {
			if c >= len(col4row) || r >= len(row4col) {
				return 0, ErrCertificationFailed
			}
			col4row[c] = r
			row4col[r] = c
		}
	}
	for i := range m {
		if col4row[i] < 0 {
			return 0, ErrCertificationFailed
		}
	}

	exactObjFloat, exactSum := sumUser.toFloat64()
	if exactObjFloat == 0 {
		exactObjFloat = 0
	}
	if math.IsNaN(claimedObj) {
		return 0, ErrCertificationFailed
	}
	if math.IsInf(exactObjFloat, 0) {
		if claimedObj != exactObjFloat {
			return 0, ErrCertificationFailed
		}
	} else if claimedObj != exactObjFloat && !(!exactSum && k >= 3 && isAdjacentFloat64(claimedObj, exactObjFloat)) {
		return 0, ErrCertificationFailed
	}

	normEntry := func(i, j int) (float64, bool) {
		var raw float64
		if !transposed {
			raw = costAt(i, j)
		} else {
			raw = costAt(j, i)
		}
		forb, _ := isForbiddenAssignEntry(raw, sense)
		if forb {
			return 0, false
		}
		if sense == Maximize {
			return -raw, true
		}
		return raw, true
	}

	var uNorm, vNorm []float64
	if checkDualVecs {
		if !transposed {
			uNorm, vNorm = uRaw, vRaw
		} else {
			uNorm, vNorm = vRaw, uRaw
		}
		if maxAbsFinite <= hugeCostThreshold && !approxDualsConsistent(m, n, normEntry, col4row, row4col, uNorm, vNorm) {
			return 0, ErrCertificationFailed
		}
	}

	if len(vNorm) == n {
		if uFast, vFast, okFast := verifyFastExactDuals(m, n, normEntry, col4row, row4col, vNorm); okFast {
			if writeDuals {
				writeRepairedDualsFast(uFast, vFast, uNorm, vNorm)
			}
			return exactObjFloat, nil
		}
	}

	var uOut, vOut []float64
	if writeDuals {
		uOut, vOut = uNorm, vNorm
	}
	if err := verifyExactResidualOptimality(ctx, m, n, normEntry, col4row, row4col, vNorm, uOut, vOut); err != nil {
		return 0, err
	}
	return exactObjFloat, nil
}

func approxDualsConsistent(
	m, n int,
	normEntry func(int, int) (float64, bool),
	col4row, row4col []int,
	uNorm, vNorm []float64,
) bool {
	if len(uNorm) != m || len(vNorm) != n {
		return false
	}
	if m < n {
		for j := range n {
			vj := vNorm[j]
			if row4col[j] < 0 {
				if vj != 0 {
					return false
				}
			} else if vj > 1e-7*max(1.0, math.Abs(vj)) {
				return false
			}
		}
	}
	for i := range m {
		mj := col4row[i]
		cm, ok := normEntry(i, mj)
		if !ok {
			return false
		}
		ui := uNorm[i]
		vm := vNorm[mj]
		scale := max(1.0, math.Abs(cm)+math.Abs(ui)+math.Abs(vm))
		if math.Abs((ui+vm)-cm) > 1e-7*scale {
			return false
		}
		for j := range n {
			cij, fin := normEntry(i, j)
			if !fin {
				continue
			}
			vj := vNorm[j]
			edgeScale := max(1.0, math.Abs(cij)+math.Abs(ui)+math.Abs(vj))
			if (ui+vj)-cij > 1e-7*edgeScale {
				return false
			}
		}
	}
	return true
}

func verifyFastExactDuals(
	m, n int,
	normEntry func(int, int) (float64, bool),
	col4row, row4col []int,
	vFloat []float64,
) ([]dyadic2304, []dyadic2304, bool) {
	vRat := make([]dyadic2304, n)
	for j := range n {
		if m < n && row4col[j] < 0 {
			continue
		}
		r, ok := dyadicFromFloat(vFloat[j])
		if !ok {
			return nil, nil, false
		}
		if m < n && r.sign() > 0 {
			return nil, nil, false
		}
		vRat[j] = r
	}

	uRat := make([]dyadic2304, m)
	for i := range m {
		cm, ok := normEntry(i, col4row[i])
		if !ok {
			return nil, nil, false
		}
		cmD, okD := dyadicFromFloat(cm)
		if !okD {
			return nil, nil, false
		}
		uRat[i] = dyadicSub(cmD, vRat[col4row[i]])
	}

	for i := range m {
		ui := uRat[i]
		for j := range n {
			cij, fin := normEntry(i, j)
			if !fin {
				continue
			}
			cTmp, okD := dyadicFromFloat(cij)
			if !okD {
				return nil, nil, false
			}
			if dyadicCmp(dyadicAdd(ui, vRat[j]), cTmp) > 0 {
				return nil, nil, false
			}
		}
	}
	return uRat, vRat, true
}

type ratArc struct {
	to int
	w  dyadic2304
}

func verifyExactResidualOptimality(
	ctx context.Context,
	m, n int,
	normEntry func(int, int) (float64, bool),
	col4row, row4col []int,
	vHint []float64,
	uOut, vOut []float64,
) error {
	cMat := make([]dyadic2304, m*n)
	cFin := make([]bool, m*n)
	for i := range m {
		if ctx != nil && i&31 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		base := i * n
		for j := range n {
			cij, fin := normEntry(i, j)
			if !fin {
				continue
			}
			r, ok := dyadicFromFloat(cij)
			if !ok {
				return ErrCertificationFailed
			}
			cMat[base+j] = r
			cFin[base+j] = true
		}
	}

	cMatch := make([]dyadic2304, m)
	for i := range m {
		idx := i*n + col4row[i]
		if !cFin[idx] {
			return ErrCertificationFailed
		}
		cMatch[i] = cMat[idx]
	}

	numVerts := m
	hub := -1
	if m < n {
		hub = m
		numVerts = m + 1
	}

	adj := make([][]ratArc, numVerts)
	for i := range m {
		base := i * n
		cmi := cMatch[i]
		arcs := make([]ratArc, 0, m+1)
		for i2 := range m {
			if i2 == i {
				continue
			}
			idx := base + col4row[i2]
			if cFin[idx] {
				arcs = append(arcs, ratArc{
					to: i2,
					w:  dyadicSub(cMat[idx], cmi),
				})
			}
		}
		if hub >= 0 {
			var minUnmatched dyadic2304
			hasUnmatched := false
			for j := range n {
				if row4col[j] >= 0 {
					continue
				}
				idx := base + j
				if cFin[idx] && (!hasUnmatched || dyadicCmp(cMat[idx], minUnmatched) < 0) {
					minUnmatched = cMat[idx]
					hasUnmatched = true
				}
			}
			if hasUnmatched {
				arcs = append(arcs, ratArc{
					to: hub,
					w:  dyadicSub(minUnmatched, cmi),
				})
			}
		}
		adj[i] = arcs
	}
	if hub >= 0 {
		hubArcs := make([]ratArc, m)
		for i := range m {
			hubArcs[i] = ratArc{to: i}
		}
		adj[hub] = hubArcs
	}

	pot := make([]dyadic2304, numVerts)

	converged := false
	if len(vHint) == n {
		warmOK := true
		for i := range m {
			r, ok := dyadicFromFloat(vHint[col4row[i]])
			if !ok {
				warmOK = false
				break
			}
			if hub >= 0 && r.sign() > 0 {
				r = dyadic2304{}
			}
			pot[i] = r
		}
		if warmOK {
			converged = relaxResidualPotentials(pot, adj, 4)
		}
	}

	if !converged {
		clear(pot)
		okBF, errBF := bellmanFordResidualZeroNegCycle(ctx, pot, adj)
		if errBF != nil {
			return errBF
		}
		if !okBF {
			return ErrCertificationFailed
		}
	}

	var piHub dyadic2304
	if hub >= 0 {
		piHub = pot[hub]
	}
	vExact := make([]dyadic2304, n)
	for i := range m {
		vExact[col4row[i]] = dyadicSub(pot[i], piHub)
	}
	uExact := make([]dyadic2304, m)
	for i := range m {
		uExact[i] = dyadicSub(cMatch[i], vExact[col4row[i]])
	}

	if !verifyExactLSAPDualsRat(m, n, cMat, cFin, col4row, row4col, uExact, vExact) {
		return ErrCertificationFailed
	}
	if len(uOut) == m && len(vOut) == n {
		writeRepairedDualsFromMatrix(m, n, cMat, cFin, uExact, vExact, uOut, vOut)
	}
	return nil
}

func relaxResidualPotentials(pot []dyadic2304, adj [][]ratArc, maxPasses int) bool {
	numVerts := len(pot)
	for range maxPasses {
		changed := false
		for u := range numVerts {
			pu := pot[u]
			for _, arc := range adj[u] {
				cand := dyadicAdd(pu, arc.w)
				if dyadicCmp(cand, pot[arc.to]) < 0 {
					pot[arc.to] = cand
					changed = true
				}
			}
		}
		if !changed {
			return true
		}
	}
	for u := range numVerts {
		pu := pot[u]
		for _, arc := range adj[u] {
			if dyadicCmp(dyadicAdd(pu, arc.w), pot[arc.to]) < 0 {
				return false
			}
		}
	}
	return true
}

func bellmanFordResidualZeroNegCycle(ctx context.Context, pot []dyadic2304, adj [][]ratArc) (bool, error) {
	numVerts := len(pot)
	queue := make([]int, 0, numVerts)
	inQueue := make([]bool, numVerts)
	reEnqCnt := make([]int, numVerts)
	for v := range numVerts {
		queue = append(queue, v)
		inQueue[v] = true
	}

	head := 0
	steps := 0
	for head < len(queue) {
		steps++
		if ctx != nil && steps&255 == 0 && ctx.Err() != nil {
			return false, ctx.Err()
		}
		u := queue[head]
		head++
		inQueue[u] = false
		pu := pot[u]
		for _, arc := range adj[u] {
			cand := dyadicAdd(pu, arc.w)
			if dyadicCmp(cand, pot[arc.to]) < 0 {
				pot[arc.to] = cand
				if !inQueue[arc.to] {
					reEnqCnt[arc.to]++
					if reEnqCnt[arc.to] >= numVerts {
						return false, nil
					}
					queue = append(queue, arc.to)
					inQueue[arc.to] = true
				}
			}
		}
	}
	return true, nil
}

func verifyExactLSAPDualsRat(
	m, n int,
	cMat []dyadic2304,
	cFin []bool,
	col4row, row4col []int,
	u, v []dyadic2304,
) bool {
	if m < n {
		for j := range n {
			if row4col[j] < 0 {
				if v[j].sign() != 0 {
					return false
				}
			} else if v[j].sign() > 0 {
				return false
			}
		}
	}
	for i := range m {
		mj := col4row[i]
		mIdx := i*n + mj
		if !cFin[mIdx] {
			return false
		}
		if dyadicCmp(dyadicAdd(u[i], v[mj]), cMat[mIdx]) != 0 {
			return false
		}
		base := i * n
		for j := range n {
			if !cFin[base+j] {
				continue
			}
			if dyadicCmp(dyadicAdd(u[i], v[j]), cMat[base+j]) > 0 {
				return false
			}
		}
	}
	return true
}

func certifyAssignInfeasible(
	ctx context.Context,
	rows, cols int,
	costAt func(int, int) float64,
	sense Sense,
) error {
	m := min(rows, cols)
	n := max(rows, cols)
	if m == 0 {
		return ErrCertificationFailed
	}
	transposed := rows > cols

	adj := make([][]int, m)
	for r := range rows {
		if ctx != nil && r&63 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		for c := range cols {
			forb, valid := isForbiddenAssignEntry(costAt(r, c), sense)
			if !valid {
				return ErrCertificationFailed
			}
			if !forb {
				if !transposed {
					adj[r] = append(adj[r], c)
				} else {
					adj[c] = append(adj[c], r)
				}
			}
		}
	}

	matchL := make([]int, m)
	matchR := make([]int, n)
	for i := range m {
		matchL[i] = -1
	}
	for j := range n {
		matchR[j] = -1
	}

	dist := make([]int, m)
	bfsQ := make([]int, 0, m)
	ptr := make([]int, m)
	matched := 0

	bfsAugment := func() bool {
		bfsQ = bfsQ[:0]
		foundFree := false
		for i := range m {
			if matchL[i] < 0 {
				dist[i] = 0
				bfsQ = append(bfsQ, i)
			} else {
				dist[i] = -1
			}
		}
		for head := 0; head < len(bfsQ); head++ {
			u := bfsQ[head]
			for _, v := range adj[u] {
				u2 := matchR[v]
				if u2 < 0 {
					foundFree = true
				} else if dist[u2] < 0 {
					dist[u2] = dist[u] + 1
					bfsQ = append(bfsQ, u2)
				}
			}
		}
		return foundFree
	}

	var dfsAugment func(u int) bool
	dfsAugment = func(u int) bool {
		edges := adj[u]
		for ptr[u] < len(edges) {
			v := edges[ptr[u]]
			ptr[u]++
			u2 := matchR[v]
			if u2 < 0 || (dist[u2] == dist[u]+1 && dfsAugment(u2)) {
				matchL[u] = v
				matchR[v] = u
				return true
			}
		}
		dist[u] = -1
		return false
	}

	for bfsAugment() {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		clear(ptr)
		for i := range m {
			if matchL[i] < 0 && dfsAugment(i) {
				matched++
			}
		}
	}

	if matched >= m {
		return ErrCertificationFailed
	}

	// Construct the Hall witness S subset L reachable from unmatched left vertices via alternating paths.
	inS := make([]bool, m)
	visR := make([]bool, n)
	bfsQ = bfsQ[:0]
	for i := range m {
		if matchL[i] < 0 {
			inS[i] = true
			bfsQ = append(bfsQ, i)
		}
	}
	for head := 0; head < len(bfsQ); head++ {
		u := bfsQ[head]
		for _, v := range adj[u] {
			if visR[v] {
				continue
			}
			visR[v] = true
			if u2 := matchR[v]; u2 >= 0 && !inS[u2] {
				inS[u2] = true
				bfsQ = append(bfsQ, u2)
			}
		}
	}

	if !verifyHallWitness(rows, cols, costAt, sense, inS) {
		return ErrCertificationFailed
	}
	return nil
}

func verifyHallWitness(
	rows, cols int,
	costAt func(int, int) float64,
	sense Sense,
	inS []bool,
) bool {
	m := min(rows, cols)
	n := max(rows, cols)
	if len(inS) != m || m == 0 {
		return false
	}
	cardS := 0
	for _, b := range inS {
		if b {
			cardS++
		}
	}
	if cardS == 0 {
		return false
	}

	transposed := rows > cols
	cardN := 0
	for j := range n {
		hasNeighbor := false
		for i := range m {
			if !inS[i] {
				continue
			}
			var raw float64
			if !transposed {
				raw = costAt(i, j)
			} else {
				raw = costAt(j, i)
			}
			forb, valid := isForbiddenAssignEntry(raw, sense)
			if !valid {
				return false
			}
			if !forb {
				hasNeighbor = true
				break
			}
		}
		if hasNeighbor {
			cardN++
		}
	}
	return cardN < cardS
}
