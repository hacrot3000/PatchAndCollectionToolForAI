// Package contextcompact also owns file-attachment compaction. It is pure:
// callers supply a file reader, name and verified provenance; it never
// executes code, opens paths, or mutates the canonical Agent journal.
package contextcompact

import (
 "bufio"
 "bytes"
 "encoding/binary"
 "fmt"
 "image"
 _ "image/gif"
 _ "image/jpeg"
 _ "image/png"
 "io"
 "mime"
 "net/http"
 "path/filepath"
 "regexp"
 "strconv"
 "strings"
 "unicode/utf8"
)

const (
 AttachmentSummaryLimit=22<<10
 AttachmentScanLimit=int64(64<<20)
 AttachmentSampleLimit=int64(256<<10)
 attachmentMaxSymbols=4096
)

type AttachmentSummary struct {
 Name string `json:"name"`
 Text string `json:"text"`
 MIME string `json:"mime"`
 Kind string `json:"kind"`
 Language string `json:"language,omitempty"`
 OriginalSize int64 `json:"original_size"`
 ScannedBytes int64 `json:"scanned_bytes,omitempty"`
 Partial bool `json:"partial,omitempty"`
}

var codeLanguages=map[string]string{
 ".nim":"Nim",".nims":"Nim",".py":"Python",".pyi":"Python",
 ".go":"Go",".js":"JavaScript",".jsx":"JavaScript",".mjs":"JavaScript",
 ".cjs":"JavaScript",".ts":"TypeScript",".tsx":"TypeScript",
 ".java":"Java",".kt":"Kotlin",".kts":"Kotlin",".swift":"Swift",
 ".rs":"Rust",".c":"C",".h":"C",".cc":"C++",".cpp":"C++",".cxx":"C++",
 ".hpp":"C++",".hh":"C++",".cs":"C#",".php":"PHP",".rb":"Ruby",
 ".lua":"Lua",".sh":"Shell",".bash":"Shell",".zsh":"Shell",
 ".sql":"SQL",".dart":"Dart",".scala":"Scala",".sc":"Scala",
 ".vue":"Vue",".svelte":"Svelte",".css":"CSS",".scss":"CSS",
 ".html":"HTML",".htm":"HTML",
}

func SourceLanguage(name string)string{
 return codeLanguages[strings.ToLower(filepath.Ext(name))]
}
func cleanAttachmentLabel(s string)string{
 s=strings.Map(func(r rune)rune{
  if r=='\n'||r=='\r'||r==0||r=='\t'{return ' '}
  return r
 },s)
 s=strings.TrimSpace(s)
 if len(s)>512{s=s[:512]}
 return s
}

var patterns=map[string]*regexp.Regexp{
 "Nim":regexp.MustCompile(`^(?:proc|func|method|iterator|converter|template|macro)\s+|^ee\.(?:on|once)\s*\(|^(?:type|var|let|const|when|static|import|from|include|export)\b`),
 "Python":regexp.MustCompile(`^(?:async\s+def|def|class|from|import)\b`),
 "Go":regexp.MustCompile(`^(?:func|type|var|const|import|package)\b`),
 "JavaScript":regexp.MustCompile(`^(?:(?:export|declare|default|async|static)\s+)*(?:function|class|interface|type|enum|const|let|var|import|export)\b|^(?:get|set)\s+\w+\s*\(`),
 "TypeScript":regexp.MustCompile(`^(?:(?:export|declare|default|async|static|abstract)\s+)*(?:function|class|interface|type|enum|const|let|var|import|export|namespace)\b|^(?:get|set)\s+\w+\s*\(`),
 "Rust":regexp.MustCompile(`^(?:(?:pub(?:\([^)]*\))?|async|unsafe|extern)\s+)*(?:fn|struct|enum|trait|impl|mod|use|type|const|static|macro_rules!)\b`),
 "Swift":regexp.MustCompile(`^(?:(?:public|private|internal|open|static|override|final|mutating)\s+)*(?:func|class|struct|enum|protocol|extension|import|typealias|let|var|init|subscript)\b`),
 "Kotlin":regexp.MustCompile(`^(?:(?:public|private|protected|internal|static|final|abstract|data|sealed|open|override|suspend|inline)\s+)*(?:fun|class|interface|enum|object|package|import|typealias|val|var)\b`),
 "Java":regexp.MustCompile(`^(?:(?:public|private|protected|static|final|abstract|native|synchronized|default)\s+)*(?:class|interface|enum|record|package|import|void|[\w<>\[\],.?]+)\s+[\w$]+\s*\(|^(?:import|package|class|interface|enum|record)\b`),
 "C#":regexp.MustCompile(`^(?:(?:public|private|protected|internal|static|partial|sealed|abstract|override|async)\s+)*(?:class|interface|struct|enum|record|namespace|using|delegate|event)\b`),
 "C":regexp.MustCompile(`^(?:#\s*(?:include|define)|(?:typedef|struct|enum|union)\b|(?:[\w*]+\s+){1,5}[\w*]+\s*\()`),
 "C++":regexp.MustCompile(`^(?:#\s*(?:include|define)|template\s*<|(?:class|struct|enum|namespace|using|typedef|concept)\b|(?:[\w:*<>~&]+\s+){1,5}[\w:~*]+\s*\()`),
 "PHP":regexp.MustCompile(`^(?:(?:abstract|final|public|private|protected|static)\s+)*(?:namespace|use|class|trait|interface|function|const|enum)\b`),
 "Ruby":regexp.MustCompile(`^(?:class|module|def|require|include|extend|attr_reader|attr_writer|attr_accessor)\b`),
 "Lua":regexp.MustCompile(`^(?:local\s+)?function\b|^(?:local\s+)?\w+(?:\.\w+)*\s*=\s*function\b|^require\s*\(`),
 "Shell":regexp.MustCompile(`^(?:function\s+\w+|\w+\s*\(\s*\)\s*\{|source\s+|export\s+|\. )`),
 "SQL":regexp.MustCompile(`^(?:(?:CREATE|ALTER)\s+(?:OR\s+REPLACE\s+)?(?:TABLE|VIEW|FUNCTION|PROCEDURE|TRIGGER|INDEX|TYPE)\b)`),
 "Dart":regexp.MustCompile(`^(?:import|export|part|class|mixin|extension|enum|typedef|(?:Future<[^>]+>|void|[\w<>?,]+)\s+\w+\s*\()`),
 "Scala":regexp.MustCompile(`^(?:import|package|class|trait|object|enum|case\s+class|def|type|val|var)\b`),
 "Vue":regexp.MustCompile(`^<(?:template|script|style)\b|^(?:import|export|function|class|const|interface|type)\b`),
 "Svelte":regexp.MustCompile(`^<(?:script|style)\b|^(?:import|export|function|class|const|interface|type)\b`),
 "CSS":regexp.MustCompile(`^(?:@(?:import|media|keyframes|layer|font-face)\b|[.#:\w\-][^{]{0,160}\{)`),
 "HTML":regexp.MustCompile(`^<(?:!DOCTYPE|html|head|body|script|style|main|section|article|template|form|header|footer)\b`),
}

type attachmentSymbol struct{
 line int
 indent int
 kind string
 signature string
 end int
 multiline bool
}
func leadingIndent(s string)int{
 n:=0
 for _,r:=range s{
  if r==' '{n++}else if r=='\t'{n+=4}else{break}
 }
 return n
}
func isSignatureContinuation(lang,signature string)bool{
 if lang=="Nim"{
  if strings.HasPrefix(signature,"ee.on")||strings.HasPrefix(signature,"ee.once"){return !strings.Contains(signature,"):")}
  if strings.HasPrefix(signature,"proc ")||strings.HasPrefix(signature,"func ")||
   strings.HasPrefix(signature,"method ")||strings.HasPrefix(signature,"template ")||
   strings.HasPrefix(signature,"macro ")||strings.HasPrefix(signature,"iterator ")||
   strings.HasPrefix(signature,"converter "){
    return !strings.Contains(signature,"=")
  }
  return false
 }
 if lang=="Go" && (signature=="import ("||signature=="type ("){return false}
 return false
}
func outlineSignature(s string)string{
 s=strings.Join(strings.Fields(s)," ")
 if len(s)>320{s=s[:317]+"..."}
 return s
}
func nimBlockChild(line string)string{
 trimmed:=strings.TrimSpace(line)
 if trimmed==""||strings.HasPrefix(trimmed,"#")||strings.HasPrefix(trimmed,"{."){return ""}
 if p:=strings.IndexAny(trimmed,":= ");p>=0{trimmed=trimmed[:p]}
 if trimmed==""||len(trimmed)>100{return ""}
 for _,r:=range trimmed{if !((r>='a'&&r<='z')||(r>='A'&&r<='Z')||(r>='0'&&r<='9')||r=='_'||r=='*'){return ""}}
 return trimmed
}
func extractOutline(lang string,r io.Reader) (string,int64,bool,error){
 matcher:=patterns[lang]
 if matcher==nil{return "",0,false,nil}
 reader:=bufio.NewScanner(io.LimitReader(r,AttachmentScanLimit+1))
 reader.Buffer(make([]byte,16<<10),4<<20)
 symbols:=make([]attachmentSymbol,0,128)
 lineNumber:=0
 var bytesRead int64
 partial:=false
 overflow:=0
 // Declaration boundaries are based on the next peer declaration (not
 // guessed braces); nesting is tracked by indentation for Python/Nim.
 for reader.Scan(){
  raw:=reader.Text();lineNumber++;bytesRead+=int64(len(raw))+1
  if bytesRead>AttachmentScanLimit{partial=true;break}
  if !utf8.ValidString(raw)||strings.ContainsRune(raw,0){
    return "",bytesRead,true,fmt.Errorf("source is not UTF-8 text")
  }
  trim:=strings.TrimSpace(raw)
  if trim==""||strings.HasPrefix(trim,"# ")||strings.HasPrefix(trim,"//")||strings.HasPrefix(trim,"##"){continue}
  indent:=leadingIndent(raw)
  // Nim top-level declaration blocks list member names without initializers
  // to avoid accidentally sharing secrets contained in const assignments.
  if lang=="Nim"&&indent>0&&len(symbols)>0 {
   top:=&symbols[len(symbols)-1]
   if (top.signature=="type"||top.signature=="const"||top.signature=="var"||top.signature=="let"||
       strings.HasPrefix(top.signature,"type ")||strings.HasPrefix(top.signature,"const ")||
       strings.HasPrefix(top.signature,"var ")||strings.HasPrefix(top.signature,"let "))&&top.indent==0 {
    if child:=nimBlockChild(raw);child!=""&&len(top.signature)<250{
     top.signature+=" "+child
    }
   }
  }
  // Continuation of a multiline Nim proc/template signature. Do not include
  // the body; stop once the declaration's '=' is reached.
  if lang=="Nim"&&len(symbols)>0 {
   last:=&symbols[len(symbols)-1]
   if last.multiline&&(indent>last.indent||strings.HasPrefix(trim,")"))&&lineNumber-last.line<=14{
    tail:=outlineSignature(trim)
    if len(last.signature)+len(tail)<420 {
     last.signature+=" "+tail
    }
    last.multiline=isSignatureContinuation(lang,last.signature)
    continue
   }
   if last.multiline&&(lineNumber-last.line>14||indent<=last.indent){last.multiline=false}
  }
  if lang=="Nim"&&indent!=0{continue}
  // Capturing class members for non-Nim languages is useful, but avoid
  // arbitrary statements: only explicit declaration patterns enter here.
  if !matcher.MatchString(trim){continue}
  if strings.HasPrefix(trim,"@")&&lang=="Python"{continue}
  for i:=len(symbols)-1;i>=0;i--{
   if symbols[i].end==0 && indent<=symbols[i].indent && lineNumber>symbols[i].line{
    symbols[i].end=lineNumber-1
   }
  }
  if len(symbols)>=attachmentMaxSymbols{partial=true;overflow++;continue}
  signature:=outlineSignature(trim)
  // Index declaration names and types, not source initializer values. For
  // instance a top-level const SECRET = "value" is NOT model context.
  head:=strings.TrimSpace(signature)
  for _,prefix:=range []string{"export ","default ","declare ","public ","private ","static "}{
   if strings.HasPrefix(head,prefix){head=strings.TrimPrefix(head,prefix)}
  }
  if strings.HasPrefix(head,"const ")||strings.HasPrefix(head,"let ")||
      strings.HasPrefix(head,"var ")||strings.HasPrefix(head,"val ") {
   if equals:=strings.Index(signature," = ");equals>=0{
    signature=signature[:equals]+" = [initializer omitted]"
   }
  }
  kind:=strings.Fields(signature)[0]
  if strings.HasPrefix(signature,"ee.on(")||strings.HasPrefix(signature,"ee.once("){kind="event"}
  symbols=append(symbols,attachmentSymbol{
   line:lineNumber,indent:indent,kind:kind,signature:signature,
   multiline:isSignatureContinuation(lang,signature),
  })
 }
 if err:=reader.Err();err!=nil{
  // An extremely long line has no safe declaration structure. Keep the
  // verified prefix and mark the result incomplete instead of sending raw.
  partial=true
 }
 for i:=range symbols{if symbols[i].end==0{symbols[i].end=lineNumber}}
 var b strings.Builder
 fmt.Fprintf(&b,"LANGUAGE: %s\nSOURCE_LINES_SCANNED: %d\nDECLARATIONS_INDEXED: %d\n",lang,lineNumber,len(symbols))
 b.WriteString("STRUCTURAL OUTLINE (signatures/imports/events only; implementation bodies omitted):\n")
 omitted:=overflow
 for i,s:=range symbols{
  line:=fmt.Sprintf("L%d-L%d [%s] %s\n",s.line,s.end,s.kind,s.signature)
  if b.Len()+len(line)>AttachmentSummaryLimit-400{
   omitted+=len(symbols)-i;partial=true;break
  }
  b.WriteString(line)
 }
 if omitted>0{fmt.Fprintf(&b,"OTHER_DECLARATIONS_OMITTED: %d\n",omitted)}
 return b.String(),bytesRead,partial,nil
}
func attachmentMIME(name,provided string,sample []byte)string{
 m:=strings.TrimSpace(provided)
 if m==""{m=mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))}
 if m==""&&len(sample)>0{m=http.DetectContentType(sample)}
 if m==""{m="application/octet-stream"}
 if p:=strings.Index(m,";");p>=0{m=m[:p]}
 return strings.ToLower(strings.TrimSpace(m))
}
func imageExifTags(data []byte)map[string]string{
 // Bounded, read-only JPEG APP1 / PNG eXIf / WebP EXIF parsing.
 var tiff []byte
 if len(data)>4&&data[0]==0xff&&data[1]==0xd8{
  for pos:=2;pos+4<=len(data);{
   if data[pos]!=0xff{break}
   marker:=data[pos+1]
   if marker==0xda||marker==0xd9{break}
   if marker==0x01||marker>=0xd0&&marker<=0xd7{pos+=2;continue}
   length:=int(binary.BigEndian.Uint16(data[pos+2:]))
   if length<2||pos+2+length>len(data){break}
   chunk:=data[pos+4:pos+2+length]
   if marker==0xe1&&bytes.HasPrefix(chunk,[]byte("Exif\x00\x00")){
    tiff=chunk[6:];break
   }
   pos+=2+length
  }
 }else if len(data)>8&&bytes.Equal(data[:8],[]byte("\x89PNG\r\n\x1a\n")){
  for pos:=8;pos+12<=len(data);{
   n:=int(binary.BigEndian.Uint32(data[pos:]))
   if n<0||pos+12+n>len(data){break}
   if string(data[pos+4:pos+8])=="eXIf"{tiff=data[pos+8:pos+8+n];break}
   pos+=12+n
  }
 }else if len(data)>12&&string(data[:4])=="RIFF"&&string(data[8:12])=="WEBP"{
  for pos:=12;pos+8<=len(data);{
   n:=int(binary.LittleEndian.Uint32(data[pos+4:]))
   if n<0||pos+8+n>len(data){break}
   if string(data[pos:pos+4])=="EXIF"{tiff=data[pos+8:pos+8+n];break}
   pos+=8+n+(n&1)
  }
 }
 tags:=map[string]string{}
 if len(tiff)<8{return tags}
 var order binary.ByteOrder
 if string(tiff[:2])=="II"{order=binary.LittleEndian}else if string(tiff[:2])=="MM"{order=binary.BigEndian}else{return tags}
 if order.Uint16(tiff[2:])!=42{return tags}
 ifd:=int(order.Uint32(tiff[4:]))
 visited:=map[int]bool{}
 var walk func(int,int)
 walk=func(offset,depth int){
  if depth>2||offset<8||offset+2>len(tiff)||visited[offset]{return}
  visited[offset]=true
  count:=int(order.Uint16(tiff[offset:]))
  if count>200{count=200}
  names:=map[uint16]string{
   0x010f:"CameraMake",0x0110:"CameraModel",0x0131:"Software",
   0x0132:"DateTime",0x0112:"Orientation",0x9003:"DateTimeOriginal",
   0xa434:"LensModel",0xa002:"PixelXDimension",0xa003:"PixelYDimension",
  }
  for i:=0;i<count;i++{
   pos:=offset+2+i*12
   if pos+12>len(tiff){break}
   tag:=order.Uint16(tiff[pos:])
   typ:=order.Uint16(tiff[pos+2:])
   quantity:=order.Uint32(tiff[pos+4:])
   if (tag==0x8769||tag==0x8825)&&typ==4&&quantity==1{
    if tag==0x8825{tags["GPS"]="present (coordinates omitted)"}
    if tag==0x8769{walk(int(order.Uint32(tiff[pos+8:])),depth+1)}
    continue
   }
   name:=names[tag];if name==""{continue}
   var value string
   if typ==2&&quantity>0&&quantity<=256{
    var raw []byte
    if quantity<=4{raw=tiff[pos+8:pos+8+int(quantity)]}else{
     addr:=int(order.Uint32(tiff[pos+8:]))
     if addr>=0&&addr+int(quantity)<=len(tiff){raw=tiff[addr:addr+int(quantity)]}
    }
    value=strings.TrimSpace(strings.TrimRight(string(raw),"\x00"))
   }else if typ==3&&quantity==1{
    value=strconv.Itoa(int(order.Uint16(tiff[pos+8:])))
   }else if typ==4&&quantity==1{
    value=strconv.FormatUint(uint64(order.Uint32(tiff[pos+8:])),10)
   }
   if value!=""&&utf8.ValidString(value){tags[name]=cleanAttachmentLabel(value)}
  }
 }
 walk(ifd,0)
 return tags
}

// CompactAttachment extracts only code declarations or non-source file
// metadata. It never forwards raw binary or a full non-code document.
// For a huge source file only the bounded prefix is indexed; PARTIAL explicitly
// prevents a model from treating that prefix as complete source coverage.
func CompactAttachment(name,path,providedMIME string,originalSize int64,reader io.Reader)(AttachmentSummary,error){
 name=cleanAttachmentLabel(filepath.Base(name))
 path=cleanAttachmentLabel(path)
 if name==""{return AttachmentSummary{},fmt.Errorf("attachment needs a file name")}
 if originalSize<0{return AttachmentSummary{},fmt.Errorf("invalid attachment size")}
 lang:=SourceLanguage(name)
 result:=AttachmentSummary{Name:name,MIME:"text/plain",OriginalSize:originalSize,Kind:"metadata",Language:lang}
 sourceMIME:=attachmentMIME(name,providedMIME,nil)
 var b strings.Builder
 fmt.Fprintf(&b,"FILE_NAME: %s\nFILE_PATH: %s\nFILE_SIZE_BYTES: %d\nFILE_FORMAT: %s\n",
  name,func()string{if path==""{return "not available (browser-selected file)"};return path}(),
  originalSize,sourceMIME)
 if lang!=""{
  result.Kind="source_outline"
  outline,read,partial,err:=extractOutline(lang,reader)
  result.ScannedBytes=read
  if err!=nil{
   result.Partial=true
   b.WriteString("SOURCE_OUTLINE_UNAVAILABLE: invalid or non-text source\n")
  }else{
   result.Partial=partial||originalSize>read+2
   if b.Len()+len(outline)>AttachmentSummaryLimit-160{
    outline=outline[:AttachmentSummaryLimit-b.Len()-160]
    result.Partial=true
   }
   b.WriteString(outline)
  }
  if result.Partial{b.WriteString("PARTIAL: true (use native COLLECT for missing declarations or implementation bodies)\n")}
 }else{
  sample,_:=io.ReadAll(io.LimitReader(reader,AttachmentSampleLimit))
  actualMIME:=attachmentMIME(name,providedMIME,sample)
  if actualMIME!=sourceMIME{
   sourceMIME=actualMIME
   // File format is format metadata only, never a raw file excerpt.
   fmt.Fprintf(&b,"SNIFFED_FORMAT: %s\n",sourceMIME)
  }
  if strings.HasPrefix(sourceMIME,"image/")||bytes.HasPrefix(sample,[]byte{0xff,0xd8}){
   result.Kind="image_metadata"
   if c,_,err:=image.DecodeConfig(bytes.NewReader(sample));err==nil{
    fmt.Fprintf(&b,"IMAGE_DIMENSIONS: %dx%d\n",c.Width,c.Height)
   }
   tags:=imageExifTags(sample)
   for _,k:=range []string{"CameraMake","CameraModel","LensModel","Software","DateTime","DateTimeOriginal","Orientation","PixelXDimension","PixelYDimension","GPS"}{
    if v:=tags[k];v!=""{fmt.Fprintf(&b,"EXIF_%s: %s\n",k,v)}
   }
   if len(tags)==0{b.WriteString("EXIF: not found in inspected header\n")}
  } else if len(sample)>4{
   if bytes.HasPrefix(sample,[]byte("%PDF-")){b.WriteString("FORMAT_SIGNATURE: PDF\n")}
   if bytes.HasPrefix(sample,[]byte("PK\x03\x04")){b.WriteString("FORMAT_SIGNATURE: ZIP container\n")}
  }
 }
 result.Text=b.String()
 if len(result.Text)>AttachmentSummaryLimit{
  result.Text=result.Text[:AttachmentSummaryLimit-50]+"\nPARTIAL: true\n"
  result.Partial=true
 }
 return result,nil
}
