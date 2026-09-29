package raoh

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkJSONObjectMarshal(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 200; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `"member%d":%d`, i, i)
	}
	sb.WriteString("}")
	v, _ := parseJSON([]byte(sb.String()))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}
