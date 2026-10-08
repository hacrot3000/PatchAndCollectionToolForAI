'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const script=fs.readFileSync(path.join(__dirname,'../featuremods/contextviewport.js'),'utf8');
const document={
  head:{append(){}},
  createElement(){return {textContent:''};}
};
const events={},visualEvents={};
const dims={width:1280,height:720,offsetLeft:0,offsetTop:0,
  addEventListener(name,handler){visualEvents[name]=handler;}
};
const window={
  innerWidth:1280,innerHeight:720,
  visualViewport:dims,
  addEventListener(name,handler){events[name]=handler;},
  getComputedStyle(){return {minWidth:'230px',display:'block'};}
};
const globalThis={};
vm.runInNewContext(script,{document,window,globalThis});
const api=globalThis.TaskMenuContextViewport;
assert.equal(typeof api?.place,'function');

function node(naturalWidth,naturalHeight){
  return {isConnected:true,style:{},
    getBoundingClientRect(){
      return {
        width:Math.min(naturalWidth,parseFloat(this.style.maxWidth)||naturalWidth),
        height:Math.min(naturalHeight,parseFloat(this.style.maxHeight)||naturalHeight)
      };
    }
  };
}
function fit(name,{width,height,offsetLeft=0,offsetTop=0,x,y,menuWidth,menuHeight,anchor=null,submenu=false}){
  Object.assign(dims,{width,height,offsetLeft,offsetTop});
  const el=node(menuWidth,menuHeight);
  const placed=api.place(el,x,y,{anchor,submenu});
  assert.ok(placed,name+' did not return a position');
  const box=api.viewport();
  assert.ok(placed.left>=box.left+6-0.01,name+' left boundary');
  assert.ok(placed.top>=box.top+6-0.01,name+' top boundary');
  assert.ok(placed.left+placed.width<=box.right-6+0.01,name+' right boundary');
  assert.ok(placed.top+placed.height<=box.bottom-6+0.01,name+' bottom boundary');
  assert.equal(el.style.overflowY,'auto',name+' vertical scrolling enabled');
  assert.ok(parseFloat(el.style.maxHeight)<=height-12,name+' height capped');
  assert.ok(parseFloat(el.style.minWidth)<=width-12,name+' css min-width capped');
  if(submenu&&anchor&&anchor.right+4+placed.width>box.right-6){
    assert.ok(placed.left<anchor.left,name+' submenu flips left');
  }
  return placed;
}
fit('bottom-right',{width:1280,height:720,x:1275,y:715,menuWidth:300,menuHeight:520});
fit('top-left',{width:1280,height:720,x:0,y:0,menuWidth:300,menuHeight:520});
fit('bottom-left',{width:1280,height:720,x:3,y:715,menuWidth:280,menuHeight:600});
fit('top-right',{width:1280,height:720,x:1275,y:2,menuWidth:280,menuHeight:200});
fit('tall menu scroll',{width:420,height:220,x:300,y:205,menuWidth:300,menuHeight:1300});
fit('very narrow',{width:110,height:88,x:107,y:82,menuWidth:400,menuHeight:800});
fit('visual viewport zoom/pan',{width:580,height:320,offsetLeft:230,offsetTop:75,x:800,y:370,menuWidth:370,menuHeight:760});
fit('submenu flips left',{width:1000,height:600,x:980,y:560,menuWidth:340,menuHeight:420,anchor:{left:910,right:980,top:545,bottom:574},submenu:true});
fit('submenu below near left',{width:1000,height:600,x:45,y:55,menuWidth:260,menuHeight:180,anchor:{left:15,right:45,top:52,bottom:73},submenu:true});
const upward=fit('dropdown prefers above',{width:1000,height:600,x:720,y:560,menuWidth:280,menuHeight:220,anchor:{left:650,right:720,top:440,bottom:565}});
assert.ok(upward.top<440,'dropdown should flip above anchor when it fits');
const live=node(320,900);
Object.assign(dims,{width:1280,height:720,offsetLeft:0,offsetTop:0});
api.place(live,1200,690);
Object.assign(dims,{width:300,height:190,offsetLeft:0,offsetTop:0});
assert.equal(typeof events.resize,'function','window resize listener registered');
events.resize();
const left=parseFloat(live.style.left),top=parseFloat(live.style.top),rect=live.getBoundingClientRect();
assert.ok(left>=6&&top>=6&&left+rect.width<=294&&top+rect.height<=184,'resize must refit the open menu');
assert.equal(typeof visualEvents.resize,'function','visual viewport resize listener registered');
assert.equal(typeof visualEvents.scroll,'function','visual viewport scroll listener registered');
visualEvents.resize();visualEvents.scroll();
console.log('PASS: refitting already-open menus after resize and visual viewport changes');
console.log('PASS: 10 menu geometry scenarios including all corners, oversized scroll, zoom and nested submenus');
