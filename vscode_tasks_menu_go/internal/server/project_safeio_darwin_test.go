//go:build darwin && go1.25

package server

import ("os"; "path/filepath"; "testing")

func TestDarwinPinnedFileRejectsSwappedParent(t *testing.T) {
    root := t.TempDir()
    sub := filepath.Join(root,"sub")
    if err:=os.Mkdir(sub,0700);err!=nil{t.Fatal(err)}
    target:=filepath.Join(sub,"file.txt")
    if err:=os.WriteFile(target,[]byte("inside"),0600);err!=nil{t.Fatal(err)}
    outside:=t.TempDir()
    if err:=os.WriteFile(filepath.Join(outside,"file.txt"),[]byte("outside"),0600);err!=nil{t.Fatal(err)}
    if err:=os.Rename(sub,sub+"-moved");err!=nil{t.Fatal(err)}
    if err:=os.Symlink(outside,sub);err!=nil{t.Fatal(err)}
    if p,err:=openProjectPinnedFile(root,target);err==nil{
        p.close(); t.Fatal("accepted parent symlink escaping workspace")
    }
}

func TestDarwinPinnedFileAtomicSaveAndRejectLeafSymlink(t *testing.T) {
    root:=t.TempDir();target:=filepath.Join(root,"file.txt")
    if err:=os.WriteFile(target,[]byte("before"),0640);err!=nil{t.Fatal(err)}
    p,err:=openProjectPinnedFile(root,target);if err!=nil{t.Fatal(err)};defer p.close()
    _,info,err:=p.readCurrent(1024);if err!=nil{t.Fatal(err)}
    if err:=p.writeTemp([]byte("after"),info);err!=nil{t.Fatal(err)}
    if err:=p.commitTemp();err!=nil{t.Fatal(err)}
    data,err:=os.ReadFile(target);if err!=nil||string(data)!="after"{t.Fatalf("got %q: %v",data,err)}
    saved,err:=os.Stat(target);if err!=nil||saved.Mode().Perm()!=0640{t.Fatalf("mode=%v error=%v",saved,err)}
    outside:=filepath.Join(t.TempDir(),"outside.txt")
    if err:=os.WriteFile(outside,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
    if err:=os.Remove(target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(outside,target);err!=nil{t.Fatal(err)}
    if _,_,err:=p.readCurrent(1024);err==nil{t.Fatal("accepted leaf symlink")}
}
