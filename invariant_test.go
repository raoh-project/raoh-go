package raoh_test

// The invariants the review of the first version found missing, each held
// where it can be broken from outside.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/encode"
)

// allocated returns the bytes f allocates.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// Untrusted input costs memory in proportion to its length, not to the value
// a number in it writes: no constraint builds 10^exponent.
func TestANumbersValueDoesNotDecideWhatDecodingItCosts(t *testing.T) {
	decoders := map[string]raoh.DecoderOf[raoh.Decimal]{
		"min":         raoh.DecimalNumber().Min(raoh.MustDecimal("0.5")),
		"max":         raoh.DecimalNumber().Max(raoh.MustDecimal("0.5")),
		"range":       raoh.DecimalNumber().Range(raoh.MustDecimal("0"), raoh.MustDecimal("10")),
		"positive":    raoh.DecimalNumber().Positive(),
		"multiple_of": raoh.DecimalNumber().MultipleOf(raoh.MustDecimal("0.25")),
		"scale":       raoh.DecimalNumber().Scale(2),
	}
	for _, text := range []string{"1e-2000000000", "1e+2000000000", "-7e2147483647", "3e-2147483648"} {
		for name, d := range decoders {
			if n := allocated(func() { raoh.DecodeJSON([]byte(text), d) }); n > 64<<10 {
				t.Errorf("%s %s: %d bytes", name, text, n)
			}
		}
	}
	long := bytes.Repeat([]byte("7"), 1_000_000)
	for name, d := range map[string]func(){
		"int":         func() { raoh.DecodeJSON(long, raoh.Int64()) },
		"multiple_of": func() { raoh.DecodeJSON(long, raoh.DecimalNumber().MultipleOf(raoh.MustDecimal("3"))) },
		"min":         func() { raoh.DecodeJSON(long, raoh.DecimalNumber().Min(raoh.MustDecimal("1"))) },
	} {
		// The text itself and a few copies of it, not a power of it.
		if n := allocated(d); n > 8*uint64(len(long)) {
			t.Errorf("%s of %d digits: %d bytes", name, len(long), n)
		}
	}
	_, issues := decodeJSON(t, raoh.Int64(), string(long))
	if len(issues) != 1 || issues[0].MessageKey() != raoh.KeyTypeMismatchNumericRange {
		t.Errorf("a long integer is out of range: %v", issues)
	}
	if _, issues := decodeJSON(t, raoh.DecimalNumber(), "1e-2147483649"); len(issues) != 1 ||
		issues[0].Code() != raoh.CodeTypeMismatch {
		t.Errorf("a scale beyond an int32 is refused: %v", issues)
	}
}

// ratOf reads a decimal text exactly, for the small exponents the comparison
// uses.
func ratOf(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return r
}

func randomDecimal(r *rand.Rand) string {
	var b strings.Builder
	if r.IntN(3) == 0 {
		b.WriteByte('-')
	}
	for range r.IntN(4) + 1 {
		b.WriteByte(byte('0' + r.IntN(10)))
	}
	if r.IntN(2) == 0 {
		b.WriteByte('.')
		for range r.IntN(4) + 1 {
			b.WriteByte(byte('0' + r.IntN(10)))
		}
	}
	if r.IntN(2) == 0 {
		fmt.Fprintf(&b, "e%d", r.IntN(13)-6)
	}
	return b.String()
}

// Cmp and MultipleOf agree with exact rational arithmetic.
func TestDecimalArithmeticAgreesWithRationals(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		a, b := randomDecimal(r), randomDecimal(r)
		da, db := raoh.MustDecimal(a), raoh.MustDecimal(b)
		if got, want := da.Cmp(db), ratOf(a).Cmp(ratOf(b)); got != want {
			t.Fatalf("Cmp(%s, %s) = %d, want %d", a, b, got, want)
		}
		if db.Sign() == 0 {
			continue
		}
		q := new(big.Rat).Quo(ratOf(a), ratOf(b))
		_, issues := decodeJSON(t, raoh.DecimalNumber().MultipleOf(db), jsonNumber(a))
		if got, want := issues == nil, q.IsInt(); got != want {
			t.Fatalf("%s multiple of %s: %v, want %v", a, b, got, want)
		}
	}
}

// jsonNumber writes a decimal text as JSON writes a number.
func jsonNumber(s string) string {
	d := raoh.MustDecimal(s)
	b, _ := d.MarshalJSON()
	return string(b)
}

func TestDecimalsFarApartCompareWithoutTheirPowers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1e-2000000000", "0", 1},
		{"1e-2000000000", "1e-1999999999", -1},
		{"-1e+2000000000", "1", -1},
		{"10e5", "1e6", 0},
		{"0.000", "0e+9", 0},
		{"12345678901234567890e-10", "1234567890.1234567890", 0},
	} {
		if got := raoh.MustDecimal(c.a).Cmp(raoh.MustDecimal(c.b)); got != c.want {
			t.Errorf("Cmp(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// A decoder written with NewDecoder is handed the public input model whether
// the input came from DecodeJSON or from values the caller built.
func TestACustomDecoderSeesOnlyPublicValues(t *testing.T) {
	type seen struct {
		keys   []string
		number json.Number
		absent bool
	}
	custom := raoh.NewDecoder(func(in any) (seen, error) {
		o, ok := raoh.AsObject(in)
		if !ok {
			return seen{}, raoh.Invalid(raoh.NewIssue("not_an_object"))
		}
		n, _ := o.Get("n")
		nu, _ := n.(json.Number)
		return seen{o.Keys(), nu, false}, nil
	})
	field := raoh.NewDecoder(func(in any) (bool, error) { return raoh.IsMissing(in), nil })
	d := raoh.Object(raoh.Fields().Field("v", custom).Field("gone", field)).
		Map(func(s seen, absent bool) seen { s.absent = absent; return s })

	v, _ := decodeJSON(t, d, `{"v": {"z": 1, "n": 12345678901234567890}}`)
	if strings.Join(v.keys, ",") != "z,n" || v.number != "12345678901234567890" || !v.absent {
		t.Errorf("from JSON: %+v", v)
	}
	v, err := d.Decode(map[string]any{"v": map[string]any{"z": 1, "n": json.Number("5")}})
	if err != nil || strings.Join(v.keys, ",") != "n,z" || v.number != "5" || !v.absent {
		t.Errorf("from Go values: %+v %v", v, err)
	}
}

// DecodeJSON gives only the values the input model names.
func TestDecodeJSONGivesOnlyTheInputModel(t *testing.T) {
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case nil, bool, string, json.Number:
		case []any:
			for _, e := range x {
				walk(e)
			}
		case *raoh.JSONObject:
			for _, e := range x.All() {
				walk(e)
			}
		default:
			t.Errorf("%T is not in the input model", v)
		}
	}
	raw := raoh.NewDecoder(func(in any) (any, error) { return in, nil })
	v, err := raoh.DecodeJSON([]byte(`{"a": [1, 2.5, "s", true, null, {"b": -0}], "c": {}}`), raw)
	if err != nil {
		t.Fatal(err)
	}
	walk(v)
}

func TestDecodeJSONFromReadsAtMostItsLimit(t *testing.T) {
	body := `{"n": 1}`
	d := raoh.Object(raoh.Fields().Field("n", raoh.Int())).Map(func(n int) int { return n })
	if n, err := raoh.DecodeJSONFrom(strings.NewReader(body), int64(len(body)), d); err != nil || n != 1 {
		t.Fatalf("%v %v", n, err)
	}
	_, err := raoh.DecodeJSONFrom(strings.NewReader(body), int64(len(body)-1), d)
	if !errors.Is(err, raoh.ErrInputTooLarge) {
		t.Fatalf("%v", err)
	}
	if _, ok := errors.AsType[*raoh.Issues](err); ok {
		t.Error("input over the limit is not invalid input")
	}
}

// No input makes a decode panic; a definition that could is refused when it
// is built.
func TestUniqueIsRefusedForTypesWhoseValuesMayNotCompare(t *testing.T) {
	anyList := raoh.List(raoh.OneOf[any](raoh.List(raoh.Int()).Map(func(v []int) any { return v })))
	if !panics(func() { anyList.Unique() }) {
		t.Error("Unique on []any")
	}
	type pair struct {
		a int
		b any
	}
	pairs := raoh.List(raoh.Int().Map(func(n int) pair { return pair{n, nil} }))
	if !panics(func() { pairs.Unique() }) {
		t.Error("Unique on a struct holding an interface")
	}
	if !panics(func() { encode.EnumOf(map[string]any{"a": 1}) }) {
		t.Error("encode.EnumOf on any")
	}
	byLength := anyList.UniqueBy(func(v any) int { return len(v.([]int)) })
	_, issues := decodeJSON(t, byLength, `[[1], [2], [1, 2]]`)
	if len(issues) != 1 || issues[0].Code() != raoh.CodeDuplicateElement {
		t.Errorf("%v", issues)
	}
	if !panics(func() { anyList.UniqueBy(func(v any) any { return v }) }) {
		t.Error("UniqueBy with an interface key")
	}
}

func TestContainsAndToSetAreRefusedForTypesWhoseValuesMayNotCompare(t *testing.T) {
	anyList := raoh.List(raoh.OneOf[any](raoh.List(raoh.Int()).Map(func(v []int) any { return v })))
	if !panics(func() { anyList.Contains(1) }) {
		t.Error("Contains on []any")
	}
	if !panics(func() { anyList.ContainsAll(1) }) {
		t.Error("ContainsAll on []any")
	}
	if !panics(func() { raoh.ToSet(anyList) }) {
		t.Error("ToSet on []any")
	}
	if !panics(func() { raoh.List(raoh.Int()).ContainsAll() }) {
		t.Error("ContainsAll without elements")
	}
}

// Raoh for Java refuses a null element of contains and containsAll when the
// decoder is built; a nil pointer is the null of a Go element.
func TestContainsRefusesANilElementWhenBuilt(t *testing.T) {
	l := raoh.List(raoh.Nullable(raoh.Int()))
	one := 1
	if !panics(func() { l.Contains(nil) }) {
		t.Error("Contains(nil)")
	}
	if !panics(func() { l.ContainsAll(&one, nil) }) {
		t.Error("ContainsAll with a nil")
	}
	if panics(func() { l.Contains(&one) }) {
		t.Error("Contains of a pointer")
	}
	pointers := raoh.List(raoh.Int().Map(func(int) unsafe.Pointer { return nil }))
	if !panics(func() { pointers.Contains(nil) }) {
		t.Error("Contains of a nil unsafe.Pointer")
	}
	if !panics(func() { raoh.List(raoh.Int()).UniqueBy[int](nil) }) {
		t.Error("UniqueBy(nil)")
	}
}

// A function or decoder a decode would call is refused when the decoder is
// built, with a message that names the constructor and the argument.
func TestANilArgumentIsRefusedWhenTheDecoderIsBuilt(t *testing.T) {
	d := raoh.Int()
	object := raoh.Object(raoh.Fields().Field("a", d))
	inner := object.Map(func(a int) int { return a })
	flat := raoh.Object(raoh.Fields().Flat(inner))
	empty := raoh.Object(raoh.Fields())
	tests := []struct {
		build func()
		want  string
	}{
		{func() { raoh.NewDecoder[any, int](nil) }, "raoh: NewDecoder needs f that is not nil"},
		{func() { raoh.NewDecoderWithPath[any, int](nil) }, "raoh: NewDecoderWithPath needs f that is not nil"},
		{func() { raoh.Lazy[int](nil) }, "raoh: Lazy needs f that is not nil"},
		{func() { d.Map[int](nil) }, "raoh: Decoder.Map needs f that is not nil"},
		{func() { d.AndThen[int](nil) }, "raoh: Decoder.AndThen needs f that is not nil"},
		{func() { d.AndThenWithPath[int](nil) }, "raoh: Decoder.AndThenWithPath needs f that is not nil"},
		{func() { d.Refine(nil, "c", "m") }, "raoh: Decoder.Refine needs ok that is not nil"},
		{func() { d.RefineWithMeta(nil, "c", "m", func(int) map[string]any { return nil }) }, "raoh: Decoder.RefineWithMeta needs ok that is not nil"},
		{func() { d.RefineWithMeta(func(int) bool { return true }, "c", "m", nil) }, "raoh: Decoder.RefineWithMeta needs meta that is not nil"},
		{func() { d.RefineWithPath(nil) }, "raoh: Decoder.RefineWithPath needs check that is not nil"},
		{func() { d.DefaultFunc(nil) }, "raoh: Decoder.DefaultFunc needs f that is not nil"},
		{func() { d.FallbackFunc(nil) }, "raoh: Decoder.FallbackFunc needs f that is not nil"},
		{func() { raoh.List(d).UniqueBy[int](nil) }, "raoh: UniqueBy needs key that is not nil"},
		{func() { object.Map[int](nil) }, "raoh: Object.Map needs fn that is not nil"},
		{func() { object.AndThen[int](nil) }, "raoh: Object.AndThen needs fn that is not nil"},
		{func() { object.AndThenWithPath[int](nil) }, "raoh: Object.AndThenWithPath needs fn that is not nil"},
		{func() { flat.Map[int](nil) }, "raoh: Object.Map needs fn that is not nil"},
		{func() { empty.Map[int](nil) }, "raoh: Object.Map needs fn that is not nil"},
		{func() { d.Pipe(raoh.Decoder[int, int]{}) }, "raoh: Decoder.Pipe needs a next decoder that is not the zero Decoder"},
		{func() { raoh.List[int](nil) }, "raoh: List needs element that is not nil"},
		{func() { raoh.Dict[int](nil) }, "raoh: Dict needs value that is not nil"},
		{func() { raoh.ToSet[int](nil) }, "raoh: ToSet needs d that is not nil"},
		{func() { raoh.Nullable[int](nil) }, "raoh: Nullable needs d that is not nil"},
		{func() { raoh.Optional[int](nil) }, "raoh: Optional needs d that is not nil"},
		{func() { raoh.PresenceOf[int](nil) }, "raoh: PresenceOf needs d that is not nil"},
		{func() { raoh.OneOf[int](d, nil) }, "raoh: OneOf needs each alternative that is not nil"},
		{func() { raoh.Strict[int](nil) }, "raoh: Strict needs d that is not nil"},
		{func() { raoh.Variant[int]("t", nil) }, "raoh: Variant needs d that is not nil"},
		{func() { raoh.EnumOfWith(map[string]int{"a": 1}, nil) }, "raoh: EnumOfWith needs stringDecoder that is not nil"},
		{func() { raoh.LiteralWith("a", nil) }, "raoh: LiteralWith needs stringDecoder that is not nil"},
		{func() { raoh.DiscriminateWith[int]("k", nil, nil) }, "raoh: DiscriminateWith needs tagDecoder that is not nil"},
		{func() { raoh.DiscriminateWith("k", raoh.String(), map[string]raoh.DecoderOf[int]{"a": nil}) }, "raoh: DiscriminateWith needs each variant that is not nil"},
		{func() { raoh.DecodeJSONFrom[int](strings.NewReader(""), 1, nil) }, "raoh: DecodeJSONFrom needs d that is not nil"},
		{func() { raoh.DecodeJSON[int](nil, nil) }, "raoh: DecodeJSON needs d that is not nil"},
		{func() { raoh.Fields().Field[int]("a", nil) }, "raoh: Field needs src that is not nil"},
		{func() { raoh.Fields().Flat[int](nil) }, "raoh: Flat needs d that is not nil"},
	}
	for _, tt := range tests {
		if got := panicText(tt.build); got != tt.want {
			t.Errorf("panic = %q, want %q", got, tt.want)
		}
	}
}

// The zero Decoder has nothing to run, so a decoder built from it, as an
// argument or as the receiver of a combinator, is refused, and so is a decode
// with it.
func TestTheZeroDecoderIsRefusedWhenAnotherIsBuiltFromIt(t *testing.T) {
	var zero raoh.Decoder[any, int]
	built := "raoh: a decoder was built from the zero Decoder, which has nothing to run"
	receiver := func(method string) string {
		return "raoh: Decoder." + method + " needs a receiver that is not the zero Decoder"
	}
	tests := []struct {
		name  string
		build func()
		want  string
	}{
		{"List", func() { raoh.List(zero) }, built},
		{"Dict", func() { raoh.Dict(zero) }, built},
		{"Nullable", func() { raoh.Nullable(zero) }, built},
		{"Optional", func() { raoh.Optional(zero) }, built},
		{"PresenceOf", func() { raoh.PresenceOf(zero) }, built},
		{"OneOf", func() { raoh.OneOf[int](zero) }, built},
		{"Field", func() { raoh.Fields().Field("a", zero) }, "raoh: Field needs src that is not nil"},
		{"Field of Optional", func() { raoh.Fields().Field("a", raoh.OptionalSource[int]{}) }, "raoh: Field needs src that is not nil"},
		{"Field of PresenceOf", func() { raoh.Fields().Field("a", raoh.PresenceSource[int]{}) }, "raoh: Field needs src that is not nil"},
		{"Map", func() { zero.Map(func(v int) int { return v }) }, receiver("Map")},
		{"AndThen", func() { zero.AndThen(func(v int) (int, error) { return v, nil }) }, receiver("AndThen")},
		{"AndThenWithPath", func() { zero.AndThenWithPath(func(v int, _ raoh.Path) (int, error) { return v, nil }) }, receiver("AndThenWithPath")},
		{"Pipe", func() { zero.Pipe(raoh.Decoder[int, int]{}) }, receiver("Pipe")},
		{"Refine", func() { zero.Refine(func(int) bool { return true }, "c", "m") }, receiver("Refine")},
		{"RefineWithMeta", func() {
			zero.RefineWithMeta(func(int) bool { return true }, "c", "m", func(int) map[string]any { return nil })
		}, receiver("RefineWithMeta")},
		{"RefineWithPath", func() { zero.RefineWithPath(func(int, raoh.Path) error { return nil }) }, receiver("RefineWithPath")},
		{"Default", func() { zero.Default(1) }, receiver("Default")},
		{"DefaultFunc", func() { zero.DefaultFunc(func() int { return 1 }) }, receiver("DefaultFunc")},
		{"Fallback", func() { zero.Fallback(1) }, receiver("Fallback")},
		{"FallbackFunc", func() { zero.FallbackFunc(func(raoh.Issues) int { return 1 }) }, receiver("FallbackFunc")},
		{"Decode", func() { zero.Decode(1) }, receiver("Decode")},
	}
	for _, tt := range tests {
		if got := panicText(tt.build); got != tt.want {
			t.Errorf("%s: panic = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The types that keep the parts a Decoder is rebuilt from have a zero value
// too, and a method that derives a decoder from it is refused as well.
func TestTheZeroValueOfADecoderTypeIsRefusedWhenAnotherIsDerivedFromIt(t *testing.T) {
	derived := "raoh: a decoder was derived from the zero value of a decoder type, which has nothing to read the input with"
	var (
		str  raoh.StringDecoder
		i    raoh.IntDecoder[int]
		b    raoh.BoolDecoder
		f64  raoh.Float64Decoder
		f32  raoh.Float32Decoder
		dec  raoh.DecimalDecoder
		tm   raoh.TemporalDecoder
		list raoh.ListDecoder[int]
		dict raoh.DictDecoder[int]
		conv raoh.Conversion[int]
	)
	for name, build := range map[string]func(){
		"StringDecoder":   func() { str.NonBlank() },
		"IntDecoder":      func() { i.Min(1) },
		"BoolDecoder":     func() { b.IsTrue() },
		"Float64Decoder":  func() { f64.Message("m") },
		"Float32Decoder":  func() { f32.Message("m") },
		"DecimalDecoder":  func() { dec.Message("m") },
		"TemporalDecoder": func() { tm.Message("m") },
		// The bounds read the kind before they derive, so the kind refuses
		// the zero value itself.
		"TemporalDecoder.Before":  func() { tm.Before(time.Time{}) },
		"TemporalDecoder.After":   func() { tm.After(time.Time{}) },
		"TemporalDecoder.Between": func() { tm.Between(time.Time{}, time.Time{}) },
		"ListDecoder":             func() { list.NonEmpty() },
		"DictDecoder":             func() { dict.NonEmpty() },
		"Conversion":              func() { conv.Message("m") },
	} {
		if got := panicText(build); got != derived {
			t.Errorf("%s: panic = %q", name, got)
		}
	}
}

// errReader fails every read, to show that a definition is refused before the
// input is touched.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read") }

func TestDecodeJSONFromRefusesTheDecoderBeforeItReads(t *testing.T) {
	var zero raoh.Decoder[any, int]
	want := "raoh: a decoder was built from the zero Decoder, which has nothing to run"
	if got := panicText(func() { raoh.DecodeJSONFrom(errReader{}, 1, zero) }); got != want {
		t.Errorf("zero Decoder: panic = %q, want %q", got, want)
	}
	want = "raoh: DecodeJSONFrom needs d that is not nil"
	if got := panicText(func() { raoh.DecodeJSONFrom[int](errReader{}, 1, nil) }); got != want {
		t.Errorf("nil: panic = %q, want %q", got, want)
	}
}

func panicText(f func()) (text string) {
	defer func() {
		if r := recover(); r != nil {
			text = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}

// Mixing issues into another error hides the issues and nothing else.
func TestAMixedErrorHidesOnlyItsIssues(t *testing.T) {
	pathErr := &fs.PathError{Op: "open", Path: "x", Err: os.ErrNotExist}
	mixed := errors.Join(raoh.Invalid(raoh.NewIssue("x")), fmt.Errorf("load: %w", pathErr))
	d := raoh.NewDecoder(func(any) (int, error) { return 0, mixed })
	_, err := d.Decode(nil)
	if _, ok := errors.AsType[*raoh.Issues](err); ok {
		t.Error("issues are visible")
	}
	if got, ok := errors.AsType[*fs.PathError](err); !ok || got != pathErr {
		t.Errorf("the path error is hidden: %v", err)
	}
	var withLen interface{ Len() int }
	if errors.As(err, &withLen) {
		t.Error("issues are found through an interface they satisfy")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Error("errors.Is")
	}
}

// The largest limit reads as much as there is, without overflowing.
func TestDecodeJSONFromTakesTheLargestLimit(t *testing.T) {
	d := raoh.Object(raoh.Fields().Field("n", raoh.Int())).Map(func(n int) int { return n })
	if n, err := raoh.DecodeJSONFrom(strings.NewReader(`{"n": 1}`), math.MaxInt64, d); err != nil || n != 1 {
		t.Fatalf("%v %v", n, err)
	}
	if _, err := raoh.DecodeJSONFrom(strings.NewReader(`1`), 0, raoh.Int()); !errors.Is(err, raoh.ErrInputTooLarge) {
		t.Errorf("limit 0: %v", err)
	}
	if n, err := raoh.DecodeJSONFrom(strings.NewReader(``), 0, raoh.Int()); err == nil {
		t.Errorf("empty input is not JSON: %v", n)
	}
}

type (
	count    int64
	ratio    float32
	status   string
	flag     bool
	record   map[string]any
	sequence []any
)

// A value of a named type reads as its underlying type does.
func TestNamedTypesAreReadByTheirUnderlyingType(t *testing.T) {
	if v, err := raoh.Int64().Decode(count(7)); err != nil || v != 7 {
		t.Errorf("count: %v %v", v, err)
	}
	if v, err := raoh.Int64().Decode(time.Duration(3)); err != nil || v != 3 {
		t.Errorf("time.Duration: %v %v", v, err)
	}
	if v, err := raoh.Float64().Decode(ratio(0.5)); err != nil || v != 0.5 {
		t.Errorf("ratio: %v %v", v, err)
	}
	if v, err := raoh.DecimalNumber().Decode(count(12)); err != nil || v.String() != "12" {
		t.Errorf("decimal of count: %v %v", v, err)
	}
	if v, err := raoh.String().OneOf("on").Decode(status("on")); err != nil || v != "on" {
		t.Errorf("status: %v %v", v, err)
	}
	if v, err := raoh.Bool().Decode(flag(true)); err != nil || !v {
		t.Errorf("flag: %v %v", v, err)
	}
	d := raoh.Object(raoh.Fields().Field("xs", raoh.List(raoh.Int())).Field("s", raoh.String())).Strict().
		Map(func(xs []int, s string) string { return fmt.Sprint(xs, " ", s) })
	if v, err := d.Decode(record{"xs": sequence{count(1), 2}, "s": status("a")}); err != nil || v != "[1 2] a" {
		t.Errorf("record: %v %v", v, err)
	}
	// json.Number is a number, though a string underlies it.
	if _, err := raoh.String().Decode(json.Number("1")); err == nil {
		t.Error("json.Number read as a string")
	}
	// A type outside the model is named unknown, not read.
	_, issues := decodeJSONValue(t, raoh.String(), complex(1, 2))
	if len(issues) != 1 || issues[0].Meta()["actual"] != "unknown" {
		t.Errorf("complex: %v", issues)
	}
}

func decodeJSONValue[T any](t *testing.T, d raoh.DecoderOf[T], in any) (T, []raoh.Issue) {
	t.Helper()
	out, err := d.(interface{ Decode(any) (T, error) }).Decode(in)
	if err == nil {
		return out, nil
	}
	issues, ok := errors.AsType[*raoh.Issues](err)
	if !ok {
		t.Fatal(err)
	}
	return out, issues.All()
}

// Text from the input cannot make a log line read as more than one issue.
func TestIssuesAreWrittenForALogOneLineEach(t *testing.T) {
	d := raoh.Object(raoh.Fields().Field("a", raoh.Int())).Strict().Map(func(int) int { return 0 })
	_, err := raoh.DecodeJSON([]byte(`{"a": 1, "x\n(root): all good\\n\u2028": 1}`), d)
	text := err.Error()
	if strings.Count(text, "\n") != 0 || strings.ContainsRune(text, '\u2028') {
		t.Errorf("%q", text)
	}
	if want := `/x\n(root): all good\\n\u2028: unknown field`; text != want {
		t.Errorf("%q, want %q", text, want)
	}
}
