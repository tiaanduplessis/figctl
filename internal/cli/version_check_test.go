package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.1.0", false},
		{"v0.1.0", "v0.1.0", false},
		{"0.1.0", "v0.1.1", true},
		{"v1.0.0", "v1.0.1", true},
		{"v1.2.3", "v1.10.0", true},
		{"v2.0.0", "v1.9.9", false},
		// A build from source should be told what shipped.
		{"dev", "v0.1.0", true},
		{"", "v0.1.0", true},
		// Pre-release suffixes are ignored rather than guessed at.
		{"v0.1.0-rc1", "v0.1.0", false},
		{"v0.1.0", "v0.2.0-rc1", true},
		// Unparsable pairs fall back to inequality.
		{"nightly", "v0.1.0", true},
		{"nightly", "nightly", false},
	}
	for _, tt := range tests {
		if got := isNewer(tt.current, tt.latest); got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestVersionCheck(t *testing.T) {
	for _, tt := range []struct {
		name       string
		handler    http.HandlerFunc
		wantUpdate any
		wantHint   string
	}{
		{"a newer release exists", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v9.9.9"})
		}, true, "is available"},
		{"the feed is unreachable", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, nil, "Could not reach the release feed"},
		{"nothing is published yet", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, nil, "Could not reach the release feed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			original := ReleasesURL
			ReleasesURL = srv.URL
			defer func() { ReleasesURL = original }()

			r := execute(t, "", "version", "--check")
			// A failed lookup must not fail the command: the user asked for
			// the version, and they got it.
			ok(t, r)
			d := data(t, r, "version")
			if d["version"] == nil {
				t.Fatalf("version is always reported: %v", d)
			}
			if got := d["updateAvailable"]; got != tt.wantUpdate {
				t.Fatalf("updateAvailable = %v, want %v", got, tt.wantUpdate)
			}
			env := decodeEnvelope(t, r.stdout)
			hints, _ := env["hints"].([]any)
			found := false
			for _, h := range hints {
				if s, isString := h.(string); isString && strings.Contains(s, tt.wantHint) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a hint containing %q, got %v", tt.wantHint, hints)
			}
		})
	}
}

// TestVersionWithoutCheckMakesNoRequest keeps the plain command offline.
func TestVersionWithoutCheckMakesNoRequest(t *testing.T) {
	isolate(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	original := ReleasesURL
	ReleasesURL = srv.URL
	defer func() { ReleasesURL = original }()

	r := execute(t, "", "version")
	ok(t, r)
	if calls != 0 {
		t.Fatalf("figctl version must not reach the network, got %d call(s)", calls)
	}
	if d := data(t, r, "version"); d["updateAvailable"] != nil || d["latest"] != nil {
		t.Fatalf("no check was asked for: %v", d)
	}
}

// TestFillFromBuildInfo covers the go install path, where no linker flags are
// passed and the version has to come from the module Go recorded. Without it
// such a build reports itself as dev, which also makes --check claim an
// update is always available.
func TestFillFromBuildInfo(t *testing.T) {
	tests := []struct {
		name                             string
		info                             *debug.BuildInfo
		ok                               bool
		startVersion                     string
		wantVersion, wantCommit, wantAge string
	}{
		{
			name: "a module version fills in the placeholders",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v1.2.3"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abc123"},
					{Key: "vcs.time", Value: "2026-01-02T03:04:05Z"},
				},
			},
			ok:           true,
			startVersion: "dev",
			wantVersion:  "v1.2.3", wantCommit: "abc123", wantAge: "2026-01-02T03:04:05Z",
		},
		{
			name:         "a build from source is left as dev",
			info:         &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:           true,
			startVersion: "dev",
			wantVersion:  "dev", wantCommit: "none", wantAge: "unknown",
		},
		{
			name:         "linker flags win over the module version",
			info:         &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}},
			ok:           true,
			startVersion: "v9.9.9",
			wantVersion:  "v9.9.9", wantCommit: "none", wantAge: "unknown",
		},
		{
			name:         "no build info changes nothing",
			ok:           false,
			startVersion: "dev",
			wantVersion:  "dev", wantCommit: "none", wantAge: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldV, oldC, oldD := version, commit, date
			defer func() { version, commit, date = oldV, oldC, oldD }()
			version, commit, date = tt.startVersion, "none", "unknown"

			fillFromBuildInfo(func() (*debug.BuildInfo, bool) { return tt.info, tt.ok })

			if version != tt.wantVersion || commit != tt.wantCommit || date != tt.wantAge {
				t.Fatalf("got %s/%s/%s, want %s/%s/%s",
					version, commit, date, tt.wantVersion, tt.wantCommit, tt.wantAge)
			}
		})
	}
}
