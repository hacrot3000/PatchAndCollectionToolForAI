# TaskDeck improvement roadmap — completion checkpoint

Ngày rà soát: 2026-10-05  
Base trước khi triển khai: `17f9e1af05378154da9410b695c9cabce5967e29`

Mục tiêu của đợt này là cải thiện chiều sâu thao tác, khả năng phục hồi và UX cho Git, Terminal, Database, FTP/SFTP và SSH. Quy tắc triển khai: commit nhỏ, không dùng npm nếu còn giải pháp khác, hạn chế dependency bên thứ ba, giữ compatibility với workflow hiện tại.

## Trạng thái tổng hợp

| # | Nhóm | Trạng thái | Nội dung chính |
|---|---|---|---|
| 1 | Git jobs + partial staging | ✅ Hoàn tất | long-running job, live progress, Cancel, hunk stage/unstage/discard |
| 2 | FTP/SFTP durable queue + resume | ✅ Hoàn tất | journal qua daemon restart, resumable transfer primitives, resume interrupted jobs |
| 3 | Terminal process + shell integration | ✅ Hoàn tất | Interrupt/Terminate/Kill/process tree, OSC 7/133, command history, Copy/Rerun, resource monitor, broadcast paste safety |
| 4 | Database transaction/cancel/explain | ✅ Hoàn tất | cancel không kill adapter, EXPLAIN/ANALYZE, SQLite/MySQL persistent transaction, Begin/Commit/Rollback |
| 5 | Git LFS recovery | ✅ Hoàn tất | preflight, safe unpushed-history migration, exact path/pattern wizard |
| 6 | SSH depth | ✅ Hoàn tất | keepalive UI, L/R/D forwarding, safe host-key recovery |
| 7 | FTPS + folder sync/mirror | ✅ Hoàn tất | verified FTPS, TLS profile controls, dry-run sync/mirror |
| 8 | Utility depth | ✅ Hoàn tất | DB history/snippets/FK navigation, Git blame/history/tags/cherry-pick/revert/branch cleanup, terminal resource monitor |

## 1. Git operation jobs + hunk staging

Hoàn tất:
- Git fetch/pull/push/merge dài chạy qua cancellable job.
- Browser nhận live stdout/stderr/progress thay vì chờ request đồng bộ.
- Cancel tác động process group thay vì chỉ hủy HTTP request.
- Recovery classification vẫn được giữ sau khi job kết thúc.
- Diff có stage/unstage/discard theo hunk với safety check.
- Hunk discard yêu cầu confirmation.

Commits chính:
- `f5bd0dcf` — Add cancellable Git operation jobs
- `ba2be27f` — Stream Git job progress in panel
- `712cf236` — Add safe Git hunk operations
- `5ae4df72` — Add Git hunk controls to diff view

## 2. FTP/SFTP durable queue + resume

Hoàn tất:
- Server-side transfer queue được journal durable qua daemon restart.
- Interrupted upload/download có resumable primitives.
- Resume tiếp tục từ partial state khi transport hỗ trợ.
- Existing queue semantics Pause/Resume/Retry vẫn giữ nguyên.

Commits chính:
- `55275d56` — Persist file transfer queues across restart
- `8b1c856b` — Add resumable FTP and SFTP primitives
- `21f3ca9d` — Resume interrupted file transfers

## 3. Terminal process control + OSC shell integration

Hoàn tất:
- Interrupt / Terminate / Kill theo process group.
- Process tree inspector.
- Local Bash shell integration bằng TaskDeck-generated rc wrapper, không sửa ~/.bashrc.
- Consume OSC 7 để theo dõi semantic CWD.
- Consume OSC 133 để biết prompt/command boundary/exit status.
- Command history UI có CWD, exit code, duration, Copy command, Copy output, Rerun.
- Process resource monitor.
- Broadcast paste safety confirmation cho paste nhiều dòng/lượng lớn.

Commits chính:
- `198d81ee` — Add terminal terminate and process tree controls
- `4717f5c6` — Add terminal process controls UI
- `52d8eb96` — Add terminal OSC shell integration
- `c1be45f0` — Add terminal command history UI
- `537a7e67` — Add terminal process resource monitor
- `798732a2` — Add broadcast paste safety confirmation

## 4. Database transaction + cancel + EXPLAIN

Hoàn tất:
- Adapter protocol hỗ trợ cancel request đang chạy mà không kill toàn adapter.
- Query editor đổi Run thành Cancel khi adapter support.
- Multi-statement dừng đúng statement hiện tại.
- SQLite: EXPLAIN QUERY PLAN.
- MySQL/MariaDB: EXPLAIN và EXPLAIN ANALYZE; ANALYZE bị giới hạn SELECT/WITH.
- SQLite transaction giữ một sqlite3.Connection xuyên Begin → query → Commit/Rollback.
- MySQL transaction giữ một mysql/mariadb CLI process/server connection xuyên Begin → query → Commit/Rollback.
- MySQL cancel trong transaction dùng CONNECTION_ID + KILL QUERY, giữ transaction connection sống.
- Grid/object operations bị gate trong explicit transaction để tránh chạy trên connection khác.
- Session metadata giữ transaction_active để UI restore đúng trạng thái.

Commits chính:
- `4e96b5a9` — Make database requests cancelable
- `4cb094ca` — Add database query cancel and explain
- `f5feaeef` — Add real SQLite transaction sessions
- `bd35171e` — Add database transaction controls
- `e50c4c3a` — Add real MySQL transaction sessions

## 5. Git LFS large-file recovery

Hoàn tất:
- Wizard chỉ hiện LFS option khi `git lfs version` thành công.
- Chỉ migrate khi worktree/index sạch.
- Upstream phải là ancestor của HEAD.
- Mặc định rewrite đúng outgoing/unpushed history với `git lfs migrate import --skip-fetch`.
- Hỗ trợ migrate exact file hoặc một include pattern.
- Sau migrate, TaskDeck quét lại outgoing large blobs để verify.
- Không tự push/force-push remote refs.
- Original HEAD được ghi rõ để recovery qua reflog.

Commits:
- `9f6ecd05` — Add safe Git LFS migration backend
- `518df0d2` — Add Git LFS large-file recovery UI

## 6. SSH keepalive + forwarding + host-key recovery

Hoàn tất:
- UI expose ConnectTimeout, ServerAliveInterval, ServerAliveCountMax.
- Profile hỗ trợ tối đa 16 forwarding rules:
  - Local `-L`
  - Remote `-R`
  - Dynamic SOCKS `-D`
- IPv6 bind/target được format đúng.
- Generic forwarding chỉ áp dụng interactive SSH terminal; SSH probe, SFTP và database tunnel không inherit.
- Changed host key được phân loại riêng.
- TaskDeck dùng `ssh -G` để tìm effective UserKnownHostsFile.
- Dùng `ssh-keygen -F` để inspect matching entry.
- Xóa old known_hosts entry qua `ssh-keygen -R` chỉ sau explicit confirmation.
- Không tự disable StrictHostKeyChecking và không tự trust key mới.

Commits chính:
- `99c80754` — Expose SSH keepalive settings
- `2afd2f32` — Add SSH profile port forwarding backend
- `11868d19` — Add SSH port forwarding profile UI
- `3d10cef0` — Add safe SSH host-key recovery backend
- `ae1d2ca9` — Add SSH host-key recovery UI

## 7. FTPS + folder sync/mirror

Hoàn tất:
- FTP transport hỗ trợ TLS verified modes.
- Profile có FTPS security controls.
- Folder sync/mirror có dry-run trước khi áp dụng.
- Delete/mirror không chạy mù; preview plan trước.

Commits:
- `805cea64` — Add verified FTPS transport backend
- `363a5ddc` — Add FTPS profile security controls
- `e3a2e39f` — Add folder sync mirror dry run

## 8. Utility depth

Database:
- Persist query history + saved snippets.
- Query history/snippets UI.
- Foreign-key metadata.
- Foreign-key navigation UI.

Git:
- File history + blame.
- Tag management.
- Cherry-pick / revert.
- Safe branch cleanup.
- Recovery classifier cho cherry-pick/revert conflicts.

Terminal:
- Process resource monitor.
- Broadcast paste safety.

Commits:
- `adca5981`, `f54ef2a1` — DB history/snippets
- `5e1bce3f`, `2508f2d4` — DB foreign keys/navigation
- `faedb2d3` — Git file history/blame
- `7686c38e` — Git tag management
- `ab618067` — Git cherry-pick/revert
- `5070a02f` — Git branch cleanup
- `6f9558af` — conflict classification
- `537a7e67` — terminal resource monitor
- `798732a2` — broadcast paste safety

## Validation checkpoint

HEAD đã kiểm tra: `798732a2af30742814387c1ac021f03703d25a11`

GitHub Actions:
- Run #1645
- Conclusion: **success**
- Go matrix và production validation đều pass.

Các commit được chia nhỏ xuyên suốt quá trình để giảm rủi ro timeout/mất tiến độ.

## Ngoài phạm vi roadmap đã thống nhất

Các mục sau vẫn có thể làm về sau nhưng không nằm trong 8 bước đã chốt ở đợt này:
- terminal Detached/Running/Exited session manager riêng;
- configurable terminal scrollback limit;
- DB bulk CSV/JSON import wizard;
- DB streaming full-table export;
- FTP/SFTP bandwidth limit và configurable concurrency;
- stronger post-transfer verification policy sau mọi transfer;
- SSH explicit reconnect preserving tab metadata;
- SSH connection multiplexing/ControlMaster.

Các mục này nên được lập thành roadmap mới thay vì coi là phần chưa hoàn tất của đợt hiện tại.
