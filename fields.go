package raoh

import "slices"

// part reads one component of an object: a named field, or the whole input
// for a component added with Flat. in is the input the object decoder was
// given; m is its members.
type part[T any] func(in any, m *JSONObject, at Path) outcome[T]

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
	// notAnObject is what the field gives where the input it is a member of,
	// in, is not an object, at the path of the member.
	notAnObject(in any, at Path) outcome[T]
	// valid reports whether the source has a decoder to read with, which the
	// zero value of a source has not.
	valid() bool
}

// decodeAt carries the input type I in its signature, which is what keeps a
// Decoder[I, T] with I other than any out of [FieldSource].
func (d Decoder[I, T]) decodeAt(in I, at Path) outcome[T] { return d.run(in, at) }

func (d Decoder[I, T]) valid() bool { return d.run != nil }

// notAnObject is type_mismatch with expected object, for a required field.
func (d Decoder[I, T]) notAnObject(in any, at Path) outcome[T] {
	return invalid[T](NewIssue(CodeTypeMismatch).WithMeta("expected", "object").WithMeta("actual", kind(in)).At(at))
}

// notAnObject is nil: an optional field reads an input that is not an object
// as not having the member.
func (s OptionalSource[T]) notAnObject(any, Path) outcome[*T] { return succeed[*T](nil) }

func (s OptionalSource[T]) valid() bool { return s.d.run != nil }

func toPart[T any](name string, src FieldSource[T]) part[T] {
	requireArgument(src != nil && src.valid(), "Field", "src")
	return func(in any, m *JSONObject, at Path) outcome[T] {
		if m == nil {
			return src.notAnObject(in, at.Key(name))
		}
		return src.decodeAt(m.member(name), at.Key(name))
	}
}

// toFlatPart reads with d the same input as the object, at the object's path.
func toFlatPart[T any](d DecoderOf[T]) part[T] {
	dec := decoderOf(d, "Flat", "d")
	return func(in any, _ *JSONObject, at Path) outcome[T] { return dec.run(in, at) }
}

// OptionalSource is the [FieldSource] that [Optional] returns.
type OptionalSource[T any] struct {
	d Decoder[any, T]
}

// Optional reads a field that may be left out: a missing member gives nil, and
// a present one, null included, is decoded with d. Wrap d with [Nullable] to
// accept null, or use [PresenceOf] to tell the three apart.
func Optional[T any](d DecoderOf[T]) OptionalSource[T] {
	return OptionalSource[T]{decoderOf(d, "Optional", "d")}
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
//
// A set holds up to 16 components, each a Field or a Flat: the arity of the
// function given to Map. An object of more fields is read by moving some of
// them into a decoder of its own and adding that with Flat, which reads the
// same object:
//
//	raoh.Object(
//		raoh.Fields().
//			Field("id", idDecoder).
//			Flat(contactDecoder),
//	).Map(NewAccount)
//
// Each level has its constructor checked by the compiler, so the number of
// fields is not limited. The decoder Flat is given is opaque, so the members
// it reads are not known: an object with a Flat has no Strict method.
func Fields() fields0 { return fields0{} }

// Object returns the builder of the decoder of an object made of the fields in
// f. Call Map or AndThen on it with a function whose parameters are the types
// of the components, in the order they were added; Strict also reports the
// members the fields do not name.
//
// Each field checks for itself that the input is an object, and every field is
// read and the issues of all of them reported, in the order they were added.
// Where the input is not an object, null and a missing member included, a
// field read with a decoder is type_mismatch with expected object at the
// member's path, a field read with [Optional] gives nil and one read with
// [PresenceOf] an absent Presence, and a component added with Flat is given the
// input as it is. Strict reports nothing of an input that has no members.
func Object[F interface{ object() O }, O any](f F) O { return f.object() }

// collector gathers the outcomes of the fields of one object.
type collector struct {
	issues Issues
	err    error
}

func read[T any](c *collector, p part[T], in any, m *JSONObject, at Path) T {
	var zero T
	if c.err != nil {
		return zero
	}
	o := p(in, m, at)
	if o.err != nil {
		c.err = o.err
		return zero
	}
	c.issues.appendInPlace(o.issues.items...)
	return o.value
}

// unknown reports unknown_field with the member's name as field for each
// member not in names, in the order the input has its members.
func (c *collector) unknown(m *JSONObject, at Path, names []string) {
	for _, k := range m.names {
		if !slices.Contains(names, k) {
			c.issues.appendInPlace(unknownMember(k, at.Key(k)))
		}
	}
}

func (c *collector) failed() bool { return c.err != nil || c.issues.Len() > 0 }

func collected[R any](c *collector) outcome[R] {
	return outcome[R]{issues: c.issues, err: c.err}
}
