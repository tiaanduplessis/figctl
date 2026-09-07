package figma_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// TestByKeyEndpointsNameLibraryAssetsScope guards the scope named in an
// AUTH_SCOPE hint for the by-key lookups. Figma requires library_assets:read
// for these, not library_content:read, so naming the wrong one sends the user
// away to mint another token that still fails.
func TestByKeyEndpointsNameLibraryAssetsScope(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name   string
		method string
		path   string
		call   func(*figma.Client) error
	}{
		{"component", http.MethodGet, "/v1/components/k", func(c *figma.Client) error {
			_, err := c.GetComponent(ctx, "k")
			return err
		}},
		{"component set", http.MethodGet, "/v1/component_sets/k", func(c *figma.Client) error {
			_, err := c.GetComponentSet(ctx, "k")
			return err
		}},
		{"style", http.MethodGet, "/v1/styles/k", func(c *figma.Client) error {
			_, err := c.GetStyle(ctx, "k")
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := figmatest.NewServer(t)
			s.Respond(tt.method, tt.path, figmatest.Response{
				Status: http.StatusForbidden,
				Body:   map[string]any{"status": 403, "err": "Invalid scope(s): missing scope"},
			})
			c, _, _ := newClient(t, s, nil)
			e := code(t, tt.call(c))
			if e.Code != figctl.CodeAuthScope {
				t.Fatalf("code = %s, want %s", e.Code, figctl.CodeAuthScope)
			}
			if !strings.Contains(e.Hint, figma.ScopeLibraryAssets) {
				t.Fatalf("hint should name %s, got %q", figma.ScopeLibraryAssets, e.Hint)
			}
		})
	}
}

// TestDevResourcesNotFoundExplainsAccess guards the 404 message for dev
// resources. Figma answers 404 there when the token lacks the scope or the
// plan does not expose Dev Mode, so asserting the file is missing sends the
// user to debug the wrong thing.
func TestDevResourcesNotFoundExplainsAccess(t *testing.T) {
	s := figmatest.NewServer(t)
	s.Respond(http.MethodGet, "/v1/files/"+figmatest.FileKey+"/dev_resources", figmatest.Response{
		Status: http.StatusNotFound,
		Body:   map[string]any{"status": 404, "err": "Not found"},
	})
	c, _, _ := newClient(t, s, nil)
	_, err := c.GetDevResources(context.Background(), figmatest.FileKey, nil)
	e := code(t, err)
	if e.Code != figctl.CodeNotFound {
		t.Fatalf("code = %s, want %s", e.Code, figctl.CodeNotFound)
	}
	if !strings.Contains(e.Hint, figma.ScopeFileDevResources) {
		t.Errorf("hint should name the scope, got %q", e.Hint)
	}
	if !strings.Contains(e.Hint, "plan") {
		t.Errorf("hint should mention the plan, got %q", e.Hint)
	}
	// The message must scope the 404 to dev resources rather than reading as
	// a claim that the file itself is missing.
	if !strings.HasPrefix(e.Message, "dev resources for file ") {
		t.Errorf("message should scope the failure to dev resources, got %q", e.Message)
	}
}
