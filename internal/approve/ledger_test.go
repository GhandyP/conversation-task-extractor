package approve_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/conversation-task-extractor/internal/approve"
	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

func approvedTasks(t *testing.T, count int) []approve.ApprovedTask {
	t.Helper()
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)
	if count > len(unique) {
		t.Fatalf("only %d unique candidates available", len(unique))
	}

	tasks := make([]approve.ApprovedTask, 0, count)
	for i := 0; i < count; i++ {
		tasks = append(tasks, approve.ApprovedTask{
			IdempotencyKey: approve.IdempotencyKey(conv, unique[i]),
			Candidate:      unique[i],
		})
	}
	return tasks
}

func TestFileLedgerApplyIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	ledger := approve.NewFileLedger(path)
	tasks := approvedTasks(t, 2)

	first, err := ledger.Apply(tasks)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(first.Written) != 2 || len(first.Skipped) != 0 {
		t.Fatalf("first apply: written %d, skipped %d", len(first.Written), len(first.Skipped))
	}

	second, err := ledger.Apply(tasks)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(second.Written) != 0 || len(second.Skipped) != 2 {
		t.Fatalf("a repeated apply must write nothing: written %d, skipped %d",
			len(second.Written), len(second.Skipped))
	}

	records, err := approve.ReadLedger(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("ledger must keep exactly 2 records, got %d", len(records))
	}
}

func TestFileLedgerWritesPrivateFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	if _, err := approve.NewFileLedger(path).Apply(approvedTasks(t, 1)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ledger holds conversation text and must be private, got %o", perm)
	}
}

func TestFileLedgerCreatesNothingWithoutWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	applied, err := approve.NewFileLedger(path).Apply(nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied.Written) != 0 || len(applied.Skipped) != 0 {
		t.Fatalf("empty apply did work: %+v", applied)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("an apply with nothing to write must not create the ledger: %v", err)
	}
}

func TestFileLedgerLeavesTheFileUntouchedWhenItRefuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := approve.NewFileLedger(path).Apply(approvedTasks(t, 1))
	if err == nil {
		t.Fatal("a corrupt ledger must be refused, not overwritten")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error must point at the corrupt line: %v", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(after) != "not-json\n" {
		t.Errorf("a refused apply modified the ledger: %q", after)
	}
}

func TestFileLedgerSkipsWhatIsAlreadyRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	ledger := approve.NewFileLedger(path)
	tasks := approvedTasks(t, 2)

	if _, err := ledger.Apply(tasks[:1]); err != nil {
		t.Fatalf("apply: %v", err)
	}
	applied, err := ledger.Apply(tasks)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied.Written) != 1 || len(applied.Skipped) != 1 {
		t.Fatalf("want 1 written and 1 skipped, got %+v", applied)
	}
	if applied.Skipped[0].IdempotencyKey != tasks[0].IdempotencyKey {
		t.Errorf("the skipped task must be the recorded one: %+v", applied.Skipped[0])
	}
}

func TestReadLedgerReportsMissingFile(t *testing.T) {
	records, err := approve.ReadLedger(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("a missing ledger is an empty ledger: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("want no records, got %+v", records)
	}
}

func TestIdempotencyKeyFollowsTheConversation(t *testing.T) {
	conv := testConversation()
	all := candidates(t)
	unique, _ := domain.Deduplicate(all)

	first := approve.IdempotencyKey(conv, unique[0])
	repeat := approve.IdempotencyKey(conv, unique[0])
	if first != repeat {
		t.Errorf("keys must be stable: %q vs %q", first, repeat)
	}

	other := conv
	other.ConversationID = "conv-2"
	if approve.IdempotencyKey(other, unique[0]) == first {
		t.Error("the same task in another conversation needs its own key")
	}
}
