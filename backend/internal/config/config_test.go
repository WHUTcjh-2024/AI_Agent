package config

import "testing"

func TestLoadRejectsIncompleteOpenAICompatibleConfiguration(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "openai-compatible")
	t.Setenv("ASKU_LLM_BASE_URL", "")
	t.Setenv("ASKU_LLM_API_KEY", "")
	t.Setenv("ASKU_LLM_MODEL", "model")
	if _, err := Load(); err == nil {
		t.Fatal("incomplete provider configuration must fail at startup")
	}
}

func TestLoadRejectsNegativePricing(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_LLM_INPUT_RMB_PER_MTOK", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("negative token pricing must be rejected")
	}
}

func TestLoadMockProviderDefaults(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_DEV_AUTH_ENABLED", "")
	t.Setenv("ASKU_LLM_INPUT_RMB_PER_MTOK", "")
	t.Setenv("ASKU_LLM_OUTPUT_RMB_PER_MTOK", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMProvider != "mock" || cfg.LLMModel == "" || cfg.AgentMode != "policy" || cfg.KnowledgeProvider != "disabled" {
		t.Fatalf("unexpected mock defaults: %#v", cfg)
	}
	if cfg.DevAuthEnabled {
		t.Fatal("development login must be disabled by default")
	}
}

func TestLoadRejectsIncompleteWeKnoraConfiguration(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_KNOWLEDGE_PROVIDER", "weknora")
	t.Setenv("ASKU_WEKNORA_BASE_URL", "")
	t.Setenv("ASKU_WEKNORA_API_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("incomplete WeKnora configuration must fail at startup")
	}
}

func TestLoadRejectsInvalidKnowledgeTopN(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_KNOWLEDGE_PROVIDER", "disabled")
	t.Setenv("ASKU_KNOWLEDGE_TOP_N", "11")
	if _, err := Load(); err == nil {
		t.Fatal("invalid knowledge Top-N must fail at startup")
	}
}

func TestLoadRejectsSearXNGWithoutBaseURL(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_WEB_SEARCH_PROVIDER", "searxng")
	t.Setenv("ASKU_WEB_SEARCH_BASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing web search URL error")
	}
}

func TestLoadRejectsInvalidWebSearchTopN(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_WEB_SEARCH_PROVIDER", "mock")
	t.Setenv("ASKU_WEB_SEARCH_TOP_N", "7")
	if _, err := Load(); err == nil {
		t.Fatal("expected Top-N validation error")
	}
}

func TestLoadRejectsMalformedOperationalSettings(t *testing.T) {
	t.Setenv("ASKU_LLM_PROVIDER", "mock")
	t.Setenv("ASKU_WEB_SEARCH_PROVIDER", "mock")
	t.Setenv("ASKU_DEV_AUTH_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid boolean error")
	}

	t.Setenv("ASKU_DEV_AUTH_ENABLED", "true")
	t.Setenv("ASKU_LLM_TIMEOUT", "forever")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}
