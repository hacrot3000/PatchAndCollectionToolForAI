package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

// A resumed download MUST prove that the remote bytes preceding the resume
// point still match the private partial file. Size alone is not evidence:
// rotated/replaced log files may be larger than the old version.
const liveFileOverlap int64 = 64 << 10
const liveFileCatchupRounds = 5

var errLiveSourceChanged = errors.New("remote file changed or rotated: overlap does not match previous download; partial kept, restart as a NEW download instead of appending")

func readPartialOverlap(f *os.File, offset int64) ([]byte,int64,error) {
	if offset<=0 { return nil,0,nil }
	start:=offset-liveFileOverlap
	if start<0 {start=0}
	tail:=make([]byte,offset-start)
	n,err:=f.ReadAt(tail,start)
	if err!=nil || n!=len(tail) {return nil,0,fmt.Errorf("read saved overlap: %w",err)}
	return tail,start,nil
}

// The comparison is made BEFORE writing any new bytes to the partial file.
// The first (up to) 64 KiB from REST offset must match the previous tail.
type verifiedAppendWriter struct {
	dst io.Writer
	tail []byte
	checked int
}
func (w *verifiedAppendWriter) Write(p []byte)(int,error){
	saved:=len(p)
	if w.checked<len(w.tail){
		needed:=len(w.tail)-w.checked
		if needed>len(p){needed=len(p)}
		if !bytes.Equal(w.tail[w.checked:w.checked+needed],p[:needed]){
			return 0,errLiveSourceChanged
		}
		w.checked+=needed
		p=p[needed:]
	}
	if len(p)==0{return saved,nil}
	n,err:=w.dst.Write(p)
	return saved-len(p)+n,err
}
func (w *verifiedAppendWriter) validated() bool {return w.checked==len(w.tail)}

func (s *Server) ftpAppendVerified(ctx context.Context, profile filetransferprofile.Profile, remotePath string, part *os.File, offset int64, progress func(int64)) error {
	tail,start,err:=readPartialOverlap(part,offset)
	if err!=nil{return err}
	if _,err=part.Seek(offset,io.SeekStart);err!=nil{return err}
	counter:=&fileTransferProgressWriter{writer:part,base:offset,callback:progress}
	check:=&verifiedAppendWriter{dst:counter,tail:tail}
	err=s.withFTPClient(ctx,profile,nil,func(client *ftpclient.Client) error{
		if start>0 {
			return client.RetrieveFrom(ctx,remotePath,start,check)
		}
		return client.Retrieve(ctx,remotePath,check)
	})
	if err!=nil {return err}
	if !check.validated(){return fmt.Errorf("%w (source shorter than saved overlap)",errLiveSourceChanged)}
	return nil
}

// OpenSSH's reget resumes at the LOCAL file length. Rewind a small tail,
// then verify that the remote re-supplied bytes are IDENTICAL before accepting
// the append. On mismatch restore the original partial bytes, not a hybrid.
func (s *Server) sftpAppendVerified(ctx context.Context,profile filetransferprofile.Profile,remotePath,partial string, offset int64,progress func(int64)) error {
	part,err:=os.OpenFile(partial,os.O_RDWR,0600)
	if err!=nil{return err}
	tail,start,err:=readPartialOverlap(part,offset)
	if err!=nil{_ =part.Close();return err}
	if offset>0 {
		if err=part.Truncate(start);err!=nil{_ =part.Close();return err}
	}
	if err=part.Close();err!=nil{return err}
	var command string
	if offset>0 {command,err=sftpclient.RegetCommand(remotePath,partial)
	}else{command,err=sftpclient.GetCommand(remotePath,partial)}
	if err!=nil{return err}
	done:=make(chan struct{})
	if progress!=nil{go pollSFTPHostPartial(ctx,partial,offset,progress,done)}
	_,transferErr:=s.runSFTP(longFileTransferContext(ctx),profile,command+"quit\n",64<<10)
	close(done)
	part,err=os.OpenFile(partial,os.O_RDWR,0600)
	if err!=nil{return err}
	defer part.Close()
	info,err:=part.Stat()
	if err!=nil{return err}
	// Roll back if remote is shorter or its old tail differs. A failure to
	// read a full tail after a canceled transfer may simply be a transient
	// interruption: restore the old valid prefix in that case as well.
	valid:=info.Size()>=offset
	if valid&&offset>0{
		observed:=make([]byte,len(tail))
		if _,err=part.ReadAt(observed,start);err!=nil {valid=false
		}else{valid=bytes.Equal(observed,tail)}
	}
	if !valid {
		if err=part.Truncate(start);err!=nil{return err}
		if len(tail)>0 {
			if _,err=part.WriteAt(tail,start);err!=nil{return err}
		}
		if err=part.Truncate(offset);err!=nil{return err}
		if err=part.Sync();err!=nil{return err}
		if transferErr==nil{return errLiveSourceChanged}
		return fmt.Errorf("%w; SFTP interruption: %v",errLiveSourceChanged,transferErr)
	}
	if progress!=nil{progress(info.Size())}
	return transferErr
}

func (s *Server) backgroundRemoteToHostGrowing(ctx context.Context, profileID, remotePath, hostRel string, overwrite bool, expectedSize int64, progress func(int64)) error {
	target,err:=s.backgroundHostTarget(hostRel,overwrite)
	if err!=nil{return err}
	partial:=fileTransferPartialHostPath(target,remotePath)
	if st,statErr:=os.Lstat(partial);statErr==nil {
		if !st.Mode().IsRegular(){return errors.New("download partial must be a regular file")}
		if st.Size()>maxFileTransferBytes{return errors.New("download partial exceeds size limit")}
	}else if !os.IsNotExist(statErr){return statErr}
	part,err:=os.OpenFile(partial,os.O_CREATE|os.O_RDWR,0600)
	if err!=nil{return err}
	defer part.Close()
	profile,err:=s.resolveFileTransferProfile(profileID)
	if err!=nil{return err}
	prev,err:=part.Stat()
	if err!=nil{return err}
	startSize:=prev.Size()
	restarts:=0
	restartNewSource:=func(cause error) error{
		if restarts>=1{return cause}
		restarts++
		if truncErr:=part.Truncate(0);truncErr!=nil{return truncErr}
		if _,seekErr:=part.Seek(0,io.SeekStart);seekErr!=nil{return seekErr}
		if progress!=nil{progress(-1)} // Explicitly reset queue bytes and note safe restart.
		return nil
	}
	// Source size captured during scan is a hint, NEVER a strict equality
	// check for an append-only log. A fresh remote stat is authoritative.
	_ =expectedSize
	var lastMeta *fileTransferEntry
	const catchupDelay=180*time.Millisecond
	for round:=0;round<liveFileCatchupRounds;round++ {
		if err=ctx.Err();err!=nil{return err}
		meta,lookupErr:=s.backgroundRemoteEntry(ctx,profileID,remotePath)
		if lookupErr!=nil{return fmt.Errorf("stat remote log: %w",lookupErr)}
		if meta==nil||meta.Type!="file"{return fmt.Errorf("%w: remote path unavailable",errLiveSourceChanged)}
		if meta.Size>maxFileTransferBytes{return fmt.Errorf("file transfer exceeds %d bytes",maxFileTransferBytes)}
		stat,statErr:=part.Stat()
		if statErr!=nil{return statErr}
		offset:=stat.Size()
		if meta.Size<offset {
			cause:=fmt.Errorf("%w: remote size %d smaller than saved %d",errLiveSourceChanged,meta.Size,offset)
			if resetErr:=restartNewSource(cause);resetErr!=nil{return resetErr}
			round=-1;continue
		}
		if progress!=nil{progress(offset)}
		// Even when both lengths are equal, re-check the old tail before
		// finalizing a resumed file (especially if replaced with equal size).
		switch profile.Protocol {
		case filetransferprofile.ProtocolFTP:
			err=s.ftpAppendVerified(ctx,profile,remotePath,part,offset,progress)
		case filetransferprofile.ProtocolSFTP:
			err=s.sftpAppendVerified(ctx,profile,remotePath,partial,offset,progress)
		default:return fmt.Errorf("unsupported transfer protocol %q",profile.Protocol)
		}
		if err!=nil{
			if errors.Is(err,errLiveSourceChanged){
				if resetErr:=restartNewSource(err);resetErr!=nil{return resetErr}
				round=-1;continue
			}
			return fmt.Errorf("download %s: %w",remotePath,err)
		}
		if err=part.Sync();err!=nil{return err}
		after,statErr:=part.Stat()
		if statErr!=nil{return statErr}
		if after.Size()>maxFileTransferBytes{return fmt.Errorf("file transfer exceeds %d bytes",maxFileTransferBytes)}
		next,lookupErr:=s.backgroundRemoteEntry(ctx,profileID,remotePath)
		if lookupErr!=nil{return fmt.Errorf("stat remote log after download: %w",lookupErr)}
		if next==nil||next.Type!="file"||next.Size<after.Size(){
			cause:=fmt.Errorf("%w: remote size shrank/rotated after transfer",errLiveSourceChanged)
			if resetErr:=restartNewSource(cause);resetErr!=nil{return resetErr}
			round=-1;continue
		}
		// When length is stable but a known modification timestamp changed
		// WITHOUT append growth, treat the file as rewritten, not appended.
		if next.Size==meta.Size&&meta.Size==after.Size()&&meta.Modified!=""&&next.Modified!=""&&meta.Modified!=next.Modified {
			cause:=fmt.Errorf("%w: modified at same length",errLiveSourceChanged)
			if resetErr:=restartNewSource(cause);resetErr!=nil{return resetErr}
			round=-1;continue
		}
		lastMeta=next
		if next.Size==after.Size(){
			// Re-read a bounded suffix after an initial transfer if the
			// source grew while the first FTP/SFTP stream was active.
			if round==0&&startSize==0&&after.Size()>0&&
				(next.Size!=meta.Size||(meta.Modified!=""&&next.Modified!=""&&meta.Modified!=next.Modified)){
				continue
			}
			break
		}
		// If append continues for the entire bounded catch-up window, save
		// the verified complete prefix as a snapshot, never an invented EOF.
		if round==liveFileCatchupRounds-1{break}
		select{case <-ctx.Done():return ctx.Err();case <-time.After(catchupDelay):}
	}
	finalInfo,err:=part.Stat()
	if err!=nil{return err}
	if lastMeta!=nil&&finalInfo.Size()>lastMeta.Size {
		return fmt.Errorf("%w: remote log truncated before completion",errLiveSourceChanged)
	}
	if progress!=nil{
		if lastMeta!=nil&&finalInfo.Size()<lastMeta.Size {progress(-2)}
		progress(finalInfo.Size())
	}
	if err=part.Sync();err!=nil{return err}
	// FTP writer must not keep the file open when renaming on Windows.
	if err=part.Close();err!=nil{return err}
	if err=os.Chmod(partial,0644);err!=nil{return err}
	if _,err=s.backgroundHostTarget(hostRel,overwrite);err!=nil{return err}
	if overwrite {
		if existing,statErr:=os.Lstat(target);statErr==nil{
			if !existing.Mode().IsRegular(){return errors.New("download destination was replaced by unsafe file type")}
			if err=os.Remove(target);err!=nil{return err}
		}else if !os.IsNotExist(statErr){return statErr}
	}
	return os.Rename(partial,target)
}
