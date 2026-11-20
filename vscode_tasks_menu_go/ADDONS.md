# TaskDeck Add-on API

TaskDeck add-ons extend the browser UI through a small registry and may optionally delegate actions to a local process using a bounded JSON request/response protocol.

## Security model

Process add-on manifests are loaded only from the current user's private configuration directory:

- Linux/macOS: the platform `os.UserConfigDir()/taskdeck/addons` location.
- Windows: the corresponding per-user config directory returned by Go.

TaskDeck does **not** execute add-on commands declared by a project/workspace. This avoids turning an untrusted repository into an arbitrary command-execution source.

A manifest command must:

- be an absolute path;
- point to a regular executable file;
- use manifest version `1`;
- reference only permission keys already known to TaskDeck.

The public `/api/addons` response strips the executable path and arguments.

## Manifest

Create one `.json` file per add-on in the add-on directory.

Example:

```json
{
  "version": 1,
  "id": "fixture-tools",
  "name": "Fixture Tools",
  "description": "Example TaskDeck process add-on",
  "command": "/home/user/.local/lib/taskdeck-addons/fixture-tools",
  "args": [],
  "actions": [
    {
      "id": "inspect",
      "label": "Inspect selected file",
      "keywords": "inspect metadata fixture",
      "permissions": ["files.read"]
    },
    {
      "id": "long-check",
      "label": "Run long check",
      "permissions": ["files.read"],
      "background": true
    }
  ],
  "panels": [
    {
      "id": "status",
      "title": "Fixture status",
      "action_id": "inspect"
    }
  ],
  "context_menus": [
    {
      "id": "inspect-file",
      "label": "Inspect with Fixture Tools",
      "action_id": "inspect",
      "scopes": ["file"],
      "extensions": [".txt", ".json"]
    }
  ],
  "file_previews": [
    {
      "id": "fixture-preview",
      "label": "Fixture Preview",
      "action_id": "inspect",
      "extensions": [".fixture"]
    }
  ]
}
```

Supported context scopes are `file`, `directory`, and `project`.

## Process protocol

TaskDeck starts the configured executable with fixed manifest arguments, writes exactly one JSON request to stdin, and waits for one JSON response on stdout.

Request:

```json
{
  "version": 1,
  "addon_id": "fixture-tools",
  "action_id": "inspect",
  "workspace": "/absolute/project/path",
  "context": {
    "path": "README.md",
    "source": "context-menu"
  }
}
```

Successful response:

```json
{
  "ok": true,
  "message": "Inspection completed",
  "content": "Human-readable result shown in a TaskDeck add-on panel.",
  "data": {
    "optional": "structured values"
  }
}
```

Failure response:

```json
{
  "ok": false,
  "error": "Reason the action failed"
}
```

Rules:

- timeout: 5 minutes per invocation;
- stdout/stderr capture is bounded to 2 MiB each;
- non-zero exit is treated as failure;
- browser abort/cancel propagates through the HTTP request context and terminates the process;
- output is rendered as text/JSON, not trusted HTML.

## Browser registry

Built-in or future browser modules can register the same extension points directly:

```javascript
const addons = globalThis.TaskMenuAddons;

addons.registerAction('my-addon', {
  id: 'hello',
  label: 'Say hello',
  permissions: ['files.read'],
  run: async context => ({ok: true, content: 'Hello ' + (context.path || '')})
});

addons.registerPanel('my-addon', {
  id: 'hello-panel',
  title: 'Hello',
  action_id: 'hello'
});

addons.registerContextMenuCommand('my-addon', {
  id: 'hello-file',
  label: 'Hello file',
  action_id: 'hello',
  scopes: ['file'],
  extensions: ['.txt']
});

addons.registerFilePreviewHandler('my-addon', {
  id: 'hello-preview',
  label: 'Hello Preview',
  action_id: 'hello',
  extensions: ['.hello']
});

addons.registerBackgroundJob('my-addon', {
  id: 'scan',
  label: 'Scan project',
  permissions: ['files.read'],
  run: async context => ({ok: true, content: 'done'})
});
```

Registered actions and panels automatically appear in the Unified Command Palette. Context commands are appended to the Project file menu. Matching preview handlers appear in **Open With…**. Background actions are surfaced through Operation Center and can expose cancellation.

## Permissions

The manifest does not create new RBAC keys. Each action declares zero or more existing TaskDeck permissions such as:

- `files.read`
- `files.write`
- `git.status`
- `git.write`
- `db.read`
- `db.write`
- `ssh.use`
- `transfer.read`

Unknown permissions make the manifest invalid. In shared-server mode, TaskDeck checks all declared action permissions server-side before spawning the process; browser-side disabled states are convenience only.

## Reloading

The server reads manifests on API access, so manifest changes do not require a daemon restart. Reload the browser page after adding/removing manifests so the browser registry and command palette rebuild from the current manifest set.
