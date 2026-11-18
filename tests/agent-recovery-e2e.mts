// Positive and negative journeys; no domain success is seeded. Every request
// restarts bot-probe against the same temporary SQLite. Model/STT are fixtures.
import {AgentFlow} from './agent-flow.mts';
import assert from 'node:assert/strict';
import fs from 'node:fs';
const f=new AgentFlow(4184),results:any[]=[];
async function scenario(id:string,kind:'positive'|'negative'|'mixed',name:string,body:()=>Promise<void>){
 await body();results.push({id,kind,name,status:'PASS'});console.log(`PASS ${id} [${kind}] ${name}`);
}
const jobCount=()=>f.last.ai_jobs.length;
try{
 await f.start();await f.create();
 await scenario('REC01','mixed','Consent cancellation prevents creation; resubmission succeeds',async()=>{
  await f.raw({tick:true,agent_fixture:{operation:'shopping_add',items:[{name:'Хлеб',quantity:'1',note:''}]}});
  await f.send('Добавь хлеб');assert.equal(f.last.state.title,'Помощник по дому');await f.click('Отмена');
  await f.inject({tick:true,process_ai:true});assert.equal(f.last.shopping_needed,0);
  await f.request('shopping_add','Добавь хлеб',{items:[{name:'Хлеб',quantity:'1',note:''}]});await f.apply();assert.equal(f.last.shopping_needed,1);
 });
 await scenario('REC02','mixed','Duplicate Telegram update enqueues once and applies once',async()=>{
  await f.raw({tick:true,agent_fixture:{operation:'task_create',name:'Ручное дело',points:50}});
  await f.send('Создай ручное дело');const update=f.last.update_id,count=jobCount();
  await f.inject({update_id:update,text:'Создай ручное дело'});assert.equal(jobCount(),count);
  await f.inject({tick:true,process_ai:true});await f.preview();const confirm=f.token('Подтвердить план');await f.click('Подтвердить план');
  const applied=f.last.update_id;await f.inject({update_id:applied,action:confirm});assert.equal((f.last.tasks_open||[]).length,1);
 });
 for(const [id,fault] of [['REC03','failure'],['REC04','timeout'],['REC05','malformed']]){
  await scenario(id,'negative',`${fault}: no mutation and no stuck confirmation`,async()=>{
   const snapshot=JSON.stringify(f.last.tasks_open);
   await f.request('task_complete','Я сделал ручное дело',{search:'Ручное',[fault]:true});
   if (fault==='timeout') assert.match(f.last.payload.text,/не дождался ответа сервиса/);
   else f.denied();
   assert.equal(JSON.stringify(f.last.tasks_open),snapshot);assert.equal(f.last.balance,0);assert.equal(f.last.ai_jobs.at(-1).State,'failed');
   await f.send('Подтверждаю');await f.inject({tick:true,process_ai:true});assert.match(f.last.payload.text,/Нет актуального плана/);assert.equal(f.last.balance,0);
  });
 }
 await scenario('REC06','mixed','Buttons work during gateway failure; agent recovers',async()=>{
  await f.openTask('Ручное дело');await f.click('Выполнено');assert.equal(f.last.balance,50);
  await f.request('account_read','Сколько баллов?');assert.match(f.last.payload.text,/Баланс: 50/);
 });
 await scenario('REC07','negative','STT failure keeps purchases unchanged',async()=>{
  await f.raw({tick:true,agent_fixture:{operation:'shopping_buy',search:'Хлеб'}});
  await f.inject({voice:true});await f.inject({tick:true,process_ai:true,speech_failure:true});f.denied();assert.equal(f.last.shopping_bought,0);
 });
 await scenario('REC08','negative','Oversized/overlong voice never enters queue',async()=>{
  const before=jobCount();
  await f.inject({voice:true,voice_duration:61});assert.match(f.last.payload.text,/60 секунд/);assert.equal(jobCount(),before);
  await f.inject({voice:true,voice_duration:3,voice_size:4194305});assert.match(f.last.payload.text,/4 МБ/);assert.equal(jobCount(),before);
 });
 await scenario('REC09','positive','Boundary 60-second voice and voice confirmation work',async()=>{
  await f.raw({tick:true,speech:'Я купил хлеб',agent_fixture:{operation:'shopping_buy',search:'Хлеб'}});
  await f.inject({voice:true,voice_duration:60,voice_size:4194304});await f.inject({tick:true,process_ai:true});await f.preview();
  await f.raw({tick:true,speech:'Подтверждаю'});await f.inject({voice:true});await f.inject({tick:true,process_ai:true});assert.equal(f.last.shopping_bought,1);
 });
 await scenario('REC10','negative','Forged resource snapshot cannot produce a plan',async()=>{
  await f.request('shopping_reopen','Верни хлеб в список',{search:'Хлеб',forged_snapshot:true});f.denied();assert.equal(f.last.shopping_bought,1);
 });
 await scenario('REC11','mixed','Failed second batch operation rolls back first purchase',async()=>{
  await f.request('shopping_reopen','Верни хлеб в список',{search:'Хлеб'});await f.apply();
  await f.request('shopping_buy','Купил хлеб и добавь дело',{search:'Хлеб',extra_changes:[{operation:'task_create',name:'Неверное дело',points:0}]});f.denied();assert.equal(f.last.shopping_bought,0);assert.equal((f.last.tasks_open||[]).length,0);
  await f.request('shopping_buy','Я купил хлеб',{search:'Хлеб'});await f.apply();assert.equal(f.last.shopping_bought,1);
 });
 await scenario('REC12','mixed','Ten-change boundary succeeds; eleven changes write nothing',async()=>{
  const changes=Array.from({length:10},(_,i)=>({operation:'task_create',name:`Пакет ${i+1}`,points:1}));
  await f.request('fixture','Создай десять дел',{changes});assert.ok(!f.last.state.buttons.some((b:any)=>b.label==='Подтвердить план'));
  await f.apply();assert.equal((f.last.tasks_open||[]).length,10);
  await f.request('fixture','Создай одиннадцать дел',{changes:[...changes,{operation:'task_create',name:'Лишнее',points:1}]});f.denied();assert.equal((f.last.tasks_open||[]).length,10);
 });
 await scenario('REC13','mixed','Reward input boundaries and HTML-like names remain safe',async()=>{
  for(const points of [0,100001]){await f.request('reward_create','Добавь награду',{name:'Неверная',points});f.denied();assert.equal((f.last.reward_catalog||[]).length,0);}
  await f.request('reward_create','Добавь награду',{name:'<script>window.hacked=1</script>',points:1,description:'Я'.repeat(500)});await f.apply();assert.equal((f.last.reward_catalog||[]).length,1);
  assert.equal(await f.page.frameLocator('iframe').locator('body').evaluate(()=>Boolean((window as any).hacked)),false);
  await f.request('reward_create','Добавь награду',{name:'Большая',points:100000});await f.apply();
  await f.request('reward_request','Хочу большую награду',{search:'Большая'});f.denied();assert.equal(f.last.balance,50);
 });
 await scenario('REC14','mixed','Insufficient combined spend is atomic, single request succeeds',async()=>{
  for(const name of ['Вечер один','Вечер два']){await f.request('reward_create','Добавь награду',{name,points:30});await f.apply();}
  await f.request('reward_request','Хочу обе награды',{search:'Вечер'});f.denied();assert.equal(f.last.balance,50);assert.equal((f.last.redemptions||[]).length,0);
  await f.request('reward_request','Хочу вечер один',{search:'Вечер один'});await f.apply();assert.equal(f.last.balance,20);assert.equal((f.last.redemptions||[]).length,1);
 });
 await scenario('REC15','mixed','Competing reviewer plans invalidate stale decision; fresh rejection refunds once',async()=>{
  await f.as(900002);await f.request('redemption_approve','Прими заявку',{search:'Вечер один',status:'pending'});await f.preview();const approval=f.token('Подтвердить план');
  await f.as(900003);await f.request('redemption_reject','Отклони заявку',{search:'Вечер один',status:'pending'});await f.preview();const stale=f.token('Подтвердить план');
  await f.raw({viewer:900002,action:approval});await f.inject({action:stale});assert.match(f.last.payload.text,/данные|права/);assert.equal(f.last.redemptions[0].status,1);
  await f.request('redemption_reject','Отклони принятую заявку',{search:'Вечер один',status:'approved'});await f.preview();const refund=f.token('Подтвердить план');await f.click('Подтвердить план');await f.raw({action:refund});
  await f.as(900001);assert.equal(f.last.balance,50);assert.equal(f.last.xp,50);
 });
 await scenario('REC16','negative','Timezone change invalidates pending plan',async()=>{
  await f.request('task_due','Перенеси пакет один',{search:'Пакет 1',due:'2026-03-05 19:00'});await f.preview();const confirm=f.token('Подтвердить план');
  await f.home();await f.click('Настройки');await f.click('Управление');await f.click('Часовой пояс');await f.send('Asia/Tokyo');
  const before=JSON.stringify(f.last.tasks_open);await f.inject({action:confirm});assert.equal(JSON.stringify(f.last.tasks_open),before);assert.match(f.last.payload.text,/данные|права|Контекст/);
 });
 await scenario('REC17','mixed','Departed member cannot confirm; remaining family can buy',async()=>{
  await f.request('shopping_add','Добавь масло',{items:[{name:'Масло',quantity:'1',note:''}]});await f.apply();
  await f.as(900002);await f.request('shopping_claim','Масло куплю я',{search:'Масло'});await f.preview();const stale=f.token('Подтвердить план');
  await f.home();await f.click('Настройки');await f.click('Выйти из дома');await f.click('Подтвердить выход');await f.inject({action:stale});assert.equal(f.last.family_id,0);
  await f.as(900001);assert.equal(f.last.shopping_items.find((x:any)=>x.Name==='Масло').Status,'needed');
  await f.request('shopping_buy','Я купил масло',{search:'Масло'});await f.apply();assert.equal(f.last.shopping_bought,2);
 });
 await scenario('REC18','mixed','Explicit family switch invalidates old plan; original household is unchanged',async()=>{
  await f.request('task_due','Перенеси пакет два',{search:'Пакет 2',due:'2026-03-06 19:00'});await f.preview();const stale=f.token('Подтвердить план'),before=JSON.stringify(f.last.tasks_open),first=f.last.family_id;
  await f.as(900004);await f.click('Настройки');await f.click('Пригласить');const invite=f.last.payload.text.split('join_')[1],second=f.last.family_id;
  await f.as(900001);await f.send('/start join_'+invite);assert.equal(f.last.family_id,first);
  for(let n=0;n<10&&f.last.state.title!=='Другой дом';n++)await f.click('Следующий дом');assert.equal(f.last.state.title,'Другой дом');await f.click('Открыть этот дом');assert.equal(f.last.family_id,second);
  await f.inject({action:stale});assert.equal(f.last.family_id,second);assert.equal((f.last.tasks_open||[]).length,0);
  await f.home();await f.click('Мои дома');for(let n=0;n<10&&f.last.state.title!=='Тёплый дом';n++)await f.click('Следующий дом');assert.equal(f.last.state.title,'Тёплый дом');await f.click('Открыть этот дом');assert.equal(JSON.stringify(f.last.tasks_open),before);
 });
 await scenario('REC19','positive','Read and act on second resource page among 21 items',async()=>{
  for(let base=0;base<21;base+=10){const items=Array.from({length:Math.min(10,21-base)},(_,i)=>({name:`Страница ${String(base+i+1).padStart(2,'0')}`,quantity:'1',note:''}));await f.request('shopping_add','Добавь позиции', {items});await f.apply();}
  await f.request('shopping_read','Покажи следующую страницу',{search:'Страница',offset:20});assert.match(f.last.payload.text,/Найдено позиций: 1/);
  await f.request('shopping_buy','Я купил последнюю позицию',{search:'Страница',offset:20});await f.apply();assert.equal(f.last.shopping_items.find((x:any)=>x.Name==='Страница 21').Status,'bought');assert.equal(f.last.shopping_items.find((x:any)=>x.Name==='Страница 01').Status,'needed');
 });
 await scenario('REC20','negative','Unobserved existing ID and invalid page offsets are rejected',async()=>{
  const first=f.last.shopping_items.find((x:any)=>x.Name==='Страница 01').ID;
  await f.request('shopping_buy','Я купил позицию',{search:'Страница',offset:20,foreign_id:first});f.denied();assert.equal(f.last.shopping_items.find((x:any)=>x.ID===first).Status,'needed');
  for(const offset of [-1,22]){await f.request('shopping_read','Покажи страницу',{search:'Страница',offset});f.denied();}
 });
}finally{
 fs.writeFileSync('.tools/agent-recovery-cards.json',JSON.stringify(f.cards));
 fs.writeFileSync('.tools/agent-recovery-results.json',JSON.stringify(results,null,2));
 await f.close();
}
console.log(`PASS ${results.length} named recovery/boundary journeys; ${f.cards.length} HTML states`);
