const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const test=require('node:test');

const root=__dirname;
const helpCode=fs.readFileSync(path.join(root,'web/featuremods/help.js'),'utf8');
const menuCode=fs.readFileSync(path.join(root,'web/featuremods/menus.js'),'utf8');
const loadCode=fs.readFileSync(path.join(root,'web/featuremods/next.js'),'utf8');
const helpLabels=[
  'Ctrl/Cmd + Shift + P','Ctrl/Cmd + P','Ctrl/Cmd + Shift + F',
  'Ctrl/Cmd + Shift + Backquote','Ctrl + Alt + W','Esc',
  'Ctrl/Cmd + F','Ctrl/Cmd + H','Ctrl/Cmd + S','Ctrl/Cmd + G',
  'Ctrl + C','Tabs & Split','Patch Tool'
];

test('Settings Help is loaded before the Settings menu and has a real entry point',()=>{
  assert.ok(loadCode.indexOf("import '/featuremods/help.js';")<loadCode.indexOf("import '/featuremods/menus.js';"));
  assert.match(menuCode,/addSection\(settings\.pop,'HELP',\[helpButton\]\)/);
  assert.match(menuCode,/globalThis\.TaskMenuHelp\?\.open\?\.\(\)/);
  for(const label of helpLabels)assert.ok(helpCode.includes(label),'Missing documented shortcut/topic '+label);
  assert.ok(!helpCode.includes('innerHTML'),'Help must not inject HTML');
});

function harness(){
  const nodes=[];
  let currentActive={isConnected:true,focused:false,focus(){this.focused=true;document.activeElement=this;}};
  function element(tag){
    const node={
      tagName:tag,children:[],attributes:{},hidden:false,value:'',className:'',isConnected:true,listeners:{},
      append(...values){this.children.push(...values);},
      setAttribute(k,v){this.attributes[k]=v;},
      addEventListener(kind,cb){this.listeners[kind]=cb;},
      get classList(){
        const self=this;
        return {contains(c){return self.className.split(/\s+/).includes(c);},
          add(c){if(!this.contains(c))self.className+=' '+c;},
          remove(c){self.className=self.className.split(/\s+/).filter(s=>s!==c).join(' ');}};
      },
      focus(){document.activeElement=this;},
    };
    nodes.push(node);return node;
  }
  const document={createElement:element,head:{append(){}},body:{append(){}},activeElement:currentActive};
  vm.runInNewContext(helpCode,{document,globalThis:{api:null}}, {filename:'help.js',timeout:1000});
  // A fresh context with a plain global object is needed to recover the export.
  // We read it off the context rather than relying on browser globals.
  const global={};
  vm.runInNewContext(helpCode,{document,globalThis:global}, {filename:'help.js',timeout:1000});
  return {nodes,document,api:global.TaskMenuHelp,prior:currentActive};
}
test('Help opens, filters topics, and closes without touching browser tabs',()=>{
  const {nodes,document,api,prior}=harness();
  assert.equal(typeof api.open,'function');
  api.open();
  assert.equal(api.visible,true);
  const search=nodes.find(n=>n.className==='taskdeck-help-search');
  assert.equal(document.activeElement,search);
  search.value='Ctrl/Cmd + Shift + F';
  search.oninput();
  const descriptions=nodes.filter(n=>n.className==='taskdeck-help-description');
  assert.equal(descriptions.filter(n=>!n.hidden).length,1);
  assert.match(descriptions.find(n=>!n.hidden).textContent,/project/);
  const dialog=nodes.find(n=>n.className==='taskdeck-help-dialog');
  let prevented=false,stopped=false;
  dialog.listeners.keydown({
    key:'Escape',preventDefault(){prevented=true;},stopPropagation(){stopped=true;}
  });
  assert.equal(api.visible,false);
  assert.equal(prevented,true);assert.equal(stopped,true);
  assert.equal(document.activeElement,prior);
});
