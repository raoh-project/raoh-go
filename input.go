package raoh

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"
	"unicode/utf8"
)

// The input a decoder is handed is one of these values, and a decoder written
// with NewDecoder sees the same:
//
//   - nil, for null
//   - a missing member, which [IsMissing] tells apart from null; only the
//     decoder of an object's field is handed one
//   - a boolean: a value of any type whose underlying type is bool
//   - a string: a value of any type whose underlying type is string, except
//     json.Number
//   - a number: json.Number, or a value of any integer or float type, named
//     types such as time.Duration included
//   - an array: a value of any type whose underlying type is []any
//   - an object: a value of any type whose underlying type is map[string]any,
//     or a *JSONObject, which DecodeJSON gives to keep the members in the order
//     written; [AsObject] reads either
//
// These include the values encoding/json gives when it decodes into an any,
// with or without UseNumber, and DecodeJSON gives no others: it keeps each
// number as written, as a json.Number, and each object as a *JSONObject.

// JSONObject is an object with its members in the order they were written.
// It is read only.
type JSONObject struct {
	names  []string
	values map[string]any
}

// AsObject returns v as a JSONObject if v is an object: a *JSONObject as it
// is, or a map[string]any with its members in the order of Go strings.
func AsObject(v any) (*JSONObject, bool) {
	switch o := plain(v).(type) {
	case *JSONObject:
		return o, o != nil
	case map[string]any:
		if o == nil {
			return nil, false
		}
		names := make([]string, 0, len(o))
		for k := range o {
			names = append(names, k)
		}
		slices.Sort(names)
		return &JSONObject{names, o}, true
	}
	return nil, false
}

// Len returns the number of members.
func (o *JSONObject) Len() int { return len(o.names) }

// Keys returns the names of the members in order.
func (o *JSONObject) Keys() []string { return slices.Clone(o.names) }

// Get returns the member name.
func (o *JSONObject) Get(name string) (any, bool) {
	v, ok := o.values[name]
	return v, ok
}

// All returns the members in order.
func (o *JSONObject) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for _, k := range o.names {
			if !yield(k, o.values[k]) {
				return
			}
		}
	}
}

// member returns the member name, or the missing member when there is none.
func (o *JSONObject) member(name string) any {
	if v, ok := o.values[name]; ok {
		return v
	}
	return missing
}

// missingValue is what a field's decoder is handed when its member is not
// there.
type missingValue struct{}

var missing any = missingValue{}

// IsMissing reports whether v is a member that is not there, as the decoder of
// an object's field is handed one, rather than null or a value.
func IsMissing(v any) bool { return v == missing }

// isNull reports whether v is null or a missing member.
func isNull(v any) bool {
	return v == nil || v == missing
}

var (
	anySlice = reflect.TypeFor[[]any]()
	anyMap   = reflect.TypeFor[map[string]any]()
)

// plain returns v as a value of the predeclared type the input model reads it
// as: a named boolean, string or number type as bool, string, int64, uint64,
// float32 or float64, and a named []any or map[string]any as the unnamed
// type. Any other value is returned as it is.
func plain(v any) any {
	switch v.(type) {
	case nil, missingValue, bool, string, json.Number,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64,
		[]any, map[string]any, *JSONObject:
		return v
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint()
	case reflect.Float32:
		return float32(rv.Float())
	case reflect.Float64:
		return rv.Float()
	case reflect.Slice:
		if rv.Type().ConvertibleTo(anySlice) {
			return rv.Convert(anySlice).Interface()
		}
	case reflect.Map:
		if rv.Type().ConvertibleTo(anyMap) {
			return rv.Convert(anyMap).Interface()
		}
	}
	return v
}

// kind names the type of an input value as an issue reports it in actual.
func kind(v any) string {
	switch plain(v).(type) {
	case missingValue:
		return "missing"
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
		return "number"
	case []any:
		return "array"
	case *JSONObject, map[string]any:
		return "object"
	}
	return "unknown"
}

// unexpected is required for a null or missing value and type_mismatch with
// expected and actual for anything else.
func unexpected(expected string, v any) Issue {
	if isNull(v) {
		return NewIssue(CodeRequired)
	}
	return NewIssue(CodeTypeMismatch).WithMeta("expected", expected).WithMeta("actual", kind(v))
}

// numberText returns the text of a number kept as written.
func numberText(v any) (string, bool) {
	n, ok := v.(json.Number)
	return string(n), ok
}

// DecodeJSON parses data as JSON and decodes it with d. Text that is not JSON
// is one invalid_format issue at the root, under the message key
// invalid_format.json, with the line and column where it stopped being JSON.
// Numbers are kept as written and object members in the order written.
func DecodeJSON[T any](data []byte, d DecoderOf[T]) (T, error) {
	v, issue := parseJSON(data)
	if issue != nil {
		var zero T
		return zero, &Issues{items: []Issue{*issue}}
	}
	return d.decoder().Decode(v)
}

// ErrInputTooLarge is the error DecodeJSONFrom returns, wrapped, when the
// input is longer than its limit.
var ErrInputTooLarge = errors.New("raoh: input too large")

// DecodeJSONFrom reads r and decodes it as [DecodeJSON] does, reading at most
// limit bytes. Input longer than that is not decoded: the error wraps
// [ErrInputTooLarge], and is not an *Issues, so that an HTTP handler can answer
// 413 rather than 400. An error reading r is returned as it is.
//
// The limit is required because the value a decoder reads is built in memory,
// and takes memory in proportion to the input whether the text is read at once
// or as a stream; a bound on the input is what bounds it.
func DecodeJSONFrom[T any](r io.Reader, limit int64, d DecoderOf[T]) (T, error) {
	var zero T
	if limit < 0 {
		panic("raoh: negative limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return zero, err
	}
	// The input is over the limit when a byte follows the limit's worth. It
	// is read rather than counted as limit+1, which overflows for the
	// largest limit.
	if int64(len(data)) == limit {
		switch _, err := io.ReadFull(r, make([]byte, 1)); {
		case err == nil:
			return zero, fmt.Errorf("%w: more than %d bytes", ErrInputTooLarge, limit)
		case err != io.EOF:
			return zero, err
		}
	}
	return DecodeJSON(data, d)
}

func parseJSON(data []byte) (any, *Issue) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	v, err := readJSON(dec)
	if err == nil {
		if _, next := dec.ReadToken(); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("raoh: text after the JSON value")
			}
		}
	}
	if err == nil {
		return v, nil
	}
	offset := dec.InputOffset()
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		offset = se.ByteOffset
	}
	line, column := position(data, int(min(offset, int64(len(data)))))
	i := NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatJSON).
		WithMeta("line", line).WithMeta("column", column)
	return nil, &i
}

func readJSON(dec *jsontext.Decoder) (any, error) {
	switch dec.PeekKind() {
	case '{':
		if _, err := dec.ReadToken(); err != nil {
			return nil, err
		}
		obj := &JSONObject{values: map[string]any{}}
		for dec.PeekKind() != '}' {
			tok, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}
			// A token is voided by the next read, so its text is taken first.
			name := tok.String()
			v, err := readJSON(dec)
			if err != nil {
				return nil, err
			}
			obj.names = append(obj.names, name)
			obj.values[name] = v
		}
		_, err := dec.ReadToken()
		return obj, err
	case '[':
		if _, err := dec.ReadToken(); err != nil {
			return nil, err
		}
		items := []any{}
		for dec.PeekKind() != ']' {
			v, err := readJSON(dec)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		_, err := dec.ReadToken()
		return items, err
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case 'n':
		return nil, nil
	case 't':
		return true, nil
	case 'f':
		return false, nil
	case '"':
		return tok.String(), nil
	case '0':
		return json.Number(tok.String()), nil
	}
	return nil, errors.New("raoh: unexpected JSON token " + tok.String())
}

// position returns the line and column, both counted from 1, of the byte at
// offset. The column counts characters.
func position(data []byte, offset int) (line, column int) {
	before := data[:offset]
	line = bytes.Count(before, []byte("\n")) + 1
	lineStart := bytes.LastIndexByte(before, '\n') + 1
	return line, utf8.RuneCount(before[lineStart:]) + 1
}
