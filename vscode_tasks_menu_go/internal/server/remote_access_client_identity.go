package server

import (
 "context"
 "errors"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// LookupRemoteClientLocalUserID is a HOST-ONLY callback used by the Agent
// before it returns an encrypted software-key-approved result to the client.
// It does not authenticate a password, issue a browser session, or read
// Portal account roles. A missing/disabled/replaced TaskDeck local account,
// project or membership must deny BEFORE the password prompt.
func (s *Server) LookupRemoteClientLocalUserID(ctx context.Context,username string)(string,error) {
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Identity==nil||
  !s.Config.RemoteAccessEnabled||!s.Config.SharedServerEnabled||
  !s.Config.TLS()||s.Config.SharedProjectID==""||
  username==""{return "",denied}
 lookupCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
 defer cancel()
 user,err:=s.Identity.UserByUsername(lookupCtx,username)
 if err!=nil||!user.Enabled||user.ID==""{return "",denied}
 project,err:=s.Identity.ProjectByKey(lookupCtx,s.Config.SharedProjectID)
 if err!=nil||!project.Enabled {return "",denied}
 member,err:=s.Identity.ProjectMember(lookupCtx,project.ID,user.ID)
 if err!=nil||!member.Enabled{return "",denied}
 // Re-fetch current identity just before the preflight status is released.
 // A replaced user with the same username has a different immutable ID.
 again,err:=s.Identity.UserByID(lookupCtx,user.ID)
 if err!=nil||!again.Enabled||again.ID!=user.ID||
  !errors.Is(lookupCtx.Err(),nil){return "",denied}
 return string(user.ID),nil
}
