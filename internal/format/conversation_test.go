package format_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/GhandyP/conversation-task-extractor/internal/format"
)

const standupPath = "../../testdata/conversations/standup-2026-03-02.json"

func TestLoadConversationFileAcceptsRepositoryFixture(t *testing.T) {
	conv, err := format.LoadConversationFile(standupPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if conv.ConversationID != "standup-2026-03-02" {
		t.Errorf("conversationId: got %q", conv.ConversationID)
	}
	if len(conv.Messages) != 4 {
		t.Fatalf("want 4 messages, got %d", len(conv.Messages))
	}
	if conv.Messages[0].MessageID != "m1" {
		t.Errorf("first message id: got %q", conv.Messages[0].MessageID)
	}
	if conv.Messages[0].SentAt.IsZero() {
		t.Error("sentAt was not parsed")
	}
}

func TestLoadConversationAcceptsValidReader(t *testing.T) {
	valid := `{"conversationId":"c1","source":"chat","timeZone":"UTC",
	  "messages":[{"messageId":"m1","sender":"ana","sentAt":"2026-03-02T09:15:00Z","text":"Hola"}]}`
	if _, err := format.LoadConversation(strings.NewReader(valid)); err != nil {
		t.Fatalf("valid conversation rejected: %v", err)
	}
}

func TestLoadConversationRejectsBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", "   "},
		{"not json", "conversation: hello"},
		{"trailing data", `{"conversationId":"c1","source":"chat","timeZone":"UTC","messages":[]} extra`},
		{"no messages", `{"conversationId":"c1","source":"chat","timeZone":"UTC","messages":[]}`},
		{"bad time zone", `{"conversationId":"c1","source":"chat","timeZone":"Mars/Olympus","messages":[{"messageId":"m1","sender":"ana","sentAt":"2026-03-02T09:15:00Z","text":"Hola"}]}`},
		{"missing id", `{"source":"chat","timeZone":"UTC","messages":[{"messageId":"m1","sender":"ana","sentAt":"2026-03-02T09:15:00Z","text":"Hola"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conv, err := format.LoadConversation(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("input was accepted: %+v", conv)
			}
			if !errors.Is(err, format.ErrInvalidConversation) {
				t.Errorf("error must match ErrInvalidConversation, got %v", err)
			}
			if conv.ConversationID != "" || len(conv.Messages) != 0 {
				t.Errorf("failed loads must not return partial input: %+v", conv)
			}
		})
	}
}

func TestLoadConversationValidationErrorNamesTheField(t *testing.T) {
	_, err := format.LoadConversation(strings.NewReader(
		`{"conversationId":"c1","source":"chat","timeZone":"UTC","messages":[]}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "messages") {
		t.Errorf("error must name the rejected field: %v", err)
	}
}

func TestLoadConversationFileMissing(t *testing.T) {
	conv, err := format.LoadConversationFile("../../testdata/conversations/does-not-exist.json")
	if err == nil {
		t.Fatalf("missing file was accepted: %+v", conv)
	}
	if !errors.Is(err, format.ErrInvalidConversation) {
		t.Errorf("error must match ErrInvalidConversation, got %v", err)
	}
	if !strings.Contains(err.Error(), "does-not-exist.json") {
		t.Errorf("error must name the file: %v", err)
	}
}
