package console

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

const testPassword = "console-test-password"
const testAPIToken = "server-only-api-token"

func testHandler(t *testing.T, upstream http.HandlerFunc) http.Handler {
	t.Helper()
	backend := httptest.NewServer(upstream)
	t.Cleanup(backend.Close)
	handler, err := New(Config{Addr: "127.0.0.1:18090", APIBaseURL: backend.URL, APIToken: testAPIToken, Password: testPassword},
		fstest.MapFS{"index.html": {Data: []byte("<html>console</html>")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func call(handler http.Handler, method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://console.local"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12000"
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("Authorization", "Bearer user-supplied-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func login(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	w := call(h, "POST", "/api/session", `{"password":"`+testPassword+`"}`, nil, "http://console.local")
	if w.Code != 200 {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("missing protected session cookie")
	}
	if strings.Contains(w.Body.String(), testPassword) || strings.Contains(w.Body.String(), testAPIToken) {
		t.Fatal("secret in login response")
	}
	return cookies[0]
}

func TestSessionProxyAndLogout(t *testing.T) {
	var calls atomic.Int32
	h := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/v1/admin/overview" || r.URL.Query().Get("from") != "2026-09-01" {
			t.Error("incorrect proxy scope")
		}
		if r.Header.Get("Authorization") != "Bearer "+testAPIToken || r.Header.Get("Cookie") != "" {
			t.Error("incorrect credential boundary")
		}
		_, _ = io.WriteString(w, `{"generatedAt":"2026-09-03T00:00:00Z"}`)
	})
	if w := call(h, "GET", "/api/overview", "", nil, ""); w.Code != 401 {
		t.Fatal("anonymous proxy access accepted")
	}
	if w := call(h, "POST", "/api/session", `{"password":"wrong"}`, nil, "http://console.local"); w.Code != 401 {
		t.Fatal("wrong password accepted")
	}
	if w := call(h, "POST", "/api/session", `{"password":"`+testPassword+`"}`, nil, "http://other.local"); w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	cookie := login(t, h)
	if w := call(h, "GET", "/api/session", "", cookie, ""); w.Code != 200 {
		t.Fatal("session not restored")
	}
	w := call(h, "GET", "/api/overview?from=2026-09-01&to=2026-09-03", "", cookie, "")
	if w.Code != 200 || !json.Valid(w.Body.Bytes()) || calls.Load() != 1 {
		t.Fatalf("proxy failed: %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing response protections")
	}
	for _, query := range []string{"schoolId=other", "from=1&from=2", "url=http://elsewhere"} {
		if w := call(h, "GET", "/api/overview?"+query, "", cookie, ""); w.Code != 400 {
			t.Fatal("invalid proxy query accepted")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("invalid request reached upstream")
	}
	if w := call(h, "DELETE", "/api/session", "", cookie, "http://other.local"); w.Code != 403 {
		t.Fatal("cross-origin logout accepted")
	}
	if w := call(h, "DELETE", "/api/session", "", cookie, "http://console.local"); w.Code != 204 {
		t.Fatal("logout failed")
	}
	if w := call(h, "GET", "/api/overview", "", cookie, ""); w.Code != 401 {
		t.Fatal("logged-out session still usable")
	}
}

func TestProxyRedactsFailuresAndRejectsRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		expected int
	}{
		{"unauthorized", 401, testAPIToken, 503}, {"disabled", 404, testAPIToken, 503}, {"bad-range", 400, "sensitive error", 400},
		{"server-error", 500, testAPIToken, 502}, {"malformed", 200, "not-json", 502},
		{"oversize", 200, `"` + strings.Repeat("x", maxOverviewBytes) + `"`, 502}, {"redirect", 307, "", 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			w := call(h, "GET", "/api/overview", "", login(t, h), "")
			if w.Code != tc.expected || strings.Contains(w.Body.String(), testAPIToken) {
				t.Fatalf("unsafe/error response: %d %s", w.Code, w.Body.String())
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("proxy followed redirect with server credential")
	}
}

func TestLoginLimitAndStaticAllowlist(t *testing.T) {
	h := testHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream call") })
	for range 5 {
		if w := call(h, "POST", "/api/session", `{"password":"wrong"}`, nil, "http://console.local"); w.Code != 401 {
			t.Fatal("unexpected login result")
		}
	}
	if w := call(h, "POST", "/api/session", `{"password":"wrong"}`, nil, "http://console.local"); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("login attempts not bounded")
	}
	for _, path := range []string{"/.env", "/go.mod", "/web/", "/internal/console/config.go"} {
		if w := call(h, "GET", path, "", nil, ""); w.Code != 404 {
			t.Fatalf("served non-public file %s", path)
		}
	}
}

func TestSessionExpiryAndRotation(t *testing.T) {
	s := newSessions()
	now := time.Now()
	s.now = func() time.Time { return now }
	token, expires, err := s.create()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.get(token); !ok {
		t.Fatal("fresh session rejected")
	}
	now = expires
	if _, ok := s.get(token); ok {
		t.Fatal("expired session accepted")
	}
	h := testHandler(t, func(http.ResponseWriter, *http.Request) {})
	old := login(t, h)
	w := call(h, "POST", "/api/session", `{"password":"`+testPassword+`"}`, old, "http://console.local")
	if w.Code != 200 || w.Result().Cookies()[0].Value == old.Value {
		t.Fatal("session was not rotated")
	}
	if w := call(h, "GET", "/api/session", "", old, ""); w.Code != 401 {
		t.Fatal("rotated session still accepted")
	}
}

func TestConfigRejectsUnsafeInputs(t *testing.T) {
	base := Config{Addr: "127.0.0.1:18090", APIBaseURL: "http://localhost:18080", APIToken: testAPIToken, Password: testPassword}
	for _, change := range []func(*Config){
		func(c *Config) { c.Addr = ":18090" }, func(c *Config) { c.APIBaseURL = "http://user:pass@example.com" },
		func(c *Config) { c.APIBaseURL = "http://localhost/path" }, func(c *Config) { c.APIToken = "" }, func(c *Config) { c.Password = "short" },
	} {
		c := base
		change(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	base.SecureCookie = true
	base.Addr = ":18090"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
}
