import test from 'node:test';
import assert from 'node:assert/strict';
import {chromeIdentity} from './chrome_identity.mjs';

test('standard Chrome User-Agent matches actual Linux Chromium version',()=>{
 const identity=chromeIdentity('156.0.8078.4','linux','x64');
 assert.match(identity.userAgent,/^Mozilla\/5\.0 \(X11; Linux x86_64\)/);
 assert.match(identity.userAgent,/Chrome\/156\.0\.0\.0 Safari\/537\.36$/);
 assert.doesNotMatch(identity.userAgent,/HeadlessChrome|TaskDeck|Playwright/i);
 assert.deepEqual(identity.userAgentMetadata.brands.map(b=>b.brand),
  ['Chromium','Google Chrome','Not A(Brand']);
 assert.equal(identity.userAgentMetadata.fullVersion,'156.0.8078.4');
 assert.equal(identity.userAgentMetadata.platform,'Linux');
});
test('Mac Chrome User-Agent and Client Hints are consistent',()=>{
 const identity=chromeIdentity('156.0.8078.4','darwin','arm64');
 assert.match(identity.userAgent,/Macintosh; Intel Mac OS X 10_15_7/);
 assert.match(identity.userAgent,/Chrome\/156\.0\.0\.0/);
 assert.equal(identity.userAgentMetadata.platform,'macOS');
 assert.equal(identity.userAgentMetadata.architecture,'arm');
});
test('rejects malformed version, rather than inventing a Chrome build',()=>{
 assert.throws(()=>chromeIdentity('HeadlessChrome/156','linux'),/valid browser version/);
 assert.throws(()=>chromeIdentity('','linux'),/valid browser version/);
 assert.throws(()=>chromeIdentity('156.0.8078.4','unknown'),/Unsupported Chromium/);
});
