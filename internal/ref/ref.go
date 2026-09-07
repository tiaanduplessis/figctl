// Package ref parses Figma references: bare file or branch keys and every
// Figma URL form, including the node-id and version-id query parameters.
package ref

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Kind says which URL form (or bare key) a reference came from.
type Kind string

// Reference kinds.
const (
	KindKey    Kind = "key"
	KindFile   Kind = "file"
	KindDesign Kind = "design"
	KindBoard  Kind = "board"
	KindProto  Kind = "proto"
	KindSlides Kind = "slides"
)

// Ref is a parsed Figma reference.
type Ref struct {
	// FileKey is the file key, or the branch key for branch URLs.
	FileKey string
	// NodeID is the node-id query parameter in API form (1:2), if present.
	NodeID string
	// VersionID is the version-id query parameter, if present.
	VersionID string
	// Kind is the URL form the reference was parsed from.
	Kind Kind
}

var (
	keyPattern  = regexp.MustCompile(`^[A-Za-z0-9]{10,128}$`)
	nodePattern = regexp.MustCompile(`^I?[0-9]+[:-][0-9]+(?:;[0-9]+[:-][0-9]+)*$`)
)

const refHint = "Pass a file key or a Figma URL such as https://www.figma.com/design/KEY/Name?node-id=1-2."

// Parse parses a file key, branch key, or Figma URL.
func Parse(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}, figctl.New(figctl.CodeUsage, "empty Figma reference").WithHint(refHint)
	}
	if keyPattern.MatchString(s) {
		return Ref{FileKey: s, Kind: KindKey}, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return Ref{}, figctl.Newf(figctl.CodeUsage, "invalid Figma reference %q", s).WithHint(refHint)
	}
	host := strings.ToLower(u.Hostname())
	if host != "figma.com" && !strings.HasSuffix(host, ".figma.com") {
		return Ref{}, figctl.Newf(figctl.CodeUsage, "not a Figma URL: %q", s).WithHint(refHint)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return Ref{}, figctl.Newf(figctl.CodeUsage, "Figma URL has no file key: %q", s).WithHint(refHint)
	}
	var kind Kind
	switch segments[0] {
	case "file":
		kind = KindFile
	case "design":
		kind = KindDesign
	case "board":
		kind = KindBoard
	case "proto":
		kind = KindProto
	case "slides":
		kind = KindSlides
	default:
		return Ref{}, figctl.Newf(figctl.CodeUsage, "unsupported Figma URL form %q", segments[0]).
			WithHint("Supported forms: /file, /design, /board, /proto, /slides.")
	}
	key := segments[1]
	if len(segments) >= 4 && segments[2] == "branch" {
		key = segments[3]
	}
	if !keyPattern.MatchString(key) {
		return Ref{}, figctl.Newf(figctl.CodeUsage, "invalid file key %q in URL", key).WithHint(refHint)
	}
	r := Ref{FileKey: key, Kind: kind}
	q := u.Query()
	if nodeID := q.Get("node-id"); nodeID != "" {
		r.NodeID, err = ParseNodeID(nodeID)
		if err != nil {
			return Ref{}, err
		}
	}
	r.VersionID = q.Get("version-id")
	return r, nil
}

// ParseNodeID accepts a node ID in API form (1:2) or URL form (1-2) and
// returns the API form.
func ParseNodeID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !nodePattern.MatchString(s) {
		return "", figctl.Newf(figctl.CodeUsage, "invalid node id %q", s).
			WithHint("Node ids look like 1:2 (or 1-2 as found in Figma URLs).")
	}
	return strings.ReplaceAll(s, "-", ":"), nil
}
