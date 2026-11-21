package format

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	gb "github.com/paulourio/goblin"
)

const (
	eightSpaces       = 0x2020202020202020
	fourSpaces        = 0x20202020
	maxExactFloatInt  = (1 << 53) - 1
	maxPoolVarHashCap = 32 * 1024
	maxPoolEndsCap    = 16 * 1024
)

var plainStrChar = func() [256]bool {
	var t [256]bool
	for c := 0x20; c < 0x80; c++ {
		if c != '"' && c != '\\' {
			t[c] = true
		}
	}
	return t
}()

var pow10Table = [23]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7,
	1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15,
	1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22,
}

type rawObjCoeff struct {
	start int
	end   int
	val   float64
}

type fastVarMeta struct {
	lb  float64
	ub  float64
	cat gb.VarCategory
}

type pulpFastScratch struct {
	varHash  []int32
	ends     []int
	terms    []gb.Term
	varIDs   []gb.Var
	varNames []string
	varMeta  []fastVarMeta
	objSpans []rawObjCoeff
}

var pulpScratchPool = sync.Pool{
	New: func() any {
		return &pulpFastScratch{}
	},
}

type pulpScanner struct {
	b        []byte
	pos      int
	indented bool
	vars     []PuLPVariable
	varHash  []int32
	ends     []int
}

func detectIndented(data []byte) bool {
	limit := min(len(data), 64)
	for i := range limit {
		if data[i] == '\n' {
			return true
		}
	}
	return false
}

func cloneBytesToString(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func internVarCat(raw []byte) string {
	switch string(raw) {
	case "Continuous":
		return "Continuous"
	case "Integer":
		return "Integer"
	case "Binary":
		return "Binary"
	default:
		return cloneBytesToString(raw)
	}
}

func hashBytes(b []byte) int {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for _, c := range b {
		h ^= uint32(c)
		h *= prime32
	}
	return int(h & 0x7fffffff)
}

func hashString(s string) int {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= prime32
	}
	return int(h & 0x7fffffff)
}

func buildVarHash(buf []int32, vars []PuLPVariable) []int32 {
	if len(vars) == 0 || len(vars) >= math.MaxInt32/2 {
		return nil
	}
	size := 16
	target := len(vars) * 2
	for size < target {
		size <<= 1
	}
	var table []int32
	if cap(buf) >= size {
		table = buf[:size]
		clear(table)
	} else {
		table = make([]int32, size)
	}
	mask := size - 1
	for i := range vars {
		name := vars[i].Name
		if name == "" {
			continue
		}
		slot := hashString(name) & mask
		for {
			cur := table[slot]
			if cur == 0 {
				table[slot] = int32(i + 1)
				break
			}
			if vars[cur-1].Name == name {
				break
			}
			slot = (slot + 1) & mask
		}
	}
	return table
}

func (sc *pulpScanner) resetEnds(wantCap int) []int {
	if cap(sc.ends) < wantCap {
		sc.ends = make([]int, 0, wantCap)
	}
	return sc.ends[:0]
}

func (sc *pulpScanner) lookupVarName(raw []byte) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	table := sc.varHash
	if len(table) == 0 {
		return "", false
	}
	vars := sc.vars
	mask := len(table) - 1
	slot := hashBytes(raw) & mask
	for {
		cur := table[slot]
		if cur == 0 {
			return "", false
		}
		name := vars[cur-1].Name
		if name == string(raw) {
			return name, true
		}
		slot = (slot + 1) & mask
	}
}

func (sc *pulpScanner) skipWS() {
	if sc.pos < len(sc.b) && sc.b[sc.pos] > ' ' {
		return
	}
	sc.skipWSSlow()
}

func (sc *pulpScanner) skipWSSlow() {
	b := sc.b
	i := sc.pos
	n := len(b)
	for i < n {
		c := b[i]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		i++
		if i < n && b[i] > ' ' {
			sc.pos = i
			return
		}
		for i+8 <= n && binary.LittleEndian.Uint64(b[i:i+8]) == eightSpaces {
			i += 8
		}
		if i+4 <= n && binary.LittleEndian.Uint32(b[i:i+4]) == fourSpaces {
			i += 4
		}
		for i < n && b[i] == ' ' {
			i++
		}
		if i < n && b[i] > ' ' {
			sc.pos = i
			return
		}
	}
	sc.pos = i
}

func (sc *pulpScanner) consumeByte(want byte) bool {
	if sc.pos < len(sc.b) && sc.b[sc.pos] == want {
		sc.pos++
		return true
	}
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

// readUnescapedSpan reads a JSON string containing no backslash escapes or control bytes
// and returns its unquoted byte range [start, end) within sc.b.
func (sc *pulpScanner) readUnescapedSpan() (start, end int, ok bool) {
	sc.skipWS()
	b := sc.b
	i := sc.pos
	n := len(b)
	if i >= n || b[i] != '"' {
		return 0, 0, false
	}
	i++
	start = i
	for i < n && plainStrChar[b[i]] {
		i++
	}
	if i < n && b[i] == '"' {
		sc.pos = i + 1
		return start, i, true
	}
	for i < n {
		c := b[i]
		if c == '"' {
			if !utf8.Valid(b[start:i]) {
				return 0, 0, false
			}
			sc.pos = i + 1
			return start, i, true
		}
		if c == '\\' || c < 0x20 {
			return 0, 0, false
		}
		i++
	}
	return 0, 0, false
}

// readUnescapedBytes reads a JSON string containing no backslash escapes or control bytes.
// Any escape sequence or invalid UTF-8 returns ok=false so the caller falls back to encoding/json/v2.
func (sc *pulpScanner) readUnescapedBytes() ([]byte, bool) {
	start, end, ok := sc.readUnescapedSpan()
	if !ok {
		return nil, false
	}
	return sc.b[start:end], true
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
	limit := uint64(math.MaxInt)
	if neg {
		limit = uint64(math.MaxInt) + 1
	}
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		d := uint64(b[i] - '0')
		if u > (limit-d)/10 {
			return 0, false
		}
		u = u*10 + d
		i++
	}
	if i < len(b) && (b[i] == '.' || b[i] == 'e' || b[i] == 'E') {
		return 0, false
	}
	sc.pos = i
	if neg {
		if u == uint64(math.MaxInt)+1 {
			return math.MinInt, true
		}
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
	var u uint64
	exact := true
	if b[i] == '0' {
		i++
		if i >= n || (b[i] != '.' && b[i] != 'e' && b[i] != 'E' && (b[i] < '0' || b[i] > '9')) {
			sc.pos = i
			if neg {
				return math.Copysign(0, -1), true
			}
			return 0, true
		}
		if b[i] >= '0' && b[i] <= '9' {
			return 0, false
		}
	} else if b[i] >= '1' && b[i] <= '9' {
		for i < n && b[i] >= '0' && b[i] <= '9' {
			d := uint64(b[i] - '0')
			if exact {
				if u <= (maxExactFloatInt-d)/10 {
					u = u*10 + d
				} else {
					exact = false
				}
			}
			i++
		}
		if exact && (i >= n || (b[i] != '.' && b[i] != 'e' && b[i] != 'E')) {
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
		fracDigits := 0
		for i < n && b[i] >= '0' && b[i] <= '9' {
			d := uint64(b[i] - '0')
			if exact {
				if u <= (maxExactFloatInt-d)/10 {
					u = u*10 + d
				} else {
					exact = false
				}
			}
			fracDigits++
			i++
		}
		if exact && fracDigits < len(pow10Table) && (i >= n || (b[i] != 'e' && b[i] != 'E')) {
			sc.pos = i
			if u == 0 {
				if neg {
					return math.Copysign(0, -1), true
				}
				return 0, true
			}
			v := float64(u) / pow10Table[fracDigits]
			if neg {
				return -v, true
			}
			return v, true
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
	v, err := strconv.ParseFloat(string(b[numStart:i]), 64)
	if err != nil {
		return 0, false
	}
	sc.pos = i
	return v, true
}

func allocFloatSlab(slab *[]float64, v float64) *float64 {
	s := *slab
	if len(s) == cap(s) {
		s = make([]float64, 0, max(16, 2*cap(s)))
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

func (sc *pulpScanner) consumePythonNonFinite() bool {
	sc.skipWS()
	rem := sc.b[sc.pos:]
	var tokLen int
	switch {
	case len(rem) >= 3 && string(rem[:3]) == "NaN":
		tokLen = 3
	case len(rem) >= 8 && string(rem[:8]) == "Infinity":
		tokLen = 8
	case len(rem) >= 9 && string(rem[:9]) == "-Infinity":
		tokLen = 9
	default:
		return false
	}
	if sc.pos+tokLen < len(sc.b) {
		next := sc.b[sc.pos+tokLen]
		if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	sc.pos += tokLen
	return true
}

func (sc *pulpScanner) readIgnoredMetricFloat(slab *[]float64) (*float64, bool) {
	if sc.consumeNull() || sc.consumePythonNonFinite() {
		return nil, true
	}
	v, ok := sc.readFloat()
	if !ok {
		return nil, false
	}
	return allocFloatSlab(slab, v), true
}

func (sc *pulpScanner) readCoefficients(
	arena *[]PuLPCoefficient,
	fallbackSB *strings.Builder,
	fallbackEnds *[]int,
) ([]PuLPCoefficient, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return []PuLPCoefficient{}, true
	}
	startIdx := len(*arena)
	useVarHash := len(sc.varHash) > 0
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var coeff PuLPCoefficient
		var nameRaw []byte
		var seen uint8
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedBytes()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				var bit uint8
				switch string(key) {
				case "name":
					bit = 1 << 0
					nameRaw, ok = sc.readUnescapedBytes()
				case "value":
					bit = 1 << 1
					coeff.Value, ok = sc.readFloat()
				default:
					return nil, false
				}
				if !ok || seen&bit != 0 {
					return nil, false
				}
				seen |= bit
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		if useVarHash {
			if interned, hit := sc.lookupVarName(nameRaw); hit {
				coeff.Name = interned
			} else {
				coeff.Name = cloneBytesToString(nameRaw)
			}
		} else {
			_, _ = fallbackSB.Write(nameRaw)
			*fallbackEnds = append(*fallbackEnds, fallbackSB.Len())
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
	var seen uint8
	for {
		key, ok := sc.readUnescapedBytes()
		if !ok || !sc.consumeByte(':') {
			return false
		}
		var bit uint8
		switch string(key) {
		case "name":
			bit = 1 << 0
			var raw []byte
			raw, ok = sc.readUnescapedBytes()
			if ok {
				dst.Name = cloneBytesToString(raw)
			}
		case "sense":
			bit = 1 << 1
			dst.Sense, ok = sc.readInt()
		case "status":
			bit = 1 << 2
			dst.Status, ok = sc.readInt()
		case "sol_status":
			bit = 1 << 3
			dst.SolStatus, ok = sc.readInt()
		default:
			return false
		}
		if !ok || seen&bit != 0 {
			return false
		}
		seen |= bit
		if sc.consumeByte(',') {
			continue
		}
		return sc.consumeByte('}')
	}
}

func (sc *pulpScanner) readObjective(dst *PuLPObjective) bool {
	if !sc.consumeByte('{') {
		return false
	}
	if sc.consumeByte('}') {
		return true
	}
	var (
		objSB        strings.Builder
		hasObjName   bool
		objNameStart int
		objNameEnd   int
		coeffBase    int
		objCoeffEnds []int
		seen         uint8
	)
	for {
		key, ok := sc.readUnescapedBytes()
		if !ok || !sc.consumeByte(':') {
			return false
		}
		var bit uint8
		switch string(key) {
		case "name":
			bit = 1 << 0
			if sc.consumeNull() {
				dst.Name = nil
				ok = true
			} else {
				var raw []byte
				raw, ok = sc.readUnescapedBytes()
				if ok {
					hasObjName = true
					objNameStart = objSB.Len()
					_, _ = objSB.Write(raw)
					objNameEnd = objSB.Len()
				}
			}
		case "coefficients":
			bit = 1 << 1
			coeffBase = objSB.Len()
			var objCoeffs []PuLPCoefficient
			if sc.consumeNull() {
				dst.Coefficients = nil
				ok = true
			} else if sc.consumeByte('[') {
				bracketPos := sc.pos - 1
				if sc.consumeByte(']') {
					dst.Coefficients = []PuLPCoefficient{}
					ok = true
				} else {
					sc.pos = bracketPos
					objDiv := 320
					if !sc.indented {
						objDiv = 160
					}
					estObj := max(8, min(len(sc.b)/objDiv, 1024))
					objCoeffs = make([]PuLPCoefficient, 0, estObj)
					if len(sc.varHash) == 0 {
						objCoeffEnds = sc.resetEnds(estObj)
						objSB.Grow(estObj * 7)
					}
					dst.Coefficients, ok = sc.readCoefficients(&objCoeffs, &objSB, &objCoeffEnds)
				}
			} else {
				ok = false
			}
		case "constant":
			bit = 1 << 2
			dst.Constant, ok = sc.readFloat()
		default:
			return false
		}
		if !ok || seen&bit != 0 {
			return false
		}
		seen |= bit
		if sc.consumeByte(',') {
			continue
		}
		if !sc.consumeByte('}') {
			return false
		}
		break
	}
	if hasObjName || len(objCoeffEnds) > 0 {
		all := objSB.String()
		if hasObjName {
			s := all[objNameStart:objNameEnd]
			dst.Name = &s
		}
		if len(objCoeffEnds) > 0 {
			off := coeffBase
			for i, end := range objCoeffEnds {
				dst.Coefficients[i].Name = all[off:end]
				off = end
			}
		}
	}
	if cap(objCoeffEnds) > cap(sc.ends) {
		sc.ends = objCoeffEnds[:0]
	}
	return true
}

func (sc *pulpScanner) readVariables(slab *[]float64, objCoeffHint int, hashBuf []int32) ([]PuLPVariable, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return []PuLPVariable{}, true
	}
	rem := len(sc.b) - sc.pos
	varDiv := 1024
	minVarBytes := 80
	if !sc.indented {
		varDiv = 512
		minVarBytes = 40
	}
	estVars := max(8, rem/varDiv)
	if objHint := min(objCoeffHint, rem/minVarBytes); objHint > estVars {
		estVars = objHint
	}
	vars := make([]PuLPVariable, 0, estVars)
	if cap(*slab) == 0 {
		*slab = make([]float64, 0, max(16, estVars*2))
	}
	var varSB strings.Builder
	varSB.Grow(min(rem/4, estVars*8))
	varEnds := sc.resetEnds(estVars)
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var v PuLPVariable
		var nameRaw []byte
		var seen uint8
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedBytes()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				var bit uint8
				switch string(key) {
				case "name":
					bit = 1 << 0
					nameRaw, ok = sc.readUnescapedBytes()
				case "cat":
					bit = 1 << 1
					var raw []byte
					raw, ok = sc.readUnescapedBytes()
					if ok {
						v.Cat = internVarCat(raw)
					}
				case "lowBound":
					bit = 1 << 2
					v.LowBound, ok = sc.readOptFloat(slab)
				case "upBound":
					bit = 1 << 3
					v.UpBound, ok = sc.readOptFloat(slab)
				case "varValue":
					bit = 1 << 4
					v.VarValue, ok = sc.readIgnoredMetricFloat(slab)
				case "dj":
					bit = 1 << 5
					v.Dj, ok = sc.readIgnoredMetricFloat(slab)
				default:
					return nil, false
				}
				if !ok || seen&bit != 0 {
					return nil, false
				}
				seen |= bit
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		_, _ = varSB.Write(nameRaw)
		varEnds = append(varEnds, varSB.Len())
		if seen&(1<<2) == 0 {
			v.LowBound = allocFloatSlab(slab, 0)
		}
		vars = append(vars, v)
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			varNames := varSB.String()
			off := 0
			for i, end := range varEnds {
				vars[i].Name = varNames[off:end]
				off = end
			}
			sc.ends = varEnds[:0]
			sc.vars = vars
			sc.varHash = buildVarHash(hashBuf, vars)
			return vars, true
		}
		return nil, false
	}
}

func (sc *pulpScanner) readConstraints(slab *[]float64) ([]PuLPConstraint, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return []PuLPConstraint{}, true
	}
	rem := len(sc.b) - sc.pos
	conDiv := 1536
	coeffDiv := 76
	if !sc.indented {
		conDiv = 768
		coeffDiv = 32
	}
	estCons := max(8, rem/conDiv)
	estCoeffs := max(8, rem/coeffDiv)
	cons := make([]PuLPConstraint, 0, estCons)
	coeffArena := make([]PuLPCoefficient, 0, estCoeffs)
	var conSB strings.Builder
	conSB.Grow(min(rem/8, estCons*8))
	conNameEnds := sc.resetEnds(estCons)
	var conCoeffSB strings.Builder
	var conCoeffEnds []int
	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var c PuLPConstraint
		var nameRaw []byte
		var seen uint8
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedBytes()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				var bit uint8
				switch string(key) {
				case "name":
					bit = 1 << 0
					nameRaw, ok = sc.readUnescapedBytes()
				case "sense":
					bit = 1 << 1
					c.Sense, ok = sc.readInt()
				case "coefficients":
					bit = 1 << 2
					c.Coefficients, ok = sc.readCoefficients(&coeffArena, &conCoeffSB, &conCoeffEnds)
				case "pi":
					bit = 1 << 3
					c.Pi, ok = sc.readIgnoredMetricFloat(slab)
				case "constant":
					bit = 1 << 4
					c.Constant, ok = sc.readFloat()
				default:
					return nil, false
				}
				if !ok || seen&bit != 0 {
					return nil, false
				}
				seen |= bit
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		_, _ = conSB.Write(nameRaw)
		conNameEnds = append(conNameEnds, conSB.Len())
		cons = append(cons, c)
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			conNames := conSB.String()
			off := 0
			for i, end := range conNameEnds {
				cons[i].Name = conNames[off:end]
				off = end
			}
			sc.ends = conNameEnds[:0]
			if len(conCoeffEnds) > 0 {
				allCoeffNames := conCoeffSB.String()
				coeffOff := 0
				idx := 0
				for i := range cons {
					coeffs := cons[i].Coefficients
					for k := range coeffs {
						end := conCoeffEnds[idx]
						coeffs[k].Name = allCoeffNames[coeffOff:end]
						coeffOff = end
						idx++
					}
				}
			}
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

// parsePuLPJSONFast decodes standard unescaped PuLP JSON payloads without reflection
// and without pinning the raw input slice in live heap memory. Surviving names are
// packed into compact string arenas and variable categories are interned to constants.
// Any escape sequence, unknown field, duplicate key, non-empty SOS list, or syntax error
// returns ok=false so ParsePuLPJSON falls back to encoding/json/v2.
func parsePuLPJSONFast(data []byte) (*PuLPJSON, bool) {
	scratch, _ := pulpScratchPool.Get().(*pulpFastScratch)
	if scratch == nil {
		scratch = &pulpFastScratch{}
	}
	sc := pulpScanner{
		b:        data,
		indented: detectIndented(data),
		ends:     scratch.ends[:0],
	}
	hashBuf := scratch.varHash
	defer func() {
		if cap(sc.varHash) > cap(hashBuf) {
			hashBuf = sc.varHash
		}
		if cap(hashBuf) <= maxPoolVarHashCap {
			scratch.varHash = hashBuf[:0]
		} else {
			scratch.varHash = nil
		}
		if cap(sc.ends) <= maxPoolEndsCap {
			scratch.ends = sc.ends[:0]
		} else {
			scratch.ends = nil
		}
		pulpScratchPool.Put(scratch)
	}()

	if !sc.consumeByte('{') {
		return nil, false
	}
	var doc PuLPJSON
	var floatSlab []float64
	var seen uint8
	if !sc.consumeByte('}') {
		for {
			key, ok := sc.readUnescapedBytes()
			if !ok || !sc.consumeByte(':') {
				return nil, false
			}
			var bit uint8
			switch string(key) {
			case "parameters":
				bit = 1 << 0
				ok = sc.readParameters(&doc.Parameters)
			case "objective":
				bit = 1 << 1
				ok = sc.readObjective(&doc.Objective)
			case "variables":
				bit = 1 << 2
				doc.Variables, ok = sc.readVariables(&floatSlab, len(doc.Objective.Coefficients), hashBuf)
			case "constraints":
				bit = 1 << 3
				doc.Constraints, ok = sc.readConstraints(&floatSlab)
			case "sos1":
				bit = 1 << 4
				doc.SOS1, ok = sc.readEmptyArray()
			case "sos2":
				bit = 1 << 5
				doc.SOS2, ok = sc.readEmptyArray()
			default:
				return nil, false
			}
			if !ok || seen&bit != 0 {
				return nil, false
			}
			seen |= bit
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

func (sc *pulpScanner) skipIgnoredMetricFloat() bool {
	if sc.consumeNull() || sc.consumePythonNonFinite() {
		return true
	}
	_, ok := sc.readFloat()
	return ok
}

func insertStreamVarHash(table []int32, varNames []string, idx int, name string) bool {
	if idx < 0 || idx >= math.MaxInt32 {
		return false
	}
	mask := len(table) - 1
	slot := hashString(name) & mask
	for {
		cur := table[slot]
		if cur == 0 {
			table[slot] = int32(idx + 1)
			return true
		}
		if varNames[cur-1] == name {
			return false
		}
		slot = (slot + 1) & mask
	}
}

func lookupStreamVarID(table []int32, varNames []string, varIDs []gb.Var, raw []byte) (gb.Var, bool) {
	if len(table) == 0 || len(raw) == 0 {
		return 0, false
	}
	mask := len(table) - 1
	slot := hashBytes(raw) & mask
	for {
		cur := table[slot]
		if cur == 0 {
			return 0, false
		}
		if varNames[cur-1] == string(raw) {
			return varIDs[cur-1], true
		}
		slot = (slot + 1) & mask
	}
}

func parseStreamCoeff(sc *pulpScanner) (start, end int, val float64, ok bool) {
	if !sc.consumeByte('{') {
		return 0, 0, 0, false
	}
	if sc.consumeByte('}') {
		return 0, 0, 0, true
	}
	var seen uint8
	for {
		key, keyOK := sc.readUnescapedBytes()
		if !keyOK || !sc.consumeByte(':') {
			return 0, 0, 0, false
		}
		var bit uint8
		switch string(key) {
		case "name":
			bit = 1 << 0
			start, end, keyOK = sc.readUnescapedSpan()
		case "value":
			bit = 1 << 1
			val, keyOK = sc.readFloat()
		default:
			return 0, 0, 0, false
		}
		if !keyOK || seen&bit != 0 {
			return 0, 0, 0, false
		}
		seen |= bit
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte('}') {
			return start, end, val, true
		}
		return 0, 0, 0, false
	}
}

func streamObjectiveSection(
	sc *pulpScanner,
	m *gb.Model,
	varsReady bool,
	table []int32,
	varNames []string,
	varIDs []gb.Var,
	termsBuf *[]gb.Term,
	spansBuf *[]rawObjCoeff,
) (objName string, objConst float64, hasDeferred bool, ok bool) {
	if !sc.consumeByte('{') {
		return "", 0, false, false
	}
	if sc.consumeByte('}') {
		return "", 0, false, true
	}
	spans := (*spansBuf)[:0]
	terms := (*termsBuf)[:0]
	var seen uint8
	for {
		key, keyOK := sc.readUnescapedBytes()
		if !keyOK || !sc.consumeByte(':') {
			return "", 0, false, false
		}
		var bit uint8
		switch string(key) {
		case "name":
			bit = 1 << 0
			if sc.consumeNull() {
				objName = ""
				keyOK = true
			} else {
				var raw []byte
				raw, keyOK = sc.readUnescapedBytes()
				if keyOK {
					objName = cloneBytesToString(raw)
				}
			}
		case "coefficients":
			bit = 1 << 1
			if sc.consumeNull() {
				keyOK = true
			} else if !sc.consumeByte('[') {
				keyOK = false
			} else if sc.consumeByte(']') {
				keyOK = true
			} else {
				for {
					s, e, val, cOK := parseStreamCoeff(sc)
					if !cOK {
						keyOK = false
						break
					}
					if varsReady {
						vid, hit := lookupStreamVarID(table, varNames, varIDs, sc.b[s:e])
						if !hit {
							keyOK = false
							break
						}
						terms = append(terms, gb.Term{Var: vid, Coeff: val})
					} else {
						spans = append(spans, rawObjCoeff{start: s, end: e, val: val})
					}
					if sc.consumeByte(',') {
						continue
					}
					keyOK = sc.consumeByte(']')
					break
				}
			}
		case "constant":
			bit = 1 << 2
			objConst, keyOK = sc.readFloat()
		default:
			return "", 0, false, false
		}
		if !keyOK || seen&bit != 0 {
			return "", 0, false, false
		}
		seen |= bit
		if sc.consumeByte(',') {
			continue
		}
		if !sc.consumeByte('}') {
			return "", 0, false, false
		}
		break
	}
	*spansBuf = spans
	*termsBuf = terms
	if varsReady {
		m.SetObjectiveNamed(objName, gb.Expr{Terms: terms, Constant: objConst})
		return objName, objConst, false, true
	}
	return objName, objConst, true, true
}

func streamVariablesSection(
	sc *pulpScanner,
	m *gb.Model,
	hashBuf *[]int32,
	metaBuf *[]fastVarMeta,
	namesBuf *[]string,
	idsBuf *[]gb.Var,
) ([]int32, bool) {
	if sc.consumeNull() {
		return nil, true
	}
	if !sc.consumeByte('[') {
		return nil, false
	}
	if sc.consumeByte(']') {
		return nil, true
	}
	rem := len(sc.b) - sc.pos
	varDiv := 1024
	conDiv := 1536
	if !sc.indented {
		varDiv = 512
		conDiv = 768
	}
	estVars := max(8, rem/varDiv)
	metas := (*metaBuf)[:0]
	if cap(metas) < estVars {
		metas = make([]fastVarMeta, 0, estVars)
	}
	var varSB strings.Builder
	varSB.Grow(min(rem/4, estVars*8))
	varEnds := sc.resetEnds(estVars)

	for {
		if !sc.consumeByte('{') {
			return nil, false
		}
		var (
			nameRaw           []byte
			catRaw            []byte
			lb                float64
			ub                = math.Inf(1)
			hasExplicitNullLB bool
			hasExplicitUB     bool
			seen              uint8
		)
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedBytes()
				if !ok || !sc.consumeByte(':') {
					return nil, false
				}
				var bit uint8
				switch string(key) {
				case "name":
					bit = 1 << 0
					nameRaw, ok = sc.readUnescapedBytes()
				case "cat":
					bit = 1 << 1
					catRaw, ok = sc.readUnescapedBytes()
				case "lowBound":
					bit = 1 << 2
					if sc.consumeNull() {
						hasExplicitNullLB = true
						lb = math.Inf(-1)
						ok = true
					} else {
						lb, ok = sc.readFloat()
					}
				case "upBound":
					bit = 1 << 3
					if sc.consumeNull() {
						hasExplicitUB = false
						ub = math.Inf(1)
						ok = true
					} else {
						ub, ok = sc.readFloat()
						hasExplicitUB = true
					}
				case "varValue":
					bit = 1 << 4
					ok = sc.skipIgnoredMetricFloat()
				case "dj":
					bit = 1 << 5
					ok = sc.skipIgnoredMetricFloat()
				default:
					return nil, false
				}
				if !ok || seen&bit != 0 {
					return nil, false
				}
				seen |= bit
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return nil, false
			}
		}
		if len(nameRaw) == 0 {
			return nil, false
		}
		var cat gb.VarCategory
		switch string(catRaw) {
		case "Continuous":
			cat = gb.Continuous
		case "Binary":
			if hasExplicitNullLB {
				lb = 0
			}
			if !hasExplicitUB {
				ub = 1
			}
			if lb >= 0 && ub <= 1 && lb <= ub {
				cat = gb.Binary
			} else {
				cat = gb.Integer
			}
		case "Integer":
			if lb >= 0 && ub <= 1 && lb <= ub {
				cat = gb.Binary
			} else {
				cat = gb.Integer
			}
		default:
			return nil, false
		}
		_, _ = varSB.Write(nameRaw)
		varEnds = append(varEnds, varSB.Len())
		metas = append(metas, fastVarMeta{lb: lb, ub: ub, cat: cat})
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			break
		}
		return nil, false
	}

	nVars := len(metas)
	*metaBuf = metas
	estCons := max(8, (len(sc.b)-sc.pos)/conDiv)
	m.Reserve(nVars, estCons)

	if nVars >= math.MaxInt32/2 {
		return nil, false
	}
	tableSize := 16
	for tableSize < nVars*2 {
		tableSize <<= 1
	}
	var table []int32
	if cap(*hashBuf) >= tableSize {
		table = (*hashBuf)[:tableSize]
		clear(table)
	} else {
		table = make([]int32, tableSize)
	}
	*hashBuf = table

	varNames := (*namesBuf)[:0]
	if cap(varNames) < nVars {
		varNames = make([]string, 0, nVars)
	}
	varIDs := (*idsBuf)[:0]
	if cap(varIDs) < nVars {
		varIDs = make([]gb.Var, 0, nVars)
	}

	allNames := varSB.String()
	off := 0
	for i, end := range varEnds {
		vName := allNames[off:end]
		off = end
		if !insertStreamVarHash(table, varNames, i, vName) {
			*namesBuf = varNames
			*idsBuf = varIDs
			sc.ends = varEnds[:0]
			return nil, false
		}
		meta := metas[i]
		vid := m.AddVar(vName, meta.lb, meta.ub, meta.cat)
		varNames = append(varNames, vName)
		varIDs = append(varIDs, vid)
	}
	*namesBuf = varNames
	*idsBuf = varIDs
	sc.ends = varEnds[:0]
	return table, true
}

func streamConstraintsSection(
	sc *pulpScanner,
	m *gb.Model,
	table []int32,
	varNames []string,
	varIDs []gb.Var,
	termsBuf *[]gb.Term,
) bool {
	if sc.consumeNull() {
		return true
	}
	if !sc.consumeByte('[') {
		return false
	}
	if sc.consumeByte(']') {
		return true
	}
	terms := (*termsBuf)[:0]
	for {
		if !sc.consumeByte('{') {
			return false
		}
		var (
			nameRaw  []byte
			senseInt int
			constant float64
			seen     uint8
			termsOK  = true
		)
		terms = terms[:0]
		if !sc.consumeByte('}') {
			for {
				key, ok := sc.readUnescapedBytes()
				if !ok || !sc.consumeByte(':') {
					return false
				}
				var bit uint8
				switch string(key) {
				case "name":
					bit = 1 << 0
					nameRaw, ok = sc.readUnescapedBytes()
				case "sense":
					bit = 1 << 1
					senseInt, ok = sc.readInt()
				case "coefficients":
					bit = 1 << 2
					if sc.consumeNull() {
						ok = true
					} else if !sc.consumeByte('[') {
						ok = false
					} else if sc.consumeByte(']') {
						ok = true
					} else {
						for {
							s, e, val, cOK := parseStreamCoeff(sc)
							if !cOK {
								ok = false
								break
							}
							vid, hit := lookupStreamVarID(table, varNames, varIDs, sc.b[s:e])
							if !hit {
								termsOK = false
								ok = false
								break
							}
							terms = append(terms, gb.Term{Var: vid, Coeff: val})
							if sc.consumeByte(',') {
								continue
							}
							ok = sc.consumeByte(']')
							break
						}
					}
				case "pi":
					bit = 1 << 3
					ok = sc.skipIgnoredMetricFloat()
				case "constant":
					bit = 1 << 4
					constant, ok = sc.readFloat()
				default:
					return false
				}
				if !ok || !termsOK || seen&bit != 0 {
					return false
				}
				seen |= bit
				if sc.consumeByte(',') {
					continue
				}
				if sc.consumeByte('}') {
					break
				}
				return false
			}
		}
		var csense gb.ConstraintSense
		switch senseInt {
		case -1:
			csense = gb.LessEqual
		case 0:
			csense = gb.Equal
		case 1:
			csense = gb.GreaterEqual
		default:
			return false
		}
		m.AddConstraint(cloneBytesToString(nameRaw), gb.Constraint{
			Expr:  gb.Expr{Terms: terms},
			Sense: csense,
			RHS:   -constant,
		})
		if sc.consumeByte(',') {
			continue
		}
		if sc.consumeByte(']') {
			*termsBuf = terms
			return true
		}
		return false
	}
}

// fromPuLPJSONFast streams standard unescaped PuLP JSON directly into a *gb.Model
// without allocating intermediate PuLPVariable, PuLPConstraint, or PuLPCoefficient slices.
func fromPuLPJSONFast(data []byte) (*gb.Model, bool) {
	scratch, _ := pulpScratchPool.Get().(*pulpFastScratch)
	if scratch == nil {
		scratch = &pulpFastScratch{}
	}
	sc := pulpScanner{
		b:        data,
		indented: detectIndented(data),
		ends:     scratch.ends[:0],
	}
	hashBuf := scratch.varHash
	termsBuf := scratch.terms[:0]
	varIDs := scratch.varIDs[:0]
	varNames := scratch.varNames[:0]
	varMeta := scratch.varMeta[:0]
	objSpans := scratch.objSpans[:0]

	defer func() {
		if cap(hashBuf) <= maxPoolVarHashCap {
			scratch.varHash = hashBuf[:0]
		} else {
			scratch.varHash = nil
		}
		if cap(sc.ends) <= maxPoolEndsCap {
			scratch.ends = sc.ends[:0]
		} else {
			scratch.ends = nil
		}
		if cap(termsBuf) <= maxPoolEndsCap {
			scratch.terms = termsBuf[:0]
		} else {
			scratch.terms = nil
		}
		clear(varNames[:cap(varNames)])
		if cap(varNames) <= maxPoolEndsCap {
			scratch.varNames = varNames[:0]
		} else {
			scratch.varNames = nil
		}
		if cap(varIDs) <= maxPoolEndsCap {
			scratch.varIDs = varIDs[:0]
		} else {
			scratch.varIDs = nil
		}
		if cap(varMeta) <= maxPoolEndsCap {
			scratch.varMeta = varMeta[:0]
		} else {
			scratch.varMeta = nil
		}
		if cap(objSpans) <= maxPoolEndsCap {
			scratch.objSpans = objSpans[:0]
		} else {
			scratch.objSpans = nil
		}
		pulpScratchPool.Put(scratch)
	}()

	if !sc.consumeByte('{') {
		return nil, false
	}
	var (
		params        PuLPParameters
		m             *gb.Model
		table         []int32
		seen          uint8
		hasDeferred   bool
		defObjName    string
		defObjConst   float64
		hasObjSection bool
	)
	if !sc.consumeByte('}') {
		for {
			key, ok := sc.readUnescapedBytes()
			if !ok || !sc.consumeByte(':') {
				return nil, false
			}
			var bit uint8
			switch string(key) {
			case "parameters":
				bit = 1 << 0
				ok = sc.readParameters(&params)
				if ok {
					var sense gb.Sense
					switch params.Sense {
					case 1:
						sense = gb.Minimize
					case -1:
						sense = gb.Maximize
					default:
						return nil, false
					}
					m = gb.NewModel(params.Name, sense)
				}
			case "objective":
				bit = 1 << 1
				hasObjSection = true
				varsReady := seen&(1<<2) != 0
				defObjName, defObjConst, hasDeferred, ok = streamObjectiveSection(
					&sc, m, varsReady, table, varNames, varIDs, &termsBuf, &objSpans,
				)
			case "variables":
				bit = 1 << 2
				if m == nil {
					return nil, false
				}
				table, ok = streamVariablesSection(&sc, m, &hashBuf, &varMeta, &varNames, &varIDs)
				if ok && hasObjSection && seen&(1<<1) != 0 {
					termsBuf = termsBuf[:0]
					if hasDeferred {
						for _, sp := range objSpans {
							vid, hit := lookupStreamVarID(table, varNames, varIDs, sc.b[sp.start:sp.end])
							if !hit {
								return nil, false
							}
							termsBuf = append(termsBuf, gb.Term{Var: vid, Coeff: sp.val})
						}
					}
					m.SetObjectiveNamed(defObjName, gb.Expr{Terms: termsBuf, Constant: defObjConst})
					hasDeferred = false
				}
			case "constraints":
				bit = 1 << 3
				if m == nil || seen&(1<<2) == 0 {
					return nil, false
				}
				ok = streamConstraintsSection(&sc, m, table, varNames, varIDs, &termsBuf)
			case "sos1":
				bit = 1 << 4
				var sos1 []any
				sos1, ok = sc.readEmptyArray()
				_ = sos1
			case "sos2":
				bit = 1 << 5
				var sos2 []any
				sos2, ok = sc.readEmptyArray()
				_ = sos2
			default:
				return nil, false
			}
			if !ok || seen&bit != 0 {
				return nil, false
			}
			seen |= bit
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
	if sc.pos != len(sc.b) || m == nil || hasDeferred {
		return nil, false
	}
	if err := m.Validate(); err != nil {
		return nil, false
	}
	return m, true
}
