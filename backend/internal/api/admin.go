package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"asku/backend/internal/httpx"
	"asku/backend/internal/observability"
)

const maxAdminWindow = 90 * 24 * time.Hour

type AdminReporter interface {
	Overview(ctx context.Context, window observability.Window) (observability.Overview, error)
}

type AdminOptions struct {
	Reporter AdminReporter
	Token    string
	TimeZone string
}

func (s *Server) adminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/admin/overview", s.handleAdminOverview)
	return s.requireAdmin(mux)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(s.adminToken))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.admin == nil || s.adminToken == "" {
			httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusNotFound, Code: "not_found", Message: "接口不存在。"})
			return
		}
		provided := strings.TrimSpace(r.Header.Get("Authorization"))
		provided = strings.TrimSpace(strings.TrimPrefix(provided, "Bearer "))
		actual := sha256.Sum256([]byte(provided))
		if provided == "" || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="asku-admin"`)
			httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusUnauthorized, Code: "admin_unauthorized", Message: "管理凭证无效。"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	window, err := parseAdminWindow(r, s.schools.Current().ID, s.reportingTimeZone, time.Now())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	report, reportErr := s.admin.Overview(r.Context(), window)
	if reportErr != nil {
		httpx.Error(w, r, reportErr)
		return
	}
	httpx.JSON(w, http.StatusOK, report)
}

func parseAdminWindow(r *http.Request, currentSchoolID, timeZone string, now time.Time) (observability.Window, error) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("schoolId"))
	if schoolID == "" {
		schoolID = currentSchoolID
	}
	if schoolID != currentSchoolID {
		return observability.Window{}, badRequest("invalid_school", "当前服务未开放该学校的管理数据。")
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return observability.Window{}, err
	}
	to := now.UTC()
	from := to.Add(-7 * 24 * time.Hour)
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		from, err = parseReportTime(raw, location, false)
		if err != nil {
			return observability.Window{}, badRequest("invalid_time_window", "from 必须是 RFC3339 时间或 YYYY-MM-DD 日期。")
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		to, err = parseReportTime(raw, location, true)
		if err != nil {
			return observability.Window{}, badRequest("invalid_time_window", "to 必须是 RFC3339 时间或 YYYY-MM-DD 日期。")
		}
	}
	if !from.Before(to) || to.Sub(from) > maxAdminWindow {
		return observability.Window{}, badRequest("invalid_time_window", "统计时间窗必须大于 0 且不能超过 90 天。")
	}
	return observability.Window{SchoolID: schoolID, From: from.UTC(), To: to.UTC(), TimeZone: timeZone}, nil
}

func parseReportTime(value string, location *time.Location, endOfDate bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDate {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return parsed.UTC(), nil
}
