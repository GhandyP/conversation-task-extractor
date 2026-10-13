package extract_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
	"github.com/GhandyP/conversation-task-extractor/internal/extract"
	"github.com/GhandyP/conversation-task-extractor/internal/format"
)

const (
	fixtureDir      = "../../testdata/fixtures"
	conversationDir = "../../testdata/conversations"
)

func loadConversation(t *testing.T, name string) domain.Conversation {
	t.Helper()
	conv, err := format.LoadConversationFile(conversationDir + "/" + name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return conv
}

func TestFixtureExtractorRunsOffline(t *testing.T) {
	provider := extract.NewFixture(fixtureDir)
	if provider.Name() != "fixture" {
		t.Errorf("name: got %q", provider.Name())
	}

	conv := loadConversation(t, "standup-2026-03-02.json")
	candidates, err := provider.Extract(context.Background(), conv)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(candidates) != 4 {
		t.Fatalf("want the 4 recorded candidates, got %d: %+v", len(candidates), candidates)
	}
	for _, candidate := range candidates {
		if candidate.CandidateID == "" {
			t.Errorf("candidate without id: %+v", candidate)
		}
		if candidate.SourceMessageID == "m4" {
			t.Errorf("the wish in m4 must not become a task: %+v", candidate)
		}
	}
}

func TestFixtureExtractorMarksTentativeAndMissingDetails(t *testing.T) {
	conv := loadConversation(t, "standup-2026-03-02.json")
	candidates, err := extract.NewFixture(fixtureDir).Extract(context.Background(), conv)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	var tentative []domain.TaskCandidate
	for _, candidate := range candidates {
		if candidate.Status == domain.StatusTentative {
			tentative = append(tentative, candidate)
		}
	}
	if len(tentative) != 1 {
		t.Fatalf("want exactly 1 tentative candidate, got %d", len(tentative))
	}
	if tentative[0].Owner != domain.OwnerUnknown {
		t.Errorf("tentative candidate must mark the missing owner: %q", tentative[0].Owner)
	}
	if tentative[0].DueDate != "" {
		t.Errorf("tentative candidate must keep the missing due date empty: %q", tentative[0].DueDate)
	}
	notes := strings.Join(tentative[0].Notes, " | ")
	for _, want := range []string{"owner", "due date", "tentative"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes must mention %q: %v", want, tentative[0].Notes)
		}
	}
}

func TestFixtureExtractorProvesDuplicateAndConflictBehaviour(t *testing.T) {
	conv := loadConversation(t, "standup-2026-03-02.json")
	candidates, err := extract.NewFixture(fixtureDir).Extract(context.Background(), conv)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	unique, duplicates := domain.Deduplicate(candidates)
	if len(unique) != 3 {
		t.Fatalf("want 3 unique candidates, got %d: %+v", len(unique), unique)
	}
	if len(duplicates) != 1 || duplicates[0].Count != 1 {
		t.Fatalf("want 1 duplicated candidate, got %+v", duplicates)
	}

	conflicts := domain.FindDateConflicts(unique)
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflicting date, got %+v", conflicts)
	}
	if got := conflicts[0].DueDates; len(got) != 2 || got[0] != "2026-03-06" || got[1] != "2026-03-09" {
		t.Errorf("conflict must list both dates in order, got %v", got)
	}
}

func TestFixtureExtractorHandlesNoTaskConversation(t *testing.T) {
	conv := loadConversation(t, "quiet-thread-2026-03-03.json")
	candidates, err := extract.NewFixture(fixtureDir).Extract(context.Background(), conv)
	if err != nil {
		t.Fatalf("a conversation with no tasks is not an error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("want no candidates, got %+v", candidates)
	}
}

func TestFixtureExtractorRejectsFabricatedQuote(t *testing.T) {
	conv := loadConversation(t, "hallucinated-2026-03-04.json")
	_, err := extract.NewFixture(fixtureDir).Extract(context.Background(), conv)
	if err == nil {
		t.Fatal("a fabricated quote must be rejected")
	}
	if !errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("error must match ErrInvalidPayload, got %v", err)
	}
	if !strings.Contains(err.Error(), "sourceQuote") {
		t.Errorf("error must name the offending field: %v", err)
	}
}

func TestFixtureExtractorRejectsMissingFixture(t *testing.T) {
	conv := testConversation()
	_, err := extract.NewFixtureFS(fstest.MapFS{}).Extract(context.Background(), conv)
	if err == nil {
		t.Fatal("a missing fixture must fail loudly")
	}
	if errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("a missing fixture is a provider problem, not a payload problem: %v", err)
	}
	if !strings.Contains(err.Error(), conv.ConversationID) {
		t.Errorf("error must name the conversation: %v", err)
	}
}

func TestFixtureExtractorRejectsMalformedFixture(t *testing.T) {
	conv := testConversation()
	fsys := fstest.MapFS{"conv-1.json": {Data: []byte("{not json")}}
	_, err := extract.NewFixtureFS(fsys).Extract(context.Background(), conv)
	if !errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("a malformed fixture is a payload problem, got %v", err)
	}
}

func TestFixtureExtractorRefusesUnsafeConversationID(t *testing.T) {
	fsys := fstest.MapFS{"conv-1.json": {Data: []byte(`{"candidates":[]}`)}}
	conv := testConversation()
	conv.ConversationID = "../../../etc/passwd"

	_, err := extract.NewFixtureFS(fsys).Extract(context.Background(), conv)
	if err == nil {
		t.Fatal("a conversation id with path separators must be refused")
	}
	if errors.Is(err, extract.ErrInvalidPayload) {
		t.Errorf("an unsafe id is provider configuration, not a payload problem: %v", err)
	}
	if !strings.Contains(err.Error(), "conversation id") {
		t.Errorf("error must explain the unsafe id: %v", err)
	}
}
