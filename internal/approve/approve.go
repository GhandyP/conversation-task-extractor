// Package approve turns validated candidates plus explicit human decisions
// into an approved task set, and records approved work exactly once.
//
// The gate never invents approval: a candidate without a decision stays
// pending, and two contradicted candidates cannot both be approved.
package approve

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// ErrInvalidDecisions reports a decision record that cannot be trusted.
var ErrInvalidDecisions = errors.New("invalid decisions")

// ErrUnresolvedConflict reports decisions that approve two candidates which
// disagree about the same task's due date. It is a separate signal because the
// fix is a human choice, not a corrected file.
var ErrUnresolvedConflict = errors.New("unresolved conflict")

// DecisionsError explains why a decision record was rejected.
type DecisionsError struct {
	Reason string
	Cause  error

	// Conflict marks a well-formed record that leaves a date conflict open.
	Conflict bool
}

// Error renders the reason and, when present, the underlying failure.
func (e *DecisionsError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", ErrInvalidDecisions, e.Reason, e.Cause)
	}
	return fmt.Sprintf("%s: %s", ErrInvalidDecisions, e.Reason)
}

// Unwrap exposes the underlying failure.
func (e *DecisionsError) Unwrap() error { return e.Cause }

// Is reports the categories this error belongs to.
func (e *DecisionsError) Is(target error) bool {
	switch target {
	case ErrInvalidDecisions:
		return true
	case ErrUnresolvedConflict:
		return e.Conflict
	default:
		return false
	}
}

func invalidDecisions(reason string) error { return &DecisionsError{Reason: reason} }

// Decision is one human verdict about one candidate.
type Decision string

const (
	// DecisionApprove accepts the candidate as a real task.
	DecisionApprove Decision = "approve"
	// DecisionReject drops the candidate.
	DecisionReject Decision = "reject"
)

// Valid reports whether d is a known verdict.
func (d Decision) Valid() bool {
	return d == DecisionApprove || d == DecisionReject
}

// Decisions is the human approval record for one conversation, as written by a
// reviewer and read back from a file.
type Decisions struct {
	ConversationID string              `json:"conversationId"`
	Verdicts       map[string]Decision `json:"decisions"`
}

// ApprovedTask is one task a human approved, with the key that makes writing it
// idempotent.
type ApprovedTask struct {
	IdempotencyKey string               `json:"idempotencyKey"`
	Candidate      domain.TaskCandidate `json:"candidate"`
}

// Result reports what the gate did, with nothing hidden: approved, rejected,
// and still-pending candidates, plus the duplicates and conflicts it found.
type Result struct {
	ConversationID string
	Unique         []domain.TaskCandidate
	Approved       []ApprovedTask
	Rejected       []domain.TaskCandidate
	Pending        []domain.TaskCandidate
	Duplicates     []domain.Duplicate
	Conflicts      []domain.DateConflict
}

// IdempotencyKey derives the stable write key for one candidate: the same task
// in the same conversation always maps to the same key.
func IdempotencyKey(conv domain.Conversation, candidate domain.TaskCandidate) string {
	sum := sha256.Sum256([]byte(conv.ConversationID + "\x1f" + candidate.CandidateID))
	return "task-" + hex.EncodeToString(sum[:])[:16]
}

// Evaluate applies human decisions to extracted candidates.
//
// An empty conversation id means "no decisions recorded yet": everything stays
// pending, because silence is not approval. Verdicts without a conversation id
// are refused, and so is a decision about a candidate that does not exist.
func Evaluate(conv domain.Conversation, candidates []domain.TaskCandidate, decisions Decisions) (Result, error) {
	result := Result{ConversationID: conv.ConversationID}

	if decisions.ConversationID != conv.ConversationID {
		if decisions.ConversationID == "" {
			if len(decisions.Verdicts) > 0 {
				return result, invalidDecisions(
					"decision record carries verdicts but no conversationId; refusing to guess which conversation they belong to")
			}
		} else {
			return result, invalidDecisions(fmt.Sprintf(
				"decision record is for conversation %q but the input is conversation %q",
				decisions.ConversationID, conv.ConversationID))
		}
	}

	unique, duplicates := domain.Deduplicate(candidates)
	result.Unique = unique
	result.Duplicates = duplicates

	known := make(map[string]bool, len(unique))
	for _, candidate := range unique {
		known[candidate.CandidateID] = true
	}
	for id, verdict := range decisions.Verdicts {
		if !known[id] {
			return result, invalidDecisions(fmt.Sprintf(
				"decision record mentions candidate %q, which is not part of conversation %q", id, conv.ConversationID))
		}
		if !verdict.Valid() {
			return result, invalidDecisions(fmt.Sprintf(
				"decision record gives candidate %q the unknown verdict %q; use %q or %q",
				id, string(verdict), DecisionApprove, DecisionReject))
		}
	}

	approvedByID := make(map[string]bool, len(unique))
	for _, candidate := range unique {
		switch decisions.Verdicts[candidate.CandidateID] {
		case DecisionApprove:
			approvedByID[candidate.CandidateID] = true
			result.Approved = append(result.Approved, ApprovedTask{
				IdempotencyKey: IdempotencyKey(conv, candidate),
				Candidate:      candidate,
			})
		case DecisionReject:
			result.Rejected = append(result.Rejected, candidate)
		default:
			result.Pending = append(result.Pending, candidate)
		}
	}

	for _, conflict := range domain.FindDateConflicts(unique) {
		approved := make([]string, 0, len(conflict.CandidateIDs))
		for _, id := range conflict.CandidateIDs {
			if approvedByID[id] {
				approved = append(approved, id)
			}
		}
		if len(approved) > 1 {
			return Result{ConversationID: conv.ConversationID, Unique: unique, Duplicates: duplicates, Conflicts: []domain.DateConflict{conflict}},
				&DecisionsError{
					Conflict: true,
					Reason: fmt.Sprintf(
						"candidates %s are all approved but disagree about the due date of %q (%s); resolve the conflict by rejecting the dates that do not hold",
						strings.Join(approved, ", "), conflict.Title, strings.Join(conflict.DueDates, ", ")),
				}
		}
		result.Conflicts = append(result.Conflicts, conflict)
	}

	return result, nil
}

// SortByDueDate orders approved tasks by the date a human accepted, then by
// title, so a report reads the same way every run.
func SortByDueDate(tasks []ApprovedTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left, right := tasks[i].Candidate, tasks[j].Candidate
		if left.DueDate != right.DueDate {
			return left.DueDate < right.DueDate
		}
		return left.Title < right.Title
	})
}
