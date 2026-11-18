// Every distinct projection produced by the detailed HTML journeys also runs in
// Telegram's official QuickJS/DOM/renderer. Browser gestures stay a separate level.
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";
import assert from "node:assert/strict";
const root = process.env.WIDGET_SANDBOX!;
const { runWidget } = await import(
  pathToFileURL(path.join(root, "packages/core/scripts/sandbox-node.ts")).href
);
const require = createRequire(path.join(root, "package.json"));
const { chromium } = require("@playwright/test");
const launch = chromium.launch.bind(chromium);
let screenshot = "";
chromium.launch = async (...args: any[]) => {
  const browser = await launch(...args),
    close = browser.close.bind(browser);
  browser.close = async () => {
    if (screenshot) {
      for (const context of browser.contexts())
        for (const page of context.pages())
          await page.screenshot({ path: screenshot });
      screenshot = "";
    }
    return close();
  };
  return browser;
};
const html = fs.readFileSync("widgets/family.html", "utf8");
const unique = new Map<string, any>();
for (const file of [
  "e2e-cards",
  "plans-e2e-cards",
  "shopping-e2e-cards",
  "assistant-e2e-cards",
  "agent-e2e-cards",
  "agent-resources-e2e-cards",
  "agent-review-cards",
  "agent-recovery-cards",
  "agent-family-cards",
  "agent-reply-cards",
])
  for (const c of JSON.parse(fs.readFileSync(`.tools/${file}.json`, "utf8"))) {
    const key = JSON.stringify({
      ...c,
      buttons: c.buttons.map((b: any) => b.label),
    });
    unique.set(key, c);
  }
let n = 0;
for (const c of unique.values()) {
  assert.ok(Buffer.byteLength(JSON.stringify(c)) <= 1024, c.title);
  const r = await runWidget(html, {
    width: 200,
    layout: true,
    data: { chatData: JSON.stringify(c), publicData: null },
  });
  assert.deepEqual(r.diags, [], c.title);
  assert.ok(r.height <= 600, `${c.title} height ${r.height}`);
  n++;
}
for (const title of [
  "Набор: Кухня",
  "Чередование дел",
  "Неделя нашего дома",
  "Проверьте покупки",
  "Проверьте изменения",
  "Помощник по дому",
]) {
  const c = [...unique.values()].find((c) => c.title === title && (title !== "Помощник по дому" || c.text.startsWith("Контекст:")));
  assert.ok(c, title);
  for (const width of [320, 390])
    for (const theme of ["dark", "custom"]) {
      if (width === 320 && theme === "custom")
        screenshot =
          "docs/screenshots/" +
          (
            {
              "Набор: Кухня": "preset",
              "Чередование дел": "rotation",
              "Неделя нашего дома": "weekly",
              "Проверьте покупки": "shopping",
              "Проверьте изменения": "agent-plan",
              "Помощник по дому": "agent-context",
            } as any
          )[title] +
          ".png";
      const r = await runWidget(html, {
        width,
        layout: true,
        theme: theme === "dark" ? "dark" : "light",
        ...(theme === "custom"
          ? {
              themeParams: {
                bg_color: "#edf4e8",
                text_color: "#183020",
                button_color: "#345a40",
              },
            }
          : {}),
        data: { chatData: JSON.stringify(c), publicData: null },
      });
      assert.deepEqual(r.diags, []);
      assert.ok(r.height <= 600);
    }
}
console.log(
  `PASS official runtime: ${n} distinct E2E projections at 200px + 24 screen/theme/width combinations`,
);
