package raoh

import (
	"errors"
	"fmt"
	"reflect"
)

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

// fromRelativeError turns what a caller's function returned into an outcome. An
// error made of issues only is invalid input, moved to at. Any other error is a
// failure of the program; when issues are mixed into it, it is hidden behind
// an executionError so that no *Issues can be found in what Decode returns.
func fromRelativeError[T any](v T, err error, at Path) outcome[T] {
	if err == nil {
		return succeed(v)
	}
	o := failedError[T](err)
	if o.issues.Len() > 0 {
		o.issues = o.issues.Rebase(at)
	}
	return o
}

// fromAbsoluteError is fromRelativeError for a function that was given the
// path: the paths of its issues are kept as it returned them.
func fromAbsoluteError[T any](v T, err error) outcome[T] {
	if err == nil {
		return succeed(v)
	}
	return failedError[T](err)
}

func failedError[T any](err error) outcome[T] {
	if is, ok := asIssues(err); ok {
		return invalid[T](is.items...)
	}
	if _, mixed := errors.AsType[*Issues](err); mixed {
		err = &executionError{err}
	}
	return outcome[T]{err: err}
}

// executionError is a failure of the program whose error tree also holds
// issues. It hides the issues and nothing else: errors.Is, errors.As and
// errors.AsType see every error in the tree except an *Issues, so the caller
// can still find, say, the *os.PathError it holds, but never takes it for
// invalid input.
type executionError struct {
	cause error
}

func (e *executionError) Error() string { return e.cause.Error() }

// It has no Unwrap, which would show the issues; Is and As walk the tree in
// its place.

func (e *executionError) Is(target error) bool {
	if _, ok := target.(*Issues); ok {
		return false
	}
	return errors.Is(e.cause, target)
}

func (e *executionError) As(target any) bool { return asOutsideIssues(e.cause, target) }

// asOutsideIssues is errors.As over the tree of err with every *Issues in it
// and beneath it left out.
func asOutsideIssues(err error, target any) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*Issues); ok {
		return false
	}
	dst := reflect.ValueOf(target).Elem()
	if reflect.TypeOf(err).AssignableTo(dst.Type()) {
		dst.Set(reflect.ValueOf(err))
		return true
	}
	if x, ok := err.(interface{ As(any) bool }); ok && x.As(target) {
		if _, found := dst.Interface().(*Issues); !found {
			return true
		}
		dst.SetZero()
	}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		return asOutsideIssues(u.Unwrap(), target)
	case interface{ Unwrap() []error }:
		for _, branch := range u.Unwrap() {
			if asOutsideIssues(branch, target) {
				return true
			}
		}
	}
	return false
}

// Decoder decodes an I into a T. A decoder holds no state and can be reused
// and shared between goroutines.
//
// The zero Decoder has nothing to run. A decoder built from it, or a method
// called on it, panics: when a decoder is built if it can be, and otherwise at
// the decode.
type Decoder[I, T any] struct {
	run func(in I, at Path) outcome[T]
}

// DecoderOf is a decoder of the input values Raoh reads, such as a
// Decoder[any, T] or a [StringDecoder]. The functions that combine decoders
// take one.
type DecoderOf[T any] interface {
	decoder() Decoder[any, T]
}

// decoder lets a type that embeds a Decoder be passed as a DecoderOf. It
// refuses the zero Decoder, which has nothing to run, so a decoder built from
// one is refused when it is built.
func (d Decoder[I, T]) decoder() Decoder[I, T] {
	if d.run == nil {
		panic("raoh: a decoder was built from the zero Decoder, which has nothing to run")
	}
	return d
}

// requireRun panics if d is the zero Decoder, which has nothing to run: a
// decoder derived from it, or a decode with it, would panic whatever the input.
func (d Decoder[I, T]) requireRun(method string) {
	if d.run == nil {
		panic(fmt.Sprintf("raoh: %s needs a receiver that is not the zero Decoder", method))
	}
}

// decoderOf is d.decoder() for a decoder a constructor is given, refused when
// it is nil.
func decoderOf[T any](d DecoderOf[T], constructor, argument string) Decoder[any, T] {
	requireArgument(d != nil, constructor, argument)
	return d.decoder()
}

// requireArgument panics, when a decoder is built, if a function or decoder it
// is given is nil: the decode would call it and panic, whatever the input.
func requireArgument(ok bool, constructor, argument string) {
	if !ok {
		panic(fmt.Sprintf("raoh: %s needs %s that is not nil", constructor, argument))
	}
}

// NewDecoder returns a decoder that runs f. An error made of issues (see
// [Invalid]) reports the input as invalid, with the issues read as relative to
// the path the decoder is at; any other error stops the decode.
func NewDecoder[I, T any](f func(in I) (T, error)) Decoder[I, T] {
	requireArgument(f != nil, "NewDecoder", "f")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		v, err := f(in)
		return fromRelativeError(v, err, at)
	}}
}

// NewDecoderWithPath returns a decoder that runs f, which is given the path the
// decoder is at. The paths of the issues f returns are kept as f gives them,
// so build them from the path it was given, as at.Key("end").
func NewDecoderWithPath[I, T any](f func(in I, at Path) (T, error)) Decoder[I, T] {
	requireArgument(f != nil, "NewDecoderWithPath", "f")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		v, err := f(in, at)
		return fromAbsoluteError(v, err)
	}}
}

// Decode decodes in. When the error holds an *Issues, the whole error means
// the input was invalid, so errors.AsType[*raoh.Issues] tells invalid input
// from a failure of the program. A failure of the program is returned as the
// function that produced it returned it, unless issues were mixed into it.
func (d Decoder[I, T]) Decode(in I) (T, error) {
	d.requireRun("Decoder.Decode")
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
	d.requireRun("Decoder.Map")
	requireArgument(f != nil, "Decoder.Map", "f")
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
	d.requireRun("Decoder.AndThen")
	requireArgument(f != nil, "Decoder.AndThen", "f")
	return Decoder[I, U]{func(in I, at Path) outcome[U] {
		o := d.run(in, at)
		if o.failed() {
			return failAs[U](o)
		}
		v, err := f(o.value)
		return fromRelativeError(v, err, at)
	}}
}

// AndThenWithPath is AndThen for a function that is given the path this
// decoder is at, so it can report an issue at that path or beside it. Unlike
// AndThen, it does not move the issues f returns: their paths are kept as f
// gives them, so build them from the path it was given, as at.Key("end").
func (d Decoder[I, T]) AndThenWithPath[U any](f func(T, Path) (U, error)) Decoder[I, U] {
	d.requireRun("Decoder.AndThenWithPath")
	requireArgument(f != nil, "Decoder.AndThenWithPath", "f")
	return Decoder[I, U]{func(in I, at Path) outcome[U] {
		o := d.run(in, at)
		if o.failed() {
			return failAs[U](o)
		}
		v, err := f(o.value, at)
		return fromAbsoluteError(v, err)
	}}
}

// Pipe returns a decoder that hands the decoded value to next as its input,
// at the same path.
func (d Decoder[I, T]) Pipe[U any](next Decoder[T, U]) Decoder[I, U] {
	d.requireRun("Decoder.Pipe")
	if next.run == nil {
		panic("raoh: Decoder.Pipe needs a next decoder that is not the zero Decoder")
	}
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
	d.requireRun("Decoder.Refine")
	requireArgument(ok != nil, "Decoder.Refine", "ok")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.failed() || ok(o.value) {
			return o
		}
		return invalid[T](NewIssue(code).WithMessage(message).At(at))
	}}
}

// RefineWithMeta is Refine for a failure that carries metadata computed from
// the decoded value. meta is called only when ok does not hold, and the map it
// returns is copied into the issue.
func (d Decoder[I, T]) RefineWithMeta(ok func(T) bool, code, message string, meta func(T) map[string]any) Decoder[I, T] {
	d.requireRun("Decoder.RefineWithMeta")
	requireArgument(ok != nil, "Decoder.RefineWithMeta", "ok")
	requireArgument(meta != nil, "Decoder.RefineWithMeta", "meta")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.failed() || ok(o.value) {
			return o
		}
		return invalid[T](NewIssue(code).WithMessage(message).withMetaMap(meta(o.value)).At(at))
	}}
}

// RefineWithPath returns a decoder that also runs check on the decoded value
// with the path the decoder is at. When check returns an error made of issues
// (see [Invalid]), they are reported with their paths as check gives them, so
// build them from the path it was given, as at.Key("end"). Any other error
// stops the decode.
func (d Decoder[I, T]) RefineWithPath(check func(T, Path) error) Decoder[I, T] {
	d.requireRun("Decoder.RefineWithPath")
	requireArgument(check != nil, "Decoder.RefineWithPath", "check")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.failed() {
			return o
		}
		if err := check(o.value, at); err != nil {
			return fromAbsoluteError(o.value, err)
		}
		return o
	}}
}

// Default returns a decoder that gives v when every issue d reports is
// required, as for a missing or null value, and reports any other problem.
func (d Decoder[I, T]) Default(v T) Decoder[I, T] {
	d.requireRun("Decoder.Default")
	return d.DefaultFunc(func() T { return v })
}

// DefaultFunc is Default with the default computed by f, which is called once
// each time the default is needed and not otherwise.
func (d Decoder[I, T]) DefaultFunc(f func() T) Decoder[I, T] {
	d.requireRun("Decoder.DefaultFunc")
	requireArgument(f != nil, "Decoder.DefaultFunc", "f")
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
		return succeed(f())
	}}
}

// Fallback returns a decoder that gives v whatever issues d reports. A failure
// of the program is still returned.
func (d Decoder[I, T]) Fallback(v T) Decoder[I, T] {
	d.requireRun("Decoder.Fallback")
	return d.FallbackFunc(func(Issues) T { return v })
}

// FallbackFunc is Fallback with the value computed by f from the issues d
// reported, at their paths in the whole input. A failure of the program is
// still returned.
func (d Decoder[I, T]) FallbackFunc(f func(Issues) T) Decoder[I, T] {
	d.requireRun("Decoder.FallbackFunc")
	requireArgument(f != nil, "Decoder.FallbackFunc", "f")
	return Decoder[I, T]{func(in I, at Path) outcome[T] {
		o := d.run(in, at)
		if o.err == nil && o.issues.Len() > 0 {
			return succeed(f(o.issues))
		}
		return o
	}}
}

// Nullable returns a decoder that gives nil for null and decodes anything else
// with d. A missing member is not null: it is handed to d, which reports it as
// required. Use [Optional] for a member that may be left out.
func Nullable[T any](d DecoderOf[T]) Decoder[any, *T] {
	dd := decoderOf(d, "Nullable", "d")
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
	requireArgument(f != nil, "Lazy", "f")
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
		ds[i] = decoderOf(a, "OneOf", "each alternative")
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
