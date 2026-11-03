// Regression tests for real File Editor + Edit Big File indentation handling.
// Run: node --test vscode_tasks_menu_go/test_editor_indentation.cjs
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/editor.js'),'utf8');

function section(start,end){
  const a=source.indexOf(start),b=source.indexOf(end,a+start.length);
  assert.ok(a>=0&&b>a,'Cannot find editor implementation: '+start);
  return source.slice(a,b);
}
const implementation=section('function editorLeadingIndent(text){','function editorCompletionMenuOpen(view){');
const context={
  editorLanguageDefinition:filename=>({id:({
    py:'python',nim:'nim',nims:'nim',go:'go',yaml:'yaml',yml:'yaml',js:'javascript'
  })[String(filename).split('.').pop()]||''}),
  editorReadOnly:view=>Boolean(view.readonly),
  TaskMenuLargeEditor:{stateFor:view=>view.large}
};
vm.createContext(context);
vm.runInContext(implementation+
  '\nthis.indentEditor=editorIndentSelection;this.indentStyle=editorIndentStyle;',context);
const indent=context.indentEditor;

function documentOf(sourceText){
  const text=String(sourceText),lines=[];let offset=0;
  for(const [index,content] of text.split('\n').entries()){
    lines.push({number:index+1,from:offset,to:offset+content.length,text:content});
    offset+=content.length+1;
  }
  return {
    length:text.length,lines:lines.length,
    line:n=>lines[n-1],
    lineAt:position=>{
      assert.ok(position>=0&&position<=text.length);
      for(let i=0;i<lines.length;i++)
        if(i===lines.length-1||position<lines[i+1].from)return lines[i];
      return lines[lines.length-1];
    }
  };
}
function editor(sourceText,filename='main.py',{anchor=0,head=anchor,streamed=false,loaded=true,readonly=false}={}){
  let text=sourceText,dispatched=[];
  let selection={anchor,head,from:Math.min(anchor,head),to:Math.max(anchor,head),empty:anchor===head};
  let state={doc:documentOf(text),selection:{main:selection}};
  const cm={
    get state(){return state;},
    dispatch(change){
      dispatched.push(change);
      const edits=Array.isArray(change.changes)?change.changes:[change.changes];
      if(change.changes)for(const edit of [...edits].reverse()){
        const to=edit.to??edit.from;
        text=text.slice(0,edit.from)+edit.insert+text.slice(to);
      }
      if(change.selection){
        const {anchor:a,head:h=a}=change.selection;
        selection={anchor:a,head:h,from:Math.min(a,h),to:Math.max(a,h),empty:a===h};
      }
      state={doc:documentOf(text),selection:{main:selection}};
    }
  };
  const view={closed:false,readonly,cm,file:{path:filename,streamed},
    large:streamed?{window:loaded?{}:null,loading:!loaded,saving:null,replacing:null}:null};
  return {view,content:()=>text,changes:()=>dispatched,selection:()=>selection};
}

test('real editor source parses and no longer replaces a selection with a single tab',()=>{
  assert.doesNotThrow(()=>new Function(source));
  assert.doesNotMatch(source,/replaceSelection\('\\t'\)/);
  assert.match(source,/editorIndentSelection\(view,event\.shiftKey\)/);
});

test('Tab indents an entire selected Python block in one undoable transaction',()=>{
  const text='def demo():\n    first()\n    second()\n  \nend()';
  const e=editor(text,'demo.py',{anchor:text.indexOf('    first'),head:text.indexOf('  \n')});
  assert.equal(indent(e.view),true);
  assert.equal(e.content(),'def demo():\n        first()\n        second()\n  \nend()');
  assert.equal(e.changes().length,1);
  assert.equal(e.changes()[0].changes.length,2);
  assert.equal(e.selection().empty,false);
});

test('selection ending at the start of the next line excludes that line',()=>{
  const text='def demo():\n    first()\n    second()';
  const e=editor(text,'demo.py',{anchor:text.indexOf('    first'),head:text.indexOf('    second')});
  indent(e.view);
  assert.equal(e.content(),'def demo():\n        first()\n    second()');
});

test('partial one-line selection indents the line without deleting the selection',()=>{
  const text='def demo():\n    call()';
  const e=editor(text,'demo.py',{anchor:text.indexOf('call'),head:text.indexOf('call')+4});
  indent(e.view);
  assert.equal(e.content(),'def demo():\n        call()');
  assert.equal(e.selection().to-e.selection().from,4);
});

test('Nim block indent detects two spaces after proc equals',()=>{
  const text='proc demo() =\n  let x = 1\n  echo x';
  const e=editor(text,'demo.nim',{anchor:text.indexOf('  let'),head:text.length});
  indent(e.view);
  assert.equal(e.content(),'proc demo() =\n    let x = 1\n    echo x');
});

test('Go block uses tabs without converting existing tab indentation',()=>{
  const text='func demo() {\n\tfoo()\n\tbar()\n}';
  const e=editor(text,'demo.go',{anchor:text.indexOf('\tfoo'),head:text.indexOf('}')});
  indent(e.view);
  assert.equal(e.content(),'func demo() {\n\t\tfoo()\n\t\tbar()\n}');
  indent(e.view,true);
  assert.equal(e.content(),text);
});

test('a stray tab elsewhere does not change a space-indented Python block',()=>{
  const text='def first():\n\tpass\n\n\ndef second():\n    first()\n    second()';
  const e=editor(text,'demo.py',{anchor:text.indexOf('    first'),head:text.length});
  indent(e.view);
  assert.equal(e.content(),'def first():\n\tpass\n\n\ndef second():\n        first()\n        second()');
});

test('Shift+Tab removes the inferred level from nested Python lines',()=>{
  const text='def demo():\n    if yes:\n        first()\n        second()';
  const e=editor(text,'demo.py',{anchor:text.indexOf('        first'),head:text.length});
  indent(e.view,true);
  assert.equal(e.content(),'def demo():\n    if yes:\n    first()\n    second()');
});

test('single cursor Tab matches local spaces and empty Nim files use safe default',()=>{
  const py='def demo():\n    call()';
  const p=editor(py,'demo.py',{anchor:py.indexOf('call')});
  indent(p.view);
  assert.equal(p.content(),'def demo():\n        call()');
  const n=editor('echo "hi"','new.nim',{anchor:0});
  indent(n.view);
  assert.equal(n.content(),'  echo "hi"');
});

test('backward selections remain backward after block indentation',()=>{
  const text='def demo():\n    first()\n    second()';
  const e=editor(text,'demo.py',{anchor:text.length,head:text.indexOf('    first')});
  indent(e.view);
  assert.equal(e.content(),'def demo():\n        first()\n        second()');
  assert.ok(e.selection().anchor>e.selection().head);
});

test('Edit Big File refuses writes until its window loads, then uses the same handler',()=>{
  const text='proc demo() =\n  echo "hi"';
  const e=editor(text,'demo.nim',{anchor:text.indexOf('  echo'),head:text.length,streamed:true,loaded:false});
  assert.equal(indent(e.view),false);
  assert.equal(e.content(),text);
  e.view.large.window={};e.view.large.loading=false;
  assert.equal(indent(e.view),true);
  assert.equal(e.content(),'proc demo() =\n    echo "hi"');
});

test('read-only editor refuses Tab/Shift+Tab without modifying contents',()=>{
  const e=editor('  echo 1','demo.nim',{anchor:0,head:8,readonly:true});
  assert.equal(indent(e.view),false);
  assert.equal(indent(e.view,true),false);
  assert.equal(e.content(),'  echo 1');
});
