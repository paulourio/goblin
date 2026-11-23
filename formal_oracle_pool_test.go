package goblin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	oracleMaxWorkersPerExe = 4
	oracleScannerInitBuf   = 64 * 1024
	oracleScannerMaxBuf    = 32 * 1024 * 1024
)

// FormalRatJSON encodes an exact rational pair.
type FormalRatJSON struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

// BigRat converts Num/Den strings into an exact *big.Rat after validating denominator positivity.
func (r FormalRatJSON) BigRat() (*big.Rat, error) {
	num, ok := new(big.Int).SetString(r.Num, 10)
	if !ok {
		return nil, fmt.Errorf("invalid rational numerator %q", r.Num)
	}
	den, ok := new(big.Int).SetString(r.Den, 10)
	if !ok || den.Sign() <= 0 {
		return nil, fmt.Errorf("invalid rational denominator %q", r.Den)
	}
	return new(big.Rat).SetFrac(num, den), nil
}

// FormalOracleResp represents one JSONL response line from a Lean checker CLI.
type FormalOracleResp struct {
	OK     bool           `json:"ok"`
	Detail string         `json:"detail,omitzero"`
	Mode   string         `json:"mode,omitzero"`
	GapTol *FormalRatJSON `json:"gapTol,omitzero"`
	Error  string         `json:"error,omitzero"`
	Bound  *FormalRatJSON `json:"bound,omitzero"`
	Result jsontext.Value `json:"result,omitzero"`
}

type jsonlOracleWorker struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdin  *bufio.Writer
	inPipe interface{ Close() error }
	stdout *bufio.Scanner
	stderr *bytes.Buffer
}

func (w *jsonlOracleWorker) close() {
	if w == nil {
		return
	}
	if w.inPipe != nil {
		_ = w.inPipe.Close()
	}
	if w.cancel != nil {
		w.cancel()
	}
	if w.cmd != nil {
		_ = w.cmd.Wait()
	}
}

type exeOraclePool struct {
	binPath   string
	extraArgs []string
	sem       chan struct{}
	idle      chan *jsonlOracleWorker
}

var (
	globalOraclePoolsMu sync.Mutex
	globalOraclePools   = make(map[string]*exeOraclePool)
	globalOracleBinMap  sync.Map
	oracleSpawnCount    atomic.Int64
)

// FormalOracleEnabled reports whether GOBLIN_FORMAL=1 is set in the environment.
func FormalOracleEnabled() bool {
	return os.Getenv("GOBLIN_FORMAL") == "1"
}

func formalFuzzEnabled() bool {
	return FormalOracleEnabled()
}

func compactJSONLine(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("empty JSONL request payload")
	}
	if bytes.IndexByte(trimmed, '\n') < 0 && bytes.IndexByte(trimmed, '\r') < 0 {
		return trimmed, nil
	}
	val := make(jsontext.Value, len(trimmed))
	copy(val, trimmed)
	if err := val.Compact(); err != nil {
		return nil, fmt.Errorf("compact multi-line JSON payload: %w", err)
	}
	return val, nil
}

func spawnJSONLWorker(binPath string, extraArgs ...string) (*jsonlOracleWorker, error) {
	ctx, cancel := context.WithCancel(context.Background())
	args := make([]string, 0, len(extraArgs)+1)
	args = append(args, extraArgs...)
	args = append(args, "--jsonl")
	cmd := exec.CommandContext(ctx, binPath, args...)
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe for %s: %w", binPath, err)
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = inPipe.Close()
		cancel()
		return nil, fmt.Errorf("stdout pipe for %s: %w", binPath, err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		_ = inPipe.Close()
		_ = outPipe.Close()
		cancel()
		return nil, fmt.Errorf("start %s %v: %w", binPath, args, err)
	}
	oracleSpawnCount.Add(1)
	scanner := bufio.NewScanner(outPipe)
	scanner.Buffer(make([]byte, oracleScannerInitBuf), oracleScannerMaxBuf)
	return &jsonlOracleWorker{
		cmd:    cmd,
		cancel: cancel,
		stdin:  bufio.NewWriterSize(inPipe, oracleScannerInitBuf),
		inPipe: inPipe,
		stdout: scanner,
		stderr: &stderrBuf,
	}, nil
}

func (w *jsonlOracleWorker) roundTrip(line []byte) ([]byte, error) {
	if _, err := w.stdin.Write(line); err != nil {
		return nil, err
	}
	if err := w.stdin.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := w.stdin.Flush(); err != nil {
		return nil, err
	}
	if !w.stdout.Scan() {
		scanErr := w.stdout.Err()
		if scanErr == nil {
			scanErr = errors.New("unexpected EOF from oracle stdout")
		}
		return nil, fmt.Errorf("%w (stderr: %s)", scanErr, strings.TrimSpace(w.stderr.String()))
	}
	raw := w.stdout.Bytes()
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return cp, nil
}

func getExeOraclePool(poolKey, binPath string, extraArgs ...string) *exeOraclePool {
	globalOraclePoolsMu.Lock()
	defer globalOraclePoolsMu.Unlock()
	if p, ok := globalOraclePools[poolKey]; ok {
		return p
	}
	argsCopy := make([]string, len(extraArgs))
	copy(argsCopy, extraArgs)
	p := &exeOraclePool{
		binPath:   binPath,
		extraArgs: argsCopy,
		sem:       make(chan struct{}, oracleMaxWorkersPerExe),
		idle:      make(chan *jsonlOracleWorker, oracleMaxWorkersPerExe),
	}
	globalOraclePools[poolKey] = p
	return p
}

func (p *exeOraclePool) call(reqJSON []byte) (FormalOracleResp, error) {
	line, err := compactJSONLine(reqJSON)
	if err != nil {
		return FormalOracleResp{}, err
	}

	p.sem <- struct{}{}
	defer func() { <-p.sem }()

	var w *jsonlOracleWorker
	select {
	case w = <-p.idle:
	default:
		w, err = spawnJSONLWorker(p.binPath, p.extraArgs...)
		if err != nil {
			return FormalOracleResp{}, err
		}
	}

	respBytes, ioErr := w.roundTrip(line)
	if ioErr != nil {
		w.close()
		w, err = spawnJSONLWorker(p.binPath, p.extraArgs...)
		if err != nil {
			return FormalOracleResp{}, fmt.Errorf("respawn after %w: %w", ioErr, err)
		}
		respBytes, ioErr = w.roundTrip(line)
		if ioErr != nil {
			w.close()
			return FormalOracleResp{}, fmt.Errorf("retry roundTrip failed: %w", ioErr)
		}
	}

	var resp FormalOracleResp
	if unmarshalErr := json.Unmarshal(respBytes, &resp); unmarshalErr != nil {
		w.close()
		return FormalOracleResp{}, fmt.Errorf("unmarshal oracle response %q: %w", respBytes, unmarshalErr)
	}

	p.idle <- w
	return resp, nil
}

func resolveFormalOracleBin(tb testing.TB, exeName string) string {
	tb.Helper()
	if v, ok := globalOracleBinMap.Load(exeName); ok {
		return v.(string)
	}
	binPath := formalExe(tb, exeName)
	globalOracleBinMap.Store(exeName, binPath)
	return binPath
}

// CallFormalOracleJSONL dispatches one request payload to a bounded pool of persistent `<exeName> --jsonl` child processes over buffered stdin/stdout pipes, automatically respawning once if a worker pipe breaks.
func CallFormalOracleJSONL(tb testing.TB, exeName string, payload []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleSubcmdJSONL(tb, exeName, "", payload)
}

// CallFormalOracleSubcmdJSONL dispatches one request payload to a bounded pool of persistent
// `<exeName> [<subcmd>] --jsonl` child processes. When subcmd is empty, subcommand routing uses
// the JSON payload's `subcommand`, `op`, or `mode` field.
func CallFormalOracleSubcmdJSONL(tb testing.TB, exeName, subcmd string, payload []byte) (FormalOracleResp, error) {
	tb.Helper()
	if !FormalOracleEnabled() {
		return FormalOracleResp{}, errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	binPath := resolveFormalOracleBin(tb, exeName)
	if subcmd == "" {
		return getExeOraclePool(exeName, binPath).call(payload)
	}
	poolKey := exeName + "\x00" + subcmd
	return getExeOraclePool(poolKey, binPath, subcmd).call(payload)
}

// FormalOracleCall unpacks status, rational bound, and message.
func FormalOracleCall(tb testing.TB, exeName string, reqJSON []byte) (ok bool, boundRat [2]string, detailOrErr string) {
	tb.Helper()
	resp, err := CallFormalOracleJSONL(tb, exeName, reqJSON)
	if err != nil {
		return false, [2]string{}, err.Error()
	}
	if resp.Bound != nil {
		boundRat = [2]string{resp.Bound.Num, resp.Bound.Den}
	}
	if !resp.OK {
		return false, boundRat, resp.Error
	}
	return true, boundRat, resp.Detail
}

// FormalCheckLPJSONL queries lp-check.
func FormalCheckLPJSONL(tb testing.TB, reqJSON []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleJSONL(tb, "lp-check", reqJSON)
}

// FormalCheckCutsJSONL queries cuts-check.
func FormalCheckCutsJSONL(tb testing.TB, reqJSON []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleJSONL(tb, "cuts-check", reqJSON)
}

// FormalCheckRulesJSONL queries rules-check.
func FormalCheckRulesJSONL(tb testing.TB, reqJSON []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleJSONL(tb, "rules-check", reqJSON)
}

// FormalCheckVIPRJSONL queries vipr-check.
func FormalCheckVIPRJSONL(tb testing.TB, reqJSON []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleJSONL(tb, "vipr-check", reqJSON)
}

// FormalCheckLSAPJSONL queries lsap-check.
func FormalCheckLSAPJSONL(tb testing.TB, reqJSON []byte) (FormalOracleResp, error) {
	tb.Helper()
	return CallFormalOracleJSONL(tb, "lsap-check", reqJSON)
}

// FormalOracleBatch serializes items into a single JSON array and executes `<exeName> [extraArgs...] --batch -` over standard input without temporary files.
func FormalOracleBatch(tb testing.TB, exeName string, items [][]byte, extraArgs ...string) (string, error) {
	tb.Helper()
	if !FormalOracleEnabled() {
		return "", errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	binPath := resolveFormalOracleBin(tb, exeName)
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(bytes.TrimSpace(item))
	}
	buf.WriteByte(']')

	args := make([]string, 0, len(extraArgs)+2)
	args = append(args, extraArgs...)
	args = append(args, "--batch", "-")
	cmd := exec.CommandContext(tb.Context(), binPath, args...)
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// CloseFormalOraclePools shuts down all pooled Lean worker processes.
func CloseFormalOraclePools() {
	globalOraclePoolsMu.Lock()
	defer globalOraclePoolsMu.Unlock()
	for name, p := range globalOraclePools {
		for {
			select {
			case w := <-p.idle:
				w.close()
			default:
				delete(globalOraclePools, name)
				goto nextPool
			}
		}
	nextPool:
	}
}

func killOneIdleWorkerForTest(exeName string) bool {
	globalOraclePoolsMu.Lock()
	p := globalOraclePools[exeName]
	globalOraclePoolsMu.Unlock()
	if p == nil {
		return false
	}
	select {
	case w := <-p.idle:
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
			_ = w.cmd.Wait()
		}
		p.idle <- w
		return true
	default:
		return false
	}
}

func TestOraclePoolOfflineHelpers(t *testing.T) {
	t.Parallel()

	if !formalFuzzEnabled() && oracleSpawnCount.Load() != 0 {
		t.Fatalf("expected zero spawned Lean processes when GOBLIN_FORMAL!=1, got %d", oracleSpawnCount.Load())
	}

	pretty := []byte("{\n  \"mode\": \"safe-bound\",\n  \"msg\": \"line1\\nline2\"\n}\n")
	line, err := compactJSONLine(pretty)
	if err != nil {
		t.Fatalf("compactJSONLine failed: %v", err)
	}
	if bytes.ContainsAny(line, "\r\n") {
		t.Fatalf("compacted line still contains newline: %q", line)
	}

	if _, err := compactJSONLine([]byte("   \n\t  ")); err == nil {
		t.Fatal("expected error on whitespace-only JSONL payload")
	}

	var okResp FormalOracleResp
	rawOK := []byte(`{"ok":true,"detail":"VERIFIED","bound":{"num":"-17","den":"4"},"result":{"gcd":6,"infeasible":true}}`)
	if err := json.Unmarshal(rawOK, &okResp); err != nil {
		t.Fatalf("unmarshal okResp: %v", err)
	}
	if !okResp.OK || okResp.Detail != "VERIFIED" || okResp.Bound == nil {
		t.Fatalf("unexpected okResp: %+v", okResp)
	}
	rat, err := okResp.Bound.BigRat()
	if err != nil || rat.Cmp(big.NewRat(-17, 4)) != 0 {
		t.Fatalf("unexpected Bound.BigRat(): rat=%v err=%v", rat, err)
	}

	var errResp FormalOracleResp
	if err := json.Unmarshal([]byte(`{"ok":false,"error":"bad cert"}`), &errResp); err != nil {
		t.Fatalf("unmarshal errResp: %v", err)
	}
	if errResp.OK || errResp.Error != "bad cert" {
		t.Fatalf("unexpected errResp: %+v", errResp)
	}
}

func TestFormalVIPRBoundMode(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)

	viprSampleRaw, err := os.ReadFile(filepath.Join("testdata", "formal", "vipr_sample.json"))
	if err != nil {
		t.Fatalf("read vipr_sample.json: %v", err)
	}

	mutateVIPR := func(tb testing.TB, fn func(m map[string]any)) []byte {
		tb.Helper()
		var doc map[string]any
		if uErr := json.Unmarshal(viprSampleRaw, &doc); uErr != nil {
			tb.Fatalf("unmarshal vipr_sample: %v", uErr)
		}
		fn(doc)
		out, mErr := json.Marshal(doc)
		if mErr != nil {
			tb.Fatalf("marshal mutated vipr: %v", mErr)
		}
		return out
	}

	// Case 1: Exact lower bound with non-zero objConst (3/2 + 4 = 11/2) and no incumbent.
	boundNoInc := mutateVIPR(t, func(m map[string]any) {
		m["objConst"] = ratPayload{Num: "3", Den: "2"}
		m["mode"] = "bound"
		m["targetLB"] = ratPayload{Num: "11", Den: "2"}
		delete(m, "incumbent")
	})
	resp1, err := FormalCheckVIPRJSONL(t, boundNoInc)
	if err != nil || !resp1.OK || resp1.Bound == nil {
		t.Fatalf("boundNoInc failed: resp=%+v err=%v", resp1, err)
	}
	gotB1, err := resp1.Bound.BigRat()
	if err != nil || gotB1.Cmp(big.NewRat(11, 2)) != 0 {
		t.Fatalf("boundNoInc bound=%v err=%v, want 11/2", gotB1, err)
	}

	// Case 2: Relaxed lower bound (5 <= 11/2) with valid optional incumbent [0, 2, 0].
	boundWithInc := mutateVIPR(t, func(m map[string]any) {
		m["objConst"] = ratPayload{Num: "3", Den: "2"}
		m["mode"] = "bound"
		m["targetLB"] = ratPayload{Num: "5", Den: "1"}
	})
	resp2, err := FormalCheckVIPRJSONL(t, boundWithInc)
	if err != nil || !resp2.OK {
		t.Fatalf("boundWithInc failed: resp=%+v err=%v", resp2, err)
	}

	// Case 3: Single-file vipr-check CLI execution on mode="bound".
	viprExe := resolveFormalOracleBin(t, "vipr-check")
	tmpFile := filepath.Join(t.TempDir(), "vipr_bound.json")
	if writeErr := os.WriteFile(tmpFile, boundNoInc, 0o600); writeErr != nil {
		t.Fatalf("write vipr_bound.json: %v", writeErr)
	}
	cliOut, cliErr := exec.CommandContext(t.Context(), viprExe, tmpFile).CombinedOutput()
	if cliErr != nil || strings.TrimSpace(string(cliOut)) != "VERIFIED" {
		t.Fatalf("vipr-check CLI on bound mode failed: err=%v out=%q", cliErr, cliOut)
	}

	// Case 4: Infeasible root derivation vacuously certifies any targetLB in bound mode.
	infeasBound := []byte(`{
		"n": 1,
		"objConst": 0,
		"obj": [1],
		"isInt": [true],
		"lb": [0],
		"ub": [1],
		"baseRows": [{"coeffs": [1], "sense": ">=", "rhs": 2}],
		"mode": "bound",
		"targetLB": "999999/1",
		"steps": [{
			"rule": "farkas",
			"refs": [{"id": 0, "mult": -1}],
			"bounds": [{"col": 0, "kind": "ub", "mult": 1}]
		}],
		"rootStep": 0
	}`)
	resp3, err := FormalCheckVIPRJSONL(t, infeasBound)
	if err != nil || !resp3.OK {
		t.Fatalf("infeasBound failed: resp=%+v err=%v", resp3, err)
	}

	// Case 5 (Negative): targetLB = 6 > 11/2 must be rejected.
	badTarget := mutateVIPR(t, func(m map[string]any) {
		m["objConst"] = ratPayload{Num: "3", Den: "2"}
		m["mode"] = "bound"
		m["targetLB"] = ratPayload{Num: "6", Den: "1"}
		delete(m, "incumbent")
	})
	resp4, err := FormalCheckVIPRJSONL(t, badTarget)
	if err != nil || resp4.OK {
		t.Fatalf("badTarget unexpectedly passed: resp=%+v err=%v", resp4, err)
	}

	// Case 6 (Negative): valid targetLB = 4, but infeasible optional incumbent [0, 0, 0].
	badInc := mutateVIPR(t, func(m map[string]any) {
		m["mode"] = "bound"
		m["targetLB"] = ratPayload{Num: "4", Den: "1"}
		m["incumbent"] = []int{0, 0, 0}
	})
	resp5, err := FormalCheckVIPRJSONL(t, badInc)
	if err != nil || resp5.OK {
		t.Fatalf("badInc unexpectedly passed: resp=%+v err=%v", resp5, err)
	}
}

func TestOraclePoolConcurrentAndBatchAllCLIs(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)

	lsapPos, err := os.ReadFile(filepath.Join("testdata", "formal", "lsap_sample.json"))
	if err != nil {
		t.Fatalf("read lsap_sample.json: %v", err)
	}
	lsapNeg, err := os.ReadFile(filepath.Join("testdata", "formal", "lsap_negative.json"))
	if err != nil {
		t.Fatalf("read lsap_negative.json: %v", err)
	}
	lpOptPos, err := os.ReadFile(filepath.Join("testdata", "formal", "lp_optimal_sample.json"))
	if err != nil {
		t.Fatalf("read lp_optimal_sample.json: %v", err)
	}
	lpFarkasPos, err := os.ReadFile(filepath.Join("testdata", "formal", "lp_farkas_sample.json"))
	if err != nil {
		t.Fatalf("read lp_farkas_sample.json: %v", err)
	}
	viprPos, err := os.ReadFile(filepath.Join("testdata", "formal", "vipr_sample.json"))
	if err != nil {
		t.Fatalf("read vipr_sample.json: %v", err)
	}

	var lsapSafeMap map[string]any
	if err := json.Unmarshal(lsapPos, &lsapSafeMap); err != nil {
		t.Fatalf("unmarshal lsapPos: %v", err)
	}
	lsapSafeMap["mode"] = "safe"
	lsapSafeMap["tol"] = "1/1000"
	lsapSafePos, err := json.Marshal(lsapSafeMap)
	if err != nil {
		t.Fatalf("marshal lsapSafePos: %v", err)
	}

	var lpSafeMap map[string]any
	if err := json.Unmarshal(lpOptPos, &lpSafeMap); err != nil {
		t.Fatalf("unmarshal lpOptPos: %v", err)
	}
	lpSafeMap["mode"] = "safe-bound"
	lpSafePos, err := json.Marshal(lpSafeMap)
	if err != nil {
		t.Fatalf("marshal lpSafePos: %v", err)
	}
	lpBad := []byte(`{"mode":"farkas","numVars":1,"numRows":1,"obj":[{"num":"1","den":"1"}],"lb":[{"num":"0","den":"1"}],"ub":[null],"rows":[{"sense":"ge","rhs":{"num":"1","den":"1"},"terms":[{"col":0,"coeff":{"num":"1","den":"1"}}]}],"w":[{"num":"1","den":"1"}]}`)

	cutsPos := []byte(`{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"1/1000"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"5/1"}}`)
	cutsNeg := []byte(`{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"-1/10"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"5/1"}}`)

	rulesPos := []byte(`{"op":"gcd-infeas","coeffs":[6,10],"lo":5,"hi":5,"go_infeasible":true}`)
	rulesActPos := []byte(`{"subcommand":"activity","op":"summary","rhs":"5/1","has_lo":false,"has_hi":true,"terms":[{"a":"2/1","l":"0/1","u":"10/1","is_int":true,"go_ub":"2/1"}]}`)
	rulesNeg := []byte(`{"op":"gcd-infeas","coeffs":[6,10],"lo":6,"hi":6,"go_infeasible":true}`)

	viprNeg := []byte(`{"n":1,"obj":[1],"isInt":[true],"lb":[0],"ub":[1],"baseRows":[],"mode":"infeasible","steps":[{"rule":"assume","col":0,"sense":"<=","bound":0}],"rootStep":0}`)

	// Verify broken-pipe auto-respawn on lp-check.
	firstResp, err := FormalCheckLPJSONL(t, lpSafePos)
	if err != nil || !firstResp.OK || firstResp.Bound == nil {
		t.Fatalf("initial lpSafePos check failed: resp=%+v err=%v", firstResp, err)
	}
	if !killOneIdleWorkerForTest("lp-check") {
		t.Fatal("expected idle lp-check worker to kill for respawn test")
	}
	respawnResp, err := FormalCheckLPJSONL(t, lpSafePos)
	if err != nil || !respawnResp.OK || respawnResp.Bound == nil {
		t.Fatalf("respawn lpSafePos check failed: resp=%+v err=%v", respawnResp, err)
	}

	// Fire 150 concurrent JSONL calls (positive + negative) across all 5 CLIs.
	const workers = 6
	const itersPerWorker = 25
	errCh := make(chan error, workers*itersPerWorker)
	var wg sync.WaitGroup
	for wIdx := range workers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for iter := range itersPerWorker {
				k := (workerID*itersPerWorker + iter) % 10
				switch k {
				case 0:
					ok, bnd, msg := FormalOracleCall(t, "lsap-check", lsapSafePos)
					if !ok || bnd != [2]string{"9", "1"} {
						errCh <- fmt.Errorf("lsap safe pos failed: ok=%v bnd=%v msg=%s", ok, bnd, msg)
					}
				case 1:
					ok, _, _ := FormalOracleCall(t, "lsap-check", lsapNeg)
					if ok {
						errCh <- errors.New("lsap neg unexpectedly passed")
					}
				case 2:
					ok, bnd, msg := FormalOracleCall(t, "lp-check", lpSafePos)
					if !ok || bnd != [2]string{"113", "2"} {
						errCh <- fmt.Errorf("lp safe-bound pos failed: ok=%v bnd=%v msg=%s", ok, bnd, msg)
					}
				case 3:
					ok, _, _ := FormalOracleCall(t, "lp-check", lpBad)
					if ok {
						errCh <- errors.New("lp bad farkas unexpectedly passed")
					}
				case 4:
					ok, _, msg := FormalOracleCall(t, "cuts-check", cutsPos)
					if !ok {
						errCh <- fmt.Errorf("cuts pos failed: %s", msg)
					}
				case 5:
					ok, _, _ := FormalOracleCall(t, "cuts-check", cutsNeg)
					if ok {
						errCh <- errors.New("cuts neg unexpectedly passed")
					}
				case 6:
					r, rErr := FormalCheckRulesJSONL(t, rulesPos)
					if rErr != nil {
						errCh <- fmt.Errorf("rules gcd pos transport failed: %w", rErr)
					} else if !r.OK || len(r.Result) == 0 {
						errCh <- fmt.Errorf("rules gcd pos rejected: resp=%+v", r)
					}
				case 7:
					r, rErr := FormalCheckRulesJSONL(t, rulesActPos)
					if rErr != nil {
						errCh <- fmt.Errorf("rules activity pos transport failed: %w", rErr)
					} else if !r.OK {
						errCh <- fmt.Errorf("rules activity pos rejected: resp=%+v", r)
					}
					rNeg, rNegErr := FormalCheckRulesJSONL(t, rulesNeg)
					if rNegErr != nil {
						errCh <- fmt.Errorf("rules neg transport failed: %w", rNegErr)
					} else if rNeg.OK {
						errCh <- fmt.Errorf("rules neg unexpectedly passed: resp=%+v", rNeg)
					}
				case 8:
					ok, _, msg := FormalOracleCall(t, "vipr-check", viprPos)
					if !ok {
						errCh <- fmt.Errorf("vipr pos failed: %s", msg)
					}
				case 9:
					ok, _, _ := FormalOracleCall(t, "vipr-check", viprNeg)
					if ok {
						errCh <- errors.New("vipr neg unexpectedly passed")
					}
				}
			}
		}(wIdx)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}

	// Verify --batch mode across all 5 CLIs (positive and negative controls).
	batchCases := []struct {
		exe  string
		good [][]byte
		bad  [][]byte
	}{
		{"lsap-check", [][]byte{lsapPos, lsapSafePos}, [][]byte{lsapPos, lsapNeg}},
		{"lp-check", [][]byte{lpOptPos, lpFarkasPos, lpSafePos}, [][]byte{lpOptPos, lpBad}},
		{"cuts-check", [][]byte{cutsPos, cutsPos}, [][]byte{cutsPos, cutsNeg}},
		{"rules-check", [][]byte{rulesPos, rulesActPos}, [][]byte{rulesPos, rulesNeg}},
		{"vipr-check", [][]byte{viprPos, viprPos}, [][]byte{viprPos, viprNeg}},
	}
	for _, bc := range batchCases {
		wantLine := fmt.Sprintf("VERIFIED (%d items)", len(bc.good))
		outGood, errGood := FormalOracleBatch(t, bc.exe, bc.good)
		if errGood != nil || outGood != wantLine {
			t.Fatalf("%s --batch good failed: err=%v out=%q want=%q", bc.exe, errGood, outGood, wantLine)
		}
		outBad, errBad := FormalOracleBatch(t, bc.exe, bc.bad)
		if errBad == nil || !strings.Contains(outBad, "BATCH FAIL at index 1") {
			t.Fatalf("%s --batch bad expected index 1 failure: err=%v out=%q", bc.exe, errBad, outBad)
		}
	}
}

func TestFormalWave2SubcommandsAndExporters(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)

	// 1. Free and one-sided non-dyadic LP via ExportLPCheckPayload + ExactBasisDuals.
	mFree := NewModel("w2_free_nondyadic", Minimize)
	xf := mFree.AddContVar("xf", math.Inf(-1), math.Inf(1))
	xp := mFree.AddContVar("xp", 0, math.Inf(1))
	xn := mFree.AddContVar("xn", math.Inf(-1), 0)
	xb := mFree.AddContVar("xb", 0, 4)
	mFree.SetObjective(NewExpr(xf.T(5), xp.T(7), xn.T(-3), xb.T(2)).AddConstant(12.5))
	mFree.AddConstraint("r0", NewExpr(xf.T(3), xp.T(2), xn.T(1)).Eq(4))
	mFree.AddConstraint("r1", NewExpr(xf.T(1), xp.T(5), xn.T(-2), xb.T(3)).Ge(9))
	mFree.AddConstraint("r2", NewExpr(xp.T(2), xb.T(7)).Le(15))
	pFree := mFree.Compile()

	lpFree := NewLPSolver(pFree, nil)
	defer lpFree.Close()
	resFree := lpFree.Solve()
	if resFree.Status != StatusOptimal {
		t.Fatalf("free LP status=%v, want StatusOptimal", resFree.Status)
	}
	modeFree, payFree, okFree := ExportLPCheckPayload(pFree, lpFree, resFree)
	if !okFree || modeFree != "optimal" {
		t.Fatalf("ExportLPCheckPayload on free non-dyadic LP failed: ok=%v mode=%q", okFree, modeFree)
	}
	respFree, err := FormalCheckLPJSONL(t, payFree)
	if err != nil || !respFree.OK || respFree.Bound == nil || respFree.Mode != "optimal" {
		t.Fatalf("lp-check rejected free non-dyadic LP: resp=%+v err=%v", respFree, err)
	}

	// 2. LP unbounded & MILP Meyer unbounded via ExportLPCheckPayload / ExportUnboundedCheckPayload.
	mLPUnb := NewModel("w2_lp_unb", Minimize)
	ux := mLPUnb.AddContVar("x", 0, math.Inf(1))
	uy := mLPUnb.AddContVar("y", 0, 10)
	mLPUnb.SetObjective(NewExpr(ux.T(-3), uy.T(2)))
	mLPUnb.AddConstraint("ge", NewExpr(ux.T(5), uy.T(1)).Ge(2))
	pLPUnb := mLPUnb.Compile()
	lpUnb := NewLPSolver(pLPUnb, nil)
	defer lpUnb.Close()
	resLPUnb := lpUnb.Solve()
	modeUnb, payUnb, okUnb := ExportLPCheckPayload(pLPUnb, lpUnb, resLPUnb)
	if !okUnb || modeUnb != "unbounded" {
		t.Fatalf("ExportLPCheckPayload unbounded failed: ok=%v mode=%q", okUnb, modeUnb)
	}
	if r, rErr := CallFormalOracleSubcmdJSONL(t, "lp-check", "unbounded", payUnb); rErr != nil || !r.OK {
		t.Fatalf("lp-check unbounded --jsonl failed: resp=%+v err=%v", r, rErr)
	}
	if r, rErr := FormalCheckLPJSONL(t, payUnb); rErr != nil || !r.OK {
		t.Fatalf("lp-check --jsonl unbounded failed: resp=%+v err=%v", r, rErr)
	}

	mMIPUnb := NewModel("w2_milp_unb", Minimize)
	mx := mMIPUnb.AddIntVar("x", 2, math.Inf(1))
	my := mMIPUnb.AddIntVar("y", 0, math.Inf(1))
	mc := mMIPUnb.AddContVar("c", 0, math.Inf(1))
	mMIPUnb.SetObjective(NewExpr(mx.T(-2), my.T(1)))
	mMIPUnb.AddConstraint("dio", NewExpr(mx.T(3), my.T(-2)).Eq(0))
	mMIPUnb.AddConstraint("cont", NewExpr(mc.T(3), mx.T(-1)).Eq(0))
	pMIPUnb := mMIPUnb.Compile()
	modeMIPUnb, payMIPUnb, okMIPUnb := ExportUnboundedCheckPayload(pMIPUnb, nil, nil)
	if !okMIPUnb || modeMIPUnb != "milp-unbounded" {
		t.Fatalf("ExportUnboundedCheckPayload failed: ok=%v mode=%q", okMIPUnb, modeMIPUnb)
	}
	if r, rErr := CallFormalOracleSubcmdJSONL(t, "lp-check", "milp-unbounded", payMIPUnb); rErr != nil || !r.OK {
		t.Fatalf("lp-check milp-unbounded --jsonl failed: resp=%+v err=%v", r, rErr)
	}
	if r, rErr := FormalCheckLPJSONL(t, payMIPUnb); rErr != nil || !r.OK {
		t.Fatalf("lp-check --jsonl milp-unbounded failed: resp=%+v err=%v", r, rErr)
	}
	// Negative control: fractional ray on MILP must fail.
	badMIPUnb := bytes.Replace(payMIPUnb, []byte(`"intCols":[0,1]`), []byte(`"intCols":[0,1,2]`), 1)
	if r, rErr := FormalCheckLPJSONL(t, badMIPUnb); rErr != nil || r.OK {
		t.Fatalf("lp-check unexpectedly accepted fractional c as integer col: resp=%+v err=%v", r, rErr)
	}

	// 3. LP presolve certificate via lp-check lp-presolve CLI, --batch, and --jsonl.
	mOrig := NewModel("w2_pre_orig", Minimize)
	ox0 := mOrig.AddContVar("x0", 0, 10)
	ox1 := mOrig.AddContVar("x1", 0, 10)
	mOrig.SetObjective(NewExpr(ox0.T(-2), ox1.T(-1)).AddConstant(4))
	mOrig.AddConstraint("sng", NewExpr(ox0.T(2)).Le(6))
	mOrig.AddConstraint("cap", NewExpr(ox0.T(1), ox1.T(1)).Le(8))
	pOrig := mOrig.Compile()

	mRed := NewModel("w2_pre_red", Minimize)
	rx0 := mRed.AddContVar("x0", 0, 3)
	rx1 := mRed.AddContVar("x1", 0, 10)
	mRed.SetObjective(NewExpr(rx0.T(-2), rx1.T(-1)).AddConstant(4))
	mRed.AddConstraint("cap", NewExpr(rx0.T(1), rx1.T(1)).Le(8))
	pRed := mRed.Compile()
	lpRed := NewLPSolver(pRed, nil)
	defer lpRed.Close()
	_ = lpRed.Solve()
	solRed, okSolRed := ExactBasisSolution(t.Context(), lpRed)
	if !okSolRed {
		t.Fatal("ExactBasisSolution failed on pRed")
	}
	idxZero := 0
	payLPPre, err := json.Marshal(lpPresolveCertPayload{
		Orig:    compiledProblemToLPPayload(t, pOrig, "lp-presolve"),
		Reduced: compiledProblemToLPPayload(t, pRed, "optimal"),
		Step: lpPresolveStepPayload{
			Kind:       "singletonRow",
			OrigRow:    &idxZero,
			Col:        &idxZero,
			OrigRowIdx: []int{1},
		},
		XReduced: ratSliceToPayloads(solRed.Primal),
		YReduced: ratSliceToPayloads(solRed.DualMin),
	})
	if err != nil {
		t.Fatalf("marshal payLPPre: %v", err)
	}
	respPre, err := CallFormalOracleSubcmdJSONL(t, "lp-check", "lp-presolve", payLPPre)
	if err != nil || !respPre.OK || respPre.Bound == nil {
		t.Fatalf("lp-check lp-presolve --jsonl failed: resp=%+v err=%v", respPre, err)
	}
	if b, bErr := respPre.Bound.BigRat(); bErr != nil || b.Cmp(big.NewRat(-7, 1)) != 0 {
		t.Fatalf("lp-presolve bound=%v err=%v, want -7", b, bErr)
	}
	// Auto-detected mode="lp-presolve" over default lp-check --jsonl and --batch.
	if r, rErr := FormalCheckLPJSONL(t, payLPPre); rErr != nil || !r.OK {
		t.Fatalf("lp-check auto lp-presolve --jsonl failed: resp=%+v err=%v", r, rErr)
	}
	if out, bErr := FormalOracleBatch(t, "lp-check", [][]byte{payUnb, payMIPUnb, payLPPre}); bErr != nil || out != "VERIFIED (3 items)" {
		t.Fatalf("lp-check --batch mixed modes failed: err=%v out=%q", bErr, out)
	}

	// 4. ExportPresolveRulesItems: gcd-div-check + >256-row pagination.
	mRules := NewModel("w2_rules_gcd_paginated", Minimize)
	gx0 := mRules.AddIntVar("x0", 0, 20)
	gx1 := mRules.AddIntVar("x1", 0, 20)
	mRules.SetObjective(NewExpr(gx0.T(1), gx1.T(1)))
	for i := range 300 {
		mRules.AddConstraint(fmt.Sprintf("pad_%d", i), NewExpr(gx0.T(1), gx1.T(1)).Le(float64(40+i)))
	}
	mRules.AddConstraint("gcd_le", NewExpr(gx0.T(6), gx1.T(9)).Le(17.5))
	mRules.AddConstraint("gcd_ge", NewExpr(gx0.T(4), gx1.T(6)).Ge(9.2))
	mRules.AddConstraint("gcd_eq", NewExpr(gx0.T(6), gx1.T(10)).Eq(14))
	ruleItems, _ := ExportPresolveRulesItems(mRules.Compile())
	sawGcdDiv := 0
	for _, raw := range ruleItems {
		var hdr struct {
			Subcommand string `json:"subcommand"`
		}
		if json.Unmarshal(raw, &hdr) == nil && hdr.Subcommand == "gcd-div-check" {
			sawGcdDiv++
			if rSub, subErr := CallFormalOracleSubcmdJSONL(t, "rules-check", "gcd-div-check", raw); subErr != nil || !rSub.OK {
				t.Fatalf("rules-check gcd-div-check --jsonl failed: resp=%+v err=%v", rSub, subErr)
			}
		}
		if r, rErr := FormalCheckRulesJSONL(t, raw); rErr != nil || !r.OK {
			t.Fatalf("rules-check --jsonl rejected exported item: resp=%+v err=%v\nraw=%s", r, rErr, raw)
		}
	}
	if sawGcdDiv < 3 {
		t.Fatalf("expected >=3 gcd-div-check items past row 300, got %d", sawGcdDiv)
	}

	// 5. ExportSeparatedCutsItems on semi-bounded MIP with nil feasibleSols covering
	// cover-check, lotsizing-check, agg-cmir-check, and drop-check!
	mCuts := NewModel("w2_cuts_all_track2c", Minimize)
	// Lot-sizing 3-period chain with semi-bounded continuous production/inventory [0, +Inf).
	y0 := mCuts.AddBinaryVar("y0")
	y1 := mCuts.AddBinaryVar("y1")
	y2 := mCuts.AddBinaryVar("y2")
	p0 := mCuts.AddContVar("p0", 0, math.Inf(1))
	p1 := mCuts.AddContVar("p1", 0, math.Inf(1))
	p2 := mCuts.AddContVar("p2", 0, math.Inf(1))
	s0 := mCuts.AddContVar("s0", 0, math.Inf(1))
	s1 := mCuts.AddContVar("s1", 0, math.Inf(1))
	s2 := mCuts.AddContVar("s2", 0, math.Inf(1))
	mCuts.AddConstraint("vub0", NewExpr(p0.T(1), y0.T(-40)).Le(0))
	mCuts.AddConstraint("vub1", NewExpr(p1.T(1), y1.T(-40)).Le(0))
	mCuts.AddConstraint("vub2", NewExpr(p2.T(1), y2.T(-40)).Le(0))
	mCuts.AddConstraint("bal0", NewExpr(p0.T(1), s0.T(-1)).Eq(10))
	mCuts.AddConstraint("bal1", NewExpr(s0.T(1), p1.T(1), s1.T(-1)).Eq(12))
	mCuts.AddConstraint("bal2", NewExpr(s1.T(1), p2.T(1), s2.T(-1)).Eq(15))
	// Knapsack cover block.
	b0 := mCuts.AddBinaryVar("b0")
	b1 := mCuts.AddBinaryVar("b1")
	b2 := mCuts.AddBinaryVar("b2")
	b3 := mCuts.AddBinaryVar("b3")
	mCuts.AddConstraint("knap", NewExpr(b0.T(5), b1.T(5), b2.T(5), b3.T(4)).Le(11))
	// Multi-row aggregated MIR block with semi-bounded continuous variable sc in [0, +Inf).
	ix0 := mCuts.AddIntVar("ix0", 0, 5)
	ix1 := mCuts.AddIntVar("ix1", 0, 5)
	sc := mCuts.AddContVar("sc", 0, math.Inf(1))
	mCuts.AddConstraint("agg_eq", NewExpr(ix0.T(2), sc.T(1)).Eq(3))
	mCuts.AddConstraint("agg_le", NewExpr(sc.T(-1), ix1.T(2)).Le(2))
	mCuts.SetObjective(NewExpr(
		y0.T(15), y1.T(15), y2.T(15),
		b0.T(-6), b1.T(-6), b2.T(-6), b3.T(-3),
		ix0.T(-4), ix1.T(-4),
	))
	pCuts := mCuts.Compile()

	cutItems, err := ExportSeparatedCutsItems(pCuts, nil)
	if err != nil {
		t.Fatalf("ExportSeparatedCutsItems(pCuts, nil) failed: %v", err)
	}
	seenCutOps := make(map[string]int)
	for _, raw := range cutItems {
		var hdr struct {
			Subcommand string `json:"subcommand"`
			Op         string `json:"op"`
		}
		if uErr := json.Unmarshal(raw, &hdr); uErr != nil {
			t.Fatalf("unmarshal cut item: %v", uErr)
		}
		sub := hdr.Subcommand
		if sub == "" {
			sub = hdr.Op
		}
		if sub == "" {
			t.Fatalf("cut item missing subcommand/op: %s", string(raw))
		}
		seenCutOps[sub]++
		if rSub, subErr := CallFormalOracleSubcmdJSONL(t, "cuts-check", sub, raw); subErr != nil || !rSub.OK {
			t.Fatalf("cuts-check %s --jsonl failed: resp=%+v err=%v", sub, rSub, subErr)
		}
		if r, rErr := FormalCheckCutsJSONL(t, raw); rErr != nil || !r.OK {
			t.Fatalf("cuts-check --jsonl rejected %s: resp=%+v err=%v", sub, r, rErr)
		}
	}
	for _, wantOp := range []string{"cover-check", "lotsizing-check", "agg-cmir-check", "drop-check"} {
		if seenCutOps[wantOp] == 0 {
			t.Fatalf("expected %s in exported semi-bounded cuts, got %v", wantOp, seenCutOps)
		}
	}
	if out, bErr := FormalOracleBatch(t, "cuts-check", cutItems); bErr != nil || out != fmt.Sprintf("VERIFIED (%d items)", len(cutItems)) {
		t.Fatalf("cuts-check --batch on Track 2C items failed: err=%v out=%q", bErr, out)
	}
}
