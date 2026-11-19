package agentchat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxSessionBytes = 4 << 20

var (
	ErrNotFound = errors.New("agent session not found")
	ErrStaleRevision = errors.New("agent session revision mismatch")
	ErrInvalidSession = errors.New("invalid agent session")
	// All Store instances in this daemon serialize journal mutation.
	// Multi-process writers must use a shared database/lock before deployment.
	agentJournalMu sync.Mutex
)

// ToolCallRecord is a durable, owner-bound execution intent. An absent or
// ambiguous run is never retried automatically. A call is identified by its
// provider call_id and the canonical transcript contains its exact arguments.
type ToolCallRecord struct {
 CallID string `json:"call_id"`
 Name string `json:"name"`
 Status string `json:"status"`
 QueuePath string `json:"queue_path,omitempty"`
 PackageSHA256 string `json:"package_sha256,omitempty"`
 PatchSessionID string `json:"patch_session_id,omitempty"`
 Result string `json:"result,omitempty"`
 FeedbackSent bool `json:"feedback_sent,omitempty"`
}

// Session is persisted locally even when the provider offers server-side
// context. It never contains API keys. All mutations require owner/identity
// matching and revision compare-and-swap.
type Session struct {
	ID                string        `json:"id"`
	Owner             string        `json:"owner"`
	Workspace         string        `json:"workspace"`
	ProjectID         string        `json:"project_id"`
	ProviderID        string        `json:"provider_id"`
	ModelID           string        `json:"model_id"`
	Mode              ContextMode   `json:"mode"`
	AIPackFingerprint string        `json:"ai_pack_fingerprint,omitempty"`
	PreviousResponseID string       `json:"previous_response_id,omitempty"`
	ConversationID    string        `json:"conversation_id,omitempty"`
	Status            string        `json:"status"`
	History           []HistoryItem `json:"history"`
	ToolCalls         []ToolCallRecord `json:"tool_calls,omitempty"`
	Revision          uint64        `json:"revision"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

func NewSession(owner, workspace, projectID, providerID, modelID string, mode ContextMode)(Session,error){
	if owner==""||len(owner)>256||workspace==""||len(workspace)>4096||projectID==""||len(projectID)>128||providerID==""||len(providerID)>128||modelID==""||len(modelID)>128 {
		return Session{},ErrInvalidSession
	}
	if mode!=Stateless&&mode!=PreviousResponse&&mode!=Conversation{return Session{},ErrInvalidSession}
	var bytes [16]byte
	if _,err:=rand.Read(bytes[:]);err!=nil{return Session{},err}
	now:=time.Now().UTC()
	return Session{
		ID:hex.EncodeToString(bytes[:]),Owner:owner,Workspace:filepath.Clean(workspace),
		ProjectID:projectID,ProviderID:providerID,ModelID:modelID,Mode:mode,
		Status:"created",History:[]HistoryItem{},CreatedAt:now,UpdatedAt:now,
	},nil
}

// Store keeps canonical session history outside the project source tree.
type Store struct{ dir string }

func NewStore(dir string)(*Store,error){
	if dir==""||!filepath.IsAbs(dir){return nil,errors.New("absolute private state directory required")}
	return &Store{dir:filepath.Clean(dir)},nil
}
func validID(id string) bool{
	if len(id)!=32{return false}
	for _,c:=range id{
		if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}
	}
	return true
}
func (s *Store) ensureDir()error{
	if err:=os.MkdirAll(s.dir,0700);err!=nil{return err}
	info,err:=os.Lstat(s.dir)
	if err!=nil{return err}
	if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{
		return errors.New("unsafe agent session storage directory")
	}
	return nil
}
func (s *Store) file(id string)string{return filepath.Join(s.dir,id+".json")}
func validateSession(x Session)error{
	if !validID(x.ID)||x.Owner==""||len(x.Owner)>256||x.Workspace==""||x.ProjectID==""||x.ProviderID==""||x.ModelID==""{
		return ErrInvalidSession
	}
	if x.Mode!=Stateless&&x.Mode!=PreviousResponse&&x.Mode!=Conversation{return ErrInvalidSession}
	if x.Status==""||len(x.Status)>64||len(x.History)>maxItems{return ErrInvalidSession}
	for _,item:=range x.History{if err:=validHistoryItem(item);err!=nil{return err}}
	if len(x.ToolCalls)>32{return ErrInvalidSession}
	seenCalls:=make(map[string]bool,len(x.ToolCalls))
	for _,call:=range x.ToolCalls{
		if call.CallID==""||len(call.CallID)>256||call.Name!="taskdeck_submit_patch"||seenCalls[call.CallID]||len(call.QueuePath)>512||len(call.PackageSHA256)>64||len(call.PatchSessionID)>256||len(call.Result)>4096{return ErrInvalidSession}
		seenCalls[call.CallID]=true
		switch call.Status{
		case "pending","rejected","queuing","queued","dispatching","running","done","error","unknown":
		default:return ErrInvalidSession
		}
		if call.Status=="queued" && call.QueuePath=="" {return ErrInvalidSession}
		found:=false
		for _,item:=range x.History{
			if item.Type=="function_call"&&item.CallID==call.CallID&&item.Name==call.Name{found=true;break}
		}
		if !found{return ErrInvalidSession}
	}
	if len(x.PreviousResponseID)>256||len(x.ConversationID)>256||len(x.AIPackFingerprint)>128{return ErrInvalidSession}
	return nil
}
func (s *Store) readLocked(id string)(Session,error){
	if !validID(id){return Session{},ErrNotFound}
	info,err:=os.Lstat(s.file(id))
	if os.IsNotExist(err){return Session{},ErrNotFound}
	if err!=nil{return Session{},err}
	if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>maxSessionBytes{
		return Session{},errors.New("unsafe agent session file")
	}
	raw,err:=os.ReadFile(s.file(id));if err!=nil{return Session{},err}
	var x Session
	if err=json.Unmarshal(raw,&x);err!=nil{return Session{},err}
	if x.ID!=id{return Session{},ErrInvalidSession}
	if err=validateSession(x);err!=nil{return Session{},err}
	return x,nil
}
func (s *Store) Get(id,owner string)(Session,error){
	agentJournalMu.Lock();defer agentJournalMu.Unlock()
	if err:=s.ensureDir();err!=nil{return Session{},err}
	x,err:=s.readLocked(id)
	if err!=nil{return Session{},err}
	if x.Owner!=owner{return Session{},ErrNotFound}
	return x,nil
}

// Save creates a new session when expectedRevision == 0; otherwise it requires
// an exact match to prevent stale model responses overwriting another turn.
func (s *Store) Save(x Session,expectedRevision uint64)(Session,error){
	agentJournalMu.Lock();defer agentJournalMu.Unlock()
	if err:=s.ensureDir();err!=nil{return Session{},err}
	if err:=validateSession(x);err!=nil{return Session{},err}
	existing,err:=s.readLocked(x.ID)
	if errors.Is(err,ErrNotFound){
		if expectedRevision!=0{return Session{},ErrStaleRevision}
	} else if err!=nil{return Session{},err
	} else {
		if existing.Revision!=expectedRevision{return Session{},ErrStaleRevision}
		if existing.Owner!=x.Owner||existing.Workspace!=x.Workspace||existing.ProjectID!=x.ProjectID||existing.ProviderID!=x.ProviderID||existing.ModelID!=x.ModelID||existing.Mode!=x.Mode||!existing.CreatedAt.Equal(x.CreatedAt){
			return Session{},ErrInvalidSession
		}
	}
	x.Revision=expectedRevision+1
	x.UpdatedAt=time.Now().UTC()
	raw,err:=json.Marshal(x);if err!=nil{return Session{},err}
	if len(raw)>maxSessionBytes{return Session{},errors.New("agent session exceeds size limit")}
	temp,err:=os.CreateTemp(s.dir,".agent-session-*");if err!=nil{return Session{},err}
	defer os.Remove(temp.Name())
	if err=temp.Chmod(0600);err!=nil{temp.Close();return Session{},err}
	if _,err=temp.Write(raw);err!=nil{temp.Close();return Session{},err}
	if err=temp.Sync();err!=nil{temp.Close();return Session{},err}
	if err=temp.Close();err!=nil{return Session{},err}
	if err=os.Rename(temp.Name(),s.file(x.ID));err!=nil{return Session{},err}
	return x,nil
}
func (s *Store) List(owner string)([]Session,error){
	agentJournalMu.Lock();defer agentJournalMu.Unlock()
	if err:=s.ensureDir();err!=nil{return nil,err}
	entries,err:=os.ReadDir(s.dir);if err!=nil{return nil,err}
	if len(entries)>1024{return nil,fmt.Errorf("too many agent session files")}
	result:=make([]Session,0)
	for _,entry:=range entries{
		name:=entry.Name()
		if !strings.HasSuffix(name,".json") {continue}
		id:=strings.TrimSuffix(name,".json")
		if !validID(id){continue}
		x,err:=s.readLocked(id)
		if err!=nil{return nil,err}
		if x.Owner==owner{result=append(result,x)}
	}
	sort.Slice(result,func(i,j int)bool{return result[i].UpdatedAt.After(result[j].UpdatedAt)})
	return result,nil
}
