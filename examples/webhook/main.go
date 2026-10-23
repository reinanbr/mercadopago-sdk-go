// Receives Mercado Pago payment webhooks with signature validation,
// answering immediately, processing in the background and finishing the
// pending work on shutdown (Ctrl+C).
//
//	MERCADOPAGO_ACCESS_TOKEN=APP_USR-... MERCADOPAGO_WEBHOOK_SECRET=... go run ./examples/webhook
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	mp "github.com/reinanbr/mercadopago-sdk-go"
)

// orderTotal stands for a lookup in your database.
func orderTotal(ctx context.Context, orderID string) (mp.Amount, bool) {
	return mp.Reais(2.00), orderID != ""
}

func main() {
	client, err := mp.NewFromEnv(mp.WithLogger(slog.Default()))
	if err != nil {
		log.Fatal(err)
	}

	webhook := client.NewWebhookHandler(mp.WebhookConfig{
		Validator: &mp.WebhookValidator{
			Secret:    os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"),
			Tolerance: 10 * time.Minute,
		},
		Async: true, // answer 200 within Mercado Pago's 22s limit
		Handle: func(ctx context.Context, p *mp.Payment) error {
			total, ok := orderTotal(ctx, p.ExternalReference)
			if !ok {
				return nil // not one of our orders
			}
			// Never trust the reference alone: check the amount too.
			if err := p.CheckOrder(p.ExternalReference, total); err != nil {
				return err
			}
			switch {
			case p.IsPaid():
				// Mark the order as paid. Webhooks may arrive more than
				// once, so make this idempotent.
			case p.Status == mp.StatusRefunded || p.Status == mp.StatusChargedBack:
				// Reverse the order.
			}
			log.Printf("pedido %s: pagamento %d %s", p.ExternalReference, p.ID, p.Status)
			return nil
		},
		OnError: func(n *mp.Notification, err error) {
			log.Printf("webhook request-id=%s: %v", n.RequestID, err)
		},
	})

	mux := http.NewServeMux()
	mux.Handle("POST /webhook", webhook)
	srv := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		log.Println("escutando em :8080")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	if err := webhook.Wait(shutdown); err != nil {
		log.Printf("notificações não concluídas: %v", err)
	}
}
