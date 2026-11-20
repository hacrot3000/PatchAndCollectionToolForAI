package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSplitMergeCombinesWholeGroupsWithoutDetachingTarget(t *testing.T) {
 data, err := webassets.Files.ReadFile("featuremods/split.js")
 if err != nil { t.Fatal(err) }
 js := string(data)
 for _, value := range []string{
  "function mergeSplitGroups(sourceID,targetID,orientation)",
  "const sourceRoot=findRoot(sourceID),targetRoot=findRoot(targetID)",
  "roots=roots.filter(root=>root!==sourceRoot&&root!==targetRoot)",
  "roots.push(splitNode(target,source,orientation,0.5))",
  "function mergeTargetsFor(sourceID)",
  "const canMerge=mergeTargetsFor(view.meta.id).length>0",
 } {
  if !strings.Contains(js,value) { t.Errorf("missing merge group protection: %q",value) }
 }
 a := strings.Index(js,"function mergeWith(view,orientation){")
 if a<0 { t.Fatal("mergeWith missing") }
 b := strings.Index(js[a:],"function bindFocusTracking(view)")
 if b<0 { t.Fatal("mergeWith end marker missing") }
 body := js[a:a+b]
 if strings.Contains(body,"detachTerminal(")||strings.Contains(body,"splitLeaf("){
  t.Fatal("merge must not detach existing sibling panes")
 }
 if !strings.Contains(body,"mergeSplitGroups(view.meta.id,target.meta.id,orientation)"){
  t.Fatal("merge must combine whole groups")
 }
}
