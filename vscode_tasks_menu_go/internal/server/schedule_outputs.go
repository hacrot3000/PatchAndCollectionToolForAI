package server

import(
 "os"
 "regexp"
 "sort"
 "strconv"
 "strings"
)

var scheduleOutputPattern=regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,40}$`)

// TASKDECK_OUTPUT NAME=value is opt-in stdout metadata from an executed job.
// Bounded values can be passed to the directly triggered downstream job using
// TASKDECK_DEP_NAME env vars; arbitrary user output never becomes shell code.
func scheduleParseOutputs(output string)map[string]string{
 vars:=map[string]string{}
 for _,line:=range strings.Split(output,"\n"){
  line=strings.TrimSuffix(line,"\r")
  if !strings.HasPrefix(line,"TASKDECK_OUTPUT "){continue}
  raw:=strings.TrimPrefix(line,"TASKDECK_OUTPUT ")
  key,value,ok:=strings.Cut(raw,"=")
  if !ok||!scheduleOutputPattern.MatchString(key)||len(value)>1024||strings.ContainsRune(value,'\x00'){continue}
  if len(vars)>=20&&vars[key]==""{continue}
  vars[key]=value
 }
 return vars
}
func scheduleDependencyEnv(parentID string,runs []scheduleRun)[]string{
 if parentID==""{return nil}
 for _,run:=range runs{
  if run.ID!=parentID{continue}
  names:=make([]string,0,len(run.Outputs))
  for key:=range run.Outputs{names=append(names,key)}
  sort.Strings(names)
  env:=[]string{}
  for _,key:=range names{
   value:=run.Outputs[key]
   if scheduleOutputPattern.MatchString(key)&&len(value)<=1024{env=append(env,"TASKDECK_DEP_"+key+"="+value)}
  }
  return env
 }
 return nil
}
// scheduleDependencyEnvAll exposes outputs from every parent run in a
// workflow fan-in without guessing which parent owns a duplicate key.
// TASKDECK_DEP_NAME remains an alias for the directly linked parent, while
// TASKDECK_DEP_PARENT_1_NAME, ... are stable indexed parent namespaces.
// Each parent already has a bounded output map (20 keys, <=1024 bytes/key).
// At most 16 parents are exposed to prevent excessive process environments.
func scheduleDependencyEnvAll(primaryID string,parentIDs []string,runs []scheduleRun)[]string{
 result:=scheduleDependencyEnv(primaryID,runs)
 ids:=[]string{}
 seen:=map[string]bool{}
 for _,id:=range parentIDs{
  if id==""||seen[id]{continue}
  seen[id]=true
  ids=append(ids,id)
  if len(ids)==16{break}
 }
 if len(ids)==0&&primaryID!=""{ids=append(ids,primaryID)}
 lookup:=map[string]scheduleRun{}
 for _,run:=range runs{lookup[run.ID]=run}
 for index,id:=range ids{
  run,found:=lookup[id]
  if !found{continue}
  // Use stable condition traversal order instead of sorting by timestamps.
  prefix:="TASKDECK_DEP_PARENT_"+strconv.Itoa(index+1)+"_"
  result=append(result,prefix+"RUN_ID="+run.ID,prefix+"SCHEDULE_ID="+run.ScheduleID)
  keys:=make([]string,0,len(run.Outputs))
  for key:=range run.Outputs{keys=append(keys,key)}
  sort.Strings(keys)
  for _,key:=range keys{
   // Never permit an output variable to impersonate run provenance.
   if key=="RUN_ID"||key=="SCHEDULE_ID"{continue}
   value:=run.Outputs[key]
   if scheduleOutputPattern.MatchString(key)&&len(value)<=1024&&!strings.ContainsRune(value,'\x00'){
    result=append(result,prefix+key+"="+value)
   }
  }
 }
 return result
}

func scheduleEnvironment(specEnv,dependencyEnv []string)[]string{
 if len(specEnv)==0&&len(dependencyEnv)==0{return nil}
 base:=specEnv
 if len(base)==0{base=os.Environ()}
 return append(append([]string{},base...),dependencyEnv...)
}
