import { Flow } from "./flow-driver.mts";
import assert from "node:assert/strict";
import fs from "node:fs";
const f = new Flow(4177);
try {
  await f.start();
  await f.create();
  await f.click("Домашние планы");
  await f.click("Покупки");
  await f.click("Добавить покупку");
  await f.send("Молоко");
  await f.send("2 л");
  await f.send("Без лактозы <script>window.hacked=1</script>");
  const save = f.token("Сохранить");
  await f.click("Сохранить");
  assert.equal(f.last.state.title, "Молоко");
  assert.match(f.last.state.text, /2 л/);
  await f.raw({ action: save });
  await f.as(900002);
  await f.click("Домашние планы");
  await f.click("Покупки");
  const b = f.token("Куплю я");
  await f.as(900003);
  await f.click("Домашние планы");
  await f.click("Покупки");
  const c = f.token("Куплю я");
  const rs = await Promise.all([
    f.raw({ viewer: 900002, action: b }),
    f.raw({ viewer: 900003, action: c }),
  ]);
  assert.equal(
    rs.filter((r) =>
      r.state?.buttons.some((b: any) => b.label === "Освободить"),
    ).length,
    1,
  );
  const winner = rs[0].state ? 900002 : 900003,
    loser = winner === 900002 ? 900003 : 900002;
  await f.as(winner);
  await f.click("Домашние планы");
  await f.click("Покупки");
  const buy = f.token("Куплено");
  await f.raw({ viewer: loser, action: buy });
  await f.raw({ viewer: 900004, action: buy });
  await f.click("Куплено");
  assert.match(f.last.state.text, /Куплено/);
  assert.equal(f.last.balance, 0);
  assert.equal(f.last.xp, 0);
  await f.raw({ action: buy });
  const reopen = f.token("Вернуть в список");
  await f.click("Вернуть в список");
  await f.raw({ action: reopen });
  await f.as(900001);
  await f.click("Домашние планы");
  await f.click("Покупки");
  await f.click("Параметры");
  await f.click("Править");
  await f.send("Молоко другое");
  await f.click("Оставить");
  await f.click("Оставить");
  await f.click("Сохранить");
  assert.equal(f.last.state.title, "Молоко другое");
  await f.click("Параметры");
  await f.click("Подробнее");
  assert.match(f.last.payload.text, /<script>/);
  assert.equal(
    await f.page.evaluate(() => Boolean((window as any).hacked)),
    false,
  );
  await f.home();
  await f.click("Домашние планы");
  await f.click("Покупки");
  await f.click("Параметры");
  await f.click("Добавить покупку");
  await f.send("Хлеб, корм");
  const cancelled = f.token("Сохранить");
  await f.click("Отмена");
  await f.click("Параметры");
  await f.click("Добавить покупку");
  await f.send("Крупа");
  await f.click("Пропустить");
  await f.click("Пропустить");
  await f.raw({ action: cancelled });
  await f.click("Сохранить");
  await f.click("Следующая");
  assert.equal(f.last.state.title, "Крупа");
  await f.click("Параметры");
  await f.click("Убрать");
  await f.click("Назад");
  await f.click("Следующая");
  await f.click("Параметры");
  await f.click("Убрать");
  await f.click("Да, убрать");
  assert.equal(f.last.state.title, "Молоко другое");
  await f.as(900004);
  await f.click("Домашние планы");
  await f.click("Покупки");
  assert.match(f.last.state.text, /пуст/);
  await f.as(900001);
  // Maximum Cyrillic lengths and input that resembles HTML stay plain text.
  await f.click("Домашние планы");
  await f.click("Покупки");
  await f.click("Параметры");
  await f.click("Добавить покупку");
  await f.send("Я".repeat(101));
  assert.match(f.last.state.text, /100/);
  await f.send("Я".repeat(100));
  await f.send("К".repeat(40));
  await f.send("З".repeat(300));
  await f.click("Сохранить");
  // Fixture Russian voice -> real queue/consent -> validated shopping draft -> HTML confirmation.
  await f.home();
  const ai = {
    action: "shopping_draft",
    ids: [],
    draft: null,
    shopping: [
      { name: "Яблоки", quantity: "1 кг", note: "" },
      { name: "Чай", quantity: "1 пачка", note: "" },
    ],
  };
  await f.raw({
    tick: true,
    ai_command: ai,
    speech: "добавь яблоки и чай в список покупок",
  });
  const before = (await f.inject({ voice: true })).shopping_needed;
  await f.click("Понятно, продолжить");
  await f.inject({ tick: true, process_ai: true });
  assert.equal(f.last.state.title, "Проверьте покупки");
  assert.equal(f.last.shopping_needed, before);
  await f.click("Следующая");
  assert.match(f.last.state.text, /Чай/);
  const aiSave = f.token("Сохранить");
  await f.click("Сохранить");
  assert.equal(f.last.shopping_needed, before + 2);
  assert.equal((await f.raw({ action: aiSave })).shopping_needed, before + 2);
  await f.home();
  await f.send("Добавь яблоки и чай");
  await f.inject({ tick: true, process_ai: true });
  await f.click("Отмена");
  assert.equal(f.last.shopping_needed, before + 2);
  fs.writeFileSync(".tools/shopping-e2e-cards.json", JSON.stringify(f.cards));
  console.log(
    `PASS HTML shopping batch/draft/edit/claim race/buy/reopen/archive/stale/foreign/XSS/max text; ${f.cards.length} states`,
  );
} finally {
  await f.close();
}
