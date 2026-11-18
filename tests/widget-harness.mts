import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=process.env.WIDGET_SANDBOX;
if(!root||!process.env.WIDGET_PROBE)throw Error('Set WIDGET_SANDBOX and WIDGET_PROBE');
const {runWidget}=await import(pathToFileURL(path.join(root,'packages/core/scripts/sandbox-node.ts')).href);
const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'homequest-widget-'));
try {
 const db=path.join(tmp,'probe.db');
 const call=(action='')=>JSON.parse(execFileSync(process.env.WIDGET_PROBE!,[db],{input:JSON.stringify({viewer:900001,action}),encoding:'utf8'}));
 const html=fs.readFileSync('widgets/home.html','utf8');
 for(const width of [200,320,390])for(const theme of ['light','dark','custom']){
  const initial=call();let updated=initial;
  const data={chatData:JSON.stringify(initial),publicData:null};
  const step={data:{...data}};
  const layout=await runWidget(html,{width,layout:true,data});
  assert.ok(layout.height<600);
  const result=await runWidget(html,{
   width,layout:true,theme:theme==='dark'?'dark':'light',
   ...(theme==='custom'?{themeParams:{bg_color:'#edf4e8',text_color:'#183020',button_color:'#345a40'}}:{}),
   context:{canQuery:true,backgroundQuery:true,chatType:'private',viewer:{id:900001,first_name:'A'}},
   data,gestures:[{kind:'tap',x:80,y:layout.height-45}],
   onRequest(request){updated=call(request.data);step.data.chatData=JSON.stringify(updated);return {result:null}},
   script:[step],probes:["document.getElementById('status').textContent"]
  });
  assert.deepEqual(result.diags,[]);
  if(result.requests.length!==1)console.log(JSON.stringify({width,height:result.height,gestures:result.gestures}));
  assert.equal(result.requests.length,1);
  assert.equal(result.requests[0].verdict,'foreground');
  assert.equal(updated.status,'Подключение работает');
  assert.ok(JSON.stringify(result.probes).includes('Подключение работает'));
  assert.deepEqual(call(initial.action),updated);
  console.log(`PASS QuickJS + renderer + Go + SQLite + state: ${width}px ${theme}`);
 }
}finally{fs.rmSync(tmp,{recursive:true,force:true})}
