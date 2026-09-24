---
name: implement-go-task
description: Implement a bounded, already-defined Go change within existing package boundaries, with focused checks and a clear worker report.
---

# Implement a bounded Go task

1. Read the delegated goal, scope, constraints, acceptance criteria, and requested checks. Clarify any architectural ambiguity with the main agent before changing code; do not expand scope for unrelated improvements.
2. Inspect only relevant interfaces, implementation, and nearby tests. Confirm current behavior before editing.
3. Make the smallest coherent change that preserves package boundaries and existing APIs unless the assignment explicitly changes them. Use idiomatic Go and avoid unnecessary dependencies.
4. Format changed Go files with `gofmt`. Run focused tests first, then broader tests when the change warrants them or the assignment requests them.
5. Report what changed, files changed, checks run and results, failures or uncertainties, and anything deliberately left undone.

Prefer simple structs and functions and small interfaces. Accept an interface where it helps decouple the caller; avoid giant provider APIs, global mutable state, unnecessary reflection or generics, and `utils` or `common` dumping grounds. Wrap errors with context using `%w` when appropriate. Use `context.Context` for cancellable operations, not arbitrary storage. Start goroutines only when ownership and shutdown behavior are clear.
