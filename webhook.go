package mercadopagopix

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxWebhookSize = 1 << 20

// Notification is a webhook sent by Mercado Pago.
type Notification struct {
	ID          flexString `json:"id"`
	Type        string     `json:"type"`   // e.g. "payment"
	Action      string     `json:"action"` // e.g. "payment.updated"
	APIVersion  string     `json:"api_version"`
	LiveMode    bool       `json:"live_mode"`
	UserID      flexString `json:"user_id"`
	DateCreated string     `json:"date_created"`
	Data        struct {
		ID flexString `json:"id"`
	} `json:"data"`

	// RequestID is the x-request-id header of the delivery.
	RequestID string `json:"-"`
}

// DataID returns the ID of the resource the notification refers to.
func (n *Notification) DataID() string { return string(n.Data.ID) }

// PaymentID returns the payment ID for "payment" notifications.
func (n *Notification) PaymentID() (int64, error) {
	if n.Type != "payment" {
		return 0, fmt.Errorf("mercadopagopix: notification type %q is not a payment", n.Type)
	}
	return strconv.ParseInt(n.DataID(), 10, 64)
}

// SignatureFailure says why a webhook signature was rejected.
type SignatureFailure string

const (
	SignatureMissingHeader   SignatureFailure = "missing_signature_header"
	SignatureMalformedHeader SignatureFailure = "malformed_signature_header"
	SignatureMissingTS       SignatureFailure = "missing_timestamp"
	SignatureMissingHash     SignatureFailure = "missing_hash"
	SignatureMismatch        SignatureFailure = "signature_mismatch"
	SignatureOutOfTolerance  SignatureFailure = "timestamp_out_of_tolerance"
)

// SignatureError is returned when a webhook signature is rejected. It
// matches ErrInvalidSignature with errors.Is. Log Reason and RequestID to
// correlate with the Mercado Pago notifications panel; do not echo them in
// the HTTP response.
type SignatureError struct {
	Reason    SignatureFailure
	RequestID string
	Timestamp string
}

func (e *SignatureError) Error() string {
	return "mercadopagopix: invalid webhook signature: " + string(e.Reason)
}

func (e *SignatureError) Is(target error) bool { return target == ErrInvalidSignature }

// WebhookValidator checks the x-signature header of Mercado Pago webhooks.
// Note: Mercado Pago does not sign QR-code (in-store) notifications.
type WebhookValidator struct {
	// Secret is the "assinatura secreta" of your application (Suas
	// integrações > Webhooks).
	Secret string
	// Tolerance, if > 0, rejects signatures whose timestamp differs from
	// now by more than this (replay protection).
	Tolerance time.Duration
	// Versions lists accepted signature versions in order of preference
	// (default: "v1").
	Versions []string
	// Now returns the current time (for tests). Default: time.Now.
	Now func() time.Time
}

var versionKeyRe = regexp.MustCompile(`^v\d+$`)

// Validate checks a signature. dataID is the "data.id" query parameter and
// xRequestID the x-request-id header; either may be empty.
func (v WebhookValidator) Validate(xSignature, xRequestID, dataID string) error {
	xSignature, xRequestID, dataID = strings.TrimSpace(xSignature), strings.TrimSpace(xRequestID), strings.TrimSpace(dataID)
	fail := func(r SignatureFailure, ts string) error {
		return &SignatureError{Reason: r, RequestID: xRequestID, Timestamp: ts}
	}
	if xSignature == "" {
		return fail(SignatureMissingHeader, "")
	}

	var ts string
	hashes := map[string]string{}
	for _, part := range strings.Split(xSignature, ",") {
		k, val, ok := strings.Cut(part, "=")
		k, val = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(val)
		if !ok || k == "" || val == "" {
			continue
		}
		if k == "ts" {
			ts = val
		} else if versionKeyRe.MatchString(k) {
			hashes[k] = strings.ToLower(val)
		}
	}
	if ts == "" && len(hashes) == 0 {
		return fail(SignatureMalformedHeader, "")
	}
	if ts == "" {
		return fail(SignatureMissingTS, "")
	}
	tsNum, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || tsNum < 0 {
		return fail(SignatureMalformedHeader, ts)
	}

	versions := v.Versions
	if len(versions) == 0 {
		versions = []string{"v1"}
	}
	var received string
	for _, ver := range versions {
		if h, ok := hashes[ver]; ok {
			received = h
			break
		}
	}
	if received == "" {
		return fail(SignatureMissingHash, ts)
	}

	// The official SDKs sign data.id as received; the docs ask for
	// lowercase when it is alphanumeric. Accept either.
	ok := hmac.Equal([]byte(signManifest(v.Secret, dataID, xRequestID, ts)), []byte(received))
	if !ok && strings.ToLower(dataID) != dataID {
		ok = hmac.Equal([]byte(signManifest(v.Secret, strings.ToLower(dataID), xRequestID, ts)), []byte(received))
	}
	if !ok {
		return fail(SignatureMismatch, ts)
	}

	if v.Tolerance > 0 {
		now := time.Now
		if v.Now != nil {
			now = v.Now
		}
		sent := time.Unix(tsNum, 0)
		if tsNum > 1e12 { // milliseconds
			sent = time.UnixMilli(tsNum)
		}
		if d := now().Sub(sent); d > v.Tolerance || d < -v.Tolerance {
			return fail(SignatureOutOfTolerance, ts)
		}
	}
	return nil
}

func signManifest(secret, dataID, requestID, ts string) string {
	var m strings.Builder
	if dataID != "" {
		m.WriteString("id:" + dataID + ";")
	}
	if requestID != "" {
		m.WriteString("request-id:" + requestID + ";")
	}
	m.WriteString("ts:" + ts + ";")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(m.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignWebhook returns an x-signature header for a notification, as
// Mercado Pago computes it. It is meant for tests of webhook handlers.
func SignWebhook(secret, dataID, requestID string, ts time.Time) string {
	t := strconv.FormatInt(ts.Unix(), 10)
	return "ts=" + t + ",v1=" + signManifest(secret, dataID, requestID, t)
}

// VerifySignature checks the x-signature header of a webhook using the
// secret from your Mercado Pago application. dataID is the "data.id" query
// parameter. If tolerance > 0, signatures older than that are rejected.
// On failure the error is a *SignatureError.
func VerifySignature(secret, xSignature, xRequestID, dataID string, tolerance time.Duration) error {
	return WebhookValidator{Secret: secret, Tolerance: tolerance}.Validate(xSignature, xRequestID, dataID)
}

// ParseWebhook reads a webhook request. If secret is non-empty the
// x-signature header is verified and a *SignatureError is returned on
// mismatch. Legacy IPN calls (?topic=payment&id=123) are also accepted.
func ParseWebhook(r *http.Request, secret string) (*Notification, error) {
	var v *WebhookValidator
	if secret != "" {
		v = &WebhookValidator{Secret: secret}
	}
	return parseWebhook(r, v)
}

func parseWebhook(r *http.Request, v *WebhookValidator) (*Notification, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookSize))
	if err != nil {
		return nil, fmt.Errorf("mercadopagopix: reading webhook: %w", err)
	}
	var n Notification
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, fmt.Errorf("mercadopagopix: decoding webhook: %w", err)
		}
	}
	n.RequestID = r.Header.Get("X-Request-Id")
	q := r.URL.Query()
	if n.Type == "" {
		n.Type = q.Get("type")
		if n.Type == "" {
			n.Type = q.Get("topic")
		}
	}
	dataID := q.Get("data.id")
	if n.Data.ID == "" {
		n.Data.ID = flexString(dataID)
		if n.Data.ID == "" {
			n.Data.ID = flexString(q.Get("id"))
		}
	}
	if dataID == "" {
		dataID = n.DataID()
	}
	if v != nil {
		if err := v.Validate(r.Header.Get("X-Signature"), n.RequestID, dataID); err != nil {
			return nil, err
		}
	}
	return &n, nil
}

// WebhookConfig configures NewWebhookHandler.
type WebhookConfig struct {
	// Validator checks signatures. Nil disables validation (not
	// recommended in production).
	Validator *WebhookValidator
	// Handle is called with the current payment, fetched from the API —
	// the webhook body is never trusted. It may run more than once for the
	// same payment (Mercado Pago redelivers), so make it idempotent.
	Handle func(ctx context.Context, p *Payment) error
	// Async acknowledges the webhook (200) right after validating it and
	// runs Handle in a goroutine. Mercado Pago expects an answer within 22
	// seconds and otherwise redelivers every 15 minutes.
	Async bool
	// Timeout bounds the payment fetch plus Handle in async mode
	// (default 2 minutes).
	Timeout time.Duration
	// MaxConcurrent limits how many notifications are processed at once
	// in async mode (default 32). Beyond it the handler answers 503 and
	// Mercado Pago delivers the notification again later.
	MaxConcurrent int
	// OnError receives errors from async processing and rejected
	// signatures (for logging). Optional.
	OnError func(n *Notification, err error)
}

// WebhookReceiver is the http.Handler returned by NewWebhookHandler.
type WebhookReceiver struct {
	client *Client
	cfg    WebhookConfig

	mu     sync.Mutex
	active int           // async notifications being processed
	idle   chan struct{} // closed when active drops to 0
}

// NewWebhookHandler returns an http.Handler for payment webhooks.
//
// Synchronous mode answers 401 on a bad signature, 200 for non-payment
// events, 502 if the payment cannot be fetched and 500 if Handle fails, so
// Mercado Pago retries the delivery. In async mode, call Wait on shutdown
// so notifications already acknowledged are not lost.
func (c *Client) NewWebhookHandler(cfg WebhookConfig) *WebhookReceiver {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 32
	}
	return &WebhookReceiver{client: c, cfg: cfg}
}

// acquire reserves a slot for async processing, if one is free.
func (h *WebhookReceiver) acquire() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active >= h.cfg.MaxConcurrent {
		return false
	}
	if h.active == 0 {
		h.idle = make(chan struct{})
	}
	h.active++
	return true
}

func (h *WebhookReceiver) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active--; h.active == 0 {
		close(h.idle)
	}
}

func (h *WebhookReceiver) report(n *Notification, err error) {
	if h.cfg.OnError != nil {
		h.cfg.OnError(n, err)
	}
}

func (h *WebhookReceiver) process(ctx context.Context, id int64) (status int, err error) {
	p, err := h.client.GetPayment(ctx, id)
	if err != nil {
		return http.StatusBadGateway, err
	}
	if err := h.cfg.Handle(ctx, p); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func (h *WebhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := parseWebhook(r, h.cfg.Validator)
	var sigErr *SignatureError
	if errors.As(err, &sigErr) {
		h.report(&Notification{RequestID: sigErr.RequestID}, err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, err := n.PaymentID()
	if err != nil {
		w.WriteHeader(http.StatusOK) // not a payment event: acknowledge and ignore
		return
	}

	if h.cfg.Async {
		if !h.acquire() {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		go func() {
			defer h.release()
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), h.cfg.Timeout)
			defer cancel()
			if _, err := h.process(ctx, id); err != nil {
				h.report(n, err)
			}
		}()
		return
	}

	status, err := h.process(r.Context(), id)
	if err != nil {
		h.report(n, err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Wait blocks until the notifications being processed in async mode
// finish, or ctx is done. Call it after http.Server.Shutdown.
func (h *WebhookReceiver) Wait(ctx context.Context) error {
	h.mu.Lock()
	if h.active == 0 {
		h.mu.Unlock()
		return nil
	}
	idle := h.idle
	h.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WebhookHandler is a shortcut for NewWebhookHandler in synchronous mode,
// validating signatures with secret (if non-empty).
func (c *Client) WebhookHandler(secret string, fn func(ctx context.Context, p *Payment) error) *WebhookReceiver {
	cfg := WebhookConfig{Handle: fn}
	if secret != "" {
		cfg.Validator = &WebhookValidator{Secret: secret}
	}
	return c.NewWebhookHandler(cfg)
}
