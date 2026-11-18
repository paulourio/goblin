package format

import (
	"strconv"
	"unicode/utf8"
)

type pulpScanner struct {
	b   []byte
	s   string
	pos int
}

func (sc *pulpScanner) skipWS() {
	b := sc.b
	i := sc.pos
	for i < len(b) {
		c := b[i]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		i++
	}
	sc.pos = i
}

func (sc *pulpScanner) consumeByte(want byte) bool {
	sc.skipWS()
	if sc.pos < len(sc.b) && sc.b[sc.pos] == want {
		sc.pos++
		return true
	}
	return false
}

func (sc *pulpScanner) consumeNull() bool {
	sc.skipWS()
	i := sc.pos
	b := sc.b
	if i+4 <= len(b) && b[i] == 'n' && b[i+1] == 'u' && b[i+2] == 'l' && b[i+3] == 'l' {
		sc.pos = i + 4
		return true
	}
	return false
}

// readUnescapedString reads a JSON string containing no backslash escapes or control bytes.
// Any escape sequence returns ok=false so the caller falls back to encoding/json.
func (sc *pulpScanner) readUnescapedString() (string, bool) {
	sc.skipWS()
	b := sc.b
	i := sc.pos
	if i >= len(b) || b[i] != '"' {
		return "", false
	}
	i++
	start := i
	hasHighBit := false
	for i < len(b) {
		c := b[i]
		if c == '"' {
			str := sc.s[start:i]
			if hasHighBit && !utf8.ValidString(str) {
				return "", false
			}
			sc.pos = i + 1
			return str, true
		}
		if c == '\\' || c < 0x20 {
			return "", false
		}
		if c >= 0x80 {
			hasHighBit = true
		}
		i++
	}
	return "", false
}

func (sc *pulpScanner) readOptString() (*string, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	str, ok := sc.readUnescapedString()
	if !ok {
		return nil, false
	}
	return &str, true
}

func (sc *pulpScanner) readInt() (int, bool) {
	sc.skipWS()
	b := sc.b
	i := sc.pos
	if i >= len(b) {
		return 0, false
	}
	neg := false
	if b[i] == '-' {
		neg = true
		i++
		if i >= len(b) {
			return 0, false
		}
	}
	if b[i] == '0' {
		i++
		if i < len(b) && ((b[i] >= '0' && b[i] <= '9') || b[i] == '.' || b[i] == 'e' || b[i] == 'E') {
			return 0, false
		}
		sc.pos = i
		return 0, true
	}
	if b[i] < '1' || b[i] > '9' {
		return 0, false
	}
	var u uint64
	digits := 0
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		u = u*10 + uint64(b[i]-'0')
		i++
		digits++
		if digits > 18 {
			return 0, false
		}
	}
	if i < len(b) && (b[i] == '.' || b[i] == 'e' || b[i] == 'E') {
		return 0, false
	}
	sc.pos = i
	if neg {
		return -int(u), true
	}
	return int(u), true
}

func (sc *pulpScanner) readFloat() (float64, bool) {
	sc.skipWS()
	b := sc.b
	i := sc.pos
	n := len(b)
	if i >= n {
		return 0, false
	}
	numStart := i
	neg := false
	if b[i] == '-' {
		neg = true
		i++
		if i >= n {
			return 0, false
		}
	}
	if b[i] == '0' {
		i++
		if i >= n || (b[i] != '.' && b[i] != 'e' && b[i] != 'E' && (b[i] < '0' || b[i] > '9')) {
			sc.pos = i
			return 0, true
		}
		if b[i] >= '0' && b[i] <= '9' {
			return 0, false
		}
	} else if b[i] >= '1' && b[i] <= '9' {
		var u uint64
		digits := 0
		for i < n && b[i] >= '0' && b[i] <= '9' {
			if digits < 15 {
				u = u*10 + uint64(b[i]-'0')
			}
			i++
			digits++
		}
		if digits <= 15 && (i >= n || (b[i] != '.' && b[i] != 'e' && b[i] != 'E')) {
			sc.pos = i
			v := float64(u)
			if neg {
				return -v, true
			}
			return v, true
		}
	} else {
		return 0, false
	}

	if i < n && b[i] == '.' {
		i++
		if i >= n || b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		for i < n && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	if i < n && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < n && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= n || b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		for i < n && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	v, err := strconv.ParseFloat(sc.s[numStart:i], 64)
	if err != nil {
		return 0, false
	}
	sc.pos = i
	return v, true
}

func allocFloatSlab(slab *[]float64, v float64) *float64 {
	s := *slab
	if len(s) == cap(s) {
		s = make([]float64, 0, max(64, 2*cap(s)))
	}
	s = append(s, v)
	*slab = s
	return &s[len(s)-1]
}

func (sc *pulpScanner) readOptFloat(slab *[]float64) (*float64, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	v, ok := sc.readFloat()
	if !ok {
		return nil, false
	}
	return allocFloatSlab(slab, v), true
}

func (sc *pulpScanner) readCoefficients(arena *[]PuLPCoefficient) ([]PuLPCoefficient, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	startIdx := len(*arena)
	if sc.consumeByte(']') {
		return (*arena)[startIdx:startIdx:startIdx], true
	}
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var coeff PuLPCoefficient
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedString()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				switch key {
				case "name":
					coeff.Name, ok = sc.readUnescapedString()
				case "value":
					coeff.Value, ok = sc.readFloat()
				default:
					return nil, false
				}
				if !ok {
					return nil, false
				}
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		*arena = append(*arena, coeff)
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			endIdx := len(*arena)
			return (*arena)[startIdx:endIdx:endIdx], true
		}
		return nil, false
	}
}

func (sc *pulpScanner) readParameters(dst *PuLPParameters) bool {
	if !sc.consumeByte('{') {
		return false
	}
	if sc.consumeByte('}') {
		return true
	}
	for {
		key, ok := sc.readUnescapedString()
		if !ok || !sc.consumeByte(':') {
			return false
		}
		switch key {
		case "name":
			dst.Name, ok = sc.readUnescapedString()
		case "sense":
			dst.Sense, ok = sc.readInt()
		case "status":
			dst.Status, ok = sc.readInt()
		case "sol_status":
			dst.SolStatus, ok = sc.readInt()
		default:
			return false
		}
		if !ok {
			return false
		}
		if sc.consumeByte(',') {
			continue
		}
		return sc.consumeByte('}')
	}
}

func (sc *pulpScanner) readObjective(dst *PuLPObjective, arena *[]PuLPCoefficient) bool {
	if !sc.consumeByte('{') {
		return false
	}
	if sc.consumeByte('}') {
		return true
	}
	for {
		key, ok := sc.readUnescapedString()
		if !ok || !sc.consumeByte(':') {
			return false
		}
		switch key {
		case "name":
			dst.Name, ok = sc.readOptString()
		case "coefficients":
			dst.Coefficients, ok = sc.readCoefficients(arena)
		case "constant":
			dst.Constant, ok = sc.readFloat()
		default:
			return false
		}
		if !ok {
			return false
		}
		if sc.consumeByte(',') {
			continue
		}
		return sc.consumeByte('}')
	}
}

func (sc *pulpScanner) readVariables(slab *[]float64) ([]PuLPVariable, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return []PuLPVariable{}, true
	}
	vars := make([]PuLPVariable, 0, max(8, len(sc.b)/256))
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var v PuLPVariable
		seenLB := false
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedString()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				switch key {
				case "name":
					v.Name, ok = sc.readUnescapedString()
				case "cat":
					v.Cat, ok = sc.readUnescapedString()
				case "lowBound":
					seenLB = true
					v.LowBound, ok = sc.readOptFloat(slab)
				case "upBound":
					v.UpBound, ok = sc.readOptFloat(slab)
				case "varValue":
					v.VarValue, ok = sc.readOptFloat(slab)
				case "dj":
					v.Dj, ok = sc.readOptFloat(slab)
				default:
					return nil, false
				}
				if !ok {
					return nil, false
				}
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		if !seenLB {
			v.LowBound = allocFloatSlab(slab, 0)
		}
		vars = append(vars, v)
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			return vars, true
		}
		return nil, false
	}
}

func (sc *pulpScanner) readConstraints(arena *[]PuLPCoefficient, slab *[]float64) ([]PuLPConstraint, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return []PuLPConstraint{}, true
	}
	cons := make([]PuLPConstraint, 0, max(8, len(sc.b)/512))
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var c PuLPConstraint
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedString()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				switch key {
				case "name":
					c.Name, ok = sc.readUnescapedString()
				case "sense":
					c.Sense, ok = sc.readInt()
				case "coefficients":
					c.Coefficients, ok = sc.readCoefficients(arena)
				case "pi":
					c.Pi, ok = sc.readOptFloat(slab)
				case "constant":
					c.Constant, ok = sc.readFloat()
				default:
					return nil, false
				}
				if !ok {
					return nil, false
				}
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		cons = append(cons, c)
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			return cons, true
		}
		return nil, false
	}
}

func (sc *pulpScanner) readEmptyArray() ([]any, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if !sc.consumeByte(']') {
		return nil, false
	}
	return []any{}, true
}

// parsePuLPJSONFast decodes standard unescaped PuLP JSON payloads without reflection,
// backing all coefficient slices and float pointers with two contiguous arenas.
// Any escape sequence, unknown field, non-empty SOS list, or syntax error returns ok=false
// so ParsePuLPJSON falls back to encoding/json.
func parsePuLPJSONFast(data []byte) (*PuLPJSON, bool) {
	sc := pulpScanner{
		b: data,
		s: string(data),
	}
	if !sc.consumeByte('{') {
		return nil, false
	}
	var doc PuLPJSON
	coeffArena := make([]PuLPCoefficient, 0, max(16, len(data)/28))
	floatSlab := make([]float64, 0, max(32, len(data)/64))
	if !sc.consumeByte('}') {
		for {
			key, ok := sc.readUnescapedString()
			if !ok || !sc.consumeByte(':') {
				return nil, false
			}
			switch key {
			case "parameters":
				ok = sc.readParameters(&doc.Parameters)
			case "objective":
				ok = sc.readObjective(&doc.Objective, &coeffArena)
			case "variables":
				doc.Variables, ok = sc.readVariables(&floatSlab)
			case "constraints":
				doc.Constraints, ok = sc.readConstraints(&coeffArena, &floatSlab)
			case "sos1":
				doc.SOS1, ok = sc.readEmptyArray()
			case "sos2":
				doc.SOS2, ok = sc.readEmptyArray()
			default:
				return nil, false
			}
			if !ok {
				return nil, false
			}
			if sc.consumeByte(',') {
				continue
			}
			if sc.consumeByte('}') {
				break
			}
			return nil, false
		}
	}
	sc.skipWS()
	if sc.pos != len(sc.b) {
		return nil, false
	}
	return &doc, true
}
