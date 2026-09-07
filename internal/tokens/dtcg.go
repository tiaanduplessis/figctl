package tokens

import (
	"bytes"
	"encoding/json"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// FigmaExtension is the reverse domain key every figctl specific
// addition is nested under, as the DTCG format requires.
const FigmaExtension = "com.figma"

// Output is one rendered file.
type Output struct {
	// Name is the suggested file name, for example tokens.json.
	Name string
	// Mode is the mode of a per mode file, or empty.
	Mode string
	// Content is the rendered bytes.
	Content []byte
	// Tokens counts the tokens the file carries.
	Tokens int
	// Warnings reports values the writer had to drop or merge.
	Warnings []string
}

// dtcgToken is one token object in field order: $value first, then the
// type, then the optional metadata.
type dtcgToken struct {
	Value       any            `json:"$value"`
	Type        string         `json:"$type"`
	Description string         `json:"$description,omitempty"`
	Extensions  map[string]any `json:"$extensions,omitempty"`
}

// DTCG renders the document as Design Tokens Format Module 2025.10 JSON,
// one file per set. Groups nest by path segment, every token carries its
// own $type rather than hoisting a group level one, and an alias becomes
// a {group.token} reference when its target is in the same file.
//
// The --format style-dictionary output is byte identical: Style
// Dictionary v4 consumes DTCG unchanged, so it is an alias of this
// writer rather than a second format.
func (d *Document) DTCG() ([]Output, error) {
	outs := make([]Output, 0, len(d.Sets))
	for _, set := range d.Sets {
		root := map[string]any{}
		for _, t := range set.Tokens {
			if err := insert(root, t.Path, d.dtcgToken(t)); err != nil {
				return nil, err
			}
		}
		content, err := marshalIndent(root)
		if err != nil {
			return nil, err
		}
		outs = append(outs, Output{Name: fileName("tokens", set.Mode, "json"), Mode: set.Mode, Content: content, Tokens: len(set.Tokens)})
	}
	return outs, nil
}

func (d *Document) dtcgToken(t Token) dtcgToken {
	out := dtcgToken{Value: t.Value, Type: t.Type, Description: t.Description}
	if t.Ref != "" {
		out.Value = "{" + t.Ref + "}"
	}
	ext := map[string]any{}
	for k, v := range t.Extensions {
		ext[k] = v
	}
	if len(t.Scopes) > 0 {
		ext["scopes"] = t.Scopes
	}
	if len(t.CodeSyntax) > 0 {
		ext["codeSyntax"] = t.CodeSyntax
	}
	// Only the default strategy has to carry the other modes, because
	// the separate and select strategies express the mode in the file or
	// in the selection instead.
	if d.Strategy == StrategyDefault && len(t.Modes) > 1 {
		ext["modes"] = t.Modes
	}
	if len(ext) > 0 {
		out.Extensions = map[string]any{FigmaExtension: ext}
	}
	return out
}

// insert places a token at its path, creating groups as it walks.
func insert(root map[string]any, path []string, token dtcgToken) error {
	node := root
	for i, seg := range path {
		if i == len(path)-1 {
			node[seg] = token
			return nil
		}
		child, exists := node[seg]
		if !exists {
			next := map[string]any{}
			node[seg] = next
			node = next
			continue
		}
		next, isGroup := child.(map[string]any)
		if !isGroup {
			return figctl.Newf(figctl.CodeUsage, "token %q is both a token and a group", joinPath(path[:i+1])).
				WithHint("A Figma name cannot be both a token and the prefix of another token. Rename one of them, or export with --name-case none.")
		}
		node = next
	}
	return nil
}

func joinPath(path []string) string {
	out := ""
	for i, seg := range path {
		if i > 0 {
			out += "."
		}
		out += seg
	}
	return out
}

// JSON renders figctl's own flat token listing, one file per set. It
// carries the fields DTCG has no place for (the Figma ids, the CSS
// name, the Tailwind category) and is meant for a build script that
// wants the resolved values without walking a tree.
func (d *Document) JSON() ([]Output, error) {
	outs := make([]Output, 0, len(d.Sets))
	for _, set := range d.Sets {
		tokenList := set.Tokens
		if tokenList == nil {
			tokenList = []Token{}
		}
		content, err := marshalIndent(tokenList)
		if err != nil {
			return nil, err
		}
		outs = append(outs, Output{Name: fileName("tokens", set.Mode, "json"), Mode: set.Mode, Content: content, Tokens: len(set.Tokens)})
	}
	return outs, nil
}

func marshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "encoding tokens: "+err.Error())
	}
	return buf.Bytes(), nil
}

// fileName builds tokens.json or tokens.<mode>.json.
func fileName(base, mode, ext string) string {
	if mode == "" {
		return base + "." + ext
	}
	return base + "." + kebab(mode) + "." + ext
}
