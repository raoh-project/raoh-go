package raoh

// BoolDecoder decodes a boolean.
//
// Null or a missing member is required; any other type is type_mismatch.
type BoolDecoder struct {
	Decoder[any, bool]
	s scalar[bool]
}

// Bool returns a decoder of a boolean.
func Bool() BoolDecoder {
	return newBool(scalar[bool]{read: func(in any) (bool, *Issue) {
		b, ok := plain(in).(bool)
		if !ok {
			i := unexpected("boolean", in)
			return false, &i
		}
		return b, nil
	}})
}

func newBool(s scalar[bool]) BoolDecoder { return BoolDecoder{s.build(), s} }

func (d BoolDecoder) exactly(expected bool) BoolDecoder {
	return newBool(d.s.require(func(v bool) bool { return v == expected }, func(v bool) Issue {
		return NewIssue(CodeInvalidValue).WithMeta("expected", expected).WithMeta("actual", v)
	}))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d BoolDecoder) Message(message string) BoolDecoder {
	return newBool(d.s.message(message))
}

// IsTrue requires true: invalid_value with expected and actual.
func (d BoolDecoder) IsTrue() BoolDecoder { return d.exactly(true) }

// IsFalse requires false: invalid_value with expected and actual.
func (d BoolDecoder) IsFalse() BoolDecoder { return d.exactly(false) }
