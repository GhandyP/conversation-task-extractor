package approve_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/approve"
	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

func testConversation() domain.Conversation {
	return domain.Conversation{
		ConversationID: "conv-1",
		Source:         domain.SourceChat,
		TimeZone:       "UTC",
		Messages: []domain.Message{
			{MessageID: "m1", Sender: "ana", SentAt: time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC), Text: "Dale, mandame el informe el viernes 6."},
			{MessageID: "m2", Sender: "beto", SentAt: time.Date(2026, 3, 2, 9, 5, 0, 0, time.UTC), Text: "Perdón, el informe lo entrego el lunes 9."},
			{MessageID: "m3", Sender: "ana", SentAt: time.Date(2026, 3, 2, 9, 9, 0, 0, time.UTC), Text: "¿Podrías revisar el deck si llegás?"},
		},
	}
}

func candidates(t *testing.T) []domain.TaskCandidate {
	t.Helper()
	conv := testConversation()
	return domain.AssignCandidateIDs(conv, []domain.TaskCandidate{
		{
			Title:           "Send the report",
			Owner:           "beto",
			DueDate:         "2026-03-06",
			Status:          domain.StatusExplicit,
			SourceQuote:     "mandame el informe el viernes 6",
			SourceMessageID: "m1",
			Confidence:      0.9,
		},
		{
			Title:           "Send the report",
			Owner:           "beto",
			DueDate:         "2026-03-06",
			Status:          domain.StatusExplicit,
			SourceQuote:     "mandame el informe el viernes 6",
			SourceMessageID: "m1",
			Confidence:      0.8,
		},
		{
			Title:           "Send the report",
			Owner:           "beto",
			DueDate:         "2026-03-09",
			Status:          domain.StatusExplicit,
			SourceQuote:     "el informe lo entrego el lunes 9",
			SourceMessageID: "m2",
			Confidence:      0.7,
		},
		{
			Title:           "Review the deck",
			Status:          domain.StatusTentative,
			SourceQuote:     "¿Podrías revisar el deck si llegás?",
			SourceMessageID: "m3",
			Confidence:      0.4,
		},
	})
}

func decisions(t *testing.T, all []domain.TaskCandidate, verdicts ...approve.Decision) approve.Decisions {
	t.Helper()
	decision := approve.Decisions{
		ConversationID: "conv-1",
		Verdicts:       map[string]approve.Decision{},
	}
	for i, verdict := range verdicts {
		decision.Verdicts[all[i].CandidateID] = verdict
	}
	return decision
}

func TestEvaluateCarriesEveryCandidateThrough(t *testing.T) {
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)

	// The human approves the first date, rejects the contradicted second one,
	// and approves the tentative task.
	verdicts := approve.Decisions{
		ConversationID: conv.ConversationID,
		Verdicts: map[string]approve.Decision{
			unique[0].CandidateID: approve.DecisionApprove,
			unique[1].CandidateID: approve.DecisionReject,
			unique[2].CandidateID: approve.DecisionApprove,
		},
	}

	result, err := approve.Evaluate(conv, all, verdicts)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(result.Approved) != 2 {
		t.Fatalf("want 2 approved tasks, got %d: %+v", len(result.Approved), result.Approved)
	}
	if len(result.Rejected) != 1 {
		t.Errorf("want 1 rejected candidate, got %d", len(result.Rejected))
	}
	if len(result.Pending) != 0 {
		t.Errorf("want no pending candidates, got %d", len(result.Pending))
	}
	if len(result.Duplicates) != 1 {
		t.Errorf("want the duplicate reported, got %+v", result.Duplicates)
	}
	if len(result.Conflicts) != 1 {
		t.Errorf("want the date conflict reported, got %+v", result.Conflicts)
	}
	for _, task := range result.Approved {
		if task.IdempotencyKey == "" {
			t.Errorf("approved task without idempotency key: %+v", task)
		}
	}
}

func TestEvaluateWithoutADecisionFileApprovesNothing(t *testing.T) {
	conv := testConversation()
	all := candidates(t)

	result, err := approve.Evaluate(conv, all, approve.Decisions{})
	if err != nil {
		t.Fatalf("missing decisions are not an error: %v", err)
	}
	if len(result.Approved) != 0 {
		t.Fatalf("silence is not approval: %+v", result.Approved)
	}
	if len(result.Pending) != 3 {
		t.Errorf("want every unique candidate pending, got %d", len(result.Pending))
	}

	if _, err := approve.Evaluate(conv, all, approve.Decisions{
		Verdicts: map[string]approve.Decision{all[0].CandidateID: approve.DecisionApprove},
	}); err == nil {
		t.Error("verdicts without a conversation id must be refused")
	}
}

func TestEvaluateNeverInventsApproval(t *testing.T) {
	conv := testConversation()
	all := candidates(t)

	result, err := approve.Evaluate(conv, all, approve.Decisions{ConversationID: conv.ConversationID})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(result.Approved) != 0 {
		t.Fatalf("candidates without a decision must not be approved: %+v", result.Approved)
	}
	if len(result.Pending) != 3 {
		t.Errorf("want the 3 unique candidates pending, got %d", len(result.Pending))
	}
}

func TestEvaluateAssignsStableIdempotencyKeys(t *testing.T) {
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)
	verdicts := approve.Decisions{
		ConversationID: conv.ConversationID,
		Verdicts:       map[string]approve.Decision{unique[0].CandidateID: approve.DecisionApprove},
	}

	first, err := approve.Evaluate(conv, all, verdicts)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	second, err := approve.Evaluate(conv, all, verdicts)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if first.Approved[0].IdempotencyKey != second.Approved[0].IdempotencyKey {
		t.Errorf("idempotency keys must be stable: %q vs %q",
			first.Approved[0].IdempotencyKey, second.Approved[0].IdempotencyKey)
	}

	other := verdicts
	other.Verdicts = map[string]approve.Decision{unique[2].CandidateID: approve.DecisionApprove}
	third, err := approve.Evaluate(conv, all, other)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if third.Approved[0].IdempotencyKey == first.Approved[0].IdempotencyKey {
		t.Error("different tasks must not share an idempotency key")
	}
}

func TestEvaluateRejectsBadDecisionFiles(t *testing.T) {
	conv := testConversation()
	all := candidates(t)

	tests := []struct {
		name     string
		verdicts approve.Decisions
		want     string
	}{
		{
			name: "wrong conversation",
			verdicts: approve.Decisions{
				ConversationID: "another-conversation",
				Verdicts:       map[string]approve.Decision{all[0].CandidateID: approve.DecisionApprove},
			},
			want: "another-conversation",
		},
		{
			name: "unknown candidate",
			verdicts: approve.Decisions{
				ConversationID: conv.ConversationID,
				Verdicts:       map[string]approve.Decision{"cand-does-not-exist": approve.DecisionApprove},
			},
			want: "cand-does-not-exist",
		},
		{
			name: "unknown verdict",
			verdicts: approve.Decisions{
				ConversationID: conv.ConversationID,
				Verdicts:       map[string]approve.Decision{all[0].CandidateID: "maybe"},
			},
			want: "maybe",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := approve.Evaluate(conv, all, tc.verdicts)
			if err == nil {
				t.Fatalf("decision file was accepted: %+v", result)
			}
			if !errors.Is(err, approve.ErrInvalidDecisions) {
				t.Errorf("error must match ErrInvalidDecisions, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q must mention %q", err, tc.want)
			}
			if len(result.Approved) != 0 {
				t.Errorf("a rejected decision file must approve nothing: %+v", result.Approved)
			}
		})
	}
}

func TestEvaluateRefusesAnUnresolvedConflict(t *testing.T) {
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)

	verdicts := approve.Decisions{
		ConversationID: conv.ConversationID,
		Verdicts: map[string]approve.Decision{
			unique[0].CandidateID: approve.DecisionApprove,
			unique[1].CandidateID: approve.DecisionApprove,
		},
	}
	result, err := approve.Evaluate(conv, all, verdicts)
	if err == nil {
		t.Fatalf("approving both sides of a date conflict must fail: %+v", result)
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("error must explain the conflict: %v", err)
	}
	if len(result.Approved) != 0 {
		t.Errorf("nothing may be approved while the conflict is open: %+v", result.Approved)
	}
}

func TestDecisionsRoundTripAsJSON(t *testing.T) {
	raw := `{"conversationId":"conv-1","decisions":{"cand-1":"approve","cand-2":"reject"}}`
	var parsed approve.Decisions
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Verdicts["cand-1"] != approve.DecisionApprove {
		t.Errorf("decisions did not decode: %+v", parsed)
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"conversationId", "decisions"} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("encoded decisions are missing %q: %s", key, encoded)
		}
	}
}
