---
name: review-change
description: Review a completed repository code change before integration or a PR, prioritizing concrete correctness, architecture, scope, and test issues.
---

# Review a completed change

Inspect the change and enough surrounding code to evaluate it. Prioritize concrete findings with affected paths and an explanation of the failure or risk. Check correctness, scope, architecture consistency, error handling, concurrency safety, test coverage, unnecessary complexity, accidental API changes, and unrelated modifications. Do not rewrite working code merely because another style is possible.

When the relevant components exist, verify these project boundaries:

- App owns mutable `AppState`.
- UI does not directly control `Player`.
- Hardware-specific logic stays out of application core.
- Events remain typed and fake implementations remain usable.
- New goroutines have clear ownership and shutdown semantics.
- Package dependencies point in the intended direction.

Read `docs/architecture.md` if it exists and the change affects those boundaries. Report findings in priority order, then briefly state checks performed and any remaining verification gaps. If no concrete issues are found, say so plainly.
