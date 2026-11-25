// Package main implements goblin-fuzzwatch, a supervisor that monitors
// coverage-instrumented Go fuzz test binaries and sends SIGINT when both
// elapsed post-baseline time and cumulative execution thresholds confirm saturation.
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// PresetOff disables early stopping.
	PresetOff = "off"
	// PresetLocal configures developer soak thresholds.
	PresetLocal = "local"
	// PresetSoak selects high-bar thresholds designed for up-to-4h Cloud Batch runs across diverse machine types.
	PresetSoak = "soak"
	// PresetDeep selects extended release thresholds.
	PresetDeep = "deep"

	// ClassParser marks fast grammar fuzzers.
	ClassParser = "parser"
	// ClassFast marks high-throughput numerical targets.
	ClassFast = "fast"
	// ClassHeavy marks MILP, B&C, CBC, and Lean-oracle targets where single iterations take milliseconds to seconds.
	ClassHeavy = "heavy"
	// ClassRace marks targets compiled with the Go race detector (-race) at roughly 10x lower execution throughput than ClassHeavy.
	ClassRace = "race"

	// StopReasonRunning marks an active slice.
	StopReasonRunning = "running"
	// StopReasonCompleted indicates the target exhausted its configured -fuzztime budget without triggering saturation.
	StopReasonCompleted = "completed"
	// StopReasonSaturatedPlateau records a plateau stop.
	StopReasonSaturatedPlateau = "saturated_plateau"
	// StopReasonSaturatedTrickle records a trickle stop.
	StopReasonSaturatedTrickle = "saturated_trickle"
	// StopReasonFailed records a non-zero exit.
	StopReasonFailed = "failed"
	// StopReasonInterrupted records an external signal.
	StopReasonInterrupted = "interrupted"

	sampleIntervalSec   = 15.0
	minRetainWindowSec  = 600.0
	stateFilePerm       = 0o600
	defaultWindowMinNew = 3
)

// Config defines warmup, plateau, and sliding-window thresholds for one target class and preset,
// requiring both elapsed fuzzing time and cumulative execution counts before early stopping can fire.
type Config struct {
	Preset       string
	Class        string
	MinTime      time.Duration
	MinExecs     int64
	StallTime    time.Duration
	StallExecs   int64
	WindowTime   time.Duration
	WindowExecs  int64
	WindowMinNew int64
}

// ConfigOverrides specifies optional flag-level threshold overrides.
type ConfigOverrides struct {
	MinTime      *time.Duration
	MinExecs     *int64
	StallTime    *time.Duration
	StallExecs   *int64
	WindowTime   *time.Duration
	WindowExecs  *int64
	WindowMinNew *int64
}

type presetKey struct {
	preset string
	class  string
}

var presetDefaults = map[presetKey]Config{
	{PresetLocal, ClassParser}: {
		Preset:       PresetLocal,
		Class:        ClassParser,
		MinTime:      90 * time.Second,
		MinExecs:     75_000,
		StallTime:    3 * time.Minute,
		StallExecs:   150_000,
		WindowTime:   5 * time.Minute,
		WindowExecs:  300_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetLocal, ClassFast}: {
		Preset:       PresetLocal,
		Class:        ClassFast,
		MinTime:      3 * time.Minute,
		MinExecs:     150_000,
		StallTime:    6 * time.Minute,
		StallExecs:   500_000,
		WindowTime:   10 * time.Minute,
		WindowExecs:  1_000_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetLocal, ClassHeavy}: {
		Preset:       PresetLocal,
		Class:        ClassHeavy,
		MinTime:      5 * time.Minute,
		MinExecs:     8_000,
		StallTime:    10 * time.Minute,
		StallExecs:   40_000,
		WindowTime:   15 * time.Minute,
		WindowExecs:  75_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetLocal, ClassRace}: {
		Preset:       PresetLocal,
		Class:        ClassRace,
		MinTime:      5 * time.Minute,
		MinExecs:     1_000,
		StallTime:    10 * time.Minute,
		StallExecs:   5_000,
		WindowTime:   15 * time.Minute,
		WindowExecs:  10_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetSoak, ClassParser}: {
		Preset:       PresetSoak,
		Class:        ClassParser,
		MinTime:      5 * time.Minute,
		MinExecs:     250_000,
		StallTime:    10 * time.Minute,
		StallExecs:   500_000,
		WindowTime:   15 * time.Minute,
		WindowExecs:  1_000_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetSoak, ClassFast}: {
		Preset:       PresetSoak,
		Class:        ClassFast,
		MinTime:      10 * time.Minute,
		MinExecs:     500_000,
		StallTime:    20 * time.Minute,
		StallExecs:   2_000_000,
		WindowTime:   30 * time.Minute,
		WindowExecs:  3_000_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetSoak, ClassHeavy}: {
		Preset:       PresetSoak,
		Class:        ClassHeavy,
		MinTime:      15 * time.Minute,
		MinExecs:     25_000,
		StallTime:    30 * time.Minute,
		StallExecs:   150_000,
		WindowTime:   45 * time.Minute,
		WindowExecs:  250_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetSoak, ClassRace}: {
		Preset:       PresetSoak,
		Class:        ClassRace,
		MinTime:      15 * time.Minute,
		MinExecs:     2_500,
		StallTime:    30 * time.Minute,
		StallExecs:   15_000,
		WindowTime:   45 * time.Minute,
		WindowExecs:  25_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetDeep, ClassParser}: {
		Preset:       PresetDeep,
		Class:        ClassParser,
		MinTime:      10 * time.Minute,
		MinExecs:     500_000,
		StallTime:    15 * time.Minute,
		StallExecs:   1_000_000,
		WindowTime:   25 * time.Minute,
		WindowExecs:  2_000_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetDeep, ClassFast}: {
		Preset:       PresetDeep,
		Class:        ClassFast,
		MinTime:      15 * time.Minute,
		MinExecs:     1_000_000,
		StallTime:    35 * time.Minute,
		StallExecs:   4_000_000,
		WindowTime:   50 * time.Minute,
		WindowExecs:  6_000_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetDeep, ClassHeavy}: {
		Preset:       PresetDeep,
		Class:        ClassHeavy,
		MinTime:      25 * time.Minute,
		MinExecs:     50_000,
		StallTime:    50 * time.Minute,
		StallExecs:   300_000,
		WindowTime:   75 * time.Minute,
		WindowExecs:  500_000,
		WindowMinNew: defaultWindowMinNew,
	},
	{PresetDeep, ClassRace}: {
		Preset:       PresetDeep,
		Class:        ClassRace,
		MinTime:      25 * time.Minute,
		MinExecs:     5_000,
		StallTime:    50 * time.Minute,
		StallExecs:   30_000,
		WindowTime:   75 * time.Minute,
		WindowExecs:  50_000,
		WindowMinNew: defaultWindowMinNew,
	},
}

func isValidClass(class string) bool {
	return class == ClassParser || class == ClassFast || class == ClassHeavy || class == ClassRace
}

// ResolveConfig builds a validated Config from preset, class, and CLI overrides.
func ResolveConfig(preset, class string, ov ConfigOverrides) (Config, error) {
	if !isValidClass(class) {
		return Config{}, fmt.Errorf("unknown target class %q (expected parser, fast, heavy, or race)", class)
	}
	if preset == PresetOff {
		return Config{Preset: PresetOff, Class: class}, nil
	}
	cfg, ok := presetDefaults[presetKey{preset: preset, class: class}]
	if !ok {
		return Config{}, fmt.Errorf("unknown preset %q (expected off, local, soak, or deep)", preset)
	}
	if ov.MinTime != nil {
		cfg.MinTime = *ov.MinTime
	}
	if ov.MinExecs != nil {
		cfg.MinExecs = *ov.MinExecs
	}
	if ov.StallTime != nil {
		cfg.StallTime = *ov.StallTime
	}
	if ov.StallExecs != nil {
		cfg.StallExecs = *ov.StallExecs
	}
	if ov.WindowTime != nil {
		cfg.WindowTime = *ov.WindowTime
	}
	if ov.WindowExecs != nil {
		cfg.WindowExecs = *ov.WindowExecs
	}
	if ov.WindowMinNew != nil {
		cfg.WindowMinNew = *ov.WindowMinNew
	}
	if cfg.MinTime < 0 || cfg.MinExecs < 0 || cfg.StallTime < 0 || cfg.StallExecs < 0 ||
		cfg.WindowTime < 0 || cfg.WindowExecs < 0 || cfg.WindowMinNew < 0 {
		return Config{}, errors.New("early-stop thresholds must be non-negative")
	}
	return cfg, nil
}

// Sample captures a cumulative progress checkpoint for sliding-window rate checks.
type Sample struct {
	CumSec   float64 `json:"cum_sec"`
	CumExecs int64   `json:"cum_execs"`
	CumNew   int64   `json:"cum_new"`
}

// State persists cumulative fuzzing progress and saturation telemetry across slices.
type State struct {
	Target            string   `json:"target"`
	Preset            string   `json:"preset"`
	Class             string   `json:"class"`
	Slices            int      `json:"slices"`
	CumFuzzSec        float64  `json:"cum_fuzz_sec"`
	CumExecs          int64    `json:"cum_execs"`
	CumNewInteresting int64    `json:"cum_new_interesting"`
	TotalCorpus       int64    `json:"total_corpus"`
	LastNewSec        float64  `json:"last_new_sec"`
	LastNewExecs      int64    `json:"last_new_execs"`
	StopReason        string   `json:"stop_reason"`
	StopDetail        string   `json:"stop_detail,omitempty"`
	Samples           []Sample `json:"samples,omitempty"`
}

// LoadState reads a persisted State JSON file, returning found=false when absent.
func LoadState(path string) (State, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, false, nil
		}
		return State{}, false, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, false, fmt.Errorf("parse state %s: %w", path, err)
	}
	return st, true, nil
}

// SaveState writes st atomically to path using a temporary file and rename.
func SaveState(path string, st State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(&st, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(dir, ".fuzzwatch-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(stateFilePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// EventKind classifies a parsed line from Go's fuzz coordinator output.
type EventKind uint8

const (
	// EventIgnore marks non-progress or baseline-in-progress lines.
	EventIgnore EventKind = iota
	// EventBaselineDone marks the transition from baseline coverage to active fuzzing.
	EventBaselineDone
	// EventStats carries a periodic fuzz execution and coverage update.
	EventStats
	// EventDisarm signals crash minimization or failure, disabling early stop.
	EventDisarm
)

// ProgressEvent holds parsed fields from a single coordinator log line.
type ProgressEvent struct {
	Kind              EventKind
	ElapsedSec        float64
	Execs             int64
	HasNewInteresting bool
	NewInteresting    int64
	TotalCorpus       int64
}

// ParseProgressLine parses one stdout/stderr line produced by Go's fuzz runtime.
func ParseProgressLine(line string) (ProgressEvent, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ProgressEvent{}, false
	}
	if isDisarmLine(trimmed) {
		return ProgressEvent{Kind: EventDisarm}, true
	}
	const prefix = "fuzz: elapsed: "
	rest, ok := strings.CutPrefix(trimmed, prefix)
	if !ok {
		return ProgressEvent{}, false
	}
	durStr, afterDur, ok := strings.Cut(rest, ",")
	if !ok {
		return ProgressEvent{}, false
	}
	dur, err := time.ParseDuration(strings.TrimSpace(durStr))
	if err != nil {
		return ProgressEvent{}, false
	}
	elapsedSec := dur.Seconds()
	if strings.Contains(afterDur, "now fuzzing") {
		return ProgressEvent{
			Kind:       EventBaselineDone,
			ElapsedSec: elapsedSec,
		}, true
	}
	execs, ok := parseLabeledInt64(afterDur, "execs: ")
	if !ok {
		return ProgressEvent{}, false
	}
	ev := ProgressEvent{
		Kind:       EventStats,
		ElapsedSec: elapsedSec,
		Execs:      execs,
	}
	if newInt, hasNew := parseLabeledInt64(afterDur, "new interesting: "); hasNew {
		ev.HasNewInteresting = true
		ev.NewInteresting = newInt
		if total, hasTotal := parseLabeledInt64(afterDur, "total: "); hasTotal {
			ev.TotalCorpus = total
		}
	}
	return ev, true
}

func isDisarmLine(trimmed string) bool {
	if trimmed == "FAIL" || strings.Contains(trimmed, "--- FAIL:") || strings.Contains(trimmed, "panic:") {
		return true
	}
	return strings.HasPrefix(trimmed, "fuzz: ") && strings.Contains(trimmed, "minimizing")
}

func parseLabeledInt64(s, label string) (int64, bool) {
	_, tail, ok := strings.Cut(s, label)
	if !ok {
		return 0, false
	}
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	val, err := strconv.ParseInt(tail[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// Watcher evaluates live fuzz progress across slices against Config saturation rules.
type Watcher struct {
	cfg               Config
	target            string
	base              State
	sliceBaselineSet  bool
	sliceBaselineSec  float64
	sliceFuzzSec      float64
	sliceExecs        int64
	sliceNew          int64
	totalCorpus       int64
	lastNewSec        float64
	lastNewExecs      int64
	samples           []Sample
	disarmed          bool
	triggered         bool
	triggerReason     string
	triggerDetail     string
	sawNewInteresting bool
}

// NewWatcher constructs a Watcher for target, resuming prior cumulative state when target matches.
func NewWatcher(cfg Config, target string, prior *State) *Watcher {
	w := &Watcher{
		cfg:     cfg,
		target:  target,
		samples: []Sample{{CumSec: 0, CumExecs: 0, CumNew: 0}},
	}
	if prior != nil && (prior.Target == "" || prior.Target == target) {
		w.base = *prior
		w.totalCorpus = prior.TotalCorpus
		w.lastNewSec = prior.LastNewSec
		w.lastNewExecs = prior.LastNewExecs
		if len(prior.Samples) > 0 {
			w.samples = append([]Sample(nil), prior.Samples...)
		} else {
			w.samples = []Sample{{
				CumSec:   prior.CumFuzzSec,
				CumExecs: prior.CumExecs,
				CumNew:   prior.CumNewInteresting,
			}}
		}
	}
	return w
}

// ObserveLine updates cumulative state from one output line and reports whether early stop triggered.
func (w *Watcher) ObserveLine(line string) bool {
	ev, ok := ParseProgressLine(line)
	if !ok {
		return false
	}
	switch ev.Kind {
	case EventDisarm:
		w.disarmed = true
		w.triggered = false
		w.triggerReason = ""
		w.triggerDetail = ""
		return false
	case EventBaselineDone:
		if !w.sliceBaselineSet {
			w.sliceBaselineSet = true
			w.sliceBaselineSec = ev.ElapsedSec
		}
		return false
	case EventStats:
		return w.observeStats(ev)
	default:
		return false
	}
}

func (w *Watcher) observeStats(ev ProgressEvent) bool {
	if !w.sliceBaselineSet {
		w.sliceBaselineSet = true
		w.sliceBaselineSec = 0
	}
	fuzzSec := max(0.0, ev.ElapsedSec-w.sliceBaselineSec)
	w.sliceFuzzSec = max(w.sliceFuzzSec, fuzzSec)
	w.sliceExecs = max(w.sliceExecs, ev.Execs)

	if !ev.HasNewInteresting {
		return false
	}
	w.sawNewInteresting = true
	prevCumNew := w.base.CumNewInteresting + w.sliceNew
	w.sliceNew = max(w.sliceNew, ev.NewInteresting)
	if ev.TotalCorpus > w.totalCorpus {
		w.totalCorpus = ev.TotalCorpus
	}

	cumSec := w.base.CumFuzzSec + w.sliceFuzzSec
	cumExecs := w.base.CumExecs + w.sliceExecs
	cumNew := w.base.CumNewInteresting + w.sliceNew
	if cumNew > prevCumNew {
		w.lastNewSec = cumSec
		w.lastNewExecs = cumExecs
	}

	w.recordSample(Sample{CumSec: cumSec, CumExecs: cumExecs, CumNew: cumNew})

	if w.cfg.Preset == PresetOff || w.disarmed || w.triggered {
		return false
	}
	if cumSec < w.cfg.MinTime.Seconds() || cumExecs < w.cfg.MinExecs {
		return false
	}

	if w.cfg.StallTime > 0 || w.cfg.StallExecs > 0 {
		idleSec := cumSec - w.lastNewSec
		idleExecs := cumExecs - w.lastNewExecs
		if idleSec >= w.cfg.StallTime.Seconds() && idleExecs >= w.cfg.StallExecs {
			w.triggered = true
			w.triggerReason = StopReasonSaturatedPlateau
			w.triggerDetail = fmt.Sprintf("0 new interesting for %.0fs and %d execs", idleSec, idleExecs)
			return true
		}
	}

	if w.cfg.WindowMinNew > 0 && (w.cfg.WindowTime > 0 || w.cfg.WindowExecs > 0) {
		winSec := w.cfg.WindowTime.Seconds()
		for _, s := range slices.Backward(w.samples) {
			spanSec := cumSec - s.CumSec
			spanExecs := cumExecs - s.CumExecs
			if spanSec >= winSec && spanExecs >= w.cfg.WindowExecs {
				deltaNew := cumNew - s.CumNew
				if deltaNew < w.cfg.WindowMinNew {
					w.triggered = true
					w.triggerReason = StopReasonSaturatedTrickle
					w.triggerDetail = fmt.Sprintf(
						"%d new interesting (< %d) over %.0fs and %d execs",
						deltaNew, w.cfg.WindowMinNew, spanSec, spanExecs,
					)
					return true
				}
				break
			}
		}
	}
	return false
}

func (w *Watcher) recordSample(cur Sample) {
	n := len(w.samples)
	if n == 0 || cur.CumNew > w.samples[n-1].CumNew || cur.CumSec-w.samples[n-1].CumSec >= sampleIntervalSec {
		w.samples = append(w.samples, cur)
	}
	retainSec := max(2.0*w.cfg.WindowTime.Seconds(), minRetainWindowSec)
	cut := 0
	for cut+1 < len(w.samples) && cur.CumSec-w.samples[cut+1].CumSec > retainSec {
		cut++
	}
	if cut > 0 {
		w.samples = append([]Sample(nil), w.samples[cut:]...)
	}
}

// Disarmed reports whether crash minimization or failure disabled early stopping.
func (w *Watcher) Disarmed() bool {
	return w.disarmed
}

// Triggered reports whether saturation triggered along with its stop reason and detail.
func (w *Watcher) Triggered() (ok bool, reason, detail string) {
	return w.triggered && !w.disarmed, w.triggerReason, w.triggerDetail
}

// SawNewInteresting reports whether coverage-guided progress lines were observed.
func (w *Watcher) SawNewInteresting() bool {
	return w.sawNewInteresting
}

// SnapshotState returns the updated persistent State after this slice finishes.
func (w *Watcher) SnapshotState(finalReason, finalDetail string) State {
	return State{
		Target:            w.target,
		Preset:            w.cfg.Preset,
		Class:             w.cfg.Class,
		Slices:            w.base.Slices + 1,
		CumFuzzSec:        w.base.CumFuzzSec + w.sliceFuzzSec,
		CumExecs:          w.base.CumExecs + w.sliceExecs,
		CumNewInteresting: w.base.CumNewInteresting + w.sliceNew,
		TotalCorpus:       w.totalCorpus,
		LastNewSec:        w.lastNewSec,
		LastNewExecs:      w.lastNewExecs,
		StopReason:        finalReason,
		StopDetail:        finalDetail,
		Samples:           append([]Sample(nil), w.samples...),
	}
}
