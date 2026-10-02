package raoh

// Strict returns d that also reports unknown_field, at the member, for each
// member of the input that is not one of fields, in the order the input has its
// members, after the issues of d.
//
// A member a Strict inside d already reported, or the Strict of an Object
// inside it, is not reported again: a strict inside a strict reports a member
// once, by the innermost one that does not know it, and still accepts only the
// members every one of them knows. Only an unknown_field such a Strict made
// counts. One a function of the caller's returns, an issue of another code at
// the member, and the issues of a one_of_failed's candidates do not.
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
		var names []string
		if m, ok := AsObject(in); ok {
			names = m.names
		}
		o := dec.run(in, at)
		if o.err != nil {
			return o
		}
		var reported map[string]struct{}
		for _, i := range o.issues.items {
			if i.byStrict && i.code == CodeUnknownField {
				if reported == nil {
					reported = map[string]struct{}{}
				}
				reported[i.path.String()] = struct{}{}
			}
		}
		var unknown Issues
		for _, k := range names {
			if _, ok := known[k]; ok {
				continue
			}
			p := at.Key(k)
			if _, ok := reported[p.String()]; !ok {
				unknown.appendInPlace(unknownMember(k, p))
			}
		}
		if unknown.Len() == 0 {
			return o
		}
		// What d returned may be shared, so it is merged, not appended to.
		o.issues = o.issues.Merge(unknown)
		return o
	}}
}
