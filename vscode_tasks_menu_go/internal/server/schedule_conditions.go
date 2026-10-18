package server

import (
 "errors"
 "fmt"
 "path/filepath"
 "time"
)

// condition_group is a recursive expression tree: every group may contain
// arbitrarily many leaf conditions and arbitrarily many nested AND/OR groups.
// Condition IDs are stable across saves, and identify independent poll baselines.
type scheduleCondition struct {
 ID string `json:"id"`
 Type string `json:"type"`
 Cron string `json:"cron,omitempty"`
 Timezone string `json:"timezone,omitempty"`
 IntervalSeconds int `json:"interval_seconds,omitempty"`
 URL string `json:"url,omitempty"`
 MatchMode string `json:"match_mode,omitempty"`
 MatchText string `json:"match_text,omitempty"`
 JSONPath string `json:"json_path,omitempty"`
 GitPath string `json:"git_path,omitempty"`
 GitRef string `json:"git_ref,omitempty"`
 GitCondition string `json:"git_condition,omitempty"`
 FilePath string `json:"file_path,omitempty"`
 FileCondition string `json:"file_condition,omitempty"`
 ParentID string `json:"parent_id,omitempty"`
 ParentState string `json:"parent_state,omitempty"`
 ExitCode *int `json:"exit_code,omitempty"`
 OutputContains string `json:"output_contains,omitempty"`
 OutputAbsent string `json:"output_absent,omitempty"`
}
type scheduleConditionGroup struct {
 Operator string `json:"operator"` // and / or
 Conditions []scheduleCondition `json:"conditions,omitempty"`
 Groups []scheduleConditionGroup `json:"groups,omitempty"`
}
func scheduleConditionsFor(def scheduleDefinition) scheduleConditionGroup {
 if def.ConditionGroup!=nil{return *def.ConditionGroup}
 // The legacy single-trigger definitions remain readable and executable.
 return scheduleConditionGroup{Operator:"or",Conditions:[]scheduleCondition{{
  ID:"legacy",Type:def.Type,Cron:def.Cron,Timezone:def.Timezone,
  IntervalSeconds:def.IntervalSeconds,URL:def.URL,MatchMode:def.MatchMode,
  MatchText:def.MatchText,JSONPath:def.JSONPath,GitPath:def.GitPath,
  GitRef:def.GitRef,GitCondition:def.GitCondition,FilePath:def.FilePath,FileCondition:def.FileCondition,ParentID:def.ParentID,
  ParentState:def.ParentState,ExitCode:def.ExitCode,OutputContains:def.OutputContains,OutputAbsent:def.OutputAbsent,
 }}}
}
func scheduleLeaves(group scheduleConditionGroup) []scheduleCondition {
 // The editor limits depth to 12; bound recursion defensively for old files.
 leaves:=[]scheduleCondition{}
 var walk func(scheduleConditionGroup,int)
 walk=func(g scheduleConditionGroup,depth int){
  if depth>16{return}
  leaves=append(leaves,g.Conditions...)
  for _,child:=range g.Groups{walk(child,depth+1)}
 }
 walk(group,0);return leaves
}
func scheduleGroupSatisfied(group scheduleConditionGroup,met map[string]bool) bool {
 // Invalid or empty groups NEVER match accidentally.
 count:=len(group.Conditions)+len(group.Groups)
 if count==0{return false}
 if group.Operator!="and"&&group.Operator!="or"{return false}
 matches:=0
 for _,c:=range group.Conditions{if met[c.ID]{matches++}}
 for _,g:=range group.Groups{if scheduleGroupSatisfied(g,met){matches++}}
 if group.Operator=="and"{return matches==count}
 return matches>0
}
func scheduleParentReferences(def scheduleDefinition) []string {
 parents:=[]string{}
 for _,c:=range scheduleLeaves(scheduleConditionsFor(def)){
  if c.Type=="dependency"&&c.ParentID!=""{parents=append(parents,c.ParentID)}
 }
 return parents
}
func (s *Server) validateCondition(def scheduleDefinition,cond scheduleCondition,store scheduleStore)error{
 switch cond.Type{
 case "cron":
  loc:=time.Local
  if cond.Timezone!=""&&cond.Timezone!="Local"{
   var err error;loc,err=time.LoadLocation(cond.Timezone)
   if err!=nil{return fmt.Errorf("invalid timezone: %w",err)}
  }
  _,err:=cronMatches(cond.Cron,time.Now().In(loc));return err
 case "url":
  if cond.IntervalSeconds<30||cond.IntervalSeconds>86400{return errors.New("URL interval must be 30-86400 seconds")}
  if cond.MatchText==""||len(cond.MatchText)>2048{return errors.New("URL match text is required (max 2048 characters)")}
  if cond.MatchMode!="contains"&&cond.MatchMode!="equals"&&cond.MatchMode!="json"&&cond.MatchMode!="xml"{return errors.New("URL mode must be contains, equals, json or xml")}
  return validateScheduleURL(cond.URL)
 case "git":
  if cond.IntervalSeconds<30||cond.IntervalSeconds>86400{return errors.New("Git interval must be 30-86400 seconds")}
  if cond.GitPath==""{return errors.New("Git repository directory required")}
  if cond.GitCondition!="change"&&cond.GitCondition!="dirty"&&cond.GitCondition!="clean"{return errors.New("Git condition must be change, dirty or clean")}
  requested,err:=canonicalExistingPath(filepath.Join(s.Workspace,cond.GitPath))
  if err!=nil{return err}
  root,err:=verifyGitRepository(s.Workspace,requested);if err!=nil{return err}
  if filepath.Clean(root)!=filepath.Clean(requested){return errors.New("choose Git repository root, not a nested subdirectory")}
  return nil
 case "file":
  if cond.IntervalSeconds<30||cond.IntervalSeconds>86400{return errors.New("file interval must be 30-86400 seconds")}
  return s.validateScheduleFile(cond.FilePath,cond.FileCondition)
 case "webhook":
  // A webhook leaf is an event pushed through the authenticated Schedule API.
  // It is never polled, and has no additional mandatory fields.
  return nil
 case "dependency":
  if cond.ParentID==""||cond.ParentID==def.ID{return errors.New("valid parent schedule required")}
  if cond.ParentState!=""&&cond.ParentState!="any"&&cond.ParentState!="success"&&cond.ParentState!="failure"{return errors.New("parent state must be success, failure or any")}
  for _,item:=range store.Schedules{if item.ID==cond.ParentID{return nil}}
  return errors.New("parent schedule not found")
 default:return fmt.Errorf("unknown condition type %q",cond.Type)
 }
}
func (s *Server) validateScheduleGroup(def scheduleDefinition,store scheduleStore)error {
 if def.ConditionGroup==nil{return errors.New("condition group required")}
 ids:=map[string]bool{}
 total:=0
 var walk func(scheduleConditionGroup,int)error
 walk=func(g scheduleConditionGroup,depth int)error{
  if depth>12{return errors.New("condition nesting exceeds 12 levels")}
  if g.Operator!="and"&&g.Operator!="or"{return errors.New("condition group operator must be and or or")}
  if len(g.Conditions)+len(g.Groups)==0{return errors.New("condition group cannot be empty")}
  for _,cond:=range g.Conditions{
   total++
   if cond.ID==""||len(cond.ID)>80||ids[cond.ID]{return errors.New("each condition must have a unique nonempty id")}
   ids[cond.ID]=true
   if err:=s.validateCondition(def,cond,store);err!=nil{return fmt.Errorf("condition %s: %w",cond.ID,err)}
  }
  for _,child:=range g.Groups{if err:=walk(child,depth+1);err!=nil{return err}}
  return nil
 }
 if err:=walk(*def.ConditionGroup,0);err!=nil{return err}
 if total==0{return errors.New("at least one trigger condition is required")}
 return validateScheduleDependencyGraph(def,store)
}

// Check cycles in the whole mixed legacy/multi graph even when a user
// converts a multi schedule back to a single dependency condition.
func validateScheduleDependencyGraph(def scheduleDefinition,store scheduleStore)error{
 // Check cycles over the entire graph, including legacy definitions and
 // references in different OR branches; all cycles are disallowed.
 byID:=map[string]scheduleDefinition{}
 for _,item:=range store.Schedules{byID[item.ID]=item}
 byID[def.ID]=def
 visiting,done:=map[string]bool{},map[string]bool{}
 var visit func(string)error
 visit=func(id string)error{
  if visiting[id]{return errors.New("schedule dependency cycle")}
  if done[id]{return nil}
  visiting[id]=true
  for _,parent:=range scheduleParentReferences(byID[id]){
   if _,exists:=byID[parent];!exists{return fmt.Errorf("dependency %s not found",parent)}
   if err:=visit(parent);err!=nil{return err}
  }
  delete(visiting,id);done[id]=true
  return nil
 }
 // Check all nodes to catch cycles that involve unaffected existing entries.
 for id:=range byID{if err:=visit(id);err!=nil{return err}}
 return nil
}
