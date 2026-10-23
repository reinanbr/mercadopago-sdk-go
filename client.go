package mercadopagopix

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Mercado Pago API endpoint.
const DefaultBaseURL = "https://api.mercadopago.com"

const maxResponseSize = 10 << 20

// Client talks to the Mercado Pago payments API. It is safe for concurrent use.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	userAgent  string
	headers    http.Header
	retry      RetryPolicy
}

// RetryPolicy controls how transient failures are retried. Network errors
// and the statuses in RetryOn are retried; every attempt of one call reuses
// the same idempotency key, so retried POSTs never charge twice.
type RetryPolicy struct {
	// MaxRetries is the number of extra attempts (0 disables retries).
	MaxRetries int
	// InitialDelay is the wait before the first retry; it doubles each time.
	InitialDelay time.Duration
	// MaxDelay caps any single wait, including one asked for by Retry-After.
	MaxDelay time.Duration
	// Jitter randomizes each wait between 50% and 100% of its value, so
	// many clients do not retry in lockstep.
	Jitter bool
	// RetryOn lists the HTTP statuses to retry.
	RetryOn []int
	// OnRetry, if set, is called before each retry (for logs or metrics).
	OnRetry func(RetryInfo)
}

// RetryInfo describes a retry about to happen.
type RetryInfo struct {
	Method  string
	Path    string
	Attempt int           // 1 for the first retry
	Delay   time.Duration // wait before this retry
	Err     error         // error of the previous attempt
}

// DefaultRetryPolicy mirrors the official SDKs: up to 3 retries on 429 and
// 5xx gateway errors, exponential backoff from 500ms capped at 30s, with
// jitter. Retry-After is honored when the API sends it.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       true,
		RetryOn:      []int{429, 500, 502, 503, 504},
	}
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the underlying HTTP client (default: 60s timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithTimeout sets the timeout of each HTTP attempt.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		h := *c.httpClient
		h.Timeout = d
		c.httpClient = &h
	}
}

// WithBaseURL overrides the API endpoint, mainly for tests.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithRetryPolicy replaces the retry policy.
func WithRetryPolicy(p RetryPolicy) Option { return func(c *Client) { c.retry = p } }

// WithRetries sets the number of retries and the initial backoff, keeping
// the rest of the policy. Use WithRetries(0, 0) to disable retries.
func WithRetries(n int, wait time.Duration) Option {
	return func(c *Client) { c.retry.MaxRetries, c.retry.InitialDelay = n, wait }
}

// WithHeader adds a header sent on every request.
func WithHeader(key, value string) Option { return func(c *Client) { c.headers.Set(key, value) } }

// WithIntegratorID sends x-integrator-id, identifying the developer or agency
// certified by Mercado Pago that built the integration.
func WithIntegratorID(id string) Option { return WithHeader("X-Integrator-Id", id) }

// WithPlatformID sends x-platform-id, identifying the platform (e-commerce
// software) the integration runs on.
func WithPlatformID(id string) Option { return WithHeader("X-Platform-Id", id) }

// WithCorporationID sends x-corporation-id, grouping accounts of a corporation.
func WithCorporationID(id string) Option { return WithHeader("X-Corporation-Id", id) }

// New creates a Client with the given access token.
func New(accessToken string, opts ...Option) (*Client, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, ErrMissingToken
	}
	c := &Client{
		token:      accessToken,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		userAgent:  "mercadopago-sdk-go/1.1.1",
		headers:    http.Header{},
		retry:      DefaultRetryPolicy(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// envTokenVars are checked in order by NewFromEnv.
var envTokenVars = []string{"MERCADOPAGO_ACCESS_TOKEN", "ACCESS_TOKEN", "TOKEN"}

// NewFromEnv creates a Client reading the token from the first non-empty of
// MERCADOPAGO_ACCESS_TOKEN, ACCESS_TOKEN or TOKEN (the variable used by the
// Python library). Combine with LoadEnvFile to read a .env file.
func NewFromEnv(opts ...Option) (*Client, error) {
	for _, k := range envTokenVars {
		if token := os.Getenv(k); token != "" {
			return New(token, opts...)
		}
	}
	return nil, ErrMissingToken
}

// TestMode reports whether the client uses sandbox credentials ("TEST-").
func (c *Client) TestMode() bool { return strings.HasPrefix(c.token, "TEST-") }

// CallOption customizes a single API call.
type CallOption func(*callConfig)

type callConfig struct {
	token          string
	idempotencyKey string
	headers        http.Header
}

// WithAccessToken uses another token for this call only, e.g. the OAuth
// token of a seller in a marketplace.
func WithAccessToken(token string) CallOption {
	return func(cc *callConfig) { cc.token = strings.TrimSpace(token) }
}

// WithIdempotencyKey sets the X-Idempotency-Key of this call (1–64 chars).
// Reusing a key returns the result of the first request instead of
// performing the operation again.
func WithIdempotencyKey(key string) CallOption {
	return func(cc *callConfig) { cc.idempotencyKey = key }
}

// WithCallHeader adds a header to this call only.
func WithCallHeader(key, value string) CallOption {
	return func(cc *callConfig) {
		if cc.headers == nil {
			cc.headers = http.Header{}
		}
		cc.headers.Set(key, value)
	}
}

// request is one logical API call (possibly several HTTP attempts).
type request struct {
	method string
	path   string
	query  url.Values
	body   any
	out    any
	// idempotent requests get an idempotency key generated if none is set.
	idempotent bool
	opts       []CallOption
}

// Do performs a raw authenticated JSON call to any Mercado Pago endpoint,
// with the client's retries, headers and error handling. body may be nil;
// out may be nil to discard the response. POST requests always carry an
// idempotency key (WithIdempotencyKey or a random one).
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any, opts ...CallOption) error {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.do(ctx, request{method: method, path: path, query: query, body: body, out: out,
		idempotent: method == http.MethodPost, opts: opts})
}

func (c *Client) do(ctx context.Context, r request) error {
	cc := callConfig{token: c.token}
	for _, opt := range r.opts {
		opt(&cc)
	}
	if cc.token == "" {
		return ErrMissingToken
	}
	if cc.idempotencyKey == "" && r.idempotent {
		cc.idempotencyKey = newIdempotencyKey()
	}
	if n := len(cc.idempotencyKey); n > 64 {
		return fmt.Errorf("%w: idempotency key must have 1 to 64 characters, got %d", ErrInvalidRequest, n)
	}

	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return fmt.Errorf("mercadopagopix: encoding request: %w", err)
		}
	}
	target := c.baseURL + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}

	var lastErr error
	var retryAfter time.Duration
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt, retryAfter)
			if c.retry.OnRetry != nil {
				c.retry.OnRetry(RetryInfo{Method: r.method, Path: r.path, Attempt: attempt, Delay: delay, Err: lastErr})
			}
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
		var retry bool
		retry, retryAfter, lastErr = c.attempt(ctx, r, target, payload, &cc)
		if lastErr == nil || !retry || attempt >= c.retry.MaxRetries {
			return lastErr
		}
	}
}

func (c *Client) attempt(ctx context.Context, r request, target string, payload []byte, cc *callConfig) (retry bool, retryAfter time.Duration, err error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return false, 0, fmt.Errorf("mercadopagopix: building request: %w", err)
	}
	for k, v := range c.headers {
		req.Header[k] = v
	}
	for k, v := range cc.headers {
		req.Header[k] = v
	}
	req.Header.Set("Authorization", "Bearer "+cc.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cc.idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", cc.idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, ctx.Err()
		}
		return true, 0, fmt.Errorf("mercadopagopix: %s %s: %w", r.method, r.path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return true, 0, fmt.Errorf("mercadopagopix: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := parseAPIError(resp.StatusCode, data)
		apiErr.RequestID = resp.Header.Get("X-Request-Id")
		return slices.Contains(c.retry.RetryOn, resp.StatusCode), parseRetryAfter(resp.Header.Get("Retry-After")), apiErr
	}
	if r.out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, r.out); err != nil {
			return false, 0, fmt.Errorf("mercadopagopix: decoding response (HTTP %d): %w", resp.StatusCode, err)
		}
	}
	return false, 0, nil
}

// backoff returns the wait before the given retry (1-based).
func (c *Client) backoff(attempt int, retryAfter time.Duration) time.Duration {
	d := c.retry.InitialDelay << (attempt - 1)
	if d < 0 || (c.retry.InitialDelay > 0 && d < c.retry.InitialDelay) { // overflow
		d = c.retry.MaxDelay
	}
	if c.retry.Jitter && d > 0 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(d/2)+1))
		d = d/2 + time.Duration(n.Int64())
	}
	if retryAfter > d {
		d = retryAfter
	}
	if c.retry.MaxDelay > 0 && d > c.retry.MaxDelay {
		d = c.retry.MaxDelay
	}
	return d
}

// parseRetryAfter reads a Retry-After header (seconds or HTTP date).
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

func pathID(id int64) string { return strconv.FormatInt(id, 10) }

// newIdempotencyKey returns a random UUIDv4.
func newIdempotencyKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
