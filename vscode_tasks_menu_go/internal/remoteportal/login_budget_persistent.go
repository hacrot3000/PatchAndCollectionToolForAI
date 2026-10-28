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

	st:=durableLoginRate{Version:1,DeviceID:b.deviceID,Tokens:agentLoginProbeBurst,LastNanos:now.UnixNano()}
	if err:=privateRegularFile(b.path);err==nil {
		f,openErr:=os.Open(b.path)
		if openErr!=nil{return false,openErr}
		raw,readErr:=io.ReadAll(io.LimitReader(f,1025))
		closeErr:=f.Close()
		if readErr!=nil{return false,readErr}
		if closeErr!=nil{return false,closeErr}
		if len(raw)==0 || len(raw)>1024{return false,errors.New("invalid Agent rate ledger length")}
		d:=json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err:=d.Decode(&st);err!=nil{return false,fmt.Errorf("invalid Agent rate ledger: %w",err)}
		if d.More(){return false,errors.New("multiple Agent rate ledger values")}
		var extra any
		if err:=d.Decode(&extra);!errors.Is(err,io.EOF) {
			return false,errors.New("Agent rate ledger has trailing data")
		}
		if st.Version!=1 || st.DeviceID!=b.deviceID ||
			st.Tokens<0 || st.Tokens>agentLoginProbeBurst || st.LastNanos<=0 {
			return false,errors.New("Agent rate ledger integrity check failed")
		}
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
