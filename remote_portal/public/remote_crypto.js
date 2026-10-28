// End-to-end authenticated ephemeral crypto for future TaskDeck remote sessions.
// A Portal ticket only grants rendezvous; this code never grants TaskDeck access.
// The Relay transports these bytes but cannot derive the P-256 shared secret.
(() => {
  'use strict';
  const protocol = 'taskdeck-e2ee-p256-v1';
  const expectedReady = 'taskdeck-e2ee-ready-v1';
  const utf8 = new TextEncoder();
  const hex = b => Array.from(b, x => x.toString(16).padStart(2, '0')).join('');
  const b64 = b => btoa(String.fromCharCode(...b));
  const unbase64 = value => {
    if (typeof value !== 'string' || value.length > 5000) {
      throw new Error('Invalid encoded E2EE field');
    }
    const decoded = atob(value);
    return Uint8Array.from(decoded, x => x.charCodeAt(0));
  };
  const concat = (...arrays) => {
    const out = new Uint8Array(arrays.reduce((n, a) => n + a.length, 0));
    let cursor = 0;
    for (const a of arrays) {out.set(a, cursor);cursor += a.length;}
    return out;
  };
  const expectedHex = (value, bytes) => typeof value === 'string' &&
    new RegExp('^[a-f0-9]{' + (bytes * 2) + '}$').test(value);
  const requireCrypto = () => {
    if (!globalThis.crypto?.subtle || !globalThis.crypto?.getRandomValues) {
      throw new Error('WebCrypto unavailable; encrypted Remote cannot be used');
    }
    return globalThis.crypto.subtle;
  };

  async function newBrowserSession() {
    const subtle = requireCrypto();
    const keys = await subtle.generateKey({name: 'ECDH', namedCurve: 'P-256'},
      true, ['deriveBits']);
    const publicBytes = new Uint8Array(await subtle.exportKey('raw', keys.publicKey));
    if (publicBytes.length !== 65 || publicBytes[0] !== 4) {
      throw new Error('Invalid P-256 key from browser');
    }
    return {privateKey: keys.privateKey, publicKey: publicBytes,
      publicKeyBase64: b64(publicBytes)};
  }

  async function verifyAgentReady(session, deviceID, probeID, browserNonce, hello) {
    const subtle = requireCrypto();
    if (!session?.privateKey || session.publicKey?.length !== 65 ||
        !expectedHex(deviceID, 32) || !expectedHex(probeID, 16) ||
        !expectedHex(browserNonce, 16) || !hello) {
      throw new Error('Invalid authenticated E2EE session');
    }
    const identity = unbase64(hello.identity);
    const ephemeral = unbase64(hello.ephemeral);
    const signature = unbase64(hello.signature);
    const ciphertext = unbase64(hello.encrypted_ready);
    if (identity.length !== 32 || ephemeral.length !== 65 || ephemeral[0] !== 4 ||
        signature.length !== 64 || ciphertext.length < 9 + 16 ||
        ciphertext.length > 9 + 32768 + 16) {
      throw new Error('Invalid Agent cryptographic handshake');
    }
    const actualID = hex(new Uint8Array(await subtle.digest('SHA-256', identity)));
    if (actualID !== deviceID) {
      throw new Error('Agent cryptographic key does not match paired Device ID');
    }
    const transcript = concat(utf8.encode(protocol), new Uint8Array([0]),
      utf8.encode(deviceID), utf8.encode(probeID), utf8.encode(browserNonce),
      session.publicKey, ephemeral);
    const idKey = await subtle.importKey('raw', identity, {name:'Ed25519'},
      false, ['verify']);
    if (!await subtle.verify({name:'Ed25519'}, idKey, signature, transcript)) {
      throw new Error('Agent did not sign the browser encryption keys');
    }
    const peer = await subtle.importKey('raw', ephemeral,
      {name:'ECDH',namedCurve:'P-256'},false,[]);
    const secret = await subtle.deriveBits({name:'ECDH',public:peer},
      session.privateKey,256);
    const binding = new Uint8Array(await subtle.digest('SHA-256', transcript));
    const hkdf = await subtle.importKey('raw', secret, 'HKDF', false, ['deriveBits']);
    const material = new Uint8Array(await subtle.deriveBits({
      name:'HKDF',hash:'SHA-256',salt:binding,
      info:utf8.encode('taskdeck-e2ee-p256-v1/aes-gcm')
    },hkdf,72*8));
    // Go Agent sends in direction 2 using its 36..68 key and 68..72 IV prefix.
    const receivingKey = await subtle.importKey('raw',material.slice(36,68),
      {name:'AES-GCM'},false,['decrypt']);
    const header = ciphertext.slice(0,9);
    if (header[0] !== 1 || !header.slice(1,8).every(n => n === 0) ||
        header[8] !== 1) {
      throw new Error('Unexpected E2EE sequence number');
    }
    const iv = concat(material.slice(68,72),header.slice(1,9));
    const aad = concat(binding,new Uint8Array([2]),header);
    let plaintext;
    try {
      plaintext = new Uint8Array(await subtle.decrypt({
        name:'AES-GCM',iv,additionalData:aad,tagLength:128
      },receivingKey,ciphertext.slice(9)));
    } catch (_) {
      throw new Error('Encrypted Agent response authentication failed');
    }
    if (new TextDecoder('utf-8',{fatal:true}).decode(plaintext) !== expectedReady) {
      throw new Error('Unexpected authenticated Agent response');
    }
    return true;
  }

  globalThis.TaskDeckRemoteCrypto = Object.freeze({
    newBrowserSession, verifyAgentReady
  });
})();
