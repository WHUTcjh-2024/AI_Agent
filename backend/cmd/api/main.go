package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"asku/backend/internal/agent"
	"asku/backend/internal/api"
	"asku/backend/internal/auth"
	"asku/backend/internal/cache"
	"asku/backend/internal/config"
	"asku/backend/internal/knowledge"
	"asku/backend/internal/llm"
	"asku/backend/internal/run"
	"asku/backend/internal/school"
	"asku/backend/internal/store"
	"asku/backend/internal/websearch"
)

const version = "0.6.0"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	schools, err := school.Load(cfg.SchoolConfig)
	if err != nil {
		logger.Error("load school context", "error", err, "path", cfg.SchoolConfig)
		os.Exit(1)
	}
	api.LogConfiguration(schools.Current())

	startupContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := store.Open(startupContext, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Migrate(startupContext); err != nil {
		logger.Error("migrate postgres", "error", err)
		os.Exit(1)
	}
	interruptedRuns, err := database.RecoverInterruptedRuns(startupContext)
	if err != nil {
		logger.Error("recover interrupted runs", "error", err)
		os.Exit(1)
	}
	if interruptedRuns > 0 {
		logger.Warn("recovered interrupted runs", "count", interruptedRuns)
	}
	redisCache, err := cache.Open(startupContext, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		logger.Error("connect redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisCache.Close() }()

	hub := run.NewHub()
	authService := auth.NewService(database, schools, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	var llmProvider llm.Provider
	switch cfg.LLMProvider {
	case "mock":
		llmProvider = llm.NewMockProvider(cfg.LLMModel)
	case "openai-compatible":
		llmProvider, err = llm.NewOpenAICompatibleProvider(
			cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel,
			&http.Client{Timeout: cfg.LLMTimeout},
		)
		if err != nil {
			logger.Error("configure llm provider", "provider", cfg.LLMProvider, "error", err)
			os.Exit(1)
		}
	}
	llmGateway := llm.NewGateway(llmProvider, database, llm.Pricing{
		InputRMBPerMillionTokens: cfg.LLMInputPrice, OutputRMBPerMillionTokens: cfg.LLMOutputPrice,
	})
	var searchProvider websearch.Provider
	var pageFetcher websearch.Fetcher
	switch cfg.WebSearchProvider {
	case "mock":
		searchProvider = websearch.NewMockProvider()
		pageFetcher = websearch.NewMockFetcher()
	case "searxng":
		searchClient := &http.Client{Timeout: cfg.WebSearchTimeout}
		searchProvider, err = websearch.NewSearXNGProvider(cfg.WebSearchBaseURL, cfg.WebSearchAPIKey, searchClient)
		if err != nil {
			logger.Error("configure web search provider", "provider", cfg.WebSearchProvider, "error", err)
			os.Exit(1)
		}
		pageFetcher = websearch.NewHTTPFetcher(&http.Client{Timeout: cfg.WebSearchTimeout}, 0)
	}
	searchGateway, err := websearch.NewGateway(
		searchProvider, pageFetcher, websearch.NewHTMLExtractor(3, 1200), redisCache, schools,
		websearch.CachePolicy{SearchTTL: cfg.WebSearchSearchTTL, PageTTL: cfg.WebSearchPageTTL, ExtractTTL: cfg.WebSearchExtractTTL},
	)
	if err != nil {
		logger.Error("configure web search gateway", "error", err)
		os.Exit(1)
	}
	var knowledgeSearcher knowledge.Searcher
	switch cfg.KnowledgeProvider {
	case "disabled":
		knowledgeSearcher = knowledge.NewDisabledSearcher()
	case "weknora":
		if schools.Current().OfficialKnowledgeBaseID == "" {
			logger.Error("configure knowledge provider", "provider", cfg.KnowledgeProvider, "error", "current school has no official_knowledge_base_id")
			os.Exit(1)
		}
		provider, providerErr := knowledge.NewWeKnoraProvider(
			cfg.WeKnoraBaseURL, cfg.WeKnoraAPIKey, &http.Client{Timeout: cfg.WeKnoraTimeout},
		)
		if providerErr != nil {
			logger.Error("configure knowledge provider", "provider", cfg.KnowledgeProvider, "error", providerErr)
			os.Exit(1)
		}
		knowledgeSearcher, err = knowledge.NewGateway(provider, schools)
		if err != nil {
			logger.Error("configure knowledge gateway", "error", err)
			os.Exit(1)
		}
	}
	var agentRouter agent.Router
	switch cfg.AgentMode {
	case "mock":
		agentRouter = agent.NewMockRouter()
	case "policy":
		agentRouter = agent.NewPolicyRouter()
	}
	agentExecutor, err := agent.NewOrchestrator(agentRouter, agent.Capabilities{
		Generator: llmGateway, Knowledge: knowledgeSearcher, WebSearch: searchGateway,
		SearchTopN: cfg.WebSearchTopN, KnowledgeTopN: cfg.KnowledgeTopN,
	})
	if err != nil {
		logger.Error("configure agent orchestrator", "error", err)
		os.Exit(1)
	}
	runService := run.NewService(database, agentExecutor, hub, true)
	apiServer := api.New(database, redisCache, authService, runService, hub, schools, cfg.DevAuthEnabled, cfg.AllowedOrigins, api.RuntimeInfo{
		Version: version, AgentMode: cfg.AgentMode, LLMProvider: cfg.LLMProvider,
		WebSearchProvider: cfg.WebSearchProvider, KnowledgeProvider: cfg.KnowledgeProvider,
	})
	httpServer := &http.Server{
		Addr: cfg.HTTPAddr, Handler: apiServer.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second,
	}

	go func() {
		logger.Info("AskU API started", "addr", cfg.HTTPAddr, "agent_mode", cfg.AgentMode, "llm_provider", cfg.LLMProvider, "llm_model", cfg.LLMModel, "web_search_provider", cfg.WebSearchProvider, "knowledge_provider", cfg.KnowledgeProvider)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	logger.Info("AskU API stopped")
}
