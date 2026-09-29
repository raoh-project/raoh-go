package raoh

// Bytes returns a decoder of a byte slice, the counterpart of Raoh for Java's
// ObjectDecoders.bytes. It is for a value a Go program already holds as bytes,
// such as a binary column read from a database.
//
// Null or a missing member is required; any other type is type_mismatch with
// expected byte[]. The slice is returned as it is, not copied. Text is not
// read as bytes: DecodeJSON never gives a byte slice, and a JSON array or
// Base64 string is not one.
func Bytes() Decoder[any, []byte] {
	return scalar[[]byte]{read: func(in any) ([]byte, *Issue) {
		b, ok := plain(in).([]byte)
		if !ok {
			i := unexpected("byte[]", in)
			return nil, &i
		}
		return b, nil
	}}.build()
}
