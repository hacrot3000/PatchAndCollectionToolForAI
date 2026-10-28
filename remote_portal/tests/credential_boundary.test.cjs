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
assert.doesNotMatch(source, /remote_login\\.js|remote-login-probe-form|taskdeck_password|taskdeck_username/,
  'PHP-rendered page still contains TaskDeck password entry or a login script');
assert.doesNotMatch(source, /REMOTE_PORTAL_EXPERIMENTAL_LOGIN/,
  'Portal configuration must not re-enable a PHP-supplied password form');
assert.match(source, /không nhập tài khoản hoặc mật khẩu TaskDeck vào trang Portal/,
  'Portal page must warn that PHP-hosted code is not a trusted credential client');
assert.match(source, /remote-probe-button/,
  'password-free signed Agent diagnostics must remain accessible');
assert.match(source, /remote_control_available|Điều khiển Remote chưa mở/,
  'Portal must not imply remote execution is authorized');
console.log('PASS: PHP origin has no served TaskDeck credential UI even with login probe flag');
console.log('PASS: password-free Agent diagnostic remains available, Remote Control disabled');
