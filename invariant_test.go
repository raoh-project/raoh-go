package raoh_test

// The invariants the review of the first version found missing, each held
// where it can be broken from outside.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"testing"

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
