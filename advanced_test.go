package mercadopagopix

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallOptionsAndHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.WriteString(w, paymentFixture)
	}))
	defer srv.Close()
	c, _ := New("TEST-default", WithBaseURL(srv.URL), WithIntegratorID("dev_123"),
		WithPlatformID("plat_9"), WithCorporationID("corp_1"), WithHeader("X-Custom", "a"))

	_, err := c.CreatePix(context.Background(),
		CreatePixRequest{Amount: 100, Payer: Payer{Email: "a@b.com"}, IdempotencyKey: "from-request"},
		WithAccessToken("APP_USR-seller"), WithIdempotencyKey("from-option"), WithCallHeader("X-Custom", "b"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Authorization":     "Bearer APP_USR-seller",
		"X-Idempotency-Key": "from-option",
		"X-Integrator-Id":   "dev_123",
		"X-Platform-Id":     "plat_9",
		"X-Corporation-Id":  "corp_1",
		"X-Custom":          "b",
	}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, got.Get(k), v)
		}
	}

	if _, err := c.GetPayment(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer TEST-default" || got.Get("X-Idempotency-Key") != "" {
		t.Errorf("GET headers leaked from previous call: %v", got)
	}
}

func TestIdempotencyKeyTooLong(t *testing.T) {
	c, _ := New("x")
	_, err := c.CreatePix(context.Background(), CreatePixRequest{Amount: 1, Payer: Payer{Email: "a@b.com"},
		IdempotencyKey: strings.Repeat("k", 65)})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestRetryPolicy(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			io.WriteString(w, `{"id":1,"status":"approved"}`)
		}
	}))
	defer srv.Close()

	var infos []RetryInfo
	c, _ := New("x", WithBaseURL(srv.URL), WithRetryPolicy(RetryPolicy{
		MaxRetries:   3,
		InitialDelay: time.Millisecond,
		MaxDelay:     50 * time.Millisecond, // caps Retry-After: 1s
		RetryOn:      []int{429, 503},
		OnRetry:      func(i RetryInfo) { infos = append(infos, i) },
	}))
	start := time.Now()
	p, err := c.GetPayment(context.Background(), 1)
	if err != nil || !p.IsPaid() {
		t.Fatalf("p=%v err=%v", p, err)
	}
	if len(infos) != 2 || infos[0].Delay != 50*time.Millisecond || infos[1].Attempt != 2 {
		t.Errorf("retries = %+v", infos)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("MaxDelay not applied: took %s", time.Since(start))
	}
	var apiErr *APIError
	if !errors.As(infos[0].Err, &apiErr) || apiErr.StatusCode != 429 {
		t.Errorf("first retry error = %v", infos[0].Err)
	}
}

func TestRetryOnlyListedStatuses(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-Request-Id", "req-abc")
		w.WriteHeader(http.StatusNotImplemented) // 501: not in RetryOn
	})
	_, err := c.GetPayment(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RequestID != "req-abc" || !strings.Contains(err.Error(), "req-abc") {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("501 retried %d times", calls.Load()-1)
	}
}

func TestBackoff(t *testing.T) {
	c, _ := New("x", WithRetryPolicy(RetryPolicy{InitialDelay: 100 * time.Millisecond, MaxDelay: time.Second, Jitter: true}))
	for attempt := 1; attempt <= 10; attempt++ {
		base := min(100*time.Millisecond<<(attempt-1), time.Second)
		d := c.backoff(attempt, 0)
		if d > time.Second || d < base/2 {
			t.Errorf("attempt %d: delay %s outside [%s, 1s]", attempt, d, base/2)
		}
	}
	if d := c.backoff(1, 700*time.Millisecond); d != 700*time.Millisecond {
		t.Errorf("Retry-After ignored: %s", d)
	}
	if d := parseRetryAfter(time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)); d < time.Second || d > 3*time.Second {
		t.Errorf("HTTP-date Retry-After = %s", d)
	}
}

func TestEmptyResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	var out map[string]any
	if err := c.Do(context.Background(), http.MethodDelete, "v1/anything", nil, nil, &out); err != nil {
		t.Fatalf("204 should not fail: %v", err)
	}
}

func TestDoRaw(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/identification_types" || r.URL.Query().Get("x") != "1" {
			t.Errorf("unexpected %s", r.URL)
		}
		io.WriteString(w, `[{"id":"CPF","name":"CPF"}]`)
	})
	var types []struct{ ID string }
	err := c.Do(context.Background(), http.MethodGet, "/v1/identification_types", url.Values{"x": {"1"}}, nil, &types)
	if err != nil || len(types) != 1 || types[0].ID != "CPF" {
		t.Fatalf("types=%v err=%v", types, err)
	}
}

func TestSearchParams(t *testing.T) {
	begin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	q := SearchParams{
		Status: StatusApproved, PaymentMethodID: "pix", Begin: begin, End: begin.Add(24 * time.Hour),
		Limit: 10, Offset: 20, Extra: url.Values{"collector.id": {"9"}},
	}.values()
	want := map[string]string{
		"status": "approved", "payment_method_id": "pix", "range": "date_created",
		"begin_date": "2026-01-01T00:00:00.000-03:00", "end_date": "2026-01-02T00:00:00.000-03:00",
		"sort": "date_created", "criteria": "desc", "limit": "10", "offset": "20", "collector.id": "9",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Has("external_reference") {
		t.Error("empty filter should be omitted")
	}
}

func TestAllPaymentsPaginates(t *testing.T) {
	const total = 7
	var offsets []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offsets = append(offsets, r.URL.Query().Get("offset"))
		var items []string
		for i := off; i < min(off+lim, total); i++ {
			items = append(items, fmt.Sprintf(`{"id":%d}`, i+1))
		}
		fmt.Fprintf(w, `{"results":[%s],"paging":{"total":%d,"limit":%d,"offset":%d}}`, strings.Join(items, ","), total, lim, off)
	})
	var ids []int64
	for p, err := range c.AllPayments(context.Background(), SearchParams{Limit: 3}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if len(ids) != total || ids[6] != 7 || strings.Join(offsets, ",") != "0,3,6" {
		t.Errorf("ids=%v offsets=%v", ids, offsets)
	}

	// Breaking early stops fetching.
	offsets = nil
	for range c.AllPayments(context.Background(), SearchParams{Limit: 3}) {
		break
	}
	if len(offsets) != 1 {
		t.Errorf("fetched %d pages after break", len(offsets))
	}
}

func TestAllPaymentsYieldsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	n := 0
	for p, err := range c.AllPayments(context.Background(), SearchParams{}) {
		n++
		if p != nil || !errors.Is(err, ErrUnauthorized) {
			t.Errorf("p=%v err=%v", p, err)
		}
	}
	if n != 1 {
		t.Errorf("yielded %d times", n)
	}
}

func TestRefundEndpoints(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/payments/5/refunds":
			io.WriteString(w, `[{"id":1,"payment_id":5,"amount":1.5},{"id":2,"payment_id":5,"amount":0.5}]`)
		case "/v1/payments/5/refunds/2":
			io.WriteString(w, `{"id":2,"payment_id":5,"amount":0.5,"status":"approved"}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	rs, err := c.ListRefunds(context.Background(), 5)
	if err != nil || len(rs) != 2 || rs[0].Amount+rs[1].Amount != Reais(2) {
		t.Fatalf("rs=%v err=%v", rs, err)
	}
	r, err := c.GetRefund(context.Background(), 5, 2)
	if err != nil || r.ID != 2 || r.Status != "approved" {
		t.Fatalf("r=%v err=%v", r, err)
	}
}

func TestUpdatePayment(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || string(body) != `{"metadata":{"order":"42"}}` {
			t.Errorf("%s %s", r.Method, body)
		}
		io.WriteString(w, `{"id":3,"status":"pending","metadata":{"order":"42"}}`)
	})
	p, err := c.UpdatePayment(context.Background(), 3, map[string]any{"metadata": map[string]string{"order": "42"}})
	if err != nil || p.Metadata["order"] != "42" {
		t.Fatalf("p=%v err=%v", p, err)
	}
}

func TestSignatureFailureReasons(t *testing.T) {
	good := sign("s", "123", "req", "1700000000")
	cases := []struct {
		header string
		v      WebhookValidator
		want   SignatureFailure
	}{
		{"", WebhookValidator{Secret: "s"}, SignatureMissingHeader},
		{"garbage", WebhookValidator{Secret: "s"}, SignatureMalformedHeader},
		{"v1=abc", WebhookValidator{Secret: "s"}, SignatureMissingTS},
		{"ts=abc,v1=abc", WebhookValidator{Secret: "s"}, SignatureMalformedHeader},
		{"ts=1700000000,v2=abc", WebhookValidator{Secret: "s"}, SignatureMissingHash},
		{good, WebhookValidator{Secret: "other"}, SignatureMismatch},
		{good, WebhookValidator{Secret: "s", Tolerance: time.Minute}, SignatureOutOfTolerance},
	}
	for _, tc := range cases {
		err := tc.v.Validate(tc.header, "req", "123")
		var se *SignatureError
		if !errors.As(err, &se) || se.Reason != tc.want || !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("header %q: err=%v, want %s", tc.header, err, tc.want)
			continue
		}
		if se.RequestID != "req" {
			t.Errorf("RequestID = %q", se.RequestID)
		}
	}

	fixed := WebhookValidator{Secret: "s", Tolerance: time.Minute, Now: func() time.Time { return time.Unix(1700000030, 0) }}
	if err := fixed.Validate(good, "req", "123"); err != nil {
		t.Errorf("within tolerance: %v", err)
	}
}

func TestSignatureVersionsAndCase(t *testing.T) {
	// Signed with lowercase data.id and no request id; header hex in uppercase.
	v1 := signManifest("s", "abc123", "", "1700000000")
	header := "ts=1700000000,v2=deadbeef,v1=" + strings.ToUpper(v1)
	if err := (WebhookValidator{Secret: "s"}).Validate(header, "", "ABC123"); err != nil {
		t.Errorf("case-insensitive validation failed: %v", err)
	}
	if err := (WebhookValidator{Secret: "s", Versions: []string{"v2"}}).Validate(header, "", "abc123"); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("v2 preference should check the v2 hash: %v", err)
	}
}

func sign2(secret, dataID, ts string) string {
	return "ts=" + ts + ",v1=" + signManifest(secret, dataID, "", ts)
}

func TestAsyncWebhookHandler(t *testing.T) {
	api := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":555,"status":"approved"}`)
	})
	var wg sync.WaitGroup
	wg.Add(1)
	var got *Payment
	var errs []error
	h := api.NewWebhookHandler(WebhookConfig{
		Validator: &WebhookValidator{Secret: "s"},
		Async:     true,
		Handle: func(ctx context.Context, p *Payment) error {
			defer wg.Done()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			got = p
			return nil
		},
		OnError: func(n *Notification, err error) { errs = append(errs, err) },
	})

	r := httptest.NewRequest(http.MethodPost, "/wh?data.id=555&type=payment", strings.NewReader(`{"type":"payment","data":{"id":"555"}}`))
	r.Header.Set("X-Signature", sign2("s", "555", "1700000000"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	wg.Wait()
	if got == nil || got.ID != 555 {
		t.Errorf("payment = %v", got)
	}

	bad := httptest.NewRequest(http.MethodPost, "/wh?data.id=555&type=payment", nil)
	bad.Header.Set("X-Signature", sign2("wrong", "555", "1700000000"))
	bad.Header.Set("X-Request-Id", "rid-1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	var se *SignatureError
	if rec.Code != http.StatusUnauthorized || len(errs) != 1 || !errors.As(errs[0], &se) || se.RequestID != "rid-1" {
		t.Errorf("bad signature: code=%d errs=%v", rec.Code, errs)
	}
}

func TestSyncWebhookHandlerErrors(t *testing.T) {
	api := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":1,"status":"approved"}`)
	})
	h := api.NewWebhookHandler(WebhookConfig{Handle: func(context.Context, *Payment) error { return errors.New("db down") }})
	r := httptest.NewRequest(http.MethodPost, "/wh?data.id=1&type=payment", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("handler error: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/wh?topic=merchant_order&id=9", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("non-payment event: code=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wh", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: code=%d", rec.Code)
	}
}
