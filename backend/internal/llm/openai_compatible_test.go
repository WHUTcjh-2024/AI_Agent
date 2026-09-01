package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["model"] != "configured-model" || request["stream"] != nil {
			t.Fatalf("unexpected payload: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"resolved-model","choices":[{"message":{"content":"回答"}}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret", "configured-model", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "问题"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "回答" || response.Model != "resolved-model" || response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 7 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestOpenAICompatibleStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(server.URL, "secret", "configured-model", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	var content string
	var final *Response
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		content += event.Delta
		if event.Response != nil {
			final = event.Response
		}
	}
	if content != "第一段第二段" || final == nil || final.Content != content || final.Usage.InputTokens != 9 {
		t.Fatalf("unexpected stream result: %q %#v", content, final)
	}
}

func TestOpenAICompatibleErrorDoesNotLeakSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret vendor payload", http.StatusUnauthorized)
	}))
	defer server.Close()
	provider, err := NewOpenAICompatibleProvider(server.URL, "top-secret-key", "model", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if strings.Contains(err.Error(), "top-secret-key") || strings.Contains(err.Error(), "secret vendor payload") {
		t.Fatalf("provider error leaked sensitive data: %v", err)
	}
}
