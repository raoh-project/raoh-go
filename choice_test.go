package raoh_test

import (
	"testing"

	"github.com/raoh-project/raoh-go"
)

var lowerTag = raoh.String().Trim().ToLower()

func TestEnumOfWithReadsTheStringWithTheDecoderGiven(t *testing.T) {
	e := raoh.EnumOfWith(map[string]int{"red": 1}, raoh.String().Trim())
	if v, _ := decodeJSON(t, e, `" RED "`); v != 1 {
		t.Error(v)
	}
	expect(t, e, `"blue"`, " invalid_format")
	expect(t, e, `1`, " type_mismatch")

	short := raoh.EnumOfWith(map[string]int{"red": 1}, raoh.String().MinLength(5).Message("too short"))
	_, issues := decodeJSON(t, short, `"red"`)
	if len(issues) != 1 || issues[0].Code() != raoh.CodeTooShort || issues[0].Message(raoh.English) != "too short" {
		t.Error(codes(issues))
	}
}

func TestLiteralWithComparesWhatTheDecoderReturns(t *testing.T) {
	l := raoh.LiteralWith("v1", lowerTag)
	if v, issues := decodeJSON(t, l, `" V1 "`); v != "v1" || len(issues) != 0 {
		t.Error(v, codes(issues))
	}
	expect(t, l, `"v2"`, " invalid_format")
	expect(t, l, `null`, " required")
}

func shapeVariants() map[string]raoh.DecoderOf[shape] {
	return map[string]raoh.DecoderOf[shape]{
		"square": raoh.Object(raoh.Fields().Field("side", raoh.Int())).
			Map(func(s int) shape { return shape{"square", s * s} }),
		"rect": raoh.Object(raoh.Fields().Field("w", raoh.Int()).Field("h", raoh.Int())).
			Map(func(w, h int) shape { return shape{"rect", w * h} }),
	}
}

func TestDiscriminateWithReadsTheTagWithTheDecoderGiven(t *testing.T) {
	d := raoh.DiscriminateWith("kind", lowerTag, shapeVariants())
	if v, issues := decodeJSON(t, d, `{"kind": " RECT ", "w": 2, "h": 3}`); v.area != 6 || len(issues) != 0 {
		t.Error(v, codes(issues))
	}
	_, issues := decodeJSON(t, d, `{"kind": "circle"}`)
	if len(issues) != 1 || codes(issues)[0] != "/kind not_allowed" || issues[0].Message(raoh.English) != "must be one of [rect, square]" {
		t.Error(codes(issues))
	}
	expect(t, d, `{}`, "/kind required")
	expect(t, d, `{"kind": 1}`, "/kind type_mismatch")
	expect(t, d, `"rect"`, " type_mismatch")

	long := raoh.DiscriminateWith("kind", raoh.String().MinLength(9), shapeVariants())
	expect(t, long, `{"kind": "rect"}`, "/kind too_short")
}

func TestDiscriminateWithDoesNotChangeTheKeysOfTheVariants(t *testing.T) {
	v := map[string]raoh.DecoderOf[shape]{"Square": shapeVariants()["square"]}
	expect(t, raoh.DiscriminateWith("kind", lowerTag, v), `{"kind": "Square", "side": 1}`, "/kind not_allowed")
}

func TestDiscriminateWithCopiesTheMap(t *testing.T) {
	v := shapeVariants()
	d := raoh.DiscriminateWith("kind", raoh.String(), v)
	delete(v, "rect")
	v["circle"] = v["square"]
	if r, _ := decodeJSON(t, d, `{"kind": "rect", "w": 2, "h": 3}`); r.area != 6 {
		t.Error(r)
	}
	expect(t, d, `{"kind": "circle", "side": 1}`, "/kind not_allowed")
}

func TestStrictReportsMembersTheFieldsDoNotNameAfterTheIssuesOfTheDecoder(t *testing.T) {
	d := raoh.Strict(raoh.Object(raoh.Fields().Field("side", raoh.Int())).
		Map(func(s int) int { return s }), "kind", "side", "side")
	if v, issues := decodeJSON(t, d, `{"kind": "square", "side": 3}`); v != 3 || len(issues) != 0 {
		t.Error(v, codes(issues))
	}
	expect(t, d, `{"kind": "square", "side": 3, "w": 1}`, "/w unknown_field")
	expect(t, d, `{"z": 1, "side": "x", "a": 1}`, "/side type_mismatch", "/z unknown_field", "/a unknown_field")
	expect(t, d, `{"side": 1, "z": 1, "a": 1}`, "/z unknown_field", "/a unknown_field")
}

func TestStrictAddsNothingToAnInputThatIsNotAnObject(t *testing.T) {
	d := raoh.Strict(raoh.Int())
	if v, issues := decodeJSON(t, d, `1`); v != 1 || len(issues) != 0 {
		t.Error(v, codes(issues))
	}
	expect(t, d, `"a"`, " type_mismatch")
	expect(t, raoh.Strict(raoh.Object(raoh.Fields().Field("a", raoh.Int())).Map(func(a int) int { return a })),
		`[1]`, " type_mismatch")
}

func TestStrictOfANativeMapKeepsTheOrderOfGoStrings(t *testing.T) {
	d := raoh.Strict(raoh.Object(raoh.Fields().Field("a", raoh.Int())).Map(func(a int) int { return a }), "a")
	_, issues := decodeJSONValue(t, d, map[string]any{"a": 1.0, "z": 1.0, "b": 1.0})
	if got := codes(issues); len(got) != 2 || got[0] != "/b unknown_field" || got[1] != "/z unknown_field" {
		t.Error(got)
	}
}

func TestStrictAtANestedPathReportsBelowIt(t *testing.T) {
	inner := raoh.Strict(raoh.Object(raoh.Fields().Field("a", raoh.Int())).Map(func(a int) int { return a }), "a")
	d := raoh.Object(raoh.Fields().Field("p", inner)).Map(func(a int) int { return a })
	expect(t, d, `{"p": {"a": 1, "x": 2}}`, "/p/x unknown_field")
}

func TestStrictDoesNotTurnAnErrorIntoAnInvalidInput(t *testing.T) {
	boom := errBoom{}
	d := raoh.Strict(raoh.NewDecoder(func(any) (int, error) { return 0, boom }))
	_, err := d.Decode(map[string]any{"x": 1})
	if err != boom {
		t.Error(err)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
