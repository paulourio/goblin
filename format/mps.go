package format

import (
	"fmt"
	"io"
	"math"

	"github.com/paulourio/goblin"
)

// WriteMPS writes a CompiledProblem in free-format MPS (with INTORG/INTEND markers
// for integer and binary variables) suitable for direct execution by COIN-OR CBC.
// Like PuLP's writeMPS, the objective is written in minimization form (negated when Maximize).
func WriteMPS(p *goblin.CompiledProblem, w io.Writer) error {
	objName := p.ObjName
	if objName == "" {
		objName = "OBJ"
	}
	if _, err := fmt.Fprintf(w, "*SENSE:%s\n", p.Sense.String()); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "NAME          %s\n", p.Name); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "ROWS\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, " N  %s\n", objName); err != nil {
		return err
	}
	for i := range p.NumConstraints {
		senseChar := "E"
		switch p.RowSense[i] {
		case goblin.LessEqual:
			senseChar = "L"
		case goblin.GreaterEqual:
			senseChar = "G"
		}
		if _, err := fmt.Fprintf(w, " %s  %s\n", senseChar, p.RowNames[i]); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "COLUMNS\n"); err != nil {
		return err
	}
	inIntBlock := false
	for j := range p.NumVars {
		isInt := p.VarCat[j] != goblin.Continuous
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
		vname := p.VarNames[j]
		if p.ObjMin[j] != 0 {
			if _, err := fmt.Fprintf(w, "    %-10s %-10s %15.12g\n", vname, objName, p.ObjMin[j]); err != nil {
				return err
			}
		}
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			r := p.RowIdx[k]
			val := p.ColVal[k]
			if _, err := fmt.Fprintf(w, "    %-10s %-10s %15.12g\n", vname, p.RowNames[r], val); err != nil {
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
	for i := range p.NumConstraints {
		if p.RowRHS[i] != 0 {
			if _, err := fmt.Fprintf(w, "    RHS       %-10s %15.12g\n", p.RowNames[i], p.RowRHS[i]); err != nil {
				return err
			}
		}
	}

	if _, err := io.WriteString(w, "BOUNDS\n"); err != nil {
		return err
	}
	for j := range p.NumVars {
		vname := p.VarNames[j]
		lb, ub := p.VarLB[j], p.VarUB[j]
		if p.VarCat[j] == goblin.Binary && lb == 0 && ub == 1 {
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
			if _, err := fmt.Fprintf(w, " FX BND       %-10s %15.12g\n", vname, lb); err != nil {
				return err
			}
			continue
		}
		if math.IsInf(lb, -1) {
			if _, err := fmt.Fprintf(w, " MI BND       %-10s\n", vname); err != nil {
				return err
			}
		} else if lb != 0 || p.VarCat[j] != goblin.Continuous {
			bndType := "LO"
			if p.VarCat[j] != goblin.Continuous {
				bndType = "LI"
			}
			if _, err := fmt.Fprintf(w, " %s BND       %-10s %15.12g\n", bndType, vname, lb); err != nil {
				return err
			}
		}
		if !math.IsInf(ub, 1) {
			bndType := "UP"
			if p.VarCat[j] != goblin.Continuous {
				bndType = "UI"
			}
			if _, err := fmt.Fprintf(w, " %s BND       %-10s %15.12g\n", bndType, vname, ub); err != nil {
				return err
			}
		} else if p.VarCat[j] != goblin.Continuous {
			if _, err := fmt.Fprintf(w, " PL BND       %-10s\n", vname); err != nil {
				return err
			}
		}
	}

	_, err := io.WriteString(w, "ENDATA\n")
	return err
}
