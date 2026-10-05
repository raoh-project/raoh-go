package main

import (
	"encoding/json"
	"fmt"

	"github.com/raoh-project/raoh-go"
)

// node is a decoder a form names, as far as the runner has bound it: its type,
// and the decoder of raoh-go it is, typed where an operation of its kind can
// still be applied and otherwise only as values of any.
type node struct {
	t     *typ
	typed any
	g     raoh.Decoder[any, any]
}

// binder translates the forms of one case into raoh-go and records the
// features they use.
type binder struct {
	features map[string]bool
}

func (b *binder) use(feature string) { b.features[feature] = true }

// boxed is d as a decoder of values of any.
func boxed[T any](d raoh.Decoder[any, T]) raoh.Decoder[any, any] {
	return d.Map(func(v T) any { return v })
}

// decoder binds a decoder form.
func (b *binder) decoder(form any) (node, error) {
	f, ok := form.([]any)
	if !ok || len(f) == 0 {
		return node{}, fmt.Errorf("not a form: %v", form)
	}
	name, _ := f[0].(string)
	b.use("decoder." + name)
	n, rest, err := b.constructor(name, f[1:])
	if err != nil {
		return node{}, err
	}
	for _, op := range rest {
		if n, err = b.operation(n, op); err != nil {
			return node{}, err
		}
	}
	return n, nil
}

// message takes the optional trailing message of a constructor's arguments.
func (b *binder) message(feature string, rest []any) (*string, []any) {
	if len(rest) > 0 {
		if m, ok := rest[0].(string); ok {
			b.use(feature + ".message")
			return &m, rest[1:]
		}
	}
	return nil, rest
}

func stringList(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	return out
}

// constructor binds a constructor and its arguments, and answers the
// operations after them.
func (b *binder) constructor(name string, args []any) (node, []any, error) {
	scalar := func(t string, typed any, g raoh.Decoder[any, any]) (node, []any, error) {
		return node{&typ{kind: t}, typed, g}, args, nil
	}
	switch name {
	case "string":
		d := raoh.String()
		return scalar("string", d, boxed(d.Decoder))
	case "int":
		d := raoh.Int32()
		return scalar("int32", d, boxed(d.Decoder))
	case "long":
		d := raoh.Int64()
		return scalar("int64", d, boxed(d.Decoder))
	case "float":
		d := raoh.Float32()
		return scalar("float32", d, boxed(d.Decoder))
	case "double":
		d := raoh.Float64()
		return scalar("float64", d, boxed(d.Decoder))
	case "decimal":
		d := raoh.DecimalNumber()
		return scalar("decimal", d, boxed(d.Decoder))
	case "bool":
		d := raoh.Bool()
		return scalar("bool", d, boxed(d.Decoder))
	case "list":
		e, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		return listOf(e), args[1:], nil
	case "dict":
		e, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		d := raoh.Dict(e.g)
		return node{&typ{kind: "map", args: []*typ{e.t}}, d, boxed(d.Decoder)}, args[1:], nil
	case "object", "strictObject":
		n, err := b.object(args[0].([]any), name == "strictObject")
		return n, args[1:], err
	case "strict":
		inner, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		return node{inner.t, nil, raoh.Strict(inner.g, stringList(args[1])...)}, args[2:], nil
	case "nullable":
		inner, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		return node{&typ{kind: "nullable", args: []*typ{inner.t}}, nil, boxed(raoh.Nullable(inner.g))}, args[1:], nil
	case "enum":
		symbols := stringList(args[0])
		str, err := b.stringDecoder(args[1])
		if err != nil {
			return node{}, nil, err
		}
		values := make(map[string]string, len(symbols))
		for _, s := range symbols {
			values[s] = s
		}
		d := raoh.EnumOfWith(values, str)
		message, rest := b.message("decoder.enum", args[2:])
		if message != nil {
			d = d.Message(*message)
		}
		return node{&typ{kind: "symbol", symbols: symbols}, nil, boxed(d.Decoder)}, rest, nil
	case "literal":
		str, err := b.stringDecoder(args[1])
		if err != nil {
			return node{}, nil, err
		}
		d := raoh.LiteralWith(args[0].(string), str)
		message, rest := b.message("decoder.literal", args[2:])
		if message != nil {
			d = d.Message(*message)
		}
		return node{tString, nil, boxed(d.Decoder)}, rest, nil
	case "discriminate":
		// Discriminate with the tag read by String, which is what Discriminate
		// is; its variants are written as a map here because a list of them
		// has a type outside raoh-go cannot name.
		variants := map[string]raoh.DecoderOf[any]{}
		var t *typ
		for tag, form := range args[1].(map[string]any) {
			v, err := b.decoder(form)
			if err != nil {
				return node{}, nil, err
			}
			t = v.t
			variants[tag] = v.g
		}
		return node{t, nil, raoh.DiscriminateWith(args[0].(string), raoh.String(), variants)}, args[2:], nil
	case "discriminateBy":
		tag, err := b.stringDecoder(args[1])
		if err != nil {
			return node{}, nil, err
		}
		variants := map[string]raoh.DecoderOf[any]{}
		var t *typ
		for name, form := range args[2].(map[string]any) {
			v, err := b.decoder(form)
			if err != nil {
				return node{}, nil, err
			}
			t = v.t
			variants[name] = v.g
		}
		return node{t, nil, raoh.DiscriminateBy(args[0].(string), tag, variants)}, args[3:], nil
	case "oneOf":
		var candidates []raoh.DecoderOf[any]
		var t *typ
		for _, form := range args[0].([]any) {
			c, err := b.decoder(form)
			if err != nil {
				return node{}, nil, err
			}
			t = c.t
			candidates = append(candidates, c.g)
		}
		return node{t, nil, raoh.OneOf(candidates...)}, args[1:], nil
	case "withDefault", "recover":
		inner, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		v, err := materialise(inner.t, args[1])
		if err != nil {
			return node{}, nil, err
		}
		if name == "withDefault" {
			return node{inner.t, nil, inner.g.Default(v)}, args[2:], nil
		}
		return node{inner.t, nil, inner.g.Fallback(v)}, args[2:], nil
	case "recoverWith":
		inner, err := b.decoder(args[0])
		if err != nil {
			return node{}, nil, err
		}
		fixture := args[1].(string)
		b.use("fixture." + fixture)
		if fixture != "issue_count_plus_10" {
			return node{}, nil, fmt.Errorf("no recover fixture %q", fixture)
		}
		return node{inner.t, nil, inner.g.FallbackFunc(func(is raoh.Issues) any {
			return int32(is.Len() + 10)
		})}, args[2:], nil
	}
	return node{}, nil, fmt.Errorf("no constructor %q", name)
}

// stringDecoder binds the string decoder enum and literal read with.
func (b *binder) stringDecoder(form any) (raoh.DecoderOf[string], error) {
	n, err := b.decoder(form)
	if err != nil {
		return nil, err
	}
	switch d := n.typed.(type) {
	case raoh.StringDecoder:
		return d, nil
	}
	return n.g.Map(func(v any) string { return v.(string) }), nil
}

// object binds the fields of an object. raoh-go types an object by the kinds
// and the order of its components, so each shape the suite uses is written
// out; a shape not written here is an error, and the case fails.
func (b *binder) object(fields []any, strict bool) (node, error) {
	type field struct {
		kind, name string
		n          node
	}
	var parts []field
	shape := ""
	t := &typ{kind: "product"}
	for _, x := range fields {
		f := x.([]any)
		kind := f[0].(string)
		b.use("field." + kind)
		form, name := f[len(f)-1], ""
		if kind != "flat" {
			name = f[1].(string)
		}
		n, err := b.decoder(form)
		if err != nil {
			return node{}, err
		}
		pt := n.t
		switch kind {
		case "field":
			shape += "f"
		case "optionalField":
			shape += "o"
			pt = &typ{kind: "optional", args: []*typ{n.t}}
		case "optionalNullableField":
			shape += "p"
			pt = &typ{kind: "presence", args: []*typ{n.t}}
		case "flat":
			shape += "L"
		}
		t.args = append(t.args, pt)
		parts = append(parts, field{kind, name, n})
	}
	p := func(i int) raoh.Decoder[any, any] { return parts[i].n.g }
	name := func(i int) string { return parts[i].name }
	product := func(v ...any) any { return v }
	var g raoh.Decoder[any, any]
	switch {
	case shape == "f":
		o := raoh.Object(raoh.Fields().Field(name(0), p(0)))
		if strict {
			o = o.Strict()
		}
		g = o.Map(func(a any) any { return product(a) })
	case shape == "ff":
		o := raoh.Object(raoh.Fields().Field(name(0), p(0)).Field(name(1), p(1)))
		if strict {
			o = o.Strict()
		}
		g = o.Map(func(a, b any) any { return product(a, b) })
	case shape == "fo":
		o := raoh.Object(raoh.Fields().Field(name(0), p(0)).Field(name(1), raoh.Optional(p(1))))
		if strict {
			o = o.Strict()
		}
		g = o.Map(func(a any, b *any) any { return product(a, b) })
	case shape == "fp":
		o := raoh.Object(raoh.Fields().Field(name(0), p(0)).Field(name(1), raoh.PresenceOf(p(1))))
		if strict {
			o = o.Strict()
		}
		g = o.Map(func(a any, b raoh.Presence[any]) any { return product(a, b) })
	case shape == "oo" && !strict:
		g = raoh.Object(raoh.Fields().Field(name(0), raoh.Optional(p(0))).Field(name(1), raoh.Optional(p(1)))).
			Map(func(a, b *any) any { return product(a, b) })
	case shape == "p" && !strict:
		g = raoh.Object(raoh.Fields().Field(name(0), raoh.PresenceOf(p(0)))).
			Map(func(a raoh.Presence[any]) any { return product(a) })
	case shape == "fL" && !strict:
		g = raoh.Object(raoh.Fields().Field(name(0), p(0)).Flat(p(1))).
			Map(func(a, b any) any { return product(a, b) })
	case shape == "Lf" && !strict:
		g = raoh.Object(raoh.Fields().Flat(p(0)).Field(name(1), p(1))).
			Map(func(a, b any) any { return product(a, b) })
	case shape == "fffffffffffffffff" && !strict:
		// More fields than a set of raoh-go holds, read as a user of it would:
		// the last two by an object of their own, added with Flat.
		tail := raoh.Object(raoh.Fields().Field(name(15), p(15)).Field(name(16), p(16))).
			Map(func(a, b any) any { return []any{a, b} })
		g = raoh.Object(raoh.Fields().
			Field(name(0), p(0)).Field(name(1), p(1)).Field(name(2), p(2)).Field(name(3), p(3)).
			Field(name(4), p(4)).Field(name(5), p(5)).Field(name(6), p(6)).Field(name(7), p(7)).
			Field(name(8), p(8)).Field(name(9), p(9)).Field(name(10), p(10)).Field(name(11), p(11)).
			Field(name(12), p(12)).Field(name(13), p(13)).Field(name(14), p(14)).Flat(tail)).
			Map(func(a0, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, rest any) any {
				return append([]any{a0, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14}, rest.([]any)...)
			})
	default:
		return node{}, fmt.Errorf("no binding of an object of the shape %q (strict: %v)", shape, strict)
	}
	return node{t, nil, g}, nil
}

// number is a JSON number of a form, which the suite file was read keeping.
func number(v any) json.Number { return v.(json.Number) }
