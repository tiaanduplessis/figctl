// Package config loads and saves the user config file, discovers the
// project config, stores credentials, and resolves the active profile.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

const (
	appDir         = "figctl"
	configFileName = "config.yaml"
	credentialFile = "credentials"
	// ExpiresLayout is the date layout accepted by --expires.
	ExpiresLayout = "2006-01-02"
	// ExpiryWarning is how close to expiry a token warning is shown.
	ExpiryWarning = 7 * 24 * time.Hour
)

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Profile is the non-secret part of a named account. Tokens live in the
// credential store.
type Profile struct {
	TeamID  string `yaml:"teamId,omitempty" json:"teamId,omitempty"`
	BaseURL string `yaml:"baseUrl,omitempty" json:"baseUrl,omitempty"`
	Handle  string `yaml:"handle,omitempty" json:"handle,omitempty"`
	Email   string `yaml:"email,omitempty" json:"email,omitempty"`
	Expires string `yaml:"expires,omitempty" json:"expires,omitempty"`
}

// ExpiresAt parses the recorded expiry date. It returns the zero time when
// no expiry was recorded.
func (p Profile) ExpiresAt() (time.Time, error) {
	if p.Expires == "" {
		return time.Time{}, nil
	}
	return time.Parse(ExpiresLayout, p.Expires)
}

// DaysUntilExpiry returns whole calendar days from the day of now until the
// recorded expiry date, and whether an expiry is recorded at all. The value is
// negative once the date has passed. Both dates are compared at midnight in
// now's location, so the result never depends on the time of day.
func (p Profile) DaysUntilExpiry(now time.Time) (int, bool) {
	if p.Expires == "" {
		return 0, false
	}
	exp, err := time.ParseInLocation(ExpiresLayout, p.Expires, now.Location())
	if err != nil {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return int(math.Round(exp.Sub(today).Hours() / 24)), true
}

// Config is the user level config file.
type Config struct {
	DefaultProfile string             `yaml:"defaultProfile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

// Dir returns the figctl config directory: $XDG_CONFIG_HOME/figctl or
// ~/.config/figctl.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appDir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "locating home directory: "+err.Error()).
			WithHint("Set XDG_CONFIG_HOME or HOME.")
	}
	return filepath.Join(home, ".config", appDir), nil
}

// Path returns the user config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load reads the user config. A missing file yields an empty config.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Profiles: map[string]Profile{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("reading config %s: %v", path, err))
	}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("parsing config %s: %v", path, err)).
			WithHint("Fix or remove the file and run figctl auth login again.")
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, nil
}

// Save writes the user config with owner-only permissions.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "creating config directory: "+err.Error())
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding config: "+err.Error())
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("writing config %s: %v", path, err))
	}
	return nil
}

// Names returns the profile names in sorted order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateName checks a profile name.
func ValidateName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return figctl.Newf(figctl.CodeUsage, "invalid profile name %q", name).
			WithHint("Use letters, digits, dots, dashes, or underscores, starting with a letter or digit.")
	}
	return nil
}

// ValidateExpires checks a --expires value.
func ValidateExpires(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(ExpiresLayout, s); err != nil {
		return figctl.Newf(figctl.CodeUsage, "invalid --expires value %q", s).
			WithHint("Use the form YYYY-MM-DD, for example 2026-12-01.")
	}
	return nil
}
