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


test('Help navigation groups features and searches across categories',()=>{
  const {nodes,api}=harness();
  assert.ok(api.topics.length>=10,'important features have their own Help groups');
  for(const feature of ['Database & SQL results','SSH & connections','Git','Diff & compare','FTP / SFTP & files','Patch Tool & Collection','AI Assistant']){
    assert.ok(api.topics.some(t=>t.title===feature),'missing group '+feature);
  }
  api.open();
  const nav=nodes.find(n=>n.className==='taskdeck-help-nav');
  const sections=nodes.filter(n=>n.className==='taskdeck-help-section');
  const search=nodes.find(n=>n.className==='taskdeck-help-search');
  const buttons=nav.children;
  assert.equal(sections.filter(n=>!n.hidden).length,1,'default view shows a concise Quick start group');
  const database=buttons.find(n=>n.children[0]?.textContent==='Database & SQL results');
  assert.ok(database);
  database.onclick();
  const visibleSections=sections.filter(n=>!n.hidden);
  assert.equal(visibleSections.length,1);
  assert.equal(visibleSections[0].children[0].textContent,'Database & SQL results');
  search.value='Separate ZIP for each result tab';
  search.oninput();
  assert.equal(nodes.filter(n=>n.className==='taskdeck-help-description'&&!n.hidden).length,1);
  search.value='ssh reconnect';
  search.oninput();
  assert.ok(nodes.filter(n=>n.className==='taskdeck-help-description'&&!n.hidden).length>0,
    'category keywords and feature terms should match together');
  search.value='no-topic-with-this-string';
  search.oninput();
  assert.equal(nodes.find(n=>n.className==='taskdeck-help-empty').hidden,false);
  const all=buttons.find(n=>n.children[0]?.textContent==='All topics');
  all.onclick();
  assert.equal(sections.filter(n=>!n.hidden).length,api.topics.length);
});

test('Help opens a category or filtered help query via reserved terminal command',()=>{
  const {nodes,api}=harness();
  api.open('database');
  // Category names are matched by id or title; otherwise treat input as global search.
  const search=nodes.find(n=>n.className==='taskdeck-help-search');
  const sections=nodes.filter(n=>n.className==='taskdeck-help-section');
  assert.equal(sections.filter(n=>!n.hidden).length,1);
  assert.equal(sections.find(n=>!n.hidden).children[0].textContent,'Database & SQL results');
  api.open('SSH & connections');
  assert.equal(sections.find(n=>!n.hidden).children[0].textContent,'SSH & connections');
  api.open('reconnect');
  assert.equal(search.value,'reconnect');
  assert.ok(nodes.filter(n=>n.className==='taskdeck-help-description'&&!n.hidden).length>0);
  api.close();
  assert.equal(api.visible,false);
});

test('Settings, palette and terminal all provide a Help entry point',()=>{
  const palette=fs.readFileSync(path.join(root,'web/featuremods/terminalpalette.js'),'utf8');
  const agents=fs.readFileSync(path.join(root,'web/featuremods/smartterminal_agents.js'),'utf8');
  assert.match(palette,/id:'taskdeck-help'/);
  assert.match(palette,/TaskMenuHelp\?\.open\?\.\(\)/);
  assert.match(agents,/ai\|do\|help/);
  assert.match(agents,/TaskMenuHelp\?\.open\?\./);
  assert.ok(helpCode.includes('Separate ZIP for each result tab'));
  assert.ok(helpCode.includes('Auto reconnect'));
  assert.ok(helpCode.includes('Branch Compare'));
  assert.ok(helpCode.includes('Transfer queue'));
});
