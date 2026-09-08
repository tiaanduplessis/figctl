package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// setupEnv isolates the config directory, home, and environment for a test.
func setupEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv(StoreEnv, "file")
	t.Setenv(ProfileEnv, "")
	t.Setenv(TokenEnv, "")
	return root
}

func writeConfig(t *testing.T, cfg *Config) {
	t.Helper()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestDirFallsBackToHome(t *testing.T) {
	root := setupEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "home", ".config", "figctl")
	if dir != want {
		t.Fatalf("Dir = %s, want %s", dir, want)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	setupEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 0 || cfg.DefaultProfile != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
	cfg.DefaultProfile = "acme"
	cfg.Profiles["acme"] = Profile{TeamID: "123", BaseURL: "https://api.figma.com", Expires: "2026-12-01"}
	writeConfig(t, cfg)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config perm = %o, want 600", info.Mode().Perm())
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "defaultProfile: acme\nprofiles:\n    acme:\n        teamId: \"123\"\n        baseUrl: https://api.figma.com\n        expires: \"2026-12-01\"\n" {
		t.Fatalf("unexpected config file:\n%s", raw)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultProfile != "acme" || loaded.Profiles["acme"].TeamID != "123" {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	if got := loaded.Names(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("Names = %v", got)
	}
}

func TestValidateNameAndExpires(t *testing.T) {
	for _, ok := range []string{"acme", "client-a", "a.b_c", "x1"} {
		if err := ValidateName(ok); err != nil {
			t.Fatalf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-x", "a b", "a/b"} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("%q should be invalid", bad)
		}
	}
	if err := ValidateExpires("2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExpires(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExpires("01/12/2026"); err == nil {
		t.Fatal("expected error for bad date")
	}
}

func TestFindProjectWalksUp(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Project{Profile: "acme", Files: map[string]string{"web": "AbC123def456GHI789jkl0"}, Default: "web"}
	path, err := p.Save(repo)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(repo, ProjectFileName) {
		t.Fatalf("unexpected path %s", path)
	}

	found, err := FindProject(nested)
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.Profile != "acme" || found.Files["web"] != p.Files["web"] || found.Path != path {
		t.Fatalf("unexpected project: %+v", found)
	}

	outside := filepath.Join(root, "other")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err = FindProject(outside)
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Fatalf("expected no project outside the repo, got %+v", found)
	}

	if err := os.WriteFile(filepath.Join(outside, ProjectFileName), []byte("profile: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FindProject(outside); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials")
	store := NewFileStore(path)
	if _, err := store.Get("acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := store.Set("acme", "figd_secret"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("other", "figd_other"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credentials perm = %o, want 600", info.Mode().Perm())
		}
	}
	token, err := store.Get("acme")
	if err != nil || token != "figd_secret" {
		t.Fatalf("Get = %q, %v", token, err)
	}
	if err := store.Delete("acme"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if token, err := store.Get("other"); err != nil || token != "figd_other" {
		t.Fatalf("other token lost: %q, %v", token, err)
	}
	if store.Kind() != "file" {
		t.Fatalf("Kind = %s", store.Kind())
	}
}

type failingStore struct{}

func (failingStore) Get(string) (string, error) { return "", errors.New("no keychain") }
func (failingStore) Set(string, string) error   { return errors.New("no keychain") }
func (failingStore) Delete(string) error        { return errors.New("no keychain") }
func (failingStore) Kind() string               { return "keyring" }

func TestFallbackStore(t *testing.T) {
	file := NewFileStore(filepath.Join(t.TempDir(), "credentials"))
	warnings := 0
	store := &fallbackStore{primary: failingStore{}, fallback: file, warn: func(string, ...any) { warnings++ }}
	if err := store.Set("acme", "tok"); err != nil {
		t.Fatal(err)
	}
	token, err := store.Get("acme")
	if err != nil || token != "tok" {
		t.Fatalf("Get = %q, %v", token, err)
	}
	if err := store.Delete("acme"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("acme"); err == nil {
		t.Fatal("expected error deleting missing entry")
	}
	if warnings != 1 {
		t.Fatalf("expected one warning, got %d", warnings)
	}
	if store.Kind() != "file" {
		t.Fatalf("Kind after fallback = %s", store.Kind())
	}
}

func TestOpenStoreHonorsEnv(t *testing.T) {
	setupEnv(t)
	store, err := OpenStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("expected file store, got %T", store)
	}
	t.Setenv(StoreEnv, "bogus")
	if _, err := OpenStore(nil); err == nil {
		t.Fatal("expected error for bogus store")
	}
}

func TestSelectOrder(t *testing.T) {
	root := setupEnv(t)
	cfg := &Config{
		DefaultProfile: "dflt",
		Profiles: map[string]Profile{
			"dflt":  {TeamID: "1"},
			"flag":  {TeamID: "2"},
			"envp":  {TeamID: "3"},
			"proj":  {TeamID: "4"},
			"extra": {},
		},
	}
	writeConfig(t, cfg)
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Project{Profile: "proj"}).Save(repo); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		flag       string
		envProfile string
		envToken   string
		cwd        string
		wantName   string
		wantSource Source
		wantCode   figctl.Code
	}{
		{"flag wins", "flag", "envp", "tok", nested, "flag", SourceFlag, ""},
		{"env profile beats token", "", "envp", "tok", nested, "envp", SourceEnv, ""},
		{"token env beats project", "", "", "tok", nested, EnvProfileName, SourceTokenEnv, ""},
		{"project beats default", "", "", "", nested, "proj", SourceProject, ""},
		{"default", "", "", "", outside, "dflt", SourceDefault, ""},
		{"flag unknown", "missing", "", "", outside, "", "", figctl.CodeAuthMissing},
		{"env unknown", "", "missing", "", outside, "", "", figctl.CodeAuthMissing},
		{"flag env with token", "env", "", "tok", outside, "env", SourceFlag, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ProfileEnv, tc.envProfile)
			t.Setenv(TokenEnv, tc.envToken)
			active, err := Select(cfg, Options{ProfileFlag: tc.flag, Cwd: tc.cwd})
			if tc.wantCode != "" {
				if err == nil {
					t.Fatalf("expected error, got %+v", active)
				}
				if figctl.From(err).Code != tc.wantCode {
					t.Fatalf("code = %s, want %s", figctl.From(err).Code, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if active.Name != tc.wantName || active.Source != tc.wantSource {
				t.Fatalf("got %s/%s, want %s/%s", active.Name, active.Source, tc.wantName, tc.wantSource)
			}
			if tc.cwd == nested && (active.Project == nil || active.Project.Profile != "proj") {
				t.Fatalf("project config should be attached, got %+v", active.Project)
			}
		})
	}

	t.Run("no profile at all", func(t *testing.T) {
		_, err := Select(&Config{Profiles: map[string]Profile{}}, Options{Cwd: outside})
		e := figctl.From(err)
		if e == nil || e.Code != figctl.CodeAuthMissing {
			t.Fatalf("expected AUTH_MISSING, got %v", err)
		}
		if e.Hint == "" {
			t.Fatal("expected a hint")
		}
	})
}

func TestResolveTokens(t *testing.T) {
	root := setupEnv(t)
	cfg := &Config{DefaultProfile: "acme", Profiles: map[string]Profile{"acme": {}, "empty": {}}}
	writeConfig(t, cfg)
	store, err := OpenStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("acme", "figd_store"); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(root, "token.txt")
	if err := os.WriteFile(tokenFile, []byte("figd_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()

	active, err := Resolve(cfg, Options{Cwd: cwd, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if active.Token != "figd_store" || active.TokenSource != "file" || active.Source != SourceDefault {
		t.Fatalf("unexpected store resolution: %+v", active)
	}

	active, err = Resolve(cfg, Options{Cwd: cwd, Store: store, TokenFile: tokenFile})
	if err != nil {
		t.Fatal(err)
	}
	if active.Token != "figd_file" || active.TokenSource != "token-file" || active.Name != "acme" {
		t.Fatalf("unexpected token file resolution: %+v", active)
	}

	t.Setenv(TokenEnv, " figd_env ")
	active, err = Resolve(cfg, Options{Cwd: cwd, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if active.Token != "figd_env" || active.Name != EnvProfileName || active.TokenSource != TokenEnv {
		t.Fatalf("unexpected env resolution: %+v", active)
	}
	t.Setenv(TokenEnv, "")

	_, err = Resolve(cfg, Options{Cwd: cwd, Store: store, ProfileFlag: "empty"})
	if figctl.From(err).Code != figctl.CodeAuthMissing {
		t.Fatalf("expected AUTH_MISSING for profile without token, got %v", err)
	}

	active, err = Resolve(&Config{Profiles: map[string]Profile{}}, Options{Cwd: cwd, Store: store, TokenFile: tokenFile})
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != TokenFileProfileName || active.Source != SourceTokenFile || active.Token != "figd_file" {
		t.Fatalf("unexpected token-file-only resolution: %+v", active)
	}

	_, err = Resolve(cfg, Options{Cwd: cwd, Store: store, TokenFile: filepath.Join(root, "missing")})
	if figctl.From(err).Code != figctl.CodeAuthMissing {
		t.Fatalf("expected AUTH_MISSING for missing token file, got %v", err)
	}
}
