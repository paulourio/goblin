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
	"strconv"
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
	Mode   string           `json:"mode,omitzero"`
	Tol    *formalAssignRat `json:"tol,omitzero"`
	Error  string           `json:"error,omitzero"`
	Bound  *formalAssignRat `json:"bound,omitzero"`
}

type lsapBitsVal struct {
	Bits uint64 `json:"bits"`
}

type hallWitness struct {
	RowSide   bool
	Subset    []int
	Neighbors []int
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

func appendCompactBitsCostMatrix(buf []byte, rows, cols int, flatCost []float64, sense assign.Sense) ([]byte, error) {
	if len(flatCost) < rows*cols {
		return nil, fmt.Errorf("cost len %d < %d*%d", len(flatCost), rows, cols)
	}
	buf = append(buf, `"cost_encoding":"bits","cost":[`...)
	for r := range rows {
		if r > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '[')
		base := r * cols
		for c := range cols {
			if c > 0 {
				buf = append(buf, ',')
			}
			v := flatCost[base+c]
			if math.IsNaN(v) {
				return nil, fmt.Errorf("NaN cost at (%d,%d)", r, c)
			}
			if math.IsInf(v, 0) {
				if (sense == assign.Minimize && v < 0) || (sense == assign.Maximize && v > 0) {
					return nil, fmt.Errorf("wrong-sign infinity %v at (%d,%d) for sense %s", v, r, c, sense)
				}
				buf = append(buf, "null"...)
				continue
			}
			buf = strconv.AppendUint(buf, math.Float64bits(v), 10)
		}
		buf = append(buf, ']')
	}
	buf = append(buf, ']')
	return buf, nil
}

func appendCompactBitsVec(buf []byte, vals []float64) ([]byte, error) {
	buf = append(buf, '[')
	for i, v := range vals {
		if i > 0 {
			buf = append(buf, ',')
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("non-finite dual at %d: %v", i, v)
		}
		buf = strconv.AppendUint(buf, math.Float64bits(v), 10)
	}
	buf = append(buf, ']')
	return buf, nil
}

func appendIntVec(buf []byte, vals []int) []byte {
	buf = append(buf, '[')
	for i, v := range vals {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, int64(v), 10)
	}
	buf = append(buf, ']')
	return buf
}

func appendRatObj(buf []byte, r *big.Rat) []byte {
	if r == nil {
		return append(buf, `{"num":"0","den":"1"}`...)
	}
	buf = append(buf, `{"num":"`...)
	buf = append(buf, r.Num().String()...)
	buf = append(buf, `","den":"`...)
	buf = append(buf, r.Denom().String()...)
	buf = append(buf, `"}`...)
	return buf
}

func appendRatVec(buf []byte, vals []*big.Rat) []byte {
	buf = append(buf, '[')
	for i, r := range vals {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendRatObj(buf, r)
	}
	buf = append(buf, ']')
	return buf
}

func buildCompactFormalLSAPJSON(
	rows, cols int,
	flatCost []float64,
	sense assign.Sense,
	res assign.Result,
	mode string,
	tol float64,
	includeObjective bool,
) ([]byte, error) {
	if rows < 0 || cols < 0 || len(flatCost) < rows*cols {
		return nil, fmt.Errorf("invalid matrix dimensions %dx%d (len=%d)", rows, cols, len(flatCost))
	}
	if includeObjective && (math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0)) {
		return nil, fmt.Errorf("non-finite objective %v", res.Objective)
	}
	if mode == "safe" && (math.IsNaN(tol) || math.IsInf(tol, 0) || tol < 0) {
		return nil, fmt.Errorf("invalid safe tol %v", tol)
	}
	senseStr := "min"
	if sense == assign.Maximize {
		senseStr = "max"
	}
	estSize := rows*cols*20 + (rows+cols)*32 + 256
	buf := make([]byte, 0, estSize)
	buf = append(buf, `{"rows":`...)
	buf = strconv.AppendInt(buf, int64(rows), 10)
	buf = append(buf, `,"cols":`...)
	buf = strconv.AppendInt(buf, int64(cols), 10)
	buf = append(buf, `,"sense":"`...)
	buf = append(buf, senseStr...)
	buf = append(buf, `","mode":"`...)
	buf = append(buf, mode...)
	buf = append(buf, `","dual_encoding":"bits",`...)
	var err error
	buf, err = appendCompactBitsCostMatrix(buf, rows, cols, flatCost, sense)
	if err != nil {
		return nil, err
	}
	buf = append(buf, `,"row_ind":`...)
	buf = appendIntVec(buf, res.RowInd)
	buf = append(buf, `,"col_ind":`...)
	buf = appendIntVec(buf, res.ColInd)
	if includeObjective {
		buf = append(buf, `,"objective":{"bits":`...)
		buf = strconv.AppendUint(buf, math.Float64bits(res.Objective), 10)
		buf = append(buf, '}')
	}
	buf = append(buf, `,"u":`...)
	buf, err = appendCompactBitsVec(buf, res.U)
	if err != nil {
		return nil, err
	}
	buf = append(buf, `,"v":`...)
	buf, err = appendCompactBitsVec(buf, res.V)
	if err != nil {
		return nil, err
	}
	if mode == "safe" {
		buf = append(buf, `,"tol":{"bits":`...)
		buf = strconv.AppendUint(buf, math.Float64bits(tol), 10)
		buf = append(buf, '}')
	}
	buf = append(buf, '}')
	return buf, nil
}

// buildFormalLSAPJSON streams a compact bit-exact LSAP certificate payload for lsap-check.
// Finite cells encode as raw IEEE-754 uint64 bit patterns under cost_encoding="bits";
// forbidden cells (+Inf for Minimize, -Inf for Maximize) encode natively as null.
func buildFormalLSAPJSON(rows, cols int, flatCost []float64, sense assign.Sense, res assign.Result, mode string, tol float64) ([]byte, bool) {
	if len(flatCost) != rows*cols {
		return nil, false
	}
	raw, err := buildCompactFormalLSAPJSON(rows, cols, flatCost, sense, res, mode, tol, true)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func assignULPStep(v float64) float64 {
	av := math.Abs(v)
	if av == 0 {
		return math.SmallestNonzeroFloat64
	}
	if av >= math.MaxFloat64 {
		return math.MaxFloat64 - math.Nextafter(math.MaxFloat64, 0)
	}
	return math.Nextafter(av, math.Inf(1)) - av
}

func ceilRatToFloat64(r *big.Rat) float64 {
	if r == nil || r.Sign() <= 0 {
		return 0
	}
	f, exact := r.Float64()
	if math.IsInf(f, 1) {
		return math.MaxFloat64
	}
	if !exact {
		fr := new(big.Rat).SetFloat64(f)
		if fr == nil || fr.Cmp(r) < 0 {
			nxt := math.Nextafter(f, math.Inf(1))
			if math.IsInf(nxt, 1) {
				return math.MaxFloat64
			}
			return nxt
		}
	}
	return f
}

// buildExactRatNativeLSAPJSON streams a compact bit-exact LSAP exact-mode certificate with
// native null encoding for forbidden +/-Inf cells and arbitrary-precision rational duals.
func buildExactRatNativeLSAPJSON(
	rows, cols int,
	flatCost []float64,
	sense assign.Sense,
	rowInd, colInd []int,
	gotOrigRat *big.Rat,
	uOrigRat, vOrigRat []*big.Rat,
) ([]byte, error) {
	senseStr := "min"
	if sense == assign.Maximize {
		senseStr = "max"
	}
	estSize := rows*cols*20 + (rows+cols)*64 + 256
	buf := make([]byte, 0, estSize)
	buf = append(buf, `{"rows":`...)
	buf = strconv.AppendInt(buf, int64(rows), 10)
	buf = append(buf, `,"cols":`...)
	buf = strconv.AppendInt(buf, int64(cols), 10)
	buf = append(buf, `,"sense":"`...)
	buf = append(buf, senseStr...)
	buf = append(buf, `","mode":"exact",`...)
	var err error
	buf, err = appendCompactBitsCostMatrix(buf, rows, cols, flatCost, sense)
	if err != nil {
		return nil, err
	}
	buf = append(buf, `,"row_ind":`...)
	buf = appendIntVec(buf, rowInd)
	buf = append(buf, `,"col_ind":`...)
	buf = appendIntVec(buf, colInd)
	buf = append(buf, `,"objective":`...)
	buf = appendRatObj(buf, gotOrigRat)
	buf = append(buf, `,"u":`...)
	buf = appendRatVec(buf, uOrigRat)
	buf = append(buf, `,"v":`...)
	buf = appendRatVec(buf, vOrigRat)
	buf = append(buf, '}')
	return buf, nil
}

// buildHallLSAPJSON streams a compact Hall infeasibility certificate for lsap-check mode="hall".
func buildHallLSAPJSON(
	rows, cols int,
	flatCost []float64,
	sense assign.Sense,
	hw hallWitness,
) ([]byte, error) {
	senseStr := "min"
	if sense == assign.Maximize {
		senseStr = "max"
	}
	sideStr := "rows"
	if !hw.RowSide {
		sideStr = "cols"
	}
	estSize := rows*cols*8 + len(hw.Subset)*8 + 128
	buf := make([]byte, 0, estSize)
	buf = append(buf, `{"rows":`...)
	buf = strconv.AppendInt(buf, int64(rows), 10)
	buf = append(buf, `,"cols":`...)
	buf = strconv.AppendInt(buf, int64(cols), 10)
	buf = append(buf, `,"sense":"`...)
	buf = append(buf, senseStr...)
	buf = append(buf, `","mode":"hall","side":"`...)
	buf = append(buf, sideStr...)
	buf = append(buf, `",`...)
	var err error
	buf, err = appendCompactBitsCostMatrix(buf, rows, cols, flatCost, sense)
	if err != nil {
		return nil, err
	}
	buf = append(buf, `,"subset":`...)
	buf = appendIntVec(buf, hw.Subset)
	buf = append(buf, '}')
	return buf, nil
}

// checkFormalAssignResult verifies an LSAP solution with zero escape hatches across both
// finite and +Inf/-Inf forbidden-edge matrices of any size:
//  1. Validates bijection, finite matched edges, objective rounding, and tight ULP-proportional
//     dual feasibility and complementary slackness on the solver's returned (U, V) (0-tolerance
//     when exactMode is true).
//  2. Proves 0-ULP global optimality over Q via Bellman-Ford negative-cycle detection and
//     exact rational potential recovery on the finite-edge residual graph when needed.
//  3. Verifies the certificate via lsap-check --jsonl using native null forbidden-edge encoding
//     and compact bit-pattern streaming: mode="exact" (tol=0) on solver duals when exact, or
//     both mode="safe" on the solver's Result.V/Result.U and mode="exact" on recovered exact duals.
func checkFormalAssignResult(
	tb testing.TB,
	label string,
	rows, cols int,
	cost []float64,
	sense assign.Sense,
	res assign.Result,
	exactMode bool,
) error {
	tb.Helper()
	if !formalAssignOracleEnabled() {
		return errors.New("GOBLIN_FORMAL=1 is not enabled")
	}
	if rows <= 0 || cols <= 0 {
		if len(res.RowInd) != 0 || len(res.ColInd) != 0 || res.Objective != 0 {
			return fmt.Errorf("[%s] non-trivial result on empty %dx%d matrix", label, rows, cols)
		}
		return nil
	}
	if len(cost) < rows*cols {
		return fmt.Errorf("[%s] cost slice len=%d < %d*%d", label, len(cost), rows, cols)
	}
	m := min(rows, cols)
	n := max(rows, cols)
	transposed := rows > cols
	if len(res.RowInd) != m || len(res.ColInd) != m {
		return fmt.Errorf("[%s] matching length (%d,%d) != %d", label, len(res.RowInd), len(res.ColInd), m)
	}
	if len(res.U) != rows || len(res.V) != cols {
		return fmt.Errorf("[%s] dual lengths (%d,%d) != (%d,%d)", label, len(res.U), len(res.V), rows, cols)
	}
	if math.IsNaN(res.Objective) {
		return fmt.Errorf("[%s] NaN objective", label)
	}

	maxFiniteAbs := 0.0
	for idx := range rows * cols {
		v := cost[idx]
		if math.IsNaN(v) {
			return fmt.Errorf("[%s] NaN cost at index %d", label, idx)
		}
		if math.IsInf(v, 0) {
			if (sense == assign.Minimize && v < 0) || (sense == assign.Maximize && v > 0) {
				return fmt.Errorf("[%s] unbounded infinity %v for sense %s at index %d", label, v, sense, idx)
			}
		} else {
			maxFiniteAbs = max(maxFiniteAbs, math.Abs(v))
		}
	}

	maxDualAbs := 0.0
	for i, u := range res.U {
		if math.IsNaN(u) || math.IsInf(u, 0) {
			return fmt.Errorf("[%s] non-finite U[%d]=%v", label, i, u)
		}
		maxDualAbs = max(maxDualAbs, math.Abs(u))
	}
	for j, v := range res.V {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("[%s] non-finite V[%d]=%v", label, j, v)
		}
		maxDualAbs = max(maxDualAbs, math.Abs(v))
	}

	usedRow := make([]bool, rows)
	usedCol := make([]bool, cols)
	pNorm := make([]int, m)
	matchedN := make([]bool, n)
	prevRow := -1
	gotOrigRat := new(big.Rat)
	termRat := new(big.Rat)
	maxMatchedAbs := 0.0

	for k := range m {
		r, c := res.RowInd[k], res.ColInd[k]
		if r < 0 || r >= rows || c < 0 || c >= cols {
			return fmt.Errorf("[%s] out-of-range pair (%d,%d) for %dx%d", label, r, c, rows, cols)
		}
		if r <= prevRow {
			return fmt.Errorf("[%s] RowInd not strictly ascending at %d: %v", label, k, res.RowInd)
		}
		prevRow = r
		if usedRow[r] || usedCol[c] {
			return fmt.Errorf("[%s] duplicate row/col at (%d,%d)", label, r, c)
		}
		usedRow[r] = true
		usedCol[c] = true
		v := cost[r*cols+c]
		if math.IsInf(v, 0) || math.IsNaN(v) || termRat.SetFloat64(v) == nil {
			return fmt.Errorf("[%s] matched forbidden/invalid edge (%d,%d)=%v", label, r, c, v)
		}
		gotOrigRat.Add(gotOrigRat, termRat)
		maxMatchedAbs = max(maxMatchedAbs, math.Abs(v))
		if !transposed {
			pNorm[r] = c
			matchedN[c] = true
		} else {
			pNorm[c] = r
			matchedN[r] = true
		}
	}

	wantObjF, exactObjF := gotOrigRat.Float64()
	var objDiffFloat float64
	solverExact := true
	if gotOrigRat.Sign() == 0 {
		if math.Float64bits(res.Objective) != 0 {
			return fmt.Errorf("[%s] zero objective must be +0.0, got %g (%016x)",
				label, res.Objective, math.Float64bits(res.Objective))
		}
	} else if math.IsInf(wantObjF, 0) {
		solverExact = false
		if res.Objective != wantObjF {
			return fmt.Errorf("[%s] overflow objective got %v, want %v", label, res.Objective, wantObjF)
		}
	} else {
		if math.IsInf(res.Objective, 0) {
			return fmt.Errorf("[%s] unexpected infinite objective %v for finite sum %s",
				label, res.Objective, gotOrigRat.RatString())
		}
		objRat := new(big.Rat).SetFloat64(res.Objective)
		diffRat := new(big.Rat).Sub(gotOrigRat, objRat)
		diffRat.Abs(diffRat)
		if diffRat.Sign() != 0 {
			solverExact = false
			if exactMode && exactObjF {
				return fmt.Errorf("[%s] exact-mode objective mismatch: got %.17g, want %.17g",
					label, res.Objective, wantObjF)
			}
			objDiffFloat = ceilRatToFloat64(diffRat)
			ulpLim := max(4.0*math.SmallestNonzeroFloat64, 8.0*assignULPStep(max(math.Abs(wantObjF), maxMatchedAbs)))
			if math.Abs(res.Objective-wantObjF) > ulpLim {
				return fmt.Errorf("[%s] objective %.17g differs from exact sum %.17g by %g > %g",
					label, res.Objective, wantObjF, math.Abs(res.Objective-wantObjF), ulpLim)
			}
		}
	}

	var rawUNorm, rawVNorm []float64
	if !transposed {
		rawUNorm, rawVNorm = res.U, res.V
	} else {
		rawUNorm, rawVNorm = res.V, res.U
	}
	uSolverRat := make([]*big.Rat, m)
	vSolverRat := make([]*big.Rat, n)
	for i := range m {
		uSolverRat[i] = new(big.Rat).SetFloat64(rawUNorm[i])
	}
	for j := range n {
		vSolverRat[j] = new(big.Rat).SetFloat64(rawVNorm[j])
	}

	dimF := float64(n)
	if m < n {
		rectTol := max(8.0*math.SmallestNonzeroFloat64, dimF*64.0*assignULPStep(maxDualAbs))
		for j := range n {
			if vSolverRat[j].Sign() > 0 {
				solverExact = false
				if exactMode || rawVNorm[j] > rectTol {
					return fmt.Errorf("[%s] rectangular dual vNorm[%d]=%g > 0 (tol=%g)",
						label, j, rawVNorm[j], rectTol)
				}
			}
			if !matchedN[j] && vSolverRat[j].Sign() != 0 {
				solverExact = false
				if exactMode || math.Abs(rawVNorm[j]) > rectTol {
					return fmt.Errorf("[%s] unmatched rectangular dual vNorm[%d]=%g != 0 (tol=%g)",
						label, j, rawVNorm[j], rectTol)
				}
			}
		}
	}

	matchedCost := make([]*big.Rat, m)
	cRat := new(big.Rat)
	sumRat := new(big.Rat)
	diffRat := new(big.Rat)
	var tightRowTolSum float64

	globalScale := max(maxFiniteAbs, maxDualAbs)
	compSlackGlobalTol := max(8.0*math.SmallestNonzeroFloat64, dimF*64.0*assignULPStep(globalScale))

	for i := range m {
		rowMaxScale := math.Abs(rawUNorm[i])
		for j := range n {
			var origV float64
			if !transposed {
				origV = cost[i*cols+j]
			} else {
				origV = cost[j*cols+i]
			}
			if math.IsInf(origV, 0) {
				continue
			}
			normV := origV
			if sense == assign.Maximize {
				normV = -origV
			}
			cRat.SetFloat64(normV)
			if j == pNorm[i] {
				matchedCost[i] = new(big.Rat).Set(cRat)
			}

			edgeScale := max(math.Abs(normV), max(math.Abs(rawUNorm[i]), math.Abs(rawVNorm[j])))
			rowMaxScale = max(rowMaxScale, edgeScale)
			edgeTol := max(8.0*math.SmallestNonzeroFloat64, dimF*64.0*assignULPStep(edgeScale))

			sumRat.Add(uSolverRat[i], vSolverRat[j])
			if j == pNorm[i] {
				diffRat.Sub(sumRat, cRat)
				if diffRat.Sign() != 0 {
					solverExact = false
					if exactMode {
						return fmt.Errorf("[%s] exact complementary slackness violated at (%d,%d): diff=%s",
							label, i, j, diffRat.RatString())
					}
					csTol := edgeTol
					if diffRat.Sign() < 0 {
						csTol = max(edgeTol, compSlackGlobalTol)
					}
					diffRat.Abs(diffRat)
					if df, _ := diffRat.Float64(); df > csTol {
						return fmt.Errorf("[%s] complementary slackness diff=%g > ulpTol=%g at (%d,%d)",
							label, df, csTol, i, j)
					}
				}
			} else if sumRat.Cmp(cRat) > 0 {
				solverExact = false
				diffRat.Sub(sumRat, cRat)
				if exactMode {
					return fmt.Errorf("[%s] exact dual feasibility violated at (%d,%d): excess=%s",
						label, i, j, diffRat.RatString())
				}
				if exF, _ := diffRat.Float64(); exF > edgeTol {
					return fmt.Errorf("[%s] dual violation excess=%g > ulpTol=%g at (%d,%d)",
						label, exF, edgeTol, i, j)
				}
			}
		}
		if matchedCost[i] == nil {
			return fmt.Errorf("[%s] missing matched finite edge for normalized row %d", label, i)
		}
		tightRowTolSum += max(8.0*math.SmallestNonzeroFloat64, dimF*64.0*assignULPStep(rowMaxScale))
	}

	// Verify via lsap-check --jsonl with zero escape hatches (no size cap, native null forbidden cells).
	if solverExact {
		raw, rawErr := buildCompactFormalLSAPJSON(rows, cols, cost, sense, res, "exact", 0, !math.IsInf(res.Objective, 0))
		if rawErr != nil {
			return fmt.Errorf("[%s] buildCompactFormalLSAPJSON exact failed: %w", label, rawErr)
		}
		resp, callErr := callFormalAssignJSONL(tb, raw)
		if callErr != nil {
			return fmt.Errorf("[%s] lsap-check --jsonl exact call failed: %w", label, callErr)
		}
		if !resp.OK || resp.Mode != "exact" {
			return fmt.Errorf("[%s] lsap-check --jsonl exact rejected (resp=%+v)", label, resp)
		}
		return nil
	}

	// Recover exact rational potentials on the finite-edge residual graph for exact verification.
	finiteCols := make([][]int, m)
	finiteCRat := make([][]*big.Rat, m)
	for i := range m {
		fCols := make([]int, 0, n)
		fVals := make([]*big.Rat, 0, n)
		for j := range n {
			var origV float64
			if !transposed {
				origV = cost[i*cols+j]
			} else {
				origV = cost[j*cols+i]
			}
			if math.IsInf(origV, 0) {
				continue
			}
			normV := origV
			if sense == assign.Maximize {
				normV = -origV
			}
			fCols = append(fCols, j)
			fVals = append(fVals, new(big.Rat).SetFloat64(normV))
		}
		finiteCols[i] = fCols
		finiteCRat[i] = fVals
	}

	uExact, vExact, bfErr := recoverExactRatPotentialsBellmanFord(label, m, n, pNorm, matchedN, matchedCost, finiteCols, finiteCRat, vSolverRat)
	if bfErr != nil {
		return bfErr
	}

	var uOrigExact, vOrigExact []*big.Rat
	if !transposed {
		uOrigExact, vOrigExact = uExact, vExact
	} else {
		uOrigExact, vOrigExact = vExact, uExact
	}

	// 1. Check the solver's own Result.V (or Result.U when transposed) in lsap-check mode="safe"
	// on both finite and forbidden-edge matrices.
	safeTol := max(tightRowTolSum, float64(m)*compSlackGlobalTol) + objDiffFloat
	if math.IsInf(safeTol, 0) {
		safeTol = math.MaxFloat64
	}
	safeRes := res
	if m < n {
		if !transposed {
			vc := make([]float64, len(res.V))
			copy(vc, res.V)
			for j, val := range vc {
				if val > 0 {
					vc[j] = 0
				}
			}
			safeRes.V = vc
		} else {
			uc := make([]float64, len(res.U))
			copy(uc, res.U)
			for i, val := range uc {
				if val > 0 {
					uc[i] = 0
				}
			}
			safeRes.U = uc
		}
	}
	rawSafe, errSafe := buildCompactFormalLSAPJSON(rows, cols, cost, sense, safeRes, "safe", safeTol, !math.IsInf(res.Objective, 0))
	if errSafe != nil {
		return fmt.Errorf("[%s] failed to encode safe LSAP payload (tol=%g): %w", label, safeTol, errSafe)
	}
	respSafe, callSafeErr := callFormalAssignJSONL(tb, rawSafe)
	if callSafeErr != nil {
		return fmt.Errorf("[%s] lsap-check --jsonl safe (tol=%g) call failed: %w", label, safeTol, callSafeErr)
	}
	if !respSafe.OK || respSafe.Mode != "safe" {
		return fmt.Errorf("[%s] lsap-check --jsonl safe (tol=%g) rejected (resp=%+v)", label, safeTol, respSafe)
	}
	if respSafe.Bound != nil {
		bndRat, bErr := respSafe.Bound.bigRat()
		if bErr != nil {
			return fmt.Errorf("[%s] invalid safe bound %+v: %w", label, respSafe.Bound, bErr)
		}
		if sense == assign.Minimize && bndRat.Cmp(gotOrigRat) > 0 {
			return fmt.Errorf("[%s] safe lower bound %s > exact primal %s",
				label, bndRat.RatString(), gotOrigRat.RatString())
		}
		if sense == assign.Maximize && bndRat.Cmp(gotOrigRat) < 0 {
			return fmt.Errorf("[%s] safe upper bound %s < exact primal %s",
				label, bndRat.RatString(), gotOrigRat.RatString())
		}
	}

	// 2. Also verify Bellman-Ford exact rational potentials in mode="exact" (tol=0) on the
	// native bit-exact matrix (with null for forbidden cells, no big-M substitution).
	rawExactPol, errPol := buildExactRatNativeLSAPJSON(rows, cols, cost, sense, res.RowInd, res.ColInd, gotOrigRat, uOrigExact, vOrigExact)
	if errPol != nil {
		return fmt.Errorf("[%s] buildExactRatNativeLSAPJSON failed: %w", label, errPol)
	}
	respEx, callExErr := callFormalAssignJSONL(tb, rawExactPol)
	if callExErr != nil {
		return fmt.Errorf("[%s] lsap-check --jsonl exact (polished) call failed: %w", label, callExErr)
	}
	if !respEx.OK || respEx.Mode != "exact" {
		return fmt.Errorf("[%s] lsap-check --jsonl exact (polished) rejected (resp=%+v)", label, respEx)
	}
	return nil
}

func recoverExactRatPotentialsBellmanFord(
	label string,
	m, n int,
	pNorm []int,
	matchedN []bool,
	matchedCost []*big.Rat,
	finiteCols [][]int,
	finiteCRat [][]*big.Rat,
	vInit []*big.Rat,
) (uExact, vExact []*big.Rat, err error) {
	distCol := make([]*big.Rat, n)
	for j := range n {
		distCol[j] = new(big.Rat).Set(vInit[j])
		if m < n && (!matchedN[j] || distCol[j].Sign() > 0) {
			distCol[j].SetInt64(0)
		}
	}
	distVirt := new(big.Rat)
	distRow := make([]*big.Rat, m)
	for i := range m {
		distRow[i] = new(big.Rat).Sub(distCol[pNorm[i]], matchedCost[i])
	}

	cand := new(big.Rat)
	maxPasses := m + 3
	converged := false
	for range maxPasses {
		changed := false
		for i := range m {
			dr := distRow[i]
			colsI := finiteCols[i]
			valsI := finiteCRat[i]
			for k, j := range colsI {
				cand.Add(dr, valsI[k])
				if cand.Cmp(distCol[j]) < 0 {
					distCol[j].Set(cand)
					changed = true
				}
			}
		}
		if m < n {
			for j := range n {
				if !matchedN[j] && distCol[j].Cmp(distVirt) < 0 {
					distVirt.Set(distCol[j])
					changed = true
				}
			}
			for j := range n {
				if distVirt.Cmp(distCol[j]) < 0 {
					distCol[j].Set(distVirt)
					changed = true
				}
			}
		}
		for i := range m {
			cand.Sub(distCol[pNorm[i]], matchedCost[i])
			if cand.Cmp(distRow[i]) < 0 {
				distRow[i].Set(cand)
				changed = true
			}
		}
		if !changed {
			converged = true
			break
		}
	}
	if !converged {
		return nil, nil, fmt.Errorf("[%s] suboptimal LSAP matching: negative-cost augmenting cycle exists over Q", label)
	}

	uExact = make([]*big.Rat, m)
	vExact = make([]*big.Rat, n)
	if m < n {
		for i := range m {
			uExact[i] = new(big.Rat).Sub(distVirt, distRow[i])
		}
		for j := range n {
			vExact[j] = new(big.Rat).Sub(distCol[j], distVirt)
		}
	} else {
		for i := range m {
			uExact[i] = new(big.Rat).Neg(distRow[i])
		}
		for j := range n {
			vExact[j] = new(big.Rat).Set(distCol[j])
		}
	}

	// Independently verify all exact KKT conditions over Q with zero tolerance.
	if m < n {
		for j := range n {
			if vExact[j].Sign() > 0 {
				return nil, nil, fmt.Errorf("[%s] polished rectangular v[%d]=%s > 0", label, j, vExact[j].RatString())
			}
			if !matchedN[j] && vExact[j].Sign() != 0 {
				return nil, nil, fmt.Errorf("[%s] polished unmatched rectangular v[%d]=%s != 0", label, j, vExact[j].RatString())
			}
		}
	}
	sum := new(big.Rat)
	for i := range m {
		sum.Add(uExact[i], vExact[pNorm[i]])
		if sum.Cmp(matchedCost[i]) != 0 {
			return nil, nil, fmt.Errorf("[%s] polished complementary slackness failed at row %d", label, i)
		}
		for k, j := range finiteCols[i] {
			sum.Add(uExact[i], vExact[j])
			if sum.Cmp(finiteCRat[i][k]) > 0 {
				return nil, nil, fmt.Errorf("[%s] polished dual feasibility failed at (%d,%d)", label, i, j)
			}
		}
	}
	return uExact, vExact, nil
}

// extractAndVerifyHallWitness computes a maximum finite bipartite matching via Hopcroft-Karp
// and extracts an explicit Hall's Marriage Theorem witness S on the smaller side satisfying
// |N_finite(S)| < |S|, then independently verifies S from scratch directly on cost.
func extractAndVerifyHallWitness(rows, cols int, cost []float64, sense assign.Sense) (hallWitness, error) {
	if rows <= 0 || cols <= 0 {
		return hallWitness{}, fmt.Errorf("empty %dx%d matrix cannot be infeasible", rows, cols)
	}
	if len(cost) < rows*cols {
		return hallWitness{}, fmt.Errorf("cost len=%d < %d*%d", len(cost), rows, cols)
	}
	rowSide := rows <= cols
	m := min(rows, cols)
	n := max(rows, cols)

	adj := make([][]int, m)
	for r := range rows {
		base := r * cols
		for c := range cols {
			v := cost[base+c]
			if math.IsNaN(v) {
				return hallWitness{}, fmt.Errorf("NaN cost at (%d,%d)", r, c)
			}
			if math.IsInf(v, 0) {
				if (sense == assign.Minimize && v < 0) || (sense == assign.Maximize && v > 0) {
					return hallWitness{}, fmt.Errorf("wrong-sign infinity %v for %s at (%d,%d)", v, sense, r, c)
				}
				continue
			}
			if rowSide {
				adj[r] = append(adj[r], c)
			} else {
				adj[c] = append(adj[c], r)
			}
		}
	}

	matchL := make([]int, m)
	for u := range m {
		matchL[u] = -1
	}
	matchR := make([]int, n)
	for v := range n {
		matchR[v] = -1
	}

	dist := make([]int, m)
	queue := make([]int, 0, m)
	ptr := make([]int, m)
	matched := 0

	bfs := func() bool {
		queue = queue[:0]
		foundFree := false
		for u := range m {
			if matchL[u] < 0 {
				dist[u] = 0
				queue = append(queue, u)
			} else {
				dist[u] = -1
			}
		}
		for head := 0; head < len(queue); head++ {
			u := queue[head]
			for _, v := range adj[u] {
				u2 := matchR[v]
				if u2 < 0 {
					foundFree = true
				} else if dist[u2] < 0 {
					dist[u2] = dist[u] + 1
					queue = append(queue, u2)
				}
			}
		}
		return foundFree
	}

	var dfs func(u int) bool
	dfs = func(u int) bool {
		edges := adj[u]
		for ptr[u] < len(edges) {
			v := edges[ptr[u]]
			ptr[u]++
			u2 := matchR[v]
			if u2 < 0 || (dist[u2] == dist[u]+1 && dfs(u2)) {
				matchL[u] = v
				matchR[v] = u
				return true
			}
		}
		dist[u] = -1
		return false
	}

	for bfs() {
		clear(ptr)
		for u := range m {
			if matchL[u] < 0 && dfs(u) {
				matched++
			}
		}
	}
	if matched == m {
		return hallWitness{}, fmt.Errorf("false ErrInfeasible: complete finite matching of size %d exists", m)
	}

	inS := make([]bool, m)
	inN := make([]bool, n)
	queue = queue[:0]
	for u := range m {
		if matchL[u] == -1 {
			inS[u] = true
			queue = append(queue, u)
		}
	}
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		for _, v := range adj[u] {
			if inN[v] {
				continue
			}
			inN[v] = true
			uNext := matchR[v]
			if uNext < 0 {
				return hallWitness{}, errors.New("internal augmenting path reached free R-vertex")
			}
			if !inS[uNext] {
				inS[uNext] = true
				queue = append(queue, uNext)
			}
		}
	}

	subset := make([]int, 0, len(queue))
	for u := range m {
		if inS[u] {
			subset = append(subset, u)
		}
	}
	if len(subset) == 0 {
		return hallWitness{}, errors.New("empty Hall witness subset")
	}

	hasNeigh := make([]bool, n)
	neighbors := make([]int, 0, len(subset))
	for _, u := range subset {
		for _, v := range adj[u] {
			if !hasNeigh[v] {
				hasNeigh[v] = true
				neighbors = append(neighbors, v)
			}
		}
	}
	if len(neighbors) >= len(subset) {
		return hallWitness{}, fmt.Errorf("invalid Hall witness: |N_finite(S)|=%d >= |S|=%d", len(neighbors), len(subset))
	}
	return hallWitness{
		RowSide:   rowSide,
		Subset:    subset,
		Neighbors: neighbors,
	}, nil
}

func verifyFormalAssignInfeasible(tb testing.TB, label string, rows, cols int, cost []float64, sense assign.Sense) {
	tb.Helper()
	hw, err := extractAndVerifyHallWitness(rows, cols, cost, sense)
	if err != nil {
		tb.Fatalf("[%s] %v", label, err)
	}
	if !formalAssignOracleEnabled() {
		return
	}
	rawHall, encErr := buildHallLSAPJSON(rows, cols, cost, sense, hw)
	if encErr != nil {
		tb.Fatalf("[%s] buildHallLSAPJSON failed: %v", label, encErr)
	}
	resp, callErr := callFormalAssignJSONL(tb, rawHall)
	if callErr != nil || !resp.OK || resp.Mode != "hall" {
		tb.Fatalf("[%s] lsap-check --jsonl hall rejected (err=%v resp=%+v)", label, callErr, resp)
	}
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
	// Forbidden +Inf on Minimize encodes natively as null; wrong-sign -Inf and NaN must be rejected.
	rawForb, okForb := buildFormalLSAPJSON(1, 1, []float64{math.Inf(1)}, assign.Minimize, res, "safe", 1e-9)
	if !okForb || !strings.Contains(string(rawForb), "null") {
		t.Fatalf("expected buildFormalLSAPJSON to encode forbidden +Inf natively as null, got ok=%v raw=%s", okForb, rawForb)
	}
	if _, okWrongInf := buildFormalLSAPJSON(1, 1, []float64{math.Inf(-1)}, assign.Minimize, res, "safe", 1e-9); okWrongInf {
		t.Fatal("expected buildFormalLSAPJSON to reject wrong-sign -Inf under Minimize")
	}
	if _, okNaN := buildFormalLSAPJSON(1, 1, []float64{math.NaN()}, assign.Minimize, res, "safe", 1e-9); okNaN {
		t.Fatal("expected buildFormalLSAPJSON to reject NaN")
	}
}

func TestFormalAssignZeroEscapeControls(t *testing.T) {
	t.Parallel()
	if !formalAssignOracleEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run zero-escape LSAP formal checks")
	}

	// 1. Positive control: forbidden-edge square, wide, and tall matrices across Minimize and Maximize.
	inf := math.Inf(1)
	ninf := math.Inf(-1)
	posCostMin := []float64{
		4.0, inf, 1.5, 9.0,
		2.0, 3.0, inf, 5.0,
		inf, 1.0, 2.5, inf,
	}
	resPosMin, err := assign.LinearSumFlat(t.Context(), 3, 4, posCostMin, assign.Minimize)
	if err != nil {
		t.Fatalf("unexpected error on feasible forbidden-edge LSAP: %v", err)
	}
	if err := checkFormalAssignResult(t, "pos_forbid_min", 3, 4, posCostMin, assign.Minimize, resPosMin, true); err != nil {
		t.Fatalf("expected feasible forbidden-edge LSAP to certify cleanly, got: %v", err)
	}

	posCostMax := []float64{
		10.0, ninf,
		4.0, 8.0,
		ninf, 7.0,
	}
	resPosMax, err := assign.LinearSumFlat(t.Context(), 3, 2, posCostMax, assign.Maximize)
	if err != nil {
		t.Fatalf("unexpected error on feasible tall Maximize LSAP: %v", err)
	}
	if err := checkFormalAssignResult(t, "pos_forbid_max", 3, 2, posCostMax, assign.Maximize, resPosMax, true); err != nil {
		t.Fatalf("expected feasible tall Maximize LSAP to certify cleanly, got: %v", err)
	}

	// 2. Negative control A: 1-ULP suboptimal matching on a +Inf forbidden-edge square matrix,
	// even when Duals are crafted within 1 ULP to try spoofing loose float tolerances.
	eps := assignULPStep(1.0)
	subCostSq := []float64{
		1.0, 1.0, inf,
		1.0, 1.0 - eps, 5.0,
		inf, 5.0, 2.0,
	}
	// Optimal matching is (0->0, 1->1, 2->2) with cost = 4.0 - eps;
	// suboptimal matching (0->1, 1->0, 2->2) has cost = 4.0 (1 ULP worse) with spoofed duals u=[1,1,2], v=[0,0,0].
	spoofSq := assign.Result{
		RowInd:    []int{0, 1, 2},
		ColInd:    []int{1, 0, 2},
		Objective: 4.0,
		U:         []float64{1.0, 1.0, 2.0},
		V:         []float64{0.0, 0.0, 0.0},
	}
	if err := checkFormalAssignResult(t, "neg_1ulp_forbid_sq", 3, 3, subCostSq, assign.Minimize, spoofSq, false); err == nil {
		t.Fatal("expected 1-ULP suboptimal forbidden-edge square LSAP to fail closed")
	}

	// 3. Negative control B: 1-ULP suboptimal matching on a rectangular +Inf forbidden-edge matrix.
	subCostRect := []float64{
		1.0, 10.0, inf,
		inf, 1.0, 1.0 - eps,
	}
	spoofRect := assign.Result{
		RowInd:    []int{0, 1},
		ColInd:    []int{0, 1},
		Objective: 2.0,
		U:         []float64{1.0, 1.0},
		V:         []float64{0.0, 0.0, 0.0},
	}
	if err := checkFormalAssignResult(t, "neg_1ulp_forbid_rect", 2, 3, subCostRect, assign.Minimize, spoofRect, false); err == nil {
		t.Fatal("expected 1-ULP suboptimal rectangular forbidden-edge LSAP to fail closed")
	}

	// 4. Negative control C: exactMode rejects even 1-ULP dual slack perturbation.
	badExact := resPosMin
	badExact.U = append([]float64(nil), resPosMin.U...)
	badExact.U[0] += assignULPStep(badExact.U[0])
	if err := checkFormalAssignResult(t, "neg_exact_1ulp_dual", 3, 4, posCostMin, assign.Minimize, badExact, true); err == nil {
		t.Fatal("expected exactMode to reject 1-ULP dual slack perturbation")
	}

	// 5. Negative control D: safe mode tightened ULP bound rejects 1e-11 dual error (which passed old 1e-9 tol).
	finCost := []float64{1.0, 2.0, 3.0, 1.0}
	resFin, err := assign.LinearSumFlat(t.Context(), 2, 2, finCost, assign.Minimize)
	if err != nil {
		t.Fatalf("LinearSumFlat: %v", err)
	}
	badSafe := resFin
	badSafe.U = append([]float64(nil), resFin.U...)
	badSafe.U[0] += 1e-11
	if err := checkFormalAssignResult(t, "neg_safe_loose_tol", 2, 2, finCost, assign.Minimize, badSafe, false); err == nil {
		t.Fatal("expected tightened ULP safe check to reject 1e-11 dual drift")
	}

	// 6. Positive & Negative control E: Hall's Marriage Theorem witness verified directly via lsap-check mode="hall".
	infeasHall := []float64{
		1.0, inf, inf,
		2.0, inf, inf,
		inf, 3.0, 4.0,
	}
	verifyFormalAssignInfeasible(t, "pos_hall_sq", 3, 3, infeasHall, assign.Minimize)
	if _, falseErr := extractAndVerifyHallWitness(3, 4, posCostMin, assign.Minimize); falseErr == nil {
		t.Fatal("expected false ErrInfeasible on feasible forbidden-edge matrix to fail closed")
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
			tol = 64.0 * assignULPStep(maxAbs) * float64(rows*cols)
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
