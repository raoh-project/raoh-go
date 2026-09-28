package raoh

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"slices"
	"unicode/utf8"
)

// The input a decoder reads is one of the values encoding/json gives when it
// decodes into an any: nil, bool, string, float64 or json.Number, []any and
// map[string]any. Other Go integer and float types are read as numbers too.
//
// DecodeJSON reads JSON text into the same shapes, except that it keeps each
// number as written and each object's members in the order written, in the
// private types below, so that a large integer or a decimal is read exactly and
// unknown members are reported in input order.

// jsonNumber is a number read from JSON text, as written.
type jsonNumber string

// jsonObject is an object read from JSON text, with its members in the order
// written.
type jsonObject struct {
	names  []string
	values map[string]any
}

// missingValue is what a field reads when its member is not there. It is told
// apart from nil, which is a member present as null.
type missingValue struct{}

var missing any = missingValue{}

// isNull reports whether v is null or a missing member.
func isNull(v any) bool {
	return v == nil || v == missing
}

// kind names the type of an input value as an issue reports it in actual.
func kind(v any) string {
	switch v.(type) {
	case missingValue:
		return "missing"
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case jsonNumber, json.Number,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return "number"
	case []any:
		return "array"
	case *jsonObject, map[string]any:
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
	switch n := v.(type) {
	case jsonNumber:
		return string(n), true
	case json.Number:
		return string(n), true
	}
	return "", false
}

// members is an object's members, in the order a decoder visits them: input
// order for JSON text, and the order of Go strings for a map[string]any.
type members struct {
	names  []string
	values map[string]any
}

func asMembers(v any) (members, bool) {
	switch o := v.(type) {
	case *jsonObject:
		return members{o.names, o.values}, true
	case map[string]any:
		names := make([]string, 0, len(o))
		for k := range o {
			names = append(names, k)
		}
		slices.Sort(names)
		return members{names, o}, true
	}
	return members{}, false
}

// get returns the member name, or missing when there is none.
func (m members) get(name string) any {
	if v, ok := m.values[name]; ok {
		return v
	}
	return missing
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

// DecodeJSONFrom reads all of r and decodes it as [DecodeJSON] does. An error
// reading r is returned as it is.
func DecodeJSONFrom[T any](r io.Reader, d DecoderOf[T]) (T, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		var zero T
		return zero, err
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
		obj := &jsonObject{values: map[string]any{}}
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
		return jsonNumber(tok.String()), nil
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
