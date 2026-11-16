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
