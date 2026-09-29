package encode_test

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/encode"
)

type Email struct{ value string }

func (e Email) String() string { return e.value }

type User struct {
	email    Email
	age      int
	nickname *string
	note     raoh.Presence[string]
	joined   time.Time
	balance  raoh.Decimal
}

func (u User) Email() Email                { return u.email }
func (u User) Age() int                    { return u.age }
func (u User) Nickname() *string           { return u.nickname }
func (u User) Note() raoh.Presence[string] { return u.note }
func (u User) Joined() time.Time           { return u.joined }
func (u User) Balance() raoh.Decimal       { return u.balance }

var userEncoder = encode.Object(
	encode.Property("email", User.Email, encode.String().Contramap(Email.String)),
	encode.Property("age", User.Age, encode.Int()),
	encode.OptionalProperty("nickname", User.Nickname, encode.String()),
	encode.PresenceProperty("note", User.Note, encode.String()),
	encode.Property("joined", User.Joined, encode.Date()),
	encode.Property("balance", User.Balance, encode.Decimal()),
)

var userDecoder = raoh.Object(raoh.Fields().
	Field("email", raoh.String().Email().Map(func(s string) Email { return Email{s} })).
	Field("age", raoh.Int()).
	Field("nickname", raoh.Optional(raoh.String())).
	Field("note", raoh.PresenceOf(raoh.String())).
	Field("joined", raoh.String().Date()).
	Field("balance", raoh.DecimalNumber()),
).Strict().Map(func(e Email, a int, n *string, note raoh.Presence[string], j time.Time, b raoh.Decimal) User {
	return User{e, a, n, note, j, b}
})

func TestAnEncodedValueDecodesBackToTheSameValue(t *testing.T) {
	nick := "ken"
	for _, u := range []User{
		{Email{"a@example.com"}, 30, &nick, raoh.Present("hi"), time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), raoh.MustDecimal("1.20")},
		{Email{"b@example.com"}, 0, nil, raoh.Null[string](), time.Date(-1, 12, 31, 0, 0, 0, 0, time.UTC), raoh.MustDecimal("0")},
		{Email{"c@example.com"}, 7, nil, raoh.Absent[string](), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), raoh.MustDecimal("1E+3")},
	} {
		// Through the Go values as they are, and through JSON text.
		back, err := userDecoder.Decode(userEncoder.Encode(u))
		if err != nil {
			t.Fatalf("%+v: %v", u, err)
		}
		text, err := json.Marshal(userEncoder.Encode(u))
		if err != nil {
			t.Fatal(err)
		}
		fromJSON, err := raoh.DecodeJSON(text, userDecoder)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		for _, got := range []User{back, fromJSON} {
			if got.email != u.email || got.age != u.age || !got.joined.Equal(u.joined) ||
				got.balance.String() != u.balance.String() || !reflect.DeepEqual(got.note, u.note) ||
				(got.nickname == nil) != (u.nickname == nil) || (u.nickname != nil && *got.nickname != *u.nickname) {
				t.Errorf("got %+v, want %+v", got, u)
			}
		}
	}
}

func TestPropertiesWriteNullOrLeaveTheMemberOut(t *testing.T) {
	type row struct {
		a, b *int
		c    raoh.Presence[int]
	}
	enc := encode.Object(
		encode.NullableProperty("a", func(r row) *int { return r.a }, encode.Int()),
		encode.OptionalProperty("b", func(r row) *int { return r.b }, encode.Int()),
		encode.PresenceProperty("c", func(r row) raoh.Presence[int] { return r.c }, encode.Int()),
	)
	got := enc.Encode(row{c: raoh.Absent[int]()})
	if !reflect.DeepEqual(got, map[string]any{"a": nil}) {
		t.Errorf("%v", got)
	}
	one := 1
	got = enc.Encode(row{a: &one, b: &one, c: raoh.Null[int]()})
	if !reflect.DeepEqual(got, map[string]any{"a": 1, "b": 1, "c": nil}) {
		t.Errorf("%v", got)
	}
}

type Shape interface{ area() int }
type Square struct{ Side int }
type Rect struct{ W, H int }

func (s Square) area() int { return s.Side * s.Side }
func (r Rect) area() int   { return r.W * r.H }

var shapeEncoder = encode.Discriminate[Shape]("kind",
	encode.Variant("square", encode.Object(encode.Property("side", func(s Square) int { return s.Side }, encode.Int()))),
	encode.Variant("rect", encode.Object(
		encode.Property("w", func(r Rect) int { return r.W }, encode.Int()),
		encode.Property("h", func(r Rect) int { return r.H }, encode.Int()),
		encode.Property("kind", func(Rect) string { return "ignored" }, encode.String()),
	)),
)

func TestDiscriminateWritesTheTagOfTheValuesType(t *testing.T) {
	if got := shapeEncoder.Encode(Square{3}); !reflect.DeepEqual(got, map[string]any{"kind": "square", "side": 3}) {
		t.Errorf("%v", got)
	}
	// The tag wins over a member of the same name the variant writes.
	if got := shapeEncoder.Encode(Rect{2, 3}); got["kind"] != "rect" {
		t.Errorf("%v", got)
	}
	decode := raoh.Discriminate("kind",
		raoh.Variant("square", raoh.Object(raoh.Fields().Field("side", raoh.Int())).
			Map(func(s int) Shape { return Square{s} })),
		raoh.Variant("rect", raoh.Object(raoh.Fields().Field("w", raoh.Int()).Field("h", raoh.Int())).
			Map(func(w, h int) Shape { return Rect{w, h} })),
	)
	for _, s := range []Shape{Square{3}, Rect{2, 3}} {
		if back, err := decode.Decode(shapeEncoder.Encode(s)); err != nil || back != s {
			t.Errorf("%v: %v %v", s, back, err)
		}
	}
}

type Triangle struct{}

func (Triangle) area() int { return 0 }

func panics(f func()) (ok bool) {
	defer func() { ok = recover() != nil }()
	f()
	return false
}

func TestADefinitionThatDoesNotCoverAValuePanics(t *testing.T) {
	if !panics(func() { shapeEncoder.Encode(Triangle{}) }) {
		t.Error("unknown variant type")
	}
	sq := encode.Object(encode.Property("side", func(s Square) int { return s.Side }, encode.Int()))
	if !panics(func() { encode.Discriminate[Shape]("k", encode.Variant("a", sq), encode.Variant("b", sq)) }) {
		t.Error("duplicate type")
	}
	colors := map[string]int{"red": 1, "green": 2}
	if !panics(func() { encode.EnumOf(colors)(3) }) {
		t.Error("unknown enum value")
	}
	if !panics(func() { encode.EnumOf(map[string]int{"a": 1, "b": 1}) }) {
		t.Error("duplicate enum value")
	}
}

func TestEnumOfUsesTheMapTheDecoderUses(t *testing.T) {
	colors := map[string]int{"red": 1, "green": 2}
	name := encode.EnumOf(colors).Encode(2)
	if back, err := raoh.EnumOf(colors).Decode(name); name != "green" || err != nil || back != 2 {
		t.Errorf("%q %v %v", name, back, err)
	}
}

type Node struct {
	Value    int
	Children []Node
}

func nodeEncoder() encode.Encoder[Node, map[string]any] {
	return encode.Object(
		encode.Property("value", func(n Node) int { return n.Value }, encode.Int()),
		encode.Property("children", func(n Node) []Node { return n.Children }, encode.List(encode.Lazy(nodeEncoder))),
	)
}

func TestLazyEncodesRecursiveStructures(t *testing.T) {
	got := nodeEncoder().Encode(Node{1, []Node{{2, nil}}})
	want := map[string]any{"value": 1, "children": []any{map[string]any{"value": 2, "children": []any{}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v", got)
	}
}

func TestValueEncoders(t *testing.T) {
	at := time.Date(2024, 1, 15, 10, 30, 0, 500_000_000, time.FixedZone("", 9*3600))
	for name, pair := range map[string][2]string{
		"instant":          {encode.Instant()(at), "2024-01-15T01:30:00.500Z"},
		"date":             {encode.Date()(at), "2024-01-15"},
		"time":             {encode.Time()(at), "10:30:00.500"},
		"date_time":        {encode.DateTime()(at), "2024-01-15T10:30:00.500"},
		"offset_date_time": {encode.OffsetDateTime()(at), "2024-01-15T10:30:00.500+09:00"},
		"minute":           {encode.Time()(time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)), "09:00"},
		"negative year":    {encode.Date()(time.Date(-1, 12, 31, 0, 0, 0, 0, time.UTC)), "-0001-12-31"},
		"far year":         {encode.Date()(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)), "+10000-01-01"},
		"offset seconds":   {encode.OffsetDateTime()(time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", 5*3600+30*60+15))), "2024-01-15T10:30+05:30:15"},
		"negative offset":  {encode.OffsetDateTime()(time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", -3*3600))), "2024-01-15T10:30-03:00"},
		"uuid":             {encode.UUID()(raoh.UUID{0x12, 0x3e, 0x45, 0x67}), "123e4567-0000-0000-0000-000000000000"},
		"url":              {encode.URL()(&url.URL{Scheme: "https", Host: "example.com", Path: "/a"}), "https://example.com/a"},
		"decimal":          {string(encode.Decimal()(raoh.MustDecimal("1.20"))), "1.20"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s: %q, want %q", name, pair[0], pair[1])
		}
	}
	type Age int
	age := encode.Int().Contramap(func(a Age) int { return int(a) }).AndThen(func(n int) string { return "age " + string(rune('0'+n)) })
	if got := age.Encode(Age(7)); got != "age 7" {
		t.Error(got)
	}
	if got := encode.Dict(encode.Int()).Encode(map[string]int{"a": 1}); !reflect.DeepEqual(got, map[string]any{"a": 1}) {
		t.Error(got)
	}
}
