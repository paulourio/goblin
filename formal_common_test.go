package goblin

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

type ratPayload struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

func requireFormalEnv(tb testing.TB) {
	tb.Helper()
	if os.Getenv("GOBLIN_FORMAL") != "1" {
		tb.Skip("set GOBLIN_FORMAL=1 to run Lean 4 Prove Lane checks")
	}
}

func findFormalDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("resolve working directory: %v", err)
	}
	for {
		lakefile := filepath.Join(dir, "formal", "lakefile.toml")
		if info, statErr := os.Stat(lakefile); statErr == nil && !info.IsDir() {
			return filepath.Join(dir, "formal")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("locate formal/lakefile.toml from %s", dir)
		}
		dir = parent
	}
}

func locateLakeBinary(tb testing.TB) string {
	tb.Helper()
	if p, err := exec.LookPath("lake"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		tb.Fatalf("locate lake binary and user home directory: %v", err)
	}
	candidate := filepath.Join(home, ".elan", "bin", "lake")
	if _, statErr := os.Stat(candidate); statErr != nil {
		tb.Fatalf("lake binary not found in PATH or %s: %v", candidate, statErr)
	}
	return candidate
}

func formalExe(tb testing.TB, exeName string) string {
	tb.Helper()
	formalDir := findFormalDir(tb)
	binName := exeName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(formalDir, ".lake", "build", "bin", binName)
	if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
		return binPath
	}

	lakePath := locateLakeBinary(tb)
	var cmd *exec.Cmd
	if flockPath, err := exec.LookPath("flock"); err == nil {
		cmd = exec.CommandContext(tb.Context(), flockPath, "/tmp/goblin-formal-lake.lock", lakePath, "build", exeName)
	} else {
		cmd = exec.CommandContext(tb.Context(), lakePath, "build", exeName)
	}
	cmd.Dir = formalDir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(lakePath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("lake build %s failed: %v\n%s", exeName, err, out)
	}
	if _, err := os.Stat(binPath); err != nil {
		tb.Fatalf("built binary missing at %s: %v", binPath, err)
	}
	return binPath
}

func bigRatToPayload(r *big.Rat) ratPayload {
	if r == nil {
		return ratPayload{Num: "0", Den: "1"}
	}
	return ratPayload{
		Num: r.Num().String(),
		Den: r.Denom().String(),
	}
}

func float64ToPayload(v float64) (ratPayload, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ratPayload{}, fmt.Errorf("non-finite float64 %v", v)
	}
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		return ratPayload{}, fmt.Errorf("convert float64 %v to rational failed", v)
	}
	return bigRatToPayload(r), nil
}

func int64ToPayload(v int64) ratPayload {
	return ratPayload{
		Num: strconv.FormatInt(v, 10),
		Den: "1",
	}
}

func TestFormalCommonRatPayload(t *testing.T) {
	t.Parallel()

	pInt := int64ToPayload(-42)
	if pInt.Num != "-42" || pInt.Den != "1" {
		t.Fatalf("unexpected int64 payload: %+v", pInt)
	}

	// Verify exact dyadic encoding for 0.1 without 64-bit truncation.
	pFloat, err := float64ToPayload(0.1)
	if err != nil {
		t.Fatalf("float64ToPayload(0.1) failed: %v", err)
	}
	if pFloat.Num != "3602879701896397" || pFloat.Den != "36028797018963968" {
		t.Fatalf("unexpected 0.1 dyadic payload: %+v", pFloat)
	}

	// Verify >64-bit rational serialization via big.Rat.
	bigNum, ok := new(big.Int).SetString("123456789012345678901234567890", 10)
	if !ok {
		t.Fatal("failed to parse large integer")
	}
	pBig := bigRatToPayload(new(big.Rat).SetFrac(bigNum, big.NewInt(7)))
	raw, err := json.Marshal(pBig)
	if err != nil {
		t.Fatalf("json.Marshal(ratPayload) failed: %v", err)
	}
	var decoded ratPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ratPayload) failed: %v", err)
	}
	if decoded != pBig {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", decoded, pBig)
	}

	if _, err := float64ToPayload(math.Inf(1)); err == nil {
		t.Fatal("expected error for +Inf")
	}
	if _, err := float64ToPayload(math.NaN()); err == nil {
		t.Fatal("expected error for NaN")
	}

	t.Run("FormalExeSmoke", func(t *testing.T) {
		t.Parallel()
		requireFormalEnv(t)
		exe := formalExe(t, "lp-check")
		cmd := exec.CommandContext(t.Context(), exe)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", exe, err, out)
		}
	})
}
