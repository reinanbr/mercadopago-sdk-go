# Relatório de testes de integração (sandbox Mercado Pago)

✅ **Todos os testes passaram**

- **Data:** 23/10/2026 14:44:43 -03
- **Go:** go1.26.8 (linux/amd64)
- **Comando:** `go test -json -count=1 -tags integration -run Integration .`

## Resumo

| Total | Passou | Falhou | Pulado | Duração |
|---:|---:|---:|---:|---:|
| 16 | 16 | 0 | 0 | 36.16s |

## Resultados

| # | Teste | Status | Duração |
|---:|---|---|---:|
| 1 | `TestIntegrationAccount` | ✅ passou | 1.61s |
| 2 | `TestIntegrationCreatePix` | ✅ passou | 2.46s |
| 3 | `TestIntegrationIdempotency` | ✅ passou | 3.56s |
| 4 | `TestIntegrationGetPayment` | ✅ passou | 2.80s |
| 5 | `TestIntegrationSearchByExternalReference` | ✅ passou | 2.58s |
| 6 | `TestIntegrationCancelPayment` | ✅ passou | 2.77s |
| 7 | `TestIntegrationWaitForPaymentTimeout` | ✅ passou | 5.29s |
| 8 | `TestIntegrationNotFound` | ✅ passou | 0.36s |
| 9 | `TestIntegrationInvalidToken` | ✅ passou | 0.37s |
| 10 | `TestIntegrationAPIValidationError` | ✅ passou | 0.81s |
| 11 | `TestIntegrationSearchFiltersAndPaging` | ✅ passou | 3.38s |
| 12 | `TestIntegrationUpdateRestrictions` | ✅ passou | 3.46s |
| 13 | `TestIntegrationRefundPendingPayment` | ✅ passou | 3.67s |
| 14 | `TestIntegrationIntegratorHeadersAndRawDo` | ✅ passou | 0.36s |
| 15 | `TestIntegrationRequestIDOnErrors` | ✅ passou | 0.36s |
| 16 | `TestIntegrationOrderData` | ✅ passou | 2.09s |

## Detalhes

### TestIntegrationAccount — ✅ passou

```text
integration_test.go:145: account site=MLB test_user=false tags=[user_product_seller messages_as_seller normal]
integration_test.go:149: payer email used by the suite: comprador.teste.mpgo@gmail.com
```

### TestIntegrationCreatePix — ✅ passou

```text
integration_test.go:159: payment id=1354113987 status=pending detail=pending_waiting_transfer amount=R$ 2,00 live_mode=false
integration_test.go:159: created=2026-10-23T13:44:10-04:00 expires=2026-10-23T14:14:09-04:00 (in 29m58s)
integration_test.go:159: qr_code=00020126580014br.gov.bcb.pix0136b76aa9c2…(164 chars) qr_png_base64=3700 chars
integration_test.go:159: ticket_url=https://www.mercadopago.com.br/sandbox/payments/1354113987/ticket
```

### TestIntegrationIdempotency — ✅ passou

```text
integration_test.go:209: first id=1354113991 second id=1354113991 (same idempotency key)
```

### TestIntegrationGetPayment — ✅ passou

```text
integration_test.go:223: payment id=1354095429 status=pending detail=pending_waiting_transfer amount=R$ 2,00 live_mode=false
integration_test.go:223: created=2026-10-23T13:44:15-04:00 expires=2026-10-23T14:14:15-04:00 (in 29m58s)
integration_test.go:223: qr_code=00020126580014br.gov.bcb.pix0136b76aa9c2…(164 chars) qr_png_base64=3716 chars
integration_test.go:223: ticket_url=https://www.mercadopago.com.br/sandbox/payments/1354095429/ticket
```

### TestIntegrationSearchByExternalReference — ✅ passou

```text
integration_test.go:250: found 1 result(s) after 1 attempt(s)
```

### TestIntegrationCancelPayment — ✅ passou

```text
integration_test.go:271: payment id=1354114001 status=cancelled detail=by_collector amount=R$ 2,00 live_mode=false
integration_test.go:271: created=2026-10-23T13:44:21-04:00 expires=2026-10-23T14:14:20-04:00 (in 29m58s)
integration_test.go:271: qr_code=00020126580014br.gov.bcb.pix0136b76aa9c2…(164 chars) qr_png_base64=3744 chars
integration_test.go:271: ticket_url=https://www.mercadopago.com.br/sandbox/payments/1354114001/ticket
integration_test.go:282: WaitForPayment returned in 524ms
```

### TestIntegrationWaitForPaymentTimeout — ✅ passou

```text
integration_test.go:297: unpaid PIX still pending after timeout, as expected
```

### TestIntegrationNotFound — ✅ passou

```text
integration_test.go:303: api error: status=404 code="not_found" message="Payment not found" causes=[{2000 Payment not found}]
```

### TestIntegrationInvalidToken — ✅ passou

```text
integration_test.go:313: api error: status=401 code="" message="invalid access token" causes=[]
```

### TestIntegrationAPIValidationError — ✅ passou

```text
integration_test.go:332: api error: status=400 code="bad_request" message="notificaction_url attribute must be url valid" causes=[{4020 notificaction_url attribute must be url valid}]
```

### TestIntegrationSearchFiltersAndPaging — ✅ passou

```text
integration_test.go:360: page: 1 result(s), paging={Total:1 Limit:1 Offset:0}
integration_test.go:373: AllPayments yielded [1354095437] and stopped at paging.total
```

### TestIntegrationUpdateRestrictions — ✅ passou

```text
integration_test.go:402: update metadata           -> 403 The caller is not authorized to access this resource
integration_test.go:402: update external_reference -> 401 Unauthorized access
integration_test.go:402: update date_of_expiration -> 400 date_of_expiration attribute is not allowed
```

### TestIntegrationRefundPendingPayment — ✅ passou

```text
integration_test.go:410: api error: status=400 code="bad_request" message="Transaction Intent must be in processed state to start post-processing" causes=[{120010 Transaction Intent must be in processed state to start post-processing}]
integration_test.go:417: api error: status=403 code="" message="At least one policy returned UNAUTHORIZED." causes=[]
```

### TestIntegrationIntegratorHeadersAndRawDo — ✅ passou

```text
integration_test.go:441: identification types: [CPF CNPJ]
```

### TestIntegrationRequestIDOnErrors — ✅ passou

```text
integration_test.go:454: request id of the 404: "00938e8b-2a25-4bac-9c75-c3e98970b92e"
```

### TestIntegrationOrderData — ✅ passou

```text
integration_test.go:485: payment id=1354095445 status=pending detail=pending_waiting_transfer amount=R$ 3,50 live_mode=false
integration_test.go:485: created=2026-10-23T13:44:42-04:00 expires=2026-10-23T14:14:41-04:00 (in 29m59s)
integration_test.go:485: qr_code=00020126580014br.gov.bcb.pix0136b76aa9c2…(164 chars) qr_png_base64=3752 chars
integration_test.go:485: ticket_url=https://www.mercadopago.com.br/sandbox/payments/1354095445/ticket
```

