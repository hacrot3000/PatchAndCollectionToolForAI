# TaskDeck Patch Tool — Final Native Parity Audit

Date: 2026-09-24  
Audited baseline: `64dbe8e8` (`Add native History cleanup UI`)  
Scope: Phase 2H.7 final native/terminal parity gate  
Runtime changes in this checkpoint: **none**

## Result

**PASS — no MUST-native blocker remains.**

TaskDeck now covers the interactive operator surfaces that previously required the Patch Tool
terminal UI. Python remains authoritative for queue/recovery/history policy. TaskDeck consumes
bounded structured protocol state and sends only narrow prompt-bound actions.

The PTY is not removed. It remains a supported evidence/fallback surface and the direct
`taskdeck patch ...` CLI remains supported.

## Interactive parity matrix

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

1. Queue, Resume, History, Plan and Health keep their PTY session attached but inactive.
2. Running keeps an explicit terminal-evidence action.
3. Protocol loss/unsupported capability continues to fail back to PTY instead of reimplementing
   Python policy in JavaScript or Go.
4. `taskdeck patch` and compatibility launchers continue to work from a terminal.

The consolidated Go regression test for this audit guards these invariants plus the major
MUST-native controls/endpoints.
