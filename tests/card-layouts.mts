// Real product projections → official QuickJS validator → official renderer.
import fs from 'node:fs';import path from 'node:path';import {pathToFileURL} from 'node:url';import {createRequire} from 'node:module';import assert from 'node:assert/strict';
const root=process.env.WIDGET_SANDBOX!,input=process.env.HQ_WIDGET_CARDS_OUT!;if(!root||!input)throw Error('Set WIDGET_SANDBOX and HQ_WIDGET_CARDS_OUT');
const {runWidget}=await import(pathToFileURL(path.join(root,'packages/core/scripts/sandbox-node.ts')).href);
const require=createRequire(path.join(root,'package.json'));const {chromium}=require('@playwright/test');
const launch=chromium.launch.bind(chromium);let screenshot='';
chromium.launch=async(...args:any[])=>{const browser=await launch(...args),close=browser.close.bind(browser);browser.close=async()=>{if(screenshot){for(const context of browser.contexts())for(const page of context.pages())await page.screenshot({path:screenshot});screenshot=''}return close()};return browser};
const cards=JSON.parse(fs.readFileSync(input,'utf8'));const html=fs.readFileSync('widgets/family.html','utf8');
fs.mkdirSync('docs/screenshots',{recursive:true});let count=0;
const extra=process.env.HQ_FEATURE_CARDS_OUT?JSON.parse(fs.readFileSync(process.env.HQ_FEATURE_CARDS_OUT,'utf8')):{};
const all={...cards,...Object.fromEntries(Object.entries(extra).map(([k,v])=>['feature:'+k,v]))};
for(const [title,card] of Object.entries(all)){
 assert.ok(Buffer.byteLength(JSON.stringify(card))<=1024,title+' state budget');
 // Each product screen at the narrowest profile width; themes share the same template.
 const r=await runWidget(html,{width:200,layout:true,data:{chatData:JSON.stringify(card),publicData:null}});
 assert.deepEqual(r.diags,[],title);assert.ok(r.height<=600,`${title}: height ${r.height}`);count++;
}
for(const theme of ['light','dark','custom']){
 screenshot=`docs/screenshots/home-${theme}.png`;
 const r=await runWidget(html,{width:320,layout:true,theme:theme==='dark'?'dark':'light',...(theme==='custom'?{themeParams:{bg_color:'#edf4e8',text_color:'#183020',button_color:'#345a40'}}:{}),context:{canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:900001,first_name:'A'}},data:{chatData:JSON.stringify(cards['Тёплый дом']),publicData:null}});
 assert.deepEqual(r.diags,[]);assert.ok(r.height<=600);
}
// Legal long Russian strings and a larger client font must keep actions reachable.
for(const [width,font] of [[200,15],[200,20],[320,20],[390,20]]){
 const long={title:'Дом близких людей, в котором всем спокойно и хорошо',text:'Часовой пояс: America/Argentina/Buenos_Aires\nУчастников: 12\nДела по дому — общий вклад в уют.',buttons:[{label:'Сегодня',action:'a'.repeat(32)},{label:'Добавить задание',action:'b'.repeat(32)},{label:'Мои баллы',action:'c'.repeat(32)},{label:'Подарки',action:'d'.repeat(32)},{label:'Настройки',action:'e'.repeat(32)},{label:'Мои дома',action:'f'.repeat(32)}]};
 const r=await runWidget(html,{width,layout:true,text:{fontSize:font},data:{chatData:JSON.stringify(long),publicData:null}});
 assert.deepEqual(r.diags,[]);assert.ok(r.height<=600,`long Russian ${width}px font ${font}: ${r.height}`);
}
for(const title of ['Проверьте желание','Напоминания дома']){
 assert.ok(extra[title],title);screenshot=`docs/screenshots/${title==='Проверьте желание'?'wish-review':'reminder-plan'}.png`;
 const r=await runWidget(html,{width:320,layout:true,data:{chatData:JSON.stringify(extra[title]),publicData:null}});assert.deepEqual(r.diags,[]);assert.ok(r.height<=600);
}
console.log(`PASS ${count} product cards at 200px; 5 official-renderer screenshots`);
