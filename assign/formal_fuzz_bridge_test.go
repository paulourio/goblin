package assign_test

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
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/paulourio/goblin/assign"
)

const (
	assignOracleMaxWorkers = 4
	assignOracleInitBuf    = 64 * 1024
	assignOracleMaxBuf     = 32 * 1024 * 1024
)

type formalAssignRat struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

func (r formalAssignRat) bigRat() (*big.Rat, error) {
	n, ok := new(big.Int).SetString(r.Num, 10)
	if !ok {
		return nil, fmt.Errorf("invalid rational num %q", r.Num)
	}
	d, ok := new(big.Int).SetString(r.Den, 10)
	if !ok || d.Sign() <= 0 {
		return nil, fmt.Errorf("invalid rational den %q", r.Den)
	}
	return new(big.Rat).SetFrac(n, d), nil
}

type formalAssignOracleResp struct {
	OK     bool             `json:"ok"`
	Detail string           `json:"detail,omitzero"`
	Error  string           `json:"error,omitzero"`
	Bound  *formalAssignRat `json:"bound,omitzero"`
}

type lsapBitsVal struct {
	Bits uint64 `json:"bits"`
}

type lsapBitsCertReq struct {
	Rows      int             `json:"rows"`
	Cols      int             `json:"cols"`
	Sense     string          `json:"sense"`
	Mode      string          `json:"mode"`
	Cost      [][]lsapBitsVal `json:"cost"`
	RowInd    []int           `json:"row_ind"`
	ColInd    []int           `json:"col_ind"`
	Objective lsapBitsVal     `json:"objective"`
	U         []lsapBitsVal   `json:"u"`
	V         []lsapBitsVal   `json:"v"`
	Tol       *lsapBitsVal    `json:"tol,omitzero"`
}

type assignJSONLProc struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdin  *bufio.Writer
	inPipe interface{ Close() error }
	stdout *bufio.Scanner
	stderr *bytes.Buffer
}

func (p *assignJSONLProc) shutdown() {
	if p == nil {
		return
	}
	if p.inPipe != nil {
		_ = p.inPipe.Close()
	}
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil {
		_ = p.cmd.Wait()
	}
}

var (
	assignLsapBinOnce sync.Once
	assignLsapBinPath string
	assignLsapSem     = make(chan struct{}, assignOracleMaxWorkers)
	assignLsapIdle    = make(chan *assignJSONLProc, assignOracleMaxWorkers)
)

func formalAssignOracleEnabled() bool {
	return os.Getenv("GOBLIN_FORMAL") == "1"
}

func locateLSAPCheckBin(tb testing.TB) string {
	tb.Helper()
	assignLsapBinOnce.Do(func() {
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
		binName := "lsap-check"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(formalDir, ".lake", "build", "bin", binName)
		if info, statErr := os.Stat(binPath); statErr == nil && !info.IsDir() {
			assignLsapBinPath = binPath
			return
		}
		lakePath, lookErr := exec.LookPath("lake")
		if lookErr != nil {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				tb.Fatalf("locate lake: %v (%v)", lookErr, homeErr)
			}
			lakePath = filepath.Join(home, ".elan", "bin", "lake")
		}
		var cmd *exec.Cmd
		if flockPath, flockErr := exec.LookPath("flock"); flockErr == nil {
			cmd = exec.CommandContext(tb.Context(), flockPath, "/tmp/goblin-formal-lake.lock", lakePath, "build", "lsap-check")
		} else {
			cmd = exec.CommandContext(tb.Context(), lakePath, "build", "lsap-check")
		}
		cmd.Dir = formalDir
		cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(lakePath)+string(os.PathListSeparator)+os.Getenv("PATH"))
		if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
			tb.Fatalf("lake build lsap-check: %v\n%s", buildErr, out)
		}
		assignLsapBinPath = binPath
	})
	return assignLsapBinPath
}

func startAssignLSAPProc(binPath string) (*assignJSONLProc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath, "--jsonl")
	inPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = inPipe.Close()
		cancel()
		return nil, err
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		_ = inPipe.Close()
		_ = outPipe.Close()
		cancel()
		return nil, err
	}
	sc := bufio.NewScanner(outPipe)
	sc.Buffer(make([]byte, assignOracleInitBuf), assignOracleMaxBuf)
	return &assignJSONLProc{
		cmd:    cmd,
		cancel: cancel,
		stdin:  bufio.NewWriterSize(inPipe, assignOracleInitBuf),
		inPipe: inPipe,
		stdout: sc,
		stderr: &errBuf,
	}, nil
}

func (p *assignJSONLProc) exchange(line []byte) ([]byte, error) {
	if _, err := p.stdin.Write(line); err != nil {
		return nil, err
	}
	if err := p.stdin.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := p.stdin.Flush(); err != nil {
		return nil, err
	}
	if !p.stdout.Scan() {
		scErr := p.stdout.Err()
		if scErr == nil {
			scErr = errors.New("lsap-check --jsonl unexpected EOF")
		}
		return nil, fmt.Errorf("%w (%s)", scErr, strings.TrimSpace(p.stderr.String()))
	}
	out := p.stdout.Bytes()
	cp := make([]byte, len(out))
	copy(cp, out)
	return cp, nil
}

func normalizeAssignJSONLine(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("empty lsap JSONL payload")
	}
	if bytes.IndexByte(trimmed, '\n') < 0 && bytes.IndexByte(trimmed, '\r') < 0 {
		return trimmed, nil
	}
	v := make(jsontext.Value, len(trimmed))
	copy(v, trimmed)
	if err := v.Compact(); err != nil {
		return nil, err
	}
	return v, nil
}

func callFormalAssignJSONL(tb testing.TB, reqJSON []byte) (formalAssignOracleResp, error) {
	tb.Helper()
	if !formalAssignOracleEnabled() {
		return formalAssignOracleResp{}, errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	line, err := normalizeAssignJSONLine(reqJSON)
	if err != nil {
		return formalAssignOracleResp{}, err
	}
	binPath := locateLSAPCheckBin(tb)

	assignLsapSem <- struct{}{}
	defer func() { <-assignLsapSem }()

	var proc *assignJSONLProc
	select {
	case proc = <-assignLsapIdle:
	default:
		proc, err = startAssignLSAPProc(binPath)
		if err != nil {
			return formalAssignOracleResp{}, err
		}
	}

	rawResp, ioErr := proc.exchange(line)
	if ioErr != nil {
		proc.shutdown()
		proc, err = startAssignLSAPProc(binPath)
		if err != nil {
			return formalAssignOracleResp{}, err
		}
		rawResp, ioErr = proc.exchange(line)
		if ioErr != nil {
			proc.shutdown()
			return formalAssignOracleResp{}, ioErr
		}
	}

	var parsed formalAssignOracleResp
	if uErr := json.Unmarshal(rawResp, &parsed); uErr != nil {
		proc.shutdown()
		return formalAssignOracleResp{}, uErr
	}
	assignLsapIdle <- proc
	return parsed, nil
}

func formalAssignOracleCall(tb testing.TB, reqJSON []byte) (ok bool, boundRat [2]string, detailOrErr string) {
	tb.Helper()
	resp, err := callFormalAssignJSONL(tb, reqJSON)
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

// buildFormalLSAPJSON encodes a finite flat LSAP instance and Result using exact IEEE-754 bit patterns.
func buildFormalLSAPJSON(rows, cols int, flatCost []float64, sense assign.Sense, res assign.Result, mode string, tol float64) ([]byte, bool) {
	if len(flatCost) != rows*cols || math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) {
		return nil, false
	}
	costBits := make([][]lsapBitsVal, rows)
	for r := range rows {
		row := make([]lsapBitsVal, cols)
		base := r * cols
		for c := range cols {
			v := flatCost[base+c]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, false
			}
			row[c] = lsapBitsVal{Bits: math.Float64bits(v)}
		}
		costBits[r] = row
	}
	uBits := make([]lsapBitsVal, len(res.U))
	for i, v := range res.U {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
		uBits[i] = lsapBitsVal{Bits: math.Float64bits(v)}
	}
	vBits := make([]lsapBitsVal, len(res.V))
	for j, v := range res.V {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
		vBits[j] = lsapBitsVal{Bits: math.Float64bits(v)}
	}
	senseStr := "min"
	if sense == assign.Maximize {
		senseStr = "max"
	}
	req := lsapBitsCertReq{
		Rows:      rows,
		Cols:      cols,
		Sense:     senseStr,
		Mode:      mode,
		Cost:      costBits,
		RowInd:    res.RowInd,
		ColInd:    res.ColInd,
		Objective: lsapBitsVal{Bits: math.Float64bits(res.Objective)},
		U:         uBits,
		V:         vBits,
	}
	if mode == "safe" {
		if math.IsNaN(tol) || math.IsInf(tol, 0) || tol < 0 {
			return nil, false
		}
		tVal := lsapBitsVal{Bits: math.Float64bits(tol)}
		req.Tol = &tVal
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func TestFormalAssignBridgeOffline(t *testing.T) {
	t.Parallel()
	res := assign.Result{
		RowInd:    []int{0},
		ColInd:    []int{0},
		Objective: 3.5,
		U:         []float64{3.5},
		V:         []float64{0},
	}
	raw, ok := buildFormalLSAPJSON(1, 1, []float64{3.5}, assign.Minimize, res, "safe", 1e-9)
	if !ok || len(raw) == 0 {
		t.Fatal("expected valid JSON payload for finite 1x1 matrix")
	}
	if !formalAssignOracleEnabled() {
		okCall, _, _ := formalAssignOracleCall(t, raw)
		if okCall {
			t.Fatal("expected formalAssignOracleCall to return false when GOBLIN_FORMAL!=1")
		}
	}
	if _, okInf := buildFormalLSAPJSON(1, 1, []float64{math.Inf(1)}, assign.Minimize, res, "safe", 1e-9); okInf {
		t.Fatal("expected buildFormalLSAPJSON to skip +Inf forbidden edge matrix")
	}
}

func TestFormalAssignOraclePoolConcurrent(t *testing.T) {
	t.Parallel()
	if !formalAssignOracleEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run lsap-check --jsonl pool tests")
	}

	// Build 120 diverse LSAP problems (square, wide, tall, Minimize, Maximize, exact & safe).
	type item struct {
		payload []byte
		wantOK  bool
		isSafe  bool
	}
	rng := rand.New(rand.NewPCG(0x51a7, 0x99d1))
	items := make([]item, 0, 128)
	var validBatch [][]byte
	for idx := range 110 {
		var rows, cols int
		switch idx % 3 {
		case 0:
			rows = 2 + rng.IntN(8)
			cols = rows
		case 1:
			rows = 2 + rng.IntN(6)
			cols = rows + 1 + rng.IntN(5)
		default:
			cols = 2 + rng.IntN(6)
			rows = cols + 1 + rng.IntN(5)
		}
		sense := assign.Minimize
		if idx%2 == 1 {
			sense = assign.Maximize
		}
		flat := make([]float64, rows*cols)
		mode := "exact"
		tol := 0.0
		if idx%3 == 0 {
			for i := range flat {
				flat[i] = float64(rng.IntN(201)-100) * 0.25
			}
		} else {
			mode = "safe"
			maxAbs := 1.0
			scale := math.Ldexp(1.0, rng.IntN(60)-30)
			for i := range flat {
				v := (rng.Float64() - 0.5) * 100.0 * scale
				flat[i] = v
				maxAbs = max(maxAbs, math.Abs(v))
			}
			tol = 1e-10 * maxAbs * float64(max(rows, cols))
		}
		res, err := assign.LinearSumFlat(t.Context(), rows, cols, flat, sense)
		if err != nil {
			t.Fatalf("LinearSumFlat %d: %v", idx, err)
		}
		raw, ok := buildFormalLSAPJSON(rows, cols, flat, sense, res, mode, tol)
		if !ok {
			t.Fatalf("buildFormalLSAPJSON %d failed", idx)
		}
		items = append(items, item{payload: raw, wantOK: true, isSafe: mode == "safe"})
		if len(validBatch) < 10 {
			validBatch = append(validBatch, raw)
		}
	}

	// Append 14 negative controls (duplicate col_ind and violated objective).
	badFlat := []float64{4, 1, 2, 0}
	badRes, err := assign.LinearSumFlat(t.Context(), 2, 2, badFlat, assign.Minimize)
	if err != nil {
		t.Fatalf("LinearSumFlat badRes: %v", err)
	}
	dupRes := badRes
	dupRes.ColInd = []int{0, 0}
	dupRaw, _ := buildFormalLSAPJSON(2, 2, badFlat, assign.Minimize, dupRes, "safe", 1.0)
	wrongObj := badRes
	wrongObj.Objective += 5.0
	wrongObjRaw, _ := buildFormalLSAPJSON(2, 2, badFlat, assign.Minimize, wrongObj, "exact", 0)
	for i := range 14 {
		if i%2 == 0 {
			items = append(items, item{payload: dupRaw, wantOK: false})
		} else {
			items = append(items, item{payload: wrongObjRaw, wantOK: false})
		}
	}

	const workers = 4
	errCh := make(chan error, len(items))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := workerID; i < len(items); i += workers {
				it := items[i]
				if i%5 == 0 {
					okCall, bndPair, detail := formalAssignOracleCall(t, it.payload)
					if okCall != it.wantOK {
						errCh <- fmt.Errorf("item %d tuple ok=%v, want %v (%s)", i, okCall, it.wantOK, detail)
						continue
					}
					if it.wantOK && it.isSafe && (bndPair[0] == "" || bndPair[1] == "") {
						errCh <- fmt.Errorf("item %d tuple missing safe bound", i)
					}
					continue
				}
				resp, cErr := callFormalAssignJSONL(t, it.payload)
				if cErr != nil {
					errCh <- fmt.Errorf("item %d transport error: %w", i, cErr)
					continue
				}
				if resp.OK != it.wantOK {
					errCh <- fmt.Errorf("item %d ok=%v, want %v (err=%q)", i, resp.OK, it.wantOK, resp.Error)
					continue
				}
				if it.wantOK && it.isSafe {
					if resp.Bound == nil {
						errCh <- fmt.Errorf("item %d missing safe bound", i)
						continue
					}
					if _, bErr := resp.Bound.bigRat(); bErr != nil {
						errCh <- fmt.Errorf("item %d invalid safe bound: %w", i, bErr)
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}

	// Verify lsap-check --batch smoke check (positive and negative).
	binPath := locateLSAPCheckBin(t)
	runBatch := func(batch [][]byte) (string, error) {
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, b := range batch {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(b)
		}
		buf.WriteByte(']')
		cmd := exec.CommandContext(t.Context(), binPath, "--batch", "-")
		cmd.Stdin = &buf
		out, bErr := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), bErr
	}
	outOK, bErr := runBatch(validBatch)
	wantBatch := fmt.Sprintf("VERIFIED (%d items)", len(validBatch))
	if bErr != nil || outOK != wantBatch {
		t.Fatalf("lsap-check --batch valid failed: err=%v out=%q want=%q", bErr, outOK, wantBatch)
	}
	badBatch := append(append([][]byte{}, validBatch[:2]...), dupRaw)
	outBad, errBad := runBatch(badBatch)
	if errBad == nil || !strings.Contains(outBad, "BATCH FAIL at index 2") {
		t.Fatalf("lsap-check --batch invalid expected index 2 failure: err=%v out=%q", errBad, outBad)
	}
}
