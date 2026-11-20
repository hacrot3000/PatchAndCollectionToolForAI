package server

import "testing"

func TestScheduleRetryExitCodeAllowlist(t *testing.T){
 cases:=[]struct{
  name string
  code,attempt,max int
  allow []int
  canceled,want bool
 }{
  {"legacy retries any nonzero",42,1,3,nil,false,true},
  {"allowlisted exit",42,1,3,[]int{1,42},false,true},
  {"other exit not retried",2,1,3,[]int{42},false,false},
  {"zero exit never retried",0,1,3,[]int{0},false,false},
  {"cancelled never retried",-1,1,3,[]int{-1},true,false},
  {"final attempt not retried",42,3,3,[]int{42},false,false},
  {"negative process failure selected",-1,1,2,[]int{-1},false,true},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   got:=scheduleShouldRetry(tc.code,tc.attempt,tc.max,tc.allow,tc.canceled)
   if got!=tc.want{t.Fatalf("got %v, want %v",got,tc.want)}
  })
 }
}
