(function(root){
 'use strict';
 const app=root.TaskMenuApp,agentChat=root.TaskMenuAgentChat,doc=root.document;
 function create(options={}){
  const document=options.document||doc,api=options.app||app,chat=options.agentChat||agentChat;
  const mount=options.mount||(document&&document.querySelector?.('.agentchat-toolbar-actions'));
  if(!document||!api||typeof api.jsonFetch!=='function'||!chat||!mount)throw new Error('Agent coordination UI dependencies are unavailable.');
  const el=(tag,text='')=>{const node=document.createElement(tag);if(text)node.textContent=text;return node;};
  const button=(text,action)=>{const node=el('button',text);node.type='button';node.addEventListener('click',()=>Promise.resolve(action()).catch(showError));return node;};
  const label=(text,control)=>{const node=el('label',text);node.append(control);return node;};
  const action=(method,body)=>({method,headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const trigger=button('Groups & Coordination',open);
  trigger.setAttribute('aria-haspopup','dialog');trigger.setAttribute('aria-expanded','false');
  const overlay=el('div');overlay.className='agentchat-coordination-overlay';overlay.hidden=true;
  const panel=el('section');panel.className='agentchat-coordination-panel';panel.setAttribute('role','dialog');panel.setAttribute('aria-modal','true');panel.setAttribute('aria-label','Conversation groups and coordination');
  const style=el('style');style.textContent='.agentchat-coordination-overlay{position:fixed;inset:0;z-index:10020;background:#0009;display:grid;place-items:center;padding:16px}.agentchat-coordination-overlay[hidden]{display:none}.agentchat-coordination-panel{width:min(900px,96vw);max-height:90vh;overflow:auto;background:var(--panel-bg,#202833);color:var(--text,#eef2f6);border:1px solid #68798e;border-radius:10px;padding:14px;display:grid;gap:10px}.agentchat-coordination-panel header,.agentchat-coordination-row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.agentchat-coordination-panel header strong{flex:1}.agentchat-coordination-panel label{display:grid;gap:4px;flex:1;min-width:180px}.agentchat-coordination-panel input,.agentchat-coordination-panel select,.agentchat-coordination-panel textarea{min-width:0;max-width:100%;padding:6px}.agentchat-coordination-panel textarea{min-height:70px}.agentchat-coordination-feed{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:8px}.agentchat-coordination-feed section{border:1px solid #526176;border-radius:6px;padding:8px;min-width:0}.agentchat-coordination-feed pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:180px;overflow:auto;font-size:11px}.agentchat-coordination-status{min-height:1.4em}';
  const heading=el('header'),title=el('strong','Conversation groups & coordination');
  const status=el('div');status.className='agentchat-coordination-status';status.setAttribute('role','status');
  const close=button('Close',closePanel);heading.append(title,button('Refresh',refresh),close);
  const conversationSelect=el('select');conversationSelect.setAttribute('aria-label','Conversation');
  const groupSelect=el('select');groupSelect.setAttribute('aria-label','Conversation group');
  const selectors=el('div');selectors.className='agentchat-coordination-row';selectors.append(label('Conversation',conversationSelect),label('Group',groupSelect));
  const createTitle=el('input');createTitle.maxLength=120;createTitle.placeholder='New group title';
  const instructions=el('textarea');instructions.maxLength=32768;instructions.placeholder='Shared instructions for this group';
  const groupActions=el('div');groupActions.className='agentchat-coordination-row';
  groupActions.append(button('Create group',createGroup),button('Add conversation',()=>changeMembership('add_member')),button('Remove conversation',()=>changeMembership('remove_member')),button('Save shared instructions',saveInstructions));
  const groupForm=el('section');groupForm.append(label('Create group',createTitle),label('Shared instructions',instructions),groupActions);
  const progressStatus=el('select');
  for(const value of ['idle','running','waiting','completed','failed','unknown']){const option=el('option',value);option.value=value;progressStatus.append(option);}
  const progressSummary=el('input');progressSummary.maxLength=2000;progressSummary.placeholder='Short peer progress summary';
  const progressRow=el('div');progressRow.className='agentchat-coordination-row';progressRow.append(label('Peer status',progressStatus),label('Summary',progressSummary),button('Publish progress',publishProgress));
  const peerTarget=el('input');peerTarget.maxLength=256;peerTarget.placeholder='Target conversation ID';
  const peerKind=el('select');
  for(const value of ['ask','answer','progress','blocker']){const option=el('option',value);option.value=value;peerKind.append(option);}
  peerKind.value='ask';
  const peerCorrelation=el('input');peerCorrelation.maxLength=128;peerCorrelation.placeholder='Correlation ID (required for answer)';
  const peerBody=el('textarea');peerBody.maxLength=8000;peerBody.placeholder='Message or question for a group member';
  const messageRow=el('div');messageRow.className='agentchat-coordination-row';messageRow.append(label('To conversation',peerTarget),label('Message kind',peerKind),label('Correlation ID',peerCorrelation),label('Peer message',peerBody),button('Send peer message',sendPeerMessage));
  const contractKey=el('input');contractKey.maxLength=200;contractKey.placeholder='Contract key';
  const contractSignature=el('input');contractSignature.maxLength=2000;contractSignature.placeholder='Function/API signature';
  const contractOwner=el('input');contractOwner.maxLength=256;contractOwner.placeholder='Owner task (optional)';
  const contractFiles=el('input');contractFiles.placeholder='Related file paths, comma-separated';
  const contractRow=el('div');contractRow.className='agentchat-coordination-row';contractRow.append(label('Contract key',contractKey),label('Signature',contractSignature),label('Owner task',contractOwner),label('Files',contractFiles),button('Publish contract',publishContract));
  const contractRevision=el('input');contractRevision.type='number';contractRevision.min='1';contractRevision.placeholder='Revision';
  const contractStatus=el('select');
  for(const value of ['integrated','verified','needs_review']){const option=el('option',value);option.value=value;contractStatus.append(option);}
  const integratedRef=el('input');integratedRef.placeholder='Integrated artifact/reference';
  const testEvidence=el('input');testEvidence.placeholder='Build/test evidence';
  const advanceRow=el('div');advanceRow.className='agentchat-coordination-row';advanceRow.append(label('Revision',contractRevision),label('Transition',contractStatus),label('Integrated reference',integratedRef),label('Test evidence',testEvidence),button('Advance contract',advanceContract));
  const artifactKind=el('select');
  for(const value of ['commit','diff','patch','file','build']){const option=el('option',value);option.value=value;artifactKind.append(option);}
  const artifactRef=el('input');artifactRef.placeholder='Artifact reference';
  const artifactSHA=el('input');artifactSHA.maxLength=64;artifactSHA.placeholder='SHA-256';
  const artifactRun=el('input');artifactRun.placeholder='Producer run ID (optional)';
  const artifactRow=el('div');artifactRow.className='agentchat-coordination-row';artifactRow.append(label('Artifact kind',artifactKind),label('Artifact reference',artifactRef),label('SHA-256',artifactSHA),label('Producer run',artifactRun),button('Publish artifact',publishArtifact));
  const dependencyConsumer=el('input');dependencyConsumer.placeholder='Consumer task';
  const dependencyProducer=el('input');dependencyProducer.placeholder='Producer task';
  const dependencyContract=el('input');dependencyContract.placeholder='Contract key';
  const dependencyRevision=el('input');dependencyRevision.type='number';dependencyRevision.min='1';dependencyRevision.placeholder='Revision (optional)';
  const dependencyStatus=el('select');
  for(const value of ['planned','implemented','integrated','verified']){const option=el('option',value);option.value=value;dependencyStatus.append(option);}
  const dependencyWorkspace=el('input');dependencyWorkspace.placeholder='Workspace/ref (required for verified)';
  const dependencyEvidence=el('input');dependencyEvidence.placeholder='Test evidence (required for verified)';
  const dependencyRow=el('div');dependencyRow.className='agentchat-coordination-row';dependencyRow.append(label('Consumer',dependencyConsumer),label('Producer',dependencyProducer),label('Contract key',dependencyContract),label('Contract revision',dependencyRevision),label('Status',dependencyStatus),label('Workspace/ref',dependencyWorkspace),label('Test evidence',dependencyEvidence),button('Register dependency',registerDependency));
  const feed=el('div');feed.className='agentchat-coordination-feed';
  const views={};
  for(const key of ['peers','messages','contracts','artifacts','dependencies','events']){
   const section=el('section'),pre=el('pre','None');section.append(el('strong',key[0].toUpperCase()+key.slice(1)),pre);feed.append(section);views[key]=pre;
  }
  panel.append(heading,selectors,groupForm,progressRow,messageRow,contractRow,advanceRow,artifactRow,dependencyRow,status,feed);overlay.append(style,panel);document.body.append(overlay);mount.append(trigger);
  let groups=[],conversations=[];
  function showError(error){status.textContent=String(error?.message||error||'Coordination request failed.');}
  function selectedConversation(){return conversationSelect.value||chat.selectedSessionID||chat.activeSessionID||'';}
  function selectedGroup(){return groups.find(group=>group.id===groupSelect.value)||null;}
  function setOptions(select,items,placeholder,selected){
   select.replaceChildren();const empty=el('option',placeholder);empty.value='';select.append(empty);
   for(const item of items){const option=el('option',item.label);option.value=item.value;select.append(option);}
   if(items.some(item=>item.value===selected))select.value=selected;else select.value='';
  }
  async function refresh(){
   const priorGroup=groupSelect.value;
   const [conversationData,groupData]=await Promise.all([api.jsonFetch('/api/agent/conversations'),api.jsonFetch('/api/agent/conversation-groups')]);
   conversations=Array.isArray(conversationData.conversations)?conversationData.conversations:[];
   groups=Array.isArray(groupData.groups)?groupData.groups:[];
   const current=chat.selectedSessionID||chat.activeSessionID||conversationSelect.value;
   setOptions(conversationSelect,conversations.map(item=>({value:item.id,label:(item.title||item.id)+' · '+item.id})), 'Select conversation',current);
   setOptions(groupSelect,groups.map(item=>({value:item.id,label:item.title+' · '+item.members.length+' member(s)'})),'Select group',priorGroup||groups.find(group=>group.members.includes(conversationSelect.value))?.id||'');
   const group=selectedGroup();instructions.value=group?.shared_instructions||'';
   const query=new URLSearchParams();if(group)query.set('group_id',group.id);if(selectedConversation())query.set('conversation_id',selectedConversation());
   const data=await api.jsonFetch('/api/agent/coordination'+(query.size?'?'+query.toString():''));
   for(const key of Object.keys(views)){const items=Array.isArray(data[key])?data[key]:[];views[key].textContent=items.length?JSON.stringify(items,null,2):'None';}
   status.textContent='Group and coordination data refreshed.';
   return data;
  }
  async function open(){overlay.hidden=false;trigger.setAttribute('aria-expanded','true');await refresh();}
  function closePanel(){overlay.hidden=true;trigger.setAttribute('aria-expanded','false');}
  async function createGroup(){
   const title=createTitle.value.trim();if(!title)throw new Error('Enter a group title.');
   await api.jsonFetch('/api/agent/conversation-groups',action('POST',{title}));createTitle.value='';await refresh();
  }
  async function changeMembership(kind){
   const group=selectedGroup(),conversationID=selectedConversation();if(!group||!conversationID)throw new Error('Select both a group and a conversation.');
   await api.jsonFetch('/api/agent/conversation-groups/'+encodeURIComponent(group.id),action('POST',{action:kind,conversation_id:conversationID}));await refresh();
  }
  async function saveInstructions(){
   const group=selectedGroup();if(!group)throw new Error('Select a group first.');
   await api.jsonFetch('/api/agent/conversation-groups/'+encodeURIComponent(group.id),action('POST',{action:'update_instructions',expected_revision:group.shared_revision,instructions:instructions.value}));await refresh();
  }
  async function publishProgress(){
   const conversationID=selectedConversation();if(!conversationID)throw new Error('Select a conversation first.');
   const group=selectedGroup();await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'peer_progress',conversation_id:conversationID,group_id:group?.id||'',status:progressStatus.value,body:progressSummary.value.trim()}));progressSummary.value='';await refresh();
  }
  async function sendPeerMessage(){
   const group=selectedGroup(),conversationID=selectedConversation(),body=peerBody.value.trim(),messageKind=peerKind.value;
   if(!group||!conversationID||!peerTarget.value.trim()||!body)throw new Error('Select a group and conversation, then provide a target and message.');
   let correlationID=peerCorrelation.value.trim();
   if(!correlationID&&messageKind!=='answer')correlationID=root.crypto?.randomUUID?.()||('coord-'+Date.now().toString(36));
   if(messageKind==='answer'&&!correlationID)throw new Error('Provide the correlation ID of the question being answered.');
   await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'peer_message',message_kind:messageKind,conversation_id:conversationID,group_id:group.id,to_conversation_id:peerTarget.value.trim(),body,correlation_id:correlationID}));peerBody.value='';await refresh();
  }
  async function publishContract(){
   const group=selectedGroup(),key=contractKey.value.trim(),signature=contractSignature.value.trim();
   if(!group||!key||!signature)throw new Error('Select a group and provide a contract key and signature.');
   const filePaths=contractFiles.value.split(',').map(value=>value.trim()).filter(Boolean);
   await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'contract_publish',conversation_id:selectedConversation(),group_id:group.id,contract:{key,signature,owner_task:contractOwner.value.trim(),owner_group:group.id,file_paths:filePaths,status:'planned'}}));await refresh();
  }
  async function advanceContract(){
   const key=contractKey.value.trim(),revision=Number(contractRevision.value||0);
   if(!key)throw new Error('Provide the contract key to advance.');
   await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'contract_advance',conversation_id:selectedConversation(),contract_key:key,revision,status:contractStatus.value,integrated_ref:integratedRef.value.trim(),test_evidence:testEvidence.value.trim()}));await refresh();
  }
  async function publishArtifact(){
   const ref=artifactRef.value.trim(),sha256=artifactSHA.value.trim().toLowerCase();
   if(!ref||!/^[0-9a-f]{64}$/.test(sha256))throw new Error('Provide an artifact reference and a 64-character SHA-256 digest.');
   await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'artifact_publish',conversation_id:selectedConversation(),artifact:{kind:artifactKind.value,ref,sha256,producer_run:artifactRun.value.trim(),conversation_id:selectedConversation()}}));await refresh();
  }
  async function registerDependency(){
   const consumerTask=dependencyConsumer.value.trim(),producerTask=dependencyProducer.value.trim(),contractKeyValue=dependencyContract.value.trim();
   if(!consumerTask||!producerTask||!contractKeyValue)throw new Error('Provide consumer task, producer task, and contract key.');
   await api.jsonFetch('/api/agent/coordination',action('POST',{kind:'dependency_register',conversation_id:selectedConversation(),dependency:{consumer_task:consumerTask,producer_task:producerTask,contract_key:contractKeyValue,contract_revision:Number(dependencyRevision.value||0),status:dependencyStatus.value,workspace_ref:dependencyWorkspace.value.trim(),test_evidence:dependencyEvidence.value.trim()}}));await refresh();
  }
  conversationSelect.addEventListener('change',()=>refresh().catch(showError));groupSelect.addEventListener('change',()=>refresh().catch(showError));
  overlay.addEventListener('click',event=>{if(event.target===overlay)closePanel();});
  return {element:overlay,button:trigger,open,close:closePanel,refresh,createGroup,changeMembership,saveInstructions,publishProgress,sendPeerMessage,publishContract,advanceContract,publishArtifact,registerDependency,
   fields:{conversationSelect,groupSelect,createTitle,instructions,progressStatus,progressSummary,peerTarget,peerKind,peerCorrelation,peerBody,contractKey,contractSignature,contractOwner,contractFiles,contractRevision,contractStatus,integratedRef,testEvidence,artifactKind,artifactRef,artifactSHA,artifactRun,dependencyConsumer,dependencyProducer,dependencyContract,dependencyRevision,dependencyStatus,dependencyWorkspace,dependencyEvidence}};
 }
 root.TaskMenuAgentCoordination={create};
 if(doc&&root.TaskMenuApp&&root.TaskMenuAgentChat){
  try{create();}catch(error){console.warn('Agent coordination UI unavailable:',error);}
 }
})(globalThis);
