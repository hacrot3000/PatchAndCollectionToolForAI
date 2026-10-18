import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source=readFileSync(new URL('../web/featuremods/menus.js',import.meta.url),'utf8');
class Element {
 constructor(tag){this.tagName=tag;this.children=[];this.parentElement=null;this.className='';
  this.dataset={};this.value='';this.hidden=false;this.textContent='';this.title='';
  this.classList={
   add:(...names)=>{for(const name of names)if(!this.className.split(' ').includes(name))this.className+=' '+name},
   remove:(...names)=>{this.className=this.className.split(' ').filter(x=>!names.includes(x)).join(' ')},
   contains:name=>this.className.split(' ').includes(name),
   toggle:(name,on)=>{if(on===undefined)on=!this.classList.contains(name);on?this.classList.add(name):this.classList.remove(name);return on}
  };
 }
 append(...children){for(const child of children){if(!child)continue;this.children.push(child);child.parentElement=this}}
 after(){}
 setAttribute(){}
 querySelector(selector){return this.children.find(child=>child.className.split(' ').includes(selector.slice(1)))||null}
}
const find=(root,needle)=>{
 const all=[];
 function walk(n){all.push(n);for(const child of n.children)walk(child)}
 walk(root);
 return all.filter(x=>x.textContent===needle || x.className.split(' ').includes(needle));
};
function render({shared=true,grants=[]}={}){
 const header=new Element('header'),workspace=new Element('div');
 const nodes=new Map();
 for(const id of ['terminal-cwd','env-profile','env-profile-manage','notifications-toggle',
  'running-indicator-settings','self-update-branch-settings','self-update-check',
  'config-backup-settings','tasks-json-editor','reload','edit-title','auth-mode-settings']){
  const node=new Element('button');node.id=id;node.textContent=id;nodes.set('#'+id,node);
 }
 const appearance=new Element('div');appearance.className='appearance-controls';
 nodes.set('.appearance-controls',appearance);
 const document={
  head:new Element('head'),
  createElement:tag=>new Element(tag),
  addEventListener(){},
  querySelector:selector=>selector==='header'?header:selector==='#workspace'?workspace:nodes.get(selector)||null
 };
 const perms=new Set(grants);
 const app={taskData:{workspace:'/project'},sharedMode:shared,views:new Map(),
  hasPermission:key=>perms.has(key),showError(){}};
 const sandbox={document,window:{addEventListener(){}},globalThis:{TaskMenuApp:app},
  localStorage:{getItem(){return null},setItem(){}},setTimeout(){},
  CustomEvent:class {},console:{warn(){}}
 };
 vm.runInNewContext(source,sandbox);
 const menus=find(header,'taskdeck-settings-menu');
 assert.equal(menus.length,1,'Settings dropdown must install');
 return {header,nodes,settings:menus[0],app};
}
const attached=(o,id)=>o.nodes.get('#'+id).parentElement!==null;
test('settings.read only cannot see update branch or shared/global config',()=>{
 const o=render({grants:['settings.read','selfupdate.check']});
 for(const id of ['self-update-branch-settings','config-backup-settings',
  'terminal-cwd','env-profile','env-profile-manage','edit-title',
  'running-indicator-settings','tasks-json-editor']){
  assert.equal(attached(o,id),false,id+' leaked into Settings');
 }
 assert.equal(attached(o,'self-update-check'),true,'explicit selfupdate.check remains visible');
 assert.equal(attached(o,'notifications-toggle'),true,'personal notifications remain available');
 assert.ok(find(o.settings,'patch-ui-mode-control').length,'personal UI switch remains available');
});
test('role with settings.write sees shared configuration, not admin authentication controls',()=>{
 const o=render({grants:['settings.write','settings.read']});
 for(const id of ['self-update-branch-settings','config-backup-settings','terminal-cwd',
   'env-profile','env-profile-manage','edit-title','running-indicator-settings','tasks-json-editor']){
  assert.equal(attached(o,id),true,id+' missing for editor');
 }
 assert.equal(attached(o,'auth-mode-settings'),false,'auth-mode must remain project-admin only');
 assert.equal(attached(o,'self-update-check'),false,'settings.write alone cannot grant selfupdate.check');
});
test('project admin sees all shared settings, including authentication mode',()=>{
 const o=render({grants:['project.admin']});
 for(const id of ['self-update-branch-settings','config-backup-settings','auth-mode-settings',
  'edit-title','terminal-cwd','running-indicator-settings','self-update-check']){
  assert.equal(attached(o,id),true,id+' missing for admin');
 }
});
test('single user maintains original full settings menu',()=>{
 const o=render({shared:false});
 for(const id of ['self-update-branch-settings','config-backup-settings','auth-mode-settings','self-update-check']){
  assert.equal(attached(o,id),true,id+' missing in local mode');
 }
});
