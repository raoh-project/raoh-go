package raoh

import (
	"cmp"
	"slices"
)

// step is one thing a built-in decoder does to its value: change it, which
// cannot fail, or check it, which can and may carry a custom message.
type step[T any] struct {
	transform func(T) T
	check     func(T) *Issue
	message   *string
}

// scalar is a built-in decoder as the steps it takes: a read of the input,
// which fails when the input is not of the type read, then the transformations
// and constraints in the order they were added. The first constraint to fail
// stops the rest.
type scalar[T any] struct {
	read        func(in any) (T, *Issue)
	from        *scalar[string]
	parse       func(string) (T, *Issue)
	steps       []step[T]
	baseMessage *string
}

func (s scalar[T]) with(st step[T]) scalar[T] {
	s.steps = append(slices.Clip(s.steps), st)
	return s
}

func (s scalar[T]) transform(f func(T) T) scalar[T] {
	return s.with(step[T]{transform: f})
}

// require adds a constraint that fails with the issue fail makes when ok does
// not hold.
func (s scalar[T]) require(ok func(T) bool, fail func(T) Issue) scalar[T] {
	return s.with(step[T]{check: func(v T) *Issue {
		if ok(v) {
			return nil
		}
		i := fail(v)
		return &i
	}})
}

// message gives the most recent constraint, or the type check when there is
// none, a custom message. Transformations are passed over: they cannot fail,
// so a message on one would never show.
func (s scalar[T]) message(m string) scalar[T] {
	s.steps = slices.Clone(s.steps)
	for i := len(s.steps) - 1; i >= 0; i-- {
		if s.steps[i].check != nil {
			s.steps[i].message = &m
			return s
		}
	}
	s.baseMessage = &m
	return s
}

func withCustom(i Issue, message *string) Issue {
	if message != nil {
		return i.WithMessage(*message)
	}
	return i
}

// viaString is a scalar that reads a string with from and converts it with
// parse. Only the issue parse gives is the type check's: it takes the custom
// message given to Message before any constraint, and the issues of from keep
// their own.
func viaString[T any](from scalar[string], parse func(string) (T, *Issue)) scalar[T] {
	return scalar[T]{from: &from, parse: parse}
}

func (s scalar[T]) build() Decoder[any, T] {
	if s.from != nil {
		str := s.from.build()
		return Decoder[any, T]{func(in any, at Path) outcome[T] {
			o := str.run(in, at)
			if o.failed() {
				return failAs[T](o)
			}
			v, issue := s.parse(o.value)
			if issue != nil {
				return s.typeIssue(*issue, at)
			}
			return s.run(v, at)
		}}
	}
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		v, issue := s.read(in)
		if issue != nil {
			return s.typeIssue(*issue, at)
		}
		return s.run(v, at)
	}}
}

// typeIssue is the outcome of an input the read refused, with the custom
// message of the type check if one was given.
func (s scalar[T]) typeIssue(i Issue, at Path) outcome[T] {
	return invalid[T](withCustom(i.At(at), s.baseMessage))
}

// run applies the steps to v.
func (s scalar[T]) run(v T, at Path) outcome[T] {
	for _, st := range s.steps {
		if st.transform != nil {
			v = st.transform(v)
			continue
		}
		if issue := st.check(v); issue != nil {
			return invalid[T](withCustom(issue.At(at), st.message))
		}
	}
	return succeed(v)
}

// sortedUnique returns the values sorted and without repeats.
func sortedUnique[T cmp.Ordered](values []T) []T {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}
