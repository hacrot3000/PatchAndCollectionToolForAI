// Task quick picker runtime regression: keyboard behavior, groups, permissions and task launcher.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const root=__dirname;
const source=fs.readFileSync(path.join(root,'web/featuremods/taskpicker.js'),'utf8');
const imports=fs.readFileSync(path.join(root,'web/featuremods/next.js'),'utf8');
const palette=fs.readFileSync(path.join(root,'web/featuremods/terminalpalette.js'),'utf8');
function fixture(){
 return [
  {id:1,label:'Build Client',menu_label:'Build Client',group:['Client','Build'],detail:'Compile client',
   raw:{group:{kind:'build',isDefault:true}}},
  {id:2,label:'Build Server',menu_label:'Build Server',group:['Server','build'],detail:'Compile server',
   raw:{group:{kind:'build'}}},
  {id:3,label:'Run Unit Tests',menu_label:'Run Unit Tests',group:['Tests'],detail:'Unit tests',
   raw:{group:{kind:'test'}}},
  {id:4,label:'Deploy staging',menu_label:'Deploy staging',group:['Deploy'],detail:'Stage deploy',raw:{}},
  {id:5,label:'Tools check',menu_label:'Tools check',group:['Build'],detail:'Toolchain',raw:{}}
 ];
}
function runtime({canRun=true,tasks=fixture()}={}){
 const keys=new Map(),storage=new Map(),runCalls=[];
 const document={activeElement:null};
 function node(tag){
  const events=new Map();
  const element={tag,children:[],listeners:events,attributes:new Map(),className:'',
   textContent:'',value:'',disabled:false,isConnected:true,style:{},
   append(...children){this.children.push(...children);},
   replaceChildren(...children){this.children=children;},
   setAttribute(name,value){this.attributes.set(name,String(value));},
   getAttribute(name){return this.attributes.get(name);},
   addEventListener(name,fn){events.set(name,fn);},
   focus(){document.activeElement=this;},
   scrollIntoView(){},
   querySelectorAll(selector){
    if(selector==='.task-picker-result')return this.children.filter(n=>n.className?.split(' ').includes('task-picker-result'));
    if(selector==='.task-picker-result.selected')return this.children.filter(n=>n.className?.split(' ').includes('task-picker-result')&&n.className.split(' ').includes('selected'));
    return [];
   },
   querySelector(selector){return this.querySelectorAll(selector)[0]||null;},
   insertBefore(child){this.children.unshift(child);}
  };
  element.classList={
   add(value){if(!this.contains(value))element.className+=' '+value;},
   remove(value){element.className=element.className.split(/\s+/).filter(x=>x!==value).join(' ');},
   toggle(value,enabled){if(enabled===undefined)enabled=!this.contains(value);if(enabled)this.add(value);else this.remove(value);return enabled;},
   contains(value){return element.className.split(/\s+/).includes(value);}
  };
  return element;
 }
 const header=node('header'),reload=node('button');header.append(reload);
 document.createElement=node;document.head=node('head');document.body=node('body');
 document.querySelector=selector=>selector==='header'?header:selector==='#reload'?reload:null;
 document.activeElement=node('button');
 const window={addEventListener:(name,fn)=>keys.set(name,fn)};
 const localStorage={getItem:key=>storage.get(key)||null,setItem:(key,value)=>storage.set(key,value)};
 const app={taskData:{workspace:'/workspace/project',tasks},browserLeaseLost:false,
  hasPermission:name=>name==='tasks.run'?canRun:true,
  startTask:async task=>{runCalls.push(task.id);},
  showError:error=>{throw error;}};
 const context={console,document,window,localStorage,TaskMenuApp:app,Promise};
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(source,context,{filename:'taskpicker.js'});
 const backdrop=document.body.children[0],dialog=backdrop.children[0];
 const input=dialog.children[1],results=dialog.children[3];
 function key({key,ctrlKey=true,altKey=false,shiftKey=false,metaKey=false,repeat=false}){
  const event={key,ctrlKey,altKey,shiftKey,metaKey,repeat,isComposing:false,
   prevented:false,stopped:false,preventDefault(){this.prevented=true;},
   stopPropagation(){this.stopped=true;},getModifierState(){return false;}};
  keys.get('keydown')(event);return event;
 }
 function pressInput(key){
  const event={key,preventDefault(){},stopPropagation(){}};
  input.listeners.get('keydown')(event);
 }
 return {context,app,header,backdrop,dialog,input,results,runCalls,keys,
  picker:context.TaskMenuTaskPicker,key,pressInput};
}
const settle=()=>new Promise(resolve=>setImmediate(resolve));

test('task picker is imported and exposed from the Command Palette',()=>{
 assert.match(imports,/featuremods\/taskpicker\.js/);
 for(const label of ['Tasks: Run Task','Tasks: Run Build Task','Tasks: Run Test Task','Tasks: Rerun Last Task'])
  assert.ok(palette.includes(label),label);
});
test('Build/Test classification uses VS Code task group and nested menu group',()=>{
 const h=runtime();
 assert.deepEqual(Array.from(h.picker.taskList('build').map(item=>item.task.id)),[1,2,5]);
 assert.deepEqual(Array.from(h.picker.taskList('test').map(item=>item.task.id)),[3]);
 assert.deepEqual(Array.from(h.picker.taskList('all').map(item=>item.task.id)),[1,2,3,4,5]);
 assert.deepEqual(Array.from(h.picker.taskList('build','client compile').map(item=>item.task.id)),[1]);
 assert.deepEqual(Array.from(h.picker.taskList('all','deploy STAGE').map(item=>item.task.id)),[4]);
 assert.deepEqual(Array.from(h.picker.taskList('test','non-existent')),[]);
});
test('Ctrl+Shift+B and TaskDeck keys open the appropriate modal; no run without Enter',()=>{
 const h=runtime();
 const build=h.key({key:'B',shiftKey:true});
 assert.equal(build.prevented,true);
 assert.equal(h.picker.visible,true);
 assert.equal(h.results.children.length,3);
 assert.equal(h.dialog.getAttribute('aria-label'),'Tasks: Run Build Task');
 assert.equal(h.runCalls.length,0);
 h.pressInput('Escape');assert.equal(h.picker.visible,false);
 const all=h.key({key:'r',altKey:true});
 assert.equal(all.prevented,true);
 assert.equal(h.results.children.length,5);
 assert.equal(h.dialog.getAttribute('aria-label'),'Tasks: Run Task');
 h.picker.close();
 const testKey=h.key({key:'T',altKey:true,shiftKey:true});
 assert.equal(testKey.prevented,true);
 assert.equal(h.results.children.length,1);
 assert.equal(h.dialog.getAttribute('aria-label'),'Tasks: Run Test Task');
 h.picker.close();
 const linuxTerminal=h.key({key:'t',altKey:true});
 assert.equal(linuxTerminal.prevented,false,'Ctrl+Alt+T is reserved by Linux');
});
test('filter, arrows and Enter use the existing task launcher; last task can be rerun',async()=>{
 const h=runtime();
 h.picker.open('all');
 h.input.value='build';
 h.input.listeners.get('input')();
 assert.equal(h.results.children.length,3);
 h.pressInput('ArrowDown');
 assert.equal(h.results.children[1].getAttribute('aria-selected'),'true');
 h.pressInput('Enter');
 await settle();
 assert.deepEqual(h.runCalls,[2]);
 assert.equal(h.picker.visible,false);
 h.picker.rerunLast();
 await settle();
 assert.deepEqual(h.runCalls,[2,2]);
});
test('task permissions and invalid shortcuts do not execute tasks',async()=>{
 const h=runtime({canRun:false});
 h.picker.open('all');
 assert.equal(h.results.children.length,5);
 assert.ok(h.results.children.every(item=>item.disabled));
 h.pressInput('Enter');await settle();
 assert.deepEqual(h.runCalls,[]);
 assert.equal(h.picker.visible,true);
 h.picker.close();
 const extra=h.key({key:'b',shiftKey:true,repeat:true});
 assert.equal(extra.prevented,false);
 const wrong=h.key({key:'b'});
 assert.equal(wrong.prevented,false);
});
test('updated tasks are shown without stale cache while the popup is open',()=>{
 const h=runtime();
 h.picker.open('all');
 assert.equal(h.results.children.length,5);
 h.app.taskData.tasks=[{id:77,label:'New Task',menu_label:'New Task',group:['Build'],raw:{}}];
 h.keys.get('taskmenu:tasks')();
 assert.equal(h.results.children.length,1);
 assert.match(h.results.children[0].children[0].children[0].textContent,/New Task/);
});
