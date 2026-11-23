// Command goblin-optcheck profiles Goblin benchmark suites, correlates CPU and allocation
// hotspots with Go compiler optimization diagnostics (bounds check elimination, inlining
// costs, and escape analysis) and x86-64 disassembly metrics, and ranks optimization
// targets by Amdahl's Law speedup ceiling.
package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
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
	"runtime"
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
	largeFrameBytes     = 512
	hexRadix            = 16
	hexAlphaBase        = 10
	maxHexDigits        = 7
	minRuntimeFlatPct   = 0.5
	minRuntimeCumPct    = 1.5
	maxRuntimeHot       = 8
	maxTopCallers       = 3
	peekCallerMaxFields = 2
	peekTargetMinFields = 4
	pprofTopMinFields   = 6
	msPerSec            = 1000.0
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
	InlineCost    int      `json:"inline_cost,omitzero"`
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
	HasBCE    bool    `json:"has_bce,omitzero"`
	HasEscape bool    `json:"has_escape,omitzero"`
	Source    string  `json:"source"`
}

type disasmMetrics struct {
	InstCount        int      `json:"inst_count"`
	FrameBytes       int      `json:"frame_bytes"`
	PanicBoundsCalls int      `json:"panic_bounds_calls"`
	GrowSliceCalls   int      `json:"grow_slice_calls"`
	WriteBarriers    int      `json:"write_barriers"`
	DeferReturnCalls int      `json:"defer_return_calls"`
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

type runtimeHotFunc struct {
	Symbol     string   `json:"symbol"`
	FlatMS     float64  `json:"flat_ms"`
	FlatPct    float64  `json:"flat_pct"`
	CumMS      float64  `json:"cum_ms"`
	CumPct     float64  `json:"cum_pct"`
	Inlined    bool     `json:"inlined,omitempty"`
	TopCallers []string `json:"top_callers,omitempty"`
}

type vectorHazard struct {
	Func string `json:"func"`
	Addr string `json:"addr"`
	Kind string `json:"kind"`
	Insn string `json:"insn"`
}

type vectorAuditReport struct {
	Available       bool           `json:"available"`
	EVEXFuncCount   int            `json:"evex_func_count"`
	VEXYMMFuncCount int            `json:"vex_ymm_func_count"`
	EVEXFuncs       []string       `json:"evex_funcs,omitempty"`
	UnguardedEVEX   []string       `json:"unguarded_evex,omitempty"`
	Hazards         []vectorHazard `json:"hazards,omitempty"`
}

type suiteReport struct {
	Suite            string             `json:"suite"`
	Package          string             `json:"package"`
	BenchRegex       string             `json:"bench_regex"`
	SIMDEnabled      bool               `json:"simd_enabled"`
	TotalSamplesS    float64            `json:"total_samples_sec"`
	HotFunctions     []hotFuncReport    `json:"hot_functions"`
	RuntimeStdlibHot []runtimeHotFunc   `json:"runtime_stdlib_hot,omitempty"`
	VectorAudit      *vectorAuditReport `json:"vector_audit,omitempty"`
	TopAllocations   []string           `json:"top_allocations,omitempty"`
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
	reDiagLoc         = regexp.MustCompile(`^(\./)?([^:\s]+\.go):(\d+):(\d+):\s*(.+)$`)
	reCannotInline    = regexp.MustCompile(`^cannot inline ([^:]+): (.+)$`)
	reCanInline       = regexp.MustCompile(`^can inline ([^ ]+) with cost (\d+)`)
	reCostBudget      = regexp.MustCompile(`cost (\d+) exceeds budget (\d+)`)
	rePprofTopLine    = regexp.MustCompile(`^\s*([0-9.]+)(ms|s)\s+([0-9.]+)%\s+([0-9.]+)%\s+([0-9.]+)(ms|s)\s+([0-9.]+)%\s+(\S+)`)
	rePprofTotal      = regexp.MustCompile(`Total samples = ([0-9.]+)(ms|s)`)
	rePprofListRout   = regexp.MustCompile(`^ROUTINE ======================== (\S+) in (.+)$`)
	rePprofListLine   = regexp.MustCompile(`^\s*([0-9.]+(?:ms|s)|\.)\s+([0-9.]+(?:ms|s)|\.)\s+(\d+):(.*)$`)
	reObjdumpHeader   = regexp.MustCompile(`^TEXT\s+(\S+)\(SB\)`)
	reObjdumpInst     = regexp.MustCompile(`^\s*(?:\S+:\d+\s+)?0x[0-9a-f]+\s+[0-9a-f]+\s+([A-Z0-9_]+)\s*(.*)$`)
	reCallTarget      = regexp.MustCompile(`CALL\s+([^\s(]+)\(SB\)`)
	reSubSP           = regexp.MustCompile(`^\$(0x[0-9a-fA-F]+|[0-9]+),\s*SP\b`)
	reObjdumpAddrFunc = regexp.MustCompile(`^([0-9a-f]+)\s+<([^>]+)>:$`)
	reObjdumpInsn     = regexp.MustCompile(`^\s*([0-9a-f]+):\s+([A-Za-z0-9.]+)(?:\s+(.*))?$`)
	reOpmaskReg       = regexp.MustCompile(`%k[0-7]\b`)
	reHighSimdReg     = regexp.MustCompile(`%[xy]mm(?:1[6-9]|2[0-9]|3[01])\b`)
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
	checkInvariants := flag.Bool("check-invariants", false, "verify inlining, BCE, frame/defer/growslice, and vector CFG locks without running benchmarks")
	jsonOut := flag.String("json", "", "optional path to write structured JSON report")
	flag.Parse()

	if *checkInvariants {
		ctx := context.Background()
		res, err := checkCodegenInvariants(ctx, ".", *useSIMD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invariant check failed: %v\n", err)
			return 1
		}
		if len(res.Violations) > 0 {
			for _, v := range res.Violations {
				fmt.Fprintf(os.Stderr, "VIOLATION: %s\n", v)
			}
			return 1
		}
		if *useSIMD && runtime.GOARCH == "amd64" && res.VectorAudits == 0 {
			fmt.Fprintln(os.Stderr, "VIOLATION: vector CFG audit unavailable (GNU objdump missing or failed)")
			return 1
		}
		fmt.Printf("goblin-optcheck invariants PASS (simd=%v): inline=%d noinline=%d bce_loops=%d disasm=%d vector_audits=%d\n",
			*useSIMD, res.CheckedInline, res.CheckedNoinline, res.CheckedBCELoops, res.CheckedDisasm, res.VectorAudits)
		return 0
	}

	if *topN < 0 {
		fmt.Fprintf(os.Stderr, "-top must be non-negative (got %d)\n", *topN)
		return exitCodeBadArgument
	}

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
		raw, err := json.Marshal(&rep, jsontext.WithIndent("  "))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*jsonOut, append(raw, '\n'), reportPerm); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *jsonOut, err)
			return 1
		}
	}
	if *useSIMD && runtime.GOARCH == "amd64" {
		hasVectorFailure := false
		for _, sr := range rep.Suites {
			if sr.VectorAudit == nil || !sr.VectorAudit.Available {
				fmt.Fprintf(os.Stderr, "VIOLATION: vector CFG audit unavailable for suite %s\n", sr.Suite)
				hasVectorFailure = true
				continue
			}
			if len(sr.VectorAudit.UnguardedEVEX) > 0 {
				fmt.Fprintf(os.Stderr, "VIOLATION: unguarded EVEX in suite %s: %s\n",
					sr.Suite, strings.Join(sr.VectorAudit.UnguardedEVEX, ", "))
				hasVectorFailure = true
			}
			for _, hz := range sr.VectorAudit.Hazards {
				fmt.Fprintf(os.Stderr, "VIOLATION: vector CFG hazard in suite %s [%s] %s @ %s: %s\n",
					sr.Suite, hz.Kind, hz.Func, hz.Addr, hz.Insn)
				hasVectorFailure = true
			}
		}
		if hasVectorFailure {
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
		if useSIMD && strings.HasSuffix(path, "_scalar.go") {
			return nil
		}
		if !useSIMD && strings.HasSuffix(path, "_amd64.go") {
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

func recordCannotInlineDiag(
	sp *funcSpan,
	d *staticFuncDiag,
	file string,
	lineNum int,
	fnName, reason string,
	seenNear map[string]bool,
	nearInline []nearInlineCandidate,
) []nearInlineCandidate {
	if sp == nil || sp.StartLine != lineNum {
		return nearInline
	}
	d.CanInline = false
	d.InlineReason = reason
	bm := reCostBudget.FindStringSubmatch(reason)
	if bm == nil {
		return nearInline
	}
	cost, _ := strconv.Atoi(bm[1])
	budget, _ := strconv.Atoi(bm[2])
	d.InlineCost = cost
	if cost > nearInlineCostLimit || budget != inlineBudgetDefault {
		return nearInline
	}
	key := fmt.Sprintf("%s:%d:%s", file, lineNum, fnName)
	if seenNear[key] {
		return nearInline
	}
	seenNear[key] = true
	return append(nearInline, nearInlineCandidate{
		File:      file,
		Line:      lineNum,
		FuncName:  fnName,
		Cost:      cost,
		Budget:    budget,
		OverBy:    cost - budget,
		RawReason: reason,
	})
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
		sp := findEnclosingSpan(spans, file, lineNum)
		var d *staticFuncDiag
		if sp != nil {
			d = diagMap[sp.File+":"+sp.FullName]
		}

		if cm := reCanInline.FindStringSubmatch(msg); cm != nil {
			if d != nil && sp != nil && sp.StartLine == lineNum {
				cost, _ := strconv.Atoi(cm[2])
				d.CanInline = true
				d.InlineCost = cost
			}
			continue
		}

		if cm := reCannotInline.FindStringSubmatch(msg); cm != nil {
			nearInline = recordCannotInlineDiag(sp, d, file, lineNum, cm[1], cm[2], seenNear, nearInline)
			continue
		}

		if strings.Contains(msg, "Found IsInBounds") || strings.Contains(msg, "Found IsSliceInBounds") {
			bceKey := fmt.Sprintf("%s:%d:%s", file, lineNum, m[4])
			if !seenBCE[bceKey] {
				seenBCE[bceKey] = true
				totalBCE++
				if d != nil {
					d.BCECount++
					if !slices.Contains(d.BCELines, lineNum) {
						d.BCELines = append(d.BCELines, lineNum)
					}
				}
			}
			continue
		}

		if (strings.Contains(msg, "escapes to heap") || strings.Contains(msg, "moved to heap")) && !strings.Contains(msg, " in ") {
			escKey := fmt.Sprintf("%s:%d:%s", file, lineNum, msg)
			if !seenEsc[escKey] {
				seenEsc[escKey] = true
				totalEsc++
				if d != nil {
					d.EscapeCount++
					d.EscapeDetails = append(d.EscapeDetails, fmt.Sprintf("L%d: %s", lineNum, msg))
				}
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
	if absTmp, err := filepath.Abs(tmpDir); err == nil {
		tmpDir = absTmp
	}
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
	if runDir := resolveRunDir(sp.Pkg); runDir != "" && runDir != "." {
		runCmd.Dir = runDir
	}
	if out, err := runCmd.CombinedOutput(); err != nil {
		return suiteReport{}, fmt.Errorf("benchmark %s: %w\n%s", sp.Name, err, string(out))
	}

	topCPUOut, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", "-nodecount=80", binPath, cpuProf).CombinedOutput()
	if err != nil {
		return suiteReport{}, fmt.Errorf("pprof top cpu: %w\n%s", err, string(topCPUOut))
	}

	topMemOut, _ := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", "-alloc_space", "-nodecount=10", binPath, memProf).CombinedOutput()
	allocLines := parseTopAllocLines(string(topMemOut))

	runtimeHot := parseRuntimeStdlibHot(string(topCPUOut))
	if len(runtimeHot) > 0 {
		targets := make([]string, len(runtimeHot))
		for i, rh := range runtimeHot {
			targets[i] = rh.Symbol
		}
		callersByTarget := collectTopCallers(ctx, binPath, cpuProf, targets)
		for i := range runtimeHot {
			base := strings.TrimSuffix(runtimeHot[i].Symbol, " (inline)")
			runtimeHot[i].TopCallers = callersByTarget[base]
		}
	}

	var vectorAudit *vectorAuditReport
	if useSIMD {
		va := collectVectorAudit(ctx, binPath)
		vectorAudit = &va
	}

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
		Suite:            sp.Name,
		Package:          sp.Pkg,
		BenchRegex:       sp.Bench,
		SIMDEnabled:      useSIMD,
		TotalSamplesS:    totalSec,
		HotFunctions:     hotReports,
		RuntimeStdlibHot: runtimeHot,
		VectorAudit:      vectorAudit,
		TopAllocations:   allocLines,
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

func pprofPackageDir(pprofSym string) string {
	const modPrefix = "github.com/paulourio/goblin"
	_, rest, ok := strings.Cut(pprofSym, modPrefix)
	if !ok {
		return ""
	}
	if strings.HasPrefix(rest, ".") {
		return "."
	}
	if rest, ok = strings.CutPrefix(rest, "/"); ok {
		pkgPath, _, found := strings.Cut(rest, ".")
		if found {
			return filepath.Clean(pkgPath)
		}
	}
	return ""
}

func matchFuncSpan(spans []funcSpan, pprofSym, shortSym string) *funcSpan {
	wantDir := pprofPackageDir(pprofSym)
	for i := range spans {
		s := &spans[i]
		if s.FullName != shortSym && s.ShortName != shortSym {
			continue
		}
		if wantDir != "" && filepath.Dir(s.File) != wantDir {
			continue
		}
		return s
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
		flatMS := parseSampleTokenMS(lm[1])
		cumMS := parseSampleTokenMS(lm[2])
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

func resolveRunDir(pkg string) string {
	p := strings.TrimSpace(pkg)
	if p == "" {
		return "."
	}
	p = strings.TrimSuffix(p, "/...")
	if p == "" || p == "." || p == "..." {
		return "."
	}
	const modPrefix = "github.com/paulourio/goblin"
	if after, ok := strings.CutPrefix(p, modPrefix); ok {
		after = strings.TrimPrefix(after, "/")
		if after == "" {
			return "."
		}
		return filepath.Clean(after)
	}
	return filepath.Clean(p)
}

func parseImmBytes(s string) (int, bool) {
	if hexStr, ok := strings.CutPrefix(s, "0x"); ok {
		if hexStr == "" || len(hexStr) > maxHexDigits {
			return 0, false
		}
		val := 0
		for i := range len(hexStr) {
			c := hexStr[i]
			var digit int
			switch {
			case c >= '0' && c <= '9':
				digit = int(c - '0')
			case c >= 'a' && c <= 'f':
				digit = int(c-'a') + hexAlphaBase
			case c >= 'A' && c <= 'F':
				digit = int(c-'A') + hexAlphaBase
			default:
				return 0, false
			}
			val = val*hexRadix + digit
		}
		return val, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func collectDisasmMetrics(ctx context.Context, binPath, shortSym string) disasmMetrics {
	pattern := regexp.QuoteMeta(symbolTail(shortSym)) + "$"
	out, err := exec.CommandContext(ctx, "go", "tool", "objdump", "-s", pattern, binPath).CombinedOutput()
	if err != nil {
		return disasmMetrics{}
	}
	return parseDisasmMetrics(string(out), shortSym)
}

func parseDisasmMetrics(raw, shortSym string) disasmMetrics {
	var m disasmMetrics
	seenCalls := make(map[string]bool)
	inTarget := false

	for line := range strings.SplitSeq(raw, "\n") {
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
		args := strings.TrimSpace(im[2])
		m.InstCount++

		if op == "SUBQ" {
			if sm := reSubSP.FindStringSubmatch(args); sm != nil {
				if nb, ok := parseImmBytes(sm[1]); ok && nb > m.FrameBytes {
					m.FrameBytes = nb
				}
			}
		}

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
			case strings.HasPrefix(target, "runtime.deferreturn"):
				m.DeferReturnCalls++
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

func parseSampleTokenMS(tok string) float64 {
	if tok == "." || tok == "" || tok == "0" {
		return 0
	}
	if v, ok := strings.CutSuffix(tok, "ms"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	if v, ok := strings.CutSuffix(tok, "s"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f * msPerSec
	}
	return 0
}

func parseRuntimeStdlibHot(pprofTopOut string) []runtimeHotFunc {
	const modPrefix = "github.com/paulourio/goblin"
	var out []runtimeHotFunc
	seen := make(map[string]bool)

	for line := range strings.SplitSeq(pprofTopOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) < pprofTopMinFields {
			continue
		}
		if !strings.HasSuffix(fields[1], "%") || !strings.HasSuffix(fields[4], "%") {
			continue
		}
		flatPct, err1 := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64)
		cumPct, err2 := strconv.ParseFloat(strings.TrimSuffix(fields[4], "%"), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if flatPct < minRuntimeFlatPct && cumPct < minRuntimeCumPct {
			continue
		}
		sym := strings.Join(fields[5:], " ")
		if strings.HasPrefix(sym, modPrefix) || strings.HasPrefix(sym, "testing.") {
			continue
		}
		baseSym, inlined := strings.CutSuffix(sym, " (inline)")
		if seen[baseSym] {
			continue
		}
		seen[baseSym] = true
		out = append(out, runtimeHotFunc{
			Symbol:  sym,
			FlatMS:  parseSampleTokenMS(fields[0]),
			FlatPct: flatPct,
			CumMS:   parseSampleTokenMS(fields[3]),
			CumPct:  cumPct,
			Inlined: inlined,
		})
	}

	slices.SortFunc(out, func(a, b runtimeHotFunc) int {
		if a.FlatPct != b.FlatPct {
			if a.FlatPct > b.FlatPct {
				return -1
			}
			return 1
		}
		if a.CumPct != b.CumPct {
			if a.CumPct > b.CumPct {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Symbol, b.Symbol)
	})
	if len(out) > maxRuntimeHot {
		out = out[:maxRuntimeHot]
	}
	return out
}

func formatCallerLabel(rawSym, pctStr string) string {
	const modDot = "github.com/paulourio/goblin."
	const modSlash = "github.com/paulourio/goblin/"
	s := strings.TrimSpace(rawSym)
	s = strings.TrimPrefix(s, modDot)
	s = strings.TrimPrefix(s, modSlash)
	return s + " (" + pctStr + ")"
}

func parsePprofPeekCallers(peekOut string) map[string][]string {
	result := make(map[string][]string)

	var blockCallers []string
	targetBase := ""

	flushBlock := func() {
		if targetBase != "" && len(blockCallers) > 0 {
			existing := result[targetBase]
			for _, c := range blockCallers {
				if len(existing) >= maxTopCallers {
					break
				}
				if !slices.Contains(existing, c) {
					existing = append(existing, c)
				}
			}
			result[targetBase] = existing
		}
		blockCallers = nil
		targetBase = ""
	}

	for line := range strings.SplitSeq(peekOut, "\n") {
		if strings.Contains(line, "---+---") {
			flushBlock()
			continue
		}
		left, right, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		lFields := strings.Fields(left)
		sym := strings.TrimSpace(right)
		if sym == "" || strings.HasPrefix(sym, "calls") {
			continue
		}
		if len(lFields) >= peekTargetMinFields {
			targetBase = strings.TrimSuffix(sym, " (inline)")
			continue
		}
		if targetBase == "" && len(lFields) == peekCallerMaxFields {
			blockCallers = append(blockCallers, formatCallerLabel(sym, lFields[1]))
		}
	}
	flushBlock()
	return result
}

func collectTopCallers(ctx context.Context, binPath, cpuProf string, targets []string) map[string][]string {
	if len(targets) == 0 {
		return nil
	}
	quoted := make([]string, 0, len(targets))
	for _, t := range targets {
		base := strings.TrimSuffix(t, " (inline)")
		quoted = append(quoted, regexp.QuoteMeta(base))
	}
	pattern := "^(" + strings.Join(quoted, "|") + ")( \\(inline\\))?$"
	out, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-peek="+pattern, binPath, cpuProf).CombinedOutput()
	if err != nil {
		return nil
	}
	return parsePprofPeekCallers(string(out))
}

type objdumpInsn struct {
	addr string
	op   string
	args string
}

func normalizeHexAddr(addr string) string {
	s := strings.ToLower(strings.TrimSpace(addr))
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

func isX86PrefixToken(tok string) bool {
	switch tok {
	case "notrack", "bnd", "rep", "repz", "repe", "repne", "repnz", "lock",
		"cs", "ds", "es", "fs", "gs", "ss",
		"data16", "data32", "addr16", "addr32",
		"xacquire", "xrelease", "rex":
		return true
	default:
		return strings.HasPrefix(tok, "rex.")
	}
}

func splitFirstWord(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, "", true
	}
	return s[:idx], strings.TrimSpace(s[idx+1:]), true
}

func normalizeX86PrefixOpArgs(op, args string) (string, string) {
	opL := strings.ToLower(strings.TrimSpace(op))
	argsTrim := strings.TrimSpace(args)
	for range 16 {
		if !isX86PrefixToken(opL) {
			break
		}
		nextOp, nextArgs, ok := splitFirstWord(argsTrim)
		if !ok {
			break
		}
		opL = strings.ToLower(nextOp)
		argsTrim = nextArgs
	}
	return opL, argsTrim
}

func isEVEXInsn(op, args string) bool {
	opL, argsEff := normalizeX86PrefixOpArgs(op, args)
	argsL := strings.ToLower(argsEff)
	if strings.Contains(argsL, "%zmm") ||
		reOpmaskReg.MatchString(argsL) ||
		reHighSimdReg.MatchString(argsL) ||
		strings.Contains(argsL, "{1to") ||
		strings.Contains(argsL, "{z}") ||
		strings.Contains(argsL, "{rn-") ||
		strings.Contains(argsL, "{rz-") ||
		strings.Contains(argsL, "{rd-") ||
		strings.Contains(argsL, "{ru-") ||
		strings.Contains(argsL, "{sae}") {
		return true
	}
	for _, prefix := range []string{
		"vpternlog", "vpermi2", "vpermt2", "vcompress", "vexpand",
		"vpblendm", "vmovdqu64", "vmovdqu32", "vmovdqa64", "vmovdqa32",
		"vpcmpq", "vpcmpuq", "vscalef", "vrcp14", "vrsqrt14",
		"vrndscale", "vfixupimm", "vfpclass", "vptestm",
		"vpmovm2", "vpmovq2m", "vpmovd2m",
	} {
		if strings.HasPrefix(opL, prefix) {
			return true
		}
	}
	return false
}

func isVEXYMMInsn(op, args string) bool {
	opL, argsEff := normalizeX86PrefixOpArgs(op, args)
	argsL := strings.ToLower(argsEff)
	if isEVEXInsn(opL, argsL) {
		return false
	}
	if opL == "vzeroupper" || opL == "vzeroall" {
		return false
	}
	return strings.HasPrefix(opL, "v") && strings.Contains(argsL, "%ymm")
}

func isLegacySSEInsn(op, args string) bool {
	opL, argsEff := normalizeX86PrefixOpArgs(op, args)
	argsL := strings.ToLower(argsEff)
	return !strings.HasPrefix(opL, "v") && strings.Contains(argsL, "%xmm")
}

func dirtiesUpperVector(op, args string) bool {
	opL, argsEff := normalizeX86PrefixOpArgs(op, args)
	argsL := strings.ToLower(argsEff)
	if !strings.HasPrefix(opL, "v") || opL == "vzeroupper" || opL == "vzeroall" {
		return false
	}
	return strings.Contains(argsL, "%ymm") || strings.Contains(argsL, "%zmm") || isEVEXInsn(opL, argsL)
}

func isRetInsn(op, args string) bool {
	opL, _ := normalizeX86PrefixOpArgs(op, args)
	return opL == "ret" || opL == "retq"
}

func stripTrailingComment(s string) string {
	if before, _, ok := strings.Cut(s, "//"); ok {
		s = before
	}
	if before, _, ok := strings.Cut(s, "#"); ok {
		s = before
	}
	return s
}

func parseTargetSymbol(args string) string {
	fields := strings.Fields(stripTrailingComment(args))
	if len(fields) < 2 {
		return ""
	}
	symTok := fields[1]
	if !strings.HasPrefix(symTok, "<") || !strings.HasSuffix(symTok, ">") || len(symTok) < 3 {
		return ""
	}
	inner := symTok[1 : len(symTok)-1]
	base, _, _ := strings.Cut(inner, "+")
	return base
}

func isNoreturnRuntimeCall(args string) bool {
	sym := parseTargetSymbol(args)
	return strings.HasPrefix(sym, "runtime.panic") || strings.HasPrefix(sym, "runtime.gopanic")
}

func isMorestackTarget(args string) bool {
	sym := parseTargetSymbol(args)
	return strings.HasPrefix(sym, "runtime.morestack")
}

func isCpuFeatureCheck(args string) bool {
	for tok := range strings.FieldsSeq(stripTrailingComment(args)) {
		if strings.HasPrefix(tok, "<") && strings.HasSuffix(tok, ">") && len(tok) > 2 {
			inner := strings.ToLower(tok[1 : len(tok)-1])
			if strings.Contains(inner, "hasavx512") ||
				strings.Contains(inner, "x86hasavx512") ||
				strings.Contains(inner, "activetier") {
				return true
			}
		}
	}
	return false
}

func isCpuFeatureGuardAt(insns []objdumpInsn, idx int) bool {
	if idx < 0 || idx >= len(insns) {
		return false
	}
	op := insns[idx].op
	if op == "jmp" || op == "jmpq" || !strings.HasPrefix(op, "j") {
		return false
	}
	if idx >= 1 && isCpuFeatureCheck(insns[idx-1].args) {
		return true
	}
	if idx >= 2 && isCpuFeatureCheck(insns[idx-2].args) {
		prevOp := insns[idx-1].op
		if strings.HasPrefix(prevOp, "cmp") || strings.HasPrefix(prevOp, "test") {
			return true
		}
	}
	return false
}

func parseJumpTargetHex(args string) string {
	tok, _, ok := splitFirstWord(args)
	if !ok || tok == "" || strings.HasPrefix(tok, "*") || strings.HasPrefix(tok, "%") {
		return ""
	}
	hexTok := tok
	if strings.HasPrefix(hexTok, "0x") || strings.HasPrefix(hexTok, "0X") {
		hexTok = hexTok[2:]
	}
	if hexTok == "" {
		return ""
	}
	for i := range len(hexTok) {
		c := hexTok[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return ""
		}
	}
	return normalizeHexAddr(hexTok)
}

func auditFuncVectorCFG(funcName string, insns []objdumpInsn, report *vectorAuditReport) {
	hasEVEX := false
	hasVEXYMM := false
	for i := range insns {
		insns[i].op, insns[i].args = normalizeX86PrefixOpArgs(insns[i].op, insns[i].args)
		if isEVEXInsn(insns[i].op, insns[i].args) {
			hasEVEX = true
		} else if isVEXYMMInsn(insns[i].op, insns[i].args) {
			hasVEXYMM = true
		}
	}
	if !hasEVEX && !hasVEXYMM {
		return
	}
	if hasEVEX {
		report.EVEXFuncCount++
		report.EVEXFuncs = append(report.EVEXFuncs, funcName)
	}
	if hasVEXYMM {
		report.VEXYMMFuncCount++
	}

	recorded := make(map[string]bool)
	addHazard := func(ins objdumpInsn, kind string) {
		k := ins.addr + ":" + kind
		if recorded[k] {
			return
		}
		recorded[k] = true
		report.Hazards = append(report.Hazards, vectorHazard{
			Func: funcName,
			Addr: ins.addr,
			Kind: kind,
			Insn: ins.op + " " + ins.args,
		})
	}

	addrToIdx := make(map[string]int, len(insns))
	for i, ins := range insns {
		if _, dup := addrToIdx[ins.addr]; dup {
			addHazard(ins, "DUPLICATE_ADDR")
			continue
		}
		addrToIdx[ins.addr] = i
	}

	if hasEVEX && !strings.Contains(strings.ToLower(funcName), "avx512") {
		if checkUnguardedEVEX(insns, addrToIdx) {
			report.UnguardedEVEX = append(report.UnguardedEVEX, funcName)
		}
	}

	seen := make([]bool, len(insns)*2)
	type cfgState struct {
		idx   int
		dirty bool
	}
	queue := make([]cfgState, 0, len(insns))
	push := func(idx int, dirty bool) {
		if idx < 0 || idx >= len(insns) {
			return
		}
		key := idx * 2
		if dirty {
			key++
		}
		if !seen[key] {
			seen[key] = true
			queue = append(queue, cfgState{idx: idx, dirty: dirty})
		}
	}
	push(0, false)

	for head := 0; head < len(queue); {
		st := queue[head]
		head++
		ins := insns[st.idx]
		dirty := st.dirty
		isLast := st.idx+1 == len(insns)

		nextDirty := dirty
		switch {
		case ins.op == "vzeroupper" || ins.op == "vzeroall":
			nextDirty = false
		case dirtiesUpperVector(ins.op, ins.args):
			nextDirty = true
		case dirty && isLegacySSEInsn(ins.op, ins.args):
			addHazard(ins, "LEGACY_SSE_WHILE_DIRTY")
		}

		if isRetInsn(ins.op, ins.args) {
			if nextDirty {
				addHazard(ins, "DIRTY_RET")
			}
			continue
		}
		if ins.op == "ud2" || ins.op == "int3" {
			continue
		}
		if ins.op == "call" || ins.op == "callq" {
			if isNoreturnRuntimeCall(ins.args) {
				continue
			}
			if nextDirty && !isMorestackTarget(ins.args) {
				addHazard(ins, "DIRTY_CALL")
			} else if isLast && nextDirty {
				addHazard(ins, "DIRTY_FALL_OFF_END")
			}
			push(st.idx+1, nextDirty)
			continue
		}
		if ins.op == "jmp" || ins.op == "jmpq" {
			tgt := parseJumpTargetHex(ins.args)
			if tgt == "" {
				addHazard(ins, "INDIRECT_OR_UNRESOLVED_JMP")
				continue
			}
			if tgtIdx, ok := addrToIdx[tgt]; ok {
				push(tgtIdx, nextDirty)
			} else if nextDirty && !isMorestackTarget(ins.args) {
				addHazard(ins, "DIRTY_RET")
			}
			continue
		}
		if strings.HasPrefix(ins.op, "j") {
			tgt := parseJumpTargetHex(ins.args)
			if tgt == "" {
				addHazard(ins, "INDIRECT_OR_UNRESOLVED_JMP")
			} else if tgtIdx, ok := addrToIdx[tgt]; ok {
				push(tgtIdx, nextDirty)
			} else if nextDirty && !isMorestackTarget(ins.args) {
				addHazard(ins, "DIRTY_COND_EXT_JMP")
			}
			if isLast && nextDirty {
				addHazard(ins, "DIRTY_FALL_OFF_END")
			}
			push(st.idx+1, nextDirty)
			continue
		}
		if isLast && nextDirty {
			addHazard(ins, "DIRTY_FALL_OFF_END")
		}
		push(st.idx+1, nextDirty)
	}
}

func checkUnguardedEVEX(insns []objdumpInsn, addrToIdx map[string]int) bool {
	visited := make([]bool, len(insns))
	queue := make([]int, 0, len(insns))
	enqueue := func(idx int) {
		if idx >= 0 && idx < len(insns) && !visited[idx] {
			visited[idx] = true
			queue = append(queue, idx)
		}
	}
	enqueue(0)

	for head := 0; head < len(queue); {
		idx := queue[head]
		head++
		ins := insns[idx]
		if isEVEXInsn(ins.op, ins.args) {
			return true
		}
		if isRetInsn(ins.op, ins.args) || ins.op == "ud2" || ins.op == "int3" {
			continue
		}
		if ins.op == "call" || ins.op == "callq" {
			if isNoreturnRuntimeCall(ins.args) {
				continue
			}
			enqueue(idx + 1)
			continue
		}
		if ins.op == "jmp" || ins.op == "jmpq" {
			if tgt := parseJumpTargetHex(ins.args); tgt != "" {
				if tgtIdx, ok := addrToIdx[tgt]; ok {
					enqueue(tgtIdx)
				}
			}
			continue
		}
		if strings.HasPrefix(ins.op, "j") {
			if isCpuFeatureGuardAt(insns, idx) {
				continue
			}
			if tgt := parseJumpTargetHex(ins.args); tgt != "" {
				if tgtIdx, ok := addrToIdx[tgt]; ok {
					enqueue(tgtIdx)
				}
			}
			enqueue(idx + 1)
			continue
		}
		enqueue(idx + 1)
	}
	return false
}

func auditObjdumpVectorCFG(raw string) vectorAuditReport {
	const modPrefix = "github.com/paulourio/goblin"
	report := vectorAuditReport{Available: true}

	curFunc := ""
	var insns []objdumpInsn
	flushFunc := func() {
		if curFunc != "" && strings.Contains(curFunc, modPrefix) && len(insns) > 0 {
			auditFuncVectorCFG(curFunc, insns, &report)
		}
		curFunc = ""
		insns = insns[:0]
	}

	for line := range strings.SplitSeq(raw, "\n") {
		if hm := reObjdumpAddrFunc.FindStringSubmatch(line); hm != nil {
			flushFunc()
			curFunc = hm[2]
			continue
		}
		if curFunc == "" || !strings.Contains(curFunc, modPrefix) {
			continue
		}
		codePart := line
		if before, _, hasComment := strings.Cut(line, "#"); hasComment {
			codePart = before
		}
		if im := reObjdumpInsn.FindStringSubmatch(codePart); im != nil {
			insns = append(insns, objdumpInsn{
				addr: normalizeHexAddr(im[1]),
				op:   strings.ToLower(im[2]),
				args: strings.TrimSpace(im[3]),
			})
		}
	}
	flushFunc()
	return report
}

func collectVectorAudit(ctx context.Context, binPath string) vectorAuditReport {
	if _, err := exec.LookPath("objdump"); err != nil {
		return vectorAuditReport{Available: false}
	}
	out, err := exec.CommandContext(ctx, "objdump", "-d", "--no-show-raw-insn", binPath).Output()
	if err != nil {
		return vectorAuditReport{Available: false}
	}
	return auditObjdumpVectorCFG(string(out))
}

func printRuntimeAndVectorSections(s suiteReport, thin string) {
	if len(s.RuntimeStdlibHot) > 0 {
		fmt.Println(thin)
		fmt.Println("RUNTIME / STDLIB HOTSPOTS (non-goblin CPU consumers):")
		for _, rh := range s.RuntimeStdlibHot {
			tag := ""
			if rh.Inlined {
				tag = " [inline]"
			}
			fmt.Printf("  %-48s flat=%5.2f%% (%.1fms) cum=%5.2f%% (%.1fms)%s\n",
				rh.Symbol, rh.FlatPct, rh.FlatMS, rh.CumPct, rh.CumMS, tag)
			if len(rh.TopCallers) > 0 {
				fmt.Printf("     top callers: %s\n", strings.Join(rh.TopCallers, "; "))
			}
		}
	}
	if s.VectorAudit != nil {
		fmt.Println(thin)
		fmt.Println("VECTOR ISA AUDIT (GNU objdump CFG):")
		va := s.VectorAudit
		if !va.Available {
			fmt.Println("  status: unavailable (GNU objdump not found or failed)")
			return
		}
		fmt.Printf("  EVEX funcs=%d  VEX-YMM funcs=%d  Unguarded EVEX=%d  CFG hazards=%d\n",
			va.EVEXFuncCount, va.VEXYMMFuncCount, len(va.UnguardedEVEX), len(va.Hazards))
		if len(va.UnguardedEVEX) > 0 {
			fmt.Printf("  unguarded EVEX funcs: %s\n", strings.Join(va.UnguardedEVEX, ", "))
		}
		for _, hz := range va.Hazards {
			fmt.Printf("  HAZARD [%s] %s @ %s: %s\n", hz.Kind, hz.Func, hz.Addr, hz.Insn)
		}
	}
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
	if r.Disasm.DeferReturnCalls > 0 {
		findings = append(findings, fmt.Sprintf(
			"%d runtime.deferreturn call(s) on hot path (consider scoping or removing defer)",
			r.Disasm.DeferReturnCalls,
		))
	}
	if r.Disasm.FrameBytes >= largeFrameBytes {
		findings = append(findings, fmt.Sprintf(
			"large stack frame (%d bytes); inspect stack spills (%d (SP) refs)",
			r.Disasm.FrameBytes, r.Disasm.StackSpillRefs,
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

		printRuntimeAndVectorSections(sr, thin)
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
