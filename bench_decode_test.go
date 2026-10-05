package raoh_test

// What decoding costs where an issue is made for every element or member, and on the
// paths text takes, for comparing one version with another with benchstat.

import (
	"strings"
	"testing"

	"github.com/raoh-project/raoh-go"
)

type person struct {
	name string
	age  int
}

var personD = raoh.Object(raoh.Fields().Field("name", raoh.String().Trim().MinLength(2)).Field("age", raoh.Int().Min(0).Max(150))).
	Map(func(n string, a int) person { return person{n, a} })

var okJSON = []byte(`{"name":"Kenshiro","age":27}`)
var badJSON = []byte(`{"name":"K","age":200}`)

func BenchmarkObjectOK(b *testing.B) {
	for range b.N {
		raoh.DecodeJSON(okJSON, personD)
	}
}

func BenchmarkObjectTwoIssues(b *testing.B) {
	for range b.N {
		raoh.DecodeJSON(badJSON, personD)
	}
}

var listBad = []byte("[" + strings.TrimSuffix(strings.Repeat(`"x",`, 1000), ",") + "]")

func BenchmarkList1000Failures(b *testing.B) {
	d := raoh.List(raoh.Int())
	for range b.N {
		raoh.DecodeJSON(listBad, d)
	}
}

var strictIn = func() []byte {
	var sb strings.Builder
	sb.WriteString(`{"a":1`)
	for i := range 1000 {
		sb.WriteString(`,"k` + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676)) + `":1`)
	}
	sb.WriteString("}")
	return []byte(sb.String())
}()

func BenchmarkStrict1000Unknown(b *testing.B) {
	d := raoh.Strict(raoh.Object(raoh.Fields().Field("a", raoh.Int())).Map(func(a int) int { return a }), "a")
	for range b.N {
		raoh.DecodeJSON(strictIn, d)
	}
}

func BenchmarkOneOfFails(b *testing.B) {
	d := raoh.OneOf[any](raoh.Int().Map(func(v int) any { return v }), raoh.Bool().Map(func(v bool) any { return v }))
	in := []byte(`"x"`)
	for range b.N {
		raoh.DecodeJSON(in, d)
	}
}

func BenchmarkInstant(b *testing.B) {
	d := raoh.String().Instant()
	in := []byte(`"2026-10-02T12:34:56.789+09:00"`)
	for range b.N {
		raoh.DecodeJSON(in, d)
	}
}

func BenchmarkLowerText(b *testing.B) {
	d := raoh.String().ToLower()
	in := []byte(`"` + strings.Repeat("Hello World ", 80) + `"`)
	for range b.N {
		raoh.DecodeJSON(in, d)
	}
}

func BenchmarkPattern(b *testing.B) {
	d := raoh.String().Pattern(`[a-z]+\d`)
	in := []byte(`"abcdefghij1"`)
	for range b.N {
		raoh.DecodeJSON(in, d)
	}
}

func BenchmarkRenderIssues(b *testing.B) {
	_, err := raoh.DecodeJSON(listBad, raoh.List(raoh.Int()))
	is := err.(*raoh.Issues)
	b.ResetTimer()
	for range b.N {
		is.Render(raoh.English)
	}
}

func BenchmarkALongString(b *testing.B) {
	d := raoh.String().MaxLength(100000)
	in := []byte(`"` + strings.Repeat("日本語テキスト", 2000) + `"`)
	for range b.N {
		raoh.DecodeJSON(in, d)
	}
}
