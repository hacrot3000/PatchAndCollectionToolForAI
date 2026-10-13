const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');

const script = fs.readFileSync(path.join(__dirname, 'web/featuremods/tabclosehotkey.js'), 'utf8');

function setup(kind, options={}) {
  const callbacks={capture:[],bubble:[]};
  const clicks=[];
  const dialogs=[];
  class HTMLElement {}
  const tab=new HTMLElement();
  tab.dataset={id:kind==='terminal'?'terminal-1':kind==='patch'?'patch':'editor-1'};
  tab.hidden=false;
  const classes=new Set(['tab','active',kind==='patch'?'task-patch-tab':kind]);
  tab.classList={contains(name){return classes.has(name);}};
  tab.matches=function(selector){
    return selector.split(',').some(item=>classes.has(item.trim().slice(1)));
  };
  tab.children=[{
    classList:{contains(name){return name==='close';}},
    click(){clicks.push('close');}
  }];
  const editorView={closed:false,dirty:Boolean(options.dirty),everEdited:Boolean(options.everEdited),
    findPanel:{classList:{contains(name){return name==='hidden'&&!options.findOpen;}}}};
  const editor={
    editors:new Map([['editor-1',editorView]]),
    closeEditorFind(){clicks.push('close-find');options.findOpen=false;}
  };
  const globals={
    TaskMenuApp:{active:kind==='terminal'?'terminal-1':'external:'+tab.dataset.id},
    TaskMenuEditor:editor
  };
  const window={
    addEventListener(name,handler,capture){
      if(name==='keydown')callbacks[capture?'capture':'bubble'].push(handler);
    }
  };
  const document={
    querySelector(q){return q==='#tabs'?{children:[tab]}:null;},
    querySelectorAll(){return dialogs;}
  };
  vm.runInNewContext(script, {globalThis:globals,document,window,HTMLElement,WeakSet},
    {filename:'tabclosehotkey.js',timeout:1000});
  function keydown(name='Escape',opts={}){
    const event={
      key:name,ctrlKey:false,metaKey:false,altKey:false,shiftKey:false,repeat:false,
      defaultPrevented:false,stopped:false,preventDefault(){this.defaultPrevented=true;},
      stopPropagation(){this.stopped=true;},...opts
    };
    for(const fn of callbacks.capture)fn(event);
    if(opts.afterCapture)opts.afterCapture();
    for(const fn of callbacks.bubble)fn(event);
    return event;
  }
  return {keydown,clicks,dialogs,tab,editorView};
}

for(const kind of ['editor-tab','file-compare-tab','dircmp-tab','hex-tab','task-patch-tab']){
  test('Escape closes eligible clean '+kind,()=>{
    const h=setup(kind,{everEdited:false});
    const e=h.keydown();
    assert.deepEqual(h.clicks,['close']);assert.equal(e.defaultPrevented,true);
  });
}

test('Escape never closes a previously edited saved editor',()=>{
  const h=setup('editor-tab',{dirty:false,everEdited:true});
  const e=h.keydown();
  assert.deepEqual(h.clicks,[]);assert.equal(e.defaultPrevented,false);
});

test('Escape never closes an editor with unsaved changes',()=>{
  const h=setup('editor-tab',{dirty:true,everEdited:true});
  h.keydown();
  assert.deepEqual(h.clicks,[]);
});

test('Escape closes inline Find instead of the editor tab',()=>{
  const h=setup('editor-tab',{findOpen:true});
  h.keydown();assert.deepEqual(h.clicks,['close-find']);
  h.keydown();assert.deepEqual(h.clicks,['close-find','close']);
});

test('Escape never closes terminal or SSH tabs',()=>{
  const h=setup('terminal');h.keydown();assert.deepEqual(h.clicks,[]);
});

test('Escape leaves modal overlays to their own Escape handlers',()=>{
  const h=setup('editor-tab');
  const overlay={getClientRects(){return [1];}};
  h.dialogs.push(overlay);
  const e=h.keydown('Escape',{afterCapture(){h.dialogs.pop();}});
  assert.deepEqual(h.clicks,[]);assert.equal(e.defaultPrevented,false);
  h.keydown();assert.deepEqual(h.clicks,['close']);
});

test('Escape respects a previously prevented event',()=>{
  const h=setup('editor-tab');
  h.keydown('Escape',{defaultPrevented:true});
  assert.deepEqual(h.clicks,[]);
});

test('Escape does not fire for modified shortcut combinations',()=>{
  const h=setup('editor-tab');
  h.keydown('Escape',{shiftKey:true});h.keydown('Escape',{altKey:true});
  h.keydown('Escape',{ctrlKey:true});h.keydown('Escape',{repeat:true});
  assert.deepEqual(h.clicks,[]);
});
