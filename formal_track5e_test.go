package goblin

import (
	"bytes"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

func runTrack5ECmd(tb testing.TB, dir string, extraEnv []string, name string, args ...string) (int, string) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), string(out)
	}
	tb.Fatalf("execute %s %v: %v\n%s", name, args, err, out)
	return -1, ""
}

func TestFormalTrack5E_ExtractorAndLeanVerify(t *testing.T) {
	requireFormalEnv(t)
	t.Parallel()

	formalDir := findFormalDir(t)
	repoRoot := filepath.Dir(formalDir)

	t.Run("ExtractorVerify", func(t *testing.T) {
		t.Parallel()
		pyExe, err := exec.LookPath("python3")
		if err != nil {
			t.Fatalf("python3 not found in PATH: %v", err)
		}
		scriptPath := filepath.Join(repoRoot, "scripts", "extract_amd64_track5e.py")
		code, out := runTrack5ECmd(
			t,
			repoRoot,
			[]string{"PYTHONDONTWRITEBYTECODE=1"},
			pyExe,
			scriptPath,
			"--verify",
			"--repo-root",
			repoRoot,
		)
		if code != 0 || !strings.Contains(out, "ALL 6 Track 5E amd64 AVX2/AVX-512 ELF kernels verified cleanly.") {
			t.Fatalf("extract_amd64_track5e.py --verify failed (exit=%d):\n%s", code, out)
		}
	})

	t.Run("LeanBuildAndAxiomAudit", func(t *testing.T) {
		t.Parallel()
		lakePath := locateLakeBinary(t)
		pathEnv := "PATH=" + filepath.Dir(lakePath) + string(os.PathListSeparator) + os.Getenv("PATH")

		oleanPath := filepath.Join(formalDir, ".lake", "build", "lib", "lean", "GoblinFormal", "Track5E.olean")
		leanSrcPath := filepath.Join(formalDir, "GoblinFormal", "Track5E.lean")
		needBuild := true
		if oleanInfo, err := os.Stat(oleanPath); err == nil {
			if srcInfo, srcErr := os.Stat(leanSrcPath); srcErr == nil && !oleanInfo.ModTime().Before(srcInfo.ModTime()) {
				needBuild = false
			}
		}
		if needBuild {
			var code int
			var out string
			if flockPath, err := exec.LookPath("flock"); err == nil {
				code, out = runTrack5ECmd(
					t, formalDir, []string{pathEnv},
					flockPath, "/tmp/goblin-formal-lake.lock", lakePath, "build", "GoblinFormal.Track5E",
				)
			} else {
				code, out = runTrack5ECmd(
					t, formalDir, []string{pathEnv},
					lakePath, "build", "GoblinFormal.Track5E",
				)
			}
			if code != 0 {
				t.Fatalf("lake build GoblinFormal.Track5E failed (exit=%d):\n%s", code, out)
			}
		}

		theorems := []string{
			"allAbsLE64AVX2_decodes",
			"allAbsLE64AVX512_decodes",
			"minNoNaN64AVX2_decodes",
			"minNoNaN64AVX512_decodes",
			"initReducedRow64AVX2_decodes",
			"initReducedRow64AVX512_decodes",
			"evex_broadcast_and_zeromask_decode_witness",
			"allAbsLE64AVX2_abi_and_zeroGC_safe",
			"allAbsLE64AVX512_abi_and_zeroGC_safe",
			"minNoNaN64AVX2_abi_and_zeroGC_safe",
			"minNoNaN64AVX512_abi_and_zeroGC_safe",
			"initReducedRow64AVX2_abi_and_zeroGC_safe",
			"initReducedRow64AVX512_abi_and_zeroGC_safe",
			"stripped_vzeroupper_rejected",
			"vminpd_track5d_canon_bridge",
			"minsd_por_eq_f64Min",
			"allAbsLE64AVX2_functional_equiv",
			"allAbsLE64AVX512_functional_equiv",
			"allAbsLE64_avx2_eq_avx512_mult8",
			"minNoNaN64AVX2_functional_equiv",
			"minNoNaN64AVX512_functional_equiv",
			"minNoNaN64_avx2_eq_avx512",
			"initReducedRow64AVX2_functional_equiv",
			"initReducedRow64AVX512_functional_equiv",
			"initReducedRow64_avx2_eq_avx512_mult8",
		}
		var script strings.Builder
		script.WriteString("import GoblinFormal.Track5E\nopen GoblinFormal.Track5E\n")
		for _, thm := range theorems {
			script.WriteString("#print axioms ")
			script.WriteString(thm)
			script.WriteByte('\n')
		}

		cmd := exec.CommandContext(t.Context(), lakePath, "env", "lean", "--stdin")
		cmd.Dir = formalDir
		cmd.Env = append(os.Environ(), pathEnv)
		cmd.Stdin = bytes.NewBufferString(script.String())
		outBytes, err := cmd.CombinedOutput()
		out := string(outBytes)
		if err != nil {
			t.Fatalf("axiom audit failed: %v\n%s", err, out)
		}
		for _, forbidden := range []string{"sorryAx", "Lean.trustCompiler", "Lean.ofReduceBool"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("forbidden axiom %q found in Track5E audit:\n%s", forbidden, out)
			}
		}
		for _, thm := range theorems {
			if !strings.Contains(out, thm) {
				t.Fatalf("missing theorem %q in axiom audit output:\n%s", thm, out)
			}
		}
	})
}

func TestFormalTrack5E_AdversarialNegativeControls(t *testing.T) {
	requireFormalEnv(t)
	t.Parallel()

	formalDir := findFormalDir(t)
	repoRoot := filepath.Dir(formalDir)
	leanPath := filepath.Join(formalDir, "GoblinFormal", "Track5E.lean")
	origBytes, err := os.ReadFile(leanPath)
	if err != nil {
		t.Fatalf("read Track5E.lean: %v", err)
	}
	orig := string(origBytes)

	pyExe, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3 not found in PATH: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "scripts", "extract_amd64_track5e.py")

	type mutationCase struct {
		name    string
		section string
		oldTok  string
		newTok  string
	}
	cases := []mutationCase{
		{
			name:    "MutatedOpcodeByteInAVX2",
			section: "def allAbsLE64AVX2ByteChunks",
			oldTok:  "0x55",
			newTok:  "0x90",
		},
		{
			name:    "StrippedVZEROUPPERInMinAVX2",
			section: "def minNoNaN64AVX2ByteChunks",
			oldTok:  "0xc5, 0xf8, 0x77",
			newTok:  "0x90, 0x90, 0x90",
		},
		{
			name:    "StrippedVZEROUPPERInInitAVX512",
			section: "def initReducedRow64AVX512ByteChunks",
			oldTok:  "0xc5, 0xf8, 0x77",
			newTok:  "0x90, 0x90, 0x90",
		},
		{
			name:    "CorruptedRodataPosInfInMinAVX512",
			section: "def minNoNaN64AVX512Rodata",
			oldTok:  "0x7ff0000000000000#64",
			newTok:  "0x7ff0000000000001#64",
		},
	}

	tmpDir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			secIdx := strings.Index(orig, tc.section)
			if secIdx < 0 {
				t.Fatalf("missing section %q in Track5E.lean", tc.section)
			}
			relIdx := strings.Index(orig[secIdx:], tc.oldTok)
			if relIdx < 0 {
				t.Fatalf("missing token %q after %q in Track5E.lean", tc.oldTok, tc.section)
			}
			pos := secIdx + relIdx
			mutated := orig[:pos] + tc.newTok + orig[pos+len(tc.oldTok):]

			mutPath := filepath.Join(tmpDir, tc.name+".lean")
			if writeErr := os.WriteFile(mutPath, []byte(mutated), 0o600); writeErr != nil {
				t.Fatalf("write mutated Track5E: %v", writeErr)
			}

			code, out := runTrack5ECmd(
				t,
				repoRoot,
				[]string{"PYTHONDONTWRITEBYTECODE=1"},
				pyExe,
				scriptPath,
				"--verify",
				"--repo-root",
				repoRoot,
				"--lean-file",
				mutPath,
			)
			if code == 0 {
				t.Fatalf("negative control %s unexpectedly passed --verify:\n%s", tc.name, out)
			}
		})
	}
}

func buildTrack5EAdversarialRows(n int) [][]float64 {
	negZero := math.Copysign(0, -1)
	posZero := 0.0
	minSub := math.SmallestNonzeroFloat64
	maxSub := math.Float64frombits(0x000fffffffffffff)
	minNorm := math.Float64frombits(0x0010000000000000)
	posInf := math.Inf(1)
	negInf := math.Inf(-1)

	fillConst := func(val float64) []float64 {
		row := make([]float64, n)
		for i := range n {
			row[i] = val
		}
		return row
	}

	var rows [][]float64

	// Target VMINPD signed-zero asymmetry across 2x-unrolled body, 1x vector tail, and scalar tail.
	rNegThenPos := fillConst(1.0)
	rPosThenNeg := fillConst(1.0)
	for _, idx := range []int{0, 1, 3, 4, 7, 8, 15, 16, 31, 32, 35, 36, 63, 64, 68, 72, n - 5, n - 4, n - 1} {
		if idx >= 0 && idx < n {
			if idx%2 == 0 {
				rNegThenPos[idx] = negZero
				rPosThenNeg[idx] = posZero
			} else {
				rNegThenPos[idx] = posZero
				rPosThenNeg[idx] = negZero
			}
		}
	}
	rows = append(rows, rNegThenPos, rPosThenNeg, fillConst(negZero), fillConst(posZero))

	// Subnormal and minPosNormal traps (positive and negative).
	palette := []float64{
		-maxSub, -minSub, negZero, posZero, minSub, maxSub, minNorm, -minNorm,
		0.5, -0.5, 1.0, -1.0,
	}
	rSub := make([]float64, n)
	rSubRev := make([]float64, n)
	for i := range n {
		rSub[i] = palette[i%len(palette)]
		rSubRev[i] = palette[(len(palette)-1)-(i%len(palette))]
	}
	rows = append(rows, rSub, rSubRev)

	// Infinities and NaN placements across head, AVX2/AVX-512 bodies, vector tails, and scalar tails.
	rows = append(rows, fillConst(posInf), fillConst(negInf), fillConst(math.NaN()))
	if n > 0 {
		for _, idx := range []int{0, n / 2, n - 1} {
			rNaN := slices.Clone(rSub)
			rNaN[idx] = math.NaN()
			rows = append(rows, rNaN)
		}
		rTailZero := fillConst(posInf)
		rTailZero[n-1] = negZero
		rows = append(rows, rTailZero)
	}
	return rows
}

func TestFormalTrack5E_LiveSIMDDifferential(t *testing.T) {
	t.Parallel()

	t.Logf("Track5E live SIMD environment: hardware=%s active=%s",
		simd.HardwareSIMDTier(), simd.ActiveSIMDTier())

	lengths := []int{
		0, 1, 2, 3, 4, 5, 7, 8, 9,
		15, 16, 17, 31, 32, 33, 35, 36, 39, 40,
		47, 48, 63, 64, 65, 71, 72, 79, 80,
		127, 128, 129, 255, 256, 257,
	}

	negZero := math.Copysign(0, -1)
	minSub := math.SmallestNonzeroFloat64
	maxSub := math.Float64frombits(0x000fffffffffffff)

	limits := []float64{-1.0, negZero, 0.0, minSub, maxSub, 1.0, math.Inf(1), math.NaN()}
	uiVals := []float64{negZero, 0.0, minSub, -1.25, math.Inf(1), math.NaN()}

	for _, n := range lengths {
		t.Run("Len_"+strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			v := make([]float64, n)
			for i := range n {
				switch i % 4 {
				case 0:
					v[i] = negZero
				case 1:
					v[i] = 0.0
				case 2:
					v[i] = minSub
				default:
					v[i] = -0.25
				}
			}
			dstSIMD := make([]float64, n)
			dstScalar := make([]float64, n)

			for rIdx, row := range buildTrack5EAdversarialRows(n) {
				ctx := "n=" + strconv.Itoa(n) + "/row=" + strconv.Itoa(rIdx)

				// 1. MinNoNaN64With vs MinNoNaN64Scalar (bit-exact including signed-zero canon).
				gotMin := simd.MinNoNaN64With(row, false)
				wantMin := simd.MinNoNaN64Scalar(row)
				assertFloat64BitsEq(t, ctx+"/MinNoNaN64", gotMin, wantMin)

				// 2. AllAbsLE64With vs AllAbsLE64Scalar across subnormal, signed-zero, Inf, NaN limits.
				for _, lim := range limits {
					gotOK := simd.AllAbsLE64With(row, lim, false)
					wantOK := simd.AllAbsLE64Scalar(row, lim)
					if gotOK != wantOK {
						t.Fatalf("%s/AllAbsLE64(lim=%v): SIMD=%v != Scalar=%v", ctx, lim, gotOK, wantOK)
					}
				}

				// 3. InitReducedRow64With vs InitReducedRow64Scalar (bit-exact dst + minVal).
				for _, ui := range uiVals {
					gotIR := simd.InitReducedRow64With(dstSIMD, row, v, ui, false)
					wantIR := simd.InitReducedRow64Scalar(dstScalar, row, v, ui)
					assertFloat64BitsEq(t, ctx+"/InitReduced.min", gotIR, wantIR)
					assertFloat64SliceBitsEq(t, ctx+"/InitReduced.dst", dstSIMD, dstScalar)
				}
			}
		})
	}
}

func TestFormalTrack5E_ZeroAllocContracts(t *testing.T) {
	const n = 129
	row := make([]float64, n)
	v := make([]float64, n)
	dst := make([]float64, n)
	for i := range n {
		row[i] = float64(i%17) - 8.0
		v[i] = 0.5
	}

	if allocs := testing.AllocsPerRun(50, func() {
		_ = simd.AllAbsLE64With(row, 10.0, false)
	}); allocs != 0 {
		t.Fatalf("AllAbsLE64With allocated %.1f times/op, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		_ = simd.MinNoNaN64With(row, false)
	}); allocs != 0 {
		t.Fatalf("MinNoNaN64With allocated %.1f times/op, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		_ = simd.InitReducedRow64With(dst, row, v, 0.25, false)
	}); allocs != 0 {
		t.Fatalf("InitReducedRow64With allocated %.1f times/op, want 0", allocs)
	}
}
