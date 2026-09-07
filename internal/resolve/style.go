package resolve

import (
	"sort"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
)

// Style is a style with its metadata and resolved value.
type Style struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
	// Value is nil when the style node was not supplied.
	Value *StyleValue `json:"value"`
}

// Ref returns the style as a reference.
func (s *Style) Ref() *model.StyleRef {
	return &model.StyleRef{ID: s.ID, Key: s.Key, Name: s.Name, Type: s.Type}
}

// StyleValue is the resolved value of a style; exactly one field is set
// depending on the style type.
type StyleValue struct {
	Fills      []model.Fill       `json:"fills,omitempty"`
	Typography *model.Text        `json:"typography,omitempty"`
	Effects    []model.Effect     `json:"effects,omitempty"`
	Grids      []model.LayoutGrid `json:"grids,omitempty"`
}

// AddStyleNode registers a style node fetched after construction.
func (r *Resolver) AddStyleNode(id string, node *figma.Node) {
	if node != nil {
		r.styleNodes[id] = node
	}
}

// AddStyle registers style metadata that did not come from the file
// document, for example from the file or team styles endpoints.
func (r *Resolver) AddStyle(id string, s figma.Style) {
	if r.file == nil {
		r.file = &figma.GetFileResponse{}
	}
	if r.file.Styles == nil {
		r.file.Styles = map[string]figma.Style{}
	}
	if _, exists := r.file.Styles[id]; !exists {
		r.file.Styles[id] = s
	}
}

// StyleRef returns a reference to a style by id, or nil when the file
// does not list it.
func (r *Resolver) StyleRef(id string) *model.StyleRef {
	if id == "" || r.file == nil {
		return nil
	}
	s, ok := r.file.Styles[id]
	if !ok {
		return &model.StyleRef{ID: id, Name: "", Type: ""}
	}
	return &model.StyleRef{ID: id, Key: s.Key, Name: s.Name, Type: s.StyleType}
}

// Style returns a style with its resolved value: FILL as paints, TEXT as
// typography, EFFECT as effects, GRID as layout grids.
func (r *Resolver) Style(id string) (*Style, bool) {
	if r.file == nil {
		return nil, false
	}
	s, ok := r.file.Styles[id]
	if !ok {
		return nil, false
	}
	out := &Style{ID: id, Key: s.Key, Name: s.Name, Type: s.StyleType, Description: s.Description, Remote: s.Remote}
	if node, ok := r.styleNodes[id]; ok && node != nil {
		out.Value = r.styleValue(s.StyleType, node)
	}
	return out, true
}

// StyleByKey returns a style by its key.
func (r *Resolver) StyleByKey(key string) (*Style, bool) {
	if r.file == nil {
		return nil, false
	}
	for id, s := range r.file.Styles {
		if s.Key == key {
			return r.Style(id)
		}
	}
	return nil, false
}

// StyleByName returns a style by name.
func (r *Resolver) StyleByName(name string) (*Style, bool) {
	for _, s := range r.Styles() {
		if s.Name == name {
			return r.Style(s.ID)
		}
	}
	return nil, false
}

// Styles lists every style in the file sorted by type then name.
func (r *Resolver) Styles() []Style {
	if r.file == nil {
		return nil
	}
	ids := make([]string, 0, len(r.file.Styles))
	for id := range r.file.Styles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := r.file.Styles[ids[i]], r.file.Styles[ids[j]]
		if a.StyleType != b.StyleType {
			return a.StyleType < b.StyleType
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return ids[i] < ids[j]
	})
	out := make([]Style, 0, len(ids))
	for _, id := range ids {
		s, _ := r.Style(id)
		out = append(out, *s)
	}
	return out
}

// StyleValueOf resolves the value of a style node for the given style
// type without needing the style to be listed in the file.
func (r *Resolver) StyleValueOf(styleType string, node *figma.Node) *StyleValue {
	return r.styleValue(styleType, node)
}

func (r *Resolver) styleValue(styleType string, node *figma.Node) *StyleValue {
	switch styleType {
	case "FILL":
		return &StyleValue{Fills: r.Fills(node, "fills", node.Fills, nil)}
	case "TEXT":
		return &StyleValue{Typography: r.Typography(node.Style, nil)}
	case "EFFECT":
		return &StyleValue{Effects: r.Effects(node.Effects, nil)}
	case "GRID":
		return &StyleValue{Grids: r.Grids(node.LayoutGrids, nil)}
	}
	return nil
}
