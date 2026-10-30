package remotetransport

import (
    "testing"
    "time"
)

func requireChannelClosed(t *testing.T, ch *Channel){
    t.Helper()
    select {
    case <-ch.Done():
    case <-time.After(time.Second):t.Fatal("poisoned channel did not signal cancellation")
    }
    if ch.Established() || ch.BoundDeviceID()!="" || ch.BrowserTicketBinding()!="" {
        t.Fatal("closed channel still retained authenticated identity bindings")
    }
}

func TestE2EEChannelDoneOnCloseAndMalformedOrReplay(t *testing.T) {
    select {
    case <-(&Channel{}).Done():
    default:t.Fatal("zero-valued channel exposes never-closing Done channel")
    }
    var nilChannel *Channel
    select {
    case <-nilChannel.Done():
    default:t.Fatal("nil channel exposes never-closing Done channel")
    }

    for _,scenario:=range []string{"close","tampered tag","replay","short frame","out of order"} {
        t.Run(scenario,func(t *testing.T){
            client,agent,_:=participants(t)
            done:=agent.Done()
            select {
            case <-done:t.Fatal("authenticated E2EE channel already marked closed")
            default:
            }
            first,err:=client.Seal([]byte("first"))
            if err!=nil{t.Fatal(err)}
            second,err:=client.Seal([]byte("second"))
            if err!=nil{t.Fatal(err)}
            switch scenario {
            case "close":
                agent.Close()
                agent.Close()
            case "tampered tag":
                corrupt:=append([]byte(nil),first...)
                corrupt[len(corrupt)-1]^=0x40
                if _,err:=agent.Open(corrupt);err==nil{t.Fatal("bad tag accepted")}
            case "replay":
                if _,err:=agent.Open(first);err!=nil{t.Fatal(err)}
                if _,err:=agent.Open(first);err==nil{t.Fatal("replay accepted")}
            case "short frame":
                if _,err:=agent.Open([]byte{1,2});err==nil{t.Fatal("short frame accepted")}
            case "out of order":
                if _,err:=agent.Open(second);err==nil{t.Fatal("out-of-order record accepted")}
            }
            requireChannelClosed(t,agent)
            select {
            case <-done:
            case <-time.After(time.Second):t.Fatal("previously returned Done channel not closed")
            }
            if _,err:=agent.Seal([]byte("after revoke"));err==nil{
                t.Fatal("closed Agent can send encrypted frame")
            }
            client.Close()
        })
    }
}
