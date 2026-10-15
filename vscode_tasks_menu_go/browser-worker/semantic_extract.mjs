// A pure page-context semantic extractor; no website JavaScript leaves Chromium.
// Exported separately to allow real Chromium smoke tests of div click detection.
export function extractSemanticElements(revision){
    const maxScan=3500,maxOutput=450,result=[];
    const root=document.body||document.documentElement;
    if(!root)return result;
    const prefix='td-sem-';
    const mark='data-taskdeck-semantic-id';
    const store=window.__taskdeckSemanticState||(window.__taskdeckSemanticState={counter:0});
    const listenerNodes=window.__taskdeckInteractiveListeners;
    function label(el){
      const aria=el.getAttribute('aria-label')||el.getAttribute('title')||el.getAttribute('alt');
      const raw=aria||el.innerText||el.textContent||'';
      return String(raw).replace(/\s+/g,' ').trim().slice(0,360);
    }
    // Iterative bounded traversal: huge SPAs must not recurse over the
    // complete DOM or allocate unbounded nested arrays.
    const nodes=[],stack=[...root.children].reverse();
    while(stack.length&&nodes.length<maxScan){
      const current=stack.pop();
      nodes.push(current);
      const children=[...current.children];
      if(current.shadowRoot)children.push(...current.shadowRoot.children);
      for(let i=children.length-1;i>=0;i--)stack.push(children[i]);
    }
    for(const el of nodes){
      if(result.length>=maxOutput)break;
      const tag=el.tagName?.toLowerCase();
      if(!tag||['script','style','noscript','meta','link','svg','path','template'].includes(tag))continue;
      const computed=getComputedStyle(el);
      if(computed.display==='none'||computed.visibility==='hidden'||Number(computed.opacity)===0)continue;
      if(el.closest('[aria-hidden="true"],[inert]'))continue;
      const rect=el.getBoundingClientRect();
      if(!rect.width||!rect.height)continue;
      const role=(el.getAttribute('role')||'').toLowerCase();
      const type=(el.getAttribute('type')||'').toLowerCase();
      if(type==='hidden')continue;
      const interactiveTag=['button','a','input','textarea','select','summary'].includes(tag);
      const roleAction=['button','link','checkbox','switch','tab','menuitem','option','radio'].includes(role);
      const inlineEvent=typeof el.onclick==='function'||el.hasAttribute('onclick')||el.hasAttribute('ng-click')||el.hasAttribute('v-on:click')||el.hasAttribute('@click')||el.hasAttribute('x-on:click')||el.hasAttribute('jsaction');
      const listener=Boolean(listenerNodes?.has(el));
      const reactProps=Object.keys(el).some(key=>(key.startsWith('__reactProps$')||key.startsWith('__vue'))&&typeof el[key]?.onClick==='function');
      const cssAction=computed.cursor==='pointer'&&tag!=='body'&&tag!=='html';
      const isAction=interactiveTag||roleAction||inlineEvent||listener||reactProps||cssAction;
      const textTag=/^h[1-6]$/.test(tag)||['p','li','label','th','td','pre','blockquote'].includes(tag);
      const leafText=(tag==='div'||tag==='span')&&el.children.length===0;
      if(!isAction&&!textTag&&!leafText)continue;
      // Suppress decorative text nested inside a known action element.
      if(!isAction&&(el.parentElement?.closest('button,a,[role="button"],[role="link"]')||getComputedStyle(el.parentElement||el).cursor==='pointer'))continue;
      let text=label(el);
      if(!isAction&&!text)continue;
      const kind=tag==='input'?(type==='checkbox'||type==='radio'?'check':'input'):
        tag==='textarea'?'input':tag==='select'?'select':
        (isAction?'action':'text');
      if(kind==='input'&&type==='password')text=text||'Password';
      if(isAction&&!text)text='Interactive element';
      const id=prefix+(++store.counter).toString(36);
      el.setAttribute(mark,id);
      const entry={id,kind,tag,role,text,disabled:el.disabled===true,href:tag==='a'?el.href||'':undefined};
      if(kind==='input'){
        entry.input_type=type||'text';entry.placeholder=el.getAttribute('placeholder')||'';
        entry.value=type==='password'?'':String(el.value||'').slice(0,800);
      }
      if(kind==='check'){entry.input_type=type;entry.checked=Boolean(el.checked||el.getAttribute('aria-checked')==='true');}
      if(kind==='select'){
        entry.value=String(el.value||'');
        entry.options=[...el.options].slice(0,75).map(o=>({value:String(o.value),text:String(o.text).slice(0,120)}));
      }
      result.push(entry);
    }
    return result;

}

// Registers direct click/pointer listeners before site scripts run.
export function installEventInstrumentation(){>{
      const original=EventTarget.prototype.addEventListener;
      const listeners=new WeakSet();
      Object.defineProperty(window,'__taskdeckInteractiveListeners',{value:listeners,configurable:false});
      EventTarget.prototype.addEventListener=function(type,...args){
        if(['click','pointerdown','mousedown','touchstart'].includes(String(type).toLowerCase()))listeners.add(this);
        return Reflect.apply(original,this,[type,...args]);
      };
    
}
