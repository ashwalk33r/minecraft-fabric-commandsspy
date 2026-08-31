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
	"strconv"
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
	case "babric":
		// No range regex: the band is one hardcoded version, so the only
		// question is whether the separate Gradle build ships at all.
		st, err := os.Stat(filepath.Join(repoRoot, "babric", "build.gradle"))
		return err == nil && !st.IsDir()
	case "bta":
		// Same shape as babric: the declared set is an enumerated list in
		// bta/gradle.properties rather than an interval, so there is no range
		// regex to match — only whether the separate Gradle build ships.
		st, err := os.Stat(filepath.Join(repoRoot, "bta", "build.gradle"))
		return err == nil && !st.IsDir()
	}
	return false
}

// Versions the FABRIC jar boots and QUILT LOADER has no build for, so they run
// as a fabric-only row instead of a permanently red quilt twin (issue #69).
// Membership is an upstream fact — meta.quiltmc.org/v3/versions/game starts the
// 1.14 line at 1.14.4 — which contracts cannot re-probe, because that is a
// network call and contracts makes none. What IS asserted offline, in
// gen_matrix_test.go, is that every version named here leaves the shared row
// and lands in the _fabric one: the set cannot be edited without the grid
// following it.
//
// It is also what makes the PUBLISHED quilt list differ from the fabric one:
// the mc1.14.x jar ships as two Modrinth versions, fabric and fabric+quilt, so
// a version Quilt cannot install is never advertised on Quilt (issue #84).
// 1.14.1-1.14.3 joined 1.14 here when #84 enumerated the declared range: the
// same feed lists none of the four.
var quiltUnavailable = map[string]bool{
	"1.14": true, "1.14.1": true, "1.14.2": true, "1.14.3": true,
}

func ends(list []string) []string {
	if len(list) < 2 {
		return list
	}
	return []string{list[0], list[len(list)-1]}
}

// ---------------------------------------------------------------------------
// THE RELEASE AXIS (issue #84)
//
// Every Mojang release from 1.14 (the oldest version any band declares) to the
// newest, in release order. It exists because the PUBLISHED version lists —
// Modrinth's game_versions — are the declared ranges enumerated over exactly
// this axis, and until #84 nothing in the repository could enumerate them: each
// band's `declared` list was hand-written, and five real releases inside
// declared ranges (1.14.1 1.14.2 1.14.3 1.15 1.15.1) appeared in no list here
// while being advertised on Modrinth all the same.
//
// A version joins this list when Mojang ships it, not when this repository
// decides to care about it. gen_matrix_test.go asserts every band's `declared`
// equals `releasesIn(<the band's minecraft_range_* line>)`, read from the real
// gradle.properties, so the axis cannot drift from the ranges the jars ship.
//
// Snapshots (1.14.5, 22w13oneblockatatime, ...) are not releases and are not
// here; b1.7.3 is not on this axis either — the babric band declares an exact
// version, not an interval, so there is nothing to enumerate.
var mojangAxis = []string{
	"1.14", "1.14.1", "1.14.2", "1.14.3", "1.14.4",
	"1.15", "1.15.1", "1.15.2",
	"1.16", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5",
	"1.17", "1.17.1",
	"1.18", "1.18.1", "1.18.2",
	"1.19", "1.19.1", "1.19.2", "1.19.3", "1.19.4",
	"1.20", "1.20.1", "1.20.2", "1.20.3", "1.20.4", "1.20.5", "1.20.6",
	"1.21", "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5", "1.21.6",
	"1.21.7", "1.21.8", "1.21.9", "1.21.10", "1.21.11",
	"26.1", "26.1.1", "26.1.2", "26.2",
}

// cmpVer orders two dotted numeric Minecraft versions. Field-wise integer
// compare, not string compare: "1.21.10" sorts after "1.21.9", and "26.2" after
// "1.22". A missing field reads as 0, so "1.21" < "1.21.1".
func cmpVer(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// releasesIn enumerates mojangAxis over a declared range, in either notation
// the repository uses: the Fabric loader's `>=1.14 <1.19` and the Maven
// interval `[1.14,1.17)` Forge and NeoForge write. Both bounds are optional and
// each may be inclusive or exclusive; an exact version (`[1.21.1]`, or a bare
// `1.0.0-beta.7.3`) yields that version if the axis holds it.
//
// This is the one place a declared range turns into a version list. The
// coverage table below and the published game_versions lists both run through
// it, which is what makes them the same claim rather than two lists that agree
// until someone edits one.
func releasesIn(spec string) []string {
	lo, hi := "", ""
	loInc, hiInc := true, true
	spec = strings.TrimSpace(spec)
	switch {
	case strings.HasPrefix(spec, "[") || strings.HasPrefix(spec, "("):
		loInc = spec[0] == '['
		hiInc = strings.HasSuffix(spec, "]")
		body := strings.Trim(spec, "[]()")
		lo, hi, _ = strings.Cut(body, ",")
		if !strings.Contains(body, ",") {
			hi = lo // [1.21.1] — an exact version
		}
	default:
		for _, tok := range strings.Fields(spec) {
			switch {
			case strings.HasPrefix(tok, ">="):
				lo, loInc = tok[2:], true
			case strings.HasPrefix(tok, ">"):
				lo, loInc = tok[1:], false
			case strings.HasPrefix(tok, "<="):
				hi, hiInc = tok[2:], true
			case strings.HasPrefix(tok, "<"):
				hi, hiInc = tok[1:], false
			default:
				lo, hi, loInc, hiInc = tok, tok, true, true
			}
		}
	}
	lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
	out := []string{}
	for _, v := range mojangAxis {
		if lo != "" {
			if c := cmpVer(v, lo); c < 0 || (c == 0 && !loInc) {
				continue
			}
		}
		if hi != "" {
			if c := cmpVer(v, hi); c > 0 || (c == 0 && !hiInc) {
				continue
			}
		}
		out = append(out, v)
	}
	return out
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
//	declared — every Mojang release this band's minecraft_range_* covers,
//	           enumerated over mojangAxis by releasesIn(). Since issue #84 this
//	           is the WIDE reading: the same set the published Modrinth
//	           game_versions list advertises, not the narrower "every release
//	           this repo names" the wiki settled on for #59. The two differed by
//	           five real releases — 1.14.1 1.14.2 1.14.3 1.15 1.15.1 — that were
//	           advertised to users and booted by nothing, which is exactly the
//	           gap #84 measured. Stated independently of the emit calls below;
//	           that independence is what makes the invariant bite.
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

	// >=26.1 <26.3. Every release in the interval, all booted by the deep
	// sweep since issue #84: the two patch releases used to be excluded as a
	// budget call, and they were published on Modrinth all the same.
	"mc26": {
		declared: releasesIn(">=26.1 <26.3"),
		sampled:  []string{"26.1", "26.2"},
		deep:     []string{"26.1", "26.1.1", "26.1.2", "26.2"},
	},

	// The 1.20.x half of minecraft_range_121: four versions, all booted.
	"t0": {
		declared: releasesIn(">=1.20.3 <1.21"),
		sampled:  []string{"1.20.3", "1.20.4", "1.20.5", "1.20.6"},
		deep:     []string{"1.20.3", "1.20.4", "1.20.5", "1.20.6"},
	},

	// >=1.19.1 <1.20.3. The sample is both ends of the 1.19 line, the
	// most-run legacy version and the 1.20.2/1.20.3 boundary; the deep sweep
	// adds the three the sample skips, 1.19.1 among them — the jar's own
	// floor, named in scripts/test-jar-routing.sh as a boundary probe and
	// until now booted by nothing.
	"mc1192": {
		declared: releasesIn(">=1.19.1 <1.20.3"),
		sampled:  []string{"1.19.2", "1.19.4", "1.20.1", "1.20.2"},
		deep:     []string{"1.19.1", "1.19.2", "1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2"},
	},

	// >=1.14 <1.19, every release in it. The five the repo used to name
	// nowhere — 1.14.1 1.14.2 1.14.3 1.15 1.15.1 — boot in the deep sweep since
	// #84; three of them are Quilt-less and ride the fabric-only row. 1.16
	// matters for the reason it is in test-jar-routing.sh's EXPECTED table:
	// 1.15.2|1.16 is the exact edge where the RCON source name flips from Recon
	// to Rcon.
	// 1.18 and 1.18.1 are sampled, not deep-only, and that is deliberate: they
	// were the last two versions of the band where quilt-loader silently never
	// invoked the "main" entrypoint (QuiltMC/quilt-loader#500, fixed in 0.30.1).
	// While the gap was open the banner was asserted expected-absent on them and
	// nothing on a pull request booted them, so the assertion that matters most
	// there ran only on a manual dispatch. Now that the banner is asserted
	// PRESENT on every version, every version the gap ever covered boots on
	// every pull request.
	"mc114": {
		declared: releasesIn(">=1.14 <1.19"),
		sampled:  []string{"1.14.4", "1.15.2", "1.16.5", "1.17.1", "1.18", "1.18.1", "1.18.2"},
		deep:     releasesIn(">=1.14 <1.19"),
	},

	// Forge modern, [1.20.6,1.21.6). The sample is the measured floor and
	// ceiling; the interior is uniform in mapping regime and EventBus
	// generation, which is why the sample stops there and why the deep sweep
	// is the right place for the other five. (1.20.4 rides in the same job but
	// belongs to forge_legacy — see the FORGE section below.)
	"forge": {
		declared: releasesIn("[1.20.6,1.21.6)"),
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
		declared: releasesIn("[1.17.1,1.20.5)"),
		sampled: []string{"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19.1", "1.19.2",
			"1.20.1", "1.20.2", "1.20.3", "1.20.4"},
		deep: []string{"1.17.1", "1.18", "1.18.1", "1.18.2", "1.19.1", "1.19.2",
			"1.19.3", "1.19.4", "1.20", "1.20.1", "1.20.2", "1.20.3", "1.20.4"},
		excluded: map[string]string{
			"1.19": "blocked by the harness, not by Forge: Forge publishes 41.1.0 for 1.19 on both channels and would load the mod here. What stops it is the shared era-routing gate in scripts/e2e-run-one.sh, which rejects 1.19 before any loader routing runs because the FABRIC mc1192 jar's floor is 1.19.1 (1.19.0's execute() lacks the ParseResults overload that jar hooks). The only way past is FABRIC_EXPECT_REFUSED=1, the wrong flag to set on a leg whose point is Forge coverage",
		},
	},

	// Forge mc116, [1.14.4,1.17). Every measured version is sampled, same
	// cross-major reasoning as legacy; the deep sweep adds 1.15 and 1.15.1,
	// the two releases #84 enumerated into the range that Forge both publishes
	// and this jar's javafml range accepts. The range floor moved 1.14 ->
	// 1.14.4 in the same change: 1.14/1.14.1 have no Forge build, and
	// 1.14.2/1.14.3 are Forge 26.x/27.x, which FML refuses against the jar's
	// own forge_range_mc116=[28,37). The one exclusion left is 1.16, which
	// Forge never published.
	"forge_mc116": {
		declared: releasesIn("[1.14.4,1.17)"),
		sampled:  []string{"1.14.4", "1.15.2", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5"},
		deep: []string{"1.14.4", "1.15", "1.15.1", "1.15.2",
			"1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5"},
		excluded: map[string]string{
			"1.16": "no Forge build published: the promotions feed jumps 1.15 -> 1.16.1. The Fabric mc114 band boots 1.16 in the deep sweep, so the 1.15.2|1.16 RCON-name edge is still proven — on the loader whose jar declares it",
		},
	},

	// Forge eventbus7, [1.21.6,26.3). Every measured version is sampled — the
	// proof is that one official-name jar fires across Forge majors 56-65 —
	// so the deep sweep adds nothing.
	"forge_eventbus7": {
		declared: releasesIn("[1.21.6,26.3)"),
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
		declared: releasesIn("[1.20.2,26.3)"),
		sampled:  []string{"1.20.2", "1.20.4", "1.20.6", "1.21.1", "1.21.11", "26.2"},
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

	// Babric, b1.7.3 and nothing else — the only Minecraft version the loader
	// exists for. declared == sampled == deep, so the deep sweep adds nothing
	// and there is no exclusion to write a reason for. One leg, one JVM (21),
	// Tier 1 on every pull request: a Tier 2 row on a frozen platform would rot
	// unnoticed, and this is the cheapest band in the grid.
	"babric": {
		declared: []string{"b1.7.3"},
		sampled:  []string{"b1.7.3"},
		deep:     []string{"b1.7.3"},
	},

	// BTA — "Better than Adventure!", a fork of the GAME, not another loader for
	// Beta 1.7.3. The axis is BTA's own version line, so every token carries a
	// `bta` prefix: `7.3` on an axis whose other members are `1.21.11` and `26.2`
	// would be unreadable and one renumbering from a real collision.
	//
	// declared is an enumerated list of BTA's stable releases from 7.3 up, and
	// 7.3 is a hard floor: `net/minecraft/core/net/command/CommandManager.class`
	// — the Brigadier dispatcher this jar's one mixin targets — is absent from
	// the 7.1 and 7.2 jars. Prereleases are never declared, booted or published:
	// a green CI run must not depend on prerelease game code, the same reason the
	// neo band skips beta builds.
	//
	// The sample buys three distinct facts for three package downloads: bta7.3 is
	// the seam floor, bta7.3_04 is both the first `bta_fabric_server_` asset name
	// and the first underscore build the enumerated jar predicate must match, and
	// bta8.0.1 is the head, proving the seam survived a BTA major. Everything else
	// is a patch inside an already-proven line — what the deep sweep is for.
	// deep == declared, so `excluded` is empty and there is no reason to write.
	"bta": {
		declared: []string{"bta7.3", "bta7.3_01", "bta7.3_02", "bta7.3_03", "bta7.3_04", "bta8.0", "bta8.0.1"},
		sampled:  []string{"bta7.3", "bta7.3_04", "bta8.0.1"},
		deep:     []string{"bta7.3", "bta7.3_01", "bta7.3_02", "bta7.3_03", "bta7.3_04", "bta8.0", "bta8.0.1"},
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

// ---------------------------------------------------------------------------
// THE PUBLISHED LISTS (issue #84)
//
// One row per Modrinth version: the jar as it is uploaded, the loaders it is
// tagged with, and the game_versions list. Before this, those lists were
// hand-carried from one release to the next and matched nothing in the
// repository — 40 of 1.7.0's 113 loader-and-version claims were backed by no CI
// leg at all, and one of them (Quilt 1.14) could not be installed by anybody.
//
// game_versions is the band's DECLARED range — what the loader accepts — minus
// the versions nothing can be installed on:
//
//   - a loader that never published a build for the version (the "no Forge
//     build published" exclusions; Quilt's missing 1.14-1.14.3). There is no
//     file to download, so advertising it is a claim with no product behind it.
//   - nothing else. A version excluded from CI for a reason that is about CI —
//     a beta-only NeoForge line, a harness gate — is still installable and
//     stays listed.
//
// Everything else that is declared is now booted: the deep sweep's version set
// IS the declared set minus those exclusions, which is what closes #84's gap.
//
// The mc1.14.x jar emits TWO rows, fabric and fabric+quilt, because Modrinth
// cannot say "this version, but not on that loader" inside one version — the
// same jar is uploaded twice so the Quilt list can be the shorter one.
type publishedJar struct {
	name    string // the Modrinth version suffix, e.g. "mc1.14.x"
	loaders string
	bands   []string
	quilt   bool // drop the versions Quilt Loader has no build for
	// gameVersions overrides the store's version list instead of deriving it
	// from the bands' declared sets. Exactly one row needs it: BTA, whose
	// declared tokens are BTA releases and therefore name nothing Modrinth's
	// game-version tag list contains.
	gameVersions []string
}

var publishedJars = []publishedJar{
	// Two uploads of ONE jar. The fabric+quilt row keeps the plain suffix — it is
	// the version that already exists on Modrinth and the one most users install
	// — and the fabric-only row, which carries the versions Quilt has no build
	// for, takes a `-fabric` suffix. Modrinth version numbers must be distinct,
	// so the suffix is not decoration: it is the release step's file name.
	{name: "mc1.14.x", loaders: "fabric,quilt", bands: []string{"mc114"}, quilt: true},
	{name: "mc1.14.x-fabric", loaders: "fabric", bands: []string{"mc114"}},
	{name: "mc1.19-1.20.2", loaders: "fabric,quilt", bands: []string{"mc1192"}, quilt: true},
	{name: "mc1.21.x", loaders: "fabric,quilt", bands: []string{"t0", "mc121"}, quilt: true},
	{name: "mc26.x", loaders: "fabric,quilt", bands: []string{"mc26"}, quilt: true},
	{name: "mc1.16.x-forge", loaders: "forge", bands: []string{"forge_mc116"}},
	{name: "mc1.17-1.20.4-forge", loaders: "forge", bands: []string{"forge_legacy"}},
	{name: "mc1.21.x-forge", loaders: "forge", bands: []string{"forge"}},
	{name: "mc1.21.6-26.2-forge", loaders: "forge", bands: []string{"forge_eventbus7"}},
	{name: "mc1.20.2-26.2-neoforge", loaders: "neoforge", bands: []string{"neo"}},
	{name: "mcb1.7.3-babric", loaders: "babric", bands: []string{"babric"}},
	// BTA's uploads are the one place a jar's game_versions are NOT its declared
	// list. Modrinth's loader tag list carries `bta-babric`, but its game-version
	// list carries no BTA version at all — `b1.7.3`, BTA's base version, is the
	// only entry that exists — so the store claim is pinned there and the jar's
	// own enumerated depends.minecraft is what actually refuses an unbooted BTA
	// release at install time. The changelog line carries the real range.
	{name: "bta7.3-8.0.1-bta", loaders: "bta-babric", bands: []string{"bta"}, gameVersions: []string{"b1.7.3"}},
}

// noBuildPublished marks the exclusions that mean "the loader project shipped
// nothing for this version", the only reason that removes a version from a
// published list. Matched on the reason's prefix, which printCoverage already
// emits, so one sentence carries both the CI exclusion and the store claim.
const noBuildPublished = "no Forge build published:"

// publishedVersions is game_versions for one uploaded jar.
func publishedVersions(j publishedJar) []string {
	if len(j.gameVersions) > 0 {
		return j.gameVersions
	}
	out := []string{}
	for _, b := range j.bands {
		c := coverage[b]
		for _, v := range c.declared {
			if strings.HasPrefix(c.excluded[v], noBuildPublished) {
				continue
			}
			if j.quilt && quiltUnavailable[v] {
				continue
			}
			out = append(out, v)
		}
	}
	return out
}

// printPublish dumps the published lists as `jar<TAB>loaders<TAB>game_versions`.
// docs/modrinth-versions.tsv is this output, committed, and gen_matrix_test.go
// fails when the two differ — so the store lists cannot drift from the coverage
// table without a build going red first. Regenerate, never hand-edit:
//
//	cd tools && REPO_ROOT=.. go run . gen-matrix --publish > ../docs/modrinth-versions.tsv
func printPublish(w io.Writer) error {
	for _, j := range publishedJars {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n",
			j.name, j.loaders, strings.Join(publishedVersions(j), ",")); err != nil {
			return err
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
		if a == "--publish" {
			return printPublish(os.Stdout)
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

	// STAGE 7 — Babric. One row, because the band is one version and one JVM.
	// Java 21 is the LOADER STACK's floor, not the game's: b1.7.3 itself
	// predates every modern JVM, but the Babric/Ornithe chain and the mixin
	// compatibilityLevel are pinned at 21. No coverage row above the floor —
	// there is no second Java the toolchain is pinned for.
	emit("babric_java21", band("babric", booted("babric", full)...))

	// STAGE 8 — BTA. One row, one JVM. Java 17 is the LOADER STACK's floor: the
	// game's own classes are Java 8 bytecode, but BTA's loader fork sets the Mixin
	// compatibility level to JAVA_17 itself and BTA's own docs point users at
	// OpenJDK 17. No coverage row above the floor -- declaring 21 would refuse the
	// JVM the platform recommends, and booting only 21 would assert a floor CI
	// never ran.
	emit("bta_java17", band("bta", booted("bta", full)...))

	if emitErr != nil {
		return emitErr
	}

	// GATED_PAIRS counts submatrix legs (versions x rows) once each.
	// TOTAL_JOBS counts what the workflow actually spawns: every fabric band
	// key feeds TWO caller jobs in ci.yml (-fabric and -quilt), forge_* and
	// neo_* keys feed ONE (LOADER=forge/neoforge/babric have no quilt twin;
	// babric_java21 is the one Babric row), plus the
	// 26 fixed jobs: contracts, go-quality, lint-java, unit-tests, the 10
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
	// Babric moved BOTH numbers by exactly one, and the two terms are separate:
	// +1 fixed job is build-babric (9 build jobs -> 10, so 25 -> 26 and the
	// push subset 14 -> 15), and +1 gated pair is the babric_java21 row's one
	// version, which spawns one caller job because Babric has no quilt twin.
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
			strings.HasPrefix(r.name, "babric") || strings.HasPrefix(r.name, "bta") ||
			strings.HasSuffix(r.name, "_fabric") {
			jobs += r.n
		} else {
			jobs += 2 * r.n
		}
		_, _ = fmt.Fprintf(stdout, "%-16s %3d  %s\n", r.name+":", r.n, r.json)
	}
	_, _ = fmt.Fprintf(stdout, "EVENT_NAME=%s\n", eventName)
	_, _ = fmt.Fprintf(stdout, "GATED_PAIRS=%d\n", total)
	fixedJobs := 27
	if push {
		fixedJobs = 16
	}
	_, _ = fmt.Fprintf(stdout, "TOTAL_JOBS=%d\n", jobs+fixedJobs)
	return nil
}
