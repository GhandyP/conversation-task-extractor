package extract_test

import (
	"strings"
	"testing"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
	"github.com/GhandyP/conversation-task-extractor/internal/extract"
)

func TestPromptCarriesEveryMessageAndTheTimeZone(t *testing.T) {
	conv := testConversation()
	prompt := extract.Prompt(conv)

	for _, want := range []string{
		conv.ConversationID,
		conv.TimeZone,
		"m1",
		"m2",
		"ana",
		"beto",
		"mandame el informe el viernes 6",
		"¿Podrías revisar el deck si llegás?",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

// The prompt carries the safety rules the decoder enforces. If one of these
// instructions is dropped, the model output starts failing the guard instead,
// so the phrases are pinned here on purpose.
func TestPromptStatesTheExtractionRules(t *testing.T) {
	prompt := strings.ToLower(extract.Prompt(testConversation()))
	for _, needle := range []string{
		"verbatim",
		"sourcemessageid",
		"tentative",
		"unknown",
		"explicit",
		"json",
	} {
		if !strings.Contains(prompt, needle) {
			t.Errorf("prompt is missing the rule marker %q", needle)
		}
	}
}

func TestPromptAsksForOnlyExplicitTasks(t *testing.T) {
	prompt := strings.ToLower(extract.Prompt(testConversation()))
	for _, banned := range []string{"infer implied tasks", "assume a due date"} {
		if strings.Contains(prompt, banned) {
			t.Errorf("prompt must not invite guessing: %q", banned)
		}
	}
	if !strings.Contains(prompt, "suggestion") && !strings.Contains(prompt, "sugerencia") {
		t.Error("prompt must tell the model to exclude suggestions")
	}
}

func TestResponseSchemaMatchesTheDomainStatuses(t *testing.T) {
	schema := extract.ResponseSchema()

	candidates := schema["properties"].(map[string]any)["candidates"].(map[string]any)
	items := candidates["items"].(map[string]any)
	properties := items["properties"].(map[string]any)

	statusProperty, ok := properties["status"].(map[string]any)
	if !ok {
		t.Fatal("schema has no status property")
	}
	enum, ok := statusProperty["enum"].([]string)
	if !ok {
		t.Fatalf("status enum has unexpected type %T", statusProperty["enum"])
	}

	want := map[string]bool{
		string(domain.StatusExplicit):  true,
		string(domain.StatusTentative): true,
		string(domain.StatusAmbiguous): true,
	}
	if len(enum) != len(want) {
		t.Fatalf("status enum must list exactly the domain statuses, got %v", enum)
	}
	for _, value := range enum {
		if !want[value] {
			t.Errorf("schema allows status %q which the domain rejects", value)
		}
	}

	required, ok := items["required"].([]string)
	if !ok {
		t.Fatalf("required has unexpected type %T", items["required"])
	}
	for _, field := range []string{"title", "owner", "status", "sourceQuote", "sourceMessageId", "confidence"} {
		if !contains(required, field) {
			t.Errorf("schema must require %q, got %v", field, required)
		}
	}
}

func TestResponseSchemaRequiresNoExtraTopLevelKey(t *testing.T) {
	schema := extract.ResponseSchema()
	if got := schema["type"]; got != "object" {
		t.Errorf("schema type: got %v", got)
	}
	required, _ := schema["required"].([]string)
	if !contains(required, "candidates") {
		t.Errorf("top-level schema must require candidates, got %v", required)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
