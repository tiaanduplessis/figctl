package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

type result struct {
	stdout string
	stderr string
	code   int
}

func execute(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(Options{Args: args, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut})
	return result{stdout: out.String(), stderr: errOut.String(), code: code}
}

// isolate points config, home, credentials, and cwd at temp directories.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	t.Setenv(figma.BaseURLEnv, "")
	t.Setenv(config.StoreEnv, "file")
	t.Setenv(config.ProfileEnv, "")
	t.Setenv(config.TokenEnv, "")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	return root
}

// fakeAPI starts the fake Figma server and points the CLI at it.
func fakeAPI(t *testing.T) *figmatest.Server {
	t.Helper()
	s := figmatest.NewServer(t)
	t.Setenv(figma.BaseURLEnv, s.URL)
	return s
}

func decodeEnvelope(t *testing.T, s string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
	return env
}

func data(t *testing.T, r result, command string) map[string]any {
	t.Helper()
	env := decodeEnvelope(t, r.stdout)
	if env["command"] != command {
		t.Fatalf("command = %v, want %s", env["command"], command)
	}
	if env["schemaVersion"] != float64(1) {
		t.Fatalf("schemaVersion = %v", env["schemaVersion"])
	}
	d, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want object", env["data"])
	}
	return d
}

func errorCode(t *testing.T, r result) string {
	t.Helper()
	env := decodeEnvelope(t, r.stdout)
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error block in %s", r.stdout)
	}
	code, _ := e["code"].(string)
	return code
}

func TestHelp(t *testing.T) {
	r := execute(t, "", "--help")
	if r.code != figctl.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", r.code, r.stderr)
	}
	for _, want := range []string{"file tree", "node context", "render", "tokens export", "figctl <cmd> --help", "--output", "--profile", "--token-file", "--file-version"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("help missing %q:\n%s", want, r.stdout)
		}
	}
	r = execute(t, "")
	if r.code != figctl.ExitOK || !strings.Contains(r.stdout, "Recommended workflow") {
		t.Fatalf("bare invocation should print help, exit = %d\n%s", r.code, r.stdout)
	}
}

func TestVersion(t *testing.T) {
	r := execute(t, "", "version")
	if r.code != figctl.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", r.code, r.stderr)
	}
	d := data(t, r, "version")
	if d["version"] != "dev" || d["commit"] != "none" || d["os"] == "" {
		t.Fatalf("unexpected version data: %v", d)
	}
	env := decodeEnvelope(t, r.stdout)
	if env["profile"] != nil || env["file"] != nil || env["truncated"] != false {
		t.Fatalf("unexpected envelope: %v", env)
	}

	// The behaviour of --check against a release feed is covered in
	// version_check_test.go; here it only has to not fail when the feed is
	// unreachable from the test environment.

	r = execute(t, "", "version", "-o", "table")
	if r.code != figctl.ExitOK || !strings.HasPrefix(r.stdout, "field      value\nversion    dev\n") {
		t.Fatalf("unexpected table output (exit %d):\n%s", r.code, r.stdout)
	}
	r = execute(t, "", "version", "-o", "md", "--fields", "version,os")
	if r.code != figctl.ExitOK || !strings.Contains(r.stdout, "# version") || !strings.Contains(r.stdout, "| version | dev |") || strings.Contains(r.stdout, "commit") {
		t.Fatalf("unexpected markdown output (exit %d):\n%s", r.code, r.stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	r := execute(t, "", "bogus")
	if r.code != figctl.ExitUsage {
		t.Fatalf("exit = %d, want 2", r.code)
	}
	if code := errorCode(t, r); code != "USAGE" {
		t.Fatalf("code = %s", code)
	}
	if r.stderr != "" {
		t.Fatalf("stderr should be empty in JSON mode, got %q", r.stderr)
	}

	r = execute(t, "", "bogus", "--json")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("--json: exit = %d, stdout = %s", r.code, r.stdout)
	}

	r = execute(t, "", "bogus", "-o", "table")
	if r.code != figctl.ExitUsage || r.stdout != "" {
		t.Fatalf("table mode: exit = %d, stdout = %q", r.code, r.stdout)
	}
	if !strings.Contains(r.stderr, "error: unknown command \"bogus\" for \"figctl\" (USAGE)") || !strings.Contains(r.stderr, "hint: Run figctl --help") {
		t.Fatalf("unexpected stderr: %q", r.stderr)
	}

	r = execute(t, "", "version", "--bogus-flag")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("unknown flag: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "version", "extra")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("extra arg: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "version", "-o", "xml")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("bad output mode: exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestAuthStatusWithoutProfiles(t *testing.T) {
	isolate(t)
	r := execute(t, "", "auth", "status")
	if r.code != figctl.ExitAuth {
		t.Fatalf("exit = %d, want 3; stdout = %s", r.code, r.stdout)
	}
	if errorCode(t, r) != "AUTH_MISSING" {
		t.Fatalf("code = %s", errorCode(t, r))
	}
	env := decodeEnvelope(t, r.stdout)
	hint := env["error"].(map[string]any)["hint"].(string)
	if !strings.Contains(hint, "figctl auth login --profile <name>") {
		t.Fatalf("hint = %q", hint)
	}
}

func TestProfileLifecycle(t *testing.T) {
	root := isolate(t)
	api := fakeAPI(t)
	expires := time.Now().Add(3 * 24 * time.Hour).Format(config.ExpiresLayout)

	r := execute(t, figmatest.Token+"\n", "profile", "add", "acme", "--team", "123", "--expires", expires)
	if r.code != figctl.ExitOK {
		t.Fatalf("add: exit = %d, stdout = %s stderr = %s", r.code, r.stdout, r.stderr)
	}
	d := data(t, r, "profile.add")
	if d["name"] != "acme" || d["default"] != true || d["teamId"] != "123" || d["tokenStore"] != "file" || d["handle"] != figmatest.UserHandle || d["email"] != figmatest.UserEmail {
		t.Fatalf("unexpected add data: %v", d)
	}
	if strings.Contains(r.stdout, figmatest.Token) {
		t.Fatal("token leaked into output")
	}
	if api.Count("GET", "/v1/me") != 1 {
		t.Fatalf("profile add should validate with GET me once, got %d", api.Count("GET", "/v1/me"))
	}

	env := decodeEnvelope(t, r.stdout)
	if env["profile"].(map[string]any)["name"] != "acme" {
		t.Fatalf("envelope profile = %v", env["profile"])
	}

	r = execute(t, "figd_bad\n", "profile", "add", "bad")
	if r.code != figctl.ExitAuth || errorCode(t, r) != "AUTH_INVALID" {
		t.Fatalf("bad token: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if cfg, _ := config.Load(); len(cfg.Profiles) != 1 {
		t.Fatalf("a rejected token must not create a profile: %v", cfg.Profiles)
	}

	r = execute(t, "", "profile", "add", "bad name")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("bad name: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "profile", "add", "empty")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("empty stdin: exit = %d, stdout = %s", r.code, r.stdout)
	}

	tokenFile := filepath.Join(root, "token.txt")
	if err := os.WriteFile(tokenFile, []byte(figmatest.Token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = execute(t, "", "auth", "login", "--profile", "other", "--token-file", tokenFile)
	if r.code != figctl.ExitOK {
		t.Fatalf("login: exit = %d, stdout = %s stderr = %s", r.code, r.stdout, r.stderr)
	}
	if d := data(t, r, "auth.login"); d["default"] != false {
		t.Fatalf("second profile should not become default: %v", d)
	}

	r = execute(t, "", "profile", "list")
	if r.code != figctl.ExitOK {
		t.Fatalf("list: exit = %d", r.code)
	}
	env = decodeEnvelope(t, r.stdout)
	list := env["data"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "acme" || list[1].(map[string]any)["name"] != "other" {
		t.Fatalf("unexpected list: %v", list)
	}
	hints := env["hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "expires in 3 day(s)") {
		t.Fatalf("expected expiry hint, got %v", hints)
	}
	r = execute(t, "", "profile", "list", "-o", "plain")
	if r.code != figctl.ExitOK || !strings.HasPrefix(r.stdout, "acme\t*\t"+figmatest.UserHandle+"\t"+figmatest.UserEmail+"\t123\t\t"+expires+"\nother\t") {
		t.Fatalf("unexpected plain list: %q", r.stdout)
	}

	r = execute(t, "", "auth", "status")
	if r.code != figctl.ExitOK {
		t.Fatalf("status: exit = %d, stdout = %s", r.code, r.stdout)
	}
	d = data(t, r, "auth.status")
	if d["profile"] != "acme" || d["source"] != "default" || d["tokenSource"] != "file" || d["hasToken"] != true || d["validated"] != true || d["email"] != figmatest.UserEmail {
		t.Fatalf("unexpected status: %v", d)
	}
	if strings.Contains(r.stdout, "figd_") {
		t.Fatal("token leaked into status output")
	}

	t.Setenv(config.TokenEnv, "figd_env")
	r = execute(t, "", "auth", "status")
	if d := data(t, r, "auth.status"); d["profile"] != "env" || d["source"] != "token-env" || d["validated"] != false || d["validationError"].(map[string]any)["code"] != "AUTH_INVALID" {
		t.Fatalf("FIGMA_TOKEN should win over default and fail validation: %v", d)
	}
	t.Setenv(config.TokenEnv, "")

	r = execute(t, "", "auth", "status", "--profile", "other")
	if d := data(t, r, "auth.status"); d["profile"] != "other" || d["source"] != "flag" {
		t.Fatalf("--profile should win: %v", d)
	}

	r = execute(t, "", "init", "--profile", "other", "--file", "web=https://www.figma.com/design/AbC123def456GHI789jkl0/Web-App?node-id=1-2")
	if r.code != figctl.ExitOK {
		t.Fatalf("init: exit = %d, stdout = %s", r.code, r.stdout)
	}
	d = data(t, r, "init")
	files, _ := d["files"].(map[string]any)
	if d["profile"] != "other" || files["web"] != "AbC123def456GHI789jkl0" {
		t.Fatalf("unexpected init data: %v", d)
	}
	raw, err := os.ReadFile(filepath.Join(root, "work", config.ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "profile: other\nfiles:\n    web: AbC123def456GHI789jkl0\n" {
		t.Fatalf("unexpected project file:\n%s", raw)
	}
	r = execute(t, "", "init", "--profile", "other")
	if r.code != figctl.ExitUsage {
		t.Fatalf("init should refuse to overwrite without --yes, exit = %d", r.code)
	}

	nested := filepath.Join(root, "work", "src", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	r = execute(t, "", "auth", "status")
	d = data(t, r, "auth.status")
	if d["profile"] != "other" || d["source"] != "project" || !strings.HasSuffix(d["projectConfig"].(string), config.ProjectFileName) {
		t.Fatalf("project config should select the profile: %v", d)
	}
	r = execute(t, "", "profile", "show")
	if d := data(t, r, "profile.show"); d["name"] != "other" || d["source"] != "project" {
		t.Fatalf("profile show should follow selection: %v", d)
	}
	t.Chdir(filepath.Join(root, "work"))

	r = execute(t, "", "profile", "use", "other")
	if r.code != figctl.ExitOK {
		t.Fatalf("use: exit = %d", r.code)
	}
	r = execute(t, "", "profile", "show", "acme")
	if d := data(t, r, "profile.show"); d["default"] != false || d["expires"] != expires {
		t.Fatalf("unexpected show: %v", d)
	}
	r = execute(t, "", "profile", "show", "missing")
	if r.code != figctl.ExitNotFound || errorCode(t, r) != "NOT_FOUND" {
		t.Fatalf("missing profile: exit = %d, stdout = %s", r.code, r.stdout)
	}

	r = execute(t, "", "auth", "logout", "acme")
	if r.code != figctl.ExitOK {
		t.Fatalf("logout: exit = %d", r.code)
	}
	r = execute(t, "", "auth", "status", "--profile", "acme")
	if r.code != figctl.ExitAuth || errorCode(t, r) != "AUTH_MISSING" {
		t.Fatalf("status after logout: exit = %d, stdout = %s", r.code, r.stdout)
	}

	r = execute(t, "", "profile", "remove", "other")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("remove without --yes: exit = %d, stdout = %s", r.code, r.stdout)
	}
	r = execute(t, "", "profile", "remove", "other", "--yes")
	if r.code != figctl.ExitOK {
		t.Fatalf("remove: exit = %d, stdout = %s", r.code, r.stdout)
	}
	if d := data(t, r, "profile.remove"); d["removed"] != true {
		t.Fatalf("unexpected remove data: %v", d)
	}
	r = execute(t, "", "auth", "status", "--quiet")
	if r.code != figctl.ExitAuth {
		t.Fatalf("status after remove: exit = %d, stdout = %s", r.code, r.stdout)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProfile != "" || len(cfg.Profiles) != 1 {
		t.Fatalf("unexpected config after remove: %+v", cfg)
	}
}

func TestAuthScopes(t *testing.T) {
	r := execute(t, "", "auth", "scopes")
	if r.code != figctl.ExitOK {
		t.Fatalf("exit = %d", r.code)
	}
	env := decodeEnvelope(t, r.stdout)
	list := env["data"].([]any)
	if len(list) != len(scopes) || list[0].(map[string]any)["scope"] != "file_content:read" {
		t.Fatalf("unexpected scopes: %v", list)
	}
	r = execute(t, "", "auth", "scopes", "-o", "md")
	if !strings.Contains(r.stdout, "| scope | required | usedBy |") {
		t.Fatalf("unexpected markdown: %s", r.stdout)
	}
}
