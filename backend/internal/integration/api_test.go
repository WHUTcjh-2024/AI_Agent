package integration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"asku/backend/internal/domain"
	"asku/backend/internal/observability"
)

func TestAPIHybridRoundTrip(t *testing.T) {
	h := newHarness(t, nil, false)
	h.json("GET", "/healthz", "", nil, nil, 200)
	session := h.session()
	run := h.send(session.ID, "今年校历怎么安排？")
	events := h.events(run.ID, 0, false)
	terminal(t, events, "run.completed")
	assertRoute(t, events, "hybrid")
	h.status(run.ID, "COMPLETED")
	retrieval := payload[struct {
		Mode      string `json:"retrievalMode"`
		Knowledge struct {
			Configured bool `json:"configured"`
		} `json:"knowledgeStats"`
		Degraded []string `json:"degradedCapabilities"`
	}](t, events, "retrieval.completed")
	if retrieval.Mode != "hybrid" || retrieval.Knowledge.Configured || !reflect.DeepEqual(retrieval.Degraded, []string{"knowledge"}) {
		t.Fatalf("missing disabled knowledge metadata: %#v", retrieval)
	}
	message := finalMessage(t, events)
	if message.Status != "completed" || message.Content == "" || len(message.Citations) != 1 || len(message.SourceIDs) != 1 {
		t.Fatalf("incomplete answer: %#v", message)
	}
	var delta strings.Builder
	for _, e := range events {
		if e.Type == "message.delta" {
			var item struct {
				Delta     string `json:"delta"`
				MessageID string `json:"messageId"`
			}
			if err := json.Unmarshal(e.Data, &item); err != nil {
				t.Fatal(err)
			}
			if item.MessageID != message.ID {
				t.Fatal("stream message id changed")
			}
			delta.WriteString(item.Delta)
		}
	}
	if delta.String() != message.Content {
		t.Fatal("streamed answer differs from final answer")
	}
	messages := h.messages(session.ID)
	if len(messages) != 2 || !equalMessages(messages[1], message) {
		t.Fatalf("history differs from message.completed: %#v", messages)
	}
	sources := payload[struct {
		Sources []domain.Source `json:"sources"`
	}](t, events, "sources.updated").Sources
	citation := message.Citations[0]
	if len(sources) != 1 || citation.Index != 1 || citation.SourceID != sources[0].ID || citation.SourceID != message.SourceIDs[0] || citation.EvidenceText == "" || citation.CitationID == "" {
		t.Fatal("source/citation mapping is inconsistent")
	}
	var source domain.Source
	h.json("GET", "/v1/sources/"+citation.SourceID, h.tokens.AccessToken, nil, &source, 200)
	if !source.Official || source.URL != "https://university.example/eval" || citation.OfficialURL != source.URL {
		t.Fatal("citation source is not the expected fixture URL")
	}
	h.json("POST", "/v1/feedback", h.tokens.AccessToken, map[string]string{"messageId": message.ID, "value": "unhelpful"}, nil, 201)
	var overview observability.Overview
	h.json("GET", "/v1/admin/overview", h.adminToken, nil, &overview, 200)
	if overview.Engagement.Questions != 1 || overview.Quality.CompletedRuns != 1 || overview.Quality.Unhelpful != 1 || overview.Cost.InputTokens <= 0 || overview.Cost.OutputTokens <= 0 {
		t.Fatalf("Admin did not aggregate test run: %#v", overview)
	}
	if len(overview.Routes) != 1 || overview.Routes[0].Name != "hybrid" || overview.Routes[0].Count != 1 {
		t.Fatalf("incorrect route aggregation: %#v", overview.Routes)
	}
	h.json("GET", "/v1/admin/overview", h.tokens.AccessToken, nil, nil, 401)
	if h.count("SELECT count(*) FROM usage_records WHERE run_id=$1", run.ID) != 1 {
		t.Fatal("Usage must be recorded once")
	}
	h.json("DELETE", "/v1/sessions/"+session.ID, h.tokens.AccessToken, nil, nil, 204)
	if h.count("SELECT count(*) FROM message_citations WHERE message_id=$1", message.ID) != 0 {
		t.Fatal("session delete left citations behind")
	}
}

func TestAPIKnowledgeDisabled(t *testing.T) {
	h := newHarness(t, nil, false)
	session := h.session()
	run := h.send(session.ID, "学校宿舍允许饲养宠物吗？")
	events := h.events(run.ID, 0, false)
	terminal(t, events, "run.completed")
	assertRoute(t, events, "knowledge")
	message := finalMessage(t, events)
	if len(message.Citations) != 0 || len(message.SourceIDs) != 0 || !strings.Contains(message.Content, "可靠") {
		t.Fatalf("disabled Knowledge invented a supported answer: %#v", message)
	}
	if len(h.messages(session.ID)) != 2 || h.web.calls.Load() != 0 || h.count("SELECT count(*) FROM usage_records") != 0 {
		t.Fatal("disabled Knowledge must persist a controlled answer without paid calls")
	}
}

func TestAPIReconnect(t *testing.T) {
	web := &fixtureWeb{entered: make(chan struct{}, 1), release: make(chan struct{})}
	h := newHarness(t, web, false)
	run := h.send(h.session().ID, "今年校历怎么安排？")
	waitEntered(t, web)
	response, scanner := h.openStream(run.ID, 0, false)
	first, err := nextEvent(scanner)
	if err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if first.Type != "run.started" {
		t.Fatal("expected first durable event")
	}
	close(web.release)
	resumed := h.events(run.ID, first.ID, true)
	terminal(t, resumed, "run.completed")
	h.status(run.ID, "COMPLETED")
	full := h.events(run.ID, 0, false)
	if len(full) != len(resumed)+1 || !equalEvents(full[1:], resumed) {
		t.Fatal("Last-Event-ID replay lost or duplicated events")
	}
	queryResume := h.events(run.ID, first.ID, false)
	if !equalEvents(queryResume, resumed) {
		t.Fatal("after and Last-Event-ID disagree")
	}
	if events := h.events(run.ID, full[len(full)-1].ID, true); len(events) != 0 {
		t.Fatal("terminal cursor replayed duplicate events")
	}
}

func TestAPICancelDuringRetrieval(t *testing.T) {
	web := &fixtureWeb{entered: make(chan struct{}, 1), release: make(chan struct{})}
	h := newHarness(t, web, false)
	session := h.session()
	run := h.send(session.ID, "今年校历怎么安排？")
	waitEntered(t, web)
	h.json("POST", "/v1/runs/"+run.ID+"/cancel", h.tokens.AccessToken, nil, nil, 204)
	events := h.events(run.ID, 0, false)
	terminal(t, events, "run.failed")
	failure := payload[struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}](t, events, "run.failed")
	if failure.Code != "cancelled" || failure.Retryable {
		t.Fatalf("cancel misclassified as provider failure: %#v", failure)
	}
	h.status(run.ID, "CANCELLED")
	h.json("POST", "/v1/runs/"+run.ID+"/cancel", h.tokens.AccessToken, nil, nil, 204)
	terminal(t, h.events(run.ID, 0, false), "run.failed")
	if len(h.messages(session.ID)) != 1 || h.count("SELECT count(*) FROM usage_records") != 0 {
		t.Fatal("cancelled retrieval produced an answer or LLM call")
	}
}

func TestAPIFailureThenRetry(t *testing.T) {
	web := &fixtureWeb{failFirst: true}
	h := newHarness(t, web, false)
	session := h.session()
	failed := h.send(session.ID, "今年校历怎么安排？")
	events := h.events(failed.ID, 0, false)
	terminal(t, events, "run.failed")
	failure := payload[struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}](t, events, "run.failed")
	if failure.Code != "web_search_provider_error" || !failure.Retryable {
		t.Fatalf("wrong failure contract: %#v", failure)
	}
	h.status(failed.ID, "FAILED")
	if len(h.messages(session.ID)) != 1 || h.count("SELECT count(*) FROM usage_records") != 0 {
		t.Fatal("failed retrieval fabricated an answer or charged LLM usage")
	}
	retry := h.send(session.ID, "今年校历怎么安排？")
	terminal(t, h.events(retry.ID, 0, false), "run.completed")
	if web.calls.Load() != 2 || h.count("SELECT count(*) FROM usage_records") != 1 || len(h.messages(session.ID)) != 3 {
		t.Fatal("retry did not recover exactly once")
	}
}

func TestAPIIdempotencyAndIsolation(t *testing.T) {
	web := &fixtureWeb{entered: make(chan struct{}, 1), release: make(chan struct{})}
	h := newHarness(t, web, false)
	session := h.session()
	body := map[string]string{"question": "今年校历怎么安排？", "userMessageId": "eval-" + uniqueID()}
	headers := map[string]string{"Idempotency-Key": uniqueID()}
	type response struct {
		status int
		data   []byte
		err    error
	}
	responses := make(chan response, 2)
	for range 2 {
		go func() {
			status, data, err := h.request("POST", "/v1/sessions/"+session.ID+"/messages", h.tokens.AccessToken, body, headers)
			responses <- response{status, data, err}
		}()
	}
	accepted, rejected := 0, 0
	var run domain.AgentRun
	for range 2 {
		result := <-responses
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case 202:
			accepted++
			var value struct {
				Run domain.AgentRun `json:"run"`
			}
			if err := json.Unmarshal(result.data, &value); err != nil {
				t.Fatal(err)
			}
			run = value.Run
		case 409:
			rejected++
		default:
			t.Fatalf("unexpected idempotency HTTP status: %d", result.status)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatal("concurrent duplicate must start exactly one run")
	}
	waitEntered(t, web)
	other := h.login()
	h.json("GET", "/v1/sessions/"+session.ID, other.AccessToken, nil, nil, 404)
	h.json("GET", "/v1/runs/"+run.ID+"/events", other.AccessToken, nil, nil, 404)
	h.json("POST", "/v1/runs/"+run.ID+"/cancel", other.AccessToken, nil, nil, 404)
	close(web.release)
	events := h.events(run.ID, 0, false)
	terminal(t, events, "run.completed")
	message := finalMessage(t, events)
	// Official source metadata is school-scoped and intentionally shared;
	// conversations and runs remain account-scoped.
	h.json("GET", "/v1/sources/"+message.Citations[0].SourceID, other.AccessToken, nil, nil, 200)
	h.execSQL("UPDATE users SET current_school_id='other-school' WHERE id=$1", other.User.ID)
	h.json("GET", "/v1/sources/"+message.Citations[0].SourceID, other.AccessToken, nil, nil, 404)
	if h.count("SELECT count(*) FROM agent_runs") != 1 || h.count("SELECT count(*) FROM usage_records") != 1 || len(h.messages(session.ID)) != 2 {
		t.Fatal("duplicate request created duplicate facts or costs")
	}
	// A failed start must release its reservation so a valid retry can proceed.
	badHeaders := map[string]string{"Idempotency-Key": uniqueID()}
	status, _, err := h.request("POST", "/v1/sessions/missing/messages", h.tokens.AccessToken, map[string]string{"question": "你好"}, badHeaders)
	if err != nil || status != 404 {
		t.Fatal("expected rejected start for missing session")
	}
	status, data, err := h.request("POST", "/v1/sessions/"+session.ID+"/messages", h.tokens.AccessToken, map[string]string{"question": "你好"}, badHeaders)
	if err != nil || status != 202 {
		t.Fatal("failed start retained idempotency reservation")
	}
	var retry struct {
		Run domain.AgentRun `json:"run"`
	}
	if err := json.Unmarshal(data, &retry); err != nil {
		t.Fatal(err)
	}
	terminal(t, h.events(retry.Run.ID, 0, false), "run.completed")
}
