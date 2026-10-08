# TaskDeck — outstanding implementation work

This file is an **active backlog only**. Completed features and their original design notes were removed as requested on 2026-10-08. For historical details, see [the archived roadmap in Git history](https://github.com/hacrot3000/PatchAndCollectionToolForAI/blob/120c9b549a01883ad9e49178c485c631ca26fd59/implementing.md).

## Remaining verification

- [ ] Verify the arbitrary-file-compare flow after the test-contract cleanup: two selected files in Project Explorer or the FTP/SFTP panes; Select for compare → Compare with selected file across surfaces; editor-tab source; editing either writable side, dirty indicator, Left/Right Save, and optimistic-lock rejection for concurrent external updates.
- [ ] Confirm the Go 1.19/1.23 matrix on the latest compare Reload changes (`643f8fea`), then remove this CI task. The earlier matrix at `fcbe7394` and subsequent documentation commit `7aa50f76` passed.

## Progress checkpoints

- `17f552e2` — added regression coverage for arbitrary, editable compare workflows.
- `a9b3dd4d` — corrected stale comparison test assertions to match current module contracts (tests previously expected code in the wrong module or obsolete variable names).
- `8245c773`, `3febcbfa` — safe concurrent Save and transactional compare selection/open; `ab09aad2`, `d0cf2a97` add regression checks.
- `294b00f7`, `253825eb` — read-only source protection + tests for Project, Editor, and local browser files.
- `27ab611d`, `643f8fea` — transactional two-source Reload and regression assertions.
- `af460bd3`, `cc267751`, `3541b20a`, `0b0c8e2d`, `fcbe7394` — fix outdated editor, file-transfer, Patch Panel, and Project Actions UI contract tests.
- [CI 37764118445](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/37764118445) — full matrix **PASS** for Go 1.19/1.23 at `fcbe7394`. Later compare Reload commits still need their own CI confirmation.


Do not re-add completed roadmap sections here. Commit each remaining fix independently and retain only unverified tasks.
