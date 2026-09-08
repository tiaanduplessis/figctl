package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// ProjectFileName is the project config file committed in a repository.
const ProjectFileName = ".figctl.yaml"

// Project is the project level config. It never contains secrets.
type Project struct {
	// Profile names the profile to use in this repository.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// Files maps a short name to a Figma file key. A design system usually
	// lives in its own file, so one repository routinely refers to several,
	// and naming them keeps keys out of every command.
	Files map[string]string `yaml:"files,omitempty" json:"files,omitempty"`
	// Default names the entry of Files used when a command is given no ref.
	Default string `yaml:"default,omitempty" json:"default,omitempty"`
	// Path is where the file was found. It is not serialized.
	Path string `yaml:"-" json:"path,omitempty"`
}

// Lookup resolves a configured name to its file key.
func (p *Project) Lookup(name string) (string, bool) {
	if p == nil {
		return "", false
	}
	key, ok := p.Files[name]
	return key, ok
}

// DefaultKey returns the file key a command should use when given no ref.
// A single configured file is the default whether or not it was named as
// one, because there is nothing else it could mean.
func (p *Project) DefaultKey() (string, bool) {
	if p == nil || len(p.Files) == 0 {
		return "", false
	}
	if p.Default != "" {
		key, ok := p.Files[p.Default]
		return key, ok
	}
	if len(p.Files) == 1 {
		for _, key := range p.Files {
			return key, true
		}
	}
	return "", false
}

// Names lists the configured file names in a stable order, for error
// messages that have to say what the alternatives are.
func (p *Project) Names() []string {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.Files))
	for name := range p.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Validate rejects a config that names a default which does not exist, so
// the mistake surfaces where it was made rather than at the first command
// that relies on it.
func (p *Project) Validate() error {
	if p == nil || p.Default == "" {
		return nil
	}
	if _, ok := p.Files[p.Default]; !ok {
		return figctl.Newf(figctl.CodeUsage, "%s names %q as the default file, but no such file is configured", p.Path, p.Default).
			WithHint("Configured files: %s.", strings.Join(p.Names(), ", "))
	}
	return nil
}

// FindProject walks up from start looking for ProjectFileName. It returns
// nil when no project config exists.
func FindProject(start string) (*Project, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "resolving working directory: "+err.Error())
	}
	for {
		path := filepath.Join(dir, ProjectFileName)
		raw, err := os.ReadFile(path)
		if err == nil {
			p := &Project{Path: path}
			if err := yaml.Unmarshal(raw, p); err != nil {
				return nil, figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("parsing %s: %v", path, err)).
					WithHint("Fix the file or regenerate it with figctl init.")
			}
			p.Path = path
			return p, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("reading %s: %v", path, err))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// Save writes the project config into dir.
func (p *Project) Save(dir string) (string, error) {
	path := filepath.Join(dir, ProjectFileName)
	raw, err := yaml.Marshal(p)
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "encoding project config: "+err.Error())
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil { //nolint:gosec // shared, non-secret, committed file
		return "", figctl.Wrap(figctl.CodeInternal, err, fmt.Sprintf("writing %s: %v", path, err))
	}
	p.Path = path
	return path, nil
}
