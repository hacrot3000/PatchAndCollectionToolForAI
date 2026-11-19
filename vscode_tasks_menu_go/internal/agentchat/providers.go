package agentchat

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"
)

type ModelConfig struct {
 ID string `json:"id"`
 Name string `json:"name"`
 ToolCalling bool `json:"tool_calling"`
 Vision bool `json:"vision"`
 MaxInputTokens int `json:"max_input_tokens,omitempty"`
 MaxOutputTokens int `json:"max_output_tokens,omitempty"`
 ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`
 DefaultReasoningEffort string `json:"default_reasoning_effort,omitempty"`
}
type ProviderConfig struct {
 ID string `json:"id"`
 Name string `json:"name"`
 APIType string `json:"api_type"`
 BaseURL string `json:"base_url"`
 Models []ModelConfig `json:"models"`
}
var providerIDPattern=regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
var modelIDPattern=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/~-]{0,127}$`)
var blockedIPNetworks=func()[]*net.IPNet{
 cidrs:=[]string{"0.0.0.0/8","100.64.0.0/10","192.0.0.0/24","198.18.0.0/15","240.0.0.0/4","2001:db8::/32","2001::/32"}
 out:=make([]*net.IPNet,0,len(cidrs))
 for _,cidr:=range cidrs {_,sub,_:=net.ParseCIDR(cidr);out=append(out,sub)}
 return out
}()
func publicIP(ip net.IP) bool {
 if ip==nil||!ip.IsGlobalUnicast()||ip.IsPrivate()||ip.IsLoopback()||ip.IsLinkLocalUnicast(){return false}
 for _,sub:=range blockedIPNetworks{if sub.Contains(ip){return false}}
 return true
}
func ValidateProvider(p ProviderConfig)error {
 if !providerIDPattern.MatchString(p.ID)||len(p.Name)==0||len(p.Name)>128{return errors.New("invalid provider ID/name")}
 if p.APIType!="responses"{return errors.New("only Responses API is enabled for Agent Chat")}
 u,err:=url.Parse(p.BaseURL)
 if err!=nil||u.Scheme!="https"||u.Host==""||u.Opaque!=""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.RawFragment!=""{
  return errors.New("provider endpoint must be a public HTTPS base URL without credentials/query/fragment")
 }
 if strings.Contains(u.Hostname(),"%")||strings.ContainsAny(u.Hostname()," \t\r\n")||strings.HasSuffix(u.Hostname(),".local")||strings.EqualFold(u.Hostname(),"localhost")||strings.HasSuffix(u.Hostname(),".localhost"){
  return errors.New("local or unsafe provider hostname refused")
 }
 if ip:=net.ParseIP(u.Hostname());ip!=nil&&!publicIP(ip){return errors.New("provider IP must be public")}
 if u.Port()!="" {port,err:=strconv.Atoi(u.Port());if err!=nil||port<1||port>65535{return errors.New("invalid HTTPS port")}}
 if strings.Contains(u.EscapedPath(),"%2f")||strings.Contains(u.EscapedPath(),"%2F")||strings.Contains(u.EscapedPath(),"%5c")||strings.Contains(u.EscapedPath(),"%5C"){return errors.New("encoded path separators refused")}
 if len(p.Models)==0||len(p.Models)>24{return errors.New("provider must declare 1-24 models")}
 seen:=map[string]bool{}
 for _,m:=range p.Models {
  if !modelIDPattern.MatchString(m.ID)||len(m.Name)>128{return errors.New("invalid model ID/name")}
  if seen[m.ID]{return errors.New("duplicate model ID")}
  seen[m.ID]=true
  if m.MaxInputTokens<0||m.MaxInputTokens>4000000||m.MaxOutputTokens<0||m.MaxOutputTokens>200000{return errors.New("invalid model token budget")}
  seenEffort:=map[string]bool{}
  for _,effort:=range m.ReasoningEfforts {
   if !allowedEffort(effort)||seenEffort[effort]{return errors.New("invalid/duplicate reasoning effort")}
   seenEffort[effort]=true
  }
  if m.DefaultReasoningEffort!=""&&!seenEffort[m.DefaultReasoningEffort] {return errors.New("default reasoning effort is not advertised")}
 }
 return nil
}
func allowedEffort(s string)bool {
 switch s{case "none","minimal","low","medium","high","xhigh","max":return true}
 return false
}
func (p ProviderConfig) FindModel(id string)(ModelConfig,bool){
 for _,m:=range p.Models{if m.ID==id{return m,true}}
 return ModelConfig{},false
}
func (p ProviderConfig) ResponsesURL()(string,error){
 if err:=ValidateProvider(p);err!=nil{return "",err}
 u,_:=url.Parse(p.BaseURL)
 path:=strings.TrimRight(u.Path,"/")
 if !strings.HasSuffix(path,"/responses"){path+="/responses"}
 u.Path=path
 u.RawPath=""
 return u.String(),nil
}

// ProviderStore saves public model metadata only. API keys belong in the
// existing encrypted secretstore; provider config JSON MUST NEVER contain one.
type ProviderStore struct{dir string}
var providerStoreMu sync.Mutex
func NewProviderStore(dir string)(*ProviderStore,error){
 if dir==""||!filepath.IsAbs(dir){return nil,errors.New("absolute provider directory required")}
 return &ProviderStore{dir:filepath.Clean(dir)},nil
}
func (s *ProviderStore) file(owner string)(string,error){
 if owner==""||len(owner)>256{return "",errors.New("invalid owner")}
 hash:=sha256.Sum256([]byte(owner))
 return filepath.Join(s.dir,"providers-"+hex.EncodeToString(hash[:16])+".json"),nil
}
func (s *ProviderStore) ensureDir()error {
 if err:=os.MkdirAll(s.dir,0700);err!=nil{return err}
 fi,err:=os.Lstat(s.dir);if err!=nil{return err}
 if !fi.IsDir()||fi.Mode()&os.ModeSymlink!=0||fi.Mode().Perm()&0077!=0{return errors.New("unsafe provider config directory")}
 return nil
}
type providerFile struct {
 Version int `json:"version"`
 Providers []ProviderConfig `json:"providers"`
}
func (s *ProviderStore) read(owner string)(providerFile,error){
 path,err:=s.file(owner);if err!=nil{return providerFile{},err}
 fi,err:=os.Lstat(path)
 if os.IsNotExist(err){return providerFile{Version:1,Providers:[]ProviderConfig{}},nil}
 if err!=nil{return providerFile{},err}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>128<<10{return providerFile{},errors.New("unsafe provider config file")}
 raw,err:=os.ReadFile(path);if err!=nil{return providerFile{},err}
 var p providerFile
 if err=json.Unmarshal(raw,&p);err!=nil{return providerFile{},err}
 if p.Version!=1||len(p.Providers)>24{return providerFile{},errors.New("invalid provider config version/count")}
 names:=map[string]bool{}
 for _,item:=range p.Providers {
  if err=ValidateProvider(item);err!=nil{return providerFile{},err}
  if names[item.ID]{return providerFile{},errors.New("duplicate provider")}
  names[item.ID]=true
 }
 return p,nil
}
func (s *ProviderStore) List(owner string)([]ProviderConfig,error){
 providerStoreMu.Lock();defer providerStoreMu.Unlock()
 if err:=s.ensureDir();err!=nil{return nil,err}
 p,err:=s.read(owner);if err!=nil{return nil,err}
 return p.Providers,nil
}
func (s *ProviderStore) Get(owner,id string)(ProviderConfig,error){
 ps,err:=s.List(owner);if err!=nil{return ProviderConfig{},err}
 for _,p:=range ps{if p.ID==id{return p,nil}}
 return ProviderConfig{},ErrNotFound
}
func (s *ProviderStore) Put(owner string,p ProviderConfig)error{
 if err:=ValidateProvider(p);err!=nil{return err}
 providerStoreMu.Lock();defer providerStoreMu.Unlock()
 if err:=s.ensureDir();err!=nil{return err}
 cfg,err:=s.read(owner);if err!=nil{return err}
 replaced:=false
 for i:=range cfg.Providers{if cfg.Providers[i].ID==p.ID{cfg.Providers[i]=p;replaced=true;break}}
 if !replaced{if len(cfg.Providers)>=24{return errors.New("provider count exceeded")};cfg.Providers=append(cfg.Providers,p)}
 sort.Slice(cfg.Providers,func(i,j int)bool{return cfg.Providers[i].ID<cfg.Providers[j].ID})
 raw,err:=json.Marshal(cfg);if err!=nil{return err}
 if len(raw)>128<<10{return errors.New("provider config size exceeded")}
 path,_:=s.file(owner)
 temp,err:=os.CreateTemp(s.dir,".provider-*");if err!=nil{return err}
 defer os.Remove(temp.Name())
 if err=temp.Chmod(0600);err!=nil{temp.Close();return err}
 if _,err=temp.Write(raw);err!=nil{temp.Close();return err}
 if err=temp.Sync();err!=nil{temp.Close();return err}
 if err=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),path)
}

// DoResponses validates the endpoint then pins each DNS resolution to a public
// address at connection time (DNS rebinding defense). Proxy/redirect disabled.
func DoResponses(ctx context.Context,p ProviderConfig,key string,body []byte)(ResponsesResult,error){
 if key==""||len(key)>8192{return ResponsesResult{},errors.New("provider credential unavailable")}
 endpoint,err:=p.ResponsesURL();if err!=nil{return ResponsesResult{},err}
 if len(body)==0||len(body)>maxRequestBytes{return ResponsesResult{},errors.New("request payload exceeds limit")}
 if !json.Valid(body){return ResponsesResult{},errors.New("invalid provider request payload")}
 dialer:=&net.Dialer{Timeout:8*time.Second}
 resolver:=net.DefaultResolver
 transport:=&http.Transport{
  Proxy:nil,
  TLSHandshakeTimeout:10*time.Second,
  ResponseHeaderTimeout:30*time.Second,
  DisableKeepAlives:true,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   host,port,err:=net.SplitHostPort(address);if err!=nil{return nil,err}
   if ip:=net.ParseIP(host);ip!=nil{
    if !publicIP(ip){return nil,errors.New("private or reserved provider IP refused")}
    return dialer.DialContext(ctx,network,net.JoinHostPort(host,port))
   }
   ips,err:=resolver.LookupIPAddr(ctx,host);if err!=nil{return nil,errors.New("provider DNS lookup failed")}
   // Fail closed on mixed public/private DNS answers.
   if len(ips)==0{return nil,errors.New("provider DNS returned no addresses")}
   for _,entry:=range ips{if !publicIP(entry.IP){return nil,errors.New("provider DNS includes private/reserved IP")}}
   var lastErr error
   for _,entry:=range ips{
    var conn net.Conn
    conn,lastErr=dialer.DialContext(ctx,network,net.JoinHostPort(entry.IP.String(),port))
    if lastErr==nil{return conn,nil}
   }
   return nil,fmt.Errorf("provider connection failed: %w",lastErr)
  },
 }
 defer transport.CloseIdleConnections()
 client:=&http.Client{Transport:transport,Timeout:60*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return errors.New("provider redirect refused")}}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,strings.NewReader(string(body)));if err!=nil{return ResponsesResult{},err}
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Authorization","Bearer "+key)
 resp,err:=client.Do(req);if err!=nil{return ResponsesResult{},errors.New("provider request failed")}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return ResponsesResult{},fmt.Errorf("provider returned HTTP %d",resp.StatusCode)}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,maxRequestBytes+1));if err!=nil{return ResponsesResult{},errors.New("cannot read provider response")}
 if len(raw)>maxRequestBytes{return ResponsesResult{},errors.New("provider response too large")}
 return ParseResponsesResult(raw)
}
