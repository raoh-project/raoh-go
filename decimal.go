package raoh

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Decimal is a decimal number as written: an unscaled integer times ten to the
// power of minus its scale, as Java's BigDecimal holds one. 1.20 has the
// unscaled value 120 and the scale 2, and is not the same Decimal as 1.2,
// though they compare equal with Cmp. The scale fits an int32, as in Java. The
// zero value is 0.
//
// A Decimal keeps the digits as written, and Cmp, Sign, String and the
// constraints of [DecimalDecoder] work on them in time and memory linear in
// their number. None of them builds a power of ten from the scale, so an input
// such as 1e-1000000000 costs no more than its dozen bytes. Unscaled builds a
// big.Int of the digits; converting a Decimal to a rational or binary number
// that holds its value exactly takes memory in proportion to its scale, which
// is the caller's to bound, with [DecimalDecoder.Scale] for one.
type Decimal struct {
	neg bool
	// digits is the unscaled value's decimal digits without leading zeros,
	// and empty for zero.
	digits string
	scale  int32
}

var errNotDecimal = errors.New("raoh: not a decimal")

// ParseDecimal reads s as Java's new BigDecimal(String) does: an optional
// sign, digits with an optional decimal point, and an optional exponent. The
// resulting scale must fit an int32.
func ParseDecimal(s string) (Decimal, error) {
	fail := func() (Decimal, error) { return Decimal{}, fmt.Errorf("%w: %q", errNotDecimal, s) }
	mantissa, exp, hasExp := s, "", false
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exp, hasExp = s[:i], s[i+1:], true
	}
	neg := false
	if mantissa != "" && (mantissa[0] == '+' || mantissa[0] == '-') {
		neg, mantissa = mantissa[0] == '-', mantissa[1:]
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := whole + fraction
	if digits == "" || !allBytes(digits, isDigit) {
		return fail()
	}
	scale := int64(len(fraction))
	if hasExp {
		// An exponent with more digits than an int32 can hold is out of range
		// whatever follows, so it is refused before it is read as a number.
		if len(strings.TrimLeft(strings.TrimLeft(exp, "+-"), "0")) > 10 {
			return fail()
		}
		e, err := strconv.ParseInt(exp, 10, 64)
		if err != nil {
			return fail()
		}
		scale -= e
	}
	if scale < math.MinInt32 || scale > math.MaxInt32 {
		return fail()
	}
	digits = strings.TrimLeft(digits, "0")
	return Decimal{neg: neg && digits != "", digits: digits, scale: int32(scale)}, nil
}

// MustDecimal is ParseDecimal that panics when s is not a decimal.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// decimalOfInt returns the integer n as a Decimal of scale 0.
func decimalOfInt(n *big.Int) Decimal {
	digits := new(big.Int).Abs(n).String()
	if digits == "0" {
		digits = ""
	}
	return Decimal{neg: n.Sign() < 0, digits: digits}
}

// Unscaled returns the unscaled value. It takes time that grows faster than
// the number of digits, so bound the digits of an untrusted Decimal first.
func (d Decimal) Unscaled() *big.Int {
	n := new(big.Int)
	if d.digits != "" {
		n.SetString(d.digits, 10)
	}
	if d.neg {
		n.Neg(n)
	}
	return n
}

// Scale returns the number of digits after the decimal point; a negative
// scale multiplies by a power of ten.
func (d Decimal) Scale() int { return int(d.scale) }

// Sign returns -1, 0 or +1.
func (d Decimal) Sign() int {
	switch {
	case d.digits == "":
		return 0
	case d.neg:
		return -1
	}
	return 1
}

// adjusted is the exponent of the leading digit: d is at least 10^adjusted and
// below 10^(adjusted+1). d must not be zero.
func (d Decimal) adjusted() int64 { return int64(len(d.digits)) - 1 - int64(d.scale) }

// Cmp compares d and e by value: -1, 0 or +1.
func (d Decimal) Cmp(e Decimal) int {
	if sd, se := d.Sign(), e.Sign(); sd != se || sd == 0 {
		return cmpInt(sd, se)
	}
	c := cmpMagnitude(d, e)
	if d.neg {
		return -c
	}
	return c
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpMagnitude compares two non-zero decimals by absolute value: by the
// exponent of their leading digits, and when that is the same, digit by digit
// from the leading one, a missing digit reading as 0.
func cmpMagnitude(d, e Decimal) int {
	if ad, ae := d.adjusted(), e.adjusted(); ad != ae {
		if ad < ae {
			return -1
		}
		return 1
	}
	for i := range max(len(d.digits), len(e.digits)) {
		if c := cmpInt(int(digitAt(d.digits, i)), int(digitAt(e.digits, i))); c != 0 {
			return c
		}
	}
	return 0
}

func digitAt(digits string, i int) byte {
	if i < len(digits) {
		return digits[i]
	}
	return '0'
}

// isMultipleOf reports whether d is an integer multiple of divisor, which is
// not zero, in time linear in the digits of d and without building a power of
// ten from either scale.
//
// With the trailing zeros of each moved into its exponent, d = a×10^ea and
// divisor = b×10^eb, where neither a nor b is a multiple of 10. d/divisor =
// (a/b)×10^(ea-eb). When ea < eb, that is an integer only if b×10^(eb-ea)
// divides a, which needs 10 to divide a, so it is not. Otherwise it is an
// integer when b/gcd(b, 10^(ea-eb)) divides a; the gcd stops growing once the
// power of ten holds every factor 2 and 5 of b, so the exponent is capped at
// the bit length of b.
func (d Decimal) isMultipleOf(divisor Decimal) bool {
	if d.digits == "" {
		return true
	}
	a, ea := stripZeros(d)
	b, eb := stripZeros(divisor)
	if ea < eb {
		return false
	}
	bn, _ := new(big.Int).SetString(b, 10)
	k := min(ea-eb, int64(bn.BitLen()))
	g := new(big.Int).GCD(nil, nil, bn, new(big.Int).Exp(big.NewInt(10), big.NewInt(k), nil))
	r := new(big.Int).Quo(bn, g)
	return remainder(a, r) == 0
}

// stripZeros returns the digits of d without trailing zeros and the exponent
// of ten they are multiplied by.
func stripZeros(d Decimal) (string, int64) {
	a := strings.TrimRight(d.digits, "0")
	return a, int64(len(d.digits)-len(a)) - int64(d.scale)
}

// remainder returns the decimal digits a modulo r, reading one digit at a
// time.
func remainder(a string, r *big.Int) int {
	if r.IsUint64() && r.Uint64() < 1<<59 {
		m, rem := r.Uint64(), uint64(0)
		for i := range len(a) {
			rem = (rem*10 + uint64(a[i]-'0')) % m
		}
		return int(min(rem, 1))
	}
	rem, ten, digit := new(big.Int), big.NewInt(10), new(big.Int)
	for i := range len(a) {
		rem.Mul(rem, ten).Add(rem, digit.SetInt64(int64(a[i]-'0'))).Rem(rem, r)
	}
	return rem.Sign()
}

// String writes d as Java's BigDecimal.toString does: plainly when the scale
// is not negative and the exponent it would take is at least -6, and as
// d.dddE±n otherwise.
func (d Decimal) String() string {
	digits := d.digits
	if digits == "" {
		digits = "0"
	}
	sign := ""
	if d.neg {
		sign = "-"
	}
	scale := int64(d.scale)
	adjusted := int64(len(digits)) - 1 - scale
	if scale >= 0 && adjusted >= -6 {
		n := int(scale)
		switch {
		case n == 0:
			return sign + digits
		case len(digits) > n:
			return sign + digits[:len(digits)-n] + "." + digits[len(digits)-n:]
		}
		return sign + "0." + strings.Repeat("0", n-len(digits)) + digits
	}
	body := digits[:1]
	if len(digits) > 1 {
		body += "." + digits[1:]
	}
	exp := strconv.FormatInt(adjusted, 10)
	if adjusted >= 0 {
		exp = "+" + exp
	}
	return sign + body + "E" + exp
}

// MarshalJSON writes d as a JSON number, as String writes it.
func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

// number is d as a JSON number for an issue's metadata.
func (d Decimal) number() json.Number { return json.Number(d.String()) }

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
			return decimalOfInt(n), nil
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
	return newDecimal(d.s.require(func(v Decimal) bool { return v.isMultipleOf(divisor) }, func(v Decimal) Issue {
		return NewIssue(CodeNotMultipleOf).WithMeta("divisor", divisor.number()).WithMeta("actual", v.number())
	}))
}

// Scale allows at most digits digits after the decimal point, as the number is
// written: invalid_scale with maxScale and actualScale.
func (d DecimalDecoder) Scale(digits int) DecimalDecoder {
	return newDecimal(d.s.require(func(v Decimal) bool { return int64(v.scale) <= int64(digits) }, func(v Decimal) Issue {
		return NewIssue(CodeInvalidScale).WithMeta("maxScale", digits).WithMeta("actualScale", int(v.scale))
	}))
}
