package raoh

import (
	"encoding/json"
	"fmt"
	"time"

	notation199x "github.com/raoh-project/notation-199x/go"
	"github.com/raoh-project/raoh-go/internal/javatime"
)

// Which text is a date, a time, a date-time, a date-time with an offset or an
// instant is the grammar of notation-199x, which Raoh for Java follows too.
// Once a text is admitted, its value is read off it here: the grammar has
// already put each field where it is, so nothing here decides what is a
// temporal, and a text the grammar admits always has a value.

// admittedFields is what an admitted text writes, as numbers, read from left to
// right.
type admittedFields struct {
	text string
	at   int
}

// number reads the digits from where the reading is.
func (f *admittedFields) number() int {
	n := 0
	for f.at < len(f.text) && f.text[f.at] >= '0' && f.text[f.at] <= '9' {
		n = n*10 + int(f.text[f.at]-'0')
		f.at++
	}
	return n
}

// skip moves past c where it is there, and answers whether it was.
func (f *admittedFields) skip(c byte) bool {
	if f.at < len(f.text) && f.text[f.at] == c {
		f.at++
		return true
	}
	return false
}

// date reads a year, a month and a day. A year has a sign or none, and runs up
// to the hyphen before the month.
func (f *admittedFields) date() (int, time.Month, int) {
	sign := 1
	if f.skip('-') {
		sign = -1
	} else {
		f.skip('+')
	}
	year := sign * f.number()
	f.skip('-')
	month := time.Month(f.number())
	f.skip('-')
	return year, month, f.number()
}

// clock reads an hour, a minute, and the second and the fraction of one where
// they are written. An hour of 24 is left to time.Date, which makes it the
// start of the next day.
func (f *admittedFields) clock() (hour, minute, second, nano int) {
	hour = f.number()
	f.skip(':')
	minute = f.number()
	if f.skip(':') {
		second = f.number()
		if f.skip('.') {
			from := f.at
			nano = f.number()
			for range 9 - (f.at - from) {
				nano *= 10
			}
		}
	}
	return hour, minute, second, nano
}

// offset reads Z, or a sign and hh:mm[:ss], as a fixed zone.
func (f *admittedFields) offset() *time.Location {
	if f.skip('Z') {
		return time.UTC
	}
	sign := 1
	if f.skip('-') {
		sign = -1
	} else {
		f.skip('+')
	}
	seconds := f.number() * 3600
	f.skip(':')
	seconds += f.number() * 60
	if f.skip(':') {
		seconds += f.number()
	}
	return time.FixedZone("", sign*seconds)
}

// admitted reads the value of text where the grammar admits it as kind, with
// read, and is false where the grammar refuses it.
func admitted(kind notation199x.TemporalKind, text string, read func(f *admittedFields) time.Time) (time.Time, bool) {
	if notation199x.CheckTemporal(kind, text) != notation199x.Admitted {
		return time.Time{}, false
	}
	f := &admittedFields{text: text}
	t := read(f)
	if f.at != len(text) {
		panic(fmt.Sprintf("raoh: %q was admitted as %v and read only to byte %d", text, kind, f.at))
	}
	return t, true
}

func parseDate(s string) (time.Time, bool) {
	return admitted(notation199x.Date, s, func(f *admittedFields) time.Time {
		y, m, d := f.date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	})
}

func parseClock(s string) (time.Time, bool) {
	return admitted(notation199x.Time, s, func(f *admittedFields) time.Time {
		h, mi, sec, nano := f.clock()
		return time.Date(0, 1, 1, h, mi, sec, nano, time.UTC)
	})
}

func parseDateTime(s string) (time.Time, bool) {
	return admitted(notation199x.DateTime, s, func(f *admittedFields) time.Time {
		y, m, d := f.date()
		f.skip('T')
		h, mi, sec, nano := f.clock()
		return time.Date(y, m, d, h, mi, sec, nano, time.UTC)
	})
}

func parseOffsetDateTime(s string) (time.Time, bool) {
	return admitted(notation199x.OffsetDateTime, s, func(f *admittedFields) time.Time {
		y, m, d := f.date()
		f.skip('T')
		h, mi, sec, nano := f.clock()
		return time.Date(y, m, d, h, mi, sec, nano, f.offset())
	})
}

// parseInstant reads an instant: the offset is applied, so the value is the
// moment the text names, in UTC, and 24:00:00 is the start of the next day.
func parseInstant(s string) (time.Time, bool) {
	return admitted(notation199x.Instant, s, func(f *admittedFields) time.Time {
		y, m, d := f.date()
		f.skip('T')
		h, mi, sec, nano := f.clock()
		return time.Date(y, m, d, h, mi, sec, nano, f.offset()).UTC()
	})
}

// temporalKind is one of the java.time types a temporal decoder reads. Its
// zero value is none of them, and every operation on it refuses that, so a
// bound method of a zero TemporalDecoder cannot reach a nil function.
type temporalKind struct {
	ops *temporalOps
}

// temporalOps is what a kind does with a value. Every kind defined below
// provides every operation.
type temporalOps struct {
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

// require gives the operations of the kind, and refuses the zero kind. It is
// the only place that checks, and the operations below go through it.
func (k temporalKind) require() *temporalOps {
	if k.ops == nil {
		refuseZeroValue()
	}
	return k.ops
}

func (k temporalKind) normalize(t time.Time) time.Time { return k.require().normalize(t) }

func (k temporalKind) compare(a, b time.Time) int { return k.require().compare(a, b) }

func (k temporalKind) format(t time.Time) string { return k.require().format(t) }

func (k temporalKind) value(t time.Time) temporalValue {
	ops := k.require()
	return temporalValue{ops.format(t), ops.jsonFormat(t)}
}

func wallUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

var (
	instantKind = temporalKind{&temporalOps{parseInstant, KeyInvalidFormatInstant, javatime.Instant, javatime.Instant,
		func(t time.Time) time.Time { return t.UTC() }, time.Time.Compare}}
	dateKind = temporalKind{&temporalOps{parseDate, KeyInvalidFormatDate, javatime.Date, javatime.Date,
		func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) },
		time.Time.Compare}}
	clockKind = temporalKind{&temporalOps{parseClock, KeyInvalidFormatTime, javatime.Time, javatime.ISOTime,
		func(t time.Time) time.Time {
			return time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		}, time.Time.Compare}}
	dateTimeKind = temporalKind{&temporalOps{parseDateTime, KeyInvalidFormatDateTime, javatime.DateTime, javatime.ISODateTime, wallUTC,
		time.Time.Compare}}
	// Offset date-times are compared by the instant alone, so the same
	// instant at two offsets is neither before nor after the other. The value
	// keeps the offset it was written with.
	offsetDateTimeKind = temporalKind{&temporalOps{parseOffsetDateTime, KeyInvalidFormatOffsetDateTime,
		javatime.OffsetDateTime, javatime.ISOOffsetDateTime,
		func(t time.Time) time.Time { return t }, time.Time.Compare}}
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
	ops := kind.require()
	d := TemporalDecoder{str: str, kind: kind, s: s}
	strDecoder := str.build()
	d.Decoder = Decoder[any, time.Time]{func(in any, at Path) outcome[time.Time] {
		o := strDecoder.run(in, at)
		if o.failed() {
			return failAs[time.Time](o)
		}
		v, ok := ops.parse(o.value)
		if !ok {
			return invalid[time.Time](withCustom(NewIssue(CodeInvalidFormat).WithMessageKey(ops.key).At(at), s.baseMessage))
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
