package mercadopagopix

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Amount is a monetary value in BRL stored as an integer number of centavos,
// avoiding the rounding issues of float64.
type Amount int64

// Reais converts a value in reais (e.g. 2.50) to an Amount, rounding to the
// nearest centavo.
func Reais(v float64) Amount { return Amount(math.Round(v * 100)) }

// Cents builds an Amount from a number of centavos.
func Cents(c int64) Amount { return Amount(c) }

// Cents returns the amount in centavos.
func (a Amount) Cents() int64 { return int64(a) }

// Float64 returns the amount in reais.
func (a Amount) Float64() float64 { return float64(a) / 100 }

// String formats the amount in Brazilian notation, e.g. "R$ 1.234,56".
func (a Amount) String() string {
	sign := ""
	c := int64(a)
	if c < 0 {
		sign = "-"
		c = -c
	}
	intPart := strconv.FormatInt(c/100, 10)
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, b.String(), c%100)
}

// MarshalJSON encodes the amount as a decimal number in reais (e.g. 2.50).
func (a Amount) MarshalJSON() ([]byte, error) {
	c := int64(a)
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return []byte(fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)), nil
}

// UnmarshalJSON decodes a decimal number in reais (quoted or not). Plain
// decimals are parsed exactly; extra decimal places are rounded half away
// from zero.
func (a *Amount) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	if len(b) == 0 || string(b) == "null" {
		*a = 0
		return nil
	}
	s := string(b)
	neg := strings.HasPrefix(s, "-")
	intPart, frac, _ := strings.Cut(strings.TrimPrefix(s, "-"), ".")
	if v, err := decimalCents(intPart, frac, neg); err == nil {
		*a = v
		return nil
	}
	f, err := strconv.ParseFloat(s, 64) // exponent notation
	if err != nil {
		return fmt.Errorf("mercadopagopix: invalid amount %q: %w", b, err)
	}
	*a = Reais(f)
	return nil
}

// ParseAmount parses a value typed by a person or stored as text. Both
// Brazilian ("1.234,56", "R$ 10,00") and plain ("1234.56", "10") notation
// are accepted: when a comma is present it is the decimal separator and
// dots group thousands; otherwise a dot is the decimal separator. More than
// two decimal places is an error, so "1.234" must be written "1.234,00".
func ParseAmount(s string) (Amount, error) {
	in := s
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimSpace(strings.TrimPrefix(s, "-"))
	s = strings.TrimSpace(strings.TrimPrefix(s, "R$"))
	var intPart, frac string
	if strings.Contains(s, ",") {
		var ok bool
		intPart, frac, ok = strings.Cut(s, ",")
		if ok && strings.Contains(frac, ",") {
			return 0, fmt.Errorf("%w: invalid amount %q", ErrInvalidRequest, in)
		}
		if strings.Contains(intPart, ".") {
			groups := strings.Split(intPart, ".")
			for i, g := range groups {
				if (i == 0 && (len(g) == 0 || len(g) > 3)) || (i > 0 && len(g) != 3) {
					return 0, fmt.Errorf("%w: invalid amount %q", ErrInvalidRequest, in)
				}
			}
			intPart = strings.Join(groups, "")
		}
	} else {
		intPart, frac, _ = strings.Cut(s, ".")
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("%w: amount %q has more than two decimal places", ErrInvalidRequest, in)
	}
	v, err := decimalCents(intPart, frac, neg)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid amount %q", ErrInvalidRequest, in)
	}
	return v, nil
}

// decimalCents converts the digits of a decimal number to centavos,
// rounding extra decimal places half away from zero.
func decimalCents(intPart, frac string, neg bool) (Amount, error) {
	if intPart == "" && frac == "" {
		return 0, strconv.ErrSyntax
	}
	for _, part := range []string{intPart, frac} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, strconv.ErrSyntax
			}
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || whole > math.MaxInt64/100-1 {
		return 0, strconv.ErrRange
	}
	padded := frac + "00"
	c := whole*100 + int64(padded[0]-'0')*10 + int64(padded[1]-'0')
	if len(frac) > 2 && frac[2] >= '5' {
		c++
	}
	if neg {
		c = -c
	}
	return Amount(c), nil
}
