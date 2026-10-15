# AI Smart Terminal — Contextual Troubleshooting Handoff

Updated: 2026-10-15  
Branch: `main`  
Scope: local/SSH xterm selection + nonzero OSC 133 status + strict structured AI response + user-approved per-command PTY execution.

## Implemented

- `web/featuremods/ai_terminal_troubleshoot.js`: right-click **selected text in an xterm screen** → Ask AI; preserve exact terminal session; preview and edit a bounded, redacted excerpt before sending; use existing `/api/ai/session-context` attestation, configured AI provider and per-request external consent. No request is sent on context-menu opening.
- On `taskmenu:shell-integration` `command-finished`, nonzero integer exit status (excluding interactive Ctrl+C=130) on a regular terminal produces an **opt-in** prompt. Click Yes to open the wizard and begin assistance **after** provider consent. Success, tasks, missing exit code and Ctrl+C are excluded.
- `internal/server/ai_assistant.go`: new `troubleshoot` mode in existing `/api/ai/assist` endpoint. System instructions require JSON: `{"summary":"...","commands":[{"command":"...","purpose":"...","risk":"read-only|low|medium|high|critical"}]}`. The terminal excerpt is untrusted data; no autonomous tool calls. Existing model configuration, encrypted key storage, session attestation, quotas, timeouts and provider consent all apply.
- `web/featuremods/ai_terminal_actions_core.js`: parse JSON (optionally one JSON fence), reject multiline/chained commands, shell interpolation and dangerous syntax, cap at 8 commands, deduplicate, mark all commands confirmation-required and apply risk floors independent of the AI's classification.
- Each proposal is individually selectable, copyable or executable **only after another explicit confirmation**, on the original active, attached, writable terminal, with a verified empty idle OSC 133 shell prompt and refreshed server-side session attestation. Execution sends only that command plus Enter to the specified PTY; **no batch execution** and no silent broadcast to terminal groups.
- Existing `aiplans.js` remains approval-only and has not had autonomous execution enabled.

## User validation steps

1. Update TaskDeck, hard-refresh Firefox, open a regular terminal with OSC 133 shell integration.
2. Run a harmless failing command, for example `git push` with no upstream on a test branch. Observe exit code != 0 → **AI Smart Terminal can help** prompt; click **Yes**.
3. Inspect/edit the error excerpt, grant explicit consent for provider upload, and inspect AI's JSON result and individual action cards.
4. Select a proposal and click **Run this command**. Review the second confirmation. A command is executed only after both consent/selection and confirmation; the shell must be idle and the current input line empty.
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
