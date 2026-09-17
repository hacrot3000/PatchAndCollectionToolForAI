const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for task inputs');

const originalStartTask=app.startTask;
const nativeFetch=window.fetch.bind(window);
const pendingByTask=new Map();

function enqueueInputs(taskID,values){
  const queue=pendingByTask.get(taskID)||[];queue.push(values);pendingByTask.set(taskID,queue);
}
function dequeueInputs(taskID){
  const queue=pendingByTask.get(taskID);if(!queue?.length)return null;
  const value=queue.shift();if(!queue.length)pendingByTask.delete(taskID);return value;
}

window.fetch=async function(input,init={}){
  try{
    const method=String(init?.method||'GET').toUpperCase();
    const rawURL=typeof input==='string'?input:input?.url||'';
    const url=new URL(rawURL,location.href);
    if(method==='POST'&&url.origin===location.origin&&url.pathname==='/api/sessions'&&typeof init.body==='string'){
      const payload=JSON.parse(init.body);
      if(Number.isInteger(payload.task_id)&&payload.task_id>0){
        const values=dequeueInputs(payload.task_id);
        if(values) init={...init,body:JSON.stringify({...payload,inputs:values})};
      }
    }
  }catch(e){console.warn('Task input request injection skipped',e);}
  return nativeFetch(input,init);
};

function taskInputs(task){return Array.isArray(task?.inputs)?task.inputs:[];}
function promptString(input){
  const label=input.description||input.id;const value=window.prompt(label,input.default??'');
  if(value===null)return {cancelled:true};return {value};
}
function pickString(input){
  const options=Array.isArray(input.options)?input.options.map(String):[];
  if(!options.length)throw new Error(`Input ${input.id}: pickString không có options`);
  const defaultValue=String(input.default??'');const defaultIndex=Math.max(0,options.indexOf(defaultValue));
  const message=(input.description||input.id)+'\n\n'+options.map((value,index)=>`${index+1}. ${value}${index===defaultIndex?'  [default]':''}`).join('\n')+'\n\nNhập số hoặc giá trị:';
  const answer=window.prompt(message,String(defaultIndex+1));if(answer===null)return {cancelled:true};
  const trimmed=answer.trim();const number=Number(trimmed);
  if(Number.isInteger(number)&&number>=1&&number<=options.length)return {value:options[number-1]};
  const exact=options.find(value=>value===trimmed);if(exact!==undefined)return {value:exact};
  throw new Error(`Giá trị không hợp lệ cho ${input.id}`);
}
async function collectTaskInputs(task){
  const values={};
  for(const input of taskInputs(task)){
    let result;
    switch(String(input.type||'')){
      case 'promptString':result=promptString(input);break;
      case 'pickString':result=pickString(input);break;
      case 'command':throw new Error(`Input ${input.id}: type command chưa được hỗ trợ vì có thể tự thực thi command ngoài task`);
      case 'missing':throw new Error(`Input ${input.id} được tham chiếu nhưng chưa khai báo trong tasks.json`);
      default:throw new Error(`Input ${input.id}: type ${input.type||'(trống)'} chưa được hỗ trợ`);
    }
    if(result.cancelled)return null;values[input.id]=result.value;
  }
  return values;
}

async function runTaskWithInputs(task){
  const inputs=taskInputs(task);if(!inputs.length)return originalStartTask(task);
  const values=await collectTaskInputs(task);if(values===null)return;
  enqueueInputs(task.id,values);
  try{return await originalStartTask(task);}catch(e){dequeueInputs(task.id);throw e;}
}
app.startTask=runTaskWithInputs;

function findTaskForButton(button){
  const label=button.textContent.trim();const title=button.title||'';
  const matches=(app.taskData?.tasks||[]).filter(task=>(task.menu_label||task.label)===label&&(!title||(task.detail||task.label)===title));
  return matches.length===1?matches[0]:null;
}

document.addEventListener('click',event=>{
  const button=event.target?.closest?.('button.task');if(!button)return;
  const task=findTaskForButton(button);if(!task||!taskInputs(task).length)return;
  event.preventDefault();event.stopImmediatePropagation();runTaskWithInputs(task).catch(app.showError);
},true);
