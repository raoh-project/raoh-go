package raoh

import (
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

// Integer is a Go integer type a number can be decoded into.
type Integer interface {
	int | int32 | int64 | uint | uint32 | uint64
}

// IntDecoder decodes an integer into a T.
//
// Null or a missing member is required. A value of another type, and a number
// with a fraction or an exponent, is type_mismatch with the type found as
// actual (number for such a number). An integer T cannot hold is type_mismatch
// under the message key type_mismatch.numeric_range, with expected alone, as
// Raoh for Java reports it. Constraints run in the order they are written, and
// the first to fail is the one reported.
//
// In JSON text a number is an integer when it is written without a fraction or
// an exponent, so 1.0 is not one, as in Raoh for Java; -0 is read as 0. A Go
// float given as input is an integer when it holds one, since encoding/json
// gives every number as a float64.
//
// Issues name the type in expected as Raoh for Java names the type of the same
// width: integer for 32 bits and long for 64.
type IntDecoder[T Integer] struct {
	Decoder[any, T]
	s scalar[T]
}

// Int returns a decoder of an integer into an int.
func Int() IntDecoder[int] { return intDecoder[int]() }

// Int32 returns a decoder of an integer into an int32.
func Int32() IntDecoder[int32] { return intDecoder[int32]() }

// Int64 returns a decoder of an integer into an int64.
func Int64() IntDecoder[int64] { return intDecoder[int64]() }

// Uint returns a decoder of an integer into a uint.
func Uint() IntDecoder[uint] { return intDecoder[uint]() }

// Uint32 returns a decoder of an integer into a uint32.
func Uint32() IntDecoder[uint32] { return intDecoder[uint32]() }

// Uint64 returns a decoder of an integer into a uint64.
func Uint64() IntDecoder[uint64] { return intDecoder[uint64]() }

func intDecoder[T Integer]() IntDecoder[T] {
	return newInt(scalar[T]{read: readInteger[T]})
}

func newInt[T Integer](s scalar[T]) IntDecoder[T] { return IntDecoder[T]{s.build(), s} }

// integerBounds returns the smallest and largest T and the name Raoh for Java
// gives a type of its width.
func integerBounds[T Integer]() (lo, hi *big.Int, name string) {
	var zero T
	switch any(zero).(type) {
	case int:
		lo, hi = big.NewInt(math.MinInt), big.NewInt(math.MaxInt)
	case int32:
		lo, hi = big.NewInt(math.MinInt32), big.NewInt(math.MaxInt32)
	case int64:
		lo, hi = big.NewInt(math.MinInt64), big.NewInt(math.MaxInt64)
	case uint:
		lo, hi = new(big.Int), new(big.Int).SetUint64(math.MaxUint)
	case uint32:
		lo, hi = new(big.Int), big.NewInt(math.MaxUint32)
	case uint64:
		lo, hi = new(big.Int), new(big.Int).SetUint64(math.MaxUint64)
	}
	name = "long"
	if hi.BitLen() <= 32 {
		name = "integer"
	}
	return lo, hi, name
}

func readInteger[T Integer](in any) (T, *Issue) {
	_, _, name := integerBounds[T]()
	if isNull(in) {
		i := NewIssue(CodeRequired)
		return 0, &i
	}
	n, ok := integerOf(in)
	if !ok {
		i := NewIssue(CodeTypeMismatch).WithMeta("expected", name)
		if actual := kind(in); actual != "" {
			i = i.WithMeta("actual", actual)
		}
		return 0, &i
	}
	return fitInteger[T](n)
}

// fitInteger returns n as a T, or the issue for a number T cannot hold.
func fitInteger[T Integer](n *big.Int) (T, *Issue) {
	lo, hi, name := integerBounds[T]()
	if n.Cmp(lo) < 0 || n.Cmp(hi) > 0 {
		i := NewIssue(CodeTypeMismatch).WithMessageKey(KeyTypeMismatchNumericRange).WithMeta("expected", name)
		return 0, &i
	}
	if n.Sign() < 0 {
		return T(n.Int64()), nil
	}
	return T(n.Uint64()), nil
}

// integerOf returns the integer in, if in is a number that is one.
func integerOf(in any) (*big.Int, bool) {
	in = plain(in)
	if text, ok := numberText(in); ok {
		return integerText(text)
	}
	switch n := in.(type) {
	case int:
		return big.NewInt(int64(n)), true
	case int8:
		return big.NewInt(int64(n)), true
	case int16:
		return big.NewInt(int64(n)), true
	case int32:
		return big.NewInt(int64(n)), true
	case int64:
		return big.NewInt(n), true
	case uint:
		return new(big.Int).SetUint64(uint64(n)), true
	case uint8:
		return big.NewInt(int64(n)), true
	case uint16:
		return big.NewInt(int64(n)), true
	case uint32:
		return big.NewInt(int64(n)), true
	case uint64:
		return new(big.Int).SetUint64(n), true
	case uintptr:
		return new(big.Int).SetUint64(uint64(n)), true
	case float32:
		return floatInteger(float64(n))
	case float64:
		return floatInteger(n)
	}
	return nil, false
}

// beyondAnyInteger stands for an integer written with more digits than any Go
// integer type holds, so it is out of range for all of them without being read.
var beyondAnyInteger = new(big.Int).Lsh(big.NewInt(1), 70)

// integerText returns the integer a number's text writes, if it writes one:
// without a fraction or an exponent. An integer of more than 20 digits is out
// of range for every Go integer type; it is given as beyondAnyInteger with its
// sign rather than read, since reading a decimal string into a big.Int takes
// time that grows faster than its length.
func integerText(text string) (*big.Int, bool) {
	if strings.ContainsAny(text, ".eE") {
		return nil, false
	}
	neg := strings.HasPrefix(text, "-")
	digits := strings.TrimLeft(strings.TrimLeft(text, "+-"), "0")
	if len(digits) > 20 {
		if neg {
			return new(big.Int).Neg(beyondAnyInteger), true
		}
		return beyondAnyInteger, true
	}
	return new(big.Int).SetString(text, 10)
}

func floatInteger(f float64) (*big.Int, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) || f != math.Trunc(f) {
		return nil, false
	}
	n, _ := big.NewFloat(f).Int(nil)
	return n, true
}

func outOfRange(key string) Issue {
	return NewIssue(CodeOutOfRange).WithMessageKey(key)
}

func (d IntDecoder[T]) require(ok func(T) bool, fail func(T) Issue) IntDecoder[T] {
	return newInt(d.s.require(ok, fail))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d IntDecoder[T]) Message(message string) IntDecoder[T] {
	return newInt(d.s.message(message))
}

// Min requires at least min: out_of_range with min and actual.
func (d IntDecoder[T]) Min(min T) IntDecoder[T] {
	return d.require(func(v T) bool { return v >= min }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeMinimum).WithMeta("min", min).WithMeta("actual", v)
	})
}

// Max allows at most max: out_of_range with max and actual.
func (d IntDecoder[T]) Max(max T) IntDecoder[T] {
	return d.require(func(v T) bool { return v <= max }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeMaximum).WithMeta("max", max).WithMeta("actual", v)
	})
}

// Range requires a value from min to max, both included: out_of_range with
// min, max and actual.
func (d IntDecoder[T]) Range(min, max T) IntDecoder[T] {
	return d.require(func(v T) bool { return min <= v && v <= max }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeRange).WithMeta("min", min).WithMeta("max", max).WithMeta("actual", v)
	})
}

// Positive requires a value above zero: out_of_range with min 1 and actual.
func (d IntDecoder[T]) Positive() IntDecoder[T] {
	return d.require(func(v T) bool { return v >= 1 }, func(v T) Issue {
		return outOfRange(KeyOutOfRangePositive).WithMeta("min", T(1)).WithMeta("actual", v)
	})
}

// Negative requires a value below zero: out_of_range with max -1 and actual.
// An unsigned T has none.
func (d IntDecoder[T]) Negative() IntDecoder[T] {
	return d.require(func(v T) bool { return v < 0 }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeNegative).WithMeta("max", -1).WithMeta("actual", v)
	})
}

// NonNegative requires zero or above: out_of_range with min 0 and actual.
func (d IntDecoder[T]) NonNegative() IntDecoder[T] {
	return d.require(func(v T) bool { return v >= 0 }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeNonNegative).WithMeta("min", T(0)).WithMeta("actual", v)
	})
}

// NonPositive requires zero or below: out_of_range with max 0 and actual.
func (d IntDecoder[T]) NonPositive() IntDecoder[T] {
	return d.require(func(v T) bool { return v <= 0 }, func(v T) Issue {
		return outOfRange(KeyOutOfRangeNonPositive).WithMeta("max", T(0)).WithMeta("actual", v)
	})
}

// MultipleOf requires a multiple of divisor: not_multiple_of with divisor and
// actual. It panics when divisor is zero.
func (d IntDecoder[T]) MultipleOf(divisor T) IntDecoder[T] {
	if divisor == 0 {
		panic("raoh: divisor must not be zero")
	}
	return d.require(func(v T) bool { return v%divisor == 0 }, func(v T) Issue {
		return NewIssue(CodeNotMultipleOf).WithMeta("divisor", divisor).WithMeta("actual", v)
	})
}

// OneOf requires one of allowed: not_allowed with allowed sorted, and actual.
func (d IntDecoder[T]) OneOf(allowed ...T) IntDecoder[T] {
	sorted := sortedUnique(allowed)
	return d.require(func(v T) bool { _, ok := slices.BinarySearch(sorted, v); return ok }, func(v T) Issue {
		return NewIssue(CodeNotAllowed).WithMeta("allowed", sorted).WithMeta("actual", v)
	})
}

// Float64Decoder decodes a number into a float64.
//
// Null or a missing member is required; any other type is type_mismatch. An
// integer is read as the nearest float64. Bounds appear in messages as Java's
// Double.toString writes them, such as 1.0E7.
type Float64Decoder struct {
	Decoder[any, float64]
	s scalar[float64]
}

// Float64 returns a decoder of a number into a float64.
func Float64() Float64Decoder {
	return newFloat(scalar[float64]{read: readFloat})
}

func newFloat(s scalar[float64]) Float64Decoder { return Float64Decoder{s.build(), s} }

func readFloat(in any) (float64, *Issue) {
	in = plain(in)
	if isNull(in) {
		i := NewIssue(CodeRequired)
		return 0, &i
	}
	if text, ok := numberText(in); ok {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsInf(f, 0) {
			i := NewIssue(CodeTypeMismatch).WithMeta("expected", "double")
			return 0, &i
		}
		return f, nil
	}
	switch n := in.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	}
	if n, ok := integerOf(in); ok {
		f, _ := new(big.Float).SetInt(n).Float64()
		return f, nil
	}
	i := NewIssue(CodeTypeMismatch).WithMeta("expected", "double").WithMeta("actual", kind(in))
	return 0, &i
}

func (d Float64Decoder) bound(ok func(float64) bool, key string, bounds ...any) Float64Decoder {
	return newFloat(d.s.require(ok, func(v float64) Issue {
		i := outOfRange(key)
		for n := 0; n < len(bounds); n += 2 {
			i = i.WithMeta(bounds[n].(string), bounds[n+1])
		}
		return i.WithMeta("actual", v)
	}))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d Float64Decoder) Message(message string) Float64Decoder {
	return newFloat(d.s.message(message))
}

// Min requires at least min: out_of_range with min and actual.
func (d Float64Decoder) Min(min float64) Float64Decoder {
	return d.bound(func(v float64) bool { return v >= min }, KeyOutOfRangeMinimum, "min", min)
}

// Max allows at most max: out_of_range with max and actual.
func (d Float64Decoder) Max(max float64) Float64Decoder {
	return d.bound(func(v float64) bool { return v <= max }, KeyOutOfRangeMaximum, "max", max)
}

// Range requires a value from min to max, both included: out_of_range with
// min, max and actual.
func (d Float64Decoder) Range(min, max float64) Float64Decoder {
	return d.bound(func(v float64) bool { return min <= v && v <= max }, KeyOutOfRangeRange, "min", min, "max", max)
}

// Positive requires a value above zero: out_of_range with min 0.0 and actual.
func (d Float64Decoder) Positive() Float64Decoder {
	return d.bound(func(v float64) bool { return v > 0 }, KeyOutOfRangePositive, "min", 0.0)
}

// Negative requires a value below zero: out_of_range with max 0.0 and actual.
func (d Float64Decoder) Negative() Float64Decoder {
	return d.bound(func(v float64) bool { return v < 0 }, KeyOutOfRangeNegative, "max", 0.0)
}

// NonNegative requires zero or above: out_of_range with min 0.0 and actual.
func (d Float64Decoder) NonNegative() Float64Decoder {
	return d.bound(func(v float64) bool { return v >= 0 }, KeyOutOfRangeNonNegative, "min", 0.0)
}

// NonPositive requires zero or below: out_of_range with max 0.0 and actual.
func (d Float64Decoder) NonPositive() Float64Decoder {
	return d.bound(func(v float64) bool { return v <= 0 }, KeyOutOfRangeNonPositive, "max", 0.0)
}

// OneOf requires one of allowed: not_allowed with allowed sorted, and actual.
func (d Float64Decoder) OneOf(allowed ...float64) Float64Decoder {
	sorted := sortedUnique(allowed)
	return newFloat(d.s.require(func(v float64) bool { return slices.Contains(sorted, v) }, func(v float64) Issue {
		return NewIssue(CodeNotAllowed).WithMeta("allowed", sorted).WithMeta("actual", v)
	}))
}
