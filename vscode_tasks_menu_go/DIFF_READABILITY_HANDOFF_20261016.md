# Diff readability update — 2026-10-16

**Branch:** main
**Scope:** TaskDeck File Compare (side-by-side and inline) and Git Quick Actions' syntax-aware diff preview.
**Request:** Keep the recognizable diff highlighting from the supplied visual reference: logic-sensitive changes visibly distinct, comment/format-only changes soft cyan, changed words readable without masking normal syntax tokens.

## Root cause

- Full File Compare originally used conspicuous dark rose/amber line backgrounds (`#34212b`, `#332a1d`) and Git preview repeated those styles.
- The inline change selector set `color:...!important` over syntax-highlighted spans, replacing keyword/comment/string/number colors with a single text color.
- Unimportant changes had only a whole-row tint and were difficult to distinguish from logic-sensitive changes when syntax colors were overridden.

## Implementation

1. `web/featuremods/filecompare.js`
   - Dark important rows: muted red/rose `#261b24` / `#282023`; cosmetic rows cyan `#182a30`; two-pixel inset line marker.
   - Light important rows: `#fff6f7` / `#fff4f3`; cosmetic rows `#eff9fc`.
   - Changed word chips use soft rose/red backgrounds. Only **plain text** in important changed spans gets the rose foreground; syntax-classified text retains the language token foreground.
   - Changed comment/format-only spans use muted cyan foreground/chip, including in inline mode.
   - No `color:...!important` on inline spans; no opacity for changed rows.
2. `web/featuremods/gitstatus.js`
   - Git preview uses exactly the same important/unimportant row palette and markers as full compare. It already uses `TaskMenuFileCompare` for line importance and token rendering.
3. `test_diff_color_contrast.cjs`
   - Accessible syntax contrast over all changed row and inline backgrounds, matching Git/File Compare colors, and regression guard against forced token foreground.
4. `internal/server/gitdiff_visual_ui_test.go` and `internal/server/filecompare_ui_test.go`
   - Static UI contract assertions updated to the new palette and required syntax-preservation styling.

## Verification

- JavaScript parse (File Compare, Git panel, Node test): PASS.
- Executed all **8 color/readability tests** using the exact repository test source with an in-memory Node assert/fs/test adapter: **8/8 PASS**. This does not replace running `node --test test_diff_color_contrast.cjs` on an actual checkout.
- Nim importance classification with real extracted code (comment-only, code changes, mixed comment/code) **5/5 PASS**.
- Go UI static assertions checked against live source: PASS.
- **Not run here:** full `go test ./...`, real Firefox visual smoke, multi-theme screenshot inspection. A self-update validation run or checkout with Node/Go should execute both test suites.

## Remaining manual acceptance

1. Open File Compare side-by-side and inline for a file with comments, whitespace-only changes and executable changes. Confirm full-line backgrounds are quiet, comments remain distinguishable, and strings/keywords remain color-coded.
2. Repeat in light theme and Git Preview, verifying matching palette.
3. Check accessibility/readability with scaled fonts and long lines, copy actions, horizontal scrolling and important/unimportant toggle.
4. Run `cd vscode_tasks_menu_go && node --test test_diff_color_contrast.cjs && go test ./...`.

No browser-worker or restricted developer code was changed. GitHub Actions were not manually triggered.

## 2026-10-16 follow-up — Text-only inline differences

User screenshots showed `defer:` and `nil and` still wrapped in separate red filled rectangles, even though their entire lines were already tinted. This duplicated the highlight and reduced readability.

- Replaced every inline changed-word chip with a **foreground-only** treatment in `filecompare.js`: changed important code uses bright rose/red (`#ff9ca8` dark, `#aa2036` light); changed cosmetic/comment text uses cyan (`#8fdaf2` dark, `#126079` light).
- **No inline changed-word background, box-shadow, border radius, or bold weight.** The row retains its existing muted important/cosmetic background and thin edge marker.
- The important selector now colors changed syntax-classified keywords too. Unchanged keywords, comments, strings, numbers and ordinary text keep their existing syntax colors. Shared rendering also updates Git diff preview and the inline/side-by-side views.
- Updated `test_diff_color_contrast.cjs` to verify changed text against the **actual row background** in both themes, and to assert all changed-token CSS declarations consist of `color` only. Updated the Go UI regression guard accordingly.
- Static/JS runtime contrast checks: 8/8 successful. The Go suite and a real Firefox visual acceptance run still need to execute on the user's installed TaskDeck.

No other diff classification, text comparison or file operations were changed.
