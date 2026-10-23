# mercadopago-sdk-go

[![Go Reference](https://pkg.go.dev/badge/github.com/reinanbr/mercadopago-sdk-go.svg)](https://pkg.go.dev/github.com/reinanbr/mercadopago-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/reinanbr/mercadopago-sdk-go)](https://goreportcard.com/report/github.com/reinanbr/mercadopago-sdk-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Cliente Go **sem dependências** para criar e acompanhar pagamentos **PIX** via API do Mercado Pago.

Reescrita em Go da biblioteca Python [mercadopagovp](https://github.com/reinanbr/mercadopagovp), com melhorias.

## Instalação

```sh
go get github.com/reinanbr/mercadopago-sdk-go@latest
```

```go
import mp "github.com/reinanbr/mercadopago-sdk-go" // pacote mercadopagopix
```

Requer Go 1.23+.

📘 **Manual de uso avançado:** [`docs/manual.html`](docs/manual.html), que cobre opções por chamada, marketplace, retry, paginação, webhooks em produção, o comportamento do sandbox e a comparação com o SDK oficial.

## Uso rápido

```go
_ = mercadopagopix.LoadEnvFile(".env")       // opcional: carrega um .env sem sobrescrever o ambiente
client, err := mercadopagopix.NewFromEnv() // MERCADOPAGO_ACCESS_TOKEN, ACCESS_TOKEN ou TOKEN
// ou: mercadopagopix.New("APP_USR-...")

p, err := client.CreatePix(ctx, mercadopagopix.CreatePixRequest{
    Amount:            mercadopagopix.Reais(2.00),
    Description:       "Pedido #42",
    Payer:             mercadopagopix.Payer{Email: "cliente@example.com", FirstName: "Fulano"},
    NotificationURL:   "https://seu-dominio.com/webhook",
    ExternalReference: "pedido-42",
    ExpiresIn:         10 * time.Minute,
})
if err != nil { ... }

fmt.Println(p.QRCode)       // PIX copia e cola
png, _ := p.QRCodePNG()     // imagem do QR code
fmt.Println(p.TicketURL)    // página do Mercado Pago com o QR
fmt.Println(p.TimeToExpiration())
```

### Consultar / aguardar / cancelar / estornar

```go
p, err := client.GetPayment(ctx, id)
p, err := client.WaitForPayment(ctx, id, 5*time.Second) // até status final; ErrExpired se expirar
p, err := client.CancelPayment(ctx, id)
r, err := client.Refund(ctx, id, 0)                     // 0 = estorno total
r, err := client.Refund(ctx, id, mercadopagopix.Reais(1))
ps, err := client.SearchByExternalReference(ctx, "pedido-42")
acc, err := client.Me(ctx)                              // conta dona do token (checa credenciais)
client.TestMode()                                       // true para tokens TEST-
```

## E-commerce

### Checkout: itens, documento e reaproveitamento do PIX

```go
items := []mp.Item{
    {ID: "SKU-1", Title: "Camiseta", Quantity: 2, UnitPrice: mp.Reais(49.90)},
    {ID: "SKU-2", Title: "Caneca", Quantity: 1, UnitPrice: mp.Reais(30)},
}
p, created, err := client.GetOrCreatePix(ctx, mp.CreatePixRequest{
    Items:             items, // Amount vazio = soma dos itens (R$ 129,80)
    Description:       "Pedido #42",
    Payer: mp.Payer{
        Email:          "cliente@example.com",
        FirstName:      "Ana",
        Identification: mp.CPF("529.982.247-25"), // dígitos verificados antes de chamar a API
        Phone:          &mp.Phone{AreaCode: "11", Number: "987654321"},
    },
    Shipment:          &mp.Address{ZipCode: "01310-100", City: "São Paulo", State: "SP"},
    ExternalReference: "pedido-42",
    IdempotencyKey:    "pedido-42-1",
    NotificationURL:   "https://seu-dominio.com/webhook",
    ExpiresIn:         30 * time.Minute,
})
```

`GetOrCreatePix` devolve o PIX ainda pagável do pedido (mesmo valor, pendente, não expirado) ou cria um novo, então o cliente que recarrega a página vê o mesmo QR code. Itens, telefone e endereço vão em `additional_info` e melhoram a aprovação na análise antifraude.

Outros auxiliares:

```go
mp.ParseAmount("1.234,56")             // valor digitado: R$ 1.234,56 (sem float)
mp.ValidCPF("529.982.247-25"); mp.ValidCNPJ("11.222.333/0001-81")
client.FindByExternalReference(ctx, "pedido-42", (*mp.Payment).IsPaid) // pagamento aprovado do pedido
p.IsPayable()          // pendente, não expirado e com QR code
p.RefundableAmount()   // quanto ainda dá para estornar
p.NetReceivedAmount    // valor líquido após a tarifa
```

### Webhook de produção

Sempre confira o pedido antes de liberá-lo: `CheckOrder` recusa um pagamento de outro pedido ou com valor diferente (alguém pode gerar um PIX de R$ 1 com o `external_reference` do seu pedido).

```go
webhook := client.NewWebhookHandler(mp.WebhookConfig{
    Validator: &mp.WebhookValidator{Secret: secret, Tolerance: 10 * time.Minute},
    Async:     true, // responde 200 na hora (limite de 22 s do Mercado Pago)
    Handle: func(ctx context.Context, p *mp.Payment) error {
        order, err := db.Order(ctx, p.ExternalReference)
        if err != nil { return err }
        if err := p.CheckOrder(order.ID, order.Total); err != nil { return err } // ErrOrderMismatch
        if p.IsPaid() { return db.MarkPaid(ctx, order.ID, p.ID) } // idempotente: pode chegar repetido
        return nil
    },
})
mux.Handle("POST /webhook", webhook)
// no desligamento:
srv.Shutdown(ctx)
webhook.Wait(ctx) // termina as notificações já aceitas
```

No modo assíncrono, `MaxConcurrent` (padrão 32) limita o processamento simultâneo; acima disso responde 503 e o Mercado Pago reenvia depois. Veja [`examples/webhook`](examples/webhook/main.go).

### Logs

```go
client, _ := mp.NewFromEnv(mp.WithLogger(slog.Default()))
// cada tentativa HTTP: método, caminho, status, duração e X-Request-Id (nunca o token)
```

### Testes da sua loja com `mptest`

O sandbox não deixa aprovar um PIX. O pacote `mptest` é um Mercado Pago falso em memória que deixa: aprova, expira, estorna e envia o webhook assinado para a `notification_url`.

```go
import "github.com/reinanbr/mercadopago-sdk-go/mptest"

srv := mptest.NewServer()
defer srv.Close()
client := srv.Client()                 // aponta para o servidor falso
shop := httptest.NewServer(client.NewWebhookHandler(mp.WebhookConfig{
    Validator: srv.Validator(),        // assinatura com srv.WebhookSecret
    Handle:    minhaLoja.Handle,
}))

p, _ := client.CreatePix(ctx, mp.CreatePixRequest{..., NotificationURL: shop.URL})
srv.Approve(p.ID)      // pago + webhook (erro se o seu handler não responder 2xx)
srv.Expire(id)         // cancelled/expired, como o Mercado Pago faz
srv.ChargeBack(id)
srv.FailNext(503, 500) // injeta falhas para testar retry
srv.Now = func() time.Time { ... } // relógio controlado
srv.Requests()         // chamadas recebidas (com X-Idempotency-Key)
```

Suporta criar, consultar, cancelar, estornos, busca e idempotência. Veja [`mptest/mptest_test.go`](mptest/mptest_test.go) para um fluxo de checkout completo.

### Webhooks (simples)

```go
http.Handle("/webhook", client.WebhookHandler(os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"),
    func(ctx context.Context, p *mercadopagopix.Payment) error {
        if p.IsPaid() { /* liberar pedido (de forma idempotente) */ }
        return nil // erro => responde 500 e o Mercado Pago reenvia
    }))
```

O handler valida a assinatura `x-signature` (HMAC-SHA256), **busca o pagamento na API** (nunca confia no corpo do webhook) e só então chama sua função. Para uso manual: `ParseWebhook` e `VerifySignature`.

### Erros

```go
var apiErr *mercadopagopix.APIError
if errors.As(err, &apiErr) {
    fmt.Println(apiErr.StatusCode, apiErr.Code, apiErr.Message, apiErr.Causes)
}
errors.Is(err, mercadopagopix.ErrNotFound)       // 404
errors.Is(err, mercadopagopix.ErrUnauthorized)   // 401/403
errors.Is(err, mercadopagopix.ErrInvalidRequest) // validação local
apiErr.HasCause("4390")                          // código de causa específico
```

Para códigos comuns e pouco óbvios (4390, 4020, 13253, 2000), a mensagem de erro já vem com uma dica (`hint: ...`).

### Opções

```go
mercadopagopix.New(token,
    mercadopagopix.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
    mercadopagopix.WithRetries(3, time.Second), // padrão: 3 tentativas extras, 500ms com backoff
    mercadopagopix.WithUserAgent("minha-loja/1.0"),
)
```

## O que mudou em relação à versão Python

| Python (`mercadopagovp`) | Go (`mercadopagopix`) |
|---|---|
| Setters (`set_value`, `set_email`…) com estado mutável | Uma struct `CreatePixRequest`, cliente imutável e seguro para concorrência |
| Valores em `float` | `Amount` em centavos (`int64`), sem erro de arredondamento |
| Erros engolidos (`return None`) e `print` no meio da lib | Erros tipados (`APIError`, sentinelas com `errors.Is`), sem logs |
| Quebra com `KeyError` se faltar campo (ex.: `charges_execution_info`) | Campos ausentes/`null` tolerados; resposta crua em `Payment.Raw` |
| Sem idempotência | `X-Idempotency-Key` automático (ou o seu), reutilizado nas retentativas |
| Sem timeout/retry | `context.Context`, timeout padrão e retry com backoff em 429/5xx |
| Mínimo fixo de R$ 2,00 e nome obrigatório | Só exige valor > 0 e e-mail (o que a API exige) |
| Só criar e consultar | + cancelar, estornar, buscar por referência, aguardar pagamento, webhooks com assinatura |
| Depende do SDK oficial, `pytz`, `dotenv`, `kitano` | Só biblioteca padrão |

## Exemplos

```sh
MERCADOPAGO_ACCESS_TOKEN=TEST-... go run ./examples/create
MERCADOPAGO_ACCESS_TOKEN=APP_USR-... MERCADOPAGO_WEBHOOK_SECRET=... go run ./examples/webhook
```

## Testes e relatórios

```sh
make test               # unitários (httptest, sem rede)
make test-integration   # integração contra o sandbox do Mercado Pago
make report             # gera reports/unit.md (com cobertura) e reports/integration.md
```

Os testes de integração (`integration_test.go`, build tag `integration`):

- leem o `ACCESS_TOKEN` do `.env.test` (copie o `.env.example` e preencha; ou aponte `MP_ENV_FILE` para outro arquivo);
- **se recusam a rodar com token de produção** (`APP_USR-`), a menos que `MP_ALLOW_LIVE=1`;
- cancelam ao final cada PIX que criam;
- aceitam um e-mail de pagador alternativo em `MP_PAYER_EMAIL`.

O gerador de relatórios (`cmd/testreport`) mascara os tokens na saída.

### Comportamento observado no sandbox

- Com credenciais `TEST-` de uma conta **real**, o pagador `@testuser.com` é recusado (`403 Payer email forbidden`, causa 4390). Um e-mail comum funciona.
- Em modo teste, o `ticket_url` aponta para `.../sandbox/payments/...` e vem `live_mode=false`.
- A API **não** valida o tipo de documento do pagador nem limita a expiração a 30 dias no sandbox.
- `notification_url` com `localhost` é recusada (400, causa 4020).
- A mesma `X-Idempotency-Key` devolve o mesmo pagamento, sem duplicar a cobrança.

## Licença

[MIT](LICENSE)
