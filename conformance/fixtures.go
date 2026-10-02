package main

import (
	"fmt"
	"strconv"

	"github.com/raoh-project/raoh-go"
)

// generic binds map, refine and flatMap, which apply to a decoder of any type
// through the fixture they name (catalog/fixtures.json, spec/fixtures.md).
func (b *binder) generic(n node, name, fixture string) (node, error) {
	b.use("fixture." + fixture)
	switch name {
	case "map":
		var out *typ
		var f func(any) any
		switch fixture {
		case "first":
			out, f = n.t.args[0], func(v any) any { return v.([]any)[0] }
		case "square_side":
			out, f = tInt32, func(v any) any { s := v.([]any)[0].(int32); return s * s }
		case "square":
			out, f = tInt32, func(v any) any { s := v.([]any)[0].(int32); return s * s }
		case "area":
			out, f = tInt32, func(v any) any { p := v.([]any); return p[0].(int32) * p[1].(int32) }
		case "shift_add_10", "shift_add_100", "shift_add_1000":
			k := map[string]int32{"shift_add_10": 10, "shift_add_100": 100, "shift_add_1000": 1000}[fixture]
			out, f = tInt32, func(v any) any { p := v.([]any); return p[0].(int32)*k + p[1].(int32) }
		case "decimal_string":
			out, f = tString, func(v any) any { return strconv.FormatInt(int64(v.(int32)), 10) }
		default:
			return node{}, fmt.Errorf("no map fixture %q", fixture)
		}
		return node{out, nil, n.g.Map(f)}, nil
	case "refine":
		if fixture != "even" {
			return node{}, fmt.Errorf("no refine fixture %q", fixture)
		}
		return node{n.t, nil, n.g.RefineWithMeta(
			func(v any) bool { return v.(int32)%2 == 0 },
			"must_be_even", "must be even",
			func(v any) map[string]any { return map[string]any{"actual": v} })}, nil
	case "flatMap":
		if fixture != "ordered_period" {
			return node{}, fmt.Errorf("no flatMap fixture %q", fixture)
		}
		return node{n.t, nil, n.g.AndThen(func(v any) (any, error) {
			p := v.([]any)
			if p[0].(int32) <= p[1].(int32) {
				return v, nil
			}
			return nil, raoh.Invalid(raoh.NewIssue("invalid_value").WithMessage("end is before start").At(raoh.PathOf("end")))
		})}, nil
	}
	return node{}, fmt.Errorf("no generic operation %q", name)
}
