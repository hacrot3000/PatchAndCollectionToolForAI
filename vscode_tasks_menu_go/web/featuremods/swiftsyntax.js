// Lightweight, offline Swift lexer shared by the editor and File Compare.
// Not a Swift compiler: tokenization is visual only and must never change file contents.
const swiftKeywords=new Set((
  'actor any as associatedtype async await borrowing break case catch class consuming continue '+
  'convenience default defer deinit didSet distributed do dynamic else enum extension fallthrough false '+
  'fileprivate final for func get guard if import indirect infix init in inout internal is isolated '+
  'lazy left let mutating nil nonisolated none nonmutating open operator optional override package '+
  'postfix precedencegroup prefix private protocol public repeat required rethrows return right '+
  'self Self sending set some static struct subscript super switch throw throws true try typealias '+
  'unowned var weak where while willSet'
).split(/\s+/));
const swiftDocumentCache=new WeakMap();
function initialState(){return {blockDepth:0,quote:'',hashes:0};}
function tokenizeLine(line,previous=initialState()){
  const state={...previous},tokens=[];
  let i=0,mode='code';
  const add=(from,to,type)=>{if(to>from)tokens.push({from,to,type});};
  while(i<line.length){
    if(state.blockDepth){
      const from=i;
      while(i<line.length&&state.blockDepth){
        if(line.startsWith('/*',i)){state.blockDepth++;i+=2;}
        else if(line.startsWith('*/',i)){state.blockDepth--;i+=2;}
        else i++;
      }
      add(from,i,'comment');
      mode=state.blockDepth?'comment':'code';
      continue;
    }
    let opener=null;
    if(!state.quote){
      if(line.startsWith('//',i)){add(i,line.length,'comment');i=line.length;mode='comment';break;}
      if(line.startsWith('/*',i)){state.blockDepth=1;const from=i;i+=2;
        while(i<line.length&&state.blockDepth){
          if(line.startsWith('/*',i)){state.blockDepth++;i+=2;}
          else if(line.startsWith('*/',i)){state.blockDepth--;i+=2;}
          else i++;
        }
        add(from,i,'comment');mode=state.blockDepth?'comment':'code';continue;
      }
      // Standard, multiline and #raw Swift string literals.
      const match=line.slice(i).match(/^(#{0,8})("""|")/);
      if(match){
        opener=match[0];state.hashes=match[1].length;state.quote=match[2];
      }
    }
    if(state.quote){
      const from=i;
      if(opener)i+=opener.length;
      const terminator=state.quote+'#'.repeat(state.hashes);
      const escaped='\\'+'#'.repeat(state.hashes);
      let closed=false;
      while(i<line.length){
        // Swift interpolation can contain calls and nested string literals.
        // Keep its embedded quotes inside the outer string token.
        if(line.startsWith(escaped+'(',i)){
          i+=escaped.length+1;
          let depth=1,innerQuote='';
          while(i<line.length&&depth){
            const ch=line[i];
            if(innerQuote){
              if(ch==='\\'){i+=2;continue;}
              if(ch===innerQuote)innerQuote='';
              i++;continue;
            }
            if(ch==='"'||ch==="'"){innerQuote=ch;i++;continue;}
            if(ch==='(')depth++;
            else if(ch===')')depth--;
            i++;
          }
          continue;
        }
        if(line.startsWith(escaped,i)){
          i=Math.min(line.length,i+escaped.length+1);continue;
        }
        if(line.startsWith(terminator,i)){
          i+=terminator.length;closed=true;break;
        }
        i++;
      }
      add(from,i,'string');
      if(closed){state.quote='';state.hashes=0;mode='code';}
      else{
        mode='string';
        // Only triple-quoted literals may legally span lines.
        if(state.quote==='"'){state.quote='';state.hashes=0;}
      }
      continue;
    }
    const ch=line[i];
    if(ch.charCodeAt(0)===96){ // Swift escaped identifiers: \`class\`
      const from=i++;while(i<line.length&&line.charCodeAt(i)!==96)i++;
      if(i<line.length)i++;
      add(from,i,'variable');continue;
    }
    if(ch==='@'||ch==='#'){
      const match=line.slice(i).match(/^[@#][A-Za-z_][A-Za-z0-9_]*/);
      if(match){add(i,i+match[0].length,'command');i+=match[0].length;continue;}
    }
    if(/[0-9]/.test(ch)){
      const match=line.slice(i).match(/^(?:0[xX][\da-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:\d[\d_]*)(?:\.[\d_]+)?(?:[eE][+-]?[\d_]+)?)/);
      if(match){add(i,i+match[0].length,'number');i+=match[0].length;continue;}
    }
    if(/[A-Za-z_]/.test(ch)){
      const match=line.slice(i).match(/^[A-Za-z_][A-Za-z_0-9]*/);
      const word=match[0],to=i+word.length;
      if(swiftKeywords.has(word))add(i,to,'keyword');
      else if(/^[A-Z]/.test(word))add(i,to,'variable');
      i=to;continue;
    }
    i++;
  }
  if(state.blockDepth)mode='comment';
  else if(state.quote)mode='string';
  return {tokens,state,mode};
}
function tokenizeText(text){
  let state=initialState();
  return String(text??'').split(/\r?\n/).map(line=>{
    const result=tokenizeLine(line,state);
    state=result.state;
    return result.tokens;
  });
}
function ensureDocumentLines(doc,throughLine){
  let cache=swiftDocumentCache.get(doc);
  if(!cache){
    cache={tokens:[],states:[],state:initialState()};
    swiftDocumentCache.set(doc,cache);
  }
  const limit=Math.min(doc.lines,Math.max(0,throughLine));
  while(cache.tokens.length<limit){
    const result=tokenizeLine(doc.line(cache.tokens.length+1).text,cache.state);
    cache.tokens.push(result.tokens);
    cache.state=result.state;
    cache.states.push({...cache.state});
  }
  return cache;
}
function lexicalModeAt(doc,pos){
  const line=doc.lineAt(pos);
  const cache=ensureDocumentLines(doc,line.number-1);
  const state=line.number>1?cache.states[line.number-2]:initialState();
  return tokenizeLine(doc.sliceString(line.from,pos),state).mode;
}
globalThis.TaskMenuSwiftSyntax={tokenizeLine,tokenizeText,ensureDocumentLines,lexicalModeAt};
