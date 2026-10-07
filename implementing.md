# TaskDeck implementation roadmap

> **Trạng thái hiện tại: COMPLETE.** Toàn bộ các mục trong roadmap dưới đây đã được triển khai trên branch `main`, kèm regression/integration tests phù hợp. Phần mô tả bên dưới được giữ lại như hồ sơ thiết kế và phạm vi đã hoàn tất, không còn là danh sách việc đang chờ triển khai.

Roadmap đã được thực hiện theo thứ tự ưu tiên sau:

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

21. **P1 — Unified “Open With”** — ✅ **COMPLETE**
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

    Hoàn tất:
    - shared Project/File context action giữ **Open** mặc định và bổ sung **Open With…** chooser dùng chung cho Explorer, editor tab, Git workspace file và Host pane;
    - project preview endpoint riêng detect text / Markdown / image / binary trong workspace; image content dùng same-origin project endpoint, không tái sử dụng host-path preview semantics;
    - chooser tự enable/disable Text Editor, Image Preview, Markdown Preview và Diff theo loại file; Hex, Download và terminal directory luôn sẵn sàng;
    - raw project download có endpoint riêng với `files.download` permission và safe workspace path resolution;
    - Open via host application chỉ xuất hiện khi `!app.sharedMode`; backend dùng argv trực tiếp với `xdg-open` / `open` / `rundll32.exe`, không qua shell;
    - host-app endpoint cố ý không map shared-server permission nên shared mode fail-closed ngay cả khi client cố gọi thủ công;
    - regression test bao phủ text/Markdown/image preview, download, traversal rejection, Hex integration, shared authorization và UI action matrix.

22. **P1 — Project Tasks dependency graph** — ✅ **COMPLETE**
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

    Hoàn tất:
    - `dependsOn` theo label và `dependsOrder` = parallel/sequence được parse + validate fail-fast; cycle, self-dependency, missing dependency và duplicate label bị chặn trước launch;
    - DAG tối đa 256 task; shared dependency dùng per-run lock/status file nên chỉ execute một lần dù nhiều nhánh cùng tham chiếu;
    - dependency parallel chạy bằng child jobs + wait; sequence giữ đúng thứ tự khai báo;
    - `taskdeck.workflow.retry` (0–10), `timeoutSeconds` (0–86400), `continueOnError`, `condition=success|failure|always` được compile vào workflow execution;
    - timeout fail rõ nếu host không có command `timeout`, không silently bỏ qua giới hạn;
    - declared outputs tối đa 32 tên an toàn; task emit `::taskdeck-output name=value`, downstream dùng `${output:Task.name}`; undeclared output dependency bị reject;
    - output propagation dùng sentinel-before-quoting để vẫn hoạt động qua shell/process command materialization; runtime files nằm trong temp dir riêng và cleanup bằng trap;
    - dependency inputs `${input:...}` được gom theo topological order lên root task để UI hỏi đủ input trước khi launch;
    - visual tasks editor hỗ trợ Depends on, parallel/sequence, retry, timeout, condition, continue-on-error, outputs và **Graph** preview có roots/edges/missing/cycle warning;
    - workflow vẫn compile thành một `tasks.Execution` và chạy qua session/broker hiện hữu; không tạo scheduler/session pipeline thứ hai;
    - regression test bao phủ graph validation, strict config, input aggregation, parallel execution thật, sequential contract, retry, continue-on-error, failure condition và output propagation.

23. **P1 — Task run history** — ✅ **COMPLETE**
    - Lần chạy gần nhất.
    - Duration.
    - Exit code.
    - Git commit lúc chạy.
    - Environment/profile.
    - Log artifact.
    - Compare two task runs.
    
    Rất hữu ích cho build/OTA/deployment.

    Hoàn tất:
    - durable server-side history giữ tối đa 50 run gần nhất, serialize update bằng mutex và cleanup log artifact khi record bị evict;
    - record PASS/FAIL/STOPPED, exit code, started/ended/duration, CWD, command preview, Git HEAD tại task CWD, target type/profile và Project Profile active;
    - không persist raw environment variables để tránh lưu credential/secret ngoài ý muốn; environment context được biểu diễn bằng target/profile + Project Profile;
    - mỗi run có log artifact private, bounded 1 MiB, truncate có cờ rõ ràng và hỗ trợ view/download;
    - UI HISTORY ưu tiên durable history, có Details, Run again, Download log và chọn đúng hai run để Compare metadata + log side-by-side;
    - chỉ task session đã kết thúc mới được record; active session bị reject;
    - shared-server list/detail/log áp cùng visibility/ownership với task session và route dùng task-history permission mapping hiện có;
    - regression test bao phủ record từ PTY task thật, Git commit, bounded/truncated log, active-session rejection và UI compare/detail contract.

24. **P1 — Unified command palette** — ✅ **COMPLETE**
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

    Hoàn tất:
    - nâng `terminalpalette.js` thành **TaskDeck Unified Command Palette** thay vì tạo overlay thứ hai; `Ctrl/Cmd+Shift+P` chỉ có một owner;
    - giữ `TaskMenuTerminalPalette` làm alias tương thích và bổ sung `TaskMenuCommandPalette`;
    - palette có File Quick Open, Explorer, Search in Files, Symbols, Project Profiles, Workspace Snapshots;
    - mỗi task hiện tại được materialize thành command `Task: <label>` và chạy qua `app.startTask`, nên vẫn dùng input/workflow/session/broker hiện hữu;
    - Git có Open panel + Refresh status; Git module expose open/close action dùng chung;
    - SSH / Database / SFTP/FTP đều có command rõ ràng mở Connections; Settings có command mở menu + Edit tasks.json;
    - toàn bộ terminal actions cũ vẫn có: new/duplicate/search/save/split/move/process tree/rename/clear;
    - command filtering token-based, keyboard Up/Down/Enter/Escape và disabled state tôn trọng shared-server permission;
    - header menu module expose API open/close thay vì palette tự click DOM bằng selector brittle;
    - regression test khóa command matrix, hotkey ownership và alias compatibility.

25. **P1 — Global Quick Open** — ✅ **COMPLETE**
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

    Hoàn tất:
    - nâng chính `quickopen.js` thành **Global Quick Open**, giữ một owner duy nhất cho Ctrl/Cmd+P;
    - project file vẫn dùng `/api/project/files/search` bounded + AbortController/debounce/refresh fallback; không fetch toàn project tree;
    - empty/query view trộn recently opened từ Explorer, editor tabs, generic workspace tabs, terminal/task sessions, database tabs và File Transfer tabs;
    - saved SSH, Database và SFTP/FTP profiles được expose qua registry hiện hữu và mở bằng API module tương ứng;
    - prefix `>` tái sử dụng command registry của Unified Command Palette ngay trong Quick Open;
    - prefix `@` mở Project Symbols với query, `#` mở Project Search với query;
    - prefix `git:` mở Git Graph với search filter cho SHA/ref/commit text; Git module expose `openGraphSearch`;
    - prefix `ssh:` lọc trực tiếp saved SSH profiles và mở profile được chọn;
    - local/global result có keyboard Up/Down/Enter/Escape, disabled command được bỏ qua khi navigation;
    - Database/File Transfer expose read-only profile getters thay vì Quick Open đọc private module state;
    - regression test giữ contract file-search cũ và khóa recent/tab/session/database/connection sources, prefix routing và single-hotkey ownership.

26. **P1 — Unified Activity/Operation Center** — ✅ **COMPLETE**
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

    Hoàn tất:
    - Operation Center là panel tổng hợp, **không tạo scheduler/job engine thứ hai**; mỗi source expose snapshot/control adapter hoặc được đọc từ API hiện hữu;
    - Git background job API hỗ trợ list toàn bộ jobs, newest-first và bounded 100 record; running jobs không bị retention eviction;
    - file-transfer adapter gom Host↔Remote copy/delete/SFTP/FTP queue theo profile, dùng chính `remove_selected / retry_failed / clear_done` semantics;
    - Patch adapter expose foreground + background Patch/COLLECT sessions và Cancel qua session stop API;
    - Task source gom task session đang chạy với durable task-run history; Retry chạy lại task definition hiện tại, Open đưa về active tab hoặc History;
    - self-update expose running/completed/failed state và retry qua workflow self-update hiện hữu;
    - DB streamed import, query export và table export publish operation browser-local quanh chính request/save workflow, không thay đổi DB adapter execution;
    - panel chuẩn hóa đúng 4 nhóm **Queued / Running / Completed / Failed**, poll khi panel visible và có activity-bar launcher + command-palette action;
    - action **Cancel / Retry / Open / Copy error** chỉ hiện khi source thực sự hỗ trợ; Clear completed ẩn completed history mà không chạm operation đang chạy;
    - Git/Task/Patch/Transfer controls vẫn đi qua permission/session ownership/shared-server backend hiện hữu;
    - regression test khóa source matrix, delegated controls, DB tracking, import order và Git job listing/retention.

27. **P2 — Database schema diff** — ✅ **COMPLETE**
    Vì DB module đã khá mạnh:
    - Compare DB A/B.
    - Compare schema only.
    - Generate ALTER migration.
    - Preview before apply.
    - Export structure.
    
    Có thể đặc biệt hữu ích staging ↔ production.

    Hoàn tất:
    - mỗi DB pane có nút **Schema Diff** và Unified Command Palette có `Database: Schema Diff…`;
    - compare hai database session đang mở theo hướng **Source → Target**, chọn catalog/database riêng cho từng bên;
    - snapshot chỉ dùng `list_objects` + `describe_object`, không browse/read table data; bounded tối đa 500 objects và concurrency 4;
    - normalize columns, indexes, foreign keys, SQL definition và adapter-specific info trước khi diff;
    - phân loại object thành changed / missing-target / extra-target / same và hiển thị chi tiết Source/Target side-by-side;
    - export Source/Target structure JSON qua save-location workflow hiện hữu;
    - migration generation yêu cầu hai bên cùng adapter; MySQL và SQLite được hỗ trợ;
    - MySQL sinh CREATE/DROP object khi có definition và `ADD / DROP / MODIFY COLUMN`; index/FK khác biệt được cảnh báo để review thủ công;
    - SQLite chỉ tự sinh DDL an toàn/tương thích rõ ràng; column remove/change cần table rebuild được ghi warning thay vì đoán migration nguy hiểm;
    - migration luôn có preview + Copy SQL + Export SQL trước Apply;
    - destructive migration yêu cầu nhập chính xác `APPLY <target catalog>`; target read-only bị chặn;
    - Apply thực thi tuần tự qua DB session `execute`, dừng ngay khi statement lỗi và publish tiến độ vào Operation Center;
    - regression test khóa schema-only boundary, module load order, launcher, migration safety và SQLite non-guessing behavior.

28. **P2 — DB transaction workspace** — ✅ **COMPLETE**
    Query tab có:
    - Begin transaction.
    - Commit.
    - Rollback.
    - Auto-commit toggle.
    - Show transaction active indicator.
    
    Khi edit dữ liệu trực tiếp, có thể gom nhiều edit rồi commit cùng lúc.

    Hoàn tất:
    - tái sử dụng transaction capability/backend hiện hữu của MySQL/SQLite; không tạo transaction state ở browser riêng;
    - Query toolbar có Begin / Commit / Rollback, badge `TX ACTIVE · pending commit/rollback` và **Auto-commit** toggle;
    - Auto-commit mặc định bật; tắt toggle gọi `BEGIN` thật trên DB session; không cho bật lại khi transaction còn active để tránh commit ngầm;
    - Begin thủ công đồng bộ toggle về manual mode; Commit/Rollback trả UI về Auto-commit;
    - editable Query result và Data Grid đều gửi `mutate_rows` qua cùng session id, vì vậy có thể Apply nhiều nhóm edit rồi Commit một lần;
    - Commit/Rollback bị chặn nếu còn local Data Grid/query-result edit chưa Apply/Revert, tránh hiểu nhầm edit chưa gửi đã nằm trong transaction;
    - Workbench expose pending-grid bridge và refresh toàn bộ Data Grid sau Commit/Rollback; SELECT result đang mở cũng được reload để phản ánh committed/rolled-back state;
    - transaction controls đồng bộ trên mọi Query tab thuộc cùng DB session;
    - regression test khóa Auto-commit semantics, pending-edit guard, Data Grid bridge và transaction refresh.

29. **P2 — DB query explain** — ✅ **COMPLETE**
    MySQL:
    - EXPLAIN.
    - EXPLAIN ANALYZE nếu server hỗ trợ.
    
    SQLite:
    - EXPLAIN QUERY PLAN.
    
    Hiển thị plan dạng tree.

    Hoàn tất:
    - Query toolbar đã có Explain cho MySQL/SQLite và Explain Analyze riêng cho MySQL/MariaDB;
    - sửa lỗi wiring cũ khiến nút Explain Analyze không truyền cờ `analyze` và thực tế chỉ chạy EXPLAIN thường;
    - SQLite dùng `EXPLAIN QUERY PLAN`, normalize các row `id / parent / detail` thành cây;
    - MySQL EXPLAIN thường nhóm plan theo SELECT id/select_type rồi table/access type;
    - MySQL EXPLAIN ANALYZE parse output text `->`/indent thành cây execution plan;
    - Analyze chỉ cho SELECT/WITH vì có thực thi query; nếu server trả syntax/not-supported, UI báo capability rõ ràng và ẩn nút Analyze cho session đó;
    - plan tree giữ Raw plan result dạng JSON để debug adapter/server-specific output;
    - Explain vẫn dùng query cancel capability hiện hữu và không thay đổi normal Run result flow;
    - regression test khóa Analyze wiring thật, SQLite/MySQL tree builders, raw fallback và unsupported-server behavior.

30. **P2 — Connection tunneling graph** — ✅ **COMPLETE**
    Với SSH + DB + SFTP:
    
    Hiển thị:
    `Browser → TaskDeck → SSH host → Tunnel → Database`
    
    Có thể xem:
    - tunnel port;
    - PID;
    - active connection;
    - reconnect.
    
    Khi lỗi `1045`, timeout, SSH drop sẽ dễ chẩn đoán hơn.

    Hoàn tất:
    - SSH tunnel runtime metadata expose PID thật sau process start; Manager trả live metadata thay vì snapshot trước khi process có PID;
    - DB session metadata liên kết chính xác transport + tunnel ID/PID + SSH profile + local endpoint + remote endpoint + started_at;
    - direct DB session ghi rõ transport `direct`; tunneled session lấy metadata từ chính tunnel được mở cho session, không suy đoán theo port;
    - Connections panel có nút **Graph** và Unified Command Palette có `Connections: Tunnel Graph…`;
    - graph tổng hợp DB sessions, SSH profiles và SFTP/FTP profiles; trạng thái active dựa trên session/tab runtime hiện hữu;
    - DB tunnel route hiển thị **Browser → TaskDeck → SSH host → Tunnel → Database**, kèm tunnel port/PID/remote endpoint/session ID;
    - SSH/SFTP/FTP hiển thị route tương ứng và có Check/Open bằng API/module hiện hữu;
    - DB có Reconnect an toàn: đóng session/tunnel cũ rồi mở lại từ profile; cảnh báo nếu còn local pending edits hoặc transaction active;
    - graph poll nhẹ khi visible để phản ánh connection/tunnel thay đổi;
    - graph không render credential material (password/private key/secret ref);
    - regression test khóa graph UI/load order, DB reconnect safety, tunnel PID metadata và DB-session↔tunnel runtime linkage.

31. **P2 — File watcher** — ✅ **COMPLETE**
    - Detect file changed externally.
    - Auto reload nếu tab chưa dirty.
    - Conflict dialog nếu tab đang sửa.
    - Detect generated files.
    
    Cực kỳ quan trọng nếu TaskDeck chạy cùng IDE khác hoặc compiler.

    Hoàn tất:
    - thêm metadata-only read `/api/project/file?meta=1` trả `mtime_ns/size`, không tải content;
    - watcher chỉ theo dõi open editor và root + tối đa 30 Explorer directories đang expanded; không crawl/search toàn workspace;
    - open-file poll 1.5s, directory poll 2.5s và tạm dừng khi browser tab hidden;
    - chỉ khi metadata file đổi mới fetch full file/SHA;
    - editor chưa dirty tự reload bằng `setEditorDocument`;
    - editor dirty tái sử dụng conflict flow hiện hữu Compare / Reload / Overwrite / Cancel; overwrite vẫn optimistic-lock bằng latest SHA;
    - file biến mất ngoài TaskDeck phát event `taskmenu:editor-external-missing` để UI/integration khác có thể phản ứng;
    - Explorer diff directory listing để phát hiện added/removed/generated file và refresh đúng directory thay đổi;
    - phát event `taskmenu:project-directory-changed` chứa `added / removed / generated`;
    - watcher baseline được refresh khi browser quay lại foreground để tránh false conflict sau thời gian tab hidden;
    - regression test khóa bounded polling, metadata-only payload, auto reload/conflict bridge, generated-file detection và module load order.

32. **P2 — Project health/dashboard** — ✅ **COMPLETE**
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

    Hoàn tất:
    - thêm **Project Health Dashboard** với Activity Bar launcher + Unified Command Palette action;
    - Git branch/head/changed/ahead/behind lấy từ `/api/git/status`;
    - running tasks/open terminals lấy trực tiếp runtime `app.views`;
    - failing transfers lấy từ Operation Center; DB sessions lấy từ Database registry; SSH active/saved lấy từ Connection Graph;
    - durable task history được dùng để tìm **last build** theo label/command build/assemble/compile/package; nếu không có build match thì hiển thị rõ **Last task** thay vì gắn nhãn sai;
    - thêm `/api/project/health` cho project disk usage/file/dir count, bounded tối đa 200k entries và bỏ qua `.git`;
    - endpoint health trong shared-server chỉ cần `files.read`;
    - browser dashboard không crawl `/api/project/tree` hay project search để tính disk usage;
    - dashboard auto-refresh 5s chỉ khi panel visible và browser tab foreground;
    - mỗi card có shortcut về subsystem tương ứng: Git, Operation Center, Connections, Tunnel Graph, Task History;
    - regression test khóa source matrix, load order, shared permission và bounded disk scan.

33. **P2 — Permission granularity cho Shared Server** — ✅ **COMPLETE**
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

    Hoàn tất:
    - mở rộng permission registry với `filesystem.delete`, `git.write`, `git.push`, `git.remote.delete`, `git.force_delete`, `db.read/write/schema`, `ssh.use`, `transfer.read/upload/delete`; self-update tiếp tục tách `selfupdate.check/run`;
    - seed/upgrade permission theo hướng conservative: admin có toàn bộ; các role mặc định khác không tự nhận quyền destructive/privileged mới;
    - terminal giữ ownership-aware `terminal.view/control_own/all` + `terminal.create`; remote SSH terminal yêu cầu thêm `ssh.use`;
    - Git route/action gate riêng push/merge-to, remote branch delete và force local branch delete; Git background job status/cancel cũng dùng permission tương ứng;
    - DB session/read operations yêu cầu `db.read`; mutate/transaction/write và import/schema operation được phân tách `db.write` / `db.schema`;
    - SFTP/FTP read/list/download tách khỏi upload/delete; background queue job cũng gate theo chính job kind;
    - project/host trash/delete yêu cầu `filesystem.delete` ngoài write permission; read/write/upload/download vẫn tách riêng;
    - self-update browser preflight tách check/run: user có `selfupdate.check` vẫn kiểm tra được update nhưng không thể install/retry nếu thiếu `selfupdate.run`;
    - UI preflight cho Git/Explorer/Quick Open/Tasks và các action trọng yếu để tránh nút rõ ràng dẫn đến 403; backend vẫn là authority cuối cùng;
    - shared authorization route matrix deny-by-default cho API chưa map và audit authorization-denied với required permission;
    - regression test khóa permission registry/seed, route matrix, action-level Git/DB/transfer/filesystem guards, terminal ownership, self-update UI preflight và conservative defaults.

34. **P2 — Approval workflow cho action nguy hiểm** — ✅ **COMPLETE**
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

    Hoàn tất:
    - thêm approval policy store với mặc định **OFF** và 4 mode `off / confirm / type / admin`;
    - one-time approval grant gắn chặt `action + resource + requester`, consume một lần rồi không reuse được;
    - browser `jsonFetch` nhận challenge 428, mở approval wizard và retry đúng request với `X-TaskDeck-Approval-ID`;
    - `confirm` yêu cầu nhập `CONFIRM`; `type` yêu cầu nhập chính xác resource; `admin` tạo pending request và poll trạng thái approve/reject/expire;
    - gate remote branch delete, force local branch delete, `git reset --hard`, recursive remote directory delete, production DB write/schema và task được đánh dấu deploy;
    - production DB chỉ được xác định bằng profile option rõ ràng (`environment=production/prod` hoặc `production=true`), không suy từ tên profile;
    - approval là **gate thứ hai**; RBAC permission hiện hữu vẫn phải pass trước/sau approval;
    - thêm permission `approvals.view/manage`, conservative upgrade seed và route authorization riêng;
    - thêm manager UI để xem pending/history, approve/reject và cấu hình policy từng action; có Command Palette action;
    - approval store chuyển sang private user config path, không nằm trong project/workspace và không chứa credential;
    - audit ghi policy update, request, resolve và consume;
    - regression test khóa policy defaults, one-time grant, production marker, dangerous action coverage, route permission, UI wizard/manager và conservative permission seed.

35. **P2 — Audit timeline** — ✅ **COMPLETE**
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

    Hoàn tất:
    - nâng Audit Log trong shared admin thành **Audit Timeline** newest-first, giữ permission `audit.view` độc lập với `users.view`;
    - backend enrich actor `username` từ `user_id` mà không yêu cầu UI gọi Users API;
    - filter server-side theo username hoặc user ID, exact action, resource type, resource ID và result;
    - `AuditQuery`/SQLite store thực thi filter trước LIMIT nên pagination/filter không bị sai do lọc ở browser;
    - API trả bounded page tối đa 500 và `next_before` RFC3339Nano cursor; UI mặc định page 100 và **Load older**;
    - timeline card hiển thị timestamp, actor, category, action, resource, result, client IP và raw structured details trong `<details>`;
    - category UI gom Git / Files / Task-Terminal / Database / Transfer / Security / Config-Admin nhưng vẫn giữ action gốc để điều tra;
    - file create/write/delete/upload/archive/search-replace, settings/config, task/terminal/session và approval/auth/admin events tiếp tục dùng audit hooks hiện hữu;
    - `auditConnection` được bridge vào shared audit store, phủ DB session/mutation, SSH và FTP/SFTP operations với metadata profile/session בלבד, không ghi query/password/request payload;
    - Git mutation handler ghi `git.<action>` success/error cho sync, async job start, hunk, ignore, rebase/repair, Merge To và các action chung; không ghi commit message/output vào details;
    - timeline details tiếp tục tuân thủ nguyên tắc không chứa password/token/private key/raw credential;
    - regression test khóa username/resource/result filter, cursor pagination và timeline UI contract.

36. **P2 — Secrets/credentials manager** — ✅ **COMPLETE**
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

    Hoàn tất:
    - unified **Secrets Manager** quản lý secret generic / Git token / deploy / SSH / database / FTP trên encrypted user-private store hiện hữu;
    - API chỉ expose ID/kind/present/reference usage; secret value là write-only, không có plaintext read operation;
    - create / rotate / delete có audit metadata an toàn; secret đang được profile hoặc task tham chiếu không thể bị delete;
    - SSH/DB/FTP tiếp tục chỉ lưu private secret reference trong profile store; public/browser projection chỉ có `has_secret`;
    - thêm `taskdeckSecrets` trong task definition để map `ENV_NAME -> secret_id` cho deploy/Git/custom task mà không ghi plaintext vào project config;
    - `taskdeckSecrets` được parse thành field server-only và bị loại khỏi `Task.Raw`/`/api/tasks`, nên user chỉ có `tasks.view` không thấy secret identifiers;
    - dependency workflow chỉ resolve secrets từ node reachable; cùng ENV name map nhiều secret ID khác nhau fail-closed;
    - server resolve secret ngay trước spawn và inject **sau** browser/profile env overrides; managed secret không thể bị override bởi request env;
    - execution preview/browser JSON/task history/project profile không persist raw secret environment;
    - shared-server tách `secrets.view / secrets.use / secrets.manage`; các permission này conservative, chỉ admin được auto-seed;
    - task có managed secret bắt buộc thêm `secrets.use`; audit `secret.use` chỉ ghi `secret_count`, không ghi secret ID/value;
    - Secrets Manager UI có Create/Rotate/Delete, usage metadata và Command Palette action;
    - regression test khóa write-only API, encrypted identifier listing, referenced-delete guard, private task projection, reachable workflow resolution, environment override precedence và exact permission gate.

37. **P2 — Backup/restore TaskDeck configuration** — ✅ **COMPLETE**
    Export:
    - project settings;
    - command presets;
    - task layout;
    - terminal layouts;
    - profiles;
    - connections không bao gồm secret hoặc optionally encrypted secrets.
    
    Import trên máy khác.

    Hoàn tất:
    - thêm portable config bundle versioned gồm project settings, command presets, task favorites/recent/history, desktop/mobile terminal layouts, Project Profiles và SSH/DB/FTP/SFTP connection profiles;
    - session ID/live split runtime không được export; terminal state được chuẩn hóa thành layout portable;
    - SecretStore plaintext và original secret reference **không bao giờ** nằm trong backup; chỉ giữ metadata `credential_required`;
    - khi restore trên máy cũ, credential local có cùng profile ID được giữ nguyên; trên máy mới profile cần credential được gắn missing managed reference để UI buộc người dùng nhập lại;
    - environment values mặc định không export vì có thể chứa token/password; chỉ include khi user opt-in rõ ràng;
    - Project Profile environment và client Environment Profiles cùng tuân theo cờ opt-in này;
    - UI có Export / Import JSON, preview count, cảnh báo credential/environment, Merge hoặc Replace;
    - restore validate toàn bundle/version/schema trước khi mutation và giới hạn file/request 8 MiB;
    - server restore có rollback về exact local snapshot nếu một bước ghi cấu hình thất bại;
    - shared-server route giữ permission settings read/write hiện hữu; restore có audit event;
    - Unified Command Palette có `Settings: Backup / Restore…`;
    - regression test khóa secret-reference stripping, missing credential behavior trên máy mới, preserve local credential theo profile ID, invalid-bundle no-mutation, UI/load-order và Environment Profile import validation.

38. **P2 — Plugin/add-on API** — ✅ **COMPLETE**
    Khi số module tăng thêm, thay vì tiếp tục hard-code:
    - register panel;
    - register action;
    - register context-menu command;
    - register file preview handler;
    - register background job;
    - permission declaration.
    
    Có thể dùng Go/process protocol đơn giản, không cần npm ecosystem.
    
    Đây là hướng kiến trúc dài hạn, chưa cần làm ngay.

    Hoàn tất:
    - thêm browser registry `TaskMenuAddons` với `registerAction / registerPanel / registerContextMenuCommand / registerFilePreviewHandler / registerBackgroundJob`;
    - Unified Command Palette tự consume registered add-on actions/panels, không cần thêm hard-code cho từng add-on mới;
    - Project file context menu và Open With/preview dùng hook registry động;
    - background add-on job tích hợp Operation Center source generic và Abort/Cancel; Operation Center local callback không còn hard-code riêng source database;
    - process add-on manifest version 1 được load từ **user-private config** `taskdeck/addons/*.json`; workspace/project không thể khai báo executable để tránh untrusted-project RCE;
    - executable bắt buộc absolute regular executable; manifest symlink/oversized/unknown field bị reject;
    - action declaration chỉ được dùng permission key đã tồn tại trong TaskDeck registry; shared-server backend kiểm tra toàn bộ permission trước khi spawn process;
    - process protocol dùng một JSON request stdin / một JSON response stdout, timeout 5 phút, stdout/stderr bounded 2 MiB và request cancel giết process qua context;
    - public `/api/addons` strip command + args; UI render response bằng text/JSON, không trusted HTML;
    - action success/error được audit mà không ghi command/args/output payload;
    - thêm `ADDONS.md` mô tả manifest, process protocol, browser registration API và security model;
    - regression test khóa manifest validation, known-permission declaration, executable privacy, JSON protocol, extension point action refs, browser registry/hooks/load order và generic background operation control.

39. **P3 — LSP integration** — ✅ **COMPLETE**
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

    Hoàn tất:
    - TaskDeck chỉ làm LSP client, không bundle compiler/npm/language server;
    - extension mapping dùng `gopls` (Go), `clangd` (C/C++), `pylsp` (Python), `typescript-language-server --stdio` (JS/TS), tất cả resolve từ host PATH;
    - thêm bounded JSON-RPC/LSP stdio bridge với Content-Length framing, message cap 8 MiB và request lifecycle 15 giây;
    - mỗi request chạy ephemeral LSP lifecycle `initialize → initialized → didOpen → action → shutdown/exit`, tránh daemon/session leak và giữ API browser đơn giản;
    - hỗ trợ diagnostics, hover, definition, references và rename;
    - diagnostics lấy `textDocument/publishDiagnostics`; definition/references hỗ trợ Location và LocationLink;
    - mọi URI/location/workspace edit từ language server được normalize về project-relative path và target thoát workspace bị reject/skip;
    - Rename chỉ trả normalized text edits; browser bắt active/affected editors clean, preflight toàn bộ target, preview trước Apply rồi ghi qua `/api/project/file` với `expected_sha256`;
    - shared-server route cần `files.read`; rename còn bắt `files.write` server-side trước khi spawn language server;
    - Editor tự thêm nút **LSP** cho extension được hỗ trợ, hiển thị availability/server name; Unified Command Palette có `Editor: Language Server…`;
    - Definition/References mở đúng project file và jump tới line/character; Hover/Diagnostics dùng text-only UI, không render trusted Markdown/HTML từ server;
    - regression test khóa server mapping, framing, workspace escape rejection, Location/WorkspaceEdit normalization, diagnostics document scope, editor integration, optimistic rename safety, module load order và permission guard.

40. **P3 — Remote workspace** — ✅ **COMPLETE**
    Cho TaskDeck daemon quản lý project nằm trên SSH host:
    - explorer remote;
    - terminal remote;
    - Git remote-host side;
    - remote editor save;
    - database/tunnel integration.
    
    Đây là bước lớn nhưng khi hoàn thiện sẽ đưa TaskDeck từ “web task manager” thành một dạng lightweight remote development environment.

    Hoàn tất:
    - thêm Remote Workspace profile private theo user config, ghép **SSH profile + SFTP profile cùng SSH + RemoteRoot + linked DB profiles**; validate reference, SFTP-only transport, absolute POSIX root và DB tunnel phải dùng cùng SSH profile;
    - profile CRUD có API + shared-server `settings.read/settings.write`, không chứa/copy credential;
    - Remote Workspace panel mở từ **Connections → Remote** hoặc Unified Command Palette, có profile selector/New/Edit/Delete;
    - remote Explorer duyệt folder theo relative path, folders-first, Up/Go/Refresh và double-click/click mở file;
    - thêm sandboxed `/api/remote-workspace-files` cho list/read/write: browser không gửi absolute remote path, lexical normalize chặn `..`/absolute escape và server canonicalize target bằng remote `realpath` để chặn symlink escape khỏi `RemoteRoot`;
    - read/write text tái sử dụng SFTP/file-transfer primitive hiện hữu, giới hạn editable-file hiện hữu, binary/oversize reject và SHA-256 optimistic locking;
    - remote Save verify SHA sau ghi; external change trả 409 và đi vào conflict flow của Editor thay vì silently overwrite;
    - Editor local hiện hữu được tái sử dụng qua virtual `remote://<workspace>/<path>`, giữ dirty/Save/Ctrl+S/reload/conflict/line-ending/BOM UX; không tạo editor thứ hai;
    - remote editor bị loại khỏi local File Watcher, host LSP, Local History, reopen-local history và project workspace snapshot để path ảo không bị xử lý như file local;
    - Quick Open nhận biết remote editor tab và chỉ activate tab hiện hữu, không gọi local `/api/project/file` với `remote://`;
    - SSH terminal hỗ trợ `remote_cwd`; Remote Workspace có **Terminal here** tại đúng folder Explorer đang duyệt, kể cả root `/`;
    - Git remote-host side có whitelist `status / branches / log / diff / fetch / pull / push` chạy tại `RemoteRoot`, dùng permission `git.*` + `ssh.use`, không nhận raw command từ browser;
    - linked database profiles hiển thị ngay trong workspace panel; DB tunnel profile được validate phải dùng cùng SSH profile và mở qua Database/session+tunnel workflow hiện hữu;
    - shared-server runtime yêu cầu `settings.read + ssh.use`, rồi list/read/write/terminal/Git tiếp tục bắt `files.* / transfer.* / terminal.create / git.*`; remote editor tự read-only khi user thiếu quyền write;
    - regression test khóa path sandbox/root normalization/canonical symlink guard, post-write SHA verification, browser integration, editor optimistic transport, local-only exclusions, launcher/load order và permission preflight.

## Completion note

Toàn bộ roadmap trên đã hoàn tất. Các ưu tiên ban đầu như Project Explorer, Project Search/Replace, Generic Visual Diff/3-way Merge, Git Commit Graph, Git Reflog Recovery, Command Palette, Unified Operation Center, Git Worktree, Project Snapshots và File Watcher hiện đều đã có implementation thực tế trên `main`.

Các thay đổi tiếp theo nên được mở thành roadmap/ticket mới dựa trên nhu cầu sử dụng thực tế hoặc bug report, thay vì tiếp tục coi tài liệu này là backlog đang mở.

## Active follow-up — Syntax-aware visual diff (2026-10-07)

Requested improvement for Git/File Compare. Keep this section updated after every small commit so work can resume safely after interruption.

- [x] Reuse the editor language registry for diff syntax highlighting on both sides.
- [x] Add conservative important/unimportant classification. Default is important; only confirmed comment-only, blank-only, comment-tail-only, or indentation-only changes in non-indentation-sensitive languages may be marked unimportant.
- [x] Add intra-line changed-span highlighting for paired removed/added lines.
- [x] Add display controls: View all, View diff, View diff context, and View unimportant.
- [x] Add structural diff-context expansion (brace/indent-aware with bounded fallback) and preserve hunk copy/apply behavior.
- [x] Add regression coverage and update user-facing documentation.
- [ ] Run/verify CI on the resulting main branch commits.

Checkpoint: `bfa7af5c` adds View all / View diff / View diff context / View unimportant, bounded brace/indent-aware context expansion, hidden-line separators, and filter-safe hunk apply behavior. `4b1277f8` adds regression coverage; `811bb301` and `36a06abb` update user-facing docs. `3854fa51` added fail-safe important/unimportant classification; `ca89d572` added syntax/intra-line highlighting.
