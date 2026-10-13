// Package domain holds the conversation and task-candidate model plus the
// checks that keep a candidate honest: a real status, a confidence in range,
// and a source quote that appears verbatim in the message it claims to come from.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	// Embed the IANA database so time-zone validation does not depend on the
	// host image having tzdata installed.
	_ "time/tzdata"
)

// Status records how strongly the conversation committed to a task.
type Status string

const (
	// StatusExplicit is a direct request or commitment.
	StatusExplicit Status = "explicit"
	// StatusTentative is wording that leaves the task open.
	StatusTentative Status = "tentative"
	// StatusAmbiguous is a task whose details contradict each other.
	StatusAmbiguous Status = "ambiguous"
)

// Valid reports whether s is one of the documented statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusExplicit, StatusTentative, StatusAmbiguous:
		return true
	default:
		return false
	}
}

// Source is the kind of conversation the text came from.
type Source string

const (
	// SourceChat is a chat thread.
	SourceChat Source = "chat"
	// SourceEmail is an email thread.
	SourceEmail Source = "email"
)

// Valid reports whether s is a supported source.
func (s Source) Valid() bool {
	switch s {
	case SourceChat, SourceEmail:
		return true
	default:
		return false
	}
}

// Constants that make missing details visible instead of implicit.
const (
	// OwnerUnknown marks a task whose owner was not stated.
	OwnerUnknown = "unknown"

	// Notes added by Normalize so a reader never has to guess.
	NoteOwnerUnknown   = "owner not stated in the conversation"
	NoteDueDateUnknown = "due date not stated in the conversation"
	NoteTentative      = "tentative wording: not a commitment"
)

// Message is one turn in a conversation.
type Message struct {
	MessageID string    `json:"messageId"`
	Sender    string    `json:"sender"`
	SentAt    time.Time `json:"sentAt"`
	Text      string    `json:"text"`
}

// Conversation is the extraction input: a message thread plus the time zone
// its dates must be read in.
type Conversation struct {
	ConversationID string    `json:"conversationId"`
	Source         Source    `json:"source"`
	TimeZone       string    `json:"timeZone"`
	Messages       []Message `json:"messages"`
}

// Message returns the message with the given id.
func (c Conversation) Message(id string) (Message, bool) {
	for _, m := range c.Messages {
		if m.MessageID == id {
			return m, true
		}
	}
	return Message{}, false
}

// TaskCandidate is one extracted task, always tied to the quote it came from.
type TaskCandidate struct {
	CandidateID     string   `json:"candidateId"`
	Title           string   `json:"title"`
	Owner           string   `json:"owner"`
	DueDate         string   `json:"dueDate"`
	Status          Status   `json:"status"`
	SourceQuote     string   `json:"sourceQuote"`
	SourceMessageID string   `json:"sourceMessageId"`
	Confidence      float64  `json:"confidence"`
	Notes           []string `json:"notes,omitempty"`
}

// FieldError names one rejected field.
type FieldError struct {
	Entity  string
	Index   int
	Field   string
	Problem string
}

// Error renders the failure with enough context to fix it.
func (e FieldError) Error() string {
	where := e.Entity
	if e.Index >= 0 {
		where = fmt.Sprintf("%s[%d]", e.Entity, e.Index)
	}
	return fmt.Sprintf("%s.%s: %s", where, e.Field, e.Problem)
}

// ValidationErrors aggregates every problem found in one pass.
type ValidationErrors []FieldError

// Error reports all problems at once, in the order they were found.
func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, err := range e {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

// Validate checks that the conversation is usable as extraction input.
func (c Conversation) Validate() error {
	var errs ValidationErrors
	add := func(index int, field, problem string) {
		errs = append(errs, FieldError{Entity: "conversation", Index: index, Field: field, Problem: problem})
	}

	if strings.TrimSpace(c.ConversationID) == "" {
		add(-1, "conversationId", "is required")
	}
	if !c.Source.Valid() {
		add(-1, "source", fmt.Sprintf("must be one of %s, %s", SourceChat, SourceEmail))
	}
	if strings.TrimSpace(c.TimeZone) == "" {
		add(-1, "timeZone", "is required (IANA name, for example UTC)")
	} else if _, err := time.LoadLocation(c.TimeZone); err != nil {
		add(-1, "timeZone", fmt.Sprintf("%q is not an IANA time zone", c.TimeZone))
	}
	if len(c.Messages) == 0 {
		add(-1, "messages", "must contain at least one message")
	}

	seen := make(map[string]bool, len(c.Messages))
	for i, m := range c.Messages {
		where := func(field, problem string) {
			errs = append(errs, FieldError{Entity: "message", Index: i, Field: field, Problem: problem})
		}
		switch {
		case strings.TrimSpace(m.MessageID) == "":
			where("messageId", "is required")
		case seen[m.MessageID]:
			where("messageId", fmt.Sprintf("%q is duplicated; message ids must be unique", m.MessageID))
		default:
			seen[m.MessageID] = true
		}
		if strings.TrimSpace(m.Sender) == "" {
			where("sender", "is required")
		}
		if m.SentAt.IsZero() {
			where("sentAt", "is required")
		}
		if strings.TrimSpace(m.Text) == "" {
			where("text", "is required")
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateCandidates checks every candidate against the conversation it claims
// to come from, and reports all problems at once without touching its input.
func ValidateCandidates(conv Conversation, candidates []TaskCandidate) error {
	var errs ValidationErrors
	for i, c := range candidates {
		errs = append(errs, validateCandidate(conv, c, i)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateCandidate checks a single candidate.
func ValidateCandidate(conv Conversation, c TaskCandidate) error {
	errs := validateCandidate(conv, c, 0)
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateCandidate(conv Conversation, c TaskCandidate, index int) ValidationErrors {
	var errs ValidationErrors
	where := func(field, problem string) {
		errs = append(errs, FieldError{Entity: "candidate", Index: index, Field: field, Problem: problem})
	}

	if strings.TrimSpace(c.Title) == "" {
		where("title", "is required")
	}
	if strings.TrimSpace(c.Owner) == "" {
		where("owner", fmt.Sprintf("is required; use %q when the conversation does not state one", OwnerUnknown))
	}
	if !c.Status.Valid() {
		where("status", fmt.Sprintf("must be one of %s, %s, %s", StatusExplicit, StatusTentative, StatusAmbiguous))
	}
	if math.IsNaN(c.Confidence) || c.Confidence < 0 || c.Confidence > 1 {
		where("confidence", "must be a number between 0 and 1")
	}
	if strings.TrimSpace(c.SourceQuote) == "" {
		where("sourceQuote", "is required; every candidate must quote its source")
	}
	if strings.TrimSpace(c.SourceMessageID) == "" {
		where("sourceMessageId", "is required")
	} else if message, ok := conv.Message(c.SourceMessageID); !ok {
		where("sourceMessageId", fmt.Sprintf("%q does not exist in conversation %q", c.SourceMessageID, conv.ConversationID))
	} else if strings.TrimSpace(c.SourceQuote) != "" && !containsQuote(message.Text, c.SourceQuote) {
		where("sourceQuote", fmt.Sprintf("%q does not appear verbatim in message %q", c.SourceQuote, c.SourceMessageID))
	}
	if dueDate := strings.TrimSpace(c.DueDate); dueDate != "" {
		parsed, err := time.Parse(time.DateOnly, dueDate)
		if err != nil {
			where("dueDate", fmt.Sprintf("%q is not a date in YYYY-MM-DD form", c.DueDate))
		} else if parsed.Format(time.DateOnly) != dueDate {
			where("dueDate", fmt.Sprintf("%q is not a canonical date", c.DueDate))
		}
	}

	return errs
}

// Normalize trims the free-text fields and makes missing details explicit:
// an absent owner becomes "unknown", and every gap is explained in Notes.
// It is idempotent.
func Normalize(c TaskCandidate) TaskCandidate {
	c.Title = strings.TrimSpace(c.Title)
	c.Owner = strings.TrimSpace(c.Owner)
	c.DueDate = strings.TrimSpace(c.DueDate)
	c.SourceQuote = strings.TrimSpace(c.SourceQuote)
	c.SourceMessageID = strings.TrimSpace(c.SourceMessageID)
	c.Notes = append([]string(nil), c.Notes...)

	if c.Owner == "" {
		c.Owner = OwnerUnknown
		c.Notes = appendNote(c.Notes, NoteOwnerUnknown)
	}
	if c.DueDate == "" {
		c.Notes = appendNote(c.Notes, NoteDueDateUnknown)
	}
	if c.Status == StatusTentative {
		c.Notes = appendNote(c.Notes, NoteTentative)
	}
	return c
}

// AssignCandidateIDs gives every candidate a stable id derived from its
// conversation, source message, quote, and title, so the same candidate keeps
// the same id across runs and a duplicated extraction collapses to one id.
func AssignCandidateIDs(conv Conversation, candidates []TaskCandidate) []TaskCandidate {
	out := make([]TaskCandidate, 0, len(candidates))
	for _, c := range candidates {
		c.CandidateID = CandidateID(conv, c)
		out = append(out, c)
	}
	return out
}

// CandidateID derives the stable id for one candidate.
func CandidateID(conv Conversation, c TaskCandidate) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		conv.ConversationID,
		c.SourceMessageID,
		normalizeSpace(c.SourceQuote),
		normalizeKey(c.Title),
	}, "\x1f")))
	return "cand-" + hex.EncodeToString(sum[:])[:12]
}

// Duplicate reports candidates that describe the same task.
type Duplicate struct {
	CandidateID string
	Count       int
}

// Deduplicate keeps the first candidate of every id and reports how many extra
// copies each id had.
func Deduplicate(candidates []TaskCandidate) (unique []TaskCandidate, duplicates []Duplicate) {
	extras := make(map[string]int, len(candidates))
	order := make([]string, 0, len(candidates))

	for _, c := range candidates {
		if _, seen := extras[c.CandidateID]; seen {
			extras[c.CandidateID]++
			continue
		}
		extras[c.CandidateID] = 0
		order = append(order, c.CandidateID)
		unique = append(unique, c)
	}

	for _, id := range order {
		if count := extras[id]; count > 0 {
			duplicates = append(duplicates, Duplicate{CandidateID: id, Count: count})
		}
	}
	return unique, duplicates
}

// DateConflict reports one task that carries two different due dates.
type DateConflict struct {
	Title        string
	Owner        string
	DueDates     []string
	CandidateIDs []string
}

// FindDateConflicts groups candidates by task identity (title and owner) and
// reports the groups whose due dates disagree.
func FindDateConflicts(candidates []TaskCandidate) []DateConflict {
	type group struct {
		title string
		owner string
		dates map[string]bool
		ids   []string
	}
	order := make([]string, 0, len(candidates))
	groups := make(map[string]*group)

	for _, c := range candidates {
		key := normalizeKey(c.Title) + "\x1f" + normalizeKey(c.Owner)
		g, ok := groups[key]
		if !ok {
			g = &group{title: c.Title, owner: c.Owner, dates: map[string]bool{}}
			groups[key] = g
			order = append(order, key)
		}
		g.ids = append(g.ids, c.CandidateID)
		if date := strings.TrimSpace(c.DueDate); date != "" {
			g.dates[date] = true
		}
	}

	var conflicts []DateConflict
	for _, key := range order {
		g := groups[key]
		if len(g.dates) < 2 {
			continue
		}
		dates := make([]string, 0, len(g.dates))
		for date := range g.dates {
			dates = append(dates, date)
		}
		sort.Strings(dates)
		conflicts = append(conflicts, DateConflict{
			Title:        g.title,
			Owner:        g.owner,
			DueDates:     dates,
			CandidateIDs: g.ids,
		})
	}
	return conflicts
}

func appendNote(notes []string, note string) []string {
	for _, existing := range notes {
		if existing == note {
			return notes
		}
	}
	return append(notes, note)
}

// containsQuote reports whether quote appears in text once whitespace runs are
// collapsed. Everything else must match exactly: a paraphrase is not a quote.
func containsQuote(text, quote string) bool {
	return strings.Contains(normalizeSpace(text), normalizeSpace(quote))
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalizeKey makes titles and owners comparable: case-insensitive, with
// collapsed whitespace and no trailing punctuation.
func normalizeKey(s string) string {
	return strings.TrimRight(strings.ToLower(normalizeSpace(s)), ".,;:!?¡¿\"'")
}
