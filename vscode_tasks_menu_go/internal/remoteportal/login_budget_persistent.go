package remoteportal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// This is a per-Agent-identity local CPU budget, NOT a Browser token or a
// TaskDeck account. The rate ledger is never sent to Portal/Relay and stores
// no username, password, IP, ticket or session details.
//
// All Agent processes sharing this identity coordinate with a private flock.
// The rate record is durably replaced BEFORE an allowed login starts scrypt.
// Corrupt, missing permissions or unwriteable storage is a hard denial.
type durableLoginRate struct {
	Version int `json:"v"`
	DeviceID string `json:"device_id"`
	Tokens int `json:"tokens"`
	LastNanos int64 `json:"last_nanos"`
}

func privateRegularFile(path string) error {
	info,err:=os.Lstat(path)
	if err!=nil{return err}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 {
		return errors.New("Agent login budget file must be owner-only, regular and not a symlink")
	}
	return nil
}

// loadRateLedger never accepts partial/corrupt records; migration and normal
// reads use identical checks. An empty object cannot inherit fresh tokens.
func loadRateLedger(path string) (durableLoginRate,error) {
	var st durableLoginRate
	if err:=privateRegularFile(path);err!=nil{return st,err}
	f,err:=os.Open(path)
	if err!=nil{return st,err}
	raw,readErr:=io.ReadAll(io.LimitReader(f,1025))
	closeErr:=f.Close()
	if readErr!=nil{return st,readErr}
	if closeErr!=nil{return st,closeErr}
	if len(raw)==0 || len(raw)>1024{return st,errors.New("invalid Agent rate ledger length")}
	dec:=json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err:=dec.Decode(&st);err!=nil{return st,fmt.Errorf("invalid Agent rate ledger: %w",err)}
	var extra any
	if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){
		return st,errors.New("Agent rate ledger has trailing data")
	}
	if st.Version!=1 || st.Tokens<0 || st.Tokens>agentLoginProbeBurst ||
		st.LastNanos<=0 || len(st.DeviceID)!=64 {
		return st,errors.New("Agent rate ledger integrity check failed")
	}
	return st,nil
}

// migrateLegacyLocked is called while the identity-specific ledger lock is
// held. The previous shared name could be owned by *another* TaskDeck Agent.
// Only that Device ID may inherit its spent tokens. All new Agent processes
// coordinate migration through the OLD lock, and the old ledger is renamed
// instead of copied so the budget cannot be refreshed by repeated migration.
func (b *agentLoginBudget) migrateLegacyLocked() error {
	if b.legacyPath=="" || b.legacyPath==b.path {return nil}
	if err:=privateRegularFile(b.path);err==nil{return nil
	} else if !os.IsNotExist(err){return err}
	legacyLock:=b.legacyPath+".lock"
	if err:=privateRegularFile(legacyLock);err!=nil && !os.IsNotExist(err){return err}
	lf,err:=os.OpenFile(legacyLock,os.O_CREATE|os.O_RDWR,0600)
	if err!=nil{return err}
	defer lf.Close()
	if err:=privateRegularFile(legacyLock);err!=nil{return err}
	if err:=syscall.Flock(int(lf.Fd()),syscall.LOCK_EX);err!=nil{return err}
	defer syscall.Flock(int(lf.Fd()),syscall.LOCK_UN)

	saved,err:=loadRateLedger(b.legacyPath)
	if os.IsNotExist(err){return nil}
	if err!=nil{return err}
	if saved.DeviceID!=b.deviceID {
		// This was another project; do not give it this device's budget.
		return nil
	}
	if err:=os.Rename(b.legacyPath,b.path);err!=nil{return err}
	dir,err:=os.Open(filepath.Dir(b.path))
	if err!=nil{return err}
	syncErr:=dir.Sync()
	closeErr:=dir.Close()
	if syncErr!=nil{return syncErr}
	return closeErr
}

func (b *agentLoginBudget) allowPersistent(now time.Time) (bool,error) {
	if b.path=="" {return b.allow(now),nil} // legacy in-memory unit fixtures
	b.mu.Lock()
	defer b.mu.Unlock()

	lockPath:=b.path+".lock"
	if err:=privateRegularFile(lockPath);err!=nil && !os.IsNotExist(err){
		return false,err
	}
	lf,err:=os.OpenFile(lockPath,os.O_CREATE|os.O_RDWR,0600)
	if err!=nil{return false,err}
	defer lf.Close()
	if err:=privateRegularFile(lockPath);err!=nil{return false,err}
	if err:=syscall.Flock(int(lf.Fd()),syscall.LOCK_EX);err!=nil{return false,err}
	defer syscall.Flock(int(lf.Fd()),syscall.LOCK_UN)

	if err:=b.migrateLegacyLocked();err!=nil{return false,err}
	st:=durableLoginRate{Version:1,DeviceID:b.deviceID,Tokens:agentLoginProbeBurst,LastNanos:now.UnixNano()}
	if loaded,err:=loadRateLedger(b.path);err==nil {
		if loaded.DeviceID!=b.deviceID {
			return false,errors.New("Agent rate ledger identity mismatch")
		}
		st=loaded
	} else if !os.IsNotExist(err){return false,err}

	// A clock rollback cannot replenish the budget. Refill happens in
	// exact integer intervals, capped by the original burst capacity.
	when:=now.UnixNano()
	if when>st.LastNanos {
		intervals:=(when-st.LastNanos)/int64(agentLoginProbeRefill)
		if intervals>=int64(agentLoginProbeBurst) {
			st.Tokens=agentLoginProbeBurst
			st.LastNanos=when
		} else if intervals>0 {
			st.Tokens+=int(intervals)
			if st.Tokens>agentLoginProbeBurst{st.Tokens=agentLoginProbeBurst}
			st.LastNanos+=intervals*int64(agentLoginProbeRefill)
		}
	}
	if st.Tokens==0{return false,nil}
	st.Tokens--
	serialized,err:=json.Marshal(st)
	if err!=nil{return false,err}
	dir:=filepath.Dir(b.path)
	tf,err:=os.CreateTemp(dir,".rate-ledger-*")
	if err!=nil{return false,err}
	tmp:=tf.Name()
	defer os.Remove(tmp)
	if err:=tf.Chmod(0600);err!=nil{tf.Close();return false,err}
	_,err=tf.Write(serialized)
	if err==nil{err=tf.Sync()}
	closeErr:=tf.Close()
	if err!=nil{return false,err}
	if closeErr!=nil{return false,closeErr}
	if err:=os.Rename(tmp,b.path);err!=nil{return false,err}
	df,err:=os.Open(dir)
	if err!=nil{return false,err}
	err=df.Sync()
	closeErr=df.Close()
	if err!=nil{return false,err}
	if closeErr!=nil{return false,closeErr}
	return true,nil
}
