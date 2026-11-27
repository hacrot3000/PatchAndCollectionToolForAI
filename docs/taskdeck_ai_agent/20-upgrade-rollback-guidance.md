# HANDOFF 20 — Upgrade, rollback and diagnostic guidance

> **Current local workflow (2026-11-27):** [20-coder-execution-contract.md](20-coder-execution-contract.md) supersedes historical process/stop instructions and completion claims below. Upgrade and rollback execution belong to the operator. Local coder authors missing fixtures/procedures and continues other source work. Do not use external verification or delivery as a local BLOCKED condition. No acceptance or safety relaxation.

> Operator-run guidance for CLP-09.02.04 (regression gates R11, R24).
> Commands here are DOCUMENTATION ONLY. They were not executed by the TaskDeck AI session.
> Runtime verification, binary self-update, SIGKILL crash injection and browser smoke are NOT available in that session; all such steps below are PENDING for the operator.

## Scope and authoritative sources
- Master checklist: `docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md` (CLP-09.02.01-.03, section 6.1 gates R11/R24).
- Operator worklist: `docs/taskdeck_ai_agent/20-external-verification-queue.md` (V-BATCH-06 — Upgrade smoke and rollback).
- CI workflow (exact commands): `.github/workflows/taskdeck_selfupdate_multi_instance.yml` (working-directory `vscode_tasks_menu_go`, `GOPROXY=off`, `GOSUMDB=off`).
- Runtime procedure: `vscode_tasks_menu_go/SELF_UPDATE.md` (dry-run, atomic activation, listener handoff, broker preservation, rollback).
- Go module: `bletonfc/vscode_tasks_menu`; Go working directory: `vscode_tasks_menu_go`.

## 1. Safety invariants (never violate)
1. Use a SYNTHETIC copy of legacy state only. Never point smoke commands at a production workspace, real provider credentials, or a real user journal.
2. Do not re-apply a production patch or mutate the checkout under test. Self-update itself must not run `git pull`, `git reset`, `git checkout` or `git clean` on the current checkout (SELF_UPDATE.md, "An toàn working tree"); source for a candidate exists only in a temporary directory.
3. Do NOT use `--restart-daemon` or `--stop-daemon` while tasks/terminals must be preserved; those paths can stop broker sessions. Prefer listener handoff / broker-preserving detach.
4. Never treat a build success, a PATCH receipt, a wrapper rc=0, or operator-supplied text as runtime verification of upgrade behaviour.
5. Global update lock `~/.local/lib/taskdeck/.self-update-global.lock` serializes prepare → activation → handoff → rollback across daemons of the same OS account. Do not delete a live lock or kill a broker to work around it.
6. Releases are versioned under `~/.local/lib/taskdeck/releases/`; per-workspace runtime progress is in `self-update.json`.

## 2. Preconditions and synthetic backup
Working directory: repository root.
- Confirm checkout is clean and record the build SHA: `git rev-parse HEAD`.
- Copy the synthetic workspace directory (state, config, runtime dirs) to a timestamped backup location outside the checkout. Record the copy command, exit code, and a SHA-256 manifest of the copied files.
- Record the currently serving binary path, version (`taskdeck --version`), revision, listening address and TLS fingerprint so rollback can be verified.
- If the host has no Go compiler, see SELF_UPDATE.md "Thông báo riêng khi hệ điều hành thiếu Go": pin `TASKDECK_GO` to an absolute compiler path in the DAEMON's environment (a shell `export` in another terminal does not change a running daemon), or allow the documented checksum-pinned SDK bootstrap.

## 3. Build and validate from source (no network)
Working directory: `vscode_tasks_menu_go`.
```sh
gofmt -e internal/selfupdate/toolchain_bootstrap.go >/dev/null
gofmt -e internal/selfupdate/toolchain_bootstrap_test.go >/dev/null
gofmt -e internal/selfupdate/toolchain_lock_unix.go >/dev/null
gofmt -e internal/selfupdate/toolchain_lock_other.go >/dev/null
gofmt -e internal/selfupdate/selfupdate.go >/dev/null
gofmt -e cmd/vscode_tasks_menu/main.go >/dev/null
bash -n ../install.sh
go test -count=1 ./internal/selfupdate
go test -count=1 ./cmd/vscode_tasks_menu -run 'Test(SelfUpdate|Global|Release|Daemon|Preferred)'
node --check web/featuremods/selfupdate.js
node --test test_selfupdate_failure_ui.cjs
go test -count=1 ./internal/server -run '^TestSelfUpdate'
for arch in amd64 arm64; do GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go test -c -o "$RUNNER_TEMP/selfupdate-darwin-$arch.test" ./internal/selfupdate; done
go build -buildvcs=false -trimpath -o "$RUNNER_TEMP/taskdeck" ./cmd/vscode_tasks_menu
```
Expected: all commands exit 0; candidate binary `--version` matches the recorded build SHA. Return: exact commands, exit codes, build SHA, binary `--version` output, CI/job logs if run in CI.

## 4. Synthetic self-update / reload / restart smoke
- From the synthetic workspace, run `taskdeck --workspace <synthetic-workspace> --self-update`. Monitor `self-update.json` through states: `confirmed → downloading → testing → building → installing → ready_restart → restarting → completed`.
- During DRY-RUN the current release keeps serving. If download/test/build/validation fails, candidate must be discarded, current daemon kept, and the failure shown non-blocking.
- On PASS only: snapshot current activation, atomic-switch candidate, then handoff TCP listener to the new daemon while keeping the session broker. Verify the same URL/port, same session IDs, PTY, scrollback and tab state survive; browser reloads only after the new daemon is healthy.
- Confirm runtime state `completed`; record the new release path, version/revision, and the exact log excerpt showing listener handoff (or broker-preserving detach) — not a legacy stop.
- Reload/restart variant: repeat once with a stopped daemon (no `--restart-daemon`) and confirm the new binary is used on next start.

## 5. Post-upgrade verification checks
- **Migration**: older synthetic session/journal (including a legacy AI Pack envelope and at least one acknowledged Patch) remains readable; counts and canonical history unchanged.
- **Recovery / continuation**: an interrupted turn with an exact UNKNOWN native receipt resumes only on explicit owner Continue, exactly once; no replay of egress.
- **Counters**: collection guard counters (native reads, verified new, duplicate, unknown, error, HTTP attempts, Patch proposal/execution) match pre-upgrade values plus only the new synthetic events.
- **ACK**: acknowledged Patch tool-call/output pairs stay paired and byte-exact in canonical history; no orphan fabricated.
- **Auto flags**: with auto-apply off and on, a new Patch still requires the configured approval path; opt-out survives restart.
- **Provider switch**: pinned provider/config/credential and capability fingerprint survive upgrade and a controlled switch; rotated credential is rejected.
- **Stop**: user Stop during a provider call cancels the request; a late response cannot publish a tool call or ACK.
- **Concurrency**: two synthetic workspaces/daemons update under the global lock without overlapping activation or rollback; a second updater waits and re-reads current release.
- **TLS identity**: self-signed certificate/key pair is reused across the upgrade unless genuinely invalid; network interface changes do not silently rotate the fingerprint.

## 6. Rollback procedure
- If any dry-run step fails: no activation occurred; discard staging, keep the current daemon, capture the failure state from `self-update.json` and logs.
- If failure occurs AFTER activation: confirm the updater restores the previous global activation snapshot. If the old daemon already left the listener, it must be restarted directly from the previous executable. The built candidate stays in its versioned release for diagnosis/retry.
- If a handoff leaves the listener on a different port, confirm `target_url` is recorded and the open browser tab redirects to it.
- Verify rollback with: restored `taskdeck --version`/revision equals pre-upgrade values, same listening address, same session IDs/PTY/scrollback, canonical journal SHA-256 unchanged from the section-2 backup manifest, and collection counters equal the backed-up values.
- BROKER CAUTION (SELF_UPDATE.md §macOS): if the daemon reports a missing `session-broker.json` while a broker still holds the lock, do NOT delete sockets/locks or kill the broker automatically; verify the exact `--session-broker --workspace ...` process before stopping it, and only for a session with no terminal that needs preserving.

## 7. Evidence to return to the AI/operator
For each executed step provide: working directory, exact command/procedure, exit code, timestamps, build SHA, before/after version+revision, `self-update.json` state transcript, ZIP/TXT artifact paths, backup manifest SHA-256, checklist of section-5 checks with PASS/FAIL/N-A and a short reason, and any failure diagnostics (redacted of secrets only by the operator, never by the AI).

## 8. Runtime verification status (honest)
NOT PERFORMED in the TaskDeck AI session that authored this document. That session had only read-only native COLLECT and source PATCH; no shell, no Go toolchain, no network/GitHub, no binary self-update, no SIGKILL, no browser. Consequently gates R11 (real binary self-update) and the browser/self-update portion of R24 remain VERIFY_PENDING. The synthetic Go CI fixtures that exercise migration/restart/ACK exist in the repository but were not executed here; operator-supplied CI evidence applies only to source commit `88a909b4d564b9a1f1e3f35959bc2de55d85647d` and was not re-verified. Run V-BATCH-06 from `20-external-verification-queue.md` and return real receipts before changing any gate status.
