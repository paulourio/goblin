package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	unrollFactor4           = 4
	maxFramePushFTWrapper   = 64
	maxFrameComputeTabRow   = 640
	maxFrameRecomputeRes    = 512
	maxFrameEvalColDualDot  = 400
	maxFrameEvalRowActivity = 256
)

type inlineExpectation struct {
	file string
	name string
}

type bceLoopTarget struct {
	file     string
	funcName string
}

type disasmExpectation struct {
	shortSym      string
	maxFrameBytes int
	maxDefers     int
	maxGrowSlices int
}

type lineSpan struct {
	start int
	end   int
}

type invariantCheckResult struct {
	CheckedInline   int
	CheckedNoinline int
	CheckedBCELoops int
	CheckedDisasm   int
	VectorAudits    int
	Violations      []string
}

func requiredInlineHelpers(useSIMD bool) []inlineExpectation {
	reqs := []inlineExpectation{
		{file: "cuts.go", name: "nextUp"},
		{file: "cuts.go", name: "nextDown"},
		{file: "presolve.go", name: "isFractionalFloat64"},
		{file: "simplex.go", name: "combineDualNoiseTol"},
		{file: "lu.go", name: "(*LUFactorization).removeURowEntry"},
		{file: "lu.go", name: "(*LUFactorization).removeUColEntry"},
		{file: filepath.Join("format", "pulp_json_fast.go"), name: "(*pulpScanner).consumeByte"},
		{file: filepath.Join("format", "pulp_json_fast.go"), name: "(*pulpScanner).readUnescapedBytes"},
	}
	if useSIMD && runtime.GOARCH == "amd64" {
		reqs = append(reqs, inlineExpectation{
			file: filepath.Join("internal", "simd", "kernels_amd64.go"),
			name: "evalDualLeaveRow",
		})
	}
	return reqs
}

func requiredNoinlineHelpers() []inlineExpectation {
	return []inlineExpectation{
		{file: "simplex.go", name: "recomputeWorkingRowResidualWide"},
		{file: "simplex.go", name: "evalColDualDotWide"},
		{file: "presolve.go", name: "evalBinaryTightenRHSWide"},
		{file: "presolve.go", name: "evalRowActivityWide"},
		{file: "heuristics.go", name: "evalEpigraphCompObjWide"},
	}
}

func requiredBCEFreeLoops() []bceLoopTarget {
	assignFile := filepath.Join("internal", "simd", "assign_scalar.go")
	kernelsFile := filepath.Join("internal", "simd", "kernels_scalar.go")
	return []bceLoopTarget{
		{file: kernelsFile, funcName: "AllAbsLE64Scalar"},
		{file: kernelsFile, funcName: "Min2ReducedRow64Scalar"},
		{file: assignFile, funcName: "RelaxRow64Scalar"},
		{file: assignFile, funcName: "MinNoNaN64Scalar"},
		{file: assignFile, funcName: "ValidateAndNegate64Scalar"},
		{file: assignFile, funcName: "ColReduceValidateRows64Scalar"},
		{file: assignFile, funcName: "ColReduceRows64Scalar"},
		{file: assignFile, funcName: "ScanNearTightRow64Scalar"},
		{file: assignFile, funcName: "ScanNearTightMatrix64Scalar"},
		{file: assignFile, funcName: "AllAbsLECheckInt64Scalar"},
		{file: assignFile, funcName: "ValidateAndNegateCheckInt64Scalar"},
		{file: assignFile, funcName: "ColReduceValidateCheckIntRows64Scalar"},
	}
}

func requiredDisasmInvariants() []disasmExpectation {
	return []disasmExpectation{
		{
			shortSym:      "(*LUFactorization).pushFTFromSpike",
			maxFrameBytes: maxFramePushFTWrapper,
			maxDefers:     0,
			maxGrowSlices: 0,
		},
		{
			shortSym:      "(*LPSolver).ComputeTableauRow",
			maxFrameBytes: maxFrameComputeTabRow,
			maxDefers:     0,
			maxGrowSlices: 0,
		},
		{
			shortSym:      "(*LPSolver).recomputeWorkingRowResidual",
			maxFrameBytes: maxFrameRecomputeRes,
			maxDefers:     0,
			maxGrowSlices: -1,
		},
		{
			shortSym:      "evalColDualDot",
			maxFrameBytes: maxFrameEvalColDualDot,
			maxDefers:     0,
			maxGrowSlices: -1,
		},
		{
			shortSym:      "evalRowActivitySummaryInto",
			maxFrameBytes: maxFrameEvalRowActivity,
			maxDefers:     0,
			maxGrowSlices: -1,
		},
	}
}

func isStep4Post(post ast.Stmt) bool {
	as, ok := post.(*ast.AssignStmt)
	if !ok || as.Tok != token.ADD_ASSIGN || len(as.Rhs) != 1 {
		return false
	}
	lit, ok := as.Rhs[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	step, err := strconv.Atoi(lit.Value)
	return err == nil && step == unrollFactor4
}

func findUnrolledLoop4Spans(repoRoot, relFile, funcName string) ([]lineSpan, error) {
	fullPath := filepath.Join(repoRoot, relFile)
	fset := token.NewFileSet()
	fileAst, err := parser.ParseFile(fset, fullPath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relFile, err)
	}
	var spans []lineSpan
	foundFunc := false
	for _, decl := range fileAst.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name != funcName {
			continue
		}
		foundFunc = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			fs, ok := n.(*ast.ForStmt)
			if !ok || !isStep4Post(fs.Post) {
				return true
			}
			spans = append(spans, lineSpan{
				start: fset.Position(fs.Pos()).Line,
				end:   fset.Position(fs.End()).Line,
			})
			return false
		})
	}
	if !foundFunc {
		return nil, fmt.Errorf("function %s not found in %s", funcName, relFile)
	}
	return spans, nil
}

type parsedCompilerDiags struct {
	canInline    map[string]int
	cannotInline map[string]string
	bceByFile    map[string]map[int][]string
}

func parseCompilerDiagnostics(stderr string) parsedCompilerDiags {
	out := parsedCompilerDiags{
		canInline:    make(map[string]int),
		cannotInline: make(map[string]string),
		bceByFile:    make(map[string]map[int][]string),
	}
	for line := range strings.SplitSeq(stderr, "\n") {
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
			out.canInline[file+":"+cm[1]] = cost
			continue
		}
		if cm := reCannotInline.FindStringSubmatch(msg); cm != nil {
			out.cannotInline[file+":"+cm[1]] = cm[2]
			continue
		}
		if strings.Contains(msg, "Found IsInBounds") || strings.Contains(msg, "Found IsSliceInBounds") {
			byLine := out.bceByFile[file]
			if byLine == nil {
				byLine = make(map[int][]string)
				out.bceByFile[file] = byLine
			}
			byLine[lineNum] = append(byLine[lineNum], msg)
		}
	}
	return out
}

func verifyStaticInvariants(repoRoot string, useSIMD bool, diags parsedCompilerDiags, res *invariantCheckResult) error {
	for _, req := range requiredInlineHelpers(useSIMD) {
		res.CheckedInline++
		key := filepath.Clean(req.file) + ":" + req.name
		if _, ok := diags.canInline[key]; !ok {
			reason := diags.cannotInline[key]
			if reason == "" {
				reason = "missing can-inline diagnostic"
			}
			res.Violations = append(res.Violations,
				fmt.Sprintf("inline lock failed for %s (%s): %s", req.name, req.file, reason))
		}
	}

	for _, req := range requiredNoinlineHelpers() {
		res.CheckedNoinline++
		key := filepath.Clean(req.file) + ":" + req.name
		reason := diags.cannotInline[key]
		if !strings.Contains(reason, "go:noinline") {
			if cost, inlined := diags.canInline[key]; inlined {
				res.Violations = append(res.Violations,
					fmt.Sprintf("noinline lock failed for %s (%s): unexpectedly inlineable with cost %d", req.name, req.file, cost))
			} else {
				res.Violations = append(res.Violations,
					fmt.Sprintf("noinline lock failed for %s (%s): got %q, want marked go:noinline", req.name, req.file, reason))
			}
		}
	}

	for _, target := range requiredBCEFreeLoops() {
		spans, err := findUnrolledLoop4Spans(repoRoot, target.file, target.funcName)
		if err != nil {
			return err
		}
		if len(spans) == 0 {
			res.Violations = append(res.Violations,
				fmt.Sprintf("4-way unrolled loop missing in %s (%s)", target.funcName, target.file))
			continue
		}
		res.CheckedBCELoops += len(spans)
		fileBCE := diags.bceByFile[filepath.Clean(target.file)]
		for _, sp := range spans {
			for ln, msgs := range fileBCE {
				if ln >= sp.start && ln <= sp.end {
					res.Violations = append(res.Violations,
						fmt.Sprintf("BCE regression inside 4-way loop of %s (%s:%d, loop L%d-L%d): %s",
							target.funcName, target.file, ln, sp.start, sp.end, strings.Join(msgs, "; ")))
				}
			}
		}
	}
	return nil
}

func checkCodegenInvariants(ctx context.Context, repoRoot string, useSIMD bool) (invariantCheckResult, error) {
	var res invariantCheckResult

	buildCmd := exec.CommandContext(
		ctx,
		"go", "build",
		"-gcflags=./...=-m=2 -d=ssa/check_bce/debug=1",
		".", "./format", "./internal/simd",
	)
	buildCmd.Dir = repoRoot
	buildCmd.Env = buildEnv(useSIMD)
	var stderr bytes.Buffer
	buildCmd.Stderr = &stderr
	if err := buildCmd.Run(); err != nil {
		return res, fmt.Errorf("go build invariant diagnostics failed: %w\n%s", err, stderr.String())
	}

	diags := parseCompilerDiagnostics(stderr.String())
	if err := verifyStaticInvariants(repoRoot, useSIMD, diags, &res); err != nil {
		return res, err
	}

	if runtime.GOARCH != "amd64" {
		return res, nil
	}

	tmpDir, err := os.MkdirTemp("", "goblin-invariants-*")
	if err != nil {
		return res, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	rootBin := filepath.Join(tmpDir, "root.test")
	rootCompile := exec.CommandContext(ctx, "go", "test", "-c", "-o", rootBin, ".")
	rootCompile.Dir = repoRoot
	rootCompile.Env = buildEnv(useSIMD)
	if out, err := rootCompile.CombinedOutput(); err != nil {
		return res, fmt.Errorf("compile root test binary: %w\n%s", err, string(out))
	}

	for _, exp := range requiredDisasmInvariants() {
		res.CheckedDisasm++
		dm := collectDisasmMetrics(ctx, rootBin, exp.shortSym)
		if dm.InstCount == 0 {
			res.Violations = append(res.Violations, "disasm symbol missing: "+exp.shortSym)
			continue
		}
		if exp.maxFrameBytes >= 0 && dm.FrameBytes > exp.maxFrameBytes {
			res.Violations = append(res.Violations,
				fmt.Sprintf("frame size lock failed for %s: %d bytes > limit %d", exp.shortSym, dm.FrameBytes, exp.maxFrameBytes))
		}
		if exp.maxDefers >= 0 && dm.DeferReturnCalls > exp.maxDefers {
			res.Violations = append(res.Violations,
				fmt.Sprintf("defer lock failed for %s: %d runtime.deferreturn calls > limit %d", exp.shortSym, dm.DeferReturnCalls, exp.maxDefers))
		}
		if exp.maxGrowSlices >= 0 && dm.GrowSliceCalls > exp.maxGrowSlices {
			res.Violations = append(res.Violations,
				fmt.Sprintf("growslice lock failed for %s: %d runtime.growslice calls > limit %d", exp.shortSym, dm.GrowSliceCalls, exp.maxGrowSlices))
		}
	}

	if useSIMD {
		simdBin := filepath.Join(tmpDir, "simd.test")
		simdCompile := exec.CommandContext(ctx, "go", "test", "-c", "-o", simdBin, "./internal/simd")
		simdCompile.Dir = repoRoot
		simdCompile.Env = buildEnv(true)
		if out, err := simdCompile.CombinedOutput(); err != nil {
			return res, fmt.Errorf("compile simd test binary: %w\n%s", err, string(out))
		}
		for _, item := range []struct {
			label string
			path  string
		}{
			{label: "root.test", path: rootBin},
			{label: "simd.test", path: simdBin},
		} {
			va := collectVectorAudit(ctx, item.path)
			if !va.Available {
				continue
			}
			res.VectorAudits++
			if len(va.UnguardedEVEX) > 0 {
				res.Violations = append(res.Violations,
					fmt.Sprintf("unguarded EVEX in %s: %s", item.label, strings.Join(va.UnguardedEVEX, ", ")))
			}
			for _, hz := range va.Hazards {
				res.Violations = append(res.Violations,
					fmt.Sprintf("vector CFG hazard in %s [%s] %s @ %s: %s", item.label, hz.Kind, hz.Func, hz.Addr, hz.Insn))
			}
		}
	}

	return res, nil
}
