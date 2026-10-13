// Package format renders what one run decided, for a human or for a machine.
package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// Modes a run can report.
const (
	// ModeDryRun means nothing was written.
	ModeDryRun = "dry_run"
	// ModeApplied means approved tasks were recorded in the ledger.
	ModeApplied = "applied"
)

// Report is everything one run decided, with nothing hidden: what the provider
// returned, what the guard kept, what a human approved, and what the ledger
// recorded.
type Report struct {
	ConversationID string                 `json:"conversationId"`
	Provider       string                 `json:"provider"`
	Mode           string                 `json:"mode"`
	Candidates     []domain.TaskCandidate `json:"candidates"`
	Unique         []domain.TaskCandidate `json:"unique"`
	Duplicates     []domain.Duplicate     `json:"duplicates"`
	Conflicts      []domain.DateConflict  `json:"conflicts"`
	Pending        []domain.TaskCandidate `json:"pending"`
	Approved       []domain.TaskCandidate `json:"approved"`
	Rejected       []domain.TaskCandidate `json:"rejected"`
	Written        []string               `json:"written"`
	Skipped        []string               `json:"skipped"`
}

// RenderJSON writes the machine-readable report. Empty lists are rendered as
// empty arrays, never as null, so a consumer can iterate without checking.
func RenderJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report.normalized())
}

// RenderText writes the report for a human reader.
func RenderText(w io.Writer, report Report) error {
	mode := "dry run (nothing is written; pass -apply to record approved tasks)"
	if report.Mode == ModeApplied {
		mode = "applied"
	}

	lines := []string{
		fmt.Sprintf("conversation: %s", report.ConversationID),
		fmt.Sprintf("provider: %s", report.Provider),
		fmt.Sprintf("mode: %s", mode),
		"",
		fmt.Sprintf("candidates: %d (unique %d, duplicates %d, conflicts %d)",
			len(report.Candidates), len(report.Unique), len(report.Duplicates), len(report.Conflicts)),
		fmt.Sprintf("pending: %d", len(report.Pending)),
		fmt.Sprintf("approved: %d", len(report.Approved)),
		fmt.Sprintf("rejected: %d", len(report.Rejected)),
		fmt.Sprintf("written: %d", len(report.Written)),
		fmt.Sprintf("skipped: %d", len(report.Skipped)),
	}

	if len(report.Unique) > 0 {
		lines = append(lines, "", "candidates:")
		for _, candidate := range report.Unique {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s",
				candidate.CandidateID, candidate.Status, candidate.Title))
			lines = append(lines, fmt.Sprintf("  owner: %s | due: %s | confidence: %.2f | state: %s",
				ownerOrUnknown(candidate.Owner), dueOrUnscheduled(candidate.DueDate),
				candidate.Confidence, stateOf(candidate, report)))
			lines = append(lines, fmt.Sprintf("  quote (%s): %q", candidate.SourceMessageID, candidate.SourceQuote))
			if len(candidate.Notes) > 0 {
				lines = append(lines, "  notes: "+strings.Join(candidate.Notes, "; "))
			}
		}
	}

	if len(report.Duplicates) > 0 {
		lines = append(lines, "", "duplicates:")
		for _, duplicate := range report.Duplicates {
			lines = append(lines, fmt.Sprintf("- %s was extracted %d extra time(s)",
				duplicate.CandidateID, duplicate.Count))
		}
	}

	if len(report.Conflicts) > 0 {
		lines = append(lines, "", "conflicts:")
		for _, conflict := range report.Conflicts {
			lines = append(lines, fmt.Sprintf("- %q (owner %s) carries %s",
				conflict.Title, ownerOrUnknown(conflict.Owner), strings.Join(conflict.DueDates, ", ")))
		}
		lines = append(lines, "  an open conflict blocks -apply until the dates that do not hold are rejected")
	}

	if len(report.Written) > 0 {
		lines = append(lines, "", "recorded:")
		for _, key := range report.Written {
			lines = append(lines, "- "+key)
		}
	}

	if len(report.Skipped) > 0 {
		lines = append(lines, "", "already recorded:")
		for _, key := range report.Skipped {
			lines = append(lines, "- "+key)
		}
	}

	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

func (r Report) normalized() Report {
	r.Candidates = emptyIfNil(r.Candidates)
	r.Unique = emptyIfNil(r.Unique)
	r.Duplicates = emptyIfNil(r.Duplicates)
	r.Conflicts = emptyIfNil(r.Conflicts)
	r.Pending = emptyIfNil(r.Pending)
	r.Approved = emptyIfNil(r.Approved)
	r.Rejected = emptyIfNil(r.Rejected)
	r.Written = emptyIfNil(r.Written)
	r.Skipped = emptyIfNil(r.Skipped)
	return r
}

func emptyIfNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

func stateOf(candidate domain.TaskCandidate, report Report) string {
	for _, approved := range report.Approved {
		if approved.CandidateID == candidate.CandidateID {
			return "approved"
		}
	}
	for _, rejected := range report.Rejected {
		if rejected.CandidateID == candidate.CandidateID {
			return "rejected"
		}
	}
	return "pending"
}

func ownerOrUnknown(owner string) string {
	if strings.TrimSpace(owner) == "" {
		return domain.OwnerUnknown
	}
	return owner
}

func dueOrUnscheduled(dueDate string) string {
	if strings.TrimSpace(dueDate) == "" {
		return "not stated"
	}
	return dueDate
}
