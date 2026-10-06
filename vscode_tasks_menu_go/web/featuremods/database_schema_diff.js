const app=globalThis.TaskMenuApp;
const database=globalThis.TaskMenuDatabase;
if(!app||!database)throw new Error('Database unavailable for Schema Diff');

const MAX_SCHEMA_OBJECTS=500;
const SNAPSHOT_CONCURRENCY=4;

const style=document.createElement('style');
style.textContent=`
.db-schema-diff-backdrop{display:none;position:fixed;inset:0;z-index:17650;background:rgba(0,0,0,.52);align-items:flex-start;justify-content:center;padding:4vh 16px}
.db-schema-diff-backdrop.visible{display:flex}
.db-schema-diff-dialog{width:min(1220px,97vw);max-height:92vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 65px rgba(0,0,0,.58);overflow:hidden}
.db-schema-diff-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.db-schema-diff-head strong{flex:1}
.db-schema-diff-config{display:grid;grid-template-columns:auto minmax(180px,1fr) minmax(160px,.8fr) auto minmax(180px,1fr) minmax(160px,.8fr) auto;gap:6px;align-items:center;padding:8px 10px;border-bottom:1px solid #303843}
.db-schema-diff-config select{min-width:0}.db-schema-diff-note{grid-column:1/-1;font-size:10px;opacity:.66}
.db-schema-diff-summary{display:flex;gap:6px;flex-wrap:wrap;padding:7px 10px;border-bottom:1px solid #303843}.db-schema-diff-chip{padding:3px 7px;border:1px solid #3d4653;border-radius:11px;font-size:10px}
.db-schema-diff-body{display:grid;grid-template-columns:minmax(300px,.8fr) minmax(420px,1.2fr);min-height:260px;max-height:68vh}
.db-schema-diff-list{overflow:auto;border-right:1px solid #303843;padding:6px}.db-schema-diff-detail{overflow:auto;padding:8px}
.db-schema-diff-row{display:grid;grid-template-columns:78px minmax(0,1fr);gap:6px;width:100%;text-align:left;padding:6px 7px;border:1px solid transparent;background:transparent}.db-schema-diff-row:hover,.db-schema-diff-row.active{background:#28313e;border-color:#415168}
.db-schema-diff-kind{font-size:9px;text-transform:uppercase;font-weight:700}.db-schema-diff-name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.db-schema-diff-section{margin:8px 0}.db-schema-diff-section>strong{display:block;margin-bottom:4px}.db-schema-diff-code{white-space:pre-wrap;overflow:auto;background:#0b0f14;border:1px solid #303843;border-radius:6px;padding:7px;font:10px/1.45 ui-monospace,monospace;max-height:280px}
.db-schema-diff-actions{display:flex;gap:6px;flex-wrap:wrap;padding:8px 10px;border-top:1px solid #303843}.db-schema-diff-actions .spacer{flex:1}
.db-schema-diff-loading{padding:30px;text-align:center;opacity:.68}.db-schema-diff-warning{color:#f0c66b}.db-schema-diff-error{color:#ff929d}
html[data-taskmenu-theme="light"] .db-schema-diff-dialog{background:#fff;border-color:#b9c0c8}.db-schema-diff-code{color:#e7edf5}html[data-taskmenu-theme="light"] .db-schema-diff-code{background:#f6f8fa;color:#202124;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='db-schema-diff-backdrop';
const dialog=document.createElement('div');dialog.className='db-schema-diff-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Database Schema Diff');
const head=document.createElement('div');head.className='db-schema-diff-head';
const title=document.createElement('strong');title.textContent='DATABASE SCHEMA DIFF';
const status=document.createElement('span');status.style.fontSize='10px';status.style.opacity='.65';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close';
head.append(title,status,closeButton);

const config=document.createElement('div');config.className='db-schema-diff-config';
const sourceLabel=document.createElement('span');sourceLabel.textContent='Source';
const sourceSession=document.createElement('select');
const sourceCatalog=document.createElement('select');
const arrow=document.createElement('strong');arrow.textContent='→';
const targetSession=document.createElement('select');
const targetCatalog=document.createElement('select');
const compareButton=document.createElement('button');compareButton.type='button';compareButton.textContent='Compare schema';
const note=document.createElement('div');note.className='db-schema-diff-note';note.textContent='Source → Target compares schema metadata only. No table data is read.';
config.append(sourceLabel,sourceSession,sourceCatalog,arrow,targetSession,targetCatalog,compareButton,note);

const summary=document.createElement('div');summary.className='db-schema-diff-summary';
const body=document.createElement('div');body.className='db-schema-diff-body';
const list=document.createElement('div');list.className='db-schema-diff-list';
const detail=document.createElement('div');detail.className='db-schema-diff-detail';
body.append(list,detail);
const actions=document.createElement('div');actions.className='db-schema-diff-actions';
const exportSource=document.createElement('button');exportSource.type='button';exportSource.textContent='Export Source structure';
const exportTarget=document.createElement('button');exportTarget.type='button';exportTarget.textContent='Export Target structure';
const migrationButton=document.createElement('button');migrationButton.type='button';migrationButton.textContent='Preview migration';
const spacer=document.createElement('span');spacer.className='spacer';
const closeBottom=document.createElement('button');closeBottom.type='button';closeBottom.textContent='Close';
actions.append(exportSource,exportTarget,migrationButton,spacer,closeBottom);
dialog.append(head,config,summary,body,actions);backdrop.append(dialog);document.body.append(backdrop);

let sourceSnapshot=null;
let targetSnapshot=null;
let currentDiff=null;
let selectedDiffKey='';
let busy=false;

function openViews(){
  return [...(database.views?.values?.()||[])].filter(view=>view?.meta?.id);
}
function viewLabel(view){
  return String(view?.profile?.name||view?.meta?.adapter_kind||view?.meta?.id||'Database');
}
function findView(id){return database.views?.get?.(String(id||''))||null;}
function setBusy(value,message=''){
  busy=Boolean(value);compareButton.disabled=busy;sourceSession.disabled=busy;targetSession.disabled=busy;sourceCatalog.disabled=busy;targetCatalog.disabled=busy;
  status.textContent=message;
}
function objectRows(result){
  const rows=Array.isArray(result)?result:(Array.isArray(result?.objects)?result.objects:[]);
  return rows.filter(item=>item&&item.name).slice(0,MAX_SCHEMA_OBJECTS).map(item=>({
    name:String(item.name||''),kind:String(item.kind||'table'),catalog:String(item.catalog||'')
  }));
}
function catalogRows(result){
  const rows=Array.isArray(result)?result:(Array.isArray(result?.catalogs)?result.catalogs:[]);
  return rows.map(item=>String(item?.name||item?.catalog||'').trim()).filter(Boolean);
}
async function loadCatalogOptions(select,view,preferred=''){
  select.replaceChildren();
  if(!view)return;
  let names=[];
  try{names=catalogRows(await database.request(view.meta.id,'list_catalogs'));}catch{}
  if(!names.length){
    const current=String(view.catalog?.value||view.profile?.database||'').trim();
    if(current)names=[current];
  }
  for(const name of [...new Set(names)]){
    const option=document.createElement('option');option.value=name;option.textContent=name;select.append(option);
  }
  const wanted=String(preferred||view.catalog?.value||view.profile?.database||'');
  if(wanted&&[...select.options].some(option=>option.value===wanted))select.value=wanted;
}
function populateSessionOptions(preferredSource=''){
  const views=openViews();sourceSession.replaceChildren();targetSession.replaceChildren();
  for(const view of views){
    for(const select of [sourceSession,targetSession]){
      const option=document.createElement('option');option.value=String(view.meta.id);option.textContent=viewLabel(view)+' · '+String(view.meta.adapter_kind||'database');select.append(option);
    }
  }
  if(preferredSource&&views.some(view=>String(view.meta.id)===String(preferredSource)))sourceSession.value=String(preferredSource);
  const sourceIndex=views.findIndex(view=>String(view.meta.id)===String(sourceSession.value));
  const target=views.find((_,index)=>index!==sourceIndex)||views[0];
  if(target)targetSession.value=String(target.meta.id);
}
function stableObject(value){
  if(Array.isArray(value))return value.map(stableObject);
  if(!value||typeof value!=='object')return value;
  const out={};
  for(const key of Object.keys(value).sort())out[key]=stableObject(value[key]);
  return out;
}
function normalizedColumn(column){
  return stableObject({
    name:String(column?.name||''),
    type:String(column?.type||''),
    nullable:typeof column?.nullable==='boolean'?column.nullable:(typeof column?.not_null==='boolean'?!column.not_null:null),
    default:column?.default??null,
    primary_key:Boolean(column?.primary_key),
    extra:String(column?.extra||'')
  });
}
function normalizedIndex(index){
  return stableObject({
    name:String(index?.name||''),
    column_name:String(index?.column_name??index?.key??''),
    unique:typeof index?.unique==='boolean'?index.unique:null,
    type:String(index?.type||index?.origin||''),
    sequence:index?.sequence??index?.seq??null
  });
}
function normalizedForeignKey(item){
  return stableObject({
    name:String(item?.name||''),column:String(item?.column||''),
    referenced_catalog:String(item?.referenced_catalog||''),referenced_table:String(item?.referenced_table||''),referenced_column:String(item?.referenced_column||''),
    update_rule:String(item?.update_rule||''),delete_rule:String(item?.delete_rule||'')
  });
}
function normalizeDetail(detail){
  return stableObject({
    columns:Array.isArray(detail?.columns)?detail.columns.map(normalizedColumn):[],
    indexes:Array.isArray(detail?.indexes)?detail.indexes.map(normalizedIndex):(Array.isArray(detail?.detail?.indexes)?detail.detail.indexes.map(normalizedIndex):[]),
    foreign_keys:Array.isArray(detail?.foreign_keys)?detail.foreign_keys.map(normalizedForeignKey):[],
    sql:String(detail?.sql||''),
    info:detail?.detail?.info?stableObject(detail.detail.info):null
  });
}
async function describeObjects(view,catalog,objects){
  const results=new Array(objects.length);let cursor=0;
  async function worker(){
    while(true){
      const index=cursor++;if(index>=objects.length)return;
      const object=objects[index];
      try{
        const detail=await database.request(view.meta.id,'describe_object',{name:object.name,kind:object.kind,catalog});
        results[index]={...object,catalog,detail:normalizeDetail(detail)};
      }catch(error){
        results[index]={...object,catalog,detail:{columns:[],indexes:[],foreign_keys:[],sql:'',info:null},error:String(error?.message||error)};
      }
    }
  }
  await Promise.all(Array.from({length:Math.min(SNAPSHOT_CONCURRENCY,Math.max(1,objects.length))},()=>worker()));
  return results;
}
async function captureSchema(view,catalog){
  if(!view)throw new Error('Database session is unavailable');
  catalog=String(catalog||view.catalog?.value||'').trim();
  if(!catalog)throw new Error('Choose a database/catalog');
  const listed=await database.request(view.meta.id,'list_objects',{catalog});
  const rows=objectRows(listed);
  if(rows.length>=MAX_SCHEMA_OBJECTS)status.textContent='Schema snapshot limited to '+MAX_SCHEMA_OBJECTS+' objects';
  const objects=await describeObjects(view,catalog,rows);
  return {
    version:1,adapter:String(view.meta.adapter_kind||''),session_id:String(view.meta.id||''),profile_id:String(view.meta.profile_id||''),
    profile_name:viewLabel(view),catalog,captured_at:new Date().toISOString(),objects
  };
}
function objectKey(item){return String(item?.kind||'')+'\u0000'+String(item?.name||'');}
function columnMap(object){return new Map((object?.detail?.columns||[]).map(column=>[String(column.name||''),column]));}
function diffColumns(left,right){
  const a=columnMap(left),b=columnMap(right),added=[],removed=[],changed=[];
  for(const [name,column] of a){
    if(!b.has(name))added.push(column);
    else if(JSON.stringify(column)!==JSON.stringify(b.get(name)))changed.push({name,source:column,target:b.get(name)});
  }
  for(const [name,column] of b)if(!a.has(name))removed.push(column);
  return {added,removed,changed};
}
function compareSchemas(source,target){
  const a=new Map((source?.objects||[]).map(item=>[objectKey(item),item]));
  const b=new Map((target?.objects||[]).map(item=>[objectKey(item),item]));
  const rows=[];
  for(const [key,item] of a){
    if(!b.has(key))rows.push({key,status:'missing-target',kind:item.kind,name:item.name,source:item,target:null,column_diff:diffColumns(item,null)});
    else{
      const other=b.get(key);
      if(JSON.stringify(item.detail)!==JSON.stringify(other.detail))rows.push({key,status:'changed',kind:item.kind,name:item.name,source:item,target:other,column_diff:diffColumns(item,other)});
      else rows.push({key,status:'same',kind:item.kind,name:item.name,source:item,target:other,column_diff:{added:[],removed:[],changed:[]}});
    }
  }
  for(const [key,item] of b)if(!a.has(key))rows.push({key,status:'extra-target',kind:item.kind,name:item.name,source:null,target:item,column_diff:diffColumns(null,item)});
  rows.sort((x,y)=>{
    const order={'changed':0,'missing-target':1,'extra-target':2,'same':3};
    return (order[x.status]-order[y.status])||x.kind.localeCompare(y.kind)||x.name.localeCompare(y.name);
  });
  return {source,target,rows,changed:rows.filter(row=>row.status==='changed').length,missing:rows.filter(row=>row.status==='missing-target').length,extra:rows.filter(row=>row.status==='extra-target').length,same:rows.filter(row=>row.status==='same').length};
}
function appendChip(text){const chip=document.createElement('span');chip.className='db-schema-diff-chip';chip.textContent=text;summary.append(chip);}
function renderSummary(){
  summary.replaceChildren();
  if(!currentDiff){appendChip('No comparison yet');return;}
  appendChip('Changed '+currentDiff.changed);appendChip('Missing target '+currentDiff.missing);appendChip('Extra target '+currentDiff.extra);appendChip('Same '+currentDiff.same);
  appendChip(currentDiff.source.adapter+' · '+currentDiff.source.catalog+' → '+currentDiff.target.catalog);
}
function renderDetail(row){
  detail.replaceChildren();
  if(!row){const empty=document.createElement('div');empty.className='db-schema-diff-loading';empty.textContent='Select a schema difference';detail.append(empty);return;}
  const heading=document.createElement('h3');heading.textContent=row.kind+' · '+row.name+' · '+row.status;detail.append(heading);
  const columns=row.column_diff||{added:[],removed:[],changed:[]};
  const summaryText=['Columns +'+columns.added.length,'-'+columns.removed.length,'~'+columns.changed.length].join(' · ');
  const p=document.createElement('div');p.textContent=summaryText;p.style.opacity='.7';p.style.fontSize='11px';detail.append(p);
  const sections=[
    ['Source structure',row.source?.detail],
    ['Target structure',row.target?.detail]
  ];
  for(const [label,value] of sections){
    const section=document.createElement('div');section.className='db-schema-diff-section';
    const strong=document.createElement('strong');strong.textContent=label;
    const pre=document.createElement('pre');pre.className='db-schema-diff-code';pre.textContent=value?JSON.stringify(value,null,2):'(not present)';
    section.append(strong,pre);detail.append(section);
  }
}
function renderDiff(){
  renderSummary();list.replaceChildren();
  const rows=currentDiff?.rows||[];
  const changedRows=rows.filter(row=>row.status!=='same');
  const display=changedRows.length?changedRows:rows;
  if(!display.length){const empty=document.createElement('div');empty.className='db-schema-diff-loading';empty.textContent='No schema objects found';list.append(empty);renderDetail(null);return;}
  if(!selectedDiffKey||!display.some(row=>row.key===selectedDiffKey))selectedDiffKey=display[0].key;
  for(const row of display){
    const button=document.createElement('button');button.type='button';button.className='db-schema-diff-row';button.classList.toggle('active',row.key===selectedDiffKey);
    const kind=document.createElement('span');kind.className='db-schema-diff-kind';kind.textContent=row.status;
    const name=document.createElement('span');name.className='db-schema-diff-name';name.textContent=row.kind+' · '+row.name;
    button.append(kind,name);button.onclick=()=>{selectedDiffKey=row.key;renderDiff();};list.append(button);
  }
  renderDetail(display.find(row=>row.key===selectedDiffKey)||display[0]);
}
async function compare(){
  if(busy)return;
  const sourceView=findView(sourceSession.value),targetView=findView(targetSession.value);
  if(!sourceView||!targetView)throw new Error('Open at least two database sessions to compare');
  setBusy(true,'Capturing source schema…');list.replaceChildren();detail.replaceChildren();
  const loading=document.createElement('div');loading.className='db-schema-diff-loading';loading.textContent='Capturing schema metadata…';list.append(loading);
  try{
    sourceSnapshot=await captureSchema(sourceView,sourceCatalog.value);
    setBusy(true,'Capturing target schema…');
    targetSnapshot=await captureSchema(targetView,targetCatalog.value);
    currentDiff=compareSchemas(sourceSnapshot,targetSnapshot);selectedDiffKey='';renderDiff();
    status.textContent='Compared '+sourceSnapshot.objects.length+' source / '+targetSnapshot.objects.length+' target objects';
  }finally{setBusy(false,status.textContent);}
}
async function exportSnapshot(snapshot,label){
  if(!snapshot)throw new Error('Run schema comparison first');
  const safe=String(snapshot.catalog||'schema').replace(/[^A-Za-z0-9._-]+/g,'_')||'schema';
  await database.saveTextWithLocation('Export '+label+' structure',safe+'-structure.json',JSON.stringify(snapshot,null,2)+'\n',{
    description:'Database schema structure',mime:'application/json;charset=utf-8',extensions:['.json'],hostFileLabel:'Structure file name:'
  });
}
async function refreshCatalogs(which){
  const select=which==='source'?sourceSession:targetSession;
  const catalog=which==='source'?sourceCatalog:targetCatalog;
  await loadCatalogOptions(catalog,findView(select.value));
}
async function open(preferredView=null){
  const views=openViews();
  if(views.length<2)throw new Error('Open at least two database sessions before using Schema Diff');
  populateSessionOptions(preferredView?.meta?.id||'');
  await Promise.all([refreshCatalogs('source'),refreshCatalogs('target')]);
  sourceSnapshot=null;targetSnapshot=null;currentDiff=null;selectedDiffKey='';status.textContent='';renderSummary();list.replaceChildren();renderDetail(null);
  backdrop.classList.add('visible');
}
function close(){if(busy)return;backdrop.classList.remove('visible');}

sourceSession.onchange=()=>refreshCatalogs('source').catch(app.showError);
targetSession.onchange=()=>refreshCatalogs('target').catch(app.showError);
compareButton.onclick=()=>compare().catch(error=>{status.textContent=String(error?.message||error);app.showError(error);setBusy(false,status.textContent);});
exportSource.onclick=()=>exportSnapshot(sourceSnapshot,'Source').catch(app.showError);
exportTarget.onclick=()=>exportSnapshot(targetSnapshot,'Target').catch(app.showError);
closeButton.onclick=close;closeBottom.onclick=close;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuDatabaseSchemaDiff={
  open,close,captureSchema,compareSchemas,normalizeDetail,
  get sourceSnapshot(){return sourceSnapshot;},get targetSnapshot(){return targetSnapshot;},get diff(){return currentDiff;}
};
