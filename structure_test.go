package raoh_test

import (
	"reflect"
	"testing"

	raoh "github.com/raoh-project/raoh-go"
)

func TestPathOf(t *testing.T) {
	if !raoh.PathOf().IsRoot() {
		t.Error("PathOf() is the root")
	}
	for _, tc := range []struct {
		segs []string
		want string
	}{
		{[]string{"user", "id"}, "/user/id"},
		{[]string{"", "a/b", "~x"}, "//a~1b/~0x"},
		{[]string{"items", "0"}, "/items/0"},
	} {
		if got := raoh.PathOf(tc.segs...).String(); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.segs, got, tc.want)
		}
	}
	q := raoh.Path{}.Key("items").Index(0).Key("tags").Index(12)
	if !q.Equal(raoh.PathOf(q.Segments()...)) {
		t.Error("Segments then PathOf must give an equal path")
	}
	if q.String() != raoh.PathOf(q.Segments()...).String() {
		t.Error("equal paths write the same JSON Pointer")
	}
	if raoh.PathOf("items", "01").Equal(raoh.Path{}.Key("items").Index(1)) {
		t.Error("\"01\" is not the segment 1")
	}
	if !raoh.PathOf("a", "b").Equal(raoh.PathOf("a", "b")) {
		t.Error("equal paths built apart")
	}
}

func TestPathAppend(t *testing.T) {
	var root raoh.Path
	p := root.Key("items").Index(2).Key("name")
	if !root.Append(p).Equal(p) || !p.Append(root).Equal(p) {
		t.Error("root is the identity")
	}
	got := root.Key("a").Append(root.Key("items").Index(2))
	if !got.Equal(root.Key("a").Key("items").Index(2)) {
		t.Errorf("index kind lost: %s", got)
	}
	if !got.Equal(raoh.PathOf("a", "items", "2")) {
		t.Error("Append changed a segment")
	}
}

func TestGroupByPathAgreesWithEqual(t *testing.T) {
	a, b := raoh.Path{}.Key("items").Index(0), raoh.PathOf("items", "0")
	g := raoh.Issues{}.Add(raoh.NewIssue("x").At(a), raoh.NewIssue("y").At(b)).GroupByPath()
	if !a.Equal(b) || len(g) != 1 {
		t.Errorf("equal paths must share a group: %v", g)
	}
}

func TestIssuesAddMergeRebaseLeaveReceiver(t *testing.T) {
	a := raoh.Issues{}.Add(raoh.NewIssue("x").At(raoh.PathOf("a")))
	b := a.Add(raoh.NewIssue("y"))
	if a.Len() != 1 || b.Len() != 2 {
		t.Fatalf("Add: %d %d", a.Len(), b.Len())
	}
	m := a.Merge(b)
	if m.Len() != 3 || a.Len() != 1 || b.Len() != 2 {
		t.Fatalf("Merge: %d", m.Len())
	}
	r := m.Rebase(raoh.PathOf("p"))
	if got := r.All()[0].Path().String(); got != "/p/a" {
		t.Error(got)
	}
	if got := m.All()[0].Path().String(); got != "/a" {
		t.Errorf("Rebase changed the receiver: %s", got)
	}
	if got := raoh.NewIssue("z").Rebase(raoh.PathOf("p")).Path().String(); got != "/p" {
		t.Error(got)
	}
}

func TestGroupByPath(t *testing.T) {
	is := raoh.Issues{}.Add(
		raoh.NewIssue("first").At(raoh.PathOf("a")),
		raoh.NewIssue("other").At(raoh.PathOf("b")),
		raoh.NewIssue("second").At(raoh.PathOf("a")),
	)
	g := is.GroupByPath()
	if len(g) != 2 || g["/a"].Len() != 2 || g["/b"].Len() != 1 {
		t.Fatalf("%v", g)
	}
	if g["/a"].All()[0].Code() != "first" || g["/a"].All()[1].Code() != "second" {
		t.Error("order within a path")
	}
}

func TestFormat(t *testing.T) {
	is := raoh.Issues{}.Add(
		raoh.NewIssue("required").At(raoh.PathOf("address", "city")),
		raoh.NewIssue("required").WithMessage("again").At(raoh.PathOf("address", "city")),
		raoh.NewIssue("required").At(raoh.Path{}.Key("items").Index(0)),
		raoh.NewIssue("required").At(raoh.PathOf("a/b", "~")),
		raoh.NewIssue("required"),
	)
	got := is.Format(raoh.English)
	want := map[string]any{
		"_errors": []string{"is required"},
		"address": map[string]any{"city": map[string]any{"_errors": []string{"is required", "again"}}},
		"items":   map[string]any{"0": map[string]any{"_errors": []string{"is required"}}},
		"a/b":     map[string]any{"~": map[string]any{"_errors": []string{"is required"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestInterpolate(t *testing.T) {
	meta := map[string]any{"min": 3, "v": "{other}", "other": "X"}
	if got := raoh.Interpolate("{min} of {max} {v} { }", meta); got != "3 of {max} {other} { }" {
		t.Error(got)
	}
	if got, ok := raoh.InterpolateFully("{min} {v}", meta); !ok || got != "3 {other}" {
		t.Error(got, ok)
	}
	if got, ok := raoh.InterpolateFully("{min} {max}", meta); ok || got != "" {
		t.Error(got, ok)
	}
}
