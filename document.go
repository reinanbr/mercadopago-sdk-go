package mercadopagopix

import (
	"fmt"
	"strings"
)

// CPF returns the identification of a person, keeping only the digits of
// number ("123.456.789-09" becomes "12345678909").
func CPF(number string) *Identification {
	return &Identification{Type: "CPF", Number: onlyDigits(number)}
}

// CNPJ returns the identification of a company, keeping only the digits of
// number.
func CNPJ(number string) *Identification {
	return &Identification{Type: "CNPJ", Number: onlyDigits(number)}
}

// ValidCPF reports whether number (with or without punctuation) is a CPF
// with valid check digits.
func ValidCPF(number string) bool {
	d := onlyDigits(number)
	if len(d) != 11 || allSame(d) {
		return false
	}
	return checkDigit(d[:9], 10) == d[9] && checkDigit(d[:10], 11) == d[10]
}

// ValidCNPJ reports whether number (with or without punctuation) is a CNPJ
// with valid check digits.
func ValidCNPJ(number string) bool {
	d := onlyDigits(number)
	if len(d) != 14 || allSame(d) {
		return false
	}
	return cnpjDigit(d[:12]) == d[12] && cnpjDigit(d[:13]) == d[13]
}

// validate checks CPF and CNPJ numbers; other document types are passed
// through to the API unchecked.
func (id *Identification) validate() error {
	switch strings.ToUpper(id.Type) {
	case "CPF":
		if !ValidCPF(id.Number) {
			return fmt.Errorf("%w: invalid CPF", ErrInvalidRequest)
		}
	case "CNPJ":
		if !ValidCNPJ(id.Number) {
			return fmt.Errorf("%w: invalid CNPJ", ErrInvalidRequest)
		}
	case "":
		return fmt.Errorf("%w: identification type is required", ErrInvalidRequest)
	}
	return nil
}

// normalized returns the identification as Mercado Pago expects it: type in
// uppercase and CPF/CNPJ numbers without punctuation.
func (id *Identification) normalized() *Identification {
	if id == nil {
		return nil
	}
	n := Identification{Type: strings.ToUpper(id.Type), Number: id.Number}
	if n.Type == "CPF" || n.Type == "CNPJ" {
		n.Number = onlyDigits(n.Number)
	}
	return &n
}

// checkDigit computes a CPF check digit, weights starting at w.
func checkDigit(d string, w int) byte {
	sum := 0
	for i := range len(d) {
		sum += int(d[i]-'0') * (w - i)
	}
	r := sum * 10 % 11
	if r == 10 {
		r = 0
	}
	return byte('0' + r)
}

// cnpjDigit computes a CNPJ check digit (weights 2..9 from the right).
func cnpjDigit(d string) byte {
	sum, w := 0, 2
	for i := len(d) - 1; i >= 0; i-- {
		sum += int(d[i]-'0') * w
		if w++; w > 9 {
			w = 2
		}
	}
	r := sum % 11
	if r < 2 {
		return '0'
	}
	return byte('0' + 11 - r)
}

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func allSame(s string) bool { return strings.Count(s, s[:1]) == len(s) }
