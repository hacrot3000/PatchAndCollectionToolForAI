'use strict';
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const nodeCrypto = require('node:crypto');
const assert = require('node:assert/strict');

const source = fs.readFileSync(path.join(__dirname, '../public/remote_probe.js'), 'utf8');
const {publicKey, privateKey} = nodeCrypto.generateKeyPairSync('ed25519');
const rawPublic = publicKey.export({type: 'spki', format: 'der'}).subarray(-32);
const device = nodeCrypto.createHash('sha256').update(rawPublic).digest('hex');

let clickHandler;
let mode = 'valid';
const resultText = {textContent: ''};
const button = {
  disabled: false,
  dataset: {deviceId: device},
  parentElement: {querySelector: () => resultText}
};
const doc = {
  getElementById: name => name === 'remote-probe-config'
    ? {dataset: {relayOrigin: 'https://relay.example.com', csrf: 'test-csrf'}} : null,
  addEventListener: (name, callback) => {
    if (name === 'click') clickHandler = callback;
  }
};

class MockWebSocket {
  constructor(url) {
    assert.equal(url, 'wss://relay.example.com/v1/browser');
    setImmediate(() => this.onopen?.());
  }
  send(body) {
    const request = JSON.parse(body);
    assert.equal(request.ticket, 'signed-portal-ticket');
    assert.match(request.client_nonce, /^[a-f0-9]{32}$/);
    const probeID = 'e'.repeat(32);
    const wire = [
      'taskdeck-browser-attestation-v1',
      device, probeID, request.client_nonce
    ].join('\n');
    const signature = nodeCrypto.sign(null, Buffer.from(wire), privateKey);
    const attestation = {
      probe_id: probeID,
      client_nonce: request.client_nonce,
      public_key: rawPublic.toString('base64'),
      signature: signature.toString('base64')
    };
    if (mode === 'forged-signature') {
      attestation.signature = Buffer.alloc(64).toString('base64');
    }
    if (mode === 'wrong-nonce') {
      attestation.client_nonce = '0'.repeat(32);
    }
    setImmediate(() => this.onmessage?.({data: JSON.stringify({
      ticket_valid: true,
      device_id: device,
      agent_reachable: true,
      remote_control_available: false,
      agent_proof: attestation
    })}));
  }
  close() {}
}
const context = {
  document: doc,
  URL,
  URLSearchParams,
  TextEncoder,
  atob,
  crypto: nodeCrypto.webcrypto,
  fetch: async () => ({
    ok: true,
    json: async () => ({ticket: 'signed-portal-ticket'})
  }),
  WebSocket: MockWebSocket,
  setTimeout,
  clearTimeout
};
vm.runInNewContext(source, context, {filename: 'remote_probe.js'});
assert.equal(typeof clickHandler, 'function', 'Browser should register click handler');

async function runCase(label, expected) {
  mode = label;
  resultText.textContent = '';
  clickHandler({target: {closest: () => button}});
  const deadline = Date.now() + 2500;
  while (Date.now() < deadline) {
    if (!button.disabled && resultText.textContent !== '' &&
        !resultText.textContent.startsWith('Đang')) {
      assert.match(resultText.textContent, expected,
        label + ' incorrectly accepted/rejected');
      process.stdout.write('PASS: ' + label + '\n');
      return;
    }
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  throw new Error('Timed out waiting for browser verification: ' + label +
    ', last status=' + resultText.textContent);
}

(async () => {
  await runCase('valid', /Agent đã phản hồi/);
  await runCase('forged-signature', /Không xác minh được chữ ký Agent/);
  await runCase('wrong-nonce', /Agent không cung cấp bằng chứng danh tính hợp lệ/);
})().catch(error => {console.error(error); process.exitCode = 1;});
