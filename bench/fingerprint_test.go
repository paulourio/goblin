package bench

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSourceFingerprintScope verifies that the fingerprint changes with solver or harness
// sources and ignores tests and rendering-only files, so that the freshness gate demands a
// re-benchmark exactly when measured behavior can change.
func TestSourceFingerprintScope(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module github.com/paulourio/goblin\n")
	write("solver.go", "package goblin\n")
	write("cbc_bridge.h", "int goblin_bridge(void);\n")
	write("cbc_bridge.cpp", "int goblin_bridge(void) { return 0; }\n")
	write("solver_test.go", "package goblin\n")
	write("assign/solver.go", "package assign\n")
	write("format/mps.go", "package format\n")
	write("internal/simd/k.go", "package simd\n")
	write("bench/corpus.go", "package bench\n")
	write("bench/docgen.go", "package bench\n")
	write("cmd/goblin-bench/main.go", "package main\n")
	write("cmd/other/main.go", "package main\n")

	fp := func() string {
		t.Helper()
		s, err := SourceFingerprint(root)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	base := fp()

	for _, rel := range []string{"solver_test.go", "bench/docgen.go", "bench/difftest_cbc.py", "cmd/other/main.go", "testdata/public/afiro.json"} {
		write(rel, "package x // edited\n")
		if got := fp(); got != base {
			t.Fatalf("editing %s changed the fingerprint", rel)
		}
	}
	for _, rel := range []string{
		"solver.go",
		"cbc_bridge.h",
		"cbc_bridge.cpp",
		"assign/solver.go",
		"format/mps.go",
		"internal/simd/k.go",
		"bench/corpus.go",
		"bench/python_baseline.py",
		"testdata/corpus/t1.json",
		"testdata/assign_corpus/manifest.json",
		"cmd/goblin-bench/main.go",
		"go.mod",
	} {
		write(rel, "package x // edited "+rel+"\n")
		got := fp()
		if got == base {
			t.Fatalf("editing %s did not change the fingerprint", rel)
		}
		base = got
	}
}
