package main

// gen-matrix is the single source of e2e stage definitions. Every {band, Java}
// submatrix in .github/workflows/e2e.yml reads its version list from an output
// emitted here; adding or removing a band is a change to this file plus one
// uses: block in e2e.yml.
//
// Contract with the workflow (see docs/ci.md):
//   - EVERY output name is emitted on EVERY run — the literal [] for an absent
//     band. A missing output evaluates to '' in GitHub expressions and
//     fromJSON('') hard-errors a matrix.
//   - The two gate canaries (1.21.11/java21, 26.2/java25) are moved to the
//     gate, never duplicated here.
//
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
// forced (FORCE_BANDS) overrides detection for offline testing.
func bandPresent(repoRoot, name string, forced []string) bool {
	for _, f := range forced {
		if f == name {
			return true
		}
	}
	switch name {
	case "t0":
		// mc121's jar covers >=1.20.3 (void-descriptor era), so key off the range.
		data, err := os.ReadFile(filepath.Join(repoRoot, "gradle.properties"))
		return err == nil && t0RangeRe.Match(data)
	case "mc1192":
		// mc1192 (>=1.19.1 <1.20.3) and mc114 (>=1.14 <1.19) are real source
		// sets; the dir is the proof.
		st, err := os.Stat(filepath.Join(repoRoot, "src", "mc1192", "java"))
		return err == nil && st.IsDir()
	case "mc114":
		st, err := os.Stat(filepath.Join(repoRoot, "src", "mc114", "java"))
		return err == nil && st.IsDir()
	}
	return false
}

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
		// writes are tracked via emitErr, so Close's error adds nothing.
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

	// STAGE 2 — current mainstream (1.21.x floor 21, 26.x floor 25).
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

	// STAGE 3 — t0 band (1.20.3-1.20.6): floor 21, coverage 25/26. 1.20.3-1.20.6
	// run the mc121 jar, which is Java 21 bytecode, even though the vanilla
	// floor is 17.
	t0 := band("t0", "1.20.3", "1.20.4", "1.20.5", "1.20.6")
	emit("t0_java21", t0)
	emitCoverage("t0_java25", t0)
	emitCoverage("t0_java26", t0)

	// STAGE 4 — mc1192 band (1.19-1.20.2): floor 17, coverage 21 only.
	mc1192 := band("mc1192", "1.19.2", "1.19.4", "1.20.1", "1.20.2")
	emit("mc1192_java17", mc1192)
	emitCoverage("mc1192_java21", mc1192)

	// STAGE 5 — mc114 band (1.14-1.18): split floors 8 / 17, coverage 21.
	// 1.17 floor is 17: no Temurin 16 jre image exists.
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

	// GATED_PAIRS counts submatrix legs only (+4 for build, unit tests, two canaries).
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
