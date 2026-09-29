package raoh

// Presence is a member of an object that may be missing, null or present with a
// value, as a PATCH request needs to tell apart. The zero value is absent.
type Presence[T any] struct {
	state presenceState
	value T
}

type presenceState int

const (
	absent presenceState = iota
	null
	present
)

// Absent returns a Presence of a missing member.
func Absent[T any]() Presence[T] { return Presence[T]{} }

// Null returns a Presence of a member given as null.
func Null[T any]() Presence[T] { return Presence[T]{state: null} }

// Present returns a Presence of a member given with v.
func Present[T any](v T) Presence[T] { return Presence[T]{state: present, value: v} }

// IsAbsent reports whether the member was missing.
func (p Presence[T]) IsAbsent() bool { return p.state == absent }

// IsNull reports whether the member was given as null.
func (p Presence[T]) IsNull() bool { return p.state == null }

// IsGiven reports whether the member was there, as null or with a value.
func (p Presence[T]) IsGiven() bool { return p.state != absent }

// Value returns the value the member was given with, and whether it was.
func (p Presence[T]) Value() (T, bool) { return p.value, p.state == present }

// PresenceOf reads a field that may be missing, null or present: a missing
// member gives an absent Presence, null a null one, and anything else is decoded
// with d.
func PresenceOf[T any](d DecoderOf[T]) PresenceSource[T] {
	return PresenceSource[T]{decoderOf(d, "PresenceOf", "d")}
}

// PresenceSource is the [FieldSource] that [PresenceOf] returns.
type PresenceSource[T any] struct {
	d Decoder[any, T]
}

func (s PresenceSource[T]) valid() bool { return s.d.run != nil }

func (s PresenceSource[T]) decodeAt(in any, at Path) outcome[Presence[T]] {
	switch {
	case in == missing:
		return succeed(Absent[T]())
	case in == nil:
		return succeed(Null[T]())
	}
	o := s.d.run(in, at)
	if o.failed() {
		return failAs[Presence[T]](o)
	}
	return succeed(Present(o.value))
}
