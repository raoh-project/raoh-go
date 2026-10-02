package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/raoh-project/raoh-go"
)

// operation binds one operation applied to n.
func (b *binder) operation(n node, form any) (node, error) {
	f, ok := form.([]any)
	if !ok || len(f) == 0 {
		return node{}, fmt.Errorf("not an operation: %v", form)
	}
	name := f[0].(string)
	args := f[1:]
	switch name {
	case "map", "refine", "flatMap":
		b.use("operation.any." + name)
		return b.generic(n, name, args[0].(string))
	}
	feature := "operation." + n.t.kind + "." + name
	b.use(feature)
	// The message is the argument after the others, where it is given.
	var message *string
	if len(args) > 0 {
		if m, ok := args[len(args)-1].(string); ok && takesMessage(name, len(args)) {
			message = &m
			args = args[:len(args)-1]
			b.use(feature + ".message")
		}
	}
	switch d := n.typed.(type) {
	case raoh.StringDecoder:
		return stringOperation(d, name, args, message)
	case raoh.IntDecoder[int32]:
		return numberOperation(n.t, d, name, args, message, int32Of)
	case raoh.IntDecoder[int64]:
		return numberOperation(n.t, d, name, args, message, int64Of)
	case raoh.Float32Decoder:
		return numberOperation(n.t, d, name, args, message, float32Of)
	case raoh.Float64Decoder:
		return numberOperation(n.t, d, name, args, message, float64Of)
	case raoh.DecimalDecoder:
		if name == "scale" {
			r := d.Scale(int(int32Of(args[0])))
			return withMessage(n.t, r, message)
		}
		return numberOperation(n.t, d, name, args, message, decimalOf)
	case raoh.BoolDecoder:
		if name == "isTrue" {
			return withMessage(n.t, d.IsTrue(), message)
		}
	case raoh.TemporalDecoder:
		bound := func(v any) (any, error) { return materialise(n.t, v) }
		var r raoh.TemporalDecoder
		switch name {
		case "before", "after":
			v, err := bound(args[0])
			if err != nil {
				return node{}, err
			}
			if name == "before" {
				r = d.Before(v.(timeT))
			} else {
				r = d.After(v.(timeT))
			}
		case "between":
			from, err := bound(args[0])
			if err != nil {
				return node{}, err
			}
			to, err := bound(args[1])
			if err != nil {
				return node{}, err
			}
			r = d.Between(from.(timeT), to.(timeT))
		default:
			return node{}, fmt.Errorf("no temporal operation %q", name)
		}
		return withMessage(n.t, r, message)
	case raoh.ListDecoder[any]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[string]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[int32]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[int64]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[float32]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[float64]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[bool]:
		return listOperation(n.t, d, name, args, message)
	case raoh.ListDecoder[raoh.Decimal]:
		return listOperation(n.t, d, name, args, message)
	case raoh.DictDecoder[any]:
		var r raoh.DictDecoder[any]
		switch name {
		case "nonempty":
			r = d.NonEmpty()
		case "minSize":
			r = d.MinSize(int(int32Of(args[0])))
		case "maxSize":
			r = d.MaxSize(int(int32Of(args[0])))
		case "fixedSize":
			r = d.Size(int(int32Of(args[0])))
		default:
			return node{}, fmt.Errorf("no map operation %q", name)
		}
		return withMessage(n.t, r, message)
	}
	return node{}, fmt.Errorf("no operation %q on %v as raoh-go has bound it", name, n.t)
}

// takesMessage is whether the last of args is the operation's message: the
// operation takes one, as catalog/operations.json says, and args has one
// more than its other arguments.
func takesMessage(name string, args int) bool {
	shape, ok := operationShapes[name]
	return ok && shape.message && args == shape.values+1
}

// messaged is a typed decoder of raoh-go that takes a message for its most
// recent constraint.
type messaged[D any] interface {
	Message(string) D
}

// withMessage is the node of d, given message where there is one.
func withMessage[D messaged[D]](t *typ, d D, message *string) (node, error) {
	if message != nil {
		d = d.Message(*message)
	}
	return node{t, d, boxedTyped(d)}, nil
}

// boxedTyped is a typed decoder of raoh-go as a decoder of values of any.
func boxedTyped(d any) raoh.Decoder[any, any] {
	switch x := d.(type) {
	case raoh.StringDecoder:
		return boxed(x.Decoder)
	case raoh.IntDecoder[int32]:
		return boxed(x.Decoder)
	case raoh.IntDecoder[int64]:
		return boxed(x.Decoder)
	case raoh.Float32Decoder:
		return boxed(x.Decoder)
	case raoh.Float64Decoder:
		return boxed(x.Decoder)
	case raoh.DecimalDecoder:
		return boxed(x.Decoder)
	case raoh.BoolDecoder:
		return boxed(x.Decoder)
	case raoh.TemporalDecoder:
		return boxed(x.Decoder)
	case raoh.ListDecoder[any]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[string]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[int32]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[int64]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[float32]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[float64]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[bool]:
		return boxed(x.Decoder)
	case raoh.ListDecoder[raoh.Decimal]:
		return boxed(x.Decoder)
	case raoh.DictDecoder[any]:
		return boxed(x.Decoder)
	case raoh.Conversion[raoh.UUID]:
		return boxed(x.Decoder)
	case raoh.Conversion[raoh.URI]:
		return boxed(x.Decoder)
	}
	panic(fmt.Sprintf("no decoder of a %T", d))
}

func stringOperation(d raoh.StringDecoder, name string, args []any, message *string) (node, error) {
	str := func(i int) string { return args[i].(string) }
	length := func() int { return int(int32Of(args[0])) }
	var r raoh.StringDecoder
	switch name {
	case "trim":
		r = d.Trim()
	case "toLowerCase":
		r = d.ToLower()
	case "toUpperCase":
		r = d.ToUpper()
	case "normalize":
		form := raoh.NFC
		if len(args) > 0 {
			switch str(0) {
			case "NFC":
			case "NFD":
				form = raoh.NFD
			case "NFKC":
				form = raoh.NFKC
			case "NFKD":
				form = raoh.NFKD
			default:
				return node{}, fmt.Errorf("no normalization form %q", str(0))
			}
		}
		r = d.NormalizeAs(form)
	case "nonBlank":
		r = d.NonBlank()
	case "minLength":
		r = d.MinLength(length())
	case "maxLength":
		r = d.MaxLength(length())
	case "fixedLength":
		r = d.Length(length())
	case "oneOf":
		r = d.OneOf(stringList(args[0])...)
	case "startsWith":
		r = d.StartsWith(str(0))
	case "endsWith":
		r = d.EndsWith(str(0))
	case "includes":
		r = d.Contains(str(0))
	case "pattern":
		r = d.Pattern(str(0))
	case "email":
		r = d.Email()
	case "ipv4":
		r = d.IPv4()
	case "ipv6":
		r = d.IPv6()
	case "ip":
		r = d.IP()
	case "ulid":
		r = d.ULID()
	case "cuid":
		r = d.CUID()
	case "uuid":
		return withMessage(&typ{kind: "uuid"}, d.UUID(), message)
	case "url":
		return withMessage(&typ{kind: "uri"}, d.URL(), message)
	case "uri":
		return withMessage(&typ{kind: "uri"}, d.URI(), message)
	case "toInt":
		return withMessage(tInt32, d.ToInt(), message)
	case "toLong":
		return withMessage(&typ{kind: "int64"}, d.ToLong(), message)
	case "toDecimal":
		return withMessage(&typ{kind: "decimal"}, d.ToDecimal(), message)
	case "toBool":
		return withMessage(&typ{kind: "bool"}, d.ToBool(), message)
	case "iso8601":
		return withMessage(&typ{kind: "instant"}, d.Instant(), message)
	case "date":
		return withMessage(&typ{kind: "date"}, d.Date(), message)
	case "time":
		return withMessage(&typ{kind: "time"}, d.Time(), message)
	case "dateTime":
		return withMessage(&typ{kind: "datetime"}, d.DateTime(), message)
	case "offsetDateTime":
		return withMessage(&typ{kind: "offset_datetime"}, d.OffsetDateTime(), message)
	default:
		return node{}, fmt.Errorf("no string operation %q", name)
	}
	return withMessage(tString, r, message)
}

// numeric is what the numeric decoders of raoh-go have in common.
type numeric[T any, D any] interface {
	messaged[D]
	Min(T) D
	Max(T) D
	Range(T, T) D
	Positive() D
	Negative() D
	NonNegative() D
	NonPositive() D
}

func numberOperation[T any, D numeric[T, D]](t *typ, d D, name string, args []any, message *string, of func(any) T) (node, error) {
	var r D
	switch name {
	case "min":
		r = d.Min(of(args[0]))
	case "max":
		r = d.Max(of(args[0]))
	case "range":
		r = d.Range(of(args[0]), of(args[1]))
	case "positive":
		r = d.Positive()
	case "negative":
		r = d.Negative()
	case "nonNegative":
		r = d.NonNegative()
	case "nonPositive":
		r = d.NonPositive()
	case "multipleOf":
		m, ok := any(d).(interface{ MultipleOf(T) D })
		if !ok {
			return node{}, fmt.Errorf("no multipleOf on %v", t)
		}
		r = m.MultipleOf(of(args[0]))
	case "oneOf":
		o, ok := any(d).(interface{ OneOf(...T) D })
		if !ok {
			return node{}, fmt.Errorf("no oneOf on %v", t)
		}
		var allowed []T
		for _, a := range args[0].([]any) {
			allowed = append(allowed, of(a))
		}
		r = o.OneOf(allowed...)
	default:
		return node{}, fmt.Errorf("no numeric operation %q", name)
	}
	return withMessage(t, r, message)
}

// listOf is the list whose elements e decodes. Where e is a decoder of a
// scalar its elements are of its Go type, as a user of raoh-go writes a list:
// Contains, Unique and ToSet hold the elements as keys, which no interface
// type can be.
func listOf(e node) node {
	t := &typ{kind: "list", args: []*typ{e.t}}
	typed := func(d any) node { return node{t, d, boxedTyped(d)} }
	switch d := e.typed.(type) {
	case raoh.StringDecoder:
		return typed(raoh.List(d))
	case raoh.IntDecoder[int32]:
		return typed(raoh.List(d))
	case raoh.IntDecoder[int64]:
		return typed(raoh.List(d))
	case raoh.Float32Decoder:
		return typed(raoh.List(d))
	case raoh.Float64Decoder:
		return typed(raoh.List(d))
	case raoh.BoolDecoder:
		return typed(raoh.List(d))
	case raoh.DecimalDecoder:
		return typed(raoh.List(d))
	}
	return typed(raoh.List(e.g))
}

func listOperation[T comparable](t *typ, d raoh.ListDecoder[T], name string, args []any, message *string) (node, error) {
	element := func(o any) (T, error) {
		v, err := materialise(t.args[0], o)
		if err != nil {
			var zero T
			return zero, err
		}
		return v.(T), nil
	}
	var r raoh.ListDecoder[T]
	switch name {
	case "nonempty":
		r = d.NonEmpty()
	case "minSize":
		r = d.MinSize(int(int32Of(args[0])))
	case "maxSize":
		r = d.MaxSize(int(int32Of(args[0])))
	case "fixedSize":
		r = d.Size(int(int32Of(args[0])))
	case "unique":
		r = d.Unique()
	case "contains":
		e, err := element(args[0])
		if err != nil {
			return node{}, err
		}
		r = d.Contains(e)
	case "containsAll":
		var elements []T
		for _, o := range args[0].([]any) {
			e, err := element(o)
			if err != nil {
				return node{}, err
			}
			elements = append(elements, e)
		}
		r = d.ContainsAll(elements...)
	case "toSet":
		return node{&typ{kind: "set", args: t.args}, nil, boxed(raoh.ToSet(d))}, nil
	default:
		return node{}, fmt.Errorf("no list operation %q", name)
	}
	return withMessage(t, r, message)
}

func int32Of(v any) int32 {
	n, err := strconv.ParseInt(string(number(v)), 10, 32)
	if err != nil {
		panic(err)
	}
	return int32(n)
}

func int64Of(v any) int64 {
	n, err := strconv.ParseInt(string(number(v)), 10, 64)
	if err != nil {
		panic(err)
	}
	return n
}

func float32Of(v any) float32 {
	f, err := materialise(&typ{kind: "float32"}, v)
	if err != nil {
		panic(err)
	}
	return f.(float32)
}

func float64Of(v any) float64 {
	f, err := materialise(&typ{kind: "float64"}, v)
	if err != nil {
		panic(err)
	}
	return f.(float64)
}

func decimalOf(v any) raoh.Decimal {
	d, err := materialise(&typ{kind: "decimal"}, v)
	if err != nil {
		panic(err)
	}
	return d.(raoh.Decimal)
}

type timeT = time.Time

// operationShape is how many value arguments an operation takes and whether it
// takes a message, read from catalog/operations.json.
type operationShape struct {
	values  int
	message bool
}

var operationShapes = map[string]operationShape{}
