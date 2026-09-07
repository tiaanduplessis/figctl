package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Environment variables that take part in profile selection.
const (
	ProfileEnv = "FIGCTL_PROFILE"
	TokenEnv   = "FIGMA_TOKEN" //nolint:gosec // environment variable name, not a credential
)

// EnvProfileName is the implicit profile created by FIGMA_TOKEN.
const EnvProfileName = "env"

// TokenFileProfileName is the implicit profile used when only --token-file
// is given and no profile could be selected.
const TokenFileProfileName = "file"

// Source says how the active profile was chosen.
type Source string

// Profile sources, in selection order.
const (
	SourceFlag      Source = "flag"
	SourceEnv       Source = "env"
	SourceTokenEnv  Source = "token-env"
	SourceProject   Source = "project"
	SourceDefault   Source = "default"
	SourceTokenFile Source = "token-file"
)

// Options controls profile resolution.
type Options struct {
	// ProfileFlag is the --profile flag value.
	ProfileFlag string
	// TokenFile is the --token-file flag value.
	TokenFile string
	// Cwd is where the project config walk-up starts.
	Cwd string
	// Store provides tokens. Required by Resolve, ignored by Select.
	Store Store
}

// Active is the selected profile.
type Active struct {
	Name    string
	Profile Profile
	Source  Source
	// Token is empty when resolved with Select.
	Token string
	// TokenSource names where the token came from: token-file, env, or
	// the store kind.
	TokenSource string
	// Project is the project config found by walking up from Cwd, if any,
	// regardless of whether it selected the profile.
	Project *Project
}

// ErrNoProfile builds the AUTH_MISSING error shown when nothing selects a
// profile.
func ErrNoProfile() *figctl.Error {
	return figctl.New(figctl.CodeAuthMissing, "no Figma profile configured").
		WithHint("Run figctl auth login --profile <name>, or set FIGMA_TOKEN.")
}

func errUnknownProfile(name string, source Source) *figctl.Error {
	return figctl.Newf(figctl.CodeAuthMissing, "profile %q (from %s) does not exist", name, source).
		WithHint("Run figctl auth login --profile %s, or figctl profile list to see configured profiles.", name)
}

// Select chooses the active profile without loading its token, following
// the order: --profile, FIGCTL_PROFILE, FIGMA_TOKEN, project config, default.
func Select(cfg *Config, opts Options) (*Active, error) {
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	project, err := FindProject(cwd)
	if err != nil {
		return nil, err
	}
	tokenEnv := strings.TrimSpace(os.Getenv(TokenEnv))

	lookup := func(name string, source Source) (*Active, error) {
		if p, ok := cfg.Profiles[name]; ok {
			return &Active{Name: name, Profile: p, Source: source, Project: project}, nil
		}
		if name == EnvProfileName && tokenEnv != "" {
			return &Active{Name: name, Source: source, Project: project}, nil
		}
		return nil, errUnknownProfile(name, source)
	}

	if opts.ProfileFlag != "" {
		return lookup(opts.ProfileFlag, SourceFlag)
	}
	if name := strings.TrimSpace(os.Getenv(ProfileEnv)); name != "" {
		return lookup(name, SourceEnv)
	}
	if tokenEnv != "" {
		return &Active{Name: EnvProfileName, Source: SourceTokenEnv, Project: project}, nil
	}
	if project != nil && project.Profile != "" {
		return lookup(project.Profile, SourceProject)
	}
	if cfg.DefaultProfile != "" {
		return lookup(cfg.DefaultProfile, SourceDefault)
	}
	return nil, ErrNoProfile()
}

// Resolve selects the active profile and loads its token. Token sources in
// order: --token-file, FIGMA_TOKEN (for the env profile), the credential
// store.
func Resolve(cfg *Config, opts Options) (*Active, error) {
	active, err := Select(cfg, opts)
	if err != nil {
		if opts.TokenFile == "" {
			return nil, err
		}
		active = &Active{Name: TokenFileProfileName, Source: SourceTokenFile}
	}
	if opts.TokenFile != "" {
		token, err := ReadTokenFile(opts.TokenFile)
		if err != nil {
			return nil, err
		}
		active.Token = token
		active.TokenSource = string(SourceTokenFile)
		return active, nil
	}
	if active.Source == SourceTokenEnv || (active.Name == EnvProfileName && os.Getenv(TokenEnv) != "") {
		active.Token = strings.TrimSpace(os.Getenv(TokenEnv))
		active.TokenSource = TokenEnv
		return active, nil
	}
	if opts.Store == nil {
		return nil, figctl.New(figctl.CodeInternal, "no credential store configured")
	}
	token, err := opts.Store.Get(active.Name)
	if errors.Is(err, ErrNotFound) {
		return nil, figctl.Newf(figctl.CodeAuthMissing, "profile %q has no stored token", active.Name).
			WithHint("Run figctl auth login --profile %s to store one.", active.Name)
	}
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "reading credentials: "+err.Error())
	}
	active.Token = token
	active.TokenSource = opts.Store.Kind()
	return active, nil
}

// ReadTokenFile reads and trims a token from a file.
func ReadTokenFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", figctl.Wrap(figctl.CodeAuthMissing, err, fmt.Sprintf("reading token file %s: %v", path, err)).
			WithHint("Pass a readable file containing only the token.")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", figctl.Newf(figctl.CodeAuthMissing, "token file %s is empty", path).
			WithHint("Write the Figma personal access token into the file.")
	}
	return token, nil
}
