# TaskDeck Remote — mainline development

## Merge record (2026-11-06)

- Source: `feature/taskdeck-remote-access-architecture` at
  `ff9b31fa486f3aac31d7bd04fb0a6124aa063c8c`.
- Target before merge: `main` at
  `2de1825038ffa4a2c7f9c2c385006f9409f57d6d`.
- GitHub PR #146 was merged with method `merge`, preserving full commit
  history, at merge commit
  `5569c99e4c0a6e379996968ba084a2fc8004addf`.
- Source branch was 759 commits ahead / 0 behind, and its last code
  revision `6769022abc26efa125a36fa2ede028b46a92eb85`
  passed both TaskDeck Remote Agent (Go) and Remote Portal (PHP) CI.
  The source HEAD after that code commit only added documentation.

## Authoritative working branch

**From this merge onward, development of TaskDeck Remote is done directly
on `main`.** Do not open new feature branches for this workflow unless
the repository owner explicitly changes the decision. Do not continue
committing to `feature/taskdeck-remote-access-architecture`.

Remote Agent and Remote Portal push CI now target `main`; normal
`pull_request` triggers remain for externally submitted changes.
No force-push or history rewriting is authorized by this policy.

Before a direct `main` commit: read current HEAD, use a compare-and-swap
expected SHA on ref updates, keep E2EE/RBAC/audit boundaries, add bounded
regression tests, then inspect the CI for the committed revision. Never
silently mix incompatible pre-E2EE protocols from the historical `main`
implementation.

## Remaining code work (priority order)

1. Tasks — investigate and restore genuinely missing dependent-workflow
   reports and managed-task behavior; no blind protocol copying.
2. Git — larger paged reviews, remaining read-only interactive reviews and
   narrowly authorized mutations.
3. Files — safe deeper navigation, file-content reads within explicitly
   allowed subfolders and reviewed E2EE binary transfer contracts.
4. Editor/GUI — compare-and-swap guarded edits, conflict handling, and
   comprehensive file browser/editor interface.
5. Security — filesystem races, session revocation, quotas, concurrency,
   comprehensive permission and release-gate regression coverage.

Features whose code and automated checks are complete but only need
real-host testing are considered **code complete**.
