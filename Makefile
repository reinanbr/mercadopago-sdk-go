.PHONY: test test-integration report report-unit report-integration

# Unit tests (no network).
test:
	go test -race ./...

# Integration tests against the Mercado Pago sandbox (.env.test, TEST- token).
test-integration:
	go test -tags integration -run Integration -v .

# Markdown reports in reports/.
report: report-unit report-integration

report-unit:
	go run ./cmd/testreport -title "Relatório de testes unitários" -cover -out reports/unit.md

report-integration:
	go run ./cmd/testreport -tags integration -run Integration -title "Relatório de testes de integração (sandbox Mercado Pago)" -out reports/integration.md
