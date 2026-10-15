// HTTP 401 browser authentication, independent of HTML login forms.
// Credentials are never returned by semantic snapshots or persisted in TaskDeck.
export function observeHTTPChallenge(session,response){
  if(response.status()!==401||response.request().resourceType()!=='document')return;
  const origin=(()=>{
    try{const u=new URL(response.url());return u.protocol==='https:'?u.origin:'';}catch{return '';}
  })();
  if(!origin)return;
  const generation=session.httpAuthGeneration||0;
  response.headerValue('www-authenticate').then(value=>{
    if((session.httpAuthGeneration||0)!==generation)return;
    if(!/^\s*Basic\b/i.test(String(value||'')))return;
    session.httpChallenge={origin,kind:'basic',message:'HTTP authentication required'};
  }).catch(()=>{});
}
export async function submitHTTPCredentials(session,username,password){
  if(!session.httpChallenge?.origin)throw Object.assign(new Error('No HTTP authentication challenge is active'),{status:409});
  if(typeof username!=='string'||username.length>256||
     typeof password!=='string'||password.length>4096)
    throw Object.assign(new Error('Invalid HTTP authentication credentials'),{status:400});
  const origin=session.httpChallenge.origin;
  // The response origin comes from the actual Chromium 401 challenge; callers
  // cannot choose an unrelated origin and cause credential exfiltration.
  await session.context.setHTTPCredentials({origin,username,password});
  session.httpAuthGeneration=(session.httpAuthGeneration||0)+1;
  session.httpChallenge=null;
}
export function dismissHTTPChallenge(session){
  session.httpAuthGeneration=(session.httpAuthGeneration||0)+1;
  session.httpChallenge=null;
}
