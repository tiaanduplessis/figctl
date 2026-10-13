package figma

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the public Figma API host.
const DefaultBaseURL = "https://api.figma.com"

// BaseURLEnv overrides the API base URL for every profile. It is meant for
// tests and proxies.
const BaseURLEnv = "FIGMA_API_BASE_URL"

const (
	tokenHeader   = "X-Figma-Token" //nolint:gosec // header name, not a credential
	maxURLBytes   = 8 * 1024
	defaultTries  = 3
	backoffBase   = 500 * time.Millisecond
	backoffMax    = 8 * time.Second
	maxRetryAfter = 60 * time.Second
)

// Logger receives verbose HTTP traces. *output.Logger satisfies it.
type Logger interface {
	Debugf(format string, args ...any)
}

// Sleeper waits for d or until ctx is done. Tests inject one that does not
// wait.
type Sleeper func(ctx context.Context, d time.Duration) error

// Options configures a Client.
type Options struct {
	// Token is the personal access token. Empty means unauthenticated
	// requests, which Figma rejects.
	Token string
	// BaseURL defaults to DefaultBaseURL. FIGMA_API_BASE_URL wins when set
	// and the caller passes an empty value.
	BaseURL string
	// Version is embedded in the User-Agent as figctl/<version>.
	Version string
	// Timeout bounds one logical request including retries. Zero means
	// no timeout beyond the context deadline.
	Timeout time.Duration
	// Logger receives request and response traces with the token
	// redacted. Nil disables tracing.
	Logger Logger
	// Transport overrides the HTTP transport, for tests.
	Transport http.RoundTripper
	// Sleep overrides the retry sleeper, for tests.
	Sleep Sleeper
	// MaxAttempts caps retries on 429 and 5xx. Defaults to 3.
	MaxAttempts int
	// ProfileNames returns other configured profile names for NOT_FOUND
	// and FORBIDDEN hints. Nil means no hint.
	ProfileNames func() []string
}

// RateLimit is the last rate limit information seen on any response.
type RateLimit struct {
	// Seen reports whether any rate limit header was observed.
	Seen bool `json:"seen"`
	// Status is the HTTP status of the response that carried the headers.
	Status int `json:"status,omitempty"`
	// PlanTier is X-Figma-Plan-Tier.
	PlanTier string `json:"planTier,omitempty"`
	// Type is X-Figma-Rate-Limit-Type (low or high).
	Type string `json:"type,omitempty"`
	// UpgradeLink is X-Figma-Upgrade-Link.
	UpgradeLink string `json:"upgradeLink,omitempty"`
	// RetryAfterSeconds is Retry-After on a 429.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// At is when the headers were observed.
	At time.Time `json:"at"`
}

// Client calls the Figma REST API.
type Client struct {
	token        string
	baseURL      string
	userAgent    string
	timeout      time.Duration
	log          Logger
	http         *http.Client
	sleep        Sleeper
	maxAttempts  int
	profileNames func() []string

	mu        sync.Mutex
	rateLimit RateLimit
}

// New creates a client.
func New(opts Options) *Client {
	c := &Client{
		token:        opts.Token,
		baseURL:      strings.TrimRight(opts.BaseURL, "/"),
		userAgent:    "figctl/" + firstNonEmpty(opts.Version, "dev"),
		timeout:      opts.Timeout,
		log:          opts.Logger,
		sleep:        opts.Sleep,
		maxAttempts:  opts.MaxAttempts,
		profileNames: opts.ProfileNames,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.sleep == nil {
		c.sleep = realSleep
	}
	if c.maxAttempts <= 0 {
		c.maxAttempts = defaultTries
	}
	transport := opts.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	c.http = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			original := via[0]
			if original.Header.Get(tokenHeader) != "" &&
				(req.URL.Scheme != original.URL.Scheme || !strings.EqualFold(req.URL.Host, original.URL.Host)) {
				return errors.New("refusing to send the Figma token to a different origin")
			}
			return nil
		},
	}
	return c
}

// BaseURL returns the API base URL in use.
func (c *Client) BaseURL() string { return c.baseURL }

// LastRateLimit returns the most recent rate limit headers observed.
func (c *Client) LastRateLimit() RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rateLimit
}

func realSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// request describes one API call.
type request struct {
	method string
	path   string
	query  url.Values
	body   any
	// scope is the token scope the endpoint needs, for AUTH_SCOPE hints.
	scope string
	// resource names what a 404 refers to, for example "file ABC".
	resource string
	// notFoundHint replaces the default 404 hint for endpoints where a 404
	// does not mean the resource is missing. Figma answers 404 on some
	// endpoints when the token or plan lacks access to that feature.
	notFoundHint string
	// plan marks endpoints that need an Enterprise plan (variables).
	plan bool
}

// do performs a request with retries and decodes the JSON body into out.
func (c *Client) do(ctx context.Context, req request, out any) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	var bodyBytes []byte
	if req.body != nil {
		var err error
		bodyBytes, err = json.Marshal(req.body)
		if err != nil {
			return internalErr(err, "encoding request body")
		}
	}
	target := c.baseURL + req.path
	if len(req.query) > 0 {
		target += "?" + req.query.Encode()
	}

	for attempt := 1; ; attempt++ {
		resp, body, err := c.once(ctx, req.method, target, bodyBytes)
		if err != nil {
			if attempt < c.maxAttempts && c.wait(ctx, attempt, 0) == nil {
				continue
			}
			return networkErr(err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || len(body) == 0 {
				return nil
			}
			if raw, ok := out.(*json.RawMessage); ok {
				*raw = body
				return nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return internalErr(err, "decoding Figma response for "+req.method+" "+req.path)
			}
			return nil
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if retryable && attempt < c.maxAttempts {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			if c.wait(ctx, attempt, retryAfter) == nil {
				continue
			}
		}
		return c.mapError(req, resp, body)
	}
}

// once performs a single HTTP exchange and captures rate limit headers.
func (c *Client) once(ctx context.Context, method, target string, body []byte) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("User-Agent", c.userAgent)
	httpReq.Header.Set("Accept", "application/json")
	if c.token != "" {
		httpReq.Header.Set(tokenHeader, c.token)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	c.trace(httpReq)
	started := time.Now()
	resp, err := c.http.Do(httpReq)
	if err != nil {
		c.debugf("%s %s failed after %s: %v", method, redactURL(target), time.Since(started).Round(time.Millisecond), err)
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	c.capture(resp)
	c.debugf("%d %s %s (%d bytes, %s)%s", resp.StatusCode, method, redactURL(target), len(data), time.Since(started).Round(time.Millisecond), rateLimitTrace(resp.Header))
	return resp, data, nil
}

// wait sleeps before a retry: Retry-After when given, otherwise exponential
// backoff with jitter. It fails when the wait would pass the deadline.
func (c *Client) wait(ctx context.Context, attempt int, retryAfter time.Duration) error {
	d := retryAfter
	if d <= 0 {
		d = backoffBase << (attempt - 1)
		if d > backoffMax {
			d = backoffMax
		}
		d += time.Duration(rand.Int64N(int64(d / 2))) //nolint:gosec // jitter, not security sensitive
	}
	if deadline, ok := ctx.Deadline(); ok && time.Now().Add(d).After(deadline) {
		return context.DeadlineExceeded
	}
	c.debugf("retrying attempt %d in %s", attempt+1, d.Round(time.Millisecond))
	return c.sleep(ctx, d)
}

func (c *Client) capture(resp *http.Response) {
	h := resp.Header
	rl := RateLimit{
		PlanTier:    h.Get("X-Figma-Plan-Tier"),
		Type:        h.Get("X-Figma-Rate-Limit-Type"),
		UpgradeLink: h.Get("X-Figma-Upgrade-Link"),
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		rl.RetryAfterSeconds = int(parseRetryAfter(h.Get("Retry-After"), time.Now()).Seconds())
	}
	if rl.PlanTier == "" && rl.Type == "" && rl.UpgradeLink == "" && rl.RetryAfterSeconds == 0 {
		return
	}
	rl.Seen = true
	rl.Status = resp.StatusCode
	rl.At = time.Now()
	c.mu.Lock()
	c.rateLimit = rl
	c.mu.Unlock()
}

func rateLimitTrace(h http.Header) string {
	var parts []string
	for _, name := range []string{"Retry-After", "X-Figma-Plan-Tier", "X-Figma-Rate-Limit-Type", "X-Figma-Upgrade-Link"} {
		if v := h.Get(name); v != "" {
			parts = append(parts, name+": "+v)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, ", ") + "]"
}

func (c *Client) trace(req *http.Request) {
	if c.log == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", req.Method, redactURL(req.URL.String()))
	for name, values := range req.Header {
		for _, v := range values {
			if strings.EqualFold(name, tokenHeader) {
				v = "[redacted]"
			}
			fmt.Fprintf(&b, "\n  %s: %s", name, v)
		}
	}
	c.debugf("%s", b.String())
}

func (c *Client) debugf(format string, args ...any) {
	if c.log != nil {
		c.log.Debugf(format, args...)
	}
}

// redactURL removes any token-like query parameters from a URL for logs.
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	q := u.Query()
	changed := false
	for key := range q {
		if strings.Contains(strings.ToLower(key), "token") {
			q.Set(key, "[redacted]")
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// parseRetryAfter reads a Retry-After header given in seconds or as an
// HTTP date. It caps the result at maxRetryAfter.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(value); err == nil {
		d = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		d = at.Sub(now)
	}
	if d < 0 {
		d = 0
	}
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

// Download fetches a pre-signed URL (rendered image or image fill) into w.
// No token is sent because the URL is not on the API host.
func (c *Client) Download(ctx context.Context, target string, w io.Writer) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return networkErr(err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	c.trace(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return networkErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.debugf("%d GET %s", resp.StatusCode, redactURL(target))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return networkErr(fmt.Errorf("download %s: HTTP %d", redactURL(target), resp.StatusCode))
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return networkErr(err)
	}
	return nil
}

func isContextErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
