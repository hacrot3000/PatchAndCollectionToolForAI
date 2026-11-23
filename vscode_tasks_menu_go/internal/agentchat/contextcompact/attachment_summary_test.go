package contextcompact

import (
 "bytes"
 "encoding/binary"
 "fmt"
 "strings"
 "testing"
)

func TestAttachmentNimOutlinePreservesImportsProcTemplateEventAndExactRanges(t *testing.T){
 source:=strings.Join([]string{
  "import private/[common, types, dispatcher]",
  "from std/strutils import split",
  "var",
  "  gHeroExtraAttrs: int",
  "  customProfessionbyPos: bool",
  "",
  "template resetCustomProfessionByPos*() =",
  "  customProfessionbyPos = false",
  "",
  "proc mwfDecodeHeroChangeHeroId*(nHeroId: HeroId): int =",
  "  result = 42",
  "  # implementation body hidden",
  "",
  "proc mwfEncodeMainFormationRaw(",
  "    realPos: int32,",
  "    maxWave: int32",
  "): MwfBuildMainResult =",
  "  result.raw = 0",
  "",
  "ee.on(READY) do(a: Args):",
  "  ReadExtraHeroConfig()",
  "",
  "ee.on(PREPARE_HOOK) do(a: Args):",
  "  args.fh.prepare(faddr One, NI_One)",
 },"\n")+"\n"
 got,err:=CompactAttachment("hero.nim","src/hero.nim","",int64(len(source)),strings.NewReader(source))
 if err!=nil{t.Fatal(err)}
 if got.Kind!="source_outline"||got.Partial{
  t.Fatalf("Nim summary unexpectedly incomplete: %+v",got)
 }
 for _,want:=range []string{
  "FILE_PATH: src/hero.nim","LANGUAGE: Nim",
  "L1-L1 [import] import private/[common, types, dispatcher]",
  "L3-L6 [var] var gHeroExtraAttrs customProfessionbyPos",
  "L7-L9 [template] template resetCustomProfessionByPos*() =",
  "L10-L13 [proc] proc mwfDecodeHeroChangeHeroId*",
  "L14-L19 [proc] proc mwfEncodeMainFormationRaw( realPos: int32, maxWave: int32 ): MwfBuildMainResult =",
  "L20-L22 [event] ee.on(READY)",
  "L23-L24 [event] ee.on(PREPARE_HOOK)",
 }{
  if !strings.Contains(got.Text,want){t.Errorf("missing %q in:\n%s",want,got.Text)}
 }
 if strings.Contains(got.Text,"result = 42")||strings.Contains(got.Text,"args.fh.prepare"){
  t.Fatal("implementation body leaked into compact Nim outline")
 }
}

func TestAttachmentLargeNimSourceBoundedButDeclarationsIndexed(t *testing.T){
 var src strings.Builder
 src.WriteString("import std/[times, os]\n")
 for i:=0;i<1100;i++{
  fmt.Fprintf(&src,"proc handler%04d*(value: int): int =\n",i)
  src.WriteString("  result = value + 10\n")
  src.WriteString("  echo \"UNTRUSTED_IMPLEMENTATION_BODY_NOT_FOR_PROVIDER\"\n")
 }
 input:=src.String()
 if len(input)<=32<<10{t.Fatal("fixture must exceed legacy 32 KiB bound")}
 got,err:=CompactAttachment("handlers.nim","handlers.nim","",int64(len(input)),strings.NewReader(input))
 if err!=nil{t.Fatal(err)}
 if len(got.Text)>AttachmentSummaryLimit{t.Fatalf("compaction exceeds bounded model payload: %d",len(got.Text))}
 if !strings.Contains(got.Text,"proc handler0000*")||
    !strings.Contains(got.Text,"OTHER_DECLARATIONS_OMITTED:")||!got.Partial{
  t.Fatalf("large file did not keep bounded navigable signatures: %s",got.Text)
 }
 if strings.Contains(got.Text,"UNTRUSTED_IMPLEMENTATION_BODY_NOT_FOR_PROVIDER"){
  t.Fatal("source body leaked into model context")
 }
}

func TestAttachmentOtherLanguagesOnlyDeclarations(t *testing.T){
 for _,tc:=range []struct{name,source,want string}{
  {"main.go","package main\nimport \"fmt\"\nfunc Safe(name string) string {\n secret := \"do not include\"\n return name\n}\n","func Safe(name string) string"},
  {"hello.py","from os import path\nclass Service:\n    def work(self, x):\n        print('private detail')\n","def work(self, x):"},
  {"main.ts","import {x} from 'pkg';\nexport function calculate(value: number) {\n  const password = 'secret';\n}\n","export function calculate"},
  {"main.rs","use std::io;\npub fn serve(x: u32) -> u32 {\n let secret = 15;\n}\n","pub fn serve"},
  {"app.php","<?php\nuse Foo\\Bar;\nfunction calculate($arg) {\n echo 'secret';\n}\n","function calculate"},
  {"migrate.sql","CREATE TABLE things(id INT);\nINSERT INTO things VALUES (123);\n","CREATE TABLE things"},
 }{
  t.Run(tc.name,func(t *testing.T){
   got,err:=CompactAttachment(tc.name,tc.name,"",int64(len(tc.source)),strings.NewReader(tc.source))
   if err!=nil{t.Fatal(err)}
   if got.Kind!="source_outline"||!strings.Contains(got.Text,tc.want){
    t.Fatalf("missing declarations from %s: %s",tc.name,got.Text)
   }
   for _,secret:=range []string{"private detail","do not include","VALUES (123)","'secret'"}{
    if strings.Contains(got.Text,secret){t.Fatalf("body leaked: %s",got.Text)}
   }
  })
 }
}
func TestAttachmentNonCodeOnlyMetadataAndImageExif(t *testing.T){
 for _,tc:=range []struct{name,data string}{
  {"notes.txt","Notes contain SECRET_NOT_TO_EGRESS\n"}, {"config.yaml","password: hidden\n"},
  {"huge.csv","private,record,that,must,not,be,forwarded\n"},
 }{
  got,err:=CompactAttachment(tc.name,"docs/"+tc.name,"",int64(len(tc.data)),strings.NewReader(tc.data))
  if err!=nil{t.Fatal(err)}
  if got.Kind!="metadata"||strings.Contains(got.Text,"SECRET_NOT_TO_EGRESS")||
    strings.Contains(got.Text,"password:")||strings.Contains(got.Text,"private,record"){
   t.Fatalf("non-code content leaked into summary: %s",got.Text)
  }
  if !strings.Contains(got.Text,"FILE_SIZE_BYTES:"){t.Fatal(got.Text)}
 }
 // An EXIF-only JPEG prefix is enough; no photo bytes or GPS coordinates
 // should be placed into a model-visible text attachment.
 tiff:=make([]byte,32)
 copy(tiff[:4],[]byte{'I','I',42,0})
 binary.LittleEndian.PutUint32(tiff[4:],8)
 binary.LittleEndian.PutUint16(tiff[8:],1)
 binary.LittleEndian.PutUint16(tiff[10:],0x0110)
 binary.LittleEndian.PutUint16(tiff[12:],2)
 binary.LittleEndian.PutUint32(tiff[14:],6)
 binary.LittleEndian.PutUint32(tiff[18:],26)
 copy(tiff[26:],[]byte("Nikon\x00"))
 app1:=append([]byte("Exif\x00\x00"),tiff...)
 jpeg:=[]byte{0xff,0xd8,0xff,0xe1,0,0}
 binary.BigEndian.PutUint16(jpeg[4:],uint16(len(app1)+2))
 jpeg=append(jpeg,app1...);jpeg=append(jpeg,0xff,0xd9)
 result,err:=CompactAttachment("camera.jpg","camera.jpg","",int64(len(jpeg)),bytes.NewReader(jpeg))
 if err!=nil{t.Fatal(err)}
 if result.Kind!="image_metadata"||!strings.Contains(result.Text,"EXIF_CameraModel: Nikon"){
  t.Fatalf("missing safe JPEG EXIF model: %s",result.Text)
 }
}

func TestAttachmentSameLineImplementationsStayOutOfOutline(t *testing.T){
 for _,tc:=range []struct{name,source,signature,body string}{
  {"inline.nim","proc inline*(): int = return 123\n","proc inline*(): int =","return 123"},
  {"inline.js","export function run(){console.log('private body')}\n","export function run(){","private body"},
  {"secret.ts","const token = 'never forward literal'\n","const token = [initializer omitted]","never forward literal"},
  {"config.h","#define SECRET_VALUE some-private-value\n","#define SECRET_VALUE","some-private-value"},
 }{
  summary,err:=CompactAttachment(tc.name,tc.name,"",int64(len(tc.source)),strings.NewReader(tc.source))
  if err!=nil{t.Fatal(err)}
  if !strings.Contains(summary.Text,tc.signature)||strings.Contains(summary.Text,tc.body){
   t.Fatalf("%s leaked implementation/value or lost signature: %s",tc.name,summary.Text)
  }
 }
}
