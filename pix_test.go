package mercadopagopix

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const paymentFixture = `{
	"id": 82298079966,
	"status": "pending",
	"status_detail": "pending_waiting_transfer",
	"description": "Pagamento de teste",
	"external_reference": null,
	"payment_method_id": "pix",
	"currency_id": "BRL",
	"transaction_amount": 2.5,
	"transaction_amount_refunded": 0,
	"notification_url": "https://example.com/notify",
	"live_mode": true,
	"metadata": {},
	"date_created": "2024-07-07T19:14:37.000-04:00",
	"date_approved": null,
	"date_last_updated": "2024-07-07T19:14:39.000-04:00",
	"date_of_expiration": "2099-07-08T19:14:36.000-04:00",
	"charges_execution_info": null,
	"point_of_interaction": {
		"transaction_data": {
			"qr_code": "00020126580014br.gov.bcb.pix",
			"qr_code_base64": "aGVsbG8=",
			"ticket_url": "https://www.mercadopago.com.br/payments/82298079966/ticket"
		}
	}
}`

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New("TEST-token", WithBaseURL(srv.URL), WithRetries(2, time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewRequiresToken(t *testing.T) {
	if _, err := New("  "); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("got %v, want ErrMissingToken", err)
	}
}

func TestCreatePix(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer TEST-token" {
			t.Errorf("authorization = %q", got)
		}
		if r.Header.Get("X-Idempotency-Key") == "" {
			t.Error("missing idempotency key")
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"transaction_amount":2.50`) {
			t.Errorf("amount not encoded as decimal: %s", raw)
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["payment_method_id"] != "pix" {
			t.Errorf("payment_method_id = %v", body["payment_method_id"])
		}
		exp, _ := body["date_of_expiration"].(string)
		if _, err := time.Parse(expirationLayout, exp); err != nil {
			t.Errorf("bad expiration %q: %v", exp, err)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, paymentFixture)
	})

	p, err := c.CreatePix(context.Background(), CreatePixRequest{
		Amount:      Reais(2.50),
		Description: "Pagamento de teste",
		Payer:       Payer{Email: "a@b.com", FirstName: "Ana"},
		ExpiresIn:   10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 82298079966 || p.Amount != Cents(250) || p.Status != StatusPending {
		t.Errorf("unexpected payment: %+v", p)
	}
	if p.QRCode == "" || p.TicketURL == "" || p.ExecutionID != "" {
		t.Errorf("transaction data not mapped: %+v", p)
	}
	if !p.DateApproved.IsZero() || p.DateCreated.IsZero() {
		t.Errorf("dates not mapped: created=%v approved=%v", p.DateCreated, p.DateApproved)
	}
	if png, err := p.QRCodePNG(); err != nil || string(png) != "hello" {
		t.Errorf("QRCodePNG = %q, %v", png, err)
	}
	if p.TimeToExpiration() <= 0 || p.IsExpired() {
		t.Error("payment should not be expired")
	}
}

func TestCreatePixValidation(t *testing.T) {
	c, _ := New("x")
	cases := []CreatePixRequest{
		{Amount: 0, Payer: Payer{Email: "a@b.com"}},
		{Amount: 100},
		{Amount: 100, Payer: Payer{Email: "x@"}},
		{Amount: 100, Payer: Payer{Email: "Nome <a@b.com>"}},
		{Amount: 100, Payer: Payer{Email: "a@b.com"}, NotificationURL: "not a url"},
		{Amount: 100, Payer: Payer{Email: "a@b.com"}, ExpiresIn: -time.Second},
		{Amount: 100, Payer: Payer{Email: "a@b.com"}, ExpiresAt: time.Now().Add(-time.Hour)},
	}
	for i, req := range cases {
		if _, err := c.CreatePix(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("case %d: got %v, want ErrInvalidRequest", i, err)
		}
	}
}

func TestAPIError(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"Payment not found","error":"not_found","status":404,"cause":[{"code":2000,"description":"Payment not found"}]}`)
	})
	_, err := c.GetPayment(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 || apiErr.Code != "not_found" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apiErr.Causes) != 1 || apiErr.Causes[0].Code != "2000" {
		t.Errorf("causes = %+v", apiErr.Causes)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is(err, ErrNotFound) = false")
	}
	if calls.Load() != 1 {
		t.Errorf("4xx should not be retried, got %d calls", calls.Load())
	}
}

func TestRetryOn5xxKeepsIdempotencyKey(t *testing.T) {
	var calls atomic.Int32
	var keys []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("X-Idempotency-Key"))
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, paymentFixture)
	})
	_, err := c.CreatePix(context.Background(), CreatePixRequest{Amount: 100, Payer: Payer{Email: "a@b.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || keys[0] != keys[1] || keys[1] != keys[2] {
		t.Errorf("calls=%d keys=%v", calls.Load(), keys)
	}
}

func TestCancelAndRefund(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/payments/7":
			if string(raw) != `{"status":"cancelled"}` {
				t.Errorf("cancel body = %s", raw)
			}
			io.WriteString(w, `{"id":7,"status":"cancelled"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payments/7/refunds":
			if string(raw) != `{"amount":1.00}` {
				t.Errorf("refund body = %s", raw)
			}
			io.WriteString(w, `{"id":99,"payment_id":7,"amount":1,"status":"approved","date_created":"2024-07-07T19:14:37.000-04:00"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	p, err := c.CancelPayment(context.Background(), 7)
	if err != nil || p.Status != StatusCancelled {
		t.Fatalf("cancel: %v %v", p, err)
	}
	ref, err := c.Refund(context.Background(), 7, Reais(1))
	if err != nil || ref.ID != 99 || ref.Amount != 100 || ref.DateCreated.IsZero() {
		t.Fatalf("refund: %+v %v", ref, err)
	}
}

func TestSearchByExternalReference(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/search" || r.URL.Query().Get("external_reference") != "order-1" {
			t.Errorf("unexpected %s", r.URL)
		}
		io.WriteString(w, `{"results":[`+paymentFixture+`],"paging":{"total":1}}`)
	})
	res, err := c.SearchByExternalReference(context.Background(), "order-1")
	if err != nil || len(res) != 1 || res[0].ID != 82298079966 {
		t.Fatalf("got %v, %v", res, err)
	}
}

func TestWaitForPayment(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			io.WriteString(w, `{"id":1,"status":"pending"}`)
			return
		}
		io.WriteString(w, `{"id":1,"status":"approved"}`)
	})
	p, err := c.WaitForPayment(context.Background(), 1, time.Millisecond)
	if err != nil || !p.IsPaid() || calls.Load() != 3 {
		t.Fatalf("p=%v err=%v calls=%d", p, err, calls.Load())
	}
}

func TestWaitForPaymentTimeoutKeepsLastPayment(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			time.Sleep(200 * time.Millisecond) // outlives the context
		}
		io.WriteString(w, `{"id":1,"status":"pending"}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p, err := c.WaitForPayment(ctx, 1, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if p == nil || p.Status != StatusPending {
		t.Fatalf("last payment = %v, want pending", p)
	}
}

func TestWaitForPaymentExpired(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":1,"status":"pending","date_of_expiration":"2020-01-01T00:00:00.000-03:00"}`)
	})
	if _, err := c.WaitForPayment(context.Background(), 1, time.Millisecond); !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want ErrExpired", err)
	}
}

func TestAmount(t *testing.T) {
	if got := Reais(0.1 + 0.2); got != 30 {
		t.Errorf("Reais(0.3) = %d", got)
	}
	if got := Cents(123456789).String(); got != "R$ 1.234.567,89" {
		t.Errorf("String = %q", got)
	}
	if got := Cents(-5).String(); got != "-R$ 0,05" {
		t.Errorf("String = %q", got)
	}
	b, _ := json.Marshal(Cents(-105))
	if string(b) != "-1.05" {
		t.Errorf("Marshal = %s", b)
	}
	var a Amount
	for in, want := range map[string]Amount{`19.99`: 1999, `"3.5"`: 350, `null`: 0, `1`: 100} {
		if err := json.Unmarshal([]byte(in), &a); err != nil || a != want {
			t.Errorf("Unmarshal(%s) = %d, %v", in, a, err)
		}
	}
}
