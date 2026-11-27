# Owner-registered Nim gateway integration

## Status
Implemented source for local Agent Chat, using existing native PATCH approval and COLLECT workers. No direct Docker or shell tool, no general external filesystem access. Actual Docker/Nim execution and full CI remain unverified.

## Recommended setup: PATCH TOOL → Nim Gateway

1. Open **PATCH TOOL → Nim Gateway** in the local project. Existing valid registration is recognized automatically; no reinstall or re-registration is necessary.
2. If unregistered, select a detected script (root `nim-agent.sh`, `nim-gateway-v4/nim-agent.sh`, `tools/nim-agent.sh`) or enter its project-relative path and click **Find script**. If no gateway exists, choose **Install from file…** and select the reviewed `nim-agent.sh` supplied by the owner. Installation never overwrites a file and does not run it.
3. Click **Register / Update**, verify the displayed script and digest, and approve. TaskDeck initializes/reads the correct Project ID and writes the private registration automatically. No manual Python, chmod, JSON or sudo needed. Existing single-account registration can be replaced only by an explicit owner action.
4. Click **Copy AI Prompt** and paste into the existing Agent Chat. The prompt contains actual project identity, approved path, SHA-256, native COLLECT doctor request and command-only PATCH example. Refresh checks registration and digest without executing Docker.
5. The AI should perform `nim-doctor` using native COLLECT and verify the matching bind mount before executing builds. Registration status is not evidence of a successful container test.

**Revoke** removes this project's registration only, not its script or previously queued native packages. A changed script requires explicit review and re-registration, never automatic authorization of changed bytes. The setup endpoint is local-only, behind existing browser lease and same-origin controls.

## Alternative: one-time terminal registration (not an AI PATCH)

Use the reviewed `nim-agent.sh` v4 selected by the operator. It must reside within the actual bound host project, at its existing relative path, and must already exist. Its configured host/container root must agree with the TaskDeck project. For the supplied v4 that root is `$HOME/568E/Naruto/server-patching`. Do not register a copy in an unrelated worktree; the gateway operates on its configured project, not TaskDeck cwd.

From that project root run the following as the development account (no sudo). Replace the script argument if it lives in a subdirectory.

```sh
python3 - nim-agent.sh <<'PY'
import hashlib, json, os, pathlib, sys, tempfile
root = pathlib.Path.cwd().resolve()
rel = pathlib.PurePosixPath(sys.argv[1])
assert not rel.is_absolute() and '..' not in rel.parts and rel.name == 'nim-agent.sh'
script = root / rel
assert script.is_file() and script.resolve() == script
identity = json.loads((root / '.python_patch_tool.json').read_text())['project']
cfg = pathlib.Path.home() / '.config/taskdeck/nim-gateway.json'
cfg.parent.mkdir(parents=True, exist_ok=True)
registration = dict(project_root=str(root), project_id=identity['id'],
                    script=rel.as_posix(), sha256=hashlib.sha256(script.read_bytes()).hexdigest())
fd, name = tempfile.mkstemp(dir=cfg.parent, prefix='.nim-gateway-')
with os.fdopen(fd, 'w') as f:
    json.dump(registration, f, indent=2)
    f.write('\n')
    f.flush()
    os.fsync(f.fileno())
os.replace(name, cfg)
print('Registered:', cfg)
PY
```

One registration per development account. A later script change invalidates authorization; review it before re-registering. Remove the config to revoke future admission. A native package already queued is not retroactively cancelled. Owner registration is outside project and must not be created or updated by the AI. The same OS account remains a trusted development account; this integration is not an OS sandbox.

## Workflow

1. New Agent Chat requests detect an exact project root, project UUID, existing script path and SHA. They receive explicit registered-gateway instructions.
2. Native COLLECT command action for diagnostics: `{"type":"command","transport":"local","argv":["nim-gateway","nim-doctor"]}`; `nim-profiles`, `nim-profile-info PROFILE`, `nim-source-compare`.
3. Build/test via ordinary native command-only manifest: `argv:["bash","nim-agent.sh","nim-test","tests/actual_test.nim","server-probe","-d:mwfVerboseDiagnostics"]`; root cwd, one command, legacy_strict, run_when_no_changes, valid reason and `preflight.files:[{"path":"nim-agent.sh","exists":true,"sha256":"REGISTERED_SHA"}]`. Substitute actual existing file and gateway path. Existing project identity, approval, lineage, SHA and native queue rules apply. `nim-build` permits compile-only profiles (not release-copy); `nim-check`, `nim-compile`, `nim-test`, `nim-test-suite` permit existing .nim files, known file profile and -d:NAME flags. No source writes/deletes, release-copy, cache-clean, arbitrary Docker, sudo, or shell forwarding.
4. Gateway emits its own 32 lowercase hex RUN_ID; do not confuse this with TaskDeck native run_id. Obtain it from RUN_ID= or the RECEIPT= line in the native output. Do not guess or use latest if missing.
5. Native COLLECT command action `{"type":"command","transport":"local","argv":["nim-gateway","nim-build-result","ACTUAL_32_HEX_RUN_ID"],"max_output_bytes":65536}`. Same explicit ID for nim-build-status, nim-build-log, nim-artifacts. The collector resolves the approved gateway, not an arbitrary executable or path; it runs the gateway read action and captures stdout/stderr with existing bounded report and ZIP/TXT. Log output over 64 KiB is truncated, not complete evidence. Native report may also be excerpted in provider context. Never pretend truncated evidence is complete.
6. Verify requested commands and flags, matching RUN_ID, SUCCESS, exit_code=0, source_unchanged=true, and actual test results. Gateway hash only covers its documented Nim/C/C++/config scope. Compile success is not test success. Stale RUNNING, interrupted Docker client, missing receipt or source changed are not PASS; inspect before retry.

## Boundaries

Local Agent Chat with an exact project binding only; Shared Server/SSH and isolated worktrees do not get this gateway permission. Normal COLLECT remains project-scoped. There is no generic external path read. Existing script and PATCH Python capabilities are not a sandbox against indirect execution. Gateway and tests can have side effects with the existing development user's container permissions; trust and review them.

## Evidence

- Go: TestAgentNimGatewayBuildRegistration (registration, manifest pin, exact project, SHA drift, approved/denied argv).
- Python: extended self_test_collect_command_v6_20_0.py (stub gateway, read-only action, exact ID, deny shared-worker flag, SSH, mutation, traversal, changed digest).
- No live Nim container access in this environment. No claim that I0 clone/generation has run or passed.
