package raoh

import (
	"fmt"
	"slices"
)

// asciiLower folds ASCII letters to lower case and leaves every other
// character as it is.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// EnumOf returns a decoder of a string naming one of values, compared
// ignoring ASCII case only. A string that names none is invalid_format under
// the message key invalid_format.enum, with the names allowed, in lower case
// and sorted by code point. It panics when two names are equal under ASCII case
// folding.
func EnumOf[T any](values map[string]T) Decoder[any, T] {
	folded := make(map[string]T, len(values))
	for name, v := range values {
		key := asciiLower(name)
		if _, dup := folded[key]; dup {
			panic(fmt.Sprintf("raoh: enum names equal to %q under ASCII case folding appear twice", key))
		}
		folded[key] = v
	}
	allowed := make([]string, 0, len(folded))
	for k := range folded {
		allowed = append(allowed, k)
	}
	slices.Sort(allowed)
	str := String().Decoder
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		o := str.run(in, at)
		if o.failed() {
			return failAs[T](o)
		}
		if v, ok := folded[asciiLower(o.value)]; ok {
			return succeed(v)
		}
		return invalid[T](NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatEnum).
			WithMeta("allowed", allowed).At(at))
	}}
}

// Literal returns a decoder of exactly the string expected: invalid_format
// under the message key invalid_format.literal, with expected, for any other
// string.
func Literal(expected string) Decoder[any, string] {
	str := String().Decoder
	return Decoder[any, string]{func(in any, at Path) outcome[string] {
		o := str.run(in, at)
		if o.failed() || o.value == expected {
			return o
		}
		return invalid[string](NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatLiteral).
			WithMeta("expected", expected).At(at))
	}}
}

// variant is one of the choices of [Discriminate].
type variant[T any] struct {
	tag string
	d   Decoder[any, T]
}

// Variant returns the choice of [Discriminate] that tag names, decoded with d.
func Variant[T any](tag string, d DecoderOf[T]) variant[T] {
	return variant[T]{tag, d.decoder()}
}

// Discriminate returns a decoder of an object whose member tagField names the
// variant that decodes it. The whole object is handed to that variant's
// decoder.
//
// The input must be an object with the member tagField as a string, reported
// as an object field would be. A tag no variant has is not_allowed at the
// member, with the tags allowed sorted by code point. It panics when two
// variants have the same tag.
func Discriminate[T any](tagField string, variants ...variant[T]) Decoder[any, T] {
	byTag := make(map[string]Decoder[any, T], len(variants))
	allowed := make([]string, 0, len(variants))
	for _, v := range variants {
		if _, dup := byTag[v.tag]; dup {
			panic(fmt.Sprintf("raoh: duplicate variant tag %q", v.tag))
		}
		byTag[v.tag] = v.d
		allowed = append(allowed, v.tag)
	}
	slices.Sort(allowed)
	tag := Object(Fields().Field(tagField, String())).Map(func(t string) string { return t })
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		t := tag.run(in, at)
		if t.failed() {
			return failAs[T](t)
		}
		if d, ok := byTag[t.value]; ok {
			return d.run(in, at)
		}
		return invalid[T](NewIssue(CodeNotAllowed).WithMeta("allowed", allowed).At(at.Key(tagField)))
	}}
}
