# conversation-task-extractor

Turn conversation text (chat threads, email threads) into explicit, source-backed task
candidates — and refuse to turn suggestions into commitments.

[![ci](https://github.com/GhandyP/conversation-task-extractor/actions/workflows/ci.yml/badge.svg)](https://github.com/GhandyP/conversation-task-extractor/actions/workflows/ci.yml)

A model can read a thread and hand you a tidy list of tasks. The hard part is not the
list: it is knowing which entries are real, which quote actually exists, and which date
nobody ever said. This example is built around those questions.

## Why

Four failure modes, all of them reproduced in this repository's fixtures:

| Failure mode | What the repository does |
| --- | --- |
| The model quotes a sentence nobody wrote | The candidate is rejected before a human ever sees it. The quote must appear verbatim in the message it claims to come from, so a fabricated commitment cannot reach approval. |
| A maybe becomes a must | Tentative wording stays `tentative`. A wish is not a task at all: `"Estaría bueno tener un tablero de métricas algún día."` yields nothing. |
| The owner or the date is missing | They are marked `unknown` and `not stated`, counted in `Notes`, and never guessed. |
| Two dates for the same task | The conflict is reported, and approving both sides is refused until a human rejects one. |

On top of that: nothing is written unless you ask for it, and writing twice records
nothing twice.

## Quickstart (no API key)

The repository ships recorded provider payloads, so the whole pipeline runs offline.

```bash
go test ./...
go run ./cmd/task-extract \
  -input testdata/conversations/standup-2026-03-02.json \
  -provider fixture \
  -fixture-dir testdata/fixtures \
  -ledger /tmp/tasks.jsonl
```

Real output from `testdata/conversations/standup-2026-03-02.json`:

```text
conversation: standup-2026-03-02
provider: fixture
mode: dry run (nothing is written; pass -apply to record approved tasks)

candidates: 4 (unique 3, duplicates 1, conflicts 1)
pending: 3
approved: 0
rejected: 0
written: 0
skipped: 0

candidates:
- cand-1e74923793d4 [explicit] Send the March report
  owner: beto | due: 2026-03-06 | confidence: 0.86 | state: pending
  quote (m1): "mandame el informe de marzo el viernes 6"
- cand-9d2dce82e779 [explicit] Send the March report
  owner: beto | due: 2026-03-09 | confidence: 0.74 | state: pending
  quote (m2): "el informe lo entrego el lunes 9"
- cand-789e758e16a9 [tentative] Review the deck
  owner: unknown | due: not stated | confidence: 0.42 | state: pending
  quote (m3): "¿Podrías revisar el deck si llegás?"
  notes: owner not stated in the conversation; due date not stated in the conversation; tentative wording: not a commitment

duplicates:
- cand-1e74923793d4 was extracted 1 extra time(s)

conflicts:
- "Send the March report" (owner beto) carries 2026-03-06, 2026-03-09
  an open conflict blocks -apply until the dates that do not hold are rejected
```

Four candidates went in, one exact duplicate collapsed, one date conflict surfaced, and
the conversation's wish never became a task. Nothing was written, and no key was needed.

## The guard, in action

`testdata/fixtures/hallucinated-2026-03-04.json` records a model turning
`"Quizás podríamos migrar la base el mes que viene, vemos."` into a dated commitment
with the quote `"migrá la base el 1 de abril"`. That sentence does not exist:

```console
$ go run ./cmd/task-extract -input testdata/conversations/hallucinated-2026-03-04.json \
    -provider fixture -fixture-dir testdata/fixtures -ledger /tmp/tasks.jsonl
task-extract: extraction failed: fixture failed: provider payload rejected: the provider returned candidates that do not match the conversation: candidate[0].sourceQuote: "migrá la base el 1 de abril" does not appear verbatim in message "h1"
$ echo $?
2
```

The rejection is all-or-nothing: if one candidate is fabricated, the whole payload is
refused, because a model that invented one task cannot be trusted with the rest.

## Approving and recording

Decisions are a file, so approving is explicit, reviewable, and repeatable:

```json
{
  "conversationId": "standup-2026-03-02",
  "decisions": {
    "cand-1e74923793d4": "approve",
    "cand-9d2dce82e779": "reject",
    "cand-789e758e16a9": "approve"
  }
}
```

```console
$ go run ./cmd/task-extract -input testdata/conversations/standup-2026-03-02.json \
    -provider fixture -fixture-dir testdata/fixtures -ledger /tmp/tasks.jsonl \
    -decisions /tmp/decisions.json -apply
mode: applied
written: 2
skipped: 0

$ go run ./cmd/task-extract -input testdata/conversations/standup-2026-03-02.json \
    -provider fixture -fixture-dir testdata/fixtures -ledger /tmp/tasks.jsonl \
    -decisions /tmp/decisions.json -apply
mode: applied
written: 0
skipped: 2
```

The ledger is append-only JSONL, private (`0600`), and keyed by a stable idempotency key,
so replaying an approval is free:

```text
task-32aae65352397ec4 | Review the deck | (none) | tentative
task-5b5826137f908151 | Send the March report | 2026-03-06 | explicit
```

Asking to record tasks without decisions is refused, not guessed:

```console
$ go run ./cmd/task-extract -input testdata/conversations/standup-2026-03-02.json \
    -provider fixture -fixture-dir testdata/fixtures -ledger /tmp/tasks.jsonl -apply
task-extract: approval blocked: refusing to record tasks without explicit human decisions: pass -decisions <file>
$ echo $?
3
```

## The extraction contract

A conversation is one JSON object. `timeZone` is an IANA name; `sentAt` is RFC 3339.

```json
{
  "conversationId": "standup-2026-03-02",
  "source": "chat",
  "timeZone": "America/Argentina/Buenos_Aires",
  "messages": [
    {
      "messageId": "m1",
      "sender": "ana",
      "sentAt": "2026-03-02T09:15:00Z",
      "text": "Buen día. Dale, mandame el informe de marzo el viernes 6."
    }
  ]
}
```

`Conversation{conversationId, messages[]}` becomes
`TaskCandidate{title, owner, dueDate, status, sourceQuote, confidence}[]`, plus a derived
`candidateId` and the `sourceMessageId` the quote came from:

```json
{
  "candidateId": "cand-1e74923793d4",
  "title": "Send the March report",
  "owner": "beto",
  "dueDate": "2026-03-06",
  "status": "explicit",
  "sourceQuote": "mandame el informe de marzo el viernes 6",
  "sourceMessageId": "m1",
  "confidence": 0.86
}
```

`status` is one of `explicit`, `tentative`, or `ambiguous`. A candidate is refused when

- the quote is empty or does not appear verbatim in the referenced message (whitespace
  differences are tolerated, paraphrases are not);
- `sourceMessageId` is missing or unknown;
- `status` is empty or not one of the three values;
- `confidence` is outside `[0, 1]` or not a number;
- `title` or `owner` is empty, or `dueDate` is not a `YYYY-MM-DD` date.

`candidateId` is derived from the conversation, source message, quote, and title, so the
same candidate keeps the same id across runs and an exact duplicate collapses into one
task. Unknown provider fields are ignored; a payload that is not the documented shape is
rejected.

## How it works

```text
conversation JSON ──▶ domain.Validate ──▶ Extractor ──▶ extract.Decode ──▶ approve.Evaluate ──▶ ledger
                       (input guard)      gemini │         (quote guard,      (human decisions,      (JSONL,
                                          fixture         normalize, ids)     duplicates,            idempotent)
                                                                              conflicts)
```

| Package | Responsibility |
| --- | --- |
| `internal/domain` | Conversation and candidate types, validation, normalization, duplicate and conflict detection |
| `internal/extract` | The `Extractor` port, the provider prompt and response schema, the guarded decoder, the offline `fixture` provider, and the Gemini provider |
| `internal/approve` | Human decisions, the approval gate, idempotency keys, and the append-only JSONL ledger |
| `internal/format` | Conversation input loading and text/JSON report rendering |
| `cmd/task-extract` | Flags, wiring, exit codes |

`go.mod` has no dependencies.

## Safety model

- **Dry run by default.** Without `-apply`, nothing is created: `-ledger` is not even
  opened. `-apply` without `-decisions` exits `3` instead of writing.
- **No partial acceptance.** A payload is rejected as a whole, and a rejected payload
  never returns candidates.
- **Verbatim quotes.** The guard checks the quote against the actual message text.
- **Missing details are visible.** `owner: unknown`, `due: not stated`, and explicit
  notes, instead of a plausible invention.
- **Conflicts block.** Approving two candidates that disagree about one task's due date
  is refused until one side is rejected.
- **Idempotent writes.** The ledger skips idempotency keys it already holds, never
  rewrites, and is created `0600` because it holds conversation text.
- **A corrupt ledger is an error.** A line that cannot be parsed stops the run; the file
  is left byte-identical rather than repaired or overwritten.
- **Keys stay out of logs.** The Gemini API key travels in a header, never in a URL, and
  is never included in an error message.

## Using Gemini

The `gemini` provider is the default. It needs a key, and it is the only part of the
project that needs the network.

```bash
export GOOGLE_API_KEY="..."      # or GEMINI_API_KEY
go run ./cmd/task-extract -input testdata/conversations/standup-2026-03-02.json -provider gemini
```

- `-model` overrides the model; `GEMINI_MODEL` is the environment fallback; the default is
  `gemini-flash-latest`.
- `-timeout` bounds one call (default `60s`).
- The request is `generateContent` with `responseMimeType: application/json`,
  `responseSchema` — the same schema the prompt describes — and `temperature: 0`.
- Keeping the provider behind the `extract.Extractor` port is what lets the whole test
  suite run with no key and no network.

**Verification status.** The Gemini provider is unit-tested against a local HTTP server
(request shape, structured-output schema on the wire, API errors, and the guard). It has
not been exercised against the live API in this repository, and CI does not call it.

## CLI reference

`go build -o task-extract ./cmd/task-extract` produces the binary; every example in this
README uses `go run ./cmd/task-extract` instead, so nothing has to be installed.

```text
Usage:
  task-extract -input <file|-> [flags]

Flags:
  -input <path>        conversation JSON file, or - for stdin (required)
  -provider <name>     extraction provider: gemini | fixture (default "gemini")
  -fixture-dir <path>  recorded provider payloads for -provider fixture (default "testdata/fixtures")
  -model <name>        Gemini model (default $GEMINI_MODEL or gemini-flash-latest)
  -decisions <path>    JSON file with the human approve/reject decisions
  -ledger <path>       JSONL ledger of approved tasks (default "tasks.jsonl")
  -apply               record approved tasks in the ledger (default: dry run)
  -format <name>       output format: text | json (default "text")
  -timeout <duration>  bound for one extraction call (default 1m0s)
```

| Exit code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Usage or input error: bad flags, unreadable conversation, invalid decision record |
| `2` | Extraction failed: provider error, or a payload the guard rejected |
| `3` | Approval blocked: an open date conflict, or `-apply` without `-decisions` |

`-format json` prints the same run as a machine-readable report (`conversationId`,
`provider`, `mode`, `candidates`, `unique`, `duplicates`, `conflicts`, `pending`,
`approved`, `rejected`, `written`, `skipped`). Empty lists are `[]`, never `null`.

## Development

```bash
go test ./...    # offline, no API key
go vet ./...
gofmt -l .
```

`.github/workflows/ci.yml` runs `gofmt -l`, `go vet ./...`, `go test -race ./...`, and
`go build ./...` on every push to `main` and every pull request.

Test fixtures live in two paired directories: `testdata/conversations/` holds the
conversation, and `testdata/fixtures/<conversationId>.json` holds the recorded provider
payload for it.

The requirements, the work units, and the evidence behind every commit are tracked in
[`odd/tasks/conversation-task-extractor.md`](odd/tasks/conversation-task-extractor.md).

| Conversation | What it proves |
| --- | --- |
| `standup-2026-03-02` | Explicit tasks, tentative wording, a duplicate, a date conflict, and a wish that must not become a task |
| `quiet-thread-2026-03-03` | A thread with no task at all |
| `hallucinated-2026-03-04` | A fabricated quote, refused by the guard |

Adding a provider means implementing one interface:

```go
type Extractor interface {
	Name() string
	Extract(ctx context.Context, conv domain.Conversation) ([]domain.TaskCandidate, error)
}
```

## Limitations and roadmap

- **No connectors, no tracker.** Nothing reads mail or chat APIs, and nothing writes to
  Jira, Todoist, or any other tracker. The ledger is a local audit artifact.
- **Dates only.** `dueDate` is a calendar date read in the conversation's time zone; there
  is no time-of-day handling or timezone conversion.
- **Single user.** There is no authentication and no multi-user isolation; the ledger's
  file permissions are the whole access story.
- **The guard is not a semantic check.** It proves a quote exists in the message it claims
  to come from; it cannot prove the quote means what the candidate says it means. That is
  what the human approval step is for.
- **Sample fixtures are Spanish.** The prompt and the guard are language-agnostic; the
  fixtures demonstrate the failure modes in the language the example grew from.
- Next: optional connectors behind the same port, an ADK or SDK adapter for the Gemini
  call, and a `-format sarif`-style report for CI use.

## License

MIT — see [LICENSE](LICENSE).
