const numberFields = {
  users: [
    "totalRegistered",
    "newRegistered",
    "activeUsers",
    "d1Eligible",
    "d1Retained",
    "d1RetentionRate",
    "d7Eligible",
    "d7Retained",
    "d7RetentionRate",
  ],
  engagement: [
    "questions",
    "activeSessions",
    "questionsPerActiveUser",
    "averageSessionTurns",
  ],
  quality: [
    "runs",
    "completedRuns",
    "failedRuns",
    "cancelledRuns",
    "successRate",
    "noSourceAnswers",
    "noSourceRate",
    "unhelpfulFeedback",
  ],
  performance: [
    "averageLatencyMs",
    "p50LatencyMs",
    "p95LatencyMs",
    "averageTtftMs",
    "p95TtftMs",
  ],
  cost: [
    "inputTokens",
    "outputTokens",
    "estimatedCostMicroRmb",
    "estimatedCostPerQuestionMicroRmb",
    "cacheHits",
    "cacheHitRate",
  ],
};
const dailyFields = [
  "newUsers",
  "activeUsers",
  "questions",
  "runs",
  "completedRuns",
  "failedRuns",
  "estimatedCostMicroRmb",
];
const numeric = (value) =>
  typeof value === "number" &&
  Number.isFinite(value) &&
  value >= 0 &&
  value <= Number.MAX_SAFE_INTEGER;

export function parseOverview(data) {
  const invalid = () => {
    throw new Error("统计数据不完整或格式有误，请联系维护人员。");
  };
  if (
    !data ||
    !data.window ||
    !["schoolId", "timeZone", "from", "to"].every(
      (key) => typeof data.window[key] === "string" && data.window[key],
    )
  )
    invalid();
  if (
    ![data.generatedAt, data.window.from, data.window.to].every(
      (value) =>
        typeof value === "string" && Number.isFinite(Date.parse(value)),
    )
  )
    invalid();
  try {
    new Intl.DateTimeFormat("zh-CN", {
      timeZone: data.window.timeZone,
    }).format();
  } catch {
    invalid();
  }
  if (Date.parse(data.window.from) >= Date.parse(data.window.to)) invalid();
  for (const [group, keys] of Object.entries(numberFields)) {
    if (!data[group] || !keys.every((key) => numeric(data[group][key])))
      invalid();
  }
  for (const key of ["routes", "errorCodes"]) {
    if (
      !Array.isArray(data[key]) ||
      !data[key].every(
        (row) => row && typeof row.name === "string" && numeric(row.count),
      )
    )
      invalid();
  }
  if (
    !Array.isArray(data.topQuestions) ||
    !data.topQuestions.every(
      (row) => row && typeof row.question === "string" && numeric(row.count),
    )
  )
    invalid();
  if (
    !Array.isArray(data.daily) ||
    !data.daily.every(
      (row) =>
        row &&
        validDate(row.date) &&
        dailyFields.every((key) => numeric(row[key])),
    )
  )
    invalid();
  const dates = data.daily.map((row) => row.date);
  if (
    new Set(dates).size !== dates.length ||
    dates.some((date, i) => i > 0 && date <= dates[i - 1])
  )
    invalid();
  return data;
}

export function validDate(value) {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value))
    return false;
  const date = new Date(value + "T00:00:00Z");
  return (
    Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value
  );
}

export function validateRange(from, to) {
  if (!validDate(from) || !validDate(to)) return "请选择开始和结束日期。";
  const days = (Date.parse(to) - Date.parse(from)) / 86400000 + 1;
  if (days <= 0) return "结束日期不能早于开始日期。";
  if (days > 90) return "日期范围最多为 90 天（包含结束当天）。";
  return "";
}

export function dateInZone(value, timeZone) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(value));
  const part = (type) => parts.find((p) => p.type === type).value;
  return `${part("year")}-${part("month")}-${part("day")}`;
}

export function rangeForDays(days, now, timeZone) {
  const to = dateInZone(now, timeZone);
  const start = new Date(to + "T00:00:00Z");
  start.setUTCDate(start.getUTCDate() - days + 1);
  return { from: start.toISOString().slice(0, 10), to };
}

export const integer = (value) =>
  new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
export const decimal = (value) =>
  new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 }).format(value);
export const percent = (value, denominator) =>
  denominator > 0
    ? new Intl.NumberFormat("zh-CN", {
        style: "percent",
        maximumFractionDigits: 1,
      }).format(value)
    : "—";
export const money = (micro) =>
  new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
    minimumFractionDigits: 2,
    maximumFractionDigits: micro > 0 && micro < 10000 ? 6 : 4,
  }).format(micro / 1e6);
export const duration = (ms) =>
  ms > 0 ? (ms >= 1000 ? `${decimal(ms / 1000)} s` : `${decimal(ms)} ms`) : "—";
export const routeName = (value) =>
  ({
    knowledge: "知识库",
    hybrid: "知识库 + 官网",
    web_search: "官网检索",
    cache: "答案缓存",
    controlled: "受控回答",
    unresolved: "未完成路由",
  })[value] || value;
export const errorName = (value) =>
  ({
    cancelled: "主动取消",
    server_restarted: "服务重启",
    web_search_provider_error: "官网检索失败",
    knowledge_provider_error: "知识检索失败",
    llm_provider_error: "模型调用失败",
    internal_error: "内部错误",
  })[value] || value;

export function chartScale(values) {
  const max = Math.max(1, ...values);
  const step = 10 ** Math.floor(Math.log10(max));
  return Math.ceil(max / step) * step;
}
