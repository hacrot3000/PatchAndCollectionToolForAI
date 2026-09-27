# Database Workbench UI — Handoff / Roadmap

> **Branch:** `feat/database-workbench-ui`  
> **Base:** `main@3de8cafef4a8bfaa7a631466ee90bb7bf2aade24`  
> **Status:** Active implementation  
> **Rule:** This file is the durable handoff. Update it at every meaningful checkpoint before moving to the next phase.

## Goal

Evolve TaskDeck's existing database panel from a schema/query viewer into a practical database workbench while preserving the existing adapter-isolated architecture and direct SQL/query editor.

Requested capabilities:

1. Perform common database/table/data actions from the UI instead of only listing objects.
2. Right-click schema/database/table/row/cell objects to expose common context-menu actions.
3. Use MySQL Workbench as the primary UX/functionality reference, adapted to TaskDeck's browser UI and multi-adapter model.
4. Provide an editable data grid with paging, row insert/edit/delete workflows, explicit save/revert, sorting/filtering where safely supported, and clear read-only behavior.
5. Keep direct SQL/query execution available as a first-class workflow.

## Reference behavior

MySQL Workbench concepts used as UX references:

- Visual SQL Editor: schema/object navigator + query editor + result grid + output/info panels.
- Schema navigator context menus for common object actions.
- “Select Rows - Limit …” to open editable live table data.
- Editable result grid only when a row can be identified safely (for example via primary/unique key).
- Table/schema inspector style metadata views.
- Result-grid actions such as opening a field value in a larger editor.

References:
- https://dev.mysql.com/doc/workbench/en/wb-sql-editor.html
- https://dev.mysql.com/doc/workbench/en/wb-sql-editor-navigator.html
- https://dev.mysql.com/doc/workbench/en/wb-develop-sql-editor-results.html
- https://dev.mysql.com/doc/workbench/en/wb-sql-editor-query-panel.html

TaskDeck will not clone Workbench pixel-for-pixel. The target is the same workflow density and discoverability while keeping TaskDeck lightweight and dependency-minimal.

## Architectural constraints

- No npm dependency when a viable browser/Go implementation exists.
- Keep database-specific behavior inside adapters; browser UI must rely on declared capabilities and generic operations.
- Preserve current process-isolated DB adapter protocol.
- Preserve bounded payload/output limits and timeouts.
- Never expose stored credentials or internal secret references to the browser.
- Mutating operations must respect profile `read_only`.
- Destructive actions require explicit confirmation in the UI.
- Row updates/deletes must use a stable row identity supplied/validated by the adapter; never guess using every visible cell.
- Existing free-form SQL/query editor remains available.
- Commit small, independently reviewable slices.

## Target UX

### Left side: navigator

- Catalog/schema selector.
- Grouped object tree (tables/views; adapter-specific groups may follow).
- Expand/collapse where metadata supports it.
- Search/filter objects.
- Double-click table: open data grid.
- Right-click database/schema/table/view: context menu.

### Main workspace

Tabs inside a DB connection:

- **Data** — table rows in paged editable grid.
- **Structure / Inspector** — columns, keys/indexes, table metadata.
- **Query** — existing direct SQL/query editor.
- Future optional tabs: DDL, explain, export/import, statistics.

### Context menus

Initial relational table menu:
- View Data
- Refresh
- Inspect / Structure
- Copy Name
- Copy Qualified Name
- Open in Query Editor
- Count Rows
- Generate SELECT
- Generate INSERT template
- Generate UPDATE template
- Generate DELETE template
- Truncate Table (write-enabled only; confirmation)
- Drop Table (write-enabled only; confirmation)

Database/schema menu:
- Set/current catalog where adapter supports it
- Refresh
- Copy Name
- Open Query Editor
- Create Table shortcut/template
- Drop database/schema only in a later hardened phase

Row/cell menu:
- Edit
- Revert cell/row
- Set NULL
- Copy value
- Copy row
- Delete row
- Open value in larger editor for long text/JSON/binary-safe representation

### Data grid

- Server-side pagination (default 100 rows, selectable bounded page sizes).
- Previous / next / first page.
- Page indicator and returned-row count; total count loaded separately/optionally.
- Sort support through generic adapter sort descriptors where safely supported.
- Simple column filters in later phase.
- Dirty-cell/dirty-row state.
- Explicit **Apply changes** and **Revert**.
- Insert row workflow.
- Delete row workflow.
- Read-only badge and disabled mutation controls.
- Adapter supplies:
  - visible columns,
  - stable identity columns / identity token,
  - editability reason when read-only,
  - typed values,
  - optional total count.

## Generic adapter protocol extensions

Do not implement UI by generating vendor SQL in the browser.

Planned generic operations:

- `browse_rows`
  - input: catalog/object/page_size/page_offset/sort/filter
  - output: columns, rows, row identity, editability, optional total count
- `mutate_rows`
  - input: ordered inserts/updates/deletes with row identity and changed values
  - output: per-change status + refreshed rows where useful
- `object_action`
  - bounded allowlisted actions such as count/truncate/drop depending on adapter capabilities
- extend `describe_object` for columns/keys/indexes/DDL-ish metadata where adapter supports it
- adapter manifest/capability flags for browse/edit/object actions

If existing `execute` safely satisfies a generated-template action, generation can remain UI-only as text inserted into Query without executing it.

## Adapter scope

### MySQL / MariaDB
Primary full-feature implementation target:
- browse/paging
- PK/unique-key based editing
- insert/update/delete
- count
- truncate/drop table via explicit object actions
- column/index metadata

### SQLite
Second relational target:
- browse/paging
- `rowid` only when valid, otherwise PK-based identity
- insert/update/delete
- table metadata
- no network-specific concepts

### MongoDB
Adapt Workbench-style workflow:
- collection data grid/document view
- paging
- `_id` identity
- document edit/insert/delete using safe JSON DSL
- no arbitrary JS

### Redis
Non-tabular semantics:
- key browser + paged SCAN
- type-aware value viewer/editor in a later phase
- mutation only when explicitly supported and profile is not read-only
- do not force relational table concepts onto Redis

## Implementation phases

### Phase 0 — durable design / protocol contract
- [x] Create feature branch from main.
- [x] Create this handoff/roadmap.
- [x] Inventory current adapter protocol and capability manifests.
- [x] Finalize generic request/response structs for browse/mutate/object actions.
- [x] Add protocol normalization/limit tests before UI depends on them.

### Phase 1 — Workbench-style navigator foundation
- [x] Add reusable native browser context-menu component (no dependency).
- [x] Add object filtering/search.
- [x] Double-click table/view opens a data workspace.
- [x] Right-click table/database exposes non-destructive actions first.
- [x] Preserve current click-to-inspect behavior.

### Phase 2 — read-only paged data grid
- [x] Implement `browse_rows` protocol.
- [x] MySQL browse_rows.
- [x] SQLite browse_rows.
- [x] UI Data tab with server-side pagination.
- [x] Page-size selector and first/prev/next navigation.
- [x] Read-only/editability status.
- [x] Row/cell copy actions.

### Phase 3 — relational inline editing
- [x] Stable row identity model.
- [x] `mutate_rows` protocol.
- [x] MySQL insert/update/delete.
- [x] SQLite insert/update/delete.
- [x] Dirty-grid state.
- [x] Apply / Revert.
- [x] Insert row.
- [x] Delete row workflow with explicit Apply/Revert; destructive table actions have confirmation.
- [x] NULL editing.
- [x] Long text/JSON value editor dialog.
- [x] Reject unsafe/no-key updates with clear reason.

### Phase 4 — object context tools / inspector
- [x] Structure/Inspector tab.
- [x] Columns and indexes.
- [x] Count Rows.
- [x] Copy / qualified name.
- [x] Generate SELECT/INSERT/UPDATE/DELETE templates into Query.
- [x] Truncate/Clear action with UI confirmation and capability/read-only gates.
- [x] Drop/Delete action with typed confirmation and capability/read-only gates.
- [x] Refresh object/schema actions.

### Phase 5 — sorting/filtering + UX depth
- [x] Column sort descriptors.
- [x] Simple safe filters.
- [x] Persist per-table page size where appropriate.
- [x] Keyboard navigation/edit shortcuts.
- [x] Better loading/error/empty states.
- [x] Result-grid cell context menu.
- [x] Optional total-row-count without blocking first page.

### Phase 6 — MongoDB workbench workflow
- [x] Collection browse_rows with `_id`.
- [x] Paged document grid.
- [x] Safe document edit/insert/delete.
- [x] Document JSON editor.
- [x] Collection context actions appropriate to MongoDB.

### Phase 7 — Redis workbench workflow
- [x] Existing SCAN-backed key browser retained; Data viewer now opens selected keys.
- [x] Type-aware viewer for String/List/Hash/Set/ZSet.
- [x] Safe bounded editor for Hash/Set/ZSet; String/List remain read-only in grid.
- [x] Count Entries / Generate Read Command / Delete Key with read-only gates.

### Phase 8 — hardening / documentation / CI
- [x] Audit events for data/object mutations without leaking values.
- [x] Payload, filter, mutation and cell-size boundary tests.
- [x] Per-row mutation/read-only/error-path tests; Workbench batches report item-level failures.
- [x] SSH-tunneled DB regression covers Workbench browse request and tunnel lifetime.
- [x] Browser source/regression tests.
- [x] README/Connections documentation.
- [ ] Full CI Go 1.19 + 1.23.
- [ ] Final diff review against main.

## Current checkpoint

**2026-09-27 — checkpoint 4: feature-complete, final CI pending**

Feature implementation is now complete for the requested Database Workbench scope:

- Workbench navigator with object search, double-click open and right-click context actions.
- Data / Structure / Query tabs; direct SQL/query/Redis command workflow remains first-class.
- Server-side paging, sort, relational filters and on-demand total count.
- Editable grid with dirty state, Apply/Revert, insert/delete, NULL and JSON/long-value editor.
- Per-object page-size persistence and keyboard shortcuts.
- MySQL/MariaDB: browse/edit/filter/count/truncate/drop + columns/indexes.
- SQLite: browse/edit/filter/count/delete-all/drop + PK/rowid identity + columns/indexes/DDL.
- MongoDB: paged documents, `_id` identity, JSON edit/insert/delete, count/clear/drop, Generate Find.
- Redis: type-aware String/List/Hash/Set/ZSet viewer; Hash/Set/ZSet safe editing; Count Entries/Delete Key and generated read commands.
- Loading/error/empty states and late adapter-capability refresh.
- Audit payload non-disclosure tests, protocol bounds, per-row mutation errors and SSH-tunnel Workbench regression.
- Workbench usage/safety documented in `CONNECTIONS.md` and linked from TaskDeck README.

Mutation batches intentionally use per-item result semantics across adapters; they are not advertised as one cross-adapter/cross-row transaction. UI reloads after Apply and reports failed items so partial success is visible.

Recent hardening/UX commits:
- `c020f8c6` page-size persistence + shortcuts
- `c12cb568` CI source-assertion fix
- `167dd912` mutation audit non-disclosure test
- `c6b95b61` Workbench documentation
- `daffc734` on-demand total count
- `1fb90341` protocol bounds
- `5977d2f4` SSH-tunnel Workbench regression
- `44636288` loading/capability hardening
- `df901f23` per-row Redis mutation error test

Current base relationship:
- branch created from `main@3de8cafef4a8bfaa7a631466ee90bb7bf2aade24`
- main had not moved as of the last comparison; branch was ahead only, behind 0.

Remaining release steps:
1. Confirm both Go 1.19.x and Go 1.23.x CI jobs green at latest functional HEAD.
2. Remove the temporary `feat/database-workbench-ui` workflow trigger.
3. Verify workflow file is byte-for-byte equivalent to main again.
4. Final compare/diff review against main.
5. Record final CI evidence in this handoff.

## Continuation rule

When resuming after timeout/error:
1. Read this file first.
2. Inspect branch HEAD and the last 5–10 commits.
3. Continue from **Current checkpoint**; do not infer progress only from file names.
4. Before starting a new phase, update this file and commit the checkpoint.
5. Prefer one behavior/test slice per commit.
