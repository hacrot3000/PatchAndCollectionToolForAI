'use strict';
// Security invariant: a compromised PHP origin can replace ALL JavaScript
// served from that origin. E2EE does not protect credentials before encryption.
// Therefore the public Portal MUST NOT render or ship a TaskDeck-password UI.
// This is a deployment-boundary regression, not proof that a hacked PHP host
// cannot add malicious HTML: a separate trusted client is still required.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const publicDir = path.join(__dirname, '../public');
const source = fs.readFileSync(path.join(publicDir,'index.php'),'utf8');
const hostedFiles = fs.readdirSync(publicDir);
assert.ok(!hostedFiles.includes('remote_login.js'),
  'PHP webroot must not ship a credential capture/TaskDeck login script');
const portalCrypto=fs.readFileSync(path.join(publicDir,'remote_crypto.js'),'utf8');
assert.doesNotMatch(portalCrypto, /function sealRemoteLogin|function verifyRemoteLoginResult|sealRemoteLogin,verifyRemoteLoginResult/,
  'PHP-served E2EE diagnostics must not expose password-handling API');
assert.ok(fs.existsSync(path.join(__dirname,'fixtures/remote_login_crypto.js')),
  'interoperability test fixture must remain outside the public document root');
assert.doesNotMatch(source, /remote_login\\.js|remote-login-probe-form|taskdeck_password|taskdeck_username/,
  'PHP-rendered page still contains TaskDeck password entry or a login script');
assert.doesNotMatch(source, /REMOTE_PORTAL_EXPERIMENTAL_LOGIN/,
  'Portal configuration must not re-enable a PHP-supplied password form');
assert.match(source, /không nhập tài khoản hoặc mật khẩu TaskDeck vào trang Portal/,
  'Portal page must warn that PHP-hosted code is not a trusted credential client');
assert.match(source, /remote-trusted-client-ticket/,
  'PHP Portal must offer only public-key-bound tickets for separately installed clients');
assert.match(source, /name="client_nonce"/,
  'trusted-client ticket flow requires a fresh public nonce');
assert.match(source, /name="browser_key"/,
  'trusted-client ticket flow requires a public P-256 key from local client');
assert.match(source, /action="\/api\/remote\/ticket"/,
  'trusted-client ticket flow must use existing authenticated and rate-limited Portal API');
assert.match(source, /remote-probe-button/,
  'password-free signed Agent diagnostics must remain accessible');
assert.match(source, /remote_control_available|Điều khiển Remote chưa mở/,
  'Portal must not imply remote execution is authorized');
console.log('PASS: PHP origin has no served TaskDeck credential UI even with login probe flag');
console.log('PASS: password-free Agent diagnostic remains available, Remote Control disabled');
