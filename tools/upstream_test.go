package main

// Offline: every feed is an httptest server serving a fixture, so make ci needs no network.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// driftFixtures publish 26.3 on every Mojang-numbered backend, plus the noise the stable filters must drop.
func driftFixtures() map[string]string {
	return map[string]string{
		"/fabric": `[{"version":"26.4-snapshot-1","stable":false},{"version":"26.3","stable":true},
			{"version":"26.3-rc-1","stable":false},{"version":"26.2","stable":true}]`,
		"/quilt": `[{"version":"26.4-snapshot-1","stable":false},{"version":"26.3","stable":true},
			{"version":"26.3-rc-1","stable":false},{"version":"26.2","stable":true}]`,
		"/mojang": `{"versions":[{"id":"26.4-snapshot-1","type":"snapshot"},{"id":"26.3","type":"release"},
			{"id":"26.3-rc-1","type":"snapshot"},{"id":"26.2","type":"release"}]}`,
		"/forge": `{"promos":{"26.2-latest":"65.0.1","26.3-latest":"66.0.13","26.4":"67.0.0"}}`,
		"/neoforge": `<metadata><versioning><versions><version>26.2.0.64</version>
			<version>26.3.0.5-beta</version><version>26.3.0.92</version></versions></versioning></metadata>`,
		"/bta":    `<a href="v7.2/">v7.2/</a><a href="v7.3/">v7.3/</a><a href="v8.0.1/">v8.0.1/</a>`,
		"/babric": `[{"version":"b1.7.3","stable":true}]`,
	}
}

// noiseOnly keeps 26.3 only as an rc, a beta build and an unpromoted Forge key, so a dead filter would surface it.
func noiseOnly() map[string]string {
	f := driftFixtures()
	f["/fabric"] = `[{"version":"26.4-snapshot-1","stable":false},{"version":"26.3-rc-1","stable":false},
		{"version":"26.2","stable":true}]`
	f["/quilt"] = f["/fabric"]
	f["/mojang"] = `{"versions":[{"id":"26.4-snapshot-1","type":"snapshot"},{"id":"26.3-rc-1","type":"snapshot"},
		{"id":"26.2","type":"release"}]}`
	f["/forge"] = `{"promos":{"26.2-latest":"65.0.1","26.3":"66.0.13","26.4-snapshot-1-latest":"67.0.0"}}`
	f["/neoforge"] = `<metadata><versioning><versions><version>26.2.0.64</version>
		<version>26.3.0.5-beta</version><version>26.4.0.0-alpha.1+snapshot-1</version></versions></versioning></metadata>`
	return f
}

// declaredTo262 mirrors the tree before the 26.3 bump.
var declaredTo262 = map[string][]string{
	"fabric":   {"26.1", "26.2"},
	"quilt":    {"26.1", "26.2"},
	"forge":    {"26.1", "26.2"},
	"neoforge": {"26.1", "26.2"},
	"bta":      {"bta7.3", "bta8.0.1"},
	"babric":   {"b1.7.3"},
}

func serveFeeds(t *testing.T, bodies map[string]string) upstreamFeeds {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	u := srv.URL
	return upstreamFeeds{u + "/fabric", u + "/quilt", u + "/mojang", u + "/forge", u + "/neoforge", u + "/bta", u + "/babric"}
}

func TestUpstreamDriftNamesEveryBackendBehindUpstream(t *testing.T) {
	var out bytes.Buffer
	if err := upstreamDrift(serveFeeds(t, driftFixtures()), declaredTo262, nil, &out); err == nil {
		t.Fatalf("want failure, got pass:\n%s", out.String())
	}
	for _, line := range []string{"DRIFT fabric 26.3", "DRIFT quilt 26.3", "DRIFT forge 26.3", "DRIFT neoforge 26.3"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output lacks %q:\n%s", line, out.String())
		}
	}
	if n := strings.Count(out.String(), "DRIFT"); n != 4 {
		t.Errorf("want 4 DRIFT lines, got %d:\n%s", n, out.String())
	}
}

func TestUpstreamDriftPassesWhenTheGapHasAReason(t *testing.T) {
	excluded := map[string]map[string]string{}
	for _, b := range []string{"fabric", "quilt", "forge", "neoforge"} {
		excluded[b] = map[string]string{"26.3": "not booted yet"}
	}
	var out bytes.Buffer
	if err := upstreamDrift(serveFeeds(t, driftFixtures()), declaredTo262, excluded, &out); err != nil {
		t.Fatalf("want pass, got %v:\n%s", err, out.String())
	}
}

func TestUpstreamDriftRejectsAnEmptyReason(t *testing.T) {
	excluded := map[string]map[string]string{"fabric": {"26.3": " "}}
	err := upstreamDrift(serveFeeds(t, noiseOnly()), declaredTo262, excluded, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "empty reason") {
		t.Fatalf("want an empty-reason failure, got %v", err)
	}
}

func TestUpstreamDriftIgnoresPrereleasesAndUnpromotedBuilds(t *testing.T) {
	var out bytes.Buffer
	if err := upstreamDrift(serveFeeds(t, noiseOnly()), declaredTo262, nil, &out); err != nil {
		t.Fatalf("want pass, got %v:\n%s", err, out.String())
	}
}

func TestUpstreamDriftFailsOnABrokenFeed(t *testing.T) {
	cases := map[string]func(f map[string]string){
		"unreachable":      func(f map[string]string) { delete(f, "/forge") },
		"malformed json":   func(f map[string]string) { f["/fabric"] = `{not json` },
		"malformed xml":    func(f map[string]string) { f["/neoforge"] = `<metadata><versioning>` },
		"empty feed":       func(f map[string]string) { f["/babric"] = `[]` },
		"no release dirs":  func(f map[string]string) { f["/bta"] = `<html>moved</html>` },
		"mojang malformed": func(f map[string]string) { f["/mojang"] = `{"versions":[]}` },
	}
	for name, breakFeed := range cases {
		t.Run(name, func(t *testing.T) {
			f := noiseOnly()
			breakFeed(f)
			if err := upstreamDrift(serveFeeds(t, f), declaredTo262, nil, &bytes.Buffer{}); err == nil {
				t.Fatal("want failure, got pass")
			}
		})
	}
}

// The live check's own exclusions must carry reasons, or runUpstreamDrift fails before fetching anything.
func TestUpstreamExcludedHasReasonsAndKnownBackends(t *testing.T) {
	for backend, m := range upstreamExcluded {
		if _, ok := upstreamBands[backend]; !ok {
			t.Errorf("upstreamExcluded names unknown backend %q", backend)
		}
		for v, reason := range m {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("upstreamExcluded[%q][%q] has an empty reason", backend, v)
			}
		}
	}
	for backend, bands := range upstreamBands {
		for _, b := range bands {
			if _, ok := coverage[b]; !ok {
				t.Errorf("upstreamBands[%q] names unknown band %q", backend, b)
			}
		}
	}
}
