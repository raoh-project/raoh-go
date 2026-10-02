package raoh

import (
	"fmt"

	notation199x "github.com/raoh-project/199x-notation/go"
)

// NormalForm is a Unicode normalization form. The zero value is NFC, the form
// Normalize applies.
type NormalForm int

const (
	// NFC is canonical decomposition followed by canonical composition: é is
	// one character.
	NFC NormalForm = iota
	// NFD is canonical decomposition: é is e and a combining acute accent.
	NFD
	// NFKC is compatibility decomposition followed by canonical composition:
	// the ligature ﬁ becomes fi and ① becomes 1, and é is one character.
	NFKC
	// NFKD is compatibility decomposition: the ligature ﬁ becomes fi and ①
	// becomes 1, and é is e and a combining acute accent.
	NFKD
)

// Normalize converts to Unicode normalization form NFC, as Raoh for Java's
// normalize does: a character written as a base and combining marks becomes
// the single character where there is one, so that the same text compares
// equal however it was typed.
//
// The forms are those of Unicode 18.0.0, whatever Go release this is built
// with. A string that is already normalized is returned as it is, and the time
// taken is linear in the length of the string whatever it holds.
func (d StringDecoder) Normalize() StringDecoder { return d.NormalizeAs(NFC) }

// NormalizeAs converts to Unicode normalization form f, as Raoh for Java's
// normalize(Normalizer.Form) does, and as Normalize describes. It panics when
// f is not one of NFC, NFD, NFKC and NFKD.
func (d StringDecoder) NormalizeAs(f NormalForm) StringDecoder {
	var form notation199x.Form
	switch f {
	case NFC:
		form = notation199x.NFC
	case NFD:
		form = notation199x.NFD
	case NFKC:
		form = notation199x.NFKC
	case NFKD:
		form = notation199x.NFKD
	default:
		panic(fmt.Sprintf("raoh: invalid NormalForm(%d)", int(f)))
	}
	return newString(d.s.transform(func(v string) string { return notation199x.Normalize(form, v) }))
}
