package raoh

// Strict returns d that also reports unknown_field, at the member, for each
// member of the input that is not one of fields, in the order the input has its
// members, after the issues of d.
//
// It is the boundary of what d accepts, apart from what d reads: a [Discriminate]
// whose variants declare different fields can be made strict with the fields of
// all of them, and each variant with its own. An input that is not an object has
// no members, so Strict adds nothing to what d reports for it. It is the same as
// the Strict method of an [Object] for a decoder that is not one.
func Strict[T any](d DecoderOf[T], fields ...string) Decoder[any, T] {
	dec := d.decoder()
	known := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		known[f] = struct{}{}
	}
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		o := dec.run(in, at)
		if o.err != nil {
			return o
		}
		m, ok := AsObject(in)
		if !ok {
			return o
		}
		for _, k := range m.names {
			if _, ok := known[k]; !ok {
				o.issues.appendInPlace(NewIssue(CodeUnknownField).WithMeta("field", k).At(at.Key(k)))
			}
		}
		return o
	}}
}
