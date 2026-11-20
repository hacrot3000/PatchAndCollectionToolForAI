//go:build darwin

package session

import (
    "bytes"
    "fmt"
    "os/exec"
    "sort"
    "strconv"
    "strings"
)

// BSD ps uses elapsed time strings, unlike Linux's etimes seconds.
func collectProcessTree(rootPID int) ([]ProcessInfo,error) {
    if rootPID <= 0 {return nil,fmt.Errorf("invalid session process id")}
    cmd:=exec.Command("ps","-axo","pid=,ppid=,pgid=,stat=,etime=,%cpu=,rss=,comm=,args=")
    buffer:= &darwinProcessBuffer{limit:2<<20}
    cmd.Stdout=buffer
    var stderr bytes.Buffer
    cmd.Stderr=&stderr
    if err:=cmd.Run();err!=nil{return nil,fmt.Errorf("read macOS process tree: %w: %s",err,strings.TrimSpace(stderr.String()))}
    if buffer.truncated{return nil,fmt.Errorf("process table exceeds 2 MiB")}
    return parseDarwinProcessTable(string(buffer.data),rootPID),nil
}

type darwinProcessBuffer struct {data []byte;limit int;truncated bool}
func (b *darwinProcessBuffer) Write(p []byte)(int,error){
    n:=len(p)
    if len(b.data)+n>b.limit {
        remaining:=b.limit-len(b.data)
        if remaining>0 {b.data=append(b.data,p[:remaining]...)}
        b.truncated=true
        return n,nil
    }
    b.data=append(b.data,p...)
    return n,nil
}

func parseDarwinElapsed(value string)(int64,error) {
    days:=int64(0)
    if strings.Contains(value,"-"){
        split:=strings.SplitN(value,"-",2)
        var err error
        days,err=strconv.ParseInt(split[0],10,64)
        if err!=nil||days<0{return 0,fmt.Errorf("invalid elapsed days")}
        value=split[1]
    }
    parts:=strings.Split(value,":")
    if len(parts)!=2 && len(parts)!=3{return 0,fmt.Errorf("invalid elapsed time")}
    nums:=make([]int64,len(parts))
    for i,s:=range parts{
        n,err:=strconv.ParseInt(s,10,64)
        if err!=nil||n<0{return 0,fmt.Errorf("invalid elapsed time")}
        nums[i]=n
    }
    seconds:=days*86400
    if len(nums)==3 {return seconds+nums[0]*3600+nums[1]*60+nums[2],nil}
    return seconds+nums[0]*60+nums[1],nil
}

func parseDarwinProcessTable(raw string,rootPID int) []ProcessInfo {
    all:=map[int]ProcessInfo{}
    children:=map[int][]int{}
    for _,line:=range strings.Split(raw,"\n"){
        f:=strings.Fields(line)
        if len(f)<9{continue}
        pid,e1:=strconv.Atoi(f[0]);ppid,e2:=strconv.Atoi(f[1]);pgid,e3:=strconv.Atoi(f[2])
        elapsed,e4:=parseDarwinElapsed(f[4]);cpu,e5:=strconv.ParseFloat(f[5],64);rss,e6:=strconv.ParseInt(f[6],10,64)
        if e1!=nil||e2!=nil||e3!=nil||e4!=nil||e5!=nil||e6!=nil||pid<=0{continue}
        all[pid]=ProcessInfo{
            PID:pid,PPID:ppid,PGID:pgid,State:f[3],
            ElapsedSeconds:elapsed,CPUPercent:cpu,RSSBytes:rss*1024,
            Command:f[7],Args:strings.Join(f[8:]," "),
        }
        children[ppid]=append(children[ppid],pid)
    }
    if _,ok:=all[rootPID];!ok{return []ProcessInfo{}}
    for parent:=range children{sort.Ints(children[parent])}
    result:=[]ProcessInfo{}
    seen:=map[int]bool{}
    var walk func(int,int)
    walk=func(pid,depth int){
        if seen[pid]{return}
        seen[pid]=true
        item,ok:=all[pid];if !ok{return}
        item.Depth=depth;result=append(result,item)
        for _,child:=range children[pid]{walk(child,depth+1)}
    }
    walk(rootPID,0)
    return result
}
