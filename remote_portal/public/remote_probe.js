// Browser-only reachability diagnostic. This does not open a TaskDeck session.
(() => {
  'use strict';
  const config = document.getElementById('remote-probe-config');
  if (!config) return;

  let socketURL;
  try {
    const relay = new URL(config.dataset.relayOrigin);
    if (relay.protocol !== 'https:' || relay.username || relay.password ||
        relay.pathname !== '/' || relay.search || relay.hash) return;
    socketURL = 'wss://' + relay.host + '/v1/browser';
  } catch (_) {
    return;
  }

  async function checkAgent(button) {
    const output = button.parentElement.querySelector('.remote-probe-result');
    if (!output || button.disabled) return;
    const device = button.dataset.deviceId;
    if (!/^[a-f0-9]{64}$/.test(device || '')) return;
    button.disabled = true;
    output.textContent = 'Đang xác minh phiên và kiểm tra kết nối Agent…';
    try {
      const response = await fetch('/api/remote/ticket', {
        method: 'POST',
        credentials: 'same-origin',
        cache: 'no-store',
        headers: {'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8'},
        body: new URLSearchParams({
          device_id: device,
          csrf: config.dataset.csrf || ''
        })
      });
      if (!response.ok) throw new Error('Portal từ chối cấp vé (' + response.status + ')');
      const issued = await response.json();
      if (typeof issued.ticket !== 'string' || issued.ticket.length > 4096) {
        throw new Error('Định dạng vé kết nối không hợp lệ');
      }
      // The short-lived ticket exists only in this function's memory.
      const verdict = await new Promise((resolve, reject) => {
        const ws = new WebSocket(socketURL);
        let finished = false;
        const timer = setTimeout(() => fail('Relay không phản hồi trong 32 giây'), 32000);
        function finish() {
          if (finished) return false;
          finished = true;
          clearTimeout(timer);
          ws.close();
          return true;
        }
        function fail(msg) {
          if (finish()) reject(new Error(msg));
        }
        ws.onopen = () => ws.send(issued.ticket);
        ws.onerror = () => fail('Kết nối TLS/WebSocket tới Relay thất bại');
        ws.onclose = () => fail('Relay đóng kết nối trước khi xác minh xong');
        ws.onmessage = event => {
          try {
            const result = JSON.parse(event.data);
            if (!result || result.ticket_valid !== true ||
                result.remote_control_available !== false) {
              fail('Phản hồi Relay không hợp lệ');
              return;
            }
            if (finish()) resolve(result.agent_reachable === true);
          } catch (_) {
            fail('Không giải mã được phản hồi Relay');
          }
        };
      });
      output.textContent = verdict
        ? 'Agent đã phản hồi qua Relay. Điều khiển Remote chưa được kích hoạt.'
        : 'Không xác nhận được phản hồi của Agent.';
    } catch (error) {
      output.textContent = error && error.message
        ? error.message
        : 'Không thể kiểm tra kết nối.';
    } finally {
      button.disabled = false;
    }
  }

  document.addEventListener('click', event => {
    const button = event.target.closest('.remote-probe-button');
    if (button) void checkAgent(button);
  });
})();
