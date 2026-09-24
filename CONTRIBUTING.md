# Contributing

Take one meaningful task per branch and pull request. Keep changes focused: do not mix an unrelated refactor with a feature. Prefer small, understandable commits. Add tests for application logic and run relevant checks before opening a PR. Discuss large architecture changes separately. Tasks are available to any contributor; components are not assigned to individuals.

## Commit and PR policy

Each commit should represent one complete, reviewable idea and explain what changed and why. Do not commit after every file edit or intermediate fix. A PR may contain one or several commits when each stands as a useful logical unit. Avoid commits such as `fix`, `wip`, `fix tests`, or `format` when they are merely steps in the same change.

Use a concise, descriptive, imperative subject of at most 50 characters,
without trailing punctuation. Separate it from the body with a blank line.
Wrap body lines at 72 characters. Add a short body when the reason, behavior,
or architectural consequence is not clear from the subject. Before committing,
inspect `git status` and the relevant diff, run appropriate checks, and exclude
unrelated files.

Before merge, review the branch history with `git log --oneline <base>..HEAD`. Squash or rework noisy intermediate commits, such as a sequence of implementation, compilation fixes, test fixes, and formatting for one small task. Preserve good independent commits; there is no one-commit-per-PR rule. If rewriting history is unsafe or has not been requested, tell the maintainer what cleanup is needed.

Give the PR a title that states its purpose. The description should be brief and grounded in the actual diff and checks:

```markdown
## What

What changed.

## Why

Why it was needed.

## Testing

Commands run and their results; state any checks not run.
```

AI agents may prepare local commits, commit messages, and PR text. They must never push commits, branches, tags, or refs, including through scripts, aliases, or an API. Only a human pushes. If a task requires a push, the agent stops after preparing the local state and reports: “Local commits are ready. Push must be performed by a human.”

For Markdown prose, 80 characters is a useful target when it improves reading
or diffs. Longer lines are fine for links, tables, code, or clearer wording.
Do not reflow unrelated text solely to satisfy a target.
