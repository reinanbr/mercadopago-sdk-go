package mptest_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	mp "github.com/reinanbr/mercadopago-sdk-go"
	"github.com/reinanbr/mercadopago-sdk-go/mptest"
)

// shop is a minimal store: orders in memory, paid through the webhook.
type shop struct {
	mu     sync.Mutex
	orders map[string]mp.Amount // order ID → total
	paid   map[string]int64     // order ID → payment ID
	events []string
}

func (s *shop) handle(ctx context.Context, p *mp.Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, string(p.Status))
	total, ok := s.orders[p.ExternalReference]
	if !ok {
		return nil // not ours
	}
	if err := p.CheckOrder(p.ExternalReference, total); err != nil {
		return err
	}
	if p.IsPaid() {
		s.paid[p.ExternalReference] = p.ID
	}
	return nil
}

func setup(t *testing.T) (*mptest.Server, *mp.Client, *shop, *httptest.Server) {
	t.Helper()
	srv := mptest.NewServer()
	t.Cleanup(srv.Close)
	client := srv.Client()
	sh := &shop{orders: map[string]mp.Amount{}, paid: map[string]int64{}}
	web := httptest.NewServer(client.NewWebhookHandler(mp.WebhookConfig{
		Validator: srv.Validator(),
		Handle:    sh.handle,
	}))
	t.Cleanup(web.Close)
	return srv, client, sh, web
}

func TestCheckoutFlow(t *testing.T) {
	srv, client, sh, web := setup(t)
	ctx := context.Background()
	items := []mp.Item{
		{ID: "SKU-1", Title: "Camiseta", Quantity: 2, UnitPrice: mp.Reais(49.90)},
		{ID: "SKU-2", Title: "Caneca", Quantity: 1, UnitPrice: mp.Reais(30)},
	}
	sh.orders["pedido-1"] = mp.ItemsTotal(items)

	req := mp.CreatePixRequest{
		Description:       "Pedido #1",
		Items:             items,
		Payer:             mp.Payer{Email: "cliente@example.com", Identification: mp.CPF("529.982.247-25")},
		ExternalReference: "pedido-1",
		NotificationURL:   web.URL + "/webhook",
		ExpiresIn:         30 * time.Minute,
	}
	p, created, err := client.GetOrCreatePix(ctx, req)
	if err != nil || !created {
		t.Fatalf("GetOrCreatePix: created=%v err=%v", created, err)
	}
	if p.Amount != mp.Reais(129.80) || !p.IsPayable() || p.QRCode == "" {
		t.Fatalf("payment = %+v", p)
	}
	if p.Payer.Identification == nil || p.Payer.Identification.Number != "52998224725" {
		t.Errorf("CPF not normalized: %+v", p.Payer.Identification)
	}
	if _, err := p.QRCodePNG(); err != nil {
		t.Errorf("QRCodePNG: %v", err)
	}

	// Reloading the checkout reuses the same PIX.
	again, created, err := client.GetOrCreatePix(ctx, req)
	if err != nil || created || again.ID != p.ID {
		t.Fatalf("reload: id=%d created=%v err=%v", again.ID, created, err)
	}

	if err := srv.Approve(p.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if sh.paid["pedido-1"] != p.ID {
		t.Fatalf("order not paid by webhook: %v", sh.paid)
	}
	paid, err := client.FindByExternalReference(ctx, "pedido-1", (*mp.Payment).IsPaid)
	if err != nil || paid.ID != p.ID || paid.NetReceivedAmount == 0 || paid.NetReceivedAmount >= paid.Amount {
		t.Fatalf("FindByExternalReference: %+v, %v", paid, err)
	}

	// Partial then full refund.
	if _, err := client.Refund(ctx, p.ID, mp.Reais(30)); err != nil {
		t.Fatal(err)
	}
	cur, _ := client.GetPayment(ctx, p.ID)
	if cur.RefundableAmount() != mp.Reais(99.80) {
		t.Errorf("refundable = %s", cur.RefundableAmount())
	}
	if _, err := client.Refund(ctx, p.ID, 0); err != nil {
		t.Fatal(err)
	}
	cur, _ = client.GetPayment(ctx, p.ID)
	rs, _ := client.ListRefunds(ctx, p.ID)
	if cur.Status != mp.StatusRefunded || len(rs) != 2 || cur.RefundableAmount() != 0 {
		t.Errorf("after full refund: status=%s refunds=%d", cur.Status, len(rs))
	}
}

func TestWebhookRejectsTamperedAmount(t *testing.T) {
	srv, client, sh, web := setup(t)
	sh.orders["pedido-2"] = mp.Reais(100)
	// An attacker creates a cheaper PIX pointing at the same order.
	p, err := client.CreatePix(context.Background(), mp.CreatePixRequest{
		Amount: mp.Reais(1), Payer: mp.Payer{Email: "x@example.com"},
		ExternalReference: "pedido-2", NotificationURL: web.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Approve(p.ID); err == nil {
		t.Fatal("webhook should fail for a tampered amount")
	}
	if _, ok := sh.paid["pedido-2"]; ok {
		t.Fatal("order released for the wrong amount")
	}
}

func TestExpirationAndCancel(t *testing.T) {
	srv, client, _, _ := setup(t)
	ctx := context.Background()
	now := time.Now()
	srv.Now = func() time.Time { return now }
	p, err := client.CreatePix(ctx, mp.CreatePixRequest{Amount: mp.Reais(5), Payer: mp.Payer{Email: "a@example.com"}, ExternalReference: "pedido-3", ExpiresIn: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	cur, _ := client.GetPayment(ctx, p.ID)
	if cur.Status != mp.StatusCancelled || cur.StatusDetail != "expired" {
		t.Errorf("not expired: %s/%s", cur.Status, cur.StatusDetail)
	}
	now = time.Now()
	// An expired PIX is not reused: a new one is created.
	again, created, err := client.GetOrCreatePix(ctx, mp.CreatePixRequest{Amount: mp.Reais(5), Payer: mp.Payer{Email: "a@example.com"}, ExternalReference: "pedido-3", ExpiresIn: time.Hour})
	if err != nil || !created || again.ID == p.ID {
		t.Errorf("expected a new PIX: created=%v err=%v", created, err)
	}
	if _, err := client.CancelPayment(ctx, again.ID); err != nil {
		t.Fatal(err)
	}
	if err := srv.Approve(again.ID); err == nil {
		t.Error("cancelled payment should not be approvable")
	}
	_, err = client.UpdatePayment(ctx, p.ID, map[string]any{"external_reference": "x"})
	var apiErr *mp.APIError
	if !errors.As(err, &apiErr) || !apiErr.HasCause("4243") {
		t.Errorf("update restriction: %v", err)
	}
}

func TestIdempotencyAndFailures(t *testing.T) {
	srv, _, _, _ := setup(t)
	client := srv.Client(mp.WithRetries(2, time.Millisecond))
	ctx := context.Background()
	req := mp.CreatePixRequest{Amount: mp.Reais(1), Payer: mp.Payer{Email: "a@example.com"}, IdempotencyKey: "pedido-4-1"}

	srv.FailNext(http.StatusServiceUnavailable)
	a, err := client.CreatePix(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.CreatePix(ctx, req)
	if err != nil || a.ID != b.ID {
		t.Fatalf("same key created two payments: %d %d %v", a.ID, b.ID, err)
	}
	for _, r := range srv.Requests() {
		if r.Method == http.MethodPost && r.IdempotencyKey != "pedido-4-1" {
			t.Errorf("key = %q", r.IdempotencyKey)
		}
	}

	srv.FailNext(500, 500, 500)
	if _, err := client.GetPayment(ctx, a.ID); err == nil {
		t.Error("expected error after retries")
	}
	if _, err := client.GetPayment(ctx, 1); !errors.Is(err, mp.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestChargeBackWebhook(t *testing.T) {
	srv, client, sh, web := setup(t)
	sh.orders["pedido-5"] = mp.Reais(10)
	p, _ := client.CreatePix(context.Background(), mp.CreatePixRequest{Amount: mp.Reais(10), Payer: mp.Payer{Email: "a@example.com"}, ExternalReference: "pedido-5", NotificationURL: web.URL})
	if err := srv.Approve(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := srv.ChargeBack(p.ID); err != nil {
		t.Fatal(err)
	}
	if got := sh.events; len(got) != 2 || got[1] != string(mp.StatusChargedBack) {
		t.Errorf("events = %v", got)
	}
}
