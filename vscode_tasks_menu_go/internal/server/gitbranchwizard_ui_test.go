package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitBranchWizardLoadedBeforeGitPanel(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/next.js")
 if err!=nil {t.Fatal(err)}
 js:=string(data)
 wizard:=strings.Index(js,"/featuremods/gitbranchwizard.js")
 git:=strings.Index(js,"/featuremods/gitstatus.js")
 if wizard<0||git<wizard {t.Fatal("Git branch wizard must load before Git status panel")}
}

func TestGitBranchChoiceWizardHasRadioButtonsCancelAndKeyboardAccess(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/gitbranchwizard.js")
 if err!=nil {t.Fatal(err)}
 js:=string(data)
 for _,want:=range []string{
  "TaskDeckGitBranchChoiceWizard={open,close}",
  "dialog.setAttribute('role','dialog')",
  "dialog.setAttribute('aria-modal','true')",
  "radio.type='radio'",
  "radio.name=radioName",
  "radio.onchange=()=>{if(radio.checked)choose(choice.id);}",
  "confirm.onclick=()=>{if(session?.selected)close(session.selected);}",
  "cancel.onclick=()=>close(null)",
  "if(event.key==='Escape')",
  "if(event.key!=='Tab')return",
  "choices.some(item=>item.id===defaultChoice)?defaultChoice:choices[0].id",
  "name.textContent=choice.label",
  "ref.textContent=choice.ref",
  "desc.textContent=choice.description",
  "optionsNode.querySelector('.git-branch-choice-option.selected input')?.focus()",
  "html[data-taskmenu-theme=\"light\"] .git-branch-choice-dialog",
 } {
  if !strings.Contains(js,want) {t.Errorf("branch wizard missing %q",want)}
 }
 if strings.Contains(js,"innerHTML") || strings.Contains(js,"window.prompt(") || strings.Contains(js,"eval(") {
  t.Fatal("branch wizard must be safe DOM selection UI, not text input or HTML injection")
 }
}

func TestGitMergeDivergenceUsesChoiceAndSHAProtection(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
 if err!=nil {t.Fatal(err)}
 js:=string(data)
 start:=strings.Index(js,"async function mergeBranch(branch){")
 end:=strings.Index(js[start:],"async function mergeToBranch(branch)")
 if start<0||end<0 {t.Fatal("merge branch UI missing")}
 merge:=js[start:start+end]
 for _,want:=range []string{
  "if(check.requires_choice){",
  "gitBranchChoiceWizard.open({",
  "id:'local'",
  "id:'remote'",
  "check.local_ref+' @ '+localSHA",
  "check.remote_ref+' @ '+remoteSHA",
  "confirmLabel:'Merge selected version'",
  "if(!source){showOperation(label,'Merge canceled');return false;}",
  "if(activeRepoID!==repoID)",
  "expected_sha:expectedSHA",
  "expected_current:check.current",
  "merge_ref:mergeRef",
  "return action('merge'",
  "{repoID}",
 } {
  if !strings.Contains(merge,want) {t.Errorf("merge preflight wizard missing %q",want)}
 }
 if strings.Contains(merge,"window.prompt(") || strings.Contains(merge,"Type LOCAL or REMOTE") {
  t.Fatal("Git merge must not ask users to type LOCAL/REMOTE")
 }
}

func TestDeleteUpstreamRemoteFollowupDefaultsToKeepAndNeverForcesLocalDelete(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
 if err!=nil {t.Fatal(err)}
 js:=string(data)
 start:=strings.Index(js,"async function deleteRemoteBranch(branch,")
 end:=strings.Index(js[start:],"async function loadBranches()")
 if start<0||end<0 {t.Fatal("remote deletion handler missing")}
 handler:=js[start:start+end]
 for _,want:=range []string{
  "if(result===false)return false",
  "if(localBranch?.name&&repoID===activeRepoID)",
  "branches=await gitView('branches')",
  "const existing=branches?.local?.find(item=>item.name===localBranch.name)",
  "if(existing&&!existing.current)",
  "title:'Remote branch deleted · delete local too?'",
  "id:'keep'",
  "id:'delete'",
  "defaultChoice:'keep'",
  "git branch -d ",
  "if(choice==='delete'&&activeRepoID===repoID)",
  "action('delete_branch',{branch:existing.name,confirmed:true},'',{repoID,refresh:false})",
  "else if(existing?.current)",
  "removeRenderedBranchRow(repoID,remoteRef,true)",
  "if(localResult!==false)removeRenderedBranchRow(repoID,existing.name,false)",
  "if(repoID===activeRepoID)await refresh()",
 } {
  if !strings.Contains(handler,want) {t.Errorf("remote branch cleanup missing %q",want)}
 }
 if strings.Contains(handler,"force_delete_branch")||strings.Contains(handler,"delete_branch',{branch:name") {
  t.Fatal("remote cleanup must never force delete or use a guessed branch")
 }
 if !strings.Contains(js,"deleteRemoteBranch({name:upstream},{localBranch:branch})") {
  t.Fatal("explicit upstream deletion wizard must preserve association with the selected local branch")
 }
 if !strings.Contains(js,"if(choice==='remote')return deleteRemoteBranch(branch)") {
  t.Fatal("remote-tracking delete wizard must not assume a matching local branch")
 }
}

func TestGitBranchRowsUseConsolidatedWizardAndClickableName(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
 if err!=nil {t.Fatal(err)}
 js:=string(data)
 start:=strings.Index(js,"async function loadBranches(){")
 end:=strings.Index(js[start:],"function worktreeDirectorySuggestion")
 if start<0||end<0{t.Fatal("Branches row rendering missing")}
 rows:=js[start:start+end]
 for _,want:=range []string{
  "actionButton('Merge',()=>openBranchMergeWizard(branch)",
  "actionButton('Delete',()=>openBranchDeleteWizard(branch)",
  "const branchName=el('button','git-branch-name-button',branch.name)",
  "branchName.onclick=async()=>",
  "await copyText(branch.name)",
  "branchName.classList.add('copied')",
  "branchName.setAttribute('aria-label','Copy branch name '+branch.name)",
 }{
  if !strings.Contains(rows,want){t.Errorf("consolidated Git branch row missing %q",want)}
 }
 for _,old:=range []string{
  "actionButton('Merge From'",
  "actionButton('Merge To'",
  "actionButton('Force delete local'",
  "actionButton('Delete upstream remote'",
  "actionButton('Forget local ref'",
  "actionButton('Delete on remote'",
  "actionButton('Copy name'",
 }{
  if strings.Contains(rows,old){t.Errorf("old branch row action must not remain: %q",old)}
 }
 if strings.Count(rows,"actionButton('Merge',")!=1||strings.Count(rows,"actionButton('Delete',")!=1 {
  t.Error("one Merge and one Delete action per eligible branch row")
 }
}
