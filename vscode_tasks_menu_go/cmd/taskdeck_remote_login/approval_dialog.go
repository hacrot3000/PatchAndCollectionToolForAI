package main

import (
 "fmt"
 "os"
 "os/exec"
 "runtime"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
)

// showPendingClientKey displays PUBLIC material only in an OS-owned UI.
// It NEVER opens PHP-supplied HTML and never shows or copies the private seed.
// Native dialog availability is best-effort; terminal output is mandatory.
func showPendingClientKey(tty *os.File,username string,client remoteclientkey.ClientIdentity){
 if tty==nil{return}
 fmt.Fprintln(tty,"\nCLIENT KEY NOT APPROVED / Khóa client chưa được duyệt")
 fmt.Fprintln(tty,"TaskDeck user:",username)
 fmt.Fprintln(tty,"Client public key (copy this, NEVER private key):")
 fmt.Fprintln(tty,client.PublicKey)
 fmt.Fprintln(tty,"Fingerprint:",client.Fingerprint)
 fmt.Fprintln(tty,"On the TaskDeck host, under the same Unix user as the TaskDeck daemon:")
 fmt.Fprintf(tty,"taskdeck remote-key --workspace /path/to/project add %q %q\n",username,client.PublicKey)
 fmt.Fprintln(tty,"Then retry this independent native client. PHP Portal cannot approve this key.")
 // No shell invocation; a malicious Portal cannot alter this trusted dialog.
 if runtime.GOOS=="darwin" {
  message:="Khóa client chưa được thêm vào TaskDeck. Copy public key và dùng CLI trên host để duyệt."
  script:=fmt.Sprintf("display dialog %q default answer %q buttons {\"Đóng\"} default button \"Đóng\" with title \"TaskDeck Remote key approval\"",message,client.PublicKey)
  _=exec.Command("osascript","-e",script).Run()
 } else if runtime.GOOS=="linux" {
  if _,err:=exec.LookPath("zenity");err==nil {
   _=exec.Command("zenity","--entry","--title=TaskDeck Remote key approval",
    "--text=Client key chưa được duyệt. Copy public key bên dưới để thêm từ CLI trên host:",
    "--entry-text="+client.PublicKey,"--width=700").Run()
  } else if _,err:=exec.LookPath("kdialog");err==nil {
   _=exec.Command("kdialog","--inputbox","Client key chưa được duyệt. Copy public key:",client.PublicKey).Run()
  }
 }
 _=strings.TrimSpace // reserve no unexpected shell interpolation
}
