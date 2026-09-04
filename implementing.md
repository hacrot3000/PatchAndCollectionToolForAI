# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.14.2**  
Trạng thái: **WINDOWS LAUNCHER SUPPORT — COMPLETE / STOP**

## Baseline

Baseline kỹ thuật của release này là **v6.14.1**, trong đó robustness audit đã COMPLETE. Release v6.14.2 không thay đổi contract PATCH/COLLECT, không đưa SANDBOX/worktree trở lại và không đổi zero-argument workflow.

## Mục tiêu của phiên này

Bổ sung cách chạy native trên Windows tương tự `./tools/run_python_patches.sh` trên Linux, với tài liệu tối thiểu đủ dùng và không tạo một workflow riêng cho Windows.

## Acceptance / task status

| ID | Hạng mục | Trạng thái | Acceptance |
|---|---|---|---|
| WIN-1 | PowerShell public launcher | **COMPLETE** | Có `tools/run_python_patches.ps1`, PowerShell 5.1+, tự xác định project root, zero-argument route vào cùng dispatcher. |
| WIN-2 | CMD/BAT public launcher | **COMPLETE** | Có `tools/run_python_patches.bat`; gọi PowerShell launcher và giữ exit code; không cần sửa ExecutionPolicy toàn máy. |
| WIN-3 | Python discovery trên Windows | **COMPLETE** | Thử `py -3`, `python`, `python3`; chỉ chấp nhận Python **3.10+**; lỗi có hướng dẫn rõ. |
| WIN-4 | Routing parity | **COMPLETE** | Zero-argument, legacy `collect`, utility command và PATCH route dùng cùng core; PATCH vẫn bị ép `--transaction off` như launcher Linux. |
| WIN-5 | Install/Tool Health/package integrity | **COMPLETE** | `.ps1` + `.bat` là managed runtime, bắt buộc có trong checksum coverage và Tool Health. |
| WIN-6 | Windows user/AI documentation | **COMPLETE** | Cập nhật HTML guide tối thiểu, `PORTABLE_USAGE.md`, `AI_USAGE_CONTRACT.md`, feature docs và package contents. |
| WIN-7 | Regression | **COMPLETE** | Thêm `self_test_windows_launchers_v6_14_2.py`; PowerShell native parse/smoke chạy tự động nếu test host có PowerShell; full existing regression vẫn bắt buộc PASS. |
| STOP-1 | Dừng sau task | **COMPLETE** | Không tự mở feature khác sau khi hoàn tất. |

## Public commands

Linux / POSIX:

```bash
./tools/run_python_patches.sh
```

Windows CMD (khuyến nghị vì không vướng script ExecutionPolicy):

```bat
tools\run_python_patches.bat
```

Windows PowerShell:

```powershell
.\tools\run_python_patches.ps1
```

Cả ba đều dùng **cùng project root, cùng `patchs/`, cùng dispatcher/runner/collector và cùng contract**.

## Windows behavior cần biết

- Yêu cầu Python **3.10+**. Launcher ưu tiên Python Launcher `py -3`, sau đó `python` / `python3` trên PATH.
- Vì fullscreen selector hiện dựa trên POSIX `termios`, Windows dùng **line selector**: nhập `1`, `1,3-5`, `a`, `d <range>`, `i <index>`, `h`, `q`; khi một item đã được chọn sẵn, Enter xác nhận như bình thường.
- PATCH package có `post_patch.commands[].argv` vẫn phải dùng executable có thật trên máy hiện tại. Một command hard-code `bash`, `sh` hoặc tool chỉ có trên Linux không tự trở thành portable chỉ vì launcher Windows đã tồn tại.
- In-place/SANDBOX removal, PATCH/COLLECT exclusivity, local-history, checksum, recovery evidence và schemas giữ nguyên.

## Release stop condition

**COMPLETE. DỪNG. Không tự bắt đầu task/tính năng tiếp theo. Hỏi người dùng muốn làm gì tiếp.**
