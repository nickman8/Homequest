import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";
import assert from "node:assert/strict";
const require = createRequire(
  path.join(process.env.WIDGET_SANDBOX!, "package.json"),
);
const { chromium } = require("@playwright/test");
export class Flow {
  server: any;
  browser: any;
  page: any;
  last: any;
  uid = 900001;
  cards: any[] = [];
  errors: string[] = [];
  url: string;
  constructor(port: number) {
    this.url = `http://127.0.0.1:${port}`;
    this.server = spawn("node", ["scripts/preview-server.mjs"], {
      env: {
        ...process.env,
        HOMEQUEST_PREVIEW_PORT: String(port),
        HOMEQUEST_PREVIEW_NOW: "2026-03-01T12:00:00Z",
      },
      stdio: "pipe",
    });
  }
  async start() {
    for (let n = 0; n < 100; n++) {
      try {
        if ((await fetch(this.url)).ok) break;
      } catch {}
      await new Promise((r) => setTimeout(r, 100));
    }
    this.browser = await chromium.launch();
    this.page = await this.browser.newPage();
    this.page.setDefaultTimeout(12000);
    this.page.on("pageerror", (e: any) => this.errors.push(e.message));
    await this.page.goto(this.url);
    await this.page.waitForFunction(() => document.body.dataset.updateId);
    await this.home();
  }
  async effect(fn: () => Promise<any>) {
    const renders = await this.page
      .locator("body")
      .getAttribute("data-render-count");
    const wait = this.page.waitForResponse((r: any) =>
      r.url().endsWith("/update"),
    );
    await fn();
    const r = await wait;
    assert.equal(r.status(), 200);
    this.last = await r.json();
    if (this.last.state) this.cards.push(this.last.state);
    await this.page.waitForFunction(
      (n: any) => Number(document.body.dataset.renderCount) > Number(n),
      renders,
    );
    return this.last;
  }
  async send(text: string) {
    const r = await this.effect(async () => {
      await this.page.locator("#message").fill(text);
      await this.page.locator("#composer button").click();
    });
    await this.page.waitForFunction(
      () => !(document.querySelector("#message") as HTMLInputElement).value,
    );
    return r;
  }
  async click(label: string) {
    const f = this.page.frameLocator("iframe");
    for (let i = 0; i < 15; i++) {
      const b = f.getByRole("button", { name: label, exact: true });
      if (await b.count()) return this.effect(() => b.click());
      const next = f.getByRole("button", {
        name: /^(Далее|Ещё действия|К началу) ·/,
      });
      if (!(await next.count()))
        throw Error(`Missing ${label}: ${JSON.stringify(this.last)}`);
      await next.click();
    }
    throw Error("pagination loop " + label);
  }
  token(label: string) {
    const b = this.last.state?.buttons.find((b: any) => b.label === label);
    assert.ok(b, `Missing ${label}`);
    return b.action;
  }
  home() {
    return this.send("/start");
  }
  async as(uid: number) {
    this.uid = uid;
    return this.effect(() =>
      this.page.locator("#viewer").selectOption(String(uid)),
    );
  }
  async raw(data: any) {
    const r = await this.page.request.post(this.url + "/update", {
      headers: { Origin: this.url },
      data: { viewer: this.uid, ...data },
    });
    assert.equal(r.status(), 200);
    return r.json();
  }
  async inject(data: any) {
    const r = await this.raw(data);
    await this.page.evaluate((r: any) => (window as any).previewReceive(r), r);
    this.last = r;
    if (r.state) this.cards.push(r.state);
    return r;
  }
  async showDelivery(label: string) {
    for (const d of this.last.deliveries || []) {
      const payload = JSON.parse(d.Payload);
      if (!payload.chat_data) continue;
      const state = JSON.parse(payload.chat_data);
      if (state.buttons.some((b: any) => b.label === label)) {
        this.last = { ...this.last, payload, state };
        this.cards.push(state);
        await this.page.evaluate(
          (r: any) => (window as any).previewReceive(r),
          this.last,
        );
        return;
      }
    }
    throw Error("No actual delivery " + label);
  }
  async create() {
    await this.click("Создать дом");
    await this.send("Тёплый дом");
    await this.send("Europe/Berlin");
    await this.click("Настройки");
    await this.click("Пригласить");
    const invite = this.last.payload.text.split("join_")[1];
    for (const n of [900002, 900003]) {
      await this.as(n);
      await this.send("/start join_" + invite);
    }
    await this.as(900004);
    await this.click("Создать дом");
    await this.send("Другой дом");
    await this.send("UTC");
    await this.as(900001);
  }
  async openTask(name: string) {
    await this.home();
    await this.click("Сегодня");
    for (let i = 0; i < 30; i++) {
      if (this.last.state.title === name) return;
      await this.click("Следующее");
    }
    throw Error("Task absent " + name);
  }
  async close() {
    await this.browser?.close();
    this.server.kill("SIGTERM");
    await new Promise((r) => this.server.once("exit", r));
    assert.deepEqual(this.errors, []);
  }
}
