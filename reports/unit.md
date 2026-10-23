# Relatório de testes unitários

✅ **Todos os testes passaram**

- **Data:** 23/10/2026 11:47:44 -03
- **Go:** go1.26.8 (linux/amd64)
- **Comando:** `go test -json -count=1 -coverprofile /tmp/cover-2696085977.out .`

## Resumo

| Total | Passou | Falhou | Pulado | Duração |
|---:|---:|---:|---:|---:|
| 35 | 35 | 0 | 0 | 680ms |

## Resultados

| # | Teste | Status | Duração |
|---:|---|---|---:|
| 1 | `TestCallOptionsAndHeaders` | ✅ passou | 0.00s |
| 2 | `TestIdempotencyKeyTooLong` | ✅ passou | 0.00s |
| 3 | `TestRetryPolicy` | ✅ passou | 0.05s |
| 4 | `TestRetryOnlyListedStatuses` | ✅ passou | 0.00s |
| 5 | `TestBackoff` | ✅ passou | 0.00s |
| 6 | `TestEmptyResponseBody` | ✅ passou | 0.00s |
| 7 | `TestDoRaw` | ✅ passou | 0.00s |
| 8 | `TestSearchParams` | ✅ passou | 0.00s |
| 9 | `TestAllPaymentsPaginates` | ✅ passou | 0.00s |
| 10 | `TestAllPaymentsYieldsError` | ✅ passou | 0.00s |
| 11 | `TestRefundEndpoints` | ✅ passou | 0.00s |
| 12 | `TestUpdatePayment` | ✅ passou | 0.00s |
| 13 | `TestSignatureFailureReasons` | ✅ passou | 0.00s |
| 14 | `TestSignatureVersionsAndCase` | ✅ passou | 0.00s |
| 15 | `TestAsyncWebhookHandler` | ✅ passou | 0.00s |
| 16 | `TestSyncWebhookHandlerErrors` | ✅ passou | 0.00s |
| 17 | `TestReadEnvFile` | ✅ passou | 0.00s |
| 18 | `TestLoadEnvFileDoesNotOverride` | ✅ passou | 0.00s |
| 19 | `TestNewFromEnvFallbacks` | ✅ passou | 0.00s |
| 20 | `TestMe` | ✅ passou | 0.00s |
| 21 | `TestAPIErrorHints` | ✅ passou | 0.00s |
| 22 | `TestNewRequiresToken` | ✅ passou | 0.00s |
| 23 | `TestCreatePix` | ✅ passou | 0.00s |
| 24 | `TestCreatePixValidation` | ✅ passou | 0.00s |
| 25 | `TestAPIError` | ✅ passou | 0.00s |
| 26 | `TestRetryOn5xxKeepsIdempotencyKey` | ✅ passou | 0.00s |
| 27 | `TestCancelAndRefund` | ✅ passou | 0.00s |
| 28 | `TestSearchByExternalReference` | ✅ passou | 0.00s |
| 29 | `TestWaitForPayment` | ✅ passou | 0.00s |
| 30 | `TestWaitForPaymentTimeoutKeepsLastPayment` | ✅ passou | 0.21s |
| 31 | `TestWaitForPaymentExpired` | ✅ passou | 0.00s |
| 32 | `TestAmount` | ✅ passou | 0.00s |
| 33 | `TestVerifySignature` | ✅ passou | 0.00s |
| 34 | `TestWebhookHandler` | ✅ passou | 0.00s |
| 35 | `TestParseWebhookLegacyIPN` | ✅ passou | 0.00s |

## Cobertura

**Cobertura total: 87.5%**

<details><summary>Por função</summary>

| Arquivo | Função | Cobertura |
|---|---|---:|
| `amount.go:17` | `Reais` | 100.0% |
| `amount.go:20` | `Cents` | 100.0% |
| `amount.go:23` | `Cents` | 0.0% |
| `amount.go:26` | `Float64` | 0.0% |
| `amount.go:29` | `String` | 100.0% |
| `amount.go:48` | `MarshalJSON` | 100.0% |
| `amount.go:59` | `UnmarshalJSON` | 88.9% |
| `client.go:66` | `DefaultRetryPolicy` | 100.0% |
| `client.go:80` | `WithHTTPClient` | 0.0% |
| `client.go:83` | `WithTimeout` | 0.0% |
| `client.go:92` | `WithBaseURL` | 100.0% |
| `client.go:97` | `WithUserAgent` | 0.0% |
| `client.go:100` | `WithRetryPolicy` | 100.0% |
| `client.go:104` | `WithRetries` | 100.0% |
| `client.go:109` | `WithHeader` | 100.0% |
| `client.go:113` | `WithIntegratorID` | 100.0% |
| `client.go:117` | `WithPlatformID` | 100.0% |
| `client.go:120` | `WithCorporationID` | 100.0% |
| `client.go:123` | `New` | 100.0% |
| `client.go:148` | `NewFromEnv` | 100.0% |
| `client.go:158` | `TestMode` | 100.0% |
| `client.go:171` | `WithAccessToken` | 100.0% |
| `client.go:178` | `WithIdempotencyKey` | 100.0% |
| `client.go:183` | `WithCallHeader` | 100.0% |
| `client.go:208` | `Do` | 100.0% |
| `client.go:216` | `do` | 87.5% |
| `client.go:267` | `attempt` | 88.2% |
| `client.go:319` | `backoff` | 90.9% |
| `client.go:338` | `parseRetryAfter` | 85.7% |
| `client.go:351` | `pathID` | 100.0% |
| `client.go:354` | `newIdempotencyKey` | 100.0% |
| `env.go:12` | `LoadEnvFile` | 85.7% |
| `env.go:28` | `ReadEnvFile` | 95.5% |
| `errors.go:44` | `Error` | 100.0% |
| `errors.go:75` | `HasCause` | 100.0% |
| `errors.go:85` | `Is` | 75.0% |
| `errors.go:95` | `parseAPIError` | 100.0% |
| `errors.go:120` | `UnmarshalJSON` | 72.7% |
| `payment.go:27` | `IsFinal` | 100.0% |
| `payment.go:116` | `UnmarshalJSON` | 83.3% |
| `payment.go:149` | `IsPaid` | 100.0% |
| `payment.go:152` | `IsExpired` | 100.0% |
| `payment.go:158` | `TimeToExpiration` | 66.7% |
| `payment.go:166` | `QRCodePNG` | 66.7% |
| `payment.go:174` | `String` | 0.0% |
| `payment.go:194` | `fmtTime` | 0.0% |
| `payment.go:210` | `UnmarshalJSON` | 85.7% |
| `payment.go:227` | `UnmarshalJSON` | 88.9% |
| `pix.go:38` | `validate` | 100.0% |
| `pix.go:77` | `CreatePix` | 92.3% |
| `pix.go:108` | `GetPayment` | 100.0% |
| `pix.go:119` | `UpdatePayment` | 75.0% |
| `pix.go:128` | `CancelPayment` | 100.0% |
| `pix.go:134` | `Refund` | 80.0% |
| `pix.go:152` | `ListRefunds` | 100.0% |
| `pix.go:159` | `GetRefund` | 80.0% |
| `pix.go:170` | `SearchByExternalReference` | 75.0% |
| `pix.go:183` | `WaitForPayment` | 88.9% |
| `pix.go:223` | `IsTestUser` | 75.0% |
| `pix.go:234` | `Me` | 75.0% |
| `search.go:34` | `values` | 100.0% |
| `search.go:63` | `orDefault` | 66.7% |
| `search.go:70` | `orDefaultInt` | 100.0% |
| `search.go:91` | `SearchPayments` | 100.0% |
| `search.go:108` | `AllPayments` | 100.0% |
| `webhook.go:39` | `DataID` | 100.0% |
| `webhook.go:42` | `PaymentID` | 100.0% |
| `webhook.go:71` | `Error` | 0.0% |
| `webhook.go:75` | `Is` | 100.0% |
| `webhook.go:97` | `Validate` | 97.9% |
| `webhook.go:172` | `signManifest` | 100.0% |
| `webhook.go:190` | `VerifySignature` | 100.0% |
| `webhook.go:197` | `ParseWebhook` | 75.0% |
| `webhook.go:205` | `parseWebhook` | 91.7% |
| `webhook.go:268` | `NewWebhookHandler` | 90.7% |
| `webhook.go:334` | `WebhookHandler` | 100.0% |

</details>
