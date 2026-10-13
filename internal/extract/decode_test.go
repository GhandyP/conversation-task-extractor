package extract_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
	"github.com/GhandyP/conversation-task-extractor/internal/extract"
)

func testConversation() domain.Conversation {
	return domain.Conversation{
		ConversationID: "conv-1",
		Source:         domain.SourceChat,
		TimeZone:       "America/Argentina/Buenos_Aires",
		Messages: []domain.Message{
			{
				MessageID: "m1",
				Sender:    "ana",
				SentAt:    time.Date(2026, 3, 2, 9, 15, 0, 0, time.UTC),
				Text:      "Dale, mandame el informe el viernes 6.",
			},
			{
				MessageID: "m2",
				Sender:    "beto",
				SentAt:    time.Date(2026, 3, 2, 9, 20, 0, 0, time.UTC),
				Text:      "¿Podrías revisar el deck si llegás?",
			},
		},
	}
}

const goodPayload = `{
  "candidates": [
    {
      "title": "Send the report",
      "owner": "ana",
      "dueDate": "2026-03-06",
      "status": "explicit",
      "sourceQuote": "mandame el informe el viernes 6",
      "sourceMessageId": "m1",
      "confidence": 0.82
    }
  ]
}`

// fakeExtractor pins the port shape every provider must satisfy.
type fakeExtractor struct {
	name     string
	fallback []domain.TaskCandidate
}

func (f fakeExtractor) Name() string { return f.name }

func (f fakeExtractor) Extract(context.Context, domain.Conversation) ([]domain.TaskCandidate, error) {
	return f.fallback, nil
}

var _ extract.Extractor = fakeExtractor{}

func TestDecodeAcceptsProviderPayload(t *testing.T) {
	got, err := extract.Decode(testConversation(), []byte(goodPayload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(got))
	}
	candidate := got[0]
	if candidate.CandidateID == "" {
		t.Error("decoded candidate has no id")
	}
	if candidate.Status != domain.StatusExplicit {
		t.Errorf("status: got %q", candidate.Status)
	}
	if candidate.SourceQuote != "mandame el informe el viernes 6" {
		t.Errorf("quote not preserved: %q", candidate.SourceQuote)
	}
}

func TestDecodeAcceptsBareArray(t *testing.T) {
	payload := `[
	  {
	    "title": "Send the report",
	    "owner": "ana",
	    "dueDate": "2026-03-06",
	    "status": "explicit",
	    "sourceQuote": "mandame el informe el viernes 6",
	    "sourceMessageId": "m1",
	    "confidence": 0.82
	  }
	]`
	got, err := extract.Decode(testConversation(), []byte(payload))
	if err != nil {
		t.Fatalf("bare array payload must be accepted: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(got))
	}
}

func TestDecodeAcceptsEmptyCandidateList(t *testing.T) {
	got, err := extract.Decode(testConversation(), []byte(`{"candidates": []}`))
	if err != nil {
		t.Fatalf("a conversation with no tasks is a valid result: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 candidates, got %d", len(got))
	}
}

func TestDecodeIgnoresUnknownFields(t *testing.T) {
	payload := `{"candidates":[{"title":"Send the report","owner":"ana","status":"explicit",
	  "sourceQuote":"mandame el informe el viernes 6","sourceMessageId":"m1","confidence":0.5,
	  "reasoning":"the model added an extra field","dueDate":""}]}`
	got, err := extract.Decode(testConversation(), []byte(payload))
	if err != nil {
		t.Fatalf("unknown provider fields must not break the run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(got))
	}
}

func TestDecodeNormalizesMissingDetails(t *testing.T) {
	payload := `{"candidates":[{"title":"Review the deck","status":"tentative",
	  "sourceQuote":"¿Podrías revisar el deck si llegás?","sourceMessageId":"m2","confidence":0.4}]}`
	got, err := extract.Decode(testConversation(), []byte(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	candidate := got[0]
	if candidate.Owner != domain.OwnerUnknown {
		t.Errorf("owner: got %q, want %q", candidate.Owner, domain.OwnerUnknown)
	}
	notes := strings.Join(candidate.Notes, " | ")
	for _, want := range []string{"owner", "due date", "tentative"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes must mention %q: %v", want, candidate.Notes)
		}
	}
}

func TestDecodeAssignsDeterministicIDs(t *testing.T) {
	first, err := extract.Decode(testConversation(), []byte(goodPayload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	second, err := extract.Decode(testConversation(), []byte(goodPayload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first[0].CandidateID != second[0].CandidateID {
		t.Errorf("ids must be stable: %q vs %q", first[0].CandidateID, second[0].CandidateID)
	}
}

func TestDecodeRejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"not json", `this is not json`},
		{"empty body", ``},
		{"trailing garbage", goodPayload + " and more text"},
		{"missing candidates key", `{"tasks":[]}`},
		{"wrong candidates type", `{"candidates":{"title":"Send the report"}}`},
		{"wrong confidence type", `{"candidates":[{"title":"Send the report","owner":"ana","status":"explicit","sourceQuote":"mandame el informe el viernes 6","sourceMessageId":"m1","confidence":"high"}]}`},
		{"empty candidate", `{"candidates":[{}]}`},
		{"invented quote", `{"candidates":[{"title":"Send the report","owner":"ana","status":"explicit","sourceQuote":"mandame el informe el lunes 9","sourceMessageId":"m1","confidence":0.7}]}`},
		{"unknown status", `{"candidates":[{"title":"Send the report","owner":"ana","status":"probably","sourceQuote":"mandame el informe el viernes 6","sourceMessageId":"m1","confidence":0.7}]}`},
		{"unknown source message", `{"candidates":[{"title":"Send the report","owner":"ana","status":"explicit","sourceQuote":"mandame el informe el viernes 6","sourceMessageId":"m7","confidence":0.7}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extract.Decode(testConversation(), []byte(tc.payload))
			if err == nil {
				t.Fatalf("payload was accepted; want rejection, got %+v", got)
			}
			if !errors.Is(err, extract.ErrInvalidPayload) {
				t.Errorf("error must match ErrInvalidPayload, got %v", err)
			}
			if len(got) != 0 {
				t.Errorf("a rejected payload must not return partial results: %+v", got)
			}
		})
	}
}

func TestDecodeRejectsAnInvalidConversation(t *testing.T) {
	broken := testConversation()
	broken.Messages[1].MessageID = "m1" // duplicate id

	got, err := extract.Decode(broken, []byte(goodPayload))
	if err == nil {
		t.Fatal("decoding against an invalid conversation must fail")
	}
	if errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("an input problem is not a provider problem: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("no candidates on failure: %+v", got)
	}
}

func TestDecodeRejectsOneBadCandidateFromMany(t *testing.T) {
	payload := `{"candidates":[
	  {"title":"Send the report","owner":"ana","status":"explicit","sourceQuote":"mandame el informe el viernes 6","sourceMessageId":"m1","confidence":0.8},
	  {"title":"Invented task","owner":"ana","status":"explicit","sourceQuote":"nobody said this","sourceMessageId":"m1","confidence":0.8}
	]}`
	got, err := extract.Decode(testConversation(), []byte(payload))
	if err == nil {
		t.Fatal("one fabricated candidate must reject the whole payload")
	}
	if len(got) != 0 {
		t.Errorf("no partial acceptance: %+v", got)
	}
	if !strings.Contains(err.Error(), "sourceQuote") {
		t.Errorf("error must name the offending field: %v", err)
	}
}
