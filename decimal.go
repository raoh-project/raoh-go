package raoh

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Decimal is a decimal number as written: an unscaled integer times ten to the
// power of minus its scale, as Java's BigDecimal holds one. 1.20 has the
// unscaled value 120 and the scale 2, and is not the same Decimal as 1.2,
// though they compare equal with Cmp. The zero value is 0.
type Decimal struct {
	unscaled *big.Int
	scale    int
}

// ParseDecimal reads s as Java's new BigDecimal(String) does: an optional
// sign, digits with an optional decimal point, and an optional exponent.
func ParseDecimal(s string) (Decimal, error) {
	fail := errors.New("raoh: not a decimal: " + strconv.Quote(s))
	mantissa, exp, hasExp := s, "", false
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exp, hasExp = s[:i], s[i+1:], true
	}
	sign := ""
	if mantissa != "" && (mantissa[0] == '+' || mantissa[0] == '-') {
		sign, mantissa = mantissa[:1], mantissa[1:]
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := whole + fraction
	if digits == "" || !allBytes(digits, isDigit) {
		return Decimal{}, fail
	}
	scale := len(fraction)
	if hasExp {
		e, err := strconv.Atoi(exp)
		if err != nil {
			return Decimal{}, fail
		}
		scale -= e
	}
	n, _ := new(big.Int).SetString(sign+digits, 10)
	return Decimal{n, scale}, nil
}

// MustDecimal is ParseDecimal that panics when s is not a decimal.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) coefficient() *big.Int {
	if d.unscaled == nil {
		return new(big.Int)
	}
	return d.unscaled
}

// Unscaled returns the unscaled value.
func (d Decimal) Unscaled() *big.Int { return new(big.Int).Set(d.coefficient()) }

// Scale returns the number of digits after the decimal point; a negative
// scale multiplies by a power of ten.
func (d Decimal) Scale() int { return d.scale }

// Sign returns -1, 0 or +1.
func (d Decimal) Sign() int { return d.coefficient().Sign() }

// Rat returns d as a rational number.
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat).SetInt(d.coefficient())
	p := new(big.Rat).SetInt(pow10(abs(d.scale)))
	if d.scale >= 0 {
		return r.Quo(r, p)
	}
	return r.Mul(r, p)
}

// Cmp compares d and e by value: -1, 0 or +1.
func (d Decimal) Cmp(e Decimal) int {
	a, b := aligned(d, e)
	return a.Cmp(b)
}

// String writes d as Java's BigDecimal.toString does: plainly when the scale
// is not negative and the exponent it would take is at least -6, and as
// d.dddE±n otherwise.
func (d Decimal) String() string {
	c := d.coefficient()
	digits := new(big.Int).Abs(c).String()
	sign := ""
	if c.Sign() < 0 {
		sign = "-"
	}
	adjusted := len(digits) - 1 - d.scale
	if d.scale >= 0 && adjusted >= -6 {
		if d.scale == 0 {
			return sign + digits
		}
		if len(digits) > d.scale {
			return sign + digits[:len(digits)-d.scale] + "." + digits[len(digits)-d.scale:]
		}
		return sign + "0." + strings.Repeat("0", d.scale-len(digits)) + digits
	}
	body := digits[:1]
	if len(digits) > 1 {
		body += "." + digits[1:]
	}
	exp := strconv.Itoa(adjusted)
	if adjusted >= 0 {
		exp = "+" + exp
	}
	return sign + body + "E" + exp
}

// MarshalJSON writes d as a JSON number, as String writes it.
func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

// number is d as a JSON number for an issue's metadata.
func (d Decimal) number() json.Number { return json.Number(d.String()) }

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// aligned returns the unscaled values of d and e brought to the same scale.
func aligned(d, e Decimal) (*big.Int, *big.Int) {
	a, b := new(big.Int).Set(d.coefficient()), new(big.Int).Set(e.coefficient())
	switch {
	case d.scale < e.scale:
		a.Mul(a, pow10(e.scale-d.scale))
	case e.scale < d.scale:
		b.Mul(b, pow10(d.scale-e.scale))
	}
	return a, b
}

// DecimalDecoder decodes a number into a [Decimal], exactly as written in JSON
// text.
//
// Null or a missing member is required; any other type is type_mismatch with
// expected number. Bounds appear in issues as JSON numbers and in messages as
// BigDecimal.toString writes them.
type DecimalDecoder struct {
	Decoder[any, Decimal]
	s scalar[Decimal]
}

// DecimalNumber returns a decoder of a number into a Decimal. A Go float given
// as input is read as the shortest decimal that reads back as it.
func DecimalNumber() DecimalDecoder {
	return newDecimal(scalar[Decimal]{read: readDecimal})
}

func newDecimal(s scalar[Decimal]) DecimalDecoder { return DecimalDecoder{s.build(), s} }

func readDecimal(in any) (Decimal, *Issue) {
	if isNull(in) {
		i := NewIssue(CodeRequired)
		return Decimal{}, &i
	}
	mismatch := NewIssue(CodeTypeMismatch).WithMeta("expected", "number")
	if text, ok := numberText(in); ok {
		if d, err := ParseDecimal(text); err == nil {
			return d, nil
		}
		return Decimal{}, &mismatch
	}
	var f float64
	switch n := in.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	default:
		if n, ok := integerOf(in); ok {
			return Decimal{n, 0}, nil
		}
		mismatch = mismatch.WithMeta("actual", kind(in))
		return Decimal{}, &mismatch
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return Decimal{}, &mismatch
	}
	return MustDecimal(strconv.FormatFloat(f, 'g', -1, 64)), nil
}

func (d DecimalDecoder) bound(ok func(Decimal) bool, key string, bounds ...any) DecimalDecoder {
	return newDecimal(d.s.require(ok, func(v Decimal) Issue {
		i := outOfRange(key)
		for n := 0; n < len(bounds); n += 2 {
			i = i.WithMeta(bounds[n].(string), bounds[n+1].(Decimal).number())
		}
		return i.WithMeta("actual", v.number())
	}))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d DecimalDecoder) Message(message string) DecimalDecoder {
	return newDecimal(d.s.message(message))
}

// Min requires at least min: out_of_range with min and actual.
func (d DecimalDecoder) Min(min Decimal) DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Cmp(min) >= 0 }, KeyOutOfRangeMinimum, "min", min)
}

// Max allows at most max: out_of_range with max and actual.
func (d DecimalDecoder) Max(max Decimal) DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Cmp(max) <= 0 }, KeyOutOfRangeMaximum, "max", max)
}

// Range requires a value from min to max, both included: out_of_range with
// min, max and actual.
func (d DecimalDecoder) Range(min, max Decimal) DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Cmp(min) >= 0 && v.Cmp(max) <= 0 },
		KeyOutOfRangeRange, "min", min, "max", max)
}

// Positive requires a value above zero: out_of_range with min 0 and actual.
func (d DecimalDecoder) Positive() DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Sign() > 0 }, KeyOutOfRangePositive, "min", Decimal{})
}

// Negative requires a value below zero: out_of_range with max 0 and actual.
func (d DecimalDecoder) Negative() DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Sign() < 0 }, KeyOutOfRangeNegative, "max", Decimal{})
}

// NonNegative requires zero or above: out_of_range with min 0 and actual.
func (d DecimalDecoder) NonNegative() DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Sign() >= 0 }, KeyOutOfRangeNonNegative, "min", Decimal{})
}

// NonPositive requires zero or below: out_of_range with max 0 and actual.
func (d DecimalDecoder) NonPositive() DecimalDecoder {
	return d.bound(func(v Decimal) bool { return v.Sign() <= 0 }, KeyOutOfRangeNonPositive, "max", Decimal{})
}

// MultipleOf requires a multiple of divisor: not_multiple_of with divisor and
// actual. It panics when divisor is zero.
func (d DecimalDecoder) MultipleOf(divisor Decimal) DecimalDecoder {
	if divisor.Sign() == 0 {
		panic("raoh: divisor must not be zero")
	}
	return newDecimal(d.s.require(func(v Decimal) bool {
		a, b := aligned(v, divisor)
		return new(big.Int).Rem(a, b).Sign() == 0
	}, func(v Decimal) Issue {
		return NewIssue(CodeNotMultipleOf).WithMeta("divisor", divisor.number()).WithMeta("actual", v.number())
	}))
}

// Scale allows at most digits digits after the decimal point, as the number is
// written: invalid_scale with maxScale and actualScale.
func (d DecimalDecoder) Scale(digits int) DecimalDecoder {
	return newDecimal(d.s.require(func(v Decimal) bool { return v.scale <= digits }, func(v Decimal) Issue {
		return NewIssue(CodeInvalidScale).WithMeta("maxScale", digits).WithMeta("actualScale", v.scale)
	}))
}
