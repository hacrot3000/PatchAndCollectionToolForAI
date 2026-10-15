package server

import (
 "io"
 "net/http"
)

// This dedicated page never mounts the legacy terminal/editor (which would
// request disabled full-file, task, download or SSH routes). The script is an
// external embedded asset to comply with the normal CSP script-src 'self'.
func (s *Server) restrictedPage(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodGet {
  w.Header().Set("Allow","GET")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 _,_=io.WriteString(w,restrictedHTML)
}

const restrictedHTML=`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>TaskDeck · Restricted Developer</title>
<style>
:root{font:14px/1.55 system-ui,sans-serif;color-scheme:dark;background:#111820;color:#e6edf8}
*{box-sizing:border-box}body{margin:0}
header{padding:12px 20px;border-bottom:1px solid #455364;display:flex;align-items:center;gap:14px}
small,.muted{color:#a6b2c5}h1{font-size:17px;margin:0}h2{font-size:16px;margin:0 0 12px}
main{display:grid;grid-template-columns:minmax(310px,1fr) minmax(400px,1.5fr);gap:16px;padding:16px}
section{padding:16px;background:#192431;border:1px solid #455669;border-radius:10px;min-width:0}
label{display:block;font-weight:600;margin:9px 0 5px}
input{background:#0d1720;color:inherit;border:1px solid #6d8095;padding:9px;border-radius:5px;width:100%;font:13px monospace}
.actions{display:flex;gap:8px;align-items:center;margin-top:10px}
button{background:#2a4e74;border:1px solid #5c87b0;color:#fff;padding:8px 14px;border-radius:6px;cursor:pointer}
button:disabled{opacity:.55}button.secondary{background:#263647}
pre{margin:12px 0 0;padding:12px;min-height:150px;max-height:55vh;overflow:auto;
 background:#0b141e;border:1px solid #364960;border-radius:5px;font:12px/1.55 ui-monospace,monospace;
 white-space:pre;tab-size:4;user-select:text}
.status{margin-top:10px;min-height:20px;color:#c5d5ec;font-size:12px}
.warning{color:#efc890}.fields{display:grid;grid-template-columns:1fr 100px 100px;gap:8px;align-items:end}
.fields label{margin-top:0}
code{background:#111a26;padding:1px 5px}
@media(max-width:880px){main{grid-template-columns:1fr}.fields{grid-template-columns:1fr 90px 90px}}
</style>
</head>
<body>
<header><h1>TaskDeck · Restricted Developer</h1><small>Protected workspace preview · Startup-only security</small></header>
<main>
<section aria-label="Secure Command Gateway">
<h2>Secure Command Gateway</h2>
<p class="muted">Only approved, exact commands are available. No Bash/SSH, arbitrary Git commands or source export.</p>
<label for="command">Command</label>
<input id="command" autocomplete="off" list="command-presets" placeholder="git status">
<datalist id="command-presets">
<option value="git status"></option><option value="git branch"></option>
<option value="git rev-parse --abbrev-ref HEAD"></option><option value="ls"></option><option value="pwd"></option>
</datalist>
<div class="actions"><button id="run">Run command</button><span class="muted">↑/↓ command history</span></div>
<p id="command-status" class="status" role="status"></p>
<pre id="command-output" aria-label="Command output"></pre>
</section>
<section aria-label="Protected source view">
<h2>Protected Code View</h2>
<p class="muted">Only a quota-charged line window is sent to your browser. Editing, full-file views, Diff, History, Compare and exports remain locked pending security review.</p>
<div class="fields">
<label for="file">Relative file path<input id="file" placeholder="src/main.go" autocomplete="off"></label>
<label for="start">Start line<input id="start" type="number" min="1" value="1"></label>
<label for="count">Lines<input id="count" type="number" min="1" value="10"></label>
</div>
<div class="actions"><button id="view">View lines</button><button id="copy" class="secondary" disabled>Copy selection</button></div>
<p id="file-status" class="status" role="status"></p>
<pre id="source-output" aria-label="Protected code excerpt"></pre>
<p class="muted">Copy is best-effort limited in the browser. The server enforces cumulative viewing quotas across reloads; screenshots or browser developer tools cannot be prevented absolutely.</p>
</section>
</main>
<script type="module" src="/featuremods/restricted.js"></script>
</body></html>`
