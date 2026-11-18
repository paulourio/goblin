package format

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParsePuLPJSONFastParityAndFallback verifies bit-exact structural parity between the
// zero-reflection fast parser and encoding/json/v2 across all 31 benchmark corpus files, scientific
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

// BenchmarkParsePuLPJSON compares the arena-backed fast scanner against the encoding/json/v2
// fallback across Tier 1, Tier 2, and Tier 3 PuLP JSON payloads.
func BenchmarkParsePuLPJSON(b *testing.B) {
	workloads := []struct {
		name string
		file string
	}{
		{"T1_Whiskas", "t1_whiskas.json"},
		{"T2_Lotsizing48", "t2_lotsizing_48.json"},
		{"T3_CFLP300", "t3_cflp_300.json"},
		{"T3_Sched1000", "t3_sched_1000.json"},
	}
	for _, wl := range workloads {
		raw, err := os.ReadFile(filepath.Join("..", "testdata", "corpus", wl.file))
		if err != nil {
			raw, err = os.ReadFile(filepath.Join("testdata", "corpus", wl.file))
		}
		if err != nil {
			b.Fatalf("read %s: %v", wl.file, err)
		}
		b.Run(wl.name+"/pulp_json_fast", func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				doc, ok := parsePuLPJSONFast(raw)
				if !ok || doc == nil {
					b.Fatal("parsePuLPJSONFast failed")
				}
			}
		})
		b.Run(wl.name+"/json_v2", func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				doc, err := parsePuLPJSONStd(raw)
				if err != nil || doc == nil {
					b.Fatal(err)
				}
			}
		})
	}
}
