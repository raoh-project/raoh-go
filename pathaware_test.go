package raoh_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/raoh-project/raoh-go"
)

type span struct{ start, end int }

func spanDecoder() raoh.Decoder[any, span] {
	return raoh.Object(raoh.Fields().
		Field("start", raoh.Int()).
		Field("end", raoh.Int()),
	).AndThenWithPath(func(start, end int, at raoh.Path) (span, error) {
		if start <= end {
			return span{start, end}, nil
		}
		return span{}, raoh.Invalid(raoh.NewIssue("start_after_end").
			WithMeta("start", start).WithMeta("end", end).
			At(at.Key("end")))
	})
}

func TestObjectAndThenWithPathKeepsThePathsItReports(t *testing.T) {
	d := raoh.Object(raoh.Fields().Field("period", spanDecoder())).Map(func(p span) span { return p })

	_, issues := decodeJSON(t, d, `{"period": {"start": 3, "end": 2}}`)
	if got, want := codes(issues), []string{"/period/end start_after_end"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if v, issues := decodeJSON(t, d, `{"period": {"start": 1, "end": 2}}`); issues != nil || v != (span{1, 2}) {
		t.Errorf("got %v %v", v, issues)
	}
	// Parts that fail keep fn from running.
	_, issues = decodeJSON(t, d, `{"period": {"start": "a", "end": 2}}`)
	if got, want := codes(issues), []string{"/period/start type_mismatch"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAndThenMovesRelativeIssuesAndAndThenWithPathDoesNot(t *testing.T) {
	relative := raoh.Object(raoh.Fields().Field("n", raoh.Int())).
		AndThen(func(int) (int, error) {
			return 0, raoh.Invalid(raoh.NewIssue("bad").At(raoh.Path{}.Key("end")))
		})
	absolute := raoh.Int().AndThenWithPath(func(int, raoh.Path) (int, error) {
		return 0, raoh.Invalid(raoh.NewIssue("bad").At(raoh.Path{}.Key("end")))
	})
	_, issues := decodeJSON(t, raoh.Object(raoh.Fields().Field("a", relative)).Map(func(int) int { return 0 }), `{"a": {"n": 1}}`)
	if got, want := codes(issues), []string{"/a/end bad"}; !slices.Equal(got, want) {
		t.Errorf("AndThen: got %v, want %v", got, want)
	}
	_, issues = decodeJSON(t, raoh.Object(raoh.Fields().Field("a", absolute)).Map(func(int) int { return 0 }), `{"a": 1}`)
	if got, want := codes(issues), []string{"/end bad"}; !slices.Equal(got, want) {
		t.Errorf("AndThenWithPath: got %v, want %v", got, want)
	}
}

func TestPathAwareFunctionsSeeThePathTheDecoderIsAt(t *testing.T) {
	var seen []string
	record := func(at raoh.Path) { seen = append(seen, at.String()) }
	d := raoh.Object(raoh.Fields().
		Field("a", raoh.Int().AndThenWithPath(func(v int, at raoh.Path) (int, error) { record(at); return v, nil })).
		Field("b", raoh.Int().RefineWithPath(func(_ int, at raoh.Path) error { record(at); return nil })).
		Field("c", raoh.NewDecoderWithPath(func(in any, at raoh.Path) (any, error) { record(at); return in, nil })),
	).Map(func(a, b int, c any) int { return a + b })
	if _, issues := decodeJSON(t, d, `{"a": 1, "b": 2, "c": 3}`); issues != nil {
		t.Fatal(issues)
	}
	if want := []string{"/a", "/b", "/c"}; !slices.Equal(seen, want) {
		t.Errorf("got %v, want %v", seen, want)
	}
}

func TestNewDecoderWithPathAndRefineWithPathKeepAbsolutePaths(t *testing.T) {
	newD := raoh.NewDecoderWithPath(func(in any, at raoh.Path) (int, error) {
		return 0, raoh.Invalid(raoh.NewIssue("bad").At(at.Key("x")))
	})
	refined := raoh.Int().RefineWithPath(func(n int, at raoh.Path) error {
		if n > 0 {
			return nil
		}
		return raoh.Invalid(raoh.NewIssue("not_positive").At(at), raoh.NewIssue("other").At(at.Key("y")))
	})
	d := raoh.Object(raoh.Fields().Field("p", newD).Field("q", refined)).Map(func(a, b int) int { return a + b })
	_, issues := decodeJSON(t, d, `{"p": 1, "q": 0}`)
	if got, want := codes(issues), []string{"/p/x bad", "/q not_positive", "/q/y other"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPathAwareFunctionsTreatOtherErrorsAsFailuresOfTheProgram(t *testing.T) {
	boom := errors.New("boom")
	mixed := errors.Join(boom, raoh.Invalid(raoh.NewIssue("x")))
	for name, d := range map[string]raoh.Decoder[any, int]{
		"AndThenWithPath":    raoh.Int().AndThenWithPath(func(v int, _ raoh.Path) (int, error) { return v, boom }),
		"RefineWithPath":     raoh.Int().RefineWithPath(func(int, raoh.Path) error { return boom }),
		"NewDecoderWithPath": raoh.NewDecoderWithPath(func(any, raoh.Path) (int, error) { return 0, boom }),
		"mixed":              raoh.Int().AndThenWithPath(func(v int, _ raoh.Path) (int, error) { return v, mixed }),
	} {
		_, err := d.Decode(1)
		if !errors.Is(err, boom) {
			t.Errorf("%s: got %v", name, err)
		}
		if _, ok := errors.AsType[*raoh.Issues](err); ok {
			t.Errorf("%s: a failure of the program was taken for invalid input", name)
		}
	}
}

func TestRefineWithMetaComputesMetaOnlyOnFailureAndCopiesIt(t *testing.T) {
	calls := 0
	meta := map[string]any{"actual": 3}
	d := raoh.Int().RefineWithMeta(func(n int) bool { return n%2 == 0 }, "must_be_even", "must be even",
		func(n int) map[string]any { calls++; return meta })
	if _, err := d.Decode(4); err != nil || calls != 0 {
		t.Fatalf("err %v, calls %d", err, calls)
	}
	_, err := d.Decode(3)
	meta["actual"] = 99
	issues, _ := errors.AsType[*raoh.Issues](err)
	if calls != 1 || issues == nil {
		t.Fatalf("calls %d, err %v", calls, err)
	}
	got := issues.All()[0].Meta()
	if got["actual"] != 3 {
		t.Errorf("meta %v was changed after the decode", got)
	}
}

func TestDefaultFuncAndFallbackFuncRunOnlyWhenNeeded(t *testing.T) {
	defaults, fallbacks := 0, 0
	dd := raoh.Int().DefaultFunc(func() int { defaults++; return 7 })
	if v, err := dd.Decode(1); err != nil || v != 1 || defaults != 0 {
		t.Errorf("valid: %v %v %d", v, err, defaults)
	}
	if v, err := dd.Decode("x"); err == nil || defaults != 0 {
		t.Errorf("mismatch: %v %v %d", v, err, defaults)
	}
	if v, err := dd.Decode(nil); err != nil || v != 7 || defaults != 1 {
		t.Errorf("missing: %v %v %d", v, err, defaults)
	}

	fd := raoh.Object(raoh.Fields().Field("n", raoh.Int().FallbackFunc(func(is raoh.Issues) int {
		fallbacks++
		if is.Len() != 1 || is.All()[0].Path().String() != "/n" {
			t.Errorf("issues %v are not at their absolute paths", codes(is.All()))
		}
		return -1
	}))).Map(func(n int) int { return n })
	if v, issues := decodeJSON(t, fd, `{"n": 5}`); issues != nil || v != 5 || fallbacks != 0 {
		t.Errorf("valid: %v %v %d", v, issues, fallbacks)
	}
	if v, issues := decodeJSON(t, fd, `{"n": "x"}`); issues != nil || v != -1 || fallbacks != 1 {
		t.Errorf("invalid: %v %v %d", v, issues, fallbacks)
	}

	boom := errors.New("boom")
	failing := raoh.NewDecoder(func(any) (int, error) { return 0, boom }).
		FallbackFunc(func(raoh.Issues) int { fallbacks++; return 0 })
	if _, err := failing.Decode(1); !errors.Is(err, boom) || fallbacks != 1 {
		t.Errorf("a failure of the program was recovered: %v %d", err, fallbacks)
	}
}

func TestPathAwareFormsAddNoAllocationOnSuccess(t *testing.T) {
	plain := raoh.Int().AndThen(func(n int) (int, error) { return n, nil })
	aware := raoh.Int().AndThenWithPath(func(n int, _ raoh.Path) (int, error) { return n, nil })
	po := raoh.Object(raoh.Fields().Field("a", raoh.Int()).Field("b", raoh.Int())).
		AndThen(func(a, b int) (int, error) { return a + b, nil })
	pa := raoh.Object(raoh.Fields().Field("a", raoh.Int()).Field("b", raoh.Int())).
		AndThenWithPath(func(a, b int, _ raoh.Path) (int, error) { return a + b, nil })
	in := map[string]any{"a": 1, "b": 2}

	for name, pair := range map[string][2]func(){
		"Decoder": {func() { plain.Decode(1) }, func() { aware.Decode(1) }},
		"object":  {func() { po.Decode(in) }, func() { pa.Decode(in) }},
	} {
		base := testing.AllocsPerRun(100, pair[0])
		got := testing.AllocsPerRun(100, pair[1])
		if got > base {
			t.Errorf("%s: %v allocations with the path, %v without", name, got, base)
		}
	}
}
