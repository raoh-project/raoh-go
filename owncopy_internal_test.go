package raoh

import (
	"reflect"
	"testing"
	"time"
)

// ownCopy is equal to what it copies, nil kept nil and empty kept empty, at every depth and
// whichever path it takes, and shares no slice, array or map with it.
func TestAnOwnCopyIsEqualAndSharesNothing(t *testing.T) {
	when := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for _, v := range []any{
		nil, "s", 1, int32(2), 1.5, float32(0.5), true, MustDecimal("1.50"), when,
		[]any(nil), []any{}, map[string]any(nil), map[string]any{},
		[]string(nil), []string{}, []string{"a"}, map[string][]string{"a": nil, "b": {}},
		[]any{nil, []any(nil), map[string]any(nil), []int(nil), []any{}, "x"},
		map[string]any{"a": []any(nil), "b": map[string]any(nil), "c": []any{map[string]any{}}},
		[2][]int{nil, {1}}, [][2]any{{[]any(nil), map[string]any{}}},
	} {
		got := ownCopy(v)
		if !reflect.DeepEqual(got, v) {
			t.Errorf("ownCopy(%#v) is %#v", v, got)
		}
		var slow any
		if v != nil {
			slow = deepCopy(reflect.ValueOf(v)).Interface()
		}
		if !reflect.DeepEqual(got, slow) {
			t.Errorf("ownCopy(%#v) is %#v, and the copy through reflect %#v", v, got, slow)
		}
		if p, ok := shared(reflect.ValueOf(got), reflect.ValueOf(v)); ok {
			t.Errorf("ownCopy(%#v) shares %s", v, p)
		}
	}
}

// shared is where a and b, equal trees, hold the same non-empty slice or the same map.
func shared(a, b reflect.Value) (string, bool) {
	if !a.IsValid() || !b.IsValid() {
		return "", false
	}
	if a.Kind() == reflect.Interface {
		return shared(a.Elem(), b.Elem())
	}
	switch a.Kind() {
	case reflect.Slice:
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			return a.Type().String(), true
		}
		fallthrough
	case reflect.Array:
		for i := range a.Len() {
			if p, ok := shared(a.Index(i), b.Index(i)); ok {
				return p, true
			}
		}
	case reflect.Map:
		if !a.IsNil() && a.Pointer() == b.Pointer() {
			return a.Type().String(), true
		}
		for it := a.MapRange(); it.Next(); {
			if p, ok := shared(it.Value(), b.MapIndex(it.Key())); ok {
				return p, true
			}
		}
	}
	return "", false
}
