package raoh

import (
	"encoding/json"
	"errors"
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
// It panics when a value is repeated.
func (d IntDecoder[T]) OneOf(allowed ...T) IntDecoder[T] {
	sorted := sortedDistinct(allowed)
	return d.require(func(v T) bool { _, ok := slices.BinarySearch(sorted, v); return ok }, func(v T) Issue {
		return NewIssue(CodeNotAllowed).WithMeta("allowed", sorted).WithMeta("actual", v)
	})
}

// binaryFloat is the floating-point types a decoder reads, the counterparts of
// Java's double and float.
type binaryFloat interface{ float32 | float64 }

// floatName is the name Raoh for Java gives the type in expected.
func floatName[T binaryFloat]() string {
	var zero T
	if _, ok := any(zero).(float32); ok {
		return "float"
	}
	return "double"
}

// compareFloat is Double.compare and Float.compare, which order the values the
// operators of Go leave equal or unordered: -0 is below +0, and NaN is above
// every other value and equal to itself.
func compareFloat[T binaryFloat](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	aNaN, bNaN := a != a, b != b
	switch {
	case aNaN && bNaN:
		return 0
	case aNaN:
		return 1
	case bNaN:
		return -1
	}
	// Equal as numbers; only the sign of a zero tells them apart.
	aNeg, bNeg := math.Signbit(float64(a)), math.Signbit(float64(b))
	switch {
	case aNeg && !bNeg:
		return -1
	case !aNeg && bNeg:
		return 1
	}
	return 0
}

// numericRangeIssue is the issue for a number the type cannot hold: the
// message key type_mismatch.numeric_range, with expected alone.
func numericRangeIssue(expected string) *Issue {
	i := NewIssue(CodeTypeMismatch).WithMessageKey(KeyTypeMismatchNumericRange).WithMeta("expected", expected)
	return &i
}

func readFloat[T binaryFloat](in any) (T, *Issue) {
	name := floatName[T]()
	in = plain(in)
	if isNull(in) {
		i := NewIssue(CodeRequired)
		return 0, &i
	}
	f, ok := floatOf[T](in)
	if !ok {
		i := NewIssue(CodeTypeMismatch).WithMeta("expected", name).WithMeta("actual", kind(in))
		return 0, &i
	}
	// NaN is let through, for a constraint to reject as Raoh for Java does.
	if math.IsInf(float64(f), 0) {
		return 0, numericRangeIssue(name)
	}
	return f, nil
}

// floatOf returns the number in as a T, rounded to the nearest, or an infinity
// for one beyond the range of T.
func floatOf[T binaryFloat](in any) (T, bool) {
	switch n := in.(type) {
	case json.Number:
		return floatText[T](string(n))
	case float64:
		return T(n), true
	case float32:
		return T(n), true
	// An integer type converts directly, rounded once to the nearest, without the
	// big numbers integerOf makes.
	case int:
		return T(n), true
	case int8:
		return T(n), true
	case int16:
		return T(n), true
	case int32:
		return T(n), true
	case int64:
		return T(n), true
	case uint:
		return T(n), true
	case uint8:
		return T(n), true
	case uint16:
		return T(n), true
	case uint32:
		return T(n), true
	case uint64:
		return T(n), true
	case uintptr:
		return T(n), true
	}
	n, ok := integerOf(in)
	if !ok {
		return 0, false
	}
	bf := new(big.Float).SetInt(n)
	var zero T
	if _, is32 := any(zero).(float32); is32 {
		f, _ := bf.Float32()
		return T(f), true
	}
	f, _ := bf.Float64()
	return T(f), true
}

// floatText reads the text of a number as the Raoh Specification says: the
// exact value the text writes, rounded to the type once, so a float is never
// a double rounded again. A zero written with a minus sign, -0, -0.0 or
// -0e10, is -0.
func floatText[T binaryFloat](text string) (T, bool) {
	var zero T
	bits := 64
	if _, is32 := any(zero).(float32); is32 {
		bits = 32
	}
	f, err := strconv.ParseFloat(text, bits)
	// A number beyond the range is reported with the infinity, which the
	// caller turns into the issue.
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	return T(f), true
}

// floatBound requires ok, or is out_of_range under key with the bounds, given
// as name and value pairs, and actual.
func floatBound[T binaryFloat](s scalar[T], ok func(T) bool, key string, bounds ...any) scalar[T] {
	return s.require(ok, func(v T) Issue {
		i := outOfRange(key)
		for n := 0; n < len(bounds); n += 2 {
			i = i.WithMeta(bounds[n].(string), bounds[n+1])
		}
		return i.WithMeta("actual", v)
	})
}

func floatMin[T binaryFloat](s scalar[T], min T) scalar[T] {
	return floatBound(s, func(v T) bool { return compareFloat(v, min) >= 0 }, KeyOutOfRangeMinimum, "min", min)
}

func floatMax[T binaryFloat](s scalar[T], max T) scalar[T] {
	return floatBound(s, func(v T) bool { return compareFloat(v, max) <= 0 }, KeyOutOfRangeMaximum, "max", max)
}

func floatRange[T binaryFloat](s scalar[T], min, max T) scalar[T] {
	if compareFloat(min, max) > 0 {
		panic("raoh: min must not be greater than max")
	}
	return floatBound(s, func(v T) bool { return compareFloat(min, v) <= 0 && compareFloat(v, max) <= 0 },
		KeyOutOfRangeRange, "min", min, "max", max)
}

func floatPositive[T binaryFloat](s scalar[T]) scalar[T] {
	var zero T
	return floatBound(s, func(v T) bool { return compareFloat(v, zero) > 0 }, KeyOutOfRangePositive, "min", zero)
}

func floatNegative[T binaryFloat](s scalar[T]) scalar[T] {
	var zero T
	return floatBound(s, func(v T) bool { return compareFloat(v, zero) < 0 }, KeyOutOfRangeNegative, "max", zero)
}

func floatNonNegative[T binaryFloat](s scalar[T]) scalar[T] {
	var zero T
	return floatBound(s, func(v T) bool { return compareFloat(v, zero) >= 0 }, KeyOutOfRangeNonNegative, "min", zero)
}

func floatNonPositive[T binaryFloat](s scalar[T]) scalar[T] {
	var zero T
	return floatBound(s, func(v T) bool { return compareFloat(v, zero) <= 0 }, KeyOutOfRangeNonPositive, "max", zero)
}

func floatOneOf[T binaryFloat](s scalar[T], allowed []T) scalar[T] {
	sorted := sortedDistinctFunc(allowed, compareFloat[T])
	return s.require(func(v T) bool {
		_, ok := slices.BinarySearchFunc(sorted, v, compareFloat[T])
		return ok
	}, func(v T) Issue {
		return NewIssue(CodeNotAllowed).WithMeta("allowed", sorted).WithMeta("actual", v)
	})
}

// Float64Decoder decodes a number into a float64.
//
// Null or a missing member is required; any other type is type_mismatch. An
// integer is read as the nearest float64. A number beyond the range of a
// float64, and an infinity, is type_mismatch under the message key
// type_mismatch.numeric_range; NaN is read, for a constraint to reject.
// Constraints compare as Java's Double.compare does, so -0 is below 0 and NaN
// is above every other value. Bounds appear in messages as Java's
// Double.toString writes them, such as 1.0E7.
type Float64Decoder struct {
	Decoder[any, float64]
	s scalar[float64]
}

// Float64 returns a decoder of a number into a float64.
func Float64() Float64Decoder {
	return newFloat64(scalar[float64]{read: readFloat[float64]})
}

func newFloat64(s scalar[float64]) Float64Decoder { return Float64Decoder{s.build(), s} }

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d Float64Decoder) Message(message string) Float64Decoder {
	return newFloat64(d.s.message(message))
}

// Min requires at least min: out_of_range with min and actual.
func (d Float64Decoder) Min(min float64) Float64Decoder { return newFloat64(floatMin(d.s, min)) }

// Max allows at most max: out_of_range with max and actual.
func (d Float64Decoder) Max(max float64) Float64Decoder { return newFloat64(floatMax(d.s, max)) }

// Range requires a value from min to max, both included: out_of_range with
// min, max and actual. It panics when min is above max.
func (d Float64Decoder) Range(min, max float64) Float64Decoder {
	return newFloat64(floatRange(d.s, min, max))
}

// Positive requires a value above zero: out_of_range with min 0.0 and actual.
func (d Float64Decoder) Positive() Float64Decoder { return newFloat64(floatPositive(d.s)) }

// Negative requires a value below zero: out_of_range with max 0.0 and actual.
func (d Float64Decoder) Negative() Float64Decoder { return newFloat64(floatNegative(d.s)) }

// NonNegative requires zero or above: out_of_range with min 0.0 and actual.
func (d Float64Decoder) NonNegative() Float64Decoder { return newFloat64(floatNonNegative(d.s)) }

// NonPositive requires zero or below: out_of_range with max 0.0 and actual.
func (d Float64Decoder) NonPositive() Float64Decoder { return newFloat64(floatNonPositive(d.s)) }

// OneOf requires one of allowed: not_allowed with allowed sorted, and actual.
// It panics when a value is repeated; NaN is equal to NaN, and -0 is not equal
// to 0.
func (d Float64Decoder) OneOf(allowed ...float64) Float64Decoder {
	return newFloat64(floatOneOf(d.s, allowed))
}

// Float32Decoder decodes a number into a float32, the counterpart of Raoh for
// Java's FloatDecoder.
//
// It reads what a [Float64Decoder] reads, rounded to the nearest float32, so
// 16777217 is read as 16777216. A number beyond the range of a float32 is
// type_mismatch under the message key type_mismatch.numeric_range, with expected
// float. Constraints compare as Java's Float.compare does, and bounds appear in
// messages as Java's Float.toString writes them.
type Float32Decoder struct {
	Decoder[any, float32]
	s scalar[float32]
}

// Float32 returns a decoder of a number into a float32.
func Float32() Float32Decoder {
	return newFloat32(scalar[float32]{read: readFloat[float32]})
}

func newFloat32(s scalar[float32]) Float32Decoder { return Float32Decoder{s.build(), s} }

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d Float32Decoder) Message(message string) Float32Decoder {
	return newFloat32(d.s.message(message))
}

// Min requires at least min: out_of_range with min and actual.
func (d Float32Decoder) Min(min float32) Float32Decoder { return newFloat32(floatMin(d.s, min)) }

// Max allows at most max: out_of_range with max and actual.
func (d Float32Decoder) Max(max float32) Float32Decoder { return newFloat32(floatMax(d.s, max)) }

// Range requires a value from min to max, both included: out_of_range with
// min, max and actual. It panics when min is above max.
func (d Float32Decoder) Range(min, max float32) Float32Decoder {
	return newFloat32(floatRange(d.s, min, max))
}

// Positive requires a value above zero: out_of_range with min 0.0 and actual.
func (d Float32Decoder) Positive() Float32Decoder { return newFloat32(floatPositive(d.s)) }

// Negative requires a value below zero: out_of_range with max 0.0 and actual.
func (d Float32Decoder) Negative() Float32Decoder { return newFloat32(floatNegative(d.s)) }

// NonNegative requires zero or above: out_of_range with min 0.0 and actual.
func (d Float32Decoder) NonNegative() Float32Decoder { return newFloat32(floatNonNegative(d.s)) }

// NonPositive requires zero or below: out_of_range with max 0.0 and actual.
func (d Float32Decoder) NonPositive() Float32Decoder { return newFloat32(floatNonPositive(d.s)) }

// OneOf requires one of allowed: not_allowed with allowed sorted, and actual.
// It panics when a value is repeated; NaN is equal to NaN, and -0 is not equal
// to 0.
func (d Float32Decoder) OneOf(allowed ...float32) Float32Decoder {
	return newFloat32(floatOneOf(d.s, allowed))
}
