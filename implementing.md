# TaskDeck — outstanding implementation work

The 40 original roadmap milestones and the 2026-10-07 syntax-aware/arbitrary-file-compare follow-ups have been completed and removed from this active backlog. Their design details remain available in [Git history](https://github.com/hacrot3000/PatchAndCollectionToolForAI/blob/120c9b549a01883ad9e49178c485c631ca26fd59/implementing.md).

The full automated CI matrix passed on **Go 1.19 and Go 1.23**, including the final File Compare regression tests: [GitHub Actions run 37764409588](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/37764409588) (commit `643f8fea`).

## Remaining manual acceptance

- [ ] In a live TaskDeck browser with an actual FTP/SFTP connection, exercise cross-pane A/B selection, two-file direct Compare, selection from an Editor tab, editing/copying hunks, independent Save Left/Right, cancel/reload with unsaved changes, and optimistic-lock rejection after an external file change. Automated Go UI contract tests and server integration tests have passed, but this end-to-end real remote-server scenario has not been personally exercised in the current session.

Only newly discovered issues or unverified acceptance items should be added below. Commit fixes in small, independently verifiable steps.
