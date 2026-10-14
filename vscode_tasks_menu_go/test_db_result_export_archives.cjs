const fs=require('node:fs');
const path=require('node:path');
const assert=require('node:assert/strict');
const test=require('node:test');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/database.js'),'utf8');
const start=source.indexOf('function queryExportContexts(view){');
const end=source.indexOf('\nasync function exportQueryData(',start);
assert.ok(start>=0&&end>start,'database.js exports archive helpers');
const sourceHelpers=source.slice(start,end);
function harness(){
  const uploads=[],downloads=[],locations=[],calls=[];
  const owner={queryResultContexts:[]};
  const fakeGlobal={
    TaskMenuDatabaseWorkbench:{
      serializeClipboardData(format,columns,rows,headers){
        if(format==='json')return JSON.stringify(rows.map(row=>Object.fromEntries(columns.map((key,i)=>[key,row[i]??null]))));
        const encode=values=>values.map(value=>String(value??'')).join(format==='csv'?',':'\t');
        return (headers?[encode(columns)]:[]).concat(rows.map(encode)).join('\n');
      }
    },
    TaskMenuDirectoryBrowser:{
      async chooseFile(options){calls.push(options);return {dir:'exports',name:'batch.zip'};}
    }
  };
  const document={
    body:{append(node){node.attached=true;}},
    createElement(){return {
      href:'',download:'',rel:'',
      click(){downloads.push(this.download);},
      remove(){this.attached=false;}
    };}
  };
  const url={
    createObjectURL(blob){assert.equal(blob.type,'application/zip');return 'blob:mock-'+blob.size;},
    revokeObjectURL(){}
  };
  const factory=new Function(
    'globalThis','queryClipboardData','queryExportExtension',
    'chooseScriptLocation','uploadTextToHost','saveTextWithLocation',
    'document','URL','setTimeout','console','Blob','TextEncoder',
    sourceHelpers+'\nreturn {queryExportContexts,queryExportContent,queryExportZip,queryExportArchives,saveQueryResultArchives};'
  );
  const helpers=factory(
    fakeGlobal,
    (ctx,result)=>({columns:result.columns.map(c=>c.name),rows:result.rows}),
    format=>format==='csv'?'.csv':format==='json'?'.json':'.txt',
    async name=>{locations.push(name);return 'host';},
    async (name,blob,dir,mime)=>{uploads.push({name,blob,dir,mime});return {ok:true};},
    async (...args)=>{calls.push(args);return {name:'one-zip.zip'};},
    document,url,()=>{},console,Blob,TextEncoder
  );
  const add=(sql,name,value)=>{
    const ctx={resultOwner:owner,resultStatement:sql,queryResult:{columns:[{name}],rows:[[value]]}};
    owner.queryResultContexts.push(ctx);return ctx;
  };
  return {owner,add,helpers,uploads,downloads,locations,calls,fakeGlobal};
}
async function zipEntries(zip){
  const bytes=Buffer.from(await zip.arrayBuffer()),entries=[];
  let pos=0;
  while(pos+30<=bytes.length&&bytes.readUInt32LE(pos)===0x04034b50){
    const compressed=bytes.readUInt32LE(pos+18);
    const nameSize=bytes.readUInt16LE(pos+26);
    const extraSize=bytes.readUInt16LE(pos+28);
    const name=bytes.subarray(pos+30,pos+30+nameSize).toString('utf8');
    const content=bytes.subarray(pos+30+nameSize+extraSize,pos+30+nameSize+extraSize+compressed).toString('utf8');
    entries.push({name,content});
    pos+=30+nameSize+extraSize+compressed;
  }
  assert.equal(bytes.readUInt32LE(pos),0x02014b50,'central directory present');
  assert.equal(bytes.readUInt32LE(bytes.length-22),0x06054b50,'ZIP end record present');
  assert.equal(bytes.readUInt16LE(bytes.length-12),entries.length,'ZIP entry count matches');
  return entries;
}

test('combined ZIP stores each successful result and its originating SQL',async()=>{
  const h=harness();
  const one=h.add('SELECT * FROM users','id',12);
  h.owner.queryResultContexts.push({queryResult:null}); // failed tab 2
  const three=h.add('SELECT * FROM orders','total',50);
  const archives=h.helpers.queryExportArchives(h.owner,[one,three],'csv',true,true);
  assert.deepEqual(archives.map(x=>x.name),['query-results.zip']);
  const entries=await zipEntries(archives[0].content);
  assert.deepEqual(entries.map(x=>x.name),['result-01.csv','result-03.csv']);
  assert.match(entries[0].content,/-- SELECT \* FROM users/);
  assert.match(entries[1].content,/-- SELECT \* FROM orders/);
  assert.doesNotMatch(entries[0].content,/orders/);
});

test('per-result ZIP produces independent valid archive for each result tab',async()=>{
  const h=harness();
  const one=h.add('SELECT user_name FROM users','user_name','A');
  const two=h.add('SELECT 200 AS balance','balance',200);
  const files=h.helpers.queryExportArchives(h.owner,[one,two],'json',true,true,'per-result');
  assert.deepEqual(files.map(x=>x.name),['result-01.zip','result-02.zip']);
  const zip1=await zipEntries(files[0].content);
  const zip2=await zipEntries(files[1].content);
  assert.equal(zip1.length,1);
  assert.equal(zip2.length,1);
  assert.equal(zip1[0].name,'result-01.json');
  assert.equal(zip2[0].name,'result-02.json');
  assert.equal(JSON.parse(zip1[0].content).sql,'SELECT user_name FROM users');
  assert.equal(JSON.parse(zip2[0].content).sql,'SELECT 200 AS balance');
  assert.equal(JSON.parse(zip1[0].content).result[0].user_name,'A');
  assert.equal(JSON.parse(zip2[0].content).result[0].balance,200);
});

test('multiple host ZIPs use one folder chooser and preserve tab numbering',async()=>{
  const h=harness();
  const a=h.add('SELECT 1','a',1);
  h.owner.queryResultContexts.push({queryResult:null});
  const b=h.add('SELECT 3','b',3);
  const files=h.helpers.queryExportArchives(h.owner,[a,b],'txt',true,false,'per-result');
  const result=await h.helpers.saveQueryResultArchives(files);
  assert.equal(h.locations.length,1);
  assert.equal(h.calls.length,1,'choose folder only once');
  assert.deepEqual(h.uploads.map(x=>x.name),['batch-01.zip','batch-03.zip']);
  assert.ok(h.uploads.every(x=>x.dir==='exports'&&x.mime==='application/zip'));
  assert.equal(result.saved,2);
});

test('single combined ZIP uses existing one-file save workflow',async()=>{
  const h=harness();
  const a=h.add('SELECT 1','a',1);
  const archive=h.helpers.queryExportArchives(h.owner,[a],'csv',true,false);
  await h.helpers.saveQueryResultArchives(archive);
  assert.equal(h.calls.length,1);
  assert.equal(h.locations.length,0);
  assert.equal(h.uploads.length,0);
});
