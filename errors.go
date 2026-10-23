package mercadopagopix

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	// ErrMissingToken is returned when no access token is provided.
	ErrMissingToken = errors.New("mercadopagopix: missing access token")
	// ErrInvalidRequest wraps client-side validation failures.
	ErrInvalidRequest = errors.New("mercadopagopix: invalid request")
	// ErrNotFound matches API errors with HTTP status 404.
	ErrNotFound = errors.New("mercadopagopix: not found")
	// ErrUnauthorized matches API errors with HTTP status 401 or 403.
	ErrUnauthorized = errors.New("mercadopagopix: unauthorized")
	// ErrExpired is returned by WaitForPayment when the PIX expired unpaid.
	ErrExpired = errors.New("mercadopagopix: payment expired")
	// ErrInvalidSignature is returned when a webhook signature does not match.
	ErrInvalidSignature = errors.New("mercadopagopix: invalid webhook signature")
)

// APIError is an error response returned by the Mercado Pago API.
type APIError struct {
	StatusCode int     `json:"-"`
	Code       string  `json:"error"`
	Message    string  `json:"message"`
	Causes     []Cause `json:"-"`
	Body       []byte  `json:"-"`
	// RequestID is the X-Request-Id of the response; quote it when
	// contacting Mercado Pago support.
	RequestID string `json:"-"`
}

// Cause is a detailed reason attached to an APIError.
type Cause struct {
	Code        flexString `json:"code"`
	Description string     `json:"description"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mercadopagopix: api error %d", e.StatusCode)
	if e.Code != "" {
		fmt.Fprintf(&b, " (%s)", e.Code)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	for _, c := range e.Causes {
		fmt.Fprintf(&b, "; cause %s: %s", c.Code, c.Description)
		if h, ok := causeHints[string(c.Code)]; ok {
			fmt.Fprintf(&b, " (hint: %s)", h)
		}
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request-id %s]", e.RequestID)
	}
	return b.String()
}

// causeHints explains common, non-obvious Mercado Pago cause codes.
var causeHints = map[string]string{
	"4390":  "payer e-mail not allowed; it must differ from the seller's, and with TEST credentials of a regular account use an ordinary address, not @testuser.com",
	"13253": "the account has no PIX key registered; create one in the Mercado Pago app",
	"4020":  "notification_url must be a public http(s) URL; localhost and private addresses are rejected",
	"4243":  "this attribute cannot be changed; a PIX payment only accepts status=cancelled",
	"2000":  "payment not found; check the ID and that it belongs to this account",
}

// HasCause reports whether the error includes the given cause code.
func (e *APIError) HasCause(code string) bool {
	for _, c := range e.Causes {
		if string(c.Code) == code {
			return true
		}
	}
	return false
}

// Is lets errors.Is match ErrNotFound and ErrUnauthorized.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
	}
	return false
}

func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{StatusCode: status, Body: body}
	var wire struct {
		Code    string          `json:"error"`
		Message string          `json:"message"`
		Cause   json.RawMessage `json:"cause"`
	}
	if json.Unmarshal(body, &wire) != nil {
		e.Message = strings.TrimSpace(string(body))
		return e
	}
	e.Code, e.Message = wire.Code, wire.Message
	// "cause" may be an array or a single object.
	if json.Unmarshal(wire.Cause, &e.Causes) != nil {
		var c Cause
		if json.Unmarshal(wire.Cause, &c) == nil && (c.Code != "" || c.Description != "") {
			e.Causes = []Cause{c}
		}
	}
	return e
}

// flexString accepts JSON strings and numbers.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	*s = flexString(b)
	return nil
}
