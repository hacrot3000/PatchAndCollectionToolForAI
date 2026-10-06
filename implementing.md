Dựa trên những gì TaskDeck hiện đã có, phần nền tảng đã khá rộng: file editor/split, terminal/session broker, SSH, FTP/SFTP, database, Patch Tool, Git Quick Actions, multi-repo, shared-server, self-update… Phần còn thiếu để tiến gần một công cụ quản trị project “trọn bộ” chủ yếu nằm ở **workflow liên kết giữa các module**, không phải chỉ thêm từng nút riêng lẻ.

Tôi đề xuất roadmap sau, theo thứ tự ưu tiên:

1. **P0 — Project/File Explorer thực sự hoàn chỉnh** — ✅ **COMPLETE**
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

2. **P0 — Project-wide Search / Replace** — ✅ **COMPLETE**
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

3. **P0 — File compare / compare arbitrary versions** — ✅ **COMPLETE**
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

   Hoàn tất hiện tại:
   - generic compare độc lập với Git panel, có side-by-side/inline và copy từng change block theo chiều writable;
   - nguồn Project file, buffer Editor chưa save, Saved file, Clipboard, Git commit, Git HEAD/Index/Working, Host/Local browser file và FTP/SFTP remote text;
   - Git commit ↔ current ưu tiên buffer editor đang mở; Git state compare hỗ trợ HEAD ↔ Staged, Staged ↔ Working và HEAD ↔ Working;
   - remote compare dùng text API giới hạn kích thước + SHA compare-and-swap để không overwrite file đã đổi sau lúc load.

4. **P0 — Git 3-way conflict editor** — ✅ **COMPLETE**
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

5. **P0 — Git commit graph** — ✅ **COMPLETE**
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

   
   Hoàn tất commit graph hiện tại:
   - DAG branch/merge bằng SVG, hiển thị HEAD/local/remote/tag refs;
   - search và filter author/date/message/path;
   - click commit xem changed files, merge commit chọn parent làm baseline;
   - visual diff commit↔parent và commit↔HEAD dùng lại generic File Compare, kể cả add/delete/rename;
   - context menu checkout detached, create branch here, cherry-pick, revert, reset soft/mixed/hard, tag, compare with HEAD;
   - mutation theo full SHA + expected SHA; checkout/create-branch-at yêu cầu clean worktree, reset bắt buộc confirmation.

6. **P0 — Git Reflog + Recovery Center** — ✅ **COMPLETE**
   Vì TaskDeck đã hỗ trợ khá nhiều thao tác Git nguy hiểm, nên nên có một lớp cứu hộ:
   - `git reflog`.
   - Restore branch accidentally deleted.
   - Recover detached HEAD.
   - Recover reset commit.
   - Recover lost commit after force-delete branch.
   - Recreate branch from reflog entry.
   - Copy SHA.
   
   Đặc biệt sau khi vừa thêm `Force delete local`, đây là tính năng rất hợp lý.

   Hoàn tất Recovery Center hiện tại:
   - reflog read-only toàn repo, phân loại reset/checkout/commit/merge/rebase/cherry-pick/revert/pull/branch và filter search/ref/kind;
   - tạo recovery branch tại full SHA mà không switch, nên không thay đổi HEAD/Index/Working tree kể cả worktree đang dirty;
   - checkout detached, reset có semantics soft/mixed/hard, tag, compare with HEAD, copy SHA/selector;
   - scan `git fsck --no-reflogs --unreachable` theo yêu cầu để tìm commit không còn branch/tag và thậm chí không còn trong reflog sau force-delete branch;
   - lost commit scan cho Recover branch / Compare HEAD / Copy SHA;
   - regression test chứng minh commit của branch đã force-delete không còn trong reflog vẫn được tìm và phục hồi an toàn.

7. **P1 — Git Worktree Manager** — ✅ **COMPLETE**
   Rất phù hợp với workflow developer:
   - List worktrees.
   - Create worktree from branch.
   - Create branch + worktree.
   - Open TaskDeck project tại worktree mới.
   - Remove/prune worktree.
   - Hiển thị branch đang bị checkout ở worktree nào.
   
   Ví dụ thay vì switch branch làm mất trạng thái hiện tại:
   `main` ở folder A và `fix/foo` ở folder B cùng lúc.

   Hoàn tất Worktree Manager hiện tại:
   - list bằng `git worktree list --porcelain -z`, hiển thị branch/HEAD/current/primary/detached/locked/prunable và worktree nào đang giữ branch;
   - API không leak absolute host path: browser chỉ nhận display path + opaque worktree ID;
   - tạo worktree từ local branch có sẵn, chặn branch đã được checkout ở worktree khác;
   - tạo branch mới + worktree từ HEAD/local/remote/full SHA, có expected-SHA guard;
   - target create chỉ là một sibling directory name đã validate, không nhận arbitrary filesystem path từ browser;
   - remove dùng opaque ID resolve lại server-side, không dùng `--force`; prune stale bắt buộc confirmation;
   - Open TaskDeck resolve opaque ID server-side rồi launcher chạy TaskDeck với workspace thật; response không trả absolute path;
   - shared-server vẫn read-only theo policy Git mutation hiện tại; list worktree dùng quyền `git.status`.

8. **P1 — Git interactive rebase** — ✅ **COMPLETE**
   - Chọn range commit.
   - Pick / Reword / Edit / Squash / Fixup / Drop.
   - Drag reorder.
   - Preview resulting history.
   - Abort/continue khi conflict.
   
   Nếu làm được phần này, Git Panel gần như thay thế được nhiều Git GUI desktop phổ biến.

   Hoàn tất interactive rebase hiện tại:
   - chọn base branch/full SHA, verify base là ancestor của HEAD và lấy range theo thứ tự cũ→mới;
   - Pick / Reword / Edit / Squash / Fixup / Drop, drag reorder và Projected history preview;
   - structured plan chỉ chứa SHA/action/message, không có raw command/argv từ browser;
   - backend re-read toàn bộ range, khóa expected branch/base/HEAD/full SHA, chặn dirty worktree, stale plan, duplicate/missing commit, merge-containing range và range quá lớn;
   - TaskDeck binary làm sequence/message editor nội bộ cho Git; Reword map theo original SHA;
   - integration tests chạy rebase thật cho reorder/reword/squash/fixup/drop, Edit pause→Continue, conflict→Abort và stale/dirty rejection;
   - continue/skip/abort dùng lại Recovery Center hiện có và cleanup editor state đúng khi operation kết thúc.

9. **P1 — Git reset/revert rõ semantics** — ✅ **COMPLETE**
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

   Hoàn tất semantics wizard hiện tại:
   - wizard riêng cho Reset soft/mixed/hard, Revert commit, Restore file from commit và Restore staged only;
   - preview snapshot hiển thị target/current HEAD, staged/unstaged/untracked/conflicted và changed files;
   - giải thích rõ tác động tới HEAD / Index / Working tree trước khi confirm;
   - Visual diff tái sử dụng generic File Compare;
   - mutations dùng full target SHA + expected target SHA + expected current HEAD guard;
   - restore worktree chỉ đổi Working tree; restore staged chỉ đổi Index; unsafe path và missing confirmation bị chặn;
   - reset/revert từ chối stale HEAD và reset từ chối active Git operation;
   - UI không phát raw Git command, chỉ gửi structured operation.

10. **P1 — Git Submodule Manager** — ✅ **COMPLETE**
    - Status.
    - Init/update.
    - Update recursive.
    - Sync.
    - Checkout expected commit.
    - Detect dirty submodule.
    - Open submodule như repository riêng trong Git Panel.
    
    Multi-repo hiện có rồi nên bước này tương đối tự nhiên.

    Hoàn tất Submodule Manager hiện tại:
    - status từ `.gitmodules` + gitlink HEAD, hiển thị expected SHA / actual SHA / initialized / dirty / mismatch;
    - Init, Update, Update recursive, Sync và Checkout expected commit;
    - mọi mutation dùng opaque `submodule_id`, resolve lại server-side; browser không gửi arbitrary filesystem path;
    - expected-SHA guard, missing-gitlink guard, dirty-worktree guard và explicit confirmation;
    - network/update operations chạy qua cancellable Git jobs;
    - initialized submodule luôn được discovery như repository riêng kể cả nested repository scan đang tắt hoặc scan depth thấp;
    - Open repo rescan multi-repo và chuyển Git Panel sang repository con;
    - integration tests local-only bao phủ clean/dirty/mismatch/uninitialized, init/update/recursive/sync, stale SHA, unknown ID và dirty Init bypass.

11. **P1 — Project snapshots/checkpoints** — ✅ **COMPLETE**
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

    Hoàn tất Project snapshots hiện tại:
    - store project-scoped + user-scoped trong shared-server mode, có giới hạn kích thước/số lượng, normalize path và permission `settings.read/settings.write`;
    - Save / Update / Restore / Delete từ Workspace Snapshots manager, có `Ctrl+Alt+S`;
    - semantic tab identity thay cho runtime session ID để snapshot vẫn restore được sau daemon/session ID thay đổi;
    - khôi phục Editor files/active editor, Explorer expanded folders, Database query tabs, FTP/SFTP profile + local/remote location và Git repository đang chọn;
    - terminal local được tạo lại với CWD, title, broadcast group khi group còn tồn tại và split layout; mapping theo đúng snapshot index nên một terminal restore lỗi không làm lệch split sang terminal khác;
    - global tab order và active tab được dựng lại sau khi các subsystem đã khôi phục;
    - Git branch + HEAD được capture làm context/evidence, nhưng restore chỉ chọn đúng repository và **không tự switch branch**, tránh thay đổi worktree ngoài ý muốn;
    - shared-server audit không ghi snapshot name vào shared metadata.

12. **P1 — Project profiles** — ✅ **COMPLETE**
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
    
    Hoàn tất:
    - profile CRUD được lưu project-local, atomic, quyền `0600`;
    - snapshot biến môi trường thực + nhãn environment profile, không phụ thuộc profile browser còn tồn tại;
    - command-preset set và task set chỉ được chọn/lọc, **không tự chạy** khi apply profile;
    - startup local terminals giữ CWD/title;
    - DB / SSH / FTP-SFTP chỉ lưu reference tới profile hiện hữu, không sao chép secret;
    - default Git repository được chọn lại nhưng không tự thay branch/worktree;
    - shared-server dùng `settings.read/settings.write`; profile data không chứa connection secret;
    - environment snapshot chỉ inject vào local task/terminal, không đẩy local env sang SSH remote.

13. **P1 — Workspace multi-root thực sự** — ✅ **COMPLETE**
    - attach / rename / detach nhiều root folder với virtual root IDs đã validate;
    - mỗi root có task/config riêng; Task Editor đọc/ghi theo đúng workspace root;
    - Explorer hiển thị primary + attached roots, hỗ trợ mutation/file actions theo root;
    - project search/replace chạy across roots hoặc scope theo root;
    - Git repository discovery/map chạy trên toàn bộ workspace roots;
    - terminal có thể mở trực tiếp tại root/folder được chọn;
    - attached-root paths không leak arbitrary host path qua browser; backend resolve virtual path server-side;
    - integration/regression tests bao phủ workspace-root CRUD, search, Git discovery, terminal và task routing.
    
    Ví dụ M3:
    `m3-client`, `m3-server`, tools, deploy scripts có thể là các root độc lập trong cùng TaskDeck workspace.

14. **P1 — File editor mạnh hơn** — ✅ **COMPLETE**
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

    Hoàn tất:
    - Go to line, Find/Replace trong file với Next/Previous/wrap, Match case, Replace/Replace All và shortcut Ctrl/Cmd+F/H;
    - bracket matching nhẹ cho `() [] {}`, highlight current line và whitespace toggle;
    - minimap canvas optional, click-to-jump, viewport indicator, preference persist và redraw bằng requestAnimationFrame;
    - detect EOL/encoding/BOM, đổi UTF-8 ↔ UTF-8 BOM và LF ↔ CRLF khi Save;
    - large-file mode read-only, bounded byte reader + hex viewer, file/tab read-only guard;
    - auto-save configurable, dirty indicator trên tab và reopen closed tab;
    - local file history giữ tối đa 12 bản trước TaskDeck editor Save cho mỗi virtual project path, dữ liệu/index private `0600`, directory `0700`, checksum verify + retention;
    - History UI hỗ trợ Compare và Restore; Restore vẫn đi qua editor PUT + `expected_sha256` nên chặn external-change race và tự lưu bản disk hiện tại vào history trước khi thay;
    - shared-server chỉ cho đọc history với `files.read`; history không lưu vào browser localStorage.

15. **P1 — Symbol navigation nhẹ** — ✅ **COMPLETE**
    Không cần LSP ngay.
    
    Có thể bắt đầu bằng:
    - function/class/method outline;
    - jump to symbol;
    - search symbol trong project;
    - breadcrumb.
    
    Parser heuristic/ctags nếu có sẵn trên host, tránh kéo dependency nặng.

    Hoàn tất:
    - Outline trong file đang mở, parser heuristic nhẹ cho Go, JS/TS, Python, Java/C/C++/C#/Kotlin, PHP, Rust và shell;
    - jump tới symbol + `Ctrl/Cmd+Shift+O`, breadcrumb cập nhật theo vị trí con trỏ và dùng chính buffer hiện tại nên thấy cả thay đổi chưa Save;
    - Project Symbols… trong Files menu, debounce/cancel request, keyboard navigation, mở file + jump đúng line;
    - backend project-wide symbol search bounded: tối đa 25.000 source files / 32 MiB mỗi request, source file tối đa 2 MiB, tối đa 100 kết quả;
    - multi-root trả virtual path `@root/<id>/...`, tôn trọng ignore, không follow symlink và dùng pinned/no-follow reads để tránh symlink race;
    - shared-server yêu cầu `files.read`; không cần LSP, ctags hay package bên thứ ba.

16. **P1 — Terminal workspace nâng cao** — ✅ **COMPLETE**
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
    
    Hoàn tất:
    - Duplicate terminal here clone local shell với CWD hiện tại + launch environment, đi xuyên session broker và giữ ownership/permission trong shared-server;
    - terminal có thể move giữa các split group hiện có; validation chặn target/group không hợp lệ;
    - Project Profile đóng vai trò terminal layout/startup preset: capture danh sách local terminal, CWD/title và split layout rồi recreate đúng mapping khi apply profile;
    - Terminal Command Palette có shortcut Ctrl/Cmd+Shift+P và gom các action terminal/split/search/process phổ biến;
    - Search all terminal scrollback tìm trên nhiều terminal và mở đúng session/kết quả;
    - Broadcast preview yêu cầu confirm cho paste/input nhiều dòng, bracketed paste hoặc payload lớn trước khi fan-out;
    - Broadcast Group có environment riêng dạng `NAME=VALUE`; action **NEW TERMINAL IN GROUP** tạo terminal với group env rồi assign vào group, explicit group env override env profile hiện tại;
    - Explorer đã có **Open terminal here** cho folder hoặc thư mục chứa file đang chọn;
    - terminal đang chạy không bị mutate environment; group environment chỉ áp dụng khi tạo terminal mới, tránh thay đổi process state ngoài ý muốn.

17. **P1 — Terminal session recording** — ✅ **COMPLETE**
    - Record command + output + exit code + duration.
    - Export session Markdown/text.
    - Bookmark output line.
    - Annotate session.
    - Restore command history after daemon restart.
    
    Có thể dùng rất tốt khi debug firmware/server.
    
    Hoàn tất:
    - local Bash shell integration dùng OSC 7/133 để lấy semantic CWD, command boundary và exit status; Commands history lưu command/output/exit code/duration/CWD và hỗ trợ Rerun;
    - history được persist server-side trong file project-local bounded/atomic `.vscode/vscode_tasks_menu.terminal_history.json` với mode `0600`, tối đa 64 session và 200 command/session; command/output/note đều có size cap;
    - browser hydrate persisted history vào đúng state OSC hiện có và auto-save khi command hoàn tất; Clear history có confirmation và cập nhật persistent store;
    - Export Markdown/text dùng dữ liệu recording đã hydrate, escape Markdown fence theo content và kèm exit/duration/CWD/session note/bookmark;
    - mỗi session có persistent **Session note**; mỗi command có thể bookmark một dòng output cụ thể, lưu cả line number + text snapshot;
    - khi terminal được recreate sau daemon/broker restart, backend remap history từ old session ID sang new session ID cùng terminal state restore;
    - shared-server authorize GET theo quyền view của đúng terminal và PUT theo quyền control của đúng terminal; route không rơi vào settings permission project-wide;
    - khi session bị xóa thật khỏi daemon, history tương ứng được cleanup để không tích tụ record stale.

18. **P1 — Unified transfer/sync engine** — ✅ **COMPLETE**
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

    Hoàn tất:
    - Folder Sync hỗ trợ Host hoặc Local browser ↔ FTP/SFTP Remote, luôn đi qua Setup + read-only dry-run trước khi mutate;
    - recursive compare dùng size + Modified time hoặc SHA-256; checksum mode short-circuit khi size khác và hash hai phía khi size bằng nhau;
    - exclude glob `*` / `?` / `**` được áp dụng ngay khi scan và có giới hạn số lượng/độ dài;
    - Sync Profile lưu source/root/path, remote path, compare mode, default direction, exclude và allow-delete theo workspace/profile; không lưu credential;
    - one-way Sync và Mirror tái sử dụng transfer queue/scanner/recovery hiện có;
    - Bidirectional Sync tự copy chỉ các path chỉ có ở một phía; file khác nhau ở cả hai phía được đánh dấu Conflict và không tự chọn bên thắng;
    - Mirror delete mặc định OFF, action bị khóa nếu chưa explicit enable và vẫn cần confirmation riêng trước khi xóa;
    - type mismatch/conflict không bị xóa tự động; Local browser vẫn tôn trọng File System Access permission boundary;
    - regression test khóa contract checksum/exclude/profile/bidirectional/delete safety.

19. **P1 — Checksum / file integrity tools** — ✅ **COMPLETE**
    Context menu file:
    - SHA-256.
    - MD5 nếu cần compatibility.
    - Compare checksum.
    - Generate manifest.
    - Verify manifest.
    
    Rất phù hợp với workflow firmware/deploy của bạn.

    Hoàn tất:
    - Project/File context action dùng chung giữa Explorer, editor tab, Git workspace file và Host file pane;
    - SHA-256 và MD5 được tính server-side trên file regular trong workspace; MD5 chỉ dành cho compatibility, không dùng làm security decision;
    - Compare checksum nhận SHA-256 64-hex hoặc MD5 32-hex và báo mismatch rõ ràng;
    - folder action tạo manifest chuẩn kiểu `SHA256SUMS`, sort deterministic, tải xuống browser và không tự ghi file vào project;
    - Verify SHA-256 manifest resolve path tương đối theo thư mục chứa manifest, từ chối absolute/traversal/duplicate/invalid entry và trả danh sách lỗi bounded;
    - generate manifest bỏ qua symlink, chỉ hash regular file, giới hạn 20.000 file / 16 GiB tổng / 4 MiB manifest; single-file hash giới hạn 4 GiB;
    - shared-server map GET/POST integrity vào `files.read`, vì generate/verify đều read-only;
    - regression test bao phủ SHA-256/MD5, generate/verify PASS, mismatch detection, authorization và UI integration.

20. **P1 — Archive support** — ✅ **COMPLETE**
    - Preview ZIP/tar.gz without extracting.
    - Extract.
    - Create archive.
    - Add selected files.
    - Download selected as ZIP.
    - Upload and extract remote.
    
    Đặc biệt hữu ích cho Patch Tool và artifact handoff.

    Hoàn tất:
    - preview ZIP / tar.gz / tgz chỉ đọc metadata, không extract; từ chối unsafe path, symlink/special entry và áp giới hạn entry/uncompressed size;
    - create ZIP hoặc tar.gz từ một file/folder hoặc multi-select Project Explorer, output tạo atomic qua temp + rename và không overwrite archive đã tồn tại;
    - extract ZIP/tar.gz chỉ vào **destination mới**, rollback toàn bộ destination nếu lỗi, không overwrite entry và chặn path traversal/symlink/special entry;
    - Project Explorer hỗ trợ **Create archive from selected…** và **Download selected as ZIP**; download stream trực tiếp về browser, không tạo artifact tạm trong project;
    - file/folder context action dùng chung có Preview archive, Extract archive và Create archive;
    - Host pane của File Transfer có **Upload + extract archive on remote…** cho SFTP profile; archive được safe-preflight local trước khi upload rồi extract qua SSH profile liên kết;
    - FTP không expose remote extract vì protocol không có portable remote shell semantics; fail-safe thay vì giả định server hỗ trợ command riêng;
    - shared-server tách permission: preview=read, create/extract=write, download=read+download, remote upload/extract=read+upload+write;
    - resource budget 20.000 entry / 16 GiB được áp trên toàn archive multi-selection, không reset theo từng source;
    - regression test bao phủ ZIP, tar.gz, traversal rejection, no-overwrite, rollback, shared authorization, UI integration và remote command quoting.

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
