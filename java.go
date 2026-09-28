package raoh

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// What the operations Raoh for Java takes from the JDK mean, written once.
// Where a built-in decoder does what Raoh for Java does with a JDK method, it
// calls the function here rather than the Go function that looks like it:
// len counts UTF-8 bytes where String.length() counts UTF-16 units,
// strconv writes 1e+07 where Double.toString writes 1.0E7, and no Go package
// reads a .properties file as Properties.load does.

// utf16Len is String.length(): the number of UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// doubleToString is Double.toString(double): the shortest decimal that reads
// back as v, written plainly with at least one fraction digit when
// 10⁻³ ≤ |v| < 10⁷ (100.0, 0.001), and as d.dddE±n otherwise (1.0E7, 1.0E-4).
func doubleToString(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	sign := ""
	if math.Signbit(v) {
		sign = "-"
	}
	if v == 0 {
		return sign + "0.0"
	}
	// strconv writes the same shortest digits Java chooses, as d.ddde±nn.
	scientific := strconv.FormatFloat(math.Abs(v), 'e', -1, 64)
	mantissa, exp, _ := strings.Cut(scientific, "e")
	exponent, _ := strconv.Atoi(exp)
	digits := strings.Replace(mantissa, ".", "", 1)
	magnitude := math.Abs(v)
	var body string
	if magnitude >= 1e-3 && magnitude < 1e7 {
		// The decimal point goes after exponent + 1 digits.
		point := exponent + 1
		switch {
		case point <= 0:
			body = "0." + strings.Repeat("0", -point) + digits
		case point >= len(digits):
			body = digits + strings.Repeat("0", point-len(digits)) + ".0"
		default:
			body = digits[:point] + "." + digits[point:]
		}
	} else {
		rest := digits[1:]
		if rest == "" {
			rest = "0"
		}
		body = digits[:1] + "." + rest + "E" + strconv.Itoa(exponent)
	}
	return sign + body
}

// PropertiesError is where [ParseProperties] stopped reading.
type PropertiesError struct {
	// Line is the line the malformed entry starts on, counting from 1.
	Line   int
	reason string
}

func (e *PropertiesError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.reason)
}

type property struct{ key, value string }

// loadProperties is Properties.load(Reader): the key and value pairs of a
// .properties text, in order.
//
// A logical line continues onto the next when it ends in an odd number of
// backslashes, and the next line's leading white space is skipped. Lines that
// are blank or whose first character other than white space is # or ! are
// comments. A key ends at the first =, : or white space not escaped; white
// space around the separator is skipped. Both key and value undo the escapes
// \t, \n, \r, \f and \uXXXX, and a backslash before any other character stands
// for that character. A \u not followed by four hexadecimal digits is an
// error, as in the JDK.
func loadProperties(text string) ([]property, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	isSpace := func(r rune) bool { return r == ' ' || r == '\t' || r == '\f' }
	var pairs []property
	var logical strings.Builder
	start := 0
	continuing := false
	for index, line := range strings.Split(text, "\n") {
		if continuing {
			line = strings.TrimLeftFunc(line, isSpace)
		} else {
			content := strings.TrimLeftFunc(line, isSpace)
			if content == "" || content[0] == '#' || content[0] == '!' {
				continue
			}
			start = index + 1
			line = content
		}
		trailing := len(line) - len(strings.TrimRight(line, `\`))
		if trailing%2 == 1 {
			logical.WriteString(line[:len(line)-1])
			continuing = true
			continue
		}
		logical.WriteString(line)
		continuing = false
		p, err := splitEntry(logical.String(), start)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
		logical.Reset()
	}
	if continuing {
		p, err := splitEntry(logical.String(), start)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

func splitEntry(line string, number int) (property, error) {
	isSpace := func(r rune) bool { return r == ' ' || r == '\t' || r == '\f' }
	keyEnd := len(line)
	escaped := false
	for i, c := range line {
		if escaped {
			escaped = false
		} else if c == '\\' {
			escaped = true
		} else if c == '=' || c == ':' || isSpace(c) {
			keyEnd = i
			break
		}
	}
	rest := strings.TrimLeftFunc(line[keyEnd:], isSpace)
	if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ":") {
		rest = rest[1:]
	}
	rest = strings.TrimLeftFunc(rest, isSpace)
	key, err := unescape(line[:keyEnd], number)
	if err != nil {
		return property{}, err
	}
	value, err := unescape(rest, number)
	if err != nil {
		return property{}, err
	}
	return property{key, value}, nil
}

func unescape(raw string, number int) (string, error) {
	var units []uint16
	runes := []rune(raw)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c != '\\' {
			units = utf16.AppendRune(units, c)
			continue
		}
		i++
		if i >= len(runes) {
			break
		}
		switch runes[i] {
		case 't':
			units = append(units, '\t')
		case 'n':
			units = append(units, '\n')
		case 'r':
			units = append(units, '\r')
		case 'f':
			units = append(units, '\f')
		case 'u':
			end := min(i+5, len(runes))
			hex := string(runes[i+1 : end])
			unit, err := strconv.ParseUint(hex, 16, 16)
			if len(hex) != 4 || err != nil {
				return "", &PropertiesError{Line: number, reason: `malformed \uXXXX encoding`}
			}
			units = append(units, uint16(unit))
			i = end - 1
		default:
			units = utf16.AppendRune(units, runes[i])
		}
	}
	return string(utf16.Decode(units)), nil
}
