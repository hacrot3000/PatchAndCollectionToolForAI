const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const directory=path.join(__dirname,'web','featuremods');
const editor=fs.readFileSync(path.join(directory,'editor.js'),'utf8');
const projectActions=fs.readFileSync(path.join(directory,'projectfileactions.js'),'utf8');
const explorer=fs.readFileSync(path.join(directory,'explorer.js'),'utf8');

function editorHelpers(preview){
 const begin=editor.indexOf('function editorIsMarkdown(pathValue){');
 const end=editor.indexOf('function editorLanguageDefinition(pathValue){',begin);
 assert.ok(begin>=0&&end>begin,'production editor Markdown helpers exist');
 return new Function('editorLanguagePathInfo','globalThis','Blob',
   editor.slice(begin,end)+'\nreturn {editorIsMarkdown,openEditorMarkdownPreview,syncEditorMarkdownPreview};'
 )(
  value=>({ext:String(value||'').split('/').pop().split('.').pop().toLowerCase()}),
  {TaskMenuMarkdownPreview:preview},Blob
 );
}
function projectContextActions(openMarkdown){
 const begin=projectActions.indexOf("function standardActions(pathValue,type='file'){");
 const end=projectActions.indexOf('\nfunction appendAction(',begin);
 assert.ok(begin>=0&&end>begin,'production project context actions exist');
 return new Function('cleanPath','projectArchiveKind','projectCompareActions','openProjectMarkdownPreview','globalThis',
   projectActions.slice(begin,end)+'\nreturn standardActions;'
 )(value=>String(value),()=>null,()=>[],openMarkdown,{TaskMenuAddons:{contextMenuActions:()=>[]}});
}
test('Markdown editor button uses unsaved CodeMirror text without reloading the file',()=>{
 const calls=[];
 const helpers=editorHelpers({open:info=>calls.push(info)});
 const view={
  closed:false,file:{path:'docs/README.MD',content:'# Old file from disk'},
  cm:{state:{doc:{toString:()=> '# Current unsaved text\n\n**bold**'}}},
  markdownPreview:{hidden:true}
 };
 assert.equal(helpers.openEditorMarkdownPreview(view),true);
 assert.equal(calls.length,1);
 assert.equal(calls[0].kind,'markdown');
 assert.equal(calls[0].path,'docs/README.MD');
 assert.equal(calls[0].content,'# Current unsaved text\n\n**bold**');
 assert.equal(calls[0].size,new Blob([calls[0].content]).size);
 helpers.syncEditorMarkdownPreview(view);
 assert.equal(view.markdownPreview.hidden,false);
 view.file.path='docs/file.txt';
 helpers.syncEditorMarkdownPreview(view);
 assert.equal(view.markdownPreview.hidden,true);
 assert.equal(helpers.openEditorMarkdownPreview(view),false);
 assert.equal(calls.length,1);
});
test('Markdown toolbar supports known extensions and excludes unrelated files',()=>{
 const helpers=editorHelpers({open(){}});
 for(const name of ['README.md','FILE.MD','notes.markdown','notes.MkD','notes.mdown','notes.MKDN'])
  assert.equal(helpers.editorIsMarkdown(name),true,name);
 for(const name of ['README.mdx','README.md.bak','source.js','README'])
  assert.equal(helpers.editorIsMarkdown(name),false,name);
 assert.ok(editor.includes('save,autoSave,reload,markdownPreview,find,outline'));
 assert.ok(editor.includes('markdownPreview.onclick=()=>'));
 assert.ok(editor.includes('syncEditorMarkdownPreview(view);'));
});
test('Explorer right-click action previews supported Markdown files only',async()=>{
 const opened=[];
 const actions=projectContextActions(async path=>opened.push(path));
 for(const name of ['README.md','DOC.MD','docs/plan.markdown','docs/test.mkd','docs/notes.mdown','docs/notes.mkdn']){
  const menu=actions(name,'file');
  const preview=menu.filter(item=>item.label==='Preview Markdown');
  assert.equal(preview.length,1,name);
  assert.equal(menu.findIndex(item=>item.label==='Preview Markdown'),1,
    'Preview must appear directly after Open');
  await preview[0].run();
 }
 assert.deepEqual(opened,['README.md','DOC.MD','docs/plan.markdown','docs/test.mkd','docs/notes.mdown','docs/notes.mkdn']);
 for(const name of ['readme.txt','README.md.bak','archive.zip']){
  const menu=actions(name,'file');
  assert.equal(menu.some(item=>item.label==='Preview Markdown'),false,name);
 }
 assert.equal(actions('docs','dir').some(item=>item.label==='Preview Markdown'),false);
 assert.ok(explorer.includes('appendSharedProjectActions(pathValue,type)'));
 assert.ok(explorer.includes('TaskMenuProjectFileActions?.standardActions?.(pathValue,type)'));
});

test('Source editors initialize without Markdown preview and dialog code does not own its button',()=>{
 const createBegin=editor.indexOf('function createEditor(file){');
 const toolbarEnd=editor.indexOf('  head.append(pathNode,',createBegin);
 assert.ok(createBegin>=0&&toolbarEnd>createBegin,'createEditor toolbar setup exists');
 const setup=editor.slice(createBegin,toolbarEnd);
 assert.match(setup,/const markdownPreview=document\.createElement\('button'\)/,
  'preview button must be declared in createEditor before toolbar references it');
 assert.match(setup,/markdownPreview\.hidden=!editorIsMarkdown\(file\.path\)/,
  'preview button must be hidden for non-Markdown files');
 const conflictBegin=editor.indexOf("dialog.className='editor-dirty-dialog editor-conflict-dialog'");
 assert.ok(conflictBegin>=0&&conflictBegin<createBegin,'save-conflict dialog exists');
 assert.equal(editor.slice(conflictBegin,createBegin).includes('const markdownPreview='),false,
  'save-conflict dialog must not initialize an unrelated preview button');
 const helpers=editorHelpers({open(){throw new Error('preview must not run for source files');}});
 for(const pathValue of ['code/main.nim','src/main.go','src/utils.lua','script.js','main.ts',
  'app.c','app.cpp','module.py','readme.txt','README.md.bak']){
  assert.equal(helpers.editorIsMarkdown(pathValue),false,pathValue);
  const view={closed:false,file:{path:pathValue},markdownPreview:{hidden:false}};
  helpers.syncEditorMarkdownPreview(view);
  assert.equal(view.markdownPreview.hidden,true,pathValue);
  assert.equal(helpers.openEditorMarkdownPreview(view),false,pathValue);
 }
});
