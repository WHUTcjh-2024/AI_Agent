import { request, ApiError } from "./api.js";
import {
  parseOverview,
  validateRange,
  dateInZone,
  rangeForDays,
} from "./metrics.js";
import { renderOverview } from "./render.js";

const byId = (id) => document.getElementById(id);
let timeZone = null;
let controller = null;
let requestID = 0;
let requested = {};

function showLogin(message = "") {
  controller?.abort();
  requestID++;
  timeZone = null;
  requested = {};
  byId("filter-error").textContent = "";
  byId("app-view").hidden = true;
  byId("dashboard").hidden = true;
  for (const id of [
    "kpis",
    "trend-chart",
    "daily-table",
    "routes",
    "quality-metrics",
    "cost-metrics",
    "user-metrics",
    "top-questions",
    "errors",
  ])
    byId(id).replaceChildren();
  byId("login-view").hidden = false;
  byId("login-error").textContent = message;
  byId("password").value = "";
  byId("password").focus();
}

function showApp() {
  byId("login-view").hidden = true;
  byId("app-view").hidden = false;
}

async function load(filters = requested) {
  requested = { ...filters };
  controller?.abort();
  controller = new AbortController();
  const id = ++requestID;
  byId("dashboard").hidden = true;
  byId("error-state").hidden = true;
  byId("loading").hidden = false;
  byId("sync-status").textContent = "正在同步";
  byId("refresh").disabled = true;
  try {
    const query = new URLSearchParams(filters).toString();
    const data = parseOverview(
      await request("/api/overview" + (query ? "?" + query : ""), {
        signal: controller.signal,
      }),
    );
    if (id !== requestID) return;
    timeZone = data.window.timeZone;
    byId("from").value = filters.from || dateInZone(data.window.from, timeZone);
    byId("to").value =
      filters.to || dateInZone(Date.parse(data.window.to) - 1, timeZone);
    renderOverview(data);
    byId("dashboard").hidden = false;
  } catch (error) {
    if (id !== requestID || error.name === "AbortError") return;
    if (error instanceof ApiError && error.status === 401) {
      showLogin(error.message);
      return;
    }
    byId("sync-status").textContent = "同步失败";
    byId("error-message").textContent =
      error.message || "网络连接失败，请稍后重试。";
    byId("error-state").hidden = false;
  } finally {
    if (id === requestID) {
      byId("loading").hidden = true;
      byId("refresh").disabled = false;
    }
  }
}

byId("login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  byId("login-error").textContent = "";
  byId("login-button").disabled = true;
  try {
    await request("/api/session", {
      method: "POST",
      body: { password: byId("password").value },
    });
    byId("password").value = "";
    requested = {};
    showApp();
    await load({});
  } catch (error) {
    byId("login-error").textContent = error.message;
  } finally {
    byId("login-button").disabled = false;
  }
});

byId("logout").addEventListener("click", async () => {
  byId("logout").disabled = true;
  controller?.abort();
  requestID++;
  try {
    await request("/api/session", { method: "DELETE" });
    showLogin();
  } catch {
    byId("sync-status").textContent = "退出失败，请重试";
    byId("loading").hidden = true;
    byId("refresh").disabled = false;
    byId("error-message").textContent =
      "退出请求未完成，会话可能仍然有效，请重试退出。";
    byId("dashboard").hidden = true;
    byId("error-state").hidden = false;
  } finally {
    byId("logout").disabled = false;
  }
});
byId("refresh").addEventListener("click", () => load());
byId("retry").addEventListener("click", () => load());

byId("filters").addEventListener("submit", (event) => {
  event.preventDefault();
  const range = { from: byId("from").value, to: byId("to").value };
  byId("filter-error").textContent = validateRange(range.from, range.to);
  if (!byId("filter-error").textContent) {
    document
      .querySelectorAll("[data-days]")
      .forEach((button) => button.classList.remove("selected"));
    load(range);
  }
});

document.querySelectorAll("[data-days]").forEach((button) =>
  button.addEventListener("click", () => {
    if (!timeZone) {
      byId("filter-error").textContent = "请先成功加载数据，再选择快捷日期。";
      return;
    }
    const range = rangeForDays(
      Number(button.dataset.days),
      new Date(),
      timeZone,
    );
    byId("from").value = range.from;
    byId("to").value = range.to;
    byId("filter-error").textContent = "";
    document
      .querySelectorAll("[data-days]")
      .forEach((item) => item.classList.toggle("selected", item === button));
    load(range);
  }),
);
document
  .querySelectorAll(".sidebar nav a")
  .forEach((link) =>
    link.addEventListener("click", () =>
      document
        .querySelectorAll(".sidebar nav a")
        .forEach((item) => item.classList.toggle("active", item === link)),
    ),
  );

try {
  await request("/api/session");
  showApp();
  await load({});
} catch {
  showLogin();
}
