//go:build darwin

package session

import "testing"

func TestDarwinElapsed(t *testing.T) {
    for s,want:=range map[string]int64{"01:02":62,"01:02:03":3723,"2-01:02:03":176523} {
        got,err:=parseDarwinElapsed(s)
        if err!=nil||got!=want{t.Fatalf("%q: got=%d err=%v, want %d",s,got,err,want)}
    }
}
func TestDarwinProcessTree(t *testing.T) {
    raw:="100 1 100 Ss 00:03 1.5 24 bash bash -l\n101 100 100 S 00:01 0.0 12 sleep sleep 1\n"
    nodes:=parseDarwinProcessTable(raw,100)
    if len(nodes)!=2||nodes[0].PID!=100||nodes[1].PID!=101||nodes[1].Depth!=1{
        t.Fatalf("unexpected tree: %+v",nodes)
    }
}
