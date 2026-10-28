// Opt-in authentication-only diagnostic; never opens the TaskDeck UI.
// Portal receives only an ephemeral ECDH public key, device ID and CSRF.
// The TaskDeck password is sealed with browser-only WebCrypto before it
// crosses the Relay. No HTTP method, file, terminal or bearer is supported.
(() => {
  'use strict';
  const config=document.getElementById('remote-probe-config');
  if (!config || !globalThis.TaskDeckRemoteCrypto) return;
  let destination;
  try {
    const origin=new URL(config.dataset.relayOrigin);
    if (origin.protocol!=='https:' || origin.username || origin.password ||
        origin.pathname!=='/' || origin.search || origin.hash) return;
    destination='wss://'+origin.host+'/v1/browser';
  } catch (_) {return;}

  const hex=bytes=>Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('');
  const cryptoModule=globalThis.TaskDeckRemoteCrypto;

  async function submitLogin(form) {
    if (form.dataset.running==='1') return;
    const row=form.closest('tr');
    const device=row?.querySelector('.remote-probe-button')?.dataset.deviceId;
    const output=form.querySelector('.remote-login-probe-result');
    const userField=form.elements.namedItem('taskdeck_username');
    const passField=form.elements.namedItem('taskdeck_password');
    const submit=form.querySelector('button[type="submit"]');
    if (!output || !userField || !passField || !submit ||
        !/^[a-f0-9]{64}$/.test(device||'')) return;
    let username=userField.value;
    let password=passField.value;
    // Never retain a real TaskDeck password in the DOM after submission,
    // including on a failed ticket request or a crypto/Relay error.
    passField.value='';
    form.dataset.running='1';
    submit.disabled=true;
    output.textContent='Đang xác minh danh tính Agent và tài khoản TaskDeck…';
    try {
      if (!crypto?.subtle || !crypto.getRandomValues) {
        throw new Error('WebCrypto không khả dụng trên trình duyệt này.');
      }
      const nonce=hex(crypto.getRandomValues(new Uint8Array(16)));
      const browser=await cryptoModule.newBrowserSession();
      if (!browser?.publicKeyBase64) throw new Error('Không tạo được khóa phiên E2EE.');
      const response=await fetch('/api/remote/ticket',{
        method:'POST',credentials:'same-origin',cache:'no-store',
        headers:{'Content-Type':'application/x-www-form-urlencoded;charset=UTF-8'},
        // CRITICAL: username/password MUST NEVER enter this request.
        body:new URLSearchParams({
          device_id:device,client_nonce:nonce,
          browser_key:browser.publicKeyBase64,
          csrf:config.dataset.csrf||''
        })
      });
      if (!response.ok) throw new Error('Portal từ chối cấp vé ('+response.status+').');
      const issued=await response.json();
      if (typeof issued.ticket!=='string' || issued.ticket.length<100 ||
          issued.ticket.length>4096) throw new Error('Vé Portal không hợp lệ.');
      const result=await new Promise((resolve,reject)=>{
        let ended=false,validating=false,phase='proof';
        const ws=new WebSocket(destination);
        const timer=setTimeout(()=>fail('Kết nối Agent hết thời gian.'),75000);
        function finish(){
          if (ended) return false;
          ended=true;clearTimeout(timer);ws.close();return true;
        }
        function fail(msg){if(finish()) reject(new Error(msg));}
        ws.onopen=()=>ws.send(JSON.stringify({
          ticket:issued.ticket,client_nonce:nonce,browser_key:browser.publicKeyBase64
        }));
        ws.onerror=()=>fail('Kết nối WebSocket không thành công.');
        ws.onclose=()=>{if(!ended && !validating) fail('Relay ngắt trước khi hoàn thành xác thực.');};
        ws.onmessage=async event=>{
          if (ended || validating) return;
          validating=true;
          try {
            const message=JSON.parse(event.data);
            if (phase==='proof') {
              if (message?.ticket_valid!==true || message.agent_reachable!==true ||
                  message.device_id!==device || message.remote_control_available!==false ||
                  !message.agent_proof?.crypto) {
                throw new Error('Không thể xác thực Agent.');
              }
              await cryptoModule.verifyAgentReady(browser,device,
                message.agent_proof.probe_id,nonce,message.agent_proof.crypto);
              let ciphertext;
              try {
                ciphertext=await cryptoModule.sealRemoteLogin(browser,username,password);
              } finally {
                username='';password='';
              }
              if (ended) return;
              phase='login';
              validating=false;
              ws.send(JSON.stringify({type:'e2ee_login',ciphertext}));
              return;
            }
            if (phase!=='login' || message?.type!=='e2ee_login_result' ||
                message.remote_control_available!==false ||
                typeof message.ciphertext!=='string') {
              throw new Error('Phản hồi đăng nhập không hợp lệ.');
            }
            const passed=await cryptoModule.verifyRemoteLoginResult(browser,message.ciphertext);
            if (finish()) resolve(passed);
          } catch (error) {
            fail(error?.message||'Không xác minh được kết quả mã hóa.');
          }
        };
      });
      output.textContent=result
        ? 'Xác minh mật khẩu TaskDeck thành công. Chưa mở phiên Remote.'
        : 'Thông tin đăng nhập TaskDeck không hợp lệ. Chưa mở phiên Remote.';
    } catch (error) {
      output.textContent=error?.message||'Không thể xác minh đăng nhập.';
    } finally {
      username='';password='';passField.value='';
      submit.disabled=false;
      delete form.dataset.running;
    }
  }

  document.addEventListener('submit',event=>{
    const form=event.target.closest?.('.remote-login-probe-form');
    if (!form) return;
    event.preventDefault();
    void submitLogin(form);
  });
})();