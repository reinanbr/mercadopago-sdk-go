# Changelog

Todas as mudanças relevantes deste projeto. O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/)
e o projeto usa [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [1.1.1] - 2026-10-23

### Adicionado
- Licença MIT.
- Badges do pkg.go.dev, Go Report Card e licença no README.

### Alterado
- Repositório público; módulo disponível via `go get github.com/reinanbr/mercadopago-sdk-go@latest`.
- User-Agent `mercadopago-sdk-go/1.1.1`.

## [1.1.0] - 2026-10-23

Primeira versão publicada. Melhorias inspiradas na engenharia reversa do
[sdk-python oficial](https://github.com/mercadopago/sdk-python) (v3.6.0).

### Adicionado
- `RetryPolicy` completa: `Retry-After`, jitter, `MaxDelay`, `RetryOn` configurável e hook `OnRetry`; `DefaultRetryPolicy()`.
- Opções por chamada (`CallOption`): `WithAccessToken` (marketplace/OAuth), `WithIdempotencyKey`, `WithCallHeader`.
- Opções de cliente `WithTimeout`, `WithHeader`, `WithIntegratorID`, `WithPlatformID`, `WithCorporationID`.
- Busca com filtros (`SearchPayments`, `SearchParams`) e paginação por iterador (`AllPayments`, `iter.Seq2`).
- `ListRefunds`, `GetRefund`, `UpdatePayment` e `Do` (chamada crua a qualquer endpoint).
- `Client.Me` e `Account.IsTestUser`; `Client.TestMode`.
- `LoadEnvFile` / `ReadEnvFile` para arquivos `.env`, sem dependências.
- `WebhookValidator` com motivos tipados (`SignatureError`, `SignatureFailure`), várias versões de assinatura e tolerância de tempo.
- `NewWebhookHandler` com modo assíncrono (resposta dentro dos 22 s exigidos pelo Mercado Pago), `Timeout` e `OnError`.
- `APIError.RequestID` (cabeçalho `X-Request-Id`), `APIError.HasCause` e dicas para as causas 4390, 4020, 4243, 13253 e 2000.
- Testes de integração contra o sandbox (`-tags integration`) e gerador de relatórios Markdown (`cmd/testreport`, `make report`).
- Manual de uso avançado em `docs/manual.html`.

### Alterado
- Requer Go 1.23 (iteradores).
- Timeout padrão de 30 s para 60 s; até 3 retentativas (antes 2), como no SDK oficial.
- Chave de idempotência só em `POST`, reaproveitada em todas as tentativas da mesma chamada.
- `VerifySignature` compara o `data.id` como recebido e, se necessário, em minúsculas.
- E-mail do pagador validado com `net/mail`.
- `WebhookHandler` passa a ser um atalho para `NewWebhookHandler` (síncrono).

### Corrigido
- `WaitForPayment` devolvia `nil` em vez do último pagamento conhecido quando o prazo do contexto estourava durante uma consulta.
- Respostas vazias (204) não geram mais erro de decodificação.

## [1.0.0] - 2026-10-23

Reescrita em Go da biblioteca Python [mercadopagovp](https://github.com/reinanbr/mercadopagovp) (não publicada separadamente).

### Adicionado
- `CreatePix`, `GetPayment`, `CancelPayment`, `Refund`, `SearchByExternalReference`, `WaitForPayment`.
- `Amount` em centavos, erros tipados (`APIError`, `ErrNotFound`, `ErrUnauthorized`, `ErrInvalidRequest`, `ErrExpired`).
- Idempotência automática, retry com backoff e `context.Context` em todas as chamadas.
- Validação de assinatura de webhooks e `WebhookHandler`.

[1.1.1]: https://github.com/reinanbr/mercadopago-sdk-go/releases/tag/v1.1.1
[1.1.0]: https://github.com/reinanbr/mercadopago-sdk-go/releases/tag/v1.1.0
[1.0.0]: https://github.com/reinanbr/mercadopago-sdk-go/blob/v1.1.0/CHANGELOG.md#100---2026-10-23
