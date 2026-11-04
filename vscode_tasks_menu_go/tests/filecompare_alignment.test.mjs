import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

// Exercise the actual browser diff engine without a DOM. The build/model
// algorithm is intentionally pure and shared by Git, File, Directory Compare.
const filename=path.join(path.dirname(fileURLToPath(import.meta.url)),'../web/featuremods/filecompare.js');
const source=fs.readFileSync(filename,'utf8');
const start=source.indexOf('function splitLines(text){');
const end=source.indexOf('// Reuse the SAME language-aware row criteria',start);
assert.ok(start>=0&&end>start,'diff engine markers must remain discoverable');
const engine=Function(source.slice(start,end)+'\nreturn {diffOperations,buildCompareModel,replaceLineRange};')();
const {diffOperations,buildCompareModel,replaceLineRange}=engine;

function check(left,right,label){
  const a=left.join('\n'),b=right.join('\n');
  const ops=diffOperations(a,b),model=buildCompareModel(a,b);
  assert.deepEqual(ops.filter(op=>op.kind!=='add').map(op=>op.text),left,label+' left projection');
  assert.deepEqual(ops.filter(op=>op.kind!=='remove').map(op=>op.text),right,label+' right projection');
  assert.equal(model.identical,JSON.stringify(left)===JSON.stringify(right),label+' identical flag');
  // Hunk copy uses absolute left/right offsets. Apply in reverse so earlier
  // replacements cannot be shifted by a later insertion/deletion.
  let forward=a,backward=b;
  for(const hunk of [...model.hunks].reverse()){
    forward=replaceLineRange(forward,hunk.leftStart,hunk.leftCount,hunk.rightLines);
    backward=replaceLineRange(backward,hunk.rightStart,hunk.rightCount,hunk.leftLines);
  }
  assert.equal(forward,b,label+' Left -> Right hunk copy');
  assert.equal(backward,a,label+' Right -> Left hunk copy');
  const changed=ops.filter(op=>op.kind!=='equal').length;
  return {changed,hunks:model.hunks.length,matched:ops.length-changed,model};
}

test('single inserted line and separate edit in a 2400-line Java file remain two hunks',()=>{
  // Previously 2400x2400 exceeded the 1,200,000-cell LCS limit and became
  // one huge change block with >2000 false-positive modified lines.
  const left=Array.from({length:2400},(_,i)=>'  public void callback'+i+'() { return '+i+'; }');
  const right=[...left];
  right.splice(58,0,'  import com.du.dtc.bike.companion.BikeCompanionManager;');
  right[2110]='  public void callback2109() { return 99999; }';
  const result=check(left,right,'one-line shift');
  assert.equal(result.hunks,2);
  assert.equal(result.changed,3);
  assert.equal(result.matched,2399);
  assert.ok(result.model.rows.filter(row=>row.hunk<0).length>=2399);
});

test('repeated syntax lines do not cause cascading false changes',()=>{
  const left=Array.from({length:750},(_,i)=>[
    '    if (ready) {',
    '        run('+i+');',
    '        return;',
    '    }',
    '}'
  ]).flat();
  const right=[...left];
  right.splice(22,0,'        // newly inserted comment');
  right.splice(2950,1);
  const result=check(left,right,'repetitive Java braces');
  assert.equal(result.changed,2);
  assert.equal(result.hunks,2);
});

test('bounded Myers realigns even with no unique lines',()=>{
  const left=Array.from({length:3000},(_,i)=>['if (ready) {','  next();','}'][i%3]);
  const right=[...left];
  right.splice(20,0,'  log();');
  right[right.length-2]='  finish();';
  const result=check(left,right,'no unique lines');
  assert.equal(result.changed,3);
  assert.equal(result.hunks,2);
});

test('trailing newline, equal files, prefix and suffix are preserved',()=>{
  assert.equal(check(['a','b',''],['a','b',''],'same newline').hunks,0);
  assert.equal(check(['head','old','tail',''],['head','new','tail',''],'single replacement').changed,2);
  assert.equal(check(['a','a','b','b',''],['a','a','b','b','new',''],'trailing insertion').changed,1);
});

test('line moves and ambiguous repeated blocks preserve both original files',()=>{
  const common=Array.from({length:620},(_,i)=>[
    'public static final int ACTION_'+i+' = '+i+';',
    '  if (ready) {',
    '    return;',
    '  }'
  ]).flat();
  const right=[...common.slice(0,450),...common.slice(600,760),...common.slice(450,600),...common.slice(760)];
  const result=check(common,right,'block moved');
  assert.ok(result.matched>1500,'unaltered blocks should stay matched');
});

test('large unrelated files avoid quadratic LCS allocation',()=>{
  const left=Array.from({length:2400},(_,i)=>'old:'+i);
  const right=Array.from({length:2400},(_,i)=>'new:'+i);
  const result=check(left,right,'unrelated');
  assert.equal(result.matched,0);
  assert.equal(result.changed,4800);
});

test('seeded insert/delete/replace fuzz retains exact hunk copy offsets',()=>{
  let state=0x12345678;
  const next=()=>{state^=state<<13;state^=state>>>17;state^=state<<5;return state>>>0;};
  for(let run=0;run<40;run++){
    const left=Array.from({length:600+(run%3)*520},(_,i)=>i%5===0?'    }':i%7===0?'': 'symbol_'+(i%70));
    const right=left.slice();
    for(let j=0;j<5;j++){
      const at=next()%right.length;
      if(j%3===0)right.splice(at,0,'added_'+run+'_'+j);
      else if(j%3===1)right.splice(at,1);
      else right[at]='replacement_'+run+'_'+j;
    }
    check(left,right,'random run '+run);
  }
});
