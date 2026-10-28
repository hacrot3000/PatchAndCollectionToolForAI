package remoterelay

import (
 "testing"
 "time"
)
func TestReachabilityProbeIsBoundToAuthenticatedSocket(t *testing.T) {
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com");if err!=nil{t.Fatal(err)}
 proof:=newAgentProof(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 if _,err:=r.BeginProbe(proof.DeviceID);err==nil{t.Fatal("probe without browser reservation accepted")}
 release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now);if err!=nil{t.Fatal(err)}
 result,err:=r.BeginProbe(proof.DeviceID);if err!=nil{t.Fatal(err)}
 if _,err:=r.BeginProbe(proof.DeviceID);err==nil{t.Fatal("second probe accepted")}
 if _,ok:=r.TakeProbe(proof.DeviceID,"forged");ok{t.Fatal("foreign Agent took probe")}
 id,ok:=r.TakeProbe(proof.DeviceID,proof.Nonce);if !ok{t.Fatal("valid Agent could not take probe")}
 if _,ok:=r.TakeProbe(proof.DeviceID,proof.Nonce);ok{t.Fatal("probe redelivered")}
 if r.AckProbe(proof.DeviceID,proof.Nonce,"invalid"){t.Fatal("wrong ACK accepted")}
 if !r.AckProbe(proof.DeviceID,proof.Nonce,id){t.Fatal("valid ACK rejected")}
 if !<-result {t.Fatal("probe failed after verified ACK")}
 if r.AckProbe(proof.DeviceID,proof.Nonce,id){t.Fatal("duplicate ACK accepted")}
 release()
 again,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now);if err!=nil{t.Fatal(err)}
 pending,err:=r.BeginProbe(proof.DeviceID);if err!=nil{t.Fatal(err)}
 r.Unregister(proof.DeviceID,proof.Nonce)
 if <-pending {t.Fatal("disconnect incorrectly completed pending probe")}
 again()
}
