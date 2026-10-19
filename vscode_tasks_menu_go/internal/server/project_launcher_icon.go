package server

import (
 "bytes"
 _ "embed"
 "encoding/binary"
 "fmt"
 "image"
 "image/color"
 "image/png"
 "sync"
)

// This is the exact Glossy Terminal Task Stack.png image from the repository
// root. It is embedded into the TaskDeck binary so launcher installation
// works offline, including after self-update and in source-only builds.
//
//go:embed taskdeck_icon.png
var taskdeckLauncherPNG []byte

var taskdeckMacIconCache struct {
 once sync.Once
 data []byte
 err error
}

// Resize using bilinear sampling. The bundled 1254x1254 source does not match
// any macOS ICNS pixel-size slot, so each ICNS PNG must be resized before use.
func launcherScaleIcon(source image.Image, width int) *image.NRGBA {
 bounds := source.Bounds()
 sw,sh := bounds.Dx(), bounds.Dy()
 target := image.NewNRGBA(image.Rect(0,0,width,width))
 for y:=0;y<width;y++{
  sy:=(float64(y)+0.5)*float64(sh)/float64(width)-0.5
  y0:=int(sy)
  if y0<0{y0=0}
  if y0>=sh{y0=sh-1}
  y1:=y0+1;if y1>=sh{y1=sh-1}
  fy:=sy-float64(y0);if fy<0{fy=0}
  for x:=0;x<width;x++{
   sx:=(float64(x)+0.5)*float64(sw)/float64(width)-0.5
   x0:=int(sx)
   if x0<0{x0=0}
   if x0>=sw{x0=sw-1}
   x1:=x0+1;if x1>=sw{x1=sw-1}
   fx:=sx-float64(x0);if fx<0{fx=0}
   p00:=color.NRGBAModel.Convert(source.At(bounds.Min.X+x0,bounds.Min.Y+y0)).(color.NRGBA)
   p10:=color.NRGBAModel.Convert(source.At(bounds.Min.X+x1,bounds.Min.Y+y0)).(color.NRGBA)
   p01:=color.NRGBAModel.Convert(source.At(bounds.Min.X+x0,bounds.Min.Y+y1)).(color.NRGBA)
   p11:=color.NRGBAModel.Convert(source.At(bounds.Min.X+x1,bounds.Min.Y+y1)).(color.NRGBA)
   mix:=func(a,b,c,d uint8)uint8{
    top:=float64(a)*(1-fx)+float64(b)*fx
    bot:=float64(c)*(1-fx)+float64(d)*fx
    return uint8(top*(1-fy)+bot*fy+0.5)
   }
   target.SetNRGBA(x,y,color.NRGBA{
    R:mix(p00.R,p10.R,p01.R,p11.R),
    G:mix(p00.G,p10.G,p01.G,p11.G),
    B:mix(p00.B,p10.B,p01.B,p11.B),
    A:mix(p00.A,p10.A,p01.A,p11.A),
   })
  }
 }
 return target
}

func launcherBuildICNS(source []byte)([]byte,error){
 picture,err:=png.Decode(bytes.NewReader(source))
 if err!=nil{return nil,fmt.Errorf("decode bundled TaskDeck icon: %w",err)}
 bounds:=picture.Bounds()
 if bounds.Dx()<1024||bounds.Dy()<1024{return nil,fmt.Errorf("TaskDeck icon must be at least 1024x1024 pixels")}
 // ic09=512x512, ic10=1024x1024. Both hold a PNG-encoded image.
 result:=[]byte{'i','c','n','s',0,0,0,0}
 for _,entry:=range []struct{name string;size int}{{"ic09",512},{"ic10",1024}}{
  var pngData bytes.Buffer
  if err:=png.Encode(&pngData,launcherScaleIcon(picture,entry.size));err!=nil{return nil,err}
  length:=uint32(8+pngData.Len())
  result=append(result,entry.name...)
  var sizeBytes [4]byte
  binary.BigEndian.PutUint32(sizeBytes[:],length)
  result=append(result,sizeBytes[:]...)
  result=append(result,pngData.Bytes()...)
 }
 if uint64(len(result))>uint64(^uint32(0)){return nil,fmt.Errorf("TaskDeck icon exceeds ICNS size limit")}
 binary.BigEndian.PutUint32(result[4:8],uint32(len(result)))
 return result,nil
}

func launcherMacIcon()([]byte,error){
 taskdeckMacIconCache.once.Do(func(){
  taskdeckMacIconCache.data,taskdeckMacIconCache.err=launcherBuildICNS(taskdeckLauncherPNG)
 })
 return taskdeckMacIconCache.data,taskdeckMacIconCache.err
}
