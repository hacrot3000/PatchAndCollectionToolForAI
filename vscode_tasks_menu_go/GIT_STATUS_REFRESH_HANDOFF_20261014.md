# Git status scan and index.lock reduction — 2026-10-14

## Problem and verified cause
- Git panel formerly called `setInterval(refresh,5000)` indefinitely, even when the panel was closed. Opening the panel also scanned repo statuses and the Explorer could trigger additional read-only scans.
- Git status normally attempts optional index refreshes and may write `.git/index.lock`. This races with a user's terminal Git commit/add/checkout/GC.
- Readers in `runGitInDirectory` now set `GIT_OPTIONAL_LOCKS=0` ONLY for `status` and `diff`, not for mutating operations. The same runner is used by Explorer's `/api/project/git-status`. A Go test checks an existing index.lock is neither modified nor blocking Git status. Note: optional-lock semantics apply to newer Git; very old Git may not support them.

## New client refresh behavior
- No continuous 5-second `git status` calls.
- Refresh on opening Git panel, explicit Refresh/Scan, Git UI actions, browser refocus, tab becoming visible, selected project directory changes, and completed Git commands (including common Git shell aliases) from integrated TaskDeck terminals.
- Debounce/coalesce notifications and collapse concurrent refreshes. Minimum gaps prevent repeated editor directory changes from causing constant Git scans.
- Fallback once every **60 seconds** when Git panel is open, **5 minutes** when closed; hidden documents skip these checks.
- UI receives the same Git status response; no external packages.
- External changes may not become visible immediately if no browser-visible file or shell event occurs. Browser refocus/manual Refresh catches them, and the fallback poll bounds staleness.

## Commits and testing
- Commit 1: scoped `GIT_OPTIONAL_LOCKS=0` in read-only Git runner, Go regression test.
- Commit 2: event-driven client refresh, fallback cooldown and UI contract adjustment.
- CI: verify Go tests, JS syntax and build on GitHub Actions.

### Additional dashboard reader
- Project Health also reads `/api/git/status` while its dashboard is visible. Its automatic refresh now runs every **30 seconds** (previously 5 seconds). Opening the dashboard and its explicit Refresh button remain immediate; closing the dashboard stops the timer.
- Added Go UI-contract tests for reduced intervals, event triggers, hidden-tab guard, single-flight gating, and the Project Health cadence.
