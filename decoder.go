package raoh

import "errors"

// outcome is what a decoder produces at one step. Exactly one holds:
// success (no issues, nil err), invalid input (issues, nil err) or a failure
// of the program (non-nil err), which stops the decode at once.
type outcome[T any] struct {
	value  T
	issues Issues
	err    error
}

func (o outcome[T]) failed() bool { return o.err != nil || o.issues.Len() > 0 }

func succeed[T any](v T) outcome[T] { return outcome[T]{value: v} }

func invalid[T any](issues ...Issue) outcome[T] {
	return outcome[T]{issues: Issues{items: issues}}
}

// failAs carries the failure of o over to an outcome of another type.
func failAs[U, T any](o outcome[T]) outcome[U] {
	return outcome[U]{issues: o.issues, err: o.err}
}

// fromError turns what a caller's function returned into an outcome. An error
// made of issues only is invalid input, moved to at. Any other error is a
// failure of the program; when issues are mixed into it, it is hidden behind
// an executionError so that no *Issues can be found in what Decode returns.
func fromError[T any](v T, err error, at Path) outcome[T] {
	if err == nil {
		return succeed(v)
	}
	if is, ok := asIssues(err); ok {
		return invalid[T](is.under(at)...)
	}
	if _, mixed := errors.AsType[*Issues](err); mixed {
		err = &executionError{err}
	}
	return outcome[T]{err: err}
}

// executionError is a failure of the program whose error tree also holds
// issues. It has no Unwrap, so errors.As and errors.AsType never find the
// issues inside; errors.Is still sees every other error in the tree.
type executionError struct {
	cause error
}

func (e *executionError) Error() string { return e.cause.Error() }

func (e *executionError) Is(target error) bool {
	if _, ok := target.(*Issues); ok {
		return false
	}
	return errors.Is(e.cause, target)
}

// Decoder decodes an I into a T. A decoder holds no state and can be reused
// and shared between goroutines.
type Decoder[I, T any] struct {
	run func(in I, at Path) outcome[T]
}

// DecoderOf is a decoder of the input values Raoh reads, such as a
// Decoder[any, T] or a [StringDecoder]. The functions that combine decoders
// take one.
type DecoderOf[T any] interface {
	decoder() Decoder[any, T]
}

// decoder lets a type that embeds a Decoder be passed as a DecoderOf.
func (d Decoder[I, T]) decoder() Decoder[I, T] { return d }

// NewDecoder returns a decoder that runs f. An error made of issues (see
// [Invalid]) reports the input as invalid, with the issues read as relative to
// the path the decoder is at; any other error stops the decode.
func NewDecoder[I, T any](f func(in I) (T, error)) Decoder[I, T] {
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		v, err := f(in)
		return fromError(v, err, at)
	}}
}

// Decode decodes in. When the error holds an *Issues, the whole error means
// the input was invalid, so errors.AsType[*raoh.Issues] tells invalid input
// from a failure of the program. A failure of the program is returned as the
// function that produced it returned it, unless issues were mixed into it.
func (d Decoder[I, T]) Decode(in I) (T, error) {
	o := d.run(in, Path{})
	var zero T
	switch {
	case o.err != nil:
		return zero, o.err
	case o.issues.Len() > 0:
		return zero, &o.issues
	}
	return o.value, nil
}

// Map returns a decoder that applies f to the decoded value.
func (d Decoder[I, T]) Map[U any](f func(T) U) Decoder[I, U] {
	return Decoder[I, U]{func(in I, at Path) outcome[U] {
		o := d.run(in, at)
		if o.failed() {
			return failAs[U](o)
		}
		return succeed(f(o.value))
	}}
}

// AndThen returns a decoder that applies f, which may fail, to the decoded
// value. A rule that relates several parts runs here, once the parts have
// decoded. When f returns an error made of issues (see [Invalid]), the issues
// are reported as relative to the path this decoder is at. Any other error
// stops the decode and is returned from Decode.
func (d Decoder[I, T]) AndThen[U any](f func(T) (U, error)) Decoder[I, U] {
	return Decoder[I, U]{func(in I, at Path) outcome[U] {
		o := d.run(in, at)
		if o.failed() {
			return failAs[U](o)
		}
		v, err := f(o.value)
		return fromError(v, err, at)
	}}
}

// Pipe returns a decoder that hands the decoded value to next as its input,
// at the same path.
func (d Decoder[I, T]) Pipe[U any](next Decoder[T, U]) Decoder[I, U] {
	return Decoder[I, U]{func(in I, at Path) outcome[U] {
		o := d.run(in, at)
		if o.failed() {
			return failAs[U](o)
		}
		return next.run(o.value, at)
	}}
}

// Refine returns a decoder that also requires ok to hold for the decoded
// value, and reports code with message as a custom message when it does not.
func (d Decoder[I, T]) Refine(ok func(T) bool, code, message string) Decoder[I, T] {
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.failed() || ok(o.value) {
			return o
		}
		return invalid[T](NewIssue(code).WithMessage(message).At(at))
	}}
}

// Default returns a decoder that gives v when every issue d reports is
// required, as for a missing or null value, and reports any other problem.
func (d Decoder[I, T]) Default(v T) Decoder[I, T] {
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.err != nil || o.issues.Len() == 0 {
			return o
		}
		for _, i := range o.issues.items {
			if i.code != CodeRequired {
				return o
			}
		}
		return succeed(v)
	}}
}

// Fallback returns a decoder that gives v whatever issues d reports. A failure
// of the program is still returned.
func (d Decoder[I, T]) Fallback(v T) Decoder[I, T] {
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.err == nil && o.issues.Len() > 0 {
			return succeed(v)
		}
		return o
	}}
}

// Nullable returns a decoder that gives nil for null and decodes anything else
// with d. A missing member is not null: it is handed to d, which reports it as
// required. Use [Optional] for a member that may be left out.
func Nullable[T any](d DecoderOf[T]) Decoder[any, *T] {
	dd := d.decoder()
	return Decoder[any, *T]{func(in any, at Path) outcome[*T] {
		if in == nil {
			return succeed[*T](nil)
		}
		o := dd.run(in, at)
		if o.failed() {
			return failAs[*T](o)
		}
		return succeed(&o.value)
	}}
}

// Lazy returns a decoder that calls f each time it decodes and decodes with
// what it returns. It lets a decoder refer to itself:
//
//	func category() raoh.Decoder[any, Category] {
//		return raoh.Object(raoh.Fields().
//			Field("name", raoh.String()).
//			Field("children", raoh.List(raoh.Lazy(category))),
//		).Map(NewCategory)
//	}
func Lazy[T any](f func() Decoder[any, T]) Decoder[any, T] {
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		return f().run(in, at)
	}}
}

// OneOf returns a decoder that tries each alternative in turn and gives what
// the first to decode gives. When none does, it reports one_of_failed with the
// issues of each alternative in meta.candidates, as
// [{"candidate": index, "issues": [...]}] with English messages.
func OneOf[T any](alternatives ...DecoderOf[T]) Decoder[any, T] {
	ds := make([]Decoder[any, T], len(alternatives))
	for i, a := range alternatives {
		ds[i] = a.decoder()
	}
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		candidates := make([]any, 0, len(ds))
		for n, d := range ds {
			o := d.run(in, at)
			if !o.failed() || o.err != nil {
				return o
			}
			candidates = append(candidates, map[string]any{
				"candidate": n,
				"issues":    renderedJSON(o.issues),
			})
		}
		return invalid[T](NewIssue(CodeOneOfFailed).WithMeta("candidates", candidates).At(at))
	}}
}

// renderedJSON is the issues as the JSON values Raoh for Java puts in meta.
func renderedJSON(is Issues) []any {
	out := make([]any, is.Len())
	for n, r := range is.Render(English) {
		out[n] = map[string]any{"path": r.Path, "code": r.Code, "message": r.Message, "meta": r.Meta}
	}
	return out
}
