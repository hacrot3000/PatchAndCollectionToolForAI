# Git Quick Actions · consolidated branch operations
Date: 2026-10-14
Target: `main`

## Request
- Replace Merge From and Merge To branch-row buttons with **Merge**, opening an explicit direction-choice wizard.
- Replace three local branch delete buttons with **Delete**, opening a wizard with safe-local, force-local, and upstream-remote choices. For remote-tracking rows, consolidate forget-local-ref and delete-on-remote into their own Delete wizard.
- Clicking a branch name copies that exact branch name. Remove redundant Copy name action.
- Preserve existing preflight, confirmation dialogs and non-reloading branch deletion mechanics.

## Checkpoint 1
- Added `openBranchMergeWizard(branch)`, reusing `TaskDeckGitBranchChoiceWizard`.
- Choice of Merge From / Merge To is explicit, defaults to From, guards repository switching, and delegates to unchanged `mergeBranch()` / `mergeToBranch()` logic for all validation and confirmations.
