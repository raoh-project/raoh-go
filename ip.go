package raoh

import "strings"

// The text forms IPv4, IPv6 and IP accept, as Raoh for Java's IpSyntax defines
// them: decided by grammar, not by a parser of the platform, so the answer is
// the same everywhere.

func isDigit(b byte) bool    { return b >= '0' && b <= '9' }
func isHexDigit(b byte) bool { return isDigit(b) || b|0x20 >= 'a' && b|0x20 <= 'f' }

// isIPv4 reports whether s is a dotted quad: four decimal octets from 0 to
// 255, without leading zeros.
func isIPv4(s string) bool {
	pos := 0
	for octet := range 4 {
		if octet > 0 {
			if pos >= len(s) || s[pos] != '.' {
				return false
			}
			pos++
		}
		start := pos
		number := 0
		for pos < len(s) && pos-start < 3 && isDigit(s[pos]) {
			number = number*10 + int(s[pos]-'0')
			pos++
		}
		digits := pos - start
		if digits == 0 || number > 255 || digits > 1 && s[start] == '0' {
			return false
		}
	}
	return pos == len(s)
}

// isIPv6 reports whether s is an IPv6 address, optionally followed by a zone
// ID.
//
// The address is the RFC 4291 section 2.2 text form, RFC 3986's IPv6address:
// eight groups of one to four hexadecimal digits, at most one ::, and
// optionally a dotted quad in place of the last two groups. Brackets belong to
// the URI host syntax and are not accepted.
//
// A zone ID (fe80::1%eth0, RFC 4007) is accepted when the address's scope is
// below global: link-local unicast (fe80::/10) or multicast of scope 1 to D
// (RFC 4291 section 2.7 as updated by RFC 7346). The zone ID is any non-empty
// text without % or NUL; it is not looked up among the host's interfaces.
func isIPv6(s string) bool {
	address, zone, found := strings.Cut(s, "%")
	if !found {
		return isIPv6Address(s)
	}
	return zone != "" && !strings.ContainsAny(zone, "%\x00") &&
		isIPv6Address(address) && canHaveZone(firstGroup(address))
}

func isIPv6Address(s string) bool {
	to := len(s)
	groups := 0
	compressed := false
	pos := 0
	if strings.HasPrefix(s, "::") {
		compressed = true
		pos = 2
		if pos == to {
			return true
		}
	}
	for {
		start := pos
		for pos < to && pos-start < 4 && isHexDigit(s[pos]) {
			pos++
		}
		if pos < to && s[pos] == '.' {
			room := groups+2 == 8
			if compressed {
				room = groups+2 <= 7
			}
			return room && isIPv4(s[start:])
		}
		if pos == start {
			return false
		}
		groups++
		if pos == to {
			if compressed {
				return groups <= 7
			}
			return groups == 8
		}
		if s[pos] != ':' {
			return false
		}
		pos++
		if pos < to && s[pos] == ':' {
			if compressed {
				return false
			}
			compressed = true
			pos++
			if pos == to {
				return groups <= 7
			}
		}
		if groups > 8 {
			return false
		}
	}
}

// firstGroup returns the value of the first 16-bit group of an address
// isIPv6Address accepted.
func firstGroup(address string) int {
	group := 0
	for i := 0; i < len(address) && address[i] != ':'; i++ {
		c := address[i] | 0x20
		switch {
		case isDigit(address[i]):
			group = group*16 + int(address[i]-'0')
		case c >= 'a' && c <= 'f':
			group = group*16 + int(c-'a'+10)
		default:
			group *= 16
		}
	}
	return group
}

// canHaveZone reports whether a zone ID may follow an address with this first
// group: link-local unicast (fe80::/10, RFC 4291 section 2.5.6), or multicast
// whose scope, the low four bits of the second byte, is below global. Scope 0
// is reserved, E is global and F is reserved and treated as global. The
// loopback address ::1 does not take a zone.
func canHaveZone(group int) bool {
	first, second := group>>8, group&0xff
	if first == 0xfe && second&0xc0 == 0x80 {
		return true
	}
	scope := second & 0x0f
	return first == 0xff && scope != 0x0 && scope != 0xe && scope != 0xf
}
