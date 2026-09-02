package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"asku/backend/internal/observability"
)

func TestAdminAuthorizationIsIndependentAndDisabledByDefault(t *testing.T) {
	server := &Server{}
	handler := server.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/overview", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled admin API must be hidden, got %d", response.Code)
	}

	server.admin = stubAdminReporter{}
	server.adminToken = "correct-token"
	response = httptest.NewRecorder()
	handler = server.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request = httptest.NewRequest(http.MethodGet, "/v1/admin/overview", nil)
	request.Header.Set("Authorization", "Bearer wrong-token")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token must be rejected, got %d", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/admin/overview", nil)
	request.Header.Set("Authorization", "Bearer correct-token")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid token must pass, got %d", response.Code)
	}
}

func TestParseAdminWindowUsesSchoolTimezoneAndHalfOpenDates(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/overview?from=2026-08-01&to=2026-08-07", nil)
	window, err := parseAdminWindow(request, "whut", "Asia/Shanghai", time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := window.From.Format(time.RFC3339); got != "2026-07-31T16:00:00Z" {
		t.Fatalf("unexpected from: %s", got)
	}
	if got := window.To.Format(time.RFC3339); got != "2026-08-07T16:00:00Z" {
		t.Fatalf("date to must include the selected local day, got %s", got)
	}
}

func TestParseAdminWindowRejectsCrossSchoolAndOversizedRange(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/overview?schoolId=other", nil)
	if _, err := parseAdminWindow(request, "whut", "Asia/Shanghai", time.Now()); err == nil {
		t.Fatal("cross-school query must fail")
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/admin/overview?from=2026-01-01&to=2026-08-01", nil)
	if _, err := parseAdminWindow(request, "whut", "Asia/Shanghai", time.Now()); err == nil {
		t.Fatal("oversized query must fail")
	}
}

type stubAdminReporter struct{}

func (stubAdminReporter) Overview(_ context.Context, _ observability.Window) (observability.Overview, error) {
	return observability.Overview{}, nil
}
