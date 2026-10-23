# mercadopago-sdk-go

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

### Webhooks

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
    mercadopagopix.WithRetries(3, time.Second), // padrão: 2 tentativas extras, 500ms com backoff
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
