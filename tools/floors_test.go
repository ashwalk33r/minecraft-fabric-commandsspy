package main

// The era Java-floor table's single home is scripts/e2e-run-one.sh; the
// floors are HARDCODED here on purpose to turn silent drift into a failure.

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The Makefile's default VERSIONS list with each version's era floor.
var defaultVersionFloors = map[string]string{
	"1.20.3": "21", "1.20.4": "21", "1.20.5": "21", "1.20.6": "21",
	"1.21": "21", "1.21.1": "21", "1.21.2": "21", "1.21.3": "21",
	"1.21.4": "21", "1.21.5": "21", "1.21.6": "21", "1.21.7": "21",
	"1.21.8": "21", "1.21.9": "21", "1.21.10": "21", "1.21.11": "21",
	"26.1": "25", "26.2": "25",
	"1.19.2": "17", "1.19.4": "17", "1.20.1": "17", "1.20.2": "17",
	"1.16.5": "8", "1.17.1": "17", "1.18.2": "17",
}

func printJava(t *testing.T, version string) string {
	t.Helper()
	script := filepath.Join("..", "scripts", "e2e-run-one.sh")
	out, err := exec.Command("bash", script, "--print-java", version).Output()
	if err != nil {
		t.Fatalf("--print-java %s: %v", version, err)
	}
	return strings.TrimSpace(string(out))
}

func TestPrintJavaFloors(t *testing.T) {
	for version, want := range defaultVersionFloors {
		if got := printJava(t, version); got != want {
			t.Errorf("--print-java %s = %q, want %q", version, got, want)
		}
	}
}

// Floor rows of the gen-matrix grid (band keys mcXXX_javaN where N is the
// floor JVM for every version in the row). Coverage rows (newest-Java runs
// above the floor) are deliberately not listed and not checked.
var floorRows = map[string]string{
	"mc121_java21":  "21",
	"mc26_java25":   "25",
	"t0_java21":     "21",
	"mc1192_java17": "17",
	"mc114_java8":   "8",
	"mc114_java17":  "17",
}

func TestGenMatrixFloorRowsAgreeWithPrintJava(t *testing.T) {
	var gh bytes.Buffer
	if err := genMatrix("..", "workflow_dispatch", "", io.Discard, &gh); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(gh.String(), "\n") {
		name, j, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		want, isFloorRow := floorRows[name]
		if !isFloorRow {
			continue
		}
		seen++
		var versions []string
		if err := json.Unmarshal([]byte(j), &versions); err != nil {
			t.Fatalf("%s: bad JSON %q: %v", name, j, err)
		}
		for _, v := range versions {
			if got := printJava(t, v); got != want {
				t.Errorf("%s contains %s, but --print-java %s = %q, want %q", name, v, v, got, want)
			}
		}
	}
	if seen != len(floorRows) {
		t.Errorf("saw %d floor rows in gen-matrix output, want %d", seen, len(floorRows))
	}
}

// Forge rows are deliberately NOT in floorRows: --print-java is the FABRIC
// floor table (1.20.3/1.20.4 report 21 there, while the Forge legacy jar runs
// them on 17 — e2e-run-one.sh overrides FLOOR_JAVA for LOADER=forge). The
// Forge drift check is against --print-forge-routing instead: every version
// the generator emits must route to the jar band its row boots.
func TestGenMatrixForgeRowsAgreeWithForgeRouting(t *testing.T) {
	script := filepath.Join("..", "scripts", "e2e-run-one.sh")
	printForgeRouting := func(version string) string {
		out, err := exec.Command("bash", script, "--print-forge-routing", version).Output()
		if err != nil {
			t.Fatalf("--print-forge-routing %s: %v", version, err)
		}
		return strings.TrimSpace(string(out))
	}

	var gh bytes.Buffer
	if err := genMatrix("..", "workflow_dispatch", "", io.Discard, &gh); err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(gh.String(), "\n") {
		name, j, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(name, "forge") {
			continue
		}
		var versions []string
		if err := json.Unmarshal([]byte(j), &versions); err != nil {
			t.Fatalf("%s: bad JSON %q: %v", name, j, err)
		}
		rows[name] = versions
	}

	for _, v := range rows["forge_legacy_java17"] {
		if got := printForgeRouting(v); got != "legacy 0" {
			t.Errorf("forge_legacy_java17 contains %s, routing = %q, want \"legacy 0\"", v, got)
		}
	}
	for _, v := range rows["forge_java21"] {
		// 1.20.4 rides in the modern job but boots the legacy jar in-range
		// (the legacy ceiling on a modern JVM) — see gen_matrix.go.
		want := "modern 0"
		if v == "1.20.4" {
			want = "legacy 0"
		}
		if got := printForgeRouting(v); got != want {
			t.Errorf("forge_java21 contains %s, routing = %q, want %q", v, got, want)
		}
	}
	// mc116 band: every emitted version must route to the mc116 jar in-range —
	// 1.16.4 included, now known-good via e2e-run-one.sh's install-time
	// ModLauncher 8.1.3 drop-in (see docs/version-matrix.md).
	for _, v := range rows["forge_mc116_java8"] {
		if got := printForgeRouting(v); got != "mc116 0" {
			t.Errorf("forge_mc116_java8 contains %s, routing = %q, want \"mc116 0\"", v, got)
		}
	}
	for _, key := range []string{"forge_eventbus7_java21", "forge_eventbus7_java25"} {
		for _, v := range rows[key] {
			if got := printForgeRouting(v); got != "eventbus7 0" {
				t.Errorf("%s contains %s, routing = %q, want \"eventbus7 0\"", key, v, got)
			}
		}
	}
	// The forward-JVM row (#58) probes the eventbus7 era only — Forge's
	// modern-era bootstrap cannot boot java 26, or java 25 (issue #66; see
	// gen_matrix.go), so the modern band has no above-floor probe to check.
	for _, v := range rows["forge_java26"] {
		if got, want := printForgeRouting(v), "eventbus7 0"; got != want {
			t.Errorf("forge_java26 contains %s, routing = %q, want %q", v, got, want)
		}
	}
	for _, name := range []string{"forge_java21", "forge_legacy_java17", "forge_mc116_java8",
		"forge_eventbus7_java21", "forge_eventbus7_java25", "forge_java26"} {
		if len(rows[name]) == 0 {
			t.Errorf("%s: no versions emitted (band missing from the real tree?)", name)
		}
	}
}

// NeoForge rows are out of floorRows for the same reason the Forge rows are,
// but with its own table: --print-java is the FABRIC floor table (1.20.4
// reports 21 there, while NeoForge's 20.4 line is a Java-17 line, and
// e2e-run-one.sh overrides FLOOR_JAVA for LOADER=neoforge). The drift check is
// --print-neo-routing's SECOND field — the Java floor of the loader build that
// version actually fetches. One band jar serves every row, so nothing here
// checks a jar band; only the floors, which are what the row names promise.
func TestGenMatrixNeoRowsAgreeWithNeoRouting(t *testing.T) {
	script := filepath.Join("..", "scripts", "e2e-run-one.sh")
	printNeoFloor := func(version string) string {
		out, err := exec.Command("bash", script, "--print-neo-routing", version).Output()
		if err != nil {
			t.Fatalf("--print-neo-routing %s: %v", version, err)
		}
		fields := strings.Fields(string(out))
		if len(fields) != 2 {
			t.Fatalf("--print-neo-routing %s = %q, want \"<build> <floor>\"", version, out)
		}
		if fields[0] == "unsupported" {
			t.Fatalf("--print-neo-routing %s = unsupported: no NeoForge line, the row cannot run", version)
		}
		return fields[1]
	}

	var gh bytes.Buffer
	if err := genMatrix("..", "workflow_dispatch", "", io.Discard, &gh); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(gh.String(), "\n") {
		name, j, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(name, "neo_java") {
			continue
		}
		seen++
		want := strings.TrimPrefix(name, "neo_java")
		var versions []string
		if err := json.Unmarshal([]byte(j), &versions); err != nil {
			t.Fatalf("%s: bad JSON %q: %v", name, j, err)
		}
		if len(versions) == 0 {
			t.Errorf("%s: no versions emitted (neoforge/gradle.properties missing its band range?)", name)
		}
		for _, v := range versions {
			if got := printNeoFloor(v); got != want {
				t.Errorf("%s contains %s, but its NeoForge floor is %q, want %q", name, v, got, want)
			}
		}
	}
	if seen != 3 {
		t.Errorf("saw %d neo_java rows in gen-matrix output, want 3", seen)
	}

	// The forward-JVM row (#58) is the mirror image of the three above: it
	// exists precisely because its versions sit BELOW the JVM it boots them
	// on, so the equality just applied would be exactly the wrong assertion.
	// Naming it outside the neo_java<N> shape is what lets both checks stay
	// strict — the floor rows keep proving "this row IS the floor", and this
	// one proves "this row is ABOVE the floor", which is the whole point.
	const fwdRow, fwdJava = "neo_fwd_java25", 25
	fwdSeen := false
	for _, line := range strings.Split(gh.String(), "\n") {
		name, j, ok := strings.Cut(line, "=")
		if !ok || name != fwdRow {
			continue
		}
		fwdSeen = true
		var versions []string
		if err := json.Unmarshal([]byte(j), &versions); err != nil {
			t.Fatalf("%s: bad JSON %q: %v", name, j, err)
		}
		if len(versions) == 0 {
			t.Errorf("%s: no versions emitted (neoforge/gradle.properties missing its band range?)", name)
		}
		for _, v := range versions {
			floor, err := strconv.Atoi(printNeoFloor(v))
			if err != nil {
				t.Fatalf("%s: --print-neo-routing %s floor %q is not a number: %v", name, v, printNeoFloor(v), err)
			}
			if floor >= fwdJava {
				t.Errorf("%s contains %s, whose NeoForge floor is %d — that is not forward coverage; this row must boot versions whose floor is below java %d",
					name, v, floor, fwdJava)
			}
		}
	}
	if !fwdSeen {
		t.Errorf("%s is missing from the gen-matrix output entirely", fwdRow)
	}
}
