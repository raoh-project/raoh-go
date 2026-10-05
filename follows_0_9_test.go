package raoh_test

import (
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
)

// Email accepts an ASCII profile of RFC 5321's Mailbox, as the Raoh Specification 0.9 says.
func TestEmailIsAnASCIIProfileOfRFC5321(t *testing.T) {
	long := func(n int) string { return string(slices.Repeat([]byte{'a'}, n)) }
	for in, ok := range map[string]bool{
		"a@b.co": true, "a@localhost": true, "a@123": true, "o'brien@example.com": true, "a@b.c": true,
		"!#$%&'*+-/=?^_`{|}~@x": true, "a.b.c@d-e.f": true,
		long(64) + "@b.co": true, long(65) + "@b.co": false,
		"a@" + long(63) + ".co": true, "a@" + long(64) + ".co": false,
		"a@" + long(63) + "." + long(63) + "." + long(63) + "." + long(60): true,
		"a@" + long(63) + "." + long(63) + "." + long(63) + "." + long(61): false,
		".a@b.co": false, "a.@b.co": false, "a..b@b.co": false, "a@-b.co": false, "a@b-.co": false,
		"a@b..co": false, "a@b.co.": false, "\"a\"@b.co": false, "a@[127.0.0.1]": false, "é@b.co": false,
		"a@b@c": false, "@b.co": false, "a@": false, "a": false,
	} {
		if _, issues := decodeJSONValue(t, raoh.String().Email(), in); (issues == nil) != ok {
			t.Errorf("%q: %v", in, issues)
		}
	}
}

// A ULID is Crockford's base 32 in either case, and fits in 128 bits.
func TestAULIDIsEitherCaseAndAt128Bits(t *testing.T) {
	for in, ok := range map[string]bool{
		"01ARZ3NDEKTSV4RRFFQ69G5FAV": true, "01arz3ndektsv4rrffq69g5fav": true,
		"7ZZZZZZZZZZZZZZZZZZZZZZZZZ": true, "80000000000000000000000000": false,
		"01ARZ3NDEKTSV4RRFFQ69G5FAI": false, "01arz3ndektsv4rrffq69g5fau": false, "01ARZ3NDEKTSV4RRFFQ69G5FA": false,
	} {
		got, issues := decodeJSONValue(t, raoh.String().ULID(), in)
		if (issues == nil) != ok || ok && got != in {
			t.Errorf("%q: %q, %v", in, got, issues)
		}
	}
}

// Offset date-times are compared by the instant alone.
func TestOffsetDateTimesCompareByInstant(t *testing.T) {
	bound := time.Date(2024, 1, 15, 10, 0, 0, 0, time.FixedZone("", 3600))
	same := `"2024-01-15T09:00Z"`
	if _, issues := decodeJSON(t, raoh.String().OffsetDateTime().Before(bound), same); issues == nil {
		t.Error("the same instant is before the bound")
	}
	if _, issues := decodeJSON(t, raoh.String().OffsetDateTime().After(bound), same); issues == nil {
		t.Error("the same instant is after the bound")
	}
	if got, issues := decodeJSON(t, raoh.String().OffsetDateTime().Between(bound, bound), same); issues != nil {
		t.Errorf("the same instant is between the bound and itself: %v", issues)
	} else if _, offset := got.Zone(); offset != 0 {
		t.Errorf("the value keeps the offset it was written with: %v", got)
	}
	// Bounds at one instant are in order, whatever their offsets.
	raoh.String().OffsetDateTime().Between(bound, bound.UTC())
}

// A zero written with a minus sign is -0 for a float in every form, and a float is the exact value
// rounded once.
func TestAFloatIsTheValueTheTextWritesRoundedOnce(t *testing.T) {
	for _, text := range []string{"-0", "-0.0", "-0.000e10"} {
		if got, _ := decodeJSON(t, raoh.Float64(), text); got != 0 || !math.Signbit(got) {
			t.Errorf("Float64 of %s is %v", text, got)
		}
		if got, _ := decodeJSON(t, raoh.Float32(), text); got != 0 || !math.Signbit(float64(got)) {
			t.Errorf("Float32 of %s is %v", text, got)
		}
	}
	if got, _ := decodeJSON(t, raoh.Int(), "-0"); got != 0 {
		t.Errorf("Int of -0 is %v", got)
	}
	// Just past the middle of 1 and the float after it: rounded once it is the float after 1, and
	// through a double it would be 1.
	if got, _ := decodeJSON(t, raoh.Float32(), "1.000000059604644775390625000000001"); got != math.Nextafter32(1, 2) {
		t.Errorf("Float32 rounds twice: %v", got)
	}
}

// Default gives the default for a null or missing value, looked at before the decoder runs.
func TestDefaultIsForANullOrMissingValue(t *testing.T) {
	type pair struct{ A, B int }
	obj := raoh.Object(raoh.Fields().Field("a", raoh.Int()).Field("b", raoh.Int())).
		Map(func(a, b int) pair { return pair{a, b} }).Default(pair{7, 7})
	if _, issues := decodeJSON(t, obj, `{"a":1}`); len(issues) != 1 || issues[0].Code() != raoh.CodeRequired {
		t.Errorf("an object with a missing member is not given the default: %v", issues)
	}
	if got, issues := decodeJSON(t, obj, `null`); issues != nil || got != (pair{7, 7}) {
		t.Errorf("null: %v, %v", got, issues)
	}
	five := 5
	if got, issues := decodeJSON(t, raoh.Nullable(raoh.Int()).Default(&five), `null`); issues != nil || got == nil || *got != 5 {
		t.Errorf("Nullable(Int()).Default given null: %v, %v", got, issues)
	}
	if _, issues := decodeJSON(t, raoh.Int().Default(5), `"x"`); len(issues) != 1 || issues[0].Code() != raoh.CodeTypeMismatch {
		t.Errorf("another value is decoded: %v", issues)
	}
}

// What an issue's metadata holds is the issue's own: changing what Meta returns changes neither
// the issue nor the decoder that made it.
func TestAnIssuesMetadataIsItsOwn(t *testing.T) {
	d := raoh.String().OneOf("a", "b", "c")
	_, issues := decodeJSON(t, d, `"x"`)
	issues[0].Meta()["allowed"].([]string)[0] = "zz"
	if issues[0].Meta()["allowed"].([]string)[0] != "a" {
		t.Error("the issue changed")
	}
	if _, issues := decodeJSON(t, d, `"a"`); issues != nil {
		t.Errorf("the decoder changed: %v", issues)
	}
	list := raoh.List(raoh.String()).ContainsAll("a", "b")
	_, issues = decodeJSON(t, list, `[]`)
	issues[0].Meta()["expected"].([]string)[0] = "zz"
	if _, issues := decodeJSON(t, list, `["a","b"]`); issues != nil {
		t.Errorf("the list decoder changed: %v", issues)
	}
	given := []string{"x"}
	issue := raoh.NewIssue("c").WithMeta("k", given)
	given[0] = "y"
	if issue.Meta()["k"].([]string)[0] != "x" {
		t.Error("the slice given to WithMeta changed the issue")
	}
}

// A strict inside a strict reports a member once, by the innermost one that does not know it, and
// the members accepted are those every one of them knows.
func TestAStrictInsideAStrictReportsAMemberOnce(t *testing.T) {
	type at struct{ path, code string }
	summary := func(issues []raoh.Issue) []at {
		var out []at
		for _, i := range issues {
			out = append(out, at{i.Path().String(), i.Code()})
		}
		return out
	}
	id := func(v int) int { return v }
	a := raoh.Object(raoh.Fields().Field("a", raoh.Int()))
	for name, tc := range map[string]struct {
		d    raoh.DecoderOf[int]
		in   string
		want []at
	}{
		"Strict in Strict": {raoh.Strict(raoh.Strict(a.Map(id), "a"), "a"), `{"a":1,"b":2}`,
			[]at{{"/b", raoh.CodeUnknownField}}},
		"Strict around an Object's Strict": {raoh.Strict(a.Strict().Map(id), "a"), `{"a":1,"b":2}`,
			[]at{{"/b", raoh.CodeUnknownField}}},
		"each reports what only it does not know": {raoh.Strict(raoh.Strict(a.Map(id), "a", "b"), "a", "c"),
			`{"a":1,"b":2,"c":3,"d":4}`, []at{{"/c", raoh.CodeUnknownField}, {"/d", raoh.CodeUnknownField}, {"/b", raoh.CodeUnknownField}}},
	} {
		if _, issues := decodeJSON(t, tc.d, tc.in); !slices.Equal(summary(issues), tc.want) {
			t.Errorf("%s: %v, want %v", name, summary(issues), tc.want)
		}
	}
	// Another issue at the member does not count as its report.
	b := raoh.Object(raoh.Fields().Field("b", raoh.Int()))
	if _, issues := decodeJSON(t, raoh.Strict(b.Map(id), "a"), `{"b":"x"}`); !slices.Equal(summary(issues),
		[]at{{"/b", raoh.CodeTypeMismatch}, {"/b", raoh.CodeUnknownField}}) {
		t.Errorf("a type_mismatch at the member: %v", summary(issues))
	}
	// An unknown_field of the caller's own does not count either.
	own := raoh.Strict(raoh.Object(raoh.Fields()).Map(func() int { return 0 }).AndThen(func(int) (int, error) {
		return 0, raoh.Invalid(raoh.NewIssue(raoh.CodeUnknownField).At(raoh.PathOf("b")))
	}), "a")
	if _, issues := decodeJSON(t, own, `{"b":1}`); !slices.Equal(summary(issues),
		[]at{{"/b", raoh.CodeUnknownField}, {"/b", raoh.CodeUnknownField}}) {
		t.Errorf("an unknown_field the caller made: %v", summary(issues))
	}
	// Nor does one a Strict made in another decode, which the caller kept and returned: what a
	// Strict marks lives as long as the decode it was made in.
	_, kept := raoh.DecodeJSON([]byte(`{"b":1}`), raoh.Strict(raoh.Object(raoh.Fields()).Map(func() int { return 0 }), "a"))
	reused := raoh.Strict(raoh.Object(raoh.Fields()).Map(func() int { return 0 }).AndThen(func(int) (int, error) {
		return 0, kept
	}), "a")
	if _, issues := decodeJSON(t, reused, `{"b":1}`); !slices.Equal(summary(issues),
		[]at{{"/b", raoh.CodeUnknownField}, {"/b", raoh.CodeUnknownField}}) {
		t.Errorf("an unknown_field another decode's Strict made: %v", summary(issues))
	}
	// Nor one handed to FallbackFunc and returned from a function the caller gave.
	var handed raoh.Issues
	recovered := raoh.Strict(a.Strict().Map(id).FallbackFunc(func(is raoh.Issues) int { handed = is; return 0 }), "a")
	decodeJSON(t, recovered, `{"a":1,"b":2}`)
	again := raoh.Strict(raoh.Object(raoh.Fields()).Map(func() int { return 0 }).AndThen(func(int) (int, error) {
		return 0, &handed
	}), "a")
	if _, issues := decodeJSON(t, again, `{"b":1}`); !slices.Equal(summary(issues),
		[]at{{"/b", raoh.CodeUnknownField}, {"/b", raoh.CodeUnknownField}}) {
		t.Errorf("an unknown_field FallbackFunc was handed: %v", summary(issues))
	}
}

// The candidates of a one_of_failed are kept as issues: written in the language the issue is
// written in, a custom message among them kept, and moved with it.
func TestTheCandidatesOfOneOfAreIssues(t *testing.T) {
	d := raoh.OneOf[any](
		raoh.Int().Map(func(v int) any { return v }),
		raoh.String().MinLength(3).Message("three or more").Map(func(v string) any { return v }))
	_, issues := decodeJSON(t, d, `true`)
	if len(issues) != 1 || issues[0].Code() != raoh.CodeOneOfFailed {
		t.Fatalf("%v", issues)
	}
	ja := mustIssues(t, issues).Render(raoh.Japanese)
	first := ja[0].Meta["candidates"].([]any)[0].(map[string]any)["issues"].([]any)[0].(map[string]any)
	if first["message"] != "long型が必要です" {
		t.Errorf("the candidate is written in Japanese: %v", first["message"])
	}
	_, issues = decodeJSON(t, d, `"ab"`)
	second := mustIssues(t, issues).Render(raoh.Japanese)[0].Meta["candidates"].([]any)[1].(map[string]any)["issues"].([]any)[0].(map[string]any)
	if second["message"] != "three or more" {
		t.Errorf("the custom message is kept: %v", second["message"])
	}
	// Rebase moves the candidates with the issue that holds them.
	_, issues = decodeJSON(t, d, `true`)
	moved := mustIssues(t, issues).Rebase(raoh.PathOf("y")).Render(raoh.English)[0]
	path := moved.Meta["candidates"].([]any)[0].(map[string]any)["issues"].([]any)[0].(map[string]any)["path"]
	if moved.Path != "/y" || path != "/y" {
		t.Errorf("the candidates are at %v, the issue at %v", path, moved.Path)
	}
}

func mustIssues(t *testing.T, issues []raoh.Issue) raoh.Issues {
	t.Helper()
	err := raoh.Invalid(issues...)
	is, ok := err.(*raoh.Issues)
	if !ok {
		t.Fatal(err)
	}
	return *is
}

// Adding to the most recent list takes time in what is added, and two lists made from one hold
// what each was given.
func TestAddingIssuesIsLinearAndListsStayApart(t *testing.T) {
	start := time.Now()
	var is raoh.Issues
	for n := range 100_000 {
		is = is.Add(raoh.NewIssue("c").WithMeta("n", n))
		if n%2 == 0 {
			is = is.Merge(raoh.Issues{}.Add(raoh.NewIssue("m")))
		}
	}
	if is.Len() != 150_000 {
		t.Fatalf("%d issues", is.Len())
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("150,000 additions took %v", elapsed)
	}
	base := raoh.Issues{}.Add(raoh.NewIssue("a"))
	left := base.Add(raoh.NewIssue("l"))
	right := base.Add(raoh.NewIssue("r"))
	left2 := left.Add(raoh.NewIssue("l2"))
	if base.Len() != 1 || left.All()[1].Code() != "l" || right.All()[1].Code() != "r" || left2.All()[2].Code() != "l2" {
		t.Errorf("base %v, left %v, right %v, left2 %v", base.All(), left.All(), right.All(), left2.All())
	}
	// Lists made from one at once, from several goroutines, each hold their own.
	var wg sync.WaitGroup
	got := make([]raoh.Issues, 8)
	for g := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[g] = base.Add(raoh.NewIssue("g").WithMeta("g", g))
		}()
	}
	wg.Wait()
	for g, each := range got {
		if each.Len() != 2 || each.All()[1].Meta()["g"] != g {
			t.Errorf("goroutine %d: %v", g, each.All())
		}
	}
}

// An enum and a literal take a message for their own issue, and the string decoder's issues keep
// theirs.
func TestEnumAndLiteralTakeAMessage(t *testing.T) {
	color := raoh.EnumOf(map[string]int{"red": 1}).Message("pick a color")
	if _, issues := decodeJSON(t, color, `"blue"`); len(issues) != 1 || issues[0].Message(raoh.English) != "pick a color" {
		t.Errorf("%v", issues)
	}
	if _, issues := decodeJSON(t, color, `1`); len(issues) != 1 || issues[0].Code() != raoh.CodeTypeMismatch ||
		issues[0].Message(raoh.English) == "pick a color" {
		t.Errorf("type_mismatch keeps its message: %v", issues)
	}
	v1 := raoh.Literal("v1").Message("only v1")
	if _, issues := decodeJSON(t, v1, `"v2"`); len(issues) != 1 || issues[0].Message(raoh.English) != "only v1" {
		t.Errorf("%v", issues)
	}
	if _, issues := decodeJSON(t, v1, `null`); len(issues) != 1 || issues[0].Code() != raoh.CodeRequired ||
		issues[0].Message(raoh.English) == "only v1" {
		t.Errorf("required keeps its message: %v", issues)
	}
	if got, issues := decodeJSON(t, raoh.Literal("v1"), `"v1"`); issues != nil || got != "v1" {
		t.Errorf("%v %v", got, issues)
	}
}

// The issue owns the tree of its metadata: slices, arrays and maps at any depth, in and out. A
// struct or a pointer is the caller's value, held as given.
func TestAnIssueOwnsTheTreeOfItsMetadata(t *testing.T) {
	nested := map[string]any{"list": []any{[]string{"a"}, [2][]int{{1}, {2}}}}
	i := raoh.NewIssue("c").WithMeta("tree", nested)
	nested["list"].([]any)[0].([]string)[0] = "changed"
	nested["list"].([]any)[1].([2][]int)[1][0] = 9
	got := i.Meta()["tree"].(map[string]any)["list"].([]any)
	if got[0].([]string)[0] != "a" || got[1].([2][]int)[1][0] != 2 {
		t.Errorf("the tree given changed the issue: %v", got)
	}
	got[0].([]string)[0] = "out"
	if i.Meta()["tree"].(map[string]any)["list"].([]any)[0].([]string)[0] != "a" {
		t.Error("the tree handed out changed the issue")
	}
	when := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if i := raoh.NewIssue("c").WithMeta("at", when); !i.Meta()["at"].(time.Time).Equal(when) {
		t.Error("a struct is held as given")
	}
}
