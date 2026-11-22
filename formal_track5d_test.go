package goblin

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/paulourio/goblin/internal/simd"
)

var (
	reLeanHexWord       = regexp.MustCompile(`0x([0-9a-fA-F]{8})`)
	reLeanTargetArm     = regexp.MustCompile(`^\s*\|([\d\s|]+)=>\s*some\s*"([^"]+)"`)
	reLeanDefPbPrefix   = regexp.MustCompile(`^\s*let\s+defPb\s*:=\s*\[([^\]]*)\]`)
	reLeanIfPbElem      = regexp.MustCompile(`if\s+idx\s*∈\s*defPb\s+then\s+some\s*"([^"]+)"`)
	reGoToolObjdumpText = regexp.MustCompile(`^TEXT\s+(\S+)\(SB\)`)
	reGoToolObjdumpWord = regexp.MustCompile(`^\s*\S+\s+0x[0-9a-fA-F]+\s+([0-9a-fA-F]{8})\s+`)
)

func normalizeArm64WordTrack5D(w uint32, isExternalBranch bool) uint32 {
	if isExternalBranch && ((w&0xFC000000) == 0x14000000 || (w&0xFC000000) == 0x94000000) {
		return w & 0xFC000000
	}
	if (w & 0x9F000000) == 0x90000000 {
		return w & 0x9F00001F
	}
	if (w & 0xFFC003E0) == 0xFD400360 {
		return w & 0xFFC003FF
	}
	return w
}

func parseLeanTrack5DRoutine(tb testing.TB, leanSrc, baseName string) ([]uint32, []binTargetEntry) {
	tb.Helper()
	chunksMarker := "def " + baseName + "Chunks : List (List UInt32) := ["
	cStart := strings.Index(leanSrc, chunksMarker)
	if cStart < 0 {
		tb.Fatalf("missing %s in Track5D.lean", chunksMarker)
	}
	cBody := leanSrc[cStart+len(chunksMarker):]
	cEnd := strings.Index(cBody, "\ndef "+baseName+"Words")
	if cEnd < 0 {
		tb.Fatalf("missing %sWords terminator in Track5D.lean", baseName)
	}
	matches := reLeanHexWord.FindAllStringSubmatch(cBody[:cEnd], -1)
	if len(matches) == 0 {
		tb.Fatalf("parsed zero instruction words for %s", baseName)
	}
	words := make([]uint32, 0, len(matches))
	for _, m := range matches {
		v, err := strconv.ParseUint(m[1], 16, 32)
		if err != nil {
			tb.Fatalf("parse word %q: %v", m[1], err)
		}
		words = append(words, uint32(v))
	}

	tgtPrefix := "def " + baseName + "Targets"
	tStart := strings.Index(leanSrc, tgtPrefix)
	if tStart < 0 {
		tb.Fatalf("missing %s in Track5D.lean", tgtPrefix)
	}
	tLines := strings.Split(leanSrc[tStart:], "\n")
	if strings.Contains(tLines[0], ":= none") {
		return words, nil
	}

	byIdx := make(map[int]string)
	var defPbIndices []int
	defPbSym := ""
	for i := 1; i < len(tLines); i++ {
		line := tLines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "/--") || strings.HasPrefix(trimmed, "/-!") {
			break
		}
		if m := reLeanDefPbPrefix.FindStringSubmatch(line); m != nil {
			for tok := range strings.SplitSeq(m[1], ",") {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				idx, err := strconv.Atoi(tok)
				if err != nil {
					tb.Fatalf("parse defPb index %q: %v", tok, err)
				}
				defPbIndices = append(defPbIndices, idx)
			}
		}
		if m := reLeanIfPbElem.FindStringSubmatch(line); m != nil {
			defPbSym = m[1]
		}
		if m := reLeanTargetArm.FindStringSubmatch(line); m != nil {
			for rawIdx := range strings.SplitSeq(m[1], "|") {
				rawIdx = strings.TrimSpace(rawIdx)
				if rawIdx == "" {
					continue
				}
				idx, err := strconv.Atoi(rawIdx)
				if err != nil {
					tb.Fatalf("parse target index %q: %v", rawIdx, err)
				}
				byIdx[idx] = m[2]
			}
		}
	}
	if defPbSym != "" {
		for _, idx := range defPbIndices {
			if _, exists := byIdx[idx]; !exists {
				byIdx[idx] = defPbSym
			}
		}
	}
	targets := make([]binTargetEntry, 0, len(byIdx))
	for idx, sym := range byIdx {
		targets = append(targets, binTargetEntry{Idx: idx, Symbol: sym})
	}
	slices.SortFunc(targets, func(a, b binTargetEntry) int {
		return a.Idx - b.Idx
	})
	return words, targets
}

func assertTrack5DRoutineMatchesLean(tb testing.TB, leanSrc, binPath, goSym, leanBase string) {
	tb.Helper()
	gotMap := extractArm64Routines(tb, binPath, []string{goSym})
	got := gotMap[goSym]
	wantWords, wantTargets := parseLeanTrack5DRoutine(tb, leanSrc, leanBase)

	if len(got.words) != len(wantWords) {
		tb.Fatalf("%s (%s): word count %d != Lean %d", goSym, leanBase, len(got.words), len(wantWords))
	}
	if !slices.Equal(got.targets, wantTargets) {
		tb.Fatalf("%s (%s): relocation targets %v != Lean %v", goSym, leanBase, got.targets, wantTargets)
	}
	extSet := make(map[int]bool, len(wantTargets))
	for _, te := range wantTargets {
		extSet[te.Idx] = true
	}
	for i := range wantWords {
		gn := normalizeArm64WordTrack5D(got.words[i], extSet[i])
		wn := normalizeArm64WordTrack5D(wantWords[i], extSet[i])
		if gn != wn {
			tb.Fatalf("%s (%s) word[%d]: ELF 0x%08x (norm 0x%08x) != Lean 0x%08x (norm 0x%08x)",
				goSym, leanBase, i, got.words[i], gn, wantWords[i], wn)
		}
	}

	// Cross-check instruction words against go tool objdump -s output.
	code, disasm := runToolWithExitCode(tb, "go", "tool", "objdump", "-s", "^"+regexp.QuoteMeta(track5ModSlashPrefix+goSym)+"$", binPath)
	if code != 0 {
		tb.Fatalf("go tool objdump failed for %s (exit=%d):\n%s", goSym, code, disasm)
	}
	var objdumpWords []uint32
	inTargetFunc := false
	for line := range strings.SplitSeq(disasm, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := reGoToolObjdumpText.FindStringSubmatch(line); m != nil {
			inTargetFunc = stripModulePrefixes(m[1]) == goSym
			continue
		}
		if !inTargetFunc {
			continue
		}
		if m := reGoToolObjdumpWord.FindStringSubmatch(line); m != nil {
			w, err := strconv.ParseUint(m[1], 16, 32)
			if err != nil {
				tb.Fatalf("parse objdump word %q: %v", m[1], err)
			}
			objdumpWords = append(objdumpWords, uint32(w))
		}
	}
	for len(objdumpWords) > 0 && objdumpWords[len(objdumpWords)-1] == 0 {
		objdumpWords = objdumpWords[:len(objdumpWords)-1]
	}
	if !slices.Equal(objdumpWords, got.words) {
		tb.Fatalf("%s objdump words (len %d) != ELF .text words (len %d)", goSym, len(objdumpWords), len(got.words))
	}
}

func TestFormalTrack5D_ELFDisassemblyMatchesLean(t *testing.T) {
	t.Parallel()

	formalDir := findFormalDir(t)
	repoRoot := filepath.Dir(formalDir)
	leanPath := filepath.Join(formalDir, "GoblinFormal", "Track5D.lean")
	leanBytes, err := os.ReadFile(leanPath)
	if err != nil {
		t.Fatalf("read Track5D.lean: %v", err)
	}
	leanSrc := string(leanBytes)

	tmp := t.TempDir()
	assignNoinlineELF := filepath.Join(tmp, "assign_arm64_noinline.elf")
	assignOptELF := filepath.Join(tmp, "assign_arm64_opt.elf")
	arm64Env := []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64"}
	buildTestBinary(t, assignNoinlineELF, "./assign", arm64Env, []string{"-gcflags=-l"})
	buildTestBinary(t, assignOptELF, "./assign", arm64Env, nil)

	t.Run("NoinlineLeavesAndExactReducedCost", func(t *testing.T) {
		t.Parallel()
		assertTrack5DRoutineMatchesLean(t, leanSrc, assignNoinlineELF, "assign.twoSumAssign", "twoSumAssign")
		assertTrack5DRoutineMatchesLean(t, leanSrc, assignNoinlineELF, "assign.clampFinite", "clampFinite")
		assertTrack5DRoutineMatchesLean(t, leanSrc, assignNoinlineELF, "assign.exactReducedCost", "exactReducedCostNoinline")
	})

	t.Run("OptimizedExactReducedCostAndKahnCycleChecker", func(t *testing.T) {
		t.Parallel()
		assertTrack5DRoutineMatchesLean(t, leanSrc, assignOptELF, "assign.exactReducedCost", "exactReducedCost")
		assertTrack5DRoutineMatchesLean(t, leanSrc, assignOptELF, "assign.hasPotentialCancellation", "hasPotentialCancellation")
	})

	t.Run("ExtractorScriptVerifyMode", func(t *testing.T) {
		t.Parallel()
		pyExe, lookErr := exec.LookPath("python3")
		if lookErr != nil {
			t.Skip("python3 unavailable in PATH")
		}
		scriptPath := filepath.Join(repoRoot, "scripts", "extract_elf_track5.py")
		code, out := runToolWithExitCode(t, pyExe, scriptPath, "--verify", "--repo-root", repoRoot)
		if code != 0 || !strings.Contains(out, "ALL 14 Track 5A/5B/5D arm64 ELF routines verified cleanly.") {
			t.Fatalf("extract_elf_track5.py --verify failed (exit=%d):\n%s", code, out)
		}
	})
}

func TestFormalTrack5D_LeanProofCheck(t *testing.T) {
	requireFormalEnv(t)
	t.Parallel()

	formalDir := findFormalDir(t)
	lakePath := locateLakeBinary(t)
	cmd := exec.CommandContext(t.Context(), lakePath, "env", "lean", filepath.Join("GoblinFormal", "Track5D.lean"))
	cmd.Dir = formalDir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(lakePath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lake env lean GoblinFormal/Track5D.lean failed: %v\n%s", err, out)
	}
}

func assertFloat64BitsEq(tb testing.TB, ctx string, got, want float64) {
	tb.Helper()
	if math.IsNaN(got) && math.IsNaN(want) {
		return
	}
	if math.Float64bits(got) != math.Float64bits(want) {
		tb.Fatalf("%s: got %v (0x%016x) != want %v (0x%016x)",
			ctx, got, math.Float64bits(got), want, math.Float64bits(want))
	}
}

func assertFloat64SliceBitsEq(tb testing.TB, ctx string, got, want []float64) {
	tb.Helper()
	if len(got) != len(want) {
		tb.Fatalf("%s: slice len %d != %d", ctx, len(got), len(want))
	}
	for i := range want {
		if math.IsNaN(got[i]) && math.IsNaN(want[i]) {
			continue
		}
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			tb.Fatalf("%s[%d]: got %v (0x%016x) != want %v (0x%016x)",
				ctx, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func buildTrack5DEdgeRows(n int) [][]float64 {
	negZero := math.Copysign(0, -1)
	posZero := 0.0
	minSub := math.SmallestNonzeroFloat64
	maxSub := math.Float64frombits(0x000fffffffffffff)
	minNegSub := -minSub
	maxNegSub := -maxSub
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

	// 1. Signed-zero pairs (+0.0 before -0.0 and -0.0 before +0.0) across head, vector body, and tail.
	rPosNeg := fillConst(4.0)
	rNegPos := fillConst(4.0)
	for _, idx := range []int{0, 3, 4, 8, 17, 31, 32, 129, 254, n - 2, n - 1} {
		if idx >= 0 && idx < n {
			if idx%2 == 0 {
				rPosNeg[idx] = posZero
				rNegPos[idx] = negZero
			} else {
				rPosNeg[idx] = negZero
				rNegPos[idx] = posZero
			}
		}
	}
	rows = append(rows, rPosNeg, rNegPos)

	// 2. Subnormals, signed zeros, and minPosNormal staircase.
	palette := []float64{
		maxNegSub, minNegSub, negZero, posZero, minSub, maxSub, minNorm, -minNorm, 1.5, -2.25,
	}
	rSub := make([]float64, n)
	rSubRev := make([]float64, n)
	for i := range n {
		rSub[i] = palette[i%len(palette)]
		rSubRev[i] = palette[(len(palette)-1)-(i%len(palette))]
	}
	rows = append(rows, rSub, rSubRev)

	// 3. +Inf tail padding, all-+Inf row, and single finite entry with +Inf tail.
	rows = append(rows, fillConst(posInf))
	rInfTail := fillConst(posInf)
	if n > 0 {
		rInfTail[n/3] = minSub
	}
	if n > 4 {
		rInfTail[n-1] = negZero
	}
	rows = append(rows, rInfTail)

	// 4. Descending and ascending distinct ramps plus -Inf sentinel and ties.
	rDesc := make([]float64, n)
	rAsc := make([]float64, n)
	rTies := fillConst(-0.75)
	for i := range n {
		rDesc[i] = float64(n - i)
		rAsc[i] = float64(i + 1)
	}
	if n > 5 {
		rTies[n/4] = -3.5
		rTies[(3*n)/4] = -3.5
		rAsc[n-1] = negInf
	}
	rows = append(rows, rDesc, rAsc, rTies)

	// 5. NaN propagation in head, vector body, and unaligned tail.
	if n > 0 {
		for _, nanIdx := range []int{0, n / 2, n - 1} {
			rNaN := slices.Clone(rSub)
			rNaN[nanIdx] = math.NaN()
			rows = append(rows, rNaN)
		}
	}

	return rows
}

func TestFormalTrack5D_SIMDParityAndEdgeAudit(t *testing.T) {
	t.Parallel()

	lengths := []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 191, 192, 193, 255, 256, 257, 263, 512, 515}
	zeroDual := make([]float64, 516)
	altDual := make([]float64, 516)
	for i := range altDual {
		if i%5 == 0 {
			altDual[i] = math.Copysign(0, -1)
		} else if i%7 == 0 {
			altDual[i] = math.SmallestNonzeroFloat64
		} else {
			altDual[i] = 0.0
		}
	}

	for _, n := range lengths {
		t.Run("Len_"+strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			vZero := zeroDual[:n]
			vAlt := altDual[:n]
			for rIdx, row := range buildTrack5DEdgeRows(n) {
				ctx := "n=" + strconv.Itoa(n) + "/case=" + strconv.Itoa(rIdx)
				for _, v := range [][]float64{vZero, vAlt} {
					// 1. Min2ReducedRow64With vs Scalar (bit-exact values and witness indices).
					u1S, u2S, j1S, j2S := simd.Min2ReducedRow64With(row, v, false)
					u1R, u2R, j1R, j2R := simd.Min2ReducedRow64Scalar(row, v)
					assertFloat64BitsEq(t, ctx+"/Min2.u1", u1S, u1R)
					assertFloat64BitsEq(t, ctx+"/Min2.u2", u2S, u2R)
					if j1S != j1R || j2S != j2R {
						t.Fatalf("%s/Min2.idx: SIMD (%d,%d) != Scalar (%d,%d)", ctx, j1S, j2S, j1R, j2R)
					}

					// 2. MinReducedSlice64With and MinReducedSkip64With vs Scalar.
					msS := simd.MinReducedSlice64With(row, v, false)
					msR := simd.MinReducedSlice64Scalar(row, v)
					assertFloat64BitsEq(t, ctx+"/MinReducedSlice", msS, msR)

					for _, skip := range []int{-1, 0, n / 2, n - 1, n} {
						skS := simd.MinReducedSkip64With(row, v, skip, false)
						skR := simd.MinReducedSkip64Scalar(row, v, skip)
						assertFloat64BitsEq(t, ctx+"/MinReducedSkip", skS, skR)
					}

					// 3. InitReducedRow64With vs Scalar.
					dstS := make([]float64, n)
					dstR := make([]float64, n)
					irS := simd.InitReducedRow64With(dstS, row, v, 0.0, false)
					irR := simd.InitReducedRow64Scalar(dstR, row, v, 0.0)
					assertFloat64BitsEq(t, ctx+"/InitReduced.min", irS, irR)
					assertFloat64SliceBitsEq(t, ctx+"/InitReduced.dst", dstS, dstR)

					// 4. RelaxRow64With vs Scalar across tight and non-tight thresholds via the documented resumption loop.
					for _, minVal := range []float64{-10.0, 0.0, math.Copysign(0, -1), math.SmallestNonzeroFloat64} {
						runRelax := func(disableSIMD bool) ([]float64, []int, []int) {
							d := make([]float64, n)
							pred := make([]int, n)
							for i := range n {
								d[i] = 2.0
								pred[i] = -1
							}
							var tights []int
							for j := 0; j < n; {
								j = simd.RelaxRow64With(row, v, d, pred, j, 3, 0.0, minVal, disableSIMD)
								if j >= n {
									break
								}
								lim := min(j+4, n)
								for ; j < lim; j++ {
									r := minVal + ((row[j] - v[j]) - 0.0)
									if r < d[j] {
										pred[j] = 3
										if r <= minVal {
											tights = append(tights, j)
											d[j] = math.NaN()
										} else {
											d[j] = r
										}
									}
								}
							}
							return d, pred, tights
						}
						dS, pS, tS := runRelax(false)
						dR, pR, tR := runRelax(true)
						if !slices.Equal(tS, tR) {
							t.Fatalf("%s/RelaxRow.tights: SIMD %v != Scalar %v", ctx, tS, tR)
						}
						assertFloat64SliceBitsEq(t, ctx+"/RelaxRow.d", dS, dR)
						if !slices.Equal(pS, pR) {
							t.Fatalf("%s/RelaxRow.pred: SIMD %v != Scalar %v", ctx, pS, pR)
						}
					}
				}

				// 5. ValidateAndNegate64With, AbsMax64, AllAbsLE64With, and MinNoNaN64.
				negS := make([]float64, n)
				negR := make([]float64, n)
				okS := simd.ValidateAndNegate64With(negS, row, 1e100, false)
				okR := simd.ValidateAndNegate64Scalar(negR, row, 1e100)
				if okS != okR {
					t.Fatalf("%s/ValidateAndNegate.ok: SIMD %v != Scalar %v", ctx, okS, okR)
				}
				assertFloat64SliceBitsEq(t, ctx+"/ValidateAndNegate.dst", negS, negR)

				amS := simd.AbsMax64(row)
				amR := simd.AbsMax64Scalar(row)
				assertFloat64BitsEq(t, ctx+"/AbsMax64", amS, amR)

				leS := simd.AllAbsLE64With(row, 1.0, false)
				leR := simd.AllAbsLE64Scalar(row, 1.0)
				if leS != leR {
					t.Fatalf("%s/AllAbsLE64: SIMD %v != Scalar %v", ctx, leS, leR)
				}

				mnS := simd.MinNoNaN64With(row, false)
				mnR := simd.MinNoNaN64Scalar(row)
				assertFloat64BitsEq(t, ctx+"/MinNoNaN64", mnS, mnR)

				// CollectLE64With single-call parity across full buffers and unaligned vector tails.
				for _, limit := range []float64{-1.0, 0.0, math.Copysign(0, -1), 1.5} {
					for _, outCap := range []int{1, 2, 4, 8} {
						outS := make([]int, outCap)
						outR := make([]int, outCap)
						nextS, cntS := simd.CollectLE64With(row, 0, limit, outS, false)
						nextR, cntR := simd.CollectLE64Scalar(row, 0, limit, outR)
						if nextS != nextR || cntS != cntR || !slices.Equal(outS[:cntS], outR[:cntR]) {
							t.Fatalf("%s/CollectLE64(lim=%v,cap=%d): SIMD (next=%d,cnt=%d,%v) != Scalar (next=%d,cnt=%d,%v)",
								ctx, limit, outCap, nextS, cntS, outS[:cntS], nextR, cntR, outR[:cntR])
						}
					}
				}

				// 6. SelectDualLeaveRow64With vs Scalar.
				lb := make([]float64, n)
				ub := make([]float64, n)
				tol := make([]float64, n)
				dse := make([]float64, n)
				for i := range n {
					lb[i] = -math.SmallestNonzeroFloat64
					ub[i] = math.SmallestNonzeroFloat64
					tol[i] = 0.0
					dse[i] = 1.0 + float64(i%3)
				}
				lrS, ldS, piS := simd.SelectDualLeaveRow64With(row, lb, ub, tol, dse, false)
				lrR, ldR, piR := simd.SelectDualLeaveRow64Scalar(row, lb, ub, tol, dse)
				if lrS != lrR {
					t.Fatalf("%s/SelectDualLeave.row: SIMD %d != Scalar %d", ctx, lrS, lrR)
				}
				assertFloat64BitsEq(t, ctx+"/SelectDualLeave.dir", ldS, ldR)
				assertFloat64BitsEq(t, ctx+"/SelectDualLeave.infeas", piS, piR)
			}
		})
	}

	// Matrix-level ColReduceValidateRows64With, ColReduceRows64With, and Transpose64With parity.
	for _, dim := range []int{1, 4, 7, 8, 9, 17, 33} {
		t.Run("Mat_"+strconv.Itoa(dim), func(t *testing.T) {
			t.Parallel()
			mat := make([]float64, dim*dim)
			rows := buildTrack5DEdgeRows(dim)
			for i := range dim {
				src := rows[i%len(rows)]
				for j := range dim {
					v := src[j]
					if math.IsNaN(v) || math.IsInf(v, 0) {
						v = math.Copysign(0, -1)
					}
					mat[i*dim+j] = v
				}
			}

			for _, neg := range []bool{false, true} {
				trS := make([]float64, dim*dim)
				trR := make([]float64, dim*dim)
				simd.Transpose64With(trS, mat, dim, dim, neg, false)
				simd.Transpose64Scalar(trR, mat, dim, dim, neg)
				assertFloat64SliceBitsEq(t, "Transpose64", trS, trR)
			}

			vS := make([]float64, dim)
			vR := make([]float64, dim)
			r4S := make([]int, dim)
			r4R := make([]int, dim)
			okS := simd.ColReduceValidateRows64With(dim, 0, dim, mat, vS, r4S, 1e100, false)
			okR := simd.ColReduceValidateRows64Scalar(dim, 0, dim, mat, vR, r4R, 1e100)
			if okS != okR || !slices.Equal(r4S, r4R) {
				t.Fatalf("ColReduceValidate(%d): ok (%v vs %v) r4c (%v vs %v)", dim, okS, okR, r4S, r4R)
			}
			assertFloat64SliceBitsEq(t, "ColReduceValidate.v", vS, vR)

			v2S := make([]float64, dim)
			v2R := make([]float64, dim)
			r2S := make([]int, dim)
			r2R := make([]int, dim)
			simd.ColReduceRows64With(dim, 0, dim, mat, v2S, r2S, false)
			simd.ColReduceRows64Scalar(dim, 0, dim, mat, v2R, r2R)
			if !slices.Equal(r2S, r2R) {
				t.Fatalf("ColReduceRows64(%d): r4c %v != %v", dim, r2S, r2R)
			}
			assertFloat64SliceBitsEq(t, "ColReduceRows64.v", v2S, v2R)
		})
	}
}
