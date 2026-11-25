package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseProgressLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		line      string
		wantOK    bool
		wantKind  EventKind
		wantSec   float64
		wantExecs int64
		wantHasNI bool
		wantNI    int64
		wantTotal int64
	}{
		{
			name:   "blank",
			line:   "   ",
			wantOK: false,
		},
		{
			name:   "baseline_in_progress",
			line:   "fuzz: elapsed: 3s, gathering baseline coverage: 923/942 completed",
			wantOK: false,
		},
		{
			name:     "baseline_done",
			line:     "fuzz: elapsed: 3s, gathering baseline coverage: 942/942 completed, now fuzzing with 6 workers",
			wantOK:   true,
			wantKind: EventBaselineDone,
			wantSec:  3,
		},
		{
			name:     "seed_corpus_done",
			line:     "fuzz: elapsed: 0s, testing seed corpus: 13/13 completed, now fuzzing with 2 workers",
			wantOK:   true,
			wantKind: EventBaselineDone,
			wantSec:  0,
		},
		{
			name:      "instrumented_stats_multiunit",
			line:      "fuzz: elapsed: 1m33s, execs: 20801 (6626/sec), new interesting: 4 (total: 946)",
			wantOK:    true,
			wantKind:  EventStats,
			wantSec:   93,
			wantExecs: 20801,
			wantHasNI: true,
			wantNI:    4,
			wantTotal: 946,
		},
		{
			name:      "uninstrumented_stats",
			line:      "fuzz: elapsed: 3s, execs: 267413 (89125/sec)",
			wantOK:    true,
			wantKind:  EventStats,
			wantSec:   3,
			wantExecs: 267413,
			wantHasNI: false,
		},
		{
			name:     "disarm_minimizing",
			line:     "fuzz: elapsed: 12s, minimizing",
			wantOK:   true,
			wantKind: EventDisarm,
		},
		{
			name:     "disarm_minimizing_bytes",
			line:     "fuzz: minimizing 128-byte failing input file",
			wantOK:   true,
			wantKind: EventDisarm,
		},
		{
			name:     "disarm_fail_line",
			line:     "    --- FAIL: FuzzMILPSolver/seed#0 (0.04s)",
			wantOK:   true,
			wantKind: EventDisarm,
		},
		{
			name:     "disarm_panic",
			line:     "panic: runtime error: index out of range",
			wantOK:   true,
			wantKind: EventDisarm,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, ok := ParseProgressLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ParseProgressLine(%q) ok=%v, want %v", tc.line, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if ev.Kind != tc.wantKind {
				t.Fatalf("kind=%v, want %v", ev.Kind, tc.wantKind)
			}
			if ev.ElapsedSec != tc.wantSec {
				t.Fatalf("elapsedSec=%v, want %v", ev.ElapsedSec, tc.wantSec)
			}
			if ev.Execs != tc.wantExecs {
				t.Fatalf("execs=%d, want %d", ev.Execs, tc.wantExecs)
			}
			if ev.HasNewInteresting != tc.wantHasNI || ev.NewInteresting != tc.wantNI || ev.TotalCorpus != tc.wantTotal {
				t.Fatalf("newInteresting=(%v,%d,%d), want (%v,%d,%d)",
					ev.HasNewInteresting, ev.NewInteresting, ev.TotalCorpus,
					tc.wantHasNI, tc.wantNI, tc.wantTotal)
			}
		})
	}
}

func TestResolveConfigAndOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := ResolveConfig(PresetSoak, ClassHeavy, ConfigOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig(soak, heavy) unexpected error: %v", err)
	}
	if cfg.StallTime != 30*time.Minute || cfg.StallExecs != 150_000 {
		t.Fatalf("unexpected soak/heavy plateau defaults: %+v", cfg)
	}

	cfg, err = ResolveConfig(PresetLocal, ClassFast, ConfigOverrides{
		WindowMinNew: new(int64(42)),
		StallTime:    new(time.Duration(0)),
	})
	if err != nil {
		t.Fatalf("ResolveConfig override error: %v", err)
	}
	if cfg.StallTime != 0 || cfg.WindowMinNew != 42 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}

	offCfg, err := ResolveConfig(PresetOff, ClassParser, ConfigOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig(off) error: %v", err)
	}
	if offCfg.StallTime != 0 || offCfg.MinTime != 0 {
		t.Fatalf("expected zeroed off config, got %+v", offCfg)
	}

	if _, err := ResolveConfig("bogus", ClassFast, ConfigOverrides{}); err == nil {
		t.Fatal("expected error for unknown preset")
	}
	if _, err := ResolveConfig(PresetSoak, "bogus", ConfigOverrides{}); err == nil {
		t.Fatal("expected error for unknown class")
	}
}

func TestResolveConfigClassRace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		preset     string
		minTime    time.Duration
		minExecs   int64
		stallTime  time.Duration
		stallExecs int64
		winTime    time.Duration
		winExecs   int64
	}{
		{
			preset:     PresetLocal,
			minTime:    5 * time.Minute,
			minExecs:   1_000,
			stallTime:  10 * time.Minute,
			stallExecs: 5_000,
			winTime:    15 * time.Minute,
			winExecs:   10_000,
		},
		{
			preset:     PresetSoak,
			minTime:    15 * time.Minute,
			minExecs:   2_500,
			stallTime:  30 * time.Minute,
			stallExecs: 15_000,
			winTime:    45 * time.Minute,
			winExecs:   25_000,
		},
		{
			preset:     PresetDeep,
			minTime:    25 * time.Minute,
			minExecs:   5_000,
			stallTime:  50 * time.Minute,
			stallExecs: 30_000,
			winTime:    75 * time.Minute,
			winExecs:   50_000,
		},
	}

	for _, tc := range cases {
		cfg, err := ResolveConfig(tc.preset, ClassRace, ConfigOverrides{})
		if err != nil {
			t.Fatalf("ResolveConfig(%s, race): %v", tc.preset, err)
		}
		if cfg.Class != ClassRace ||
			cfg.MinTime != tc.minTime || cfg.MinExecs != tc.minExecs ||
			cfg.StallTime != tc.stallTime || cfg.StallExecs != tc.stallExecs ||
			cfg.WindowTime != tc.winTime || cfg.WindowExecs != tc.winExecs ||
			cfg.WindowMinNew != defaultWindowMinNew {
			t.Fatalf("unexpected (%s, race) config: %+v", tc.preset, cfg)
		}
	}

	offCfg, err := ResolveConfig(PresetOff, ClassRace, ConfigOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig(off, race): %v", err)
	}
	if offCfg.Class != ClassRace || offCfg.Preset != PresetOff || offCfg.StallTime != 0 {
		t.Fatalf("unexpected (off, race) config: %+v", offCfg)
	}
}

func TestWatcherRaceClassPlateauAndTrickle(t *testing.T) {
	t.Parallel()

	cfg := mustResolve(t, PresetLocal, ClassRace, ConfigOverrides{})
	w := NewWatcher(cfg, "FuzzParallelBABRace", nil)
	w.ObserveLine("fuzz: elapsed: 5s, gathering baseline coverage: 20/20 completed, now fuzzing with 4 workers")

	// Hold below plateau.
	if w.ObserveLine("fuzz: elapsed: 11m5s, execs: 4500 (7/sec), new interesting: 0 (total: 20)") {
		t.Fatal("race class triggered plateau before StallExecs=5000")
	}
	// Trigger plateau now.
	if !w.ObserveLine("fuzz: elapsed: 12m5s, execs: 5200 (7/sec), new interesting: 0 (total: 20)") {
		t.Fatal("expected race class plateau once both 10m and 5000 execs reached")
	}
	ok, reason, _ := w.Triggered()
	if !ok || reason != StopReasonSaturatedPlateau {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", ok, reason, StopReasonSaturatedPlateau)
	}

	// Verify trickle gate.
	wTrickle := NewWatcher(cfg, "FuzzParallelBABRace", nil)
	wTrickle.ObserveLine("fuzz: elapsed: 0s, gathering baseline coverage: 20/20 completed, now fuzzing with 4 workers")
	wTrickle.ObserveLine("fuzz: elapsed: 1m, execs: 800 (13/sec), new interesting: 3 (total: 23)")
	wTrickle.ObserveLine("fuzz: elapsed: 8m, execs: 5000 (10/sec), new interesting: 4 (total: 24)")
	if wTrickle.ObserveLine("fuzz: elapsed: 15m, execs: 9500 (10/sec), new interesting: 4 (total: 24)") {
		t.Fatal("race class triggered trickle before WindowTime=15m and WindowExecs=10000")
	}
	if !wTrickle.ObserveLine("fuzz: elapsed: 16m30s, execs: 11000 (11/sec), new interesting: 4 (total: 24)") {
		t.Fatal("expected race class trickle once WindowTime=15m and WindowExecs=10000 met")
	}
	okTrickle, reasonTrickle, _ := wTrickle.Triggered()
	if !okTrickle || reasonTrickle != StopReasonSaturatedTrickle {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", okTrickle, reasonTrickle, StopReasonSaturatedTrickle)
	}
}

func mustResolve(tb testing.TB, preset, class string, ov ConfigOverrides) Config {
	tb.Helper()
	cfg, err := ResolveConfig(preset, class, ov)
	if err != nil {
		tb.Fatalf("ResolveConfig(%s, %s): %v", preset, class, err)
	}
	return cfg
}

func TestWatcherPlateauAndReset(t *testing.T) {
	t.Parallel()

	cfg := mustResolve(t, PresetLocal, ClassHeavy, ConfigOverrides{
		MinTime:      new(20 * time.Second),
		MinExecs:     new(int64(200)),
		StallTime:    new(30 * time.Second),
		StallExecs:   new(int64(300)),
		WindowMinNew: new(int64(0)),
	})
	w := NewWatcher(cfg, "FuzzMILPSolver", nil)

	// Baseline takes 10s; timers must measure post-baseline fuzzing time.
	if w.ObserveLine("fuzz: elapsed: 10s, gathering baseline coverage: 50/50 completed, now fuzzing with 4 workers") {
		t.Fatal("should not stop on baseline completion")
	}
	// 25s total (15s fuzzing), 500 execs -> warmup MinTime (20s) not met yet.
	if w.ObserveLine("fuzz: elapsed: 25s, execs: 500 (33/sec), new interesting: 0 (total: 50)") {
		t.Fatal("stopped before warmup MinTime")
	}

	w2 := NewWatcher(cfg, "FuzzMILPSolver", nil)
	w2.ObserveLine("fuzz: elapsed: 10s, gathering baseline coverage: 50/50 completed, now fuzzing with 4 workers")
	if w2.ObserveLine("fuzz: elapsed: 35s, execs: 400 (16/sec), new interesting: 1 (total: 51)") {
		t.Fatal("unexpected stop right after discovering new interesting path")
	}
	// At 50s total (40s fuzzing, idle=15s), 800 execs (idleExecs=400): idleSec < 30s -> must NOT stop.
	if w2.ObserveLine("fuzz: elapsed: 50s, execs: 800 (26/sec), new interesting: 1 (total: 51)") {
		t.Fatal("stopped before plateau StallTime after reset")
	}
	// At 65s total (55s fuzzing, idle=30s), 900 execs (idleExecs=500) -> plateau triggers.
	if !w2.ObserveLine("fuzz: elapsed: 65s, execs: 900 (15/sec), new interesting: 1 (total: 51)") {
		t.Fatal("expected plateau trigger at 65s")
	}
	ok, reason, _ := w2.Triggered()
	if !ok || reason != StopReasonSaturatedPlateau {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", ok, reason, StopReasonSaturatedPlateau)
	}
}

func TestWatcherRequiresBothTimeAndExecs(t *testing.T) {
	t.Parallel()

	cfg := mustResolve(t, PresetLocal, ClassFast, ConfigOverrides{
		MinTime:      new(10 * time.Second),
		MinExecs:     new(int64(1_000)),
		StallTime:    new(20 * time.Second),
		StallExecs:   new(int64(5_000)),
		WindowMinNew: new(int64(0)),
	})
	w := NewWatcher(cfg, "FuzzLinearSum", nil)
	w.ObserveLine("fuzz: elapsed: 0s, gathering baseline coverage: 10/10 completed, now fuzzing with 2 workers")

	// Time met (30s >= 20s) but execs too low (2000 < 5000).
	if w.ObserveLine("fuzz: elapsed: 30s, execs: 2000 (66/sec), new interesting: 0 (total: 10)") {
		t.Fatal("stopped even though execs < StallExecs")
	}
	// Both met (33s >= 20s and 5500 >= 5000).
	if !w.ObserveLine("fuzz: elapsed: 33s, execs: 5500 (1166/sec), new interesting: 0 (total: 10)") {
		t.Fatal("expected plateau once both thresholds met")
	}
	ok, reason, _ := w.Triggered()
	if !ok || reason != StopReasonSaturatedPlateau {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", ok, reason, StopReasonSaturatedPlateau)
	}
}

func TestWatcherTrickleWindow(t *testing.T) {
	t.Parallel()

	cfg := mustResolve(t, PresetLocal, ClassParser, ConfigOverrides{
		MinTime:      new(20 * time.Second),
		MinExecs:     new(int64(1_000)),
		StallTime:    new(1 * time.Hour),
		StallExecs:   new(int64(100_000_000)),
		WindowTime:   new(60 * time.Second),
		WindowExecs:  new(int64(5_000)),
		WindowMinNew: new(int64(3)),
	})
	w := NewWatcher(cfg, "FuzzPuLPJSON", nil)
	w.ObserveLine("fuzz: elapsed: 0s, gathering baseline coverage: 5/5 completed, now fuzzing with 2 workers")

	// Burst of 6 new paths across 10s..20s keeps trickle above threshold until window passes them.
	w.ObserveLine("fuzz: elapsed: 10s, execs: 1000 (100/sec), new interesting: 4 (total: 9)")
	w.ObserveLine("fuzz: elapsed: 20s, execs: 2000 (100/sec), new interesting: 6 (total: 11)")
	if w.ObserveLine("fuzz: elapsed: 60s, execs: 6000 (100/sec), new interesting: 6 (total: 11)") {
		t.Fatal("trickle fired before WindowTime elapsed past early burst")
	}
	// At 75s, newest qualifying window starts at 10s (65s, 6500 execs) with 7-4=3 new paths -> not trickle.
	if w.ObserveLine("fuzz: elapsed: 75s, execs: 7500 (100/sec), new interesting: 7 (total: 12)") {
		t.Fatal("trickle fired when window still held 3 new paths")
	}
	// At 95s, newest qualifying window starts at 20s (75s, 7500 execs) with 7-6=1 (< 3) new paths -> triggers.
	if !w.ObserveLine("fuzz: elapsed: 95s, execs: 9500 (100/sec), new interesting: 7 (total: 12)") {
		t.Fatal("expected saturated_trickle at 95s")
	}
	ok, reason, _ := w.Triggered()
	if !ok || reason != StopReasonSaturatedTrickle {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", ok, reason, StopReasonSaturatedTrickle)
	}
}

func TestWatcherDisarmAndUninstrumentedSafety(t *testing.T) {
	t.Parallel()

	cfg := mustResolve(t, PresetLocal, ClassFast, ConfigOverrides{
		MinTime:    new(5 * time.Second),
		MinExecs:   new(int64(10)),
		StallTime:  new(5 * time.Second),
		StallExecs: new(int64(10)),
	})

	// Uninstrumented output without "new interesting:" must never trigger saturation.
	wUninst := NewWatcher(cfg, "FuzzLinearSum", nil)
	wUninst.ObserveLine("fuzz: elapsed: 0s, testing seed corpus: 5/5 completed, now fuzzing with 2 workers")
	if wUninst.ObserveLine("fuzz: elapsed: 30s, execs: 50000 (1666/sec)") {
		t.Fatal("must not stop on uninstrumented output lacking new interesting")
	}

	// Minimizing crasher must permanently disarm early stop.
	wCrash := NewWatcher(cfg, "FuzzLinearSum", nil)
	wCrash.ObserveLine("fuzz: elapsed: 0s, gathering baseline coverage: 5/5 completed, now fuzzing with 2 workers")
	wCrash.ObserveLine("fuzz: minimizing 32-byte failing input file")
	if !wCrash.Disarmed() {
		t.Fatal("expected Disarmed() after minimizing line")
	}
	if wCrash.ObserveLine("fuzz: elapsed: 30s, execs: 50000 (1666/sec), new interesting: 0 (total: 5)") {
		t.Fatal("must not stop once disarmed")
	}
}

func TestWatcherCrossSlicePersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	cfg := mustResolve(t, PresetSoak, ClassHeavy, ConfigOverrides{
		MinTime:      new(20 * time.Minute),
		MinExecs:     new(int64(20_000)),
		StallTime:    new(20 * time.Minute),
		StallExecs:   new(int64(20_000)),
		WindowMinNew: new(int64(0)),
	})

	// Slice 1 runs 15m (below 20m plateau) with 15,000 execs and 2 early new paths at 2m.
	w1 := NewWatcher(cfg, "FuzzMILPSolver", nil)
	w1.ObserveLine("fuzz: elapsed: 10s, gathering baseline coverage: 40/40 completed, now fuzzing with 8 workers")
	w1.ObserveLine("fuzz: elapsed: 2m10s, execs: 2000 (16/sec), new interesting: 2 (total: 42)")
	if w1.ObserveLine("fuzz: elapsed: 15m10s, execs: 15000 (16/sec), new interesting: 2 (total: 42)") {
		t.Fatal("slice 1 stopped prematurely at 15m")
	}
	st1 := w1.SnapshotState(StopReasonCompleted, "")
	if err := SaveState(statePath, st1); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, found, err := LoadState(statePath)
	if err != nil || !found {
		t.Fatalf("LoadState found=%v err=%v", found, err)
	}
	if loaded.Slices != 1 || loaded.CumExecs != 15000 || loaded.CumNewInteresting != 2 {
		t.Fatalf("unexpected persisted slice 1 state: %+v", loaded)
	}

	// Slice 2 runs 8m more with 9,000 execs -> idle=13m+8m=21m >= 20m, idleExecs=13000+9000=22000 >= 20000.
	w2 := NewWatcher(cfg, "FuzzMILPSolver", &loaded)
	w2.ObserveLine("fuzz: elapsed: 5s, gathering baseline coverage: 42/42 completed, now fuzzing with 8 workers")
	if w2.ObserveLine("fuzz: elapsed: 5m5s, execs: 5000 (16/sec), new interesting: 0 (total: 42)") {
		t.Fatal("slice 2 stopped too early at 18m cumulative idle")
	}
	if !w2.ObserveLine("fuzz: elapsed: 8m5s, execs: 9000 (18/sec), new interesting: 0 (total: 42)") {
		t.Fatal("expected cross-slice plateau in slice 2")
	}
	ok, reason, _ := w2.Triggered()
	if !ok || reason != StopReasonSaturatedPlateau {
		t.Fatalf("Triggered()=(%v, %q), want (true, %q)", ok, reason, StopReasonSaturatedPlateau)
	}
}

func TestRunWatchSubprocessIntegration(t *testing.T) {
	if os.Getenv("GO_WANT_FUZZWATCH_HELPER") == "1" {
		runHelperFuzzProcess(false)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	statePath := filepath.Join(dir, "state.json")

	opts, err := parseCLI([]string{
		"--target=FuzzHelperTarget",
		"--preset=local",
		"--class=fast",
		"--min-time=2s",
		"--min-execs=100",
		"--stall-time=2s",
		"--stall-execs=100",
		"--window-min-new=0",
		"--log-file=" + logPath,
		"--state-file=" + statePath,
		"--",
		exe,
		"-test.run=^TestRunWatchSubprocessIntegration$",
	})
	if err != nil {
		t.Fatalf("parseCLI: %v", err)
	}

	t.Setenv("GO_WANT_FUZZWATCH_HELPER", "1")
	code := runWatch(t.Context(), opts)
	if code != 0 {
		logBytes, _ := os.ReadFile(logPath)
		t.Fatalf("runWatch exited %d, log:\n%s", code, string(logBytes))
	}

	st, found, err := LoadState(statePath)
	if err != nil || !found {
		t.Fatalf("LoadState found=%v err=%v", found, err)
	}
	if st.StopReason != StopReasonSaturatedPlateau {
		t.Fatalf("StopReason=%q, want %q (detail=%q)", st.StopReason, StopReasonSaturatedPlateau, st.StopDetail)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logRaw), "\nPASS\n") {
		t.Fatalf("expected PASS in log, got:\n%s", string(logRaw))
	}
}

func TestRunWatchCrashPreservesFailure(t *testing.T) {
	if os.Getenv("GO_WANT_FUZZWATCH_HELPER_CRASH") == "1" {
		runHelperFuzzProcess(true)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "crash.log")
	statePath := filepath.Join(dir, "crash_state.json")

	opts, err := parseCLI([]string{
		"--target=FuzzHelperCrashTarget",
		"--preset=local",
		"--class=fast",
		"--min-time=1s",
		"--min-execs=10",
		"--stall-time=1s",
		"--stall-execs=10",
		"--window-min-new=0",
		"--log-file=" + logPath,
		"--state-file=" + statePath,
		"--",
		exe,
		"-test.run=^TestRunWatchCrashPreservesFailure$",
	})
	if err != nil {
		t.Fatalf("parseCLI: %v", err)
	}

	t.Setenv("GO_WANT_FUZZWATCH_HELPER_CRASH", "1")
	code := runWatch(t.Context(), opts)
	if code == 0 {
		t.Fatal("expected non-zero exit code when child crashes")
	}

	st, found, err := LoadState(statePath)
	if err != nil || !found {
		t.Fatalf("LoadState found=%v err=%v", found, err)
	}
	if st.StopReason != StopReasonFailed {
		t.Fatalf("StopReason=%q, want %q", st.StopReason, StopReasonFailed)
	}
}

func runHelperFuzzProcess(crash bool) {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("fuzz: elapsed: 0s, gathering baseline coverage: 1/1 completed, now fuzzing with 1 workers")
	if crash {
		fmt.Println("fuzz: elapsed: 0s, minimizing")
		fmt.Println("fuzz: elapsed: 5s, execs: 500 (100/sec), new interesting: 0 (total: 1)")
		fmt.Println("--- FAIL: FuzzHelperCrashTarget (0.01s)")
		fmt.Println("FAIL")
		os.Exit(1)
	}

	lines := []string{
		"fuzz: elapsed: 1s, execs: 150 (150/sec), new interesting: 0 (total: 1)",
		"fuzz: elapsed: 3s, execs: 450 (150/sec), new interesting: 0 (total: 1)",
		"fuzz: elapsed: 6s, execs: 900 (150/sec), new interesting: 0 (total: 1)",
	}
	for _, ln := range lines {
		fmt.Println(ln)
		select {
		case <-sigC:
			fmt.Println("fuzz: elapsed: 3s, execs: 450 (0/sec), new interesting: 0 (total: 1)")
			fmt.Println("PASS")
			os.Exit(0)
		case <-time.After(15 * time.Millisecond):
		}
	}
	select {
	case <-sigC:
		fmt.Println("PASS")
		os.Exit(0)
	case <-time.After(3 * time.Second):
		fmt.Fprintln(os.Stderr, "helper timed out waiting for SIGINT")
		os.Exit(2)
	}
}
