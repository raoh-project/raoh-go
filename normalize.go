package raoh

//go:generate go run ./internal/gennorm

import (
	"fmt"

	"github.com/raoh-project/raoh-go/internal/norm"
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

func (f NormalForm) String() string {
	switch f {
	case NFC:
		return "NFC"
	case NFD:
		return "NFD"
	case NFKC:
		return "NFKC"
	case NFKD:
		return "NFKD"
	}
	return fmt.Sprintf("NormalForm(%d)", int(f))
}

// Normalize converts to Unicode normalization form NFC, as Raoh for Java's
// normalize does: a character written as a base and combining marks becomes
// the single character where there is one, so that the same text compares
// equal however it was typed.
//
// The result is what java.text.Normalizer gives on Java 25, which knows
// Unicode 16.0. A character that Unicode has assigned since is left as it is.
// A byte that is not part of valid UTF-8 is kept as it is, and it composes with
// nothing and keeps the characters on either side of it apart. A string
// that is already normalized is returned as it is, and the time taken is
// linear in the length of the string whatever it holds.
func (d StringDecoder) Normalize() StringDecoder { return d.NormalizeAs(NFC) }

// NormalizeAs converts to Unicode normalization form f, as Raoh for Java's
// normalize(Normalizer.Form) does, and as Normalize describes. It panics when
// f is not one of NFC, NFD, NFKC and NFKD.
func (d StringDecoder) NormalizeAs(f NormalForm) StringDecoder {
	var form norm.Form
	switch f {
	case NFC:
		form = norm.NFC
	case NFD:
		form = norm.NFD
	case NFKC:
		form = norm.NFKC
	case NFKD:
		form = norm.NFKD
	default:
		panic(fmt.Sprintf("raoh: invalid %v", f))
	}
	return newString(d.s.transform(func(v string) string { return norm.String(form, v) }))
}
