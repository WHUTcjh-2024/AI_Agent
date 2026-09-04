package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr               string
	DatabaseURL            string
	RedisAddr              string
	RedisPassword          string
	SchoolConfig           string
	DevAuthEnabled         bool
	AgentMode              string
	LLMProvider            string
	LLMBaseURL             string
	LLMAPIKey              string
	LLMModel               string
	LLMTimeout             time.Duration
	LLMInputPrice          float64
	LLMOutputPrice         float64
	WebSearchProvider      string
	WebSearchBaseURL       string
	WebSearchAPIKey        string
	WebSearchTimeout       time.Duration
	WebSearchTopN          int
	WebSearchSearchTTL     time.Duration
	WebSearchPageTTL       time.Duration
	WebSearchExtractTTL    time.Duration
	KnowledgeProvider      string
	WeKnoraBaseURL         string
	WeKnoraAPIKey          string
	WeKnoraTimeout         time.Duration
	KnowledgeTopN          int
	KnowledgeQueryCacheTTL time.Duration
	AnswerCacheTTL         time.Duration
	QuestionRateLimit      int
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	ShutdownTimeout        time.Duration
	AllowedOrigins         []string
	AdminToken             string
	ReportingTimeZone      string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:               env("ASKU_HTTP_ADDR", ":18080"),
		DatabaseURL:            env("ASKU_DATABASE_URL", "postgres://asku:asku_dev@localhost:55432/asku?sslmode=disable"),
		RedisAddr:              env("ASKU_REDIS_ADDR", "localhost:6385"),
		RedisPassword:          os.Getenv("ASKU_REDIS_PASSWORD"),
		SchoolConfig:           strings.TrimSpace(os.Getenv("ASKU_SCHOOL_CONFIG")),
		DevAuthEnabled:         false,
		AgentMode:              env("ASKU_AGENT_MODE", "policy"),
		LLMProvider:            env("ASKU_LLM_PROVIDER", "mock"),
		LLMBaseURL:             strings.TrimSpace(os.Getenv("ASKU_LLM_BASE_URL")),
		LLMAPIKey:              strings.TrimSpace(os.Getenv("ASKU_LLM_API_KEY")),
		LLMModel:               env("ASKU_LLM_MODEL", "asku-mock"),
		LLMTimeout:             45 * time.Second,
		WebSearchProvider:      env("ASKU_WEB_SEARCH_PROVIDER", "mock"),
		WebSearchBaseURL:       strings.TrimSpace(os.Getenv("ASKU_WEB_SEARCH_BASE_URL")),
		WebSearchAPIKey:        strings.TrimSpace(os.Getenv("ASKU_WEB_SEARCH_API_KEY")),
		WebSearchTimeout:       12 * time.Second,
		WebSearchTopN:          3,
		WebSearchSearchTTL:     10 * time.Minute,
		WebSearchPageTTL:       30 * time.Minute,
		WebSearchExtractTTL:    30 * time.Minute,
		KnowledgeProvider:      env("ASKU_KNOWLEDGE_PROVIDER", "disabled"),
		WeKnoraBaseURL:         strings.TrimSpace(os.Getenv("ASKU_WEKNORA_BASE_URL")),
		WeKnoraAPIKey:          strings.TrimSpace(os.Getenv("ASKU_WEKNORA_API_KEY")),
		WeKnoraTimeout:         12 * time.Second,
		KnowledgeTopN:          4,
		KnowledgeQueryCacheTTL: 10 * time.Minute,
		AnswerCacheTTL:         30 * time.Minute,
		QuestionRateLimit:      30,
		AccessTokenTTL:         time.Hour,
		RefreshTokenTTL:        30 * 24 * time.Hour,
		ShutdownTimeout:        10 * time.Second,
		AllowedOrigins:         splitCSV(env("ASKU_CORS_ORIGINS", "*")),
		AdminToken:             strings.TrimSpace(os.Getenv("ASKU_ADMIN_TOKEN")),
		ReportingTimeZone:      env("ASKU_REPORTING_TIMEZONE", "Asia/Shanghai"),
	}
	var err error
	if cfg.DevAuthEnabled, err = envBool("ASKU_DEV_AUTH_ENABLED", cfg.DevAuthEnabled); err != nil {
		return Config{}, err
	}
	if cfg.LLMTimeout, err = envDuration("ASKU_LLM_TIMEOUT", cfg.LLMTimeout); err != nil {
		return Config{}, err
	}
	if cfg.WebSearchTimeout, err = envDuration("ASKU_WEB_SEARCH_TIMEOUT", cfg.WebSearchTimeout); err != nil {
		return Config{}, err
	}
	if cfg.WebSearchTopN, err = envPositiveInt("ASKU_WEB_SEARCH_TOP_N", cfg.WebSearchTopN); err != nil {
		return Config{}, err
	}
	if cfg.WebSearchSearchTTL, err = envDuration("ASKU_WEB_SEARCH_SEARCH_TTL", cfg.WebSearchSearchTTL); err != nil {
		return Config{}, err
	}
	if cfg.WebSearchPageTTL, err = envDuration("ASKU_WEB_SEARCH_PAGE_TTL", cfg.WebSearchPageTTL); err != nil {
		return Config{}, err
	}
	if cfg.WebSearchExtractTTL, err = envDuration("ASKU_WEB_SEARCH_EXTRACT_TTL", cfg.WebSearchExtractTTL); err != nil {
		return Config{}, err
	}
	if cfg.WeKnoraTimeout, err = envDuration("ASKU_WEKNORA_TIMEOUT", cfg.WeKnoraTimeout); err != nil {
		return Config{}, err
	}
	if cfg.KnowledgeTopN, err = envPositiveInt("ASKU_KNOWLEDGE_TOP_N", cfg.KnowledgeTopN); err != nil {
		return Config{}, err
	}
	if cfg.KnowledgeQueryCacheTTL, err = envDuration("ASKU_KNOWLEDGE_QUERY_CACHE_TTL", cfg.KnowledgeQueryCacheTTL); err != nil {
		return Config{}, err
	}
	if cfg.AnswerCacheTTL, err = envDuration("ASKU_ANSWER_CACHE_TTL", cfg.AnswerCacheTTL); err != nil {
		return Config{}, err
	}
	if cfg.QuestionRateLimit, err = envPositiveInt("ASKU_QUESTION_RATE_LIMIT_PER_MINUTE", cfg.QuestionRateLimit); err != nil {
		return Config{}, err
	}
	if cfg.AccessTokenTTL, err = envDuration("ASKU_ACCESS_TOKEN_TTL", cfg.AccessTokenTTL); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTokenTTL, err = envDuration("ASKU_REFRESH_TOKEN_TTL", cfg.RefreshTokenTTL); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("ASKU_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.LLMInputPrice, err = envNonNegativeFloat("ASKU_LLM_INPUT_RMB_PER_MTOK", 0); err != nil {
		return Config{}, err
	}
	if cfg.LLMOutputPrice, err = envNonNegativeFloat("ASKU_LLM_OUTPUT_RMB_PER_MTOK", 0); err != nil {
		return Config{}, err
	}
	if cfg.AgentMode != "mock" && cfg.AgentMode != "policy" {
		return Config{}, fmt.Errorf("unsupported ASKU_AGENT_MODE %q", cfg.AgentMode)
	}
	if cfg.LLMProvider != "mock" && cfg.LLMProvider != "openai-compatible" {
		return Config{}, fmt.Errorf("unsupported ASKU_LLM_PROVIDER %q", cfg.LLMProvider)
	}
	if cfg.LLMProvider == "openai-compatible" {
		if cfg.LLMBaseURL == "" || cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
			return Config{}, errors.New("openai-compatible provider requires ASKU_LLM_BASE_URL, ASKU_LLM_API_KEY and ASKU_LLM_MODEL")
		}
	}
	if cfg.WebSearchProvider != "mock" && cfg.WebSearchProvider != "searxng" {
		return Config{}, fmt.Errorf("unsupported ASKU_WEB_SEARCH_PROVIDER %q", cfg.WebSearchProvider)
	}
	if cfg.WebSearchProvider == "searxng" && cfg.WebSearchBaseURL == "" {
		return Config{}, errors.New("searxng provider requires ASKU_WEB_SEARCH_BASE_URL")
	}
	if cfg.KnowledgeProvider != "disabled" && cfg.KnowledgeProvider != "weknora" {
		return Config{}, fmt.Errorf("unsupported ASKU_KNOWLEDGE_PROVIDER %q", cfg.KnowledgeProvider)
	}
	if cfg.KnowledgeProvider == "weknora" && (cfg.WeKnoraBaseURL == "" || cfg.WeKnoraAPIKey == "") {
		return Config{}, errors.New("weknora provider requires ASKU_WEKNORA_BASE_URL and ASKU_WEKNORA_API_KEY")
	}
	if cfg.WebSearchTimeout <= 0 || cfg.WebSearchSearchTTL <= 0 || cfg.WebSearchPageTTL <= 0 || cfg.WebSearchExtractTTL <= 0 {
		return Config{}, errors.New("web search timeout and cache TTLs must be positive")
	}
	if cfg.WebSearchTopN < 1 || cfg.WebSearchTopN > 5 {
		return Config{}, errors.New("ASKU_WEB_SEARCH_TOP_N must be between 1 and 5")
	}
	if cfg.WeKnoraTimeout <= 0 {
		return Config{}, errors.New("WeKnora timeout must be positive")
	}
	if cfg.KnowledgeTopN < 1 || cfg.KnowledgeTopN > 10 {
		return Config{}, errors.New("ASKU_KNOWLEDGE_TOP_N must be between 1 and 10")
	}
	if cfg.QuestionRateLimit > 1000 {
		return Config{}, errors.New("ASKU_QUESTION_RATE_LIMIT_PER_MINUTE must be between 1 and 1000")
	}
	if _, err := time.LoadLocation(cfg.ReportingTimeZone); err != nil {
		return Config{}, fmt.Errorf("invalid ASKU_REPORTING_TIMEZONE %q: %w", cfg.ReportingTimeZone, err)
	}
	return cfg, nil
}

func envNonNegativeFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", key)
	}
	return parsed, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func envPositiveInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
