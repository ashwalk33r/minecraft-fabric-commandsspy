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
//     gate, never duplicated here — that is a FABRIC-loader rule; the gate
//     runs those two versions on fabric/quilt only, so the forge and neoforge
//     rows below list them without duplicating anything.
//   - Forge and NeoForge bands emit floor rows only (see those stages below).
//     A new such band is one range-key case in bandPresent, one emit here, and
//     one uses: block in e2e.yml.
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

// Forge bands are keyed off their declared range lines in
// forge/gradle.properties, same idiom as t0 above.
var forgeRangeRe = map[string]*regexp.Regexp{
	"forge":           regexp.MustCompile(`(?m)^minecraft_range_modern=`),
	"forge_legacy":    regexp.MustCompile(`(?m)^minecraft_range_legacy=`),
	"forge_eventbus7": regexp.MustCompile(`(?m)^minecraft_range_eventbus7=`),
	"forge_mc116":     regexp.MustCompile(`(?m)^minecraft_range_mc116=`),
}

// The NeoForge band jar is keyed off its declared range line in
// neoforge/gradle.properties, same idiom as the Forge bands above. One band
// row only: a single jar covers 1.20.2-26.2.
var neoRangeRe = regexp.MustCompile(`(?m)^minecraft_range_neo_all=`)

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
	case "forge", "forge_legacy", "forge_eventbus7", "forge_mc116":
		data, err := os.ReadFile(filepath.Join(repoRoot, "forge", "gradle.properties"))
		return err == nil && forgeRangeRe[name].Match(data)
	case "neo":
		data, err := os.ReadFile(filepath.Join(repoRoot, "neoforge", "gradle.properties"))
		return err == nil && neoRangeRe.Match(data)
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
	// push-to-main builds and publishes jars but runs no e2e; FORCE_BANDS
	// included — an empty grid is the contract ci.yml relies on.
	push := eventName == "push"
	forced := strings.Fields(forceBands)

	type row struct {
		name string
		json string
		n    int
	}
	var rows []row
	var emitErr error

	emit := func(name string, versions []string) {
		if push {
			versions = nil
		}
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

	// FORGE — floor rows ONLY, no coverage rows, no lean/full split. The Forge
	// jars' own bytecode floors are what matter (legacy = java-17 uniform
	// across 1.17.1-1.20.4, modern = 21), NOT the per-MC-version fabric era
	// table above; forward-JVM coverage rows (the mc114_java17/mc114_java21
	// pattern) are a Fabric-jar concept and must not be reused with
	// LOADER=forge (scripts/e2e-run-one.sh overrides FLOOR_JAVA for
	// LOADER=forge for exactly this reason). This section is the single home
	// of the Forge leg rationale (e2e.yml's jobs just point here):
	//
	// Modern band: edges only — 1.20.6 and 1.21.5 are the measured floor and
	// ceiling, and the mapping regime and EventBus generation are uniform
	// across the range, so nothing can fail in the middle while both edges
	// pass. 1.20.4 rides in this java-21 job but routes to the LEGACY jar
	// in-range (--print-forge-routing 1.20.4 = "legacy 0"): it boots the
	// legacy jar's ceiling on a modern JVM — so it is keyed on the LEGACY
	// band's presence, not the modern one's; a modern-only tree has no
	// legacy jar for it to boot. (An older comment called it a refusal
	// GUARD leg; that was stale.)
	//
	// Legacy band: EVERY measured version, not just the edges — the R1
	// measurement's whole point was SRG member-id stability ACROSS Forge
	// major branches 37-49 (seven of them), so a floor+ceiling pair would
	// not exercise the thing being proven.
	//
	// mc116 band (1.14-1.16.5, Forge 28-36, issue #30): EVERY measured
	// version, same reasoning as legacy — the measurement's point was that
	// one SRG-renamed java-8 jar resolves and fires across five consecutive
	// pre-1.17 Forge major branches (28/31/32/33/34/36), so a floor+ceiling
	// pair would not exercise the thing being proven. All on java 8, the
	// era's real deployment JVM and the jar's own bytecode floor. 1.16.4
	// rides too, via a harness-side cure: its whole Forge 35.x line
	// predates the ModLauncher fix for the JDK 8u321+ ManifestEntryVerifier
	// change and cannot boot a STOCK current JDK 8, so e2e-run-one.sh drops
	// the fixed ModLauncher 8.1.3 (sha256-pinned) into the server install
	// at install time (see docs/version-matrix.md).
	// No sub-floor refusal guard leg exists either: Forge's next line down
	// (1.13.2) is below the harness's own 1.14 floor. The old
	// forge_legacy_guard_java8 leg (1.16.5 expected REFUSED) flipped to an
	// in-range PASS here.
	//
	// EventBus-7 band (1.21.6-26.2, Forge 56-65): EVERY measured version,
	// same reasoning as legacy — the measurement's point was that one
	// official-name java-21 jar registers and fires across ten consecutive
	// EventBus-7 Forge major branches (56-65), so a floor+ceiling pair would
	// not exercise the thing being proven. Split by the era Java floor the
	// generic table already assigns: 1.21.x on 21, 26.x (including the
	// 26.1.1/26.1.2 patch releases — each its own Forge major, 63/64) on 25.
	forgeModern := band("forge", "1.20.6", "1.21.5")
	if len(forgeModern) > 0 {
		forgeModern = append(band("forge_legacy", "1.20.4"), forgeModern...)
	}
	emit("forge_java21", forgeModern)
	emit("forge_legacy_java17", band("forge_legacy",
		"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19.1", "1.19.2",
		"1.20.1", "1.20.2", "1.20.3", "1.20.4"))
	emit("forge_mc116_java8", band("forge_mc116",
		"1.14.4", "1.15.2", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5"))
	emit("forge_eventbus7_java21", band("forge_eventbus7",
		"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11"))
	emit("forge_eventbus7_java25", band("forge_eventbus7",
		"26.1", "26.1.1", "26.1.2", "26.2"))

	// NEOFORGE — floor rows only, like Forge, but for a different reason: ONE
	// band jar covers 1.20.2-26.2 (measured; the metadata seam — FML 1.x/2.x
	// mods.toml+mandatory vs FML 3.x+ neoforge.mods.toml+type — is handled by
	// shipping both files in the one jar). So these legs are NOT the
	// cross-major stability proof the Forge legs are; there is no second jar
	// whose overlap could drift. They are the band EDGES (1.20.2, 26.2) plus
	// the three Java floors NEOFORGE ITSELF changes at (17 up to line 20.4, 21
	// through 21.11, 25 on 26.x — e2e-run-one.sh's neo routing table, NOT the
	// Fabric era table, which reports 21 for 1.20.4), plus 1.21.1, the
	// modpack-dominant interior line. Single-loader rows: LOADER=neoforge has
	// no quilt twin, which the job count at the bottom depends on.
	emit("neo_java17", band("neo", "1.20.2", "1.20.4"))
	emit("neo_java21", band("neo", "1.20.6", "1.21.1", "1.21.11"))
	emit("neo_java25", band("neo", "26.2"))

	if emitErr != nil {
		return emitErr
	}

	// GATED_PAIRS counts submatrix legs (versions x rows) once each.
	// TOTAL_JOBS counts what the workflow actually spawns: every fabric band
	// key feeds TWO caller jobs in ci.yml (-fabric and -quilt), forge_* and
	// neo_* keys feed ONE (LOADER=forge/neoforge have no quilt twin), plus the
	// 22 fixed jobs: contracts, go-quality, lint-java, unit-tests, the 9
	// build jobs, the Build aggregator, the 4 e2e-gate canaries (2 versions x
	// fabric/quilt), plus the 4 config-behaviors legs (#34, one per loader).
	// On push the gate canaries and config-behaviors legs are event-skipped,
	// leaving 14. The build jobs went 10 -> 9 when the two per-version
	// NeoForge jobs collapsed into the one band job, and the 2 literal
	// NeoForge e2e jobs this count used to carry are generated rows now.
	total, jobs := 0, 0
	for _, r := range rows {
		total += r.n
		// Prefix test, never strings.Contains(name, "forge"): that matches
		// "neoforge" too, and a neo_* row named that way would be counted as
		// a quilt pair the workflow never spawns.
		if strings.HasPrefix(r.name, "forge") || strings.HasPrefix(r.name, "neo") {
			jobs += r.n
		} else {
			jobs += 2 * r.n
		}
		_, _ = fmt.Fprintf(stdout, "%-16s %3d  %s\n", r.name+":", r.n, r.json)
	}
	_, _ = fmt.Fprintf(stdout, "EVENT_NAME=%s\n", eventName)
	_, _ = fmt.Fprintf(stdout, "GATED_PAIRS=%d\n", total)
	fixedJobs := 22
	if push {
		fixedJobs = 14
	}
	_, _ = fmt.Fprintf(stdout, "TOTAL_JOBS=%d\n", jobs+fixedJobs)
	return nil
}
