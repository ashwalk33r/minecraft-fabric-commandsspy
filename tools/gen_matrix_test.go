package main

// Offline assertions. Every number here is a deliberate contract.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var allKeys = []string{
	"mc121_java21", "mc121_java25", "mc121_java26", "mc26_java25", "mc26_java26",
	"t0_java21", "t0_java25", "t0_java26",
	"mc1192_java17", "mc1192_java21",
	"mc114_java8", "mc114_java8_fabric", "mc114_java17", "mc114_java21",
	"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
	"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26",
	"neo_java17", "neo_java21", "neo_java25", "neo_fwd_java25",
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
// whole SAMPLE when full. On workflow_dispatch the FLOOR rows widen too, from
// each band's `sampled` list to its `deep` one (issue #59): before that, the
// full grid added Java legs over the very same Minecraft versions.
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
	"mc1192_java17": {"pull_request": 4, "workflow_dispatch": 7},
	"mc1192_java21": {"pull_request": 2, "workflow_dispatch": 4},
	// mc114 = 1.14.4 1.15.2 1.16.5 | 1.17.1 1.18.2: split floors
	// 8 / 17, coverage 21 across the whole band.
	"mc114_java8": {"pull_request": 3, "workflow_dispatch": 8},
	// Versions Quilt Loader has no build for, so they get a fabric-only leg
	// (issue #69). Empty on pull_request: 1.14 is deep-only.
	"mc114_java8_fabric": {"pull_request": 0, "workflow_dispatch": 1},
	"mc114_java17":       {"pull_request": 2, "workflow_dispatch": 5},
	"mc114_java21":       {"pull_request": 2, "workflow_dispatch": 5},
	// Forge bands: floor rows plus the one forward-JVM row, no lean/full split
	// except where the deep sweep widens the band's own version list.
	"forge_java21":           {"pull_request": 4, "workflow_dispatch": 7},
	"forge_legacy_java17":    {"pull_request": 10, "workflow_dispatch": 13},
	"forge_mc116_java8":      {"pull_request": 7, "workflow_dispatch": 7},
	"forge_eventbus7_java21": {"pull_request": 6, "workflow_dispatch": 6},
	"forge_eventbus7_java25": {"pull_request": 4, "workflow_dispatch": 4},
	// The forward-JVM row (#58): the eventbus7 band's ceiling alone, on the
	// newest JVM the harness has. The modern band is deliberately absent — its
	// bootstrap cannot boot above its java-21 floor at all (issue #66), so
	// there is no forward JVM to probe for it.
	"forge_java26": {"pull_request": 1, "workflow_dispatch": 1},
	// NeoForge: one band jar, so floor rows — band edges plus NeoForge's own
	// three Java floors plus the 1.21.1 interior — and the one forward-JVM
	// row (#58), 1.21.1 (floor 21) booted on 25.
	"neo_java17":     {"pull_request": 2, "workflow_dispatch": 2},
	"neo_java21":     {"pull_request": 3, "workflow_dispatch": 9},
	"neo_java25":     {"pull_request": 1, "workflow_dispatch": 2},
	"neo_fwd_java25": {"pull_request": 1, "workflow_dispatch": 1},
}

const allBands = "t0 mc1192 mc114 forge forge_legacy forge_eventbus7 forge_mc116 neo"

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
		"forge_java21":           `["1.20.4","1.20.6","1.21.1","1.21.5"]`,
		"forge_legacy_java17":    `["1.17.1","1.18","1.18.1","1.18.2","1.19.1","1.19.2","1.20.1","1.20.2","1.20.3","1.20.4"]`,
		"forge_mc116_java8":      `["1.14.4","1.15.2","1.16.1","1.16.2","1.16.3","1.16.4","1.16.5"]`,
		"forge_eventbus7_java21": `["1.21.6","1.21.7","1.21.8","1.21.9","1.21.10","1.21.11"]`,
		"forge_eventbus7_java25": `["26.1","26.1.1","26.1.2","26.2"]`,
		// The forward-JVM row: the eventbus7 band's ceiling on java 26. Not the
		// modern band's — it has no above-floor JVM (issue #66).
		"forge_java26": `["26.2"]`,
		// NeoForge band-jar legs: edges + the three NeoForge Java floors +
		// 1.21.1. 1.21.11 and 26.2 are gate canaries on FABRIC only, so they
		// belong here without duplicating the gate. neo_fwd_java25 is the
		// forward-JVM row and is deliberately outside the neo_java<N> naming
		// so floors_test's floor equality keeps applying to floor rows only.
		"neo_java17":     `["1.20.2","1.20.4"]`,
		"neo_java21":     `["1.20.6","1.21.1","1.21.11"]`,
		"neo_java25":     `["26.2"]`,
		"neo_fwd_java25": `["1.21.1"]`,
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
			"mc114_java8", "mc114_java8_fabric", "mc114_java17", "mc114_java21",
			"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
			"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26",
			"neo_java17", "neo_java21", "neo_java25", "neo_fwd_java25"} {
			if got, ok := out[name]; !ok || got != "[]" {
				t.Errorf("[%s] %s = %q, want the literal []", event, name, got)
			}
		}
	}
}

func TestSubmatrixCountsAndTotals(t *testing.T) {
	totals := map[string]int{"pull_request": 78, "workflow_dispatch": 132}
	// TOTAL_JOBS = 2*fabric pairs (each band key feeds a -fabric AND a -quilt
	// caller job) + forge and neo pairs (single-loader) plus 25 fixed jobs
	// (contracts, go-quality, lint-java, unit-tests, the 10 build jobs, the
	// Build aggregator, the 4 e2e-gate canaries, the 4 config-behaviors legs
	// (#34, one per loader) and the 3 out-of-range refusal guards (fabric and
	// quilt on 1.19.0, forge on 1.21.6 handed the modern jar); on push only 14
	// of these run — gate, config-behaviors and the refusal guards are
	// event-skipped). The NeoForge legs are generated now, not fixed jobs.
	// PR: 2*39 + 32 + 7 + 25 = 142. Dispatch: 2*79 + 38 + 14 + 25 + 1 = 236 — the
	// deep sweep's whole delta is Minecraft versions the PR grid never boots.
	// The forge and neo terms carry the #58 forward-JVM rows: forge_java26 (1)
	// and neo_fwd_java25 (1), the same on every event.
	jobTotals := map[string]int{"pull_request": 142, "workflow_dispatch": 236}
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
// jobs per fabric pair (-fabric/-quilt), 1 per forge or neo pair
// (single-loader), plus 25 fixed jobs (contracts, go-quality, lint-java,
// unit-tests, the 10 build jobs, the Build aggregator, the 4 e2e-gate
// canaries, the 4 config-behaviors legs (#34, one per loader) and the 3
// out-of-range refusal guards (fabric and quilt on 1.19.0, forge on 1.21.6);
// on push only 14 of these run — gate, config-behaviors and the refusal
// guards are event-skipped). Run
// against an empty fixture root so FORCE_BANDS alone decides. The forge-less
// cases double as proof that absent Forge bands emit [] and add zero pairs.
// Note the forge-without-forge_legacy case: forge_java21 drops to 2 pairs
// because 1.20.4 is keyed on the legacy band (it boots the legacy jar). The
// last case adds `neo`: +7 pairs and +7 jobs, NOT +14 — see
// TestNeoRowsCountAsOneJobEach.
// The #58 forward-JVM rows show up per band, not in a lump: forge_java26 is
// keyed on forge_eventbus7 alone (the modern band has no java-26 probe, issue
// #66), and `neo` brings neo_fwd_java25.
func TestOptionCombinationTotals(t *testing.T) {
	cases := []struct {
		bands              string
		lean, full         int
		leanJobs, fullJobs int
	}{
		{"", 18, 38, 61, 101},
		{"t0", 26, 50, 77, 125},
		{"t0 mc1192", 32, 61, 89, 147},
		{"t0 mc1192 mc114", 39, 80, 103, 184},
		{"t0 mc1192 mc114 forge", 42, 86, 106, 190},
		{"t0 mc1192 mc114 forge forge_legacy", 53, 100, 117, 204},
		{"t0 mc1192 mc114 forge forge_legacy forge_eventbus7", 64, 111, 128, 215},
		{"t0 mc1192 mc114 forge forge_legacy forge_eventbus7 forge_mc116", 71, 118, 135, 222},
		{allBands, 78, 132, 142, 236},
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
		if want := `["1.20.4","1.20.6","1.21.1","1.21.5"]`; out["forge_java21"] != want {
			t.Errorf("forge_java21 = %s, want %s", out["forge_java21"], want)
		}
		if got := versionsOf(t, out["forge_legacy_java17"]); len(got) != 10 {
			t.Errorf("forge_legacy_java17 = %s, want 10 versions", out["forge_legacy_java17"])
		}
		// The forward-JVM row is keyed on eventbus7 alone, so without that
		// band it is empty even though the modern band is present.
		if out["forge_java26"] != "[]" {
			t.Errorf("forge_java26 = %s, want []", out["forge_java26"])
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
			"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26"} {
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
		// The forward-JVM row's eventbus7 half rides this band; its modern
		// half needs minecraft_range_modern, which this fixture omits.
		if want := `["26.2"]`; out["forge_java26"] != want {
			t.Errorf("forge_java26 = %s, want %s", out["forge_java26"], want)
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
		if want := `["1.20.6","1.21.1","1.21.5"]`; out["forge_java21"] != want {
			t.Errorf("forge_java21 = %s, want %s", out["forge_java21"], want)
		}
		if out["forge_java26"] != "[]" {
			t.Errorf("forge_java26 = %s, want []", out["forge_java26"])
		}
		for _, name := range []string{"forge_legacy_java17", "forge_mc116_java8"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("neo via the band range key in neoforge/gradle.properties", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "neoforge/gradle.properties",
			"neoforge_version_all=20.4.251\nminecraft_range_neo_all=[1.20.2,26.3)\n")
		_, out := runGrid(t, root, "pull_request", "")
		for name, want := range map[string]string{
			"neo_java17":     `["1.20.2","1.20.4"]`,
			"neo_java21":     `["1.20.6","1.21.1","1.21.11"]`,
			"neo_java25":     `["26.2"]`,
			"neo_fwd_java25": `["1.21.1"]`,
		} {
			if out[name] != want {
				t.Errorf("%s = %s, want %s", name, out[name], want)
			}
		}
	})
	t.Run("no neoforge/gradle.properties yields all NeoForge rows empty", func(t *testing.T) {
		_, out := runGrid(t, t.TempDir(), "pull_request", "")
		for _, name := range []string{"neo_java17", "neo_java21", "neo_java25", "neo_fwd_java25"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
	t.Run("no forge/gradle.properties yields all Forge rows empty", func(t *testing.T) {
		_, out := runGrid(t, t.TempDir(), "pull_request", "")
		for _, name := range []string{"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
			"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26"} {
			if out[name] != "[]" {
				t.Errorf("%s = %s, want []", name, out[name])
			}
		}
	})
}

// push-to-main builds jars but runs ZERO e2e: every band emits the literal
// [], even when FORCE_BANDS would force it present, and TOTAL_JOBS counts
// only the 14 fixed jobs that actually run on push (the 4 e2e-gate canaries,
// the 4 config-behaviors legs and the 3 out-of-range refusal guards are all
// event-skipped in ci.yml). The NeoForge
// legs are generated rows now, so push zeroes them like every other band
// instead of them riding in the fixed count. The deep sweep does not reach
// push either: `full` and `push` are different events, and push wins.
func TestPushEmitsEmptyBands(t *testing.T) {
	stdout, out := runGrid(t, emptyRoot(t), "push", allBands)
	for _, name := range allKeys {
		if got, ok := out[name]; !ok || got != "[]" {
			t.Errorf("[push] %s = %q, want the literal []", name, got)
		}
	}
	for _, line := range []string{"GATED_PAIRS=0\n", "TOTAL_JOBS=14\n", "EVENT_NAME=push\n"} {
		if !strings.Contains(stdout, line) {
			t.Errorf("[push] summary missing %q", line)
		}
	}
}

// The NeoForge rows are SINGLE-loader: LOADER=neoforge has no quilt twin, so
// each version is one caller job, not two. The job-count loop keys that off a
// name prefix, and strings.Contains(name, "forge") would match "neoforge" and
// silently double these — this test is the tripwire for that regression.
func TestNeoRowsCountAsOneJobEach(t *testing.T) {
	jobs := func(bands string) int {
		stdout, _ := runGrid(t, emptyRoot(t), "pull_request", bands)
		var n int
		for _, line := range strings.Split(stdout, "\n") {
			if _, v, ok := strings.Cut(line, "TOTAL_JOBS="); ok {
				if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
					t.Fatalf("bad TOTAL_JOBS line %q: %v", line, err)
				}
			}
		}
		return n
	}
	_, out := runGrid(t, emptyRoot(t), "pull_request", "neo")
	for name, want := range map[string]string{
		"neo_java17":     `["1.20.2","1.20.4"]`,
		"neo_java21":     `["1.20.6","1.21.1","1.21.11"]`,
		"neo_java25":     `["26.2"]`,
		"neo_fwd_java25": `["1.21.1"]`,
	} {
		if out[name] != want {
			t.Errorf("%s = %s, want %s", name, out[name], want)
		}
	}
	if got := jobs("neo") - jobs(""); got != 7 {
		t.Errorf("the neo band adds %d jobs, want 7 (one per version, the #58 forward-JVM row included; 14 means the fabric -quilt doubling leaked in)", got)
	}
}

// The human summary keeps its exact printf shape.
func TestSummaryLineFormat(t *testing.T) {
	stdout, _ := runGrid(t, emptyRoot(t), "pull_request", "")
	want := "mc26_java25:       1  [\"26.1\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("summary missing the %%-16s %%3d line %q in:\n%s", want, stdout)
	}
	// The neo_* names must stay inside the %-16s column, or the whole summary
	// loses its alignment the way the forge_eventbus7_* names already do.
	stdout, _ = runGrid(t, emptyRoot(t), "pull_request", allBands)
	want = "neo_java25:        1  [\"26.2\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("summary missing the %%-16s %%3d line %q in:\n%s", want, stdout)
	}
	// neo_fwd_java25 is the longest neo_* name; it must still fit the column.
	want = "neo_fwd_java25:    1  [\"1.21.1\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("summary missing the %%-16s %%3d line %q in:\n%s", want, stdout)
	}
}

// ---------------------------------------------------------------------------
// The sampling rule (issue #59).

// bandRows maps each coverage-table band to the emitted rows that boot it.
// Every row in allKeys appears exactly once here — TestEveryRowBelongsToABand
// is the tripwire for a new row added without a band.
var bandRows = map[string][]string{
	"mc121":           {"mc121_java21", "mc121_java25", "mc121_java26"},
	"mc26":            {"mc26_java25", "mc26_java26"},
	"t0":              {"t0_java21", "t0_java25", "t0_java26"},
	"mc1192":          {"mc1192_java17", "mc1192_java21"},
	"mc114":           {"mc114_java8", "mc114_java8_fabric", "mc114_java17", "mc114_java21"},
	"forge":           {"forge_java21"},
	"forge_legacy":    {"forge_legacy_java17"},
	"forge_mc116":     {"forge_mc116_java8"},
	"forge_eventbus7": {"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26"},
	"neo":             {"neo_java17", "neo_java21", "neo_java25", "neo_fwd_java25"},
}

func setOf(list []string) map[string]bool {
	s := map[string]bool{}
	for _, v := range list {
		s[v] = true
	}
	return s
}

// diffSets reports what is in a but not b, and what is in b but not a.
func diffSets(a, b map[string]bool) (onlyA, onlyB []string) {
	for v := range a {
		if !b[v] {
			onlyA = append(onlyA, v)
		}
	}
	for v := range b {
		if !a[v] {
			onlyB = append(onlyB, v)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return onlyA, onlyB
}

// THE anti-drift assertion. Every version a band DECLARES is either booted by
// the deep sweep or excluded with a written reason, and nothing is both. That
// is what stops the gap the issue found from re-opening quietly: a version
// cannot leave the grid by being deleted from a list, only by acquiring a
// reason someone had to type.
func TestCoverageTableAccountsForEveryDeclaredVersion(t *testing.T) {
	for name, c := range coverage {
		declared := setOf(c.declared)
		if len(declared) != len(c.declared) {
			t.Errorf("%s: declared list has duplicates: %v", name, c.declared)
		}
		deep := setOf(c.deep)
		for _, v := range c.sampled {
			if !deep[v] {
				t.Errorf("%s: %s is sampled but absent from deep — deep must be a superset", name, v)
			}
		}
		for _, v := range c.deep {
			if !declared[v] {
				t.Errorf("%s: %s is booted but is not in the band's declared range", name, v)
			}
		}
		accounted := setOf(c.deep)
		for v, reason := range c.excluded {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s: %s is excluded with an empty reason", name, v)
			}
			if accounted[v] {
				t.Errorf("%s: %s is both booted and excluded", name, v)
			}
			accounted[v] = true
		}
		unaccounted, undeclared := diffSets(declared, accounted)
		if len(unaccounted) > 0 {
			t.Errorf("%s: declared but neither booted nor excluded with a reason: %v", name, unaccounted)
		}
		if len(undeclared) > 0 {
			t.Errorf("%s: booted or excluded but not declared: %v", name, undeclared)
		}
	}
}

// The table is only worth anything if the grid obeys it: pull_request boots
// exactly each band's sample, workflow_dispatch boots exactly its deep list.
// Without this, the table above is decoration and the emit calls could say
// something else entirely.
func TestGridBootsTheSampleThenTheDeepList(t *testing.T) {
	for _, tc := range []struct {
		event string
		want  func(bandCoverage) []string
	}{
		{"pull_request", func(c bandCoverage) []string { return c.sampled }},
		{"workflow_dispatch", func(c bandCoverage) []string { return c.deep }},
	} {
		_, out := runGrid(t, emptyRoot(t), tc.event, allBands)
		for name, rows := range bandRows {
			got := map[string]bool{}
			for _, r := range rows {
				for _, v := range versionsOf(t, out[r]) {
					got[v] = true
				}
			}
			// forge_java21 also carries 1.20.4, which is keyed on the LEGACY
			// band (it boots the legacy jar on a modern JVM) — see gen_matrix.go.
			if name == "forge" {
				delete(got, "1.20.4")
			}
			missing, extra := diffSets(setOf(tc.want(coverage[name])), got)
			if len(missing) > 0 {
				t.Errorf("[%s] band %s: %v are in the table but booted by no row", tc.event, name, missing)
			}
			if len(extra) > 0 {
				t.Errorf("[%s] band %s: rows boot %v, which the table does not list", tc.event, name, extra)
			}
		}
	}
}

func TestEveryRowBelongsToABand(t *testing.T) {
	seen := map[string]int{}
	for _, rows := range bandRows {
		for _, r := range rows {
			seen[r]++
		}
	}
	for _, k := range allKeys {
		if seen[k] != 1 {
			t.Errorf("%s appears in bandRows %d times, want exactly 1", k, seen[k])
		}
		delete(seen, k)
	}
	for r := range seen {
		t.Errorf("bandRows names %s, which the grid does not emit", r)
	}
}

// quiltUnavailable names versions Quilt Loader has no build for (issue #69).
// Whether that is TRUE of the world is a network fact and contracts cannot
// check it. What contracts can check, and this does, is that the grid obeys the
// set: every version in it must leave the shared row -- which feeds a quilt
// caller job that could only ever fail -- and appear in the fabric-only row
// instead. Without this, adding a version to the set and forgetting the row, or
// deleting the row and leaving the set, both pass silently.
func TestQuiltUnavailableVersionsRunFabricOnly(t *testing.T) {
	if len(quiltUnavailable) == 0 {
		t.Skip("no quilt-unavailable versions declared")
	}
	for _, event := range []string{"pull_request", "workflow_dispatch"} {
		_, out := runGrid(t, emptyRoot(t), event, allBands)
		for _, name := range allKeys {
			fabricOnly := strings.HasSuffix(name, "_fabric")
			for _, v := range versionsOf(t, out[name]) {
				if !quiltUnavailable[v] {
					continue
				}
				if !fabricOnly {
					t.Errorf("[%s] %s emits %s, which Quilt cannot boot: it belongs in a _fabric row",
						event, name, v)
				}
			}
		}
	}
	// And the fabric-only row is not merely absent-by-accident: on the event
	// that boots the deep list, every declared quilt-unavailable version the
	// band covers must actually appear somewhere.
	_, out := runGrid(t, emptyRoot(t), "workflow_dispatch", allBands)
	seen := map[string]bool{}
	for _, name := range allKeys {
		for _, v := range versionsOf(t, out[name]) {
			seen[v] = true
		}
	}
	for v := range quiltUnavailable {
		if !seen[v] {
			t.Errorf("workflow_dispatch: %s is quilt-unavailable but booted by no row at all; "+
				"it should still run on fabric", v)
		}
	}
}
