package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fingerprintInputs lists the sources that determine benchmark telemetry: the solver
// package, its format and kernel packages, the corpus definition, and the harness.
var fingerprintInputs = []struct {
	dir       string
	recursive bool
}{
	{dir: ".", recursive: false},
	{dir: "format", recursive: false},
	{dir: "internal", recursive: true},
	{dir: "bench", recursive: false},
	{dir: filepath.Join("cmd", "goblin-bench"), recursive: false},
}

// fingerprintExcluded lists bench files that only render documents and do not influence
// any measured value.
var fingerprintExcluded = map[string]bool{
	filepath.Join("bench", "docgen.go"):      true,
	filepath.Join("bench", "fingerprint.go"): true,
}

// SourceFingerprint returns a SHA-256 digest (first 16 hex digits) over go.mod and every
// non-test Go source that influences benchmark telemetry, relative to the module root.
// goblin-bench stores it in each telemetry entry, and the documentation gate rejects
// telemetry whose fingerprint differs from the current tree, so published timings always
// describe the code that is checked in.
func SourceFingerprint(root string) (string, error) {
	files := []string{"go.mod"}
	for _, in := range fingerprintInputs {
		base := filepath.Join(root, in.dir)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
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
			if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") && !fingerprintExcluded[rel] {
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

// CheckTelemetryFreshness fails when the goblin telemetry was produced by sources other than
// those currently under root.
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
			"re-run goblin-bench for all configurations and -sync-docs", pv.fingerprint, cur)
	}
	return nil
}
