// Command goblin-optcheck profiles Goblin benchmark suites, correlates CPU and allocation
// hotspots with Go compiler optimization diagnostics (bounds check elimination, inlining
// costs, and escape analysis) and x86-64 disassembly metrics, and ranks optimization
// targets by Amdahl's Law speedup ceiling.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBenchTime    = 250 * time.Millisecond
	defaultTopFuncs     = 12
	defaultMinFlatPct   = 1.0
	defaultHotLines     = 6
	exitCodeBadArgument = 2
	pctScale            = 100.0
	amdahlFactor15x     = 1.5
	amdahlFactor20x     = 2.0
	inlineBudgetDefault = 80
	nearInlineCostLimit = 160
	reportPerm          = 0o644
	tableWidth          = 124
)

type suiteSpec struct {
	Name  string
	Pkg   string
	Bench string
}

type funcSpan struct {
	Pkg       string
	File      string
	ShortName string
	FullName  string
	StartLine int
	EndLine   int
}

type staticFuncDiag struct {
	File          string   `json:"file"`
	StartLine     int      `json:"start_line"`
	EndLine       int      `json:"end_line"`
	CanInline     bool     `json:"can_inline"`
	InlineCost    int      `json:"inline_cost,omitempty"`
	InlineReason  string   `json:"inline_reason,omitempty"`
	BCECount      int      `json:"bce_count"`
	BCELines      []int    `json:"bce_lines,omitempty"`
	EscapeCount   int      `json:"escape_count"`
	EscapeDetails []string `json:"escape_details,omitempty"`
}

type hotLineSample struct {
	Line      int     `json:"line"`
	FlatMS    float64 `json:"flat_ms"`
	CumMS     float64 `json:"cum_ms"`
	HasBCE    bool    `json:"has_bce,omitempty"`
	HasEscape bool    `json:"has_escape,omitempty"`
	Source    string  `json:"source"`
}

type disasmMetrics struct {
	InstCount        int      `json:"inst_count"`
	PanicBoundsCalls int      `json:"panic_bounds_calls"`
	GrowSliceCalls   int      `json:"grow_slice_calls"`
	WriteBarriers    int      `json:"write_barriers"`
	AllocCalls       int      `json:"alloc_calls"`
	StackSpillRefs   int      `json:"stack_spill_refs"`
	ScalarFPOps      int      `json:"scalar_fp_ops"`
	VectorFPOps      int      `json:"vector_fp_ops"`
	NonInlinedCalls  []string `json:"non_inlined_calls,omitempty"`
}

type hotFuncReport struct {
	Rank            int             `json:"rank"`
	FuncName        string          `json:"func_name"`
	ShortName       string          `json:"short_name"`
	File            string          `json:"file"`
	StartLine       int             `json:"start_line"`
	EndLine         int             `json:"end_line"`
	FlatSec         float64         `json:"flat_sec"`
	FlatPct         float64         `json:"flat_pct"`
	CumSec          float64         `json:"cum_sec"`
	CumPct          float64         `json:"cum_pct"`
	AmdahlFlat15x   float64         `json:"amdahl_flat_1_5x"`
	AmdahlFlat20x   float64         `json:"amdahl_flat_2_0x"`
	AmdahlFlatMax   float64         `json:"amdahl_flat_max"`
	AmdahlCum20x    float64         `json:"amdahl_cum_2_0x"`
	Static          staticFuncDiag  `json:"static"`
	HotLineBCEMiss  int             `json:"hot_line_bce_misses"`
	Disasm          disasmMetrics   `json:"disasm"`
	TopSampledLines []hotLineSample `json:"top_sampled_lines,omitempty"`
	Findings        []string        `json:"findings,omitempty"`
}

type nearInlineCandidate struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	FuncName  string `json:"func_name"`
	Cost      int    `json:"cost"`
	Budget    int    `json:"budget"`
	OverBy    int    `json:"over_by"`
	RawReason string `json:"raw_reason"`
}

type suiteReport struct {
	Suite          string          `json:"suite"`
	Package        string          `json:"package"`
	BenchRegex     string          `json:"bench_regex"`
	SIMDEnabled    bool            `json:"simd_enabled"`
	TotalSamplesS  float64         `json:"total_samples_sec"`
	HotFunctions   []hotFuncReport `json:"hot_functions"`
	TopAllocations []string        `json:"top_allocations,omitempty"`
}

type fullReport struct {
	GeneratedAt          string                `json:"generated_at"`
	SIMDEnabled          bool                  `json:"simd_enabled"`
	TotalRepoBCEMisses   int                   `json:"total_repo_bce_misses"`
	TotalRepoHeapEscapes int                   `json:"total_repo_heap_escapes"`
	NearInlineCandidates []nearInlineCandidate `json:"near_inline_candidates"`
	Suites               []suiteReport         `json:"suites"`
}

var (
	reDiagLoc       = regexp.MustCompile(`^(\./)?([^:\s]+\.go):(\d+):(\d+):\s*(.+)$`)
	reCannotInline  = regexp.MustCompile(`^cannot inline ([^:]+): (.+)$`)
	reCanInline     = regexp.MustCompile(`^can inline ([^ ]+) with cost (\d+)`)
	reCostBudget    = regexp.MustCompile(`cost (\d+) exceeds budget (\d+)`)
	rePprofTopLine  = regexp.MustCompile(`^\s*([0-9.]+)(ms|s)\s+([0-9.]+)%\s+([0-9.]+)%\s+([0-9.]+)(ms|s)\s+([0-9.]+)%\s+(\S+)`)
	rePprofTotal    = regexp.MustCompile(`Total samples = ([0-9.]+)(ms|s)`)
	rePprofListRout = regexp.MustCompile(`^ROUTINE ======================== (\S+) in (.+)$`)
	rePprofListLine = regexp.MustCompile(`^\s*([0-9.]+(?:ms|s)|\.)\s+([0-9.]+(?:ms|s)|\.)\s+(\d+):(.*)$`)
	reObjdumpHeader = regexp.MustCompile(`^TEXT\s+(\S+)\(SB\)`)
	reObjdumpInst   = regexp.MustCompile(`^\s*(?:\S+:\d+\s+)?0x[0-9a-f]+\s+[0-9a-f]+\s+([A-Z0-9_]+)\s*(.*)$`)
	reCallTarget    = regexp.MustCompile(`CALL\s+([^\s(]+)\(SB\)`)
)

func main() {
	os.Exit(run())
}

func run() int {
	suiteFlag := flag.String("suite", "all", "benchmark suite to analyze: all, mip, lp, assign")
	pkgFlag := flag.String("pkg", "", "explicit package override for custom benchmark run")
	benchFlag := flag.String("bench", "", "custom benchmark regex override")
	benchTime := flag.Duration("benchtime", defaultBenchTime, "benchmark duration per target")
	topN := flag.Int("top", defaultTopFuncs, "maximum number of hot functions per suite")
	minFlat := flag.Float64("min-flat", defaultMinFlatPct, "minimum flat CPU percentage for detailed analysis")
	useSIMD := flag.Bool("simd", false, "enable GOEXPERIMENT=simd during compilation and profiling")
	jsonOut := flag.String("json", "", "optional path to write structured JSON report")
	flag.Parse()

	suites, err := resolveSuites(*suiteFlag, *pkgFlag, *benchFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCodeBadArgument
	}

	tmpDir, err := os.MkdirTemp("", "goblin-optcheck-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	spans, err := parseRepoFuncSpans(".", *useSIMD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse AST spans: %v\n", err)
		return 1
	}

	ctx := context.Background()
	diagByFunc, nearInline, totalBCE, totalEsc, err := collectStaticDiagnostics(ctx, *useSIMD, spans)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compiler diagnostic pass failed: %v\n", err)
		return 1
	}

	rep := fullReport{
		GeneratedAt:          time.Now().Format(time.RFC3339),
		SIMDEnabled:          *useSIMD,
		TotalRepoBCEMisses:   totalBCE,
		TotalRepoHeapEscapes: totalEsc,
		NearInlineCandidates: nearInline,
		Suites:               make([]suiteReport, 0, len(suites)),
	}

	for _, sp := range suites {
		sr, err := analyzeSuite(ctx, tmpDir, sp, *benchTime, *topN, *minFlat, *useSIMD, diagByFunc, spans)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suite %s failed: %v\n", sp.Name, err)
			return 1
		}
		rep.Suites = append(rep.Suites, sr)
	}

	printTerminalReport(rep)

	if *jsonOut != "" {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*jsonOut, append(raw, '\n'), reportPerm); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *jsonOut, err)
			return 1
		}
	}
	return 0
}

func resolveSuites(suite, pkg, bench string) ([]suiteSpec, error) {
	if pkg != "" || bench != "" {
		if pkg == "" {
			pkg = "."
		}
		if bench == "" {
			bench = "."
		}
		return []suiteSpec{{Name: "custom", Pkg: pkg, Bench: bench}}, nil
	}
	mipLP := suiteSpec{
		Name:  "mip_lp",
		Pkg:   ".",
		Bench: "BenchmarkSolveCorpus/t2_cflp_36|BenchmarkSolveCorpus/t2_gap_45|BenchmarkSolveCorpus/t3_cflp_300|BenchmarkRootLP/t3_cflp_300|BenchmarkRootLP/t3_sched_1000",
	}
	lpOnly := suiteSpec{
		Name:  "lp",
		Pkg:   ".",
		Bench: "BenchmarkRootLP/t2_mad_g4_i12|BenchmarkRootLP/t2_lotsizing_48|BenchmarkRootLP/t3_cflp_300|BenchmarkRootLP/t3_sched_1000",
	}
	assignSuite := suiteSpec{
		Name:  "assign",
		Pkg:   "./assign",
		Bench: "BenchmarkLinearSumSquare/N=500/SolveIntoWarm|BenchmarkLinearSumRectangularAndTied/Wide_250x1000|BenchmarkLinearSumRectangularAndTied/Tied_500x500",
	}
	switch suite {
	case "all":
		return []suiteSpec{mipLP, assignSuite}, nil
	case "mip":
		return []suiteSpec{mipLP}, nil
	case "lp":
		return []suiteSpec{lpOnly}, nil
	case "assign":
		return []suiteSpec{assignSuite}, nil
	default:
		return nil, fmt.Errorf("unknown -suite %q (expected all, mip, lp, or assign)", suite)
	}
}

func parseRepoFuncSpans(root string, useSIMD bool) ([]funcSpan, error) {
	fset := token.NewFileSet()
	var spans []funcSpan

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "third_party" || name == "testdata" || name == "cmd" || name == "bench" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if useSIMD && strings.HasSuffix(path, "kernels_scalar.go") {
			return nil
		}
		if !useSIMD && strings.HasSuffix(path, "kernels_amd64.go") {
			return nil
		}
		fileAst, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		relPath := filepath.Clean(path)
		pkgName := fileAst.Name.Name
		for _, decl := range fileAst.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line
			short := fn.Name.Name
			full := short
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recvName := exprTypeName(fn.Recv.List[0].Type)
				full = "(" + recvName + ")." + short
			}
			spans = append(spans, funcSpan{
				Pkg:       pkgName,
				File:      relPath,
				ShortName: short,
				FullName:  full,
				StartLine: start,
				EndLine:   end,
			})
		}
		return nil
	})
	return spans, err
}

func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + exprTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return exprTypeName(t.X)
	default:
		return "unknown"
	}
}

func findEnclosingSpan(spans []funcSpan, file string, line int) *funcSpan {
	cleanFile := filepath.Clean(file)
	for i := range spans {
		s := &spans[i]
		if s.File == cleanFile && line >= s.StartLine && line <= s.EndLine {
			return s
		}
	}
	return nil
}

func collectStaticDiagnostics(
	ctx context.Context,
	useSIMD bool,
	spans []funcSpan,
) (map[string]*staticFuncDiag, []nearInlineCandidate, int, int, error) {
	diagMap := make(map[string]*staticFuncDiag, len(spans))
	for _, s := range spans {
		key := s.File + ":" + s.FullName
		diagMap[key] = &staticFuncDiag{
			File:      s.File,
			StartLine: s.StartLine,
			EndLine:   s.EndLine,
		}
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-a", "-gcflags=./...=-m=2 -d=ssa/check_bce/debug=1", "./...")
	cmd.Env = buildEnv(useSIMD)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("go build gcflags failed: %w\n%s", err, stderr.String())
	}

	var nearInline []nearInlineCandidate
	seenNear := make(map[string]bool)
	seenBCE := make(map[string]bool)
	seenEsc := make(map[string]bool)
	totalBCE := 0
	totalEsc := 0

	for line := range strings.SplitSeq(stderr.String(), "\n") {
		m := reDiagLoc.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		file := filepath.Clean(m[2])
		if strings.HasPrefix(file, "/") || strings.HasPrefix(file, "..") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		lineNum, err := strconv.Atoi(m[3])
		if err != nil {
			continue
		}
		msg := m[5]

		if cm := reCanInline.FindStringSubmatch(msg); cm != nil {
			cost, _ := strconv.Atoi(cm[2])
			if sp := findEnclosingSpan(spans, file, lineNum); sp != nil {
				d := diagMap[sp.File+":"+sp.FullName]
				d.CanInline = true
				d.InlineCost = cost
			}
			continue
		}

		if cm := reCannotInline.FindStringSubmatch(msg); cm != nil {
			fnName := cm[1]
			reason := cm[2]
			if sp := findEnclosingSpan(spans, file, lineNum); sp != nil {
				d := diagMap[sp.File+":"+sp.FullName]
				if sp.StartLine == lineNum {
					d.CanInline = false
					d.InlineReason = reason
					if bm := reCostBudget.FindStringSubmatch(reason); bm != nil {
						cost, _ := strconv.Atoi(bm[1])
						budget, _ := strconv.Atoi(bm[2])
						d.InlineCost = cost
						if cost <= nearInlineCostLimit && budget == inlineBudgetDefault {
							key := fmt.Sprintf("%s:%d:%s", file, lineNum, fnName)
							if !seenNear[key] {
								seenNear[key] = true
								nearInline = append(nearInline, nearInlineCandidate{
									File:      file,
									Line:      lineNum,
									FuncName:  fnName,
									Cost:      cost,
									Budget:    budget,
									OverBy:    cost - budget,
									RawReason: reason,
								})
							}
						}
					}
				}
			}
			continue
		}

		if strings.Contains(msg, "Found IsInBounds") || strings.Contains(msg, "Found IsSliceInBounds") {
			bceKey := fmt.Sprintf("%s:%d:%s", file, lineNum, m[4])
			if seenBCE[bceKey] {
				continue
			}
			seenBCE[bceKey] = true
			totalBCE++
			if sp := findEnclosingSpan(spans, file, lineNum); sp != nil {
				d := diagMap[sp.File+":"+sp.FullName]
				d.BCECount++
				if !slices.Contains(d.BCELines, lineNum) {
					d.BCELines = append(d.BCELines, lineNum)
				}
			}
			continue
		}

		if (strings.Contains(msg, "escapes to heap") || strings.Contains(msg, "moved to heap")) && !strings.Contains(msg, " in ") {
			escKey := fmt.Sprintf("%s:%d:%s", file, lineNum, msg)
			if seenEsc[escKey] {
				continue
			}
			seenEsc[escKey] = true
			totalEsc++
			if sp := findEnclosingSpan(spans, file, lineNum); sp != nil {
				d := diagMap[sp.File+":"+sp.FullName]
				d.EscapeCount++
				d.EscapeDetails = append(d.EscapeDetails, fmt.Sprintf("L%d: %s", lineNum, msg))
			}
		}
	}

	slices.SortFunc(nearInline, func(a, b nearInlineCandidate) int {
		if a.OverBy != b.OverBy {
			return a.OverBy - b.OverBy
		}
		return strings.Compare(a.File, b.File)
	})

	return diagMap, nearInline, totalBCE, totalEsc, nil
}

func analyzeSuite(
	ctx context.Context,
	tmpDir string,
	sp suiteSpec,
	benchTime time.Duration,
	topN int,
	minFlatPct float64,
	useSIMD bool,
	diagByFunc map[string]*staticFuncDiag,
	spans []funcSpan,
) (suiteReport, error) {
	binPath := filepath.Join(tmpDir, sp.Name+".test")
	cpuProf := filepath.Join(tmpDir, sp.Name+".cpu.prof")
	memProf := filepath.Join(tmpDir, sp.Name+".mem.prof")

	compileCmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", binPath, sp.Pkg)
	compileCmd.Env = buildEnv(useSIMD)
	if out, err := compileCmd.CombinedOutput(); err != nil {
		return suiteReport{}, fmt.Errorf("compile %s: %w\n%s", sp.Pkg, err, string(out))
	}

	runCmd := exec.CommandContext(
		ctx,
		binPath,
		"-test.run=^$",
		"-test.bench="+sp.Bench,
		"-test.benchtime="+benchTime.String(),
		"-test.cpuprofile="+cpuProf,
		"-test.memprofile="+memProf,
	)
	runCmd.Env = buildEnv(useSIMD)
	if out, err := runCmd.CombinedOutput(); err != nil {
		return suiteReport{}, fmt.Errorf("benchmark %s: %w\n%s", sp.Name, err, string(out))
	}

	topCPUOut, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", "-nodecount=80", binPath, cpuProf).CombinedOutput()
	if err != nil {
		return suiteReport{}, fmt.Errorf("pprof top cpu: %w\n%s", err, string(topCPUOut))
	}

	topMemOut, _ := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", "-alloc_space", "-nodecount=10", binPath, memProf).CombinedOutput()
	allocLines := parseTopAllocLines(string(topMemOut))

	totalSec, entries := parsePprofTop(string(topCPUOut))
	hotReports := make([]hotFuncReport, 0, topN)

	for _, e := range entries {
		if len(hotReports) >= topN {
			break
		}
		if e.flatPct < minFlatPct || !strings.Contains(e.funcName, "paulourio/goblin") {
			continue
		}
		if strings.Contains(e.funcName, "_test.") {
			continue
		}

		shortSym := shortSymbolName(e.funcName)
		spMatch := matchFuncSpan(spans, e.funcName, shortSym)
		var st staticFuncDiag
		if spMatch != nil {
			if d, ok := diagByFunc[spMatch.File+":"+spMatch.FullName]; ok {
				st = *d
			}
		}

		sampledLines, hotBCEMisses := collectHotLines(ctx, binPath, cpuProf, shortSym, st.BCELines, st.EscapeDetails)
		disasm := collectDisasmMetrics(ctx, binPath, shortSym)

		f := e.flatPct / pctScale
		c := e.cumPct / pctScale
		rep := hotFuncReport{
			Rank:            len(hotReports) + 1,
			FuncName:        e.funcName,
			ShortName:       shortSym,
			File:            st.File,
			StartLine:       st.StartLine,
			EndLine:         st.EndLine,
			FlatSec:         e.flatSec,
			FlatPct:         e.flatPct,
			CumSec:          e.cumSec,
			CumPct:          e.cumPct,
			AmdahlFlat15x:   amdahlSpeedup(f, amdahlFactor15x),
			AmdahlFlat20x:   amdahlSpeedup(f, amdahlFactor20x),
			AmdahlFlatMax:   amdahlCeiling(f),
			AmdahlCum20x:    amdahlSpeedup(c, amdahlFactor20x),
			Static:          st,
			HotLineBCEMiss:  hotBCEMisses,
			Disasm:          disasm,
			TopSampledLines: sampledLines,
		}
		rep.Findings = synthesizeFindings(rep)
		hotReports = append(hotReports, rep)
	}

	return suiteReport{
		Suite:          sp.Name,
		Package:        sp.Pkg,
		BenchRegex:     sp.Bench,
		SIMDEnabled:    useSIMD,
		TotalSamplesS:  totalSec,
		HotFunctions:   hotReports,
		TopAllocations: allocLines,
	}, nil
}

type pprofEntry struct {
	flatSec  float64
	flatPct  float64
	cumSec   float64
	cumPct   float64
	funcName string
}

func parsePprofTop(raw string) (float64, []pprofEntry) {
	var totalSec float64
	if m := rePprofTotal.FindStringSubmatch(raw); m != nil {
		totalSec = parseDurationUnit(m[1], m[2])
	}
	var out []pprofEntry
	for line := range strings.SplitSeq(raw, "\n") {
		m := rePprofTopLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		flatSec := parseDurationUnit(m[1], m[2])
		flatPct, _ := strconv.ParseFloat(m[3], 64)
		cumSec := parseDurationUnit(m[5], m[6])
		cumPct, _ := strconv.ParseFloat(m[7], 64)
		fn := strings.TrimSpace(m[8])
		out = append(out, pprofEntry{
			flatSec:  flatSec,
			flatPct:  flatPct,
			cumSec:   cumSec,
			cumPct:   cumPct,
			funcName: fn,
		})
	}
	return totalSec, out
}

func parseTopAllocLines(raw string) []string {
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "paulourio/goblin") && !strings.Contains(trimmed, "_test.") {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseDurationUnit(valStr, unit string) float64 {
	v, _ := strconv.ParseFloat(valStr, 64)
	if unit == "ms" {
		return v / 1000.0
	}
	return v
}

func parseSampleMS(tok string) float64 {
	if tok == "." || tok == "" {
		return 0
	}
	if v, ok := strings.CutSuffix(tok, "ms"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	if v, ok := strings.CutSuffix(tok, "s"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f * 1000.0
	}
	return 0
}

func shortSymbolName(full string) string {
	if _, after, ok := strings.CutLast(full, "/"); ok {
		full = after
	}
	if _, after, ok := strings.Cut(full, "."); ok {
		return after
	}
	return full
}

func symbolTail(shortSym string) string {
	if _, after, ok := strings.CutLast(shortSym, "."); ok {
		return after
	}
	return shortSym
}

func matchFuncSpan(spans []funcSpan, pprofSym, shortSym string) *funcSpan {
	for i := range spans {
		s := &spans[i]
		if s.FullName == shortSym || s.ShortName == shortSym {
			if strings.Contains(pprofSym, "/assign.") && !strings.HasPrefix(s.File, "assign/") {
				continue
			}
			if strings.Contains(pprofSym, "/internal/simd.") && !strings.HasPrefix(s.File, "internal/simd/") {
				continue
			}
			return s
		}
	}
	return nil
}

func collectHotLines(
	ctx context.Context,
	binPath, cpuProf, shortSym string,
	bceLines []int,
	escDetails []string,
) ([]hotLineSample, int) {
	pattern := regexp.QuoteMeta(symbolTail(shortSym)) + "$"
	out, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-list="+pattern, binPath, cpuProf).CombinedOutput()
	if err != nil {
		return nil, 0
	}

	escLineSet := make(map[int]bool)
	for _, ed := range escDetails {
		if strings.HasPrefix(ed, "L") {
			if colon := strings.IndexByte(ed, ':'); colon > 1 {
				ln, _ := strconv.Atoi(ed[1:colon])
				escLineSet[ln] = true
			}
		}
	}

	var samples []hotLineSample
	hotBCEMisses := 0
	inTarget := false
	for line := range strings.SplitSeq(string(out), "\n") {
		if rm := rePprofListRout.FindStringSubmatch(line); rm != nil {
			inTarget = strings.HasSuffix(rm[1], "."+shortSym)
			continue
		}
		if !inTarget {
			continue
		}
		lm := rePprofListLine.FindStringSubmatch(line)
		if lm == nil {
			continue
		}
		flatMS := parseSampleMS(lm[1])
		cumMS := parseSampleMS(lm[2])
		if flatMS == 0 && cumMS == 0 {
			continue
		}
		ln, _ := strconv.Atoi(lm[3])
		hasBCE := slices.Contains(bceLines, ln)
		if hasBCE && flatMS > 0 {
			hotBCEMisses++
		}
		samples = append(samples, hotLineSample{
			Line:      ln,
			FlatMS:    flatMS,
			CumMS:     cumMS,
			HasBCE:    hasBCE,
			HasEscape: escLineSet[ln],
			Source:    strings.TrimSpace(lm[4]),
		})
	}

	slices.SortFunc(samples, func(a, b hotLineSample) int {
		if a.FlatMS != b.FlatMS {
			if a.FlatMS > b.FlatMS {
				return -1
			}
			return 1
		}
		if a.CumMS > b.CumMS {
			return -1
		}
		if a.CumMS < b.CumMS {
			return 1
		}
		return a.Line - b.Line
	})
	if len(samples) > defaultHotLines {
		samples = samples[:defaultHotLines]
	}
	return samples, hotBCEMisses
}

func collectDisasmMetrics(ctx context.Context, binPath, shortSym string) disasmMetrics {
	pattern := regexp.QuoteMeta(symbolTail(shortSym)) + "$"
	out, err := exec.CommandContext(ctx, "go", "tool", "objdump", "-s", pattern, binPath).CombinedOutput()
	if err != nil {
		return disasmMetrics{}
	}

	var m disasmMetrics
	seenCalls := make(map[string]bool)
	inTarget := false

	for line := range strings.SplitSeq(string(out), "\n") {
		if hm := reObjdumpHeader.FindStringSubmatch(line); hm != nil {
			inTarget = strings.HasSuffix(hm[1], "."+shortSym)
			continue
		}
		if !inTarget {
			continue
		}
		im := reObjdumpInst.FindStringSubmatch(line)
		if im == nil {
			continue
		}
		op := im[1]
		args := im[2]
		m.InstCount++

		if strings.Contains(args, "(SP)") {
			m.StackSpillRefs++
		}
		if isScalarFPOp(op) {
			m.ScalarFPOps++
		} else if isVectorFPOp(op) {
			m.VectorFPOps++
		}

		if op == "CALL" {
			cm := reCallTarget.FindStringSubmatch(line)
			if cm == nil {
				continue
			}
			target := cm[1]
			switch {
			case strings.HasPrefix(target, "runtime.panicBounds"),
				strings.HasPrefix(target, "runtime.panicIndex"),
				strings.HasPrefix(target, "runtime.panicSlice"):
				m.PanicBoundsCalls++
			case strings.HasPrefix(target, "runtime.growslice"):
				m.GrowSliceCalls++
			case strings.HasPrefix(target, "runtime.gcWriteBarrier"):
				m.WriteBarriers++
			case strings.HasPrefix(target, "runtime.newobject"),
				strings.HasPrefix(target, "runtime.makeslice"),
				strings.HasPrefix(target, "runtime.mallocgc"):
				m.AllocCalls++
			case strings.HasPrefix(target, "runtime.morestack"):
				// Ignore function prologue stack-split calls.
			default:
				if !seenCalls[target] {
					seenCalls[target] = true
					m.NonInlinedCalls = append(m.NonInlinedCalls, target)
				}
			}
		}
	}
	return m
}

func isScalarFPOp(op string) bool {
	switch op {
	case "ADDSD", "SUBSD", "MULSD", "DIVSD", "MINSD", "MAXSD", "SQRTSD", "UCOMISD",
		"VADDSD", "VSUBSD", "VMULSD", "VDIVSD", "VFMADD231SD", "VFMADD132SD", "VFMADD213SD":
		return true
	default:
		return false
	}
}

func isVectorFPOp(op string) bool {
	switch op {
	case "VADDPD", "VSUBPD", "VMULPD", "VDIVPD", "VMINPD", "VMAXPD", "VCMPPD",
		"VFMADD231PD", "VFMADD132PD", "VFMADD213PD", "VBROADCASTSD":
		return true
	default:
		return false
	}
}

func amdahlSpeedup(frac, factor float64) float64 {
	if frac <= 0 || factor <= 1 {
		return 1.0
	}
	if frac >= 1 {
		return factor
	}
	return 1.0 / (1.0 - frac*(1.0-1.0/factor))
}

func amdahlCeiling(frac float64) float64 {
	if frac <= 0 {
		return 1.0
	}
	if frac >= 0.99 {
		return 100.0
	}
	return 1.0 / (1.0 - frac)
}

func synthesizeFindings(r hotFuncReport) []string {
	var findings []string
	if r.HotLineBCEMiss > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d CPU-sampled hot line(s) contain surviving bounds checks (%d total BCE misses, %d panicBounds calls in binary)",
			r.HotLineBCEMiss, r.Static.BCECount, r.Disasm.PanicBoundsCalls,
		))
	} else if r.Disasm.PanicBoundsCalls > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d surviving runtime.panicBounds call sites in compiled disassembly (%d static BCE misses)",
			r.Disasm.PanicBoundsCalls, r.Static.BCECount,
		))
	}
	if r.Disasm.GrowSliceCalls > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d runtime.growslice call sites from append() inside function; pre-sized index writes avoid capacity checks and register spills",
			r.Disasm.GrowSliceCalls,
		))
	}
	if r.Disasm.WriteBarriers > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d GC write-barrier calls (runtime.gcWriteBarrier) from mutating heap-resident struct pointer/slice fields",
			r.Disasm.WriteBarriers,
		))
	}
	if r.Static.EscapeCount > 0 || r.Disasm.AllocCalls > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d heap escape(s) and %d runtime allocation call(s) inside hot function",
			r.Static.EscapeCount, r.Disasm.AllocCalls,
		))
	}
	if !r.Static.CanInline && r.Static.InlineCost > 0 && r.Static.InlineCost <= nearInlineCostLimit {
		findings = append(findings, fmt.Sprintf(
			"near-miss inlining candidate: cost %d exceeds budget %d by %d",
			r.Static.InlineCost, inlineBudgetDefault, r.Static.InlineCost-inlineBudgetDefault,
		))
	}
	if len(r.Disasm.NonInlinedCalls) > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d non-inlined call target(s): %s",
			len(r.Disasm.NonInlinedCalls), strings.Join(r.Disasm.NonInlinedCalls, ", "),
		))
	}
	return findings
}

func buildEnv(useSIMD bool) []string {
	env := os.Environ()
	if !useSIMD {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GOEXPERIMENT=") {
			out = append(out, kv)
		}
	}
	return append(out, "GOEXPERIMENT=simd")
}

func printTerminalReport(rep fullReport) {
	rule := strings.Repeat("=", tableWidth)
	thin := strings.Repeat("-", tableWidth)
	fmt.Println(rule)
	fmt.Printf("GOBLIN COMPILER & HOT-PATH OPTIMIZATION AUDIT (Amdahl's Law Prioritized) | SIMD=%v\n", rep.SIMDEnabled)
	fmt.Printf("Repo-wide Static Totals: %d BCE misses | %d heap escapes | %d near-miss inline functions (cost <= %d)\n",
		rep.TotalRepoBCEMisses, rep.TotalRepoHeapEscapes, len(rep.NearInlineCandidates), nearInlineCostLimit)
	fmt.Println(rule)

	if len(rep.NearInlineCandidates) > 0 {
		fmt.Println("\nNEAR-MISS INLINING CANDIDATES (Budget = 80):")
		for _, c := range rep.NearInlineCandidates {
			fmt.Printf("  - %s:%d  %-28s  cost=%d (+%d over budget)\n",
				c.File, c.Line, c.FuncName, c.Cost, c.OverBy)
		}
	}

	for _, sr := range rep.Suites {
		fmt.Printf("\n%s\n", rule)
		fmt.Printf("SUITE: %-10s | Pkg: %-10s | Total CPU Samples: %.2fs\n", sr.Suite, sr.Package, sr.TotalSamplesS)
		fmt.Println(thin)
		fmt.Printf("%-3s %-32s %7s %7s %8s %8s %8s %5s %6s %5s %6s %7s\n",
			"#", "Function", "Flat%", "Cum%", "Amd1.5x", "Amd2.0x", "AmdMax", "BCE", "Panics", "Grow", "Spills", "FP(S/V)")
		fmt.Println(thin)

		for _, hf := range sr.HotFunctions {
			fpMix := fmt.Sprintf("%d/%d", hf.Disasm.ScalarFPOps, hf.Disasm.VectorFPOps)
			fmt.Printf("%-3d %-32s %6.2f%% %6.2f%% %7.3fx %7.3fx %7.3fx %5d %6d %5d %6d %7s\n",
				hf.Rank,
				truncateStr(hf.ShortName, 32),
				hf.FlatPct,
				hf.CumPct,
				hf.AmdahlFlat15x,
				hf.AmdahlFlat20x,
				hf.AmdahlFlatMax,
				hf.Static.BCECount,
				hf.Disasm.PanicBoundsCalls,
				hf.Disasm.GrowSliceCalls,
				hf.Disasm.StackSpillRefs,
				fpMix,
			)
		}

		fmt.Println(thin)
		fmt.Println("HOT-PATH COMPILER & DISASSEMBLY DIAGNOSTICS:")
		for _, hf := range sr.HotFunctions {
			loc := hf.File
			if hf.StartLine > 0 {
				loc = fmt.Sprintf("%s:%d-%d", hf.File, hf.StartLine, hf.EndLine)
			}
			fmt.Printf("\n[%d] %s (%s) — Flat: %.2f%% (%.2fs), Cum: %.2f%% (%.2fs), Amdahl 2x: %.3fx (Cum 2x: %.3fx)\n",
				hf.Rank, hf.ShortName, loc, hf.FlatPct, hf.FlatSec, hf.CumPct, hf.CumSec, hf.AmdahlFlat20x, hf.AmdahlCum20x)
			for _, f := range hf.Findings {
				fmt.Printf("    * %s\n", f)
			}
			if len(hf.TopSampledLines) > 0 {
				fmt.Println("    Top CPU-sampled source lines:")
				for _, sl := range hf.TopSampledLines {
					tags := ""
					if sl.HasBCE {
						tags += " [BCE-MISS]"
					}
					if sl.HasEscape {
						tags += " [HEAP-ESCAPE]"
					}
					fmt.Printf("      L%-4d  flat=%4.0fms  cum=%4.0fms%-14s  %s\n",
						sl.Line, sl.FlatMS, sl.CumMS, tags, truncateStr(sl.Source, 72))
				}
			}
		}
	}
	fmt.Println(rule)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
