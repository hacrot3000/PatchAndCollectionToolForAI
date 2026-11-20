// An isolated cryptographic interoperability fixture. This CLI never
// imports the TaskDeck server, identity database or HTTP handler.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func main(){
	dec:=json.NewDecoder(os.Stdin)
	var init struct{
		BrowserKey string `json:"browser_key"`
		ProbeID string `json:"probe_id"`
		ClientNonce string `json:"client_nonce"`
	}
	if dec.Decode(&init)!=nil {fail()}
	public,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{fail()}
	digest:=sha256.Sum256(public)
	device:=hex.EncodeToString(digest[:])
	browserKey,err:=base64.StdEncoding.Strict().DecodeString(init.BrowserKey)
	if err!=nil{fail()}
	hello,agent,err:=remotetransport.ClientHello(private,device,init.ProbeID,init.ClientNonce,browserKey)
	if err!=nil{fail()}
	ready,err:=agent.Seal([]byte("taskdeck-e2ee-ready-v1"))
	if err!=nil{fail()}
	if err:=json.NewEncoder(os.Stdout).Encode(map[string]any{
		"device_id":device,
		"crypto":map[string]string{
			"identity":base64.StdEncoding.EncodeToString(hello.Identity),
			"ephemeral":base64.StdEncoding.EncodeToString(hello.Ephemeral),
			"signature":base64.StdEncoding.EncodeToString(hello.Signature),
			"encrypted_ready":base64.StdEncoding.EncodeToString(ready),
		},
	});err!=nil{fail()}
	var message struct{
		Type string `json:"type"`
		Ciphertext string `json:"ciphertext"`
	}
	if dec.Decode(&message)!=nil || message.Type!="e2ee_login" ||
		len(message.Ciphertext)>9000 {fail()}
	raw,err:=base64.StdEncoding.Strict().DecodeString(message.Ciphertext)
	if err!=nil{fail()}
	credentials,err:=remotetransport.OpenRemoteLogin(agent,raw)
	if err!=nil{fail()}
	// Test-only constant, NOT a real TaskDeck account. The production
	// verifier lives in internal/server and independently exercises SQLite.
	accepted:=credentials.Username=="alice" && credentials.Password=="test-only-e2ee-password"
	credentials.Password=""
	sealed,err:=remotetransport.SealRemoteLoginResult(agent,accepted)
	if err!=nil{fail()}
	if err:=json.NewEncoder(os.Stdout).Encode(map[string]string{
		"type":"e2ee_login_result",
		"ciphertext":base64.StdEncoding.EncodeToString(sealed),
	});err!=nil{fail()}
}

func fail() {
	fmt.Fprintln(os.Stderr,"invalid encrypted login fixture frame")
	os.Exit(1)
}
