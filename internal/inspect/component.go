package inspect

import (
	"fmt"
	"strings"

	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/model"
	"github.com/tiaanduplessis/figctl/internal/resolve"
)

// propertyName strips the #id suffix Figma appends to non-variant
// property names.
func propertyName(key string) string {
	if i := strings.LastIndex(key, "#"); i > 0 {
		return key[:i]
	}
	return key
}

func (in *inspector) component(n *figma.Node) *model.Component {
	switch n.Type {
	case "INSTANCE":
		return in.instance(n)
	case "COMPONENT", "COMPONENT_SET":
		return in.definition(n)
	}
	return nil
}

func (in *inspector) instance(n *figma.Node) *model.Component {
	c := &model.Component{IsInstance: true, MainComponentID: n.ComponentID, ExposedInstances: n.ExposedInstances}
	if in.file != nil {
		if comp, ok := in.file.Components[n.ComponentID]; ok {
			c.Key = comp.Key
			c.MainComponentName = comp.Name
			c.Description = comp.Description
			c.Remote = comp.Remote
			c.DocumentationLinks = links(comp.DocumentationLinks)
			if set, ok := in.file.ComponentSets[comp.ComponentSetID]; ok {
				c.SetID = comp.ComponentSetID
				c.SetName = set.Name
				if c.Description == "" {
					c.Description = set.Description
				}
				if len(c.DocumentationLinks) == 0 {
					c.DocumentationLinks = links(set.DocumentationLinks)
				}
			}
		}
	}
	if c.MainComponentName == "" && in.file != nil && in.file.Document != nil {
		if main := in.file.Document.Find(n.ComponentID); main != nil {
			c.MainComponentName = main.Name
		}
	}
	for key, p := range n.ComponentProperties {
		if p.Type == "VARIANT" {
			if c.VariantProperties == nil {
				c.VariantProperties = map[string]string{}
			}
			c.VariantProperties[key] = stringValue(p.Value)
			continue
		}
		if c.Properties == nil {
			c.Properties = map[string]model.Property{}
		}
		prop := model.Property{Type: p.Type, Value: p.Value, Key: key}
		for _, alias := range p.BoundVariables {
			prop.Token = in.r.Token(alias.ID)
			break
		}
		c.Properties[propertyName(key)] = prop
	}
	for _, o := range n.Overrides {
		c.Overrides = append(c.Overrides, model.Override{NodeID: o.ID, Fields: o.OverriddenFields})
	}
	return c
}

func (in *inspector) definition(n *figma.Node) *model.Component {
	c := &model.Component{IsComponent: n.Type == "COMPONENT", IsComponentSet: n.Type == "COMPONENT_SET"}
	if in.file != nil {
		if comp, ok := in.file.Components[n.ID]; ok {
			c.Key = comp.Key
			c.Description = comp.Description
			c.Remote = comp.Remote
			c.DocumentationLinks = links(comp.DocumentationLinks)
			if set, ok := in.file.ComponentSets[comp.ComponentSetID]; ok {
				c.SetID = comp.ComponentSetID
				c.SetName = set.Name
			}
		}
		if set, ok := in.file.ComponentSets[n.ID]; ok {
			c.Key = set.Key
			c.Description = set.Description
			c.Remote = set.Remote
			c.DocumentationLinks = links(set.DocumentationLinks)
		}
	}
	if n.Type == "COMPONENT" {
		c.VariantProperties = variantProperties(n.Name)
	}
	for key, d := range n.ComponentPropertyDefinitions {
		if c.PropertyDefinitions == nil {
			c.PropertyDefinitions = map[string]model.PropertyDefinition{}
		}
		c.PropertyDefinitions[propertyName(key)] = model.PropertyDefinition{Type: d.Type, DefaultValue: d.DefaultValue, VariantOptions: d.VariantOptions, Key: key}
	}
	return c
}

// variantProperties parses "Prop=Value, Other=Value" component names.
func variantProperties(name string) map[string]string {
	if !strings.Contains(name, "=") {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(name, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return out
}

func links(l []figma.DocumentationLink) []string {
	var out []string
	for _, d := range l {
		if d.URI != "" {
			out = append(out, d.URI)
		}
	}
	return out
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case float64:
		return resolve.Num(x, 4)
	}
	return fmt.Sprint(v)
}
