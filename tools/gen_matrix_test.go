package main

// Offline assertions for the gen-matrix subcommand (porting the retired
// bash grid suite). Every number here is a deliberate contract: if a
// change moves a count, this file is the place the change gets argued about.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var allKeys = []string{
	"mc121_java21", "mc121_java25", "mc121_java26", "mc26_java25", "mc26_java26",
	"t0_java21", "t0_java25", "t0_java26",
	"mc1192_java17", "mc1192_java21",
	"mc114_java8", "mc114_java17", "mc114_java21",
}

// runGrid runs genMatrix against repoRoot and returns the stdout summary and
// the emitted name->json map (the GITHUB_OUTPUT lines).
func runGrid(t *testing.T, repoRoot, event, bands string) (string, map[string]string) {
	t.Helper()
	var stdout, ghOut bytes.Buffer
	if err := genMatrix(repoRoot, event, bands, &stdout, &ghOut); err != nil {
		t.Fatalf("genMatrix(%q, %q): %v", event, bands, err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(ghOut.String(), "\n"), "\n") {
		name, val, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed output line %q", line)
		}
		out[name] = val
	}
	return stdout.String(), out
}

// emptyRoot is a fixture tree with no band shipped, so FORCE_BANDS fully
// controls presence and the tests are independent of the real repo's state.
func emptyRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func versionsOf(t *testing.T, jsonVal string) []string {
	t.Helper()
	var v []string
	if err := json.Unmarshal([]byte(jsonVal), &v); err != nil {
		t.Fatalf("value %q is not a JSON string array: %v", jsonVal, err)
	}
	return v
}

// --- expected per-submatrix job counts (band forced present) ---------------
// Era-correct floors (2026-08-17 addendum): each version on its own floor JVM
// (exhaustive on both triggers), plus newest-Java coverage rows — band ends
// when lean, whole band when full.
var expected = map[string]map[string]int{
	// mainstream: 1.21.x floor 21 (canary 1.21.11 moved to gate), coverage 25/26
	"mc121_java21": {"pull_request": 11, "workflow_dispatch": 11},
	"mc121_java25": {"pull_request": 2, "workflow_dispatch": 12},
	"mc121_java26": {"pull_request": 2, "workflow_dispatch": 12},
	// 26.x floor 25 (canary 26.2 moved to gate)
	"mc26_java25": {"pull_request": 1, "workflow_dispatch": 1},
	"mc26_java26": {"pull_request": 2, "workflow_dispatch": 2},
	// t0 = 1.20.3-1.20.6 (Option A): floor 21, coverage 25/26
	"t0_java21": {"pull_request": 4, "workflow_dispatch": 4},
	"t0_java25": {"pull_request": 2, "workflow_dispatch": 4},
	"t0_java26": {"pull_request": 2, "workflow_dispatch": 4},
	// mc1192 = 1.19.2 1.19.4 1.20.1 1.20.2 (Option B): floor 17, coverage 21
	// only (pre-1.20.3 bands have no 25/26 rows).
	"mc1192_java17": {"pull_request": 4, "workflow_dispatch": 4},
	"mc1192_java21": {"pull_request": 2, "workflow_dispatch": 4},
	// mc114 = 1.14.4 1.15.2 1.16.5 | 1.17.1 1.18.2 (Option C): split floors
	// 8 / 17, coverage 21 across the whole band.
	"mc114_java8":  {"pull_request": 3, "workflow_dispatch": 3},
	"mc114_java17": {"pull_request": 2, "workflow_dispatch": 2},
	"mc114_java21": {"pull_request": 2, "workflow_dispatch": 5},
}

const allBands = "t0 mc1192 mc114"

// --- 1. every output key exists on every combination, exact band lists -----
func TestKeysAlwaysPresentAndBandLists(t *testing.T) {
	_, out := runGrid(t, emptyRoot(t), "pull_request", allBands)
	for _, name := range allKeys {
		if _, ok := out[name]; !ok {
			t.Errorf("%s: key missing entirely", name)
		}
	}
	for name, want := range map[string]string{
		"t0_java21":     `["1.20.3","1.20.4","1.20.5","1.20.6"]`,
		"mc1192_java17": `["1.19.2","1.19.4","1.20.1","1.20.2"]`,
		"mc114_java8":   `["1.14.4","1.15.2","1.16.5"]`,
		"mc114_java17":  `["1.17.1","1.18.2"]`,
	} {
		if out[name] != want {
			t.Errorf("%s = %s, want %s", name, out[name], want)
		}
	}
}

// Absent bands still emit every key, with the literal []. A missing GitHub
// output evaluates to the empty string, which is != '[]' and would feed
// fromJSON of an empty string to a matrix and hard-error the run.
func TestAbsentBandsEmitEmptyArrayLiteral(t *testing.T) {
	for _, event := range []string{"pull_request", "workflow_dispatch"} {
		_, out := runGrid(t, emptyRoot(t), event, "")
		for _, name := range []string{"t0_java21", "t0_java25", "t0_java26",
			"mc1192_java17", "mc1192_java21",
			"mc114_java8", "mc114_java17", "mc114_java21"} {
			if got, ok := out[name]; !ok || got != "[]" {
				t.Errorf("[%s] %s = %q, want the literal []", event, name, got)
			}
		}
	}
}

// --- 2+4. per-submatrix counts and gated-pair totals (all bands present) ---
func TestSubmatrixCountsAndTotals(t *testing.T) {
	totals := map[string]int{"pull_request": 39, "workflow_dispatch": 68}
	for _, event := range []string{"pull_request", "workflow_dispatch"} {
		stdout, out := runGrid(t, emptyRoot(t), event, allBands)
		total := 0
		for _, name := range allKeys {
			n := len(versionsOf(t, out[name]))
			total += n
			if n != expected[name][event] {
				t.Errorf("%s [%s] = %d versions, want %d", name, event, n, expected[name][event])
			}
		}
		if total != totals[event] {
			t.Errorf("[%s] gated pairs = %d, want %d", event, total, totals[event])
		}
		for _, line := range []string{
			fmt.Sprintf("GATED_PAIRS=%d\n", totals[event]),
			fmt.Sprintf("TOTAL_JOBS=%d\n", totals[event]+4),
			fmt.Sprintf("EVENT_NAME=%s\n", event),
		} {
			if !strings.Contains(stdout, line) {
				t.Errorf("[%s] summary missing %q", event, line)
			}
		}
	}
}

// --- 3. every option combination, both triggers ----------------------------
// Totals are GATED PAIRS; whole-run job count = gated + 4 (build-jars,
// unit-tests, and the two e2e-gate canaries). Derivation, per the header
// table: pre-A baseline 18/38; t0 adds 8/12; mc1192 adds 6/8; mc114 adds
// 7/10. Run against an empty fixture root so FORCE_BANDS alone decides.
func TestOptionCombinationTotals(t *testing.T) {
	cases := []struct {
		bands      string
		lean, full int
	}{
		{"", 18, 38},
		{"t0", 26, 50},
		{"t0 mc1192", 32, 58},
		{"t0 mc1192 mc114", 39, 68},
	}
	for _, c := range cases {
		for event, want := range map[string]int{"pull_request": c.lean, "workflow_dispatch": c.full} {
			stdout, out := runGrid(t, emptyRoot(t), event, c.bands)
			total := 0
			for _, v := range out {
				total += len(versionsOf(t, v))
			}
			if total != want {
				t.Errorf("gated pairs [%s] bands=%q = %d, want %d", event, c.bands, total, want)
			}
			if !strings.Contains(stdout, fmt.Sprintf("TOTAL_JOBS=%d\n", want+4)) {
				t.Errorf("TOTAL_JOBS [%s] bands=%q: want %d", event, c.bands, want+4)
			}
		}
	}
}

// --- 5. the canaries are never duplicated into a stage list ----------------
func TestCanariesMovedNotDuplicated(t *testing.T) {
	for _, event := range []string{"pull_request", "workflow_dispatch"} {
		_, out := runGrid(t, emptyRoot(t), event, allBands)
		for _, v := range versionsOf(t, out["mc121_java21"]) {
			if v == "1.21.11" {
				t.Errorf("[%s] 1.21.11 must not appear in mc121_java21", event)
			}
		}
		for _, v := range versionsOf(t, out["mc26_java25"]) {
			if v == "26.2" {
				t.Errorf("[%s] 26.2 must not appear in mc26_java25", event)
			}
		}
	}
}

// --- 6. every emitted value is a valid JSON array of strings ---------------
func TestOutputsAreJSONStringArrays(t *testing.T) {
	_, out := runGrid(t, emptyRoot(t), "workflow_dispatch", allBands)
	for name, v := range out {
		versionsOf(t, v) // fatals on anything that is not []string
		_ = name
	}
}

// --- band presence detection (fixture trees, no FORCE_BANDS) ---------------
func TestBandDetection(t *testing.T) {
	write := func(t *testing.T, root, rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("t0 via widened gradle.properties range", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "gradle.properties", "mod_version=1\nminecraft_range_121=>=1.20.3 <1.22\n")
		_, out := runGrid(t, root, "pull_request", "")
		if want := `["1.20.3","1.20.4","1.20.5","1.20.6"]`; out["t0_java21"] != want {
			t.Errorf("t0_java21 = %s, want %s", out["t0_java21"], want)
		}
	})
	t.Run("t0 absent on the pre-widening range", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "gradle.properties", "minecraft_range_121=>=1.21 <1.22\n")
		_, out := runGrid(t, root, "pull_request", "")
		if out["t0_java21"] != "[]" {
			t.Errorf("t0_java21 = %s, want []", out["t0_java21"])
		}
	})
	t.Run("mc1192 and mc114 via source-set directories", func(t *testing.T) {
		root := t.TempDir()
		for _, d := range []string{"src/mc1192/java", "src/mc114/java"} {
			if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		_, out := runGrid(t, root, "pull_request", "")
		if want := `["1.19.2","1.19.4","1.20.1","1.20.2"]`; out["mc1192_java17"] != want {
			t.Errorf("mc1192_java17 = %s, want %s", out["mc1192_java17"], want)
		}
		if want := `["1.14.4","1.15.2","1.16.5"]`; out["mc114_java8"] != want {
			t.Errorf("mc114_java8 = %s, want %s", out["mc114_java8"], want)
		}
		if want := `["1.17.1","1.18.2"]`; out["mc114_java17"] != want {
			t.Errorf("mc114_java17 = %s, want %s", out["mc114_java17"], want)
		}
	})
}

// --- the human summary keeps its exact printf shape ------------------------
func TestSummaryLineFormat(t *testing.T) {
	stdout, _ := runGrid(t, emptyRoot(t), "pull_request", "")
	want := "mc26_java25:       1  [\"26.1\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("summary missing the %%-16s %%3d line %q in:\n%s", want, stdout)
	}
}
