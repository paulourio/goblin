package goblin

import (
	"debug/elf"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	track5ModDotPrefix   = "github.com/paulourio/gob" + "lin."
	track5ModSlashPrefix = "github.com/paulourio/gob" + "lin/"
)

var (
	reTrack5ObjdumpFunc = regexp.MustCompile(`^([0-9a-f]+)\s+<([^>]+)>:$`)
	reTrack5ObjdumpInsn = regexp.MustCompile(`^\s*([0-9a-f]+):\s+([A-Za-z0-9.]+)(?:\s+(.*))?$`)
)

type binTargetEntry struct {
	Idx    int    `json:"idx"`
	Symbol string `json:"symbol"`
}

type binCheckRequest struct {
	Mode    string           `json:"mode,omitzero"`
	Symbol  string           `json:"symbol,omitzero"`
	Words   []uint32         `json:"words"`
	Targets []binTargetEntry `json:"targets,omitzero"`
}

type arm64ElfRoutine struct {
	words   []uint32
	targets []binTargetEntry
}

type elfFuncSym struct {
	addr uint64
	size uint64
	name string
}

type simdCfgInsnPayload struct {
	Addr string `json:"addr"`
	Op   string `json:"op"`
	Args string `json:"args"`
	Raw  string `json:"raw,omitzero"`
}

type simdCfgFuncPayload struct {
	Name  string               `json:"name"`
	Insns []simdCfgInsnPayload `json:"insns"`
}

type simdCfgRequest struct {
	Functions []simdCfgFuncPayload `json:"functions"`
}

func stripModulePrefixes(s string) string {
	s = strings.ReplaceAll(s, track5ModSlashPrefix, "")
	s = strings.ReplaceAll(s, track5ModDotPrefix, "")
	return s
}

func hasModulePrefix(s string) bool {
	return strings.HasPrefix(s, track5ModSlashPrefix) || strings.HasPrefix(s, track5ModDotPrefix)
}

func runToolWithExitCode(tb testing.TB, exe string, args ...string) (int, string) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), exe, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), string(out)
	}
	tb.Fatalf("failed executing %s %v: %v\n%s", exe, args, err, out)
	return -1, ""
}

func buildTestBinary(tb testing.TB, outPath, pkg string, extraEnv, buildFlags []string) {
	tb.Helper()
	args := make([]string, 0, len(buildFlags)+4)
	args = append(args, "test", "-c")
	args = append(args, buildFlags...)
	args = append(args, "-o", outPath, pkg)
	cmd := exec.CommandContext(tb.Context(), "go", args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("go %v failed: %v\n%s", args, err, out)
	}
}

func resolveElfSymAt(syms []elfFuncSym, addr uint64) (string, bool) {
	idx, found := slices.BinarySearchFunc(syms, addr, func(s elfFuncSym, target uint64) int {
		if s.addr < target {
			return -1
		}
		if s.addr > target {
			return 1
		}
		return 0
	})
	if found {
		return syms[idx].name, true
	}
	if idx > 0 {
		prev := syms[idx-1]
		if addr >= prev.addr && addr < prev.addr+prev.size {
			return prev.name, true
		}
	}
	return "", false
}

func extractArm64Routines(tb testing.TB, binPath string, want []string) map[string]arm64ElfRoutine {
	tb.Helper()
	f, err := elf.Open(binPath)
	if err != nil {
		tb.Fatalf("elf.Open(%s): %v", binPath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			tb.Errorf("elf close(%s): %v", binPath, closeErr)
		}
	}()

	if f.Machine != elf.EM_AARCH64 {
		tb.Fatalf("expected EM_AARCH64 ELF, got %v", f.Machine)
	}
	textSec := f.Section(".text")
	if textSec == nil {
		tb.Fatal("missing .text section in arm64 ELF")
	}
	textBytes, err := textSec.Data()
	if err != nil {
		tb.Fatalf("read .text: %v", err)
	}
	rawSyms, err := f.Symbols()
	if err != nil {
		tb.Fatalf("read .symtab: %v", err)
	}

	funcSyms := make([]elfFuncSym, 0, len(rawSyms))
	byShort := make(map[string]elfFuncSym, len(want))
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}

	for _, s := range rawSyms {
		if s.Section == elf.SHN_UNDEF || elf.ST_TYPE(s.Info) != elf.STT_FUNC {
			continue
		}
		short := stripModulePrefixes(s.Name)
		funcSyms = append(funcSyms, elfFuncSym{
			addr: s.Value,
			size: s.Size,
			name: short,
		})
		if hasModulePrefix(s.Name) {
			if _, ok := wantSet[short]; ok {
				byShort[short] = elfFuncSym{addr: s.Value, size: s.Size, name: short}
			}
		}
	}

	slices.SortFunc(funcSyms, func(a, b elfFuncSym) int {
		if a.addr != b.addr {
			if a.addr < b.addr {
				return -1
			}
			return 1
		}
		aAbi0 := strings.HasSuffix(a.name, ".abi0")
		bAbi0 := strings.HasSuffix(b.name, ".abi0")
		if aAbi0 != bAbi0 {
			if aAbi0 {
				return -1
			}
			return 1
		}
		return strings.Compare(a.name, b.name)
	})

	dedup := make([]elfFuncSym, 0, len(funcSyms))
	for _, s := range funcSyms {
		if len(dedup) > 0 && dedup[len(dedup)-1].addr == s.addr {
			continue
		}
		dedup = append(dedup, s)
	}

	out := make(map[string]arm64ElfRoutine, len(want))
	for _, name := range want {
		sym, ok := byShort[name]
		if !ok {
			tb.Fatalf("symbol %q not found in %s", name, binPath)
		}
		if sym.addr < textSec.Addr {
			tb.Fatalf("symbol %q VA 0x%x precedes .text 0x%x", name, sym.addr, textSec.Addr)
		}
		off := sym.addr - textSec.Addr
		end := off + sym.size
		if end > uint64(len(textBytes)) || sym.size%4 != 0 {
			tb.Fatalf("symbol %q out of .text bounds (off=%d size=%d textLen=%d)", name, off, sym.size, len(textBytes))
		}
		nWords := int(sym.size / 4)
		words := make([]uint32, nWords)
		for i := range nWords {
			words[i] = binary.LittleEndian.Uint32(textBytes[off+uint64(i)*4:])
		}
		for len(words) > 0 && words[len(words)-1] == 0 {
			words = words[:len(words)-1]
		}
		if len(words) == 0 {
			tb.Fatalf("symbol %q decoded empty after trimming alignment padding", name)
		}

		var targets []binTargetEntry
		for i, w := range words {
			top := w & 0xFC000000
			if top != 0x14000000 && top != 0x94000000 {
				continue
			}
			imm26 := int64(int32(w<<6) >> 6)
			tgtIdx := int64(i) + imm26
			if tgtIdx >= 0 && tgtIdx < int64(len(words)) {
				continue
			}
			tgtAddr := uint64(int64(sym.addr) + tgtIdx*4)
			tgtSym, found := resolveElfSymAt(dedup, tgtAddr)
			if !found {
				tb.Fatalf("%s[%d]: unresolved external branch target VA 0x%x", name, i, tgtAddr)
			}
			targets = append(targets, binTargetEntry{Idx: i, Symbol: tgtSym})
		}

		out[name] = arm64ElfRoutine{words: words, targets: targets}
	}
	return out
}

func runBinCheckJSON(tb testing.TB, exe, fileName string, req binCheckRequest) (int, string) {
	tb.Helper()
	payload, err := json.Marshal(req)
	if err != nil {
		tb.Fatalf("marshal bin-check request: %v", err)
	}
	reqPath := filepath.Join(tb.TempDir(), fileName)
	if err := os.WriteFile(reqPath, payload, 0o600); err != nil {
		tb.Fatalf("write %s: %v", reqPath, err)
	}
	return runToolWithExitCode(tb, exe, reqPath)
}

func TestFormalTrack5_BinCheckELF(t *testing.T) {
	requireFormalEnv(t)
	t.Parallel()

	binCheckExe := formalExe(t, "bin-check")
	tmp := t.TempDir()

	noinlineELF := filepath.Join(tmp, "arm64_noinline.elf")
	optELF := filepath.Join(tmp, "arm64_opt.elf")
	arm64Env := []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64"}
	buildTestBinary(t, noinlineELF, ".", arm64Env, []string{"-gcflags=-l"})
	buildTestBinary(t, optELF, ".", arm64Env, nil)

	leafNames := []string{
		"presolveSplitmix64",
		"canonicalFloatBits",
		"divCeilInt64",
		"gcdInt64",
	}
	leafRoutines := extractArm64Routines(t, noinlineELF, leafNames)

	solverNames := []string{
		"hasInfeasibleIntegerEqRow",
		"assign.augmentRowScalar",
		"internal/simd.InitReducedRow64",
		"assign.refillQueueIndirectScalar",
		"assign.relaxRowIndirectScalar",
	}
	solverRoutines := extractArm64Routines(t, optELF, solverNames)

	for _, name := range leafNames {
		r := leafRoutines[name]
		t.Run("Leaf_"+name, func(t *testing.T) {
			t.Parallel()
			code, out := runBinCheckJSON(t, binCheckExe, "leaf.json", binCheckRequest{
				Mode:    "match",
				Symbol:  name,
				Words:   r.words,
				Targets: r.targets,
			})
			if code != 0 || !strings.Contains(out, "VERIFIED") {
				t.Fatalf("bin-check failed for %s (exit=%d):\n%s", name, code, out)
			}
		})
	}

	for _, name := range solverNames {
		r := solverRoutines[name]
		sub := strings.ReplaceAll(name, "/", "_")
		t.Run("Solver_"+sub, func(t *testing.T) {
			t.Parallel()
			code, out := runBinCheckJSON(t, binCheckExe, "solver.json", binCheckRequest{
				Mode:    "match-and-cfg",
				Symbol:  name,
				Words:   r.words,
				Targets: r.targets,
			})
			if code != 0 || !strings.Contains(out, "VERIFIED") {
				t.Fatalf("bin-check failed for %s (exit=%d):\n%s", name, code, out)
			}
		})
	}

	t.Run("Neg_InjectedMallocgc", func(t *testing.T) {
		t.Parallel()
		base := solverRoutines["hasInfeasibleIntegerEqRow"]
		words := slices.Clone(base.words)
		targets := slices.Clone(base.targets)
		// Replace index 1 (NOP/stack body) with an external BL targeting runtime.mallocgc.
		words[1] = 0x94100000
		targets = append(targets, binTargetEntry{Idx: 1, Symbol: "runtime.mallocgc"})
		code, out := runBinCheckJSON(t, binCheckExe, "neg_malloc.json", binCheckRequest{
			Mode:    "cfg",
			Words:   words,
			Targets: targets,
		})
		if code == 0 || !strings.Contains(out, "ZeroGC violation") {
			t.Fatalf("expected REJECTED on injected mallocgc (exit=%d):\n%s", code, out)
		}
	})

	t.Run("Neg_UnguardedPanicBounds", func(t *testing.T) {
		t.Parallel()
		base := solverRoutines["hasInfeasibleIntegerEqRow"]
		words := slices.Clone(base.words)
		targets := slices.Clone(base.targets)
		// Replace an early prologue/body instruction with an unguarded external BL to runtime.panicBounds.
		words[1] = 0x94100000
		targets = append(targets, binTargetEntry{Idx: 1, Symbol: "runtime.panicBounds"})
		code, out := runBinCheckJSON(t, binCheckExe, "neg_bounds.json", binCheckRequest{
			Mode:    "cfg",
			Words:   words,
			Targets: targets,
		})
		if code == 0 || !strings.Contains(out, "REJECTED") {
			t.Fatalf("expected REJECTED on unguarded panicBounds jump (exit=%d):\n%s", code, out)
		}
	})

	t.Run("Neg_CorruptedImmediate", func(t *testing.T) {
		t.Parallel()
		base := leafRoutines["presolveSplitmix64"]
		words := slices.Clone(base.words)
		words[1] ^= 1
		code, out := runBinCheckJSON(t, binCheckExe, "neg_corrupt.json", binCheckRequest{
			Mode:    "match",
			Symbol:  "presolveSplitmix64",
			Words:   words,
			Targets: slices.Clone(base.targets),
		})
		if code == 0 || !strings.Contains(out, "REJECTED") {
			t.Fatalf("expected REJECTED on corrupted instruction word (exit=%d):\n%s", code, out)
		}
	})
}

func parseObjdumpModuleFuncs(tb testing.TB, disasm string) []simdCfgFuncPayload {
	tb.Helper()
	var funcs []simdCfgFuncPayload
	var cur *simdCfgFuncPayload
	flush := func() {
		if cur != nil && len(cur.Insns) > 0 {
			funcs = append(funcs, *cur)
		}
		cur = nil
	}

	for line := range strings.SplitSeq(disasm, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := reTrack5ObjdumpFunc.FindStringSubmatch(line); m != nil {
			flush()
			rawName := m[2]
			if hasModulePrefix(rawName) {
				cur = &simdCfgFuncPayload{Name: stripModulePrefixes(rawName)}
			}
			continue
		}
		if cur == nil {
			continue
		}
		codePart := line
		if before, _, hasComment := strings.Cut(line, "#"); hasComment {
			codePart = before
		}
		if m := reTrack5ObjdumpInsn.FindStringSubmatch(codePart); m != nil {
			args := ""
			if len(m) >= 4 {
				args = strings.TrimSpace(stripModulePrefixes(m[3]))
			}
			cur.Insns = append(cur.Insns, simdCfgInsnPayload{
				Addr: strings.ToLower(m[1]),
				Op:   strings.ToLower(m[2]),
				Args: args,
				Raw:  strings.TrimSpace(stripModulePrefixes(line)),
			})
		}
	}
	flush()
	return funcs
}

func runSimdCfgCheckJSON(tb testing.TB, exe, fileName string, req simdCfgRequest) (int, string) {
	tb.Helper()
	payload, err := json.Marshal(req)
	if err != nil {
		tb.Fatalf("marshal simd-cfg-check request: %v", err)
	}
	reqPath := filepath.Join(tb.TempDir(), fileName)
	if err := os.WriteFile(reqPath, payload, 0o600); err != nil {
		tb.Fatalf("write %s: %v", reqPath, err)
	}
	return runToolWithExitCode(tb, exe, reqPath)
}

func runSimdCfgCheckRaw(tb testing.TB, exe, fileName, raw string) (int, string) {
	tb.Helper()
	reqPath := filepath.Join(tb.TempDir(), fileName)
	if err := os.WriteFile(reqPath, []byte(raw), 0o600); err != nil {
		tb.Fatalf("write %s: %v", reqPath, err)
	}
	return runToolWithExitCode(tb, exe, reqPath)
}

func TestFormalTrack5_SimdCfgCheck(t *testing.T) {
	requireFormalEnv(t)
	t.Parallel()

	simdExe := formalExe(t, "simd-cfg-check")
	tmp := t.TempDir()

	pkgs := []struct {
		pkg string
		bin string
	}{
		{pkg: "./internal/simd", bin: filepath.Join(tmp, "simd.testbin")},
		{pkg: ".", bin: filepath.Join(tmp, "root.testbin")},
		{pkg: "./assign", bin: filepath.Join(tmp, "assign.testbin")},
	}

	seen := make(map[string]struct{})
	var allFuncs []simdCfgFuncPayload
	for _, p := range pkgs {
		buildTestBinary(t, p.bin, p.pkg, []string{"GOEXPERIMENT=simd"}, nil)
		out, err := exec.CommandContext(t.Context(), "objdump", "-d", "--no-show-raw-insn", p.bin).Output()
		if err != nil {
			t.Fatalf("objdump %s failed: %v", p.bin, err)
		}
		for _, fn := range parseObjdumpModuleFuncs(t, string(out)) {
			if _, ok := seen[fn.Name]; ok {
				continue
			}
			seen[fn.Name] = struct{}{}
			allFuncs = append(allFuncs, fn)
		}
	}

	t.Run("ShippingFunctionsVerified", func(t *testing.T) {
		t.Parallel()
		code, out := runSimdCfgCheckJSON(t, simdExe, "shipping.json", simdCfgRequest{
			Functions: allFuncs,
		})
		if code != 0 || !strings.Contains(out, "VERIFIED") {
			t.Fatalf("simd-cfg-check failed on shipping binaries (exit=%d):\n%s", code, out)
		}
		trimmed := strings.TrimSpace(out)
		nStr := strings.TrimSuffix(strings.TrimPrefix(trimmed, "VERIFIED ("), " vector functions)")
		n, err := strconv.Atoi(nStr)
		if err != nil || n == 0 {
			t.Fatalf("expected positive vector function count in %q (err=%v)", trimmed, err)
		}
		t.Logf("%s across %d total module functions", trimmed, len(allFuncs))
	})

	negCases := []struct {
		name     string
		wantText string
		fn       simdCfgFuncPayload
	}{
		{
			name:     "Neg_DirtyRet",
			wantText: "DIRTY_RET",
			fn: simdCfgFuncPayload{
				Name: "syntheticDirtyRet",
				Insns: []simdCfgInsnPayload{
					{Addr: "1000", Op: "vpaddq", Args: "%ymm0,%ymm1,%ymm2"},
					{Addr: "1004", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_LegacySSEWhileDirty",
			wantText: "LEGACY_SSE_WHILE_DIRTY",
			fn: simdCfgFuncPayload{
				Name: "syntheticLegacySSE",
				Insns: []simdCfgInsnPayload{
					{Addr: "1000", Op: "vpaddq", Args: "%ymm0,%ymm1,%ymm2"},
					{Addr: "1004", Op: "movsd", Args: "%xmm0,%xmm1"},
					{Addr: "1008", Op: "vzeroupper", Args: ""},
					{Addr: "100c", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_DirtyCall",
			wantText: "DIRTY_CALL",
			fn: simdCfgFuncPayload{
				Name: "syntheticDirtyCall",
				Insns: []simdCfgInsnPayload{
					{Addr: "1000", Op: "vpaddq", Args: "%ymm0,%ymm1,%ymm2"},
					{Addr: "1004", Op: "call", Args: "400100 <helper>", Raw: "1004:\tcall 400100 <helper>"},
					{Addr: "1009", Op: "vzeroupper", Args: ""},
					{Addr: "100c", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_UnguardedEVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "syntheticScalarNoGuard",
				Insns: []simdCfgInsnPayload{
					{Addr: "1000", Op: "vpaddq", Args: "%zmm0,%zmm1,%zmm2"},
					{Addr: "1006", Op: "vzeroupper", Args: ""},
					{Addr: "1009", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_LenGuardEVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.sumFast",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "cmp", Args: "$0x8,%rbx"},
					{Addr: "14", Op: "jl", Args: "30 <pkg.sumFast+0x20>"},
					{Addr: "16", Op: "vmovdqu64", Args: "(%rax),%zmm0"},
					{Addr: "1c", Op: "vzeroupper", Args: ""},
					{Addr: "30", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_GoPrologueGuardEVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "internal/simd.sumZ",
				Insns: []simdCfgInsnPayload{
					{Addr: "4a0000", Op: "cmp", Args: "0x10(%r14),%rsp"},
					{Addr: "4a0004", Op: "jbe", Args: "4a0030 <internal/simd.sumZ+0x30>"},
					{Addr: "4a0006", Op: "push", Args: "%rbp"},
					{Addr: "4a0007", Op: "vmovdqu64", Args: "(%rax),%zmm0"},
					{Addr: "4a000d", Op: "vzeroupper", Args: ""},
					{Addr: "4a0010", Op: "pop", Args: "%rbp"},
					{Addr: "4a0011", Op: "ret", Args: ""},
					{Addr: "4a0030", Op: "call", Args: "460000 <runtime.morestack_noctxt.abi0>", Raw: "call 460000 <runtime.morestack_noctxt.abi0>"},
					{Addr: "4a0035", Op: "jmp", Args: "4a0000 <internal/simd.sumZ>"},
				},
			},
		},
		{
			name:     "Neg_DirtyIndirectJmp",
			wantText: "INDIRECT_OR_UNRESOLVED_JMP",
			fn: simdCfgFuncPayload{
				Name: "pkg.f",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "jmp", Args: "*%rcx"},
				},
			},
		},
		{
			name:     "Neg_JumptableHidden",
			wantText: "INDIRECT_OR_UNRESOLVED_JMP",
			fn: simdCfgFuncPayload{
				Name: "pkg.sw",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "lea", Args: "0x100(%rip),%rdx"},
					{Addr: "1b", Op: "jmp", Args: "*(%rdx,%rcx,8)"},
					{Addr: "20", Op: "movups", Args: "%xmm0,(%rbx)"},
					{Addr: "23", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_DirtyCondExtJmp",
			wantText: "DIRTY_COND_EXT_JMP",
			fn: simdCfgFuncPayload{
				Name: "pkg.g",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "test", Args: "%rbx,%rbx"},
					{Addr: "17", Op: "je", Args: "9999 <other.func>"},
					{Addr: "19", Op: "vzeroupper", Args: ""},
					{Addr: "1c", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_RepzRet",
			wantText: "DIRTY_RET",
			fn: simdCfgFuncPayload{
				Name: "pkg.h",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "repz", Args: "ret"},
				},
			},
		},
		{
			name:     "Neg_Plan9Syntax",
			wantText: "Plan 9",
			fn: simdCfgFuncPayload{
				Name: "pkg.k",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "VMOVDQU", Args: "(AX), Y0"},
					{Addr: "14", Op: "MOVUPS", Args: "X0, (BX)"},
					{Addr: "18", Op: "RET", Args: ""},
				},
			},
		},
		{
			name:     "Neg_Ymm16EVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.m",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu64", Args: "(%rax),%ymm16"},
					{Addr: "16", Op: "vzeroupper", Args: ""},
					{Addr: "19", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_Ymm17EVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.m17",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vaddpd", Args: "%ymm17,%ymm0,%ymm0"},
					{Addr: "16", Op: "vzeroupper", Args: ""},
					{Addr: "19", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_Xmm20EVEX",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.m20",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vaddpd", Args: "%xmm20,%xmm0,%xmm0"},
					{Addr: "16", Op: "vzeroupper", Args: ""},
					{Addr: "19", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_VPTERNLOGQ",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.tern",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "VPTERNLOGQ", Args: "$0xff,%ymm0,%ymm1,%ymm2"},
					{Addr: "16", Op: "vzeroupper", Args: ""},
					{Addr: "19", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_RawSpoofPanicComment",
			wantText: "DIRTY_CALL",
			fn: simdCfgFuncPayload{
				Name: "pkg.n",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "call", Args: "4000 <pkg.helper>", Raw: "call 4000 <pkg.helper> // runtime.panicIndex"},
					{Addr: "19", Op: "vzeroupper", Args: ""},
					{Addr: "1c", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_DuplicateAddr",
			wantText: "duplicate instruction address",
			fn: simdCfgFuncPayload{
				Name: "pkg.p",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "jmp", Args: "20"},
					{Addr: "20", Op: "vzeroupper", Args: ""},
					{Addr: "20", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "24", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_FallOffEndDirty",
			wantText: "DIRTY_FALL_OFF_END",
			fn: simdCfgFuncPayload{
				Name: "pkg.r",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "nop", Args: ""},
				},
			},
		},
		{
			name:     "Neg_JmpHexPrefix",
			wantText: "DIRTY_RET",
			fn: simdCfgFuncPayload{
				Name: "pkg.u",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "jmp", Args: "0x4000"},
				},
			},
		},
		{
			name:     "Neg_ZeroInsns",
			wantText: "0 instructions",
			fn: simdCfgFuncPayload{
				Name:  "pkg.empty",
				Insns: []simdCfgInsnPayload{},
			},
		},
		{
			name:     "Neg_NotrackIndirectJmp",
			wantText: "INDIRECT_OR_UNRESOLVED_JMP",
			fn: simdCfgFuncPayload{
				Name: "pkg.ntj",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "notrack", Args: "jmp *%rcx"},
					{Addr: "18", Op: "vzeroupper", Args: ""},
					{Addr: "1b", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_BndDirtyCall",
			wantText: "DIRTY_CALL",
			fn: simdCfgFuncPayload{
				Name: "pkg.bc",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "bnd", Args: "call 4000 <pkg.helper>"},
					{Addr: "19", Op: "vzeroupper", Args: ""},
					{Addr: "1c", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_ChainedPrefixDirtyJmp",
			wantText: "DIRTY_RET",
			fn: simdCfgFuncPayload{
				Name: "pkg.cpj",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "data16", Args: "cs notrack jmp 0x4000 <pkg.ext>"},
					{Addr: "1a", Op: "vzeroupper", Args: ""},
					{Addr: "1d", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_PrefixEVEXUnguarded",
			wantText: "UNGUARDED_EVEX",
			fn: simdCfgFuncPayload{
				Name: "pkg.pev",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "cs", Args: "vpternlogq $0xff,%xmm0,%xmm1,%xmm2"},
					{Addr: "18", Op: "vzeroupper", Args: ""},
					{Addr: "1b", Op: "ret", Args: ""},
				},
			},
		},
		{
			name:     "Neg_RexPrefixDirtyRet",
			wantText: "DIRTY_RET",
			fn: simdCfgFuncPayload{
				Name: "pkg.rexret",
				Insns: []simdCfgInsnPayload{
					{Addr: "10", Op: "vmovdqu", Args: "(%rax),%ymm0"},
					{Addr: "14", Op: "rex.W", Args: "retq"},
					{Addr: "16", Op: "vzeroupper", Args: ""},
					{Addr: "19", Op: "ret", Args: ""},
				},
			},
		},
	}

	for _, tc := range negCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, out := runSimdCfgCheckJSON(t, simdExe, tc.name+".json", simdCfgRequest{
				Functions: []simdCfgFuncPayload{tc.fn},
			})
			if code == 0 || !strings.Contains(out, tc.wantText) {
				t.Fatalf("expected REJECTED with %s (exit=%d):\n%s", tc.wantText, code, out)
			}
		})
	}

	rawNegCases := []struct {
		name     string
		wantText string
		raw      string
	}{
		{
			name:     "Neg_DuplicateFunctionsKey",
			wantText: "duplicate JSON object key",
			raw:      `{"functions":[{"name":"pkg.q","insns":[{"addr":"10","op":"vmovdqu","args":"(%rax),%ymm0"},{"addr":"14","op":"ret","args":""}]}],"functions":[]}`,
		},
		{
			name:     "Neg_EmptyFunctions",
			wantText: "non-empty",
			raw:      `{"functions":[]}`,
		},
	}
	for _, tc := range rawNegCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, out := runSimdCfgCheckRaw(t, simdExe, tc.name+".json", tc.raw)
			if code == 0 || !strings.Contains(out, tc.wantText) {
				t.Fatalf("expected REJECTED with %s (exit=%d):\n%s", tc.wantText, code, out)
			}
		})
	}

	t.Run("Neg_EmptyBatch", func(t *testing.T) {
		t.Parallel()
		code, out := runToolWithExitCode(t, simdExe, "--batch", "[]")
		if code == 0 || !strings.Contains(out, "non-empty") {
			t.Fatalf("expected REJECTED on empty --batch [] (exit=%d):\n%s", code, out)
		}
	})
}
