package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

const (
	// DefaultGeminiModel is used when no model is configured.
	DefaultGeminiModel = "gemini-flash-latest"

	// defaultBaseURL is the public Gemini API root.
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// defaultTimeout bounds one extraction call.
	defaultTimeout = 60 * time.Second

	// maxResponseBytes bounds how much of an answer is read, so a runaway
	// response cannot exhaust memory.
	maxResponseBytes = 1 << 20
)

// GeminiOptions configures the Gemini provider.
type GeminiOptions struct {
	// APIKey is required and travels in a header, never in the URL.
	APIKey string

	// Model defaults to DefaultGeminiModel.
	Model string

	// BaseURL defaults to the public API and exists so tests can point the
	// provider at a local server.
	BaseURL string

	// Client defaults to a client with defaultTimeout.
	Client *http.Client
}

// Gemini extracts candidates with the Gemini API's structured output: the same
// response schema the prompt describes, plus temperature zero, then the same
// guard every other provider goes through.
type Gemini struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewGemini builds the provider, refusing to run without a key.
func NewGemini(opts GeminiOptions) (*Gemini, error) {
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		return nil, errors.New("gemini: an API key is required (set GOOGLE_API_KEY or GEMINI_API_KEY)")
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = DefaultGeminiModel
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	return &Gemini{apiKey: apiKey, model: model, baseURL: baseURL, client: client}, nil
}

// Name identifies the provider and the model in output and errors.
func (g *Gemini) Name() string { return "gemini:" + g.model }

// Extract sends one conversation to the model and returns candidates that
// passed the domain guard.
func (g *Gemini) Extract(ctx context.Context, conv domain.Conversation) ([]domain.TaskCandidate, error) {
	body, err := json.Marshal(generateContentRequest{
		Contents: []requestContent{{
			Role:  "user",
			Parts: []requestPart{{Text: Prompt(conv)}},
		}},
		GenerationConfig: requestGenerationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   ResponseSchema(),
			Temperature:      0,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: building the request failed: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/models/%s:generateContent", g.baseURL, g.model), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: building the request failed: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", g.apiKey)

	response, err := g.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("gemini: the request failed: %w", err)
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini: reading the response failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: %s returned %s: %s", g.model, response.Status, summarize(payload))
	}

	text, err := generateContentText(payload)
	if err != nil {
		return nil, err
	}
	return Decode(conv, []byte(text))
}

type generateContentRequest struct {
	Contents         []requestContent        `json:"contents"`
	GenerationConfig requestGenerationConfig `json:"generationConfig"`
}

type requestContent struct {
	Role  string        `json:"role"`
	Parts []requestPart `json:"parts"`
}

type requestPart struct {
	Text string `json:"text"`
}

type requestGenerationConfig struct {
	ResponseMIMEType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
	Temperature      float64        `json:"temperature"`
}

// generateContentText pulls the model's text out of a generateContent answer.
func generateContentText(payload []byte) (string, error) {
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("gemini: the response is not valid JSON: %w", err)
	}
	if len(decoded.Candidates) == 0 {
		return "", errors.New("gemini: the response has no text: the API returned no candidates")
	}

	var text strings.Builder
	for _, part := range decoded.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		reason := decoded.Candidates[0].FinishReason
		if reason == "" {
			reason = "unknown"
		}
		return "", fmt.Errorf("gemini: the response has no text (finishReason %s)", reason)
	}
	return text.String(), nil
}

// summarize keeps an error body short and single-line.
func summarize(payload []byte) string {
	const limit = 400
	text := strings.Join(strings.Fields(string(payload)), " ")
	if len(text) > limit {
		return text[:limit] + "..."
	}
	if text == "" {
		return "(empty body)"
	}
	return text
}
