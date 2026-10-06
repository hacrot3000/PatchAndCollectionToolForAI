package server

import (
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func sharedAdminUIAllowed(principal identity.Principal) bool {
	for _, permission := range []string{
		identity.PermissionUsersView,
		identity.PermissionUsersManage,
		identity.PermissionRolesView,
		identity.PermissionRolesManage,
		identity.PermissionSessionsManage,
		identity.PermissionAuditView,
	} {
		if principal.Allowed(permission) {
			return true
		}
	}
	return false
}

func (s *Server) sharedAdminUI(w http.ResponseWriter, r *http.Request) {
	if !s.Config.SharedServerEnabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	if !sharedAdminUIAllowed(principal) {
		s.appendSharedAudit(r, &principal, nil, "authorization.denied", "admin_ui", r.URL.Path, "denied", nil)
		writePermissionDenied(w)
		return
	}
	switch r.URL.Path {
	case "/admin/access":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(sharedAdminHTML))
		}
	case "/admin/access.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(sharedAdminCSS))
		}
	case "/admin/access.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(sharedAdminJS))
		}
	default:
		http.NotFound(w, r)
	}
}

const sharedAdminHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>TaskDeck · Users & Access</title>
  <link rel="stylesheet" href="/admin/access.css">
</head>
<body>
  <header>
    <a href="/">← TaskDeck</a>
    <strong>Users & Access</strong>
    <span id="admin-identity"></span>
  </header>
  <main>
    <nav aria-label="Administration sections">
      <button type="button" data-view="users">Users</button>
      <button type="button" data-view="roles">Roles</button>
      <button type="button" data-view="sessions">Sessions</button>
      <button type="button" data-view="audit">Audit Log</button>
    </nav>
    <section id="admin-content" aria-live="polite">Loading access data…</section>
  </main>
  <script type="module" src="/admin/access.js"></script>
</body>
</html>`

const sharedAdminCSS = `:root{font-family:system-ui,sans-serif;color-scheme:dark;background:#101216;color:#e8eaed}*{box-sizing:border-box}body{margin:0}header{height:52px;display:flex;gap:14px;align-items:center;padding:0 16px;border-bottom:1px solid #30343b}header a{color:#9fc8f5;text-decoration:none}header strong{font-size:16px}#admin-identity{margin-left:auto;font-size:12px;opacity:.75}main{max-width:1180px;margin:0 auto;padding:20px}nav{display:flex;gap:8px;margin-bottom:18px}button,input,select{font:inherit;background:#252a33;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 10px}button{cursor:pointer}button:disabled{opacity:.45;cursor:default}button.active{background:#29445f;border-color:#47759e}#admin-content{border:1px solid #30343b;border-radius:10px;min-height:240px;padding:18px;background:#15181e}.muted{opacity:.7}.error{color:#ffb4b4}.success{color:#9ee7b0}.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin:0 0 14px}.create-user{display:grid;grid-template-columns:1fr 1fr 1.2fr 1fr auto;gap:8px;margin:12px 0 20px}.table-wrap{overflow:auto;border:1px solid #30343b;border-radius:8px}table{width:100%;border-collapse:collapse;font-size:13px}th,td{padding:10px;border-bottom:1px solid #2a2e35;text-align:left;vertical-align:top}th{background:#1d2128;position:sticky;top:0}tr:last-child td{border-bottom:0}.permissions{max-width:360px;white-space:normal;font-size:11px;opacity:.8}.access-controls,.override-controls{display:flex;gap:7px;align-items:center;flex-wrap:wrap;margin-top:7px;min-width:270px}.role-permission-editor{display:flex;gap:8px;align-items:flex-start;min-width:330px}.role-permission-editor select{min-width:220px}.access-controls select{min-width:125px}.override-controls select{max-width:180px}.access-controls label{display:flex;gap:5px;align-items:center}.access-controls input[type=checkbox]{width:auto}.self-note{font-size:11px;opacity:.65}.audit-filters{display:grid;grid-template-columns:repeat(3,minmax(150px,1fr));gap:8px;margin-bottom:12px}.audit-filters .audit-filter-actions{display:flex;gap:8px;align-items:center;grid-column:1/-1}.audit-timeline{display:flex;flex-direction:column;gap:0;position:relative;margin-top:8px}.audit-timeline:before{content:'';position:absolute;left:18px;top:8px;bottom:8px;width:1px;background:#3a414c}.audit-event{position:relative;margin-left:38px;padding:9px 10px 9px 12px;border-bottom:1px solid #2a2e35}.audit-event:before{content:'';position:absolute;left:-24px;top:14px;width:9px;height:9px;border-radius:50%;background:#677386;border:2px solid #15181e}.audit-event.success:before{background:#56b870}.audit-event.denied:before,.audit-event.error:before,.audit-event.failed:before{background:#db6b74}.audit-event-head{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.audit-event-time{font-size:11px;opacity:.62;min-width:150px}.audit-event-actor{font-weight:700}.audit-event-category{font-size:9px;text-transform:uppercase;letter-spacing:.05em;padding:2px 5px;border:1px solid #3b414d;border-radius:999px;opacity:.75}.audit-event-action{font:11px ui-monospace,monospace}.audit-event-result{margin-left:auto;font-size:9px;font-weight:800;text-transform:uppercase}.audit-event-resource{margin-top:4px;font-size:11px;opacity:.78;overflow-wrap:anywhere}.audit-event-meta{margin-top:4px;font-size:10px;opacity:.55}.audit-event details{margin-top:6px}.audit-event pre{white-space:pre-wrap;overflow-wrap:anywhere;font:10px/1.4 ui-monospace,monospace;background:#0f1217;border:1px solid #2d333c;border-radius:5px;padding:7px;max-height:240px;overflow:auto}.audit-load-more{margin-top:12px}.self-note{font-size:11px;opacity:.65}@media(max-width:900px){.create-user{grid-template-columns:1fr}.access-controls{min-width:230px}.audit-filters{grid-template-columns:1fr}.audit-event-head{align-items:flex-start}.audit-event-result{margin-left:0}}`

const sharedAdminJS = `const content=document.querySelector('#admin-content');
const identity=document.querySelector('#admin-identity');
let currentUser=null;
let permissionSet=new Set();
let currentView='';

function has(permission){return permissionSet.has(permission);}
function canOpen(view){
  if(view==='users')return has('users.view')||has('users.manage');
  if(view==='roles')return has('roles.view');
  if(view==='sessions')return has('sessions.manage');
  if(view==='audit')return has('audit.view');
  return false;
}
function node(tag,text,className){
  const value=document.createElement(tag);
  if(text!=null)value.textContent=text;
  if(className)value.className=className;
  return value;
}
async function api(path,options={}){
  const headers=new Headers(options.headers||{});
  if(options.body!=null&&!headers.has('Content-Type'))headers.set('Content-Type','application/json');
  const response=await fetch(path,{cache:'no-store',...options,headers});
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  if(response.status===204)return null;
  return response.json();
}
function setStatus(host,message,kind='muted'){
  host.textContent=message;
  host.className=kind;
}
function roleSelect(roles,selected){
  const select=document.createElement('select');
  for(const role of roles){
    const option=document.createElement('option');
    option.value=role.id;option.textContent=role.name+(role.system_role?' · system':'');
    option.selected=role.id===selected;
    select.append(option);
  }
  return select;
}
async function renderUsers(){
  currentView='users';
  content.replaceChildren();
  const title=node('h2','Users and project access');
  const status=node('p','Loading…','muted');
  content.append(title,status);
  let users=[],roles=[],permissions=[];
  try{
    if(has('users.view'))users=(await api('/api/admin/users')).users||[];
    if(has('roles.view')){
      roles=(await api('/api/admin/roles')).roles||[];
      permissions=(await api('/api/admin/permissions')).permissions||[];
    }
  }catch(error){setStatus(status,'ERROR: '+error.message,'error');return;}
  status.remove();

  if(has('users.manage')&&roles.length){
    const form=document.createElement('form');form.className='create-user';
    const username=document.createElement('input');username.required=true;username.maxLength=128;username.placeholder='Username';username.autocomplete='off';
    const display=document.createElement('input');display.maxLength=256;display.placeholder='Display name';
    const password=document.createElement('input');password.required=true;password.type='password';password.minLength=12;password.maxLength=4096;password.placeholder='Initial password (12+ chars)';password.autocomplete='new-password';
    const role=roleSelect(roles,'system:viewer');
    const submit=node('button','Create user');submit.type='submit';
    const feedback=node('div','', 'muted');feedback.style.gridColumn='1/-1';
    form.append(username,display,password,role,submit,feedback);
    form.onsubmit=async event=>{
      event.preventDefault();submit.disabled=true;setStatus(feedback,'Creating…');
      try{
        await api('/api/admin/users',{method:'POST',body:JSON.stringify({username:username.value,display_name:display.value,password:password.value,role_id:role.value})});
        password.value='';setStatus(feedback,'User created.','success');
        await renderUsers();
      }catch(error){password.value='';setStatus(feedback,'ERROR: '+error.message,'error');}
      finally{submit.disabled=false;}
    };
    content.append(form);
  }else if(has('users.manage')&&!roles.length){
    content.append(node('p','Role visibility is required before a role can be selected for a new user.','muted'));
  }

  if(!has('users.view')){
    content.append(node('p','You do not have users.view, so existing project members are hidden.','muted'));
    return;
  }
  if(!users.length){
    content.append(node('p','No project users found.','muted'));
    return;
  }
  const wrap=node('div',null,'table-wrap');
  const table=document.createElement('table');
  const head=document.createElement('thead');
  const hr=document.createElement('tr');
  for(const label of ['User','Role','Status','Effective permissions','Overrides','Project access'])hr.append(node('th',label));
  head.append(hr);table.append(head);
  const body=document.createElement('tbody');
  for(const user of users){
    const row=document.createElement('tr');
    const userCell=document.createElement('td');
    userCell.append(node('strong',user.username));
    if(user.display_name)userCell.append(document.createElement('br'),node('span',user.display_name,'muted'));
    row.append(userCell,node('td',user.role_name||user.role_id),node('td',(user.user_enabled?'user enabled':'user disabled')+' · '+(user.member_enabled?'access enabled':'access disabled')));
    row.append(node('td',(user.effective_permissions||[]).join(', '),'permissions'));
    const overrideCell=document.createElement('td');
    const overrideEntries=Object.entries(user.permission_overrides||{}).sort((a,b)=>a[0].localeCompare(b[0]));
    overrideCell.append(node('div',overrideEntries.length?overrideEntries.map(item=>item[0]+'='+item[1]).join(', '):'none','permissions'));
    if(has('users.manage')&&permissions.length&&user.id!==currentUser.user_id){
      const overrideControls=node('div',null,'override-controls');
      const permissionSelect=document.createElement('select');
      for(const permission of permissions){
        const option=document.createElement('option');option.value=permission.key;option.textContent=permission.key;permissionSelect.append(option);
      }
      const effectSelect=document.createElement('select');
      for(const effectValue of ['ALLOW','DENY']){
        const option=document.createElement('option');option.value=effectValue;option.textContent=effectValue;effectSelect.append(option);
      }
      const setButton=node('button','Set');setButton.type='button';
      const clearButton=node('button','Clear');clearButton.type='button';
      const overrideMessage=node('span','', 'muted');
      setButton.onclick=async()=>{
        setButton.disabled=true;clearButton.disabled=true;setStatus(overrideMessage,'Saving…');
        try{
          await api('/api/admin/users/permission',{method:'PUT',body:JSON.stringify({user_id:user.id,permission_key:permissionSelect.value,effect:effectSelect.value})});
          setStatus(overrideMessage,'Saved','success');await renderUsers();
        }catch(error){setStatus(overrideMessage,'ERROR: '+error.message,'error');}
        finally{setButton.disabled=false;clearButton.disabled=false;}
      };
      clearButton.onclick=async()=>{
        setButton.disabled=true;clearButton.disabled=true;setStatus(overrideMessage,'Clearing…');
        try{
          await api('/api/admin/users/permission',{method:'DELETE',body:JSON.stringify({user_id:user.id,permission_key:permissionSelect.value})});
          setStatus(overrideMessage,'Cleared','success');await renderUsers();
        }catch(error){setStatus(overrideMessage,'ERROR: '+error.message,'error');}
        finally{setButton.disabled=false;clearButton.disabled=false;}
      };
      overrideControls.append(permissionSelect,effectSelect,setButton,clearButton,overrideMessage);
      overrideCell.append(overrideControls);
    }else if(user.id===currentUser.user_id){
      overrideCell.append(node('div','Self overrides cannot be changed here.','self-note'));
    }
    row.append(overrideCell);
    const access=document.createElement('td');
    if(has('users.manage')&&roles.length&&user.id!==currentUser.user_id){
      const controls=node('div',null,'access-controls');
      const select=roleSelect(roles,user.role_id);
      const enabled=document.createElement('input');enabled.type='checkbox';enabled.checked=Boolean(user.member_enabled);
      const enabledLabel=document.createElement('label');enabledLabel.append(enabled,document.createTextNode('Enabled'));
      const save=node('button','Save');save.type='button';
      const message=node('span','', 'muted');
      save.onclick=async()=>{
        save.disabled=true;setStatus(message,'Saving…');
        try{
          await api('/api/admin/users/access',{method:'PATCH',body:JSON.stringify({user_id:user.id,role_id:select.value,enabled:enabled.checked})});
          setStatus(message,'Saved','success');await renderUsers();
        }catch(error){setStatus(message,'ERROR: '+error.message,'error');}
        finally{save.disabled=false;}
      };
      controls.append(select,enabledLabel,save,message);access.append(controls);
    }else if(user.id===currentUser.user_id){
      access.append(node('span','Current account cannot modify its own project access.','self-note'));
    }else{
      access.append(node('span','Read only','muted'));
    }
    row.append(access);body.append(row);
  }
  table.append(body);wrap.append(table);content.append(wrap);
}
async function renderRoles(){
  currentView='roles';
  content.replaceChildren();
  const title=node('h2','Roles');
  const status=node('p','Loading…','muted');
  content.append(title,status);
  let roles=[],permissions=[];
  try{
    roles=(await api('/api/admin/roles')).roles||[];
    permissions=(await api('/api/admin/permissions')).permissions||[];
  }catch(error){setStatus(status,'ERROR: '+error.message,'error');return;}
  status.remove();

  if(has('roles.manage')){
    const form=document.createElement('form');form.className='create-user';
    const name=document.createElement('input');name.required=true;name.maxLength=128;name.placeholder='Custom role name';
    const description=document.createElement('input');description.maxLength=512;description.placeholder='Description';
    const submit=node('button','Create custom role');submit.type='submit';
    const feedback=node('div','', 'muted');feedback.style.gridColumn='1/-1';
    form.append(name,description,submit,feedback);
    form.onsubmit=async event=>{
      event.preventDefault();submit.disabled=true;setStatus(feedback,'Creating…');
      try{
        await api('/api/admin/roles',{method:'POST',body:JSON.stringify({name:name.value,description:description.value})});
        setStatus(feedback,'Role created.','success');await renderRoles();
      }catch(error){setStatus(feedback,'ERROR: '+error.message,'error');}
      finally{submit.disabled=false;}
    };
    content.append(form);
  }

  const wrap=node('div',null,'table-wrap');
  const table=document.createElement('table');
  const head=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['Role','Type','Description','Permissions','Edit permissions'])hr.append(node('th',label));
  head.append(hr);table.append(head);
  const body=document.createElement('tbody');
  for(const role of roles){
    const row=document.createElement('tr');
    row.append(
      node('td',role.name),
      node('td',role.system_role?'system':'custom'),
      node('td',role.description||'—'),
      node('td',(role.permissions||[]).join(', ')||'none','permissions')
    );
    const edit=document.createElement('td');
    if(has('roles.manage')&&!role.system_role){
      const controls=node('div',null,'role-permission-editor');
      const select=document.createElement('select');select.multiple=true;select.size=Math.min(10,Math.max(5,permissions.length));
      const selected=new Set(role.permissions||[]);
      for(const permission of permissions){
        const option=document.createElement('option');option.value=permission.key;option.textContent=permission.key;option.selected=selected.has(permission.key);select.append(option);
      }
      const save=node('button','Save permissions');save.type='button';
      const message=node('span','', 'muted');
      save.onclick=async()=>{
        save.disabled=true;setStatus(message,'Saving…');
        const values=[...select.selectedOptions].map(option=>option.value);
        try{
          await api('/api/admin/roles',{method:'PATCH',body:JSON.stringify({role_id:role.id,permissions:values})});
          setStatus(message,'Saved','success');await renderRoles();
        }catch(error){setStatus(message,'ERROR: '+error.message,'error');}
        finally{save.disabled=false;}
      };
      controls.append(select,save,message);edit.append(controls);
    }else{
      edit.append(node('span',role.system_role?'System roles are read-only.':'Read only','muted'));
    }
    row.append(edit);body.append(row);
  }
  table.append(body);wrap.append(table);content.append(wrap);
}
function formatTime(value){
  if(!value)return '—';
  const date=new Date(value);
  return Number.isNaN(date.getTime())?String(value):date.toLocaleString();
}
async function renderSessions(){
  currentView='sessions';
  content.replaceChildren();
  const title=node('h2','Active sessions');
  const toolbar=node('div',null,'toolbar');
  const refresh=node('button','Refresh');refresh.type='button';
  const status=node('span','Loading…','muted');
  refresh.onclick=()=>renderSessions();
  toolbar.append(refresh,status);content.append(title,toolbar);
  let sessions=[];
  try{sessions=(await api('/api/admin/sessions')).sessions||[];}
  catch(error){setStatus(status,'ERROR: '+error.message,'error');return;}
  setStatus(status,sessions.length?sessions.length+' active session(s)':'No active sessions');
  if(!sessions.length)return;
  const wrap=node('div',null,'table-wrap');
  const table=document.createElement('table');
  const head=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['User','Created','Last seen','Expires','Client','Action'])hr.append(node('th',label));
  head.append(hr);table.append(head);
  const body=document.createElement('tbody');
  for(const session of sessions){
    const row=document.createElement('tr');
    row.append(
      node('td',session.username||session.user_id),
      node('td',formatTime(session.created_at)),
      node('td',formatTime(session.last_seen_at)),
      node('td',formatTime(session.expires_at)),
      node('td',session.client_metadata||'—','permissions')
    );
    const action=document.createElement('td');
    const revoke=node('button','Force logout');revoke.type='button';
    const message=node('span','', 'muted');
    revoke.onclick=async()=>{
      if(!window.confirm('Revoke this login session?'))return;
      revoke.disabled=true;setStatus(message,'Revoking…');
      try{
        await api('/api/admin/sessions',{method:'DELETE',body:JSON.stringify({session_id:session.id})});
        setStatus(message,'Revoked','success');await renderSessions();
      }catch(error){setStatus(message,'ERROR: '+error.message,'error');}
      finally{revoke.disabled=false;}
    };
    action.append(revoke,message);row.append(action);body.append(row);
  }
  table.append(body);wrap.append(table);content.append(wrap);
}
function auditCategory(action){
  action=String(action||'').toLowerCase();
  if(action.startsWith('git.'))return 'Git';
  if(action.startsWith('db.'))return 'Database';
  if(action.startsWith('transfer.')||action.includes('file-transfer'))return 'Transfer';
  if(action.startsWith('task.')||action.includes('session_create')||action.startsWith('terminal.'))return 'Task / Terminal';
  if(action.startsWith('file.')||action.startsWith('project.file')||action.includes('filesystem'))return 'Files';
  if(action.startsWith('approval.')||action.startsWith('auth.')||action.startsWith('authorization.'))return 'Security';
  if(action.startsWith('admin.')||action.startsWith('config.')||action.startsWith('settings.'))return 'Config / Admin';
  return 'Other';
}
function auditDetailsText(details){
  const raw=String(details||'').trim();
  if(!raw)return '{}';
  try{return JSON.stringify(JSON.parse(raw),null,2);}catch{return raw;}
}
function auditEventNode(event){
  const result=String(event.result||'').toLowerCase();
  const item=node('article',null,'audit-event '+result);
  const head=node('div',null,'audit-event-head');
  const time=node('span',formatTime(event.timestamp),'audit-event-time');
  const actor=node('span',event.username||event.user_id||'system','audit-event-actor');
  if(event.username&&event.user_id)actor.title=event.user_id;
  const category=node('span',auditCategory(event.action),'audit-event-category');
  const action=node('span',event.action||'unknown','audit-event-action');
  const resultNode=node('span',event.result||'—','audit-event-result');
  head.append(time,actor,category,action,resultNode);
  const resource=node('div',null,'audit-event-resource');
  const resourceParts=[];
  if(event.resource_type)resourceParts.push(event.resource_type);
  if(event.resource_id)resourceParts.push(event.resource_id);
  resource.textContent=resourceParts.length?resourceParts.join(' · '):'—';
  const meta=node('div','Client '+(event.client_ip||'—')+' · Event '+(event.id||'—'),'audit-event-meta');
  const details=document.createElement('details');
  const detailsSummary=node('summary','Details');
  const pre=document.createElement('pre');pre.textContent=auditDetailsText(event.details);
  details.append(detailsSummary,pre);
  item.append(head,resource,meta,details);
  return item;
}
async function renderAudit(filters={}){
  currentView='audit';
  content.replaceChildren();
  const title=node('h2','Audit timeline');
  const intro=node('p','Security and operational activity for this project. Results are newest-first and loaded in bounded pages.','muted');
  const form=node('form',null,'audit-filters');
  const user=document.createElement('input');user.placeholder='Username or user ID';user.maxLength=128;user.value=filters.user||'';
  const action=document.createElement('input');action.placeholder='Action (exact, e.g. git.push)';action.maxLength=128;action.value=filters.action||'';
  const resourceType=document.createElement('input');resourceType.placeholder='Resource type';resourceType.maxLength=128;resourceType.value=filters.resource_type||'';
  const resourceID=document.createElement('input');resourceID.placeholder='Resource ID';resourceID.maxLength=512;resourceID.value=filters.resource_id||'';
  const result=document.createElement('select');
  for(const [value,label] of [['','Any result'],['success','Success'],['denied','Denied'],['error','Error'],['failed','Failed']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;option.selected=value===String(filters.result||'');result.append(option);
  }
  const placeholder=document.createElement('span');
  const controls=node('div',null,'audit-filter-actions');
  const apply=node('button','Apply filters');apply.type='submit';
  const reset=node('button','Reset');reset.type='button';
  const refresh=node('button','Refresh');refresh.type='button';
  const status=node('span','Loading…','muted');
  controls.append(apply,reset,refresh,status);
  form.append(user,action,resourceType,resourceID,result,placeholder,controls);
  content.append(title,intro,form);

  const timeline=node('div',null,'audit-timeline');
  content.append(timeline);
  const loadOlder=node('button','Load older','audit-load-more');loadOlder.type='button';loadOlder.hidden=true;content.append(loadOlder);

  const currentFilters=()=>({
    user:user.value.trim(),action:action.value.trim(),resource_type:resourceType.value.trim(),
    resource_id:resourceID.value.trim(),result:result.value
  });
  let nextBefore='',loading=false,total=0;
  function buildQuery(before=''){
    const values=currentFilters();const query=new URLSearchParams({limit:'100'});
    for(const [key,value] of Object.entries(values))if(value)query.set(key,value);
    if(before)query.set('before',before);
    return query;
  }
  async function loadPage({append=false}={}){
    if(loading)return;loading=true;apply.disabled=true;refresh.disabled=true;loadOlder.disabled=true;
    setStatus(status,append?'Loading older events…':'Loading…');
    try{
      const data=await api('/api/admin/audit?'+buildQuery(append?nextBefore:'').toString());
      const events=Array.isArray(data?.events)?data.events:[];
      if(!append){timeline.replaceChildren();total=0;}
      for(const event of events)timeline.append(auditEventNode(event));
      total+=events.length;nextBefore=String(data?.next_before||'');
      setStatus(status,total?total+' event(s) loaded':'No audit events');
      loadOlder.hidden=!nextBefore;
      if(!total){
        const empty=node('div','No events match these filters.','muted');timeline.append(empty);
      }
    }catch(error){setStatus(status,'ERROR: '+error.message,'error');}
    finally{loading=false;apply.disabled=false;refresh.disabled=false;loadOlder.disabled=false;}
  }
  form.onsubmit=event=>{event.preventDefault();renderAudit(currentFilters());};
  reset.onclick=()=>renderAudit({});
  refresh.onclick=()=>loadPage({append:false});
  loadOlder.onclick=()=>loadPage({append:true});
  await loadPage({append:false});
}
function renderPlaceholder(view){
  currentView=view;
  const labels={sessions:'Active sessions',audit:'Audit log'};
  content.replaceChildren();
  content.append(node('h2',labels[view]||'Administration'),node('p','This section will load from its permission-gated API.','muted'));
}
function selectView(view){
  if(!canOpen(view))return;
  for(const button of document.querySelectorAll('nav button'))button.classList.toggle('active',button.dataset.view===view);
  if(view==='users')renderUsers();
  else if(view==='roles')renderRoles();
  else if(view==='sessions')renderSessions();
  else if(view==='audit')renderAudit();
  else renderPlaceholder(view);
}
async function start(){
  currentUser=await api('/api/auth/me');
  permissionSet=new Set(currentUser.permissions||[]);
  identity.textContent=currentUser.username+' · '+currentUser.project_key;
  const buttons=[...document.querySelectorAll('nav button')];
  for(const button of buttons){
    button.hidden=!canOpen(button.dataset.view);
    button.onclick=()=>selectView(button.dataset.view);
  }
  const first=buttons.find(button=>!button.hidden);
  if(first)selectView(first.dataset.view);
  else content.textContent='No administration capability is available.';
}
start().catch(error=>{content.textContent='ERROR: '+error.message;});`
