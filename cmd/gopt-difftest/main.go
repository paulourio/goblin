// Command gopt-difftest solves every PuLP-JSON document of a JSONL file with gopt and writes
// one JSON result line per document. bench/difftest_cbc.py uses it to compare gopt with CBC.
//
// Input lines are objects {"id": string, "doc": <PuLP toDict() document>}. Output lines carry
// the status, objective, best bound, node count, wall time, gopt's own violation measures,
// and the solution keyed by variable name so that it can be verified independently.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gopt"
	"gopt/format"
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

func main() {
	in := flag.String("in", "", "input JSONL file")
	timeLimit := flag.Duration("time-limit", 20*time.Second, "per-instance time limit")
	noPresolve := flag.Bool("no-presolve", false, "disable presolve and node propagation")
	noCuts := flag.Bool("no-cuts", false, "disable cutting planes")
	noHeur := flag.Bool("no-heuristics", false, "disable primal heuristics")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: gopt-difftest -in instances.jsonl [-no-presolve] [-no-cuts] [-no-heuristics]")
		os.Exit(2)
	}

	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }() // read-only file: a close error cannot lose data

	opts := gopt.DefaultOptions()
	opts.TimeLimit = *timeLimit
	opts.EnablePresolve = !*noPresolve
	opts.EnableCuts = !*noCuts
	opts.EnableHeuristics = !*noHeur

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var line struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := enc.Encode(solve(line.ID, line.Doc, opts)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func solve(id string, raw json.RawMessage, opts gopt.Options) result {
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
	res := gopt.SolveCompiled(p, opts)
	r.MS = float64(time.Since(t0).Microseconds()) / 1000
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
