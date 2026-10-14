package server

import (
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "strings"
)

const aiDefaultProvider = "openai"

// AI providers have fixed HTTPS endpoints. Browser-supplied URLs are NEVER
// accepted; this is intentionally not a general-purpose HTTP proxy.
type aiProviderSpec struct {
 endpoint string
 defaultModel string
 nativeMessages bool
}

func aiProviderConfig(provider string) (aiProviderSpec,error) {
 switch provider {
 case "openai":
  return aiProviderSpec{endpoint:"https://api.openai.com/v1/chat/completions",defaultModel:"gpt-5.4-mini"},nil
 case "anthropic":
  return aiProviderSpec{endpoint:"https://api.anthropic.com/v1/messages",defaultModel:"claude-sonnet-5-5",nativeMessages:true},nil
 case "gemini":
  return aiProviderSpec{endpoint:"https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",defaultModel:"gemini-3.8-flash"},nil
 case "deepseek":
  return aiProviderSpec{endpoint:"https://api.deepseek.com/chat/completions",defaultModel:"deepseek-chat"},nil
 case "groq":
  return aiProviderSpec{endpoint:"https://api.groq.com/openai/v1/chat/completions",defaultModel:"llama-3.3-70b-versatile"},nil
 case "openrouter":
  return aiProviderSpec{endpoint:"https://openrouter.ai/api/v1/chat/completions",defaultModel:"openai/gpt-5.4-mini"},nil
 default:
  return aiProviderSpec{},errors.New("unsupported AI provider")
 }
}

// Preserve existing OpenAI keys saved prior to multi-provider support.
func aiProviderSecretID(workspace, provider string) string {
 if provider=="openai" {return aiSecretID(workspace)}
 return "ai/provider/"+aiWorkspaceHash(workspace)+"/"+provider
}

func aiProviderRequest(provider, model, instructions, contextText string) ([]byte,error) {
 spec,err:=aiProviderConfig(provider)
 if err!=nil{return nil,err}
 if spec.nativeMessages {
  return json.Marshal(map[string]any{
   "model":model,
   "max_tokens":1200,
   "system":instructions,
   "messages":[]map[string]string{{"role":"user","content":contextText}},
  })
 }
 payload:=map[string]any{
  "model":model,
  "messages":[]map[string]string{{"role":"system","content":instructions},{"role":"user","content":contextText}},
 }
 if provider=="openai"{
  // Current reasoning-capable models prefer max_completion_tokens.
  payload["max_completion_tokens"]=1200
 }else{
  payload["max_tokens"]=1200
 }
 return json.Marshal(payload)
}

func aiProviderHeaders(request *http.Request, provider string,key []byte) {
 request.Header.Set("Content-Type","application/json")
 if provider=="anthropic" {
  request.Header.Set("x-api-key",string(key))
  request.Header.Set("anthropic-version","2023-06-01")
 } else {
  request.Header.Set("Authorization","Bearer "+string(key))
 }
}

func aiProviderAnswer(provider string, body []byte)(string,error){
 if provider=="anthropic"{
  var result struct{ Content []struct{Type string `json:"type"`; Text string `json:"text"`} `json:"content"` }
  if json.Unmarshal(body,&result)!=nil{return "",errors.New("invalid Claude response")}
  pieces:=make([]string,0,len(result.Content))
  for _,item:=range result.Content{if item.Type=="text"&&item.Text!=""{pieces=append(pieces,item.Text)}}
  if len(pieces)==0{return "",errors.New("Claude returned no text")}
  return strings.Join(pieces,"\n"),nil
 }
 var result struct{ Choices []struct{Message struct{Content string `json:"content"`} `json:"message"`} `json:"choices"` }
 if json.Unmarshal(body,&result)!=nil||len(result.Choices)==0||strings.TrimSpace(result.Choices[0].Message.Content)==""{
  return "",fmt.Errorf("%s returned no readable text",provider)
 }
 return result.Choices[0].Message.Content,nil
}
