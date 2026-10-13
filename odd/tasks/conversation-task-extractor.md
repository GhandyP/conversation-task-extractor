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
Checks: the CLI depends on an `Extractor` interface and selects the Google implementation only through `-provider gemini`; the CLI is exercised end to end through the deterministic `fixture` provider, which is a real implementation of the same port; a malformed provider payload is rejected as a whole, and the CLI surfaces that rejection as exit code 2 without creating a ledger; a candidate whose quote is not present in the source message never reaches approval.
Amendment (2026-10-01, after independent verification): the earlier wording required "a fake extractor drives the CLI tests". The CLI is driven by the `fixture` provider instead, and the `fakeExtractor` stays in `internal/extract`. A test-only injection seam in the CLI would exist with no production caller, so the check was rewritten to the real contract instead of adding dead surface.

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
| T3 | Extraction port, prompt, structured-output schema, decoder and quote guard | S10, S5 | inline | done | commit `08a9c2b`; RED (`no non-test Go files`), then GREEN: `go test -count=1 ./...` ok for `domain` and `extract`; `go vet` + `gofmt` clean |
| T4 | Deterministic fixture provider plus the conversation fixture matrix | S9, S6, S7 | inline | done | commit `1a93a86`; RED (`no non-test Go files` for `format`, `extract`), then GREEN across `domain`, `extract`, `format`; fixture matrix covers explicit, tentative, wish-not-extracted, duplicate, conflicting dates, and no-task threads |
| T5 | Gemini provider on the official Go SDK behind the port | S10 | inline | done | commit `24744f7`; `extract` ok, `vet` exit 0, `gofmt` clean; implemented over REST instead of the SDK — see D4 |
| T6 | Approval gate: decisions, duplicate/conflict detection, idempotency keys, JSONL ledger | S7, S8 | inline | done | commit `f589490`; RED observed (`no non-test Go files`), then GREEN; `approve` ok |
| T7 | CLI (`cmd/task-extract`), rendering, exit codes | S8, S12 | inline | done | commit `b0daa71`; RED observed in the T6 run (`cmd/task-extract/cli_test.go:23:10: undefined: run`), then GREEN |
| T8 | README and docs rewrite; doc-vs-help consistency pass | S12, S9 | inline | done | commit `c8ff7d3`; race suite `ok` for all five packages, `vet exit=0`, `gofmt` clean; exit codes 0/1/2/3 observed with a built binary; README commands executed before being documented |
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

D4 (2026-10-01, decision, supersedes the SDK line in the task table): the Gemini provider speaks the REST `generateContent` endpoint through `net/http` instead of depending on `google.golang.org/genai`. Evidence: `go get google.golang.org/genai@latest` resolved v1.74.0 together with about fifteen indirect modules (`grpc`, `protobuf`, `cloud.google.com/go/auth`, `go.opencensus.io`, ...) for a single JSON request whose response schema this repository already builds. The REST shape accepts `ResponseSchema()` verbatim, keeps `go.mod` at zero dependencies, and keeps the whole suite runnable offline; the port still allows an SDK or ADK adapter later.

E1 (2026-10-01, T2 evidence): RED observed as a build failure (`internal/domain: no non-test Go files`), then GREEN with 14 passing tests, `go vet ./...` exit 0, and `gofmt -l .` empty. The domain tests found a real defect before GREEN: `Deduplicate` indexed an empty `duplicates` slice, and its first fix still reported zero-count groups; the final version keeps first-occurrence order and reports only ids with extra copies.

E2 (2026-10-01, T3 evidence): RED observed as a build failure for `internal/extract`, then GREEN with the `domain` and `extract` packages passing, `go vet` exit 0, `gofmt` clean. The decoder is all-or-nothing: an invalid conversation returns the validation error (not `ErrInvalidPayload`), while every provider-side problem returns a `*PayloadError` matching `ErrInvalidPayload` and no partial candidates.

R1 (2026-10-01, RDD assess over `da860522..24e17e3`): native tier `medium`, `reviewDue: false` (`under_budget`), plan `writerSelfVerification: true`, `independentVerifier: false`, writer profile `large` (`runtime`). Recorded as evidence, not as an override of the tier table.

E3 (2026-10-01, T4 evidence): RED observed as build failures for `internal/format` and `internal/extract`, then GREEN with `go test -count=1 ./...` ok for `domain`, `extract`, and `format`; `go vet` exit 0; `gofmt` clean. The fixture provider refuses a conversation id that is not a safe file name, so untrusted input cannot walk out of the fixture directory, and it separates provider configuration failures from payload rejections (`ErrInvalidPayload`).

B1 (2026-10-01, delivery budget): `git diff --shortstat da86052..HEAD` reports 1951 insertions and 3 deletions across 18 files, over the ~400-line advisory. Delivery strategy `chain_strategy=stacked-to-main` was chosen by the user (L5); slices stay per work unit on `main`, and the oversized total is the point of a first publish of a complete example, so no artificial splitting follows.

E4 (2026-10-01, T5 evidence): `go test -count=1 -timeout 120s ./...` returned `ok` for `domain`, `extract`, and `format` with `test exit=0`, and `go vet ./...` returned `vet exit=0`, after a 6m19s run caused by machine contention (load average 25.4 on 4 CPUs from other concurrent sessions). `gofmt -l .` flagged `internal/extract/gemini.go`, fixed with `gofmt -w` before the commit. RED for this task was attempted twice and is not claimed: both runs were starved (`timeout` exit 124 with no output) while unrelated sessions saturated the machine, so the pre-implementation failing state is evidenced by the missing package rather than by a captured run.

M1 (2026-10-01, memory status): the Engram mirror of this document is **pending** after the latest refresh. `mem_save` for topic `odd/conversation-task-extractor/tasks` timed out after 15000 ms waiting for the local Engram server (`http://127.0.0.1:7437`), and the follow-up `mem_search` verification timed out the same way, so the write is unconfirmed rather than failed. This file remains the authoritative copy; observations 4230, 4231, and 4232 cover the earlier state. Resynchronize when Engram answers, and do not claim the mirror is current before that.

E5 (2026-10-01, T5-T7 evidence): after `gofmt -w internal/extract/gemini.go`, the full run `go test -count=1 -timeout 300s ./...` reported `ok` for `cmd/task-extract`, `internal/approve`, `internal/domain`, `internal/extract`, and `internal/format`, with `test exit=0`, `vet exit=0`, and an empty `gofmt -l .`. A verbose pass counts 77 passing test cases and 0 failures. One real compile defect was found and fixed on the way: `cli.go` declared a local `report` variable that shadowed the `report(stderr, err)` exit-code helper (`cannot call report (variable of struct type format.Report)`).

E6 (2026-10-01, T6 evidence): RED observed for the package itself (`internal/approve: no non-test Go files`, `red exit=1` with the implementation moved aside and restored), then GREEN (`ok internal/approve`). The decision-record loader and the conflict signal were added under a second RED: `undefined: approve.LoadDecisions`, `undefined: approve.LoadDecisionsFile`, `undefined: approve.ErrUnresolvedConflict`, then GREEN.

E7 (2026-10-01, T7 evidence): the RED for this task came from the same run (`cmd/task-extract/cli_test.go:23:10: undefined: run`), then GREEN after the CLI, the renderer, and the domain JSON tags for `Duplicate` and `DateConflict` landed.

B2 (2026-10-01, delivery budget, continued): the branch now reports 4138 insertions and 3 deletions across 30 files since the root commit. Commits are `da86052`, `24e17e3`, `08a9c2b`, `1a93a86`, `24744f7`, `f589490`, `b0daa71`; each work unit was verified before the next one started, which is the chosen `stacked-to-main` strategy (L5).

I1 (2026-10-01, background jobs): a background verification job (`job-2`) was killed before it finished its `vet` and `gofmt` steps, and a later `job_list` showed no jobs at all. Consequence: long verification commands now run synchronously inside the turn, and no result is claimed from a job that did not report its own exit status.

V1 (2026-10-01, T9 independent verification, `gentle-ai-verify`): every probe ran in isolated scratch copies of HEAD and of the root commit, with both API-key variables unset and `GOPROXY=off`. Met: S1 (module with no dependencies and no `replace` directive; no cookbook import), S6, S7, S8 (dry run leaves no ledger; apply writes two records at mode `0600`; replay writes zero and skips two; the ledger stayed byte-identical across a dry run, a refused apply, and the failed extraction), S9, and S11 (no credential in the committed tree or in the full history; the only pattern matches were the README placeholder and the literal pattern names in this document). Not met: S4 (JSON tests asserted field names through maps, not a typed round trip), S5 (the guard's negative cases were proven only at unit level), S10 (the spec demanded a fake extractor in the CLI tests), S12 (a README example invoked a bare `task-extract` that a clone does not install), S3 (the Engram mirror is pending, and the visible `todo` is session UI rather than a file the verifier can read), plus S2, which is the task that had not run yet.

C1 (2026-10-01, one correction batch for every verifier blocker): added `TestConversationAndCandidateTypedJSONRoundTrip`; added `TestRunRejectsUnsafeProviderPayloads`, a table that drives an unknown status, an empty status, an unknown source message, a fabricated quote, and an out-of-range confidence through the command line, asserting exit code 2, the named field, and no ledger; fixed the README example to `go run ./cmd/task-extract` and documented how the binary is built; amended the S10 check with its reason. Evidence after the batch: `go test -race -count=1 ./...` ok for all five packages (`race test exit=0`), `vet exit=0`, `gofmt -l .` empty, 79 passing test cases and 0 failures.
