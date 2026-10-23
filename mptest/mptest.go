// Package mptest is an in-memory fake of the Mercado Pago payments API for
// testing code built on mercadopagopix, end to end and without network.
//
// Unlike the sandbox, it lets a test pay a PIX (Approve), let it expire
// (Expire) and receive the signed webhook that follows:
//
//	srv := mptest.NewServer()
//	defer srv.Close()
//	client := srv.Client()
//
//	p, _ := client.CreatePix(ctx, mercadopagopix.CreatePixRequest{
//		Amount:          mercadopagopix.Reais(10),
//		Payer:           mercadopagopix.Payer{Email: "cliente@example.com"},
//		NotificationURL: shop.URL + "/webhook",
//	})
//	err := srv.Approve(p.ID) // POSTs a webhook signed with srv.WebhookSecret
//
// Supported: create, get, cancel, refunds, search by external_reference,
// status and payment_method_id, /users/me and idempotency keys.
package mptest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	mp "github.com/reinanbr/mercadopago-sdk-go"
)

// Token is the access token used by Server.Client.
const Token = "TEST-mptest"

// timeLayout is the timestamp format of the Mercado Pago API.
const timeLayout = "2006-01-02T15:04:05.000-07:00"

// qrCodePNG is a 1×1 PNG returned as the QR code image.
const qrCodePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAAAAAA6fptVAAAACklEQVR4nGNgAAAAAgABc3UBGAAAAABJRU5ErkJggg=="

// Server is a fake Mercado Pago API. Its methods are safe for concurrent use.
type Server struct {
	*httptest.Server

	// WebhookSecret signs the webhooks sent by Approve, Expire, etc.
	WebhookSecret string
	// WebhookClient delivers webhooks (default: http.DefaultClient).
	WebhookClient *http.Client
	// Now returns the current time (default: time.Now). Set it to make
	// pending payments expire without waiting.
	Now func() time.Time

	mu       sync.Mutex
	nextID   int64
	payments map[int64]*record
	idem     map[string]any // idempotency key → *record or *refund
	failures []int
	requests []Request
}

// Request is a call received by the server, for assertions.
type Request struct {
	Method, Path   string
	IdempotencyKey string
}

type record struct {
	id                int64
	status            mp.Status
	statusDetail      string
	description       string
	externalReference string
	notificationURL   string
	amount, refunded  mp.Amount
	payer             json.RawMessage
	metadata          map[string]any
	additionalInfo    json.RawMessage
	created, updated  time.Time
	approved, expires time.Time
	refunds           []*refund
}

type refund struct {
	ID          int64     `json:"id"`
	PaymentID   int64     `json:"payment_id"`
	Amount      mp.Amount `json:"amount"`
	Status      string    `json:"status"`
	DateCreated string    `json:"date_created"`
}

// NewServer starts a fake API. Close it when done.
func NewServer() *Server {
	s := &Server{
		WebhookSecret: "mptest-secret",
		nextID:        1000000001,
		payments:      map[int64]*record{},
		idem:          map[string]any{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Client returns a client for this server, without retry delays.
func (s *Server) Client(opts ...mp.Option) *mp.Client {
	opts = append([]mp.Option{mp.WithBaseURL(s.URL), mp.WithRetries(0, 0)}, opts...)
	c, err := mp.New(Token, opts...)
	if err != nil {
		panic(err)
	}
	return c
}

// Validator returns a webhook validator for WebhookSecret.
func (s *Server) Validator() *mp.WebhookValidator {
	return &mp.WebhookValidator{Secret: s.WebhookSecret}
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// FailNext makes the next len(statuses) API calls fail with the given HTTP
// statuses, in order, to exercise retries and error handling.
func (s *Server) FailNext(statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, statuses...)
}

// Requests returns the calls received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

// Payment returns the current state of a payment.
func (s *Server) Payment(id int64) (*mp.Payment, bool) {
	s.mu.Lock()
	r, ok := s.payments[id]
	var b []byte
	if ok {
		s.expire(r)
		b, _ = json.Marshal(r.json())
	}
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	var p mp.Payment
	if err := json.Unmarshal(b, &p); err != nil {
		panic(err)
	}
	return &p, true
}

// Approve marks a pending payment as paid, as if the payer had paid the
// PIX, and sends the webhook.
func (s *Server) Approve(id int64) error {
	return s.transition(id, func(r *record) error {
		if r.status != mp.StatusPending {
			return fmt.Errorf("mptest: payment %d is %s, not pending", id, r.status)
		}
		r.status, r.statusDetail, r.approved = mp.StatusApproved, "accredited", s.now()
		return nil
	})
}

// Expire makes a pending payment expire (status cancelled, detail
// "expired", as Mercado Pago does) and sends the webhook.
func (s *Server) Expire(id int64) error {
	return s.transition(id, func(r *record) error {
		if r.status != mp.StatusPending {
			return fmt.Errorf("mptest: payment %d is %s, not pending", id, r.status)
		}
		r.status, r.statusDetail = mp.StatusCancelled, "expired"
		return nil
	})
}

// ChargeBack marks an approved payment as charged back and sends the webhook.
func (s *Server) ChargeBack(id int64) error {
	return s.transition(id, func(r *record) error {
		if r.status != mp.StatusApproved {
			return fmt.Errorf("mptest: payment %d is %s, not approved", id, r.status)
		}
		r.status, r.statusDetail = mp.StatusChargedBack, "settled"
		return nil
	})
}

func (s *Server) transition(id int64, change func(*record) error) error {
	s.mu.Lock()
	r, ok := s.payments[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("mptest: payment %d not found", id)
	}
	if err := change(r); err != nil {
		s.mu.Unlock()
		return err
	}
	r.updated = s.now()
	s.mu.Unlock()
	return s.SendWebhook(id)
}

// SendWebhook delivers a signed "payment.updated" notification to the
// payment's notification_url, as Mercado Pago does. It does nothing if the
// payment has no notification URL, and fails unless the receiver answers 2xx.
func (s *Server) SendWebhook(id int64) error {
	s.mu.Lock()
	r, ok := s.payments[id]
	var target string
	if ok {
		target = r.notificationURL
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("mptest: payment %d not found", id)
	}
	if target == "" {
		return nil
	}

	dataID := strconv.FormatInt(id, 10)
	body, _ := json.Marshal(map[string]any{
		"id":           s.now().UnixNano(),
		"type":         "payment",
		"action":       "payment.updated",
		"api_version":  "v1",
		"live_mode":    false,
		"date_created": s.now().Format(timeLayout),
		"data":         map[string]string{"id": dataID},
	})
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	req, err := http.NewRequest(http.MethodPost, target+sep+"data.id="+dataID+"&type=payment", bytes.NewReader(body))
	if err != nil {
		return err
	}
	requestID := fmt.Sprintf("mptest-%d", s.now().UnixNano())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Signature", mp.SignWebhook(s.WebhookSecret, dataID, requestID, s.now()))
	hc := s.WebhookClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("mptest: delivering webhook: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("mptest: webhook receiver answered %d", resp.StatusCode)
	}
	return nil
}

// expire applies the expiration of a pending payment. s.mu must be held.
func (s *Server) expire(r *record) {
	if r.status == mp.StatusPending && s.now().After(r.expires) {
		r.status, r.statusDetail, r.updated = mp.StatusCancelled, "expired", r.expires
	}
}

func (r *record) json() map[string]any {
	ts := func(t time.Time) any {
		if t.IsZero() {
			return nil
		}
		return t.Format(timeLayout)
	}
	m := map[string]any{
		"id":                          r.id,
		"status":                      r.status,
		"status_detail":               r.statusDetail,
		"description":                 r.description,
		"external_reference":          r.externalReference,
		"notification_url":            r.notificationURL,
		"payment_method_id":           "pix",
		"payment_type_id":             "bank_transfer",
		"currency_id":                 "BRL",
		"transaction_amount":          r.amount,
		"transaction_amount_refunded": r.refunded,
		"live_mode":                   false,
		"payer":                       r.payer,
		"metadata":                    r.metadata,
		"date_created":                ts(r.created),
		"date_approved":               ts(r.approved),
		"date_last_updated":           ts(r.updated),
		"date_of_expiration":          ts(r.expires),
		"point_of_interaction": map[string]any{
			"type": "PIX",
			"transaction_data": map[string]any{
				"qr_code":        fmt.Sprintf("00020126580014br.gov.bcb.pix0136mptest-%d520400005303986540%d6304", r.id, r.amount.Cents()),
				"qr_code_base64": qrCodePNG,
				"ticket_url":     fmt.Sprintf("https://www.mercadopago.com.br/sandbox/payments/%d/ticket", r.id),
			},
		},
	}
	if r.additionalInfo != nil {
		m["additional_info"] = r.additionalInfo
	}
	if r.status == mp.StatusApproved || r.status == mp.StatusRefunded {
		fee := r.amount * 99 / 10000 // 0.99%
		m["transaction_details"] = map[string]any{"total_paid_amount": r.amount, "net_received_amount": r.amount - fee}
		m["money_release_date"] = ts(r.approved)
	}
	return m
}

func (s *Server) serve(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{Method: req.Method, Path: req.URL.Path, IdempotencyKey: req.Header.Get("X-Idempotency-Key")})

	if len(s.failures) > 0 {
		status := s.failures[0]
		s.failures = s.failures[1:]
		apiError(w, status, "simulated_failure", "failure injected by mptest", "")
		return
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") || len(req.Header.Get("Authorization")) == len("Bearer ") {
		apiError(w, http.StatusUnauthorized, "unauthorized", "invalid access token", "")
		return
	}

	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/users/me":
		writeJSON(w, http.StatusOK, map[string]any{"id": 123456789, "nickname": "MPTEST", "email": "seller@example.com", "site_id": "MLB", "tags": []string{"normal", "test_user"}})
	case req.Method == http.MethodPost && req.URL.Path == "/v1/payments":
		s.create(w, req)
	case req.Method == http.MethodGet && req.URL.Path == "/v1/payments/search":
		s.search(w, req)
	case len(parts) >= 3 && parts[0] == "v1" && parts[1] == "payments":
		id, err := strconv.ParseInt(parts[2], 10, 64)
		r, ok := s.payments[id]
		if err != nil || !ok {
			apiError(w, http.StatusNotFound, "not_found", "Payment not found", "2000")
			return
		}
		s.expire(r)
		switch {
		case len(parts) == 3 && req.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, r.json())
		case len(parts) == 3 && req.Method == http.MethodPut:
			s.update(w, req, r)
		case len(parts) == 4 && parts[3] == "refunds" && req.Method == http.MethodPost:
			s.refund(w, req, r)
		case len(parts) == 4 && parts[3] == "refunds" && req.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, r.refunds)
		case len(parts) == 5 && parts[3] == "refunds" && req.Method == http.MethodGet:
			for _, rf := range r.refunds {
				if strconv.FormatInt(rf.ID, 10) == parts[4] {
					writeJSON(w, http.StatusOK, rf)
					return
				}
			}
			apiError(w, http.StatusNotFound, "not_found", "Refund not found", "")
		default:
			apiError(w, http.StatusNotFound, "not_found", "resource not found", "")
		}
	default:
		apiError(w, http.StatusNotFound, "not_found", "resource not found", "")
	}
}

func (s *Server) create(w http.ResponseWriter, req *http.Request) {
	key := req.Header.Get("X-Idempotency-Key")
	if prev, ok := s.idem[key].(*record); ok && key != "" {
		writeJSON(w, http.StatusOK, prev.json())
		return
	}
	var body struct {
		TransactionAmount mp.Amount       `json:"transaction_amount"`
		PaymentMethodID   string          `json:"payment_method_id"`
		Description       string          `json:"description"`
		Payer             json.RawMessage `json:"payer"`
		NotificationURL   string          `json:"notification_url"`
		ExternalReference string          `json:"external_reference"`
		DateOfExpiration  string          `json:"date_of_expiration"`
		Metadata          map[string]any  `json:"metadata"`
		AdditionalInfo    json.RawMessage `json:"additional_info"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	var payer struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body.Payer, &payer)
	switch {
	case body.PaymentMethodID != "pix":
		apiError(w, http.StatusBadRequest, "bad_request", "mptest only supports payment_method_id pix", "")
		return
	case body.TransactionAmount <= 0:
		apiError(w, http.StatusBadRequest, "bad_request", "Invalid transaction_amount", "4037")
		return
	case payer.Email == "":
		apiError(w, http.StatusBadRequest, "bad_request", "payer.email must not be null", "4050")
		return
	}
	now := s.now()
	expires := now.Add(24 * time.Hour)
	if body.DateOfExpiration != "" {
		t, err := time.Parse(timeLayout, body.DateOfExpiration)
		if err != nil {
			apiError(w, http.StatusBadRequest, "bad_request", "invalid date_of_expiration", "")
			return
		}
		expires = t
	}
	r := &record{
		id:                s.nextID,
		status:            mp.StatusPending,
		statusDetail:      "pending_waiting_transfer",
		description:       body.Description,
		externalReference: body.ExternalReference,
		notificationURL:   body.NotificationURL,
		amount:            body.TransactionAmount,
		payer:             body.Payer,
		metadata:          body.Metadata,
		additionalInfo:    body.AdditionalInfo,
		created:           now,
		updated:           now,
		expires:           expires,
	}
	if r.metadata == nil {
		r.metadata = map[string]any{}
	}
	s.nextID++
	s.payments[r.id] = r
	if key != "" {
		s.idem[key] = r
	}
	writeJSON(w, http.StatusCreated, r.json())
}

func (s *Server) update(w http.ResponseWriter, req *http.Request, r *record) {
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	if len(body) != 1 || body["status"] != string(mp.StatusCancelled) {
		apiError(w, http.StatusBadRequest, "bad_request", "the attribute cannot be modified", "4243")
		return
	}
	if r.status != mp.StatusPending {
		apiError(w, http.StatusBadRequest, "bad_request", "Invalid status for cancel", "")
		return
	}
	r.status, r.statusDetail, r.updated = mp.StatusCancelled, "by_collector", s.now()
	writeJSON(w, http.StatusOK, r.json())
}

func (s *Server) refund(w http.ResponseWriter, req *http.Request, r *record) {
	key := req.Header.Get("X-Idempotency-Key")
	if prev, ok := s.idem[key].(*refund); ok && key != "" {
		writeJSON(w, http.StatusCreated, prev)
		return
	}
	var body struct {
		Amount mp.Amount `json:"amount"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	if r.status != mp.StatusApproved {
		apiError(w, http.StatusBadRequest, "bad_request", "Payment not approved", "")
		return
	}
	left := r.amount - r.refunded
	if body.Amount == 0 {
		body.Amount = left
	}
	if body.Amount < 0 || body.Amount > left {
		apiError(w, http.StatusBadRequest, "bad_request", "Invalid refund amount", "")
		return
	}
	rf := &refund{ID: s.nextID, PaymentID: r.id, Amount: body.Amount, Status: "approved", DateCreated: s.now().Format(timeLayout)}
	s.nextID++
	r.refunds = append(r.refunds, rf)
	r.refunded += body.Amount
	if r.refunded == r.amount {
		r.status, r.statusDetail = mp.StatusRefunded, "refunded"
	}
	r.updated = s.now()
	if key != "" {
		s.idem[key] = rf
	}
	writeJSON(w, http.StatusCreated, rf)
}

func (s *Server) search(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	var matches []*record
	for _, r := range s.payments {
		s.expire(r)
		if (q.Get("external_reference") == "" || q.Get("external_reference") == r.externalReference) &&
			(q.Get("status") == "" || q.Get("status") == string(r.status)) &&
			(q.Get("payment_method_id") == "" || q.Get("payment_method_id") == "pix") {
			matches = append(matches, r)
		}
	}
	asc := q.Get("criteria") == "asc"
	slices.SortFunc(matches, func(a, b *record) int {
		if asc {
			return int(a.id - b.id)
		}
		return int(b.id - a.id)
	})
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 30
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	results := []map[string]any{}
	for i := offset; i < len(matches) && i < offset+limit; i++ {
		results = append(results, matches[i].json())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"paging":  map[string]int{"total": len(matches), "limit": limit, "offset": offset},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", fmt.Sprintf("mptest-%d", time.Now().UnixNano()))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, code, message, cause string) {
	body := map[string]any{"status": status, "error": code, "message": message, "cause": []any{}}
	if cause != "" {
		body["cause"] = []map[string]any{{"code": cause, "description": message}}
	}
	writeJSON(w, status, body)
}
