// Actual-run DAG. Nodes are run IDs, not static schedule definitions.
const scheduleRunGraphStyle=document.createElement('style');
scheduleRunGraphStyle.textContent=[
'.schedule-run-graph{max-width:100%;overflow:auto;border:1px solid #3e536d;border-radius:6px;margin:8px 0}',
'.schedule-run-graph svg{display:block;background:#14202d}',
'.schedule-run-graph rect{fill:#344661;stroke:#91a5bf;stroke-width:1.5}',
'.schedule-run-graph .success rect{fill:#1b5042;stroke:#60b397}',
'.schedule-run-graph .failure rect,.schedule-run-graph .interrupted rect{fill:#673636;stroke:#e28c8c}',
'.schedule-run-graph .queued rect{fill:#645436;stroke:#d4af6e}',
'.schedule-run-graph .edge{fill:none;stroke:#91a5bf;stroke-width:1.8}',
'.schedule-run-graph .run-node{cursor:pointer}',
'.schedule-run-graph .run-node text{fill:#fff;font:11px system-ui,sans-serif;pointer-events:none}',
'.schedule-run-graph .run-node:focus-visible rect{stroke-width:3}'
].join('\n');
document.head.append(scheduleRunGraphStyle);

function scheduleDrawWorkflowRuns(host,runs,openRun=()=>{}){
 host.replaceChildren();
 if(!runs.length)return;
 const ns='http://www.w3.org/2000/svg';
 const element=(tag,attributes={})=>{
  const node=document.createElementNS(ns,tag);
  for(const [name,value] of Object.entries(attributes))node.setAttribute(name,String(value));
  return node;
 };
 const byID=new Map(runs.map(run=>[run.id,run]));
 const parents=new Map();
 for(const run of runs){
  const refs=(run.trigger_run_ids?.length?run.trigger_run_ids:
   run.trigger_run_id?[run.trigger_run_id]:[]).filter(id=>byID.has(id));
  parents.set(run.id,[...new Set(refs)]);
 }
 const depth=new Map(),visiting=new Set();
 const visit=id=>{
  if(depth.has(id))return depth.get(id);
  if(visiting.has(id))return 0;
  visiting.add(id);
  const refs=parents.get(id)||[];
  const d=refs.length?1+Math.max(...refs.map(visit)):0;
  visiting.delete(id);depth.set(id,Math.min(20,d));return d;
 };
 for(const run of runs)visit(run.id);
 const cols=new Map();
 for(const run of runs){
  const d=depth.get(run.id)||0;
  if(!cols.has(d))cols.set(d,[]);
  cols.get(d).push(run);
 }
 const positions=new Map(),w=185,h=65,dx=240,dy=94;
 for(const [column,items] of cols){
  items.sort((a,b)=>new Date(a.started_at)-new Date(b.started_at));
  items.forEach((run,row)=>positions.set(run.id,{x:22+column*dx,y:24+row*dy}));
 }
 const svg=element('svg',{role:'img','aria-label':'Actual workflow execution graph',
  width:Math.max(460,(Math.max(...depth.values())+1)*dx+50),
  height:Math.max(120,Math.max(...[...cols.values()].map(c=>c.length))*dy+30)});
 const defs=element('defs'),marker=element('marker',{id:'schedule-run-arrow',markerWidth:8,markerHeight:8,refX:7,refY:4,orient:'auto'});
 marker.append(element('path',{d:'M0,0 L8,4 L0,8 Z',fill:'#9cacc2'}));
 defs.append(marker);svg.append(defs);
 for(const [child,ids] of parents){
  const to=positions.get(child);
  for(const parent of ids){
   const from=positions.get(parent);
   const x1=from.x+w,y1=from.y+h/2,x2=to.x,y2=to.y+h/2,m=(x1+x2)/2;
   svg.append(element('path',{d:'M'+x1+' '+y1+' C'+m+' '+y1+' '+m+' '+y2+' '+x2+' '+y2,
    class:'edge','marker-end':'url(#schedule-run-arrow)'}));
  }
 }
 for(const run of runs){
  const p=positions.get(run.id),g=element('g',{class:'run-node '+run.status,role:'button',tabindex:0,
   'aria-label':'Open console for '+run.name});
  g.append(element('rect',{x:p.x,y:p.y,width:w,height:h,rx:7}));
  const title=element('text',{x:p.x+10,y:p.y+25});
  title.textContent=run.name.length>23?run.name.slice(0,22)+'…':run.name;g.append(title);
  const caption=element('text',{x:p.x+10,y:p.y+45});
  caption.textContent=run.status+' · attempt '+(run.attempts||1);g.append(caption);
  const open=()=>openRun(run.id,run.schedule_id);
  g.addEventListener('click',open);
  g.addEventListener('keydown',event=>{
   if(event.key==='Enter'||event.key===' '){event.preventDefault();open()}
  });
  svg.append(g);
 }
 const scroll=document.createElement('div');scroll.className='schedule-run-graph';
 scroll.append(svg);host.append(scroll);
}
globalThis.TaskMenuWorkflowGraph={render:scheduleDrawWorkflowRuns};
