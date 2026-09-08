// Command gen-skill-docs writes the generated reference documents of the
// figctl agent skill into internal/skill/templates, where they are embedded
// in the binary, and mirrors the whole skill into skills/figctl so that skill
// installers which walk a repository can find it.
//
// Run it with "make gen" after changing a command, a flag, or an output
// type. "make verify-gen" fails when the committed files are stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tiaanduplessis/figctl/internal/skill"
	"github.com/tiaanduplessis/figctl/internal/skilldoc"
)

// defaultOut is the template directory, relative to the repository root.
const defaultOut = "internal/skill/templates"

// publishDir is where the skill is published for installers that discover
// skills by walking a repository, such as "npx skills add <owner>/<repo>".
// They look for a SKILL.md at the repository root, under skills/, or under an
// agent directory, and internal/skill/templates is none of those.
const publishDir = "skills/figctl"

func main() {
	out := flag.String("out", defaultOut, "template directory to write into")
	publishTo := flag.String("publish", publishDir, "directory to publish the installable skill into")
	check := flag.Bool("check", false, "exit non-zero when a file on disk differs, writing nothing")
	flag.Parse()

	if err := generate(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, "gen-skill-docs:", err)
		os.Exit(1)
	}
	if err := publish(*publishTo, *check); err != nil {
		fmt.Fprintln(os.Stderr, "gen-skill-docs:", err)
		os.Exit(1)
	}
}

func generate(dir string, check bool) error {
	docs := skilldoc.Documents()
	paths := make([]string, 0, len(docs))
	for path := range docs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var stale []string
	for _, rel := range paths {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		want := docs[rel]
		got, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if string(got) == want {
			continue
		}
		if check {
			stale = append(stale, path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Println("wrote", path, len(want), "bytes")
	}
	if len(stale) > 0 {
		return fmt.Errorf("out of date: %v; run make gen", stale)
	}
	return nil
}

// publish mirrors the embedded skill into a directory a skill installer can
// discover. The published copy carries no ownership marker: another tool
// installs it, so figctl does not claim it.
func publish(dir string, check bool) error {
	var stale []string
	for _, f := range skill.Files() {
		path := filepath.Join(dir, filepath.FromSlash(f.Path))
		want := f.Template()
		got, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if string(got) == want {
			continue
		}
		if check {
			stale = append(stale, path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Println("published", path, len(want), "bytes")
	}
	if len(stale) > 0 {
		return fmt.Errorf("out of date: %v; run make gen", stale)
	}
	return nil
}
