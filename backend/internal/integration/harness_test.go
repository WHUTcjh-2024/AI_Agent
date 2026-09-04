package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"asku/backend/internal/agent"
	"asku/backend/internal/api"
	"asku/backend/internal/auth"
	"asku/backend/internal/cache"
	"asku/backend/internal/domain"
	"asku/backend/internal/knowledge"
	"asku/backend/internal/llm"
	"asku/backend/internal/run"
	"asku/backend/internal/school"
	"asku/backend/internal/store"
	"asku/backend/internal/websearch"
	"github.com/jackc/pgx/v5"
)

type fixtureWeb struct {
	calls     atomic.Int32
	failFirst bool
	entered   chan struct{}
	release   chan struct{}
}

func (*fixtureWeb) Name() string { return "eval-web" }
func (p *fixtureWeb) Search(ctx context.Context, _ websearch.ProviderRequest) ([]websearch.SearchResult, error) {
	call := p.calls.Add(1)
	if p.entered != nil {
		select {
		case p.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.release:
		}
	}
	if p.failFirst && call == 1 {
		return nil, context.DeadlineExceeded
	}
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return []websearch.SearchResult{{Title: "工程评测通知（合成样本）", URL: "https://university.example/eval", Publisher: "Fixture", PublishedAt: &date}}, nil
}

type fixtureFetcher struct{}

func (fixtureFetcher) Fetch(ctx context.Context, rawURL string, scope websearch.Scope) (websearch.Page, error) {
	if err := ctx.Err(); err != nil {
		return websearch.Page{}, err
	}
	if !websearch.IsAllowedURL(rawURL, scope.AllowedDomains) {
		return websearch.Page{}, websearch.ErrDisallowedURL
	}
	return websearch.Page{URL: rawURL, ContentType: "text/html", Body: "<html><body><p>今年校历的日期只是工程评测合成内容，不代表任何学校政策。请核验官方通知。</p></body></html>", FetchedAt: time.Now().UTC()}, nil
}

type fixtureKnowledge struct{ calls atomic.Int32 }

func (*fixtureKnowledge) Name() string { return "eval-knowledge" }
func (p *fixtureKnowledge) Search(ctx context.Context, _ knowledge.ProviderRequest) ([]knowledge.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.calls.Add(1)
	return []knowledge.Evidence{{KnowledgeID: "kb-document", ChunkID: "fixture-chunk", Content: "奖学金合成条款，仅供工程回归使用。"}}, nil
}

// Prefix only test cache entries; never flush a Redis database.
type scopedCache struct {
	*cache.Redis
	prefix string
}

func (c scopedCache) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	return c.Redis.GetJSON(ctx, c.prefix+key, target)
}
func (c scopedCache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.Redis.SetJSON(ctx, c.prefix+key, value, ttl)
}

type harness struct {
	t          *testing.T
	db         *store.Postgres
	sql        *pgx.Conn
	cache      scopedCache
	schools    *school.Registry
	web        *fixtureWeb
	knowledge  *fixtureKnowledge
	server     *httptest.Server
	client     *http.Client
	tokens     domain.TokenPair
	adminToken string
}

func newHarness(t *testing.T, web *fixtureWeb, enableKnowledge bool) *harness {
	t.Helper()
	if os.Getenv("ASKU_EVAL_INTEGRATION") != "1" {
		t.Skip("run with the evaluation integration suite")
	}
	dataURL, redisAddr := os.Getenv("ASKU_EVAL_POSTGRES_URL"), os.Getenv("ASKU_EVAL_REDIS_ADDR")
	if dataURL == "" || redisAddr == "" {
		t.Fatal("integration requires ASKU_EVAL_POSTGRES_URL and ASKU_EVAL_REDIS_ADDR (dedicated eval services)")
	}
	cfg, err := pgx.ParseConfig(dataURL)
	if err != nil || cfg.Database != "asku_eval" {
		t.Fatal("integration PostgreSQL bootstrap database must be named asku_eval")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("cannot connect to evaluation PostgreSQL")
	}
	dbName := "asku_eval_" + uniqueID()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		// Only the exact random database created by this test can be dropped.
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop own evaluation database: %v", err)
		}
		_ = admin.Close(cleanup)
	})
	cfg.Database = dbName
	// ConnConfig.ConnString returns the original input even after Database is
	// changed. Build and verify a new URL so both clients use the same own DB.
	isolatedURL, err := url.Parse(dataURL)
	if err != nil || (isolatedURL.Scheme != "postgres" && isolatedURL.Scheme != "postgresql") {
		t.Fatal("evaluation database requires a PostgreSQL URL")
	}
	isolatedURL.Path = "/" + dbName
	isolatedURL.RawPath = ""
	check, err := pgx.ParseConfig(isolatedURL.String())
	if err != nil || check.Database != dbName {
		t.Fatal("isolated database URL was overridden")
	}
	db, err := store.Open(ctx, isolatedURL.String())
	if err != nil {
		t.Fatal("open isolated evaluation database")
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sql, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open isolated SQL verification connection")
	}
	t.Cleanup(func() { _ = sql.Close(context.Background()) })
	redisCache, err := cache.Open(ctx, redisAddr, os.Getenv("ASKU_EVAL_REDIS_PASSWORD"))
	if err != nil {
		t.Fatal("cannot connect to evaluation Redis")
	}
	t.Cleanup(func() { _ = redisCache.Close() })
	root := os.Getenv("ASKU_EVAL_ROOT")
	if root == "" {
		root = filepath.Join("..", "..", "..")
	}
	schools, err := school.Load(filepath.Join(root, "evals", "fixtures", "school.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if web == nil {
		web = &fixtureWeb{}
	}
	if web.release != nil {
		t.Cleanup(func() {
			select {
			case <-web.release:
			default:
				close(web.release)
			}
		})
	}
	h := &harness{t: t, db: db, sql: sql, cache: scopedCache{redisCache, dbName + ":"}, schools: schools, web: web, adminToken: uniqueID()}
	search, err := websearch.NewGateway(web, fixtureFetcher{}, websearch.NewHTMLExtractor(3, 1200), h.cache, schools,
		websearch.CachePolicy{SearchTTL: time.Minute, PageTTL: time.Minute, ExtractTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	var searcher knowledge.Searcher = knowledge.NewDisabledSearcher()
	if enableKnowledge {
		h.seedKnowledge()
		h.knowledge = &fixtureKnowledge{}
		searcher, err = knowledge.NewGateway(h.knowledge, schools, h.cache, knowledge.CachePolicy{QueryTTL: time.Minute}, db)
		if err != nil {
			t.Fatal(err)
		}
	}
	answerCache, err := agent.NewVersionedAnswerCache(h.cache, schools, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := agent.NewOrchestrator(agent.NewPolicyRouter(), agent.Capabilities{
		Generator: llm.NewGateway(llm.NewMockProvider("eval-model"), db, llm.Pricing{InputRMBPerMillionTokens: 1, OutputRMBPerMillionTokens: 2}),
		Knowledge: searcher, WebSearch: search, AnswerCache: answerCache, SearchTopN: 3, KnowledgeTopN: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := run.NewHub()
	service := run.NewService(db, executor, hub, false)
	authService := auth.NewService(db, schools, time.Hour, 24*time.Hour)
	knowledgeName := "disabled"
	if enableKnowledge {
		knowledgeName = "eval-knowledge"
	}
	server := api.New(db, h.cache, authService, service, hub, schools, true, nil,
		api.RuntimeInfo{Version: "eval", AgentMode: "policy", LLMProvider: "mock", WebSearchProvider: "eval-web", KnowledgeProvider: knowledgeName},
		api.RuntimePolicy{QuestionRateLimitPerMinute: 1000}, api.AdminOptions{Reporter: db, Token: h.adminToken, TimeZone: "Asia/Shanghai"})
	h.server = httptest.NewServer(server.Handler())
	h.client = &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(h.server.Close)
	h.tokens = h.login()
	return h
}

func uniqueID() string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}

func (h *harness) request(method, path, token string, body any, headers map[string]string) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	response, err := h.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return response.StatusCode, data, err
}

func (h *harness) json(method, path, token string, body, target any, expected int) {
	h.t.Helper()
	status, data, err := h.request(method, path, token, body, nil)
	if err != nil || status != expected {
		h.t.Fatalf("%s %s: status=%d expected=%d err=%v", method, path, status, expected, err)
	}
	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *harness) login() domain.TokenPair {
	var pair domain.TokenPair
	h.json("POST", "/v1/auth/dev-login", "", map[string]string{"externalId": "eval-" + uniqueID(), "nickname": "Engineering fixture"}, &pair, 200)
	if pair.User.ID == "" || pair.AccessToken == "" {
		h.t.Fatal("login returned no identity/token")
	}
	return pair
}

func (h *harness) session() domain.Session {
	var session domain.Session
	h.json("POST", "/v1/sessions", h.tokens.AccessToken, map[string]string{"title": "Engineering evaluation"}, &session, 201)
	return session
}

func (h *harness) send(sessionID, question string) domain.AgentRun {
	var accepted struct {
		Run domain.AgentRun `json:"run"`
	}
	h.json("POST", "/v1/sessions/"+sessionID+"/messages", h.tokens.AccessToken,
		map[string]string{"question": question, "userMessageId": "eval-" + uniqueID()}, &accepted, 202)
	return accepted.Run
}

func (h *harness) messages(sessionID string) []domain.Message {
	var result struct {
		Messages []domain.Message `json:"messages"`
	}
	h.json("GET", "/v1/sessions/"+sessionID+"/messages", h.tokens.AccessToken, nil, &result, 200)
	return result.Messages
}

func (h *harness) execSQL(sql string, args ...any) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.sql.Exec(ctx, sql, args...); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) count(sql string, args ...any) int {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err := h.sql.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		h.t.Fatal(err)
	}
	return count
}

func (h *harness) seedKnowledge() {
	h.execSQL(`INSERT INTO knowledge.sources(id,school_id,source_name,official_url,active) VALUES('source','eval','Fixture','https://university.example/rules',true)`)
	h.execSQL(`INSERT INTO knowledge.documents(id,school_id,source_id,title,publish_date,rag_eligible,pii_detected,review_status,local_file_path)
		VALUES('document','eval','source','工程合成资料','2026-09-01',true,false,'ACCEPTED','/private/never-publish.pdf')`)
	h.execSQL(`UPDATE knowledge.documents SET parse_status='PARSED',pii_scan_status='CLEAR',
		content_hash='fixture-hash',pii_content_hash='fixture-hash',content_chars=200,
		secondary_topic='scholarship',admission_status='READY',admission_version='admission-v1',
		source_url='https://university.example/rules',canonical_url='https://university.example/rules'
		WHERE id='document'`)
	h.execSQL(`INSERT INTO knowledge.weknora_mappings(school_id,weknora_knowledge_id,asku_document_id,import_status) VALUES('eval','kb-document','document','IMPORTED')`)
}

func waitEntered(t *testing.T, web *fixtureWeb) {
	t.Helper()
	select {
	case <-web.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("retrieval did not start")
	}
}

func (h *harness) status(runID, expected string) {
	h.t.Helper()
	_, _, actual, err := h.db.RunOwner(context.Background(), runID)
	if err != nil || actual != expected {
		h.t.Fatal(fmt.Sprintf("run status=%s expected=%s err=%v", actual, expected, err))
	}
}
