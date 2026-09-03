package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"asku/backend/internal/domain"
)

type event struct {
	ID   int64
	Type string
	Data json.RawMessage
}

func equalEvents(a, b []event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Type != b[i].Type {
			return false
		}
		var left, right any
		if json.Unmarshal(a[i].Data, &left) != nil || json.Unmarshal(b[i].Data, &right) != nil || !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return true
}

func equalMessages(a, b domain.Message) bool {
	// PostgreSQL stores microsecond precision and may return a different zone.
	normalize := func(m domain.Message) domain.Message {
		m.CreatedAt = m.CreatedAt.UTC().Truncate(time.Microsecond)
		m.Citations = append([]domain.Citation{}, m.Citations...)
		for i := range m.Citations {
			m.Citations[i].PublishDate = m.Citations[i].PublishDate.UTC().Truncate(time.Microsecond)
		}
		return m
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func nextEvent(scanner *bufio.Scanner) (event, error) {
	var e event
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if e.Type == "" && len(data) == 0 {
				continue
			}
			e.Data = json.RawMessage(strings.Join(data, "\n"))
			if e.ID <= 0 || e.Type == "" || !json.Valid(e.Data) {
				return e, fmt.Errorf("invalid SSE envelope")
			}
			return e, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return e, fmt.Errorf("invalid SSE id")
			}
			e.ID = id
		case "event":
			e.Type = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return e, err
	}
	if e.Type != "" || len(data) != 0 {
		return e, fmt.Errorf("truncated SSE frame")
	}
	return e, io.EOF
}

func (h *harness) openStream(runID string, after int64, header bool) (*http.Response, *bufio.Scanner) {
	h.t.Helper()
	path := h.server.URL + "/v1/runs/" + runID + "/events"
	if !header {
		path += "?after=" + strconv.FormatInt(after, 10)
	}
	request, err := http.NewRequest("GET", path, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+h.tokens.AccessToken)
	request.Header.Set("Accept", "text/event-stream")
	if header {
		request.Header.Set("Last-Event-ID", strconv.FormatInt(after, 10))
	}
	response, err := h.client.Do(request)
	if err != nil {
		h.t.Fatal(err)
	}
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		response.Body.Close()
		h.t.Fatalf("invalid SSE status/content type: %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 65536), 4<<20)
	return response, scanner
}

func (h *harness) events(runID string, after int64, header bool) []event {
	h.t.Helper()
	response, scanner := h.openStream(runID, after, header)
	defer response.Body.Close()
	events := []event{}
	last := after
	for {
		e, err := nextEvent(scanner)
		if err == io.EOF {
			break
		}
		if err != nil {
			h.t.Fatal(err)
		}
		if e.ID <= last {
			h.t.Fatal("duplicate or out-of-order SSE event")
		}
		last = e.ID
		events = append(events, e)
		if len(events) > 1000 {
			h.t.Fatal("unexpected SSE event budget exceeded")
		}
	}
	return events
}

func payload[T any](t *testing.T, events []event, kind string) T {
	t.Helper()
	var result T
	found := false
	for _, e := range events {
		if e.Type != kind {
			continue
		}
		if found {
			t.Fatalf("duplicate %s event", kind)
		}
		if err := json.Unmarshal(e.Data, &result); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	if !found {
		t.Fatalf("missing %s event", kind)
	}
	return result
}

func terminal(t *testing.T, events []event, expected string) {
	t.Helper()
	count := 0
	for _, e := range events {
		if e.Type == "run.completed" || e.Type == "run.failed" {
			count++
		}
	}
	if count != 1 || len(events) == 0 || events[len(events)-1].Type != expected {
		t.Fatalf("expected one final %s event, got %#v", expected, events)
	}
}

func finalMessage(t *testing.T, events []event) domain.Message {
	t.Helper()
	return payload[struct {
		Message domain.Message `json:"message"`
	}](t, events, "message.completed").Message
}

func assertRoute(t *testing.T, events []event, expected string) {
	t.Helper()
	actual := payload[struct {
		Route string `json:"route"`
	}](t, events, "route.resolved")
	if actual.Route != expected {
		t.Fatalf("route=%s expected=%s", actual.Route, expected)
	}
}

func TestSSEDecoderRejectsTruncatedAndMalformedFrames(t *testing.T) {
	for _, input := range []string{"id: 1\nevent: done\ndata: {}", "id: x\nevent: done\ndata: {}\n\n", "id: 1\nevent: done\ndata: nope\n\n"} {
		if _, err := nextEvent(bufio.NewScanner(strings.NewReader(input))); err == nil {
			t.Fatalf("accepted invalid SSE: %q", input)
		}
	}
	valid := ": heartbeat\r\n\r\nid: 5\r\nevent: message\r\ndata: {\r\ndata: \"ok\": true}\r\n\r\n"
	e, err := nextEvent(bufio.NewScanner(strings.NewReader(valid)))
	if err != nil || e.ID != 5 || !reflect.DeepEqual(string(e.Data), "{\n\"ok\": true}") {
		t.Fatalf("multiline SSE parse failed: %#v %v", e, err)
	}
}
