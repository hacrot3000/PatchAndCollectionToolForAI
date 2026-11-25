package agentchat

import (
 "context"
 "errors"
)

// Outcome is bounded local transport telemetry only; it is never a provider
// ACK, native receipt, retry key or authorization. Empty means no observed
// outcome (legacy, interrupted, or local telemetry write failed).
type ProviderAttemptOutcome string
const (
 AttemptResponse ProviderAttemptOutcome="response_returned"
 AttemptCancelled ProviderAttemptOutcome="cancelled"
 AttemptConnectTimeout ProviderAttemptOutcome="connect_timeout"
 AttemptHeaderTimeout ProviderAttemptOutcome="header_timeout"
 AttemptRequestTimeout ProviderAttemptOutcome="request_timeout"
 AttemptHTTP499 ProviderAttemptOutcome="http_499"
 AttemptContextRejected ProviderAttemptOutcome="context_rejected"
 AttemptIncomplete ProviderAttemptOutcome="response_incomplete"
 AttemptHTTPRejected ProviderAttemptOutcome="http_rejected"
 AttemptUnknown ProviderAttemptOutcome="transport_or_parse_unknown"
)

type ProviderAttemptPolicy struct{
 Mode RetryMode
 RecoveryGeneration int
}
func validProviderAttemptOutcome(o ProviderAttemptOutcome)bool{
 switch o{
 case "",AttemptResponse,AttemptCancelled,AttemptConnectTimeout,AttemptHeaderTimeout,AttemptRequestTimeout,AttemptHTTP499,AttemptContextRejected,AttemptIncomplete,AttemptHTTPRejected,AttemptUnknown:return true
 }
 return false
}
func validProviderAttemptTelemetry(a ProviderAttempt)bool{
 switch a.RetryMode{case "",RetryOff,RetryLimited,RetryUnlimited:default:return false}
 return a.PayloadBytes>=0&&a.PayloadBytes<=maxProviderRunPayload&&a.EstimatedInputTokens>=0&&a.EstimatedInputTokens<=maxProviderRunPayload&&
  a.OutputBudget>=0&&a.OutputBudget<=1000000&&a.RecoveryGeneration>=0&&a.RecoveryGeneration<=4&&validProviderAttemptOutcome(a.Outcome)
}

// ClassifyProviderAttemptOutcome intentionally never retains provider error
// messages, arbitrary codes, endpoints, prompt content or credentials.
func ClassifyProviderAttemptOutcome(parentErr,err error)ProviderAttemptOutcome{
 if parentErr!=nil||errors.Is(err,context.Canceled){return AttemptCancelled}
 if err==nil{return AttemptResponse}
 var pe *ProviderError
 if errors.As(err,&pe){
  if pe.HTTPStatus==499{return AttemptHTTP499}
  if pe.HTTPStatus==400&&!pe.Ambiguous&&(pe.ProviderErrorCode=="context_length_exceeded"||pe.ProviderErrorCode=="context_window_exceeded"){return AttemptContextRejected}
  switch pe.Code{
  case "CONNECT_TIMEOUT","DIAL_TIMEOUT","DNS_LOOKUP_TIMEOUT","TLS_TIMEOUT":return AttemptConnectTimeout
  case "RESPONSE_HEADER_TIMEOUT":return AttemptHeaderTimeout
  case "REQUEST_TIMEOUT","PROVIDER_TIMEOUT":return AttemptRequestTimeout
  case "RESPONSE_INCOMPLETE":return AttemptIncomplete
  }
  if pe.HTTPStatus>=400{return AttemptHTTPRejected}
 }
 if errors.Is(err,context.DeadlineExceeded){return AttemptRequestTimeout}
 return AttemptUnknown
}
func (o ProviderAttemptOutcome) StagnantTransport()bool{
 switch o{case AttemptConnectTimeout,AttemptHeaderTimeout,AttemptRequestTimeout,AttemptHTTP499,AttemptUnknown:return true}
 return false
}

// RecordProviderAttemptOutcome updates only the exact latest pre-egress
// reservation under the same existing audit lock. A late result cannot
// overwrite a new run, another attempt, or a conflicting recorded outcome.
func(s *Store) RecordProviderAttemptOutcome(id,owner,runID string,attempt uint64,digest string,outcome ProviderAttemptOutcome)error{
 if !validID(id)||outcome==""||!validProviderAttemptOutcome(outcome)||!validGuardDigest(digest){return ErrInvalidSession}
 return s.withLock(func()error{
  x,err:=s.readLocked(id);if err!=nil{return err}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil||x.ProviderRun.ID!=runID||x.ProviderRun.Phase!="sending"||x.Status!="requesting_ai"{return ErrInvalidSession}
  audit,err:=s.readProviderAttemptsLocked(id);if err!=nil{return err}
  if audit.RunID!=runID||audit.Total!=attempt||len(audit.Attempts)==0{return ErrStaleRevision}
  last:=&audit.Attempts[len(audit.Attempts)-1]
  if last.PayloadSHA256!=digest{return ErrInvalidSession}
  if last.Outcome==outcome{return nil}
  if last.Outcome!=""{return ErrStaleRevision}
  last.Outcome=outcome
  return s.writeProviderAttemptsLocked(id,audit)
 })
}
