package goblin

import (
	"strconv"
	"unicode/utf8"
)

// viprRawJSON holds a slice of raw JSON bytes without pulling in encoding/json
// package init tasks into binaries that do not enable VIPR tracing or certification.
type viprRawJSON []byte

func (v viprRawJSON) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return v, nil
}

func (v *viprRawJSON) UnmarshalJSON(data []byte) error {
	*v = append((*v)[:0], data...)
	return nil
}

func appendVIPRQuoted(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x20 && c < 0x80 && c != '"' && c != '\\' {
			dst = append(dst, c)
			i++
			continue
		}
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
			i++
		case '\\':
			dst = append(dst, '\\', '\\')
			i++
		case '\b':
			dst = append(dst, '\\', 'b')
			i++
		case '\f':
			dst = append(dst, '\\', 'f')
			i++
		case '\n':
			dst = append(dst, '\\', 'n')
			i++
		case '\r':
			dst = append(dst, '\\', 'r')
			i++
		case '\t':
			dst = append(dst, '\\', 't')
			i++
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
				i++
			} else {
				r, w := utf8.DecodeRuneInString(s[i:])
				dst = utf8.AppendRune(dst, r)
				i += w
			}
		}
	}
	return append(dst, '"')
}

func appendVIPRRatPayload(dst []byte, r viprRatPayload) []byte {
	dst = append(dst, `{"num":`...)
	dst = appendVIPRQuoted(dst, r.Num)
	dst = append(dst, `,"den":`...)
	dst = appendVIPRQuoted(dst, r.Den)
	return append(dst, '}')
}

func appendVIPRRatSlice(dst []byte, xs []viprRatPayload) []byte {
	dst = append(dst, '[')
	for i, x := range xs {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendVIPRRatPayload(dst, x)
	}
	return append(dst, ']')
}

func appendVIPROptRatSlice(dst []byte, xs []*viprRatPayload) []byte {
	dst = append(dst, '[')
	for i, x := range xs {
		if i > 0 {
			dst = append(dst, ',')
		}
		if x == nil {
			dst = append(dst, "null"...)
		} else {
			dst = appendVIPRRatPayload(dst, *x)
		}
	}
	return append(dst, ']')
}

func appendVIPRBoolSlice(dst []byte, xs []bool) []byte {
	dst = append(dst, '[')
	for i, b := range xs {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = strconv.AppendBool(dst, b)
	}
	return append(dst, ']')
}

func appendVIPRIntSlice(dst []byte, xs []int) []byte {
	dst = append(dst, '[')
	for i, x := range xs {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = strconv.AppendInt(dst, int64(x), 10)
	}
	return append(dst, ']')
}

func appendVIPRBaseRows(dst []byte, rows []viprLinConJSON) []byte {
	dst = append(dst, '[')
	for i, r := range rows {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, `{"coeffs":`...)
		dst = appendVIPRRatSlice(dst, r.Coeffs)
		dst = append(dst, `,"sense":`...)
		dst = appendVIPRQuoted(dst, r.Sense)
		dst = append(dst, `,"rhs":`...)
		dst = appendVIPRRatPayload(dst, r.RHS)
		dst = append(dst, '}')
	}
	return append(dst, ']')
}

func appendVIPRInstanceJSON(dst []byte, inst viprInstanceJSON) []byte {
	dst = append(dst, `{"n":`...)
	dst = strconv.AppendInt(dst, int64(inst.N), 10)
	dst = append(dst, `,"objConst":`...)
	dst = appendVIPRRatPayload(dst, inst.ObjConst)
	dst = append(dst, `,"obj":`...)
	dst = appendVIPRRatSlice(dst, inst.Obj)
	dst = append(dst, `,"isInt":`...)
	dst = appendVIPRBoolSlice(dst, inst.IsInt)
	dst = append(dst, `,"lb":`...)
	dst = appendVIPROptRatSlice(dst, inst.LB)
	dst = append(dst, `,"ub":`...)
	dst = appendVIPROptRatSlice(dst, inst.UB)
	dst = append(dst, `,"baseRows":`...)
	dst = appendVIPRBaseRows(dst, inst.BaseRows)
	return append(dst, '}')
}

func appendVIPRPresolveStepJSON(dst []byte, s viprPresolveStepJSON) []byte {
	dst = append(dst, `{"op":`...)
	dst = appendVIPRQuoted(dst, s.Op)
	if s.Row != nil {
		dst = append(dst, `,"row":`...)
		dst = strconv.AppendInt(dst, int64(*s.Row), 10)
	}
	if s.Col != nil {
		dst = append(dst, `,"col":`...)
		dst = strconv.AppendInt(dst, int64(*s.Col), 10)
	}
	if s.U != nil {
		dst = append(dst, `,"u":`...)
		dst = strconv.AppendInt(dst, int64(*s.U), 10)
	}
	if s.V != nil {
		dst = append(dst, `,"v":`...)
		dst = strconv.AppendInt(dst, int64(*s.V), 10)
	}
	if s.IsUpper != nil {
		dst = append(dst, `,"isUpper":`...)
		dst = strconv.AppendBool(dst, *s.IsUpper)
	}
	if s.RoundInt != nil {
		dst = append(dst, `,"roundInt":`...)
		dst = strconv.AppendBool(dst, *s.RoundInt)
	}
	if s.Divisor != nil {
		dst = append(dst, `,"divisor":`...)
		dst = strconv.AppendInt(dst, *s.Divisor, 10)
	}
	if s.NewBound != nil {
		dst = append(dst, `,"newBound":`...)
		dst = appendVIPRRatPayload(dst, *s.NewBound)
	}
	if s.Val != nil {
		dst = append(dst, `,"val":`...)
		dst = appendVIPRRatPayload(dst, *s.Val)
	}
	if s.Delta != nil {
		dst = append(dst, `,"delta":`...)
		dst = appendVIPRRatPayload(dst, *s.Delta)
	}
	if len(s.ColPerm) > 0 {
		dst = append(dst, `,"colPerm":`...)
		dst = appendVIPRIntSlice(dst, s.ColPerm)
	}
	if len(s.RowPerm) > 0 {
		dst = append(dst, `,"rowPerm":`...)
		dst = appendVIPRIntSlice(dst, s.RowPerm)
	}
	return append(dst, '}')
}

func appendVIPRStepJSON(dst []byte, s viprStepJSON) []byte {
	dst = append(dst, `{"rule":`...)
	dst = appendVIPRQuoted(dst, s.Rule)
	if s.Row != nil {
		dst = append(dst, `,"row":`...)
		dst = strconv.AppendInt(dst, int64(*s.Row), 10)
	}
	if s.Col != nil {
		dst = append(dst, `,"col":`...)
		dst = strconv.AppendInt(dst, int64(*s.Col), 10)
	}
	if s.Sense != "" {
		dst = append(dst, `,"sense":`...)
		dst = appendVIPRQuoted(dst, s.Sense)
	}
	if len(s.Bound) > 0 {
		dst = append(dst, `,"bound":`...)
		dst = append(dst, s.Bound...)
	}
	if s.Refs != nil {
		dst = append(dst, `,"refs":[`...)
		for i, rf := range *s.Refs {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"id":`...)
			dst = strconv.AppendInt(dst, int64(rf.ID), 10)
			dst = append(dst, `,"mult":`...)
			dst = appendVIPRRatPayload(dst, rf.Mult)
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	if len(s.Bounds) > 0 {
		dst = append(dst, `,"bounds":[`...)
		for i, br := range s.Bounds {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"col":`...)
			dst = strconv.AppendInt(dst, int64(br.Col), 10)
			dst = append(dst, `,"kind":`...)
			dst = appendVIPRQuoted(dst, br.Kind)
			dst = append(dst, `,"mult":`...)
			dst = appendVIPRRatPayload(dst, br.Mult)
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	if len(s.BoundAsms) > 0 {
		dst = append(dst, `,"boundAsms":`...)
		dst = appendVIPRIntSlice(dst, s.BoundAsms)
	}
	if s.Ref != nil {
		dst = append(dst, `,"ref":`...)
		dst = strconv.AppendInt(dst, int64(*s.Ref), 10)
	}
	if len(s.SplitVal) > 0 {
		dst = append(dst, `,"splitVal":`...)
		dst = append(dst, s.SplitVal...)
	}
	if s.LeftAsm != nil {
		dst = append(dst, `,"leftAsm":`...)
		dst = strconv.AppendInt(dst, int64(*s.LeftAsm), 10)
	}
	if s.RightAsm != nil {
		dst = append(dst, `,"rightAsm":`...)
		dst = strconv.AppendInt(dst, int64(*s.RightAsm), 10)
	}
	if s.Left != nil {
		dst = append(dst, `,"left":`...)
		dst = strconv.AppendInt(dst, int64(*s.Left), 10)
	}
	if s.Right != nil {
		dst = append(dst, `,"right":`...)
		dst = strconv.AppendInt(dst, int64(*s.Right), 10)
	}
	if len(s.OrbitAsms) > 0 {
		dst = append(dst, `,"orbitAsms":`...)
		dst = appendVIPRIntSlice(dst, s.OrbitAsms)
	}
	if len(s.Witnesses) > 0 {
		dst = append(dst, `,"witnesses":[`...)
		for i, w := range s.Witnesses {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"companion":`...)
			dst = strconv.AppendInt(dst, int64(w.Companion), 10)
			dst = append(dst, `,"colPerm":`...)
			dst = appendVIPRIntSlice(dst, w.ColPerm)
			dst = append(dst, `,"rowPerm":`...)
			dst = appendVIPRIntSlice(dst, w.RowPerm)
			if len(w.SupportEntries) > 0 {
				dst = append(dst, `,"supportEntries":`...)
				dst = appendVIPRIntSlice(dst, w.SupportEntries)
			}
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	if len(s.ExtraAsms) > 0 {
		dst = append(dst, `,"extraAsms":`...)
		dst = appendVIPRIntSlice(dst, s.ExtraAsms)
	}
	if s.LBRef != nil {
		dst = append(dst, `,"lbRef":`...)
		dst = strconv.AppendInt(dst, int64(*s.LBRef), 10)
	}
	if s.UBRef != nil {
		dst = append(dst, `,"ubRef":`...)
		dst = strconv.AppendInt(dst, int64(*s.UBRef), 10)
	}
	if len(s.Assumptions) > 0 {
		dst = append(dst, `,"assumptions":`...)
		dst = appendVIPRIntSlice(dst, s.Assumptions)
	}
	return append(dst, '}')
}

func marshalVIPRCertJSON(c viprCertJSON) []byte {
	dst := make([]byte, 0, 512+len(c.Steps)*96)
	dst = append(dst, '{')
	needComma := false
	if c.Orig != nil {
		dst = append(dst, `"orig":`...)
		dst = appendVIPRInstanceJSON(dst, *c.Orig)
		needComma = true
	}
	if len(c.Presolve) > 0 {
		if needComma {
			dst = append(dst, ',')
		}
		dst = append(dst, `"presolve":[`...)
		for i, ps := range c.Presolve {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendVIPRPresolveStepJSON(dst, ps)
		}
		dst = append(dst, ']')
		needComma = true
	}
	if needComma {
		dst = append(dst, ',')
	}
	dst = append(dst, `"n":`...)
	dst = strconv.AppendInt(dst, int64(c.N), 10)
	dst = append(dst, `,"objConst":`...)
	dst = appendVIPRRatPayload(dst, c.ObjConst)
	dst = append(dst, `,"obj":`...)
	dst = appendVIPRRatSlice(dst, c.Obj)
	dst = append(dst, `,"isInt":`...)
	dst = appendVIPRBoolSlice(dst, c.IsInt)
	dst = append(dst, `,"lb":`...)
	dst = appendVIPROptRatSlice(dst, c.LB)
	dst = append(dst, `,"ub":`...)
	dst = appendVIPROptRatSlice(dst, c.UB)
	dst = append(dst, `,"baseRows":`...)
	dst = appendVIPRBaseRows(dst, c.BaseRows)
	dst = append(dst, `,"mode":`...)
	dst = appendVIPRQuoted(dst, c.Mode)
	if c.TargetLB != nil {
		dst = append(dst, `,"targetLB":`...)
		dst = appendVIPRRatPayload(dst, *c.TargetLB)
	}
	if len(c.Incumbent) > 0 {
		dst = append(dst, `,"incumbent":`...)
		dst = appendVIPRRatSlice(dst, c.Incumbent)
	}
	dst = append(dst, `,"steps":[`...)
	for i, st := range c.Steps {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendVIPRStepJSON(dst, st)
	}
	dst = append(dst, `],"rootStep":`...)
	dst = strconv.AppendInt(dst, int64(c.RootStep), 10)
	return append(dst, '}')
}
