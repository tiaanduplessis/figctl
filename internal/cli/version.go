package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// ReleasesURL is where --check looks for the newest published release. It is
// a variable so tests can point it at a stub server.
var ReleasesURL = "https://api.github.com/repos/tiaanduplessis/figctl/releases/latest"

// ReleasesPage is shown to a user who wants to download a new version.
const ReleasesPage = "https://github.com/tiaanduplessis/figctl/releases"

// Set at build time through -ldflags (see the Makefile).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	// Latest is the newest published release, set only by --check.
	Latest string `json:"latest,omitempty"`
	// UpdateAvailable is nil unless --check reached the release feed.
	UpdateAvailable *bool `json:"updateAvailable,omitempty"`
	// CheckError explains why --check could not answer.
	CheckError string `json:"checkError,omitempty"`
}

func (v versionInfo) Columns() []string { return []string{"field", "value"} }

func (v versionInfo) Rows() [][]string {
	kv := output.KeyValues{
		{"version", v.Version},
		{"commit", v.Commit},
		{"date", v.Date},
		{"goVersion", v.GoVersion},
		{"os", v.OS},
		{"arch", v.Arch},
	}
	if v.Latest != "" {
		kv = append(kv, [2]string{"latest", v.Latest})
	}
	if v.UpdateAvailable != nil {
		kv = append(kv, [2]string{"updateAvailable", fmt.Sprint(*v.UpdateAvailable)})
	}
	if v.CheckError != "" {
		kv = append(kv, [2]string{"checkError", v.CheckError})
	}
	return kv.Rows()
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the figctl version",
	Example: "  figctl version\n  figctl version --json",
	Args:    cobra.NoArgs,
}

func init() {
	check := versionCmd.Flags().Bool("check", false, "check for a newer release")
	versionCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		info := versionInfo{
			Version:   version,
			Commit:    commit,
			Date:      date,
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
		}
		env := ctx.Envelope(info)
		if !*check {
			return ctx.Printer.Print(env)
		}
		rctx, cancel := context.WithTimeout(context.Background(), ctx.Flags.Timeout)
		defer cancel()
		latest, err := latestRelease(rctx, ctx.Flags.Timeout)
		switch {
		case err != nil:
			// Not being able to reach the feed is not a failure of the
			// command the user ran; report it and carry on.
			info.CheckError = err.Error()
			env = ctx.Envelope(info)
			env.AddHint("Could not reach the release feed. See " + ReleasesPage + " for new versions.")
		default:
			info.Latest = latest
			newer := isNewer(version, latest)
			info.UpdateAvailable = &newer
			env = ctx.Envelope(info)
			if newer {
				env.AddHint("figctl " + latest + " is available; this is " + version + ". Download it from " + ReleasesPage + ", or upgrade the way you installed it.")
			} else {
				env.AddHint("This is the newest published release.")
			}
		}
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(versionCmd)
}

// latestRelease reads the newest published release tag. It never updates
// anything: figctl is installed by a package manager or a downloaded archive,
// and replacing its own binary would fight whichever one the user chose.
func latestRelease(ctx context.Context, timeout time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ReleasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "figctl/"+version)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting the release feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", figctl.New(figctl.CodeNotFound, "no release has been published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the release feed answered %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding the release feed: %w", err)
	}
	if body.TagName == "" {
		return "", fmt.Errorf("the release feed named no tag")
	}
	return body.TagName, nil
}

// isNewer compares two release names. A development build is always treated
// as older so that a contributor running from source is told what shipped,
// and an unparsable pair falls back to inequality rather than guessing.
func isNewer(current, latest string) bool {
	if current == "dev" || current == "" {
		return true
	}
	cur, curOK := parseVersion(current)
	lat, latOK := parseVersion(latest)
	if !curOK || !latOK {
		return strings.TrimPrefix(current, "v") != strings.TrimPrefix(latest, "v")
	}
	for i := range cur {
		if cur[i] != lat[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

// parseVersion reads a major.minor.patch tag, ignoring a leading v and any
// pre-release or build suffix.
func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
