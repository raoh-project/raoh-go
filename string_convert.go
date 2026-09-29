package raoh

// These methods continue as the decoder of the type the string is read as,
// where UUID, URL and URI end in a Conversion. The string's own constraints
// written before them run first and report as they do; the conversion reports
// type_mismatch when the text is not of the type. Message after a conversion,
// before any constraint of the new type, is the message of that report.
//
// ToInt and ToLong carry the contract of Raoh for Java's toInt and toLong: 32
// and 64 bits, whatever the width of Go's int, which is what Int reads.

// ToInt reads the string as a 32-bit integer: an optional + or - and one or
// more ASCII digits, leading zeros allowed. Other Unicode digits and white
// space are type_mismatch with expected integer, and a number outside the
// range of int32 is type_mismatch under type_mismatch.numeric_range.
func (d StringDecoder) ToInt() IntDecoder[int32] { return toInteger[int32](d) }

// ToLong reads the string as a 64-bit integer, in the text ToInt accepts. The
// type_mismatch issues name the type long.
func (d StringDecoder) ToLong() IntDecoder[int64] { return toInteger[int64](d) }

// ToDecimal reads the string as a [Decimal]: an optional + or -, ASCII digits
// with an optional decimal point that has a digit on at least one side (12,
// 12.5, 5., .5), and an optional exponent (1e3, 2.5E-4). Other Unicode digits,
// white space, NaN and Infinity are type_mismatch with expected decimal, as is
// an exponent a Decimal cannot hold.
//
// The decoder sets no limit on the number of digits. Put MaxLength before it
// when the input's contract has one.
func (d StringDecoder) ToDecimal() DecimalDecoder {
	return newDecimal(viaString(d.s, func(v string) (Decimal, *Issue) {
		if n, err := ParseDecimal(v); err == nil {
			return n, nil
		}
		i := NewIssue(CodeTypeMismatch).WithMeta("expected", "decimal")
		return Decimal{}, &i
	}))
}

// ToBool reads the string as a boolean: true, 1, yes or on, and false, 0, no
// or off, with A to Z and a to z equal and no other case mapping. Any other
// text is type_mismatch with expected boolean.
func (d StringDecoder) ToBool() BoolDecoder {
	return newBool(viaString(d.s, func(v string) (bool, *Issue) {
		switch asciiLower(v) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		}
		i := NewIssue(CodeTypeMismatch).WithMeta("expected", "boolean")
		return false, &i
	}))
}

func toInteger[T Integer](d StringDecoder) IntDecoder[T] {
	return newInt(viaString(d.s, func(v string) (T, *Issue) {
		if n, ok := integerText(v); ok && isSignedDigits(v) {
			return fitInteger[T](n)
		}
		_, _, name := integerBounds[T]()
		i := NewIssue(CodeTypeMismatch).WithMeta("expected", name)
		return 0, &i
	}))
}

// isSignedDigits is [+-]?[0-9]+.
func isSignedDigits(s string) bool {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	return s != "" && allBytes(s, isDigit)
}
