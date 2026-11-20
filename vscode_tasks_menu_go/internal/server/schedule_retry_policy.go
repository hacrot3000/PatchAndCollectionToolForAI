package server

// scheduleShouldRetry respects the maximum attempt count and optional exit
// code allowlist. An empty list retains the previous retry-on-any-failure
// behavior. Cancellation and successful completion must never be retried.
func scheduleShouldRetry(exitCode,attempt,maxAttempts int,allowedCodes []int,canceled bool)bool{
 if canceled||exitCode==0||attempt>=maxAttempts{return false}
 if len(allowedCodes)==0{return true}
 for _,code:=range allowedCodes{if code==exitCode{return true}}
 return false
}
