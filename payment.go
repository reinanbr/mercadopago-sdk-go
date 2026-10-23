package mercadopagopix

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Status is the status of a payment.
type Status string

const (
	StatusPending     Status = "pending"
	StatusApproved    Status = "approved"
	StatusAuthorized  Status = "authorized"
	StatusInProcess   Status = "in_process"
	StatusInMediation Status = "in_mediation"
	StatusRejected    Status = "rejected"
	StatusCancelled   Status = "cancelled"
	StatusRefunded    Status = "refunded"
	StatusChargedBack Status = "charged_back"
)

// IsFinal reports whether the status will no longer change on its own.
func (s Status) IsFinal() bool {
	switch s {
	case StatusApproved, StatusRejected, StatusCancelled, StatusRefunded, StatusChargedBack:
		return true
	}
	return false
}

// Payer identifies who pays. Email is required by Mercado Pago for PIX.
type Payer struct {
	Email          string          `json:"email,omitempty"`
	FirstName      string          `json:"first_name,omitempty"`
	LastName       string          `json:"last_name,omitempty"`
	Identification *Identification `json:"identification,omitempty"`
}

// Identification is a payer document, e.g. {Type: "CPF", Number: "12345678909"}.
type Identification struct {
	Type   string `json:"type,omitempty"`
	Number string `json:"number,omitempty"`
}

// Payment is a Mercado Pago payment, flattened for PIX use.
type Payment struct {
	ID                int64
	Status            Status
	StatusDetail      string
	Description       string
	ExternalReference string
	PaymentMethodID   string
	CurrencyID        string
	Amount            Amount
	AmountRefunded    Amount
	NotificationURL   string
	LiveMode          bool
	ExecutionID       string
	Metadata          map[string]any

	DateCreated      time.Time
	DateApproved     time.Time
	DateLastUpdated  time.Time
	DateOfExpiration time.Time

	// QRCode is the PIX "copia e cola" payload.
	QRCode string
	// QRCodeBase64 is a PNG image of the QR code, base64-encoded.
	QRCodeBase64 string
	// TicketURL is a Mercado Pago page showing the QR code to the payer.
	TicketURL string

	// Raw holds the full API response, for fields not mapped above.
	Raw json.RawMessage
}

type paymentWire struct {
	ID                int64          `json:"id"`
	Status            Status         `json:"status"`
	StatusDetail      string         `json:"status_detail"`
	Description       string         `json:"description"`
	ExternalReference string         `json:"external_reference"`
	PaymentMethodID   string         `json:"payment_method_id"`
	CurrencyID        string         `json:"currency_id"`
	Amount            Amount         `json:"transaction_amount"`
	AmountRefunded    Amount         `json:"transaction_amount_refunded"`
	NotificationURL   string         `json:"notification_url"`
	LiveMode          bool           `json:"live_mode"`
	Metadata          map[string]any `json:"metadata"`
	DateCreated       mpTime         `json:"date_created"`
	DateApproved      mpTime         `json:"date_approved"`
	DateLastUpdated   mpTime         `json:"date_last_updated"`
	DateOfExpiration  mpTime         `json:"date_of_expiration"`

	PointOfInteraction struct {
		TransactionData struct {
			QRCode       string `json:"qr_code"`
			QRCodeBase64 string `json:"qr_code_base64"`
			TicketURL    string `json:"ticket_url"`
		} `json:"transaction_data"`
	} `json:"point_of_interaction"`

	ChargesExecutionInfo struct {
		InternalExecution struct {
			ExecutionID flexString `json:"execution_id"`
		} `json:"internal_execution"`
	} `json:"charges_execution_info"`
}

// UnmarshalJSON decodes a Mercado Pago payment response. Missing or null
// nested objects are tolerated.
func (p *Payment) UnmarshalJSON(b []byte) error {
	var w paymentWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	td := w.PointOfInteraction.TransactionData
	*p = Payment{
		ID:                w.ID,
		Status:            w.Status,
		StatusDetail:      w.StatusDetail,
		Description:       w.Description,
		ExternalReference: w.ExternalReference,
		PaymentMethodID:   w.PaymentMethodID,
		CurrencyID:        w.CurrencyID,
		Amount:            w.Amount,
		AmountRefunded:    w.AmountRefunded,
		NotificationURL:   w.NotificationURL,
		LiveMode:          w.LiveMode,
		ExecutionID:       string(w.ChargesExecutionInfo.InternalExecution.ExecutionID),
		Metadata:          w.Metadata,
		DateCreated:       time.Time(w.DateCreated),
		DateApproved:      time.Time(w.DateApproved),
		DateLastUpdated:   time.Time(w.DateLastUpdated),
		DateOfExpiration:  time.Time(w.DateOfExpiration),
		QRCode:            td.QRCode,
		QRCodeBase64:      td.QRCodeBase64,
		TicketURL:         td.TicketURL,
		Raw:               append(json.RawMessage(nil), b...),
	}
	return nil
}

// IsPaid reports whether the payment was approved.
func (p *Payment) IsPaid() bool { return p.Status == StatusApproved }

// IsExpired reports whether the payment is still pending past its expiration.
func (p *Payment) IsExpired() bool {
	return p.Status == StatusPending && !p.DateOfExpiration.IsZero() && time.Now().After(p.DateOfExpiration)
}

// TimeToExpiration returns how long until the PIX expires (0 if expired or
// no expiration is set).
func (p *Payment) TimeToExpiration() time.Duration {
	if p.DateOfExpiration.IsZero() {
		return 0
	}
	return max(time.Until(p.DateOfExpiration), 0)
}

// QRCodePNG returns the decoded QR code image.
func (p *Payment) QRCodePNG() ([]byte, error) {
	if p.QRCodeBase64 == "" {
		return nil, fmt.Errorf("mercadopagopix: payment %d has no QR code image", p.ID)
	}
	return base64.StdEncoding.DecodeString(p.QRCodeBase64)
}

// String returns a human-readable summary (without the QR code image).
func (p *Payment) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Payment %d\n", p.ID)
	fmt.Fprintf(&b, "  Status:      %s (%s)\n", p.Status, p.StatusDetail)
	fmt.Fprintf(&b, "  Amount:      %s\n", p.Amount)
	fmt.Fprintf(&b, "  Description: %s\n", p.Description)
	if p.ExternalReference != "" {
		fmt.Fprintf(&b, "  Reference:   %s\n", p.ExternalReference)
	}
	fmt.Fprintf(&b, "  Created:     %s\n", fmtTime(p.DateCreated))
	fmt.Fprintf(&b, "  Expires:     %s (in %s)\n", fmtTime(p.DateOfExpiration), p.TimeToExpiration().Round(time.Second))
	if p.TicketURL != "" {
		fmt.Fprintf(&b, "  Ticket URL:  %s\n", p.TicketURL)
	}
	if p.QRCode != "" {
		fmt.Fprintf(&b, "  QR Code:     %s\n", p.QRCode)
	}
	return b.String()
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

// Refund is a (total or partial) refund of a payment.
type Refund struct {
	ID          int64     `json:"id"`
	PaymentID   int64     `json:"payment_id"`
	Amount      Amount    `json:"amount"`
	Status      string    `json:"status"`
	DateCreated time.Time `json:"-"`
}

func (r *Refund) UnmarshalJSON(b []byte) error {
	type alias Refund
	var w struct {
		alias
		DateCreated mpTime `json:"date_created"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*r = Refund(w.alias)
	r.DateCreated = time.Time(w.DateCreated)
	return nil
}

// mpTime parses Mercado Pago timestamps, tolerating null and "".
type mpTime time.Time

func (t *mpTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*t = mpTime{}
		return nil
	}
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("mercadopagopix: invalid time %q: %w", s, err)
	}
	*t = mpTime(v)
	return nil
}
