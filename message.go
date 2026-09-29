package raoh

import (
	_ "embed"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Resolver writes the sentence for an issue that has no custom message.
// [Issue.Message] calls it only when the issue carries none, so a resolver
// does not look for one.
type Resolver interface {
	Resolve(Issue) string
}

// ResolverFunc is a function used as a [Resolver].
type ResolverFunc func(Issue) string

// Resolve calls f.
func (f ResolverFunc) Resolve(i Issue) string { return f(i) }

// Messages is a catalogue of message templates keyed by message key or code,
// over the catalogue it falls back to.
//
// A catalogue is a stack of layers, most specific first, as Raoh for Java's
// ResourceBundleMessageResolver reads a locale's .properties file before its
// parent's. An issue is looked up in one layer at a time: by its message key,
// then by its code, and only when neither gives a sentence in the next layer
// down. So a layer that translates just invalid_format wins over a refined key
// such as invalid_format.email in the layer beneath it. A template's {name}
// placeholders are filled from the issue's metadata; a template naming an entry
// the metadata lacks is passed over. When no template fits, the sentence is
// "validation failed: <code>".
//
// A Messages is not changed once made; the methods that add to it return a new
// one.
type Messages struct {
	templates map[string]string
	parent    *Messages
}

var (
	//go:embed messages/en.properties
	englishProperties string
	//go:embed messages/ja.properties
	japaneseProperties string
)

// English is the English catalogue Raoh for Java ships, word for word, plus a
// template for invalid_format.json. [Issues.Error] writes its messages with it.
var English = mustProperties(englishProperties)

// Japanese is the Japanese catalogue Raoh for Java ships, over [English].
var Japanese = mustProperties(japaneseProperties).FallingBackTo(English)

func mustProperties(text string) *Messages {
	m, err := ParseProperties(text)
	if err != nil {
		panic(err)
	}
	return m
}

// ParseProperties reads a catalogue written as Java's Properties.load reads a
// .properties file, such as the messages*.properties of Raoh for Java: one
// raoh.<key>=<template> per entry, \uXXXX escapes and continued lines
// included. The raoh. prefix is optional. The catalogue falls back to nothing;
// give it one with [Messages.FallingBackTo].
func ParseProperties(text string) (*Messages, error) {
	pairs, err := loadProperties(text)
	if err != nil {
		return nil, err
	}
	templates := make(map[string]string, len(pairs))
	for _, p := range pairs {
		templates[strings.TrimPrefix(p.key, "raoh.")] = p.value
	}
	return &Messages{templates: templates}, nil
}

// FallingBackTo returns m with parent beneath its last layer, as a locale's
// file sits over its parent's.
func (m *Messages) FallingBackTo(parent *Messages) *Messages {
	if m.parent == nil {
		return &Messages{templates: m.templates, parent: parent}
	}
	return &Messages{templates: m.templates, parent: m.parent.FallingBackTo(parent)}
}

// WithOverrides returns a layer of overrides, keyed by message key or code,
// over m.
func (m *Messages) WithOverrides(overrides map[string]string) *Messages {
	return &Messages{templates: maps.Clone(overrides), parent: m}
}

// Template returns the template the most specific layer holding key has.
func (m *Messages) Template(key string) (string, bool) {
	for l := m; l != nil; l = l.parent {
		if t, ok := l.templates[key]; ok {
			return t, true
		}
	}
	return "", false
}

// Templates returns every key and the template the most specific layer
// holding it has.
func (m *Messages) Templates() map[string]string {
	out := map[string]string{}
	for l := m; l != nil; l = l.parent {
		for k, t := range l.templates {
			if _, ok := out[k]; !ok {
				out[k] = t
			}
		}
	}
	return out
}

// Resolve writes the sentence for i.
func (m *Messages) Resolve(i Issue) string {
	for l := m; l != nil; l = l.parent {
		for _, key := range []string{i.messageKey, i.code} {
			if t, ok := l.templates[key]; ok {
				if s, ok := fill(t, i.meta); ok {
					return s
				}
			}
		}
	}
	return "validation failed: " + i.code
}

// fill returns template with each {name} replaced by the metadata entry name,
// or false when an entry is missing. A brace that does not open a placeholder
// name is kept as it is.
func fill(template string, meta map[string]any) (string, bool) {
	var out strings.Builder
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			break
		}
		out.WriteString(rest[:open])
		after := rest[open+1:]
		end := strings.IndexByte(after, '}')
		if end >= 0 && isPlaceholderName(after[:end]) {
			v, ok := meta[after[:end]]
			if !ok {
				return "", false
			}
			out.WriteString(display(v))
			rest = after[end+1:]
		} else {
			out.WriteByte('{')
			rest = after
		}
	}
	out.WriteString(rest)
	return out.String(), true
}

func isPlaceholderName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range []byte(name) {
		alpha := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		if i == 0 && !alpha {
			return false
		}
		if !alpha && !(c >= '0' && c <= '9') && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

// display writes a metadata value as Java's String.valueOf writes the value
// Raoh for Java holds: strings without quotes, lists as [a, b], maps as
// {k=v}, a float as Double.toString does, and a decimal as written.
func display(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case float64:
		return doubleToString(x)
	case float32:
		return doubleToString(float64(x))
	case json.Number:
		return string(x)
	case Decimal:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return doubleToString(rv.Float())
	case reflect.Slice, reflect.Array:
		items := make([]string, rv.Len())
		for i := range items {
			items[i] = display(rv.Index(i).Interface())
		}
		return "[" + strings.Join(items, ", ") + "]"
	case reflect.Map:
		keys := rv.MapKeys()
		names := make([]string, len(keys))
		byName := map[string]reflect.Value{}
		for i, k := range keys {
			names[i] = display(k.Interface())
			byName[names[i]] = k
		}
		slices.Sort(names)
		entries := make([]string, len(names))
		for i, n := range names {
			entries[i] = n + "=" + display(rv.MapIndex(byName[n]).Interface())
		}
		return "{" + strings.Join(entries, ", ") + "}"
	}
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return reflect.ValueOf(v).String()
}
