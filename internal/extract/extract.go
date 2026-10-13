// Package extract defines the extraction port, the provider prompt, and the
// decoder that turns provider output into validated task candidates.
package extract

import (
	"context"
	"errors"
	"fmt"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// Extractor turns a conversation into task candidates.
//
// Implementations may call a model, read a fixture, or fail; either way the
// caller only trusts the candidates that come back through Decode.
type Extractor interface {
	// Name identifies the provider in output and error messages.
	Name() string

	// Extract returns candidates for one conversation.
	Extract(ctx context.Context, conv domain.Conversation) ([]domain.TaskCandidate, error)
}

// ErrInvalidPayload reports provider output that was rejected as a whole.
// A rejected payload never yields partial results.
var ErrInvalidPayload = errors.New("provider payload rejected")

// PayloadError explains why a provider payload was rejected.
type PayloadError struct {
	Reason string
	Cause  error
}

// Error renders the reason and, when present, the underlying failure.
func (e *PayloadError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", ErrInvalidPayload, e.Reason, e.Cause)
	}
	return fmt.Sprintf("%s: %s", ErrInvalidPayload, e.Reason)
}

// Unwrap exposes the underlying failure.
func (e *PayloadError) Unwrap() error { return e.Cause }

// Is makes errors.Is(err, ErrInvalidPayload) work for every rejection.
func (e *PayloadError) Is(target error) bool { return target == ErrInvalidPayload }

func invalidPayload(reason string, cause error) error {
	return &PayloadError{Reason: reason, Cause: cause}
}
