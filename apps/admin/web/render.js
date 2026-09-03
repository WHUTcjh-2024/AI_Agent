import {
  integer,
  decimal,
  percent,
  money,
  duration,
  routeName,
  errorName,
  chartScale,
} from "./metrics.js";

export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function metricRow(label, value, note) {
  const row = el("div", "metric-row");
  const description = el("div");
  description.append(el("span", "metric-label", label));
  if (note) description.append(el("small", "metric-note", note));
  row.append(description, el("strong", "metric-value", value));
  return row;
}

function card(label, value, detail, accent) {
  const node = el("article", `kpi-card ${accent || ""}`);
  node.append(
    el("span", "kpi-label", label),
    el("strong", "kpi-value", value),
    el("span", "kpi-detail", detail),
  );
  return node;
}

function table(headers, rows, empty) {
  if (!rows.length) return el("p", "empty-block", empty);
  const node = el("table");
  const head = el("thead");
  const header = el("tr");
  for (const title of headers) {
    const th = el("th", "", title);
    th.scope = "col";
    header.append(th);
  }
  head.append(header);
  const body = el("tbody");
  for (const cells of rows) {
    const tr = el("tr");
    for (const value of cells) tr.append(el("td", "", value));
    body.append(tr);
  }
  node.append(head, body);
  return node;
}

function breakdown(rows, label, empty) {
  const node = el("div", "breakdown");
  if (!rows.length) {
    node.append(el("p", "empty-block", empty));
    return node;
  }
  const total = rows.reduce((sum, row) => sum + row.count, 0);
  rows.forEach((row, index) => {
    const item = el("div", "breakdown-item");
    const heading = el("div", "breakdown-label");
    heading.append(
      el("span", "", label(row.name)),
      el("strong", "", integer(row.count)),
    );
    const meter = el("progress", `bar tone-${index % 5}`);
    meter.max = Math.max(1, total);
    meter.value = row.count;
    meter.setAttribute("aria-label", `${label(row.name)} ${row.count} 次`);
    item.append(heading, meter);
    node.append(item);
  });
  return node;
}

function svgNode(tag, attributes = {}, text) {
  const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [key, value] of Object.entries(attributes))
    node.setAttribute(key, String(value));
  if (text !== undefined) node.textContent = text;
  return node;
}

function trend(rows) {
  if (!rows.length) return el("p", "empty-block", "暂无逐日记录");
  const svg = svgNode("svg", {
    viewBox: "0 0 760 256",
    role: "img",
    "aria-label": "每日提问数与活跃用户，详细数值可在下方明细查看",
  });
  const width = 690;
  const left = 44;
  const top = 16;
  const height = 192;
  const ceiling = chartScale(
    rows.flatMap((row) => [row.questions, row.activeUsers]),
  );
  const ticks = ceiling < 4 ? ceiling : 4;
  for (let i = 0; i <= ticks; i++) {
    const value = (ceiling * i) / ticks;
    const y = top + height * (1 - i / ticks);
    svg.append(
      svgNode("line", {
        x1: left,
        y1: y,
        x2: left + width,
        y2: y,
        class: "grid-line",
      }),
      svgNode(
        "text",
        { x: left - 10, y: y + 4, "text-anchor": "end", class: "axis-label" },
        decimal(value),
      ),
    );
  }
  const slot = width / rows.length;
  const bar = Math.min(16, slot * 0.3);
  rows.forEach((row, index) => {
    const x = left + slot * (index + 0.5);
    for (const [offset, field, className] of [
      [-bar - 1, "questions", "question-bar"],
      [1, "activeUsers", "user-bar"],
    ]) {
      const h = (height * row[field]) / ceiling;
      const rect = svgNode("rect", {
        x: x + offset,
        y: top + height - h,
        width: bar,
        height: h,
        rx: 2,
        class: className,
      });
      rect.append(
        svgNode(
          "title",
          {},
          `${row.date} ${field === "questions" ? "提问" : "活跃用户"} ${row[field]}`,
        ),
      );
      svg.append(rect);
    }
    if (
      index % Math.max(1, Math.ceil(rows.length / 7)) === 0 ||
      index === rows.length - 1
    )
      svg.append(
        svgNode(
          "text",
          {
            x,
            y: top + height + 28,
            "text-anchor": "middle",
            class: "axis-label",
          },
          row.date.slice(5).replace("-", "/"),
        ),
      );
  });
  return svg;
}

export function renderOverview(data) {
  const set = (id, ...nodes) =>
    document.getElementById(id).replaceChildren(...nodes);
  const { users: u, engagement: e, quality: q, performance: p, cost: c } = data;
  set(
    "kpis",
    card(
      "提问总量",
      integer(e.questions),
      `${integer(e.activeSessions)} 个活跃会话`,
      "accent",
    ),
    card(
      "期间活跃用户",
      integer(u.activeUsers),
      `新增 ${integer(u.newRegistered)} 位用户`,
    ),
    card(
      "运行完成率",
      percent(q.successRate, q.runs),
      `${integer(q.completedRuns)} / ${integer(q.runs)} 次运行完成`,
    ),
    card(
      "估算调用成本",
      money(c.estimatedCostMicroRmb),
      e.questions
        ? `平均 ${money(c.estimatedCostPerQuestionMicroRmb)} / 问题`
        : "暂无可计算问题",
    ),
  );
  set("trend-chart", trend(data.daily));
  set(
    "daily-table",
    table(
      ["日期", "提问", "活跃用户", "完成运行", "失败运行", "估算成本"],
      data.daily.map((row) => [
        row.date,
        integer(row.questions),
        integer(row.activeUsers),
        integer(row.completedRuns),
        integer(row.failedRuns),
        money(row.estimatedCostMicroRmb),
      ]),
      "暂无逐日明细",
    ),
  );
  set("routes", breakdown(data.routes, routeName, "尚无路由记录"));
  set(
    "quality-metrics",
    metricRow("失败运行", integer(q.failedRuns)),
    metricRow("主动取消", integer(q.cancelledRuns)),
    metricRow(
      "无引用回答",
      integer(q.noSourceAnswers),
      "有检索需求、已完成但无引用",
    ),
    metricRow("负向反馈", integer(q.unhelpfulFeedback)),
    metricRow(
      "答案缓存命中",
      percent(c.cacheHitRate, q.runs),
      `${integer(c.cacheHits)} / ${integer(q.runs)} 次运行`,
    ),
  );
  set(
    "cost-metrics",
    metricRow(
      "首字延迟 · 平均 / P95",
      `${duration(p.averageTtftMs)} / ${duration(p.p95TtftMs)}`,
    ),
    metricRow(
      "总耗时 · P50 / P95",
      `${duration(p.p50LatencyMs)} / ${duration(p.p95LatencyMs)}`,
    ),
    metricRow("平均总耗时", duration(p.averageLatencyMs)),
    metricRow(
      "输入 / 输出 Token",
      `${integer(c.inputTokens)} / ${integer(c.outputTokens)}`,
    ),
    metricRow(
      "估算成本",
      money(c.estimatedCostMicroRmb),
      "依据服务端配置的模型单价",
    ),
  );
  set(
    "user-metrics",
    metricRow("累计注册", integer(u.totalRegistered), "截至统计窗口结束"),
    metricRow(
      "人均提问",
      u.activeUsers ? decimal(e.questionsPerActiveUser) : "—",
      "提问数 / 期间活跃用户",
    ),
    metricRow(
      "每会话提问",
      e.activeSessions ? decimal(e.averageSessionTurns) : "—",
    ),
    metricRow(
      "次日留存 D1",
      percent(u.d1RetentionRate, u.d1Eligible),
      u.d1Eligible
        ? `${integer(u.d1Retained)} / ${integer(u.d1Eligible)} 个可计算样本`
        : "暂无可计算样本",
    ),
    metricRow(
      "七日留存 D7",
      percent(u.d7RetentionRate, u.d7Eligible),
      u.d7Eligible
        ? `${integer(u.d7Retained)} / ${integer(u.d7Eligible)} 个可计算样本`
        : "暂无可计算样本",
    ),
  );
  set(
    "top-questions",
    table(
      ["排名", "问题", "次数"],
      data.topQuestions.map((row, i) => [
        String(i + 1).padStart(2, "0"),
        row.question,
        integer(row.count),
      ]),
      "暂时没有热门问题",
    ),
  );
  set("errors", breakdown(data.errorCodes, errorName, "本时间段没有错误记录"));
  document.getElementById("empty-notice").hidden =
    e.questions !== 0 || q.runs !== 0;
  document.getElementById("school-label").textContent =
    data.window.schoolId.toUpperCase();
  document.getElementById("timezone").textContent = data.window.timeZone;
  const format = (date) =>
    new Intl.DateTimeFormat("zh-CN", {
      timeZone: data.window.timeZone,
      dateStyle: "medium",
      timeStyle: "short",
      hour12: false,
    }).format(new Date(date));
  document.getElementById("window-summary").textContent =
    `统计窗口：${format(data.window.from)} 至 ${format(data.window.to)}（不含结束时刻）`;
  document.getElementById("sync-status").textContent =
    `已同步 ${new Intl.DateTimeFormat("zh-CN", { timeZone: data.window.timeZone, timeStyle: "short", hour12: false }).format(new Date(data.generatedAt))}`;
}
