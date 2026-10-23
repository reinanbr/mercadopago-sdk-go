// Creates a PIX charge, prints the "copia e cola" code and waits for payment.
//
//	MERCADOPAGO_ACCESS_TOKEN=APP_USR-... go run ./examples/create
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	mp "github.com/reinanbr/mercadopago-sdk-go"
)

func main() {
	client, err := mp.NewFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	payment, err := client.CreatePix(ctx, mp.CreatePixRequest{
		Amount:            mp.Reais(2.00),
		Description:       "Pagamento de teste",
		Payer:             mp.Payer{Email: "cliente@example.com", FirstName: "Fulano", LastName: "Silva"},
		ExternalReference: "pedido-42",
		ExpiresIn:         10 * time.Minute,
	})
	if err != nil {
		var apiErr *mp.APIError
		if errors.As(err, &apiErr) {
			log.Fatalf("Mercado Pago recusou (%d): %s", apiErr.StatusCode, apiErr.Message)
		}
		log.Fatal(err)
	}
	fmt.Println(payment)

	if png, err := payment.QRCodePNG(); err == nil {
		_ = os.WriteFile("qrcode.png", png, 0o644)
		fmt.Println("QR code salvo em qrcode.png")
	}

	fmt.Println("Aguardando pagamento...")
	final, err := client.WaitForPayment(ctx, payment.ID, 5*time.Second)
	switch {
	case errors.Is(err, mp.ErrExpired):
		fmt.Println("PIX expirou sem pagamento.")
	case err != nil:
		log.Fatal(err)
	case final.IsPaid():
		fmt.Printf("Pago! %s em %s\n", final.Amount, final.DateApproved.Format(time.RFC3339))
	default:
		fmt.Println("Status final:", final.Status, final.StatusDetail)
	}
}
