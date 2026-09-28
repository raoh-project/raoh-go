package raoh

//go:generate go run ./internal/gencase

import (
	"strings"
	"unicode"
)

// toUpperJava is String.toUpperCase(Locale.ROOT): Unicode's full case
// mapping, in which one character can become several (ß becomes SS).
func toUpperJava(s string) string {
	var b strings.Builder
	for _, r := range s {
		if m, ok := upperExceptions[r]; ok {
			b.WriteString(m)
		} else {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

// toLowerJava is String.toLowerCase(Locale.ROOT): Unicode's full case
// mapping, with a capital sigma at the end of a word becoming the final form
// ς.
func toLowerJava(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		switch m, ok := lowerExceptions[r]; {
		case r == 'Σ' && isFinalSigma(rs, i):
			b.WriteRune('ς')
		case ok:
			b.WriteString(m)
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// isFinalSigma is the Final_Sigma condition of Unicode's SpecialCasing, as
// Java applies it within a word: a cased letter comes before the sigma and none
// comes after it, passing over case-ignorable characters.
func isFinalSigma(rs []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0 && inWord(rs[j]); j-- {
		if isCased(rs[j]) {
			before = true
			break
		}
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(rs) && inWord(rs[j]); j++ {
		if isCased(rs[j]) {
			return false
		}
	}
	return true
}

func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r)
}

// inWord reports whether r continues a word: a letter, a mark, a digit, or a
// case-ignorable character such as an apostrophe.
func inWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r) ||
		unicode.In(r, unicode.Cf, unicode.Lm, unicode.Sk) ||
		strings.ContainsRune("'.:·‘’․﹒＇．", r)
}
