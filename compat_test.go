package raoh_test

// Holds the decoders to what Raoh for Java gives for the same input.
//
// testdata/compat/cases.json lists inputs by decoder name;
// scripts/compat/generate.sh runs them through the Java decoders of the same
// name in scripts/compat/Generate.java and writes testdata/compat/expected.json.
// Each decoder below is the Go counterpart of the Java one, and gives its
// output as the JSON the Java one's output serializes to.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/encode"
)

// out turns a decoder's output into the JSON value the Java counterpart's
// output serializes to.
func out[T, O any](d raoh.DecoderOf[T], f func(T) O) func([]byte) (any, error) {
	return func(text []byte) (any, error) {
		v, err := raoh.DecodeJSON(text, d)
		if err != nil {
			return nil, err
		}
		return f(v), nil
	}
}

func same[T any](v T) any { return v }

func opt[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func presence(p raoh.Presence[int32]) any {
	if p.IsAbsent() {
		return "absent"
	}
	if p.IsNull() {
		return "null"
	}
	v, _ := p.Value()
	return map[string]any{"present": v}
}

func list(values ...any) any { return values }

func compatDecoder(name string) func([]byte) (any, error) {
	s := raoh.String
	i := raoh.Int32
	switch name {
	case "string":
		return out(s(), same)
	case "string_normalized_email":
		return out(s().Trim().ToLower().Email(), same)
	case "string_non_blank":
		return out(s().NonBlank(), same)
	case "string_min_3":
		return out(s().MinLength(3), same)
	case "string_max_3":
		return out(s().MaxLength(3), same)
	case "string_length_4":
		return out(s().Length(4), same)
	case "string_one_of":
		return out(s().OneOf("b", "a"), same)
	case "string_starts_with":
		return out(s().StartsWith("ab"), same)
	case "string_ends_with":
		return out(s().EndsWith("ab"), same)
	case "string_contains":
		return out(s().Contains("ab"), same)
	case "string_ipv4":
		return out(s().IPv4(), same)
	case "string_ipv6":
		return out(s().IPv6(), same)
	case "string_ip":
		return out(s().IP(), same)
	case "string_ulid":
		return out(s().ULID(), same)
	case "string_cuid":
		return out(s().CUID(), same)
	case "string_pattern":
		return out(s().Pattern(`[a-z]+\d`), same)
	case "string_trim":
		return out(s().Trim(), same)
	case "string_lower":
		return out(s().ToLower(), same)
	case "string_upper":
		return out(s().ToUpper(), same)
	case "string_normalize":
		return out(s().Normalize(), same)
	case "string_normalize_nfd":
		return out(s().NormalizeAs(raoh.NFD), same)
	case "string_normalize_nfkc":
		return out(s().NormalizeAs(raoh.NFKC), same)
	case "string_normalize_nfkd":
		return out(s().NormalizeAs(raoh.NFKD), same)
	case "string_normalize_max_2":
		return out(s().Normalize().MaxLength(2), same)
	case "string_email":
		return out(s().Email(), same)
	case "string_one_of_astral":
		return out(s().OneOf("Ａ", "\U0001f600"), same)
	case "string_uuid":
		return out(s().UUID(), func(u raoh.UUID) any { return u.String() })
	case "string_url":
		return out(s().URL(), func(u *url.URL) any { return u.String() })

	case "int":
		return out(i(), same)
	case "int_min_1":
		return out(i().Min(1), same)
	case "int_max_10":
		return out(i().Max(10), same)
	case "int_range":
		return out(i().Range(0, 150), same)
	case "int_positive":
		return out(i().Positive(), same)
	case "int_negative":
		return out(i().Negative(), same)
	case "int_non_negative":
		return out(i().NonNegative(), same)
	case "int_non_positive":
		return out(i().NonPositive(), same)
	case "int_multiple_of_3":
		return out(i().MultipleOf(3), same)
	case "int_one_of":
		return out(i().OneOf(3, 1), same)
	case "long":
		return out(raoh.Int64(), same)

	case "double":
		return out(raoh.Float64(), same)
	case "double_positive":
		return out(raoh.Float64().Positive(), same)
	case "double_negative":
		return out(raoh.Float64().Negative(), same)
	case "double_range":
		return out(raoh.Float64().Range(0.5, 1.5), same)
	case "double_min":
		return out(raoh.Float64().Min(0.5), same)
	case "double_one_of":
		return out(raoh.Float64().OneOf(2.0, 1.0), same)
	case "double_min_1e7":
		return out(raoh.Float64().Min(1e7), same)
	case "double_max_small":
		return out(raoh.Float64().Max(1e-4), same)
	case "double_one_of_big":
		return out(raoh.Float64().OneOf(1e7, 0.5), same)

	case "decimal":
		return out(raoh.DecimalNumber(), same)
	case "decimal_scale_2":
		return out(raoh.DecimalNumber().Scale(2), same)
	case "decimal_positive":
		return out(raoh.DecimalNumber().Positive(), same)
	case "decimal_range":
		return out(raoh.DecimalNumber().Range(raoh.MustDecimal("0"), raoh.MustDecimal("10")), same)
	case "decimal_min_small":
		return out(raoh.DecimalNumber().Min(raoh.MustDecimal("0.0005")), same)

	case "bool":
		return out(raoh.Bool(), same)
	case "bool_is_true":
		return out(raoh.Bool().IsTrue(), same)

	case "list_int":
		return out(raoh.List(i()), same)
	case "list_non_empty":
		return out(raoh.List(i()).NonEmpty(), same)
	case "list_min_2":
		return out(raoh.List(i()).MinSize(2), same)
	case "list_max_2":
		return out(raoh.List(i()).MaxSize(2), same)
	case "list_size_2":
		return out(raoh.List(i()).Size(2), same)
	case "list_unique":
		return out(raoh.List(i()).Unique(), same)

	case "person":
		return out(raoh.Object(raoh.Fields().Field("name", s()).Field("age", i())).
			Map(func(n string, a int32) any { return list(n, a) }), same)
	case "person_strict":
		return out(raoh.Object(raoh.Fields().Field("name", s()).Field("age", i())).Strict().
			Map(func(n string, a int32) any { return list(n, a) }), same)
	case "escaped_keys":
		return out(raoh.Object(raoh.Fields().Field("a/b", i()).Field("~c", i())).
			Map(func(a, b int32) any { return list(a, b) }), same)
	case "optional":
		return out(raoh.Object(raoh.Fields().Field("id", i()).Field("nick", raoh.Optional(s()))).
			Map(func(id int32, nick *string) any { return list(id, opt(nick)) }), same)
	case "nullable":
		return out(raoh.Object(raoh.Fields().Field("id", i()).Field("note", raoh.Nullable(s()))).
			Map(func(id int32, note *string) any { return list(id, opt(note)) }), same)
	case "presence":
		return out(raoh.Object(raoh.Fields().Field("id", i()).Field("n", raoh.PresenceOf(i()))).
			Map(func(id int32, n raoh.Presence[int32]) any { return list(id, presence(n)) }), same)
	case "nested":
		item := raoh.Object(raoh.Fields().Field("name", s().NonBlank()).Field("qty", i().Positive())).
			Map(func(n string, q int32) any { return list(n, q) })
		return out(raoh.Object(raoh.Fields().Field("items", raoh.List(item)).Field("count", i().Default(0))).
			Map(func(items []any, count int32) any { return list(items, count) }), same)
	case "dict":
		return out(raoh.Dict(i()), same)
	case "optional_only":
		return out(raoh.Object(raoh.Fields().Field("a", raoh.Optional(s())).Field("b", raoh.Optional(s()))).
			Map(func(a, b *string) any { return list(opt(a), opt(b)) }), same)

	case "enum":
		return out(raoh.EnumOf(map[string]string{"RED": "red", "GREEN": "green"}), same)
	case "literal":
		return out(raoh.Literal("v1"), same)
	case "shape":
		return out(raoh.Discriminate("kind",
			raoh.Variant("square", raoh.Object(raoh.Fields().Field("side", i()).Field("kind", s())).
				Map(func(side int32, _ string) int32 { return side * side })),
			raoh.Variant("rect", raoh.Object(raoh.Fields().Field("w", i()).Field("h", i())).
				Map(func(w, h int32) int32 { return w * h })),
		), same)
	case "one_of":
		return out(raoh.OneOf[string](
			i().Map(func(n int32) string { return fmt.Sprint(n) }),
			s().MinLength(3),
		), same)
	case "with_default":
		return out(raoh.Object(raoh.Fields().Field("id", i().Default(0)).Field("page", i().Default(1))).
			Map(func(id, page int32) any { return list(id, page) }), same)
	case "recover":
		return out(raoh.Object(raoh.Fields().Field("id", i().Default(0)).Field("page", i().Fallback(1))).
			Map(func(id, page int32) any { return list(id, page) }), same)
	case "instant":
		return out(s().Instant(), encode.Instant())
	case "instant_after":
		return out(s().Instant().After(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)), encode.Instant())
	case "date":
		return out(s().Date(), encode.Date())
	case "date_before":
		return out(s().Date().Before(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)), encode.Date())
	case "date_between":
		return out(s().Date().Between(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)), encode.Date())
	case "time":
		return out(s().Time(), encode.Time())
	case "time_after":
		return out(s().Time().After(time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)), encode.Time())
	case "date_time":
		return out(s().DateTime(), encode.DateTime())
	case "date_time_before":
		return out(s().DateTime().Before(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)), encode.DateTime())
	case "offset_date_time":
		return out(s().OffsetDateTime(), encode.OffsetDateTime())
	case "offset_date_time_after":
		return out(s().OffsetDateTime().After(time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("", 9*3600))),
			encode.OffsetDateTime())
	case "period":
		return out(raoh.Object(raoh.Fields().Field("start", i()).Field("end", i())).
			AndThen(func(start, end int32) (any, error) {
				if start <= end {
					return list(start, end), nil
				}
				return nil, raoh.Invalid(raoh.NewIssue(raoh.CodeInvalidValue).
					WithMessage("end is before start").At(raoh.Path{}.Key("end")))
			}), same)
	}
	return nil
}

// outcome is what a case gives, as JSON: {"ok": output} or {"issues": [...]}.
func outcome(v any, err error) (any, error) {
	if err == nil {
		return map[string]any{"ok": v}, nil
	}
	issues, ok := errors.AsType[*raoh.Issues](err)
	if !ok {
		return nil, err
	}
	var list []any
	for _, i := range issues.All() {
		list = append(list, map[string]any{
			"path":        i.Path().String(),
			"code":        i.Code(),
			"message_key": i.MessageKey(),
			"message":     i.Message(raoh.English),
			"meta":        i.Meta(),
		})
	}
	return map[string]any{"issues": list}, nil
}

func failure(path, code, key, message, meta string) string {
	return fmt.Sprintf(`{"issues": [{"path": %q, "code": %q, "message_key": %q, "message": %q, "meta": %s}]}`,
		path, code, key, message, meta)
}

// divergence is a case where this package gives something else than Raoh for
// Java on purpose: the decoder, the input as JSON text, what this package
// gives, and why.
type divergence struct {
	decoder, input, ours, why string
}

func divergences() []divergence {
	notAnObject := func(actual string) string {
		return failure("", "type_mismatch", "type_mismatch", "expected object",
			fmt.Sprintf(`{"expected": "object", "actual": %q}`, actual))
	}
	required := failure("", "required", "required", "is required", "{}")
	const objectScope = "Object checks the input is an object once, at its own path; Java checks it " +
		"in each field and reads a non-object as holding no optional field"
	return []divergence{
		{"person", `[1]`, notAnObject("array"), objectScope},
		{"person", `null`, required, objectScope},
		{"person", `"str"`, notAnObject("string"), objectScope},
		{"person_strict", `[1]`, notAnObject("array"), objectScope},
		{"optional_only", `"x"`, notAnObject("string"), objectScope},
		{"optional_only", `null`, required, objectScope},
		{"optional_only", `[1]`, notAnObject("array"), objectScope},
		{"shape", `"rect"`, notAnObject("string"), objectScope},
	}
}

type compatCase struct {
	Decoder   string          `json:"decoder"`
	Input     json.RawMessage `json:"input"`
	InputJSON *string         `json:"input_json"`
	Ok        json.RawMessage `json:"ok"`
	Issues    json.RawMessage `json:"issues"`
}

func TestEveryCaseGivesWhatRaohForJavaGives(t *testing.T) {
	data, err := os.ReadFile("testdata/compat/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []compatCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	divs := divergences()
	used := map[int]bool{}
	var mismatches, skipped, stale []string
	for _, c := range cases {
		// A number whose text matters, such as -0, is given as the JSON text
		// to read.
		input := []byte(c.Input)
		if c.InputJSON != nil {
			input = []byte(*c.InputJSON)
		}
		decode := compatDecoder(c.Decoder)
		if decode == nil {
			skipped = append(skipped, c.Decoder)
			continue
		}
		java := `{"issues": ` + string(c.Issues) + `}`
		if c.Ok != nil {
			java = `{"ok": ` + string(c.Ok) + `}`
		}
		wanted := canonicalJSON(t, java)
		for n, d := range divs {
			if d.decoder == c.Decoder && reflect.DeepEqual(canonicalJSON(t, d.input), canonicalJSON(t, string(input))) {
				used[n] = true
				if reflect.DeepEqual(canonicalJSON(t, d.ours), wanted) {
					stale = append(stale, d.decoder+" "+d.input)
				}
				wanted = canonicalJSON(t, d.ours)
			}
		}
		got, err := outcome(decode(input))
		if err != nil {
			t.Errorf("%s %s: %v", c.Decoder, input, err)
			continue
		}
		if actual := canonical(got); !reflect.DeepEqual(actual, wanted) {
			mismatches = append(mismatches, fmt.Sprintf("%s %s\n  want: %v\n  go:   %v", c.Decoder, input, wanted, actual))
		}
	}
	if len(skipped) > 0 {
		t.Errorf("no Go decoder for %v", skipped)
	}
	if len(used) != len(divs) {
		t.Errorf("%d divergences name no case", len(divs)-len(used))
	}
	if len(stale) > 0 {
		t.Errorf("these divergences now give what Raoh for Java gives; remove them: %v", stale)
	}
	if len(mismatches) > 0 {
		t.Errorf("%d of %d cases differ:\n%s", len(mismatches), len(cases), strings.Join(mismatches, "\n"))
	}
}

// num is a number as the comparison sees it: whether it is written as an
// integer or a float, and its exact value. It tells 1 from 1.0, as Raoh for
// Java's JSON does.
type num struct {
	float bool
	value string
}

func numText(text string) num {
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		panic("not a number: " + text)
	}
	return num{strings.ContainsAny(text, ".eE"), r.RatString()}
}

// canonicalJSON parses text into the form canonical gives.
func canonicalJSON(t *testing.T, text string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return canonical(v)
}

// canonical turns a Go value into plain maps, slices, strings, bools and nums.
func canonical(v any) any {
	switch x := v.(type) {
	case nil, string, bool:
		return x
	case json.Number:
		return numText(string(x))
	case raoh.Decimal:
		return numText(x.String())
	case float64:
		// The shortest decimal that reads back as x, as Java writes a double.
		n := numText(strconv.FormatFloat(x, 'g', -1, 64))
		n.float = true
		return n
	case float32:
		return canonical(float64(x))
	case json.Marshaler:
		text, err := x.MarshalJSON()
		if err != nil {
			panic(err)
		}
		var parsed any
		dec := json.NewDecoder(strings.NewReader(string(text)))
		dec.UseNumber()
		if err := dec.Decode(&parsed); err != nil {
			panic(err)
		}
		return canonical(parsed)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return num{false, fmt.Sprint(rv.Int())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return num{false, fmt.Sprint(rv.Uint())}
	case reflect.Slice:
		items := make([]any, rv.Len())
		for i := range items {
			items[i] = canonical(rv.Index(i).Interface())
		}
		return items
	case reflect.Map:
		m := map[string]any{}
		for _, k := range rv.MapKeys() {
			m[fmt.Sprint(k.Interface())] = canonical(rv.MapIndex(k).Interface())
		}
		return m
	}
	panic(fmt.Sprintf("canonical: %T", v))
}

// The catalogues hold every template of Raoh for Java's, word for word, under
// the same key; the copies in testdata/compat/java come from the jar
// generate.sh ran.
func TestTheCataloguesHoldRaohForJavasTemplates(t *testing.T) {
	for file, ours := range map[string]*raoh.Messages{
		"messages.properties":    raoh.English,
		"messages_ja.properties": raoh.Japanese,
	} {
		text, err := os.ReadFile("testdata/compat/java/" + file)
		if err != nil {
			t.Fatal(err)
		}
		java, err := raoh.ParseProperties(string(text))
		if err != nil {
			t.Fatal(err)
		}
		templates := java.Templates()
		if len(templates) <= 40 {
			t.Fatalf("%s: only %d templates", file, len(templates))
		}
		for key, template := range templates {
			if got, _ := ours.Template(key); got != template {
				t.Errorf("%s %s: %q, want %q", file, key, got, template)
			}
		}
	}
}
