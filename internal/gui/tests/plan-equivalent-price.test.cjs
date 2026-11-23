// Run with Node's test runner and Playwright on the module path; see README.md.
// A provider's plan-equivalent price (provider.PlanPrice): its editor sets
// plan price (per month/week) and full quota in API USD (per week/month),
// refuses bad inputs, saves them and clears them with {}; the Usage view
// displays the plan-equivalent cost beside the official API price ("API $X")
// when calls are on a plan. In all five UI languages, Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const unfold = async (page) => {
  const adv = page.locator(".editor details.advanced");
  await adv.waitFor({ state: "attached" });
  if (!(await adv.evaluate((d) => d.open))) await adv.locator(":scope > summary").click();
};

const gemini = {
  id: "gemini", name: "Gemini", icon: "gemini-color", chat: "https://gemini.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "gemini-2.5", name: "Gemini 2.5", on: true }], agents: [], fallback: [], headers: {},
  key: { set: true, masked: "sk-…gem" }, keyList: [], balanceToken: { takes: false, set: false }, proxy: "", maxConcurrency: null,
};
const codex = {
  id: "codex", name: "Codex", icon: "codex-color", chat: "", responses: "", anthropic: "", catalog: "",
  models: [{ id: "gpt-6", name: "GPT-6", on: true }], agents: [], fallback: [], headers: {}, keyList: [],
  account: { agent: "codex", agentName: "Codex", user: "me@example.com", plan: "PLUS", logins: [{ user: "me@example.com", plan: "PLUS", active: true, on: true }] },
  proxy: "", maxConcurrency: null,
  plan: { price: 20, pricePer: "month", quota: 60, quotaPer: "week", models: { "gpt-6": 100 } },
};

function serve(lang, posts) {
  const providers = { providers: [{ ...gemini }, { ...codex }], presets: [], excluded: [], gateway: { running: true, window: true } };
  const settings = { lang, theme: "light", quotaLeft: false, currency: "usd" };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage") {
      return json({
        period: url.searchParams.get("period") || "today",
        calls: 4, errors: 0, input: 4000000, output: 0, cacheRead: 0, reasoning: 0, unpriced: 0,
        cost: 24, planCost: 13, planAPI: 20, planned: 3, bucket: "hour",
        series: [{ t: new Date().toISOString(), label: "10", input: 4000000, output: 0, calls: 4, cost: 24, planCost: 13, planAPI: 20, planned: 3 }],
        agents: [{ id: "codex", name: "Codex", calls: 4, errors: 0, input: 4000000, output: 0, cost: 24, planCost: 13, planAPI: 20, planned: 3 }],
        models: [
          { id: "codex/gpt-6", provider: "codex", model: "gpt-6", calls: 3, errors: 0, input: 3000000, output: 0, cost: 20, planCost: 13, planAPI: 20, planned: 3 },
          { id: "gemini/gemini-2.5", provider: "gemini", model: "gemini-2.5", calls: 1, errors: 0, input: 1000000, output: 0, cost: 4 },
        ],
        path: "",
      });
    }
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/sessions") return json({ sessions: [], dirs: [] });
    if (url.pathname === "/api/sessions/stats") return json({ from: "", to: "", days: [], agents: {} });
    if (url.pathname.startsWith("/api/provider/") && route.request().method() === "POST") {
      const body = route.request().postDataJSON();
      posts.push({ action: url.pathname.slice("/api/provider/".length), body });
      const saved = providers.providers.find((p) => p.id === body.id);
      if (saved && body.plan && Object.keys(body.plan).length) saved.plan = body.plan;
      else if (saved && body.plan) delete saved.plan;
      return json(providers);
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const words = {
  en: { save: "Save", planLabel: "at plan prices", perWeek: "per week", perMonth: "per month" },
  zh: { save: "保存", planLabel: "按套餐价格", perMonth: "每月" },
  "zh-TW": { save: "儲存", planLabel: "按方案價格", perMonth: "每月" },
  ja: { save: "保存", planLabel: "プラン価格", perMonth: "1か月あたり" },
  de: { save: "Speichern", planLabel: "zu Tarifpreisen", perMonth: "pro Monat" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    const w = words[lang];

    test(`${engine} ${lang}: provider plan price is edited, validated, saved and cleared`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, posts));
      await page.goto("http://magpie.test/?view=providers");

      // signed-in account opens with its saved plan
      await page.locator(".row.provider", { hasText: "Codex" }).click();
      await unfold(page);
      await page.locator(".editor input.plan-price").waitFor();
      assert.equal(await page.locator(".editor input.plan-price").inputValue(), "20");
      assert.equal(await page.locator(".editor input.plan-quota").inputValue(), "60");
      assert.equal(await page.locator(".editor .plan-model input").first().inputValue(), "gpt-6");
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      for (let i = 0; i < 50 && posts.length === 0; i++) await page.waitForTimeout(50);
      assert.deepEqual(posts.at(-1).body.plan, { price: 20, pricePer: "month", quota: 60, quotaPer: "week", models: { "gpt-6": 100 } });

      // edit a key provider's plan: missing quota is rejected before POST
      await page.locator(".row.provider", { hasText: "Gemini" }).click();
      await unfold(page);
      await page.locator(".editor input.plan-price").waitFor();
      await page.locator(".editor input.plan-price").fill("20");
      const beforeBad = posts.length;
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      await page.locator(".editor .editor-error").waitFor();
      assert.equal(posts.length, beforeBad, "missing quota must not post");

      await page.locator(".editor .plan-quota + .segs").getByRole("button", { name: w.perMonth, exact: true }).click();
      const advanced = page.locator(".editor details.advanced");
      await advanced.locator(".stack > button").filter({ hasText: new RegExp({ en: "Add model quota", zh: "添加模型额度", "zh-TW": "新增模型額度", ja: "モデルの利用枠を追加", de: "Modellkontingent hinzufügen" }[lang]) }).click();
      const override = page.locator(".editor .plan-model");
      await override.locator("input").nth(0).fill("gemini-2.5");
      await override.locator("input").nth(1).fill("2580");
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      for (let i = 0; i < 50 && posts.length === beforeBad; i++) await page.waitForTimeout(50);
      assert.equal(posts.at(-1).action, "save");
      assert.deepEqual(posts.at(-1).body.plan, { price: 20, pricePer: "month", quotaPer: "month", models: { "gemini-2.5": 2580 } });

      // The saved override reloads. Removing it and clearing the price posts {}.
      await page.locator(".row.provider", { hasText: "Gemini" }).click();
      await unfold(page);
      await page.locator(".editor input.plan-price").waitFor();
      assert.equal(await override.locator("input").nth(1).inputValue(), "2580");
      await override.getByRole("button").click();
      await page.locator(".editor input.plan-price").fill("");
      const prev = posts.length;
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      for (let i = 0; i < 50 && posts.length === prev; i++) await page.waitForTimeout(50);
      assert.deepEqual(posts.at(-1).body.plan, {});
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: Usage shows plan-equivalent cost beside API cost`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 760 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, []));
      await page.goto("http://magpie.test/?view=usage");
      await page.locator("#usageCost b").waitFor();

      // $24 total API - $20 plan API + $13 plan cost = $17.00, with API $24.00 beside it
      assert.equal(await page.locator("#usageCost b").textContent(), "$17.00");
      assert.equal(await page.locator("#usageCost .api-cost").textContent(), "API $24.00");
      assert((await page.locator("#usageCost").textContent()).includes(w.planLabel));

      // model row on a plan shows plan cost with API cost tooltip; non-plan row has no API tooltip
      const plannedRow = page.locator('#usageModels .row.stat[data-id="codex/gpt-6"] .cost');
      assert.equal(await plannedRow.textContent(), "$13.00");
      assert.equal(await plannedRow.getAttribute("title"), "API $20.00");
      const paygRow = page.locator('#usageModels .row.stat[data-id="gemini/gemini-2.5"] .cost');
      assert.equal(await paygRow.textContent(), "$4.00");
      assert.equal(await paygRow.getAttribute("title"), "");
      assert.deepEqual(errors, []);
    });
  }
}
