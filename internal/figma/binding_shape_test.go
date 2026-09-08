package figma_test

import (
	"encoding/json"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

// TestBoundVariablesAcceptBothShapes pins the shape Figma actually sends.
// A bound variable arrives as a bare object on some properties and as a list
// on others, and a production design system binds typography with the list
// form. Typing these as a single alias made the whole node fail to decode,
// which silently reported every bound value as having no token at all: the
// opposite of the truth, and the one thing an agent must not be told, since
// it hardcodes a value that the design system governs.
func TestBoundVariablesAcceptBothShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"a list, as text styles send", `{"boundVariables":{"fontSize":[{"id":"VariableID:31:15","type":"VARIABLE_ALIAS"}]}}`},
		{"a bare object, as other properties send", `{"boundVariables":{"fontSize":{"id":"VariableID:31:15","type":"VARIABLE_ALIAS"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts figma.TypeStyle
			if err := json.Unmarshal([]byte(tt.raw), &ts); err != nil {
				t.Fatalf("decoding failed: %v", err)
			}
			a, ok := ts.BoundVariables["fontSize"].First()
			if !ok || a.ID != "VariableID:31:15" {
				t.Fatalf("binding lost: %+v", ts.BoundVariables)
			}
		})
	}
}

// TestPaintAndEffectBindingsAcceptBothShapes covers the other places Figma
// attaches a variable.
func TestPaintAndEffectBindingsAcceptBothShapes(t *testing.T) {
	var node figma.Node
	raw := `{"id":"1:1","type":"FRAME",
	  "fills":[{"type":"SOLID","boundVariables":{"color":[{"id":"VariableID:1:1","type":"VARIABLE_ALIAS"}]}}],
	  "effects":[{"type":"DROP_SHADOW","boundVariables":{"color":{"id":"VariableID:2:2","type":"VARIABLE_ALIAS"}}}]}`
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if a, ok := node.Fills[0].BoundVariables["color"].First(); !ok || a.ID != "VariableID:1:1" {
		t.Errorf("fill binding lost: %+v", node.Fills[0].BoundVariables)
	}
	if a, ok := node.Effects[0].BoundVariables["color"].First(); !ok || a.ID != "VariableID:2:2" {
		t.Errorf("effect binding lost: %+v", node.Effects[0].BoundVariables)
	}
}
