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

func TestCheckUnguardedEVEXPolarityAndOpcodes(t *testing.T) {
	// Verify Track5C EVEX-only mnemonics on YMM/XMM operands without ZMM or mask tokens.
	evexOnlyCases := []struct {
		op   string
		args string
	}{
		{op: "vpmullq", args: "%ymm1,%ymm0,%ymm0"},
		{op: "vpopcntq", args: "%ymm0,%ymm0"},
		{op: "vplzcntd", args: "%ymm0,%ymm0"},
		{op: "vcvtqq2pd", args: "%ymm0,%ymm0"},
		{op: "vcvtuqq2pd", args: "%ymm0,%ymm0"},
		{op: "vcvtpd2qq", args: "%ymm0,%ymm0"},
		{op: "vpermt2pd", args: "%ymm2,%ymm1,%ymm0"},
		{op: "vpcompressq", args: "%ymm0,(%rdi)"},
		{op: "vrangepd", args: "$0x0,%ymm1,%ymm0,%ymm0"},
	}
	for _, tc := range evexOnlyCases {
		if !isEVEXInsn(tc.op, tc.args) {
			t.Errorf("isEVEXInsn(%q, %q) = false, want true", tc.op, tc.args)
		}
	}
	if !checkUnguardedEVEX([]objdumpInsn{
		{addr: "1000", op: "cmpb", args: "$0x0,0x10(%rip)", comment: "2000 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>"},
		{addr: "1007", op: "je", args: "100a <f+0xa>"},
		{addr: "1009", op: "ret"},
		{addr: "100a", op: "vpmullq", args: "%ymm1,%ymm0,%ymm0"},
		{addr: "1010", op: "ret"},
	}, map[string]int{"1000": 0, "1007": 1, "1009": 2, "100a": 3, "1010": 4}) {
		t.Fatal("checkUnguardedEVEX = false on inverted guard into vpmullq, want true")
	}

	raw := `0000000000402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>:
  402000:	vmovupd (%rsi),%zmm0
  402006:	vzeroupper
  402009:	ret

0000000000402100 <github.com/paulourio/goblin/internal/simd.invertedAVX512Dispatch>:
  402100:	cmpb $0x0,0x200(%rip)        # 602307 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402107:	je 40210a <github.com/paulourio/goblin/internal/simd.invertedAVX512Dispatch+0xa>
  402109:	ret
  40210a:	vmovdqu64 (%rax),%zmm0
  402110:	vzeroupper
  402113:	ret

0000000000402200 <github.com/paulourio/goblin/internal/simd.evexOnNonAVX512Fallthrough>:
  402200:	cmpb $0x0,0x100(%rip)        # 602307 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402207:	jne 402213 <github.com/paulourio/goblin/internal/simd.evexOnNonAVX512Fallthrough+0x13>
  402209:	vmovdqu64 (%rax),%zmm0
  40220f:	vzeroupper
  402212:	ret
  402213:	ret

0000000000402300 <github.com/paulourio/goblin/internal/simd.BadUnknownPolarity>:
  402300:	cmpb $0x0,0x0(%rip)          # 602307 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402307:	jl 40230a <github.com/paulourio/goblin/internal/simd.BadUnknownPolarity+0xa>
  402309:	ret
  40230a:	vmovdqu64 (%rax),%zmm0
  402310:	vzeroupper
  402313:	ret

0000000000402400 <github.com/paulourio/goblin/internal/simd.unguardedAVX512DQ>:
  402400:	vpmullq %ymm1,%ymm0,%ymm0
  402406:	vzeroupper
  402409:	ret

0000000000402500 <github.com/paulourio/goblin/internal/simd.BadUnguardedHelperCall>:
  402500:	call 402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>
  402505:	ret

0000000000402600 <github.com/paulourio/goblin/internal/simd.BadActiveTierNonZeroOnly>:
  402600:	mov 0x100(%rip),%edx         # 602708 <github.com/paulourio/goblin/internal/simd.activeTier>
  402606:	test %edx,%edx
  402608:	je 402610 <github.com/paulourio/goblin/internal/simd.BadActiveTierNonZeroOnly+0x10>
  40260a:	call 402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>
  40260f:	ret
  402610:	ret

0000000000402680 <github.com/paulourio/goblin/internal/simd.BadSpoofedNonRipComment>:
  402680:	cmp $0x0,%eax                # <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402683:	je 40268f <github.com/paulourio/goblin/internal/simd.BadSpoofedNonRipComment+0xf>
  402685:	vmovdqu64 (%rax),%zmm0
  40268b:	vzeroupper
  40268e:	ret
  40268f:	ret

0000000000402700 <github.com/paulourio/goblin/internal/simd.GoodFallthroughGuard>:
  402700:	cmpb $0x0,0x100(%rip)        # 602307 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402707:	je 402713 <github.com/paulourio/goblin/internal/simd.GoodFallthroughGuard+0x13>
  402709:	vmovdqu64 (%rax),%zmm0
  40270f:	vzeroupper
  402712:	ret
  402713:	ret

0000000000402800 <github.com/paulourio/goblin/internal/simd.GoodTargetGuard>:
  402800:	movzbl 0x100(%rip),%edx      # 602307 <github.com/paulourio/goblin/internal/simd.x86HasAVX512>
  402807:	test %dl,%dl
  402809:	jne 40280c <github.com/paulourio/goblin/internal/simd.GoodTargetGuard+0xc>
  40280b:	ret
  40280c:	call 402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>
  402811:	ret

0000000000402900 <github.com/paulourio/goblin/internal/simd.GoodSpilledActiveTier>:
  402900:	sub $0x28,%rsp
  402904:	mov 0x100(%rip),%r12d        # 602708 <github.com/paulourio/goblin/internal/simd.activeTier>
  40290b:	test %r12d,%r12d
  40290e:	je 402927 <github.com/paulourio/goblin/internal/simd.GoodSpilledActiveTier+0x27>
  402910:	mov %r12d,0x10(%rsp)
  402915:	mov 0x10(%rsp),%r12d
  40291a:	cmp $0x2,%r12d
  40291e:	jne 402927 <github.com/paulourio/goblin/internal/simd.GoodSpilledActiveTier+0x27>
  402920:	call 402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>
  402925:	jmp 402927 <github.com/paulourio/goblin/internal/simd.GoodSpilledActiveTier+0x27>
  402927:	add $0x28,%rsp
  40292b:	ret

0000000000402a00 <github.com/paulourio/goblin/internal/simd.GoodCallerStackBool>:
  402a00:	sub $0x40,%rsp
  402a04:	mov 0x100(%rip),%edx         # 602708 <github.com/paulourio/goblin/internal/simd.activeTier>
  402a0a:	cmp $0x2,%edx
  402a0d:	jne 402a1b <github.com/paulourio/goblin/internal/simd.GoodCallerStackBool+0x1b>
  402a0f:	movb $0x1,0x30(%rsp)
  402a14:	call 402b00 <github.com/paulourio/goblin/internal/simd.goodCalleeStackBool>
  402a19:	jmp 402a25 <github.com/paulourio/goblin/internal/simd.GoodCallerStackBool+0x25>
  402a1b:	movb $0x0,0x30(%rsp)
  402a20:	call 402b00 <github.com/paulourio/goblin/internal/simd.goodCalleeStackBool>
  402a25:	add $0x40,%rsp
  402a29:	ret

0000000000402b00 <github.com/paulourio/goblin/internal/simd.goodCalleeStackBool>:
  402b00:	sub $0x68,%rsp
  402b04:	movzbl 0xa0(%rsp),%edx
  402b0c:	test %dl,%dl
  402b0e:	je 402b17 <github.com/paulourio/goblin/internal/simd.goodCalleeStackBool+0x17>
  402b10:	call 402000 <github.com/paulourio/goblin/internal/simd.dot64AVX512>
  402b15:	jmp 402b17 <github.com/paulourio/goblin/internal/simd.goodCalleeStackBool+0x17>
  402b17:	add $0x68,%rsp
  402b1b:	ret
`

	rep := auditObjdumpVectorCFG(raw)
	wantBad := []string{
		"github.com/paulourio/goblin/internal/simd.invertedAVX512Dispatch",
		"github.com/paulourio/goblin/internal/simd.evexOnNonAVX512Fallthrough",
		"github.com/paulourio/goblin/internal/simd.BadUnknownPolarity",
		"github.com/paulourio/goblin/internal/simd.unguardedAVX512DQ",
		"github.com/paulourio/goblin/internal/simd.BadUnguardedHelperCall",
		"github.com/paulourio/goblin/internal/simd.BadActiveTierNonZeroOnly",
		"github.com/paulourio/goblin/internal/simd.BadSpoofedNonRipComment",
	}
	if !slices.Equal(rep.UnguardedEVEX, wantBad) {
		t.Fatalf("UnguardedEVEX = %v, want %v", rep.UnguardedEVEX, wantBad)
	}
}
