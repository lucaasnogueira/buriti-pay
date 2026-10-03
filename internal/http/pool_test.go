package http_test

import (
	"bytes"
	"encoding/json"
	"testing"

	apphttp "github.com/lucaasnogueira/buriti-pay/internal/http"
)

type samplePayload struct {
	ID    string `json:"id"`
	Value int64  `json:"value"`
	Desc  string `json:"desc"`
}

func BenchmarkEncodeJSON_Standard(b *testing.B) {
	data := samplePayload{
		ID:    "7f3c1a9e-0001",
		Value: 15000,
		Desc:  "Standard benchmark payload",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(data)
	}
}

func BenchmarkEncodeJSON_Pooled(b *testing.B) {
	data := samplePayload{
		ID:    "7f3c1a9e-0001",
		Value: 15000,
		Desc:  "Pooled benchmark payload",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = apphttp.EncodeJSONPooled(&buf, data)
	}
}
