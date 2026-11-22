package goblin_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/paulourio/goblin/bench"
)

// TestDocumentationAndBenchmarkSync verifies that all Markdown tables and the final benchmark
// report match the canonical JSON telemetry artifacts on disk.
func TestDocumentationAndBenchmarkSync(t *testing.T) {
	t.Parallel()
	requiredDocs := []string{
		"README.md",
		"docs/index.md",
		"docs/modeling-and-formats.md",
		"docs/solver-internals.md",
		"docs/benchmarks.md",
		"bench/results/final_report.md",
	}
	for _, p := range requiredDocs {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing required documentation file %s: %v", p, err)
		}
		if info.Size() == 0 {
			t.Fatalf("documentation file %s is empty", p)
		}
	}

	tb, err := bench.LoadTelemetryBundle(
		"bench/results/python_baseline.json",
		"bench/results/goblin_scalar.json",
		"bench/results/goblin_simd.json",
	)
	if err != nil {
		t.Fatalf("failed to load canonical benchmark telemetry: %v", err)
	}

	if err := bench.ValidateTelemetryAgainstCorpus("testdata/corpus", tb); err != nil {
		t.Fatalf("corpus vs. telemetry validation failed: %v", err)
	}
	if err := bench.CheckTelemetryFreshness(".", tb); err != nil {
		t.Fatal(err)
	}

	syncedFiles := []string{
		"README.md",
		"docs/benchmarks.md",
		"bench/results/final_report.md",
	}
	for _, path := range syncedFiles {
		if err := bench.SyncMarkdownFile(path, tb, true); err != nil {
			t.Errorf("documentation sync gate failed: %v", err)
		}
	}

	expectedReport, err := bench.BuildFinalReportMarkdown(tb)
	if err != nil {
		t.Fatalf("BuildFinalReportMarkdown failed: %v", err)
	}
	rawReport, err := os.ReadFile("bench/results/final_report.md")
	if err != nil {
		t.Fatalf("read final_report.md: %v", err)
	}
	if string(rawReport) != expectedReport {
		t.Errorf("bench/results/final_report.md is out of sync with BuildFinalReportMarkdown (run `go run ./cmd/goblin-bench -sync-docs`)")
	}
}

// Verifies that CamelCase Go identifiers cited in architecture and internals docs match
// live AST declarations across the repository so stale symbol names fail CI immediately.
func TestDocBacktickIdentifiersResolve(t *testing.T) {
	t.Parallel()
	dirs := []string{".", "assign", "format", "bench", filepath.Join("internal", "simd")}
	declared := map[string]bool{
		// External Go stdlib / archsimd / Python PuLP / COIN-OR C++ names referenced in prose:
		"Pool":              true,
		"Context":           true,
		"Writer":            true,
		"FormatFloat":       true,
		"Infinity":          true,
		"Float64x4":         true,
		"ClearAVXUpperBits": true,
		"StoreArray":        true,
		"GetLo":             true,
		"GetHi":             true,
		"GetElem":           true,
		"MulAdd":            true,
		"PuLP":              true,
		"LpProblem":         true,
		"LpStatus":          true,
		"toJson":            true,
		"fromJson":          true,
		"toDict":            true,
		"fromDict":          true,
		"varValue":          true,
		"lowBound":          true,
		"upBound":           true,
		"loadProblem":       true,
		"CoinUtils":         true,
		"ClpPresolve":       true,
		"CbcModel":          true,
		"CglPreProcess":     true,
		"Netlib":            true,
		"SplitMix64":        true,
		"Markowitz":         true,
		"Savelsbergh":       true,
		"NumericalError":    true,
		"OptimalObj":        true,
		"TimeLimit":         true,
		"NodeLimit":         true,
	}

	fset := token.NewFileSet()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, ent.Name()), nil, 0)
			if err != nil {
				t.Fatalf("parse %s/%s: %v", dir, ent.Name(), err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.FuncDecl:
					if decl.Name != nil {
						declared[decl.Name.Name] = true
					}
					if decl.Type != nil && decl.Type.Params != nil {
						for _, fld := range decl.Type.Params.List {
							for _, id := range fld.Names {
								declared[id.Name] = true
							}
						}
					}
				case *ast.TypeSpec:
					if decl.Name != nil {
						declared[decl.Name.Name] = true
					}
				case *ast.ValueSpec:
					for _, id := range decl.Names {
						declared[id.Name] = true
					}
				case *ast.Field:
					for _, id := range decl.Names {
						declared[id.Name] = true
					}
				case *ast.AssignStmt:
					if decl.Tok == token.DEFINE {
						for _, lhs := range decl.Lhs {
							if id, ok := lhs.(*ast.Ident); ok {
								declared[id.Name] = true
							}
						}
					}
				}
				return true
			})
		}
	}

	reCodeSpan := regexp.MustCompile("`([^`\n]+)`")
	reIdent := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	isMixedCamel := func(s string) bool {
		if len(s) < 4 {
			return false
		}
		var hasUpper, hasLower bool
		for _, r := range s {
			if unicode.IsUpper(r) {
				hasUpper = true
			} else if unicode.IsLower(r) {
				hasLower = true
			}
		}
		return hasUpper && hasLower
	}

	for _, docPath := range []string{"docs/index.md", "docs/modeling-and-formats.md", "docs/solver-internals.md"} {
		raw, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		// Strip fenced code blocks before scanning inline backtick spans.
		lines := strings.Split(string(raw), "\n")
		inFence := false
		for lineNo, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			for _, m := range reCodeSpan.FindAllStringSubmatch(line, -1) {
				span := strings.TrimSpace(m[1])
				// Strip trailing () or [j] or leading package/receiver qualifier (e.g. LPSolver.Solve -> Solve).
				if idx := strings.IndexAny(span, "(["); idx > 0 {
					span = span[:idx]
				}
				if dot := strings.LastIndexByte(span, '.'); dot >= 0 {
					span = span[dot+1:]
				}
				if !reIdent.MatchString(span) || !isMixedCamel(span) {
					continue
				}
				if !declared[span] {
					t.Errorf("%s:%d references unknown CamelCase identifier %q", docPath, lineNo+1, span)
				}
			}
		}
	}
}
