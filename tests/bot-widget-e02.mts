import fs from 'node:fs';import os from 'node:os';import path from 'node:path';import {pathToFileURL} from 'node:url';import {execFileSync} from 'node:child_process';import assert from 'node:assert/strict';
import {measureButtons} from './renderer-layout.mts';
const root=process.env.WIDGET_SANDBOX;if(!root)throw Error('WIDGET_SANDBOX required');
const {runWidget}=await import(pathToFileURL(path.join(root,'packages/core/scripts/sandbox-node.ts')).href);
const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'homequest-e02-'));const html=fs.readFileSync('widgets/family.html','utf8');
try{for(const width of [200,320,390])for(const theme of ['light','dark','custom']){
 const db=path.join(tmp,`${width}-${theme}.db`);
 const call=(viewer:number,text='',action='',update_id=0)=>JSON.parse(execFileSync('./bin/bot-probe',[db],{input:JSON.stringify({viewer,text,action,update_id}),encoding:'utf8'}));
 const A=900001,B=900002,D=900004;
 call(A,'/start');call(A,'/create');call(A,'Тёплый дом');call(A,'Europe/Berlin');call(A,'/task');call(A,'Вынести мусор');call(A,'10');
 const home=call(A,'/start');const settings=call(A,'',home.state.buttons.find((b:any)=>b.label==='Настройки').action);
 const invitation=call(A,'',settings.state.buttons.find((b:any)=>b.label==='Пригласить').action);
 const token=invitation.payload.text.split('join_')[1];assert.ok(token);
 call(B,'/start join_'+token);
 const bhome=call(B,'/start');const tasks=call(B,'',bhome.state.buttons.find((b:any)=>b.label==='Сегодня').action);
 assert.equal(tasks.state.title,'Вынести мусор');assert.equal(tasks.balance,0);
 call(D,'/start');const denied=call(D,'',tasks.state.buttons[0].action);assert.equal(denied.balance,0);assert.equal(denied.family_id,0);
 const data={chatData:JSON.stringify(tasks.state),publicData:null};
 const layout=await measureButtons(runWidget,html,{width,data});assert.ok(layout.height<600);
 let changed:any;const step={data:{...data}};
 const first=layout.boxes.find((b:any)=>b.text==='Выполнено');assert.ok(first);
 const result=await runWidget(html,{width,layout:true,theme:theme==='dark'?'dark':'light',...(theme==='custom'?{themeParams:{bg_color:'#edf4e8',text_color:'#183020',button_color:'#345a40'}}:{}),context:{canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:B,first_name:'B'}},data,gestures:[{kind:'tap',x:first.x+30,y:first.y+20}],onRequest(request:any){changed=call(B,'',request.data);step.data.chatData=JSON.stringify(changed.state);return {result:null}},script:[step],probes:["document.getElementById('text').textContent"]});
 if(result.requests.length!==1)console.log(JSON.stringify({height:layout.height,gestures:result.gestures,diags:result.diags}));
 assert.deepEqual(result.diags,[]);assert.equal(result.requests.length,1);assert.equal(changed.balance,10);assert.equal(changed.week,10);assert.equal(changed.xp,10);assert.ok(JSON.stringify(result.probes).includes('Пока нет доступных заданий'));
 assert.equal(call(B,'',tasks.state.buttons[0].action,changed.update_id).balance,10);assert.equal(call(B,'',tasks.state.buttons[0].action).balance,10);
 console.log(`PASS E02 product handlers + SQLite + official renderer ${width}px ${theme}`);
}}finally{fs.rmSync(tmp,{recursive:true,force:true})}
