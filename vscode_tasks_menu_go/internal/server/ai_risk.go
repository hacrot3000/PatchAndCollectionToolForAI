package server

import "strings"

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
 switch lower {
 case "pwd","whoami","id","date","uptime","uname -a","df -h","free -h",
 "git status","git status --short","git diff --check","git diff --stat","git branch --show-current":
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
