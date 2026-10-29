// Command taskdeck_remote_login is an independently built, local, one-shot
// TaskDeck password probe. It deliberately never fetches code or TaskDeck
// credentials from the potentially compromised PHP Portal.
//
// It is NOT a Remote Control client and obtains no HTTP/session token.
package main

import (
 "bufio"
 "context"
 "crypto/rand"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "path/filepath"
 "os"
 "os/exec"
 "regexp"
 "strings"
 "syscall"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "github.com/coder/websocket"
)

var devicePattern=regexp.MustCompile("^[a-f0-9]{64}$")
var originPattern=regexp.MustCompile("^https://[a-zA-Z0-9][a-zA-Z0-9.-]*(?::[0-9]{2,5})?$")

func validOrigin(raw string) bool {
 if !originPattern.MatchString(raw){return false}
 u,err:=url.Parse(raw)
 return err==nil && u.Hostname()!="" && u.User==nil &&
  u.Path=="" && u.RawQuery=="" && u.Fragment==""
}


// loadPinnedDevice requires an owner-only, out-of-band trust anchor on the
// CLIENT host. A Device ID copied only from a hacked Portal is not trusted:
// the attacker could substitute a device whose Ed25519 private key it owns.
// In addition to 0700/0600 permissions, confirm the effective owner. This
// matters if a user mistakenly runs the client with elevated privileges:
// a 0600 file owned by a DIFFERENT tenant is not the operator's own pin.
//
// The trusted login client is built for Linux/macOS, where Stat_t.Uid,
// O_NOFOLLOW and O_NONBLOCK are available. Open the actual pin descriptor
// without following a symlink: Lstat+ReadFile was vulnerable to path swaps.
// O_NONBLOCK prevents an attacker-created FIFO from hanging the CLI before
// its descriptor can be rejected by the regular-file type check.
func ownerOnlyPin(info os.FileInfo, directory bool) bool {
 if info==nil || info.Mode().Perm()&0077!=0{return false}
 stat,ok:=info.Sys().(*syscall.Stat_t)
 if !ok || stat.Uid!=uint32(os.Geteuid()){return false}
 if directory{return info.IsDir()}
 return info.Mode().IsRegular()
}

func loadPinnedDevice(path string) (string,error) {
 if !filepath.IsAbs(path) {return "",errors.New("Device ID pin path must be absolute")}
 parent:=filepath.Dir(path)
 parentInfo,err:=os.Lstat(parent)
 if err!=nil || !ownerOnlyPin(parentInfo,true) {
  return "",errors.New("Device ID pin directory must belong to current OS user and be private (0700)")
 }
 fd,err:=syscall.Open(path,syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if err!=nil{return "",errors.New("Device ID pin must be a non-symlink, readable private file")}
 file:=os.NewFile(uintptr(fd),path)
 defer file.Close()
 info,err:=file.Stat()
 if err!=nil || !ownerOnlyPin(info,false) {
  return "",errors.New("Device ID pin must belong to current OS user and be a private regular file (0600)")
 }
 if info.Size()<64 || info.Size()>65 {return "",errors.New("invalid pinned Device ID length")}
 raw,err:=io.ReadAll(io.LimitReader(file,66))
 if err!=nil{return "",err}
 if len(raw)<64 || len(raw)>65{return "",errors.New("invalid pinned Device ID length")}
 id:=strings.TrimSuffix(string(raw),"\n")
 if !devicePattern.MatchString(id) {return "",errors.New("invalid pinned Device ID")}
 return id,nil
}

type clientSettings struct { Relay,Portal,DeviceID string }

func (s clientSettings) validate() error {
 if !validOrigin(s.Relay) || !validOrigin(s.Portal) || !devicePattern.MatchString(s.DeviceID) {
  return errors.New("relay/portal must be exact HTTPS origins and device must be a 64-character pinned fingerprint")
 }
 return nil
}

func boundedRead(conn *websocket.Conn,ctx context.Context) ([]byte,error) {
 kind,body,err:=conn.Read(ctx)
 if err!=nil{return nil,err}
 if kind!=websocket.MessageText{return nil,errors.New("unexpected binary WebSocket message")}
 if len(body)>12288{return nil,errors.New("oversized Relay response")}
 return body,nil
}

type relayProof struct {
 TicketValid bool `json:"ticket_valid"`
 AgentReachable bool `json:"agent_reachable"`
 RemoteControlAvailable bool `json:"remote_control_available"`
 DeviceID string `json:"device_id"`
 AgentProof struct {
  ProbeID string `json:"probe_id"`
  Crypto struct {
   Identity string `json:"identity"`
   Ephemeral string `json:"ephemeral"`
   Signature string `json:"signature"`
   EncryptedReady string `json:"encrypted_ready"`
  } `json:"crypto"`
 } `json:"agent_proof"`
}

func decodeBoundedBase64(value string,max int) ([]byte,error) {
 if len(value)>max*2 {return nil,errors.New("oversized encoded proof")}
 result,err:=base64.StdEncoding.Strict().DecodeString(value)
 if err!=nil || len(result)>max{return nil,errors.New("invalid encoded proof")}
 return result,nil
}

// sealLoginAfterAgentProof can request a TaskDeck password only from an
// authenticated, live E2EE Channel. An attacker-controlled Portal ticket or
// spoofed Relay response cannot create this Channel: the CLI first verifies
// its independently pinned Ed25519 Device ID and the encrypted ready record.
// This narrow helper makes the no-password-before-proof invariant testable.
func sealLoginAfterAgentProof(ch *remotetransport.Channel, username string,
 readPassword func()(string,error)) ([]byte,error) {
 if ch==nil || !ch.Established() || readPassword==nil {
  return nil,errors.New("trusted Agent E2EE proof required before password entry")
 }
 password,err:=readPassword()
 if err!=nil{return nil,err}
 if len(password)==0 || len(password)>4096 {
  password=""
  return nil,errors.New("invalid TaskDeck password length")
 }
 frame,err:=remotetransport.RemoteLoginRequestFrame(ch,username,password)
 password=""
 return frame,err
}

// The PHP origin can be compromised and may lie about every field. Only
// the Ed25519 identity locally pinned by Device ID, its signed ECDH
// transcript and the authenticated AES-GCM ready frame establish trust.
func verifyAgentProof(body []byte, deviceID,nonce string, browser *remotetransport.Ephemeral) (*remotetransport.Channel,error) {
 var reply relayProof
 if err:=json.Unmarshal(body,&reply);err!=nil{return nil,err}
 if !reply.TicketValid || !reply.AgentReachable || reply.RemoteControlAvailable ||
  reply.DeviceID!=deviceID || !regexp.MustCompile("^[a-f0-9]{32}$").MatchString(reply.AgentProof.ProbeID) {
  return nil,errors.New("Relay did not return an authorized, bounded identity proof")
 }
 identity,err:=decodeBoundedBase64(reply.AgentProof.Crypto.Identity,32)
 if err!=nil{return nil,err}
 ephemeral,err:=decodeBoundedBase64(reply.AgentProof.Crypto.Ephemeral,65)
 if err!=nil{return nil,err}
 signature,err:=decodeBoundedBase64(reply.AgentProof.Crypto.Signature,64)
 if err!=nil{return nil,err}
 ready,err:=decodeBoundedBase64(reply.AgentProof.Crypto.EncryptedReady,1024)
 if err!=nil{return nil,err}
 ch,err:=remotetransport.AcceptHello(browser,deviceID,reply.AgentProof.ProbeID,nonce,
  remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:signature})
 if err!=nil{return nil,err}
 clear,err:=ch.Open(ready)
 if err!=nil || string(clear)!="taskdeck-e2ee-ready-v1" {
  ch.Close()
  return nil,errors.New("Agent did not prove possession of session encryption key")
 }
 return ch,nil
}

func loginReply(body []byte,ch *remotetransport.Channel) (bool,error) {
 var reply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(body,&reply);err!=nil{return false,err}
 if reply.Type!="e2ee_login_result" || reply.RemoteControlAvailable {
  return false,errors.New("Relay attempted a forbidden application response")
 }
 ciphertext,err:=decodeBoundedBase64(reply.Ciphertext,2048)
 if err!=nil{return false,err}
 return remotetransport.OpenRemoteLoginResult(ch,ciphertext)
}

func probe(ctx context.Context,s clientSettings, ticket,nonce,username string,
 browser *remotetransport.Ephemeral, readPassword func() (string,error)) (bool,error) {
 if err:=s.validate();err!=nil{return false,err}
 if len(ticket)<100 || len(ticket)>4096 || len(username)==0 || len(username)>128 ||
  readPassword==nil {return false,errors.New("invalid ticket or TaskDeck login input")}
 conn,_,err:=websocket.Dial(ctx,strings.Replace(s.Relay,"https://","wss://",1)+"/v1/browser",
  &websocket.DialOptions{HTTPHeader:http.Header{"Origin":[]string{s.Portal}}})
 if err!=nil{return false,err}
 defer conn.CloseNow()
 conn.SetReadLimit(12288)
 first,err:=json.Marshal(map[string]string{
  "ticket":ticket,"client_nonce":nonce,
  "browser_key":base64.StdEncoding.EncodeToString(browser.Public),
 })
 if err!=nil{return false,err}
 if err:=conn.Write(ctx,websocket.MessageText,first);err!=nil{return false,err}
 response,err:=boundedRead(conn,ctx)
 if err!=nil{return false,err}
 ch,err:=verifyAgentProof(response,s.DeviceID,nonce,browser)
 if err!=nil{return false,err}
 defer ch.Close()
 // A malicious PHP Portal cannot trigger the local TTY password prompt
 // until the independently pinned Agent has completed authenticated E2EE.
 encrypted,err:=sealLoginAfterAgentProof(ch,username,readPassword)
 if err!=nil{return false,err}
 frame,err:=json.Marshal(map[string]string{
  "type":"e2ee_login","ciphertext":base64.StdEncoding.EncodeToString(encrypted),
 })
 if err!=nil{return false,err}
 if err:=conn.Write(ctx,websocket.MessageText,frame);err!=nil{return false,err}
 response,err=boundedRead(conn,ctx)
 if err!=nil{return false,err}
 return loginReply(response,ch)
}

func ttyLine(tty *os.File,reader *bufio.Reader,label string) (string,error) {
 fmt.Fprint(tty,label)
 line,err:=reader.ReadString('\n')
 if err!=nil{return "",err}
 return strings.TrimSuffix(strings.TrimSuffix(line,"\n"),"\r"),nil
}

// Never fall back to echoing a TaskDeck password on stdin or command line.
// The local stty utility is invoked without a shell. Restore exact previous
// terminal settings even if reading fails. Linux/macOS TTY only.
func ttyPassword(tty *os.File,reader *bufio.Reader) (string,error) {
 get:=exec.Command("stty","-g");get.Stdin=tty
 saved,err:=get.Output()
 if err!=nil{return "",errors.New("unable to inspect terminal echo; refusing password input")}
 old:=strings.TrimSpace(string(saved))
 hide:=exec.Command("stty","-echo");hide.Stdin=tty
 if err:=hide.Run();err!=nil{return "",errors.New("unable to disable terminal echo")}
 defer func(){restore:=exec.Command("stty",old);restore.Stdin=tty;_ = restore.Run()}()
 fmt.Fprint(tty,"Mật khẩu TaskDeck (chỉ nhập ở client này): ")
 value,err:=reader.ReadString('\n')
 fmt.Fprintln(tty)
 if err!=nil{return "",err}
 return strings.TrimSuffix(strings.TrimSuffix(value,"\n"),"\r"),nil
}

func run() error {
 var s clientSettings
 var pinPath string
 flag.StringVar(&pinPath,"pin-file","","Absolute path to owner-only (0600) Device ID pin verified outside Portal")
 flag.StringVar(&s.Relay,"relay","","Trusted Relay HTTPS origin")
 flag.StringVar(&s.Portal,"portal","","Portal HTTPS origin (for Relay Origin header only)")
 flag.StringVar(&s.DeviceID,"device","","Owner-verified TaskDeck Device ID (64 hex characters)")
 flag.Parse()
 if err:=s.validate();err!=nil{return err}
 trustedPin,err:=loadPinnedDevice(pinPath)
 if err!=nil{return fmt.Errorf("independent TaskDeck identity pin required: %w",err)}
 if trustedPin!=s.DeviceID {return errors.New("Device ID mismatch: Portal or command-line value disagrees with independent local pin")}
 tty,err:=os.OpenFile("/dev/tty",os.O_RDWR,0)
 if err!=nil{return errors.New("interactive local terminal is required; TaskDeck password cannot be supplied via argv, env or pipe")}
 defer tty.Close()
 reader:=bufio.NewReader(tty)
 fmt.Fprintln(tty,"TaskDeck E2EE login-only diagnostic; no Remote Control.")
 fmt.Fprintln(tty,"The PHP Portal may be compromised. NEVER enter your TaskDeck password in its page.")
 username,err:=ttyLine(tty,reader,"Tài khoản TaskDeck: ")
 if err!=nil{return err}
 // No password prompt here: PHP and Relay have not yet proved the remote
 // endpoint is the owner-pinned TaskDeck Agent. The password prompt runs
 // only AFTER verifying the Agent-signed E2EE ready packet.
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{return err}
 var random [16]byte
 if _,err:=io.ReadFull(rand.Reader,random[:]);err!=nil{return err}
 nonce:=hex.EncodeToString(random[:])
 fmt.Fprintln(tty,"Device ID cần ghim xác minh trực tiếp tại TaskDeck:",s.DeviceID)
 fmt.Fprintln(tty,"Client nonce:",nonce)
 fmt.Fprintln(tty,"Client public key:",base64.StdEncoding.EncodeToString(browser.Public))
 fmt.Fprintln(tty,"Nhập hai giá trị công khai ở trên vào biểu mẫu cấp ticket trên Portal,")
 fmt.Fprintln(tty,"sau đó sao chép ticket trả về. Không nhập mật khẩu TaskDeck lên Portal.")
 ticket,err:=ttyLine(tty,reader,"Ticket (30 giây hiệu lực): ")
 if err!=nil{return err}
 ctx,cancel:=context.WithTimeout(context.Background(),70*time.Second)
 defer cancel()
 ok,err:=probe(ctx,s,strings.TrimSpace(ticket),nonce,username,browser,
  func()(string,error){return ttyPassword(tty,reader)})
 if err!=nil{return fmt.Errorf("remote login diagnostic failed: %w",err)}
 if !ok {return errors.New("TaskDeck rejected the credentials (no remote session opened)")}
 fmt.Fprintln(tty,"TaskDeck xác thực mật khẩu thành công; chỉ xác minh E2EE, không có quyền điều khiển Remote.")
 return nil
}

func main() {
 if err:=run();err!=nil{
  fmt.Fprintln(os.Stderr,err)
  os.Exit(1)
 }
}
