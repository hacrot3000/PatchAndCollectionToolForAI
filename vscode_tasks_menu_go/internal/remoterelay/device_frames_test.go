package remoterelay

import (
    "encoding/json"
    "strings"
    "testing"
)

func TestRelayAgentFrameStrictCanonicalJSON(t *testing.T) {
    good := []any{
        map[string]string{"type":"ping","device_lease":"lease"},
        map[string]string{"type":"probe_rejected","probe_id":"probe","device_lease":"lease"},
        map[string]string{"type":"login_rejected","probe_id":"probe","device_lease":"lease"},
        map[string]string{"type":"echo_rejected","probe_id":"probe","device_lease":"lease"},
        map[string]string{"type":"login_ack","probe_id":"probe","ciphertext":"abc","device_lease":"lease"},
        map[string]string{"type":"echo_ack","probe_id":"probe","ciphertext":"abc","device_lease":"lease"},
        map[string]any{
            "type":"probe_ack","probe_id":"probe","client_nonce":"nonce",
            "public_key":"public","signature":"signature","device_lease":"lease",
            "crypto":map[string]string{"identity":"identity","ephemeral":"ephemeral",
                "signature":"signature","encrypted_ready":"encrypted"},
        },
    }
    for _, value := range good {
        raw,err:=json.Marshal(value)
        if err!=nil{t.Fatal(err)}
        if _,err:=decodeAgentRelayFrame(raw);err!=nil {
            t.Fatalf("legitimate Agent frame rejected: %s: %v",raw,err)
        }
    }
    bad := []string{
        "{\"type\":\"ping\",\"type\":\"ping\",\"device_lease\":\"lease\"}",
        "{\"type\":\"ping\",\"\\u0074ype\":\"ping\",\"device_lease\":\"lease\"}",
        "{\"type\":\"ping\",\"Type\":\"ping\",\"device_lease\":\"lease\"}",
        "{\"type\":\"ping\",\"device_lease\":\"lease\",\"admin\":true}",
        "{\"type\":\"ping\",\"device_lease\":null}",
        "{\"type\":\"ping\",\"device_lease\":\"lease\"}{}",
        "{\"type\":\"login_ack\",\"probe_id\":\"p\",\"device_lease\":\"l\"}",
        "{\"type\":\"login_ack\",\"probe_id\":\"p\",\"device_lease\":\"l\",\"ciphertext\":\"x\",\"session_token\":\"fake\"}",
        "{\"type\":\"probe_ack\",\"probe_id\":\"p\",\"client_nonce\":\"n\",\"public_key\":\"k\",\"signature\":\"s\",\"device_lease\":\"l\",\"crypto\":{\"identity\":\"i\",\"ephemeral\":\"e\",\"signature\":\"s\",\"signature\":\"s\",\"encrypted_ready\":\"r\"}}",
        "{\"type\":\"probe_ack\",\"probe_id\":\"p\",\"client_nonce\":\"n\",\"public_key\":\"k\",\"signature\":\"s\",\"device_lease\":\"l\",\"crypto\":{\"identity\":\"i\",\"ephemeral\":\"e\",\"signature\":\"s\",\"encrypted_ready\":\"r\",\"role\":\"admin\"}}",
        "{\"type\":\"app_action\",\"device_lease\":\"lease\"}",
        "{\"type\":\"ping\",\"device_lease\":[\"lease\"]}",
        "{\"type\":\"ping\",\"device_lease\":\"lease\",\"ciphertext\":\"x\"}",
    }
    for _, value:=range bad {
        if _,err:=decodeAgentRelayFrame([]byte(value));err==nil {
            t.Fatalf("ambiguous/unknown Agent frame accepted: %s",value)
        }
    }
}

func TestRelayDeviceProofRejectsDuplicateIdentityOrExtraAuthority(t *testing.T) {
    valid := DeviceProof{
        DeviceID:strings.Repeat("a",64),PublicKey:"publickey",
        Audience:"https://relay.example.com",Challenge:"challenge",
        DeviceLease:"signed_lease",Timestamp:1790000000,
        Nonce:strings.Repeat("b",32),Signature:"signature",
    }
    raw,err:=json.Marshal(valid)
    if err!=nil{t.Fatal(err)}
    decoded,err:=decodeDeviceConnectProof(raw)
    if err!=nil || decoded!=valid{t.Fatalf("canonical proof rejected: %v",err)}
    bad := []string{
        strings.Replace(string(raw), "\"device_id\":", "\"DEVICE_ID\":", 1),
        strings.Replace(string(raw), "\"device_id\":", "\"device_id\":\"evil\",\"device_id\":", 1),
        strings.Replace(string(raw), "\"device_id\":", "\"\\u0064evice_id\":\"evil\",\"device_id\":", 1),
        strings.TrimSuffix(string(raw),"}")+",\"admin\":true}",
        strings.Replace(string(raw), "\"timestamp\":1790000000", "\"timestamp\":\"1790000000\"", 1),
        strings.Replace(string(raw), "\"timestamp\":1790000000", "\"timestamp\":1.79e9", 1),
        string(raw)+"{}",
    }
    for _, frame:=range bad {
        if _,err:=decodeDeviceConnectProof([]byte(frame));err==nil {
            t.Fatalf("non-canonical signed Agent proof accepted: %s",frame)
        }
    }
}
