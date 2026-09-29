package raoh_test

import (
	"strings"
	"testing"

	"github.com/raoh-project/raoh-go"
)

func TestNormalize(t *testing.T) {
	const (
		decomposed = "é"
		composed   = "é"
		ligature   = "ﬁ"
	)
	for _, tc := range []struct {
		name string
		d    raoh.StringDecoder
		in   string
		want string
	}{
		{"default is NFC", raoh.String().Normalize(), decomposed, composed},
		{"NFC", raoh.String().NormalizeAs(raoh.NFC), decomposed, composed},
		{"zero value is NFC", raoh.String().NormalizeAs(raoh.NormalForm(0)), decomposed, composed},
		{"NFD", raoh.String().NormalizeAs(raoh.NFD), composed, decomposed},
		{"NFC leaves a compatibility character", raoh.String().Normalize(), ligature, ligature},
		{"NFKC", raoh.String().NormalizeAs(raoh.NFKC), ligature + decomposed, "fi" + composed},
		{"NFKD", raoh.String().NormalizeAs(raoh.NFKD), ligature + composed, "fi" + decomposed},
		{"an invalid byte is kept", raoh.String().Normalize(), decomposed + "\xff" + decomposed, composed + "\xff" + composed},
	} {
		got, err := tc.d.Decode(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("%s: %+q gives %+q, %v; want %+q", tc.name, tc.in, got, err, tc.want)
		}
	}
}

// Normalization is a transformation that runs where it is written, so a length limit after it counts
// the characters it leaves.
func TestNormalizeRunsBeforeTheConstraintsAfterIt(t *testing.T) {
	in := strings.Repeat("é", 3)
	if _, err := raoh.String().MaxLength(3).Decode(in); err == nil {
		t.Fatalf("%+q has six characters and MaxLength(3) accepted it", in)
	}
	if _, err := raoh.String().Normalize().MaxLength(3).Decode(in); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAsRefusesAnUnknownForm(t *testing.T) {
	for _, f := range []raoh.NormalForm{-1, 4} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NormalizeAs(%v) did not panic", f)
				}
			}()
			raoh.String().NormalizeAs(f)
		}()
	}
}

func TestNormalFormString(t *testing.T) {
	for f, want := range map[raoh.NormalForm]string{raoh.NFC: "NFC", raoh.NFD: "NFD", raoh.NFKC: "NFKC", raoh.NFKD: "NFKD", 7: "NormalForm(7)"} {
		if got := f.String(); got != want {
			t.Errorf("%d is %q, want %q", int(f), got, want)
		}
	}
}
