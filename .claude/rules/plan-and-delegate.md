# Plan first, then delegate to the cheapest capable model

Every task in this repo should be planned before any work starts. Each piece of
the plan then goes to the cheapest model that can do it well. The goal is to
keep usage cost down without lowering quality.

## 1. Plan
- Before running anything, break the task into concrete steps.
- Give the plan in a few lines, naming the model that will handle each step.

## 2. Delegate by cost
Use the Agent tool with a `model` override. Pick the cheapest model that fits:

| Model | Use for |
|-------|---------|
| **Haiku** | Mechanical work: code searches, simple or repetitive edits, running `go build` / `go test` / `go vet`, Docker builds, summarizing logs or output. |
| **Sonnet** | Standard work: implementing features, refactors, writing tests, moderate debugging. |
| **Opus** (main session) | Architecture decisions, tricky debugging, final review, and integrating the subagents' results. |

If a step could go to two tiers, pick the cheaper one. Escalate only if that
model fails or the result isn't good enough.

## 3. Do tiny tasks inline
Subagents start with no context. Delegating a single file read, a one-line
edit, or a quick command costs more than doing it yourself, so handle those in
the main session.

## 4. Write self-contained prompts
Give each subagent everything it needs so it doesn't spend tokens finding it
again:
- Exact file paths (e.g. `main.go`, `internal/...`).
- The exact goal and what counts as done.
- The expected output format (a diff summary, test results, a list of matches).

## 5. Review and integrate
The main session reviews what subagents return, checks the result (build and
tests pass), and owns the final answer.
