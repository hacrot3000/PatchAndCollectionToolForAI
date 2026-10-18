package server

import(
 "os"
 "regexp"
 "sort"
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
func scheduleEnvironment(specEnv,dependencyEnv []string)[]string{
 if len(specEnv)==0&&len(dependencyEnv)==0{return nil}
 base:=specEnv
 if len(base)==0{base=os.Environ()}
 return append(append([]string{},base...),dependencyEnv...)
}
