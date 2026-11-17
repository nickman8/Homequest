// Measure the actual official renderer, never canned QuickJS layout queries.
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(path.join(process.env.WIDGET_SANDBOX!,'package.json'));
const {chromium}=require('@playwright/test');
export async function measureButtons(runWidget:any,html:string,options:any){
 const launch=chromium.launch;let boxes:any[]=[];
 chromium.launch=async function(...args:any[]){
  const browser=await launch.apply(chromium,args),close=browser.close.bind(browser);
  browser.close=async()=>{
   for(const context of browser.contexts())for(const page of context.pages())
    boxes=await page.locator('button').evaluateAll((nodes:any[])=>nodes.map(n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,text:n.textContent}}));
   return close();
  };return browser;
 };
 try{const result=await runWidget(html,{...options,layout:true});return {...result,boxes}}finally{chromium.launch=launch}
}

// Reach an action with real local pagination gestures in the official renderer.
export async function reachButton(runWidget:any,html:string,options:any,label:string){
 const gestures:any[]=[];
 for(let n=0;n<20;n++){
  const layout=await measureButtons(runWidget,html,{...options,gestures});
  const button=layout.boxes.find((b:any)=>b.text===label);
  if(button)return {button,gestures,layout};
  const next=layout.boxes.find((b:any)=>/^(Далее|Ещё действия) ·/.test(b.text));
  if(!next)throw Error('Unreachable action: '+label);
  gestures.push({kind:'tap',x:next.x+20,y:next.y+20});
 }
 throw Error('Local pagination exceeded its bound');
}
