package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"asku/backend/internal/auth"
	"asku/backend/internal/domain"
	"asku/backend/internal/httpx"
	"asku/backend/internal/id"
	"asku/backend/internal/school"
)

type Server struct {
	store             Repository
	cache             Cache
	auth              Authenticator
	runs              RunController
	hub               EventHub
	schools           SchoolRegistry
	devAuthEnabled    bool
	allowedOrigins    []string
	runtime           RuntimeInfo
	policy            RuntimePolicy
	admin             AdminReporter
	adminToken        string
	reportingTimeZone string
}

type RuntimeInfo struct {
	Version           string
	AgentMode         string
	LLMProvider       string
	WebSearchProvider string
	KnowledgeProvider string
}

type RuntimePolicy struct {
	QuestionRateLimitPerMinute int64
}

func New(database Repository, redisCache Cache, authService Authenticator, runService RunController, hub EventHub, schools SchoolRegistry, devAuthEnabled bool, allowedOrigins []string, runtime RuntimeInfo, policy RuntimePolicy, admin AdminOptions) *Server {
	return &Server{store: database, cache: redisCache, auth: authService, runs: runService, hub: hub, schools: schools, devAuthEnabled: devAuthEnabled, allowedOrigins: allowedOrigins, runtime: runtime, policy: policy, admin: admin.Reporter, adminToken: strings.TrimSpace(admin.Token), reportingTimeZone: admin.TimeZone}
}

func (s *Server) Handler() http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /{$}", s.handleRoot)
	public.HandleFunc("GET /healthz", s.handleHealth)
	public.HandleFunc("POST /v1/auth/dev-login", s.handleDevLogin)
	public.HandleFunc("POST /v1/auth/refresh", s.handleRefresh)
	public.HandleFunc("POST /v1/auth/wechat", s.handleWechatUnavailable)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /v1/me", s.handleMe)
	protected.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	protected.HandleFunc("GET /v1/sessions", s.handleListSessions)
	protected.HandleFunc("DELETE /v1/sessions", s.handleClearSessions)
	protected.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	protected.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	protected.HandleFunc("GET /v1/sessions/{id}/messages", s.handleGetMessages)
	protected.HandleFunc("POST /v1/sessions/{id}/messages", s.handleSendMessage)
	protected.HandleFunc("GET /v1/runs/{id}/events", s.handleRunEvents)
	protected.HandleFunc("POST /v1/runs/{id}/cancel", s.handleCancelRun)
	protected.HandleFunc("GET /v1/sources/{id}", s.handleGetSource)
	protected.HandleFunc("POST /v1/feedback", s.handleFeedback)
	protected.HandleFunc("POST /v1/dev/seed", s.handleDevSeed)

	root := http.NewServeMux()
	root.Handle("/v1/admin/", s.adminHandler())
	root.Handle("/v1/", route(public, s.auth.Middleware(protected)))
	root.Handle("/healthz", public)
	root.Handle("/", public)
	handler := httpx.CORS(s.allowedOrigins, httpx.RequestMiddleware(root))
	return handler
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"service": "AskU API",
		"version": s.runtime.Version,
		"status":  "running",
		"health":  "/healthz",
		"api":     "/v1",
	})
}

func route(public, protected http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/auth/") {
			public.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	postgresStatus, redisStatus := "ok", "ok"
	status := http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		postgresStatus, status = "error", http.StatusServiceUnavailable
	}
	if err := s.cache.Ping(ctx); err != nil {
		redisStatus, status = "error", http.StatusServiceUnavailable
	}
	httpx.JSON(w, status, map[string]any{
		"status":  map[bool]string{true: "ok", false: "degraded"}[status == http.StatusOK],
		"service": "asku-api", "version": s.runtime.Version, "postgres": postgresStatus, "redis": redisStatus,
		"school": s.schools.Current(), "agentMode": s.runtime.AgentMode,
		"providers": map[string]string{
			"llm": s.runtime.LLMProvider, "webSearch": s.runtime.WebSearchProvider, "knowledge": s.runtime.KnowledgeProvider,
		},
	})
}

func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.devAuthEnabled {
		httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusNotFound, Code: "not_found", Message: "开发登录未启用。"})
		return
	}
	var request struct {
		ExternalID string `json:"externalId"`
		Nickname   string `json:"nickname"`
	}
	if err := httpx.Decode(r, &request); err != nil {
		httpx.Error(w, r, badRequest("invalid_json", "请求格式不正确。"))
		return
	}
	request.ExternalID = strings.TrimSpace(request.ExternalID)
	request.Nickname = strings.TrimSpace(request.Nickname)
	if request.ExternalID == "" {
		request.ExternalID = "local-tester"
	}
	if request.Nickname == "" {
		request.Nickname = "AskU 测试同学"
	}
	pair, err := s.auth.DevLogin(r.Context(), request.ExternalID, request.Nickname)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pair)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := httpx.Decode(r, &request); err != nil || strings.TrimSpace(request.RefreshToken) == "" {
		httpx.Error(w, r, badRequest("invalid_refresh_token", "Refresh Token 不能为空。"))
		return
	}
	pair, err := s.auth.Refresh(r.Context(), request.RefreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusUnauthorized, Code: "invalid_refresh_token", Message: "登录状态已失效，请重新登录。"})
			return
		}
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pair)
}

func (s *Server) handleWechatUnavailable(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusNotImplemented, Code: "wechat_not_configured", Message: "尚未配置微信开放平台 AppID 与签名，当前请使用开发测试登录。"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	httpx.JSON(w, http.StatusOK, user)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	var request struct {
		Title string `json:"title"`
	}
	if err := httpx.Decode(r, &request); err != nil {
		httpx.Error(w, r, badRequest("invalid_json", "请求格式不正确。"))
		return
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = "新对话"
	}
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, user.SchoolID, title)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, session)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	sessions, err := s.store.ListSessions(r.Context(), user.ID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	session, err := s.store.GetSession(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		s.writeStoreError(w, r, err, "对话不存在。")
		return
	}
	httpx.JSON(w, http.StatusOK, session)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	if err := s.store.DeleteSession(r.Context(), user.ID, r.PathValue("id")); err != nil {
		s.writeStoreError(w, r, err, "对话不存在。")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearSessions(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	if err := s.store.ClearSessions(r.Context(), user.ID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	messages, err := s.store.ListMessages(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	var request struct {
		Question      string `json:"question"`
		UserMessageID string `json:"userMessageId"`
	}
	if err := httpx.Decode(r, &request); err != nil {
		httpx.Error(w, r, badRequest("invalid_json", "请求格式不正确。"))
		return
	}
	request.Question = strings.TrimSpace(request.Question)
	if request.Question == "" || len([]rune(request.Question)) > 2000 {
		httpx.Error(w, r, badRequest("invalid_question", "问题不能为空，且不能超过 2000 个字符。"))
		return
	}
	allowed, err := s.cache.AllowQuestion(r.Context(), user.ID, s.policy.QuestionRateLimitPerMinute)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if !allowed {
		httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "提问过于频繁，请稍后再试。"})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	reserved, err := s.cache.ReserveIdempotency(r.Context(), user.ID, idempotencyKey)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if !reserved {
		httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusConflict, Code: "duplicate_request", Message: "相同请求正在处理中。"})
		return
	}
	if strings.TrimSpace(request.UserMessageID) == "" {
		request.UserMessageID = id.New("msg")
	}
	runRecord, message, err := s.runs.Start(r.Context(), user.ID, user.SchoolID, r.PathValue("id"), request.Question, request.UserMessageID)
	if err != nil {
		releaseContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if releaseErr := s.cache.ReleaseIdempotency(releaseContext, user.ID, idempotencyKey); releaseErr != nil {
			slog.Warn("release failed run idempotency reservation", "user_id", user.ID, "error", releaseErr)
		}
		s.writeStoreError(w, r, err, "对话不存在。")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"run": runRecord, "userMessage": message})
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	runID := r.PathValue("id")
	ownerID, _, status, err := s.store.RunOwner(r.Context(), runID)
	if err != nil || ownerID != user.ID {
		s.writeStoreError(w, r, domain.ErrNotFound, "运行记录不存在。")
		return
	}
	after := parseAfter(r)
	channel, unsubscribe := s.hub.Subscribe(runID)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, r, errors.New("streaming unsupported"))
		return
	}
	events, err := s.store.ListRunEvents(r.Context(), runID, after)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	terminalEventSeen := false
	for _, event := range events {
		if event.Sequence <= after {
			continue
		}
		writeEvent(w, event)
		after = event.Sequence
		terminalEventSeen = terminalEventSeen || event.Type == "run.completed" || event.Type == "run.failed"
	}
	flusher.Flush()
	if terminalEventSeen || isTerminal(status) {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case event, open := <-channel:
			if !open {
				return
			}
			if event.Sequence <= after {
				continue
			}
			writeEvent(w, event)
			after = event.Sequence
			flusher.Flush()
			if event.Type == "run.completed" || event.Type == "run.failed" {
				return
			}
		}
	}
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	if err := s.runs.Cancel(r.Context(), user.ID, r.PathValue("id")); err != nil {
		s.writeStoreError(w, r, err, "运行记录不存在。")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	source, err := s.store.GetSourceForUser(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		s.writeStoreError(w, r, err, "来源不存在。")
		return
	}
	httpx.JSON(w, http.StatusOK, source)
}

func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	var request struct {
		MessageID string `json:"messageId"`
		Value     string `json:"value"`
	}
	if err := httpx.Decode(r, &request); err != nil || (request.Value != "helpful" && request.Value != "unhelpful") {
		httpx.Error(w, r, badRequest("invalid_feedback", "反馈参数不正确。"))
		return
	}
	feedback, err := s.store.CreateFeedback(r.Context(), user.ID, request.MessageID, request.Value)
	if err != nil {
		s.writeStoreError(w, r, err, "消息不存在。")
		return
	}
	httpx.JSON(w, http.StatusCreated, feedback)
}

func (s *Server) handleDevSeed(w http.ResponseWriter, r *http.Request) {
	if !s.devAuthEnabled {
		httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusNotFound, Code: "not_found", Message: "开发能力未启用。"})
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	seedConversations := []struct{ question, answer string }{
		{"今年转专业什么时候开始？", "校园政策知识尚未接入，请以学校正式通知为准。"},
		{"四六级什么时候报名？", "这是开发环境历史记录示例，不代表学校正式安排。"},
	}
	for _, seed := range seedConversations {
		question := seed.question
		session, err := s.store.CreateSession(r.Context(), user.ID, user.SchoolID, question)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		_, _ = s.store.CreateMessage(r.Context(), user.ID, domain.Message{SessionID: session.ID, Role: "user", Content: question, Status: "completed"})
		_, err = s.store.CreateMessage(r.Context(), user.ID, domain.Message{SessionID: session.ID, Role: "assistant", Content: seed.answer, Status: "completed"})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"seeded": len(seedConversations)})
}

func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error, message string) {
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusNotFound, Code: "not_found", Message: message})
		return
	}
	httpx.Error(w, r, err)
}

func badRequest(code, message string) *httpx.HandlerError {
	return &httpx.HandlerError{Status: http.StatusBadRequest, Code: code, Message: message}
}

func parseAfter(r *http.Request) int64 {
	value := strings.TrimSpace(r.URL.Query().Get("after"))
	if value == "" {
		value = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	after, _ := strconv.ParseInt(value, 10, 64)
	return after
}

func writeEvent(w http.ResponseWriter, event domain.RunEvent) {
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, event.Payload)
}

func isTerminal(status string) bool {
	return status == "COMPLETED" || status == "FAILED" || status == "CANCELLED"
}

func LogConfiguration(schoolContext school.Context) {
	encoded, _ := json.Marshal(schoolContext)
	slog.Info("school context loaded", "school", string(encoded))
}
