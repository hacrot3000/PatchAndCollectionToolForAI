// Regression for File Editor: repeated Enter in Find must never move keyboard
// focus into CodeMirror (which has the selected match ready to be replaced).
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/editor.js'),'utf8');
function section(start,end){
 const a=source.indexOf(start);
 const b=source.indexOf(end,a+start.length);
 assert.ok(a>=0&&b>a,'Could not find live editor implementation: '+start);
 return source.slice(a,b);
}
// Execute the real implementation rather than an independently copied model.
const findImplementation=section('function editorFindComparable(', 'function editorSelectionMatchesFind(');
const inputKeydown=section('findInput.onkeydown=event=>{','findReplaceInput.onkeydown=event=>{');

function harness({content='alpha beta alpha gamma alpha',query='alpha',selection={from:0,to:0}}={}){
 const values={text:content,focus:'find',editorFocuses:0,highlightUpdates:0,dispatches:[],closed:false};
 const findInput={value:query,focus(){values.focus='find';}};
 const cm={
  state:{
   doc:{toString:()=>values.text,lineAt:index=>({number:values.text.slice(0,index).split('\n').length})},
   selection:{main:{...selection}}
  },
  dispatch(transaction){
   values.dispatches.push(transaction);
   if(transaction.selection){
    const {anchor,head}=transaction.selection;
    cm.state.selection.main={from:Math.min(anchor,head),to:Math.max(anchor,head)};
   }
  },
  focus(){values.focus='editor';values.editorFocuses++;}
 };
 const view={closed:false,findInput,findCase:{checked:false},findStatus:{textContent:''},
  findHighlightState:{activeFrom:-1,activeTo:-1},cm};
 const context={view,findInput,values,
  syncEditorFindHighlights(){values.highlightUpdates++;},
  closeEditorFind(){values.closed=true;values.focus='editor';}};
 vm.createContext(context);
 vm.runInContext(findImplementation+'\n'+inputKeydown,context);
 function press(key='Enter',{shift=false}={}){
  let prevent=0;
  const event={key,shiftKey:shift,preventDefault(){prevent++;}};
  findInput.onkeydown(event);
  return {prevent,selection:{...view.cm.state.selection.main}};
 }
 return {values,view,press,findInput};
}
test('Enter repeatedly advances, wraps and preserves Find focus and file contents',()=>{
 const h=harness();
 const positions=[{from:0,to:5},{from:11,to:16},{from:23,to:28},{from:0,to:5}];
 for(const [i,want] of positions.entries()){
  const event=h.press();
  assert.equal(event.prevent,1,'Enter '+(i+1)+' must be handled by search input');
  assert.deepEqual(event.selection,want);
  assert.equal(h.values.focus,'find','Enter '+(i+1)+' must not move focus to selected source');
  assert.equal(h.values.editorFocuses,0);
  assert.equal(h.values.text,'alpha beta alpha gamma alpha','search must not edit file');
  assert.equal(h.findInput.value,'alpha');
 }
});
test('Shift+Enter searches backward, wraps and keeps focus in Find',()=>{
 const h=harness();
 const first=h.press('Enter',{shift:true});
 assert.deepEqual(first.selection,{from:23,to:28});
 const second=h.press('Enter',{shift:true});
 assert.deepEqual(second.selection,{from:11,to:16});
 assert.equal(h.values.focus,'find');
 assert.equal(h.values.editorFocuses,0);
});
test('Not-found queries leave Find focused without modifying CodeMirror',()=>{
 const h=harness({query:'missing'});
 const event=h.press();
 assert.equal(event.prevent,1);
 assert.equal(h.view.findStatus.textContent,'No matches');
 assert.equal(h.values.focus,'find');
 assert.equal(h.values.editorFocuses,0);
 assert.equal(h.values.text,'alpha beta alpha gamma alpha');
});
test('Find navigation still scrolls/highlights matching source, not editor focus',()=>{
 const h=harness();
 const result=h.press();
 assert.deepEqual(result.selection,{from:0,to:5});
 assert.ok(h.values.dispatches.some(x=>x.scrollIntoView===true));
 assert.equal(h.view.findHighlightState.activeFrom,0);
 assert.equal(h.view.findHighlightState.activeTo,5);
 assert.equal(h.values.focus,'find');
});
test('Escape closes find deliberately, distinct from Enter navigation',()=>{
 const h=harness();
 const result=h.press('Escape');
 assert.equal(result.prevent,1);
 assert.equal(h.values.closed,true);
 assert.equal(h.values.focus,'editor');
});
test('Find matches with Match case off but keeps query input focus',()=>{
 const h=harness({content:'Alpha and ALPHA and alpha'});
 h.press();
 assert.deepEqual(h.view.cm.state.selection.main,{from:0,to:5});
 h.press();
 assert.deepEqual(h.view.cm.state.selection.main,{from:10,to:15});
 assert.equal(h.values.focus,'find');
});
