package raoh

import (
	"errors"
	"net/url"
	"strings"
)

// The RFC 3986 URI rule URL reads, as Raoh for Java's UriSyntax defines it.
// It decides acceptance on its own, not by a parser of the platform. It accepts
// the URI rule of RFC 3986 section 3, which requires a scheme, so a relative
// reference is not a URI; raw non-ASCII characters are not accepted.

// URI is an RFC 3986 URI, held as the text that was accepted.
//
// Acceptance is decided by the grammar of Raoh for Java's UriSyntax, not by a
// parser of the platform, so a URI is valid whatever package net/url makes of
// it: http://%41.example/ is one, and package net/url refuses it. For that
// reason the value is the text and the offsets of its components, and String
// gives the text as it was written, with the scheme in the case it had and an
// empty query or fragment kept. The components are not percent-decoded. Use
// [URI.URL] to get a *url.URL, which does not exist for every URI.
//
// A URI is comparable. Its zero value is not a URI, since every URI has a
// scheme: String is empty, no component is present, URL and MarshalText report
// an error.
type URI struct {
	p parsedURI
}

// isZero reports whether u is the zero value.
func (u URI) isZero() bool { return u.p.value == "" }

// ParseURI reads s as an RFC 3986 URI of any scheme, by the rule of
// [StringDecoder.URI].
func ParseURI(s string) (URI, error) {
	if u, ok := newURI(s); ok {
		return u, nil
	}
	return URI{}, errNotURI
}

var errNotURI = errors.New("raoh: not a URI")

// newURI is the URI s writes, if s is an RFC 3986 URI.
func newURI(s string) (URI, bool) {
	p, ok := parseURI(s)
	if !ok {
		return URI{}, false
	}
	return URI{p}, true
}

// String returns the text as it was written.
func (u URI) String() string { return u.p.value }

// Scheme returns the scheme as written, in any case.
func (u URI) Scheme() string { return u.p.value[:u.p.schemeEnd] }

// Authority returns the authority, without the two slashes that start it, and
// whether the URI has one. An empty authority (file:///a) is present and empty.
func (u URI) Authority() (string, bool) {
	if u.isZero() || u.p.authorityStart < 0 {
		return "", false
	}
	return u.p.value[u.p.authorityStart:u.p.authorityEnd], true
}

// Host returns the host as written, an IPv6 address in its brackets, and
// whether the URI has an authority. The host of a registered name can be empty.
func (u URI) Host() (string, bool) {
	if u.isZero() || u.p.authorityStart < 0 {
		return "", false
	}
	return u.p.value[u.p.hostStart:u.p.hostEnd], true
}

// Path returns the path as written, which can be empty. For a URI without an
// authority it is everything between the colon and the query or fragment.
func (u URI) Path() string { return u.p.value[u.p.authorityEnd:u.p.hierEnd] }

// Query returns the query without its question mark, and whether the URI has
// one. An empty query (a:b?) is present and empty.
func (u URI) Query() (string, bool) {
	if !u.p.hasQuery {
		return "", false
	}
	return u.p.value[u.p.hierEnd+1 : u.p.queryEnd()], true
}

// Fragment returns the fragment without its number sign, and whether the URI
// has one. An empty fragment (a:b#) is present and empty.
func (u URI) Fragment() (string, bool) {
	if !u.p.hasFragment {
		return "", false
	}
	return u.p.value[u.p.queryEnd()+1:], true
}

// URL returns the URI as a *url.URL, or the error package net/url gives when it
// cannot hold the text, as it cannot for a percent-encoded host. The *url.URL
// may write itself differently from String: the scheme in lower case and an
// empty fragment left out.
func (u URI) URL() (*url.URL, error) {
	if u.isZero() {
		return nil, errNotURI
	}
	return url.Parse(u.p.value)
}

// MarshalText writes the URI as String does. The zero value is not a URI, and
// writing it would give text that cannot be read back, so it is an error.
func (u URI) MarshalText() ([]byte, error) {
	if u.isZero() {
		return nil, errNotURI
	}
	return []byte(u.p.value), nil
}

// parsedURI is the components of an accepted URI, as offsets into the text.
type parsedURI struct {
	value          string
	schemeEnd      int
	authorityStart int // -1 when there is no authority
	hostStart      int
	hostEnd        int
	authorityEnd   int
	hierEnd        int
	hasQuery       bool
	hasFragment    bool
}

// queryEnd is the offset where the query ends: the number sign that starts the
// fragment, or the end of the text.
func (p parsedURI) queryEnd() int {
	if !p.hasFragment {
		return len(p.value)
	}
	if i := strings.IndexByte(p.value[p.hierEnd:], '#'); i >= 0 {
		return p.hierEnd + i
	}
	return len(p.value)
}

func (p parsedURI) schemeIs(expected string) bool {
	return p.schemeEnd == len(expected) && strings.EqualFold(p.value[:p.schemeEnd], expected)
}

func (p parsedURI) hasHost() bool {
	return p.authorityStart >= 0 && p.hostStart < p.hostEnd
}

func parseURI(value string) (parsedURI, bool) {
	length := len(value)
	schemeEnd := uriScheme(value)
	if schemeEnd < 0 {
		return parsedURI{}, false
	}
	hierStart := schemeEnd + 1
	hierEnd := hierStart
	for hierEnd < length && value[hierEnd] != '?' && value[hierEnd] != '#' {
		hierEnd++
	}
	queryEnd := hierEnd
	if hierEnd < length && value[hierEnd] == '?' {
		queryEnd = indexIn(value, '#', hierEnd, length)
		if queryEnd < 0 {
			queryEnd = length
		}
		if !allMatch(value, hierEnd+1, queryEnd, uriQuery) {
			return parsedURI{}, false
		}
	}
	if queryEnd < length && !allMatch(value, queryEnd+1, length, uriQuery) {
		return parsedURI{}, false
	}

	authorityStart := -1
	hostStart, hostEnd, authorityEnd := hierStart, hierStart, hierStart
	if hierEnd-hierStart >= 2 && value[hierStart] == '/' && value[hierStart+1] == '/' {
		authorityStart = hierStart + 2
		authorityEnd = indexIn(value, '/', authorityStart, hierEnd)
		if authorityEnd < 0 {
			authorityEnd = hierEnd
		}
		hostStart = authorityStart
		if at := indexIn(value, '@', authorityStart, authorityEnd); at >= 0 {
			if !allMatch(value, authorityStart, at, uriUserinfo) {
				return parsedURI{}, false
			}
			hostStart = at + 1
		}
		if hostStart < authorityEnd && value[hostStart] == '[' {
			closing := indexIn(value, ']', hostStart, authorityEnd)
			if closing < 0 {
				return parsedURI{}, false
			}
			inner := value[hostStart+1 : closing]
			if !isIPv6Address(inner) && !isIPFuture(inner) {
				return parsedURI{}, false
			}
			hostEnd = closing + 1
		} else {
			hostEnd = hostStart
			for hostEnd < authorityEnd && value[hostEnd] != ':' {
				hostEnd++
			}
			// IPv4address is a subset of reg-name, so one check covers both.
			if !allMatch(value, hostStart, hostEnd, uriRegName) {
				return parsedURI{}, false
			}
		}
		if hostEnd < authorityEnd {
			if value[hostEnd] != ':' {
				return parsedURI{}, false
			}
			for i := hostEnd + 1; i < authorityEnd; i++ {
				if !isDigit(value[i]) {
					return parsedURI{}, false
				}
			}
		}
	}
	// Every path form is pchar and "/"; the forms differ only in how they
	// start, and a path starting with "//" has already been read as an
	// authority.
	if !allMatch(value, authorityEnd, hierEnd, uriPath) {
		return parsedURI{}, false
	}
	return parsedURI{value, schemeEnd, authorityStart, hostStart, hostEnd, authorityEnd, hierEnd,
		queryEnd > hierEnd, queryEnd < length}, true
}

// indexIn is the index of c in value[from:to], or -1.
func indexIn(value string, c byte, from, to int) int {
	if i := strings.IndexByte(value[from:to], c); i >= 0 {
		return from + i
	}
	return -1
}

// uriScheme returns the index of the ":" that ends the scheme, or -1.
// scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )
func uriScheme(value string) int {
	if value == "" || !isAlpha(value[0]) {
		return -1
	}
	for i := 1; i < len(value); i++ {
		c := value[i]
		if c == ':' {
			return i
		}
		if !isAlpha(c) && !isDigit(c) && c != '+' && c != '-' && c != '.' {
			return -1
		}
	}
	return -1
}

// isIPFuture reports whether s is an IPvFuture:
// "v" 1*HEXDIG "." 1*( unreserved / sub-delims / ":" ). The "v" is
// case-insensitive, as every quoted string in ABNF is (RFC 5234 section 2.3).
func isIPFuture(s string) bool {
	if s == "" || s[0]|0x20 != 'v' {
		return false
	}
	pos := 1
	for pos < len(s) && isHexDigit(s[pos]) {
		pos++
	}
	if pos == 1 || pos >= len(s) || s[pos] != '.' {
		return false
	}
	pos++
	if pos == len(s) {
		return false
	}
	for ; pos < len(s); pos++ {
		if c := s[pos]; !isUnreserved(c) && !isSubDelim(c) && c != ':' {
			return false
		}
	}
	return true
}

// The characters each component allows besides unreserved, sub-delims and
// pct-encoded.
const (
	uriUserinfo = ":"    // userinfo = *( unreserved / pct-encoded / sub-delims / ":" )
	uriRegName  = ""     // reg-name = *( unreserved / pct-encoded / sub-delims )
	uriPath     = ":@/"  // pchar = unreserved / pct-encoded / sub-delims / ":" / "@"; paths add "/"
	uriQuery    = ":@/?" // query = fragment = *( pchar / "/" / "?" )
)

// allMatch reports whether value[from:to] is made of unreserved, sub-delims,
// pct-encoded and the extra characters.
func allMatch(value string, from, to int, extra string) bool {
	for i := from; i < to; i++ {
		c := value[i]
		switch {
		case c == '%':
			if i+2 >= to || !isHexDigit(value[i+1]) || !isHexDigit(value[i+2]) {
				return false
			}
			i += 2
		case !isUnreserved(c) && !isSubDelim(c) && strings.IndexByte(extra, c) < 0:
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// unreserved = ALPHA / DIGIT / "-" / "." / "_" / "~"
func isUnreserved(c byte) bool {
	return isAlpha(c) || isDigit(c) || c == '-' || c == '.' || c == '_' || c == '~'
}

// sub-delims = "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / ";" / "="
func isSubDelim(c byte) bool { return strings.IndexByte("!$&'()*+,;=", c) >= 0 }
