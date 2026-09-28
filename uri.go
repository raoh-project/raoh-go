package raoh

import "strings"

// The RFC 3986 URI rule URL reads, as Raoh for Java's UriSyntax defines it.
// It decides acceptance on its own, not by a parser of the platform. It accepts
// the URI rule of RFC 3986 section 3, which requires a scheme, so a relative
// reference is not a URI; raw non-ASCII characters are not accepted.

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

func (p parsedURI) schemeIs(expected string) bool {
	return p.schemeEnd == len(expected) && strings.EqualFold(p.value[:p.schemeEnd], expected)
}

func (p parsedURI) hasHost() bool {
	return p.authorityStart >= 0 && p.hostStart < p.hostEnd
}

// representableAsJavaURI reports whether java.net.URI can be built from the
// text. It follows RFC 2396 and RFC 2732 and cannot hold four kinds of RFC 3986
// URI: an empty scheme-specific part, an empty authority with nothing after
// it, an IPvFuture host, and a port that does not fit an int after an IP
// literal. Raoh for Java refuses them, and so does this package.
func (p parsedURI) representableAsJavaURI() bool {
	pathEmpty := p.authorityEnd == p.hierEnd
	if p.authorityStart < 0 {
		return p.schemeEnd+1 < p.hierEnd || p.hasQuery
	}
	if p.authorityStart == p.authorityEnd && pathEmpty && !p.hasQuery && !p.hasFragment {
		return false
	}
	// A bracket starts an IP-literal, and an IPvFuture inside it starts with "v".
	if p.hostStart >= p.hostEnd || p.value[p.hostStart] != '[' {
		return true
	}
	if p.value[p.hostStart+1]|0x20 == 'v' {
		return false
	}
	// hostEnd is the "]"'s successor; a port, when there is one, follows a ":".
	return p.hostEnd == p.authorityEnd || p.fitsInt(p.hostEnd+1, p.authorityEnd)
}

// fitsInt reports whether the decimal digits in value[from:to] denote a value
// no greater than 2147483647.
func (p parsedURI) fitsInt(from, to int) bool {
	start := from
	for start < to-1 && p.value[start] == '0' {
		start++
	}
	length := to - start
	return length < 10 || length == 10 && p.value[start:to] <= "2147483647"
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
