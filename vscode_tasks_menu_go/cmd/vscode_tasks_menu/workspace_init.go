package main

import (
 "bufio"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/workspaceinit"
)

// bootstrapWorkspaceTasks is limited to direct interactive launches (or an
// explicit --init-tasks). Daemon restarts, self-update, internal subprocesses
// and remote requests must not ask for input or create a project silently.
func bootstrapWorkspaceTasks(value string, in io.Reader, out io.Writer, interactive, explicit bool) error {
 if strings.TrimSpace(value)==""{
  cwd,err:=os.Getwd();if err!=nil{return err};value=cwd
 }
 root,err:=filepath.Abs(value);if err!=nil{return err}
 stat,err:=os.Stat(root);if err!=nil{return err}
 if !stat.IsDir(){return fmt.Errorf("workspace không phải thư mục: %s",root)}
 target:=filepath.Join(root,".vscode","tasks.json")
 _,err=os.Lstat(target)
 if err==nil{return nil} // Preserve existing tasks.json, even if invalid.
 if !errors.Is(err,os.ErrNotExist){return fmt.Errorf("kiểm tra %s: %w",target,err)}
 if !explicit && !interactive {
  return fmt.Errorf("không tìm thấy %s\nChạy tương tác bằng taskdeck hoặc dùng --init-tasks để tạo project mới",target)
 }
 if !explicit {
  fmt.Fprintf(out,"Không tìm thấy %s\n",target)
  fmt.Fprint(out,"Bạn có muốn quét project và tạo .vscode/tasks.json với các task mẫu? [y/N]: ")
  line,err:=bufio.NewReader(io.LimitReader(in,256)).ReadString('\n')
  if err!=nil&&err!=io.EOF{return fmt.Errorf("đọc lựa chọn: %w",err)}
  answer:=strings.ToLower(strings.TrimSpace(line))
  if answer!="y"&&answer!="yes"&&answer!="c"&&answer!="co"&&answer!="có" {
   return fmt.Errorf("đã hủy tạo tasks.json, không thay đổi workspace %s",root)
  }
 }
 fmt.Fprintln(out,"Đang nhận diện ngôn ngữ/framework/build tool (quét tối đa 3 cấp thư mục)...")
 proposal,err:=workspaceinit.Detect(root)
 if err!=nil{return fmt.Errorf("quét project: %w",err)}
 if len(proposal.Detections)==0 {
  fmt.Fprintln(out,"Không tìm thấy ngôn ngữ được nhận diện; tạo một task cơ bản có thể chỉnh sửa.")
 }else {
  fmt.Fprintln(out,"Các nhóm được nhận diện:")
  for _,d:=range proposal.Detections {
   fmt.Fprintf(out,"  • %s [%s] — %s\n",d.Kind,d.Folder,d.Evidence)
  }
 }
 file,err:=workspaceinit.Create(root,proposal)
 if err!=nil{return fmt.Errorf("khởi tạo project: %w",err)}
 fmt.Fprintf(out,"Đã tạo %s (%d task mẫu). Có thể chỉnh sửa trong TaskDeck hoặc VS Code.\n",file,len(proposal.Tasks))
 return nil
}
