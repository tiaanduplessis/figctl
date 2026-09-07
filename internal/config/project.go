package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// ProjectFileName is the project config file committed in a repository.
const ProjectFileName = ".figctl.yaml"

// Project is the project level config. It never contains secrets.
type Project struct {
	// Profile names the profile to use in this repository.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// File is the default file key for commands that take a ref.
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// Path is where the file was found. It is not serialized.
	Path string `yaml:"-" json:"path,omitempty"`
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
