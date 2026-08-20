package main

// Offline assertions. Every number here is a deliberate contract.

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
	"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
	"forge_eventbus7_java21", "forge_eventbus7_java25",
}

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

// Expected per-submatrix job counts (band forced present): each version on
// its own floor JVM, plus newest-Java coverage rows — band ends when lean,
// whole band when full.
var expected = map[string]map[string]int{
	// mainstream: 1.21.x floor 21 (canary 1.21.11 moved to gate), coverage 25/26
	"mc121_java21": {"pull_request": 11, "workflow_dispatch": 11},
	"mc121_java25": {"pull_request": 2, "workflow_dispatch": 12},
	"mc121_java26": {"pull_request": 2, "workflow_dispatch": 12},
	// 26.x floor 25 (canary 26.2 moved to gate)
	"mc26_java25": {"pull_request": 1, "workflow_dispatch": 1},
	"mc26_java26": {"pull_request": 2, "workflow_dispatch": 2},
	// t0 = 1.20.3-1.20.6: floor 21, coverage 25/26
	"t0_java21": {"pull_request": 4, "workflow_dispatch": 4},
	"t0_java25": {"pull_request": 2, "workflow_dispatch": 4},
	"t0_java26": {"pull_request": 2, "workflow_dispatch": 4},
	// mc1192 = 1.19.2 1.19.4 1.20.1 1.20.2: floor 17, coverage 21
	// only (pre-1.20.3 bands have no 25/26 rows).
	"mc1192_java17": {"pull_request": 4, "workflow_dispatch": 4},
	"mc1192_java21": {"pull_request": 2, "workflow_dispatch": 4},
	// mc114 = 1.14.4 1.15.2 1.16.5 | 1.17.1 1.18.2: split floors
	// 8 / 17, coverage 21 across the whole band.
	"mc114_java8":  {"pull_request": 3, "workflow_dispatch": 3},
	"mc114_java17": {"pull_request": 2, "workflow_dispatch": 2},
	"mc114_java21": {"pull_request": 2, "workflow_dispatch": 5},
	// Forge bands: floor rows only, no coverage rows, no lean/full split.
	"forge_java21":           {"pull_request": 3, "workflow_dispatch": 3},
	"forge_legacy_java17":    {"pull_request": 10, "workflow_dispatch": 10},
	"forge_mc116_java8":      {"pull_request": 7, "workflow_dispatch": 7},
	"forge_eventbus7_java21": {"pull_request": 6, "workflow_dispatch": 6},
	"forge_eventbus7_java25": {"pull_request": 4, "workflow_dispatch": 4},
}

const allBands = "t0 mc1192 mc114 forge forge_legacy forge_eventbus7 forge_mc116"

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
		// Forge literals, formerly hand-listed in e2e.yml — byte-pinned so a
		// coverage change is a deliberate edit here, not drift.
		"forge_java21":           `["1.20.4","1.20.6","1.21.5"]`,
		"forge_legacy_java17":    `["1.17.1","1.18","1.18.1","1.18.2","1.19.1","1.19.2","1.20.1","1.20.2","1.20.3","1.20.4"]`,
		"forge_mc116_java8":      `["1.14.4","1.15.2","1.16.1","1.16.2","1.16.3","1.16.4","1.16.5"]`,
		"forge_eventbus7_java21": `["1.21.6","1.21.7","1.21.8","1.21.9","1.21.10","1.21.11"]`,
		"forge_eventbus7_java25": `["26.1","26.1.1","26.1.2","26.2"]`,
	} {
		if out[name] != want {
			t.Errorf("%s = %s, want %s", name, out[name], want)
		}
	}
}

// Absent bands still emit every key, with the literal []; see the
// gen_matrix.go header.
func TestAbsentBandsEmitEmptyArrayLiteral(t *testing.T) {
	for _, event := range []string{"pull_request", "workflow_dispatch"} {
		_, out := runGrid(t, emptyRoot(t), event, "")
		for _, name := range []string{"t0_java21", "t0_java25", "t0_java26",
			"mc1192_java17", "mc1192_java21",
			"mc114_java8", "mc114_java17", "mc114_java21",
			"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
			"forge_eventbus7_java21", "forge_eventbus7_java25"} {
			if got, ok := out[name]; !ok || got != "[]" {
				t.Errorf("[%s] %s = %q, want the literal []", event, name, got)
			}
		}
	}
}

func TestSubmatrixCountsAndTotals(t *testing.T) {
	totals := map[string]int{"pull_request": 69, "workflow_dispatch": 98}
	// TOTAL_JOBS = 2*fabric pairs (each band key feeds a -fabric AND a -quilt
	// caller job) + forge pairs (single-loader) plus 25 fixed jobs (contracts,
	// go-quality, lint-java, unit-tests, the 10 build jobs, the Build
	// aggregator, the 4 e2e-gate canaries, the 2 literal NeoForge jobs, and
	// the 4 config-behaviors legs (#34, one per loader); on push only 15 of
	// these run — gate, NeoForge and config-behaviors are event-skipped).
	// PR: 2*39 + 30 + 25 = 133. Dispatch: 2*68 + 30 + 25 = 191.
	jobTotals := map[string]int{"pull_request": 133, "workflow_dispatch": 191}
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
			fmt.Sprintf("TOTAL_JOBS=%d\n", jobTotals[event]),
			fmt.Sprintf("EVENT_NAME=%s\n", event),
		} {
			if !strings.Contains(stdout, line) {
				t.Errorf("[%s] summary missing %q", event, line)
			}
		}
	}
}

// 3. every option combination, both triggers
// Totals are GATED PAIRS; TOTAL_JOBS is what the workflow spawns: 2 caller
// jobs per fabric pair (-fabric/-quilt), 1 per forge pair (single-loader),
// plus 25 fixed jobs (contracts, go-quality, lint-java, unit-tests, the 5
// build jobs, the Build aggregator, the 4 e2e-gate canaries, the 2
// literal NeoForge jobs, and the 4 config-behaviors legs (#34, one per
// loader); on push only 15 of these run — gate, NeoForge and
// config-behaviors are event-skipped). Run against an empty fixture root so
// FORCE_BANDS alone decides. The forge-less cases double as proof that
// absent Forge bands emit [] and add zero pairs. Note the forge-without-
// forge_legacy case: forge_java21 drops to 2 pairs because 1.20.4 is keyed
// on the legacy band (it boots the legacy jar).
func TestOptionCombinationTotals(t *testing.T) {
	cases := []struct {
		bands              string
		lean, full         int
		leanJobs, fullJobs int
	}{
		{"", 18, 38, 61, 101},
		{"t0", 26, 50, 77, 125},
		{"t0 mc1192", 32, 58, 89, 141},
		{"t0 mc1192 mc114", 39, 68, 103, 161},
		{"t0 mc1192 mc114 forge", 41, 70, 105, 163},
		{"t0 mc1192 mc114 forge forge_legacy", 52, 81, 116, 174},
		{"t0 mc1192 mc114 forge forge_legacy forge_eventbus7", 62, 91, 126, 184},
		{"t0 mc1192 mc114 forge forge_legacy forge_eventbus7 forge_mc116", 69, 98, 133, 191},
	}
	for _, c := range cases {
		for event, want := range map[string][2]int{
			"pull_request":      {c.lean, c.leanJobs},
			"workflow_dispatch": {c.full, c.fullJobs},
		} {
			stdout, out := runGrid(t, emptyRoot(t), event, c.bands)
			total := 0
			for _, v := range out {
				total += len(versionsOf(t, v))
			}
			if total != want[0] {
				t.Errorf("gated pairs [%s] bands=%q = %d, want %d", event, c.bands, total, want[0])
			}
			if !strings.Contains(stdout, fmt.Sprintf("TOTAL_JOBS=%d\n", want[1])) {
				t.Errorf("TOTAL_JOBS [%s] bands=%q: want %d", event, c.bands, want[1])
			}
		}
	}
}

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

func TestOutputsAreJSONStringArrays(t *testing.T) {
	_, out := runGrid(t, emptyRoot(t), "workflow_dispatch", allBands)
	for name, v := range out {
		versionsOf(t, v) // fatals on anything that is not []string
		_ = name
	}
}

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
	t.Run("forge via range keys in forge/gradle.properties", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "forge/gradle.properties",
			"minecraft_range_modern=[1.20.6,1.21.6)\nminecraft_range_legacy=[1.17.1,1.20.5)\n")
		_, out := runGrid(t, root, "pull_request", "")
		if want := `["1.20.4","1.20.6","1.21.5"]`; out["forge_java21"] != want {
			t.Errorf("forge_java21 = %s, want %s", out["forge_java21"], want)
		}
		if got := versionsOf(t, out["forge_legacy_java17"]); len(got) != 10 {
			t.Errorf("forge_legacy_java17 = %s, want 10 versions", out["forge_legacy_java17"])
		}
		// No eventbus7/mc116 range lines -> their rows empty.
		for _, name := range []string{"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_mc116_java8"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("forge mc116 via its own range key", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "forge/gradle.properties",
			"minecraft_range_mc116=[1.14,1.17)\n")
		_, out := runGrid(t, root, "pull_request", "")
		if want := `["1.14.4","1.15.2","1.16.1","1.16.2","1.16.3","1.16.4","1.16.5"]`; out["forge_mc116_java8"] != want {
			t.Errorf("forge_mc116_java8 = %s, want %s", out["forge_mc116_java8"], want)
		}
		// The other Forge bands stay empty without their own range keys.
		for _, name := range []string{"forge_java21", "forge_legacy_java17",
			"forge_eventbus7_java21", "forge_eventbus7_java25"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("forge eventbus7 via its own range key", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "forge/gradle.properties",
			"minecraft_range_eventbus7=[1.21.6,26.3)\n")
		_, out := runGrid(t, root, "pull_request", "")
		if want := `["1.21.6","1.21.7","1.21.8","1.21.9","1.21.10","1.21.11"]`; out["forge_eventbus7_java21"] != want {
			t.Errorf("forge_eventbus7_java21 = %s, want %s", out["forge_eventbus7_java21"], want)
		}
		if want := `["26.1","26.1.1","26.1.2","26.2"]`; out["forge_eventbus7_java25"] != want {
			t.Errorf("forge_eventbus7_java25 = %s, want %s", out["forge_eventbus7_java25"], want)
		}
		// The other Forge bands stay empty without their own range keys.
		for _, name := range []string{"forge_java21", "forge_legacy_java17", "forge_mc116_java8"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("forge modern-only range leaves the legacy rows empty", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "forge/gradle.properties", "minecraft_range_modern=[1.20.6,1.21.6)\n")
		_, out := runGrid(t, root, "pull_request", "")
		// 1.20.4 boots the legacy jar, so it must NOT appear without the
		// legacy band.
		if want := `["1.20.6","1.21.5"]`; out["forge_java21"] != want {
			t.Errorf("forge_java21 = %s, want %s", out["forge_java21"], want)
		}
		for _, name := range []string{"forge_legacy_java17", "forge_mc116_java8"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("no forge/gradle.properties yields all Forge rows empty", func(t *testing.T) {
		_, out := runGrid(t, t.TempDir(), "pull_request", "")
		for _, name := range []string{"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
			"forge_eventbus7_java21", "forge_eventbus7_java25"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
}

// push-to-main builds jars but runs ZERO e2e: every band emits the literal
// [], even when FORCE_BANDS would force it present, and TOTAL_JOBS counts
// only the 15 fixed jobs that actually run on push (the 4 e2e-gate
// canaries and 2 NeoForge legs are event-skipped in ci.yml).
func TestPushEmitsEmptyBands(t *testing.T) {
	stdout, out := runGrid(t, emptyRoot(t), "push", allBands)
	for _, name := range allKeys {
		if got, ok := out[name]; !ok || got != "[]" {
			t.Errorf("[push] %s = %q, want the literal []", name, got)
		}
	}
	for _, line := range []string{"GATED_PAIRS=0\n", "TOTAL_JOBS=15\n", "EVENT_NAME=push\n"} {
		if !strings.Contains(stdout, line) {
			t.Errorf("[push] summary missing %q", line)
		}
	}
}

// The human summary keeps its exact printf shape.
func TestSummaryLineFormat(t *testing.T) {
	stdout, _ := runGrid(t, emptyRoot(t), "pull_request", "")
	want := "mc26_java25:       1  [\"26.1\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("summary missing the %%-16s %%3d line %q in:\n%s", want, stdout)
	}
}
