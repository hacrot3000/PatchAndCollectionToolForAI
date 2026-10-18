package server

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "time"
)

// scheduleNotificationPayload intentionally excludes command lines, console
// content, environment variables and secrets. A notification webhook is an
// outbound, administrator-configured public HTTPS/HTTP destination.
type scheduleNotificationPayload struct{
 Event string `json:"event"`
 ScheduleID string `json:"schedule_id"`
 ScheduleName string `json:"schedule_name"`
 RunID string `json:"run_id"`
 WorkflowID string `json:"workflow_id,omitempty"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 Attempts int `json:"attempts"`
 StartedAt time.Time `json:"started_at"`
 FinishedAt time.Time `json:"finished_at"`
}

func scheduleNotificationEnabled(def scheduleDefinition,status string)bool{
 if def.NotifyWebhookURL==""{return false}
 return (status=="success"&&def.NotifyOnSuccess)||
        (status=="failure"&&def.NotifyOnFailure)
}

// Only public IP addresses may be reached; DNS is validated at dial time,
// redirects are disabled, and no inherited HTTP proxy may bypass the checks.
func schedulePostNotification(ctx context.Context,target string,payload scheduleNotificationPayload)error{
 if err:=validateScheduleURL(target);err!=nil{return err}
 encoded,err:=json.Marshal(payload);if err!=nil{return err}
 transport:=&http.Transport{Proxy:nil,DisableKeepAlives:true,MaxIdleConns:0,TLSHandshakeTimeout:5*time.Second,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   host,port,err:=net.SplitHostPort(address);if err!=nil{return nil,err}
   ips,err:=publicIPs(ctx,host);if err!=nil{return nil,err}
   dialer:=net.Dialer{Timeout:5*time.Second}
   return dialer.DialContext(ctx,"tcp",net.JoinHostPort(ips[0].String(),port))
  },
 }
 defer transport.CloseIdleConnections()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,target,bytes.NewReader(encoded))
 if err!=nil{return err}
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("User-Agent","TaskDeck-Schedule-Notification/1")
 client:=&http.Client{Timeout:8*time.Second,Transport:transport,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
 }
 response,err:=client.Do(req);if err!=nil{return err}
 defer response.Body.Close()
 _,_=io.Copy(io.Discard,io.LimitReader(response.Body,4<<10))
 if response.StatusCode<200||response.StatusCode>=300{
  return fmt.Errorf("notification endpoint returned HTTP %d",response.StatusCode)
 }
 return nil
}

func (s *Server) scheduleNotifyRun(def scheduleDefinition,run scheduleRun,exit int,finished time.Time){
 status:="success";if exit!=0{status="failure"}
 if !scheduleNotificationEnabled(def,status){return}
 payload:=scheduleNotificationPayload{Event:"schedule.run.finished",
  ScheduleID:def.ID,ScheduleName:def.Name,RunID:run.ID,WorkflowID:run.WorkflowID,
  Status:status,ExitCode:exit,Attempts:run.Attempts,
  StartedAt:run.StartedAt,FinishedAt:finished,
 }
 // Never delay dependent schedules or mutate run status if the receiver is
 // offline. Network errors are retained as inspector diagnostics.
 go func(){
  ctx,cancel:=context.WithTimeout(context.Background(),9*time.Second)
  defer cancel()
  err:=schedulePostNotification(ctx,def.NotifyWebhookURL,payload)
  if err!=nil{
   e,engineErr:=s.scheduleEngine();if engineErr!=nil{return}
   e.mu.Lock();e.lastStatus[def.ID+":notification"]=errors.New("notification: "+err.Error()).Error();e.mu.Unlock()
  }
 }()
}
