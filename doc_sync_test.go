package goblin_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
		"docs/formal-verification.md",
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

	tsBundle, err := bench.LoadTargetSiliconBundle(filepath.Join("bench", "results", "target_silicon"))
	if err != nil {
		t.Fatalf("failed to load target-silicon telemetry: %v", err)
	}
	if err := bench.ValidateTargetSiliconTelemetry(bench.BuildCorpus(), bench.AssignCorpusSpecs(), tsBundle); err != nil {
		t.Fatalf("target-silicon telemetry validation failed: %v", err)
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
	if err := bench.SyncFormalDocs(".", true); err != nil {
		t.Errorf("formal documentation sync gate failed: %v", err)
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
	dirs := []string{".", "assign", "mad", "format", "bench", filepath.Join("internal", "simd")}
	declared := map[string]bool{
		// External Go stdlib / archsimd / Python PuLP / COIN-OR C++ names referenced in prose:
		"Pool":                 true,
		"Context":              true,
		"Writer":               true,
		"FormatFloat":          true,
		"Infinity":             true,
		"Float64x4":            true,
		"ClearAVXUpperBits":    true,
		"StoreArray":           true,
		"GetLo":                true,
		"GetHi":                true,
		"GetElem":              true,
		"MulAdd":               true,
		"PuLP":                 true,
		"LpProblem":            true,
		"LpStatus":             true,
		"toJson":               true,
		"fromJson":             true,
		"toDict":               true,
		"fromDict":             true,
		"varValue":             true,
		"lowBound":             true,
		"upBound":              true,
		"loadProblem":          true,
		"CoinUtils":            true,
		"ClpPresolve":          true,
		"CbcModel":             true,
		"CglPreProcess":        true,
		"Netlib":               true,
		"SplitMix64":           true,
		"Markowitz":            true,
		"Savelsbergh":          true,
		"NumericalError":       true,
		"OptimalObj":           true,
		"TimeLimit":            true,
		"NodeLimit":            true,
		"CertifiedWitness":     true,
		"WithCertifiedWitness": true,
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

// TestFuzzHarnessTargetsSync checks that Go Fuzz* functions match
// scripts/gcp-fuzz/targets.tsv, scripts/corpus_audit.sh, and scripts/local_fuzz_soak.sh.
func TestFuzzHarnessTargetsSync(t *testing.T) {
	t.Parallel()
	inv, err := bench.CollectFormalInventory(".")
	if err != nil {
		t.Fatalf("CollectFormalInventory: %v", err)
	}
	wantAll := make([]string, 0, len(inv.FuzzTargets))
	wantDef := make([]string, 0, inv.DefaultFuzzCount)
	for _, ft := range inv.FuzzTargets {
		wantAll = append(wantAll, ft.Name)
		if !ft.IsCBC {
			wantDef = append(wantDef, ft.Name)
		}
	}
	slices.Sort(wantAll)
	slices.Sort(wantDef)

	if runtime.GOOS == "windows" {
		return
	}
	listFromScript := func(args ...string) []string {
		cmd := exec.CommandContext(t.Context(), "bash", args...)
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			t.Fatalf("bash %v failed: %v\n%s", args, cmdErr, out)
		}
		var got []string
		for line := range strings.SplitSeq(string(out), "\n") {
			s := strings.TrimSpace(line)
			if s != "" {
				got = append(got, s)
			}
		}
		slices.Sort(got)
		return got
	}

	gotAudit := listFromScript(filepath.Join("scripts", "corpus_audit.sh"), "--skip-gcs", "--list-targets")
	if !slices.Equal(gotAudit, wantAll) {
		t.Fatalf("corpus_audit.sh targets mismatch:\ngot=%v\nwant=%v", gotAudit, wantAll)
	}
	gotSoakAllCBC := listFromScript(filepath.Join("scripts", "local_fuzz_soak.sh"), "--phase=all-cbc", "--list-targets")
	if !slices.Equal(gotSoakAllCBC, wantAll) {
		t.Fatalf("local_fuzz_soak.sh --phase=all-cbc targets mismatch:\ngot=%v\nwant=%v", gotSoakAllCBC, wantAll)
	}
	gotSoakDefault := listFromScript(filepath.Join("scripts", "local_fuzz_soak.sh"), "--phase=all", "--list-targets")
	if !slices.Equal(gotSoakDefault, wantDef) {
		t.Fatalf("local_fuzz_soak.sh --phase=all targets mismatch:\ngot=%v\nwant=%v", gotSoakDefault, wantDef)
	}
}

// TestTargetSiliconDocumentationSync verifies that bench/results/target_silicon/*.json
// satisfy schema and parity invariants and match docs/benchmarks.md (TARGET_SILICON_SUMMARY)
// and formal inventory blocks byte-for-byte without requiring live cloud access.
func TestTargetSiliconDocumentationSync(t *testing.T) {
	t.Parallel()
	tsBundle, err := bench.LoadTargetSiliconBundle(filepath.Join("bench", "results", "target_silicon"))
	if err != nil {
		t.Fatalf("load target-silicon telemetry: %v", err)
	}
	if err := bench.ValidateTargetSiliconTelemetry(bench.BuildCorpus(), bench.AssignCorpusSpecs(), tsBundle); err != nil {
		t.Fatalf("validate target-silicon telemetry: %v", err)
	}
	if err := bench.SyncFormalDocs(".", true); err != nil {
		t.Fatalf("target-silicon and formal documentation sync check failed: %v", err)
	}
}

// TestTargetSiliconHygieneGate ensures scripts/verify_target_silicon.sh and the committed
// target-silicon JSON files never leak cloud project IDs, account emails, IP addresses,
// GCP regions/zones, deprecated storage CLI names, VM hostnames, or local user home paths.
func TestTargetSiliconHygieneGate(t *testing.T) {
	t.Parallel()

	checks := []struct {
		path                string
		allowInstancePrefix bool
	}{
		{path: filepath.Join("scripts", "verify_target_silicon.sh"), allowInstancePrefix: true},
		{path: filepath.Join("bench", "results", "target_silicon", "amd64_avx512.json"), allowInstancePrefix: false},
		{path: filepath.Join("bench", "results", "target_silicon", "linux_arm64.json"), allowInstancePrefix: false},
	}

	var dynamicSecrets []string
	if gcloudExe, err := exec.LookPath("gcloud"); err == nil {
		for _, prop := range []string{"project", "account"} {
			cmd := exec.CommandContext(t.Context(), gcloudExe, "config", "get-value", prop, "--quiet")
			if out, cmdErr := cmd.Output(); cmdErr == nil {
				val := strings.TrimSpace(string(out))
				if val != "" && val != "(unset)" && len(val) >= 4 {
					dynamicSecrets = append(dynamicSecrets, val)
				}
			}
		}
	}

	for _, c := range checks {
		raw, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("read %s: %v", c.path, err)
		}
		txt := string(raw)
		if err := bench.ValidateTargetSiliconHygiene(c.path, txt, c.allowInstancePrefix); err != nil {
			t.Fatalf("hygiene check failed: %v", err)
		}
		for _, secret := range dynamicSecrets {
			if strings.Contains(txt, secret) {
				t.Fatalf("%s leaked active local gcloud configuration token", c.path)
			}
		}
	}

	// Negative controls verifying that each forbidden pattern class fails closed.
	poisonSamples := []struct {
		name  string
		input string
		allow bool
	}{
		{name: "email", input: "operator" + "@" + "corp.example.org", allow: true},
		{name: "home_path", input: "/" + "ho" + "me/operator/work", allow: true},
		{name: "ipv4", input: "10." + "128.0.42", allow: true},
		{name: "gcp_zone", input: "us-" + "central1-a", allow: true},
		{name: "legacy_storage_cli", input: "gs" + "util cp", allow: true},
		{name: "vm_hostname_in_json", input: "goblin-" + "silicon-amd64-123", allow: false},
	}
	for _, ps := range poisonSamples {
		if err := bench.ValidateTargetSiliconHygiene(ps.name, ps.input, ps.allow); err == nil {
			t.Fatalf("expected ValidateTargetSiliconHygiene to reject synthetic %s sample", ps.name)
		}
	}
}

// TestMADSubpackageZeroMILPDeps verifies that the transitive dependency closure of ./mad
// excludes the root MILP solver, internal packages, math/big, and encoding/json packages.
func TestMADSubpackageZeroMILPDeps(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "./mad")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./mad failed: %v\n%s", err, out)
	}

	const rootPkg = "github.com/paulourio/gob" + "lin"
	for raw := range strings.SplitSeq(string(out), "\n") {
		pkg := strings.TrimSpace(raw)
		if pkg == "" {
			continue
		}
		if pkg == rootPkg || (strings.HasPrefix(pkg, rootPkg+"/") && pkg != rootPkg+"/mad") {
			t.Fatalf("./mad transitive dependency closure contains forbidden package %q", pkg)
		}
		if pkg == "math/big" || strings.HasPrefix(pkg, "math/big/") {
			t.Fatalf("./mad transitive dependency closure contains forbidden package %q", pkg)
		}
		if pkg == "encoding/json" || strings.HasPrefix(pkg, "encoding/json/") {
			t.Fatalf("./mad transitive dependency closure contains forbidden package %q", pkg)
		}
	}
}
