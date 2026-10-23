// Package mercadopagopix is a small, dependency-free client for creating and
// tracking PIX payments through the Mercado Pago REST API.
//
//	import mp "github.com/reinanbr/mercadopago-sdk-go"
//
// It is a Go rewrite of the Python library mercadopagovp, with a few
// improvements: context support, money stored in centavos (no float
// rounding), automatic idempotency keys, retries on transient failures,
// typed API errors, cancellation, refunds, polling and webhook signature
// validation.
//
//	client, err := mercadopagopix.NewFromEnv()
//	if err != nil { ... }
//
//	p, err := client.CreatePix(ctx, mercadopagopix.CreatePixRequest{
//		Amount:      mercadopagopix.Reais(2.50),
//		Description: "Pedido #42",
//		Payer:       mercadopagopix.Payer{Email: "cliente@example.com"},
//		ExpiresIn:   15 * time.Minute,
//	})
//	fmt.Println(p.QRCode) // PIX "copia e cola"
package mercadopagopix
