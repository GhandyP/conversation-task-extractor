package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GhandyP/conversation-task-extractor/internal/approve"
	"github.com/GhandyP/conversation-task-extractor/internal/domain"
	"github.com/GhandyP/conversation-task-extractor/internal/extract"
	"github.com/GhandyP/conversation-task-extractor/internal/format"
)

// Exit codes are part of the documented interface.
const (
	exitOK               = 0
	exitUsage            = 1
	exitExtractionFailed = 2
	exitApprovalBlocked  = 3
)

// defaultTimeout bounds one extraction call.
const defaultTimeout = 60 * time.Second

const usageTemplate = `task-extract turns a conversation into source-backed task candidates.

Usage:
  task-extract -input <file|-> [flags]

Flags:
  -input <path>        conversation JSON file, or - for stdin (required)
  -provider <name>     extraction provider: gemini | fixture (default "gemini")
  -fixture-dir <path>  recorded provider payloads for -provider fixture (default "testdata/fixtures")
  -model <name>        Gemini model (default $GEMINI_MODEL or %s)
  -decisions <path>    JSON file with the human approve/reject decisions
  -ledger <path>       JSONL ledger of approved tasks (default "tasks.jsonl")
  -apply               record approved tasks in the ledger (default: dry run)
  -format <name>       output format: text | json (default "text")
  -timeout <duration>  bound for one extraction call (default %s)

Exit codes:
  0  success
  1  usage or input error (bad flags, unreadable conversation, invalid decisions)
  2  extraction failed (provider error, or a payload the guard rejected)
  3  approval blocked (a date conflict is open, or -apply was used without -decisions)

Nothing is written without -apply, and -apply needs -decisions.
`

type options struct {
	input      string
	provider   string
	fixtureDir string
	model      string
	decisions  string
	ledger     string
	apply      bool
	format     string
	timeout    time.Duration
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	parsed, err := parseOptions(args, stdout)
	if err != nil {
		return report(stderr, err)
	}
	if parsed == nil {
		return exitOK // -h printed the usage already
	}

	conv, err := loadConversation(parsed.input, stdin)
	if err != nil {
		return report(stderr, err)
	}

	provider, err := newExtractor(parsed)
	if err != nil {
		return report(stderr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), parsed.timeout)
	defer cancel()

	candidates, err := provider.Extract(ctx, conv)
	if err != nil {
		return report(stderr, fmt.Errorf("%w: %s failed: %s", errExtraction, provider.Name(), err))
	}

	decisions, err := loadDecisions(parsed, conv)
	if err != nil {
		return report(stderr, err)
	}

	result, err := approve.Evaluate(conv, candidates, decisions)
	if err != nil {
		if errors.Is(err, approve.ErrUnresolvedConflict) {
			return report(stderr, fmt.Errorf("%w: %s", errApproval, err))
		}
		return report(stderr, err)
	}

	built := buildReport(parsed, provider.Name(), result, candidates)
	if parsed.apply {
		if err := applyToLedger(parsed, result, &built); err != nil {
			return report(stderr, err)
		}
	}

	if err := render(stdout, parsed.format, built); err != nil {
		return report(stderr, err)
	}
	return exitOK
}

// parseOptions returns nil options when the usage was requested.
func parseOptions(args []string, stdout io.Writer) (*options, error) {
	flags := flag.NewFlagSet("task-extract", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}

	parsed := &options{}
	flags.StringVar(&parsed.input, "input", "", "conversation JSON file, or - for stdin")
	flags.StringVar(&parsed.provider, "provider", "gemini", "extraction provider: gemini | fixture")
	flags.StringVar(&parsed.fixtureDir, "fixture-dir", "testdata/fixtures", "recorded provider payloads for -provider fixture")
	flags.StringVar(&parsed.model, "model", "", "Gemini model")
	flags.StringVar(&parsed.decisions, "decisions", "", "JSON file with the human approve/reject decisions")
	flags.StringVar(&parsed.ledger, "ledger", "tasks.jsonl", "JSONL ledger of approved tasks")
	flags.BoolVar(&parsed.apply, "apply", false, "record approved tasks in the ledger")
	flags.StringVar(&parsed.format, "format", "text", "output format: text | json")
	flags.DurationVar(&parsed.timeout, "timeout", defaultTimeout, "bound for one extraction call")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return nil, nil
		}
		fmt.Fprintf(stdout, "task-extract: %v\n\n", err)
		return nil, usageErrorf("%v", err)
	}

	switch {
	case strings.TrimSpace(parsed.input) == "":
		return nil, usageErrorf("-input is required (a conversation JSON file, or - for stdin)")
	case parsed.provider != "gemini" && parsed.provider != "fixture":
		return nil, usageErrorf("-provider must be gemini or fixture, got %q", parsed.provider)
	case parsed.format != "text" && parsed.format != "json":
		return nil, usageErrorf("-format must be text or json, got %q", parsed.format)
	case parsed.apply && strings.TrimSpace(parsed.decisions) == "":
		return nil, fmt.Errorf("%w: refusing to record tasks without explicit human decisions: pass -decisions <file>", errApproval)
	case parsed.timeout <= 0:
		return nil, usageErrorf("-timeout must be positive")
	}
	return parsed, nil
}

func writeUsage(w io.Writer) {
	fmt.Fprintf(w, usageTemplate, extract.DefaultGeminiModel, defaultTimeout)
}

func loadConversation(input string, stdin io.Reader) (domain.Conversation, error) {
	if input == "-" {
		return format.LoadConversation(stdin)
	}
	return format.LoadConversationFile(input)
}

func newExtractor(parsed *options) (extract.Extractor, error) {
	if parsed.provider == "fixture" {
		return extract.NewFixture(parsed.fixtureDir), nil
	}

	apiKey := os.Getenv("GOOGLE_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	model := parsed.model
	if strings.TrimSpace(model) == "" {
		model = os.Getenv("GEMINI_MODEL")
	}

	provider, err := extract.NewGemini(extract.GeminiOptions{APIKey: apiKey, Model: model})
	if err != nil {
		return nil, usageErrorf("%v", err)
	}
	return provider, nil
}

func loadDecisions(parsed *options, conv domain.Conversation) (approve.Decisions, error) {
	if strings.TrimSpace(parsed.decisions) == "" {
		return approve.Decisions{ConversationID: conv.ConversationID}, nil
	}
	decisions, err := approve.LoadDecisionsFile(parsed.decisions)
	if err != nil {
		return approve.Decisions{}, err
	}
	return decisions, nil
}

func buildReport(parsed *options, providerName string, result approve.Result, extracted []domain.TaskCandidate) format.Report {
	built := format.Report{
		ConversationID: result.ConversationID,
		Provider:       providerName,
		Mode:           format.ModeDryRun,
		Candidates:     extracted,
		Unique:         result.Unique,
		Duplicates:     result.Duplicates,
		Conflicts:      result.Conflicts,
		Pending:        result.Pending,
		Rejected:       result.Rejected,
	}
	built.Approved = candidatesOf(result.Approved)
	if parsed.apply {
		built.Mode = format.ModeApplied
	}
	return built
}

func applyToLedger(parsed *options, result approve.Result, built *format.Report) error {
	approved := result.Approved
	approve.SortByDueDate(approved)

	applied, err := approve.NewFileLedger(parsed.ledger).Apply(approved)
	if err != nil {
		return usageErrorf("%v", err)
	}
	built.Written = keysOf(applied.Written)
	built.Skipped = keysOf(applied.Skipped)
	return nil
}

func render(w io.Writer, name string, report format.Report) error {
	if name == "json" {
		return format.RenderJSON(w, report)
	}
	return format.RenderText(w, report)
}

func candidatesOf(tasks []approve.ApprovedTask) []domain.TaskCandidate {
	candidates := make([]domain.TaskCandidate, 0, len(tasks))
	for _, task := range tasks {
		candidates = append(candidates, task.Candidate)
	}
	return candidates
}

func keysOf(tasks []approve.ApprovedTask) []string {
	keys := make([]string, 0, len(tasks))
	for _, task := range tasks {
		keys = append(keys, task.IdempotencyKey)
	}
	return keys
}

// report writes the failure and returns its exit code.
func report(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "task-extract: %v\n", err)
	switch {
	case errors.Is(err, errApproval):
		return exitApprovalBlocked
	case errors.Is(err, errExtraction):
		return exitExtractionFailed
	default:
		return exitUsage
	}
}

// The error categories the exit codes are derived from.
var (
	errUsage      = errors.New("usage or input error")
	errExtraction = errors.New("extraction failed")
	errApproval   = errors.New("approval blocked")
)

func usageErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}
