package mercadopagopix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]Amount{
		"10": 1000, "10.5": 1050, "10,5": 1050, "0,01": 1, "1.234,56": 123456,
		"R$ 1.234.567,89": 123456789, "-3,00": -300, " 12.34 ": 1234, ",50": 50,
	} {
		if got, err := ParseAmount(in); err != nil || got != want {
			t.Errorf("ParseAmount(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "1.234", "1,2,3", "12.345,678", "1.23.456,00", "12a", "R$"} {
		if _, err := ParseAmount(in); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("ParseAmount(%q) err = %v", in, err)
		}
	}
}

func TestAmountUnmarshalExact(t *testing.T) {
	var a Amount
	for in, want := range map[string]Amount{`0.29`: 29, `1.005`: 101, `-2.5`: -250, `1e2`: 10000, `"0.1"`: 10, `9999999.99`: 999999999} {
		if err := json.Unmarshal([]byte(in), &a); err != nil || a != want {
			t.Errorf("Unmarshal(%s) = %d, %v; want %d", in, a, err, want)
		}
	}
	if err := json.Unmarshal([]byte(`"x"`), &a); err == nil {
		t.Error("expected error")
	}
}

func TestDocuments(t *testing.T) {
	for _, n := range []string{"529.982.247-25", "52998224725", "111.444.777-35"} {
		if !ValidCPF(n) {
			t.Errorf("ValidCPF(%q) = false", n)
		}
	}
	for _, n := range []string{"529.982.247-24", "111.111.111-11", "1234", ""} {
		if ValidCPF(n) {
			t.Errorf("ValidCPF(%q) = true", n)
		}
	}
	for _, n := range []string{"11.222.333/0001-81", "11222333000181", "45.723.174/0001-10"} {
		if !ValidCNPJ(n) {
			t.Errorf("ValidCNPJ(%q) = false", n)
		}
	}
	for _, n := range []string{"11.222.333/0001-80", "00000000000000", "123"} {
		if ValidCNPJ(n) {
			t.Errorf("ValidCNPJ(%q) = true", n)
		}
	}
	if id := CNPJ("11.222.333/0001-81"); id.Type != "CNPJ" || id.Number != "11222333000181" {
		t.Errorf("CNPJ = %+v", id)
	}
}

func TestCreatePixOrderData(t *testing.T) {
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, paymentFixture)
	})
	_, err := c.CreatePix(context.Background(), CreatePixRequest{
		Items: []Item{{ID: "A", Title: "Livro", Quantity: 3, UnitPrice: Reais(10.10)}},
		Payer: Payer{
			Email: "a@example.com", FirstName: "Ana",
			Identification: &Identification{Type: "cpf", Number: "529.982.247-25"},
			Phone:          &Phone{AreaCode: "11", Number: "987654321"},
		},
		Shipment: &Address{ZipCode: "01310-100", City: "São Paulo", State: "SP"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["transaction_amount"] != 30.3 {
		t.Errorf("amount from items = %v", body["transaction_amount"])
	}
	payer := body["payer"].(map[string]any)
	if id := payer["identification"].(map[string]any); id["type"] != "CPF" || id["number"] != "52998224725" {
		t.Errorf("identification = %v", id)
	}
	if _, ok := payer["phone"]; ok {
		t.Error("phone must go in additional_info, not payer")
	}
	ai := body["additional_info"].(map[string]any)
	items := ai["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["unit_price"] != 10.1 {
		t.Errorf("items = %v", items)
	}
	if ai["payer"].(map[string]any)["phone"].(map[string]any)["area_code"] != "11" {
		t.Errorf("additional_info.payer = %v", ai["payer"])
	}
	if ai["shipments"].(map[string]any)["receiver_address"].(map[string]any)["state_name"] != "SP" {
		t.Errorf("shipments = %v", ai["shipments"])
	}
}

func TestCreatePixOrderValidation(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("should not reach the API") })
	payer := Payer{Email: "a@example.com"}
	for name, req := range map[string]CreatePixRequest{
		"bad CPF":     {Amount: 100, Payer: Payer{Email: "a@example.com", Identification: CPF("123.456.789-00")}},
		"bad CNPJ":    {Amount: 100, Payer: Payer{Email: "a@example.com", Identification: CNPJ("11.222.333/0001-00")}},
		"no doc type": {Amount: 100, Payer: Payer{Email: "a@example.com", Identification: &Identification{Number: "1"}}},
		"item qty":    {Payer: payer, Items: []Item{{Title: "x", UnitPrice: 100}}},
		"item title":  {Payer: payer, Items: []Item{{Quantity: 1, UnitPrice: 100}}},
		"free items":  {Payer: payer, Items: []Item{{Title: "x", Quantity: 1}}},
	} {
		if _, err := c.CreatePix(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, _, err := c.GetOrCreatePix(context.Background(), CreatePixRequest{Amount: 100, Payer: payer}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("GetOrCreatePix without reference: %v", err)
	}
}

func TestPaymentOrderHelpers(t *testing.T) {
	var p Payment
	if err := json.Unmarshal([]byte(paymentFixture), &p); err != nil {
		t.Fatal(err)
	}
	if !p.IsPayable() || p.RefundableAmount() != 0 {
		t.Errorf("pending: payable=%v refundable=%s", p.IsPayable(), p.RefundableAmount())
	}
	p.ExternalReference = "pedido-1"
	if err := p.CheckOrder("pedido-1", Reais(2.5)); err != nil {
		t.Error(err)
	}
	if err := p.CheckOrder("pedido-2", Reais(2.5)); !errors.Is(err, ErrOrderMismatch) {
		t.Errorf("wrong reference: %v", err)
	}
	if err := p.CheckOrder("pedido-1", Reais(25)); !errors.Is(err, ErrOrderMismatch) {
		t.Errorf("wrong amount: %v", err)
	}
	p.Status, p.AmountRefunded = StatusApproved, Reais(1)
	if p.IsPayable() || p.RefundableAmount() != Reais(1.5) {
		t.Errorf("approved: payable=%v refundable=%s", p.IsPayable(), p.RefundableAmount())
	}

	var q Payment
	_ = json.Unmarshal([]byte(`{"id":1,"payer":{"id":"77","email":"a@b.com"},"transaction_details":{"net_received_amount":9.9,"total_paid_amount":10},"money_release_date":"2026-01-01T00:00:00.000-03:00"}`), &q)
	if q.PayerID != "77" || q.Payer.Email != "a@b.com" || q.NetReceivedAmount != 990 || q.TotalPaidAmount != 1000 || q.MoneyReleaseDate.IsZero() {
		t.Errorf("payment fields = %+v", q)
	}
}

func TestWithLogger(t *testing.T) {
	var buf bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "rid-9")
		http.Error(w, `{"message":"nope"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	c, _ := New("TEST-secret-token", WithBaseURL(srv.URL), WithLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	_, _ = c.GetPayment(context.Background(), 1)
	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "status=404") || !strings.Contains(out, "request_id=rid-9") {
		t.Errorf("log = %s", out)
	}
	if strings.Contains(out, "secret-token") {
		t.Error("token leaked into logs")
	}
}

func TestWebhookReceiverConcurrencyAndWait(t *testing.T) {
	api := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"id":1,"status":"approved"}`) })
	release := make(chan struct{})
	h := api.NewWebhookHandler(WebhookConfig{
		Async:         true,
		MaxConcurrent: 1,
		Handle:        func(context.Context, *Payment) error { <-release; return nil },
	})
	send := func() int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/wh?data.id=1&type=payment", nil))
		return rec.Code
	}
	if code := send(); code != http.StatusOK {
		t.Fatalf("first: %d", code)
	}
	if code := send(); code != http.StatusServiceUnavailable {
		t.Errorf("second while busy: %d", code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := h.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait while busy: %v", err)
	}
	close(release)
	if err := h.Wait(context.Background()); err != nil {
		t.Errorf("Wait: %v", err)
	}
	if code := send(); code != http.StatusOK {
		t.Errorf("after release: %d", code)
	}
	_ = h.Wait(context.Background())
}

func TestSignWebhook(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	h := SignWebhook("s", "123", "rid", ts)
	if err := (WebhookValidator{Secret: "s", Tolerance: time.Minute, Now: func() time.Time { return ts }}).Validate(h, "rid", "123"); err != nil {
		t.Error(err)
	}
}
