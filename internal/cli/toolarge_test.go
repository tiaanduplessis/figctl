package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figma/figmatest"
)

// TestWholeFileTooLargeFallsBack covers the large-file path. Figma answers 400
// "Request too large" for the whole document of a big production file, and the
// attempt can also outrun the timeout. Fetching the whole document is only an
// optimisation, so either outcome must degrade to targeted requests instead of
// failing the command.
func TestWholeFileTooLargeFallsBack(t *testing.T) {
	for _, tt := range []struct {
		name string
		resp figmatest.Response
	}{
		{"figma refuses the whole document", figmatest.Response{
			Status: http.StatusBadRequest,
			Body:   map[string]any{"status": 400, "err": "Request too large. If applicable, filter by query params."},
		}},
		{"the whole document attempt keeps failing", figmatest.Response{
			Status: http.StatusInternalServerError,
			Body:   map[string]any{"status": 500, "err": "Internal server error"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := setup(t)
			// The whole-document fetch is the first hit on this path; the
			// queue then drains so depth-limited requests still succeed.
			// Queued once per retry attempt, so the whole-document fetch
			// exhausts its retries and the queue then drains, letting the
			// depth-limited fallback through.
			api.Enqueue(http.MethodGet, fileKeyPath(""), tt.resp, tt.resp, tt.resp)

			r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2", "--depth", "1")
			ok(t, r)
			if !strings.Contains(r.stdout, "2:2") {
				t.Fatalf("inspect should still return the node:\n%s", r.stdout)
			}
			if !strings.Contains(r.stderr, "targeted requests") {
				t.Fatalf("the fallback should be reported on stderr:\n%s", r.stderr)
			}
		})
	}
}

// TestWholeFileErrorsThatMustSurface keeps the fallback narrow. A smaller
// request would not survive a bad token, a missing file, or a rate limit, so
// those must reach the user instead of being retried as targeted requests.
func TestWholeFileErrorsThatMustSurface(t *testing.T) {
	for _, tt := range []struct {
		name string
		resp figmatest.Response
		want string
	}{
		{"missing file", figmatest.Response{
			Status: http.StatusNotFound,
			Body:   map[string]any{"status": 404, "err": "Not found"},
		}, "NOT_FOUND"},
		{"rate limited", figmatest.Response{
			Status: http.StatusTooManyRequests,
			Body:   map[string]any{"status": 429, "err": "Rate limited"},
			Header: map[string]string{"Retry-After": "0"},
		}, "RATE_LIMITED"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := setup(t)
			api.Respond(http.MethodGet, fileKeyPath(""), tt.resp)
			r := execute(t, "", "node", "inspect", figmatest.FileKey, "--node", "2:2")
			if r.code == 0 {
				t.Fatalf("expected a failure, got:\n%s", r.stdout)
			}
			if !strings.Contains(r.stdout, tt.want) {
				t.Fatalf("expected %s in the error envelope:\n%s", tt.want, r.stdout)
			}
		})
	}
}
