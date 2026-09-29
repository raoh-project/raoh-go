package raoh

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/raoh-project/raoh-go/internal/hashable"
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
		if _, ok := plain(in).([]any); !ok {
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
		items := plain(in).([]any)
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
	return l.require(listSize[T]().nonEmpty())
}

// MinSize requires at least n elements: too_small with min and actual.
func (l ListDecoder[T]) MinSize(n int) ListDecoder[T] {
	return l.require(listSize[T]().atLeast(n))
}

// MaxSize allows at most n elements: too_big with max and actual.
func (l ListDecoder[T]) MaxSize(n int) ListDecoder[T] {
	return l.require(listSize[T]().atMost(n))
}

// Size requires exactly n elements: invalid_size with expected and actual.
func (l ListDecoder[T]) Size(n int) ListDecoder[T] {
	return l.require(listSize[T]().exactly(n))
}

// Contains requires the list to hold element, compared with ==:
// missing_element with expected. It panics, when the decoder is built, if T
// holds an interface anywhere, as Unique does.
func (l ListDecoder[T]) Contains(element T) ListDecoder[T] {
	requireHashable(reflect.TypeFor[T](), "Contains")
	return l.require(func(v []T) bool { return slices.ContainsFunc(v, func(e T) bool { return equal(e, element) }) },
		func([]T) Issue { return NewIssue(CodeMissingElement).WithMeta("expected", element) })
}

// ContainsAll requires the list to hold every one of elements, compared with
// ==: missing_elements with expected, all of elements, and missing, those the
// list lacks, in the order given. It panics, when the decoder is built, if
// elements is empty or T holds an interface anywhere, as Unique does.
func (l ListDecoder[T]) ContainsAll(elements ...T) ListDecoder[T] {
	requireHashable(reflect.TypeFor[T](), "ContainsAll")
	if len(elements) == 0 {
		panic("raoh: ContainsAll needs at least one element")
	}
	required := slices.Clone(elements)
	missing := func(v []T) []T {
		var out []T
		for _, r := range required {
			if !slices.ContainsFunc(v, func(e T) bool { return equal(e, r) }) {
				out = append(out, r)
			}
		}
		return out
	}
	return l.require(func(v []T) bool { return len(missing(v)) == 0 }, func(v []T) Issue {
		return NewIssue(CodeMissingElements).WithMeta("expected", required).WithMeta("missing", missing(v))
	})
}

// ToSet turns the elements d decodes into a set: a map whose keys are the
// elements, each once. The order of the list is not kept. Constraints on d,
// such as Unique, run before the elements are gathered. It panics, when the
// decoder is built, if T holds an interface anywhere, as Unique does.
func ToSet[T comparable](d DecoderOf[[]T]) Decoder[any, map[T]struct{}] {
	requireHashable(reflect.TypeFor[T](), "ToSet")
	return d.decoder().Map(func(v []T) map[T]struct{} {
		set := make(map[T]struct{}, len(v))
		for _, e := range v {
			set[e] = struct{}{}
		}
		return set
	})
}

func equal[T any](a, b T) bool { return any(a) == any(b) }

// Unique requires every element to differ from the others, compared with ==:
// duplicate_element with duplicates, each repeated element once, in the order
// it was first repeated.
//
// It panics, when the decoder is built, if T holds an interface anywhere, such
// as any: comparing two interface values panics when they hold slices or maps,
// so the input could make the decode panic. Use UniqueBy with a key of a type
// that holds none.
func (l ListDecoder[T]) Unique() ListDecoder[T] {
	requireHashable(reflect.TypeFor[T](), "Unique")
	return l.unique(func(v T) any { return v })
}

// UniqueBy requires the elements to have different keys: duplicate_element
// with duplicates, each element whose key repeats an earlier one's, once per
// key, in the order the key was first repeated. It panics, when the decoder is
// built, if K holds an interface anywhere, as Unique does.
func (l ListDecoder[T]) UniqueBy[K comparable](key func(T) K) ListDecoder[T] {
	requireHashable(reflect.TypeFor[K](), "UniqueBy")
	return l.unique(func(v T) any { return key(v) })
}

func requireHashable(t reflect.Type, name string) {
	if !hashable.Type(t) {
		panic(fmt.Sprintf("raoh: %s needs a type that holds no interface, not %v", name, t))
	}
}

func (l ListDecoder[T]) unique(key func(T) any) ListDecoder[T] {
	return l.require(func(v []T) bool { return len(duplicates(v, key)) == 0 }, func(v []T) Issue {
		return NewIssue(CodeDuplicateElement).WithMeta("duplicates", duplicates(v, key))
	})
}

func duplicates[T any](items []T, key func(T) any) []T {
	seen := map[any]bool{}
	repeated := map[any]bool{}
	var out []T
	for _, item := range items {
		k := key(item)
		if seen[k] && !repeated[k] {
			repeated[k] = true
			out = append(out, item)
		}
		seen[k] = true
	}
	return out
}

// DictDecoder decodes an object used as a map into a map[string]T.
//
// Null or a missing member is required; any other type is type_mismatch. Every
// member is decoded, and the issues of each are reported under its name, in
// the order the input has its members. The constraints on the map run once
// every member has decoded, in the order they are written.
type DictDecoder[T any] struct {
	Decoder[any, map[string]T]
	value Decoder[any, T]
	s     scalar[map[string]T]
}

// Dict returns a decoder of an object used as a map, whose member values value
// decodes.
func Dict[T any](value DecoderOf[T]) DictDecoder[T] {
	return newDict(value.decoder(), scalar[map[string]T]{read: func(in any) (map[string]T, *Issue) {
		if _, ok := AsObject(in); !ok {
			i := unexpected("object", in)
			return nil, &i
		}
		return nil, nil
	}})
}

func newDict[T any](value Decoder[any, T], s scalar[map[string]T]) DictDecoder[T] {
	return DictDecoder[T]{Decoder[any, map[string]T]{func(in any, at Path) outcome[map[string]T] {
		if _, issue := s.read(in); issue != nil {
			return s.typeIssue(*issue, at)
		}
		m, _ := AsObject(in)
		values := make(map[string]T, len(m.names))
		var issues Issues
		for _, name := range m.names {
			o := value.run(m.values[name], at.Key(name))
			if o.err != nil {
				return failAs[map[string]T](o)
			}
			issues.add(o.issues.items...)
			values[name] = o.value
		}
		if issues.Len() > 0 {
			return outcome[map[string]T]{issues: issues}
		}
		return s.run(values, at)
	}}, value, s}
}

func (d DictDecoder[T]) require(ok func(map[string]T) bool, fail func(map[string]T) Issue) DictDecoder[T] {
	return newDict(d.value, d.s.require(ok, fail))
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written.
func (d DictDecoder[T]) Message(message string) DictDecoder[T] {
	return newDict(d.value, d.s.message(message))
}

// NonEmpty requires a member: too_small under the message key
// too_small.nonempty, with min 1 and actual 0.
func (d DictDecoder[T]) NonEmpty() DictDecoder[T] {
	return d.require(dictSize[T]().nonEmpty())
}

// MinSize requires at least n members: too_small with min and actual.
func (d DictDecoder[T]) MinSize(n int) DictDecoder[T] {
	return d.require(dictSize[T]().atLeast(n))
}

// MaxSize allows at most n members: too_big with max and actual.
func (d DictDecoder[T]) MaxSize(n int) DictDecoder[T] {
	return d.require(dictSize[T]().atMost(n))
}

// Size requires exactly n members: invalid_size with expected and actual.
func (d DictDecoder[T]) Size(n int) DictDecoder[T] {
	return d.require(dictSize[T]().exactly(n))
}
