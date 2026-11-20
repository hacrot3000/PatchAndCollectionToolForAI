# F02 — Provider, credentials và capability

Điều hướng: [kế hoạch chung](README.md). Áp dụng F02, F02.1, F02.1.1–F02.1.2, F02.2, F02.2.1–F02.2.2, F02.3, F02.3.1–F02.3.3.

## Hiện trạng

Provider schema supports `responses` and `chat_completions`; metadata is owner-scoped, keys use encrypted `ConnectionSecrets`, and provider API type participates in capability fingerprinting. HTTPS/public-IP DNS pinning, no proxy/redirect, bounded response, write-only key API and local credential deletion are present. Capability probes are stored separately from declared model capabilities; only verified tool support enables the Patch function. The Chat adapter is stateless and does not forward reasoning fields without a vendor-specific contract.

For a trusted endpoint resolved by a VPN or local DNS to a shared-network address, a provider may explicitly set `"trusted_shared_ips": ["100.64.0.19"]` to allow that exact address. This is a deliberate SSRF protection exception: confirm the address and endpoint are trusted before allowing credentials or project context to be sent. Only exact canonical IPv4 addresses in `100.64.0.0/10` are accepted; loopback, other private ranges and unlisted DNS answers remain blocked. Each connection rechecks DNS, TLS verification remains enabled, and changing this list invalidates capability probes. Shared-server users need project-admin permission to save a provider with this exception. Remove the list to restore the default public-IP-only policy.

When Test connection encounters a blocked shared-network IP, Agent provider settings displays the address and offers **Allow this IP**. The button requires confirmation and saves only the IP exception on the selected saved provider; it does not send another provider request. Retry Test connection after saving. Other blocked address ranges cannot be allowed this way.

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

Responses SSE và Chat Completions có parser/adapter riêng, đều dùng chung transport hardening (DNS public-IP pin, no proxy/redirect, TLS/timeout/body bounds). Chat adapter ánh xạ canonical messages/tool IDs/tool results về Chat Completions, parser chặn finish reason không hỗ trợ và ghép tool arguments theo index/call ID; Chat mode stateless, không giả lập `previous_response_id`/Conversations. Tool vẫn đi cùng gateway/approval hiện hành. Không có adapter Anthropic Messages trong scope.

## Trạng thái sau đợt hardening

Trạng thái hiện tại: `DELETE /api/agent/providers` xóa metadata và tùy chọn credential cục bộ (không tuyên bố revoke ở vendor); probe evidence gắn endpoint/model/API type/config fingerprint. Chat Completions adapter (`internal/agentchat/chat_completions.go`) + SSE (`chat_completions_stream.go`) đã thêm, tool capability vẫn phải probe verified và đi qua cùng approval gateway. Stateful refs không được gửi trong Chat mode; generic reasoning options bị tắt vì khác nhau theo vendor. Anthropic Messages vẫn deferred; model discovery/pricing/token accounting vẫn thiếu. Không có provider live smoke.

## P0 — Capability evidence boundary

The code gates `taskdeck_submit_patch` on verified probe evidence for the selected provider/model fingerprint; a declared `ModelConfig.ToolCalling` flag alone does not enable the tool. Chat Completions is stateless and does not claim previous-response/Conversations support. Mock tests cover adapter/capability mapping; no real external provider was used, so provider-live compatibility remains **NOT VERIFIED**. Scrubbing and preview are best-effort, not a guarantee. [Source audit](09-source-audit.md).
