package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultGracePeriod = 25 * time.Second
	termKillTimeout    = 5 * time.Second
	pipeDrainTimeout   = 2 * time.Second
	exitCodeBadUsage   = 2
	exitCodeSIGINT     = 130
	exitCodeSIGTERM    = 143
	logFilePerm        = 0o644
	readerBufSize      = 64 * 1024
)

type cliOptions struct {
	target       string
	preset       string
	class        string
	logFile      string
	appendLog    bool
	stateFile    string
	grace        time.Duration
	minTime      time.Duration
	minExecs     int64
	stallTime    time.Duration
	stallExecs   int64
	windowTime   time.Duration
	windowExecs  int64
	windowMinNew int64
	seenFlags    map[string]bool
	cmdArgs      []string
}

func parseCLI(args []string) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("goblin-fuzzwatch", flag.ContinueOnError)
	fs.StringVar(&opts.target, "target", "", "fuzz target name (required)")
	fs.StringVar(&opts.preset, "preset", PresetSoak, "early-stop preset: off, local, soak, or deep")
	fs.StringVar(&opts.class, "class", ClassHeavy, "target speed class: parser, fast, or heavy")
	fs.StringVar(&opts.logFile, "log-file", "", "path to child fuzz log output (default stdout)")
	fs.BoolVar(&opts.appendLog, "append-log", false, "append to --log-file instead of truncating")
	fs.StringVar(&opts.stateFile, "state-file", "", "optional JSON state file for cross-slice tracking")
	fs.DurationVar(&opts.grace, "grace", defaultGracePeriod, "grace period after SIGINT before SIGTERM")
	fs.DurationVar(&opts.minTime, "min-time", 0, "override warmup minimum fuzzing time")
	fs.Int64Var(&opts.minExecs, "min-execs", 0, "override warmup minimum executions")
	fs.DurationVar(&opts.stallTime, "stall-time", 0, "override plateau idle duration")
	fs.Int64Var(&opts.stallExecs, "stall-execs", 0, "override plateau idle executions")
	fs.DurationVar(&opts.windowTime, "window-time", 0, "override trickle sliding window duration")
	fs.Int64Var(&opts.windowExecs, "window-execs", 0, "override trickle sliding window executions")
	fs.Int64Var(&opts.windowMinNew, "window-min-new", 0, "override trickle minimum new-interesting count")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.seenFlags = make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		opts.seenFlags[f.Name] = true
	})
	opts.cmdArgs = fs.Args()
	if opts.target == "" {
		return opts, errors.New("--target is required")
	}
	if len(opts.cmdArgs) == 0 {
		return opts, errors.New("missing child test binary command after '--'")
	}
	if opts.grace <= 0 {
		return opts, errors.New("--grace must be positive")
	}
	return opts, nil
}

func buildOverrides(opts cliOptions) ConfigOverrides {
	var ov ConfigOverrides
	if opts.seenFlags["min-time"] {
		ov.MinTime = &opts.minTime
	}
	if opts.seenFlags["min-execs"] {
		ov.MinExecs = &opts.minExecs
	}
	if opts.seenFlags["stall-time"] {
		ov.StallTime = &opts.stallTime
	}
	if opts.seenFlags["stall-execs"] {
		ov.StallExecs = &opts.stallExecs
	}
	if opts.seenFlags["window-time"] {
		ov.WindowTime = &opts.windowTime
	}
	if opts.seenFlags["window-execs"] {
		ov.WindowExecs = &opts.windowExecs
	}
	if opts.seenFlags["window-min-new"] {
		ov.WindowMinNew = &opts.windowMinNew
	}
	return ov
}

func openLogWriter(path string, appendMode bool) (io.WriteCloser, error) {
	if path == "" {
		return nopWriteCloser{Writer: os.Stdout}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	return os.OpenFile(path, flags, logFilePerm)
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func readChildStream(pr io.Reader, logW io.Writer, w *Watcher, stopReq chan<- struct{}) error {
	br := bufio.NewReaderSize(pr, readerBufSize)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if _, werr := io.WriteString(logW, line); werr != nil {
				return werr
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if w.ObserveLine(trimmed) {
				select {
				case stopReq <- struct{}{}:
				default:
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func waitForExitAndDrain(
	cmd *exec.Cmd,
	grace time.Duration,
	waitCh <-chan error,
	readDone <-chan error,
	pr io.Closer,
	stopReq <-chan struct{},
	sigCh <-chan os.Signal,
) (waitErr error, extSignal os.Signal, readErr error) {
	sigSent := false
	var graceTimer *time.Timer
	var graceC <-chan time.Time
	var killTimer *time.Timer
	var killC <-chan time.Time

	defer func() {
		if graceTimer != nil {
			graceTimer.Stop()
		}
		if killTimer != nil {
			killTimer.Stop()
		}
	}()

	sendGracefulSIGINT := func() {
		if sigSent || cmd.Process == nil {
			return
		}
		sigSent = true
		_ = cmd.Process.Signal(syscall.SIGINT)
		graceTimer = time.NewTimer(grace)
		graceC = graceTimer.C
	}

waitLoop:
	for {
		select {
		case waitErr = <-waitCh:
			break waitLoop
		case s := <-sigCh:
			if extSignal == nil {
				extSignal = s
			}
			sendGracefulSIGINT()
		case <-stopReq:
			sendGracefulSIGINT()
		case <-graceC:
			graceC = nil
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			killTimer = time.NewTimer(termKillTimeout)
			killC = killTimer.C
		case <-killC:
			killC = nil
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}

	drainTimer := time.NewTimer(pipeDrainTimeout)
	defer drainTimer.Stop()
	select {
	case readErr = <-readDone:
	case <-drainTimer.C:
		_ = pr.Close()
		readErr = <-readDone
	}
	return waitErr, extSignal, readErr
}

func exitCodeFromWaitErr(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if code := exitErr.ExitCode(); code > 0 {
			return code
		}
	}
	return 1
}

func runWatch(ctx context.Context, opts cliOptions) int {
	cfg, err := ResolveConfig(opts.preset, opts.class, buildOverrides(opts))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] config error: %v\n", err)
		return exitCodeBadUsage
	}

	var priorPtr *State
	if opts.stateFile != "" {
		st, found, lerr := LoadState(opts.stateFile)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] state load warning (%s): %v\n", opts.stateFile, lerr)
		} else if found {
			priorPtr = &st
		}
	}

	w := NewWatcher(cfg, opts.target, priorPtr)

	logW, err := openLogWriter(opts.logFile, opts.appendLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] open log %s: %v\n", opts.logFile, err)
		return 1
	}
	defer func() {
		_ = logW.Close()
	}()

	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] create pipe: %v\n", err)
		return 1
	}
	defer func() {
		_ = pr.Close()
	}()

	cmd := exec.CommandContext(ctx, opts.cmdArgs[0], opts.cmdArgs[1:]...)
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] start child: %v\n", err)
		return 1
	}
	_ = pw.Close()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	stopReq := make(chan struct{}, 1)
	readDone := make(chan error, 1)
	go func() {
		readDone <- readChildStream(pr, logW, w, stopReq)
	}()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	waitErr, extSignal, readErr := waitForExitAndDrain(cmd, opts.grace, waitCh, readDone, pr, stopReq, sigCh)
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) {
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] stream read error: %v\n", readErr)
	}

	reason, detail, exitCode := classifyOutcome(w, waitErr, extSignal)
	if opts.stateFile != "" {
		st := w.SnapshotState(reason, detail)
		if serr := SaveState(opts.stateFile, st); serr != nil {
			fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] save state %s: %v\n", opts.stateFile, serr)
		}
	}
	if reason == StopReasonSaturatedPlateau || reason == StopReasonSaturatedTrickle {
		fmt.Fprintf(os.Stderr, "[goblin-fuzzwatch] %s early stop (%s): %s\n", opts.target, reason, detail)
	}
	return exitCode
}

func classifyOutcome(w *Watcher, waitErr error, extSignal os.Signal) (reason, detail string, exitCode int) {
	if extSignal != nil {
		if extSignal == syscall.SIGINT {
			return StopReasonInterrupted, "received SIGINT", exitCodeSIGINT
		}
		return StopReasonInterrupted, "received SIGTERM", exitCodeSIGTERM
	}
	if waitErr != nil {
		return StopReasonFailed, waitErr.Error(), exitCodeFromWaitErr(waitErr)
	}
	if ok, trigReason, trigDetail := w.Triggered(); ok {
		return trigReason, trigDetail, 0
	}
	return StopReasonCompleted, "", 0
}

func main() {
	opts, err := parseCLI(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitCodeBadUsage)
	}
	os.Exit(runWatch(context.Background(), opts))
}
