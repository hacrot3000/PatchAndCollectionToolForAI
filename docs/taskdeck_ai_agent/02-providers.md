# F02 — Provider, credentials và capability

Điều hướng: [kế hoạch chung](README.md). Áp dụng F02, F02.1, F02.1.1–F02.1.2, F02.2, F02.2.1–F02.2.2, F02.3, F02.3.1–F02.3.3.

## Hiện trạng

`internal/agentchat/providers.go` chỉ hỗ trợ Responses, xác thực HTTPS và public DNS tại kết nối, từ chối proxy/redirect, timeout; metadata owner-scoped file JSON riêng. `internal/server/agent_chat_providers.go` có GET/PUT và test text đơn giản; key ở `ConnectionSecrets`, `agentProviderSecretID` gắn workspace/owner/provider. PUT ghi key trước metadata nên cần xét lỗi nửa chừng. `web/featuremods/agentchat.js` sửa JSON, nhập key write-only; chưa có xóa hay probe nâng cao. `ModelConfig.ToolCalling` hiện là khai báo, không phải bằng chứng đã probe.

## F02.1.2/F02.2.1 — lifecycle và settings

1. Xác định semantics: xóa metadata và xóa local credential là hai hành vi hiển thị rõ; xóa local không thể tuyên bố đã revoke key phía nhà cung cấp. Quyết định chính sách session đang dùng provider, rollback và retention trước khi code; đang chạy không được âm thầm đổi endpoint/key giữa turn.
2. Bổ sung API delete/revoke-local có request validation, owner, same-origin, `settings.write`, audit không chứa key; cập nhật `shared_authorization.go` cho method/route mới. Tận dụng API xóa hiện có của `internal/secretstore/store.go` sau khi kiểm tra contract; không tạo key trống giả success. Đặt trạng thái trung gian durable hoặc có bù trừ khi xóa/rotate hai store thất bại; báo partial failure chính xác, test restart.
3. `ProviderStore` bổ sung mutation có lock và atomic write, version migration nếu thêm trường; bảo vệ session khỏi dangling reference và owner khác. UI hiển thị add/list/edit/delete, chọn default có scope rõ ràng, rotate/remove key với xác nhận, không bao giờ GET plaintext. Tách effective model lựa chọn theo session; không đổi session immutable ngầm.
4. Test owner A/B, duplicate, stale/race, delete đang chạy, lỗi store từng bước, key không xuất hiện JSON/log; test shared RBAC/CSRF. `go test ./internal/agentchat ./internal/server -run 'TestAgentChat|Test.*Provider' -count=1` và `node --check web/featuremods/agentchat.js`.

## F02.2.2 — probes theo capability

- Phân biệt `unknown`, `verified`, `unsupported`, `inconclusive`; lưu kết quả theo fingerprint endpoint/model/API type/config version, thời điểm, test version; đổi config làm kết quả cũ hết hiệu lực. Capability người dùng khai báo và capability đã chứng minh là hai trường, không quảng cáo execution tool chỉ từ cờ khai báo.
- Probe chỉ theo yêu cầu có consent và cảnh báo chi phí: text; streaming kiểm tra frame/done; tool bằng function vô hại giả lập, tuyệt đối không dispatch patch; previous-response bằng hai lượt có assertion context; Conversations bằng create/reuse nếu endpoint hỗ trợ. Timeout/network/429 là inconclusive, không kết luận unsupported; không gửi AI Pack hay project data.
- Mock HTTP/transport dependency injection vẫn phải kiểm thử public-IP, DNS hỗn hợp, redirect và response size; injection chỉ trong test, không thêm bypass production. Kiểm tra malformed SSE, response ID thiếu, stateful không nhớ. Tự fallback chỉ tại ranh giới an toàn: không replay tool side effect hay request có kết quả bất định.

## F02.3.2/F02.3.3 — adapter

Trích transport dùng chung từ `DoResponses` để streaming giữ nguyên DNS pin, TLS, cấm redirect/proxy, giới hạn bytes/time và cancellation; parser incremental có giới hạn frame/total, tập hợp function arguments theo event rồi chỉ persist kết quả hoàn chỉnh. Phân loại timeout/429/4xx/5xx không lộ secret; backoff chỉ cho probe/read an toàn, không tự retry turn không rõ kết quả. Test fragmented UTF-8/SSE, event lặp, EOF thiếu completion, cancel, tool-call ID. Chỉ xây Chat Completions/Messages khi được duyệt riêng; adapter phải ánh xạ canonical history và không giả định hỗ trợ tool/stateful.

## Trạng thái sau đợt hardening

Đã có: `DELETE /api/agent/providers` (metadata và tùy chọn xóa credential cục bộ, nêu rõ không phải revoke phía vendor, báo partial failure), `POST /api/agent/providers/probe` lưu evidence theo fingerprint endpoint/model/API type/config/bump version, `tool_calling: verified` là điều kiện duy nhất để bật `taskdeck_submit_patch`. Probe lỗi/timeout/429 = `inconclusive`; streaming = `unsupported` vì adapter chưa có. Đã có thêm streaming SSE adapter (`internal/agentchat/stream.go`) với giới hạn event/tổng byte, yêu cầu completion, giới hạn tool-call fragments; chỉ dùng khi probe `streaming: verified` và provider bật `stream`. Chưa có: Chat Completions/Messages, model discovery, pricing/token accounting.

## P0 — Declared tool capability không phải verified capability

`agentChatMessageAPI` hiện lấy `model.ToolCalling` từ cấu hình người dùng và đưa trực tiếp vào `EnablePatchTool`; **không đợi probe**. Vì vậy phần F02.2.2 là việc **chưa hoàn tất**, không phải bảo đảm sẵn có. Phải probe với function vô hại, consent rõ, fingerprint endpoint/model; lỗi timeout/429/5xx là inconclusive chứ không phải unsupported. Basic `/api/agent/providers/test` chỉ chứng minh text Responses, không chứng minh tool calling, Conversations, `previous_response_id` hay streaming. Chặn auto-execution khi capability chưa verified. [Source audit](09-source-audit.md).
