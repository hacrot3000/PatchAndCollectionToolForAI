package server

import "net/http"

func sharedLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content := sharedLoginHTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.URL.Path == "/login.js" {
		content = sharedLoginJS
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(content))
	}
}

const sharedLoginHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sign in · TaskDeck</title><style>
:root{color-scheme:dark;font:16px system-ui,sans-serif;background:#101216;color:#e8eaed}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px}
main{width:min(420px,100%);padding:32px;border:1px solid #30343b;border-radius:14px;background:#171a20}
h1{font-size:25px;margin:8px 0 24px}p{line-height:1.5;color:#afb9c7}label{display:block;margin:18px 0 7px}
input,button{width:100%;padding:12px;border-radius:7px;font:inherit;border:1px solid #48515f}
input{background:#101216;color:inherit}input:focus-visible,button:focus-visible{outline:2px solid #80bcf4;outline-offset:3px}
button{cursor:pointer;margin-top:20px;background:#285680;color:white}button:disabled{opacity:.5;cursor:wait}
.secondary{background:transparent}#message{min-height:24px;overflow-wrap:anywhere;color:#f2cb84}
[hidden]{display:none!important}.brand{color:#8ec6f5;font-weight:700;letter-spacing:.03em}
</style></head><body><main><div class="brand">TaskDeck</div><h1>Shared workspace</h1>
<form id="login-form"><label for="username">Username</label><input id="username" name="username" autocomplete="username" maxlength="128" required autofocus>
<label for="password">Password</label><input id="password" name="password" type="password" autocomplete="current-password" required>
<button id="sign-in" type="submit">Sign in</button></form>
<section id="signed-in" hidden><p id="identity"></p><p>Workspace access is not available yet.</p>
<button id="check-access" type="button" class="secondary">Check access</button><button id="sign-out" type="button">Sign out</button></section>
<p id="message" role="status" aria-live="polite"></p></main><script src="/login.js" defer></script></body></html>`

const sharedLoginJS = `'use strict';
const form=document.getElementById('login-form');
const signedIn=document.getElementById('signed-in');
const message=document.getElementById('message');
const password=document.getElementById('password');
function showUser(user){
  form.hidden=!!user;signedIn.hidden=!user;message.textContent='';
  document.getElementById('identity').textContent=user?'Signed in as '+user.username+' · '+user.project_key:'';
}
async function request(path,options={}){
  const response=await fetch(path,{credentials:'same-origin',cache:'no-store',...options});
  if(!response.ok){const error=new Error((await response.text()).trim()||'Request failed');error.status=response.status;throw error;}
  return response.status===204?null:response.json();
}
async function checkAccess(){
  try{showUser(await request('/api/auth/me'));}
  catch(error){if(error.status===401)showUser(null);else message.textContent=error.message;}
}
form.addEventListener('submit',async event=>{
  event.preventDefault();const button=document.getElementById('sign-in');button.disabled=true;message.textContent='Signing in…';
  try{showUser(await request('/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:document.getElementById('username').value,password:password.value})}));}
  catch(error){message.textContent=error.message;}
  finally{password.value='';button.disabled=false;}
});
document.getElementById('sign-out').addEventListener('click',async event=>{
  const button=event.currentTarget;button.disabled=true;
  try{await request('/api/auth/logout',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});showUser(null);password.value='';document.getElementById('username').focus();}
  catch(error){message.textContent=error.message;}
  finally{button.disabled=false;}
});
document.getElementById('check-access').addEventListener('click',checkAccess);
checkAccess();
`
