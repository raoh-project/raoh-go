package raoh_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
)

// The text rules are those of 199x-notation, which Raoh for Java follows too: Unicode 18.0.0,
// whatever Go release the package is built with.
func TestTheTextRulesAreUnicode18(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    raoh.StringDecoder
		in   string
		want string
	}{
		{"a final sigma ends a cased run", raoh.String().ToLower(), "ΑΣ ΒΣ", "ας βς"},
		{"a sigma before a digit is not final", raoh.String().ToLower(), "Α1Σ", "α1σ"},
		{"a sigma alone is not final", raoh.String().ToLower(), "Σ", "σ"},
		{"İ lowercases to i and a dot", raoh.String().ToLower(), "İ", "i̇"},
		{"ß uppercases to SS", raoh.String().ToUpper(), "straße", "STRASSE"},
		{"a ligature uppercases to its letters", raoh.String().ToUpper(), "ﬁ", "FI"},
		{"a Unicode 18.0.0 case pair", raoh.String().ToUpper(), "ꭋ", "꭬"},
		{"U+1E6E3 has a combining class from Unicode 18.0.0", raoh.String().NormalizeAs(raoh.NFD), "a\U0001E6E3\u0327", "a\u0327\U0001E6E3"},
		{"trim takes White_Space and leaves NUL", raoh.String().Trim(), "　  a\x00  ", "a\x00"},
	} {
		got, err := tc.d.Decode(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("%s: %+q gives %+q, %v; want %+q", tc.name, tc.in, got, err, tc.want)
		}
	}
	if _, err := raoh.String().NonBlank().Decode("\u0085　"); err == nil {
		t.Error("U+0085 and U+3000 are not blank")
	}
	if _, err := raoh.String().NonBlank().Decode("\x1c"); err != nil {
		t.Error("U+001C, an information separator, is not white space")
	}
}

// A Go string holding bytes that are not UTF-8 is no string of the input model, and is refused as
// any other value that is not a string.
func TestAStringThatIsNotUTF8IsNoString(t *testing.T) {
	for _, in := range []string{"\xff", "a\xed\xa0\x80", "é\xc3"} {
		_, err := raoh.String().Decode(in)
		if issues, ok := errors.AsType[*raoh.Issues](err); !ok || issues.All()[0].Code() != raoh.CodeTypeMismatch {
			t.Errorf("%+q gives %v", in, err)
		}
	}
	if got, err := raoh.String().Decode("�"); err != nil || got != "�" {
		t.Errorf("U+FFFD written as itself is a character: %+q, %v", got, err)
	}
}

// A pattern is one of the Raoh pattern language, and what is not one, or is past a limit, panics
// when the decoder is built, with a message that tells the two apart.
func TestAPatternIsOneOfTheRaohLanguage(t *testing.T) {
	d := raoh.String().Pattern(`[a-z]+\d|😀`)
	for in, ok := range map[string]bool{"abc1": true, "😀": true, "abc": false, "ABC1": false, "abc١": false} {
		if _, err := d.Decode(in); (err == nil) != ok {
			t.Errorf("%+q: %v", in, err)
		}
	}
	for pattern, says := range map[string]string{
		`(?i)a`:        "not a pattern of the Raoh pattern language",
		`\p{L}`:        "not a pattern of the Raoh pattern language",
		`(a)\1`:        "not a pattern of the Raoh pattern language",
		`a{249999}`:    "past the limit of 250000 states",
		`a{134217728}`: "past the limit of 134217727 on a repetition count",
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(string), says) {
					t.Errorf("%q: %v", pattern, r)
				}
			}()
			raoh.String().Pattern(pattern)
		}()
	}
	raoh.String().Pattern(`a{249998}`)
}

// Which text is a temporal is the grammar of 199x-notation, and the value is the moment or the day
// the admitted text names.
func TestATemporalIsReadOffTheTextTheGrammarAdmits(t *testing.T) {
	for _, tc := range []struct {
		d    raoh.TemporalDecoder
		in   string
		want time.Time
	}{
		{raoh.String().Instant(), "2024-01-15T24:00:00+09:00", time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)},
		{raoh.String().Instant(), "2024-01-15T10:30:00.5-00:00:01", time.Date(2024, 1, 15, 10, 30, 1, 500_000_000, time.UTC)},
		{raoh.String().Instant(), "-1000000000-01-01T00:00:00Z", time.Date(-1_000_000_000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{raoh.String().Date(), "-0001-03-01", time.Date(-1, 3, 1, 0, 0, 0, 0, time.UTC)},
		{raoh.String().Date(), "+10000-12-31", time.Date(10000, 12, 31, 0, 0, 0, 0, time.UTC)},
		{raoh.String().Time(), "23:59:59.123456789", time.Date(0, 1, 1, 23, 59, 59, 123456789, time.UTC)},
		{raoh.String().DateTime(), "2024-02-29T10:30", time.Date(2024, 2, 29, 10, 30, 0, 0, time.UTC)},
		{raoh.String().OffsetDateTime(), "2024-02-29T10:30+18:00", time.Date(2024, 2, 29, 10, 30, 0, 0, time.FixedZone("", 18*3600))},
	} {
		got, err := tc.d.Decode(tc.in)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%q gives %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for d, in := range map[*raoh.TemporalDecoder]string{
		ptr(raoh.String().Instant()):        "2016-12-31T23:59:60Z",
		ptr(raoh.String().Instant()):        "2024-01-15T24:00:00.0Z",
		ptr(raoh.String().Date()):           "-0000-01-01",
		ptr(raoh.String().Time()):           "24:00",
		ptr(raoh.String().OffsetDateTime()): "2024-01-15T10:30+18:00:01",
	} {
		if _, err := d.Decode(in); err == nil {
			t.Errorf("%q is admitted", in)
		}
	}
}

func ptr[T any](v T) *T { return &v }
