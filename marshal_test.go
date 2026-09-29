package raoh

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"
)

var (
	_ json.Marshaler = (*JSONObject)(nil)
)

func decodeAny(t *testing.T, text string) any {
	t.Helper()
	v, issue := parseJSON([]byte(text))
	if issue != nil {
		t.Fatalf("parseJSON(%s): %v", text, issue)
	}
	return v
}

func TestJSONObjectMarshalRoundTrip(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{}`, `{}`},
		{`{"b":1,"a":2,"c":{"z":null,"y":[true,false]}}`, ``},
		{`[{"b":[{"d":1,"c":2}],"a":{}},[]]`, ``},
		{`{"n":[1.2300,1e+09,-0,1E-2,12345678901234567890123]}`, ``},
		{` { "a\"b" : 1 , "\n" : 2 , "é" : 3 } `, `{"a\"b":1,"\n":2,"é":3}`},
		// encoding/json escapes <, > and & itself.
		{`{"<&>":"<&>"}`, `{"\u003c\u0026\u003e":"\u003c\u0026\u003e"}`},
	} {
		want := tc.want
		if want == "" {
			want = tc.in
		}
		got, err := json.Marshal(decodeAny(t, tc.in))
		if err != nil || string(got) != want {
			t.Errorf("Marshal(%s) = %s, %v; want %s", tc.in, got, err, want)
		}
	}
}

func TestJSONObjectMarshalNested(t *testing.T) {
	obj := decodeAny(t, `{"b":1,"a":2}`)
	for name, tc := range map[string]struct {
		in   any
		want string
	}{
		"slice": {[]any{obj}, `[{"b":1,"a":2}]`},
		"map":   {map[string]any{"v": obj}, `{"v":{"b":1,"a":2}}`},
	} {
		got, err := json.Marshal(tc.in)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: Marshal = %s, %v; want %s", name, got, err, tc.want)
		}
	}
}

func TestJSONObjectMarshalEscapesNamesAsEncodingJSON(t *testing.T) {
	got, err := json.Marshal(decodeAny(t, `{"a":1}`))
	if err != nil || string(got) != `{"a":1}` {
		t.Errorf("Marshal = %s, %v", got, err)
	}
}

func TestJSONObjectMarshalToPassesOptionsToMembers(t *testing.T) {
	obj, _ := AsObject(map[string]any{"items": []any(nil)})
	v1, err := json.Marshal(obj)
	if err != nil || string(v1) != `{"items":null}` {
		t.Errorf("v1 = %s, %v", v1, err)
	}
	v2, err := jsonv2.Marshal(obj)
	if err != nil || string(v2) != `{"items":[]}` {
		t.Errorf("v2 = %s, %v", v2, err)
	}
	null, err := jsonv2.Marshal(obj, jsonv2.FormatNilSliceAsNull(true))
	if err != nil || string(null) != `{"items":null}` {
		t.Errorf("v2 FormatNilSliceAsNull = %s, %v", null, err)
	}
}
