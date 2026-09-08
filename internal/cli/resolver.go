package cli

import (
	"context"
	"errors"
	"sort"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/inspect"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// Hints shown when variables cannot be read.
const (
	hintVariablesPlan  = "Variables need an Enterprise plan with a full seat; showing styles and raw values instead."
	hintVariablesScope = "The token is missing the " + figma.ScopeFileVariables + " scope, so variables are not resolved; showing styles and raw values instead. That scope is offered only to members of an Enterprise organization and does not appear on the token screen on other plans, so this is a plan limit rather than a token to re-create."
)

// Variables fetches the local variables of a file once per version and
// caches them. On plans or tokens without variables access it returns
// nil and a hint instead of an error so commands degrade to styles and
// raw values.
func (s *Session) Variables(ctx context.Context, key string) (*figma.GetLocalVariablesResponse, string, error) {
	vars, err := s.localVariables(ctx, key)
	if err == nil {
		return vars, "", nil
	}
	switch figctl.From(err).Code {
	case figctl.CodePlanRequired:
		s.ctx.Log.Warnf("%s", hintVariablesPlan)
		return nil, hintVariablesPlan, nil
	case figctl.CodeAuthScope:
		s.ctx.Log.Warnf("%s", hintVariablesScope)
		return nil, hintVariablesScope, nil
	}
	return nil, "", err
}

// localVariables fetches local variables, cached per file version.
func (s *Session) localVariables(ctx context.Context, key string) (*figma.GetLocalVariablesResponse, error) {
	return cached(ctx, s, key, cache.Key("variables_local", nil), func(ctx context.Context) (*figma.GetLocalVariablesResponse, error) {
		return s.Client.GetLocalVariables(ctx, key)
	})
}

// PublishedVariables fetches the published variables of a file, cached
// per version.
func (s *Session) PublishedVariables(ctx context.Context, key string) (*figma.GetPublishedVariablesResponse, error) {
	return cached(ctx, s, key, cache.Key("variables_published", nil), func(ctx context.Context) (*figma.GetPublishedVariablesResponse, error) {
		return s.Client.GetPublishedVariables(ctx, key)
	})
}

// StyleNodes fetches style nodes in one batched GET nodes call, cached
// per version. Style nodes are not part of the document, so this always
// goes to the nodes endpoint. Unknown ids are absent from the result.
func (s *Session) StyleNodes(ctx context.Context, key string, ids []string) (map[string]*figma.Node, error) {
	out := map[string]*figma.Node{}
	if len(ids) == 0 {
		return out, nil
	}
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	opts := figma.NodesOptions{Version: s.version}
	q := opts.Query()
	q.Set("ids", joinIDs(ids))
	resp, err := cached(ctx, s, key, cache.Key("nodes", q), func(ctx context.Context) (*figma.GetFileNodesResponse, error) {
		return s.Client.GetFileNodes(ctx, key, ids, opts)
	})
	if err != nil {
		return nil, err
	}
	for id, entry := range resp.Nodes {
		if entry != nil && entry.Document != nil {
			out[id] = entry.Document
		}
	}
	return out, nil
}

// StyleCatalog returns the styles of a file keyed by node id: the styles
// referenced by the document plus the published styles of the file. A
// missing library scope only drops the published list.
func (s *Session) StyleCatalog(ctx context.Context, key string) (map[string]figma.Style, string, error) {
	out := map[string]figma.Style{}
	file, err := s.File(ctx, key)
	if err == nil {
		for id, st := range file.Styles {
			out[id] = st
		}
	} else if !errors.Is(err, errTooLarge) {
		return nil, "", err
	}
	published, err := cached(ctx, s, key, cache.Key("styles", nil), func(ctx context.Context) (*figma.StylesResponse, error) {
		return s.Client.GetFileStyles(ctx, key)
	})
	if err != nil {
		switch figctl.From(err).Code {
		case figctl.CodeAuthScope, figctl.CodeForbidden:
			hint := "Published styles were not listed: " + figctl.From(err).Message + ". Only styles used by the document are shown."
			return out, hint, nil
		}
		return nil, "", err
	}
	s.Touch(key)
	for _, st := range published.Meta.Styles {
		if _, exists := out[st.NodeID]; !exists {
			out[st.NodeID] = figma.Style{Key: st.Key, Name: st.Name, Description: st.Description, StyleType: st.StyleType}
		}
	}
	return out, "", nil
}

// fileMetadata returns the whole document when available, otherwise a
// file built from the metadata carried by the fetched node entries so
// components, sets, and styles still resolve for large files.
func (s *Session) fileMetadata(ctx context.Context, key string, nodes *figma.GetFileNodesResponse, ids ...string) (*figma.GetFileResponse, error) {
	file, err := s.File(ctx, key)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, errTooLarge) {
		return nil, err
	}
	// The whole document is out of reach, but the document narrowed to these
	// nodes is not, and it still carries the page and the ancestors. Without
	// it the caller loses the page name, the path, and the measurements the
	// designer pinned on the page.
	if scoped, serr := s.Scoped(ctx, key, ids); serr == nil && scoped.Document != nil {
		return scoped, nil
	} else if serr != nil && !errors.Is(serr, errTooLarge) {
		s.ctx.Log.Debugf("scoped document fetch failed for %s: %v", key, serr)
	}
	file = &figma.GetFileResponse{
		Components:    map[string]figma.Component{},
		ComponentSets: map[string]figma.ComponentSet{},
		Styles:        map[string]figma.Style{},
	}
	if nodes == nil {
		return file, nil
	}
	file.Name, file.Version, file.LastModified = nodes.Name, nodes.Version, nodes.LastModified
	for _, entry := range nodes.Nodes {
		if entry == nil {
			continue
		}
		for id, c := range entry.Components {
			file.Components[id] = c
		}
		for id, c := range entry.ComponentSets {
			file.ComponentSets[id] = c
		}
		for id, st := range entry.Styles {
			file.Styles[id] = st
		}
	}
	return file, nil
}

// Resolver builds a resolver for the given subtrees: variables once per
// file (with graceful fallback), and every referenced style node in one
// batched request.
func (s *Session) Resolver(ctx context.Context, key string, file *figma.GetFileResponse, roots ...*figma.Node) (*resolve.Resolver, []string, error) {
	var hints []string
	vars, hint, err := s.Variables(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if hint != "" {
		hints = append(hints, hint)
	}
	styleNodes, err := s.StyleNodes(ctx, key, inspect.StyleIDs(roots...))
	if err != nil {
		return nil, nil, err
	}
	return resolve.New(resolve.Options{File: file, Variables: vars, StyleNodes: styleNodes}), hints, nil
}
