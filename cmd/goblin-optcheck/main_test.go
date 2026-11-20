package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSuitesAndEnclosingSpans(t *testing.T) {
	for _, name := range []string{"all", "mip", "lp", "assign"} {
		suites, err := resolveSuites(name, "", "")
		if err != nil || len(suites) == 0 {
			t.Fatalf("resolveSuites(%q) = (%v, %v), want non-empty", name, suites, err)
		}
	}
	if _, err := resolveSuites("bogus", "", ""); err == nil {
		t.Fatal("expected error from resolveSuites(bogus)")
	}

	spans := []funcSpan{
		{Pkg: "p", File: "foo.go", ShortName: "Outer", FullName: "Outer", StartLine: 10, EndLine: 30},
	}
	if sp := findEnclosingSpan(spans, "./foo.go", 20); sp == nil || sp.FullName != "Outer" {
		t.Fatalf("findEnclosingSpan(./foo.go, 20) = %v, want Outer", sp)
	}
	if sp := findEnclosingSpan(spans, "foo.go", 5); sp != nil {
		t.Fatalf("findEnclosingSpan(foo.go, 5) = %v, want nil", sp)
	}
}

func TestInnerClosureDoesNotFlipOuterCanInline(t *testing.T) {
	tmp := t.TempDir()
	modSrc := "module example.com/optchecktest\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(modSrc), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Outer cannot be inlined because it uses defer, whereas its inner closure is trivially inlineable.
	goSrc := `package optchecktest

func Outer(xs []int) int {
	defer func() {}()
	inc := func(v int) int { return v + 1 }
	sum := 0
	for _, x := range xs {
		sum += inc(x)
	}
	return sum
}
`
	if err := os.WriteFile(filepath.Join(tmp, "foo.go"), []byte(goSrc), 0o600); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}

	t.Chdir(tmp)
	spans, err := parseRepoFuncSpans(".", false)
	if err != nil {
		t.Fatalf("parseRepoFuncSpans: %v", err)
	}
	if len(spans) != 1 || spans[0].FullName != "Outer" {
		t.Fatalf("spans = %+v, want single Outer span", spans)
	}

	diagMap, _, _, _, err := collectStaticDiagnostics(t.Context(), false, spans)
	if err != nil {
		t.Fatalf("collectStaticDiagnostics: %v", err)
	}
	d := diagMap["foo.go:Outer"]
	if d == nil {
		t.Fatalf("missing foo.go:Outer in diagMap: %+v", diagMap)
	}
	if d.CanInline {
		t.Fatalf("Outer CanInline = true (clobbered by inner closure), want false (reason=%q)", d.InlineReason)
	}
}
