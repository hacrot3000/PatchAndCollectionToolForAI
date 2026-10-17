import assert from 'node:assert/strict';
import test from 'node:test';
await import('../web/featuremods/swiftsyntax.js');

const lexer=globalThis.TaskMenuSwiftSyntax;
const typed=(line,needle,tokens)=>tokens.find(token=>line.slice(token.from,token.to).includes(needle))?.type;

function makeDoc(source){
  const lines=source.split('\n');
  const starts=[];let offset=0;
  for(const line of lines){starts.push(offset);offset+=line.length+1;}
  return {
    lines:lines.length,
    line(number){return {number,text:lines[number-1],from:starts[number-1],to:starts[number-1]+lines[number-1].length};},
    lineAt(pos){
      let number=1;
      while(number<lines.length&&starts[number]<=pos)number++;
      return this.line(number);
    },
    sliceString(from,to){return source.slice(from,to);}
  };
}

test('Swift keywords, types, property wrappers, compiler directives and escaped identifiers',()=>{
  const line='@MainActor struct ContentView: View { let ' + String.fromCharCode(96)+'class'+String.fromCharCode(96)+' = 0xFF } #if DEBUG';
  const tokens=lexer.tokenizeLine(line).tokens;
  for(const word of ['struct','let'])assert.equal(typed(line,word,tokens),'keyword');
  assert.equal(typed(line,'ContentView',tokens),'variable');
  assert.equal(typed(line,'@MainActor',tokens),'command');
  assert.equal(typed(line,'#if',tokens),'command');
  assert.equal(typed(line,'0xFF',tokens),'number');
  assert.equal(typed(line,'class'+String.fromCharCode(96),tokens),'variable');
});

test('Swift URL and comment markers inside strings stay strings',()=>{
  const line='let url = "https://example.test/a/*path*/" // final';
  const tokens=lexer.tokenizeLine(line).tokens;
  assert.equal(typed(line,'https://',tokens),'string');
  assert.equal(typed(line,'// final',tokens),'comment');
  assert.equal(tokens.filter(token=>token.type==='comment').length,1);
});

test('Swift interpolation with nested string quotes remains one string token',()=>{
  const line='let message = "\:(format("Hello"))" // completed'.replace(':(','\\(');
  const tokens=lexer.tokenizeLine(line).tokens;
  assert.equal(typed(line,'Hello',tokens),'string');
  assert.equal(typed(line,'completed',tokens),'comment');
  assert.equal(tokens.filter(token=>token.type==='string').length,1);
});

test('Swift block comments nest and retain state across lines',()=>{
  const lines=['/* outer','  /* inner */ still outer','*/ let ready = true'];
  const tokens=lexer.tokenizeText(lines.join('\n'));
  assert.equal(typed(lines[1],'still outer',tokens[1]),'comment');
  assert.equal(typed(lines[2],'*/',tokens[2]),'comment');
  assert.equal(typed(lines[2],'let',tokens[2]),'keyword');
  const open=lexer.tokenizeLine('/* outer');
  assert.equal(open.state.blockDepth,1);
  assert.equal(lexer.tokenizeLine('/* nested',open.state).state.blockDepth,2);
});

test('Raw strings and multiline Swift strings do not create fake comments',()=>{
  const lines=['let raw = #"// not a comment /* ok */"#','let multiline = """','hello // from a string','""" // actual comment'];
  const tokens=lexer.tokenizeText(lines.join('\n'));
  assert.equal(typed(lines[0],'// not a comment',tokens[0]),'string');
  assert.equal(typed(lines[2],'hello //',tokens[2]),'string');
  assert.equal(typed(lines[3],'actual comment',tokens[3]),'comment');
});

test('Lexical mode and document cache handle cursor positions and replaced documents',()=>{
  const source=['/* first','second */ let x = "hello" // end','let result = true'].join('\n');
  const doc=makeDoc(source);
  assert.equal(lexer.lexicalModeAt(doc,source.indexOf('first')+2),'comment');
  assert.equal(lexer.lexicalModeAt(doc,source.indexOf('hello')+3),'string');
  assert.equal(lexer.lexicalModeAt(doc,source.indexOf('end')+2),'comment');
  assert.equal(lexer.lexicalModeAt(doc,source.indexOf('result')+3),'code');
  assert.equal(lexer.ensureDocumentLines(doc,2).tokens.length,2);
  const changed=makeDoc('let x = 4');
  assert.equal(typed(changed.line(1).text,'4',lexer.ensureDocumentLines(changed,1).tokens[0]),'number');
});
