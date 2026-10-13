package extract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// Decode turns raw provider output into candidates that already passed the
// domain guard. It accepts either the documented envelope
// {"candidates":[...]} or a bare [...] array.
//
// The contract is all-or-nothing: if one candidate is malformed or quotes text
// that is not in the conversation, the whole payload is rejected, because a
// model that fabricated one task cannot be trusted with the rest.
func Decode(conv domain.Conversation, payload []byte) ([]domain.TaskCandidate, error) {
	if err := conv.Validate(); err != nil {
		return nil, err
	}

	raw, err := singleJSONValue(payload)
	if err != nil {
		return nil, err
	}

	candidates, err := decodeCandidates(raw)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	normalized := make([]domain.TaskCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		normalized = append(normalized, domain.Normalize(candidate))
	}
	normalized = domain.AssignCandidateIDs(conv, normalized)

	if err := domain.ValidateCandidates(conv, normalized); err != nil {
		return nil, invalidPayload("the provider returned candidates that do not match the conversation", err)
	}
	return normalized, nil
}

// singleJSONValue returns the first JSON value in payload and rejects trailing
// data, so a truncated response followed by prose cannot slip through.
func singleJSONValue(payload []byte) (json.RawMessage, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, invalidPayload("payload is empty", nil)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, invalidPayload("payload is not valid JSON", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, invalidPayload("payload has trailing data after the JSON value", err)
	}
	return raw, nil
}

func decodeCandidates(raw json.RawMessage) ([]domain.TaskCandidate, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, invalidPayload("payload is empty", nil)
	}

	switch trimmed[0] {
	case '[':
		var candidates []domain.TaskCandidate
		if err := json.Unmarshal(trimmed, &candidates); err != nil {
			return nil, invalidPayload("payload is not a list of candidates", err)
		}
		return candidates, nil
	case '{':
		// A pointer keeps "candidates":[] (no tasks) distinct from a payload
		// that forgot the key entirely.
		var envelope struct {
			Candidates *[]domain.TaskCandidate `json:"candidates"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, invalidPayload(`payload does not match the documented {"candidates":[...]} shape`, err)
		}
		if envelope.Candidates == nil {
			return nil, invalidPayload(`payload has no "candidates" list`, nil)
		}
		return *envelope.Candidates, nil
	default:
		return nil, invalidPayload("payload is neither a JSON object nor a JSON array", nil)
	}
}
