// Launch the Chromium worker from TaskDeck's private managed settings.
// The shared secret is NEVER printed in terminal logs or returned to browsers.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

function defaultConfig(){
  const base=process.platform==='win32'
    ?(process.env.APPDATA||path.join(os.homedir(),'AppData','Roaming'))
    :process.platform==='darwin'
      ?path.join(os.homedir(),'Library','Application Support')
      :(process.env.XDG_CONFIG_HOME||path.join(os.homedir(),'.config'));
  return path.join(base,'taskdeck-semantic-browser','worker.json');
}
const file=process.argv[2]||defaultConfig();
if(!fs.existsSync(file)){
  process.stderr.write('Semantic Browser: worker.json chưa tồn tại tại '+file+'\\n'+
    'Mở TaskDeck → Browser → Setup wizard → Cấu hình để lưu các domain được phép, '+ 
    'hoặc chọn Cài đặt tự động. Không cần tải lại Chromium.\\n');
  process.exit(2);
}
const meta=fs.lstatSync(file);
if(!meta.isFile()||(process.platform!=='win32'&&(meta.mode&0o077)!==0)){
  throw new Error('Worker config must be a private regular file (chmod 600)');
}
const data=JSON.parse(fs.readFileSync(file,'utf8'));
const hosts=Array.isArray(data.allowed_hosts)?data.allowed_hosts.join(','):'';
if(typeof data.socket!=='string'||!path.isAbsolute(data.socket)||
   typeof data.token!=='string'||data.token.length<48||!hosts){
  throw new Error('Worker config incomplete; use Semantic Browser setup wizard');
}
// The worker itself continues to enforce HTTPS, DNS and host allowlist.
process.env.TASKDECK_BROWSER_WORKER_SOCKET=data.socket;
process.env.TASKDECK_BROWSER_WORKER_TOKEN=data.token;
process.env.TASKDECK_BROWSER_ALLOWED_HOSTS=hosts;
await import('./worker.mjs');
