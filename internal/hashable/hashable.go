// Package hashable tells which types can be compared with == and used as map
// keys whatever their values are.
package hashable

import "reflect"

// Type reports whether every value of t can be compared with == and used as a
// map key without a run-time panic.
//
// Go's comparable admits interface types, but comparing two interface values
// panics when their dynamic type is a slice, a map or a function, so a type is
// hashable only when it holds no interface: a boolean, number, string, pointer
// or channel, or an array or struct of hashable types.
func Type(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface, reflect.Slice, reflect.Map, reflect.Func:
		return false
	case reflect.Array:
		return Type(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if !Type(t.Field(i).Type) {
				return false
			}
		}
	}
	return true
}
