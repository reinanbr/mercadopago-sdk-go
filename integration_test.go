//go:build integration

// Integration tests against the real Mercado Pago API using sandbox
// credentials. They read .env.test (override with MP_ENV_FILE; see
// .env.example) and refuse to run with production tokens unless
// MP_ALLOW_LIVE=1.
//
//	go test -tags integration -run Integration -v ./...
package mercadopagopix

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	itOnce   sync.Once
	itClient *Client
	itErr    error
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	itOnce.Do(func() {
		file := os.Getenv("MP_ENV_FILE")
		if file == "" {
			file = ".env.test"
		}
		vars, err := ReadEnvFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			itErr = err
			return
		}
		if os.Getenv("MERCADOPAGO_ACCESS_TOKEN") == "" && vars["ACCESS_TOKEN"] != "" {
			os.Setenv("MERCADOPAGO_ACCESS_TOKEN", vars["ACCESS_TOKEN"])
		}
		itClient, itErr = NewFromEnv(WithRetries(2, time.Second))
	})
	if errors.Is(itErr, ErrMissingToken) {
		t.Skip("no access token configured (copy .env.example to .env.test and fill ACCESS_TOKEN)")
	}
	if itErr != nil {
		t.Fatal(itErr)
	}
	if !itClient.TestMode() && os.Getenv("MP_ALLOW_LIVE") != "1" {
		t.Skip("refusing to run integration tests with a production token; set MP_ALLOW_LIVE=1 to override")
	}
	return itClient
}

func itCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// itPayerEmail returns the buyer email (override with MP_PAYER_EMAIL).
// With TEST credentials of a regular account Mercado Pago rejects
// @testuser.com payers and the seller's own e-mail (cause 4390), but
// accepts an ordinary address.
func itPayerEmail() string {
	if e := os.Getenv("MP_PAYER_EMAIL"); e != "" {
		return e
	}
	return "comprador.teste.mpgo@gmail.com"
}

// itPayer returns a buyer different from the seller account, as required by
// Mercado Pago (paying yourself is rejected).
func itPayer() Payer {
	return Payer{
		Email:          itPayerEmail(),
		FirstName:      "Comprador",
		LastName:       "Teste",
		Identification: &Identification{Type: "CPF", Number: "19119119100"},
	}
}

func itRef(name string) string {
	return fmt.Sprintf("it-%s-%d", name, time.Now().UnixNano())
}

func logPayment(t *testing.T, p *Payment) {
	t.Helper()
	t.Logf("payment id=%d status=%s detail=%s amount=%s live_mode=%v", p.ID, p.Status, p.StatusDetail, p.Amount, p.LiveMode)
	t.Logf("created=%s expires=%s (in %s)", fmtTime(p.DateCreated), fmtTime(p.DateOfExpiration), p.TimeToExpiration().Round(time.Second))
	if p.QRCode != "" {
		t.Logf("qr_code=%s…(%d chars) qr_png_base64=%d chars", p.QRCode[:min(40, len(p.QRCode))], len(p.QRCode), len(p.QRCodeBase64))
	}
	if p.TicketURL != "" {
		t.Logf("ticket_url=%s", strings.SplitN(p.TicketURL, "?", 2)[0])
	}
}

func logAPIError(t *testing.T, err error) {
	t.Helper()
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Logf("api error: status=%d code=%q message=%q causes=%v", apiErr.StatusCode, apiErr.Code, apiErr.Message, apiErr.Causes)
	} else if err != nil {
		t.Logf("error: %v", err)
	}
}

// createTestPix creates a R$ 2,00 PIX and registers a cleanup that cancels it.
func createTestPix(t *testing.T, c *Client, ref string) *Payment {
	t.Helper()
	p, err := c.CreatePix(itCtx(t), CreatePixRequest{
		Amount:            Reais(2.00),
		Description:       "Teste de integração mercadopagopix",
		Payer:             itPayer(),
		ExternalReference: ref,
		ExpiresIn:         30 * time.Minute,
		Metadata:          map[string]any{"suite": "integration"},
	})
	if err != nil {
		logAPIError(t, err)
		t.Fatalf("CreatePix: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if cur, err := c.GetPayment(ctx, p.ID); err == nil && cur.Status == StatusPending {
			_, _ = c.CancelPayment(ctx, p.ID)
		}
	})
	return p
}

func TestIntegrationAccount(t *testing.T) {
	c := integrationClient(t)
	a, err := c.Me(itCtx(t))
	if err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	t.Logf("account site=%s test_user=%v tags=%v", a.SiteID, a.IsTestUser(), a.Tags)
	if a.ID == 0 {
		t.Error("account ID is zero")
	}
	t.Logf("payer email used by the suite: %s", itPayerEmail())
	if a.SiteID != "MLB" {
		t.Errorf("site_id = %s, PIX requires a Brazilian (MLB) account", a.SiteID)
	}
}

func TestIntegrationCreatePix(t *testing.T) {
	c := integrationClient(t)
	ref := itRef("create")
	p := createTestPix(t, c, ref)
	logPayment(t, p)

	if p.ID == 0 {
		t.Fatal("payment ID is zero")
	}
	if p.Status != StatusPending {
		t.Errorf("status = %s, want pending", p.Status)
	}
	if p.Amount != Reais(2) {
		t.Errorf("amount = %s, want R$ 2,00", p.Amount)
	}
	if p.CurrencyID != "BRL" || p.PaymentMethodID != "pix" {
		t.Errorf("currency=%s method=%s", p.CurrencyID, p.PaymentMethodID)
	}
	if p.ExternalReference != ref {
		t.Errorf("external_reference = %q, want %q", p.ExternalReference, ref)
	}
	if p.QRCode == "" || !strings.Contains(p.QRCode, "br.gov.bcb.pix") {
		t.Errorf("unexpected QR code %q", p.QRCode)
	}
	if png, err := p.QRCodePNG(); err != nil || len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Errorf("QRCodePNG invalid: %d bytes, %v", len(png), err)
	}
	if d := p.TimeToExpiration(); d < 25*time.Minute || d > 31*time.Minute {
		t.Errorf("time to expiration = %s, want ~30m", d)
	}
	if p.LiveMode {
		t.Error("live_mode = true with a TEST token")
	}
}

func TestIntegrationIdempotency(t *testing.T) {
	c := integrationClient(t)
	req := CreatePixRequest{
		Amount:         Reais(2.00),
		Description:    "Teste de idempotência",
		Payer:          itPayer(),
		IdempotencyKey: newIdempotencyKey(),
	}
	a, err := c.CreatePix(itCtx(t), req)
	if err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = c.CancelPayment(context.Background(), a.ID) })
	b, err := c.CreatePix(itCtx(t), req)
	if err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	t.Logf("first id=%d second id=%d (same idempotency key)", a.ID, b.ID)
	if a.ID != b.ID {
		t.Errorf("same idempotency key created two payments: %d and %d", a.ID, b.ID)
	}
}

func TestIntegrationGetPayment(t *testing.T) {
	c := integrationClient(t)
	created := createTestPix(t, c, itRef("get"))
	got, err := c.GetPayment(itCtx(t), created.ID)
	if err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	logPayment(t, got)
	if got.ID != created.ID || got.QRCode != created.QRCode || got.Amount != created.Amount {
		t.Errorf("GetPayment differs from created payment")
	}
	if got.Metadata["suite"] != "integration" {
		t.Errorf("metadata = %v", got.Metadata)
	}
	if len(got.Raw) == 0 {
		t.Error("Raw response is empty")
	}
}

func TestIntegrationSearchByExternalReference(t *testing.T) {
	c := integrationClient(t)
	ref := itRef("search")
	created := createTestPix(t, c, ref)

	// The search index is eventually consistent.
	var found []Payment
	for attempt := 1; attempt <= 6; attempt++ {
		res, err := c.SearchByExternalReference(itCtx(t), ref)
		if err != nil {
			logAPIError(t, err)
			t.Fatal(err)
		}
		if len(res) > 0 {
			found = res
			t.Logf("found %d result(s) after %d attempt(s)", len(res), attempt)
			break
		}
		time.Sleep(2 * time.Second)
	}
	if len(found) == 0 {
		t.Fatalf("payment %d not found by external reference %q", created.ID, ref)
	}
	if found[0].ID != created.ID {
		t.Errorf("first result id=%d, want %d", found[0].ID, created.ID)
	}
}

func TestIntegrationCancelPayment(t *testing.T) {
	c := integrationClient(t)
	created := createTestPix(t, c, itRef("cancel"))
	p, err := c.CancelPayment(itCtx(t), created.ID)
	if err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	logPayment(t, p)
	if p.Status != StatusCancelled {
		t.Errorf("status = %s, want cancelled", p.Status)
	}

	// A cancelled payment is final, so WaitForPayment returns at once.
	start := time.Now()
	w, err := c.WaitForPayment(itCtx(t), created.ID, time.Second)
	if err != nil || w.Status != StatusCancelled {
		t.Errorf("WaitForPayment = %v, %v", w, err)
	}
	t.Logf("WaitForPayment returned in %s", time.Since(start).Round(time.Millisecond))
}

func TestIntegrationWaitForPaymentTimeout(t *testing.T) {
	c := integrationClient(t)
	created := createTestPix(t, c, itRef("wait"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := c.WaitForPayment(ctx, created.ID, time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if p == nil || p.Status != StatusPending {
		t.Fatalf("last known payment = %v, want pending", p)
	}
	t.Logf("unpaid PIX still %s after timeout, as expected", p.Status)
}

func TestIntegrationNotFound(t *testing.T) {
	c := integrationClient(t)
	_, err := c.GetPayment(itCtx(t), 1)
	logAPIError(t, err)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestIntegrationInvalidToken(t *testing.T) {
	integrationClient(t) // honour the skip rules
	bad, _ := New("TEST-0000000000000000-000000-00000000000000000000000000000000-000000000", WithRetries(0, 0))
	_, err := bad.GetPayment(itCtx(t), 1)
	logAPIError(t, err)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestIntegrationAPIValidationError(t *testing.T) {
	c := integrationClient(t)
	// Passes local validation but is rejected by the API: Mercado Pago
	// cannot deliver webhooks to localhost.
	p, err := c.CreatePix(itCtx(t), CreatePixRequest{
		Amount:          Reais(2),
		Payer:           itPayer(),
		NotificationURL: "http://localhost:8080/webhook",
	})
	if err == nil {
		_, _ = c.CancelPayment(context.Background(), p.ID)
		t.Fatalf("API accepted a localhost notification_url (payment %d)", p.ID)
	}
	logAPIError(t, err)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		t.Fatalf("err = %v, want APIError 400", err)
	}
}

func TestIntegrationSearchFiltersAndPaging(t *testing.T) {
	c := integrationClient(t)
	ref := itRef("paging")
	created := createTestPix(t, c, ref)

	// Sandbox quirks: only external_reference (optionally with status)
	// matches, and only while a single payment uses that reference.
	// Multi-page iteration is covered by the unit tests.
	var page *SearchResult
	for attempt := 1; attempt <= 6; attempt++ {
		var err error
		page, err = c.SearchPayments(itCtx(t), SearchParams{ExternalReference: ref, Status: StatusPending, Limit: 1})
		if err != nil {
			logAPIError(t, err)
			t.Fatal(err)
		}
		if page.Paging.Total > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("page: %d result(s), paging=%+v", len(page.Results), page.Paging)
	if page.Paging.Total != 1 || len(page.Results) != 1 || page.Results[0].ID != created.ID {
		t.Fatalf("paging = %+v with %d results, want payment %d", page.Paging, len(page.Results), created.ID)
	}

	var ids []int64
	for p, err := range c.AllPayments(itCtx(t), SearchParams{ExternalReference: ref, Limit: 1}) {
		if err != nil {
			logAPIError(t, err)
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	t.Logf("AllPayments yielded %v and stopped at paging.total", ids)
	if len(ids) != 1 || ids[0] != created.ID {
		t.Errorf("AllPayments = %v, want [%d]", ids, created.ID)
	}

	cancelled, err := c.SearchPayments(itCtx(t), SearchParams{ExternalReference: ref, Status: StatusApproved})
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled.Results) != 0 {
		t.Errorf("status filter ignored: %d approved results", len(cancelled.Results))
	}
}

func TestIntegrationUpdateRestrictions(t *testing.T) {
	c := integrationClient(t)
	created := createTestPix(t, c, itRef("update"))
	// Mercado Pago only lets a PIX be cancelled; other updates are refused.
	for name, fields := range map[string]map[string]any{
		"metadata":           {"status": "pending", "metadata": map[string]any{"pedido": "42"}},
		"external_reference": {"external_reference": "outro"},
		"date_of_expiration": {"date_of_expiration": time.Now().Add(2 * time.Hour).Format(expirationLayout)},
	} {
		_, err := c.UpdatePayment(itCtx(t), created.ID, fields)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("update %s: err = %v, want APIError", name, err)
			continue
		}
		t.Logf("update %-18s -> %d %s", name, apiErr.StatusCode, apiErr.Message)
	}
}

func TestIntegrationRefundPendingPayment(t *testing.T) {
	c := integrationClient(t)
	created := createTestPix(t, c, itRef("refunds"))
	_, err := c.Refund(itCtx(t), created.ID, 0)
	logAPIError(t, err)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("refunding a pending PIX should fail with APIError, got %v", err)
	}
	// The sandbox also denies listing refunds of a pending payment (403).
	rs, err := c.ListRefunds(itCtx(t), created.ID)
	logAPIError(t, err)
	if err == nil && len(rs) != 0 {
		t.Errorf("pending payment has %d refunds", len(rs))
	}
}

func TestIntegrationIntegratorHeadersAndRawDo(t *testing.T) {
	integrationClient(t)
	c, err := NewFromEnv(WithIntegratorID("dev_mercadopagopix_test"), WithPlatformID("mercadopagopix"))
	if err != nil {
		t.Fatal(err)
	}
	var types []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.Do(itCtx(t), http.MethodGet, "/v1/identification_types", nil, nil, &types); err != nil {
		logAPIError(t, err)
		t.Fatal(err)
	}
	var ids []string
	for _, it := range types {
		ids = append(ids, it.ID)
	}
	t.Logf("identification types: %v", ids)
	if !slices.Contains(ids, "CPF") {
		t.Errorf("CPF not among %v", ids)
	}
}

func TestIntegrationRequestIDOnErrors(t *testing.T) {
	c := integrationClient(t)
	_, err := c.GetPayment(itCtx(t), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
	t.Logf("request id of the 404: %q", apiErr.RequestID)
	if apiErr.RequestID == "" {
		t.Error("API did not return X-Request-Id")
	}
}

func TestIntegrationOrderData(t *testing.T) {
	c := integrationClient(t)
	ctx := itCtx(t)
	payer := itPayer()
	payer.FirstName, payer.LastName = "Ana", "Souza"
	payer.Identification = CPF("529.982.247-25")
	payer.Phone = &Phone{AreaCode: "11", Number: "987654321"}
	payer.Address = &Address{ZipCode: "01310-100", StreetName: "Av. Paulista", StreetNumber: "1000"}
	req := CreatePixRequest{
		Items: []Item{
			{ID: "SKU-1", Title: "Camiseta", CategoryID: "fashion", Quantity: 2, UnitPrice: Reais(1.50)},
			{ID: "SKU-2", Title: "Adesivo", Quantity: 1, UnitPrice: Reais(0.50)},
		},
		Description:       "Pedido de integração",
		Payer:             payer,
		Shipment:          &Address{ZipCode: "01310-100", StreetName: "Av. Paulista", StreetNumber: "1000", City: "São Paulo", State: "SP", Complement: "apto 1"},
		ExternalReference: itRef("order"),
		ExpiresIn:         30 * time.Minute,
	}
	p, created, err := c.GetOrCreatePix(ctx, req)
	if err != nil {
		logAPIError(t, err)
		t.Fatalf("GetOrCreatePix: %v", err)
	}
	t.Cleanup(func() { _, _ = c.CancelPayment(context.Background(), p.ID) })
	logPayment(t, p)
	if !created || p.Amount != Reais(3.50) || !p.IsPayable() {
		t.Fatalf("created=%v amount=%s payable=%v", created, p.Amount, p.IsPayable())
	}
	if err := p.CheckOrder(req.ExternalReference, Reais(3.50)); err != nil {
		t.Error(err)
	}
	if !strings.Contains(string(p.Raw), `"SKU-1"`) {
		t.Errorf("additional_info.items not echoed back: %s", p.Raw)
	}
}
