package goblin

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// viprRawJSON holds a slice of raw JSON bytes without pulling in encoding/json
// package init tasks into binaries that do not enable VIPR tracing or certification.
type viprRawJSON []byte

type viprRule string

const (
	viprRuleAssume        viprRule = "assume"
	viprRuleConIndex      viprRule = "conIndex"
	viprRuleLinComb       viprRule = "lincomb"
	viprRuleFarkas        viprRule = "farkas"
	viprRuleDualBound     viprRule = "dualBound"
	viprRuleSafeDual      viprRule = "safeDual"
	viprRuleRoundDown     viprRule = "roundDown"
	viprRuleRoundUp       viprRule = "roundUp"
	viprRuleBoundConflict viprRule = "boundConflict"
	viprRuleBranch        viprRule = "branch"
	viprRuleOrbitBranch   viprRule = "orbitBranch"
)

type viprMode string

const (
	viprModeOptimal    viprMode = "optimal"
	viprModeInfeasible viprMode = "infeasible"
	viprModeBound      viprMode = "bound"
)

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

const viprMaxJSONDepth = 512

func isVIPRJSONWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func skipVIPRJSONWS(b []byte, pos int) int {
	for pos < len(b) && isVIPRJSONWS(b[pos]) {
		pos++
	}
	return pos
}

func parseVIPRHex4(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var v rune
	for i := range 4 {
		c := b[i]
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v |= rune(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return v, true
}

// scanVIPRJSONString scans a RFC 8259 JSON string starting at b[pos] == '"',
// returning the decoded Go string and the byte index immediately after the closing quote.
func scanVIPRJSONString(b []byte, pos int) (string, int, bool) {
	if pos >= len(b) || b[pos] != '"' {
		return "", 0, false
	}
	i := pos + 1
	hasEscape := false
	for i < len(b) {
		c := b[i]
		if c == '"' {
			if !hasEscape {
				return string(b[pos+1 : i]), i + 1, true
			}
			break
		}
		if c < 0x20 {
			return "", 0, false
		}
		if c == '\\' {
			hasEscape = true
			i++
			if i >= len(b) {
				return "", 0, false
			}
			switch b[i] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				i++
			case 'u':
				if _, ok := parseVIPRHex4(b[i+1:]); !ok {
					return "", 0, false
				}
				i += 5
			default:
				return "", 0, false
			}
			continue
		}
		i++
	}
	if i >= len(b) || b[i] != '"' {
		return "", 0, false
	}
	end := i + 1
	var sb strings.Builder
	sb.Grow(i - (pos + 1))
	p := pos + 1
	for p < i {
		c := b[p]
		if c != '\\' {
			r, w := utf8.DecodeRune(b[p:i])
			if r == utf8.RuneError && w == 1 {
				return "", 0, false
			}
			sb.WriteRune(r)
			p += w
			continue
		}
		p++
		esc := b[p]
		p++
		switch esc {
		case '"', '\\', '/':
			sb.WriteByte(esc)
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'u':
			r1, ok := parseVIPRHex4(b[p:])
			if !ok {
				return "", 0, false
			}
			p += 4
			if utf16.IsSurrogate(r1) {
				if p+6 <= i && b[p] == '\\' && b[p+1] == 'u' {
					if r2, ok2 := parseVIPRHex4(b[p+2:]); ok2 {
						dec := utf16.DecodeRune(r1, r2)
						if dec != utf8.RuneError {
							sb.WriteRune(dec)
							p += 6
							continue
						}
					}
				}
				return "", 0, false
			}
			sb.WriteRune(r1)
		default:
			return "", 0, false
		}
	}
	return sb.String(), end, true
}

func scanVIPRJSONNumber(b []byte, pos int) (int, bool) {
	i := pos
	if i < len(b) && b[i] == '-' {
		i++
	}
	if i >= len(b) {
		return 0, false
	}
	if b[i] == '0' {
		i++
		if i < len(b) && b[i] >= '0' && b[i] <= '9' {
			return 0, false
		}
	} else if b[i] >= '1' && b[i] <= '9' {
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	} else {
		return 0, false
	}
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	return i, true
}

func scanVIPRJSONValue(b []byte, pos, depth int) (int, bool) {
	if depth > viprMaxJSONDepth {
		return 0, false
	}
	pos = skipVIPRJSONWS(b, pos)
	if pos >= len(b) {
		return 0, false
	}
	switch b[pos] {
	case '"':
		_, next, ok := scanVIPRJSONString(b, pos)
		return next, ok
	case '{':
		i := skipVIPRJSONWS(b, pos+1)
		if i >= len(b) {
			return 0, false
		}
		if b[i] == '}' {
			return i + 1, true
		}
		seen := make(map[string]struct{})
		for {
			key, next, ok := scanVIPRJSONString(b, i)
			if !ok {
				return 0, false
			}
			if _, dup := seen[key]; dup {
				return 0, false
			}
			seen[key] = struct{}{}
			i = skipVIPRJSONWS(b, next)
			if i >= len(b) || b[i] != ':' {
				return 0, false
			}
			valEnd, okV := scanVIPRJSONValue(b, i+1, depth+1)
			if !okV {
				return 0, false
			}
			i = skipVIPRJSONWS(b, valEnd)
			if i >= len(b) {
				return 0, false
			}
			if b[i] == '}' {
				return i + 1, true
			}
			if b[i] != ',' {
				return 0, false
			}
			i = skipVIPRJSONWS(b, i+1)
		}
	case '[':
		i := skipVIPRJSONWS(b, pos+1)
		if i >= len(b) {
			return 0, false
		}
		if b[i] == ']' {
			return i + 1, true
		}
		for {
			valEnd, okV := scanVIPRJSONValue(b, i, depth+1)
			if !okV {
				return 0, false
			}
			i = skipVIPRJSONWS(b, valEnd)
			if i >= len(b) {
				return 0, false
			}
			if b[i] == ']' {
				return i + 1, true
			}
			if b[i] != ',' {
				return 0, false
			}
			i = skipVIPRJSONWS(b, i+1)
		}
	case 't':
		if pos+4 <= len(b) && string(b[pos:pos+4]) == "true" {
			return pos + 4, true
		}
		return 0, false
	case 'f':
		if pos+5 <= len(b) && string(b[pos:pos+5]) == "false" {
			return pos + 5, true
		}
		return 0, false
	case 'n':
		if pos+4 <= len(b) && string(b[pos:pos+4]) == "null" {
			return pos + 4, true
		}
		return 0, false
	default:
		return scanVIPRJSONNumber(b, pos)
	}
}

func trimVIPRJSON(raw viprRawJSON) (viprRawJSON, bool) {
	if !utf8.Valid(raw) {
		return nil, false
	}
	st := skipVIPRJSONWS(raw, 0)
	if st >= len(raw) {
		return nil, false
	}
	end, ok := scanVIPRJSONValue(raw, st, 0)
	if !ok {
		return nil, false
	}
	if skipVIPRJSONWS(raw, end) != len(raw) {
		return nil, false
	}
	return raw[st:end], true
}

func viprUnmarshalString(raw viprRawJSON) (string, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok || len(t) == 0 || t[0] != '"' {
		return "", false
	}
	s, end, okS := scanVIPRJSONString(t, 0)
	if !okS || end != len(t) {
		return "", false
	}
	return s, true
}

func viprUnmarshalOptString(raw viprRawJSON) (string, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok {
		return "", false
	}
	if string(t) == "null" {
		return "", true
	}
	return viprUnmarshalString(t)
}

func viprUnmarshalBool(raw viprRawJSON) (bool, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok {
		return false, false
	}
	switch string(t) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func viprUnmarshalBoolPtr(raw viprRawJSON) (*bool, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok {
		return nil, false
	}
	if string(t) == "null" {
		return nil, true
	}
	b, okB := viprUnmarshalBool(t)
	if !okB {
		return nil, false
	}
	return &b, true
}

func viprUnmarshalIntPtr(raw viprRawJSON) (*int, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok {
		return nil, false
	}
	s := string(t)
	if s == "null" {
		return nil, true
	}
	if s == "" || s[0] == '"' || strings.ContainsAny(s, ".eE") {
		return nil, false
	}
	v, err := strconv.ParseInt(s, 10, strconv.IntSize)
	if err != nil {
		return nil, false
	}
	iv := int(v)
	return &iv, true
}

func viprUnmarshalRawArray(raw viprRawJSON, allowNull bool) ([]viprRawJSON, bool, bool) {
	t, ok := trimVIPRJSON(raw)
	if !ok {
		return nil, false, false
	}
	if string(t) == "null" {
		if allowNull {
			return nil, true, true
		}
		return nil, true, false
	}
	if t[0] != '[' {
		return nil, false, false
	}
	i := skipVIPRJSONWS(t, 1)
	if i < len(t) && t[i] == ']' {
		return []viprRawJSON{}, false, true
	}
	var elems []viprRawJSON
	for {
		st := skipVIPRJSONWS(t, i)
		end, okV := scanVIPRJSONValue(t, st, 1)
		if !okV {
			return nil, false, false
		}
		elems = append(elems, t[st:end])
		i = skipVIPRJSONWS(t, end)
		if i >= len(t) {
			return nil, false, false
		}
		if t[i] == ']' {
			return elems, false, true
		}
		if t[i] != ',' {
			return nil, false, false
		}
		i++
	}
}

func viprUnmarshalBoolSlice(raw viprRawJSON) ([]bool, bool) {
	elems, isNull, ok := viprUnmarshalRawArray(raw, true)
	if !ok {
		return nil, false
	}
	if isNull {
		return nil, true
	}
	out := make([]bool, len(elems))
	for i, el := range elems {
		b, okB := viprUnmarshalBool(el)
		if !okB {
			return nil, false
		}
		out[i] = b
	}
	return out, true
}

func viprScanObject(raw viprRawJSON, onField func(key string, val viprRawJSON) bool) bool {
	t, ok := trimVIPRJSON(raw)
	if !ok || len(t) == 0 || t[0] != '{' {
		return false
	}
	i := skipVIPRJSONWS(t, 1)
	if i < len(t) && t[i] == '}' {
		return true
	}
	seen := make(map[string]struct{})
	for {
		key, next, okK := scanVIPRJSONString(t, i)
		if !okK {
			return false
		}
		if _, dup := seen[key]; dup {
			return false
		}
		seen[key] = struct{}{}
		i = skipVIPRJSONWS(t, next)
		if i >= len(t) || t[i] != ':' {
			return false
		}
		valSt := skipVIPRJSONWS(t, i+1)
		valEnd, okV := scanVIPRJSONValue(t, valSt, 1)
		if !okV {
			return false
		}
		if !onField(key, t[valSt:valEnd]) {
			return false
		}
		i = skipVIPRJSONWS(t, valEnd)
		if i >= len(t) {
			return false
		}
		if t[i] == '}' {
			return true
		}
		if t[i] != ',' {
			return false
		}
		i = skipVIPRJSONWS(t, i+1)
	}
}

func unmarshalCertRatObjJSON(raw viprRawJSON) (certRatObjJSON, bool) {
	var out certRatObjJSON
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		switch key {
		case "num":
			out.Num = val
		case "den":
			out.Den = val
		}
		return true
	})
	return out, ok
}

func unmarshalCertSparseVecJSON(raw viprRawJSON) (certSparseVecJSON, bool) {
	var out certSparseVecJSON
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		if key != "terms" {
			return true
		}
		elems, isNull, okA := viprUnmarshalRawArray(val, true)
		if !okA {
			return false
		}
		if isNull {
			out.Terms = nil
			return true
		}
		terms := make([]certSparseTermJSON, len(elems))
		for i, el := range elems {
			var tm certSparseTermJSON
			if !viprScanObject(el, func(tk string, tv viprRawJSON) bool {
				switch tk {
				case "col":
					p, okI := viprUnmarshalIntPtr(tv)
					if !okI {
						return false
					}
					tm.Col = p
				case "coeff":
					tm.Coeff = tv
				}
				return true
			}) {
				return false
			}
			terms[i] = tm
		}
		out.Terms = terms
		return true
	})
	return out, ok
}

func decodeVIPRIntPtrField(dst **int, val viprRawJSON) bool {
	p, ok := viprUnmarshalIntPtr(val)
	if !ok {
		return false
	}
	*dst = p
	return true
}

func decodeVIPRStrictIntPtrField(dst **int, val viprRawJSON) bool {
	p, ok := viprUnmarshalIntPtr(val)
	if !ok || p == nil {
		return false
	}
	*dst = p
	return true
}

func decodeVIPRBoolPtrField(dst **bool, val viprRawJSON) bool {
	p, ok := viprUnmarshalBoolPtr(val)
	if !ok {
		return false
	}
	*dst = p
	return true
}

func decodeVIPROptStringField[S ~string](dst *S, val viprRawJSON) bool {
	s, ok := viprUnmarshalOptString(val)
	if !ok {
		return false
	}
	*dst = S(s)
	return true
}

func decodeVIPRStrictStringPtrField(dst **string, val viprRawJSON) bool {
	s, ok := viprUnmarshalString(val)
	if !ok {
		return false
	}
	*dst = &s
	return true
}

func viprUnmarshalOptRawArray(dst *[]viprRawJSON, val viprRawJSON) bool {
	elems, isNull, ok := viprUnmarshalRawArray(val, true)
	if !ok {
		return false
	}
	if !isNull {
		*dst = elems
	}
	return true
}

func unmarshalCertLinConJSON(raw viprRawJSON) (certLinConJSON, bool) {
	var out certLinConJSON
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		switch key {
		case "coeffs":
			out.Coeffs = val
		case "sense":
			return decodeVIPROptStringField(&out.Sense, val)
		case "rhs":
			out.RHS = val
		}
		return true
	})
	return out, ok
}

func unmarshalCertPresolveStepDoc(raw viprRawJSON) (certPresolveStepDoc, bool) {
	var out certPresolveStepDoc
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		switch key {
		case "op":
			return decodeVIPROptStringField(&out.Op, val)
		case "row":
			out.Row = val
		case "col":
			out.Col = val
		case "u":
			out.U = val
		case "v":
			out.V = val
		case "isUpper":
			out.IsUpper = val
		case "roundInt":
			out.RoundInt = val
		case "divisor":
			out.Divisor = val
		case "newBound":
			out.NewBound = val
		case "val":
			out.Val = val
		case "delta":
			out.Delta = val
		case "colPerm":
			out.ColPerm = val
		case "rowPerm":
			out.RowPerm = val
		}
		return true
	})
	return out, ok
}

func unmarshalCertStepDoc(raw viprRawJSON) (certStepDoc, bool) {
	var st certStepDoc
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		switch key {
		case "rule":
			return decodeVIPROptStringField(&st.Rule, val)
		case "row":
			return decodeVIPRIntPtrField(&st.Row, val)
		case "col":
			return decodeVIPRIntPtrField(&st.Col, val)
		case "sense":
			return decodeVIPRStrictStringPtrField(&st.Sense, val)
		case "isUpper":
			return decodeVIPRBoolPtrField(&st.IsUpper, val)
		case "bound":
			st.Bound = val
		case "refs":
			elems, isNull, okA := viprUnmarshalRawArray(val, true)
			if !okA {
				return false
			}
			if isNull {
				st.Refs = nil
				return true
			}
			refs := make([]certLinRefDoc, len(elems))
			for i, el := range elems {
				var rf certLinRefDoc
				if !viprScanObject(el, func(rk string, rv viprRawJSON) bool {
					switch rk {
					case "id":
						return decodeVIPRIntPtrField(&rf.ID, rv)
					case "mult":
						rf.Mult = rv
					}
					return true
				}) {
					return false
				}
				refs[i] = rf
			}
			st.Refs = &refs
		case "bounds":
			elems, _, okA := viprUnmarshalRawArray(val, false)
			if !okA {
				return false
			}
			bnds := make([]certBoundRefDoc, len(elems))
			for i, el := range elems {
				var br certBoundRefDoc
				if !viprScanObject(el, func(bk string, bv viprRawJSON) bool {
					switch bk {
					case "col":
						return decodeVIPRIntPtrField(&br.Col, bv)
					case "kind":
						return decodeVIPRStrictStringPtrField(&br.Kind, bv)
					case "isUpper":
						return decodeVIPRBoolPtrField(&br.IsUpper, bv)
					case "mult":
						br.Mult = bv
					}
					return true
				}) {
					return false
				}
				bnds[i] = br
			}
			st.Bounds = &bnds
		case "boundAsms":
			st.BoundAsms = val
		case "ref":
			return decodeVIPRIntPtrField(&st.Ref, val)
		case "splitVal":
			st.SplitVal = val
		case "leftAsm":
			return decodeVIPRIntPtrField(&st.LeftAsm, val)
		case "rightAsm":
			return decodeVIPRIntPtrField(&st.RightAsm, val)
		case "left":
			return decodeVIPRIntPtrField(&st.Left, val)
		case "right":
			return decodeVIPRIntPtrField(&st.Right, val)
		case "orbitAsms":
			st.OrbitAsms = val
		case "witnesses":
			elems, isNull, okA := viprUnmarshalRawArray(val, true)
			if !okA {
				return false
			}
			if isNull {
				st.Witnesses = nil
				return true
			}
			wits := make([]certOrbitWitDoc, len(elems))
			for i, el := range elems {
				var wd certOrbitWitDoc
				if !viprScanObject(el, func(wk string, wv viprRawJSON) bool {
					switch wk {
					case "companion":
						return decodeVIPRIntPtrField(&wd.Companion, wv)
					case "colPerm":
						wd.ColPerm = wv
					case "rowPerm":
						wd.RowPerm = wv
					case "supportEntries":
						wd.SupportEntries = wv
					}
					return true
				}) {
					return false
				}
				wits[i] = wd
			}
			st.Witnesses = &wits
		case "extraAsms":
			st.ExtraAsms = val
		case "lbRef":
			st.LBRef = val
		case "ubRef":
			st.UBRef = val
		case "con":
			st.Con = val
		case "assumptions":
			st.Assumptions = val
		}
		return true
	})
	return st, ok
}

func unmarshalCertVIPRDoc(raw viprRawJSON) (certVIPRDoc, bool) {
	var doc certVIPRDoc
	ok := viprScanObject(raw, func(key string, val viprRawJSON) bool {
		switch key {
		case "orig":
			doc.Orig = val
		case "presolve":
			doc.Presolve = val
		case "n":
			return decodeVIPRIntPtrField(&doc.N, val)
		case "objConst":
			doc.ObjConst = val
		case "obj":
			doc.Obj = val
		case "isInt":
			bs, okB := viprUnmarshalBoolSlice(val)
			if !okB {
				return false
			}
			doc.IsInt = bs
		case "lb":
			return viprUnmarshalOptRawArray(&doc.LB, val)
		case "ub":
			return viprUnmarshalOptRawArray(&doc.UB, val)
		case "baseRows":
			return viprUnmarshalOptRawArray(&doc.BaseRows, val)
		case "mode":
			return decodeVIPROptStringField(&doc.Mode, val)
		case "targetLB":
			doc.TargetLB = val
		case "incumbent":
			return viprUnmarshalOptRawArray(&doc.Incumbent, val)
		case "steps":
			elems, isNull, okA := viprUnmarshalRawArray(val, true)
			if !okA {
				return false
			}
			if isNull {
				doc.Steps = nil
				return true
			}
			steps := make([]certStepDoc, len(elems))
			for i, el := range elems {
				st, okSt := unmarshalCertStepDoc(el)
				if !okSt {
					return false
				}
				steps[i] = st
			}
			doc.Steps = steps
		case "rootStep":
			return decodeVIPRStrictIntPtrField(&doc.RootStep, val)
		}
		return true
	})
	return doc, ok
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

func appendVIPROptIntField(dst []byte, key string, v *int) []byte {
	if v == nil {
		return dst
	}
	dst = append(dst, key...)
	return strconv.AppendInt(dst, int64(*v), 10)
}

func appendVIPROptBoolField(dst []byte, key string, v *bool) []byte {
	if v == nil {
		return dst
	}
	dst = append(dst, key...)
	return strconv.AppendBool(dst, *v)
}

func appendVIPROptRatField(dst []byte, key string, v *viprRatPayload) []byte {
	if v == nil {
		return dst
	}
	dst = append(dst, key...)
	return appendVIPRRatPayload(dst, *v)
}

func appendVIPROptIntSliceField(dst []byte, key string, xs []int) []byte {
	if len(xs) == 0 {
		return dst
	}
	dst = append(dst, key...)
	return appendVIPRIntSlice(dst, xs)
}

func appendVIPRPresolveStepJSON(dst []byte, s viprPresolveStepJSON) []byte {
	dst = append(dst, `{"op":`...)
	dst = appendVIPRQuoted(dst, s.Op)
	dst = appendVIPROptIntField(dst, `,"row":`, s.Row)
	dst = appendVIPROptIntField(dst, `,"col":`, s.Col)
	dst = appendVIPROptIntField(dst, `,"u":`, s.U)
	dst = appendVIPROptIntField(dst, `,"v":`, s.V)
	dst = appendVIPROptBoolField(dst, `,"isUpper":`, s.IsUpper)
	dst = appendVIPROptBoolField(dst, `,"roundInt":`, s.RoundInt)
	if s.Divisor != nil {
		dst = append(dst, `,"divisor":`...)
		dst = strconv.AppendInt(dst, *s.Divisor, 10)
	}
	dst = appendVIPROptRatField(dst, `,"newBound":`, s.NewBound)
	dst = appendVIPROptRatField(dst, `,"val":`, s.Val)
	dst = appendVIPROptRatField(dst, `,"delta":`, s.Delta)
	dst = appendVIPROptIntSliceField(dst, `,"colPerm":`, s.ColPerm)
	dst = appendVIPROptIntSliceField(dst, `,"rowPerm":`, s.RowPerm)
	return append(dst, '}')
}

func appendVIPRStepJSON(dst []byte, s viprStepJSON) []byte {
	dst = append(dst, `{"rule":`...)
	dst = appendVIPRQuoted(dst, string(s.Rule))
	dst = appendVIPROptIntField(dst, `,"row":`, s.Row)
	dst = appendVIPROptIntField(dst, `,"col":`, s.Col)
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
	dst = appendVIPROptIntSliceField(dst, `,"boundAsms":`, s.BoundAsms)
	dst = appendVIPROptIntField(dst, `,"ref":`, s.Ref)
	if len(s.SplitVal) > 0 {
		dst = append(dst, `,"splitVal":`...)
		dst = append(dst, s.SplitVal...)
	}
	dst = appendVIPROptIntField(dst, `,"leftAsm":`, s.LeftAsm)
	dst = appendVIPROptIntField(dst, `,"rightAsm":`, s.RightAsm)
	dst = appendVIPROptIntField(dst, `,"left":`, s.Left)
	dst = appendVIPROptIntField(dst, `,"right":`, s.Right)
	dst = appendVIPROptIntSliceField(dst, `,"orbitAsms":`, s.OrbitAsms)
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
			dst = appendVIPROptIntSliceField(dst, `,"supportEntries":`, w.SupportEntries)
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	dst = appendVIPROptIntSliceField(dst, `,"extraAsms":`, s.ExtraAsms)
	dst = appendVIPROptIntField(dst, `,"lbRef":`, s.LBRef)
	dst = appendVIPROptIntField(dst, `,"ubRef":`, s.UBRef)
	dst = appendVIPROptIntSliceField(dst, `,"assumptions":`, s.Assumptions)
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
	dst = appendVIPRQuoted(dst, string(c.Mode))
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
