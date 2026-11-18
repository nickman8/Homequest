// HTML conversations, normal handler actions and the same database across viewers.
import assert from "node:assert/strict";
import fs from "node:fs";
import {AgentFlow} from "./agent-flow.mts";
const f = new AgentFlow(4186);
async function change(operation:string, text:string, fields={}) {
  await f.request(operation, text, {changes:[{operation,...fields}]});
  return f.apply();
}
try {
  await f.start();
  await f.create();
  await change("premium_on","Включи тестовый Premium");
  await f.as(900002);
  await f.request("premium_off","Отключи Premium",{changes:[{operation:"premium_off"}]});
  f.denied();
  await f.home(); await f.click("Мои баллы"); await f.click("Premium");
  assert.match(f.last.state.text,/включён/);
  await change("quiet_hours","Установи тихие часы с 21 до 9",{quiet_start:21,quiet_end:9});
  await change("vacation","Включи отпуск на три дня",{days:3});
  await f.home(); await f.click("Настройки"); await f.click("Напоминания");
  assert.match(f.last.state.text,/21–09/); assert.match(f.last.state.text,/Отпуск/);
  await change("wish_create","Хочу получить атлас",{name:"Атлас"});
  await f.home(); await f.click("Подарки");
  const ownerView = () => [f.last.state.title,f.last.state.text,...f.last.state.buttons.map((b:any)=>b.label)];
  const before = ownerView();
  await f.as(900003); await f.request("wish_claim","Забронируй атлас",{search:"Атлас"});
  await f.apply();
  await f.as(900002); await f.home(); await f.click("Подарки");
  assert.deepEqual(ownerView(),before);
  await f.as(900004); await f.request("wish_read","Покажи атлас",{search:"Атлас"});
  assert.match(f.last.payload.text,/позиций: 0/);
  await f.as(900001);
  await f.request("plan","Создай библиотеку и задание в ней",{changes:[
    {operation:"room_create",name:"Библиотека"},
    {operation:"task_create",name:"Расставить книги",room_from_step:1,points:7,approval:true}
  ]});
  assert.match(await f.apply(),/Библиотека/);
  await f.as(900002); await f.request("task_complete","Я расставил книги",{search:"Расставить книги",room_id:-1});
  await f.apply(); assert.equal(f.last.balance,0);
  await f.request("report_approve","Прими мой отчёт",{search:"Расставить книги"});
  f.denied();
  await f.as(900003); await f.request("report_approve","Прими отчёт по книгам",{search:"Расставить книги"});
  await f.apply(); await f.as(900002); assert.equal(f.last.balance,7);
  fs.writeFileSync(".tools/agent-family-cards.json",JSON.stringify(f.cards));
  console.log("PASS family agent: permissions, rest, private gifts, room dependency and review;",f.cards.length,"states");
} finally { await f.close(); }
