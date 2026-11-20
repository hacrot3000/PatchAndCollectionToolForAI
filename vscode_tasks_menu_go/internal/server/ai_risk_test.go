package server

import "testing"

func TestAIProposalRiskConservativeAgainstModelDowngrade(t *testing.T){
 cases:=[]struct{command,model,want string}{
  {"pwd","read-only","read-only"},
  {"git status --short","low","read-only"},
  {"pwd","critical","critical"},
  {"echo hello","read-only","high"},
  {"unknown-tool --flag","low","high"},
  {"git reset --hard","read-only","critical"},
  {"rm -rf /","read-only","critical"},
  {"systemctl restart game","low","critical"},
  {"git status; rm /tmp/x","read-only","critical"},
  {"pwd | cat","read-only","critical"},
  {"sh -c 'pwd'","read-only","high"},
  {"echo $(touch x)","read-only","critical"},
  {"","read-only","critical"},
 }
 for _,tc:=range cases{
  got:=aiProposalRisk(tc.command,tc.model)
  if got!=tc.want{t.Errorf("risk(%q,%q)=%q want %q",tc.command,tc.model,got,tc.want)}
 }
}
