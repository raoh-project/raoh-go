package raoh

import "slices"

// MaxFields is the number of fields one set can hold. A larger object is
// decoded by grouping some of its fields into a nested object.
const MaxFields = 16

// part reads one field of an object.
type part[T any] struct {
	name string
	src  FieldSource[T]
}

// FieldSource is what a field is read with: a Decoder[any, T] such as
// raoh.String(), or [Optional] or [PresenceOf]. A decoder of any other input
// type, such as a Decoder[string, T], does not satisfy it, so the mistake is
// found by the compiler.
//
// A missing member is handed to the source as a value of its own, told apart
// from null: a decoder reports it as required, Optional gives nil for it and
// PresenceOf an absent Presence.
type FieldSource[T any] interface {
	decodeAt(in any, at Path) outcome[T]
}

// decodeAt carries the input type I in its signature, which is what keeps a
// Decoder[I, T] with I other than any out of [FieldSource].
func (d Decoder[I, T]) decodeAt(in I, at Path) outcome[T] { return d.run(in, at) }

func toPart[T any](name string, src FieldSource[T]) part[T] { return part[T]{name, src} }

// OptionalSource is the [FieldSource] that [Optional] returns.
type OptionalSource[T any] struct {
	d Decoder[any, T]
}

// Optional reads a field that may be left out: a missing member gives nil, and
// a present one, null included, is decoded with d. Wrap d with [Nullable] to
// accept null, or use [PresenceOf] to tell the three apart.
func Optional[T any](d DecoderOf[T]) OptionalSource[T] {
	return OptionalSource[T]{d.decoder()}
}

func (s OptionalSource[T]) decodeAt(in any, at Path) outcome[*T] {
	if in == missing {
		return succeed[*T](nil)
	}
	o := s.d.run(in, at)
	if o.failed() {
		return failAs[*T](o)
	}
	return succeed(&o.value)
}

// Fields returns an empty set of fields. Add fields with its Field method and
// pass the set to [Object]:
//
//	raoh.Object(
//		raoh.Fields().
//			Field("email", emailDecoder).
//			Field("age", ageDecoder),
//	).Map(NewUser)
func Fields() fields0 { return fields0{} }

// Object returns the builder of the decoder of an object made of the fields in
// f. Call Map or AndThen on it with a function whose parameters are the types
// of the fields, in the order they were added; Strict also reports the members
// the fields do not name.
//
// The input must be an object: null or a missing member is required, and any
// other type is type_mismatch with expected object, reported once at the
// object's own path. Once it is one, every field is read and the issues of all
// of them are reported.
func Object[F interface{ object() O }, O any](f F) O { return f.object() }

// openObject returns the input's members, or the issue why it is not an
// object.
func openObject(in any, at Path) (*JSONObject, *Issue) {
	o, ok := AsObject(in)
	if !ok {
		i := unexpected("object", in).At(at)
		return nil, &i
	}
	return o, nil
}

// collector gathers the outcomes of the fields of one object.
type collector struct {
	issues Issues
	err    error
}

func read[T any](c *collector, p part[T], m *JSONObject, at Path) T {
	var zero T
	if c.err != nil {
		return zero
	}
	o := p.src.decodeAt(m.member(p.name), at.Key(p.name))
	if o.err != nil {
		c.err = o.err
		return zero
	}
	c.issues.add(o.issues.items...)
	return o.value
}

// unknown reports unknown_field with the member's name as field for each
// member not in names, in the order the input has its members.
func (c *collector) unknown(m *JSONObject, at Path, names []string) {
	for _, k := range m.names {
		if !slices.Contains(names, k) {
			c.issues.add(NewIssue(CodeUnknownField).WithMeta("field", k).At(at.Key(k)))
		}
	}
}

func (c *collector) failed() bool { return c.err != nil || c.issues.Len() > 0 }

func collected[R any](c *collector) outcome[R] {
	return outcome[R]{issues: c.issues, err: c.err}
}
