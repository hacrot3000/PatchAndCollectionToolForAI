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


func TestClientIdentitySeedStableAndNeverLeavesOwnerPrivateDirectory(t *testing.T){
 home:=t.TempDir()
 t.Setenv("XDG_CONFIG_HOME",home)
 device:=strings.Repeat("a",64)
 path,err:=ClientSeedPath(device,"alice");if err!=nil{t.Fatal(err)}
 original,err:=LoadOrCreateClientIdentity(device,"alice");if err!=nil{t.Fatal(err)}
 recovered,err:=LoadOrCreateClientIdentity(device,"alice");if err!=nil{t.Fatal(err)}
 if original.PublicKey!=recovered.PublicKey || original.Fingerprint!=recovered.Fingerprint{
  t.Fatal("stable client identity lost after restart")
 }
 info,err:=os.Lstat(path)
 if err!=nil{t.Fatal(err)}
 if info.Mode().Perm()!=0600{t.Fatalf("private seed permissions %#o",info.Mode().Perm())}
 dirInfo,err:=os.Lstat(filepath.Dir(path))
 if err!=nil{t.Fatal(err)}
 if dirInfo.Mode().Perm()!=0700{t.Fatalf("client directory permissions %#o",dirInfo.Mode().Perm())}
 alternative,err:=LoadOrCreateClientIdentity(strings.Repeat("b",64),"alice")
 if err!=nil{t.Fatal(err)}
 if alternative.PublicKey==original.PublicKey{t.Fatal("software key reused for a different pinned Agent")}
 binding:=sha256.Sum256([]byte("bound-key"))
 sig,err:=original.SignLoginProof("alice",binding[:])
 if err!=nil{t.Fatal(err)}
 if _,err:=Add(testStore(t),"alice",original.PublicKey,"client");err!=nil{t.Fatal(err)}
 if len(sig)==0{t.Fatal("missing client proof signature")}
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/dev/null",path);err!=nil{t.Fatal(err)}
 if _,err:=LoadOrCreateClientIdentity(device,"alice");err==nil{
  t.Fatal("client followed attacker-controlled seed symlink")
 }
}

 
// SQLite's built-in NOCASE only folds ASCII. Go EqualFold would otherwise
// equate these DISTINCT SQLite local accounts. A compromised PHP Portal may
// know the victim's password and possess a legitimately approved key for a
// Unicode-confusable *other* account; that cannot satisfy the victim's MFA.
func TestApprovedClientKeyCannotCrossSQLiteDistinctUnicodeUsernames(t *testing.T){
 for _,tc:=range []struct{
  approved string
  victim string
  equalUnderSQLite bool
 }{
  {approved:"Karl",victim:"Karl",equalUnderSQLite:false},
  {approved:"Émilie",victim:"émilie",equalUnderSQLite:false},
  {approved:"Alice",victim:"ALICE",equalUnderSQLite:true},
 }{
  t.Run(tc.approved+"_to_"+tc.victim,func(t *testing.T){
   path:=testStore(t)
   pub,priv:=testPair(t)
   if _,err:=Add(path,tc.approved,pub,"locally authorized");err!=nil{t.Fatal(err)}
   challenge:=sha256.Sum256([]byte("fresh E2EE transcript for this test"))
   forgedForVictim:=base64.StdEncoding.EncodeToString(
    ed25519.Sign(priv,ProofMessage(tc.victim,challenge[:])))
   verified:=Verify(path,tc.victim,pub,forgedForVictim,challenge[:])==nil
   if verified!=tc.equalUnderSQLite{
    t.Fatalf("approved key for %q crossed SQLite account boundary to %q: accepted=%v",
     tc.approved,tc.victim,verified)
   }
   if SameTaskDeckUsername(tc.approved,tc.victim)!=tc.equalUnderSQLite{
    t.Fatal("local host username equality disagrees with SQLite NOCASE")
   }
   if !tc.equalUnderSQLite {
    if strings.EqualFold(tc.approved,tc.victim)!=true{
     t.Fatal("test no longer exercises Unicode-vs-SQLite case folding mismatch")
    }
    if string(ProofMessage(tc.approved,challenge[:]))==
     string(ProofMessage(tc.victim,challenge[:])){
     t.Fatal("distinct SQLite account names generate identical proof message")
    }
   }
  })
 }
}


func TestHostKeyRegistryRejectsDuplicateAndCaseAliasedJSONMembers(t *testing.T){
 path:=testStore(t)
 public,priv:=testPair(t)
 if _,err:=Add(path,"alice",public,"trusted laptop");err!=nil{t.Fatal(err)}
 good,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 goodJSON:=string(good)
 binding:=sha256.Sum256([]byte("current signed Agent E2EE session"))
 signature:=base64.StdEncoding.EncodeToString(
  ed25519.Sign(priv,ProofMessage("alice",binding[:])))
 if err:=Verify(path,"alice",public,signature,binding[:]);err!=nil{t.Fatal(err)}
 cases:=map[string]string{
  "duplicate root version":strings.Replace(goodJSON,
   "\"version\": 1","\"version\": 1, \"version\": 1",1),
  "root casefold alias":strings.Replace(goodJSON,"\"version\":","\"Version\":",1),
  "duplicate root keys":strings.Replace(goodJSON,
   "\"keys\": [","\"keys\": [], \"keys\": [",1),
  "duplicate nested username":strings.Replace(goodJSON,
   "\"username\": \"alice\"","\"username\": \"alice\", \"username\": \"bob\"",1),
  "nested uppercase alias":strings.Replace(goodJSON,
   "\"username\":","\"Username\":",1),
  "duplicate nested public key":strings.Replace(goodJSON,
   "\"public_key\":","\"public_key\": \"attacker\", \"public_key\":",1),
  "unknown nested field":strings.Replace(goodJSON,
   "\"fingerprint\":","\"is_admin\": true, \"fingerprint\":",1),
  "trailing JSON":goodJSON+"\n{\"version\":1}",
 }
 for name,body:=range cases {
  t.Run(name,func(t *testing.T){
   if body==goodJSON{t.Fatal("malformed fixture unchanged")}
   if err:=os.WriteFile(path,[]byte(body),0600);err!=nil{t.Fatal(err)}
   if _,err:=List(path);err==nil {
    t.Fatal("ambiguous local client allowlist was silently accepted")
   }
   if err:=Verify(path,"alice",public,signature,binding[:]);err==nil {
    t.Fatal("P0: malformed local client-key registry still authenticated")
   }
  })
 }
}
