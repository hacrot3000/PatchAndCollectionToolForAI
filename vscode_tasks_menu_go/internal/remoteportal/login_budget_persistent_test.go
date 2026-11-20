package remoteportal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPersistentLoginQuotaAcrossIndependentAgentRestarts(t *testing.T) {
	root:=t.TempDir()
	path:=filepath.Join(root,"rate.json")
	device:=strings.Repeat("a",64)
	when:=time.Now()
	newAgent:=func() *agentLoginBudget {
		return &agentLoginBudget{path:path,deviceID:device}
	}
	first:=newAgent()
	for i:=0;i<agentLoginProbeBurst;i++{
		ok,err:=first.allowPersistent(when)
		if err!=nil || !ok {t.Fatalf("initial login %d denied: %v",i,err)}
	}
	restarted:=newAgent()
	if ok,err:=restarted.allowPersistent(when);err!=nil || ok {
		t.Fatalf("Agent restart refilled attacker's exhausted budget: %v, %v",ok,err)
	}
	if ok,err:=restarted.allowPersistent(when.Add(-30*time.Second));err!=nil || ok{
		t.Fatalf("system clock rollback refilled Agent quota: %v %v",ok,err)
	}
	if ok,err:=restarted.allowPersistent(when.Add(11*time.Second));err!=nil || ok{
		t.Fatalf("quota replenished early: %v %v",ok,err)
	}
	if ok,err:=restarted.allowPersistent(when.Add(12*time.Second));err!=nil || !ok {
		t.Fatalf("expected exactly one refill: %v %v",ok,err)
	}
	if ok,err:=newAgent().allowPersistent(when.Add(12*time.Second));err!=nil || ok{
		t.Fatalf("second process bypassed shared quota at refill boundary: %v %v",ok,err)
	}
	if ok,err:=newAgent().allowPersistent(when.Add(3*time.Minute));err!=nil || !ok{
		t.Fatalf("future quota did not refill: %v %v",ok,err)
	}
	info,err:=os.Lstat(path)
	if err!=nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 {
		t.Fatalf("persistent Agent quota not owner-only: %v %v",info,err)
	}
	info,err=os.Lstat(path+".lock")
	if err!=nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 {
		t.Fatalf("persistent Agent quota lock not owner-only: %v %v",info,err)
	}
}

func TestPersistentLoginQuotaFlockAcrossMultipleAgentObjects(t *testing.T){
	p:=filepath.Join(t.TempDir(),"private.json")
	device:=strings.Repeat("b",64)
	at:=time.Now()
	var success atomic.Int32
	var failure atomic.Int32
	var wg sync.WaitGroup
	for i:=0;i<100;i++{
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each Agent uses its own Go mutex. File locking is the only
			// shared synchronization between independent processes.
			b:=&agentLoginBudget{path:p,deviceID:device}
			allowed,err:=b.allowPersistent(at)
			if err!=nil {failure.Add(1);return}
			if allowed{success.Add(1)}
		}()
	}
	wg.Wait()
	if failure.Load()!=0 || success.Load()!=agentLoginProbeBurst {
		t.Fatalf("concurrent Agents bypassed quota: allowed=%d IOerrors=%d",success.Load(),failure.Load())
	}
}

func TestPersistentLoginBudgetCorruptionAndIdentitySwapFailClosed(t *testing.T){
	dir:=t.TempDir()
	path:=filepath.Join(dir,"rate.json")
	owner:=&agentLoginBudget{path:path,deviceID:strings.Repeat("a",64)}
	at:=time.Now()
	if ok,err:=owner.allowPersistent(at);err!=nil || !ok{t.Fatal(err)}
	wrong:=&agentLoginBudget{path:path,deviceID:strings.Repeat("b",64)}
	if ok,err:=wrong.allowPersistent(at);err==nil || ok{
		t.Fatal("another Device ID inherited original Agent CPU allowance")
	}
	if err:=os.WriteFile(path,[]byte("invalid-JSON"),0600);err!=nil{t.Fatal(err)}
	if ok,err:=owner.allowPersistent(at);err==nil || ok{
		t.Fatal("damaged Agent quota database failed open")
	}
	if err:=os.WriteFile(path,[]byte("{}"),0600);err!=nil{t.Fatal(err)}
	if ok,err:=owner.allowPersistent(at);err==nil || ok{
		t.Fatal("quota ledger with missing version/device accepted")
	}
	if err:=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
	if ok,err:=owner.allowPersistent(at);err==nil || ok{
		t.Fatal("world-readable Agent login ledger accepted")
	}
}

func TestPersistentLoginBudgetSymlinkAndUnavailableStorageFailClosed(t *testing.T){
	dir:=t.TempDir()
	target:=filepath.Join(dir,"private")
	if err:=os.WriteFile(target,[]byte("no-overwrite"),0600);err!=nil{t.Fatal(err)}
	path:=filepath.Join(dir,"ledger.json")
	if err:=os.Symlink(target,path);err!=nil{t.Skipf("symlink unavailable: %v",err)}
	b:=&agentLoginBudget{path:path,deviceID:strings.Repeat("f",64)}
	if ok,err:=b.allowPersistent(time.Now());err==nil || ok{
		t.Fatal("symlinked Agent login ledger accepted")
	}
	bytes,err:=os.ReadFile(target)
	if err!=nil || string(bytes)!="no-overwrite"{
		t.Fatal("unsafe rate ledger modified symlink target")
	}
	if err:=os.Remove(path);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(target,path+".lock");err!=nil{t.Skipf("symlink unavailable: %v",err)}
	if ok,err:=b.allowPersistent(time.Now());err==nil || ok{
		t.Fatal("symlinked Agent quota lock accepted")
	}
	missing:=&agentLoginBudget{path:filepath.Join(dir,"nonexistent","ledger.json"),
		deviceID:strings.Repeat("f",64)}
	if ok,err:=missing.allowPersistent(time.Now());err==nil || ok {
		t.Fatal("unwritable Agent quota storage allowed password CPU work")
	}
}

func TestAgentsWithSeparateWorkspacesNeverShareLoginBudget(t *testing.T) {
	dir:=filepath.Join(t.TempDir(),"private")
	first,err:=NewAgent("https://portal.example.com",filepath.Join(dir,"workspace-a.seed"),nil)
	if err!=nil{t.Fatal(err)}
	second,err:=NewAgent("https://portal.example.com",filepath.Join(dir,"workspace-b.seed"),nil)
	if err!=nil{t.Fatal(err)}
	if first.DeviceID()==second.DeviceID() || first.loginBudget.path==second.loginBudget.path {
		t.Fatal("different TaskDeck identities still share one login CPU budget")
	}
	if first.loginBudget.legacyPath!=second.loginBudget.legacyPath {
		t.Fatal("legacy migration did not refer to the same old file")
	}
	when:=time.Now()
	// Simulate a fully exhausted budget created by an older Agent version.
	legacy:=&agentLoginBudget{path:first.loginBudget.legacyPath,deviceID:first.DeviceID()}
	for i:=0;i<agentLoginProbeBurst;i++ {
		if ok,err:=legacy.allowPersistent(when);err!=nil||!ok {
			t.Fatalf("legacy quota initialization %d: %v",i,err)
		}
	}
	// B encounters a different device ID in the old shared path. It must
	// neither inherit its spent budget nor overwrite A's migration source.
	if ok,err:=second.loginBudget.allowPersistent(when);err!=nil||!ok {
		t.Fatalf("unrelated Agent incorrectly denied by legacy quota: %v",err)
	}
	if ok,err:=first.loginBudget.allowPersistent(when);err!=nil||ok {
		t.Fatalf("migration reset exhausted original Agent's quota: allowed=%v err=%v",ok,err)
	}
	if _,err:=os.Stat(first.loginBudget.legacyPath);!os.IsNotExist(err) {
		t.Fatal("legacy quota was copied instead of moved; migration can be repeated")
	}
	newFirst,err:=NewAgent("https://portal.example.com",filepath.Join(dir,"workspace-a.seed"),nil)
	if err!=nil{t.Fatal(err)}
	if ok,err:=newFirst.loginBudget.allowPersistent(when);err!=nil||ok {
		t.Fatalf("restarted Agent A received new login CPU budget: %v %v",ok,err)
	}
	for i:=1;i<agentLoginProbeBurst;i++ {
		if ok,err:=second.loginBudget.allowPersistent(when);err!=nil||!ok {
			t.Fatalf("Agent B budget incorrectly shared with exhausted A at %d: %v",i,err)
		}
	}
	if ok,err:=second.loginBudget.allowPersistent(when);err!=nil||ok {
		t.Fatalf("Agent B exceeded its own 5-attempt quota: %v %v",ok,err)
	}
	if ok,err:=newFirst.loginBudget.allowPersistent(when.Add(12*time.Second));err!=nil||!ok {
		t.Fatalf("migrated Agent A did not retain normal refill behavior: %v %v",ok,err)
	}
}

func TestLegacyAgentRateMigrationDeniesUnsafeRecordsWithoutReset(t *testing.T) {
	dir:=filepath.Join(t.TempDir(),"private")
	agent,err:=NewAgent("https://portal.example.com",filepath.Join(dir,"identity.seed"),nil)
	if err!=nil{t.Fatal(err)}
	legacy:=agent.loginBudget.legacyPath
	if err:=os.WriteFile(legacy,[]byte("{}"),0600);err!=nil{t.Fatal(err)}
	if ok,err:=agent.loginBudget.allowPersistent(time.Now());err==nil||ok {
		t.Fatal("damaged legacy ledger allowed new unlimited password guesses")
	}
	if _,err:=os.Stat(agent.loginBudget.path);!os.IsNotExist(err) {
		t.Fatal("corrupt legacy ledger created new initial quota")
	}
	if err:=os.Remove(legacy);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(agent.loginBudget.path,legacy);err!=nil{t.Skipf("symlink unavailable: %v",err)}
	if ok,err:=agent.loginBudget.allowPersistent(time.Now());err==nil||ok {
		t.Fatal("legacy quota symlink bypassed migration fail-closed policy")
	}
}
