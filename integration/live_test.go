//go:build integration

// Package integration holds the opt-in live test: it drives the built
// figctl binary against the real Figma REST API to catch the defects a
// fake server cannot show, such as a document Figma refuses to return
// whole, a slow optional request, a scope the by-key endpoints really
// need, or an endpoint whose 404 does not mean the file is missing.
//
// It never runs in the normal suite. The build tag keeps it out of
// go test ./..., and the guard skips it unless FIGCTL_INTEGRATION=1 and
// FIGMA_TOKEN are both set. Run it by hand with:
//
//	make build
//	FIGCTL_INTEGRATION=1 FIGMA_TOKEN=... go test -tags integration ./integration/...
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// Environment variables that steer the live test.
const (
	// enableEnv must be "1" for the test to run at all.
	enableEnv = "FIGCTL_INTEGRATION"
	// tokenEnv holds the Figma personal access token.
	tokenEnv = "FIGMA_TOKEN"
	// fileEnv overrides the target file key.
	fileEnv = "FIGCTL_INTEGRATION_FILE"
	// nodeEnv overrides the target node id.
	nodeEnv = "FIGCTL_INTEGRATION_NODE"
	// binEnv overrides the path of the binary under test.
	binEnv = "FIGCTL_INTEGRATION_BIN"
	// baseURLEnv is the API host override the CLI itself honors; the
	// request accounting follows it so a proxy is still counted.
	baseURLEnv = "FIGMA_API_BASE_URL"
)

// Defaults target the public figma-export demo file, which any token can
// read. It carries components, styles with real values, and vector icons,
// and node 54:22 is a component with export settings.
const (
	defaultFileKey = "fzYhvQpqwhZDUImRz431Qo"
	defaultNodeID  = "54:22"
	defaultAPIHost = "api.figma.com"
)

// tier1Budget is the upper bound on Tier 1 requests (GET file, GET nodes,
// GET images) for the whole run. Tier 1 is limited to as few as 10 per
// minute, which is why this test is nightly and not on every PR.
//
// Expected spend, with one cache directory shared by every step and no
// --refresh or --no-cache anywhere:
//
//	file info                 1  GET file at depth 2
//	file tree                 1  GET file whole document
//	file tree (second call)   0  served from the cache
//	file find                 0  served from the cache
//	node inspect              0-1 GET nodes for the style nodes it resolves
//	node context              1-3 screenshot render, icon render, style nodes
//	render --format svg       1  GET images
//	assets export --dry-run   0  no image request by definition
//	tokens export             0-1 GET nodes for the whole style catalogue
//	styles list               0  same style node batch, already cached
//	me, cache status, 404     0  Tier 3 endpoints only
//
// so roughly 8, and never more than tier1Budget.
const tier1Budget = 10

// traceLine matches the verbose response trace the client writes to
// stderr: "debug: 200 GET https://api.figma.com/v1/files/KEY (12 bytes...)".
var traceLine = regexp.MustCompile(`^debug: (\d{3}) ([A-Z]+) (\S+)`)

// envelope is the success envelope, decoded loosely so a shape change
// shows up as a failed assertion rather than a decode error.
type envelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Command       string          `json:"command"`
	Profile       *profile        `json:"profile"`
	File          *fileRef        `json:"file"`
	Data          json.RawMessage `json:"data"`
	Truncated     bool            `json:"truncated"`
	NextCursor    *string         `json:"nextCursor"`
	Hints         []string        `json:"hints"`
}

type profile struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
}

type fileRef struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// errorEnvelope is the error envelope, which JSON mode writes to stdout.
type errorEnvelope struct {
	SchemaVersion int `json:"schemaVersion"`
	Error         struct {
		Code       figctl.Code `json:"code"`
		Message    string      `json:"message"`
		Hint       string      `json:"hint"`
		HTTPStatus int         `json:"httpStatus"`
	} `json:"error"`
}

// runtime is the shared state of the run: the binary, the isolated
// environment, the one cache directory every step reuses, and the running
// Tier 1 request count.
type runtime struct {
	bin      string
	workdir  string
	cacheDir string
	env      []string
	fileKey  string
	nodeID   string
	apiHost  string
	tier1    int
}

// result is one CLI invocation.
type result struct {
	args     []string
	exitCode int
	stdout   []byte
	stderr   string
	tier1    int
}

func TestLive(t *testing.T) {
	rt := newRuntime(t)

	t.Run("me", func(t *testing.T) {
		res := rt.run(t, "me")
		env := rt.ok(t, res, "me")
		var me struct {
			ID     string `json:"id"`
			Handle string `json:"handle"`
		}
		rt.decode(t, env.Data, &me)
		if me.Handle == "" {
			t.Fatalf("me reported no handle: %s", res.stdout)
		}
		t.Logf("authenticated as %s", me.Handle)
	})

	t.Run("file info", func(t *testing.T) {
		res := rt.run(t, "file", "info", rt.fileKey)
		env := rt.ok(t, res, "file.info")
		var info struct {
			Version string `json:"version"`
			Pages   []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"pages"`
		}
		rt.decode(t, env.Data, &info)
		if len(info.Pages) == 0 {
			t.Fatalf("file info returned no pages.%s", rt.targetHint())
		}
		if info.Version == "" {
			t.Fatalf("file info returned no version.%s", rt.targetHint())
		}
		if env.File == nil || env.File.Key != rt.fileKey {
			t.Fatalf("envelope file block is %+v, want key %s", env.File, rt.fileKey)
		}
		t.Logf("file %q version %s with %d page(s)", env.File.Name, info.Version, len(info.Pages))
	})

	t.Run("file tree", func(t *testing.T) {
		res := rt.run(t, "file", "tree", rt.fileKey)
		env := rt.ok(t, res, "file.tree")
		var rows []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
		}
		rt.decode(t, env.Data, &rows)
		if len(rows) == 0 {
			t.Fatalf("file tree returned no nodes.%s", rt.targetHint())
		}
	})

	t.Run("file tree is cached", func(t *testing.T) {
		res := rt.run(t, "file", "tree", rt.fileKey)
		rt.ok(t, res, "file.tree")
		if res.tier1 != 0 {
			t.Fatalf("the second file tree call made %d Tier 1 request(s), want 0; the cache directory %s was not reused:\n%s",
				res.tier1, rt.cacheDir, res.stderr)
		}
	})

	t.Run("file find", func(t *testing.T) {
		res := rt.run(t, "file", "find", rt.fileKey, "--type", "COMPONENT,COMPONENT_SET")
		env := rt.ok(t, res, "file.find")
		var hits []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		rt.decode(t, env.Data, &hits)
		if len(hits) == 0 {
			t.Fatalf("file find --type COMPONENT,COMPONENT_SET returned no hits.%s", rt.targetHint())
		}
	})

	t.Run("node inspect", func(t *testing.T) {
		res := rt.run(t, "node", "inspect", rt.fileKey, "--node", rt.nodeID)
		env := rt.ok(t, res, "node.inspect")
		var nodes []inspectNode
		rt.decode(t, env.Data, &nodes)
		if len(nodes) == 0 {
			t.Fatalf("node inspect returned no nodes.%s", rt.targetHint())
		}
		root := nodes[0]
		if root.Layout == nil {
			t.Fatalf("node %s has no resolved layout.%s", rt.nodeID, rt.targetHint())
		}
		if root.Box == nil || root.Box.W <= 0 || root.Box.H <= 0 {
			t.Fatalf("node %s has no resolved box: %+v.%s", rt.nodeID, root.Box, rt.targetHint())
		}
		fills, texts := countStyled(&root)
		if fills == 0 && texts == 0 {
			t.Fatalf("node %s resolved neither a fill nor a text style.%s", rt.nodeID, rt.targetHint())
		}
		t.Logf("node %s resolved %d node(s) with fills and %d with text", rt.nodeID, fills, texts)
	})

	t.Run("node context", func(t *testing.T) {
		res := rt.run(t, "node", "context", rt.fileKey, "--node", rt.nodeID)
		// Screenshot or asset failures exit 6 (PARTIAL) with the rest of
		// the bundle intact, which is a pass as long as the screenshot is
		// on disk.
		if res.exitCode != figctl.ExitOK && res.exitCode != figctl.ExitPartial {
			t.Fatalf("node context exited %d, want 0 or 6:\n%s\n%s", res.exitCode, res.stdout, res.stderr)
		}
		env := rt.envelopeOf(t, res, "node.context")
		var data struct {
			OutDir string `json:"outDir"`
			Nodes  []struct {
				ID         string `json:"id"`
				Screenshot *struct {
					Path   string `json:"path"`
					Format string `json:"format"`
					Bytes  int64  `json:"bytes"`
				} `json:"screenshot"`
			} `json:"nodes"`
			Summary struct {
				Nodes      *int `json:"nodes"`
				Tokens     *int `json:"tokens"`
				Styles     *int `json:"styles"`
				Components *int `json:"components"`
			} `json:"summary"`
			Failures []struct {
				Kind  string `json:"kind"`
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"failures"`
		}
		rt.decode(t, env.Data, &data)
		for _, f := range data.Failures {
			t.Logf("node context reported a %s failure on %s: %s", f.Kind, f.ID, f.Error)
		}
		if len(data.Nodes) == 0 {
			t.Fatalf("node context returned no nodes.%s", rt.targetHint())
		}
		shot := data.Nodes[0].Screenshot
		if shot == nil {
			t.Fatalf("node context wrote no screenshot for %s.%s", rt.nodeID, rt.targetHint())
		}
		if shot.Format != "png" {
			t.Fatalf("screenshot format is %q, want png", shot.Format)
		}
		if !filepath.IsAbs(shot.Path) {
			t.Fatalf("screenshot path %q is not absolute; an agent cannot read it", shot.Path)
		}
		checkPNG(t, shot.Path)
		if data.Summary.Tokens == nil || data.Summary.Components == nil || data.Summary.Nodes == nil {
			t.Fatalf("node context summary is missing counts: %s", env.Data)
		}
		if *data.Summary.Nodes == 0 {
			t.Fatalf("node context summarized 0 nodes.%s", rt.targetHint())
		}
		t.Logf("context: %d nodes, %d tokens, %d styles, %d components",
			*data.Summary.Nodes, *data.Summary.Tokens, *data.Summary.Styles, *data.Summary.Components)
	})

	t.Run("render", func(t *testing.T) {
		out := filepath.Join(rt.workdir, "renders")
		res := rt.run(t, "render", rt.fileKey, "--node", rt.nodeID, "--format", "svg", "--out", out)
		env := rt.ok(t, res, "render")
		var data struct {
			Items []struct {
				Path   string `json:"path"`
				Format string `json:"format"`
				Bytes  int64  `json:"bytes"`
			} `json:"items"`
			Failures []struct {
				NodeID string `json:"nodeId"`
				Error  string `json:"error"`
			} `json:"failures"`
		}
		rt.decode(t, env.Data, &data)
		if len(data.Failures) > 0 {
			t.Fatalf("render reported failures: %+v.%s", data.Failures, rt.targetHint())
		}
		if len(data.Items) == 0 {
			t.Fatalf("render wrote no files.%s", rt.targetHint())
		}
		item := data.Items[0]
		if item.Format != "svg" {
			t.Fatalf("render wrote format %q, want svg", item.Format)
		}
		body, err := os.ReadFile(item.Path)
		if err != nil {
			t.Fatalf("reading the rendered file: %v", err)
		}
		if !bytes.Contains(body, []byte("<svg")) {
			t.Fatalf("%s is not an SVG document (first bytes: %q)", item.Path, head(body))
		}
	})

	t.Run("assets export dry run", func(t *testing.T) {
		out := filepath.Join(rt.workdir, "assets-dry-run")
		res := rt.run(t, "assets", "export", rt.fileKey, "--node", rt.nodeID, "--dry-run", "--out", out)
		env := rt.ok(t, res, "assets.export")
		var data struct {
			OutDir   string `json:"outDir"`
			DryRun   bool   `json:"dryRun"`
			Requests int    `json:"requests"`
			Icons    []struct {
				NodeID string `json:"nodeId"`
				Path   string `json:"path"`
			} `json:"icons"`
			Exports []struct {
				NodeID string `json:"nodeId"`
				Path   string `json:"path"`
			} `json:"exports"`
			ImageFills []struct {
				ImageRef string `json:"imageRef"`
			} `json:"imageFills"`
		}
		rt.decode(t, env.Data, &data)
		if !data.DryRun {
			t.Fatal("assets export --dry-run did not report dryRun")
		}
		if data.Requests != 0 {
			t.Fatalf("assets export --dry-run made %d image request(s), want 0", data.Requests)
		}
		candidates := len(data.Icons) + len(data.Exports) + len(data.ImageFills)
		if candidates == 0 {
			t.Fatalf("assets export --dry-run listed no candidates under %s.%s", rt.nodeID, rt.targetHint())
		}
		if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("assets export --dry-run created %s; a dry run must not write", out)
		}
		t.Logf("dry run listed %d candidate(s)", candidates)
	})

	t.Run("tokens export css", func(t *testing.T) {
		res := rt.run(t, "tokens", "export", rt.fileKey, "--format", "css")
		env := rt.ok(t, res, "tokens.export")
		var data struct {
			Format  string `json:"format"`
			Content string `json:"content"`
			Summary struct {
				Tokens int `json:"tokens"`
			} `json:"summary"`
		}
		rt.decode(t, env.Data, &data)
		if data.Format != "css" {
			t.Fatalf("tokens export reported format %q, want css", data.Format)
		}
		if !strings.Contains(data.Content, "--") {
			t.Fatalf("tokens export --format css produced no custom properties: %q", head([]byte(data.Content)))
		}
		// A token without file_variables:read, or a plan without
		// variables, must still succeed with styles only and say so.
		if degraded := variablesHint(env.Hints); degraded != "" {
			t.Logf("variables were not available, styles only: %s", degraded)
		}
		t.Logf("exported %d token(s)", data.Summary.Tokens)
	})

	t.Run("styles list", func(t *testing.T) {
		res := rt.run(t, "styles", "list", rt.fileKey)
		env := rt.ok(t, res, "styles.list")
		var styles []struct {
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		}
		rt.decode(t, env.Data, &styles)
		if len(styles) == 0 {
			t.Fatalf("styles list returned nothing.%s", rt.targetHint())
		}
		resolved := 0
		for _, s := range styles {
			if len(s.Value) > 0 && !bytes.Equal(s.Value, []byte("null")) {
				resolved++
			}
		}
		if resolved == 0 {
			t.Fatalf("styles list resolved no style values out of %d style(s).%s", len(styles), rt.targetHint())
		}
		t.Logf("styles list resolved %d of %d style(s)", resolved, len(styles))
	})

	t.Run("not found", func(t *testing.T) {
		// A well formed key that cannot exist: the envelope must carry
		// NOT_FOUND and the process must exit 4.
		res := rt.run(t, "file", "info", "figctlIntegrationMissingKey")
		if res.exitCode != figctl.ExitNotFound {
			t.Fatalf("a missing file exited %d, want %d:\n%s\n%s", res.exitCode, figctl.ExitNotFound, res.stdout, res.stderr)
		}
		var errEnv errorEnvelope
		if err := json.Unmarshal(res.stdout, &errEnv); err != nil {
			t.Fatalf("decoding the error envelope: %v\n%s", err, res.stdout)
		}
		if errEnv.SchemaVersion != output.SchemaVersion {
			t.Fatalf("error envelope schemaVersion is %d, want %d", errEnv.SchemaVersion, output.SchemaVersion)
		}
		if errEnv.Error.Code != figctl.CodeNotFound {
			t.Fatalf("error code is %q, want %q", errEnv.Error.Code, figctl.CodeNotFound)
		}
	})

	t.Run("cache and rate limit budget", func(t *testing.T) {
		res := rt.run(t, "cache", "status")
		env := rt.ok(t, res, "cache.status")
		var status struct {
			Root     string `json:"root"`
			Profiles []struct {
				Name  string `json:"name"`
				Files []struct {
					Key     string `json:"key"`
					Version string `json:"version"`
					Entries []struct {
						Key string `json:"key"`
					} `json:"entries"`
				} `json:"files"`
			} `json:"profiles"`
		}
		rt.decode(t, env.Data, &status)
		entries := 0
		for _, p := range status.Profiles {
			for _, f := range p.Files {
				if f.Key == rt.fileKey {
					entries += len(f.Entries)
				}
			}
		}
		if entries == 0 {
			t.Fatalf("the cache holds no entry for %s; the run did not share one cache directory (%s)", rt.fileKey, rt.cacheDir)
		}
		t.Logf("cache holds %d entr(ies) for %s", entries, rt.fileKey)

		if rt.tier1 > tier1Budget {
			t.Fatalf("the run made %d Tier 1 requests, above the budget of %d; Tier 1 allows as few as 10 per minute", rt.tier1, tier1Budget)
		}
		t.Logf("Tier 1 requests: %d of a budget of %d", rt.tier1, tier1Budget)
	})
}

// inspectNode is the part of the inspected model this test asserts on.
type inspectNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Box  *struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	} `json:"box"`
	Layout *struct {
		Mode string `json:"mode"`
	} `json:"layout"`
	Visual *struct {
		Fills []json.RawMessage `json:"fills"`
	} `json:"visual"`
	Text     json.RawMessage `json:"text"`
	Children []inspectNode   `json:"children"`
}

// countStyled counts the nodes of a subtree that resolved at least one
// fill and the ones that resolved a text style.
func countStyled(n *inspectNode) (fills, texts int) {
	if n.Visual != nil && len(n.Visual.Fills) > 0 {
		fills++
	}
	if len(n.Text) > 0 && !bytes.Equal(n.Text, []byte("null")) {
		texts++
	}
	for i := range n.Children {
		f, tx := countStyled(&n.Children[i])
		fills += f
		texts += tx
	}
	return fills, texts
}

// newRuntime builds the isolated environment the whole run shares: one
// working directory, one cache directory, and the binary under test.
func newRuntime(t *testing.T) *runtime {
	t.Helper()
	if os.Getenv(enableEnv) != "1" {
		t.Skipf("live integration test skipped: set %s=1 and %s to run it against the real Figma API (see CONTRIBUTING.md)", enableEnv, tokenEnv)
	}
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if token == "" {
		t.Skipf("live integration test skipped: %s=1 is set but %s is empty; a Figma personal access token is required", enableEnv, tokenEnv)
	}

	bin := os.Getenv(binEnv)
	if bin == "" {
		root, err := repoRoot()
		if err != nil {
			t.Fatalf("locating the repository root: %v", err)
		}
		bin = filepath.Join(root, "bin", "figctl")
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		t.Fatalf("resolving %s: %v", bin, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("the binary under test is missing at %s: run make build first, or set %s (%v)", abs, binEnv, err)
	}

	work := t.TempDir()
	cacheDir := filepath.Join(work, "cache")
	rt := &runtime{
		bin:      abs,
		workdir:  work,
		cacheDir: cacheDir,
		fileKey:  envOr(fileEnv, defaultFileKey),
		nodeID:   envOr(nodeEnv, defaultNodeID),
		apiHost:  apiHost(),
	}
	// A minimal environment: no inherited profile, no keychain, and one
	// cache directory for the whole run so the file is fetched once.
	rt.env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + work,
		"XDG_CACHE_HOME=" + cacheDir,
		"XDG_CONFIG_HOME=" + filepath.Join(work, "config"),
		"FIGCTL_CREDENTIAL_STORE=file",
		tokenEnv + "=" + token,
		"NO_COLOR=1",
	}
	if base := os.Getenv(baseURLEnv); base != "" {
		rt.env = append(rt.env, baseURLEnv+"="+base)
	}
	t.Logf("binary %s, file %s, node %s, cache %s", rt.bin, rt.fileKey, rt.nodeID, rt.cacheDir)
	return rt
}

// run executes one command and accounts for the Tier 1 requests it made.
// Every call is JSON and verbose, and no call passes --refresh or
// --no-cache: the cache is what keeps the run inside the rate limit.
func (rt *runtime) run(t *testing.T, args ...string) result {
	t.Helper()
	full := append([]string{"--json", "--verbose", "--timeout", "120s"}, args...)
	cmd := exec.Command(rt.bin, full...)
	cmd.Dir = rt.workdir
	cmd.Env = rt.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatalf("running %s %s: %v", rt.bin, strings.Join(full, " "), err)
	}

	res := result{args: args, exitCode: code, stdout: stdout.Bytes(), stderr: stderr.String()}
	res.tier1 = rt.countTier1(res.stderr)
	rt.tier1 += res.tier1
	t.Logf("figctl %s -> exit %d, %d Tier 1 request(s), %d total", strings.Join(args, " "), code, res.tier1, rt.tier1)
	return res
}

// ok asserts the command succeeded and returns its envelope.
func (rt *runtime) ok(t *testing.T, res result, command string) envelope {
	t.Helper()
	if res.exitCode != figctl.ExitOK {
		t.Fatalf("figctl %s exited %d, want 0:\n%s\n%s", strings.Join(res.args, " "), res.exitCode, res.stdout, res.stderr)
	}
	return rt.envelopeOf(t, res, command)
}

// envelopeOf decodes stdout as the success envelope and asserts the parts
// of the output contract every command shares.
func (rt *runtime) envelopeOf(t *testing.T, res result, command string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(res.stdout, &env); err != nil {
		t.Fatalf("figctl %s did not print a JSON envelope: %v\n%s", strings.Join(res.args, " "), err, head(res.stdout))
	}
	if env.SchemaVersion != output.SchemaVersion {
		t.Fatalf("schemaVersion is %d, want %d", env.SchemaVersion, output.SchemaVersion)
	}
	if env.Command != command {
		t.Fatalf("command is %q, want %q", env.Command, command)
	}
	if env.Profile == nil || env.Profile.Name == "" {
		t.Fatalf("envelope carries no profile: %s", head(res.stdout))
	}
	if env.Hints == nil {
		t.Fatalf("envelope carries no hints array: %s", head(res.stdout))
	}
	for _, h := range env.Hints {
		t.Logf("hint: %s", h)
	}
	return env
}

// decode unmarshals the data block of an envelope.
func (rt *runtime) decode(t *testing.T, data json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("decoding the data block: %v\n%s", err, head(data))
	}
}

// targetHint names the environment variables that select the target, so a
// failure caused by a changed public file is obvious.
func (rt *runtime) targetHint() string {
	return fmt.Sprintf(" The target is file %s node %s; override it with %s and %s.",
		rt.fileKey, rt.nodeID, fileEnv, nodeEnv)
}

// countTier1 counts the Tier 1 requests in a verbose trace: GET file, GET
// nodes, and the image renders. Tier 2 and Tier 3 endpoints, and the
// downloads from the CDN, are not counted because they are not the scarce
// resource.
func (rt *runtime) countTier1(stderr string) int {
	n := 0
	for _, line := range strings.Split(stderr, "\n") {
		m := traceLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		if rt.isTier1(m[3]) {
			n++
		}
	}
	return n
}

// isTier1 reports whether a traced URL is one of the Tier 1 endpoints.
func (rt *runtime) isTier1(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host != rt.apiHost {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" {
		return false
	}
	switch parts[1] {
	case "images":
		return true
	case "files":
		return len(parts) == 3 || (len(parts) == 4 && parts[3] == "nodes")
	}
	return false
}

// checkPNG asserts a file is a PNG with non-zero dimensions.
func checkPNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the screenshot: %v", err)
	}
	defer func() { _ = f.Close() }()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("%s is not a valid PNG: %v", path, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Fatalf("%s is %dx%d", path, cfg.Width, cfg.Height)
	}
	t.Logf("screenshot %s is %dx%d", path, cfg.Width, cfg.Height)
}

// variablesHint returns the hint that explains a styles-only export, if
// the run degraded to one.
func variablesHint(hints []string) string {
	for _, h := range hints {
		lower := strings.ToLower(h)
		if strings.Contains(lower, "variable") && (strings.Contains(lower, "scope") || strings.Contains(lower, "plan") || strings.Contains(lower, "enterprise")) {
			return h
		}
	}
	return ""
}

// repoRoot walks up from the test's directory to the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found above " + dir)
		}
		dir = parent
	}
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// apiHost is the host the request accounting counts against.
func apiHost() string {
	base := strings.TrimSpace(os.Getenv(baseURLEnv))
	if base == "" {
		return defaultAPIHost
	}
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return defaultAPIHost
}

// head trims long output so a failure stays readable.
func head(b []byte) string {
	const max = 2000
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "... (truncated)"
}
