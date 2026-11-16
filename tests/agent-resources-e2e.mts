// Browser HTML -> real bot/Eino tools -> existing reward/shopping services -> SQLite.
// The deterministic model reads tool outputs; it never inserts a successful result.
import {Flow} from './flow-driver.mts';
import assert from 'node:assert/strict';
import fs from 'node:fs';
const f=new Flow(4181);
async function request(operation:string,text:string,fields:any={}){
 await f.raw({tick:true,agent_fixture:{operation,...fields}});
 await f.send(text);
 if(f.last.state?.buttons.some((b:any)=>b.label==='Понятно, продолжить'))await f.click('Понятно, продолжить');
 await f.inject({tick:true,process_ai:true});
}
async function preview(){
 assert.equal(f.last.state?.title,'Проверьте изменения');
 let text='';
 for(let n=0;n<100;n++){
  text+=f.last.state.text+'\n';
  if(f.last.state.buttons.some((b:any)=>b.label==='Подтвердить план'))return text;
  await f.click('Следующая часть');
 }
 throw Error('preview loop');
}
async function apply(){const summary=await preview();await f.click('Подтвердить план');assert.equal(f.last.state.title,'План применён');return summary;}
async function denied(){assert.match(f.last.payload.text,/безопасно/);}
try{
 await f.start();await f.create();
 await request('shopping_add','Добавь молоко',{items:[{name:'Молоко',quantity:'1 л',note:'<script>window.hacked=1</script>'}]});
 assert.equal(f.last.shopping_needed,0);await apply();assert.equal(f.last.shopping_needed,1);
 await request('shopping_edit','Измени количество молока на два литра',{search:'Молоко',items:[{name:'Молоко',quantity:'2 л',note:'Без лактозы'}]});
 assert.match(await apply(),/2 л/);
 // B claims, C cannot buy B's claimed item, D cannot use B's confirmation token.
 await f.as(900002);await request('shopping_claim','Молоко куплю я',{search:'Молоко'});await preview();const claim=f.token('Подтвердить план');
 assert.equal((await f.raw({viewer:900004,action:claim})).shopping_needed,0);
 await f.click('Подтвердить план');assert.equal(f.last.state.title,'План применён');
 await f.as(900003);await request('shopping_buy','Я купил молоко',{search:'Молоко'});await denied();assert.equal(f.last.shopping_bought,0);
 await f.as(900002);await request('shopping_release','Освободи мою покупку молока',{search:'Молоко'});await apply();
 await request('shopping_buy','Я не купил молоко',{search:'Молоко'});await denied();assert.equal(f.last.shopping_bought,0);
 await request('shopping_buy','Я купил молоко',{search:'Молоко'});await preview();const buy=f.token('Подтвердить план');await f.click('Подтвердить план');
 assert.equal(f.last.shopping_bought,1);assert.equal((await f.raw({action:buy})).shopping_bought,1);assert.equal(f.last.balance,0);
 await request('shopping_reopen','Верни молоко в список',{search:'Молоко'});await apply();assert.equal(f.last.shopping_needed,1);
 await request('shopping_archive','Убери молоко',{search:'Молоко'});await denied(); // B is not creator or owner.
 await f.as(900001);await request('shopping_archive','Убери молоко',{search:'Молоко'});await apply();assert.equal(f.last.shopping_needed,0);
 // Earn through product handlers, not by seeding a balance.
 await request('task_create','Создай дело за 100 баллов',{name:'Большое дело',points:100});await apply();
 await f.as(900002);await request('task_complete','Я сделал большое дело',{search:'Большое дело'});await apply();assert.equal(f.last.balance,100);
 await f.as(900001);await request('reward_create','Добавь награду кино за 30 баллов',{name:'Кино',points:30,description:'Выбрать фильм'});await apply();
 await f.as(900002);await request('reward_request','Хочу кино',{search:'Кино'});assert.match(await preview(),/списать 30/);const oldPrice=f.token('Подтвердить план');assert.equal(f.last.balance,100);
 await f.as(900001);await request('reward_edit','Измени цену кино на 40',{search:'Кино',name:'Кино',points:40,description:'Выбрать фильм'});await apply();
 const changed=await f.raw({viewer:900002,action:oldPrice});assert.match(changed.payload.text,/данные|права/);assert.equal(changed.balance,100);
 await f.as(900002);await request('reward_request','Хочу кино',{search:'Кино'});await preview();const redeem=f.token('Подтвердить план');await f.click('Подтвердить план');assert.equal(f.last.balance,60);
 assert.equal((await f.raw({action:redeem})).balance,60);assert.equal(f.last.xp,100);assert.equal(f.last.week,100);
 await request('redemption_approve','Прими мою заявку',{search:'Кино'});await denied();
 await f.as(900001);await request('redemption_approve','Прими заявку на кино',{search:'Кино'});assert.match(await apply(),/заявка №/);
 // Archived catalog retains fulfillment; a delivered request has no normal refund.
 await request('reward_archive','Убери кино из каталога',{search:'Кино'});await apply();
 await request('redemption_deliver','Награда кино выдана',{search:'Кино'});await apply();
 await f.as(900002);await request('redemption_cancel','Отмени заявку на кино',{search:'Кино'});await denied();assert.equal(f.last.balance,60);
 // Another request can be rejected/refunded once, at its original price.
 await f.as(900001);await request('reward_create','Добавь награду чай за 20 баллов',{name:'Чай',points:20});await apply();
 await f.as(900002);await request('reward_request','Хочу чай',{search:'Чай'});await apply();assert.equal(f.last.balance,40);
 await f.as(900001);await request('reward_edit','Измени цену чая на 50',{search:'Чай',name:'Чай',points:50});await apply();
 await request('redemption_reject','Отклони заявку на чай',{search:'Чай'});assert.match(await preview(),/вернуть 20/);const refund=f.token('Подтвердить план');await f.click('Подтвердить план');await f.raw({action:refund});
 await f.as(900002);assert.equal(f.last.balance,60);assert.equal(f.last.xp,100);assert.equal(f.last.week,100);
 // Cancellation before fulfillment also refunds once.
 await request('reward_request','Хочу чай',{search:'Чай'});await apply();assert.equal(f.last.balance,10);
 await request('redemption_cancel','Отмени мои заявки на чай',{search:'Чай'});await denied(); // includes rejected historical request; no partial cancellation.
 assert.equal(f.last.balance,10);
 await request('redemption_cancel','Отмени ожидающую заявку на чай',{search:'Чай',status:'pending'});await apply();assert.equal(f.last.balance,60);
 // An invented existing ID cannot pass the observed-resource gate.
 await request('reward_request','Хочу чай',{search:'Чай',foreign_id:999999});await denied();assert.equal(f.last.balance,60);
 await f.as(900004);await request('reward_read','Какие есть награды?');assert.match(f.last.payload.text,/Найдено позиций: 0/);
 await request('redemption_read','Какие есть заявки?');assert.match(f.last.payload.text,/Найдено позиций: 0/);
 // One request creates the full task; required photo and reviewer are not lost.
 await f.as(900001);const owner=f.last.members.find((m:any)=>m.Owner).ID;const member=f.last.members.find((m:any)=>!m.Owner).ID;
 await request('task_create','Создай задание помыть плиту, назначь участнику, нужны фото и проверка владельца',{name:'Плита',points:25,description:'Сначала остудить',assigned_to:member,reviewer:owner,approval:true,photo:true,due:'2026-03-02 19:00'});
 const rich=await apply();assert.match(rich,/подтверждение: нужно/);assert.match(rich,/фото: нужно/);assert.match(rich,/проверяет:/);assert.match(rich,/Сначала остудить/);
 const created=f.last.tasks_open.find((t:any)=>t.Name==='Плита');assert.equal(created.AssignedTo,member);assert.equal(created.DueAt,'2026-03-02T18:00:00Z');
 await request('task_reviewer','Сними конкретного проверяющего у плиты',{search:'Плита',target:0});assert.match(await apply(),/любой другой взрослый/);
 await request('task_reviewer','Назначь владельца проверяющим плиты',{search:'Плита',target:owner});await apply();
 await f.as(900002);await request('task_complete','Я помыл плиту',{search:'Плита'});const beforePhoto=f.last.balance;await apply();assert.equal(f.last.balance,beforePhoto);
 assert.ok(f.last.deliveries.some((d:any)=>JSON.parse(d.Payload).text?.includes('Пришлите фотографию')));
 // Photo goes through the regular Telegram handler, not the model.
 await f.inject({photo:'synthetic-agent-stove-photo'});assert.equal(f.last.state.title,'Ожидает подтверждения');assert.equal(f.last.balance,beforePhoto);
 await f.as(900003);await f.click('Настройки');await f.click('Отчёты');assert.match(f.last.state.text,/нет отчётов/);
 await f.as(900001);await f.click('Настройки');await f.click('Отчёты');await f.click('Принять');
 await f.as(900002);assert.equal(f.last.balance,beforePhoto+25);
 await request('account_read','Сколько у меня баллов и какой вклад за неделю?');assert.match(f.last.payload.text,/Баланс: 85.*Вклад недели: 125.*Опыт: 125/);
 await f.as(900004);await request('account_read','Какие у нас баллы?');assert.match(f.last.payload.text,/Баланс: 0.*Вклад недели: 0.*Опыт: 0.*рейтинге: 1/);
 assert.equal(await f.page.evaluate(()=>Boolean((window as any).hacked)),false);
 fs.writeFileSync('.tools/agent-resources-e2e-cards.json',JSON.stringify(f.cards));
 console.log(`PASS agent resources HTML/Eino: shopping lifecycle, permissions, negative intent, replay, rewards create/edit/archive/request/approve/deliver/refund, price drift, balances, foreign IDs, isolation, rejected batch; ${f.cards.length} states`);
}finally{await f.close();}
