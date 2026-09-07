package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

// The helpers in this file work on Figma JSON decoded generically, so that
// properties this CLI does not model survive a round trip. Numbers are
// kept as json.Number to avoid float formatting changes.

// decodeGeneric decodes JSON into generic values with numbers preserved.
func decodeGeneric(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding Figma JSON: "+err.Error())
	}
	return out, nil
}

// genericChildren returns the children array of a generic node.
func genericChildren(node map[string]any) []map[string]any {
	raw, _ := node["children"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// genericWalk visits node and its descendants depth first. fn returns
// false to skip a node's children.
func genericWalk(node map[string]any, fn func(node map[string]any) bool) {
	if node == nil || !fn(node) {
		return
	}
	for _, child := range genericChildren(node) {
		genericWalk(child, fn)
	}
}

// genericFind returns the descendant (or the node itself) with the given
// id, or nil.
func genericFind(node map[string]any, id string) map[string]any {
	var found map[string]any
	genericWalk(node, func(n map[string]any) bool {
		if found != nil {
			return false
		}
		if s, _ := n["id"].(string); s == id {
			found = n
			return false
		}
		return true
	})
	return found
}

// genericPrune returns a copy of node with children removed below depth.
// Depth 0 keeps the node only; a negative depth keeps everything.
func genericPrune(node map[string]any, depth int) map[string]any {
	copied := make(map[string]any, len(node))
	for k, v := range node {
		copied[k] = v
	}
	if depth < 0 {
		return copied
	}
	children := genericChildren(node)
	if depth == 0 {
		delete(copied, "children")
		return copied
	}
	if _, has := node["children"]; has {
		pruned := make([]any, 0, len(children))
		for _, child := range children {
			pruned = append(pruned, genericPrune(child, depth-1))
		}
		copied["children"] = pruned
	}
	return copied
}

// genericNodeEntries builds the nodes object of a GET nodes response from
// a generic whole document, carrying the components, component sets, and
// styles each subtree references. Unknown ids map to nil.
func genericNodeEntries(file map[string]any, ids []string, depth int) map[string]any {
	document, _ := file["document"].(map[string]any)
	components, _ := file["components"].(map[string]any)
	componentSets, _ := file["componentSets"].(map[string]any)
	styles, _ := file["styles"].(map[string]any)
	pruneDepth := -1
	if depth > 0 {
		pruneDepth = depth
	}
	out := make(map[string]any, len(ids))
	for _, id := range ids {
		node := genericFind(document, id)
		if node == nil {
			out[id] = nil
			continue
		}
		entry := map[string]any{
			"document":      genericPrune(node, pruneDepth),
			"components":    map[string]any{},
			"componentSets": map[string]any{},
			"styles":        map[string]any{},
		}
		if v, ok := file["schemaVersion"]; ok {
			entry["schemaVersion"] = v
		}
		usedComponents := entry["components"].(map[string]any)
		usedSets := entry["componentSets"].(map[string]any)
		usedStyles := entry["styles"].(map[string]any)
		genericWalk(node, func(n map[string]any) bool {
			if cid, _ := n["componentId"].(string); cid != "" {
				if c, ok := components[cid]; ok {
					usedComponents[cid] = c
					if cm, ok := c.(map[string]any); ok {
						if sid, _ := cm["componentSetId"].(string); sid != "" {
							if set, ok := componentSets[sid]; ok {
								usedSets[sid] = set
							}
						}
					}
				}
			}
			if refs, ok := n["styles"].(map[string]any); ok {
				for _, ref := range refs {
					if sid, _ := ref.(string); sid != "" {
						if st, ok := styles[sid]; ok {
							usedStyles[sid] = st
						}
					}
				}
			}
			return true
		})
		out[id] = entry
	}
	return out
}

// NodesRaw returns the requested nodes as Figma sent them, keyed by id
// with nil for unknown ids. Without geometry or plugin data they are cut
// from the cached whole document; otherwise, or when the document is too
// large, GET nodes is called and its bytes cached.
func (s *Session) NodesRaw(ctx context.Context, key string, ids []string, opts figma.NodesOptions) (map[string]any, error) {
	opts.Version = s.version
	if opts.Geometry == "" && opts.PluginData == "" {
		raw, err := s.FileRaw(ctx, key)
		if err == nil {
			file, err := decodeGeneric(raw)
			if err != nil {
				return nil, err
			}
			return genericNodeEntries(file, ids, opts.Depth), nil
		}
		if !errors.Is(err, errTooLarge) {
			return nil, err
		}
	}
	q := opts.Query()
	q.Set("ids", joinIDs(ids))
	raw, err := cached(ctx, s, key, cache.Key("nodes", q), func(ctx context.Context) (*json.RawMessage, error) {
		body, err := s.Client.GetFileNodesRaw(ctx, key, ids, opts)
		if err != nil {
			return nil, err
		}
		return &body, nil
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		fileHead
		Nodes map[string]json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(*raw, &resp); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding nodes response: "+err.Error())
	}
	s.touch(key, resp.Name, resp.Version, resp.LastModified)
	out := make(map[string]any, len(ids))
	for _, id := range ids {
		entry, ok := resp.Nodes[id]
		if !ok || bytes.Equal(bytes.TrimSpace(entry), []byte("null")) {
			out[id] = nil
			continue
		}
		out[id] = entry
	}
	return out, nil
}

// FileRawWith fetches the document with non-default query options (depth,
// geometry, plugin data) as Figma sent it, cached under its own entry.
func (s *Session) FileRawWith(ctx context.Context, key string, opts figma.FileOptions) (json.RawMessage, error) {
	opts.Version = s.version
	opts.BranchData = true
	raw, err := cached(ctx, s, key, cache.Key("file", opts.Query()), func(ctx context.Context) (*json.RawMessage, error) {
		body, err := s.Client.GetFileRaw(ctx, key, opts)
		if err != nil {
			return nil, err
		}
		return &body, nil
	})
	if err != nil {
		return nil, err
	}
	var head fileHead
	if err := json.Unmarshal(*raw, &head); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding file document: "+err.Error())
	}
	s.touch(key, head.Name, head.Version, head.LastModified)
	return *raw, nil
}
