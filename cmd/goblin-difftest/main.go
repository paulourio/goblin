// Command goblin-difftest solves every PuLP-JSON document of a JSONL file with goblin and writes
// one JSON result line per document. bench/difftest_cbc.py uses it to compare goblin with CBC.
//
// Input lines are objects {"id": string, "doc": <PuLP toDict() document>}. Output lines carry
// the status, objective, best bound, node count, wall time, goblin's own violation measures,
// and the solution keyed by variable name so that it can be verified independently.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
	"github.com/paulourio/goblin/format"
)

type result struct {
	ID     string             `json:"id"`
	Status string             `json:"status"`
	Obj    float64            `json:"obj"`
	Bound  float64            `json:"bound"`
	Nodes  int                `json:"nodes"`
	MS     float64            `json:"ms"`
	BViol  float64            `json:"bviol"`
	RViol  float64            `json:"rviol"`
	IViol  float64            `json:"iviol"`
	X      map[string]float64 `json:"x,omitempty"`
	Err    string             `json:"err,omitempty"`
}

const (
	defaultTimeLimitSec = 20
	exitCodeUsage       = 2
	scannerInitBufBytes = 1 << 20
	scannerMaxBufBytes  = 1 << 28
	usecPerMillisecond  = 1000
)

func main() {
	os.Exit(run())
}

type assignResult struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Obj    float64 `json:"obj"`
	RowInd []int   `json:"row_ind,omitempty"`
	ColInd []int   `json:"col_ind,omitempty"`
	Err    string  `json:"err,omitempty"`
}

func run() int {
	in := flag.String("in", "", "input JSONL file")
	assignMode := flag.Bool("assign", false, "solve Linear Sum Assignment JSONL records")
	backendFlag := flag.String("backend", "go", "solver backend ('go' or 'cbc')")
	noSIMD := flag.Bool("no-simd", false, "disable hardware SIMD kernels")
	timeLimit := flag.Duration("time-limit", defaultTimeLimitSec*time.Second, "per-instance time limit")
	noPresolve := flag.Bool("no-presolve", false, "disable presolve and node propagation")
	noCuts := flag.Bool("no-cuts", false, "disable cutting planes")
	noHeur := flag.Bool("no-heuristics", false, "disable primal heuristics")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: goblin-difftest -in instances.jsonl [-assign] [-backend go|cbc] [-no-simd] [-no-presolve] [-no-cuts] [-no-heuristics]")
		return exitCodeUsage
	}

	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = f.Close() }() // read-only file: a close error cannot lose data

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, scannerInitBufBytes), scannerMaxBufBytes)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	if *assignMode {
		var assignOpts []assign.Option
		if *noSIMD {
			assignOpts = append(assignOpts, assign.WithoutSIMD())
		} else {
			assignOpts = append(assignOpts, assign.WithSIMD(true))
		}
		for sc.Scan() {
			var line struct {
				ID    string `json:"id"`
				Rows  int    `json:"rows"`
				Cols  int    `json:"cols"`
				Sense int    `json:"sense"`
				Cost  []any  `json:"cost"`
			}
			if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if err := enc.Encode(solveAssign(line.ID, line.Rows, line.Cols, line.Sense, line.Cost, assignOpts...)); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		if err := sc.Err(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	opts := []gb.Option{
		gb.WithTimeLimit(*timeLimit),
		gb.WithPresolve(!*noPresolve),
		gb.WithCuts(!*noCuts),
		gb.WithHeuristics(!*noHeur),
	}
	if strings.EqualFold(*backendFlag, "cbc") {
		opts = append(opts, gb.WithCBC())
	} else {
		opts = append(opts, gb.WithPureGo())
	}
	if *noSIMD {
		opts = append(opts, gb.WithoutSIMD())
	}

	for sc.Scan() {
		var line struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := enc.Encode(solve(line.ID, line.Doc, opts...)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func solve(id string, raw json.RawMessage, opts ...gb.Option) result {
	r := result{ID: id, BViol: -1, RViol: -1, IViol: -1}
	doc, err := format.ParsePuLPJSON(raw)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	m, err := format.BuildModelFromPuLPJSON(doc)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	p, err := m.CompileChecked()
	if err != nil {
		r.Err = err.Error()
		return r
	}
	t0 := time.Now()
	res, err := gb.SolveCompiled(context.Background(), p, opts...)
	r.MS = float64(time.Since(t0).Microseconds()) / usecPerMillisecond
	if err != nil {
		r.Err = err.Error()
		r.Status = res.Status.String()
		return r
	}
	r.Status = res.Status.String()
	r.Obj = res.Objective
	r.Bound = res.BestBound
	r.Nodes = res.Nodes
	if len(res.Values) == p.NumVars {
		r.BViol, r.RViol, r.IViol = p.MaxViolation(res.Values, p.IsMIP)
		r.X = make(map[string]float64, p.NumVars)
		for j, name := range p.VarNames {
			r.X[name] = res.Values[j]
		}
	}
	return r
}

func solveAssign(id string, rows, cols, senseInt int, rawCost []any, opts ...assign.Option) assignResult {
	flat := make([]float64, len(rawCost))
	for i, item := range rawCost {
		switch v := item.(type) {
		case float64:
			flat[i] = v
		case string:
			switch v {
			case "+Inf", "Inf":
				flat[i] = math.Inf(1)
			case "-Inf":
				flat[i] = math.Inf(-1)
			default:
				flat[i] = math.NaN()
			}
		default:
			flat[i] = math.NaN()
		}
	}
	sense := assign.Minimize
	if senseInt < 0 {
		sense = assign.Maximize
	}
	res, err := assign.LinearSumFlat(context.Background(), rows, cols, flat, sense, opts...)
	if err != nil {
		if errors.Is(err, assign.ErrInfeasible) {
			return assignResult{ID: id, Status: "Infeasible"}
		}
		return assignResult{ID: id, Status: "Error", Err: err.Error()}
	}
	return assignResult{
		ID:     id,
		Status: "Optimal",
		Obj:    res.Objective,
		RowInd: res.RowInd,
		ColInd: res.ColInd,
	}
}
