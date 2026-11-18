// Browser HTML -> controlled Telegram updates -> real handlers/migrations/SQLite.
// No domain success is seeded into the DB. Every probe process is also a restart.
import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";
import fs from "node:fs";
import assert from "node:assert/strict";
const require = createRequire(
  path.join(process.env.WIDGET_SANDBOX!, "package.json"),
);
const { chromium } = require("@playwright/test");
const port = "4175",
  url = `http://127.0.0.1:${port}`;
const server = spawn("node", ["scripts/preview-server.mjs"], {
  env: { ...process.env, HOMEQUEST_PREVIEW_PORT: port },
  stdio: "pipe",
});
let browser: any;
const cards: any[] = [];
let last: any;
let page: any;
let uid = 900001;
try {
  for (let n = 0; n < 100; n++) {
    try {
      if ((await fetch(url)).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  browser = await chromium.launch();
  page = await browser.newPage();
  page.setDefaultTimeout(10000);
  const errors: string[] = [];
  page.on("pageerror", (e: any) => errors.push(e.message));
  await page.goto(url);
  const frame = () => page.frameLocator("iframe");
  const effect = async (fn: () => Promise<any>) => {
    const wait = page.waitForResponse((r: any) => r.url().endsWith("/update"));
    await fn();
    const r = await wait;
    assert.equal(r.status(), 200);
    last = await r.json();
    if (last.state) cards.push(last.state);
    await page.waitForFunction(
      (id: any) => document.body.dataset.updateId === String(id),
      last.update_id,
    );
    return last;
  };
  const send = async (text: string) => {
    const result = await effect(async () => {
      await page.locator("#message").fill(text);
      await page.locator("#composer button").click();
    });
    await page.waitForFunction(
      () => !(document.querySelector("#message") as HTMLInputElement).value,
    );
    return result;
  };
  const click = async (label: string) => {
    for (let i = 0; i < 12; i++) {
      const b = frame().getByRole("button", { name: label, exact: true });
      if (await b.count()) return effect(() => b.click());
      const next = frame().getByRole("button", {
        name: /^(Далее|Ещё действия|К началу) ·/,
      });
      if (!(await next.count()))
        throw Error(`Missing ${label}: ${JSON.stringify(last)}`);
      await next.click();
    }
    throw Error("pagination loop " + label);
  };
  const home = () => send("/start");
  const as = async (n: number) => {
    uid = n;
    await effect(() => page.locator("#viewer").selectOption(String(n)));
  };
  const raw = async (data: any) => {
    const r = await page.request.post(url + "/update", {
      headers: { Origin: url },
      data: { viewer: uid, ...data },
    });
    assert.equal(r.status(), 200);
    return r.json();
  };
  const token = (label: string) => {
    const b = last.state?.buttons.find((b: any) => b.label === label);
    assert.ok(b, label);
    return b.action;
  };
  const reports = async () => {
    await home();
    await click("Настройки");
    await click("Отчёты");
  };
  const points = async () => {
    await home();
    return click("Мои баллы");
  };
  const task = async (name: string, n = "10") => {
    await home();
    await click("Добавить задание");
    await send(name);
    await send(n);
  };
  await send("/start");
  await click("Создать дом");
  await send("Тёплый дом");
  await send("Not/A_Zone");
  assert.match(last.payload.text, /пояс|timezone|часов/i);
  await send("Europe/Berlin");
  await click("Настройки");
  await click("Пригласить");
  const invite = last.payload.text.split("join_")[1];
  assert.ok(invite);
  for (const n of [900002, 900003]) {
    await as(n);
    await send("/start join_" + invite);
  }
  await as(900004);
  await click("Создать дом");
  await send("Другой дом");
  await send("UTC");
  // Photo + rejection + retry + approval; no points before acceptance; foreign/self tokens denied.
  await as(900001);
  await task("Фото <script>window.hacked=1</script>");
  await click("Параметры");
  await click("Правила");
  await click("Фото и подтверждение");
  await as(900002);
  await click("Сегодня");
  const oldDone = token("Выполнено");
  await click("Выполнено");
  assert.equal(last.balance, 0);
  assert.match(last.payload.text, /фотографию/);
  await raw({ photo: "fixture_photo_one" });
  await reports();
  assert.ok(!last.state.buttons.some((b: any) => b.label === "Принять"));
  await as(900003);
  await reports();
  const photoToken = token("Посмотреть фото"),
    reject = token("Отклонить");
  assert.equal(
    (await raw({ viewer: 900002, action: token("Принять") })).balance,
    0,
  );
  const foreign = await raw({ viewer: 900004, action: photoToken });
  assert.ok(!JSON.stringify(foreign).includes("fixture_photo_one"));
  await click("Посмотреть фото");
  assert.equal(last.payload.photo, "fixture_photo_one");
  await reports();
  await click("Отклонить");
  await as(900002);
  await click("Сегодня");
  await click("Выполнено");
  await raw({ photo: "fixture_photo_two" });
  await as(900003);
  await reports();
  const approve = token("Принять");
  await click("Принять");
  await raw({ action: approve });
  await raw({ action: reject });
  await as(900002);
  await points();
  assert.equal(last.balance, 10);
  assert.equal(last.xp, 10);
  await raw({ action: oldDone });
  await points();
  assert.equal(last.balance, 10);
  assert.equal(
    await page.evaluate(() => Boolean((window as any).hacked)),
    false,
  );
  // Premium is family scoped; direct wrong mode and wrong viewer cannot grant it.
  await as(900001);
  await points();
  await click("Premium");
  const on = token("Включить Premium для теста");
  await raw({ viewer: 900002, action: on });
  await raw({ action: on, billing_mode: "stars" });
  await home();
  await points();
  await click("Premium");
  assert.match(last.state.text, /проверяется бесплатно/);
  await click("Включить Premium для теста");
  await click("Расширенный вклад");
  assert.match(last.state.text, /Принято: 1/);
  await as(900002);
  await points();
  await click("Premium");
  assert.match(last.state.text, /включён/);
  await as(900004);
  await points();
  await click("Premium");
  assert.match(last.state.text, /проверяется бесплатно/);
  // Rewards snapshot, refund, duplicate actions, insufficient balance, delivery cannot refund.
  await as(900001);
  await points();
  await click("Награды");
  await click("Добавить награду");
  await send("Фильм | 7 | Выбрать фильм");
  await as(900002);
  await points();
  await click("Награды");
  const request = token("Запросить");
  await click("Запросить");
  assert.equal(last.balance, 3);
  await raw({ action: request });
  await as(900001);
  await points();
  await click("Награды");
  await click("Каталог");
  await click("Изменить");
  await send("Фильм дороже | 50 | Новая цена");
  await click("Заявки");
  assert.match(last.state.text, /7 баллов/);
  const refuse = token("Отказать");
  await click("Отказать");
  await raw({ action: refuse });
  await as(900002);
  await points();
  assert.equal(last.balance, 10);
  assert.equal(last.week, 10);
  await click("Награды");
  await click("Запросить");
  assert.match(last.payload.text, /балл/i);
  assert.equal(last.balance, 10);
  await as(900001);
  await points();
  await click("Награды");
  await click("Каталог");
  await click("Изменить");
  await send("Фильм | 7 | Вечер");
  await as(900002);
  await points();
  await click("Награды");
  await click("Запросить");
  const cancel = token("Отменить запрос");
  await as(900001);
  await points();
  await click("Награды");
  await click("Заявки"); // oldest request first; next to pending
  if (!last.state.buttons.some((b: any) => b.label === "Принять"))
    await click("Следующая");
  await click("Принять");
  if (!last.state.buttons.some((b: any) => b.label === "Отметить выдачу"))
    await click("Следующая");
  await click("Отметить выдачу");
  await raw({ viewer: 900002, action: cancel });
  await as(900002);
  await points();
  assert.equal(last.balance, 3);
  assert.equal(last.xp, 10);
  // Privacy and racing claims, edited XSS text; a stale donor button cannot buy an edited gift.
  await as(900001);
  await home();
  await click("Подарки");
  await click("Добавить желание");
  const giftTitle = "Книга — семейный подарок 8241";
  const giftNote = "<img src=x onerror=alert(1)> Семейная заметка 8241";
  const giftURL = "https://example.invalid/family-gift-8241";
  await send(giftTitle);
  await send("javascript:alert(1)");
  assert.match(last.state.text, /ссылк|URL|http/i);
  await send(giftURL);
  await send(giftNote);
  await click("Пропустить");
  await click("Сохранить");
  const owner = JSON.stringify({
    ...last.state,
    buttons: last.state.buttons.map((b: any) => b.label),
  });
  await as(900002);
  await click("Подарки");
  const bclaim = token("Забронировать тайно");
  await as(900003);
  await click("Подарки");
  const cclaim = token("Забронировать тайно");
  const race = await Promise.all([
    raw({ viewer: 900002, action: bclaim }),
    raw({ viewer: 900003, action: cclaim }),
  ]);
  assert.equal(
    race.filter((r) =>
      r.state?.buttons.some((b: any) => b.label === "Уже куплено"),
    ).length,
    1,
  );
  await as(900001);
  await click("Подарки");
  assert.equal(
    JSON.stringify({
      ...last.state,
      buttons: last.state.buttons.map((b: any) => b.label),
    }),
    owner,
  );
  await click("Подробнее");
  assert.ok(!/брон|куплено/.test(last.payload.text));
  // Schedule edit, pause/resume, invalid input and cancelled dialog; old completion rejected.
  await task("Полить цветы");
  const stale = token("Выполнено");
  await click("Параметры");
  await click("Повторение");
  await send("вт 99:00");
  assert.match(last.payload.text, /врем|Пример|час|распис/i);
  await send("вт 19:00");
  await raw({ action: stale });
  await home();
  await click("Сегодня");
  await click("Параметры");
  await click("Изменить");
  await click("Пауза / отмена");
  await click("Приостановить");
  await click("Параметры");
  await click("Изменить");
  await click("Пауза / отмена");
  await click("Продолжить");
  await click("Параметры");
  await click("Изменить");
  await click("Название и баллы");
  await send("/cancel");
  await home();
  await click("Сегодня");
  assert.equal(last.state.title, "Полить цветы");
  // Ownership safety: cannot expel self; removed member's old token stops working.
  await home();
  await click("Настройки");
  await click("Участники");
  await click("Управление");
  await click("Исключить");
  await click("Подтвердить");
  assert.match(last.payload.text, /владел|нельзя|себ|передайте управление/i);
  await as(900003);
  await click("Подарки");
  // Positive control: C really could read these fields before removal. Otherwise
  // an empty/error response could make a privacy assertion pass vacuously.
  await click("Подробнее");
  for (const value of [giftTitle, giftNote, giftURL]) {
    assert.ok(last.payload.text.includes(value), `Member cannot read ${value}`);
  }
  await home();
  await click("Подарки");
  const oldGift = token("Подробнее");
  const oldGiftCard = last;
  await as(900001);
  await click("Настройки");
  await click("Участники");
  await click("Следующий");
  await click("Следующий");
  await click("Управление");
  await click("Исключить");
  await click("Подтвердить");
  assert.equal(last.members.length, 2, "C was not removed from the household");
  await as(900003);
  assert.equal(last.family_id, 0, "Removed member still has an active household");
  // Reopen the exact previously delivered card and click it through the real
  // HTML bridge. Telegram users retain old messages after membership changes.
  await page.evaluate((result: any) => (window as any).previewReceive(result), oldGiftCard);
  const gone = await click("Подробнее");
  const assertRevoked = (response: any) => {
    assert.match(response.payload?.text || "", /Доступ к этому дому закончился/);
    assert.equal(response.state, null, "Denied response exposed a family card");
    assert.equal(response.family_id, 0);
    for (const key of ["members", "tasks_open", "shopping_items", "reward_catalog", "redemptions", "attempts"]) {
      assert.deepEqual(response[key] ?? [], [], `Revoked member received ${key}`);
    }
    assert.equal(response.balance, 0);
    assert.equal(response.week, 0);
    assert.equal(response.xp, 0);
    assert.ok(response.deliveries.length > 0, "Denial must produce a fresh response");
    for (const delivery of response.deliveries) {
      assert.equal(delivery.Viewer, 900003);
      assert.equal(delivery.Method, "sendMessage", "Revoked member received media/widget delivery");
      assert.match(JSON.parse(delivery.Payload).text || "", /Доступ к этому дому закончился/);
    }
    // Include every payload and queued delivery, not only a literal HTML tag:
    // harmless escaping must not disguise leaked title, note, URL or photo IDs.
    const serialized = JSON.stringify(response);
    for (const value of [giftTitle, "Семейная заметка 8241", giftURL, "fixture_photo_one", "fixture_photo_two"]) {
      assert.ok(!serialized.includes(value), `Revoked member received private value: ${value}`);
    }
  };
  assertRevoked(gone);
  await page.locator("#reply").waitFor({ state: "visible" });
  assert.match(await page.locator("#reply").textContent(), /Доступ к этому дому закончился/);
  assert.equal(await page.locator("iframe").isVisible(), false, "Old family card stayed visible after denial");
  assertRevoked(await raw({ action: oldGift }));
  // Ownership transfer immediately revokes old admin actions; invitations can be revoked.
  await as(900001);
  await points();
  await click("Premium");
  const off = token("Выключить тестовый Premium");
  await home();
  await click("Настройки");
  await click("Участники");
  await click("Следующий");
  await click("Управление");
  await click("Передать управление");
  await click("Подтвердить");
  const deniedOff = await raw({ action: off });
  assert.match(deniedOff.payload.text, /владельцу/);
  await as(900002);
  await points();
  await click("Premium");
  assert.match(last.state.text, /включён/);
  await home();
  await click("Настройки");
  await click("Управление");
  await click("Отозвать приглашения");
  await as(900004);
  const otherId = last.family_id;
  await send("/start join_" + invite);
  assert.equal(last.family_id, otherId);
  assert.match(last.payload.text, /приглашен|ссылк|Действие недоступно/i);
  await as(900001);
  await click("Настройки");
  await click("Выйти из дома");
  await click("Подтвердить выход");
  assert.equal(last.family_id, 0);
  assert.deepEqual(errors, []);
  fs.mkdirSync(".tools", { recursive: true });
  fs.writeFileSync(".tools/e2e-cards.json", JSON.stringify(cards));
  console.log(
    `PASS HTML household E2E: photo/reject/retry/approval, premium, rewards/refund/delivery, gift race/privacy, schedules/pause, membership; ${cards.length} rendered states`,
  );
} finally {
  await browser?.close();
  server.kill("SIGTERM");
  await new Promise((r) => server.once("exit", r));
}
