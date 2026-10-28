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


func TestRemoteRevisionPrefersGitHTTPSAndSkipsAPI(t *testing.T) {
	const sha = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	gitCalls, apiCalls := 0, 0
	got, err := remoteRevisionPreferGit(context.Background(), "feature/test",
		func(_ context.Context, branch string) (string, error) {
			gitCalls++
			if branch != "feature/test" { t.Fatalf("branch=%q", branch) }
			return sha, nil
		},
		func(context.Context, string) (string, error) {
			apiCalls++
			return "", errors.New("API must not be used when Git HTTPS works")
		})
	if err != nil || got != sha || gitCalls != 1 || apiCalls != 0 {
		t.Fatalf("got=%q err=%v git=%d api=%d", got, err, gitCalls, apiCalls)
	}
}

func TestRemoteRevisionFallsBackToAPIIfGitUnavailable(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	got, err := remoteRevisionPreferGit(context.Background(), "main",
		func(context.Context, string) (string, error) { return "", errors.New("git unavailable") },
		func(_ context.Context, branch string) (string, error) {
			if branch != "main" { t.Fatalf("branch=%q", branch) }
			return sha, nil
		})
	if err != nil || got != sha { t.Fatalf("got=%q err=%v", got, err) }
}

func TestRemoteRevisionRejectsInvalidGitSHAWithoutAPIFallback(t *testing.T) {
	_, err := remoteRevisionPreferGit(context.Background(), "main",
		func(context.Context, string) (string, error) { return "bad-sha", nil },
		func(context.Context, string) (string, error) {
			t.Fatal("must reject malformed Git success instead of falling back")
			return "", nil
		})
	if err == nil || !strings.Contains(err.Error(), "invalid revision") {
		t.Fatalf("err=%v", err)
	}
}

func TestGitRemoteBranchesParsingIsBoundedAndExact(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	lines := sha + "\trefs/heads/main\n" +
		sha + "\trefs/heads/feature/compact\n"
	branches, err := parseGitRemoteBranches([]byte(lines))
	if err != nil || len(branches) != 2 ||
		branches[0] != "feature/compact" || branches[1] != "main" {
		t.Fatalf("branches=%v err=%v", branches, err)
	}
	for _, malformed := range []string{
		sha + "\trefs/tags/v1\n",
		sha + "\trefs/heads/main\n" + sha + "\trefs/heads/main\n",
		"not-sha\trefs/heads/main\n",
	} {
		if _, err := parseGitRemoteBranches([]byte(malformed)); err == nil {
			t.Fatalf("malformed git remote ref accepted: %q", malformed)
		}
	}
}

func TestRemoteBranchesPrefersGitThenFallbackAPI(t *testing.T) {
	gitCalls, apiCalls := 0, 0
	got, err := remoteBranchesPreferGit(context.Background(),
		func(context.Context) ([]string, error) {
			gitCalls++
			return []string{"main", "develop"}, nil
		},
		func(context.Context) ([]string, error) {
			apiCalls++
			return []string{"other"}, nil
		})
	if err != nil || gitCalls != 1 || apiCalls != 0 || len(got) != 2 {
		t.Fatalf("branches=%v git=%d api=%d err=%v", got, gitCalls, apiCalls, err)
	}
	got, err = remoteBranchesPreferGit(context.Background(),
		func(context.Context) ([]string, error) { return nil, errors.New("git blocked") },
		func(context.Context) ([]string, error) {
			apiCalls++
			return []string{"main"}, nil
		})
	if err != nil || len(got) != 1 || got[0] != "main" || apiCalls != 1 {
		t.Fatalf("fallback branches=%v api=%d err=%v", got, apiCalls, err)
	}
}
