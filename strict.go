package raoh

// Strict returns d that also reports unknown_field, at the member, for each
// member of the input that is not one of fields, in the order the input has its
// members, after the issues of d.
//
// It is the boundary of what d accepts, apart from what d reads: a [Discriminate]
// whose variants declare different fields can be made strict with the fields of
// all of them, and each variant with its own. The members are those of the input
// before d runs. An input that is not an object has
// no members, so Strict adds nothing to what d reports for it. It is the same as
// the Strict method of an [Object] for a decoder that is not one.
func Strict[T any](d DecoderOf[T], fields ...string) Decoder[any, T] {
	dec := decoderOf(d, "Strict", "d")
	known := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		known[f] = struct{}{}
	}
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		// The members are read before d runs, so what d does to the input does
		// not change which members are unknown.
		var unknown Issues
		if m, ok := AsObject(in); ok {
			for _, k := range m.names {
				if _, ok := known[k]; !ok {
					unknown.appendInPlace(NewIssue(CodeUnknownField).WithMeta("field", k).At(at.Key(k)))
				}
			}
		}
		o := dec.run(in, at)
		if o.err != nil || unknown.Len() == 0 {
			return o
		}
		// What d returned may be shared, so it is merged, not appended to.
		o.issues = o.issues.Merge(unknown)
		return o
	}}
}
