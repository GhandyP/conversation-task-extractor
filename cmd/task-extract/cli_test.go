package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	standupConversation      = "../../testdata/conversations/standup-2026-03-02.json"
	quietConversation        = "../../testdata/conversations/quiet-thread-2026-03-03.json"
	hallucinatedConversation = "../../testdata/conversations/hallucinated-2026-03-04.json"
	fixtures                 = "../../testdata/fixtures"
)

// runCLI calls the command the way main does and captures everything.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func fixtureArgs(t *testing.T, extra ...string) []string {
	t.Helper()
	args := []string{
		"-input", standupConversation,
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", filepath.Join(t.TempDir(), "tasks.jsonl"),
	}
	return append(args, extra...)
}

func jsonReport(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, stdout, stderr := runCLI(t, append(args, "-format", "json")...)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	return report
}

func candidateIDs(report map[string]any, key string) []string {
	raw, _ := report[key].([]any)
	ids := make([]string, 0, len(raw))
	for _, entry := range raw {
		candidate, _ := entry.(map[string]any)
		if id, ok := candidate["candidateId"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func writeDecisions(t *testing.T, conversationID string, verdicts map[string]string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"conversationId": conversationID, "decisions": verdicts})
	if err != nil {
		t.Fatalf("marshal decisions: %v", err)
	}
	path := filepath.Join(t.TempDir(), "decisions.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write decisions: %v", err)
	}
	return path
}

func TestRunIsDryRunByDefault(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "tasks.jsonl")
	code, stdout, stderr := runCLI(t, []string{
		"-input", standupConversation,
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", ledger,
	}...)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"mode: dry run", "Send the March report", "quote (m1)", "written: 0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout is missing %q:\n%s", want, stdout)
		}
	}
	if _, err := os.Stat(ledger); !os.IsNotExist(err) {
		t.Errorf("a dry run must not create the ledger: %v", err)
	}
}

func TestRunRefusesToWriteWithoutDecisions(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "tasks.jsonl")
	code, _, stderr := runCLI(t, []string{
		"-input", standupConversation,
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", ledger,
		"-apply",
	}...)
	if code != 3 {
		t.Fatalf("exit: got %d, want 3 (approval blocked)\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "decisions") {
		t.Errorf("stderr must explain what is missing: %s", stderr)
	}
	if _, err := os.Stat(ledger); !os.IsNotExist(err) {
		t.Errorf("nothing may be written without decisions: %v", err)
	}
}

func TestRunAppliesDecisionsOnce(t *testing.T) {
	report := jsonReport(t, fixtureArgs(t)...)
	unique := candidateIDs(report, "unique")
	if len(unique) != 3 {
		t.Fatalf("want 3 unique candidates, got %v", unique)
	}
	decisions := writeDecisions(t, "standup-2026-03-02", map[string]string{
		unique[0]: "approve",
		unique[1]: "reject",
		unique[2]: "approve",
	})
	ledger := filepath.Join(t.TempDir(), "tasks.jsonl")

	args := []string{
		"-input", standupConversation,
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", ledger,
		"-decisions", decisions,
		"-apply",
	}
	code, stdout, stderr := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"mode: applied", "written: 2", "skipped: 0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout is missing %q:\n%s", want, stdout)
		}
	}

	records, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("ledger was not written: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(records)), "\n") + 1; lines != 2 {
		t.Errorf("want 2 ledger records, got %d:\n%s", lines, records)
	}

	// The same approval again writes nothing new.
	code, stdout, stderr = runCLI(t, args...)
	if code != 0 {
		t.Fatalf("second run exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"written: 0", "skipped: 2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("second run stdout is missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunBlocksOnAnOpenConflict(t *testing.T) {
	report := jsonReport(t, fixtureArgs(t)...)
	unique := candidateIDs(report, "unique")
	decisions := writeDecisions(t, "standup-2026-03-02", map[string]string{
		unique[0]: "approve",
		unique[1]: "approve",
	})

	code, _, stderr := runCLI(t, append(fixtureArgs(t), "-decisions", decisions, "-apply")...)
	if code != 3 {
		t.Fatalf("exit: got %d, want 3 for an unresolved conflict\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "conflict") {
		t.Errorf("stderr must name the conflict: %s", stderr)
	}
}

func TestRunRejectsAFabricatedQuote(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "tasks.jsonl")
	code, _, stderr := runCLI(t, []string{
		"-input", hallucinatedConversation,
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", ledger,
	}...)
	if code != 2 {
		t.Fatalf("exit: got %d, want 2 (extraction failed)\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "sourceQuote") {
		t.Errorf("stderr must name the rejected field: %s", stderr)
	}
	if _, err := os.Stat(ledger); !os.IsNotExist(err) {
		t.Errorf("a rejected extraction must not write: %v", err)
	}
}

func TestRunHandlesAConversationWithoutTasks(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"-input", quietConversation, "-provider", "fixture", "-fixture-dir", fixtures,
		"-ledger", filepath.Join(t.TempDir(), "tasks.jsonl"))
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "candidates: 0") {
		t.Errorf("a quiet thread is a valid empty result:\n%s", stdout)
	}
}

func TestRunRejectsUsageErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"no input", []string{"-provider", "fixture"}, 1, "-input"},
		{"unknown provider", []string{"-input", standupConversation, "-provider", "openai"}, 1, "provider"},
		{"unknown format", []string{"-input", standupConversation, "-provider", "fixture", "-fixture-dir", fixtures, "-format", "yaml"}, 1, "format"},
		{"missing fixture directory", []string{"-input", standupConversation, "-provider", "fixture", "-fixture-dir", "/nope"}, 2, "fixture"},
		{"missing gemini key", []string{"-input", standupConversation, "-provider", "gemini"}, 1, "GOOGLE_API_KEY"},
		{"missing conversation file", []string{"-input", "/nope/conversation.json", "-provider", "fixture", "-fixture-dir", fixtures}, 1, "conversation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "missing gemini key" {
				t.Setenv("GOOGLE_API_KEY", "")
				t.Setenv("GEMINI_API_KEY", "")
			}
			code, _, stderr := runCLI(t, tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit: got %d, want %d\nstderr: %s", code, tc.wantCode, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr %q must mention %q", stderr, tc.wantErr)
			}
		})
	}
}

func TestRunReadsTheConversationFromStdin(t *testing.T) {
	body, err := os.ReadFile(quietConversation)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-input", "-",
		"-provider", "fixture",
		"-fixture-dir", fixtures,
		"-ledger", filepath.Join(t.TempDir(), "tasks.jsonl"),
	}, bytes.NewReader(body), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "quiet-thread-2026-03-03") {
		t.Errorf("stdin conversation was not used:\n%s", stdout.String())
	}
}

func TestRunJSONReportCarriesTheWholePicture(t *testing.T) {
	report := jsonReport(t, fixtureArgs(t)...)

	for _, key := range []string{
		"conversationId", "provider", "mode", "candidates", "unique",
		"duplicates", "conflicts", "pending", "approved", "rejected", "written", "skipped",
	} {
		if _, ok := report[key]; !ok {
			t.Errorf("JSON report is missing %q", key)
		}
	}
	if report["mode"] != "dry_run" {
		t.Errorf("mode: got %v", report["mode"])
	}
	candidates, ok := report["candidates"].([]any)
	if !ok || len(candidates) != 4 {
		t.Fatalf("want the 4 extracted candidates, got %v", report["candidates"])
	}
	first, _ := candidates[0].(map[string]any)
	for _, key := range []string{"candidateId", "title", "owner", "dueDate", "status", "sourceQuote", "sourceMessageId", "confidence"} {
		if _, ok := first[key]; !ok {
			t.Errorf("candidate JSON is missing %q: %v", key, first)
		}
	}
	if duplicates, _ := report["duplicates"].([]any); len(duplicates) != 1 {
		t.Errorf("want the duplicate reported, got %v", report["duplicates"])
	}
	if conflicts, _ := report["conflicts"].([]any); len(conflicts) != 1 {
		t.Errorf("want the conflict reported, got %v", report["conflicts"])
	}
}

func TestRunHelpDocumentsEveryOption(t *testing.T) {
	code, stdout, stderr := runCLI(t, "-h")
	if code != 0 {
		t.Fatalf("exit %d for -h", code)
	}
	help := stdout + stderr
	for _, flag := range []string{"-input", "-provider", "-fixture-dir", "-model", "-decisions", "-ledger", "-apply", "-format"} {
		if !strings.Contains(help, flag) {
			t.Errorf("help is missing %q:\n%s", flag, help)
		}
	}
	for _, code := range []string{"0", "1", "2", "3"} {
		if !strings.Contains(help, code) {
			t.Errorf("help must document exit code %s:\n%s", code, help)
		}
	}
}
