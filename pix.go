package mercadopagopix

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"time"
)

// expirationLayout is the timestamp format Mercado Pago expects.
const expirationLayout = "2006-01-02T15:04:05.000-07:00"

// CreatePixRequest describes a PIX charge.
type CreatePixRequest struct {
	Amount      Amount // required, > 0
	Description string
	Payer       Payer // Payer.Email is required

	// NotificationURL receives webhooks about this payment (http/https).
	NotificationURL string
	// ExternalReference is your own ID (order number, etc.), searchable later.
	ExternalReference string
	// ExpiresAt sets an absolute expiration. It takes precedence over ExpiresIn.
	ExpiresAt time.Time
	// ExpiresIn sets the expiration relative to now. If both are zero,
	// Mercado Pago's default (24h) applies.
	ExpiresIn time.Duration
	Metadata  map[string]any

	// IdempotencyKey prevents duplicate charges when a request is retried.
	// A random key is generated if empty; set it yourself (e.g. to your
	// order ID) to make retries safe across process restarts.
	IdempotencyKey string
}

func (r *CreatePixRequest) validate() error {
	if r.Amount <= 0 {
		return fmt.Errorf("%w: amount must be greater than zero", ErrInvalidRequest)
	}
	if r.Payer.Email == "" {
		return fmt.Errorf("%w: payer email is required", ErrInvalidRequest)
	}
	if a, err := mail.ParseAddress(r.Payer.Email); err != nil || a.Address != r.Payer.Email {
		return fmt.Errorf("%w: invalid payer email %q", ErrInvalidRequest, r.Payer.Email)
	}
	if r.ExpiresIn < 0 {
		return fmt.Errorf("%w: ExpiresIn must not be negative", ErrInvalidRequest)
	}
	if !r.ExpiresAt.IsZero() && r.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("%w: ExpiresAt is in the past", ErrInvalidRequest)
	}
	if r.NotificationURL != "" {
		u, err := url.Parse(r.NotificationURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: invalid notification URL %q", ErrInvalidRequest, r.NotificationURL)
		}
	}
	return nil
}

type createPaymentBody struct {
	TransactionAmount Amount         `json:"transaction_amount"`
	PaymentMethodID   string         `json:"payment_method_id"`
	Description       string         `json:"description,omitempty"`
	Payer             Payer          `json:"payer"`
	NotificationURL   string         `json:"notification_url,omitempty"`
	ExternalReference string         `json:"external_reference,omitempty"`
	DateOfExpiration  string         `json:"date_of_expiration,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// CreatePix creates a PIX payment and returns it with its QR code.
// The idempotency key is, in order: WithIdempotencyKey, req.IdempotencyKey,
// or a random one.
func (c *Client) CreatePix(ctx context.Context, req CreatePixRequest, opts ...CallOption) (*Payment, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	body := createPaymentBody{
		TransactionAmount: req.Amount,
		PaymentMethodID:   "pix",
		Description:       req.Description,
		Payer:             req.Payer,
		NotificationURL:   req.NotificationURL,
		ExternalReference: req.ExternalReference,
		Metadata:          req.Metadata,
	}
	switch {
	case !req.ExpiresAt.IsZero():
		body.DateOfExpiration = req.ExpiresAt.Format(expirationLayout)
	case req.ExpiresIn > 0:
		body.DateOfExpiration = time.Now().Add(req.ExpiresIn).Format(expirationLayout)
	}
	if req.IdempotencyKey != "" {
		opts = append([]CallOption{WithIdempotencyKey(req.IdempotencyKey)}, opts...)
	}
	var p Payment
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/payments", body: body, out: &p, idempotent: true, opts: opts})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPayment fetches a payment by ID.
func (c *Client) GetPayment(ctx context.Context, id int64, opts ...CallOption) (*Payment, error) {
	var p Payment
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/payments/" + pathID(id), out: &p, opts: opts}); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePayment sends a raw update (PUT /v1/payments/{id}). For PIX,
// Mercado Pago only accepts {"status": "cancelled"} (see CancelPayment);
// metadata, external_reference and date_of_expiration are refused.
func (c *Client) UpdatePayment(ctx context.Context, id int64, fields map[string]any, opts ...CallOption) (*Payment, error) {
	var p Payment
	if err := c.do(ctx, request{method: http.MethodPut, path: "/v1/payments/" + pathID(id), body: fields, out: &p, opts: opts}); err != nil {
		return nil, err
	}
	return &p, nil
}

// CancelPayment cancels a pending payment so its QR code can no longer be paid.
func (c *Client) CancelPayment(ctx context.Context, id int64, opts ...CallOption) (*Payment, error) {
	return c.UpdatePayment(ctx, id, map[string]any{"status": StatusCancelled}, opts...)
}

// Refund refunds an approved payment. Pass amount 0 for a full refund.
// Use WithIdempotencyKey to make a retried refund safe across restarts.
func (c *Client) Refund(ctx context.Context, id int64, amount Amount, opts ...CallOption) (*Refund, error) {
	if amount < 0 {
		return nil, fmt.Errorf("%w: refund amount must not be negative", ErrInvalidRequest)
	}
	body := map[string]any{}
	if amount > 0 {
		body["amount"] = amount
	}
	var r Refund
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/payments/" + pathID(id) + "/refunds",
		body: body, out: &r, idempotent: true, opts: opts})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRefunds returns all refunds of a payment.
func (c *Client) ListRefunds(ctx context.Context, paymentID int64, opts ...CallOption) ([]Refund, error) {
	var rs []Refund
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/payments/" + pathID(paymentID) + "/refunds", out: &rs, opts: opts})
	return rs, err
}

// GetRefund fetches one refund of a payment.
func (c *Client) GetRefund(ctx context.Context, paymentID, refundID int64, opts ...CallOption) (*Refund, error) {
	var r Refund
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/payments/" + pathID(paymentID) + "/refunds/" + pathID(refundID), out: &r, opts: opts})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SearchByExternalReference returns payments with the given external
// reference, newest first (first page, up to 100).
func (c *Client) SearchByExternalReference(ctx context.Context, ref string, opts ...CallOption) ([]Payment, error) {
	res, err := c.SearchPayments(ctx, SearchParams{ExternalReference: ref, Limit: 100}, opts...)
	if err != nil {
		return nil, err
	}
	return res.Results, nil
}

// WaitForPayment polls a payment every interval until its status is final
// (approved, rejected, cancelled...). It returns ErrExpired, together with
// the last known payment, if the PIX expires while still pending. Use ctx
// to bound the total wait; on timeout or error the last payment fetched (if
// any) is returned with the error. Prefer webhooks in production.
func (c *Client) WaitForPayment(ctx context.Context, id int64, interval time.Duration, opts ...CallOption) (*Payment, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var last *Payment
	for {
		p, err := c.GetPayment(ctx, id, opts...)
		if err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return last, err
		}
		last = p
		if p.Status.IsFinal() {
			return p, nil
		}
		if p.IsExpired() {
			return p, ErrExpired
		}
		select {
		case <-ctx.Done():
			return p, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Account is the Mercado Pago account that owns the access token.
type Account struct {
	ID       int64    `json:"id"`
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	SiteID   string   `json:"site_id"`
	Tags     []string `json:"tags"`
}

// IsTestUser reports whether the account is a Mercado Pago test user.
func (a *Account) IsTestUser() bool {
	for _, t := range a.Tags {
		if t == "test_user" {
			return true
		}
	}
	return false
}

// Me returns the account that owns the access token. It is a cheap way to
// check that the credentials work.
func (c *Client) Me(ctx context.Context, opts ...CallOption) (*Account, error) {
	var a Account
	if err := c.do(ctx, request{method: http.MethodGet, path: "/users/me", out: &a, opts: opts}); err != nil {
		return nil, err
	}
	return &a, nil
}
