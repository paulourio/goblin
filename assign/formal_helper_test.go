package assign

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

func formalExe(tb testing.TB, exeName string) string {
	tb.Helper()
	wd, err := os.Getwd()
	if err != nil {
		tb.Fatalf("getwd: %v", err)
	}
	var formalDir string
	for cur := wd; ; {
		cand := filepath.Join(cur, "formal", "lakefile.toml")
		if info, statErr := os.Stat(cand); statErr == nil && !info.IsDir() {
			formalDir = filepath.Join(cur, "formal")
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			tb.Fatalf("locate formal/lakefile.toml from %s", wd)
		}
		cur = parent
	}

	binName := exeName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(formalDir, ".lake", "build", "bin", binName)
	if info, statErr := os.Stat(binPath); statErr == nil && !info.IsDir() {
		return binPath
	}

	lakePath, lookErr := exec.LookPath("lake")
	if lookErr != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			tb.Fatalf("locate lake binary: %v (user home: %v)", lookErr, homeErr)
		}
		lakePath = filepath.Join(home, ".elan", "bin", "lake")
		if _, statErr := os.Stat(lakePath); statErr != nil {
			tb.Fatalf("lake not found in PATH or %s: %v", lakePath, statErr)
		}
	}

	var cmd *exec.Cmd
	if flockPath, flockErr := exec.LookPath("flock"); flockErr == nil {
		cmd = exec.CommandContext(tb.Context(), flockPath, "/tmp/goblin-formal-lake.lock", lakePath, "build", exeName)
	} else {
		cmd = exec.CommandContext(tb.Context(), lakePath, "build", exeName)
	}
	cmd.Dir = formalDir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(lakePath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		tb.Fatalf("lake build %s: %v\n%s", exeName, buildErr, out)
	}
	return binPath
}

func bigRatToPayload(r *big.Rat) ratPayload {
	if r == nil {
		return ratPayload{Num: "0", Den: "1"}
	}
	return ratPayload{Num: r.Num().String(), Den: r.Denom().String()}
}

func float64ToPayload(v float64) (ratPayload, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ratPayload{}, fmt.Errorf("non-finite float64 %v", v)
	}
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		return ratPayload{}, fmt.Errorf("exact rational conversion failed for %v", v)
	}
	return bigRatToPayload(r), nil
}

func int64ToPayload(v int64) ratPayload {
	return ratPayload{Num: strconv.FormatInt(v, 10), Den: "1"}
}

func TestFormalAssignRatPayload(t *testing.T) {
	t.Parallel()

	if got := int64ToPayload(17); got.Num != "17" || got.Den != "1" {
		t.Fatalf("unexpected int64ToPayload: %+v", got)
	}
	if got := bigRatToPayload(nil); got.Num != "0" || got.Den != "1" {
		t.Fatalf("unexpected nil bigRatToPayload: %+v", got)
	}
	p, err := float64ToPayload(0.25)
	if err != nil {
		t.Fatalf("float64ToPayload(0.25): %v", err)
	}
	if p.Num != "1" || p.Den != "4" {
		t.Fatalf("unexpected 0.25 payload: %+v", p)
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if string(raw) != `{"num":"1","den":"4"}` {
		t.Fatalf("unexpected JSON encoding %s", raw)
	}
	if _, err := float64ToPayload(math.Inf(-1)); err == nil {
		t.Fatal("expected error for -Inf")
	}

	t.Run("LsapExeSmoke", func(t *testing.T) {
		t.Parallel()
		requireFormalEnv(t)
		exe := formalExe(t, "lsap-check")
		cmd := exec.CommandContext(t.Context(), exe)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", exe, err, out)
		}
	})
}
