# Git Safe Operations — v6.20.0

Python Patch Tool exposes Git only through a fixed COLLECT action allowlist. It does **not** accept raw Git command strings or argv supplied by a request.

## Manifest shape

```json
{
  "type": "git",
  "repo": "projects/m3-client",
  "operations": [
    {"op": "status"},
    {"op": "log", "ref": "HEAD", "max_entries": 30},
    {"op": "diff_refs", "from": "main", "to": "feature/foo"}
  ]
}
```

`repo` is project-relative and must resolve to the exact Git worktree root. This permits nested repositories while preventing traversal outside the project.

## Allowed operations

- `status`
- `current_branch`
- `branches`
- `log` — exact safe ref, bounded entry count, optional project-relative paths
- `show` — exact safe ref, optional project-relative paths
- `diff_worktree`
- `diff_staged`
- `diff_refs` — between two safe refs
- `diff_ref_worktree` — ref versus current worktree
- `switch` — only an already-existing **local** branch and only when index/worktree/untracked state is clean

Diff/show execution disables external diff/textconv helpers. Git pager and optional locking are disabled for collection. Hooks are disabled with `core.hooksPath=/dev/null`.

## Forbidden

Mutation operations including `add`, `commit`, `merge`, `rebase`, `reset`, `push`, `pull`, `cherry-pick`, and `checkout` are rejected. Escape fields such as `argv`, `command`, and `raw_git` are rejected.

`switch` is the only state-changing exception requested by the operator. It uses `git switch --no-guess`, cannot create/force/detach a branch, rejects remote-only/nonexistent targets, and refuses to run when there are tracked, staged, or untracked local changes.

## PATCH-side Git policy

Historical PATCH automatic Git add/commit/push is retired in v6.20.0 by explicit safety requirement. Legacy manifest fields remain recognizable only so the validator can return a precise `git_mutation_forbidden` error; they are never executed.
