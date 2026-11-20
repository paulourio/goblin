package goblin_test

import (
	"encoding/json/v2"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/format"
)

type publicInstanceMeta struct {
	ID                 string  `json:"id"`
	Collection         string  `json:"collection"`
	UpstreamName       string  `json:"upstream_name"`
	IsMIP              bool    `json:"is_mip"`
	NumVars            int     `json:"num_vars"`
	NumConstraints     int     `json:"num_constraints"`
	ReferenceObjective float64 `json:"reference_objective"`
	GoMaxNodes         int     `json:"go_max_nodes,omitzero"`
}

// Verifies status, objective parity, and primal feasibility on held-out public Netlib LP
// and MIPLIB / COIN-OR MILP benchmark instances stored in testdata/public.
func TestPublicNetlibAndMIPLIBSuite(t *testing.T) {
	rawManifest, err := os.ReadFile(filepath.Join("testdata", "public", "manifest.json"))
	if err != nil {
		t.Fatalf("read public manifest: %v", err)
	}
	var manifest []publicInstanceMeta
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("unmarshal public manifest: %v", err)
	}
	if len(manifest) < 10 {
		t.Fatalf("expected at least 10 public benchmark instances, got %d", len(manifest))
	}

	for _, inst := range manifest {
		t.Run(inst.ID, func(t *testing.T) {
			if testing.Short() && inst.IsMIP && (inst.NumVars > 200 || inst.ID == "miplib_flugpl") {
				t.Skip("skipping larger-tree public MIP under -short")
			}
			f, err := os.Open(filepath.Join("testdata", "public", inst.ID+".json"))
			if err != nil {
				t.Fatalf("open %s: %v", inst.ID, err)
			}
			m, err := format.FromPuLPJSON(f)
			_ = f.Close()
			if err != nil {
				t.Fatalf("FromPuLPJSON(%s): %v", inst.ID, err)
			}
			p, err := m.CompileChecked()
			if err != nil {
				t.Fatalf("CompileChecked(%s): %v", inst.ID, err)
			}
			if p.NumVars() != inst.NumVars || p.NumConstraints() != inst.NumConstraints {
				t.Fatalf("%s dimensions = (%d, %d), want (%d, %d)",
					inst.ID, p.NumVars(), p.NumConstraints(), inst.NumVars, inst.NumConstraints)
			}

			limit := 30 * time.Second
			if raceEnabled {
				limit = 120 * time.Second
			}

			for _, cfg := range []struct {
				name string
				opts []gb.Option
			}{
				{"go_default", []gb.Option{gb.WithTimeLimit(limit)}},
				{"go_scalar", []gb.Option{gb.WithoutSIMD(), gb.WithTimeLimit(limit)}},
			} {
				opts := cfg.opts
				if inst.GoMaxNodes > 0 {
					opts = append(append([]gb.Option(nil), opts...), gb.WithCuts(false), gb.WithHeuristics(false), gb.WithMaxNodes(inst.GoMaxNodes))
				}
				res, err := gb.SolveCompiled(t.Context(), p, opts...)
				if err != nil {
					t.Fatalf("[%s/%s] SolveCompiled error: %v", inst.ID, cfg.name, err)
				}
				if inst.GoMaxNodes > 0 && res.Status == gb.StatusNodeLimit {
					boundTol := 1e-4 * max(1.0, math.Abs(inst.ReferenceObjective))
					if p.Sense() == gb.Minimize && res.BestBound > inst.ReferenceObjective+boundTol {
						t.Fatalf("[%s/%s] invalid lower bound %.10g > reference %.10g",
							inst.ID, cfg.name, res.BestBound, inst.ReferenceObjective)
					}
					if len(res.Values) == p.NumVars() {
						bV, rV, iV := p.MaxViolation(res.Values, inst.IsMIP)
						if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
							t.Fatalf("[%s/%s] node-limit incumbent violation: bound=%.2e row=%.2e int=%.2e",
								inst.ID, cfg.name, bV, rV, iV)
						}
					}
					continue
				}
				if res.Status != gb.StatusOptimal {
					t.Fatalf("[%s/%s] status = %v, want Optimal (collection=%s)",
						inst.ID, cfg.name, res.Status, inst.Collection)
				}
				rel := math.Abs(res.Objective-inst.ReferenceObjective) / max(1.0, math.Abs(inst.ReferenceObjective))
				if rel > 1e-5 {
					t.Fatalf("[%s/%s] objective = %.10g, want %.10g (rel=%.2e)",
						inst.ID, cfg.name, res.Objective, inst.ReferenceObjective, rel)
				}
				bV, rV, iV := p.MaxViolation(res.Values, inst.IsMIP)
				if bV > 1e-5 || rV > 1e-5 || iV > 1e-5 {
					t.Fatalf("[%s/%s] primal violation: bound=%.2e row=%.2e int=%.2e",
						inst.ID, cfg.name, bV, rV, iV)
				}
			}

			if gb.CBCAvailable() {
				resCBC, err := gb.SolveCompiled(t.Context(), p, gb.WithCBC(), gb.WithTimeLimit(limit))
				if err != nil || resCBC.Status != gb.StatusOptimal {
					t.Fatalf("[%s/cbc] status=%v err=%v", inst.ID, resCBC.Status, err)
				}
				rel := math.Abs(resCBC.Objective-inst.ReferenceObjective) / max(1.0, math.Abs(inst.ReferenceObjective))
				if rel > 1e-5 {
					t.Fatalf("[%s/cbc] objective = %.10g, want %.10g (rel=%.2e)",
						inst.ID, resCBC.Objective, inst.ReferenceObjective, rel)
				}
			}
		})
	}
}
