# raoh

Go port of [Raoh](https://github.com/kawasima/raoh), a decoder library for turning untyped
boundary input into typed domain values.

It is built around a parse-don't-validate approach:

- decode at the boundary
- keep invalid states out of the domain model
- return failures as values, through Go's `error`
- attach structured errors to precise paths

`encoding/json` turns JSON text into structs whose fields anyone can set. raoh turns input into
domain values whose fields stay unexported, and when the input is wrong it reports every problem it
found, each with the JSON Pointer of where it was, instead of stopping at the first one.

```text
JSON text --raoh.DecodeJSON--> domain values
                          \--> *raoh.Issues (path, code, message, meta)
```

The only dependency is the standard library. Go 1.27 or later is required, for generic methods.

## Installation

```sh
go get github.com/raoh-project/raoh-go
```

## Quick start

```go
type Email struct{ value string }
type Age int
type User struct {
	email Email
	age   Age
}

func NewUser(e Email, a Age) User { return User{e, a} }

var userDecoder = raoh.Object(
	raoh.Fields().
		Field("email", raoh.String().Trim().ToLower().Email().
			Map(func(s string) Email { return Email{s} })).
		Field("age", raoh.Int().Range(0, 150).
			Map(func(n int) Age { return Age(n) })),
).Map(NewUser)

func createUser(w http.ResponseWriter, r *http.Request) error {
	user, err := raoh.DecodeJSONFrom(r.Body, 1<<20, userDecoder) // at most 1 MiB
	if errors.Is(err, raoh.ErrInputTooLarge) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return nil
	}
	if issues, ok := errors.AsType[*raoh.Issues](err); ok {
		w.WriteHeader(http.StatusBadRequest)
		return json.NewEncoder(w).Encode(issues.Render(raoh.English))
	} else if err != nil {
		return err
	}
	return service.CreateUser(r.Context(), user)
}
```

For `{"email": "not an email", "age": 200}` the response body is:

```json
[
  {"path": "/email", "code": "invalid_format", "message": "not a valid email", "meta": {}},
  {"path": "/age", "code": "out_of_range", "message": "must be between 0 and 150",
   "meta": {"actual": 200, "max": 150, "min": 0}}
]
```

The body shows the input back to its sender: `path` names the members the input has, and `meta`
holds values from it, such as `actual`, the duplicates of a list or the name of an unknown member,
along with anything your own code put there with `WithMeta`. That suits a form that points at its
fields; for a public API, filter or omit `meta` before writing the response.

## The model

### `Decoder`

```go
type Decoder[I, T any] struct{ /* ... */ }

func (d Decoder[I, T]) Decode(in I) (T, error)
func (d Decoder[I, T]) Map[U any](f func(T) U) Decoder[I, U]
func (d Decoder[I, T]) AndThen[U any](f func(T) (U, error)) Decoder[I, U]
func (d Decoder[I, T]) Pipe[U any](next Decoder[T, U]) Decoder[I, U]
// Refine, RefineWithMeta, RefineWithPath, Default, DefaultFunc, Fallback, FallbackFunc
```

A decoder is a value that describes how to read an input. It holds no state and can be reused and
shared between goroutines. The built-in decoders, such as `raoh.String()`, embed a
`Decoder[any, T]` and add their own constraints, so `raoh.String().Trim().Email().Map(...)` reads
as one chain.

### The input model

A decoder is handed one of these values, and a decoder you write with `raoh.NewDecoder` sees the
same:

- `nil`, for `null`
- a missing member, which `raoh.IsMissing` tells apart from `null`; only the decoder of an
  object's field is handed one
- a boolean or a string: a value of any type whose underlying type is `bool` or `string`, such as
  `type Status string`, except `json.Number`
- a number: `json.Number`, or a value of any integer or float type, named types such as
  `time.Duration` included
- an array: a value of any type whose underlying type is `[]any`
- bytes: a value of any type whose underlying type is `[]byte`, which only `raoh.Bytes()` reads;
  `DecodeJSON` never gives one
- an object: a value of any type whose underlying type is `map[string]any`, or a
  `*raoh.JSONObject`; `raoh.AsObject` reads either

These include the values `encoding/json` gives when it decodes into an `any`, with or without
`UseNumber`. `DecodeJSON` gives no others: it keeps each number as written, as a `json.Number`, so
a large integer or a decimal is read exactly, and each object as a `*raoh.JSONObject`, which keeps
the members in the order written, so unknown members are reported in input order. Text that is not
JSON is one `invalid_format` issue at the root, under the message key `invalid_format.json`, with
the `line` and `column` where it stopped being JSON.

`DecodeJSONFrom` takes a limit on the bytes it reads. The value a decoder reads is built in
memory, in proportion to the input, so a bound on the input is what bounds the memory a request
can take; input over the limit gives an error wrapping `raoh.ErrInputTooLarge`, not issues.

What decoding costs is in proportion to the input's length, not to the values written in it: an
integer of more digits than any Go integer holds is out of range without being read, and a
`raoh.Decimal` keeps its digits and compares them without building a power of ten from its scale,
so `1e-1000000000` costs no more than its dozen bytes.

### `error`, `Issues` and failures of the program

`Decode` returns an `error`. When the error holds an `*Issues`, the whole error means the input
was invalid, and `errors.AsType[*raoh.Issues](err)` finds it. Any other error is a failure of the
program itself and is returned as the function that produced it returned it.

A function given to `AndThen` returns `(U, error)`, so an existing constructor such as
`NewPeriod(start, end time.Time) (Period, error)` can be passed as it is. What it returns decides
what the error means:

- an error made of issues only, as `raoh.Invalid(...)` returns, reports the input as invalid; its
  issues are gathered with those of the other fields, read as relative to where the decoder is
- any other error, such as a lost database connection, stops the decode and is returned from
  `Decode`
- an error that mixes issues with any other error is a failure of the program too, and the issues
  in it are hidden, so `errors.AsType[*raoh.Issues]` never takes it for invalid input

### `Issue` and `Issues`

Each issue has:

- `Path()`: a JSON Pointer (RFC 6901), such as `/items/0/name`
- `Code()`: what kind of problem it is, such as `required` or `out_of_range`
- `MessageKey()`: the code, or a refinement of it such as `out_of_range.minimum`
- `Meta()`: what else the code says, such as `min`, `max` and `actual`

An issue carries no sentence of its own. `issue.Message(raoh.English)` writes one from the
English catalogue, and `issue.Message(raoh.Japanese)` from another. The only sentence an issue
carries is one its creator gave with `WithMessage(...)`, which every language then shows as
written.

The codes, message keys and meta keys are the same as in Raoh for Java from 0.8 on, and in its
Rust and PHP ports, so the same client-side handling works for all of them, and a catalogue
written for Raoh for Java resolves these issues too. `compat_test.go` holds the decoders to what
Raoh for Java gives for the same inputs; the cases where it differs on purpose are listed there and
under [Differences from Raoh for Java](#differences-from-raoh-for-java).

`Issues` keeps them in the order they were found. `Render(r)` gives the
`[{"path", "code", "message", "meta"}]` form, ready for `json.Marshal`, and `Flatten(r)` groups
the messages by path. `Error()` writes each issue as its path and English message. `Issues` does
not implement `json.Marshaler`, because which language to write it in is the caller's choice.

## Objects

An object is decoded from a set of fields. `raoh.Fields()` is an empty set, each `.Field(name,
source)` adds one, and `raoh.Object(set)` gives the builder whose `Map` or `AndThen` takes a
function of the fields' values, in the order they were added:

```go
var period = raoh.Object(
	raoh.Fields().
		Field("start", raoh.String().AndThen(parseDate)).
		Field("end", raoh.String().AndThen(parseDate)),
).AndThen(NewPeriod)
```

The compiler checks that the function takes the fields' types in that order, and that each field
is read by a decoder of the input values; a `Decoder[string, T]` is not accepted as a field. A set
holds up to 16 components, each a field or a `Flat`, which is the arity of the function given to
`Map`. Moving some fields into a decoder of their own and adding it with `Flat` reads an object of
any number of fields, and the compiler checks the function of every level:

```go
contact := raoh.Object(raoh.Fields().Field("email", emailDecoder).Field("phone", phoneDecoder)).
	Map(NewContact)

account := raoh.Object(raoh.Fields().Field("id", idDecoder).Flat(contact)).Map(NewAccount)
```

`Flat` reads the same object as the fields, at the same path, so `{"id": 1, "email": "…", "phone": "…"}`
is one object. It is `flat` of Raoh for Java; `CombinerList`, which takes an untyped `Object[]`, has
no counterpart. The members a `Flat` decoder reads are not known, so an object with a `Flat` has no
`Strict` method.

`Object` requires its input to be an object. Anything else is one issue at the object's own path:
`required` for missing or `null`, `type_mismatch` otherwise. Every field is read and the issues of
all of them are reported. `.Strict()` also reports every member the fields do not declare as
`unknown_field`, after the fields' own issues.

A field is read with one of:

- a decoder: a member that must be there; a missing one is `required`
- `raoh.Optional(d)`: `*T`, `nil` when the member is missing
- `raoh.PresenceOf(d)`: `Presence[T]`, telling a missing member, a `null` one and one with a
  value apart, as a PATCH request needs

`raoh.Nullable(d)` is a decoder of `*T` that accepts `null`. A missing member and a `null` one are
different inputs: `Field("note", raoh.Nullable(raoh.String()))` accepts `null` but reports a
missing member as `required`, while `raoh.Optional` accepts a missing member but not `null`.

## Built-in decoders

Missing or `null` input is `required` for every one of them, and a value of another type is
`type_mismatch`. The constraints of one decoder run in the order written, and the first to fail is
reported.

`raoh.String()`: `Trim`, `ToLower`, `ToUpper`, `Normalize`, `NormalizeAs`, `NonBlank`, `MinLength`, `MaxLength`, `Length`,
`StartsWith`, `EndsWith`, `Contains`, `OneOf`, `Email`, `IP`, `IPv4`, `IPv6`, `ULID`, `CUID`,
`Pattern`, and the conversions `UUID()`, `URL()` and `URI()`. Lengths count characters, not bytes.

`ToInt()`, `ToLong()`, `ToDecimal()` and `ToBool()` read the string as that type and continue as its
decoder, so `String().MaxLength(40).ToDecimal().Positive()` is one decoder. Text that does not
convert is `type_mismatch` with `expected` set to `integer`, `long`, `decimal` or `boolean`, and
`Message` after the conversion is the message of that issue alone. `ToInt()` is a 32-bit integer
and `ToLong()` a 64-bit one, as in Raoh for Java, and not Go's `int`, which `Int()` reads and
whose width depends on the platform. `URI()` is `URL()` without the http or https scheme and the
host: any scheme, but a scheme is required. Both give a `raoh.URI`, which holds the text as it was
written and does not depend on `net/url` for what is a URI, so `http://%41.example/` is accepted
though `url.Parse` refuses it; `URI.URL()` converts to a `*url.URL` and can fail.

`raoh.Int()`, `Int32()`, `Int64()`, `Uint()`, `Uint32()`, `Uint64()`: `Min`, `Max`, `Range`,
`Positive`, `Negative`, `NonNegative`, `NonPositive`, `MultipleOf`, `OneOf`. In JSON text a number
with a fraction or an exponent is not an integer (`type_mismatch`), and one the type cannot hold
is `type_mismatch` under `type_mismatch.numeric_range`. Issues name the type in `expected` as Raoh
for Java names the type of the same width: `integer` for 32 bits and `long` for 64, so a Go `int`
is a `long`.

`raoh.Float64()` and `raoh.Float32()`: `Min`, `Max`, `Range`, `Positive`, `Negative`, `NonNegative`,
`NonPositive`, `OneOf`. A number beyond the range of the type is `type_mismatch` under
`type_mismatch.numeric_range`, with `expected` `double` or `float`. Constraints compare as Java's
`Double.compare` and `Float.compare` do, so `-0` is below `0` and `NaN` is above every other value,
and `Range` panics when `min` is above `max`. `OneOf`, for every type, panics when a value is
repeated, as Raoh for Java refuses it. A `Float32` reads a decimal in JSON text as a double
first and rounds that to a float, as Raoh for Java does.

`raoh.Bytes()` reads a `[]byte` handed to the decoder as a Go value, such as a binary column read
from a database, and returns it without copying. JSON text is never read as bytes: a JSON array or a
Base64 string is not one.

`raoh.DecimalNumber()`: the numeric constraints plus `MultipleOf` and `Scale`, decoding into
`raoh.Decimal`, which holds a number as written as Java's `BigDecimal` does: `1.20` keeps its
scale of 2.

`raoh.Bool()`: `IsTrue`, `IsFalse`.

`raoh.String().Instant()`, `.Date()`, `.Time()`, `.DateTime()`, `.OffsetDateTime()`: the ISO 8601
forms Java's `Instant`, `LocalDate`, `LocalTime`, `LocalDateTime` and `OffsetDateTime` read, with
`Before`, `After` and `Between`. Each gives a `time.Time`. The kinds without an offset give it in
UTC, as `time.Parse` does for text without one: a date is its midnight, and a clock time is on
January 1 of year 0. An offset date-time keeps its offset as a fixed zone, and an instant is given
in UTC. The text accepted is the text Raoh for Java accepts: a year outside 0000 to 9999 takes a
sign, `T` and `Z` are upper case only, a date that does not exist is refused, and an instant
refuses second 60 and reads `24:00:00` as the start of the next day. Bounds are compared by the
fields the kind has, so a date bound is compared by its date alone. In an issue, a bound is
written in a message as the Java type's `toString` writes it (`09:00`) and in JSON as Jackson
writes it (`09:00:00`), as Raoh for Java gives them.

`raoh.List(d)`: `[]T`, with `NonEmpty`, `MinSize`, `MaxSize`, `Size`, `Contains`, `ContainsAll`,
`Unique` and `UniqueBy`. Every element is decoded and its issues reported under its index.
`Contains`, `ContainsAll` and `Unique` compare elements with `==` and are refused, when the decoder
is built, for an element type that holds an interface, such as `any`, whose values could panic when
compared; `UniqueBy(key)` compares a key instead. Pointers are compared by identity, `0.0` and
`-0.0` are equal, and NaN is not equal to itself, none of which Java's `equals` gives. `Contains`
and `ContainsAll` are refused a nil element, and `ContainsAll` no element, as Java refuses a null.
Java has no custom message for `containsAll`; here `.Message` works on it as on every constraint.

`raoh.ToSet(d)` turns a decoder of `[]T`, such as `raoh.List(...).MaxSize(3)`, into a decoder of
`map[T]struct{}`. The constraints of `d` run first, then the elements are gathered, each once. The
order of the list is not kept, unlike Java's `toSet`.

`raoh.Dict(d)`: a `map[string]T` from an object used as a map, with `NonEmpty`, `MinSize`,
`MaxSize` and `Size`, which report `too_small`, `too_big` and `invalid_size` as a list does. They
run after every member has decoded.

Every built-in decoder takes `.Message("...")`, which gives the most recent constraint written
before it a custom message. Transformations such as `Trim` cannot fail and are passed over, so
`raoh.String().Trim().Message("...")` gives the message to the type check.

Whitespace, character counts, string order, case mapping and number formatting follow Raoh for
Java 0.8: `Trim` and `NonBlank` use Unicode's `White_Space`, lengths count code points, `OneOf`,
`Discriminate` and `EnumOf` sort by code point, `EnumOf` folds ASCII case only, `ToLower` and
`ToUpper` use Unicode's full case mapping as `Locale.ROOT` does (`ß` becomes `SS`, a final `Σ`
becomes `ς`), `Normalize` and `NormalizeAs` give what `java.text.Normalizer` gives on Java 25
(Unicode 16.0) from tables generated from it, and a fractional bound appears in a message as `Double.toString` writes it, such as
`1.0E7`. `Email`, `IP`, `URL` and `UUID` accept the text Raoh for Java accepts, decided by the
same grammar rather than by a parser of the platform.

## Choices

- `raoh.EnumOf(map[string]T{...})`: a string naming one of the values, ignoring ASCII case
- `raoh.Literal("v1")`: exactly that string
- `raoh.OneOf(a, b, ...)`: the first alternative that decodes, or `one_of_failed` with each
  alternative's issues in `meta.candidates`
- `raoh.Discriminate("type", raoh.Variant("a", da), raoh.Variant("b", db), ...)`: the variant the
  member `type` names

## Defaults and recovery

`d.Default(v)` gives `v` when the input is missing or `null`, and still reports any other problem.
`d.Fallback(v)` gives `v` whatever issues `d` reports. Neither hides a failure of the program.
`d.DefaultFunc(f)` and `d.FallbackFunc(f)` compute the value instead: `f` is called only when the
default or the fallback is needed, and the function given to `FallbackFunc` receives the `Issues`
`d` reported, at their paths in the whole input.

## Functions that see the path

A function given to `NewDecoder`, `AndThen` or an object's `AndThen` does not know where the value
is, so the issues it returns are read as relative to the decoder's path. When it needs the path,
for instance to report an issue beside the value, use the form that takes it:

```go
raoh.Object(raoh.Fields().
	Field("start", raoh.Int()).
	Field("end", raoh.Int()),
).AndThenWithPath(func(start, end int, at raoh.Path) (Period, error) {
	if start <= end {
		return Period{start, end}, nil
	}
	return Period{}, raoh.Invalid(raoh.NewIssue("start_after_end").At(at.Key("end")))
})
```

`NewDecoderWithPath`, `Decoder.AndThenWithPath`, `Decoder.RefineWithPath` and an object's
`AndThenWithPath` give the function the `Path` as its last parameter. The issues they return keep
their paths as the function wrote them, so build each from the path it was given. The `Path` is an
immutable value that decoding already carries, so passing it costs nothing more.
`d.RefineWithMeta(ok, code, message, meta)` is `Refine` with metadata computed from the value, and
only when `ok` does not hold.

## Recursive structures

A decoder that refers to itself does so through `raoh.Lazy`:

```go
func categoryDecoder() raoh.Decoder[any, Category] {
	return raoh.Object(raoh.Fields().
		Field("name", raoh.String().NonBlank()).
		Field("children", raoh.List(raoh.Lazy(categoryDecoder))),
	).Map(NewCategory)
}
```

## Encoders

Package `encode` turns domain values back into the `map[string]any`, `[]any` and scalar values
that the decoders read and `encoding/json` writes. `encoding/json` cannot write a domain type whose
fields are unexported; an encoder reads it through its accessors, and is written next to its
decoder:

```go
var userEncoder = encode.Object(
	encode.Property("email", User.Email, encode.String().Contramap(Email.String)),
	encode.Property("age", User.Age, encode.Int()),
	encode.OptionalProperty("nickname", User.Nickname, encode.String()),
	encode.PresenceProperty("note", User.Note, encode.String()),
)

body, err := json.Marshal(userEncoder.Encode(user))
```

- `Property`, `NullableProperty` (writes `null` for `nil`), `OptionalProperty` (leaves the member
  out for `nil`) and `PresenceProperty` are the counterparts of a field, `raoh.Nullable`,
  `raoh.Optional` and `raoh.PresenceOf`, so what one decodes the other writes back.
  `PropertyWithDefault` writes a default, encoded like any other value, for `nil` instead of
  `null`; `PropertyWithDefaultFunc` makes the default only when `nil` calls for it.
- `Object`, `List`, `Dict`, `Lazy` and `Discriminate` are the counterparts of the decoders of the
  same name. `Discriminate` picks the variant by the value's dynamic type and writes its tag.
- `String`, `Int` and the other scalars give a value as it is; `Decimal` gives a `json.Number`
  that keeps the scale; `UUID`, `URI` (for the `raoh.URI` that `String().URI()` and
  `String().URL()` give), `URL` (for a `*url.URL`) and the temporal encoders `Instant`, `Date`,
  `Time`, `DateTime` and `OffsetDateTime` give text as Raoh for Java's encoders do. `EnumOf` takes the
  map `raoh.EnumOf` decodes with.
- `Contramap` and `AndThen` adapt an encoder on either side.

An encoder cannot fail. A definition that does not cover a value, such as a `Discriminate` given a
type none of its variants names, is a mistake in the program and panics.

## Messages in other languages

`raoh.English` and `raoh.Japanese` hold the catalogues Raoh for Java ships, word for word, plus a
template for `invalid_format.json`. A catalogue is a stack of layers, as a locale's `.properties`
file sits over its parent's: `Japanese` is a layer over `English`, `WithOverrides` puts a layer of
your own on top, and `FallingBackTo` puts another catalogue beneath. An issue is looked up one
layer at a time, by message key and then by code, so a layer that translates only
`invalid_format` wins over the refined `invalid_format.email` beneath it, as in Raoh for Java.
`raoh.ParseProperties` reads a `.properties` file as Java's `Properties.load` does, `\uXXXX`
escapes included, so an existing Raoh for Java catalogue can be used as it is. A template's
`{name}` placeholders are filled from `meta`.

```go
ours := raoh.English.WithOverrides(map[string]string{"too_short": "{min} characters or more"})
issue.Message(ours) // "3 characters or more"
```

Any `func(raoh.Issue) string` is a resolver too, as a `raoh.ResolverFunc`.
`raoh.Interpolate` and `raoh.InterpolateFully` fill a template from `meta` the way the catalogues
do: the first keeps a placeholder whose entry is missing, the second gives up.

An `Issues` can be reshaped without being changed: `Add`, `Merge` and `Rebase(prefix)` return a
new one, `GroupByPath` groups by JSON Pointer, and `Format(r)` returns Raoh for Java's nested
`_errors` tree. `_errors` is reserved there, so a member of that name cannot be told from the
messages; read the issues from `All`, `Render` or `GroupByPath`. `raoh.PathOf("a", "b")` builds a
path from segments, and `Path.Append` joins two paths. A segment is text, so the member `"0"` and the
index 0 are the same segment. A `Path` is compared with `Equal`, not `==`.

Raoh for Java's `Issues.resolve(resolver)` and `MessageResolver.resolve(key, meta)` have no
counterpart. An issue here holds no sentence until one is asked for, so `Render`, `Flatten` and
`Format` do what `resolve` is for, and a `Resolver` reads the whole issue, message key included.
`Issues.EMPTY` is the zero `Issues`.

## Differences from Raoh for Java

In what it reports:

- `Object` checks once that its input is an object and reports one issue at its own path when it
  is not. Raoh for Java checks in each field, reporting `type_mismatch` at every field's path and
  reading a non-object as an object without any optional field.
- A Go float given as input, as `encoding/json` gives every number, is an integer when it holds
  one. In JSON text read by `DecodeJSON`, `1.0` is not an integer, as in Raoh for Java.
- `DecodeJSON` refuses an object that names a member twice, as `invalid_format.json`.

In the API:

- Combining is done with `Object(Fields().Field(...)...)`, not `combine`.
- `flatMap` is `AndThen`, `flatMapWithPath` is `AndThenWithPath`, and `recover` is `Fallback`, since `recover` means panic recovery in Go.
  There are no `Result`, `Ok` or `Err` types: decoding gives `(T, error)`.
- The temporal decoders give a `time.Time` for every kind, as Go has no separate date and time
  types, and there is no `Year`, `YearMonth` or `ZonedDateTime` decoder.
- Encoders write `map[string]any`, whose members `encoding/json` writes in sorted order, where Raoh
  for Java's keep the order the entries are written in.
- There is no domain construction guard (`raoh-gsh`). Unexported fields and package boundaries
  stop a domain value from being built outside its package, but the zero value of a struct can
  still be made anywhere.

## Development

```sh
go test ./...
```

`compat_test.go` also covers the temporal decoders, with cases raoh-rust does not have.

`go generate ./...` writes `fields_gen.go`, the field sets and object builders for up to 16
components, and `casing_table.go`, the code points where Java's case mapping differs from Go's.

`scripts/compat/generate.sh` regenerates `testdata/compat/expected.json` and copies the message
catalogues from the Raoh for Java version `scripts/compat/pom.xml` names.
`scripts/casing/generate.sh` regenerates the case-mapping data from the Java on the `PATH`. Both
need Java 25; the first also needs Maven.

## License

Apache License 2.0. The Unicode data the normalization and case mapping are built from, and
the Unicode Consortium's test file that holds them to Unicode, are under the Unicode License; see
[THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES).
