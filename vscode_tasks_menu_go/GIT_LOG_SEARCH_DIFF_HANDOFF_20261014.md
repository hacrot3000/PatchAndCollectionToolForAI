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

## Checkpoint B · paged history
- `view=log&limit=N&skip=OFFSET` now uses bounded `git log --skip=N -n N+1` (limit ≤100, offset ≤10000), returns `has_more` and `skip`, and rejects invalid offsets.
- No background log polling. The browser keeps 100 commits at a time and can fetch older pages on demand. Search is in-memory across loaded commits, so typing does not start additional Git processes.

## Checkpoint D · Finished UI & validations
- New `web/featuremods/gitlog.js`, registered before `gitstatus.js`, controls the Log view. It caches rows by repository, retains the typed filter when tabs change, and fetches older commits on demand.
- New `DirectoryCompare.openGitCommit(repoID,parentSHA,commitSHA)` directly requests the existing read-only `compare-tree-diff` endpoint, showing only changed paths. This avoids scanning huge lists of identical files for typical commits.
- Tests: `test_git_log_ui.cjs` exercises live filtering, multi-file/single-file routing, selected merge parent, root fallback, and pagination. The workflow includes this Node test. Go tests validate full-SHA compare, rejection of unsafe refs, log offset pagination, and UI module wiring.
- The `TestDirectoryCompareLoadsAsIndependentWorkspaceTab` contract was updated to include `openGitCommit` in the exported API.
- Limitations: the in-view filter covers **loaded** commits, not unseen history. Use `Load older commits` to extend scope. For root commits, changed files are presented in the existing Graph detail list. The multi-file full view supports the same text/binary limitations as Directory Compare.
- Tests on real browser sessions / large Git repositories have not yet been executed; use CI and an in-app smoke test before deploying into production.
