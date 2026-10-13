package domain_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
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

func explicitCandidate() domain.TaskCandidate {
	return domain.TaskCandidate{
		Title:           "Send the report",
		Owner:           "ana",
		DueDate:         "2026-03-06",
		Status:          domain.StatusExplicit,
		SourceQuote:     "mandame el informe el viernes 6",
		SourceMessageID: "m1",
		Confidence:      0.9,
	}
}

func wantFieldError(t *testing.T, err error, field string) {
	t.Helper()
	var verrs domain.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("want domain.ValidationErrors, got %T: %v", err, err)
	}
	for _, e := range verrs {
		if e.Field == field {
			return
		}
	}
	t.Fatalf("no validation error for field %q; got %v", field, verrs)
}

func TestConversationJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(testConversation())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"conversationId", "source", "timeZone", "messages"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("conversation JSON is missing %q: %s", key, raw)
		}
	}
	messages, ok := decoded["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("messages is not a non-empty array: %s", raw)
	}
	first, _ := messages[0].(map[string]any)
	for _, key := range []string{"messageId", "sender", "sentAt", "text"} {
		if _, ok := first[key]; !ok {
			t.Errorf("message JSON is missing %q: %s", key, raw)
		}
	}
}

func TestTaskCandidateJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(explicitCandidate())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := decoded["confidence"], float64(0.9); got != want {
		t.Errorf("confidence round-trip: got %v, want %v", got, want)
	}
	for _, key := range []string{
		"candidateId", "title", "owner", "dueDate", "status",
		"sourceQuote", "sourceMessageId", "confidence",
	} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("candidate JSON is missing %q: %s", key, raw)
		}
	}
}

func TestStatusValid(t *testing.T) {
	valid := []domain.Status{domain.StatusExplicit, domain.StatusTentative, domain.StatusAmbiguous}
	for _, status := range valid {
		if !status.Valid() {
			t.Errorf("status %q should be valid", status)
		}
	}
	for _, status := range []domain.Status{"", "maybe", "EXPLICIT", "todo"} {
		if status.Valid() {
			t.Errorf("status %q should be invalid", status)
		}
	}
}

func TestConversationValidate(t *testing.T) {
	if err := testConversation().Validate(); err != nil {
		t.Fatalf("valid conversation was rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(c *domain.Conversation)
		field  string
	}{
		{"empty id", func(c *domain.Conversation) { c.ConversationID = "  " }, "conversationId"},
		{"bad source", func(c *domain.Conversation) { c.Source = "sms" }, "source"},
		{"empty time zone", func(c *domain.Conversation) { c.TimeZone = "" }, "timeZone"},
		{"unknown time zone", func(c *domain.Conversation) { c.TimeZone = "Mars/Olympus" }, "timeZone"},
		{"no messages", func(c *domain.Conversation) { c.Messages = nil }, "messages"},
		{"empty message id", func(c *domain.Conversation) { c.Messages[0].MessageID = "" }, "messageId"},
		{"duplicate message id", func(c *domain.Conversation) { c.Messages[1].MessageID = "m1" }, "messageId"},
		{"empty sender", func(c *domain.Conversation) { c.Messages[0].Sender = "" }, "sender"},
		{"zero sent time", func(c *domain.Conversation) { c.Messages[0].SentAt = time.Time{} }, "sentAt"},
		{"empty text", func(c *domain.Conversation) { c.Messages[0].Text = "   " }, "text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conv := testConversation()
			tc.mutate(&conv)
			wantFieldError(t, conv.Validate(), tc.field)
		})
	}
}

func TestValidateCandidates(t *testing.T) {
	conv := testConversation()
	if err := domain.ValidateCandidates(conv, []domain.TaskCandidate{explicitCandidate()}); err != nil {
		t.Fatalf("valid candidate was rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(c *domain.TaskCandidate)
		field  string
	}{
		{"empty title", func(c *domain.TaskCandidate) { c.Title = " " }, "title"},
		{"empty owner", func(c *domain.TaskCandidate) { c.Owner = "" }, "owner"},
		{"empty status", func(c *domain.TaskCandidate) { c.Status = "" }, "status"},
		{"unknown status", func(c *domain.TaskCandidate) { c.Status = "probably" }, "status"},
		{"confidence below zero", func(c *domain.TaskCandidate) { c.Confidence = -0.01 }, "confidence"},
		{"confidence above one", func(c *domain.TaskCandidate) { c.Confidence = 1.01 }, "confidence"},
		{"confidence not a number", func(c *domain.TaskCandidate) { c.Confidence = math.NaN() }, "confidence"},
		{"empty quote", func(c *domain.TaskCandidate) { c.SourceQuote = "  " }, "sourceQuote"},
		{"empty source message id", func(c *domain.TaskCandidate) { c.SourceMessageID = "" }, "sourceMessageId"},
		{"unknown source message id", func(c *domain.TaskCandidate) { c.SourceMessageID = "m9" }, "sourceMessageId"},
		{"invented quote", func(c *domain.TaskCandidate) { c.SourceQuote = "mandame el informe el lunes 9" }, "sourceQuote"},
		{"quote from another message", func(c *domain.TaskCandidate) {
			c.SourceMessageID = "m2"
			c.SourceQuote = "mandame el informe el viernes 6"
		}, "sourceQuote"},
		{"bad due date", func(c *domain.TaskCandidate) { c.DueDate = "06/03/2026" }, "dueDate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := explicitCandidate()
			tc.mutate(&candidate)
			wantFieldError(t, domain.ValidateCandidates(conv, []domain.TaskCandidate{candidate}), tc.field)
		})
	}
}

func TestValidateCandidatesAcceptsQuoteWithDifferentWhitespace(t *testing.T) {
	conv := testConversation()
	candidate := explicitCandidate()
	candidate.SourceQuote = "mandame   el informe\nel viernes 6"
	if err := domain.ValidateCandidates(conv, []domain.TaskCandidate{candidate}); err != nil {
		t.Fatalf("whitespace-only differences must not break the verbatim rule: %v", err)
	}
}

func TestValidateCandidatesDoesNotMutateInput(t *testing.T) {
	conv := testConversation()
	bad := explicitCandidate()
	bad.Status = "probably"
	candidates := []domain.TaskCandidate{bad, explicitCandidate()}
	before := make([]domain.TaskCandidate, len(candidates))
	copy(before, candidates)

	err := domain.ValidateCandidates(conv, candidates)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if !reflect.DeepEqual(before, candidates) {
		t.Errorf("validation mutated its input:\nbefore %+v\nafter  %+v", before, candidates)
	}
}

func TestValidateCandidatesAggregatesEveryProblem(t *testing.T) {
	conv := testConversation()
	broken := explicitCandidate()
	broken.Title = ""
	broken.Owner = ""
	err := domain.ValidateCandidates(conv, []domain.TaskCandidate{broken})
	var verrs domain.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("want domain.ValidationErrors, got %T", err)
	}
	if len(verrs) < 2 {
		t.Fatalf("want every problem reported at once, got %v", verrs)
	}
	for _, want := range []string{"title", "owner"} {
		found := false
		for _, e := range verrs {
			if e.Field == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing error for field %q in %v", want, verrs)
		}
	}
	if !strings.Contains(err.Error(), "candidate") {
		t.Errorf("aggregated message should name the entity: %q", err.Error())
	}
}

func TestNormalizeMarksMissingDetails(t *testing.T) {
	bare := domain.TaskCandidate{
		Title:           "  Send the report  ",
		Status:          domain.StatusExplicit,
		SourceQuote:     "mandame el informe el viernes 6",
		SourceMessageID: "m1",
		Confidence:      0.5,
	}
	got := domain.Normalize(bare)

	if got.Title != "Send the report" {
		t.Errorf("title not trimmed: %q", got.Title)
	}
	if got.Owner != domain.OwnerUnknown {
		t.Errorf("missing owner must be marked %q, got %q", domain.OwnerUnknown, got.Owner)
	}
	if got.DueDate != "" {
		t.Errorf("missing due date must stay empty, got %q", got.DueDate)
	}
	joined := strings.Join(got.Notes, " | ")
	if !strings.Contains(joined, "owner") {
		t.Errorf("notes must explain the missing owner: %v", got.Notes)
	}
	if !strings.Contains(joined, "due date") {
		t.Errorf("notes must explain the missing due date: %v", got.Notes)
	}
	if err := domain.ValidateCandidates(testConversation(), []domain.TaskCandidate{got}); err != nil {
		t.Errorf("normalized candidate must validate: %v", err)
	}
}

func TestNormalizeMarksTentativeWording(t *testing.T) {
	candidate := explicitCandidate()
	candidate.Status = domain.StatusTentative
	candidate.SourceQuote = "¿Podrías revisar el deck si llegás?"
	candidate.SourceMessageID = "m2"
	got := domain.Normalize(candidate)
	if !strings.Contains(strings.Join(got.Notes, " | "), "tentative") {
		t.Errorf("tentative candidates need an explicit note: %v", got.Notes)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	once := domain.Normalize(domain.TaskCandidate{
		Title:           "Send the report",
		Status:          domain.StatusTentative,
		SourceQuote:     "mandame el informe el viernes 6",
		SourceMessageID: "m1",
	})
	twice := domain.Normalize(once)
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("normalize is not idempotent:\nonce  %+v\ntwice %+v", once, twice)
	}
}

func TestAssignCandidateIDsIsDeterministic(t *testing.T) {
	conv := testConversation()
	first := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{explicitCandidate()})[0]
	second := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{explicitCandidate()})[0]
	if first.CandidateID == "" {
		t.Fatal("candidate id was not assigned")
	}
	if first.CandidateID != second.CandidateID {
		t.Errorf("same candidate got different ids: %q vs %q", first.CandidateID, second.CandidateID)
	}

	other := explicitCandidate()
	other.Title = "Send the deck"
	if got := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{other})[0]; got.CandidateID == first.CandidateID {
		t.Error("different tasks must not share a candidate id")
	}
}

func TestDeduplicate(t *testing.T) {
	conv := testConversation()
	base := explicitCandidate()
	repeat := explicitCandidate()
	other := explicitCandidate()
	other.Title = "Send the deck"
	other.SourceQuote = "revisar el deck"
	other.SourceMessageID = "m2"

	candidates := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{base, repeat, other})
	unique, duplicates := domain.Deduplicate(candidates)

	if len(unique) != 2 {
		t.Fatalf("want 2 unique candidates, got %d: %+v", len(unique), unique)
	}
	if len(duplicates) != 1 {
		t.Fatalf("want 1 duplicate group, got %d: %+v", len(duplicates), duplicates)
	}
	if duplicates[0].CandidateID != unique[0].CandidateID {
		t.Errorf("duplicate group must point at the kept candidate: %+v", duplicates[0])
	}
	if duplicates[0].Count != 1 {
		t.Errorf("want 1 extra copy, got %d", duplicates[0].Count)
	}
}

func TestFindDateConflicts(t *testing.T) {
	conv := testConversation()
	first := explicitCandidate()
	second := explicitCandidate()
	second.Title = "Send the report!"
	second.DueDate = "2026-03-09"
	third := explicitCandidate()
	third.Owner = "beto"

	candidates := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{first, second})
	conflicts := domain.FindDateConflicts(candidates)
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	if got := conflicts[0].DueDates; len(got) != 2 || got[0] == got[1] {
		t.Errorf("conflict must list both due dates: %v", got)
	}

	sameDate := domain.AssignCandidateIDs(conv, []domain.TaskCandidate{first, domain.Normalize(third)})
	if conflicts := domain.FindDateConflicts(sameDate); len(conflicts) != 0 {
		t.Errorf("different owners must not conflict: %+v", conflicts)
	}

	noDates := domain.Normalize(explicitCandidate())
	noDates.DueDate = ""
	if conflicts := domain.FindDateConflicts([]domain.TaskCandidate{noDates}); len(conflicts) != 0 {
		t.Errorf("missing due dates are not conflicts: %+v", conflicts)
	}
}
