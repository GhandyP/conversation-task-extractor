package extract_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GhandyP/conversation-task-extractor/internal/extract"
)

const geminiStructuredOutput = `{
  "candidates": [
    {
      "title": "Send the report",
      "owner": "ana",
      "dueDate": "2026-03-06",
      "status": "explicit",
      "sourceQuote": "mandame el informe el viernes 6",
      "sourceMessageId": "m1",
      "confidence": 0.9
    }
  ]
}`

// geminiResponse wraps text the way generateContent does.
func geminiResponse(text string) string {
	body := map[string]any{
		"candidates": []any{
			map[string]any{
				"content":      map[string]any{"parts": []any{map[string]any{"text": text}}},
				"finishReason": "STOP",
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

type capturedRequest struct {
	mu     sync.Mutex
	path   string
	apiKey string
	body   map[string]any
}

func (c *capturedRequest) record(r *http.Request) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = r.URL.Path
	c.apiKey = r.Header.Get("x-goog-api-key")
	return json.Unmarshal(raw, &c.body)
}

func (c *capturedRequest) snapshot() (path, apiKey string, body map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path, c.apiKey, c.body
}

func TestGeminiExtractSendsAStructuredRequestAndDecodesTheAnswer(t *testing.T) {
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := captured.record(r); err != nil {
			t.Errorf("record request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, geminiResponse(geminiStructuredOutput))
	}))
	defer server.Close()

	provider, err := extract.NewGemini(extract.GeminiOptions{
		APIKey:  "test-key",
		Model:   "gemini-flash-latest",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if got := provider.Name(); got != "gemini:gemini-flash-latest" {
		t.Errorf("name: got %q", got)
	}

	conv := testConversation()
	candidates, err := provider.Extract(context.Background(), conv)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("want 1 candidate, got %+v", candidates)
	}
	if candidates[0].CandidateID == "" {
		t.Error("decoded candidate has no id")
	}

	path, apiKey, body := captured.snapshot()
	if !strings.Contains(path, "gemini-flash-latest:generateContent") {
		t.Errorf("request path: got %q", path)
	}
	if apiKey != "test-key" {
		t.Errorf("api key header: got %q", apiKey)
	}

	generationConfig, ok := body["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("request has no generationConfig: %v", body)
	}
	if got := generationConfig["responseMimeType"]; got != "application/json" {
		t.Errorf("responseMimeType: got %v", got)
	}
	if temperature, ok := generationConfig["temperature"].(float64); !ok || temperature != 0 {
		t.Errorf("temperature must be 0 for extraction: got %v", generationConfig["temperature"])
	}

	schema, ok := generationConfig["responseSchema"].(map[string]any)
	if !ok {
		t.Fatalf("request has no responseSchema: %v", generationConfig)
	}
	statusEnum := schema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["status"].(map[string]any)["enum"].([]any)
	if len(statusEnum) != 3 {
		t.Fatalf("schema lost the status enum on the wire: %v", statusEnum)
	}

	contents, ok := body["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("request has no contents: %v", body)
	}
	parts := contents[0].(map[string]any)["parts"].([]any)
	prompt := parts[0].(map[string]any)["text"].(string)
	for _, want := range []string{"m1", "m2", "verbatim"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt sent to the model is missing %q", want)
		}
	}
}

func TestGeminiExtractReportsAPIErrorsWithoutLeakingTheKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"Quota exceeded"}}`)
	}))
	defer server.Close()

	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "super-secret-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.Extract(context.Background(), testConversation())
	if err == nil {
		t.Fatal("an API error must fail the extraction")
	}
	if errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("an HTTP failure is not a payload problem: %v", err)
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") {
		t.Errorf("error must keep the API status: %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Errorf("error must never contain the API key: %v", err)
	}
}

func TestGeminiExtractRejectsFabricatedQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiResponse(
			`{"candidates":[{"title":"Send the report","owner":"ana","status":"explicit","sourceQuote":"nobody said this","sourceMessageId":"m1","confidence":0.9}]}`))
	}))
	defer server.Close()

	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.Extract(context.Background(), testConversation())
	if !errors.Is(err, extract.ErrInvalidPayload) {
		t.Fatalf("a fabricated quote must fail the guard, got %v", err)
	}
	if !strings.Contains(err.Error(), "sourceQuote") {
		t.Errorf("error must name the offending field: %v", err)
	}
}

func TestGeminiExtractRejectsResponsesWithoutText(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"no candidates", `{"candidates":[]}`, "no text"},
		{"empty parts", `{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`, "SAFETY"},
		{"not json", `oops`, "not valid JSON"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "test-key", BaseURL: server.URL})
			if err != nil {
				t.Fatalf("new provider: %v", err)
			}
			_, err = provider.Extract(context.Background(), testConversation())
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, extract.ErrInvalidPayload) {
				t.Errorf("a transport-level problem is not a payload problem: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q must mention %q", err, tc.want)
			}
		})
	}
}

func TestGeminiExtractReportsTransportFailures(t *testing.T) {
	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "test-key", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	_, err = provider.Extract(context.Background(), testConversation())
	if err == nil {
		t.Fatal("a refused connection must fail")
	}
	if errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("a transport failure is not a payload problem: %v", err)
	}
	if !strings.Contains(err.Error(), "gemini") {
		t.Errorf("error must name the provider: %v", err)
	}
}

func TestGeminiRequiresAnAPIKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if _, err := extract.NewGemini(extract.GeminiOptions{APIKey: key}); err == nil {
			t.Fatalf("empty key %q was accepted", key)
		} else if !strings.Contains(err.Error(), "GOOGLE_API_KEY") {
			t.Errorf("error must name the expected variable: %v", err)
		}
	}
}

func TestGeminiDefaultsTheModel(t *testing.T) {
	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if got := provider.Name(); got != "gemini:"+extract.DefaultGeminiModel {
		t.Errorf("name: got %q, want the documented default model", got)
	}
}

func TestGeminiAcceptsTheDefaultModelStatus(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		_, _ = io.WriteString(w, geminiResponse(`{"candidates":[]}`))
	}))
	defer server.Close()

	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := provider.Extract(context.Background(), testConversation()); err != nil {
		t.Fatalf("an empty candidate list is a valid answer: %v", err)
	}
	if len(seen) != 1 || !strings.Contains(seen[0], extract.DefaultGeminiModel) {
		t.Errorf("model path: got %v", seen)
	}
}
