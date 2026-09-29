package raoh

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
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
func (i Issue) WithMeta(key string, value any) Issue {
	m := maps.Clone(i.meta)
	if m == nil {
		m = map[string]any{}
	}
	m[key] = value
	i.meta = m
	return i
}

// withMetaMap returns i with a copy of meta as its metadata, in place of what
// it had. It copies once, where a WithMeta per entry would copy the whole map
// each time.
func (i Issue) withMetaMap(meta map[string]any) Issue {
	i.meta = maps.Clone(meta)
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

// Rebase returns i with its path read as relative to prefix.
func (i Issue) Rebase(prefix Path) Issue {
	i.path = prefix.Append(i.path)
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
// the bound a value fell outside of.
func (i Issue) Meta() map[string]any {
	if i.meta == nil {
		return map[string]any{}
	}
	return maps.Clone(i.meta)
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
}

// Invalid returns issues as an error. Returned from a function given to
// AndThen, it reports the input as invalid, so its issues are gathered with
// those of the other fields; they are read as relative to the path the decoder
// is at. Any other error a function returns stops the decode.
func Invalid(issues ...Issue) error {
	return &Issues{items: slices.Clone(issues)}
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
		out[n] = RenderedIssue{i.path.String(), i.code, i.Message(r), i.Meta()}
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

// Add returns the issues followed by more, leaving is as it was.
func (is Issues) Add(more ...Issue) Issues {
	return Issues{items: slices.Concat(is.items, more)}
}

// Merge returns the issues followed by those of other, leaving both as they
// were.
func (is Issues) Merge(other Issues) Issues {
	return Issues{items: slices.Concat(is.items, other.items)}
}

// Rebase returns the issues with each path read as relative to prefix.
func (is Issues) Rebase(prefix Path) Issues {
	out := make([]Issue, len(is.items))
	for n, i := range is.items {
		out[n] = i.Rebase(prefix)
	}
	return Issues{items: out}
}

func (is *Issues) appendInPlace(i ...Issue) { is.items = append(is.items, i...) }

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
