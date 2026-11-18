// Real HTML cards and confirmation controls -> bot updates -> SQLite.
// Telegram reply metadata, LLM and STT are controlled test adapters.
import {AgentFlow} from './agent-flow.mts';
import assert from 'node:assert/strict';
import fs from 'node:fs';
const f=new AgentFlow(4196),results:any[]=[];
const actions=(card=f.last.state)=>card.buttons.map((b:any)=>b.action);
const jobs=()=>f.last.ai_jobs.length;
async function scenario(id:string,name:string,fn:()=>Promise<void>){await fn();results.push({id,name,status:'PASS'});console.log(`PASS ${id} ${name}`);}
async function rewards(name:string){
 await f.home();await f.click('Мои баллы');await f.click('Награды');
 for(let i=0;i<20&&f.last.state.title!==name;i++)await f.click('Следующая');
 assert.equal(f.last.state.title,name);
 await f.page.frameLocator('iframe').getByText(name,{exact:true}).waitFor();
 return actions();
}
async function reply(tokens:string[],text:string,fixture:any,voice=false){
 await f.raw({tick:true,agent_fixture:fixture,speech:text});
 await f.inject({text:voice?'':text,voice,reply_actions:tokens});
 if(f.last.state?.buttons.some((b:any)=>b.label==='Понятно, продолжить'))await f.click('Понятно, продолжить');
 await f.inject({tick:true,process_ai:true});
}
try{
 await f.start();await f.create();
 await f.request('reward_create','Добавь семейное кино',{name:'Кино',points:10,description:'Вечер вместе'});await f.apply();
 await f.request('reward_create','Добавь семейный чай',{name:'Чай',points:5,description:'Спокойный вечер'});await f.apply();
 await scenario('REP01','Reply to reward card binds resource, previews and edits only named target',async()=>{
  const tokens=await rewards('Кино');
  await reply(tokens,'Назови это «Семейное кино», цена 12 баллов',{operation:'reward_edit',search:'Кино',name:'Семейное кино',points:12,description:'Вечер вместе'});
  assert.equal(f.last.ai_jobs.at(-1).ReplyTarget.kind,'reward');
  assert.match(await f.apply(),/Семейное кино/);
  assert.equal(f.last.reward_catalog.find((r:any)=>r.name==='Семейное кино').pointsCost,12);
  assert.equal(f.last.reward_catalog.find((r:any)=>r.name==='Чай').pointsCost,5);
 });
 await scenario('REP02','Model selects a different valid resource: rejected, neither reward changes',async()=>{
  const tokens=await rewards('Семейное кино'),before=JSON.stringify(f.last.reward_catalog);
  await reply(tokens,'Измени только эту награду',{operation:'reward_edit',search:'Чай',name:'Подмена',points:30});
  f.denied();assert.equal(JSON.stringify(f.last.reward_catalog),before);assert.equal(f.last.ai_jobs.at(-1).State,'failed');
 });
 await scenario('REP03','Two target IDs in a reply require clarification before enqueue',async()=>{
  const first=await rewards('Семейное кино'),second=await rewards('Чай'),count=jobs();
  await f.inject({text:'Измени это',reply_actions:[...first,...second]});
  assert.match(f.last.payload.text,/несколько объектов/);assert.equal(jobs(),count);
 });
 await scenario('REP04','Another family member cannot reuse a personal reply card',async()=>{
  const tokens=await rewards('Чай');await f.as(900002);const count=jobs();
  await f.inject({text:'Убери это',reply_actions:tokens});assert.match(f.last.payload.text,/актуальную личную карточку/);assert.equal(jobs(),count);
  await f.as(900001);
 });
 await scenario('REP05','Changed reward invalidates old confirmation after preview',async()=>{
  const tokens=await rewards('Чай');
  await reply(tokens,'Переименуй эту награду',{operation:'reward_edit',search:'Чай',name:'Старый план',points:7});
  await f.preview();const confirm=f.token('Подтвердить план');
  await rewards('Чай');await f.click('Каталог');await f.click('Изменить');await f.send('Новый чай | 9 | Правка вручную');
  await f.inject({action:confirm});assert.match(f.last.payload.text,/данные или права изменились/);
  assert.equal(f.last.reward_catalog.find((r:any)=>r.name==='Новый чай').pointsCost,9);
  assert.ok(!f.last.reward_catalog.some((r:any)=>r.name==='Старый план'));
 });
 await scenario('REP06','Consumed action token cannot supply reply context',async()=>{
  await rewards('Новый чай');const token=f.token('Каталог');await f.click('Каталог');const count=jobs();
  await f.inject({text:'Убери это',reply_actions:[token]});assert.match(f.last.payload.text,/актуальную личную карточку/);assert.equal(jobs(),count);
 });
 await scenario('REP07','Voice reply to shopping card buys item after explicit HTML confirmation',async()=>{
  await f.request('shopping_add','Добавь молоко',{items:[{name:'Молоко',quantity:'1',note:''}]});await f.apply();
  await f.home();await f.click('Домашние планы');await f.click('Покупки');const tokens=actions();
  await reply(tokens,'Я купил это',{operation:'shopping_buy',search:'Молоко'},true);
  assert.equal(f.last.shopping_bought,0);await f.apply();assert.equal(f.last.shopping_bought,1);
 });
 await scenario('REP08','Gift card reply edits only owner-visible data, with no claim projection',async()=>{
  await f.request('wish_create','Добавь желание атлас',{name:'Атлас',description:'Подарок'});await f.apply();
  await f.home();await f.click('Подарки');const tokens=actions();
  await reply(tokens,'Назови это «Атлас мира»',{operation:'wish_edit',search:'Атлас',name:'Атлас мира',description:'Подарок'});
  const job=f.last.ai_jobs.at(-1);assert.equal(job.ReplyTarget.kind,'wish');assert.ok(!JSON.stringify(job.ReplyTarget).includes('claim'));
  await f.apply();await f.home();await f.click('Подарки');assert.equal(f.last.state.title,'Атлас мира');
 });
 await scenario('REP09','Reply cannot add an unrelated new object',async()=>{
  const tokens=await rewards('Новый чай'),before=f.last.reward_catalog.length;
  await reply(tokens,'Измени это',{operation:'reward_create',name:'Незапрошенная награда',points:1});
  f.denied();assert.equal(f.last.reward_catalog.length,before);
 });
 await scenario('REP10','Expired reply card is rejected before queueing',async()=>{
  const tokens=await rewards('Новый чай');await f.inject({tick:true,now:'2026-03-03T12:00:00Z'});const count=jobs();
  await f.inject({text:'Убери это',reply_actions:tokens});assert.match(f.last.payload.text,/актуальную личную карточку/);assert.equal(jobs(),count);
 });
 assert.deepEqual(f.errors,[]);fs.mkdirSync('.tools',{recursive:true});fs.writeFileSync('.tools/agent-reply-e2e.json',JSON.stringify({results,states:f.cards.length},null,2));fs.writeFileSync('.tools/agent-reply-cards.json',JSON.stringify(f.cards));
 console.log(`PASS ${results.length} scoped-reply HTML E2E scenarios; ${f.cards.length} rendered states`);
}finally{await f.close();}
