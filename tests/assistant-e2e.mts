// Fixture STT/LLM, real consent/queue/validation/domain/outbox, real HTML gestures.
// Provider availability and human Telegram voice are separate live gates.
import { Flow } from "./flow-driver.mts";
import assert from "node:assert/strict";
import fs from "node:fs";
const f = new Flow(4178);
try {
  await f.start();
  await f.create();
  await f.click("Добавить задание");
  await f.send("Вынести мусор");
  await f.send("10");
  const taskID = f.last.tasks_open[0].ID;
  await f.raw({
    tick: true,
    ai_command: { action: "complete", ids: [taskID] },
    speech: "я выкинул мусор",
  });
  await f.inject({ voice: true });
  await f.click("Понятно, продолжить");
  await f.inject({ tick: true, process_ai: true });
  assert.equal(f.last.balance, 10);
  assert.equal(f.last.xp, 10);
  await f.showDelivery("Отменить выполнение");
  const undo = f.token("Отменить выполнение");
  await f.click("Отменить выполнение");
  assert.equal(f.last.balance, 0);
  assert.equal((await f.raw({ action: undo })).balance, 0);
  // The backend refuses unsafe completions even when an erroneous model requests them.
  for (const text of [
    "Я не выкинул мусор",
    "Я завтра выкину мусор",
    "Надо выкинуть мусор",
    "Мне кажется, мусор уже вынесли",
    "Я выкинул мусор вчера",
  ]) {
    await f.home();
    await f.send(text);
    await f.inject({ tick: true, process_ai: true });
    assert.equal(f.last.balance, 0);
    assert.equal(f.last.tasks_open.length, 1);
    assert.match(f.last.payload.text, /безопасно/);
  }
  await f.raw({
    tick: true,
    ai_command: { action: "complete", ids: [987654] },
  });
  await f.home();
  await f.send("Я выкинул мусор");
  await f.inject({ tick: true, process_ai: true });
  assert.equal(f.last.balance, 0);
  // A draft does not create tasks/jobs until confirmed; cancelled and expired buttons stay harmless.
  const draft = {
    action: "draft",
    draft: {
      title: "Полить цветы",
      points: 7,
      schedule: { mode: "weekdays", days: ["tuesday"], hour: 19, minute: 0 },
    },
  };
  await f.raw({ tick: true, ai_command: draft });
  await f.home();
  await f.send(
    "Напоминай поливать цветы каждый вторник в семь вечера, 7 баллов",
  );
  await f.inject({ tick: true, process_ai: true });
  assert.equal(f.last.state.title, "Проверьте перед действием");
  assert.equal(f.last.tasks_open.length, 1);
  assert.match(f.last.state.text, /19:00/);
  const cancelled = f.token("Подтвердить");
  await f.click("Отмена");
  assert.equal((await f.raw({ action: cancelled })).tasks_open.length, 1);
  await f.home();
  await f.send(
    "Напоминай поливать цветы каждый вторник в семь вечера, 7 баллов",
  );
  await f.inject({ tick: true, process_ai: true });
  const confirm = f.token("Подтвердить");
  await f.click("Подтвердить");
  assert.equal(f.last.tasks_open.length, 2);
  assert.equal((await f.raw({ action: confirm })).tasks_open.length, 2);
  assert.equal(f.last.balance, 0);
  await f.openTask("Полить цветы");
  assert.match(f.last.state.text, /19:00/);
  // Optional photo preserves direct acceptance. Undo expires without deleting the audit.
  await f.openTask("Вынести мусор");
  await f.click("Параметры");
  await f.click("Выполнить с фото");
  await f.inject({ photo: "fixture_optional_photo" });
  assert.equal(f.last.balance, 10);
  await f.showDelivery("Отменить выполнение");
  const expired = f.token("Отменить выполнение");
  await f.raw({ tick: true, now: "2026-03-01T12:06:00Z" });
  await f.click("Отменить выполнение");
  assert.equal(f.last.balance, 10);
  assert.match(f.last.payload.text, /минут|отмен/i);
  assert.equal((await f.raw({ action: expired })).balance, 10);
  // Statistics are navigated through their actual HTML, including accepted vs performed timestamps.
  await f.home();
  await f.click("Мои баллы");
  await f.click("Рейтинг");
  assert.match(f.last.state.text, /понедельника|10/);
  await f.click("Назад");
  await f.click("Достижения");
  assert.match(f.last.state.text, /Перв|фото/i);
  await f.click("Назад");
  await f.click("Дом и история");
  assert.match(f.last.state.text, /Общий опыт: 10/);
  await f.click("История баллов");
  assert.match(f.last.state.text, /Принято|Заявлено/);
  fs.writeFileSync(".tools/assistant-e2e-cards.json", JSON.stringify(f.cards));
  console.log(
    `PASS HTML assistant consent/voice fixture/safe completion/undo/negative phrases/foreign ID/draft cancel-confirm-replay/optional photo/expired undo/statistics; ${f.cards.length} states`,
  );
} finally {
  await f.close();
}
