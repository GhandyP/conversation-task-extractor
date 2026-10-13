package approve_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/conversation-task-extractor/internal/approve"
	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

func TestLoadDecisionsAcceptsAValidRecord(t *testing.T) {
	decisions, err := approve.LoadDecisions(strings.NewReader(
		`{"conversationId":"conv-1","decisions":{"cand-1":"approve","cand-2":"reject"}}`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if decisions.ConversationID != "conv-1" {
		t.Errorf("conversationId: got %q", decisions.ConversationID)
	}
	if decisions.Verdicts["cand-1"] != approve.DecisionApprove {
		t.Errorf("verdicts were not decoded: %+v", decisions.Verdicts)
	}
}

func TestLoadDecisionsRejectsBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "   ", "empty"},
		{"not json", "decisions: none", "not valid JSON"},
		{"trailing data", `{"conversationId":"c","decisions":{}} nonsense`, "trailing data"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decisions, err := approve.LoadDecisions(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("input was accepted: %+v", decisions)
			}
			if !errors.Is(err, approve.ErrInvalidDecisions) {
				t.Errorf("error must match ErrInvalidDecisions, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q must mention %q", err, tc.want)
			}
			if decisions.ConversationID != "" || len(decisions.Verdicts) != 0 {
				t.Errorf("a rejected record must not come back half-loaded: %+v", decisions)
			}
		})
	}
}

func TestLoadDecisionsFileNamesThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := approve.LoadDecisionsFile(path); err == nil {
		t.Fatal("a corrupt decision record must fail")
	} else if !strings.Contains(err.Error(), "decisions.json") {
		t.Errorf("error must name the file: %v", err)
	}

	if _, err := approve.LoadDecisionsFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("a missing decision record must fail")
	} else if !errors.Is(err, approve.ErrInvalidDecisions) {
		t.Errorf("error must match ErrInvalidDecisions, got %v", err)
	}
}

func TestUnresolvedConflictIsItsOwnSignal(t *testing.T) {
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)

	_, err := approve.Evaluate(conv, all, approve.Decisions{
		ConversationID: conv.ConversationID,
		Verdicts: map[string]approve.Decision{
			unique[0].CandidateID: approve.DecisionApprove,
			unique[1].CandidateID: approve.DecisionApprove,
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	// The command line tells an open conflict apart from a malformed record
	// with these two signals, so both have to hold.
	if !errors.Is(err, approve.ErrUnresolvedConflict) {
		t.Errorf("error must match ErrUnresolvedConflict: %v", err)
	}
	if !errors.Is(err, approve.ErrInvalidDecisions) {
		t.Errorf("error must also match ErrInvalidDecisions: %v", err)
	}
}
