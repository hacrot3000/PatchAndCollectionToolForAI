# TaskDeck Project Notes

Project Notes stores reusable prompts, templates and project reminders in each TaskDeck **workspace**. Open the existing grouped workspace-tools menu (alongside Workspace snapshots, Operation Center and Project Health), then choose **Project notes**. Mobile has the same entry in Workspace · Tools.

## Behavior

- Each note has an optional **category**, a required **title**, and multiline **content**.
- Create, edit, delete, search by title/category/content, and filter by category.
- **Copy** copies only the raw content, preserving newlines, formatting, code blocks and whitespace, so it can be pasted directly as a prompt.
- Notes are ordered by most recently updated. Deletion is confirmed in the UI.
- Text is inserted with DOM `textContent`, never interpreted as HTML.

## Storage and isolation

- File: `<project>/.vscode/vscode_tasks_menu.notes.json`, written atomically with file mode `0600`.
- No browser localStorage and no online note service.
- Each workspace has its own file. In shared-server mode the file additionally keys notes to the authenticated **project ID**, preventing cross-project reads, edits and deletes even if physical storage is shared.
- Members of the same authenticated project share notes when authorized: `settings.read` for listing/copy, `settings.write` for create/edit/delete (project admins also allowed). This is **project-shared**, not a private notebook per user.
- Limits: 250 notes per project, maximum 180 characters/title, 80 characters/category, 128 KiB/content, 8 MiB/store.

**Privacy:** project notes are plaintext project files. Avoid putting passwords, API keys or other secrets in prompts. Project backup, file syncing or Git commits may include them unless excluded by your workflow.

## API

`GET /api/project-notes` → `{"notes":[...]}` (most recently updated first)

`POST /api/project-notes` → create, JSON `{"category":"Prompt","title":"...","content":"..."}`

`PUT /api/project-notes` → update, same JSON plus `"id":"<existing id>"`

`DELETE /api/project-notes?id=<id>` → delete only within current project.

The API validates text and request size; it does not execute prompt content.

## Validation

```bash
cd vscode_tasks_menu_go
go test ./internal/server -run '^TestProjectNotes' -count=1
node --check web/featuremods/projectnotes.js
```

The TaskDeck Remote Agent (Go) workflow also runs these checks on pushes to `main`.
