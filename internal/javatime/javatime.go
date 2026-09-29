// Package javatime writes a time.Time as the toString methods of Java's
// java.time types write the value of the same kind, so the text an issue or an
// encoder gives is the text Raoh for Java gives.
package javatime

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Date writes the date of t as LocalDate.toString does: yyyy-MM-dd, with a
// year outside 0000 to 9999 written with a sign.
func Date(t time.Time) string {
	return year(t.Year()) + fmt.Sprintf("-%02d-%02d", int(t.Month()), t.Day())
}

func year(y int) string {
	switch {
	case y > 9999:
		return "+" + strconv.Itoa(y)
	case y < 0:
		return fmt.Sprintf("-%04d", -y)
	}
	return fmt.Sprintf("%04d", y)
}

// Time writes the clock time of t as LocalTime.toString does: HH:mm, with the
// seconds when they or the nanoseconds are not zero, and a fraction of 3, 6 or
// 9 digits when the nanoseconds are not zero.
func Time(t time.Time) string {
	s := fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
	if t.Second() == 0 && t.Nanosecond() == 0 {
		return s
	}
	return s + fmt.Sprintf(":%02d", t.Second()) + fraction(t.Nanosecond())
}

func fraction(nano int) string {
	switch {
	case nano == 0:
		return ""
	case nano%1_000_000 == 0:
		return fmt.Sprintf(".%03d", nano/1_000_000)
	case nano%1_000 == 0:
		return fmt.Sprintf(".%06d", nano/1_000)
	}
	return fmt.Sprintf(".%09d", nano)
}

// DateTime writes the date and clock time of t as LocalDateTime.toString
// does.
func DateTime(t time.Time) string {
	return Date(t) + "T" + Time(t)
}

// OffsetDateTime writes t with its offset as OffsetDateTime.toString does:
// Z for a zero offset, otherwise ±HH:MM, with :SS when the seconds are not
// zero.
func OffsetDateTime(t time.Time) string {
	return DateTime(t) + Offset(t)
}

// Offset writes the offset of t as ZoneOffset.toString does.
func Offset(t time.Time) string {
	_, seconds := t.Zone()
	if seconds == 0 {
		return "Z"
	}
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	s := fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, seconds/60%60)
	if seconds%60 != 0 {
		s += fmt.Sprintf(":%02d", seconds%60)
	}
	return s
}

// Instant writes t as Instant.toString does: in UTC, always with the seconds,
// a fraction in groups of three digits when there is one, and Z.
func Instant(t time.Time) string {
	u := t.UTC()
	s := Date(u) + fmt.Sprintf("T%02d:%02d:%02d", u.Hour(), u.Minute(), u.Second())
	if nano := u.Nanosecond(); nano != 0 {
		digits := strings.TrimRight(fmt.Sprintf("%09d", nano), "0")
		for len(digits)%3 != 0 {
			digits += "0"
		}
		s += "." + digits
	}
	return s + "Z"
}

// The ISO formatters Jackson writes java.time values with differ from
// toString for the kinds with a clock time: they always write the seconds, and
// a fraction without trailing zeros.

// ISOTime writes the clock time of t as DateTimeFormatter.ISO_LOCAL_TIME does.
func ISOTime(t time.Time) string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
	if nano := t.Nanosecond(); nano != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%09d", nano), "0")
	}
	return s
}

// ISODateTime writes t as DateTimeFormatter.ISO_LOCAL_DATE_TIME does.
func ISODateTime(t time.Time) string {
	return Date(t) + "T" + ISOTime(t)
}

// ISOOffsetDateTime writes t as DateTimeFormatter.ISO_OFFSET_DATE_TIME does.
func ISOOffsetDateTime(t time.Time) string {
	return ISODateTime(t) + Offset(t)
}
