package main

// gen-matrix — THE single source of e2e stage definitions (a byte-for-byte port
// of the retired bash grid generator).
//
// Every {band, Java} submatrix in .github/workflows/e2e.yml reads its version
// list from an output emitted here. Adding or removing a supported Minecraft
// band is a change to THIS FILE (plus one uses: block in e2e.yml), never a
// rewrite of the workflow's job bodies.
//
// Contract with the workflow:
//   - EVERY output name is emitted on EVERY run, even for a band that does not
//     exist yet, in which case the value is the literal `[]`. This matters:
//     `needs.build-jars.outputs.missing_key` evaluates to '' in GitHub
//     expressions, and '' != '[]' is TRUE, which would feed fromJSON('') to a
//     matrix and hard-error the run. A literal [] keeps the `!= '[]'` skip
//     guard honest.
//   - The two gate canaries (1.21.11/java21, 26.2/java25) are MOVED to the
//     gate, never duplicated here.
//
// Grid policy (era-correct Java floors, per the 2026-08-17 addendum): every
// version runs on its own floor JVM, plus "newest-Java" coverage rows — the
// band's OLDEST and NEWEST versions on each higher pinned JVM when lean
// (pull_request), the whole band when full (workflow_dispatch). Floor rows are
// exhaustive on BOTH triggers: Minecraft breaks are per-patch, JVM breaks are
// per-JVM and do not vary inside a band that shares one jar, so the ends of a
// band cover the real variable on higher JVMs.
//
// Era floors (keep in sync with the Makefile and scripts/e2e-run-one.sh):
//
//	1.14-1.16.x -> 8
//	1.17.x      -> 17  (historical floor 16 has no Temurin jre image)
//	1.18-1.20.2 -> 17
//	1.20.3-1.21 -> 21  (1.20.3/1.20.4's vanilla floor is 17, but they run the
//	                    mc1.21.x jar, which is Java 21 bytecode)
//	26.x        -> 25
//
// Java 11 is supported for MANUAL override runs only and never appears here.
//
// Job-count derivation (gated pairs; whole-run job count adds 4 for
// build-jars, unit-tests, and the two e2e-gate canaries):
//
//	band    versions                          lean rows        full rows
//	mc121   1.21..1.21.11 (canary moved)      11+2+2   = 15    11+12+12 = 35
//	mc26    26.1, 26.2 (canary moved)         1+2      = 3     1+2      = 3
//	t0      1.20.3 1.20.4 1.20.5 1.20.6       4+2+2    = 8     4+4+4    = 12
//	mc1192  1.19.2 1.19.4 1.20.1 1.20.2       4+2      = 6     4+4      = 8
//	mc114   1.14.4 1.15.2 1.16.5|1.17.1 1.18.2  3+2+2  = 7     3+2+5    = 10
//
//	combo        lean gated (total)   full gated (total)
//	baseline     18 (22)              38 (42)
//	+A (t0)      26 (30)              50 (54)
//	+B (mc1192)  32 (36)              58 (62)
//	+C (mc114)   39 (43)              68 (72)   <- current in-tree state
//
// Env:
//
//	EVENT_NAME    pull_request | workflow_dispatch   (default: pull_request)
//	GITHUB_OUTPUT file to append `name=json` lines to (optional)
//	FORCE_BANDS   space-separated band names to treat as present (testing)
//	REPO_ROOT     repo root (default: the current working directory — the
//	              workflow runs the binary from the repo root)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var t0RangeRe = regexp.MustCompile(`(?m)^minecraft_range_121=>=1\.20\.3`)

// bandPresent reports whether a band's build target exists in the tree.
// Option plans A/B/C each added ONE arm here when their band shipped.
// forced (FORCE_BANDS) overrides detection for offline testing.
func bandPresent(repoRoot, name string, forced []string) bool {
	for _, f := range forced {
		if f == name {
			return true
		}
	}
	switch name {
	case "t0":
		// Option A widened the mc121 jar's range floor below 1.21 (now
		// >=1.20.3: the void-descriptor era), so its arm keys off the range.
		data, err := os.ReadFile(filepath.Join(repoRoot, "gradle.properties"))
		return err == nil && t0RangeRe.Match(data)
	case "mc1192":
		// Option B ships src/mc1192 (>=1.19 <1.20.3); Option C ships src/mc114
		// (>=1.14 <1.19). Each is a real source set, so its dir is the proof.
		st, err := os.Stat(filepath.Join(repoRoot, "src", "mc1192", "java"))
		return err == nil && st.IsDir()
	case "mc114":
		st, err := os.Stat(filepath.Join(repoRoot, "src", "mc114", "java"))
		return err == nil && st.IsDir()
	}
	return false
}

// ends returns the oldest and the newest entry (the whole list if <2 items).
func ends(list []string) []string {
	if len(list) < 2 {
		return list
	}
	return []string{list[0], list[len(list)-1]}
}

func runGenMatrix(args []string) error {
	var ghOut io.Writer
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		// Every write to ghOut is tracked via emitErr; Close on an os.File
		// whose writes all succeeded is safe to drop.
		defer func() { _ = f.Close() }()
		ghOut = f
	}
	repoRoot := os.Getenv("REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}
	eventName := os.Getenv("EVENT_NAME")
	if eventName == "" {
		eventName = "pull_request"
	}
	return genMatrix(repoRoot, eventName, os.Getenv("FORCE_BANDS"), os.Stdout, ghOut)
}

// genMatrix computes and emits the grid: `name=json` lines appended to ghOut
// (when non-nil) and the human summary on stdout.
func genMatrix(repoRoot, eventName, forceBands string, stdout, ghOut io.Writer) error {
	full := eventName == "workflow_dispatch"
	forced := strings.Fields(forceBands)

	type row struct {
		name string
		json string
		n    int
	}
	var rows []row
	var emitErr error

	emit := func(name string, versions []string) {
		j := "[]"
		if len(versions) > 0 {
			b, err := json.Marshal(versions)
			if err != nil {
				emitErr = err
				return
			}
			j = string(b)
		}
		if ghOut != nil {
			if _, err := fmt.Fprintf(ghOut, "%s=%s\n", name, j); err != nil {
				emitErr = err
				return
			}
		}
		rows = append(rows, row{name, j, len(versions)})
	}
	// emitCoverage is a newest-Java coverage row: the band's ends when lean,
	// everything when full.
	emitCoverage := func(name string, versions []string) {
		if full || len(versions) == 0 {
			emit(name, versions)
		} else {
			emit(name, ends(versions))
		}
	}

	band := func(name string, versions ...string) []string {
		if bandPresent(repoRoot, name, forced) {
			return versions
		}
		return nil
	}

	// =======================================================================
	// STAGE 2 — current mainstream (1.21.x floor 21, 26.x floor 25).
	// =======================================================================
	all121 := []string{"1.21", "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5",
		"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11"}
	// 1.21.11 is the java-21 gate canary and must not appear in the j21 list.
	floor121 := all121[:len(all121)-1]

	emit("mc121_java21", floor121)
	if full {
		emit("mc121_java25", all121)
		emit("mc121_java26", all121)
	} else {
		emit("mc121_java25", []string{"1.21", "1.21.11"})
		emit("mc121_java26", []string{"1.21", "1.21.11"})
	}

	// 26.2 is the java-25 gate canary and must not appear in the java-25 list.
	emit("mc26_java25", []string{"26.1"})
	emit("mc26_java26", []string{"26.1", "26.2"})

	// =======================================================================
	// STAGE 3 — T0 band (1.20.3-1.20.6, Option A): floor 21, coverage 25/26.
	// 1.20.3 is where execute() became void — the mc121 jar's hook shape — so
	// the band starts there; 1.20.3/1.20.4 run on java 21 because the jar is
	// Java 21 bytecode, even though their vanilla floor is 17.
	// =======================================================================
	t0 := band("t0", "1.20.3", "1.20.4", "1.20.5", "1.20.6")
	emit("t0_java21", t0)
	emitCoverage("t0_java25", t0)
	emitCoverage("t0_java26", t0)

	// =======================================================================
	// STAGE 4 — mc1192 band (1.19-1.20.2, Option B): floor 17, coverage 21.
	// No 25/26 rows: pre-1.20.3 bands keep 21 as their newest coverage JVM.
	// =======================================================================
	mc1192 := band("mc1192", "1.19.2", "1.19.4", "1.20.1", "1.20.2")
	emit("mc1192_java17", mc1192)
	emitCoverage("mc1192_java21", mc1192)

	// =======================================================================
	// STAGE 5 — mc114 band (1.14-1.18, Option C): SPLIT floors — 1.14-1.16.x
	// boot on java 8, 1.17.x/1.18.x on java 17 (no Temurin 16 jre image exists
	// for 1.17's historical floor). Coverage row on 21 spans the whole band.
	// 1.16.5 / 1.17.1 / 1.18.2 are the targeted niches; 1.14.4 and 1.15.2 are
	// free-riders on the identical jar and cost nothing extra to assert.
	// =======================================================================
	mc114 := band("mc114", "1.14.4", "1.15.2", "1.16.5", "1.17.1", "1.18.2")
	var mc114j8, mc114j17 []string
	for _, v := range mc114 {
		if strings.HasPrefix(v, "1.14") || strings.HasPrefix(v, "1.15") || strings.HasPrefix(v, "1.16") {
			mc114j8 = append(mc114j8, v)
		} else {
			mc114j17 = append(mc114j17, v)
		}
	}
	emit("mc114_java8", mc114j8)
	emit("mc114_java17", mc114j17)
	emitCoverage("mc114_java21", mc114)

	if emitErr != nil {
		return emitErr
	}

	// -----------------------------------------------------------------------
	// Human-readable summary. GATED_PAIRS counts submatrix legs only; the
	// whole-run job count adds 4 (build-jars, unit-tests, two gate canaries).
	// -----------------------------------------------------------------------
	total := 0
	for _, r := range rows {
		total += r.n
		_, _ = fmt.Fprintf(stdout, "%-16s %3d  %s\n", r.name+":", r.n, r.json)
	}
	_, _ = fmt.Fprintf(stdout, "EVENT_NAME=%s\n", eventName)
	_, _ = fmt.Fprintf(stdout, "GATED_PAIRS=%d\n", total)
	_, _ = fmt.Fprintf(stdout, "TOTAL_JOBS=%d\n", total+4)
	return nil
}
