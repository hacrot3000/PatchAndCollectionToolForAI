// Ctrl+/ regression tests for TaskDeck's real Edit File / Edit Big File handler.
// Run: node --test vscode_tasks_menu_go/test_editor_comment_toggle.cjs
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/editor.js'),'utf8');
function slice(start,end){
  const a=source.indexOf(start),b=source.indexOf(end,a+start.length);
  assert.ok(a>=0&&b>a,'Missing live editor section: '+start);
  return source.slice(a,b);
}
const impl=slice('function editorCommentSyntax(pathValue,','function editorCompletionMenuOpen(view){');
const languageFor=filename=>{
  const lower=String(filename||'').toLowerCase(),name=lower.split('/').pop();
  const ext=name.includes('.')?name.split('.').pop():'';
  const ids={
    py:'python',nim:'nim',nims:'nim',nimble:'nim',go:'go',js:'javascript',jsx:'jsx',
    ts:'typescript',tsx:'tsx',swift:'swift',lua:'lua',sql:'sql',sh:'shell',
    bash:'shell',yml:'yaml',yaml:'yaml',css:'css',scss:'scss',html:'html',
    htm:'html',xml:'xml',md:'markdown',vue:'vue',php:'php',ini:'config',
    cfg:'config',properties:'config',json:'json',json5:'json',jsonc:'json',
    csv:'csv',svg:'xml',strings:'iosstrings'
  };
  return {id:ids[ext]||(['.env','.editorconfig'].includes(name)?'config':'')};
};
const context={
  editorLanguageDefinition:languageFor,
  editorLanguagePathInfo:filename=>{
    const name=String(filename||'').toLowerCase().split('/').pop();
    return {name,ext:name.includes('.')?name.split('.').pop():''};
  },
  editorLeadingIndent:text=>String(text||'').match(/^[ \t]*/)?.[0]||'',
  editorReadOnly:view=>!!view.readonly,
  editorIndentMapPos:(changes,pos,assoc)=>{
    let delta=0;
    for(const change of changes){
      const end=change.to??change.from;
      if(pos<change.from)break;
      if(pos<=end){
        if(change.from===end)return pos+delta+(assoc<0?0:change.insert.length);
        return change.from+delta+(assoc<0?0:change.insert.length);
      }
      delta+=change.insert.length-(end-change.from);
    }
    return pos+delta;
  },
  TaskMenuLargeEditor:{stateFor:view=>view.large}
};
vm.createContext(context);
vm.runInContext(impl+'\nthis.toggle=editorToggleComments;this.syntax=editorCommentSyntax;',context);

function docOf(text){
  let offset=0;
  const lines=String(text).split('\n').map((value,index)=>{
    const row={number:index+1,from:offset,to:offset+value.length,text:value};
    offset+=value.length+1;return row;
  });
  return {
    length:text.length,lines:lines.length,
    line:n=>lines[n-1],
    lineAt:position=>{
      for(let i=0;i<lines.length;i++)
        if(i===lines.length-1||position<lines[i+1].from)return lines[i];
      throw Error('Invalid position');
    },
    sliceString:(from,to)=>text.slice(from,to)
  };
}
function editor(text,filename='x.py',{anchor=0,head=anchor,readonly=false,streamed=false,loaded=true,saving=false,replacing=false}={}){
  let sourceText=text;
  let selection={anchor,head,from:Math.min(anchor,head),to:Math.max(anchor,head),empty:anchor===head};
  const dispatches=[];
  const cm={
    get state(){return {doc:docOf(sourceText),selection:{main:selection}};},
    dispatch(tx){
      dispatches.push(tx);
      const changes=Array.isArray(tx.changes)?tx.changes:[tx.changes];
      for(const change of changes.slice().reverse()){
        sourceText=sourceText.slice(0,change.from)+change.insert+
          sourceText.slice(change.to??change.from);
      }
      if(tx.selection){
        const {anchor,head}=tx.selection;
        selection={anchor,head,from:Math.min(anchor,head),
          to:Math.max(anchor,head),empty:anchor===head};
      }
    }
  };
  const view={file:{path:filename,streamed},cm,closed:false,readonly,
    large:streamed?{window:loaded?{}:null,loading:!loaded,saving,replacing}:null};
  return {
    view,
    text:()=>sourceText,
    selection:()=>({...selection}),
    dispatches,
    toggle:()=>context.toggle(view)
  };
}
function selected(text,filename,from=0,to=text.length,extra={}){
  return editor(text,filename,{anchor:from,head:to,...extra});
}

test('real source installs Ctrl+/ and Cmd+/ on the editor keyboard handler',()=>{
  assert.doesNotThrow(()=>new Function(source));
  assert.match(source,/event\.key==='\/'\|\|event\.code==='Slash'/);
  assert.match(source,/shortcut&&!event\.shiftKey/);
  assert.match(source,/if\(!event\.repeat\)editorToggleComments\(view\)/);
});

test('mixed Python block comments every nonblank line one level, then reverts',()=>{
  const text='def test():\n    # existing\n    do_it()\n\n    # another';
  const e=selected(text,'test.py',text.indexOf('    # existing'));
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),'def test():\n    # # existing\n    # do_it()\n\n    # # another');
  assert.equal(e.dispatches.length,1,'one Undo transaction for the entire block');
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),text);
});

test('all-commented Nim block removes exactly one leading marker and optional space',()=>{
  const text='proc test() =\n  ## doc\n  # echo 1\n  #echo 2';
  const e=selected(text,'test.nim',text.indexOf('  ##'));
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),'proc test() =\n  # doc\n  echo 1\n  echo 2');
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),'proc test() =\n  # # doc\n  # echo 1\n  # echo 2');
});

test('Go tabs and trailing spaces stay unmodified on comment toggle round trip',()=>{
  const text='func main() {\n\tfoo()  \n\tbar()\n}';
  const e=selected(text,'main.go',text.indexOf('\tfoo'),text.indexOf('}'));
  e.toggle();
  assert.equal(e.text(),'func main() {\n\t// foo()  \n\t// bar()\n}');
  e.toggle();
  assert.equal(e.text(),text);
});

test('selected trailing newline does not comment the following line',()=>{
  const text='a()\nb()\nc()';
  const e=selected(text,'main.js',0,text.indexOf('b()'));
  e.toggle();
  assert.equal(e.text(),'// a()\nb()\nc()');
});

test('partial selected token comments the entire line and keeps a nonempty selection',()=>{
  const text='    run(task)';
  const e=selected(text,'main.py',text.indexOf('run'),text.indexOf('run')+3);
  e.toggle();
  assert.equal(e.text(),'    # run(task)');
  assert.equal(e.selection().to-e.selection().from,3);
  e.toggle();
  assert.equal(e.text(),text);
});

test('backward selection stays backward after Ctrl+/',()=>{
  const text='foo()\nbar()';
  const e=selected(text,'main.ts',0,text.length,{anchor:text.length,head:0});
  e.toggle();
  assert.equal(e.text(),'// foo()\n// bar()');
  assert.ok(e.selection().anchor>e.selection().head);
  e.toggle();
  assert.equal(e.text(),text);
});

test('cursor only comments or uncomments the current source line',()=>{
  const text='    hello()\n    world()';
  const e=selected(text,'main.py',text.indexOf('hello')+2,text.indexOf('hello')+2);
  e.toggle();
  assert.equal(e.text(),'    # hello()\n    world()');
  e.toggle();
  assert.equal(e.text(),text);
});

test('all-blank selection leaves existing spaces and tabs untouched',()=>{
  const text='  \n\t\n';
  const e=selected(text,'main.py');
  assert.equal(e.toggle(),false);
  assert.equal(e.text(),text);
  assert.equal(e.dispatches.length,0);
});

test('SQL/Lua use -- with a separating space; INI uses semicolon',()=>{
  for(const filename of ['db.sql','main.lua']){
    const e=selected('SELECT x\n-- existing',filename);
    e.toggle();assert.equal(e.text(),'-- SELECT x\n-- -- existing');
    e.toggle();assert.equal(e.text(),'SELECT x\n-- existing');
  }
  const ini=selected('port=8000\n; flag','settings.ini');
  ini.toggle();assert.equal(ini.text(),'; port=8000\n; ; flag');
  ini.toggle();assert.equal(ini.text(),'port=8000\n; flag');
});

test('strict JSON and data formats are not corrupted by injected comments',()=>{
  for(const filename of ['data.json','scene.prefab','table.csv']){
    const e=selected('{"name":"ok"}',filename);
    assert.equal(e.toggle(),false);
    assert.equal(e.text(),'{"name":"ok"}');
  }
});

test('JSONC and JSON5 allow slash comments',()=>{
  for(const filename of ['settings.jsonc','settings.json5']){
    const e=selected('{"ok":true}',filename);
    e.toggle();assert.equal(e.text(),'// {"ok":true}');
    e.toggle();assert.equal(e.text(),'{"ok":true}');
  }
});

test('CSS wraps entire selected block with /* */ then removes the wrapping',()=>{
  const text='body {\n  color: red;\n}';
  const e=selected(text,'main.css');
  e.toggle();assert.equal(e.text(),'/* body {\n  color: red;\n} */');
  e.toggle();assert.equal(e.text(),text);
});

test('HTML and Markdown wrap an entire block in HTML comments',()=>{
  for(const filename of ['page.html','README.md']){
    const text='<div>\n  title\n</div>';
    const e=selected(text,filename);
    e.toggle();assert.equal(e.text(),'<!-- <div>\n  title\n</div> -->');
    e.toggle();assert.equal(e.text(),text);
  }
});

test('CSS/HTML preserve existing independent block comments instead of nesting',()=>{
  const tests=[
    {filename:'style.css',text:'/* old */\nnew {}',expected:'/* old */\n/* new {} */'},
    {filename:'page.html',text:'<!-- old -->\n<div></div>',expected:'<!-- old -->\n<!-- <div></div> -->'}
  ];
  for(const {filename,text,expected} of tests){
    const e=selected(text,filename);
    e.toggle();assert.equal(e.text(),expected);
  }
});

test('multi-line XML comments can be uncommented without losing internal newlines',()=>{
  const text='<!--\n  <entry/>\n-->';
  const e=selected(text,'file.xml');
  e.toggle();assert.equal(e.text(),'\n  <entry/>\n');
  e.toggle();assert.equal(e.text(),'<!-- \n  <entry/>\n -->');
});

test('HTML and Vue script sections use //; style sections use /* */',()=>{
  for(const filename of ['page.html','component.vue']){
    const text='<script>\n  foo()\n  bar()\n</script>\n<style>\n  body{}\n</style>';
    const first=selected(text,filename,text.indexOf('  foo()'),text.indexOf('</script>'));
    first.toggle();
    assert.equal(first.text(),'<script>\n  // foo()\n  // bar()\n</script>\n<style>\n  body{}\n</style>');
    const style=selected(text,filename,text.indexOf('  body{}'),text.indexOf('</style>'));
    style.toggle();
    assert.equal(style.text(),'<script>\n  foo()\n  bar()\n</script>\n<style>\n  /* body{} */\n</style>');
  }
});

test('HTML/Vue mixed section boundary is rejected rather than guessing',()=>{
  const text='<script>\nfoo()\n</script>\n<div>Hi</div>';
  const e=selected(text,'component.vue');
  assert.equal(e.toggle(),false);
  assert.equal(e.text(),text);
});

test('read-only, loading, saving and replacing windows refuse edits',()=>{
  for(const options of [{readonly:true},{streamed:true,loaded:false},
    {streamed:true,saving:true},{streamed:true,replacing:true}]){
    const e=selected('echo x','main.nim',0,6,options);
    assert.equal(e.toggle(),false);
    assert.equal(e.text(),'echo x');
    assert.equal(e.dispatches.length,0);
  }
});

test('Edit Big File uses exactly the same implementation after its window loads',()=>{
  const text='proc foo() =\n  echo 1\n  echo 2';
  const e=selected(text,'main.nim',text.indexOf('  echo'),text.length,{streamed:true});
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),'proc foo() =\n  # echo 1\n  # echo 2');
  assert.equal(e.toggle(),true);
  assert.equal(e.text(),text);
});

test('CRLF-style in-editor text is not wholesale rewritten by toggling',()=>{
  const text='a()\r\nb()\r\n';
  const e=selected(text,'main.js',0,4);
  e.toggle();
  assert.equal(e.text(),'// a()\r\nb()\r\n');
  e.toggle();
  assert.equal(e.text(),text);
});
