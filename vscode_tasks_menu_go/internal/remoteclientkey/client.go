package remoteclientkey

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "path/filepath"
 "syscall"
)

type ClientIdentity struct {
 Private ed25519.PrivateKey
 PublicKey string
 Fingerprint string
}
func ClientSeedPath(deviceID,username string)(string,error){
 if len(deviceID)!=64{return "",errors.New("invalid pinned TaskDeck Device ID")}
 if _,err:=hex.DecodeString(deviceID);err!=nil{return "",err}
 if validateUser(username)!=nil{return "",errors.New("invalid client username")}
 base,err:=os.UserConfigDir();if err!=nil{return "",err}
 name:=sha256.Sum256([]byte(username))
 return filepath.Join(base,"taskdeck","remote-client-keys",deviceID+"-"+hex.EncodeToString(name[:8])+".seed"),nil
}
// LoadOrCreateClientIdentity never contacts PHP and never exports a private
// key to tickets, log or UI. Only the public key is offered for host approval.
func LoadOrCreateClientIdentity(deviceID,username string)(ClientIdentity,error){
 var out ClientIdentity
 path,err:=ClientSeedPath(deviceID,username);if err!=nil{return out,err}
 if err=checkDir(filepath.Dir(path),true);err!=nil{return out,err}
 file,err:=openRegular(path)
 var seed []byte
 if os.IsNotExist(err) {
  seed=make([]byte,ed25519.SeedSize)
  if _,err=io.ReadFull(rand.Reader,seed);err!=nil{return out,err}
  fd,createErr:=syscall.Open(path,syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0600)
  if createErr==syscall.EEXIST {
   for i:=range seed{seed[i]=0}
   file,err=openRegular(path)
   if err!=nil{return out,err}
  } else if createErr!=nil{return out,createErr
  } else {
   f:=os.NewFile(uintptr(fd),path)
   _,err=f.Write(seed)
   if err==nil{err=f.Sync()}
   closeErr:=f.Close()
   if err==nil{err=closeErr}
   if err!=nil{return out,err}
   dir,dirErr:=os.Open(filepath.Dir(path));if dirErr!=nil{return out,dirErr}
   syncErr:=dir.Sync();closeErr=dir.Close()
   if syncErr!=nil{return out,syncErr}
   if closeErr!=nil{return out,closeErr}
  }
 } else if err!=nil{return out,err}
 if file!=nil{
  seed,err=io.ReadAll(io.LimitReader(file,33))
  closeErr:=file.Close()
  if err!=nil{return out,err}
  if closeErr!=nil{return out,closeErr}
 }
 if len(seed)!=ed25519.SeedSize{return out,errors.New("invalid stored client Ed25519 seed")}
 priv:=ed25519.NewKeyFromSeed(seed)
 for i:=range seed{seed[i]=0}
 pub:=base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
 fp,err:=Fingerprint(pub);if err!=nil{return out,err}
 return ClientIdentity{Private:priv,PublicKey:pub,Fingerprint:fp},nil
}
func (client ClientIdentity) SignLoginProof(username string,binding []byte)(string,error){
 if len(client.Private)!=ed25519.PrivateKeySize||len(binding)!=32||validateUser(username)!=nil{return "",errors.New("invalid bound client proof")}
 return base64.StdEncoding.EncodeToString(ed25519.Sign(client.Private,ProofMessage(username,binding))),nil
}
