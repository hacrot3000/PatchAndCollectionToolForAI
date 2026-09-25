# TaskDeck Patch Tool — Final Native Parity Audit

Date: 2026-09-24  
Audited baseline: `64dbe8e8` (`Add native History cleanup UI`)  
Scope: Phase 2H.7 final native/terminal parity gate  
Runtime changes in this checkpoint: **none**

## 2026-09-25 correction — INVALIDATED AS A NATIVE-UI CUTOVER GATE

The 2026-09-24 PASS below validated structured protocol/control parity, but it missed a product-level
UI requirement: built-in Patch actions still created normal TaskDeck terminal tabs. The Patch panel
called `app.attachSession(...)`, and the global `syncSessions()` loop would also auto-attach those
sessions. Therefore the audit did **not** prove that Patch ran as a native TaskDeck UI.

Treat the matrix below as historical protocol/control parity evidence only. A replacement Phase 2
gate must require that built-in Patch backing sessions stay headless by default and that a terminal
view is materialized only through an explicit evidence/fallback action.

## Historical result

**INVALIDATED — previously reported as PASS.**

TaskDeck now covers the interactive operator surfaces that previously required the Patch Tool
terminal UI. Python remains authoritative for queue/recovery/history policy. TaskDeck consumes
bounded structured protocol state and sends only narrow prompt-bound actions.

The PTY is not removed. It remains a supported evidence/fallback surface and the direct
`taskdeck patch ...` CLI remains supported.

## Historical protocol/control parity matrix

| Terminal capability | Native TaskDeck equivalent | Result |
| --- | --- | --- |
| Queue / Failed discovery | Queue + Failed native views from Python `queue_snapshot` | PASS |
| Select/cancel PATCH or COLLECT | Python-owned `queue_selection` prompt | PASS |
| COLLECT-exclusive selection rule | Advertised constraint; Python revalidates final response | PASS |
| PATCH priorities 0–9 | Capability-driven native priority inputs; Python orders execution | PASS |
| Select all PATCH / clear | Native controls | PASS |
| Search name/id/summary/target | Native filtering over Python-projected search text | PASS |
| Inspect / Preview / Validate | Native prompt-bound item actions | PASS |
| Delete queue item | Native confirmed delete, refreshed Python snapshot/prompt | PASS |
| Tool Health | Native read-only Health primary view | PASS |
| Plan policies/resources/conflicts/previews | Native read-only Plan primary view | PASS |
| Running lifecycle | Native item state | PASS |
| COLLECT progress | Native structured progress | PASS |
| User-facing artifacts | Native Download/Open actions | PASS |
| Smart Resume all/failed/remaining | Native Resume | PASS |
| Smart Resume collect_failed/delete_failed | Native Resume | PASS |
| Resume → History | Native History handoff | PASS |
| History list/default meaningful run | Native History browser | PASS |
| History run detail | Native report projection | PASS |
| Run summary + aggregate log | Native projected files | PASS |
| Source diff | Native projected item artifact | PASS |
| Pin / Unpin / Delete / Export | Native capability-driven History management | PASS |
| Support ZIP | Native item-level Support | PASS |
| Explicit History cleanup | Native Python-owned cleanup plan + confirmation | PASS |
| Open terminal for evidence/fallback | Explicit **Open terminal evidence** action | PASS |

## Non-blocking differences

The terminal report shortcuts for PASS/problem/changed subsets are presentation conveniences,
not separate Patch Tool semantics. Native History already has the projected rows/changed counts,
so dedicated shortcut buttons are optional UI polish rather than a cutover requirement.

Direct automation and advanced CLI flags intentionally remain terminal/CLI surfaces. They include
policy overrides, explicit package selectors, recipe import/export, report automation flags,
legacy automation compatibility flags, direct COLLECT, and runner utilities. They are not missing
native interactions.

## Cutover invariant

Native-primary must never mean terminal removal:

1. Queue, Resume, History, Plan and Health may keep a backing PTY/session, but it must remain headless and must not create a normal terminal tab by default.
2. Running keeps an explicit terminal-evidence action; that action is the point where a terminal tab may be materialized.
3. Protocol loss/unsupported capability exposes an explicit fallback state; it must not silently or automatically open a terminal tab. Python policy must still not be reimplemented in JavaScript or Go.
4. `taskdeck patch` and compatibility launchers continue to work from a terminal.

The old consolidated Go regression test is itself part of the invalidation: it explicitly required `app.attachSession(...)`. Replace it with a product-level gate that rejects implicit Patch terminal tabs.
