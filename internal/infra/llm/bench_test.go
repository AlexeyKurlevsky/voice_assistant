package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func BenchmarkOpenAICompat_MarshalRequest(b *testing.B) {
	req := chatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []chatMessage{
			{Role: "system", Content: "Ты — ассистент для конспектирования встреч."},
			{Role: "user", Content: strings.Repeat("транскрипция ", 2000)},
		},
		Stream: false,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenAICompat_ParseResponse(b *testing.B) {
	data := []byte(`{"choices":[{"message":{"role":"assistant","content":"краткая выжимка встречи"}}]}`)
	var out chatCompletionResponse
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}
