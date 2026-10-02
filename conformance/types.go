package main

import "strings"

// typ is a type of the Raoh Specification's value model, as
// catalog/operations.json writes one: a kind, and the types it is made of.
type typ struct {
	kind string
	// args are the element of a list, a set, a map, a nullable, an optional or
	// a presence, and the parts of a product.
	args []*typ
	// symbols are the alternatives of a symbol type.
	symbols []string
}

var (
	tString = &typ{kind: "string"}
	tInt32  = &typ{kind: "int32"}
)

func (t *typ) String() string {
	if len(t.args) == 0 {
		return t.kind
	}
	parts := make([]string, len(t.args))
	for n, a := range t.args {
		parts[n] = a.String()
	}
	return t.kind + "<" + strings.Join(parts, ",") + ">"
}
