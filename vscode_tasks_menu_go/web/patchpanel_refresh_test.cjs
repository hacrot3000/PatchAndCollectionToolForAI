// Run with: node vscode_tasks_menu_go/web/patchpanel_refresh_test.cjs
// Execute the production DOM reconciliation helper with a tiny DOM model.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'featuremods/patchpanel.js'), 'utf8');
const begin = source.indexOf('  function reconcilePatchRows(host,desired){');
const end = source.indexOf('  function patchRow(host,key,signature,node){',begin);
assert.ok(begin >= 0 && end > begin, 'keyed DOM reconciler is present');
const reconcile = vm.runInNewContext(source.slice(begin,end)+'\nreconcilePatchRows;');

class Host {
  constructor() { this.children = []; }
  get firstChild() { return this.children[0] || null; }
  insertBefore(node,reference) {
    const old = this.children.indexOf(node);
    if(old >= 0) this.children.splice(old,1);
    const index = reference == null ? -1 : this.children.indexOf(reference);
    if(index < 0) this.children.push(node);
    else this.children.splice(index,0,node);
    node.parentNode = this;
  }
}
class Node {
  constructor(key,signature) {
    this.dataset = {patchRowKey:key,patchRowVersion:signature};
    this.handler = () => {};
    this.checked = false;
    this.priority = '';
    this.focused = false;
    this.parentNode = null;
  }
  get nextSibling(){
    if(!this.parentNode) return null;
    const siblings = this.parentNode.children;
    return siblings[siblings.indexOf(this)+1] || null;
  }
  remove(){
    if(!this.parentNode) return;
    const siblings=this.parentNode.children;
    const index=siblings.indexOf(this);
    if(index>=0)siblings.splice(index,1);
    this.parentNode=null;
  }
}
function assertOrder(host,nodes) {
  assert.equal(host.children.length,nodes.length,'correct number of DOM nodes');
  nodes.forEach((node,index)=>assert.equal(host.children[index],node,'DOM order and identity'));
}
const host=new Host();
const a=new Node('PATCH:a','v1');
const b=new Node('COLLECT:b','v1');
reconcile(host,[a,b]);
assertOrder(host,[a,b]);
a.checked=true;
a.priority='8';
a.focused=true;
const click=a.handler;
const untouchedA=new Node('PATCH:a','v1');
const untouchedB=new Node('COLLECT:b','v1');
reconcile(host,[untouchedA,untouchedB]);
assertOrder(host,[a,b]);
assert.equal(a.handler,click,'existing button handlers are retained');
assert.equal(a.checked,true,'checked state retained');
assert.equal(a.priority,'8','priority retained');
assert.equal(a.focused,true,'focused control retained');

const c=new Node('COLLECT:c','v1');
reconcile(host,[untouchedA,c,untouchedB]);
assertOrder(host,[a,c,b]);
assert.equal(a.checked,true,'insert does not disrupt prior selection');

const changedB=new Node('COLLECT:b','v2');
reconcile(host,[c,new Node('PATCH:a','v1'),changedB]);
assertOrder(host,[c,a,changedB]);
assert.equal(b.parentNode,null,'changed node removed');
assert.equal(a.checked,true,'reorder preserves unchanged input');

reconcile(host,[new Node('PATCH:a','v1')]);
assertOrder(host,[a]);
assert.equal(c.parentNode,null,'removed COLLECT gone');
assert.equal(changedB.parentNode,null,'removed updated row gone');

reconcile(host,[new Node('PATCH:a','v1'),new Node('PATCH:a','v1')]);
assertOrder(host,[a]);
console.log('PASS: keyed PATCH/COLLECT refresh retains unchanged DOM identity, handlers, selection and focus');
