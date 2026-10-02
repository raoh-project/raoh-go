// Package encode turns domain values back into boundary data: the
// map[string]any, []any and scalar values that raoh's decoders read and
// encoding/json writes.
//
// It is the counterpart of the decoders in package raoh, written to sit next
// to them. A domain type whose fields are unexported cannot be written by
// encoding/json's struct tags; an encoder reads it through its accessors
// instead:
//
//	var userEncoder = encode.Object(
//		encode.Property("email", User.Email, encode.String().Contramap(Email.String)),
//		encode.Property("age", User.Age, encode.Int()),
//		encode.OptionalProperty("nickname", User.Nickname, encode.String()),
//	)
//
//	body, err := json.Marshal(userEncoder.Encode(user))
//
// An encoder cannot fail. A definition that does not cover a value, such as a
// Discriminate given a type none of its variants names, is a mistake in the
// program and panics.
package encode

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"time"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/internal/hashable"
	"github.com/raoh-project/raoh-go/internal/javatime"
)

// requireArgument panics, when an encoder is built, if a function or encoder
// it is given is nil: the encoder would call it and panic, whatever the value.
func requireArgument(ok bool, constructor, argument string) {
	if !ok {
		panic(fmt.Sprintf("raoh/encode: %s needs %s that is not nil", constructor, argument))
	}
}

// requireReceiver panics if an encoder is nil: the nil Encoder is the zero
// value of the type and has nothing to run.
func requireReceiver(ok bool, method string) {
	if !ok {
		panic(fmt.Sprintf("raoh/encode: %s needs a receiver that is not nil", method))
	}
}

// Encoder turns a T into its boundary representation O.
type Encoder[T, O any] func(T) O

// Encode encodes v.
func (e Encoder[T, O]) Encode(v T) O {
	requireReceiver(e != nil, "Encoder.Encode")
	return e(v)
}

// Contramap returns an encoder of S that turns an S into a T with f and
// encodes that, such as the value a domain type wraps.
func (e Encoder[T, O]) Contramap[S any](f func(S) T) Encoder[S, O] {
	requireReceiver(e != nil, "Encoder.Contramap")
	requireArgument(f != nil, "Encoder.Contramap", "f")
	return func(v S) O { return e(f(v)) }
}

// AndThen returns an encoder that hands what e gives to next.
func (e Encoder[T, O]) AndThen[P any](next Encoder[O, P]) Encoder[T, P] {
	requireReceiver(e != nil, "Encoder.AndThen")
	requireArgument(next != nil, "Encoder.AndThen", "next")
	return func(v T) P { return next(e(v)) }
}

// Value returns the encoder that gives a value as it is.
func Value[T any]() Encoder[T, T] { return func(v T) T { return v } }

// String returns the encoder of a string, as it is.
func String() Encoder[string, string] { return Value[string]() }

// Int returns the encoder of an int, as it is.
func Int() Encoder[int, int] { return Value[int]() }

// Int32 returns the encoder of an int32, as it is.
func Int32() Encoder[int32, int32] { return Value[int32]() }

// Int64 returns the encoder of an int64, as it is.
func Int64() Encoder[int64, int64] { return Value[int64]() }

// Uint returns the encoder of a uint, as it is.
func Uint() Encoder[uint, uint] { return Value[uint]() }

// Uint32 returns the encoder of a uint32, as it is.
func Uint32() Encoder[uint32, uint32] { return Value[uint32]() }

// Uint64 returns the encoder of a uint64, as it is.
func Uint64() Encoder[uint64, uint64] { return Value[uint64]() }

// Float64 returns the encoder of a float64, as it is.
func Float64() Encoder[float64, float64] { return Value[float64]() }

// Bool returns the encoder of a bool, as it is.
func Bool() Encoder[bool, bool] { return Value[bool]() }

// Decimal returns the encoder of a decimal as a JSON number, written as
// BigDecimal.toString writes it, so 1.20 keeps its scale.
func Decimal() Encoder[raoh.Decimal, json.Number] {
	return func(d raoh.Decimal) json.Number { return json.Number(d.String()) }
}

// UUID returns the encoder of a UUID in its lower-case 8-4-4-4-12 form.
func UUID() Encoder[raoh.UUID, string] { return raoh.UUID.String }

// URL returns the encoder of a URL as text.
func URL() Encoder[*url.URL, string] { return (*url.URL).String }

// URI returns the encoder of a URI as the text it was written in. It is the
// encoder of what StringDecoder.URI and URL give; URL is the one for a *url.URL.
// The zero URI is not a URI and gives an empty string.
func URI() Encoder[raoh.URI, string] { return raoh.URI.String }

// Instant returns the encoder of a moment as Instant.toString writes it: in
// UTC, such as 2024-01-15T10:30:00Z.
func Instant() Encoder[time.Time, string] { return javatime.Instant }

// Date returns the encoder of the date of a time as LocalDate.toString writes
// it, such as 2024-01-15. The time's own location decides the date.
func Date() Encoder[time.Time, string] { return javatime.Date }

// Time returns the encoder of the clock time of a time as LocalTime.toString
// writes it, such as 10:30 or 10:30:45.123.
func Time() Encoder[time.Time, string] { return javatime.Time }

// DateTime returns the encoder of the date and clock time of a time as
// LocalDateTime.toString writes it, such as 2024-01-15T10:30.
func DateTime() Encoder[time.Time, string] { return javatime.DateTime }

// OffsetDateTime returns the encoder of a time with its offset as
// OffsetDateTime.toString writes it, such as 2024-01-15T10:30+09:00.
func OffsetDateTime() Encoder[time.Time, string] { return javatime.OffsetDateTime }

// EnumOf returns the encoder of a value as the name names gives it, the map
// raoh.EnumOf decodes with, so one map serves both directions. It panics when
// two names have the same value or T holds an interface anywhere, whose values
// could panic when compared, and the encoder panics on a value no name has.
func EnumOf[T comparable](names map[string]T) Encoder[T, string] {
	if t := reflect.TypeFor[T](); !hashable.Type(t) {
		panic(fmt.Sprintf("raoh/encode: EnumOf needs a type that holds no interface, not %v", t))
	}
	byValue := make(map[T]string, len(names))
	for name, v := range names {
		if other, dup := byValue[v]; dup {
			panic(fmt.Sprintf("raoh/encode: %q and %q name the same value", other, name))
		}
		byValue[v] = name
	}
	return func(v T) string {
		name, ok := byValue[v]
		if !ok {
			panic(fmt.Sprintf("raoh/encode: no name for %v", v))
		}
		return name
	}
}

// Entry is one member of an object: the key it owns, and what it writes there
// for a value. An entry writes its key at most once, as the encoded value, as
// null, or not at all, and writes no other key, so an Object knows from its
// entries which keys each one owns. Only the functions below make one.
type Entry[T any] struct {
	key string
	// emit is what the entry writes for v, and false where it leaves the key
	// out.
	emit func(v T) (any, bool)
}

// Property writes the member key: what get reads from the value, encoded
// with enc. It is the counterpart of a field read by a decoder.
func Property[T, V, O any](key string, get func(T) V, enc Encoder[V, O]) Entry[T] {
	requireArgument(get != nil, "Property", "get")
	requireArgument(enc != nil, "Property", "enc")
	return Entry[T]{key, func(v T) (any, bool) { return enc(get(v)), true }}
}

// NullableProperty writes the member key as null when get gives nil, and
// otherwise what it points to, encoded with enc. It is the counterpart of a
// field read with raoh.Nullable.
func NullableProperty[T, V, O any](key string, get func(T) *V, enc Encoder[V, O]) Entry[T] {
	requireArgument(get != nil, "NullableProperty", "get")
	requireArgument(enc != nil, "NullableProperty", "enc")
	return Entry[T]{key, func(v T) (any, bool) {
		if p := get(v); p != nil {
			return enc(*p), true
		}
		return nil, true
	}}
}

// OptionalProperty leaves the member key out when get gives nil, and
// otherwise writes what it points to, encoded with enc. It is the counterpart
// of a field read with raoh.Optional.
func OptionalProperty[T, V, O any](key string, get func(T) *V, enc Encoder[V, O]) Entry[T] {
	requireArgument(get != nil, "OptionalProperty", "get")
	requireArgument(enc != nil, "OptionalProperty", "enc")
	return Entry[T]{key, func(v T) (any, bool) {
		if p := get(v); p != nil {
			return enc(*p), true
		}
		return nil, false
	}}
}

// PropertyWithDefault writes the member key as what get reads from the value,
// encoded with enc, and as defaultValue, encoded with enc, when get gives nil.
// It is the counterpart of Raoh for Java's MapEncoders.propertyWithDefault, and
// of a field read with a decoder's Default or DefaultFunc.
func PropertyWithDefault[T, V, O any](key string, get func(T) *V, enc Encoder[V, O], defaultValue V) Entry[T] {
	requireArgument(get != nil, "PropertyWithDefault", "get")
	requireArgument(enc != nil, "PropertyWithDefault", "enc")
	return PropertyWithDefaultFunc(key, get, enc, func() V { return defaultValue })
}

// PropertyWithDefaultFunc is [PropertyWithDefault] with a default that is made
// when it is needed: defaultValue is called once for each value get gives nil
// for, and not called for any other.
func PropertyWithDefaultFunc[T, V, O any](key string, get func(T) *V, enc Encoder[V, O], defaultValue func() V) Entry[T] {
	requireArgument(get != nil, "PropertyWithDefaultFunc", "get")
	requireArgument(enc != nil, "PropertyWithDefaultFunc", "enc")
	requireArgument(defaultValue != nil, "PropertyWithDefaultFunc", "defaultValue")
	return Entry[T]{key, func(v T) (any, bool) {
		if p := get(v); p != nil {
			return enc(*p), true
		}
		return enc(defaultValue()), true
	}}
}

// PresenceProperty leaves the member key out for an absent Presence, writes
// null for a null one, and writes the value of a present one, encoded with
// enc. It is the counterpart of a field read with raoh.PresenceOf, so such a
// member round-trips without loss.
func PresenceProperty[T, V, O any](key string, get func(T) raoh.Presence[V], enc Encoder[V, O]) Entry[T] {
	requireArgument(get != nil, "PresenceProperty", "get")
	requireArgument(enc != nil, "PresenceProperty", "enc")
	return Entry[T]{key, func(v T) (any, bool) {
		p := get(v)
		if value, ok := p.Value(); ok {
			return enc(value), true
		}
		return nil, p.IsNull()
	}}
}

// Object returns the encoder of a value as an object, with the member each
// entry writes. It is the counterpart of raoh.Object. It panics when an entry
// is the zero Entry, or two entries own the same key, whatever kind each is
// and whether or not either would leave the key out for a given value.
func Object[T any](entries ...Entry[T]) Encoder[T, map[string]any] {
	entries = slices.Clone(entries)
	owner := make(map[string]int, len(entries))
	for n, e := range entries {
		if e.emit == nil {
			panic("raoh/encode: Object needs each entry to be one a property function made, not the zero Entry")
		}
		if first, ok := owner[e.key]; ok {
			panic(fmt.Sprintf("raoh/encode: object key %q is owned by entries %d and %d", e.key, first, n))
		}
		owner[e.key] = n
	}
	return func(v T) map[string]any {
		out := make(map[string]any, len(entries))
		for _, e := range entries {
			if value, ok := e.emit(v); ok {
				out[e.key] = value
			}
		}
		return out
	}
}

// List returns the encoder of a slice, each element encoded with element. It
// is the counterpart of raoh.List.
func List[T, O any](element Encoder[T, O]) Encoder[[]T, []any] {
	requireArgument(element != nil, "List", "element")
	return func(values []T) []any {
		out := make([]any, len(values))
		for i, v := range values {
			out[i] = element(v)
		}
		return out
	}
}

// Dict returns the encoder of a map, each value encoded with value. It is the
// counterpart of raoh.Dict.
func Dict[V, O any](value Encoder[V, O]) Encoder[map[string]V, map[string]any] {
	requireArgument(value != nil, "Dict", "value")
	return func(values map[string]V) map[string]any {
		out := make(map[string]any, len(values))
		for k, v := range values {
			out[k] = value(v)
		}
		return out
	}
}

// Lazy returns an encoder that calls f each time it encodes and encodes with
// what it returns. It lets an encoder refer to itself.
func Lazy[T, O any](f func() Encoder[T, O]) Encoder[T, O] {
	requireArgument(f != nil, "Lazy", "f")
	return func(v T) O { return f()(v) }
}

// variant is one of the choices of Discriminate.
type variant struct {
	typ    reflect.Type
	tag    string
	encode func(any) map[string]any
}

// Variant returns the choice of Discriminate for the values of the concrete
// type S: the tag it writes and the encoder of the rest of the object.
func Variant[S any](tag string, enc Encoder[S, map[string]any]) variant {
	requireArgument(enc != nil, "Variant", "enc")
	return variant{reflect.TypeFor[S](), tag, func(v any) map[string]any { return enc(v.(S)) }}
}

// Discriminate returns the encoder of a value of an interface type T that
// picks the variant by the value's dynamic type and writes its tag as the
// member tagField, before the members the variant's encoder writes. It is the
// counterpart of raoh.Discriminate. It panics when two variants name the same
// type or tag, and the encoder panics on a value of a type no variant names.
func Discriminate[T any](tagField string, variants ...variant) Encoder[T, map[string]any] {
	byType := make(map[reflect.Type]variant, len(variants))
	tags := map[string]bool{}
	for _, v := range variants {
		if _, dup := byType[v.typ]; dup {
			panic(fmt.Sprintf("raoh/encode: duplicate variant for %v", v.typ))
		}
		if tags[v.tag] {
			panic(fmt.Sprintf("raoh/encode: duplicate variant tag %q", v.tag))
		}
		byType[v.typ], tags[v.tag] = v, true
	}
	return func(value T) map[string]any {
		v, ok := byType[reflect.TypeOf(any(value))]
		if !ok {
			known := make([]string, 0, len(byType))
			for t := range byType {
				known = append(known, t.String())
			}
			slices.Sort(known)
			panic(fmt.Sprintf("raoh/encode: no variant for %T; known variants: %v", any(value), known))
		}
		out := map[string]any{tagField: v.tag}
		for k, member := range v.encode(value) {
			if k != tagField {
				out[k] = member
			}
		}
		return out
	}
}
