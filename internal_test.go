package raoh

import (
	"errors"
	"math"
	"testing"
)

func TestDoublesAreWrittenAsJavaWritesThem(t *testing.T) {
	for v, java := range map[float64]string{
		0:                       "0.0",
		1:                       "1.0",
		100:                     "100.0",
		0.5:                     "0.5",
		0.001:                   "0.001",
		0.0001:                  "1.0E-4",
		1234567:                 "1234567.0",
		1e7:                     "1.0E7",
		1.25e7:                  "1.25E7",
		-1.5e-5:                 "-1.5E-5",
		1e21:                    "1.0E21",
		123.456:                 "123.456",
		math.MaxFloat64:         "1.7976931348623157E308",
		2.2250738585072014e-308: "2.2250738585072014E-308",
	} {
		if got := doubleToString(v); got != java {
			t.Errorf("%v: %q, want %q", v, got, java)
		}
	}
	if got := doubleToString(math.Copysign(0, -1)); got != "-0.0" {
		t.Errorf("-0: %q", got)
	}
}

func TestPropertiesFollowPropertiesLoad(t *testing.T) {
	text := "# comment\n  ! also a comment\n" +
		"a=1\n" +
		"b : 2\n" +
		"c 3\n" +
		`d=必須` + "\n" +
		"e=one \\\n     two\n" +
		`f\=g=h\:i` + "\n" +
		`emoji=😀` + "\n" +
		"empty\n"
	pairs, err := loadProperties(text)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range pairs {
		got[p.key] = p.value
	}
	want := map[string]string{"a": "1", "b": "2", "c": "3", "d": "必須", "e": "one two",
		"f=g": "h:i", "emoji": "😀", "empty": ""}
	if len(pairs) != len(want) {
		t.Fatalf("%d pairs: %v", len(pairs), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
}

func TestAMalformedUnicodeEscapeIsAnError(t *testing.T) {
	_, err := loadProperties("ok=1\nbad=\\u12")
	pe, ok := errors.AsType[*PropertiesError](err)
	if !ok || pe.Line != 2 {
		t.Fatalf("got %v", err)
	}
}

func TestIPv4RejectsLeadingZerosAndLargeOctets(t *testing.T) {
	for _, ok := range []string{"192.168.0.1", "0.0.0.0"} {
		if !isIPv4(ok) {
			t.Errorf("%q", ok)
		}
	}
	for _, bad := range []string{"192.168.00.1", "256.0.0.1", "1.2.3", "1.2.3.4.", "1.2.3.4 ", "١.2.3.4"} {
		if isIPv4(bad) {
			t.Errorf("%q", bad)
		}
	}
}

func TestIPv6FollowsTheRFC4291TextForm(t *testing.T) {
	for _, ok := range []string{"::", "::1", "2001:db8::1", "1:2:3:4:5:6:7:8", "::ffff:1.2.3.4",
		"::1.2.3.4", "1:2:3:4:5:6:1.2.3.4", "ABCD::ef"} {
		if !isIPv6(ok) {
			t.Errorf("%q", ok)
		}
	}
	for _, bad := range []string{"[::1]", "1::2::3", "::00001", "::01.2.3.4", "1:2:3:4:5:6:7:8:9",
		"1:2:3:4:5:6:7", "1:2:3:4:5:6:7:1.2.3.4", "2001:db8::g", "1.2.3.4", ":1", "1:", ""} {
		if isIPv6(bad) {
			t.Errorf("%q", bad)
		}
	}
}

func TestAZoneIsTakenOnlyBelowGlobalScope(t *testing.T) {
	for _, ok := range []string{"fe80::1%eth0", "fe80::1%3", "febf::1%x", "ff02::1%en0", "ff15::1%a"} {
		if !isIPv6(ok) {
			t.Errorf("%q", ok)
		}
	}
	for _, bad := range []string{"::1%lo0", "2001:db8::1%eth0", "fec0::1%eth0", "ff0e::1%eth0",
		"ff00::1%eth0", "fe80::1%", "fe80::1%a%b", "fe80::1%a\x00"} {
		if isIPv6(bad) {
			t.Errorf("%q", bad)
		}
	}
}

func TestURLsFollowTheRFC3986Rule(t *testing.T) {
	url := func(s string) bool {
		p, ok := parseURI(s)
		return ok && p.representableAsJavaURI() && (p.schemeIs("http") || p.schemeIs("https")) && p.hasHost()
	}
	for _, ok := range []string{"https://example.com", "HTTP://example.com/a?b=c#d", "http://my_host.com",
		"http://user:pw@[::1]:8080/", "http://example.com:99999/", "http://a/%41"} {
		if !url(ok) {
			t.Errorf("%q", ok)
		}
	}
	for _, bad := range []string{"https://", "ftp://example.com", "https://日本.jp/", "http://[fe80::1%25eth0]/",
		"http://[v1.x]/", "http://a/%4", "http://a b/", "example.com", "http://[::1]:99999999999/"} {
		if url(bad) {
			t.Errorf("%q", bad)
		}
	}
}

func TestCaseMappingFollowsJava(t *testing.T) {
	for in, want := range map[string]string{"ΟΔΟΣ": "οδος", "İ": "i̇", "ΣΑ": "σα", "Σ": "σ", "ΑΣ ΒΣ": "ας βς"} {
		if got := toLowerJava(in); got != want {
			t.Errorf("lower %q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"ß": "SS", "ﬁ": "FI", "straße": "STRASSE"} {
		if got := toUpperJava(in); got != want {
			t.Errorf("upper %q: %q, want %q", in, got, want)
		}
	}
}

func TestDecimalsAreWrittenAsBigDecimalWritesThem(t *testing.T) {
	for in, want := range map[string]string{"1.20": "1.20", "0.0005": "0.0005", "1E+3": "1E+3",
		"1e3": "1E+3", "-12.5": "-12.5", "0.0000001": "1E-7", "12e-1": "1.2", "0": "0"} {
		d, err := ParseDecimal(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got := d.String(); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "-", "1.2.3", "1e", "abc", ".", "1e+-3"} {
		if _, err := ParseDecimal(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if MustDecimal("1.20").Cmp(MustDecimal("1.2")) != 0 || MustDecimal("1E+3").Cmp(MustDecimal("999.9")) <= 0 {
		t.Error("Cmp")
	}
}

func TestPathsAreJSONPointers(t *testing.T) {
	var root Path
	if root.String() != "" || !root.IsRoot() {
		t.Error("root")
	}
	if got := root.Key("items").Index(0).Key("name").String(); got != "/items/0/name" {
		t.Error(got)
	}
	if got := root.Key("a/b").Key("~c").String(); got != "/a~1b/~0c" {
		t.Error(got)
	}
	if got := root.Key("").String(); got != "/" {
		t.Error(got)
	}
	if got := root.Key("end").under(root.Key("period")).String(); got != "/period/end" {
		t.Error(got)
	}
}
