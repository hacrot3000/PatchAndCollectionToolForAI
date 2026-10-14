const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const test=require('node:test');

function sourceCSS(filename){
  const js=fs.readFileSync(path.join(__dirname,'web/featuremods',filename),'utf8');
  const tick=String.fromCharCode(96);
  const start=js.indexOf('style.textContent='+tick);
  // Some feature modules append dialog-specific styles in a second
  // style.textContent += '...' statement. Extract the base template first,
  // rather than requiring the next line to append the style element.
  const end=js.indexOf(tick+';',start);
  assert.ok(start>=0&&end>start,filename+' contains embedded CSS');
  return js.slice(start+'style.textContent='.length+1,end);
}
const compare=sourceCSS('filecompare.js');
const git=sourceCSS('gitstatus.js');
function declaration(css,selector,property,light=false){
  const wanted=(light?'html[data-taskmenu-theme="light"] ':'')+selector;
  for(const match of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)){
    const sels=match[1].split(',').map(s=>s.trim());
    if(!sels.some(sel=>sel.endsWith(wanted)))continue;
    if(!light&&sels.some(sel=>sel.includes('html[data-taskmenu-theme="light"]')))continue;
    for(const part of match[2].split(';')){
      if(part.trim().startsWith(property+':')){
        return part.trim().slice(property.length+1).trim().replace(/!important$/,'');
      }
    }
  }
  assert.fail('Missing '+(light?'light':'dark')+' CSS '+selector+' {'+property+'}');
}
function luminance(color){
  assert.match(color,/^#[0-9a-f]{6}$/i);
  const bytes=[1,3,5].map(index=>parseInt(color.slice(index,index+2),16)/255);
  const linear=bytes.map(c=>c<=0.04045?c/12.92:((c+0.055)/1.055)**2.4);
  return 0.2126*linear[0]+0.7152*linear[1]+0.0722*linear[2];
}
function contrast(foreground,background){
  const a=luminance(foreground),b=luminance(background);
  return (Math.max(a,b)+0.05)/(Math.min(a,b)+0.05);
}
function readable(label,foreground,background){
  const value=contrast(foreground,background);
  assert.ok(value>=4.5,
    label+' must meet WCAG AA for small code text: '+foreground+' on '+background+' = '+value.toFixed(2)+':1');
}

const tokenClasses=['keyword','comment','string','number','command','variable'];
const lineKinds=['.removed.important','.added.important','.removed.unimportant','.added.unimportant'];
for(const theme of ['dark','light']){
  const light=theme==='light';
  test('File Compare '+theme+' theme: syntax tokens legible across diff line colors',()=>{
    const backgrounds=lineKinds.map(kind=>declaration(compare,'.file-compare-cell'+kind,'background',light));
    const base=declaration(compare,'.file-compare-cell','color',light);
    for(const background of backgrounds){
      readable(theme+' normal code',base,background);
      for(const token of tokenClasses){
        readable(theme+' '+token,
          declaration(compare,'.file-compare-syntax-'+token,'color',light),background);
      }
    }
  });
  test('File Compare '+theme+' theme: highlighted changed text stays readable',()=>{
    const foreground=declaration(compare,'.file-compare-inline-change','color',light);
    for(const side of ['removed','added']){
      const background=declaration(compare,'.file-compare-inline-change.'+side,'background',light);
      readable(theme+' changed text '+side,foreground,background);
    }
  });
  test('Git preview '+theme+' theme matches accessible File Compare palette',()=>{
    const code=declaration(git,'.git-diff-cell','color',light);
    for(const kind of lineKinds){
      const gitBg=declaration(git,'.git-diff-cell'+kind,'background',light);
      const compareBg=declaration(compare,'.file-compare-cell'+kind,'background',light);
      assert.equal(gitBg,compareBg,'Git '+kind+' background matches File Compare');
      readable(theme+' Git diff text '+kind,code,gitBg);
    }
  });
}
test('Source text is rendered via textContent instead of interpolation',()=>{
  const js=fs.readFileSync(path.join(__dirname,'web/featuremods/filecompare.js'),'utf8');
  assert.match(js,/span\.textContent=text\.slice\(from,to\)/);
  assert.match(js,/span\.classList\.add\('file-compare-inline-change',changeKind\)/);
});
