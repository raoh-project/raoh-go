package raoh

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	notation199x "github.com/raoh-project/notation-199x/go"
)

const maxEmailLength = 254

// StringDecoder decodes a string.
//
// Null or a missing member is required; any other type is type_mismatch.
// Constraints and transformations run in the order they are written, and the
// first constraint to fail is the one reported. An empty string is accepted
// unless NonBlank says otherwise. What counts as white space, a character, the
// order of strings, case, normalization and a pattern are the rules of
// notation-199x, which Raoh for Java follows too, so none of them changes with
// the Go release.
type StringDecoder struct {
	Decoder[any, string]
	s scalar[string]
}

// String returns a decoder of a string.
//
// A string is a sequence of Unicode scalar values. A Go string holding bytes
// that are not UTF-8 is none, and is type_mismatch as any other value that is
// not a string. Text read by DecodeJSON is never such a string: encoding/json
// puts U+FFFD in place of those bytes.
func String() StringDecoder {
	return newString(scalar[string]{read: func(in any) (string, *Issue) {
		s, ok := plain(in).(string)
		if !ok || notation199x.InvalidUTF8At(s) >= 0 {
			i := unexpected("string", in)
			return "", &i
		}
		return s, nil
	}})
}

func newString(s scalar[string]) StringDecoder { return StringDecoder{s.build(), s} }

func (d StringDecoder) require(ok func(string) bool, fail func(string) Issue) StringDecoder {
	return newString(d.s.require(ok, fail))
}

func (d StringDecoder) format(ok func(string) bool, key string) StringDecoder {
	return d.require(ok, func(string) Issue { return NewIssue(CodeInvalidFormat).WithMessageKey(key) })
}

// Message gives the most recent constraint written before it, or the type
// check when there is none, a custom message that every language shows as
// written. Transformations such as Trim are passed over, as they cannot fail.
func (d StringDecoder) Message(message string) StringDecoder {
	return newString(d.s.message(message))
}

// Trim removes white space from both ends: the characters with the Unicode
// 18.0.0 White_Space property, which include U+3000 and U+00A0 and not control
// characters such as NUL. It is the set NonBlank uses.
func (d StringDecoder) Trim() StringDecoder {
	return newString(d.s.transform(func(v string) string {
		return strings.TrimFunc(v, notation199x.IsWhiteSpace)
	}))
}

// ToLower converts to lower case with the Unicode 18.0.0 default case
// conversion, untailored: İ becomes i̇, and a capital sigma becomes ς only at the
// end of a cased run, so ΟΣ becomes ος and Α1Σ becomes α1σ.
func (d StringDecoder) ToLower() StringDecoder {
	return newString(d.s.transform(notation199x.Lowercase))
}

// ToUpper converts to upper case with the Unicode 18.0.0 default case
// conversion, untailored: ß becomes SS.
func (d StringDecoder) ToUpper() StringDecoder {
	return newString(d.s.transform(notation199x.Uppercase))
}

// NonBlank requires a character that is not white space, in the sense Trim
// uses: blank. An empty string is blank.
func (d StringDecoder) NonBlank() StringDecoder {
	return d.require(
		func(v string) bool {
			return strings.IndexFunc(v, func(r rune) bool { return !notation199x.IsWhiteSpace(r) }) >= 0
		},
		func(string) Issue { return NewIssue(CodeBlank) })
}

// MinLength requires at least n characters, counted as scalar values:
// too_short with min and actual.
func (d StringDecoder) MinLength(n int) StringDecoder {
	return d.require(
		func(v string) bool { return notation199x.ScalarCount(v) >= n },
		func(v string) Issue {
			return NewIssue(CodeTooShort).WithMeta("min", n).WithMeta("actual", notation199x.ScalarCount(v))
		})
}

// MaxLength allows at most n characters, counted as scalar values: too_long
// with max and actual.
func (d StringDecoder) MaxLength(n int) StringDecoder {
	return d.require(
		func(v string) bool { return notation199x.ScalarCount(v) <= n },
		func(v string) Issue {
			return NewIssue(CodeTooLong).WithMeta("max", n).WithMeta("actual", notation199x.ScalarCount(v))
		})
}

// Length requires exactly n characters, counted as scalar values:
// invalid_length with expected and actual.
func (d StringDecoder) Length(n int) StringDecoder {
	return d.require(
		func(v string) bool { return notation199x.ScalarCount(v) == n },
		func(v string) Issue {
			return NewIssue(CodeInvalidLength).WithMeta("expected", n).WithMeta("actual", notation199x.ScalarCount(v))
		})
}

// StartsWith requires the string to start with prefix: invalid_format with
// prefix.
func (d StringDecoder) StartsWith(prefix string) StringDecoder {
	return d.require(
		func(v string) bool { return strings.HasPrefix(v, prefix) },
		func(string) Issue {
			return NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatStartsWith).WithMeta("prefix", prefix)
		})
}

// EndsWith requires the string to end with suffix: invalid_format with
// suffix.
func (d StringDecoder) EndsWith(suffix string) StringDecoder {
	return d.require(
		func(v string) bool { return strings.HasSuffix(v, suffix) },
		func(string) Issue {
			return NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatEndsWith).WithMeta("suffix", suffix)
		})
}

// Contains requires the string to contain substring: invalid_format with
// substring.
func (d StringDecoder) Contains(substring string) StringDecoder {
	return d.require(
		func(v string) bool { return strings.Contains(v, substring) },
		func(string) Issue {
			return NewIssue(CodeInvalidFormat).WithMessageKey(KeyInvalidFormatIncludes).WithMeta("substring", substring)
		})
}

// OneOf requires one of allowed: not_allowed with allowed sorted by code
// point, and actual. It panics when a string is repeated.
func (d StringDecoder) OneOf(allowed ...string) StringDecoder {
	sorted := sortedDistinct(allowed)
	return d.require(
		func(v string) bool { _, ok := slices.BinarySearch(sorted, v); return ok },
		func(v string) Issue {
			return NewIssue(CodeNotAllowed).WithMeta("allowed", sorted).WithMeta("actual", v)
		})
}

// Email requires a mailbox of an ASCII profile of RFC 5321's Mailbox,
// Dot-string "@" Domain: invalid_format. The local part is RFC 5322 atext atoms
// joined by single dots, at most 64 octets; the domain is labels joined by
// single dots, each starting and ending with a letter or digit and holding
// letters, digits and hyphens, at most 63 octets; at most 254 octets in all. A
// single-label or all-digit domain (a@localhost, a@123) is accepted. A quoted
// local part, an address literal and non-ASCII characters are not.
func (d StringDecoder) Email() StringDecoder {
	return d.format(isEmail, KeyInvalidFormatEmail)
}

// IPv4 requires an IPv4 address in dotted decimal without leading zeros:
// invalid_format.
func (d StringDecoder) IPv4() StringDecoder {
	return d.format(isIPv4, KeyInvalidFormatIPv4)
}

// IPv6 requires an IPv6 address in the RFC 4291 text form, as Raoh for Java
// checks it: invalid_format. An embedded dotted quad (::ffff:192.0.2.1) is
// allowed and brackets ([::1]) are not. A zone ID (fe80::1%eth0) is allowed on
// a link-local or non-global multicast address and decided by its text alone,
// not by the host's interfaces.
func (d StringDecoder) IPv6() StringDecoder {
	return d.format(isIPv6, KeyInvalidFormatIPv6)
}

// IP requires an address IPv4 or IPv6 accepts: invalid_format.
func (d StringDecoder) IP() StringDecoder {
	return d.format(func(v string) bool { return isIPv4(v) || isIPv6(v) }, KeyInvalidFormatIP)
}

// ULID requires a ULID, 26 characters of Crockford's base 32 in either case
// whose value fits in 128 bits, so that the first is 0 to 7 and
// 7ZZZZZZZZZZZZZZZZZZZZZZZZZ is the largest: invalid_format. The string is
// given unchanged.
func (d StringDecoder) ULID() StringDecoder {
	return d.format(func(v string) bool {
		if len(v) != 26 || v[0] < '0' || v[0] > '7' {
			return false
		}
		for i := range len(v) {
			if !isCrockford(v[i]) {
				return false
			}
		}
		return true
	}, KeyInvalidFormatULID)
}

// isCrockford is whether b is a digit of Crockford's base 32, in either case:
// the letters but I, L, O and U.
func isCrockford(b byte) bool {
	if isDigit(b) {
		return true
	}
	b |= 0x20 // lower case
	return b >= 'a' && b <= 'z' && b != 'i' && b != 'l' && b != 'o' && b != 'u'
}

// CUID requires a CUID, c followed by 24 lower-case letters or digits:
// invalid_format.
func (d StringDecoder) CUID() StringDecoder {
	return d.format(func(v string) bool {
		if len(v) != 25 || v[0] != 'c' {
			return false
		}
		for i := 1; i < len(v); i++ {
			if b := v[i]; !isDigit(b) && !(b >= 'a' && b <= 'z') {
				return false
			}
		}
		return true
	}, KeyInvalidFormatCUID)
}

// Pattern requires the whole string to be one of the strings pattern accepts:
// invalid_format with pattern. pattern is in the pattern language of the Raoh
// Specification, the one Raoh for Java and Souther read, and not in the syntax
// of package regexp: a back reference, a lookaround, a property class such as
// \p{L} and a flag such as (?i) are not in it. \d, \w and \s are ASCII sets.
//
// A pattern is admitted within three limits, counted from its text: a
// repetition count of at most 134,217,727, groups nested at most 200 deep, and
// at most 250,000 states once its repetitions are written out. So a{249998} is
// admitted and a{249999} is not.
//
// It panics when pattern is not a pattern of the language, or is past a limit,
// with a message that tells the two apart. A value is matched in one pass over
// it, so the time a match takes is linear in the length of the value whatever
// the pattern.
func (d StringDecoder) Pattern(pattern string) StringDecoder {
	read := compilePattern(pattern)
	return d.require(read.Matches, func(string) Issue {
		return NewIssue(CodeInvalidFormat).WithMeta("pattern", pattern)
	})
}

// compilePattern is what pattern means, or a panic that says why it is not a
// pattern or which limit it is past.
func compilePattern(pattern string) *notation199x.Pattern {
	switch read := notation199x.ReadPattern(pattern).(type) {
	case *notation199x.Pattern:
		return read
	case notation199x.PatternRefused:
		where := ""
		if read.Construct != "" {
			where = fmt.Sprintf(": %q", read.Construct)
		}
		panic(fmt.Sprintf("raoh: not a pattern of the Raoh pattern language: %q (%v at byte %d%s)",
			pattern, read.Why, read.From, where))
	case notation199x.PatternBeyond:
		where := ""
		if read.From != 0 || read.Construct != pattern {
			where = fmt.Sprintf(" at byte %d: %q", read.From, read.Construct)
		}
		panic(fmt.Sprintf("raoh: pattern %q is past the limit of %d %s%s",
			pattern, read.Limit.Most(), limitName(read.Limit), where))
	default:
		panic(fmt.Sprintf("raoh: ReadPattern answered %T", read))
	}
}

// limitName is what a limit counts, as a message says it.
func limitName(limit notation199x.PatternLimit) string {
	switch limit {
	case notation199x.RepetitionCount:
		return "on a repetition count"
	case notation199x.NestingDepth:
		return "on groups nested one inside another"
	case notation199x.MachineStates:
		return "states once its repetitions are written out"
	default:
		return limit.String()
	}
}

// UUID returns a decoder that reads the string as a UUID in the RFC 9562 form:
// 32 hexadecimal digits grouped 8-4-4-4-12 by hyphens, in any case.
// Otherwise it reports invalid_format.
func (d StringDecoder) UUID() Conversion[UUID] {
	return newConversion(d.s, KeyInvalidFormatUUID, parseUUID)
}

// URL returns a decoder that reads the string as an http or https URL with a
// host: invalid_format when it is not one.
//
// The text is an RFC 3986 URI, with the http or https scheme in any case, an
// authority and a non-empty host, as the Raoh Specification says. The host is the
// RFC 3986 host, not a DNS name, so a reg-name such as my_host is accepted, and
// raw non-ASCII characters are not. The value is a [URI], which keeps the text
// as written.
func (d StringDecoder) URL() Conversion[URI] {
	return newConversion(d.s, KeyInvalidFormatURL, func(v string) (URI, bool) {
		u, ok := newURI(v)
		return u, ok && (u.p.schemeIs("http") || u.p.schemeIs("https")) && u.p.hasHost()
	})
}

// URI returns a decoder that reads the string as an RFC 3986 URI of any
// scheme: invalid_format.uri when it is not one.
//
// It is the rule URL applies without the check for the http or https scheme
// and a host. A scheme is still required, so a relative reference such as
// foo/bar or #top is refused, and so are raw non-ASCII characters. It is the
// whole RFC 3986 URI production, as the Raoh Specification says: a: and a:#f,
// a:// with nothing after it, an IPvFuture host such as [v1.abc], and a port of
// any length are URIs. Raoh for Java refuses these, since java.net.URI, which
// it gives, cannot hold them. An IPv6 host is checked as IPv6 checks it,
// without a zone ID.
//
// Whether the text is a URI does not depend on package net/url. The value is a
// [URI] that keeps the text as written; [URI.URL] gives a *url.URL when net/url
// can hold it.
func (d StringDecoder) URI() Conversion[URI] {
	return newConversion(d.s, KeyInvalidFormatURI, newURI)
}

// Conversion is a decoder that reads a string and converts it to a T,
// reporting invalid_format when the string does not convert.
type Conversion[T any] struct {
	Decoder[any, T]
	s       scalar[string]
	key     string
	convert func(string) (T, bool)
	message *string
}

func newConversion[T any](s scalar[string], key string, convert func(string) (T, bool)) Conversion[T] {
	c := Conversion[T]{s: s, key: key, convert: convert}
	c.Decoder = c.build()
	return c
}

// Message gives the issue a string that does not convert is reported with a
// custom message.
func (c Conversion[T]) Message(message string) Conversion[T] {
	c.message = &message
	c.Decoder = c.build()
	return c
}

func (c Conversion[T]) build() Decoder[any, T] {
	if c.convert == nil {
		refuseZeroValue()
	}
	str := c.s.build()
	return Decoder[any, T]{func(in any, at Path) outcome[T] {
		o := str.run(in, at)
		if o.failed() {
			return failAs[T](o)
		}
		v, ok := c.convert(o.value)
		if !ok {
			return invalid[T](withCustom(NewIssue(CodeInvalidFormat).WithMessageKey(c.key).At(at), c.message))
		}
		return succeed(v)
	}}
}

// UUID is a universally unique identifier.
type UUID [16]byte

// String writes u in the lower-case 8-4-4-4-12 form.
func (u UUID) String() string {
	h := hex.EncodeToString(u[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func parseUUID(v string) (UUID, bool) {
	var u UUID
	if len(v) != 36 || v[8] != '-' || v[13] != '-' || v[18] != '-' || v[23] != '-' {
		return u, false
	}
	digits := v[:8] + v[9:13] + v[14:18] + v[19:23] + v[24:]
	if _, err := hex.Decode(u[:], []byte(digits)); err != nil {
		return u, false
	}
	return u, true
}

const (
	// maxEmailLocalPart is RFC 5321 section 4.5.3.1.1's limit on a local part.
	maxEmailLocalPart = 64
	// maxEmailLabel is RFC 1035 section 2.3.4's limit on a label.
	maxEmailLabel = 63
)

// isEmail is whether s is a mailbox of the profile Email describes. Every
// character the profile accepts is ASCII, so once one outside it is refused a
// count of bytes is a count of octets. It runs in time linear in s and builds
// nothing.
func isEmail(s string) bool {
	if len(s) > maxEmailLength {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at < 0 || at > maxEmailLocalPart {
		return false
	}
	return isDotString(s[:at]) && isMailDomain(s[at+1:])
}

// isDotString is RFC 5321's Dot-string: Atom *("." Atom), an atom being one or
// more RFC 5322 atext.
func isDotString(s string) bool {
	atom := 0
	for i := range len(s) {
		switch c := s[i]; {
		case c == '.':
			if atom == 0 {
				return false
			}
			atom = 0
		case isAlpha(c) || isDigit(c) || strings.IndexByte("!#$%&'*+-/=?^_`{|}~", c) >= 0:
			atom++
		default:
			return false
		}
	}
	return atom > 0
}

// isMailDomain is RFC 5321's Domain: sub-domain *("." sub-domain), a
// sub-domain being a letter or digit, optionally followed by letters, digits
// and hyphens that end in a letter or digit.
func isMailDomain(s string) bool {
	label, previous := 0, byte('.')
	for i := range len(s) {
		switch c := s[i]; {
		case c == '.':
			if label == 0 || previous == '-' {
				return false
			}
			label = 0
		case isAlpha(c) || isDigit(c) || c == '-' && label > 0:
			label++
			if label > maxEmailLabel {
				return false
			}
		default:
			return false
		}
		previous = s[i]
	}
	return label > 0 && previous != '-'
}

func allBytes(s string, ok func(byte) bool) bool {
	for i := range len(s) {
		if !ok(s[i]) {
			return false
		}
	}
	return true
}
