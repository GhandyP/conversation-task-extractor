package approve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Applied reports what one Apply wrote and what it left alone.
type Applied struct {
	Written []ApprovedTask
	Skipped []ApprovedTask
}

// FileLedger is an append-only JSONL record of approved tasks.
//
// It never rewrites or deletes: a task whose idempotency key is already present
// is skipped, so re-running the same approval writes nothing twice.
type FileLedger struct {
	path string
}

// NewFileLedger returns a ledger stored at path.
func NewFileLedger(path string) *FileLedger {
	return &FileLedger{path: path}
}

// Path is where the ledger lives.
func (l *FileLedger) Path() string { return l.path }

// Records returns everything the ledger holds. A missing file is an empty
// ledger; a corrupt file is an error, because quietly ignoring a line would
// lose an approved task.
func (l *FileLedger) Records() ([]ApprovedTask, error) {
	return ReadLedger(l.path)
}

// Apply writes the tasks that are not recorded yet and reports both sets.
// An empty Apply touches nothing at all: no file is created.
func (l *FileLedger) Apply(tasks []ApprovedTask) (Applied, error) {
	var applied Applied
	if len(tasks) == 0 {
		return applied, nil
	}

	existing, err := ReadLedger(l.path)
	if err != nil {
		return applied, err
	}

	recorded := make(map[string]bool, len(existing))
	for _, record := range existing {
		recorded[record.IdempotencyKey] = true
	}

	var pending []ApprovedTask
	seen := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if recorded[task.IdempotencyKey] || seen[task.IdempotencyKey] {
			applied.Skipped = append(applied.Skipped, task)
			continue
		}
		seen[task.IdempotencyKey] = true
		pending = append(pending, task)
	}
	if len(pending) == 0 {
		return applied, nil
	}

	var buffer bytes.Buffer
	for _, task := range pending {
		line, err := json.Marshal(task)
		if err != nil {
			return applied, fmt.Errorf("%s: encoding an approved task failed: %w", l.path, err)
		}
		buffer.Write(line)
		buffer.WriteByte('\n')
	}

	// Conversation text lives here, so the ledger is private by default.
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return applied, fmt.Errorf("%s: opening the ledger failed: %w", l.path, err)
	}
	defer file.Close()

	if _, err := file.Write(buffer.Bytes()); err != nil {
		return applied, fmt.Errorf("%s: writing the ledger failed: %w", l.path, err)
	}
	if err := file.Sync(); err != nil {
		return applied, fmt.Errorf("%s: syncing the ledger failed: %w", l.path, err)
	}

	applied.Written = pending
	return applied, nil
}

// ReadLedger reads every record of a JSONL ledger.
func ReadLedger(path string) ([]ApprovedTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: reading the ledger failed: %w", path, err)
	}

	var records []ApprovedTask
	for index, line := range strings.Split(string(data), "\n") {
		number := index + 1
		if strings.TrimSpace(line) == "" {
			continue
		}

		var record ApprovedTask
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("%s: line %d is not a valid ledger record: %w", path, number, err)
		}
		if strings.TrimSpace(record.IdempotencyKey) == "" {
			return nil, fmt.Errorf("%s: line %d has no idempotencyKey", path, number)
		}
		if strings.TrimSpace(record.Candidate.SourceQuote) == "" {
			return nil, fmt.Errorf("%s: line %d has a candidate without a source quote", path, number)
		}
		records = append(records, record)
	}
	return records, nil
}
