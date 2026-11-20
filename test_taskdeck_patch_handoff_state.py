#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parent

def read(name: str) -> str:
    path = ROOT / name
    if not path.is_file():
        raise SystemExit(f"missing required document: {name}")
    return path.read_text(encoding="utf-8")

handoff = read("TASKDECK_PATCH_HANDOFF.md")
plan = read("TASKDECK_PATCH_ADDON_PLAN.md")
readme = read("README.md")

required_handoff = [
    "Integration is NOT complete. Phase 2 is IN PROGRESS.",
    "Phase 2E.2 — Installed-runtime browser smoke — PENDING",
    "Do not mark Phase 2E DONE until this real-runtime smoke passes.",
    "Phase 2F — Cutover closeout — PENDING",
    "89d3a458",
]
required_plan = [
    "Status: **IN PROGRESS**",
    "- [ ] Phase 2E.2 installed-runtime browser smoke after self-update",
    "- [ ] **Phase 2F — Cutover closeout.**",
    "TASKDECK_PATCH_HANDOFF.md",
]
required_readme = [
    "browser smoke trên bản TaskDeck đã self-update vẫn còn pending",
    "installed-runtime browser smoke after self-update is still pending",
    "TASKDECK_PATCH_HANDOFF.md",
]
forbidden_plan = [
    "Integration roadmap complete.",
    "Status: **DONE**\n\n> **Correction 2026-09-25:**",
]

for needle in required_handoff:
    if needle not in handoff:
        raise SystemExit(f"handoff missing recovery invariant: {needle!r}")
for needle in required_plan:
    if needle not in plan:
        raise SystemExit(f"roadmap missing incomplete-state contract: {needle!r}")
for needle in required_readme:
    if needle not in readme:
        raise SystemExit(f"README overstates or loses native Patch recovery state: missing {needle!r}")
for needle in forbidden_plan:
    if needle in plan:
        raise SystemExit(f"roadmap regressed to premature completion marker: {needle!r}")

print("PASS: TaskDeck Patch handoff/roadmap remain explicitly incomplete until Phase 2E.2 browser smoke")
