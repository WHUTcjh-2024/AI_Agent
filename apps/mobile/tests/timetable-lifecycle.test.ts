import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import { runInNewContext } from 'node:vm';
import { build } from 'esbuild';
import React from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';

import type { CourseImportResult } from '../src/features/timetable/domain/timetable';
import { normalizeJwappCourses } from '../src/features/timetable/providers/jwapp/jwapp-parser';
import { TIMETABLE_STORAGE_KEY } from '../src/features/timetable/store/timetable-repository';

// Mount the real screen, browser callbacks, provider, schemas, store and repository.
// Only platform services are replaced; browser fetch execution is covered in bridge tests.
const compiled = build({
  stdin: { contents: `export { TimetableScreen } from './src/features/timetable/screens/TimetableScreen';
    export { TimetableStoreProvider, useTimetableStore } from './src/features/timetable/store/timetable-store';`, resolveDir: process.cwd(), loader: 'ts' },
  bundle: true, write: false, platform: 'node', format: 'cjs', packages: 'external', jsx: 'automatic',
});
const nodeRequire = createRequire(import.meta.url);
const home = 'https://jwxt.whut.edu.cn/jwapp/sys/homeapp/index.do';
const fixture = (name = '大学物理') => normalizeJwappCourses({ datas: { cxxskcb: { rows: [
  { KCM: name, SKXQ: 3, KSJC: 3, JSJC: 5, SKZC: '1'.repeat(16) },
] } } }, { datas: { cxxljc: { rows: [{ XQKSRQ: '2026-08-31' }] } } }, '2026-2027-1');

async function mount(disk = new Map<string, string>()) {
  let rejectWrite = false;
  let deferWrite: (() => Promise<void>) | undefined;
  let nonce = 0;
  let timerId = 0;
  const timers = new Map<number, { callback(): void; delay: number }>();
  let state: { timetable: CourseImportResult | null; loading: boolean };
  const storage = {
    getItem: async (key: string) => disk.get(key) ?? null,
    setItem: async (key: string, value: string) => {
      if (rejectWrite) throw new Error('synthetic disk failure');
      await deferWrite?.();
      disk.set(key, value);
    },
    removeItem: async (key: string) => { disk.delete(key); },
  };
  const native = {
    ...Object.fromEntries(['View', 'Text', 'Pressable', 'ScrollView', 'ActivityIndicator', 'Modal'].map((name) => [name, name])),
    Platform: { OS: 'android', select: (values: Record<string, unknown>) => values.android ?? values.default },
    StyleSheet: { create: (value: unknown) => value, hairlineWidth: 1, absoluteFill: {} },
    AppState: { addEventListener: () => ({ remove() {} }) },
    PanResponder: { create: () => ({ panHandlers: {} }) },
  };
  const mocks: Record<string, unknown> = {
    'react-native': native,
    'react-native-webview': { WebView: 'WebView' },
    '@react-native-async-storage/async-storage': storage,
    '@react-navigation/native': { useFocusEffect: function useFocusEffect(callback: React.EffectCallback) { React.useEffect(callback, [callback]); } },
    '@expo/vector-icons': { Ionicons: 'Ionicons' },
    'react-native-safe-area-context': { useSafeAreaInsets: () => ({ top: 0, bottom: 0 }) },
    'expo-crypto': { randomUUID: () => `lifecycle-${++nonce}` },
  };
  const module = { exports: {} as {
    TimetableScreen: React.ComponentType;
    TimetableStoreProvider: React.ComponentType<React.PropsWithChildren>;
    useTimetableStore(): typeof state;
  } };
  runInNewContext((await compiled).outputFiles[0].text, {
    module, exports: module.exports, require: (name: string) => mocks[name] ?? nodeRequire(name),
    __DEV__: false, AbortController, URL, Intl, Date,
    setTimeout: (callback: () => void, delay: number) => { const id = ++timerId; timers.set(id, { callback, delay }); return id; },
    clearTimeout: (id: number) => timers.delete(id), setInterval: () => 0, clearInterval: () => {},
  });
  const { TimetableStoreProvider, TimetableScreen, useTimetableStore } = module.exports;
  function Probe() { state = useTimetableStore(); return null; }
  let renderer: ReactTestRenderer;
  await act(async () => { renderer = create(React.createElement(TimetableStoreProvider, null,
    React.createElement(TimetableScreen), React.createElement(Probe))); });
  const press = async (label: string) => act(async () => {
    const button = renderer.root.findAll((item) => String(item.type) === 'Pressable').find((item) => item.props.accessibilityLabel === label || item.props.children?.props?.children === label);
    assert.ok(button, `Missing button: ${label}`);
    button.props.onPress();
  });
  const browser = () => renderer.root.find((item) => String(item.type) === 'WebView');
  const open = async () => {
    await press(state.timetable ? '重新导入课表' : '导入课表');
    await act(async () => browser().props.onLoadEnd({ nativeEvent: { url: home } }));
  };
  const message = (body: object, requestNonce = `lifecycle-${nonce}`) => ({ nativeEvent: { url: home,
    data: JSON.stringify({ channel: 'asku.timetable', version: 1, requestId: requestNonce, ...body }) } });
  return {
    disk, press, open, browser, message,
    state: () => state,
    renderedText: () => JSON.stringify(renderer.toJSON()),
    failWrite: () => { rejectWrite = true; },
    holdWrite: (wait: () => Promise<void>) => { deferWrite = wait; },
    success: async (payload = fixture()) => act(async () => browser().props.onMessage(message({ type: 'success', payload }))),
    timeout: async () => act(async () => {
      const timer = [...timers.values()].find(({ delay }) => delay === 30000);
      assert.ok(timer); timer.callback();
    }),
    unmount: async () => act(async () => renderer.unmount()),
  };
}

// React's act environment flag applies only to this isolated Node test process.
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

test('real screen lifecycle: empty → import → persisted → remount → updated import', async () => {
  const first = await mount();
  try {
    assert.equal(first.state().loading, false);
    assert.equal(first.state().timetable, null);
    await first.open(); await first.success();
    assert.equal(first.state().timetable?.courses[0].name, '大学物理');
    assert.ok(first.disk.has(TIMETABLE_STORAGE_KEY));
  } finally { await first.unmount(); }
  const reopened = await mount(first.disk);
  try {
    assert.equal(reopened.state().timetable?.courses[0].name, '大学物理');
    await reopened.open(); await reopened.success(fixture('高等数学'));
    assert.equal(reopened.state().timetable?.courses[0].name, '高等数学');
  } finally { await reopened.unmount(); }
  const updated = await mount(first.disk);
  try { assert.equal(updated.state().timetable?.courses[0].name, '高等数学'); }
  finally { await updated.unmount(); }
});

for (const code of ['NETWORK', 'AUTH', 'TIMEOUT', 'FORMAT', 'SYSTEM', 'STORAGE', 'CANCELLED'] as const) {
  test(`${code}: failed reimport preserves rendered state and disk after remount`, async () => {
    const original = JSON.stringify(fixture());
    const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, original]]));
    try {
      await app.open();
      if (code === 'CANCELLED') await app.press('取消导入');
      else if (code === 'STORAGE') { app.failWrite(); await app.success(fixture('新课程')); }
      else if (code === 'TIMEOUT') await app.timeout();
      else if (code === 'NETWORK') await act(async () => app.browser().props.onError());
      else await act(async () => app.browser().props.onMessage(app.message({ type: 'error', code })));
      assert.equal(app.state().timetable?.courses[0].name, '大学物理');
      assert.equal(app.disk.get(TIMETABLE_STORAGE_KEY), original);
    } finally { await app.unmount(); }
    const reopened = await mount(app.disk);
    try { assert.equal(reopened.state().timetable?.courses[0].name, '大学物理'); }
    finally { await reopened.unmount(); }
  });
}

test('cancellation rejects late and previous-nonce messages, then allows a fresh import', async () => {
  const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, JSON.stringify(fixture())]]));
  try {
    await app.open();
    const oldCallback = app.browser().props.onMessage;
    const stale = app.message({ type: 'success', payload: fixture('过期课表') });
    await app.press('取消导入');
    await app.open();
    await act(async () => { oldCallback(stale); app.browser().props.onMessage(stale); });
    assert.equal(app.state().timetable?.courses[0].name, '大学物理');
    await app.success(fixture('新课程'));
    assert.equal(app.state().timetable?.courses[0].name, '新课程');
  } finally { await app.unmount(); }
});

test('new state publishes only after persistence; legal empty imports survive remount', async () => {
  const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, JSON.stringify(fixture())]]));
  let release: () => void = () => {};
  const waiting = new Promise<void>((resolve) => { release = resolve; });
  try {
    app.holdWrite(() => waiting);
    await app.open(); await app.success({ ...fixture(), courses: [] });
    assert.equal(app.state().timetable?.courses.length, 1);
    assert.equal(JSON.parse(app.disk.get(TIMETABLE_STORAGE_KEY)!).courses.length, 1);
    await act(async () => { release(); await waiting; });
    assert.equal(app.state().timetable?.courses.length, 0);
  } finally { release(); await app.unmount(); }
  const reopened = await mount(app.disk);
  try { assert.equal(reopened.state().timetable?.courses.length, 0); }
  finally { await reopened.unmount(); }
});

test('main-frame HTTP authentication errors are distinct from failed subresources', async () => {
  const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, JSON.stringify(fixture())]]));
  try {
    await app.open();
    await act(async () => app.browser().props.onHttpError({ nativeEvent: { url: `${home}/favicon.ico`, statusCode: 404 } }));
    assert.ok(app.browser());
    await act(async () => app.browser().props.onHttpError({ nativeEvent: { url: home, statusCode: 403 } }));
    assert.ok(app.renderedText().includes('学校登录状态已失效，请重新登录'));
    assert.equal(app.state().timetable?.courses[0].name, '大学物理');
  } finally { await app.unmount(); }
});

test('UI shows term without inferring the semester end', async () => {
  const oldTerm = { ...fixture(), termStartDate: '2020-08-31', termCode: '2020-2021-1' };
  const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, JSON.stringify(oldTerm)]]));
  try {
    const text = app.renderedText();
    assert.ok(text.includes('2020-2021-1'));
    assert.ok(text.includes('超出课表浏览范围'));
    assert.equal(text.includes('本学期已结束'), false);
  } finally { await app.unmount(); }
});

test('malformed success payload fails Native schema validation before storage', async () => {
  const original = JSON.stringify(fixture());
  const app = await mount(new Map([[TIMETABLE_STORAGE_KEY, original]]));
  try {
    await app.open();
    await act(async () => app.browser().props.onMessage(app.message({ type: 'success', payload: {
      ...fixture(), courses: [{ ...fixture().courses[0], weekday: 8 }],
    } })));
    assert.equal(app.disk.get(TIMETABLE_STORAGE_KEY), original);
    assert.equal(app.state().timetable?.courses[0].weekday, 3);
    assert.ok(app.renderedText().includes('课表数据格式发生变化'));
  } finally { await app.unmount(); }
});
