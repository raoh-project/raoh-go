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

// EnumDecoder decodes a string naming one of a set of values. See [EnumOf].
type EnumDecoder[T any] struct {
	Decoder[any, T]
	with func(message *string) Decoder[any, T]
}

// Message gives the invalid_format of a string that names none of the values a
// custom message, which every language shows as written. What the string
// decoder reports keeps its own message.
func (d EnumDecoder[T]) Message(message string) EnumDecoder[T] {
	if d.with == nil {
		panic("raoh: EnumDecoder.Message needs a receiver that is not the zero EnumDecoder")
	}
	return EnumDecoder[T]{d.with(&message), d.with}
}

// EnumOf returns a decoder of a string naming one of values, compared
// ignoring ASCII case only. A string that names none is invalid_format under
// the message key invalid_format.enum, with the names allowed, in lower case
// and sorted by code point. It panics when two names are equal under ASCII case
// folding.
func EnumOf[T any](values map[string]T) EnumDecoder[T] {
	return EnumOfWith(values, String())
}

// EnumOfWith is [EnumOf] with the string read by stringDecoder, such as one
// that trims or lower-cases it first. What stringDecoder reports, at the path
// it reports, is reported as it is.
func EnumOfWith[T any](values map[string]T, stringDecoder DecoderOf[string]) EnumDecoder[T] {
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
	str := decoderOf(stringDecoder, "EnumOfWith", "stringDecoder")
	with := func(message *string) Decoder[any, T] {
		return Decoder[any, T]{func(in any, at Path) outcome[T] {
			o := str.run(in, at)
			if o.failed() {
				return failAs[T](o)
			}
			if v, ok := folded[asciiLower(o.value)]; ok {
				return succeed(v)
			}
			return invalid[T](withGiven(NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatEnum).
				WithMeta("allowed", allowed).At(at), message))
		}}
	}
	return EnumDecoder[T]{with(nil), with}
}

// withGiven is i with message as its custom message where one was given.
func withGiven(i Issue, message *string) Issue {
	if message != nil {
		return i.WithMessage(*message)
	}
	return i
}

// LiteralDecoder decodes exactly one string. See [Literal].
type LiteralDecoder struct {
	Decoder[any, string]
	with func(message *string) Decoder[any, string]
}

// Message gives the invalid_format of any other string a custom message,
// which every language shows as written. What the string decoder reports
// keeps its own message.
func (d LiteralDecoder) Message(message string) LiteralDecoder {
	if d.with == nil {
		panic("raoh: LiteralDecoder.Message needs a receiver that is not the zero LiteralDecoder")
	}
	return LiteralDecoder{d.with(&message), d.with}
}

// Literal returns a decoder of exactly the string expected: invalid_format
// under the message key invalid_format.literal, with expected, for any other
// string.
func Literal(expected string) LiteralDecoder {
	return LiteralWith(expected, String())
}

// LiteralWith is [Literal] with the string read by stringDecoder, and the
// value it returns compared with expected. What stringDecoder reports is
// reported as it is.
func LiteralWith(expected string, stringDecoder DecoderOf[string]) LiteralDecoder {
	str := decoderOf(stringDecoder, "LiteralWith", "stringDecoder")
	with := func(message *string) Decoder[any, string] {
		return Decoder[any, string]{func(in any, at Path) outcome[string] {
			o := str.run(in, at)
			if o.failed() || o.value == expected {
				return o
			}
			return invalid[string](withGiven(NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatLiteral).
				WithMeta("expected", expected).At(at), message))
		}}
	}
	return LiteralDecoder{with(nil), with}
}

// variant is one of the choices of [Discriminate].
type variant[T any] struct {
	tag string
	d   Decoder[any, T]
}

// Variant returns the choice of [Discriminate] that tag names, decoded with d.
func Variant[T any](tag string, d DecoderOf[T]) variant[T] {
	return variant[T]{tag, decoderOf(d, "Variant", "d")}
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
	for _, v := range variants {
		if _, dup := byTag[v.tag]; dup {
			panic(fmt.Sprintf("raoh: duplicate variant tag %q", v.tag))
		}
		byTag[v.tag] = v.d
	}
	return discriminate(tagField, String().Decoder, byTag)
}

// DiscriminateWith is [Discriminate] with the variants given as a map from tag,
// and the tag read by tagDecoder, such as one that trims or lower-cases it
// first.
//
// tagDecoder decodes the value of the member tagField, not the whole object:
// what it reports is reported at that member, and the value it returns is the
// tag looked up in variants. The keys of variants are not changed by it. The map
// is copied, so changing it afterwards does not change the decoder.
func DiscriminateWith[T any](tagField string, tagDecoder DecoderOf[string], variants map[string]DecoderOf[T]) Decoder[any, T] {
	byTag := make(map[string]Decoder[any, T], len(variants))
	for tag, d := range variants {
		byTag[tag] = decoderOf(d, "DiscriminateWith", "each variant")
	}
	return discriminate(tagField, decoderOf(tagDecoder, "DiscriminateWith", "tagDecoder"), byTag)
}

func discriminate[T any](tagField string, tagDecoder Decoder[any, string], byTag map[string]Decoder[any, T]) Decoder[any, T] {
	allowed := make([]string, 0, len(byTag))
	for tag := range byTag {
		allowed = append(allowed, tag)
	}
	slices.Sort(allowed)
	tag := Object(Fields().Field(tagField, tagDecoder)).Map(func(t string) string { return t })
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
