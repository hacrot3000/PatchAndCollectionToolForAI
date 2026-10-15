# AI Smart Terminal — Contextual Troubleshooting Handoff

Updated: 2026-10-15  
Branch: `main`  
Scope: local/SSH xterm selection + nonzero OSC 133 status + strict structured AI response + user-approved per-command PTY execution.

## Implemented

- `web/featuremods/ai_terminal_troubleshoot.js`: right-click **selected text in an xterm screen** → Ask AI; preserve exact terminal session; preview and edit a bounded, redacted excerpt before sending; use existing `/api/ai/session-context` attestation, configured AI provider and explicit Ask AI click as per-request consent, while configured provider consent remains required. No request is sent on context-menu opening.
- On `taskmenu:shell-integration` `command-finished`, nonzero integer exit status (excluding interactive Ctrl+C=130) on a regular terminal produces an **opt-in** prompt. The prompt contains only **Ask AI Smart Terminal to help diagnose this error?** and Yes / Not now (no separate error-code heading). Clicking Yes explicitly opens the wizard and starts the provider request without another confirmation dialog. Success, tasks, missing exit code and Ctrl+C are excluded.
- `internal/server/ai_assistant.go`: new `troubleshoot` mode in existing `/api/ai/assist` endpoint. System instructions require JSON: `{"summary":"...","commands":[{"command":"...","purpose":"...","risk":"read-only|low|medium|high|critical"}]}`. The terminal excerpt is untrusted data; no autonomous tool calls. Existing model configuration, encrypted key storage, session attestation, quotas, timeouts and provider consent all apply.
- `web/featuremods/ai_terminal_actions_core.js`: parse JSON (optionally one JSON fence), reject multiline/chained commands, shell interpolation and dangerous syntax, cap at 8 commands, deduplicate, mark all commands confirmation-required and apply risk floors independent of the AI's classification.
- Each proposal is individually selectable, copyable or executable **only after another explicit confirmation**, on the original active, attached, writable terminal, with a verified empty idle OSC 133 shell prompt and refreshed server-side session attestation. Execution sends only that command plus Enter to the specified PTY; **no batch execution** and no silent broadcast to terminal groups.
- Existing `aiplans.js` remains approval-only and has not had autonomous execution enabled.

## Prevent stale prompts after page reload (2026-10-15)

**Root cause:** When the browser reconnects to an existing PTY after reload, the server replays stored scrollback. The replay contains historical OSC 133 shell execution/completion markers. The browser-side shell parser assigned `Date.now()` to the replayed `command-finished`, so the previous error looked fresh and unexpectedly opened the Ask AI popup.

**Fail-closed behavior (implemented in `ai_terminal_troubleshoot.js`):**
- The AI failure prompt is **not** restored from scrollback or created merely because OSC 133 reports an error. It is armed only by a verified **`execution-started` with `source:'interactive-enter'`** emitted for actual user Enter input in this page, or an explicitly confirmed AI Run action, for that terminal.
- A prompt requires a nonzero exit status, the same command text, exact current shell-integration command object, the matching `startedAt` execution identity, the same **active terminal tab/pane**, and a page-local submission with a matching execution identity (bounded to 24 hours to accommodate long-running commands). The submission is consumed once. If any evidence is missing or mismatched, there is **no popup**.
- The popup shows the verified command text (e.g. `gp`), without the old `Command failed (exit N)` title. When clicking Yes, it checks that the same command is still the latest in the original terminal. Tab switches dismiss the popup and discard pending submissions. A command started by another attached client or a replayed shell preexec also dismisses stale offers.
- The existing **manual right-click → Ask AI about selection** remains available at any time, including for historical output.
- **Trade-off:** For terminals that cannot prove a live Enter (older shell without matching integration), TaskDeck deliberately skips the automatic popup rather than risk showing an unrelated old error.

Regression cases include old OSC 133 replay, refreshed timestamps, mismatched commands, active/split-pane switching, latest command correctness, and normal fresh failure assistance.

## Split-aware minimize / restore (2026-10-15)

- After a successful per-card **Copy**, **Copy JSON**, or confirmed **Run this command**, the same troubleshooting dialog minimizes to a compact **✦ AI Smart Terminal** dock at the bottom-right of the **original `view.pane`**. There is also a manual **Minimize** button.
- The dock has two controls: **✦ AI Smart Terminal** restores the exact same dialog and existing AI response/command cards, **×** closes and disposes of that dialog. Restoring never calls `/api/ai/assist` and never re-sends terminal output.
- The dock is appended to the terminal pane (which TaskDeck's split layout already positions and resizes) instead of the global browser viewport. Resizing/splitting or switching tabs therefore keeps the button on its own terminal; a hidden pane hides its dock.
- A 320ms `transform: translate/scale` Web Animations transition shows the dialog shrinking into the icon and reversing on restore. Browsers without Web Animations, or users enabling reduced motion, receive an immediate state transition.
- Keyboard focus moves back to the originating terminal after minimizing **only if** that terminal is the active pane, and returns to the restored dialog on expansion.
- A failed clipboard write or declined/blocked command leaves the wizard expanded. The command execution re-attestation, shell-idle checks and confirmation remain mandatory.
- Regression tests cover split pane ownership, no duplicate AI request on restore, successful Copy/Run, denied Run, failed clipboard, manual Minimize, dock Close, and animation/focus.

## User validation steps

1. Update TaskDeck, hard-refresh Firefox, open a regular terminal with OSC 133 shell integration.
2. Run a harmless failing command, for example `git push` with no upstream on a test branch. Observe exit code != 0 → **AI Smart Terminal can help** prompt; click **Yes**.
3. With a selected excerpt, review/edit the text and click **Ask AI** once. With an exit-error prompt, **Yes · Ask AI** directly starts the provider request. There is **no duplicate send-excerpt confirm dialog**. Configured AI consent and session attestation remain required.
4. Select a proposal and click **Run this command**. The separate command-execution confirmation remains mandatory; the shell must be idle and the input line empty.
5. For selection, highlight terminal text, right-click **Ask AI about selection**; verify opening the wizard alone causes no provider request.
6. Verify a running interactive program or a nonempty input line causes **Run** to be refused; changing tabs or disconnecting the target also blocks execution.
7. Review model-specific provider behavior: JSON formatting may vary; if the AI returns unstructured text, the raw answer may be viewed/copied but no executable cards are created.

## Tests and constraints

- `node --test test_ai_terminal_actions.cjs`: structured parsing, malformed / injection rejection, risk and idle-shell gate.
- `node --test test_ai_terminal_troubleshoot.cjs`: selection context menu, opt-in exit prompt, consent, action cards, explicit per-command confirmation, non-idle and permission rejection.
- Existing JavaScript simulated tests were run via JS harness; full GitHub CI, full Go build and real Firefox/PTY smoke **have not been verified** in this session.
- No automatic execution and no external-provider request on terminal command failure alone. The provider may still return incorrect or unsafe suggestions; humans must review output/commands. The browser client is not a security boundary against someone already able to directly control the PTY.
- Trace-worthy future improvements: persistent rejection/ignore controls for noisy failing commands, richer risk classification and named command-argument templates, better detection of unknown OSC 133 shells, showing provider request diagnostics in this wizard.

## Mở rộng / Future integrations

Reuse `TaskDeckAITerminalActionsCore` and the `troubleshoot` response schema for other command families. Maintain explicit per-action approval and server attestation. Do not make existing Guided Plans auto-executable.
