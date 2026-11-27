package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fingerprintInputs lists the directories whose sources and benchmark corpus files
// determine benchmark telemetry across Go, C++ CBC, and Python harnesses.
var fingerprintInputs = []struct {
	dir       string
	recursive bool
	optional  bool
}{
	{dir: ".", recursive: false},
	{dir: "assign", recursive: false},
	{dir: "format", recursive: false},
	{dir: "internal", recursive: true},
	{dir: "bench", recursive: false},
	{dir: filepath.Join("cmd", "goblin-bench"), recursive: false},
	{dir: filepath.Join("testdata", "corpus"), recursive: true, optional: true},
	{dir: filepath.Join("testdata", "assign_corpus"), recursive: false, optional: true},
	{dir: filepath.Join("testdata", "mad_corpus"), recursive: false, optional: true},
}

// fingerprintExcluded lists files that render documents or run standalone fuzz/diff checks
// without influencing saved benchmark telemetry.
var fingerprintExcluded = map[string]bool{
	filepath.Join("bench", "docgen.go"):          true,
	filepath.Join("bench", "fingerprint.go"):     true,
	filepath.Join("bench", "difftest_cbc.py"):    true,
	filepath.Join("bench", "difftest_assign.py"): true,
}

func isFingerprintSource(rel string) bool {
	if fingerprintExcluded[rel] {
		return false
	}
	if strings.HasPrefix(rel, filepath.Join("testdata", "corpus")+string(filepath.Separator)) {
		return strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, ".jsonl")
	}
	if rel == filepath.Join("testdata", "assign_corpus", "manifest.json") ||
		rel == filepath.Join("testdata", "mad_corpus", "manifest.json") {
		return true
	}
	if strings.HasSuffix(rel, ".go") {
		return !strings.HasSuffix(rel, "_test.go")
	}
	if strings.HasSuffix(rel, ".py") {
		return strings.HasPrefix(rel, "bench"+string(filepath.Separator))
	}
	return strings.HasSuffix(rel, ".c") || strings.HasSuffix(rel, ".cpp") || strings.HasSuffix(rel, ".h")
}

// SourceFingerprint returns a SHA-256 digest (first 16 hex digits) over go.mod, every
// non-test Go, C/C++ bridge, or Python baseline harness file, and every benchmark corpus
// payload in testdata/corpus, testdata/assign_corpus, and testdata/mad_corpus, relative to
// root. All Go and Python benchmark harnesses store this digest in every telemetry entry, and
// the documentation gate rejects telemetry whose fingerprint differs from the current tree.
func SourceFingerprint(root string) (string, error) {
	files := []string{"go.mod"}
	for _, in := range fingerprintInputs {
		base := filepath.Join(root, in.dir)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if path == base && in.optional && errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				if path != base && !in.recursive {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if isFingerprintSource(rel) {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("fingerprint %s: %w", base, err)
		}
	}
	slices.Sort(files)
	h := sha256.New()
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		// hash.Hash.Write never returns an error.
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// CheckTelemetryFreshness fails when any benchmark telemetry file was produced from
// sources, harness scripts, or corpus payloads other than those currently under root.
func CheckTelemetryFreshness(root string, tb *TelemetryBundle) error {
	pv, err := telemetryProvenance(tb)
	if err != nil {
		return err
	}
	cur, err := SourceFingerprint(root)
	if err != nil {
		return err
	}
	if pv.fingerprint != cur {
		return fmt.Errorf("benchmark telemetry is stale: measured with source fingerprint %s, current tree is %s; "+
			"re-run benchmarks for all configurations and -sync-docs", pv.fingerprint, cur)
	}
	return nil
}
