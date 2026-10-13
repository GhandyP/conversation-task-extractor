// Package format loads conversation input and renders extraction results.
package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// ErrInvalidConversation reports input that cannot be used for extraction.
var ErrInvalidConversation = errors.New("invalid conversation")

// ConversationError explains why a conversation was rejected.
type ConversationError struct {
	Source string
	Reason string
	Cause  error
}

// Error renders the source and the reason, with the cause when there is one.
func (e *ConversationError) Error() string {
	source := e.Source
	if source == "" {
		source = "conversation input"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", source, e.Reason, e.Cause)
	}
	return fmt.Sprintf("%s: %s", source, e.Reason)
}

// Unwrap exposes the underlying failure.
func (e *ConversationError) Unwrap() error { return e.Cause }

// Is makes errors.Is(err, ErrInvalidConversation) work for every rejection.
func (e *ConversationError) Is(target error) bool { return target == ErrInvalidConversation }

// LoadConversation reads one conversation from r, rejects trailing data, and
// validates it. A rejected conversation never comes back half-loaded.
func LoadConversation(r io.Reader) (domain.Conversation, error) {
	return loadConversation(r, "conversation input")
}

// LoadConversationFile reads one conversation from a file.
func LoadConversationFile(path string) (domain.Conversation, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.Conversation{}, &ConversationError{
			Source: path,
			Reason: "cannot read conversation",
			Cause:  err,
		}
	}
	defer file.Close()

	return loadConversation(file, path)
}

func loadConversation(r io.Reader, source string) (domain.Conversation, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return domain.Conversation{}, &ConversationError{
			Source: source,
			Reason: "cannot read conversation",
			Cause:  err,
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return domain.Conversation{}, &ConversationError{Source: source, Reason: "conversation is empty"}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var conv domain.Conversation
	if err := decoder.Decode(&conv); err != nil {
		return domain.Conversation{}, &ConversationError{
			Source: source,
			Reason: "conversation is not valid JSON",
			Cause:  err,
		}
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domain.Conversation{}, &ConversationError{
			Source: source,
			Reason: "conversation has trailing data after the JSON value",
		}
	}

	if err := conv.Validate(); err != nil {
		return domain.Conversation{}, &ConversationError{
			Source: source,
			Reason: "conversation failed validation",
			Cause:  err,
		}
	}
	return conv, nil
}
