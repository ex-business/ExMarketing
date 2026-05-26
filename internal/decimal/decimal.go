package decimal

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

type Decimal struct {
	value *big.Rat
}

var ErrInvalidDecimal = errors.New("invalid decimal")
var ErrDivideByZero = errors.New("divide by zero")

var decimalPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$`)

func Parse(s string) (Decimal, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return Decimal{}, ErrInvalidDecimal
	}
	if !decimalPattern.MatchString(text) {
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

func (d Decimal) Sub(other Decimal) Decimal {
	return Decimal{value: new(big.Rat).Sub(d.rat(), other.rat())}
}

func (d Decimal) Mul(other Decimal) Decimal {
	return Decimal{value: new(big.Rat).Mul(d.rat(), other.rat())}
}

func (d Decimal) Div(other Decimal) (Decimal, error) {
	if other.IsZero() {
		return Decimal{}, ErrDivideByZero
	}
	return Decimal{value: new(big.Rat).Quo(d.rat(), other.rat())}, nil
}

func (d Decimal) Cmp(other Decimal) int {
	return d.rat().Cmp(other.rat())
}

func (d Decimal) IsZero() bool {
	return d.Cmp(Zero()) == 0
}

func (d Decimal) IsNegative() bool {
	return d.Cmp(Zero()) < 0
}

func (d Decimal) IsPositive() bool {
	return d.Cmp(Zero()) > 0
}

func (d Decimal) IsInteger() bool {
	return d.rat().Denom().Cmp(big.NewInt(1)) == 0
}

func (d Decimal) Min(other Decimal) Decimal {
	if d.Cmp(other) <= 0 {
		return d
	}
	return other
}

func (d Decimal) FloorDivInt64(other Decimal) (int64, error) {
	if other.IsZero() {
		return 0, ErrDivideByZero
	}
	ratio := new(big.Rat).Quo(d.rat(), other.rat())
	quotient := new(big.Int).Quo(ratio.Num(), ratio.Denom())
	if !quotient.IsInt64() {
		return 0, ErrInvalidDecimal
	}
	return quotient.Int64(), nil
}

func (d Decimal) rat() *big.Rat {
	if d.value == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d.value)
}
