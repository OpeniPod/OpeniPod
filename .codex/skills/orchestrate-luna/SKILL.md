---
name: orchestrate-luna
description: Decompose a substantial repository task into bounded Luna implementation, investigation, test, or verification work when delegation or parallel work can save time.
---

# Orchestrate Luna workers

Use this workflow as the main agent. Follow the responsibility and delegation rules in `AGENTS.md`.

1. Understand the request and inspect only relevant repository context. Resolve ambiguous requirements and make architecture, interface, package, event, and concurrency decisions at the main-agent level.
2. Identify dependency boundaries. Choose zero to three Luna workers; use parallel workers only for genuinely independent tasks. Sequence tasks when one depends on another. Never assign concurrent edits to the same files.
3. Give each worker a bounded assignment: goal, relevant context, allowed files or packages, areas to avoid, constraints, acceptance criteria, checks, and expected report. Delegate mechanical implementation, focused investigation, tests, or independent verification. Do not send the whole repository when a few paths suffice.
4. Collect reports, inspect the relevant code and diff, verify acceptance criteria and architectural fit, resolve conflicts, integrate changes, run appropriate combined checks, and inspect the final diff. Retain responsibility for the final answer.

Do not delegate when the task is tiny, delegation costs more than doing it directly, the next step depends heavily on the previous one, or multiple workers would need to mutate the same core code.

Delegate when there are independent investigations, independent packages to implement, tests that can be written independently, a mechanical implementation already designed, or verification that can happen independently. Availability alone is not a reason to use all three workers.
