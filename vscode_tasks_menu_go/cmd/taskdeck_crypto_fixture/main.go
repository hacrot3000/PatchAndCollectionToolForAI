// taskdeck_crypto_fixture is a CI-only deterministic-protocol interoperability
// helper. It generates temporary device keys, never accesses TaskDeck secrets,
// and NEVER forwards local HTTP, terminal or file commands.
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

func main() {
	var request struct {
		BrowserKey string `json:"browser_key"`
		ProbeID string `json:"probe_id"`
		ClientNonce string `json:"client_nonce"`
	}
	decoder:=json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&request);err!=nil{fatal(err)}
	publicKey,err:=base64.StdEncoding.Strict().DecodeString(request.BrowserKey)
	if err!=nil{fatal(err)}
	public,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{fatal(err)}
	hash:=sha256.Sum256(public)
	device:=hex.EncodeToString(hash[:])
	hello,channel,err:=remotetransport.ClientHello(private,device,request.ProbeID,request.ClientNonce,publicKey)
	if err!=nil{fatal(err)}
	encrypted,err:=channel.Seal([]byte("taskdeck-e2ee-ready-v1"))
	if err!=nil{fatal(err)}
	result:=map[string]any{
		"device_id":device,
		"crypto":map[string]string{
			"identity":base64.StdEncoding.EncodeToString(hello.Identity),
			"ephemeral":base64.StdEncoding.EncodeToString(hello.Ephemeral),
			"signature":base64.StdEncoding.EncodeToString(hello.Signature),
			"encrypted_ready":base64.StdEncoding.EncodeToString(encrypted),
		},
	}
	if err:=json.NewEncoder(os.Stdout).Encode(result);err!=nil{fatal(err)}
}
func fatal(err error) {
	fmt.Fprintln(os.Stderr,err)
	os.Exit(1)
}
