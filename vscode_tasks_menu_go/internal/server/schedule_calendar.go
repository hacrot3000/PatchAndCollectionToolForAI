package server

import (
 "errors"
 "fmt"
 "strconv"
 "strings"
 "time"
)

// Calendar gates prevent automatic execution. Manual Run deliberately ignores
// this window, so an administrator can re-run a missed schedule on demand.
func validateScheduleCalendar(def scheduleDefinition)error{
 if len(def.ExcludeDates)>366{return errors.New("exclude_dates exceeds 366 days")}
 for _,date:=range def.ExcludeDates{
  if _,err:=time.Parse("2006-01-02",date);err!=nil{return fmt.Errorf("invalid exclude date %q",date)}
 }
 seen:=map[int]bool{}
 for _,weekday:=range def.AllowedWeekdays{
  if weekday<0||weekday>6||seen[weekday]{return errors.New("allowed_weekdays must contain unique numbers 0-6")}
  seen[weekday]=true
 }
 for _,value:=range []string{def.WindowStart,def.WindowEnd}{
  if value==""{continue}
  if len(value)!=5||value[2]!=':'{return errors.New("time window must use HH:MM")}
  hour,herr:=strconv.Atoi(value[:2]);minute,merr:=strconv.Atoi(value[3:])
  if herr!=nil||merr!=nil||hour<0||hour>23||minute<0||minute>59{return errors.New("time window must use valid HH:MM")}
 }
 return nil
}
func scheduleCalendarAllows(def scheduleDefinition,now time.Time)bool{
 loc:=time.Local
 if def.Timezone!=""&&def.Timezone!="Local"{
  if zone,err:=time.LoadLocation(def.Timezone);err==nil{loc=zone}
 }
 local:=now.In(loc)
 for _,date:=range def.ExcludeDates{if local.Format("2006-01-02")==date{return false}}
 if len(def.AllowedWeekdays)>0{
  found:=false
  for _,day:=range def.AllowedWeekdays{if int(local.Weekday())==day{found=true;break}}
  if !found{return false}
 }
 clock:=local.Format("15:04")
 if def.WindowStart!=""&&def.WindowEnd!=""&&strings.Compare(def.WindowStart,def.WindowEnd)>0{
  // Overnight window, e.g. 22:00-06:00
  return clock>=def.WindowStart||clock<def.WindowEnd
 }
 if def.WindowStart!=""&&clock<def.WindowStart{return false}
 if def.WindowEnd!=""&&clock>=def.WindowEnd{return false}
 return true
}
