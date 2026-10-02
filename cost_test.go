package raoh_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/raoh-project/raoh-go"
)

// What making an issue costs does not grow with how many there are: on the paths where an issue
// is made for each element or member, and where issues are written out, the allocations for each
// issue are held to a bound, and are no more for ten times the issues. Counted in allocations,
// which do not vary from run to run as time does; a step that renders a path to look one up, a
// list that copies itself as it grows or a resource made afresh for each issue shows here.
func TestEachIssueCostsTheSameWhateverTheCount(t *testing.T) {
	strict := raoh.Strict(raoh.Object(raoh.Fields().Field("a", raoh.Int())).Map(func(a int) int { return a }), "a")
	list := raoh.List(raoh.Int())
	for _, each := range []struct {
		name  string
		most  float64
		input func(n int) func()
	}{
		{"a list whose every element fails", 18, func(n int) func() {
			text := []byte("[" + strings.TrimSuffix(strings.Repeat(`"x",`, n), ",") + "]")
			return func() { raoh.DecodeJSON(text, list) }
		}},
		{"a Strict around an object of unknown members", 10, func(n int) func() {
			var sb strings.Builder
			sb.WriteString(`{"a":1`)
			for i := range n {
				fmt.Fprintf(&sb, `,"k%d":1`, i)
			}
			sb.WriteString("}")
			text := []byte(sb.String())
			return func() { raoh.DecodeJSON(text, strict) }
		}},
		{"rendering the issues", 10, func(n int) func() {
			_, err := raoh.DecodeJSON([]byte("["+strings.TrimSuffix(strings.Repeat(`"x",`, n), ",")+"]"), list)
			is := err.(*raoh.Issues)
			return func() { is.Render(raoh.English) }
		}},
	} {
		per := func(n int) float64 { return testing.AllocsPerRun(3, each.input(n)) / float64(n) }
		few, many := per(100), per(1000)
		if few > each.most || many > each.most || many > few*1.2+1 {
			t.Errorf("%s: %.1f allocations an issue for 100, %.1f for 1000, at most %.0f", each.name, few, many, each.most)
		}
	}
}

// A path is written as a JSON Pointer in allocations that grow with its depth, not with each
// segment's escaping.
func TestWritingAPathCostsItsDepth(t *testing.T) {
	for _, p := range []raoh.Path{raoh.PathOf("a", "b", "c", "d"), raoh.PathOf("a/b", "c~d", "e", "f")} {
		if n := testing.AllocsPerRun(10, func() { _ = p.String() }); n > 7 {
			t.Errorf("%s: %.0f allocations", p, n)
		}
	}
}
