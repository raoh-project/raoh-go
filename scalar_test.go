package raoh_test

import (
	"errors"
	"testing"

	"github.com/kawasima/raoh-go"
)

// The same sets as Raoh for Java's StringDecoderTest: Unicode White_Space,
// and characters that are whitespace to the JDK or invisible but are not
// White_Space. Written out as literals, never derived from unicode.IsSpace.
var whiteSpace = []string{
	"\u0009", "\n", "\u000B", "\u000C", "\r", " ", "\u0085", "\u00A0", "\u1680",
	"\u2000", "\u2001", "\u2002", "\u2003", "\u2004", "\u2005", "\u2006", "\u2007",
	"\u2008", "\u2009", "\u200A", "\u2028", "\u2029", "\u202F", "\u205F", "\u3000",
}

var notWhiteSpace = []string{
	"\u0000", "\u0001", "\u001C", "\u001D", "\u001E", "\u001F", "\u180E", "\u200B",
	"\u200C", "\u200D", "\u2060", "\uFEFF", "a",
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	issues, ok := errors.AsType[*raoh.Issues](err)
	if !ok || issues.Len() != 1 {
		t.Fatalf("want one issue, got %v", err)
	}
	return issues.All()[0].Code()
}

func TestNonBlankRejectsExactlyUnicodeWhiteSpace(t *testing.T) {
	if len(whiteSpace) != 25 {
		t.Fatalf("%d", len(whiteSpace))
	}
	d := raoh.String().NonBlank()
	for _, ws := range append(whiteSpace, "") {
		for _, in := range []string{ws, ws + ws} {
			if _, err := d.Decode(in); codeOf(t, err) != raoh.CodeBlank {
				t.Errorf("%q", in)
			}
		}
	}
	for _, ch := range notWhiteSpace {
		if v, err := d.Decode(ch); err != nil || v != ch {
			t.Errorf("%q: %q, %v", ch, v, err)
		}
	}
}

func TestTrimStripsExactlyUnicodeWhiteSpace(t *testing.T) {
	d := raoh.String().Trim()
	for _, ws := range whiteSpace {
		if v, _ := d.Decode(ws + "x y" + ws); v != "x y" {
			t.Errorf("%q: %q", ws, v)
		}
		if v, _ := d.Decode(ws); v != "" {
			t.Errorf("%q: %q", ws, v)
		}
	}
	for _, ch := range notWhiteSpace {
		if v, _ := d.Decode(ch + "x" + ch); v != ch+"x"+ch {
			t.Errorf("%q: %q", ch, v)
		}
	}
	if v, _ := d.Decode("\u3000\U0001F600\u00A0"); v != "\U0001F600" {
		t.Errorf("%q", v)
	}
}
