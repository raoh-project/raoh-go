package raoh_test

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
)

type Email string
type Age int

type User struct {
	email    Email
	age      Age
	nickname *string
}

func NewUser(e Email, a Age, n *string) User { return User{e, a, n} }

var emailDecoder = raoh.String().Trim().NonBlank().Map(func(s string) Email { return Email(s) })
var ageDecoder = raoh.Int().Range(0, 150).Map(func(n int) Age { return Age(n) })

var userDecoder = raoh.Object(
	raoh.Fields().
		Field("email", emailDecoder).
		Field("age", ageDecoder).
		Field("nickname", raoh.Optional(raoh.String().Trim())),
).Strict().Map(NewUser)

type summary struct{ path, code, key string }

func issuesOf(t *testing.T, err error) []summary {
	t.Helper()
	issues, ok := errors.AsType[*raoh.Issues](err)
	if !ok {
		t.Fatalf("want *raoh.Issues, got %v", err)
	}
	var out []summary
	for _, i := range issues.All() {
		out = append(out, summary{i.Path().String(), i.Code(), i.MessageKey()})
	}
	return out
}

func TestObjectDecodesEveryField(t *testing.T) {
	u, err := userDecoder.Decode(map[string]any{"email": " a@example.com ", "age": 30, "nickname": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if u.email != "a@example.com" || u.age != 30 || u.nickname == nil || *u.nickname != "k" {
		t.Fatalf("got %+v", u)
	}
}

func TestObjectAccumulatesIssuesOfEveryField(t *testing.T) {
	_, err := userDecoder.Decode(map[string]any{"email": "  ", "age": 200.0, "extra": 1, "another": 2})
	got := issuesOf(t, err)
	want := []summary{
		{"/email", "blank", "blank"},
		{"/age", "out_of_range", "out_of_range.range"},
		{"/another", "unknown_field", "unknown_field"},
		{"/extra", "unknown_field", "unknown_field"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestMissingFieldIsRequiredAndMissingOptionalIsNil(t *testing.T) {
	_, err := userDecoder.Decode(map[string]any{"age": 1})
	if got := issuesOf(t, err); !slices.Equal(got, []summary{{"/email", "required", "required"}}) {
		t.Fatalf("got %v", got)
	}
	u, err := userDecoder.Decode(map[string]any{"email": "a", "age": 1})
	if err != nil || u.nickname != nil {
		t.Fatalf("got %+v, %v", u, err)
	}
}

// Each field checks for itself that the input is an object: a required field
// reports type_mismatch at its member, and an optional one reads the input as
// not having the member.
func TestNonObjectInput(t *testing.T) {
	_, err := userDecoder.Decode("x")
	if got := issuesOf(t, err); !slices.Equal(got, []summary{
		{"/email", "type_mismatch", "type_mismatch"}, {"/age", "type_mismatch", "type_mismatch"}}) {
		t.Fatalf("got %v", got)
	}
}

type Period struct{ start, end time.Time }

var errStore = errors.New("store unavailable")

func period(fail error) raoh.Decoder[any, Period] {
	date := raoh.String().AndThen(func(s string) (time.Time, error) {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return time.Time{}, raoh.Invalid(raoh.NewIssue(raoh.CodeInvalidFormat))
		}
		return t, nil
	})
	return raoh.Object(
		raoh.Fields().
			Field("start", date).
			Field("end", date),
	).AndThen(func(start, end time.Time) (Period, error) {
		if fail != nil {
			return Period{}, fail
		}
		if !start.Before(end) {
			return Period{}, fmt.Errorf("new period: %w",
				raoh.Invalid(raoh.NewIssue(raoh.CodeInvalidValue).WithMessageKey("period.start_before_end")))
		}
		return Period{start, end}, nil
	})
}

func TestAndThenIssuesAreReportedAtTheObject(t *testing.T) {
	d := raoh.Object(raoh.Fields().Field("period", period(nil))).Map(func(p Period) Period { return p })
	_, err := d.Decode(map[string]any{"period": map[string]any{"start": "2026-02-01", "end": "2026-01-01"}})
	if got := issuesOf(t, err); !slices.Equal(got, []summary{{"/period", "invalid_value", "period.start_before_end"}}) {
		t.Fatalf("got %v", got)
	}
	_, err = d.Decode(map[string]any{"period": map[string]any{"start": "x", "end": "y"}})
	want := []summary{{"/period/start", "invalid_format", "invalid_format"}, {"/period/end", "invalid_format", "invalid_format"}}
	if got := issuesOf(t, err); !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestOrdinaryErrorStopsTheDecode(t *testing.T) {
	in := map[string]any{"start": "2026-01-01", "end": "2026-02-01"}
	if _, err := period(errStore).Decode(in); err != errStore {
		t.Fatalf("got %v", err)
	}
	mixed := errors.Join(raoh.Invalid(raoh.NewIssue("x")), io.ErrUnexpectedEOF)
	// A join holding an ordinary error stops the decode, and the issues in it
	// are hidden so that the caller does not take it for invalid input.
	_, err := period(mixed).Decode(in)
	if _, ok := errors.AsType[*raoh.Issues](err); ok {
		t.Fatalf("issues must not be found in a failure of the program: %v", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) || err.Error() != mixed.Error() {
		t.Fatalf("got %v", err)
	}
}

func TestJoinOfIssuesIsMerged(t *testing.T) {
	joined := errors.Join(raoh.Invalid(raoh.NewIssue("a")), raoh.Invalid(raoh.NewIssue("b")))
	_, err := period(joined).Decode(map[string]any{"start": "2026-01-01", "end": "2026-02-01"})
	if got := issuesOf(t, err); !slices.Equal(got, []summary{{"", "a", "a"}, {"", "b", "b"}}) {
		t.Fatalf("got %v", got)
	}
}

func TestEmptyObject(t *testing.T) {
	d := raoh.Object(raoh.Fields()).Strict().Map(func() struct{} { return struct{}{} })
	if _, err := d.Decode(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	_, err := d.Decode(map[string]any{"a": 1})
	if got := issuesOf(t, err); !slices.Equal(got, []summary{{"/a", "unknown_field", "unknown_field"}}) {
		t.Fatalf("got %v", got)
	}
}

func TestErrorMessage(t *testing.T) {
	_, err := userDecoder.Decode(map[string]any{"email": "a", "age": -1})
	if got, want := err.Error(), "/age: must be between 0 and 150"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type extra struct {
	p16 int32
	p17 string
}

type row struct {
	p1, p2, p3, p4, p5, p6, p7, p8, p9, p10, p11, p12, p13, p14, p15 int32
	extra                                                            extra
}

var extraDecoder = raoh.Object(
	raoh.Fields().
		Field("p16", raoh.Int32()).
		Field("p17", raoh.String()),
).Map(func(p16 int32, p17 string) extra { return extra{p16, p17} })

var rowDecoder = raoh.Object(
	raoh.Fields().
		Field("p1", raoh.Int32()).Field("p2", raoh.Int32()).Field("p3", raoh.Int32()).
		Field("p4", raoh.Int32()).Field("p5", raoh.Int32()).Field("p6", raoh.Int32()).
		Field("p7", raoh.Int32()).Field("p8", raoh.Int32()).Field("p9", raoh.Int32()).
		Field("p10", raoh.Int32()).Field("p11", raoh.Int32()).Field("p12", raoh.Int32()).
		Field("p13", raoh.Int32()).Field("p14", raoh.Int32()).Field("p15", raoh.Int32()).
		Flat(extraDecoder),
).Map(func(p1, p2, p3, p4, p5, p6, p7, p8, p9, p10, p11, p12, p13, p14, p15 int32, e extra) row {
	return row{p1, p2, p3, p4, p5, p6, p7, p8, p9, p10, p11, p12, p13, p14, p15, e}
})

func TestFlatReadsTheFieldsOfTheSameObject(t *testing.T) {
	in := map[string]any{"p17": "x", "p16": 16}
	for i := 1; i <= 15; i++ {
		in[fmt.Sprintf("p%d", i)] = i
	}
	r, err := rowDecoder.Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if r.p1 != 1 || r.p15 != 15 || r.extra != (extra{16, "x"}) {
		t.Fatalf("got %+v", r)
	}
}

func TestFlatIssuesAreAtTheObjectsOwnPathAndAccumulateWithTheFields(t *testing.T) {
	in := map[string]any{"p1": "a", "p16": "b"}
	for i := 2; i <= 15; i++ {
		in[fmt.Sprintf("p%d", i)] = i
	}
	_, err := rowDecoder.Decode(in)
	got := issuesOf(t, err)
	want := []summary{
		{"/p1", "type_mismatch", "type_mismatch"},
		{"/p16", "type_mismatch", "type_mismatch"},
		{"/p17", "required", "required"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestFlatIsGivenTheInputTheObjectWasGiven(t *testing.T) {
	type named map[string]any
	var seen any
	probe := raoh.NewDecoder(func(in any) (string, error) {
		seen = in
		return "", nil
	})
	d := raoh.Object(raoh.Fields().Flat(probe)).Map(func(string) int { return 0 })
	in := named{"a": 1}
	if _, err := d.Decode(in); err != nil {
		t.Fatal(err)
	}
	if got, ok := seen.(named); !ok || got["a"] != 1 {
		t.Fatalf("flat was given %#v, want the input %#v", seen, in)
	}
}

func TestFlatNestsToAnyDepth(t *testing.T) {
	inner := raoh.Object(raoh.Fields().Field("c", raoh.Int())).Map(func(c int) int { return c })
	middle := raoh.Object(raoh.Fields().Field("b", raoh.Int()).Flat(inner)).Map(func(b, c int) int { return b*10 + c })
	outer := raoh.Object(raoh.Fields().Flat(middle).Field("a", raoh.Int())).Map(func(bc, a int) int { return a*100 + bc })
	got, err := outer.Decode(map[string]any{"a": 1, "b": 2, "c": 3})
	if err != nil || got != 123 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestFlatStopsTheDecodeOnAnErrorThatIsNotIssues(t *testing.T) {
	boom := errors.New("boom")
	failing := raoh.NewDecoder(func(any) (int, error) { return 0, boom })
	d := raoh.Object(raoh.Fields().Field("a", raoh.Int()).Flat(failing)).Map(func(a, b int) int { return a })
	if _, err := d.Decode(map[string]any{"a": 1}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestFlatObjectAndThenWithPathIsGivenThePathOfTheObject(t *testing.T) {
	inner := raoh.Object(raoh.Fields().Field("b", raoh.Int())).Map(func(b int) int { return b })
	d := raoh.Object(raoh.Fields().Field("a", raoh.Int()).Flat(inner)).
		AndThenWithPath(func(a, b int, at raoh.Path) (int, error) { return a + b, nil })
	if got, err := d.Decode(map[string]any{"a": 1, "b": 2}); err != nil || got != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}
