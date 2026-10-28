package remoteportal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"
)

// Every experimental TaskDeck password-check request must confirm that the
// user still has the claimed device-scoped Portal role. This prevents an
// already signed 30-second ticket from authorizing password attempts after
// immediate Portal account disable, role downgrade, grant revoke or device
// unlink. The signed ticket is still required independently at the Agent.
//
// Only a signed Device-ID challenge (no Browser credentials) reaches Portal.
// A failed/unreachable Portal is a hard denial. Never use a Relay-provided
// response as current Portal authorization.
type liveGrantCheckMessage struct {
	signedMessage
	PortalUserID string `json:"portal_user_id"`
	AccessRole string `json:"access_role"`
	TicketJTI string `json:"ticket_jti"`
	PortalAuthEpoch int64 `json:"portal_auth_epoch"`
}

func (a *Agent) confirmCurrentPortalGrant(ctx context.Context, grant *AgentGrant) error {
	if a==nil || grant==nil || grant.subject=="" || grant.ticketJTI=="" ||
		grant.authEpoch<1 || a.identity==nil || a.base=="" {
		// The test-only injection below deliberately cannot be used to
		// bypass a missing signed Agent identity or grant metadata.
		return errors.New("no authenticated Portal grant identity")
	}
	if a.liveGrantCheck!=nil {return a.liveGrantCheck(ctx,grant)}
	signed,err:=a.identity.sign("grant-check",a.DeviceID(),
		grant.subject,string(grant.role),grant.ticketJTI,
		strconv.FormatInt(grant.authEpoch,10))
	if err!=nil{return err}
	body,err:=json.Marshal(liveGrantCheckMessage{
		signedMessage:signed,PortalUserID:grant.subject,
		AccessRole:string(grant.role),TicketJTI:grant.ticketJTI,
		PortalAuthEpoch:grant.authEpoch,
	})
	if err!=nil{return err}
	checkCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	defer cancel()
	// Reuse Agent HTTPS client, which disables redirects and has TLS
	// verification. Portal lives on shared PHP hosting, not on Relay.
	response,err:=a.postRaw(checkCtx,"/api/agent/grant-check",body)
	if err!=nil{return err}
	d:=json.NewDecoder(bytes.NewReader(response))
	d.DisallowUnknownFields()
	var verdict struct {
		OK bool `json:"ok"`
		Allowed bool `json:"allowed"`
	}
	if err:=d.Decode(&verdict);err!=nil{return err}
	var extra any
	if err:=d.Decode(&extra);!errors.Is(err,io.EOF) {
		return errors.New("Portal grant check had trailing content")
	}
	if !verdict.OK || !verdict.Allowed {
		return errors.New("Portal grant revoked or downgraded")
	}
	return nil
}
