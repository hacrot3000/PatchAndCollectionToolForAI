import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source=readFileSync(new URL('../web/featuremods/projectlauncher.js',import.meta.url),'utf8');
const menus=readFileSync(new URL('../web/featuremods/menus.js',import.meta.url),'utf8');
const next=readFileSync(new URL('../web/featuremods/next.js',import.meta.url),'utf8');

function setup({sharedMode=false,supported=true,installed=false}={}){
 let getCount=0,posts=0;
 const button={hidden:false,disabled:false,textContent:'',title:'',onclick:null};
 const doc={createElement:tag=>{
  assert.equal(tag,'button');return button;
 }};
 const app={
  taskData:{workspace:'/projects/example'},sharedMode,
  showError:error=>{throw error},
  jsonFetch:async(url,opts)=>{
   assert.equal(url,'/api/system/project-launcher');
   if(opts?.method==='POST'){posts++;return {supported:true,installed:true,path:'/home/test/.local/share/applications/taskdeck.desktop'}}
   getCount++;
   return {supported,installed,path:'/home/test/.local/share/applications/taskdeck.desktop'};
  }
 };
 const ctx=vm.createContext({document:doc,window:{addEventListener:()=>{}},globalThis:{TaskMenuApp:app},console});
 vm.runInContext(source,ctx);
 return {button,api:ctx.globalThis.TaskMenuProjectLauncher,requests:()=>({getCount,posts})};
}
test('Settings exposes the launcher only when native host reports support',async()=>{
 const enabled=setup();
 await enabled.api.refresh();
 assert.equal(enabled.button.hidden,false);
 assert.match(enabled.button.textContent,/Create project launcher/);
 const unsupported=setup({supported:false});
 await unsupported.api.refresh();
 assert.equal(unsupported.button.hidden,true);
 const shared=setup({sharedMode:true});
 await shared.api.refresh();
 assert.equal(shared.button.hidden,true);
 assert.equal(shared.requests().getCount,0);
});

test('one click posts once and shows installed state',async()=>{
 const ui=setup();
 await ui.api.refresh();
 await ui.button.onclick();
 assert.equal(ui.requests().posts,1);
 assert.equal(ui.button.disabled,false);
 assert.match(ui.button.textContent,/Launcher created/);
 assert.match(ui.button.title,/applications/);
});

test('grouped Settings exposes launch control under Workspace without shared users',()=>{
 assert.match(next,/import '\/featuremods\/projectlauncher\.js'/);
 assert.match(menus,/only\(!app\.sharedMode,globalThis\.TaskMenuProjectLauncher\?\.button\)/);
});
