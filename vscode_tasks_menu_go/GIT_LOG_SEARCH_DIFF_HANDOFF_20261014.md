# Git Log · search and commit diff (2026-10-14)
Repository: hacrot3000/PatchAndCollectionToolForAI · branch main

## Scope
- Fast commit-log filter without rerunning Git for every keystroke.
- Optional loading of older commits with stable paging.
- Per-commit Show Diff; one-file changes open File Compare, multi-file changes use Directory Compare, as in Branch Compare.
- Parent choice for merge commits and root-commit fallback to the existing Graph detail.
- All commit snapshot comparisons are read-only.

## Checkpoint A
Expanded `gitCompareTreeReference` to accept only **full validated SHA-1/SHA-256 commit IDs**, verify that the object is a commit, and return a pinned SHA; existing branch-only semantics remain for named refs. Reject untrusted revision expressions and abbreviations.
