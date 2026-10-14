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

## Checkpoint 2
- Added `openBranchDeleteWizard(branch)`, using the same accessible single-choice wizard.
- Local branch choices are **Delete local · safe** (`-d`), **Delete local · FORCE** (`-D`), and **Delete upstream remote branch** only if an upstream is known.
- Remote-tracking row choices are **Forget local tracking ref** and **Delete actual remote branch**, preserving both operations.
- Choice defaults to least destructive option, requires explicit selection/Continue and existing downstream confirmation. Repo switch cancels execution.
- Existing remote deletion -> optional local cleanup wizard remains untouched.
