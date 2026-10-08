package server

import (
 "encoding/hex"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestDirectoryCompareHashesSupportCRC32MD5AndSHA256(t *testing.T) {
 cases:=[]struct{name,expected string}{
  {"crc32","cbf43926"},
  {"md5","25f9e794323b453885f5181f1b624d0b"},
  {"sha256","15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   hasher,err:=directoryCompareHasher(tc.name)
   if err!=nil{t.Fatal(err)}
   _,_ = hasher.Write([]byte("123456789"))
   if actual:=hex.EncodeToString(hasher.Sum(nil));actual!=tc.expected{t.Errorf("digest %q, expected %q",actual,tc.expected)}
  })
 }
 if _,err:=directoryCompareHasher("sha1");err==nil{t.Fatal("unsupported hash algorithm must be rejected")}
}

func TestDirectoryCompareHashPermissionsAreSeparated(t *testing.T) {
 cases:=[]struct{path,permission string}{
  {"/api/directory-compare/project-hash",identity.PermissionFilesRead},
  {"/api/directory-compare/remote-hash",identity.PermissionTransferRead},
 }
 for _,tc:=range cases{
  r,err:=http.NewRequest(http.MethodPost,tc.path,strings.NewReader("{}"))
  if err!=nil{t.Fatal(err)}
  actual:=sharedRoutePermissions(r)
  if len(actual)!=1||actual[0]!=tc.permission{t.Errorf("%s: got %v, expected %s",tc.path,actual,tc.permission)}
 }
}
