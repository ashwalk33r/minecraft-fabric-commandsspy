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
//   - Forge and NeoForge bands emit floor rows plus ONE forward-JVM row each
//     (forge_java26, neo_fwd_java25) — not the Fabric per-band coverage-row
//     pattern; see those stages below for why the two are different things.
//     A new such band is one range-key case in bandPresent, one emit here, and
//     one uses: block in e2e.yml.
//   - Which VERSIONS a row lists is not decided at the emit call: it comes from
//     the coverage table below, which states each band's declared range, the
//     sample every event boots, the deep list workflow_dispatch boots, and the
//     reason for every declared version booted by neither. See THE SAMPLING
//     RULE below and "The denominator, settled" in the wiki's
//     Supported-Versions page.
//
import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// Versions the FABRIC jar boots and QUILT LOADER has no build for, so they run
// as a fabric-only row instead of a permanently red quilt twin (issue #69).
// Membership is an upstream fact — meta.quiltmc.org/v3/versions/game lists
// 1.14.4 and not plain 1.14 — which contracts cannot re-probe, because that is
// a network call and contracts makes none. What IS asserted offline, in
// gen_matrix_test.go, is that every version named here leaves the shared row
// and lands in the _fabric one: the set cannot be edited without the grid
// following it.
var quiltUnavailable = map[string]bool{"1.14": true}

func ends(list []string) []string {
	if len(list) < 2 {
		return list
	}
	return []string{list[0], list[len(list)-1]}
}

// The 1.21 line in release order. 1.21.11 is the java-21 gate canary: it is
// dropped from the FLOOR row below, never from the band's coverage.
var all121 = []string{"1.21", "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5",
	"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11"}

// ---------------------------------------------------------------------------
// THE SAMPLING RULE (issue #59)
//
// A jar's minecraft_range_* is what the LOADER accepts. It is deliberately
// wider than what CI proves, and until this table the difference was
// invisible: the grid booted a sample of each range, the sample's rationale
// lived only in prose comments, and nothing failed when a version quietly fell
// out of one. The "full" workflow_dispatch grid did not close that gap either
// — emitCoverage widened only the newest-Java rows, so the per-loader
// Minecraft version SET was byte-identical on both events and 50
// (loader, version) pairs sat inside a declared range that no CI event of any
// kind booted.
//
// Each band now states its version contract as data:
//
//	declared — every release on the axis this repo names (see "What 'covered'
//	           settled" in the wiki's Supported-Versions) that this band's declared
//	           minecraft_range_* covers. Restated here INDEPENDENTLY of the
//	           emit calls below; that independence is what makes the invariant
//	           bite.
//	sampled  — booted on every e2e event. The rows below split it by Java floor.
//	deep     — booted on workflow_dispatch, which is now the deep sweep its
//	           name always implied. Always a superset of sampled.
//	excluded — declared, and booted by nothing on any event, each with the
//	           reason it is not. Three different situations end up here and the
//	           reason has to say which, because they are not the same claim:
//	           the LOADER refuses the version; the loader project never
//	           published a build for it, so there is nothing to install at all;
//	           or this HARNESS gates it for a reason of its own. Only the first
//	           is re-probeable offline — see scripts/test-jar-routing.sh.
//
// The invariant gen_matrix_test.go asserts is `deep + excluded == declared`,
// exactly, per band. A version therefore cannot leave the grid by being
// deleted from a list; it can only leave by acquiring a written reason.
//
// minecraft_range_121 (>=1.20.3 <1.22) is split across two entries, t0 and
// mc121, because the grid splits that one jar's range into two bands that
// sample differently.
type bandCoverage struct {
	declared []string
	sampled  []string
	deep     []string
	excluded map[string]string
}

var coverage = map[string]bandCoverage{
	// Exhaustive already: every 1.21 patch is booted, 1.21.11 by the gate and
	// the rest by the floor row, so the deep sweep adds nothing here.
	"mc121": {declared: all121, sampled: all121, deep: all121},

	// >=26.1 <26.3. The two patch releases are the only Fabric-side exclusion
	// with a technical cause rather than a budget one.
	"mc26": {
		declared: []string{"26.1", "26.1.1", "26.1.2", "26.2"},
		sampled:  []string{"26.1", "26.2"},
		deep:     []string{"26.1", "26.2"},
		excluded: map[string]string{
			"26.1.1": "26.x mapping breaks land on the MINOR boundaries, not the patch releases, and each extra version costs a full server download (the wiki, Supported-Versions -> \"The Makefile's default version list\")",
			"26.1.2": "same as 26.1.1 — and the Forge eventbus7 band boots both, so a patch-level break would still surface there",
		},
	},

	// The 1.20.x half of minecraft_range_121: four versions, all booted.
	"t0": {
		declared: []string{"1.20.3", "1.20.4", "1.20.5", "1.20.6"},
		sampled:  []string{"1.20.3", "1.20.4", "1.20.5", "1.20.6"},
		deep:     []string{"1.20.3", "1.20.4", "1.20.5", "1.20.6"},
	},

	// >=1.19.1 <1.20.3. The sample is both ends of the 1.19 line, the
	// most-run legacy version and the 1.20.2/1.20.3 boundary; the deep sweep
	// adds the three the sample skips, 1.19.1 among them — the jar's own
	// floor, named in scripts/test-jar-routing.sh as a boundary probe and
	// until now booted by nothing.
	"mc1192": {
		declared: []string{"1.19.1", "1.19.2", "1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2"},
		sampled:  []string{"1.19.2", "1.19.4", "1.20.1", "1.20.2"},
		deep:     []string{"1.19.1", "1.19.2", "1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2"},
	},

	// >=1.14 <1.19. 1.16 is on the axis for the reason it is in
	// test-jar-routing.sh's EXPECTED table: 1.15.2|1.16 is the exact edge
	// where the RCON source name flips from Recon to Rcon. It was named there
	// and booted nowhere; the deep sweep is where it now boots.
	"mc114": {
		declared: []string{"1.14", "1.14.4", "1.15.2", "1.16", "1.16.1", "1.16.2",
			"1.16.3", "1.16.4", "1.16.5", "1.17", "1.17.1", "1.18", "1.18.1", "1.18.2"},
		sampled: []string{"1.14.4", "1.15.2", "1.16.5", "1.17.1", "1.18.2"},
		deep: []string{"1.14", "1.14.4", "1.15.2", "1.16", "1.16.1", "1.16.2",
			"1.16.3", "1.16.4", "1.16.5", "1.17", "1.17.1", "1.18", "1.18.1", "1.18.2"},
	},

	// Forge modern, [1.20.6,1.21.6). The sample is the measured floor and
	// ceiling; the interior is uniform in mapping regime and EventBus
	// generation, which is why the sample stops there and why the deep sweep
	// is the right place for the other five. (1.20.4 rides in the same job but
	// belongs to forge_legacy — see the FORGE section below.)
	"forge": {
		declared: []string{"1.20.6", "1.21", "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5"},
		sampled:  []string{"1.20.6", "1.21.1", "1.21.5"},
		deep:     []string{"1.20.6", "1.21", "1.21.1", "1.21.3", "1.21.4", "1.21.5"},
		excluded: map[string]string{
			"1.21.2": "no Forge build published: the promotions feed goes 51.0.33 (1.21) -> 52.1.16 (1.21.1) -> 53.1.12 (1.21.3) with nothing for 1.21.2, so e2e-run-one.sh exits at no-forge-build-for-version before a container starts. Same situation as 1.14 and 1.16 in the mc116 band, found the same way: the first deep sweep on main booted it and it could not install",
		},
	},

	// Forge legacy, [1.17.1,1.20.5). Every MEASURED version is sampled, since
	// the proof is SRG member-id stability across Forge majors 37-49; the deep
	// sweep adds the three in-range versions the measurement never covered.
	"forge_legacy": {
		declared: []string{"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19", "1.19.1", "1.19.2",
			"1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2", "1.20.3", "1.20.4"},
		sampled: []string{"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19.1", "1.19.2",
			"1.20.1", "1.20.2", "1.20.3", "1.20.4"},
		deep: []string{"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19.1", "1.19.2",
			"1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2", "1.20.3", "1.20.4"},
		excluded: map[string]string{
			"1.19": "blocked by the harness, not by Forge: Forge publishes 41.1.0 for 1.19 on both channels and would load the mod here. What stops it is the shared era-routing gate in scripts/e2e-run-one.sh, which rejects 1.19 before any loader routing runs because the FABRIC mc1192 jar's floor is 1.19.1 (1.19.0's execute() lacks the ParseResults overload that jar hooks). The only way past is FABRIC_EXPECT_REFUSED=1, the wrong flag to set on a leg whose point is Forge coverage",
		},
	},

	// Forge mc116, [1.14,1.17). Every measured version is sampled, same
	// cross-major reasoning as legacy. The two exclusions are not budget
	// calls: FORGE_KNOWN_GOOD_MC116 in scripts/e2e-run-one.sh does not list
	// them, so the harness itself expects a refusal.
	"forge_mc116": {
		declared: []string{"1.14", "1.14.4", "1.15.2", "1.16", "1.16.1", "1.16.2",
			"1.16.3", "1.16.4", "1.16.5"},
		sampled: []string{"1.14.4", "1.15.2", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5"},
		deep:    []string{"1.14.4", "1.15.2", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5"},
		excluded: map[string]string{
			"1.14": "no Forge build published: the promotions feed goes 1.13.2 -> 1.14.2 and carries no 1.14 key at all, so there is nothing to install. FORGE_EXPECT_REFUSED does raise here, but only because 1.14 is missing from FORGE_KNOWN_GOOD_MC116 — that flag means unproven-by-CI, not out-of-range, and it is not the reason this version cannot run",
			"1.16": "no Forge build published: the promotions feed jumps 1.15 -> 1.16.1. The Fabric mc114 band boots 1.16 in the deep sweep, so the 1.15.2|1.16 RCON-name edge is still proven — on the loader whose jar declares it",
		},
	},

	// Forge eventbus7, [1.21.6,26.3). Every measured version is sampled — the
	// proof is that one official-name jar fires across Forge majors 56-65 —
	// so the deep sweep adds nothing.
	"forge_eventbus7": {
		declared: []string{"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11",
			"26.1", "26.1.1", "26.1.2", "26.2"},
		sampled: []string{"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11",
			"26.1", "26.1.1", "26.1.2", "26.2"},
		deep: []string{"1.21.6", "1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11",
			"26.1", "26.1.1", "26.1.2", "26.2"},
	},

	// NeoForge, [1.20.2,26.3) — one band jar, so the sample is the band edges
	// plus NeoForge's own three Java floors plus the modpack-dominant 1.21.1.
	// The eight exclusions are the lines whose newest NeoForge build is a
	// BETA: booting a beta build would make CI's green depend on prerelease
	// loader code, which is a different claim from the one this repo makes.
	// Verified with --print-neo-routing, and asserted in
	// scripts/test-jar-routing.sh so the list cannot go stale when NeoForge
	// promotes one of them.
	"neo": {
		declared: []string{"1.20.2", "1.20.3", "1.20.4", "1.20.5", "1.20.6",
			"1.21", "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5", "1.21.6",
			"1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11",
			"26.1", "26.1.1", "26.1.2", "26.2"},
		sampled: []string{"1.20.2", "1.20.4", "1.20.6", "1.21.1", "1.21.11", "26.2"},
		deep: []string{"1.20.2", "1.20.4", "1.20.6", "1.21", "1.21.1", "1.21.3",
			"1.21.4", "1.21.5", "1.21.8", "1.21.10", "1.21.11", "26.1.2", "26.2"},
		excluded: map[string]string{
			"1.20.3": "newest NeoForge build for this line is 20.3.8-beta",
			"1.20.5": "newest NeoForge build for this line is 20.5.21-beta",
			"1.21.2": "newest NeoForge build for this line is 21.2.1-beta",
			"1.21.6": "newest NeoForge build for this line is 21.6.20-beta",
			"1.21.7": "newest NeoForge build for this line is 21.7.25-beta",
			"1.21.9": "newest NeoForge build for this line is 21.9.16-beta",
			"26.1":   "newest NeoForge build for this line is 26.1.0.19-beta",
			"26.1.1": "newest NeoForge build for this line is 26.1.1.15-beta",
		},
	},
}

// booted returns the band's version list for this event: the sample on every
// event, the wider deep-sweep list on workflow_dispatch. An unknown name is a
// programming error, not a data condition — every caller passes a literal.
func booted(name string, full bool) []string {
	c, ok := coverage[name]
	if !ok {
		panic("gen-matrix: no coverage entry for band " + name)
	}
	if full {
		return c.deep
	}
	return c.sampled
}

// neoFloor is NeoForge's OWN Java floor for a Minecraft version, i.e. which of
// the three neo_java* rows a version belongs in. It mirrors the neo routing
// table in scripts/e2e-run-one.sh (NOT the Fabric era table, which reports 21
// for 1.20.4); floors_test.go checks every emitted row against
// --print-neo-routing, so a drift here fails offline.
func neoFloor(v string) string {
	switch {
	case is26(v):
		return "25"
	case v == "1.20.2", v == "1.20.3", v == "1.20.4":
		return "17"
	default:
		return "21"
	}
}

// pick returns the members of list for which keep reports true, in order.
func pick(list []string, keep func(string) bool) []string {
	var out []string
	for _, v := range list {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func is26(v string) bool { return strings.HasPrefix(v, "26.") }

// printCoverage dumps the coverage table as `band<TAB>state<TAB>version<TAB>reason`
// so scripts/test-jar-routing.sh can probe it instead of restating it. The
// exclusions in particular carry claims about the world — "this NeoForge line
// only has a beta build", "Forge refuses this version" — and a claim nobody
// re-checks is how the table goes stale the day NeoForge promotes a build.
func printCoverage(w io.Writer) error {
	for _, name := range sortedKeys(coverage) {
		c := coverage[name]
		for _, s := range []struct {
			state    string
			versions []string
		}{{"declared", c.declared}, {"sampled", c.sampled}, {"deep", c.deep}} {
			for _, v := range s.versions {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t\n", name, s.state, v); err != nil {
					return err
				}
			}
		}
		for _, v := range sortedKeys(c.excluded) {
			if _, err := fmt.Fprintf(w, "%s\texcluded\t%s\t%s\n", name, v, c.excluded[v]); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func runGenMatrix(args []string) error {
	for _, a := range args {
		if a == "--coverage" {
			return printCoverage(os.Stdout)
		}
	}
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
	// workflow_dispatch is the DEEP SWEEP: it boots each band's `deep` list
	// instead of its `sampled` one, and widens the newest-Java coverage rows
	// from the band ends to the whole sample. Before issue #59 it did only the
	// second of those, which made its per-loader Minecraft version set
	// byte-identical to the pull_request grid's — the name promised a full
	// grid and delivered extra Java legs over the same versions.
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

	// Every version list below comes from the coverage table above: floor rows
	// take booted(band, full) — the sample, or the deep list on
	// workflow_dispatch — and the newest-Java coverage rows deliberately keep
	// taking the SAMPLE, so the deep sweep's delta is new Minecraft versions
	// and nothing else.
	sampledOf := func(name string) []string { return coverage[name].sampled }

	// STAGE 2 — current mainstream (1.21.x floor 21, 26.x floor 25).
	// 1.21.11 is the java-21 gate canary and must not appear in the j21 list.
	mc121 := booted("mc121", full)
	floor121 := mc121[:len(mc121)-1]

	emit("mc121_java21", floor121)
	if full {
		emit("mc121_java25", mc121)
		emit("mc121_java26", mc121)
	} else {
		emit("mc121_java25", ends(mc121))
		emit("mc121_java26", ends(mc121))
	}

	// 26.2 is the java-25 gate canary and must not appear in the java-25 list.
	mc26 := booted("mc26", full)
	emit("mc26_java25", mc26[:len(mc26)-1])
	emit("mc26_java26", mc26)

	// STAGE 3 — t0 band (1.20.3-1.20.6): floor 21, coverage 25/26. 1.20.3-1.20.6
	// run the mc121 jar, which is Java 21 bytecode, even though the vanilla
	// floor is 17.
	emit("t0_java21", band("t0", booted("t0", full)...))
	t0Sample := band("t0", sampledOf("t0")...)
	emitCoverage("t0_java25", t0Sample)
	emitCoverage("t0_java26", t0Sample)

	// STAGE 4 — mc1192 band (1.19-1.20.2): floor 17, coverage 21 only.
	emit("mc1192_java17", band("mc1192", booted("mc1192", full)...))
	emitCoverage("mc1192_java21", band("mc1192", sampledOf("mc1192")...))

	// STAGE 5 — mc114 band (1.14-1.18): split floors 8 / 17, coverage 21.
	// 1.17 floor is 17: no Temurin 16 jre image exists.
	mc114j8 := func(v string) bool {
		return strings.HasPrefix(v, "1.14") || strings.HasPrefix(v, "1.15") || strings.HasPrefix(v, "1.16")
	}
	mc114 := band("mc114", booted("mc114", full)...)
	// Versions the FABRIC jar boots and the QUILT one cannot. Quilt Loader is
	// not published for every Minecraft release Fabric supports —
	// meta.quiltmc.org/v3/versions/game lists 1.14.4 but not plain 1.14 — so
	// the installer produces no quilt-server-launch.jar and the leg dies with
	// "Unable to access jarfile" before a server exists. That is a Quilt fact,
	// not a jar fact: the mc114 jar declares 1.14 and Fabric boots it green.
	//
	// The coverage table above is keyed per BAND, which cannot express
	// "declared, booted on one loader, unbootable on the other" — both caller
	// jobs read one list. Rather than drop the version (losing real Fabric
	// coverage) or leave a permanently red Quilt leg, the affected versions
	// move to a fabric-only row and the shared row keeps the rest. The band's
	// deep list is unchanged, so deep + excluded == declared still holds; what
	// changes is which caller job boots which version. See issue #69.
	mc114j8List := pick(mc114, mc114j8)
	emit("mc114_java8", pick(mc114j8List, func(v string) bool { return !quiltUnavailable[v] }))
	emit("mc114_java8_fabric", pick(mc114j8List, func(v string) bool { return quiltUnavailable[v] }))
	emit("mc114_java17", pick(mc114, func(v string) bool { return !mc114j8(v) }))
	emitCoverage("mc114_java21", band("mc114", sampledOf("mc114")...))

	// FORGE — floor rows plus ONE forward-JVM row, no lean/full split. The
	// Forge jars' own bytecode floors are what decide the floor rows (legacy =
	// java-17 uniform across 1.17.1-1.20.4, modern = 21), NOT the
	// per-MC-version fabric era table above; scripts/e2e-run-one.sh overrides
	// FLOOR_JAVA for LOADER=forge for exactly that reason, and the Fabric
	// per-band coverage-row pattern (mc114_java17 -> mc114_java21, the whole
	// band re-run one JVM up) still must not be reused here: it samples a jar
	// family whose bytecode floor varies per Minecraft version, which is not
	// how the Forge jars are cut.
	//
	// What DOES carry over is the reason those Fabric rows exist. Bytecode
	// binds downward, so a java-21 jar on java 26 cannot fail to LINK — but
	// bytecode is not the only thing a newer JVM changes, and this project has
	// the scar to prove it: Forge 35.x cannot boot a stock current JDK 8 at
	// all, because 8u321+ changed an internal
	// sun.security.util.ManifestEntryVerifier constructor that 2020-era
	// ModLauncher links against (the wiki, Version-Boundaries-And-Root-Causes;
	// e2e-run-one.sh cures
	// it with an install-time ModLauncher 8.1.3 drop-in). That is a
	// forward-JVM failure with no mod and no bytecode in it, and it landed on
	// the loader that had no forward-JVM row. So each mapping/EventBus era
	// gets an above-floor data point instead of the two that used to fall out
	// incidentally (1.20.4's legacy jar riding the java-21 job, and
	// eventbus7's java-21 bytecode on the 26.x era's java 25): forge_java26
	// boots 26.2 on the newest JVM the harness has. The MODERN band gets
	// none, at any JVM: its bootstrap cannot resolve modules on java 24+
	// (issue #66; see the row's comment below). One row, not a per-band pair,
	// because what is being probed is the JVM.
	//
	// This section is the single home of the Forge leg rationale (e2e.yml's
	// jobs just point here):
	//
	// Modern band: the sample is the two edges plus one interior line.
	// 1.20.6 and 1.21.5 are the measured floor and ceiling, and the mapping
	// regime and EventBus generation are uniform across the range, so as a
	// MAPPING-REGIME probe the edges alone would do — nothing can fail in the
	// middle while both edges pass. That argues for not paying for the middle
	// on every pull request, not for never checking it, so the deep sweep
	// boots it. 1.21.1 is sampled on EVERY event and is not there as a
	// mapping-regime probe: it must not be tidied back out on the grounds
	// that both edges already pass (issue #57). It is the modpack-dominant
	// Forge line, the version a real server operator is most likely to run,
	// so its proof should be a band row rather than a side effect of
	// e2e-config-behaviors-forge, whose versions: literal happens to be
	// ["1.21.1"] but whose job is blacklist suppression and logArguments.
	// Same reasoning as the 1.21.1 row in the NeoForge stage below.
	// 1.20.4 rides in this java-21 job but routes to the LEGACY jar
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
	// at install time (see the wiki, Version-Boundaries-And-Root-Causes).
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
	forgeModern := band("forge", booted("forge", full)...)
	if len(forgeModern) > 0 {
		forgeModern = append(band("forge_legacy", "1.20.4"), forgeModern...)
	}
	emit("forge_java21", forgeModern)
	emit("forge_legacy_java17", band("forge_legacy", booted("forge_legacy", full)...))
	emit("forge_mc116_java8", band("forge_mc116", booted("forge_mc116", full)...))
	eb7 := band("forge_eventbus7", booted("forge_eventbus7", full)...)
	emit("forge_eventbus7_java21", pick(eb7, func(v string) bool { return !is26(v) }))
	emit("forge_eventbus7_java25", pick(eb7, is26))
	// The forward-JVM row (issue #58). One version: 26.2, the eventbus7 band's
	// ceiling, on the newest JVM the harness has. The modern band has no
	// java-26 probe and cannot have one at any JVM: its bootstrap has ZERO
	// forward headroom above its java-21 floor. nimbus-jose-jwt's module-info
	// requires jdk.crypto.ec, a JDK module REMOVED in java 24 (EC folded into
	// java.base), so net.minecraftforge.bootstrap 2.1.7 dies in module
	// resolution before Minecraft starts — "FindException: Module
	// jdk.crypto.ec not found, required by com.nimbusds.jose.jwt" — measured
	// on java 25 and java 26 alike, and 21/25/26 is the whole ladder above its
	// floor (issue #66, and the wiki's Supported-Versions -> "Forge modern is Java 21 only").
	// That is upstream Forge's bug,
	// not this mod's, and it is a compatibility fact for the version matrix
	// rather than a leg: an inverted guard could never change state and would
	// have to match a third party's stack trace to mean anything, since "the
	// boot failed" goes green for any failure, including one this mod causes.
	// What this row proves is the other half — the eventbus7 bootstrap does
	// not have the problem. Keyed on the band so a tree without it emits an
	// empty row instead of a leg with no jar. The plain forge_javaN name
	// stands because Forge row names have never promised a floor (forge_java21
	// already carries 1.20.4, whose Forge floor is 17); floors_test checks the
	// row against --print-forge-routing.
	emit("forge_java26", band("forge_eventbus7", "26.2"))

	// NEOFORGE — floor rows plus one forward-JVM row, like Forge, but the
	// floor rows are floor rows for a different reason than Forge's: ONE
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
	//
	// The deep sweep adds the seven stable interior lines. It does NOT add the
	// eight whose newest NeoForge build is a BETA — booting those would make a
	// green CI run depend on prerelease loader code, which is a different claim
	// from the one this repo makes. Which eight is not a guess: the coverage
	// table names each with its beta build, and scripts/test-jar-routing.sh
	// re-probes them, so the list cannot go stale the day NeoForge promotes
	// one.
	neo := band("neo", booted("neo", full)...)
	for _, floor := range []string{"17", "21", "25"} {
		emit("neo_java"+floor, pick(neo, func(v string) bool { return neoFloor(v) == floor }))
	}
	// The forward-JVM row (issue #58): before it, EVERY NeoForge leg ran at
	// exactly its floor, so the one band jar's "java-17 bytecode boots
	// anywhere in 17/21/25" claim was asserted only at the three floors — and
	// the ManifestEntryVerifier precedent above says the JVM can break a
	// loader with no bytecode question involved. 1.21.1 is the interior line
	// to spend it on: modpack-dominant, and its NeoForge floor is 21, so
	// running it on 25 is a real step up. Deliberately NOT folded into
	// neo_java25 and deliberately not named neo_java*: floors_test pins every
	// neo_java<N> row to "every version's NeoForge floor is exactly N", which
	// is the tripwire for an upstream floor moving, and folding would have
	// meant weakening that equality on all three floor rows to buy one job
	// block of YAML. This row gets the opposite assertion instead — its
	// versions must sit strictly BELOW java 25, or it is not forward coverage.
	emit("neo_fwd_java25", band("neo", "1.21.1"))

	if emitErr != nil {
		return emitErr
	}

	// GATED_PAIRS counts submatrix legs (versions x rows) once each.
	// TOTAL_JOBS counts what the workflow actually spawns: every fabric band
	// key feeds TWO caller jobs in ci.yml (-fabric and -quilt), forge_* and
	// neo_* keys feed ONE (LOADER=forge/neoforge have no quilt twin), plus the
	// 25 fixed jobs: contracts, go-quality, lint-java, unit-tests, the 9
	// build jobs, the Build aggregator, the 4 e2e-gate canaries (2 versions x
	// fabric/quilt), the 4 config-behaviors legs (#34, one per loader), and
	// the 3 out-of-range refusal guards — fabric + quilt on 1.19.0, the version
	// no declared minecraft_range_* covers, plus forge on 1.21.6 handed the
	// modern jar, whose declared range excludes it (#56; Forge published no
	// build for either of its own holes, 1.17 and 1.20.5, so the mismatch has
	// to be made on the jar axis rather than the version axis). On push the
	// gate canaries, the config-behaviors legs and the refusal guards are all
	// event-skipped, leaving 14. The build jobs went 10 -> 9 when the two per-version
	// NeoForge jobs collapsed into the one band job, and the 2 literal
	// NeoForge e2e jobs this count used to carry are generated rows now.
	total, jobs := 0, 0
	for _, r := range rows {
		total += r.n
		// Prefix test, never strings.Contains(name, "forge"): that matches
		// "neoforge" too, and a neo_* row named that way would be counted as
		// a quilt pair the workflow never spawns.
		// Single-loader rows spawn ONE caller job; every other row spawns two,
		// a -fabric and a -quilt. Prefix test for forge/neo, never
		// strings.Contains(name, "forge"): that matches "neoforge" too. The
		// _fabric suffix marks a row whose versions Quilt cannot boot at all
		// (issue #69) — it has no quilt twin either.
		if strings.HasPrefix(r.name, "forge") || strings.HasPrefix(r.name, "neo") ||
			strings.HasSuffix(r.name, "_fabric") {
			jobs += r.n
		} else {
			jobs += 2 * r.n
		}
		_, _ = fmt.Fprintf(stdout, "%-16s %3d  %s\n", r.name+":", r.n, r.json)
	}
	_, _ = fmt.Fprintf(stdout, "EVENT_NAME=%s\n", eventName)
	_, _ = fmt.Fprintf(stdout, "GATED_PAIRS=%d\n", total)
	fixedJobs := 25
	if push {
		fixedJobs = 14
	}
	_, _ = fmt.Fprintf(stdout, "TOTAL_JOBS=%d\n", jobs+fixedJobs)
	return nil
}
