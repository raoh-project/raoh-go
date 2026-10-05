package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/encode"
)

// observe writes v, a value of t as the runner holds it, as its observation
// (spec/observation.md).
func observe(t *typ, v any) (any, error) {
	switch t.kind {
	case "string", "symbol":
		return v.(string), nil
	case "uri":
		return v.(raoh.URI).String(), nil
	case "uuid":
		return v.(raoh.UUID).String(), nil
	case "bool":
		return v.(bool), nil
	case "int32":
		return json.Number(strconv.FormatInt(int64(v.(int32)), 10)), nil
	case "int64":
		return json.Number(strconv.FormatInt(v.(int64), 10)), nil
	case "float32":
		return observeFloat(float64(v.(float32)), 32), nil
	case "float64":
		return observeFloat(v.(float64), 64), nil
	case "decimal":
		return v.(raoh.Decimal).String(), nil
	case "instant":
		return encode.Instant()(v.(time.Time)), nil
	case "date":
		return encode.Date()(v.(time.Time)), nil
	case "time":
		return encode.Time()(v.(time.Time)), nil
	case "datetime":
		return encode.DateTime()(v.(time.Time)), nil
	case "offset_datetime":
		return encode.OffsetDateTime()(v.(time.Time)), nil
	case "list", "product":
		elements := reflect.ValueOf(v)
		out := make([]any, elements.Len())
		for n := range elements.Len() {
			et := t.args[0]
			if t.kind == "product" {
				et = t.args[n]
			}
			o, err := observe(et, elements.Index(n).Interface())
			if err != nil {
				return nil, err
			}
			out[n] = o
		}
		return out, nil
	case "set":
		out := []any{}
		for it := reflect.ValueOf(v).MapRange(); it.Next(); {
			o, err := observe(t.args[0], it.Key().Interface())
			if err != nil {
				return nil, err
			}
			out = append(out, o)
		}
		return out, nil
	case "map":
		out := map[string]any{}
		for k, e := range v.(map[string]any) {
			o, err := observe(t.args[0], e)
			if err != nil {
				return nil, err
			}
			out[k] = o
		}
		return out, nil
	case "nullable", "optional":
		p := v.(*any)
		if p == nil {
			return nil, nil
		}
		return observe(t.args[0], *p)
	case "presence":
		p := v.(raoh.Presence[any])
		if p.IsAbsent() {
			return "absent", nil
		}
		if p.IsNull() {
			return "null", nil
		}
		value, _ := p.Value()
		o, err := observe(t.args[0], value)
		if err != nil {
			return nil, err
		}
		return map[string]any{"present": o}, nil
	}
	return nil, fmt.Errorf("no observation of %v", t)
}

// observeFloat writes a float as its canonical decimal, or as a tag where
// a JSON number cannot carry it.
func observeFloat(f float64, bits int) any {
	switch {
	case math.IsNaN(f):
		return map[string]any{"float": "NaN"}
	case math.IsInf(f, 1):
		return map[string]any{"float": "+Infinity"}
	case math.IsInf(f, -1):
		return map[string]any{"float": "-Infinity"}
	case f == 0 && math.Signbit(f):
		return map[string]any{"float": "-0"}
	case f == 0:
		return json.Number("0")
	}
	return json.Number(canonicalDecimal(f, bits))
}

// canonicalDecimal is the canonical decimal of a finite non-zero float: of the
// shortest decimals that round to it, the closest, except that where one digit
// would do, two are allowed and the closest of those is taken.
func canonicalDecimal(f float64, bits int) string {
	shortest := strconv.FormatFloat(f, 'e', -1, bits)
	if digits(shortest) > 1 {
		return shortest
	}
	two := strconv.FormatFloat(f, 'e', 1, bits)
	if back, err := strconv.ParseFloat(two, bits); err == nil && back == f {
		return two
	}
	return shortest
}

// digits is how many significant digits a number written by FormatFloat in
// 'e' format has.
func digits(s string) int {
	mantissa, _, _ := strings.Cut(strings.TrimPrefix(s, "-"), "e")
	return len(strings.TrimRight(strings.Replace(mantissa, ".", "", 1), "0"))
}

// observeMeta writes a value of an issue's metadata, by the Go type it has.
func observeMeta(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string, bool:
		return x, nil
	case int:
		return json.Number(strconv.Itoa(x)), nil
	case int32:
		return json.Number(strconv.FormatInt(int64(x), 10)), nil
	case int64:
		return json.Number(strconv.FormatInt(x, 10)), nil
	case float32:
		return observeFloat(float64(x), 32), nil
	case float64:
		return observeFloat(x, 64), nil
	case raoh.Decimal:
		return x.String(), nil
	case raoh.UUID:
		return x.String(), nil
	case fmt.Stringer:
		return x.String(), nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		out := make([]any, rv.Len())
		for n := range rv.Len() {
			o, err := observeMeta(rv.Index(n).Interface())
			if err != nil {
				return nil, err
			}
			out[n] = o
		}
		return out, nil
	case reflect.Map:
		out := map[string]any{}
		keys := rv.MapKeys()
		sort.Slice(keys, func(a, b int) bool { return fmt.Sprint(keys[a]) < fmt.Sprint(keys[b]) })
		for _, k := range keys {
			o, err := observeMeta(rv.MapIndex(k).Interface())
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(k.Interface())] = o
		}
		return out, nil
	}
	return nil, fmt.Errorf("no observation is known for a %T in metadata", v)
}

// materialise builds the value an observation of t denotes, as the runner
// holds a value of t.
func materialise(t *typ, o any) (any, error) {
	switch t.kind {
	case "string", "symbol":
		s, ok := o.(string)
		if !ok {
			return nil, fmt.Errorf("%v is not a string", o)
		}
		return s, nil
	case "bool":
		b, ok := o.(bool)
		if !ok {
			return nil, fmt.Errorf("%v is not a bool", o)
		}
		return b, nil
	case "int32":
		n, err := strconv.ParseInt(string(o.(json.Number)), 10, 32)
		return int32(n), err
	case "int64":
		return strconv.ParseInt(string(o.(json.Number)), 10, 64)
	case "float32", "float64":
		bits := 64
		if t.kind == "float32" {
			bits = 32
		}
		var f float64
		switch x := o.(type) {
		case json.Number:
			var err error
			if f, err = strconv.ParseFloat(string(x), bits); err != nil {
				return nil, err
			}
		case map[string]any:
			switch x["float"] {
			case "-0":
				f = math.Copysign(0, -1)
			case "NaN":
				f = math.NaN()
			case "+Infinity":
				f = math.Inf(1)
			case "-Infinity":
				f = math.Inf(-1)
			default:
				return nil, fmt.Errorf("no float tag %v", x)
			}
		default:
			return nil, fmt.Errorf("%v is not a float", o)
		}
		if bits == 32 {
			return float32(f), nil
		}
		return f, nil
	case "decimal":
		return raoh.ParseDecimal(o.(string))
	case "instant", "date", "time", "datetime", "offset_datetime":
		s, ok := o.(string)
		if !ok {
			return nil, fmt.Errorf("%v is not a temporal text", o)
		}
		var d raoh.TemporalDecoder
		switch t.kind {
		case "instant":
			d = raoh.String().Instant()
		case "date":
			d = raoh.String().Date()
		case "time":
			d = raoh.String().Time()
		case "datetime":
			d = raoh.String().DateTime()
		default:
			d = raoh.String().OffsetDateTime()
		}
		return d.Decode(s)
	case "list":
		elements, ok := o.([]any)
		if !ok {
			return nil, fmt.Errorf("%v is not a list", o)
		}
		out := make([]any, len(elements))
		for n, e := range elements {
			v, err := materialise(t.args[0], e)
			if err != nil {
				return nil, err
			}
			out[n] = v
		}
		return out, nil
	case "product":
		parts, ok := o.([]any)
		if !ok || len(parts) != len(t.args) {
			return nil, fmt.Errorf("%v is not a product of %d", o, len(t.args))
		}
		out := make([]any, len(parts))
		for n, p := range parts {
			v, err := materialise(t.args[n], p)
			if err != nil {
				return nil, err
			}
			out[n] = v
		}
		return out, nil
	case "nullable", "optional":
		if o == nil {
			return (*any)(nil), nil
		}
		v, err := materialise(t.args[0], o)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	return nil, fmt.Errorf("no materialisation of %v", t)
}
