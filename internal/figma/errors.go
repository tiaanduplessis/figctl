package figma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// errorBody covers both Figma error shapes: {status, err} and
// {error: true, status, message}.
type errorBody struct {
	Status  int    `json:"status"`
	Err     string `json:"err"`
	Message string `json:"message"`
}

func (b errorBody) text() string {
	return firstNonEmpty(b.Err, b.Message)
}

func parseErrorBody(body []byte) errorBody {
	var b errorBody
	_ = json.Unmarshal(body, &b)
	if b.text() == "" {
		if s := strings.TrimSpace(string(body)); s != "" && len(s) <= 200 && !strings.HasPrefix(s, "<") {
			b.Message = s
		}
	}
	return b
}

func internalErr(err error, what string) *figctl.Error {
	return figctl.Wrap(figctl.CodeInternal, err, what+": "+err.Error())
}

func networkErr(err error) *figctl.Error {
	msg := "network error talking to Figma: " + err.Error()
	hint := "Check the connection and retry; raise --timeout for large files."
	if isContextErr(err) {
		msg = "request to Figma timed out or was cancelled: " + err.Error()
	}
	return figctl.Wrap(figctl.CodeNetwork, err, msg).WithHint("%s", hint)
}

// mapError converts a non-2xx response into a figctl error.
func (c *Client) mapError(req request, resp *http.Response, body []byte) *figctl.Error {
	b := parseErrorBody(body)
	msg := b.text()
	lower := strings.ToLower(msg)
	status := resp.StatusCode
	var e *figctl.Error

	switch {
	case status == http.StatusBadRequest && req.plan && strings.Contains(lower, "enterprise"):
		e = planRequired(msg)
	case status == http.StatusBadRequest:
		e = figctl.Newf(figctl.CodeUsage, "Figma rejected the request: %s", firstNonEmpty(msg, "bad request")).
			WithHint("Check the node ids, query flags, and file key.")
	case status == http.StatusUnauthorized, status == http.StatusForbidden && strings.Contains(lower, "token"):
		e = figctl.Newf(figctl.CodeAuthInvalid, "Figma rejected the token: %s", firstNonEmpty(msg, "unauthorized")).
			WithHint("The token may have expired (personal access tokens expire within 90 days) or belong to the wrong profile. Run figctl auth login --profile <name> with a new token, or pick another profile with --profile.")
	case status == http.StatusForbidden && req.plan && (strings.Contains(lower, "enterprise") || strings.Contains(lower, "plan")):
		e = planRequired(msg)
	case status == http.StatusForbidden && strings.Contains(lower, "scope"):
		// Figma's own message lists the scopes the token does have under the
		// heading "Invalid scope(s)", which reads as though those scopes are
		// the problem. Lead with the scope that is actually missing and keep
		// Figma's wording in the details for debugging.
		if req.scope != "" {
			e = figctl.Newf(figctl.CodeAuthScope, "the token is missing the %s scope", req.scope)
		} else {
			e = figctl.Newf(figctl.CodeAuthScope, "the token is missing a scope: %s", msg)
		}
		if EnterpriseOnlyScope(req.scope) {
			e = e.WithHint("The %s scope is offered only to members of an Enterprise organization, so it does not appear on the token screen on other plans. This endpoint is unavailable rather than misconfigured.", req.scope)
		} else {
			e = e.WithHint("This endpoint needs the %s scope. Create a token with it (see figctl auth scopes) and run figctl auth login again.", req.scope)
		}
		e.Details = map[string]any{"scope": req.scope, "figmaMessage": msg}
	case status == http.StatusForbidden:
		e = figctl.Newf(figctl.CodeForbidden, "access denied by Figma: %s", firstNonEmpty(msg, "forbidden")).
			WithHint("The active profile cannot access %s.%s", req.resource, c.profileHint())
	case status == http.StatusNotFound:
		e = figctl.Newf(figctl.CodeNotFound, "%s not found", req.resource)
		if req.notFoundHint != "" {
			e = e.WithHint("%s", req.notFoundHint)
		} else {
			e = e.WithHint("Check the key or id and that the active profile has access.%s", c.profileHint())
		}
	case status == http.StatusTooManyRequests:
		e = c.rateLimitErr(resp, msg)
	case status >= 500:
		e = figctl.Newf(figctl.CodeNetwork, "Figma returned HTTP %d: %s", status, firstNonEmpty(msg, http.StatusText(status))).
			WithHint("Figma is having trouble; retry in a moment.")
	default:
		e = figctl.Newf(figctl.CodeInternal, "unexpected HTTP %d from Figma: %s", status, firstNonEmpty(msg, http.StatusText(status)))
	}
	e.HTTPStatus = status
	return e
}

func planRequired(msg string) *figctl.Error {
	return figctl.Newf(figctl.CodePlanRequired, "the variables API needs an Enterprise plan with a full seat: %s", firstNonEmpty(msg, "forbidden")).
		WithHint("Fall back to styles: figctl styles list <ref>, and figctl tokens export <ref> exports styles only on this plan.")
}

func (c *Client) rateLimitErr(resp *http.Response, msg string) *figctl.Error {
	h := resp.Header
	retryAfter := parseRetryAfter(h.Get("Retry-After"), time.Now())
	tier := h.Get("X-Figma-Plan-Tier")
	kind := h.Get("X-Figma-Rate-Limit-Type")
	text := "Figma rate limit hit"
	if tier != "" || kind != "" {
		text += fmt.Sprintf(" (plan: %s, seat limit: %s)", firstNonEmpty(tier, "unknown"), firstNonEmpty(kind, "unknown"))
	}
	if msg != "" {
		text += ": " + msg
	}
	hint := "Reuse cached data instead of re-fetching; the file cache serves file tree, find, inspect and get without new requests."
	if retryAfter > 0 {
		hint = fmt.Sprintf("Retry after %ds. ", int(retryAfter.Seconds())) + hint
	}
	e := figctl.New(figctl.CodeRateLimited, text).WithHint("%s", hint)
	e.RetryAfterSeconds = int(retryAfter.Seconds())
	details := map[string]any{}
	if tier != "" {
		details["planTier"] = tier
	}
	if kind != "" {
		details["rateLimitType"] = kind
	}
	if link := h.Get("X-Figma-Upgrade-Link"); link != "" {
		details["upgradeLink"] = link
	}
	if len(details) > 0 {
		e.Details = details
	}
	return e
}

func (c *Client) profileHint() string {
	if c.profileNames == nil {
		return ""
	}
	names := c.profileNames()
	if len(names) == 0 {
		return ""
	}
	return " Other configured profiles: " + strings.Join(names, ", ") + "; try --profile <name>."
}

// IsRequestTooLarge reports whether Figma refused a request because the
// response would be too big. Figma answers 400 with "Request too large" for
// whole-document fetches of large files, which means the caller must ask for
// less rather than retry the same request.
func IsRequestTooLarge(err error) bool {
	e := figctl.From(err)
	return e != nil && e.HTTPStatus == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(e.Message), "request too large")
}
