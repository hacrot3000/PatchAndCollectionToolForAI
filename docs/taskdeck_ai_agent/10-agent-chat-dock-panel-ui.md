# F04.UI-D — Agent Chat Dockable Panel (bên phải / phía dưới / auto-hide)

**Loại tài liệu:** Đặc tả UI + hướng dẫn AI Coder triển khai, **chưa phải code đã thực hiện**.  
**Ngày lập:** 2026-11-19. **Branch đích:** main.  
**HEAD khảo sát:** f940b208954ca91b4db3c7c98d2585c891834aab. Trước khi code phải fetch và đọc HEAD mới nhất.  
**Phạm vi:** chỉ bố cục, trạng thái giao diện và trải nghiệm Agent Chat; giữ nguyên logic provider, session, approval và native Patch Tool.

Liên quan: [F04 — Conversation UI](04-conversation-ui.md), [README](README.md), [F05 — Patch feedback](05-patch-feedback.md), [Acceptance](07-acceptance.md).

**Mở rộng chức năng khi triển khai UI dock:** [11 — AI Debug observability](11-agent-chat-debug-observability.md). Icon AI Debug ON/OFF phải nằm trực tiếp trên title của dock ở cả Right/Bottom, kể cả khi hẹp; khi bật có tab Debug để xem cấu hình, lỗi, timeline, console, HTTP, serialized payload, response/SSE và tool evidence. Yêu cầu Debug Safe/Raw/ACL nằm ở tài liệu 11.

## 1. Yêu cầu cần đạt

Thay **modal toàn màn hình với nền tối** hiện tại bằng **panel gắn trong workspace** (kiểu VS Code) có hai vị trí:

1. **Bên phải:** panel dọc sát mép phải khu vực làm việc, song song với editor/terminal; có thanh chia kéo ngang.
2. **Phía dưới:** panel ngang sát đáy khu vực làm việc, dưới nội dung/editor/terminal; có thanh chia kéo dọc.

Có chế độ **Auto Hide riêng cho Agent Chat**. Chế độ này không sử dụng hoặc sửa trạng thái auto-hide của panel trái. Khi thu gọn, Agent Chat **không biến mất hoàn toàn** mà thành một **dải mảnh có icon AI Agent** để click mở lại; dải nằm sát cạnh phải hay cạnh dưới tương ứng với vị trí đã chọn.

**Nút chuyển Right ↔ Bottom nằm ngay trên title/header Agent Chat**, không phải chỉ trong Settings. Các lựa chọn và nút tác vụ hiện có phải được đặt trong **thanh tiêu đề/toolbar của panel**, khi hẹp thì gộp những hành động không đủ chỗ vào **menu ngữ cảnh “⋯”** trên title. Các nút quan trọng liên quan soạn tin vẫn truy cập được.

**Bất biến:** hide, collapse, đổi dock position, đổi workspace hoặc mất focus **không đồng nghĩa Cancel**. Không hủy lượt gọi provider hoặc native Patch đã bắt đầu chỉ vì panel bị thu gọn. Không thay logic thực thi đang chạy.

## 2. Hiện trạng đã đối chiếu từ mã nguồn

- **web/featuremods/agentchat.js:** khởi tạo root có class “agentchat-overlay”, cửa sổ “agentchat-window” với role dialog/aria-modal; CSS dùng position:fixed, inset:0, nền che toàn bộ workspace, nút mở/đóng thông qua openAgentPanel/closeAgentPanel. Header chứa New session, Preview context, Rename, Archive, Providers / API keys, Refresh, Close. Composer chứa + Text file, Auto-send results, Cancel request, Send. Có session list, tabbar, model selector, transcript và settings overlay riêng.
- **web/featuremods/menus.js:** mục “Open AI Agent Chat” gọi public API globalThis.TaskMenuAgentChat.open(); mục provider gọi settings().
- **web/featuremods/activitybar.js:** sidebar trái có class body “task-sidebar-auto-hide”, logic mở/đóng/pointerdown ngoài panel, layout grid, hoạt động khác nhau khi pinned và auto-hide; có event “taskmenu:sidebar-layout”.
- **web/featuremods/sidebar.js:** resizer sidebar trái, storage key theo workspace, can thiệp gridTemplateColumns của main. **Không** sử dụng lại resizer này cho Agent Chat.
- **web/featuremods/split.js:** “#panes” là cây split các **terminal panes**. Agent Chat không phải một terminal và không được thêm thành leaf của split-tree.
- **web/featuremods/next.js:** import các feature module; kiểm tra thứ tự init nếu tách thêm module layout.
- **web/agentchat_bootstrap_test.cjs** và **internal/server/agent_chat_ui_test.go:** hiện có test khởi tạo + contract. Test khởi tạo thật đặc biệt quan trọng do lỗi TDZ ở header vừa được sửa.

Đây là **baseline tại HEAD khảo sát**, không phải tuyên bố code ở HEAD tương lai luôn giữ nguyên. Khi bắt đầu triển khai phải xác minh lại các selector, public API và bố cục host thực tế.

## 3. Hình dung bố cục (wireframe)

### 3.1. Right dock, PINNED

~~~text
┌────────────────────── TaskDeck app header / menus ──────────────────────────┐
│ LEFT sidebar │ Main editor / terminal area │↔│ AI Agent Chat        ▣  ⋯  │
│              │                             │ │ [Right][Bottom][Auto] × │
│ Tasks /      │                             │ │ Session tabs / list      │
│ Explorer     │  terminal / editor          │ │ Provider · Model · Effort│
│              │                             │ │ Transcript               │
│              │                             │ │                         │
│              │                             │ │ Attachments / composer  │
│              │                             │ │ [Cancel] [Send]          │
└────────────────────────────────────────────────────────────────────────────┘
~~~

Ranh giới dock nằm trong **workspace phía phải sidebar trái**. Left sidebar pinned/auto-hide đều tiếp tục hoạt động độc lập.

### 3.2. Bottom dock, PINNED

~~~text
┌───────────────────────── TaskDeck app header / menus ───────────────────────┐
│ LEFT sidebar │ Main editor / terminal area                                  │
│              │                                                               │
│              │                                                               │
│              ├──────────────────── draggable splitter ───────────────────────┤
│              │ AI Agent Chat       [Right][Bottom][Auto] [More ⋯] [Hide]      │
│              │ [Session tabs] [Provider / Model] [Transcript]                │
│              │ [Composer + attachments]                   [Cancel] [Send]    │
└────────────────────────────────────────────────────────────────────────────┘
~~~

Bottom dock **không tự thay thế tab Terminal** và không thay đổi orientation, tab/split ratio hoặc focus ownership của terminal hiện tại.

### 3.3. Thu gọn / Auto Hide

- **Right collapsed:** một **dải dọc mảnh** sát mép phải, có icon AI Agent (và tooltip “Open Agent Chat”), toàn bộ transcript/composer không chiếm chỗ; nút/icon phải có vùng click đủ rộng.
- **Bottom collapsed:** một **dải ngang mảnh** sát cạnh dưới, có icon AI Agent và text ngắn tùy không gian. Click dải/icon mở lại đúng vị trí và chiều rộng/cao trước đó.
- **Auto Hide ON:** sau khi user click icon để mở, panel xuất hiện dạng **flyout** neo vào dải, che trên workspace mà **không resize vùng terminal**. Click ra ngoài sẽ thu về dải, trừ các trường hợp “keep open” ở mục 5.
- **Auto Hide OFF / pinned:** mở panel chiếm phần diện tích trong layout, làm nhỏ workspace bằng splitter. Click ra ngoài **không** thu gọn. Nút Hide thu gọn về dải, icon vẫn luôn sẵn có.
- **Hide ≠ Close session ≠ Cancel**. Không xóa tab, draft, attachment, lịch sử, phiên đang chạy.

## 4. Thành phần giao diện chi tiết

### 4.1. Header/title — yêu cầu bắt buộc

Header cố định ở đầu panel (kể cả khi transcript dài), gồm ba vùng:

1. **Leading:** icon Agent + tiêu đề “AI Agent Chat”; trạng thái nhỏ (Working / Waiting approval / Error) nếu có, không giả lập kết quả.
2. **Center / primary actions:** New session; Preview context; Rename; Archive; Providers / API keys; Refresh; Auto-send results; Add text file; Cancel request khi đang chạy. Hành động dùng đúng handler/permissions hiện có, không copy logic.
3. **Trailing — luôn thấy:** nút đổi **Right / Bottom** (một click chuyển sang vị trí còn lại hoặc hai icon rõ trạng thái); **Auto Hide / Pin**; nút **⋯ More**; **Hide**. Có aria-label và title cho từng icon.

**Send** vẫn phải có nút trực tiếp cạnh ô soạn để thao tác dễ dàng và Enter semantics không đổi; có thể đặt thêm shortcut trong More khi thích hợp, nhưng không tạo hai luồng submit. Chức năng đính kèm và Auto-send phải truy cập được từ header/More; vùng composer vẫn phải hiển thị trạng thái attachment, prompt và lỗi. Nếu vẫn giữ nút bên cạnh composer để thuận tiện thì cả hai phải gọi **cùng action/handler** và đồng bộ disabled/loading.

**Ưu tiên khi thiếu không gian:**
1. Title ngắn gọn + Right/Bottom + Auto Hide/Pin + More + Hide **không bao giờ biến mất**.
2. Nút Cancel đang hoạt động và New session ưu tiên hiển thị hơn các tiện ích còn lại (trong khả năng).
3. Preview, Rename, Archive, Providers, Refresh, Auto-send và Add text file lần lượt chuyển vào More dựa trên **kích thước header thực**.
4. Không chỉ dùng một media query cố định theo viewport: dùng **ResizeObserver** của header/dock với cơ chế đo/trường hiển thị ổn định, tránh resize loop.

**More menu** là popover/menu gắn với nút ⋯ (context menu theo cách gọi trong yêu cầu). Click nút mở, chọn hành động thực thi, Esc/click ngoài đóng. Nút trong menu phản ánh chính xác trạng thái disabled, quyền, auto-send ON/OFF, và tên hành động, không lộ các hành động đã bị cấm. Menu tự định vị không tràn viewport, không bị panel overflow:hidden cắt mất, có keyboard navigation/focus restore. Khi panel đủ rộng, nút trở về toolbar; **không có hai bản nút cùng active trong DOM gây double fire**.

**Không được** đặt các nút dock trong menu ẩn duy nhất; user phải đổi Right/Bottom trực tiếp từ title. Nếu quá hẹp, giữ tối thiểu icon title + dock toggle + Pin + **Debug toggle** + More + Hide. Debug ON/OFF không được chỉ có trong More.

### 4.2. Nội dung panel

Giữ nguyên các nhóm thành phần hiện tại và state:

- Session list và tabbar, session title, selection/rename/archive.
- Provider / Model / Mode / Reasoning Effort.
- Transcript (assistant/user/tool) + patch proposal review + evidence/console + copy.
- Composer, attachments, Send/Cancel, auto-send status.

**Right dock** hẹp: không dành cố định 190px cho session list nếu phần chat bị bóp. Cho phép thu session list thành tab/dropdown “Sessions” hoặc sidebar con nhỏ có nút mở; không để transcript/chữ/textarea bị cắt ngang không thể đọc. Tab dài ellipsis + tooltip; tab và editor phải có min-width:0, overflow:auto phù hợp; mọi nút trong tool card vẫn truy cập được.

**Bottom dock** rộng: có thể bố trí session list bên cạnh transcript hoặc thu như Right tùy chiều cao; transcript cuộn riêng, composer bám đáy, header và tab không bị kéo khỏi màn hình.

**Scrolling:** mở panel/đổi dock **không** làm transcript tự kéo cuối nếu user đang cuộn xem cũ. Khi đang ở gần cuối mới follow output mới; khi đổi từ Right ↔ Bottom giữ trạng thái cuộn và nội dung prompt. Command console cũng giữ hành vi follow-tail hiện có.

### 4.3. Settings / các dialog con

Providers / API keys hiện dùng settingsOverlay riêng. Vẫn có thể dùng **modal riêng cho Settings** vì đó là thao tác phụ; nhưng **Agent Chat chính tuyệt đối không còn modal/backdrop chắn toàn app**. Settings modal phải nằm trên dock, hoạt động trong cả pinned/auto-hide, không làm đóng flyout khi click bên trong, và trả focus về đúng control sau khi đóng.

## 5. Auto-hide độc lập và state machine

### 5.1. State model đề xuất

~~~text
dockPosition: 'right' | 'bottom'
autoHide: boolean        // lựa chọn độc lập sidebar trái
expanded: boolean        // false = chỉ còn dải + icon
rightWidth: number       // chiều rộng pinned/flyout ghi nhớ
bottomHeight: number     // chiều cao pinned/flyout ghi nhớ
overflowOpen: boolean    // trạng thái More menu, không cần persist
~~~

Phân biệt rõ:
- **position** không quyết định trạng thái mở/đóng;
- **autoHide** không đồng nghĩa expanded=false, chỉ thay đổi hành vi khi mở/ra ngoài;
- **expanded=false** không xóa session, event listener hay backend request;
- **pin/unpin** không thay đổi class/setting “task-sidebar-auto-hide”.

Một state transition phải thực hiện qua **một controller** (ví dụ AgentChatDockController): setPosition, setAutoHide, expand, collapse, toggle, restore, applyLayout. Tránh nhiều DOM handlers cùng sửa style trực tiếp.

### 5.2. Quy tắc tương tác

| Tình huống | Kỳ vọng |
| --- | --- |
| Mở Agent Chat từ menu khi đang collapsed | Expand đúng vị trí, giữ session |
| Bấm icon trên dải thu gọn | Expand panel, focus vùng phù hợp |
| Right → Bottom | Di chuyển đúng vị trí ngay; vẫn expanded, giữ draft/tab/scroll |
| Bottom → Right | Tương tự; khôi phục rightWidth |
| Bật Auto Hide khi đang mở | Chuyển từ pinned thành flyout, không hủy request |
| Click ngoài flyout khi autoHide=true | Collapse, giữ dải |
| Click ngoài khi autoHide=false | Không collapse |
| Click trong panel/More/popover/Settings/file picker | Không tự collapse giữa thao tác |
| Bấm Hide trong mọi mode | Chỉ collapse; dải/icon tồn tại |
| Click lại menu “Open AI Agent Chat” | Expand/focus, không tạo duplicate dock |
| Đổi auto-hide sidebar trái | Không đổi Agent dock autoHide/position/expanded |
| Agent auto-hide khi sidebar trái mở | Không ép sidebar trái đóng, không tự toggle nó |
| Esc | Ưu tiên đóng More/Settings; nếu không có và Agent flyout mở thì collapse, không cancel |
| Server request/native Patch đang RUNNING | Collapse không cancel; status khi mở lại lấy theo API |

**Outside-click an toàn:** chỉ auto-collapse khi pointerdown thực sự ở ngoài panel và mọi popover/settings thuộc panel. Không collapse khi drag splitter, di chuyển chuột, text selection, menu mở, confirm dialog, focus file upload, hoặc khi đang thao tác trong panel. Không tự hủy draft. Không cần tự collapse theo timer hoặc hover-out.

### 5.3. Persist và restore

Key riêng cho Agent Chat, ví dụ **vscode-tasks-menu:agent-chat-dock:v1:{workspace}[:user-scope]**; dùng cùng cách xác định project/workspace và shared user hiện có, nhưng **không tái sử dụng** key/sidebar class trái. Lưu position, autoHide, width, height và expanded theo chính sách khôi phục UI. Không lưu API key, transcript, attachments, prompt nhạy cảm trong localStorage. Dữ liệu session đã có backend journal riêng.

- Mặc định lần đầu: **Right**, pinned/autoHide OFF, expanded theo lần mở qua menu (không tự chiếm chỗ trước khi user mở).
- Persist kích thước mỗi hướng độc lập; chuyển Right→Bottom→Right không mất chiều rộng.
- Refresh/reopen/restart: restore preference, không tạo duplicate dock; nếu trạng thái persisted không hợp lệ thì fallback an toàn.
- Nếu workspace đổi, state của workspace A không rò sang B. Nếu dùng single-user vs shared mode, tách scope hợp lý.
- Không làm mount panel khi các host DOM còn chưa sẵn sàng; phải có lifecycle initialize/retry idempotent.

## 6. Dock layout và resize — không phá cấu trúc hiện tại

### 6.1. Ranh giới layout

**Không** chèn Agent Chat vào “#panes” hay “.pane” của terminal split; không chèn vào sidebar trái hay chế độ Patch Panel. Tạo **dock host riêng** ở khu vực workspace, tương thích cấu trúc “main > section” sau khi kiểm tra DOM thật. Main/aside/resizer do sidebar.js quản lý, nên **không ghi đè** main.style.gridTemplateColumns hoặc đổi thứ tự node aside/resizer.

Giải pháp khả thi: một workspace dock layout quản lý **content host hiện có** và **Agent dock host** thành hai khu vực theo Right/Bottom. Nếu phải bọc DOM cũ, chỉ **move node thật**, không clone/innerHTML/recreate terminal; tránh làm mất xterm instance/event handlers. Kiểm tra CSS hiện hành và thứ tự modules trước khi chọn kỹ thuật chính xác.

- **Right PINNED:** content và dock nằm cạnh nhau, có vertical splitter của Agent riêng.
- **Bottom PINNED:** content và dock xếp trên/dưới, horizontal splitter của Agent riêng.
- **COLLAPSED:** content chiếm lại không gian; chỉ giữ dải có icon (khoảng 26–36px theo hướng), không còn vùng trống bằng kích thước panel cũ.
- **Auto-hide flyout:** content không resize, panel phủ cục bộ tại cạnh host và có z-index rõ ràng, **không phủ toàn màn hình** và không che sidebar trái.
- Tôn trọng phần header/status bar và modal đang có; không làm mất khả năng click tab terminal/patch/editor.
- Chủ động báo layout changed / resize cho terminal/editor bằng cơ chế có sẵn (ví dụ fitTerminals nếu đúng API đã xác minh), theo sau resize/dock transition bằng rAF/ResizeObserver. Không tạo resize event loop.

### 6.2. Kéo resize và giới hạn

Đề xuất mặc định desktop (có thể tinh chỉnh sau kiểm thử):
- **Right:** width mặc định 420px; min khoảng 300px; max là min(65vw, diện tích còn lại sau khi workspace content tối thiểu ~320px). Nếu cửa sổ quá hẹp, dùng responsive fallback thay vì ép layout tràn.
- **Bottom:** height mặc định 320px; min khoảng 190px; max khoảng 65% chiều cao vùng workspace, vẫn chừa chỗ hữu ích cho terminal.
- **Collapsed strip:** chiều rộng/cao gợi ý 30–34px, riêng hit target icon tối thiểu ~32px.

Splitter dùng pointer events/pointer capture, cursor đúng chiều, keyboard Arrow/Home/End hợp lý, double-click reset kích thước mặc định, aria-orientation/value và focus-visible. Persist **sau kết thúc drag** (không ghi localStorage liên tục). Xử lý pointercancel/lostpointercapture, viewport resize, zoom và scrollbar, không kéo âm kích thước.

### 6.3. Responsive / mobile / narrow width

Khi workspace không đủ chiều rộng/chiều cao để chia thật:
- Ưu tiên giữ icon/dải mở lại.
- Chuyển sang **temporary flyout** hoặc dock chiếm tỷ lệ hợp lý bên trong workspace, **không** áp đặt kích thước cố định vượt viewport.
- Nếu app có layoutProfile='mobile', tôn trọng hành vi mobile hiện có; có thể dùng single-panel drawer/full-height tạm thời nhưng **không tái sử dụng modal có backdrop chặn workspace desktop**.
- Vẫn có cách đổi Right/Bottom và truy cập More.
- Không để header/control bị overflow ra ngoài viewport.

## 7. Kỹ thuật triển khai được đề xuất

**Không viết lại toàn bộ Agent Chat.** Tách phần layout/state/controller khỏi hội thoại. Tùy cấu trúc thực tế có thể:
1. Dùng **agentchat.js** giữ state hội thoại, provider, API calls, transcript và handlers.
2. Tạo thêm **agentchatdock.js** (hoặc module tương đương nếu cần) chịu trách nhiệm mount dock host, vị trí, resize, auto-hide, strip, header overflow và persisted UI state. Public API của controller phải nhỏ, không truy cập API provider.
3. Sửa **next.js** import theo thứ tự rõ ràng, tránh circular reference và Temporal Dead Zone. Mọi nút phải được tạo trước khi append/use. UI DOM test phải khởi tạo thật module.
4. Duy trì contract **globalThis.TaskMenuAgentChat.open / close / settings / visible** đang được menus.js sử dụng. Định nghĩa lại rõ: open=expand+focus; close=collapse-to-strip (không xóa session); visible=true khi nội dung expanded. Có thể thêm get dockPosition, autoHide, toggleDock nếu cần, **không xóa API cũ**.
5. Không cho nút đóng header xóa root/permanently unmount khi vẫn cần dải khôi phục; settingsOverlay có thể tiếp tục tồn tại riêng.
6. Các event/async callbacks cần kiểm tra sessionId/sequence trước khi render để tránh result từ tab A cập nhật tab B; dock không được làm nặng hoặc reset state dữ liệu.
7. Có thể tạo action registry cho header/menu (id, label, icon, enabled, handler, priority) rồi render vào toolbar hoặc More theo width; **không clone nút DOM mang closure cũ**.
8. Không viết CSS global quá rộng nhắm vào main, body, section, #panes hay .pane có thể phá terminal. Dùng namespace **task-agent-dock-***, các CSS variables riêng và theme light/dark hiện có.
9. Không gán role=dialog/aria-modal=true cho dock chính (dock là non-modal region/complementary); Settings modal có thể giữ role=dialog/aria-modal.
10. Quản lý cleanup ResizeObserver, pointer listeners, document outside handlers; module init chỉ một lần. Tránh event listeners nhân đôi sau refresh/reopen.

**Một lưu ý thực tế:** Cơ chế sidebar trái trong activitybar.js dựa trên body classes/pointerdown capture. Agent cần tự định nghĩa outside-click và popup keep-open, không sử dụng class “task-sidebar-panel-open”, không để các handler hai bên xung đột khi user click qua lại.

## 8. Kiểm thử bắt buộc / acceptance

### 8.1. Unit / DOM runtime

- Test gọi khởi tạo module **thực**, không chỉ node --check hoặc static string. Bắt các lỗi ReferenceError/TDZ; tiếp tục test agentchat_bootstrap_test.cjs.
- Test controller: Right, Bottom, pinned, autoHide, expanded/collapsed và mọi tổ hợp; hai hướng lưu chiều riêng.
- Test header overflow với ResizeObserver/fake widths: rộng đủ thì action hiện; hẹp thì More có đủ action; mọi action chỉ được gọi một lần, disabled giữ nguyên.
- Test strip click mở lại, state restore, invalid persistence fallback; không có duplicate listener/root.
- Test keyboard/focus: Tab/Shift+Tab, Escape precedence, enter/space ở buttons, arrow resize.
- Test settings popup, menu overflow, file picker và splitter drag không làm outside-click auto-collapse.
- Test reparent/remount không làm mất state/terminal DOM object.

### 8.2. Browser smoke thật (Playwright hoặc hệ tương đương nếu khả dụng)

1. Right pinned và left sidebar pinned; resize hai bên độc lập.
2. Right auto-hide + left sidebar auto-hide cùng ON; mở/đóng từng bên không ảnh hưởng bên kia.
3. Bottom pinned, kéo height, terminal/editor còn tương tác và fit đúng.
4. Bottom auto-hide; thu gọn thành dải ngang; click icon khôi phục.
5. Đổi Right→Bottom→Right giữ width/height/tab/draft/scroll.
6. Thu nhỏ viewport: title không clip, More mở/đóng được và không bị che.
7. Session A có draft/attachment, session B có dữ liệu khác; dock transition/hide/refresh không gửi nhầm.
8. Provider request đang chạy: Hide/expand không gọi cancel và không tạo request trùng.
9. Native Patch RUNNING: hide/show không gửi lệnh khác; hiển thị trạng thái authoritative khi mở lại.
10. Settings/modal/more/file picker, Esc/click outside, focus restoration và keyboard.
11. Theme sáng/tối, Chrome/Firefox và mobile fallback.
12. Open AI Agent Chat từ menu cũ vẫn mở panel; provider settings menu vẫn hoạt động.
13. Patch Tool, terminal split/tabs, left Explorer/Tasks, sidebar resizer và các GUI khác không regression.

**Không kết luận “GUI PASS” chỉ vì Go test chứa chuỗi hoặc node --check PASS.** Nếu không thể chạy browser test thì ghi rõ NOT VERIFIED, kèm test tự động đã chạy.

### 8.3. Lệnh kiểm thử gợi ý

~~~bash
cd vscode_tasks_menu_go
node --check web/featuremods/agentchat.js
node web/agentchat_bootstrap_test.cjs
go test -count=1 ./internal/server -run '^TestAgent'
go test -race -count=1 ./internal/agentchat
go vet ./internal/server ./internal/agentchat
node test_ai_smart_terminal.cjs
~~~

Bổ sung test cho module dock mới và cấu hình CI phù hợp với file mới. Không thay UI chỉ dựa vào unit/mock.

## 9. Checklist giao AI Coder

- [ ] Fetch origin/main; đọc các file được dẫn ở mục 2 và CSS/layout thật tại HEAD mới.
- [ ] Chuyển Agent Chat chính từ modal overlay sang dock host non-modal bên trong workspace.
- [ ] Right pinned layout + resizer riêng, không sửa grid sidebar trái.
- [ ] Bottom pinned layout + resizer riêng, không chèn vào terminal split tree.
- [ ] Header: nút chuyển Right/Bottom luôn nhìn thấy; Auto Hide/Pin, More, Hide luôn nhìn thấy.
- [ ] Tái sử dụng **toàn bộ** hành động Agent hiện có trong title/More, không mất tính năng.
- [ ] Responsive overflow context menu, đúng permission/loading/disabled và keyboard.
- [ ] Right/Bottom collapsed thành dải nhỏ có icon, click khôi phục.
- [ ] Auto Hide độc lập sidebar trái; outside-click, Esc, modal/popover keep-open đúng.
- [ ] Preserve session/tab/draft/attachments/scroll, không phát Cancel hoặc tool execution ngoài ý muốn.
- [ ] Persist từng workspace/user và restore kích thước cho mỗi hướng.
- [ ] Theme sáng/tối, mobile/narrow fallback, a11y, scrollbar, editor/terminal fit.
- [ ] Test DOM bootstrap + layout state + browser smoke + regression sidebar/terminal/Patch.
- [ ] Cập nhật README, 04-conversation-ui, handoff/changelog theo chứng cứ thực tế.
- [ ] Commit nhỏ, push **main** không force; kiểm tra CI; gửi commit SHA và test evidence.

## 10. Yêu cầu bàn giao / định nghĩa DONE

**Chỉ DONE khi:** người dùng có thể mở Agent Chat dưới dạng panel ở bên phải hoặc dưới; bấm icon trên title để chuyển ngay; thu gọn thành dải mảnh có icon rồi click mở lại; auto-hide của Agent vận hành độc lập với sidebar trái; mọi action được dùng qua header/More khi hẹp; terminal, Patch Tool và session đang chạy không bị ảnh hưởng.

Phản hồi AI Coder phải liệt kê: HEAD trước/sau, file sửa, ảnh/video hoặc kết quả GUI test nếu chạy được, lệnh test PASS/FAIL, commit SHA/push, các mục NOT VERIFIED. Không gọi code chưa kiểm tra browser là “GUI verified”. Không cần xây F08 scheduler, không thực hiện remote, không chỉnh native Patch backend trừ khi phát hiện regression bắt buộc và có lý do rõ.

**Lệnh giao việc:** “Thực hiện đầy đủ tài liệu này trực tiếp trên main; viết code, test, commit và push. Không dừng ở việc lập prompt hoặc kế hoạch khác.”
