// Command gen-skill-docs writes the generated reference documents of the
// figctl agent skill into internal/skill/templates, where they are
// embedded in the binary.
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

	"github.com/tiaanduplessis/figctl/internal/skilldoc"
)

// defaultOut is the template directory, relative to the repository root.
const defaultOut = "internal/skill/templates"

func main() {
	out := flag.String("out", defaultOut, "template directory to write into")
	check := flag.Bool("check", false, "exit non-zero when a file on disk differs, writing nothing")
	flag.Parse()

	if err := generate(*out, *check); err != nil {
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
