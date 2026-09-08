// Command spec-check reports whether the Figma OpenAPI specification
// still carries the version that the hand-written types in internal/figma
// were built against.
//
// Run it with "make spec-check". It downloads the spec, so it needs
// network access and is deliberately not part of "make check".
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

// timeout bounds the download of the spec document.
const timeout = 30 * time.Second

// maxSpecBytes bounds how much of the document is read, so a redirect to
// something unexpected cannot exhaust memory.
const maxSpecBytes = 32 << 20

func main() {
	published, err := fetchSpecVersion(context.Background(), figma.SpecURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec-check:", err)
		os.Exit(1)
	}
	if published == figma.SpecVersion {
		fmt.Printf("Figma OpenAPI spec is still version %s, matching the pin in internal/figma/spec.go.\n", published)
		return
	}
	fmt.Fprintf(os.Stderr, `spec-check: the Figma OpenAPI spec has moved.
  pinned in internal/figma/spec.go: %s
  published at %s: %s

The types, endpoints, and node properties in internal/figma are written by
hand and do not follow the spec on their own. Review what changed, update
them, then move figma.SpecVersion to %s.
  changelog: %s
  releases:  %s
`, figma.SpecVersion, figma.SpecURL, published, published, figma.SpecChangelogURL, figma.SpecReleasesURL)
	os.Exit(1)
}

// fetchSpecVersion downloads the OpenAPI document and returns its
// info.version.
func fetchSpecVersion(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", url, err)
	}

	var doc struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("parsing %s: %w", url, err)
	}
	if doc.Info.Version == "" {
		return "", fmt.Errorf("%s has no info.version", url)
	}
	return doc.Info.Version, nil
}
