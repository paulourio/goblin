package main

import (
	"os"
	"path/filepath"
	"slices"
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

func TestResolveRunDir(t *testing.T) {
	cases := []struct {
		pkg  string
		want string
	}{
		{pkg: "", want: "."},
		{pkg: ".", want: "."},
		{pkg: "./...", want: "."},
		{pkg: "github.com/paulourio/goblin", want: "."},
		{pkg: "github.com/paulourio/goblin/...", want: "."},
		{pkg: "./format", want: "format"},
		{pkg: "./format/...", want: "format"},
		{pkg: "github.com/paulourio/goblin/format", want: "format"},
		{pkg: "./assign", want: "assign"},
		{pkg: "./internal/simd", want: "internal/simd"},
	}
	for _, tc := range cases {
		if got := resolveRunDir(tc.pkg); got != tc.want {
			t.Errorf("resolveRunDir(%q) = %q, want %q", tc.pkg, got, tc.want)
		}
	}
}

func TestParseDisasmMetricsFrameAndDefer(t *testing.T) {
	raw := `TEXT github.com/paulourio/goblin.(*LUFactorization).pushFTFromSpike(SB) /repo/lu.go
  lu.go:100 0x68b0b6 4881ec60030000 SUBQ $0x360, SP
  lu.go:101 0x68b100 f20f5900 MULSD (AX), X0
  lu.go:102 0x68b110 e800000000 CALL runtime.deferreturn(SB)
  lu.go:103 0x68b120 e800000000 CALL runtime.panicBounds(SB)
  lu.go:104 0x68b130 e800000000 CALL runtime.growslice(SB)
  lu.go:105 0x68b140 e800000000 CALL github.com/paulourio/goblin.helper(SB)
`
	got := parseDisasmMetrics(raw, "(*LUFactorization).pushFTFromSpike")
	if got.FrameBytes != 0x360 {
		t.Fatalf("FrameBytes = %d, want %d", got.FrameBytes, 0x360)
	}
	if got.DeferReturnCalls != 1 {
		t.Fatalf("DeferReturnCalls = %d, want 1", got.DeferReturnCalls)
	}
	if got.PanicBoundsCalls != 1 || got.GrowSliceCalls != 1 {
		t.Fatalf("PanicBoundsCalls=%d GrowSliceCalls=%d, want 1/1", got.PanicBoundsCalls, got.GrowSliceCalls)
	}
	if got.ScalarFPOps != 1 || len(got.NonInlinedCalls) != 1 {
		t.Fatalf("ScalarFPOps=%d NonInlinedCalls=%v, want 1 and [github.com/paulourio/goblin.helper]", got.ScalarFPOps, got.NonInlinedCalls)
	}
}

func TestParseRuntimeStdlibHotAndPeekCallers(t *testing.T) {
	pprofTop := `File: before_root_scalar.test
Duration: 2.89s, Total samples = 2950ms (102.02%)
      flat  flat%   sum%        cum   cum%
     170ms  5.76%  5.76%      180ms  6.10%  github.com/paulourio/goblin.(*LUFactorization).solveUDense
      60ms  2.03% 50.51%       60ms  2.03%  math.Float64bits (inline)
      60ms  2.03% 52.54%       80ms  2.71%  math.ldexp
      30ms  1.02% 53.56%       30ms  1.02%  runtime.memclrNoHeapPointers
       5ms  0.17% 53.73%        5ms  0.17%  runtime.futex
`
	hot := parseRuntimeStdlibHot(pprofTop)
	if len(hot) != 3 {
		t.Fatalf("len(hot) = %d, want 3: %+v", len(hot), hot)
	}
	if hot[0].Symbol != "math.ldexp" || hot[0].Inlined {
		t.Fatalf("hot[0] = %+v, want math.ldexp", hot[0])
	}
	if hot[1].Symbol != "math.Float64bits (inline)" || !hot[1].Inlined {
		t.Fatalf("hot[1] = %+v, want math.Float64bits (inline)", hot[1])
	}
	if hot[2].Symbol != "runtime.memclrNoHeapPointers" {
		t.Fatalf("hot[2] = %+v, want runtime.memclrNoHeapPointers", hot[2])
	}

	peekOut := `----------------------------------------------------------+-------------
      flat  flat%   sum%        cum   cum%   calls calls% + context
----------------------------------------------------------+-------------
                                             0.08s   100% |   math.Ldexp
     0.06s  2.03%  2.03%      0.08s  2.71%                | math.ldexp
                                             0.01s 12.50% |   math.Float64bits (inline)
----------------------------------------------------------+-------------
                                             0.01s 33.33% |   github.com/paulourio/goblin.(*LUFactorization).FactorizeSparse
                                             0.01s 33.33% |   github.com/paulourio/goblin.resizeFloat64
                                             0.01s 33.33% |   github.com/paulourio/goblin.resizeInt
     0.03s  1.02%  3.05%      0.03s  1.02%                | runtime.memclrNoHeapPointers
----------------------------------------------------------+-------------
`
	callers := parsePprofPeekCallers(peekOut)
	if got := callers["math.ldexp"]; len(got) != 1 || got[0] != "math.Ldexp (100%)" {
		t.Fatalf("callers[math.ldexp] = %v, want [math.Ldexp (100%%)]", got)
	}
	if got := callers["runtime.memclrNoHeapPointers"]; len(got) != 3 || got[0] != "(*LUFactorization).FactorizeSparse (33.33%)" {
		t.Fatalf("callers[memclrNoHeapPointers] = %v", got)
	}
}

func TestAuditObjdumpVectorCFG(t *testing.T) {
	raw := `0000000000401000 <github.com/paulourio/goblin/internal/simd.cleanAVX2>:
  401000:	vmovdqu (%rsi),%ymm0
  401004:	vaddpd %ymm1,%ymm0,%ymm0
  401008:	je 401012 <github.com/paulourio/goblin/internal/simd.cleanAVX2+0x12>
  40100a:	vzeroupper
  40100d:	ret
  401012:	call 400000 <runtime.panicBounds>

0000000000401100 <github.com/paulourio/goblin/internal/simd.cleanAVX512>:
  401100:	vbroadcastsd %xmm0,%zmm0
  401106:	vcmpgepd %zmm1,%zmm0,%k1
  401108:	je 401116 <github.com/paulourio/goblin/internal/simd.cleanAVX512+0x16>
  40110a:	vzeroupper
  40110d:	kmovb %k1,%eax
  401111:	ret
  401116:	vzeroupper
  401119:	ret

0000000000401200 <github.com/paulourio/goblin/internal/simd.dirtyRetAndSSE>:
  401200:	vmovdqu (%rsi),%ymm0
  401204:	je 401210 <github.com/paulourio/goblin/internal/simd.dirtyRetAndSSE+0x10>
  401206:	vzeroupper
  401209:	ret
  401210:	movsd (%rdi),%xmm1
  401214:	ret

0000000000401300 <github.com/paulourio/goblin/internal/simd.HoistedEVEX>:
  401300:	vmovdqu64 (%rax),%zmm0
  401306:	vzeroupper
  401309:	ret
`
	rep := auditObjdumpVectorCFG(raw)
	if rep.EVEXFuncCount != 2 || rep.VEXYMMFuncCount != 2 {
		t.Fatalf("EVEX=%d VEX-YMM=%d, want 2 and 2", rep.EVEXFuncCount, rep.VEXYMMFuncCount)
	}
	if len(rep.UnguardedEVEX) != 1 || rep.UnguardedEVEX[0] != "github.com/paulourio/goblin/internal/simd.HoistedEVEX" {
		t.Fatalf("UnguardedEVEX = %v, want [HoistedEVEX]", rep.UnguardedEVEX)
	}
	var kinds []string
	for _, hz := range rep.Hazards {
		kinds = append(kinds, hz.Kind)
	}
	if !slices.Contains(kinds, "LEGACY_SSE_WHILE_DIRTY") || !slices.Contains(kinds, "DIRTY_RET") {
		t.Fatalf("Hazards = %+v, want LEGACY_SSE_WHILE_DIRTY and DIRTY_RET", rep.Hazards)
	}
}
