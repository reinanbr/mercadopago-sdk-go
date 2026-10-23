package mercadopagopix

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sign(secret, dataID, requestID, ts string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("id:" + dataID + ";request-id:" + requestID + ";ts:" + ts + ";"))
	return "ts=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := sign("s3cret", "123", "req-1", ts)
	if err := VerifySignature("s3cret", sig, "req-1", "123", time.Minute); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySignature("wrong", sig, "req-1", "123", 0); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("wrong secret: %v", err)
	}
	if err := VerifySignature("s3cret", sig, "req-1", "124", 0); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("tampered id: %v", err)
	}
	if err := VerifySignature("s3cret", "garbage", "req-1", "123", 0); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("malformed header: %v", err)
	}
	old := sign("s3cret", "123", "req-1", "1000")
	if err := VerifySignature("s3cret", old, "req-1", "123", time.Minute); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("stale timestamp: %v", err)
	}
}

func TestWebhookHandler(t *testing.T) {
	api := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/555" {
			t.Errorf("unexpected %s", r.URL.Path)
		}
		io.WriteString(w, `{"id":555,"status":"approved"}`)
	})
	var got *Payment
	h := api.WebhookHandler("s3cret", func(_ context.Context, p *Payment) error {
		got = p
		return nil
	})

	body := `{"action":"payment.updated","type":"payment","data":{"id":"555"}}`
	newReq := func(sig string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/webhook?data.id=555&type=payment", strings.NewReader(body))
		r.Header.Set("X-Signature", sig)
		r.Header.Set("X-Request-Id", "req-9")
		return r
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(sign("s3cret", "555", "req-9", "1700000000")))
	if rec.Code != http.StatusOK || got == nil || !got.IsPaid() {
		t.Fatalf("code=%d payment=%v", rec.Code, got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(sign("other", "555", "req-9", "1700000000")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad signature: code=%d", rec.Code)
	}
}

func TestParseWebhookLegacyIPN(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/webhook?topic=payment&id=42", nil)
	n, err := ParseWebhook(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := n.PaymentID(); err != nil || id != 42 {
		t.Errorf("PaymentID = %d, %v", id, err)
	}
}
