// Actual HTML -> bot updates -> Eino with an offline tool-calling model -> SQLite.
// The model emits requests, not successful domain results. Every probe restarts.
import { Flow } from "./flow-driver.mts";
import assert from "node:assert/strict";
import fs from "node:fs";
const f=new Flow(4180);
async function plan(){
  await f.inject({tick:true,process_ai:true});
  assert.equal(f.last.state?.title,"Проверьте изменения");
  for(let n=0;n<100;n++){
    if(f.last.state.buttons.some((b:any)=>b.label==="Подтвердить план"))return;
    await f.click("Следующая часть");
  }
  throw Error("preview pagination did not finish");
}
try{
 await f.start();await f.create();
 await f.raw({tick:true,agent_fixture:{operation:"task_due",due:"2026-03-02 19:00"}});
 // Two rooms with real preset tasks, created through product dialogs.
 for(const room of ["Кухня","Ванная"]){
  await f.home();await f.click("Домашние планы");await f.click("Готовые наборы");await f.click(room);await f.click("Добавить набор");
 }
 await f.home();await f.click("Домашние планы");await f.click("Комнаты");await f.click("Дела комнаты");
 assert.match(f.last.state.text,/Контекст:.*Кухня/);
 const original=f.last.tasks_open.map((t:any)=>({...t}));
 await f.send("/agent");assert.match(f.last.state.text,/Кухня/);
 await f.send("Перенеси все дела здесь на завтра в 19:00");await f.click("Понятно, продолжить");await plan();
 assert.deepEqual(f.last.tasks_open,original,"preview committed changes");
 const confirm=f.token("Подтвердить план");
 // A different family cannot use a personalized confirmation token.
 const foreign=await f.raw({viewer:900004,action:confirm});assert.equal((foreign.tasks_open||[]).length,0);
 await f.click("Подтвердить план");assert.equal(f.last.state.title,"План применён");
 for(const t of f.last.tasks_open){const old=original.find((x:any)=>x.ID===t.ID);if(t.ID<=3)assert.equal(t.DueAt,"2026-03-02T18:00:00Z");else assert.equal(t.DueAt,old.DueAt);}
 const accepted=JSON.stringify(f.last.tasks_open);
 assert.equal(JSON.stringify((await f.raw({action:confirm})).tasks_open),accepted,"replay changed tasks");
 // Context is personal: selecting the bathroom as B does not move A's focus.
 await f.as(900002);await f.click("Домашние планы");await f.click("Комнаты");await f.click("Следующая комната");await f.click("Дела комнаты");
 await f.send("/agent");assert.match(f.last.state.text,/Ванная/);
 await f.send("Перенеси дела на завтра");await f.click("Понятно, продолжить");await f.inject({tick:true,process_ai:true});assert.match(f.last.payload.text,/безопасно/);
 await f.as(900001);await f.send("/agent");assert.match(f.last.state.text,/Кухня/);
 // Changing the room after enqueue invalidates the captured scope.
 await f.send("Перенеси здесь всё на завтра");await f.send("/agent");await f.click("Сбросить контекст");
 await f.inject({tick:true,process_ai:true});assert.equal(JSON.stringify(f.last.tasks_open),accepted);
 // Natural language room selection is a tool call; no task changes.
 await f.raw({tick:true,agent_fixture:{operation:"select_room",room_id:2}});
 await f.send("Выбери ванную");await f.inject({tick:true,process_ai:true});assert.match(f.last.state.text,/Ванная/);
 // Voice enters the same queue. Confirmation is tied to the current plan.
 await f.raw({tick:true,agent_fixture:{operation:"task_due",due:"2026-03-03 20:30"},speech:"Перенеси дела здесь на послезавтра в 20:30"});
 await f.inject({voice:true});await plan();
 await f.raw({tick:true,speech:"Подтверждаю"});await f.inject({voice:true});await f.inject({tick:true,process_ai:true});
 assert.equal(f.last.state.title,"План применён");
 for(const t of f.last.tasks_open)if(t.ID>3)assert.equal(t.DueAt,"2026-03-03T19:30:00Z");
 // Hostile model ID, expired plan, stop, and stale task version remain harmless.
 await f.raw({tick:true,agent_fixture:{operation:"task_due",due:"2026-03-05 19:00",foreign_id:999999}});
 await f.send("Перенеси дела");await f.inject({tick:true,process_ai:true});assert.match(f.last.payload.text,/безопасно/);
 await f.raw({tick:true,agent_fixture:{operation:"task_due",due:"2026-03-05 19:00"}});
 await f.send("Перенеси дела");await plan();const stale=f.token("Подтвердить план");
 await f.openTask("Протереть зеркало");await f.click("Параметры");await f.click("Изменить");await f.click("Название и баллы");await f.send("11 | Протереть зеркало");
 const beforeStale=JSON.stringify(f.last.tasks_open);
 const rejected=await f.raw({action:stale});assert.equal(JSON.stringify(rejected.tasks_open),beforeStale);assert.match(rejected.payload.text,/данные|права/);
 await f.send("Перенеси дела");await plan();const expired=f.token("Подтвердить план");
 await f.raw({tick:true,now:"2026-03-01T12:16:00Z"});const expiration=await f.raw({action:expired});assert.match(expiration.payload.text,/устарел/);
 await f.send("Перенеси дела");await f.send("/stop");const stopped=JSON.stringify(f.last.tasks_open);await f.inject({tick:true,process_ai:true});assert.equal(JSON.stringify(f.last.tasks_open),stopped);
 // A yes after an intervening question must not approve the older intent.
 await f.raw({tick:true,agent_fixture:{operation:"task_due",due:"2026-03-06 19:00"}});
 await f.send("Перенеси дела");await plan();const beforeQuestion=JSON.stringify(f.last.tasks_open);
 await f.raw({tick:true,agent_fixture:{operation:"read"}});await f.send("Какие дела здесь остались?");await f.inject({tick:true,process_ai:true});
 await f.send("Да");await f.inject({tick:true,process_ai:true});assert.match(f.last.payload.text,/Нет актуального плана/);assert.equal(JSON.stringify(f.last.tasks_open),beforeQuestion);
 // Reading is allowed despite failed mutations; reset clears the visible room.
 await f.raw({tick:true,agent_fixture:{operation:"read"}});await f.send("Что здесь осталось?");await f.inject({tick:true,process_ai:true});assert.match(f.last.payload.text,/Найдено дел: 3/);
 await f.send("/agent");await f.click("Сбросить контекст");assert.match(f.last.state.text,/Весь дом/);
 fs.writeFileSync(".tools/agent-e2e-cards.json",JSON.stringify(f.cards));
 console.log(`PASS room-aware HTML/Eino agent: preview, batch, replay, two families, per-user room, queued stale context, voice fixture confirmation, forbidden edit, foreign ID, stale version, expiry, stop, read/reset; ${f.cards.length} states`);
}finally{await f.close();}
