package norm

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

var forms = []struct {
	name string
	form Form
}{{"NFC", NFC}, {"NFD", NFD}, {"NFKC", NFKC}, {"NFKD", NFKD}}

func runes(t *testing.T, field string) string {
	t.Helper()
	var b strings.Builder
	for _, h := range strings.Fields(field) {
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteRune(rune(n))
	}
	return b.String()
}

// TestUnicodeConformance holds the forms to the Unicode Consortium's NormalizationTest.txt, which
// UAX #15 requires an implementation to reproduce.
func TestUnicodeConformance(t *testing.T) {
	f, err := os.Open("../../testdata/unicode/16.0.0/NormalizationTest.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	listed := map[rune]bool{}
	part1, cases := false, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "@Part") {
			part1 = strings.HasPrefix(line, "@Part1")
			continue
		}
		if line == "" || line[0] == '#' {
			continue
		}
		c := strings.Split(line, ";")
		if len(c) < 5 {
			t.Fatalf("bad line %q", line)
		}
		var col [5]string
		for i := range col {
			col[i] = runes(t, c[i])
		}
		if part1 && len([]rune(col[0])) == 1 {
			listed[[]rune(col[0])[0]] = true
		}
		cases++
		// Each column is a form of the ones the file lists it under.
		check := func(f Form, name string, in []int, out int) {
			for _, i := range in {
				if got := String(f, col[i]); got != col[out] {
					t.Errorf("%s(c%d) of line %q = %+q, want c%d %+q", name, i+1, line, got, out+1, col[out])
				}
			}
		}
		check(NFC, "NFC", []int{0, 1, 2}, 1)
		check(NFC, "NFC", []int{3, 4}, 3)
		check(NFD, "NFD", []int{0, 1, 2}, 2)
		check(NFD, "NFD", []int{3, 4}, 4)
		check(NFKC, "NFKC", []int{0, 1, 2, 3, 4}, 3)
		check(NFKD, "NFKD", []int{0, 1, 2, 3, 4}, 4)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if cases < 18000 {
		t.Fatalf("only %d cases read", cases)
	}
	// A code point the file does not list in Part 1 is unchanged by every form.
	for r := rune(0); r <= 0x10FFFF; r++ {
		if listed[r] || (r >= 0xD800 && r <= 0xDFFF) {
			continue
		}
		s := string(r)
		for _, f := range forms {
			if got := String(f.form, s); got != s {
				t.Fatalf("%s(%U) = %+q, want it unchanged", f.name, r, got)
			}
		}
	}
}

func TestHangul(t *testing.T) {
	for _, tc := range []struct{ composed, decomposed string }{
		{"가", "가"},
		{"각", "각"},
		{"힣", "힣"},
	} {
		if got := String(NFD, tc.composed); got != tc.decomposed {
			t.Errorf("NFD(%+q) = %+q", tc.composed, got)
		}
		if got := String(NFC, tc.decomposed); got != tc.composed {
			t.Errorf("NFC(%+q) = %+q", tc.decomposed, got)
		}
	}
	// A vowel does not compose with a trailing consonant on its own.
	if got := String(NFC, "ᅡᆨ"); got != "ᅡᆨ" {
		t.Errorf("NFC(V T) = %+q", got)
	}
}

func TestCompositionBlocking(t *testing.T) {
	for _, tc := range []struct {
		name string
		form Form
		in   string
		want string
	}{
		{"order", NFC, "ạ́", "ạ́"},
		{"reordered then composed", NFC, "ạ́", "ạ́"},
		{"same class blocks", NFC, "á́", "á́"},
		{"lower class does not block", NFC, "ậ", "ậ"},
		{"a starter blocks", NFC, "aá", "aá"},
		{"starters compose", NFC, "ୋ", "ୋ"},
		{"compat", NFKC, "ﬁ", "fi"},
		{"compat decomposition is not in NFD", NFD, "ﬁ", "ﬁ"},
		{"excluded", NFC, "क़", "क़"},
		{"singleton", NFC, "Å", "Å"},
	} {
		if got := String(tc.form, tc.in); got != tc.want {
			t.Errorf("%s: %+q gives %+q, want %+q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestInvalidUTF8(t *testing.T) {
	for _, tc := range []struct {
		name string
		form Form
		in   string
		want string
	}{
		{"kept", NFC, "a\xffb", "a\xffb"},
		{"the replacement character is not an invalid byte", NFC, "\xef\xbf\xbd\xff", "\xef\xbf\xbd\xff"},
		{"composes on each side", NFC, "é\xffé", "é\xffé"},
		{"composes with nothing across it", NFC, "e\xff́", "e\xff́"},
		{"separates the marks", NFD, "ẹ\xff̣́", "ẹ\xff̣́"},
		{"a truncated sequence", NFD, "é\xe3\x81", "é\xe3\x81"},
		{"a lone surrogate", NFD, "\xed\xa0\x80é", "\xed\xa0\x80é"},
	} {
		if got := String(tc.form, tc.in); got != tc.want {
			t.Errorf("%s: %+q gives %+q, want %+q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestUnchangedIsNotCopied holds text a form leaves as it is to the same string.
func TestUnchangedIsNotCopied(t *testing.T) {
	for _, in := range []string{"", "abc", "日本語のテキスト", "é", "é\xff"} {
		for _, f := range forms {
			got := String(f.form, in)
			same := unsafe.StringData(got) == unsafe.StringData(in)
			if got == in && len(in) > 0 && !same {
				t.Errorf("%s(%+q) is a copy", f.name, in)
			}
		}
	}
}

// TestLinear holds the work to a bound linear in the input on inputs that make a naive
// implementation quadratic: long runs of marks in descending class, marks that compose or fail to,
// and characters that decompose to many.
func TestLinear(t *testing.T) {
	inputs := map[string]func(n int) string{
		"descending marks": func(n int) string {
			return "a" + strings.Repeat("̣̖́", n/3) // 230, 220, 220 class
		},
		"one class": func(n int) string { return "a" + strings.Repeat("́", n) },
		"classes 232 to 202": func(n int) string {
			var b strings.Builder
			b.WriteString("a")
			for i := 0; b.Len() < 2*n; i++ {
				b.WriteRune([]rune{0x0327, 0x0323, 0x0316, 0x0301, 0x035C}[i%5])
			}
			return b.String()
		},
		"composing pairs": func(n int) string { return strings.Repeat("é", n/2) },
		"starters":        func(n int) string { return strings.Repeat("각", n/3) },
		"expansions":      func(n int) string { return strings.Repeat("ﷺ", n) },
		"astral":          func(n int) string { return strings.Repeat("\U0001D15E", n) },
	}
	for name, gen := range inputs {
		for _, f := range forms {
			for _, size := range []int{1000, 100000} {
				in := gen(size)
				n := normalizer{form: f.form, in: in}
				n.run()
				if bound := 100*len(in) + 100; n.work > bound {
					t.Errorf("%s %s of %d bytes: %d steps, over %d", name, f.name, len(in), n.work, bound)
				}
			}
		}
	}
}

func BenchmarkNormalizeLongNonStarters(b *testing.B) {
	in := "a" + strings.Repeat("̣̖́", 100000)
	for _, f := range forms {
		b.Run(f.name, func(b *testing.B) {
			b.SetBytes(int64(len(in)))
			for range b.N {
				String(f.form, in)
			}
		})
	}
}
