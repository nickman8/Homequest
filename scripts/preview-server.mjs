// Local development only: synthetic identities + temporary SQLite, no bot token.
import http from 'node:http';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {execFile} from 'node:child_process';
const dir=fs.mkdtempSync(path.join(os.tmpdir(),'homequest-preview-'));
const port=Number(process.env.HOMEQUEST_PREVIEW_PORT||4173);
const origin=`http://127.0.0.1:${port}`;
let chain=Promise.resolve(),clock=process.env.HOMEQUEST_PREVIEW_NOW||'',aiCommand=null,speech='';
const files={'/':['preview/index.html','text/html; charset=utf-8'],'/app.js':['preview/app.js','text/javascript'],'/style.css':['preview/style.css','text/css'],'/widget.html':['widgets/family.html','text/html; charset=utf-8']};
const server=http.createServer(async(req,res)=>{
 res.setHeader('Cache-Control','no-store');
 res.setHeader('X-Content-Type-Options','nosniff');
 if(req.headers.host!==`127.0.0.1:${port}`){res.writeHead(403);res.end();return}
 if(req.method==='GET'&&files[req.url]){
  const [file,type]=files[req.url];res.setHeader('Content-Type',type);res.end(fs.readFileSync(file));return;
 }
 if(req.method!=='POST'||req.url!=='/update'||req.headers.origin!==origin||!req.headers['content-type']?.startsWith('application/json')){res.writeHead(403);res.end();return}
 try{
  let raw='';for await(const chunk of req){raw+=chunk;if(Buffer.byteLength(raw)>8192)throw Error('size')}
  const input=JSON.parse(raw);
  if(![900001,900002,900003,900004].includes(input.viewer)||typeof(input.text??'')!=='string'||typeof(input.action??'')!=='string')throw Error('input');
  if(input.ai_command)aiCommand=input.ai_command;if(input.speech)speech=input.speech;
  if(input.now){if(typeof input.now!=='string'||!Number.isFinite(Date.parse(input.now)))throw Error('clock');clock=input.now}
  // A bounded subprocess; no production environment variables are inherited.
  const invoke=()=>new Promise((resolve,reject)=>{
   const child=execFile('./bin/bot-probe',[path.join(dir,'demo.db')],{maxBuffer:1<<20,timeout:15000,env:{PATH:process.env.PATH}},(err,stdout)=>err?reject(Error('handler')):resolve(stdout));
   child.stdin.end(JSON.stringify({viewer:input.viewer,text:input.text||'',action:input.action||'',photo:input.photo||'',now:clock,tick:input.tick===true,ai_command:aiCommand,speech,voice:input.voice===true,process_ai:input.process_ai===true,update_id:input.update_id||0,billing_mode:input.billing_mode||''}));
  });
  const pending=chain.then(invoke);chain=pending.catch(()=>{});
  const result=await pending;
  res.setHeader('Content-Type','application/json');res.end(result);
 }catch{res.writeHead(400,{'Content-Type':'application/json'});res.end('{"error":"Не удалось обработать действие"}')}
});
server.listen(port,'127.0.0.1',()=>console.log(`HomeQuest preview: ${origin} (synthetic users, temporary database, no Telegram)`));
function stop(){server.close(()=>{fs.rmSync(dir,{recursive:true,force:true});process.exit(0)})}
process.on('SIGTERM',stop);process.on('SIGINT',stop);
