package raoh

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/kawasima/raoh-go/internal/javatime"
)

// The ISO 8601 text forms the temporal decoders accept, as Raoh for Java's
// TemporalText defines them. A year is written as LocalDate.toString writes it:
// four digits and no sign for 0000 to 9999; otherwise a sign, and no leading
// zero beyond the four-digit minimum. -0000 is not year 0.
const (
	yearPattern     = `(?P<year>[0-9]{4}|\+[1-9][0-9]{4,9}|-[0-9]{4}|-[1-9][0-9]{4,9})`
	datePattern     = yearPattern + `-(?P<month>[0-9]{2})-(?P<day>[0-9]{2})`
	hourMinute      = `(?P<hour>[0-9]{2}):(?P<minute>[0-9]{2})`
	secondPattern   = `:(?P<second>[0-9]{2})(?:\.(?P<fraction>[0-9]{1,9}))?`
	timePattern     = hourMinute + `(?:` + secondPattern + `)?`
	offsetPattern   = `(?:(?P<utc>Z)|(?P<offsetSign>[+-])(?P<offsetHour>[0-9]{2}):(?P<offsetMinute>[0-9]{2})(?::(?P<offsetSecond>[0-9]{2}))?)`
	maxYear         = 999_999_999
	maxInstantYear  = 1_000_000_000
	maxOffsetSecond = 18 * 3600
)

var (
	dateRE           = regexp.MustCompile(`^` + datePattern + `$`)
	timeRE           = regexp.MustCompile(`^` + timePattern + `$`)
	dateTimeRE       = regexp.MustCompile(`^` + datePattern + `T` + timePattern + `$`)
	offsetDateTimeRE = regexp.MustCompile(`^` + datePattern + `T` + timePattern + offsetPattern + `$`)
	// An instant requires the seconds.
	instantRE = regexp.MustCompile(`^` + datePattern + `T` + hourMinute + secondPattern + offsetPattern + `$`)

	instantMin = time.Date(-maxInstantYear, 1, 1, 0, 0, 0, 0, time.UTC)
	instantMax = time.Date(maxInstantYear, 12, 31, 23, 59, 59, 999_999_999, time.UTC)
)

// groups returns the named groups of re that s matches whole, or nil.
func groups(re *regexp.Regexp, s string) map[string]string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	g := map[string]string{}
	for i, name := range re.SubexpNames() {
		if name != "" {
			g[name] = m[i]
		}
	}
	return g
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// dateFields returns the date the groups name, if it exists and its year is
// within limit.
func dateFields(g map[string]string, limit int) (y int, m time.Month, d int, ok bool) {
	if g["year"] == "-0000" {
		return 0, 0, 0, false
	}
	year, err := strconv.ParseInt(g["year"], 10, 64)
	if err != nil || year < -int64(limit) || year > int64(limit) {
		return 0, 0, 0, false
	}
	month, day := atoi(g["month"]), atoi(g["day"])
	if month < 1 || month > 12 || day < 1 || day > time.Date(int(year), time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return 0, 0, 0, false
	}
	return int(year), time.Month(month), day, true
}

// clockFields returns the clock time the groups name. hour may be 24 only
// when allow24 holds; the caller decides what it means.
func clockFields(g map[string]string, allow24 bool) (h, mi, s, nano int, ok bool) {
	h, mi, s = atoi(g["hour"]), atoi(g["minute"]), atoi(g["second"])
	if f := g["fraction"]; f != "" {
		nano = atoi(f)
		for i := len(f); i < 9; i++ {
			nano *= 10
		}
	}
	if mi > 59 || s > 59 {
		return 0, 0, 0, 0, false
	}
	if h == 24 && allow24 && mi == 0 && s == 0 && nano == 0 {
		return h, mi, s, nano, true
	}
	return h, mi, s, nano, h <= 23
}

// offsetFields returns the offset the groups name, as ZoneOffset allows it:
// at most 18 hours, and minutes and seconds below 60.
func offsetFields(g map[string]string) (*time.Location, bool) {
	if g["utc"] == "Z" {
		return time.UTC, true
	}
	h, m, s := atoi(g["offsetHour"]), atoi(g["offsetMinute"]), atoi(g["offsetSecond"])
	seconds := h*3600 + m*60 + s
	if h > 18 || m > 59 || s > 59 || seconds > maxOffsetSecond {
		return nil, false
	}
	if g["offsetSign"] == "-" {
		seconds = -seconds
	}
	return time.FixedZone("", seconds), true
}

func parseDate(s string) (time.Time, bool) {
	g := groups(dateRE, s)
	if g == nil {
		return time.Time{}, false
	}
	y, m, d, ok := dateFields(g, maxYear)
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), ok
}

func parseClock(s string) (time.Time, bool) {
	g := groups(timeRE, s)
	if g == nil {
		return time.Time{}, false
	}
	h, mi, sec, nano, ok := clockFields(g, false)
	return time.Date(0, 1, 1, h, mi, sec, nano, time.UTC), ok
}

func dateTimeIn(g map[string]string, loc *time.Location) (time.Time, bool) {
	y, m, d, ok := dateFields(g, maxYear)
	if !ok {
		return time.Time{}, false
	}
	h, mi, sec, nano, ok := clockFields(g, false)
	return time.Date(y, m, d, h, mi, sec, nano, loc), ok
}

func parseDateTime(s string) (time.Time, bool) {
	g := groups(dateTimeRE, s)
	if g == nil {
		return time.Time{}, false
	}
	return dateTimeIn(g, time.UTC)
}

func parseOffsetDateTime(s string) (time.Time, bool) {
	g := groups(offsetDateTimeRE, s)
	if g == nil {
		return time.Time{}, false
	}
	loc, ok := offsetFields(g)
	if !ok {
		return time.Time{}, false
	}
	return dateTimeIn(g, loc)
}

// parseInstant reads an instant as Java's ISO_INSTANT does once the grammar
// has matched: the offset is applied, 24:00:00 is the start of the next day,
// a second of 60 is refused because Java would read it as 59, and the moment
// must be within the range of Instant.
func parseInstant(s string) (time.Time, bool) {
	g := groups(instantRE, s)
	if g == nil {
		return time.Time{}, false
	}
	y, m, d, ok := dateFields(g, maxInstantYear)
	if !ok {
		return time.Time{}, false
	}
	h, mi, sec, nano, ok := clockFields(g, true)
	if !ok {
		return time.Time{}, false
	}
	loc, ok := offsetFields(g)
	if !ok {
		return time.Time{}, false
	}
	t := time.Date(y, m, d, h, mi, sec, nano, loc).UTC()
	return t, !t.Before(instantMin) && !t.After(instantMax)
}

// temporalKind is one of the java.time types a temporal decoder reads.
type temporalKind struct {
	parse func(string) (time.Time, bool)
	key   string
	// format writes a value as the Java type's toString does, and jsonFormat
	// as Jackson writes it.
	format     func(time.Time) string
	jsonFormat func(time.Time) string
	// normalize turns a bound into a value of this kind, so that it compares
	// with decoded values by the fields the kind has.
	normalize func(time.Time) time.Time
	// compare orders two values of this kind as the Java type's compareTo does.
	compare func(a, b time.Time) int
}

// temporalValue is a bound or a value in an issue's metadata. A message writes
// it as the Java type's toString does, and JSON as Jackson writes it, as Raoh
// for Java gives them.
type temporalValue struct {
	text, json string
}

func (v temporalValue) String() string { return v.text }

func (v temporalValue) MarshalJSON() ([]byte, error) { return json.Marshal(v.json) }

func (k temporalKind) value(t time.Time) temporalValue {
	return temporalValue{k.format(t), k.jsonFormat(t)}
}

func wallUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

var (
	instantKind = temporalKind{parseInstant, KeyInvalidFormatInstant, javatime.Instant, javatime.Instant,
		func(t time.Time) time.Time { return t.UTC() }, time.Time.Compare}
	dateKind = temporalKind{parseDate, KeyInvalidFormatDate, javatime.Date, javatime.Date,
		func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) },
		time.Time.Compare}
	clockKind = temporalKind{parseClock, KeyInvalidFormatTime, javatime.Time, javatime.ISOTime,
		func(t time.Time) time.Time {
			return time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		}, time.Time.Compare}
	dateTimeKind = temporalKind{parseDateTime, KeyInvalidFormatDateTime, javatime.DateTime, javatime.ISODateTime, wallUTC,
		time.Time.Compare}
	// OffsetDateTime.compareTo orders by the instant, then by the local
	// date-time, so the same instant at two offsets is not equal.
	offsetDateTimeKind = temporalKind{parseOffsetDateTime, KeyInvalidFormatOffsetDateTime,
		javatime.OffsetDateTime, javatime.ISOOffsetDateTime,
		func(t time.Time) time.Time { return t },
		func(a, b time.Time) int {
			if c := a.Compare(b); c != 0 {
				return c
			}
			return wallUTC(a).Compare(wallUTC(b))
		}}
)

// TemporalDecoder decodes a string into a time.Time as one of Java's
// java.time types reads it, and checks it against bounds.
//
// The kinds without an offset give a time.Time in UTC, as time.Parse does for
// text without one: a date is its midnight, and a clock time is on January 1
// of year 0. An offset date-time keeps its offset as a fixed zone, and an
// instant is given in UTC. Bounds are compared by the fields the kind has, so
// a date bound is compared by its date alone. Bounds appear in issues and
// messages as the Java type's toString writes them.
type TemporalDecoder struct {
	Decoder[any, time.Time]
	str  scalar[string]
	kind temporalKind
	s    scalar[time.Time]
}

func newTemporal(str scalar[string], kind temporalKind, s scalar[time.Time]) TemporalDecoder {
	d := TemporalDecoder{str: str, kind: kind, s: s}
	strDecoder := str.build()
	d.Decoder = Decoder[any, time.Time]{func(in any, at Path) outcome[time.Time] {
		o := strDecoder.run(in, at)
		if o.failed() {
			return failAs[time.Time](o)
		}
		v, ok := kind.parse(o.value)
		if !ok {
			return invalid[time.Time](withCustom(NewIssue(CodeInvalidFormat).WithMessageKey(kind.key).At(at), s.baseMessage))
		}
		return s.run(v, at)
	}}
	return d
}

// Instant returns a decoder that reads the string as an ISO 8601 instant:
// yyyy-MM-ddTHH:mm:ss, an optional fraction of 1 to 9 digits, and an offset
// Z, ±HH:mm or ±HH:mm:ss, such as 2024-01-15T10:30:00Z. The seconds are
// required and T and Z are upper case only. The offset is applied, so the
// result is the moment the text names, in UTC. A clock time with second 60 is
// rejected, and 24:00:00 is the start of the next day. Otherwise it reports
// invalid_format under invalid_format.instant.
func (d StringDecoder) Instant() TemporalDecoder {
	return newTemporal(d.s, instantKind, scalar[time.Time]{})
}

// Date returns a decoder that reads the string as a local date, yyyy-MM-dd,
// such as 2024-01-15. A year outside 0000 to 9999 takes a sign and no leading
// zeros beyond four digits (+10000, -0001), so +2024 and -0000 are rejected,
// and so is a date that does not exist, such as 2023-02-29. Otherwise it
// reports invalid_format under invalid_format.date.
func (d StringDecoder) Date() TemporalDecoder { return newTemporal(d.s, dateKind, scalar[time.Time]{}) }

// Time returns a decoder that reads the string as a local time: HH:mm,
// HH:mm:ss, or HH:mm:ss and a fraction of 1 to 9 digits, such as 10:30.
// Hours run from 00 to 23; 24:00 and second 60 are rejected. Otherwise it
// reports invalid_format under invalid_format.time.
func (d StringDecoder) Time() TemporalDecoder {
	return newTemporal(d.s, clockKind, scalar[time.Time]{})
}

// DateTime returns a decoder that reads the string as a local date-time: a
// date as Date reads it, an upper-case T, and a time as Time reads it, such as
// 2024-01-15T10:30. Otherwise it reports invalid_format under
// invalid_format.date_time.
func (d StringDecoder) DateTime() TemporalDecoder {
	return newTemporal(d.s, dateTimeKind, scalar[time.Time]{})
}

// OffsetDateTime returns a decoder that reads the string as a date-time with
// an offset: a date-time as DateTime reads it and an offset Z, ±HH:mm or
// ±HH:mm:ss of at most 18 hours, such as 2024-01-15T10:30:00+09:00. Otherwise
// it reports invalid_format under invalid_format.offset_date_time.
func (d StringDecoder) OffsetDateTime() TemporalDecoder {
	return newTemporal(d.s, offsetDateTimeKind, scalar[time.Time]{})
}

func (d TemporalDecoder) require(ok func(time.Time) bool, fail func(time.Time) Issue) TemporalDecoder {
	return newTemporal(d.str, d.kind, d.s.require(ok, fail))
}

// Message gives the most recent constraint written before it, or the issue of
// a string that does not parse when there is none, a custom message that every
// language shows as written.
func (d TemporalDecoder) Message(message string) TemporalDecoder {
	return newTemporal(d.str, d.kind, d.s.message(message))
}

// Before requires a value before bound: out_of_range under
// out_of_range.before, with before and actual.
func (d TemporalDecoder) Before(bound time.Time) TemporalDecoder {
	b := d.kind.normalize(bound)
	return d.require(func(v time.Time) bool { return d.kind.compare(v, b) < 0 }, func(v time.Time) Issue {
		return outOfRange(KeyOutOfRangeBefore).WithMeta("before", d.kind.value(b)).WithMeta("actual", d.kind.value(v))
	})
}

// After requires a value after bound: out_of_range under out_of_range.after,
// with after and actual.
func (d TemporalDecoder) After(bound time.Time) TemporalDecoder {
	b := d.kind.normalize(bound)
	return d.require(func(v time.Time) bool { return d.kind.compare(v, b) > 0 }, func(v time.Time) Issue {
		return outOfRange(KeyOutOfRangeAfter).WithMeta("after", d.kind.value(b)).WithMeta("actual", d.kind.value(v))
	})
}

// Between requires a value from from to to, both included: out_of_range under
// out_of_range.between, with from, to and actual. It panics when from is after
// to.
func (d TemporalDecoder) Between(from, to time.Time) TemporalDecoder {
	f, t := d.kind.normalize(from), d.kind.normalize(to)
	if d.kind.compare(f, t) > 0 {
		panic(fmt.Sprintf("raoh: from (%s) must not be greater than to (%s)", d.kind.format(f), d.kind.format(t)))
	}
	return d.require(func(v time.Time) bool { return d.kind.compare(v, f) >= 0 && d.kind.compare(v, t) <= 0 },
		func(v time.Time) Issue {
			return outOfRange(KeyOutOfRangeBetween).WithMeta("from", d.kind.value(f)).
				WithMeta("to", d.kind.value(t)).WithMeta("actual", d.kind.value(v))
		})
}
