package main

// The era Java-floor table's single home is scripts/e2e-run-one.sh; the
// floors are HARDCODED here on purpose to turn silent drift into a failure.

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
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
