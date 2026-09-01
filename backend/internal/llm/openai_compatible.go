package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type OpenAICompatibleProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompatibleProvider(baseURL, apiKey, model string, client *http.Client) (*OpenAICompatibleProvider, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("ASKU_LLM_BASE_URL must be an absolute HTTP(S) URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("ASKU_LLM_BASE_URL must use HTTP or HTTPS")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("ASKU_LLM_API_KEY is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("ASKU_LLM_MODEL is required")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAICompatibleProvider{baseURL: baseURL, apiKey: apiKey, model: model, client: client}, nil
}

func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible" }

func (p *OpenAICompatibleProvider) Generate(ctx context.Context, request Request) (Response, error) {
	payload := p.payload(request, false)
	var result chatCompletionResponse
	if err := p.doJSON(ctx, payload, &result); err != nil {
		return Response{}, err
	}
	if len(result.Choices) == 0 {
		return Response{}, &ProviderError{Code: "empty_response", Retryable: true}
	}
	return Response{
		Content: result.Choices[0].Message.Content,
		Model:   firstNonEmpty(result.Model, payload.Model),
		Usage:   Usage{InputTokens: result.Usage.PromptTokens, OutputTokens: result.Usage.CompletionTokens},
	}, nil
}

func (p *OpenAICompatibleProvider) Stream(ctx context.Context, request Request) (<-chan StreamEvent, error) {
	payload := p.payload(request, true)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode llm request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create llm request: %w", err)
	}
	p.setHeaders(httpRequest)
	httpRequest.Header.Set("Accept", "text/event-stream")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, &ProviderError{Code: "transport_error", Retryable: true, Cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, statusError(response.StatusCode)
	}

	events := make(chan StreamEvent)
	go p.readStream(ctx, response.Body, payload.Model, events)
	return events, nil
}

type chatCompletionRequest struct {
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	Temperature   *float64  `json:"temperature,omitempty"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAICompatibleProvider) payload(request Request, stream bool) chatCompletionRequest {
	model := request.Model
	if model == "" {
		model = p.model
	}
	payload := chatCompletionRequest{
		Model: model, Messages: request.Messages, Temperature: request.Temperature,
		MaxTokens: request.MaxTokens, Stream: stream,
	}
	if stream {
		payload.StreamOptions = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
	}
	return payload
}

func (p *OpenAICompatibleProvider) doJSON(ctx context.Context, payload chatCompletionRequest, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode llm request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create llm request: %w", err)
	}
	p.setHeaders(request)
	response, err := p.client.Do(request)
	if err != nil {
		return &ProviderError{Code: "transport_error", Retryable: true, Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return statusError(response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(target); err != nil {
		return &ProviderError{Code: "invalid_response", Retryable: true, Cause: err}
	}
	return nil
}

func (p *OpenAICompatibleProvider) readStream(ctx context.Context, body io.ReadCloser, fallbackModel string, events chan<- StreamEvent) {
	defer close(events)
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var content strings.Builder
	var usage Usage
	model := fallbackModel
	completed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			completed = true
			break
		}
		var chunk chatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			sendStreamEvent(ctx, events, StreamEvent{Err: &ProviderError{Code: "invalid_stream", Retryable: true, Cause: err}})
			return
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			usage = Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			content.WriteString(choice.Delta.Content)
			if !sendStreamEvent(ctx, events, StreamEvent{Delta: choice.Delta.Content}) {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		sendStreamEvent(ctx, events, StreamEvent{Err: &ProviderError{Code: "stream_read_error", Retryable: true, Cause: err}})
		return
	}
	if !completed {
		sendStreamEvent(ctx, events, StreamEvent{Err: &ProviderError{Code: "stream_incomplete", Retryable: true}})
		return
	}
	final := Response{Content: content.String(), Model: model, Usage: usage}
	sendStreamEvent(ctx, events, StreamEvent{Response: &final})
}

func (p *OpenAICompatibleProvider) setHeaders(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
}

func statusError(statusCode int) error {
	return &ProviderError{
		Code:       "upstream_http_error",
		StatusCode: statusCode,
		Retryable:  statusCode == http.StatusTooManyRequests || statusCode >= 500,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func sendStreamEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
