package raoh_test

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kawasima/raoh-go"
)

func ExampleObject() {
	type Signup struct {
		Name string
		Age  int
	}
	signup := raoh.Object(
		raoh.Fields().
			Field("name", raoh.String().Trim().MinLength(1)).
			Field("age", raoh.Int().Min(18)),
	).Strict().Map(func(name string, age int) Signup { return Signup{name, age} })

	_, err := signup.Decode(map[string]any{"name": "", "age": 12, "admin": true})
	if issues, ok := errors.AsType[*raoh.Issues](err); ok {
		for _, i := range issues.All() {
			fmt.Println(i.Path(), i.Code(), i.Message(raoh.English))
		}
	}
	// Output:
	// /name too_short must be at least 1 characters
	// /age out_of_range must be at least 18
	// /admin unknown_field unknown field
}

func ExampleDecodeJSON() {
	type Point struct{ X, Y int }
	point := raoh.Object(raoh.Fields().Field("x", raoh.Int()).Field("y", raoh.Int())).Strict().
		Map(func(x, y int) Point { return Point{x, y} })

	_, err := raoh.DecodeJSON([]byte(`{"x": "1", "y": 2, "z": 3}`), point)
	fmt.Println(err)
	// Output:
	// /x: expected long
	// /z: unknown field
}

func ExamplePresenceOf() {
	nickname := raoh.Object(raoh.Fields().Field("nickname", raoh.PresenceOf(raoh.String()))).
		Map(func(p raoh.Presence[string]) string {
			switch v, ok := p.Value(); {
			case p.IsAbsent():
				return "keep"
			case p.IsNull():
				return "clear"
			case ok:
				return "set to " + v
			}
			return ""
		})
	for _, body := range []string{`{}`, `{"nickname": null}`, `{"nickname": "Ken"}`} {
		v, _ := raoh.DecodeJSON([]byte(body), nickname)
		fmt.Println(v)
	}
	// Output:
	// keep
	// clear
	// set to Ken
}

func ExampleIssues_Render() {
	_, err := raoh.DecodeJSON([]byte(`"ab"`), raoh.String().MinLength(3))
	issues, _ := errors.AsType[*raoh.Issues](err)
	body, _ := json.Marshal(issues.Render(raoh.Japanese))
	fmt.Println(string(body))
	// Output:
	// [{"path":"","code":"too_short","message":"3文字以上で入力してください","meta":{"actual":2,"min":3}}]
}
