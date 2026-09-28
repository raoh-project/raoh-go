// Package raoh decodes untyped boundary input into typed domain values,
// reporting every problem it finds with the JSON Pointer of where it was.
//
// It is the Go port of Raoh (https://github.com/kawasima/raoh). A [Decoder]
// turns an input into a value, or reports every issue in it as [Issues].
// Decode returns the value and a nil error, or the zero value and an error.
// When the input was invalid, that error is an *Issues, so
// errors.AsType[*raoh.Issues] tells invalid input from a failure of the
// program:
//
//	user, err := raoh.DecodeJSON(body, userDecoder)
//	if issues, ok := errors.AsType[*raoh.Issues](err); ok {
//		return badRequest(issues.Render(raoh.English))
//	} else if err != nil {
//		return err
//	}
//
// An object is decoded from a set of fields built with [Fields] and passed to
// [Object]:
//
//	var userDecoder = raoh.Object(
//		raoh.Fields().
//			Field("email", raoh.String().Trim().ToLower().Email().Map(NewEmail)).
//			Field("age", raoh.Int().Range(0, 150).Map(NewAge)),
//	).Strict().Map(NewUser)
//
// The function given to Map takes the fields' values in the order they were
// added, and the compiler checks that it does.
//
// The input is what encoding/json gives when it decodes into an any: nil,
// bool, string, float64 or json.Number, []any and map[string]any. [DecodeJSON]
// reads JSON text into the same shapes but keeps numbers as written and object
// members in the order written.
package raoh

//go:generate go run ./internal/gen
