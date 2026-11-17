// Independently reported defects reproduced through HTML, Eino tools and SQLite.
import {AgentFlow} from './agent-flow.mts';
import assert from 'node:assert/strict';
import fs from 'node:fs';
const cards:any[]=[],results:any[]=[];
async function scenario(id:string,name:string,body:(f:AgentFlow)=>Promise<void>){
 if(process.env.REVIEW_CASE && process.env.REVIEW_CASE!==id)return;
 const f=new AgentFlow(4183);
 try{await f.start();await f.create();await body(f);results.push({id,name,status:'PASS'});console.log(`PASS ${id}: ${name}`);}
 catch(e){results.push({id,name,status:'FAIL',error:String(e)});console.error(`FAIL ${id}: ${String(e)}`);}
 finally{cards.push(...f.cards);await f.close();}
}
await scenario('REV01','Late cancel preserves applied job; real cancellation prevents effects',async f=>{
 await f.request('reward_create','Добавь награду фильм за 10 баллов',{name:'Фильм',points:10});await f.preview();
 const cancel=f.token('Отмена');await f.click('Подтвердить план');const id=f.last.ai_jobs.at(-1).ID;
 await f.inject({action:cancel});assert.equal(f.last.ai_jobs.find((j:any)=>j.ID===id).State,'done');assert.match(f.last.payload.text,/уже примен|уже заверш/i);assert.equal(f.last.reward_catalog.length,1);
 await f.request('reward_create','Добавь награду прогулка',{name:'Прогулка',points:5});await f.preview();const confirm=f.token('Подтвердить план');await f.click('Отмена');
 assert.match(f.last.payload.text,/отмен/i);assert.equal(f.last.ai_jobs.at(-1).State,'cancelled');await f.inject({action:confirm});assert.equal(f.last.reward_catalog.length,1);
});
await scenario('REV02','Two photo completions require explicit named selection, survive cancel/restart',async f=>{
 for(const name of ['Первый отчёт','Второй отчёт']){await f.request('task_create','Создай дело с фото',{name,points:10,photo:true});await f.apply();}
 await f.request('task_complete','Я сделал оба отчёта',{search:'отчёт'});await f.apply();assert.equal(f.last.attempts.filter((a:any)=>a.status==='photo').length,2);
 // Unselected photo may not silently attach to the last item in the batch.
 await f.inject({photo:'unselected-photo'});assert.equal(f.last.attempts.filter((a:any)=>a.has_photo).length,0);
 await f.reports();assert.match(f.last.state.text,/Первый отчёт/);await f.click('Добавить фото');assert.match(f.last.payload.text,/Первый отчёт/);
 await f.send('/cancel');await f.inject({photo:'cancelled-photo'});assert.equal(f.last.attempts.filter((a:any)=>a.has_photo).length,0);
 await f.reports();const next=f.token('Следующий отчёт');await f.click('Добавить фото');await f.inject({action:next});assert.match(f.last.state.text,/Второй отчёт/);await f.inject({photo:'browsed-without-selection'});assert.equal(f.last.attempts.filter((a:any)=>a.has_photo).length,0);await f.reports();await f.click('Следующий отчёт');await f.click('Добавить фото');
 await f.inject({photo:'second-report-photo'});assert.equal(f.last.balance,10);assert.equal(f.last.attempts[0].has_photo,false);assert.equal(f.last.attempts[1].has_photo,true);
 await f.reports();assert.match(f.last.state.text,/Первый отчёт/);await f.click('Добавить фото');await f.inject({photo:'first-report-photo'});assert.equal(f.last.balance,20);
});
await scenario('REV03','Reviewer cannot become assignee; changing reviewer restores valid assignment',async f=>{
 const owner=f.last.members.find((m:any)=>m.Owner).ID, members=f.last.members.filter((m:any)=>!m.Owner).map((m:any)=>m.ID);
 await f.request('task_create','Создай дело с проверяющим',{name:'Проверяемое дело',points:10,assigned_to:members[0],reviewer:owner,approval:true});await f.apply();
 await f.request('task_assign','Назначь владельцу это дело',{search:'Проверяемое',target:owner});f.denied();assert.equal(f.last.tasks_open[0].AssignedTo,members[0]);
 // The ordinary button path must enforce the same domain rule.
 await f.openTask('Проверяемое дело');await f.click('Параметры');await f.click('Изменить');await f.click('Исполнитель');
 await f.click('Назначить');assert.match(f.last.payload.text,/проверяющ|разными/);assert.equal(f.last.tasks_open[0].AssignedTo,members[0]);
 await f.home();await f.request('task_reviewer','Назначь третьего участника проверяющим',{search:'Проверяемое',target:members[1]});await f.apply();
 await f.request('task_assign','Назначь владельцу это дело',{search:'Проверяемое',target:owner});await f.apply();assert.equal(f.last.tasks_open[0].AssignedTo,owner);
 await f.request('task_complete','Я сделал проверяемое дело',{search:'Проверяемое'});await f.apply();assert.equal(f.last.balance,0);assert.equal(f.last.attempts[0].status,'review');
 await f.as(900003);await f.reports();await f.click('Принять');await f.as(900001);assert.equal(f.last.balance,10);
});
await scenario('REV04','All preview text precedes next/confirm at narrow and normal widths',async f=>{
 for(const width of ['200','320']){
  await f.request('task_create','Создай дело с подробным описанием',{name:'Подробное дело',points:10,description:'Не пропусти эту важную часть описания. '.repeat(8)});
  await f.page.locator('#width').selectOption(width);
  const frame=f.page.frameLocator('iframe');
  for(let part=0;part<20;part++){
   const text=frame.locator('#text');await text.waitFor();
   for(let local=0;local<15;local++){
    const value=await text.textContent();
    if(!value?.endsWith('…'))break;
    assert.equal(await frame.getByRole('button',{name:'Следующая часть',exact:true}).count(),0,'server next skips unread text');
    assert.equal(await frame.getByRole('button',{name:'Подтвердить план',exact:true}).count(),0,'confirmation precedes unread text');
    assert.equal(await frame.getByRole('button',{name:'Отмена',exact:true}).count(),1,'cancel must stay available');
    await frame.getByRole('button',{name:/^Далее ·/}).click();
   }
   if(f.last.state.buttons.some((b:any)=>b.label==='Подтвердить план')){await f.click('Подтвердить план');break;}
   await f.click('Следующая часть');
  }
  assert.equal(f.last.state.title,'План применён');
 }
 assert.equal(f.last.tasks_open.length,2);
});
fs.writeFileSync('.tools/agent-review-cards.json',JSON.stringify(cards));
fs.writeFileSync('.tools/agent-review-results.json',JSON.stringify(results,null,2));
assert.equal(results.filter(r=>r.status==='FAIL').length,0,JSON.stringify(results));
