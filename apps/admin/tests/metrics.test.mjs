import test from "node:test";
import assert from "node:assert/strict";
import {
  parseOverview,
  validateRange,
  rangeForDays,
  percent,
  money,
  duration,
  chartScale,
} from "../web/metrics.js";

export function emptyOverview() {
  return {
    generatedAt: "2026-09-03T00:00:00Z",
    window: {
      schoolId: "eval",
      from: "2026-09-01T00:00:00Z",
      to: "2026-09-04T00:00:00Z",
      timeZone: "Asia/Shanghai",
    },
    users: {
      totalRegistered: 0,
      newRegistered: 0,
      activeUsers: 0,
      d1Eligible: 0,
      d1Retained: 0,
      d1RetentionRate: 0,
      d7Eligible: 0,
      d7Retained: 0,
      d7RetentionRate: 0,
    },
    engagement: {
      questions: 0,
      activeSessions: 0,
      questionsPerActiveUser: 0,
      averageSessionTurns: 0,
    },
    quality: {
      runs: 0,
      completedRuns: 0,
      failedRuns: 0,
      cancelledRuns: 0,
      successRate: 0,
      noSourceAnswers: 0,
      noSourceRate: 0,
      unhelpfulFeedback: 0,
    },
    performance: {
      averageLatencyMs: 0,
      p50LatencyMs: 0,
      p95LatencyMs: 0,
      averageTtftMs: 0,
      p95TtftMs: 0,
    },
    cost: {
      inputTokens: 0,
      outputTokens: 0,
      estimatedCostMicroRmb: 0,
      estimatedCostPerQuestionMicroRmb: 0,
      cacheHits: 0,
      cacheHitRate: 0,
    },
    routes: [],
    errorCodes: [],
    topQuestions: [],
    daily: [],
  };
}

test("empty response is valid but missing/invalid metrics are not zero-filled", () => {
  assert.deepEqual(parseOverview(emptyOverview()), emptyOverview());
  for (const value of [null, NaN, Infinity, -1, "3", undefined]) {
    const data = emptyOverview();
    data.quality.runs = value;
    assert.throws(() => parseOverview(data));
  }
  const missing = emptyOverview();
  delete missing.cost;
  assert.throws(() => parseOverview(missing));
  const zone = emptyOverview();
  zone.window.timeZone = "Bad/Zone";
  assert.throws(() => parseOverview(zone));
});

test("zero denominators are shown as no sample; real zero rates are preserved", () => {
  assert.equal(percent(0, 0), "—");
  assert.equal(percent(0, 10), "0%");
  assert.equal(percent(0.125, 8), "12.5%");
  assert.equal(duration(0), "—");
});

test("RMB micro-units preserve tiny non-zero costs", () => {
  const numeric = (value) => Number(value.replace(/[^0-9.]/g, ""));
  assert.equal(numeric(money(1000000)), 1);
  assert.equal(numeric(money(125000)), 0.125);
  assert.equal(numeric(money(1)), 0.000001);
});

test("inclusive date filters validate dates and the 90-day boundary", () => {
  assert.equal(validateRange("2026-06-01", "2026-08-29"), "");
  assert.notEqual(validateRange("2026-06-01", "2026-08-30"), "");
  assert.equal(validateRange("2026-09-03", "2026-09-03"), "");
  assert.notEqual(validateRange("2026-02-30", "2026-03-02"), "");
  assert.notEqual(validateRange("2026-09-04", "2026-09-03"), "");
});

test("presets use reporting timezone and calendar days across DST", () => {
  assert.deepEqual(
    rangeForDays(7, new Date("2026-09-02T20:00:00Z"), "Asia/Shanghai"),
    { from: "2026-08-28", to: "2026-09-03" },
  );
  assert.deepEqual(
    rangeForDays(7, new Date("2026-03-10T05:00:00Z"), "America/New_York"),
    { from: "2026-03-04", to: "2026-03-10" },
  );
  assert.equal(chartScale([0, 0]), 1);
  assert.equal(chartScale([27, 14]), 30);
});

test("duplicate or reordered daily rows are rejected", () => {
  const row = {
    date: "2026-09-01",
    newUsers: 0,
    activeUsers: 0,
    questions: 0,
    runs: 0,
    completedRuns: 0,
    failedRuns: 0,
    estimatedCostMicroRmb: 0,
  };
  const data = emptyOverview();
  data.daily = [row, row];
  assert.throws(() => parseOverview(data));
});
