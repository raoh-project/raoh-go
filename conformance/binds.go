package main

import "strings"

// binds is every feature the runner has a binding of, written as the
// catalogue names it. A form the catalogue has and this does not is not
// bound, so the cases that need it are not run, and the verifier reports
// them.
var binds = func() map[string]bool {
	out := map[string]bool{}
	add := func(prefix string, names string) {
		for _, n := range strings.Fields(names) {
			out[prefix+n] = true
		}
	}
	add("decoder.", `string int long float double decimal bool list dict object strictObject strict
		nullable enum enum.message literal literal.message discriminate discriminateBy oneOf
		withDefault recover recoverWith`)
	add("field.", "field optionalField optionalNullableField flat")
	operations := map[string]string{
		"string": `trim toLowerCase toUpperCase normalize nonBlank minLength maxLength fixedLength oneOf
			startsWith endsWith includes pattern email ipv4 ipv6 ip ulid cuid uuid url uri toInt toLong
			toDecimal toBool iso8601 date time dateTime offsetDateTime`,
		"int32":   "min max range positive negative nonNegative nonPositive oneOf multipleOf",
		"int64":   "min max range positive negative nonNegative nonPositive oneOf multipleOf",
		"float32": "min max range positive negative nonNegative nonPositive oneOf",
		"float64": "min max range positive negative nonNegative nonPositive oneOf",
		"decimal": "min max range positive negative nonNegative nonPositive multipleOf scale",
		"bool":    "isTrue",
		"list":    "nonempty minSize maxSize fixedSize unique contains containsAll toSet",
		"map":     "nonempty minSize maxSize fixedSize",
		"any":     "map refine flatMap",
	}
	for _, kind := range []string{"instant", "date", "time", "datetime", "offset_datetime"} {
		operations[kind] = "before after between"
	}
	for kind, names := range operations {
		for _, n := range strings.Fields(names) {
			out["operation."+kind+"."+n] = true
			// Every operation that reports an issue takes its message, through
			// the Message of the decoder it gives.
			out["operation."+kind+"."+n+".message"] = true
		}
	}
	add("encoder.", "string object")
	add("property.", "propertyWithDefault")
	add("fixture.", `first square_side square area shift_add_10 shift_add_100 shift_add_1000
		decimal_string even ordered_period issue_count_plus_10 identity`)
	return out
}()
