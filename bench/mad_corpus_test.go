package bench

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/internal/mad"
)

func TestMADCorpusSpecsAndTrial0Base(t *testing.T) {
	t.Parallel()

	specs := MADCorpusSpecs()
	if len(specs) != 13 {
		t.Fatalf("len(MADCorpusSpecs()) = %d, want 13", len(specs))
	}

	seen := make(map[string]bool, len(specs))
	for _, mc := range specs {
		if seen[mc.ID] {
			t.Fatalf("duplicate MAD corpus ID %q", mc.ID)
		}
		seen[mc.ID] = true
		if len(mc.TrialSeeds) != mc.Trials || mc.Trials <= 0 {
			t.Fatalf("%s invalid trial count %d / %d", mc.ID, mc.Trials, len(mc.TrialSeeds))
		}
		prob := GenerateMADProblem(mc, 0)
		if err := mad.ValidateProblem(prob); err != nil {
			t.Fatalf("%s trial 0 invalid problem: %v", mc.ID, err)
		}
		dfsStates, dpStates, dpWork := MADPlanMetrics(prob)
		if dfsStates <= 0 || dpStates <= 0 || dpWork <= 0 {
			t.Fatalf("%s invalid plan metrics: dfs=%d dpStates=%d dpWork=%d", mc.ID, dfsStates, dpStates, dpWork)
		}
		if MADDFSStates(prob) != dfsStates {
			t.Fatalf("%s MADDFSStates mismatch", mc.ID)
		}
	}

	// Verify canonical PMAD base Trial 0 produces exact 24137 and [6, 9].
	baseProb := GenerateMADProblem(specs[0], 0)
	res, err := mad.Solve(t.Context(), baseProb, mad.WithCertified(true))
	if err != nil {
		t.Fatalf("pmad_base_g10_i2 solve failed: %v", err)
	}
	if res.Objective != 24137 || len(res.Assignment) != 2 || res.Assignment[0] != 6 || res.Assignment[1] != 9 {
		t.Fatalf("pmad_base_g10_i2 = (obj=%v, assign=%v), want (24137, [6 9])", res.Objective, res.Assignment)
	}

	// Verify mad_comb_rand_g2_i4 matches t1_mad_rand_g2_i4 algebraic MILP objective.
	var randSpec MADCorpusCase
	for _, mc := range specs {
		if mc.ID == "mad_comb_rand_g2_i4" {
			randSpec = mc
			break
		}
	}
	randProb := GenerateMADProblem(randSpec, 0)
	randRes, err := mad.Solve(t.Context(), randProb, mad.WithCertified(true))
	if err != nil {
		t.Fatalf("mad_comb_rand_g2_i4 solve failed: %v", err)
	}
	milpModel := BuildMADRandom("t1_mad_rand_g2_i4", 2, 4, 6, 7000)
	milpRes, err := milpModel.Solve(context.Background(), gb.WithPureGo(), gb.WithCombinatorialSearch(false))
	if err != nil {
		t.Fatalf("BuildMADRandom MILP solve failed: %v", err)
	}
	if math.Abs(randRes.Objective-milpRes.Objective) > 1e-6 {
		t.Fatalf("mad_comb_rand_g2_i4 obj=%v vs BuildMADRandom MILP obj=%v", randRes.Objective, milpRes.Objective)
	}
	combRes, err := milpModel.Solve(context.Background(), gb.WithPureGo())
	if err != nil {
		t.Fatalf("BuildMADRandom default solve failed: %v", err)
	}
	if math.Abs(combRes.Objective-milpRes.Objective) > 1e-6 {
		t.Fatalf("BuildMADRandom default obj=%v vs MILP obj=%v", combRes.Objective, milpRes.Objective)
	}

	tmpDir := filepath.Join(t.TempDir(), "mad_corpus")
	written, err := WriteMADCorpusManifest(tmpDir)
	if err != nil {
		t.Fatalf("WriteMADCorpusManifest: %v", err)
	}
	if len(written) != 13 {
		t.Fatalf("written cases = %d, want 13", len(written))
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json missing: %v", err)
	}

	jsonlDir := filepath.Join(t.TempDir(), "mad_jsonl")
	exported, err := WriteMADCorpusTrialJSONL(jsonlDir)
	if err != nil {
		t.Fatalf("WriteMADCorpusTrialJSONL: %v", err)
	}
	if len(exported) != 13 {
		t.Fatalf("exported cases = %d, want 13", len(exported))
	}
	for _, mc := range exported {
		raw, rErr := os.ReadFile(filepath.Join(jsonlDir, mc.ID+".jsonl"))
		if rErr != nil {
			t.Fatalf("read %s.jsonl: %v", mc.ID, rErr)
		}
		lines := 0
		for _, b := range raw {
			if b == '\n' {
				lines++
			}
		}
		if lines != mc.Trials {
			t.Fatalf("%s.jsonl has %d lines, want %d", mc.ID, lines, mc.Trials)
		}
	}
}
