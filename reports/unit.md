# Relatório de testes unitários

✅ **Todos os testes passaram**

- **Data:** 23/10/2026 14:44:07 -03
- **Go:** go1.26.8 (linux/amd64)
- **Comando:** `go test -json -count=1 -coverprofile /tmp/cover-3553227468.out .`

## Resumo

| Total | Passou | Falhou | Pulado | Duração |
|---:|---:|---:|---:|---:|
| 44 | 44 | 0 | 0 | 560ms |

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
| 17 | `TestParseAmount` | ✅ passou | 0.00s |
| 18 | `TestAmountUnmarshalExact` | ✅ passou | 0.00s |
| 19 | `TestDocuments` | ✅ passou | 0.00s |
| 20 | `TestCreatePixOrderData` | ✅ passou | 0.00s |
| 21 | `TestCreatePixOrderValidation` | ✅ passou | 0.00s |
| 22 | `TestPaymentOrderHelpers` | ✅ passou | 0.00s |
| 23 | `TestWithLogger` | ✅ passou | 0.00s |
| 24 | `TestWebhookReceiverConcurrencyAndWait` | ✅ passou | 0.02s |
| 25 | `TestSignWebhook` | ✅ passou | 0.00s |
| 26 | `TestReadEnvFile` | ✅ passou | 0.00s |
| 27 | `TestLoadEnvFileDoesNotOverride` | ✅ passou | 0.00s |
| 28 | `TestNewFromEnvFallbacks` | ✅ passou | 0.00s |
| 29 | `TestMe` | ✅ passou | 0.00s |
| 30 | `TestAPIErrorHints` | ✅ passou | 0.00s |
| 31 | `TestNewRequiresToken` | ✅ passou | 0.00s |
| 32 | `TestCreatePix` | ✅ passou | 0.00s |
| 33 | `TestCreatePixValidation` | ✅ passou | 0.00s |
| 34 | `TestAPIError` | ✅ passou | 0.00s |
| 35 | `TestRetryOn5xxKeepsIdempotencyKey` | ✅ passou | 0.00s |
| 36 | `TestCancelAndRefund` | ✅ passou | 0.00s |
| 37 | `TestSearchByExternalReference` | ✅ passou | 0.00s |
| 38 | `TestWaitForPayment` | ✅ passou | 0.00s |
| 39 | `TestWaitForPaymentTimeoutKeepsLastPayment` | ✅ passou | 0.21s |
| 40 | `TestWaitForPaymentExpired` | ✅ passou | 0.00s |
| 41 | `TestAmount` | ✅ passou | 0.00s |
| 42 | `TestVerifySignature` | ✅ passou | 0.00s |
| 43 | `TestWebhookHandler` | ✅ passou | 0.00s |
| 44 | `TestParseWebhookLegacyIPN` | ✅ passou | 0.00s |

## Cobertura

**Cobertura total: 87.7%**

<details><summary>Por função</summary>

| Arquivo | Função | Cobertura |
|---|---|---:|
| `amount.go:17` | `Reais` | 100.0% |
| `amount.go:20` | `Cents` | 100.0% |
| `amount.go:23` | `Cents` | 0.0% |
| `amount.go:26` | `Float64` | 0.0% |
| `amount.go:29` | `String` | 100.0% |
| `amount.go:48` | `MarshalJSON` | 100.0% |
| `amount.go:61` | `UnmarshalJSON` | 100.0% |
| `amount.go:87` | `ParseAmount` | 100.0% |
| `amount.go:124` | `decimalCents` | 94.4% |
| `client.go:68` | `DefaultRetryPolicy` | 100.0% |
| `client.go:82` | `WithHTTPClient` | 0.0% |
| `client.go:85` | `WithTimeout` | 0.0% |
| `client.go:94` | `WithBaseURL` | 100.0% |
| `client.go:99` | `WithUserAgent` | 0.0% |
| `client.go:102` | `WithRetryPolicy` | 100.0% |
| `client.go:106` | `WithRetries` | 100.0% |
| `client.go:113` | `WithLogger` | 100.0% |
| `client.go:116` | `WithHeader` | 100.0% |
| `client.go:120` | `WithIntegratorID` | 100.0% |
| `client.go:124` | `WithPlatformID` | 100.0% |
| `client.go:127` | `WithCorporationID` | 100.0% |
| `client.go:130` | `New` | 100.0% |
| `client.go:155` | `NewFromEnv` | 100.0% |
| `client.go:165` | `TestMode` | 100.0% |
| `client.go:178` | `WithAccessToken` | 100.0% |
| `client.go:185` | `WithIdempotencyKey` | 100.0% |
| `client.go:190` | `WithCallHeader` | 100.0% |
| `client.go:215` | `Do` | 100.0% |
| `client.go:223` | `do` | 87.5% |
| `client.go:274` | `attempt` | 89.5% |
| `client.go:329` | `log` | 87.5% |
| `client.go:343` | `backoff` | 90.9% |
| `client.go:362` | `parseRetryAfter` | 85.7% |
| `client.go:375` | `pathID` | 100.0% |
| `client.go:378` | `newIdempotencyKey` | 100.0% |
| `document.go:10` | `CPF` | 100.0% |
| `document.go:16` | `CNPJ` | 100.0% |
| `document.go:22` | `ValidCPF` | 100.0% |
| `document.go:32` | `ValidCNPJ` | 100.0% |
| `document.go:42` | `validate` | 100.0% |
| `document.go:60` | `normalized` | 100.0% |
| `document.go:72` | `checkDigit` | 100.0% |
| `document.go:85` | `cnpjDigit` | 100.0% |
| `document.go:100` | `onlyDigits` | 100.0% |
| `document.go:109` | `allSame` | 100.0% |
| `env.go:12` | `LoadEnvFile` | 85.7% |
| `env.go:28` | `ReadEnvFile` | 95.5% |
| `errors.go:47` | `Error` | 100.0% |
| `errors.go:78` | `HasCause` | 100.0% |
| `errors.go:88` | `Is` | 75.0% |
| `errors.go:98` | `parseAPIError` | 100.0% |
| `errors.go:123` | `UnmarshalJSON` | 72.7% |
| `payment.go:27` | `IsFinal` | 100.0% |
| `payment.go:78` | `Total` | 100.0% |
| `payment.go:81` | `ItemsTotal` | 100.0% |
| `payment.go:181` | `UnmarshalJSON` | 83.3% |
| `payment.go:220` | `IsPaid` | 100.0% |
| `payment.go:224` | `IsPayable` | 100.0% |
| `payment.go:230` | `RefundableAmount` | 100.0% |
| `payment.go:241` | `CheckOrder` | 80.0% |
| `payment.go:254` | `IsExpired` | 100.0% |
| `payment.go:260` | `TimeToExpiration` | 66.7% |
| `payment.go:268` | `QRCodePNG` | 66.7% |
| `payment.go:276` | `String` | 0.0% |
| `payment.go:296` | `fmtTime` | 0.0% |
| `payment.go:312` | `UnmarshalJSON` | 85.7% |
| `payment.go:329` | `UnmarshalJSON` | 88.9% |
| `pix.go:46` | `validate` | 100.0% |
| `pix.go:111` | `additionalInfo` | 100.0% |
| `pix.go:130` | `CreatePix` | 93.3% |
| `pix.go:164` | `GetPayment` | 100.0% |
| `pix.go:175` | `UpdatePayment` | 75.0% |
| `pix.go:184` | `CancelPayment` | 100.0% |
| `pix.go:190` | `Refund` | 80.0% |
| `pix.go:208` | `ListRefunds` | 100.0% |
| `pix.go:215` | `GetRefund` | 80.0% |
| `pix.go:226` | `SearchByExternalReference` | 75.0% |
| `pix.go:242` | `FindByExternalReference` | 0.0% |
| `pix.go:263` | `GetOrCreatePix` | 16.7% |
| `pix.go:288` | `WaitForPayment` | 88.9% |
| `pix.go:328` | `IsTestUser` | 75.0% |
| `pix.go:339` | `Me` | 75.0% |
| `search.go:34` | `values` | 100.0% |
| `search.go:63` | `orDefault` | 66.7% |
| `search.go:70` | `orDefaultInt` | 100.0% |
| `search.go:91` | `SearchPayments` | 100.0% |
| `search.go:108` | `AllPayments` | 100.0% |
| `webhook.go:40` | `DataID` | 100.0% |
| `webhook.go:43` | `PaymentID` | 100.0% |
| `webhook.go:72` | `Error` | 0.0% |
| `webhook.go:76` | `Is` | 100.0% |
| `webhook.go:98` | `Validate` | 97.9% |
| `webhook.go:173` | `signManifest` | 100.0% |
| `webhook.go:189` | `SignWebhook` | 100.0% |
| `webhook.go:198` | `VerifySignature` | 100.0% |
| `webhook.go:205` | `ParseWebhook` | 75.0% |
| `webhook.go:213` | `parseWebhook` | 91.7% |
| `webhook.go:291` | `NewWebhookHandler` | 100.0% |
| `webhook.go:302` | `acquire` | 100.0% |
| `webhook.go:315` | `release` | 100.0% |
| `webhook.go:323` | `report` | 100.0% |
| `webhook.go:329` | `process` | 83.3% |
| `webhook.go:340` | `ServeHTTP` | 91.2% |
| `webhook.go:390` | `Wait` | 77.8% |
| `webhook.go:408` | `WebhookHandler` | 100.0% |

</details>
