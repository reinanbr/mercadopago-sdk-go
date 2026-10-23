// Receives Mercado Pago payment webhooks with signature validation,
// answering immediately and processing in the background.
//
//	MERCADOPAGO_ACCESS_TOKEN=APP_USR-... MERCADOPAGO_WEBHOOK_SECRET=... go run ./examples/webhook
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	mp "github.com/reinanbr/mercadopago-sdk-go"
)

func main() {
	client, err := mp.NewFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/webhook", client.NewWebhookHandler(mp.WebhookConfig{
		Validator: &mp.WebhookValidator{
			Secret:    os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"),
			Tolerance: 10 * time.Minute,
		},
		Async: true, // answer 200 within Mercado Pago's 22s limit
		Handle: func(ctx context.Context, p *mp.Payment) error {
			log.Printf("pagamento %d (%s): %s", p.ID, p.ExternalReference, p.Status)
			if p.IsPaid() {
				// Mark the order as paid. Webhooks may arrive more than
				// once, so make this idempotent.
			}
			return nil
		},
		OnError: func(n *mp.Notification, err error) {
			log.Printf("webhook request-id=%s: %v", n.RequestID, err)
		},
	}))

	log.Println("escutando em :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
