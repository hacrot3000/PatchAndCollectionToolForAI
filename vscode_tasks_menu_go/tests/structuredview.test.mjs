import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const script=readFileSync(new URL('../web/featuremods/structuredview.js',import.meta.url),'utf8');

class MockElement{
  constructor(tag){this.tagName=tag;this.children=[];this.hidden=false;this.value='';this.textContent='';this.classList={
    values:new Set(),add:(name)=>this.classList.values.add(name),remove:(name)=>this.classList.values.delete(name)
  };}
  append(...elements){this.children.push(...elements);}
  replaceChildren(...elements){this.children=[...elements];}
  addEventListener(){}
}
const doc={head:new MockElement('head'),createElement:tag=>new MockElement(tag)};
const context=vm.createContext({document:doc,TextEncoder,console,navigator:{clipboard:{writeText:async()=>{}}}});
vm.runInContext(script,context);
const api=context.TaskMenuStructuredView;
const normalize=value=>JSON.parse(JSON.stringify(value));

function viewFor(path,source){
  const body=new MockElement('body'),pane=new MockElement('pane'),select=new MockElement('select');
  let dispatchCount=0;
  const view={closed:false,file:{path,large_file:false},body,pane,structuredSelect:select,
    structuredMode:'raw',dirty:true,cm:{state:{doc:{toString:()=>source}},requestMeasure:()=>{},dispatch:()=>{dispatchCount++;}}
  };
  return {view,get dispatchCount(){return dispatchCount;}};
}

test('recognizes structured document file extensions without labeling binary as text',()=>{
  for(const path of ['data.json','view.xml','assets/scene.prefab','metadata.meta','data.csv','data.tsv']){
    assert.ok(api.kindForPath(path),path);
  }
  assert.equal(api.kindForPath('scene.bin'),'');
  assert.equal(api.kindForPath('content.json5'),'');
  assert.equal(api.kindForPath('script.gradle'),'');
  assert.deepEqual(normalize(api.modeOptions('json')),['raw','pretty','tree']);
  assert.deepEqual(normalize(api.modeOptions('csv')),['raw','pretty','table']);
});

test('JSON Pretty preserves huge numeric values and original string escapes',()=>{
  const input='{"big":900719925474099312345,"text":"slash\\u002Fhere","list":[1,2,{}],"empty":[]}';
  const pretty=api.formatJson(input);
  assert.ok(pretty.includes('900719925474099312345'));
  assert.ok(pretty.includes('slash\\u002Fhere'));
  assert.ok(pretty.includes('\n  "list": [\n'));
  assert.ok(pretty.includes('"empty": []'));
  assert.equal(api.formatJson('[]'),'[]');
  assert.equal(api.formatJson('{}'),'{}');
  assert.throws(()=>api.formatJson('{"oops":}'),SyntaxError);
});

test('CSV parser honors RFC4180 quoting, escaped quotes, CRLF and multiline cells',()=>{
  const rows=normalize(api.parseDelimited('name,note\r\nAlice,"Hi, ""world"""\r\nBob,"two\nlines"\r\n',','));
  assert.deepEqual(rows,[['name','note'],['Alice','Hi, "world"'],['Bob','two\nlines']]);
  assert.ok(api.formatDelimited(rows).includes('two↵lines'));
  assert.deepEqual(normalize(api.parseDelimited('a\tb\n1\t2','\t')),[['a','b'],['1','2']]);
  assert.throws(()=>api.parseDelimited('a,"unfinished',','),/Unterminated/);
  assert.throws(()=>api.parseDelimited('a,xy"bad',','),/Unexpected quote/);
});

test('Pretty preview reads unsaved editor content but never dispatches, saves or marks dirty',()=>{
  const sample='{"a":[1,2],"nested":{"flag":true}}';
  const fixture=viewFor('settings.json',sample);
  const {view}=fixture;
  assert.equal(api.setMode(view,'pretty'),true);
  assert.equal(view.structuredMode,'pretty');
  assert.equal(view.dirty,true);
  assert.equal(fixture.dispatchCount,0);
  assert.ok(view.body.classList.values.has('structured-hidden'));
  assert.equal(view.structuredContent.children[0].tagName,'pre');
  assert.ok(view.structuredContent.children[0].textContent.includes('\n'));
  assert.equal(api.setMode(view,'raw'),true);
  assert.equal(view.structuredMode,'raw');
  assert.equal(view.body.classList.values.has('structured-hidden'),false);
  assert.equal(fixture.dispatchCount,0);
  assert.equal(view.cm.state.doc.toString(),sample);
});

test('bad JSON does not replace Raw editor or mark the document dirty',()=>{
  const fixture=viewFor('bad.json','{"a":1,}');
  assert.throws(()=>api.setMode(fixture.view,'pretty'),SyntaxError);
  assert.equal(fixture.view.structuredMode,'raw');
  assert.equal(fixture.view.body.classList.values.has('structured-hidden'),false);
  assert.equal(fixture.dispatchCount,0);
});

test('oversized structured previews are rejected without changing raw source',()=>{
  const fixture=viewFor('big.json','"'+ 'x'.repeat(1024*1024)+'"');
  assert.throws(()=>api.setMode(fixture.view,'pretty'),/1 MiB/);
  assert.equal(fixture.view.structuredMode,'raw');
  assert.equal(fixture.dispatchCount,0);
});
