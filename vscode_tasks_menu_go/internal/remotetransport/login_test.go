package remotetransport

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRemoteLoginEncryptedCodecRoundtripAndNoSessionToken(t *testing.T) {
	browser,agent,_:=participants(t)
	username,password:="alice","not-a-real-password-used-only-in-tests"
	request,err:=RemoteLoginRequestFrame(browser,username,password)
	if err!=nil{t.Fatal(err)}
	if bytes.Contains(request,[]byte(username)) || bytes.Contains(request,[]byte(password)) {
		t.Fatal("Relay can read TaskDeck credentials from AES-GCM frame")
	}
	got,err:=OpenRemoteLogin(agent,request)
	if err!=nil || got.Username!=username || got.Password!=password {
		t.Fatalf("signed E2EE login request rejected: %v",err)
	}
	result,err:=SealRemoteLoginResult(agent,true)
	if err!=nil{t.Fatal(err)}
	if bytes.Contains(result,[]byte("authenticated")) {t.Fatal("Relay can read authenticated login result")}
	accepted,err:=OpenRemoteLoginResult(browser,result)
	if err!=nil || !accepted{t.Fatalf("browser cannot verify encrypted login result: %v",err)}
	if _,err:=OpenRemoteLogin(agent,request);err==nil {
		t.Fatal("replayed password frame accepted")
	}
	if agent.Established(){t.Fatal("replay did not destroy authenticated E2EE channel")}
}

func TestRemoteLoginStrictRejectsArbitraryCommandsAndMalformedJSON(t *testing.T){
	cases:=[]struct{name,plain string}{
		{"HTTP request",`{"type":"http.request","username":"alice","password":"test"}`},
		{"remote command",`{"type":"taskdeck-e2ee-login-v1","username":"alice","password":"test","command":"terminal.run"}`},
		{"duplicate username",`{"type":"taskdeck-e2ee-login-v1","username":"alice","username":"admin","password":"test"}`},
		{"duplicate type",`{"type":"taskdeck-e2ee-login-v1","username":"alice","password":"test","type":"taskdeck-e2ee-login-v1"}`},
		{"unexpected request array",`["taskdeck-e2ee-login-v1","alice","test"]`},
		{"missing password",`{"type":"taskdeck-e2ee-login-v1","username":"alice"}`},
		{"blank username",`{"type":"taskdeck-e2ee-login-v1","username":"  ","password":"test"}`},
		{"control username",`{"type":"taskdeck-e2ee-login-v1","username":"bad\u0000name","password":"test"}`},
		{"invalid password type",`{"type":"taskdeck-e2ee-login-v1","username":"alice","password":{"secret":"test"}}`},
		{"trailing JSON",`{"type":"taskdeck-e2ee-login-v1","username":"alice","password":"test"}{}`},
	}
	for _,test:=range cases{
		t.Run(test.name,func(t *testing.T){
			browser,agent,_:=participants(t)
			frame,err:=browser.Seal([]byte(test.plain))
			if err!=nil{t.Fatal(err)}
			if _,err:=OpenRemoteLogin(agent,frame);!errors.Is(err,ErrInvalidLoginEnvelope) {
				t.Fatalf("unapproved login content accepted: %v",err)
			}
			if agent.Established(){t.Fatal("malformed login did not close encrypted session")}
		})
	}
}

func TestRemoteLoginLimitsAndNegativeAuthentication(t *testing.T) {
	cases:=[]struct{user,pass string}{
		{strings.Repeat("x",129),"pass"},
		{"alice",strings.Repeat("x",4097)},
		{"","pass"},
		{"alice",""},
	}
	for _,tc:=range cases {
		browser,agent,_:=participants(t)
		frame,err:=RemoteLoginRequestFrame(browser,tc.user,tc.pass)
		if err==nil {
			if _,err:=OpenRemoteLogin(agent,frame);err==nil {t.Fatal("excessive or empty credentials accepted")}
		}
	}
	browser,agent,_:=participants(t)
	frame,err:=SealRemoteLoginResult(agent,false)
	if err!=nil{t.Fatal(err)}
	result,err:=OpenRemoteLoginResult(browser,frame)
	if err!=nil || result {t.Fatal("failed login result was elevated")}
	_,otherAgent,_:=participants(t)
	if _,err:=OpenRemoteLogin(otherAgent,frame);err==nil {
		t.Fatal("Agent accepted encrypted result as a password-bearing login request")
	}
}

func TestRemoteLoginResultRejectsMissingDuplicateAndTokenFields(t *testing.T){
	cases:=[]struct{name,body string}{
		{"missing required field",`{"type":"taskdeck-e2ee-login-result-v1","authenticated":true}`},
		{"duplicate privilege field",`{"type":"taskdeck-e2ee-login-result-v1","authenticated":true,"remote_control_available":false,"authenticated":true}`},
		{"unexpected bearer",`{"type":"taskdeck-e2ee-login-result-v1","authenticated":true,"remote_control_available":false,"token":"secret"}`},
		{"remote control bypass",`{"type":"taskdeck-e2ee-login-result-v1","authenticated":true,"remote_control_available":true}`},
		{"wrong auth type",`{"type":"taskdeck-e2ee-login-result-v1","authenticated":"true","remote_control_available":false}`},
		{"unexpected protocol",`{"type":"taskdeck-http-proxy","authenticated":true,"remote_control_available":false}`},
	}
	for _,test:=range cases{
		t.Run(test.name,func(t *testing.T){
			browser,agent,_:=participants(t)
			ready,err:=agent.Seal([]byte("taskdeck-e2ee-ready-v1"))
			if err!=nil{t.Fatal(err)}
			if _,err:=browser.Open(ready);err!=nil{t.Fatal(err)}
			frame,err:=agent.Seal([]byte(test.body))
			if err!=nil{t.Fatal(err)}
			if _,err:=OpenRemoteLoginResult(browser,frame);!errors.Is(err,ErrInvalidLoginEnvelope){
				t.Fatalf("malformed login result accepted: %v",err)
			}
			if browser.Established(){t.Fatal("invalid result did not revoke encrypted session")}
		})
	}
}
