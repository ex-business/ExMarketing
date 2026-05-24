package decimal

import (
	"errors"
	"math/big"
	"strings"
)

type Decimal struct {
	value *big.Rat
}

var ErrInvalidDecimal = errors.New("invalid decimal")

func Parse(s string) (Decimal, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return Decimal{}, ErrInvalidDecimal
	}

	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return Decimal{}, ErrInvalidDecimal
	}
	return Decimal{value: rat}, nil
}

func MustParse(s string) Decimal {
	value, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return value
}

func Zero() Decimal {
	return Decimal{value: new(big.Rat)}
}

func (d Decimal) String() string {
	if d.value == nil {
		return "0.0000"
	}
	return d.value.FloatString(4)
}

func (d Decimal) Add(other Decimal) Decimal {
	return Decimal{value: new(big.Rat).Add(d.rat(), other.rat())}
}

func (d Decimal) Mul(other Decimal) Decimal {
	return Decimal{value: new(big.Rat).Mul(d.rat(), other.rat())}
}

func (d Decimal) Cmp(other Decimal) int {
	return d.rat().Cmp(other.rat())
}

func (d Decimal) Min(other Decimal) Decimal {
	if d.Cmp(other) <= 0 {
		return d
	}
	return other
}

func (d Decimal) rat() *big.Rat {
	if d.value == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d.value)
}
