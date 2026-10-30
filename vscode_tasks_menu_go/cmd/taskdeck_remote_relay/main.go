// TaskDeck Remote Relay prototype: HTTPS/WSS rendezvous only.
// No HTTP proxying, command execution, terminal or file forwarding.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remoterelay"
)

func main() {
	listen:=flag.String("listen","127.0.0.1:8443","TCP listen address")
	issuer:=flag.String("portal-issuer","","exact Portal HTTPS origin")
	relayOrigin:=flag.String("relay-origin","","exact public Relay HTTPS origin")
 allowOfflineClient:=flag.Bool("allow-offline-file-client",false,"opt-in file:// Firefox offline E2EE diagnostics only (Origin:null is not client identity)")
 allowAppPing:=flag.Bool("allow-app-ping",false,"opt-in encrypted session.ping ONLY; never enables arbitrary TaskDeck commands")
 allowOfflineLogin:=flag.Bool("allow-offline-login-probe",false,"optional one-shot E2EE password check after signed Agent and host-approved client-key preflight; no remote commands")
	keyFile:=flag.String("portal-public-key-file","","absolute file containing base64 Portal Ed25519 public key")
	replayDir:=flag.String("ticket-replay-dir","","private absolute directory for consumed ticket IDs (required)")
	cert:=flag.String("tls-cert","","PEM TLS certificate")
	key:=flag.String("tls-key","","PEM TLS private key")
	flag.Parse()
 if *allowOfflineLogin && !*allowOfflineClient {fatal(errors.New("offline login probe requires -allow-offline-file-client; offline route remains disabled by default"))}
	if *issuer==""||*relayOrigin==""||*keyFile==""||*replayDir==""||*cert==""||*key=="" {
		flag.Usage()
		os.Exit(2)
	}
	data,err:=os.ReadFile(*keyFile)
	if err!=nil {fatal(err)}
	tickets,err:=remoterelay.NewVerifier(strings.TrimSpace(string(data)),*issuer)
	if err!=nil {fatal(err)}
	ledger,err:=remoterelay.NewReplayLedger(*replayDir)
	if err!=nil {fatal(err)}
	if err:=tickets.ConfigureReplayLedger(ledger);err!=nil {fatal(err)}
	devices,err:=remoterelay.NewRegistry(*relayOrigin)
	if err!=nil {fatal(err)}
	server:=&http.Server{
		Addr:*listen,
		Handler:&remoterelay.Handler{Tickets:tickets,Devices:devices,AllowOfflineFileClient:*allowOfflineClient,AllowOfflineLoginProbe:*allowOfflineLogin,AllowAppPing:*allowAppPing},
		ReadHeaderTimeout:10*time.Second,
		IdleTimeout:2*time.Minute,
		MaxHeaderBytes:16<<10,
		TLSConfig:&tls.Config{MinVersion:tls.VersionTLS12},
	}
	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer stop()
	go func(){
		<-ctx.Done()
		shutdownCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("TaskDeck Remote Relay rendezvous listening on %s; command forwarding DISABLED",*listen)
	err=server.ListenAndServeTLS(*cert,*key)
	if err!=nil&&!errors.Is(err,http.ErrServerClosed){fatal(err)}
}
func fatal(err error) {
	fmt.Fprintln(os.Stderr,"TaskDeck Relay:",err)
	os.Exit(1)
}
