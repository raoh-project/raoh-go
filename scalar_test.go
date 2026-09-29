package raoh_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/raoh-project/raoh-go"
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

func TestFloat32ReadsNumbersAsRaohForJavaDoes(t *testing.T) {
	nan := float32(math.NaN())
	if v, err := raoh.Float32().Decode(float64(math.NaN())); err != nil || v == v {
		t.Errorf("NaN is read: %v, %v", v, err)
	}
	if _, err := raoh.Float32().Range(0, 1).Decode(nan); codeOf(t, err) != raoh.CodeOutOfRange {
		t.Errorf("a range rejects NaN: %v", err)
	}
	if _, err := raoh.Float32().Max(1).Decode(nan); codeOf(t, err) != raoh.CodeOutOfRange {
		t.Errorf("NaN is above every value: %v", err)
	}
	if _, err := raoh.Float32().OneOf(nan).Decode(nan); err != nil {
		t.Errorf("NaN is one of NaN, as Float.equals has it: %v", err)
	}
	for _, in := range []any{math.Inf(1), float32(math.Inf(-1)), math.MaxFloat64} {
		_, err := raoh.Float32().Decode(in)
		issues, _ := errors.AsType[*raoh.Issues](err)
		if issues == nil || issues.All()[0].MessageKey() != raoh.KeyTypeMismatchNumericRange {
			t.Errorf("%v: %v", in, err)
		}
	}
	if _, err := raoh.Float64().Decode(math.Inf(1)); err == nil {
		t.Error("Float64 reads an infinity")
	}
	if v, err := raoh.Float32().Decode(int64(16777217)); err != nil || v != 16777216 {
		t.Errorf("16777217 is %v, %v", v, err)
	}
}

func TestFloatRangePanicsWhenMinIsAboveMax(t *testing.T) {
	for name, f := range map[string]func(){
		"float32": func() { raoh.Float32().Range(2, 1) },
		"float64": func() { raoh.Float64().Range(2, 1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
	raoh.Float64().Range(math.Copysign(0, -1), 0)
}

func TestBytesReadsAByteSliceAsItIs(t *testing.T) {
	type Blob []byte
	in := Blob{1, 2, 3}
	got, err := raoh.Bytes().Decode(in)
	if err != nil || len(got) != 3 || &got[0] != &in[0] {
		t.Errorf("a named []byte is read without a copy: %v, %v", got, err)
	}
	if _, err := raoh.Bytes().Decode(nil); codeOf(t, err) != raoh.CodeRequired {
		t.Errorf("nil: %v", err)
	}
	for _, c := range []struct {
		in     any
		actual string
	}{{"abc", "string"}, {1, "number"}, {[]any{1.0}, "array"}} {
		_, err := raoh.Bytes().Decode(c.in)
		issues, _ := errors.AsType[*raoh.Issues](err)
		if issues == nil || issues.All()[0].Meta()["expected"] != "byte[]" || issues.All()[0].Meta()["actual"] != c.actual {
			t.Errorf("%v: %v", c.in, err)
		}
	}
	if _, err := raoh.Int().Decode([]byte{1}); err == nil {
		t.Error("bytes are not a number")
	}
	if _, err := raoh.DecodeJSON([]byte(`[1,2,3]`), raoh.Bytes()); err == nil {
		t.Error("a JSON array is not bytes")
	}
}

func TestFloatReadsAnIntegerTypeRoundedOnce(t *testing.T) {
	for _, in := range []any{int64(16777217), int64(math.MaxInt64), int64(math.MinInt64), uint64(math.MaxUint64),
		uint64(1<<53 + 1), int32(math.MaxInt32), uint8(255), int(-3)} {
		n, _ := new(big.Int).SetString(fmt.Sprint(in), 10)
		want, _ := new(big.Float).SetInt(n).Float32()
		if got, err := raoh.Float32().Decode(in); err != nil || got != want {
			t.Errorf("%T %v as float32: %v, %v; want %v", in, in, got, err, want)
		}
	}
	if got, err := raoh.Float64().Decode(uint64(1<<53 + 1)); err != nil || got != 1<<53 {
		t.Errorf("1<<53+1 as float64: %v, %v", got, err)
	}
}
