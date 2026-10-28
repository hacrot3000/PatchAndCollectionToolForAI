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

  const hex = bytes => Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
  const base64Bytes = value => Uint8Array.from(atob(value), c => c.charCodeAt(0));

  async function verifyAgentProof(proof, device, clientNonce) {
    // A Relay response alone is not evidence of the Agent's identity.
    // The browser verifies the pinned Ed25519 fingerprint and fresh nonce.
    if (!proof || !/^[a-f0-9]{32}$/.test(proof.probe_id || '') ||
        proof.client_nonce !== clientNonce || !crypto.subtle) {
      throw new Error('Agent không cung cấp bằng chứng danh tính hợp lệ.');
    }
    const publicKey = base64Bytes(proof.public_key);
    const signature = base64Bytes(proof.signature);
    if (publicKey.length !== 32 || signature.length !== 64) {
      throw new Error('Khóa hoặc chữ ký Agent không hợp lệ.');
    }
    const fingerprint = hex(new Uint8Array(await crypto.subtle.digest('SHA-256', publicKey)));
    if (fingerprint !== device) {
      throw new Error('Danh tính Agent không trùng Device ID đã ghép.');
    }
    const signedText = ['taskdeck-browser-attestation-v1', device,
      proof.probe_id, clientNonce].join('\n');
    const key = await crypto.subtle.importKey('raw', publicKey,
      {name: 'Ed25519'}, false, ['verify']);
    if (!await crypto.subtle.verify({name: 'Ed25519'}, key,
      signature, new TextEncoder().encode(signedText))) {
      throw new Error('Không xác minh được chữ ký Agent.');
    }
    return true;
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
      // The browser contributes a fresh unpredictable challenge. Relay
      // cannot forge a valid Agent response for this browser's challenge.
      if (!globalThis.crypto || !crypto.getRandomValues || !crypto.subtle) {
        throw new Error('Trình duyệt không hỗ trợ xác minh danh tính thiết bị.');
      }
      const clientNonce = hex(crypto.getRandomValues(new Uint8Array(16)));
      // A fresh browser-only ECDH key is bound by the Agent signature.
      // A missing WebCrypto helper is a hard failure, never plaintext fallback.
      const cryptoSession = await globalThis.TaskDeckRemoteCrypto?.newBrowserSession();
      if (!cryptoSession?.publicKeyBase64) {
        throw new Error('Không thể tạo khóa mã hóa đầu cuối.');
      }
      const verdict = await new Promise((resolve, reject) => {
        const ws = new WebSocket(socketURL);
        let finished = false;
        let verifying = false;
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
        ws.onopen = () => ws.send(JSON.stringify({
          ticket: issued.ticket, client_nonce: clientNonce,
          browser_key: cryptoSession.publicKeyBase64
        }));
        ws.onerror = () => fail('Kết nối TLS/WebSocket tới Relay thất bại');
        ws.onclose = () => {
          if (!verifying) fail('Relay đóng kết nối trước khi xác minh xong');
        };
        ws.onmessage = async event => {
          if (finished || verifying) return;
          verifying = true;
          try {
            const result = JSON.parse(event.data);
            if (!result || result.ticket_valid !== true ||
                result.remote_control_available !== false ||
                result.device_id !== device) {
              throw new Error('Phản hồi Relay không hợp lệ.');
            }
            if (result.agent_reachable !== true) {
              if (finish()) resolve(false);
              return;
            }
            await verifyAgentProof(result.agent_proof, device, clientNonce);
            // Relay can forward these fields but cannot decrypt the ready
            // proof; browser authenticates keys, transcript and AES-GCM tag.
            await globalThis.TaskDeckRemoteCrypto.verifyAgentReady(
              cryptoSession, device, result.agent_proof.probe_id,
              clientNonce, result.agent_proof.crypto
            );
            if (finish()) resolve(true);
          } catch (error) {
            fail(error?.message || 'Không xác minh được danh tính Agent.');
          }
        };
      });
      output.textContent = verdict
        ? 'Agent đã xác thực và trả lời qua kênh mã hóa đầu cuối. Điều khiển Remote chưa được kích hoạt.'
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
