---
name: test-go-change
description: Add meaningful Go unit, regression, or integration tests, or verify an existing Go change, including race and shutdown checks when concurrency is involved.
---

# Test a Go change

1. Establish expected behavior and the change's scope. Identify important success, boundary, and failure cases. Reproduce a bug before adding a regression test when practical.
2. Choose the right level: unit tests for isolated behavior, integration tests for interactions, and manual hardware tests for behavior that requires real devices. Do not fake hardware-only behavior into meaningless unit tests to raise the test count.
3. Prefer table-driven tests when cases share a setup. Use existing fakes where possible. Assert observable behavior instead of coupling tests to implementation details.
4. Run focused package tests first, then broader tests when appropriate. Use `go test ./...` and `go vet ./...` when the repository and task warrant them. Run `go test -race ./...` for concurrency-related changes when feasible; report any skipped checks.
5. For concurrency changes, consider goroutine leaks, channel closure ownership, blocked sends or receives, context cancellation, data races, and shutdown behavior.
6. Report test files changed, commands and results, remaining gaps, and hardware checks that need a real device.
