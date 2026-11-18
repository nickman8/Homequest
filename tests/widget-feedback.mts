// A real SDK acknowledgement is not a state update. The UI must recover from both.
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
import {measureButtons} from './renderer-layout.mts';
const root=process.env.WIDGET_SANDBOX!;
const {runWidget}=await import(pathToFileURL(path.join(root,'packages/core/scripts/sandbox-node.ts')).href);
const html=fs.readFileSync('widgets/family.html','utf8');
const data={chatData:JSON.stringify({title:'Сегодня',text:'Вынести мусор',buttons:[{label:'Выполнено',action:'a'.repeat(32)}]}),publicData:null};
const context={canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:900001,first_name:'A'}};
const position=await measureButtons(runWidget,html,{width:320,data});
const rect=position.boxes[0];
for(const outcome of ['ack','RATE_LIMITED','TIMEOUT','FAILED']){
 const r=await runWidget(html,{width:320,layout:true,data,context,gestures:[{kind:'tap',x:rect.x+30,y:rect.y+20}],onRequest(){if(outcome!=='ack')throw outcome;return {result:null}},probes:["JSON.stringify({disabled:document.querySelector('button').disabled,text:document.getElementById('feedback')?.textContent})"]});
 assert.deepEqual(r.diags,[]);assert.equal(r.requests.length,1);
 const view=JSON.parse(r.probes[0]);assert.equal(view.disabled,false,`${outcome}: button stays disabled`);assert.ok(view.text,`${outcome}: missing feedback`);
}
console.log('PASS official runtime: ACK without state, rate limit, timeout, failure');
const paged={chatData:JSON.stringify({title:'Дом',text:'Вместе',buttons:['Сегодня','Добавить','Баллы','Подарки','Настройки','Мои дома'].map((label,i)=>({label,action:String(i).repeat(32)}))}),publicData:null};
const narrow=await measureButtons(runWidget,html,{width:200,data:paged});
const next=narrow.boxes.find((b:any)=>b.text.startsWith('Ещё действия'));assert.ok(next,'narrow pagination missing');
const r=await runWidget(html,{width:200,layout:true,data:paged,context,gestures:[{kind:'tap',x:next.x+30,y:next.y+20}],onRequest(){throw Error('pagination must stay local')},probes:["document.getElementById('buttons').textContent"]});
assert.deepEqual(r.diags,[]);assert.equal(r.requests.length,0);assert.ok(r.probes[0].includes('Настройки'));
console.log('PASS narrow card: all actions reachable with local pagination');
const longState=JSON.parse(paged.chatData);longState.text='Описание '.repeat(8)+'Конец описания';
const longData={chatData:JSON.stringify(longState),publicData:null};
const textLayout=await measureButtons(runWidget,html,{width:200,data:longData});
const more=textLayout.boxes.find((b:any)=>b.text.startsWith('Далее'));assert.ok(more,'text pagination missing');
const textResult=await runWidget(html,{width:200,layout:true,data:longData,context,gestures:[{kind:'tap',x:more.x+30,y:more.y+20}],probes:["document.getElementById('text').textContent"]});
assert.deepEqual(textResult.diags,[]);assert.equal(textResult.requests.length,0);assert.ok(textResult.probes[0].includes('Конец описания'));
console.log('PASS long text: full content remains reachable without a host request');
