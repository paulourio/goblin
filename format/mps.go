package format

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	gb "github.com/paulourio/goblin"
)

const (
	mpsFlushThreshold = 32 * 1024
	mpsPoolMaxCap     = 64 * 1024
	mpsSpaces         = "               "
)

var mpsBufPool sync.Pool

type mpsConflictRow struct {
	col   int
	name  string
	rhs   float64
	effLB float64
}

func mpsBoundTag(cat gb.VarCategory, contTag, intTag string) string {
	if cat != gb.Continuous {
		return intTag
	}
	return contTag
}

func appendMPSField(buf []byte, s string, width int, rightAlign bool) []byte {
	pad := width - len(s)
	if pad <= 0 {
		return append(buf, s...)
	}
	if rightAlign {
		buf = append(buf, mpsSpaces[:pad]...)
		return append(buf, s...)
	}
	buf = append(buf, s...)
	return append(buf, mpsSpaces[:pad]...)
}

func appendMPSFloatField(buf []byte, val float64, width int) []byte {
	var numBuf [32]byte
	num := strconv.AppendFloat(numBuf[:0], val, 'g', -1, 64)
	if pad := width - len(num); pad > 0 {
		buf = append(buf, mpsSpaces[:pad]...)
	}
	return append(buf, num...)
}

type mpsWriter struct {
	w   io.Writer
	buf []byte
	err error
}

func (mw *mpsWriter) maybeFlush() {
	if mw.err == nil && len(mw.buf) >= mpsFlushThreshold {
		mw.flush()
	}
}

func (mw *mpsWriter) flush() {
	if mw.err != nil || len(mw.buf) == 0 {
		return
	}
	_, err := mw.w.Write(mw.buf)
	mw.buf = mw.buf[:0]
	if err != nil {
		mw.err = err
	}
}

func (mw *mpsWriter) writeString(s string) {
	if mw.err != nil {
		return
	}
	mw.buf = append(mw.buf, s...)
	mw.maybeFlush()
}

func (mw *mpsWriter) writeRowLine(sense byte, name string) {
	if mw.err != nil {
		return
	}
	mw.buf = append(mw.buf, ' ', sense, ' ', ' ')
	mw.buf = append(mw.buf, name...)
	mw.buf = append(mw.buf, '\n')
	mw.maybeFlush()
}

func (mw *mpsWriter) writeColEntry(vname, rname string, val float64) {
	if mw.err != nil {
		return
	}
	buf := mw.buf
	buf = append(buf, "    "...)
	buf = appendMPSField(buf, vname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSField(buf, rname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSFloatField(buf, val, 15)
	buf = append(buf, '\n')
	mw.buf = buf
	mw.maybeFlush()
}

func (mw *mpsWriter) writeColEntryText(vname, rname, numStr string) {
	if mw.err != nil {
		return
	}
	buf := mw.buf
	buf = append(buf, "    "...)
	buf = appendMPSField(buf, vname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSField(buf, rname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSField(buf, numStr, 15, true)
	buf = append(buf, '\n')
	mw.buf = buf
	mw.maybeFlush()
}

func (mw *mpsWriter) writeRHSEntry(rname string, val float64) {
	if mw.err != nil {
		return
	}
	buf := mw.buf
	buf = append(buf, "    RHS       "...)
	buf = appendMPSField(buf, rname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSFloatField(buf, val, 15)
	buf = append(buf, '\n')
	mw.buf = buf
	mw.maybeFlush()
}

func (mw *mpsWriter) writeBoundVal(bndType, vname string, val float64) {
	if mw.err != nil {
		return
	}
	buf := mw.buf
	buf = append(buf, ' ')
	buf = append(buf, bndType...)
	buf = append(buf, " BND       "...)
	buf = appendMPSField(buf, vname, 10, false)
	buf = append(buf, ' ')
	buf = appendMPSFloatField(buf, val, 15)
	buf = append(buf, '\n')
	mw.buf = buf
	mw.maybeFlush()
}

func (mw *mpsWriter) writeBoundFlag(bndType, vname string) {
	if mw.err != nil {
		return
	}
	buf := mw.buf
	buf = append(buf, ' ')
	buf = append(buf, bndType...)
	buf = append(buf, " BND       "...)
	buf = appendMPSField(buf, vname, 10, false)
	buf = append(buf, '\n')
	mw.buf = buf
	mw.maybeFlush()
}

func validateMPSIdent(kind, s string, allowEmpty bool) error {
	if s == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("MPS %s identifier cannot be empty", kind)
	}
	if strings.EqualFold(s, "'MARKER'") {
		return fmt.Errorf("MPS %s identifier %q is reserved for INTORG/INTEND markers", kind, s)
	}
	for i := range len(s) {
		ch := s[i]
		if ch <= ' ' || ch >= 0x7f {
			return fmt.Errorf("MPS %s identifier %q contains whitespace or non-printable ASCII", kind, s)
		}
	}
	return nil
}

func validateMPSProblem(p *gb.CompiledProblem) (probName, objName string, conflictRows []mpsConflictRow, err error) {
	if p == nil {
		return "", "", nil, gb.ErrNilProblem
	}
	if err := p.Err(); err != nil {
		return "", "", nil, err
	}
	probName = p.Name()
	if err := validateMPSIdent("problem", probName, true); err != nil {
		return "", "", nil, err
	}
	objName = p.ObjName()
	if objName == "" {
		objName = "OBJ"
	}
	if err := validateMPSIdent("objective", objName, false); err != nil {
		return "", "", nil, err
	}
	numRows := p.NumConstraints()
	numVars := p.NumVars()
	var seenRows map[string]struct{}
	if numRows > 0 {
		seenRows = make(map[string]struct{}, numRows)
		for i := range numRows {
			rname := p.RowName(i)
			if err := validateMPSIdent("row", rname, false); err != nil {
				return "", "", nil, err
			}
			if rname == objName {
				return "", "", nil, fmt.Errorf("MPS row name %q collides with objective row name", objName)
			}
			if _, dup := seenRows[rname]; dup {
				return "", "", nil, fmt.Errorf("MPS duplicate row name %q", rname)
			}
			seenRows[rname] = struct{}{}
		}
	}
	if numVars > 0 {
		seenCols := make(map[string]struct{}, numVars)
		for j := range numVars {
			vname := p.VarName(j)
			if err := validateMPSIdent("column", vname, false); err != nil {
				return "", "", nil, err
			}
			if _, dup := seenCols[vname]; dup {
				return "", "", nil, fmt.Errorf("MPS duplicate column name %q", vname)
			}
			seenCols[vname] = struct{}{}
			lb, ub := p.VarBounds(j)
			if lb > ub {
				if seenRows == nil {
					seenRows = make(map[string]struct{}, 1)
				}
				rname := fmt.Sprintf("_INFEAS_UB_%d", j)
				for idx := 0; ; idx++ {
					if idx > 0 {
						rname = fmt.Sprintf("_INFEAS_UB_%d_%d", j, idx)
					}
					if rname != objName {
						if _, exists := seenRows[rname]; !exists {
							break
						}
					}
				}
				seenRows[rname] = struct{}{}
				rhs, effLB := ub, lb
				if math.IsInf(lb, 1) || math.IsInf(ub, -1) || lb-ub > 1e-7 || math.Abs(lb) >= 1e20 || math.Abs(ub) >= 1e20 {
					rhs, effLB = 0, 1
				}
				conflictRows = append(conflictRows, mpsConflictRow{col: j, name: rname, rhs: rhs, effLB: effLB})
			}
		}
	}
	return probName, objName, conflictRows, nil
}

func writeMPSRowsSection(mw *mpsWriter, p *gb.CompiledProblem, probName, objName string, conflictRows []mpsConflictRow) {
	mw.buf = append(mw.buf, "*SENSE:"...)
	mw.buf = append(mw.buf, p.Sense().String()...)
	mw.buf = append(mw.buf, "\nNAME          "...)
	mw.buf = append(mw.buf, probName...)
	mw.buf = append(mw.buf, "\nROWS\n"...)
	mw.writeRowLine('N', objName)
	for i := range p.NumConstraints() {
		var senseByte byte = 'E'
		switch p.RowSense(i) {
		case gb.LessEqual:
			senseByte = 'L'
		case gb.GreaterEqual:
			senseByte = 'G'
		}
		mw.writeRowLine(senseByte, p.RowName(i))
	}
	for _, cr := range conflictRows {
		mw.writeRowLine('L', cr.name)
	}
}

func writeMPSColumnsSection(mw *mpsWriter, p *gb.CompiledProblem, objName string, conflictRows []mpsConflictRow) {
	mw.writeString("COLUMNS\n")
	inIntBlock := false
	crIdx := 0
	var (
		curVarName string
		hasEntry   bool
	)
	colTermFn := func(r int, val float64) {
		if mw.err != nil {
			return
		}
		mw.writeColEntry(curVarName, p.RowName(r), val)
		hasEntry = true
	}
	for j := range p.NumVars() {
		isInt := p.VarCategory(j) != gb.Continuous
		if isInt && !inIntBlock {
			mw.writeString("    MARK      'MARKER'                 'INTORG'\n")
			inIntBlock = true
		} else if !isInt && inIntBlock {
			mw.writeString("    MARKEND   'MARKER'                 'INTEND'\n")
			inIntBlock = false
		}
		curVarName = p.VarName(j)
		hasEntry = false
		if objMin := p.ObjMinCoeff(j); objMin != 0 {
			mw.writeColEntry(curVarName, objName, objMin)
			hasEntry = true
		}
		p.ForEachColTerm(j, colTermFn)
		if mw.err != nil {
			return
		}
		if crIdx < len(conflictRows) && conflictRows[crIdx].col == j {
			mw.writeColEntryText(curVarName, conflictRows[crIdx].name, "1")
			hasEntry = true
			crIdx++
		}
		if !hasEntry {
			// Emit an explicit zero objective entry so MPS parsers register unused columns before BOUNDS.
			mw.writeColEntryText(curVarName, objName, "0")
		}
		if mw.err != nil {
			return
		}
	}
	if inIntBlock {
		mw.writeString("    MARKEND   'MARKER'                 'INTEND'\n")
	}
}

func writeMPSRHSSection(mw *mpsWriter, p *gb.CompiledProblem, objName string, conflictRows []mpsConflictRow) {
	mw.writeString("RHS\n")
	if objConst := p.ObjConstant(); objConst != 0 {
		senseSign := 1.0
		if p.Sense() == gb.Maximize {
			senseSign = -1.0
		}
		mw.writeRHSEntry(objName, -senseSign*objConst)
	}
	for i := range p.NumConstraints() {
		if rhs := p.RowRHS(i); rhs != 0 {
			mw.writeRHSEntry(p.RowName(i), rhs)
		}
	}
	for _, cr := range conflictRows {
		if cr.rhs != 0 {
			mw.writeRHSEntry(cr.name, cr.rhs)
		}
	}
}

func writeMPSBoundsSection(mw *mpsWriter, p *gb.CompiledProblem, conflictRows []mpsConflictRow) {
	mw.writeString("BOUNDS\n")
	crIdx := 0
	for j := range p.NumVars() {
		vname := p.VarName(j)
		lb, ub := p.VarBounds(j)
		cat := p.VarCategory(j)
		if crIdx < len(conflictRows) && conflictRows[crIdx].col == j {
			cr := conflictRows[crIdx]
			crIdx++
			mw.writeBoundVal(mpsBoundTag(cat, "LO", "LI"), vname, cr.effLB)
			if cat != gb.Continuous {
				mw.writeBoundFlag("PL", vname)
			}
			if mw.err != nil {
				return
			}
			continue
		}
		if cat == gb.Binary && lb == 0 && ub == 1 {
			mw.writeBoundFlag("BV", vname)
			continue
		}
		if math.IsInf(lb, -1) && math.IsInf(ub, 1) {
			mw.writeBoundFlag("FR", vname)
			continue
		}
		if lb == ub {
			mw.writeBoundVal("FX", vname, lb)
			continue
		}
		if math.IsInf(lb, -1) {
			mw.writeBoundFlag("MI", vname)
		} else if lb != 0 || ub < 0 || cat != gb.Continuous {
			mw.writeBoundVal(mpsBoundTag(cat, "LO", "LI"), vname, lb)
		}
		if !math.IsInf(ub, 1) {
			mw.writeBoundVal(mpsBoundTag(cat, "UP", "UI"), vname, ub)
		} else if cat != gb.Continuous {
			mw.writeBoundFlag("PL", vname)
		}
		if mw.err != nil {
			return
		}
	}
}

// WriteMPS writes a CompiledProblem in free-format MPS (with INTORG/INTEND markers
// for integer and binary variables) suitable for direct execution by COIN-OR CBC.
// Like PuLP's writeMPS, the objective is written in minimization form (negated when Maximize).
func WriteMPS(p *gb.CompiledProblem, w io.Writer) error {
	probName, objName, conflictRows, err := validateMPSProblem(p)
	if err != nil {
		return err
	}

	bp, _ := mpsBufPool.Get().(*[]byte)
	var buf []byte
	if bp != nil {
		buf = (*bp)[:0]
	} else {
		initCap := min(mpsFlushThreshold, max(4096, (p.NumVars()+p.NumConstraints())*128))
		buf = make([]byte, 0, initCap)
	}
	mw := mpsWriter{w: w, buf: buf}
	defer func() {
		if cap(mw.buf) <= mpsPoolMaxCap {
			if bp == nil {
				b := mw.buf[:0]
				bp = &b
			} else {
				*bp = mw.buf[:0]
			}
			mpsBufPool.Put(bp)
		}
	}()

	writeMPSRowsSection(&mw, p, probName, objName, conflictRows)
	if mw.err != nil {
		return mw.err
	}
	writeMPSColumnsSection(&mw, p, objName, conflictRows)
	if mw.err != nil {
		return mw.err
	}
	writeMPSRHSSection(&mw, p, objName, conflictRows)
	if mw.err != nil {
		return mw.err
	}
	writeMPSBoundsSection(&mw, p, conflictRows)
	if mw.err != nil {
		return mw.err
	}
	mw.writeString("ENDATA\n")
	mw.flush()
	return mw.err
}
