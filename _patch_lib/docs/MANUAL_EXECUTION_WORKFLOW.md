# Manual Execution Workflow — v6.20.0

`manual_execution` lets a PATCH describe commands that the **human operator** must run. Python Patch Tool never executes these commands.

## Manifest

```json
{
  "manual_execution": {
    "stop_on_failure": true,
    "package_result": true,
    "steps": [
      {
        "id": "build-server",
        "title": "Build server",
        "description": "Build jdqs_server to verify compilation",
        "cwd": "projects/m3-server/trunk/jdqs_server",
        "argv": ["mvn", "-DskipTests", "package"],
        "expected_exit_codes": [0]
      }
    ]
  }
}
```

Steps use structured argv. Raw `command` fields and inline evaluator escape forms such as `bash -c`, `sh -c`, `python -c`, `node -e`, and PowerShell `-Command`/`-EncodedCommand` are rejected.

## Operator flow

For each step the tool prints the working directory, command, evidence log path, and a copyable capture wrapper to run in **a new terminal**. The wrapper uses `tee` and appends a marker like `[PTV_MANUAL_EXIT_CODE=0]` to the evidence log.

At the prompt:

- `Enter` verifies the current log and advances only when valid evidence exists.
- `r` prints the instruction again.
- `m` uses a log copied manually to the indicated file, then asks the operator for the command exit code and records it.
- `q` aborts the manual workflow.

No subsequent step is accepted before evidence for the current step has been recorded. `stop_on_failure=true` stops on an unexpected exit code.

## Result artifacts

With `package_result=true`, the tool creates:

- `MANUAL_EXECUTION_RESULT_<patch>_<time>.zip`
- `MANUAL_EXECUTION_RESULT_<patch>_<time>.txt`

The ZIP contains `MANUAL_EXECUTION.json`, `MANUAL_EXECUTION_REPORT.md`, and per-step instruction/console evidence under `steps/`. HISTORY recognizes both result artifacts. When the PATCH fails, FAIL_HANDOFF includes available manual evidence under `manual_execution/`.

## Manual-only package

Set top-level `payload` to `manual_only` to create a package with no source-changing payload. The workflow still uses the same evidence and result packaging rules.

Because manual execution requires a human, non-TTY/CI invocation fails before payload mutation rather than silently skipping or running a command itself.
