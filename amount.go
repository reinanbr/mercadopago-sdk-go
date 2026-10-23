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

// UnmarshalJSON decodes a decimal number in reais (quoted or not).
func (a *Amount) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	if len(b) == 0 || string(b) == "null" {
		*a = 0
		return nil
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return fmt.Errorf("mercadopagopix: invalid amount %q: %w", b, err)
	}
	*a = Reais(f)
	return nil
}
