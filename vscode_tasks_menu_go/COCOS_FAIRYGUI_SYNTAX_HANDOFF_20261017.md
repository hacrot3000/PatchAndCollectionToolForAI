# TaskDeck — Cocos Creator / FairyGUI syntax (2026-10-17)

## Tóm tắt / Summary

**VI.** TaskDeck đã có TypeScript syntax qua CodeMirror 6 cho `.ts`, `.tsx`,
`.d.ts` và các dạng `.mts` / `.cts`. Bản thay đổi này bổ sung nhận diện
định dạng Cocos/FairyGUI và snippet Cocos trong TypeScript, **không thêm một
parser TypeScript cạnh tranh**, không đổi cơ chế Save và không xử lý sai binary.

**EN.** Existing vendored CodeMirror 6 provides TypeScript highlighting.
This change adds Cocos Creator and FairyGUI filename mapping and context-gated
TypeScript snippets without replacing the parser or touching save semantics.

### File mapping

| Extension | Editor | File Compare / Git diff |
|---|---|---|
| `.ts`, `.tsx`, `.d.ts`, `.mts`, `.cts` | TypeScript / TSX | TypeScript |
| `.prefab`, `.scene`, `.fire`, `.anim`, `.meta` | Cocos JSON (JSON mode) | JSON |
| `.fairy` | FairyGUI Project (XML mode) | XML |
| `package.xml` and FairyGUI component `.xml` | XML (already supported) | XML |
| `.bin`, `.fui` | **No text syntax mapping — binary** | **No text syntax mapping** |

The FairyGUI official Cocos sample `UIProject/*.fairy` starts with
`<?xml ...?><projectDescription .../>`. Published Cocos GUI descriptors
are binary by default; do not rename or parse them as text/XML.

### TypeScript snippets

- Completion for Cocos 2.x / 3.x components, Cocos `@property` and a
  FairyGUI `UIPackage.createObject` expression is **opt-in by source**.
- Enabled only when a TypeScript document contains a reference such as
  `from 'cc'`, `from 'fairygui-cc'`, `cc._decorator`, `@ccclass`
  or `fgui.UIPackage` (bounded to the first 32 KiB).
- Ordinary TypeScript files retain their original keyword/snippet lists;
  existing autocomplete and LSP integration are not replaced.
- Templates are optional scaffolding; developers should adapt imports,
  property types and SDK APIs for their Cocos version.

## Thay đổi / Changed areas

- `web/featuremods/editor.js`: Cocos JSON + FairyGUI XML registry entries.
- `web/featuremods/filecompare.js`: mapped comparison language IDs.
- `web/featuremods/editorcompletion.js`: source-gated extra snippets.
- `internal/server/cocos_fairygui_syntax_ui_test.go`: embedding assertions.
- `tests/cocos_fairygui_syntax.test.mjs`: runtime detection & regression cases.
- `README.md`: Vietnamese/English sections.

## Kiểm thử / Verification

Run from `vscode_tasks_menu_go`:

```bash
node --test tests/cocos_fairygui_syntax.test.mjs
go test ./internal/server -run 'Test(CocosFairyGuiSyntaxBindings|EditorInitialLanguageCoverageMapping|EditorCompletionTierOneProfilesCoverRegisteredLanguages)' -count=1
```

**Completed in this chat:** V8 parse of 3 modified frontend modules PASS,
17/17 focused runtime assertions PASS (TypeScript regression, Swift regression,
Cocos JSON/FairyGUI XML mappings, binary exclusions, diff mappings, and TS
context detection). **Not executed here:** repository-local Node / Go tests
and manual browser smoke.

Recommended browser smoke:
1. Open a Cocos script `Player.ts` containing
   `import { _decorator } from 'cc'` and invoke completion with Ctrl+Space.
2. Open an unrelated `normal.ts`; confirm Cocos templates do not appear.
3. Open `UIProject/Project.fairy`, `UIProject/assets/Bag/package.xml`,
   `assets/Player.prefab` and `assets/Player.prefab.meta`.
4. Open File Compare and a Git diff for the above file types.
5. Do not interpret a published `*.bin` or `*.fui` asset as text.

## Reference / Tham khảo

- FairyGUI Editor project structure:
  https://en.fairygui.com/docs/guide/index.html
- FairyGUI official Cocos integration:
  https://en.fairygui.com/en/docs/sdk/creator/
- Cocos Creator decorators:
  https://docs.cocos.com/creator/3.8/api/en/namespace/_decorator
