// Executed *inside* Chromium. Never execute remote site HTML or JavaScript in
// TaskDeck's own frontend. Stable IDs are scoped to a page and verified against
// a snapshot revision before performing an action.
export function extractSemanticElements() {
  const maxScan=3500,maxOutput=450,result=[];
  const root=document.body||document.documentElement;
  if(!root)return result;
  const store=window.__taskdeckSemanticState||(window.__taskdeckSemanticState={counter:0});
  const listened=window.__taskdeckInteractiveListeners;
  const idAttribute='data-taskdeck-semantic-id';
  const usedIDs=new Set();

  function safeText(value,max=360) {
    return String(value||'').replace(/\s+/g,' ').trim().slice(0,max);
  }
  function isSensitive(el) {
    const type=(el.getAttribute('type')||'').toLowerCase();
    if(type==='password')return true;
    const autocomplete=(el.getAttribute('autocomplete')||'').toLowerCase();
    if(/password|one-time-code|cc-csc/.test(autocomplete))return true;
    const hint=[
      el.getAttribute('aria-label'),el.getAttribute('name'),el.getAttribute('id'),
      el.getAttribute('placeholder'),el.getAttribute('data-testid')
    ].join(' ').toLowerCase();
    return /\b(password|passwd|passcode|pin|otp|token|secret|verification.code|security.code|cvv|cvc)\b/i.test(hint);
  }
  function label(el,editor=false) {
    const aria=el.getAttribute('aria-label')||el.getAttribute('title')||el.getAttribute('alt');
    const associated=[...(el.labels||[])].map(node=>node.innerText||node.textContent||'').join(' ');
    const placeholder=el.getAttribute('placeholder')||el.getAttribute('data-placeholder')||el.getAttribute('aria-placeholder');
    // Editor body text is the VALUE, not its label.
    const raw=aria||associated||placeholder||(editor?'':(el.innerText||el.textContent));
    return safeText(raw);
  }
  const nodes=[],stack=[...root.children].reverse();
  while(stack.length&&nodes.length<maxScan) {
    const el=stack.pop();nodes.push(el);
    const children=[...el.children];
    if(el.shadowRoot)children.push(...el.shadowRoot.children);
    for(let j=children.length-1;j>=0;j--)stack.push(children[j]);
  }

  for(const el of nodes) {
    if(result.length>=maxOutput)break;
    const tag=el.tagName?.toLowerCase();
    if(!tag||['script','style','noscript','meta','link','svg','path','template'].includes(tag))continue;
    const type=(el.getAttribute('type')||'').toLowerCase();
    if(type==='hidden')continue;
    const style=getComputedStyle(el),rect=el.getBoundingClientRect();
    if(style.display==='none'||style.visibility==='hidden'||style.visibility==='collapse'||Number(style.opacity)===0||!rect.width||!rect.height)continue;
    if(el.closest('[aria-hidden="true"],[inert]'))continue;
    const role=(el.getAttribute('role')||'').toLowerCase();
    const nativeInput=tag==='input'||tag==='textarea';
    const explicitEditable=el.hasAttribute('contenteditable')&&el.getAttribute('contenteditable')!=='false';
    const contentEditor=explicitEditable&&el.isContentEditable;
    const textboxRole=['textbox','searchbox'].includes(role);
    const knownEditor=el.matches('.monaco-editor,.CodeMirror,.cm-editor,[data-lexical-editor],[data-slate-editor]');
    // Prefer the true editable descendant to an editor shell, unless that
    // descendant is hidden/zero-sized (e.g. Monaco's hidden input).
    const visibleEditableDescendant=[...el.querySelectorAll('input,textarea,[contenteditable="true"],[contenteditable="plaintext-only"]')].some(node=>{
      const box=node.getBoundingClientRect(),css=getComputedStyle(node);
      return box.width>0&&box.height>0&&css.visibility!=='hidden'&&css.display!=='none';
    });
    if(knownEditor&&visibleEditableDescendant&&!contentEditor&&!textboxRole)continue;
    if(contentEditor){
      const parent=el.parentElement?.closest('[contenteditable="true"],[contenteditable="plaintext-only"]');
      if(parent)continue;
    }
    const customInput=!nativeInput&&(contentEditor||textboxRole||knownEditor);
    const inputCandidate=nativeInput||customInput;
    const interactiveTag=['button','a','summary','select'].includes(tag);
    const roleAction=['button','link','checkbox','switch','tab','menuitem','option','radio','combobox'].includes(role);
    const inline=typeof el.onclick==='function'||el.hasAttribute('onclick')||el.hasAttribute('ng-click')||
      el.hasAttribute('v-on:click')||el.hasAttribute('@click')||el.hasAttribute('x-on:click')||el.hasAttribute('jsaction');
    const listener=Boolean(listened?.has(el));
    const framework=Object.keys(el).some(k=>(k.startsWith('__reactProps$')||k.startsWith('__vue'))&&typeof el[k]?.onClick==='function');
    const pointer=style.cursor==='pointer'&&tag!=='body'&&tag!=='html';
    const delegateHint=el.hasAttribute('data-action')||el.hasAttribute('data-command')||el.hasAttribute('aria-haspopup');
    const isAction=interactiveTag||roleAction||inline||listener||framework||pointer||delegateHint;
    if(pointer&&!inputCandidate&&!interactiveTag&&!roleAction&&!inline&&!listener&&!framework&&el.parentElement&&getComputedStyle(el.parentElement).cursor==='pointer')continue;
    const textTag=/^h[1-6]$/.test(tag)||['p','li','label','th','td','pre','blockquote'].includes(tag);
    const leaf=(tag==='div'||tag==='span')&&el.children.length===0;
    if(!inputCandidate&&!isAction&&!textTag&&!leaf)continue;
    if(!inputCandidate&&!isAction&&el.parentElement?.closest('button,a,[role="button"],[role="link"],[contenteditable="true"],[contenteditable="plaintext-only"]'))continue;
    const sensitive=inputCandidate&&isSensitive(el);
    const kind=tag==='select'?'select':
      type==='checkbox'||type==='radio'||role==='checkbox'||role==='switch'||role==='radio'?'check':
      tag==='input'&&['submit','reset','button','image'].includes(type)?'action':
      inputCandidate?'input':isAction?'action':'text';
    const editor=customInput||tag==='textarea';
    let text=label(el,kind==='input');
    if(kind==='action'&&!text&&tag==='input')text=safeText(el.value,120);
    if(kind==='input'&&!text)text=sensitive?'Sensitive input':(editor?'Text editor':'Text input');
    if(kind==='action'&&!text)text='Interactive element';
    if(kind==='text'&&!text)continue;
    let id=el.getAttribute(idAttribute)||'';
    if(!/^td-sem-[0-9a-z]+$/.test(id)||usedIDs.has(id))id='td-sem-'+(++store.counter).toString(36);
    usedIDs.add(id);el.setAttribute(idAttribute,id);
    const item={id,kind,tag,role,text,disabled:el.disabled===true||el.getAttribute('aria-disabled')==='true'};
    if(kind==='action'&&tag==='a')item.href=el.href||'';
    if(kind==='input'){
      item.input_type=sensitive?'password':(nativeInput?type||'text':'text');
      item.sensitive=sensitive;
      item.multiline=tag==='textarea'||el.getAttribute('aria-multiline')==='true'||contentEditor||knownEditor;
      item.editor=Boolean(customInput);
      item.placeholder=el.getAttribute('placeholder')||el.getAttribute('data-placeholder')||'';
      const value=nativeInput?el.value:el.innerText||el.textContent||'';
      item.value=sensitive?'':String(value||'').slice(0,2000);
    }
    if(kind==='check'){
      item.input_type=type||role;
      item.checked=Boolean(el.checked||el.getAttribute('aria-checked')==='true');
    }
    if(kind==='select'){
      item.value=String(el.value||'');
      item.options=[...el.options].slice(0,75).map(o=>({value:String(o.value),text:safeText(o.text,120)}));
    }
    result.push(item);
  }
  return result;
}

// Instrument direct listeners before page scripts initialize. Framework event
// delegation can also be discovered through ARIA/CSS/attributes above.
export function installEventInstrumentation() {
  const original=EventTarget.prototype.addEventListener;
  const listeners=new WeakSet();
  Object.defineProperty(window,'__taskdeckInteractiveListeners',{value:listeners,configurable:false});
  EventTarget.prototype.addEventListener=function(type,...args) {
    if(['click','pointerdown','mousedown','touchstart','keydown'].includes(String(type).toLowerCase()))listeners.add(this);
    return Reflect.apply(original,this,[type,...args]);
  };
}
