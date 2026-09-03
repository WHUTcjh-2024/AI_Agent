package console

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cookieName = "asku_admin_session"
const maxOverviewBytes = 2 << 20

type Server struct {
	config       Config
	client       *http.Client
	sessions     *sessionStore
	passwordHash [32]byte
}

func New(cfg Config, files fs.FS, transport http.RoundTripper) (http.Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	s := &Server{config: cfg, passwordHash: sha256.Sum256([]byte(cfg.Password)), sessions: newSessions(),
		client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	s.config.Password = ""
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/session", s.login)
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("DELETE /api/session", s.logout)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "not_found", "接口不存在。") })
	static := http.FileServerFS(files)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Serve a fixed public asset list, never directory listings or source files.
		switch r.URL.Path {
		case "/", "/index.html", "/styles.css", "/app.js", "/api.js", "/metrics.js", "/render.js":
			static.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Method == "POST" || r.Method == "DELETE" {
			scheme := "https"
			if !cfg.SecureCookie {
				scheme = "http"
			}
			if r.Header.Get("Origin") != scheme+"://"+r.Host {
				problem(w, 403, "origin_denied", "请从控制台页面发起操作。")
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.sessions.allowLogin(ip) {
		w.Header().Set("Retry-After", "60")
		problem(w, 429, "login_rate_limited", "尝试次数较多，请一分钟后再试。")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		problem(w, 415, "invalid_content_type", "请使用 JSON 请求。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		Password string `json:"password"`
	}
	if err := decoder.Decode(&input); err != nil {
		problem(w, 400, "invalid_request", "登录请求格式不正确。")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		problem(w, 400, "invalid_request", "登录请求格式不正确。")
		return
	}
	provided := sha256.Sum256([]byte(input.Password))
	if subtle.ConstantTimeCompare(s.passwordHash[:], provided[:]) != 1 {
		problem(w, 401, "invalid_password", "控制台口令不正确。")
		return
	}
	if old, err := r.Cookie(cookieName); err == nil {
		s.sessions.remove(old.Value)
	}
	token, expires, err := s.sessions.create()
	if err != nil {
		problem(w, 503, "session_unavailable", "暂时无法创建会话，请稍后再试。")
		return
	}
	s.cookie(w, token, int(sessionTTL.Seconds()))
	jsonResponse(w, 200, map[string]any{"authenticated": true, "expiresAt": expires})
}

func (s *Server) cookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.config.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err == nil {
		if _, ok := s.sessions.get(cookie.Value); ok {
			return true
		}
	}
	problem(w, 401, "auth_required", "登录已过期，请重新登录。")
	return false
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if s.authenticate(w, r) {
		jsonResponse(w, 200, map[string]bool{"authenticated": true})
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		s.sessions.remove(cookie.Value)
	}
	s.cookie(w, "", -1)
	w.WriteHeader(204)
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(w, r) {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		problem(w, 400, "invalid_filter", "日期参数无效。")
		return
	}
	for key, values := range query {
		if (key != "from" && key != "to") || len(values) != 1 || len(values[0]) > 40 {
			problem(w, 400, "invalid_filter", "仅支持起止日期筛选。")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.config.APIBaseURL, "/")+"/v1/admin/overview?"+query.Encode(), nil)
	if err != nil {
		problem(w, 502, "backend_unavailable", "暂时无法连接统计服务。")
		return
	}
	request.Header.Set("Authorization", "Bearer "+s.config.APIToken)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		problem(w, 502, "backend_unavailable", "统计服务暂时不可用，请稍后重试。")
		return
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
		data, err := io.ReadAll(io.LimitReader(response.Body, maxOverviewBytes+1))
		if err != nil || len(data) > maxOverviewBytes || !json.Valid(data) {
			problem(w, 502, "invalid_backend_response", "统计服务返回了无效数据。")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write(data)
	case 400:
		problem(w, 400, "invalid_time_window", "请选择有效日期，起止范围不能超过 90 天。")
	case 401, 403, 404:
		problem(w, 503, "admin_not_configured", "统计接口尚未启用或服务端凭证无效，请联系维护人员。")
	default:
		problem(w, 502, "backend_unavailable", "统计服务暂时不可用，请稍后重试。")
	}
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func problem(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
