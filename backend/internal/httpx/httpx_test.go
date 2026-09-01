package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"known":"ok","unknown":1}`, `{"known":"ok"}{"known":"again"}`} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var target struct {
			Known string `json:"known"`
		}
		if err := Decode(request, &target); err == nil {
			t.Fatalf("expected strict decode error for %s", body)
		}
	}
}

func TestDecodeAcceptsSingleJSONObject(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"known":"ok"}`))
	var target struct {
		Known string `json:"known"`
	}
	if err := Decode(request, &target); err != nil {
		t.Fatal(err)
	}
	if target.Known != "ok" {
		t.Fatalf("unexpected target: %#v", target)
	}
}
