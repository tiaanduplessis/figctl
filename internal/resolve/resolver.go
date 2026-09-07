package resolve

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

// Errors returned by variable resolution.
var (
	ErrUnknownVariable = errors.New("unknown variable")
	ErrCycle           = errors.New("alias cycle")
)

// Options are the inputs of a Resolver. Every field is optional; missing
// inputs reduce what can be resolved rather than causing errors.
type Options struct {
	// File supplies style and component metadata (file.styles).
	File *figma.GetFileResponse
	// Variables is the local variables response (Enterprise only).
	Variables *figma.GetLocalVariablesResponse
	// Published is the published variables response, used to map
	// subscribed ids and keys when a node references library variables.
	Published *figma.GetPublishedVariablesResponse
	// StyleNodes holds style nodes fetched with GET nodes, keyed by node
	// id, which is also the style id in file.styles.
	StyleNodes map[string]*figma.Node
}

// Resolver resolves variables, styles, and paints for one file.
type Resolver struct {
	file        *figma.GetFileResponse
	vars        map[string]figma.Variable
	collections map[string]figma.VariableCollection
	byKey       map[string]string
	published   *figma.GetPublishedVariablesResponse
	styleNodes  map[string]*figma.Node
	modeOwner   map[string]string
}

// New builds a Resolver.
func New(opts Options) *Resolver {
	r := &Resolver{
		file:        opts.File,
		vars:        map[string]figma.Variable{},
		collections: map[string]figma.VariableCollection{},
		byKey:       map[string]string{},
		published:   opts.Published,
		styleNodes:  opts.StyleNodes,
		modeOwner:   map[string]string{},
	}
	if r.styleNodes == nil {
		r.styleNodes = map[string]*figma.Node{}
	}
	if opts.Variables != nil {
		for id, v := range opts.Variables.Meta.Variables {
			if v.ID == "" {
				v.ID = id
			}
			r.vars[v.ID] = v
			if v.Key != "" {
				r.byKey[v.Key] = v.ID
			}
		}
		for id, c := range opts.Variables.Meta.VariableCollections {
			if c.ID == "" {
				c.ID = id
			}
			r.collections[c.ID] = c
			for _, m := range c.Modes {
				r.modeOwner[m.ModeID] = c.ID
			}
		}
	}
	return r
}

// HasVariables reports whether local variable definitions are available.
func (r *Resolver) HasVariables() bool { return len(r.vars) > 0 }

// HasPublished reports whether published variable metadata is available.
func (r *Resolver) HasPublished() bool { return r.published != nil }

// Mode is a mode of a variable collection.
type Mode struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Default      bool   `json:"default,omitempty"`
	ParentModeID string `json:"parentModeId,omitempty"`
}

// Collection is a variable collection with its modes.
type Collection struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Key                  string   `json:"key,omitempty"`
	Modes                []Mode   `json:"modes"`
	DefaultModeID        string   `json:"defaultModeId"`
	ParentCollectionID   string   `json:"parentCollectionId,omitempty"`
	Remote               bool     `json:"remote,omitempty"`
	HiddenFromPublishing bool     `json:"hiddenFromPublishing,omitempty"`
	VariableIDs          []string `json:"variableIds,omitempty"`
}

// Modes lists every collection with its modes, sorted by name.
func (r *Resolver) Modes() []Collection {
	out := make([]Collection, 0, len(r.collections))
	for _, c := range r.collections {
		out = append(out, r.collection(c))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *Resolver) collection(c figma.VariableCollection) Collection {
	out := Collection{
		ID:                   c.ID,
		Name:                 c.Name,
		Key:                  c.Key,
		Modes:                make([]Mode, 0, len(c.Modes)),
		DefaultModeID:        c.DefaultModeID,
		ParentCollectionID:   c.ParentVariableCollectionID,
		Remote:               c.Remote,
		HiddenFromPublishing: c.HiddenFromPublishing,
		VariableIDs:          c.VariableIDs,
	}
	for _, m := range c.Modes {
		out.Modes = append(out.Modes, Mode{ID: m.ModeID, Name: m.Name, Default: m.ModeID == c.DefaultModeID, ParentModeID: m.ParentModeID})
	}
	return out
}

// Collection returns one collection by id.
func (r *Resolver) Collection(id string) (Collection, bool) {
	c, ok := r.collections[id]
	if !ok {
		return Collection{}, false
	}
	return r.collection(c), true
}

// Variable is the definition of a variable without its values.
type Variable struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Key                  string            `json:"key,omitempty"`
	CollectionID         string            `json:"collectionId"`
	Collection           string            `json:"collection"`
	ResolvedType         string            `json:"resolvedType"`
	Scopes               []string          `json:"scopes,omitempty"`
	CodeSyntax           map[string]string `json:"codeSyntax,omitempty"`
	Description          string            `json:"description,omitempty"`
	HiddenFromPublishing bool              `json:"hiddenFromPublishing,omitempty"`
	Remote               bool              `json:"remote,omitempty"`
	// Modes are the modes of the variable's own collection.
	Modes []Mode `json:"modes,omitempty"`
	// Published is set when the definition came from the published
	// variables response only, so no values are available.
	Published bool `json:"published,omitempty"`
}

// Variable returns a variable definition by id, subscribed id, or key.
func (r *Resolver) Variable(id string) (*Variable, bool) {
	if v, ok := r.lookup(id); ok {
		return r.variable(v), true
	}
	if r.published != nil {
		for pid, p := range r.published.Meta.Variables {
			if p.ID == "" {
				p.ID = pid
			}
			if p.ID == id || p.SubscribedID == id || (p.Key != "" && strings.Contains(id, p.Key)) {
				out := &Variable{ID: p.ID, Name: p.Name, Key: p.Key, CollectionID: p.VariableCollectionID, ResolvedType: p.ResolvedDataType, Published: true}
				if c, ok := r.published.Meta.VariableCollections[p.VariableCollectionID]; ok {
					out.Collection = c.Name
				}
				return out, true
			}
		}
	}
	return nil, false
}

// VariableByName returns a local variable by its full name (for example
// color/brand/500), optionally qualified as collection/name.
func (r *Resolver) VariableByName(name string) (*Variable, bool) {
	name = strings.TrimSpace(name)
	for _, v := range r.sortedVariables() {
		if v.Name == name {
			return r.variable(v), true
		}
	}
	for _, v := range r.sortedVariables() {
		if c, ok := r.collections[v.VariableCollectionID]; ok && c.Name+"/"+v.Name == name {
			return r.variable(v), true
		}
	}
	for _, v := range r.sortedVariables() {
		if strings.EqualFold(v.Name, name) {
			return r.variable(v), true
		}
	}
	return nil, false
}

// Variables lists every local variable sorted by collection then name.
func (r *Resolver) Variables() []Variable {
	vars := r.sortedVariables()
	out := make([]Variable, 0, len(vars))
	for _, v := range vars {
		out = append(out, *r.variable(v))
	}
	return out
}

func (r *Resolver) sortedVariables() []figma.Variable {
	vars := make([]figma.Variable, 0, len(r.vars))
	for _, v := range r.vars {
		vars = append(vars, v)
	}
	sort.Slice(vars, func(i, j int) bool {
		ci, cj := r.collections[vars[i].VariableCollectionID].Name, r.collections[vars[j].VariableCollectionID].Name
		if ci != cj {
			return ci < cj
		}
		if vars[i].Name != vars[j].Name {
			return vars[i].Name < vars[j].Name
		}
		return vars[i].ID < vars[j].ID
	})
	return vars
}

func (r *Resolver) variable(v figma.Variable) *Variable {
	out := &Variable{
		ID:                   v.ID,
		Name:                 v.Name,
		Key:                  v.Key,
		CollectionID:         v.VariableCollectionID,
		ResolvedType:         v.ResolvedType,
		Scopes:               v.Scopes,
		CodeSyntax:           codeSyntax(v.CodeSyntax),
		Description:          v.Description,
		HiddenFromPublishing: v.HiddenFromPublishing,
		Remote:               v.Remote,
	}
	if c, ok := r.collections[v.VariableCollectionID]; ok {
		out.Collection = c.Name
		out.Modes = r.collection(c).Modes
	}
	return out
}

func codeSyntax(cs figma.VariableCodeSyntax) map[string]string {
	out := map[string]string{}
	if cs.Web != "" {
		out["WEB"] = cs.Web
	}
	if cs.Android != "" {
		out["ANDROID"] = cs.Android
	}
	if cs.IOS != "" {
		out["iOS"] = cs.IOS
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// lookup finds a local variable by id, then by subscribed id through the
// published response, then by key.
func (r *Resolver) lookup(id string) (figma.Variable, bool) {
	if v, ok := r.vars[id]; ok {
		return v, true
	}
	if r.published != nil {
		for pid, p := range r.published.Meta.Variables {
			if p.ID == "" {
				p.ID = pid
			}
			if p.SubscribedID == id || p.ID == id {
				if v, ok := r.vars[p.ID]; ok {
					return v, true
				}
				if vid, ok := r.byKey[p.Key]; ok {
					return r.vars[vid], true
				}
			}
		}
	}
	for key, vid := range r.byKey {
		if strings.Contains(id, key) {
			return r.vars[vid], true
		}
	}
	if i := strings.LastIndex(id, "/"); i > 0 {
		if v, ok := r.vars[id[:i]]; ok {
			return v, true
		}
	}
	return figma.Variable{}, false
}

// Value is a resolved variable value in one mode.
type Value struct {
	// Type is the resolved type: COLOR, FLOAT, STRING, or BOOLEAN.
	Type   string   `json:"type"`
	Color  *Color   `json:"-"`
	Number *float64 `json:"-"`
	String *string  `json:"-"`
	Bool   *bool    `json:"-"`
	// Chain lists the names of the variables followed, starting with the
	// requested one and ending with the one holding the concrete value.
	Chain []string `json:"chain,omitempty"`
}

// Any returns the value as a JSON friendly scalar: hex (or #rrggbbaa)
// for colors, float64, string, or bool.
func (v Value) Any() any {
	switch {
	case v.Color != nil:
		if h := v.Color.Hex8(); h != "" {
			return h
		}
		return v.Color.Hex()
	case v.Number != nil:
		return *v.Number
	case v.String != nil:
		return *v.String
	case v.Bool != nil:
		return *v.Bool
	}
	return nil
}

// Text returns the value for display.
func (v Value) Text() string {
	switch x := v.Any().(type) {
	case nil:
		return ""
	case float64:
		return Num(x, 4)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(x)
	}
}

// Value resolves a variable in a mode, following aliases across
// collections. An empty mode means the default mode of the variable's
// collection. When the mode belongs to another collection, the mode with
// the same name in the variable's collection is used, falling back to the
// default mode. Extended collections apply their overrides and inherit
// from their parent mode.
func (r *Resolver) Value(id, modeID string) (Value, error) {
	return r.resolve(id, modeID, map[string]bool{}, nil)
}

func (r *Resolver) resolve(id, modeID string, visited map[string]bool, chain []string) (Value, error) {
	v, ok := r.lookup(id)
	if !ok {
		return Value{Chain: chain}, fmt.Errorf("%w: %s", ErrUnknownVariable, id)
	}
	if visited[v.ID] {
		return Value{Chain: chain}, fmt.Errorf("%w: %s", ErrCycle, strings.Join(append(chain, v.Name), " -> "))
	}
	visited[v.ID] = true
	chain = append(chain, v.Name)
	raw, ok := r.rawValue(v, modeID)
	if !ok {
		return Value{Type: v.ResolvedType, Chain: chain}, fmt.Errorf("%w: %s has no value in mode %s", ErrUnknownVariable, v.Name, modeID)
	}
	if raw.Alias != nil {
		return r.resolve(raw.Alias.ID, modeID, visited, chain)
	}
	out := Value{Type: v.ResolvedType, Chain: chain}
	switch {
	case raw.Color != nil:
		c := Color(*raw.Color)
		out.Color = &c
	case raw.Number != nil:
		out.Number = raw.Number
	case raw.String != nil:
		out.String = raw.String
	case raw.Bool != nil:
		out.Bool = raw.Bool
	}
	return out, nil
}

// rawValue picks the value of v for a requested mode, applying extended
// collection overrides and parent mode inheritance.
func (r *Resolver) rawValue(v figma.Variable, modeID string) (figma.VariableValue, bool) {
	own, hasOwn := r.collections[v.VariableCollectionID]
	if modeID == "" && hasOwn {
		modeID = own.DefaultModeID
	}
	for hops := 0; modeID != "" && hops < 16; hops++ {
		ownerID, known := r.modeOwner[modeID]
		if !known {
			break
		}
		owner := r.collections[ownerID]
		if ownerID == v.VariableCollectionID {
			if val, ok := v.ValuesByMode[modeID]; ok {
				return val, true
			}
			break
		}
		if r.extends(owner, v.VariableCollectionID) {
			if modes, ok := owner.VariableOverrides[v.ID]; ok {
				if val, ok := modes[modeID]; ok {
					return val, true
				}
			}
			parent := ""
			for _, m := range owner.Modes {
				if m.ModeID == modeID {
					parent = m.ParentModeID
				}
			}
			if parent == "" {
				parent = r.collections[owner.ParentVariableCollectionID].DefaultModeID
			}
			modeID = parent
			continue
		}
		// The mode belongs to an unrelated collection: prefer the mode
		// with the same name in the variable's collection.
		if hasOwn {
			name := modeName(owner, modeID)
			for _, m := range own.Modes {
				if m.Name == name {
					if val, ok := v.ValuesByMode[m.ModeID]; ok {
						return val, true
					}
				}
			}
		}
		break
	}
	if hasOwn {
		if val, ok := v.ValuesByMode[own.DefaultModeID]; ok {
			return val, true
		}
	}
	// No collection metadata: take any value.
	for _, val := range v.ValuesByMode {
		return val, true
	}
	return figma.VariableValue{}, false
}

// extends reports whether collection c extends (transitively) the
// collection with the given id.
func (r *Resolver) extends(c figma.VariableCollection, id string) bool {
	for hops := 0; c.ParentVariableCollectionID != "" && hops < 16; hops++ {
		if c.ParentVariableCollectionID == id {
			return true
		}
		c = r.collections[c.ParentVariableCollectionID]
	}
	return false
}

func modeName(c figma.VariableCollection, modeID string) string {
	for _, m := range c.Modes {
		if m.ModeID == modeID {
			return m.Name
		}
	}
	return ""
}

// ModeValue is the value of a variable in one mode of one collection.
type ModeValue struct {
	CollectionID string `json:"collectionId"`
	Collection   string `json:"collection"`
	ModeID       string `json:"modeId"`
	Mode         string `json:"mode"`
	Default      bool   `json:"default,omitempty"`
	Value        Value  `json:"value"`
	Error        string `json:"error,omitempty"`
}

// Label is the mode label used as a map key: the mode name for the
// variable's own collection and collection/mode for other collections.
func (m ModeValue) Label(ownCollectionID string) string {
	if m.CollectionID == ownCollectionID {
		return m.Mode
	}
	return m.Collection + "/" + m.Mode
}

// ResolveAll resolves a variable in every mode of its collection, in
// every mode of the collections extending it, and, when a mode aliases
// into another collection with several modes, in each of those modes.
func (r *Resolver) ResolveAll(id string) ([]ModeValue, error) {
	v, ok := r.lookup(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownVariable, id)
	}
	own, hasOwn := r.collections[v.VariableCollectionID]
	if !hasOwn {
		val, err := r.Value(v.ID, "")
		mv := ModeValue{Value: val}
		if err != nil {
			mv.Error = err.Error()
		}
		return []ModeValue{mv}, nil
	}
	var out []ModeValue
	seen := map[string]bool{}
	add := func(c figma.VariableCollection, m figma.VariableMode) {
		if seen[m.ModeID] {
			return
		}
		seen[m.ModeID] = true
		val, err := r.Value(v.ID, m.ModeID)
		mv := ModeValue{CollectionID: c.ID, Collection: c.Name, ModeID: m.ModeID, Mode: m.Name, Default: m.ModeID == c.DefaultModeID, Value: val}
		if err != nil {
			mv.Error = err.Error()
		}
		out = append(out, mv)
	}
	for _, m := range own.Modes {
		add(own, m)
	}
	for _, m := range own.Modes {
		raw, ok := v.ValuesByMode[m.ModeID]
		if !ok || raw.Alias == nil {
			continue
		}
		target, ok := r.lookup(raw.Alias.ID)
		if !ok || target.VariableCollectionID == own.ID {
			continue
		}
		tc, ok := r.collections[target.VariableCollectionID]
		if !ok || len(tc.Modes) < 2 {
			continue
		}
		for _, tm := range tc.Modes {
			if hasModeNamed(own, tm.Name) {
				continue
			}
			add(tc, tm)
		}
	}
	for _, c := range r.sortedCollections() {
		if c.ID != own.ID && r.extends(c, own.ID) {
			for _, m := range c.Modes {
				add(c, m)
			}
		}
	}
	return out, nil
}

func hasModeNamed(c figma.VariableCollection, name string) bool {
	for _, m := range c.Modes {
		if m.Name == name {
			return true
		}
	}
	return false
}

func (r *Resolver) sortedCollections() []figma.VariableCollection {
	out := make([]figma.VariableCollection, 0, len(r.collections))
	for _, c := range r.collections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
