package remoteclientkey

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func testStore(t *testing.T) string {
 t.Helper()
 dir:=filepath.Join(t.TempDir(),"private")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 return filepath.Join(dir,"approved.json")
}
func testPair(t *testing.T)(string,ed25519.PrivateKey){
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 return base64.StdEncoding.EncodeToString(pub),priv
}
func TestHostKeyApprovalRejectsCompromisedPortalAndUnapprovedClients(t *testing.T){
 path:=testStore(t)
 pub,priv:=testPair(t)
 binding:=sha256.Sum256([]byte("fresh-E2EE-transcript"))
 sig:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,ProofMessage("alice",binding[:])))
 if Verify(path,"alice",pub,sig,binding[:])==nil{t.Fatal("P0: unapproved client authenticated")}
 e,err:=Add(path,"alice",pub,"My Linux laptop");if err!=nil{t.Fatal(err)}
 if err:=Verify(path,"alice",pub,sig,binding[:]);err!=nil{t.Fatal(err)}
 if err:=Verify(path,"bob",pub,sig,binding[:]);err==nil{t.Fatal("P0: approved key crossed TaskDeck local accounts")}
 wrong:=sha256.Sum256([]byte("different-E2EE-transcript"))
 if err:=Verify(path,"alice",pub,sig,wrong[:]);err==nil{t.Fatal("P0: signature replay from other E2EE channel")}
 if err:=Verify(path,"alice",pub,base64.StdEncoding.EncodeToString(make([]byte,64)),binding[:]);err==nil{t.Fatal("P0: attacker without private key was accepted")}
 attackerPub,attackerPrivate:=testPair(t)
 fake:=base64.StdEncoding.EncodeToString(ed25519.Sign(attackerPrivate,ProofMessage("alice",binding[:])))
 if err:=Verify(path,"alice",attackerPub,fake,binding[:]);err==nil{t.Fatal("P0: PHP attacker-created key was accepted")}
 if err:=Remove(path,e.Fingerprint);err!=nil{t.Fatal(err)}
 if Verify(path,"alice",pub,sig,binding[:])==nil{t.Fatal("P0: revoked key still admitted")}
}
func TestHostKeyRegistryFailsClosedForTamperedOrInsecureFiles(t *testing.T){
 path:=testStore(t)
 pub,priv:=testPair(t)
 if _,err:=Add(path,"alice",pub,"one");err!=nil{t.Fatal(err)}
 fp,_:=Fingerprint(pub)
 if _,err:=Add(path,"alice",pub,"same");err!=nil{t.Fatal(err)}
 entries,err:=List(path)
 if err!=nil||len(entries)!=1||entries[0].Fingerprint!=fp{t.Fatalf("dedup failed: %v %#v",err,entries)}
 if _,err:=Add(path,"bob",pub,"other");err==nil{t.Fatal("same public key was allowed for two accounts")}
 binding:=sha256.Sum256([]byte("session"))
 sig:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,ProofMessage("alice",binding[:])))
 if err:=os.WriteFile(path,[]byte("{corrupt"),0600);err!=nil{t.Fatal(err)}
 if Verify(path,"alice",pub,sig,binding[:])==nil{t.Fatal("corrupt registry accepted signature")}
 if err:=os.WriteFile(path,[]byte("{}"),0644);err!=nil{t.Fatal(err)}
 if err:=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
 if _,err:=List(path);err==nil{t.Fatal("world-readable registry accepted")}
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(filepath.Join(filepath.Dir(path),"not-registry"),path);err!=nil{t.Fatal(err)}
 if _,err:=Add(path,"alice",pub,"bad");err==nil{t.Fatal("symlink registry replaced")}
}
func TestHostKeyRejectsMalformedPublicKeysAndInvalidNames(t *testing.T){
 path:=testStore(t)
 for _,value:=range []string{"","plaintext",base64.StdEncoding.EncodeToString(make([]byte,31))}{
  if _,err:=Add(path,"alice",value,"");err==nil{t.Fatalf("invalid public key accepted: %q",value)}
 }
 pub,_:=testPair(t)
 for _,name:=range []string{""," alice","alice\nadmin",strings.Repeat("a",129)}{
  if _,err:=Add(path,name,pub,"");err==nil{t.Fatalf("invalid user accepted: %q",name)}
 }
 digest:=sha256.Sum256([]byte("safe-name"))
 if _,err:=hex.DecodeString(hex.EncodeToString(digest[:]));err!=nil{t.Fatal(err)}
}
