# Restricted Developer / Multi-user release gate

**Branch:** `feature/restricted-developer-security`  
**Status:** NOT production-ready. Keep `/api/restricted/status.release_ready=false`.  
**Do not merge this branch to `main` without an explicit release review.**

## Security boundary

- A Restricted Developer role has immutable grants, does not permit detailed member overrides, and must always use the protected source/command gateway. Denied legacy paths must remain **HTTP 403**.
- Schedule execution currently runs with **TaskDeck process credentials**, not the role's OS identity. For shared-server requests, `schedules.view` exposes only an allowlisted summary (ID, name, type, enabled, paused); it cannot return commands, URLs, trigger expressions, webhook secrets, run output/logs or diagnostics. All inspect/log/repository operations are privileged.
- **Both** `schedules.manage` **and** `schedules.run` are required for configuration changes and for manual execution/stop/backfill/webhook emission in Multi-user. A role with only one of them cannot use Schedule mutations, because **no per-schedule ownership ACL exists**. `project.admin` and local single-user preserve their existing behavior. Only grant the pair to highly trusted operators.
- This intentionally narrows formerly permitted Schedule Role configurations on `main`. Review current Role assignments after upgrade and do not automatically add the missing second grant. A custom Role with only `schedules.view` receives a redacted list rather than a complete editable definition.

## Historical database migration verification

Two incompatible historical states used `identity_role_seeds.version=5`:
1. `main` V5: Schedule keys already registered, possibly revoked by an admin; no Restricted Developer role.
2. Restricted branch V5: Restricted Developer role already registered, possibly with old member overrides; Schedule keys absent.

V6 only seeds Schedule grants on **previously unregistered keys**. This prevents accidentally restoring permissions deliberately removed by an administrator of a `main` V5 DB. V7 ensures the immutable Restricted Role exists and deletes obsolete member overrides. The new real-SQLite test builds both historical states, upgrades them through V6/V7, verifies revoked grants remain revoked, and verifies re-running the seeder cannot widen them.

**Important:** Tests that synthesize both legacy states are not a substitute for an offline upgrade on a copy of each real historical V5 database. Stop TaskDeck, safely back up `identity.db` **with its SQLite WAL/checkpoint**, run the new binary on copies, inspect `identity_role_seeds`, `role_permissions`, `project_members` and `member_permissions`, and verify no withdrawn grant or stale override reappears. Never experiment on live production data.

## Required automated verification

The existing GitHub Actions workflow `VSCode Tasks Menu Go` is **manual-only** (`workflow_dispatch`). It includes the new migration, Schedule HTTP/RBAC, legacy restricted-route, and Role UI checks. Trigger it manually for the **exact candidate HEAD**, and save a successful run URL. A success from a previous commit does not count.

From a checkout of `feature/restricted-developer-security`:

```sh
cd vscode_tasks_menu_go
go test ./internal/identity ./internal/server -count=1
go test ./internal/restrictedpolicy ./internal/restrictedcommand ./internal/restrictedview ./internal/restrictededit -count=1
go test ./...
go vet ./...
go build ./...
node --test test_restricted_ui.cjs
node --test tests/settings_role_visibility.test.mjs
```

Capture the exact `git rev-parse HEAD`, timestamps, Go/Node versions and any failures in the release evidence. An unsuccessful or unexecuted test is **not** a PASS.

## Mandatory interactive Multi-user Firefox smoke

Using a real Firefox and a running HTTPS-enabled Shared Server, provision at least **three separate accounts**: Admin, a Restricted Developer, and an ordinary Role with `settings.read` but no `settings.write`. Keep each account in a separate browser profile/session so cookies do not mix.

| Scenario | Required outcome |
| --- | --- |
| Admin opens Settings, Role management | Allowed |
| Read-only settings user attempts update repository change, including direct HTTP request | HTTP 403; no configuration change |
| Restricted member calls Terminal, SSH, SFTP, legacy Git, Schedule endpoints | HTTP 403 |
| Restricted user tries to edit self permissions | HTTP 403 |
| Admin tries to edit Restricted Role permissions | HTTP 409 |
| Restricted attempts out-of-range file read or unviewed edit | Denied; file unchanged |
| Two distinct restricted users charge different line quotas for the same file | Counters independent |
| Restricted quota after daemon restart and new browser session | Existing usage retained |
| Two restricted users edit the same file from stale SHA | One safe success; one conflict; no lost update |
| Developer's logged-in cookie used after Admin changes Role to Restricted | New API requests immediately lose old permissions |
| Admin switches Restricted user back to Developer | No previously removed member ALLOW override reappears |
| A Role has only `schedules.view` | Safe summary; no task command, trigger URL, output or logs |
| A Role has only `schedules.manage` or only `schedules.run` | HTTP 403 for execution and mutations |
| Role explicitly has both elevated Schedule grants | Actions authorized; must still comply with input validation |

Verify responses in Firefox Network and direct authenticated HTTP requests, not just whether controls are hidden. Check user session isolation, response headers, cross-user quota persistence, and application audit logs.

## OS isolation is a deployment requirement, not a Role feature

**Do not mistake per-user API restrictions for a process sandbox.** A single shared TaskDeck process cannot safely execute privileged tasks against source writable by restricted users merely by changing their Role. Code modified by a restricted user could later run inside a privileged Schedule, build job, installer or deployment workflow.

For production-like evaluation:

1. Use a **dedicated OS account**, isolated daemon, separate workspace/checkout and restricted config/data directory. An example Linux service template is in `deploy/taskdeck-restricted.service.example`; audit every permission, TLS setting and systemd directive before using it.
2. Never mount admin SSH keys, Git credentials, CI/CD tokens, production secrets, privileged Docker sockets, deploy scripts or read/write access to privileged sibling workspaces.
3. Do not let trusted jobs, Scheduled commands, hooks or CI read and execute an unreviewed restricted-writable checkout. Use reviewed commits, separate protected CI checkout/runner and explicit human approval before privileged deployment.
4. Apply operating-system filesystem permissions, minimal writable paths, process/network restrictions and outbound firewall policy; verify with an unprivileged test account. A single mixed-trust shared daemon does **not** provide a per-user OS boundary.
5. Maintain `release_ready=false` until OS isolation and the live multi-account tests are demonstrated. Passing unit tests alone is not sufficient.

## Merge decision

Block merge if any migration test, Go/Node regression, live Firefox scenario, filesystem/OS isolation check or release-candidate CI run is missing. Do not merge to `main` on the basis of this document alone.


## 2026-10-18 follow-up audit — delegated administration and File Transfer

The previous TaskDeck self-update at `42b34887` was reported by the user to have passed its Go tests. **This pass predates the changes in this section.** The new security changes must pass the full test suite again before activation or merge.

### Fixed: delegated privilege escalation in users.manage / roles.manage

The original backend allowed a user possessing `users.manage` to create a second user with `system:admin`, assign a higher-privilege role, or grant an ALLOW override for `project.admin`. It also allowed `roles.manage` to give a new custom role any registered permission. None of these actions required the acting user to possess the delegated privilege.

Now, unless the actor has `project.admin`:
- A new/assigned Role must have permissions no stronger than the actor's current effective permissions.
- A manager may not change or override an existing target whose current **role grants or effective grants** exceed the manager's. Checking the baseline role also protects *disabled* administrator accounts, whose effective permissions otherwise resolve to an empty set.
- An ALLOW member override must be a permission held by the actor. Admin changes and restricted-developer override prohibitions are otherwise unchanged.
- A Role update may not grant more than the actor holds and may not alter a preexisting stronger Role even to revoke its privileges.
- HTTP 403 is returned when the delegation ceiling is exceeded; denial is auditable.

Regression coverage: `internal/server/shared_admin_delegation_test.go` includes real project membership, login and administration API requests. This must PASS before release.

### Fixed: background transfer host access and shared queue controls

The saved-profile file-transfer background jobs previously checked transfer capabilities but omitted the associated local filesystem permission for host uploads/downloads. Creation of `host_upload` and `host_archive_upload` now requires `files.read`; `host_download` now requires `files.write`, in addition to transfer-specific grants.

`/api/file-transfer/jobs/control` affects a **shared global queue** for a saved profile and can pause, remove, retry or resume operations initiated by other accounts, including remote deletion and host file writes. Until job-level ownership/ACL is implemented, queue-wide control requires `project.admin` in shared mode. Local single-user usage is unchanged. Users with only `transfer.read` may still view the queue; they cannot mutate it. This deliberately narrows prior UI behavior for delegated roles.

Regression coverage: `internal/server/shared_transfer_job_security_test.go`, `internal/server/shared_authorization_test.go`.

### Mandatory acceptance after these commits

1. Run `go test ./... -count=1`, `go vet ./...`, and the Node tests listed above on the **exact new candidate HEAD**. A previous PASS does not apply to new commits.
2. Use two Firefox sessions to verify a delegated `users.manage` / `roles.manage` account cannot mint admins or grant `project.admin`, but can still create ordinary users and assign permissions it holds. Verify a disabled admin is protected.
3. With different users, confirm that `transfer.read` cannot control another user's saved-profile queue and that remote-to-host background downloads do not write to workspace without `files.write`. Verify `project.admin` can operate the shared queue.
4. Confirm all admin and restricted-role UI reflects HTTP 403 from the backend; consider disabling unauthorized buttons. Explicitly review whether queue ownership should be implemented before permitting delegated queue control.
5. Keep `release_ready=false` and do not merge into `main` until the OS and Firefox isolation requirements in this document have been met.


## 2026-10-18 continuation — DB sessions and transfer queue owner isolation

The previous Go-test PASS reported by the user applied to `42b34887`; **it does not validate the later commits**.

### Implemented: shared database-session owner binding

The DB adapter session manager was global. A user holding `db.read` could previously enumerate session IDs and operate another user's open database connection through `/api/db/sessions/{id}`. Every new shared database session is now bound to an immutable server-only `OwnerProjectID`/`OwnerUserID` after creation. API list, details, query, import and close require matching ownership unless the actor has `project.admin`. Ownerless metadata is fail-closed in shared mode; single-user behavior stays unchanged. Owner fields use `json:"-"` and cannot be injected through browser payloads. Opening or testing SSH-tunneled database profiles now also requires `ssh.use`.

Tests: `internal/dbsession/manager_owner_test.go`; `internal/server/db_session_ownership_test.go`, including a real adapter subprocess HTTP fixture with two user contexts.

### Implemented: owner-filtered file-transfer queue snapshots

In Multi-user, new background transfer jobs bind owner IDs from the authenticated server principal (not from browser input). GET queue snapshots filter job paths/errors and associated items to that owner. Admin still sees all jobs. Owner data is stored as private disk-serialization fields, separate from the public Job JSON, and restored after daemon restart. Old queue jobs without owner are intentionally admin-only. Global queue controls still require `project.admin` pending owner-safe mutation primitives.

Tests: `internal/server/file_transfer_owner_security_test.go`; enhanced manual workflow security preflight. Non-admin users can still enqueue permitted jobs but cannot cancel/retry shared daemon queue items. Document and address this UX limitation with owner-scoped controls before claiming full feature parity.

### Queue-state disclosure fix implemented; migration still needs real validation

The original `projectfiles.Resolve` path stored the transfer queue in `.vscode/vscode_tasks_menu.filetransfer.jobs.json`, which was reachable by normal project-file APIs under `files.read`. This bypassed owner-filtered queue JSON. The implementation now uses **two distinct paths**:

- **Single-user**: the original project-local location, preserving compatibility.
- **Shared Multi-user**: a 0700 directory under the external identity DB parent, keyed by canonical workspace and project ID. Queue files remain 0600, outside the served workspace. `Server.Serve` prepares/migrates this path before accepting shared HTTP traffic.
- Legacy state is copied to a temporary private file, synced, atomically renamed and removed from `.vscode` only after success. Unsafe source symlinks, oversized files, internal workspace paths, and divergent legacy/private states **fail closed** rather than silently dropping jobs. Identical leftover copies can be cleaned safely.

Regression tests: `TestSharedTransferQueueStateMigratesOutsideServedWorkspace`, `TestSharedTransferQueueRejectsWorkspaceResidentIdentityPath`, `TestSharedTransferQueueRejectsLegacySymlink`, and `TestSharedQueueMigrationRefusesConflictingLegacyState`.

**Important:** These are source changes and new tests, **not yet a passing candidate update or completed migration on a real deployment**. Before activation, back up queue state and verify jobs survive a shared-daemon restart. An incompatible legacy/private state deliberately requires administrator reconciliation. Do not remove the existing daemon until candidate validation and rollback preparation have succeeded.

### Other internal workspace state remains a release blocker

Other `.vscode/vscode_tasks_menu.*` artifacts (including the server INI and project metadata) may still be readable through `/api/project/file` or discoverable via search/archive/download flows by users with `files.read`. The queue migration does **not** constitute a blanket protection of TaskDeck managed project state. Audit all managed-state data and all project file APIs before production, and move secrets out of the served workspace or implement comprehensive source-side path restrictions. This issue, OS isolation and live Firefox tests continue to block release and merge into `main`.

### Validation

- Source inspection and static consistency checks only for this new revision; no live browser, OS sandbox or exact-HEAD Go/Node run in this chat.
- The manual-only GitHub Actions workflow now includes targeted DB and transfer ownership tests; it does not auto-run on push.
- The user's self-update candidate validator can run `go test ./...` against the new SHA. A prior revision PASS is not evidence for the newly changed code.


## 2026-10-18 — Block managed TaskDeck workspace state across source APIs

After relocating shared-mode transfer queue persistence, a broader served-workspace exposure remained: internal `.vscode/vscode_tasks_menu.*` files (including INI/password settings and historical queue copies) could be accessed with ordinary `files.read` or exported via archive/search operations.

The branch now implements `isTaskDeckManagedProjectPath` and canonical symlink-target checking for **shared mode**. It denies raw access to managed-state basenames (including legacy root copies) through `resolveProjectPath`, and hides them from Explorer tree, indexed/exact filename search, ripgrep/fallback content search, bulk replace previews, symbol search, code-completion results, ZIP/tar creation and ZIP download of containing folders. Host-side File Transfer and host mutation routes reject these files, including recursively selecting the shared `.vscode` folder. Existing single-user behavior is preserved.

Regression tests `shared_managed_state_guard_test.go` include symlink aliases, direct editor and chunk reads, tree search, content search, archive download of the containing directory, host transfer paths, and bulk replace preview. The manual-only workflow has a targeted preflight for these tests. A static source review checked thirteen corresponding source contracts against the latest GitHub branch.

**Validation is pending**: a full `go test ./...` run and live Firefox multi-account tests were not possible in this chat. The last user-confirmed PASS is an earlier revision, not these new commits. Require all checks before activating or merging.

Remaining security scope: standalone archives created *before* this patch may still contain old managed files and should be audited; Git history and commands, terminal/task execution, user-created files with embedded secrets, and deployment-level OS file permission isolation are not made safe simply by this path filter. Do not treat `files.read` as authorization to see arbitrary application secrets. Keep `release_ready=false`.


## 2026-10-18 — Safe upstream macOS sync and project-scoped administrator visibility

The Restricted Developer branch is now synced with upstream main as of `9357cb6eb34a4bf97baa6402d422aa6c17e1ed7f`, via feature-branch-only merge `c0dca96677cf6360d984d68adce21b62933b7a10`. All 26 non-overlapping upstream files were retained byte-for-byte, including executable macOS deployment scripts. The sole overlapping workflow file was combined to preserve the manual-only Linux security checks and the native macOS (Go 1.25) matrix. No main branch write was performed.

Also hardened project-scoped administrator visibility when resource metadata is shared inside a daemon: a `project.admin` in project B cannot view a DB connection or saved-profile transfer job whose owner is explicitly project A. Ownerless legacy transfers remain admin-only for recovery, never available to ordinary users. Regression assertions were added to existing DB-session and transfer queue isolation tests.

**Release acceptance remains pending:** run `go test ./... -count=1`, `go vet ./...`, both macOS CI runner jobs, and real Firefox session/multi-user permission-revocation checks on the precise updated feature HEAD. Keep the feature isolated from main and `release_ready=false` until these checks succeed. Historical archives, Git/terminal execution and host-OS isolation still require manual threat-model validation.


## 2026-10-18 — Additional Schedule remote-Git sync and review point

After the macOS sync, main added 11 Schedule changes including `remote_change` trigger, `git_remote`, `git_auto_pull`, runtime support and dedicated tests. These were integrated into feature commit `ce5afd43fc225e7a1bec442eb655cb301f07ba31`. The only Schedule conflicts were the two browser modules `schedulespanel.js` and `scheduleviews.js`: both were merged manually to retain RBAC-based redaction and new remote Git UI.

**Additional security review:** Auto-pull is limited to Git fast-forward without reset in implementation, but it still writes to the checkout with the TaskDeck daemon's privileges and may use the daemon's SSH/Git credentials. Confirm intended rights for schedules users, revocation while pending, remote allowlisting and OS-level isolation in the final multi-user threat model before release. Run the new `TestScheduleRemoteGitObservationAndSafePull` and `TestScheduleRemoteGitValidation` along with `go test ./... -count=1`.

Do not equate static reconciliation with green CI, and do not merge into main or set `release_ready=true` until all release-gate evidence is satisfied.


## 2026-10-18 — Historical source and Git conflict confidentiality follow-up

The managed-state source guard originally protected live project-file APIs but missed **historical versions** stored outside the live tree. Under `files.read`, `/api/project/file-history` could enumerate and return previous copies of `.vscode/vscode_tasks_menu.ini` even after that file was removed from the workspace. In shared mode the handler now returns 404 for all TaskDeck-managed path components, for both index and content requests. It also checks any currently resolvable symlink alias against the canonical protected target. Ordinary source history and local single-user access are unchanged.

Git has an analogous historical-data boundary: a full commit SHA, staged index, or conflict stage could retrieve a managed file without going through `resolveProjectPath`. In shared mode, `gitFileContent` (historical and index versions), `gitFileHistory`, `gitDiff`, `gitFileBlame`, and Git-conflict stage read/write now reject the same protected paths. A user with `git.diff` or `git.log` can no longer directly read those private file contents by naming the known managed path via these endpoints.

Regression tests: `TestSharedManagedStateHistoryRejectsOldPrivateVersions`, `TestSharedManagedGitHistoryAndBlobReadsFailClosed`, and `TestSharedGitConflictStagesCannotAccessManagedState`, added to the workflow's **manual-only** security preflight. A re-read source check confirmed all handler guards exist and new tests have no duplicate imports; this is not equivalent to compiling or executing them.

**Open risk:** path-based filters cannot prove confidentiality for a secret copied to a benign filename, historical Git rename aliases, old exported archives, arbitrary Git commit messages, daemon-side terminal/schedule execution, or commands run with TaskDeck's OS identity. Git tree/status metadata and broader host-file access still require live threat-model tests. Keep `release_ready=false` and do not merge to main before full Go/Node/macOS and Firefox multi-account validation.


## 2026-10-18 — Project-admin transfer queue mutation and Git view privilege fixes

### File Transfer cross-project mutation

The earlier shared queue GET/control response filtering redacted jobs from other projects but did **not** prevent queue-wide mutation. A `project.admin` in project A could still pause, clear, retry or resolve conflicts on a queue containing project B jobs, because both projects can reference the same saved-profile queue in one daemon.

`fileTransferQueueProjectAuthorizedLocked` now checks every job and live item **while holding `queue.mu`** for the control path. It denies a global queue operation if any job belongs to another project, has no owner (legacy restored job) or any live item has an unknown job. The `resolve_conflict` path performs the same check under the lock inside `resolveServerConflicts`. Same-project-only queues are still manageable by the project administrator; local single-user mode is unchanged. New test `TestSharedTransferQueueControlRejectsForeignAndOwnerlessJobs` covers the denial and allowed cases.

This is a conservative fail-closed policy, **not full queue tenant isolation**. When two projects genuinely share a profile, their administrators cannot use global queue controls until separate per-project queues or owner-scoped actions are implemented. Races around asynchronous global resume/recovered-scan behavior and daemon OS authority still require an integration audit. Do not declare cross-project transfer execution isolation complete from this test alone.

### Git read-only role bypass

`/api/git/status` dispatched previously-unclassified views using the fallback `git.status` capability. In particular `blame` can return source lines; `file-history`, `stashes` and tree comparison modes provide revision/history information not intended for a status-only Role.

`sharedRoutePermissions` now maps `file-history` and `stashes` to `git.log`, and `blame`, `compare-tree`, `compare-tree-diff` to `git.diff`. Existing ordinary status behavior is preserved. Added route-policy assertions and `TestSharedGitHistoricalViewsRequireLogOrDiffPermission`, exercising authorization middleware with a status-only and an appropriately granted user.

Targeted tests for both fixes are included in the manual-only Linux GitHub Actions workflow. **Full Go tests, Go vet, Node tests, macOS runners and multi-user Firefox/OS security smoke are still unexecuted on the new HEAD in this chat.** Keep `release_ready=false`; never merge into main solely from static review.


## 2026-10-18 — Shared Git historical tree metadata hardening

The previous managed-source guards blocked direct historical Git blobs, blame, diff and conflict-stage reads, but Git **metadata** still disclosed pathnames and object IDs of managed files via `gitCompareTree`, `gitCompareTreeDiff`, `gitCommitFiles`, and `gitGraphCompareHead`. Shared-server responses now filter managed filename components out of those lists, including either side of a rename. In tree listings, normal ancestor directories remain browsable but their tree OID is suppressed when the tree contains protected descendants, preventing indirect fingerprinting through an aggregate Git tree hash. Single-user responses retain their original semantics.

Implementation: `internal/server/shared_managed_git_metadata.go`, plus integration with `git_tree_compare.go` and `gitgraph.go`. Regression tests: `TestSharedGitTreeMetadataOmitsManagedFilesAndOIDs`, `TestSharedGitCommitMetadataExcludesRenamesOfManagedFiles`, including parent-tree OID suppression. Targeted tests were added to the manual-only GitHub Actions workflow.

Separate concurrent development added cross-project transfer queue mutation enforcement on this branch; avoid replacing those active commits and verify them together with the new Git filters on the **same exact final SHA**.

**Open concerns:** Git log/reflog/commit messages and other non-file-path history endpoints can still carry user-supplied sensitive strings. A benign historical filename or renamed/copy of a secret is not automatically classified as managed configuration. Global Git commit IDs, shell/terminal and schedules with the daemon OS identity remain out of scope of filename-only redaction. Tests and real multi-user Firefox / OS isolation are **not yet completed**. Keep `release_ready=false`; do not merge into main.


## 2026-10-18 — Git status and shared conflict-state metadata guard

After securing Git tree, diff, commit and historical content APIs, the `/api/git/status?view=changes` endpoint still returned raw `git status --porcelain=v1 -z` entries. In shared Multi-user this exposed internal `vscode_tasks_menu.*` basenames and historical rename sources to principals holding `git.status`. The endpoint now applies `filterManagedGitStatusChanges` to both `Path` and `OriginalPath`; local Single-user behavior is unchanged.

The `gitConflictState` source also serialized unmerged `Path`/`ProjectPath` metadata to multiple callers, including the `changes` payload. Shared-mode output now applies `filterManagedGitConflictFiles` centrally, so callers see only non-managed files. This is **metadata filtering**, not a claim of Git execution isolation.

The manual Actions security preflight now includes `TestSharedGitStatusMetadataRedactsRenamedManagedPaths` and `TestSharedGitConflictMetadataExcludesProtectedPaths`. Both helpers and HTTP integration sites were re-read from GitHub (8/8 source checks matched), but no `go test` or Firefox smoke was executed on this revision.

**Unresolved major release blocker:** Users holding both `schedules.manage` and `schedules.run` can configure raw shell commands or task executions that run as the TaskDeck daemon's OS account. Role checks do not provide process isolation. SSH/Git credentials inherited by schedule subprocesses and private filesystem locations must be threat-modeled; require a sandbox/OS identity boundary or project-admin-only execution policy before enabling delegated Schedule execution in production. Keep `release_ready=false`, and do not merge into `main` on static checks alone.


## 2026-10-18 — Main sync verified; ownerless legacy resource quarantine

Rechecked `main` HEAD `0286e36378dcc62c7b72df1e2c538a54230bd912` against `feature/restricted-developer-security`: `behind_by=0`, so there were **no newer main commits to merge** and no redundant merge commit was created.

Continued RBAC audit found a concrete cross-project confidentiality flaw for legacy resources with missing ownership metadata. A `project.admin` in project A could previously see *ownerless* DB sessions and FTP/SFTP job path/error metadata that may have originated under project B. This can happen when restoring queues persisted before per-job ownership existed. The shared-server policy now:
- Shows DB sessions to project admins only if **both** `OwnerProjectID` and `OwnerUserID` are nonempty and the project matches.
- Shows transfer jobs and their items to project admins only if **both** owner fields are nonempty and the project matches.
- Keeps ownerless or partially owned metadata in memory/private queue persistence for operator recovery, but does not expose it to any project-scoped browser principal.
- Retains original Single-user behavior.

Updated regressions `TestSharedDatabaseSessionsVisibleOnlyToOwnerOrProjectAdmin` and `TestSharedTransferQueueSnapshotsHideOtherUsersAndLegacyJobs` to include ownerless and partially owned resources. The private file-transfer queue restore path pauses recovered pending queues, and the existing project-owned queue mutation checks deny cross-project and ownerless job operations. Operators must use an offline/private recovery procedure for legacy queues rather than trusting ownership claims from browser requests.

**Validation** remains pending: these changes were source-inspected, but no complete Go/Node suite or live Firefox multi-user smoke was run at the final candidate SHA. Current `release_ready=false`; do not merge to main on static review alone.

**Outstanding security design blocker:** delegated Schedule role holders with both `schedules.manage` and `schedules.run` can execute tasks and commands with the daemon's OS account. This is not a sandbox and should receive an explicit policy decision (admin-only execution versus true isolated worker accounts/containers). Terminal permissions and Git subprocess credentials likewise require live OS-level isolation validation.


## 2026-10-19 — Multi-project confidentiality and fail-closed authorization

The latest review expressly **defers Schedule/Terminal process-identity isolation by user request**. This iteration focused solely on other risks.

The branch was verified as containing all commits from main (no merge needed). The audit found that the prior project-admin fallback would display DB sessions and FTP/SFTP queue records with missing or incomplete ownership, although such historical records cannot be attributed to any Project. Shared mode now requires **both** `OwnerProjectID` and `OwnerUserID` to be nonempty and the Project ID to match, even for `project.admin`. Historical data is retained privately for out-of-band recovery, not deleted or reassigned based on browser input.

An independent authorization weakness existed in `requireSharedActionPermission`: if the handler was invoked without a principal, it assumed local single-user mode and returned true even when `SharedServerEnabled` was true. The helper now returns HTTP 401 and denies in shared mode, while retaining permissive operation in genuine local single-user mode.

Updated tests cover anonymous shared action denial, valid/invalid permissions, local compatibility, missing or partial DB/transfer owner fields, and serialized queue JSON confidentiality for project administrators. `TestSharedActionGateNeverTreatsMissingPrincipalAsLocalMode` was added to the existing manually dispatched security preflight.

**Validation distinction:** Source review and test additions are completed and pushed; no `go test ./... -count=1`, `go vet ./...`, macOS CI, or Firefox multi-user session run was executed against the latest HEAD here. The release gate remains `release_ready=false` pending those results and any other non-deferred authorization issues.


## 2026-10-19 — Task-run historical log integrity and live role revocation

This pass continues the user-approved audit of **non-Schedule/Terminal-OS-permission issues only**.

The Task Run History API had a separate principal-missing fallback: `taskRunVisible` returned true without a project principal, even in shared mode, and `recordTaskRun` only checked session visibility after looking up a guessed session ID. Shared mode now fails closed before metadata lookup and on history/log reads; Single-user retains its preexisting behavior.

Another gap was the distinction between **viewing a same-project task run** and **mutating its durable log**: a delegated user with `tasks.view` and `tasks.run` could previously record/overwrite another user's task-run history. New `sharedTaskRunRecordAllowed` permits writing only to the originating user (with both view/run grants) or an administrator of that same project. Read-only same-project history visibility remains unchanged. Missing task owner IDs do not authorize delegated writers. This is a log-integrity check, not a change to Schedule process privileges.

`AuthenticateBrowserSession` already reloads project grants for each HTTP request. Added `TestBrowserSessionReflectsRolePermissionRevocationImmediately` to verify permission revocation and regrant while a login cookie remains active. New task-run tests check anonymous project access, cross-project isolation and owner-only writes. All targeted tests were added to the existing *manual-only* security CI workflow. Re-reading the updated files confirmed eight source-level guard and test wiring conditions.

**Evidence caveat:** exact-HEAD Go/Node tests, macOS builds, and two-account Firefox sessions have not been executed here. HTTP request-level dynamic grants are tested in source, but long-lived WebSocket sessions and existing subprocess capabilities require separate operational review. Continue to hold `release_ready=false` until build/test evidence exists; no merge to main.
