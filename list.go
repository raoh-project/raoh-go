package raoh

import (
	"fmt"
	"reflect"
)

// ListDecoder decodes an array into a []T.
//
// Null or a missing member is required; any other type is type_mismatch. Every
// element is decoded, and the issues of each are reported under its index. The
// constraints on the list run once every element has decoded, in the order
// they are written.
type ListDecoder[T any] struct {
	Decoder[any, []T]
	element Decoder[any, T]
	s       scalar[[]T]
}

// List returns a decoder of an array whose elements element decodes.
func List[T any](element DecoderOf[T]) ListDecoder[T] {
	return newList(element.decoder(), scalar[[]T]{read: func(in any) ([]T, *Issue) {
		if _, ok := in.([]any); !ok {
			i := unexpected("array", in)
			return nil, &i
		}
		return nil, nil
	}})
}

func newList[T any](element Decoder[any, T], s scalar[[]T]) ListDecoder[T] {
	return ListDecoder[T]{Decoder[any, []T]{func(in any, at Path) outcome[[]T] {
		if _, issue := s.read(in); issue != nil {
			return s.typeIssue(*issue, at)
		}
		items := in.([]any)
		values := make([]T, 0, len(items))
		var issues Issues
		for n, item := range items {
			o := element.run(item, at.Index(n))
			if o.err != nil {
				return failAs[[]T](o)
			}
			issues.add(o.issues.items...)
			values = append(values, o.value)
		}
		if issues.Len() > 0 {
			return outcome[[]T]{issues: issues}
		}
		return s.run(values, at)
	}}, element, s}
}

func (l ListDecoder[T]) require(ok func([]T) bool, fail func([]T) Issue) ListDecoder[T] {
	return newList(l.element, l.s.require(ok, fail))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (l ListDecoder[T]) Message(message string) ListDecoder[T] {
	return newList(l.element, l.s.message(message))
}

// NonEmpty requires an element: too_small under the message key
// too_small.nonempty, with min 1 and actual 0.
func (l ListDecoder[T]) NonEmpty() ListDecoder[T] {
	return l.require(func(v []T) bool { return len(v) > 0 }, func([]T) Issue {
		return NewIssue(CodeTooSmall).WithMessageKey(KeyTooSmallNonEmpty).WithMeta("min", 1).WithMeta("actual", 0)
	})
}

// MinSize requires at least n elements: too_small with min and actual.
func (l ListDecoder[T]) MinSize(n int) ListDecoder[T] {
	return l.require(func(v []T) bool { return len(v) >= n }, func(v []T) Issue {
		return NewIssue(CodeTooSmall).WithMeta("min", n).WithMeta("actual", len(v))
	})
}

// MaxSize allows at most n elements: too_big with max and actual.
func (l ListDecoder[T]) MaxSize(n int) ListDecoder[T] {
	return l.require(func(v []T) bool { return len(v) <= n }, func(v []T) Issue {
		return NewIssue(CodeTooBig).WithMeta("max", n).WithMeta("actual", len(v))
	})
}

// Size requires exactly n elements: invalid_size with expected and actual.
func (l ListDecoder[T]) Size(n int) ListDecoder[T] {
	return l.require(func(v []T) bool { return len(v) == n }, func(v []T) Issue {
		return NewIssue(CodeInvalidSize).WithMeta("expected", n).WithMeta("actual", len(v))
	})
}

// Unique requires every element to differ from the others, compared with ==:
// duplicate_element with duplicates, each repeated element once, in the order
// it was first repeated. It panics when T is not comparable.
func (l ListDecoder[T]) Unique() ListDecoder[T] {
	if t := reflect.TypeFor[T](); !t.Comparable() {
		panic(fmt.Sprintf("raoh: Unique needs comparable elements, not %v", t))
	}
	return l.require(func(v []T) bool { return len(duplicates(v)) == 0 }, func(v []T) Issue {
		return NewIssue(CodeDuplicateElement).WithMeta("duplicates", duplicates(v))
	})
}

func duplicates[T any](items []T) []T {
	seen := map[any]bool{}
	repeated := map[any]bool{}
	var out []T
	for _, item := range items {
		if seen[item] && !repeated[item] {
			repeated[item] = true
			out = append(out, item)
		}
		seen[item] = true
	}
	return out
}

// Dict returns a decoder of an object used as a map, whose member values value
// decodes. Null or a missing member is required; any other type is
// type_mismatch. The issues of each member are reported under its name, in the
// order the input has its members.
func Dict[T any](value DecoderOf[T]) Decoder[any, map[string]T] {
	d := value.decoder()
	return Decoder[any, map[string]T]{func(in any, at Path) outcome[map[string]T] {
		m, ok := asMembers(in)
		if !ok {
			return invalid[map[string]T](unexpected("object", in).At(at))
		}
		values := make(map[string]T, len(m.names))
		var issues Issues
		for _, name := range m.names {
			o := d.run(m.values[name], at.Key(name))
			if o.err != nil {
				return failAs[map[string]T](o)
			}
			issues.add(o.issues.items...)
			values[name] = o.value
		}
		if issues.Len() > 0 {
			return outcome[map[string]T]{issues: issues}
		}
		return succeed(values)
	}}
}
