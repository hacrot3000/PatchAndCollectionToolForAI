package remoterelay

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTicketReplayLedgerSurvivesRelayRestart(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {t.Fatal(err)}
	key := base64.StdEncoding.EncodeToString(public)
	origin := "https://portal.example.com"
	dir := filepath.Join(t.TempDir(), "replay")
	ledger, err := NewReplayLedger(dir)
	if err != nil {t.Fatal(err)}
	verifier, err := NewVerifier(key,origin)
	if err != nil {t.Fatal(err)}
	if err := verifier.ConfigureReplayLedger(ledger); err != nil {t.Fatal(err)}

	now := time.Now()
	claims := baselineTicketClaims(now)
	token := tokenFor(t,private,claims)
	if _,err:=verifier.Consume(token,now);err!=nil {t.Fatal(err)}
	if _,err:=verifier.Consume(token,now);err==nil {t.Fatal("same-process replay was accepted")}

	onDisk,err:=os.ReadFile(filepath.Join(dir,claims.JTI))
	if err!=nil {t.Fatal(err)}
	if strings.Contains(string(onDisk),token) || len(onDisk)>24 {
		t.Fatal("replay ledger retained a signed token or excessive data")
	}
	// Simulate Relay process death: reconstruct the verifier and ledger from
	// the same directory. The ticket remains burned until it expires.
	ledger2,err:=NewReplayLedger(dir)
	if err!=nil{t.Fatal(err)}
	restarted,err:=NewVerifier(key,origin)
	if err!=nil{t.Fatal(err)}
	if err:=restarted.ConfigureReplayLedger(ledger2);err!=nil{t.Fatal(err)}
	if _,err:=restarted.Consume(token,now);err==nil {
		t.Fatal("Relay restart allowed a previously consumed ticket")
	}
	claims.JTI=strings.Repeat("f",32)
	if _,err:=restarted.Consume(tokenFor(t,private,claims),now);err!=nil {
		t.Fatalf("fresh JTI denied: %v",err)
	}
	if err:=restarted.ConfigureReplayLedger(ledger);err==nil{
		t.Fatal("replay backend was replaced after consumption")
	}
	ledger2.mu.Lock()
	err=ledger2.pruneLocked(now.Unix()+71)
	ledger2.mu.Unlock()
	if err!=nil{t.Fatal(err)}
	files,err:=os.ReadDir(dir)
	if err!=nil{t.Fatal(err)}
	if len(files)!=0 {t.Fatal("expired replay records were not pruned")}
}

func TestReplayLedgerRejectsUnsafeStorageAndConcurrentJTI(t *testing.T){
	root:=t.TempDir()
	relative:="relative-replay-state"
	if _,err:=NewReplayLedger(relative);err==nil{t.Fatal("relative replay directory accepted")}
	dir:=filepath.Join(root,"ledger")
	ledger,err:=NewReplayLedger(dir)
	if err!=nil{t.Fatal(err)}
	if err:=os.Chmod(dir,0755);err!=nil{t.Fatal(err)}
	if _,err:=NewReplayLedger(dir);err==nil{t.Fatal("public replay directory accepted")}
	if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
	link:=filepath.Join(root,"linked")
	if err:=os.Symlink(dir,link);err==nil {
		if _,err:=NewReplayLedger(link);err==nil{t.Fatal("symlinked replay directory accepted")}
	}
	now:=time.Now()
	jti:=strings.Repeat("d",32)
	results:=make(chan bool,40)
	for i:=0;i<40;i++ {
		go func(){results<-ledger.Consume(jti,now.Unix()+30,now)==nil}()
	}
	success:=0
	for i:=0;i<40;i++ {if <-results{success++}}
	if success!=1 {t.Fatalf("JTI consumed %d times, expected once",success)}
}

func baselineTicketClaims(now time.Time) Claims {
	return Claims{AuthEpoch:1,
		V:2,BrowserBinding:strings.Repeat("f",64),Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
		Subject:"42",DeviceID:strings.Repeat("a",64),Role:"viewer",
		IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("e",32),
	}
}

func TestReplayLedgerFailsClosedOnCorruptRecordAfterCrash(t *testing.T) {
	// A crash during O_EXCL write can leave an empty JTI entry. The entry
	// must remain burned and prevent boot, rather than be deleted at startup.
	for _, payload := range []string{"", "invalid", "999999999999999999999999999999"} {
		dir:=filepath.Join(t.TempDir(),"replay")
		if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
		id:=strings.Repeat("a",32)
		file:=filepath.Join(dir,id)
		if err:=os.WriteFile(file,[]byte(payload),0600);err!=nil{t.Fatal(err)}
		if _,err:=NewReplayLedger(dir);err==nil {
			t.Fatalf("corrupt replay entry %q silently accepted",payload)
		}
		if _,err:=os.Stat(file);err!=nil {
			t.Fatal("invalid replay entry was removed and could permit token reuse")
		}
	}
}

func TestReplayLedgerRejectsSymlinkAndReadableJTI(t *testing.T) {
	for _,mode := range []string{"symlink","readable"} {
		dir:=filepath.Join(t.TempDir(),"ledger")
		if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
		path:=filepath.Join(dir,strings.Repeat("b",32))
		if mode=="symlink" {
			target:=filepath.Join(dir,"target")
			if err:=os.WriteFile(target,[]byte("9999999999"),0600);err!=nil{t.Fatal(err)}
			if err:=os.Symlink(target,path);err!=nil{t.Skipf("symlink not supported: %v",err)}
		} else {
			if err:=os.WriteFile(path,[]byte("9999999999"),0644);err!=nil{t.Fatal(err)}
		}
		if _,err:=NewReplayLedger(dir);err==nil {
			t.Fatalf("%s replay record accepted",mode)
		}
	}
}

// ownerOverrideReplayInfo models a private 0700/0600 inode whose OS owner is
// NOT the Relay process. No root-only chown or test-runner privileges required.
type ownerOverrideReplayInfo struct {
 os.FileInfo
 uid uint32
}
func (f ownerOverrideReplayInfo) Sys() interface{} {
 original,ok:=f.FileInfo.Sys().(*syscall.Stat_t)
 if !ok {return nil}
 snapshot:=*original
 snapshot.Uid=f.uid
 return &snapshot
}
func TestReplayLedgerRequiresActualEffectiveOSOwnerNotOnlyPermissions(t *testing.T) {
 dir:=t.TempDir()
 dirInfo,err:=os.Stat(dir)
 if err!=nil{t.Fatal(err)}
 if !ownedReplayEntry(dirInfo,true){t.Fatal("private process-owned test dir rejected")}
 foreign:=ownerOverrideReplayInfo{FileInfo:dirInfo,uid:uint32(os.Geteuid())+1}
 if ownedReplayEntry(foreign,true){
  t.Fatal("P1: Relay accepted 0700 replay directory controlled by another user")
 }
 rec:=filepath.Join(dir,strings.Repeat("a",32))
 if err:=os.WriteFile(rec,[]byte("9999999999"),0600);err!=nil{t.Fatal(err)}
 fileInfo,err:=os.Stat(rec)
 if err!=nil{t.Fatal(err)}
 if !ownedReplayEntry(fileInfo,false){t.Fatal("private owner-only JTI rejected")}
 foreignJTI:=ownerOverrideReplayInfo{FileInfo:fileInfo,uid:uint32(os.Geteuid())+1}
 if ownedReplayEntry(foreignJTI,false){
  t.Fatal("P1: Relay accepted 0600 JTI marker owned by another user")
 }
 if ownedReplayEntry(dirInfo,false){t.Fatal("directory accepted as JTI marker")}
 if ownedReplayEntry(fileInfo,true){t.Fatal("file accepted as replay directory")}
}
