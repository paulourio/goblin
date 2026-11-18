package format

import (
	"fmt"
	"io"
	"math"
	"strconv"

	gb "github.com/paulourio/goblin"
)

func formatMPSFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func validateMPSIdent(kind, s string, allowEmpty bool) error {
	if s == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("MPS %s identifier cannot be empty", kind)
	}
	for i := range len(s) {
		ch := s[i]
		if ch <= ' ' || ch >= 0x7f {
			return fmt.Errorf("MPS %s identifier %q contains whitespace or non-printable ASCII", kind, s)
		}
	}
	return nil
}

// WriteMPS writes a CompiledProblem in free-format MPS (with INTORG/INTEND markers
// for integer and binary variables) suitable for direct execution by COIN-OR CBC.
// Like PuLP's writeMPS, the objective is written in minimization form (negated when Maximize).
func WriteMPS(p *gb.CompiledProblem, w io.Writer) error {
	if p == nil {
		return gb.ErrNilProblem
	}
	if err := p.Err(); err != nil {
		return err
	}
	probName := p.Name()
	if err := validateMPSIdent("problem", probName, true); err != nil {
		return err
	}
	objName := p.ObjName()
	if objName == "" {
		objName = "OBJ"
	}
	if err := validateMPSIdent("objective", objName, false); err != nil {
		return err
	}
	numRows := p.NumConstraints()
	numVars := p.NumVars()
	seenRows := make(map[string]struct{}, numRows)
	for i := range numRows {
		rname := p.RowName(i)
		if err := validateMPSIdent("row", rname, false); err != nil {
			return err
		}
		if rname == objName {
			return fmt.Errorf("MPS row name %q collides with objective row name", objName)
		}
		if _, dup := seenRows[rname]; dup {
			return fmt.Errorf("MPS duplicate row name %q", rname)
		}
		seenRows[rname] = struct{}{}
	}
	seenCols := make(map[string]struct{}, numVars)
	for j := range numVars {
		vname := p.VarName(j)
		if err := validateMPSIdent("column", vname, false); err != nil {
			return err
		}
		if _, dup := seenCols[vname]; dup {
			return fmt.Errorf("MPS duplicate column name %q", vname)
		}
		seenCols[vname] = struct{}{}
	}

	sense := p.Sense()
	if _, err := fmt.Fprintf(w, "*SENSE:%s\n", sense.String()); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "NAME          %s\n", probName); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "ROWS\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, " N  %s\n", objName); err != nil {
		return err
	}
	for i := range numRows {
		senseChar := "E"
		switch p.RowSense(i) {
		case gb.LessEqual:
			senseChar = "L"
		case gb.GreaterEqual:
			senseChar = "G"
		}
		if _, err := fmt.Fprintf(w, " %s  %s\n", senseChar, p.RowName(i)); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "COLUMNS\n"); err != nil {
		return err
	}
	inIntBlock := false
	for j := range numVars {
		isInt := p.VarCategory(j) != gb.Continuous
		if isInt && !inIntBlock {
			if _, err := io.WriteString(w, "    MARK      'MARKER'                 'INTORG'\n"); err != nil {
				return err
			}
			inIntBlock = true
		} else if !isInt && inIntBlock {
			if _, err := io.WriteString(w, "    MARKEND   'MARKER'                 'INTEND'\n"); err != nil {
				return err
			}
			inIntBlock = false
		}
		vname := p.VarName(j)
		hasEntry := false
		if objMin := p.ObjMinCoeff(j); objMin != 0 {
			if _, err := fmt.Fprintf(w, "    %-10s %-10s %15s\n", vname, objName, formatMPSFloat(objMin)); err != nil {
				return err
			}
			hasEntry = true
		}
		var colWriteErr error
		p.ForEachColTerm(j, func(r int, val float64) {
			if colWriteErr != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "    %-10s %-10s %15s\n", vname, p.RowName(r), formatMPSFloat(val)); err != nil {
				colWriteErr = err
				return
			}
			hasEntry = true
		})
		if colWriteErr != nil {
			return colWriteErr
		}
		if !hasEntry {
			// Emit an explicit zero objective entry so MPS parsers register unused columns before BOUNDS.
			if _, err := fmt.Fprintf(w, "    %-10s %-10s %15s\n", vname, objName, "0"); err != nil {
				return err
			}
		}
	}
	if inIntBlock {
		if _, err := io.WriteString(w, "    MARKEND   'MARKER'                 'INTEND'\n"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "RHS\n"); err != nil {
		return err
	}
	if objConst := p.ObjConstant(); objConst != 0 {
		senseSign := 1.0
		if sense == gb.Maximize {
			senseSign = -1.0
		}
		objRHS := -senseSign * objConst
		if _, err := fmt.Fprintf(w, "    RHS       %-10s %15s\n", objName, formatMPSFloat(objRHS)); err != nil {
			return err
		}
	}
	for i := range numRows {
		if rhs := p.RowRHS(i); rhs != 0 {
			if _, err := fmt.Fprintf(w, "    RHS       %-10s %15s\n", p.RowName(i), formatMPSFloat(rhs)); err != nil {
				return err
			}
		}
	}

	if _, err := io.WriteString(w, "BOUNDS\n"); err != nil {
		return err
	}
	for j := range numVars {
		vname := p.VarName(j)
		lb, ub := p.VarBounds(j)
		cat := p.VarCategory(j)
		if cat == gb.Binary && lb == 0 && ub == 1 {
			if _, err := fmt.Fprintf(w, " BV BND       %-10s\n", vname); err != nil {
				return err
			}
			continue
		}
		if math.IsInf(lb, -1) && math.IsInf(ub, 1) {
			if _, err := fmt.Fprintf(w, " FR BND       %-10s\n", vname); err != nil {
				return err
			}
			continue
		}
		if lb == ub {
			if _, err := fmt.Fprintf(w, " FX BND       %-10s %15s\n", vname, formatMPSFloat(lb)); err != nil {
				return err
			}
			continue
		}
		if math.IsInf(lb, -1) {
			if _, err := fmt.Fprintf(w, " MI BND       %-10s\n", vname); err != nil {
				return err
			}
		} else if lb != 0 || ub < 0 || cat != gb.Continuous {
			bndType := "LO"
			if cat != gb.Continuous {
				bndType = "LI"
			}
			if _, err := fmt.Fprintf(w, " %s BND       %-10s %15s\n", bndType, vname, formatMPSFloat(lb)); err != nil {
				return err
			}
		}
		if !math.IsInf(ub, 1) {
			bndType := "UP"
			if cat != gb.Continuous {
				bndType = "UI"
			}
			if _, err := fmt.Fprintf(w, " %s BND       %-10s %15s\n", bndType, vname, formatMPSFloat(ub)); err != nil {
				return err
			}
		} else if cat != gb.Continuous {
			if _, err := fmt.Fprintf(w, " PL BND       %-10s\n", vname); err != nil {
				return err
			}
		}
	}

	_, err := io.WriteString(w, "ENDATA\n")
	return err
}
