package remotetransport

import (
 "errors"
 "sync"
)

// RemotePTYOutputWindow collects ONLY authenticated host->client PTY
// output after the per-session ownership and exact sequence gate. It has
// no transport endpoint, shell authority, PHP API, or background polling.
// A stale reader never silently receives an incomplete transcript.
const RemotePTYOutputRetainedBytes=64*1024
const RemotePTYOutputWindowBytes=2048

var ErrRemotePTYOutputGap=errors.New("remote PTY output cursor precedes the retained window")

type RemotePTYOutputWindow struct{
 mu sync.Mutex
 gate *BoundedRemotePTYSessionGate
 buf []byte
 start int64
 next int64
 closed bool
}

func NewRemotePTYOutputWindow(handle string)(*RemotePTYOutputWindow,error){
 gate,err:=NewBoundedRemotePTYSessionGate(handle)
 if err!=nil{return nil,err}
 return &RemotePTYOutputWindow{gate:gate},nil
}

// AppendFromHost is the only writer path. The sequence check and append
// are serialized by the same local mutex, so racing host frames cannot
// reorder bytes relative to their authenticated logical Sequence.
func (w *RemotePTYOutputWindow)AppendFromHost(frame RemotePTYFrame)error{
 if w==nil{return ErrInvalidRemoteAction}
 w.mu.Lock()
 defer w.mu.Unlock()
 if w.closed{return ErrInvalidRemoteAction}
 if frame.Kind!="output"||w.gate.AcceptFromHost(frame)!=nil{
  w.closeLocked()
  return ErrInvalidRemoteAction
 }
 length:=int64(len(frame.Data))
 if length<1||w.next>int64(RemotePTYMaxFramesPerDirection*RemotePTYMaxOutput)-length{
  w.closeLocked()
  return ErrInvalidRemoteAction
 }
 w.next+=length
 w.buf=append(w.buf,frame.Data...)
 if len(w.buf)>RemotePTYOutputRetainedBytes{
  evicted:=len(w.buf)-RemotePTYOutputRetainedBytes
  // Zero discarded plaintext before releasing its view.
  for i:=0;i<evicted;i++{w.buf[i]=0}
  copy(w.buf,w.buf[evicted:])
  w.buf=w.buf[:RemotePTYOutputRetainedBytes]
  w.start+=int64(evicted)
 }
 return nil
}

// ReadAfter is a read-only, bounded copy. A cursor must name an exact
// absolute byte offset in the retained interval. Missing data is an
// explicit error, NEVER an apparently complete empty transcript.
func (w *RemotePTYOutputWindow)ReadAfter(cursor int64)([]byte,int64,error){
 if w==nil{return nil,0,ErrInvalidRemoteAction}
 w.mu.Lock()
 defer w.mu.Unlock()
 if w.closed||cursor<0||cursor>w.next{
  return nil,0,ErrInvalidRemoteAction
 }
 if cursor<w.start{return nil,w.start,ErrRemotePTYOutputGap}
 offset:=int(cursor-w.start)
 available:=len(w.buf)-offset
 if available<0{return nil,0,ErrInvalidRemoteAction}
 if available>RemotePTYOutputWindowBytes{available=RemotePTYOutputWindowBytes}
 data:=append([]byte(nil),w.buf[offset:offset+available]...)
 return data,cursor+int64(available),nil
}

// RetainedRange lets the local trusted GUI recover deliberately after
// an explicit "output omitted" indication. It is not an Agent request.
func (w *RemotePTYOutputWindow)RetainedRange()(int64,int64,error){
 if w==nil{return 0,0,ErrInvalidRemoteAction}
 w.mu.Lock()
 defer w.mu.Unlock()
 if w.closed{return 0,0,ErrInvalidRemoteAction}
 return w.start,w.next,nil
}
func (w *RemotePTYOutputWindow)Close(){
 if w==nil{return}
 w.mu.Lock()
 defer w.mu.Unlock()
 w.closeLocked()
}
func (w *RemotePTYOutputWindow)closeLocked(){
 if w.closed{return}
 w.closed=true
 if w.gate!=nil{w.gate.Close()}
 for i:=range w.buf{w.buf[i]=0}
 w.buf=nil
 w.start=0
 w.next=0
}
