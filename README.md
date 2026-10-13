# conversation-task-extractor

Turn conversation text (chat threads, email threads) into explicit, source-backed task
candidates — and refuse to turn suggestions into commitments.

> Status: this repository is being rebuilt as a standalone example. The tracked plan,
> specs, and progress log live in [`odd/tasks/conversation-task-extractor.md`](odd/tasks/conversation-task-extractor.md).
> The full README lands in the documentation work unit of that plan.

## Why

Extracting tasks from conversations is easy to fake and hard to get right: a model will
happily invent a due date, resolve a maybe into a must, or quote a sentence nobody said.
This example keeps the honest parts:

- every candidate carries the exact quote it came from, and a quote that is not verbatim
  in the source message is rejected before it can reach a human;
- tentative wording stays tentative, and missing owners or dates stay visibly missing;
- nothing is written unless a human approves it, and writing is idempotent.
