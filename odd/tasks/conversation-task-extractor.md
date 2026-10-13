# Feature — conversation-task-extractor (standalone public example)

Objective: rebuild `/home/ghandy/Documents/07-task-extraction` as the standalone, dependency-free Go example repo `github.com/GhandyP/conversation-task-extractor`, published publicly on GitHub.
Branch: `main` — delivery strategy `chain_strategy=stacked-to-main` (verified work-unit commits land on the default branch; each slice verified before the next commit).
Test runner: `go test ./...` (offline, no API key), plus `go vet ./...`, `gofmt -l .`, and `go run ./cmd/task-extract` fixture runs.
Forecast authored changed lines: ~1600 (additions plus deletions), generated files excluded.

## Specs

S1 — Independent repository. This folder must be its own Git repo, with no dependency on the surrounding workspace.
Quoted intent (L1): "vamos a volver a hacerlo desde el principio para que sea su propio repo intependiendte".
Checks: `git rev-parse --show-toplevel` (from this folder) prints this folder; `go build ./...` succeeds; no import of `github.com/GhandyP/adk-go-enterprise-cookbook`; `go.mod` has no `replace` pointing outside the module.

S2 — Public GitHub repository created with the `git` and `gh` CLIs.
Quoted intent (L1): "vamos a crear un repo publico en github con el cli git y gh"; name chosen by the user: "conversation-task-extractor".
Checks: `gh repo view GhandyP/conversation-task-extractor --json nameWithOwner,visibility,url` reports `PUBLIC`; `git remote -v` shows `origin`; `git ls-remote origin` lists `refs/heads/main`; local `main` and `origin/main` point at the same commit.

S3 — ODD tracking of this work.
Quoted intent (L1): "y usar odd para la creacion de este repo de ejemplo para mi github".
Checks: this document exists with `## Specs`, `## Tasks`, `## Log`; the Engram mirror observation exists under topic `odd/conversation-task-extractor/tasks`; the visible `todo` projection matches `## Tasks`.

S4 — Domain contract (quoted from `README.md`, verbatim):
"**Contract:** Define `Conversation{conversationID, messages[]}` -> `TaskCandidate{title, owner, dueDate, status, sourceQuote, confidence}[]`."
Checks: exported types carry those JSON field names; a round-trip JSON decode/encode test passes. `status` only accepts the documented values; `confidence` is in `[0,1]`.

S5 — Source-quote and status integrity (quoted from `README.md`, verbatim): "Each returned task has a source quote and a valid status."
Checks: a candidate with an empty status, an unknown status value, a missing `sourceMessageId`, a `sourceMessageId` absent from the conversation, or a `sourceQuote` that is not a verbatim substring of that message's text is rejected with a typed validation error naming the field; the rejection mutates nothing.

S6 — Tentative wording and missing details (quoted from `README.md`, verbatim): "Tentative requests and missing owners/dates are clearly marked."
Checks: a tentative-wording fixture yields `status: "tentative"`; an absent owner renders as `unknown`; an absent due date renders empty and adds a note; both are visible in the text and JSON output.

S7 — Duplicate and conflicting-date behavior (quoted from `README.md`, verbatim): "Fixtures prove duplicate and conflicting-date behavior."
Checks: fixtures drive tests where (a) two candidates with the same source message and quote share one idempotency key and collapse to one approved task with the duplicate reported, and (b) two candidates with the same title and owner but different due dates are reported as a conflict.

S8 — Persistence safety (quoted from `README.md`, verbatim): "Persistence is dry-run by default and requires human confirmation."
Checks: a default run writes no ledger file and leaves an existing ledger byte-identical; `--apply` without `--decisions` exits non-zero with the documented message and writes nothing; `--apply --decisions` writes once, and a second identical run is idempotent (no new record, no duplicate).

S9 — Offline demo and tests (assumption: a portfolio example must be runnable by a reader with no credentials).
Quoted intent (L4): "lo que creas que sea lo mejor para que este proyecto sea un buen ejemplo de mi portafolio de github".
Checks: `go test ./...` passes with `GOOGLE_API_KEY` unset; `go run ./cmd/task-extract -provider fixture ...` produces candidates with no network access.

S10 — Provider boundary (assumption: the extraction backend sits behind a port so the example is testable and provider-swappable).
Checks: the CLI depends on an `Extractor` interface; a fake extractor drives the CLI tests; the Gemini implementation is selected only by `-provider gemini`; a malformed provider payload is rejected instead of partially accepted; a candidate whose quote is not present in the source message is rejected by the guard before it reaches approval.

S11 — No credential leakage into the published repository.
Checks: independent verification greps the published tree and Git history for API-key patterns (`AIza`, `gho_`, `ghp_`, private-key headers) and reports none; `.gitignore` excludes local ledgers, `.env`, and key files.

S12 — Portfolio-grade documentation. `README.md` states what the project is, quickstart without a key, the extraction contract, architecture, the safety model, exit codes, limitations, and roadmap; every documented command is copy-pasteable and was executed.
Constraint (harness policy): changing a command, option, or message updates its help text and the docs that describe it in the same work unit.

Out of scope (not requested): real email/chat API readers, tracker integrations (Jira/Todoist/etc.), multi-user auth, and any web UI.

## Tasks

| ID | Task | Specs | Route | Status | Evidence |
|----|------|-------|-------|--------|----------|
| T1 | Scaffold standalone module: `go.mod`, `.gitignore`, `LICENSE`, CI workflow, remove the old `main.go` prototype | S1 | inline | done | commit `da86052`; `go env GOMOD` = this module; no `adkhelper` import left; `go build ./...` exit 0 |
| T2 | Domain types and validation (`Conversation`, `Message`, `TaskCandidate`, `Status`) | S4, S5, S6 | inline | done | commit `24e17e3`; RED first (`no non-test Go files`), then GREEN with 14 tests in `internal/domain`; `go vet` and `gofmt` clean |
| T3 | Extraction port, prompt, structured-output schema, decoder and quote guard | S10, S5 | inline | pending | |
| T4 | Deterministic fixture provider plus the conversation fixture matrix | S9, S6, S7 | inline | pending | |
| T5 | Gemini provider on the official Go SDK behind the port | S10 | inline | pending | |
| T6 | Approval gate: decisions, duplicate/conflict detection, idempotency keys, JSONL ledger | S7, S8 | inline | pending | |
| T7 | CLI (`cmd/task-extract`), rendering, exit codes | S8, S12 | inline | pending | |
| T8 | README and docs rewrite; doc-vs-help consistency pass | S12, S9 | inline | pending | |
| T9 | Independent verification of the finished tree before publishing | S11 | delegated (`gentle-ai-verify`) | pending | |
| T10 | Create `github.com/GhandyP/conversation-task-extractor` (public) and push `main` | S2 | inline | pending | |

## Log

L1 (2026-10-01, user, verbatim): "mira explora este repo , vamos a volver a hacerlo desde el principio para que sea su propio repo intependiendte , ademas vamos a crear un repo publico en github con el cli git y gh , y usar odd para la creacion de este repo de ejemplo para mi github"

L2 (2026-10-01, user choice, verbatim option): "Usar esta carpeta" — target root is `/home/ghandy/Documents/07-task-extraction`, made an independent nested Git root.

L3 (2026-10-01, user choices): repository name "conversation-task-extractor"; parent-repo protection "Ignorar la carpeta en el repo home". Applied: `/home/ghandy/.gitignore` line 5 `Documents/07-task-extraction/`; the parent repo's index was not touched.

L4 (2026-10-01, user, verbatim): "lo que creas que sea lo mejor para que este proyecto sea un buen ejemplo de mi portafolio de github" — product scope delegated to the orchestrator.

L5 (2026-10-01, user choice): delivery strategy `chain_strategy=stacked-to-main`.

D1 (2026-10-01, orchestrator, exploration evidence): the folder held only `README.md` and `main.go`; `main.go` imported `github.com/GhandyP/adk-go-enterprise-cookbook/internal/adkhelper`, a different repository, so the prototype was not standalone; there was no `go.mod`, no tests, and no `.gitignore`. `/home/ghandy` is itself a Git repository, which is why isolated initialization and parent protection were required first.

D2 (2026-10-01, decision): the extraction backend is the official Gemini Go SDK behind an `Extractor` port instead of ADK. Reason: the prototype's ADK helper lives in a private personal module, which would break the standalone requirement (S1); a port keeps the example testable offline and allows an ADK adapter later. `README.md`'s "ADK layer" roadmap line is superseded here and rewritten in T8.

D3 (2026-10-01, decision): persistence is a local append-only JSONL ledger with idempotency keys, never a tracker integration. Reason: `README.md`'s "done when" criterion "Persistence is dry-run by default and requires human confirmation." would otherwise be vacuous, and `README.md`'s "does not ... persist tasks in a tracker" constraint stays intact.

E1 (2026-10-01, T2 evidence): RED observed as a build failure (`internal/domain: no non-test Go files`), then GREEN with 14 passing tests, `go vet ./...` exit 0, and `gofmt -l .` empty. The domain tests found a real defect before GREEN: `Deduplicate` indexed an empty `duplicates` slice, and its first fix still reported zero-count groups; the final version keeps first-occurrence order and reports only ids with extra copies.
