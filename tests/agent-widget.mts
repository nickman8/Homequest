// Linked official QuickJS/DOM gesture -> real Eino/bot -> SQLite -> new rendering.
import fs from 'node:fs';import os from 'node:os';import path from 'node:path';import {pathToFileURL} from 'node:url';import {execFileSync} from 'node:child_process';import assert from 'node:assert/strict';
import {measureButtons} from './renderer-layout.mts';
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
  const layout=await measureButtons(runWidget,html,{width,data});
  const button=layout.boxes.find((b:any)=>b.text==='Подтвердить план');assert.ok(button);
  let changed:any;const step={data:{...data}};
  const result=await runWidget(html,{width,layout:true,theme:'dark',context:{canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:900001,first_name:'A'}},data,gestures:[{kind:'tap',x:button.x+20,y:button.y+20}],onRequest(request:any){assert.equal(Buffer.byteLength(request.data),32);changed=call({action:request.data});step.data.chatData=JSON.stringify(changed.state);return {result:null}},script:[step],probes:["document.getElementById('title').textContent"]});
  assert.deepEqual(result.diags,[]);assert.equal(result.requests.length,1);assert.equal(changed.state.title,'План применён');
  const repeated=call({action:state.buttons.find((b:any)=>b.label==='Подтвердить план').action});assert.equal(repeated.tasks_open.length,1);
  return changed;
 };
 assert.equal((last.tasks_open||[]).length,0);const created=await gesture();assert.equal(created.tasks_open.length,1);
 fixture={operation:'task_due',due:'2026-03-02 19:00'};
 send('Перенеси дело здесь на завтра в 19:00');call({tick:true,process_ai:true});assert.equal(last.tasks_open[0].DueAt,null);
 const updated=await gesture();assert.equal(updated.tasks_open[0].DueAt,'2026-03-02T18:00:00Z');assert.equal(updated.balance,0);
 console.log(`PASS linked official agent create/room/due/confirmation/replay ${width}px dark`);
}}finally{fs.rmSync(tmp,{recursive:true,force:true})}
