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
      <button type="button" data-view="sessions">Sessions</button>
      <button type="button" data-view="audit">Audit Log</button>
    </nav>
    <section id="admin-content" aria-live="polite">Loading access data…</section>
  </main>
  <script type="module" src="/admin/access.js"></script>
</body>
</html>`

const sharedAdminCSS = `:root{font-family:system-ui,sans-serif;color-scheme:dark;background:#101216;color:#e8eaed}*{box-sizing:border-box}body{margin:0}header{height:52px;display:flex;gap:14px;align-items:center;padding:0 16px;border-bottom:1px solid #30343b}header a{color:#9fc8f5;text-decoration:none}header strong{font-size:16px}#admin-identity{margin-left:auto;font-size:12px;opacity:.75}main{max-width:1180px;margin:0 auto;padding:20px}nav{display:flex;gap:8px;margin-bottom:18px}button{background:#252a33;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 12px;cursor:pointer}button.active{background:#29445f;border-color:#47759e}#admin-content{border:1px solid #30343b;border-radius:10px;min-height:240px;padding:18px;background:#15181e}.muted{opacity:.7}`

const sharedAdminJS = `const content=document.querySelector('#admin-content');
const identity=document.querySelector('#admin-identity');
let currentUser=null;

function has(permission){return new Set(currentUser?.permissions||[]).has(permission);}
function canOpen(view){
  if(view==='users')return has('users.view')||has('users.manage')||has('roles.view')||has('roles.manage');
  if(view==='sessions')return has('sessions.manage');
  if(view==='audit')return has('audit.view');
  return false;
}
function renderPlaceholder(view){
  const labels={users:'Users and project access',sessions:'Active sessions',audit:'Audit log'};
  content.replaceChildren();
  const title=document.createElement('h2');title.textContent=labels[view]||'Administration';
  const note=document.createElement('p');note.className='muted';note.textContent='This administration section is being loaded from permission-gated server APIs.';
  content.append(title,note);
}
function selectView(view){
  if(!canOpen(view))return;
  for(const button of document.querySelectorAll('nav button'))button.classList.toggle('active',button.dataset.view===view);
  renderPlaceholder(view);
}
async function start(){
  const response=await fetch('/api/auth/me',{cache:'no-store'});
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  currentUser=await response.json();
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
