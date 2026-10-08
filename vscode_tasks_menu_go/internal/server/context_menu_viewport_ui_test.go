package server

import (
    "context"
    "os/exec"
    "strings"
    "testing"
    "time"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func TestContextMenusUseSharedViewportPlacement(t *testing.T) {
    next,err:=webassets.Files.ReadFile("featuremods/next.js")
    if err!=nil{t.Fatal(err)}
    loader:=string(next)
    shared:=strings.Index(loader,"import '/featuremods/contextviewport.js';")
    if shared<0{t.Fatal("shared menu geometry must be loaded")}
    for _,name:=range []string{"filetransfer.js","explorer.js","connections.js","tabcontext.js","database_workbench.js","gitstatus.js","projectfileactions.js"}{
        if at:=strings.Index(loader,"/featuremods/"+name);at<shared{
            t.Fatalf("%s must load after contextviewport.js",name)
        }
    }
    cases:=[]struct{filename string;expected []string}{
        {"explorer.js",[]string{"contextMenu.classList.add('visible')","TaskMenuContextViewport.place(contextMenu,event.clientX,event.clientY)"}},
        {"filetransfer.js",[]string{"TaskMenuContextViewport.place(menu,x,y)"}},
        {"connections.js",[]string{"TaskMenuContextViewport.place(menu,x,y)"}},
        {"gitstatus.js",[]string{"TaskMenuContextViewport.place(menu,event.clientX,event.clientY)"}},
        {"projectfileactions.js",[]string{"function clamp(x,y)","TaskMenuContextViewport.place(menu,x,y)"}},
        {"database_workbench.js",[]string{"TaskMenuContextViewport.place(menu,x,y)","TaskMenuContextViewport.place(submenu,anchor.right,anchor.top,{anchor,submenu:true})"}},
        {"tabcontext.js",[]string{"TaskMenuContextViewport.place(menu,x,y)","TaskMenuContextViewport.place(submenu,ownerRect.right,ownerRect.top,{anchor:ownerRect,submenu:true})"}},
    }
    for _,tc:=range cases{
        data,readErr:=webassets.Files.ReadFile("featuremods/"+tc.filename)
        if readErr!=nil{t.Fatal(readErr)}
        for _,want:=range tc.expected{
            if !strings.Contains(string(data),want){
                t.Errorf("%s missing viewport-safe context menu placement %q",tc.filename,want)
            }
        }
    }
}

func TestContextViewportBoundsOversizedMenusAndSupportsZoom(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/contextviewport.js")
    if err!=nil{t.Fatal(err)}
    js:=string(data)
    for _,want:=range []string{
        "window.visualViewport",
        "vv.offsetLeft",
        "vv.offsetTop",
        "menu.style.maxWidth=roomWidth+'px'",
        "menu.style.maxHeight=roomHeight+'px'",
        "menu.style.minWidth=Math.min(requestedMinimum,roomWidth)+'px'",
        "menu.style.overflowY='auto'",
        "menu.style.overscrollBehavior='contain'",
        "if(left+width>v.right-margin)left=anchor.left-width-gap",
        "top=anchor.top-height-gap",
        "globalThis.TaskMenuContextViewport={place,viewport}",
    } {
        if !strings.Contains(js,want){t.Errorf("shared context viewport helper missing %q",want)}
    }
    for _,selector:=range []string{
        ".project-explorer-context",".ft-context",".task-connection-context-menu",
        ".git-graph-context",".project-file-context",".db-context-menu",
        ".tab-context-menu",".tab-context-submenu",
    }{
        if !strings.Contains(js,selector){t.Errorf("context menu %s has no shared scroll style",selector)}
    }
}

func TestContextViewportGeometryNodeSmoke(t *testing.T) {
    if _,err:=exec.LookPath("node");err!=nil{
        t.Skip("Node.js unavailable; static context menu contracts remain tested")
    }
    ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
    defer cancel()
    command:=exec.CommandContext(ctx,"node","../../web/tests/contextviewport.test.cjs")
    output,err:=command.CombinedOutput()
    if err!=nil{t.Fatalf("context menu viewport JS smoke: %v\n%s",err,output)}
    if !strings.Contains(string(output),"PASS: 10 menu geometry scenarios"){
        t.Fatalf("unexpected JS viewport smoke result: %s",output)
    }
}
