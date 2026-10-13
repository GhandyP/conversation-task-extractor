package extract

import (
	"fmt"
	"strings"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// Prompt returns the extraction instructions for one conversation. The rules
// it states are the same ones Decode enforces, so a model that follows the
// prompt passes the guard.
func Prompt(conv domain.Conversation) string {
	var b strings.Builder

	b.WriteString("You extract tasks from a conversation. Return JSON only, with no prose around it.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Extract only tasks the conversation states as an explicit request or commitment. A suggestion, an idea, a question, or a wish is not a task: leave it out.\n")
	b.WriteString("- Keep tentative wording tentative: use status \"tentative\" when the speaker left it open, \"ambiguous\" when the details contradict each other, and \"explicit\" only for a direct request or commitment.\n")
	b.WriteString("- sourceQuote must be copied verbatim from the message it comes from. Never paraphrase, translate, correct, or join text from two messages.\n")
	b.WriteString("- sourceMessageId must be the id of the message the quote comes from.\n")
	b.WriteString("- Use owner \"unknown\" when no owner is stated, and an empty dueDate when no date is stated. Never guess either one.\n")
	b.WriteString("- dueDate is a calendar date in YYYY-MM-DD form, read in the conversation time zone.\n")
	b.WriteString("- confidence is your confidence in the extraction, as a number between 0 and 1.\n")
	b.WriteString("- When the conversation contains no task, return {\"candidates\":[]}.\n\n")
	b.WriteString("Return exactly this JSON object:\n")
	b.WriteString("{\"candidates\":[{\"title\":\"...\",\"owner\":\"...\",\"dueDate\":\"...\",\"status\":\"explicit\",\"sourceQuote\":\"...\",\"sourceMessageId\":\"...\",\"confidence\":0.0}]}\n\n")

	fmt.Fprintf(&b, "Conversation %s (%s), time zone %s.\n\n", conv.ConversationID, conv.Source, conv.TimeZone)
	for _, m := range conv.Messages {
		fmt.Fprintf(&b, "[%s] %s (%s):\n%s\n\n", m.MessageID, m.Sender, m.SentAt.Format(time.RFC3339), strings.TrimSpace(m.Text))
	}
	return b.String()
}

// ResponseSchema describes the JSON object the provider must return. The
// status enum is built from the domain statuses so the schema and the
// validator cannot drift apart.
func ResponseSchema() map[string]any {
	properties := map[string]any{
		"title": map[string]any{
			"type":        "string",
			"description": "Short imperative title of the task.",
		},
		"owner": map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("Person responsible, or %q when none is stated.", domain.OwnerUnknown),
		},
		"dueDate": map[string]any{
			"type":        "string",
			"description": "Calendar date in YYYY-MM-DD form, or an empty string when no date is stated.",
		},
		"status": map[string]any{
			"type":        "string",
			"description": "How strongly the conversation committed to the task.",
			"enum": []string{
				string(domain.StatusExplicit),
				string(domain.StatusTentative),
				string(domain.StatusAmbiguous),
			},
		},
		"sourceQuote": map[string]any{
			"type":        "string",
			"description": "Text copied verbatim from the source message.",
		},
		"sourceMessageId": map[string]any{
			"type":        "string",
			"description": "Id of the message the quote comes from.",
		},
		"confidence": map[string]any{
			"type":        "number",
			"description": "Confidence in the extraction, between 0 and 1.",
			"minimum":     0,
			"maximum":     1,
		},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": properties,
					"required": []string{
						"title", "owner", "status", "sourceQuote", "sourceMessageId", "confidence",
					},
				},
			},
		},
		"required": []string{"candidates"},
	}
}
