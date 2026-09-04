import { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Modal, Platform, Pressable, StyleSheet, Text, View } from 'react-native';
import { WebView } from 'react-native-webview';

import { Screen } from '../../../components/common/Screen';
import { colors } from '../../../theme';
import type { CourseImportResult } from '../domain/timetable';
import { CourseImportError, type BrowserImportRequest, type CourseBrowser, type ImportErrorCode } from '../providers/course-provider';

type Pending = {
  request: BrowserImportRequest;
  resolve(value: CourseImportResult): void;
  reject(error: CourseImportError): void;
  removeAbort(): void;
};

export function useCourseBrowser(): { browser: CourseBrowser; browserNode: React.ReactNode } {
  const [request, setRequest] = useState<BrowserImportRequest | null>(null);
  const pending = useRef<Pending | null>(null);
  const finish = useCallback((result: CourseImportResult | CourseImportError) => {
    const active = pending.current;
    if (!active) return;
    pending.current = null;
    active.removeAbort();
    setRequest(null);
    if (result instanceof CourseImportError) active.reject(result);
    else active.resolve(result);
  }, []);
  const open = useCallback<CourseBrowser['open']>((next, signal) => {
    if (signal?.aborted || pending.current) return Promise.reject(new CourseImportError('CANCELLED'));
    return new Promise((resolve, reject) => {
      const abort = () => finish(new CourseImportError('CANCELLED'));
      pending.current = { request: next, resolve, reject, removeAbort: () => signal?.removeEventListener('abort', abort) };
      signal?.addEventListener('abort', abort, { once: true });
      setRequest(next);
    });
  }, [finish]);
  useEffect(() => () => {
    const active = pending.current;
    pending.current = null;
    active?.removeAbort();
    active?.reject(new CourseImportError('CANCELLED'));
  }, []);
  return { browser: { open }, browserNode: request ? <CourseImportBrowser key={request.requestId} request={request} onFinish={finish} /> : null };
}

function CourseImportBrowser({ request, onFinish }: { request: BrowserImportRequest; onFinish(value: CourseImportResult | CourseImportError): void }) {
  const webview = useRef<WebView>(null);
  const currentUrl = useRef(request.loginUrl);
  const started = useRef(false);
  const finished = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const injectionTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [fetching, setFetching] = useState(false);
  const [loading, setLoading] = useState(true);
  const [blocked, setBlocked] = useState(false);
  const clearTimer = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    if (injectionTimer.current) clearTimeout(injectionTimer.current);
    timer.current = null;
    injectionTimer.current = null;
  }, []);
  const finish = useCallback((value: CourseImportResult | CourseImportError) => {
    if (finished.current) return;
    finished.current = true;
    clearTimer();
    webview.current?.stopLoading();
    onFinish(value);
  }, [clearTimer, onFinish]);
  const fail = useCallback((code: ImportErrorCode) => finish(new CourseImportError(code)), [finish]);
  useEffect(() => {
    finished.current = false;
    return () => { finished.current = true; clearTimer(); };
  }, [clearTimer]);

  return (
    <Modal visible animationType="slide" onRequestClose={() => fail('CANCELLED')} presentationStyle="fullScreen">
      <Screen includeBottomInset>
        <View style={styles.header}>
          <Pressable accessibilityRole="button" accessibilityLabel="取消导入" onPress={() => fail('CANCELLED')} style={styles.cancel}>
            <Text style={styles.link}>取消</Text>
          </Pressable>
          <Text accessibilityRole="header" style={styles.title}>导入课表</Text>
          <View style={styles.cancel} />
        </View>
        <Text style={styles.notice}>请使用学校统一身份认证登录。{ '\n' }登录信息由学校页面处理，AskU 不保存密码。</Text>
        {blocked && <Text accessibilityRole="alert" style={styles.warning}>已阻止外部跳转，请在当前学校页面完成登录。</Text>}
        {Platform.OS === 'web' ? (
          <View style={styles.center}><Text style={styles.notice}>学校登录仅支持 Android / iOS。请使用真机打开 AskU；网页可体验演示课表。</Text></View>
        ) : (
          <View style={styles.flex}>
            <WebView
              ref={webview}
              source={{ uri: request.loginUrl }}
              style={styles.flex}
              incognito
              cacheEnabled={false}
              saveFormDataDisabled
              sharedCookiesEnabled={false}
              thirdPartyCookiesEnabled
              javaScriptEnabled
              domStorageEnabled
              webviewDebuggingEnabled={false}
              mixedContentMode="never"
              allowFileAccess={false}
              allowFileAccessFromFileURLs={false}
              allowUniversalAccessFromFileURLs={false}
              javaScriptCanOpenWindowsAutomatically={false}
              setSupportMultipleWindows
              onOpenWindow={() => setBlocked(true)}
              // Catch every URL here; a narrow originWhitelist can hand rejected URLs to OS Linking.
              originWhitelist={['*']}
              onShouldStartLoadWithRequest={(navigation) => {
                const allowed = request.allowsNavigation(navigation.url);
                if (!allowed) setBlocked(true);
                return allowed;
              }}
              onLoadStart={({ nativeEvent }) => {
                currentUrl.current = nativeEvent.url;
                if (started.current && !request.isImportPage(nativeEvent.url)) { fail('AUTH'); return; }
                if (!started.current) {
                  setLoading(true);
                  clearTimer();
                  timer.current = setTimeout(() => fail('TIMEOUT'), 45000);
                }
              }}
              onLoadEnd={({ nativeEvent }) => {
                if (finished.current) return;
                currentUrl.current = nativeEvent.url;
                setLoading(false);
                if (started.current) return;
                clearTimer();
                if (!request.isImportPage(nativeEvent.url)) return;
                started.current = true;
                setFetching(true);
                timer.current = setTimeout(() => fail('TIMEOUT'), 30000);
                // Give the school SPA its initialization interval (also observed in iwut).
                injectionTimer.current = setTimeout(() => {
                  if (!finished.current && request.isImportPage(currentUrl.current)) webview.current?.injectJavaScript(request.script);
                }, 1500);
              }}
              onMessage={({ nativeEvent }) => {
                if (!started.current || finished.current || !request.isImportPage(currentUrl.current)) return;
                try {
                  const value = request.validateMessage(nativeEvent.data, nativeEvent.url);
                  if (value) finish(value);
                } catch (error) { fail(error instanceof CourseImportError ? error.code : 'FORMAT'); }
              }}
              onError={() => fail('NETWORK')}
              onHttpError={({ nativeEvent }) => {
                // Ignore a failed subresource; don't turn a missing favicon into a failed login.
                if (nativeEvent.url === currentUrl.current) fail('SYSTEM');
              }}
              onContentProcessDidTerminate={() => fail('SYSTEM')}
              onRenderProcessGone={() => fail('SYSTEM')}
            />
            {(loading || fetching) && (
              <View pointerEvents={fetching ? 'auto' : 'none'} style={fetching ? styles.overlay : styles.loading}>
                <ActivityIndicator color={colors.accent} />
                <Text style={styles.notice}>{fetching ? '正在读取本学期课表…' : '正在连接学校…'}</Text>
              </View>
            )}
          </View>
        )}
      </Screen>
    </Modal>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  header: { height: 56, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  cancel: { minWidth: 72, minHeight: 48, alignItems: 'center', justifyContent: 'center' },
  title: { fontSize: 17, fontWeight: '600', color: colors.textPrimary },
  link: { color: colors.accent, fontSize: 15 },
  notice: { fontSize: 13, lineHeight: 21, color: colors.textSecondary, padding: 16 },
  warning: { fontSize: 13, color: colors.warning, paddingHorizontal: 16, paddingBottom: 12 },
  center: { flex: 1, justifyContent: 'center', alignItems: 'center' },
  overlay: { ...StyleSheet.absoluteFill, backgroundColor: 'rgba(255,255,255,0.96)', justifyContent: 'center', alignItems: 'center' },
  loading: { position: 'absolute', top: 0, right: 0, left: 0, backgroundColor: colors.surface, flexDirection: 'row', justifyContent: 'center', alignItems: 'center' },
});
