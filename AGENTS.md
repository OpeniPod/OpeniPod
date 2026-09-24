# Repository agent instructions

Keep changes within the user's request. Inspect only the repository context needed for the current task. This checkout may not yet contain the Go packages or project documentation mentioned below; apply architecture checks to code that actually exists.

## Agent orchestration

The main agent (Sol in the usual setup) owns understanding the request, planning, architecture, task decomposition, coordination, integration, final checks, and the final answer. Decide whether delegation helps before using workers. Keep difficult reasoning and decisions about package boundaries, public or internal interfaces, dependency direction, state ownership and lifetime, concurrency, events, cross-cutting refactors, debugging when the root cause is unknown, security, and ambiguous requirements with the main agent. Workers may investigate these questions, but the main agent decides. Handle tiny tasks directly.

Luna workers handle bounded implementation, focused investigation, tests, and independent verification within a small, specified scope. Suitable work includes implementing a defined interface, adding reducer tests, fixing vet errors, or refactoring a local function without changing its API. Resolve ambiguity before delegation. Use zero to three workers as useful; parallel work must be genuinely independent. Do not assign concurrent edits to the same files. Run dependent tasks in order. Do not use all available workers merely because they exist.

For each delegated task, provide:

1. Goal and relevant context, with exact paths or interfaces where possible.
2. Files or packages the worker may modify, and areas to avoid unless necessary.
3. Constraints and acceptance criteria.
4. Checks to run and the expected report back.

For example, a worker could implement already-defined volume transitions in `internal/app`, clamp values to 0..100, add boundary tests, avoid changing `Event`, `Player`, or UI interfaces, and run `go test ./internal/app/...`. Do not delegate an open-ended application redesign or the choice of package boundaries. A worker must report what changed, files changed, checks run, failures or uncertainties, and anything left undone. Workers should follow the existing architecture, write idiomatic Go, avoid speculative abstractions and unrelated refactors, and report unrelated issues to the main agent. Workers must not delegate further unless the main agent explicitly asks.

After delegation, the main agent inspects the relevant code and diff, checks the acceptance criteria and architectural fit, resolves integration issues, runs appropriate combined checks, and reviews the final diff. A worker's claim that tests passed is not sufficient when the main agent can reasonably verify it.

Keep worker context small: send the task, constraints, and relevant paths rather than the whole repository. Read `docs/architecture.md` when it exists and a change affects package boundaries, application state ownership, events, or subsystem interfaces. Read other development or testing docs when relevant; small tasks do not require every document.

## Repository skills

Use the repository-local skill whose workflow matches the task:

- `orchestrate-luna` — decompose larger tasks and coordinate bounded Luna work.
- `implement-go-task` — implement a defined Go change within existing architecture.
- `test-go-change` — add or run meaningful Go tests and verification.
- `review-change` — review a completed change before integration or a PR.

Skills live under `.codex/skills/`. Do not create domain-specific hardware or subsystem skills until those workflows become repetitive.

## Project boundaries

- The application owns mutable `AppState`; UI renders it and does not call Player directly.
- Keep hardware-specific code outside the application core and keep interfaces small.
- Use a modular monolith, not microservices, and avoid global mutable state.
- Preserve useful fake implementations and run relevant tests for changes.
- Keep tasks bounded, avoid unrelated refactors, and reason deliberately about architecture changes.

## Git history

- Make each commit a meaningful, complete logical unit; do not create checkpoint or WIP commits for every edit.
- Use concise, descriptive subjects. Add a short body when the reason or behavior is not obvious.
- Before committing, inspect status and diff, run relevant checks, and exclude unrelated files.
- Clean up noisy local history before merge when safe, or report that squash or rebase is needed. Do not squash useful independent commits solely to reduce their number.
- AI agents may create local commits but must never push commits, branches, tags, or refs by any tool. A human performs every push.
- Before finishing PR-sized work, inspect branch history and report any commits that need cleanup.
