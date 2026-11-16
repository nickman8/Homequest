import { Flow } from "./flow-driver.mts";
import assert from "node:assert/strict";
import fs from "node:fs";
const f = new Flow(4176);
try {
  await f.start();
  await f.create();
  await f.click("Домашние планы");
  await f.click("Готовые наборы");
  await f.click("Кухня");
  const oldSave = f.token("Добавить набор");
  await f.click("Изменить дело");
  await f.send("Протереть стол | -1 | 2 | 19:00");
  assert.match(f.last.payload.text, /Проверьте/);
  await f.send("Протереть стол | 7 | 2 | 18:30");
  await f.click("Следующее");
  await f.click("Включить / пропустить");
  await f.click("Добавить набор");
  assert.equal(f.last.state.title, "Кухня");
  await f.raw({ action: oldSave });
  await f.home();
  await f.click("Домашние планы");
  await f.click("Готовые наборы");
  await f.click("Кухня");
  await f.click("Добавить набор");
  await f.click("Дела комнаты");
  assert.match(f.last.state.text, /1 из 2/);
  assert.equal(f.last.state.title, "Протереть стол");
  await f.click("К комнатам");
  await f.click("Переименовать");
  await f.send("Кухня и столовая");
  assert.equal(f.last.state.title, "Кухня и столовая");
  await f.click("Убрать комнату");
  await f.click("Отмена");
  assert.equal(f.last.state.title, "Кухня и столовая");
  await f.click("Убрать комнату");
  await f.click("Убрать комнату");
  await f.openTask("Протереть стол");
  assert.match(f.last.state.text, /7 баллов/);
  // Custom room, duplicate-normalized names, assignment of an existing task.
  await f.home();
  await f.click("Домашние планы");
  await f.click("Комнаты");
  await f.click("Добавить комнату");
  await f.send("Кабинет");
  await f.click("Добавить");
  await f.send(" кабинет ");
  assert.equal(f.last.state.title, "Кабинет");
  await f.openTask("Протереть стол");
  await f.click("Параметры");
  await f.click("Комната и очередь");
  await f.click("Выбрать комнату");
  await f.click("Выбрать");
  // A/B rotation; a member cannot change someone else's rules; voluntary handoff.
  await f.openTask("Протереть стол");
  await f.click("Параметры");
  await f.click("Комната и очередь");
  await f.click("Чередование");
  await f.click("Включить / убрать");
  await f.click("Следующий участник");
  await f.click("Включить / убрать");
  await f.click("Сохранить очередь");
  await f.openTask("Протереть стол");
  assert.match(f.last.state.text, /Участник 1/);
  const stale = f.token("Выполнено");
  await f.click("Параметры");
  await f.click("Комната и очередь");
  await f.click("Предложить передачу");
  await f.click("Предложить");
  const cancel = f.token("Отозвать предложение");
  await f.as(900002);
  await f.click("Домашние планы");
  await f.click("Передачи дел");
  const take = f.token("Принять дело");
  await f.raw({ viewer: 900004, action: take });
  await f.click("Принять дело");
  await f.raw({ action: take });
  await f.raw({ viewer: 900001, action: cancel });
  await f.openTask("Протереть стол");
  assert.match(f.last.state.text, /Участник 2/);
  await f.raw({ viewer: 900001, action: stale });
  await f.click("Выполнено");
  assert.equal(f.last.balance, 7);
  await f.openTask("Протереть стол");
  assert.match(f.last.state.text, /Участник 1/);
  await f.click("Параметры");
  await f.click("Комната и очередь");
  await f.click("Чередование");
  await f.click("Включить / убрать");
  await f.click("Следующий участник");
  await f.click("Включить / убрать");
  await f.click("Сохранить очередь");
  assert.match(f.last.payload.text, /недоступно/);
  await f.as(900001);
  await f.openTask("Протереть стол");
  await f.click("Параметры");
  await f.click("Комната и очередь");
  await f.click("Предложить передачу");
  await f.click("Предложить");
  await f.as(900002);
  await f.click("Домашние планы");
  await f.click("Передачи дел");
  await f.click("Отказаться");
  await f.openTask("Протереть стол");
  assert.match(f.last.state.text, /Участник 1/);
  // Goal changes apply next week; Sunday contribution belongs to previous local week.
  await f.as(900001);
  await f.click("Домашние планы");
  await f.click("Итог недели");
  await f.click("Настроить цель");
  await f.send("-1");
  assert.match(f.last.payload.text, /Цель/);
  await f.send("20");
  assert.match(f.last.state.text, /7 \/ 100/);
  await f.as(900002);
  await f.click("Настройки");
  await f.click("Напоминания");
  await f.click("Личные настройки");
  await f.send("выкл");
  const tick = await f.raw({ now: "2026-03-02T08:00:00Z", tick: true });
  const digests = (tick.sent || []).filter((r: any) =>
    r.payload.text?.startsWith("Итог недели"),
  );
  assert.equal(digests.length, 2);
  assert.ok(digests.every((r: any) => r.payload.text.includes("7 / 100")));
  const replay = await f.raw({ tick: true });
  assert.equal(
    (replay.sent || []).filter((r: any) =>
      r.payload.text?.startsWith("Итог недели"),
    ).length,
    0,
  );
  await f.as(900001);
  await f.click("Домашние планы");
  await f.click("Итог недели");
  assert.match(f.last.state.text, /0 \/ 20/);
  await f.click("Прошлая неделя");
  assert.match(f.last.state.text, /7 \/ 100/);
  await f.click("Выключить итог");
  assert.ok(f.last.state.buttons.some((b: any) => b.label === "Включить итог"));
  fs.writeFileSync(".tools/plans-e2e-cards.json", JSON.stringify(f.cards));
  console.log(
    `PASS HTML rooms/preset edit/skip/dedup/delete, rotation/handoff/decline/stale/foreign; ${f.cards.length} states`,
  );
} finally {
  await f.close();
}
