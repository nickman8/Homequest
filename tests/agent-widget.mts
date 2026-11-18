// Linked official QuickJS/DOM gesture -> real Eino/bot -> SQLite -> new rendering.
import fs from 'node:fs';import os from 'node:os';import path from 'node:path';import {pathToFileURL} from 'node:url';import {execFileSync} from 'node:child_process';import assert from 'node:assert/strict';
import {reachButton} from './renderer-layout.mts';
const root=process.env.WIDGET_SANDBOX!;
const {runWidget}=await import(pathToFileURL(path.join(root,'packages/core/scripts/sandbox-node.ts')).href);
const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'homequest-agent-linked-'));
const html=fs.readFileSync('widgets/family.html','utf8');
try{for(const width of [200,320,390]){
 const db=path.join(tmp,`${width}.db`);let fixture:any={operation:'task_create',name:'Протереть стол',points:10};let last:any;
 const call=(input:any)=>last=JSON.parse(execFileSync('./bin/bot-probe',[db],{input:JSON.stringify({viewer:900001,now:'2026-03-01T12:00:00Z',agent_fixture:fixture,...input}),encoding:'utf8',stdio:['pipe','pipe','pipe']}));
 const send=(text:string)=>call({text});
 const click=(label:string)=>call({action:last.state.buttons.find((b:any)=>b.label===label).action});
 send('/start');click('Создать дом');send('Тёплый дом');send('Europe/Berlin');click('Домашние планы');click('Комнаты');click('Добавить комнату');send('Кухня');click('Дела комнаты');
 send('Добавь дело протереть стол за 10 баллов');click('Понятно, продолжить');call({tick:true,process_ai:true});
 const gesture=async()=>{
  while(!last.state.buttons.some((b:any)=>b.label==='Подтвердить план'))click('Следующая часть');
  const state=last.state;const data={chatData:JSON.stringify(state),publicData:null};
  const {button,gestures}=await reachButton(runWidget,html,{width,data},'Подтвердить план');
  let changed:any;const step={data:{...data}};
  const result=await runWidget(html,{width,layout:true,theme:'dark',context:{canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:900001,first_name:'A'}},data,gestures:[...gestures,{kind:'tap',x:button.x+20,y:button.y+20}],onRequest(request:any){assert.equal(Buffer.byteLength(request.data),32);changed=call({action:request.data});step.data.chatData=JSON.stringify(changed.state);return {result:null}},script:[step],probes:["document.getElementById('title').textContent"]});
  assert.deepEqual(result.diags,[]);assert.equal(result.requests.length,1);assert.equal(changed.state.title,'План применён');
  const repeated=call({action:state.buttons.find((b:any)=>b.label==='Подтвердить план').action});assert.deepEqual(repeated.tasks_open,changed.tasks_open);assert.equal(repeated.balance,changed.balance);assert.equal(repeated.shopping_bought,changed.shopping_bought);
  return changed;
 };
 assert.equal((last.tasks_open||[]).length,0);const created=await gesture();assert.equal(created.tasks_open.length,1);
 fixture={operation:'task_due',due:'2026-03-02 19:00'};
 send('Перенеси дело здесь на завтра в 19:00');call({tick:true,process_ai:true});assert.equal(last.tasks_open[0].DueAt,null);
 const updated=await gesture();assert.equal(updated.tasks_open[0].DueAt,'2026-03-02T18:00:00Z');assert.equal(updated.balance,0);
 fixture={operation:'shopping_add',items:[{name:'Молоко',quantity:'2 л',note:''}]};
 send('Добавь молоко в покупки');call({tick:true,process_ai:true});assert.equal(last.shopping_needed,0);assert.equal((await gesture()).shopping_needed,1);
 fixture={operation:'shopping_buy',search:'Молоко'};
 send('Я купил молоко');call({tick:true,process_ai:true});assert.equal(last.shopping_bought,0);assert.equal((await gesture()).shopping_bought,1);
 fixture={operation:'task_complete',search:'Протереть стол'};
 send('Я протёр стол');call({tick:true,process_ai:true});assert.equal((await gesture()).balance,10);
 fixture={operation:'reward_create',name:'Чай',points:5};
 send('Добавь награду чай за пять баллов');call({tick:true,process_ai:true});await gesture();
 fixture={operation:'reward_request',search:'Чай'};
 send('Хочу чай');call({tick:true,process_ai:true});assert.equal(last.balance,10);const redeemed=await gesture();assert.equal(redeemed.balance,5);assert.equal(redeemed.xp,10);
 console.log(`PASS linked official agent create/room/due/shopping/reward/confirmation/replay ${width}px dark`);
}}finally{fs.rmSync(tmp,{recursive:true,force:true})}
