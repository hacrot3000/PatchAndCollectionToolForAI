# TaskDeck — outstanding implementation work

This file is an **active backlog only**. Completed features and their original design notes were removed as requested on 2026-10-08. For historical details, see [the archived roadmap in Git history](https://github.com/hacrot3000/PatchAndCollectionToolForAI/blob/120c9b549a01883ad9e49178c485c631ca26fd59/implementing.md).

## Remaining verification

- [ ] Restore a green full `vscode_tasks_menu_go/internal/server` Go test suite. The current main-branch CI is failing UI contract assertions not all attributable to the file-compare work. Inspect the latest CI run instead of accepting the old "11 pre-existing failures" note without rechecking.
- [ ] Verify the arbitrary-file-compare flow after the test-contract cleanup: two selected files in Project Explorer or the FTP/SFTP panes; Select for compare → Compare with selected file across surfaces; editor-tab source; editing either writable side, dirty indicator, Left/Right Save, and optimistic-lock rejection for concurrent external updates.
- [ ] Re-run GitHub Actions for the resulting `main` commits and update/remove the remaining items only when validated.

## Progress checkpoints

- `17f552e2` — added regression coverage for arbitrary, editable compare workflows.
- `a9b3dd4d` — corrected stale comparison test assertions to match current module contracts (tests previously expected code in the wrong module or obsolete variable names).

Do not re-add completed roadmap sections here. Commit each remaining fix independently and retain only unverified tasks.
