package format

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParsePuLPJSONFastParityAndFallback verifies bit-exact structural parity between the
// zero-reflection fast parser and encoding/json across all 31 benchmark corpus files, scientific
// notation, 17-digit floats, omitted lowBound keys, escaped strings, and malformed inputs.
func TestParsePuLPJSONFastParityAndFallback(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "testdata", "corpus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corpus JSON files, got %d (err=%v)", len(matches), err)
	}
	for _, p := range matches {
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			t.Fatalf("read %s: %v", p, rErr)
		}
		fastDoc, ok := parsePuLPJSONFast(raw)
		if !ok {
			t.Fatalf("expected parsePuLPJSONFast to succeed on corpus file %s", p)
		}
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if sErr != nil {
			t.Fatalf("parsePuLPJSONStd failed on %s: %v", p, sErr)
		}
		if !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("fast vs std mismatch on %s", p)
		}
	}

	specialPayloads := []struct {
		name     string
		raw      string
		wantFast bool
	}{
		{
			name:     "exponents_and_17digit_and_omitted_lb",
			raw:      `{"parameters":{"name":"sci","sense":-1,"status":1,"sol_status":1},"objective":{"name":null,"coefficients":[{"name":"x","value":1.0000000000007},{"name":"y","value":-1.25e-14}],"constant":1e+2},"variables":[{"name":"x","cat":"Continuous","upBound":1e20},{"name":"y","cat":"Integer","lowBound":null,"upBound":-0.0,"varValue":3,"dj":-2.5}],"constraints":[{"name":"r0","sense":0,"coefficients":[{"name":"x","value":9999999999999999},{"name":"y","value":-42}],"pi":0.5,"constant":-10.75}],"sos1":[],"sos2":[]}`,
			wantFast: true,
		},
		{
			name:     "escaped_string_falls_back_cleanly",
			raw:      `{"parameters":{"name":"esc\u0041","sense":1},"objective":{"name":"OBJ","coefficients":[]},"variables":[{"name":"x","cat":"Continuous","lowBound":0,"upBound":1}],"constraints":[],"sos1":[],"sos2":[]}`,
			wantFast: false,
		},
	}
	for _, tc := range specialPayloads {
		raw := []byte(tc.raw)
		fastDoc, ok := parsePuLPJSONFast(raw)
		if ok != tc.wantFast {
			t.Fatalf("%s: parsePuLPJSONFast ok=%v, want %v", tc.name, ok, tc.wantFast)
		}
		gotDoc, pErr := ParsePuLPJSON(raw)
		stdDoc, sErr := parsePuLPJSONStd(raw)
		if (pErr != nil) != (sErr != nil) {
			t.Fatalf("%s: ParsePuLPJSON err=%v vs std err=%v", tc.name, pErr, sErr)
		}
		if !reflect.DeepEqual(gotDoc, stdDoc) {
			t.Fatalf("%s: ParsePuLPJSON mismatch:\ngot:  %+v\nwant: %+v", tc.name, gotDoc, stdDoc)
		}
		if ok && !reflect.DeepEqual(fastDoc, stdDoc) {
			t.Fatalf("%s: fastDoc mismatch", tc.name)
		}
	}

	malformed := []string{
		`{"parameters":{"name":"x","sense":01}}`,
		`{"parameters":{"name":"x","sense":1}} trailing`,
		`{"parameters":{"name":"x","sense":1`,
		`{"parameters":{"name":"x","sense":1},"variables":[{"name":"v","cat":"Continuous","lowBound":1.}]}`,
	}
	for _, bad := range malformed {
		if _, ok := parsePuLPJSONFast([]byte(bad)); ok {
			t.Fatalf("expected parsePuLPJSONFast to reject malformed payload %q", bad)
		}
		if _, err := ParsePuLPJSON([]byte(bad)); err == nil {
			t.Fatalf("expected ParsePuLPJSON error on malformed payload %q", bad)
		}
	}
}
