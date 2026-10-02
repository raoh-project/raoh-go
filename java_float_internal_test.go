package raoh

import (
	"math"
	"testing"
)

// A float is written as Java's Double.toString and Float.toString write it from JDK 19 on: the
// shortest digits, or two where one would be further from the value.
func TestAFloatIsWrittenAsJavaWritesIt(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		bits int
		want string
	}{
		{math.SmallestNonzeroFloat64, 64, "4.9E-324"},
		{float64(math.SmallestNonzeroFloat32), 32, "1.4E-45"},
		{1, 64, "1.0"}, {0.1, 64, "0.1"}, {100, 64, "100.0"}, {1e7, 64, "1.0E7"}, {2e-3, 64, "0.002"},
		{1e-4, 64, "1.0E-4"}, {math.MaxFloat64, 64, "1.7976931348623157E308"}, {-0.5, 64, "-0.5"},
		{float64(float32(0.1)), 32, "0.1"}, {2e22, 64, "2.0E22"},
	} {
		if got := javaFloatString(tc.v, tc.bits); got != tc.want {
			t.Errorf("%v (%d bits): %s, want %s", tc.v, tc.bits, got, tc.want)
		}
	}
}
