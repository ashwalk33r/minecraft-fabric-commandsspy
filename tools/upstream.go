package main

/*
 * upstream-drift compares the stable versions each upstream project publishes with the versions this repository
 * declares. It is the one check that notices a new Mojang or loader release, so a declared range cannot fall behind
 * upstream without a build going red. It reads `coverage` for the declared lists, so there is one source of truth.
 */

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// upstreamFeeds holds one URL per feed so tests can point every feed at an httptest server.
type upstreamFeeds struct {
	fabric, quilt, mojang, forge, neoforge, bta, babric string
}

var liveFeeds = upstreamFeeds{
	fabric:   "https://meta.fabricmc.net/v2/versions/game",
	quilt:    "https://meta.quiltmc.org/v3/versions/game",
	mojang:   "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
	forge:    "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json",
	neoforge: "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml",
	bta:      "https://downloads.betterthanadventure.net/bta-client/release/",
	babric:   "https://meta.babric.glass-launcher.net/v2/versions/game",
}

// upstreamBands maps each backend to the coverage bands whose declared lists together form its declared set.
var upstreamBands = map[string][]string{
	"fabric":   {"mc114", "mc1192", "t0", "mc121", "mc26"},
	"quilt":    {"mc114", "mc1192", "t0", "mc121", "mc26"},
	"forge":    {"forge_mc116", "forge_legacy", "forge", "forge_eventbus7"},
	"neoforge": {"neo"},
	"bta":      {"bta"},
	"babric":   {"babric"},
}

/*
 * upstreamExcluded lists upstream stable versions at or above a backend's floor that no band declares.
 *
 * Every entry needs a non-empty reason, the same contract `coverage[band].excluded` enforces for boots.
 */
var upstreamExcluded = map[string]map[string]string{
	"fabric": {
		"1.19": "no Fabric jar declares 1.19.0: mc114 stops below 1.19 and mc1192 starts at 1.19.1, because 1.19.0's execute() lacks the ParseResults overload the mc1192 jar hooks. CI boots it only as the out-of-range refusal guard",
	},
	"quilt": {
		"1.19": "no Fabric jar declares 1.19.0: mc114 stops below 1.19 and mc1192 starts at 1.19.1, because 1.19.0's execute() lacks the ParseResults overload the mc1192 jar hooks. CI boots it only as the out-of-range refusal guard",
	},
}

// releaseRe matches a Mojang release number, so snapshots, pre-releases, rcs and betas never count as stable.
var releaseRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)

var btaDirRe = regexp.MustCompile(`href="v([0-9][0-9._]*)/"`)

var upstreamClient = &http.Client{Timeout: 30 * time.Second}

// fetchFeed tries twice because one network blip on a required check would otherwise turn every PR red.
func fetchFeed(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := upstreamClient.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode != http.StatusOK:
			lastErr = fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
		default:
			return body, nil
		}
	}
	return nil, fmt.Errorf("feed unreachable after 2 attempts: %w", lastErr)
}

// metaVersions reads the `[{version, stable}]` shape that Fabric, Quilt and Babric meta all serve.
func metaVersions(url string, stableOnly bool) ([]string, error) {
	body, err := fetchFeed(url)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("malformed feed %s: %w", url, err)
	}
	var out []string
	for _, e := range entries {
		if !stableOnly || e.Stable {
			out = append(out, e.Version)
		}
	}
	return out, nil
}

func fabricStable(f upstreamFeeds) ([]string, error) { return metaVersions(f.fabric, true) }

func babricStable(f upstreamFeeds) ([]string, error) { return metaVersions(f.babric, true) }

// quiltStable ignores Quilt's own stable flag, because Mojang's manifest is the authority on what a release is.
func quiltStable(f upstreamFeeds) ([]string, error) {
	all, err := metaVersions(f.quilt, false)
	if err != nil {
		return nil, err
	}
	body, err := fetchFeed(f.mojang)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("malformed feed %s: %w", f.mojang, err)
	}
	release := map[string]bool{}
	for _, v := range manifest.Versions {
		if v.Type == "release" {
			release[v.ID] = true
		}
	}
	if len(release) == 0 {
		return nil, fmt.Errorf("malformed feed %s: no release entries", f.mojang)
	}
	return pick(all, func(v string) bool { return release[v] }), nil
}

// forgeStable counts only versions with a `-latest` or `-recommended` promotion, since only those have a build to boot.
func forgeStable(f upstreamFeeds) ([]string, error) {
	body, err := fetchFeed(f.forge)
	if err != nil {
		return nil, err
	}
	var feed struct {
		Promos map[string]string `json:"promos"`
	}
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("malformed feed %s: %w", f.forge, err)
	}
	var out []string
	for key := range feed.Promos {
		v, ok := strings.CutSuffix(key, "-latest")
		if !ok {
			v, ok = strings.CutSuffix(key, "-recommended")
		}
		if ok && releaseRe.MatchString(v) {
			out = append(out, v)
		}
	}
	return out, nil
}

/*
 * neoforgeStable maps NeoForge build numbers to Minecraft versions and drops every `-beta` or `-alpha` build.
 *
 * Three components (`21.1.77`) read as `1.21.1`, and four components (`26.3.0.92`) read as `26.3`.
 */
func neoforgeStable(f upstreamFeeds) ([]string, error) {
	body, err := fetchFeed(f.neoforge)
	if err != nil {
		return nil, err
	}
	var meta struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("malformed feed %s: %w", f.neoforge, err)
	}
	var out []string
	for _, build := range meta.Versions {
		if !releaseRe.MatchString(build) {
			continue
		}
		p := strings.Split(build, ".")
		var mc []string
		switch len(p) {
		case 3:
			mc = []string{"1", p[0], p[1]}
		case 4:
			mc = p[:3]
		default:
			continue
		}
		if mc[2] == "0" {
			mc = mc[:2]
		}
		out = append(out, strings.Join(mc, "."))
	}
	return out, nil
}

// btaStable reads release directory names from the index, because BTA publishes no machine-readable feed.
func btaStable(f upstreamFeeds) ([]string, error) {
	body, err := fetchFeed(f.bta)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range btaDirRe.FindAllStringSubmatch(string(body), -1) {
		out = append(out, "bta"+m[1])
	}
	return out, nil
}

// cmpUpstream orders versions across all three axes by dropping the `b`/`bta` prefix and reading `_` as a dot.
func cmpUpstream(a, b string) int {
	norm := func(v string) string { return strings.ReplaceAll(strings.TrimLeft(v, "abt"), "_", ".") }
	return cmpVer(norm(a), norm(b))
}

func runUpstreamDrift(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("takes no arguments, got %q", args)
	}
	declared := map[string][]string{}
	for backend, bands := range upstreamBands {
		for _, b := range bands {
			declared[backend] = append(declared[backend], coverage[b].declared...)
		}
	}
	return upstreamDrift(liveFeeds, declared, upstreamExcluded, os.Stdout)
}

/*
 * upstreamDrift fails when an upstream stable version at or above a backend's floor is neither declared nor excluded.
 *
 * An unreachable, malformed or empty feed is a failure too, because a check that skips proves nothing.
 */
func upstreamDrift(feeds upstreamFeeds, declared map[string][]string, excluded map[string]map[string]string,
	w io.Writer) error {
	for _, backend := range sortedKeys(excluded) {
		for _, v := range sortedKeys(excluded[backend]) {
			if strings.TrimSpace(excluded[backend][v]) == "" {
				return fmt.Errorf("upstreamExcluded[%q][%q] has an empty reason", backend, v)
			}
		}
	}
	backends := []struct {
		name   string
		stable func(upstreamFeeds) ([]string, error)
		// exact means the band is one version with no range, so every other upstream version is drift.
		exact bool
	}{
		{"fabric", fabricStable, false},
		{"quilt", quiltStable, false},
		{"forge", forgeStable, false},
		{"neoforge", neoforgeStable, false},
		{"bta", btaStable, false},
		{"babric", babricStable, true},
	}
	var problems []error
	drift := 0
	for _, b := range backends {
		versions, err := b.stable(feeds)
		if err == nil && len(versions) == 0 {
			err = errors.New("feed lists no stable version, so nothing was checked")
		}
		if err == nil && len(declared[b.name]) == 0 {
			err = errors.New("no declared version to compare against")
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", b.name, err))
			continue
		}
		floor := declared[b.name][0]
		have := map[string]bool{}
		for _, v := range declared[b.name] {
			have[v] = true
			if cmpUpstream(v, floor) < 0 {
				floor = v
			}
		}
		seen := map[string]bool{}
		for _, v := range versions {
			if b.exact || cmpUpstream(v, floor) >= 0 {
				seen[v] = true
			}
		}
		checked := sortedKeys(seen)
		sort.SliceStable(checked, func(i, j int) bool { return cmpUpstream(checked[i], checked[j]) < 0 })
		for _, v := range checked {
			if !have[v] && excluded[b.name][v] == "" {
				_, _ = fmt.Fprintf(w, "DRIFT %s %s: upstream stable, neither declared nor in upstreamExcluded\n", b.name, v)
				drift++
			}
		}
		_, _ = fmt.Fprintf(w, "%-8s %3d upstream stable version(s) at or above %s checked\n", b.name, len(checked), floor)
	}
	if drift > 0 {
		problems = append(problems, fmt.Errorf("%d upstream stable version(s) neither declared nor excluded", drift))
	}
	if len(problems) > 0 {
		return errors.Join(problems...)
	}
	_, _ = fmt.Fprintln(w, "PASS: every upstream stable version is declared or excluded with a reason")
	return nil
}
