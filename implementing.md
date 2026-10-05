Dựa trên những gì TaskDeck hiện đã có, phần nền tảng đã khá rộng: file editor/split, terminal/session broker, SSH, FTP/SFTP, database, Patch Tool, Git Quick Actions, multi-repo, shared-server, self-update… Phần còn thiếu để tiến gần một công cụ quản trị project “trọn bộ” chủ yếu nằm ở **workflow liên kết giữa các module**, không phải chỉ thêm từng nút riêng lẻ.

Tôi đề xuất roadmap sau, theo thứ tự ưu tiên:

1. **P0 — Project/File Explorer thực sự hoàn chỉnh**
   - Tree file/folder riêng bên trái, không phụ thuộc Patch/File Transfer.
   - Multi-select file/folder.
   - Cut / Copy / Paste / Rename / Move / Duplicate.
   - Tạo file/folder.
   - Trash/Recycle thay vì delete vĩnh viễn ngay.
   - Undo thao tác file gần nhất.
   - Drag & drop file/folder trong tree.
   - Reveal file đang mở trong explorer.
   - Open containing folder.
   - Favorites/Pinned paths.
   - Recent files.
   - Hiển thị Git status trực tiếp cạnh file: `M`, `A`, `D`, `U`, untracked…
   - Context menu thống nhất giữa File Editor, SFTP, Git và Project Explorer.

2. **P0 — Project-wide Search / Replace**
   - Dùng `ripgrep` nếu có, fallback Go implementation, không cần npm.
   - Search text toàn project.
   - Regex / case-sensitive / whole word.
   - Include/exclude glob.
   - Respect `.gitignore`.
   - Search trong một folder được chọn.
   - Preview match theo file.
   - Replace one / replace file / replace all.
   - Trước `Replace all` phải hiện preview diff.
   - Có Undo cho replace hàng loạt.
   - Click result mở file đúng line/column.
   
   Đây là tính năng tôi đánh giá **nên làm rất sớm**, vì nó nối File Manager + Editor + Git thành một workflow thực sự.

3. **P0 — File compare / compare arbitrary versions**
   - Bạn vừa có Git visual diff, nên mở rộng engine này thành generic compare.
   - Compare:
     - file A ↔ file B;
     - local ↔ remote SFTP;
     - current file ↔ saved file;
     - current file ↔ clipboard;
     - current file ↔ Git commit;
     - commit A ↔ commit B.
   - Side-by-side hoặc inline.
   - Copy change trái→phải / phải→trái.
   - Khi có conflict có thể dùng cùng component làm merge editor 3-way.

4. **P0 — Git 3-way conflict editor**
   Hiện conflict wizard đã tốt, nhưng bước tiếp theo nên là:
   - `Base`
   - `Current / Ours`
   - `Incoming / Theirs`
   - `Result`
   
   Có các action:
   - Accept Current.
   - Accept Incoming.
   - Accept Both.
   - Copy selected block.
   - Next/previous conflict.
   - Mark resolved.
   
   Đây sẽ là nâng cấp rất lớn so với conflict wizard dạng command.

5. **P0 — Git commit graph**
   - Graph branch/merge trực quan.
   - Local + remote refs.
   - Tags.
   - Search commit.
   - Filter author/date/message/path.
   - Click commit xem changed files.
   - Visual diff commit.
   - Context menu:
     - checkout;
     - create branch here;
     - cherry-pick;
     - revert;
     - reset;
     - tag;
     - compare with HEAD.
   
   Với lượng tính năng Git hiện có, commit graph là phần UI còn thiếu rõ nhất.

6. **P0 — Git Reflog + Recovery Center**
   Vì TaskDeck đã hỗ trợ khá nhiều thao tác Git nguy hiểm, nên nên có một lớp cứu hộ:
   - `git reflog`.
   - Restore branch accidentally deleted.
   - Recover detached HEAD.
   - Recover reset commit.
   - Recover lost commit after force-delete branch.
   - Recreate branch from reflog entry.
   - Copy SHA.
   
   Đặc biệt sau khi vừa thêm `Force delete local`, đây là tính năng rất hợp lý.

7. **P1 — Git Worktree Manager**
   Rất phù hợp với workflow developer:
   - List worktrees.
   - Create worktree from branch.
   - Create branch + worktree.
   - Open TaskDeck project tại worktree mới.
   - Remove/prune worktree.
   - Hiển thị branch đang bị checkout ở worktree nào.
   
   Ví dụ thay vì switch branch làm mất trạng thái hiện tại:
   `main` ở folder A và `fix/foo` ở folder B cùng lúc.

8. **P1 — Git interactive rebase**
   - Chọn range commit.
   - Pick / Reword / Edit / Squash / Fixup / Drop.
   - Drag reorder.
   - Preview resulting history.
   - Abort/continue khi conflict.
   
   Nếu làm được phần này, Git Panel gần như thay thế được nhiều Git GUI desktop phổ biến.

9. **P1 — Git reset/revert rõ semantics**
   Nên có wizard thay vì raw buttons:
   - Reset soft.
   - Reset mixed.
   - Reset hard.
   - Revert commit.
   - Restore file from commit.
   - Restore staged only.
   
   Phải giải thích rõ:
   `HEAD`, `Index`, `Working tree` sẽ thay đổi thế nào trước khi Confirm.
   
   Visual 3-state diff vừa làm có thể tái sử dụng để giải thích.

10. **P1 — Git Submodule Manager**
    - Status.
    - Init/update.
    - Update recursive.
    - Sync.
    - Checkout expected commit.
    - Detect dirty submodule.
    - Open submodule như repository riêng trong Git Panel.
    
    Multi-repo hiện có rồi nên bước này tương đối tự nhiên.

11. **P1 — Project snapshots/checkpoints**
    Không phải Git commit, mà là checkpoint TaskDeck:
    - File layout.
    - Open tabs.
    - Terminal groups.
    - Terminal CWD.
    - Database tabs/query.
    - SFTP location.
    - Selected Git repo/branch.
    
    Ví dụ:
    `Save workspace snapshot: "Debug NFC issue"`
    
    Sau đó restore nguyên context làm việc.

12. **P1 — Project profiles**
    Một project có thể có nhiều profile:
    - `Development`
    - `Debug`
    - `Production`
    - `Customer A`
    
    Mỗi profile lưu:
    - environment variables;
    - command presets;
    - terminal startup;
    - DB connection;
    - SSH;
    - FTP/SFTP;
    - default Git repository;
    - task set.
    
    Việc chuyển project context sẽ nhanh hơn rất nhiều.

13. **P1 — Workspace multi-root thực sự**
    Hiện có multi-repository trong một workspace; bước tiếp theo là:
    - attach nhiều root folder;
    - mỗi root có tasks/config riêng;
    - explorer hiển thị nhiều roots;
    - search across roots;
    - Git repo mapping theo root;
    - terminal `Open here`.
    
    Ví dụ M3:
    `m3-client`, `m3-server`, tools, deploy scripts có thể thành các root độc lập.

14. **P1 — File editor mạnh hơn**
    Nếu muốn TaskDeck thay VS Code cho các thao tác nhanh:
    - Go to line.
    - Find/replace trong file.
    - Bracket matching.
    - Highlight current line.
    - Whitespace toggle.
    - Minimap optional.
    - Detect EOL/encoding.
    - Change encoding.
    - LF/CRLF conversion.
    - Large-file mode.
    - Hex viewer.
    - Read-only viewer.
    - Auto-save configurable.
    - Unsaved-change indicator trên tab.
    - Reopen closed tab.
    - File history local.
    
    Không nhất thiết phải biến thành IDE đầy đủ.

15. **P1 — Symbol navigation nhẹ**
    Không cần LSP ngay.
    
    Có thể bắt đầu bằng:
    - function/class/method outline;
    - jump to symbol;
    - search symbol trong project;
    - breadcrumb.
    
    Parser heuristic/ctags nếu có sẵn trên host, tránh kéo dependency nặng.

16. **P1 — Terminal workspace nâng cao**
    Terminal hiện đã khá mạnh. Phần nên thêm:
    - duplicate terminal;
    - clone terminal CWD + env;
    - move terminal giữa split groups;
    - save terminal layout thành preset;
    - startup terminal set;
    - command palette cho terminal;
    - search toàn scrollback nhiều terminal;
    - broadcast command preview trước khi gửi;
    - per-group environment.
    
    Đặc biệt hữu ích:
    `Duplicate terminal here`
    và
    `Open terminal in selected file folder`.

17. **P1 — Terminal session recording**
    - Record command + output + exit code + duration.
    - Export session Markdown/text.
    - Bookmark output line.
    - Annotate session.
    - Restore command history after daemon restart.
    
    Có thể dùng rất tốt khi debug firmware/server.

18. **P1 — Unified transfer/sync engine**
    Hiện FTP/SFTP đã có queue rất mạnh. Nên nâng thành:
    - Local ↔ Remote sync.
    - Compare by size/mtime/checksum.
    - Dry-run.
    - Mirror one-way.
    - Bidirectional sync.
    - Conflict detection.
    - Exclude patterns.
    - Save Sync Profile.
    
    Quan trọng: mặc định không delete destination nếu chưa explicit enable.

19. **P1 — Checksum / file integrity tools**
    Context menu file:
    - SHA-256.
    - MD5 nếu cần compatibility.
    - Compare checksum.
    - Generate manifest.
    - Verify manifest.
    
    Rất phù hợp với workflow firmware/deploy của bạn.

20. **P1 — Archive support**
    - Preview ZIP/tar.gz without extracting.
    - Extract.
    - Create archive.
    - Add selected files.
    - Download selected as ZIP.
    - Upload and extract remote.
    
    Đặc biệt hữu ích cho Patch Tool và artifact handoff.

21. **P1 — Unified “Open With”**
    Right-click file:
    - Text Editor.
    - Hex.
    - Image Preview.
    - Markdown Preview.
    - Diff.
    - Download.
    - Open in terminal directory.
    - Open via host application nếu local mode cho phép.
    
    Đây sẽ giúp giảm logic detect-file rời rạc hiện nay.

22. **P1 — Project Tasks dependency graph**
    Tasks hiện chủ yếu là launcher.
    
    Nên hỗ trợ:
    - dependsOn.
    - parallel/sequential.
    - retry.
    - timeout.
    - continue-on-error.
    - conditional.
    - output dependency.
    - DAG visualizer.
    
    Ví dụ:
    `Build → Test → Package → Deploy`.

23. **P1 — Task run history**
    - Lần chạy gần nhất.
    - Duration.
    - Exit code.
    - Git commit lúc chạy.
    - Environment/profile.
    - Log artifact.
    - Compare two task runs.
    
    Rất hữu ích cho build/OTA/deployment.

24. **P1 — Unified command palette**
    Tôi đánh giá đây là một trong những cải tiến UX đáng làm nhất:
    
    `Ctrl+Shift+P`
    
    Search:
    - Open terminal.
    - Open file.
    - Run task.
    - Git action.
    - SSH.
    - Database.
    - SFTP.
    - Settings.
    - Project.
    
    Với số lượng tính năng TaskDeck hiện tại, menu bằng chuột sẽ bắt đầu khó mở rộng.

25. **P1 — Global Quick Open**
    `Ctrl+P`
    
    Search:
    - files;
    - recently opened;
    - tabs;
    - terminals;
    - databases;
    - saved connections.
    
    Có thể dùng prefix:
    - `>` command
    - `@` symbol
    - `#` text search
    - `git:` branch/commit
    - `ssh:` profile

26. **P1 — Unified Activity/Operation Center**
    Hiện nhiều subsystem đều có background work riêng.
    
    Nên gom:
    - Git jobs.
    - File copy.
    - SFTP/FTP queue.
    - Patch runs.
    - Task runs.
    - self-update.
    - DB export/import.
    
    Một panel:
    `Queued / Running / Completed / Failed`
    
    Với:
    - cancel;
    - retry;
    - copy error;
    - open source;
    - clear completed.
    
    Đây sẽ giúp kiến trúc UI thống nhất hơn đáng kể.

27. **P2 — Database schema diff**
    Vì DB module đã khá mạnh:
    - Compare DB A/B.
    - Compare schema only.
    - Generate ALTER migration.
    - Preview before apply.
    - Export structure.
    
    Có thể đặc biệt hữu ích staging ↔ production.

28. **P2 — DB transaction workspace**
    Query tab có:
    - Begin transaction.
    - Commit.
    - Rollback.
    - Auto-commit toggle.
    - Show transaction active indicator.
    
    Khi edit dữ liệu trực tiếp, có thể gom nhiều edit rồi commit cùng lúc.

29. **P2 — DB query explain**
    MySQL:
    - EXPLAIN.
    - EXPLAIN ANALYZE nếu server hỗ trợ.
    
    SQLite:
    - EXPLAIN QUERY PLAN.
    
    Hiển thị plan dạng tree.

30. **P2 — Connection tunneling graph**
    Với SSH + DB + SFTP:
    
    Hiển thị:
    `Browser → TaskDeck → SSH host → Tunnel → Database`
    
    Có thể xem:
    - tunnel port;
    - PID;
    - active connection;
    - reconnect.
    
    Khi lỗi `1045`, timeout, SSH drop sẽ dễ chẩn đoán hơn.

31. **P2 — File watcher**
    - Detect file changed externally.
    - Auto reload nếu tab chưa dirty.
    - Conflict dialog nếu tab đang sửa.
    - Detect generated files.
    
    Cực kỳ quan trọng nếu TaskDeck chạy cùng IDE khác hoặc compiler.

32. **P2 — Project health/dashboard**
    Một trang overview:
    - Git branch/status.
    - modified files.
    - running tasks.
    - open terminals.
    - failing transfers.
    - DB connections.
    - SSH status.
    - project disk usage.
    - last build.
    
    Khi mở project có thể thấy tình trạng ngay.

33. **P2 — Permission granularity cho Shared Server**
    Hiện shared server đã có users/roles/permissions; nên mở rộng permission đến:
    - filesystem read/write/delete;
    - terminal create/control;
    - Git push;
    - Git remote delete;
    - force branch delete;
    - DB read/write/schema;
    - SSH;
    - FTP/SFTP upload/delete;
    - self-update.
    
    Các action nguy hiểm nên có permission riêng:
    `git.remote.delete`, `git.force_delete`, `filesystem.delete`, `db.write`.

34. **P2 — Approval workflow cho action nguy hiểm**
    Khi shared-server:
    - Delete remote branch.
    - Force delete branch.
    - `git reset --hard`.
    - recursive remote delete.
    - production DB write.
    - deploy.
    
    Có thể yêu cầu:
    - confirm;
    - nhập branch name;
    - hoặc admin approval.
    
    Không cần bật mặc định, nhưng có thể cấu hình theo project.

35. **P2 — Audit timeline**
    Shared server hiện đã có audit nền.
    
    Nên làm UI timeline:
    - user;
    - timestamp;
    - Git operation;
    - file write/delete;
    - terminal/task;
    - DB mutation;
    - remote transfer;
    - config changes.
    
    Filter theo user/action/resource.

36. **P2 — Secrets/credentials manager**
    Thay vì credential phân tán:
    - SSH key.
    - DB password.
    - FTP password.
    - Git token.
    - deploy secret.
    
    Một secret store thống nhất:
    - encrypted at rest;
    - secret reference trong profile;
    - không serialize secret vào project config;
    - permission/audit.
    
    TaskDeck hiện đã có `secretstore`, nên đây là mở rộng hợp lý.

37. **P2 — Backup/restore TaskDeck configuration**
    Export:
    - project settings;
    - command presets;
    - task layout;
    - terminal layouts;
    - profiles;
    - connections không bao gồm secret hoặc optionally encrypted secrets.
    
    Import trên máy khác.

38. **P2 — Plugin/add-on API**
    Khi số module tăng thêm, thay vì tiếp tục hard-code:
    - register panel;
    - register action;
    - register context-menu command;
    - register file preview handler;
    - register background job;
    - permission declaration.
    
    Có thể dùng Go/process protocol đơn giản, không cần npm ecosystem.
    
    Đây là hướng kiến trúc dài hạn, chưa cần làm ngay.

39. **P3 — LSP integration**
    Chỉ nên làm sau các phần trên.
    
    Có thể dùng LSP server đã cài trên host:
    - `gopls`
    - clangd
    - tsserver alternatives
    - pylsp
    
    TaskDeck chỉ làm LSP client.
    
    Khi đó có:
    - diagnostics;
    - go-to-definition;
    - references;
    - hover;
    - rename symbol.
    
    Không cần bundle compiler/npm.

40. **P3 — Remote workspace**
    Cho TaskDeck daemon quản lý project nằm trên SSH host:
    - explorer remote;
    - terminal remote;
    - Git remote-host side;
    - remote editor save;
    - database/tunnel integration.
    
    Đây là bước lớn nhưng khi hoàn thiện sẽ đưa TaskDeck từ “web task manager” thành một dạng lightweight remote development environment.

Nếu chỉ chọn **10 mục nên làm tiếp ngay**, tôi sẽ ưu tiên theo thứ tự:

**Project Explorer → Project Search/Replace → Generic Visual Diff/3-way Merge → Git Commit Graph → Git Reflog Recovery → Command Palette → Unified Operation Center → Git Worktree → Project Snapshots → File Watcher.**

Bộ 10 này có lợi hơn việc tiếp tục thêm các action Git nhỏ lẻ, vì nó bắt đầu kết nối toàn bộ **file + editor + terminal + Git + task + remote connections** thành một workflow thống nhất.
