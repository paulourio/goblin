// Command goblin-difftest solves every PuLP-JSON document of a JSONL file with goblin and writes
// one JSON result line per document. bench/difftest_cbc.py uses it to compare goblin with CBC.
//
// Input lines are objects {"id": string, "doc": <PuLP toDict() document>}. Output lines carry
// the status, objective, best bound, node count, wall time, goblin's own violation measures,
// and the solution keyed by variable name so that it can be verified independently.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/paulourio/goblin"
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

func run() int {
	in := flag.String("in", "", "input JSONL file")
	backendFlag := flag.String("backend", "go", "solver backend ('go' or 'cbc')")
	noSIMD := flag.Bool("no-simd", false, "disable hardware SIMD kernels")
	timeLimit := flag.Duration("time-limit", defaultTimeLimitSec*time.Second, "per-instance time limit")
	noPresolve := flag.Bool("no-presolve", false, "disable presolve and node propagation")
	noCuts := flag.Bool("no-cuts", false, "disable cutting planes")
	noHeur := flag.Bool("no-heuristics", false, "disable primal heuristics")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: goblin-difftest -in instances.jsonl [-backend go|cbc] [-no-simd] [-no-presolve] [-no-cuts] [-no-heuristics]")
		return exitCodeUsage
	}

	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = f.Close() }() // read-only file: a close error cannot lose data

	opts := []goblin.Option{
		goblin.WithTimeLimit(*timeLimit),
		goblin.WithPresolve(!*noPresolve),
		goblin.WithCuts(!*noCuts),
		goblin.WithHeuristics(!*noHeur),
	}
	if strings.EqualFold(*backendFlag, "cbc") {
		opts = append(opts, goblin.WithCBC())
	} else {
		opts = append(opts, goblin.WithPureGo())
	}
	if *noSIMD {
		opts = append(opts, goblin.WithoutSIMD())
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, scannerInitBufBytes), scannerMaxBufBytes)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
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

func solve(id string, raw json.RawMessage, opts ...goblin.Option) result {
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
	res := goblin.SolveCompiled(p, opts...)
	r.MS = float64(time.Since(t0).Microseconds()) / usecPerMillisecond
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
