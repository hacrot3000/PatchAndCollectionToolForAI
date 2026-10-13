package server

import "strings"

// aiReadOnlyToolName maps ONLY exact, argument-free diagnostics to typed
// candidates. It is a classifier, not an executor, and never launches shells.
func aiReadOnlyToolName(command string)(string,bool){
 normalized:=strings.ToLower(strings.Join(strings.Fields(command)," "))
 switch normalized{
 case "pwd":return "terminal.pwd",true
 case "whoami":return "terminal.whoami",true
 case "id":return "terminal.identity",true
 case "date":return "terminal.date",true
 case "uptime":return "host.uptime",true
 case "uname -a":return "host.uname",true
 case "df -h":return "host.disk",true
 case "free -h":return "host.memory",true
 case "git status":return "git.status",true
 case "git status --short":return "git.status_short",true
 case "git diff --check":return "git.diff_check",true
 case "git diff --stat":return "git.diff_stat",true
 case "git branch --show-current":return "git.branch",true
 default:return "",false
 }
}

// aiProposalRisk is a conservative annotation, not a shell security parser.
// Only fixed diagnostic commands may be called read-only. Unknown commands
// are treated as high-risk; this never grants execution permission.
func aiProposalRisk(command,modelRisk string)string{
 cmd:=strings.TrimSpace(command)
 if cmd==""{return "critical"}
 if strings.ContainsAny(cmd,"\n\r;&|><`$(){}\\"){
  return "critical"
 }
 words:=strings.Fields(cmd)
 if len(words)==0{return "critical"}
 lower:=strings.ToLower(strings.Join(words," "))
 if _,safe:=aiReadOnlyToolName(lower);safe{
  // Preserve a stricter classification if an operator supplied one.
  switch modelRisk {case "critical","high","medium":return modelRisk}
  return "read-only"
 }
 for _,prefix:=range []string{"sudo ","su ","rm ","mv ","chmod ","chown ","dd ","mkfs",
  "systemctl ","service ","docker ","kubectl ","git reset","git clean",
  "git push","git pull","git merge","git rebase","git checkout",
  "mysql ","psql ","sqlite3 ","kill ","pkill ","shutdown ","reboot ","curl ","wget ","ssh ","scp ","rsync "} {
  if lower==strings.TrimSpace(prefix)||strings.HasPrefix(lower,prefix){return "critical"}
 }
 return "high"
}
