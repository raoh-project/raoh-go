package raoh

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

// Issue is one problem found in the input: where it is, what kind it is, and
// what else its code says.
//
// An issue carries no sentence of its own. Its code, message key and meta are
// what a program reads; the sentence a person reads is written from them by a
// [Resolver] such as [English] or [Japanese]. A message given with
// [Issue.WithMessage] replaces that sentence in every language.
type Issue struct {
	path          Path
	code          string
	messageKey    string
	meta          map[string]any
	customMessage string
	hasCustom     bool
	// byStrict is whether a Strict, or the Strict of an Object, made this
	// unknown_field in the decode that is running, so that a Strict around it
	// does not report the member again. It lives as long as that decode: an
	// issue crosses into the caller's hands, and back, only through
	// Issues.published, which drops it, so an issue a caller holds or returns
	// never has it.
	byStrict bool
	// candidates is what each alternative of a OneOf reported, for
	// one_of_failed: kept as issues, so that they are written in the language
	// the issue is and move with it, and put in meta.candidates only when the
	// issue is written. A pointer, nil for every other issue, so that an issue
	// is no larger for what only one_of_failed holds.
	candidates *[]candidate
}

// candidate is the issues one alternative of a OneOf reported, and its place
// among them.
type candidate struct {
	index  int
	issues Issues
}

// NewIssue returns an issue with code at the root, with the code as its
// message key. Returned from a function given to AndThen, it is moved to the
// path the decoder is at.
func NewIssue(code string) Issue {
	return Issue{code: code, messageKey: code}
}

// WithMessageKey returns i with key naming the check that produced it.
func (i Issue) WithMessageKey(key string) Issue {
	i.messageKey = key
	return i
}

// WithMeta returns i with one more entry of metadata.
//
// Metadata is a tree of values like JSON's: scalars, and slices, arrays, maps
// and interfaces holding more of them. The issue owns that tree: WithMeta copies
// every slice, array and map in value, at any depth, so changing them
// afterwards does not change the issue, and Meta hands out copies of its own.
// A struct or a pointer in the tree, such as a time.Time, is a value of the
// caller's, held as it was given: the issue copies a struct as Go assigns it
// and does not copy what a pointer or a struct's fields refer to.
func (i Issue) WithMeta(key string, value any) Issue {
	m := maps.Clone(i.meta)
	if m == nil {
		m = map[string]any{}
	}
	m[key] = ownCopy(value)
	i.meta = m
	return i
}

// withMetaMap returns i with a copy of meta as its metadata, in place of what
// it had. It copies once, where a WithMeta per entry would copy the whole map
// each time.
func (i Issue) withMetaMap(meta map[string]any) Issue {
	i.meta = ownCopy(meta).(map[string]any)
	return i
}

// WithMessage returns i with message as its sentence in every language, in
// place of the catalogue's.
func (i Issue) WithMessage(message string) Issue {
	i.customMessage = message
	i.hasCustom = true
	return i
}

// At returns i at path p.
func (i Issue) At(p Path) Issue {
	i.path = p
	return i
}

// Rebase returns i with its path read as relative to prefix. The issues of a
// one_of_failed's candidates are of the same input, and move with it.
func (i Issue) Rebase(prefix Path) Issue {
	i.path = prefix.Append(i.path)
	if i.candidates != nil {
		moved := make([]candidate, len(*i.candidates))
		for n, c := range *i.candidates {
			moved[n] = candidate{c.index, c.issues.Rebase(prefix)}
		}
		i.candidates = &moved
	}
	return i
}

// Path returns where in the input the problem is.
func (i Issue) Path() Path { return i.path }

// Code returns what kind of problem it is: one of the Code constants or a
// caller's own.
func (i Issue) Code() string { return i.code }

// MessageKey returns the key a catalogue looks up first: the code, or a
// refinement of it such as out_of_range.minimum.
func (i Issue) MessageKey() string { return i.messageKey }

// Meta returns a copy of what else the code says about the problem, such as
// the bound a value fell outside of. Its slices, arrays and maps are copies
// too, at any depth, so changing them changes neither the issue nor the
// decoder that made it; a struct or a pointer is as [Issue.WithMeta] says.
// The candidates of a one_of_failed are written with English messages; Render
// writes them in the language it is given.
func (i Issue) Meta() map[string]any { return i.metaIn(English) }

// metaIn is Meta with the candidates of a one_of_failed written by r.
func (i Issue) metaIn(r Resolver) map[string]any {
	m := map[string]any{}
	if i.meta != nil {
		m = ownCopy(i.meta).(map[string]any)
	}
	if i.candidates != nil {
		written := make([]any, len(*i.candidates))
		for n, c := range *i.candidates {
			issues := make([]any, c.issues.Len())
			for k, ri := range c.issues.Render(r) {
				issues[k] = map[string]any{"path": ri.Path, "code": ri.Code, "message": ri.Message, "meta": ri.Meta}
			}
			written[n] = map[string]any{"candidate": c.index, "issues": issues}
		}
		m["candidates"] = written
	}
	return m
}

// ownCopy is v with every slice, array and map of its tree copied, at every
// depth, so that what an issue holds is its own and what it hands out is the
// caller's. A struct or a pointer is given as it is (see Issue.WithMeta). The
// copy is equal to v, a nil slice or map staying nil, whichever way it is
// made: a test holds the paths without reflect to what deepCopy gives.
func ownCopy(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64, Decimal, temporalValue:
		// Nothing in these can be changed through the issue.
		return v
	case map[string]any:
		// The two kinds of container metadata most often holds, copied without
		// reflect. Each answers as deepCopy does, nil kept nil.
		if x == nil {
			return x
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = ownCopy(e)
		}
		return out
	case []any:
		if x == nil {
			return x
		}
		out := make([]any, len(x))
		for n, e := range x {
			out[n] = ownCopy(e)
		}
		return out
	}
	return deepCopy(reflect.ValueOf(v)).Interface()
}

func deepCopy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(deepCopy(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(deepCopy(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		for it := v.MapRange(); it.Next(); {
			out.SetMapIndex(it.Key(), deepCopy(it.Value()))
		}
		return out
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(deepCopy(v.Elem()))
		return out
	default:
		return v
	}
}

// CustomMessage returns the message given with [Issue.WithMessage], if there
// is one.
func (i Issue) CustomMessage() (string, bool) { return i.customMessage, i.hasCustom }

// Message returns the sentence a person reads: the custom message if there is
// one, or what r writes.
func (i Issue) Message(r Resolver) string {
	if i.hasCustom {
		return i.customMessage
	}
	return r.Resolve(i)
}

// String returns the path and the English message, for a log. The path and
// the message may hold text from the input, such as the name of an unknown
// member, so a character that is not graphic, such as a newline, is written as
// a Go escape, and a backslash as two: no input can make the text read as
// more than one issue.
func (i Issue) String() string {
	p := i.path.String()
	if p == "" {
		p = "(root)"
	}
	return escapeForLog(p) + ": " + escapeForLog(i.Message(English))
}

func escapeForLog(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r == '\\' || !unicode.IsGraphic(r) }) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case unicode.IsGraphic(r):
			b.WriteRune(r)
		default:
			b.WriteString(strings.Trim(strconv.QuoteRune(r), "'"))
		}
	}
	return b.String()
}

// Issues is every problem a decode found, in the order it found them. A
// non-empty *Issues is the error Decode returns for invalid input. The zero
// value is empty.
type Issues struct {
	items []Issue
	// claimed is how much of the array under items has been taken, shared by
	// every Issues whose items lie in that array, or nil where none of it is
	// known to be free. See extended.
	claimed *atomic.Int64
}

// Invalid returns issues as an error. Returned from a function given to
// AndThen, it reports the input as invalid, so its issues are gathered with
// those of the other fields; they are read as relative to the path the decoder
// is at. Any other error a function returns stops the decode.
func Invalid(issues ...Issue) error {
	is := Issues{items: slices.Clone(issues)}.published()
	return &is
}

// published is is as a caller sees it, without the marks a decode keeps for
// itself while it runs. Every issue that leaves a decode for the caller, from
// Decode or to the function given to FallbackFunc, and every issue that comes
// back from a caller's function or Invalid, goes through it. It copies only a
// list that holds a mark.
func (is Issues) published() Issues {
	n := slices.IndexFunc(is.items, func(i Issue) bool { return i.byStrict })
	if n < 0 {
		return is
	}
	out := slices.Clone(is.items)
	for k := n; k < len(out); k++ {
		out[k].byStrict = false
	}
	return Issues{items: out}
}

// unknownMember is the unknown_field a Strict reports for the member k at p.
func unknownMember(k string, p Path) Issue {
	i := NewIssue(CodeUnknownField).WithMeta("field", k).At(p)
	i.byStrict = true
	return i
}

// Len returns the number of issues.
func (is Issues) Len() int { return len(is.items) }

// All returns a copy of the issues in the order they were found.
func (is Issues) All() []Issue { return slices.Clone(is.items) }

// Error writes each issue as its path and English message, one per line.
func (is *Issues) Error() string {
	lines := make([]string, len(is.items))
	for n, i := range is.items {
		lines[n] = i.String()
	}
	return strings.Join(lines, "\n")
}

// RenderedIssue is an issue with its message written, in the form Raoh for
// Java and PHP give an issue as JSON.
//
// Its fields can hold text from the input. Path names the members the input
// has, such as an unknown member; Meta holds values the input gave, such as
// actual, the duplicates of a list or the name of an unknown member, and what a
// caller put there with WithMeta; and Message is filled from Meta. Write them
// to a response only where showing the input back to its sender is intended,
// and filter or omit Meta otherwise.
type RenderedIssue struct {
	Path    string         `json:"path"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Meta    map[string]any `json:"meta"`
}

// Render returns the issues with their messages written by r, ready to be
// written out as JSON: [{"path", "code", "message", "meta"}, ...]. See
// [RenderedIssue] for what in them can come from the input.
func (is Issues) Render(r Resolver) []RenderedIssue {
	out := make([]RenderedIssue, len(is.items))
	for n, i := range is.items {
		out[n] = RenderedIssue{i.path.String(), i.code, i.Message(r), i.metaIn(r)}
	}
	return out
}

// Flatten returns the messages r writes, grouped by the JSON Pointer of their
// path.
func (is Issues) Flatten(r Resolver) map[string][]string {
	out := map[string][]string{}
	for _, i := range is.items {
		p := i.path.String()
		out[p] = append(out[p], i.Message(r))
	}
	return out
}

// Format returns the messages r writes as a tree following the paths, in the
// form of Raoh for Java's Issues.format: each member, and each index written
// in decimal, is a map, and the messages at a path are the list under its
// "_errors" key. Messages at the root are under "_errors" of the result.
//
// The tree cannot tell an input member named "_errors" from the list of
// messages, so it is a projection for showing, not a form to read the issues
// back from; use [Issues.All], [Issues.Render] or [Issues.GroupByPath] for
// that. The messages can hold text from the input; see [RenderedIssue].
func (is Issues) Format(r Resolver) map[string]any {
	root := map[string]any{}
	for _, i := range is.items {
		node := root
		for _, seg := range i.path.Segments() {
			next, ok := node[seg].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[seg] = next
			}
			node = next
		}
		msgs, _ := node["_errors"].([]string)
		node["_errors"] = append(msgs, i.Message(r))
	}
	return root
}

// GroupByPath returns the issues grouped by the JSON Pointer of their path,
// each group in the order its issues were found.
func (is Issues) GroupByPath() map[string]Issues {
	out := map[string]Issues{}
	for _, i := range is.items {
		p := i.path.String()
		g := out[p]
		g.appendInPlace(i)
		out[p] = g
	}
	return out
}

// Add returns the issues followed by more, leaving is as it was. Adding to
// the most recent result of an Add or Merge takes time in the issues added, so
// a loop that adds one issue at a time takes time linear in the issues.
func (is Issues) Add(more ...Issue) Issues { return is.extended(more) }

// Merge returns the issues followed by those of other, leaving both as they
// were. It takes time in the issues of other where is is the most recent
// result of an Add or Merge, as Add does.
func (is Issues) Merge(other Issues) Issues { return is.extended(other.items) }

// extended is the issues followed by more, and the one way a list of issues
// grows.
//
// A list is never changed: what extended gives is another list. To keep a loop
// of additions linear it writes more into the room after is in the same array
// where that room is free, and the room is free only to the first list to claim
// it: claimed counts what of the array has been taken, and a list that is not
// the longest taken from it copies instead. So two lists made from one hold
// what each was given, and is is never changed under whoever holds it.
func (is Issues) extended(more []Issue) Issues {
	if len(more) == 0 {
		return is
	}
	n := len(is.items)
	if is.claimed != nil && cap(is.items)-n >= len(more) &&
		is.claimed.CompareAndSwap(int64(n), int64(n+len(more))) {
		items := is.items[:n+len(more)]
		copy(items[n:], more)
		return Issues{items, is.claimed}
	}
	// A copy, grown as append grows a slice, into an array no other list has.
	items := append(slices.Clip(is.items), more...)
	claimed := new(atomic.Int64)
	claimed.Store(int64(len(items)))
	return Issues{items, claimed}
}

// Rebase returns the issues with each path read as relative to prefix.
func (is Issues) Rebase(prefix Path) Issues {
	out := make([]Issue, len(is.items))
	for n, i := range is.items {
		out[n] = i.Rebase(prefix)
	}
	return Issues{items: out}
}

func (is *Issues) appendInPlace(i ...Issue) { *is = is.extended(i) }

// asIssues reports whether err is made of Raoh issues only. A single wrapper
// such as fmt.Errorf("...: %w", issues) is looked through, and a join counts
// only when every branch is made of issues, so an ordinary error in the same
// tree is never mistaken for invalid input.
func asIssues(err error) (Issues, bool) {
	switch e := err.(type) {
	case nil:
		return Issues{}, false
	case *Issues:
		if e == nil || e.Len() == 0 {
			return Issues{}, false
		}
		return *e, true
	case interface{ Unwrap() []error }:
		var all Issues
		for _, b := range e.Unwrap() {
			is, ok := asIssues(b)
			if !ok {
				return Issues{}, false
			}
			all.appendInPlace(is.items...)
		}
		return all, all.Len() > 0
	}
	if inner := errors.Unwrap(err); inner != nil {
		return asIssues(inner)
	}
	return Issues{}, false
}
