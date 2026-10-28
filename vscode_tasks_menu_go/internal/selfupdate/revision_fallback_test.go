package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRemoteRevisionAPI403FallsBackToExactGitRef(t *testing.T) {
	const want = "0123456789abcdef0123456789abcdef01234567"
	calls := 0
	client := &http.Client{Transport: revisionHTTPRoundTrip(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.String(), "/commits/feature%2Ftest") {
			t.Fatalf("unexpected API URL %q", req.URL)
		}
		return &http.Response{StatusCode:403,Status:"403 Forbidden",Header:http.Header{"X-Ratelimit-Remaining":[]string{"0"}},Body:io.NopCloser(strings.NewReader("{}"))},nil
	})}
	got, err := remoteRevisionWithFallback(context.Background(), "feature/test", client, func(_ context.Context, branch string)(string,error){
		calls++
		if branch!="feature/test" {t.Fatalf("fallback branch %q",branch)}
		return want,nil
	})
	if err!=nil || got!=want || calls!=1 { t.Fatalf("revision=%q calls=%d err=%v",got,calls,err) }
}

func TestRemoteRevisionSuccessDoesNotUseFallback(t *testing.T) {
	const want="abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	client := &http.Client{Transport:revisionHTTPRoundTrip(func(req *http.Request)(*http.Response,error){
		return &http.Response{StatusCode:200,Status:"200 OK",Header:make(http.Header),Body:io.NopCloser(strings.NewReader(`{"sha":"abcdefabcdefabcdefabcdefabcdefabcdefabcd"}`))},nil
	})}
	got,err:=remoteRevisionWithFallback(context.Background(),"main",client,func(context.Context,string)(string,error){
		t.Fatal("fallback must not execute after API success");return "",nil
	})
	if err!=nil||got!=want {t.Fatalf("got=%q err=%v",got,err)}
}

func TestRemoteRevisionDoesNotHide404WithGit(t *testing.T) {
	client:=&http.Client{Transport:revisionHTTPRoundTrip(func(req *http.Request)(*http.Response,error){
		return &http.Response{StatusCode:404,Status:"404 Not Found",Header:make(http.Header),Body:io.NopCloser(strings.NewReader("{}"))},nil
	})}
	_,err:=remoteRevisionWithFallback(context.Background(),"missing",client,func(context.Context,string)(string,error){
		t.Fatal("git fallback for 404 must not be called");return "",nil
	})
	if err==nil||!strings.Contains(err.Error(),"404") {t.Fatalf("err=%v",err)}
}

func TestRemoteRevision403FallbackErrorKeepsDiagnosis(t *testing.T) {
	client:=&http.Client{Transport:revisionHTTPRoundTrip(func(req *http.Request)(*http.Response,error){
		return &http.Response{StatusCode:403,Status:"403 Forbidden",Header:http.Header{"X-Ratelimit-Remaining":[]string{"0"}},Body:io.NopCloser(strings.NewReader("{}"))},nil
	})}
	_,err:=remoteRevisionWithFallback(context.Background(),"main",client,func(context.Context,string)(string,error){
		return "",errors.New("git unavailable")
	})
	if err==nil||!strings.Contains(err.Error(),"403")||!strings.Contains(err.Error(),"git unavailable"){
		t.Fatalf("missing API/fallback diagnostics: %v",err)
	}
}

func TestRemoteRevisionInvalidBranchAndMalformedSHAAreRejected(t *testing.T) {
	calls:=0
	client:=&http.Client{Transport:revisionHTTPRoundTrip(func(req *http.Request)(*http.Response,error){
		calls++
		return &http.Response{StatusCode:200,Status:"200 OK",Header:make(http.Header),Body:io.NopCloser(strings.NewReader(`{"sha":"not-a-sha"}`))},nil
	})}
	for _,branch:=range []string{"../main","-bad..ref","feature/foo;whoami","refs//main","branch lock.lock"}{
		if _,err:=remoteRevisionWithFallback(context.Background(),branch,client,nil);err==nil{
			t.Fatalf("unsafe branch accepted: %q",branch)
		}
	}
	if calls!=0{t.Fatalf("invalid branch reached network %d times",calls)}
	if _,err:=remoteRevisionWithFallback(context.Background(),"main",client,nil);err==nil{
		t.Fatal("invalid API SHA accepted")
	}
}

type revisionHTTPRoundTrip func(*http.Request)(*http.Response,error)
func (f revisionHTTPRoundTrip)RoundTrip(req *http.Request)(*http.Response,error){return f(req)}
