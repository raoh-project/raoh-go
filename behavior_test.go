package raoh_test

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
)

func decodeJSON[T any](t *testing.T, d raoh.DecoderOf[T], text string) (T, []raoh.Issue) {
	t.Helper()
	v, err := raoh.DecodeJSON([]byte(text), d)
	if err == nil {
		return v, nil
	}
	issues, ok := errors.AsType[*raoh.Issues](err)
	if !ok {
		t.Fatalf("%s: %v", text, err)
	}
	return v, issues.All()
}

// codes writes each issue as path and code.
func codes(issues []raoh.Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Path().String()+" "+i.Code())
	}
	return out
}

func expect[T any](t *testing.T, d raoh.DecoderOf[T], text string, want ...string) {
	t.Helper()
	_, issues := decodeJSON(t, d, text)
	if got := codes(issues); !slices.Equal(got, want) {
		t.Errorf("%s: %v, want %v", text, got, want)
	}
}

func TestMissingAndNullAreRequiredAndOtherTypesMismatch(t *testing.T) {
	name := raoh.Object(raoh.Fields().Field("name", raoh.String())).Map(func(s string) string { return s })
	expect(t, name, `{}`, "/name required")
	expect(t, name, `{"name": null}`, "/name required")
	expect(t, name, `{"name": 1}`, "/name type_mismatch")
	expect(t, name, `[]`, " type_mismatch")
}

func TestTheFirstFailingConstraintIsReported(t *testing.T) {
	_, issues := decodeJSON(t, raoh.String().MinLength(5).MaxLength(1), `"abc"`)
	if len(issues) != 1 || issues[0].Code() != raoh.CodeTooShort {
		t.Fatal(codes(issues))
	}
}

func TestAMessageGoesToTheLatestConstraintPastTransformations(t *testing.T) {
	_, issues := decodeJSON(t, raoh.String().MinLength(3).Trim().Message("too short a code"), `"ab"`)
	if issues[0].Message(raoh.Japanese) != "too short a code" {
		t.Fatal(issues[0])
	}
	_, issues = decodeJSON(t, raoh.String().Trim().Message("give a name"), `null`)
	if issues[0].Message(raoh.English) != "give a name" {
		t.Fatal(issues[0])
	}
}

func TestFormatIssuesNameTheCheckInTheirKey(t *testing.T) {
	_, issues := decodeJSON(t, raoh.String().Email(), `"x"`)
	if issues[0].Code() != raoh.CodeInvalidFormat || issues[0].MessageKey() != raoh.KeyInvalidFormatEmail {
		t.Fatal(issues[0])
	}
}

func TestANumberThatIsNotAnIntegerNamesWhatItIs(t *testing.T) {
	for text, key := range map[string]string{
		`1.5`: raoh.CodeTypeMismatch, `1e2`: raoh.CodeTypeMismatch, `1.0`: raoh.CodeTypeMismatch,
		`3000000000`: raoh.KeyTypeMismatchNumericRange,
	} {
		_, issues := decodeJSON(t, raoh.Int32(), text)
		if len(issues) != 1 || issues[0].MessageKey() != key {
			t.Errorf("%s: %v", text, issues)
		}
	}
	if _, issues := decodeJSON(t, raoh.Uint32(), `-1`); len(issues) != 1 ||
		issues[0].MessageKey() != raoh.KeyTypeMismatchNumericRange {
		t.Errorf("-1 into uint32: %v", issues)
	}
	if v, issues := decodeJSON(t, raoh.Int64(), `-0`); v != 0 || issues != nil {
		t.Errorf("-0: %v %v", v, issues)
	}
	if v, issues := decodeJSON(t, raoh.Uint64(), `18446744073709551615`); v != 18446744073709551615 || issues != nil {
		t.Errorf("max uint64: %v %v", v, issues)
	}
}

func TestNativeGoValuesAreDecodedToo(t *testing.T) {
	type pair struct {
		n int
		s string
	}
	d := raoh.Object(raoh.Fields().Field("n", raoh.Int()).Field("s", raoh.String())).
		Map(func(n int, s string) pair { return pair{n, s} })
	got, err := d.Decode(map[string]any{"n": float64(3), "s": "x"})
	if err != nil || got != (pair{3, "x"}) {
		t.Fatalf("%v %v", got, err)
	}
	_, err = d.Decode(map[string]any{"n": 3.5, "s": "x"})
	if issues, ok := errors.AsType[*raoh.Issues](err); !ok || issues.All()[0].Code() != raoh.CodeTypeMismatch {
		t.Fatal(err)
	}
}

func TestStrictReportsEveryUnknownMemberAfterTheFieldsIssuesInInputOrder(t *testing.T) {
	point := raoh.Object(raoh.Fields().Field("x", raoh.Int()).Field("y", raoh.Int())).Strict().
		Map(func(x, y int) [2]int { return [2]int{x, y} })
	expect(t, point, `{"z": 3, "x": "1", "b": 1, "y": 2}`, "/x type_mismatch", "/z unknown_field", "/b unknown_field")
}

func TestOptionalAndPresenceFields(t *testing.T) {
	type out struct {
		nick *string
		note raoh.Presence[string]
	}
	d := raoh.Object(raoh.Fields().
		Field("nick", raoh.Optional(raoh.String())).
		Field("note", raoh.PresenceOf(raoh.String())),
	).Map(func(n *string, p raoh.Presence[string]) out { return out{n, p} })
	v, _ := decodeJSON(t, d, `{}`)
	if v.nick != nil || !v.note.IsAbsent() {
		t.Error("absent")
	}
	v, _ = decodeJSON(t, d, `{"note": null}`)
	if !v.note.IsNull() || !v.note.IsGiven() {
		t.Error("null")
	}
	v, _ = decodeJSON(t, d, `{"nick": "k", "note": "n"}`)
	if s, ok := v.note.Value(); *v.nick != "k" || !ok || s != "n" {
		t.Error("present")
	}
	expect(t, d, `{"nick": null}`, "/nick required")
}

func TestNullableAcceptsNullButNotMissing(t *testing.T) {
	d := raoh.Object(raoh.Fields().Field("note", raoh.Nullable(raoh.String()))).Map(func(s *string) *string { return s })
	if v, issues := decodeJSON(t, d, `{"note": null}`); v != nil || issues != nil {
		t.Error("null")
	}
	expect(t, d, `{}`, "/note required")
}

func TestListsReportEveryElementUnderItsIndex(t *testing.T) {
	expect(t, raoh.List(raoh.Int()), `[1, "a", 3, true]`, "/1 type_mismatch", "/3 type_mismatch")
	_, issues := decodeJSON(t, raoh.List(raoh.Int()).Unique(), `[1, 2, 1, 2, 1]`)
	if issues[0].Message(raoh.English) != "must not contain duplicates: [1, 2]" {
		t.Error(issues[0])
	}
	_, issues = decodeJSON(t, raoh.List(raoh.Int()).NonEmpty(), `[]`)
	if issues[0].MessageKey() != raoh.KeyTooSmallNonEmpty {
		t.Error(issues[0])
	}
}

func TestDictKeepsEveryMember(t *testing.T) {
	v, _ := decodeJSON(t, raoh.Dict(raoh.Int()), `{"b": 1, "a": 2}`)
	if v["a"] != 2 || v["b"] != 1 {
		t.Error(v)
	}
	expect(t, raoh.Dict(raoh.Int()), `{"b": "x", "a": "y"}`, "/b type_mismatch", "/a type_mismatch")
}

type shape struct {
	kind string
	area int
}

func shapeDecoder() raoh.Decoder[any, shape] {
	return raoh.Discriminate("kind",
		raoh.Variant("square", raoh.Object(raoh.Fields().Field("side", raoh.Int())).
			Map(func(s int) shape { return shape{"square", s * s} })),
		raoh.Variant("rect", raoh.Object(raoh.Fields().Field("w", raoh.Int()).Field("h", raoh.Int())).
			Map(func(w, h int) shape { return shape{"rect", w * h} })),
	)
}

func TestTheTagPicksTheVariant(t *testing.T) {
	if v, _ := decodeJSON(t, shapeDecoder(), `{"kind": "rect", "w": 2, "h": 3}`); v.area != 6 {
		t.Error(v)
	}
	_, issues := decodeJSON(t, shapeDecoder(), `{"kind": "circle"}`)
	if codes(issues)[0] != "/kind not_allowed" || issues[0].Message(raoh.English) != "must be one of [rect, square]" {
		t.Error(issues)
	}
	expect(t, shapeDecoder(), `{}`, "/kind required")
	expect(t, shapeDecoder(), `"rect"`, " type_mismatch")
}

func panics(f func()) (ok bool) {
	defer func() { ok = recover() != nil }()
	f()
	return false
}

func TestDuplicateTagsAndFoldedEnumNamesPanic(t *testing.T) {
	d := raoh.Int()
	if !panics(func() { raoh.Discriminate("k", raoh.Variant("a", d), raoh.Variant("a", d)) }) {
		t.Error("tags")
	}
	if !panics(func() { raoh.EnumOf(map[string]int{"Red": 1, "RED": 2}) }) {
		t.Error("enum")
	}
}

func TestEnumNamesFoldASCIICaseOnly(t *testing.T) {
	e := raoh.EnumOf(map[string]int{"straße": 1})
	if v, _ := decodeJSON(t, e, `"STRAßE"`); v != 1 {
		t.Error(v)
	}
	expect(t, e, `"STRASSE"`, " invalid_format")
	expect(t, raoh.Literal("v1"), `"V1"`, " invalid_format")
}

func TestAndThenRunsOnlyOnceThePartsDecodeAndMovesItsIssues(t *testing.T) {
	calls := 0
	period := raoh.Object(raoh.Fields().Field("start", raoh.Int()).Field("end", raoh.Int())).
		AndThen(func(s, e int) ([2]int, error) {
			calls++
			if s > e {
				return [2]int{}, raoh.Invalid(raoh.NewIssue("invalid_value").At(raoh.Path{}.Key("end")))
			}
			return [2]int{s, e}, nil
		})
	trip := raoh.Object(raoh.Fields().Field("period", period)).Map(func(p [2]int) [2]int { return p })
	expect(t, trip, `{"period": {"start": "a", "end": 1}}`, "/period/start type_mismatch")
	if calls != 0 {
		t.Error("ran before the parts decoded")
	}
	expect(t, trip, `{"period": {"start": 5, "end": 1}}`, "/period/end invalid_value")
}

func TestPipeHandsTheOutputOnAtTheSamePath(t *testing.T) {
	positive := raoh.NewDecoder(func(n int) (int, error) {
		if n <= 0 {
			return 0, raoh.Invalid(raoh.NewIssue("not_positive"))
		}
		return n, nil
	})
	d := raoh.Object(raoh.Fields().Field("n", raoh.Int().Pipe(positive))).Map(func(n int) int { return n })
	expect(t, d, `{"n": 0}`, "/n not_positive")
}

func TestDefaultCoversOnlyMissingOrNullAndFallbackEverything(t *testing.T) {
	if v, _ := decodeJSON(t, raoh.Int().Default(7), `null`); v != 7 {
		t.Error(v)
	}
	expect(t, raoh.Int().Default(7), `"x"`, " type_mismatch")
	if v, _ := decodeJSON(t, raoh.Int().Fallback(7), `"x"`); v != 7 {
		t.Error(v)
	}
}

func TestRefineReportsItsMessageAsACustomOne(t *testing.T) {
	even := raoh.Int().Refine(func(n int) bool { return n%2 == 0 }, "odd", "must be even")
	_, issues := decodeJSON(t, even, `3`)
	if issues[0].Code() != "odd" || issues[0].Message(raoh.Japanese) != "must be even" {
		t.Error(issues[0])
	}
}

func TestOneOfListsEachCandidatesIssues(t *testing.T) {
	d := raoh.OneOf[string](raoh.Int().Map(func(n int) string { return "n" }), raoh.String().MinLength(3))
	_, issues := decodeJSON(t, d, `"ab"`)
	candidates := issues[0].Meta()["candidates"].([]any)
	if issues[0].Code() != raoh.CodeOneOfFailed || len(candidates) != 2 {
		t.Fatal(issues[0])
	}
}

type category struct {
	name     string
	children []category
}

func categoryDecoder() raoh.Decoder[any, category] {
	return raoh.Object(raoh.Fields().
		Field("name", raoh.String().NonBlank()).
		Field("children", raoh.List(raoh.Lazy(categoryDecoder))),
	).Map(func(n string, c []category) category { return category{n, c} })
}

func TestLazyDecodesRecursiveStructures(t *testing.T) {
	v, issues := decodeJSON(t, categoryDecoder(), `{"name": "a", "children": [{"name": "b", "children": []}]}`)
	if issues != nil || v.children[0].name != "b" {
		t.Fatal(v, issues)
	}
	expect(t, categoryDecoder(), `{"name": "a", "children": [{"name": " ", "children": []}]}`, "/children/0/name blank")
}

func TestADecoderIsSharedAcrossGoroutines(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, issues := decodeJSON(t, categoryDecoder(), `{"name": "a", "children": []}`); issues != nil {
				t.Error(issues)
			}
		})
	}
	wg.Wait()
}

func TestTextThatIsNotJSONIsOneIssue(t *testing.T) {
	_, issues := decodeJSON(t, raoh.Int(), "{\n  \"a\": }")
	i := issues[0]
	if len(issues) != 1 || i.MessageKey() != raoh.KeyInvalidFormatJSON || i.Meta()["line"] != 2 {
		t.Fatal(i, i.Meta())
	}
	if i.Message(raoh.English) != "not valid JSON" {
		t.Error(i.Message(raoh.English))
	}
	expect(t, raoh.Int(), `1 2`, " invalid_format")
}

func TestIssuesRenderAndFlatten(t *testing.T) {
	_, err := raoh.DecodeJSON([]byte(`"ab"`), raoh.String().MinLength(3))
	issues, _ := errors.AsType[*raoh.Issues](err)
	r := issues.Render(raoh.Japanese)[0]
	if r.Path != "" || r.Code != "too_short" || r.Message != "3文字以上で入力してください" || r.Meta["min"] != 3 {
		t.Error(r)
	}
	if f := issues.Flatten(raoh.English); f[""][0] != "must be at least 3 characters" {
		t.Error(f)
	}
	if err.Error() != "(root): must be at least 3 characters" {
		t.Error(err.Error())
	}
}

func TestMessageCataloguesAreLayered(t *testing.T) {
	positive := raoh.NewIssue(raoh.CodeOutOfRange).WithMessageKey(raoh.KeyOutOfRangePositive).WithMeta("min", 1)
	if got := positive.Message(raoh.Japanese); got != "正の値で入力してください" {
		t.Error(got)
	}
	email := raoh.NewIssue(raoh.CodeInvalidFormat).WithMessageKey(raoh.KeyInvalidFormatEmail)
	french, _ := raoh.ParseProperties("raoh.invalid_format=format invalide")
	if got := email.Message(french.FallingBackTo(raoh.English)); got != "format invalide" {
		t.Error(got)
	}
	partial := raoh.English.WithOverrides(map[string]string{"too_short": "{least}+ characters"})
	if got := raoh.NewIssue("too_short").WithMeta("min", 3).Message(partial); got != "must be at least 3 characters" {
		t.Error(got)
	}
	if got := raoh.NewIssue("mine").Message(raoh.English); got != "validation failed: mine" {
		t.Error(got)
	}
	upper := raoh.ResolverFunc(func(i raoh.Issue) string { return strings.ToUpper(i.Code()) })
	if got := raoh.NewIssue("blank").Message(upper); got != "BLANK" {
		t.Error(got)
	}
	big := raoh.NewIssue(raoh.CodeOutOfRange).WithMessageKey(raoh.KeyOutOfRangeMinimum).WithMeta("min", 1e7)
	if got := big.Message(raoh.English); got != "must be at least 1.0E7" {
		t.Error(got)
	}
}

func TestTheCataloguesCoverEveryCodeAndMessageKey(t *testing.T) {
	keys := []string{raoh.CodeRequired, raoh.CodeBlank, raoh.CodeTooShort, raoh.CodeTooLong, raoh.CodeInvalidLength,
		raoh.CodeOutOfRange, raoh.CodeNotMultipleOf, raoh.CodeInvalidScale, raoh.CodeTooSmall, raoh.CodeTooBig,
		raoh.CodeInvalidSize, raoh.CodeInvalidValue, raoh.CodeInvalidFormat, raoh.CodeTypeMismatch,
		raoh.CodeUnknownField, raoh.CodeMissingElement, raoh.CodeMissingElements, raoh.CodeDuplicateElement,
		raoh.CodeNotAllowed, raoh.CodeOneOfFailed, raoh.CodeMissingField,
		raoh.KeyOutOfRangeMinimum, raoh.KeyOutOfRangeMaximum, raoh.KeyOutOfRangeRange, raoh.KeyOutOfRangePositive,
		raoh.KeyOutOfRangeNegative, raoh.KeyOutOfRangeNonNegative, raoh.KeyOutOfRangeNonPositive,
		raoh.KeyTypeMismatchNumericRange, raoh.KeyTooSmallNonEmpty, raoh.KeyInvalidFormatEmail,
		raoh.KeyInvalidFormatURL, raoh.KeyInvalidFormatUUID, raoh.KeyInvalidFormatIP, raoh.KeyInvalidFormatIPv4,
		raoh.KeyInvalidFormatIPv6, raoh.KeyInvalidFormatULID, raoh.KeyInvalidFormatCUID,
		raoh.KeyInvalidFormatStartsWith, raoh.KeyInvalidFormatEndsWith, raoh.KeyInvalidFormatIncludes,
		raoh.KeyInvalidFormatEnum, raoh.KeyInvalidFormatLiteral, raoh.KeyInvalidFormatJSON}
	japaneseOnly, _ := raoh.Japanese.Template(raoh.CodeRequired)
	english, _ := raoh.English.Template(raoh.CodeRequired)
	if japaneseOnly == english {
		t.Fatal("the Japanese catalogue gives English")
	}
	for _, k := range keys {
		for name, m := range map[string]*raoh.Messages{"English": raoh.English, "Japanese": raoh.Japanese} {
			if _, ok := m.Template(k); !ok {
				t.Errorf("no %s template for %s", name, k)
			}
		}
	}
}

func TestTemporalDecoders(t *testing.T) {
	v, _ := decodeJSON(t, raoh.String().OffsetDateTime(), `"2024-01-15T10:30+09:00"`)
	if _, offset := v.Zone(); offset != 9*3600 || v.Hour() != 10 {
		t.Errorf("offset kept: %v", v)
	}
	v, _ = decodeJSON(t, raoh.String().Instant(), `"2024-01-15T10:30:00+09:00"`)
	if v.Location() != time.UTC || v.Hour() != 1 {
		t.Errorf("instant in UTC: %v", v)
	}
	// A date bound is compared by its date alone, whatever its location.
	tokyo := time.FixedZone("JST", 9*3600)
	d := raoh.String().Date().Before(time.Date(2024, 1, 1, 8, 0, 0, 0, tokyo))
	expect(t, d, `"2024-01-01"`, " out_of_range")
	_, issues := decodeJSON(t, d, `"2024-01-01"`)
	if got := issues[0].Message(raoh.Japanese); got != "2024-01-01より前で入力してください" {
		t.Error(got)
	}
	_, issues = decodeJSON(t, raoh.String().Trim().Date().Message("give a date"), `"2024-13-01"`)
	if issues[0].Message(raoh.English) != "give a date" || issues[0].MessageKey() != raoh.KeyInvalidFormatDate {
		t.Error(issues[0])
	}
	_, issues = decodeJSON(t, raoh.String().Date().After(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Message("too early"), `"2023-01-01"`)
	if issues[0].Message(raoh.English) != "too early" {
		t.Error(issues[0])
	}
	if !panics(func() {
		raoh.String().Time().Between(time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC), time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC))
	}) {
		t.Error("Between with from after to")
	}
}

func TestStringConversionsContinueAsTheTypeTheyRead(t *testing.T) {
	v, issues := decodeJSON(t, raoh.String().ToInt().Min(1), `"+007"`)
	if len(issues) > 0 || v != 7 {
		t.Errorf("got %v %v", v, codes(issues))
	}
	expect(t, raoh.String().ToInt().Min(1), `"0"`, " out_of_range")
	expect(t, raoh.String().ToInt(), `"2147483648"`, " type_mismatch")
	expect(t, raoh.String().ToLong(), `"2147483648"`)
	expect(t, raoh.String().ToBool().IsTrue(), `"NO"`, " invalid_value")
	expect(t, raoh.String().ToDecimal().Scale(1), `"1.25"`, " invalid_scale")
	expect(t, raoh.String().ToInt(), `1`, " type_mismatch")
	expect(t, raoh.String().ToInt(), `null`, " required")
}

func TestStringConversionMessageIsForTheConversionOnly(t *testing.T) {
	d := raoh.String().MaxLength(3).ToInt().Message("bad number")
	message := func(text string) string {
		_, issues := decodeJSON(t, d, text)
		if len(issues) != 1 {
			t.Fatalf("%s: %v", text, codes(issues))
		}
		return issues[0].Message(raoh.English)
	}
	if got := message(`"abc"`); got != "bad number" {
		t.Errorf("conversion: %q", got)
	}
	if got := message(`"99999"`); got == "bad number" {
		t.Errorf("a string constraint takes the message of the conversion: %q", got)
	}
	if got := message(`1`); got == "bad number" {
		t.Errorf("the type check of the string takes the message of the conversion: %q", got)
	}
}

func TestURIAcceptsAnySchemeAndKeepsSchemeRequired(t *testing.T) {
	for _, ok := range []string{"HTTP://EXAMPLE.COM", "http://host#", "mailto:ken@example.com", "urn:isbn:0451450523"} {
		expect(t, raoh.String().URI(), `"`+ok+`"`)
	}
	for _, bad := range []string{"foo/bar", "#top", "a:", "http://[v1.abc]/"} {
		expect(t, raoh.String().URI(), `"`+bad+`"`, " invalid_format")
	}
}
